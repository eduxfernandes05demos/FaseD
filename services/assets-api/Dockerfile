# -------------------------------------------------------------------------
# Stage 1: Build
# -------------------------------------------------------------------------
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY main.go ./

RUN go mod tidy && go build -o assets-api .

# -------------------------------------------------------------------------
# Stage 2: Runtime
# -------------------------------------------------------------------------
FROM alpine:3.19

RUN adduser -D -u 1001 assets

COPY --from=builder /app/assets-api /usr/local/bin/assets-api

USER assets

EXPOSE 8070

HEALTHCHECK --interval=10s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:8070/healthz || exit 1

ENV LISTEN_ADDR=:8070

ENTRYPOINT ["assets-api"]
