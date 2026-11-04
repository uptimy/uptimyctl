// Package selfupdate replaces the running uptimyctl with the latest GitHub
// release. It installs what scripts/install.sh installs: the GoReleaser
// archive for this platform, checked against the release's checksums.txt.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const repo = "uptimy/uptimyctl"

// maxDownload bounds an archive or checksums download.
const maxDownload = 200 << 20

type Updater struct {
	APIURL string // GitHub API base
	HTTP   *http.Client
	GOOS   string
	GOARCH string
}

func New() *Updater {
	return &Updater{
		APIURL: "https://api.github.com",
		HTTP:   &http.Client{Timeout: 2 * time.Minute},
		GOOS:   runtime.GOOS,
		GOARCH: runtime.GOARCH,
	}
}

// Release is a published uptimyctl release.
type Release struct {
	Version string // without the leading "v", as `uptimyctl version` prints it
	URL     string // release page
	assets  map[string]string
}

// PermissionError means the binary's directory isn't writable, e.g.
// /usr/local/bin without sudo.
type PermissionError struct {
	Dir string
}

func (e *PermissionError) Error() string {
	return "no permission to write to " + e.Dir
}

// Latest returns the latest published release.
func (u *Updater) Latest(ctx context.Context) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.APIURL+"/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "uptimyctl")
	resp, err := u.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("look up the latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("look up the latest release: GitHub answered %s", resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("read the latest release: %w", err)
	}
	if body.TagName == "" {
		return nil, errors.New("read the latest release: no tag")
	}
	r := &Release{Version: strings.TrimPrefix(body.TagName, "v"), URL: body.HTMLURL, assets: map[string]string{}}
	for _, a := range body.Assets {
		r.assets[a.Name] = a.URL
	}
	return r, nil
}

// Install downloads r's archive for this platform, checks its checksum and
// replaces the binary at exePath with the one inside.
func (u *Updater) Install(ctx context.Context, r *Release, exePath string) error {
	name := u.archiveName(r.Version)
	archiveURL, ok := r.assets[name]
	if !ok {
		return fmt.Errorf("release %s has no build for %s/%s", r.Version, u.GOOS, u.GOARCH)
	}
	sumsURL, ok := r.assets["checksums.txt"]
	if !ok {
		return fmt.Errorf("release %s has no checksums.txt", r.Version)
	}

	archive, err := u.download(ctx, archiveURL)
	if err != nil {
		return err
	}
	sums, err := u.download(ctx, sumsURL)
	if err != nil {
		return err
	}
	want, ok := checksumFor(sums, name)
	if !ok {
		return fmt.Errorf("checksums.txt has no entry for %s", name)
	}
	sum := sha256.Sum256(archive)
	if got := hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("checksum mismatch for %s (want %s, got %s): the download is corrupt or was tampered with", name, want, got)
	}

	bin, err := u.extract(archive)
	if err != nil {
		return fmt.Errorf("unpack %s: %w", name, err)
	}
	return u.replace(exePath, bin)
}

// archiveName follows .goreleaser.yaml: tar.gz, zip on Windows.
func (u *Updater) archiveName(version string) string {
	ext := "tar.gz"
	if u.GOOS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("uptimyctl_%s_%s_%s.%s", version, u.GOOS, u.GOARCH, ext)
}

func (u *Updater) binaryName() string {
	if u.GOOS == "windows" {
		return "uptimyctl.exe"
	}
	return "uptimyctl"
}

func (u *Updater) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "uptimyctl")
	resp, err := u.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	if len(b) > maxDownload {
		return nil, fmt.Errorf("download %s: larger than %d MB", url, maxDownload>>20)
	}
	return b, nil
}

// checksumFor finds name in a checksums.txt ("<sha256>  <file>" per line).
func checksumFor(sums []byte, name string) (string, bool) {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && fields[1] == name {
			return fields[0], true
		}
	}
	return "", false
}

func (u *Updater) extract(archive []byte) ([]byte, error) {
	bin := u.binaryName()
	if u.GOOS == "windows" {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if filepath.Base(f.Name) == bin && !f.FileInfo().IsDir() {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer func() { _ = rc.Close() }()
				return io.ReadAll(io.LimitReader(rc, maxDownload))
			}
		}
		return nil, fmt.Errorf("no %s inside", bin)
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("no %s inside", bin)
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == bin {
			return io.ReadAll(io.LimitReader(tr, maxDownload))
		}
	}
}

// replace writes bin next to exePath and renames it over, so the swap is
// atomic and a failed download never leaves a broken binary. Windows can't
// replace a running executable, but it can rename it: the old one is moved to
// <exe>.old first.
func (u *Updater) replace(exePath string, bin []byte) (err error) {
	dir := filepath.Dir(exePath)
	tmp, err := os.CreateTemp(dir, ".uptimyctl-update-*")
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return &PermissionError{Dir: dir}
		}
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(bin); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	if u.GOOS == "windows" {
		old := exePath + ".old"
		_ = os.Remove(old)
		if err = os.Rename(exePath, old); err != nil {
			return err
		}
		if err = os.Rename(tmp.Name(), exePath); err != nil {
			_ = os.Rename(old, exePath)
			return err
		}
		return nil
	}
	if err = os.Rename(tmp.Name(), exePath); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return &PermissionError{Dir: dir}
		}
		return err
	}
	return nil
}
