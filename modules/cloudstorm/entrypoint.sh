#!/bin/bash
set -euo pipefail
/opt/CloudStorm/trinity_arm.sh 2>&1 &
TRINITY_PID=$!
/opt/CloudStorm/bootstrap.sh 2>&1 &
BOOT_PID=$!
wait -n $TRINITY_PID $BOOT_PID
exit 0
