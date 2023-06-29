package slogecho

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"golang.org/x/exp/slog"
)

const requestIDCtx = "slog-echo.request-id"

type Config struct {
	DefaultLevel     slog.Level
	ClientErrorLevel slog.Level
	ServerErrorLevel slog.Level

	WithRequestID bool
}

// New returns a echo.MiddlewareFunc (middleware) that logs requests using slog.
//
// Requests with errors are logged using slog.Error().
// Requests without errors are logged using slog.Info().
func New(logger *slog.Logger) echo.MiddlewareFunc {
	return NewWithConfig(logger, Config{
		DefaultLevel:     slog.LevelInfo,
		ClientErrorLevel: slog.LevelWarn,
		ServerErrorLevel: slog.LevelError,

		WithRequestID: true,
	})
}

// NewWithConfig returns a echo.HandlerFunc (middleware) that logs requests using slog.
func NewWithConfig(logger *slog.Logger, config Config) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) (err error) {
			start := time.Now()
			path := c.Path()

			requestID := uuid.New().String()
			if config.WithRequestID {
				c.Set(requestIDCtx, requestID)
				c.Response().Header().Set("X-Request-ID", requestID)
			}

			err = next(c)

			end := time.Now()
			latency := end.Sub(start)

			status := c.Response().Status

			httpErr := new(echo.HTTPError)
			if errors.As(err, &httpErr) {
				status = httpErr.Code
			}

			attributes := []slog.Attr{
				slog.Int("status", status),
				slog.String("method", c.Request().Method),
				slog.String("path", path),
				slog.String("ip", c.RealIP()),
				slog.Duration("latency", latency),
				slog.String("user-agent", c.Request().UserAgent()),
				slog.Time("time", end),
			}

			if config.WithRequestID {
				attributes = append(attributes, slog.String("request-id", requestID))
			}

			switch {
			case c.Response().Status >= http.StatusBadRequest && c.Response().Status < http.StatusInternalServerError:
				logger.LogAttrs(context.Background(), config.ClientErrorLevel, err.Error(), attributes...)
			case c.Response().Status >= http.StatusInternalServerError:
				logger.LogAttrs(context.Background(), config.ServerErrorLevel, err.Error(), attributes...)
			default:
				logger.LogAttrs(context.Background(), config.DefaultLevel, "Incoming request", attributes...)
			}

			return
		}
	}
}

// GetRequestID returns the request identifier
func GetRequestID(c echo.Context) string {
	if id, ok := c.Get(requestIDCtx).(string); ok {
		return id
	}

	return ""
}
