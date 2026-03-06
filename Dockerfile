# -------------------------------------------------------------------------
# Stage 1: Build
# -------------------------------------------------------------------------
FROM ubuntu:24.04 AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    cmake \
    gcc \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# Copy only WinQuake sources (game data is mounted at runtime)
COPY WinQuake/ /src/

RUN cmake -B build \
        -DHEADLESS=ON \
        -DNOASM=ON \
        -DCMAKE_BUILD_TYPE=Release \
        -DCMAKE_C_FLAGS="-DNOASM=1" \
    && cmake --build build --parallel "$(nproc)"

# -------------------------------------------------------------------------
# Stage 2: Runtime
# -------------------------------------------------------------------------
FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Non-root user for security (use -o to allow reuse if UID 1000 exists)
RUN useradd -r -u 1000 -o -s /usr/sbin/nologin quake

COPY --from=builder /src/build/quake-worker /usr/local/bin/quake-worker
COPY scripts/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# Game data is downloaded at startup via managed identity
RUN mkdir -p /game && chown quake:quake /game

USER quake

# Health endpoint
EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=5s --start-period=30s --retries=3 \
    CMD curl -sf http://localhost:8080/healthz || exit 1

# Environment variable defaults (override in ACA / docker run)
ENV QUAKE_BASEDIR=/game \
    QUAKE_MAP=e1m1 \
    QUAKE_SKILL=1 \
    QUAKE_MEM_MB=32 \
    QUAKE_WIDTH=640 \
    QUAKE_HEIGHT=480

ENTRYPOINT ["entrypoint.sh"]
