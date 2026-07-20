"""One-shot diagnostic: why does the Scout keep shutting down?

Single SSH session, lightweight read-only commands only.
"""
import paramiko

HOST = "192.168.4.33"
USER = "linaro"
PASS = "linaro"

CMDS = [
    ("uptime", "uptime"),
    ("temperature", "cat /sys/class/thermal/thermal_zone*/temp 2>/dev/null"),
    ("battery sysfs", "grep -r . /sys/class/power_supply/*/capacity /sys/class/power_supply/*/status /sys/class/power_supply/*/voltage_now 2>/dev/null"),
    ("last boots", "last -x reboot shutdown 2>/dev/null | head -15"),
    ("prev kernel log tail", "tail -40 /var/log/kern.log 2>/dev/null || dmesg | tail -40"),
    ("syslog before last boot", "grep -iE 'shut|power|thermal|critical|batt' /var/log/syslog 2>/dev/null | tail -25"),
    ("load + top procs", "cat /proc/loadavg; ps aux --sort=-%cpu 2>/dev/null | head -8"),
]

def main():
    c = paramiko.SSHClient()
    c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    c.connect(HOST, username=USER, password=PASS, timeout=10, banner_timeout=10)
    try:
        for label, cmd in CMDS:
            print(f"===== {label} =====")
            _, out, err = c.exec_command(cmd, timeout=15)
            print(out.read().decode(errors="replace").strip())
            e = err.read().decode(errors="replace").strip()
            if e:
                print(f"[stderr] {e}")
            print()
    finally:
        c.close()

if __name__ == "__main__":
    main()
