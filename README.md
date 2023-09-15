
# slog: Echo middleware

[![tag](https://img.shields.io/github/tag/samber/slog-echo.svg)](https://github.com/samber/slog-echo/releases)
![Go Version](https://img.shields.io/badge/Go-%3E%3D%201.21-%23007d9c)
[![GoDoc](https://godoc.org/github.com/samber/slog-echo?status.svg)](https://pkg.go.dev/github.com/samber/slog-echo)
![Build Status](https://github.com/samber/slog-echo/actions/workflows/test.yml/badge.svg)
[![Go report](https://goreportcard.com/badge/github.com/samber/slog-echo)](https://goreportcard.com/report/github.com/samber/slog-echo)
[![Coverage](https://img.shields.io/codecov/c/github/samber/slog-echo)](https://codecov.io/gh/samber/slog-echo)
[![Contributors](https://img.shields.io/github/contributors/samber/slog-echo)](https://github.com/samber/slog-echo/graphs/contributors)
[![License](https://img.shields.io/github/license/samber/slog-echo)](./LICENSE)

[Echo](https://github.com/labstack/echo) middleware to log http requests using [slog](https://pkg.go.dev/log/slog).

**See also:**

- [slog-multi](https://github.com/samber/slog-multi): `slog.Handler` chaining, fanout, routing, failover, load balancing...
- [slog-formatter](https://github.com/samber/slog-formatter): `slog` attribute formatting
- [slog-sampling](https://github.com/samber/slog-sampling): `slog` sampling policy
- [slog-gin](https://github.com/samber/slog-gin): Gin middleware for `slog` logger
- [slog-echo](https://github.com/samber/slog-echo): Echo middleware for `slog` logger
- [slog-fiber](https://github.com/samber/slog-fiber): Fiber middleware for `slog` logger
- [slog-datadog](https://github.com/samber/slog-datadog): A `slog` handler for `Datadog`
- [slog-rollbar](https://github.com/samber/slog-rollbar): A `slog` handler for `Rollbar`
- [slog-sentry](https://github.com/samber/slog-sentry): A `slog` handler for `Sentry`
- [slog-syslog](https://github.com/samber/slog-syslog): A `slog` handler for `Syslog`
- [slog-logstash](https://github.com/samber/slog-logstash): A `slog` handler for `Logstash`
- [slog-fluentd](https://github.com/samber/slog-fluentd): A `slog` handler for `Fluentd`
- [slog-graylog](https://github.com/samber/slog-graylog): A `slog` handler for `Graylog`
- [slog-loki](https://github.com/samber/slog-loki): A `slog` handler for `Loki`
- [slog-slack](https://github.com/samber/slog-slack): A `slog` handler for `Slack`
- [slog-telegram](https://github.com/samber/slog-telegram): A `slog` handler for `Telegram`
- [slog-mattermost](https://github.com/samber/slog-mattermost): A `slog` handler for `Mattermost`
- [slog-microsoft-teams](https://github.com/samber/slog-microsoft-teams): A `slog` handler for `Microsoft Teams`
- [slog-webhook](https://github.com/samber/slog-webhook): A `slog` handler for `Webhook`
- [slog-kafka](https://github.com/samber/slog-kafka): A `slog` handler for `Kafka`
- [slog-parquet](https://github.com/samber/slog-parquet): A `slog` handler for `Parquet` + `Object Storage`

## 🚀 Install

```sh
go get github.com/samber/slog-echo
```

**Compatibility**: go >= 1.21

No breaking changes will be made to exported APIs before v2.0.0.

## 💡 Usage

### Minimal

```go
import (
	"net/http"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	slogecho "github.com/samber/slog-echo"
	"log/slog"
)

// Create a slog logger, which:
//   - Logs to stdout.
logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

// Echo instance
e := echo.New()

// Middleware
e.Use(slogecho.New(logger))
e.Use(middleware.Recover())

// Routes
e.GET("/", func(c echo.Context) error {
	return c.String(http.StatusOK, "Hello, World!")
})
e.GET("/error", func(c echo.Context) error {
	return echo.NewHTTPError(http.StatusInternalServerError, "I'm angry")
})

// Start server
e.Logger.Fatal(e.Start(":4242"))

// output:
// time=2023-04-10T14:00:00Z level=INFO msg="Success"  status=200 method=GET path=/ ip=::1 latency=25.958µs user-agent=curl/7.77.0 time=2023-04-10T14:00:00Z request-id=229c7fc8-64f5-4467-bc4a-940700503b0d
```

### Filters

```go
import (
	"net/http"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	slogecho "github.com/samber/slog-echo"
	"log/slog"
)

logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

e := echo.New()
e.Use(
	slogecho.NewWithFilters(
		logger,
		slogecho.Accept(func (c echo.Context) bool {
			return xxx
		}),
		slogecho.IgnoreStatus(401, 404),
	),
)
```

Available filters:
- Accept / Ignore
- AcceptMethod / IgnoreMethod
- AcceptStatus / IgnoreStatus
- AcceptStatusGreaterThan / IgnoreStatusLessThan
- AcceptStatusGreaterThanOrEqual / IgnoreStatusLessThanOrEqual
- AcceptPath / IgnorePath
- AcceptPathContains / IgnorePathContains
- AcceptPathPrefix / IgnorePathPrefix
- AcceptPathSuffix / IgnorePathSuffix
- AcceptPathMatch / IgnorePathMatch
- AcceptHost / IgnoreHost
- AcceptHostContains / IgnoreHostContains
- AcceptHostPrefix / IgnoreHostPrefix
- AcceptHostSuffix / IgnoreHostSuffix
- AcceptHostMatch / IgnoreHostMatch

### Using custom time formatters

```go
import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	slogecho "github.com/samber/slog-echo"
	slogformatter "github.com/samber/slog-formatter"
	"log/slog"
)

// Create a slog logger, which:
//   - Logs to stdout.
//   - RFC3339 with UTC time format.
logger := slog.New(
	slogformatter.NewFormatterHandler(
		slogformatter.TimezoneConverter(time.UTC),
		slogformatter.TimeFormatter(time.DateTime, nil),
	)(
		slog.NewTextHandler(os.Stdout, nil),
	),
)

// Echo instance
e := echo.New()

// Middleware
e.Use(slogecho.New(logger))
e.Use(middleware.Recover())

// Routes
e.GET("/", func(c echo.Context) error {
	return c.String(http.StatusOK, "Hello, World!")
})
e.GET("/error", func(c echo.Context) error {
	return echo.NewHTTPError(http.StatusInternalServerError, "I'm angry")
})

// Start server
e.Logger.Fatal(e.Start(":4242"))

// output:
// time=2023-04-10T14:00:00Z level=INFO msg="Success"  status=200 method=GET path=/ ip=::1 latency=25.958µs user-agent=curl/7.77.0 time=2023-04-10T14:00:00Z request-id=229c7fc8-64f5-4467-bc4a-940700503b0d
```

### Using custom logger sub-group

```go
import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	slogecho "github.com/samber/slog-echo"
	"log/slog"
)

// Create a slog logger, which:
//   - Logs to stdout.
logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

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
	return echo.NewHTTPError(http.StatusInternalServerError, "I'm angry")
})

// Start server
e.Logger.Fatal(e.Start(":4242"))

// output:
// time=2023-04-10T14:00:00Z level=INFO msg="Success"  http.status=200 http.method=GET http.path=/ http.ip=::1 http.latency=25.958µs http.user-agent=curl/7.77.0 http.time=2023-04-10T14:00:00Z http.request-id=229c7fc8-64f5-4467-bc4a-940700503b0d
```

### Add logger to a single route

```go
import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	slogecho "github.com/samber/slog-echo"
	"log/slog"
)

// Create a slog logger, which:
//   - Logs to stdout.
logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

// Echo instance
e := echo.New()

// Middleware
e.Use(middleware.Recover())

// Routes
e.GET("/", func(c echo.Context) error {
	return c.String(http.StatusOK, "Hello, World!")
}, slogecho.New(logger))

// Start server
e.Logger.Fatal(e.Start(":4242"))

// output:
// time=2023-04-10T14:00:00Z level=INFO msg="Success"  status=200 method=GET path=/ ip=::1 latency=25.958µs user-agent=curl/7.77.0 time=2023-04-10T14:00:00Z request-id=229c7fc8-64f5-4467-bc4a-940700503b0d
```

### Adding custom attributes

```go
import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	slogecho "github.com/samber/slog-echo"
	"log/slog"
)

// Create a slog logger, which:
//   - Logs to stdout.
logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

// Add an attribute to all log entries made through this logger.
logger = logger.With("env", "production")

// Echo instance
e := echo.New()

// Middleware
e.Use(slogecho.New(logger))
e.Use(middleware.Recover())

// Routes
e.GET("/", func(c echo.Context) error {
	return c.String(http.StatusOK, "Hello, World!")
})

// Start server
e.Logger.Fatal(e.Start(":4242"))

// output:
// time=2023-04-10T14:00:00Z level=INFO msg="Success" env=production status=200 method=GET path=/ ip=::1 latency=25.958µs user-agent=curl/7.77.0 time=2023-04-10T14:00:00Z request-id=229c7fc8-64f5-4467-bc4a-940700503b0d
```

### JSON output

```go
import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	slogecho "github.com/samber/slog-echo"
	"log/slog"
)

// Create a slog logger, which:
//   - Logs to stdout.
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

// Echo instance
e := echo.New()

// Middleware
e.Use(slogecho.New(logger))
e.Use(middleware.Recover())

// Routes
e.GET("/", func(c echo.Context) error {
	return c.String(http.StatusOK, "Hello, World!")
})

// Start server
e.Logger.Fatal(e.Start(":4242"))

// output:
// {"time":"2023-04-10T14:00:00Z","level":"INFO","msg":"Success","env":"production","status":200,"method":"GET","path":"/","ip":"::1","latency":"25.958µs","user-agent":"curl/7.77.0","time":"2023-04-10T14:00:00Z","request-id":"04201917-d7ba-4b20-a3bb-2fffba5f2bd9"}
```

## 🤝 Contributing

- Ping me on twitter [@samuelberthe](https://twitter.com/samuelberthe) (DMs, mentions, whatever :))
- Fork the [project](https://github.com/samber/slog-echo)
- Fix [open issues](https://github.com/samber/slog-echo/issues) or request new features

Don't hesitate ;)

```bash
# Install some dev dependencies
make tools

# Run tests
make test
# or
make watch-test
```

## 👤 Contributors

![Contributors](https://contrib.rocks/image?repo=samber/slog-echo)

## 💫 Show your support

Give a ⭐️ if this project helped you!

[![GitHub Sponsors](https://img.shields.io/github/sponsors/samber?style=for-the-badge)](https://github.com/sponsors/samber)

## 📝 License

Copyright © 2023 [Samuel Berthe](https://github.com/samber).

This project is [MIT](./LICENSE) licensed.
