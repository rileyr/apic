package apic

import (
	"context"

	"nhooyr.io/websocket"
)

type WSClient struct {
	// endpoint is the server endpoint
	endpoint string

	// logger infos connection lifecycles, and debugs each message sent and received
	logger Logger

	// conn is the (current) underlying connection
	conn *websocket.Conn

	// handler is the global message handler
	handler func([]byte) error

	// onOpen is the callback invoked after each connection is opened
	onOpen func(*WSClient) error

	// onClose is the callback invoked after each connection is closed
	onClose func(*WSClient) error

	encoder Encoder

	shouldReconnect reconnectPolicy
}

func NewWSClient(endpoint string, opts ...WSOption) *WSClient {
	w := &WSClient{
		logger:          noLogger{},
		endpoint:        endpoint,
		encoder:         defaultEncoder,
		handler:         func(_ []byte) error { return nil },
		onOpen:          func(_ *WSClient) error { return nil },
		onClose:         func(_ *WSClient) error { return nil },
		shouldReconnect: func(_ error) bool { return false },
	}

	for _, opt := range opts {
		opt(w)
	}

	return w
}

// Start runs the client until either:
// - the context is canceled
// - the reconnect policy returns false
func (c *WSClient) Start(ctx context.Context) error {
	for {
		err := c.run(ctx)
		c.logger.Info("disconnected", "error", err)
		if !c.shouldReconnect(err) {
			return err
		}
		c.logger.Info("reconnecting...")
	}
}

// Write encodes and writes an object to the current connection.
func (c *WSClient) Write(ctx context.Context, obj any) error {
	bts, err := c.encoder(obj)
	if err != nil {
		return err
	}
	c.logger.Debug("send", "message", string(bts))
	if ctx == nil {
		ctx = context.Background()
	}
	return c.conn.Write(ctx, websocket.MessageText, bts)
}

// run connects the websocket, and runs the single connection until
// either the connection is terminated, or the global handler returns
// a non nil error.
func (c *WSClient) run(ctx context.Context) error {
	if err := c.connect(ctx); err != nil {
		return err
	}
	c.logger.Info("connected")
	defer c.conn.Close(websocket.StatusInternalError, "app closing")

	readErr := make(chan error)
	data := make(chan []byte)
	go reader(c.conn, data, readErr)

	if err := c.onOpen(c); err != nil {
		return err
	}
	defer func() {
		if err := c.onClose(c); err != nil {
			c.logger.Info("onClose returned error", "error", err.Error())
		}
	}()

	c.logger.Info("starting")
	for {
		select {
		case bts := <-data:
			c.logger.Debug("recv", "message", string(bts))
			if err := c.handler(bts); err != nil {
				return err
			}
		case err := <-readErr:
			return err
		case <-ctx.Done():
			return nil
		}
	}
}

// connect creates a new connection and assigns it
// to the receiver
// TODO: the threadsafety thing, across the whole client
func (c *WSClient) connect(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, c.endpoint, nil)
	if err != nil {
		return err
	}
	conn.SetReadLimit(-1) // that's just like, my opinion or whatever
	c.conn = conn
	return nil
}

// reader is a helper func to pump messages from a connection
func reader(conn *websocket.Conn, data chan []byte, errs chan error) {
	defer close(data)
	defer close(errs)
	for {
		_, bts, err := conn.Read(context.Background())
		if err != nil {
			errs <- err
			return
		}
		data <- bts
	}
}

// reconnectPolicy configures reconnect behavior.
// if the function returns true, the client will
// attempt a reconnect.
type reconnectPolicy func(error) bool
