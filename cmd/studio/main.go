// gram-grabber-studio: the visual version of GramGrabber.
// It runs a local-only web server and opens the interface in your browser.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"telegram-downloader/internal/studio"
)

func main() {
	port := flag.Int("port", 8410, "port to listen on (0 picks a free one)")
	noOpen := flag.Bool("no-open", false, "do not open the browser automatically")
	flag.Parse()

	engine, err := studio.NewEngine()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// Port taken. If it's another Studio instance (e.g. the .app was
		// double-clicked twice), just focus the existing one.
		if studioRunningAt(addr) {
			fmt.Printf("GramGrabber Studio is already running at http://%s\n", addr)
			if !*noOpen {
				openBrowser("http://" + addr)
			}
			return
		}
		// Something else owns the port: fall back to a random free one.
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}
	}

	url := fmt.Sprintf("http://%s", ln.Addr().String())
	fmt.Printf("GramGrabber Studio running at %s\n", url)
	fmt.Println("Press Ctrl+C to quit.")

	if !*noOpen {
		go func() {
			time.Sleep(300 * time.Millisecond)
			openBrowser(url)
		}()
	}

	if err := http.Serve(ln, studio.NewServer(engine)); err != nil {
		log.Fatalln(err)
	}
}

func studioRunningAt(addr string) bool {
	client := http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Get("http://" + addr + "/api/state")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		fmt.Printf("Open %s in your browser.\n", url)
	}
}
