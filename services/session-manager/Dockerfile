# -------------------------------------------------------------------------
# Stage 1: Build
# -------------------------------------------------------------------------
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY main.go ./

RUN go mod tidy && go build -o session-manager .

# -------------------------------------------------------------------------
# Stage 2: Runtime
# -------------------------------------------------------------------------
FROM alpine:3.19

RUN adduser -D -u 1001 sessionmgr

COPY --from=builder /app/session-manager /usr/local/bin/session-manager

USER sessionmgr

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:8080/healthz || exit 1

ENV LISTEN_ADDR=:8080

ENTRYPOINT ["session-manager"]
