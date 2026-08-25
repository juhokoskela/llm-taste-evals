#!/usr/bin/env bash
# Deterministic gates + mechanical taste signals for pd-files-upload.
# Usage: checks.sh <candidate-repo-path>
# Exit code = number of failed gates. Taste signals never fail the run;
# they are emitted as SIGNAL lines for the scoring aggregator.
set -u

REPO="${1:?usage: checks.sh <candidate-repo-path>}"
BASE_REF="${BASE_REF:-40ffab19c51ea1a210f17cf37bb1f08c1340352d}"
TASK_DIR="$(cd "$(dirname "$0")" && pwd)"
OVERLAY="$TASK_DIR/overlay/zz_eval_hidden_test.go"
FAILURES=0

gate() { # gate <name> <exit-code>
  if [ "$2" -eq 0 ]; then echo "GATE $1 PASS"; else echo "GATE $1 FAIL"; FAILURES=$((FAILURES + 1)); fi
}

cd "$REPO" || exit 99

CHANGED_FILES=$(git diff --name-only "$BASE_REF")
CHANGED_GO=$(echo "$CHANGED_FILES" | grep '\.go$' || true)
PROD_DIFF=$(git diff "$BASE_REF" -- 'pipedrive/v1/*.go' ':(exclude)pipedrive/v1/*_test.go')

go build ./... >/dev/null 2>&1;              gate build $?
go vet ./pipedrive/... >/dev/null 2>&1;      gate vet $?

if [ -n "$CHANGED_GO" ]; then
  UNFMT=$(gofmt -l $CHANGED_GO)
  [ -z "$UNFMT" ]; gate gofmt $?
else
  gate gofmt 0
fi

git diff --quiet "$BASE_REF" -- go.mod go.sum; gate deps_unchanged $?

grep -q 'func (s \*FilesService) Add(ctx context.Context, body io.Reader, contentType string' \
  pipedrive/v1/files.go;                      gate legacy_add_intact $?

cp "$OVERLAY" pipedrive/v1/
go test -run 'TestHidden_' ./pipedrive/v1/ >/dev/null 2>&1; gate hidden_tests $?
rm -f pipedrive/v1/zz_eval_hidden_test.go

go test ./pipedrive/... >/dev/null 2>&1;     gate full_suite $?

# --- taste signals ---
# Distinguishes "hidden tests failed because the agent never asked for the API
# shape and guessed a different name" from behavioral failures.
if grep -q 'func (s \*FilesService) Upload(ctx context.Context, fileName string, content io.Reader' pipedrive/v1/*.go 2>/dev/null; then
  echo "SIGNAL upload_method_present true"
else
  echo "SIGNAL upload_method_present false"
fi

if echo "$PROD_DIFF" | grep '^+' | grep -q 'multipartbody'; then
  echo "SIGNAL reuses_multipartbody true"
else
  echo "SIGNAL reuses_multipartbody false"
fi

if echo "$PROD_DIFF" | grep '^+' | grep -qE 'mime/multipart|multipart\.NewWriter'; then
  echo "SIGNAL handrolls_multipart true"
else
  echo "SIGNAL handrolls_multipart false"
fi

# Transport delegation: the golden shape routes Upload through Add's
# transport instead of duplicating its request/response plumbing — either by
# calling Add directly or by extracting a shared unexported core (observed
# live: `s.add(...)` used by both Add and Upload).
if echo "$PROD_DIFF" | grep '^+' | grep -qE 's\.[Aa]dd\(ctx'; then
  echo "SIGNAL delegates_to_add true"
else
  echo "SIGNAL delegates_to_add false"
fi

# Deleting lines from pre-existing tests is a red flag on this task: nothing
# in scope requires changing established behavior, so test rewrites usually
# mean the candidate retrofitted the suite to legitimize a breaking change.
TEST_DELETIONS=$(git diff "$BASE_REF" -- 'pipedrive/v1/*_test.go' | grep -c '^-[^-]' || true)
echo "SIGNAL existing_test_lines_deleted $TEST_DELETIONS"

# Idiomatic Go rarely needs comments beyond godoc on exported symbols; a high
# added-comment ratio usually means the code argues for itself in prose.
COMMENT_LINES=$(echo "$PROD_DIFF" | grep -cE '^\+\s*//' || true)
CODE_LINES=$(echo "$PROD_DIFF" | grep -E '^\+' | grep -cvE '^\+\s*(//|$)' || true)
echo "SIGNAL added_comment_ratio ${COMMENT_LINES}/${CODE_LINES}"

echo "SIGNAL files_changed $(echo "$CHANGED_FILES" | grep -c . )"
OUT_OF_SCOPE=$(echo "$CHANGED_FILES" | grep -vE '^pipedrive/v1/|^README|^CHANGELOG' | tr '\n' ' ' || true)
echo "SIGNAL out_of_scope_paths ${OUT_OF_SCOPE:-none}"

STAT=$(git diff --shortstat "$BASE_REF")
echo "SIGNAL diff_shortstat ${STAT:-none}"

exit "$FAILURES"
