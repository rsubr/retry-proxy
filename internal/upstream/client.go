package upstream

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var hopByHopHeaders = map[string]bool{
	"host":              true,
	"content-length":    true,
	"connection":        true,
	"transfer-encoding": true,
}

type Client struct {
	http *http.Client
}

func NewClient(timeout time.Duration) *Client {
	return &Client{
		http: &http.Client{Timeout: timeout},
	}
}

type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

func (c *Client) Do(method, targetURL string, headers map[string][]string, body []byte) (*Response, error) {
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = strings.NewReader(string(body))
	}

	req, err := http.NewRequest(method, targetURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	for k, vals := range headers {
		if hopByHopHeaders[strings.ToLower(k)] {
			continue
		}
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       respBody,
	}, nil
}
