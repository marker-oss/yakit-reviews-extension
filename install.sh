#!/bin/sh
# Reviews installer bootstrap.
#
# Downloads the prebuilt `reviews` binary for the current OS/architecture from
# the latest GitHub release, then points you at the setup wizard.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/marker-oss/yakit-reviews-extension/main/install.sh | sh
#
# Optional environment variables:
#   REVIEWS_VERSION      release tag to install (default: latest)
#   REVIEWS_INSTALL_DIR  directory to place the binary (default: current dir)

set -eu

REPO="marker-oss/yakit-reviews-extension"
VERSION="${REVIEWS_VERSION:-latest}"
INSTALL_DIR="${REVIEWS_INSTALL_DIR:-$(pwd)}"

err() {
	printf '%s\n' "error: $*" >&2
	exit 1
}

# --- detect OS ---
os="$(uname -s)"
case "$os" in
	Linux) os="linux" ;;
	Darwin) os="darwin" ;;
	MINGW* | MSYS* | CYGWIN* | Windows_NT)
		err "Windows: download reviews-windows-amd64.exe from https://github.com/$REPO/releases/latest and run it from PowerShell."
		;;
	*) err "unsupported OS: $os" ;;
esac

# --- detect architecture ---
arch="$(uname -m)"
case "$arch" in
	x86_64 | amd64) arch="amd64" ;;
	arm64 | aarch64) arch="arm64" ;;
	*) err "unsupported architecture: $arch" ;;
esac

asset="reviews-${os}-${arch}"

# --- pick a downloader ---
if command -v curl >/dev/null 2>&1; then
	download() { curl -fSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
	download() { wget -O "$2" "$1"; }
else
	err "need curl or wget to download the binary"
fi

# --- build the release URL ---
if [ "$VERSION" = "latest" ]; then
	url="https://github.com/$REPO/releases/latest/download/$asset"
else
	url="https://github.com/$REPO/releases/download/$VERSION/$asset"
fi

target="$INSTALL_DIR/reviews"

printf 'Downloading %s (%s)...\n' "$asset" "$VERSION"
if ! download "$url" "$target"; then
	rm -f "$target"
	err "download failed: $url
No release asset yet? Build from source instead:
  git clone https://github.com/$REPO.git
  cd yakit-reviews-extension && go build -o reviews ./cmd/reviews"
fi
chmod +x "$target"

printf '\nInstalled: %s\n\n' "$target"
printf 'Next step — run the setup wizard (interactive):\n'
printf '  %s install\n' "$target"
