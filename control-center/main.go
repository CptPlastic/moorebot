package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/cptplastic/moorebot/control-center/internal/api"
	"github.com/cptplastic/moorebot/control-center/internal/botcfg"
	"github.com/cptplastic/moorebot/control-center/internal/ros"
	"github.com/cptplastic/moorebot/control-center/internal/tailnet"
)

//go:embed web/*
var webFS embed.FS

//go:embed assets/icon.ico
var iconICO []byte

func main() {
	rosMaster := flag.String("ros", "SCOUT-F9C3D0:11311", "Scout hostname/IP and ROS master port")
	host := flag.String("host", "", "This PC LAN IP (auto-detect if empty)")
	listen := flag.String("listen", ":8787", "HTTP listen address")
	sshUser := flag.String("ssh-user", "linaro", "Scout SSH user")
	sshPass := flag.String("ssh-pass", "linaro", "Scout SSH password")
	tray := flag.Bool("tray", true, "run in system tray")
	openBrowser := flag.Bool("open", true, "open control UI in browser on start")
	useTailscale := flag.Bool("tailscale", true, "enable Tailscale Serve HTTPS (Tailnet only)")
	console := flag.Bool("console", false, "attach a console for logs (useful with windowsgui builds)")
	flag.Parse()

	if *console {
		attachConsole()
	}
	setupLogging(*console)

	resolvedMaster, scoutHost, scoutIP, err := resolveScoutMaster(*rosMaster)
	if err != nil {
		log.Fatalf("could not resolve Scout %q: %v", *rosMaster, err)
	}

	localHost := *host
	if localHost == "" {
		localHost, err = guessLANIP(scoutIP)
		if err != nil {
			log.Fatalf("could not guess LAN IP (pass -host): %v", err)
		}
	}

	port := strings.TrimPrefix(*listen, ":")
	if port == "" {
		port = "8787"
	}
	localURL := "http://127.0.0.1:" + port
	settingsURL := localURL + "/settings.html"
	viewURL := localURL + "/view.html"

	log.Printf("Scout %s resolved to %s", scoutHost, scoutIP)
	log.Printf("connecting ROS master %s (advertise host %s)", resolvedMaster, localHost)
	client, err := ros.Connect(ros.Config{
		MasterAddress: resolvedMaster,
		Host:          localHost,
		NodeName:      "moorebot_control_center",
	})
	if err != nil {
		log.Fatal(err)
	}

	// Keep SSH on the hostname so each new request follows DHCP changes.
	bot := &botcfg.Client{Host: scoutHost, User: *sshUser, Password: *sshPass}
	hostname, mac, scoutName, idErr := bot.ReadIdentity()
	if idErr == nil {
		log.Printf("scout identity: %s", botcfg.FormatIdentity(hostname, mac, scoutName))
	} else {
		log.Printf("warn: scout identity: %v", idErr)
	}
	if m, err := bot.ReadMotion(); err == nil {
		client.SetTracked(m.Track == 1)
		log.Printf("drive base: track=%v (1=tracked 0=mecanum)", m.Track == 1)
	} else {
		log.Printf("warn: could not read motion config: %v", err)
	}
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		client.RefreshNight()
		for range t.C {
			client.RefreshNight()
		}
	}()

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}

	wifiCache := botcfg.NewWifiCache(*bot)
	// RSSI has no native ROS topic on Scout; sample it sparingly over SSH.
	wifiCache.Start(10 * time.Second)

	srv := &api.Server{ROS: client, BotSSH: bot, Wifi: wifiCache, UI: api.NewUIStore(), Static: http.FS(sub)}
	httpServer := &http.Server{Addr: *listen, Handler: srv.Handler()}

	go func() {
		log.Printf("local:     %s/  (WebCodecs OK on localhost)", localURL)
		log.Printf("settings:  %s", settingsURL)
		log.Printf("pop-out:   %s", viewURL)
		log.Printf("LAN:       http://%s:%s/", localHost, port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	// Wait briefly for listen before opening browser / Tailscale.
	time.Sleep(300 * time.Millisecond)

	var tsURL string
	if *useTailscale {
		if !tailnet.Available() {
			log.Printf("tailscale CLI not found — skipping Serve HTTPS")
		} else if u, err := tailnet.EnableServe(port); err != nil {
			log.Printf("tailscale serve: %v", err)
		} else {
			tsURL = u
			log.Printf("tailnet:   %s/  (HTTPS / phone-safe)", tsURL)
		}
	}

	if *openBrowser {
		openURL(localURL + "/")
	}

	shutdown := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = client.Stop()
		_ = httpServer.Shutdown(ctx)
		client.Close()
		log.Printf("stopped")
	}

	if *tray {
		log.Printf("running in system tray — Quit from the Scout icon")
		runTray(localURL+"/", settingsURL, viewURL, func() string {
			if tsURL != "" {
				return tsURL
			}
			u, _ := tailnet.StatusURL()
			tsURL = u
			return tsURL
		}, shutdown)
		return
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
	shutdown()
	fmt.Println("bye")
}

func setupLogging(console bool) {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "MoorebotScout")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "control-center.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	if console || fileIsConsole(os.Stderr) {
		log.SetOutput(io.MultiWriter(os.Stderr, f))
	} else {
		log.SetOutput(f)
	}
	log.Printf("--- start --- log=%s", path)
}

func fileIsConsole(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func guessLANIP(peer string) (string, error) {
	conn, err := net.Dial("udp", net.JoinHostPort(peer, "11311"))
	if err != nil {
		return "", err
	}
	defer conn.Close()
	host, _, err := net.SplitHostPort(conn.LocalAddr().String())
	return host, err
}

func resolveScoutMaster(master string) (resolved, hostname, ip string, err error) {
	hostname, port, err := net.SplitHostPort(master)
	if err != nil {
		return "", "", "", fmt.Errorf("expected host:port: %w", err)
	}
	// LLMNR/mDNS can take a moment after the robot or Wi-Fi wakes up.
	for attempt := 0; attempt < 6 && ip == ""; attempt++ {
		addrs, lookupErr := net.LookupIP(hostname)
		if lookupErr == nil {
			for _, addr := range addrs {
				if v4 := addr.To4(); v4 != nil {
					ip = v4.String()
					break
				}
			}
		}
		if ip == "" && attempt < 5 {
			time.Sleep(time.Second)
		}
	}
	if ip == "" {
		// Last-known address keeps startup working during a transient resolver
		// outage. A successful hostname lookup always replaces this cache.
		if b, readErr := os.ReadFile(scoutIPCachePath()); readErr == nil {
			cached := strings.TrimSpace(string(b))
			if net.ParseIP(cached) != nil {
				ip = cached
			}
		}
	}
	if ip == "" {
		return "", "", "", fmt.Errorf("hostname did not resolve and no cached address exists")
	}
	_ = os.MkdirAll(filepath.Dir(scoutIPCachePath()), 0o755)
	_ = os.WriteFile(scoutIPCachePath(), []byte(ip+"\n"), 0o644)
	return net.JoinHostPort(ip, port), hostname, ip, nil
}

func scoutIPCachePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "MoorebotScout", "last-scout-ip")
}
