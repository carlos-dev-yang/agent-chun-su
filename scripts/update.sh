#!/bin/sh
set -eu

# This wrapper deliberately forwards only the fixed update subcommands. Source,
# repository, model, credentials, and service choices are configured locally.
if [ "$#" -gt 1 ]; then
  echo "Usage: update.sh [check|status|apply]" >&2
  exit 2
fi
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
update_command=chunsu
if [ -x "$script_dir/chunsu" ]; then update_command=$script_dir/chunsu; fi
case "${1:-status}" in
  check|status|apply) exec "$update_command" update "${1:-status}" ;;
  *) echo "Usage: update.sh [check|status|apply]" >&2; exit 2 ;;
esac
