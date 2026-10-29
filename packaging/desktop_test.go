package packaging

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestDesktopRuntime(t *testing.T) {
	binary := os.Getenv("GOBOOK_SMOKE_BINARY")
	if binary == "" {
		t.Skip("set GOBOOK_SMOKE_BINARY to extracted executable")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dir := t.TempDir()
	state := filepath.Join(dir, "private state")
	cmd := exec.CommandContext(ctx, binary, "-desktop", "-open=false", "-addr", "127.0.0.1:0", "-desktop-dir", state)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	defer func() {
		cancel()
		select {
		case <-waited:
		case <-time.After(5 * time.Second):
			t.Error("desktop host did not stop")
		}
	}()
	var address string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(filepath.Join(state, "desktop.url"))
		if err == nil {
			address = string(data)
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	u, err := url.Parse(address)
	if err != nil || u.Hostname() != "127.0.0.1" || len(u.Query().Get("token")) != 64 {
		t.Fatal("desktop did not publish private loopback state")
	}
	request := func(method, path, token string) int {
		req, _ := http.NewRequestWithContext(ctx, method, "http://"+u.Host+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		return response.StatusCode
	}
	if status := request("GET", "/api/desktop/status", "wrong"); status != 401 {
		t.Fatal("unauthenticated desktop status accepted")
	}
	token := u.Query().Get("token")
	if status := request("GET", "/api/desktop/status", token); status != 200 {
		t.Fatal("desktop status unavailable")
	}
	// A second app launch must return successfully without replacing the worker.
	second := exec.CommandContext(ctx, binary, "-desktop", "-open=false", "-addr", u.Host, "-desktop-dir", state)
	second.Dir = dir
	if output, err := second.CombinedOutput(); err != nil {
		t.Fatalf("cannot reopen desktop: %v %s", err, output)
	}
	after, err := os.ReadFile(filepath.Join(state, "desktop.url"))
	if err != nil || string(after) != address {
		t.Fatal("reopen replaced live host")
	}
	if status := request("GET", "/api/desktop/status", token); status != 200 {
		t.Fatal("reopen killed host")
	}

	// Quit must cancel an active fresh run rather than orphaning its process.
	marker := filepath.Join(dir, "running")
	code := fmt.Sprintf(`if err:=os.WriteFile(%q,[]byte("ready"),0600);err!=nil{panic(err)};time.Sleep(time.Minute)`, marker)
	body, _ := json.Marshal(map[string]any{"version": 1, "imports": []string{"os", "time"}, "cells": []map[string]string{{"code": code}}})
	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		req, _ := http.NewRequestWithContext(ctx, "POST", "http://"+u.Host+"/api/run", strings.NewReader(string(body)))
		req.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(req)
		if err == nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
	}()
	deadline = time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("notebook did not reach running state")
		}
		time.Sleep(25 * time.Millisecond)
	}
	quitStarted := time.Now()
	if status := request("POST", "/api/desktop/quit", token); status != 202 {
		t.Fatal("desktop quit rejected")
	}
	select {
	case err := <-waited:
		waited <- err
		if err != nil {
			t.Fatalf("host did not quit cleanly: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("desktop did not quit")
	}
	if time.Since(quitStarted) > 4*time.Second {
		t.Fatal("quit waited for graceful timeout instead of cancelling execution")
	}
	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("quit left an active execution request")
	}
	if data, err := os.ReadFile(filepath.Join(state, "desktop.url")); err != nil || string(data) != address {
		t.Fatal("last private record changed unexpectedly during quit")
	}
}
