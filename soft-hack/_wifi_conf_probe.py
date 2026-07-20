"""Read-only look at how the Scout manages Wi-Fi credentials."""
import io
import sys
import paramiko

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect("192.168.4.33", username="linaro", password="linaro", timeout=15,
          allow_agent=False, look_for_keys=False)

CMDS = [
    ("wpa processes", "ps aux | grep -i wpa | grep -v grep"),
    ("wpa conf files", "ls -la /etc/wpa_supplicant/ 2>/dev/null; ls -la /data/cfg 2>/dev/null"),
    ("wpa conf contents", "cat /etc/wpa_supplicant/*.conf /data/cfg/wpa_supplicant.conf 2>/dev/null | sed 's/psk=.*/psk=REDACTED/'"),
    ("roller_eye wifi cfg", "ls /var/roller_eye/config/ 2>/dev/null; grep -ril ssid /var/roller_eye/config 2>/dev/null"),
    ("wpa_cli status", "wpa_cli -i wlan0 status 2>&1 | head -12"),
    ("wpa_cli networks", "wpa_cli -i wlan0 list_networks 2>&1"),
]
for label, cmd in CMDS:
    _, o, e = c.exec_command("sudo -n bash -c \"%s\"" % cmd.replace('"', '\\"'), timeout=20)
    print("== %s" % label)
    out = o.read().decode("utf-8", "replace").strip()
    if out:
        print(out)
    err = e.read().decode("utf-8", "replace").strip()
    if err:
        print("[stderr]", err[:200])
    print()
c.close()
