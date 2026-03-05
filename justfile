set shell := ["bash", "-euo", "pipefail", "-c"]

# Build the st binary into the repo root.
build:
	go build -o st ./cmd/st

# Install the CLI binary to ~/.local/bin.
install: build
	mkdir -p "$HOME/.local/bin"
	install -m 0755 st "$HOME/.local/bin/st"
