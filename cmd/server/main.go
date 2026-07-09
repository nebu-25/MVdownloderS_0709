package main

import (
	"os"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"github.com/nebu-25/MVdownloderS_0709/internal/handler"
	"github.com/nebu-25/MVdownloderS_0709/internal/middleware"
	"github.com/nebu-25/MVdownloderS_0709/internal/service"
	"github.com/nebu-25/MVdownloderS_0709/internal/web"
)

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	timeout := envDuration("YTDLP_TIMEOUT", 30*time.Second)
	ytdlp := service.NewYTDLP(
		envString("YTDLP_PATH", "yt-dlp"),
		envString("FFMPEG_PATH", "ffmpeg"),
		envString("FFPROBE_PATH", "ffprobe"),
		envString("MAX_DOWNLOAD_SIZE", "450M"),
		os.Getenv("POT_PROVIDER_URL"),
		timeout,
		logger,
	)

	app := fiber.New(fiber.Config{
		AppName:      "video-downloader",
		BodyLimit:    64 * 1024,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // Streaming responses may be long-running.
		ErrorHandler: handler.FiberErrorHandler,
	})
	app.Use(middleware.RequestLogger(logger))
	web.Register(app)

	api := app.Group("/api/v1")
	api.Get("/health", handler.Health)
	api.Post("/metadata", handler.Metadata(ytdlp))
	api.Get(
		"/download",
		middleware.ConcurrentByIP(envInt("RATE_LIMIT_PER_IP", 2)),
		handler.Download(ytdlp, logger),
	)

	port := envString("PORT", "8080")
	logger.Info().Str("port", port).Msg("server starting")
	if err := app.Listen(":" + port); err != nil {
		logger.Fatal().Err(err).Msg("server stopped")
	}
}

func envString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(os.Getenv(key))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
