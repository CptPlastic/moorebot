"""One-time installer: put telemetry_httpd.py on the Scout as a systemd service.

After this runs, the control center reads battery/Wi-Fi via plain HTTP
(GET http://scout:8088/telemetry) and never needs SSH for routine polling.
"""
import os
import sys
import io
import paramiko

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")

HOST = os.environ.get("SCOUT_HOST", "192.168.4.33")
USER = "linaro"
PASS = "linaro"

UNIT = """\
[Unit]
Description=Scout telemetry HTTP endpoint (battery/wifi JSON on :8088)
After=network.target

[Service]
ExecStart=/usr/bin/env python /var/roller_eye/telemetry_httpd.py
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
"""


def main():
    here = os.path.dirname(os.path.abspath(__file__))
    with open(os.path.join(here, "telemetry_httpd.py"), "r") as f:
        daemon_src = f.read()

    c = paramiko.SSHClient()
    c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    c.connect(HOST, username=USER, password=PASS, timeout=15,
              allow_agent=False, look_for_keys=False)

    sftp = c.open_sftp()
    with sftp.file("/tmp/telemetry_httpd.py", "w") as f:
        f.write(daemon_src)
    with sftp.file("/tmp/scout-telemetry.service", "w") as f:
        f.write(UNIT)
    sftp.close()

    steps = [
        ("install files",
         "mv /tmp/telemetry_httpd.py /var/roller_eye/telemetry_httpd.py && "
         "chmod 644 /var/roller_eye/telemetry_httpd.py && "
         "mv /tmp/scout-telemetry.service /etc/systemd/system/scout-telemetry.service"),
        ("enable + start",
         "systemctl daemon-reload && systemctl enable scout-telemetry --now"),
        ("verify service", "sleep 2; systemctl is-active scout-telemetry"),
        ("verify endpoint",
         "curl -s --max-time 5 http://127.0.0.1:8088/telemetry"),
    ]
    ok = True
    for label, cmd in steps:
        _, out, err = c.exec_command("sudo -n bash -c %r" % cmd, timeout=30)
        code = out.channel.recv_exit_status()
        body = out.read().decode("utf-8", "replace").strip()
        errs = err.read().decode("utf-8", "replace").strip()
        print("== %s (exit %d)" % (label, code))
        if body:
            print(body)
        if errs:
            print("[stderr]", errs[:300])
        if code != 0:
            ok = False
            break
    c.close()
    print("INSTALL " + ("OK" if ok else "FAILED"))


if __name__ == "__main__":
    main()
