#!/usr/bin/env bash
set -euo pipefail

mdok_dir="${HOME}/.mdok"

if command -v mdok >/dev/null 2>&1; then
  mdok stop --all >/dev/null 2>&1 || true
fi

# Cleanup runtime artifacts, keep configs by default.
rm -rf "${mdok_dir}/pids" "${mdok_dir}/logs" "${mdok_dir}/data"

echo "Stopped mdok instances and cleaned runtime data in ${mdok_dir}."
