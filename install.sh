#!/bin/sh
# Install sortie-loop: build the binary, install sortie, write the shim.
#
# Usage:
#   ./install.sh                    install sortie-loop to ~/.local/bin
#   ./install.sh --prefix DIR       install binaries under DIR/bin
#   ./install.sh --sortie-bin PATH  use an existing sortie binary
#   curl -sSL https://raw.githubusercontent.com/kinged007/sortie-loop/main/install.sh | sh
#
# Requirements: go >= 1.24, git, gh (only `sortie-loop setup` needs gh).
set -eu

REPO="kinged007/sortie-loop"
PREFIX="${HOME}/.local"
SORTIE_BIN=""

while [ $# -gt 0 ]; do
  case "$1" in
    --prefix) PREFIX="$2"; shift 2 ;;
    --prefix=*) PREFIX="${1#--prefix=}"; shift ;;
    --sortie-bin) SORTIE_BIN="$2"; shift 2 ;;
    --sortie-bin=*) SORTIE_BIN="${1#--sortie-bin=}"; shift ;;
    -h|--help) sed -n '2,10p' "$0"; exit 0 ;;
    *) echo "error: unknown flag $1" >&2; exit 1 ;;
  esac
done

SRC="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="${PREFIX}/bin"
SHARE_DIR="${PREFIX}/share/sortie-loop"

command -v go >/dev/null || { echo "error: go >= 1.24 not found" >&2; exit 1; }
command -v git >/dev/null || { echo "error: git not found" >&2; exit 1; }

echo ":: building sortie-loop ..."
if [ "$SRC" = "." ] || [ ! -f "$SRC/go.mod" ]; then
  # Piped via curl: clone to a temp dir.
  SRC="$(mktemp -d)/sortie-loop"
  git clone --depth 1 "https://github.com/${REPO}.git" "$SRC"
  trap 'rm -rf "$(dirname "$SRC")"' EXIT
fi
mkdir -p "$BIN_DIR"
(cd "$SRC" && go build -trimpath -o "$BIN_DIR/sortie-loop" ./cmd/sortie-loop)
echo ":: installed $BIN_DIR/sortie-loop"

resolve_sortie() {
  if [ -n "$SORTIE_BIN" ]; then echo "$SORTIE_BIN"; return; fi
  if command -v sortie >/dev/null; then command -v sortie; return; fi
  echo ""
}

FOUND="$(resolve_sortie)"
if [ -n "$FOUND" ]; then
  mkdir -p "$SHARE_DIR"
  VERSION="$("$BIN_DIR/sortie-loop" --dump-version 2>/dev/null || echo unknown)"
  ln -sf "$FOUND" "$SHARE_DIR/sortie-$VERSION"
  echo ":: linked sortie $FOUND"
else
  cat >&2 <<'EOF'
warning: no sortie binary found. The workflows need the pi agent and
github-pr tracker, which no sortie release ships yet (v1.24.0 lacks both).
Install sortie from source and point the loop at it:

  git clone https://github.com/sortie-ai/sortie.git
  cd sortie && go build -o ~/.local/bin/sortie ./cmd/sortie

or set SORTIE_BIN=/path/to/sortie at runtime.
EOF
fi

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo ":: add to PATH: export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac

cat <<'EOF'
:: next steps (run inside the target repo):
     sortie-loop setup    # writes .sortie/config.yaml, creates labels
     sortie-loop          # run plan + dev + review loops (Ctrl-C stops all)
EOF
