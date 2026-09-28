package slogecho

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel/trace"
)

const (
	customAttributesCtxKey = "slog-echo.custom-attributes"
)

var (
	TraceIDKey   = "trace_id"
	SpanIDKey    = "span_id"
	RequestIDKey = "id"

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

	WithUserAgent      bool
	WithRequestID      bool
	WithRequestBody    bool
	WithRequestHeader  bool
	WithResponseBody   bool
	WithResponseHeader bool
	WithSpanID         bool
	WithTraceID        bool
	WithClientIP       bool
	WithCustomMessage  func(c *echo.Context, err error) string

	Filters []Filter
}

// New returns a echo.MiddlewareFunc (middleware) that logs requests using slog.
//
// Requests with errors are logged using slog.Error().
// Requests without errors are logged using slog.Info().
func New(logger *slog.Logger) echo.MiddlewareFunc {
	return NewWithConfig(logger, DefaultConfig())
}

// NewWithFilters returns a echo.MiddlewareFunc (middleware) that logs requests using slog.
//
// Requests with errors are logged using slog.Error().
// Requests without errors are logged using slog.Info().
func NewWithFilters(logger *slog.Logger, filters ...Filter) echo.MiddlewareFunc {
	config := DefaultConfig()
	config.Filters = filters
	return NewWithConfig(logger, config)
}

// DefaultConfig returns the default configuration for the request logger.
func DefaultConfig() Config {
	return Config{
		DefaultLevel:     slog.LevelInfo,
		ClientErrorLevel: slog.LevelWarn,
		ServerErrorLevel: slog.LevelError,

		WithUserAgent:      false,
		WithRequestID:      true,
		WithRequestBody:    false,
		WithRequestHeader:  false,
		WithResponseBody:   false,
		WithResponseHeader: false,
		WithSpanID:         false,
		WithTraceID:        false,
		WithClientIP:       true,
		WithCustomMessage:  nil,

		Filters: []Filter{},
	}
}

