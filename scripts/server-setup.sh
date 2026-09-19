#!/bin/sh
# Prepare Ubuntu dependencies and an integrity-checked Codex platform package.
set -eu

install_deps=false
prepare_codex=false
codex_version=
enable_linger=false
prepare_sandbox=false
defer_sandbox=false
usage() { echo 'Usage: scripts/server-setup.sh [--install-deps] [--enable-linger] [--prepare-codex --version VERSION] [--prepare-sandbox --version VERSION]'; }
while [ "$#" -gt 0 ]; do
  case "$1" in
    --install-deps) install_deps=true ;;
    --enable-linger) enable_linger=true ;;
    --prepare-codex) prepare_codex=true ;;
    --prepare-sandbox) prepare_sandbox=true ;;
    --version) codex_version=${2:?provide a Codex version}; shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
  shift
done
[ "$install_deps" = true ] || [ "$prepare_codex" = true ] || [ "$prepare_sandbox" = true ] || [ "$enable_linger" = true ] || { usage >&2; exit 2; }
[ "$(uname -s)" = Linux ] || { echo 'This helper supports Linux only.' >&2; exit 2; }
. /etc/os-release 2>/dev/null || true
[ "${ID:-}" = ubuntu ] || { echo 'This helper supports Ubuntu only.' >&2; exit 2; }
if [ "$prepare_codex" = true ] && [ "$prepare_sandbox" = true ]; then prepare_sandbox=false; defer_sandbox=true; fi
if [ "$install_deps" = true ]; then
  sudo apt-get update
  sudo apt-get install -y ca-certificates curl tar gzip python3 openssl git dbus-user-session
  printf '%s\n' 'Ubuntu prerequisites installed. For boot-time user services, an administrator must explicitly enable lingering: loginctl enable-linger USER.'
fi
if [ "$enable_linger" = true ]; then sudo loginctl enable-linger "${SUDO_USER:-$USER}"; fi
if [ "$prepare_sandbox" = true ]; then
  case "$codex_version" in ''|*[!0-9.]*|.*|*.|*..*) echo 'Sandbox profile version must contain only numeric dotted components.' >&2; exit 2;; esac
  destination=/usr/local/lib/chunsu/codex-cli-$codex_version
  canonical_bin=$(readlink -f "$destination/bin/codex")
  expected_bin=$(readlink -f "$destination/vendor/$(case "$(uname -m)" in x86_64|amd64) echo x86_64-unknown-linux-musl;; aarch64|arm64) echo aarch64-unknown-linux-musl;; *) exit 2;; esac)/bin/codex")
  [ "$canonical_bin" = "$expected_bin" ] && [ -x "$canonical_bin" ] || { echo 'Verified Codex distribution is not installed at the expected path.' >&2; exit 1; }
  sudo test "$(sudo stat -c '%U:%G' "$destination")" = root:root && sudo test "$(sudo stat -c '%U:%G' "$canonical_bin")" = root:root || { echo 'Codex distribution must be root-owned before a sandbox profile is prepared.' >&2; exit 1; }
  profile=/etc/apparmor.d/chunsu-codex-$codex_version
  staging=$(mktemp "${TMPDIR:-/tmp}/chunsu-apparmor.XXXXXX")
  trap 'unlink "$staging"' EXIT HUP INT TERM
  printf 'abi <abi/4.0>,\ninclude <tunables/global>\nprofile chunsu-codex-%s "%s" flags=(unconfined) {\n  userns,\n}\n' "$codex_version" "$canonical_bin" > "$staging"
  if sudo test -e "$profile"; then sudo cmp -s "$staging" "$profile" || { echo "Existing AppArmor profile differs and was preserved: $profile" >&2; exit 1; }; else sudo install -o root -g root -m 0644 "$staging" "$profile"; fi
  sudo apparmor_parser -r "$profile"
  printf '%s\n' "Loaded executable-specific AppArmor user-namespace profile: $profile"
