package slogecho

import (
	"regexp"
	"slices"
	"strings"

	"github.com/labstack/echo/v5"
)

type Filter func(ctx *echo.Context, err error) bool

// Basic
func Accept(filter Filter) Filter { return filter }
func Ignore(filter Filter) Filter {
	return func(ctx *echo.Context, err error) bool { return !filter(ctx, err) }
}

// Method
func AcceptMethod(methods ...string) Filter {
	for i := range methods {
		methods[i] = strings.ToLower(methods[i])
	}

	return func(c *echo.Context, err error) bool {
		return slices.Contains(methods, strings.ToLower(c.Request().Method))
	}
}

func IgnoreMethod(methods ...string) Filter {
	for i := range methods {
		methods[i] = strings.ToLower(methods[i])
	}

	return func(c *echo.Context, err error) bool {
		return !slices.Contains(methods, strings.ToLower(c.Request().Method))
	}
}

// Status
func AcceptStatus(statuses ...int) Filter {
	return func(c *echo.Context, err error) bool {
		_, gotStatus := echo.ResolveResponseStatus(c.Response(), err)
		return slices.Contains(statuses, gotStatus)
	}
}

func IgnoreStatus(statuses ...int) Filter {
	return func(c *echo.Context, err error) bool {
		_, gotStatus := echo.ResolveResponseStatus(c.Response(), err)
		return !slices.Contains(statuses, gotStatus)
	}
}

func AcceptStatusGreaterThan(status int) Filter {
	return func(c *echo.Context, err error) bool {
		_, gotStatus := echo.ResolveResponseStatus(c.Response(), err)
		return gotStatus > status
	}
}

func AcceptStatusGreaterThanOrEqual(status int) Filter {
	return func(c *echo.Context, err error) bool {
		_, gotStatus := echo.ResolveResponseStatus(c.Response(), err)
		return gotStatus >= status
	}
}

func AcceptStatusLessThan(status int) Filter {
	return func(c *echo.Context, err error) bool {
		_, gotStatus := echo.ResolveResponseStatus(c.Response(), err)
		return gotStatus < status
	}
}

func AcceptStatusLessThanOrEqual(status int) Filter {
	return func(c *echo.Context, err error) bool {
		_, gotStatus := echo.ResolveResponseStatus(c.Response(), err)
		return gotStatus <= status
	}
}

func IgnoreStatusGreaterThan(status int) Filter {
	return AcceptStatusLessThanOrEqual(status)
}

func IgnoreStatusGreaterThanOrEqual(status int) Filter {
	return AcceptStatusLessThan(status)
}

func IgnoreStatusLessThan(status int) Filter {
	return AcceptStatusGreaterThanOrEqual(status)
}

func IgnoreStatusLessThanOrEqual(status int) Filter {
	return AcceptStatusGreaterThan(status)
}

// Path
func AcceptPath(urls ...string) Filter {
	return func(c *echo.Context, err error) bool {
		return slices.Contains(urls, c.Request().URL.Path)
	}
}

func IgnorePath(urls ...string) Filter {
	return func(c *echo.Context, err error) bool {
		return !slices.Contains(urls, c.Request().URL.Path)
	}
}

func AcceptPathContains(parts ...string) Filter {
	return func(c *echo.Context, err error) bool {
		path := c.Request().URL.Path
		for _, part := range parts {
			if strings.Contains(path, part) {
				return true
			}
		}

		return false
	}
}

func IgnorePathContains(parts ...string) Filter {
	return func(c *echo.Context, err error) bool {
		path := c.Request().URL.Path
		for _, part := range parts {
			if strings.Contains(path, part) {
				return false
			}
		}

		return true
	}
}

func AcceptPathPrefix(prefixs ...string) Filter {
	return func(c *echo.Context, err error) bool {
		path := c.Request().URL.Path
		for _, prefix := range prefixs {
			if strings.HasPrefix(path, prefix) {
				return true
			}
		}

		return false
	}
}

