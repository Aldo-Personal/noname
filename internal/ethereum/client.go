package ethereum

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

var errUpstream = errors.New("upstream unavailable")
var errTimeout = errors.New("upstream timeout")

type Client struct {
	endpoint string
	http     *http.Client
}

// NewClient accepts only an operator-owned URL; callers never choose a destination.
// Startup verifies Ethereum mainnet. Errors deliberately omit credential-bearing URLs.
func NewClient(ctx context.Context, endpoint string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return nil, errors.New("invalid ETHEREUM_RPC_URL")
	}
	loopback := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, errors.New("ETHEREUM_RPC_URL requires HTTPS (HTTP allowed on loopback only)")
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   3 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		IdleConnTimeout:       60 * time.Second,
		MaxIdleConns:          32, MaxIdleConnsPerHost: 32, MaxConnsPerHost: 32,
		MaxResponseHeaderBytes: 32 << 10,
		DisableCompression:     true,
	}
	c := &Client{endpoint: endpoint, http: &http.Client{
		Transport: transport, Timeout: 8 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
	if err := c.Check(ctx); err != nil {
		c.Close()
		return nil, errors.New("Ethereum mainnet upstream verification failed")
	}
	return c, nil
}

func (c *Client) Close() { c.http.CloseIdleConnections() }

// Check verifies both liveness and chain ID during startup.
func (c *Client) Check(ctx context.Context) error {
	_, err := c.call(ctx, request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "eth_chainId", Params: []json.RawMessage{}})
	return err
}

func (c *Client) call(ctx context.Context, req request) (json.RawMessage, error) {
	// A fixed upstream ID prevents customer-controlled IDs reaching provider diagnostics.
	req.ID = json.RawMessage(`1`)
	body, err := json.Marshal(req)
	if err != nil {
		return nil, errUpstream
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errUpstream
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json")
	response, err := c.http.Do(r)
	if err != nil {
		var timeout net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
			return nil, errTimeout
		}
		return nil, errUpstream
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errUpstream
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, MaxResponseBytes+1))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errTimeout
		}
		return nil, errUpstream
	}
	if len(raw) > MaxResponseBytes {
		return nil, errUpstream
	}
	fields, err := object(raw)
	if err != nil || stringValue(fields["jsonrpc"]) != "2.0" || !bytes.Equal(fields["id"], []byte(`1`)) {
		return nil, errUpstream
	}
	result, exists := fields["result"]
	_, hasError := fields["error"]
	// Provider errors may echo URLs/credentials; do not relay their message or data.
	if hasError || !exists || !validResult(req.Method, result) {
		return nil, errUpstream
	}
	return result, nil
}
