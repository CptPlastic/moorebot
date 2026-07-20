#!/usr/bin/env python
"""Tiny read-only telemetry endpoint for the Moorebot Scout.

Serves GET /telemetry as JSON: battery (kernel fuel gauge), Wi-Fi RSSI,
SoC temperatures, and load. Stdlib only, Python 2/3 compatible, near-zero
CPU cost. Installed as the scout-telemetry systemd service so the control
center can poll plain HTTP instead of opening SSH sessions.
"""
import glob
import json
import subprocess
import time

try:
    from http.server import BaseHTTPRequestHandler, HTTPServer
except ImportError:  # Python 2
    from BaseHTTPServer import BaseHTTPRequestHandler, HTTPServer

PORT = 8088
BATT = "/sys/class/power_supply/rk-bat/"


def read(path):
    try:
        with open(path) as f:
            return f.read().strip()
    except Exception:
        return None


def battery():
    cap = read(BATT + "capacity")
    if cap is None:
        return {"ok": False}
    out = {"ok": True, "percent": int(cap), "status": read(BATT + "status") or ""}
    uv = read(BATT + "voltage_now")
    if uv:
        out["voltageMV"] = int(uv) // 1000
    return out


_ssid_cache = {"t": 0.0, "v": ""}


def ssid(iface):
    # SSID changes rarely; shelling out to iw every 30s is plenty.
    now = time.time()
    if now - _ssid_cache["t"] < 30:
        return _ssid_cache["v"]
    v = ""
    try:
        raw = subprocess.check_output(["iw", "dev", iface, "link"])
        for ln in raw.decode("utf-8", "replace").splitlines():
            ln = ln.strip()
            if ln.startswith("SSID:"):
                v = ln[5:].strip()
                break
    except Exception:
        pass
    _ssid_cache["t"] = now
    _ssid_cache["v"] = v
    return v


def wifi():
    raw = read("/proc/net/wireless") or ""
    for line in raw.splitlines():
        line = line.strip()
        if ":" not in line or line.startswith(("Inter", "face")):
            continue
        f = line.split()
        if len(f) < 4:
            continue
        try:
            rssi = int(float(f[3].rstrip(".")))
        except ValueError:
            continue
        if rssi > 0:  # some kernels report unsigned
            rssi -= 256
        iface = f[0].rstrip(":")
        return {"ok": True, "iface": iface, "rssi": rssi, "ssid": ssid(iface)}
    return {"ok": False}


def temps():
    out = []
    for p in sorted(glob.glob("/sys/class/thermal/thermal_zone*/temp")):
        v = read(p)
        try:
            out.append(int(v))
        except (TypeError, ValueError):
            pass
    return out


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path not in ("/", "/telemetry"):
            self.send_response(404)
            self.end_headers()
            return
        body = json.dumps({
            "battery": battery(),
            "wifi": wifi(),
            "tempMilliC": temps(),
            "load": read("/proc/loadavg"),
            "uptimeSec": float((read("/proc/uptime") or "0 0").split()[0]),
        }).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        pass  # stay silent; this runs forever on a tiny board


if __name__ == "__main__":
    HTTPServer(("0.0.0.0", PORT), Handler).serve_forever()
