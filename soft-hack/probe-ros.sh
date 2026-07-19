#!/bin/sh
# Run ON the Scout after SSH login.
# Usage: sh probe-ros.sh

set -eu

echo "=== host ==="
hostname 2>/dev/null || true
uname -a 2>/dev/null || true
echo

echo "=== looking for ROS setup ==="
for f in /opt/ros/*/setup.sh /home/*/catkin_ws/devel/setup.sh /root/*/devel/setup.sh; do
  if [ -f "$f" ]; then
    echo "sourcing $f"
    # shellcheck disable=SC1090
    . "$f"
  fi
done
echo

echo "=== ROS env ==="
echo "ROS_MASTER_URI=${ROS_MASTER_URI:-unset}"
echo "ROS_IP=${ROS_IP:-unset}"
echo "ROS_HOSTNAME=${ROS_HOSTNAME:-unset}"
echo

if command -v rostopic >/dev/null 2>&1; then
  echo "=== rostopic list ==="
  rostopic list || true
  echo
  echo "=== interesting topics ==="
  rostopic list 2>/dev/null | grep -E 'CoreNode|SensorNode|h264|aac|imu|tof|cmd|vel|battery' || true
  echo
  echo "=== h264 topic info (if present) ==="
  if rostopic list 2>/dev/null | grep -q '/CoreNode/h264'; then
    rostopic info /CoreNode/h264 || true
    echo "(sample one message header only — may take a second)"
    timeout 5 rostopic echo -n 1 /CoreNode/h264 2>/dev/null | head -n 40 || true
  else
    echo "/CoreNode/h264 not found yet"
  fi
else
  echo "rostopic not in PATH"
  echo "Try: find / -name rostopic 2>/dev/null | head"
fi

echo
echo "=== listening ports (ssh/ros hints) ==="
(netstat -lnt 2>/dev/null || ss -lnt 2>/dev/null || true) | head -n 40

echo
echo "Done. If /CoreNode/h264 exists, soft-hack stage 1 is green."
