#!/usr/bin/env bash
set -euo pipefail

uid="${EVAL_SCORER_UID:-}"
if [ -z "$uid" ]; then
	exit 0
fi

for firewall in iptables ip6tables; do
	"$firewall" -w -C OUTPUT -o lo -m owner --uid-owner "$uid" -j ACCEPT 2>/dev/null \
		|| "$firewall" -w -I OUTPUT 1 -o lo -m owner --uid-owner "$uid" -j ACCEPT
	"$firewall" -w -C OUTPUT -m owner --uid-owner "$uid" -j REJECT 2>/dev/null \
		|| "$firewall" -w -A OUTPUT -m owner --uid-owner "$uid" -j REJECT
done
