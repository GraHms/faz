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
COPY examples /app/examples
USER faz
WORKDIR /app
ENV FAZ_CHROME=/usr/bin/chromium FAZ_HTTP_ADDR=:8080 FAZ_DATA_DIR=/home/faz/data
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/fazd"]
