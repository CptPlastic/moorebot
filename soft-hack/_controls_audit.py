import paramiko, sys, io
sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")
c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect("192.168.4.33", username="linaro", password="linaro", timeout=15, allow_agent=False, look_for_keys=False)

cmds = [
    "source /opt/ros/melodic/setup.sh; rostopic type /SensorNode/simple_battery_status; rostopic info /SensorNode/simple_battery_status",
    "source /opt/ros/melodic/setup.sh; timeout 3 rostopic echo -n 2 /SensorNode/simple_battery_status 2>&1 | head -40",
    "source /opt/ros/melodic/setup.sh; rostopic list | grep -iE 'batt|light|night|home|motor|track|cmd|Switch|speaker'",
    "cat /var/roller_eye/config/motion; cat /var/roller_eye/config/motion_default; cat /var/roller_eye/config/motor_port",
    "source /opt/ros/melodic/setup.sh; rosservice list 2>/dev/null | grep -iE 'light|night|home|track|motor|batt|enable' | head -40",
    "ls /opt/ros/melodic/share/roller_eye/srv 2>/dev/null | head -60",
    "find /opt/ros/melodic/share/roller_eye -name '*track*' -o -name '*motor*' -o -name '*light*' 2>/dev/null | head -40",
]
for cmd in cmds:
    print("====", cmd[:70])
    _, o, e = c.exec_command(f"sudo -n bash -lc {repr(cmd)}", timeout=30)
    print(o.read().decode("utf-8", "replace"))
    err = e.read().decode("utf-8", "replace")
    if err.strip() and "mesg" not in err:
        print("ERR", err[:400])
c.close()
