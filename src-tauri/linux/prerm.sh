#!/bin/sh
# Unloads and deletes the profile postinst.sh installed, on removal alone: an
# upgrade's postinst puts the new one in its place.
set -e

if [ "$1" = remove ] && [ -e /etc/apparmor.d/kstack-bwrap ]; then
  if command -v apparmor_parser >/dev/null 2>&1 && [ -d /sys/kernel/security/apparmor ]; then
    apparmor_parser --remove /etc/apparmor.d/kstack-bwrap || true
  fi
  rm -f /etc/apparmor.d/kstack-bwrap
fi
