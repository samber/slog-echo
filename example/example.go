package main

import (
	"net/http"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	slogecho "github.com/samber/slog-echo"
	slogformatter "github.com/samber/slog-formatter"
	"golang.org/x/exp/slog"
)

func main() {
	// Create a slog logger, which:
	//   - Logs to stdout.
	//   - RFC3339 with UTC time format.
	logger := slog.New(
		slogformatter.NewFormatterHandler(
			slogformatter.TimezoneConverter(time.UTC),
			slogformatter.TimeFormatter(time.RFC3339, nil),
		)(
			slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}),
		),
	)

	// Add an attribute to all log entries made through this logger.
	logger = logger.With("env", "production")

	// Echo instance
	e := echo.New()

	// Middleware
	e.Use(slogecho.New(logger.WithGroup("http")))
	e.Use(middleware.Recover())

	// Routes
	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello, World!")
	})
	e.GET("/error", func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusInternalServerError, "A simulated error")
	})

	// Start server
	e.Logger.Fatal(e.Start(":4240"))

	// output:
	// time=2023-04-10T14:00:00Z level=INFO msg="Success" env=production http.status=200 http.method=GET http.path=/ http.ip=::1 http.latency=25.958µs http.user-agent=curl/7.77.0 http.time=2023-04-10T14:00:00Z http.request-id=229c7fc8-64f5-4467-bc4a-940700503b0d
}
