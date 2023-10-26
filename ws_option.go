package apic

type WSOption func(*WSClient)

// WithWSLogger sets the logger for the websocket client.
func WithWSLogger(l Logger) WSOption {
	return func(c *WSClient) {
		c.logger = l
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

// WithReconnect enables automatic reconnection.
func WithReconnect() WSOption {
	return func(c *WSClient) {
		c.reconnect = true
	}
}
