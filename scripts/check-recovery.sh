#!/bin/sh
# Read-only recovery readiness check. --exercise only restarts healthy managed services.
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec python3 "$script_dir/check_recovery.py" "$@"
