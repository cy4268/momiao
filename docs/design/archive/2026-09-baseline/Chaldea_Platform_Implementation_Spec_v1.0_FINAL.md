# Chaldea Platform — Implementation Spec v1.0 WORKING

> **历史参考归档（已脱敏）**：本文件的 FINAL / FROZEN 是历史状态，不代表当前实现或当前强制流程；现行决策优先。见[归档索引](../README.md)与[决策 0001](../../../decisions/0001-pragmatic-baseline.md)。`examples/` 路径仅为说明占位，相关部署、图片和私有文件不随仓库提供。

> 版本：`v1.0 FINAL`  
> 状态：`ACTIVE / IMPLEMENTATION SPEC IN PROGRESS`  
> 当前已冻结批次：`IS-01 — Repository / Build / Dependency / Environment Baseline`、`IS-02 — NewAPI Source Verification & Adapter Specification`、`IS-03 — PostgreSQL Schema & Migration Specification`、`IS-04 — Auth / Session / Master / Account / Cutover Implementation Specification`、`IS-05 — Economy / Rewards Implementation Specification`、`IS-06 — Game Platform + V1 Direct Play Implementation Specification`、`IS-07 — Poker Implementation Specification`、`IS-08 — Rankings / History / Announcements / Jobs Implementation Specification`、`IS-09 — Operations / RBAC / Audit / Maintenance Implementation Specification`、`IS-10 — Frontend Implementation Specification`、`IS-11 — Security / Observability / Compose / Backup / DR Implementation Specification`、`IS-12 — OpenAPI / Poker WS / Deployment Runbook / Final Audit`  
> 当前冻结 Implementation Decisions：`IS-FRZ-001 ～ IS-FRZ-704`  
> 上游权威链：`需求基线 v0.2.11 → IA v0.3.1 FINAL → Art Direction v0.4 FINAL → Technical Design v0.5 FINAL → Implementation Spec v1.0`  
> 下一阶段：`Implementation Execution / Source Verification / Deployment Verification`

---

# 0. 文档工作规则

## 0.1 Implementation Spec 职责

Implementation Spec 的职责不是重新设计 Chaldea，而是把已经冻结的需求、IA、Art Direction 与 Technical Design 转化为 Codex 可以直接执行的 Repository、Package、Schema、Contract、Config、Secret、Build、Compose、Test、Runbook 与实施顺序。

Technical Design `TD-FRZ-001～552` 不允许在本文件中被静默改写。实现层发现冲突时必须指出对应 TD-FRZ、给出证据与影响，并通过 Versioned Technical Change Proposal 处理。

## 0.2 累计冻结规则

```text
完整设计一批
→ 用户明确确认
→ 立即写入累计 Implementation Spec
→ 更新 Change Log
→ 冻结连续 IS-FRZ
→ 进入下一批
```

后续修改既有 IS-FRZ 时，旧条目保留并标记 `SUPERSEDED`，再新增新的 IS-FRZ。

## 0.3 当前四类非 FINAL 事项

### A — Product Decision Blocker

```text
Reward OPEN
Poker Product Gap 01～05
Public Recent Wins / Featured Records Selection Policy
```

### B — Source Verification Blocker

```text
NewAPI SV-01 ～ SV-16
Deployment environment verification
```

### C — Implementation Configuration

```text
Session Idle / Absolute TTL
Active Quota Watermarks
Read Retry / Backoff
Poker WS Reconnect Backoff
Rate Limit Thresholds
Request Body/Header Limits
Service Assertion Clock Skew
Container CPU/RAM Caps
PostgreSQL Memory Tuning
Redis Memory/Cache Sizing
JS Bundle Regression Budget
Runtime Log Hard-size Cap
Generic Operational Alert Thresholds
```

### D — Production Readiness

```text
NewAPI Source Verification
Actual VPS Deployment Verification
Rights / Final Asset Gate
Accessibility Gate
Performance / Load Gate
Security / Supply-chain Gate
Backup / Restore Drill
Full DR Verification
```

四类必须永久分离，不重新混成模糊 `TBD`。

---

# 1. IS-01 — Repository / Build / Dependency / Environment Baseline

> 状态：`FROZEN`  
> 用户确认：`可以`  
> Frozen Decision Range：`IS-FRZ-001 ～ IS-FRZ-027`

## 1.1 Purpose

IS-01 冻结 Chaldea 自身的 Repository Strategy、Physical Repository Tree、Frontend / Platform / Poker Module Boundary、Go Module、Package Ownership、Contract Directory、Toolchain Baseline、Dependency Lock Policy、Build Command、Build Metadata、Environment Profile、Config Boundary、Secret Storage / Manifest、Implementation-only Config Register、Release Manifest、Foundation CI、NewAPI Source Verification Slot、Deployment Verification Slot、Acceptance Criteria 与 Codex Implementation Boundary。

本批不写生产业务代码，不创建生产数据库 Migration，不修改 NewAPI，不猜 NewAPI Endpoint/Table/Field，不解决 Product OPEN，不部署 Production。

## 1.2 Upstream Traceability

主要落实：

```text
TD-FRZ-001～015  → Platform / Poker / NewAPI Service Boundary
TD-FRZ-044～057  → DB Role / Hybrid ID / UUIDv7 / Schema Ownership
TD-FRZ-390～439  → Frontend Technical Architecture
TD-FRZ-440～496  → Security / Deployment / Supply-chain / Release
TD-FRZ-537～545  → Machine-readable Contract / SV / Implementation Config
TD-FRZ-552       → Implementation Spec Gate
```

部署边界继续保持：

```text
examples/deployment/external-newapi
examples/deployment/platform
```

两套 Compose Project 独立，Chaldea 不接管 NewAPI Upgrade。

---

# 2. Repository Strategy

采用：

> **Single Chaldea Monorepo + External NewAPI Repository / Deployment**

```text
Chaldea Git Repository
├── Frontend SPA
├── Platform Backend
├── Poker Service
├── API / WS Contracts
├── Database Specifications
├── Deployment Specifications
├── Operations / Backup / Testing
└── Implementation Spec

NewAPI
└── 独立源码 / 独立 Compose / 独立升级生命周期
```

永久禁止把 NewAPI Full Source Vendor / Fork 进 Chaldea Repository 后进行静默修改。

---

# 3. Canonical Repository Tree

```text
chaldea-platform/
│
├── README.md
├── .editorconfig
├── .gitattributes
├── .gitignore
├── .npmrc
├── .node-version
├── Makefile
├── toolchain.lock
├── go.work
│
├── frontend/
│   ├── package.json
│   ├── package-lock.json
│   ├── tsconfig.json
│   ├── vite.config.ts
│   ├── index.html
│   ├── src/
│   │   ├── app/
│   │   │   ├── bootstrap/
│   │   │   ├── router/
│   │   │   ├── gates/
│   │   │   └── providers/
│   │   ├── routes/
│   │   │   ├── public/
│   │   │   ├── authenticated/
│   │   │   ├── games/
│   │   │   ├── poker/
│   │   │   └── operations/
│   │   ├── features/
│   │   │   ├── auth/
│   │   │   ├── master/
│   │   │   ├── models/
│   │   │   ├── api/
│   │   │   ├── wallet/
│   │   │   ├── rewards/
│   │   │   ├── games/
│   │   │   ├── rankings/
│   │   │   ├── history/
│   │   │   ├── announcements/
│   │   │   └── operations/
│   │   ├── realtime/
│   │   │   └── poker/
│   │   ├── design-system/
│   │   ├── media/
│   │   ├── generated/
│   │   ├── shared/
│   │   └── styles/
│   └── public/
│
├── backend/
│   ├── go.mod
│   ├── go.sum
│   ├── cmd/
│   │   └── chaldea-platform/
│   │       └── main.go
│   └── internal/
│       ├── app/
│       ├── buildinfo/
│       ├── config/
│       ├── httpapi/
│       ├── auth/
│       ├── identity/
│       ├── models/
│       ├── apikeys/
│       ├── apiusage/
│       ├── economy/
│       ├── rewards/
│       ├── games/
│       ├── rankings/
│       ├── history/
│       ├── content/
│       ├── operations/
│       ├── jobs/
│       ├── audit/
│       ├── migration/
│       ├── adapters/
│       │   ├── discord/
│       │   ├── newapi/
│       │   └── poker/
│       ├── storage/
│       │   ├── postgres/
│       │   └── redis/
│       ├── generated/
│       └── foundation/
│           ├── id/
│           ├── amount/
│           ├── idempotency/
│           ├── serviceauth/
│           ├── errors/
│           └── observability/
│
├── poker/
│   ├── go.mod
│   ├── go.sum
│   ├── cmd/
│   │   └── chaldea-poker/
│   │       └── main.go
│   └── internal/
│       ├── app/
│       ├── buildinfo/
│       ├── config/
│       ├── transport/
│       │   ├── ws/
│       │   └── internalhttp/
│       ├── serviceauth/
│       ├── actor/
│       ├── lobby/
│       ├── table/
│       ├── seat/
│       ├── session/
│       ├── hand/
│       ├── betting/
│       ├── pot/
│       ├── settlement/
│       ├── funding/
│       ├── recovery/
│       ├── fairness/
│       ├── chat/
│       ├── projection/
│       ├── persistence/
│       │   └── postgres/
│       ├── ephemeral/
│       │   └── redis/
│       └── generated/
│
├── contracts/
│   ├── openapi/
│   │   └── chaldea-bff-v1.yaml
│   └── poker-ws/
│       ├── envelope.schema.json
│       ├── client-messages/
│       └── server-events/
│
├── database/
│   ├── migrations/
│   ├── schema_manifest.md
│   ├── migration_manifest.md
│   └── role_grant_matrix.md
│
├── integration/
│   └── newapi/
│       ├── source_verification_register.md
│       └── deployment_facts.md
│
├── deployment/
│   ├── compose/
│   │   └── README.md
│   ├── environment_matrix.md
│   ├── secrets_manifest.md
│   ├── release_manifest.schema.json
│   └── verification/
│       └── DEPLOYMENT-VERIFY-01.md
│
├── operations/
│   ├── maintenance_runbook.md
│   └── incident_runbook.md
│
├── backup/
│   ├── backup_runbook.md
│   ├── restore_runbook.md
│   └── dr_runbook.md
│
├── testing/
│   ├── acceptance_matrix.md
│   ├── recovery_matrix.md
│   ├── security_gate.md
│   └── foundation/
│
├── scripts/
├── docs/
│   └── implementation/
└── dist/                 # gitignored
```

`database/migrations/` 在 IS-01 只建立目录和规则，不创建任何生产 Migration。

`deployment/compose/` 在 IS-01 只建立规格入口；Production Compose 必须等待真实部署环境核验。

---

# 4. Go Module Boundary

正式使用两个 Go Module：

```text
backend/go.mod
module chaldea.internal/platform

poker/go.mod
module chaldea.internal/poker
```

Root：

```text
go.work

use (
    ./backend
    ./poker
)
```

`go.work` 仅用于本地开发与统一检查。

Production / Container Build：

```text
GOWORK=off
```

Platform 与 Poker 必须分别可独立构建。

永久禁止：

```text
backend
→ import chaldea.internal/poker/internal/...

poker
→ import chaldea.internal/platform/internal/...
```

跨服务只能依赖正式 Contract / Service Identity / DB Ownership Boundary。

---

# 5. Package Ownership

## 5.1 Platform Backend

继续采用 **Go Modular Monolith**。Domain 顶层 Package 拥有业务逻辑。

`foundation/` 仅允许真正跨域、无业务所有权的技术 Primitive，例如 UUID、Amount Parser、Idempotency、Service Assertion、Safe Error 与 Observability Primitive。

`adapters/newapi/` 是唯一常规 NewAPI Integration Boundary。

禁止 Economy、Rewards、Auth 等 Domain 绕过 Adapter 直接访问 NewAPI HTTP / DB / Password Table。

## 5.2 Poker

Poker 保持独立 Runtime：

```text
actor/                → Table Single-writer Serialization
persistence/postgres/ → Durable Poker Authority
ephemeral/redis/      → Presence / Connection / Short-lived Projection
funding/              → Poker Funding Orchestration
```

Package 存在不表示 Poker Product Gap 已解决。不得按开发者常识补齐五项 Product Gap。

---

# 6. Contract Directory

```text
contracts/
├── openapi/
│   └── chaldea-bff-v1.yaml
└── poker-ws/
    ├── envelope.schema.json
    ├── client-messages/
    └── server-events/
```

生成目录：

```text
frontend/src/generated/
backend/internal/generated/
poker/internal/generated/
```

全部为 **Generator-owned / No Manual Edit**。

IS-01 不提前伪造完整接口内容。

---

# 7. Toolchain Baseline

本批冻结：

| Component | Baseline | Policy |
|---|---:|---|
| Go | `1.27.1` | Go 1.27 line |
| Node.js | `24.20.0 LTS` | Node 24 LTS |
| npm | `11.19.0` | Node 24.20.0 paired npm |
| React | `19.2.7` | React 19.2 line |
| TypeScript | `6.0.3` | TS 6.0 line |
| Vite | `8.2.0` | Vite 8.2 line |
| React Router | `8.3.0` | Client Router; no SSR framework architecture |
| TanStack Query | `5.102.2` | HTTP server-state cache |

UUIDv7 使用 Go 1.27 标准库 `uuid.NewV7()`；V1 不为 UUIDv7 再引入第三方 Runtime Dependency。

---

# 8. Toolchain Lock / Dependency Policy

Root：

```text
toolchain.lock
GO=1.27.1
NODE=24.20.0
NPM=11.19.0
```

`.node-version`：

```text
24.20.0
```

两个 `go.mod`：

```text
go 1.27.0
toolchain go1.27.1
```

`.npmrc`：

```text
engine-strict=true
save-exact=true
package-lock=true
```

永久禁止：

```text
*
latest
^
~
floating git branch
runtime npm install <unlocked>
runtime curl | sh
```

Frontend 使用 committed `package-lock.json`，CI 使用 `npm ci`；Go 使用 committed `go.mod/go.sum`；Production Container 使用 immutable image digest。

Major Toolchain / Framework Upgrade 必须进入新的 Implementation Spec Change。

---

# 9. Initial Dependency Allowlist

Frontend Runtime：

```text
react
react-dom
react-router
@tanstack/react-query
```

Build：

```text
typescript
vite
@vitejs/plugin-react
React Type Definitions
```

其他 Form / Schema / Chart / A11y / Animation / Date / Rich Text 依赖在对应 Owner Batch 首次使用时再加入。

Go 优先使用 Standard Library；HTTP Router 第一版基于 `net/http`，不为路由本身引入 Web Framework。

必要基础依赖方向：

```text
PostgreSQL Driver → pgx/v5 family
Redis Client      → go-redis/v9 family
```

精确版本在首次 Owner Batch 落地时写入并锁定。IS-01 不预装 ORM。

---

# 10. Build Command Contract

Root `Makefile` 固定：

```text
make doctor
make bootstrap
make fmt
make fmt-check
make static-check
make test
make build
make verify
make clean
```

`make doctor` 校验 Go / Node / npm / Git 与必要本地工具；Toolchain 不匹配时 Fail，不静默使用其他版本继续构建 Production Artifact。

`make bootstrap` 至少执行 `go work sync` 与 `npm ci`，不得生成 Production Secret。

`make static-check` 至少执行 Backend/Poker `go vet ./...` 与 Frontend TypeScript typecheck。

`make build` 输出：

```text
dist/
├── bin/
│   ├── chaldea-platform
│   └── chaldea-poker
├── frontend/
└── manifests/
    ├── build.json
    └── dependency.json
```

Go Production Binary 必须使用：

```text
GOWORK=off
-trimpath
-buildvcs=true
```

---

# 11. Build Metadata

每个构建至少记录：

```text
git_commit
build_id
go_version
node_version
npm_version
frontend_dependency_lock_hash
backend_go_sum_hash
poker_go_sum_hash
build_timestamp
dirty_tree
```

Production 要求：

```text
dirty_tree = false
```

`BUILD_ID` 必须进入 Frontend、Platform、Poker、Health/Diagnostic 与 Release Manifest。

---

# 12. Environment Profiles

Deployable Environment 只允许：

```text
DEVELOPMENT
STAGING
PRODUCTION
```

不新增正式 `TEST` Environment。

Production 缺失关键 Config / Secret 时 `startup fails closed`；禁止生成临时 Production Secret 后继续启动。

---

# 13. Configuration Boundary

Backend / Poker 只有各自 `internal/config/` 可以直接读取 Runtime Environment / Secret File / Config File。

Domain Package 禁止直接调用 `os.Getenv`。

启动流程：

```text
Load
→ Parse
→ Validate
→ Build Typed Config
→ Inject
```

Frontend 只有 `src/app/bootstrap/` 允许读取 Vite Public Config，Feature 不直接读取 `import.meta.env`。

---

# 14. Frontend Public Config

只允许：

```text
WEB_ORIGIN
API_ORIGIN
POKER_WS_ORIGIN
BUILD_ID
ENVIRONMENT
```

建议映射：

```text
VITE_CHALDEA_WEB_ORIGIN
VITE_CHALDEA_API_ORIGIN
VITE_CHALDEA_POKER_WS_ORIGIN
VITE_CHALDEA_BUILD_ID
VITE_CHALDEA_ENVIRONMENT
```

任何 DB Password、OAuth Secret、Provider Credential、Signing Private Key 永不进入 Vite Environment。

---

# 15. Secret Storage Contract

Production Secret 禁止进入 Git、Dockerfile、Container Image、Frontend Bundle、Compose YAML Plaintext、Asset Manifest、Audit 与普通日志。

Host：

```text
examples/deployment/platform/secrets/
```

权限：

```text
directory = 0700 root-owned
file      = 0600
```

> **Historical Supersession Note — IS-FRZ-022**  
> Earlier `0600` per-secret-file baseline is retained as history only.  
> Current normative Production contract is `IS-FRZ-561`: host parent `0700 root-owned`; each Compose file-backed secret source is `root:<service-runtime-GID> 0640`, mounted read-only into the fixed non-root service runtime.

Container：

```text
examples/runtime/secrets/<secret_name>
```

通过 Read-only File Mount / Compose Secret 注入。

---

# 16. Secret Manifest — Initial Names

```text
platform_db_dsn
poker_db_dsn
platform_redis_credential
poker_redis_credential
discord_oauth_client_secret
platform_service_signing_ed25519_private
poker_service_signing_ed25519_private
game_fairness_keyring
poker_fairness_keyring
rate_limit_hmac_key
newapi_adapter_credential
newapi_cutover_credential
chaldea_migrator_db_dsn
backup_repository_credential
backup_recovery_material
```

当前：

```text
newapi_adapter_credential
newapi_cutover_credential
→ BLOCKED_BY_NEWAPI_SOURCE_VERIFY
```

不得猜 Credential 类型。Backup Secret 不默认挂载进 Platform / Poker Runtime；Cutover Credential 不得成为常驻 Runtime Secret。

---

# 17. Implementation-only Config Register

必须显式注册：

```text
Session Idle / Absolute TTL
Active Quota LOW_WATERMARK
Active Quota TARGET_WATERMARK
Active Quota MAX_ACTIVE_BUFFER
Read Retry Count
HTTP Retry Backoff
Poker WS Reconnect Backoff
Rate Limit Thresholds
Request Body/Header Limits
Service Assertion Clock Skew
Container CPU/RAM Caps
PostgreSQL Memory Tuning
Redis Memory/Cache Sizing
JS Bundle Regression Budget
Runtime Log Hard-size Cap
Generic Operational Alert Threshold
```

每项至少记录：

```text
config_key
owner_batch
type
unit
visibility
status
source_td_frz
current_value
evidence
updated_at
```

未决定值统一：

```text
UNRESOLVED_IMPLEMENTATION_CONFIG
```

Codex 不得写“合理默认值”冒充 Production Contract。

---

# 18. Product OPEN 与 Config 严格分离

以下继续是 `PRODUCT_DECISION_BLOCKER`，不得进入普通 Config Register：

```text
Hourly Reward asset type
Hourly time policy
Hourly accumulation
Hourly daily limit
Relief asset type
Relief accumulation
Relief during Active Poker
Reward Product Maintenance Policy
Poker Product Gap 01～05
Recent Win / Featured Record Selection Policy
```

---

# 19. Release Manifest Contract

路径：

```text
deployment/release_manifest.schema.json
```

至少记录：

```text
release_id
git_commit
build_id
frontend_artifact_hash
platform_image_digest
poker_image_digest
schema_migration_version
schema_migration_checksum
asset_manifest_hash
config_version
deployed_at
deployed_by
environment
```

Git Tag 不能代替 Release Manifest。

---

# 20. Container / Supply-chain Boundary

Chaldea 自有 Container 默认：

```text
non-root
read-only root filesystem where possible
cap_drop: ALL
no-new-privileges
no privileged mode
no host network
no Docker socket
read-only secret mounts
tmpfs where suitable
```

Frontend Static Server 不进入 DB Network；Platform / Poker 只加入真正需要的 Network。

Production Image 最终锁到 Immutable Digest。

---

# 21. Foundation CI Gate

CI Provider 暂不与 GitHub / GitLab 等平台绑定。先冻结平台无关 CI Command Contract。

至少覆盖：

```text
toolchain verification
dependency lock verification
frontend typecheck
frontend production build
backend go vet
backend tests
backend build
poker go vet
poker tests
poker build
module isolation check
generated-file ownership check
secret scan
dependency vulnerability scan
container image scan
SBOM generation
migration checksum verification
frontend asset / rights gate
```

每 Release 产生：

```text
Frontend dependency SBOM
Platform Go module SBOM
Poker Go module SBOM
Container base image SBOM
```

---

# 22. NewAPI Verification Slot

Repository：

```text
integration/newapi/source_verification_register.md
integration/newapi/deployment_facts.md
```

IS-02 必须记录真实 NewAPI Version / Commit or Image Digest / Compose / Service Names / DB / Redis / Volumes / Public Routes / Auth / API Key / Quota / Logs / Admin 能力。

当前：

```text
SV-01 ～ SV-16
= BLOCKED_BY_NEWAPI_SOURCE_VERIFY
```

Source Verification 只允许确定 Adapter 实现方式，不允许修改 Chaldea 已冻结业务语义。

---

# 23. Deployment Verification Slot

Repository：

```text
deployment/verification/DEPLOYMENT-VERIFY-01.md
```

至少核验：

```text
Actual VPS OS
CPU / RAM
Swap
Disk
Current Edge Proxy
Nginx or Caddy
DNS
Certificate
80 ownership
443 ownership
Current Docker networks
Current examples/deployment/external-newapi
Current NewAPI Compose topology
Current PostgreSQL topology
Current Redis topology
Published ports
Existing backup paths
```

当前：

```text
DEPLOYMENT-VERIFY-01 = PENDING
```

IS-01 不决定实际 Edge 是 Nginx 还是 Caddy。

---

# 24. Art / Asset Foundation Boundary

Frontend 保留 `design-system/`、`media/`、`styles/`。

字体体系：

```text
IBM Plex Sans SC
IBM Plex Mono
Marcellus
Noto Serif SC
```

Production Self-host，优先 WOFF2。媒体与真实 DOM/CSS/SVG UI 永久分层。

IS-01 不把 Persona Prompt 路径或旧生成资产路径硬编码进 Frontend Bundle。

---

# 25. Foundation Error / Recovery Boundary

```text
unknown environment                 → FAIL
missing required production secret → FAIL
toolchain mismatch                 → FAIL
lockfile mismatch                  → FAIL
dirty production source tree       → FAIL
generated contract stale           → FAIL
cross-module Go import detected     → FAIL
```

`NewAPI Source Verification pending` 不是 Repository Build Failure，而是 Adapter / Production Capability Blocker。

禁止自动下载 arbitrary latest、生成 Production Secret、猜 NewAPI Field、`npm install latest` 或自动修改 Migration 来“自愈”。

---

# 26. Repository Audit Boundary

Git 可以记录 Config Key Name、Secret File Name、Schema、Hash、Build ID、Dependency Version、Fixture Identity。

Git 永不记录 Real DB/Redis Password、Discord OAuth Secret、NewAPI Credential、Private Signing Key、Fairness Encryption Key、Backup Credential、API Key Secret、Production User Data、Private Prompt/Response 或 Poker Private Card Data。

---

# 27. Codex Implementation Order — IS-01

```text
01 Create repository skeleton
02 Add toolchain.lock
03 Add Go workspace
04 Add backend Go module
05 Add poker Go module
06 Add frontend package manifest / lock
07 Add frontend base source tree
08 Add contract directories
09 Add database/deployment/runbook specification directories
10 Add config boundary
11 Add secret manifest boundary
12 Add implementation-config register
13 Add build scripts / Makefile
14 Add foundation checks
15 Add module-isolation checks
16 Add build metadata generation
17 Add release-manifest schema
18 Run foundation verification
19 Produce evidence
20 Stop
```

不得顺手继续实现 Auth、Wallet、Rewards、Game Math、Poker Rules、NewAPI SQL、Production Migration 或 Production Deployment。

---

# 28. Codex May / Must Not

## May Create / Modify

```text
Repository directories
module manifests
package manifests
lockfiles
empty package boundaries
buildinfo
typed config infrastructure
build scripts
CI-neutral verification scripts
README / boundary docs
contract directory skeleton
release manifest schema
register files
foundation tests
minimal compileable mains/bootstrap
```

## Must Not

```text
修改 examples/deployment/external-newapi
修改 NewAPI Source
猜 NewAPI Endpoint / Table / Column
创建正式 Economy / Reward / Poker Migration
实现 Reward OPEN Default
实现 Poker Product Gap Default
创建 Production Secret
把 Secret 放 .env / Compose YAML / Frontend
使用 latest Dependency / Container Image
建立 backend ↔ poker 内部 Go import
增加 Kubernetes / Kafka / Service Mesh
增加第二套 Frontend
增加 SSR / Next.js
开始 Production Deployment
```

---

# 29. IS-01 Acceptance Criteria

```text
AC-01 Repository tree == approved IS-01 tree.
AC-02 Go 1.27.1 / Node 24.20.0 / npm 11.19.0 toolchain verification passes.
AC-03 backend and poker both build with GOWORK=off.
AC-04 backend does not import poker module; poker does not import platform module.
AC-05 frontend npm ci succeeds with committed package-lock.
AC-06 frontend typecheck succeeds.
AC-07 frontend production build succeeds.
AC-08 all Direct Dependency versions are exact / locked.
AC-09 no floating latest dependency/image exists.
AC-10 no Production Secret exists in Git tracked files.
AC-11 production secret path contract uses read-only mounted files.
AC-12 DEVELOPMENT / STAGING / PRODUCTION are the only deployable Environment identities.
AC-13 Implementation Config Register contains unresolved tuning values without invented defaults.
AC-14 SV-01～16 remain explicit BLOCKED if NewAPI source has not been verified.
AC-15 DEPLOYMENT-VERIFY-01 remains explicit PENDING until actual VPS inspection.
AC-16 contracts canonical directories exist and generated directories are Generator-owned.
AC-17 database/migrations contains no invented production migrations.
AC-18 Release Manifest schema contains TD-12 mandatory identity/hash/version fields.
AC-19 Foundation CI verifies lockfiles, module isolation, secrets and build reproducibility.
AC-20 No Production deployment occurs.
```

---

# 30. IS-01 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| IS-FRZ-001 | Chaldea V1 使用单一 Chaldea Monorepo；NewAPI 源码与 Deployment 保持外部独立，不 Vendor / Fork 进 Chaldea Repo。 | FROZEN |
| IS-FRZ-002 | Repository Root 采用 IS-01 已定义的 `frontend/backend/poker/contracts/database/integration/deployment/operations/backup/testing/scripts/docs` 物理目录。 | FROZEN |
| IS-FRZ-003 | Frontend 物理目录严格实现 TD-11 的 App/Routes/Features/Realtime/Design-System/Media/Generated/Shared/Styles 分层。 | FROZEN |
| IS-FRZ-004 | Platform 与 Poker 使用两个独立 Go Module，Root `go.work` 只作为开发 Workspace。 | FROZEN |
| IS-FRZ-005 | Go Module identity 使用 `chaldea.internal/platform` 与 `chaldea.internal/poker`。 | FROZEN |
| IS-FRZ-006 | Production Build 对两个 Go Module 使用 `GOWORK=off`；Platform/Poker 禁止跨服务 internal Go import。 | FROZEN |
| IS-FRZ-007 | Platform Backend 采用 Domain-owned Modular Monolith Package Boundary；`foundation` 不承载业务所有权。 | FROZEN |
| IS-FRZ-008 | Poker 使用独立 Actor/Table/Session/Hand/Settlement/Recovery/Fairness/Persistence 等 Package Boundary；Package 存在不代表 OPEN Poker Rules 已默认解决。 | FROZEN |
| IS-FRZ-009 | Machine-readable Contract 使用 `contracts/openapi/chaldea-bff-v1.yaml` 与独立 `contracts/poker-ws/*`，Generated 目录由 Generator 独占写入。 | FROZEN |
| IS-FRZ-010 | `database/migrations` 与 Compose 目录在 IS-01 只建立边界，不生成生产 Migration 或猜测 Production Compose。 | FROZEN |
| IS-FRZ-011 | Initial Toolchain Baseline 为 Go 1.27.1、Node 24.20.0 LTS、npm 11.19.0。 | FROZEN |
| IS-FRZ-012 | Initial Frontend Baseline 为 React 19.2.7、TypeScript 6.0.3、Vite 8.2.0。 | FROZEN |
| IS-FRZ-013 | Router 采用 React Router 8.3.0，HTTP Server Cache 采用 TanStack React Query 5.102.2；不启用 SSR/Framework Server 架构。 | FROZEN |
| IS-FRZ-014 | UUIDv7 使用 Go 1.27 标准库 `uuid.NewV7()`，V1 不为 UUID 引入第三方 Runtime Dependency。 | FROZEN |
| IS-FRZ-015 | Frontend 使用 npm + committed `package-lock.json`；Direct Dependencies 使用 exact versions，CI 使用 `npm ci`。 | FROZEN |
| IS-FRZ-016 | Go 使用独立 `go.mod/go.sum`；Container 使用 pinned digest；Repository 永久禁止 Floating `latest` / Runtime `curl\|sh`。 | FROZEN |
| IS-FRZ-017 | Root Makefile 固定 `doctor/bootstrap/fmt/fmt-check/static-check/test/build/verify/clean` Foundation Command Contract。 | FROZEN |
| IS-FRZ-018 | Build Artifact 固定输出到 Git-ignored `dist/`，并携带 Git/Toolchain/Lock/Build Metadata。 | FROZEN |
| IS-FRZ-019 | Deployable Environment 只允许 DEVELOPMENT / STAGING / PRODUCTION；不新增正式 TEST Environment。 | FROZEN |
| IS-FRZ-020 | Backend/Poker 只有各自 `internal/config` 可读取 Runtime Environment / Secret File；Domain 不直接访问 Process Environment。 | FROZEN |
| IS-FRZ-021 | Frontend 只允许公开 WEB_ORIGIN/API_ORIGIN/POKER_WS_ORIGIN/BUILD_ID/ENVIRONMENT；任何 Secret 永不进入 Vite Environment。 | FROZEN |
| IS-FRZ-022 | Production Secret 使用 `examples/deployment/platform/secrets` 0700 Root Directory + 0600 Files，并只读挂载至 `examples/runtime/secrets/*`。 | SUPERSEDED — superseded by IS-FRZ-561 |
| IS-FRZ-023 | IS-01 建立明确 Secret Name Manifest；NewAPI Adapter/Cutover Credential 格式继续 `BLOCKED_BY_NEWAPI_SOURCE_VERIFY`。 | FROZEN |
| IS-FRZ-024 | TD-13 Implementation-only Config 全部进入显式 Register；未核验值状态为 `UNRESOLVED_IMPLEMENTATION_CONFIG`，不得生成假默认值。 | FROZEN |
| IS-FRZ-025 | Production Release 必须具有 Immutable Release Manifest、Dependency Lock、SBOM、Scan 与 Image Digest；具体 Runtime Cap 等仍由 Load Test 锁定。 | FROZEN |
| IS-FRZ-026 | NewAPI Source 与真实 VPS 未提供时，分别保持 `SV-01～16 BLOCKED` 与 `DEPLOYMENT-VERIFY-01 PENDING`；IS-01 不猜事实。 | FROZEN |
| IS-FRZ-027 | IS-01 只允许 Foundation Skeleton / Build / Config / Contract Boundary；禁止提前实现正式 Domain、生产 Migration、NewAPI 修改或 Production Deployment。 | FROZEN |

---

# 31. Open / Blocked Register after IS-01

```text
Product Decision Blocker:
- Reward OPEN
- Poker Product Gap 01～05
- Public Record Selection Policy

NewAPI Source Verification:
- SV-01 ～ SV-16
- Status = BLOCKED_BY_NEWAPI_SOURCE_VERIFY

Deployment Verification:
- DEPLOYMENT-VERIFY-01
- Status = PENDING

Implementation Configuration:
- Status = UNRESOLVED_IMPLEMENTATION_CONFIG

Production Readiness:
- Rights / Final Assets
- Accessibility
- Performance / Load
- Security / Supply-chain final scan
- Backup / Restore Drill
- Full DR Verification
- Actual VPS Verification
```

---

# 32. Change Log

## WORKING v0.1 — IS-01

### Added

- 正式创建 `Chaldea Platform — Implementation Spec v1.0 WORKING` 累计实施规格；
- 用户批准 `IS-01 — Repository / Build / Dependency / Environment Baseline`；
- 冻结 `IS-FRZ-001 ～ IS-FRZ-027`；
- 冻结 Monorepo、Repository Tree、Go Module、Package Boundary、Contract Directory、Toolchain、Dependency Lock、Build、Environment、Config、Secret、Release Manifest、Foundation CI、SV Slot、Deployment Verify Slot、Acceptance 与 Codex Boundary。

### Preserved

```text
TD-FRZ-001 ～ TD-FRZ-552
Reward OPEN
Poker Product Gap 01～05
Public Record Selection Policy
SV-01 ～ SV-16 unresolved facts
DEPLOYMENT-VERIFY-01 unresolved facts
Production Readiness gates
```

### Supersession

```text
Existing IS-FRZ superseded:
None
```

---

# 33. Next Batch

> **IS-02 — NewAPI Source Verification & Adapter Specification**

优先核验：

```text
SV-01 Session / Authentication
SV-02 Password Login Identifier
SV-03 Password Set / Change / Reset
SV-04 Discord Binding
SV-05 NewAPI User / Key / Log ID Types
SV-06 API Key Native Operations
SV-07 NewAPI Admin Permission Detection
SV-08 Cutover Write Freeze
SV-09 Raw Quota Read / Reset / Delta Mutation
SV-10 Quota Mutation Idempotency
SV-11 Active Quota Reactive Refill Hook
SV-12 API Request Attribution / Logs
SV-13 Chaldea → NewAPI Authentication
SV-14 NewAPI Redis Authentication / Namespace / ACL
SV-15 NewAPI Persistent Volume / Backup Scope
SV-16 Public NewAPI Model API Route Allowlist
```

如果真实 NewAPI Source / Commit / Compose 尚未提供：

```text
Verification Procedure / Evidence Template
→ 可以冻结

Actual NewAPI Facts
→ BLOCKED_BY_NEWAPI_SOURCE_VERIFY

Adapter Concrete Endpoint/Table/Column
→ 不得猜测
```


---

# 34. IS-02 — NewAPI Source Verification & Adapter Specification

> 状态：`FROZEN / SPECIFICATION COMPLETE / SOURCE FACTS BLOCKED`  
> 用户确认：`整体按上述 IS-02 方案通过`  
> Frozen Decision Range：`IS-FRZ-028 ～ IS-FRZ-062`  
> Source Verification：`SV-01 ～ SV-16 = BLOCKED_BY_NEWAPI_SOURCE_VERIFY`

## 34.1 Purpose

IS-02 冻结 NewAPI 源码核验方法、证据格式、Compatibility Manifest、Adapter 抽象 Contract、Capability Matrix、实现路径选择规则、错误与重试边界、升级失效规则、测试 Gate 与 Codex 实施边界。

本批不猜实际 NewAPI Endpoint / Table / Column / Redis Key / Credential Format。

必须永久区分：

```text
A. 当前实际 NewAPI 是怎样实现的
→ Source Fact / Verification Evidence

B. Chaldea 如何使用已经证明存在的能力
→ Implementation Decision / Adapter Mapping
```

README、GitHub Latest、其他安装实例、模型记忆均不能单独成为 Production Source Fact。

---

# 35. Upstream Traceability

IS-02 主要落实：

```text
TD-FRZ-006      NewAPI Adapter precedence / no arbitrary runtime DB write
TD-FRZ-023～026 Discord registration / password / identifier verification
TD-FRZ-047      newapi_user_id actual type requires source verification
TD-FRZ-053～055 Discord binding / API Key purpose / request attribution
TD-FRZ-440～496 Security / Secret / Deployment / DR boundary
TD-FRZ-479      NewAPI non-DB persistent backup scope verification
TD-FRZ-497      Initial Super Admin target user verification
TD-FRZ-540      SV-01 ～ SV-16 inventory
TD-FRZ-541      Source Verification may choose adapter implementation only
```

Source Verification 不允许静默改变 Chaldea 已冻结的身份、资产、Key Purpose、权限、历史或安全语义。

---

# 36. IS-02 Completion Model

当前没有用户实际 NewAPI Source / Commit / Compose，因此 IS-02 分两层：

```text
IS-02 Specification Layer
→ FROZEN

Actual Source Fact Layer
→ BLOCKED_BY_NEWAPI_SOURCE_VERIFY
```

当前可以冻结：

```text
Verification Procedure
Evidence Contract
Adapter Abstract Contract
Capability Selection Rules
Failure / Security Boundary
Upgrade Invalidation Rules
Test Gate
```

当前不能伪造：

```text
SV-01～16 Concrete Facts
Concrete Endpoint Mapping
Concrete Table/Column Mapping
Concrete Redis Key Mapping
Concrete Credential Format
```

如果未来 Source Fact 与已冻结 TD / IS 冲突，进入 Versioned Technical Change Proposal；不得为了适配当前源码而静默改写 Chaldea。

---

# 37. Verification Identity

每次核验建立：

```text
verification_id = UUIDv7
```

并绑定明确 NewAPI Deployment Identity。

至少尝试记录：

```text
verification_id
deployment_environment

newapi_version_display
git_remote
git_commit

image_repository
image_tag
image_digest

compose_project
compose_service

source_tree_hash / commit
database_schema_fingerprint
migration_state

verified_at
verified_by
```

真实信息不存在或部署不暴露时记录：

```text
NOT_EXPOSED
```

不得填推测值。

---

# 38. Version / Deployment Matching

以下单独出现均不足以证明当前实际运行版本：

```text
version display
README version
GitHub latest release
Docker tag latest
Web UI footer version
```

Production Verification 优先建立：

```text
Running Deployment
↕
Immutable Image Digest
↕
Exact Source / Git Commit
```

若提供的 Source 与实际 Running Image 无法建立对应：

```text
SOURCE_DEPLOYMENT_MATCH = UNPROVEN
```

源码分析只能标：

```text
PARTIAL_EVIDENCE
```

不能标：

```text
VERIFIED_FOR_PRODUCTION
```

---

# 39. Evidence Authority Hierarchy

正式证据优先级：

```text
1. Exact deployed source / image-corresponding source
2. Actual Compose / immutable image digest / runtime config structure
3. Actual DB schema / constraints / indexes / grants
4. Actual router / handler / service / model source
5. Safe runtime probe against matching deployment
6. Official upstream release notes / docs
7. README / examples
```

第 6～7 项只能 support，不得覆盖实际部署证据。

---

# 40. Verification Repository Files

冻结：

```text
integration/newapi/
├── source_verification_register.md
├── deployment_facts.md
├── compatibility_manifest.yaml
├── adapter_mapping.md
└── evidence_index.json
```

## 40.1 source_verification_register.md

保存：

```text
SV-01 ～ SV-16
verification result
evidence
implementation path
remaining blocker
```

## 40.2 deployment_facts.md

保存经脱敏事实：

```text
Version
Commit
Image Digest
Compose topology
Service identity
DB identity
Redis topology
Volume topology
Public routes
```

## 40.3 compatibility_manifest.yaml

机器可读保存 Source / Deployment identity、SV 状态与 Compatibility 状态。

## 40.4 adapter_mapping.md

保存：

```text
Chaldea Capability
→ Verified NewAPI Capability
→ Implementation Path
→ Exact Evidence
```

## 40.5 evidence_index.json

只保存证据索引 / Hash，不保存 Secret。

---

# 41. Raw Evidence Security Boundary

Raw Verification Evidence 默认不进入 Git。

可能包含：

```text
Compose
Environment
docker inspect
DB metadata
runtime response
config
source paths
```

Git 只保存：

```text
sanitized facts
hashes
source file/function locations
non-secret schema
redacted output
```

禁止提交：

```text
DB password
Redis password
OAuth secret
API key
NewAPI admin token
Cookie / Session
private signing material
real user credential
```

---

# 42. Verification Mutation Boundary

Production 核验默认：

```text
READ-ONLY / NON-MUTATING
```

允许：

```text
read source
read compose
inspect image/network/volume metadata
read schema/index/grant
safe GET/read endpoint
```

默认禁止为了核验直接：

```text
change password
reset quota
create/delete API key
rebind Discord
freeze production writes
modify Redis
perform cutover
```

Write Capability 核验优先：

```text
Source Inspection
→ Staging / Disposable Fixture
→ Controlled Test Environment
```

Production Mutation 只能由对应 Cutover / Deployment Runbook 执行。

---

# 43. Source Verification State Model

每个 SV 独立状态：

```text
BLOCKED
→ COLLECTING
→ EVIDENCE_READY
→ VERIFIED
```

附加状态：

```text
VERIFIED_ABSENT
CONFLICT
INVALIDATED
```

`VERIFIED_ABSENT` 表示已经证明当前版本没有 Native Capability，不是核验失败。

随后选择：

```text
NARROW_INTERNAL_BRIDGE
READ_ONLY_DB
DEPLOYMENT_PROCEDURE
TECH_CHANGE_REQUIRED
```

`INVALIDATED` 用于 NewAPI Version / Commit / Image / Schema 变化后原证据失效。

---

# 44. Implementation Path Enum

每项 SV 必须独立记录：

```text
implementation_path
```

只允许：

```text
NATIVE_API
NATIVE_INTERNAL_API
NARROW_INTERNAL_BRIDGE
READ_ONLY_DB
DEPLOYMENT_PROCEDURE
NOT_REQUIRED
TECH_CHANGE_REQUIRED
```

普通 Runtime 禁止：

```text
DIRECT_RUNTIME_DB_WRITE
```

---

# 45. Adapter Selection Rule

Runtime Integration 顺序固定：

```text
Verified Native API
        ↓
Verified Native Internal API
        ↓
Narrow Internal Bridge
        ↓
Minimum Read-only DB
```

Read-only DB 只允许真正读取领域。

不得用于：

```text
verify password locally
copy password hash
change password
change quota
change Discord binding
create/delete Key
change account status
```

Cutover 的临时 Migration Credential 属独立维护场景，不等于 Runtime DB Write。

---

# 46. Runtime Auto-discovery Prohibited

Chaldea Runtime 禁止：

```text
startup
→ crawl NewAPI routes
→ guess endpoint
→ inspect table
→ dynamically choose implementation
```

也禁止失败后试探多个可能接口：

```text
/api/foo
/api/bar
/api/user
/api/admin
...
```

Production Adapter 必须使用 Source-verified Code-owned Mapping。

部署与 Compatibility Manifest 不匹配时：

```text
dependency = DEGRADED / INCOMPATIBLE
```

不得自动重新猜接口。

---

# 47. Adapter Source Structure

冻结：

```text
backend/internal/adapters/newapi/
├── adapter.go
├── capabilities.go
├── external_ids.go
├── errors.go
├── auth.go
├── accounts.go
├── discord.go
├── apikeys.go
├── quota.go
├── usage.go
├── admin.go
└── implementation/
```

`implementation/` 只有 Source Verification 后才允许出现真正选择的：

```text
nativeapi/
internalapi/
bridge/
readonlydb/
```

不要求四类全部存在。

---

# 48. Capability-specific Ports

禁止 God Adapter：

```go
type NewAPIClient interface {
    DoAnything(...)
}
```

Domain 只依赖最小能力 Port：

```text
Auth        → AuthPort
Identity    → AccountPort / DiscordBindingPort
API Keys    → APIKeyPort
Economy     → QuotaPort
Usage       → UsageAttributionPort
Operations  → AdminDetectionPort
```

---

# 49. Capability Matrix

至少覆盖：

```text
PASSWORD_VERIFY
PASSWORD_IDENTIFIER_READ
PASSWORD_SET
PASSWORD_CHANGE
PASSWORD_RESET

ACCOUNT_READ
ACCOUNT_STATUS_READ

DISCORD_BINDING_LOOKUP
DISCORD_BINDING_ADMIN_REBIND

API_KEY_LIST
API_KEY_CREATE
API_KEY_ENABLE
API_KEY_DISABLE
API_KEY_DELETE
API_KEY_REVEAL

NEWAPI_ADMIN_DETECT

CUTOVER_REGISTRATION_FREEZE
CUTOVER_RELEVANT_WRITE_FREEZE

RAW_QUOTA_READ
RAW_QUOTA_RESET
RAW_QUOTA_DELTA
RAW_QUOTA_NATIVE_IDEMPOTENCY

ACTIVE_QUOTA_REFILL_TRIGGER
REQUEST_ATTRIBUTION_READ
NEWAPI_INTERNAL_AUTH
NEWAPI_REDIS_COMPATIBILITY
NEWAPI_BACKUP_NONDB_SCOPE
PUBLIC_MODEL_API_ALLOWLIST
```

Capability 是否存在只能由 Source Verification 决定。

禁止通过普通 Env Bool 伪造：

```text
NEWAPI_HAS_X=true
```

---

# 50. External ID Boundary

在 SV-05 完成前：

```text
newapi_user_id
newapi_key_id
newapi_log_id
model_id
```

统一视为：

```text
opaque string identity
```

禁止假定整数。

即使实际 DB 为整数，BFF JSON 仍统一 String。

---

# 51. SV-01 — Session / Authentication

核验：

```text
password verification capability
session/auth lifetime
other-session revoke ability
account-status interaction
```

Chaldea Password Login 必须最终得到：

```text
Verify Password Proof
→ stable newapi_user_id
→ account state
```

禁止复制 Password DB / Hash。

Dependency unavailable：

```text
DEPENDENCY_UNAVAILABLE
```

不得映射为 INVALID PASSWORD。

---

# 52. SV-02 — Password Login Identifier

必须核验：

```text
OAuth-created stable identifier
actual login field
type
normalization
mutability
uniqueness
```

继续保证：

```text
Password Login Identifier
!= Master Nickname
!= Chaldea Short Account ID
```

若 Native Capability 不存在：

```text
VERIFIED_ABSENT
→ controlled adapter / provisioning analysis
```

Frontend 不得造假标识。

---

# 53. SV-03 — Password Set / Change / Reset

三种能力必须分别核验：

```text
SET
CHANGE
RESET
```

确认：

```text
current password verification
password policy
reset authorization
hash ownership
error behavior
```

缺少安全 Native Capability 时优先 Narrow Internal Bridge；禁止 SQL 修改 Password Hash。

---

# 54. SV-04 — Discord Binding

核验：

```text
binding storage
Discord User ID type
newapi_user_id relation
one-to-one uniqueness
existing-binding lookup
admin rebind capability
```

Source Verification 不改变：

```text
Discord Role only first registration
Existing Binding Pre-check first
one Discord ↔ one account
```

---

# 55. SV-05 — Identity Types

真实记录：

```text
User ID
API Key ID
Log ID
Model ID
Discord ID

DB type
API representation
nullable
stable
mutable
```

BFF 外部仍为 String。

---

# 56. SV-06 — API Key Native Operations

逐项核验：

```text
List
Create
Enable
Disable
Delete
Reveal
```

Create 还必须确认：

```text
request fields
returned metadata
secret return timing
one-time secret behavior
later recoverability
```

如果 Secret 只在创建时返回一次：

```text
Create
→ return once to current browser response
→ do not persist second copy
→ do not log/audit secret
```

创建响应丢失但 Key 已存在：

```text
Secret unrecoverable
→ revoke/delete
→ create new key
```

Reveal 只有 SV-06 明确验证存在时才开放。

---

# 57. SV-07 — NewAPI Admin Detection

只核验：

```text
actual admin role / permission
actual NewAPI Admin access
```

NewAPI Admin 与 Chaldea Operations RBAC 永久独立，不互相自动授权。

---

# 58. SV-08 — Cutover Write Freeze

必须核验 Cutover 时如何阻止：

```text
new registration
quota charging / relevant NewAPI writes
```

实际实现可能来自：

```text
deployment procedure
verified NewAPI maintenance mechanism
edge admission block
narrow bridge
other verified mechanism
```

当前不预选。

Cutover Freeze 是受控部署/迁移能力，不创建普通公开 Generic Freeze Endpoint。

---

# 59. SV-09 — Raw Quota

核验：

```text
read current raw quota
reset/set zero
apply delta
read before/after
integer semantics
failure semantics
actual field/type
overflow behavior
account restriction behavior
```

禁止猜字段后直接 SQL UPDATE。

---

# 60. SV-10 — Quota Idempotency

必须回答：

```text
Does NewAPI natively support idempotent quota mutation?
```

若 YES：记录 Native Idempotency Identity / Duplicate / Conflict / Unknown-result Recovery。

若 NO：采用已冻结方向：

```text
Quota Bridge
+
NewAPI-local operation journal
```

Journal Schema / Economy Saga 进入 IS-05。

---

# 61. QuotaPort Contract

稳定抽象：

```text
ReadRawQuota(user_id)

ResetRawQuota(
    operation_id,
    user_id,
    expected_context
)

ApplyRawQuotaDelta(
    operation_id,
    user_id,
    delta_atomic
)

QueryQuotaOperation(operation_id)
```

所有 Mutation 强制 stable operation_id。

网络结果未知：Query original operation。

---

# 62. SV-11 — Reactive Refill Hook

核验 NewAPI：

```text
request admission
quota check
insufficient quota path
charge lifecycle
```

然后选择真实接入点：

```text
pre-admission hook
insufficient-quota hook
controlled bridge
other verified point
```

禁止轮询猜测式 Reserve→Active Refill。

---

# 63. SV-12 — API Request Attribution

核验 Log 能否稳定提供：

```text
logical request identity
newapi_user_id
API key identity
requested model
status
final charge
timestamp
```

并核验 Internal Provider Retry 与 Client Logical Retry 是否可区分。

如果现有 Log 不足，只增加最小 Attribution Hook；不得因此保存 Prompt / Response / Full Request Body / Provider Secret。

`key_purpose_snapshot` 必须在请求发生时固化。

---

# 64. SV-13 — Chaldea → NewAPI Authentication

核验：

```text
native internal API credential
existing auth mechanism
narrow bridge authentication
least privilege
```

不能假定支持 Platform↔Poker Ed25519 Service Assertion。

最终只能选择经过核验的：

```text
Verified API Credential
or
Verified Narrow Bridge Authentication
```

Credential 仍按 IS-01 Secret File 注入。

---

# 65. SV-14 — NewAPI Redis

核验：

```text
Redis instance
authentication
database number if applicable
key namespace/prefix
ACL support
persistence
runtime dependence
```

目标是在真实兼容的前提下形成 NewAPI / Chaldea Namespace/ACL 隔离。

Chaldea 不扫描、删除、接管未知 `newapi:*` Key。

---

# 66. SV-15 — Persistent Volume / Backup

核验 NewAPI 除 PostgreSQL 外是否存在不可重建数据：

```text
uploaded file
generated credential
local config
key material
runtime state
SQLite
model metadata
other persistent volume
```

无法由 Git + DB + Recovery Kit 重建的内容必须进入 DR Scope。

如果没有，也要用证据记录：

```text
NO_NONRECONSTRUCTIBLE_VOLUME_FOUND
```

---

# 67. SV-16 — Public API Route Allowlist

从实际 Router / Handler 确定 `api.chaldea.example.com` 可以公开的正式 Model API Path。

Edge：

```text
verified model API path
→ NewAPI

everything else
→ reject / safe handling
```

禁止公开整个 NewAPI Web/Admin/Internal/Debug Surface。

---

# 68. Adapter Error Boundary

NewAPI-specific transport/error 统一标准化为内部 Error Class：

```text
INVALID_PROOF
ACCOUNT_RESTRICTED
NOT_FOUND
CONFLICT
RATE_LIMITED
DEPENDENCY_UNAVAILABLE
CAPABILITY_UNAVAILABLE
REMOTE_RESULT_UNKNOWN
SOURCE_INCOMPATIBLE
```

对应 BFF Error Code 由 Domain Contract 再映射。

必须保证：

```text
timeout during password login
!= invalid credentials

timeout during quota mutation
!= mutation failed

404 from internal NewAPI path
!= user not found automatically
```

---

# 69. Retry Policy

Read 可以基于 Implementation Config 使用有限 retry + backoff，只针对已分类瞬时错误。

Mutation：

```text
NO GENERIC AUTOMATIC RETRY
```

Native Idempotency 存在时复用同一 operation identity。

无 Native Idempotency 时由 Bridge Journal 查询 / 恢复。

---

# 70. Dependency Degradation

只降级依赖对应 NewAPI Capability 的功能。

例如：

```text
Password Auth unavailable
→ Password Login unavailable
→ independent Discord Login may remain

API Key capability unavailable
→ API Key management degraded
→ Games not automatically closed

Usage attribution unavailable
→ Usage / Ranking lag or degraded
→ Wallet authority unchanged

Model API unavailable
→ API domain unavailable
→ accepted Poker Hand not discarded
```

保持 Partial Degradation。

---

# 71. Compatibility Manifest

最小结构：

```yaml
verification_id: ""
verified_at: ""

source:
  git_commit: ""
  image_digest: ""

deployment:
  compose_project: ""
  compose_service: ""

compatibility_status: BLOCKED

sv:
  SV-01:
    status: BLOCKED
    implementation_path: null
```

继续完整到 SV-16。

Manifest 禁止保存 Secret。

---

# 72. Upgrade Invalidation Rule

以下变化必须评估受影响 SV：

```text
Git commit change
Image digest change
DB migration change
major config topology change
Redis topology change
```

受影响项：

```text
VERIFIED
→ INVALIDATED
→ reverify
```

不要求每次 Patch 机械重做全部 16 项，但必须根据 Source Diff 明确保留和重新核验范围。

Chaldea 不自动执行 NewAPI Upgrade。

---

# 73. Source Facts vs Implementation Decisions

Source Fact 示例：

```text
actual user primary key type
Create Key secret lifecycle
quota native idempotency absent
```

进入 Source Verification Register。

Implementation Decision 示例：

```text
Native quota idempotency absent
→ use Narrow Bridge + Operation Journal
```

后者属于 IS-FRZ。

如果未来 NewAPI 新增 Native Idempotency，不静默替换 Bridge；必须 Versioned IS Change。

---

# 74. Source Verification Register Template

每项统一：

```markdown
## SV-XX — Name

Verification Status:
Implementation Path:
Verification ID:

Deployment Identity:
Source Identity:

Questions:
- ...

Evidence:
- source:
- runtime:
- database:
- deployment:

Verified Facts:
- ...

Absent Capabilities:
- ...

Security Notes:
- ...

Adapter Mapping:
- ...

Blocked By:
- ...

Affected TD-FRZ:
- ...

Verified At:
Verified By:
```

`probably / likely / usually` 不得作为 Verified Fact。

---

# 75. Initial Source Verification State

当前：

```text
SV-01  BLOCKED
SV-02  BLOCKED
SV-03  BLOCKED
SV-04  BLOCKED
SV-05  BLOCKED
SV-06  BLOCKED
SV-07  BLOCKED
SV-08  BLOCKED
SV-09  BLOCKED
SV-10  BLOCKED
SV-11  BLOCKED
SV-12  BLOCKED
SV-13  BLOCKED
SV-14  BLOCKED
SV-15  BLOCKED
SV-16  BLOCKED
```

统一 Blocker：

```text
BLOCKED_BY_NEWAPI_SOURCE_VERIFY
```

---

# 76. Verification Test Environment

会改变数据的 Integration Test 只能使用：

```text
Disposable NewAPI
or
Staging NewAPI
```

Fixture 至少：

```text
normal user
disabled/restricted user
OAuth-created user
user with password
user without password
normal API key
disabled API key
admin account
non-admin account
quota fixture
usage-log fixture
```

全部使用 Demo Identity。

禁止 Production 用户做 Password Reset / Quota Mutation / Key Delete 测试。

---

# 77. Adapter Contract Test Gate

Source Verification 完成后至少测试：

## Authentication

```text
valid credential
invalid credential
dependency unavailable
restricted account
```

## Identifier

```text
OAuth-created account
stable identifier
identifier serialization
```

## Password

只测试 VERIFIED 的 set/change/reset/policy/authorization 能力。

## API Key

```text
list
create
enable
disable
delete
one-time secret
reveal capability gate
```

## Quota

```text
read
reset
delta
same operation replay
unknown remote result
```

## Usage

```text
user attribution
key attribution
model attribution
final charge
logical retry semantics
```

## Security

```text
no password log
no API secret log
no raw credential in audit
no secret in fixture snapshot
```

---

# 78. Source / Deployment Drift Gate

必须检测：

```text
expected commit != actual source
expected image digest != running digest
expected schema fingerprint != actual schema
expected public route mapping != actual edge
```

结果：

```text
NEWAPI_COMPATIBILITY_UNVERIFIED
```

高风险 NewAPI-dependent Release：

```text
BLOCK DEPLOY
```

---

# 79. Initial Super Admin Dependency

Deployment-only Bootstrap 需要：

```text
stable newapi_user_id
→ source-verified user existence lookup
```

SV-05 / Account Read 未完成前：

```text
Bootstrap structure can be prepared
PRODUCTION_READY = false
```

仍然禁止公网 Bootstrap Endpoint。

---

# 80. Codex Implementation Order for IS-02

Implementation Spec FINAL 后执行：

```text
01 Create Verification Register template
02 Create Compatibility Manifest schema
03 Create Evidence Index schema
04 Create Adapter Mapping template

05 Create NewAPI external ID types
06 Create Capability enum/matrix
07 Create normalized adapter error taxonomy

08 Create capability-specific Port interfaces
09 Create blocked/unwired implementation behavior

10 Create Source Verification tooling skeleton
11 Create redaction checks

12 Obtain actual NewAPI source/deployment evidence
13 establish deployment ↔ source identity

14 execute SV-01 ～ SV-16
15 record evidence
16 choose implementation_path per SV
17 implement only verified adapters
18 run adapter contract tests
19 generate compatibility manifest
20 run source/deployment drift gate
```

如果步骤 12 输入不存在：

```text
STOP concrete integration
```

01～11 仍可实施。

---

# 81. Codex May Create / Modify

允许：

```text
integration/newapi/*
backend/internal/adapters/newapi/*

verification templates
capability enums
opaque external ID types
adapter interfaces
safe adapter error types
compatibility manifest schema
evidence index schema
verification scripts
contract tests
redaction tests
```

---

# 82. Codex Must Not

禁止：

```text
guess NewAPI endpoint/table/column/Redis key
copy password hash
store password
store second API-key secret copy
direct Runtime UPDATE NewAPI DB
write generic SQL bridge
write generic admin proxy
expose arbitrary NewAPI route
publish entire NewAPI Web surface
probe random endpoints at runtime
automatically retry unknown quota mutation
assume NewAPI Admin == Chaldea Admin
mutate Production data merely for verification
change Product OPEN
change asset semantics
change IA
change Poker rules
```

---

# 83. IS-02 Acceptance Criteria

## 83.1 Specification Acceptance — FROZEN

```text
AC-02-01 SV-01～16 all have exact verification questions.
AC-02-02 Source Verification Register template exists.
AC-02-03 Compatibility Manifest schema exists.
AC-02-04 Evidence Index is secret-safe.
AC-02-05 Adapter implementation precedence is explicit.
AC-02-06 Runtime direct NewAPI DB write is prohibited.
AC-02-07 Capability-specific Ports are specified.
AC-02-08 External NewAPI IDs are opaque strings.
AC-02-09 API Key one-time-secret behavior is safely handled.
AC-02-10 Quota mutation requires stable operation identity.
AC-02-11 Unknown remote result never maps to automatic replay.
AC-02-12 No runtime endpoint discovery exists.
AC-02-13 NewAPI upgrade invalidation rules exist.
AC-02-14 Product/TD decisions cannot be overwritten by source facts.
AC-02-15 All concrete source facts remain BLOCKED without evidence.
```

## 83.2 Source Verification Acceptance — BLOCKED

```text
AC-02-SV-01 Actual deployment identity captured.
AC-02-SV-02 Actual source/image identity captured.
AC-02-SV-03 Source ↔ deployment relation proven.
AC-02-SV-04 SV-01～16 resolved by evidence.
AC-02-SV-05 Adapter mappings reference exact evidence.
AC-02-SV-06 No source-dependent endpoint/table/column remains guessed.
AC-02-SV-07 Contract tests pass against matching disposable/staging NewAPI.
AC-02-SV-08 Compatibility Manifest becomes VERIFIED.
AC-02-SV-09 No Secret appears in committed evidence.
AC-02-SV-10 NewAPI-dependent Production Gate can pass.
```

当前：

```text
BLOCKED_BY_NEWAPI_SOURCE_VERIFY
```

---

# 84. Impact on Later Batches

缺少 NewAPI Source 不阻止继续设计 Chaldea-owned 实施规格。

例如：

```text
IS-03
identity.account_refs external semantic identity
→ can specify
actual NewAPI DB type mapping
→ BLOCKED_BY_SV-05

IS-04
Password Login Chaldea flow
→ can specify
actual NewAPI password endpoint
→ BLOCKED_BY_SV-01 / SV-02

IS-05
Quota Saga
→ can specify
actual mutation transport
→ BLOCKED_BY_SV-09 / SV-10
```

不得因为 Source 未核验就重新设计 Chaldea。

---

# 85. IS-02 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| IS-FRZ-028 | NewAPI Source Verification 必须绑定用户实际部署身份；README、Latest Release 或模型记忆不能单独成为 Production Fact。 | FROZEN |
| IS-FRZ-029 | Verification 建立 Deployment↔Image Digest↔Exact Source/Commit 的追溯；无法匹配时 Source Evidence 只能标 Partial，不能标 Production Verified。 | FROZEN |
| IS-FRZ-030 | IS-02 冻结 Evidence Hierarchy：实际 Deployment/Source/Schema/Runtime 证据优先于文档和示例。 | FROZEN |
| IS-FRZ-031 | 每项 SV 使用 BLOCKED→COLLECTING→EVIDENCE_READY→VERIFIED 状态，并额外支持 VERIFIED_ABSENT / CONFLICT / INVALIDATED。 | FROZEN |
| IS-FRZ-032 | NewAPI Version/Commit/Image/Schema 等相关变化必须评估 SV Invalidation；受影响项未经重新核验不得继续声称兼容。 | FROZEN |
| IS-FRZ-033 | Raw Verification Evidence 默认不进入 Git；Repository 只保存脱敏事实、Hash、证据索引和安全 Source Location。 | FROZEN |
| IS-FRZ-034 | Production Source Verification 默认 Read-only；会修改密码、Quota、Key、Binding、Write Freeze 的验证在 Staging/Disposable Fixture 执行。 | FROZEN |
| IS-FRZ-035 | 每个 SV 独立记录 implementation_path，只允许明确枚举的 Native/Bridge/Read-only/Deployment/Tech-change 类型。 | FROZEN |
| IS-FRZ-036 | Runtime Adapter 路径严格按 Verified Native API → Native Internal API → Narrow Bridge → necessary Read-only DB 优先级选择。 | FROZEN |
| IS-FRZ-037 | Read-only DB 永不用于复制 Password Hash、验证密码或执行 NewAPI Runtime Write；普通 Runtime 不存在 Direct DB Write Path。 | FROZEN |
| IS-FRZ-038 | NewAPI Runtime 禁止 Endpoint/Table 自动发现、Fallback 猜路径或根据失败响应动态试探未知接口。 | FROZEN |
| IS-FRZ-039 | NewAPI Adapter 使用 Capability-specific Port，不建立可任意代理操作的 God Client。 | FROZEN |
| IS-FRZ-040 | newapi_user_id/key_id/log_id/model_id 在 Adapter/BFF Boundary 继续作为 Opaque String；真实底层类型由 SV-05 核验。 | FROZEN |
| IS-FRZ-041 | 建立 Code-owned NewAPI Capability Matrix；Runtime Config 不允许通过布尔开关伪造未验证能力。 | FROZEN |
| IS-FRZ-042 | Password Verification 只由 Source-verified NewAPI Capability 执行；Dependency Failure 与 Invalid Credential 必须严格区分。 | FROZEN |
| IS-FRZ-043 | Password Set / Change / Reset 三种能力分别核验；缺少安全 Native Capability 时优先 Narrow Bridge，禁止 SQL 修改 Hash。 | FROZEN |
| IS-FRZ-044 | Discord Binding 必须核验真实 Storage、Unique Boundary、Existing Binding Lookup 与 Admin Rebind，不改变首次注册 Role 业务规则。 | FROZEN |
| IS-FRZ-045 | API Key Create 必须核验 Secret 生命周期；One-time Secret 模式下 Chaldea 只向创建响应传递一次，永不保存第二份 Secret。 | FROZEN |
| IS-FRZ-046 | API Key Reveal 仅在 SV-06 明确证明 Native Reveal 能力存在时启用；否则 Capability/UI 必须为 Disabled/Absent。 | FROZEN |
| IS-FRZ-047 | NewAPI Admin Detection 只投影真实 NewAPI Admin 能力，不与 Chaldea Super Admin / Operator / Auditor 自动互相授予。 | FROZEN |
| IS-FRZ-048 | Cutover Registration/Charging Write Freeze 的实际机制由 SV-08 决定，属于受控 Deployment/Cutover Procedure，不建立公共 Generic Freeze 后门。 | FROZEN |
| IS-FRZ-049 | QuotaPort 对所有 Raw Quota Mutation 强制 stable operation_id；网络结果未知时查询原 Operation，不生成新操作盲重试。 | FROZEN |
| IS-FRZ-050 | 若 SV-10 证明 NewAPI 无 Native Quota Idempotency，则采用 Narrow Quota Bridge + NewAPI-local Operation Journal 分支；Journal 细节进入 IS-05。 | FROZEN |
| IS-FRZ-051 | Active Quota Refill Hook 只能基于 SV-11 证明的 Request Admission / Insufficient-quota Hook 等真实接入点，禁止轮询猜测式补额度。 | FROZEN |
| IS-FRZ-052 | API Request Attribution 优先复用 NewAPI 真实 Log；信息不足时只增加最小 Attribution Hook，不复制 Prompt/Response，并固化请求发生时 key_purpose_snapshot。 | FROZEN |
| IS-FRZ-053 | Chaldea→NewAPI Authentication 必须由 SV-13 决定 Verified Credential / Bridge Auth，并使用最小权限 Secret；不得假设 NewAPI 支持 Poker Service Assertion。 | FROZEN |
| IS-FRZ-054 | SV-14 必须核验 NewAPI Redis Auth/Namespace/ACL；Chaldea 不扫描、删除或接管未知 newapi:* Key。 | FROZEN |
| IS-FRZ-055 | SV-15 必须核验所有 NewAPI non-DB persistent state；不可重建数据必须进入 DR Scope，不能凭假设省略。 | FROZEN |
| IS-FRZ-056 | api.chaldea.example.com 只代理 SV-16 确认的 Model API Allowlist；NewAPI Web/Admin/Internal/Debug Surface 默认拒绝。 | FROZEN |
| IS-FRZ-057 | NewAPI Transport Error 通过 Adapter Error Boundary 标准化；Timeout/Unavailable/Unknown Result 不得被错误映射为 Invalid Credential、Not Found 或 Mutation Failed。 | FROZEN |
| IS-FRZ-058 | Read 可使用有限 Retry/Backoff；NewAPI Mutation 禁止 Generic Automatic Retry，必须复用已验证 Idempotency/Operation Identity。 | FROZEN |
| IS-FRZ-059 | NewAPI Capability 故障使用 Partial Degradation，只关闭依赖该 Capability 的功能，不把 NewAPI 单模块故障升级为全站业务 Authority 变更。 | FROZEN |
| IS-FRZ-060 | Source Fact 与 Implementation Decision 永久分开；实际源码事实更新 Source Register，改变已冻结 Adapter Strategy 时必须通过 Versioned IS Change。 | FROZEN |
| IS-FRZ-061 | NewAPI Integration 必须通过 Verification Evidence、Contract Tests、Secret Redaction、Source/Deployment Drift 与 Compatibility Manifest Gate 后才能 Production Ready。 | FROZEN |
| IS-FRZ-062 | 当前无实际 NewAPI Source 时，SV-01～16 与 Concrete Mapping 继续 BLOCKED_BY_NEWAPI_SOURCE_VERIFY；该 Blocker 不阻止后续 IS 批次设计 Chaldea-owned 规格，但依赖 NewAPI Facts 的字段必须显式保持阻断。 | FROZEN |

---

# 86. Open / Blocked Register after IS-02

```text
Product Decision Blocker:
- Reward OPEN
- Poker Product Gap 01～05
- Public Record Selection Policy

NewAPI Source Verification:
- SV-01 ～ SV-16
- Status = BLOCKED_BY_NEWAPI_SOURCE_VERIFY

Concrete NewAPI Mapping:
- Endpoint/Table/Column/Redis Key/Credential Format
- Status = BLOCKED_BY_NEWAPI_SOURCE_VERIFY

Deployment Verification:
- DEPLOYMENT-VERIFY-01
- Status = PENDING

Implementation Configuration:
- Status = UNRESOLVED_IMPLEMENTATION_CONFIG where not yet evidenced

Production Readiness:
- unchanged / not implied by IS-02 specification freeze
```

---

# 87. Change Log — WORKING v0.2

## Added

- 用户批准 `IS-02 — NewAPI Source Verification & Adapter Specification`；
- 冻结 `IS-FRZ-028 ～ IS-FRZ-062`；
- 冻结 Source Verification Identity / Evidence Hierarchy / Sanitized Evidence Boundary；
- 冻结 `SV-*` 状态模型与 `implementation_path` 枚举；
- 冻结 Runtime Adapter Precedence；
- 冻结 No Runtime Auto-discovery；
- 冻结 Capability-specific Port / Capability Matrix；
- 冻结 Opaque External ID Boundary；
- 冻结 SV-01～16 逐项 Verification Contract；
- 冻结 API Key One-time Secret 处理边界；
- 冻结 QuotaPort / Unknown-result Recovery；
- 冻结 Request Attribution 最小化；
- 冻结 Redis / Persistent Volume / Public Route Verification；
- 冻结 Adapter Error / Retry / Partial Degradation；
- 冻结 Compatibility Manifest / Upgrade Invalidation；
- 冻结 Source Fact 与 Implementation Decision 分离；
- 冻结 Adapter Contract Test / Drift Gate；
- 冻结 IS-02 Codex Implementation Boundary。

## Preserved

```text
TD-FRZ-001 ～ TD-FRZ-552
IS-FRZ-001 ～ IS-FRZ-027
Reward OPEN
Poker Product Gap 01～05
Public Record Selection Policy
SV-01 ～ SV-16 concrete facts unresolved
DEPLOYMENT-VERIFY-01 unresolved
Implementation Config unresolved values
Production Readiness gates
```

## Supersession

```text
Existing IS-FRZ superseded:
None
```

---

# 88. Next Batch

> **IS-03 — PostgreSQL Schema & Migration Specification**

IS-03 将把 Chaldea-owned 数据模型落成可直接生成 Migration 的规格，至少覆盖：

```text
Schema
Table
Column
PostgreSQL Type
Nullable
Default
PK
FK (same DB only)
Unique
Check
Index
Partial Unique
Append-only / Immutable Rules
Owner Role
Runtime Grant
Migration Number / Dependency
Backfill
Verification Query
Rollback / Forward-fix Boundary
```

NewAPI 真实类型、Table、Column 或 Constraint 继续引用对应 `SV-*`，不得在 IS-03 猜测。

---

# 89. IS-03 — PostgreSQL Schema & Migration Specification

> 状态：`FROZEN`  
> 用户确认：`整体按上述 IS-03 方案通过`  
> Frozen Decision Range：`IS-FRZ-063 ～ IS-FRZ-112`  
> NewAPI External ID Type Slot：`BLOCKED_BY_SV-05 / SV-06`

## 89.1 Purpose

IS-03 将 Technical Design 已冻结的数据模型转化为 Codex 可以直接生成 PostgreSQL Migration 的物理规格，冻结：

```text
Schema / Table / Column / Type / Nullable / Default
PK / FK / Unique / Check / Partial Unique / Index
Role / Grant / Object Ownership
Append-only / Immutable Enforcement
Migration Number / Dependency / Checksum / Verification
Forward-fix / Rollback Boundary
```

本批不重新设计 Economy、Reward、Game、Poker、Ranking 或 Operations 业务语义。

---

# 90. PostgreSQL Database Boundary

V1 继续使用：

```text
PostgreSQL Instance
├── newapi
│   └── NewAPI-owned Authority
└── chaldea_platform
    └── Chaldea-owned Authority
```

永久禁止：

```text
cross-database FK
cross-database trigger
business dblink write
business FDW write
PostgreSQL 2PC as cross-authority business transaction
```

NewAPI ↔ Chaldea 跨库一致性继续由 Adapter / Saga / Bridge / Reconciliation 处理。

---

# 91. Canonical Chaldea Schemas

`chaldea_platform` 固定创建：

```text
platform_meta
identity
api
catalog
economy
rewards
games
poker
content
ranking
ops
audit
migration
```

默认安全基线：

```sql
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
REVOKE ALL ON DATABASE chaldea_platform FROM PUBLIC;
```

业务表不得放入 `public`。

---

# 92. Global Physical Type Rules

## 92.1 Chaldea Entity ID

Chaldea 新 Durable Entity：

```text
PostgreSQL type = UUID
Generation = application-generated UUIDv7
```

数据库不依赖 `uuid-ossp` / random UUIDv4 default。

## 92.2 External NewAPI ID Type Slots

```text
@NEWAPI_USER_ID_PG_TYPE
  = BLOCKED_BY_SV-05

@NEWAPI_KEY_ID_PG_TYPE
  = BLOCKED_BY_SV-05 / SV-06
```

在 Source Verification 完成前，禁止把以上 Slot 猜成 `BIGINT` / `TEXT` / `UUID` 并生成最终 Production SQL。

以下 Chaldea Contract 自有 Stable String 不受该 Blocker：

```text
discord_user_id
model_id
game_slug
logical_request_id
biz_id
```

## 92.3 Assets

所有资产、下注、赔付、Stack、Pot、Transfer：

```text
PostgreSQL = BIGINT
column suffix = *_units
```

禁止使用 `REAL / DOUBLE PRECISION / FLOAT` 保存正式资产事实。

当前单位：

```text
1 API Credit = 500,000 atomic units
1 Entertainment Chip = 500,000 atomic units
1 NewAPI raw quota = 1 Chaldea credit atomic unit
```

## 92.4 Time

正式时间：

```text
TIMESTAMPTZ
UTC instant
```

`Asia/Shanghai` 仅用于业务周期和显示。

## 92.5 State Column

V1 不使用 PostgreSQL ENUM。

采用：

```text
TEXT NOT NULL
+
CHECK(...)
```

便于后续 State Machine 版本化。

## 92.6 JSONB Boundary

JSONB 只允许：

```text
metadata
immutable snapshot
safe diagnostic context
versioned config payload
typed job payload
sanitized audit snapshot
```

禁止以一个 JSONB 取代：

```text
Wallet
Ledger
Claim Authority
Game Result Authority
Poker Hand Authority
Settlement / Pot
```

---

# 93. Database Roles / Ownership

## 93.1 `chaldea_owner`

```text
NOLOGIN
```

拥有 Chaldea Schemas / Tables / Views / Functions / Triggers。

Runtime 不使用 Owner Credential。

## 93.2 `chaldea_migrator`

Deployment Migration Job 专用 LOGIN Identity。

```text
NO SUPERUSER
NO CREATEDB
NO CREATEROLE
```

允许受控 `SET ROLE chaldea_owner`。

Runtime Container 不挂载 Migrator Secret。

## 93.3 `chaldea_app`

Platform Backend Runtime。

允许必要 Chaldea DML，无 DDL。

默认不直接任意写 `poker.*`。

## 93.4 `chaldea_poker`

Poker Runtime。

允许：

```text
RW approved poker objects
EXECUTE approved poker funding functions
```

禁止：

```text
direct economy.wallet_balances UPDATE
direct economy.wallet_ledger INSERT
arbitrary economy DML
```

## 93.5 `chaldea_cutover`

定义为 Cutover Permission Role，不作为常驻 Runtime Identity。

正式 Cutover 时由 Deployment Runbook 创建/启用临时 Executor，完成后撤销/关闭。

## 93.6 `chaldea_newapi_ro`

只有 Source Verification 确认确需 DB Read 时启用：

```text
SELECT verified objects only
```

禁止默认 `SELECT ON ALL TABLES`。

---

# 94. `platform_meta` Physical Schema

## `platform_meta.schema_migrations`

```text
version            BIGINT PRIMARY KEY
name               TEXT NOT NULL UNIQUE
checksum_sha256    BYTEA NOT NULL
applied_at         TIMESTAMPTZ NOT NULL
app_build          TEXT NOT NULL
execution_result   TEXT NOT NULL
```

Checks：

```text
octet_length(checksum_sha256) = 32
execution_result = 'APPLIED'
```

Applied Migration Row immutable；checksum mismatch 阻断 Deploy。

## `platform_meta.platform_versions`

```text
platform_version_id UUID PRIMARY KEY
component           TEXT NOT NULL
version             TEXT NOT NULL
build_id            TEXT NOT NULL
git_commit          TEXT NOT NULL
activated_at        TIMESTAMPTZ NOT NULL
retired_at          TIMESTAMPTZ NULL
metadata            JSONB NOT NULL DEFAULT '{}'
```

## `platform_meta.runtime_config_refs`

```text
config_ref_id       UUID PRIMARY KEY
environment         TEXT NOT NULL
config_version      TEXT NOT NULL
config_hash         BYTEA NOT NULL
activated_at        TIMESTAMPTZ NOT NULL
retired_at          TIMESTAMPTZ NULL
```

不得保存 Secret Value。

## `platform_meta.service_states`

```text
service_name        TEXT PRIMARY KEY
build_id            TEXT NOT NULL
last_started_at     TIMESTAMPTZ NOT NULL
last_ready_at       TIMESTAMPTZ NULL
state_version       BIGINT NOT NULL DEFAULT 1
updated_at          TIMESTAMPTZ NOT NULL
```

---

# 95. `identity` Physical Schema

## `identity.account_refs`

```text
newapi_user_id      @NEWAPI_USER_ID_PG_TYPE PRIMARY KEY
security_epoch      BIGINT NOT NULL DEFAULT 0 CHECK(security_epoch >= 0)
created_at          TIMESTAMPTZ NOT NULL
first_seen_at       TIMESTAMPTZ NOT NULL
migration_batch_id  UUID NULL
```

它是 Chaldea User FK Anchor，不保存 Password / Hash / Authoritative Username / NewAPI Account Status。

## `identity.master_profiles`

```text
newapi_user_id             @NEWAPI_USER_ID_PG_TYPE PRIMARY KEY
status                     TEXT NOT NULL
display_name               TEXT NOT NULL
normalized_name            TEXT NOT NULL
current_avatar_snapshot_id UUID NULL
nickname_changed_at        TIMESTAMPTZ NULL
rename_required            BOOLEAN NOT NULL DEFAULT FALSE
profile_version            BIGINT NOT NULL DEFAULT 1
created_at                 TIMESTAMPTZ NOT NULL
updated_at                 TIMESTAMPTZ NOT NULL
completed_at               TIMESTAMPTZ NULL
```

关键约束：

```text
FK newapi_user_id → identity.account_refs ON DELETE RESTRICT
UNIQUE(normalized_name)
```

## `identity.master_profile_name_history`

Append-only：

```text
history_id             UUID PRIMARY KEY
newapi_user_id         @NEWAPI_USER_ID_PG_TYPE NOT NULL
old_display_name       TEXT NULL
new_display_name       TEXT NOT NULL
old_normalized_name    TEXT NULL
new_normalized_name    TEXT NOT NULL
change_type            TEXT NOT NULL
reason                 TEXT NULL
actor_newapi_user_id   @NEWAPI_USER_ID_PG_TYPE NULL
changed_at             TIMESTAMPTZ NOT NULL
```

## `identity.reserved_master_names`

```text
normalized_name TEXT PRIMARY KEY
display_label   TEXT NOT NULL
status          TEXT NOT NULL
reason          TEXT NOT NULL
created_at      TIMESTAMPTZ NOT NULL
released_at     TIMESTAMPTZ NULL
```

## `identity.master_profile_avatar_snapshots`

```text
avatar_snapshot_id     UUID PRIMARY KEY
newapi_user_id         @NEWAPI_USER_ID_PG_TYPE NOT NULL
source                 TEXT NOT NULL
asset_ref              TEXT NOT NULL
provider_snapshot_ref  TEXT NULL
content_hash           BYTEA NULL
created_at             TIMESTAMPTZ NOT NULL
```

Source 仅：

```text
SYSTEM
DISCORD_SNAPSHOT
```

## `identity.identity_display_snapshots`

Immutable：

```text
identity_snapshot_id    UUID PRIMARY KEY
newapi_user_id          @NEWAPI_USER_ID_PG_TYPE NOT NULL
display_name_snapshot   TEXT NOT NULL
avatar_snapshot_id      UUID NOT NULL
created_at              TIMESTAMPTZ NOT NULL
```

## `identity.registration_operations`

```text
registration_operation_id UUID PRIMARY KEY
discord_user_id           TEXT NOT NULL
newapi_user_id            @NEWAPI_USER_ID_PG_TYPE NULL
registration_biz_id       TEXT NOT NULL UNIQUE
state                     TEXT NOT NULL
initial_grant_biz_id      TEXT NULL
failure_code              TEXT NULL
created_at                TIMESTAMPTZ NOT NULL
updated_at                TIMESTAMPTZ NOT NULL
completed_at              TIMESTAMPTZ NULL
```

## `identity.registration_idempotency_records`

```text
idempotency_record_id     UUID PRIMARY KEY
purpose                   TEXT NOT NULL
idempotency_key_hash      BYTEA NOT NULL
payload_hash              BYTEA NOT NULL
registration_operation_id UUID NOT NULL
created_at                TIMESTAMPTZ NOT NULL
UNIQUE(purpose, idempotency_key_hash)
```

Raw Idempotency Key 不持久化。

---

# 96. `api` Physical Schema

## `api.api_key_purpose_metadata`

```text
newapi_key_id       @NEWAPI_KEY_ID_PG_TYPE PRIMARY KEY
newapi_user_id      @NEWAPI_USER_ID_PG_TYPE NOT NULL
current_purpose     TEXT NOT NULL
effective_version   BIGINT NOT NULL
updated_at          TIMESTAMPTZ NOT NULL
```

Purpose：

```text
GENERAL
RP
UNCLASSIFIED
```

不保存 Key Secret。

## `api.api_key_purpose_history`

Append-only：

```text
purpose_history_id  UUID PRIMARY KEY
newapi_key_id       @NEWAPI_KEY_ID_PG_TYPE NOT NULL
newapi_user_id      @NEWAPI_USER_ID_PG_TYPE NOT NULL
old_purpose         TEXT NULL
new_purpose         TEXT NOT NULL
version             BIGINT NOT NULL
effective_at        TIMESTAMPTZ NOT NULL
changed_by          @NEWAPI_USER_ID_PG_TYPE NULL
UNIQUE(newapi_key_id, version)
```

## `api.request_attributions`

```text
logical_request_id        TEXT PRIMARY KEY
source_log_id             TEXT NULL
newapi_user_id            @NEWAPI_USER_ID_PG_TYPE NOT NULL
newapi_key_id             @NEWAPI_KEY_ID_PG_TYPE NOT NULL
key_purpose_snapshot      TEXT NOT NULL
purpose_version_snapshot  BIGINT NOT NULL
model_id                  TEXT NOT NULL
request_kind              TEXT NOT NULL
final_status              TEXT NOT NULL
error_category            TEXT NULL
actual_credit_units       BIGINT NOT NULL DEFAULT 0 CHECK(actual_credit_units >= 0)
request_at                TIMESTAMPTZ NOT NULL
finalized_at              TIMESTAMPTZ NOT NULL
attribution_source        TEXT NOT NULL
created_at                TIMESTAMPTZ NOT NULL
```

不保存 Prompt / Response / Provider Credential。

实际 Attribution Source 继续由 SV-12 决定。

---

# 97. `catalog` Physical Schema

## `catalog.model_catalog_metadata`

```text
model_id           TEXT PRIMARY KEY
display_name       TEXT NOT NULL
family             TEXT NOT NULL
summary            TEXT NULL
context_length     BIGINT NULL
metadata           JSONB NOT NULL DEFAULT '{}'
metadata_version   BIGINT NOT NULL DEFAULT 1
created_at         TIMESTAMPTZ NOT NULL
updated_at         TIMESTAMPTZ NOT NULL
```

## `catalog.model_catalog_publication`

```text
model_id           TEXT PRIMARY KEY
publication_state  TEXT NOT NULL
recommended        BOOLEAN NOT NULL DEFAULT FALSE
sort_order         INTEGER NOT NULL DEFAULT 0
published_at       TIMESTAMPTZ NULL
retired_at         TIMESTAMPTZ NULL
version            BIGINT NOT NULL DEFAULT 1
updated_at         TIMESTAMPTZ NOT NULL
```

## `catalog.model_sync_snapshots`

```text
sync_snapshot_id      UUID PRIMARY KEY
source_identity       TEXT NOT NULL
source_hash           BYTEA NOT NULL
observed_model_count  INTEGER NOT NULL
status                TEXT NOT NULL
safe_summary          JSONB NOT NULL DEFAULT '{}'
observed_at           TIMESTAMPTZ NOT NULL
```

## `catalog.model_availability_mappings`

```text
model_id             TEXT PRIMARY KEY
availability_state   TEXT NOT NULL
source_snapshot_id   UUID NOT NULL
observed_at           TIMESTAMPTZ NOT NULL
```

## `catalog.historical_model_identity`

```text
historical_identity_id UUID PRIMARY KEY
model_id               TEXT NOT NULL
display_name_snapshot  TEXT NOT NULL
family_snapshot        TEXT NOT NULL
effective_from         TIMESTAMPTZ NOT NULL
effective_until        TIMESTAMPTZ NULL
```

---

# 98. `economy` Physical Schema

## `economy.wallet_balances`

只保存：

```text
RESERVE_API_CREDIT
AVAILABLE_CHIPS
```

```text
newapi_user_id @NEWAPI_USER_ID_PG_TYPE NOT NULL
asset_type     TEXT NOT NULL
balance_units  BIGINT NOT NULL CHECK(balance_units >= 0)
ledger_seq     BIGINT NOT NULL DEFAULT 0
version        BIGINT NOT NULL DEFAULT 1
updated_at     TIMESTAMPTZ NOT NULL
PRIMARY KEY(newapi_user_id, asset_type)
```

Active Quota / Poker In Play / Processing Assets 不放入该表。

## `economy.asset_transactions`

```text
transaction_id UUID PRIMARY KEY
biz_type       TEXT NOT NULL
biz_id         TEXT NOT NULL
newapi_user_id @NEWAPI_USER_ID_PG_TYPE NOT NULL
operation_type TEXT NOT NULL
status         TEXT NOT NULL
created_at     TIMESTAMPTZ NOT NULL
confirmed_at   TIMESTAMPTZ NULL
updated_at     TIMESTAMPTZ NOT NULL
UNIQUE(biz_type, biz_id)
```

## `economy.wallet_ledger`

Append-only：

```text
ledger_entry_id       UUID PRIMARY KEY
transaction_id        UUID NOT NULL
leg_no                INTEGER NOT NULL
newapi_user_id        @NEWAPI_USER_ID_PG_TYPE NOT NULL
asset_type            TEXT NOT NULL
ledger_seq            BIGINT NOT NULL
entry_type            TEXT NOT NULL
biz_type              TEXT NOT NULL
biz_id                TEXT NOT NULL
delta_units           BIGINT NOT NULL
balance_before_units  BIGINT NOT NULL
balance_after_units   BIGINT NOT NULL
metadata              JSONB NOT NULL DEFAULT '{}'
created_at            TIMESTAMPTZ NOT NULL
```

Checks：

```text
balance_before_units >= 0
balance_after_units >= 0
balance_after_units = balance_before_units + delta_units
```

Unique：

```text
UNIQUE(newapi_user_id, asset_type, ledger_seq)
UNIQUE(transaction_id, leg_no)
```

## `economy.transfers`

```text
transfer_id          UUID PRIMARY KEY
transaction_id       UUID NOT NULL UNIQUE
newapi_user_id       @NEWAPI_USER_ID_PG_TYPE NOT NULL
direction            TEXT NOT NULL
requested_units      BIGINT NOT NULL CHECK(requested_units > 0)
state                TEXT NOT NULL
created_at           TIMESTAMPTZ NOT NULL
updated_at           TIMESTAMPTZ NOT NULL
confirmed_at         TIMESTAMPTZ NULL
failure_code         TEXT NULL
needs_review_reason  TEXT NULL
```

State：

```text
PENDING
SOURCE_DEBITING
SOURCE_DEBITED
TARGET_CREDITING
TARGET_CREDITED
CONFIRMED
COMPENSATING
COMPENSATED
FAILED_NO_EFFECT
NEEDS_REVIEW
```

## `economy.transfer_legs`

```text
leg_id               UUID PRIMARY KEY
transfer_id          UUID NOT NULL
authority            TEXT NOT NULL
position             TEXT NOT NULL
direction            TEXT NOT NULL
amount_units         BIGINT NOT NULL CHECK(amount_units > 0)
effect_operation_id  TEXT NULL
effect_state         TEXT NOT NULL
applied_at           TIMESTAMPTZ NULL
reversed_at          TIMESTAMPTZ NULL
created_at           TIMESTAMPTZ NOT NULL
```

Partial Unique：

```text
UNIQUE(authority, effect_operation_id)
WHERE effect_operation_id IS NOT NULL
```

## `economy.external_asset_events`

Append-only NewAPI Active Quota Evidence，不是第二余额 Authority。

## `economy.asset_snapshots`

保存 Reserve / Active / Chips / Poker / Processing / Total 的完整权威快照。

## `economy.reconciliation_records`

保存 Reconciliation Finding / State / Safe Detail / Operation Link。

## `economy.adjustments`

Admin Adjustment 通过独立 Transaction + Ledger，而非直接 Balance Patch。

## `economy.asset_supply_events`

Append-only `ISSUE / BURN` 事件。

---

# 99. `rewards` Physical Schema

## `rewards.reward_policies`

```text
reward_kind              TEXT PRIMARY KEY
active_policy_version_id UUID NULL
operational_state        TEXT NOT NULL
version                  BIGINT NOT NULL DEFAULT 1
updated_at               TIMESTAMPTZ NOT NULL
```

Reward Kind：

```text
INITIAL_GRANT_REGISTRATION
INITIAL_GRANT_MIGRATION
DAILY
HOURLY
RELIEF
```

## `rewards.policy_versions`

```text
policy_version_id           UUID PRIMARY KEY
reward_kind                 TEXT NOT NULL
version_number              BIGINT NOT NULL
status                      TEXT NOT NULL
asset_type                  TEXT NULL
amount_units                BIGINT NOT NULL CHECK(amount_units > 0)
business_timezone           TEXT NULL
window_mode                 TEXT NULL
cooldown_seconds            BIGINT NULL
eligibility_threshold_units BIGINT NULL
accumulation_mode           TEXT NULL
daily_limit_mode            TEXT NOT NULL DEFAULT 'UNRESOLVED'
daily_limit                 BIGINT NULL
active_poker_policy         TEXT NULL
effective_from              TIMESTAMPTZ NULL
effective_until             TIMESTAMPTZ NULL
created_at                  TIMESTAMPTZ NOT NULL
created_by                  @NEWAPI_USER_ID_PG_TYPE NULL
UNIQUE(reward_kind, version_number)
```

State：

```text
DRAFT
VALIDATED
SCHEDULED
ACTIVE
RETIRED
CONFIG_INCOMPLETE
```

Production Guard：

```text
HOURLY ACTIVE
→ asset_type/window_mode/accumulation_mode resolved
→ daily_limit_mode != UNRESOLVED

RELIEF ACTIVE
→ asset_type/accumulation_mode/active_poker_policy resolved
```

## `rewards.claims`

```text
claim_id                 UUID PRIMARY KEY
newapi_user_id           @NEWAPI_USER_ID_PG_TYPE NOT NULL
reward_kind              TEXT NOT NULL
claim_origin             TEXT NOT NULL
policy_version_id        UUID NOT NULL
biz_type                 TEXT NOT NULL
biz_id                   TEXT NOT NULL
period_key               TEXT NULL
asset_type               TEXT NOT NULL
amount_units             BIGINT NOT NULL CHECK(amount_units > 0)
status                   TEXT NOT NULL
server_requested_at      TIMESTAMPTZ NOT NULL
confirmed_at             TIMESTAMPTZ NULL
eligibility_snapshot_id  UUID NULL
economy_transaction_id   UUID NULL
failure_code             TEXT NULL
failure_detail_safe      TEXT NULL
created_at               TIMESTAMPTZ NOT NULL
updated_at               TIMESTAMPTZ NOT NULL
UNIQUE(biz_type, biz_id)
```

Partial period uniqueness：

```text
UNIQUE(newapi_user_id, reward_kind, period_key)
WHERE period_key IS NOT NULL
```

Policy Version 不进入 Period Unique Key。

## `rewards.daily_checkins`

```text
claim_id        UUID PRIMARY KEY
newapi_user_id  @NEWAPI_USER_ID_PG_TYPE NOT NULL
checkin_date    DATE NOT NULL
created_at      TIMESTAMPTZ NOT NULL
UNIQUE(newapi_user_id, checkin_date)
```

## `rewards.eligibility_snapshots`

保存 Relief 判定当时完整资产 Authority、Cooldown、Active Poker、Freshness、Policy 和 Result。

## `rewards.claim_cursors`

```text
newapi_user_id           @NEWAPI_USER_ID_PG_TYPE NOT NULL
reward_kind              TEXT NOT NULL
last_successful_claim_at TIMESTAMPTZ NULL
next_claim_at            TIMESTAMPTZ NULL
claim_sequence           BIGINT NOT NULL DEFAULT 0
version                  BIGINT NOT NULL DEFAULT 1
PRIMARY KEY(newapi_user_id, reward_kind)
```

## `rewards.entitlements`

只预留 Hourly Accumulation 技术能力；当前 `NO PRODUCTION WRITER`。

## `rewards.issuance_records`

Append-only，`claim_id` 与 `economy_transaction_id` 均 Unique。

---

# 100. `games` Physical Schema

## Registry / Metadata

正式建立：

```text
games.game_registry
games.game_metadata
games.game_categories
games.game_tags
games.game_category_assignments
games.game_tag_assignments
games.wager_policy_versions
games.game_config_versions
```

`game_registry` 使用 stable `game_slug` + Code-owned `implementation_key`，数据库不执行任意脚本成为游戏。

## Fairness

建立：

```text
games.client_seed_preferences
games.fairness_commitments
```

Fairness Commitment 至少拥有：

```text
reserved_round_id UUID UNIQUE
newapi_user_id
game_slug
server_seed_hash
encrypted seed envelope
client_seed / version
fairness_nonce
algorithm_version
state
created/consumed/revealed time
```

Unique：

```text
UNIQUE(newapi_user_id, game_slug, fairness_nonce)
```

## Common Round

### `games.game_rounds`

至少：

```text
round_id
newapi_user_id
game_slug
biz_type / biz_id
state / round_version
display snapshot
ruleset / algorithm version
config version/hash
wager policy version
fairness commitment
initial_wager_units
total_stake_units
total_payout_units
net_change_units
result_class
accepted/settled/refunded timestamps
```

Unique：

```text
UNIQUE(biz_type, biz_id)
```

Partial Unique：

```text
UNIQUE(newapi_user_id, game_slug)
WHERE state NOT IN ('SETTLED','REFUNDED','CANCELLED_NO_EFFECT')
```

## Common Bet / Result / Action

```text
games.game_bets
games.game_results
games.round_actions
```

`game_results` 强制：

```text
net_change_units = total_payout_units - total_stake_units
```

## V1 Typed Result Authority

建立：

```text
games.dice_results

games.scratch_results
games.scratch_cells

games.summon_results
games.summon_draw_results

games.slot_results
games.slot_line_results

games.blackjack_round_state
games.blackjack_hands
games.blackjack_dealt_cards
```

正式结果不能只存在任意 JSONB。

## History Projection

`games.history_index`：

```text
record_type
source_id
parent_source_id
newapi_user_id
game_slug
mode
result_class
display_status
occurred_at
ended_at
identity_snapshot_id
source_version
updated_at
PRIMARY KEY(record_type, source_id)
```

它仅为 Rebuildable List Projection。

---

# 101. `poker` Physical Schema

## `poker.ruleset_versions`

显式容纳五项仍 OPEN 的产品规则：

```text
ante_posting_mode
post_bb_now_semantics
initial_dealer_button_rule
hand_evaluator_version
raise_shortcut_formula_ver
```

任一为空时：

```text
CONFIG_INCOMPLETE
!= ACTIVE
```

## Table / Seat / Session

建立：

```text
poker.tables
poker.table_access_credentials
poker.seat_reservations
poker.seats
poker.sessions
poker.funding_operations
```

关键 Partial Unique：

```text
one owned nonclosed table / owner
one active session / user
one active reservation / table+seat
```

`table_access_credentials` 只允许 Poker 必要 Role 读取 Hash；Operations 不读取。

## Hand Authority

建立：

```text
poker.hands
poker.hand_participants
poker.dealt_cards
poker.actions
```

关键 Partial Unique：

```text
one nonterminal hand / table
```

`hand_participants` 是 Fairness 24h Participant Authorization 的 Durable Participant Set。

## Pot / Settlement

建立：

```text
poker.pots
poker.pot_eligible_players
poker.pot_awards
poker.settlements
```

`poker.settlements` 对 `hand_id` 与 `biz_id` Unique。

SETTLED 时强制：

```text
total_commitment_units
=
total_award_units
+
uncalled_return_units
```

V1 保持零和。

## Fairness / Recovery / Chat

建立：

```text
poker.hand_fairness
poker.recovery_state
poker.chat_messages
```

未公开 Server Seed / Deck 加密保存，不进入普通 Log。

Redis 丢失后必须能从 PostgreSQL 重建 Table / Session / Hand。

---

# 102. `ranking` Physical Schema

正式建立：

```text
ranking.source_facts
ranking.ingestion_cursors
ranking.current_asset_snapshots
ranking.periods
ranking.feature_activation
ranking.aggregate_sets
ranking.user_aggregates
ranking.published_pointers
ranking.source_exclusions
ranking.rebuild_requests
```

## Source Fact

`source_facts` Rebuildable + Immutable：

```text
fact_id
source_type/source_id/source_version
newapi_user_id
metric_family
dimension_key
game_slug/model_id
event_at
value_units/value_count
identity_snapshot_id
eligibility_state
```

Unique：

```text
UNIQUE(source_type, source_id, metric_family, dimension_key)
```

## Current Assets

`current_asset_snapshots` 必须是完整 Authority Snapshot；不完整 Source 不覆盖 Last Complete。

## Aggregate

`aggregate_sets` 状态：

```text
BUILDING
SHADOW
PUBLISHED
SUPERSEDED
FAILED
```

`published_pointers` 通过一个 PostgreSQL Transaction 原子切换。

`user_aggregates.rank_no` 使用 SQL `RANK()` 语义：

```text
1,2,2,4
```

Source Exclusion 不删除原 Source；Repair 通过 Shadow → Diff → Review → Publish。

---

# 103. `content` Physical Schema

## Public Game Events

`content.public_game_events` 可以建立，但：

```text
PUBLIC_RECORD_SELECTION_POLICY = UNRESOLVED
```

时：

```text
NO PRODUCTION WRITER
```

不得生成虚假 Recent Wins。

## Announcement

建立：

```text
content.announcements
content.announcement_revisions
content.notification_revisions
content.announcement_placements
content.announcement_reads
content.acknowledgement_entries
content.announcement_media_assets
```

永久区分：

```text
announcement_id
content_version
notification_revision
```

`announcement_revisions` / `notification_revisions` Append-only。

`announcement_reads`：

```text
PRIMARY KEY(
  newapi_user_id,
  announcement_id,
  notification_revision
)
```

Popup Dismissed 与 Announcement Read 不混用。

---

# 104. `ops` Physical Schema

正式建立：

```text
ops.admin_principals
ops.admin_principal_scopes
ops.admin_role_history

ops.attention_items

ops.incidents
ops.incident_events

ops.support_cases
ops.binding_cases

ops.maintenance_windows
ops.maintenance_window_scopes
ops.maintenance_scope_guards

ops.jobs
ops.job_runs
ops.job_schedules

ops.admin_operations
```

## Admin Principal

`admin_principals`：

```text
admin_principal_id UUID
newapi_user_id UNIQUE
base_role
status
authz_epoch
created/updated/disabled metadata
version
```

Role：

```text
SUPER_ADMIN
OPERATOR
AUDITOR
```

Operator Scope 只允许：

```text
MODELS
USERS_IDENTITY
GAMES
POKER
REWARDS
RANKINGS
RECORDS
ANNOUNCEMENTS
```

## Needs Attention

Unique：

```text
(attention_source_type, attention_source_id, attention_reason_code)
```

State：

```text
OPEN
ACKNOWLEDGED
RESOLVED
```

Acknowledge != Fixed。

## Incident / Support

Incident Timeline 与 Role History 都 Append-only。

Support Evidence 只保存最小安全元数据；不保存 Password / Hash / Key Secret。

---

# 105. Maintenance Durable Authority

`ops.maintenance_windows`：

```text
maintenance_id
state
reason
impact_snapshot
impact_hash
scheduled_start_at
estimated_end_at
activated_at
ended_at
created_by/activated_by/ended_by
operation_id
created_at
```

Lifecycle：

```text
DRAFT
SCHEDULED
ACTIVE
ENDING
COMPLETED
CANCELLED
ACTIVATION_FAILED
ENDING_FAILED
```

固定七 Scope：

```text
CHALDEA_USER_WRITES
WALLET_EXCHANGE
REWARDS
DIRECT_PLAY_NEW_ROUNDS
POKER_NEW_TABLES_NEW_HANDS
RANKINGS_PUBLISHING
ANNOUNCEMENTS_SCHEDULING
```

`maintenance_scope_guards` 固定七行。

同 Scope Create/Schedule/Activate：

```text
SELECT guard FOR UPDATE
→ inspect unfinished window
→ reject overlap
```

不同 Scope 可同时生效。

---

# 106. Durable Background Jobs

`ops.jobs`：

```text
job_id UUID
job_type
dedupe_key
payload_schema_version
payload JSONB
state
due_at
lease_owner
lease_expires_at
last_heartbeat_at
automatic_attempts
target_business_id
created/updated/completed
```

Unique：

```text
UNIQUE(job_type, dedupe_key)
```

State：

```text
SCHEDULED
PENDING
RUNNING
RETRY_WAIT
SUCCEEDED
NEEDS_ATTENTION
CANCELLED
BLOCKED_MAINTENANCE
```

Worker Claim：

```text
SELECT due job
FOR UPDATE SKIP LOCKED
```

Redis 不成为 Queue Authority。

`ops.job_runs` Append-only，`UNIQUE(job_id, attempt_no)`。

`job_schedules` 只存 Code-allowlisted Job Type + Versioned Typed Schedule Payload。

---

# 107. `ops.admin_operations`

Durable Critical/Admin Operation：

```text
operation_id
operation_type
risk_level
actor_newapi_user_id
actor_role
actor_scopes_snapshot
target_type / target_id
reason
impact_preview / impact_hash
confirmation_challenge_hash
fresh_auth_verified_at
state
related_business_id
created/executed/completed
```

它是 Operations Recovery / Audit / Critical Action 的持久化 Operation Identity。

---

# 108. `audit` Physical Schema

`audit.audit_events`：

```text
audit_id UUID PRIMARY KEY
actor_newapi_user_id
actor_role
actor_scopes_snapshot
action
target_type
target_id
before_snapshot
after_snapshot
reason
operation_id
result
occurred_at
related_business_id
request_id
environment
```

Runtime Grant：

```text
SELECT
INSERT
```

禁止：

```text
UPDATE
DELETE
TRUNCATE
```

Secret 在 Insert 前由安全 Serializer 脱敏。

---

# 109. `migration` Physical Schema

正式建立：

```text
migration.cutover_batches
migration.cutover_user_states
migration.pre_cutover_quota_snapshots
migration.balance_reset_audit
migration.grant_results
migration.validation_reports
migration.migration_notice_versions
migration.migration_notice_acknowledgements
```

## Batch State

```text
PLANNED
PRECHECKED
BACKUP_READY
MAINTENANCE_LOCKED
SNAPSHOT_COMPLETE
RESET_COMPLETE
GRANT_COMPLETE
VERIFYING
VERIFIED
READY_TO_OPEN
COMPLETED

FAILED_RETRYABLE
ROLLBACK_REQUIRED
ROLLED_BACK
REPAIR_REQUIRED
```

## Per-user State

```text
PENDING
SNAPSHOTTED
RESET_VERIFIED
GRANT_CONFIRMED
VERIFIED
FAILED
NEEDS_REVIEW
```

Migration Initial Grant 继续通过正式 Reward/Economy Claim + Ledger 产生，不能直接把最终 Wallet Balance SET 成 1000。

Migration Notice Ack 只确认迁移事实，不执行 Reset / Grant / Cutover。

---

# 110. Foreign-key / Delete Policy

同 `chaldea_platform` DB 的真实实体关系使用 FK。

默认：

```text
ON DELETE RESTRICT
```

特别适用于：

```text
Ledger
Claims
Rounds
Poker Sessions / Hands
Ranking Sources
Audit
Migration
```

仅纯关联表在确认不构成历史 Authority 时允许受控 `ON DELETE CASCADE`。

业务退役优先：

```text
RETIRED
ARCHIVED
DISABLED
RELEASED through audited operation
```

而不是 Hard Delete。

---

# 111. Append-only DB Enforcement

创建：

```text
platform_meta.reject_immutable_mutation()
```

并在 Runtime 下 `BEFORE UPDATE OR DELETE` 保护至少：

```text
identity.master_profile_name_history
identity.identity_display_snapshots

api.api_key_purpose_history

economy.wallet_ledger
economy.external_asset_events
economy.asset_supply_events

rewards.daily_checkins
rewards.issuance_records

ranking.source_facts

content.announcement_revisions
content.notification_revisions

ops.admin_role_history
ops.incident_events
ops.job_runs

audit.audit_events

migration.pre_cutover_quota_snapshots
migration.balance_reset_audit
migration.validation_reports
```

Correction 使用新 Reversal / Adjustment / Compensation，不改原事实。

---

# 112. Poker Funding Transaction Gateway

因 Wallet 与 Poker 位于同一 `chaldea_platform` DB，建立三个窄范围 Owner-controlled Function：

```text
economy.poker_buy_in_apply(...)
economy.poker_top_up_apply(...)
economy.poker_cash_out_apply(...)
```

属性：

```text
SECURITY DEFINER
Owner = chaldea_owner
EXECUTE = chaldea_poker only
REVOKE EXECUTE FROM PUBLIC
fixed search_path
```

Function 内只能执行已冻结资金边界：

```text
lock wallet
validate nonnegative
insert asset transaction
update wallet
insert ledger
update poker funding/session
```

不接受 arbitrary table / SQL / target account。

---

# 113. Major Index Manifest

至少建立：

```text
identity.master_profiles(normalized_name) UNIQUE
identity.master_profile_name_history(newapi_user_id, changed_at DESC)
identity.registration_operations(discord_user_id, created_at DESC)

api.request_attributions(newapi_user_id, request_at DESC)
api.request_attributions(model_id, request_at)
api.request_attributions(key_purpose_snapshot, request_at)

economy.wallet_ledger(newapi_user_id, asset_type, created_at DESC)
economy.wallet_ledger(transaction_id)
economy.transfers(state, updated_at)
economy.transfer_legs(transfer_id)
economy.reconciliation_records(state, detected_at)

rewards.claims(newapi_user_id, reward_kind, created_at DESC)
rewards.claims(status, updated_at)
rewards.claims(economy_transaction_id)
rewards.eligibility_snapshots(newapi_user_id, created_at DESC)

games.game_rounds(newapi_user_id, accepted_at DESC)
games.game_rounds(game_slug, accepted_at DESC)
games.game_rounds(state, updated_at)
games.round_actions(round_id, created_at)
games.history_index(newapi_user_id, occurred_at DESC)
games.history_index(game_slug, occurred_at DESC)

poker.tables(state)
poker.sessions(newapi_user_id, started_at DESC)
poker.sessions(table_id, state)
poker.hands(table_id, hand_no DESC)
poker.hands(state, updated_at)
poker.actions(hand_id, action_seq)
poker.chat_messages(table_id, created_at)

ranking.source_facts(event_at)
ranking.source_facts(newapi_user_id, metric_family, event_at)
ranking.aggregate_sets(domain, metric, period_id, status)
ranking.user_aggregates(aggregate_set_id, rank_no)

content.announcements(state, visible_from, visible_until)
content.announcement_placements(placement_type, enabled, effective_from)

ops.attention_items(state, severity, last_seen_at DESC)
ops.incidents(state, severity, created_at DESC)
ops.admin_operations(state, created_at DESC)

audit.audit_events(occurred_at DESC)
audit.audit_events(target_type, target_id, occurred_at DESC)
```

Jobs：

```text
ops.jobs(due_at)
WHERE state IN ('SCHEDULED','PENDING','RETRY_WAIT')

ops.jobs(lease_expires_at)
WHERE state = 'RUNNING'
```

---

# 114. Migration Manifest

正式顺序：

```text
000000__cluster_role_bootstrap
000001__schemas_and_platform_meta
000002__catalog_base

000003__identity_base
000004__api_metadata

000005__audit_base
000006__economy_base
000007__rewards_base

000008__games_platform
000009__games_v1_typed_results

000010__poker_base

000011__ranking_and_history
000012__content

000013__ops_jobs_maintenance

000014__migration_cutover

000015__cross_schema_foreign_keys
000016__poker_funding_functions
000017__immutability_and_final_grants
```

Dependency：

```text
000003+ needs @NEWAPI_USER_ID_PG_TYPE
→ BLOCKED_BY_SV-05 for SQL finalization

000004 needs @NEWAPI_KEY_ID_PG_TYPE
→ BLOCKED_BY_SV-05 / SV-06
```

---

# 115. No Placeholder Production SQL

禁止在 `database/migrations/*.sql` 提交：

```text
{{TYPE_TODO}}
UNKNOWN
@NEWAPI_USER_ID_PG_TYPE
```

正确流程：

```text
IS-03 Logical Schema FROZEN
→ SV resolves actual ID type
→ fill schema manifest
→ generate exact SQL
→ review
→ checksum
→ immutable migration
```

Spec 可以有 Blocked Type Slot，Production SQL 不可以有“貌似可执行”的 Placeholder。

---

# 116. Migration File Contract

每个 Migration Header 必须记录：

```text
migration_version
name
depends_on
introduced_objects
required_source_verifications
forward_fix_boundary
verification_query_ref
```

Applied Migration 永不原地修改。

---

# 117. Rollback / Forward-fix

V1 不要求所有 Migration 都存在普通 `down.sql`。

不可逆业务变化：

```text
Backup
→ Forward Migration
→ Verification
→ Forward Fix when necessary
```

不以 DROP / Reverse Update 假装可以恢复金融、Game、Poker 历史。

---

# 118. Schema Verification Gate

Migration 后至少验证：

## Schema / Role

```text
expected schemas exist
all checksums match
no business table in public

chaldea_app has no DDL
chaldea_poker has no arbitrary economy DML
PUBLIC has no object write grants
```

## Identity

```text
duplicate normalized_name = 0
orphan Chaldea user reference = 0
```

## Economy

```text
negative wallet = 0
ledger arithmetic mismatch = 0
duplicate biz identity = 0
materialized balance vs ledger chain mismatch = 0
```

## Rewards

```text
duplicate reward period = 0
CONFIRMED claim without economy transaction = 0
ACTIVE incomplete Hourly / Relief policy = 0
```

## Games

```text
>1 active round per user/game = 0
SETTLED + REFUNDED conflict = 0
duplicate fairness nonce = 0
```

## Poker

```text
>1 active session per user = 0
>1 nonterminal hand per table = 0
settlement conservation mismatch = 0
settled hand without settlement = 0
```

## Ranking / Content / Jobs / Audit

```text
published pointer references PUBLISHED set
published incomplete aggregate set = 0
published announcement has content revision
duplicate job_type/dedupe_key = 0
runtime has no UPDATE/DELETE/TRUNCATE audit grants
```

---

# 119. Migration CI Gate

PR 检查：

```text
monotonic migration numbers
no duplicate migration number
checksum manifest
applied migration unchanged
correct object owner
expected grants
no PUBLIC write
no FLOAT asset column
all *_units financial columns BIGINT
no Secret default/value
no cross-database FK/trigger
no unexpected public-schema business object
```

---

# 120. Codex IS-03 Boundary

允许：

```text
database/schema_manifest.md
database/migration_manifest.md
database/role_grant_matrix.md
database/migrations templates
verification SQL
blocked type-slot manifest
migration CI validator
```

SV-05 未完成时禁止：

```text
finalize user-keyed production SQL with guessed type
run production migration
modify NewAPI schema
reset NewAPI quota
perform Product Cutover
```

---

# 121. IS-03 Acceptance Criteria

```text
AC-03-01  canonical 13 schemas defined
AC-03-02  durable tables have explicit PK/type/nullability
AC-03-03  Chaldea entity ID = UUIDv7/UUID
AC-03-04  asset facts = BIGINT
AC-03-05  no generic JSONB authority for finance/game/poker
AC-03-06  NewAPI ID uncertainty explicit
AC-03-07  guessed NewAPI type never reaches production SQL
AC-03-08  normalized Master Name DB Unique
AC-03-09  wallet nonnegative
AC-03-10  append-only self-consistent ledger
AC-03-11  business IDs DB unique
AC-03-12  reward duplicate period DB protected
AC-03-13  incomplete Hourly/Relief cannot ACTIVE
AC-03-14  one active direct-play round/user/game
AC-03-15  fairness nonce DB unique
AC-03-16  typed game result authority
AC-03-17  one active Poker session/user
AC-03-18  one nonterminal hand/table
AC-03-19  Poker zero-sum settlement invariant
AC-03-20  poker private data not readable by ordinary app/ops
AC-03-21  ranking facts rebuildable, not source authority
AC-03-22  history index rebuildable only
AC-03-23  no public event writer without policy
AC-03-24  announcement content vs notification revision separated
AC-03-25  job authority = PostgreSQL
AC-03-26  job dedupe uniqueness
AC-03-27  authz_epoch durable
AC-03-28  maintenance PostgreSQL-authoritative
AC-03-29  audit append-only
AC-03-30  Poker economy mutation only via approved funding function
AC-03-31  runtime has no DDL
AC-03-32  migrations immutable/checksummed
AC-03-33  irreversible migration uses forward-fix, not fake down
AC-03-34  verification covers high-risk invariants
AC-03-35  Product OPEN / SV blockers preserved
```

---

# 122. IS-03 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| IS-FRZ-063 | `chaldea_platform` 固定使用 13 个业务 Schema：platform_meta / identity / api / catalog / economy / rewards / games / poker / content / ranking / ops / audit / migration；业务对象不使用 public Schema。 | FROZEN |
| IS-FRZ-064 | Chaldea 新 Durable Entity 使用 PostgreSQL UUID + Application-generated UUIDv7；资产事实统一 BIGINT，正式时间统一 TIMESTAMPTZ。 | FROZEN |
| IS-FRZ-065 | `newapi_user_id` 与 `newapi_key_id` 的 PostgreSQL 实体类型建立显式 Source Verification Type Slot；SV-05/SV-06 未完成前禁止猜测 BIGINT/TEXT 并生成最终生产 SQL。 | FROZEN |
| IS-FRZ-066 | DB Role 固定为 owner / migrator / app / poker / cutover permission role，并保持 source-verified NewAPI RO 与 Backup Identity 独立；Runtime 无 DDL。 | FROZEN |
| IS-FRZ-067 | 数据库权限 default-deny；PUBLIC 无业务写权限，Platform 不任意写 Poker，Poker 不任意写 Economy。 | FROZEN |
| IS-FRZ-068 | Schema Migration 使用连续版本化 Immutable SQL + checksum + dedicated migration job；Applied File 不允许修改。 | FROZEN |
| IS-FRZ-069 | Source Verification Blocker 只存在于 Spec/Manifest；正式 Migration SQL 永远不允许 `TODO` 类型或占位 Schema。 | FROZEN |
| IS-FRZ-070 | `identity.account_refs` 作为所有 Chaldea User FK Anchor，并持久化 `security_epoch`；它不成为 NewAPI Account/Password Authority。 | FROZEN |
| IS-FRZ-071 | Master Profile、Name History、Reserved Name、Avatar Snapshot、Identity Display Snapshot 按 IS-03 精确表结构实现；normalized_name 由数据库 Unique 强制。 | FROZEN |
| IS-FRZ-072 | Registration Operation 与 Registration Idempotency Record 使用独立 Durable Table；Discord Binding 本身仍不建立第二个 Chaldea 可编辑 Authority。 | FROZEN |
| IS-FRZ-073 | API Key Purpose 使用 Current Metadata + Append-only History + Immutable Request Attribution；Key Secret 永不进入 Chaldea DB。 | FROZEN |
| IS-FRZ-074 | Catalog 使用 stable textual `model_id`，Publication/Sync/Availability/Historical Identity 与模型业务身份分离。 | FROZEN |
| IS-FRZ-075 | `economy.wallet_balances` 只保存 RESERVE_API_CREDIT / AVAILABLE_CHIPS，Composite PK + Non-negative Check；Active/Poker/Processing 不伪装成 Wallet Row。 | FROZEN |
| IS-FRZ-076 | `wallet_ledger` 使用 append-only Ledger Entry、per-wallet ledger_seq、Before/Delta/After Arithmetic Check，并与 Materialized Balance 同事务。 | FROZEN |
| IS-FRZ-077 | `asset_transactions / transfers / transfer_legs` 精确表达 Biz Identity、Cross-DB Saga 和 Processing Position；每单位不得重复出现在 Source/Processing/Target。 | FROZEN |
| IS-FRZ-078 | Active NewAPI Quota 只保存 External Asset Event / Snapshot / Reconciliation Evidence，不建立 Chaldea 第二 Active Balance Authority。 | FROZEN |
| IS-FRZ-079 | Reward 使用 `reward_policies + policy_versions`，Product-locked 值进入 Versioned Policy，而非可随意编辑的普通 Config。 | FROZEN |
| IS-FRZ-080 | Reward Durable Authority 使用 `claims / daily_checkins / eligibility_snapshots / claim_cursors / issuance_records`；Claim Period/Business ID 具有 DB-level Exactly-once Constraint。 | FROZEN |
| IS-FRZ-081 | HOURLY / RELIEF Required OPEN 字段未解决时数据库和 Validator 都禁止 Policy 进入 ACTIVE；`NULL/UNRESOLVED` 永远不解释为默认业务答案。 | FROZEN |
| IS-FRZ-082 | Game Registry、Metadata、Category/Tag、Global Wager Policy、Immutable Config Version 使用独立表；Registry 只引用 Code-owned implementation_key。 | FROZEN |
| IS-FRZ-083 | Direct Play 使用 Common `game_rounds / game_bets / game_results / round_actions`；同 user/game 使用 Partial Unique 防第二 Active Round。 | FROZEN |
| IS-FRZ-084 | Direct Play Fairness 使用 durable Commitment、encrypted unrevealed Seed、client-seed preference、DB-unique nonce 与 terminal reveal contract。 | FROZEN |
| IS-FRZ-085 | Dice/Scratch/Summon/Slot/Blackjack 使用 Typed Result Tables；不能只用一个任意 JSONB 保存正式结果。 | FROZEN |
| IS-FRZ-086 | `games.history_index` 为可重建 List Projection；Round/Session/Hand Detail 永远回到各自 Durable Domain Authority。 | FROZEN |
| IS-FRZ-087 | Poker 建立 Versioned Ruleset Table 显式容纳五项 Product Gap；任一 Required Gap 未解决时 Ruleset 只能 CONFIG_INCOMPLETE，不能 ACTIVE。 | FROZEN |
| IS-FRZ-088 | Poker Table / Private Access Credential / Reservation / Seat 使用独立 Durable Tables，并由 PostgreSQL 保存 Table Version / Runtime Epoch。 | FROZEN |
| IS-FRZ-089 | Poker Session / Funding Operation 使用 DB Partial Unique 保证 V1 单用户单 Active Session；Buy-in/Top-up/Rebuy/Cash-out 具有 stable funding identity。 | FROZEN |
| IS-FRZ-090 | Poker Hand / Participant / Dealt Card / Action 使用 Typed Durable Tables；Hole Cards 只保存在受限 Poker Authority 中，Viewer Projection 不向客户端发送所有私牌。 | FROZEN |
| IS-FRZ-091 | Poker Pot 使用 pots / pot_eligible_players / pot_awards 三表，并显式记录 Odd Chip。 | FROZEN |
| IS-FRZ-092 | Poker Settlement 具有 hand-level Unique Biz Identity，并用 DB Check 保证 commitments = awards + uncalled return 的 V1 零和不变量。 | FROZEN |
| IS-FRZ-093 | Poker Fairness / encrypted Deck / Recovery / Chat 都为 Durable PostgreSQL 数据；Redis 丢失后仍可恢复正式 Table/Hand。 | FROZEN |
| IS-FRZ-094 | Poker 使用 Partial Unique 强制一桌最多一个 Nonterminal Hand、一用户最多一个 Active Session，并保留 Table Owner 唯一性。 | FROZEN |
| IS-FRZ-095 | Ranking Pipeline 使用 immutable/rebuildable `source_facts + ingestion_cursors`，Ranking 永不成为 Round/Session/API Usage 的第二业务 Authority。 | FROZEN |
| IS-FRZ-096 | Total Assets Current Snapshot、Ranking Period 与 Feature Activation 以独立表表达；不完整 Authority 不覆盖 Last Complete Snapshot。 | FROZEN |
| IS-FRZ-097 | Ranking Aggregate 使用 Versioned aggregate_sets / user_aggregates / published_pointers，并以原子 Published Pointer Swap 保证完整 Snapshot。 | FROZEN |
| IS-FRZ-098 | Ranking Source Exclusion 与 Rebuild Request 不删除原 Source；Repair 通过 SHADOW→Diff→Review→Publish。 | FROZEN |
| IS-FRZ-099 | `content.public_game_events` 可以建立安全物理表，但在 PUBLIC_RECORD_SELECTION_POLICY 未确认前没有 Production Writer，不生成虚假 Recent Wins。 | FROZEN |
| IS-FRZ-100 | Announcement 永久区分 announcement_id / immutable content_version / notification_revision，并建立独立 Revision Tables。 | FROZEN |
| IS-FRZ-101 | Announcement Placement、Logged-in Read State、Acknowledgement Entries、Media Mapping 使用独立表；Popup Dismissed 与 Read 不混用。 | FROZEN |
| IS-FRZ-102 | Background Jobs 固定由 PostgreSQL `ops.jobs / job_runs / job_schedules` 持久化，使用 `(job_type,dedupe_key)` Unique 与 SKIP LOCKED Lease；Redis 不成为 Queue Authority。 | FROZEN |
| IS-FRZ-103 | Chaldea RBAC Durable Authority 使用 admin_principals / admin_principal_scopes / append-only role history，持久化 authz_epoch。 | FROZEN |
| IS-FRZ-104 | Needs Attention / Incident / Incident Timeline / Support Case / Binding Case 使用 Durable Tables；Projection/Acknowledge 不改变底层业务 Authority。 | FROZEN |
| IS-FRZ-105 | Maintenance 使用 PostgreSQL maintenance_windows + scopes + scope guards，固定七 Scope，并通过 Guard Row Lock 阻止同 Scope 未完成窗口重叠。 | FROZEN |
| IS-FRZ-106 | Operations Critical Action 使用 durable admin_operations，记录 Risk/Impact/Fresh Auth/Confirmation/State/Business Reference。 | FROZEN |
| IS-FRZ-107 | `audit.audit_events` 为统一 Append-only Audit Authority；Runtime 只允许 SELECT/INSERT，Secret 在 Insert 前安全脱敏。 | FROZEN |
| IS-FRZ-108 | Cutover 使用 batches / per-user states / quota snapshots / reset audit / grant results / validation reports / notice versions / acknowledgements，保持 Durable Resume 语义。 | FROZEN |
| IS-FRZ-109 | Poker Funding 仅通过三个 owner-controlled SECURITY DEFINER Narrow Functions 原子跨 Economy/Poker Schema；Poker Role 不获得普通 Economy DML。 | FROZEN |
| IS-FRZ-110 | Durable History/Ledger/Audit/Snapshot 表执行 ON DELETE RESTRICT + Runtime Append-only Enforcement；业务退役使用状态而不是 Cascade Hard Delete。 | FROZEN |
| IS-FRZ-111 | IS-03 Migration Manifest 固定为 000000～000017 顺序，并明确各 Migration Source Verification Dependency / Checksum / Verification Query。 | FROZEN |
| IS-FRZ-112 | V1 Database Rollback 采用 Forward-fix 为主；高风险 Schema Migration 必须通过 Role/Grant、Financial、Reward、Game、Poker、Ranking、Job、Audit、Migration Invariant Verification 后才能部署。 | FROZEN |

---

# 123. Open / Blocked Register after IS-03

```text
Product Decision Blocker:
- Reward OPEN
- Poker Product Gap 01～05
- Public Record Selection Policy

NewAPI Source Verification:
- SV-01 ～ SV-16
- Status = BLOCKED_BY_NEWAPI_SOURCE_VERIFY

Database SQL Finalization:
- @NEWAPI_USER_ID_PG_TYPE = BLOCKED_BY_SV-05
- @NEWAPI_KEY_ID_PG_TYPE = BLOCKED_BY_SV-05 / SV-06

Deployment Verification:
- DEPLOYMENT-VERIFY-01 = PENDING

Implementation Configuration:
- unresolved values remain UNRESOLVED_IMPLEMENTATION_CONFIG
```

没有任何 Product OPEN 被数据库默认值代替。

---

# 124. Change Log — WORKING v0.3

## Added

- 用户正式确认 `IS-03 — PostgreSQL Schema & Migration Specification`；
- 冻结 `IS-FRZ-063 ～ IS-FRZ-112`；
- 冻结 13 个 Chaldea PostgreSQL Schema；
- 冻结 UUIDv7 / TIMESTAMPTZ / BIGINT / Typed-state / JSONB Boundary；
- 冻结 Chaldea Database Role / Grant Boundary；
- 冻结 Identity / API / Catalog / Economy / Reward / Games / Poker / Ranking / Content / Ops / Audit / Migration 物理模型；
- 冻结 Append-only Enforcement；
- 冻结 Poker Funding SECURITY DEFINER Narrow Functions；
- 冻结 Major Index Manifest；
- 冻结 Migration `000000 ～ 000017` 顺序；
- 冻结 Migration Checksum / Verification / Forward-fix Gate；
- 显式建立 NewAPI ID PostgreSQL Type Slot Blocker。

## Preserved

```text
TD-FRZ-001 ～ TD-FRZ-552
IS-FRZ-001 ～ IS-FRZ-112

Reward OPEN
Poker Product Gap 01～05
Public Record Selection Policy

SV-01 ～ SV-16 unresolved facts
DEPLOYMENT-VERIFY-01 pending
Implementation-only Config unresolved values
Production Readiness gates
```

## Supersession

```text
Existing IS-FRZ superseded:
None
```

---

# 125. Next Batch

> **IS-04 — Auth / Session / Master / Account / Cutover Implementation Specification**

IS-04 将把已冻结 Auth / Identity / Migration 设计落成：

```text
Chaldea opaque session
session record / Redis key
CSRF / CORS
Discord OAuth state
Existing Binding pre-check
First-registration Role verification
Discord registration idempotency
Password-login NewAPI Adapter contract
Master initialization
Master nickname/avatar mutation
Fresh Auth
security_epoch / authz_epoch interaction
logout / session revoke boundary
Poker connect ticket
Migration Notice gate
Initial Super Admin bootstrap
Cutover execution contract
```

所有实际 NewAPI Password / Binding / Identifier / Session / Quota Endpoint 继续引用 `SV-*`，不得猜测。

---

# 126. IS-04 — Auth / Session / Master / Account / Cutover Implementation Specification

> 状态：`FROZEN`  
> 用户确认：`整体按上述 IS-04 方案通过`  
> Frozen Decision Range：`IS-FRZ-113 ～ IS-FRZ-160`  
> NewAPI Source Facts：`SV-01 ～ SV-16 = BLOCKED_BY_NEWAPI_SOURCE_VERIFY`

## 126.1 Purpose

IS-04 将已冻结的 Authentication / Account / Master / Session / Migration 设计落成可直接实现的：

```text
Go package
Redis key
session record
cookie
CSRF
OAuth state
Discord registration
password adapter boundary
fresh authentication
access gate
return-to-intent
Master validator
Master mutation
Poker connect ticket
logout / revocation
Migration Notice
Cutover CLI
Initial Super Admin bootstrap CLI
recovery / test gate
```

本批不建立第二套 NewAPI Account / Password / Discord Binding Authority，也不猜测 NewAPI 实际 Endpoint / Table / Column。

---

# 127. Identity Authority Chain

正式身份链：

```text
Authentication Identity
Discord OAuth / Password Proof
        ↓
Account Identity
stable newapi_user_id
Password Login Identifier
NewAPI Account Status
Discord Binding
        ↓
Master Identity
Master Nickname
Display Avatar
```

所有正式业务归属继续使用：

```text
newapi_user_id
```

以下永远不能替代 Account Identity：

```text
Master Nickname
Discord Display Name
Short Account ID
```

---

# 128. Backend Physical Package Layout

在 IS-01 已冻结结构内细化：

```text
backend/
├── cmd/
│   ├── chaldea-platform/
│   │   └── main.go
│   ├── chaldea-bootstrap/
│   │   └── main.go
│   └── chaldea-cutover/
│       └── main.go
│
└── internal/
    ├── auth/
    │   ├── session/
    │   ├── csrf/
    │   ├── oauthflow/
    │   ├── password/
    │   ├── freshauth/
    │   ├── gate/
    │   ├── returnintent/
    │   └── pokerticket/
    │
    ├── identity/
    │   ├── account/
    │   ├── master/
    │   ├── nickname/
    │   ├── avatar/
    │   └── publicid/
    │
    ├── migration/
    │   └── cutover/
    │
    ├── adapters/
    │   ├── discord/
    │   ├── newapi/
    │   └── poker/
    │
    └── storage/
        ├── postgres/
        └── redis/
```

禁止建立无边界 `auth/utils` / `identity/helpers` 通用垃圾桶。

---

# 129. IS-04 Go Dependency Lock

Backend 在本批首次正式使用 PostgreSQL / Redis / Unicode 实现库，锁定：

```text
github.com/jackc/pgx/v5       v5.10.0
github.com/redis/go-redis/v9  v9.22.0
golang.org/x/text             v0.41.0
github.com/rivo/uniseg        v0.4.7
```

职责：

```text
pgx        → PostgreSQL
go-redis   → Chaldea Redis ephemeral state
x/text     → NFKC + Unicode Case Fold
uniseg     → Unicode Grapheme Cluster counting
```

不引入 ORM、Auth Framework 或 JWT Framework。

---

# 130. Browser Session Strategy

Browser Authentication 使用：

> **Chaldea Server-side Opaque Session**

浏览器不得持有：

```text
NewAPI Session
long-lived JWT
NewAPI Password Token
Discord OAuth Token
```

正式 Cookie：

```http
__Host-chaldea_session=<raw_sid>

Secure
HttpOnly
SameSite=Lax
Path=/
No Domain
```

V1 不提供 Remember Me，不设置普通长期认证持久化。

Logout 使用 `Max-Age=0` 清除 Cookie。

---

# 131. Session ID / Redis Key

Raw SID：

```text
32 random bytes
CSPRNG = crypto/rand.Reader
encoding = base64url without padding
```

Redis 不使用 Raw SID。

```text
sid_hash = SHA-256(raw_sid)

key =
chaldea:session:<lowercase-hex-sha256>
```

Raw SID 永远禁止进入：

```text
log
trace
audit
metric label
error payload
```

---

# 132. Anonymous Pre-auth Session

为了让匿名：

```text
GET /api/v1/session/bootstrap
OAuth Start
Password Login
```

也能安全拥有 CSRF / OAuth Context，Session State 支持：

```text
ANONYMOUS
AUTHENTICATED
REVOKED
```

Anonymous Session：

```text
newapi_user_id = null
ANONYMOUS_SESSION_TTL = 30 minutes
```

认证成功：

```text
Anonymous SID
→ generate brand-new Authenticated SID
→ revoke old Anonymous SID
```

禁止将 Anonymous Record 原地添加 `user_id` 后升级。

---

# 133. Authenticated Session Record

Redis Value 使用 Versioned Typed JSON：

```text
schema_version = 1

state
session_id_hash

newapi_user_id

created_at
auth_chain_started_at
last_seen_at
idle_expires_at
absolute_expires_at

login_method
authenticated_at

fresh_auth_at
fresh_auth_method

security_epoch_snapshot

account_status_snapshot
account_status_checked_at

csrf_secret

session_version
revoked_at

pending_return_intent_id nullable
password_reset_authorization nullable
avatar_sync_candidate_id nullable
```

`newapi_user_id` 在 Go / Redis / BFF Contract 中继续作为 opaque string identity。

---

# 134. Session Lifetime / Touch

Session 必须支持：

```text
Idle Expiration
Absolute Expiration
Rolling Touch
Server-side Revoke
Session Rotation
```

精确生产参数：

```text
SESSION_IDLE_TTL
SESSION_ABSOLUTE_TTL
SESSION_TOUCH_INTERVAL
```

当前：

```text
UNRESOLVED_IMPLEMENTATION_CONFIG
+
SV-01 evidence required
```

Redis TTL：

```text
min(
  idle_expires_at - now,
  absolute_expires_at - now
)
```

只有：

```text
now - last_seen_at >= SESSION_TOUCH_INTERVAL
```

时才 touch Redis，不要求每 HTTP Request 都写。

Fresh Auth Rotation：

```text
preserve auth_chain_started_at
preserve absolute_expires_at
refresh idle expiry
update fresh_auth_at
```

因此循环 Fresh Auth 不能无限延长 Absolute Lifetime。

---

# 135. Session Rotation

以下操作必须生成新 SID + 新 CSRF Secret：

```text
Password Login Success
Discord Login Success
Registration Authentication Success
Fresh Password Authentication Success
Fresh Discord Authentication Success
Password Reset Success
Identity Recovery / Rebind Re-authentication
```

Rotation：

```text
generate new SID
generate new CSRF secret
copy only safe authenticated context
write new session
revoke old session
set new cookie
```

旧 SID / CSRF 立即失效。

---

# 136. `security_epoch`

Durable Authority：

```text
identity.account_refs.security_epoch
```

Session 保存：

```text
security_epoch_snapshot
```

Protected Request：

```text
read current security_epoch
compare with session snapshot
```

Mismatch：

```text
SESSION_REVOKED
→ revoke Redis session
→ clear cookie
→ require authentication
```

不能依赖“删完所有 Redis Session Key”作为唯一撤权方式。

---

# 137. `authz_epoch`

Operations Authorization 与普通 Browser Authentication 分离。

Session 可以保存安全的 Operations Summary 供 UI 使用，但：

```text
cached role
cached scopes
cached authz_epoch
```

永远不是授权真相。

`/ops/*` Backend Request 必须读取当前：

```text
ops.admin_principals
authz_epoch
status
base_role
assigned scopes
```

并执行服务端授权。

---

# 138. CSRF Secret / Contract

Session 创建：

```text
csrf_secret = 32 CSPRNG bytes
base64url-no-padding
```

Frontend 仅在 Runtime Memory 持有。

禁止：

```text
localStorage
sessionStorage
IndexedDB
URL
```

所有 Cookie-authenticated：

```text
POST
PUT
PATCH
DELETE
```

必须同时通过：

```text
Valid Session
X-CSRF-Token
Origin
Fetch Metadata
```

Header：

```http
X-CSRF-Token: <token>
```

Token constant-time compare。

失败：

```text
403 CSRF_FAILED
No Business Mutation
```

---

# 139. CORS / Origin Boundary

Chaldea Browser BFF：

```text
same-origin only
```

Unsafe Browser Mutation：

```text
Origin == configured WEB_ORIGIN
```

Credentialed CORS：

```text
deny by default
```

永远禁止：

```http
Access-Control-Allow-Origin: *
Access-Control-Allow-Credentials: true
```

External NewAPI API Origin 不接收 `__Host-chaldea_session`。

---

# 140. Session Bootstrap

Endpoint：

```http
GET /api/v1/session/bootstrap
```

允许 Anonymous。

响应至少：

```text
server_time
environment
build_id

authentication_state
csrf_token

account_status
account_status_freshness

master_initialization_state
migration_notice_state

safe_identity_summary nullable
return_intent_state nullable

maintenance_summary
resource_availability_summary

operations_principal_summary nullable
```

无 Session Cookie 时：

```text
create ANONYMOUS session
set cookie
return CSRF token
```

所有响应：

```http
Cache-Control: no-store
```

Protected Data 只在 Auth/Gate 允许后出现。

---

# 141. OAuth Flow Store / State

Redis：

```text
chaldea:auth-flow:<flow_id>
```

`flow_id`：

```text
32 CSPRNG bytes
base64url
```

Record：

```text
schema_version
flow_id
purpose
state_hash
initiating_session_id_hash nullable
target_newapi_user_id nullable
return_intent_id nullable
provider_config_version
created_at
expires_at
consumed_at
```

Purpose 固定：

```text
LOGIN
REGISTRATION
FRESH_AUTH
PASSWORD_RESET
AVATAR_SYNC
```

Raw OAuth State：

```text
32 CSPRNG bytes
```

Redis 只保存：

```text
SHA-256(raw_state)
```

固定：

```text
OAUTH_FLOW_TTL = 10 minutes
single-use
purpose-bound
flow-bound
```

State 不承载：

```text
Return URL
Password
Session Token
Discord Token
Business Command
```

Consume 必须原子化：

```text
WATCH
verify state/hash/unconsumed/expiry
MULTI
mark consumed
EXEC
```

并发 Callback 最多一份成功。

---

# 142. Discord OAuth Scope / Config

按 Purpose 最小授权：

```text
LOGIN          → identify
FRESH_AUTH     → identify
PASSWORD_RESET → identify
AVATAR_SYNC    → identify

REGISTRATION
→ identify
→ guilds.members.read
```

首次注册通过当前用户 Guild Member 信息中的 `roles[]` 验证指定 Role，不额外引入 Discord Bot Token。

Non-secret Config：

```text
CHALDEA_DISCORD_CLIENT_ID
CHALDEA_DISCORD_GUILD_ID
CHALDEA_DISCORD_REQUIRED_ROLE_ID
```

Secret：

```text
examples/runtime/secrets/discord_oauth_client_secret
```

Discord API 基线：

```text
https://discord.com/api/v10
```

Callback：

```text
{WEB_ORIGIN}/api/v1/auth/discord/callback
```

只能由 Server Config 派生。

---

# 143. Discord Provider Credential Boundary

OAuth Code / Access Token：

```text
Backend Request Memory only
```

V1 不持久化：

```text
OAuth code
Discord access token
Discord refresh token
```

不得写入：

```text
ordinary log
audit
trace
```

Edge 对 OAuth Callback 的 Access Log 必须去除 Query String，避免 `code` / `state` 进入 Proxy Log。最终 Edge 配置验证进入 IS-11。

---

# 144. Discord Login

```text
POST /api/v1/auth/discord/login/start
→ purpose=LOGIN
→ Discord OAuth
→ callback
→ verify state
→ resolve Discord identity
→ Existing Binding Lookup
```

BOUND：

```text
resolve stable newapi_user_id
→ Account Status
→ issue Chaldea Authenticated Session
→ Unified Gate
```

已绑定账号登录明确不重新检查：

```text
Guild Membership
Registration Role
```

退出服务器 / 丢失首次注册 Role 不冻结既有账号。

---

# 145. Registration Ordering

首次注册固定：

```text
Discord Identity
      ↓
Existing Binding Pre-check
      │
      ├── BOUND
      │    → Existing Account Login
      │    → no Role recheck
      │    → no Registration Initial Grant
      │
      └── UNBOUND
           ↓
      Guild Membership Check
           ↓
      Required Role Check
           ↓
      New Registration
```

这是唯一允许的执行顺序。

---

# 146. Registration Durable State

`identity.registration_operations.state`：

```text
STARTED
OAUTH_STARTED
CALLBACK_VERIFIED
IDENTITY_RESOLVED
BINDING_CHECKED

EXISTING_ACCOUNT_LOGIN

ELIGIBILITY_CHECKING
ELIGIBILITY_VERIFIED

ACCOUNT_CREATING
ACCOUNT_CREATED

INITIAL_GRANT_REQUESTED

SESSION_ISSUED
PROFILE_ENSURED
POST_AUTH_GATE

COMPLETED

FAILED_NO_EFFECT
RECOVERING
NEEDS_REVIEW
```

Provider / Role Failure 必须发生在 `ACCOUNT_CREATING` 之前，不产生 Account / Grant / Provisional Profile。

安全错误：

```text
DISCORD_MEMBERSHIP_REQUIRED
DISCORD_ROLE_REQUIRED
PROVIDER_UNAVAILABLE
ACCOUNT_SUPPORT_REQUIRED
REGISTRATION_RECOVERING
REGISTRATION_NEEDS_REVIEW
```

Provider unavailable 永远不能伪装为 Role Missing。

---

# 147. Registration Idempotency

Registration 同时依赖：

```text
OAuth Flow ID
Registration Operation ID
Discord User ID uniqueness
stable NewAPI User relation
Provisional Master uniqueness
Initial Grant stable Biz ID
```

Registration Initial Grant：

```text
initial_grant:registration:{newapi_user_id}
```

最终最多：

```text
1 NewAPI Account
1 Discord Binding
1 Master Profile
1 Registration Initial Grant
```

如果 Account 已创建而 Grant 仍 PENDING/RECOVERING：

```text
keep account
issue/login session
allow Master Initialization
show truthful grant status
```

禁止删号重注册。

实际 NewAPI Account Creation / Binding Storage / Existing Binding Lookup：

```text
BLOCKED_BY_SV-04
```

---

# 148. Password Login

Endpoint：

```http
POST /api/v1/auth/password/login
```

Request：

```text
identifier
password
return_intent_id nullable
```

Password 只能瞬时存在：

```text
HTTP Request Memory
NewAPI Adapter Request Memory
```

禁止：

```text
DB
Redis
Audit
Log
Trace
Metric Label
```

Adapter：

```text
AuthPort.VerifyPassword(...)
```

Concrete implementation：

```text
BLOCKED_BY_SV-01 / SV-02
```

失败语义：

```text
Invalid identifier/password
→ 401 INVALID_CREDENTIALS

NewAPI dependency failure
→ 503 DEPENDENCY_UNAVAILABLE
```

不能通过错误差异进行 Account Enumeration。

---

# 149. Password Identifier / Set / Change / Reset

Password Login Identifier：

```text
!= Master Nickname
!= Short Account ID
!= fabricated email
```

Concrete field：

```text
BLOCKED_BY_SV-02
```

Set：

```http
POST /api/v1/account/password/set
```

要求：

```text
Authenticated Session
Fresh Auth <= 10m
New Password + Confirmation
```

Change：

```http
POST /api/v1/account/password/change
```

Request：

```text
current_password
new_password
new_password_confirmation
```

Reset：

```http
POST /api/v1/account/password/reset
```

要求：

```text
same-binding Discord PASSWORD_RESET authorization
one-time
Fresh Auth
```

Concrete NewAPI password write：

```text
BLOCKED_BY_SV-03
```

永不直接 SQL 修改 Password Hash。

---

# 150. Discord Password Reset

Flow：

```text
POST /api/v1/auth/discord/password-reset/start
→ purpose=PASSWORD_RESET
→ Discord identify
→ callback
→ Existing Binding Lookup
→ same bound account
→ Account Status
→ rotate authenticated SID
→ fresh_auth_method=DISCORD
→ create password_reset_authorization
```

Authorization：

```text
authorization_id
newapi_user_id
discord_user_id
auth_flow_id
issued_at
expires_at
consumed_at
```

固定：

```text
PASSWORD_RESET_AUTH_TTL = 10 minutes
```

Reset Secret 不进入 URL。

成功：

```text
consume authorization
→ source-verified NewAPI reset
→ rotate SID again
→ clear authorization
```

Discord A 不能 Reset Discord B 绑定的账号。

---

# 151. Fresh Authentication

固定：

```text
FRESH_AUTH_WINDOW = 10 minutes
```

判定：

```text
server_now - fresh_auth_at <= 10 minutes
```

Fresh 方法：

```text
Current Password Re-verification
or
Discord OAuth purpose=FRESH_AUTH
```

成功：

```text
rotate SID
update fresh_auth_at
update fresh_auth_method
```

Fresh Auth 完成后只恢复安全确认上下文，不自动重新执行：

```text
Asset Adjustment
Discord Rebind
Password Change
Wallet Exchange
Game Bet
Poker Action
Admin Command
```

---

# 152. Account Status / Degradation

Account Status Authority：

```text
NewAPI Account System
```

Session 只缓存：

```text
account_status_snapshot
account_status_checked_at
```

Implementation Config：

```text
ACCOUNT_STATUS_MAX_AGE
= UNRESOLVED_IMPLEMENTATION_CONFIG
```

受保护 Mutation：

```text
if cache stale
→ refresh source-verified AccountPort
```

Known ACTIVE：

```text
continue
```

Known DISABLED/SUSPENDED：

```text
Restricted Gate
```

需要 Fresh Authority 但 NewAPI 不可用：

```text
new mutation fail closed
```

已有 Chaldea Session 在 NewAPI 短暂故障时可继续安全 Chaldea-owned Read，但不得伪造 Password/Account/API Key/Quota 状态。

Restricted User 只允许：

```text
restricted status page
safe support entry
logout
```

---

# 153. Unified Access Gate

严格顺序：

```text
Authentication
↓
Account Status
↓
Master Initialization
↓
Migration Notice
↓
Role / Scope
↓
Resource Availability / Maintenance
↓
Return-to-Intent
↓
Target / Safe Parent
↓
Deferred Post-login Popup
```

Priority Test：

```text
Restricted Account + Incomplete Master
→ Restricted Account wins

Incomplete Master + Migration Notice
→ Master Initialization first

Migration Notice + Return Intent
→ Migration Notice first
```

---

# 154. Return-to-Intent

Redis：

```text
chaldea:return-intent:<intent_id>
```

ID：

```text
32 CSPRNG bytes
base64url
```

固定：

```text
RETURN_INTENT_TTL = 30 minutes
single-use
```

Record：

```text
intent_id
relative_path
safe_query
route_class
source
created_at
expires_at
consumed_at
```

只允许 same-site relative navigation。

禁止：

```text
http:// / https://
//
javascript:
data:
credentials
POST body
password
wallet amount
bet
API key command
admin command
```

Consume 前必须重新验证 Auth / Account / Master / Migration / Permission / Publication / Availability / Maintenance。

永不重放副作用操作。

---

# 155. Master Nickname Canonical Validator

Canonical 顺序：

```text
1 UTF-8 validity
2 Unicode NFKC
3 trim ordinary leading/trailing whitespace
4 collapse consecutive ordinary U+0020 spaces
5 validate grapheme/code-point policy
6 Unicode Case Fold
7 normalized folded result → uniqueness authority
```

Go：

```text
golang.org/x/text/unicode/norm.NFKC
golang.org/x/text/cases.Fold
github.com/rivo/uniseg
```

长度：

```text
1..24 Unicode Grapheme Clusters
```

允许基础：

```text
Unicode Letter
Unicode Number
valid combining Mark in text grapheme
U+0020
_
-
·
```

显式拒绝：

```text
newline
control
zero-width
bidi controls
U+200D ZWJ
U+FE0E/U+FE0F
U+20E3 keycap
Emoji/pictographic grapheme
characters outside allowed policy
```

`Alice / alice / Ａｌｉｃｅ` 必须冲突。

Reserved / Sensitive / Impersonation Validation 为 Server Authority。

Reserved Baseline 至少：

```text
Admin
Administrator
Moderator
Official
System
Support
Chaldea
NewAPI
管理员
官方
系统
客服
迦勒底
```

DB `normalized_name UNIQUE` 仍是最终并发唯一性保证。

---

# 156. Short Account ID

定义为：

> **Private Presentation Locator，非 Auth / Business Identity**

算法：

```text
canonical_id = UTF-8 opaque newapi_user_id

digest =
SHA-256(
  "chaldea-short-account-id-v1\x00"
  || canonical_id
)

short_account_id =
"CA-" +
uppercase first 12 hex chars
```

用途：

```text
Account & Security
Operations Search
Master fallback suggestion
Support
```

禁止用于：

```text
Password Login
Wallet Ownership
Poker Ownership
Authorization
Foreign Key
```

Hash Prefix Collision 时 Search 标记 Ambiguous，并要求 full stable newapi_user_id。

---

# 157. Master Initialization

认证后：

```text
ensure identity.account_refs
ensure one master_profiles row
status = INCOMPLETE
```

重复登录 / Callback / Initialize 不得创建第二 Profile。

候选昵称：

```text
1 Discord Display Name
2 Source-verified NewAPI Login Username if available
3 Master-<Short Account ID>
```

Candidate 仍必须完整 normalize / validate / uniqueness check。

Avatar Candidate：

```text
Discord Avatar Snapshot if available
else System Default Avatar
```

即使预选，用户也必须主动提交，不能自动 COMPLETE。

Endpoint：

```http
POST /api/v1/master-profile/initialize
```

Request：

```text
expected_version
display_name
avatar_selection
```

Transaction：

```text
BEGIN
lock master_profile
assert INCOMPLETE
assert expected_version
normalize + validate
enforce normalized unique
resolve avatar snapshot
update display/profile/status/completed_at/version
insert initial name history
COMMIT
```

Master Initialization 不自行发 Initial Grant；只读取真实 Registration Grant Claim 状态。

---

# 158. Master Profile Update / Rename Cooldown

Endpoint：

```http
PATCH /api/v1/master-profile
```

Request：

```text
expected_version
display_name optional
avatar_snapshot_id optional
```

必须 Row Lock + optimistic version。

Mismatch：

```text
409 STALE_RESOURCE_VERSION
```

首次 Initialization 不消费未来 Rename Cooldown。

之后用户主动改昵称：

```text
next eligible =
nickname_changed_at + 7 days
```

Server DB Time 判断。

Avatar-only Update 不更新 `nickname_changed_at`。

管理员强制 Rename Required 走 Operations Audit，不通过普通用户 Endpoint 绕过。

---

# 159. Discord Avatar Sync

Endpoint：

```http
POST /api/v1/master-profile/avatar/discord-sync/start
```

要求 Auth + CSRF。

OAuth：

```text
purpose=AVATAR_SYNC
scope=identify
```

Callback：

```text
resolved Discord User ID
==
current bound Discord User ID
```

否则：

```text
DISCORD_IDENTITY_MISMATCH
```

成功：

```text
fetch current avatar
→ create immutable candidate Discord Snapshot
→ attach candidate snapshot to current session
→ redirect Master Profile
```

不会直接修改 Current Profile。

用户 Preview 后显式 PATCH Save。

Discord Token 随后丢弃。

---

# 160. Identity Snapshot Semantics

Profile Update 不重写历史 Snapshot。

Current Profile：

```text
Rankings
```

Event-time Snapshot：

```text
Recent Wins
Historical public game event
Table Chat
```

Poker：

```text
first successful Buy-in / Seat
→ freeze Identity Display Snapshot
```

当前 Poker Session 中途改名/换头像不改变当前牌桌身份；Cash Out 后下一 Session 才使用新 Profile。

---

# 161. Account & Security Projection

Endpoint：

```http
GET /api/v1/account/security
```

安全 Projection：

```text
short_account_id

password_login_identifier:
  value nullable
  availability

password_status:
  SET
  NOT_SET
  UNKNOWN
  UNAVAILABLE

discord_connection:
  CONNECTED
  NOT_CONNECTED
  UNKNOWN
  display_name nullable

account_status:
  ACTIVE
  RESTRICTED
  UNKNOWN
  UNAVAILABLE

current_session:
  login_method
  authenticated_at
  fresh_auth_at

capabilities:
  password_set
  password_change
  password_reset
```

真实 Capability 必须来自 Source Verification。

`UNKNOWN / UNAVAILABLE` 不得显示成 `NOT_SET / NOT_CONNECTED`。

V1 明确不存在：

```text
change NewAPI username
self-service Discord unlink/rebind
email recovery
phone recovery
TOTP
Passkey
backup codes
device management center
self-service hard account delete
```

Legacy Account 无 Discord 可继续 Password Login。

---

# 162. IS-04 REST Family

固定：

```text
GET  /api/v1/session/bootstrap

POST /api/v1/auth/password/login
POST /api/v1/auth/logout
POST /api/v1/auth/fresh/password

POST /api/v1/auth/discord/login/start
POST /api/v1/registration/discord/start
POST /api/v1/auth/discord/fresh/start
POST /api/v1/auth/discord/password-reset/start

GET  /api/v1/auth/discord/callback

GET  /api/v1/registration/status

GET   /api/v1/master-profile
POST  /api/v1/master-profile/initialize
PATCH /api/v1/master-profile

POST  /api/v1/master-profile/avatar/discord-sync/start

GET  /api/v1/account/security

POST /api/v1/account/password/set
POST /api/v1/account/password/change
POST /api/v1/account/password/reset

GET  /api/v1/migration-notice
POST /api/v1/migration-notice/acknowledge
```

所有 Auth / Identity 私有 JSON 默认：

```http
Cache-Control: no-store
```

OAuth Callback 只信任 Server-side Flow `purpose/target/return_intent`。

成功 Callback 使用 `303 See Other` 回到固定 same-origin Safe Landing，URL 不带 Token / Reset Secret / newapi_user_id。

---

# 163. Poker Connect Ticket

Mint：

```http
POST /api/v1/poker/connect-tickets
```

固定：

```text
POKER_CONNECT_TICKET_TTL = 60 seconds
single-use
```

Format：

```text
ct1.<payload_base64url>.<signature_base64url>
```

Payload v1：

```text
v
kid
iss = chaldea-platform
aud = chaldea-poker
jti
newapi_user_id
session_id_hash
session_version
security_epoch_snapshot
purpose = poker_connect
target_table_id nullable
control_intent
issued_at
expires_at
```

Signature：

```text
Ed25519
sign("ct1." + payload_base64url)
```

使用 Platform Service Signing Private Key，Poker 只持 Public Verification Key。

WS：

```text
WSS /ws/poker
Sec-WebSocket-Protocol: chaldea-poker.v1
```

Ticket 不允许进入 URL Query。

---

# 164. Poker Ticket Replay / Restart

Poker Redis：

```text
chaldea:poker-ticket-used:<sha256(jti)>
```

First use：

```text
SET NX
```

Duplicate：

```text
TICKET_REPLAYED
```

Redis 不可用时：

```text
new Poker auth fail closed
```

Poker process 保存：

```text
ticket_accept_not_before = process_started_at
```

要求：

```text
ticket.issued_at >= ticket_accept_not_before
```

因此 Poker Restart 前签发的剩余 Ticket 也必须重新 Mint。

---

# 165. Logout / Poker Control Revocation

Endpoint：

```http
POST /api/v1/auth/logout
```

顺序：

```text
validate Session + CSRF
revoke current Session
invalidate CSRF
invalidate unused ticket context
request Poker control disconnect owned by session
expire cookie
return logged out
```

Logout 不：

```text
Cash Out
Safe Leave
Game Refund
Reward Cancel
Transfer Cancel
```

Active Poker Session：

```text
show warning
Return to Poker Table
or
Logout Anyway
```

Logout Anyway：

```text
control connection revoked/disconnected
Poker Session/Seat/Stack remain
Action Timer may continue
Auto Check/Fold may occur
```

Platform → Poker Typed Auth Control Contract：

```text
REVOKE_CONTROL_BY_PLATFORM_SESSION
REVOKE_CONTROLS_BY_USER_SECURITY_EVENT
```

至少绑定：

```text
operation_id
newapi_user_id
session_id_hash nullable
security_epoch
reason
```

使用既有 signed service assertion；处理幂等。

---

# 166. Auth Rate-limit Boundary

Redis Namespace：

```text
chaldea:ratelimit:auth:*
```

Identifier Bucket：

```text
HMAC-SHA-256(
  rate_limit_hmac_key,
  normalized_identifier
)
```

Raw Login Identifier 不进入 Rate-limit Key。

Implementation Config：

```text
AUTH_IP_RATE_LIMIT
AUTH_ACCOUNT_RATE_LIMIT
REGISTRATION_RATE_LIMIT
FRESH_AUTH_RATE_LIMIT
POKER_TICKET_MINT_RATE_LIMIT
```

均：

```text
UNRESOLVED_IMPLEMENTATION_CONFIG
```

Limiter Failure：

```text
Auth
Fresh Auth
Password Reset
Critical Security Flow
→ fail closed
```

---

# 167. Auth / Identity Audit Safety

记录安全事件，例如：

```text
AUTH_PASSWORD_SUCCESS
AUTH_PASSWORD_FAILURE_SAFE
AUTH_DISCORD_SUCCESS
AUTH_DISCORD_FAILURE_SAFE
AUTH_SESSION_ROTATED
AUTH_LOGOUT
AUTH_FRESH_SUCCESS
AUTH_FRESH_FAILURE

REGISTRATION_STARTED
REGISTRATION_ELIGIBLE
REGISTRATION_COMPLETED
REGISTRATION_FAILED_SAFE

PASSWORD_SET
PASSWORD_CHANGED
PASSWORD_RESET

MASTER_INITIALIZED
MASTER_PROFILE_CHANGED

SECURITY_EPOCH_CHANGED
```

禁止：

```text
Password
Password Hash
Raw SID
CSRF Secret
OAuth Code
OAuth Access Token
Raw OAuth State
Poker Ticket
```

---

# 168. Migration Notice

Migration Notice 是独立 Post-auth Gate，不是普通 Announcement。

顺序：

```text
Account Status
→ Master Initialization
→ Migration Notice
```

Endpoint：

```http
GET /api/v1/migration-notice
POST /api/v1/migration-notice/acknowledge
```

Ack 使用：

```text
INSERT acknowledgement
ON CONFLICT(user, version)
→ return original acknowledgement
```

每 User / Version exactly-once。

Ack 永远不：

```text
reset quota
issue grant
create profile
migrate API keys
```

Cutover 必须在用户首次进入前已经完成。

---

# 169. Product Cutover Binary

独立 Binary：

```text
dist/bin/chaldea-cutover
backend/cmd/chaldea-cutover/main.go
```

不是：

```text
Platform HTTP Handler
Operations arbitrary command
runtime startup migration
```

永久禁止：

```text
POST /api/v1/cutover/run
```

CLI：

```text
chaldea-cutover status --batch <uuid>

chaldea-cutover run
  --batch <uuid>
  --scope-manifest <path>

chaldea-cutover verify --batch <uuid>

chaldea-cutover open --batch <uuid>
```

每次执行：

```text
read durable state
→ execute only next legal transition
→ persist
→ continue
```

Crash 后相同命令从 Durable Fact Resume。

---

# 170. Cutover Singleton / Preconditions

Cutover 首先获取 PostgreSQL Advisory Lock：

```text
domain = chaldea-cutover-v1
```

失败：

```text
CUTOVER_ALREADY_RUNNING
```

Production Precheck 必须通过：

```text
environment = PRODUCTION

required NewAPI source verification
SV-04
SV-05
SV-06 as relevant
SV-08
SV-09
SV-10 as required
SV-13

DEPLOYMENT-VERIFY-01

release manifest
migration checksum
backup manifest
```

缺失：

```text
CUTOVER_PRECHECK_BLOCKED
```

---

# 171. Cutover Scope Manifest / Accepted-work Drain

Scope Manifest 不包含 Secret。

必须至少：

```text
manifest_version
migration_batch_id
environment
expected_user_count
ordered stable newapi_user_id list
newapi_source_identity
newapi_image_digest
chaldea_release_id
chaldea_build_id
schema_migration_version
created_at
sha256
```

Hash 写入 `migration.cutover_batches.scope_hash`，同 Batch 之后不接受另一 Scope。

Snapshot 前阻止新资产写并等待 Accepted Work 达到安全点。

至少检查：

```text
Cross-DB Transfers terminal or blocking NEEDS_REVIEW
accepted Reward Claims safe/recoverable
accepted Direct Play settled/refunded or blocking incident
Poker no active asset-changing session
Poker cashout complete
no unsettled hand
no pending funding
```

NewAPI Registration / charging freeze：

```text
BLOCKED_BY_SV-08
```

---

# 172. Cutover Backup Gate

Cutover 消费 IS-11 Backup Runbook 的 Verified Backup Manifest。

至少证明：

```text
newapi DB backup
chaldea_platform DB backup
role/grant metadata
NewAPI quota export snapshot
migration manifest
release/build manifest
scope manifest
```

`backup_verified = true` 后才进入 `BACKUP_READY`。

Redis 不作为金融恢复 Authority。

---

# 173. Per-user Cutover Ordering

V1 用户量小，固定：

> **Deterministic Sequential Migration**

排序：

```text
canonical newapi_user_id string ascending
```

每 User：

```text
1 ensure cutover_user row
2 capture pre-cutover quota snapshot
3 ensure account_ref
4 verify Chaldea Reserve/Chips/Poker initialization = 0
5 reset NewAPI active quota = 0
6 read NewAPI quota again
7 mark RESET_VERIFIED
8 ensure migration reward claim
9 query claim/economy by stable biz id
10 confirm grant
11 verify final user invariants
12 mark VERIFIED
```

Quota mismatch：

```text
current = 0
+ same batch
+ matching snapshot/original operation
→ continue

otherwise
→ NEEDS_REVIEW
```

禁止 Blind Reset。

Concrete reset：

```text
BLOCKED_BY_SV-09 / SV-10
```

---

# 174. Migration Initial Grant / Existing API Keys

Migration Grant Biz ID：

```text
initial_grant:migration:
{migration_batch_id}:
{newapi_user_id}
```

金额：

```text
1000 API Credit
= 500,000,000 atomic units
```

目标：

```text
RESERVE_API_CREDIT
```

必须经：

```text
Reward Claim
Economy Transaction
Wallet Ledger
```

Cutover Orchestrator 不直接 SET 最终余额。

Existing NewAPI API Keys：

```text
preserve unchanged
```

Chaldea Purpose：

```text
if metadata absent → UNCLASSIFIED
if GENERAL/RP exists → preserve
```

不得 regenerate/delete/change Secret。

---

# 175. Cutover Verification / Open

进入 `READY_TO_OPEN` 前至少：

```text
all scoped users preserved
account_refs complete
Discord binding integrity
password/account reference integrity
API key integrity

all scoped NewAPI active quota = 0

without later economic facts:
Reserve API Credit = 1000
Available Chips = 0
Poker In Play = 0
Total Assets = 1000

exactly one Migration Initial Grant/user

FAILED = 0
NEEDS_REVIEW = 0
unverified user = 0
duplicate biz identity = 0
```

生成 `migration.validation_reports`。

`READY_TO_OPEN` 表示：

```text
migration verified
but traffic still closed
```

只有：

```text
chaldea-cutover open --batch <id>
```

可以：

```text
re-read validation
re-read batch state
verify no new blocker
perform source-verified unfreeze/open
release Chaldea cutover maintenance
verify availability
mark COMPLETED
```

VERIFY 成功不能自动开流量。

---

# 176. Cutover Rollback Boundary

流量尚未开放：

```text
resume same batch
```

优先。

无法 Resume：

```text
ROLLBACK_REQUIRED
→ full authority backup restore
→ verify
→ ROLLED_BACK
```

开站后若已经产生：

```text
API consumption
Reward
Exchange
Game Round
Poker mutation
Admin Adjustment
```

禁止只恢复旧 NewAPI Quota。

进入：

```text
REPAIR_REQUIRED
```

通过正式 Migration Repair / Economy Compensation / Audit 处理。

---

# 177. Initial Super Admin Bootstrap

Binary：

```text
dist/bin/chaldea-bootstrap
backend/cmd/chaldea-bootstrap/main.go
```

只负责：

```text
INITIAL SUPER ADMIN BOOTSTRAP
```

不成为通用 Admin CLI。

推荐：

```text
docker compose
  --profile bootstrap
  run --rm chaldea-bootstrap
  --environment PRODUCTION
  --newapi-user-id '<stable-id>'
  --expected-empty
```

Secret 不放 CLI 参数；使用挂载：

```text
platform_db_dsn
newapi_adapter_credential
```

流程：

```text
verify environment
verify source compatibility
NewAPI AccountPort → target user exists

BEGIN
acquire bootstrap advisory lock
assert admin_principal count = 0
ensure account_ref
create SUPER_ADMIN principal
append role history
append SYSTEM_BOOTSTRAP audit
COMMIT
```

一旦：

```text
admin principal count > 0
```

永久：

```text
BOOTSTRAP_ALREADY_CLOSED
```

不提供：

```text
--force
--reset
--replace-admin
```

后续管理员全部走正式 Level 3 Access Control。

Target User existence 继续 Source Verification Blocked；未验证时 Bootstrap 拒绝。

永久不存在：

```text
/api/v1/bootstrap
/api/v1/bootstrap-super-admin
/api/v1/setup-admin
```

---

# 178. IS-04 Implementation Config Register Additions

新增：

```text
SESSION_IDLE_TTL
SESSION_ABSOLUTE_TTL
SESSION_TOUCH_INTERVAL

ACCOUNT_STATUS_MAX_AGE

AUTH_IP_RATE_LIMIT
AUTH_ACCOUNT_RATE_LIMIT
REGISTRATION_RATE_LIMIT
FRESH_AUTH_RATE_LIMIT
POKER_TICKET_MINT_RATE_LIMIT
```

状态：

```text
UNRESOLVED_IMPLEMENTATION_CONFIG
```

已经冻结为固定技术值、不得放入该 Register：

```text
ANONYMOUS_SESSION_TTL = 30m
OAUTH_FLOW_TTL = 10m
RETURN_INTENT_TTL = 30m
FRESH_AUTH_WINDOW = 10m
PASSWORD_RESET_AUTH_TTL = 10m
POKER_CONNECT_TICKET_TTL = 60s
```

---

# 179. Codex IS-04 Implementation Order

Implementation Spec FINAL 后：

```text
01 lock pgx / go-redis / x-text / uniseg

02 session types/store/cookie
03 session rotation
04 CSRF
05 return-intent
06 OAuth flow store
07 Discord adapter

08 NewAPI AuthPort blocked implementations
09 password login
10 fresh auth
11 password reset authorization

12 account status gate
13 security_epoch
14 session bootstrap

15 nickname normalization/validation
16 Short Account ID
17 Master Initialization
18 Master Update
19 Discord Avatar Sync
20 Account Security projection

21 Migration Notice

22 Poker Ticket
23 auth revocation internal contract
24 Logout

25 Super Admin bootstrap CLI

26 Cutover orchestrator
27 Cutover verification
28 explicit Open Gate

29 execute IS-04 test gate
```

NewAPI Source 未提供时，Concrete NewAPI Adapter 在 Blocked Boundary 停止；其余 Chaldea-owned 部分可实现。

---

# 180. IS-04 Prohibited Implementation

Codex 禁止：

```text
guess NewAPI password endpoint
guess NewAPI login identifier
guess binding table/column
guess quota field

copy/store password hash
persist password
persist Discord OAuth token

use Master nickname as login

recheck Role for existing bound account
reissue Initial Grant to existing bound account

auto-complete Master
auto-save nickname suggestion

invent self-service Discord rebind
invent Email/TOTP/Passkey recovery

store auth token in localStorage

put Poker Ticket in WS URL

make Migration Notice execute migration

run Product Cutover at normal backend startup
expose Cutover over HTTP

expose Super Admin bootstrap over HTTP

automatically reopen platform after verification
```

---

# 181. IS-04 Test Gate

## Session

```text
SID entropy / uniqueness

cookie exact attributes

anonymous SID cannot promote
login rotates SID
fresh auth rotates SID
fresh auth does not reset absolute chain

old SID rejected
old CSRF rejected

Redis flush
→ re-login
→ durable business facts unchanged

security_epoch++
→ stale sessions rejected
```

## CSRF / OAuth

```text
missing/wrong CSRF rejected
wrong Origin rejected
cross-site Fetch Metadata rejected

OAuth valid once
duplicate callback rejected
expired rejected
wrong purpose rejected
wrong session binding rejected

callback logs contain no code/state/token
```

## Discord Registration

```text
BOUND user without current Role
→ login succeeds
→ no grant

UNBOUND + no guild
→ no account

UNBOUND + guild/no role
→ no account

UNBOUND + role
→ one account path

100 replay/concurrent callbacks
→ one account/binding/profile/grant

provider unavailable
→ PROVIDER_UNAVAILABLE

account created + lost response
→ resume same operation
```

## Password / Fresh Auth

```text
wrong identifier/password
→ same public INVALID_CREDENTIALS

NewAPI unavailable
→ DEPENDENCY_UNAVAILABLE

password absent from logs/trace/audit/Redis

fresh <=10m accepted
fresh >10m rejected

fresh auth never auto-replays mutation

Discord reset same-binding only
reset authorization single-use
```

## Master

```text
Alice / alice / Ａｌｉｃｅ conflict

1 grapheme accepted if valid
24 accepted
25 rejected

Emoji/ZWJ/zero-width/bidi/newline rejected
reserved names rejected

100 concurrent same normalized name
→ one success

rename:
T+6d23:59:59 reject
T+7d eligible

Avatar-only change
→ cooldown unchanged

stale expected_version
→ 409
```

## Gate / Return Intent

```text
external/scheme-relative/javascript target rejected
POST body never stored
financial/game/admin mutation never replayed
expired/single-use behavior correct
permission/maintenance revalidated
priority order correct
```

## Poker Auth / Logout

```text
expired/wrong-aud/wrong-origin/wrong-protocol ticket rejected
same jti replay rejected
pre-restart ticket rejected
Redis unavailable → new auth fail closed
security_epoch blocks new ticket and revokes old control
logout with active Poker leaves wallet/stack/session authority untouched
```

## Bootstrap

```text
count=0 + valid target
→ one SUPER_ADMIN
→ one role history
→ one SYSTEM_BOOTSTRAP audit

concurrent bootstrap
→ one success

admin exists
→ permanently refuse

invalid target/source unverified
→ no DB effect

no HTTP bootstrap route
```

## Cutover

```text
backup/SV missing → block

same batch repeated/restarted
→ no duplicate snapshot/reset/grant

quota mismatch → NEEDS_REVIEW
no blind reset

one migration grant/user

keys preserved
purpose preserved or absent→UNCLASSIFIED

Migration Notice Ack → no economic effect

verification failure → no READY_TO_OPEN

READY_TO_OPEN → traffic still closed

explicit open only → COMPLETED

post-open economic facts
→ old-quota-only restore prohibited
```

---

# 182. IS-04 Acceptance Criteria

```text
AC-04-01  opaque server-side browser session
AC-04-02  exact __Host cookie hardening
AC-04-03  256-bit CSPRNG SID + hash-only Redis key
AC-04-04  anonymous SID cannot promote
AC-04-05  CSRF + Origin + Fetch Metadata
AC-04-06  no auth token persistent browser storage
AC-04-07  session TTL unknowns preserved
AC-04-08  security_epoch revokes stale sessions
AC-04-09  authz_epoch remains DB authorization authority
AC-04-10  OAuth State purpose-bound/256-bit/10m/one-time
AC-04-11  Discord provider tokens transient
AC-04-12  registration-only guild member scope
AC-04-13  existing binding before Role eligibility
AC-04-14  existing bound account no second registration grant
AC-04-15  durable idempotent registration
AC-04-16  source-dependent account/binding mapping blocked
AC-04-17  password never enters Chaldea credential storage
AC-04-18  no account enumeration
AC-04-19  Master/Short ID never login identifier
AC-04-20  password writes source-verified
AC-04-21  same-binding one-time Discord reset
AC-04-22  10-minute server fresh-auth evidence
AC-04-23  fresh auth never auto-replays mutation
AC-04-24  frozen gate order
AC-04-25  restricted account blocked from normal business
AC-04-26  dependency outage never fabricates account facts
AC-04-27  30m single-use navigation-only Return Intent
AC-04-28  NFKC/space/casefold normalization
AC-04-29  1–24 grapheme validation
AC-04-30  emoji/control/zero-width/bidi rejection
AC-04-31  DB normalized-name uniqueness
AC-04-32  7-day server rename cooldown
AC-04-33  avatar update does not consume rename cooldown
AC-04-34  Discord avatar same-binding + explicit save
AC-04-35  Short ID presentation-only
AC-04-36  Master Initialization mandatory/idempotent
AC-04-37  Master Initialization never duplicates grant
AC-04-38  Account Security represents Unknown honestly
AC-04-39  Ed25519 60s purpose/session/user-bound Poker Ticket
AC-04-40  Poker ticket replay/restart protection
AC-04-41  Logout never implicit Cash Out
AC-04-42  Migration Notice Ack has zero migration effect
AC-04-43  Cutover deployment CLI only
AC-04-44  Cutover requires Backup/SV/Deployment Gates
AC-04-45  deterministic idempotent per-user cutover
AC-04-46  Migration Grant via Reward/Economy/Ledger
AC-04-47  READY_TO_OPEN never auto-opens traffic
AC-04-48  Super Admin deployment one-shot CLI only
```

---

# 183. IS-04 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| IS-FRZ-113 | Chaldea Browser Auth 使用 Server-side Opaque Session，并增加 ANONYMOUS Pre-auth Session 以支持 Bootstrap/CSRF/OAuth；匿名 SID 永不直接晋升为 Authenticated SID。 | FROZEN |
| IS-FRZ-114 | Cookie 固定为 `__Host-chaldea_session` + Secure/HttpOnly/SameSite=Lax/Path=/No Domain；V1 无 Remember Me，浏览器持久存储不得保存 Auth Token。 | FROZEN |
| IS-FRZ-115 | Session Raw SID 使用 32-byte CSPRNG + Base64URL；Redis Key 只使用 SHA-256 SID Hash，Raw SID 永不进入 Log/Audit/Metric。 | FROZEN |
| IS-FRZ-116 | Session/Auth Flow/Return Intent 使用 Redis Ephemeral Authority；Redis 丢失只导致重新认证，不改变 Account/Master/Wallet/Poker Durable Facts。 | FROZEN |
| IS-FRZ-117 | Authenticated Session 支持 Idle/Absolute/Touch/Revoke/Rotation；三个精确 Lifetime 参数继续 `UNRESOLVED_IMPLEMENTATION_CONFIG + SV-01`，Fresh Auth Rotation 不重置原 Absolute Auth Chain。 | FROZEN |
| IS-FRZ-118 | Login/Fresh Auth/Password Reset 等 Authentication Strength Change 必须 Rotation SID 与 CSRF Secret，旧 SID/CSRF 立即失效。 | FROZEN |
| IS-FRZ-119 | Session 固化 `security_epoch_snapshot`，每个 Protected Request 与 PostgreSQL 当前 `identity.account_refs.security_epoch` 比较；Mismatch 立即撤销。 | FROZEN |
| IS-FRZ-120 | Operations `authz_epoch/role/scope` 不以 Session Cache 为权限 Authority；Ops Backend 每次按当前 Chaldea RBAC Durable State 授权。 | FROZEN |
| IS-FRZ-121 | OAuth Flow 采用 32-byte Random Flow ID + Purpose-bound Server-side Record，固定 LOGIN/REGISTRATION/FRESH_AUTH/PASSWORD_RESET/AVATAR_SYNC 五类。 | FROZEN |
| IS-FRZ-122 | OAuth State 使用 32-byte CSPRNG、Hash-only Server Storage、10 分钟 TTL、Atomic Single-use Consume；不得承载 Return URL 或业务命令。 | FROZEN |
| IS-FRZ-123 | Discord OAuth Code/Access Token 仅短期存在 Backend Memory，V1 不持久化 Provider Credential；OAuth Callback Access Log 必须移除 Query String。 | FROZEN |
| IS-FRZ-124 | Discord Purpose 使用最小 Scope：LOGIN/FRESH/PASSWORD_RESET/AVATAR_SYNC=`identify`，REGISTRATION=`identify + guilds.members.read`，以当前用户 Guild Member 的 `roles[]` 验证首次注册 Role。 | FROZEN |
| IS-FRZ-125 | Discord Registration 严格 Existing Binding First；BOUND 直接登录且不重验 Role，UNBOUND 才验证 Guild/Role。 | FROZEN |
| IS-FRZ-126 | Registration 使用 Durable State + OAuth/Operation/Binding/Profile/Grant 多层幂等；重复 Callback/Tab/Restart 最终最多一个 Account/Binding/Profile/Initial Grant。 | FROZEN |
| IS-FRZ-127 | NewAPI Account Creation / Discord Binding / Existing-binding concrete mapping 继续 `BLOCKED_BY_SV-04`，实现不得猜 Endpoint/Table/Column。 | FROZEN |
| IS-FRZ-128 | Password Login 只调用 source-verified NewAPI AuthPort；Password 只存在 Request Memory，Invalid Credential 与 Dependency Failure 严格分离。 | FROZEN |
| IS-FRZ-129 | Password Login Identifier 继续 `BLOCKED_BY_SV-02`；Master Nickname、Short Account ID 与伪 Email 永不得替代。 | FROZEN |
| IS-FRZ-130 | Password Set/Change/Reset 分别通过 SV-03 Verified Capability；Chaldea 永不读取/复制/SQL 修改 NewAPI Password Hash。 | FROZEN |
| IS-FRZ-131 | Discord Password Reset 使用同 Binding Discord Re-auth + Session-bound One-time Authorization，TTL=10m；成功后 consume + SID rotate，无 Reset Secret URL。 | FROZEN |
| IS-FRZ-132 | Fresh Authentication Window 固定 10 分钟，由 Server Session Evidence 计算；成功必须 Rotation，且不能自动重放原 Critical Mutation。 | FROZEN |
| IS-FRZ-133 | Account Status 继续 NewAPI Authority；Session Snapshot 仅 Cache，`ACCOUNT_STATUS_MAX_AGE` 为 Implementation Config；Fresh Status 不可用时 New Mutation Fail Closed。 | FROZEN |
| IS-FRZ-134 | Unified Access Gate 固定 Account Status → Master Initialization → Migration Notice → Role/Scope → Resource/Maintenance → Return Intent。 | FROZEN |
| IS-FRZ-135 | Return-to-Intent 使用 32-byte Opaque ID、Server-side Redis、30m TTL、Single-use，只保存 Safe Relative Navigation；永不保存或重放副作用请求。 | FROZEN |
| IS-FRZ-136 | Cookie-auth Unsafe BFF Request 必须同时通过 Session、Synchronizer CSRF、Origin 与 Fetch Metadata；CSRF Secret 32-byte/每 SID 独立，Same-origin Credentialed CORS default-deny。 | FROZEN |
| IS-FRZ-137 | `GET /api/v1/session/bootstrap` 允许 Anonymous，并在缺失 Cookie 时建立 Pre-auth Session；所有 Auth/Identity 私有响应默认 `Cache-Control: no-store`。 | FROZEN |
| IS-FRZ-138 | Master Nickname Canonical Normalization 固定为 UTF-8 → NFKC → trim/collapse ordinary space → server validation → Unicode Case Fold；Backend 锁 `x/text v0.41.0` 与 `uniseg v0.4.7`。 | FROZEN |
| IS-FRZ-139 | Master Nickname 强制 1–24 Unicode Grapheme；仅允许正常文字/数字/空格/_/-/· 及合法组合 Mark，拒绝 Emoji、控制、Zero-width、Bidi、换行与注入字符。 | FROZEN |
| IS-FRZ-140 | Reserved/Sensitive/Impersonation Validation 为 Server Authority，并通过 `reserved_master_names` 管理；昵称最终唯一性继续由 DB `normalized_name UNIQUE` 保证。 | FROZEN |
| IS-FRZ-141 | Short Account ID 固定为 `CA-` + domain-separated SHA-256(newapi_user_id) 前 12 个大写 Hex；仅 Presentation/Support/Search，不参与 Auth、FK、资产或权限。 | FROZEN |
| IS-FRZ-142 | Master Initialization 幂等确保一个 INCOMPLETE Provisional Profile；候选昵称优先 Discord Display → Verified NewAPI Username → `Master-<Short ID>`，用户必须主动确认才能 COMPLETE。 | FROZEN |
| IS-FRZ-143 | Master Initialization 与 Registration Initial Grant 解耦；Grant 使用 `initial_grant:registration:{newapi_user_id}` Reward/Economy Path，Initialization 只显示真实 Claim 状态。 | FROZEN |
| IS-FRZ-144 | Master Profile Mutation 使用 `expected_version` + Row Lock；用户改名成功后 7 天 Server-time Cooldown，Avatar-only Update 不推进 Cooldown。 | FROZEN |
| IS-FRZ-145 | V1 Avatar 仅 SYSTEM / DISCORD_SNAPSHOT；Discord Sync 新增 `/master-profile/avatar/discord-sync/start`，必须验证当前绑定 Discord、生成 Snapshot、Preview 后显式 Save，不自动跟随 Discord。 | FROZEN |
| IS-FRZ-146 | Identity Display Snapshot 为 Event-time Immutable Fact；Profile 改名不重写 Poker/Chat/Recent Wins 等既有 Snapshot。 | FROZEN |
| IS-FRZ-147 | Account & Security 只投影真实 Identifier/Discord/Password/Session/Account Capability；UNKNOWN/UNAVAILABLE 不得伪装为 NOT_SET/Disconnected。 | FROZEN |
| IS-FRZ-148 | IS-04 固定 Auth/Master/Account REST Family，并增加 Avatar Discord Sync Start；OAuth Purpose 永远从 Server Flow 取值而非 Client Query。 | FROZEN |
| IS-FRZ-149 | Poker Connect Ticket 固定 `ct1` Ed25519 Signed Envelope，60s TTL，绑定 user/session/security_epoch/purpose/target，浏览器不在 WS URL 传 Ticket。 | FROZEN |
| IS-FRZ-150 | Poker Ticket 使用 JTI Single-use Redis Replay Guard；Redis 不可用时新 WS Auth Fail Closed，Poker Restart 拒绝 Restart 前签发的剩余 Ticket。 | FROZEN |
| IS-FRZ-151 | Logout 只撤销当前 Chaldea Session 与其 Poker Control Connection；永不隐式 Safe Leave/Cash Out/Refund/Cancel，Active Poker Session 继续保持 Durable Fact。 | FROZEN |
| IS-FRZ-152 | Platform→Poker 增加 Typed Auth Control Revocation Contract；Identity Recovery/Rebind 必须以 `security_epoch` + Control Revocation 收敛旧认证能力。 | FROZEN |
| IS-FRZ-153 | Auth Rate-limit Identifier 使用 HMAC-safe Bucket Key；精确阈值继续 Implementation Config，Redis Limiter 故障时 Auth/Fresh/Reset/Critical Security Flow Fail Closed。 | FROZEN |
| IS-FRZ-154 | Migration Notice 是独立 Versioned Post-auth Gate；每 user/version Ack exactly-once，Ack 永远不执行 Reset/Grant/Key Migration。 | FROZEN |
| IS-FRZ-155 | Product Cutover 只通过 `chaldea-cutover` Deployment CLI 执行，使用 Durable Batch State + PostgreSQL singleton lock，可从原状态 Resume，永不提供公网 Cutover Endpoint。 | FROZEN |
| IS-FRZ-156 | Production Cutover 前强制 required SV、Deployment Verify、Release/Migration/Backup Manifest 与 Accepted-work Drain；NewAPI Freeze/Quota Reset 继续由 SV-08/SV-09/SV-10 决定。 | FROZEN |
| IS-FRZ-157 | V1 Cutover User 按 canonical newapi_user_id 顺序串行处理；Quota 与 Snapshot 不一致进入 NEEDS_REVIEW，禁止 Blind Reset。 | FROZEN |
| IS-FRZ-158 | Migration Grant 固定通过 Batch-aware Reward/Economy/Ledger Biz ID 发放，Existing API Keys 原样保留，仅缺 Purpose Metadata 时初始化 UNCLASSIFIED。 | FROZEN |
| IS-FRZ-159 | `READY_TO_OPEN` 仍保持流量关闭，只有显式 `chaldea-cutover open` 完成最终 Verify/Unfreeze/Open；开站后产生新经济事实则禁止单独恢复旧 Quota。 | FROZEN |
| IS-FRZ-160 | Initial Super Admin 固定 Deployment-only `chaldea-bootstrap` One-shot CLI：仅 admin count=0 时可创建一个 SUPER_ADMIN + Role History + SYSTEM_BOOTSTRAP Audit；一旦存在管理员永久关闭且无 HTTP Bootstrap Route。 | FROZEN |

---

# 184. Open / Blocked Register after IS-04

```text
Product Decision Blocker:
- Reward OPEN
- Poker Product Gap 01～05
- Public Record Selection Policy

NewAPI Source Verification:
- SV-01 ～ SV-16
- Status = BLOCKED_BY_NEWAPI_SOURCE_VERIFY

Database SQL Finalization:
- @NEWAPI_USER_ID_PG_TYPE = BLOCKED_BY_SV-05
- @NEWAPI_KEY_ID_PG_TYPE = BLOCKED_BY_SV-05 / SV-06

Auth / Account Source Facts:
- Password Verify = BLOCKED_BY_SV-01
- Password Login Identifier = BLOCKED_BY_SV-02
- Password Set/Change/Reset = BLOCKED_BY_SV-03
- Discord Binding mapping = BLOCKED_BY_SV-04
- Account Status concrete capability = BLOCKED_BY_NEWAPI_SOURCE_VERIFY

Cutover Source Facts:
- NewAPI Write Freeze = BLOCKED_BY_SV-08
- Raw Quota Reset = BLOCKED_BY_SV-09
- Quota Idempotency/Recovery = BLOCKED_BY_SV-10
- NewAPI Adapter Auth = BLOCKED_BY_SV-13

Deployment Verification:
- DEPLOYMENT-VERIFY-01 = PENDING

Implementation Configuration:
- SESSION_IDLE_TTL = UNRESOLVED_IMPLEMENTATION_CONFIG
- SESSION_ABSOLUTE_TTL = UNRESOLVED_IMPLEMENTATION_CONFIG
- SESSION_TOUCH_INTERVAL = UNRESOLVED_IMPLEMENTATION_CONFIG
- ACCOUNT_STATUS_MAX_AGE = UNRESOLVED_IMPLEMENTATION_CONFIG
- Auth/Registration/Fresh/Poker Ticket rate-limit production thresholds = UNRESOLVED_IMPLEMENTATION_CONFIG
```

没有任何 Source Fact / Product OPEN 被 IS-04 默认值替代。

---

# 185. Change Log — WORKING v0.4

## Added

- 用户正式确认 `IS-04 — Auth / Session / Master / Account / Cutover Implementation Specification`；
- 冻结 `IS-FRZ-113 ～ IS-FRZ-160`；
- 冻结 Chaldea Opaque Session / Anonymous Pre-auth Session；
- 冻结 `__Host-chaldea_session` Cookie；
- 冻结 SID/CSRF/OAuth/Return Intent Randomness 与 Hash Boundary；
- 冻结 security_epoch / authz_epoch 使用边界；
- 冻结 Discord Purpose-bound OAuth 与 Existing Binding First；
- 冻结 Durable Registration State / Idempotency；
- 冻结 Password Login / Set / Change / Reset Adapter Boundary；
- 冻结 10m Fresh Auth；
- 冻结 Unified Access Gate / Return-to-Intent；
- 冻结 Master Nickname Normalization / Grapheme / Reserved Name / 7d Cooldown；
- 冻结 Short Account ID；
- 冻结 Master Initialization / Discord Avatar Snapshot Sync；
- 冻结 Account Security Honest Projection；
- 冻结 Poker Connect Ticket / Replay / Restart / Logout Revocation；
- 冻结 Migration Notice Ack Zero-side-effect；
- 冻结 Deployment-only `chaldea-cutover` CLI；
- 冻结 `READY_TO_OPEN` explicit Open Gate；
- 冻结 Deployment-only `chaldea-bootstrap` One-shot Initial Super Admin；
- 新增 IS-04 Implementation Config Register 项；
- NewAPI-dependent facts继续保持 Source Verification Blocked。

## Preserved

```text
TD-FRZ-001 ～ TD-FRZ-552
IS-FRZ-001 ～ IS-FRZ-160

Reward OPEN
Poker Product Gap 01～05
Public Record Selection Policy

SV-01 ～ SV-16 unresolved facts
DEPLOYMENT-VERIFY-01 pending
Implementation-only Config unresolved values
Production Readiness gates
```

## Supersession

```text
Existing IS-FRZ superseded:
None
```

---

# 186. Next Batch

> **IS-05 — Economy / Rewards Implementation Specification**

IS-05 将正式把：

```text
Wallet
Ledger
Asset Transaction
API Credit ↔ Chips Exchange
Cross-DB Quota Saga
Active Liquidity / Reserve Refill
NewAPI Quota Bridge Idempotency
Reconciliation

Registration Initial Grant
Migration Initial Grant

Daily Claim
Hourly technical model
Relief eligibility / cooldown

Reward Recovery
Economy / Reward Lock Ordering
Workers
SQL query contract
Property / crash-point tests
```

落到 Go Service / Transaction / Query / Lock / Worker / Retry / Test 粒度。

仍然不会为 Reward OPEN 字段填 Product Default，也不会猜 NewAPI Quota Endpoint / Table / Column。

---

# 187. IS-05 — Economy / Rewards Implementation Specification

> 状态：`FROZEN`  
> 用户确认：`整体按上述 IS-05 方案通过`  
> Frozen Decision Range：`IS-FRZ-161 ～ IS-FRZ-216`  
> NewAPI Quota Facts：`SV-09 / SV-10 / SV-11 / SV-13 = BLOCKED_BY_NEWAPI_SOURCE_VERIFY`

## 187.1 Purpose

IS-05 将已冻结的 Economy / Reward 模型落成可直接编码的：

```text
Wallet / Ledger / Asset Transaction
Exact Decimal Parser
Durable Idempotency
Exchange
Cross-DB Transfer Saga
Quota Effect Operation
Processing Assets
Strict Authoritative Asset Snapshot
Reconciliation
Compensation
Active Liquidity Controller

Registration Initial Grant
Migration Initial Grant
Daily
Hourly technical model
Relief technical model
Reward Recovery
```

本批不猜 NewAPI Quota Endpoint / Table / Column，不替用户决定 Reward Product OPEN。

---

# 188. Asset Authority / Atomic Unit

Authority 固定：

```text
Reserve API Credit
→ Chaldea Economy

Active API Quota
→ NewAPI

Available Chips
→ Chaldea Economy

Poker In Play
→ Poker Durable Authority

Processing Assets
→ Economy Transfer / Leg Durable Facts
```

Poker In Play：

```text
Table Stack
+
Committed This Hand
```

NewAPI Active 不建立 Chaldea 第二可写余额。

账务单位：

```text
QUOTA_PER_CREDIT = 500,000

1 API Credit
= 500,000 atomic units

1 Entertainment Chip
= 500,000 atomic units

1 NewAPI raw quota
= 1 Chaldea atomic unit
```

所有资产事实使用 `BIGINT / int64`；禁止 float32/float64/REAL/DOUBLE PRECISION。

---

# 189. Exact Decimal Parser / Formatter

Client Amount 为 String。

输入：

```text
positive decimal
maximum 6 fractional digits
no exponent notation
no NaN/Infinity
```

解析：

```text
micro_units = decimal * 1,000,000

must:
micro_units % 2 == 0

atomic_units = micro_units / 2
```

示例：

```text
0.0372   → 18,600 units
0.000002 → 1 unit
0.000001 → AMOUNT_NOT_REPRESENTABLE
```

必须使用整数或 `math/big.Int`；禁止浮点乘法与静默舍入。

输出：

```text
atomic_units = integer string
amount       = canonical decimal string
```

所有资产 JSON 值保持 String，不依赖 JavaScript Number。

---

# 190. Backend Package Boundary

```text
backend/internal/economy/
├── service.go
├── amount.go
├── asset.go
├── snapshot.go
├── wallet/
├── ledger/
├── transaction/
├── exchange/
├── transfer/
├── quota/
├── liquidity/
├── reconciliation/
├── adjustment/
└── supply/

backend/internal/rewards/
├── service.go
├── policy/
├── claim/
├── grant/
├── daily/
├── hourly/
├── relief/
├── recovery/
└── projection/
```

不建立无边界 `helpers/utils` 作为业务状态机容器。

---

# 191. IS-05 Additive Migrations

不修改 IS-03 的 `000000 ～ 000017`。

新增：

```text
000018__economy_rewards_runtime_refinements
000019__reward_policy_seed_v1
```

`000018` 至少补齐：

```text
platform_meta.mutation_idempotency_records

economy.transfers:
  plan_hash
  state_version
  attempt_count
  last_attempt_at
  next_attempt_at
  last_error_category

economy.transfer_legs:
  local_ledger_entry_id
  external_event_id

economy.wallet_ledger:
  reverses_entry_id

rewards.policy_versions:
  policy_hash
  validated_at
  activated_at
  retired_at
```

并建立：

```text
one INITIAL_GRANT_REGISTRATION / user
one INITIAL_GRANT_MIGRATION / user
one ACTIVE policy / reward_kind
one nonterminal Active-quota movement / user
```

Production SQL 仍受 `@NEWAPI_USER_ID_PG_TYPE` / `@NEWAPI_KEY_ID_PG_TYPE` Source Verification 阻断。

---

# 192. Durable Mutation Idempotency

新增：

```text
platform_meta.mutation_idempotency_records

idempotency_record_id UUID
newapi_user_id
scope
key_hash
request_hash
resource_type
resource_id
created_at
```

Unique：

```text
(newapi_user_id, scope, key_hash)
```

Raw `Idempotency-Key` 不持久化。

Header：

```http
Idempotency-Key: <opaque>
```

接受 16–128 bytes visible ASCII、无空白/控制字符。

Frontend 默认可以使用 UUIDv7 字符串。

同 Key / 同 Semantic Request：

```text
return original resource
```

同 Key / 不同 Semantic Request：

```text
409 IDEMPOTENCY_CONFLICT
```

Semantic Hash 必须按 Domain Canonical Fields 生成，而不是直接 Hash 任意 Raw JSON。

---

# 193. Wallet Row / Ledger Primitive

正式用户 Wallet 必须幂等 Ensure：

```text
RESERVE_API_CREDIT = 0
AVAILABLE_CHIPS    = 0
```

不存在“缺 Row == 0”的业务推理。

全局 Lock Order：

```text
1 existing Domain Authority Row
2 nonterminal Transfer Rows ordered by transfer_id
3 RESERVE_API_CREDIT Wallet
4 AVAILABLE_CHIPS Wallet
5 append Ledger / dependent immutable effects
```

所有双 Wallet 操作统一：

```text
Reserve first
→ Chips second
```

Wallet Mutation 只能通过内部：

```text
ApplyWalletLeg(...)
```

逻辑：

```text
before = balance
after = before + delta
assert after >= 0

next_seq = ledger_seq + 1

UPDATE wallet
INSERT append-only ledger
```

必须同一 PostgreSQL Transaction。

`leg_no` 继续作为 Transaction 内稳定 Leg Identity，Reversal 创建新 Entry 并填 `reverses_entry_id`，不修改旧 Ledger。

---

# 194. Local Exactly-once

单 `chaldea_platform` DB 内：

```text
Unique Business Identity
+
Mutation Idempotency
+
Domain Row Lock
+
Wallet Row Lock
+
Balance / Ledger same transaction
```

形成：

> Exactly-once committed business effect.

Network Request 可以 at-least-once；Duplicate Request 返回原资源。

---

# 195. Asset Snapshot

区分：

## Projection Snapshot

用于：

```text
GET /wallet
Dashboard read
```

可以 DEGRADED。

若某 Authority 不可用：

```text
total_assets = null
completeness = DEGRADED
degraded_reasons = [...]
```

禁止：

```text
unknown Active = 0
```

## Strict Authoritative Snapshot

用于：

```text
Exchange Submit
Relief Eligibility
Negative Adjustment
Poker Buy-in
```

必须组合：

```text
Reserve
Active NewAPI Quota
Available Chips
Poker In Play
Processing Transfers
```

任一 Required Authority：

```text
Unavailable
Too stale
Economically ambiguous
```

则：

```text
ASSET_SNAPSHOT_UNAVAILABLE
→ Fail Closed
```

高风险 Snapshot：

```text
BEGIN PG transaction
lock relevant domain/transfer/wallet/poker facts
derive local facts
perform one bounded source-verified NewAPI Active read
compute total
continue/rollback
```

持锁事务中的 NewAPI Read 不执行多轮指数重试。

Implementation Config：

```text
ECONOMY_STRICT_SNAPSHOT_READ_TIMEOUT
= UNRESOLVED_IMPLEMENTATION_CONFIG
```

---

# 196. Total Assets / Processing

Total：

```text
Reserve
+
Active NewAPI Raw Quota
+
Available Chips
+
Poker In Play
+
Unrepresented Processing
```

Processing 不建立 Wallet Row。

Transfer Leg Effect State：

```text
PLANNED
APPLYING
APPLIED
REVERSING
REVERSED
FAILED_NO_EFFECT
UNKNOWN
```

Processing：

```text
Σ APPLIED Source DEBIT
-
Σ APPLIED Target CREDIT
```

REVERSED Source Leg 不再贡献 Processing。

若：

```text
derived processing < 0
or
bucket ownership cannot be proven
```

则：

```text
ECONOMIC_FACT_AMBIGUOUS
→ NEEDS_REVIEW
→ Incident / Attention
```

---

# 197. Exchange Contract

方向：

```text
API_CREDIT_TO_CHIPS
CHIPS_TO_API_CREDIT
```

固定：

```text
Rate = 1:1
Fee = 0
```

Exchange 只是 Position Conversion：

```text
no issuance
no burn
total assets unchanged
```

Poker In Play 不可直接 Exchange。

API：

```http
POST /api/v1/wallet/exchanges
```

Required：

```http
Idempotency-Key
X-CSRF-Token
```

Body：

```json
{
  "direction": "API_CREDIT_TO_CHIPS",
  "amount": "100"
}
```

Client 不提交 source split / fee / rate / atomic plan。

---

# 198. Exchange Resource Identity

Server 第一次接受：

```text
transfer_id UUIDv7
transaction_id UUIDv7

biz_id =
wallet_exchange:{transfer_id}
```

同事务持久化：

```text
idempotency record
asset_transaction
transfer
transfer_legs
```

无论 Local Exchange 还是 Cross-DB Exchange 都创建统一 Transfer Model。

Local-only Transfer 可以在同一事务直接：

```text
PENDING → CONFIRMED
```

不进入 Worker。

---

# 199. Chips → API

固定：

```text
Available Chips
→ Reserve API Credit
```

单库：

```text
BEGIN

lock Reserve
lock Chips

validate Chips

Chips -X
Ledger

Reserve +X
Ledger

Transfer CONFIRMED
Asset Transaction CONFIRMED

COMMIT
```

失败无 Partial Effect。

---

# 200. API → Chips Local Path

Strict Snapshot：

```text
Reserve >= requested
```

则：

```text
BEGIN
lock Reserve
lock Chips

Reserve -X
Ledger

Chips +X
Ledger

CONFIRMED
COMMIT
```

完全不调用 NewAPI。

---

# 201. API → Chips Cross-DB Plan

如果：

```text
Reserve < requested
```

固定：

```text
reserve_debit = Reserve
active_debit  = requested - Reserve
chips_credit  = requested
```

并要求：

```text
Active >= active_debit
```

Transfer Acceptance 时持久化 Immutable Plan：

```text
direction
requested_units

Reserve Debit Leg
Active Debit Leg
Chips Credit Leg

effect_operation_id
plan_hash
```

`plan_hash` 使用版本化 Canonical Transfer Fields 的 SHA-256。

创建后 Source Split 不允许静默 Re-plan。

若后续 Active 不足：

```text
compensate original Reserve debit
```

不改变原 Plan。

---

# 202. Cross-DB Transfer Lifecycle

涉及 NewAPI Active 的 Exchange：

```text
HTTP
→ Durable Transfer Intent
→ Durable Plan
→ Durable Recovery Job
→ 202 Accepted
```

Browser Connection 不承担 Saga 生命周期。

State：

```text
PENDING
→ SOURCE_DEBITING
→ SOURCE_DEBITED
→ TARGET_CREDITING
→ TARGET_CREDITED
→ CONFIRMED
```

异常：

```text
PENDING → FAILED_NO_EFFECT

Source affected + Target impossible
→ COMPENSATING
→ COMPENSATED

cannot prove next safe action
→ NEEDS_REVIEW
```

---

# 203. API→Chips Worker Sequence

示例：

```text
Reserve 70
Active 80
Requested 100
```

Plan：

```text
Reserve Debit 70
Active Debit 30
Chips Credit 100
```

Local Source TX：

```text
lock Transfer
lock Reserve

if reserve leg not applied:
  Reserve -70
  Ledger
  leg APPLIED

commit
```

NewAPI Effect：

```text
operation_id =
quota:{transfer_id}:active_debit

ApplyRawQuotaDelta(
  operation_id,
  user,
  -30
)
```

成功：

```text
SOURCE_DEBITED
```

Local Target TX：

```text
lock Transfer
lock Chips

if target leg not applied:
  Chips +100
  Ledger
  target leg APPLIED

TARGET_CREDITED
CONFIRMED
```

如果 NewAPI 原子 Debit 返回 INSUFFICIENT：

```text
do not credit Chips

COMPENSATING

lock Reserve
Reserve +70
append reversal ledger
source local leg REVERSED

COMPENSATED
```

不透支 Active。

---

# 204. NewAPI Quota Port

抽象：

```text
ReadRawQuota(user)

ApplyRawQuotaDelta(
  operation_id,
  user,
  delta_raw_quota
)

QueryQuotaOperation(
  operation_id
)
```

Result：

```text
APPLIED
NOT_APPLIED
INSUFFICIENT
ACCOUNT_RESTRICTED
UNKNOWN
DEPENDENCY_UNAVAILABLE
SOURCE_INCOMPATIBLE
```

NewAPI 必须保证：

```text
after_quota >= 0
```

Concrete：

```text
BLOCKED_BY_SV-09 / SV-10 / SV-13
```

---

# 205. Target-local Idempotency / Unknown Result

优先：

```text
SV-10 verified native idempotency
```

若不存在：

```text
Narrow Quota Bridge
+
NewAPI-local Operation Journal
```

Journal 至少：

```text
operation_id PRIMARY KEY
newapi_user_id
delta_raw_quota
before_quota
after_quota
result
created_at
```

Journal 与 Quota Mutation 必须同 NewAPI-local Transaction。

具体 NewAPI Table / Migration Name：

```text
BLOCKED_BY_SV-09 / SV-10
```

Mutation Timeout：

```text
never assume success
never assume failure
```

必须查询：

```text
QueryQuotaOperation(original operation_id)
```

UNKNOWN：

```text
persist uncertainty
query later
or
NEEDS_REVIEW
```

永不生成新 operation_id 盲重试。

---

# 206. Active-quota Movement Serialization

为避免：

```text
API→Chips Active Debit
vs
Reserve→Active Refill
```

同用户同时最多一个非终态 Chaldea Cross-DB Active movement。

DB Partial Unique：

```text
UNIQUE(newapi_user_id)
WHERE
  state NOT IN (
    'CONFIRMED',
    'COMPENSATED',
    'FAILED_NO_EFFECT'
  )
  AND direction IN (
    'API_TO_CHIPS',
    'RESERVE_TO_ACTIVE_REFILL'
  )
```

第二个操作：

```text
409 ASSET_OPERATION_IN_PROGRESS
```

---

# 207. Durable Economy Recovery

Cross-DB Transfer 创建时同一事务创建：

```text
ops.jobs

job_type =
ECONOMY_RECONCILIATION

dedupe_key =
transfer:{transfer_id}
```

Worker：

```text
FOR UPDATE SKIP LOCKED job
→ lock transfer
→ inspect durable facts
→ execute next legal transition
```

Recovery 只根据：

```text
Transfer
Leg
Wallet Ledger
NewAPI Operation
Target Ledger
Compensation Ledger
```

不根据 Process Memory 推测。

Implementation Config：

```text
ECONOMY_RECONCILE_BACKOFF_INITIAL
ECONOMY_RECONCILE_BACKOFF_MAX
ECONOMY_RECONCILE_JITTER_RATIO

NEWAPI_QUOTA_MUTATION_TIMEOUT
NEWAPI_QUOTA_QUERY_TIMEOUT
ECONOMY_STRICT_SNAPSHOT_READ_TIMEOUT
```

均继续：

```text
UNRESOLVED_IMPLEMENTATION_CONFIG
```

Dependency Outage 不因为 Attempt Count 自动触发 Compensation。

---

# 208. Ambiguous Transfer Guard / No Cancel

存在：

```text
NEEDS_REVIEW
or
ECONOMIC_FACT_AMBIGUOUS
```

时，只阻止触碰相同 Asset Position 的新：

```text
Exchange
Refill
Negative API Adjustment
```

不自动冻结整个账号。

Accepted Exchange：

```text
no ordinary user cancel
```

用户可离开页面。

Server 继续：

```text
finish
compensate
needs review
```

Timeout/Refresh：

```text
query original transfer
```

---

# 209. Exchange HTTP Result / User State

Local CONFIRMED：

```http
201 Created
```

Cross-DB Durable Accepted：

```http
202 Accepted
```

都返回：

```text
transfer_id
transaction_id
current user-visible state
```

Internal → UI：

```text
PENDING/SOURCE_DEBITING/SOURCE_DEBITED/TARGET_CREDITING/TARGET_CREDITED
→ PROCESSING

CONFIRMED
→ COMPLETED

COMPENSATING
→ RETURNING_ASSETS

COMPENSATED
→ RETURNED

FAILED_NO_EFFECT
→ NOT_EXECUTED

NEEDS_REVIEW
→ NEEDS_ATTENTION
```

---

# 210. Wallet BFF

固定：

```http
GET /api/v1/wallet

GET /api/v1/wallet/transactions
GET /api/v1/wallet/transactions/{transaction_id}

POST /api/v1/wallet/exchanges
GET  /api/v1/wallet/exchanges/{transfer_id}
```

Wallet Projection 返回：

```text
API Credit:
  total
  reserve
  active
  completeness

Available Chips
Poker In Play
Processing Assets
Total Assets

server_time
source_observed_at
completeness
degraded_reasons
```

Transaction List 使用：

```text
created_at DESC
+
transaction_id DESC
```

稳定 Keyset Cursor。

不提供 Manual Active Quota Top-up Endpoint。

---

# 211. Active Liquidity Controller

正式策略：

> **Bounded Active Buffer + Reactive Refill**

Implementation Config：

```text
ACTIVE_QUOTA_LOW_WATERMARK
ACTIVE_QUOTA_TARGET_WATERMARK
ACTIVE_QUOTA_MAX_ACTIVE_BUFFER
```

全部：

```text
UNRESOLVED_IMPLEMENTATION_CONFIG
```

Production 启用前要求：

```text
0 <= LOW
LOW < TARGET
TARGET <= MAX
```

且：

```text
SV-11 verified hook
```

否则：

```text
LIQUIDITY_CONTROLLER_PRODUCTION_READY = false
```

Reactive Formula：

```text
desired_active =
max(
  TARGET_WATERMARK,
  required_active_units
)

desired_active =
min(
  desired_active,
  MAX_ACTIVE_BUFFER
)

refill_units =
min(
  Reserve,
  max(0, desired_active - observed_active)
)
```

`required_active_units > MAX` 时：

```text
ACTIVE_LIQUIDITY_LIMIT
```

不得偷偷扩大 Max。

禁止高频轮询猜需求；具体 NewAPI Admission / Insufficient Hook 继续 `BLOCKED_BY_SV-11`。

---

# 212. Reserve → Active Refill

Refill 创建标准：

```text
direction = RESERVE_TO_ACTIVE_REFILL
```

Leg：

```text
CHALDEA / RESERVE / DEBIT / X
NEWAPI  / ACTIVE  / CREDIT / X
```

它是：

```text
Position Movement
not Reward
not Issuance
not Consumption
```

使用同一：

```text
Transfer State Machine
Effect Operation ID
Recovery Job
Compensation
```

Total Assets 不变。

Cutover：

```text
Active=0 verified
→ Migration Grant to Reserve
→ READY_TO_OPEN
```

之后才允许 Liquidity Controller/Prime；Prime 不改变迁移 Reset Evidence。

---

# 213. Poker / Direct Play Economy Hooks

Poker 继续使用：

```text
economy.poker_buy_in_apply(...)
economy.poker_top_up_apply(...)
economy.poker_cash_out_apply(...)
```

同库：

```text
Wallet + Ledger + Poker Funding State
→ same transaction
```

Poker Amount：

```text
amount_units % 500000 = 0
```

完整 Poker 参数在 IS-07。

Direct Play 统一 Economy Primitive：

```text
AcceptGameWager(...)
SettleGameRound(...)
RefundGameRound(...)
```

要求：

```text
Round Acceptance + Wager Debit
→ same transaction

Settlement/Refund
→ stable round/biz identity
→ exactly-once
```

完整 Game State/Math 在 IS-06。

---

# 214. Supply Event

真正新增用户总资产：

```text
Reward
Initial Grant
positive Admin Adjustment
Game Issuance
```

记录：

```text
economy.asset_supply_events
event_type = ISSUE
```

真正销毁：

```text
Game Burn
negative Admin Adjustment
```

记录：

```text
event_type = BURN
```

以下不产生 Supply Event：

```text
Exchange
Reserve↔Active
Poker Buy-in
Top-up
Cash Out
```

因为只是 Position Movement。

---

# 215. Admin Adjustment Primitive

完整 Operations Contract 留 IS-09。

Economy Primitive：

```text
positive API
→ Reserve

positive/negative Chips
→ Available Chips only

negative Unified API
→ Reserve-first
→ Active-shortfall if needed
```

普通 Adjustment 永不直接修改 Poker In Play。

跨 Active 的负向 Adjustment 复用同一 Cross-DB Transfer Engine。

---

# 216. Reward Authority Boundary

Reward Domain 决定：

```text
Policy
Eligibility
Period
Claim Identity
Claim State
```

Economy Domain 决定：

```text
Wallet Effect
Ledger
Asset Transaction
Supply Event
```

Reward 不能直接 `UPDATE wallet_balances`。

Claim CONFIRMED 必须与：

```text
Economy Transaction
Wallet Leg
Ledger
Supply Event
Issuance Record
```

同一个 Chaldea PostgreSQL Transaction 提交。

---

# 217. Reward Policy Seed v1

`000019__reward_policy_seed_v1`：

## Registration Initial Grant

```text
kind = INITIAL_GRANT_REGISTRATION
status = ACTIVE
asset_type = API_CREDIT
amount_units = 500,000,000
```

## Migration Initial Grant

```text
kind = INITIAL_GRANT_MIGRATION
status = ACTIVE
asset_type = API_CREDIT
amount_units = 500,000,000
```

## Daily

```text
kind = DAILY
status = ACTIVE
asset_type = API_CREDIT
amount_units = 250,000,000
business_timezone = Asia/Shanghai
window_mode = NATURAL_DAY
```

## Hourly

```text
kind = HOURLY
status = CONFIG_INCOMPLETE
amount_units = 50,000,000

asset_type = NULL
window_mode = NULL
accumulation_mode = NULL
daily_limit_mode = UNRESOLVED
```

## Relief

```text
kind = RELIEF
status = CONFIG_INCOMPLETE
amount_units = 150,000,000
eligibility_threshold_units = 5,000,000
cooldown_seconds = 14,400

asset_type = NULL
accumulation_mode = NULL
active_poker_policy = NULL
```

OPEN 字段不得获得假默认值。

---

# 218. Reward Policy Validation / Hash

Policy：

```text
policy_hash =
SHA-256(canonical versioned policy fields)
```

Active Version Immutable。

V1 Validator 强制：

```text
Registration:
1000 API Credit

Migration:
1000 API Credit

Daily:
500 API Credit
Asia/Shanghai
Natural Day

Hourly:
quantity 100
+
asset/window/accumulation/daily limit
must all resolve before ACTIVE

Relief:
quantity 300
threshold <10
cooldown 4h
+
asset/accumulation/active-poker
must all resolve before ACTIVE
```

Policy Version 新增只影响 Future Claim，不重算历史。

---

# 219. Reward Claim BFF / Idempotency

```http
GET  /api/v1/rewards
POST /api/v1/rewards/claims
GET  /api/v1/rewards/claims/{claim_id}
```

Claim Body：

```json
{
  "reward_kind": "DAILY"
}
```

Client 永不提交：

```text
amount
asset_type
policy_version
eligibility
period_key
claim_time
```

Server 读取 Active Validated Policy。

Claim Origin 只用于 Provenance：

```text
USER_DASHBOARD
USER_REWARDS_CENTER
```

不参与 Eligibility / Period / Biz Identity。

---

# 220. Reward Exactly-once Issuance

通用：

```text
BEGIN

lock Claim/Cursor as required
lock target Wallet

validate business uniqueness
validate Policy
validate Eligibility / Period

ensure Economy Transaction

ApplyWalletLeg(target credit)

insert Supply ISSUE
insert reward issuance record

Claim → CONFIRMED

COMMIT
```

HTTP 可以重试，但 Durable Business Effect Exactly-once。

已 CONFIRMED Claim 不得改写为失败；错误通过 Adjustment/Reversal + Incident + Audit 修正。

---

# 221. Registration Initial Grant

Biz：

```text
initial_grant:registration:{newapi_user_id}
```

固定：

```text
1000 API Credit
→ Reserve
```

额外领域 Unique：

```text
one INITIAL_GRANT_REGISTRATION per user
```

Account 创建后：

```text
ensure account_ref
ensure PENDING Grant Claim
ensure REWARD_RECOVERY job
issue local Economy effect
→ CONFIRMED
```

Existing Bound Account 永不再次创建 Registration Initial Grant。

Grant 暂未完成：

```text
keep account
keep profile
resume original claim
```

---

# 222. Migration Initial Grant

Biz：

```text
initial_grant:migration:
{migration_batch_id}:
{newapi_user_id}
```

前置：

```text
cutover_user.state = RESET_VERIFIED
```

固定：

```text
1000 API Credit
→ Reserve
```

额外领域 Unique：

```text
one INITIAL_GRANT_MIGRATION per user
```

即使错误进入第二 Migration Batch，也不得重复发当前 Launch Grant。

Grant 未 CONFIRMED：

```text
cutover user != VERIFIED
batch != READY_TO_OPEN
```

Migration Notice Ack 不触发 Grant。

---

# 223. Initial Grant Recovery

Initial Grant 可以先 Durable：

```text
PENDING
```

并同事务创建：

```text
ops.jobs
job_type = REWARD_RECOVERY
dedupe_key = reward:{claim_id}
```

Worker：

```text
read Claim
read Economy Transaction
read Ledger
read Issuance
derive durable fact
```

如果 Economy Effect 已 Commit：

```text
converge Claim → CONFIRMED
```

不重复发放。

Maintenance / transient failure 只延迟执行，不取消 Entitlement。

---

# 224. Daily Reward

固定：

```text
Timezone = Asia/Shanghai
Natural Day
Amount = 500 API Credit
Target = Reserve
No make-up
No streak
No seven-day multiplier
```

Server 生成：

```text
business_date = YYYY-MM-DD
period_key = YYYY-MM-DD

biz_id =
reward:daily:{user}:{YYYY-MM-DD}
```

Client 不能提交 Claim Date。

事务：

```text
DB now
→ Shanghai business date

BEGIN
idempotency
insert/resolve Claim by unique period
lock Reserve

Economy TX
Reserve +250,000,000
Ledger
Supply ISSUE
daily_checkin
issuance_record
Claim CONFIRMED

COMMIT
```

100 并发同一天：

```text
one Claim
one +500
```

Policy Version 不进入 Daily Unique Key；同日 Policy 切换不产生第二机会。

---

# 225. Hourly Technical Model

已冻结：

```text
quantity = 100
```

仍 OPEN：

```text
asset_type
window_mode
accumulation
daily_limit
```

Production：

```text
HOURLY = CONFIG_INCOMPLETE
no active Hourly policy
no production successful Hourly claim
```

代码同时支持：

## NATURAL_HOUR

```text
Asia/Shanghai hour_key
YYYY-MM-DDTHH

biz =
reward:hourly:{user}:{hour_key}

unique(user, HOURLY, hour_key)
```

## ROLLING_60_MINUTES

使用 locked：

```text
rewards.claim_cursors
```

只有 CONFIRMED 后：

```text
last_successful_claim_at = confirmed_at
next_claim_at = confirmed_at + 60m
```

`rewards.entitlements` 当前：

```text
NO PRODUCTION PRODUCER
NO PRODUCTION CONSUMER
```

`daily_limit_mode = UNRESOLVED` 永不解释为 Unlimited。

Test-only Policy 可以验证 Natural/Rolling，但不能进入 Production Seed。

---

# 226. Relief Technical Model

已经冻结：

```text
quantity = 300
threshold = Total Assets < 10
threshold_units = 5,000,000
cooldown = 4h
```

仍 OPEN：

```text
asset_type
accumulation
active_poker_policy
```

因此：

```text
RELIEF = CONFIG_INCOMPLETE
```

不 Production Active。

未来完整 Policy 启用后，Relief Claim Lock Order：

```text
1 Relief Cursor
2 Active Poker Session
3 Relevant nonterminal Transfers ordered by ID
4 Reserve Wallet
5 Chips Wallet
6 direct NewAPI Active read
```

事务内：

```text
derive Reserve/Active/Chips/Poker/Processing
evaluate Active Poker Policy
evaluate Total Assets
evaluate Cooldown
save Eligibility Snapshot
issue configured asset
Ledger/Economy/Supply
Claim CONFIRMED
update cursor
```

NewAPI Active 不能用缓存替代；读取不可用：

```text
ELIGIBILITY_TEMPORARILY_UNAVAILABLE
→ ROLLBACK
```

Snapshot 保存当时全部资产拆分、Total、Cursor、Active Poker、Source Freshness、Policy、Result。

---

# 227. Relief Cooldown / Concurrency

仅：

```text
Claim CONFIRMED
```

推进：

```text
last_successful_claim_at = confirmed_at

next_claim_at =
confirmed_at + 4h
```

以下不推进：

```text
ineligible
maintenance
validation failure
dependency failure
DB rollback
issuance rollback
```

例：

```text
09:37:00 confirmed
13:36:59 denied
13:37:00 cooldown satisfied
```

到点后仍需重新满足：

```text
Total Assets < 10
```

并发：

```text
cursor lock
+
claim uniqueness
+
wallet atomic issuance
```

保证最多一个成功 Claim。

Test Fixture 可分别验证 Active Poker ALLOW/DENY，但 Production 仍 UNRESOLVED。

---

# 228. Reward Maintenance / Recovery

Backend Maintenance Scope：

```text
CHALDEA_USER_WRITES
REWARDS
```

阻止：

```text
new ordinary Daily/Hourly/Relief Claim
```

继续：

```text
PENDING
ISSUING
RECOVERING
Registration Initial Grant
Migration Initial Grant
```

`reward_policies.operational_state` 技术上可表达：

```text
AVAILABLE
CLAIMS_PAUSED
```

但当前不开放新的 Product-level `SetClaimsPaused` 行为/API，因为 Reward Product Maintenance Rule 仍 OPEN。

Reward Recovery：

```text
job_type = REWARD_RECOVERY
dedupe_key = reward:{claim_id}
```

永远复用：

```text
same claim_id
same biz_id
same policy_version
same asset
same amount
```

如果 final effect 已存在：

```text
converge CONFIRMED
```

无法证明：

```text
NEEDS_REVIEW
```

管理员不得把 Failed/Rejected Claim 直接改 Success。

人工补发：

```text
Economy Adjustment
+
original claim_id
+
incident/reference
```

---

# 229. Reward / Wallet API Projection

`GET /api/v1/rewards` 返回：

```text
server_time
business_timezone

daily:
  availability
  policy_version
  asset
  amount
  business_date
  period boundaries
  claim state

hourly:
  availability
  policy_version nullable
  asset nullable
  amount
  window_mode nullable
  claim state
  next_claim_at nullable

relief:
  availability
  policy_version nullable
  asset nullable
  amount
  threshold
  claim state
  next_claim_at nullable
```

当前 Production：

```text
Daily  = ACTIVE
Hourly = CONFIG_INCOMPLETE
Relief = CONFIG_INCOMPLETE
```

Countdown 只是 Display；Submit 时 Server 重新验证。

Claim Detail Owner-safe：

```text
claim_id
reward_kind
claim_origin
policy_version
asset
amount
status
claim_time
confirmed_at
biz_id
balance_after nullable
next_claim_at nullable
```

Reward History 必须区分：

```text
INITIAL_GRANT_REGISTRATION
INITIAL_GRANT_MIGRATION
DAILY
HOURLY
RELIEF
```

两个 +1000 不得合并成无来源单一 Reward。

---

# 230. Economy / Reward Metrics

Economy 至少：

```text
transfers_by_state
oldest_pending_age
compensation_failure_count
needs_review_count
wallet_ledger_mismatch
duplicate_biz_conflict
active_quota_bridge_error
external_event_lag
processing_assets_units
poker_funding_atomic_failure
```

Critical Facts：

```text
negative wallet
ledger chain mismatch
duplicate economic effect
compensation impossible
```

Reward：

```text
reward_claims_total{kind,status}
reward_issuance_units{kind,asset}
reward_claim_latency
reward_duplicate_conflict
reward_recovery_count
reward_needs_review_count

daily_claim_success_count
hourly_claim_success_count
relief_claim_success_count

relief_ineligible_asset_count
relief_cooldown_rejection_count
relief_snapshot_unavailable_count

initial_grant_pending_count
initial_grant_recovery_age
migration_grant_unconfirmed_count
```

Alert Thresholds：

```text
UNRESOLVED_IMPLEMENTATION_CONFIG / Product OPEN
```

---

# 231. Wallet Ledger Reconciliation

定期校验：

```text
wallet balance
vs
ordered append-only ledger chain
```

每 Wallet：

```text
entry[n].before
=
entry[n-1].after

entry.after
=
entry.before + delta

last entry after
=
wallet balance
```

异常：

```text
Incident
+
Needs Attention
```

不得自动修改余额/账本掩盖错误。

---

# 232. IS-05 Test Gate

## Precision

```text
0.000002 → 1 unit
0.000001 → reject
no float path
MaxInt64 overflow reject
```

## Ledger / Local Economy

```text
100 same Biz ID → one effect
100 concurrent debits → no negative
API↔Chips reverse concurrency → no deadlock / overspend
ledger insert failure → balance rollback
```

## Cross-DB

```text
response loss → same transfer
source committed → crash → resume
NewAPI applied → response lost → query original operation
target committed → response lost → no duplicate credit
compensation committed → response lost → no duplicate compensation
Redis loss → no financial truth lost
```

## Active Concurrency

```text
API consumption vs API→Chips active debit
→ NewAPI quota never negative

Refill vs API→Chips
→ one Active movement lane
→ total API conserved
```

## Initial Grants

```text
duplicate registration callback/tabs/restart
→ one claim/economy/wallet/ledger

migration batch rerun / accidental second batch
→ one Migration Initial Grant per user
```

## Daily

```text
100 concurrent same Shanghai date
→ one +500

23:59:59 / 00:00:00 boundary
```

## Hourly

```text
test-only Natural mode
test-only Rolling mode
Production remains CONFIG_INCOMPLETE
```

## Relief

使用 Test-only 完整 Policy：

```text
9.999998 → eligible
10 → ineligible

50 concurrent
→ one success

cooldown:
T+3:59:59 reject
T+4h re-evaluate

ALLOW/DENY active-poker fixture both work
Production remains UNRESOLVED
```

---

# 233. Codex IS-05 Implementation Order

```text
01 amount parser/formatter

02 generic mutation idempotency
03 wallet repository
04 wallet mutation primitive
05 ledger repository
06 asset transaction repository

07 wallet projection
08 strict asset snapshot

09 transfer repository/legs
10 local exchange
11 quota adapter contract
12 cross-db state machine
13 reconciliation job

14 liquidity formula/controller
15 refill transfer

16 reward policy repository/validator
17 reward seed migration

18 registration grant
19 migration grant
20 daily

21 hourly natural
22 hourly rolling
23 disabled entitlement path

24 relief snapshot/cursor/claim
25 reward recovery

26 wallet/reward BFF
27 ledger reconciliation
28 metrics
29 concurrency/crash/property tests
```

NewAPI Source 未提供时，只停止 Concrete NewAPI Quota Adapter / Bridge；其余 Chaldea-owned 实现可完成。

---

# 234. IS-05 Prohibited Implementation

禁止：

```text
guess NewAPI quota table/endpoint/journal
direct Platform Runtime UPDATE NewAPI DB

enable liquidity without SV-11
invent LOW/TARGET/MAX

float money
silent rounding

client-submitted Reward amount/asset

enable Hourly Production
choose Hourly asset/window/accumulation/daily limit

enable Relief Production
choose Relief asset/accumulation/active-poker policy

invent Reward product temporary-disable behavior

change confirmed Reward to failed
manual patch Wallet final balance
manual force Transfer CONFIRMED

new operation ID retry after unknown external result

cancel accepted Transfer because user leaves page
```

---

# 235. IS-05 Acceptance Criteria

```text
AC-05-01  all asset math integer atomic units
AC-05-02  exact decimal parser, no float
AC-05-03  Wallet/Ledger same transaction
AC-05-04  append-only monotonic ledger
AC-05-05  durable Idempotency-Key conflict semantics
AC-05-06  Chips→API local atomic
AC-05-07  Reserve-only API→Chips local atomic
AC-05-08  Active-shortfall API→Chips durable Saga
AC-05-09  immutable transfer plan
AC-05-10  Processing derived, never standalone balance
AC-05-11  stable external operation identity
AC-05-12  query-first unknown result
AC-05-13  no concrete NewAPI mutation before SV-09/10
AC-05-14  one Chaldea Active-quota movement/user
AC-05-15  recovery via durable jobs/facts
AC-05-16  maintenance preserves accepted transfer
AC-05-17  display snapshot never maps unknown to zero
AC-05-18  strict snapshot fail closed
AC-05-19  exact Total Assets formula
AC-05-20  exchange 1:1 zero fee
AC-05-21  exchange no issuance/burn
AC-05-22  no user cancel after accepted exchange
AC-05-23  liquidity blocked until SV-11 + watermarks
AC-05-24  Reserve→Active uses normal recoverable transfer
AC-05-25  Poker funding same-DB atomic
AC-05-26  Reward uses Economy primitive
AC-05-27  one Registration Initial Grant/user
AC-05-28  one Migration Initial Grant/user across batches
AC-05-29  Initial Grants = 1000 API Credit→Reserve
AC-05-30  Daily = 500 API Credit
AC-05-31  Daily = Asia/Shanghai calendar day
AC-05-32  no Daily make-up
AC-05-33  Policy version does not reset period/cursor
AC-05-34  Hourly Production CONFIG_INCOMPLETE
AC-05-35  both Hourly technical modes testable
AC-05-36  Hourly accumulation disabled
AC-05-37  Hourly NULL limit != unlimited
AC-05-38  Relief Production CONFIG_INCOMPLETE
AC-05-39  Relief threshold <5,000,000 units
AC-05-40  Relief full fresh snapshot
AC-05-41  Relief cooldown only after CONFIRMED
AC-05-42  Relief OPEN not guessed
AC-05-43  Dashboard/Rewards Center cannot double claim
AC-05-44  Maintenance preserves accepted Reward recovery
AC-05-45  Confirmed Claim immutable
AC-05-46  manual compensation via Economy Adjustment
AC-05-47  Reward retry returns original Claim
AC-05-48  amount/ID strings at BFF
AC-05-49  ledger reconciliation detects corruption
AC-05-50  Redis loss preserves financial truth
```

---

# 236. IS-05 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| IS-FRZ-161 | Economy Authority 永久区分 Reserve / Active NewAPI Quota / Available Chips / Poker In Play / Processing；Active 不建立第二 Chaldea Balance Authority。 | FROZEN |
| IS-FRZ-162 | `QuotaPerCredit=500,000`，1 raw quota = 1 atomic unit；所有资产使用 BIGINT/int64，严禁浮点账务。 | FROZEN |
| IS-FRZ-163 | Amount Parser 固定使用最多 6 位 Decimal String→整数算法；0.000002=1 unit，无法精确表达时拒绝且不舍入。 | FROZEN |
| IS-FRZ-164 | 新增跨 Domain Durable `platform_meta.mutation_idempotency_records`；只存 Key Hash + Semantic Request Hash + Original Resource，不存 Raw Key。 | FROZEN |
| IS-FRZ-165 | Idempotency-Key 同 Key/同语义返回原资源，同 Key/不同语义返回 409；Fingerprint 使用各 Mutation 的 versioned semantic fields。 | FROZEN |
| IS-FRZ-166 | IS-05 只通过新增 `000018` / `000019` Forward Migration 扩展 IS-03，不修改既有 000000～000017。 | FROZEN |
| IS-FRZ-167 | `000018` 补齐 Transfer Recovery、Leg Effect Link、Ledger Reversal 与 Reward Policy/Initial Grant Unique Invariant。 | FROZEN |
| IS-FRZ-168 | 全局 Economy Lock Order 固定 Domain/Transfer Row → Reserve Wallet → Chips Wallet → Append-only Effect，所有双 Wallet 操作统一 Reserve-first Lock。 | FROZEN |
| IS-FRZ-169 | 所有 Chaldea Wallet Mutation 必须经单一 Wallet Leg Primitive，在同事务更新 Materialized Balance、ledger_seq 与 append-only Ledger。 | FROZEN |
| IS-FRZ-170 | `leg_no` 作为 IS-03 既有稳定 Transaction Leg Identity；Reversal 使用新 Ledger Entry + `reverses_entry_id`，不改旧 Ledger。 | FROZEN |
| IS-FRZ-171 | Wallet Read Projection 与 Strict Authoritative Snapshot 分离；普通读可 DEGRADED，但 Total 缺 Authority 时为 unavailable/null，而非把未知当 0。 | FROZEN |
| IS-FRZ-172 | Strict Asset Snapshot 在锁定相关 Chaldea/Poker/Transfer Rows 后直接读取 source-verified NewAPI Active；任一 Authority 不可确认时 Fail Closed。 | FROZEN |
| IS-FRZ-173 | Total Assets 精确等于 Reserve + Active + Available Chips + Poker In Play + Unrepresented Processing。 | FROZEN |
| IS-FRZ-174 | Exchange 两方向固定 1:1、零手续费、无发行/销毁；Poker In Play 不可直接兑换。 | FROZEN |
| IS-FRZ-175 | 每笔 Exchange 无论 Local/Cross-DB 都创建 asset_transaction + transfer + transfer_legs，统一用户交易模型。 | FROZEN |
| IS-FRZ-176 | Chips→API 永远在单一 Chaldea DB Transaction 中完成，并固定 Credit Reserve。 | FROZEN |
| IS-FRZ-177 | API→Chips Reserve 足够时完全 Local Atomic；不足时按 Reserve-first / Active-shortfall 创建 Immutable Cross-DB Plan。 | FROZEN |
| IS-FRZ-178 | Cross-DB Exchange HTTP 只承诺 Durable Intent/Plan/Recovery Job 后返回 202，不让 Browser Connection 成为 Saga 生命周期。 | FROZEN |
| IS-FRZ-179 | Cross-DB Transfer 继续使用 TD 已冻结 State Machine；所有 State Transition 使用 Typed Command + Expected State/Version。 | FROZEN |
| IS-FRZ-180 | Processing Assets 由 APPLIED Source Debits − APPLIED Target Credits 推导；负数/无法证明归属直接进入经济事实歧义处理。 | FROZEN |
| IS-FRZ-181 | NewAPI Quota Adapter 固定 Read/ApplyDelta/QueryOperation 三能力，Raw Quota Delta 与 Chaldea Atomic Unit 1:1。 | FROZEN |
| IS-FRZ-182 | Quota Mutation 必须 Target-local Idempotent；Native 不支持时才允许 Source-verified Narrow Bridge + NewAPI-local Journal，同事务更新 Journal 与 Quota。 | FROZEN |
| IS-FRZ-183 | External Mutation Unknown Result 永远查询原 Operation ID；不生成新 ID、不中途猜 Success/Failure。 | FROZEN |
| IS-FRZ-184 | 同用户 Chaldea 发起的 Active-quota Cross-DB Movement 同时最多一个，API→Chips 与 Reserve→Active Refill 互相序列化。 | FROZEN |
| IS-FRZ-185 | 每笔 Cross-DB Transfer 在同事务创建 `ECONOMY_RECONCILIATION` Durable Job，Recovery 使用 `ops.jobs + SKIP LOCKED`。 | FROZEN |
| IS-FRZ-186 | Economy Reconciliation 只根据 Durable Transfer/Leg/Ledger/NewAPI Effect 决策；不得依据进程内存推测恢复点。 | FROZEN |
| IS-FRZ-187 | Reconciliation Backoff/Timeout 精确数值保留 Implementation Config；Dependency Outage 本身不凭 Attempt Count触发错误 Compensation。 | FROZEN |
| IS-FRZ-188 | 存在 NEEDS_REVIEW / ambiguous Transfer 时，仅阻止会触碰相同 Asset Position 的新操作，不自动冻结整个账号。 | FROZEN |
| IS-FRZ-189 | Accepted Exchange 没有普通 Cancel；Timeout/Refresh 必须查询原 Transfer，Server 继续 Finish/Compensate/Review。 | FROZEN |
| IS-FRZ-190 | Local Exchange 返回 201；Cross-DB Durable Accepted 返回 202 + same transfer_id；用户状态映射隐藏内部 Saga State。 | FROZEN |
| IS-FRZ-191 | Liquidity Controller 使用 Bounded Active Buffer + Reactive Refill，但 LOW/TARGET/MAX 未配置及 SV-11 未验证前不可 Production Ready。 | FROZEN |
| IS-FRZ-192 | Refill 公式固定按 TARGET/required/max/reserve 计算，绝不越过 MAX_ACTIVE_BUFFER；Reactive Hook 具体接入继续 SV-11 Blocked。 | FROZEN |
| IS-FRZ-193 | Reserve→Active 使用正常 Cross-DB Transfer/Saga/Operation ID/Compensation，属于 Position Movement 而非 Issue/Reward/Consumption。 | FROZEN |
| IS-FRZ-194 | Liquidity 只在 Cutover Active=0 验证完成后允许；任何 Prime 不修改迁移 Reset Evidence。 | FROZEN |
| IS-FRZ-195 | Poker Buy-in/Top-up/Cash Out 继续使用 IS-03 Narrow SECURITY DEFINER Funding Gateway，整 Chip Constraint=500,000 Units。 | FROZEN |
| IS-FRZ-196 | Direct Play Economy 统一提供 AcceptWager/Settle/Refund Primitive；具体 Round State/Math 留 IS-06。 | FROZEN |
| IS-FRZ-197 | Reward/Initial Grant/Game Win/Admin Positive Adjustment 等真正新增资产记录 ISSUE；Game Loss/Admin Negative Adjustment记录 BURN；Exchange/Refill/Poker Funding不记录 Supply Change。 | FROZEN |
| IS-FRZ-198 | Admin Adjustment 只定义 Economy Primitive；完整 Critical Operations Contract 留 IS-09，普通 Adjustment 永不直接编辑 Poker In Play。 | FROZEN |
| IS-FRZ-199 | Reward Domain 只决定 Policy/Eligibility/Claim，真正 Wallet/Ledger Effect 必须通过 Economy Primitive。 | FROZEN |
| IS-FRZ-200 | `000019` 正式 Seed Registration 1000、Migration 1000、Daily 500 为 ACTIVE API Credit Policy；Hourly 100 与 Relief 300 仅 CONFIG_INCOMPLETE。 | FROZEN |
| IS-FRZ-201 | Reward Policy 使用 deterministic policy_hash、单 Reward Kind 单 ACTIVE Version；V1 Validator 强制当前 Product-locked 数值。 | FROZEN |
| IS-FRZ-202 | Reward Claim BFF Body 只接受 reward_kind；Amount/Asset/Policy/Eligibility/Period 全由 Server 决定，UI Origin 仅是非权威 Provenance。 | FROZEN |
| IS-FRZ-203 | Reward CONFIRMED 必须与 Economy Transaction、Wallet Leg、Ledger、Supply Event、Issuance Record 在同一 Chaldea DB Transaction 提交。 | FROZEN |
| IS-FRZ-204 | Registration Initial Grant 固定 `initial_grant:registration:{user}` + 每用户领域 Unique，只能发放一次 1000 API Credit→Reserve。 | FROZEN |
| IS-FRZ-205 | Migration Initial Grant 使用 Batch-aware Biz ID，但额外每用户领域 Unique，跨错误第二 Batch 也不得再次发 1000；仅 RESET_VERIFIED 后可发。 | FROZEN |
| IS-FRZ-206 | Initial Grant 可以先 Durable PENDING 并创建 `REWARD_RECOVERY` Job；账号/Profile 不因 Grant 暂未完成被删除或重建。 | FROZEN |
| IS-FRZ-207 | Daily 固定 Asia/Shanghai Natural Day、Server-generated YYYY-MM-DD Biz Identity、500 API Credit→Reserve、无补签；并发依赖 DB Unique。 | FROZEN |
| IS-FRZ-208 | Policy Version 不进入 Daily/Hourly Period Unique 或 Relief Cursor Identity，Policy 切换默认不重置当前资格窗口。 | FROZEN |
| IS-FRZ-209 | Hourly 技术实现同时支持 NATURAL_HOUR 与 ROLLING_60_MINUTES，但当前 Production Policy 必须 CONFIG_INCOMPLETE；测试 Fixture 不构成产品决定。 | FROZEN |
| IS-FRZ-210 | Hourly Entitlement Accumulation 无 Production Producer/Consumer；daily_limit=UNRESOLVED 永不解释为 Unlimited。 | FROZEN |
| IS-FRZ-211 | Relief 当前 Production Policy 必须 CONFIG_INCOMPLETE；未来启用后严格使用 `<5,000,000 units` + Fresh Total Assets。 | FROZEN |
| IS-FRZ-212 | Relief 采用 Cursor→Poker→Transfer→Wallet Lock Order，事务内直接读取 NewAPI Active 并保存 Eligibility Snapshot；未知 Authority 一律 Fail Closed。 | FROZEN |
| IS-FRZ-213 | Relief 只有 CONFIRMED Asset Issuance 才推进 `next_claim_at=confirmed_at+4h`；失败、维护、回滚、无资格不推进。 | FROZEN |
| IS-FRZ-214 | Relief `asset_type / accumulation / active_poker_policy` 继续 UNRESOLVED；代码/test 可表达不同模式，但 Production 不选默认。 | FROZEN |
| IS-FRZ-215 | REWARDS/CHALDEA_USER_WRITES Maintenance 只阻止新普通 Claim；已有 Claim 与 Initial Grant Entitlement 必须继续原 State Machine Recovery。 | FROZEN |
| IS-FRZ-216 | IS-05 Production Gate 必须通过 Integer Precision、Ledger Chain、Exchange Conservation、Cross-DB Crash/Idempotency、Redis Loss、Initial/Daily Exactly-once、Hourly/Relief OPEN Protection 与 Relief Concurrency Tests。 | FROZEN |

---

# 237. Open / Blocked Register after IS-05

```text
NewAPI:
SV-01 ～ SV-16
= BLOCKED_BY_NEWAPI_SOURCE_VERIFY

Quota:
SV-09 / SV-10 / SV-11 / SV-13
= required before concrete Active implementation

Database:
@NEWAPI_USER_ID_PG_TYPE
= BLOCKED_BY_SV-05

@NEWAPI_KEY_ID_PG_TYPE
= BLOCKED_BY_SV-05 / SV-06

Liquidity:
ACTIVE_QUOTA_LOW_WATERMARK
ACTIVE_QUOTA_TARGET_WATERMARK
ACTIVE_QUOTA_MAX_ACTIVE_BUFFER
= UNRESOLVED_IMPLEMENTATION_CONFIG

Economy Retry / Timeout:
ECONOMY_RECONCILE_BACKOFF_INITIAL
ECONOMY_RECONCILE_BACKOFF_MAX
ECONOMY_RECONCILE_JITTER_RATIO
NEWAPI_QUOTA_MUTATION_TIMEOUT
NEWAPI_QUOTA_QUERY_TIMEOUT
ECONOMY_STRICT_SNAPSHOT_READ_TIMEOUT
= UNRESOLVED_IMPLEMENTATION_CONFIG

Hourly:
asset_type
window_mode
accumulation
daily_limit
= OPEN

Relief:
asset_type
accumulation
active_poker_policy
= OPEN

Reward Product Maintenance
= OPEN

Future Reward Amount Change Product Policy
= OPEN

Reward Operational Alert Threshold
= OPEN

Poker Product Gap 01～05
= OPEN

Public Record Selection Policy
= UNRESOLVED

DEPLOYMENT-VERIFY-01
= PENDING
```

没有 Product OPEN 或 Source Fact 被测试 Fixture / Default 偷偷替代。

---

# 238. Change Log — WORKING v0.5

## Added

- 用户正式确认 `IS-05 — Economy / Rewards Implementation Specification`；
- 冻结 `IS-FRZ-161 ～ IS-FRZ-216`；
- 冻结 Atomic-unit Decimal Parser / Formatter；
- 冻结跨 Domain Durable Mutation Idempotency；
- 冻结统一 Wallet Mutation Primitive / Lock Order / Ledger Reversal；
- 冻结 Wallet Projection vs Strict Authoritative Snapshot；
- 冻结 Total Assets / Processing Assets 计算；
- 冻结统一 Exchange Resource Model；
- 冻结 Local Exchange Atomic Paths；
- 冻结 Active-shortfall Cross-DB Immutable Plan；
- 冻结 NewAPI Quota Operation Contract / Target-local Idempotency / Query-first；
- 冻结一用户单 Active-quota Movement Lane；
- 冻结 Durable Economy Reconciliation Job；
- 冻结 Bounded Active Buffer / Refill Formula / Production Gate；
- 冻结 Reserve→Active 复用统一 Transfer Engine；
- 冻结 Supply ISSUE/BURN Boundary；
- 冻结 Reward Policy Seed v1；
- 冻结 Registration/Migration Initial Grant exactly-once；
- 冻结 Daily exactly-once；
- 冻结 Hourly 双技术实现但 Production CONFIG_INCOMPLETE；
- 冻结 Relief 技术实现但 Production CONFIG_INCOMPLETE；
- 冻结 Reward Recovery / Maintenance Accepted-work Protection；
- 冻结 Economy / Reward Test Gate。

## Preserved

```text
TD-FRZ-001 ～ TD-FRZ-552
IS-FRZ-001 ～ IS-FRZ-216

Reward Product OPEN
Poker Product Gap 01～05
Public Record Selection Policy

SV-01 ～ SV-16 unresolved
DEPLOYMENT-VERIFY-01 pending
Implementation-only Config unresolved values
Production Readiness gates
```

## Supersession

```text
Existing IS-FRZ superseded:
None
```

---

# 239. Next Batch

> **IS-06 — Game Platform + V1 Direct Play Implementation Specification**

IS-06 将落实：

```text
Dynamic Game Registry
Global Wager Policy
Game Config Version
Round Engine
Round Acceptance / Resume
Fairness Commitment
Server Seed / Client Seed / Nonce
Reveal / Verification
Round Recovery / Refund

Dice
Scratch
Summon / Gacha
Slots
Blackjack

Typed Result
GAME_ISSUANCE / GAME_BURN
History Projection
Game Operations Boundary
Property / Fairness / Crash Tests
```

Poker 仍留 IS-07。

---

# 240. IS-06 — Game Platform + V1 Direct Play Implementation Specification

> 状态：`FROZEN`  
> 用户确认：`整体按上述 IS-06 方案通过`  
> Frozen Decision Range：`IS-FRZ-217 ～ IS-FRZ-282`  
> 覆盖：`Game Platform + Dice / Scratch / Summon / Slot / Blackjack`  
> Poker：明确留到 `IS-07`

## 240.1 Purpose

IS-06 将已冻结的：

```text
Dynamic Game Registry
Game Config Version
Global Wager Policy
Round Engine
Round Idempotency
Round Recovery
Settlement / Refund

Provably Fair
Server Seed
Client Seed
Nonce
Deterministic Stream

Dice
Scratch
Summon
Slot
Blackjack
```

落成：

```text
Go Package
Runtime Manifest
Forward Migration
Seed Data
Config Schema
Validator
Round Command
SQL Transaction
Typed Result
Fairness Byte Encoding
Golden Test Vector
Recovery Job
Math Validation Artifact
BFF DTO
Property Test
```

本批不新增 Direct Play Product Rule Gap。

---

# 241. Service / Package Boundary

五款 Direct Play：

```text
dice
scratch
summon
slot
blackjack
```

全部继续运行于：

```text
Chaldea Platform Backend
```

不新增独立 Game Service。

Poker：

```text
not part of Direct Play Round Engine
```

继续由独立：

```text
chaldea-poker
```

负责。

Backend 细化：

```text
backend/internal/games/
├── service.go
├── registry/
│   ├── manifest.go
│   ├── registry.go
│   └── compatibility.go
├── config/
│   ├── canonical.go
│   ├── hash.go
│   ├── validator.go
│   └── activation.go
├── wager/
│   └── policy.go
├── round/
│   ├── create.go
│   ├── action.go
│   ├── settlement.go
│   ├── refund.go
│   ├── recovery.go
│   └── projection.go
├── fairness/
│   ├── commitment.go
│   ├── seed.go
│   ├── stream.go
│   ├── sampling.go
│   ├── shuffle.go
│   └── verify.go
├── dice/
├── scratch/
├── summon/
├── slot/
└── blackjack/
```

Validation CLI：

```text
backend/cmd/chaldea-game-validate/main.go
dist/bin/chaldea-game-validate
```

它不是 Runtime Service。

---

# 242. Code-owned Runtime Manifest

每个 Game Implementation 编译进 Go Binary 并注册：

```text
implementation_key
game_slug

supported_config_schema_versions
supported_ruleset_versions
supported_algorithm_versions

entry_type
interaction_type

randomness_required
fairness_stream_versions

config_validator
round_creator
round_resolver
action_handler nullable
recovery_handler

result_serializer
history_serializer
fairness_verifier
```

V1 Implementation Keys：

```text
direct.dice.v1
direct.scratch.v1
direct.summon.v1
direct.slot.v1
direct.blackjack.v1
```

数据库只能引用这些 Code-owned Implementation Key。

禁止：

```text
eval database code
load arbitrary script
execute uploaded game logic
```

---

# 243. Effective Runtime Resolver

Game Effective Runtime 由以下共同决定：

```text
Publication State
Configured Runtime State
Implementation Exists
Config Compatible
Ruleset Compatible
Algorithm Compatible
Required Validation Artifact
Global Maintenance / Game Runtime Gate
```

对用户最终映射为：

```text
PLAY
RESUME
MAINTENANCE
TEMPORARILY_UNAVAILABLE
COMING_SOON
RETIRED
```

例如 Registry 标记 `PUBLISHED + AVAILABLE`，但 `implementation_key` 在当前 Build 不存在，则必须：

```text
effective runtime = TEMPORARILY_UNAVAILABLE
```

不得继续接受下注。

---

# 244. Direct Play Global Wager Policy v1

正式 Seed：

```text
version_number = 1

minimum_wager_units
= 10 Chips
= 5,000,000 units

maximum_mode
= NONE

quick_amount_units =
10 Chips   =   5,000,000
100 Chips  =  50,000,000
500 Chips  = 250,000,000
1000 Chips = 500,000,000

input_step_units
= 500,000
```

即：

```text
Direct Play 主动基础下注
必须为整数 Chip
```

不提供：

```text
Max
25%
50%
All-in
```

作为 Direct Play 快捷下注。

Summon Tenfold、Slot Line Stake、Blackjack Split / Double 可以从合法基础下注派生额外金额，但不能绕过基础最低下注策略。

---

# 245. IS-06 Forward Migrations

此前 Migration 不修改。

新增：

```text
000020__games_runtime_refinements
000021__games_v1_seed_and_validation
```

## `000020`

至少：

### `games.game_registry`

```text
active_config_version_id UUID NULL
```

### `games.wager_policy_versions`

```text
input_step_units BIGINT NOT NULL
policy_hash      BYTEA NOT NULL
```

### `games.game_config_versions`

```text
parent_version_id UUID NULL
algorithm_version TEXT NOT NULL
superseded_at     TIMESTAMPTZ NULL
```

### `games.game_rounds`

```text
implementation_key        TEXT NOT NULL
wager_policy_hash         BYTEA NOT NULL
fairness_stream_version   TEXT NOT NULL

recovery_state            TEXT NOT NULL DEFAULT 'NORMAL'

settlement_transaction_id UUID NULL
refund_transaction_id     UUID NULL
```

Recovery：

```text
NORMAL
RECOVERING
NEEDS_REVIEW
```

### `games.round_actions`

增加：

```text
action_sequence         BIGINT NOT NULL
additional_stake_units BIGINT NOT NULL DEFAULT 0
system_action           BOOLEAN NOT NULL DEFAULT FALSE
```

Unique：

```text
UNIQUE(round_id, action_sequence)
```

BFF `action_id` 直接使用 `action_id UUID PRIMARY KEY`，不增加第二套客户端 Action Identity。

---

# 246. Fairness Schema Refinements

新增：

```text
games.fairness_nonce_cursors
```

字段：

```text
newapi_user_id
game_slug
next_nonce BIGINT NOT NULL
version    BIGINT NOT NULL

PRIMARY KEY(newapi_user_id, game_slug)
```

分配：

```text
SELECT ... FOR UPDATE
nonce = next_nonce
next_nonce++
```

Gap 允许；Nonce 永不复用。

`games.fairness_commitments` 增加：

```text
ruleset_version
fairness_stream_version

game_config_version_id
game_config_hash

wager_policy_version_id
wager_policy_hash

resource_versions JSONB NOT NULL
revealed_server_seed BYTEA NULL
```

State：

```text
AVAILABLE
CONSUMED
REVEALED
INVALIDATED
```

DB 约束：

```text
one AVAILABLE commitment
per user/game
```

---

# 247. Validation Artifact

新增：

```text
games.game_validation_artifacts
```

字段：

```text
validation_artifact_id UUID PRIMARY KEY

game_slug
artifact_type
implementation_key

ruleset_version
algorithm_version
config_version_id
config_hash

validator_version
validation_build

result_summary JSONB

artifact_sha256 BYTEA

status

generated_at
verified_at
```

Status：

```text
GENERATED
VERIFIED
REJECTED
SUPERSEDED
```

Validation Artifact 不保存任何 Secret Seed。

---

# 248. V1 Version IDs

## Dice

```text
implementation_key = direct.dice.v1
ruleset_version    = dice-rules-v1
algorithm_version  = dice-map-v1
config_schema      = dice-config-v1
```

## Scratch

```text
implementation_key  = direct.scratch.v1
ruleset_version     = scratch-rules-v1
algorithm_version   = scratch-map-v1
config_schema       = scratch-config-v1
prize_table_version = scratch-prize-v1
```

## Summon

```text
implementation_key  = direct.summon.v1
ruleset_version     = summon-rules-v1
algorithm_version   = summon-map-v1
config_schema       = summon-config-v1
pool_id             = SUMMON_MAIN_V1
prize_table_version = summon-prize-v1
```

## Slot

```text
implementation_key = direct.slot.v1
ruleset_version    = slot-rules-v1
algorithm_version  = slot-map-v1
config_schema      = slot-config-v1

reel_strip_version = slot-strips-v1
payline_version    = slot-paylines-v1
paytable_version   = slot-paytable-v1
```

## Blackjack

```text
implementation_key        = direct.blackjack.v1
ruleset_version           = blackjack-rules-v1
algorithm_version         = blackjack-map-v1
config_schema             = blackjack-config-v1
shuffle_algorithm_version = blackjack-fy-v1
```

---

# 249. Config Canonical Encoding / Hash

固定：

```text
CHALDEA_GAME_CONFIG_CANONICAL_JSON_V1
```

规则：

```text
UTF-8

object keys:
ascending UTF-8 byte order

arrays:
order preserved

integers:
base-10
no leading zero except zero

boolean:
true / false

strings:
canonical JSON escaping

floating JSON number:
forbidden

duplicate object key:
forbidden

NaN / Infinity:
forbidden
```

概率使用整数 Weight；倍数使用整数 Multiplier；展示型小数指标使用 Decimal String。

Hash Input：

```text
ASCII("CHALDEA-GAME-CONFIG-V1\x00")
+
LP16(game_slug)
+
LP16(config_schema_version)
+
LP16(algorithm_version)
+
canonical_json(config_payload)
```

Hash：

```text
SHA-256
```

---

# 250. Config Lifecycle / Activation

Lifecycle：

```text
ACTIVE
→ Clone
→ DRAFT
→ VALIDATED
→ PREVIEWED
→ ACTIVE

old ACTIVE
→ SUPERSEDED
```

Active / Historical Config 永久 Immutable。

Activation：

```text
BEGIN

lock game_registry

load candidate
assert VALIDATED / PREVIEWED

verify:
  implementation_key
  config schema
  ruleset
  algorithm
  Product locks
  fairness compatibility
  required validation artifact

old active → SUPERSEDED
candidate → ACTIVE
registry.active_config_version_id = candidate

COMMIT
```

新 Config 只影响之后创建的 Round。

---

# 251. Game Validation CLI

支持：

```text
chaldea-game-validate dice
chaldea-game-validate scratch
chaldea-game-validate summon
chaldea-game-validate slot
chaldea-game-validate blackjack
chaldea-game-validate all
```

输出：

```text
dist/validation/games/
```

Artifact 文件：

```text
<game>-<ruleset>-<config-hash>.json
```

并生成：

```text
artifact_sha256
```

进入 Release / Validation Manifest。

---

# 252. Game Bootstrap / Round BFF

固定：

```http
GET /api/v1/games

GET /api/v1/games/{game_slug}/bootstrap

POST /api/v1/games/{game_slug}/rounds

GET /api/v1/games/{game_slug}/rounds/active

GET /api/v1/game-rounds/{round_id}

POST /api/v1/game-rounds/{round_id}/actions

GET /api/v1/games/{game_slug}/client-seed

PUT /api/v1/games/{game_slug}/client-seed

GET /api/v1/game-rounds/{round_id}/fairness
```

Bootstrap 返回：

```text
Registry Metadata
Effective Runtime State

Implementation Key
Ruleset Version
Algorithm Version

Config Summary / Version / Hash
Wager Policy / Version / Hash

Available Chips

Active Nonterminal Round nullable
Scratch Presentation Blocker nullable

Effective Entry Action

Next Fairness Commitment
Client Seed Preference
```

存在 Nonterminal Round：

```text
effective_entry_action = RESUME
```

不得引导创建第二 Round。

---

# 253. Typed Create Round

Header：

```http
Idempotency-Key
X-CSRF-Token
```

Dice：

```json
{
  "type": "DICE",
  "wager": "10",
  "choice": "BIG"
}
```

Scratch：

```json
{
  "type": "SCRATCH",
  "wager": "10"
}
```

Summon：

```json
{
  "type": "SUMMON",
  "base_wager": "10",
  "mode": "TENFOLD"
}
```

Slot：

```json
{
  "type": "SLOT",
  "total_wager": "10"
}
```

Blackjack：

```json
{
  "type": "BLACKJACK",
  "initial_wager": "10"
}
```

Client 永不提交：

```text
result
random bytes
server seed
nonce
config override
payout
next card
reel stop
prize tier
```

---

# 254. Round Idempotency / Acceptance

Round Create Semantic Fingerprint：

```text
game.round.create.v1
\0
user
\0
game_slug
\0
canonical typed wager/action inputs
```

同 Key / 同 Payload：

```text
return same Round
```

同 Key / 不同 Payload：

```text
409 IDEMPOTENCY_CONFLICT
```

任何付费 Round：

```text
Round
+
Accepted Stake
+
Wallet Ledger
+
Fairness Commitment Consumption
```

必须在同一 Chaldea DB Transaction。

---

# 255. Atomic Fast Settlement

以下四款：

```text
dice
scratch
summon
slot
```

正常路径采用单 PostgreSQL Transaction：

```text
BEGIN

validate:
  Auth
  Maintenance
  Runtime
  Implementation
  Config
  Wager
  Balance
  Idempotency
  Fairness Commitment

lock runtime gate
lock commitment
lock Available Chips

consume commitment

create Round
debit total stake
write Economy / Ledger

derive deterministic result
calculate total payout

if payout > 0:
  credit Available Chips
  write Economy / Ledger

persist typed game result

derive Round Net
write Game Supply Event based on Net

Round = SETTLED
reveal Server Seed

COMMIT
```

Commit 前故障：

```text
No Paid Round
No Wager Effect
No Result
```

Commit 成功、HTTP Response 丢失：

```text
complete SETTLED Round
→ duplicate request returns same Round
```

---

# 256. Direct Play Supply Accounting

```text
net_change_units
=
total_payout_units - total_stake_units
```

Supply Event：

```text
net > 0
→ GAME_ISSUANCE
→ ISSUE abs(net)

net < 0
→ GAME_BURN
→ BURN abs(net)

net = 0
→ no net supply event
```

Wallet 仍独立保存 Wager Debit 与 Payout Credit；Supply Event 只描述平台净新增 / 净回收。

---

# 257. Blackjack Create Transaction

Blackjack 不使用 Fast Settlement，但 Create Transaction 必须原子完成：

```text
Initial Wager
Fairness Commitment
Deterministic 312-card Shoe
Initial Deal
Required Dealer Peek
```

并推进到：

```text
first durable Player Decision Boundary
```

或直接 `SETTLED`。

概念：

```text
BEGIN

validate
lock runtime/commitment/wallet

debit Initial Wager
Ledger

consume commitment

create Round
regenerate deterministic Shoe
persist shoe_hash

deal:
  player card 1
  dealer upcard
  player card 2
  dealer hole card

if Dealer Peek required:
  execute Peek

if immediate terminal:
  settle
else:
  enter PLAYER_TURN

COMMIT
```

不得产生“Stake 已扣但 Initial Deal 缺失”的普通 V1 Durable State。

---

# 258. Settlement / Refund

Settlement：

```text
game_settlement:{round_id}
```

Refund：

```text
game_refund:{round_id}
```

每 Round 只能有：

```text
SETTLEMENT
or
REFUND
```

禁止：

```text
SETTLED + REFUNDED
```

Settlement 必须同事务提交：

```text
typed result
payout
wallet credit
ledger
supply event
round SETTLED
```

Refund 必须同事务提交：

```text
refundable accepted stake
wallet credit
refund ledger
round REFUNDED
reason
```

重复调用返回原结果。

---

# 259. Round Recovery / Maintenance

Durable Job：

```text
job_type = GAME_ROUND_RECOVERY

dedupe_key =
round:{round_id}
```

Recovery Priority：

```text
1 deterministic reconstruct
2 resume legal state
3 game-specific timeout automation
4 only if legal deterministic completion impossible:
  formal refund
```

Recovery 只能依赖 PostgreSQL Durable Facts。

Maintenance 与 Round Create 共享 Runtime Gate 顺序：

```text
Create committed first
→ accepted Round must continue/recover

Maintenance committed first
→ new Round rejected
```

Maintenance 不自动 Refund 已接受 Round。

---

# 260. Fairness Commitment Lifecycle

每 `user/game` 最多一个兼容 `AVAILABLE` Commitment。

Bootstrap：

```text
compatible AVAILABLE exists
→ return it

otherwise
→ invalidate incompatible old commitment
→ mint new commitment
```

Compatibility 至少包含：

```text
user
game
ruleset
algorithm
fairness stream
config version/hash
wager policy version/hash
game resource versions
client seed version
```

---

# 261. Server Seed / Encryption

Server Seed：

```text
exactly 32 random bytes
crypto/rand.Reader
```

Hash：

```text
SHA-256(raw server seed)
```

终态前使用：

```text
AES-256-GCM
```

加密。

Key：

```text
game_fairness_keyring
```

GCM nonce：

```text
12 random bytes
```

AAD：

```text
ASCII("CHALDEA-GAME-SEED-AAD-V1\x00")
+
commitment_uuid[16]
+
reserved_round_uuid[16]
+
LP16(newapi_user_id)
+
LP16(game_slug)
+
U64BE(nonce)
+
LP16(algorithm_version)
```

数据库保存：

```text
key_version
gcm_nonce
ciphertext
```

不保存 Key。

终态：

```text
revealed_server_seed = raw seed
state = REVEALED
```

Seed 永不复用。

---

# 262. Client Seed / Nonce

默认 Client Seed：

```text
cs1-
+
64 lowercase hex chars
```

Hex 来源：

```text
32 CSPRNG bytes
```

用户自定义：

```text
1–128 UTF-8 bytes
```

拒绝：

```text
NUL
CR
LF
other control chars
invalid UTF-8
```

Client Seed 不做 Unicode Normalization；其 UTF-8 原字节就是 Fairness Input。

修改只影响下一 Round：

```text
client_seed_version++
invalidate unused commitment
allocate new nonce
mint new commitment
```

Nonce per user/game：

```text
0..MaxInt64
monotonic allocation
Gap allowed
never reused
```

---

# 263. Provably Fair Stream v1

固定：

```text
fairness_stream_version =
chaldea-pf-hmac-sha256-v1
```

Key：

```text
server_seed[32]
```

每个 Block：

```text
HMAC-SHA-256(
  key = server_seed,
  message = CanonicalStreamMessage
)
```

`block_index = 0,1,2,...`

Primitive：

```text
U16BE
U64BE
UUID16
LP16(s) = U16BE(UTF8 byte length) || UTF8 bytes
```

Canonical Message：

```text
ASCII("CHALDEA-PF-HMAC-SHA256-V1")
0x00

LP16(game_slug)

UUID16(round_id)

U64BE(nonce)

LP16(client_seed)

LP16(algorithm_version)

LP16(domain)

U64BE(block_index)
```

Config / Rules / Paytable 不直接写入 Stream Message，而作为 Round-locked deterministic mapping inputs 参与最终 Verification。

---

# 264. Fairness Golden Vector v1

固定：

```text
server_seed_hex =
000102030405060708090a0b0c0d0e0f
101112131415161718191a1b1c1d1e1f

client_seed =
client-seed-demo

nonce =
42

round_id =
018f47a2-6e9d-7c31-8a4b-123456789abc

game_slug =
dice

algorithm_version =
dice-map-v1
```

Server Seed Hash：

```text
630dcd2966c4336691125448bbb25b4f
f412a49c732db2c8abc1b8581bd710dd
```

`dice:d1` Canonical Message Hex：

```text
4348414c4445412d50462d484d41432d
5348413235362d563100000464696365
018f47a26e9d7c318a4b123456789abc
000000000000002a0010636c69656e74
2d736565642d64656d6f000b64696365
2d6d61702d76310007646963653a6431
0000000000000000
```

HMAC Block 0：

```text
cd64aee5909adf6dad4968da9629db9f
b8674b713f1076b484704ba70cff75ba
```

Dice：

```text
dice:d1 → 4
dice:d2 → 4
dice:d3 → 1
```

结果：

```text
[4,4,1]
sum = 9
resolved_side = SMALL
triple = false
```

任何语言/重构必须继续通过该 Golden Vector。

---

# 265. Bias-free Sampling / Shuffle

UniformInt：

```text
1 <= N <= 2^32

x = U32BE(next 4 stream bytes)

limit =
2^32 - (2^32 mod N)

if x >= limit:
  reject and read next 4 bytes

else:
  result = x mod N
```

得到无偏 `0..N-1`。

Weighted Choice：

```text
total_weight = Σ nonnegative integer weights
r = UniformInt(total_weight)

iterate configured order
first cumulative > r wins
```

Deterministic Fisher–Yates：

```text
for i = n-1 down to 1:
  j = UniformInt(i+1)
  swap(a[i], a[j])
```

金融随机禁止 `math/rand`。

Domain Separation：

```text
Dice:
dice:d1
dice:d2
dice:d3

Scratch:
scratch:prize
scratch:filler
scratch:layout

Summon:
summon:draw:1 ... summon:draw:10

Slot:
slot:reel:1 ... slot:reel:5

Blackjack:
blackjack:shuffle
```

---

# 266. Fairness Endpoint / Common Result

Nonterminal Fairness：

```text
round_id
server_seed_hash
client_seed
nonce
fairness_stream_version
algorithm_version
locked config/resource versions
reveal_state = NOT_YET_AVAILABLE
```

禁止返回：

```text
server_seed
future stream
future Slot stop
Blackjack future Shoe
Dealer Hole Card
```

Terminal 后可返回：

```text
server_seed
verification inputs
verification algorithm
game-specific deterministic outputs
```

Common Result：

```text
net_change_units =
total_payout_units - total_stake_units

net > 0 → WIN
net = 0 → BREAK_EVEN
net < 0 → LOSS
```

异常：

```text
REFUNDED
CANCELLED_NO_EFFECT
```

---

# 267. Presentation Boundary

以下不是金融 Authority：

```text
Dice roll animation
Scratch pixel mask
Summon reveal animation
Slot reel animation
Blackjack card animation
```

允许：

```text
Skip
Fast Stop
Reduced Motion
Replay settled presentation
```

不得：

```text
reroll
redraw
re-settle
re-debit
change payout
```

V1 禁止：

```text
Auto Roll
Auto Scratch Buy
Auto Summon
Auto Spin
Auto Blackjack Strategy
```

---

# 268. Dice v1

规则：

```text
3 × d6

choice:
BIG
SMALL
```

Small：

```text
sum 4..10
AND not triple
```

Big：

```text
sum 11..17
AND not triple
```

Triple：

```text
1-1-1 ... 6-6-6
→ BIG loses
→ SMALL loses
```

Payout：

```text
win  → 2 × wager total payout
loss → 0
```

精确：

```text
SMALL = 105 / 216
BIG   = 105 / 216
TRIPLE = 6 / 216

RTP =
210 / 216
=
97.222222222...%
```

Typed Result：

```text
round_id
player_choice

die_1
die_2
die_3

total
is_triple

resolved_side:
  BIG
  SMALL
  TRIPLE

outcome
total_payout_units
net_change_units
```

Validator 必须穷举 216 Outcome。

---

# 269. Scratch v1

稳定 Functional IDs：

```text
P1
P2
P3
P5
P10
P25
P100
```

映射：

```text
P1   → BREAK_EVEN / 1x
P2   → T2 / 2x
P3   → T3 / 3x
P5   → T5 / 5x
P10  → T10 / 10x
P25  → T25 / 25x
P100 → TOP / 100x
```

Prize Table：

```text
LOSS         0x     54,000
BREAK_EVEN   1x     19,500
T2           2x     18,500
T3           3x      5,000
T5           5x      2,000
T10         10x        800
T25         25x        180
TOP        100x         20
```

Total Weight：

```text
100,000
```

RTP：

```text
96%
```

正式顺序：

```text
1 weighted sample Prize Tier
2 build valid functional multiset
3 deterministic layout shuffle
4 compute payout
5 Atomic Fast Settlement
```

不能随机九格后再判断输赢。

---

# 270. Scratch Canonical Layout / Reveal Gate

Winning：

```text
winning_symbol × 3
+
each of other six functional symbols × 1
```

得到恰好：

```text
one count = 3
six counts = 1
```

Loss：

```text
all seven functional symbols × 1
+
two symbols selected by deterministic scratch:filler
each duplicated once
```

因此任一 count ≤2。

最终用：

```text
scratch:layout
```

执行 Fisher–Yates。

Typed Result 增加：

```text
prize_tier
presentation_completed_at
```

Cells 增加：

```text
is_matching_symbol
```

Pixel Scratch Mask 不作为 Durable Financial State。

若 Round 已 `SETTLED` 但：

```text
presentation_completed_at = NULL
```

则新 Scratch 购买返回：

```text
SCRATCH_PREVIOUS_REVEAL_INCOMPLETE
```

`SCRATCH_REVEAL_COMPLETE` 只更新 Presentation Completion，不产生资产效果，不重新 Settlement。

---

# 271. Summon v1

Pool：

```text
SUMMON_MAIN_V1
```

模式：

```text
SINGLE
draw_count = 1

TENFOLD
draw_count = 10
```

Base Wager：

```text
per draw
```

Cost：

```text
Single = base_wager
Tenfold = base_wager × 10
```

Tenfold：

```text
one Round
one total debit
one settlement
```

无：

```text
Discount
11th Draw
Guarantee
Pity
Rate-up
Reroll
Cross-round Probability State
```

Prize Table：

```text
T0   0x    59,850
T1   1x    25,000
T2   2x    10,000
T3   5x     4,000
T4  20x     1,050
T5 100x       100
```

RTP：

```text
96%
```

---

# 272. Summon Indexed Determinism

Draw Index：

```text
1..10
```

Domain：

```text
summon:draw:{draw_index}
```

每 Draw 独立：

```text
UniformInt(100000)
→ weighted tier
```

Draw N 不依赖 Draw N-1 结果。

Typed：

```text
summon_results:
  round_id
  summon_mode
  draw_count
  base_wager_units
  total_cost_units
  pool_id
  prize_table_version
  highest_tier

summon_draw_results:
  round_id
  draw_index
  reward_tier
  payout_multiplier
  payout_units
```

Tenfold Round Result 只按整轮 Net。

---

# 273. Slot v1 Exact Reel Strips

## Reel 1

```text
L3,L2,L3,M1,L2,L1,L3,L2,
M2,H2,L1,M2,L3,M1,H1,L2,
L1,H1,L1,M2,L2,L3,L1,M1,
W,L1,L2,L1,L2,H2,M1,L1
```

## Reel 2

```text
L2,M2,L1,L2,L3,L2,W,L1,
H1,L1,M1,L3,M1,H2,M1,L2,
L1,L3,L1,L3,H1,L1,H2,L3,
L1,L2,M2,L2,L1,L2,M1,M2
```

## Reel 3

```text
L2,L3,L1,M2,L1,H1,L3,L1,
M1,M2,L2,M1,L3,L1,L2,L3,
H2,L2,L1,M1,L2,L1,H1,L2,
L1,H2,M1,L3,L1,L2,M2,W
```

## Reel 4

```text
M1,L3,W,L2,L1,H2,L2,M2,
L1,M1,L1,L3,H1,L2,L3,M2,
H1,L1,L2,L3,M1,L1,L3,L1,
L2,H2,L1,M1,M2,L2,L1,L2
```

## Reel 5

```text
L1,H2,L2,L1,L3,L2,L3,L1,
H1,L2,L1,L2,M1,L1,W,H2,
M1,L2,H1,L1,L2,L1,M1,M2,
L3,M1,L2,L1,M2,L3,M2,L3
```

每 Reel 32 Stops。

Frequency：

```text
L1 8
L2 7
L3 5
M1 4
M2 3
H1 2
H2 2
W  1
```

不能根据 Frequency 重新生成所谓“等价 Strip”。

---

# 274. Slot Stop / Grid / Paylines

每 Reel：

```text
stop_index = UniformInt(32)
```

Visible：

```text
T = stop - 1 mod 32
M = stop
B = stop + 1 mod 32
```

五 Reel 独立 Domain：

```text
slot:reel:1
...
slot:reel:5
```

Grid Serialization：

```text
Reel-major:
R1[T,M,B], R2[T,M,B], ... R5[T,M,B]
```

Row：

```text
T=0
M=1
B=2
```

Paylines：

```text
1  [1,1,1,1,1]
2  [0,0,0,0,0]
3  [2,2,2,2,2]
4  [0,1,2,1,0]
5  [2,1,0,1,2]
6  [0,0,1,2,2]
7  [2,2,1,0,0]
8  [1,0,0,0,1]
9  [1,2,2,2,1]
10 [2,1,1,1,0]
```

只从 Reel 1 向 Reel 5 判断，至少 3 连。

---

# 275. Slot Paytable / Wager

Multiplier 基于 Line Stake，并表示 Total Payout：

```text
       3     4      5
L1     4     15     50
L2     8     25     80
L3    10     40    150
M1    15     60    250
M2    25    100    500
H1    50    250   1000
H2   100    500   2500
W    125   1000   5000
```

Wild：

```text
substitutes L1..H2
+
has own paytable
```

同一 Payline：

```text
enumerate all legal ordinary-symbol interpretations
+
pure Wild interpretation

choose highest payout exactly once
```

5 连不叠加同线 3/4 连。

Total Wager：

```text
whole Chip
>= 10
```

Line Stake：

```text
total_wager_units / 10
```

因为 1 Chip=500,000 units，所以 1 Chip /10 =50,000 units，无损。

---

# 276. Slot Exhaustive Validation

必须枚举：

```text
32^5 = 33,554,432
```

全部 Stop Combination。

Expected：

```text
RTP
= 96.0033118724823%

any nonzero line payout
= 41.4642333984375%

zero line payout
= 58.5357666015625%

round net profit
= 19.3924903869629%

break even
= 3.46889495849609%

round net loss
= 77.1386146545410%

partial payout but net loss
= 18.6028480529785%

max round total payout
= 516.4 × Total Wager
```

Validator 内部使用整数 / Rational 累积，最后才格式化 Decimal。

不得用 Float Tolerance 掩盖 Strip/Paytable 变化。

---

# 277. Blackjack Canonical Card Encoding

Suit：

```text
0 = C / Clubs
1 = D / Diamonds
2 = H / Hearts
3 = S / Spades
```

Rank：

```text
0  A
1  2
2  3
3  4
4  5
5  6
6  7
7  8
8  9
9  T
10 J
11 Q
12 K
```

Card Code：

```text
card_code =
suit_index * 13 + rank_index
```

范围：

```text
0..51
```

Six-deck Card Instance：

```text
deck_copy_index = 0..5

card_instance_id =
deck_copy_index * 52 + card_code
```

范围：

```text
0..311
```

Canonical Unshuffled Input：

```text
deck 0: card_code 0..51
deck 1: card_code 0..51
...
deck 5: card_code 0..51
```

即：

```text
[0,1,2,...,311]
```

---

# 278. Blackjack Shuffle / Shoe Hash

Domain：

```text
blackjack:shuffle
```

对 `[0..311]` 运行 Deterministic Fisher–Yates：

```text
shoe[0..311]
```

`shoe_index` 0-based。

每次发牌：

```text
card_instance_id = shoe[shoe_index]
shoe_index++
```

Shoe Serialization：

```text
each card_instance_id
→ U16BE
```

总长：

```text
624 bytes
```

Hash：

```text
shoe_hash =
SHA-256(serialized shoe)
```

Recovery 必须重新生成 Shoe 并验证 Hash 与每条 Dealt Card 的 `shoe_index`。

Mismatch：

```text
Round → NEEDS_REVIEW
```

不得继续发牌。

---

# 279. Blackjack Initial Deal / Peek / Privacy

Initial Deal：

```text
shoe[0] → Player Card 1
shoe[1] → Dealer Upcard
shoe[2] → Player Card 2
shoe[3] → Dealer Hole Card
```

之后：

```text
shoe_index = 4
```

Dealer Hole Card：

```text
Durable
NOT_YET_PUBLIC
```

合法 Reveal 前：

```text
Browser cannot read
Operations cannot read
Logs cannot read
Audit cannot read
```

如果 Upcard：

```text
A
or
10/J/Q/K
```

必须 Player Action 前 Peek。

Dealer Blackjack：

```text
Player Natural
→ PUSH

Player not Natural
→ Initial Wager LOSS

→ immediate Settlement
```

Dealer 无 Blackjack则继续 PLAYER_TURN，但 Hole Card 仍隐藏。

---

# 280. Blackjack Natural / Payout

Natural 仅：

```text
Original
Unsplit
Initial two cards
A + any 10-value card
```

Split Hand 两张牌 21：

```text
ordinary 21
not Natural
```

每 Hand：

```text
LOSS / BUST
→ 0 × stake

PUSH
→ 1 × stake

NORMAL_WIN
→ 2 × stake

NATURAL
→ 2.5 × original initial stake
```

Natural Payout：

```text
payout_units =
stake_units * 5 / 2
```

由于主动下注为整 Chip，因此可以精确得到 0.5 Chip；禁止任何舍入。

---

# 281. Blackjack Hand / Action Engine

Hand State：

```text
ACTIVE
STOOD
BUST
DOUBLED_COMPLETE
SPLIT_ACES_COMPLETE
NATURAL_COMPLETE
```

Stable：

```text
hand_id
hand_index
```

顺序：

```text
left → right
```

一次仅 `active_hand_id` 接收 Player Action。

## Hit

```text
consume next Shoe card
persist card
recalculate total

>21 → BUST
=21 → STOOD
else → ACTIVE
```

## Stand

```text
ACTIVE → STOOD
```

然后进入下一未完成 Hand；无则进入 Dealer Turn。

---

# 282. Blackjack Double

允许：

```text
Original Unsplit Two-card Hand
or
Non-Aces Split Two-card Hand
```

DAS 开启。

Split Aces 不允许。

Transaction：

```text
BEGIN

lock Round
lock Hand
lock Available Chips Wallet

validate:
  action id
  round version
  active hand
  two-card eligibility
  balance

additional stake =
current hand stake

debit
Ledger

Hand Stake *= 2

consume exactly one Shoe card
persist dealt card

Hand = DOUBLED_COMPLETE

write Action
round_version++

if all hands complete:
  Dealer Engine + Settlement

COMMIT
```

---

# 283. Blackjack Split / Re-split / Split Aces

Split Eligibility：

```text
exactly two cards
same Blackjack Point Value
```

因此：

```text
8 + 8
A + A
10 + J
Q + K
10 + Q
```

均可 Split。

Transaction：

```text
BEGIN

lock Round
lock Active Hand
lock Wallet

assert hand_count < 4
assert balance

additional stake =
current hand stake

debit
Ledger

split original cards
→ left/right Hands

draw one Shoe card to left
draw one Shoe card to right

assign stable hand_id/index

write Action
round_version++

COMMIT
```

Non-Aces 子手再次满足条件：

```text
may re-split
```

最多：

```text
4 hands
```

Split Aces：

```text
A + A
→ split

each child:
  exactly one additional card
  → SPLIT_ACES_COMPLETE

No Hit
No Double
No Re-split Aces
```

A + 10-value：

```text
ordinary 21
not Natural
```

---

# 284. Dealer S17 / Blackjack Settlement

Dealer：

```text
Hard <=16 → Hit
Soft <=16 → Hit

Hard >=17 → Stand
Soft 17   → Stand
Soft >=18 → Stand
```

所有玩家 Bust：

```text
Reveal Hole Card
No additional Dealer Draw
→ Settlement
```

Round：

```text
Total Stake
=
Initial Wager
+
all Split Additional Stakes
+
all Double Additional Stakes

Total Payout
=
sum(each Hand payout)

Round Net
=
Total Payout - Total Stake
```

Class：

```text
Net <0 → LOSS
Net =0 → BREAK_EVEN
Net >0 → WIN
```

不能以部分 Hand Win 覆盖整轮 Net Loss。

---

# 285. Blackjack Action Idempotency

BFF：

```http
POST /api/v1/game-rounds/{round_id}/actions
```

Body：

```json
{
  "action_id": "<uuid>",
  "expected_round_version": "12",
  "hand_id": "<uuid>",
  "action_type": "DOUBLE"
}
```

Player Types：

```text
HIT
STAND
DOUBLE
SPLIT
```

System / Presentation-specific Types：

```text
SYSTEM_AUTO_STAND
SCRATCH_REVEAL_COMPLETE
```

每个 Game Handler 只接受合法 Action Type。

同 `action_id`：

```text
return original result
```

Stale：

```text
409 BLACKJACK_STALE_ROUND_VERSION
+
authoritative current Round snapshot
+
legal actions
```

不得重复发牌、Double Debit、Split Debit、Hand Creation。

---

# 286. Blackjack 24h Inactivity

字段：

```text
last_player_action_at
auto_resolve_at
```

第一次进入 `PLAYER_TURN` 且尚无 Player Action：

```text
last_player_action_at =
PLAYER_TURN entered_at
```

作为第一次 Inactivity Anchor。

每次成功人工：

```text
HIT
STAND
DOUBLE
SPLIT
```

后：

```text
last_player_action_at = DB now()
auto_resolve_at = last_player_action_at + 24h
```

System Action 不延长人工操作窗口。

Durable Job：

```text
job_type =
BLACKJACK_AUTO_RESOLVE

dedupe_key =
blackjack:auto-resolve:{round_id}
```

到期：

```text
lock Round

if not PLAYER_TURN:
  reconcile/no-op

if now < auto_resolve_at:
  reschedule

for unfinished Player Hands
left-to-right:
  write SYSTEM_AUTO_STAND

Dealer executes S17
normal Settlement
```

不是 Refund，不重新洗牌。

---

# 287. Blackjack Recovery / Fairness / RTP

Refresh / Restart 恢复：

```text
same round_id
same Server Seed / Hash
same 312-card Shoe
same shoe_hash
same shoe_index
same Dealer Hole Card
same Player Hands
same hand indexes
same stakes
same actions
same active hand
same legal actions
same auto_resolve_at
```

先重建 Shoe 并验证：

```text
shoe_hash
dealt card history
shoe_index
```

再允许下一 Action。

Nonterminal：

```text
Server Seed secret
Future Shoe secret
Dealer Hole Card secret
```

Terminal：

```text
Reveal Server Seed
Rebuild complete Shoe
Verify all shoe_index
Verify all Hand Results
Verify Payout
```

Blackjack Production RTP / House Edge 不写死。

必须存在 VERIFIED：

```text
artifact_type = BLACKJACK_RTP
```

内容至少：

```text
ruleset_version
shuffle_algorithm_version
reference_strategy_version

validation_method
sample_count / enumeration metadata

computed_rtp
computed_house_edge

validator_version
validation_build
artifact_hash
verified_at
```

由 Frozen Rules + Reference Basic Strategy + Reproducible Enumerator/Large Simulation 计算。

---

# 288. Typed Result Refinements

`000020` 至少补齐：

## Dice

```text
resolved_side
```

## Scratch

```text
prize_tier
presentation_completed_at
```

Cells：

```text
is_matching_symbol
```

## Summon

```text
summon_mode
base_wager_units
total_cost_units
pool_id
prize_table_version
```

## Slot

```text
grid_symbol_ids
reel_strip_version
payline_version
paytable_version
```

Line：

```text
resolved_symbol_id
```

## Blackjack Round

```text
shuffle_algorithm_version
shoe_hash
last_player_action_at
auto_resolve_at
```

Hands：

```text
is_from_split
is_split_aces
is_natural
hard_total
best_total
result
payout_units
net_change_units
```

Dealt Cards：

```text
card_instance_id
recipient_kind
public_state
```

Typed Result Authority 不允许退化成任意 JSONB。

---

# 289. `000021` V1 Seed

Seed：

```text
five Registry entries
global wager policy v1
five initial active configs

Scratch prize table v1

Summon prize table v1
Summon main pool

Slot exact strips
Slot paylines
Slot paytable
```

Dice / Scratch / Summon / Slot 只有在 Validation Artifact 验证通过后才 Effective AVAILABLE。

Blackjack 即使 Active Config 存在，缺少 VERIFIED `BLACKJACK_RTP` Artifact 时：

```text
effective runtime =
TEMPORARILY_UNAVAILABLE
```

---

# 290. Error / History / Operations

Common Errors：

```text
GAME_NOT_AVAILABLE
GAME_MAINTENANCE
INVALID_WAGER
INSUFFICIENT_CHIPS
ACTIVE_ROUND_EXISTS
IDEMPOTENCY_CONFLICT
FAIRNESS_COMMITMENT_INVALID
CONFIG_VERSION_UNAVAILABLE
ROUND_NEEDS_REVIEW
```

Dice：

```text
DICE_CHOICE_REQUIRED
DICE_INVALID_CHOICE
```

Scratch：

```text
SCRATCH_PREVIOUS_REVEAL_INCOMPLETE
```

Summon：

```text
SUMMON_INVALID_MODE
SUMMON_TOTAL_COST_EXCEEDS_BALANCE
```

Blackjack：

```text
BLACKJACK_HAND_NOT_ACTIVE
BLACKJACK_ACTION_NOT_ALLOWED
BLACKJACK_STALE_ROUND_VERSION
BLACKJACK_MAX_HANDS_REACHED
BLACKJACK_INSUFFICIENT_CHIPS_FOR_DOUBLE
BLACKJACK_INSUFFICIENT_CHIPS_FOR_SPLIT
BLACKJACK_AUTO_RESOLVING
```

有 Round 时 Error 附：

```text
round_id
safe authoritative state
safe next action
```

History Index 继续仅为 Rebuildable List Projection；Round Detail 从 Common + Typed Result + Economy + Fairness 读取。

Retired Game：

```text
No New Round
```

但保留：

```text
Round Detail
History
Historical Config
Fairness
Ranking Source
Validation Artifact
```

Operations 允许未来：

```text
Metadata
Publish / Hide
Safe Runtime
Clone / Validate / Preview / Activate Config
Inspect Validation / Fairness / History
```

禁止：

```text
edit settled result
edit payout
edit historical seed
edit historical config
replace reel stop
replace blackjack shoe
force accepted round to new config
```

---

# 291. Audit / Metrics

Audit 至少：

```text
GAME_REGISTERED
GAME_METADATA_CHANGED
GAME_PUBLISHED
GAME_RUNTIME_CHANGED
GAME_RETIRED

GAME_CONFIG_DRAFT_CREATED
GAME_CONFIG_VALIDATED
GAME_CONFIG_ACTIVATED

ROUND_ACCEPTED
ROUND_ACTION_ACCEPTED
ROUND_RECOVERY_STARTED
ROUND_SETTLED
ROUND_REFUNDED
ROUND_NEEDS_REVIEW

FAIRNESS_COMMITMENT_CREATED
FAIRNESS_COMMITMENT_INVALIDATED
FAIRNESS_COMMITMENT_CONSUMED
FAIRNESS_SEED_REVEALED

GAME_VALIDATION_ARTIFACT_VERIFIED
```

Audit 禁止：

```text
unrevealed Server Seed
future Blackjack Shoe
hidden Dealer Hole Card
```

Metrics 至少：

```text
game_rounds_total{game,state}
active_rounds{game}

round_accept_latency
round_resolve_latency
round_settlement_latency

round_recovery_count
oldest_recovering_round_age
round_refund_count
round_needs_review_count

duplicate_round_request_count
duplicate_action_request_count

config_validation_failure
config_activation_count

fairness_commitment_failure
fairness_nonce_conflict
fairness_verification_failure

game_runtime_state
implementation_manifest_mismatch

blackjack_auto_resolve_count
blackjack_recovery_validation_failure

slot_validation_duration
slot_validation_failure
```

Alert Threshold 继续 `UNRESOLVED_IMPLEMENTATION_CONFIG`。

---

# 292. IS-06 Test Gate

## Common

```text
Dynamic registry extensibility
Unknown implementation → unavailable
Active config immutable
Historical config retained

100 same idempotency → one round
same user/game → one nonterminal round
wager + round atomic
settlement/refund mutually exclusive
maintenance/create race deterministic
Redis loss preserves round truth
```

## Fairness / Privacy

```text
Server Seed exactly 32 bytes
Seed hash exact
AES-GCM AAD roundtrip
wrong AAD fails
Nonce never reused
Client Seed exact UTF-8 bytes

Golden Vector byte-identical

Uniform rejection sampling
Weighted selection exact
Fisher-Yates deterministic

terminal verification recreates result/payout

nonterminal:
no Server Seed
no future randomness
no future Blackjack Shoe
no Dealer Hole Card
```

## Dice

```text
216 outcomes
105/105/6
RTP exact
Triple loses both
Golden [4,4,1] → SMALL
```

## Scratch

```text
weights 100000
RTP 96%

winning:
one count exactly 3
others exactly 1

loss:
all count <=2

no second triple
same seed → same layout
Reveal All no wallet effect
Refresh same logical cells
presentation incomplete blocks next card
```

## Summon

```text
weights 100000
RTP 96%

Single exactly one draw
Tenfold exactly ten indexed draws

one Round
one total debit
one settlement

no pity/discount/guarantee/rate-up
same inputs/index → same tier
draw N independent from prior draw outcome
```

## Slot

```text
five strips length 32
exact frequency
exact strip order
10 paylines exact
paytable exact
32^5 exhaustive combinations

RTP = 96.0033118724823%
nonzero payout = 41.4642333984375%
zero payout = 58.5357666015625%
profit = 19.3924903869629%
break-even = 3.46889495849609%
loss = 77.1386146545410%
partial payout loss = 18.6028480529785%
max payout = 516.4×

Wild highest interpretation once
5-chain no same-line 3/4 stacking
Fast Stop no result change
```

## Blackjack

```text
312 unique card instances
canonical card/deck mapping
same seed → same Shoe
shoe hash exact

Initial Deal exact
Dealer Peek exact
Natural exact
3:2 half-chip exact

Double:
one added stake
one card
auto complete
DAS
Split Aces denied

Split:
ten-value cross-rank allowed
max 4 hands

Split Aces:
one card
no Hit
no Double
no re-split

left-to-right
S17 exact
all-player-bust no unnecessary draw

same action ID one effect
stale version no card/debit

restart at every action sequence
→ same Shoe/Hands/Active Hand

24h anchor/refresh/auto-resolve exact
```

---

# 293. Codex IS-06 Implementation Order

```text
01 migrations 000020 / 000021

02 Runtime Manifest / Registry
03 Effective Runtime Resolver

04 Wager Policy v1
05 Config Canonical Encoder
06 Config Hash
07 Config Lifecycle / Activation

08 Fairness Seed Encryption
09 Nonce Cursor
10 Client Seed
11 Commitment Lifecycle

12 HMAC Stream v1
13 Uniform Sampling
14 Weighted Choice
15 Fisher-Yates
16 Golden Vectors

17 Generic Round Repository
18 Round Create Idempotency
19 Settlement / Refund
20 Recovery Job

21 Dice implementation + validator

22 Scratch prize/layout
23 Scratch presentation gate
24 Scratch validator

25 Summon pool/table
26 Summon indexed draw
27 Summon validator

28 Slot strip/payline/paytable
29 Slot resolver
30 32^5 validator

31 Blackjack card encoding
32 Blackjack shoe/shuffle
33 Initial Deal / Peek
34 Hand Engine
35 Hit / Stand
36 Double
37 Split / Re-split
38 Split Aces
39 Dealer S17
40 Settlement
41 24h Auto Resolve
42 Blackjack Validation Artifact

43 Game BFF
44 Fairness BFF
45 History Projection

46 Metrics / Audit
47 Crash / Property / Privacy Tests
```

---

# 294. IS-06 Prohibited Implementation

禁止：

```text
no-code game executor
eval config script
math/rand for financial result
modulo-biased random mapping
unversioned Fairness encoding changes
Nonce reuse
Server Seed reuse
unrevealed Seed logs
future Blackjack Shoe exposure
Dealer Hole Card early exposure

animation decides result
Auto Roll/Spin/Summon/Blackjack strategy

duplicate paid Round on retry
historical active Config edit

Slot strips regenerated from frequency
silent Slot math change

hard-coded guessed Blackjack RTP
rounded Natural payout
extra Split-Aces action

Fast Settlement replay caused by animation replay
Blackjack refund solely because page closed

Poker embedded into Direct Play Engine
```

---

# 295. IS-06 Acceptance Criteria

```text
AC-06-01  Dynamic code-owned Game Registry
AC-06-02  Five V1 games are instances, not permanent platform limit
AC-06-03  Global wager 10 min / no product max / 10-100-500-1000
AC-06-04  Deterministic Config hash
AC-06-05  Active Config immutable
AC-06-06  Round locks ruleset/algorithm/config/wager/fairness
AC-06-07  same idempotency key → at most one Round
AC-06-08  one user/game nonterminal Round
AC-06-09  Dice/Scratch/Summon/Slot Fast Settlement
AC-06-10  Blackjack Durable Multi-action
AC-06-11  Settlement/Refund mutually exclusive
AC-06-12  Recovery uses PostgreSQL facts
AC-06-13  Maintenance preserves accepted Round
AC-06-14  Precommit Fairness for every random Round
AC-06-15  256-bit Server Seed
AC-06-16  unrevealed Seed AES-256-GCM
AC-06-17  Client Seed next-round only
AC-06-18  Nonce non-reused per user/game
AC-06-19  HMAC canonical encoding frozen as v1
AC-06-20  Golden vector stable
AC-06-21  rejection sampling for finite ranges
AC-06-22  deterministic unbiased Fisher-Yates
AC-06-23  Fairness reconstructs result/payout
AC-06-24  Presentation has zero result authority
AC-06-25  no auto-play economic loop

AC-06-26  Dice 105/105/6
AC-06-27  Dice exact RTP

AC-06-28  Scratch exact 96% prize table
AC-06-29  Scratch winning card exactly one triple
AC-06-30  Scratch loss no 3+ symbol
AC-06-31  Scratch Reveal All no economy effect

AC-06-32  Summon exact 96%
AC-06-33  Tenfold one Round / ten indexed draws
AC-06-34  no pity/guarantee/rate-up/cross-round state

AC-06-35  Slot frozen strips verbatim
AC-06-36  Slot exact paylines/paytable
AC-06-37  32^5 exhaustive metrics pass
AC-06-38  max Round payout 516.4×
AC-06-39  Wild highest interpretation once

AC-06-40  deterministic Blackjack card/deck encoding
AC-06-41  same fairness input → same Shoe
AC-06-42  Initial Deal exact
AC-06-43  Hole Card private
AC-06-44  Dealer Peek before action
AC-06-45  Natural / 3:2 exact
AC-06-46  Double atomic stake+card
AC-06-47  DAS supported
AC-06-48  ten-value cross-rank Split
AC-06-49  max four hands
AC-06-50  Split Aces exact restrictions
AC-06-51  left-to-right hands
AC-06-52  S17 exact
AC-06-53  Round result by aggregate Net
AC-06-54  action retry no duplicate card/stake
AC-06-55  restart same Shoe/Hands
AC-06-56  24h Auto Stand idempotent
AC-06-57  RTP from validation artifact

AC-06-58  Retired game remains verifiable
AC-06-59  Operations cannot edit settled random/financial facts
AC-06-60  no unrevealed randomness in Log/Audit/Ops DTO
```

---

# 296. IS-06 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| IS-FRZ-217 | Direct Play Game Platform 继续位于 Platform Backend；Dice/Scratch/Summon/Slot/Blackjack 是 Dynamic Registry 实例，Poker 不进入该 Round Engine。 | FROZEN |
| IS-FRZ-218 | 每个 Game 通过 Code-owned Runtime Manifest 注册 implementation/config/ruleset/algorithm/capability/validator/resolver/recovery；数据库不得执行任意游戏代码。 | FROZEN |
| IS-FRZ-219 | Effective Runtime 必须同时验证 Publication、Configured Runtime、Implementation、Config、Ruleset、Algorithm 与 Validation Artifact；任一不匹配即 TEMPORARILY_UNAVAILABLE。 | FROZEN |
| IS-FRZ-220 | Direct Play Wager Policy v1 固定最低 10 Chips、产品最高上限 NONE、快捷 10/100/500/1000、基础主动下注整 Chip。 | FROZEN |
| IS-FRZ-221 | IS-06 使用新增 `000020__games_runtime_refinements` 与 `000021__games_v1_seed_and_validation`，不修改此前已冻结 Migration。 | FROZEN |
| IS-FRZ-222 | Game Config Hash 固定使用 `CHALDEA_GAME_CONFIG_CANONICAL_JSON_V1` + SHA-256；禁止 Float 与不确定 JSON Serialization 成为 Hash Authority。 | FROZEN |
| IS-FRZ-223 | Game Config 使用 Clone→Draft→Validated→Previewed→Active→Superseded，Active/Historical Version Immutable；Activation 必须原子换指针并验证兼容性。 | FROZEN |
| IS-FRZ-224 | 建立 immutable `game_validation_artifacts`；Dice/Scratch/Summon/Slot/Blackjack Production Activation 必须通过对应可重复数学/规则验证。 | FROZEN |
| IS-FRZ-225 | Paid Round Create 使用 Durable Idempotency + Round/Stake/Fairness same transaction；同 Key/同语义返回原 Round，不同语义 409。 | FROZEN |
| IS-FRZ-226 | 同 user/game 最多一个 Nonterminal Round；Bootstrap 为 Resume-first，并可同时暴露 Scratch Terminal Presentation Blocker。 | FROZEN |
| IS-FRZ-227 | Dice/Scratch/Summon/Slot 正常路径采用一个 PostgreSQL Atomic Fast Settlement Transaction 完成 Wager、Result、Payout、Ledger、Typed Result 与 SETTLED。 | FROZEN |
| IS-FRZ-228 | Blackjack Create Transaction 原子完成 Initial Wager、Commitment、Deterministic Shoe、Initial Deal、必要 Peek，并推进到第一 Player Decision Boundary 或直接 Settlement。 | FROZEN |
| IS-FRZ-229 | Settlement 使用 `game_settlement:{round}`、Refund 使用 `game_refund:{round}`；同一 Round 两者永久互斥且重复调用返回原结果。 | FROZEN |
| IS-FRZ-230 | Game Recovery 顺序固定为 Deterministic Reconstruct→Resume→Game Timeout Automation→仅无法合法完成时 Refund；Maintenance 不自动退款。 | FROZEN |
| IS-FRZ-231 | Next-round Fairness Commitment 绑定 User/Game/Ruleset/Algorithm/Config/Wager/Resource Version/Client Seed/Nonce；每 user/game 最多一个兼容 AVAILABLE Commitment。 | FROZEN |
| IS-FRZ-232 | Server Seed 固定 32-byte CSPRNG；SHA-256 Precommit，终态前 AES-256-GCM 加密、终态后保存 Reveal Seed，Seed 永不复用。 | FROZEN |
| IS-FRZ-233 | Client Seed 默认 `cs1-` + 32-byte CSPRNG hex；允许 1–128 UTF-8 bytes 自定义，不做 Unicode Normalization，修改只影响下一 Round 并失效旧 Commitment。 | FROZEN |
| IS-FRZ-234 | Fairness Nonce 通过 `fairness_nonce_cursors` per-user/game Row Lock 单调分配；Gap 允许但任何已分配 Nonce 永不复用。 | FROZEN |
| IS-FRZ-235 | 公共随机流固定 `chaldea-pf-hmac-sha256-v1`，使用 Server Seed 作为 HMAC-SHA-256 Key，并冻结 LP16/U64BE/UUID16 精确 Canonical Message Encoding。 | FROZEN |
| IS-FRZ-236 | IS-06 冻结公共 Fairness Golden Vector；给定固定 Seed/Client Seed/Nonce/Round/Algorithm 时 `dice:d1/d2/d3` 必须产生 `[4,4,1]`。 | FROZEN |
| IS-FRZ-237 | V1 Uniform Integer Mapping 使用 U32BE + Rejection Sampling，禁止直接 Modulo Bias；所有当前 Direct Play Range ≤2^32。 | FROZEN |
| IS-FRZ-238 | Scratch/Summon Weighted Choice 使用整数总权重与无偏 Uniform Sample，配置顺序是确定性映射的一部分。 | FROZEN |
| IS-FRZ-239 | Array/Deck Shuffle 固定 Deterministic Fisher–Yates，每次索引来自无偏 Uniform Sampling；金融随机禁止 `math/rand`。 | FROZEN |
| IS-FRZ-240 | Game BFF 固定 Catalog/Bootstrap/Create/Active/Detail/Action/Client Seed/Fairness 路径，Create Body 为 Game-specific Typed Union。 | FROZEN |
| IS-FRZ-241 | Direct Play Presentation/Skip/Fast Stop/Reveal 不属于结果 Authority；V1 不允许 Auto Roll/Spin/Summon/Blackjack Strategy。 | FROZEN |
| IS-FRZ-242 | 所有 Direct Play 统一保存 Total Stake/Payout/Net，Round-level WIN/LOSS/BREAK_EVEN 只按 Net 判断；History 保留锁定版本与 Fairness。 | FROZEN |
| IS-FRZ-243 | Dice v1 固定 3d6 Big/Small、Triple Both Lose、Win Total Payout 2×，并保持 105/105/6 与 97.222222…% RTP。 | FROZEN |
| IS-FRZ-244 | Dice 使用三个独立 Fairness Domain 的无偏 d6 Sample；Validator 必须穷举全部 216 Outcome。 | FROZEN |
| IS-FRZ-245 | Scratch 固定内部 P1/P2/P3/P5/P10/P25/P100 Functional IDs；中奖布局为中奖符号×3+其他六符号各1，Loss 为七符号各1+两个无偏选中符号各再1。 | FROZEN |
| IS-FRZ-246 | Scratch 金融 Round 可先 SETTLED，但下一卡前必须完成轻量 Presentation Gate；`SCRATCH_REVEAL_COMPLETE` 只更新 presentation_completed_at，无资产效果。 | FROZEN |
| IS-FRZ-247 | Scratch Prize Table v1 固定 0/1/2/3/5/10/25/100x 权重与 RTP 96%，Prize Tier 先抽样、布局后生成，不允许九格独立随机决定中奖。 | FROZEN |
| IS-FRZ-248 | Summon v1 固定一个 `SUMMON_MAIN_V1` 功能 Pool，Single=1、Tenfold=10；Tenfold 为一 Round/一总扣款/一总 Settlement，无 Pity/Guarantee/Discount/Rate-up。 | FROZEN |
| IS-FRZ-249 | Summon Prize Table 固定 T0–T5 的 0/1/2/5/20/100x 权重，Validator 必须确认 100000 总权重与 96% RTP。 | FROZEN |
| IS-FRZ-250 | Summon Draw 使用 `summon:draw:{index}` Domain，10 个 Draw 独立确定性 Sampling；Round Result 按全部 Draw 总 Net。 | FROZEN |
| IS-FRZ-251 | Slot v1 五条 32-stop Reel Strip 必须逐项使用冻结顺序，不能根据频数重新生成；五 Reel Stop 独立无偏采样。 | FROZEN |
| IS-FRZ-252 | Slot 固定 10 Payline、LTR≥3、Wild 最高单一解释及 v1 Paytable；Line Stake=Total Wager/10，以 Atomic Units 无损计算。 | FROZEN |
| IS-FRZ-253 | Slot Production Validation 必须穷举 32^5，并精确匹配冻结的 RTP/Hit/Profit/Break-even/Loss/Partial-return 与 516.4× Max Round Payout。 | FROZEN |
| IS-FRZ-254 | Blackjack Canonical Card Encoding 固定 Suit C/D/H/S、Rank A,2..9,T,J,Q,K、card_code=suit×13+rank、card_instance_id=deck×52+card。 | FROZEN |
| IS-FRZ-255 | Blackjack Canonical Six-deck Input 固定 `[0..311]`，使用 `blackjack:shuffle` Deterministic Fisher–Yates；Shoe Hash 为 312 个 U16BE instance IDs 的 SHA-256。 | FROZEN |
| IS-FRZ-256 | Blackjack Initial Deal 固定 Player1→Dealer Up→Player2→Dealer Hole；Hole Card Durable 但合法 Reveal 前 Browser/Ops/Log/Audit 均不可见。 | FROZEN |
| IS-FRZ-257 | Dealer A/10-value Upcard 必须 Player Action 前 Peek；Dealer BJ 时 Player Natural Push、否则原下注 Loss；Natural 仅 Original Unsplit A+10-value，3:2 精确结算。 | FROZEN |
| IS-FRZ-258 | Blackjack Hand 使用稳定 hand_id/index，左到右、单 Active Hand；Hit/Stand 只依次消费锁定 Shoe，不触发新随机。 | FROZEN |
| IS-FRZ-259 | Double 固定 Original Unsplit Two-card 或 Non-Aces Split Two-card，DAS 开启；追加同 Hand Stake、只补一张、追加扣款/发牌/Action 同事务。 | FROZEN |
| IS-FRZ-260 | Split 使用 Blackjack Point Value，因此全部十点牌可互 Split；每 Split/Re-split 追加同 Hand Stake、最多 4 Hands。 | FROZEN |
| IS-FRZ-261 | Split Aces 每 Child 仅补一张后完成，禁止 Hit/Double/Re-split Aces，A+10 只算普通 21。 | FROZEN |
| IS-FRZ-262 | Dealer Engine 固定 S17；Hard/Soft≤16 Hit，Hard≥17与Soft17+ Stand；全部玩家 Bust 时不额外 Draw。 | FROZEN |
| IS-FRZ-263 | Blackjack Action 直接使用 Durable action_id + action_sequence + expected_round_version；Retry 返回原效果，Stale 不得重复发牌/扣款。 | FROZEN |
| IS-FRZ-264 | Blackjack 24h inactivity 初始 Anchor 在第一次进入 PLAYER_TURN 时建立，之后每次成功人工 Action 重置；幂等 Auto-resolve 写 SYSTEM_AUTO_STAND 后按 S17 正常结算。 | FROZEN |
| IS-FRZ-265 | Blackjack Recovery 必须从 Seed 重建同一 312-card Shoe、校验 shoe_hash 与所有 dealt shoe_index；Mismatch 进入 NEEDS_REVIEW，不继续发牌。 | FROZEN |
| IS-FRZ-266 | Blackjack Production RTP/House Edge 不写死，必须由 Ruleset+Shuffle+Reference Strategy+Reproducible Validator 的 VERIFIED Artifact 提供。 | FROZEN |
| IS-FRZ-267 | `000020` 以 additive columns 完善 Dice/Scratch/Summon/Slot/Blackjack Typed Result Authority，不以通用 JSONB 取代正式结果。 | FROZEN |
| IS-FRZ-268 | Create Round 与 Action DTO 均为 Typed Union；Browser 永远不得提交 Random Result、Payout、Next Card、Reel Stop、Prize Tier 或 Config Override。 | FROZEN |
| IS-FRZ-269 | Game-specific Stable Error 必须附可安全恢复的 Round ID/Authoritative State/Next Action；不存在“客户端自己修复 Round”的语义。 | FROZEN |
| IS-FRZ-270 | Direct Play Game Supply Event 按 Round Net 记录：Net>0 ISSUE、Net<0 BURN、Net=0 无 Supply Event；Wager/Payout Ledger 仍分别保存。 | FROZEN |
| IS-FRZ-271 | Game Recovery 使用 Durable `GAME_ROUND_RECOVERY` Job；Blackjack 另用 `BLACKJACK_AUTO_RESOLVE` Job，Redis 不成为 Round/Timer 最终 Authority。 | FROZEN |
| IS-FRZ-272 | Game Operations 只允许 Metadata/Publication/Runtime/Versioned Config/Validation/Fairness/History；不得编辑 Settlement、Payout、Seed、Shoe 或 Historical Config。 | FROZEN |
| IS-FRZ-273 | 每款游戏的 Production Readiness 强制 Active Config↔Implementation↔Ruleset↔Algorithm↔Validation Artifact 一致；Mismatch 自动降级 TEMPORARILY_UNAVAILABLE。 | FROZEN |
| IS-FRZ-274 | Game Audit/Metrics 覆盖 Registry/Config/Round/Recovery/Fairness/Validation 生命周期，未 Reveal Seed、Future Shoe、Dealer Hole Card 永不进入 Audit Payload。 | FROZEN |
| IS-FRZ-275 | IS-06 Common Gate 必须通过 Registry Extensibility、Config Immutability、Round Idempotency、Financial Atomicity、Settlement/Refund Exclusivity、Maintenance Race、Redis Loss。 | FROZEN |
| IS-FRZ-276 | Dice/Scratch Gate 必须通过 exact distribution/RTP、deterministic fairness、Scratch single-triple/loss-layout 与 Reveal no-effect Property Tests。 | FROZEN |
| IS-FRZ-277 | Summon/Slot Gate 必须通过 exact prize tables、indexed Tenfold、无跨 Round State、精确 Reel Strip 与 32^5 Exhaustive Metrics。 | FROZEN |
| IS-FRZ-278 | Blackjack Gate 必须通过 312-card Shoe、Deal/Peek/Natural/Double/DAS/Split/Split Aces/S17/half-chip/Action Idempotency/Restart/24h Auto Resolve。 | FROZEN |
| IS-FRZ-279 | Fairness/Privacy Gate 必须自动证明 Nonterminal Browser/Ops/Audit 不泄露 Server Seed、Future Randomness、Future Shoe 或 Dealer Hole Card。 | FROZEN |
| IS-FRZ-280 | Codex 实现顺序固定先 Registry/Config/Fairness/Common Round，再 Dice→Scratch→Summon→Slot→Blackjack，最后 Validation/BFF/Recovery/Property Tests。 | FROZEN |
| IS-FRZ-281 | Codex 永不得通过随机库、浮点数学、无版本 Config、重新生成“等价”Reel Strip、硬写 Blackjack RTP 或 Presentation Replay 改变金融结果。 | FROZEN |
| IS-FRZ-282 | IS-06 不新增 Product OPEN；五款 Direct Play 产品规则在本批具备完整 Implementation Contract，现有 Reward OPEN/NewAPI SV/Poker Product Gap/Public Record OPEN 全部原样保留。 | FROZEN |

---

# 297. Open / Blocked Register after IS-06

```text
NewAPI:
SV-01 ～ SV-16
= BLOCKED_BY_NEWAPI_SOURCE_VERIFY

Database:
@NEWAPI_USER_ID_PG_TYPE
= BLOCKED_BY_SV-05

@NEWAPI_KEY_ID_PG_TYPE
= BLOCKED_BY_SV-05 / SV-06

Liquidity:
ACTIVE_QUOTA_LOW_WATERMARK
ACTIVE_QUOTA_TARGET_WATERMARK
ACTIVE_QUOTA_MAX_ACTIVE_BUFFER
= UNRESOLVED_IMPLEMENTATION_CONFIG

Reward:
Hourly asset_type/window/accumulation/daily_limit
= OPEN

Relief asset_type/accumulation/active_poker_policy
= OPEN

Reward Product Maintenance / Future Amount Policy / Alert Threshold
= OPEN

Poker:
POKER-PROD-GAP-01 ～ 05
= OPEN

Public Record Selection Policy:
= UNRESOLVED

Deployment:
DEPLOYMENT-VERIFY-01
= PENDING

Generic operational / retry / resource thresholds:
= UNRESOLVED_IMPLEMENTATION_CONFIG
```

IS-06 没有增加新的 Direct Play Product OPEN。

---

# 298. Change Log — WORKING v0.6

## Added

- 用户正式确认 `IS-06 — Game Platform + V1 Direct Play Implementation Specification`；
- 冻结 `IS-FRZ-217 ～ IS-FRZ-282`；
- 冻结 Code-owned Dynamic Game Runtime Manifest；
- 冻结 Direct Play Wager Policy v1；
- 冻结 `000020 / 000021` Game Forward Migrations；
- 冻结 Config Canonical JSON / SHA-256；
- 冻结 Validation Artifact；
- 冻结 Typed Create Round / Resume-first；
- 冻结 Atomic Fast Settlement；
- 冻结 Settlement / Refund / Recovery；
- 冻结 AES-256-GCM Server Seed Protection；
- 冻结 Client Seed / Nonce；
- 冻结 `chaldea-pf-hmac-sha256-v1` Canonical Encoding；
- 冻结 Fairness Golden Vector；
- 冻结 Rejection Sampling / Weighted Choice / Fisher–Yates；
- 冻结 Dice 105/105/6 + RTP；
- 冻结 Scratch Functional ID / Layout Generator / Prize Table；
- 冻结 Summon Pool / Indexed Tenfold / Prize Table；
- 冻结 Slot 五条精确 Reel Strip / Payline / Paytable / 32^5 Validator；
- 冻结 Blackjack Canonical Card Encoding / Six-deck Input / Shoe Hash；
- 冻结 Dealer Peek / Natural / Double / DAS / Split / Split Aces / S17；
- 冻结 Blackjack 24h Inactivity Anchor / Auto Resolve；
- 冻结 Blackjack RTP Validation Artifact；
- 冻结 Direct Play Audit / Metrics / Test / Privacy Gate。

## Preserved

```text
TD-FRZ-001 ～ TD-FRZ-552
IS-FRZ-001 ～ IS-FRZ-282

Reward Product OPEN
Poker Product Gap 01～05
Public Record Selection Policy

SV-01 ～ SV-16 unresolved
DEPLOYMENT-VERIFY-01 pending
Implementation-only Config unresolved values
Production Readiness gates
```

## Supersession

```text
Existing IS-FRZ superseded:
None
```

---

# 299. Next Batch

> **IS-07 — Poker Implementation Specification**

IS-07 将把：

```text
Poker Service Actor
Runtime Epoch
Table / Seat / Session
Buy-in / Top-up / Rebuy / Cash Out

Hand Commit / Forced Bets / Deal
Action Engine
30s Timer
Disconnect / Recovery
Take Over

No-limit Betting
All-in
Main / Side Pot
Showdown
Settlement

Poker Provably Fair
Hole-card Privacy
Delayed Seed Reveal

WebSocket Envelope
Snapshot / Event Version
Chat
Spectator
Host Command

Crash / Concurrency / Property Tests
```

落到 Go Actor / SQL Transaction / WS Envelope / Durable Event / Recovery / Test 粒度。

`POKER-PROD-GAP-01 ～ 05` 继续保持 OPEN，任何依赖这些规则的 Production Ruleset 必须 `CONFIG_INCOMPLETE`，不得由 Codex 或实现人员擅自补齐。

---

# 300. IS-07 — Poker Implementation Specification

> 状态：`FROZEN`  
> 用户确认：`整体按上述 IS-07 方案通过`  
> Frozen Decision Range：`IS-FRZ-283 ～ IS-FRZ-356`  
> Poker Product Gap：`POKER-PROD-GAP-01 ～ 05 = OPEN`  
> Production Ruleset：`CONFIG_INCOMPLETE / NOT PRODUCTION READY`

## 300.1 Purpose

IS-07 将：

```text
Poker Service
Single-writer Table Actor
Runtime Epoch

Table
Seat
Session
Funding

Hand
Action
Timer
Disconnect
Service Recovery

No-limit Betting Engine
Main / Side Pot
Uncalled Excess
Settlement
Refund

Poker Provably Fair
Effective Client Seed
52-card Deck
Delayed Fairness Release

WebSocket
Snapshot / Delta
Take Over

Spectator
Chat
Host Command
History
```

落成：

```text
Go package
Actor mailbox
typed command
SQL transaction
durable state
funding gateway call
deadline worker
WS envelope
viewer projection
fairness byte encoding
recovery procedure
property test
```

本批不会以“标准德州扑克通常如此”为理由补齐五项 Product Gap。

---

# 301. Production Hard Gate

继续保持：

```text
POKER-PROD-GAP-01
Ante Posting Mode

POKER-PROD-GAP-02
Post BB Now live/dead + betting-right semantics

POKER-PROD-GAP-03
Initial Dealer Button

POKER-PROD-GAP-04
Hand Evaluator edge/tie rules

POKER-PROD-GAP-05
Pot Shortcut Raise-To formula
```

因此：

```text
POKER_PRODUCTION_READY = false
```

Production 环境禁止真实付费：

```text
Create Table
Seat Reservation
Buy-in
Start Hand
```

进入可用 Poker Ruleset。

允许实现：

```text
Actor
Persistence
Protocol
Funding Integration
Pot Builder
Timer
Recovery
Fairness
Viewer Projection
Test-only Rulesets
```

只有未来五项 Gap 全部正式冻结，并形成完整：

```text
poker_ruleset_version
hand_evaluator_version
entry_blind_rule_version
bet_shortcut_formula_version
```

后，才能发布 `PRODUCTION_READY` Ruleset。

Test Fixture 必须显式：

```text
TEST_ONLY
```

Production Seed / Build / Startup 不得自动激活。

---

# 302. Poker Go Dependency Lock

本批首次正式使用：

```text
github.com/coder/websocket v1.8.15
golang.org/x/crypto        v0.56.0
```

继续复用：

```text
github.com/jackc/pgx/v5
github.com/redis/go-redis/v9
```

用途：

```text
coder/websocket
→ /ws/poker

x/crypto/argon2
→ Password Table Argon2id
```

不引入 Poker Framework、ORM、Generic Actor Framework 或 Generic Event-sourcing Framework。

---

# 303. Poker Service Package Layout

```text
poker/
├── cmd/
│   └── chaldea-poker/
│
└── internal/
    ├── runtime/
    │   ├── coordinator.go
    │   ├── actor_registry.go
    │   ├── table_actor.go
    │   ├── mailbox.go
    │   └── epoch.go
    │
    ├── table/
    ├── seat/
    ├── session/
    ├── funding/
    │
    ├── hand/
    │   ├── lifecycle/
    │   ├── action/
    │   ├── betting/
    │   ├── dealer/
    │   ├── pot/
    │   ├── settlement/
    │   └── refund/
    │
    ├── fairness/
    │   ├── contribution/
    │   ├── effective_seed/
    │   ├── server_seed/
    │   ├── stream/
    │   ├── deck/
    │   └── release/
    │
    ├── realtime/
    │   ├── websocket/
    │   ├── connection/
    │   ├── projection/
    │   ├── snapshot/
    │   └── eventbuffer/
    │
    ├── timer/
    ├── recovery/
    ├── spectator/
    ├── chat/
    ├── host/
    ├── persistence/
    │   └── postgres/
    └── internalhttp/
```

禁止使用无边界 `poker/utils` / `poker/helpers` 承载跨 Domain 状态机。

---

# 304. Platform ↔ Poker Boundary

普通 Browser：

```text
Browser
→ Platform BFF
→ Typed PokerPort
→ Private Poker Internal HTTP
→ Poker Service
```

Browser 不直接调用 Poker Internal HTTP。

Realtime：

```text
Browser
→ WSS /ws/poker
→ Poker Service
```

Platform BFF：

```text
Chaldea Session
Account Gate
Connect Ticket Mint
HTTP CSRF
```

Poker Service：

```text
Poker Authorization
Table Authority
Session Authority
Funding Execution
Hand Authority
Viewer Projection
```

Platform Backend 不获得 arbitrary `poker.*` write 权限。

---

# 305. Internal Poker Command Contract

`PokerPort` 使用 Code-owned Typed Commands：

```text
CreateTable
VerifyTableAccess
ReserveSeat
BuyIn

RequestTopUp
RequestSafeLeave
TakeOverControl

HostCommand

ReadLobby
ReadTable
ReadSession
ReadFairness
```

Internal HTTP：

```text
private Docker network only
signed Platform service assertion
```

禁止 arbitrary SQL / arbitrary command string / generic state PATCH。

---

# 306. Table Actor / Commit-before-Broadcast

每个 Non-closed Table：

```text
one TableActor
one bounded command mailbox
```

Actor 串行：

```text
Hand Start
Player Action
Timeout
Boundary Top-up
Rebuy
Sit Out
Safe Leave
Host Command
Recovery
Settlement
```

Actor 只负责串行化、Projection 与 Broadcast 协调。

PostgreSQL 是：

```text
Table / Session / Hand / Stack / Commitment / Pot / Settlement Authority
```

每个正式 Command：

```text
receive
→ serialize
→ validate
→ PostgreSQL transaction
→ COMMIT
→ update projection
→ viewer-specific broadcast
```

永久禁止 Broadcast-before-Commit。

---

# 307. Runtime Epoch Fencing

`poker.tables.runtime_epoch BIGINT`。

Actor Claim / Recovery：

```text
BEGIN
SELECT table FOR UPDATE
runtime_epoch++
COMMIT
```

Actor 缓存 `claimed_runtime_epoch`。

所有正式 Write 必须验证：

```text
expected_runtime_epoch == durable runtime_epoch
```

Mismatch：

```text
STALE_RUNTIME_EPOCH
→ actor stops mutation
→ reload/recovery
```

Redis Lock 不作为 Split-brain 最终 Fencing。

---

# 308. Actor Mailbox / Backpressure

Actor Mailbox：

```text
bounded
per table
```

配置：

```text
POKER_ACTOR_MAILBOX_CAPACITY
= UNRESOLVED_IMPLEMENTATION_CONFIG
```

Mailbox Full：

```text
POKER_TABLE_BUSY
```

不得：

```text
drop player command
apply outside actor
```

Player 使用原 `action_id` 安全重试。

---

# 309. Poker Forward Migrations

新增：

```text
000022__poker_runtime_refinements
000023__poker_v1_presets_and_incomplete_ruleset
```

不修改此前 Migration。

---

# 310. Blind Preset / Incomplete Ruleset

新增：

```text
poker.blind_preset_versions
```

至少：

```text
blind_preset_version_id
version_name

small_blind_units
big_blind_units

ante_variant
ante_amount_units

minimum_buyin_bb
maximum_buyin_bb

status
created_at
activated_at
retired_at
```

固定：

```text
minimum_buyin_bb = 40
maximum_buyin_bb = 100
```

V1 SB / BB：

```text
5 / 10
10 / 20
25 / 50
50 / 100
100 / 200
500 / 1000
```

每档可有：

```text
NO_ANTE
ANTE_10_PERCENT_BB
```

但 Ante 版本：

```text
ante_posting_mode = NULL
status = CONFIG_INCOMPLETE
```

Seed：

```text
poker-rules-v1-incomplete
```

五个 Gap 字段全部 NULL / UNRESOLVED。

已知：

```text
NO_LIMIT_HOLDEM
52 cards
2 hole cards
5 board max
Rake = 0
action timer = 30s
intermission = 5s
rebuy window = 60s
auto-safe-leave = 15m
fairness release = 24h
```

但不因此 Active。

---

# 311. Table Physical Contract

`poker.tables` 至少：

```text
table_id
owner_newapi_user_id

table_name
access_mode

max_seats
blind_preset_version_id
poker_ruleset_version_id
game_config_version_id
config_hash

allow_spectators
chat_enabled

settings_locked_at

lifecycle_state
accepting_players
allow_new_hands

table_version
runtime_epoch

empty_since
created_at
closing_at
closed_at
```

Lifecycle：

```text
CREATED
WAITING
IN_HAND
INTERMISSION
PAUSED
RECOVERING
CLOSING
CLOSED
```

---

# 312. Table Create

```http
POST /api/v1/poker/tables
```

Required：

```http
Idempotency-Key
X-CSRF-Token
```

Body：

```text
table_name

access_mode:
  PUBLIC
  PASSWORD

password nullable

max_seats 2..9

blind_preset_version_id

allow_spectators
chat_enabled
```

禁止：

```text
custom SB/BB/Ante
custom buy-in range
UNLISTED
INVITE_ONLY
```

Create Table 不创建 Seat / Session / Wallet Debit。

资格：

```text
Authenticated
Master Complete
Account Active

no Active Poker Session
no Non-closed Owned Table

Production Rule Gate
Maintenance
```

当前 Production 返回：

```text
POKER_RULESET_INCOMPLETE
```

---

# 313. Password Table / Access Grant

Password Table 保存：

```text
Argon2id PHC-style encoded hash only
```

生成：

```text
salt = 16 CSPRNG bytes
derived key = 32 bytes
```

配置：

```text
POKER_TABLE_PASSWORD_ARGON2_MEMORY_KIB
POKER_TABLE_PASSWORD_ARGON2_TIME
POKER_TABLE_PASSWORD_ARGON2_PARALLELISM
= UNRESOLVED_IMPLEMENTATION_CONFIG
```

明文 Password：

```text
request memory only
never log/audit/redis
```

验证成功：

```text
Redis:
chaldea:poker:table-access:
<session_id_hash>:
<table_id>
```

TTL：

```text
remaining Chaldea Session TTL
```

Session Rotation 自动失效。

Redis 丢失最多要求重新输入密码，不影响 Poker Durable Facts。

---

# 314. Seat Reservation

IS-03 已冻结 `poker.seat_reservations` Durable Table。

其职责固定为：

```text
request / idempotency / audit fact
```

不是 30s Seat Lock Authority。

Durable State：

```text
REQUESTED
LEASE_ACTIVE
CONSUMED
EXPIRED
FAILED_NO_EFFECT
```

真正 Lease：

```text
Redis SET NX

chaldea:poker:seat-reservation:
<table_id>:
<seat_no>
```

TTL：

```text
30s
```

Redis Lease 丢失/过期：

```text
cannot complete buy-in
no wallet debit
reserve again
```

Durable Reservation 不得永久锁 Seat。

---

# 315. Seat / Session

Seat State：

```text
WAITING_ENTRY
ACTIVE
WAITING_BIG_BLIND
POST_BIG_BLIND_NEXT_HAND
SIT_OUT
LEAVE_AFTER_HAND
REMOVE_AFTER_HAND
REBUY_WINDOW
LEAVING
LEFT
```

Connection：

```text
CONNECTED
DISCONNECTED
```

Session：

```text
session_id
newapi_user_id
table_id
seat_no
identity_display_snapshot_id

state

initial_buyin_units
total_topup_units
current_stack_units

final_cashout_units
realized_pl_units

control_epoch

started_at
ended_at
end_reason
```

Session State：

```text
ACTIVE
SETTLING
SETTLED
NEEDS_REVIEW
```

DB 强制：

```text
one Active Poker Session per user
```

---

# 316. Integer Chip / Funding IDs

Poker 所有：

```text
Blind
Ante
Entry Blind
Buy-in
Top-up
Rebuy
Bet
Call
Raise
Pot
Side Pot
Stack
Cash Out
```

要求：

```text
units % 500000 = 0
```

Wallet 小数筹码不进入 Poker。

Stable Funding Biz：

```text
Buy-in:
poker_buyin:{session_id}

Cash Out:
poker_cashout:{session_id}

Top-up:
poker_topup:{funding_operation_id}

Rebuy:
poker_rebuy:{funding_operation_id}
```

叠加 IS-05 Durable Idempotency。

---

# 317. Funding Lock / Gateway

Lock Order：

```text
Poker Domain Authority
→ Available Chips Wallet
→ Ledger / Funding effects
```

Poker Runtime 不获得 Economy 普通 DML。

只允许执行：

```text
economy.poker_buy_in_apply(...)
economy.poker_top_up_apply(...)
economy.poker_cash_out_apply(...)
```

---

# 318. Buy-in

范围：

```text
40BB <= Buy-in <= 100BB
```

Suggested：

```text
if wallet >=100BB:
  100BB
else if wallet >=40BB:
  max affordable whole-chip <=100BB
else:
  unavailable
```

Submit 时 Server 重验。

事务：

```text
BEGIN

lock Table
lock Seat
check Active Session uniqueness
lock Available Chips Wallet

validate reservation lease
validate seat empty
validate range/whole-chip/wallet

execute narrow funding

create Session
occupy Seat
set Stack

if first successful Buy-in:
  settings_locked_at = now
  freeze running settings

COMMIT
```

---

# 319. First Buy-in Settings Lock

第一次成功 Buy-in 后锁定：

```text
Max Seats
Blind / Ante Preset
Buy-in Bounds
Spectator Policy
Economic Ruleset / Config
```

Host 后续不可修改。

本批不新增未经冻结的动态设置修改 API。

---

# 320. Running-table Entry / Intermission

尚未开始第一手：

```text
normal seat
no extra entry BB
```

运行中：

```text
WAIT_FOR_BIG_BLIND
or
POST_BIG_BLIND_NOW
```

等待期间不发 Hole Cards。

`POST_BIG_BLIND_NOW` 仅已知：

```text
post one entering BB
at next eligible Hand
```

Live/Dead、完整 Pot/Betting Rights 仍 Gap-02。

Intermission：

```text
5s
```

Boundary 顺序：

```text
1 Finish previous Settlement
2 Safe Leave / Remove After Hand
3 Sit Out
4 Pending Top-up / Rebuy
5 Activate newly bought-in Seats
6 Resolve waiting Blind states
7 Select next participants
8 Prepare next Hand
```

---

# 321. Top-up / Rebuy

Hand 中 Top-up：

```text
PENDING
```

不改变当前 Hand Stack。

Boundary：

```text
lock Session
lock Seat
lock Wallet

revalidate Wallet
revalidate Stack
revalidate resulting Stack <=100BB

atomic Wallet Debit + Stack Credit
```

失败：

```text
FAILED_NO_EFFECT
```

通过牌局赢到 >100BB 不强制降 Stack。

Stack=0：

```text
Seat = REBUY_WINDOW
deadline = DB now + 60s
```

成功 Rebuy：

```text
same Session
```

超时：

```text
Cash Out 0
Seat LEFT
Session SETTLED
BUST_NO_REBUY
```

无 Auto Rebuy。

---

# 322. Safe Leave / Cash Out

Safe Leave 统一：

```text
Return Lobby
Leave Table
Browser Back Intent
Host Remove
Auto Leave
Table Close
```

未参与 Hand：

```text
immediate Cash Out
```

当前 Hand Participant：

```text
LEAVE_AFTER_HAND
→ finish Settlement
→ Cash Out
```

Fold 后也等 Settlement。

Cash Out Biz：

```text
poker_cashout:{session_id}
```

Transaction：

```text
BEGIN

lock Session
lock Seat
lock Available Chips Wallet

assert no unsettled owned Hand
assert not cashed out

final_stack = current_stack

execute poker_cash_out_apply

current_stack = 0
final_cashout = final_stack

realized_pl =
final_cashout
-
all successful Buy-in / Top-up / Rebuy

Session = SETTLED
Seat = LEFT

COMMIT
```

无 Partial Cash Out；Duplicate 返回原结果。

---

# 323. Empty Table Auto Close

只有：

```text
no seated players
no spectators
no unsettled Hand
no pending funding
no Poker In Play
```

持续：

```text
30m
```

才允许关闭。

Durable Job：

```text
POKER_EMPTY_TABLE_CLOSE
dedupe = poker:empty-close:{table_id}
```

执行前重验；条件失效则 no-op/reschedule。

---

# 324. Two-phase Hand Start

## Phase A — Commitment

```text
BEGIN

lock Table

assert production-compatible ruleset
assert >=2 eligible participants

freeze:
  participant set
  starting stacks
  rules/config versions
  blind preset
  client seed contributions

resolve next Dealer Button

generate Server Seed
persist Server Seed Hash

derive Effective Client Seed
generate complete deterministic Deck
persist Deck Hash

create Hand
state = COMMITTED

COMMIT
```

然后 Broadcast `HAND_COMMITTED`。

## Phase B — Forced Bets / Deal

```text
BEGIN

lock Hand

verify runtime_epoch
verify COMMITTED

post Ante/SB/BB/Entry BB as allowed
record System Actions

deal Hole Cards from locked Deck

Hand → PREFLOP
set first actor
set 30s deadline

COMMIT
```

Seed Hash 必须在任何 Hole Card 前 Durable。

---

# 325. Hand Participant Physical Authority

Technical Design 的 logical `hand_players` 映射到 IS-03 已冻结：

```text
poker.hand_participants
```

不得创建第二份 Authority。

至少：

```text
hand_id
session_id
seat_no

starting_stack_units
ending_stack_units nullable

street_committed_units
total_committed_units

is_folded
is_all_in
is_showdown_eligible

hole_card_index_1
hole_card_index_2

timeout_streak_at_hand_start
last_acted_full_raise_sequence

client_seed_contribution
client_seed_contribution_version
```

Participant Set 在 COMMITTED 后 Immutable。

---

# 326. Hand Lifecycle / Dealer Order

Lifecycle：

```text
COMMITTED
POSTING_FORCED_BETS
DEALING_HOLE_CARDS
PREFLOP
FLOP
TURN
RIVER
SHOWDOWN
SETTLING
SETTLED

ALL_IN_RUNOUT

PAUSED
RECOVERING

REFUNDING
REFUNDED
```

后续 Dealer Button：

```text
clockwise
```

3+：

```text
Button left = SB
SB left = BB

Preflop:
left of BB first eligible actor

Postflop:
left of Button first live actor
```

Heads-up：

```text
Button = SB
Other = BB

Preflop:
Button/SB first

Postflop:
BB first
```

第一手 Initial Button 继续 Gap-03；实现不得自行选择。

---

# 327. Player Action / Version

Player Types：

```text
FOLD
CHECK
CALL
BET
RAISE
ALL_IN
```

Command：

```text
action_id
hand_id
expected_hand_version
control_epoch
action_type
requested_to_units nullable
```

Server Authority：

```text
legal_actions
to_call
minimum_bet
minimum_raise_to
maximum_raise_to
current_pot
current_stack
raise_rights
shortcut_targets
```

Action Transaction：

```text
BEGIN

lock Hand
lock Hand Participant
lock Session

verify:
  runtime_epoch
  control_epoch
  duplicate action
  expected hand version
  current actor
  deadline
  legal action
  legal amount

move Table Stack → Hand Commitment

insert Action

update:
  commitments
  current bet
  raise tracking
  pot projection
  next actor / street
  timer
  hand version

COMMIT
```

不触碰主 Wallet。

---

# 328. Full Raise / Shortcut Gap

Hand：

```text
current_bet_to_units
last_full_raise_increment_units
full_raise_sequence
```

Participant：

```text
last_acted_full_raise_sequence
```

Full Raise：

```text
full_raise_sequence++
last_full_raise_increment = actual increment
```

Short All-in：

```text
does not increment full_raise_sequence
```

Server 可以呈现：

```text
Min
1/2 Pot
2/3 Pot
Pot
All-in
```

已知 Fraction 需 floor 且不得低于合法 Minimum。

但 Raise Target-To 公式仍 Gap-05：

```text
bet_shortcut_formula_version = NULL
```

Frontend 不得自行计算 Production Target。

---

# 329. Action Timer / Timeout

每个玩家决策：

```text
30s
```

无 Time Bank。

PG：

```text
action_started_at
action_deadline_at
current_actor_seat
action_sequence
```

Timer Scheduler 只唤醒；PG Deadline 是 Authority。

Timeout ID：

```text
timeout:{hand_id}:{action_sequence}
```

执行：

```text
BEGIN
lock Hand

if sequence advanced → no-op
if DB now < deadline → no-op

if Check legal:
  AUTO_CHECK
else:
  AUTO_FOLD

COMMIT
```

Human/Timeout Race 由 DB Order 决定，最多一个生效。

连续两个 Timeout：

```text
sit_out_next_hand = true
reason = CONSECUTIVE_TIMEOUT
```

成功人工 Action 重置 streak。

---

# 330. Disconnect / Service Recovery

Client Disconnect：

```text
persist disconnected_since
connection_state = DISCONNECTED

Timer continues
Hand continues
```

Boundary 仍断线：

```text
Sit Out Next Hand
```

Boundary 前 Reconnect：

```text
clear disconnect-only next-hand sitout
```

不清除 Timeout-induced Sit Out。

Service Failure：

```text
Table RECOVERING
old deadline not treated as user fault
```

恢复：

```text
PG reconstruct
→ 30s Reconnect Grace
→ if player decision still required:
   fresh full 30s action window
→ Resume
```

---

# 331. Recovery State / Auto Safe Leave

`poker.recovery_state`：

```text
table_id
hand_id nullable

previous_table_state
recovery_state

detected_at
recovery_started_at

reconnect_grace_until
resumed_at

runtime_epoch
reason_code
```

State：

```text
NORMAL
RECOVERING
GRACE
RESUMED
NEEDS_REVIEW
```

Redis 不承载唯一 Recovery Truth。

Sit Out / Disconnect 连续：

```text
>=15m
AND no unsettled Hand
```

触发 Durable：

```text
POKER_AUTO_SAFE_LEAVE
→ Cash Out
→ Session Settlement
```

---

# 332. Pot Authority / Side Pot

Authority：

```text
street_committed_units
total_committed_units
```

`pot_total` 只可为 Projection。

取所有 Commitment >0 的 Unique Levels：

```text
L1 < L2 < ...
```

每层：

```text
amount =
(level - previous_level)
×
count(commitment >= level)
```

Eligible：

```text
commitment >= level
AND not folded
```

Fold Contribution：

```text
included in Pot
excluded from winner eligibility
```

形成 Main / Side Pots。

---

# 333. Uncalled Excess / Evaluator Boundary

最高层若没有第二位玩家匹配：

```text
RETURN_UNCALLED
```

原子退回原 Table Stack。

它不：

```text
become Pot
become Award
become ownership transfer
count as settled wager
```

Evaluator Interface：

```text
Evaluate(
  hole_cards,
  board_cards,
  evaluator_version
)
```

输出：

```text
category
primary_rank_vector
best_five_card_indices
hand_evaluator_version
```

Wheel / Suit Tie / Kicker / Complete Tie-break 仍 Gap-04。

因此 Production Evaluator Blocked。

允许 Test Fixture Evaluator，但不得进入 Production Ruleset。

---

# 334. Early Winner / All-in / Showdown

Early Winner：

```text
one non-folded player
→ settle eligible pots
→ no unnecessary Board
```

不强制公开未公开 Hole Cards。

All-in 且无剩余决策：

```text
ALL_IN_RUNOUT
→ remaining Board
→ SHOWDOWN
```

Ordinary Showdown：

```text
reveal only cards required by finalized rules
```

Folded / Mucked Hole Cards 实时保持隐藏。

---

# 335. Hand Settlement / Odd Chip / Zero-sum

Biz：

```text
poker_hand_settlement:{hand_id}
```

Settlement：

```text
BEGIN

lock Hand
assert not SETTLED
assert not REFUNDED

verify Action Sequence

RETURN_UNCALLED

build deterministic Pots
evaluate eligible Hands
calculate shares
assign Odd Chips

increase winner Table Stacks

persist Pot / Eligible / Awards / Settlement
close commitments

Hand = SETTLED

COMMIT
```

不触碰 Wallet。

Odd Chip：

```text
base_share = floor(pot / winner_count)
```

余数从 Dealer Button 左侧顺时针，在该 Pot Eligible Tie Winners 中逐枚分配。

Zero-sum：

```text
Total Commitments
=
Pot Awards
+
Returned Uncalled Excess

Rake = 0
```

Poker Hand 不 Issue/Burn。

---

# 336. Catastrophic Refund

仅：

```text
legal deterministic recovery impossible
```

时允许。

目标：

```text
each participant
→ hand_start_stack_units
```

Refunded Hand：

```text
No Winner
No Game Profit
No Biggest Win
No Wagered Ranking Input
```

如果无法证明全部单位来源：

```text
NEEDS_REVIEW
```

不得猜 Stack。

---

# 337. Poker Client Seed Contribution

每 Active Seat：

```text
next_client_seed_contribution
contribution_version
```

默认：

```text
pcs1-
+
64 lowercase hex
```

Hex 来自 32-byte CSPRNG。

用户自定义：

```text
1–128 UTF-8 bytes
```

拒绝 invalid UTF-8 / NUL / CR / LF / controls。

不进行 Unicode Normalization。

Phase A COMMITTED 时冻结当前 Contribution / Version，之后修改只影响下一 Hand。

---

# 338. Effective Client Seed Encoding v1

固定：

```text
chaldea-poker-effective-client-seed-v1
```

Ordering：

```text
seat_no ascending
```

Canonical：

```text
ASCII("CHALDEA-POKER-EFFECTIVE-CLIENT-SEED-V1")
0x00

UUID16(table_id)
UUID16(hand_id)

U16BE(participant_count)

for each participant:
  U16BE(seat_no)
  U64BE(contribution_version)
  LP16(contribution)
```

Effective Seed：

```text
SHA-256(canonical bytes)
```

Golden Input：

```text
table_id =
018f47a2-6e9d-7c31-8a4b-123456789abc

hand_id =
019047a2-6e9d-7c31-8a4b-abcdef012345

seat 2:
  version 3
  contribution alpha

seat 7:
  version 5
  contribution beta
```

Golden Hash：

```text
1d820ef6e80ca45951e3b9f6fb6a3cce
b31a583445e1ba7f290012199d5643db
```

---

# 339. Poker Server Seed / Deck Stream

每 Hand Server Seed：

```text
32 CSPRNG bytes
```

Hash：

```text
SHA-256(server_seed)
```

未公开阶段使用：

```text
AES-256-GCM
poker_fairness_keyring
```

与 Direct Play Keyring 分离。

Deck Stream：

```text
chaldea-poker-hmac-sha256-v1
```

Message：

```text
ASCII("CHALDEA-POKER-DECK-HMAC-SHA256-V1")
0x00

UUID16(table_id)
UUID16(hand_id)

effective_client_seed[32]

LP16(algorithm_version)

U64BE(block_index)
```

HMAC-SHA-256 连续 Block 形成随机流。

Sampling 使用 U32BE + Rejection Sampling。

---

# 340. Card / Deck / Deal Sequence

Card Code 复用 IS-06：

```text
Suit:
0 C
1 D
2 H
3 S

Rank:
0 A
1 2
...
9 T
10 J
11 Q
12 K

card_code =
suit*13 + rank
```

仅作为 Card Identity，不定义 Suit Tie。

Canonical Deck：

```text
[0..51]
```

使用无偏 Fisher–Yates。

Golden：

```text
server_seed =
000102...1f

effective_client_seed =
1d820ef6...643db
```

HMAC Block 0：

```text
e45a7cb6998add6bf7305dc92853eca6
630cc4cd05834efe45285e2f6bd415ce
```

Deck 前 15：

```text
46,25,19,38,44,
28,6,16,22,8,
11,1,47,23,35
```

Deck Hash：

```text
b04f7b14688a3a1dfe9f42bfb64cc7ed
b708bd88e2d710e2798a878ff94bc70d
```

Deal Sequence Version：

```text
poker-deal-v1
```

V1 数字桌不模拟物理 Burn Card。

Hole Cards：

```text
participants seat_no ascending

pass 1:
one card each

pass 2:
one card each
```

Board：

```text
FLOP next 3
TURN next 1
RIVER next 1
```

每次消费记录 deck_index/card/recipient/visibility。

改变需新 Deal Sequence Version。

---

# 341. Fairness Authority / Release

`poker.hand_fairness` 至少：

```text
server_seed_hash
encrypted_server_seed
server_seed_key_version

effective_client_seed_hash

deck_version
deal_sequence_version
deck_hash
next_deck_index

full_fairness_reveal_at
release_state
```

运行中不得重新随机下一张牌。

Hand SETTLED 立即公共：

```text
Server Seed Hash
Hand ID
Algorithm Version
Config Version / Hash
Public Board
Public Actions
Public Settlement
```

完整：

```text
Server Seed
Effective Client Seed composition
Complete Deck
Verification Data
```

只有：

```text
settled_at + 24h
```

且请求者属于 Durable Hand Participant Set 才能读取。

Spectator/Public 永不获得完整 Seed / Deck。

Durable Job：

```text
POKER_FAIRNESS_RELEASE
dedupe = poker:fairness-release:{hand_id}
```

只改变 Release Availability，不修改历史。

---

# 342. Viewer-specific Projection

正式：

```text
Authoritative State
→ Player Projection
→ Spectator Projection
→ Host Projection
→ Operations Projection
```

Player：

```text
own Hole Cards
+
public state
```

Other Player / Spectator / Host / Operations：

```text
no unauthorized private Hole Cards
```

所有 Viewer 在合法 Reveal 前都不能获得 Future Deck / Unreleased Seed。

永久禁止先发送全部 Secret 再由 UI 隐藏。

---

# 343. WebSocket Endpoint / Auth

```text
WSS /ws/poker
Subprotocol = chaldea-poker.v1
```

Upgrade 验证：

```text
Origin
Subprotocol
```

初始：

```text
AUTH_PENDING
```

固定：

```text
POKER_WS_AUTH_DEADLINE = 10s
```

10s 内必须第一条业务 Frame：

```text
auth.connect
```

否则关闭：

```text
AUTH_TIMEOUT
```

Connect Ticket 继续使用 IS-04：

```text
ct1
Ed25519
60s
single-use
session/security_epoch bound
restart fencing
```

Ticket 不进 URL / Subprotocol / Log。

---

# 344. WS Envelope / Message Families

Client：

```json
{
  "type": "hand.action",
  "request_id": "01...",
  "table_id": "...",
  "hand_id": "...",
  "expected_table_version": 12,
  "expected_hand_version": 57,
  "control_epoch": 3,
  "action_id": "...",
  "payload": {}
}
```

Server：

```json
{
  "type": "hand.action_applied",
  "event_id": "...",
  "event_seq": 105,
  "table_id": "...",
  "table_version": 13,
  "hand_id": "...",
  "hand_version": 58,
  "server_time": "...",
  "payload": {}
}
```

Asset Payload 继续 String。

Client Families：

```text
auth.connect
sync.request
hand.action
session.sit_out_next_hand
session.resume_play
client_seed.set_next
chat.send
ping
```

Server Families：

```text
auth.accepted
table.snapshot
table.state_changed
seat.state_changed
session.state_changed
hand.started
hand.action_applied
hand.street_changed
hand.cards_dealt
hand.pots_updated
hand.settled
hand.paused
hand.recovered
timer.started
control.changed
chat.message
service.notice
error
pong
```

Host/Ops Durable Command 走 HTTP。

---

# 345. WS Delivery / Snapshot / Backpressure

WS Frame：

```text
may duplicate
may be lost
```

DB Effect：

```text
exactly-once / idempotent
```

`event_seq`：

```text
monotonic inside current Table Actor runtime epoch
```

Actor Restart 后从新 Snapshot Baseline 开始，不要求 Event Seq 跨 Epoch 持久化。

Durable Concurrency Authority：

```text
table_version
hand_version
runtime_epoch
control_epoch
```

Client：

```text
duplicate event_seq
→ ignore

gap
→ sync.request
```

Redis Event Buffer 只能保存 Viewer-neutral / safe public descriptor。

不能安全生成 Delta：

```text
full viewer-specific snapshot
```

Reconnect：

```text
auth
→ authoritative snapshot
→ optional safe delta
→ live
```

Send Queue / Message Limit：

```text
POKER_WS_SEND_QUEUE_CAPACITY
POKER_WS_MAX_MESSAGE_BYTES
= UNRESOLVED_IMPLEMENTATION_CONFIG
```

Slow Client：

```text
disconnect
→ reconnect
→ snapshot
```

不得拖住 Actor。

---

# 346. Control / Take Over

Active Poker Session：

```text
one Active Control Connection
```

Redis Connection Lease 是 Ephemeral。

真正 Action Authority：

```text
poker.sessions.control_epoch
```

第二 Connection 可以 Read-only。

Take Over：

```http
POST /api/v1/poker/sessions/{session_id}/take-over
```

Transaction：

```text
BEGIN
lock Session
control_epoch++
Audit
COMMIT
```

旧 Controller：

```text
epoch mismatch
→ action rejected
→ READ_ONLY
```

Restart 后第一个合法 Reconnect Claim Lease；同时两个设备 first wins，另一个需显式 Take Over。

---

# 347. Spectator / Chat

Spectator：

```text
real-time
no artificial delay

no Poker Session
no Stack
no Control
```

可见 Public Master / Seats / Public Stack / Button / Blind / Ante / Actions / Board / Pots / Showdown / Chat。

不可见 Private Hole Cards / Folded Cards / raw Client Seed / Future Deck / Full Seed / Private Wallet。

已有 Active Session：

```text
cannot spectate another Table
```

Chat：

```text
USER_TEXT
SYSTEM
```

不支持 image/file/voice/DM/global chat。

Message Durable。

新增：

```text
poker.chat_mutes
```

`POKER_CHAT_MAX_BYTES = UNRESOLVED_IMPLEMENTATION_CONFIG`。

Mute / Hide 不删除原消息；Local Block 是前端偏好。

---

# 348. Host / Close / Maintenance

Host HTTP Typed Command：

```text
PAUSE_ACCEPTING_PLAYERS
RESUME_ACCEPTING_PLAYERS
REMOVE_PLAYER_AFTER_HAND
REMOVE_SPECTATOR
MUTE_CHAT_USER
CLOSE_TABLE
```

Host 永不得：

```text
view Hole Cards
view unrevealed Seed

change Deck
Pot
Stack
Winner
Settlement

force current-Hand Cash Out

change locked Blind/Buy-in economic config
```

Close：

```text
CLOSING
accepting_players=false
allow_new_hands=false
```

Active Hand 正常 finish/recover。

之后所有 Seats Safe Leave / Cash Out。

只有无 Poker In Play / pending funding / unsettled Hand 才 CLOSED。

Owner 离桌不自动转移 Table Ownership。

Maintenance：

```text
POKER_NEW_TABLES_NEW_HANDS
```

阻断：

```text
new Table
new Seat
new Buy-in
new Hand
```

继续 Active Hand / Recovery / Safe Leave / Cash Out / Accepted Funding Recovery。

---

# 349. Ranking Commit Boundary

Hand 可以 Durable 保存：

```text
hand_net_change
hand_total_wagered
eligible biggest-win candidate
```

但正式 Poker Rankings Source 只在 Parent Session Cash Out 后发布：

```text
Session Realized P/L
Eligible Biggest Win
Total Wagered
```

Refunded Hand 排除。

---

# 350. Poker HTTP BFF

固定：

```http
GET  /api/v1/poker

GET  /api/v1/poker/tables
POST /api/v1/poker/tables
GET  /api/v1/poker/tables/{table_id}

POST /api/v1/poker/tables/{table_id}/access
POST /api/v1/poker/tables/{table_id}/seat-reservations
POST /api/v1/poker/tables/{table_id}/buy-ins

POST /api/v1/poker/tables/{table_id}/commands

GET  /api/v1/poker/sessions/active
GET  /api/v1/poker/sessions/{session_id}

POST /api/v1/poker/sessions/{session_id}/top-ups
POST /api/v1/poker/sessions/{session_id}/safe-leave
POST /api/v1/poker/sessions/{session_id}/take-over

POST /api/v1/poker/connect-tickets

GET  /api/v1/poker/hands/{hand_id}/fairness
```

Lobby Projection：

```text
service state
production readiness

Active Session / Reconnect

Available Chips
Poker In Play

ruleset readiness
table summary
```

当前 Production：

```text
production_ready = false
blocker = POKER_RULESET_INCOMPLETE
```

---

# 351. Error / Implementation Config

Errors 至少：

```text
POKER_RULESET_INCOMPLETE
POKER_SERVICE_UNAVAILABLE
POKER_TABLE_NOT_AVAILABLE
POKER_TABLE_BUSY

STALE_RUNTIME_EPOCH
STALE_TABLE_VERSION
STALE_HAND_VERSION

ACTIVE_SESSION_EXISTS
ACTIVE_CONTROLLER_EXISTS
CONTROL_EPOCH_MISMATCH

TABLE_ACCESS_REQUIRED
TABLE_PASSWORD_INVALID

SEAT_NOT_AVAILABLE
SEAT_RESERVATION_EXPIRED

INVALID_BUYIN
INSUFFICIENT_CHIPS

ACTION_NOT_ALLOWED
ACTION_DEADLINE_PASSED

HAND_RECOVERING
HAND_NEEDS_REVIEW

POKER_FAIRNESS_NOT_YET_AVAILABLE
POKER_FAIRNESS_NOT_AUTHORIZED
```

有 Hand 时可附 `table_id / hand_id / safe state / next action`。

Implementation Config 新增：

```text
POKER_ACTOR_MAILBOX_CAPACITY

POKER_WS_SEND_QUEUE_CAPACITY
POKER_WS_MAX_MESSAGE_BYTES

POKER_CHAT_MAX_BYTES

POKER_TABLE_NAME_MAX_GRAPHEMES
POKER_TABLE_PASSWORD_MAX_BYTES

POKER_TABLE_PASSWORD_ARGON2_MEMORY_KIB
POKER_TABLE_PASSWORD_ARGON2_TIME
POKER_TABLE_PASSWORD_ARGON2_PARALLELISM
```

全部 `UNRESOLVED_IMPLEMENTATION_CONFIG`。

固定技术值：

```text
POKER_WS_AUTH_DEADLINE = 10s
SEAT_RESERVATION_TTL = 30s
ACTION_TIMER = 30s

SERVICE_RECOVERY_GRACE = 30s
SERVICE_RECOVERED_ACTION_WINDOW = 30s

HAND_INTERMISSION = 5s
REBUY_WINDOW = 60s
AUTO_SAFE_LEAVE = 15m

FAIRNESS_FULL_RELEASE_DELAY = 24h
```

---

# 352. Audit / Metrics

Audit 至少：

```text
POKER_TABLE_CREATED
POKER_TABLE_ACCESS_VERIFIED
POKER_TABLE_CLOSING
POKER_TABLE_CLOSED

POKER_BUYIN
POKER_TOPUP
POKER_REBUY
POKER_CASHOUT

POKER_SESSION_STARTED
POKER_SESSION_SETTLED

POKER_HAND_COMMITTED
POKER_HAND_STARTED
POKER_ACTION_APPLIED
POKER_TIMEOUT_ACTION

POKER_HAND_SETTLED
POKER_HAND_REFUNDED

POKER_RECOVERY_STARTED
POKER_RECOVERY_COMPLETED
POKER_NEEDS_REVIEW

POKER_CONTROL_TAKEN_OVER

POKER_CHAT_MUTED
POKER_CHAT_HIDDEN

POKER_FAIRNESS_RELEASED
```

Audit 不记录 Table Password / unrevealed Hole Cards / unrevealed Seed / Future Deck。

Metrics 至少：

```text
poker_tables_by_state
poker_active_sessions
poker_active_hands

poker_ws_connections
poker_reconnect_count
poker_takeover_count

poker_action_latency
poker_action_timeout_count
poker_auto_check_count
poker_auto_fold_count

poker_actor_mailbox_depth
poker_actor_mailbox_rejection

poker_hand_duration
poker_hand_recovery_count
oldest_recovering_hand_age

poker_side_pot_count
poker_settlement_failure

poker_buyin_failure
poker_topup_failure
poker_cashout_failure

poker_fairness_release_lag
poker_seed_verification_failure

poker_asset_conservation_failure
poker_viewer_projection_privacy_failure
```

Threshold 保持 Implementation Config。

---

# 353. IS-07 Test Gate

## Actor / Funding

```text
100 concurrent commands same Table
→ deterministic serialization

two actors same Table
→ only current runtime_epoch writes

commit then broadcast loss
→ state preserved

Redis loss
→ Table/Session/Hand/Stack preserved

100 duplicate Buy-ins
→ one Session/debit/Stack

duplicate Top-up/Rebuy/Cash Out
→ one effect
```

## Timer / Recovery

```text
User Action vs Timeout
→ one action

Check legal → AUTO_CHECK
otherwise → AUTO_FOLD

2 consecutive timeout
→ next Hand Sit Out

manual action → streak reset

Client disconnect
→ timer continues

Service failure
→ no timeout during outage
→ PG Recovery
→ 30s Grace
→ fresh 30s action
```

## Pot / Settlement

```text
commitment sum
=
pot sum
+
uncalled return

fold contribution included
fold eligibility excluded

odd chip deterministic

100 duplicate Settlement
→ one effect

REFUNDED
→ cannot SETTLE

Poker asset conservation
Rake=0
```

## Fairness / Privacy

```text
Effective Seed Golden Hash exact

same fairness inputs
→ same 52-card Deck

52 unique cards

Golden HMAC
Golden first 15 cards
Golden Deck Hash

no duplicate deck index

<24h participant
→ no full seed

>=24h durable participant
→ full fairness allowed

spectator
→ never full seed

Viewer projections
→ no unauthorized Hole Card / Seed / Deck
```

## WS / Control

```text
wrong Origin/Subprotocol reject
auth.connect >10s reject
ticket replay reject
pre-restart ticket reject

duplicate event ignore
gap → sync
buffer missing → snapshot
slow client → disconnect/reconnect

Take Over
→ control_epoch++
old device rejected
new device accepted
```

## Product Gap Protection

Production Startup/CI 必须 assert：

```text
ante_posting_mode != NULL
post_bb_now_semantics != NULL
initial_dealer_button_rule != NULL
hand_evaluator_version != NULL
raise_shortcut_formula_ver != NULL
```

且 Canonical Evaluator / Ruleset Artifact VERIFIED。

缺失任意一项：

```text
POKER_PRODUCTION_READY=false
```

不存在：

```text
--ignore-poker-gaps
--assume-standard-rules
--force-enable-poker
```

---

# 354. Codex IS-07 Implementation Order

```text
01 lock websocket / x-crypto

02 migration 000022
03 migration 000023

04 ruleset / preset models
05 production readiness gate

06 Table Repository
07 Session Repository
08 Seat Repository
09 Funding Repository

10 Table Actor
11 Actor Registry
12 Runtime Epoch
13 Actor Mailbox

14 Table Create
15 Password Access
16 Seat Reservation

17 Buy-in
18 Top-up
19 Rebuy
20 Safe Leave
21 Cash Out

22 Intermission Engine

23 Hand Commitment Phase A
24 Forced Bet / Deal Phase B

25 Action Engine
26 Full Raise Tracking
27 Timer Scheduler
28 Disconnect / Sit Out
29 Recovery

30 Side Pot Builder
31 Uncalled Excess
32 Settlement Skeleton
33 Catastrophic Refund

34 Client Seed Contribution
35 Effective Client Seed
36 Poker Random Stream
37 Deck Builder
38 Deal Sequencer
39 Fairness Release

40 Viewer Projection

41 WS Gateway
42 WS Auth
43 Snapshot / Sync
44 Event Buffer
45 Backpressure

46 Control / Take Over

47 Spectator
48 Chat
49 Host Commands

50 History Projection
51 Metrics / Audit

52 Actor / Funding Tests
53 Timer / Recovery Tests
54 Pot / Settlement Tests
55 Fairness Tests
56 Privacy Tests
57 WS / Control Tests
58 Product-gap Protection Tests
```

Production Hand Evaluator 与依赖 Gap 的正式 Shortcut / Entry Blind / Initial Button Rule 在 Product Gap Boundary 停止。

---

# 355. IS-07 Prohibited Implementation

禁止：

```text
choose Ante posting mode
choose Post BB live/dead semantics
choose Initial Dealer Button
implement Production Wheel/Suit/Kicker from common knowledge
choose Raise Pot shortcut formula

mark incomplete ruleset production ready
create production-paid Poker while gaps unresolved

use Redis as Stack/Pot/Hand authority
broadcast before DB commit
allow stale runtime epoch writes

direct Wallet DML outside Funding Gateway
partial cash out
mid-hand Top-up
Auto Rebuy

Host Stack/Pot/Deck/Winner/Settlement mutation
Host/Ops private Hole Card peek

send all Hole Cards then hide in UI

full fairness to spectator/public
Server Seed reuse
rerandomize deck mid-hand

settle + refund same Hand
guess asset ownership in catastrophic recovery

use WS text/event as financial authority
```

---

# 356. IS-07 Acceptance Criteria

```text
AC-07-01  Poker remains independent Go Service
AC-07-02  PostgreSQL is Poker durable authority
AC-07-03  Table Actor is single writer coordinator
AC-07-04  Commit-before-broadcast mandatory
AC-07-05  runtime_epoch fences stale actors
AC-07-06  Redis loss preserves Poker truth
AC-07-07  Production blocked by five Poker Product Gaps
AC-07-08  no Production gap bypass escape hatch

AC-07-09  Table Create uses 2–9 seats and fixed presets
AC-07-10  one nonclosed owned table/user
AC-07-11  Password Table only Argon2id hash
AC-07-12  access grant session-bound ephemeral
AC-07-13  seat lease exactly 30s
AC-07-14  durable reservation cannot permanently lock seat
AC-07-15  one active Poker Session/user
AC-07-16  Poker money integer-chip units

AC-07-17  Buy-in atomic Wallet/Session/Seat/Stack
AC-07-18  Buy-in exactly 40–100BB
AC-07-19  first Buy-in locks running settings
AC-07-20  Post-BB semantics remain unresolved
AC-07-21  intermission exactly 5s and frozen order
AC-07-22  Top-up boundary-only
AC-07-23  Top-up cannot fund above 100BB
AC-07-24  winnings above 100BB not reduced
AC-07-25  Rebuy Window exactly 60s
AC-07-26  no Auto Rebuy
AC-07-27  no Partial Cash Out
AC-07-28  Safe Leave waits current Hand
AC-07-29  Cash Out exactly-once atomic
AC-07-30  Realized P/L at Cash Out
AC-07-31  empty close 30m + zero durable work/assets

AC-07-32  Hand Start two durable commits
AC-07-33  Seed Hash before Hole Cards
AC-07-34  Hand Participant set immutable
AC-07-35  Initial Button unresolved
AC-07-36  Player Actions server-authoritative
AC-07-37  action_id effectively-once
AC-07-38  Hand Version stale protection
AC-07-39  Full Raise / Short All-in tracking exact
AC-07-40  Pot shortcut formula unresolved

AC-07-41  Action deadline exactly 30s
AC-07-42  timeout/user action mutually exclusive
AC-07-43  two timeouts → next-hand Sit Out
AC-07-44  client disconnect timer continues
AC-07-45  service failure timer semantics pause
AC-07-46  recovery = 30s Grace + fresh 30s action
AC-07-47  15m auto-safe-leave only after settled Hand

AC-07-48  Pot from participant commitments
AC-07-49  deterministic side pots
AC-07-50  Fold contributes but cannot win
AC-07-51  Uncalled Excess returns before pot settlement
AC-07-52  Production evaluator blocked by Gap-04
AC-07-53  Hand Settlement exactly-once
AC-07-54  deterministic Odd Chip
AC-07-55  zero-sum / Rake 0
AC-07-56  catastrophic refund restores start stacks
AC-07-57  ambiguous recovery → NEEDS_REVIEW

AC-07-58  Effective Client Seed deterministic encoding
AC-07-59  Effective Client Seed Golden Vector
AC-07-60  256-bit Poker Server Seed
AC-07-61  deterministic unbiased deck
AC-07-62  Poker Deck Golden Vector
AC-07-63  no deck index reuse
AC-07-64  participant-only full fairness after 24h
AC-07-65  spectator never full seed
AC-07-66  viewer projection hides unauthorized private fields

AC-07-67  WS path/subprotocol exact
AC-07-68  WS Auth first-frame deadline 10s
AC-07-69  frame loss/duplication converges via version/snapshot
AC-07-70  slow client cannot block actor
AC-07-71  control_epoch Take Over atomic

AC-07-72  Spectator creates no Session/Stack
AC-07-73  Chat original survives moderation
AC-07-74  Host cannot modify financial/random authority
```

---

# 357. IS-07 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| IS-FRZ-283 | Poker 继续作为独立 Go Service；Platform BFF 通过 Typed PokerPort / Private Internal HTTP 调用，Browser Realtime 直连 `/ws/poker`。 | FROZEN |
| IS-FRZ-284 | `POKER-PROD-GAP-01～05` 全部解决前 `POKER_PRODUCTION_READY=false`；Production 禁止真实 Table/Seat/Buy-in/Hand，测试 Fixture 不得绕过生产 Gate。 | FROZEN |
| IS-FRZ-285 | Poker Service 锁 `coder/websocket v1.8.15` 与 `x/crypto v0.56.0`，继续复用已锁 pgx/go-redis。 | FROZEN |
| IS-FRZ-286 | 每张 Non-closed Table 使用 Single-writer Table Actor + bounded mailbox；Actor 是串行化协调器，PostgreSQL 才是 Authority。 | FROZEN |
| IS-FRZ-287 | Poker 永久 Commit-before-Broadcast；DB 成功而 WS 丢失仍视为业务成功，由 Snapshot 收敛。 | FROZEN |
| IS-FRZ-288 | 每桌 Durable `runtime_epoch` 在 Actor Claim/Recovery 时递增，所有 Mutation 必须 Epoch Fencing；Redis Lock 不作为 Split-brain Authority。 | FROZEN |
| IS-FRZ-289 | Actor Mailbox/WS Queue 容量保留 Implementation Config；满队列不得丢 Player Command，返回可用原 Action ID 重试的 Backpressure Error。 | FROZEN |
| IS-FRZ-290 | IS-07 仅新增 `000022__poker_runtime_refinements` / `000023__poker_v1_presets_and_incomplete_ruleset`，不修改既有 Migration。 | FROZEN |
| IS-FRZ-291 | 新增 Versioned Blind Preset：六档 SB/BB、40–100BB Buy-in；Ante 10% BB 可保存金额但 Ante Posting Mode 仍 CONFIG_INCOMPLETE。 | FROZEN |
| IS-FRZ-292 | Seed `poker-rules-v1-incomplete` 明确保留五个 NULL Product Gap 字段；只有完整新 Ruleset Version 才可 Production Active。 | FROZEN |
| IS-FRZ-293 | Table Create 字段固定 Name/Public-or-Password/Password/2–9 Seats/Blind Preset/Spectator/Chat；禁止 arbitrary Blind/Ante/Buy-in/Unlisted。 | FROZEN |
| IS-FRZ-294 | Password Table 使用 Argon2id PHC Hash、16-byte Salt、32-byte derived key；参数进入 Implementation Config，明文永不持久化/记录。 | FROZEN |
| IS-FRZ-295 | Password Access 成功创建 session-hash/table-bound Redis Grant；Session Rotation 自动失效，Redis 丢失只要求重新输密码。 | FROZEN |
| IS-FRZ-296 | IS-03 Durable `seat_reservations` 明确仅承担请求/幂等/审计事实；真正 30s Seat Lease 使用 Redis SET NX，Redis Lease 丢失不得继续 Buy-in。 | FROZEN |
| IS-FRZ-297 | `poker.seats` 固化 Waiting/Active/Blind-wait/Sit-out/Leave/Rebuy 状态，与 Connected/Disconnected 状态分离。 | FROZEN |
| IS-FRZ-298 | `poker.sessions` 为 Table Stack/Control/最终 Cash Out Authority，并保持一用户一个 Active Poker Session。 | FROZEN |
| IS-FRZ-299 | Poker 全部资金必须为 500,000 Atomic Units 整倍数；钱包小数筹码永不进入 Poker。 | FROZEN |
| IS-FRZ-300 | Funding stable Biz ID 固定 Buy-in/Top-up/Rebuy/Cash-out family，并叠加 IS-05 Durable Idempotency。 | FROZEN |
| IS-FRZ-301 | Poker Funding Lock Order 按 IS-05 固定 Poker Domain Row→Available Chips Wallet→Ledger；Poker Runtime 只 EXECUTE 三个 Narrow Funding Gateway。 | FROZEN |
| IS-FRZ-302 | Buy-in 同事务完成 Table/Seat/Session Guard、Wallet Debit、Ledger、Session、Seat、Table Stack；范围固定 40–100BB。 | FROZEN |
| IS-FRZ-303 | 第一笔成功 Buy-in 原子锁定 Max Seats、Blind/Ante Preset、Buy-in Bounds、Spectator 与运行经济规则；后续 Host 不可修改。 | FROZEN |
| IS-FRZ-304 | Running-table Entry 保留 WAIT_FOR_BB / POST_BB_NOW，后者只能表达“下一可参与 Hand 补一 BB”；Live/Dead/Betting Rights 继续 Gap-02。 | FROZEN |
| IS-FRZ-305 | Hand Intermission 固定 5s，并按 Settlement→Leave/Remove→Sit Out→Top-up/Rebuy→New Seat→Blind State→Participants→Hand Start 顺序。 | FROZEN |
| IS-FRZ-306 | Top-up Hand 中只创建 Pending Funding，Boundary 重新校验 Wallet/Stack/100BB 并原子生效；盈利 Stack>100BB 不强制降低。 | FROZEN |
| IS-FRZ-307 | Stack=0 固定 60s REBUY_WINDOW；成功 Rebuy 继续同 Session，超时 Cash Out 0 + Session Settlement；无 Auto Rebuy。 | FROZEN |
| IS-FRZ-308 | Safe Leave 统一覆盖用户/Host/Auto/Table Close；当前 Hand Participant 必须 Leave After Hand，Fold 后仍等 Settlement。 | FROZEN |
| IS-FRZ-309 | Cash Out 使用 `poker_cashout:{session_id}`，Stack=0/Wallet Credit/Ledger/Session Settlement 同事务，重复返回原结果且无 Partial Cash Out。 | FROZEN |
| IS-FRZ-310 | Empty Table 只有连续 30m 且无人/无 Hand/无 Funding/无 Poker In Play 时由 Durable Job 幂等关闭。 | FROZEN |
| IS-FRZ-311 | Hand Start 固定 Phase A Commitment + Phase B Forced Bets/Deal 两次 Durable Commit，Seed Hash/Participant/Deck 必须先于发牌持久化。 | FROZEN |
| IS-FRZ-312 | TD `hand_players` 在实现中统一映射到 IS-03 已冻结 `poker.hand_participants`；不得创建第二份 Hand Participant Authority。 | FROZEN |
| IS-FRZ-313 | Dealer Button 后续移动及 Heads-up/3+ 行动顺序按冻结规则实现；第一手 Initial Button 继续 Gap-03，Actor 不自行选择。 | FROZEN |
| IS-FRZ-314 | Player Action 固定 action_id/expected_hand_version/control_epoch + Server Legal Action Set；Client 不计算权威 To-call/Min/Max/Raise Rights。 | FROZEN |
| IS-FRZ-315 | Full Raise 使用 full_raise_sequence/previous full increment；Short All-in 不推进 Full Raise Sequence，避免错误 reopen Raise Rights。 | FROZEN |
| IS-FRZ-316 | Min/Half/Two-third/Pot/All-in Shortcut 仅由 Server 产生；Raise 场景精确 Formula Version 继续 Gap-05，Frontend 不自行计算。 | FROZEN |
| IS-FRZ-317 | 玩家 Action Timer 固定 PG Durable 30s Deadline，无 Time Bank；Timer Scheduler 只负责唤醒而非时间 Authority。 | FROZEN |
| IS-FRZ-318 | Timeout 使用 `timeout:{hand}:{action_sequence}` Stable System Action；与人工 Action 通过 Hand Row Lock/Version 竞争，最多一个 Commit。 | FROZEN |
| IS-FRZ-319 | Timeout 可 Check→AUTO_CHECK，否则 AUTO_FOLD；连续两次 Timeout 下一 Hand Sit Out，成功人工 Action 清零 streak。 | FROZEN |
| IS-FRZ-320 | Client Disconnect 时 Hand/Timer 继续；Boundary 仍断线才 Sit Out，Boundary 前 Reconnect 只清除 disconnect-induced Sit Out。 | FROZEN |
| IS-FRZ-321 | Service Failure 恢复固定 PG Reconstruct→30s Reconnect Grace→若仍需决策则新完整30s Action Window；故障期间旧 Deadline 不触发立即 Fold。 | FROZEN |
| IS-FRZ-322 | Continuous Sit Out/Disconnect≥15m 且无 Unsettled Hand 时 Durable Job 触发 Auto Safe Leave/Cash Out。 | FROZEN |
| IS-FRZ-323 | Pot Authority 永久来自 per-participant Street/Total Commitment；单一 pot_total 只能是 Projection。 | FROZEN |
| IS-FRZ-324 | Side Pot Builder 按递增 Commitment Levels 确定性构造；Fold Contribution 入 Pot，但 Fold Player 不进入 Winner Eligible Set。 | FROZEN |
| IS-FRZ-325 | 无第二位玩家匹配的最高 Uncalled Excess 在 Settlement 前 RETURN_UNCALLED 到原 Stack，不成为 Pot/Award/Wagered Settlement。 | FROZEN |
| IS-FRZ-326 | Hand Evaluator 接口输出 Category/Rank Vector/Best-five/Version，但 Production Evaluator 在 Wheel/Suit/Kicker/Tie Gap-04 解决前保持 BLOCKED。 | FROZEN |
| IS-FRZ-327 | Early Winner 不继续无意义 Board、不强制公开 Hole Cards；全部 All-in 无决策后自动 Runout。 | FROZEN |
| IS-FRZ-328 | Hand Settlement 使用 `poker_hand_settlement:{hand_id}`，同一 PG Transaction 完成 Uncalled/Pot/Evaluator/Awards/Stack/SETTLED，且不碰主 Wallet。 | FROZEN |
| IS-FRZ-329 | Odd Chip 固定从 Button 左侧顺时针在该 Pot 的并列 Eligible Winners 中分配并持久化。 | FROZEN |
| IS-FRZ-330 | Poker V1 每 Hand 强制 Total Commitments = Pot Awards + Returned Uncalled，Rake=0，不允许 Issue/Burn。 | FROZEN |
| IS-FRZ-331 | Catastrophic Hand Refund 仅在合法恢复不可能时恢复每个 Participant 至 Hand-start Stack；来源不可证明则 NEEDS_REVIEW。 | FROZEN |
| IS-FRZ-332 | Poker Client Seed Contribution 默认 CSPRNG、可自定义 1–128 UTF-8 bytes、不做 Unicode Normalization；Hand COMMITTED 后仅影响下一 Hand。 | FROZEN |
| IS-FRZ-333 | Effective Client Seed 固定按 seat_no 升序编码 Table/Hand/(seat,version,contribution) 后 SHA-256，并冻结 Golden Hash。 | FROZEN |
| IS-FRZ-334 | Poker 每 Hand Server Seed 固定 32-byte CSPRNG，SHA-256 Precommit，AES-256-GCM 独立 Poker Keyring 加密。 | FROZEN |
| IS-FRZ-335 | Poker Deck Stream 固定 `chaldea-poker-hmac-sha256-v1`，以 Server Seed HMAC-SHA-256 + Effective Client Seed + Table/Hand/Algorithm 产生字节流。 | FROZEN |
| IS-FRZ-336 | Poker Card Code 复用 IS-06 C/D/H/S + A..K 0–51 编码，但该编码不定义 Suit Tie/Evaluator Rule。 | FROZEN |
| IS-FRZ-337 | Poker Canonical Deck 固定 `[0..51]` + unbiased Fisher–Yates，并冻结 HMAC/Deck/Deck-hash Golden Vector。 | FROZEN |
| IS-FRZ-338 | `poker-deal-v1` 固定数字牌桌不模拟 Burn Card；Hole Cards 两轮按 seat_no 升序、Board 依次 3/1/1 消费锁定 Deck；改变需新 Deal Sequence Version。 | FROZEN |
| IS-FRZ-339 | 每次 Deal 必须持久化 deck_index/card/recipient/visibility；运行中不得重新调用随机数生成下一张牌。 | FROZEN |
| IS-FRZ-340 | Full Fairness 只在 Settlement+24h 后向 Durable Hand Participant Set开放；Spectator/Public 永不获得完整 Seed/Deck。 | FROZEN |
| IS-FRZ-341 | Poker Viewer Projection 必须服务端生成；Player 只获自己私牌+公共信息，Spectator/Host/Ops 只获授权公共状态。 | FROZEN |
| IS-FRZ-342 | WS 固定 `/ws/poker` + `chaldea-poker.v1`；Upgrade 验证 Origin/Subprotocol，Auth Pending 后 10s 内必须 `auth.connect`。 | FROZEN |
| IS-FRZ-343 | Poker Connect Ticket 继续使用 IS-04 ct1/Ed25519/60s/single-use/session/security-epoch/restart fencing，Ticket 不进 URL/Subprotocol/Log。 | FROZEN |
| IS-FRZ-344 | Client/Server WS Envelope 精确沿用 TD-13 Version/Table/Hand/Control/Event 字段；Asset Payload 仍使用 String。 | FROZEN |
| IS-FRZ-345 | WS Message Family 固定 Auth/Sync/Hand Action/Sit-out/Resume/Client Seed/Chat/Ping 与 Snapshot/Table/Seat/Session/Hand/Timer/Control/Chat/Service/Error/Pong。 | FROZEN |
| IS-FRZ-346 | WS Frame 允许 loss/duplicate；event_seq 仅当前 Actor Runtime Epoch 内单调，Durable Version 是 Table/Hand/Runtime/Control；Gap 使用 Sync/Snapshot。 | FROZEN |
| IS-FRZ-347 | Redis Event Buffer 只能保存 Viewer-neutral/safe public descriptor；无法安全生成 Viewer Delta 时必须 Full Snapshot。 | FROZEN |
| IS-FRZ-348 | WS Send Queue/Message Limit 为 bounded Implementation Config；Slow Client 断开后 Snapshot Reconnect，不允许拖住 Actor。 | FROZEN |
| IS-FRZ-349 | Active Session 一次一个 Active Control Connection；真正 Action Authority 是 Durable control_epoch。 | FROZEN |
| IS-FRZ-350 | Take Over 原子 `control_epoch++`；旧 Controller 即时失去行动权限并可降为 Read-only。 | FROZEN |
| IS-FRZ-351 | Spectator 实时无延迟、无 Session/Stack/Control；已有 Active Session 的用户禁止观战另一桌。 | FROZEN |
| IS-FRZ-352 | Table Chat 仅 USER_TEXT/SYSTEM，原消息 Durable；Mute/Hide 不删除历史，Chat Length 留 Implementation Config。 | FROZEN |
| IS-FRZ-353 | Host 仅能控制 Accepting Players/Remove After Hand/Spectator/Mute/Close；永不得看私牌或修改 Stack/Pot/Deck/Winner/Settlement。 | FROZEN |
| IS-FRZ-354 | Poker Maintenance 阻断新 Table/Seat/Buy-in/Hand，但 accepted Hand/Recovery/Safe Leave/Cash Out 必须继续。 | FROZEN |
| IS-FRZ-355 | Poker Rankings 正式 Session Profit/Biggest Win/Total Wagered 只在 Parent Session Cash Out 后发布；Refunded Hand 排除。 | FROZEN |
| IS-FRZ-356 | IS-07 Production Gate 必须通过 Actor/Funding/Timer/Recovery/Pot/Zero-sum/Fairness/Privacy/WS/Takeover 与五项 Product Gap Protection Tests；当前没有 Poker Production Ruleset。 | FROZEN |

---

# 358. Open / Blocked Register after IS-07

```text
Poker:
POKER-PROD-GAP-01 Ante Posting Mode
POKER-PROD-GAP-02 Post BB Now live/dead / betting rights
POKER-PROD-GAP-03 Initial Dealer Button
POKER-PROD-GAP-04 Hand Evaluator edge/tie rules
POKER-PROD-GAP-05 Pot Shortcut Raise formula
= OPEN

Poker Production Ruleset
= CONFIG_INCOMPLETE
= NOT PRODUCTION READY

NewAPI:
SV-01 ～ SV-16
= BLOCKED_BY_NEWAPI_SOURCE_VERIFY

Database:
@NEWAPI_USER_ID_PG_TYPE
= BLOCKED_BY_SV-05

@NEWAPI_KEY_ID_PG_TYPE
= BLOCKED_BY_SV-05 / SV-06

Reward:
Hourly Product OPEN
Relief Product OPEN
Reward Product Maintenance / Future Amount Policy / Alert Threshold
= unchanged

Public Record Selection Policy
= UNRESOLVED

Deployment:
DEPLOYMENT-VERIFY-01
= PENDING

Implementation Config:
Poker mailbox / WS queue / WS message size / Chat size /
Table Name / Password max / Argon2id parameters
= UNRESOLVED_IMPLEMENTATION_CONFIG
```

---

# 359. Change Log — WORKING v0.7

## Added

- 用户正式确认 `IS-07 — Poker Implementation Specification`；
- 冻结 `IS-FRZ-283 ～ IS-FRZ-356`；
- 冻结 Poker 独立 Service / Typed Platform Boundary；
- 冻结 Poker Production Hard Gate；
- 冻结 WebSocket / Argon2id Dependency；
- 冻结 Single-writer Table Actor / Commit-before-Broadcast；
- 冻结 Runtime Epoch Fencing；
- 冻结 bounded Actor Mailbox / WS Backpressure Boundary；
- 冻结 `000022 / 000023`；
- 冻结 Blind Preset / Incomplete Ruleset Seed；
- 冻结 Table / Password Access / Seat Reservation；
- 冻结 Buy-in / Top-up / Rebuy / Safe Leave / Cash Out；
- 冻结 5s Hand Intermission；
- 冻结 Two-phase Hand Start；
- 冻结 Hand Participant Physical Mapping；
- 冻结 Action / Version / Raise Sequence / Timer；
- 冻结 Disconnect vs Service Failure；
- 冻结 30s Grace + Fresh 30s Action；
- 冻结 Side Pot / Uncalled Excess / Odd Chip / Zero-sum；
- 冻结 Catastrophic Hand Refund；
- 冻结 Poker Client Seed / Effective Seed Canonical Encoding / Golden Vector；
- 冻结 Poker Server Seed / Deck Stream / Card Encoding / Deck Golden Vector；
- 冻结 `poker-deal-v1` 数字牌桌 Deal Sequence；
- 冻结 24h Participant-only Fairness Release；
- 冻结 Viewer-specific Projection；
- 冻结 `/ws/poker` / `chaldea-poker.v1` / 10s Auth Deadline；
- 冻结 Snapshot / Sync / Backpressure；
- 冻结 `control_epoch` / Take Over；
- 冻结 Spectator / Chat / Host / Maintenance；
- 冻结 Poker Rankings Commit Boundary；
- 冻结 Poker Test / Product-gap Protection Gate。

## Preserved

```text
TD-FRZ-001 ～ TD-FRZ-552
IS-FRZ-001 ～ IS-FRZ-356

Poker Product Gap 01～05 = OPEN
Poker Production Ruleset = CONFIG_INCOMPLETE

Reward Product OPEN
Public Record Selection Policy

SV-01 ～ SV-16 unresolved
DEPLOYMENT-VERIFY-01 pending
Implementation-only Config unresolved values
```

## Supersession

```text
Existing IS-FRZ superseded:
None
```

---

# 360. Next Batch

> **IS-08 — Rankings / History / Announcements / Jobs Implementation Specification**

IS-08 将落实：

```text
Ranking Source Fact
Ingestion Cursor
Current Asset Snapshot

Asia/Shanghai Period
Daily / Weekly / All-time

Aggregate Set
Shadow Build
Publish Pointer

API / RP Ranking
Direct Play Ranking
Poker Cash-out Ranking

Unified History
Round / Session / Hand Detail

Announcements
Content / Notification Revision
Popup / Read / Ack

Durable Jobs
Lease / Retry / Schedule
Ranking Rebuild

Public Records hidden-until-policy
```

---

# 361. IS-08 — Rankings / History / Announcements / Jobs Implementation Specification

> 状态：`FROZEN`  
> 用户确认：`整体按上述 IS-08 方案通过`  
> Frozen Decision Range：`IS-FRZ-357 ～ IS-FRZ-414`  
> Public Records：`PUBLIC_RECORD_SELECTION_POLICY = UNRESOLVED`  
> RP Source：`SV-12 = BLOCKED_BY_NEWAPI_SOURCE_VERIFY`

## 361.1 Purpose

IS-08 将已冻结的 Rankings / History / Announcements / Jobs 技术设计落成：

```text
Ranking Source Fact
Ingestion Cursor
Current Asset Snapshot
Asia/Shanghai Period Engine
Aggregate Set / Shadow Rebuild / Atomic Publish

Unified History Index
Round / Session / Hand Durable Detail

Public Game Event Projection

Announcement ID / Content Version / Notification Revision
Placement / Popup / Read / Acknowledgements
Markdown Sanitization / Scheduling

PostgreSQL Durable Job Registry
Lease / Heartbeat / Retry / Schedule / Maintenance
```

本批明确不改变：

```text
SV-12 Source Verification blocker
PUBLIC_RECORD_SELECTION_POLICY unresolved
Reward Product OPEN
Poker Product Gap 01～05
```

---

# 362. Service / Package Boundary

不新增独立服务。

Platform-owned：

```text
Ranking Worker
History Index Worker
Announcement Scheduler
Generic Platform Job Worker
```

运行于 `chaldea-platform`。

Poker-owned Job 继续由 `chaldea-poker` 处理，但 Durable Job Fact 统一位于 PostgreSQL。

Redis 永远不是：

```text
Ranking Authority
History Authority
Announcement Scheduler Authority
Job Queue Authority
```

Backend 细化：

```text
backend/internal/ranking/
  source/
  ingestion/
  period/
  asset_snapshot/
  aggregate/
  publish/
  rebuild/
  exclusion/
  projection/

backend/internal/history/
  indexer/
  query/
  round/
  poker/
  authorization/

backend/internal/content/
  announcement/
  revision/
  placement/
  popup/
  readstate/
  acknowledgement/
  markdown/
  sanitizer/
  scheduler/
  publicevents/

backend/internal/jobs/
  registry/
  repository/
  dispatcher/
  lease/
  retry/
  scheduler/
  maintenance/
  worker/
```

---

# 363. IS-08 Dependency Lock

Platform Backend 新增并锁定：

```text
github.com/yuin/goldmark            v1.8.5
github.com/microcosm-cc/bluemonday  v1.0.26
```

用途：

```text
goldmark
→ Controlled Markdown → HTML

bluemonday
→ final allowlist sanitization
```

不使用 beta Markdown Runtime 作为 Production 基线。

---

# 364. Forward Migrations

新增：

```text
000024__ranking_history_runtime_refinements
000025__announcements_runtime_refinements
000026__durable_jobs_runtime_refinements
```

不修改既有 Migration。

---

# 365. Ranking Authority / Metric Families

正式管线：

```text
Economy / Games / Poker / API Attribution
        ↓
Derived Ranking Facts
        ↓
Versioned Aggregate Set
        ↓
Published Pointer
        ↓
Public Ranking
```

Ranking Table 永远不成为人工 Score Authority。

V1 Public Metrics：

```text
ASSETS_GAMES:
  TOTAL_ASSETS
  GAME_PROFIT
  BIGGEST_WIN
  TOTAL_WAGERED
  POKER_PROFIT

RP_USAGE:
  RP_CALLS
  RP_ERRORS
  RP_CREDITS_CONSUMED
```

内部 Fact Families：

```text
DIRECT_PLAY_PROFIT
DIRECT_PLAY_WAGERED
DIRECT_PLAY_BIGGEST_WIN

POKER_SESSION_PROFIT
POKER_HAND_WAGERED
POKER_HAND_BIGGEST_WIN

RP_ATTEMPT
RP_SUCCESS_CALL
RP_ERROR
RP_CREDIT_CONSUMED
```

---

# 366. Derived Ranking Facts / Conflict Safety

`ranking.source_facts` 增加：

```text
dimension_key BYTEA NOT NULL
source_payload_hash BYTEA NOT NULL
eligibility_at TIMESTAMPTZ NULL
source_observed_at TIMESTAMPTZ NOT NULL
```

Unique：

```text
(source_type, source_id, metric_family, dimension_key)
```

`dimension_key`：

```text
SHA-256(
  "CHALDEA-RANKING-DIMENSION-V1"
  || 0x00
  || LP16(newapi_user_id)
  || LP16(game_slug or "")
  || LP16(model_id or "")
)
```

重复 Source：

```text
same unique identity + same hash
→ no-op

same unique identity + different hash
→ RANKING_SOURCE_FACT_CONFLICT
→ no UPDATE
→ create attention/incident candidate
→ block untrusted routine publish when correctness uncertain
```

`source_facts` 继续 Append-only / Rebuildable Projection。

---

# 367. Ranking Ingestion Cursor

每 Source Family 保存：

```text
cursor_timestamp
cursor_source_id
updated_at
```

排序：

```text
(source_timestamp ASC, source_id ASC)
```

Worker：

```text
read committed source batch
→ insert derived facts idempotently
→ advance cursor
```

Fact 已 Commit 但 Cursor 未推进：

```text
next scan
→ duplicate no-op
→ cursor safely advances
```

---

# 368. Direct Play / Poker Ranking Facts

Direct Play 只有 `SETTLED` Round：

```text
DIRECT_PLAY_PROFIT
= net_change_units

DIRECT_PLAY_WAGERED
= total_stake_units

if net_change_units > 0:
DIRECT_PLAY_BIGGEST_WIN
= net_change_units
```

Event Time = `settled_at`。

Refunded / Cancelled 不产生 Ranking Fact。

Poker Session：

```text
state = SETTLED
→ POKER_SESSION_PROFIT = realized_pl_units
```

Period Event Time：

```text
cashout / session ended_at
```

Poker Hand 每 Participant 可以先产生：

```text
eligibility_state = HELD

hand_net_units =
ending_stack_units - starting_stack_units

hand_wagered_units =
total_committed_units - returned_uncalled_units
```

Hand Event Time：

```text
hand.settled_at
```

Eligibility Time：

```text
parent session cashout_at
```

Parent Session Cash Out 后才：

```text
HELD → ELIGIBLE
```

Refunded Hand 不进入 Wagered / Biggest Win。

---

# 369. Game Profit / Wagered / Biggest Win

Game Profit：

```text
Σ Direct Play settled net
+
Σ Poker settled Session realized P/L
```

排除 Reward、Grant、Admin Adjustment、Exchange、Poker Funding Movement。

Total Wagered：

```text
Σ Direct Play total_stake
+
Σ eligible Poker hand wagered
```

Poker 的 Blind / Ante / Call / Bet / Raise / All-in 最终由 Participant Commitment 体现；Returned Uncalled 不计。

Biggest Win：

```text
Direct Play:
positive Round Net

Poker:
positive Participant Hand Net
+
Parent Session already Cash Out
```

不使用 Gross Payout / Pot Size / Total Session Profit。

---

# 370. Current Total Assets Ranking

Total Assets：

```text
CURRENT only
```

对每个 Eligible User 使用 IS-05 Strict Authoritative Asset Snapshot：

```text
Reserve
Active NewAPI Quota
Available Chips
Poker In Play
Processing Assets
```

保存 `ranking.current_asset_snapshots` 并归属 Candidate Aggregate Set。

完整集合 Gate：

```text
any required user source unavailable / stale / ambiguous
→ entire candidate INCOMPLETE / FAILED
→ no publish
```

Public 保留：

```text
Last Good Published Set
+
STALE / DEGRADED
+
Last Updated
```

不得把未知 Active 当 0，也不得静默遗漏用户后发布“完整榜”。

Implementation Config：

```text
RANKING_ASSET_SNAPSHOT_MAX_SOURCE_AGE
RANKING_ASSET_SNAPSHOT_MAX_BUILD_SKEW
= UNRESOLVED_IMPLEMENTATION_CONFIG
```

---

# 371. Ranking Period Engine

统一业务时区：

```text
Asia/Shanghai
```

Daily：

```text
00:00 → next 00:00
```

Weekly：

```text
Monday 00:00 → next Monday 00:00
```

All Time：

```text
feature activation boundary → current watermark
```

Stable Internal Period Keys：

```text
D:YYYY-MM-DD
W:YYYY-MM-DD   # Monday date
A:<feature_activation_id>
CURRENT
```

Total Assets 不支持历史日期回放。

---

# 372. RP Feature Activation / Source Boundary

`ranking.feature_activation` 保存：

```text
feature_key
activation_at
activated_by
created_at
```

RP：

```text
feature_key = RP_RANKINGS
```

只有：

```text
request_at >= activation_at
```

才生成 RP Fact。

同时要求：

```text
SV-12 verified
```

未验证时：

```text
RP_RANKINGS production activation forbidden
```

不得回溯旧日志猜 Purpose。

Aggregator 只消费 Finalized Attribution，至少需要：

```text
newapi_user_id
logical_request_id
newapi_key_id
key_purpose_snapshot
request_model_id
request_kind
final_status
error_category
actual_credit_consumed
request_at
finalized_at
```

具体 NewAPI Feed 继续 `BLOCKED_BY_SV-12`。

---

# 373. RP Logical Request Semantics

Source Identity：

```text
source_type = API_REQUEST_ATTRIBUTION
source_id   = logical_request_id
```

只统计：

```text
key_purpose_snapshot = RP
+
actual inference / generation request
```

排除：

```text
model list
balance
health
auth
admin
```

Internal Provider Retry 使用同 `logical_request_id`，永不额外计数。

正式调用流程：

```text
RP_ATTEMPT +1
```

成功：

```text
RP_SUCCESS_CALL +1
```

Error Eligible：

```text
Timeout
429
Upstream Error
Platform Call Error
Stream Interruption
```

产生：

```text
RP_ERROR +1
```

排除 Invalid Key Probe / Cancel Before Upstream / Internal Retry Count。

实际扣费 >0：

```text
RP_CREDIT_CONSUMED
```

Credits 只用 Final Actual Settled API Credit，不用 USD / Estimate / Raw Quota。

---

# 374. Model Aggregation / Public Privacy

Model 使用 Stable Chaldea `model_id`。

同一 Aggregate Set：

```text
model_id = NULL
→ All Models

model_id != NULL
→ per-model rows
```

Model Filter 通过 per-model rows，不为每个模型建立独立 Aggregate Set。

Retired Model 继续从历史 Catalog Metadata 解析 Display Name。

Public 可以返回：

```text
Master display
rank
aggregate metric
model distribution
period
last_updated
data_freshness
```

永不公开：

```text
Key ID
Logical Request ID
Prompt / Response
Raw Error
single-request timestamp
IP
UA
Provider
Channel
Credential
Internal Route
```

---

# 375. Aggregate Set / Hash / Tie Rank

Aggregate Lifecycle：

```text
BUILDING
→ SHADOW
→ PUBLISHED
→ SUPERSEDED

or
FAILED
```

保存：

```text
domain
metric
period
version
source_watermark
data_completeness
aggregate_hash
previous_published_set_id
```

Aggregate Hash 固定 Domain：

```text
CHALDEA-RANKING-AGGREGATE-V1
```

输入：

```text
domain
metric
period
source watermark
all rows sorted by model_id then newapi_user_id
```

金额按 Integer Decimal String Canonical Encoding。

排名使用 SQL `RANK()`：

```text
100 → 1
90  → 2
90  → 2
80  → 4
```

主指标恰好 0：

```text
public row omitted
```

负 Profit 仍是合法真实值。

Tie 内稳定显示：

```text
newapi_user_id ASC
```

不改变 Rank。

---

# 376. Identity / My Rank

Ranking Row 实时解析：

```text
Current Master Nickname
Current Avatar
```

用户改名后 Ranking 展示同步变化。

真实归属永远：

```text
newapi_user_id
```

Public Event 使用 event-time Identity Snapshot。

匿名：

```text
my_rank = null
```

登录用户可以得到自己的：

```text
my_rank
my_value
```

自身指标 0：

```text
my_rank = null
my_value = "0"
```

只有自己的 My Rank 可以 Cross-link 到：

```text
/api/usage?purpose=rp
```

---

# 377. Routine Aggregation / Publish

产品目标：

```text
public aggregation lag <= 5 minutes
```

Implementation Config：

```text
RANKING_ROUTINE_INTERVAL
RANKING_SOURCE_SCAN_BATCH_SIZE
= UNRESOLVED_IMPLEMENTATION_CONFIG
```

Routine：

```text
RANKING_INCREMENTAL
→ scan source
→ derived facts
→ build candidate
→ validate
→ RANKING_PUBLISH
→ atomic pointer swap
```

Source 不完整：

```text
no publish
```

Publish：

```text
BEGIN

lock pointer(domain, metric, period)

assert candidate = verified SHADOW

old PUBLISHED → SUPERSEDED
candidate → PUBLISHED

update pointer
Audit

COMMIT
```

Reader 只按一个 `aggregate_set_id` 读取，禁止新旧混合。

---

# 378. Shadow Repair / Source Exclusion

Repair：

```text
Authoritative Full Rebuild
→ apply active Source Exclusions
→ SHADOW
→ Diff
→ Human Review
→ separate Publish
```

Repair Job `SUCCEEDED` 不等于 Published。

Diff 至少：

```text
old/new source count
old/new user count
metric total delta
Top-N changes
Rank changes
newly included/excluded sources
completeness
watermark
old/new aggregate hash
```

Source Exclusion：

```text
does not delete source
does not edit source_fact
```

Create / Revoke 后必须 Rebuild。

Public Score 仅在新 Shadow 被 Review + Publish 后改变。

---

# 379. Historical Ranking / Maintenance

Closed Daily / Weekly Published Set 永久保留。

Repair：

```text
new aggregate version
→ atomic pointer swap
```

旧 Version：

```text
SUPERSEDED
retained for audit
```

Total Assets 不建立 Historical Snapshot。

Maintenance Scope：

```text
RANKINGS_PUBLISHING
```

Maintenance 下允许：

```text
ingestion
shadow build
validation
diff
```

阻止 Pointer Swap。

Public 继续 Last Good Published Snapshot + Stale Metadata。

---

# 380. Ranking BFF

固定：

```http
GET /api/v1/rankings
GET /api/v1/rankings/snapshots
GET /api/v1/rankings/snapshots/{snapshot_id}
```

Current Query：

```text
domain
metric
period:
  CURRENT
  TODAY
  THIS_WEEK
  ALL_TIME
model_id nullable
page
page_size
```

规则：

```text
TOTAL_ASSETS → CURRENT only
model_id      → RP metrics only
```

Unsupported：

```text
400 INVALID_RANKING_FILTER
```

Historical selectors：

```text
DAY: YYYY-MM-DD
WEEK: Monday YYYY-MM-DD
```

只公开 Closed Published Snapshot。

---

# 381. History Index / Cursor

`games.history_index` 永久只是 Rebuildable List Projection。

新增：

```text
games.history_ingestion_cursors
```

Source：

```text
DIRECT_PLAY_ROUND
POKER_SESSION
POKER_HAND
```

每 Source 保存 `(timestamp, source_id)` Cursor。

Indexer：

```text
scan durable source
→ UPSERT history projection
→ advance cursor
```

全部 Index 删除后必须可从 Domain Source 重建。

---

# 382. History Query / Record Types

Record Types：

```text
DIRECT_PLAY_ROUND
POKER_SESSION
POKER_HAND
```

默认 `/history`：

```text
DIRECT_PLAY_ROUND
POKER_SESSION
```

Poker Hand 主要：

```text
Session Detail
→ Hand List
→ Hand Detail
```

也允许高级 Filter。

`GET /api/v1/history` 支持：

```text
record_type
mode
game_slug
time_from
time_to
result
status
id
```

Result：

```text
WIN
LOSS
BREAK_EVEN
CANCELLED
REFUNDED
```

Status：

```text
PROCESSING
SETTLED
CANCELLED
REFUNDED
RECOVERING
```

Game Filter 来自 Dynamic Registry，包含 Retired Game。

---

# 383. History Pagination / Detail Authority

List 默认：

```text
occurred_at DESC
record_type ASC
source_id DESC
```

使用 Stable Keyset Cursor。

ID Search：

```text
exact stable source ID
```

Detail：

```http
GET /api/v1/history/rounds/{round_id}
GET /api/v1/history/sessions/{session_id}
GET /api/v1/history/hands/{hand_id}
```

Round 读取：

```text
game_rounds
typed result
Economy
Fairness
```

Session 读取：

```text
poker.sessions
funding
hand list
```

Hand 读取：

```text
poker.hands
participants
actions
dealt cards
pots
awards
fairness
```

Index Payload 永不成为 Detail Authority。

---

# 384. History Authorization / Retention

完整私人 History：

```text
Owner
or
Authorized Records Scope
```

即使有 Records Scope，也不能绕过：

```text
Poker unrevealed Hole Cards
Poker unreleased Full Fairness
```

History 默认只读。

异常通过：

```text
Incident
→ Domain Repair
```

允许 Cross-link：

```text
Wallet Transaction
Provably Fair
Game Entry
Poker Session / Hand
Historical Table Metadata
```

Retired Game / Closed Table 历史继续保留。

V1 无：

```text
public full private-history share
CSV export
JSON export
```

RP Request 不进入 Game History。

---

# 385. Public Game Event Hard Gate

`content.public_game_events` 继续存在，但：

```text
Production Writer = DISABLED
```

直到：

```text
PUBLIC_RECORD_SELECTION_POLICY
```

明确：

```text
Recent Win minimum net
Big Win threshold
display count
automatic selection
Featured promotion
```

未定义时：

```text
no auto selection
no fake event
no synthetic win
module hidden
```

未来 Public Event 仅可保存：

```text
public_event_id
source_type/source_id
game_slug
identity_display_snapshot_id
wager_units
net_win_units
occurred_at
public_policy_version
```

不得保存 Private History / Private Cards / API / Wallet private detail。

---

# 386. Announcement Markdown / Sanitizer

Sanitizer Version：

```text
announcement-sanitize-v1
```

Pipeline：

```text
Controlled Markdown
→ goldmark v1.8.5
→ generated HTML
→ bluemonday v1.0.26 custom allowlist
→ Sanitized Canonical HTML
→ immutable Content Version
```

允许：

```text
paragraph
line break
hr
h2/h3/h4
strong/em
blockquote
ul/ol/li
inline code
code block
safe links
```

禁止：

```text
raw HTML
script
style
iframe
object
embed
event attributes
style/class/id injection
custom JS
javascript:
data:
scheme-relative // links
```

External Links：

```text
rel="noopener noreferrer nofollow"
```

Markdown Image：

```text
disabled
```

图片 / Avatar / Logo 必须来自 `announcement_media_assets`。

---

# 387. Announcement Identity / Schema

三层永久分离：

```text
announcement_id
content_version
notification_revision
```

Popup Identity：

```text
announcement_id + notification_revision
```

`000025` 补齐：

```text
content.announcements:
  canonical_key
  current_content_version_id
  current_notification_revision
  state
  publish_at
  visible_from
  visible_until
  withdrawn_at

announcement_revisions:
  body_markdown_hash
  sanitized_html_hash
  sanitizer_policy_version

notification_revisions:
  announcement_id
  notification_revision
  created_at
  created_by
  reason
```

Notification Revision Unique：

```text
(announcement_id, notification_revision)
```

---

# 388. Content Version / Re-notify

任何 title/body/type/visibility/structured acknowledgement 内容变化：

```text
new immutable Content Version
```

流程：

```text
validate markdown
render
sanitize
hash
insert revision
update current content pointer
```

旧 Published Revision 不直接 UPDATE。

`UPDATE_CONTENT_ONLY`：

```text
content_version++
notification_revision unchanged
```

效果：

```text
new Detail content
existing popup dismissal preserved
existing read state preserved
```

`RE_NOTIFY`：

```text
optional new content version
notification_revision++
insert immutable notification revision
update current revision
Audit
```

新 Revision 重新获得 Popup / Unread 资格。

Re-notify 必须 Impact Preview + Audit。

---

# 389. Announcement Lifecycle / Time

Lifecycle：

```text
DRAFT
SCHEDULED
PUBLISHED
EXPIRED
ARCHIVED
```

`withdrawn_at` 是审计化 Visibility Stop，不删除历史。

Business Input：

```text
Asia/Shanghai
```

DB：

```text
TIMESTAMPTZ UTC
```

Fields：

```text
publish_at
visible_from
visible_until nullable
```

`visible_until > visible_from`，长期公告允许 NULL。

用户侧可见性每次查询必须满足：

```text
state = PUBLISHED
viewer allowed
visible_from <= DB now()
visible_until NULL or DB now() < visible_until
withdrawn_at NULL
```

Scheduler 故障不能让过期内容继续展示。

---

# 390. Placement / Schedule Guard

Placements：

```text
PINNED_LIST
ENTRY_POPUP
POST_LOGIN_POPUP
PUBLIC_HOME_BANNER
DASHBOARD_SUMMARY
```

Pinned：

```text
multiple allowed
manual_order
```

Latest：

```text
publish_at DESC
```

新增固定 Guard Rows：

```text
ENTRY_POPUP
PRIMARY_HOME_BANNER
```

Publish / Schedule / Placement Change：

```text
BEGIN
lock guard
check [visible_from, visible_until) overlap
reject conflict
apply mutation
COMMIT
```

V1 同一时点：

```text
at most 1 ENTRY_POPUP
at most 1 PRIMARY_HOME_BANNER
```

Runtime 不随机挑选冲突内容。

---

# 391. Entry Popup / Browser-local State

```http
GET /api/v1/announcements/current-entry-popup
```

Frontend 只在匿名首次进入：

```text
/
or
/login
```

主动查询。

Intentional public deep links 不强制调用。

Failure：

```text
fail open
```

Browser Local Key：

```text
chaldea.announcement.entry-dismissed.v1
```

只保存：

```text
announcement_id
notification_revision
dismissed_at
```

相同 Revision 不重复主动展示。

清空 Browser Storage 后可能再次展示。

---

# 392. Popup Seen / Read Separation

同一入口流程避免 Entry→Login 后重复同 Revision：

```text
sessionStorage:
chaldea.announcement.popup-seen.v1
```

保存：

```text
announcement_id
notification_revision
```

仅是 Presentation De-dupe，不是 Read Authority。

Popup Dismissed：

```text
!= Announcement Read
```

关闭 Popup 不写 `announcement_reads`。

---

# 393. Logged-in Read / Detail

```http
POST /api/v1/announcements/{announcement_id}/reads
```

Body：

```json
{"notification_revision": 3}
```

Unique：

```text
(user, announcement, notification_revision)
```

Duplicate 返回原 Read。

Announcement Detail GET 无副作用。

Detail 成功渲染后，Frontend 使用 Response 中当前 Revision 调 `POST /reads`。

如果期间 Re-notify：

```text
old revision may become read
new revision remains unread
```

Public Detail 不返回 Raw Markdown，只返回 Sanitized HTML。

---

# 394. Post-login Popup Safety

```http
GET /api/v1/announcements/current-post-login-popup
```

要求：

```text
Authenticated
Master COMPLETE
Migration Notice complete
Return-to-Intent resolved
Safe Normal Page
```

若：

```text
Active Poker
Active Direct Play Round
Wallet Processing
```

则普通 Popup defer。

上游没有冻结多条 Post-login Candidate 的运营优先级，因此 BFF 保留 `candidates[]`，只使用稳定传输排序：

```text
visible_from ASC
announcement_id ASC
```

Frontend 每次 Safe Entry Flow 最多展示一条，其余保持 Eligible。

该排序不是 Product Priority。

Critical Security / Global Maintenance / Restricted Account / Migration Notice 继续使用专用 Gate，不通过普通 Popup 强制阻断。

---

# 395. Canonical Acknowledgements / Privacy

Canonical Key：

```text
ACKNOWLEDGEMENTS
```

DB 保证非 NULL canonical_key 唯一。

默认业务配置：

```text
Type = ACKNOWLEDGEMENTS
Visibility = PUBLIC

PINNED_LIST = YES
ENTRY_POPUP = YES

POST_LOGIN_POPUP = NO
PUBLIC_HOME_BANNER = NO

visible_until = NULL
```

Dashboard Summary Optional。

IS-08 不 Seed 虚构 Contributor。

Structured Entry 绑定 Content Version：

```text
display_name required
avatar_or_logo_media_id nullable
external_link nullable
acknowledgement_note nullable
group_name nullable
manual_order required
anonymous boolean
```

禁止公开：

```text
payment account
transaction record
Discord User ID
email
payment screenshot
private contact
unconsented real identity
```

头像 / Logo 只用 Managed Media；实名、Logo、External Link 需 Consent Attestation / Audit。

---

# 396. Announcement List / Scheduling

```http
GET /api/v1/announcements
GET /api/v1/announcements/{announcement_id}
```

List Filter：

```text
type
search
date_from
date_to
archive
```

排序：

```text
Pinned:
manual_order ASC

Latest:
publish_at DESC
announcement_id DESC
```

Expired / Archived 不进 Latest。

Scheduler 只执行：

```text
SCHEDULED → PUBLISHED
PUBLISHED → EXPIRED
```

不推送 Popup、不写 Dismissal/Read。

Stable Dedupe：

```text
announcement:publish:{announcement_id}:{content_version}
announcement:expire:{announcement_id}:{notification_revision}
```

---

# 397. Announcement Schedule Atomicity / Missed Window

Scheduling：

```text
BEGIN

validate content
validate placements
lock placement guards
check conflicts

Announcement → SCHEDULED

create Durable Publish Job/Schedule

if visible_until:
  create Durable Expire Job/Schedule

COMMIT
```

Schedule Fact 创建失败：

```text
whole operation rollback
```

不得 UI Scheduled 但无 Durable Schedule。

Scheduling Maintenance / 长故障恢复：

```text
if now < visible_until
→ publish normally

if visible_until exists and now >= visible_until
→ EXPIRED
→ expired_reason = MISSED_PUBLISH_WINDOW
```

不得补弹过期公告。

---

# 398. Durable Job Registry

Job Type 只能来自 Code Registry。

Descriptor：

```text
job_type
owner_service
payload_schema_version
payload_validator
handler
retry_policy_key
affected_maintenance_scopes
manual_cancel_policy
```

Owner：

```text
PLATFORM
POKER
```

未知/Arbitrary Job Type：

```text
reject
```

V1 至少：

```text
RANKING_INCREMENTAL
RANKING_ASSET_SNAPSHOT
RANKING_PUBLISH
RANKING_REBUILD

HISTORY_INDEX_INCREMENTAL
HISTORY_INDEX_REBUILD

ANNOUNCEMENT_PUBLISH
ANNOUNCEMENT_EXPIRE

REWARD_RECOVERY
ECONOMY_RECONCILIATION

GAME_ROUND_RECOVERY
BLACKJACK_AUTO_RESOLVE

POKER_RECOVERY
POKER_FAIRNESS_RELEASE
POKER_AUTO_SAFE_LEAVE
POKER_EMPTY_TABLE_CLOSE
POKER_AUTH_CONTROL_REVOKE
```

---

# 399. `ops.jobs` Runtime Refinement / Dedupe

`000026` 增加/确认：

```text
owner_service
payload_schema_version
payload_hash

due_at

attempt_count
current_attempt_no
current_run_started_at

lease_owner nullable
lease_token UUID nullable
lease_expires_at nullable

next_attempt_at nullable

last_error_category nullable
last_error_detail nullable

target_business_id nullable
```

Payload 是 Typed JSONB，必须 Schema Validate。

Unique：

```text
(job_type, dedupe_key)
```

同 Dedupe 同 Payload：

```text
return original Job
```

同 Dedupe 不同 Payload：

```text
JOB_DEDUPE_CONFLICT
```

不得覆盖旧 Payload。

---

# 400. Job State Machine

```text
SCHEDULED
→ PENDING
→ RUNNING
→ SUCCEEDED
```

Retry：

```text
RUNNING
→ RETRY_WAIT
→ PENDING
```

Maintenance：

```text
SCHEDULED/PENDING/RETRY_WAIT
→ BLOCKED_MAINTENANCE
→ PENDING
```

无法自动安全完成：

```text
→ NEEDS_ATTENTION
```

安全取消仅允许 Registry 标记 `manual_cancel_policy=SAFE` 的 Pending 类型。

Accepted Critical Recovery Job 不允许 generic cancel。

---

# 401. Job Claim / Lease / Heartbeat

Claim：

```text
BEGIN

SELECT eligible due jobs
WHERE owner_service = worker owner

FOR UPDATE SKIP LOCKED

state = RUNNING
attempt_count++
current_attempt_no = attempt_count
current_run_started_at = DB now()

lease_owner = worker_id
lease_token = UUIDv7
lease_expires_at = DB now() + lease_duration

COMMIT
```

Worker 只能 Claim 自己 Registry 中拥有的 Job Type。

Config：

```text
JOB_LEASE_DURATION
JOB_HEARTBEAT_INTERVAL
JOB_DISPATCH_BATCH_SIZE
= UNRESOLVED_IMPLEMENTATION_CONFIG
```

Validator：

```text
HEARTBEAT_INTERVAL < LEASE_DURATION / 2
```

Heartbeat / Completion 必须匹配：

```text
job_id
lease_owner
lease_token
state=RUNNING
```

失租后旧 Worker 不得写终态。

---

# 402. Append-only Job Runs

`ops.job_runs` 继续 Append-only。

因此 Claim 时不创建需要后续 UPDATE 的 Mutable Run。

Current Attempt Start Fact 保存在 `ops.jobs`：

```text
current_attempt_no
current_run_started_at
lease metadata
```

正常结束：

```text
INSERT one complete immutable job_runs row
```

Worker B 发现 Worker A Lease 过期时，同一 Claim TX 先：

```text
INSERT job_runs
result = LEASE_EXPIRED
old attempt_no / worker / start
finished_at = DB now()
```

再 Claim 下一 Attempt。

旧 Run 永不 UPDATE / DELETE。

---

# 403. Target-idempotent Completion / Retry

场景：

```text
Target Business Effect COMMIT
→ worker crashes
→ Job still RUNNING
→ lease expires
→ retry
```

新 Attempt 必须查询 Stable Target Business Identity。

若 Effect 已存在：

```text
do not execute again
→ mark Job SUCCEEDED
```

Job Delivery = at-least-once。

Domain Effect = effectively/exactly-once according to Domain Contract。

Retry：

```text
exponential backoff
+
jitter
```

Config：

```text
JOB_RETRY_INITIAL
JOB_RETRY_MAX
JOB_RETRY_JITTER_RATIO
JOB_MAX_AUTOMATIC_ATTEMPTS
= UNRESOLVED_IMPLEMENTATION_CONFIG
```

超过安全边界：

```text
NEEDS_ATTENTION
```

Handler Result 必须 Typed：

```text
SUCCESS
RETRYABLE_DEPENDENCY
RETRYABLE_CONFLICT
BLOCKED_MAINTENANCE
NONRETRYABLE_INVALID_STATE
NEEDS_ATTENTION
```

---

# 404. Job Schedules / Occurrence Identity

`ops.job_schedules`：

```text
schedule_id
schedule_key

job_type
payload_schema_version
payload

schedule_kind

next_due_at
interval_seconds nullable

enabled

created_at
updated_at
```

V1：

```text
ONE_SHOT_AT
FIXED_INTERVAL
```

不开放 arbitrary shell cron。

每 Due Occurrence：

```text
scheduled_for
```

Dedupe：

```text
schedule:{schedule_id}:{scheduled_for UTC RFC3339Nano}
```

Scheduler TX：

```text
lock schedule
insert occurrence job idempotently
advance next_due_at / disable one-shot
COMMIT
```

Ranking Routine 使用 FIXED_INTERVAL，并由 `RANKING_ROUTINE_INTERVAL` 满足 <=5m 产品目标。

---

# 405. Jobs + Maintenance / Poker DB Boundary

每 Job Descriptor 声明：

```text
affected_maintenance_scopes
```

示例：

```text
RANKING_INCREMENTAL
→ continue under Rankings Publishing maintenance

RANKING_PUBLISH
→ blocked

ANNOUNCEMENT_PUBLISH
→ blocked under Announcements Scheduling

ANNOUNCEMENT_EXPIRE
→ may execute visibility cleanup

REWARD_RECOVERY
ECONOMY_RECONCILIATION
POKER_RECOVERY
→ accepted-work recovery continues
```

Maintenance Block 不消耗 Handler Attempt。

`chaldea_poker` 不获得 arbitrary `ops.*` writes。

新增 Narrow Functions：

```text
ops.poker_job_enqueue(...)
ops.poker_job_claim(...)
ops.poker_job_heartbeat(...)
ops.poker_job_complete(...)
```

仅允许 Code-allowlisted `POKER_*` Job Types。

---

# 406. DB Time / Manual Job Controls

以下业务时间 Authority：

```text
Job Due
Schedule Due
Ranking Period
Announcement Visibility
Publish / Expire
```

全部使用：

```text
PostgreSQL now()
```

Manual Operations Domain 只允许：

```text
View
Retry
Resume
Cancel safe pending job
Create approved Rebuild
Mark Needs Attention
```

禁止：

```text
arbitrary payload editing
change job_type
arbitrary command
fake SUCCEEDED
delete failed job_run
```

完整 Operations Permission 进入 IS-09。

---

# 407. Metrics

Rankings：

```text
ranking_last_source_scan_at
ranking_last_publish_at
ranking_aggregation_lag_seconds
ranking_data_completeness
ranking_source_fact_conflict
ranking_rebuild_running
ranking_publish_failure
ranking_asset_snapshot_failure
```

History：

```text
history_index_lag
history_index_rebuild_count
history_source_mismatch
record_access_denied
fairness_access_denied
```

Announcements：

```text
announcements_by_state
entry_popup_active_count
home_banner_active_count
announcement_scheduler_lag
announcement_publish_failure
announcement_placement_conflict
content_sanitization_failure
notification_revision_count
renotify_count
```

Jobs：

```text
job_pending_count
job_running_count
job_retry_wait_count
job_needs_attention_count
job_oldest_pending_age
job_lease_recovery_count
job_dedupe_conflict
```

Alert Threshold 继续 `UNRESOLVED_IMPLEMENTATION_CONFIG`。

---

# 408. IS-08 Test Gate

## Rankings

```text
same source scanned 100x → one fact
same identity/different hash → conflict/no overwrite
Direct Play refund → no fact
Poker Active Session → no public release
Poker Cash Out → held facts eligible
Poker Profit period = cashout time
Poker Hand event time preserved
```

Period：

```text
Asia/Shanghai 23:59:59 / 00:00
Sunday → Monday boundary
```

Tie：

```text
100/90/90/80 → 1/2/2/4
```

Zero metric omitted；Negative Profit retained。

Asset：

```text
any required source unavailable
→ no new Total Assets publish
→ Last Good remains
```

RP：

```text
same logical request + 10 retries → one logical count
Purpose changes do not rewrite history
General/Unclassified excluded
invalid key probe excluded
charged failure counts credits
public response no sensitive fields
```

Repair：

```text
source exclusion
→ shadow diff
→ no public change before publish
```

## History

```text
delete history_index → rebuild
wrong index payload → source detail still correct
Retired Game / Closed Table readable
wrong user denied
Ops cannot bypass Poker reveal
RP requests absent
```

## Announcements

```text
content-only → same notification revision
renotify → revision++
overlap Entry / Banner rejected
same dismissed revision not reopened
Popup dismissal != Read
Entry fail-open
scheduler failure past visible_until → invisible
missed window → no surprise popup
```

Sanitizer attack vectors：

```text
script
onclick/onerror
javascript:
data:
iframe/object/embed
raw HTML
scheme-relative URL
remote Markdown image
```

不得进入 Public HTML。

## Jobs

```text
two workers → one lease
wrong lease token → rejected
old worker after takeover → cannot complete

worker crash
→ LEASE_EXPIRED run appended
→ resume

target effect commit + crash
→ no duplicate effect

maintenance block
→ no attempt consumed
→ safe resume

unsafe cancel rejected
job_runs update/delete rejected
```

---

# 409. Codex IS-08 Implementation Order

```text
01 lock goldmark / bluemonday

02 migration 000024
03 migration 000025
04 migration 000026

05 Job Registry
06 Job Repository
07 Claim / Lease / Heartbeat
08 Append-only Run Logic
09 Retry / Maintenance
10 Schedule Engine

11 Ranking Period
12 Ranking Source Fact / Cursor
13 Direct Play Scanner
14 Poker Session Scanner
15 Poker Hand Eligibility Scanner
16 RP Attribution Source Boundary

17 Aggregate Builder / Hash
18 Current Asset Snapshot
19 Publish Pointer
20 Shadow Rebuild / Exclusion / Diff
21 Rankings BFF

22 History Cursor / Indexer
23 History List / Detail / Auth

24 Announcement Markdown / Sanitizer
25 Content / Notification Revision
26 Placement / Schedule Guard
27 Entry Popup / Read / Post-login
28 Acknowledgements / Media Safety
29 Publish / Expire Jobs

30 Public Event disabled writer

31 Metrics / Audit Hooks

32 Ranking Tests
33 History Tests
34 Announcement Tests
35 Job Crash / Lease Tests
```

---

# 410. IS-08 Prohibited Implementation

禁止：

```text
manual score editing
rank from cache authority
publish incomplete Total Assets

guess old RP purpose
count provider retries
expose raw API error

read History Detail from index
bypass Poker privacy via History

fake Recent Wins
select Public Records without policy

render raw Markdown HTML
allow JS/iframe/event handler
remote Markdown image

content typo update increments notification_revision
popup dismissal marks read

overlapping Entry Popup publish

in-memory goroutine as sole job truth
Redis as job queue authority
arbitrary job type/payload
update/delete old job_runs
stale lease worker completes job
Rebuild Job auto-publishes Repair
```

---

# 411. IS-08 Acceptance Criteria

```text
AC-08-01  Ranking is rebuildable projection only
AC-08-02  cursor + unique fact prevents double count
AC-08-03  source hash conflict never overwrites
AC-08-04  only SETTLED Direct Play contributes
AC-08-05  Poker formal ranking waits Cash Out
AC-08-06  Poker Profit period uses Cash Out
AC-08-07  Poker Hand event/eligibility times separated
AC-08-08  returned uncalled not wagered
AC-08-09  Total Assets current snapshot only
AC-08-10  incomplete asset authority cannot publish
AC-08-11  Asia/Shanghai day/week exact
AC-08-12  RP explicit activation only
AC-08-13  SV-12 remains RP source blocker
AC-08-14  logical retries do not multiply
AC-08-15  RP credits actual settled credit only
AC-08-16  stable Chaldea Model ID for model filter
AC-08-17  versioned immutable Aggregate Set
AC-08-18  rank semantics 1/2/2/4
AC-08-19  zero primary metric omitted
AC-08-20  current Master identity in Ranking
AC-08-21  event-time identity in Public Record
AC-08-22  no sensitive API detail in public ranking
AC-08-23  routine target <=5m
AC-08-24  repair Shadow/Diff/Review
AC-08-25  exclusion never deletes source
AC-08-26  publish pointer atomic
AC-08-27  closed daily/weekly snapshots retained
AC-08-28  ranking maintenance keeps last good

AC-08-29  History Index rebuildable
AC-08-30  Detail reads Domain Source
AC-08-31  History owner/scope enforced
AC-08-32  Poker reveal survives History auth
AC-08-33  no public private-history share/export

AC-08-34  Public Records disabled without policy
AC-08-35  no fake public game event

AC-08-36  announcement/content/notification identities separate
AC-08-37  Content Version immutable
AC-08-38  content-only never renotify
AC-08-39  renotify increments revision
AC-08-40  visibility evaluates DB state/time live
AC-08-41  Entry overlap rejected
AC-08-42  Primary Banner overlap rejected
AC-08-43  anonymous dismissal Browser-local
AC-08-44  dismissal != read
AC-08-45  read revision-specific/cross-device
AC-08-46  post-login popup defers critical flows
AC-08-47  one canonical Acknowledgements
AC-08-48  sponsor privacy/consent enforced
AC-08-49  Markdown render + allowlist sanitize
AC-08-50  raw HTML/JS/iframe/event cannot reach Browser
AC-08-51  remote Markdown image disabled
AC-08-52  schedule state + durable job atomic
AC-08-53  scheduler outage cannot expose expired content

AC-08-54  jobs code-allowlisted typed only
AC-08-55  dedupe prevents duplicate logical jobs
AC-08-56  SKIP LOCKED + lease token safe claim
AC-08-57  job_runs immutable
AC-08-58  lease-expired attempt preserved
AC-08-59  target retry converges
AC-08-60  bounded retry → Needs Attention
AC-08-61  maintenance only blocks declared jobs
AC-08-62  accepted recovery never generic-cancelled
AC-08-63  DB Time is scheduler authority
AC-08-64  manual job control cannot become shell/SQL
```

---

# 412. IS-08 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| IS-FRZ-357 | IS-08 Rankings/History/Announcements/Jobs 均继续运行于既有 Platform/Poker Service；PostgreSQL 是 Ranking/History/Announcement/Job Durable Authority，Redis 不成为第二真相。 | FROZEN |
| IS-FRZ-358 | Platform Backend 锁 `goldmark v1.8.5` 与 `bluemonday v1.0.26` 用于 Controlled Markdown→Sanitized HTML。 | FROZEN |
| IS-FRZ-359 | IS-08 仅新增 `000024/000025/000026` Forward Migration，不修改此前已冻结 Migration。 | FROZEN |
| IS-FRZ-360 | Rankings 永久遵循 Durable Source→Derived Fact→Aggregate Set→Published Pointer，任何 Ranking Table 都不是可人工修改 Score Authority。 | FROZEN |
| IS-FRZ-361 | `ranking.source_facts` 增加 deterministic dimension_key/source_payload_hash/eligibility/source-observed metadata；同 Source 同 Fact 重扫幂等。 | FROZEN |
| IS-FRZ-362 | 已存在 Source Fact 若 Payload Hash 冲突不得 UPDATE，必须产生 `RANKING_SOURCE_FACT_CONFLICT` 并阻止不可信发布。 | FROZEN |
| IS-FRZ-363 | Ranking Scanner 使用 `(source_timestamp, stable source_id)` Durable Cursor；Fact 已提交但 Cursor 未推进时重扫通过 DB Unique 收敛。 | FROZEN |
| IS-FRZ-364 | Direct Play 只有 SETTLED Round 生成 Profit/Wagered/Positive Biggest Win Fact；Refunded/Cancelled 不贡献排名。 | FROZEN |
| IS-FRZ-365 | Poker Session Profit 仅在最终 Cash Out/SETTLED 后生成，并严格按 Cash Out Time 归入周期。 | FROZEN |
| IS-FRZ-366 | Poker Hand Wager/Biggest Win 保存 Hand settled_at 作为事件时间、Parent Session cashout_at 作为 eligibility_at；Session Cash Out 前不得发布。 | FROZEN |
| IS-FRZ-367 | Poker Wagered 固定使用 participant total commitment 减 RETURN_UNCALLED，确保同一 Chip 只统计一次。 | FROZEN |
| IS-FRZ-368 | Total Assets Ranking 只构建 Current Complete Aggregate Set；每用户复用 IS-05 Strict Snapshot，任一 Required Authority 不完整则整批不发布。 | FROZEN |
| IS-FRZ-369 | Ranking Period 固定 Asia/Shanghai Daily / Monday Weekly / Feature-activation All-time；Total Assets 只支持 CURRENT。 | FROZEN |
| IS-FRZ-370 | RP Ranking 只有在 SV-12 Verified 且 `RP_RANKINGS` feature_activation_at 正式设置后启用，永不回溯猜测旧 Key Purpose。 | FROZEN |
| IS-FRZ-371 | RP Source 固定 Finalized Attribution + logical_request_id + purpose snapshot + model + final status/error/actual credit；具体 NewAPI Feed 继续 SV-12 Blocked。 | FROZEN |
| IS-FRZ-372 | RP 每合格 Logical Request 最多生成 Attempt/Success/Error/Credit Fact；Provider/Internal Retry 不增加 Logical Count。 | FROZEN |
| IS-FRZ-373 | RP Errors 继续按 Error Count 排名，Credits 只用 Final Actual Settled API Credit；公共输出不暴露 Raw Error/Request/Key/Provider。 | FROZEN |
| IS-FRZ-374 | RP Aggregate 同一 Aggregate Set 同时保存 model_id=NULL All Models 与 per-model rows；Model Filter 使用稳定 Chaldea Model ID。 | FROZEN |
| IS-FRZ-375 | Ranking Aggregate Set 使用 BUILDING/SHADOW/PUBLISHED/SUPERSEDED/FAILED，保存 Source Watermark 与 deterministic Aggregate Hash。 | FROZEN |
| IS-FRZ-376 | 排名 `rank_no` 使用 SQL RANK() 等价 1/2/2/4；主指标恰好 0 的行不进入榜单，负 Profit 仍是真实有效值。 | FROZEN |
| IS-FRZ-377 | Ranking 显示 Current Master Profile；Recent/Featured Public Event 继续使用 event-time Identity Snapshot，真实归属均为 newapi_user_id。 | FROZEN |
| IS-FRZ-378 | Public Ranking BFF 只返回聚合公开字段与自己的 My Rank，不提供他人 API Usage Cross-link 或私人请求信息。 | FROZEN |
| IS-FRZ-379 | Routine Ranking 使用 Durable Fixed-interval Job，精确 Interval 进入 Implementation Config，但 Production 必须满足 <=5min 聚合目标。 | FROZEN |
| IS-FRZ-380 | Ranking Repair 固定 Authoritative Full Rebuild→SHADOW→Diff→Human Review→separate Publish；Rebuild Job Success 永不自动发布。 | FROZEN |
| IS-FRZ-381 | Source Exclusion 只标记并审计，不删除 Source/Fact；Create/Revoke 后都通过新 Shadow Rebuild 修复。 | FROZEN |
| IS-FRZ-382 | Ranking Published Pointer Swap 必须单 PostgreSQL Transaction 完成 old supersede/new publish/pointer/audit，Reader 只读取单一 Aggregate Set。 | FROZEN |
| IS-FRZ-383 | Closed Daily/Weekly Snapshot 永久保留；Repair 创建新 Version，旧 Published Version 保留 Audit；Total Assets 不建立历史回放。 | FROZEN |
| IS-FRZ-384 | Rankings Publishing Maintenance 可继续 ingestion/build/validation，但阻断 Pointer Swap，Public 继续 Last Good Snapshot + Stale Metadata。 | FROZEN |
| IS-FRZ-385 | Rankings BFF 固定 current/snapshot Contract；Total Assets 只允许 CURRENT，Model Filter 只允许 RP Metrics。 | FROZEN |
| IS-FRZ-386 | `games.history_index` 永远是可重建 List Projection；新增 history ingestion cursor，默认记录为 Direct Play Round + Poker Session。 | FROZEN |
| IS-FRZ-387 | `/history` 使用 Record Type/Mode/Game/Time/Result/Status/ID Filter 与稳定 Keyset Cursor；Poker Hand 默认从 Session Detail 进入。 | FROZEN |
| IS-FRZ-388 | Round/Session/Hand Detail 必须分别回读 Game/Poker Typed Durable Source + Economy/Fairness，History Index Payload 永远不成为 Detail Authority。 | FROZEN |
| IS-FRZ-389 | Private History 仅 Owner/Authorized Records Scope；即使管理员有 Records 权限也不得绕过 Poker Hole-card/Fairness Reveal Boundary。 | FROZEN |
| IS-FRZ-390 | Retired Game/Closed Poker Table 历史及 Cross-link 永久保留；V1 无公共完整历史 Share、CSV、JSON Export，RP 请求不进入 Game History。 | FROZEN |
| IS-FRZ-391 | `content.public_game_events` Production Writer 继续 Disabled；`PUBLIC_RECORD_SELECTION_POLICY` 未解决前 Public Recent Wins/Featured 模块直接隐藏且不得造假。 | FROZEN |
| IS-FRZ-392 | Announcement Sanitizer 固定 `announcement-sanitize-v1`，采用 goldmark→bluemonday allowlist；Public 永不渲染未经 Sanitization 的 Raw Markdown/HTML。 | FROZEN |
| IS-FRZ-393 | Sanitizer v1 只开放基础 Markdown 语义与安全 http/https/same-site Link；Raw HTML/JS/Event/iframe/data/scheme-relative URL 禁止，Markdown Remote Image 禁用。 | FROZEN |
| IS-FRZ-394 | Announcement Identity 永久区分 announcement_id/content_version/notification_revision；Popup Identity 固定 announcement_id+notification_revision。 | FROZEN |
| IS-FRZ-395 | 所有内容变化创建 Immutable Content Version；Update Content Only 更新 current content pointer 但不改变 notification_revision/read/dismiss semantics。 | FROZEN |
| IS-FRZ-396 | Re-notify 原子递增 Notification Revision，可同时携带新 Content Version，并重新产生 Popup/Unread 资格；必须 Impact Preview + Audit。 | FROZEN |
| IS-FRZ-397 | Announcement 生命周期固定 Draft/Scheduled/Published/Expired/Archived + audited withdrawn_at；用户可见性每次按 DB State/Visibility/Time/Withdraw 实时计算。 | FROZEN |
| IS-FRZ-398 | Placement 保持 Pinned/Entry/Post-login/Home Banner/Dashboard 独立；新增 Schedule Guard 序列化并强制 Entry Popup 与 Primary Home Banner 非重叠。 | FROZEN |
| IS-FRZ-399 | Entry Popup 仅匿名 `/` 或 `/login` 主动检查，失败 Fail-open；同一 Revision Browser-local Dismissal 后不重复主动展示。 | FROZEN |
| IS-FRZ-400 | Popup Dismissed 永不等于 Read；登录用户 Read 使用 `(user,announcement,notification_revision)` exactly-once，并在 Detail 成功渲染后通过 POST Read 写入。 | FROZEN |
| IS-FRZ-401 | Post-login Popup 必须在 Master/Migration/Return Intent 后，只在 Safe Normal Page 展示；Active Poker/Round/Wallet Processing 时 defer。 | FROZEN |
| IS-FRZ-402 | 同一入口流程使用 non-sensitive Browser Session Seen Marker 避免 Entry 已展示的 Revision 登录后立即重复；它不建立 Read Authority。 | FROZEN |
| IS-FRZ-403 | Canonical Acknowledgements 使用唯一 `canonical_key=ACKNOWLEDGEMENTS` 长期公告，默认 Public+Pinned+Entry，无 Post-login/Home、无结束时间，不 Seed 虚构 Contributor。 | FROZEN |
| IS-FRZ-404 | Acknowledgement Contributor 绑定 Content Version，公开字段严格受控；支付/交易/Discord/email/私密身份不可公开，头像/Logo 只用 managed media。 | FROZEN |
| IS-FRZ-405 | Announcement Schedule State、Placement Conflict Validation 与 Durable Publish/Expire Schedule 必须同 PostgreSQL Transaction；缺 Schedule Fact 时不能显示 Scheduled。 | FROZEN |
| IS-FRZ-406 | Announcement Publish/Expire Job 使用稳定 Biz/Dedupe ID；Scheduler 故障后已错过 visible window 的公告直接 Expired/Missed，不补弹。 | FROZEN |
| IS-FRZ-407 | Durable Job Type 必须 Code Registry 定义 owner_service/payload schema/handler/retry/maintenance/cancel policy；未知或 arbitrary command Job 永远拒绝。 | FROZEN |
| IS-FRZ-408 | `ops.jobs` 使用 type+dedupe Unique、Typed Payload Hash、Due/Attempt/Lease Metadata；相同 Dedupe 不同 Payload 返回冲突，不覆盖。 | FROZEN |
| IS-FRZ-409 | Job Claim 固定 `FOR UPDATE SKIP LOCKED` + owner_service + UUID lease_token；Heartbeat/Completion 必须匹配 token，旧 Worker 失租后不能写终态。 | FROZEN |
| IS-FRZ-410 | `ops.job_runs` 保持 Append-only：Claim Start 保存在 jobs 当前尝试字段，完成或 Lease Recovery 时一次性 INSERT immutable Run，不 UPDATE 旧 Run。 | FROZEN |
| IS-FRZ-411 | Target Effect Commit 后 Worker Crash 必须通过 Stable Business Identity 查询原 Effect 后收敛 SUCCEEDED；Job at-least-once 不得造成重复资产/发布/结算。 | FROZEN |
| IS-FRZ-412 | Job Retry 固定 bounded exponential backoff+jitter，精确 Lease/Heartbeat/Retry/Max Attempts 留 Implementation Config；超过安全边界进入 NEEDS_ATTENTION。 | FROZEN |
| IS-FRZ-413 | `job_schedules` 使用 Code-owned ONE_SHOT_AT/FIXED_INTERVAL + stable occurrence dedupe；每种 Job 声明 Maintenance Scope，Accepted Recovery Job 不允许 generic cancel。 | FROZEN |
| IS-FRZ-414 | IS-08 Production Gate 必须通过 Ranking Exactly-once/Period/Privacy/Shadow Publish、History Rebuild/Auth、Announcement Revision/Popup/Sanitizer/Schedule、Job Lease/Crash/Maintenance 与 Public-record Hidden-until-policy Tests。 | FROZEN |

---

# 413. Open / Blocked Register after IS-08

```text
NewAPI:
SV-01 ～ SV-16
= BLOCKED_BY_NEWAPI_SOURCE_VERIFY

SV-12
= RP Attribution Production Source Blocker

Poker:
POKER-PROD-GAP-01 ～ 05
= OPEN

Poker Production Ruleset
= CONFIG_INCOMPLETE / NOT PRODUCTION READY

Reward:
Hourly / Relief / Product Maintenance / Future Amount Policy / Alert Threshold
= OPEN as previously recorded

Public Records:
PUBLIC_RECORD_SELECTION_POLICY
= UNRESOLVED
Production Writer
= DISABLED

Deployment:
DEPLOYMENT-VERIFY-01
= PENDING

IS-08 Implementation Config:
RANKING_ASSET_SNAPSHOT_MAX_SOURCE_AGE
RANKING_ASSET_SNAPSHOT_MAX_BUILD_SKEW
RANKING_ROUTINE_INTERVAL
RANKING_SOURCE_SCAN_BATCH_SIZE

JOB_LEASE_DURATION
JOB_HEARTBEAT_INTERVAL
JOB_DISPATCH_BATCH_SIZE

JOB_RETRY_INITIAL
JOB_RETRY_MAX
JOB_RETRY_JITTER_RATIO
JOB_MAX_AUTOMATIC_ATTEMPTS

IS-08 metric alert thresholds
= UNRESOLVED_IMPLEMENTATION_CONFIG
```

没有 Product OPEN、Source Fact 或 Deployment Gate 被默认值替代。

---

# 414. Change Log — WORKING v0.8

## Added

- 用户正式确认 `IS-08 — Rankings / History / Announcements / Jobs Implementation Specification`；
- 冻结 `IS-FRZ-357 ～ IS-FRZ-414`；
- 冻结 Ranking Source Fact / Cursor / Source Hash Conflict；
- 冻结 Direct Play / Poker / RP Ranking Commit Boundary；
- 冻结 Current Complete Total Assets Aggregate；
- 冻结 Asia/Shanghai Period / RP Feature Activation；
- 冻结 Aggregate Set / Hash / RANK() / Atomic Pointer；
- 冻结 Shadow Rebuild / Diff / Source Exclusion；
- 冻结 Ranking Publish Maintenance / Snapshot API；
- 冻结 Unified History Index / Cursor / Detail Source Authority；
- 冻结 History Authorization / Retention；
- 冻结 Public Record Hidden-until-policy；
- 冻结 goldmark / bluemonday；
- 冻结 Announcement Sanitizer v1；
- 冻结 Announcement ID / Content Version / Notification Revision；
- 冻结 Content-only / Re-notify；
- 冻结 Announcement Lifecycle / Placement / Popup / Read；
- 冻结 Canonical Acknowledgements / Contributor Privacy；
- 冻结 Schedule State + Durable Job Atomicity；
- 冻结 Durable Job Registry / Dedupe / Lease / Heartbeat；
- 冻结 Append-only Job Run Recovery；
- 冻结 Bounded Retry / Schedules / Maintenance；
- 冻结 IS-08 Test / Production Gate。

## Preserved

```text
TD-FRZ-001 ～ TD-FRZ-552
IS-FRZ-001 ～ IS-FRZ-414

SV-01 ～ SV-16 unresolved
Reward Product OPEN
Poker Product Gap 01～05
Poker Production Ruleset CONFIG_INCOMPLETE
PUBLIC_RECORD_SELECTION_POLICY unresolved
DEPLOYMENT-VERIFY-01 pending
Implementation-only config unresolved values
```

## Supersession

```text
Existing IS-FRZ superseded:
None
```

---

# 415. Next Batch

> **IS-09 — Operations / RBAC / Audit / Maintenance Implementation Specification**

IS-09 将落实：

```text
Admin Principal
Super Admin / Operator / Auditor

Module Scope
Permission Matrix
authz_epoch

Operations Search
Attention Items
Incident
Support Case

Level 1 / 2 / 3 Operation Engine
Prepare / Impact Preview / Typed Confirmation / Execute

Economy Adjustment
Ranking Repair Publish
Discord Rebind
Access Control Change
Poker Emergency Pause

Append-only Audit
Before / After
Secret Redaction

Maintenance
Scope Guards
Accepted-work Protection

Job Manual Controls
Service Health
```

---

# 416. IS-09 — Operations / RBAC / Audit / Maintenance Implementation Specification

> 状态：`FROZEN`  
> 用户确认：`整体按上述 IS-09 方案通过`  
> Frozen Decision Range：`IS-FRZ-415 ～ IS-FRZ-482`  
> NewAPI Admin Detection：`SV-07 = BLOCKED_BY_NEWAPI_SOURCE_VERIFY`  
> Discord Binding Write：`SV-04 = BLOCKED_BY_NEWAPI_SOURCE_VERIFY`  
> Password Recovery Capability：`SV-03 = BLOCKED_BY_NEWAPI_SOURCE_VERIFY`

## 416.1 Purpose

IS-09 将 Chaldea Operations 落成：

```text
Admin Principal
Super Admin / Operator / Auditor

Operator Module Scope
Code-owned Permission Matrix
authz_epoch

Global Search
Needs Attention
Incident
Support Case

Level 1 / 2 / 3 Operation Engine
Prepare / Execute
Impact Preview
Typed Confirmation
Fresh Authentication

Access Control
Economy Adjustment
Discord Rebind
Ranking Repair Publish
Poker Emergency Pause

Append-only Audit
Before / After Redaction

Service Health

Maintenance
Scope Guard
Scheduled Activation / End
Accepted-work Protection

Manual Job Control
```

本批继续保持 NewAPI Admin 与 Chaldea Operations 双后台独立，不提供任意 SQL/Shell/Redis/VPS 控制面。

---

# 417. Operations Authority Topology

```text
NewAPI Authentication
        ↓
stable newapi_user_id
        ↓
ops.admin_principals
        ↓
Base Role
Operator Scopes
authz_epoch
status
        ↓
Code-owned Permission Engine
        ↓
Code-owned Operation Registry
        ↓
Risk Guard
        ↓
Domain Admin Contract
        ↓
Business Mutation
+
Append-only Audit
```

NewAPI 只提供身份；Chaldea Operations 权限 Authority 属于 `chaldea_platform` DB。

NewAPI Admin 与 Chaldea Super Admin：

```text
no automatic mapping
in either direction
```

---

# 418. Backend Package Layout

```text
backend/internal/operations/
├── service.go
├── authz/
│   ├── principal.go
│   ├── roles.go
│   ├── scopes.go
│   ├── permissions.go
│   └── epoch.go
├── operation/
│   ├── registry.go
│   ├── prepare.go
│   ├── execute.go
│   ├── preview.go
│   ├── confirmation.go
│   └── recovery.go
├── accesscontrol/
├── search/
├── attention/
├── incident/
├── support/
├── health/
├── maintenance/
├── audit/
└── projection/
```

禁止：

```text
operations/arbitrary
operations/console
operations/rawsql
```

---

# 419. IS-09 Forward Migrations

新增：

```text
000027__operations_rbac_and_operation_refinements
000028__operations_attention_support_health_refinements
000029__operations_maintenance_and_audit_refinements
```

不修改此前已冻结 Migration。

---

# 420. Admin Principal / Operator Scopes

`ops.admin_principals` 至少：

```text
admin_principal_id
newapi_user_id

base_role
status

authz_epoch
version

created_at
created_by
updated_at
updated_by
disabled_at
disabled_by
```

Base Role 固定：

```text
SUPER_ADMIN
OPERATOR
AUDITOR
```

V1 一个 Principal 同时一个 Base Role。

Status：

```text
ACTIVE
DISABLED
```

V1 Operator Scope 固定：

```text
MODELS
USERS_IDENTITY
GAMES
POKER
REWARDS
RANKINGS
RECORDS
ANNOUNCEMENTS
```

不新增普通 Operator：

```text
ECONOMY
ACCESS_CONTROL
AUDIT_ADMIN
SYSTEM
DATABASE
INFRASTRUCTURE
```

Scope。

---

# 421. Code-owned Permission Registry

Backend 注册固定 Permission Keys，例如：

```text
models.read
models.metadata.write
models.publish

users.read
users.master.moderate
users.support.write

games.read
games.metadata.write
games.runtime.write
games.config.draft
games.config.validate

poker.read
poker.accepting_players.write
poker.recovery.request
poker.chat.moderate

rewards.read
rewards.claim.retry
rewards.config.draft

rankings.read
rankings.rebuild

records.read
records.incident.create

announcements.read
announcements.write
announcements.publish

economy.read
economy.reconciliation.write
economy.adjust

access_control.read
access_control.write

audit.read

operations.health.read
operations.jobs.read
operations.maintenance.read
```

数据库无法通过新增 arbitrary Permission String 获得新执行能力。

SUPER_ADMIN 获得全部 Chaldea Code-owned 权限，但仍不能读取秘密或使用基础设施 Console。

AUDITOR 只读。

OPERATOR 按 Assigned Scope 获得 Code-owned 子集。

Cross-domain Safe Summary Read 不等于目标域 Write 权限。

---

# 422. Server-side Authorization / authz_epoch

所有 Ops REST / Admin Command / Manual Job Control 必须：

```text
load current principal
verify ACTIVE
verify role
verify scopes
verify permission
verify authz_epoch
verify target/domain constraints
```

Frontend Sidebar / Disabled Button / Route Guard 只作为 UX。

Current Authority：

```text
ops.admin_principals.authz_epoch
```

Role / Scope / Enable 状态变化：

```text
authz_epoch++
```

旧 Operations Authorization：

```text
403 AUTHORIZATION_STALE
```

普通用户前台 Session 默认不撤销。

---

# 423. Access Control Guard / Last Super Admin

新增：

```text
ops.access_control_guards
```

固定一行：

```text
guard_key = ADMIN_PRINCIPAL_SET
```

所有管理员集合 Mutation：

```text
create admin
change role
disable/enable admin
change scopes
```

先：

```text
SELECT guard FOR UPDATE
```

再锁 Target Principal。

任何 Disable / Demote / Remove 必须验证：

```text
active_super_admin_count >= 1
```

否则：

```text
LAST_SUPER_ADMIN_REQUIRED
```

Access Control：

```text
SUPER_ADMIN
LEVEL_3_CRITICAL
```

Mutation 同事务：

```text
lock guard
lock target
revalidate actor/version
validate last-super-admin
apply role/status/scopes
target.authz_epoch++
append role history
append Audit
COMMIT
```

---

# 424. NewAPI Admin Separation

Ops Bootstrap 可以投影：

```text
newapi_admin_capability:
  AVAILABLE
  NOT_AVAILABLE
  UNKNOWN
```

Concrete Detection：

```text
BLOCKED_BY_SV-07
```

Chaldea SUPER_ADMIN 不能用于推测 NewAPI Admin。

---

# 425. Risk Levels / Operation Registry

Risk 固定：

```text
LEVEL_1_ROUTINE
LEVEL_2_IMPACTFUL
LEVEL_3_CRITICAL
```

Code-owned Operation Descriptor 至少：

```text
operation_type

risk_level
required_permission
allowed_roles

target_schema
input_schema_version
input_validator

impact_builder

requires_reason
requires_fresh_auth
confirmation_mode

supports_schedule

executor
recovery_handler

audit_serializer
```

Client 不能提交 risk/permission/fresh-auth requirement 来降低 Guard。

---

# 426. Level 1 / 2 / 3 Guards

Level 1：

```text
Permission
Target Version
Business Validation
Audit
```

仍产生稳定 `operation_id`。

Level 2：

```text
Server Impact Preview
Explicit Confirmation
Audit
```

Level 3：

```text
Fresh Auth <=10m
Required Reason
Typed Confirmation
Impact Preview
Unique Operation ID
Append-only Audit
```

Level 3 包括：

```text
Economy Adjustment
Discord Rebind
Access Control
CHALDEA_USER_WRITES Maintenance
WALLET_EXCHANGE Maintenance
Poker Emergency Pause
Economic Config Activation
Critical Ranking Repair Publish
Manual Financial Compensation
```

---

# 427. Operation BFF / Durable Record

固定：

```http
POST /api/v1/ops/operations
GET  /api/v1/ops/operations/{operation_id}
POST /api/v1/ops/operations/{operation_id}/execute
```

Prepare 由 Server 计算 Risk / Permission / Impact / Confirmation。

`ops.admin_operations` 补齐：

```text
input_schema_version
input_payload
input_hash

environment

target_version_snapshot

impact_schema_version
impact_preview
impact_hash

confirmation_mode
confirmation_challenge_hash

failure_code
scheduled_for
state_version
```

创建后以下 Immutable：

```text
operation_type
actor identity
actor role/scope snapshot
target
input
environment
risk
```

---

# 428. Operation Input / Impact Hash

Input：

```text
CHALDEA-OPS-INPUT-V1
```

Impact：

```text
CHALDEA-OPS-IMPACT-V1
```

均使用 Versioned Canonical Encoding + SHA-256。

Client Execute 只返回：

```text
operation_id
impact_hash
confirmation
```

不能提交新的 Server Authority Snapshot。

Impact 至少：

```text
target
environment
current state
proposed change
affected users/items
before/delta/after
blocking facts
continuing accepted work
related operation/incident/job
unavailable measurements
```

不可用数据：

```text
UNAVAILABLE
```

而不是 0。

---

# 429. TOCTOU / Fresh Auth / Typed Confirmation

Execute：

```text
lock target
re-read facts
re-check permission
re-check authz_epoch
re-check constraints
rebuild material impact
```

Impact 变化：

```text
409 PREVIEW_STALE
```

无业务效果；重新 Prepare 新 Operation。

Fresh Auth：

```text
10 minutes
```

在 Execute 时再次验证。

过期：

```text
FRESH_AUTH_REQUIRED
```

完成 Fresh Auth 后可重试同 Operation，但仍重验 TOCTOU。

Confirmation Mode：

```text
NONE
EXPLICIT
TYPED
```

Production Level 3 Phrase：

```text
PRODUCTION <operation-code> <target-locator>
```

Challenge 绑定 Operation / Target / Environment。

数据库只存 Phrase Hash，不保存用户失败输入。

---

# 430. Environment Binding / Operation State

Environment：

```text
DEVELOPMENT
STAGING
PRODUCTION
```

来自部署配置。

Operation 创建保存 Environment Snapshot。

Execute 必须：

```text
operation.environment == runtime environment
```

否则：

```text
OPERATION_ENVIRONMENT_MISMATCH
```

Admin Operation State：

```text
PREPARED
→ AUTHORIZED
→ EXECUTING
→ SUCCEEDED
```

异常：

```text
FAILED_NO_EFFECT
RECOVERING
NEEDS_REVIEW
```

Critical Retry 复用原 `operation_id`。

---

# 431. Same-DB / Cross-DB Operation Atomicity

Same-DB：

```text
BEGIN

lock Operation
lock Target

authorize
TOCTOU validate
confirmation validate

Operation → AUTHORIZED
Operation → EXECUTING

Business Mutation
Audit Insert

Operation → SUCCEEDED

COMMIT
```

Audit Failure：

```text
whole mutation rollback
```

Cross-DB：

```text
no fake cross-db transaction
```

逻辑子状态：

```text
TARGET_VALIDATING
REMOTE_EFFECT_EXECUTING
REMOTE_EFFECT_CONFIRMED
CHALDEA_EFFECT_APPLYING
```

顶层仍为 `EXECUTING / RECOVERING / NEEDS_REVIEW`。

Unknown Remote Result：

```text
query authoritative state
```

禁止 blind retry。

---

# 432. Discord Rebind / Legacy Recovery

Discord Rebind：

```text
DISCORD_REBIND
SUPER_ADMIN
LEVEL_3_CRITICAL
```

必须：

```text
Support Case = APPROVED
Original ownership verified
New Discord uniqueness verified
Reason
Fresh Auth
SV-04 verified concrete binding capability
```

禁止 Account Merge / Asset Move / API Key Move / History Ownership Rewrite。

Remote identity：

```text
ops:discord-rebind:{operation_id}
```

必须由 source-verified Idempotent Capability / Narrow Bridge 执行。

Remote Binding Confirmed 后：

```text
target security_epoch++
update case
Audit
revoke Poker controls
```

Operation 仅在关键 Control Revocation 收敛后 SUCCEEDED。

Legacy Password Recovery：

```text
BLOCKED_BY_SV-03
```

管理员不得读取/指定/保存最终密码。

Recovery 成功同样：

```text
security_epoch++
→ sessions/control revoked
```

余额 / Keys / History Ownership 不变。

---

# 433. Master Moderation / Support Case

Users & Identity 可以：

```text
MASTER_RENAME_REQUIRED
MASTER_FORCE_RENAME
```

Forced Rename：

```text
LEVEL_2_IMPACTFUL
```

仍使用 IS-04 Nickname Validator，不绕过 Reserved/Unique/Grapheme 规则，不重写历史 Identity Snapshot。

Admin Forced Rename 不伪造用户主动 Rename，也不额外消费用户 7d Rename Cooldown。

Support Lifecycle：

```text
OPEN
→ VERIFYING
→ APPROVED
→ EXECUTED
→ CLOSED

VERIFYING
→ REJECTED
→ CLOSED
```

新增 append-only：

```text
ops.support_verification_facts
```

只保存最小安全验证事实，不保存 Password/Full Key/OAuth Secret。

---

# 434. Incident / Needs Attention

Incident：

```text
OPEN
→ TRIAGED
→ IN_PROGRESS
→ MONITORING
→ RESOLVED
→ CLOSED
```

False Positive 可以提前 CLOSED，但必须 Reason。

`ops.incident_events` Append-only：

```text
STATE_CHANGED
ASSIGNED
COMMENT
OPERATION_LINKED
JOB_LINKED
AUDIT_LINKED
BUSINESS_FACT_LINKED
```

Incident 本身不得编辑业务 Source。

Attention Item 是可重建 Projection。

补齐：

```text
severity
first_seen_at
last_seen_at
occurrence_seq
acknowledged_at/by
resolved_at
resolution_code
target_route
safe_summary
source_fingerprint
```

Identity：

```text
source_type + source_id + reason_code
```

Acknowledge = seen，不等于 fixed。

Issue 消失才 RESOLVED。

同一 Issue 再出现：

```text
occurrence_seq++
state = OPEN
ack cleared
```

新增 Durable Job：

```text
OPS_ATTENTION_REFRESH
```

---

# 435. Attention Severity

固定：

```text
CRITICAL
WARNING
INFO
```

由 Code-owned Reason Registry 决定。

Scanner 只读取 Domain Source 并维护 Projection，不拥有 Domain Mutation 能力。

---

# 436. Global Search

V1 使用：

```text
Permission-filtered Typed Fan-out Search
```

不引入 Elasticsearch。

可定位：

```text
Master Nickname
Short Account ID
newapi_user_id
Discord User ID
API Key ID

Transfer ID
Transaction ID
Round ID

Poker Table / Session / Hand ID

Announcement ID
Config Version
Audit ID
```

不得搜索/返回：

```text
Key Secret
Password / Hash
Prompt / Response
Hole Card
Server Seed
Future Deck
```

每结果：

```text
object_type
safe_title
safe_subtitle
stable_id
safe_status
target_route
required_permission
```

Server 先 Permission Filter + Redaction，再返回。

`OPS_GLOBAL_SEARCH_MAX_RESULTS` 保留 Implementation Config。

---

# 437. Stable Deep Links

保持 IA-12 路由，例如：

```text
/ops
/ops/models/:model_id
/ops/users/:user_id
/ops/games/:game_slug

/ops/poker/tables/:table_id
/ops/poker/sessions/:session_id
/ops/poker/hands/:hand_id

/ops/economy/transfers/:transfer_id
/ops/economy/transactions/:transaction_id

/ops/rewards/claims/:claim_id
/ops/rankings

/ops/records/rounds/:round_id
/ops/records/sessions/:session_id
/ops/records/hands/:hand_id

/ops/announcements/:announcement_id

/ops/operations
/ops/access

/ops/audit/:audit_id
```

复杂 Repair 不只存在临时 Drawer。

---

# 438. Audit Authority / Serializer

正式使用已有 `audit.audit_events`。

每 Admin Write 至少记录：

```text
audit_id
actor_newapi_user_id
actor_role
actor_scopes_snapshot

action
target_type
target_id

before_snapshot
after_snapshot

reason
operation_id
result
occurred_at

related_business_id
request_id
environment
```

每个 Operation Descriptor 必须提供 typed safe `audit_serializer`。

禁止 Reflection Dump 整个 Domain Object。

永久排除：

```text
Password
Password Hash
API Key Secret
OAuth Secret
Prompt / Response
Unrevealed Hole Cards
Unrevealed Seed
Future Deck
```

Serializer 发现 Secret-bearing Type：

```text
AUDIT_SERIALIZATION_REJECTED
```

Mutation 不执行。

---

# 439. Audit Size / Immutability

`OPS_AUDIT_SNAPSHOT_MAX_BYTES`：

```text
UNRESOLVED_IMPLEMENTATION_CONFIG
```

超过限制时使用 bounded safe summary + stable related IDs，不能误导性截断。

Runtime 对 Audit：

```text
INSERT
SELECT
```

禁止：

```text
UPDATE
DELETE
TRUNCATE
```

V1 无业务 TTL 删除。

金融撤销通过新的 Reversal/Compensation/Ledger/Audit 修复。

---

# 440. Domain Operations Boundaries

Economy：

```text
read-only by default
no Operator Economy Scope

Reconciliation:
Retry
Resume
Compensate
Mark for Review

No Force Confirm
No Balance Patch
```

Economy Adjustment：

```text
SUPER_ADMIN
LEVEL_3
```

复用 IS-05 Economy/Ledger/Supply Primitive；普通 Adjustment 不直接修改 Poker In Play。

Games Operator：

```text
Metadata
Catalog
Safe Runtime
Draft
Validate
Preview
```

Economic Config Activation：

```text
SUPER_ADMIN
LEVEL_3
```

由 Server Validator 判定 Economic / Non-economic。

Rewards：

```text
Product OPEN cannot be bypassed
failed Claim cannot become SUCCESS
manual grant → Economy Adjustment
```

Poker：

```text
Stop/Resume Admission
Boundary Remove
Mute
Pause
Recovery
```

无 Stack/Pot/Winner/Deck/Settlement/Secret Edit。

Poker Emergency Pause：

```text
SUPER_ADMIN
LEVEL_3
```

只改变安全运行状态，不改牌局事实。

Rankings：

```text
Operator build/inspect shadow
Critical Repair Publish → SUPER_ADMIN / LEVEL_3
No score editor
```

Records：

```text
Read/Search/Cross-link/Create Incident
No source mutation
```

Announcements 普通 Publish/Re-notify继续 Impactful Workflow并复用 IS-08 Invariants。

---

# 441. Service Health

新增：

```text
ops.service_health
```

字段：

```text
service_key
status
observed_at
last_successful_at
latency_ms
safe_summary
safe_error_category
source_check_version
updated_at
```

Status：

```text
OPERATIONAL
DEGRADED
MAINTENANCE
UNAVAILABLE
UNKNOWN
```

至少覆盖：

```text
CHALDEA_FRONTEND
CHALDEA_BACKEND
NEWAPI_CONNECTIVITY
POKER_SERVICE
POSTGRES_CHALDEA
POSTGRES_NEWAPI
REDIS
ECONOMY_RECONCILIATION
RANKING_AGGREGATOR
ANNOUNCEMENT_SCHEDULER
REWARD_JOBS
```

Health 是 Operational Projection。

Failure 不自动退款/改余额/结算/把未知资产设为 0。

新增：

```text
OPS_HEALTH_REFRESH
```

Job。

Implementation Config：

```text
OPS_HEALTH_REFRESH_INTERVAL
OPS_HEALTH_STALE_AFTER
OPS_HEALTH_CHECK_TIMEOUT
```

---

# 442. Maintenance Durable Authority / Scopes

Authority：

```text
ops.maintenance_windows
ops.maintenance_window_scopes
ops.maintenance_scope_guards
```

PostgreSQL 为唯一真相。

新增：

```text
scheduled_end_at
environment
state_version
```

`estimated_end_at` 仅展示预估。

七种 Scope 固定：

```text
CHALDEA_USER_WRITES
WALLET_EXCHANGE
REWARDS
DIRECT_PLAY_NEW_ROUNDS
POKER_NEW_TABLES_NEW_HANDS
RANKINGS_PUBLISHING
ANNOUNCEMENTS_SCHEDULING
```

NewAPI Model API Maintenance 不属于 Chaldea Maintenance。

---

# 443. Maintenance Scope Concurrency

一个 Maintenance 可选择多个 Scope。

锁顺序：

```text
scope key lexical ASC
```

按相同顺序 `FOR UPDATE` scope guards。

同 Exact Scope：

```text
no overlapping unfinished windows
```

区间：

```text
[start_at, end_at)
```

Open-ended end = +∞。

冲突：

```text
MAINTENANCE_SCOPE_OVERLAP
```

不同 Scope 可并存。

Effective Gate：

```text
union(all ACTIVE window scopes)
```

结束一个 Window 只移除其自身贡献。

---

# 444. Admission Intent Registry / Backend Gate

每个 Mutation Path Code-owned 声明：

```text
new_work_scopes[]
accepted_work_recovery
safe_exit
```

例如：

```text
Master Edit
→ CHALDEA_USER_WRITES

Exchange
→ CHALDEA_USER_WRITES + WALLET_EXCHANGE

Reward Claim
→ CHALDEA_USER_WRITES + REWARDS

Direct Play Create
→ CHALDEA_USER_WRITES + DIRECT_PLAY_NEW_ROUNDS

Poker new Table/Seat/Buy-in/Hand
→ CHALDEA_USER_WRITES + POKER_NEW_TABLES_NEW_HANDS

Ranking Publish
→ RANKINGS_PUBLISHING

Scheduled Announcement Publish
→ ANNOUNCEMENTS_SCHEDULING
```

业务 Accept 前 Server 检查 Effective Gate。

Active：

```text
503 MAINTENANCE_ACTIVE
```

返回安全 scope/message/time/announcement 信息。

Same-DB Platform Mutation 读 Durable Gate。

Poker 通过 Narrow Read Function：

```text
ops.is_maintenance_scope_active(scope)
```

不获得 Maintenance DML。

---

# 445. Accepted-work Protection

| Domain | New Work | Accepted Work | Safe Exit |
|---|---|---|---|
| Wallet | Block | Reconcile | Compensate |
| Rewards | Block Claim | Issuance Recovery | Claim Terminal |
| Direct Play | Block Round | Settle / Recover | Refund if state machine requires |
| Poker | Block Table/Seat/Hand | Finish / Recover | Safe Leave / Cash Out |
| Rankings | Block Publish | Build Continues | Last Good Snapshot |
| Announcements | Block Scheduled Publish | Visibility Enforced | Expire |

Maintenance 永不自动：

```text
refund all rounds
cash out all poker
compensate all transfers
cancel all rewards
```

Domain 原 State Machine 继续。

---

# 446. Maintenance Risk / Lifecycle

Code-owned Risk：

LEVEL_3：

```text
CHALDEA_USER_WRITES
WALLET_EXCHANGE
POKER_EMERGENCY_PAUSE
```

LEVEL_2：

```text
REWARDS
DIRECT_PLAY_NEW_ROUNDS
POKER_NEW_TABLES_NEW_HANDS
RANKINGS_PUBLISHING
ANNOUNCEMENTS_SCHEDULING
```

Multi-scope：

```text
risk = max(selected scopes)
```

所有 Maintenance Mutation 都要求 Fresh Auth；Level 3 继续 Typed Confirmation。

Lifecycle：

```text
DRAFT
→ SCHEDULED
→ ACTIVE
→ ENDING
→ COMPLETED
```

Pre-activation：

```text
SCHEDULED → CANCELLED
```

异常：

```text
ACTIVATION_FAILED
ENDING_FAILED
```

Immediate：

```text
DRAFT → SCHEDULED(now) → ACTIVE
```

---

# 447. Maintenance Schedule / Jobs

Input：

```text
scopes[]
reason

scheduled_start_at
scheduled_end_at

estimated_end_at

announcement_id
critical_notice_id
```

Schedule Transaction：

```text
lock scope guards
validate overlap

create/update maintenance fact

if future:
  create activation schedule/job

if scheduled_end:
  create end schedule/job

Audit

COMMIT
```

Job 创建失败则 Rollback。

Job Types：

```text
MAINTENANCE_ACTIVATE
MAINTENANCE_END
```

Stable Dedupe：

```text
maintenance:activate:{maintenance_id}
maintenance:end:{maintenance_id}
```

到点以 DB Time 驱动并重新验证 Scope/Environment/State。

Activation 失败：

```text
ACTIVATION_FAILED
→ Attention
```

Active Window 不能 Cancel，只能 End。

End 只关闭当前 Window。

---

# 448. Maintenance Impact Preview / Scheduled Critical Operation

Preview 至少：

```text
Scopes
Environment
Current Active Maintenance
Affected Users

Pending Transfers
Active Direct Play Rounds
Active Poker Tables / Hands
Pending Reward Claims

Scheduled Ranking Publishes
Scheduled Announcements

New Work Blocked
Accepted Work Continuing
Safe Exit

Announcement / Critical Notice
```

不可读取指标 = `UNAVAILABLE`。

Scheduled Critical Operation 在 Schedule 创建时完成 Permission/Fresh/Reason/Impact/Confirmation。

到点无需管理员在线，但仍重新验证：

```text
environment
target existence
safety preconditions
maintenance conflict
```

事实变化超出安全范围：

```text
NEEDS_REVIEW
```

不按旧 Preview 强行执行。

---

# 449. Manual Job Controls / No Infrastructure Console

Manual Job Control：

```text
View
Retry
Resume
Cancel safe pending
Create approved Rebuild
Mark Needs Attention
```

统一通过 Operation Registry。

Risk 从目标 Job Descriptor 推导，Client 不可降级 Recovery Job。

Operations 永远不提供：

```text
SSH
Docker Shell
Command Exec
SQL Console
Redis Console
Package Upgrade
VPS Firewall Editor
Arbitrary HTTP Proxy
```

基础设施工具保持外部独立。

---

# 450. Environment Isolation / Bootstrap

Production / Staging / Development 使用独立 service identity / DB config / secret。

Operations 不提供 DB URL / arbitrary server selector。

`GET /api/v1/ops/bootstrap` 返回：

```text
environment

principal:
  role
  scopes
  authz_epoch
  permissions projection

sidebar capabilities

needs_attention_count
service_health_summary

newapi_admin_capability

build_id
server_time
```

它仅是 UI Projection，后续每 Request 仍 Server Authorize。

---

# 451. Operations Read / Write Surface

Reads 至少：

```http
GET /api/v1/ops/search
GET /api/v1/ops/attention-items

GET /api/v1/ops/incidents
GET /api/v1/ops/incidents/{incident_id}

GET /api/v1/ops/support-cases
GET /api/v1/ops/support-cases/{case_id}

GET /api/v1/ops/service-health

GET /api/v1/ops/jobs
GET /api/v1/ops/jobs/{job_id}

GET /api/v1/ops/maintenance-windows
GET /api/v1/ops/maintenance-windows/{maintenance_id}

GET /api/v1/ops/audit
GET /api/v1/ops/audit/{audit_id}

GET /api/v1/ops/admin-principals
```

正式 Writes 全部走 Code-owned Operation Engine。

禁止 arbitrary state PATCH / balance PATCH / SQL endpoint。

---

# 452. Metrics / Critical Alerts

Metrics 至少：

```text
ops_admin_principals_by_role

ops_permission_denied_total
ops_authorization_stale_total

ops_operations_by_level_result
ops_critical_operation_failure

ops_audit_write_failure
ops_audit_events_total

ops_attention_open_by_severity
ops_attention_oldest_open_age

ops_incidents_by_state_severity
ops_support_cases_by_state

ops_maintenance_by_scope_state
ops_maintenance_activation_failure
ops_maintenance_overlap_conflict

ops_health_status_by_service
ops_health_check_age

ops_global_search_denied_result_count

ops_last_super_admin_guard_rejection
ops_preview_stale_total
ops_typed_confirmation_failure
```

Threshold 保持 Implementation Config。

Critical Alert：

```text
Audit failure on Critical mutation
Last Super Admin invariant threatened
Unauthorized successful operation
stale authz_epoch but mutation succeeded
maintenance gate disagreement
accepted economic work blocked
Poker Cash Out blocked by maintenance
secret/unrevealed poker leakage
arbitrary command surface detected
```

---

# 453. IS-09 Test Gate

## Permission / Revocation

```text
Operator wrong/missing Scope → 403
Auditor write → 403
raw HTTP bypass → 403

NewAPI Admin only → no Chaldea Ops
Chaldea Super Admin only → no auto NewAPI Admin

Role/Scope/Disable
→ authz_epoch++
→ old tab rejected immediately
```

## Last Super Admin

```text
two concurrent demote/disable attempts
→ final active super admin count >=1
```

Stale resource version → conflict.

## Critical Operation

```text
expired Fresh Auth → no effect
wrong Typed Phrase → no effect
stale Impact → PREVIEW_STALE
wrong environment → reject
same operation_id → one effect
HTTP response loss → query same operation
Audit insert failure → rollback
```

## Cross-DB Recovery

```text
Discord Rebind unknown remote result
→ query authoritative remote state

remote confirmed
→ security_epoch++

Poker control revoke unavailable
→ RECOVERING

final success only after control revoke convergence
```

## Audit

```text
UPDATE/DELETE/TRUNCATE → DB denied
secret serializer input → reject/redact
compensation keeps original audit + adds new audit
```

## Attention / Incident / Support

```text
same issue scanned 100x → one attention
ack != resolved
disappear → resolve
recur → occurrence++ / reopen

incident events append-only
incident cannot mutate business

Rebind without approved case/ownership/uniqueness/fresh → reject
```

## Domain Boundary

```text
no balance patch
no force transfer confirmed
no reward force success
no Poker winner/stack/deck mutation
no ranking score edit
no history mutation
```

## Health

```text
health failure
→ status only
→ no business mutation
```

## Maintenance

For each Scope：

```text
new work blocked
accepted work continues
safe exit remains
```

Same scope overlap rejected；different scope union works；multi-scope lock order deterministic；end one window leaves others active。

Maintenance never blanket refunds/cashouts/compensates/cancels.

---

# 454. Codex IS-09 Implementation Order

```text
01 migrations 000027 / 000028 / 000029

02 Role / Scope enums
03 Permission Registry
04 Principal Repository
05 authz_epoch checks

06 Access Control Guard
07 Last Super Admin invariant
08 Access Control mutation

09 Operation Registry
10 Operation Prepare
11 Impact Canonical Hash
12 Confirmation
13 Execute / Recovery

14 Audit Serializer
15 Audit Writer

16 Global Search Adapters

17 Attention Projection
18 Attention Scanner

19 Incident
20 Support / Verification Facts

21 Master Moderation

22 Discord Rebind source-blocked executor
23 Legacy Recovery source-blocked executor
24 Security Epoch / Poker Revocation

25 Economy Operations
26 Games Operations
27 Rewards Operations
28 Poker Operations
29 Rankings / Records
30 Announcements

31 Service Health

32 Maintenance Scope Guards
33 Admission Intent Registry
34 Maintenance Create / Schedule
35 Activation / End Jobs
36 Accepted-work Protection

37 Manual Job Operation Adapters

38 Ops BFF

39 Permission Tests
40 Operation Tests
41 Audit Tests
42 Recovery Tests
43 Maintenance Tests
44 Secret-boundary Tests
```

---

# 455. IS-09 Prohibited Implementation

禁止：

```text
map NewAPI Admin to Chaldea Admin
invent Operator Scope
custom arbitrary permission editor
trust frontend permission

skip authz_epoch
disable last Super Admin
client chooses risk/permission

global permanent CONFIRM phrase
execute stale preview

Level 3 without Fresh Auth

business commit then separate audit write
update/delete audit
secret in audit

direct Wallet edit
force Transfer confirmed
force Reward success

edit Poker Stack/Pot/Winner/Deck
peek Hole Cards/Seed

edit Ranking score
edit History source

arbitrary Job type/payload

SQL/Redis/Shell/VPS Console

Health failure changes business

Maintenance auto refund/cashout/cancel

guess NewAPI Discord/Password/Admin implementation
```

---

# 456. IS-09 Acceptance Criteria

```text
AC-09-01  Chaldea Ops authority Chaldea-side
AC-09-02  no automatic NewAPI Admin mapping
AC-09-03  exactly 3 base roles
AC-09-04  exactly 8 Operator scopes
AC-09-05  code-owned Permission Matrix
AC-09-06  every Ops request server-authorized
AC-09-07  authz_epoch immediate revocation
AC-09-08  Ops revocation does not normally log out public user
AC-09-09  Last Super Admin concurrency-safe
AC-09-10  Access Control Super Admin Level3
AC-09-11  Risk server-owned
AC-09-12  Level1 gets operation/audit identity
AC-09-13  Level2 impact+confirm
AC-09-14  Level3 fresh/reason/typed/impact/audit
AC-09-15  Fresh Auth checked at Execute
AC-09-16  Environment deployment-bound
AC-09-17  Operation input immutable/versioned
AC-09-18  deterministic impact hash
AC-09-19  TOCTOU → PREVIEW_STALE
AC-09-20  same operation ID one effect
AC-09-21  same-DB mutation/audit atomic
AC-09-22  cross-DB unknown query-first
AC-09-23  Discord Rebind stays SV-04 blocked
AC-09-24  Legacy Recovery stays SV-03 blocked
AC-09-25  recovery increments security_epoch
AC-09-26  Rebind moves no assets/keys/history
AC-09-27  Master moderation same nickname validator
AC-09-28  minimum safe support evidence
AC-09-29  incident timeline append-only
AC-09-30  incident cannot mutate business source
AC-09-31  attention rebuildable
AC-09-32  acknowledge != resolve
AC-09-33  recurring attention reopens
AC-09-34  global search server permission-filtered
AC-09-35  search never indexes secret/private Poker facts
AC-09-36  stable Operations deep links
AC-09-37  typed safe audit serializer
AC-09-38  Audit no update/delete/truncate
AC-09-39  reversal never deletes history
AC-09-40  Economy no balance patch
AC-09-41  economic game activation Super Admin-only
AC-09-42  Reward OPEN cannot be bypassed
AC-09-43  Poker Ops cannot edit financial/random facts
AC-09-44  Poker Emergency Pause Level3
AC-09-45  no ranking score editor
AC-09-46  Records read-only
AC-09-47  Announcement operations reuse frozen guards
AC-09-48  Service Health projection only
AC-09-49  health failure no business mutation
AC-09-50  PostgreSQL Maintenance authority
AC-09-51  exactly seven Maintenance scopes
AC-09-52  same-scope overlap rejected
AC-09-53  different scopes union
AC-09-54  deterministic multi-scope locks
AC-09-55  Backend enforces maintenance
AC-09-56  all Maintenance mutations Fresh Auth
AC-09-57  scope risk max selected
AC-09-58  CHALDEA_USER_WRITES protects accepted work
AC-09-59  wallet maintenance preserves Saga/cashout
AC-09-60  rewards maintenance preserves issuance
AC-09-61  Direct Play preserves accepted rounds
AC-09-62  Poker preserves active hands/cashout
AC-09-63  Rankings preserves ingestion/build
AC-09-64  Announcement maintenance preserves expiry
AC-09-65  no blanket compensation
AC-09-66  scheduled Maintenance fact/job/audit atomic
AC-09-67  Maintenance end clears own window only
AC-09-68  no infrastructure console
```

---

# 457. IS-09 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| IS-FRZ-415 | Chaldea Operations 权限 Authority 永久位于 Chaldea DB，通过 stable newapi_user_id 关联；NewAPI Authentication 只提供 Identity。 | FROZEN |
| IS-FRZ-416 | V1 Base Role 固定 SUPER_ADMIN / OPERATOR / AUDITOR，一个 Principal 同时只拥有一个 Base Role。 | FROZEN |
| IS-FRZ-417 | V1 Operator Scope 固定 MODELS / USERS_IDENTITY / GAMES / POKER / REWARDS / RANKINGS / RECORDS / ANNOUNCEMENTS，不新增普通 Economy/Access/System Scope。 | FROZEN |
| IS-FRZ-418 | Permission Matrix 固定 Code-owned Registry；数据库 Scope 只能映射 Backend 已注册 Permission，不能创建任意新能力。 | FROZEN |
| IS-FRZ-419 | 所有 Ops HTTP/Command/Job Manual Action 必须服务端重新读取 Principal/Role/Scope/Permission/authz_epoch；Frontend Guard 仅 UX。 | FROZEN |
| IS-FRZ-420 | Role/Scope/Enabled 状态变化必须 `authz_epoch++`；旧 Operations Authorization 下一请求立即 `AUTHORIZATION_STALE`，普通用户站点 Session 默认不撤销。 | FROZEN |
| IS-FRZ-421 | 新增单行 `ops.access_control_guards` 串行化管理员集合变更并保护 Last Super Admin。 | FROZEN |
| IS-FRZ-422 | Access Control 变更固定 Super Admin + Level 3，并在 Guard Lock 下保证最终至少一名 Active Super Admin。 | FROZEN |
| IS-FRZ-423 | NewAPI Admin 与 Chaldea Super Admin 双向不自动映射；Open NewAPI Admin 能力继续 `BLOCKED_BY_SV-07`。 | FROZEN |
| IS-FRZ-424 | Risk 固定 Level1/Level2/Level3，并且由 Code-owned Operation Descriptor 决定，Client 不得降低 Risk/Permission/Fresh Auth。 | FROZEN |
| IS-FRZ-425 | 全部 Admin Write 统一产生 Durable operation_id；Level 1 可在单请求中原子 Prepare+Execute，仍产生 Operation/Audit。 | FROZEN |
| IS-FRZ-426 | Level 2 固定 Server Impact Preview + Explicit Confirmation + Audit；Level 3 固定 Fresh Auth + Reason + Typed Confirmation + Impact + Operation ID + Audit。 | FROZEN |
| IS-FRZ-427 | Operation Registry 固定 Operation Type 的 permission/risk/input/impact/fresh/confirmation/executor/recovery/audit serializer，数据库不得注册 Executor。 | FROZEN |
| IS-FRZ-428 | `ops.admin_operations` 增加 immutable typed input/environment/version/hash 与 state_version，创建后不得修改 Actor/Target/Input/Risk。 | FROZEN |
| IS-FRZ-429 | Operation Input 与 Impact 分别使用 versioned deterministic canonical SHA-256 Hash；Client 不能提交新的 Server Authority Snapshot。 | FROZEN |
| IS-FRZ-430 | Execute 必须重新 Lock/Read/Auth/Validate；关键事实变化返回 PREVIEW_STALE 且不产生业务 Effect。 | FROZEN |
| IS-FRZ-431 | Level 3 Execute 时 Fresh Auth 必须仍在 10m Window；Fresh Auth 过期只拒绝执行，可重新认证后对同 Operation 再校验。 | FROZEN |
| IS-FRZ-432 | Confirmation Mode 固定 NONE/EXPLICIT/TYPED；Production Typed Challenge 绑定 Environment + Operation Code + Target Locator，不使用全站永久 `CONFIRM`。 | FROZEN |
| IS-FRZ-433 | Operation Environment 必须来自部署配置并在 Execute 时匹配 Runtime Environment；Frontend/URL 不能选择数据库或服务器。 | FROZEN |
| IS-FRZ-434 | Same-DB Operations 必须 Business Mutation + Operation State + Audit 同 PostgreSQL Transaction；Audit Insert Failure 整体 Rollback。 | FROZEN |
| IS-FRZ-435 | Cross-DB Critical Operation 使用同 operation_id 的 Durable Recovery State；Unknown Remote Result Query-first，禁止新 Operation ID Blind Retry。 | FROZEN |
| IS-FRZ-436 | Discord Rebind 固定 Approved Support Case + Ownership + New Discord Uniqueness + Super Admin + Reason + Fresh Auth，Concrete Binding Write 继续 SV-04 Blocked。 | FROZEN |
| IS-FRZ-437 | Discord Rebind Remote Effect 使用 stable `ops:discord-rebind:{operation_id}` 等价 Identity，必须由 source-verified idempotent capability/bridge执行。 | FROZEN |
| IS-FRZ-438 | Rebind/Legacy Recovery 成功后 target security_epoch++ 并撤销旧 Poker Control；Operation 仅在关键 Control Revocation 收敛后 SUCCEEDED。 | FROZEN |
| IS-FRZ-439 | Legacy Password Recovery 继续 SV-03 Blocked，管理员永不得查看/指定/保存最终密码；Recovery 不改变资产、Keys 或历史归属。 | FROZEN |
| IS-FRZ-440 | Master Moderation 通过正式 Ops Command；Forced Rename 仍使用 IS-04 Nickname Validator，并不重写 Event Identity Snapshot。 | FROZEN |
| IS-FRZ-441 | Admin Forced Rename 不伪造用户主动 Rename，不额外消费用户 7d Rename Cooldown。 | FROZEN |
| IS-FRZ-442 | Support Case 使用固定状态机并新增 append-only最小 `support_verification_facts`，禁止 Password/Key/OAuth Secret Evidence。 | FROZEN |
| IS-FRZ-443 | Incident 使用 Durable Lifecycle + append-only incident_events；Incident 只能组织调查/Repair Cross-link，本身不得编辑业务 Source。 | FROZEN |
| IS-FRZ-444 | Attention Item 是 source/reason 唯一的可重建 Projection；Acknowledge 仅 seen，Source 消失才 Resolve，再发生时同 Item occurrence_seq++ 并 Reopen。 | FROZEN |
| IS-FRZ-445 | Attention Severity 固定 CRITICAL/WARNING/INFO，由 Code-owned Reason Registry 决定；Scanner 不拥有 Domain Mutation 权限。 | FROZEN |
| IS-FRZ-446 | V1 Global Search 使用 permission-filtered Typed Fan-out，不引入搜索引擎；只查询允许的 Stable IDs/Safe Identity Keys。 | FROZEN |
| IS-FRZ-447 | Global Search 结果必须服务端 RBAC Filter + Redacted DTO；Password/Secret/Prompt/Response/Hole Card/Seed/Future Deck 永不进入 Search。 | FROZEN |
| IS-FRZ-448 | Operations Stable Deep Links 保持 IA-12 路由；复杂 Repair 不允许只存在易失 Drawer。 | FROZEN |
| IS-FRZ-449 | Audit Snapshot 固定由 Operation-specific typed safe serializer 生成，Forbidden Secret-bearing data 触发序列化拒绝而非写入 Audit。 | FROZEN |
| IS-FRZ-450 | `audit.audit_events` 永久 Append-only；Runtime 无 UPDATE/DELETE/TRUNCATE，V1 无业务 TTL 删除。 | FROZEN |
| IS-FRZ-451 | Audit Snapshot 大小采用 bounded safe summary + stable related IDs，精确上限留 Implementation Config；不截断成误导性 Before/After。 | FROZEN |
| IS-FRZ-452 | Economy 默认只读且无 Operator Economy Scope；Reconciliation 仅状态机合法 Retry/Resume/Compensate/Review，永无 Force Confirm/Balance Patch。 | FROZEN |
| IS-FRZ-453 | Economy Adjustment 固定 Super Admin Level 3，并复用 IS-05 Economy/Ledger/Supply Primitive；普通 Adjustment 永不直接修改 Poker In Play。 | FROZEN |
| IS-FRZ-454 | Game Config Activation 由 Server Validator 区分 Economic/Non-economic；Economic Activation Super Admin Level3，普通 Operator 不能自行分类降级。 | FROZEN |
| IS-FRZ-455 | Reward Operations 不允许绕过 Hourly/Relief Product OPEN；失败 Claim 不能改 SUCCESS，人工补发必须进入 Economy Adjustment。 | FROZEN |
| IS-FRZ-456 | Poker Operator 只能使用冻结 Typed Stop/Resume/Boundary Remove/Mute/Pause/Recovery Contract；没有 Stack/Pot/Winner/Deck/Settlement/Secret Edit Surface。 | FROZEN |
| IS-FRZ-457 | Poker Emergency Pause 固定 Super Admin Level 3，不改变 Hand/Pot/Card 事实，恢复继续读取 PostgreSQL Authority。 | FROZEN |
| IS-FRZ-458 | Ranking Operator 只 Build/Inspect Repair Shadow，Critical Repair Publish Super Admin Level3；Records Scope 保持 Read/Search/Incident，不能修改 History。 | FROZEN |
| IS-FRZ-459 | Announcement普通 Publish/Re-notify 继续冻结为 Impactful Workflow，并复用 IS-08 Revision/Placement/Schedule/Audit Invariant。 | FROZEN |
| IS-FRZ-460 | 新增 `ops.service_health` 作为业务健康 Projection；Health Failure 只影响状态/Attention/明确 Admission，不自动修改任何资产或游戏事实。 | FROZEN |
| IS-FRZ-461 | Health Refresh 通过 Durable `OPS_HEALTH_REFRESH` Job，频率/陈旧阈值/Timeout 留 Implementation Config，错误只公开 Safe Category。 | FROZEN |
| IS-FRZ-462 | Maintenance Durable Authority 继续 ops.maintenance_*；新增 scheduled_end_at/environment/state_version，estimated_end 只作展示。 | FROZEN |
| IS-FRZ-463 | Maintenance Scope 精确保持七种，不纳入 NewAPI Model API Maintenance。 | FROZEN |
| IS-FRZ-464 | Multi-scope Maintenance 按 Scope lexical order Lock Guard；同 exact Scope 的重叠未完成 Window 拒绝，不同 Scope 可以并存。 | FROZEN |
| IS-FRZ-465 | Effective Maintenance Gate 永远为全部 ACTIVE Window Scope 的 Union；结束一个 Window 只移除自身贡献。 | FROZEN |
| IS-FRZ-466 | 每个 Mutation Path 通过 Code-owned Admission Intent Registry 声明 new_work_scopes/accepted_work_recovery/safe_exit。 | FROZEN |
| IS-FRZ-467 | Backend 在业务 Accept 前检查 Maintenance；Platform Same-DB Mutation 读取 Durable Gate，Poker 仅通过 Narrow Read Function 检查相关 Scope。 | FROZEN |
| IS-FRZ-468 | Maintenance Scope Risk 由代码拥有，Multi-scope 取最大值；所有 Maintenance Mutation 均要求 Fresh Auth，Level3 继续 Typed Confirmation。 | FROZEN |
| IS-FRZ-469 | Immediate Maintenance 使用 DRAFT→SCHEDULED(now)→ACTIVE；Scheduled 使用 Durable Activation Job，Lifecycle 保留 Cancel/Failed 状态。 | FROZEN |
| IS-FRZ-470 | Maintenance Schedule/Fact/Scope Conflict/Audit/Activation-End Job 必须同 PostgreSQL Transaction 创建；Job Fact 缺失则 Scheduling Rollback。 | FROZEN |
| IS-FRZ-471 | Maintenance Activate/End Job 使用 stable `maintenance:activate:{id}` / `maintenance:end:{id}`，以 DB Time 驱动并在执行时重新验证 Scope。 | FROZEN |
| IS-FRZ-472 | Maintenance End 只清除当前 Window 对 Effective Union 的贡献；Active Window 不能 Cancel，只能正式 End。 | FROZEN |
| IS-FRZ-473 | Maintenance Impact Preview 必须显示真实受影响工作与“Block/Continue/Safe Exit”，不可读取值必须显示 UNAVAILABLE 而非 0。 | FROZEN |
| IS-FRZ-474 | Maintenance 永不自动 Refund/Cashout/Compensate/Cancel Accepted Work；Transfer/Reward/Round/Poker 等继续原 Domain State Machine。 | FROZEN |
| IS-FRZ-475 | CHALDEA_USER_WRITES/WALLET/REWARD/DIRECT_PLAY/POKER/RANKINGS/ANNOUNCEMENTS 各 Scope Accepted-work Protection 必须通过自动测试。 | FROZEN |
| IS-FRZ-476 | Scheduled Critical Operation 的授权在 Schedule 创建时完成，但到点仍重新验证 Environment/Target/Safety；条件变化时 NEEDS_REVIEW 而非按旧 Preview 强行执行。 | FROZEN |
| IS-FRZ-477 | Manual Job Control 统一走 Operation Registry，Risk 从目标 Job Descriptor 推导；Client 不得将 Recovery Job 降级成普通低风险 Job。 | FROZEN |
| IS-FRZ-478 | Operations 永不提供 SSH/Docker Shell/Command/SQL/Redis/Package Upgrade/Firewall/Arbitrary HTTP Proxy 等基础设施控制面。 | FROZEN |
| IS-FRZ-479 | Production/Staging/Development 使用部署级隔离身份/DB/Secret；Operations 不提供 DB URL/Server Selector。 | FROZEN |
| IS-FRZ-480 | `/ops/bootstrap` 仅返回当前 Admin Projection/Environment/Health/Attention/NewAPI Admin Capability，不能替代后续 Server Authorization。 | FROZEN |
| IS-FRZ-481 | IS-09 Metrics/Critical Alerts 必须覆盖权限绕过、Stale Epoch、Last Super Admin、Audit Failure、Maintenance Disagreement、Accepted-work Block、Secret Leakage 与 Arbitrary Command Surface。 | FROZEN |
| IS-FRZ-482 | IS-09 Production Gate 必须通过 Role/Scope Bypass、Immediate Revocation、Critical Operation Idempotency、Cross-DB Recovery、Audit Immutability/Redaction、Attention/Incident/Support、Maintenance Accepted-work 与 Secret-boundary Tests。 | FROZEN |

---

# 458. Open / Blocked Register after IS-09

```text
NewAPI:
SV-01 ～ SV-16
= BLOCKED_BY_NEWAPI_SOURCE_VERIFY

SV-03
= Password Recovery concrete blocker

SV-04
= Discord Binding concrete blocker

SV-07
= NewAPI Admin detection blocker

Reward:
Hourly / Relief / Product Maintenance / Future Amount / Alert Threshold
= OPEN as previously recorded

Poker:
POKER-PROD-GAP-01 ～ 05
= OPEN

Poker Production Ruleset
= CONFIG_INCOMPLETE / NOT PRODUCTION READY

Public Records:
PUBLIC_RECORD_SELECTION_POLICY
= UNRESOLVED

Deployment:
DEPLOYMENT-VERIFY-01
= PENDING

IS-09 Implementation Config:
OPS_GLOBAL_SEARCH_MAX_RESULTS
OPS_AUDIT_SNAPSHOT_MAX_BYTES

OPS_HEALTH_REFRESH_INTERVAL
OPS_HEALTH_STALE_AFTER
OPS_HEALTH_CHECK_TIMEOUT

IS-09 alert thresholds
= UNRESOLVED_IMPLEMENTATION_CONFIG
```

没有任何 Product OPEN / Source Verification / Deployment Gate 被 Operations 默认值绕过。

---

# 459. Change Log — WORKING v0.9

## Added

- 用户正式确认 `IS-09 — Operations / RBAC / Audit / Maintenance Implementation Specification`；
- 冻结 `IS-FRZ-415 ～ IS-FRZ-482`；
- 冻结 Admin Principal / Base Role / Operator Scope；
- 冻结 Code-owned Permission Registry；
- 冻结 `authz_epoch` Immediate Revocation；
- 冻结 Access Control Guard / Last Super Admin；
- 冻结 Level 1 / 2 / 3 Operation Engine；
- 冻结 Fresh Auth / Impact / Typed Confirmation / TOCTOU；
- 冻结 Same-DB Mutation + Audit Atomicity；
- 冻结 Cross-DB Critical Recovery；
- 冻结 Discord Rebind / Legacy Recovery Security Epoch；
- 冻结 Master Moderation / Support Verification；
- 冻结 Incident / Attention；
- 冻结 Typed Permission-filtered Global Search；
- 冻结 Audit-safe Serializer / Audit Immutability；
- 冻结 Economy / Games / Rewards / Poker / Ranking / Records / Announcement Ops Boundary；
- 冻结 Service Health Projection；
- 冻结七种 Maintenance Scope；
- 冻结 Multi-scope Guard / Scope Union；
- 冻结 Admission Intent Registry；
- 冻结 Accepted-work Protection；
- 冻结 Scheduled Maintenance / Activation / End；
- 冻结 Manual Job Controls；
- 冻结 No Infrastructure Console；
- 冻结 IS-09 Security / Permission / Maintenance Test Gate。

## Preserved

```text
TD-FRZ-001 ～ TD-FRZ-552
IS-FRZ-001 ～ IS-FRZ-482

SV-01 ～ SV-16 unresolved

Reward Product OPEN

Poker Product Gap 01～05
Poker Production Ruleset CONFIG_INCOMPLETE

PUBLIC_RECORD_SELECTION_POLICY unresolved

DEPLOYMENT-VERIFY-01 pending

Implementation-only config unresolved values
```

## Supersession

```text
Existing IS-FRZ superseded:
None
```

---

# 460. Next Batch

> **IS-10 — Frontend Implementation Specification**

IS-10 将落实：

```text
React/Vite Application Shape
Route Modules
Generated BFF Contracts

TanStack Query
Mutation Idempotency
Asset String / BigInt Boundary

Auth Bootstrap
Unified Gate
Return-to-Intent

Direct Play Resume
Poker Realtime Store
Snapshot / Event / Reconnect / Take Over

Operations Frontend Projection

Responsive Shell
Design Tokens
Accessibility / Reduced Motion
Media Loading
Performance Budget
Frontend Security
Testing / Build Release
```

---

# 461. IS-10 — Frontend Implementation Specification

> 状态：`FROZEN`
> 用户确认：`整体按上述 IS-10 方案通过`
> Frozen Decision Range：`IS-FRZ-483 ～ IS-FRZ-548`

## 461.1 Scope

IS-10 正式冻结以下实现契约：

```text
Single React + Vite SPA
Route-level Lazy Chunk
Code-owned Router Metadata
Generated BFF / Poker Contracts
TanStack Query Projection
Mutation Idempotency / Unknown Result Recovery
Asset String / BigInt Boundary
Forms / Safe Draft
Session Bootstrap / Unified Gate / Return-to-Intent
Direct Play Resume-first
Blackjack Authoritative Client
Poker Realtime Store / Snapshot / Event / Reconnect / Take Over
Operations Frontend Projection
Responsive / Design Tokens / Media / A11y
Telemetry / CSP / Cache / Chunk Recovery
Performance / Test / Release Gate
```

Frontend 永远只作为 Projection、Intent Collector 与 Presentation，不成为任何资产、Reward、Game、Poker、Ranking、Permission 或 Maintenance Authority。

---

# 462. Runtime / Dependency Baseline

继续使用：

```text
React 19.2.7
TypeScript 6.0.3
Vite 8.2.0
React Router 8.3.0
TanStack Query 5.102.2
Node 24.20.0
npm 11.19.0
```

新增：

```text
react-hook-form 7.87.0
zod 4.5.4
```

测试：

```text
vitest 4.1.10
@testing-library/react 16.3.3
@testing-library/user-event 14.6.7
jsdom 30.0.1
@playwright/test 1.62.1
@axe-core/playwright 4.13.0
```

继续 exact lock + `package-lock.json` + `npm ci`。

---

# 463. Frontend Application Shape

```text
frontend/src/
├── app/
│   ├── bootstrap/
│   ├── router/
│   ├── gates/
│   ├── providers/
│   ├── errors/
│   └── runtime-config/
├── routes/
│   ├── public/
│   ├── authenticated/
│   ├── games/
│   ├── poker/
│   └── operations/
├── features/
│   ├── auth/
│   ├── master/
│   ├── models/
│   ├── api/
│   ├── wallet/
│   ├── rewards/
│   ├── entertainment/
│   ├── games/
│   ├── rankings/
│   ├── history/
│   ├── announcements/
│   └── operations/
├── realtime/poker/
├── design-system/
├── media/
├── generated/
├── shared/
└── styles/
```

原则：

```text
Route owns composition
Feature owns domain-facing UI behavior
Realtime owns Poker live projection
Generated owns machine contracts
Design System owns tokens/primitives
Shared owns truly generic infrastructure
```

---

# 464. Startup / Runtime Config / Router

启动：

```text
load public runtime config
→ schema validate
→ create BFF client
→ create QueryClient
→ create Router
→ mount
```

Production Runtime Config 只允许：

```text
WEB_ORIGIN
API_ORIGIN
POKER_WS_ORIGIN
BUILD_ID
ENVIRONMENT
public feature availability
```

任何 Secret 禁止进入 Vite/Public Config。

Router 使用唯一 Code-owned Manifest，保存：

```text
route_id
path
PUBLIC / PROTECTED / ADMIN / IMMERSIVE
product_domain
shell_type
safe_parent_route
required_capability
required_ops_permission
lazy_module
```

严格复用 IA FINAL，不建立 `/public` / `/app` / `/mobile` / `/desktop` 第二套路由树。

---

# 465. Lazy Routes / Dynamic Game Runtime

至少 Lazy：

```text
Operations
Poker Table
Dice
Scratch
Summon
Slot
Blackjack
History deep detail
Large Admin Editors
```

Frontend Game Runtime Key 与 IS-06 Backend 完全一致：

```text
direct.dice.v1
direct.scratch.v1
direct.summon.v1
direct.slot.v1
direct.blackjack.v1
```

未知 Key fail closed；数据库不能加载任意客户端脚本。

---

# 466. URL / Navigation State

允许进入 URL：

```text
tab
filter
sort
page
page_size
period
model_id
history filters
ranking metric
search query
```

必须经过 Route-specific Zod Schema。

永久禁止 URL：

```text
Password
OAuth Token / State
CSRF Token
Poker Connect Ticket
Poker Action
Hole Cards
Server Seed
Wallet / Reward Mutation State
Critical Confirmation
API Key Secret
```

List→Detail→Back 只通过 URL + Navigation History 恢复过滤、页码、Scroll、Focus，再重新获取 Server Truth。

---

# 467. Generated Contracts / Runtime Schema Boundary

`src/generated/*` machine-owned，禁止手改。

IS-12 Codegen 输出 API / Poker：

```text
types
schemas
errors
```

Codegen Drift → CI fail。

Runtime Schema Boundary 强制覆盖：

```text
public config
Poker WS frames
Poker snapshots
asset manifest
route params
security-sensitive bootstrap
mutation status envelopes
```

禁止网络 JSON 直接 `as Type`。

---

# 468. BFF / CSRF / Query State

普通 Browser HTTP 统一调用 Chaldea BFF。

Cookie write 自动带：

```text
X-CSRF-Token
```

Token 来自：

```text
GET /api/v1/session/bootstrap
```

只存 Runtime Memory。

TanStack Query 只负责：

```text
read cache
request dedupe
loading
invalidation
background refetch
```

V1 不持久化完整 Query Cache。

Query Key 必须使用 Domain Key Factory；禁止 generic `["data"]`。

Read Retry 只允许 transient network/selected 5xx，不自动 retry 401/403/404/409。

---

# 469. Mutation Idempotency / Unknown Result

每个新用户 Intent 用 Web Crypto 生成：

```text
32-byte random
→ ik1_<base64url-no-padding>
```

同一 Intent 的 Submit / Processing / Unknown / Retry 始终复用原 Key。

统一 UI 状态：

```text
IDLE
SUBMITTING
ACCEPTED
PROCESSING
CONFIRMED
FAILED
RETURNED
NEEDS_ATTENTION
UNKNOWN_RESULT
```

Generic Client 不自动重放 Exchange / Reward / Round / Blackjack / Poker / Admin Mutation。

UNKNOWN_RESULT 必须查询原 Transfer / Claim / Round / Action / Operation / Active Resource。

Reload 后也不得自动 Replay。

---

# 470. No Optimistic Truth / Asset Boundary

禁止未经 Server Confirm 的：

```text
Wallet
Reward
Total Assets
Game Result
Payout
Poker Stack
Pot
Legal Actions
Ranking
Permission
Maintenance
```

资产协议使用 branded String。

精确整数才使用 BigInt。

禁止：

```text
Number(asset)
parseFloat(asset)
Math.round(asset * ratio)
```

唯一 Decimal↔Atomic Parser / Formatter 必须逐位匹配 IS-05：

```text
max 6 fractional digits
micro_units = decimal * 1,000,000
require micro_units % 2 == 0
atomic_units = micro_units / 2
```

不使用 Float。

---

# 471. Forms / Safe Draft / Browser Storage

Form：

```text
React Hook Form
+
Zod
+
Server Validation
+
Domain Error Mapping
```

Client 不替代 Balance / Permission / Eligibility / Maintenance / Version。

SafeDraftRegistry：

```text
default deny
explicit safe fields
SessionStorage only
schema version + TTL
```

永久禁止持久化：

```text
Password
OAuth Token
CSRF Token
Poker Ticket
API Key Secret
Level3 Typed Confirmation
Hole Cards
Unrevealed Seed
Future Deck
```

---

# 472. Session Bootstrap / Unified Gate

唯一 Session Bootstrap：

```text
GET /api/v1/session/bootstrap
```

Public Route 可与 Session Bootstrap 并行读取，Bootstrap 失败不得阻断公开内容。

Protected/Admin：

```text
Route Metadata
→ Session Bootstrap
→ Unified Gate
→ Feature Bootstrap
```

Unified Gate：

```text
Route Classification
→ Entry Popup when applicable
→ Authentication
→ Account Status
→ Master Initialization
→ Migration Notice
→ Role / Scope
→ Resource Availability
→ Return-to-Intent
→ Deferred Post-login Popup
→ READY
```

Return-to-Intent 只恢复安全 Route / Search / Position，永不 replay 副作用。

---

# 473. Error / Loading / Boundary

HTTP：

```text
401 → Reauth + Safe Intent
403 → Access Denied / Safe Parent
404 → privacy-preserving Not Found
409 → recoverable conflict
429 → retryable UX without threshold leak
503 → affected module / maintenance
```

React Boundary：

```text
App Root
Public Route
Protected Route
Game Route
Poker
Operations
```

Poker UI Crash 不得发送 Leave / Cash Out / Result。

Loading 使用 Skeleton/Explicit Loading；Empty 与 Failure 分离；Toast 不作为金融/Critical 唯一成功证据。

---

# 474. Direct Play / Blackjack Frontend

`/games/:game_slug` 使用单一 Bootstrap。

存在 `active_round`：

```text
RESUME first
```

Fast Games：

```text
Authoritative Server Result
!=
Presentation State
```

Skip / Reduced Motion / Media Failure 不发第二次经济请求。

Scratch Canvas/SVG 仅本地表现；九格/奖级/Payout 来自 Server，并有 DOM/Text Equivalent。

Blackjack 只保存 Server Projection，Action 使用：

```text
action_id
expected_round_version
```

不预测 Card / Hand / Dealer / Payout。

Stale 时 Server Snapshot 完全胜出。

---

# 475. Poker Realtime Client

使用：

```text
PokerRealtimeStore
useSyncExternalStore
typed reducer
fine-grained selectors
```

Connection State：

```text
IDLE
CONNECTING
AUTH_PENDING
SYNCING
LIVE
DEGRADED
RECONNECTING
TAKEN_OVER
CLOSED
```

Ticket memory-only；不进入 URL/Storage/Console/Telemetry/Query Cache。

首次/Reconnect：

```text
AUTHENTICATED
→ SYNCING
→ authoritative Snapshot
→ LIVE
```

Snapshot 前禁止 Action。

Store 保存 runtime/event/table/hand/control version。

Duplicate Event ignore；Gap → 禁止行动 + resync。

不本地计算 Stack/Pot/Legal Actions。

---

# 476. Poker Pending Action / Timer / Reconnect / Take Over

发送后断线：

```text
retain original action_id
```

Reconnect + Snapshot 判断是否已 Commit；必要 Retry 复用原 ID。

Timer 只投影 Server `action_deadline_at`；本地归零不发 Check/Fold。

Browser 恢复可见时按 Server State 校准。

Reconnect 使用 bounded exponential backoff + jitter，参数保留 Implementation Config。

Take Over 后旧 Controller 立即 Read-only。

Orientation Change 只改 Layout，不重建 Socket/Session/Hand/Timer。

---

# 477. No Offline Mutation

V1：

```text
NO Service Worker
NO PWA Offline Mutation Queue
```

禁止离线自动排队 Wager / Exchange / Claim / Poker / Admin Mutation。

---

# 478. Responsive / Operations Shell

Breakpoints：

```text
1100
720
420
```

并验证约 320 CSS px / 200% Zoom。

Layout：

```text
Content Max 1200
Gutter 24 / 16 / 14
Page Top 52 / 30
Page Gap 28 / 20
Header 72 / 60
Context Nav 46
Bottom Nav >=70 + safe-area
```

Operations：

```text
Sidebar 252
Topbar 76
<=1100 Sidebar 88
<=720 Drawer
```

Mobile Nav 固定 Dashboard / Models / Entertainment / Wallet / Me。

Poker 始终 Immersive。

---

# 479. Design Tokens / CSS / Fonts

唯一 Token Source：

```text
src/design-system/tokens/
```

生成 CSS Variables / TS Metadata / Test Fixtures。

Button 三色只作用 Button System：

```text
Ivory #F4F0E8
Royal Azure #3568B7
Moonlit Mid #95ACD0
```

不限制全站 Semantic/Data/Domain Colors。

CSS：

```text
CSS Custom Properties
+
CSS Modules
```

不引入 Runtime CSS-in-JS 或第二 Token Authority。

Fonts Self-host WOFF2 + `font-display: swap`；只 preload 首屏 Functional Font。

---

# 480. Media / Asset Manifest / Reduced Modes

媒体与真实 UI 永久分层。

图片/视频不得承载 Balance/Button/Authoritative Result/Private Card/Wager/Menu/Ranking/Login。

所有正式媒体通过 generated Asset Manifest，校验：

```text
source
fallback
geometry
focal point
status
rights
budget
```

Production 禁止 REJECTED / REFERENCE_ONLY / RIGHTS_REVIEW_REQUIRED。

格式与大小预算严格继承 Art Direction FINAL。

Reduced Motion / Reduced Media 都是完整 Production Path。

Media Failure 必须回退到 Glyph/CSS/Static Result/Silent/No-character layout，业务不阻断。

---

# 481. Accessibility / Trusted Rich Text

Production：

```text
WCAG 2.2 AA
```

要求 Semantic HTML First、Keyboard Complete、3px Focus、Focus Trap/Return、200% Zoom、320px Reflow、>=44px Touch、>=46px Input。

Game/Poker Authority 必须有真实 DOM Text Equivalent。

Poker Timer 不每秒 SR 播报。

Announcement Sanitized HTML 只能经 `TrustedRichText`；普通 Feature 禁止 direct `dangerouslySetInnerHTML`。

Poker Chat 纯文本。

---

# 482. Operations Frontend / Safe Telemetry

Ops Bootstrap Projection 仅用于 UX。

`AUTHORIZATION_STALE`：

```text
invalidate/refetch Ops Bootstrap
rebuild navigation/actions
if no rights → leave /ops
```

Level 3 Prepare / Impact / Confirmation 全部 Memory-only；离页销毁，再进入重新 Preview。

Production Telemetry 只允许 Safe Error Metadata，禁止 dump Query Cache / React State / Form / Poker Snapshot / Secret。

---

# 483. CSP / Cache / Chunk Recovery / Performance

Frontend 必须兼容：

```text
script-src 'self'
script-src-attr 'none'
NO unsafe-eval
NO arbitrary third-party script
```

Cache：

```text
index.html → no-cache / must-revalidate
hashed JS/CSS/media → long immutable
```

Dynamic Chunk mismatch：

```text
one controlled full reload
```

最多一次，禁止 reload loop。

Core Web Vitals：

```text
LCP <= 2.5s
INP <= 200ms
CLS <= 0.1
```

Route JS KB 不猜；先 Production-equivalent baseline，再提交 CI Budget。

预算未形成前 `FRONTEND_ROUTE_JS_BUDGET_READY=false`。

---

# 484. Testing / Build Manifest / Release Gate

Unit / Component / Integration / E2E / A11y / Poker Realtime / Visual 全覆盖。

Playwright PR 至少 Chromium Critical Flow；Release 执行 Chromium/Firefox/WebKit Smoke/Critical Navigation。

Axe automation 不替代 Manual Screen Reader Spot Check。

Visual Fixture 使用 demo/fixture data，禁止真实 Secret/Profile/Prompt/Hole Card/Seed。

Build 输出：

```text
dist/frontend-build-manifest.json
```

至少记录 Build ID、Commit、Node/npm、Lock Hash、Generated Contract Hash、Token Hash、Asset Manifest Hash、Route Chunk Inventory、Performance Result、Test Gate Result。

Frontend Release 必须同时 PASS：

```text
TypeScript
Lint
Generated Contract Drift
Tests
Route/Gate
Exact Amount
Mutation Idempotency
Game Resume
Poker Realtime/Take Over
Ops Permission
Responsive
Keyboard/A11y
Contrast
Reduced Motion/Media
Core Web Vitals
Route JS Budget
Asset/Rights
Media Failure
Static Cache/Chunk Recovery
Secret Scan
Fixture Privacy
```

才能 `FRONTEND_PRODUCTION_READY=true`。

---

# 485. IS-10 Implementation Config Register

继续 unresolved：

```text
FRONTEND_READ_RETRY_MAX_ATTEMPTS
FRONTEND_READ_RETRY_BACKOFF_INITIAL
FRONTEND_READ_RETRY_BACKOFF_MAX
FRONTEND_READ_RETRY_JITTER

FRONTEND_SAFE_DRAFT_TTL

POKER_CLIENT_RECONNECT_INITIAL
POKER_CLIENT_RECONNECT_MAX
POKER_CLIENT_RECONNECT_JITTER

FRONTEND_ROUTE_JS_BUDGETS

frontend telemetry sampling / transport limits
```

---

# 486. Codex IS-10 Implementation Order

```text
01 dependency lock / source tree
02 runtime config / generated contracts
03 BFF / CSRF
04 Query / Mutation
05 Amount / Forms / Safe Draft
06 Router / Gate / Return Intent
07 Direct Play / Blackjack
08 Poker Realtime
09 Operations UX
10 Tokens / CSS / Responsive / Fonts
11 Asset Manifest / Media / Reduced Modes
12 Accessibility / TrustedRichText
13 Telemetry / Cache / Chunk
14 Performance Budget
15 Unit / Component / E2E / A11y / Visual
16 Frontend Build Manifest / Production Gate
```

---

# 487. IS-10 Prohibited Implementation

禁止：

```text
second Mobile/Desktop business app
SSR
Query Cache as authority
persistent full Query Cache
JS Number asset math
generic automatic mutation retry
new intent ID after timeout
optimistic financial/game/permission truth
Blackjack prediction
Poker authoritative client calculation
Poker ticket storage
Critical confirmation storage
offline mutation queue
Service Worker
dynamic DB frontend script
third responsive IA
global button-three-color limitation
untokenized UI values
financial UI inside media
rights bypass
business result hidden by media failure
accessibility weakening
raw dangerouslySetInnerHTML
sensitive telemetry dump
unsafe-eval
reload loop
guessed JS KB target
real private fixtures
guess Product OPEN rules
```

---

# 488. IS-10 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| IS-FRZ-483 | V1 Frontend 继续使用单一 React+Vite SPA；Public/Auth/Games/Poker/Operations 通过 Route-level Lazy Module 隔离，不拆第二 App、Mobile App 或 SSR。 | FROZEN |
| IS-FRZ-484 | IS-10 新增锁定 RHF/Zod 运行依赖及 Vitest/Testing Library/jsdom/Playwright/Axe 测试栈，并继续 npm exact lock + npm ci。 | FROZEN |
| IS-FRZ-485 | Router 使用唯一 Code-owned Route Manifest，显式保存 Public/Protected/Admin/Immersive、Safe Parent、Capability、Permission 与 Shell Metadata。 | FROZEN |
| IS-FRZ-486 | Public/Auth 状态继续复用 IA FINAL 同一 Route Tree；登录态只是能力增强，不建立 `/public`/`/app` 两套页面。 | FROZEN |
| IS-FRZ-487 | Frontend Dynamic Game Adapter Key 与 IS-06 Backend `direct.*.v1` implementation_key 完全一致；未知 Key fail closed，不执行动态脚本。 | FROZEN |
| IS-FRZ-488 | 所有可分享 Tab/Filter/Sort/Page/Period/Model/History/Ranking State 必须通过 Route-specific Zod Search Schema 后进入 URL。 | FROZEN |
| IS-FRZ-489 | Password/Token/CSRF/Ticket/Mutation/Critical Confirmation/Hole Card/Seed 等秘密或副作用状态永久禁止进入 URL。 | FROZEN |
| IS-FRZ-490 | List→Detail→Back 固定由 URL + Navigation History 恢复 Filter/Page/Scroll/Focus，只重新 Fetch Server Truth，不缓存旧业务对象为 Authority。 | FROZEN |
| IS-FRZ-491 | Public Runtime Config 必须在 React Mount 前 Runtime Validate；Production BFF 使用 same-origin `/api/*`，任何 Secret 禁止进入 Vite/Public Config。 | FROZEN |
| IS-FRZ-492 | `src/generated` 完全 Machine-owned；IS-12 Contract Codegen 产出 API/Poker Type+Schema+Error，手工修改或 Codegen Drift 必须 CI Fail。 | FROZEN |
| IS-FRZ-493 | Runtime Schema Boundary 强制覆盖 Public Config、Poker Frames/Snapshot、Asset Manifest、Route Params 与 Security-critical BFF Envelope；禁止未经验证的 `as Type` 信任网络 JSON。 | FROZEN |
| IS-FRZ-494 | Browser HTTP 统一通过 single BFF Client；Cookie Write 自动附 Memory-only `X-CSRF-Token`，Browser 不直接调用 NewAPI Internal/DB/Redis。 | FROZEN |
| IS-FRZ-495 | TanStack Query 只作为 Runtime Server Projection；V1 不持久化完整 Query Cache 到 LocalStorage/IndexedDB。 | FROZEN |
| IS-FRZ-496 | Query Key 必须通过 Code-owned Domain Key Factory 生成，不允许无法精确 Invalidate 的 generic `["data"]` Key。 | FROZEN |
| IS-FRZ-497 | Read Retry 仅允许 Network/selected 5xx；401/403/404/409 不 Generic Retry，次数/Backoff/Jitter 继续 Implementation Config。 | FROZEN |
| IS-FRZ-498 | 每个新 Mutation Intent 使用 Web Crypto 32-byte random `ik1_` Base64URL Key，同一 Intent 的 Submit/Unknown/Retry 永久复用原 Key。 | FROZEN |
| IS-FRZ-499 | 建立统一 Durable Mutation UI Controller：IDLE/SUBMITTING/ACCEPTED/PROCESSING/CONFIRMED/FAILED/RETURNED/NEEDS_ATTENTION/UNKNOWN_RESULT。 | FROZEN |
| IS-FRZ-500 | Generic Mutation 永不自动重发；Unknown Result 必须通过原 Transfer/Claim/Round/Action/Operation 或 Domain Active Resource 收敛，Reload 后也不自动 Replay。 | FROZEN |
| IS-FRZ-501 | Wallet/Reward/Game/Poker/Ranking/Permission/Maintenance 禁止未确认 Optimistic Truth；Optimistic State 只允许 Presentation。 | FROZEN |
| IS-FRZ-502 | Asset/Wager/Payout/Stack/Pot 使用 branded String Types，协议/缓存保持 String，精确整数操作才受控 BigInt，JSON 永不 BigInt。 | FROZEN |
| IS-FRZ-503 | Frontend Decimal↔Atomic Parser/Formatter 必须逐位匹配 IS-05 0.000002 Atomic Contract，禁止 Number/parseFloat/舍入。 | FROZEN |
| IS-FRZ-504 | Form 统一 RHF+Zod+Server Validation+Domain Error Mapping；Client Schema 不替代 Balance/Permission/Eligibility/Maintenance/Version Authority。 | FROZEN |
| IS-FRZ-505 | Safe Draft 使用 default-deny Registry、SessionStorage、Schema Version 与 TTL；精确 TTL 保留 Implementation Config。 | FROZEN |
| IS-FRZ-506 | Password/OAuth/CSRF/Poker Ticket/API Secret/Level3 Confirmation/Hole Card/Seed 永不写 Browser Persistent/Session Storage。 | FROZEN |
| IS-FRZ-507 | `GET /api/v1/session/bootstrap` 是唯一 Session/CSRF Bootstrap；Public Route 与其并行加载且 Bootstrap Failure 不阻断公开内容。 | FROZEN |
| IS-FRZ-508 | Protected/Admin Route 必须 Session Bootstrap→Unified Gate→Feature Bootstrap，Gate 完成前绝不 Flash Protected Business Data。 | FROZEN |
| IS-FRZ-509 | 单一 UnifiedGateController 实现 frozen Auth/Account/Master/Migration/Role/Resource/Return Intent/Post-login 顺序，Feature 不复制完整 Gate。 | FROZEN |
| IS-FRZ-510 | Return-to-Intent 只恢复 Server-approved Route/Search/Position，永不 Replay Wager/Buy-in/Cashout/Exchange/Claim/Profile/Password/Admin Mutation。 | FROZEN |
| IS-FRZ-511 | HTTP Domain Error 与 React Render/Chunk Error 分层；部分 Dependency 失败只影响对应模块，不关闭无关产品域。 | FROZEN |
| IS-FRZ-512 | Read UI 使用 Skeleton/Explicit Loading；Empty 与 Failure 分开，Toast 不作为金融/Settlement/Critical Operation 唯一成功证据。 | FROZEN |
| IS-FRZ-513 | `/games/:game_slug` 必须消费单一 Game Bootstrap，存在 active_round 时 Resume-first，不能先展示新下注再异步发现旧 Round。 | FROZEN |
| IS-FRZ-514 | Dice/Scratch/Summon/Slot 的 Authoritative Result 与 Presentation State 分离；Skip/Reduced Motion/Media Failure 不触发第二次经济请求。 | FROZEN |
| IS-FRZ-515 | Scratch Canvas/SVG 仅为本地刮开表现；九格/奖级/Payout 来自 Server，并始终拥有 DOM/Text Equivalent。 | FROZEN |
| IS-FRZ-516 | Blackjack 只保存 Server Projection；Action 使用 UUID + expected_round_version，本地不预测 Card/Hand/Dealer/Payout，Stale 时 Server Snapshot 全量胜出。 | FROZEN |
| IS-FRZ-517 | Poker 使用自定义 typed `useSyncExternalStore` Realtime Store，不将 HTTP Query Cache 当作实时 Authority，并使用细粒度 Selector。 | FROZEN |
| IS-FRZ-518 | Poker Connection State 与 Hand Lifecycle 分离；首次/Reconnect 必须 Snapshot-first，Snapshot 完成前禁止 Player Action。 | FROZEN |
| IS-FRZ-519 | Poker Connect Ticket 只存在内存，发送 first auth frame 后立即释放，禁止 URL/Storage/Console/Telemetry/Query Cache。 | FROZEN |
| IS-FRZ-520 | Poker Store 以 runtime/event/table/hand/control version 校验 Event；Duplicate Ignore，Gap 禁 Action 并强制 Authoritative Snapshot Resync。 | FROZEN |
| IS-FRZ-521 | Poker Pending Action 保留原 action_id；Send 后断线不自动生成新 ID，Reconnect 后由 Snapshot 判断是否已 Commit。 | FROZEN |
| IS-FRZ-522 | Poker Timer 只显示 Server `action_deadline_at`，本地归零不执行 Check/Fold；Browser 恢复可见后按 Server State 校准。 | FROZEN |
| IS-FRZ-523 | Poker Reconnect 使用 bounded exponential backoff+jitter；精确参数继续 Implementation Config，连接未知期间 Action 禁用但最后 Snapshot 可读。 | FROZEN |
| IS-FRZ-524 | Take Over 后旧 Controller 立即 Read-only；Orientation Change 只调整 Layout，绝不重建 Socket/Session/Hand/Timer。 | FROZEN |
| IS-FRZ-525 | V1 明确无 Service Worker/PWA Offline Mutation Queue，所有副作用操作禁止离线缓存后自动提交。 | FROZEN |
| IS-FRZ-526 | Responsive Breakpoint 精确保持 1100/720/420 与约320px Reflow；PC/Mobile 可拆 Presentation 但共享 Hooks/Contracts/Business State。 | FROZEN |
| IS-FRZ-527 | Mobile Navigation 固定 Dashboard/Models/Entertainment/Wallet/My；Direct Play 保留普通 Shell，Poker Table 永久 Immersive。 | FROZEN |
| IS-FRZ-528 | Operations Shell 精确实现 252px Sidebar/76px Topbar、<=1100 88px、<=720 Drawer，不建立移动端另一套运营逻辑。 | FROZEN |
| IS-FRZ-529 | Design Tokens 使用唯一 Versioned TS Source 生成 CSS Variables/Typed Metadata/Test Fixtures，Feature 不允许散落未批准的颜色/Breakpoint/Shadow/Radius。 | FROZEN |
| IS-FRZ-530 | Ivory/Royal Azure/Moonlit Mid 三色纪律严格只约束 Button System；Semantic/Data/Domain Colors 继续读取 Art Direction FINAL。 | FROZEN |
| IS-FRZ-531 | CSS 架构固定 CSS Custom Properties + CSS Modules，避免 runtime CSS-in-JS；不引入第二套 Tailwind Token Authority。 | FROZEN |
| IS-FRZ-532 | 字体继续 Self-host WOFF2 + font-display:swap，只 Preload 首屏 Functional Font，展示/装饰字体按需加载。 | FROZEN |
| IS-FRZ-533 | UI 与媒体永久分层；所有图片/视频不得承载 Balance/Button/Authoritative Result/Poker Private Card/Wager/Menu/Ranking/Login Form。 | FROZEN |
| IS-FRZ-534 | 所有正式媒体必须通过 generated Asset Manifest 解析并通过 Production Status/Rights/Fallback/Focal/Geometry/CI Gate。 | FROZEN |
| IS-FRZ-535 | Media Format/Geometry/Preload/Lazy Load 与全部大小预算严格继承 Art Direction FINAL；核心媒体容器提前保留几何避免 CLS。 | FROZEN |
| IS-FRZ-536 | `prefers-reduced-motion` 与独立 `reduced_media` 都是正式 Production Path；Reduced Media 可本地保存但绝不改变 Business Capability。 | FROZEN |
| IS-FRZ-537 | Persona/Background/Reaction/Audio/Character 各自拥有明确 Fallback；所有视觉媒体失败时业务仍完整可操作。 | FROZEN |
| IS-FRZ-538 | Frontend Production Accessibility 固定 WCAG 2.2 AA、Semantic HTML、Keyboard Complete、3px Focus、Dialog Focus Trap/Return、200% Zoom、约320px Reflow、>=44px Touch。 | FROZEN |
| IS-FRZ-539 | Game/Poker Authoritative Result 必须有真实 DOM Text Equivalent；Async Live Region 只播报必要状态，Poker Timer 不每秒 Screen Reader 播报。 | FROZEN |
| IS-FRZ-540 | Sanitized Announcement HTML 只能通过 `TrustedRichText` Boundary，普通 Feature 禁止直接 dangerouslySetInnerHTML；Poker Chat 始终纯文本。 | FROZEN |
| IS-FRZ-541 | Ops Bootstrap 的 Role/Scope/Permission/authz_epoch 仅用于 UX；`AUTHORIZATION_STALE` 必须刷新 Bootstrap，失去权限后离开 `/ops`。 | FROZEN |
| IS-FRZ-542 | Level 3 Frontend Operation State/Impact/Typed Confirmation 全部 Memory-only，离页销毁，再进入必须重新 Preview。 | FROZEN |
| IS-FRZ-543 | Production Logging/Telemetry 只允许 Safe Error Metadata，禁止 Dump Query Cache/React State/Form/Poker Snapshot/Secret。 | FROZEN |
| IS-FRZ-544 | Frontend 必须兼容 TD-12 no-eval CSP；index.html revalidate + hashed immutable assets；Chunk Mismatch 最多一次 Controlled Reload。 | FROZEN |
| IS-FRZ-545 | Core Web Vitals 固定 LCP≤2.5s/INP≤200ms/CLS≤0.1；Route JS KB 不猜默认，先测 Production-equivalent Baseline 后提交明确 CI Budget。 | FROZEN |
| IS-FRZ-546 | Frontend 测试栈固定 Vitest/RTL/jsdom/Playwright/Axe；必须覆盖 Unit/Component/Integration/E2E/Poker Realtime/A11y/Visual。 | FROZEN |
| IS-FRZ-547 | Visual/A11y/Review Fixture 必须使用 Demo Data，不得包含真实 Secret/Prompt/Private Profile/Account/Hole Card/Seed；Design Token 与 Asset Manifest 都有 CI Gate。 | FROZEN |
| IS-FRZ-548 | IS-10 Frontend Production Gate 必须同时通过 Route/Gate、Exact Amount、Mutation Idempotency、Direct Play Resume、Poker Realtime/Take Over、Ops Permission、Responsive/A11y/Reduced Media、Asset/Rights、Performance/Cache 与 Fixture Privacy；本批不新增 Product OPEN。 | FROZEN |

---

# 489. Open / Blocked Register after IS-10

```text
SV-01 ～ SV-16
= BLOCKED_BY_NEWAPI_SOURCE_VERIFY

Reward Product OPEN
= unchanged

POKER-PROD-GAP-01 ～ 05
= OPEN

Poker Production Ruleset
= CONFIG_INCOMPLETE / NOT PRODUCTION READY

PUBLIC_RECORD_SELECTION_POLICY
= UNRESOLVED

DEPLOYMENT-VERIFY-01
= PENDING

FRONTEND_READ_RETRY_*
FRONTEND_SAFE_DRAFT_TTL
POKER_CLIENT_RECONNECT_*
FRONTEND_ROUTE_JS_BUDGETS
frontend telemetry limits
= UNRESOLVED_IMPLEMENTATION_CONFIG
```

---

# 490. Change Log — WORKING v1.0

## Added

- 用户正式确认 IS-10；
- 冻结 `IS-FRZ-483 ～ IS-FRZ-548`；
- 冻结 Frontend Runtime/Test Dependencies；
- 冻结 Single SPA / Router / Lazy Chunk；
- 冻结 Dynamic Game Adapter 与 Backend Key 一致；
- 冻结 Generated Contracts / Runtime Validation；
- 冻结 BFF / CSRF / Query / Mutation；
- 冻结 Asset String / Exact Amount；
- 冻结 Forms / Safe Draft；
- 冻结 Session Bootstrap / Unified Gate / Return Intent；
- 冻结 Direct Play / Blackjack；
- 冻结 Poker Realtime / Snapshot / Take Over；
- 冻结 No Service Worker / No Offline Mutation；
- 冻结 Responsive / Tokens / CSS / Fonts；
- 冻结 Media / Asset Gate / Reduced Modes；
- 冻结 WCAG 2.2 AA / TrustedRichText；
- 冻结 Ops Permission Projection；
- 冻结 Safe Telemetry / CSP / Cache / Chunk Recovery；
- 冻结 Web Vitals / Measured Route JS Budget；
- 冻结 Frontend Test / Build Manifest / Release Gate。

## Preserved

```text
TD-FRZ-001 ～ TD-FRZ-552
IS-FRZ-001 ～ IS-FRZ-548
SV-01 ～ SV-16 unresolved
Reward Product OPEN
Poker Product Gap 01～05
Poker Production Ruleset CONFIG_INCOMPLETE
PUBLIC_RECORD_SELECTION_POLICY unresolved
DEPLOYMENT-VERIFY-01 pending
Implementation-only Config unresolved
```

## Supersession

```text
Existing IS-FRZ superseded:
None
```

---

# 491. Next Batch

> **IS-11 — Security / Observability / Compose / Backup / DR Implementation Specification**

计划落实：

```text
Reverse Proxy / TLS / Security Headers / CSP
Cookie / CSRF / Origin / Rate Limit
Secrets / Service Assertions
DB Role / Redis Hardening
Structured Logs / Metrics / Request ID / Alerts
Docker Compose Production / Health / Resource Boundary
Deployment / Migration Gate
WAL / PITR / Off-host Backup
Restore / RPO / RTO / DR_RECOVERY_LOCK
Secret Rotation / Backup Drill / Production Security Gate
```

---

# 492. IS-11 — Security / Observability / Compose / Backup / DR Implementation Specification

> 状态：`FROZEN`  
> 用户确认：`整体按上述 IS-11 方案通过`  
> Frozen Decision Range：`IS-FRZ-549 ～ IS-FRZ-624`  
> Deployment Environment：`DEPLOYMENT-VERIFY-01 = PENDING`  
> NewAPI Deployment / Persistent Volume：继续受 `SV-14 / SV-15 / SV-16` 等 Source Verification 约束

## 492.1 Purpose

IS-11 正式冻结：

```text
Edge / TLS / HSTS / CSP / Security Headers
Cookie / CSRF / Request Limits / SSRF / Rate Limit
Platform ↔ Poker Service Assertion
Secret Injection / Key Rotation
PostgreSQL / Redis Hardening
Poker WS Security
Structured Logs / Metrics / Correlation / Alerting / NTP
Docker Compose / Container Hardening / Resources / Graceful Shutdown
Release Manifest / Deploy Gate / Migration / Rollback
pgBackRest / WAL / PITR / Off-host Backup / Logical Backup
Recovery Kit / Retention / Restore Drill
DR_RECOVERY_LOCK / Full DR
Host Compromise / Secret Leak / Supply-chain
```

本批不改变业务状态机，也不新增 Product OPEN。

---

# 493. Infrastructure Tool Lock

```text
pgBackRest       2.59.1
Prometheus       3.13.2 LTS
Alertmanager     0.34.0
node_exporter    1.12.1
age              1.3.2
```

Grafana：

```text
optional
not mandatory baseline
```

Production 最终以不可变 Artifact/Image Digest 为准。

---

# 494. Deployment Repository / Host Layout

Repository：

```text
deploy/
├── compose/
├── edge/
├── observability/
├── security/
├── backup/
├── systemd/
├── profiles/
└── deployctl/
```

Host：

```text
examples/deployment/external-newapi
examples/deployment/platform
```

Chaldea：

```text
examples/deployment/platform/
├── releases/<release_id>/
├── current -> releases/<release_id>
├── config/
├── secrets/
├── runtime/
├── state/
├── manifests/
├── observability/
└── backup-staging/
```

`backup-staging` 仅为 bounded temporary staging，不计正式 DR Backup。

IS-11 不新增业务 Schema Migration；PG/Redis/Edge/Backup 属于 Deployment/DBA Configuration。

---

# 495. Edge / Origin / TLS

实际 Edge 必须由 `DEPLOYMENT-VERIFY-01` 核验为：

```text
NGINX
or
CADDY
```

同一 Production 只允许一个 Active Edge，不得双占 80/443。

Web Origin：

```text
https://<chaldea-web-origin>/

/
→ Frontend

/api/v1/*
→ Platform BFF

/ws/poker
→ Poker WS
```

External Model API：

```text
https://<api-origin>
→ NewAPI
```

External Model API 不接收 Chaldea Browser Session Cookie。

公网业务端口仅：

```text
80
443
```

80 永久跳 HTTPS。

TLS：

```text
1.2
1.3
```

---

# 496. HSTS / CSP / Security Headers

Production 初始 HSTS：

```text
Strict-Transport-Security: max-age=31536000
```

确认相关子域全部稳定 HTTPS 后才加入 `includeSubDomains`；V1 不加入 preload list。

CSP：

```text
default-src 'self';
base-uri 'none';
object-src 'none';
frame-ancestors 'none';
form-action 'self';

script-src 'self';
script-src-attr 'none';

style-src 'self' 'unsafe-inline';

img-src 'self' data: blob:;
font-src 'self';
media-src 'self' blob:;

connect-src 'self';
worker-src 'self';
manifest-src 'self';
```

禁止：

```text
unsafe-eval
arbitrary third-party script
arbitrary iframe
arbitrary inline script
```

Staging 先 Report-Only，Production 再 Enforcement。

Other Headers：

```text
X-Content-Type-Options: nosniff
Referrer-Policy: strict-origin-when-cross-origin
X-Frame-Options: DENY
```

Permissions Policy 默认关闭 camera/microphone/geolocation/payment/usb/bluetooth/serial/accelerometer/gyroscope/magnetometer。

---

# 497. Request / Parser Limits

Edge：

```text
max body             = 512 KiB
max request-target   = 8 KiB
max aggregate header = 32 KiB
```

Backend：

```text
Default JSON             = 64 KiB
Announcement/Ops Content = 256 KiB
CSP Report               = 32 KiB
JSON nesting depth       = 32
```

要求：

```text
bounded body reader
single JSON document
reject trailing non-whitespace JSON
reject malformed payload
```

V1 无普通用户文件上传。

---

# 498. Cookie / CSRF / SSRF

Session：

```text
__Host-chaldea_session
Secure
HttpOnly
SameSite=Lax
Path=/
No Domain
```

Login/OAuth/Identity Recovery 后创建新的 Opaque Session Identity，匿名 Session 不原样提升。

Cookie-auth Write：

```text
SameSite=Lax
+
Synchronizer CSRF
+
Origin Validation
+
Fetch Metadata
```

Credentialed cross-origin BFF CORS 默认拒绝。

Backend Outbound 只允许：

```text
Discord API
Verified NewAPI Internal Endpoint
Approved Backup Endpoint
Approved Alert Sink
```

Announcement External Link 不触发 Backend Fetch。

---

# 499. Rate Limiting

Redis Namespace：

```text
chaldea:ratelimit:*
```

至少覆盖：

```text
Edge IP

Auth IP
Auth Account
Registration
Password Login
Fresh Auth

Poker Ticket
Poker WS Connect
Poker Message/Action

Reward Claim
Game Create Round

Operations Critical Action
```

采用 Redis atomic token/GCRA equivalent。

阈值保持隐藏 Implementation Config。

Limiter Failure：

```text
Public Read
→ bounded local emergency limiter

Login / Registration security /
Fresh Auth / Reset / Critical Ops /
Poker new auth ticket
→ fail closed
```

Accepted Settlement/Recovery 不被 Limiter 阻断。

---

# 500. Platform ↔ Poker Service Assertion

Internal HTTP：

```text
private network
+
Ed25519 signed Service Assertion
```

Headers：

```text
X-Chaldea-Service-Key-Id
X-Chaldea-Service-Issuer
X-Chaldea-Service-Audience
X-Chaldea-Service-Issued-At
X-Chaldea-Service-Expires-At
X-Chaldea-Service-Request-Id
X-Chaldea-Service-Body-SHA256
X-Chaldea-Service-Signature
```

Canonical：

```text
ASCII("CHALDEA-SERVICE-ASSERTION-V1")
0x00

LP16(key_id)
LP16(issuer)
LP16(audience)

U64BE(iat)
U64BE(exp)

LP16(request_id)
LP16(uppercase_method)
LP16(exact_request_uri)

body_sha256[32]
```

`exact_request_uri` = escaped path + exact raw query。

Lifetime：

```text
30s
```

Clock Skew：

```text
±5s
```

Service Assertion 不替代业务 Idempotency。

---

# 501. Service Key Rotation / NewAPI Auth Boundary

Platform/Poker 各自拥有独立 Private Signer + Peer Public Verification Keyring。

Rotation：

```text
install new verifier
→ deploy new signer
→ new signer ACTIVE
→ wait >40s
→ remove old signing capability
```

不共享长期 Bearer Secret。

Chaldea→NewAPI Authentication 继续：

```text
Verified Credential
or
Verified Narrow Bridge
```

不得假设支持 Chaldea Service Assertion。

---

# 502. Secret File Contract

Host Parent：

```text
examples/deployment/platform/secrets
root-owned
0700
```

具体 Secret Source File：

```text
owner root
group = target service runtime GID
mode 0640
```

采用该方式是因为本机 Compose file-backed secret 为 bind mount，不能假定 `uid/gid/mode` 重映射。

Container 使用固定 non-root UID/GID，并在 Release Manifest 保存。

每 Service 只只读挂载所需 `examples/runtime/secrets/<name>`。

Preflight 检查：

```text
source exists
root owner
mode <=0640
group matches runtime GID
no unrelated secret mounted
```

禁止单个 `secret.env` 给所有 Service。

---

# 503. Secret Families / Fairness Key Ring

至少：

```text
platform_db_password
poker_db_password
platform_redis_password
poker_redis_password
discord_oauth_secret
newapi_integration_credential

platform_service_ed25519_private
poker_service_ed25519_private

game_fairness_keyring
poker_fairness_keyring

alert_sink_secret

backup_repo1_cipher_pass
backup_repo2_cipher_pass
backup_provider_credentials
```

Fairness Key Ring：

```text
ACTIVE
DECRYPT_ONLY
RETIRED
```

新加密只用 ACTIVE。

旧 Key 在任何 Live DB Object 或 retained Backup 仍依赖时不得删除。

---

# 504. PostgreSQL / Redis Hardening

PostgreSQL Runtime：

```text
NO SUPERUSER
NO CREATEDB
NO CREATEROLE
NO RUNTIME DDL
SCRAM-SHA-256
restrictive pg_hba
no public port
revoke unnecessary PUBLIC privileges
```

Physical/Logical Backup 使用独立 Infra/Read-only Backup Identity；不提升 App/Poker Runtime 权限。

Redis：

```text
NewAPI namespace
→ source verified

Platform
→ chaldea:session:*
→ chaldea:auth-flow:*
→ chaldea:return-intent:*
→ chaldea:cache:*
→ chaldea:lock:*
→ chaldea:ratelimit:*

Poker
→ chaldea:poker:*
```

使用 ACL + namespace allowlist。

禁止：

```text
CONFIG
FLUSHALL
FLUSHDB
DEBUG
MODULE
```

及未用高危 Admin Commands。

Redis 无公网端口，不承担 Financial/Poker RPO。

---

# 505. Empty Redis Recovery / Poker WS Security

Redis 完全丢失：

```text
sessions/auth-flow/return-intent/cache/presence/event buffer lost
→ users reauth
→ cache rebuild
→ PG lease recovery
→ Poker PG reconstruction
```

Wallet/Stack/Round Result 不从 Redis 恢复。

Poker WS：

```text
max frame = 64 KiB
permessage-deflate = disabled
schema validation
rate limit
bounded send queue

heartbeat target = 20s
stale close = 60s
```

Ticket/Origin/Protocol/Control Epoch继续生效。

---

# 506. Debug / Observability Baseline

Public Edge 不发布：

```text
/debug/*
pprof
/metrics
internal diagnostics
```

pprof 默认关闭；仅 internal、time-bounded、incident-linked 临时开启。

8GB Mandatory：

```text
Prometheus 3.13.2 LTS
Alertmanager 0.34.0
node_exporter 1.12.1
```

Grafana Optional。

node_exporter 作为 Host systemd service，private-only，至少启用 filesystem/memory/load/network/timex。

---

# 507. Structured Logs / Metrics / Correlation

JSON Log：

```text
timestamp
level
service
environment
build_id
event_name

request_id
trace_id

route_template
status_code
duration_ms
safe_error_code

biz_id
operation_id
round_id
hand_id
job_id
```

永不记录：

```text
raw request/response
Authorization
Cookie
CSRF
OAuth
API Secret
Password
Prompt/Response
Seed
Deck
Private Hole Cards
```

Edge Access Log 不默认记录完整 Query String。

Metric Label 只允许低 Cardinality：

```text
service
route_template
method
status_class
game_slug
state
result_class
```

禁止 user/request/round/hand/operation/IP。

Edge 生成 128-bit opaque Request ID：

```text
32 lowercase hex
```

传播 `X-Request-ID`，并保持 W3C Trace Context/OpenTelemetry Compatible。

---

# 508. Health / Alerting / Retention / Time

每服务：

```text
/health/live
/health/ready
/metrics
```

`live` 只表示进程生命，不把外部 Dependency Failure 绑定为必须重启。

至少一个 Out-of-band Alert Sink。

Alert Categories：

```text
SERVICE_DOWN
HIGH_5XX
DB_UNAVAILABLE
BACKUP_STALE
WAL_ARCHIVE_FAILED
RESTORE_CHECK_FAILED
DISK_LOW
MEMORY_PRESSURE
CLOCK_DRIFT
AUDIT_WRITE_FAILED
ECONOMY_INVARIANT_FAILED
RECONCILIATION_STUCK
POKER_ASSET_CONSERVATION_FAILED
POKER_RECOVERY_STUCK
POKER_PRIVACY_BOUNDARY_FAILED
JOB_NEEDS_ATTENTION
RANKING_PUBLISH_STALE
CERTIFICATE_EXPIRY_RISK
```

Alert 使用 fingerprint + FIRING/RESOLVED。

Retention：

```text
runtime logs = 14d target + hard cap
Prometheus 8GB = 30d
Prometheus 4GB = 7d
```

同时设 Retention Size Cap。

Host 只启用 chrony 或 systemd-timesyncd 一套，并监控 sync/drift。

---

# 509. Compose Ownership / Networks

`examples/deployment/platform` 只拥有 Chaldea 自有 Compose Components，不自动接管：

```text
NewAPI
existing Edge
existing PostgreSQL
existing Redis
```

Mandatory Chaldea App Services：

```text
chaldea-frontend
chaldea-platform
chaldea-poker
prometheus
alertmanager
```

Host Infra：

```text
node_exporter
NTP
backup timers
```

Edge Modes：

```text
HOST_EDGE
or
CONTAINER_EDGE
```

Data Modes：

```text
EXTERNAL_DOCKER_DATA_NETWORK
or
HOST_LOCAL_DATA_ENDPOINT
```

均由 Deployment Verify 决定。

逻辑网络：

```text
chaldea_app
chaldea_observability
verified_data_network
optional_verified_edge_network
```

Frontend 不加入 Data Network。

---

# 510. Container Hardening / Runtime Identity

Chaldea-owned Container 默认：

```text
non-root
read_only root filesystem
cap_drop: ALL
no-new-privileges
not privileged
no host network
no Docker socket
read-only secret mounts
tmpfs /tmp
```

需要写入的路径必须显式 Named Volume / tmpfs。

Platform/Poker 使用固定 Runtime UID/GID；Preflight 验证 Secret Group 与 Volume Ownership。

禁止 Production 时临时改 Root 解决权限。

---

# 511. Resource Profile / Swap / Graceful Shutdown

8GB：

```text
reserve ~1–1.5GB host/burst headroom
```

4GB：

```text
lower worker concurrency
7d metrics
no local Grafana
smaller cache
conservative DB tuning
```

精确 CPU/Memory/Worker/PG/Redis/Prometheus 配置必须 Production-like Load Test 后写入 `deploy/profiles/<profile>/resources.env`。

未完成：

```text
RESOURCE_PROFILE_READY=false
```

Emergency Swap：

```text
8GB → 2GB
4GB → 1GB
swappiness ≈ 10
```

持续 Swap = MEMORY_PRESSURE。

Platform/Poker：

```text
stop_grace_period = 30s
```

未完成工作依赖 PG Durable Recovery，而不是 Container Restart。

---

# 512. Release Manifest / Deploy Control

Release Manifest：

```text
release_id
git_commit
build_id
frontend artifact hash
platform image digest
poker image digest
schema migration version/checksum
frontend build manifest hash
asset manifest hash
config manifest hash
runtime UID/GID
observability versions
deployed_at/by
environment
```

禁止 `latest`。

Host-only：

```text
deploy/bin/chaldea-deployctl
```

Commands：

```text
verify-environment
preflight
deploy
postcheck
backup-status
dr-lock enable/status/disable
restore-preflight
restore-verify
release-status
```

不提供公网执行面，也不是 Chaldea Operations Console。

---

# 513. Deployment Categories / High-risk Gate

固定：

```text
FRONTEND_ONLY
BACKEND_COMPATIBLE
SCHEMA_MIGRATION
POKER_RUNTIME
SECURITY_AUTH
ECONOMY_MIGRATION_HIGH_RISK
```

High-risk Preflight：

```text
correct environment
disk
PG health
Redis known
NTP
backup repository
physical backup freshness
WAL <= RPO
migration checksums
current/target release manifest
maintenance impact
no critical invariant
resource profile ready
```

Critical Fail：

```text
do not deploy
```

High-risk Sequence：

```text
Preflight
→ fresh PITR verify
→ Maintenance
→ Expand/Migrate
→ immutable deploy
→ postcheck
→ recovery verification
→ end Maintenance
→ elevated monitoring
```

Chaldea 不自动 Upgrade/Restart NewAPI。

---

# 514. Migration / Rollback / Poker Deploy

Schema：

```text
EXPAND
→ MIGRATE
→ CONTRACT
```

Contract 必须等旧 App 不再使用、Rollback Window 结束、Backup/Migration Verified。

不可逆 Data/Schema 使用 Forward Fix，不伪造 Down Migration。

Application 只有 Schema backward-compatible 才可切回 Previous Digest。

Frontend 至少保留 Previous 2 Releases 且 >=24h Hashed Assets。

Poker Upgrade：

```text
Stop New Hands
→ finish or safe pause active Hand
→ SIGTERM
→ restart immutable image
→ PG actor recovery
→ 30s reconnect grace
→ fresh action deadline
→ Resume
```

不改 Pot/Stack/Card/Hand。

---

# 515. Backup Engine / Integration Gate

Primary：

```text
pgBackRest 2.59.1
stanza = chaldea-cluster-v1
```

目标是整个 PostgreSQL Cluster，确保 `newapi + chaldea_platform` 同一 PITR Timeline。

实际 Integration 必须从：

```text
EXISTING_PGBACKREST
DERIVED_POSTGRES_IMAGE_WITH_PGBACKREST
VERIFIED_HOST_POSTGRES_INTEGRATION
```

三类之一经 `DEPLOYMENT-VERIFY-01 + SV-15` 选择。

未核验前：

```text
BACKUP_INTEGRATION_READY=false
```

不得声称 Production PITR Ready。

---

# 516. WAL / Repository Encryption

目标：

```text
archive_mode = on
archive_command = pgbackrest ... archive-push %p
archive_timeout = 300s
```

精确 Syntax 按实际 PostgreSQL Major 核验。

Repository Encryption：

```text
repo-cipher-type = aes-256-cbc
```

Cipher Pass 使用 Secret File + Recovery Kit。

Transport 使用 TLS。

---

# 517. repo1 / repo2 Schedule

repo1：

```text
encrypted off-host
continuous WAL

Sunday 18:30 UTC → FULL
Mon-Sat 18:30 UTC → DIFFERENTIAL
Daily 21:00 UTC → repository/check verification

retain 8 weekly restore chains
maintain >=30d PITR
```

Host systemd Timer 驱动，不以 App Job Queue 为唯一 Scheduler。

repo2：

```text
First Sunday monthly 19:30 UTC
→ FULL

retain 6 monthly restore points
```

repo2 使用独立 Prefix/Cipher/Credential；Provider 支持时启用 Versioning/Object Lock。

---

# 518. Off-host / Logical Backup

正式 DR 至少一个 genuinely off-host provider。

可使用 pgBackRest 官方支持兼容 Provider；实际 Provider 为 Deployment Config。

Portable Logical：

```text
Daily 20:00 UTC

pg_dump -Fc newapi
pg_dump -Fc chaldea_platform
safe roles/grants manifest
migration manifest
release manifest
```

Logical Dump 不作为 Primary Financial RPO。

使用：

```text
age 1.3.2
```

加密。

Retention：

```text
7 daily
4 weekly
```

上传与校验成功后删除 Local Staging。

---

# 519. Recovery Kit

使用：

```text
age 1.3.2
```

至少：

```text
2 independent offline recovery recipients
```

Private Recipient Identity 不存 Production VPS。

Kit 至少：

```text
Game Fairness Key Ring
Poker Fairness Key Ring

repo1 cipher pass
repo2 cipher pass

backup-provider recovery material

Discord OAuth recovery secret
NewAPI integration recovery credential

other non-regenerable production secrets
```

无需恢复：

```text
old sessions
CSRF
OAuth temporary flow
Poker ticket
Redis cache/presence
```

Service Signing Private Keys 可以 Clean-host DR 时重新生成。

Artifact：

```text
recovery-kit_<timestamp>.tar.age
recovery-kit_<timestamp>.manifest.json
```

Manifest 不保存 Secret 明文 Hash。

---

# 520. NewAPI Non-DB Backup Scope

必须核验 NewAPI：

```text
persistent volume
uploaded assets
runtime config
credential files
other non-reconstructible files
```

继续受 SV-15 / Source Verification 阻断。

未完成前不能声称 PostgreSQL Backup 已覆盖完整 NewAPI DR。

正式 Backup Scope：

```text
PG Cluster
roles/grants
Migration/Release/Cutover manifests
Chaldea config
NewAPI recovery config
Asset/Rights manifest
Recovery Kit
verified non-reconstructible volume
```

---

# 521. Backup Retention / Verification / Freshness

Retention：

```text
repo1 >=30d PITR
8 weekly

repo2 6 monthly

logical 7 daily + 4 weekly
```

删除 Backup 前检查 Historical Fairness Key / Release / Schema Dependency。

Backup Success 必须通过：

```text
repository check
manifest validation
checksum validation
WAL continuity
recoverable WAL age
off-host object presence
logical encrypted artifact checksum
```

不能仅 Exit 0。

Critical：

```text
recoverable WAL age >5m
physical backup overdue
repository check failed
```

Warning：

```text
logical backup overdue
```

Critical Backup Gate 阻止 High-risk Deploy / Cutover。

---

# 522. Restore Drill

固定：

```text
Before initial Production
→ Full DR drill

Monthly
→ isolated PostgreSQL restore

Quarterly
→ Full Application DR drill

After backup/encryption change
→ immediate restore drill
```

Restore Drill：

```text
isolated network
no public route
no real provider call
no Discord mutation
no scheduled business jobs
no Reward issuance
no Announcement publish
no public Poker
```

Production Copy 后续用于 QA 前必须 Anonymize 或 Fixture Conversion。

---

# 523. DR_RECOVERY_LOCK

Deployment-level Host Authority：

```text
examples/deployment/platform/runtime/dr-recovery.lock
```

Active File：

```text
root-owned
0444
```

独立于恢复出来的 Maintenance DB State。

Platform/Poker Read-only mount：

```text
examples/runtime/platform/dr-recovery.lock
```

Active 时阻断：

```text
new Chaldea writes
new Direct Play
new Poker table/seat/hand
Ranking publish
Scheduled Announcement publish
```

---

# 524. DR Edge Gate / Unlock

`deployctl dr-lock enable`：

```text
create Host lock
→ install verified Edge DR deny
→ reload Edge
→ verify user write denial
→ verify external model API charging path denial
```

即使 Platform DB 未启动，DR Gate 仍存在。

Public Read 是否开放由 Incident 决定，但 User Write / External Model API Charging / Poker New Work 在 Authority 验证前必须关闭。

Unlock 必须：

```text
invariant report PASS
backup/release/schema PASS
NewAPI integration PASS
operator approval
```

输入：

```text
restore_id
invariant_report_sha256
```

Typed Prompt：

```text
PRODUCTION DR_UNLOCK <restore_id>
```

之后移除 Edge Deny + Host Lock 并做 Post-unlock Verification。

---

# 525. Full DR Sequence / Invariants

固定：

```text
01 isolate failed host
02 provision clean host
03 pinned Docker/Edge baseline
04 enable DR lock
05 restore Recovery Kit
06 configure pgBackRest
07 restore PostgreSQL selected PITR
08 verify timeline
09 verify roles/grants
10 verify migration checksums
11 start Redis empty
12 deploy immutable services
13 start under DR lock
14 inspect incomplete durable states
15 Economy reconciliation
16 Reward recovery
17 Direct Play recovery
18 Poker recovery
19 Jobs verify
20 Rankings verify
21 Announcements verify
22 NewAPI integration verify
23 global invariant report
24 operator review
25 DR unlock
26 normal traffic
27 elevated monitoring
```

Invariant Report 至少：

```text
no negative wallet
ledger/materialized balance consistent
no duplicate transfer terminal effect
no settlement+refund same round
no duplicate Reward issuance
Poker asset conservation
no duplicate Poker settlement
one Active Poker Session/user
Fairness historical seed decryptable
Migration checksums valid
Schema checksums valid
Ranking pointer valid
Audit readable
NewAPI user/key/quota integration readable
```

任一 Critical Fail → DR Lock 保持。

---

# 526. Failure Recovery Boundaries

Redis Loss：

```text
start empty Redis
→ reauth
→ cache rebuild
→ PG worker/Poker recovery
```

目标：

```text
Redis infrastructure ready <=15m
```

Platform Process Loss：

```text
same immutable digest
→ restart
→ PG durable recovery
```

Poker Process Loss：

```text
restart
runtime_epoch++
same Hand from PG
30s grace
```

VPS/Disk Total Loss：

```text
Clean Host
→ Immutable Release
→ Recovery Kit
→ Off-host PITR
→ Empty Redis
→ DR Lock
→ Verify
```

---

# 527. Host / Backup Compromise

Host Compromise：

```text
isolate old host
preserve evidence
clean host restore
rotate OAuth/NewAPI/DB/Redis/service/backup credentials
revoke sessions
verify fairness keys
deploy pinned artifacts
```

禁止原地清理后继续生产。

Backup Provider Credential Leak：

```text
revoke
rotate
repository integrity verify
```

Repository Cipher Pass Leak：

```text
new repository/prefix
new cipher pass
fresh full backup
verify WAL
switch production backup
retain old read-only only as required
destroy after retention
```

不宣称 pgBackRest Repo Cipher 可安全 In-place Rotation。

age Recipient Rotation：先增加新 Recipient、重加密当前 Kit、验证，再停止未来使用旧 Recipient；历史 Kit 保持原 Recipient Set 至 Retention 结束。

---

# 528. PITR Boundary / Backup Manifest

PITR 只用于：

```text
catastrophic cluster loss
broad corruption
failed deploy before safe reopen
security incident clean restore
```

单条业务错误使用 Incident/Reconciliation/Compensation/Repair。

PITR Decision 必须记录：

```text
restore_id
restore point
source backup
timeline
expected lost window
RPO impact
external effects
reconciliation
authorized operator
```

Physical Backup Manifest：

```text
backup_id
stanza
repository
cluster timeline
start/stop
WAL start/stop
backup type
pgBackRest version
repository cipher key id
checksum/check result
release/schema reference
```

---

# 529. Supply-chain / Host Baseline

Production：

```text
package-lock.json
go.sum
pinned build inputs
immutable image digest
```

禁止：

```text
latest
runtime curl | sh
floating npm install
```

Release Gate：

```text
dependency vulnerability scan
container image scan
secret scan
SBOM
migration checksum
frontend asset/rights gate
```

Critical Vulnerability Block 或显式安全 Review。

SBOM：

```text
Frontend dependencies
Platform Go
Poker Go
Container base image
Observability versions
Backup tool versions
```

绑定 release_id。

Host Runbook：

```text
dedicated admin account
SSH key
root SSH disable after emergency path verify
firewall default deny
security updates
NTP
disk monitoring
```

这些能力不进入 Chaldea Operations UI。

---

# 530. Disk / Preflight / Postcheck / Incident

Disk Monitoring：

```text
PG volume
Docker images/logs
Prometheus data
backup staging
Redis volume if used
root filesystem
```

Disk Low = Critical。

Backup Staging 使用 Hard Quota，上传验证后清理。

High-risk Preflight：

```text
Environment
Disk
DB
Redis
NTP
Backup freshness
WAL
Repository check
Migration checksum
Current/Target Release Manifest
Maintenance
Critical invariants
DR lock
Resource Profile
```

Fail → No Deploy。

Post-deploy：

```text
Frontend/Platform/Poker Build ID
DB migration
live/ready
Session Bootstrap
Public Home
Wallet Read
Reward Status
Game Registry
Job duplication check
Poker recovery/safe status
Audit write
Prometheus scrape
Alertmanager
Backup/WAL freshness
```

Security Incident：

```text
Detect
→ Alert
→ Incident
→ Contain
→ Evidence
→ Revoke/Rotate
→ Recover
→ Verify
→ Reopen
→ Review
```

---

# 531. Observability Data Egress / RPO Breach

Alert Sink / Remote Logs / Metrics Remote Write / Error Reporter / Trace Backend 均视为 External Data Egress。

只允许 Minimum Safe Metadata，不发送 Raw Request/Response/Prompt/Poker Snapshot/Hole Cards/Seed/Secret。

若：

```text
last recoverable WAL age >5m
```

则：

```text
Critical Alert
Needs Attention
High-risk Deploy/Cutover Block
```

不要求自动关闭整个 Public Website。

Backup Failure 不改变 Wallet/Poker/Business State，只是 Critical Operational Condition。

---

# 532. IS-11 Test Gate

必须覆盖：

```text
Edge exposure
TLS / CSP / Cookie / CSRF
Request/Parser/SSRF
Rate-limit fail mode

Service Assertion
Key rotation
Secret permissions

PostgreSQL/Redis hardening
Empty Redis recovery

Poker WS size/schema/rate/backpressure/heartbeat

Request/Trace correlation
Metric cardinality
Safe log redaction
Alert fire/resolve
NTP/clock drift

Container hardening
Network exposure
Graceful shutdown
Digest rollback
Forward migration

pgBackRest full/differential/WAL
Encrypted repo restore
Logical restore
Recovery Kit decrypt
Historical fairness key validation

Clean-host DR
DR lock
External charging block
Invariant report
Typed unlock
RPO<=5m
RTO<=2h
```

---

# 533. Codex IS-11 Implementation Order

```text
01 infra version lock
02 deploy skeleton / deployctl
03 Edge / CSP / limits / SSRF / Rate Limit
04 Service Assertion / Key Rotation
05 Secret Contract
06 PG / Redis hardening
07 Poker WS security
08 Logs / Request ID / Metrics / Health
09 Prometheus / Alertmanager / node_exporter / Alerts / NTP
10 Compose / Networks / Hardening / Resources / Shutdown
11 Release Manifest / Deploy Preflight / Migration / Rollback / Poker deploy
12 pgBackRest integration / WAL / repo1 / repo2
13 Physical schedules / Logical dump / age / Recovery Kit
14 Backup Verification / Alerts
15 DR Host Lock / Edge Deny / Service Guard
16 Restore / Invariant / Typed Unlock
17 Incident / Secret Rotation / Supply-chain
18 Security / Observability / Backup / Full DR tests
```

---

# 534. IS-11 Prohibited Implementation

禁止：

```text
choose Edge without verification
expose DB/Redis/Metrics/pprof
weaken TLS/CSP
credentialed wildcard CORS
log request bodies/secrets
put secrets in Compose plaintext
world-readable secrets
run app as root for convenience
shared service bearer token
assume NewAPI assertion support
Redis financial backup
second PostgreSQL
silent NewAPI takeover
Docker socket mount
latest image
runtime curl|sh
guess PG major
claim PITR before integration verification
same-VPS backup counted as DR
unencrypted Recovery Kit
premature fairness key deletion
App Jobs as sole backup scheduler
restore DB maintenance as sole DR lock
blanket refund/cashout after DR
PITR single-record undo
in-place compromised host recovery
reopen before invariant PASS
guess Product OPEN
```

---

# 535. IS-11 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| IS-FRZ-549 | Production 固定 Edge-only Application Exposure：公网业务只经 Verified Edge 80/443，Platform/Poker Internal/PostgreSQL/Redis/Metrics/Debug 不直接发布。 | FROZEN |
| IS-FRZ-550 | Data Classification 继续 SECRET/RESTRICTED/SENSITIVE/INTERNAL/PUBLIC，Observability 默认禁止采集 SECRET/RESTRICTED Payload。 | FROZEN |
| IS-FRZ-551 | Edge Provider 必须由 DEPLOYMENT-VERIFY-01 选择 Nginx 或 Caddy；允许 HOST_EDGE / CONTAINER_EDGE 两种部署模式，但同一 Production 只能启用一种。 | FROZEN |
| IS-FRZ-552 | TLS 固定 1.2/1.3、HTTPS Redirect、HSTS 31536000，并冻结 TD-12 CSP；Staging Report-only 后 Production Enforcement。 | FROZEN |
| IS-FRZ-553 | Security Headers 固定 nosniff / strict-origin-when-cross-origin / DENY Frame，并默认关闭 V1 无需 Browser Hardware Capabilities。 | FROZEN |
| IS-FRZ-554 | V1 Request Limits 固定 Edge Body 512KiB、Request Target 8KiB、Headers 32KiB、Default JSON 64KiB、Announcement/Ops Content 256KiB、JSON Depth 32。 | FROZEN |
| IS-FRZ-555 | Session 继续 `__Host-chaldea_session` + Secure/HttpOnly/Lax，Login/OAuth/Recovery 必须新建 Session；Cookie Write 强制 CSRF+Origin+Fetch Metadata。 | FROZEN |
| IS-FRZ-556 | Backend Outbound 永远采用目的地 Allowlist；Announcement 外链不触发 Server Fetch，禁止任意 URL/metadata/internal SSRF。 | FROZEN |
| IS-FRZ-557 | Rate Limit 使用 Redis atomic bucket/GCRA 等价实现覆盖 Edge/Auth/Fresh/Poker/Game/Reward/Critical Ops；安全关键 Limiter 故障 Fail-closed，Public Read 可使用本地 emergency limit。 | FROZEN |
| IS-FRZ-558 | Platform↔Poker Internal HTTP 冻结 Ed25519 Service Assertion V1，精确签名 Key/Issuer/Audience/Time/Request ID/Method/Request URI/Body SHA-256。 | FROZEN |
| IS-FRZ-559 | Service Assertion 固定 30s Lifetime、±5s Clock Skew；Signing Key 使用版本化 kid 双阶段 Rotation，业务 Mutation 仍必须独立 Biz Idempotency。 | FROZEN |
| IS-FRZ-560 | Chaldea→NewAPI Authentication 继续 Source Verification Boundary，不假设 NewAPI 支持 Chaldea Service Assertion。 | FROZEN |
| IS-FRZ-561 | Production Secret Host Parent 固定 root 0700；具体 file 采用 root:service-runtime-GID 0640 + read-only Compose file secret，以适配本机 Compose bind-mount 与 non-root Container。 | FROZEN |
| IS-FRZ-562 | Game/Poker Fairness Key Ring 继续 ACTIVE/DECRYPT_ONLY/RETIRED；任何 DB/Backup 仍依赖旧 key_version 时禁止删除历史 Key。 | FROZEN |
| IS-FRZ-563 | PostgreSQL Runtime 固定 minimum privilege + SCRAM-SHA-256 + restrictive pg_hba + no public port；Backup DBA Identity 与 App/Poker Role 分离。 | FROZEN |
| IS-FRZ-564 | Redis 固定 Namespace+ACL、无公网端口并拒绝 CONFIG/FLUSH/DEBUG/MODULE 等高危命令；Platform 新增 `chaldea:ratelimit:*`，Redis 永不承担 Financial/Poker RPO。 | FROZEN |
| IS-FRZ-565 | Poker WS 固定最大 Frame 64KiB、Schema Validate、Rate Limit、Bounded Queue、20s heartbeat/60s stale close，V1 禁用 permessage-deflate。 | FROZEN |
| IS-FRZ-566 | Production `/debug/*`/pprof/internal metrics 永不经过 Public Edge；pprof 默认关闭，只能 Internal incident-bound 临时启用。 | FROZEN |
| IS-FRZ-567 | 8GB Observability Baseline 锁 Prometheus 3.13.2 LTS、Alertmanager 0.34.0、node_exporter 1.12.1；Grafana 不进入 mandatory baseline。 | FROZEN |
| IS-FRZ-568 | Structured Log 固定 Safe JSON Schema，永不记录 Raw Body/Cookie/Auth/CSRF/OAuth/Secret/Password/Prompt/Seed/Deck/Hole Card。 | FROZEN |
| IS-FRZ-569 | Metrics 只允许低 Cardinality Label，禁止 user/request/round/hand/operation/IP 成为 Metric Label。 | FROZEN |
| IS-FRZ-570 | Edge 生成规范 128-bit Request ID 并向 Platform/Poker 传播；代码保持 W3C Trace Context/OpenTelemetry Compatible，但 V1 不强制 Trace Backend。 | FROZEN |
| IS-FRZ-571 | 每服务固定 `/health/live`、`/health/ready`、`/metrics`；Dependency Failure 不等于 Liveness Failure，避免 Restart Loop。 | FROZEN |
| IS-FRZ-572 | Production 必须配置至少一个 Out-of-band Alert Sink；Alertmanager 按 Fingerprint 支持 FIRING/RESOLVED，并覆盖 Service/DB/Backup/WAL/Disk/Audit/Economy/Poker/Jobs/Cert。 | FROZEN |
| IS-FRZ-573 | Runtime Log Target 14d+Hard Cap；Prometheus 8GB=30d、4GB=7d 并同时配置 Retention Size Cap；Audit/Ledger/Migration 无 V1 TTL。 | FROZEN |
| IS-FRZ-574 | node_exporter 作为 Host systemd private-only Exporter，监控 filesystem/memory/load/network/timex；VPS 必须只启用一套可靠 NTP 并监控 drift。 | FROZEN |
| IS-FRZ-575 | `examples/deployment/external-newapi` 与 `examples/deployment/platform` 继续独立 Compose Project；Chaldea 不自动接管现有 Edge/PostgreSQL/Redis/NewAPI Ownership。 | FROZEN |
| IS-FRZ-576 | Chaldea Base Compose Mandatory Service 固定 Frontend/Platform/Poker/Prometheus/Alertmanager，node_exporter/NTP/Backup Timer 属于 Host Infrastructure。 | FROZEN |
| IS-FRZ-577 | Compose 使用 App/Observability/Verified Data/Optional Edge Network；Frontend 无 Data Network，Data/Edge Network 具体连接由 DEPLOYMENT-VERIFY-01 冻结后生成。 | FROZEN |
| IS-FRZ-578 | Chaldea-owned Containers 默认 Non-root/Read-only Root/Cap Drop All/No-new-privileges/No Privileged/No Host Network/No Docker Socket/Read-only Secret/tmpfs。 | FROZEN |
| IS-FRZ-579 | 8GB Profile 必须保留约1–1.5GB Host Headroom；精确 Container/PG/Redis/Worker/Prometheus Cap 通过 Production-like Load Test 写入资源 Profile，未完成则 Production Gate Fail。 | FROZEN |
| IS-FRZ-580 | 8GB Host 使用2GB Emergency Swap、4GB 使用1GB，建议 swappiness=10；Platform/Poker stop_grace_period 固定30s，业务恢复继续依赖 PostgreSQL。 | FROZEN |
| IS-FRZ-581 | Production Release Manifest 固定记录 Git/Build/Frontend Hash/Platform&Poker Image Digest/Schema Checksum/Asset&Config/Runtime UID-GID/Observability Version/Environment，禁止 `latest`。 | FROZEN |
| IS-FRZ-582 | Deployment 分类固定 Frontend-only/Compatible Backend/Schema/Poker/Security-Auth/Economy-Migration High-risk，各类走 Code-owned Deploy Gate。 | FROZEN |
| IS-FRZ-583 | High-risk Deploy 强制 Environment/Disk/DB/Redis/NTP/Backup/WAL/Migration/Manifest/Maintenance/Invariant/Resource Profile Preflight，关键 Gate Fail 直接停止。 | FROZEN |
| IS-FRZ-584 | Schema Migration 继续 immutable Forward + Expand/Migrate/Contract；破坏性 Contract 只能在旧 Release 不再使用且 Backup/Migration Verification 完成后执行。 | FROZEN |
| IS-FRZ-585 | Application Rollback 只允许当前 Schema Backward-compatible 时切 Previous Digest；不可逆 Data/Schema 使用 Forward Fix，灾难才 Restore。 | FROZEN |
| IS-FRZ-586 | Frontend 至少保留 Previous 2 Releases 且24h Hashed Assets；Poker Deploy 固定 Stop New Hands→Finish/Safe Pause→Restart→PG Recovery→30s Grace。 | FROZEN |
| IS-FRZ-587 | Chaldea Deployment 永不自动 Upgrade/Restart NewAPI；NewAPI Version/Compose/Image 变化必须评估并重跑受影响 SV。 | FROZEN |
| IS-FRZ-588 | PostgreSQL Physical/PITR Engine 锁 pgBackRest 2.59.1，Stanza=`chaldea-cluster-v1`；实际 archive/backup execution integration 在 DEPLOYMENT-VERIFY-01+SV-15 完成前保持 NOT READY。 | FROZEN |
| IS-FRZ-589 | pgBackRest Off-host Repository 使用 Client-side `aes-256-cbc` Repo Encryption + TLS；Cipher Pass Secret 与普通 Backup Data 分离进入 Recovery Kit。 | FROZEN |
| IS-FRZ-590 | PostgreSQL 目标固定 archive_mode on + pgBackRest archive-push + archive_timeout=300s；精确 Syntax 按实际 PostgreSQL Major 核验，不猜 Major。 | FROZEN |
| IS-FRZ-591 | repo1 固定 Continuous WAL + Sunday18:30UTC Full + Mon-Sat18:30UTC Differential + Daily21:00UTC Check，保留8个 Weekly Chains并满足至少30d PITR。 | FROZEN |
| IS-FRZ-592 | repo2 固定 First-Sunday19:30UTC Monthly Full、保留6个月 Restore Point，使用独立 Prefix/Cipher Credential；Provider 支持时启用 Versioning/Object Lock。 | FROZEN |
| IS-FRZ-593 | Portable Logical Backup 每日20:00UTC生成 newapi/chaldea pg_dump + safe roles/grants evidence，并使用 age1.3.2 加密，保留7 Daily +4 Weekly。 | FROZEN |
| IS-FRZ-594 | Recovery Kit 使用 age1.3.2，至少两个独立 Offline Recipient；包含历史 Fairness Key Ring、Backup Cipher/Recovery Material、OAuth/NewAPI 等不可重建 Secret，不保存 Session/CSRF/Ticket。 | FROZEN |
| IS-FRZ-595 | 正式 Backup Scope 必须覆盖 PG Cluster/Role-Grant/Migration/Release/Cutover/Runtime Config/Asset-Rights/Recovery Kit 与经 SV-15 证明不可重建的 NewAPI non-DB Persistence。 | FROZEN |
| IS-FRZ-596 | Backup Retention 固定 >=30d PITR、8 Weekly、6 Monthly、Logical7 Daily+4 Weekly；删除 Backup 前必须检查历史 Fairness Key/Release/Schema Dependency。 | FROZEN |
| IS-FRZ-597 | Backup Success 必须 Repository Check/Manifest/Checksum/WAL Continuity/Off-host Presence/Encrypted Artifact Verify 全通过，不能只凭 Process Exit 0。 | FROZEN |
| IS-FRZ-598 | WAL recoverable age>5m/Physical overdue/Repository Check failure 是 Critical，并阻断 High-risk Deploy/Cutover；Logical overdue 为 Warning。 | FROZEN |
| IS-FRZ-599 | Restore Drill 固定上线前 Full DR、每月 Isolated DB Restore、每季度 Full Application DR、Backup/Encryption Mechanism 改变后立即补测。 | FROZEN |
| IS-FRZ-600 | Restore Drill 必须 Isolated Network、无公网/Provider/Discord/Business Job Side Effect；Production Copy 用于 QA 前必须匿名化或 Fixture 化。 | FROZEN |
| IS-FRZ-601 | `DR_RECOVERY_LOCK` 固定为 Deployment-level Host File Authority `examples/deployment/platform/runtime/dr-recovery.lock`，独立于恢复出来的 Maintenance DB State。 | FROZEN |
| IS-FRZ-602 | `chaldea-deployctl dr-lock enable` 必须同时建立 Host Lock + Verified Edge Deny，阻断 Chaldea Writes、Poker New Work、Publish Jobs 与 External NewAPI Charging Path。 | FROZEN |
| IS-FRZ-603 | Full DR 顺序固定 Clean Host→DR Lock→Recovery Kit→PG PITR→Redis Empty→Immutable Services→Domain Recovery→Invariant→Operator Review→Typed Unlock→Reopen。 | FROZEN |
| IS-FRZ-604 | DR Unlock 必须提供 PASS Invariant Report Hash，并验证 Wallet/Ledger/Transfer/Reward/Round/Poker/Fairness/Migration/Ranking/Audit/NewAPI Integration。 | FROZEN |
| IS-FRZ-605 | Redis Total Loss 只导致 Session/Auth-flow/Return Intent/Cache/Presence 丢失；用户重登、Cache Rebuild、PG Jobs/Poker Recovery，不恢复 Financial Fact。 | FROZEN |
| IS-FRZ-606 | Platform/Poker Process Loss 只重启相同 Immutable Artifact 并从 PG Durable State恢复；Poker runtime_epoch++ +30s Grace，无需 PITR。 | FROZEN |
| IS-FRZ-607 | VPS/Disk Total Loss 正式恢复路径固定 Clean Host + Immutable Release + Recovery Kit + Off-host PITR + Empty Redis + DR Lock Verification。 | FROZEN |
| IS-FRZ-608 | Host Compromise 必须隔离旧机并在 Clean Host 恢复，轮换 Credentials/Service Keys/DB/Redis并撤销 Session；禁止原地清理后继续 Production。 | FROZEN |
| IS-FRZ-609 | Backup Provider Credential 泄露可 Rotate；pgBackRest Repository Cipher Pass 泄露必须建立新 Repository/Cipher Baseline，而不是宣称原 Repository 可安全 In-place Rotation。 | FROZEN |
| IS-FRZ-610 | PITR 仅用于 Cluster Disaster/Broad Corruption/Safe Pre-open Recovery/Security Clean Restore；单条业务错误必须 Incident/Reconciliation/Compensation。 | FROZEN |
| IS-FRZ-611 | 每次 Physical Backup/Restore 保存可追溯 Backup Manifest：Timeline/Time/WAL/Type/Tool/Repo/Cipher/Checksum/Release/Schema Reference。 | FROZEN |
| IS-FRZ-612 | Production Supply-chain 固定 Lockfile/go.sum/Pinned Input/Image Digest + Vulnerability/Image/Secret Scan + SBOM + Migration/Asset Gate，禁止 runtime curl|sh/floating latest。 | FROZEN |
| IS-FRZ-613 | Host Runbook 必须覆盖 Dedicated Admin/SSH Key/Root SSH Disable/Firewall/Security Update/NTP/Disk；这些 Infra 能力永不进入 Chaldea Operations。 | FROZEN |
| IS-FRZ-614 | Disk Monitoring 固定覆盖 PG/Docker/Logs/Prometheus/Backup Staging/Redis/Root；Local Backup Staging 设置 Hard Quota且上传验证后删除。 | FROZEN |
| IS-FRZ-615 | `chaldea-deployctl` Host-only 固定实现 Environment Verify/Preflight/Deploy/Postcheck/Backup Status/DR Lock/Restore Verify/Release Status，不提供公网执行面。 | FROZEN |
| IS-FRZ-616 | Secret Leak Playbook 固定按 Session/Service Key/DB-Redis/Backup Credential/Fairness Key 类型 Rotate/Revoke/Verify，历史 Decrypt Key 按 Retention 保留。 | FROZEN |
| IS-FRZ-617 | Alert/Remote Log/Metrics/Error/Trace 全部视为 Data Egress，只发送 Minimum Safe Metadata，永不发送 Raw Request/Prompt/Poker Snapshot/Private Game Secret。 | FROZEN |
| IS-FRZ-618 | DR Public Read 可由 Incident 决定，但 User Write、External Model API Charging、Poker New Work 在 Authority Verification 前必须关闭。 | FROZEN |
| IS-FRZ-619 | RPO Breach (>5m recoverable WAL age) 固定触发 Critical+Attention+High-risk Deploy Block，但不自动关闭整个 Public Website。 | FROZEN |
| IS-FRZ-620 | IS-11 Edge/Web Security Gate 必须验证 Exposure/TLS/CSP/Cookie/CSRF/Request Limit/SSRF/Rate-limit Fail Mode。 | FROZEN |
| IS-FRZ-621 | IS-11 Backup/DR Gate 必须验证 pgBackRest Full/Diff/WAL/Encrypted Restore/Logical Restore/Recovery Kit/Clean-host DR/DR Lock/Invariant/RPO/RTO。 | FROZEN |
| IS-FRZ-622 | IS-11 Observability/Deployment Gate 必须验证 Request Correlation/Cardinality/Redaction/Alert Fire-Resolve/NTP/Container Hardening/Graceful Shutdown/Digest Rollback。 | FROZEN |
| IS-FRZ-623 | Codex 实现顺序固定先 Security/Service Identity/Secrets→Observability→Compose/Deploy→Backup→DR Lock/Restore→Security/DR Tests。 | FROZEN |
| IS-FRZ-624 | IS-11 不新增 Product OPEN；所有 NewAPI Source Verification、Reward OPEN、Poker Product Gap、Public Record Policy 与 DEPLOYMENT-VERIFY-01 均显式保留，未完成时不得声称 Production Ready。 | FROZEN |

---

# 536. Open / Blocked Register after IS-11

```text
NewAPI:
SV-01 ～ SV-16
= BLOCKED_BY_NEWAPI_SOURCE_VERIFY

Deployment:
DEPLOYMENT-VERIFY-01
= PENDING

Backup:
BACKUP_INTEGRATION_READY
= false until actual PG topology + SV-15 verified

Reward:
Product OPEN
= unchanged

Poker:
POKER-PROD-GAP-01 ～ 05
= OPEN
Poker Production Ruleset
= CONFIG_INCOMPLETE

Public Records:
PUBLIC_RECORD_SELECTION_POLICY
= UNRESOLVED

Deployment Config:
Edge mode/provider
Data network mode
Backup provider
Alert sink
Resource profile caps
Rate limits
Observability size caps
= unresolved until verification/load/config closure
```

没有任何 Open/Verification Gate 被默认值绕过。

---

# 537. Change Log — WORKING v1.1

## Added

- 用户正式确认 `IS-11 — Security / Observability / Compose / Backup / DR Implementation Specification`；
- 冻结 `IS-FRZ-549 ～ IS-FRZ-624`；
- 冻结 Edge/TLS/HSTS/CSP/Security Headers；
- 冻结 Request/Parser/SSRF/Rate Limit；
- 冻结 Platform↔Poker Service Assertion V1；
- 冻结 Secret File / Key Rotation；
- 冻结 PG/Redis Hardening；
- 冻结 Poker WS Security；
- 冻结 Logs/Metrics/Correlation/Alerts/NTP；
- 冻结 Chaldea Compose Ownership / Networks / Container Hardening；
- 冻结 Resource Profile / Emergency Swap / Graceful Shutdown；
- 冻结 Release Manifest / Deployctl / High-risk Gate；
- 冻结 Expand/Migrate/Contract / Rollback / Poker Deploy；
- 冻结 pgBackRest 2.59.1 / WAL / repo1 / repo2；
- 冻结 age Logical Backup / Recovery Kit；
- 冻结 Backup Retention / Verification / Freshness；
- 冻结 Restore Drill；
- 冻结 Host+Edge `DR_RECOVERY_LOCK`；
- 冻结 Full DR / Invariant / Typed Unlock；
- 冻结 Redis/VPS/Host-compromise Recovery；
- 冻结 PITR Boundary / Backup Manifest；
- 冻结 Supply-chain / Host Security Baseline；
- 冻结 IS-11 Security / Observability / Backup / DR Test Gate。

## Preserved

```text
TD-FRZ-001 ～ TD-FRZ-552
IS-FRZ-001 ～ IS-FRZ-624

SV-01 ～ SV-16 unresolved
DEPLOYMENT-VERIFY-01 pending
Reward Product OPEN
Poker Product Gap 01～05
Poker Production Ruleset CONFIG_INCOMPLETE
PUBLIC_RECORD_SELECTION_POLICY unresolved
Implementation/Deployment Config unresolved where explicitly preserved
```

## Supersession

```text
Existing IS-FRZ superseded:
None
```

---

# 538. Next Batch

> **IS-12 — OpenAPI / Poker WS / Deployment Runbook / Final Audit**

IS-12 将完成：

```text
OpenAPI 3.1 Source
Generated Go/TypeScript Contracts

Poker WS JSON Schema
Golden Protocol Fixtures

Final BFF Endpoint / Error Registry

Deployment Environment Verification Procedure
NewAPI Source Verification Package

Production Config Register Closure

Product OPEN / Poker Gap / Public Record Blocker Report

Fresh Install Runbook
Upgrade Runbook
Rollback Runbook
Restore Runbook

Release Checklist

FINAL Cross-batch Audit
Implementation Spec v1.0 FINAL Packaging
```

---

# 539. IS-12 — OpenAPI / Poker WS / Deployment Runbook / Final Audit

> 状态：`FROZEN`  
> 用户确认：`整体按上述 IS-12 方案通过`  
> Frozen Decision Range：`IS-FRZ-625 ～ IS-FRZ-704`  
> Final Supersession：`IS-FRZ-022 → IS-FRZ-561`

## 539.1 Purpose

IS-12 是 Implementation Spec 最终收口批次，不再新增产品功能。本批正式冻结：

```text
OpenAPI 3.1.1 REST Authority
Go / TypeScript / Zod Contract Generation
Poker WS Typed Source / JSON Schema / Golden Fixtures
Final Error / Idempotency / Handler Coverage Registry
Deployment Verification Evidence
NewAPI SV-01 ～ SV-16 Evidence Package
Machine-readable Implementation Config
Product Blocker Registry
Production Readiness Registry
Fresh Install / Upgrade / Rollback / Restore / Cutover Runbooks
Final Decision / Migration / Authority / Security / Blocker Audit
Final Packaging
```

---

# 540. FINAL Contract Toolchain

```text
OpenAPI Specification       3.1.1
oapi-codegen/v2             v2.8.0
@redocly/cli                2.51.1
openapi-typescript          7.13.0
@cerios/openapi-to-zod      1.7.0
ajv                          8.20.0
ajv-formats                  3.0.1
```

CI 禁止动态 `latest`。

---

# 541. Contract Directory / Authority

```text
contracts/
├── openapi/
│   ├── chaldea-bff-v1.yaml
│   ├── paths/
│   ├── components/
│   ├── errors/
│   └── examples/
├── poker-ws/
│   ├── source/
│   ├── envelope.schema.json
│   ├── client-messages/
│   ├── server-events/
│   ├── examples/
│   └── golden/
├── errors/error-codes.json
├── readiness/
│   ├── product-blockers.json
│   ├── source-verification.json
│   ├── implementation-config.json
│   └── production-readiness.json
└── schemas/readiness/
```

REST 唯一 Root Authority：

```text
contracts/openapi/chaldea-bff-v1.yaml
```

允许 Local `$ref`，Remote `$ref` 禁止。

---

# 542. Contract Generation

Redocly 执行 lint + bundle，`REDOCLY_TELEMETRY=off`。

oapi-codegen 生成 Go DTO / Params / Responses / Strict Handler Interface，并继续适配现有 `net/http`。

openapi-typescript 生成：

```text
frontend/src/generated/api/types.ts
```

OpenAPI-to-Zod 生成：

```text
frontend/src/generated/api/schemas.ts
```

Poker WS 使用 Zod 4 typed source 导出 Draft 2020-12 JSON Schema。

Generated Files machine-owned；人工修改或 Drift → CI Fail。

---

# 543. OpenAPI Extensions / Error Registry

Code-owned Extensions：

```text
x-chaldea-idempotency
x-chaldea-business-identity
x-chaldea-resource-identity
x-chaldea-risk-level
x-chaldea-required-gates
x-chaldea-source-verification
x-chaldea-production-blockers
```

唯一 Error Registry：

```text
contracts/errors/error-codes.json
```

字段：

```text
code
http_status
retryable
message_key
details_schema
current_state_schema
allowed_next_actions
owner_domain
source_decision
```

Backend / OpenAPI / Frontend Error Union 均由同一 Registry 收敛。

---

# 544. Common HTTP Contract / Pagination

允许 Status：

```text
200 201 202 400 401 403 404 409 422 429 503 500
```

`202` 只在 Durable Resource/Business Identity 已存在时允许。

Serialization：

```text
IDs → string
UUIDv7 → lowercase canonical
*_units → decimal integer string
precise decimal/RTP/multiplier → string
Instant → RFC3339 UTC
Business Date → YYYY-MM-DD
Enum → SCREAMING_SNAKE_CASE
```

Pagination：

```text
page default=1 min=1
page_size default=20 min=1 max=100

cursor limit default=50 min=1 max=100
cursor opaque
```

---

# 545. Idempotency / Handler Coverage

所有 Durable Mutation 必须机器声明 `Idempotency-Key required` 或明确 exempt。

同 Key 同请求 → 同 Durable Effect；同 Key 不同请求 → `409 IDEMPOTENCY_CONFLICT`。

Handler 与 OpenAPI Operation 双向一致；Undocumented Handler / Missing Required Operation → CI Fail。

---

# 546. Final BFF Inventory

OpenAPI 必须覆盖：

```text
Session/Auth
Master/Account
Composite Reads
Models/API Access
API Keys
API Usage
Wallet
Rewards
Games
Poker
Rankings
History
Announcements
Operations
```

Conditional Source-blocked Capability 保留 Contract，但以 Capability/Blocker Metadata 表示不可用。

Poker Gap 未解决时依赖完整 Ruleset 的 Production Mutation返回 `POKER_RULESET_INCOMPLETE`。

---

# 547. Poker WS Contract

```text
Path: /ws/poker
Subprotocol: chaldea-poker.v1
```

Breaking Contract 使用新 Major Subprotocol。

Typed Source：

```text
contracts/poker-ws/source/
```

Zod 4 discriminated union，Object 默认 `additionalProperties=false`。

Unknown type：

```text
PROTOCOL_UNSUPPORTED_MESSAGE
```

Client Families：

```text
auth.connect
sync.request
hand.action
session.sit_out_next_hand
session.resume_play
client_seed.set_next
chat.send
ping
```

Server Families：

```text
auth.accepted
table.snapshot
table.state_changed
seat.state_changed
session.state_changed
hand.started
hand.action_applied
hand.street_changed
hand.cards_dealt
hand.pots_updated
hand.settled
hand.paused
hand.recovered
timer.started
control.changed
chat.message
service.notice
error
pong
```

---

# 548. Poker Viewer / Golden Fixtures

Viewer Profiles：

```text
PLAYER_SELF
PLAYER_OTHER
SPECTATOR
HOST
OPS
```

CI 必须证明无 unauthorized private cards / future deck / unreleased seed。

Golden Fixtures 覆盖 auth/snapshot/fold/call/raise/duplicate/gap/takeover/settled/recovery，并包含 IS-07 Fairness Golden Vectors。

全部使用 demo/fixture data。

---

# 549. Contract CI

固定：

```text
OpenAPI syntax/semantic validation
Redocly lint/bundle
oapi-codegen generation
openapi-typescript generation
OpenAPI→Zod generation
Poker Zod→JSON Schema generation
Ajv Draft2020-12 validation
Fixture validation
Generated drift
Breaking-change diff
Error registry coverage
Idempotency metadata coverage
Amount-field audit
Handler-route coverage
```

`/api/v1` Breaking Change 必须新 Major 或 Versioned Technical Change。

---

# 550. Initial Super Admin CLI

```text
chaldea-platform-admin bootstrap-super-admin
```

Input：

```text
--newapi-user-id <string>
--environment <environment>
```

Preconditions：

```text
environment matches
source-verified NewAPI user exists
admin principal count = 0
access-control guard available
```

在 `ADMIN_PRINCIPAL_SET` Guard 下原子创建 SUPER_ADMIN + Role History + `SYSTEM_BOOTSTRAP` Audit。

之后永久拒绝 Bootstrap。

Production 需要：

```text
PRODUCTION BOOTSTRAP_SUPER_ADMIN <target-id>
```

TTY Confirmation；无 Source-verified user read → `BOOTSTRAP_TARGET_VERIFY_BLOCKED`。

---

# 551. Deployment Verification Package

固定：

```text
verification/deployment/deployment-verify-01.json
```

Status：

```text
PENDING
PASS
FAIL
STALE
```

Evidence 覆盖 Host/Docker/Edge/DNS/TLS/80-443/NewAPI Compose/Image/PG/Redis/Networks/Ports/Volumes/Backup/Disk/Memory/Swap。

Verification 默认 Read-only。

PASS 计算 topology fingerprint；重大 topology 变化 → STALE。

---

# 552. NewAPI SV Evidence Package

```text
verification/newapi/
├── source-fingerprint.json
├── SV-01.json
...
├── SV-16.json
└── summary.json
```

Source Fingerprint 绑定 repository/ref、git commit、dirty state、compose hash、image digest、build metadata、relevant config schema。

SV Status：

```text
VERIFIED_NATIVE
VERIFIED_EXISTING_BRIDGE
VERIFIED_BRIDGE_REQUIRED
NOT_SUPPORTED_REQUIRES_TECH_CHANGE
BLOCKED_MISSING_SOURCE
STALE
```

Source Verification 只决定 Adapter 细节；不能改变 Chaldea 冻结语义。

Relevant fingerprint change → affected SV STALE。

优先只读；需要 Mutation 时必须隔离测试账号/环境。

---

# 553. Machine-readable Readiness Registers

```text
contracts/readiness/
implementation-config.json
product-blockers.json
source-verification.json
production-readiness.json
```

Implementation Config Status：

```text
FROZEN_VALUE
BLOCKED_BY_SOURCE_VERIFY
PENDING_LOAD_TEST
PENDING_DEPLOYMENT_VERIFY
PENDING_SECURITY_TUNING
PRODUCT_BLOCKED
NOT_APPLICABLE
```

Production Readiness 分类：

```text
PRODUCT_DECISION
SOURCE_VERIFICATION
IMPLEMENTATION_CONFIG
PRODUCTION_GATE
```

Release Script 读取机器 Registry，不解析 Markdown 勾选绕过 Blocker。

---

# 554. IS-12 Config Closure

固定：

```text
PAGE_SIZE_DEFAULT = 20
PAGE_SIZE_MAX = 100
CURSOR_LIMIT_DEFAULT = 50
CURSOR_LIMIT_MAX = 100

FRONTEND_READ_RETRY_MAX_ATTEMPTS = 2
FRONTEND_READ_RETRY_BACKOFF_INITIAL = 250ms
FRONTEND_READ_RETRY_BACKOFF_MAX = 2s
FRONTEND_READ_RETRY_JITTER = 0.20
FRONTEND_SAFE_DRAFT_TTL = 30m

POKER_CLIENT_RECONNECT_INITIAL = 500ms
POKER_CLIENT_RECONNECT_MAX = 5s
POKER_CLIENT_RECONNECT_JITTER = 0.20

POKER_WS_MAX_MESSAGE_BYTES = 65536
POKER_CHAT_MAX_BYTES = 2048
POKER_TABLE_NAME_MAX_GRAPHEMES = 40
POKER_TABLE_PASSWORD_MAX_BYTES = 128

JOB_LEASE_DURATION = 60s
JOB_HEARTBEAT_INTERVAL = 20s
JOB_DISPATCH_BATCH_SIZE = 25
JOB_RETRY_INITIAL = 5s
JOB_RETRY_MAX = 15m
JOB_RETRY_JITTER_RATIO = 0.20
JOB_MAX_AUTOMATIC_ATTEMPTS = 10

RANKING_ROUTINE_INTERVAL = 60s
RANKING_SOURCE_SCAN_BATCH_SIZE = 500
RANKING_ASSET_SNAPSHOT_MAX_SOURCE_AGE = 60s
RANKING_ASSET_SNAPSHOT_MAX_BUILD_SKEW = 60s

OPS_GLOBAL_SEARCH_MAX_RESULTS = 20
OPS_AUDIT_SNAPSHOT_MAX_BYTES = 65536
OPS_HEALTH_REFRESH_INTERVAL = 30s
OPS_HEALTH_STALE_AFTER = 90s
OPS_HEALTH_CHECK_TIMEOUT = 3s
```

---

# 555. Configs Remaining Blocked / Pending

Source-blocked：

```text
SESSION_IDLE_TTL
SESSION_ABSOLUTE_TTL
SESSION_TOUCH_INTERVAL
ACCOUNT_STATUS_MAX_AGE
ACTIVE_QUOTA_LOW_WATERMARK
ACTIVE_QUOTA_TARGET_WATERMARK
ACTIVE_QUOTA_MAX_ACTIVE_BUFFER
ECONOMY/NewAPI timeout/backoff values
```

Evidence-required：

```text
Auth/Registration/Fresh/Poker rate limits
POKER_ACTOR_MAILBOX_CAPACITY
POKER_WS_SEND_QUEUE_CAPACITY
Argon2 parameters
Container CPU/RAM
PostgreSQL memory
Redis sizing
Prometheus/log hard caps
Frontend route JS budgets
alert thresholds
telemetry sampling
Backup Provider
Alert Sink
```

保持 Pending/Blocked，不填假 Default。

---

# 556. Product Blocker Registry

Reward Product OPEN：

```text
Hourly asset type
Hourly natural-hour vs rolling-60m
Hourly accumulation
Hourly daily limit
Relief asset type
Relief unused eligibility accumulation
Relief Active Poker behavior
Reward maintenance/temporary-disable policy
future fixed-amount policy
issuance alert threshold
```

冻结金额仍：

```text
Registration/Migration 1000
Daily 500
Hourly Amount 100
Relief Amount 300
```

Poker：

```text
POKER-PROD-GAP-01 ～ 05
POKER_PRODUCTION_READY=false
```

Public Records：

```text
PUBLIC_RECORD_SELECTION_POLICY=UNRESOLVED
Recent Wins hidden
Featured Records hidden
Writer disabled
```

---

# 557. Production Readiness Status Classes

```text
IMPLEMENTATION_CONTRACT_COMPLETE
BASE_PLATFORM_PRODUCTION_READY
FULL_V1_FEATURE_SET_READY
```

IS-12 FINAL 只自动保证第一项。

Base Production 至少要求 Deployment Verify、Required SV、Resource、Security/Supply-chain、Asset/Rights、A11y、Performance、Backup Integration、Prelaunch Restore Drill PASS。

未启用 Feature 可以保持 Product Blocked，但必须 disabled。

---

# 558. Fresh Install Runbook

```text
01 authoritative docs/readiness
02 DEPLOYMENT-VERIFY-01
03 NewAPI SV
04 mandatory config closure
05 host security
06 DNS/Edge/TLS
07 release directories
08 UID/GID
09 secrets
10 PG roles / Redis ACL
11 migrations 000000～000029
12 deploy Frontend/Platform/Poker
13 first Super Admin bootstrap
14 observability
15 pgBackRest integration
16 first physical backup
17 verify WAL
18 Recovery Kit
19 isolated restore drill
20 asset/rights/a11y/performance
21 production gate
22 post-deploy smoke
23 open enabled traffic
```

无真实 NewAPI/VPS Evidence 时可实现代码但不能标记 Production Install Complete。

---

# 559. Upgrade / Rollback / Restore / Cutover Runbooks

Upgrade：

```text
category
→ manifests
→ contract diff
→ preflight
→ backup
→ maintenance if required
→ expand/migrate
→ immutable deploy
→ recover
→ postcheck
→ reopen
```

Frontend-only 保留 Previous 2 Releases / >=24h 并 Atomic Index Switch。

Poker 继续 Stop New Hands→Finish/Safe Pause→Restart→PG Recovery→30s Grace。

Rollback 仅在 Schema backward-compatible。

不可逆 Schema/Data → Forward Fix；灾难才 PITR/DR。

Restore 直接复用 IS-11 Full DR。

Cutover 只包装 IS-04 Durable Cutover 的证据/检查，不重写状态机。

---

# 560. Release Checklist

```text
Source commit clean
Contract CI PASS
Generated drift PASS
Tests PASS
Migration checksum PASS
Security/Secret scan PASS
SBOM PASS
Asset/Rights PASS
A11y PASS
Performance PASS
Deployment Verify not stale
required SV not stale
Backup/WAL healthy
Release Manifest complete
Postcheck PASS
```

---

# 561. Final Supersession

正式：

```text
IS-FRZ-022
→ SUPERSEDED_BY IS-FRZ-561
```

历史 0600 规则保留；当前规范为：

```text
host parent root 0700
specific secret source root:<service-runtime-GID> 0640
read-only Compose file secret
```

原因是 local Compose file-backed secret 为 bind mount，固定 non-root runtime 需要显式 group-read。

---

# 562. Final Audit Contract

验证：

```text
Decision continuity / uniqueness / status
Migration continuity
Contract / Handler / Error / Poker Schema coverage
IA Route coverage
State machine compatibility
Settlement xor Refund
Exactly-once semantics
Authority boundaries
DB ownership
Auth/security/privacy
Art/asset coverage
Deployment/DR coverage
Blocker completeness
```

Final Blocker Audit 必须显式保留 Reward OPEN / Poker 5 Gaps / Public Record / SV01-16 / Deployment Verify / Pending Config / Production Gates。

禁止为 FINAL 伪造 PASS。

---

# 563. Final Meaning / Codex Stop Conditions

批准后文档目标：

```text
FINAL / IMPLEMENTATION CONTRACT COMPLETE
```

不表示 Production Ready。

Codex 必须读取 Requirements / IA FINAL / Art FINAL / Technical Design FINAL / Implementation Spec FINAL 以及机器 Readiness Registers。

遇到 Product OPEN / Source Verify Missing / Deployment Unknown / Evidence-required Config，必须停在明确边界，不得假定标准答案。

---

# 564. IS-12 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| IS-FRZ-625 | IS-12 是 Implementation Spec 最终收口批次，只负责 Machine-readable Contract、Verification、Runbook、Readiness 与 Final Audit，不新增产品功能。 | FROZEN |
| IS-FRZ-626 | Contract Toolchain 锁 OpenAPI3.1.1、oapi-codegen2.8.0、Redocly CLI2.51.1、openapi-typescript7.13.0、openapi-to-zod1.7.0、Ajv8.20.0。 | FROZEN |
| IS-FRZ-627 | REST 唯一 Authority 为 `contracts/openapi/chaldea-bff-v1.yaml`，允许 local refs、禁止 remote refs；Bundled Contract 为生成 Artifact。 | FROZEN |
| IS-FRZ-628 | Redocly 固定执行 lint+bundle 且 CI Telemetry关闭；OpenAPI Root 与 Bundle 均必须通过验证。 | FROZEN |
| IS-FRZ-629 | oapi-codegen 只生成 Go DTO/Strict Contract Interface，不引入新 Web Framework，继续适配既有 net/http 架构。 | FROZEN |
| IS-FRZ-630 | openapi-typescript 生成 Frontend API Types；HTTP Client 继续由 IS-10 BFF Client 承担，不引入第二 Client Authority。 | FROZEN |
| IS-FRZ-631 | REST Runtime Zod Schemas 从同一 OpenAPI 使用 Zod4-compatible Generator 生成，不维护重复手写 Request/Response Schema。 | FROZEN |
| IS-FRZ-632 | OpenAPI 使用 Code-owned `x-chaldea-*` Extension 标记 Idempotency/Business Identity/Risk/Gate/Source Blocker，CI 校验 Extension。 | FROZEN |
| IS-FRZ-633 | 建立唯一 `contracts/errors/error-codes.json`，Backend/OpenAPI/Frontend Error Code 全部从同一 Registry 收敛。 | FROZEN |
| IS-FRZ-634 | Success/Error/HTTP Status/ID/Amount/Time/Enum Contract 精确沿用 TD-13，不允许单 Handler 私建 Envelope 或资产 Number。 | FROZEN |
| IS-FRZ-635 | Page Pagination 固定 default20/max100；Cursor Pagination 固定 default50/max100，Cursor opaque，Sort/Filter Code-allowlisted。 | FROZEN |
| IS-FRZ-636 | 所有 Durable Mutation 必须有机器可读 Idempotency Metadata；同 Key 不同 Request=409，同 Key同Request只能同 Effect。 | FROZEN |
| IS-FRZ-637 | Handler↔OpenAPI Operation 必须双向覆盖；Undocumented Handler 或 Required Operation Missing 都是 CI Failure。 | FROZEN |
| IS-FRZ-638 | Final OpenAPI 必须完整枚举 TD-13 Session/Auth/Master/Models/Keys/Usage/Wallet/Rewards/Games/Poker/Rankings/History/Announcements Contract。 | FROZEN |
| IS-FRZ-639 | Operations Module Read/Write 必须在 OpenAPI 中展开成具体 Path，不允许 wildcard/generic state PATCH/SQL-like后门。 | FROZEN |
| IS-FRZ-640 | Poker WS Canonical Typed Source 使用 Zod4 discriminated schemas，并生成 Draft2020-12 JSON Schema 文件满足机器协议 Contract。 | FROZEN |
| IS-FRZ-641 | Poker WS Object 默认 `additionalProperties=false`，Message `type` 为 discriminator，未知 Message Type 必须拒绝。 | FROZEN |
| IS-FRZ-642 | Poker Client/Server Message Family 精确保持 TD-13/IS-07 列表，不新增 arbitrary command 消息。 | FROZEN |
| IS-FRZ-643 | Poker Contract Fixtures 必须包含 Player/Spectator/Host/Ops Viewer Projection Privacy 测试，禁止秘密字段先发送后前端隐藏。 | FROZEN |
| IS-FRZ-644 | Poker 协议固定 `/ws/poker` + `chaldea-poker.v1`；Breaking Contract 使用新 Subprotocol Major。 | FROZEN |
| IS-FRZ-645 | OpenAPI/WS/Golden Fixtures 只使用 demo/fixture data，并包含已冻结 Poker Fairness Golden Vectors。 | FROZEN |
| IS-FRZ-646 | Contract CI 固定 OpenAPI/JSON Schema/Fixture/Generated Drift/Error/Idempotency/Amount/Handler Coverage 全套校验。 | FROZEN |
| IS-FRZ-647 | `/api/v1` Breaking Change 必须被 Contract Diff 阻断并进入新 Major 或 Versioned Technical Change，禁止 Silent Break。 | FROZEN |
| IS-FRZ-648 | Generated Go/TS/Zod/WS Schema 必须可重复生成且 Working Tree 无 Drift；Generated Files 人工修改 CI Fail。 | FROZEN |
| IS-FRZ-649 | Contract Example/Review Package 永久禁止真实 Account/API Secret/Prompt/Private Poker State。 | FROZEN |
| IS-FRZ-650 | Initial Super Admin 固定 Deployment-only `chaldea-platform-admin bootstrap-super-admin` CLI，不建立 Public/Web Bootstrap Endpoint。 | FROZEN |
| IS-FRZ-651 | Bootstrap 必须 Access-control Guard 下验证 Admin Count=0，原子创建 Super Admin+Role History+SYSTEM_BOOTSTRAP Audit；之后永久拒绝 Bootstrap。 | FROZEN |
| IS-FRZ-652 | Production Bootstrap 额外要求 Environment/Target/Release Preview+Typed TTY Confirmation；目标用户存在性必须经过 Source-verified NewAPI Read。 | FROZEN |
| IS-FRZ-653 | `DEPLOYMENT-VERIFY-01` 固定输出 Schema-validated JSON Evidence，状态 PENDING/PASS/FAIL/STALE。 | FROZEN |
| IS-FRZ-654 | Deployment Verify 必须确认 Host/Docker/Edge/DNS/TLS/80-443/Public Ports/NewAPI Compose/Image/PG/Redis/Networks/Volumes/Disk/Memory/Swap。 | FROZEN |
| IS-FRZ-655 | Deployment Verification 默认 Read-only，不能为采证重启服务、改 Firewall、改 DB/Redis 或 NewAPI Config。 | FROZEN |
| IS-FRZ-656 | Deployment PASS 计算 topology fingerprint；Edge/Network/NewAPI Image/Compose/PG/Redis重大变化自动使 Evidence STALE。 | FROZEN |
| IS-FRZ-657 | NewAPI Source Verification 固定 `source-fingerprint.json + SV-01..16.json + summary.json` 机器证据包。 | FROZEN |
| IS-FRZ-658 | NewAPI Source Fingerprint 绑定 Git Commit/Dirty State/Compose Hash/Image Digest/Build/Relevant Config Schema，Production 不接受无记录 Dirty Source。 | FROZEN |
| IS-FRZ-659 | 每个 SV 使用统一状态 VERIFIED_NATIVE/EXISTING_BRIDGE/BRIDGE_REQUIRED/NOT_SUPPORTED_TECH_CHANGE/BLOCKED/STALE，并绑定证据。 | FROZEN |
| IS-FRZ-660 | SV-01～04 精确核验 Auth/Identifier/Password/Discord；UI Observation 不能替代源码/Adapter Evidence。 | FROZEN |
| IS-FRZ-661 | SV-05～08 精确核验 Identity Types/API Key/Admin/Cutover Write Freeze。 | FROZEN |
| IS-FRZ-662 | SV-09～12 精确核验 Raw Quota/Idempotency/Reactive Refill/API Attribution；只有 SV-10 无 Native 能力时才使用已冻结 Quota Bridge。 | FROZEN |
| IS-FRZ-663 | SV-13～16 精确核验 Chaldea→NewAPI Auth/Redis/Persistent Volume/Public API Allowlist，禁止暴露整个 NewAPI Admin/Web Surface。 | FROZEN |
| IS-FRZ-664 | Source Verification 只可决定 Adapter 实现；若真实 NewAPI 不能满足冻结 Chaldea 语义必须 Technical Change，不允许 Codex 隐式改产品。 | FROZEN |
| IS-FRZ-665 | NewAPI Commit/Image/Compose/Bridge 等 Relevant Fingerprint 变化会把受影响 SV 自动标记 STALE，Production 前必须重验。 | FROZEN |
| IS-FRZ-666 | Source Verification 优先静态/只读；需要 Mutation 的验证必须使用隔离测试账号/环境，不得改真实用户资产/绑定。 | FROZEN |
| IS-FRZ-667 | 最终 `implementation-config.json` 使用 FROZEN_VALUE/SOURCE_BLOCKED/LOAD_TEST/DEPLOYMENT_VERIFY/SECURITY_TUNING/PRODUCT_BLOCKED 等明确状态，禁止裸 TBD。 | FROZEN |
| IS-FRZ-668 | IS-12 锁 Pagination、Frontend Read Retry(2,250ms→2s,20% jitter)、Safe Draft30m、Poker Reconnect500ms→5s/20% jitter。 | FROZEN |
| IS-FRZ-669 | Poker 固定 WS max65536 bytes、Chat max2048 bytes、Table Name40 graphemes、Table Password128 bytes；Mailbox/Send Queue/Argon2继续 Load/Benchmark。 | FROZEN |
| IS-FRZ-670 | Job 基线锁 Lease60s/Heartbeat20s/Batch25/Retry5s→15m/Jitter20%/Max10，配置变更必须版本化并有证据。 | FROZEN |
| IS-FRZ-671 | Ranking 基线锁 interval60s/batch500/source-age60s/build-skew60s；Ops锁 Search20/Audit64KiB/Health30s-Stale90s-Timeout3s。 | FROZEN |
| IS-FRZ-672 | Session TTL/Account freshness/Active Quota/Economy Timeout/Rate Limit/Argon2/Resource/JS Budget/Alert Threshold 等需 Source/Load/Deployment 证据的值继续显式 Blocked/Pending。 | FROZEN |
| IS-FRZ-673 | Implementation Config Pending 不等于 Product OPEN；Final 必须保留其 Evidence Requirement，不能为了 FINAL 填假 Default。 | FROZEN |
| IS-FRZ-674 | 建立机器 `product-blockers.json`，Reward/Poker/Public Records 三类与 Implementation Config 永久分离。 | FROZEN |
| IS-FRZ-675 | 建立机器 `production-readiness.json`，严格采用 PRODUCT_DECISION/SOURCE_VERIFICATION/IMPLEMENTATION_CONFIG/PRODUCTION_GATE 四类。 | FROZEN |
| IS-FRZ-676 | 最终状态区分 IMPLEMENTATION_CONTRACT_COMPLETE、BASE_PLATFORM_PRODUCTION_READY、FULL_V1_FEATURE_SET_READY，三者不得混写。 | FROZEN |
| IS-FRZ-677 | Poker 五 Gap 未全部解决并发布完整 Ruleset 前始终 `POKER_PRODUCTION_READY=false`，OpenAPI存在不代表 Feature 可启用。 | FROZEN |
| IS-FRZ-678 | Reward 未决 Policy 只阻断对应 Hourly/Relief/Product行为，不允许 Operations/Config 默认值绕过。 | FROZEN |
| IS-FRZ-679 | Public Record Selection 未决时模块隐藏、Writer disabled、无假中奖记录。 | FROZEN |
| IS-FRZ-680 | Base Production Release 必须读取 Deployment/SV Evidence，Required Evidence STALE/BLOCKED 时 Gate Fail。 | FROZEN |
| IS-FRZ-681 | Production Gate 固定覆盖 Resource/Load/Asset-Rights/A11y/Performance/Security/Supply-chain/Backup/Restore/DR。 | FROZEN |
| IS-FRZ-682 | Release Gate 只读取机器 Readiness/Evidence Registry，不通过人工勾 Markdown Checkbox 绕过 Blocker。 | FROZEN |
| IS-FRZ-683 | Fresh Install Runbook 固定 Verify→Host/Security→Secrets/Data→Migrations→Deploy→Bootstrap Admin→Backup/Restore→Production Gates→Open。 | FROZEN |
| IS-FRZ-684 | 没有实际 NewAPI Source/VPS Evidence 时 Codex可实现代码，但不得标记 Production Install Complete。 | FROZEN |
| IS-FRZ-685 | Fresh Install 只执行 Chaldea `000000～000029` Migration，并在 Source-verified User Read 后执行一次 Super Admin Bootstrap。 | FROZEN |
| IS-FRZ-686 | Initial Production Open 前必须具有PITR/WAL/Physical Backup/Recovery Kit且完整 Restore Drill PASS。 | FROZEN |
| IS-FRZ-687 | Upgrade Runbook 固定 Deployment Category→Manifest/Contract Diff→Preflight→Backup→Maintenance→Migration→Deploy→Postcheck→Reopen。 | FROZEN |
| IS-FRZ-688 | High-risk Upgrade 继续强制 Fresh PITR Evidence+Maintenance+Expand/Migrate+Immutable Deploy+Verification。 | FROZEN |
| IS-FRZ-689 | Frontend-only Deploy 使用 Hashed Assets+Previous2Releases/24h+Atomic Index Switch，不无故启动业务 Maintenance。 | FROZEN |
| IS-FRZ-690 | Poker Deploy 继续 Stop New Hands→Finish/Safe Pause→Restart→PG Recovery→30s Grace，不改任何 Hand资产事实。 | FROZEN |
| IS-FRZ-691 | Rollback 只使用 Previous Immutable Frontend/Container Artifact，且必须当前 Schema backward-compatible。 | FROZEN |
| IS-FRZ-692 | Schema/Data 已不可逆时只允许 Forward Fix；禁止伪造 Down Migration 或运行旧不兼容 App。 | FROZEN |
| IS-FRZ-693 | Restore Runbook 直接复用 IS-11 Clean-host/PITR/Empty Redis/DR Lock/Invariants，不另建第二套恢复状态机。 | FROZEN |
| IS-FRZ-694 | DR Unlock 继续强制 PASS Invariant Report Hash + Operator Typed Confirmation，任何 Critical Fail 保持 Lock。 | FROZEN |
| IS-FRZ-695 | Cutover Runbook 只包装 IS-04 Durable Cutover State Machine 的 Verification/Backup/Evidence，不修改其迁移语义。 | FROZEN |
| IS-FRZ-696 | 每 Release 必须通过 Contract/Test/Migration/Security/SBOM/Asset/A11y/Performance/Deployment/SV/Backup Evidence Checklist并绑定 Release Manifest。 | FROZEN |
| IS-FRZ-697 | Final Audit 强制 IS-FRZ 连续/唯一/状态校验；批准后目标范围001～704。 | FROZEN |
| IS-FRZ-698 | Final Migration Audit 强制 `000000～000029` 连续、唯一、Checksum 未修改；IS-12 不新增业务 Migration。 | FROZEN |
| IS-FRZ-699 | Final Contract/IA Coverage Audit 强制 Route→BFF/WS、Handler→OpenAPI、Error→Registry、Poker Message→Schema 全部可追踪。 | FROZEN |
| IS-FRZ-700 | Final State/Authority/Exactly-once Audit 保持 Settlement xor Refund、PG Durable Authority、Redis/Frontend Projection、At-least-once Delivery+Idempotent Effect。 | FROZEN |
| IS-FRZ-701 | Final Auth/Security/Privacy/Art/Deployment/DR Audit 强制秘密边界、Admin隔离、Design Tokens/Rights/A11y/PITR/DR Lock 均无后续批次绕开。 | FROZEN |
| IS-FRZ-702 | Final Blocker Audit 要求 Reward OPEN/Poker5Gap/PublicRecord/SV01-16/Deployment Verify/Config Pending/Production Gates 全部显式存在，漏一项即 Final Audit Fail。 | FROZEN |
| IS-FRZ-703 | Final Supersession 正式记录 `IS-FRZ-022 → SUPERSEDED_BY IS-FRZ-561`；旧0600 Secret File 决策保留历史，当前规范为 root:service-GID 0640。 | FROZEN |
| IS-FRZ-704 | IS-12 Approval 后 Implementation Spec 可标记 FINAL/IMPLEMENTATION CONTRACT COMPLETE；在现存 Source/Product/Deployment/Readiness Blocker 关闭前不得标记 Base Production或Full V1 Ready。 | FROZEN |

---

# 565. FINAL Open / Blocked Register

```text
Reward Product OPEN
POKER-PROD-GAP-01 ～ POKER-PROD-GAP-05
PUBLIC_RECORD_SELECTION_POLICY = UNRESOLVED

SV-01 ～ SV-16 = BLOCKED_BY_NEWAPI_SOURCE_VERIFY
DEPLOYMENT-VERIFY-01 = PENDING

Session TTL / Account freshness
Active Quota watermarks
Economy/NewAPI timeout tuning
Rate limits
Poker mailbox/send queue/Argon2
Container/PG/Redis resources
Prometheus/log caps
Frontend route JS budgets
Alert thresholds / telemetry
Backup Provider / Alert Sink
Rights / Assets / A11y / Performance
Backup Integration / Restore / Full DR Drill
```

---

# 566. Change Log — WORKING v1.2

- 用户正式确认 IS-12；
- 冻结 `IS-FRZ-625 ～ IS-FRZ-704`；
- 冻结最终 Contract Toolchain / OpenAPI / Poker WS / Error Registry；
- 冻结 Initial Super Admin CLI；
- 冻结 Deployment Verify / NewAPI SV Evidence Packages；
- 冻结 Readiness Registers / Config Closure；
- 冻结 Fresh Install / Upgrade / Rollback / Restore / Cutover Runbooks；
- 冻结 Final Audit；
- 正式记录 `IS-FRZ-022 → IS-FRZ-561` Supersession。

---

# 567. FINAL Audit Input Status

```text
IS-01 ～ IS-12 = FROZEN
Decision Range = IS-FRZ-001 ～ IS-FRZ-704
Expected Superseded = IS-FRZ-022 only
Implementation Contract = candidate complete
Production Readiness = not implied
```

---

# 568. FINAL Audit Result

> 状态：`FINAL AUDIT PASSED`

## 568.1 Decision Register

```text
Decision Range:
IS-FRZ-001 ～ IS-FRZ-704

Missing:
None

Duplicate Decision Register Rows:
None

PROPOSED Rows:
0
```

## 568.2 Supersession

```text
IS-FRZ-022
→ SUPERSEDED_BY IS-FRZ-561
```

除此之外无其他 IS-FRZ Superseded。

## 568.3 Migration Audit

```text
Migration Range:
000000 ～ 000029

Missing:
None

Unexpected:
None
```

Migration 历史继续遵守 Immutable / Forward-only / Checksum Contract。

## 568.4 Cross-batch Audit

```text
State Machine                         PASS
Settlement / Refund Mutual Exclusion PASS
Exactly-once Semantics               PASS
Authority Boundaries                 PASS
Schema Ownership                     PASS
Authentication                       PASS
Security / Privacy                   PASS
IA Route Coverage                    PASS
Art / Asset Coverage                 PASS
Deployment / Backup / DR Coverage    PASS
Blocker Preservation                 PASS
```

Exactly-once 最终语义：

```text
Transport / Worker Delivery:
at-least-once possible

Durable Business Effect:
stable identity
+ idempotency
+ unique constraint
+ transaction
```

不声明 HTTP / WebSocket Network Exactly-once。

## 568.5 Blockers Preserved

```text
Reward Product OPEN

Poker Product Gap 01～05

PUBLIC_RECORD_SELECTION_POLICY

NewAPI SV-01 ～ SV-16

DEPLOYMENT-VERIFY-01

Evidence-required Implementation Config

Production Asset / Rights / Accessibility / Performance Gates

Backup Integration / Restore / Full DR Drill
```

没有任何 Open / Pending / Blocked 项为了 FINAL 被伪造为 PASS。

---

# 569. FINAL Status

```text
Chaldea Platform Implementation Spec v1.0

Status:
FINAL / IMPLEMENTATION CONTRACT COMPLETE

Frozen Batches:
IS-01 ～ IS-12

Decision Range:
IS-FRZ-001 ～ IS-FRZ-704

Superseded:
IS-FRZ-022 → IS-FRZ-561

Implementation Contract Complete:
YES

Base Platform Production Ready:
NO — pending Production Readiness Register

Full V1 Feature Set Ready:
NO — pending Product Blockers / Verification
```

`FINAL` 表示 Repository / Service / Data / Transaction / Games / Poker / Rankings / Operations / Frontend / Security / Deployment / Contract / Verification / Runbook 的 Implementation Contract 已完整收口。

`FINAL` 不表示：

```text
NewAPI SV completed
VPS topology verified
Product OPEN resolved
final media rights ready
load test complete
restore drill complete
production deployed
```

后续若要改变已冻结决定，不得静默修改：必须建立 Versioned Implementation Change Proposal，指出被替代 Decision、原因、影响与新的 Decision ID。

---

# 570. Next Stage — Implementation Execution

下一阶段正式进入：

```text
Source Verification
Deployment Verification
Repository Implementation
Configuration / Load Evidence
Production Gates
Deployment
```

Codex 必须以：

```text
Requirements v0.2.11
IA v0.3.1 FINAL
Art Direction v0.4 FINAL
Technical Design v0.5 FINAL
Implementation Spec v1.0 FINAL
Final Readiness Registers
```

为权威输入。

