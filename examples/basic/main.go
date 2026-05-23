// Example: connect to a running Firefox over BiDi, navigate, click, screenshot.
//
// Prerequisite: start Firefox with
//
//	firefox --remote-debugging-port=9333 -profile <tmpdir> -no-remote
//
// Then run this example. Works on any OS where Firefox 129+ is installed.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/tuwibu/firefoxkit"
)

func main() {
	port := flag.Int("port", 9333, "BiDi port Firefox is listening on")
	target := flag.String("url", "https://example.com", "URL to navigate to")
	out := flag.String("out", "screenshot.png", "screenshot output path")
	flag.Parse()

	b, err := firefoxkit.Connect(*port)
	if err != nil {
		log.Fatalf("Connect: %v", err)
	}
	defer b.Close()

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
		log.Printf("WaitForReady: %v (continuing)", err)
	}

	title, _ := p.TitleLive(ctx)
	log.Printf("title: %q", title)

	img, err := p.Screenshot(ctx)
	if err != nil {
		log.Fatalf("Screenshot: %v", err)
	}
	if err := os.WriteFile(*out, img, 0o644); err != nil {
		log.Fatalf("write %s: %v", *out, err)
	}
	log.Printf("wrote %s (%d bytes)", *out, len(img))
}
