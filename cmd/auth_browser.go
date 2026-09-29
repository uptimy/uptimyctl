package cmd

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const browserLoginTimeout = 5 * time.Minute

const callbackPage = `<!doctype html>
<html><head><meta charset="utf-8"><title>uptimyctl</title>
<style>body{font-family:system-ui,sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;background:#0b1020;color:#e6e8ee}
main{text-align:center}h1{font-size:1.4rem;margin:0 0 .5rem}p{color:#9aa3b5;margin:0}</style></head>
<body><main><h1>uptimyctl is signed in</h1><p>You can close this tab and return to your terminal.</p></main></body></html>`

// browserLogin gets an API key through the web app, the way `gh auth login`
// does: it listens on a loopback port, opens /cli/authorize, and waits for the
// app to POST the new key back. The random state ties the callback to this
// login, so no other page can plant its own key in our config.
func browserLogin(appURL string) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("start local callback server: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	expectedHost := "127.0.0.1:" + strconv.Itoa(port)

	state, err := randomState()
	if err != nil {
		return "", err
	}

	keys := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		// The Host check stops DNS-rebinding pages from reaching the listener.
		if r.Method != http.MethodPost || r.Host != expectedHost {
			http.NotFound(w, r)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		key := r.PostFormValue("key")
		gotState := r.PostFormValue("state")
		if subtle.ConstantTimeCompare([]byte(gotState), []byte(state)) != 1 || !strings.HasPrefix(key, "upt_") {
			http.Error(w, "Invalid login callback. Run 'uptimyctl auth login' again.", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = fmt.Fprint(w, callbackPage)
		select {
		case keys <- key:
		default:
		}
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		// Shutdown (not Close) lets the success page finish writing.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	q := url.Values{}
	q.Set("port", strconv.Itoa(port))
	q.Set("state", state)
	q.Set("host", hostLabel())
	authURL := strings.TrimRight(appURL, "/") + "/cli/authorize?" + q.Encode()

	fmt.Fprintln(os.Stderr, "Opening your browser to authorize uptimyctl...")
	fmt.Fprintf(os.Stderr, "If it doesn't open, visit this URL:\n\n  %s\n\n", authURL)
	fmt.Fprintln(os.Stderr, "Working over SSH or on a headless machine? Press Ctrl+C and run 'uptimyctl auth login --no-browser'.")
	_ = openBrowser(authURL)

	select {
	case key := <-keys:
		return key, nil
	case <-time.After(browserLoginTimeout):
		return "", errors.New("timed out waiting for browser authorization; run 'uptimyctl auth login --no-browser' to paste a key instead")
	}
}

// manualLoginURL is the /cli/authorize link without a port: the app then shows
// the new key for copy-paste instead of sending it to a local listener.
func manualLoginURL(appURL string) string {
	q := url.Values{}
	q.Set("host", hostLabel())
	return strings.TrimRight(appURL, "/") + "/cli/authorize?" + q.Encode()
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate login state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hostLabel names the machine on the consent screen and in the API key name,
// so each machine's key can be told apart and revoked.
func hostLabel() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown host"
	}
	return h
}

// openBrowser is a variable so tests can capture the URL instead of launching a browser.
var openBrowser = func(u string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", u).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}
