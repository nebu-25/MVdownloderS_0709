FROM golang:1.22-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM python:3.12-slim-bookworm
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates ffmpeg nodejs \
    && rm -rf /var/lib/apt/lists/* \
    && pip install --no-cache-dir "yt-dlp[default]"
COPY --from=builder /out/server /server
ENV PORT=8080 \
    RATE_LIMIT_PER_IP=2 \
    YTDLP_TIMEOUT=30s
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/server"]
