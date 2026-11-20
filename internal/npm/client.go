// Package npm is a small client for the Nginx Proxy Manager REST API.
package npm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Client struct {
	baseURL  string
	email    string
	password string
	// CertificateID, when set, attaches an existing (e.g. wildcard) certificate
	// to new hosts. Otherwise LetsEncryptEmail requests a new one per host.
	CertificateID    int64
	LetsEncryptEmail string

	http    *http.Client
	mu      sync.Mutex
	token   string
	expires time.Time
}

type ProxyHost struct {
	ID            int64    `json:"id"`
	DomainNames   []string `json:"domain_names"`
	ForwardScheme string   `json:"forward_scheme"`
	ForwardHost   string   `json:"forward_host"`
	ForwardPort   int      `json:"forward_port"`
}

func New(baseURL, email, password string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		email:    email,
		password: password,
		http:     &http.Client{Timeout: 2 * time.Minute},
	}
}

func (c *Client) authToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.expires.Add(-5*time.Minute)) {
		return c.token, nil
	}

	body, _ := json.Marshal(map[string]string{"identity": c.email, "secret": c.password})
	resp, err := c.http.Post(c.baseURL+"/api/tokens", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("npm login: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("npm login: %s", readError(resp))
	}

	var result struct {
		Token   string    `json:"token"`
		Expires time.Time `json:"expires"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("npm login: %w", err)
	}
	c.token, c.expires = result.Token, result.Expires
	return c.token, nil
}

func (c *Client) do(method, path string, in, out any) error {
	token, err := c.authToken()
	if err != nil {
		return err
	}

	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("npm %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("npm %s %s: %s", method, path, readError(resp))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func readError(resp *http.Response) string {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &parsed) == nil && parsed.Error.Message != "" {
		return fmt.Sprintf("%d %s", resp.StatusCode, parsed.Error.Message)
	}
	return fmt.Sprintf("%d %s", resp.StatusCode, strings.TrimSpace(string(data)))
}

func (c *Client) ListProxyHosts() ([]ProxyHost, error) {
	var hosts []ProxyHost
	err := c.do("GET", "/api/nginx/proxy-hosts", nil, &hosts)
	return hosts, err
}

// FindProxyHost returns the host serving domain, if any.
func (c *Client) FindProxyHost(domain string) (*ProxyHost, error) {
	hosts, err := c.ListProxyHosts()
	if err != nil {
		return nil, err
	}
	for _, h := range hosts {
		for _, d := range h.DomainNames {
			if strings.EqualFold(d, domain) {
				return &h, nil
			}
		}
	}
	return nil, nil
}

func (c *Client) CreateProxyHost(domain, forwardHost string, forwardPort int) (int64, error) {
	payload := map[string]any{
		"domain_names":            []string{domain},
		"forward_scheme":          "http",
		"forward_host":            forwardHost,
		"forward_port":            forwardPort,
		"access_list_id":          0,
		"certificate_id":          0,
		"ssl_forced":              false,
		"caching_enabled":         false,
		"block_exploits":          true,
		"allow_websocket_upgrade": true,
		"http2_support":           false,
		"hsts_enabled":            false,
		"hsts_subdomains":         false,
		"advanced_config":         "",
		"locations":               []any{},
		"meta":                    map[string]any{},
	}

	if c.CertificateID != 0 {
		payload["certificate_id"] = c.CertificateID
	} else if c.LetsEncryptEmail != "" {
		payload["certificate_id"] = "new"
		payload["meta"] = map[string]any{
			"letsencrypt_email": c.LetsEncryptEmail,
			"letsencrypt_agree": true,
			"dns_challenge":     false,
		}
	}
	if payload["certificate_id"] != 0 {
		payload["ssl_forced"] = true
		payload["http2_support"] = true
	}

	var created ProxyHost
	if err := c.do("POST", "/api/nginx/proxy-hosts", payload, &created); err != nil {
		return 0, err
	}
	return created.ID, nil
}

// UpdateForward points an existing host at a new upstream.
func (c *Client) UpdateForward(id int64, forwardHost string, forwardPort int) error {
	return c.do("PUT", fmt.Sprintf("/api/nginx/proxy-hosts/%d", id), map[string]any{
		"forward_scheme": "http",
		"forward_host":   forwardHost,
		"forward_port":   forwardPort,
	}, nil)
}

func (c *Client) DeleteProxyHost(id int64) error {
	err := c.do("DELETE", fmt.Sprintf("/api/nginx/proxy-hosts/%d", id), nil, nil)
	if err != nil && strings.Contains(err.Error(), "404") {
		return nil
	}
	return err
}
