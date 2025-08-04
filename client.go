package browserclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"time"
	"strings"
	
	"golang.org/x/net/publicsuffix"
)

// BrowserClient wraps http.Client with additional browser-like behavior
type BrowserClient struct {
	*http.Client
	profile       *BrowserProfile
	config        *ClientConfig
	cookieJar     http.CookieJar
	headerBuilder *HeaderBuilder
	history       []string
	mu            sync.RWMutex
}

// RequestOptions permite customização por request
type RequestOptions struct {
	Headers         map[string]string
	IsNavigate      bool
	Referrer        string
	Origin          string
	FollowRedirects bool
	MaxRedirects    int
}

// NewBrowserClient cria um cliente completo com comportamento de navegador
func NewBrowserClient(config *ClientConfig) (*BrowserClient, error) {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	profile := GetThreadProfile(config.ThreadID)
	
	// Criar cookie jar com política de public suffix
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	transport, err := createBrowserTransport(config, profile)
	if err != nil {
		return nil, err
	}

	client := &BrowserClient{
		Client: &http.Client{
			Transport: transport,
			Timeout:   config.Timeout,
			Jar:       jar,
		},
		profile:       profile,
		config:        config,
		cookieJar:     jar,
		headerBuilder: NewHeaderBuilder(profile),
		history:       make([]string, 0, 10),
	}

	// Configurar política de redirect customizada
	client.Client.CheckRedirect = client.checkRedirect

	return client, nil
}

// createBrowserTransport cria o transport com todas as configurações
func createBrowserTransport(config *ClientConfig, profile *BrowserProfile) (http.RoundTripper, error) {
	transport := &http.Transport{
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialTLS(ctx, network, addr, config, profile)
		},
	}

	// Configurar proxy se fornecido
	if config.ProxyURL != "" {
		parsedProxy, err := url.Parse(config.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}

		transport.Proxy = http.ProxyURL(parsedProxy)
		
		// Adicionar autenticação do proxy se necessário
		if parsedProxy.User != nil {
			auth := parsedProxy.User.String()
			basicAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
			transport.ProxyConnectHeader = http.Header{
				"Proxy-Authorization": []string{basicAuth},
				"User-Agent":          []string{profile.UserAgent},
			}
		}
	}

	return transport, nil
}

// Get realiza uma requisição GET com comportamento de navegador
func (bc *BrowserClient) Get(url string, options ...RequestOptions) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	return bc.Do(req, options...)
}

// Post realiza uma requisição POST
func (bc *BrowserClient) Post(url string, contentType string, body []byte, options ...RequestOptions) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
	
	return bc.Do(req, options...)
}

// Do executa uma requisição com comportamento completo de navegador
func (bc *BrowserClient) Do(req *http.Request, options ...RequestOptions) (*http.Response, error) {
	opts := bc.mergeOptions(options...)
	
	// Configurar contexto da requisição
	bc.headerBuilder.SetContext(opts.IsNavigate, opts.Referrer, opts.Origin)
	
	// Construir headers apropriados
	bc.headerBuilder.BuildHeaders(req)
	
	// Aplicar headers customizados
	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}
	
	// Executar requisição
	resp, err := bc.Client.Do(req)
	if err != nil {
		return nil, err
	}
	
	// Atualizar histórico
	bc.updateHistory(req.URL.String())
	
	return resp, nil
}

