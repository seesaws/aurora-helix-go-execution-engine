package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrConflict = errors.New("dispatchId already exists with different semantic input")
var ErrNotFound = errors.New("dispatch not found")
var identifier = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,200}$`)
var isoDuration = regexp.MustCompile(`^PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+(?:\.\d+)?)S)?$`)

// BackoffDuration accepts the contract's ISO time duration and Go time durations.
func BackoffDuration(value string) (time.Duration, error) {
	if value == "" {
		return time.Second, nil
	}
	if parts := isoDuration.FindStringSubmatch(value); parts != nil {
		value = ""
		for i, unit := range []string{"h", "m", "s"} {
			if parts[i+1] != "" {
				value += parts[i+1] + unit
			}
		}
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration < time.Millisecond || duration > time.Hour {
		return 0, fmt.Errorf("backoff must be between 1ms and 1h")
	}
	return duration, nil
}

func NormalizeDispatch(d DispatchRecord) (DispatchRecord, error) {
	for name, value := range map[string]string{"dispatchId": d.DispatchID, "tenantId": d.TenantID, "processInstanceId": d.ProcessInstanceID, "nodeInstanceId": d.NodeInstanceID, "nodeKey": d.NodeKey, "idempotencyKey": d.IdempotencyKey} {
		if !identifier.MatchString(value) || value == "." || value == ".." {
			return d, fmt.Errorf("%s must be a nonempty identifier (max 200 characters)", name)
		}
	}
	if d.ExecutionGeneration < 1 {
		return d, fmt.Errorf("executionGeneration must be positive")
	}
	if d.TaskType != "system.echo" {
		return d, fmt.Errorf("unsupported taskType; allowed: system.echo")
	}
	if d.TimeoutSeconds == 0 {
		d.TimeoutSeconds = 30
	}
	if d.TimeoutSeconds < 1 || d.TimeoutSeconds > 3600 {
		return d, fmt.Errorf("timeoutSeconds must be between 1 and 3600")
	}
	if d.Retry.MaxAttempts == 0 {
		d.Retry.MaxAttempts = 3
	}
	if d.Retry.MaxAttempts < 1 || d.Retry.MaxAttempts > 10 {
		return d, fmt.Errorf("maxAttempts must be between 1 and 10")
	}
	backoff, err := BackoffDuration(d.Retry.Backoff)
	if err != nil {
		return d, err
	}
	d.Retry.Backoff = backoff.String()
	for key, value := range d.Metadata {
		if key != "traceId" && key != "correlationId" {
			return d, fmt.Errorf("unsupported metadata field")
		}
		if text, ok := value.(string); !ok || len(text) > 200 {
			return d, fmt.Errorf("invalid metadata value")
		}
	}
	if err := validateInput(d.Input, 0); err != nil {
		return d, err
	}
	// Legacy opaque payload cannot bypass the validated top-level task envelope.
	d.Payload = nil
	d.CreatedAt = time.Time{}
	d.DispatchStatus = ""
	return d, nil
}
func validateInput(value any, depth int) error {
	if depth > 128 {
		return fmt.Errorf("input is too deeply nested")
	}
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			switch strings.ToLower(key) {
			case "authorization", "bearer", "token", "accesstoken", "refreshtoken", "password", "secret", "callbackurl":
				return fmt.Errorf("credentials and callback targets are forbidden in input")
			}
			if err := validateInput(item, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range v {
			if err := validateInput(item, depth+1); err != nil {
				return err
			}
		}
	case json.Number:
		number := v.String()
		if len(number) > 256 {
			return fmt.Errorf("input number is too large")
		}
		if position := strings.IndexAny(number, "eE"); position >= 0 {
			exponent, err := strconv.Atoi(number[position+1:])
			if err != nil || exponent < -1000 || exponent > 1000 {
				return fmt.Errorf("input numeric exponent is out of range")
			}
		}
	}
	return nil
}

// SemanticHash includes the trusted execution envelope and normalized execution
// policy. Number spelling and object insertion order do not change identity.
func SemanticHash(d DispatchRecord) (string, error) {
	normalized, err := NormalizeDispatch(d)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err = decoder.Decode(&value); err != nil {
		return "", err
	}
	var canonical strings.Builder
	var encode func(any) error
	encode = func(value any) error {
		switch v := value.(type) {
		case nil:
			canonical.WriteString("null;")
		case bool:
			canonical.WriteString(strconv.FormatBool(v) + ";")
		case string:
			b, _ := json.Marshal(v)
			canonical.WriteString("s" + string(b) + ";")
		case json.Number:
			number, ok := new(big.Rat).SetString(v.String())
			if !ok {
				return fmt.Errorf("invalid number")
			}
			canonical.WriteString("n" + number.RatString() + ";")
		case []any:
			canonical.WriteString("[")
			for _, item := range v {
				if err := encode(item); err != nil {
					return err
				}
			}
			canonical.WriteString("]")
		case map[string]any:
			canonical.WriteString("{")
			keys := make([]string, 0, len(v))
			for key := range v {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				_ = encode(key)
				if err := encode(v[key]); err != nil {
					return err
				}
			}
			canonical.WriteString("}")
		default:
			return fmt.Errorf("unsupported JSON value")
		}
		return nil
	}
	if err = encode(value); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(canonical.String()))
	return hex.EncodeToString(sum[:]), nil
}
