package apic

import (
	"context"
	"time"

	"nhooyr.io/websocket"
)

type WSClient struct {
	// endpoint is the server endpoint
	endpoint string

	// logger logs each message sent and received
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

	// reconnect makes the client reconnect on error
	reconnect bool
}

func NewWSClient(endpoint string, opts ...WSOption) *WSClient {
	w := &WSClient{
		logger:   noLogger{},
		endpoint: endpoint,
		encoder:  defaultEncoder,
		handler:  func(_ []byte) error { return nil },
		onOpen:   func(_ *WSClient) error { return nil },
		onClose:  func(_ *WSClient) error { return nil },
	}

	for _, opt := range opts {
		opt(w)
	}

	return w
}

func (c *WSClient) Start(ctx context.Context) error {
	for {
		err := c.run(ctx)
		if !c.reconnect {
			return err
		}
		c.logger.Info("disconnected", "error", err)
		time.Sleep(time.Second)
		c.logger.Info("reconnecting...")
	}
}

func (c *WSClient) Write(ctx context.Context, obj any) error {
	bts, err := c.encoder(obj)
	if err != nil {
		return err
	}
	c.logger.Info("send", "message", string(bts))
	if ctx == nil {
		ctx = context.Background()
	}
	return c.conn.Write(ctx, websocket.MessageText, bts)
}

func (c *WSClient) run(ctx context.Context) error {
	if err := c.connect(ctx); err != nil {
		return err
	}
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

	for {
		select {
		case bts := <-data:
			c.logger.Info("recv", "message", string(bts))
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

func (c *WSClient) connect(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, c.endpoint, nil)
	if err != nil {
		return err
	}
	conn.SetReadLimit(-1) // that's just like, my opinion or whatever
	c.conn = conn
	return nil
}

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
