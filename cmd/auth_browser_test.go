package cmd

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestBrowserLoginCallback(t *testing.T) {
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
		key, err := browserLogin("https://app.example")
		done <- result{key, err}
	}()

	authURL, err := url.Parse(<-opened)
	if err != nil {
		t.Fatal(err)
	}
	if authURL.Host != "app.example" || authURL.Path != "/cli/authorize" {
		t.Fatalf("unexpected authorize URL %s", authURL)
	}
	q := authURL.Query()
	port, state := q.Get("port"), q.Get("state")
	if port == "" || len(state) < 32 || q.Get("host") == "" {
		t.Fatalf("authorize URL missing params: %s", authURL)
	}
	callback := "http://127.0.0.1:" + port + "/callback"

	post := func(target, host string, form url.Values) int {
		req, _ := http.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if host != "" {
			req.Host = host
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if resp, err := http.Get(callback); err != nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET callback: want 404, got %v %v", resp, err)
	}
	if code := post(callback, "localhost:"+port, url.Values{"key": {"upt_x"}, "state": {state}}); code != http.StatusNotFound {
		t.Fatalf("foreign Host header: want 404, got %d", code)
	}
	if code := post(callback, "", url.Values{"key": {"upt_x"}, "state": {"wrong"}}); code != http.StatusBadRequest {
		t.Fatalf("wrong state: want 400, got %d", code)
	}
	if code := post(callback, "", url.Values{"key": {"nope"}, "state": {state}}); code != http.StatusBadRequest {
		t.Fatalf("bad key prefix: want 400, got %d", code)
	}
	if code := post(callback, "", url.Values{"key": {"upt_good"}, "state": {state}}); code != http.StatusOK {
		t.Fatalf("valid callback: want 200, got %d", code)
	}

	r := <-done
	if r.err != nil || r.key != "upt_good" {
		t.Fatalf("browserLogin = %q, %v", r.key, r.err)
	}
}

func TestManualLoginURLHasNoPort(t *testing.T) {
	u, _ := url.Parse(manualLoginURL("https://app.example/"))
	if u.Path != "/cli/authorize" || u.Query().Has("port") || u.Query().Has("state") {
		t.Fatalf("unexpected manual URL %s", u)
	}
}