// NewWithConfig returns a echo.HandlerFunc (middleware) that logs requests using slog.
func NewWithConfig(logger *slog.Logger, config Config) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) (err error) {
			req := c.Request()
			start := time.Now()
			path := req.URL.Path
			query := req.URL.RawQuery

			params := map[string]string{}
			for _, p := range c.PathValues() {
				params[p.Name] = p.Value
			}

			// dump request body
			br := newBodyReader(req.Body, RequestBodyMaxSize, config.WithRequestBody)
			req.Body = br

			// dump response body
			bw := newBodyWriter(c.Response(), ResponseBodyMaxSize, config.WithResponseBody)
			c.SetResponse(bw)

			err = next(c)

			// Pass thru filters and skip early the code below, to prevent unnecessary processing.
			for _, filter := range config.Filters {
				if !filter(c, err) {
					return
				}
			}

			_, status := echo.ResolveResponseStatus(c.Response(), err)
			method := req.Method
			host := req.Host
			route := c.Path()
			end := time.Now()
			latency := end.Sub(start)
			userAgent := req.UserAgent()
			ip := c.RealIP()
			referer := c.Request().Referer()

			errMsg := ""

			// errors.As walks the whole error tree (%w chains and errors.Join), so an
			// *echo.HTTPError nested in another error is found, like Echo's own error handler does.
			var httpErr *echo.HTTPError
			if err != nil {
				if errors.As(err, &httpErr) {
					errMsg = httpErr.Message
				} else {
					errMsg = errorString(err)
				}
			}

			baseAttributes := make([]slog.Attr, 0, 3)
			requestAttributes := make([]slog.Attr, 0, 14)
			responseAttributes := make([]slog.Attr, 0, 6)

			requestAttributes = append(requestAttributes,
				slog.Time("time", start.UTC()),
				slog.String("method", method),
				slog.String("host", host),
				slog.String("path", path),
				slog.String("query", query),
				slog.Any("params", params),
				slog.String("route", route),
				slog.String("referer", referer),
			)

			if config.WithClientIP {
				requestAttributes = append(requestAttributes,
					slog.String("ip", ip),
				)
			}

			responseAttributes = append(responseAttributes,
				slog.Time("time", end.UTC()),
				slog.Duration("latency", latency),
				slog.Int("status", status),
			)

			if config.WithRequestID {
				requestID := req.Header.Get(echo.HeaderXRequestID)
				if requestID == "" {
					requestID = c.Response().Header().Get(echo.HeaderXRequestID)
				}
				if requestID != "" {
					baseAttributes = append(baseAttributes, slog.String(RequestIDKey, requestID))
				}
			}

			// otel
			baseAttributes = append(baseAttributes, extractTraceSpanID(c.Request().Context(), config.WithTraceID, config.WithSpanID)...)

			// request body
			requestAttributes = append(requestAttributes, slog.Int("length", br.bytes))
			if config.WithRequestBody {
				requestAttributes = append(requestAttributes, slog.String("body", br.body.String()))
			}

			// request headers
			if config.WithRequestHeader {
				kv := []any{}

				for k, v := range c.Request().Header {
					if _, found := HiddenRequestHeaders[strings.ToLower(k)]; found {
						continue
					}
					kv = append(kv, slog.Any(k, v))
				}

				requestAttributes = append(requestAttributes, slog.Group("header", kv...))
			}

			if config.WithUserAgent {
				requestAttributes = append(requestAttributes, slog.String("user-agent", userAgent))
			}

			xForwardedFor, ok := c.Get(echo.HeaderXForwardedFor).(string)
			if ok && len(xForwardedFor) > 0 {
				ips := lo.Map(strings.Split(xForwardedFor, ","), func(ip string, _ int) string {
					return strings.TrimSpace(ip)
				})
				requestAttributes = append(requestAttributes, slog.Any("x-forwarded-for", ips))
			}

			// response body
			responseAttributes = append(responseAttributes, slog.Int("length", bw.bytes))
			if config.WithResponseBody {
				responseAttributes = append(responseAttributes, slog.String("body", bw.body.String()))
			}

			// response headers
			if config.WithResponseHeader {
				kv := []any{}

				for k, v := range c.Response().Header() {
					if _, found := HiddenResponseHeaders[strings.ToLower(k)]; found {
						continue
					}
					kv = append(kv, slog.Any(k, v))
				}

				responseAttributes = append(responseAttributes, slog.Group("header", kv...))
			}

			attributes := append(
				[]slog.Attr{
					{
						Key:   "request",
						Value: slog.GroupValue(requestAttributes...),
					},
					{
						Key:   "response",
						Value: slog.GroupValue(responseAttributes...),
					},
				},
				baseAttributes...,
			)

			// custom context values
			if v := c.Get(customAttributesCtxKey); v != nil {
				switch attrs := v.(type) {
				case []slog.Attr:
					attributes = append(attributes, attrs...)
				}
			}

			level := config.DefaultLevel
			msg := "Incoming request"

			if status >= http.StatusInternalServerError {
				level = config.ServerErrorLevel
				if err != nil {
					msg = errMsg
				} else {
					msg = http.StatusText(status)
				}
			} else if status >= http.StatusBadRequest && status < http.StatusInternalServerError {
				level = config.ClientErrorLevel
				if err != nil {
					msg = errMsg
				} else {
					msg = http.StatusText(status)
				}
			}

			if err != nil {
				// error.code describes the error itself, not the wire status: once the response
				// is committed, status holds the committed code and ignores err.
				errAttr := map[string]any{
					"code":    http.StatusInternalServerError,
					"message": errMsg,
				}

				// Only an *echo.HTTPError carries a wrapped cause distinct from its own message
				// (via .Wrap). For any other error, the error itself is the internal cause.
				var internal error
				var sc echo.HTTPStatusCoder
				if httpErr != nil {
					errAttr["code"] = httpErr.Code
					internal = httpErr.Unwrap()
					// A nested *echo.HTTPError (fmt.Errorf("load user: %w", httpErr), errors.Join)
					// only holds part of the error: keep the full error so the outer context is
					// logged. Comparing interfaces cannot panic here: different dynamic types
					// compare unequal, and *echo.HTTPError is a comparable pointer.
					if err != error(httpErr) {
						internal = err
					}
				} else if errors.As(err, &sc) && sc.StatusCode() != 0 {
					errAttr["code"] = sc.StatusCode()
					internal = err
				} else {
					internal = err
				}

				// Always set, even when nil, to keep a stable log schema.
				errAttr["internal"] = internal
				if internal != nil {
					attributes = append(attributes, slog.String("internal", errorString(internal)))
				}

				attributes = append(attributes, slog.Any("error", errAttr))
			}

			if config.WithCustomMessage != nil {
				msg = config.WithCustomMessage(c, err)
			}

			logger.LogAttrs(c.Request().Context(), level, msg, attributes...)

			return
		}
	}
}

// AddCustomAttributes adds custom attributes to the request context.
func AddCustomAttributes(c *echo.Context, attrs ...slog.Attr) {
	v := c.Get(customAttributesCtxKey)
	if v == nil {
		c.Set(customAttributesCtxKey, attrs)
		return
	}

	switch vAttrs := v.(type) {
	case []slog.Attr:
		c.Set(customAttributesCtxKey, append(vAttrs, attrs...))
	}
}

// errorString returns err.Error() without letting a panic escape the logging middleware: a
// typed nil error (a nil *T stored in a non-nil error) panics when Error() dereferences its
// receiver.
func errorString(err error) (s string) {
	defer func() {
		if r := recover(); r != nil {
			s = fmt.Sprintf("%T: Error() panicked: %v", err, r)
		}
	}()
	return err.Error()
}

func extractTraceSpanID(ctx context.Context, withTraceID bool, withSpanID bool) []slog.Attr {
	if !withTraceID && !withSpanID {
		return []slog.Attr{}
	}

	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return []slog.Attr{}
	}

	attrs := make([]slog.Attr, 0, 2)
	spanCtx := span.SpanContext()

	if withTraceID && spanCtx.HasTraceID() {
		traceID := trace.SpanFromContext(ctx).SpanContext().TraceID().String()
		attrs = append(attrs, slog.String(TraceIDKey, traceID))
	}

	if withSpanID && spanCtx.HasSpanID() {
		spanID := spanCtx.SpanID().String()
		attrs = append(attrs, slog.String(SpanIDKey, spanID))
	}

	return attrs
}
