#!/bin/sh
# Install the system libraries required to compile and typecheck Ebiten on Linux.
# Ebiten uses CGO to interface with OpenGL and the windowing system, so these
# headers and shared libraries must be present for `go build`, `go vet`,
# golangci-lint, and gosec to succeed on a bare ubuntu-latest runner.
# xvfb provides a virtual X11 display so that GLFW (used by Ebiten) does not
# panic during `go test` on headless CI runners.
sudo apt-get update -qq
sudo apt-get install -y gcc libc6-dev libgl1-mesa-dev libxrandr-dev libxi-dev libxinerama-dev libxcursor-dev libxxf86vm-dev xvfb