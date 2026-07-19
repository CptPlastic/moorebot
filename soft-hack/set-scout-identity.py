#!/usr/bin/env python3
"""Set Scout hostname to SCOUT-{MAC} and make ROS advertise the Wi‑Fi IP."""
import base64
import io
import re
import sys
import time

import paramiko

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")

HOST = "192.168.4.33"
USER = "linaro"
PASSWORD = "linaro"

# Patches /usr/sbin/roller_eye-start on the robot (Python 2/3 safe).
PATCH_START = r'''
from __future__ import print_function
path = "/usr/sbin/roller_eye-start"
with open(path, "r") as f:
    text = f.read()
needle = "export ROS_HOSTNAME=$(hostname)\n"
inject = needle + """
# scout-identity: prefer numeric IP so LAN PCs don't need to resolve hostname
WLAN_IP=$(ip -4 -o addr show wlan0 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | head -1)
if [ -z "$WLAN_IP" ]; then
  WLAN_IP=$(ip -4 -o addr show scope global 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | head -1)
fi
if [ -n "$WLAN_IP" ]; then
  export ROS_IP=$WLAN_IP
  export ROS_HOSTNAME=$WLAN_IP
fi
"""
if "scout-identity: prefer numeric IP" in text:
    print("already patched")
elif needle not in text:
    raise SystemExit("needle not found in roller_eye-start")
else:
    import shutil
    shutil.copy2(path, path + ".bak.pre-scout-identity")
    text = text.replace(needle, inject, 1)
    with open(path, "w") as f:
        f.write(text)
    print("patched roller_eye-start")
'''

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect(HOST, username=USER, password=PASSWORD, timeout=15, allow_agent=False, look_for_keys=False)


def sudo(cmd, timeout=90):
    escaped = cmd.replace("'", "'\"'\"'")
    _, stdout, stderr = client.exec_command(f"sudo -n bash -lc '{escaped}'", timeout=timeout)
    out = stdout.read().decode("utf-8", "replace")
    err = stderr.read().decode("utf-8", "replace")
    code = stdout.channel.recv_exit_status()
    return code, out, err


code, mac, _ = sudo("cat /sys/class/net/wlan0/address")
mac = mac.strip().lower()
if not re.match(r"^([0-9a-f]{2}:){5}[0-9a-f]{2}$", mac):
    print("bad mac:", mac)
    sys.exit(1)

suffix = mac.replace(":", "")[-6:].upper()
name = f"SCOUT-{suffix}"
print(f"MAC={mac} → hostname={name}")

code, out, err = sudo(
    f"echo '{name}' > /etc/hostname && hostname '{name}' && "
    f"if grep -q '127.0.1.1' /etc/hosts; then "
    f"sed -i 's/^127\\.0\\.1\\.1.*/127.0.1.1\\t{name}/' /etc/hosts; "
    f"else echo -e '127.0.1.1\\t{name}' >> /etc/hosts; fi && "
    f"echo '{name}' > /var/roller_eye/config/scout_hostname && "
    f"echo OK $(hostname)"
)
print(out or err)
if code != 0:
    sys.exit(code)

# systemd-resolved can publish the hostname over mDNS/LLMNR without Avahi.
# This makes SCOUT-XXXXXX.local survive DHCP address changes.
code, out, err = sudo(
    "mkdir -p /etc/systemd/resolved.conf.d && "
    "printf '[Resolve]\\nMulticastDNS=yes\\nLLMNR=yes\\n' "
    "> /etc/systemd/resolved.conf.d/scout-mdns.conf && "
    "systemctl restart systemd-resolved"
)
print(out or err)
if code != 0:
    sys.exit(code)

b64 = base64.b64encode(PATCH_START.encode()).decode()
code, out, err = sudo(
    f"echo {b64} | base64 -d > /tmp/patch_roller_start.py && "
    f"python /tmp/patch_roller_start.py && chmod 755 /usr/sbin/roller_eye-start"
)
print(out or err)
if code != 0:
    sys.exit(code)

print("Restarting roller_eye (video/control will blip)…")
code, out, err = sudo("systemctl restart roller_eye")
print(out or err, "exit", code)

for i in range(1, 30):
    code, out, _ = sudo(
        "export ROS_LOG_DIR=/tmp; . /opt/ros/melodic/setup.bash; "
        "rosservice list 2>/dev/null | grep -c night_get || true"
    )
    if out.strip().isdigit() and int(out.strip()) > 0:
        print("ROS back after", i, "checks")
        break
    time.sleep(2)
else:
    print("warn: ROS not confirmed yet — give it a minute")

code, out, _ = sudo(
    "hostname; ip -4 -o addr show wlan0 | awk '{print $4}'; "
    "grep -n 'scout-identity\\|ROS_HOSTNAME\\|ROS_IP' /usr/sbin/roller_eye-start | head -20"
)
print(out)
print("DONE", name)
