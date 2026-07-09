package handler

import (
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
		if err := service.ValidateFormatID(formatID, ytdlp.SupportsDASH()); err != nil {
			return sendError(c, fiber.StatusBadRequest, "INVALID_FORMAT", err.Error())
		}

		download, err := ytdlp.PrepareDownload(c.UserContext(), mediaURL, formatID)
		if err != nil {
			return extractionError(c, err)
		}
		c.Type("mp4")
		c.Set(fiber.HeaderContentDisposition, contentDisposition("download.mp4"))
		c.Set(fiber.HeaderCacheControl, "no-store")
		release := middleware.ClaimConcurrency(c)
		c.Context().SetBodyStream(&downloadBody{
			PreparedDownload: download,
			release:          release,
			logger:           logger,
		}, int(download.Size()))
		return nil
	}
}

type downloadBody struct {
	*service.PreparedDownload
	release func()
	logger  zerolog.Logger
}

func (d *downloadBody) Close() error {
	defer d.release()
	if err := d.PreparedDownload.Close(); err != nil {
		d.logger.Error().Err(err).Msg("temporary download cleanup failed")
		return err
	}
	return nil
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
