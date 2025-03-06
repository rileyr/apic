package apic

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/time/rate"
	"nhooyr.io/websocket"
)

type WSClient struct {
	// endpoint is the server endpoint
	endpoint string

	// endpointFunc is an optional function at that can provide dynamic endpoints
	endpointFunc func() (string, error)

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

	// dialOptionsFunc is called ahead of dialing each new connection
	dialOptionsFunc func() (*websocket.DialOptions, error)

	encoder Encoder

	pingInterval time.Duration
	pingHandler  func(context.Context, *WSClient) error

	shouldReconnect reconnectPolicy

	staleMessageTimeout time.Duration

	writeLimiter *rate.Limiter
}

func NewWSClient(endpoint string, opts ...WSOption) *WSClient {
	w := &WSClient{
		logger:          noLogger{},
		endpoint:        endpoint,
		encoder:         defaultEncoder,
		writeLimiter:    nil,
		handler:         func(_ []byte) error { return nil },
		onOpen:          func(_ *WSClient) error { return nil },
		onClose:         func(_ *WSClient) error { return nil },
		shouldReconnect: func(_ error) bool { return false },
		dialOptionsFunc: func() (*websocket.DialOptions, error) { return nil, nil },
		pingHandler:     defaultPingHandler,
	}

	for _, opt := range opts {
		opt(w)
	}

	if w.endpointFunc == nil {
		w.endpointFunc = func() (string, error) {
			return w.endpoint, nil
		}
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

var ErrNotConnected = errors.New("websocket not connected")

// Write encodes and writes an object to the current connection.
func (c *WSClient) Write(ctx context.Context, obj any) error {
	if c.conn == nil {
		return ErrNotConnected
	}
	if ctx == nil {
		ctx = context.Background()
	}

	bts, err := c.encoder(obj)
	if err != nil {
		return err
	}

	if c.writeLimiter != nil {
		if err := c.writeLimiter.Wait(ctx); err != nil {
			return err
		}
	}

	c.logger.Debug("send", "message", string(bts))
	return c.conn.Write(ctx, websocket.MessageText, bts)
}

// run connects the websocket, and runs the single connection until
// either the connection is terminated, or the global handler returns
// a non nil error.
func (c *WSClient) run(ctx context.Context) error {
	if err := c.connect(ctx); err != nil {
		return err
	}
	connectedAt := time.Now()
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

	staleCheck := time.Second * 60
	if c.staleMessageTimeout != 0 {
		staleCheck = time.Second
	}
	staleTicker := time.NewTicker(staleCheck)
	defer staleTicker.Stop()

	pings := make(chan struct{})
	if c.pingInterval != 0 {
		go func() {
			t := time.NewTicker(c.pingInterval)
			defer t.Stop()
			for {
				defer close(pings)
				<-t.C
				select {
				case pings <- struct{}{}:
				default:
					return
				}
			}
		}()
	}

	var lastMessageTimestamp time.Time

	for {
		select {
		case bts := <-data:
			c.logger.Debug("recv", "message", string(bts))
			lastMessageTimestamp = time.Now()
			if err := c.handler(bts); err != nil {
				return err
			}
		case <-staleTicker.C:
			c.logger.Debug("checking timeout", "connected_at", connectedAt)
			if c.staleMessageTimeout == 0 {
				c.logger.Debug("no timeout configured")
				continue
			}
			// Just connected, let the connection ride for a minute before asserting
			if lastMessageTimestamp.IsZero() && connectedAt.After(time.Now().Add(time.Minute*-1)) {
				c.logger.Debug("no message yet received")
				continue
			}

			check := time.Now().Add(-1 * c.staleMessageTimeout)
			if lastMessageTimestamp.Before(check) {
				c.logger.Debug("connection appears stale!", "last_message_time", lastMessageTimestamp.Format(time.RFC3339))
				if err := c.conn.Close(websocket.StatusGoingAway, "we think this connection has died"); err != nil {
					c.logger.Debug("failed to close apparent stale connection", "err", err.Error())
				}
				staleTicker.Stop()
			} else {
				c.logger.Debug("connection seems healthy")
			}
		case err := <-readErr:
			return err
		case <-pings:
			if err := c.pingHandler(ctx, c); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// connect creates a new connection and assigns it
// to the receiver
// TODO: the threadsafety thing, across the whole client
func (c *WSClient) connect(ctx context.Context) error {
	opts, err := c.dialOptionsFunc()
	if err != nil {
		return fmt.Errorf("dial options: %w", err)
	}

	endpoint, err := c.endpointFunc()
	if err != nil {
		return err
	}

	conn, _, err := websocket.Dial(ctx, endpoint, opts)
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

type PingHandler func(context.Context, *WSClient) error

func defaultPingHandler(ctx context.Context, ws *WSClient) error {
	return ws.conn.Ping(ctx)
}
