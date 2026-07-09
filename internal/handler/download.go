package handler

import (
	"bufio"
	"context"
	"fmt"
	"mime"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"github.com/nebu-25/MVdownloderS_0709/internal/middleware"
	"github.com/nebu-25/MVdownloderS_0709/internal/service"
)

func Download(ytdlp *service.YTDLP, logger zerolog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		mediaURL := c.Query("url")
		formatID := c.Query("format_id")
		if err := service.ValidateMediaURL(mediaURL); err != nil {
			return sendError(c, fiber.StatusBadRequest, "INVALID_URL", "지원하지 않거나 올바르지 않은 URL입니다.")
		}
		if err := service.ValidateFormatID(formatID); err != nil {
			return sendError(c, fiber.StatusBadRequest, "INVALID_FORMAT", err.Error())
		}

		ctx, cancel := context.WithCancel(c.UserContext())

		c.Type("mp4")
		c.Set(fiber.HeaderContentDisposition, contentDisposition("download.mp4"))
		c.Set(fiber.HeaderCacheControl, "no-store")
		release := middleware.ClaimConcurrency(c)
		c.Context().SetBodyStreamWriter(func(writer *bufio.Writer) {
			defer release()
			defer cancel()
			err := ytdlp.Stream(ctx, mediaURL, formatID, writer, func(line string) {
				logger.Debug().Str("message", line).Msg("yt-dlp")
			})
			if err != nil {
				logger.Error().Err(err).Msg("download stream failed")
			}
		})
		return nil
	}
}

func contentDisposition(filename string) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filepath.Base(filename), ext)
	if base == "" {
		base = "download"
	}
	value := fmt.Sprintf("%s%s", base, ext)
	return mime.FormatMediaType("attachment", map[string]string{"filename": value})
}
