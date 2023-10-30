package slogecho

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"log/slog"

	"github.com/labstack/echo/v4"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel/trace"
)

const (
	customAttributesCtxKey = "slog-echo.custom-attributes"
)

var (
	RequestBodyMaxSize  = 64 * 1024 // 64KB
	ResponseBodyMaxSize = 64 * 1024 // 64KB

	HiddenRequestHeaders = map[string]struct{}{
		"authorization": {},
		"cookie":        {},
		"set-cookie":    {},
		"x-auth-token":  {},
		"x-csrf-token":  {},
		"x-xsrf-token":  {},
	}
	HiddenResponseHeaders = map[string]struct{}{
		"set-cookie": {},
	}
)

type Config struct {
	DefaultLevel     slog.Level
	ClientErrorLevel slog.Level
	ServerErrorLevel slog.Level

	WithRequestID      bool
	WithRequestBody    bool
	WithRequestHeader  bool
	WithResponseBody   bool
	WithResponseHeader bool
	WithSpanID         bool
	WithTraceID        bool

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

		WithRequestID:      true,
		WithRequestBody:    false,
		WithRequestHeader:  false,
		WithResponseBody:   false,
		WithResponseHeader: false,
		WithSpanID:         false,
		WithTraceID:        false,

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

		WithRequestID:      true,
		WithRequestBody:    false,
		WithRequestHeader:  false,
		WithResponseBody:   false,
		WithResponseHeader: false,
		WithSpanID:         false,
		WithTraceID:        false,

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

			// dump request body
			var reqBody []byte
			if config.WithRequestBody {
				buf, err := io.ReadAll(c.Request().Body)
				if err == nil {
					c.Request().Body = io.NopCloser(bytes.NewBuffer(buf))
					if len(buf) > RequestBodyMaxSize {
						reqBody = buf[:RequestBodyMaxSize]
					} else {
						reqBody = buf
					}
				}
			}

			// dump response body
			if config.WithResponseBody {
				c.Response().Writer = newBodyWriter(c.Response().Writer, ResponseBodyMaxSize)
			}

			err = next(c)

			if err != nil {
				c.Error(err)
			}

			path := req.URL.Path
			route := c.Path()
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
				slog.String("route", route),
				slog.Int("status", status),
				slog.String("ip", ip),
				slog.String("user-agent", userAgent),
			}

			xForwardedFor, ok := c.Get(echo.HeaderXForwardedFor).(string)
			if ok && len(xForwardedFor) > 0 {
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

			// otel
			if config.WithTraceID {
				traceID := trace.SpanFromContext(c.Request().Context()).SpanContext().TraceID().String()
				attributes = append(attributes, slog.String("trace-id", traceID))
			}
			if config.WithSpanID {
				spanID := trace.SpanFromContext(c.Request().Context()).SpanContext().SpanID().String()
				attributes = append(attributes, slog.String("span-id", spanID))
			}

			// request
			if config.WithRequestBody {
				attributes = append(attributes, slog.Group("request", slog.String("body", string(reqBody))))
			}
			if config.WithRequestHeader {
				for k, v := range c.Request().Header {
					if _, found := HiddenRequestHeaders[strings.ToLower(k)]; found {
						continue
					}
					attributes = append(attributes, slog.Group("request", slog.Group("header", slog.Any(k, v))))
				}
			}

			// response
			if config.WithResponseBody {
				if w, ok := c.Response().Writer.(*bodyWriter); ok {
					attributes = append(attributes, slog.Group("response", slog.String("body", w.body.String())))
				}
			}
			if config.WithResponseHeader {
				for k, v := range c.Response().Header() {
					if _, found := HiddenResponseHeaders[strings.ToLower(k)]; found {
						continue
					}
					attributes = append(attributes, slog.Group("response", slog.Group("header", slog.Any(k, v))))
				}
			}

			// custom context values
			if v := c.Get(customAttributesCtxKey); v != nil {
				switch attrs := v.(type) {
				case []slog.Attr:
					attributes = append(attributes, attrs...)
				}
			}

			for _, filter := range config.Filters {
				if !filter(c) {
					return
				}
			}

			level := config.DefaultLevel
			msg := "Incoming request"
			if status >= http.StatusInternalServerError {
				level = config.ServerErrorLevel
				if err != nil {
					msg = err.Error()
				} else {
					msg = http.StatusText(status)
				}
			} else if status >= http.StatusBadRequest && status < http.StatusInternalServerError {
				level = config.ClientErrorLevel
				if err != nil {
					msg = err.Error()
				} else {
					msg = http.StatusText(status)
				}
			}

			logger.LogAttrs(c.Request().Context(), level, msg, attributes...)

			return
		}
	}
}

func AddCustomAttributes(c echo.Context, attr slog.Attr) {
	v := c.Get(customAttributesCtxKey)
	if v == nil {
		c.Set(customAttributesCtxKey, []slog.Attr{attr})
		return
	}

	switch attrs := v.(type) {
	case []slog.Attr:
		c.Set(customAttributesCtxKey, append(attrs, attr))
	}
}
