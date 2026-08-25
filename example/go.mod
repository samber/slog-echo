module example

go 1.25.0

replace github.com/samber/slog-echo/v2 => ../

require (
	github.com/labstack/echo/v5 v5.3.1
	github.com/samber/slog-echo/v2 v2.1.0
	github.com/samber/slog-formatter v1.0.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/samber/lo v1.53.0 // indirect
	github.com/samber/slog-multi v1.0.0 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)
