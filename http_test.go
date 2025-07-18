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

type testRequest struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type testResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	Echo    string `json:"echo,omitempty"`
}

type testLogger struct {
	logs []string
}

func (l *testLogger) Info(msg string, args ...any) {
	formattedMsg := fmt.Sprintf(msg, args...)
	l.logs = append(l.logs, fmt.Sprintf("INFO: %s", formattedMsg))
}

func (l *testLogger) Debug(msg string, args ...any) {
	formattedMsg := fmt.Sprintf(msg, args...)
	l.logs = append(l.logs, fmt.Sprintf("DEBUG: %s", formattedMsg))
}

func TestHTTPClient_BasicOperations(t *testing.T) {
	tests := []struct {
		name       string
		testFunc   func(t *testing.T, client *HTTPClient)
		serverFunc func(w http.ResponseWriter, r *http.Request)
		wantResp   testResponse
		wantErr    bool
	}{
		{
			name: "GET request",
			testFunc: func(t *testing.T, client *HTTPClient) {
				var resp testResponse
				err := client.Get("/api/test", nil, &resp)
				if err != nil {
					t.Errorf("Get() error = %v", err)
				}
				expected := testResponse{ID: "123", Message: "success"}
				if resp != expected {
					t.Errorf("Get() response = %+v, want %+v", resp, expected)
				}
			},
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("expected GET, got %s", r.Method)
				}
				json.NewEncoder(w).Encode(testResponse{
					ID:      "123",
					Message: "success",
				})
			},
		},
		{
			name: "POST request with body",
			testFunc: func(t *testing.T, client *HTTPClient) {
				var resp testResponse
				err := client.Post("/api/test", testRequest{Name: "test", Value: 42}, &resp)
				if err != nil {
					t.Errorf("Post() error = %v", err)
				}
				expected := testResponse{ID: "456", Message: "created", Echo: "test:42"}
				if resp != expected {
					t.Errorf("Post() response = %+v, want %+v", resp, expected)
				}
			},
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					t.Errorf("expected POST, got %s", r.Method)
				}
				var req testRequest
				json.NewDecoder(r.Body).Decode(&req)
				json.NewEncoder(w).Encode(testResponse{
					ID:      "456",
					Message: "created",
					Echo:    fmt.Sprintf("%s:%d", req.Name, req.Value),
				})
			},
		},
		{
			name: "PUT request",
			testFunc: func(t *testing.T, client *HTTPClient) {
				var resp testResponse
				err := client.Put("/api/test/123", testRequest{Name: "updated", Value: 100}, &resp)
				if err != nil {
					t.Errorf("Put() error = %v", err)
				}
				expected := testResponse{ID: "123", Message: "updated"}
				if resp != expected {
					t.Errorf("Put() response = %+v, want %+v", resp, expected)
				}
			},
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "PUT" {
					t.Errorf("expected PUT, got %s", r.Method)
				}
				json.NewEncoder(w).Encode(testResponse{
					ID:      "123",
					Message: "updated",
				})
			},
		},
		{
			name: "DELETE request",
			testFunc: func(t *testing.T, client *HTTPClient) {
				err := client.Delete("/api/test/123", nil, nil)
				if err != nil {
					t.Errorf("Delete() error = %v", err)
				}
			},
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "DELETE" {
					t.Errorf("expected DELETE, got %s", r.Method)
				}
				w.WriteHeader(http.StatusNoContent)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
			defer server.Close()

			client := NewHTTPClient(server.URL)
			tt.testFunc(t, client)
		})
	}
}

