Module-warm snapshots: one directory per repo+ref holding that ref's go.mod
and go.sum, so the image build can `go mod download` dependencies for repos
it cannot clone (private mirrors). Snapshot dirs are gitignored because
go.mod/go.sum reveal private module paths and dependency graphs.

Regenerate with, e.g.:
  git -C ../rag-service show <ref>:go.mod > docker/warm.d/rag-<ref>/go.mod
  git -C ../rag-service show <ref>:go.sum > docker/warm.d/rag-<ref>/go.sum
