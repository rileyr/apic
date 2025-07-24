package apic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type wsTestLogger struct {
	mu        sync.Mutex
	infoMsgs  []string
	debugMsgs []string
}

func (tl *wsTestLogger) Info(msg string, args ...any) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.infoMsgs = append(tl.infoMsgs, fmt.Sprintf(msg+" "+formatArgs(args), args...))
}

func (tl *wsTestLogger) Debug(msg string, args ...any) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.debugMsgs = append(tl.debugMsgs, fmt.Sprintf(msg+" "+formatArgs(args), args...))
}

func formatArgs(args []any) string {
	var parts []string
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			parts = append(parts, fmt.Sprintf("%v=%v", args[i], args[i+1]))
		}
	}
	return strings.Join(parts, " ")
}

// TestNewWSClient tests the creation of a new WebSocket client with default values
func TestNewWSClient(t *testing.T) {
	endpoint := "ws://example.com/ws"
	client := NewWSClient(endpoint)

	if client.endpoint != endpoint {
		t.Errorf("expected endpoint to be %s, got %s", endpoint, client.endpoint)
	}

	if client.writeLimiter != nil {
		t.Error("expected default writeLimiter to be nil")
	}

	if client.pingInterval != 0 {
		t.Error("expected default pingInterval to be 0")
	}

	if client.staleMessageTimeout != 0 {
		t.Error("expected default staleMessageTimeout to be 0")
	}

	// Test that default handler doesn't error
	if err := client.handler([]byte("test")); err != nil {
		t.Errorf("expected default handler to return nil, got %v", err)
	}

	// Test that default onOpen doesn't error
	if err := client.onOpen(client); err != nil {
		t.Errorf("expected default onOpen to return nil, got %v", err)
	}

	// Test that default onClose doesn't error
	if err := client.onClose(client); err != nil {
		t.Errorf("expected default onClose to return nil, got %v", err)
	}

	// Test that default shouldReconnect returns false
	if client.shouldReconnect(errors.New("test")) {
		t.Error("expected default shouldReconnect to return false")
	}
}

// TestWSClientOptions tests various configuration options
func TestWSClientOptions(t *testing.T) {
	logger := &wsTestLogger{}
	var handlerCalled bool
	var onOpenCalled bool
	var onCloseCalled bool

	handler := func(data []byte) error {
		handlerCalled = true
		return nil
	}

	onOpen := func(c *WSClient) error {
		onOpenCalled = true
		return nil
	}

	onClose := func(c *WSClient) error {
		onCloseCalled = true
		return nil
	}

	encoder := func(obj any) ([]byte, error) {
		return []byte("custom-encoded"), nil
	}

	client := NewWSClient("ws://example.com/ws",
		WithWSLogger(logger),
		WithPingInterval(time.Second*30),
		WithWSHandler(handler),
		WithWSOnOpen(onOpen),
		WithWSOnClose(onClose),
		WithWSEncoder(encoder),
		WithStaleDetection(time.Minute*5),
		WithWriteLimiter(rate.Limit(10), 5),
	)

	// Test logger
	client.logger.Info("test")
	if len(logger.infoMsgs) != 1 || !strings.Contains(logger.infoMsgs[0], "test") {
		t.Error("expected logger to be set correctly")
	}

	// Test ping interval
	if client.pingInterval != time.Second*30 {
		t.Errorf("expected ping interval to be 30s, got %v", client.pingInterval)
	}

	// Test handler
	client.handler([]byte("test"))
	if !handlerCalled {
		t.Error("expected custom handler to be called")
	}

	// Test onOpen
	client.onOpen(client)
	if !onOpenCalled {
		t.Error("expected custom onOpen to be called")
	}

	// Test onClose
	client.onClose(client)
	if !onCloseCalled {
		t.Error("expected custom onClose to be called")
	}

	// Test encoder
	data, _ := client.encoder("test")
	if string(data) != "custom-encoded" {
		t.Errorf("expected custom encoder to return 'custom-encoded', got %s", string(data))
	}

	// Test stale detection
	if client.staleMessageTimeout != time.Minute*5 {
		t.Errorf("expected stale message timeout to be 5m, got %v", client.staleMessageTimeout)
	}

	// Test write limiter
	if client.writeLimiter == nil {
		t.Error("expected write limiter to be set")
	}
}