func TestHTTPClient_WithHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer token123" {
			t.Errorf("expected Authorization header 'Bearer token123', got '%s'", auth)
		}
		if custom := r.Header.Get("X-Custom-Header"); custom != "custom-value" {
			t.Errorf("expected X-Custom-Header 'custom-value', got '%s'", custom)
		}
		json.NewEncoder(w).Encode(testResponse{Message: "headers received"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)
	var resp testResponse
	err := client.Post("/test", nil, &resp,
		WithHeader("Authorization", "Bearer token123"),
		WithHeader("X-Custom-Header", "custom-value"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Message != "headers received" {
		t.Errorf("expected message 'headers received', got '%s'", resp.Message)
	}
}

func TestHTTPClient_WithBeforeHook(t *testing.T) {
	hookCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Hook-Header") != "hook-value" {
			t.Errorf("expected X-Hook-Header 'hook-value', got '%s'", r.Header.Get("X-Hook-Header"))
		}
		json.NewEncoder(w).Encode(testResponse{Message: "hook processed"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL,
		WithBefore(func(req *http.Request) error {
			hookCalled = true
			req.Header.Set("X-Hook-Header", "hook-value")
			return nil
		}),
	)

	var resp testResponse
	err := client.Get("/test", nil, &resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !hookCalled {
		t.Error("before hook was not called")
	}

	if resp.Message != "hook processed" {
		t.Errorf("expected message 'hook processed', got '%s'", resp.Message)
	}
}

func TestHTTPClient_WithLogger(t *testing.T) {
	logger := &testLogger{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(testResponse{Message: "logged"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL,
		WithLogger(logger),
	)

	var resp testResponse
	err := client.Get("/test", nil, &resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(logger.logs) == 0 {
		t.Error("expected logs to be written")
	}

	foundRequest := false
	foundResponse := false
	for _, log := range logger.logs {
		if strings.Contains(log, "request") {
			foundRequest = true
		}
		if strings.Contains(log, "response") {
			foundResponse = true
		}
	}

	if !foundRequest {
		t.Error("expected request log")
	}
	if !foundResponse {
		t.Error("expected response log")
	}
}

func TestHTTPClient_ErrorHandling(t *testing.T) {
	tests := []struct {
		name       string
		serverFunc func(w http.ResponseWriter, r *http.Request)
		wantErr    bool
		checkError func(err error) bool
	}{
		{
			name: "404 error",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("not found"))
			},
			wantErr: true,
			checkError: func(err error) bool {
				respErr, ok := err.(ResponseError)
				return ok && respErr.Code == 404
			},
		},
		{
			name: "500 error with body",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
			},
			wantErr: true,
			checkError: func(err error) bool {
				respErr, ok := err.(ResponseError)
				return ok && respErr.Code == 500 && strings.Contains(string(respErr.Body), "internal error")
			},
		},
		{
			name: "invalid JSON response",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("invalid json"))
			},
			wantErr: true,
			checkError: func(err error) bool {
				_, ok := err.(DecodeError)
				return ok
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
			defer server.Close()

			client := NewHTTPClient(server.URL, WithMaxStatus(299))
			var resp testResponse
			err := client.Get("/test", nil, &resp)

			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil && !tt.checkError(err) {
				t.Errorf("error check failed for error: %v", err)
			}
		})
	}
}

func TestHTTPClient_WithRateLimiter(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		json.NewEncoder(w).Encode(testResponse{Message: fmt.Sprintf("request %d", requestCount)})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL,
		WithRateLimit(rate.Every(100*time.Millisecond), 1),
	)

	start := time.Now()
	for i := 0; i < 3; i++ {
		var resp testResponse
		err := client.Get("/test", nil, &resp)
		if err != nil {
			t.Fatalf("unexpected error on request %d: %v", i+1, err)
		}
	}
	elapsed := time.Since(start)

	if elapsed < 200*time.Millisecond {
		t.Errorf("expected requests to take at least 200ms with rate limiting, took %v", elapsed)
	}

	if requestCount != 3 {
		t.Errorf("expected 3 requests, got %d", requestCount)
	}
}

