# The image the agent's tools run in. Build it with `task sandbox`; `task run` uses it.
#
# The executor starts the container with no network (unless -net), all
# capabilities dropped, and the host user's UID. So everything a tool needs
# must already be in the image, and nothing may assume a home directory.
FROM golang:1.27-bookworm

# git comes with the base image; ripgrep is for `rg` (and the planned grep tool).
RUN apt-get update \
    && apt-get install -y --no-install-recommends ripgrep \
    && rm -rf /var/lib/apt/lists/*

# The host UID has no home directory in this image. Point HOME and Go's caches
# at a writable place, and never try to download another Go toolchain offline.
ENV HOME=/tmp \
    GOCACHE=/tmp/go-build \
    GOPATH=/tmp/go \
    GOTOOLCHAIN=local
