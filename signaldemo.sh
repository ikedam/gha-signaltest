#!/usr/bin/env bash
set -euo pipefail

echo "signaldemo started with $$"
pstree -pls "$$" || pstree -pl || true
echo "Sleeping for 300 seconds..."
sleep 300