func TestHTTPClient_WithTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(testResponse{Message: "delayed"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL,
		WithClient(&http.Client{Timeout: 100 * time.Millisecond}),
	)

	var resp testResponse
	err := client.Get("/test", nil, &resp)

	if err == nil {
		t.Error("expected timeout error, got nil")
	}

	if !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

func TestHTTPClient_CustomEncoderDecoder(t *testing.T) {
	customEncoder := func(v interface{}) ([]byte, error) {
		str := fmt.Sprintf("CUSTOM:%v", v)
		return []byte(str), nil
	}

	customDecoder := func(data []byte, v interface{}) error {
		resp := v.(*testResponse)
		resp.Message = string(data)
		return nil
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.HasPrefix(string(body), "CUSTOM:") {
			t.Errorf("expected custom encoded body, got: %s", body)
		}
		w.Write([]byte("custom response"))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL,
		WithEncoder(customEncoder),
		WithDecoder(customDecoder),
	)

	var resp testResponse
	err := client.Post("/test", testRequest{Name: "test"}, &resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Message != "custom response" {
		t.Errorf("expected 'custom response', got '%s'", resp.Message)
	}
}

func TestHTTPClient_SensitiveHeaders(t *testing.T) {
	logger := &testLogger{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(testResponse{Message: "sensitive"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL,
		WithLogger(logger),
		WithSensitiveHeader("Authorization", "X-API-Key"),
	)

	var resp testResponse
	err := client.Post("/test", nil, &resp,
		WithHeader("Authorization", "Bearer secret-token"),
		WithHeader("X-API-Key", "secret-key"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, log := range logger.logs {
		if strings.Contains(log, "Bearer secret-token") {
			t.Error("Authorization header value should be filtered from logs")
		}
		if strings.Contains(log, "secret-key") {
			t.Error("X-API-Key header value should be filtered from logs")
		}
	}
}

func TestHTTPClient_MaxStatus(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		maxStatus   int
		expectError bool
	}{
		{
			name:        "status within limit",
			statusCode:  200,
			maxStatus:   299,
			expectError: false,
		},
		{
			name:        "status exceeds limit",
			statusCode:  400,
			maxStatus:   299,
			expectError: true,
		},
		{
			name:        "no max status set",
			statusCode:  400,
			maxStatus:   0,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				json.NewEncoder(w).Encode(testResponse{Message: "test"})
			}))
			defer server.Close()

			opts := []HTTPOption{}
			if tt.maxStatus > 0 {
				opts = append(opts, WithMaxStatus(tt.maxStatus))
			}

			client := NewHTTPClient(server.URL, opts...)
			var resp testResponse
			err := client.Get("/test", nil, &resp)

			if (err != nil) != tt.expectError {
				t.Errorf("Get() error = %v, expectError %v", err, tt.expectError)
			}

			if err != nil && tt.expectError {
				respErr, ok := err.(ResponseError)
				if !ok {
					t.Errorf("expected ResponseError, got %T", err)
				} else if respErr.Code != tt.statusCode {
					t.Errorf("expected status code %d, got %d", tt.statusCode, respErr.Code)
				}
			}
		})
	}
}

func TestHTTPClient_QueryParameterHandling(t *testing.T) {
	var capturedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode(testResponse{Message: "success"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)

	// Test with url.Values to verify parameter handling
	params := url.Values{}
	// Add parameters in a specific order
	params.Add("zebra", "last")
	params.Add("alpha", "first") 
	params.Add("beta", "second")
	params.Add("gamma", "third")
	
	// Make the request
	var resp testResponse
	err := client.Get("/test", params, &resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse the captured query to verify all parameters are present
	parsedParams, err := url.ParseQuery(capturedQuery)
	if err != nil {
		t.Fatalf("failed to parse captured query: %v", err)
	}

	// Verify all parameters are present with correct values
	expectedParams := map[string]string{
		"zebra": "last",
		"alpha": "first",
		"beta":  "second", 
		"gamma": "third",
	}

	for key, expectedValue := range expectedParams {
		if actualValue := parsedParams.Get(key); actualValue != expectedValue {
			t.Errorf("expected parameter %s=%s, got %s", key, expectedValue, actualValue)
		}
	}

	// Verify that the library follows Go's standard behavior for query parameter ordering
	// url.Values.Encode() sorts parameters alphabetically (Go standard library behavior)
	expectedQuery := "alpha=first&beta=second&gamma=third&zebra=last"
	if capturedQuery != expectedQuery {
		t.Errorf("expected query string %s, got %s", expectedQuery, capturedQuery)
	}
}

func TestHTTPClient_QueryParameterOrderingWithMultipleValues(t *testing.T) {
	var capturedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode(testResponse{Message: "success"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)

	// Test with multiple values for the same parameter
	params := url.Values{}
	params.Add("category", "books")
	params.Add("sort", "name")
	params.Add("category", "movies") // Add second value for category
	params.Add("limit", "10")
	
	var resp testResponse
	err := client.Get("/test", params, &resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse the captured query
	parsedParams, err := url.ParseQuery(capturedQuery)
	if err != nil {
		t.Fatalf("failed to parse captured query: %v", err)
	}

	// Verify multiple values are preserved
	categories := parsedParams["category"]
	if len(categories) != 2 {
		t.Errorf("expected 2 category values, got %d", len(categories))
	}
	if categories[0] != "books" || categories[1] != "movies" {
		t.Errorf("expected category values [books, movies], got %v", categories)
	}

	// Verify the order of parameters in the query string
	// Go's url.Values.Encode() sorts keys alphabetically and groups multiple values
	expectedQuery := "category=books&category=movies&limit=10&sort=name"
	if capturedQuery != expectedQuery {
		t.Errorf("expected query string %s, got %s", expectedQuery, capturedQuery)
	}
}

func TestHTTPClient_EmptyQueryParameters(t *testing.T) {
	var capturedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode(testResponse{Message: "success"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)

	// Test with nil params
	var resp testResponse
	err := client.Get("/test", nil, &resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedQuery != "" {
		t.Errorf("expected empty query string, got %s", capturedQuery)
	}

	// Test with empty url.Values
	emptyParams := url.Values{}
	err = client.Get("/test", emptyParams, &resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedQuery != "" {
		t.Errorf("expected empty query string, got %s", capturedQuery)
	}
}

func TestHTTPClient_QueryParameterFix(t *testing.T) {
	var capturedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		json.NewEncoder(w).Encode(testResponse{Message: "success"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)

	testCases := []struct {
		name        string
		path        string
		params      url.Values
		expectedURL string
	}{
		{
			name:        "path without query, with params",
			path:        "/test",
			params:      url.Values{"param": []string{"value"}},
			expectedURL: "/test?param=value",
		},
		{
			name:        "path with query, with params",
			path:        "/test?existing=value",
			params:      url.Values{"new": []string{"param"}},
			expectedURL: "/test?existing=value&new=param",
		},
		{
			name:        "path with query, with multiple params",
			path:        "/test?existing=value",
			params:      url.Values{"new": []string{"param"}, "another": []string{"value"}},
			expectedURL: "/test?existing=value&another=value&new=param", // params are sorted alphabetically
		},
		{
			name:        "path without query, no params",
			path:        "/test",
			params:      nil,
			expectedURL: "/test",
		},
		{
			name:        "path without query, empty params",
			path:        "/test",
			params:      url.Values{},
			expectedURL: "/test",
		},
		{
			name:        "path with query, no params",
			path:        "/test?existing=value",
			params:      nil,
			expectedURL: "/test?existing=value",
		},
		{
			name:        "path with query, empty params",
			path:        "/test?existing=value",
			params:      url.Values{},
			expectedURL: "/test?existing=value",
		},
		{
			name:        "path with multiple query params, add more",
			path:        "/test?a=1&b=2",
			params:      url.Values{"c": []string{"3"}, "d": []string{"4"}},
			expectedURL: "/test?a=1&b=2&c=3&d=4",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			var resp testResponse
			err := client.Get(tt.path, tt.params, &resp)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if capturedURL != tt.expectedURL {
				t.Errorf("Expected URL: %s, got: %s", tt.expectedURL, capturedURL)
			}

			// Ensure no malformed URLs
			if strings.Contains(capturedURL, "??") {
				t.Errorf("Malformed URL with double '?': %s", capturedURL)
			}
		})
	}
}