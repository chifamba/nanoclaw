package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProxy_APIKeyMode(t *testing.T) {
	// Setup upstream
	lastUpstreamHeaders := make(http.Header)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range r.Header {
			lastUpstreamHeaders[k] = v
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)

	// Setup proxy
	p := &Proxy{
		AuthMode:    AuthModeAPIKey,
		APIKey:      "sk-ant-real-key",
		UpstreamURL: upstreamURL,
	}

	proxyServer := httptest.NewServer(p)
	defer proxyServer.Close()

	// Make request
	req, _ := http.NewRequest("POST", proxyServer.URL+"/v1/messages", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", "placeholder")

	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	assert.Equal(t, "sk-ant-real-key", lastUpstreamHeaders.Get("x-api-key"))
}

func TestProxy_OAuthMode(t *testing.T) {
	// Setup upstream
	lastUpstreamHeaders := make(http.Header)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastUpstreamHeaders = make(http.Header)
		for k, v := range r.Header {
			lastUpstreamHeaders[k] = v
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)

	// Setup proxy
	p := &Proxy{
		AuthMode:    AuthModeOAuth,
		OAuthToken:  "real-oauth-token",
		UpstreamURL: upstreamURL,
	}

	proxyServer := httptest.NewServer(p)
	defer proxyServer.Close()

	t.Run("injects authorization header when present", func(t *testing.T) {
		req, _ := http.NewRequest("POST", proxyServer.URL+"/api/oauth/claude_cli/create_api_key", strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer placeholder")

		resp, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "Bearer real-oauth-token", lastUpstreamHeaders.Get("Authorization"))
	})

	t.Run("omits authorization header when not present", func(t *testing.T) {
		req, _ := http.NewRequest("POST", proxyServer.URL+"/v1/messages", strings.NewReader("{}"))
		req.Header.Set("x-api-key", "temp-key")

		resp, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "temp-key", lastUpstreamHeaders.Get("x-api-key"))
		assert.Empty(t, lastUpstreamHeaders.Get("Authorization"))
	})
}

func TestProxy_StripHopByHop(t *testing.T) {
	// Setup upstream
	lastUpstreamHeaders := make(http.Header)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range r.Header {
			lastUpstreamHeaders[k] = v
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)

	p := &Proxy{
		AuthMode:    AuthModeAPIKey,
		APIKey:      "key",
		UpstreamURL: upstreamURL,
	}

	proxyServer := httptest.NewServer(p)
	defer proxyServer.Close()

	req, _ := http.NewRequest("GET", proxyServer.URL, nil)
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Keep-Alive", "timeout=5")

	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	assert.Empty(t, lastUpstreamHeaders.Get("Keep-Alive"))
}

func TestProxy_UpstreamError(t *testing.T) {
	// Use an unreachable address
	upstreamURL, _ := url.Parse("http://127.0.0.1:59999")

	p := &Proxy{
		AuthMode:    AuthModeAPIKey,
		APIKey:      "key",
		UpstreamURL: upstreamURL,
	}

	proxyServer := httptest.NewServer(p)
	defer proxyServer.Close()

	resp, err := http.Get(proxyServer.URL)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "Bad Gateway", string(body))
}

func TestStartCredentialProxy(t *testing.T) {
	// Create a dummy .env file in the current package directory for testing
	dummyEnv := "ANTHROPIC_API_KEY=test-key\nANTHROPIC_BASE_URL=http://127.0.0.1:0\n"
	err := os.WriteFile(".env", []byte(dummyEnv), 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(".env")

	server, err := StartCredentialProxy(0, "127.0.0.1")
	assert.NoError(t, err)
	assert.NotNil(t, server)
	// Give the server a moment to start
	server.Close()
}

func TestDetectAuthMode(t *testing.T) {
	// Test with API Key
	dummyEnv := "ANTHROPIC_API_KEY=test-key\n"
	os.WriteFile(".env", []byte(dummyEnv), 0644)
	assert.Equal(t, AuthModeAPIKey, DetectAuthMode())
	os.Remove(".env")

	// Test with OAuth (empty env)
	os.WriteFile(".env", []byte(""), 0644)
	assert.Equal(t, AuthModeOAuth, DetectAuthMode())
	os.Remove(".env")
}
