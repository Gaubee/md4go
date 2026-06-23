#!/bin/bash
# Build script for the diffcheck tool.
# Compiles both the md4c-plain C binary and the Go CLI.
#
# Usage:
#   ./build.sh           # build everything
#   ./build.sh --c-only  # build only the C binary
#   ./build.sh --go-only # build only the Go CLI

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CSRC_DIR="$SCRIPT_DIR/csrc"
MD4C_SRC="$SCRIPT_DIR/../md4c/src"

# ── C binary: md4c-plain ──────────────────────────────────────────
build_c() {
    if [ ! -d "$MD4C_SRC" ]; then
        echo "Error: md4c source not found at $MD4C_SRC" >&2
        exit 1
    fi
    echo "Compiling md4c-plain..."
    gcc -O2 -I"$MD4C_SRC" -o "$CSRC_DIR/md4c-plain" "$CSRC_DIR/main.c" "$MD4C_SRC/md4c.c"
    echo "  → $CSRC_DIR/md4c-plain"
}

# ── Go CLI: diffcheck ─────────────────────────────────────────────
build_go() {
    echo "Building diffcheck CLI..."
    cd "$SCRIPT_DIR"
    go build -o "$SCRIPT_DIR/diffcheck" ./cmd/diffcheck
    echo "  → $SCRIPT_DIR/diffcheck"
}

# ── Main ──────────────────────────────────────────────────────────
case "${1:-}" in
    --c-only)
        build_c
        ;;
    --go-only)
        build_go
        ;;
    *)
        build_c
        build_go
        echo ""
        echo "Build complete. Run: ./diffcheck --help"
        ;;
esac
