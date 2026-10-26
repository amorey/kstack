#!/bin/sh
# Fails if a workflow or composite action under .github/ uses a third-party action
# by tag or branch. Either can be moved to new code; a commit SHA cannot.
set -eu

cd "$(dirname "$0")/.."

found=$(grep -rnE '^[[:space:]]*(-[[:space:]]+)?uses:' .github/ |
	grep -vE 'uses:[[:space:]]+\./' |
	grep -vE '@[0-9a-f]{40}([[:space:]]|$)' || true)
if [ -n "$found" ]; then
	echo "actions not pinned to a commit SHA:"
	echo "$found" | sed 's/^/  /'
	echo "Pin each to a full commit SHA, with the version in a trailing comment."
	exit 1
fi
