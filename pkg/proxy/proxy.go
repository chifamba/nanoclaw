package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/nanoclaw/nanoclaw/pkg/env"
	"github.com/nanoclaw/nanoclaw/pkg/logger"
)

type AuthMode string

const (
	AuthModeAPIKey AuthMode = "api-key"
	AuthModeOAuth  AuthMode = "oauth"
)

type Proxy struct {
	AuthMode    AuthMode
	APIKey      string
	OAuthToken  string
	UpstreamURL *url.URL
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = p.UpstreamURL.Scheme
			req.URL.Host = p.UpstreamURL.Host
			req.Host = p.UpstreamURL.Host

			// Strip hop-by-hop headers as in TS implementation
			req.Header.Del("Connection")
			req.Header.Del("Keep-Alive")
			req.Header.Del("Transfer-Encoding")

			if p.AuthMode == AuthModeAPIKey {
				// API key mode: inject x-api-key on every request
				req.Header.Del("x-api-key")
				if p.APIKey != "" {
					req.Header.Set("x-api-key", p.APIKey)
				}
			} else {
				// OAuth mode: replace placeholder Bearer token with the real one
				// only when the container actually sends an Authorization header
				if auth := req.Header.Get("Authorization"); auth != "" {
					req.Header.Del("Authorization")
					if p.OAuthToken != "" {
						req.Header.Set("Authorization", "Bearer "+p.OAuthToken)
					}
				}
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			logger.Error("Credential proxy upstream error", err, r.URL.Path)
			if w.Header().Get("Content-Type") == "" {
				w.WriteHeader(http.StatusBadGateway)
				w.Write([]byte("Bad Gateway"))
			}
		},
	}
	proxy.ServeHTTP(w, r)
}

func NewProxy(secrets map[string]string) (*Proxy, error) {
	authMode := AuthModeOAuth
	if secrets["ANTHROPIC_API_KEY"] != "" {
		authMode = AuthModeAPIKey
	}

	oauthToken := secrets["CLAUDE_CODE_OAUTH_TOKEN"]
	if oauthToken == "" {
		oauthToken = secrets["ANTHROPIC_AUTH_TOKEN"]
	}

	// Use ANTHROPIC_UPSTREAM_URL for the real target if set, otherwise fallback to ANTHROPIC_BASE_URL
	// but ONLY if ANTHROPIC_BASE_URL doesn't look like our own local proxy address.
	baseURLStr := secrets["ANTHROPIC_UPSTREAM_URL"]
	if baseURLStr == "" {
		baseURLStr = secrets["ANTHROPIC_BASE_URL"]
	}
	
	// If it's still empty or points to a local/gateway address that matches the common proxy pattern,
	// default to the real Anthropic API.
	if baseURLStr == "" || strings.Contains(baseURLStr, "192.168.64.1") || strings.Contains(baseURLStr, "localhost") || strings.Contains(baseURLStr, "127.0.0.1") {
		baseURLStr = "https://api.anthropic.com"
	}

	upstreamURL, err := url.Parse(baseURLStr)
	if err != nil {
		return nil, err
	}

	return &Proxy{
		AuthMode:    authMode,
		APIKey:      secrets["ANTHROPIC_API_KEY"],
		OAuthToken:  oauthToken,
		UpstreamURL: upstreamURL,
	}, nil
}

func StartCredentialProxy(port int, host string) (*http.Server, error) {
	secrets := env.ReadEnvFile([]string{
		"ANTHROPIC_API_KEY",
		"CLAUDE_CODE_OAUTH_TOKEN",
		"ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_UPSTREAM_URL",
	})

	p, err := NewProxy(secrets)
	if err != nil {
		return nil, err
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	server := &http.Server{
		Addr:    addr,
		Handler: p,
	}

	logger.Info("Credential proxy started", addr, string(p.AuthMode), "upstream", p.UpstreamURL.String())
	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			logger.Error("Credential proxy server error", err)
		}
	}()

	return server, nil
}

func DetectAuthMode() AuthMode {
	secrets := env.ReadEnvFile([]string{"ANTHROPIC_API_KEY"})
	if secrets["ANTHROPIC_API_KEY"] != "" {
		return AuthModeAPIKey
	}
	return AuthModeOAuth
}
