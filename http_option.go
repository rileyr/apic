package apic

import (
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

type HTTPOption func(*HTTPClient)

func WithClient(c *http.Client) HTTPOption {
	return func(client *HTTPClient) {
		client.client = c
	}
}

func WithEncoder(fn func(obj any) ([]byte, error)) HTTPOption {
	return func(c *HTTPClient) {
		c.encoder = fn
	}
}

func WithDecoder(fn func([]byte, any) error) HTTPOption {
	return func(c *HTTPClient) {
		c.decoder = fn
	}
}

func WithBefore(fn func(*http.Request) error) HTTPOption {
	return func(c *HTTPClient) {
		c.before = fn
	}
}

func WithMaxStatus(code int) HTTPOption {
	return func(c *HTTPClient) {
		c.maxStatus = code
	}
}

func WithLogger(lg Logger) HTTPOption {
	return func(c *HTTPClient) {
		c.logger = lg
	}
}

func WithLoggedBodies() HTTPOption {
	return func(c *HTTPClient) {
		c.logBodies = true
	}
}

func WithRateLimit(r rate.Limit, b int) HTTPOption {
	return func(c *HTTPClient) {
		c.limiter = rate.NewLimiter(r, b)
	}
}

func WithSensitiveHeader(keys ...string) HTTPOption {
	return func(c *HTTPClient) {
		c.sensitiveHeaders = append(c.sensitiveHeaders, keys...)
	}
}

func WithHTTPTimeout(timeout time.Duration) HTTPOption {
	return func(c *HTTPClient) {
		c.client.Timeout = timeout
	}
}
