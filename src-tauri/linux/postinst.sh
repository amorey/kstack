#!/bin/sh
# Installs and loads the AppArmor profile that lets Kstack's own bwrap make
# the user namespaces Ubuntu 23.10 and 24.04 restrict. The profile is written
# for abi 4.0, and a profile in /etc/apparmor.d that the parser cannot read
# fails apparmor.service on every boot, so it goes there only where the parser
# has that abi. Where AppArmor is not running there is nothing to load, and a
# load that fails leaves Bash unsandboxed rather than the package uninstalled.
set -e

if [ "$1" = configure ] && command -v apparmor_parser >/dev/null 2>&1 \
  && [ -d /sys/kernel/security/apparmor ] && [ -e /etc/apparmor.d/abi/4.0 ]; then
  { cp /usr/lib/kstack/apparmor/kstack-bwrap /etc/apparmor.d/kstack-bwrap \
    && apparmor_parser --replace --write-cache /etc/apparmor.d/kstack-bwrap; } || true
fi
