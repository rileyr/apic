package apic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

type testLogger struct {
	infoMsgs  []string
	debugMsgs []string
}

func (tl *testLogger) Info(msg string, args ...any) {
	tl.infoMsgs = append(tl.infoMsgs, fmt.Sprintf(msg, args...))
}

func (tl *testLogger) Debug(msg string, args ...any) {
	tl.debugMsgs = append(tl.debugMsgs, fmt.Sprintf(msg, args...))
}

type testRequest struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type testResponse struct {
	ID      int    `json:"id"`
	Message string `json:"message"`
}

func TestNewHTTPClient(t *testing.T) {
	client := NewHTTPClient("http://example.com")

	if client.root != "http://example.com" {
		t.Errorf("expected root to be 'http://example.com', got %s", client.root)
	}

	if client.client.Timeout != time.Second*5 {
		t.Errorf("expected default timeout to be 5s, got %v", client.client.Timeout)
	}

	if client.maxStatus != 0 {
		t.Errorf("expected default maxStatus to be 0, got %d", client.maxStatus)
	}

	if client.limiter != nil {
		t.Error("expected default limiter to be nil")
	}
}

func TestHTTPClientGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET method, got %s", r.Method)
		}

		if r.URL.Path != "/test" {
			t.Errorf("expected path /test, got %s", r.URL.Path)
		}

		if r.URL.Query().Get("foo") != "bar" {
			t.Errorf("expected query param foo=bar, got %s", r.URL.Query().Get("foo"))
		}

		response := testResponse{ID: 123, Message: "success"}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)

	params := url.Values{}
	params.Set("foo", "bar")

	var result testResponse
	err := client.Get("/test", params, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != 123 {
		t.Errorf("expected ID 123, got %d", result.ID)
	}

	if result.Message != "success" {
		t.Errorf("expected message 'success', got %s", result.Message)
	}
}

func TestHTTPClientPost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		var req testRequest
		json.Unmarshal(body, &req)

		if req.Name != "John" || req.Age != 30 {
			t.Errorf("unexpected request body: %+v", req)
		}

		response := testResponse{ID: 456, Message: "created"}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)

	request := testRequest{Name: "John", Age: 30}
	var result testResponse

	err := client.Post("/test", request, &result, WithHeader("Content-Type", "application/json"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != 456 {
		t.Errorf("expected ID 456, got %d", result.ID)
	}
}

func TestHTTPClientPut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("expected PUT method, got %s", r.Method)
		}

		response := testResponse{ID: 789, Message: "updated"}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)

	request := testRequest{Name: "Jane", Age: 25}
	var result testResponse

	err := client.Put("/test", request, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != 789 {
		t.Errorf("expected ID 789, got %d", result.ID)
	}
}

func TestHTTPClientPatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("expected PATCH method, got %s", r.Method)
		}

		response := testResponse{ID: 101, Message: "patched"}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)

	request := testRequest{Name: "Bob", Age: 35}
	var result testResponse

	err := client.Patch("/test", request, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != 101 {
		t.Errorf("expected ID 101, got %d", result.ID)
	}
}

func TestHTTPClientDelete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE method, got %s", r.Method)
		}

		response := testResponse{ID: 202, Message: "deleted"}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)

	request := testRequest{Name: "Alice", Age: 40}
	var result testResponse

	err := client.Delete("/test", request, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != 202 {
		t.Errorf("expected ID 202, got %d", result.ID)
	}
}

func TestHTTPClientWithLogger(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(testResponse{ID: 1, Message: "test"})
	}))
	defer server.Close()

	logger := &testLogger{}
	client := NewHTTPClient(server.URL, WithLogger(logger))

	var result testResponse
	err := client.Get("/test", nil, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(logger.infoMsgs) < 2 {
		t.Error("expected at least 2 log messages (request and response)")
	}
}

func TestHTTPClientWithBefore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token123" {
			t.Errorf("expected Authorization header, got %s", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(testResponse{ID: 1, Message: "authorized"})
	}))
	defer server.Close()

	beforeFunc := func(req *http.Request) error {
		req.Header.Set("Authorization", "Bearer token123")
		return nil
	}

	client := NewHTTPClient(server.URL, WithBefore(beforeFunc))

	var result testResponse
	err := client.Get("/test", nil, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPClientWithMaxStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, WithMaxStatus(300))

	var result testResponse
	err := client.Get("/test", nil, &result)

	if err == nil {
		t.Error("expected error due to status code exceeding maxStatus")
	}

	respErr, ok := err.(ResponseError)
	if !ok {
		t.Errorf("expected ResponseError, got %T", err)
	}

	if respErr.Code != 400 {
		t.Errorf("expected status code 400, got %d", respErr.Code)
	}

	if !strings.Contains(string(respErr.Body), "bad request") {
		t.Errorf("expected error body to contain 'bad request', got %s", string(respErr.Body))
	}
}

