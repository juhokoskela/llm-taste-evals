# Eval runner image: pinned Go toolchain, Node, and agent CLIs.
# Network access exists only at build time; at runtime the container sits on
# an internal network behind the allowlisting egress proxy (see compose file).
FROM golang:1.26.6-bookworm

ARG NODE_MAJOR=26
ARG CLAUDE_CODE_VERSION=2.1.238
ARG CODEX_VERSION=0.149.0
ARG OPENCODE_VERSION=1.18.21

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates curl gnupg iptables util-linux \
    && curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

RUN npm install -g \
    "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}" \
    "@openai/codex@${CODEX_VERSION}" \
    "opencode-ai@${OPENCODE_VERSION}"

# Pre-warm the Go module cache for every task repo at both its eval base
# commit and its default branch, so gates and agent test runs work with
# GOPROXY=off. Without this, a reachable module proxy would let the agent
# `go get` the task's published solution.
ENV GOMODCACHE=/opt/gomodcache
COPY docker/warm-modules.sh /usr/local/bin/warm-modules.sh
RUN warm-modules.sh https://github.com/juhokoskela/pipedrive-go.git \
    40ffab19c51ea1a210f17cf37bb1f08c1340352d main

# Private task repos cannot be cloned at build time; their go.mod/go.sum
# snapshots (gitignored, see docker/warm.d/README.md) are enough to download
# their dependencies into the module cache.
COPY docker/warm.d/ /tmp/warm.d/
RUN set -e; for d in /tmp/warm.d/*/; do \
      [ -f "$d/go.mod" ] || continue; \
      (cd "$d" && go mod download all); \
    done; rm -rf /tmp/warm.d

COPY runner /src/runner
RUN cd /src/runner \
	&& go build -o /usr/local/bin/eval-runner ./cmd/runner \
	&& rm -rf /src/runner
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY --chmod=0755 docker/block-scorer-network.sh /usr/local/bin/block-scorer-network.sh

# Login shells (`sh -lc`, used by codex) read /etc/profile, which resets PATH
# and would drop /usr/local/go/bin — symlink the Go tools somewhere universal.
RUN ln -s /usr/local/go/bin/go /usr/local/bin/go \
    && ln -s /usr/local/go/bin/gofmt /usr/local/bin/gofmt

RUN useradd --create-home --uid 1000 agent \
	&& useradd --no-create-home --uid 1001 scorer \
	&& mkdir -p /eval-private/auth /eval-private/tasks /eval-private/runs /eval-private/mirrors \
	&& mkdir -p /agent /score /runner \
	&& chmod 0700 /eval-private /agent /score /runner /root /home/agent /tmp /var/tmp /run/lock /go \
	&& chown root:root /home/agent \
	&& chown agent:agent /agent \
	&& ln -s /agent/workspace /work

ENV HOME=/root \
    GOPROXY=off \
    GOTOOLCHAIN=local \
    GOMODCACHE=/opt/gomodcache

WORKDIR /runner
ENTRYPOINT ["entrypoint.sh"]
