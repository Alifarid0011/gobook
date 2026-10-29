package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "IPv4 loopback address; port 0 picks a free port")
	chart := flag.String("chartframe", "", "local chartframe module (defaults to adjacent ../chartframe)")
	external := flag.String("public-origin", "", "optional HTTPS reverse proxy origin, for example https://book.example")
	delve := flag.String("delve", "dlv", "Delve executable for optional interactive debugging")
	flag.Parse()
	origin, err := publicOrigin(*external)
	if err != nil {
		log.Fatal(err)
	}
	host, _, err := net.SplitHostPort(*addr)
	if err != nil || host != "127.0.0.1" {
		log.Fatal("-addr must use 127.0.0.1 (for example 127.0.0.1:8080)")
	}
	if *chart == "" {
		if p, e := filepath.Abs("../chartframe"); e == nil {
			if _, e = os.Stat(filepath.Join(p, "go.mod")); e == nil {
				*chart = p
			}
		}
	}
	if *chart != "" {
		p, e := filepath.Abs(*chart)
		if e != nil {
			log.Fatal(e)
		}
		if _, e = os.Stat(filepath.Join(p, "go.mod")); e != nil {
			log.Fatal(e)
		}
		*chart = p
	}
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		log.Fatal(err)
	}
	listener, err := net.Listen("tcp4", *addr)
	if err != nil {
		log.Fatal(err)
	}
	s := &Server{Token: hex.EncodeToString(token), Host: listener.Addr().String(), Kernel: Kernel{Chartframe: *chart}, PublicOrigin: origin}
	s.Debugger = &DebugManager{Kernel: s.Kernel, Delve: *delve}
	defer s.Debugger.Stop()
	fmt.Printf("GoBook — trusted local Go execution (full user permissions)\nOpen http://%s/?token=%s\n", s.Host, s.Token)
	if origin != "" {
		fmt.Printf("Proxy capability URL: %s/?token=%s\n", origin, s.Token)
	}
	server := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 100 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	shutdown, cancelShutdown := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelShutdown()
	go func() { <-shutdown.Done(); _ = s.Debugger.Stop(); _ = server.Close() }()
	if err = server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func publicOrigin(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("public-origin: %w", err)
	}
	if u.Scheme != "https" || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return "", fmt.Errorf("public-origin must be an HTTPS origin without credentials, path, query, or fragment")
	}
	if u.Host != strings.ToLower(u.Host) {
		return "", fmt.Errorf("public-origin host must be lowercase")
	}
	if u.Port() == "443" {
		u.Host = strings.TrimSuffix(u.Host, ":443")
	}
	return "https://" + u.Host, nil
}
