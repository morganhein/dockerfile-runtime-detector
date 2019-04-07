#!/usr/bin/env bash
echo "Current PID: $$"
ps aux | grep $$

echo "Launching secondary process"
./second-launch.sh