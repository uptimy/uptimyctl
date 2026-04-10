package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type APIError struct {
	StatusCode int
	Code       string `json:"code"`
	Error      string `json:"error"`
}

func (e *APIError) String() string {
	if e.Code != "" {
		return fmt.Sprintf("%s: %s (HTTP %d)", e.Code, e.Error, e.StatusCode)
	}
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Error)
}

func (c *Client) doRequest(method, path string, query url.Values, body interface{}) ([]byte, error) {
	u, err := url.Parse(c.BaseURL + path)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, u.String(), reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "uptimyctl/1.0")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr APIError
		apiErr.StatusCode = resp.StatusCode
		if json.Unmarshal(respBody, &apiErr) != nil {
			apiErr.Error = string(respBody)
		}
		return nil, fmt.Errorf("%s", apiErr.String())
	}

	return respBody, nil
}

func (c *Client) Get(path string, query url.Values) ([]byte, error) {
	return c.doRequest(http.MethodGet, path, query, nil)
}

func (c *Client) Post(path string, body interface{}) ([]byte, error) {
	return c.doRequest(http.MethodPost, path, nil, body)
}

func (c *Client) Put(path string, body interface{}) ([]byte, error) {
	return c.doRequest(http.MethodPut, path, nil, body)
}

func (c *Client) Patch(path string, body interface{}) ([]byte, error) {
	return c.doRequest(http.MethodPatch, path, nil, body)
}

func (c *Client) Delete(path string) ([]byte, error) {
	return c.doRequest(http.MethodDelete, path, nil, nil)
}

// ParseDataField extracts the "data" field from a JSON response.
func ParseDataField(raw []byte) (json.RawMessage, error) {
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return wrapper.Data, nil
}

// ParseResultsField extracts data.results from a paginated response.
func ParseResultsField(raw []byte) (json.RawMessage, error) {
	data, err := ParseDataField(raw)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Results json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("parse results: %w", err)
	}
	return wrapper.Results, nil
}