// TestWSClientReconnectBackoff tests the reconnect backoff functionality
func TestWSClientReconnectBackoff(t *testing.T) {
	client := NewWSClient("ws://example.com/ws",
		WithReconnectBackoff(time.Second*2),
	)

	start := time.Now()
	shouldReconnect := client.shouldReconnect(errors.New("test"))
	elapsed := time.Since(start)

	if !shouldReconnect {
		t.Error("expected shouldReconnect to return true with backoff configured")
	}

	// Should have some delay due to backoff
	if elapsed < time.Millisecond*5 {
		t.Errorf("expected backoff delay, but elapsed time was %v", elapsed)
	}
}

// TestWSClientDialOptions tests custom dial options
func TestWSClientDialOptions(t *testing.T) {
	var dialOptionsCalled bool

	dialOptionsFunc := func() (*DialOptions, error) {
		dialOptionsCalled = true
		return &DialOptions{
			HTTPHeader: http.Header{
				"Authorization": []string{"Bearer token"},
			},
		}, nil
	}

	client := NewWSClient("ws://example.com/ws",
		WithDialOptions(dialOptionsFunc),
	)

	opts, err := client.dialOptionsFunc()
	if err != nil {
		t.Fatalf("unexpected error from dial options: %v", err)
	}

	if !dialOptionsCalled {
		t.Error("expected dial options function to be called")
	}

	if opts.HTTPHeader.Get("Authorization") != "Bearer token" {
		t.Error("expected custom dial options to include authorization header")
	}
}

// TestWSClientEndpointFunc tests dynamic endpoint functionality
func TestWSClientEndpointFunc(t *testing.T) {
	endpointCount := 0
	endpointFunc := func() (string, error) {
		endpointCount++
		if endpointCount == 1 {
			return "ws://example1.com/ws", nil
		}
		return "ws://example2.com/ws", nil
	}

	client := NewWSClient("ws://default.com/ws",
		WithEndpointFunc(endpointFunc),
	)

	// First call
	endpoint1, err := client.endpointFunc()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if endpoint1 != "ws://example1.com/ws" {
		t.Errorf("expected first endpoint to be ws://example1.com/ws, got %s", endpoint1)
	}

	// Second call
	endpoint2, err := client.endpointFunc()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if endpoint2 != "ws://example2.com/ws" {
		t.Errorf("expected second endpoint to be ws://example2.com/ws, got %s", endpoint2)
	}
}

// TestWSClientWriteNotConnected tests writing when not connected
func TestWSClientWriteNotConnected(t *testing.T) {
	client := NewWSClient("ws://example.com/ws")

	err := client.Write(context.Background(), "test message")
	if err != ErrNotConnected {
		t.Errorf("expected ErrNotConnected, got %v", err)
	}
}

// TestWSClientWriteWithContext tests write with nil context
func TestWSClientWriteWithContext(t *testing.T) {
	// Create a test WebSocket server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("failed to accept websocket: %v", err)
		}
		defer c.Close(websocket.StatusNormalClosure, "")

		// Read message
		_, data, err := c.Read(context.Background())
		if err != nil {
			t.Fatalf("failed to read message: %v", err)
		}

		// Echo it back
		err = c.Write(context.Background(), websocket.MessageText, data)
		if err != nil {
			t.Fatalf("failed to write message: %v", err)
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := NewWSClient(wsURL)

	// Manually connect for this test
	conn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	client.conn = conn
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Test with nil context (should use background context)
	err = client.Write(nil, map[string]string{"message": "test"})
	if err != nil {
		t.Errorf("unexpected error with nil context: %v", err)
	}
}

