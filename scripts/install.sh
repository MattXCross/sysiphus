#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"
REPO_ROOT="$(readlink -f "$SCRIPT_DIR/..")"
INSTALL_DIR="${1:-${HOME}/.local/bin}"
TARGET_PATH="${INSTALL_DIR}/sysiphus"

mkdir -p "$INSTALL_DIR"

printf 'Building sysiphus...\n'
go build -o "$TARGET_PATH" ./cmd/sysiphus

chmod +x "$TARGET_PATH"

printf 'Installed sysiphus to %s\n' "$TARGET_PATH"

case ":${PATH}:" in
	*":${INSTALL_DIR}:"*)
		printf 'Command is available on PATH. Run: sysiphus\n'
		;;
	*)
		printf 'Add %s to your PATH, then run: sysiphus\n' "$INSTALL_DIR"
		;;
esac
