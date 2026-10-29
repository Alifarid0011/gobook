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
	"os/exec"
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
	launch := flag.Bool("open", false, "open the local workspace in your default browser")
	desktop := flag.Bool("desktop", false, "start or reopen the local desktop host")
	desktopDir := flag.String("desktop-dir", "", "private desktop state directory (defaults to user config/gobook)")
	flag.Parse()
	if *desktop {
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "public-origin" && *external != "" {
				log.Fatal("desktop mode is local only")
			}
		})
		explicitAddr, explicitOpen := false, false
		flag.Visit(func(f *flag.Flag) {
			explicitAddr = explicitAddr || f.Name == "addr"
			explicitOpen = explicitOpen || f.Name == "open"
		})
		if !explicitAddr {
			*addr = desktopAddress
		}
		if !explicitOpen {
			*launch = true
		}
		var e error
		*desktopDir, e = desktopStateDir(*desktopDir)
		if e != nil {
			log.Fatal(e)
		}
		cache, e := os.UserCacheDir()
		if e != nil {
			log.Fatal(e)
		}
		if e = configureDesktopTemp(cache); e != nil {
			log.Fatal(e)
		}
		if _, e = exec.LookPath("go"); e != nil {
			log.Fatal("Go compiler not found. Install Go 1.25+ from https://go.dev/dl/ before running GoBook Local.")
		}
	}
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
		if *desktop {
			if e := reopenDesktop(*desktopDir, *addr, *launch); e != nil {
				log.Fatal(e)
			}
			return
		}
		log.Fatal(err)
	}
	if *desktop {
		address := fmt.Sprintf("http://%s/?token=%s", listener.Addr().String(), hex.EncodeToString(token))
		if err := saveDesktopURL(*desktopDir, address); err != nil {
			listener.Close()
			log.Fatal(err)
		}
	}
	s := &Server{Token: hex.EncodeToString(token), Host: listener.Addr().String(), Kernel: Kernel{Chartframe: *chart}, PublicOrigin: origin}
	s.Native = &NativeManager{Kernel: s.Kernel}
	defer s.Native.Stop()
	s.Debugger = &DebugManager{Kernel: s.Kernel, Delve: *delve}
	defer s.Debugger.Stop()
	fmt.Printf("GoBook — trusted local Go execution (full user permissions)\nOpen http://%s/?token=%s\n", s.Host, s.Token)
	if *launch {
		go func() {
			if err := openBrowser(fmt.Sprintf("http://%s/?token=%s", s.Host, s.Token)); err != nil {
				log.Printf("Browser launch: %v", err)
			}
		}()
	}
	if origin != "" {
		fmt.Printf("Proxy capability URL: %s/?token=%s\n", origin, s.Token)
	}
	server := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 100 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	shutdown, cancelShutdown := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelShutdown()
	server.BaseContext = func(net.Listener) context.Context { return shutdown }
	if *desktop {
		s.DesktopQuit = cancelShutdown
	}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-shutdown.Done()
		_ = s.Debugger.Stop()
		_ = s.Native.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			_ = server.Close()
		}
	}()
	if err = server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	cancelShutdown()
	<-shutdownDone
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
