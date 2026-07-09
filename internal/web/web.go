package web

import (
	"embed"
	"path"

	"github.com/gofiber/fiber/v2"
)

//go:embed static/*
var staticFiles embed.FS

func Register(app *fiber.App) {
	app.Get("/", serveFile("index.html", "text/html; charset=utf-8", "no-cache"))
	app.Get("/assets/:name", func(c *fiber.Ctx) error {
		name := path.Base(c.Params("name"))
		switch path.Ext(name) {
		case ".css":
			c.Type("css", "utf-8")
		case ".js":
			c.Type("js", "utf-8")
		default:
			return fiber.ErrNotFound
		}
		c.Set(fiber.HeaderCacheControl, "public, max-age=3600")
		return sendEmbedded(c, "static/"+name)
	})
	app.Get("/favicon.ico", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})
}

func serveFile(name, contentType, cacheControl string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderContentType, contentType)
		c.Set(fiber.HeaderCacheControl, cacheControl)
		c.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' https: data:; "+
				"style-src 'self'; script-src 'self'; connect-src 'self'; "+
				"base-uri 'none'; frame-ancestors 'none'")
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("Referrer-Policy", "no-referrer")
		return sendEmbedded(c, "static/"+name)
	}
}

func sendEmbedded(c *fiber.Ctx, name string) error {
	content, err := staticFiles.ReadFile(name)
	if err != nil {
		return fiber.ErrNotFound
	}
	return c.Send(content)
}
