package botcfg

import (
	"strconv"
	"strings"
)

// Battery is read from the kernel fuel gauge (/sys/class/power_supply).
// The SensorNode ROS topic reports a naive voltage-based percentage that can
// run 40+ points optimistic; the kernel gauge is what actually triggers the
// BMS power cutoff, so it is the number worth showing.
type Battery struct {
	OK        bool   `json:"ok"`
	Percent   int    `json:"percent"`
	Status    string `json:"status"` // Charging / Discharging / Full
	VoltageMV int    `json:"voltageMV"`
	Error     string `json:"error,omitempty"`
}

// ReadBattery samples the kernel battery gauge over SSH (best-effort).
func (c Client) ReadBattery() Battery {
	out, err := c.run(
		`cat /sys/class/power_supply/rk-bat/capacity ` +
			`/sys/class/power_supply/rk-bat/status ` +
			`/sys/class/power_supply/rk-bat/voltage_now 2>/dev/null`)
	if err != nil {
		return Battery{OK: false, Error: err.Error()}
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return Battery{OK: false, Error: "no battery sysfs"}
	}
	pct, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return Battery{OK: false, Error: "bad capacity: " + lines[0]}
	}
	b := Battery{OK: true, Percent: pct, Status: strings.TrimSpace(lines[1])}
	if len(lines) >= 3 {
		if uv, err := strconv.Atoi(strings.TrimSpace(lines[2])); err == nil {
			b.VoltageMV = uv / 1000
		}
	}
	return b
}
