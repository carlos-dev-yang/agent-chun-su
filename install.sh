#!/bin/sh
# Build and install a local Linux release candidate. This script is intended to
# be run from a trusted clone as the target OS user, never with sudo.
set -eu

source_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
prefix=${HOME:?}/.local
chunsu_home=${CHUNSU_HOME:-${XDG_CONFIG_HOME:-$HOME/.config}/chunsu}
install_deps=false
prepare_codex=false
prepare_sandbox=false
enable_linger=false
start=false
model=
skip_login=false
skip_telegram=false

usage() {
  cat <<'EOF'
Usage: ./install.sh [--install-deps] [--enable-linger] [--prepare-codex] [--prepare-sandbox] [--model MODEL] [--prefix DIRECTORY] [--chunsu-home DIRECTORY] [--start] [--skip-login] [--skip-telegram]

Builds this checked-out source into a checksummed local package and installs it
for the current user. --install-deps authorizes Ubuntu package installation.
--prepare-codex installs the verified platform artifact below /usr/local/lib
with sudo. Existing routes and explicit stopped service intent are preserved.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --install-deps) install_deps=true ;;
    --prepare-codex) prepare_codex=true ;;
    --prepare-sandbox) prepare_sandbox=true ;;
    --enable-linger) enable_linger=true ;;
    --start) start=true ;;
    --skip-login) skip_login=true ;;
    --skip-telegram) skip_telegram=true ;;
    --model) model=${2:?provide a model}; shift ;;
    --prefix) prefix=${2:?provide a prefix}; shift ;;
    --chunsu-home) chunsu_home=${2:?provide a data home}; shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
  shift
done