func TestHTTPClientWithCustomEncoder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != "custom-encoded-data" {
			t.Errorf("expected custom encoded data, got %s", string(body))
		}
		json.NewEncoder(w).Encode(testResponse{ID: 1, Message: "custom"})
	}))
	defer server.Close()

	customEncoder := func(obj any) ([]byte, error) {
		return []byte("custom-encoded-data"), nil
	}

	client := NewHTTPClient(server.URL, WithEncoder(customEncoder))

	var result testResponse
	err := client.Post("/test", testRequest{Name: "test"}, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPClientWithCustomDecoder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("custom-response-data"))
	}))
	defer server.Close()

	customDecoder := func(data []byte, dest any) error {
		if result, ok := dest.(*testResponse); ok {
			result.Message = "custom-decoded"
			result.ID = 999
		}
		return nil
	}

	client := NewHTTPClient(server.URL, WithDecoder(customDecoder))

	var result testResponse
	err := client.Get("/test", nil, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Message != "custom-decoded" || result.ID != 999 {
		t.Errorf("expected custom decoded result, got %+v", result)
	}
}

func TestHTTPClientWithRateLimit(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		json.NewEncoder(w).Encode(testResponse{ID: callCount, Message: "rate-limited"})
	}))
	defer server.Close()

	// Very restrictive rate limit: 1 request per second with burst of 1
	client := NewHTTPClient(server.URL, WithRateLimit(rate.Limit(1), 1))

	start := time.Now()

	var result1, result2 testResponse
	err1 := client.Get("/test", nil, &result1)
	err2 := client.Get("/test", nil, &result2)

	elapsed := time.Since(start)

	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}

	// Second request should be delayed by rate limiter
	if elapsed < time.Second {
		t.Errorf("expected rate limiting to delay second request, elapsed: %v", elapsed)
	}
}

func TestHTTPClientWithSensitiveHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(testResponse{ID: 1, Message: "secure"})
	}))
	defer server.Close()

	logger := &testLogger{}
	client := NewHTTPClient(server.URL,
		WithLogger(logger),
		WithSensitiveHeader("Authorization", "X-API-Key"),
	)

	beforeFunc := func(req *http.Request) error {
		req.Header.Set("Authorization", "secret-token")
		req.Header.Set("X-API-Key", "secret-key")
		req.Header.Set("X-Safe-Header", "safe-value")
		return nil
	}
	client.before = beforeFunc

	var result testResponse
	err := client.Get("/test", nil, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that sensitive headers are redacted in logs
	found := false
	for _, msg := range logger.infoMsgs {
		if strings.Contains(msg, "XXX-REDACTED-XXX") &&
			!strings.Contains(msg, "secret-token") &&
			!strings.Contains(msg, "secret-key") &&
			strings.Contains(msg, "safe-value") {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected sensitive headers to be redacted in logs")
	}
}

func TestHTTPClientDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)

	var result testResponse
	err := client.Get("/test", nil, &result)

	if err == nil {
		t.Error("expected decode error")
	}

	decodeErr, ok := err.(DecodeError)
	if !ok {
		t.Errorf("expected DecodeError, got %T", err)
	}

	if !strings.Contains(string(decodeErr.Body), "invalid json") {
		t.Errorf("expected error body to contain original response, got %s", string(decodeErr.Body))
	}
}

func TestHTTPClientDoHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "custom-value")
		json.NewEncoder(w).Encode(testResponse{ID: 1, Message: "with-headers"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)

	var result testResponse
	headers, err := client.DoHeader("GET", "/test", nil, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if headers.Get("X-Custom-Header") != "custom-value" {
		t.Errorf("expected custom header value, got %s", headers.Get("X-Custom-Header"))
	}
}

func TestHTTPClientWithLoggedBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// Echo back the request body in response
		w.Write(body)
	}))
	defer server.Close()

	logger := &testLogger{}
	client := NewHTTPClient(server.URL, WithLogger(logger), WithLoggedBodies())

	request := testRequest{Name: "John", Age: 30}
	var result interface{}

	err := client.Post("/test", request, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that request and response bodies are logged
	foundRequestBody := false
	foundResponseBody := false

	for _, msg := range logger.infoMsgs {
		if strings.Contains(msg, `"name":"John"`) {
			if strings.Contains(msg, "request") {
				foundRequestBody = true
			} else if strings.Contains(msg, "response") {
				foundResponseBody = true
			}
		}
	}

	if !foundRequestBody {
		t.Error("expected request body to be logged")
	}

	if !foundResponseBody {
		t.Error("expected response body to be logged")
	}
}

func TestGetResponseErrorCode(t *testing.T) {
	// Test with ResponseError
	respErr := ResponseError{Code: 404, Body: []byte("not found")}
	code := GetResponseErrorCode(respErr)
	if code != 404 {
		t.Errorf("expected code 404, got %d", code)
	}

	// Test with non-ResponseError
	otherErr := fmt.Errorf("some other error")
	code = GetResponseErrorCode(otherErr)
	if code != 0 {
		t.Errorf("expected code 0 for non-ResponseError, got %d", code)
	}
}
