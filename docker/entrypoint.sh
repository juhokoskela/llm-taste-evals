#!/usr/bin/env bash
# Seeds agent CLI auth from env before handing off to the runner.
# Codex has no supported pure-env auth in headless mode. Seed a root-only file
# that the runner copies into each attempt's fresh agent home.
set -euo pipefail

# Docker mounts /dev/shm at container start, after image permissions apply.
# Contestants do not need shared memory and must not use it across attempts.
chmod 0700 /dev/shm

# Scoring executes contestant-controlled tests. The scorer may use loopback
# fixtures, but it must not reach the egress proxy or any external endpoint.
/usr/local/bin/block-scorer-network.sh

if [ -n "${OPENAI_API_KEY:-}" ]; then
	auth_file=/eval-private/auth/codex-auth.json
	umask 077
	printf '{"auth_mode":"apikey","OPENAI_API_KEY":"%s"}\n' "$OPENAI_API_KEY" \
	  > "$auth_file"
	export EVAL_CODEX_AUTH_FILE="$auth_file"
fi

exec eval-runner "$@"
