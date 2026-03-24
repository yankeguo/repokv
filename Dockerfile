FROM golang:1.25-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/repokv .

FROM debian:13-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates git curl tini && \
    rm -rf /var/lib/apt/lists/*

ENTRYPOINT ["/usr/bin/tini", "--"]

WORKDIR /app

COPY --from=builder /out/repokv /app/repokv

CMD ["/app/repokv"]