func IgnorePathPrefix(prefixs ...string) Filter {
	return func(c *echo.Context, err error) bool {
		path := c.Request().URL.Path
		for _, prefix := range prefixs {
			if strings.HasPrefix(path, prefix) {
				return false
			}
		}

		return true
	}
}

func AcceptPathSuffix(suffixs ...string) Filter {
	return func(c *echo.Context, err error) bool {
		path := c.Request().URL.Path
		for _, suffix := range suffixs {
			if strings.HasSuffix(path, suffix) {
				return true
			}
		}

		return false
	}
}

func IgnorePathSuffix(suffixs ...string) Filter {
	return func(c *echo.Context, err error) bool {
		path := c.Request().URL.Path
		for _, suffix := range suffixs {
			if strings.HasSuffix(path, suffix) {
				return false
			}
		}

		return true
	}
}

func AcceptPathMatch(regs ...regexp.Regexp) Filter {
	return func(c *echo.Context, err error) bool {
		path := c.Request().URL.Path
		for _, reg := range regs {
			if reg.MatchString(path) {
				return true
			}
		}

		return false
	}
}

func IgnorePathMatch(regs ...regexp.Regexp) Filter {
	return func(c *echo.Context, err error) bool {
		path := c.Request().URL.Path
		for _, reg := range regs {
			if reg.MatchString(path) {
				return false
			}
		}

		return true
	}
}

// Host
func AcceptHost(hosts ...string) Filter {
	return func(c *echo.Context, err error) bool {
		return slices.Contains(hosts, c.Request().URL.Host)
	}
}

func IgnoreHost(hosts ...string) Filter {
	return func(c *echo.Context, err error) bool {
		return !slices.Contains(hosts, c.Request().URL.Host)
	}
}

func AcceptHostContains(parts ...string) Filter {
	return func(c *echo.Context, err error) bool {
		host := c.Request().URL.Host
		for _, part := range parts {
			if strings.Contains(host, part) {
				return true
			}
		}

		return false
	}
}

func IgnoreHostContains(parts ...string) Filter {
	return func(c *echo.Context, err error) bool {
		host := c.Request().URL.Host
		for _, part := range parts {
			if strings.Contains(host, part) {
				return false
			}
		}

		return true
	}
}

func AcceptHostPrefix(prefixs ...string) Filter {
	return func(c *echo.Context, err error) bool {
		host := c.Request().URL.Host
		for _, prefix := range prefixs {
			if strings.HasPrefix(host, prefix) {
				return true
			}
		}

		return false
	}
}

func IgnoreHostPrefix(prefixs ...string) Filter {
	return func(c *echo.Context, err error) bool {
		host := c.Request().URL.Host
		for _, prefix := range prefixs {
			if strings.HasPrefix(host, prefix) {
				return false
			}
		}

		return true
	}
}

func AcceptHostSuffix(suffixs ...string) Filter {
	return func(c *echo.Context, err error) bool {
		host := c.Request().URL.Host
		for _, suffix := range suffixs {
			if strings.HasSuffix(host, suffix) {
				return true
			}
		}

		return false
	}
}

func IgnoreHostSuffix(suffixs ...string) Filter {
	return func(c *echo.Context, err error) bool {
		host := c.Request().URL.Host
		for _, suffix := range suffixs {
			if strings.HasSuffix(host, suffix) {
				return false
			}
		}

		return true
	}
}

func AcceptHostMatch(regs ...regexp.Regexp) Filter {
	return func(c *echo.Context, err error) bool {
		host := c.Request().URL.Host
		for _, reg := range regs {
			if reg.MatchString(host) {
				return true
			}
		}

		return false
	}
}

func IgnoreHostMatch(regs ...regexp.Regexp) Filter {
	return func(c *echo.Context, err error) bool {
		host := c.Request().URL.Host
		for _, reg := range regs {
			if reg.MatchString(host) {
				return false
			}
		}

		return true
	}
}
