#!/bin/sh
set -eu

package_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
install_prefix=${HOME:?}/.local
include_secret_store=false
while [ "$#" -gt 0 ]; do
  case "$1" in
    --prefix) install_prefix=${2:?provide an installation prefix}; shift 2 ;;
    --with-secret-store) include_secret_store=true; shift ;;
    *) echo 'Usage: install.sh [--prefix DIRECTORY] [--with-secret-store]' >&2; exit 2 ;;
  esac
done
case "$install_prefix" in /*) ;; *) echo 'Installation prefix must be absolute.' >&2; exit 2 ;; esac
case "$(uname -s)" in Darwin) install_os=darwin ;; Linux) install_os=linux ;; *) exit 2 ;; esac
case "$(uname -m)" in arm64|aarch64) install_arch=arm64 ;; x86_64|amd64) install_arch=amd64 ;; *) exit 2 ;; esac
[ "$(cat "$package_dir/TARGET")" = "$install_os/$install_arch" ] || { echo 'Package does not match this host.' >&2; exit 1; }
cd "$package_dir"
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum --check SHA256SUMS >/dev/null
elif command -v shasum >/dev/null 2>&1; then
  shasum -a 256 --check SHA256SUMS >/dev/null
else
  echo 'A SHA-256 verification utility is required.' >&2; exit 1
fi
mkdir -p "$install_prefix/bin"
for install_binary in chunsu chunsu-secret-store; do
  if [ "$install_binary" = chunsu-secret-store ] && [ "$include_secret_store" != true ]; then continue; fi
  install_target=$install_prefix/bin/$install_binary
  [ ! -L "$install_target" ] || { echo 'Existing symbolic link preserved; select another prefix.' >&2; exit 1; }
  install_staging=$(mktemp "$install_prefix/bin/.chunsu-install.XXXXXX")
  trap 'rm -f "$install_staging"' EXIT HUP INT TERM
  cp "$package_dir/$install_binary" "$install_staging"
  chmod 755 "$install_staging"
  mv -f "$install_staging" "$install_target"
  trap - EXIT HUP INT TERM
done
printf 'Installed in %s/bin. Existing data and service registrations were preserved.\n' "$install_prefix"
