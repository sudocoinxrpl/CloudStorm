#!/bin/bash
set -euo pipefail

# 1) Launch Trinity in the background
/opt/CloudStorm/trinity_arm.sh &

# 2) Hand off to the Symbiote‐specific bootstrap (this ends by tailing the Clio log)
/opt/CloudStorm/bootstrap.sh
