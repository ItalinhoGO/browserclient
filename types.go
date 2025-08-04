package browserclient

import (
	"time"
)

type ClientConfig struct {
	ProxyURL        string
	DisableTLSVerify bool
	RandomizeTLS    bool
	ThreadID        int
	Timeout         time.Duration
}

type BrowserProfile struct {
	ViewportWidth  int
	ViewportHeight int
	ColorDepth     int
	PixelRatio     float32
	Language       string
	Platform       string
	Vendor         string
	TimezoneOffset int
	SessionID      string
	CanvasNoise    float32
	UserAgent      string
}

type StreamConfig struct {
	StopOnContent string
	BufferSize    int
	MaxBytes      int64
}

type StreamResult struct {
	BytesRead    int64
	Content      string
	FoundContent bool
}