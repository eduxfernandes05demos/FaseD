# -------------------------------------------------------------------------
# Stage 1: Build
# -------------------------------------------------------------------------
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY main.go ./

RUN go mod tidy && go build -o streaming-gateway .

# -------------------------------------------------------------------------
# Stage 2: Runtime
# -------------------------------------------------------------------------
FROM alpine:3.19

RUN adduser -D -u 1001 gateway

COPY --from=builder /app/streaming-gateway /usr/local/bin/streaming-gateway

USER gateway

EXPOSE 8090

HEALTHCHECK --interval=10s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:8090/healthz || exit 1

ENV LISTEN_ADDR=:8090 \
    WORKER_ADDR=localhost:9000 \
    STUN_SERVER=stun:stun.l.google.com:19302

ENTRYPOINT ["streaming-gateway"]
