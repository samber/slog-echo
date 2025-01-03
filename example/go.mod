module example

go 1.21

replace github.com/samber/slog-echo => ../

require (
	github.com/labstack/echo/v5 v5.0.0-20230722203903-ec5b858dab61
	github.com/samber/slog-echo v1.0.0
	github.com/samber/slog-formatter v1.0.0
)

require (
	github.com/samber/lo v1.47.0 // indirect
	github.com/samber/slog-multi v1.0.0 // indirect
	github.com/stretchr/testify v1.10.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	go.opentelemetry.io/otel v1.29.0 // indirect
	go.opentelemetry.io/otel/trace v1.29.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	golang.org/x/time v0.8.0 // indirect
)
