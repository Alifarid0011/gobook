package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const desktopAddress = "127.0.0.1:43856"

var capabilityToken = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Validate private state before making any request or opening a browser.
func desktopCapability(address, host string) (*url.URL, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "http" || u.Host != host || u.Hostname() != "127.0.0.1" || u.User != nil || u.Path != "/" || u.Fragment != "" || !capabilityToken.MatchString(u.Query().Get("token")) || u.RawQuery != "token="+u.Query().Get("token") {
		return nil, errors.New("invalid local app capability")
	}
	return u, nil
}
func desktopStateDir(dir string) (string, error) {
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "gobook")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	if err = os.Chmod(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}
func saveDesktopURL(dir, address string) error {
	file, err := os.CreateTemp(dir, ".desktop-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = io.WriteString(file, address)
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), filepath.Join(dir, "desktop.url"))
}
func existingDesktopURL(dir, host string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "desktop.url"))
	if err != nil {
		return "", err
	}
	u, err := desktopCapability(string(data), host)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+u.Host+"/api/desktop/status", nil)
	req.Header.Set("Authorization", "Bearer "+u.Query().Get("token"))
	client := http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var state struct {
		App string `json:"app"`
	}
	if response.StatusCode != 200 {
		return "", errors.New("local app authentication failed")
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 1024)).Decode(&state); err != nil || state.App != "gobook" {
		return "", errors.New("port belongs to another application")
	}
	return u.String(), nil
}
func reopenDesktop(dir, host string, open bool) error {
	var err error
	// A simultaneous first click may still be publishing its private record.
	for attempt := 0; attempt < 20; attempt++ {
		var address string
		if address, err = existingDesktopURL(dir, host); err == nil {
			if open {
				return openBrowser(address)
			}
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("cannot reopen GoBook Local: %w; port %s is occupied. Close the other application or choose -addr 127.0.0.1:PORT", err, host)
}
func (s *Server) desktopHTTP(w http.ResponseWriter, r *http.Request) {
	if s.DesktopQuit == nil || r.Host != s.Host {
		http.NotFound(w, r)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.Token)) != 1 {
		http.Error(w, "unauthorized", 401)
		return
	}
	switch r.URL.Path {
	case "/api/desktop/status":
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "GET required", 405)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"app":"gobook","desktop":true}`)
	case "/api/desktop/quit":
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "POST required", 405)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		s.DesktopQuit()
	default:
		http.NotFound(w, r)
	}
}

// Keep compiler scratch files off potentially small or quota-limited /tmp mounts.
func configureDesktopTemp(cache string) error {
	dir := filepath.Join(cache, "gobook", "tmp")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return err
	}
	for _, name := range []string{"TMPDIR", "TMP", "TEMP", "GOTMPDIR"} {
		if err := os.Setenv(name, dir); err != nil {
			return err
		}
	}
	return nil
}