case "$prefix" in /*) ;; *) echo 'Installation prefix must be absolute.' >&2; exit 2;; esac
case "$chunsu_home" in /*) ;; *) echo 'CHUNSU_HOME must be absolute.' >&2; exit 2;; esac
[ "$(uname -s)" = Linux ] || { echo 'This source bootstrap currently supports Linux/Ubuntu only.' >&2; exit 2; }
[ "$(id -u)" -ne 0 ] || { echo 'Run this installer as the intended non-root OS user.' >&2; exit 2; }
. /etc/os-release 2>/dev/null || true
[ "${ID:-}" = ubuntu ] || { echo 'This bootstrap supports Ubuntu. Build a package for other Linux distributions instead.' >&2; exit 2; }

if [ "$install_deps" = true ]; then
  "$source_dir/scripts/server-setup.sh" --install-deps
fi
if [ "$enable_linger" = true ]; then "$source_dir/scripts/server-setup.sh" --enable-linger; fi
command -v curl >/dev/null 2>&1 || { echo 'curl is required; rerun with --install-deps on Ubuntu.' >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo 'python3 is required for package creation; rerun with --install-deps on Ubuntu.' >&2; exit 1; }
command -v tar >/dev/null 2>&1 || { echo 'tar is required; rerun with --install-deps on Ubuntu.' >&2; exit 1; }

go_version=$(sed -n 's/^toolchain go\([0-9][0-9.]*\)$/\1/p' "$source_dir/go.mod")
[ -n "$go_version" ] || go_version=$(sed -n 's/^go \([0-9][0-9.]*\)$/\1/p' "$source_dir/go.mod")
case "$(uname -m)" in x86_64|amd64) go_arch=amd64;; aarch64|arm64) go_arch=arm64;; *) echo 'Unsupported Linux architecture.' >&2; exit 2;; esac
if command -v go >/dev/null 2>&1 && [ "$(go env GOVERSION)" = "go$go_version" ]; then
  go_bin=$(command -v go)
else
  cache_root=${XDG_CACHE_HOME:-$HOME/.cache}/chunsu/bootstrap
  go_root=$cache_root/go$go_version-linux-$go_arch
  go_bin=$go_root/bin/go
  if [ ! -x "$go_bin" ]; then
    mkdir -p "$cache_root"
    archive=$cache_root/go$go_version.linux-$go_arch.tar.gz
    go_file=go$go_version.linux-$go_arch.tar.gz
    manifest=$(curl --fail --location --proto '=https' --tlsv1.2 'https://go.dev/dl/?mode=json&include=all')
    expected_sha256=$(printf '%s' "$manifest" | python3 -c '
import json, sys
name = sys.argv[1]
for release in json.load(sys.stdin):
    for item in release.get("files", []):
        if item.get("filename") == name and item.get("sha256"):
            print(item["sha256"])
            raise SystemExit(0)
raise SystemExit("Go release manifest has no SHA-256 for " + name)
' "$go_file")
    case "$expected_sha256" in *[!0123456789abcdef]*) echo 'Go release manifest returned an invalid SHA-256.' >&2; exit 1;; esac
    [ "${#expected_sha256}" -eq 64 ] || { echo 'Go release manifest returned an invalid SHA-256.' >&2; exit 1; }
    if [ -f "$archive" ] && ! (cd "$cache_root" && printf '%s  %s\n' "$expected_sha256" "$(basename "$archive")" | sha256sum --check --status); then
      unlink "$archive"
    fi
    if [ ! -f "$archive" ]; then
      curl --fail --location --proto '=https' --tlsv1.2 -o "$archive" "https://go.dev/dl/$go_file"
    fi
    (cd "$cache_root" && printf '%s  %s\n' "$expected_sha256" "$(basename "$archive")" | sha256sum --check --status)
    staging=$(mktemp -d "$cache_root/.go.XXXXXX")
    trap 'rm -rf "$staging"' EXIT HUP INT TERM
    tar -xzf "$archive" -C "$staging"
    mv "$staging"/go "$go_root"
    trap - EXIT HUP INT TERM
  fi
fi

export PATH=$(dirname "$go_bin"):$PATH
export CHUNSU_HOME=$chunsu_home
version=local-$(git -C "$source_dir" rev-parse --short=12 HEAD)
build_root=$(mktemp -d "${TMPDIR:-/tmp}/chunsu-package.XXXXXX")
trap 'rm -rf "$build_root"' EXIT HUP INT TERM
python3 "$source_dir/scripts/package.py" --version "$version" --target "linux/$go_arch" --output "$build_root"
package_dir=$build_root/chunsu-$version-linux-$go_arch
"$package_dir/install.sh" --prefix "$prefix" --with-secret-store
bin=$prefix/bin/chunsu
if config_show=$("$bin" --json config show 2>/dev/null); then :; else
  "$bin" --json setup
  config_show=$("$bin" --json config show)
fi
has_route=$(printf '%s' "$config_show" | python3 -c 'import json,sys; print("yes" if json.load(sys.stdin).get("executor", {}).get("kind") else "no")')

metadata=$("$bin" --json config models)
if [ "$has_route" = no ] && [ -n "$model" ]; then
  selected_model=$model
elif [ "$has_route" = no ]; then
  printf '%s\n' "$metadata" | python3 -c '
import json, sys
data=json.load(sys.stdin)
for model in data.get("models", []):
 print("%s (%s)" % (model["id"], model.get("status", "available")))
'
  default_model=$(printf '%s' "$metadata" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("default_model", ""))')
  printf 'Select a supported model [%s]: ' "$default_model" >&2
  IFS= read -r selected_model
  [ -n "$selected_model" ] || selected_model=$default_model
fi
if [ "$has_route" = no ]; then
  [ -n "${selected_model:-}" ] || { echo 'A model is required; no default was selected.' >&2; exit 2; }
  printf '%s' "$metadata" | python3 -c '
import json, sys
value=sys.argv[1]
data=json.load(sys.stdin)
if value not in {m["id"] for m in data.get("models", [])}:
 raise SystemExit("Selected model is not in the supported model metadata.")
' "$selected_model"
fi

codex_version=$(printf '%s' "$metadata" | python3 -c "import json,sys; print(json.load(sys.stdin)['version'].removeprefix('codex-cli '))")
if [ "$prepare_codex" = true ]; then
  "$source_dir/scripts/server-setup.sh" --prepare-codex --version "$codex_version"
fi
if [ "$prepare_sandbox" = true ]; then
  "$source_dir/scripts/server-setup.sh" --prepare-sandbox --version "$codex_version"
fi
codex_path=/usr/local/lib/chunsu/codex-cli-$codex_version/bin/codex
if [ "$has_route" = no ] && [ ! -x "$codex_path" ]; then
  echo "Verified Codex is not prepared at $codex_path. Rerun with --prepare-codex." >&2
  exit 1
fi

if [ "$has_route" = no ]; then
  "$bin" config route task --driver codex --path "$codex_path" --model "$selected_model" --environment native-restricted
else
  codex_path=$(printf '%s' "$config_show" | python3 -c 'import json,sys; print(json.load(sys.stdin)["executor"]["path"])')
  printf '%s\n' 'An existing task route was preserved; its model and executable were not changed.'
fi

"$codex_path" sandbox -P chunsu_install_check -c 'permissions.chunsu_install_check.filesystem={ ":minimal" = "read" }' -c 'permissions.chunsu_install_check.network.enabled=false' -- /usr/bin/true || { echo 'Codex native sandbox probe failed. Review Ubuntu AppArmor policy and rerun with --prepare-sandbox if this host approves the executable-specific profile.' >&2; exit 1; }

if [ "$skip_login" = false ] && ! "$codex_path" login status >/dev/null 2>&1; then "$codex_path" login --device-auth; fi
telegram_status=$("$bin" --json telegram status)
telegram_paired=$(printf '%s' "$telegram_status" | python3 -c 'import json,sys; d=json.load(sys.stdin); print("yes" if d.get("configured") and d.get("paired") else "no")')
if [ "$skip_telegram" = true ]; then
  printf '%s\n' 'Telegram setup was skipped by request.'
elif [ "$telegram_paired" = no ]; then
  printf '%s\n' 'Telegram setup will securely prompt for the first bot token, then pair and register the user service.'
  "$bin" telegram enable
elif [ "$start" = true ]; then
  "$bin" telegram start
else
  printf '%s\n' 'Existing Telegram service intent was preserved. Use --start only when you want to start it.'
fi
if [ "$start" = true ]; then
  "$bin" controller start
  "$bin" worker start
fi
doctor=$("$bin" --json doctor)
printf '%s' "$doctor" | python3 -c '
import json, sys
report=json.load(sys.stdin)
route=report.get("execution_routes", {}).get("task", {})
if route.get("status") != "prerequisites_match":
 raise SystemExit("Task route did not pass the installed-driver prerequisite check: " + route.get("status", "missing"))
'
if [ "$telegram_paired" = no ] && [ "$skip_telegram" = false ]; then "$bin" --json telegram status; fi
printf '%s\n' "Installed for $USER. Run ./scripts/check-recovery.sh from this checkout or ./check-recovery.sh from the package."
[ "$chunsu_home" = "${XDG_CONFIG_HOME:-$HOME/.config}/chunsu" ] || printf '%s\n' "For this custom data home, run later commands with CHUNSU_HOME=$chunsu_home."
