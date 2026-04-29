module example

go 1.25.0

replace github.com/samber/slog-echo => ../

require (
	github.com/labstack/echo/v5 v5.1.0
	github.com/samber/slog-echo v1.0.0
	github.com/samber/slog-formatter v1.0.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/samber/lo v1.53.0 // indirect
	github.com/samber/slog-multi v1.0.0 // indirect
	go.opentelemetry.io/otel v1.43.0 // indirect
	go.opentelemetry.io/otel/trace v1.43.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	golang.org/x/time v0.14.0 // indirect
)