fi
if [ "$prepare_codex" = true ]; then
  [ -n "$codex_version" ] || { echo '--version is required with --prepare-codex.' >&2; exit 2; }
  command -v curl >/dev/null 2>&1 || { echo 'curl is required; use --install-deps.' >&2; exit 1; }
  command -v tar >/dev/null 2>&1 || { echo 'tar is required; use --install-deps.' >&2; exit 1; }
  case "$(uname -m)" in x86_64|amd64) platform=linux-x64; vendor=x86_64-unknown-linux-musl;; aarch64|arm64) platform=linux-arm64; vendor=aarch64-unknown-linux-musl;; *) echo 'Unsupported Linux architecture.' >&2; exit 2;; esac
  package=@openai/codex
  encoded_package=$(printf '%s' "$package" | sed 's#/#%2f#g')
  metadata=$(curl --fail --location --proto '=https' --tlsv1.2 "https://registry.npmjs.org/$encoded_package/$codex_version-$platform")
  tarball=$(printf '%s' "$metadata" | python3 -c 'import json,sys; print(json.load(sys.stdin)["dist"]["tarball"])')
  integrity=$(printf '%s' "$metadata" | python3 -c 'import json,sys; print(json.load(sys.stdin)["dist"]["integrity"])')
  case "$tarball" in https://registry.npmjs.org/*) ;; *) echo 'Registry metadata named an unexpected tarball host.' >&2; exit 1;; esac
  staging=$(mktemp -d "${TMPDIR:-/tmp}/chunsu-codex.XXXXXX")
  trap 'rm -rf "$staging"' EXIT HUP INT TERM
  curl --fail --location --proto '=https' --tlsv1.2 -o "$staging/package.tgz" "$tarball"
  expected=${integrity#sha512-}
  actual=$(openssl dgst -sha512 -binary "$staging/package.tgz" | openssl base64 -A)
  [ "$integrity" != "$expected" ] && [ "$actual" = "$expected" ] || { echo 'The npm registry SHA-512 integrity check failed.' >&2; exit 1; }
  printf '%s\n' "$integrity" > "$staging/.chunsu-integrity"
  tar -tzf "$staging/package.tgz" | grep -q '^package/' || { echo 'The Codex archive has an unexpected layout.' >&2; exit 1; }
  tar -xzf "$staging/package.tgz" -C "$staging"
  [ -x "$staging/package/vendor/$vendor/bin/codex" ] && [ -x "$staging/package/vendor/$vendor/bin/codex-code-mode-host" ] || { echo 'The Codex platform distribution is missing Codex or its code-mode host.' >&2; exit 1; }
  destination=/usr/local/lib/chunsu/codex-cli-$codex_version
  sudo install -d -m 0755 /usr/local/lib/chunsu
  if [ -e "$destination" ]; then
    sudo test -f "$destination/.chunsu-integrity" && sudo cmp -s "$staging/.chunsu-integrity" "$destination/.chunsu-integrity" || {
      echo "Existing Codex distribution at $destination does not match this registry integrity value; it was preserved." >&2
      exit 1
    }
    echo "Existing verified Codex distribution preserved at $destination" >&2
  else
    sudo install -d -m 0755 "$destination"
    sudo cp -a "$staging/package/." "$destination/"
    sudo chown -R root:root "$destination"
    sudo chmod -R go-w "$destination"
    sudo chmod -R a+rX "$destination"
    sudo install -m 0644 "$staging/.chunsu-integrity" "$destination/.chunsu-integrity"
    sudo install -d -m 0755 "$destination/bin"
    sudo ln -s "../vendor/$vendor/bin/codex" "$destination/bin/codex"
  fi
  [ -x "$destination/bin/codex" ] && [ -x "$destination/vendor/$vendor/bin/codex-code-mode-host" ] || { echo 'Installed Codex executable or code-mode host could not be verified.' >&2; exit 1; }
  printf '%s\n' "Verified $package@$codex_version in $destination (vendor layout and code-mode host retained)."
fi
if [ "$defer_sandbox" = true ]; then "$0" --prepare-sandbox --version "$codex_version"; fi
