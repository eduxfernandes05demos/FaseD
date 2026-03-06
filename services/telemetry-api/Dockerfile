# -------------------------------------------------------------------------
# Stage 1: Build
# -------------------------------------------------------------------------
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY main.go ./

RUN go mod tidy && go build -o telemetry-api .

# -------------------------------------------------------------------------
# Stage 2: Runtime
# -------------------------------------------------------------------------
FROM alpine:3.19

RUN adduser -D -u 1001 telemetry

COPY --from=builder /app/telemetry-api /usr/local/bin/telemetry-api

USER telemetry

EXPOSE 8060

HEALTHCHECK --interval=10s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:8060/healthz || exit 1

ENV LISTEN_ADDR=:8060

ENTRYPOINT ["telemetry-api"]