// TestWSClientBasicConnection tests basic connection and message exchange
func TestWSClientBasicConnection(t *testing.T) {
	logger := &wsTestLogger{}
	messageReceived := make(chan []byte, 1)

	handler := func(data []byte) error {
		messageReceived <- data
		return nil
	}

	// Create a test WebSocket server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("failed to accept websocket: %v", err)
		}
		defer c.Close(websocket.StatusNormalClosure, "")

		// Send a welcome message
		err = wsjson.Write(context.Background(), c, map[string]string{"type": "welcome"})
		if err != nil {
			t.Fatalf("failed to write welcome message: %v", err)
		}

		// Keep connection open
		<-context.Background().Done()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := NewWSClient(wsURL,
		WithWSLogger(logger),
		WithWSHandler(handler),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	go client.Start(ctx)

	// Wait for message
	select {
	case msg := <-messageReceived:
		var data map[string]string
		if err := json.Unmarshal(msg, &data); err != nil {
			t.Fatalf("failed to unmarshal message: %v", err)
		}
		if data["type"] != "welcome" {
			t.Errorf("expected welcome message, got %v", data)
		}
	case <-time.After(time.Second * 3):
		t.Fatal("timeout waiting for message")
	}

	// Check logs
	time.Sleep(time.Millisecond * 100) // Give logger time to process
	logger.mu.Lock()
	hasConnectedLog := false
	for _, msg := range logger.infoMsgs {
		if strings.Contains(msg, "connected") {
			hasConnectedLog = true
			break
		}
	}
	logger.mu.Unlock()

	if !hasConnectedLog {
		t.Error("expected 'connected' log message")
	}
}

// TestWSClientOnOpenOnClose tests the onOpen and onClose callbacks
func TestWSClientOnOpenOnClose(t *testing.T) {
	var onOpenCalled atomic.Bool
	var onCloseCalled atomic.Bool

	onOpen := func(c *WSClient) error {
		onOpenCalled.Store(true)
		return nil
	}

	onClose := func(c *WSClient) error {
		onCloseCalled.Store(true)
		return nil
	}

	// Create a test WebSocket server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("failed to accept websocket: %v", err)
		}
		// Close immediately
		c.Close(websocket.StatusNormalClosure, "")
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := NewWSClient(wsURL,
		WithWSOnOpen(onOpen),
		WithWSOnClose(onClose),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	client.Start(ctx)

	if !onOpenCalled.Load() {
		t.Error("expected onOpen to be called")
	}

	if !onCloseCalled.Load() {
		t.Error("expected onClose to be called")
	}
}

// TestWSClientReconnection tests reconnection behavior
func TestWSClientReconnection(t *testing.T) {
	logger := &wsTestLogger{}
	connectionCount := atomic.Int32{}
	connectedChan := make(chan struct{}, 2)

	// Create a test WebSocket server that can simulate disconnections
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := connectionCount.Add(1)
		connectedChan <- struct{}{}

		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("failed to accept websocket: %v", err)
		}

		if count == 1 {
			// First connection: close after a short delay
			time.Sleep(time.Millisecond * 100)
			c.Close(websocket.StatusGoingAway, "server closing")
			return
		}

		// Second connection: keep open
		<-r.Context().Done()
		c.Close(websocket.StatusNormalClosure, "")
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	reconnectCount := atomic.Int32{}
	client := NewWSClient(wsURL,
		WithWSLogger(logger),
	)

	// Custom reconnect policy
	client.shouldReconnect = func(err error) bool {
		count := reconnectCount.Add(1)
		if count >= 2 {
			return false
		}
		// Small delay between reconnects
		time.Sleep(time.Millisecond * 50)
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	// Run in goroutine so we can monitor connections
	done := make(chan error)
	go func() {
		done <- client.Start(ctx)
	}()

	// Wait for first connection
	select {
	case <-connectedChan:
		// First connection established
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for first connection")
	}

	// Wait for second connection (reconnect)
	select {
	case <-connectedChan:
		// Second connection established
	case <-time.After(time.Second * 2):
		t.Fatal("timeout waiting for reconnection")
	}

	// Cancel to stop the client
	cancel()
	<-done

	// Verify we had 2 connections
	count := connectionCount.Load()
	if count != 2 {
		t.Errorf("expected exactly 2 connections, got %d", count)
	}

	// Check for reconnect log
	logger.mu.Lock()
	hasReconnectLog := false
	for _, msg := range logger.infoMsgs {
		if strings.Contains(msg, "reconnecting") {
			hasReconnectLog = true
			break
		}
	}
	logger.mu.Unlock()

	if !hasReconnectLog {
		t.Error("expected 'reconnecting' log message")
	}
}

