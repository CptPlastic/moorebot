package botcfg

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// telemetryPort is where the scout-telemetry daemon listens on the robot
// (installed by soft-hack/install-telemetry.py).
const telemetryPort = "8088"

var telemetryHTTP = &http.Client{Timeout: 4 * time.Second}

type telemetryPayload struct {
	Battery struct {
		OK        bool   `json:"ok"`
		Percent   int    `json:"percent"`
		Status    string `json:"status"`
		VoltageMV int    `json:"voltageMV"`
	} `json:"battery"`
	Wifi struct {
		OK    bool   `json:"ok"`
		RSSI  int    `json:"rssi"`
		Iface string `json:"iface"`
		SSID  string `json:"ssid"`
	} `json:"wifi"`
	TempMilliC []int `json:"tempMilliC"`
}

// ReadTelemetry polls the robot's telemetry HTTP endpoint. This is the
// preferred path: one cheap GET instead of an SSH session per sample.
// tempC is the hottest SoC thermal zone in °C (0 when unknown).
func (c Client) ReadTelemetry() (w Wifi, b Battery, tempC int, err error) {
	host := c.Host
	if h, _, splitErr := net.SplitHostPort(host); splitErr == nil {
		host = h
	}
	url := fmt.Sprintf("http://%s/telemetry", net.JoinHostPort(host, telemetryPort))
	resp, err := telemetryHTTP.Get(url)
	if err != nil {
		return Wifi{}, Battery{}, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Wifi{}, Battery{}, 0, fmt.Errorf("telemetry: HTTP %d", resp.StatusCode)
	}
	var p telemetryPayload
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return Wifi{}, Battery{}, 0, fmt.Errorf("telemetry: %w", err)
	}

	w = Wifi{OK: p.Wifi.OK, RSSI: p.Wifi.RSSI, Iface: p.Wifi.Iface, SSID: p.Wifi.SSID}
	if w.OK {
		w = finalizeWifi(w)
	} else {
		w.Error = "no wifi stats"
	}
	b = Battery{
		OK:        p.Battery.OK,
		Percent:   p.Battery.Percent,
		Status:    strings.TrimSpace(p.Battery.Status),
		VoltageMV: p.Battery.VoltageMV,
	}
	// The 18800 zone is a board sensor that never moves; the SoC zones are
	// the hot ones, so report the max.
	for _, mc := range p.TempMilliC {
		if c := mc / 1000; c > tempC {
			tempC = c
		}
	}
	return w, b, tempC, nil
}
