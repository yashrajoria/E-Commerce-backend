#!/usr/bin/env bash
set -euo pipefail

# Render docs/architecture.mmd -> docs/architecture.svg using mermaid-cli
# Requires Node.js and npx available. Runs mermaid-cli via npx so no global install required.

INPUT=docs/architecture.mmd
OUTPUT=docs/architecture.svg

if [ ! -f "$INPUT" ]; then
  echo "Missing $INPUT"
  exit 1
fi

echo "Rendering $INPUT -> $OUTPUT"

npx -y @mermaid-js/mermaid-cli@10.0.0 -i "$INPUT" -o "$OUTPUT"

echo "Done: $OUTPUT"