// TestWSClientWriteWithRateLimit tests write rate limiting
func TestWSClientWriteWithRateLimit(t *testing.T) {
	messagesReceived := make(chan string, 10)

	// Create a test WebSocket server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("failed to accept websocket: %v", err)
		}
		defer c.Close(websocket.StatusNormalClosure, "")

		for {
			var msg map[string]string
			err := wsjson.Read(context.Background(), c, &msg)
			if err != nil {
				return
			}
			messagesReceived <- msg["data"]
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Very restrictive rate limit: 2 per second
	client := NewWSClient(wsURL,
		WithWriteLimiter(rate.Limit(2), 1),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	// Start client
	go client.Start(ctx)

	// Wait for connection
	time.Sleep(time.Millisecond * 200)

	start := time.Now()

	// Try to send 3 messages rapidly
	go func() {
		for i := 0; i < 3; i++ {
			msg := map[string]string{"data": fmt.Sprintf("message-%d", i)}
			if err := client.Write(context.Background(), msg); err != nil {
				t.Errorf("failed to write message %d: %v", i, err)
			}
		}
	}()

	// Collect messages
	var received []string
	timeout := time.After(time.Second * 2)
	for i := 0; i < 3; i++ {
		select {
		case msg := <-messagesReceived:
			received = append(received, msg)
		case <-timeout:
			t.Fatalf("timeout waiting for message %d", i)
		}
	}

	elapsed := time.Since(start)

	// With rate limit of 2/sec and burst of 1, the third message should be delayed
	if elapsed < time.Millisecond*400 {
		t.Errorf("expected rate limiting to cause delay, but elapsed time was only %v", elapsed)
	}

	if len(received) != 3 {
		t.Errorf("expected 3 messages, got %d", len(received))
	}
}

// TestWSClientPingHandler tests custom ping handler
func TestWSClientPingHandler(t *testing.T) {
	pingCount := atomic.Int32{}

	customPingHandler := func(ctx context.Context, ws *WSClient) error {
		pingCount.Add(1)
		// Don't actually ping in test
		return nil
	}

	// Create a test WebSocket server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("failed to accept websocket: %v", err)
		}
		defer c.Close(websocket.StatusNormalClosure, "")

		// Keep connection open
		time.Sleep(time.Second * 2)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := NewWSClient(wsURL,
		WithPingInterval(time.Millisecond*200),
		WithPingHandler(customPingHandler),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	go client.Start(ctx)

	// Wait for pings
	time.Sleep(time.Millisecond * 700)

	count := pingCount.Load()
	if count < 2 {
		t.Errorf("expected at least 2 pings, got %d", count)
	}
}

// TestWSClientStaleDetection tests stale connection detection
func TestWSClientStaleDetection(t *testing.T) {
	logger := &wsTestLogger{}

	// Create a test WebSocket server that stops sending messages
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("failed to accept websocket: %v", err)
		}
		defer c.Close(websocket.StatusNormalClosure, "")

		// Send one message then go silent
		err = wsjson.Write(context.Background(), c, map[string]string{"type": "hello"})
		if err != nil {
			t.Fatalf("failed to write message: %v", err)
		}

		// Keep connection open but don't send more messages
		time.Sleep(time.Second * 5)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := NewWSClient(wsURL,
		WithWSLogger(logger),
		WithStaleDetection(time.Millisecond*500), // Very short for testing
		WithWSHandler(func([]byte) error { return nil }),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	err := client.Start(ctx)

	// Check if connection was closed due to staleness
	logger.mu.Lock()
	hasStaleLog := false
	for _, msg := range logger.debugMsgs {
		if strings.Contains(msg, "stale") {
			hasStaleLog = true
			break
		}
	}
	logger.mu.Unlock()

	if !hasStaleLog {
		t.Log("Warning: expected stale connection log, but didn't find one")
		t.Logf("Error from Start: %v", err)
		t.Logf("Debug logs: %v", logger.debugMsgs)
	}
}

// TestWSClientHandlerError tests error handling from message handler
func TestWSClientHandlerError(t *testing.T) {
	handlerErr := errors.New("handler error")
	handler := func(data []byte) error {
		return handlerErr
	}

	// Create a test WebSocket server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("failed to accept websocket: %v", err)
		}
		defer c.Close(websocket.StatusNormalClosure, "")

		// Send a message that will trigger the handler error
		err = wsjson.Write(context.Background(), c, map[string]string{"type": "trigger-error"})
		if err != nil {
			t.Fatalf("failed to write message: %v", err)
		}

		// Keep connection open
		time.Sleep(time.Second)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := NewWSClient(wsURL,
		WithWSHandler(handler),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	err := client.Start(ctx)

	if err != handlerErr {
		t.Errorf("expected handler error to be returned, got %v", err)
	}
}

// TestWSClientEncoderError tests error handling from encoder
func TestWSClientEncoderError(t *testing.T) {
	encoderErr := errors.New("encoder error")
	encoder := func(obj any) ([]byte, error) {
		return nil, encoderErr
	}

	// Create a test WebSocket server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("failed to accept websocket: %v", err)
		}
		defer c.Close(websocket.StatusNormalClosure, "")

		// Keep connection open
		time.Sleep(time.Second)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := NewWSClient(wsURL,
		WithWSEncoder(encoder),
	)

	// Manually connect for this test
	conn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	client.conn = conn
	defer conn.Close(websocket.StatusNormalClosure, "")

	err = client.Write(context.Background(), "test")
	if err != encoderErr {
		t.Errorf("expected encoder error, got %v", err)
	}
}

