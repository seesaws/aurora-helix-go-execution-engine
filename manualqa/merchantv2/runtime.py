#!/usr/bin/env python3
"""Own an isolated loopback Go/Temporal runtime; never contact shared services."""

import argparse
import http.client
import json
import os
from pathlib import Path
import secrets
import shutil
import signal
import socket
import stat
import subprocess
import time
from urllib.parse import urlsplit


REPO = Path(__file__).resolve().parents[2]
ROOT = Path("/tmp/aurora-merchant-sop-completion-20261021/go-runtime")
TEMPORAL = "127.0.0.1:17243"
PORTS = (17243, 18243, 18096)
ROLES = ("service-worker", "agent-executor", "event-worker")


def private_file(path, value):
    with path.open("w") as handle:
        handle.write(value)
    path.chmod(0o600)


def state():
    path = ROOT / "processes.json"
    return json.loads(path.read_text()) if path.exists() else {}


def save(value):
    private_file(ROOT / "processes.json", json.dumps(value, indent=2) + "\n")


def running(item):
    command = subprocess.run(
        ["ps", "-p", str(item["pid"]), "-o", "command="],
        capture_output=True, text=True, check=False,
    ).stdout.strip()
    return command == item["command"]


def listening(port):
    with socket.socket() as sock:
        sock.settimeout(0.3)
        return sock.connect_ex(("127.0.0.1", port)) == 0


def launch(role, args, env=None):
    items = state()
    if role in items and running(items[role]):
        raise RuntimeError(role + " is already running")
    with (ROOT / "logs" / (role + ".log")).open("ab") as log:
        process = subprocess.Popen(
            args, env=env, cwd=REPO, stdout=log, stderr=subprocess.STDOUT,
            start_new_session=True,
        )
    items[role] = {"pid": process.pid, "command": " ".join(args)}
    save(items)
    return process


def prepare(cli):
    for port in PORTS:
        if listening(port):
            raise RuntimeError("refusing occupied dedicated port " + str(port))
    ROOT.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    if stat.S_IMODE(ROOT.parent.stat().st_mode) != 0o700:
        raise RuntimeError("task parent directory must have mode 700")
    for folder in (ROOT, ROOT / "bin", ROOT / "logs", ROOT / "secrets", ROOT / "temporal"):
        folder.mkdir(mode=0o700, exist_ok=True)
        folder.chmod(0o700)
    for name in ("execution-service-token", "sop-callback-token"):
        path = ROOT / "secrets" / name
        if not path.exists():
            with path.open("x") as handle:
                handle.write(secrets.token_urlsafe(48))
        path.chmod(0o600)
    for role in ROLES:
        subprocess.run(["rtk", "go", "build", "-o", str(ROOT / "bin" / role), "./cmd/" + role], cwd=REPO, check=True)
        (ROOT / "bin" / role).chmod(0o700)
    shutil.copy2(cli, ROOT / "bin" / "temporal")
    (ROOT / "bin" / "temporal").chmod(0o700)
    print("Prepared dedicated binaries and private machine credential files; contents omitted.")


def health():
    result = subprocess.run(
        [str(ROOT / "bin" / "temporal"), "operator", "cluster", "health", "--address", TEMPORAL, "--output", "json"],
        capture_output=True, text=True, timeout=5, check=False,
    )
    return result.returncode == 0


def temporal():
    if any(listening(port) for port in PORTS[:2]):
        raise RuntimeError("refusing occupied dedicated Temporal port")
    process = launch("temporal", [
        str(ROOT / "bin" / "temporal"), "server", "start-dev", "--ip", "127.0.0.1",
        "--port", "17243", "--ui-ip", "127.0.0.1", "--ui-port", "18243",
        "--ui-disable-news-fetch", "--db-filename", str(ROOT / "temporal" / "temporal.sqlite"),
    ])
    deadline = time.monotonic() + 45
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError("dedicated Temporal exited; inspect private log")
        if health():
            print("Dedicated Temporal healthy at " + TEMPORAL + "; persistent SQLite enabled.")
            return
        time.sleep(0.2)
    raise RuntimeError("dedicated Temporal health timeout; inspect private log")


def workers(base):
    parsed = urlsplit(base)
    if parsed.scheme != "http" or parsed.hostname not in ("127.0.0.1", "localhost", "::1") or parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise RuntimeError("callback must be an explicit isolated loopback Java URL")
    if not health() or listening(18096):
        raise RuntimeError("dedicated Temporal must be healthy and Go ingress port free")
    env = os.environ.copy()
    env.update(TEMPORAL_HOST_PORT=TEMPORAL, TEMPORAL_NAMESPACE="default", HTTP_HOST="127.0.0.1", PORT="18096", SOP_BASE_URL=base)
    env["EXECUTION_SERVICE_TOKEN"] = (ROOT / "secrets" / "execution-service-token").read_text().strip()
    env["SOP_SERVICE_TOKEN"] = (ROOT / "secrets" / "sop-callback-token").read_text().strip()
    processes = [launch(role, [str(ROOT / "bin" / role)], env) for role in ROLES]
    private_file(ROOT / "configuration.json", json.dumps({key: env[key] for key in ("TEMPORAL_HOST_PORT", "TEMPORAL_NAMESPACE", "HTTP_HOST", "PORT", "SOP_BASE_URL")}, indent=2) + "\n")
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        if any(process.poll() is not None for process in processes):
            raise RuntimeError("a dedicated Go worker exited; inspect private logs")
        try:
            conn = http.client.HTTPConnection("127.0.0.1", 18096, timeout=1)
            conn.request("GET", "/healthz")
            ok = conn.getresponse().status == 200
            conn.close()
            if ok:
                print("Dedicated Go ingress healthy at http://127.0.0.1:18096; real Java callback configured.")
                return
        except OSError:
            pass
        time.sleep(0.1)
    raise RuntimeError("dedicated Go ingress health timeout")


def status():
    print(json.dumps({"processes": {role: {"pid": item["pid"], "running": running(item)} for role, item in state().items()}, "listening": {str(port): listening(port) for port in PORTS}, "temporalHealthy": health()}, indent=2))


def stop():
    items = state()
    for role, item in reversed(list(items.items())):
        if running(item):
            os.killpg(item["pid"], signal.SIGTERM)
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline and any(running(item) for item in items.values()):
        time.sleep(0.2)
    for role, item in items.items():
        if running(item):
            os.killpg(item["pid"], signal.SIGKILL)
    result = {"ownedPids": {role: item["pid"] for role, item in items.items()}, "listeningAfterStop": {str(port): listening(port) for port in PORTS}}
    private_file(ROOT / "cleanup.json", json.dumps(result, indent=2) + "\n")
    print(json.dumps(result, indent=2))
    if any(result["listeningAfterStop"].values()):
        raise RuntimeError("dedicated port remains occupied; do not stop unrelated processes")


if __name__ == "__main__":
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("prepare").add_argument("--temporal-cli", required=True)
    commands.add_parser("temporal")
    commands.add_parser("workers").add_argument("--callback-base-url", required=True)
    commands.add_parser("status")
    commands.add_parser("stop")
    args = parser.parse_args()
    if args.command == "prepare":
        prepare(Path(args.temporal_cli))
    elif args.command == "temporal":
        temporal()
    elif args.command == "workers":
        workers(args.callback_base_url)
    elif args.command == "status":
        status()
    else:
        stop()
