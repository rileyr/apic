package apic

import (
	"math/rand"
	"time"

	"nhooyr.io/websocket"
)

type WSOption func(*WSClient)

// WithWSLogger sets the logger for the websocket client.
func WithWSLogger(l Logger) WSOption {
	return func(c *WSClient) {
		c.logger = l
	}
}

// WithPingInterval sets the ping interval
func WithPingInterval(i time.Duration) WSOption {
	return func(c *WSClient) {
		c.pingInterval = i
	}
}

// WithWSHandler sets the global message handler for the client.
func WithWSHandler(fn func([]byte) error) WSOption {
	return func(c *WSClient) {
		c.handler = fn
	}
}

// WithWSOnOpen sets the callback called whenver a new connection is opened.
func WithWSOnOpen(fn func(*WSClient) error) WSOption {
	return func(c *WSClient) {
		c.onOpen = fn
	}
}

// WithWSOnOpen sets the callback called whenver a connection is closed.
func WithWSOnClose(fn func(*WSClient) error) WSOption {
	return func(c *WSClient) {
		c.onClose = fn
	}
}

// WithWSEncoder sets the encoder for objects written to the client
func WithWSEncoder(fn func(any) ([]byte, error)) WSOption {
	return func(c *WSClient) {
		c.encoder = fn
	}
}

// WithReconnect enables exponential backoff behavior on reconnect.
func WithReconnectBackoff(maxBackoff time.Duration) WSOption {
	return func(c *WSClient) {
		const (
			minMillis = 5
			maxMillis = 999
		)
		var (
			count int
		)
		c.shouldReconnect = func(err error) bool {
			count++
			mills := rand.Intn(maxMillis-minMillis) + minMillis
			d := time.Millisecond * time.Duration((16^count)+mills)
			if d > maxBackoff {
				d = maxBackoff
			}
			t := time.NewTicker(d)
			c.logger.Info("reconnect backoff", "duration", d.String())
			<-t.C
			t.Stop()
			return true
		}
	}
}

type DialOptions = websocket.DialOptions

// WithDialOptions allows callers to inject dial options in to the underlying lib.
func WithDialOptions(fn func() (*DialOptions, error)) WSOption {
	return func(c *WSClient) {
		c.dialOptionsFunc = fn
	}
}

// WithStaleDetection, if configured, will create a goroutine that asserts on the websocket
// having received some message within some recent time interval. If the assertion fails, the connection
// is closed, and whatever reconnect behavior has been configured will take over.
func WithStaleDetection(timeout time.Duration) WSOption {
	return func(c *WSClient) {
		c.staleMessageTimeout = timeout
	}
}
