import paramiko, sys, io
sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")
c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect("192.168.4.33", username="linaro", password="linaro", timeout=15, allow_agent=False, look_for_keys=False)
cmds = [
    "cat /opt/ros/melodic/share/roller_eye/msg/status.msg; echo '---'; find /opt/ros/melodic -name 'status.msg' 2>/dev/null",
    "source /opt/ros/melodic/setup.sh; rosmsg show roller_eye/status; timeout 4 rostopic echo -n 1 /SensorNode/simple_battery_status",
    "cat /opt/ros/melodic/share/roller_eye/srv/adjust_ligth.srv; echo '---'; cat /opt/ros/melodic/share/roller_eye/srv/night_get.srv; echo '---'; cat /opt/ros/melodic/share/roller_eye/srv/nav_low_bat.srv; echo '---'; cat /opt/ros/melodic/share/roller_eye/srv/led_all_on.srv",
    "grep -r 'track' /var/roller_eye/config /opt/ros/melodic/share/roller_eye/param 2>/dev/null | head -30",
    "strings /opt/ros/melodic/lib/roller_eye/motor_node | grep -iE 'track|mecanum|wheel' | head -30",
]
for cmd in cmds:
    print("====")
    _, o, e = c.exec_command(f"sudo -n bash -lc {repr(cmd)}", timeout=25)
    print(o.read().decode("utf-8", "replace"))
c.close()
