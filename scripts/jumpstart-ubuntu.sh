# Install the tools we need to get going with Soldier-Sense on Ubuntu derived systems.

set -euo pipefail

JUST_VERSION="1.40.0"

sudo apt-get update
sudo apt-get install -y golang-go curl ca-certificates

if apt-cache show just > /dev/null 2>&1; then
	sudo apt-get install -y just
else
	ARCH="$(dpkg --print-architecture)"
	case "$ARCH" in
		amd64) JUST_ARCH="x86_64-unknown-linux-musl" ;;
		arm64) JUST_ARCH="aarch64-unknown-linux-musl" ;;
		*)
			echo "Unsupported architecture for binary fallback: $ARCH"
			echo "Install just manually: https://github.com/casey/just/releases"
			exit 1
			;;
	esac

	JUST_TARBALL="just-${JUST_VERSION}-${JUST_ARCH}.tar.gz"
	JUST_URL="https://github.com/casey/just/releases/download/${JUST_VERSION}/${JUST_TARBALL}"

	tmp_dir="$(mktemp -d)"
	trap 'rm -rf "$tmp_dir"' EXIT

	curl -fsSL "$JUST_URL" -o "$tmp_dir/$JUST_TARBALL"
	tar -xzf "$tmp_dir/$JUST_TARBALL" -C "$tmp_dir"
	sudo install -m 0755 "$tmp_dir/just" /usr/local/bin/just
fi

echo "Installed versions:"
go version
just --version

go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest
