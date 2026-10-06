package cmd

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
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

const (
	browserLoginTimeout = 5 * time.Minute

	// oauthClientID is uptimyctl's built-in OAuth client (upti.my-api utils/oauth.ts).
	oauthClientID = "uptimyctl"
	// oobRedirectURI asks the consent page to show the code instead of redirecting.
	oobRedirectURI = "urn:ietf:wg:oauth:2.0:oob"
)

const callbackPage = `<!doctype html>
<html><head><meta charset="utf-8"><title>uptimyctl</title>
<style>body{font-family:system-ui,sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;background:#0b1020;color:#e6e8ee}
main{text-align:center}h1{font-size:1.4rem;margin:0 0 .5rem}p{color:#9aa3b5;margin:0}</style></head>
<body><main><h1>%s</h1><p>%s</p></main></body></html>`

func writeCallbackPage(w http.ResponseWriter, status int, title, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, callbackPage, html.EscapeString(title), html.EscapeString(message))
}

// pkce is an RFC 7636 verifier and its S256 challenge.
type pkce struct{ verifier, challenge string }

func newPKCE() (pkce, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return pkce{}, fmt.Errorf("generate PKCE verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	return pkce{verifier: verifier, challenge: base64.RawURLEncoding.EncodeToString(sum[:])}, nil
}

// authorizeURL is the app's OAuth consent page for this login.
func authorizeURL(appURL, redirectURI, state string, p pkce) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", oauthClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("code_challenge", p.challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("host", hostLabel())
	if state != "" {
		q.Set("state", state)
	}
	return strings.TrimRight(appURL, "/") + "/oauth/authorize?" + q.Encode()
}

// exchangeCode trades a one-time code for an API key at the API's token
// endpoint. The key is created only now, so an abandoned login leaves none.
func exchangeCode(apiURL, code, redirectURI string, p pkce) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", oauthClientID)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", p.verifier)

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.PostForm(strings.TrimRight(apiURL, "/")+"/oauth/token", form)
	if err != nil {
		return "", fmt.Errorf("exchange code: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	var out struct {
		AccessToken      string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	_ = json.Unmarshal(body, &out)
	if resp.StatusCode != http.StatusOK {
		if out.ErrorDescription != "" {
			return "", fmt.Errorf("exchange code: %s", out.ErrorDescription)
		}
		return "", fmt.Errorf("exchange code: HTTP %d", resp.StatusCode)
	}
	if !strings.HasPrefix(out.AccessToken, "upt_") {
		return "", errors.New("exchange code: Uptimy didn't return an API key")
	}
	return out.AccessToken, nil
}

// browserLogin gets an API key through OAuth, the way `gh auth login` does
// (RFC 8252): it listens on a loopback port, opens the consent page, and waits
// for the browser to come back with a one-time code, which it exchanges using
// its PKCE verifier. The random state ties the callback to this login.
func browserLogin(appURL, apiURL string) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("start local callback server: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	expectedHost := "127.0.0.1:" + strconv.Itoa(port)
	redirectURI := "http://" + expectedHost + "/callback"

	state, err := randomState()
	if err != nil {
		return "", err
	}
	p, err := newPKCE()
	if err != nil {
		return "", err
	}

	type result struct {
		key string
		err error
	}
	results := make(chan result, 1)
	finish := func(r result) {
		select {
		case results <- r:
		default:
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		// The Host check stops DNS-rebinding pages from reaching the listener.
		if r.Method != http.MethodGet || r.Host != expectedHost {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 {
			writeCallbackPage(w, http.StatusBadRequest, "Invalid login callback", "Run 'uptimyctl auth login' again.")
			return
		}
		if e := q.Get("error"); e != "" {
			writeCallbackPage(w, http.StatusOK, "Login cancelled", "No API key was created. You can close this tab.")
			finish(result{err: errors.New("authorization was cancelled in the browser")})
			return
		}
		key, err := exchangeCode(apiURL, q.Get("code"), redirectURI, p)
		if err != nil {
			writeCallbackPage(w, http.StatusBadGateway, "Login failed", err.Error())
			finish(result{err: err})
			return
		}
		writeCallbackPage(w, http.StatusOK, "uptimyctl is signed in", "You can close this tab and return to your terminal.")
		finish(result{key: key})
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		// Shutdown (not Close) lets the result page finish writing.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	authURL := authorizeURL(appURL, redirectURI, state, p)
	fmt.Fprintln(os.Stderr, "Opening your browser to authorize uptimyctl...")
	fmt.Fprintf(os.Stderr, "If it doesn't open, visit this URL:\n\n  %s\n\n", authURL)
	fmt.Fprintln(os.Stderr, "Working over SSH or on a headless machine? Press Ctrl+C and run 'uptimyctl auth login --no-browser'.")
	_ = openBrowser(authURL)

	select {
	case r := <-results:
		return r.key, r.err
	case <-time.After(browserLoginTimeout):
		return "", errors.New("timed out waiting for browser authorization; run 'uptimyctl auth login --no-browser' to paste a code instead")
	}
}

// manualLogin is --no-browser: the consent page (opened on any device) shows a
// one-time code, which is exchanged here. A pasted API key is accepted too.
func manualLogin(appURL, apiURL string, prompt io.Writer, readLine func() string) (string, error) {
	p, err := newPKCE()
	if err != nil {
		return "", err
	}
	_, _ = fmt.Fprintf(prompt, "Open this URL in a browser, approve access, then paste the code it shows:\n\n  %s\n\n", authorizeURL(appURL, oobRedirectURI, "", p))
	_, _ = fmt.Fprint(prompt, "Code: ")
	input := strings.TrimSpace(readLine())
	if input == "" || strings.HasPrefix(input, "upt_") {
		return input, nil
	}
	return exchangeCode(apiURL, input, oobRedirectURI, p)
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
