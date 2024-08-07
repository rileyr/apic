package apic

import (
	"bytes"
	"context"
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

	// logBodies is a flag to log full req/rsp bodies
	logBodies bool

	// sensitiveHeaders keeps a list of headers to not log
	sensitiveHeaders []string // using a slice instead of a map, reasoning that there are only a few of these
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
	var bodyLog []byte
	if c.logBodies && body != nil {
		var err error
		bodyLog, err = io.ReadAll(body)
		if err != nil {
			return err
		}
		body = io.NopCloser(bytes.NewBuffer(bodyLog))
	}

	req, err := http.NewRequest(method, c.root+path, body)
	if err != nil {
		return err
	}

	if c.limiter != nil {
		if err := c.limiter.Wait(context.Background()); err != nil {
			return err
		}
	}

	if err := c.before(req); err != nil {
		return err
	}

	scrubbedHeaders := http.Header{}
HeaderLoop:
	for k, vals := range req.Header {
		for _, sh := range c.sensitiveHeaders {
			if k == sh {
				scrubbedHeaders.Set(k, "XXX-REDACTED-XXX")
				continue HeaderLoop
			}
		}
		scrubbedHeaders[k] = vals
	}

	c.logger.Info("request", "method", method, "path", req.URL.Path, "body", string(bodyLog), "query", req.URL.Query().Encode(), "headers", scrubbedHeaders)
	bodyLog = []byte{}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bts, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if c.logBodies {
		bodyLog = bts
	}
	c.logger.Info("response", "method", method, "path", req.URL.Path, "code", resp.StatusCode, "body", string(bodyLog))

	if c.maxStatus != 0 && resp.StatusCode > c.maxStatus {
		return badStatusError(resp)
	}

	if dest == nil {
		return nil
	}

	return c.decoder(bts, dest)
}
