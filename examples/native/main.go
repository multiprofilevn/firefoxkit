//go:build windows

// Example: native winapi input backend on Windows.
//
// Launches Firefox with a visible window, dispatches a click + keyboard
// combo via the OS input layer. Useful when scraping sites that fingerprint
// synthetic events (isTrusted=false).
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/tuwibu/firefoxkit"
)

func main() {
	binary := flag.String("firefox", `C:\Program Files\Mozilla Firefox\firefox.exe`, "absolute path to firefox.exe")
	target := flag.String("url", "https://example.com", "URL to open")
	flag.Parse()

	b, err := firefoxkit.Launch(*binary,
		firefoxkit.WithHeadless(false),
		firefoxkit.WithInputBackend(firefoxkit.BackendNative),
		firefoxkit.WithNativeWindow(100, 100, 1.0),
	)
	if err != nil {
		log.Fatalf("Launch: %v", err)
	}
	defer b.Close()

	if err := b.FocusWindow(); err != nil {
		log.Printf("FocusWindow: %v (input may miss)", err)
	}

	p := b.Current()

	navCtx, navCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := p.Navigate(navCtx, *target); err != nil {
		navCancel()
		log.Fatalf("Navigate: %v", err)
	}
	navCancel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := p.WaitForReady(ctx, 10*time.Second); err != nil {
		log.Printf("WaitForReady: %v", err)
	}

	// Click at the center of the viewport — replace with a real bounding-box
	// lookup in real code.
	if err := p.Mouse().ClickAt(ctx, 400, 300); err != nil {
		log.Printf("ClickAt: %v", err)
	}

	if err := p.Keyboard().Type(ctx, "Hello from native input"); err != nil {
		log.Printf("Type: %v", err)
	}

	time.Sleep(2 * time.Second)
	log.Printf("done")
}
