#!/bin/bash
# Build script for the diffcheck tool.
# Compiles both the md4c-plain C binary and the Go CLI.
#
# The md4c C source is fetched automatically from
#   https://github.com/mity/md4c.git
# on first use, then cached under ../md4c (which is gitignored).
#
# Usage:
#   ./build.sh             # build everything (auto-clone md4c if missing)
#   ./build.sh --c-only    # build only the C binary (auto-clone md4c if missing)
#   ./build.sh --go-only   # build only the Go CLI (does not need md4c)
#   ./build.sh --update    # git pull the cached md4c repo before building
#   ./build.sh --clean     # remove build artifacts and the cached md4c repo
#   ./build.sh -h|--help   # show this help

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CSRC_DIR="$SCRIPT_DIR/csrc"
MD4C_REPO_DIR="$SCRIPT_DIR/../md4c"
MD4C_SRC="$MD4C_REPO_DIR/src"
MD4C_REPO_URL="https://github.com/mity/md4c.git"

# ── Helpers ───────────────────────────────────────────────────────
require_git() {
    if ! command -v git >/dev/null 2>&1; then
        echo "Error: 'git' is required but was not found in PATH." >&2
        exit 1
    fi
}

# Clone the md4c repo when $MD4C_SRC is missing; otherwise refresh it
# in place if $1 == "update".
ensure_md4c() {
    local mode="${1:-}"

    if [ -d "$MD4C_SRC" ] && [ -f "$MD4C_SRC/md4c.c" ] && [ -f "$MD4C_SRC/md4c.h" ]; then
        if [ "$mode" = "update" ]; then
            require_git
            echo "Updating md4c in $MD4C_REPO_DIR ..."
            if ! git -C "$MD4C_REPO_DIR" pull --ff-only; then
                echo "Warning: 'git pull' failed; keeping existing checkout." >&2
            fi
        fi
        return 0
    fi

    if [ -e "$MD4C_REPO_DIR" ] && [ ! -d "$MD4C_SRC" ]; then
        echo "Error: $MD4C_REPO_DIR exists but does not look like an md4c checkout" >&2
        echo "       (missing $MD4C_SRC). Remove it or run: $0 --clean" >&2
        exit 1
    fi

    require_git
    echo "Cloning md4c from $MD4C_REPO_URL into $MD4C_REPO_DIR ..."
    git clone --depth 1 "$MD4C_REPO_URL" "$MD4C_REPO_DIR"

    if [ ! -d "$MD4C_SRC" ]; then
        echo "Error: clone succeeded but $MD4C_SRC is still missing." >&2
        exit 1
    fi
}

show_help() {
    sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'
}

# ── C binary: md4c-plain ──────────────────────────────────────────
build_c() {
    ensure_md4c "$1"
    echo "Compiling md4c-plain..."
    gcc -O2 -I"$MD4C_SRC" -o "$CSRC_DIR/md4c-plain" "$CSRC_DIR/main.c" "$MD4C_SRC/md4c.c"
    echo "  → $CSRC_DIR/md4c-plain"
}

# ── C binary: md4c-html ───────────────────────────────────────────
build_c_html() {
    ensure_md4c "$1"
    echo "Compiling md4c-html..."
    gcc -O2 -I"$MD4C_SRC" -o "$CSRC_DIR/md4c-html" \
        "$CSRC_DIR/main_html.c" "$MD4C_SRC/md4c.c" "$MD4C_SRC/md4c-html.c" "$MD4C_SRC/entity.c"
    echo "  → $CSRC_DIR/md4c-html"
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
        build_c ""
        build_c_html ""
        ;;
    --go-only)
        build_go
        ;;
    --update)
        build_c "update"
        build_c_html "update"
        build_go
        echo ""
        echo "Build complete. Run: ./diffcheck --help"
        ;;
    --clean)
        echo "Removing build artifacts and cached md4c checkout..."
        rm -f "$CSRC_DIR/md4c-plain" "$CSRC_DIR/md4c-html" "$SCRIPT_DIR/diffcheck"
        rm -rf "$MD4C_REPO_DIR"
        echo "Cleaned."
        ;;
    -h|--help)
        show_help
        ;;
    *)
        build_c ""
        build_c_html ""
        build_go
        echo ""
        echo "Build complete. Run: ./diffcheck --help"
        ;;
esac