package browserclient

import (
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// Ordem de headers típica por navegador
var headerOrder = map[string][]string{
	"Chrome": {
		"Host",
		"Connection",
		"Cache-Control",
		"Sec-Ch-Ua",
		"Sec-Ch-Ua-Mobile",
		"Sec-Ch-Ua-Platform",
		"Upgrade-Insecure-Requests",
		"User-Agent",
		"Accept",
		"Sec-Fetch-Site",
		"Sec-Fetch-Mode",
		"Sec-Fetch-User",
		"Sec-Fetch-Dest",
		"Accept-Encoding",
		"Accept-Language",
		"Cookie",
	},
	"Firefox": {
		"Host",
		"User-Agent",
		"Accept",
		"Accept-Language",
		"Accept-Encoding",
		"Connection",
		"Upgrade-Insecure-Requests",
		"Sec-Fetch-Dest",
		"Sec-Fetch-Mode",
		"Sec-Fetch-Site",
		"Sec-Fetch-User",
		"Cache-Control",
		"Cookie",
	},
	"Safari": {
		"Host",
		"Accept-Encoding",
		"Accept",
		"User-Agent",
		"Accept-Language",
		"Connection",
		"Cookie",
	},
}

// Headers context-aware
type HeaderBuilder struct {
	profile     *BrowserProfile
	isNavigate  bool
	referrer    string
	origin      string
}

func NewHeaderBuilder(profile *BrowserProfile) *HeaderBuilder {
	return &HeaderBuilder{
		profile:    profile,
		isNavigate: true,
	}
}

func (hb *HeaderBuilder) SetContext(isNavigate bool, referrer, origin string) {
	hb.isNavigate = isNavigate
	hb.referrer = referrer
	hb.origin = origin
}

func (hb *HeaderBuilder) BuildHeaders(req *http.Request) {
	browser := detectBrowser(hb.profile.UserAgent)
	headers := hb.generateHeaders(req, browser)
	
	// Limpar headers existentes
	req.Header = make(http.Header)
	
	// Aplicar headers na ordem correta
	order := headerOrder[browser]
	if order == nil {
		order = headerOrder["Chrome"] // fallback
	}
	
	// Adicionar headers ordenados
	for _, key := range order {
		if value, exists := headers[key]; exists {
			req.Header[key] = value
		}
	}
	
	// Adicionar headers não ordenados
	for key, value := range headers {
		if req.Header.Get(key) == "" {
			req.Header[key] = value
		}
	}
}

func (hb *HeaderBuilder) generateHeaders(req *http.Request, browser string) map[string][]string {
	headers := make(map[string][]string)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	
	// Headers comuns
	headers["User-Agent"] = []string{hb.profile.UserAgent}
	headers["Accept-Language"] = []string{hb.profile.Language}
	headers["Connection"] = []string{"keep-alive"}
	
	// Accept header baseado no contexto
	if hb.isNavigate {
		headers["Accept"] = []string{getNavigationAccept(browser)}
	} else {
		headers["Accept"] = []string{getResourceAccept(req.URL.Path)}
	}
	
	// Accept-Encoding
	headers["Accept-Encoding"] = []string{getAcceptEncoding(browser)}
	
	// Headers específicos do navegador
	switch browser {
	case "Chrome":
		hb.addChromeHeaders(headers, r)
	case "Firefox":
		hb.addFirefoxHeaders(headers, r)
	case "Safari":
		hb.addSafariHeaders(headers, r)
	}
	
	// Headers condicionais
	if hb.referrer != "" {
		headers["Referer"] = []string{hb.referrer}
	}
	
	if hb.origin != "" && !hb.isNavigate {
		headers["Origin"] = []string{hb.origin}
	}
	
	// Headers aleatórios
	if r.Float32() < 0.3 {
		headers["DNT"] = []string{"1"}
	}
	
	if r.Float32() < 0.2 {
		headers["Cache-Control"] = []string{"no-cache"}
	} else if r.Float32() < 0.4 {
		headers["Cache-Control"] = []string{"max-age=0"}
	}
	
	return headers
}

func (hb *HeaderBuilder) addChromeHeaders(headers map[string][]string, r *rand.Rand) {
	// Extrair versão do Chrome
	version := "126"
	if matches := strings.Split(hb.profile.UserAgent, "Chrome/"); len(matches) > 1 {
		if parts := strings.Split(matches[1], "."); len(parts) > 0 {
			version = parts[0]
		}
	}
	
	// Sec-CH-UA headers
	headers["Sec-Ch-Ua"] = []string{fmt.Sprintf(`"Not)A;Brand";v="99", "Google Chrome";v="%s", "Chromium";v="%s"`, version, version)}
	headers["Sec-Ch-Ua-Mobile"] = []string{"?0"}
	
	// Platform baseado no User-Agent
	platform := "Windows"
	if strings.Contains(hb.profile.UserAgent, "Macintosh") {
		platform = "macOS"
	} else if strings.Contains(hb.profile.UserAgent, "X11") {
		platform = "Linux"
	}
	headers["Sec-Ch-Ua-Platform"] = []string{fmt.Sprintf(`"%s"`, platform)}
	
	// Sec-Fetch headers
	headers["Sec-Fetch-Site"] = []string{hb.getSecFetchSite()}
	headers["Sec-Fetch-Mode"] = []string{hb.getSecFetchMode()}
	headers["Sec-Fetch-Dest"] = []string{hb.getSecFetchDest()}
	
	if hb.isNavigate {
		headers["Sec-Fetch-User"] = []string{"?1"}
		headers["Upgrade-Insecure-Requests"] = []string{"1"}
	}
	
	// Chrome às vezes envia Sec-CH-UA-Platform-Version
	if r.Float32() < 0.3 {
		headers["Sec-Ch-Ua-Platform-Version"] = []string{`"10.0.0"`}
	}
}

func (hb *HeaderBuilder) addFirefoxHeaders(headers map[string][]string, r *rand.Rand) {
	headers["Upgrade-Insecure-Requests"] = []string{"1"}
	
	// Firefox Sec-Fetch headers (mais recentes)
	version := 126
	if matches := strings.Split(hb.profile.UserAgent, "Firefox/"); len(matches) > 1 {
		if parts := strings.Split(matches[1], "."); len(parts) > 0 {
			fmt.Sscanf(parts[0], "%d", &version)
		}
	}
	
	if version >= 90 {
		headers["Sec-Fetch-Dest"] = []string{hb.getSecFetchDest()}
		headers["Sec-Fetch-Mode"] = []string{hb.getSecFetchMode()}
		headers["Sec-Fetch-Site"] = []string{hb.getSecFetchSite()}
		if hb.isNavigate {
			headers["Sec-Fetch-User"] = []string{"?1"}
		}
	}
	
	// TE header específico do Firefox
	if r.Float32() < 0.7 {
		headers["TE"] = []string{"trailers"}
	}
}

func (hb *HeaderBuilder) addSafariHeaders(headers map[string][]string, r *rand.Rand) {
	// Safari tem menos headers especiais
	if hb.isNavigate {
		headers["Upgrade-Insecure-Requests"] = []string{"1"}
	}
	
	// Safari não usa Sec-Fetch headers
	// Mas tem ordem específica de Accept-Encoding
	headers["Accept-Encoding"] = []string{"gzip, deflate, br"}
}

func (hb *HeaderBuilder) getSecFetchSite() string {
	if hb.referrer == "" {
		return "none"
	}
	if hb.origin != "" && strings.HasPrefix(hb.referrer, hb.origin) {
		return "same-origin"
	}
	return "cross-site"
}

func (hb *HeaderBuilder) getSecFetchMode() string {
	if hb.isNavigate {
		return "navigate"
	}
	return "no-cors"
}

func (hb *HeaderBuilder) getSecFetchDest() string {
	if hb.isNavigate {
		return "document"
	}
	return "empty"
}

func detectBrowser(userAgent string) string {
	if strings.Contains(userAgent, "Firefox") {
		return "Firefox"
	}
	if strings.Contains(userAgent, "Safari") && !strings.Contains(userAgent, "Chrome") {
		return "Safari"
	}
	if strings.Contains(userAgent, "Edg/") {
		return "Edge"
	}
	return "Chrome"
}

func getNavigationAccept(browser string) string {
	switch browser {
	case "Firefox":
		return "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"
	case "Safari":
		return "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	default: // Chrome/Edge
		return "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"
	}
}

func getResourceAccept(path string) string {
	ext := strings.ToLower(path[strings.LastIndex(path, ".")+1:])
	switch ext {
	case "js":
		return "*/*"
	case "css":
		return "text/css,*/*;q=0.1"
	case "jpg", "jpeg", "png", "gif", "webp":
		return "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8"
	default:
		return "*/*"
	}
}

func getAcceptEncoding(browser string) string {
	if browser == "Safari" {
		return "gzip, deflate, br"
	}
	// Chrome/Firefox/Edge
	return "gzip, deflate, br, zstd"
}