package apic

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"
)

type HTTPClient struct {
	// root is the remote api's root url
	root string

	// client is the *http.Client!
	client *http.Client

	// encoder is used to encode request bodies
	encoder Encoder

	// decoder is used to decode response bodies
	decoder Decoder

	// logger logs each request and response
	logger Logger

	// before is a function called before each
	// request is made. useful for like, auth sigs, etc.
	before func(*http.Request) error

	// maxStatus sets the max expected value for response codes.
	// if set, client will throw an error if the response code received
	// is greater than the max.
	maxStatus int

	// limiter is a rate limiter
	limiter *rate.Limiter
}

func NewHTTPClient(root string, opts ...HTTPOption) *HTTPClient {
	c := &HTTPClient{
		root:    root,
		encoder: defaultEncoder,
		decoder: defaultDecoder,
		logger:  noLogger{},
		client: &http.Client{
			Timeout: time.Second * 5,
		},
		before:    func(_ *http.Request) error { return nil },
		maxStatus: 0,
		limiter:   nil,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *HTTPClient) Get(path string, params url.Values, dest any) error {
	if params != nil {
		params.Encode()
		path = path + "?" + params.Encode()
	}
	return c.Do("GET", path, nil, dest)
}

func (c *HTTPClient) Post(path string, data any, dest any) error {
	return c.doBody("POST", path, data, dest)
}

func (c *HTTPClient) Delete(path string, data any, dest any) error {
	return c.doBody("DELETE", path, data, dest)
}

func (c *HTTPClient) Put(path string, data any, dest any) error {
	return c.doBody("PUT", path, data, dest)
}

func (c *HTTPClient) Patch(path string, data any, dest any) error {
	return c.doBody("PATCH", path, data, dest)
}

func (c *HTTPClient) doBody(method, path string, data any, dest any) error {
	var body io.Reader
	if data != nil {
		bts, err := c.encoder(data)
		if err != nil {
			return err
		}
		body = bytes.NewReader(bts)
	}
	return c.Do(method, path, body, dest)
}

func (c *HTTPClient) Do(method, path string, body io.Reader, dest any) error {
	req, err := http.NewRequest(method, c.root+path, body)
	if err != nil {
		return err
	}

	if c.limiter != nil {
		if err := c.limiter.Wait(context.Background()); err != nil {
			return err
		}
	}

	c.logger.Info("request", "method", method, "path", req.URL.Path)
	if err := c.before(req); err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	c.logger.Info("response", "method", method, "path", req.URL.Path, "code", resp.StatusCode)

	if c.maxStatus != 0 && resp.StatusCode > c.maxStatus {
		return fmt.Errorf("api returned bad code: %d", resp.StatusCode)
	}

	if dest == nil {
		return nil
	}

	bts, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return c.decoder(bts, dest)
}
