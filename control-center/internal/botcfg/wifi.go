package botcfg

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Wifi is the Scout's association to its AP (range proxy).
type Wifi struct {
	OK      bool   `json:"ok"`
	RSSI    int    `json:"rssi"`    // dBm, typically -30..-90
	Quality int    `json:"quality"` // 0..100
	Bars    int    `json:"bars"`    // 0..4
	Iface   string `json:"iface,omitempty"`
	SSID    string `json:"ssid,omitempty"`
	Error   string `json:"error,omitempty"`
	AgeMs   int64  `json:"ageMs"`
}

var (
	reIwconfigLevel = regexp.MustCompile(`(?i)Signal level[=:](-?\d+)`)
	reIwconfigQual  = regexp.MustCompile(`(?i)Link Quality[=:](\d+)(?:/(\d+))?`)
	reIwconfigSSID  = regexp.MustCompile(`(?i)ESSID:"([^"]*)"`)
	reIwconfigIface = regexp.MustCompile(`(?m)^(\S+)\s+IEEE`)
	reIwLinkSignal  = regexp.MustCompile(`(?i)signal:\s*(-?\d+)\s*dBm`)
	reIwLinkSSID    = regexp.MustCompile(`(?i)SSID:\s*(.+)`)
)

// ReadWifi samples radio link quality on the robot (best-effort).
func (c Client) ReadWifi() Wifi {
	if out, err := c.run(`for i in wlan0 wlan1 wlp1s0 wlp0s20f3; do out=$(iw dev "$i" link 2>/dev/null) && echo "$out" && break; done`); err == nil {
		if w := parseIwLink(out); w.OK {
			return w
		}
	}
	if out, err := c.run(`iwconfig 2>/dev/null`); err == nil {
		if w := parseIwconfig(out); w.OK {
			return w
		}
	}
	if out, err := c.run(`cat /proc/net/wireless 2>/dev/null`); err == nil {
		if w := parseProcWireless(out); w.OK {
			return w
		}
	}
	return Wifi{OK: false, Error: "no wifi stats"}
}

func parseIwLink(out string) Wifi {
	m := reIwLinkSignal.FindStringSubmatch(out)
	if m == nil {
		return Wifi{}
	}
	rssi, _ := strconv.Atoi(m[1])
	w := Wifi{OK: true, RSSI: rssi, Iface: "wlan0"}
	if sm := reIwLinkSSID.FindStringSubmatch(out); sm != nil {
		w.SSID = strings.TrimSpace(sm[1])
	}
	return finalizeWifi(w)
}

func parseIwconfig(out string) Wifi {
	level := reIwconfigLevel.FindStringSubmatch(out)
	if level == nil {
		return Wifi{}
	}
	rssi, _ := strconv.Atoi(level[1])
	w := Wifi{OK: true, RSSI: rssi}
	if im := reIwconfigIface.FindStringSubmatch(out); im != nil {
		w.Iface = im[1]
	}
	if sm := reIwconfigSSID.FindStringSubmatch(out); sm != nil {
		w.SSID = sm[1]
	}
	return finalizeWifi(w)
}

func parseProcWireless(out string) Wifi {
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Inter") || strings.HasPrefix(line, "face") {
			continue
		}
		// wlan0: 0000 70. -40. -256 ...
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		iface := strings.TrimSuffix(fields[0], ":")
		link := strings.TrimSuffix(fields[2], ".")
		level := strings.TrimSuffix(fields[3], ".")
		rssi, err := strconv.Atoi(level)
		if err != nil {
			continue
		}
		// Some kernels report level as unsigned; normalize.
		if rssi > 0 {
			rssi = rssi - 256
		}
		q, _ := strconv.Atoi(link)
		if q > 100 {
			q = q * 100 / 70
		}
		if q < 0 {
			q = 0
		}
		if q > 100 {
			q = 100
		}
		_ = q
		return finalizeWifi(Wifi{OK: true, RSSI: rssi, Iface: iface})
	}
	return Wifi{}
}

func finalizeWifi(w Wifi) Wifi {
	if !w.OK {
		return w
	}
	w.Quality = rssiToQuality(w.RSSI)
	w.Bars = rssiToBars(w.RSSI)
	return w
}

func rssiToQuality(rssi int) int {
	// Map -30..-90 → 100..0
	if rssi >= -30 {
		return 100
	}
	if rssi <= -90 {
		return 0
	}
	return (rssi + 90) * 100 / 60
}

func rssiToBars(rssi int) int {
	switch {
	case rssi >= -55:
		return 4
	case rssi >= -65:
		return 3
	case rssi >= -75:
		return 2
	case rssi >= -85:
		return 1
	default:
		return 0
	}
}

// WifiCache polls the robot on a timer so /api/health stays fast.
type WifiCache struct {
	c    Client
	mu   sync.RWMutex
	last Wifi
	at   time.Time
}

func NewWifiCache(c Client) *WifiCache {
	return &WifiCache{c: c}
}

func (w *WifiCache) Start(every time.Duration) {
	if every < time.Second {
		every = 3 * time.Second
	}
	go func() {
		w.refresh()
		t := time.NewTicker(every)
		defer t.Stop()
		for range t.C {
			w.refresh()
		}
	}()
}

func (w *WifiCache) refresh() {
	got := w.c.ReadWifi()
	got.AgeMs = 0
	w.mu.Lock()
	w.last = got
	w.at = time.Now()
	w.mu.Unlock()
}

func (w *WifiCache) Get() Wifi {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := w.last
	if !w.at.IsZero() {
		out.AgeMs = time.Since(w.at).Milliseconds()
	}
	return out
}
