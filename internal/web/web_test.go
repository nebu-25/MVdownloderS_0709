package web

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestRegisterServesUI(t *testing.T) {
	app := fiber.New()
	Register(app)

	response, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	if !strings.Contains(string(body), "영상 링크를") ||
		!strings.Contains(string(body), "/assets/app.js") {
		t.Fatal("root response does not contain the downloader UI")
	}
	if response.Header.Get("Content-Security-Policy") == "" {
		t.Fatal("root response is missing Content-Security-Policy")
	}
}

func TestRegisterServesAssets(t *testing.T) {
	app := fiber.New()
	Register(app)

	for _, asset := range []string{"/assets/app.css", "/assets/app.js"} {
		response, err := app.Test(httptest.NewRequest("GET", asset, nil))
		if err != nil {
			t.Fatalf("GET %s: %v", asset, err)
		}
		response.Body.Close()
		if response.StatusCode != fiber.StatusOK {
			t.Errorf("GET %s status = %d, want 200", asset, response.StatusCode)
		}
	}
}