// StreamGet realiza download com streaming
func (bc *BrowserClient) StreamGet(url string, config *StreamConfig, options ...RequestOptions) (*StreamResult, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	
	// Aplicar headers
	opts := bc.mergeOptions(options...)
	bc.headerBuilder.SetContext(opts.IsNavigate, opts.Referrer, opts.Origin)
	bc.headerBuilder.BuildHeaders(req)
	
	// Fazer requisição sem seguir redirects para streaming
	client := &http.Client{
		Transport: bc.Client.Transport,
		Timeout:   bc.Client.Timeout,
		Jar:       bc.Client.Jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	return StreamResponse(resp, config)
}

// GetWithRetry tenta múltiplas vezes com backoff exponencial
func (bc *BrowserClient) GetWithRetry(url string, maxRetries int, options ...RequestOptions) (*http.Response, error) {
	var lastErr error
	backoff := 1 * time.Second
	
	for i := 0; i <= maxRetries; i++ {
		resp, err := bc.Get(url, options...)
		if err == nil && resp.StatusCode < 500 {
			return resp, nil
		}
		
		if resp != nil {
			resp.Body.Close()
		}
		
		lastErr = err
		if i < maxRetries {
			time.Sleep(backoff)
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
	
	return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// checkRedirect implementa política de redirect customizada
func (bc *BrowserClient) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	
	// Manter headers importantes durante redirects
	if len(via) > 0 {
		prevReq := via[len(via)-1]
		
		// Atualizar referrer
		bc.headerBuilder.SetContext(true, prevReq.URL.String(), "")
		bc.headerBuilder.BuildHeaders(req)
		
		// Preservar alguns headers customizados
		for _, header := range []string{"Authorization", "X-Requested-With"} {
			if val := prevReq.Header.Get(header); val != "" {
				req.Header.Set(header, val)
			}
		}
	}
	
	return nil
}

// mergeOptions combina opções padrão com as fornecidas
func (bc *BrowserClient) mergeOptions(options ...RequestOptions) RequestOptions {
	opts := RequestOptions{
		IsNavigate:      true,
		FollowRedirects: true,
		MaxRedirects:    10,
		Headers:         make(map[string]string),
	}
	
	if len(options) > 0 {
		opt := options[0]
		if opt.Headers != nil {
			opts.Headers = opt.Headers
		}
		opts.IsNavigate = opt.IsNavigate
		if opt.Referrer != "" {
			opts.Referrer = opt.Referrer
		}
		if opt.Origin != "" {
			opts.Origin = opt.Origin
		}
		opts.FollowRedirects = opt.FollowRedirects
		if opt.MaxRedirects > 0 {
			opts.MaxRedirects = opt.MaxRedirects
		}
	}
	
	// Auto-referrer do histórico
	if opts.Referrer == "" && len(bc.history) > 0 {
		bc.mu.RLock()
		opts.Referrer = bc.history[len(bc.history)-1]
		bc.mu.RUnlock()
	}
	
	return opts
}

// updateHistory atualiza o histórico de navegação
func (bc *BrowserClient) updateHistory(url string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	
	bc.history = append(bc.history, url)
	if len(bc.history) > 10 {
		bc.history = bc.history[1:]
	}
}

// GetCookies retorna cookies para uma URL específica
func (bc *BrowserClient) GetCookies(urlStr string) ([]*http.Cookie, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	return bc.cookieJar.Cookies(u), nil
}

// SetCookie adiciona um cookie manualmente
func (bc *BrowserClient) SetCookie(urlStr string, cookie *http.Cookie) error {
	u, err := url.Parse(urlStr)
	if err != nil {
		return err
	}
	bc.cookieJar.SetCookies(u, []*http.Cookie{cookie})
	return nil
}

// ClearCookies limpa todos os cookies
func (bc *BrowserClient) ClearCookies() {
	jar, _ := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	bc.cookieJar = jar
	bc.Client.Jar = jar
}

// GetProfile retorna o perfil do navegador
func (bc *BrowserClient) GetProfile() *BrowserProfile {
	return bc.profile
}

// Close fecha conexões idle
func (bc *BrowserClient) Close() {
	if transport, ok := bc.Client.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
}

// SetRequestHeaders aplica headers específicos do navegador (função do arquivo original adaptada)
func SetRequestHeaders(req *http.Request, profile *BrowserProfile) {
	source := rand.NewSource(time.Now().UnixNano())
	r := rand.New(source)

	req.Header.Set("User-Agent", profile.UserAgent)
	req.Header.Set("Accept-Language", profile.Language)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Connection", "keep-alive")

	if strings.Contains(profile.UserAgent, "Chrome") {
		req.Header.Set("Sec-Ch-Ua", `"Not.A/Brand";v="8", "Chromium";v="126", "Google Chrome";v="126"`)
		req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
		req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Sec-Fetch-User", "?1")
	} else if strings.Contains(profile.UserAgent, "Firefox") {
		req.Header.Set("Upgrade-Insecure-Requests", "1")
		req.Header.Set("TE", "trailers")
	} else if strings.Contains(profile.UserAgent, "Safari") {
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	}

	if r.Intn(2) == 0 {
		req.Header.Set("DNT", "1")
	}

	if r.Intn(3) == 0 {
		req.Header.Set("Cache-Control", "no-cache")
	}
	
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
}