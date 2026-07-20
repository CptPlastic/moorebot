"""Compare ROS battery topic vs kernel sysfs battery, one SSH session."""
import paramiko, sys, io
sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect("192.168.4.33", username="linaro", password="linaro", timeout=15, allow_agent=False, look_for_keys=False)

script = r'''
import rospy
from roller_eye.msg import status
rospy.init_node("batt_cmp", anonymous=True)
m = rospy.wait_for_message("/SensorNode/simple_battery_status", status, timeout=6)
print("ROS status array:", list(m.status))
'''
sftp = c.open_sftp()
with sftp.file("/tmp/batt_cmp.py", "w") as f:
    f.write(script)
sftp.close()

cmd = (
    "source /opt/ros/melodic/setup.sh; timeout 12 python /tmp/batt_cmp.py; "
    "echo '--- sysfs ---'; "
    "grep -r . /sys/class/power_supply/*/capacity /sys/class/power_supply/*/status "
    "/sys/class/power_supply/*/voltage_now /sys/class/power_supply/*/current_now 2>/dev/null"
)
_, o, e = c.exec_command(f"sudo -n bash -lc {cmd!r}", timeout=30)
print(o.read().decode("utf-8", "replace"))
err = e.read().decode("utf-8", "replace").strip()
if err:
    print("[stderr]", err[:400])
c.close()
