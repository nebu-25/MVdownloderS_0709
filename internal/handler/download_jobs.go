package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"github.com/nebu-25/MVdownloderS_0709/internal/middleware"
	"github.com/nebu-25/MVdownloderS_0709/internal/model"
	"github.com/nebu-25/MVdownloderS_0709/internal/service"
)

func DownloadJobCreate(jobs *service.DownloadJobManager, ytdlp *service.YTDLP) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var request model.DownloadJobRequest
		if err := c.BodyParser(&request); err != nil {
			return sendError(c, fiber.StatusBadRequest, "INVALID_REQUEST", "JSON 요청 본문이 올바르지 않습니다.")
		}
		if err := service.ValidateMediaURL(request.URL); err != nil {
			return sendError(c, fiber.StatusBadRequest, "INVALID_URL", "지원하지 않거나 올바르지 않은 URL입니다.")
		}
		if err := service.ValidateFormatID(request.FormatID, ytdlp.SupportsDASH()); err != nil {
			return sendError(c, fiber.StatusBadRequest, "INVALID_FORMAT", err.Error())
		}

		return c.Status(fiber.StatusAccepted).JSON(jobs.Start(request.URL, request.FormatID))
	}
}

func DownloadJobStatus(jobs *service.DownloadJobManager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		job, ok := jobs.Get(c.Params("job_id"))
		if !ok {
			return sendError(c, fiber.StatusNotFound, "JOB_NOT_FOUND", "작업을 찾을 수 없습니다.")
		}
		return c.JSON(job)
	}
}

func DownloadJobFile(jobs *service.DownloadJobManager, logger zerolog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		download, job, ok := jobs.ConsumeDownload(c.Params("job_id"))
		if !ok {
			return sendError(c, fiber.StatusNotFound, "JOB_NOT_FOUND", "작업을 찾을 수 없습니다.")
		}
		if job.Status != service.DownloadJobReady || download == nil {
			return c.Status(fiber.StatusTooEarly).JSON(job)
		}

		c.Type("mp4")
		c.Set(fiber.HeaderContentDisposition, contentDisposition("download.mp4"))
		c.Set(fiber.HeaderCacheControl, "no-store")
		release := middleware.ClaimConcurrency(c)
		c.Context().SetBodyStream(&jobDownloadBody{
			PreparedDownload: download,
			release:          release,
			logger:           logger,
		}, int(download.Size()))
		return nil
	}
}

type jobDownloadBody struct {
	*service.PreparedDownload
	release func()
	logger  zerolog.Logger
}

func (d *jobDownloadBody) Close() error {
	defer d.release()
	if err := d.PreparedDownload.Close(); err != nil {
		d.logger.Error().Err(err).Msg("temporary job download cleanup failed")
		return err
	}
	return nil
}
