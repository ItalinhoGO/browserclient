package browserclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"

	utls "github.com/refraction-networking/utls"
)

// Mapeamento mais preciso de fingerprints por User-Agent
var browserFingerprints = map[string][]utls.ClientHelloID{
	"Chrome": {
		utls.HelloChrome_Auto,
		utls.HelloChrome_120,
	},
	"Firefox": {
		utls.HelloFirefox_Auto,
		utls.HelloFirefox_120,
	},
	"Safari": {
		utls.HelloSafari_Auto,
		utls.HelloSafari_16_0,
		utls.HelloIOS_Auto,
	},
	"Edge": {
		utls.HelloEdge_Auto,
		utls.HelloChrome_120, // Edge usa engine Chromium
	},
}

func dialTLS(ctx context.Context, network, addr string, config *ClientConfig, profile *BrowserProfile) (net.Conn, error) {
	// Configurar timeout para o dial
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}
	
	rawConn, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %w", err)
	}

	host, _, _ := net.SplitHostPort(addr)
	
	// Configuração TLS base
	tlsConfig := &utls.Config{
		ServerName:         host,
		InsecureSkipVerify: config.DisableTLSVerify,
		NextProtos:         getALPNProtocols(profile.UserAgent),
		MinVersion:         tls.VersionTLS12,
		MaxVersion:         tls.VersionTLS13,
	}

	if !config.DisableTLSVerify {
		tlsConfig.RootCAs = getSystemCertPool()
	}

	// Selecionar fingerprint baseado no navegador
	fingerprint := selectFingerprint(profile.UserAgent, config.RandomizeTLS)
	
	uConn := utls.UClient(rawConn, tlsConfig, fingerprint)
	
	// Aplicar configurações específicas do navegador se necessário
	if err := applyBrowserSpecificSettings(uConn, profile); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("failed to apply browser settings: %w", err)
	}

	// Handshake com timeout
	handshakeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- uConn.HandshakeContext(handshakeCtx)
	}()

	select {
	case err := <-errChan:
		if err != nil {
			rawConn.Close()
			return nil, fmt.Errorf("TLS handshake failed: %w", err)
		}
		return &tlsConn{uConn, profile}, nil
	case <-handshakeCtx.Done():
		rawConn.Close()
		return nil, fmt.Errorf("TLS handshake timeout: %w", handshakeCtx.Err())
	}
}

// Wrapper para adicionar informações do perfil à conexão
type tlsConn struct {
	*utls.UConn
	profile *BrowserProfile
}

func selectFingerprint(userAgent string, randomize bool) utls.ClientHelloID {
	if randomize {
		return utls.HelloRandomized
	}

	source := rand.NewSource(time.Now().UnixNano())
	r := rand.New(source)

	// Identificar o navegador
	browser := "Chrome" // default
	for b := range browserFingerprints {
		if strings.Contains(userAgent, b) {
			browser = b
			break
		}
	}

	fingerprints := browserFingerprints[browser]
	return fingerprints[r.Intn(len(fingerprints))]
}

func getALPNProtocols(userAgent string) []string {
	// Safari às vezes não anuncia h2
	if strings.Contains(userAgent, "Safari") && !strings.Contains(userAgent, "Chrome") {
		if rand.Float32() < 0.3 {
			return []string{"http/1.1"}
		}
	}
	return []string{"h2", "http/1.1"}
}

func applyBrowserSpecificSettings(uConn *utls.UConn, profile *BrowserProfile) error {
	// Para fingerprints específicos, podemos customizar ainda mais
	// Nota: Com versões recentes do uTLS, a customização é mais limitada
	// para manter a integridade do fingerprint
	
	if strings.Contains(profile.UserAgent, "Firefox") {
		// Firefox específico já está configurado no ClientHelloID
		return nil
	}
	
	if strings.Contains(profile.UserAgent, "Chrome") {
		// Chrome específico já está configurado no ClientHelloID
		return nil
	}
	
	return nil
}

func getSystemCertPool() *x509.CertPool {
	pool, err := x509.SystemCertPool()
	if err != nil {
		// Fallback para pool vazio se falhar
		return x509.NewCertPool()
	}
	return pool
}

// Função auxiliar para debug de TLS (opcional)
func debugTLSInfo(conn *utls.UConn) {
	state := conn.ConnectionState()
	fmt.Printf("TLS Version: %x\n", state.Version)
	fmt.Printf("Cipher Suite: %x\n", state.CipherSuite)
	fmt.Printf("Server Name: %s\n", state.ServerName)
	fmt.Printf("Negotiated Protocol: %s\n", state.NegotiatedProtocol)
}