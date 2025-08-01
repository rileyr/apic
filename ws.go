package apic

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
	conn   *websocket.Conn
	connMu sync.RWMutex

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
	maxAttempts     int
	currentAttempts int

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

		if errors.Is(err, MaxAttemptsError) {
			return err
		}

		if !c.shouldReconnect(err) {
			return err
		}
		c.logger.Info("reconnecting...")
	}
}

func (c *WSClient) Stop(reason string) error {
	if c.conn == nil {
		return nil
	}

	if reason == "" {
		reason = "going away"
	}

	return c.conn.Close(websocket.StatusGoingAway, reason)
}

var ErrNotConnected = errors.New("websocket not connected")

// IsConnected returns true if the WebSocket is currently connected.
func (c *WSClient) IsConnected() bool {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.conn != nil
}

// Close gracefully closes the WebSocket connection.
func (c *WSClient) Close() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn == nil {
		return nil
	}

	err := c.conn.Close(websocket.StatusNormalClosure, "client closing")
	c.conn = nil
	return err
}

// Write encodes and writes an object to the current connection.
func (c *WSClient) Write(ctx context.Context, obj any) error {
	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()

	if conn == nil {
		return ErrNotConnected
	}
	if ctx == nil {
		ctx = context.Background()
	}

	bts, err := c.encoder(obj)
	if err != nil {
		return err
	}

	return c.Send(ctx, bts)
}

func (c *WSClient) Send(ctx context.Context, bts []byte) error {
	if c.writeLimiter != nil {
		if err := c.writeLimiter.Wait(ctx); err != nil {
			return err
		}
	}

	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()

	if conn == nil {
		return ErrNotConnected
	}

	c.logger.Debug("send", "message", string(bts))
	return conn.Write(ctx, websocket.MessageText, bts)
}

// run connects the websocket, and runs the single connection until
// either the connection is terminated, or the global handler returns
// a non nil error.
func (c *WSClient) run(ctx context.Context) error {
	if err := c.connect(ctx); err != nil {
		return err
	}
	c.currentAttempts = 0

	connectedAt := time.Now()
	c.logger.Info("connected")

	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()

	defer func() {
		if conn != nil {
			conn.Close(websocket.StatusInternalError, "app closing")
		}
		c.connMu.Lock()
		c.conn = nil
		c.connMu.Unlock()
	}()

	readErr := make(chan error)
	data := make(chan []byte)
	go reader(ctx, conn, data, readErr)

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

	if c.pingInterval != 0 {
		go func() {
			t := time.NewTicker(c.pingInterval)
			defer t.Stop()

			for {
				select {
				case <-t.C:
					if err := c.pingHandler(ctx, c); err != nil {
						c.logger.Debug("ping handler error", "error", err.Error())
						return
					}
				case <-ctx.Done():
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
				if err := conn.Close(websocket.StatusGoingAway, "we think this connection has died"); err != nil {
					c.logger.Debug("failed to close apparent stale connection", "err", err.Error())
				}
				staleTicker.Stop()
			} else {
				c.logger.Debug("connection seems healthy")
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
	defer func() {
		c.currentAttempts++
	}()

	if c.maxAttempts > 0 && c.currentAttempts >= c.maxAttempts {
		return fmt.Errorf("%d: %w", c.currentAttempts, MaxAttemptsError)
	}

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

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()

	return nil
}

// reader is a helper func to pump messages from a connection
func reader(ctx context.Context, conn *websocket.Conn, data chan []byte, errs chan error) {
	defer close(data)
	defer close(errs)
	for {
		_, bts, err := conn.Read(ctx)
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
	ws.connMu.RLock()
	conn := ws.conn
	ws.connMu.RUnlock()

	if conn == nil {
		return ErrNotConnected
	}
	return conn.Ping(ctx)
}
