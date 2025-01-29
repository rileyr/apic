package apic

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/davecgh/go-spew/spew"
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

type HeaderFunc func(http.Header)

func WithHeader(key, value string) func(http.Header) {
	return func(hdr http.Header) {
		hdr.Set(key, value)
	}
}

func (c *HTTPClient) Post(path string, data any, dest any, hdrs ...HeaderFunc) error {
	return c.doBody("POST", path, data, dest, hdrs...)
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

func (c *HTTPClient) doBody(method, path string, data any, dest any, hdrs ...HeaderFunc) error {
	var body io.Reader
	if data != nil {
		bts, err := c.encoder(data)
		if err != nil {
			return err
		}
		body = bytes.NewReader(bts)
	}
	return c.Do(method, path, body, dest, hdrs...)
}

func (c *HTTPClient) Do(method, path string, body io.Reader, dest any, hdrs ...HeaderFunc) error {
	_, err := c.DoHeader(method, path, body, dest, hdrs...)
	return err
}

func (c *HTTPClient) DoHeader(method, path string, body io.Reader, dest any, hdrs ...HeaderFunc) (http.Header, error) {
	req, err := http.NewRequest(method, c.root+path, body)
	if err != nil {
		return nil, err
	}
	for _, hdr := range hdrs {
		hdr(req.Header)
	}

	if c.limiter != nil {
		if err := c.limiter.Wait(context.Background()); err != nil {
			return nil, err
		}
	}

	if err := c.before(req); err != nil {
		return nil, err
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

	var bodyLog []byte
	if c.logBodies && req.Body != nil {
		var err error
		bodyLog, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body.Close()
		req.Body = io.NopCloser(bytes.NewBuffer(bodyLog))
	}

	c.logger.Info("request", "method", method, "path", req.URL.Path, "body", string(bodyLog), "query", req.URL.Query().Encode(), "headers", scrubbedHeaders)
	bodyLog = []byte{}

	nr, _ := http.NewRequest(req.Method, c.root+path, req.Body)
	nr.Header = req.Header
	spew.Dump(nr)
	resp, err := c.client.Do(nr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bts, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if c.logBodies {
		bodyLog = bts
	}
	c.logger.Info("response", "method", method, "path", req.URL.Path, "code", resp.StatusCode, "body", string(bodyLog))

	if c.maxStatus != 0 && resp.StatusCode > c.maxStatus {
		return nil, badStatusError(resp)
	}

	if dest == nil {
		return resp.Header, nil
	}

	return resp.Header, c.decoder(bts, dest)
}
