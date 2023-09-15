package slogecho

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"log/slog"

	"github.com/labstack/echo/v4"
	"github.com/samber/lo"
)

type Config struct {
	DefaultLevel     slog.Level
	ClientErrorLevel slog.Level
	ServerErrorLevel slog.Level

	WithRequestID bool

	Filters []Filter
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

		Filters: []Filter{},
	})
}

// NewWithFilters returns a echo.MiddlewareFunc (middleware) that logs requests using slog.
//
// Requests with errors are logged using slog.Error().
// Requests without errors are logged using slog.Info().
func NewWithFilters(logger *slog.Logger, filters ...Filter) echo.MiddlewareFunc {
	return NewWithConfig(logger, Config{
		DefaultLevel:     slog.LevelInfo,
		ClientErrorLevel: slog.LevelWarn,
		ServerErrorLevel: slog.LevelError,

		WithRequestID: true,

		Filters: filters,
	})
}

// NewWithConfig returns a echo.HandlerFunc (middleware) that logs requests using slog.
func NewWithConfig(logger *slog.Logger, config Config) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) (err error) {
			req := c.Request()
			res := c.Response()

			start := time.Now()

			path := c.Path()
			if path == "" {
				path = req.URL.Path
			}

			err = next(c)

			if err != nil {
				c.Error(err)
			}

			status := res.Status
			method := req.Method
			end := time.Now()
			latency := end.Sub(start)
			ip := c.RealIP()
			userAgent := req.UserAgent()

			httpErr := new(echo.HTTPError)
			if err != nil && errors.As(err, &httpErr) {
				status = httpErr.Code
				if msg, ok := httpErr.Message.(string); ok {
					err = errors.New(msg)
				}
			}

			attributes := []slog.Attr{
				slog.Time("time", end),
				slog.Duration("latency", latency),
				slog.String("method", method),
				slog.String("path", path),
				slog.Int("status", status),
				slog.String("ip", ip),
				slog.String("user-agent", userAgent),
			}

			xForwardedFor := c.Get(echo.HeaderXForwardedFor).(string)
			if len(xForwardedFor) > 0 {
				ips := lo.Map(strings.Split(xForwardedFor, ","), func(ip string, _ int) string {
					return strings.TrimSpace(ip)
				})
				attributes = append(attributes, slog.Any("x-forwarded-for", ips))
			}

			if config.WithRequestID {
				requestID := req.Header.Get(echo.HeaderXRequestID)
				if requestID == "" {
					requestID = res.Header().Get(echo.HeaderXRequestID)
				}
				if requestID != "" {
					attributes = append(attributes, slog.String("request-id", requestID))
				}
			}

			for _, filter := range config.Filters {
				if !filter(c) {
					return
				}
			}

			switch {
			case status >= http.StatusInternalServerError:
				var errMsg string
				if err != nil {
					errMsg = err.Error()
				}
				logger.LogAttrs(context.Background(), config.ServerErrorLevel, errMsg, attributes...)
			case status >= http.StatusBadRequest && status < http.StatusInternalServerError:
				var errMsg string
				if err != nil {
					errMsg = err.Error()
				}
				logger.LogAttrs(context.Background(), config.ClientErrorLevel, errMsg, attributes...)
			case status >= http.StatusMultipleChoices && status < http.StatusBadRequest:
				logger.LogAttrs(context.Background(), config.DefaultLevel, "Redirection", attributes...)
			default:
				logger.LogAttrs(context.Background(), config.DefaultLevel, "Success", attributes...)
			}

			return
		}
	}
}
