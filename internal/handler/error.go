package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/nebu-25/MVdownloderS_0709/internal/model"
	"github.com/nebu-25/MVdownloderS_0709/internal/service"
)

func sendError(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(model.ErrorBody{
		Error: model.ErrorDetail{Code: code, Message: message},
	})
}

func FiberErrorHandler(c *fiber.Ctx, err error) error {
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return sendError(c, fiberErr.Code, "HTTP_ERROR", fiberErr.Message)
	}
	return sendError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "서버 내부 오류가 발생했습니다.")
}

func extractionError(c *fiber.Ctx, err error) error {
	if errors.Is(err, service.ErrExtractionFailed) {
		return sendError(c, fiber.StatusBadGateway, "EXTRACTION_FAILED", "영상 정보를 가져오지 못했습니다.")
	}
	return err
}
