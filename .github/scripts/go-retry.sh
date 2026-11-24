#!/usr/bin/env bash
# go-retry.sh: run a go command that downloads modules (go install <tool>@<version>,
# go mod download) and retry it when the module proxy or the checksum database has a
# transient failure, such as an HTTP/2 stream reset from sum.golang.org
# ("stream error ... INTERNAL_ERROR; received from peer"), a 5xx or a timeout.
#
# Verification is unchanged: every module is still checked against go.sum and
# sum.golang.org. A checksum mismatch is a security failure, not a network one, so it
# is reported at once and never retried.
#
# Usage: go-retry.sh install golang.org/x/vuln/cmd/govulncheck@v1.9.0
# Environment: GO_RETRY_ATTEMPTS (default 4) and GO_RETRY_DELAY (default 15): the
# wait before attempt n+1 is n * GO_RETRY_DELAY seconds.
set -uo pipefail

attempts=${GO_RETRY_ATTEMPTS:-4}
delay=${GO_RETRY_DELAY:-15}
log=$(mktemp)
trap 'rm -f "$log"' EXIT

for ((i = 1; i <= attempts; i++)); do
    if go "$@" 2>&1 | tee "$log"; then
        exit 0
    fi
    if grep -qE 'SECURITY ERROR|checksum mismatch' "$log"; then
        echo "::error::go $*: module verification failed (checksum mismatch); not retried"
        exit 1
    fi
    if ((i < attempts)); then
        echo "::warning::go $* failed (attempt $i of $attempts), retrying in $((i * delay))s"
        sleep $((i * delay))
    fi
done
echo "::error::go $* failed after $attempts attempts"
exit 1
