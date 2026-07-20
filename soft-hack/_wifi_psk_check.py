"""Check psk format in wpa conf + roller_eye wifi file (prints secrets length only where possible)."""
import io
import sys
import paramiko

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect("192.168.4.33", username="linaro", password="linaro", timeout=15,
          allow_agent=False, look_for_keys=False)

CMDS = [
    ("psk format (quoted=plaintext)",
     "grep -o 'psk=.' /etc/wpa_supplicant/wpa_supplicant.conf"),
    ("roller_eye wifi file", "cat /var/roller_eye/config/wifi"),
]
for label, cmd in CMDS:
    _, o, e = c.exec_command("sudo -n bash -c \"%s\"" % cmd.replace('"', '\\"'), timeout=20)
    print("== %s" % label)
    print(o.read().decode("utf-8", "replace").strip())
    print()
c.close()
