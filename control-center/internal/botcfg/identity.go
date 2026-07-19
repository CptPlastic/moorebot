package botcfg

import (
	"fmt"
	"regexp"
	"strings"
)

var macRe = regexp.MustCompile(`(?i)^([0-9a-f]{2}:){5}[0-9a-f]{2}$`)

// ScoutNameFromMAC returns SCOUT-{last6} from a MAC address.
func ScoutNameFromMAC(mac string) string {
	mac = strings.TrimSpace(strings.ToLower(mac))
	if !macRe.MatchString(mac) {
		return ""
	}
	return "SCOUT-" + strings.ToUpper(strings.ReplaceAll(mac, ":", "")[6:])
}

// ReadIdentity returns current hostname + wlan MAC / proposed SCOUT name.
func (c Client) ReadIdentity() (hostname, mac, scoutName string, err error) {
	out, err := c.run("hostname; cat /sys/class/net/wlan0/address 2>/dev/null; cat /var/roller_eye/config/scout_hostname 2>/dev/null")
	if err != nil {
		return "", "", "", err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	if len(lines) > 0 {
		hostname = lines[0]
	}
	for _, ln := range lines[1:] {
		if macRe.MatchString(ln) {
			mac = strings.ToLower(ln)
			break
		}
	}
	scoutName = ScoutNameFromMAC(mac)
	if scoutName == "" && strings.HasPrefix(hostname, "SCOUT-") {
		scoutName = hostname
	}
	return hostname, mac, scoutName, nil
}

func FormatIdentity(hostname, mac, scoutName string) string {
	if scoutName == "" {
		scoutName = "?"
	}
	return fmt.Sprintf("%s (mac %s, host %s)", scoutName, mac, hostname)
}
