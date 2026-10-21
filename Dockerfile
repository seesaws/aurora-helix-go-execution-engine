# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e
ARG SOURCE_DATE_EPOCH=0
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm@sha256:ee3901c61e423732605a51f1b8157c849ec01194ea0a274213b5202b2c11e7bf AS build
ARG SOURCE_DATE_EPOCH
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download && go mod verify
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false -ldflags='-s -w -buildid=' -o /out/event-worker ./cmd/event-worker && \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false -ldflags='-s -w -buildid=' -o /out/service-worker ./cmd/service-worker && \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false -ldflags='-s -w -buildid=' -o /out/agent-executor ./cmd/agent-executor && \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false -ldflags='-s -w -buildid=' -o /out/health-probe ./cmd/health-probe && \
    mkdir -p /rootfs/app /rootfs/etc/ssl/certs && \
    cp /out/* /rootfs/app/ && \
    cp /etc/ssl/certs/ca-certificates.crt /rootfs/etc/ssl/certs/ && \
    find /rootfs -exec touch -d "@$SOURCE_DATE_EPOCH" {} +

FROM scratch AS runtime
COPY --from=build --chown=65532:65532 /rootfs/ /
USER 65532:65532
EXPOSE 8081
ENTRYPOINT ["/app/event-worker"]
