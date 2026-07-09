package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/nebu-25/MVdownloderS_0709/internal/model"
	"github.com/nebu-25/MVdownloderS_0709/internal/service"
)

func Metadata(ytdlp *service.YTDLP) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var request model.MetadataRequest
		if err := c.BodyParser(&request); err != nil {
			return sendError(c, fiber.StatusBadRequest, "INVALID_REQUEST", "JSON 요청 본문이 올바르지 않습니다.")
		}
		if err := service.ValidateMediaURL(request.URL); err != nil {
			return sendError(c, fiber.StatusBadRequest, "INVALID_URL", "지원하지 않거나 올바르지 않은 URL입니다.")
		}
		metadata, err := ytdlp.Metadata(c.UserContext(), request.URL)
		if err != nil {
			return extractionError(c, err)
		}
		return c.JSON(metadata)
	}
}
