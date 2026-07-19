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

	scoutHost, rosPort, err := net.SplitHostPort(*rosMaster)
	if err != nil {
		log.Fatalf("invalid ROS master %q: %v", *rosMaster, err)
	}
	localHost := *host
	if localHost == "" {
		localHost, err = guessLANIP("192.168.4.1")
		if err != nil {
			localHost = "127.0.0.1"
		}
	}
	client := ros.NewOfflineClient()

	port := strings.TrimPrefix(*listen, ":")
	if port == "" {
		port = "8787"
	}
	localURL := "http://127.0.0.1:" + port
	settingsURL := localURL + "/settings.html"
	viewURL := localURL + "/view.html"

	// Keep SSH on the hostname so each new request follows DHCP changes.
	bot := &botcfg.Client{Host: scoutHost, User: *sshUser, Password: *sshPass}
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
	go connectScout(srv, bot, *rosMaster, *host, rosPort)

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
		client := srv.CurrentROS()
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

type scoutConnection struct {
	client    *ros.Client
	scoutHost string
	scoutIP   string
	localHost string
}

func connectScout(server *api.Server, bot *botcfg.Client, master, configuredLocalHost, rosPort string) {
	for {
		connection := waitForScout(master, configuredLocalHost)
		client := connection.client
		server.SetROS(client)
		client.Log("info", "Scout connected")

		stopNight := make(chan struct{})
		go refreshNightUntilStopped(client, stopNight)
		go configureConnectedScout(client, bot)

		reason := waitForMasterChange(connection.scoutHost, rosPort, connection.scoutIP)
		close(stopNight)
		log.Printf("%s; reconnecting ROS", reason)
		server.SetROS(ros.NewOfflineClient())
	}
}

func refreshNightUntilStopped(client *ros.Client, stop <-chan struct{}) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	client.RefreshNight()
	for {
		select {
		case <-ticker.C:
			client.RefreshNight()
		case <-stop:
			return
		}
	}
}

func configureConnectedScout(client *ros.Client, bot *botcfg.Client) {
	hostname, mac, scoutName, err := bot.ReadIdentity()
	if err == nil {
		log.Printf("scout identity: %s", botcfg.FormatIdentity(hostname, mac, scoutName))
	} else {
		log.Printf("warn: scout identity: %v", err)
	}
	if motion, err := bot.ReadMotion(); err == nil {
		client.SetTracked(motion.Track == 1)
		log.Printf("drive base: track=%v (1=tracked 0=mecanum)", motion.Track == 1)
	} else {
		log.Printf("warn: could not read motion config: %v", err)
	}
}

func waitForScout(master, configuredLocalHost string) scoutConnection {
	for {
		resolvedMaster, scoutHost, scoutIP, err := resolveScoutMaster(master)
		if err != nil {
			log.Printf("Scout discovery: %v; retrying", err)
			time.Sleep(5 * time.Second)
			continue
		}

		localHost := configuredLocalHost
		if localHost == "" {
			localHost, err = guessLANIP(scoutIP)
			if err != nil {
				log.Printf("local network discovery: %v; retrying", err)
				time.Sleep(5 * time.Second)
				continue
			}
		}

		log.Printf("Scout %s resolved to %s", scoutHost, scoutIP)
		log.Printf("connecting ROS master %s (advertise host %s)", resolvedMaster, localHost)
		client, err := ros.Connect(ros.Config{
			MasterAddress: resolvedMaster,
			Host:          localHost,
			NodeName:      "moorebot_control_center",
		})
		if err == nil {
			return scoutConnection{client: client, scoutHost: scoutHost, scoutIP: scoutIP, localHost: localHost}
		}
		log.Printf("ROS connection: %v; retrying", err)
		time.Sleep(5 * time.Second)
	}
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
	cacheScoutIP(ip)
	return net.JoinHostPort(ip, port), hostname, ip, nil
}

func cacheScoutIP(ip string) {
	_ = os.MkdirAll(filepath.Dir(scoutIPCachePath()), 0o755)
	_ = os.WriteFile(scoutIPCachePath(), []byte(ip+"\n"), 0o644)
}

func scoutIPCachePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "MoorebotScout", "last-scout-ip")
}

func waitForMasterChange(host, port, currentIP string) string {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	failures := 0
	for range ticker.C {
		for _, ip := range lookupScoutIPv4(host) {
			if ip != currentIP && rosMasterHealthy(ip, port) {
				cacheScoutIP(ip)
				return fmt.Sprintf("Scout moved from %s to %s", currentIP, ip)
			}
		}
		if rosMasterHealthy(currentIP, port) {
			failures = 0
			continue
		}
		failures++
		if failures >= 3 {
			return fmt.Sprintf("ROS master %s:%s is unavailable", currentIP, port)
		}
	}
	return "ROS monitor stopped"
}

func lookupScoutIPv4(host string) []string {
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return []string{v4.String()}
		}
		return nil
	}

	seen := make(map[string]struct{})
	var out []string
	names := []string{host}
	if !strings.Contains(host, ".") {
		names = append(names, host+".local")
	}
	for _, name := range names {
		out = appendUniqueIPv4(out, seen, name)
	}
	return out
}

func appendUniqueIPv4(out []string, seen map[string]struct{}, host string) []string {
	addrs, err := net.LookupIP(host)
	if err != nil {
		return out
	}
	for _, addr := range addrs {
		v4 := addr.To4()
		if v4 == nil {
			continue
		}
		ip := v4.String()
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		out = append(out, ip)
	}
	return out
}

func rosMasterHealthy(ip, port string) bool {
	const body = `<?xml version="1.0"?><methodCall><methodName>getUri</methodName><params><param><value><string>/moorebot_watchdog</string></value></param></params></methodCall>`
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+net.JoinHostPort(ip, port)+"/RPC2", strings.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "text/xml")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	return res.StatusCode == http.StatusOK
}
