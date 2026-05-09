#!/usr/bin/env bash
set -euo pipefail

echo "signaldemo started with $$"
pstree -gpls "$$" || pstree -gpl || true
echo "Sleeping for 300 seconds..."
sleep 300
