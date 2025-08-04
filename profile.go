package browserclient

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

var (
	viewportSizes = [][2]int{
		{1920, 1080}, {1366, 768}, {1536, 864}, {1440, 900},
		{1280, 720}, {1600, 900}, {1680, 1050}, {2560, 1440},
	}
	colorDepths = []int{24, 32}
	pixelRatios = []float32{1, 1.25, 1.5, 2}
	languages = []string{
		"pt-BR,pt;q=0.9,en;q=0.8",
		"pt-BR,pt;q=0.9",
		"en-US,en;q=0.9,pt-BR;q=0.8",
		"pt-BR,pt;q=0.9,en-US;q=0.8,en;q=0.7",
	}
	platforms = []string{"Win32", "Linux x86_64", "MacIntel"}
	vendors   = []string{"Google Inc.", "Apple Computer, Inc.", ""}
	userAgents = []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:126.0) Gecko/20100101 Firefox/126.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14.5; rv:126.0) Gecko/20100101 Firefox/126.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Safari/605.1.15",
	}
)

var threadProfiles sync.Map

func generateBrowserProfile() *BrowserProfile {
	source := rand.NewSource(time.Now().UnixNano())
	r := rand.New(source)
	
	viewport := viewportSizes[r.Intn(len(viewportSizes))]
	return &BrowserProfile{
		ViewportWidth:  viewport[0],
		ViewportHeight: viewport[1],
		ColorDepth:     colorDepths[r.Intn(len(colorDepths))],
		PixelRatio:     pixelRatios[r.Intn(len(pixelRatios))],
		Language:       languages[r.Intn(len(languages))],
		Platform:       platforms[r.Intn(len(platforms))],
		Vendor:         vendors[r.Intn(len(vendors))],
		TimezoneOffset: []int{-180, -120, -60, 0, 60, 120, 180}[r.Intn(7)],
		SessionID:      fmt.Sprintf("%d-%d", time.Now().Unix(), r.Int63()),
		CanvasNoise:    r.Float32(),
		UserAgent:      userAgents[r.Intn(len(userAgents))],
	}
}

func GetThreadProfile(threadID int) *BrowserProfile {
	if profile, ok := threadProfiles.Load(threadID); ok {
		return profile.(*BrowserProfile)
	}
	newProfile := generateBrowserProfile()
	threadProfiles.Store(threadID, newProfile)
	return newProfile
}