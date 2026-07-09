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
	switch {
	case errors.Is(err, service.ErrProviderUnavailable):
		return sendError(c, fiber.StatusBadGateway, "POT_PROVIDER_UNREACHABLE", "PO Token Provider에 연결할 수 없습니다.")
	case errors.Is(err, service.ErrSourceBlocked):
		return sendError(c, fiber.StatusBadGateway, "YTDLP_403", "영상 소스가 다운로드를 거부했습니다.")
	case errors.Is(err, service.ErrOutputNotCreated):
		return sendError(c, fiber.StatusBadGateway, "OUTPUT_NOT_CREATED", "다운로드 파일이 생성되지 않았습니다.")
	case errors.Is(err, service.ErrMediaVerificationFailed):
		return sendError(c, fiber.StatusBadGateway, "FFPROBE_FAILED", "생성된 파일 검증에 실패했습니다.")
	}
	if errors.Is(err, service.ErrExtractionFailed) {
		return sendError(c, fiber.StatusBadGateway, "EXTRACTION_FAILED", "영상 정보를 가져오지 못했습니다.")
	}
	return err
}
