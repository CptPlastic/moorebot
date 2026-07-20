"""One-shot: topic types + one sample for tof/imu/odom. Single SSH session."""
import io
import sys
import paramiko

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect("192.168.4.33", username="linaro", password="linaro", timeout=15,
          allow_agent=False, look_for_keys=False)

cmd = (
    "source /opt/ros/melodic/setup.sh; "
    "for t in /SensorNode/tof /SensorNode/imu /MotorNode/baselink_odom_relative; do "
    'echo "== $t"; rostopic type $t; done; '
    'echo "== tof sample"; timeout 5 rostopic echo -n 1 /SensorNode/tof; '
    'echo "== odom sample"; timeout 5 rostopic echo -n 1 /MotorNode/baselink_odom_relative | head -40'
)
_, o, e = c.exec_command('bash -lc "%s"' % cmd.replace('"', '\\"'), timeout=60)
print(o.read().decode("utf-8", "replace"))
err = e.read().decode("utf-8", "replace").strip()
if err:
    print("[stderr]", err[:400])
c.close()
