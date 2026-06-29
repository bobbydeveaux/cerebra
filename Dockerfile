# Cerebra — CGO build (mattn/go-sqlite3 + sqlite-vec cgo bindings + sqlite_fts5).
# README's "pure-Go/no-CGO" note is stale; the code requires CGO. See gopher's study.
FROM golang:1.25-bookworm AS build
# sqlite-vec cgo bindings #include "sqlite3.h" — provided by libsqlite3-dev.
# (macOS builds find it via the SDK; Debian needs the -dev package.)
RUN apt-get update && apt-get install -y --no-install-recommends libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -tags "sqlite_fts5" -o /out/cerebra .

# Runtime: debian-slim has glibc (needed by the cgo build); distroless/scratch would
# need a static build which the sqlite-vec cgo bindings don't give us.
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates libsqlite3-0 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/cerebra /usr/local/bin/cerebra
# Default data dir for the multi-tenant HTTP transport (per-user DBs live under here).
ENV CEREBRA_DATA_DIR=/data
ENTRYPOINT ["cerebra"]