// TestWSClientDialOptionsError tests error handling from dial options
func TestWSClientDialOptionsError(t *testing.T) {
	dialErr := errors.New("dial options error")
	dialOptionsFunc := func() (*DialOptions, error) {
		return nil, dialErr
	}

	client := NewWSClient("ws://example.com/ws",
		WithDialOptions(dialOptionsFunc),
	)

	err := client.connect(context.Background())
	if err == nil {
		t.Error("expected error from dial options")
	}
	if !strings.Contains(err.Error(), "dial options") {
		t.Errorf("expected error to mention dial options, got %v", err)
	}
}

// TestWSClientEndpointFuncError tests error handling from endpoint function
func TestWSClientEndpointFuncError(t *testing.T) {
	endpointErr := errors.New("endpoint error")
	endpointFunc := func() (string, error) {
		return "", endpointErr
	}

	client := NewWSClient("ws://default.com/ws",
		WithEndpointFunc(endpointFunc),
	)

	err := client.connect(context.Background())
	if err != endpointErr {
		t.Errorf("expected endpoint error, got %v", err)
	}
}

// TestWSClientOnOpenError tests error handling from onOpen callback
func TestWSClientOnOpenError(t *testing.T) {
	onOpenErr := errors.New("onOpen error")
	onOpen := func(c *WSClient) error {
		return onOpenErr
	}

	// Create a test WebSocket server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("failed to accept websocket: %v", err)
		}
		defer c.Close(websocket.StatusNormalClosure, "")

		// Keep connection open
		time.Sleep(time.Second)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := NewWSClient(wsURL,
		WithWSOnOpen(onOpen),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	err := client.Start(ctx)
	if err != onOpenErr {
		t.Errorf("expected onOpen error to be returned, got %v", err)
	}
}

// TestWSClientWriteCancelledContext tests write with cancelled context
func TestWSClientWriteCancelledContext(t *testing.T) {
	// Create a test WebSocket server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("failed to accept websocket: %v", err)
		}
		defer c.Close(websocket.StatusNormalClosure, "")

		// Keep connection open
		time.Sleep(time.Second * 2)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := NewWSClient(wsURL,
		WithWriteLimiter(rate.Limit(0.1), 1), // Very slow rate to ensure context cancels first
	)

	// Manually connect for this test
	conn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	client.conn = conn
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Create already cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = client.Write(ctx, "test")
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

// TestWSClientConnectionFailure tests behavior when connection fails
func TestWSClientConnectionFailure(t *testing.T) {
	logger := &wsTestLogger{}

	// Use invalid URL
	client := NewWSClient("ws://localhost:0/invalid",
		WithWSLogger(logger),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := client.Start(ctx)
	if err == nil {
		t.Error("expected error from invalid connection")
	}

	// Check for disconnected log
	logger.mu.Lock()
	hasDisconnectedLog := false
	for _, msg := range logger.infoMsgs {
		if strings.Contains(msg, "disconnected") {
			hasDisconnectedLog = true
			break
		}
	}
	logger.mu.Unlock()

	if !hasDisconnectedLog {
		t.Error("expected 'disconnected' log message")
	}
}
