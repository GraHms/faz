# syntax=docker/dockerfile:1.7
# fazd: the Faz API and job workers. Chromium is included for the render checks.
# Untested in the author's sandbox (no Docker there); build and run it once before deploying.
FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/fazd ./cmd/fazd

FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends chromium fonts-noto-core fonts-noto-color-emoji ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --create-home --uid 10001 faz
COPY --from=build /out/fazd /usr/local/bin/fazd
# Tailwind's standalone binary compiles kit pages (FAZ_KIT=1). x64 build, pinned
# by checksum; swap the file name for tailwindcss-linux-arm64 on ARM hosts.
ADD --chmod=755 --checksum=sha256:e47695cee3f0d90623a91f3939b08bdee610eb5ad99340adff9d4145b284d6cc \
    https://github.com/tailwindlabs/tailwindcss/releases/download/v4.3.5/tailwindcss-linux-x64 /usr/local/bin/tailwindcss
COPY examples /app/examples
USER faz
WORKDIR /app
ENV FAZ_CHROME=/usr/bin/chromium FAZ_TAILWIND=/usr/local/bin/tailwindcss FAZ_HTTP_ADDR=:8080 FAZ_DATA_DIR=/home/faz/data
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/fazd"]
