"""One-shot Wi-Fi scan from the Scout: what APs does IT hear, and how loud."""
import io
import re
import sys
import paramiko

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect("192.168.4.33", username="linaro", password="linaro", timeout=15,
          allow_agent=False, look_for_keys=False)

_, o, e = c.exec_command(
    "sudo -n iw dev wlan0 scan 2>&1; echo ===LINK===; iw dev wlan0 link 2>&1",
    timeout=45)
raw = o.read().decode("utf-8", "replace")
c.close()

scan_raw, _, link_raw = raw.partition("===LINK===")

aps = []
cur = None
for line in scan_raw.splitlines():
    line = line.strip()
    m = re.match(r"BSS ([0-9a-f:]{17})", line)
    if m:
        cur = {"bssid": m.group(1), "ssid": "", "signal": None, "freq": None}
        aps.append(cur)
        continue
    if cur is None:
        continue
    if line.startswith("signal:"):
        try:
            cur["signal"] = float(line.split()[1])
        except (IndexError, ValueError):
            pass
    elif line.startswith("freq:"):
        try:
            cur["freq"] = int(float(line.split()[1]))
        except (IndexError, ValueError):
            pass
    elif line.startswith("SSID:"):
        cur["ssid"] = line[5:].strip()

current_bssid = ""
m = re.search(r"Connected to ([0-9a-f:]{17})", link_raw)
if m:
    current_bssid = m.group(1)

aps = [a for a in aps if a["signal"] is not None]
aps.sort(key=lambda a: a["signal"], reverse=True)
print("%-20s %-18s %6s %6s  %s" % ("SSID", "BSSID", "dBm", "MHz", ""))
for a in aps:
    mark = "  <-- CONNECTED HERE" if a["bssid"] == current_bssid else ""
    print("%-20s %-18s %6.0f %6s%s" % (
        a["ssid"] or "(hidden)", a["bssid"], a["signal"], a["freq"] or "?", mark))
if not aps:
    print("scan returned nothing (may need root or busy radio); raw head:")
    print(scan_raw[:600])
