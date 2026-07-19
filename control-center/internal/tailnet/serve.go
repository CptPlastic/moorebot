package tailnet

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var httpsURLRe = regexp.MustCompile(`https://[a-zA-Z0-9._-]+\.ts\.net/?`)

// EnableServe proxies localHTTPPort on Tailscale Serve (Tailnet HTTPS only, not Funnel).
func EnableServe(localHTTPPort string) (httpsURL string, err error) {
	port := strings.TrimPrefix(localHTTPPort, ":")
	if port == "" {
		port = "8787"
	}
	out, err := run("serve", "--bg", "--yes", port)
	if err != nil {
		return "", fmt.Errorf("tailscale serve: %w (%s)", err, strings.TrimSpace(out))
	}
	if u := firstHTTPS(out); u != "" {
		return u, nil
	}
	return StatusURL()
}

// StatusURL returns the current Serve HTTPS origin, or "" if Serve is off / CLI missing.
func StatusURL() (string, error) {
	out, err := run("serve", "status")
	if err != nil {
		return "", err
	}
	return firstHTTPS(out), nil
}

func Disable() error {
	_, err := run("serve", "reset")
	return err
}

func Available() bool {
	_, err := exec.LookPath("tailscale")
	return err == nil
}

func run(args ...string) (string, error) {
	cmd := exec.Command("tailscale", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func firstHTTPS(s string) string {
	m := httpsURLRe.FindString(s)
	return strings.TrimRight(m, "/")
}
