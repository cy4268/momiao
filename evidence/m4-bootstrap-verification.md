# M4a local candidate verification

- Baseline: `744000ee37d5ee2583345a1f05c194a3811e50b4`.
- Only reviewed schema 0006/0007 were imported; prior migrations and runner are unchanged. No M2b/M3c application WIP was imported.
- PostgreSQL 18.6, isolated `momiao_test_m4_*` databases only. `go test -count=1 -timeout=90s -v ./internal/platform -run '^TestBootstrap'`: **exit 0, 16 pass / 0 fail / 0 skip**, 28.083 s (2026-09-06). The first pre-schema run failed as expected with `BOOTSTRAP_TRANSACTION_FAILED`.
- Tests cover one atomic principal/history/audit, 12 concurrent attempts → one success, rollback on principal/history/audit failures, existing roles and disabled principals, closure surviving deletion, immutable UPDATE/DELETE/TRUNCATE, and server COMMIT observed by a protocol proxy before deliberately dropping its acknowledgement. The lost-ack case returns OUTCOME_UNKNOWN and its retry returns ALREADY_CLOSED with one committed set.
- Least-privilege test uses actual synthetic LOGIN roles (`session_user=current_user`, no superuser/role/database creation), not SET ROLE on an owner connection. The function owner is a separate restricted NOLOGIN role. Deployment role has one EXECUTE entry and no direct table/DDL authority; runtime has no EXECUTE and cannot forge SYSTEM_BOOTSTRAP audit through its generic audit INSERT grant.
- Real Unix HTTP client + synthetic native source tests passed, including caller timeout and source response/credential failure cases. These do not claim a fresh run of the entire native authentication application.
- Full Go test invocation passed the command/source suites and exposed the inherited five-migration manifest expectation. After updating that test to the exact contiguous eight-migration count, its focused rerun passed (exit 0). Previously passing suites were not rerun only to produce another log.
- Final `go vet ./...`, manifest-focused test, and `go build -trimpath -ldflags '-X main.releaseBuild=m4-bootstrap-candidate' ... ./cmd/momiao-bootstrap`: exit 0. No frontend changes or frontend test rerun.
- All credentials/accounts are synthetic or private local test connection state; none are logged or committed. No production account, role, deployment, Docker object or shared application checkout was modified.

Remaining release gates: approved combination import, real Linux source/socket/build provenance, temporary deployment grants, exact real target/session verification and production typed TTY. Rollback preserves migration/history/audit data. The Level 3 administrator-management interface is not part of this candidate.
