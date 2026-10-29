package packaging

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPortableRuntime exercises the extracted release binary on its native OS.
func TestPortableRuntime(t *testing.T) {
	binary := os.Getenv("GOBOOK_SMOKE_BINARY")
	if binary == "" {
		t.Skip("set GOBOOK_SMOKE_BINARY to an extracted release executable")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-open=false", "-addr", "127.0.0.1:0")
	cmd.Dir = t.TempDir() // Do not accidentally depend on the source tree.
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	urls := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if line := scanner.Text(); strings.HasPrefix(line, "Open http://") {
				urls <- strings.TrimPrefix(line, "Open ")
				return
			}
		}
		urls <- ""
	}()
	var address string
	select {
	case address = <-urls:
	case <-ctx.Done():
		t.Fatal("host did not print its private URL")
	}
	u, err := url.Parse(address)
	if err != nil || u.Hostname() != "127.0.0.1" || len(u.Query().Get("token")) != 64 {
		t.Fatal("host did not print a private loopback capability URL")
	}
	client := &http.Client{Timeout: 70 * time.Second}
	var response *http.Response
	for attempt := 0; attempt < 50; attempt++ {
		response, err = client.Get(address)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(body), "GoBook") {
		t.Fatal("packaged host did not serve the workspace")
	}
	endpoint := "http://" + u.Host + "/api/run"
	response, err = client.Post(endpoint, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("notebook execution accepted a missing capability token")
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{"version":1,"imports":["fmt"],"cells":[{"code":"fmt.Println(6 * 7)"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+u.Query().Get("token"))
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result struct {
		Stdout string
		Error  string
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || result.Error != "" || strings.TrimSpace(result.Stdout) != "42" {
		t.Fatalf("packaged notebook execution failed: HTTP %d, result %+v", response.StatusCode, result)
	}
}
