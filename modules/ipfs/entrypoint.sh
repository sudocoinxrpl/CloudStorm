#!/bin/bash
set -euo pipefail

# 1) Launch Trinity in the background
/opt/CloudStorm/trinity_arm.sh &

# 2) Hand off to the IPFS‐specific bootstrap (this will exec into `ipfs daemon`)
exec /opt/CloudStorm/bootstrap.sh
