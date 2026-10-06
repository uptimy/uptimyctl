package cmd

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// fakeTokenEndpoint stands in for the API's /oauth/token: it accepts one code
// and checks the PKCE verifier against the challenge from the consent link.
type fakeTokenEndpoint struct {
	*httptest.Server
	mu        sync.Mutex
	code      string
	challenge string
	redirect  string
	exchanges int
}

func newFakeTokenEndpoint(t *testing.T, code string) *fakeTokenEndpoint {
	f := &fakeTokenEndpoint{code: code}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = r.ParseForm()
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		w.Header().Set("Content-Type", "application/json")
		ok := r.URL.Path == "/oauth/token" &&
			r.PostForm.Get("grant_type") == "authorization_code" &&
			r.PostForm.Get("client_id") == "uptimyctl" &&
			r.PostForm.Get("code") == f.code &&
			r.PostForm.Get("redirect_uri") == f.redirect &&
			base64.RawURLEncoding.EncodeToString(sum[:]) == f.challenge
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant", "error_description": "The authorization code is invalid or expired"})
			return
		}
		f.exchanges++
		f.code = "" // single use
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "upt_good", "token_type": "Bearer", "expires_in": 7776000})
	}))
	t.Cleanup(f.Close)
	return f
}

// consentLink checks the authorize URL uptimyctl opens and records what the
// token endpoint must see.
func (f *fakeTokenEndpoint) consentLink(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Host != "app.example" || u.Path != "/oauth/authorize" || q.Get("response_type") != "code" ||
		q.Get("client_id") != "uptimyctl" || q.Get("code_challenge_method") != "S256" ||
		len(q.Get("code_challenge")) != 43 || q.Get("host") == "" {
		t.Fatalf("unexpected authorize URL %s", u)
	}
	f.mu.Lock()
	f.challenge, f.redirect = q.Get("code_challenge"), q.Get("redirect_uri")
	f.mu.Unlock()
	return q
}

func TestBrowserLoginExchangesTheCode(t *testing.T) {
	api := newFakeTokenEndpoint(t, "code-1")
	opened := make(chan string, 1)
	orig := openBrowser
	openBrowser = func(u string) error { opened <- u; return nil }
	defer func() { openBrowser = orig }()

	type result struct {
		key string
		err error
	}
	done := make(chan result, 1)
	go func() {
		key, err := browserLogin("https://app.example", api.URL)
		done <- result{key, err}
	}()

	q := api.consentLink(t, <-opened)
	callback, state := q.Get("redirect_uri"), q.Get("state")
	cb, _ := url.Parse(callback)
	if cb.Scheme != "http" || cb.Hostname() != "127.0.0.1" || cb.Path != "/callback" || len(state) < 32 {
		t.Fatalf("unexpected redirect_uri %q / state %q", callback, state)
	}

	get := func(host string, params url.Values) int {
		req, _ := http.NewRequest(http.MethodGet, callback+"?"+params.Encode(), nil)
		if host != "" {
			req.Host = host
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	if code := get("localhost:"+cb.Port(), url.Values{"code": {"code-1"}, "state": {state}}); code != http.StatusNotFound {
		t.Fatalf("foreign Host header: want 404, got %d", code)
	}
	if code := get("", url.Values{"code": {"code-1"}, "state": {"wrong"}}); code != http.StatusBadRequest {
		t.Fatalf("wrong state: want 400, got %d", code)
	}
	if code := get("", url.Values{"code": {"code-1"}, "state": {state}}); code != http.StatusOK {
		t.Fatalf("valid callback: want 200, got %d", code)
	}

	r := <-done
	if r.err != nil || r.key != "upt_good" || api.exchanges != 1 {
		t.Fatalf("browserLogin = %q, %v (exchanges %d)", r.key, r.err, api.exchanges)
	}
}

func TestBrowserLoginCancelled(t *testing.T) {
	api := newFakeTokenEndpoint(t, "code-1")
	opened := make(chan string, 1)
	orig := openBrowser
	openBrowser = func(u string) error { opened <- u; return nil }
	defer func() { openBrowser = orig }()

	done := make(chan error, 1)
	go func() {
		_, err := browserLogin("https://app.example", api.URL)
		done <- err
	}()
	q := api.consentLink(t, <-opened)
	resp, err := http.Get(q.Get("redirect_uri") + "?" + url.Values{"error": {"access_denied"}, "state": {q.Get("state")}}.Encode())
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if err := <-done; err == nil || api.exchanges != 0 {
		t.Fatalf("want a cancellation error and no exchange, got %v (exchanges %d)", err, api.exchanges)
	}
}

func TestManualLoginExchangesThePastedCode(t *testing.T) {
	api := newFakeTokenEndpoint(t, "code-2")
	orig := openBrowser
	openBrowser = func(string) error { t.Fatal("--no-browser must not open a browser"); return nil }
	defer func() { openBrowser = orig }()

	var prompt strings.Builder
	key, err := manualLogin("https://app.example", api.URL, &prompt, func() string {
		// The user opens the printed link; the consent page shows the code.
		link := regexp.MustCompile(`https://app\.example/oauth/authorize\?\S+`).FindString(prompt.String())
		if q := api.consentLink(t, link); q.Get("redirect_uri") != oobRedirectURI || q.Has("state") {
			t.Fatalf("unexpected --no-browser link %s", link)
		}
		return "code-2\n"
	})
	if err != nil || key != "upt_good" || api.exchanges != 1 {
		t.Fatalf("manualLogin = %q, %v (exchanges %d)", key, err, api.exchanges)
	}
}

func TestManualLoginAcceptsAPastedKey(t *testing.T) {
	key, err := manualLogin("https://app.example", "http://127.0.0.1:1", io.Discard, func() string { return "upt_pasted\n" })
	if err != nil || key != "upt_pasted" {
		t.Fatalf("manualLogin = %q, %v", key, err)
	}
}
