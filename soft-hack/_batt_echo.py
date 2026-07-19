import paramiko, sys, io
sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")
c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect("192.168.4.33", username="linaro", password="linaro", timeout=15, allow_agent=False, look_for_keys=False)
script = r'''
import rospy
from roller_eye.msg import status
rospy.init_node("batt_echo", anonymous=True)
m = rospy.wait_for_message("/SensorNode/simple_battery_status", status, timeout=5)
print("LEN", len(m.status))
print("STATUS", list(m.status))
'''
sftp = c.open_sftp()
with sftp.file("/tmp/batt_echo.py", "w") as f:
    f.write(script)
sftp.close()
_, o, e = c.exec_command("sudo -n bash -lc 'source /opt/ros/melodic/setup.sh; timeout 10 python /tmp/batt_echo.py'", timeout=20)
print(o.read().decode())
print(e.read().decode()[:500])
# also search battery percent in sensor node strings
_, o, e = c.exec_command("sudo -n bash -lc 'strings /opt/ros/melodic/lib/roller_eye/sensors_node | grep -iE \"battery|percent|voltage|charge\" | head -40'", timeout=15)
print(o.read().decode())
c.close()
