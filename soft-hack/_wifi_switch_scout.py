"""Move the Scout onto the SCOUT SSID, keeping BEEFSTICK as fallback.

- Backs up both config files first.
- Updates /var/roller_eye/config/wifi (Moorebot's own record).
- Adds SCOUT via wpa_cli with higher priority + save_config (persists in
  /etc/wpa_supplicant/wpa_supplicant.conf; BEEFSTICK entry stays enabled).
- Runs under nohup on the robot so the hop completes after SSH drops.
"""
import io
import sys
import paramiko

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")

SCRIPT = """#!/bin/bash
set -x
cp /var/roller_eye/config/wifi /var/roller_eye/config/wifi.bak.pre-scout
cp /etc/wpa_supplicant/wpa_supplicant.conf /etc/wpa_supplicant/wpa_supplicant.conf.bak.pre-scout
printf 'SCOUT\\nIlovemywife!\\n' > /var/roller_eye/config/wifi
ID=$(wpa_cli -i wlan0 add_network | tail -1)
wpa_cli -i wlan0 set_network "$ID" ssid '"SCOUT"'
wpa_cli -i wlan0 set_network "$ID" psk '"Ilovemywife!"'
wpa_cli -i wlan0 set_network "$ID" priority 20
wpa_cli -i wlan0 set_network "$ID" scan_ssid 1
wpa_cli -i wlan0 enable_network "$ID"
wpa_cli -i wlan0 save_config
wpa_cli -i wlan0 reassociate
sleep 12
wpa_cli -i wlan0 status
"""

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect("192.168.4.33", username="linaro", password="linaro", timeout=15,
          allow_agent=False, look_for_keys=False)

sftp = c.open_sftp()
with sftp.file("/tmp/wifi_switch.sh", "w") as f:
    f.write(SCRIPT)
sftp.close()

_, o, e = c.exec_command(
    "sudo -n bash -c 'nohup bash /tmp/wifi_switch.sh >/tmp/wifi_switch.log 2>&1 &'",
    timeout=15)
o.channel.recv_exit_status()
print("switch launched on robot (log: /tmp/wifi_switch.log)")
c.close()
