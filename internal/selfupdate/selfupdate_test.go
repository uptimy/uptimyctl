package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// fakeGitHub serves a latest release (tag v<version>) whose assets are the
// given files, plus checksums.txt over them unless sums overrides it.
func fakeGitHub(t *testing.T, version string, files map[string][]byte, sums string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/uptimy/uptimyctl/releases/latest" {
			type asset struct {
				Name string `json:"name"`
				URL  string `json:"browser_download_url"`
			}
			assets := []asset{{"checksums.txt", srv.URL + "/dl/checksums.txt"}}
			for name := range files {
				assets = append(assets, asset{name, srv.URL + "/dl/" + name})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v" + version,
				"html_url": "https://github.com/uptimy/uptimyctl/releases/tag/v" + version,
				"assets":   assets,
			})
			return
		}
		name := filepath.Base(r.URL.Path)
		if name == "checksums.txt" {
			if sums == "" {
				for n, b := range files {
					s := sha256.Sum256(b)
					sums += fmt.Sprintf("%s  %s\n", hex.EncodeToString(s[:]), n)
				}
			}
			_, _ = w.Write([]byte(sums))
			return
		}
		if b, ok := files[name]; ok {
			_, _ = w.Write(b)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func tarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range []struct {
		name string
		body []byte
	}{{"README.md", []byte("readme")}, {name, content}} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o755, Size: int64(len(f.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(f.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zipped(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func updater(srv *httptest.Server, goos, goarch string) *Updater {
	return &Updater{APIURL: srv.URL, HTTP: srv.Client(), GOOS: goos, GOARCH: goarch}
}

// installedBinary is an "old" uptimyctl in its own directory.
func installedBinary(t *testing.T, name string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func assertOnlyFiles(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("files in %s: %v, want %v", dir, got, want)
	}
}

func TestLatest(t *testing.T) {
	srv := fakeGitHub(t, "1.8.0", nil, "")
	r, err := updater(srv, "linux", "amd64").Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "1.8.0" || r.URL != "https://github.com/uptimy/uptimyctl/releases/tag/v1.8.0" {
		t.Fatalf("got %+v", r)
	}
}

func TestInstallReplacesTheBinary(t *testing.T) {
	srv := fakeGitHub(t, "1.8.0", map[string][]byte{
		"uptimyctl_1.8.0_darwin_arm64.tar.gz": tarGz(t, "uptimyctl", []byte("new")),
		"uptimyctl_1.8.0_linux_amd64.tar.gz":  tarGz(t, "uptimyctl", []byte("wrong platform")),
	}, "")
	u := updater(srv, "darwin", "arm64")
	r, err := u.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	exe := installedBinary(t, "uptimyctl")
	if err := u.Install(context.Background(), r, exe); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(exe)
	info, _ := os.Stat(exe)
	if string(b) != "new" || info.Mode().Perm() != 0o755 {
		t.Fatalf("binary %q, mode %v", b, info.Mode().Perm())
	}
	assertOnlyFiles(t, filepath.Dir(exe), "uptimyctl")
}

func TestInstallOnWindowsMovesTheOldBinaryAside(t *testing.T) {
	srv := fakeGitHub(t, "1.8.0", map[string][]byte{
		"uptimyctl_1.8.0_windows_amd64.zip": zipped(t, "uptimyctl.exe", []byte("new")),
	}, "")
	u := updater(srv, "windows", "amd64")
	r, err := u.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	exe := installedBinary(t, "uptimyctl.exe")
	if err := u.Install(context.Background(), r, exe); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new" {
		t.Fatalf("binary %q", b)
	}
	if b, _ := os.ReadFile(exe + ".old"); string(b) != "old" {
		t.Fatalf("old binary %q", b)
	}
}

func TestInstallRefusesABadChecksum(t *testing.T) {
	name := "uptimyctl_1.8.0_linux_amd64.tar.gz"
	srv := fakeGitHub(t, "1.8.0", map[string][]byte{
		name: tarGz(t, "uptimyctl", []byte("tampered")),
	}, "0000000000000000000000000000000000000000000000000000000000000000  "+name+"\n")
	u := updater(srv, "linux", "amd64")
	r, _ := u.Latest(context.Background())
	exe := installedBinary(t, "uptimyctl")
	if err := u.Install(context.Background(), r, exe); err == nil {
		t.Fatal("want a checksum error")
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Fatalf("binary changed to %q", b)
	}
	assertOnlyFiles(t, filepath.Dir(exe), "uptimyctl")
}

func TestInstallWithoutABuildForThePlatform(t *testing.T) {
	srv := fakeGitHub(t, "1.8.0", map[string][]byte{
		"uptimyctl_1.8.0_linux_amd64.tar.gz": tarGz(t, "uptimyctl", []byte("new")),
	}, "")
	u := updater(srv, "freebsd", "amd64")
	r, _ := u.Latest(context.Background())
	if err := u.Install(context.Background(), r, installedBinary(t, "uptimyctl")); err == nil {
		t.Fatal("want an error")
	}
}

func TestInstallIntoAReadOnlyDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	srv := fakeGitHub(t, "1.8.0", map[string][]byte{
		"uptimyctl_1.8.0_linux_amd64.tar.gz": tarGz(t, "uptimyctl", []byte("new")),
	}, "")
	u := updater(srv, "linux", "amd64")
	r, _ := u.Latest(context.Background())
	exe := installedBinary(t, "uptimyctl")
	dir := filepath.Dir(exe)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := u.Install(context.Background(), r, exe)
	var perm *PermissionError
	if !errors.As(err, &perm) || perm.Dir != dir {
		t.Fatalf("want a PermissionError for %s, got %v", dir, err)
	}
}
