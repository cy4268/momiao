# 模型家族封面管理 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 管理员上传并确认家族封面后，匿名目录与所有同家族模型详情读取同一张真实 CDN 图片。

**Architecture:** 在现有模型运营域内增加家族图片登记、预览与绑定；复用现有会话、权限 epoch、操作编号和审计。管理员文件经服务端校验一次后写入 R2，公共图片请求直达 CDN。公共模型增加统一封面投影，后台与前台共享同一家族状态，而不批量修改每个模型。

**Tech Stack:** 现有 Go 1.27.1、PostgreSQL/pgx、React 19、TypeScript 6、Vite/Vitest；仅补充 AWS SDK for Go v2 的 S3/credentials 模块及 `golang.org/x/image` WebP 解码，不引入上传或测试框架。

**Spec:** [已确认的家族封面规格](../specs/2026-09-26-model-family-covers-design.md)。用户已在本会话确认，包括“仅管理员上传经服务端一次”的路径。

计划基线：`43562487641fd4696ddb93f674a9d9ca9ff3e784`，`codex/game-hall-night`。用户已确认本会话直接实施；实施基线为 `4b6f8837257ad62535a77ee648065207a195339b`。2026-09-26 三项实现、定向验证及真实 R2/CDN 联调已完成，未合并主分支或部署。

## Global Constraints

- “一个家族只显示一个目录卡片，具体模型与渠道仍在详情内选择。”保留原九套批准图和现有家族枚举，`ernie` 对应 `persona_wenxin_master`。
- “只有管理员上传新文件时，经业务服务器校验一次再写 R2；所有图片展示均直接访问 CDN。”继续使用 `VITE_ASSET_BASE_URL`。
- “接收静态 PNG、JPEG、WebP；单文件最多 8 MiB，任一边不超过 8192 像素，总像素不超过 16,777,216。”原图不转码。
- “上传需要 `models.write`，SET/RESET 公开家族绑定需要 `models.publish`。”保留确认、版本、epoch、幂等、审计；旧模型命令含义不变。
- “最多新增一条上传链路回归”；扩展下述既有用例，不增加测试框架、产品 fixture 或逐函数测试。本地不跑全量套件。
- “不合并主分支、不部署网站”；每个完成的源码阶段脱敏上传。旧 R2 对象、审计和迁移历史保留。
- 如执行方法涉及子代理，全部使用 `gpt-6-sol`、`max`；本计划编写阶段未派发代理；实施采用单人连续实现与一次独立终审。

## Review Focus

1. 慢上传穿过真实 HTTP 服务器时仍受默认 10 秒 socket deadline 限制，单改 handler 超时不够 → Task 2 唯一新增上传回归使用实际本地 HTTP server 校验路径限定的 deadline。
2. FormData 被共用客户端 JSON 化，或 POST 失败后自动重发/退出后接纳迟到结果 → Task 3 扩展现有 native/opaque 客户端测试，不复制认证实现。
3. R2 已写入后权限撤销或数据库提交失败 → Task 1/2 验证不登记成功回执、不改家族绑定，孤立对象保留且同内容重试收敛。
4. 同内容重复上传、相同操作号不同内容、恢复默认后的旧版本重放 → Task 1 生命周期测试区分内容复用、操作冲突和有效回执重放，版本只递增。
5. 用户切换家族或新图加载失败时，旧异步响应/旧图片失败次数污染当前封面 → Task 3 扩展现有公共家族选择、图片失败和后台确认用例。

## 文件边界

| 任务 | 新建 | 修改与复用 |
| --- | --- | --- |
| 1：家族状态与公共投影 | `internal/platform/catalog_covers.go`、`internal/platform/catalog_cover_ops.go`、`internal/platform/migrations/0040_model_family_covers.sql`、`deploy/sql/runtime-grants-0040-model-family-covers.psql` | `internal/platform/catalog.go`、`catalog_public.go`、`catalog_store.go`、`catalog_ops_test.go`、`catalog_ops_concurrency_test.go`、`catalog_grants_test.go` |
| 2：上传与 HTTP | `cmd/momiao/catalog_covers.go`、`cmd/momiao/catalog_assets.go` | `cmd/momiao/catalog.go`、`config.go`、`main.go`、`catalog_test.go`、`catalog_reader_test.go`、`catalog_browser_test.go`、`go.mod`、`go.sum`、`docs/deployment.md` |
| 3：真实管理与统一展示 | `web/src/OpsCatalogCovers.tsx` | `web/src/api.ts`、`catalog-api.ts`、`Catalog.tsx`、`OpsCatalog.tsx`、`ops/ops-content.css`，及下述现有前端测试 |

所有缩写文件名均归属本行对应目录。实施前重查迁移尾号；0040 若已占用，顺延迁移和授权脚本编号并更新计划。现有 `catalog-personas.json`、游戏、钱包、价格逻辑和全局布局不改。

### Task 1: 持久化家族绑定并接入公共读取

**Files:** 文件边界表第 1 行。借用现有 `catalogOpsFixture`、`catalogRuntimeStore`；不新建 fixture。

**Interfaces:** 在 `internal/platform/catalog_covers.go` 定义以下类型；字段 JSON 使用 snake_case，`Version`/`ExpectedVersion` 使用现有 `json:",string"` 十进制字符串约定，`Epoch` 对应 `authz_epoch` 数字。

```go
type CatalogCoverImage struct { AssetID, Src, Alt string; Width, Height int; FocalPoint [2]float64 }
type CatalogFamilyCover struct { Family string; Version int64; Image, DefaultImage *CatalogCoverImage }
type CatalogFamilyCoverPage struct { Principal AnnouncementPrincipal; Items []CatalogFamilyCover }
type CatalogCoverUploadCommand struct { OperationID string; Epoch int64; Family, Alt, RightsStatus, RightsNote, Reason string }
type CatalogCoverUploadImage struct { SHA256, ContentType, Extension string; Size int64; Width, Height int }
type CatalogCoverUploadResult struct { OperationID, AssetID string; Image CatalogCoverImage; RightsStatus, RightsNote string; Reused bool }
type CatalogFamilyCoverCommand struct { OperationID string; Epoch int64; Action, Family, AssetID string; ExpectedVersion int64; Reason string }
type CatalogFamilyCoverPreview struct { ID string; Before, After CatalogFamilyCover; ExpiresAt time.Time }
type CatalogFamilyCoverResult struct { OperationID string; Cover CatalogFamilyCover }

func CatalogCoverObjectKey(family, sha256, extension string) (string, error)
func (s *Store) CatalogFamilyCovers(ctx context.Context, userID int64) (CatalogFamilyCoverPage, error)
func (s *Store) RegisterCatalogCoverUpload(ctx context.Context, userID int64, c CatalogCoverUploadCommand, image CatalogCoverUploadImage) (CatalogCoverUploadResult, error)
func (s *Store) PrepareCatalogFamilyCover(ctx context.Context, userID int64, c CatalogFamilyCoverCommand) (CatalogFamilyCoverPreview, error)
func (s *Store) ExecuteCatalogFamilyCover(ctx context.Context, userID int64, c CatalogFamilyCoverCommand, previewID string, confirmed bool) (CatalogFamilyCoverResult, error)
```

`CatalogFamilyCoverPreview.ID` 序列化为 `preview_id`；`Image` 序列化为 `image`，无当前图时明确为 null。公共 `CatalogModel.FamilyCover *CatalogFamilyCover` 序列化为 `family_cover,omitempty`，新版已识别家族总是返回投影；草稿空家族保持 nil。上传结果不包含操作者或存储凭据。上传图焦点固定 `[0.5,0.5]`，默认图沿用原焦点，不新增裁剪编辑器。

- [x] **1.1 扩展现有失败用例。** 在 `TestCatalogOpsPublicationLifecycleAndIdentityHistory` 的已发布模型阶段插入上传登记、SET、RESET、重复操作与公共回读断言，之后继续原重命名、隐藏和退役断言；保留全部原覆盖。使用当前读取版本，不假定共享测试库家族版本永远为 1。

```go
// 在原用例内：after/result/list/detail 均取自真实 Store 调用。
if after.Cover.Version != before.Version+1 { t.Fatal("cover version did not advance") }
if detail.FamilyCover.Image.Src != list.Items[0].FamilyCover.Image.Src { t.Fatal("public cover diverged") }
if !reflect.DeepEqual(replay, after) { t.Fatal("receipt replay changed") }
if !errors.Is(staleErr, ErrCatalogConflict) { t.Fatal("stale family version overwrote current") }
if !errors.Is(reboundErr, ErrCatalogOperation) { t.Fatal("operation ID rebound") }
if reset.Cover.Version != after.Cover.Version+1 { t.Fatal("reset reused an old version") }
```

同一用例另断言上传后但 SET 前图不变、同家族同内容复用原登记元数据、另一家族的图片 ID 被拒绝、epoch 变化后确认失败、RESET 后 current 等于默认图或无默认的 null。新图登记的测试输入复用现有本地模型/管理员数据，内联构造哈希描述，不新增文件样本。

- [x] **1.2 运行 RED。** `go test -count=1 -timeout=90s -run '^TestCatalogOpsPublicationLifecycleAndIdentityHistory$' ./internal/platform`；缺少家族方法或上述断言失败为预期，记录实际输出。
- [x] **1.3 实现两张表和读取投影。** 迁移创建规格中的不可变 `family_cover_assets` 与预置家族行 `family_covers`；用 UNIQUE(family,sha256)、同家族复合外键、版本约束保证基本一致性。禁止 PUBLIC 权限，图片历史使用既有不可变触发函数。为 `CatalogModel` 增加字段，在列表读事务关闭 rows 后一次读取所需家族映射；详情在自身事务中解析。`loadCatalogModel` 的原命令读取不额外附带独立事务。
- [x] **1.4 实现登记、预览和确认。** `catalog_cover_ops.go` 复用 `lockCatalog`、授权行锁、全局 operation advisory lock 的既有顺序；R2 网络写入在登记事务外。操作 action 固定为 `MODEL_COVER_UPLOAD`、`MODEL_COVER_SET`、`MODEL_COVER_RESET`，model_id 留空，family 写入审计详情；先检查同主体同命令回执，再检查当前目标版本。预览沿用 10 分钟有效期、命令哈希及 epoch 校验；更新绑定和回执在同一事务内。
- [x] **1.5 扩展原运行角色和提交失败用例。** `catalogRuntimeStore` 仅增加新表最小授权；在 `TestCatalogOpsRunsUnderRuntimeGrants` 中通过独立运行角色登记/绑定/读取，并断言 DDL、改写图片历史/审计仍报错。在现有 `TestCatalogOpsCommitFailureDoesNotPartiallyPublishOrAudit` 中沿用同一提交失败机制核对封面和审计均未提交，不新建失败 fixture。
- [x] **1.6 运行 GREEN 并提交。** `go test -count=1 -timeout=90s -run '^TestCatalogOps(PublicationLifecycleAndIdentityHistory|RunsUnderRuntimeGrants|CommitFailureDoesNotPartiallyPublishOrAudit)$' ./internal/platform`；应 3 个既有用例 PASS、exit 0。同步前向最小授权脚本，执行 `git diff --check`，仅提交本任务文件，提交说明 `feat: persist model family cover bindings`。

### Task 2: 受保护的 R2 上传及家族管理 HTTP

**Files:** 文件边界表第 2 行；扩展 `catalogStore` 现有接口以暴露 Task 1 方法。

**Interfaces:** `cmd/momiao/catalog_assets.go` 提供具体存储对象，不新建通用存储接口。

```go
type catalogAssetConfig struct { AccountID, Bucket, CredentialsFile string }
type catalogCoverFile struct { Bytes []byte; Image platform.CatalogCoverUploadImage }
func loadCatalogAssetConfig(lookup func(string) (string, bool)) (catalogAssetConfig, error)
func newCatalogAssetStore(c catalogAssetConfig, client *http.Client) (*catalogAssetStore, error)
func (s *catalogAssetStore) Put(ctx context.Context, family string, f catalogCoverFile) error
func parseCatalogCoverUpload(w http.ResponseWriter, r *http.Request) (platform.CatalogCoverUploadCommand, catalogCoverFile, error)
func serveCatalogFamilyCovers(w http.ResponseWriter, r *http.Request, cfg config, userID int64)
```

配置键为 `MOMIAO_CATALOG_ASSET_R2_ACCOUNT_ID`、`MOMIAO_CATALOG_ASSET_R2_BUCKET`、`MOMIAO_CATALOG_ASSET_R2_CREDENTIALS_FILE`；凭据文件为严格 JSON `{access_key_id,secret_access_key}`。SDK 显式使用该凭据和 region `auto`、账号推导的 HTTPS R2 endpoint，不读取环境中的其它 AWS 凭据。`config` 保存 `CatalogAssets catalogAssetConfig` 与运行期 `catalogAssets *catalogAssetStore`，由 main 初始化；全缺表示未配置，部分提供或内容不合法返回配置错误。

HTTP 相对根为 `/platform/v1/ops/models/family-covers`：GET 根返回 `{principal,items,upload_enabled}`；upload 返回 Task 1 上传结果；prepare 返回预览；execute 返回确认回执。JSON 写入保持 `{command}` / `{command,preview_id,confirmed}` 包装。multipart 仅允许一个 `metadata` JSON 文本 part 和一个 `file` part，metadata 字段对应 `CatalogCoverUploadCommand`。

- [x] **2.1 增加本轮唯一新回归 `TestCatalogCoverUploadLifecycle`。** 放在现有 `cmd/momiao/catalog_test.go`，扩展已有 `catalogHTTPStore` 和 `catalogRoundTrip`；PNG/WebP 复用已跟踪的 `web/public/assets/models/kimi-family-normalized-v001.png`、`kimi-family-web-v001.webp`，JPEG 在测试内用标准库编码。损坏/超限输入在内存中派生，使用标准库 HTTP server，不创建 fixture 文件/新框架。围绕一条上传生命周期断言：

```go
if rejectedWrites != 0 { t.Fatal("invalid upload reached R2 or registration") }
if putRequest.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" { t.Fatal("cache contract") }
if uploadedSHA != calculatedSHA || putKey != expectedKey { t.Fatal("R2 payload or key changed") }
if storedBeforePutAcknowledgement { t.Fatal("unacknowledged object registered") }
if registeredAfterRevocation || bindingChangedAfterUpload { t.Fatal("upload crossed publication boundary") }
if replay.OperationID != first.OperationID || replay.AssetID != first.AssetID { t.Fatal("upload retry rebound") }
```

在同一用例中覆盖坏文件、超过 8 MiB、超过 8192 或总像素上限、PNG/WebP 动画、重复/额外 parts、声明了具体图片 MIME 却与实际格式不符；空 MIME 或 application/octet-stream 在字节验证通过时接受，兼容浏览器未识别文件类型。R2 错误保持旧绑定。用真实 HTTP server 将默认读写 deadline 在测试内缩短到毫秒级，令模拟 PUT 略长于旧 deadline，确认上传专用扩展奏效而普通路径保持原配置，不等待真实 10 秒。

- [x] **2.2 扩展既有边界与配置测试并运行 RED。** `TestCatalogHTTPProtectedWrites` 增加四条新路由的匿名、错误 Origin、缺少写/发布权限、非法查询和 JSON 确认断言，保留旧接口断言。`TestCatalogKeyFileAndTimingConfig` 增加全缺、部分配置、无效账号、超长/非普通凭据文件及未知 JSON 字段用例。命令：`go test -count=1 -timeout=30s -run '^Test(CatalogCoverUploadLifecycle|CatalogHTTPProtectedWrites|CatalogKeyFileAndTimingConfig)$' ./cmd/momiao`。
- [x] **2.3 实现受限文件读取与 R2 存储。** 请求总上限为 `8*1024*1024 + 64*1024` 字节，metadata 上限 16 KiB；用 MultipartReader 逐项解析并拒绝重复字段。家族来自固定枚举；alt 最多 120 字符，来源说明和原因最多 500 字符且非空，沿用现有纯文本校验。检查真实 PNG/JPEG/WebP 签名、尺寸后完整解码；PNG 的 acTL、WebP 的 ANIM/ANMF 或 VP8X 动画位按有界容器块遍历拒绝，截断/越界块返回格式错误。SHA-256 基于校验过的原字节，扩展名统一为 png/jpg/webp，经 Task 1 键函数写入，准确设置 MIME 和长缓存。SDK 使用显式客户端超时、至多两次尝试，认证与内部错误只返回稳定业务码。
- [x] **2.4 接入新路由而不放宽旧路由。** 保留 domainHandler 的认证投影；在模型 handler 中认证、Origin、权限预检之后，upload 专用分支在创建原 8 秒 context 前接管，其它方法仍按原规则。上传 context 上限 90 秒，R2 HTTP client 上限 60 秒；使用 `http.NewResponseController` 只为已授权上传设置读/写 deadline，保留服务器其它路径原 10 秒配置。登记时再次授权；上传不调用 SET。新 JSON 端点使用严格现有解码器和现有错误映射，返回 no-store。
- [x] **2.5 配置与部署说明随交付完成。** 只安装所列 SDK/解码依赖并锁定实际兼容版本，不修改无关依赖。main 在 platform 角色初始化存储，poker 角色拒绝这组配置。`docs/deployment.md` 记录密钥文件、仅上传路径需放行至少 9 MiB 请求及 95 秒代理等待的要求、版本路径缓存范围和新表授权；不改线上代理或桶配置。扩展现有浏览器验收入口，以显式同组配置启用真实 R2，默认仍禁用，不创建新的 fixture。
- [x] **2.6 运行 GREEN 并提交。** 重跑 2.2 的同一命令，要求 3 用例 PASS、exit 0；复核具体对象键与 PUT 字节/headers，无凭据输出。`git diff --check` 后提交本任务，说明 `feat: upload and publish model family covers`。

### Task 3: 后台图片管理、公共展示与真实联调

**Files:** 文件边界表第 3 行；复用 `web/src/OpsCatalog.test.tsx`、`Catalog.test.tsx`、`api.test.ts`、`opaque-session-flow.test.tsx`。不新增前端测试声明。

**Interfaces:** `catalog-api.ts` 定义 Task 1/2 DTO 的 TypeScript 对应类型（snake_case、version 字符串），并提供下列精确接口。API 辅助名称不改变现有 model 命令。

```ts
readFamilyCovers(client: ApiClient): Promise<CatalogFamilyCoverPage & {upload_enabled: boolean}>
uploadFamilyCover(client: ApiClient, command: CatalogCoverUploadCommand, file: File, isCurrent: () => boolean): Promise<CatalogCoverUploadResult>
prepareFamilyCover(client: ApiClient, command: CatalogFamilyCoverCommand): Promise<CatalogFamilyCoverPreview>
executeFamilyCover(client: ApiClient, command: CatalogFamilyCoverCommand, previewId: string): Promise<CatalogFamilyCoverResult>
OpsCatalogCovers(props: {client: ApiClient; initialFamily?: string; onClose: () => void; onChanged: (cover: CatalogFamilyCover) => void}): ReactElement
```

保持 `ApiClient.request` 签名不变，upload helper 把 FormData 传给其现有 body 参数并用 beforeSend 绑定 isCurrent。只在 `raw` 中为准确的 upload 路径、POST 和 FormData 保留二进制 body，并让浏览器生成 Content-Type boundary；该路径 timeout 为 95 秒，其它调用保持原 JSON 序列化和 25 秒 timeout。FormData 出现在其它路径时直接报请求格式错误。

- [x] **3.1 扩展既有 UI 用例并运行 RED。** 在 OpsCatalog 原唯一用例中保留“编辑→预览→发布→回读失败但回执保留”，再加入家族入口、文件选择、真实 FormData、CDN load 前禁用确认、SET/RESET 和只读权限；异步上传回来前切换家族时丢弃旧结果。在 Catalog 的 `selects exact channel model IDs inside one family without retaining stale detail` 和 `keeps the persona slot usable after both approved local images fail` 中加入新投影优先、同家族一致、新路径重置失败状态及默认图失败分支。

```ts
expect(uploadInit.body).toBeInstanceOf(FormData);
expect(new Headers(uploadInit.headers).has('Content-Type')).toBe(false);
expect(new Headers(uploadInit.headers).get('Accept')).toBe('application/json');
expect(firstFamilyImage.getAttribute('src')).toBe(secondModelImage.getAttribute('src'));
expect(executeBodies[0].command.expected_version).toBe(cover.version);
expect(screen.getByText(/已确认/)).toBeVisible(); // 随后的读取失败不抹掉回执。
```

- [x] **3.2 锁定共享客户端风险，复用原测试。** 在 `api.test.ts` 的 `does not replay POST on auth or ambiguous network failure`、`late protected response after logout is discarded` 中追加上传场景，原 JSON 场景保留；在 `opaque-session-flow.test.tsx` 的 `sends distinct platform and game CSRF headers on an opaque game write` 中追加普通上传只携带平台 CSRF、无 Native 凭据且旧游戏头断言不变。验证 401/网络失败不自动重放文件，退出后迟到响应不恢复草稿。
- [x] **3.3 实现公共渲染。** `CatalogPersona` 优先解析新 `family_cover.image`，随后 `default_image`，最后可访问文字；缺少整个新字段才走旧词表。支持严格 `/assets/models/uploads/<family>/<64hex>.(png|jpg|webp)` 和旧批准路径，不接受外链、反斜线、查询串或路径穿越。`assetUrl` 继续集中拼 CDN。以 family、version、src 改变重置图片错误状态；image 的 alt 来自登记文本，尺寸来自实际数据。
- [x] **3.4 实现后台面板。** 新组件集中管理家族、文件、上传编号、候选图、确认命令和回执；编辑模型处展示所属家族图并进入此面板，移除逐模型图片选择但提交时保留旧 asset_id。用现有 Modal、Alert、原生 file input，文件类型提示与服务端一致。封面替换/恢复默认必须走 prepare/execute；文件公开性说明、未配置状态、只读权限、加载失败、旧 epoch、异步切换和不确定结果均明确显示，重试使用原操作编号。样式仅加局部类，复用后台桌面外框/手机布局。
- [x] **3.5 执行最小前端 GREEN。** 首条命令仅运行 3 个既有模型 UI 用例；第二条仅运行 3 个已扩展的客户端边界用例。构建覆盖类型集成，不额外增加逐函数测试。

```sh
npm test -- src/OpsCatalog.test.tsx src/Catalog.test.tsx -t 'edits, previews and publishes a model|selects exact channel model IDs|keeps the persona slot usable'
npm test -- src/api.test.ts src/opaque-session-flow.test.tsx -t 'does not replay POST on auth or ambiguous network failure|late protected response after logout is discarded|sends distinct platform and game CSRF headers'
npm run build
```

预期分别为选中的 3 个既有用例 PASS、3 个既有用例 PASS、tsc/Vite exit 0；未匹配用例 skipped，不计作新覆盖。RED 时复用相同定向命令，记录具体新增断言失败，而非只记录“有失败”。

- [x] **3.6 完成真实联调。** 沿用 `TestCatalogBrowserFixture`，命令 `go test -v -count=1 -timeout=30m -run '^TestCatalogBrowserFixture$' ./cmd/momiao`，显式 `MOMIAO_CATALOG_BROWSER_FIXTURE=1` 和既有允许的独立 loopback 测试库。先确认所选库未含旧验收数据，不删除重建旧库；复用入口而非创建 fixture。使用真实 PostgreSQL、实际 Go handler、当前 web/dist 和专用新哈希 R2 对象：上传→预览→确认→匿名目录→同家族两种模型详情→刷新→恢复默认。桌面 1440×900、手机 390×844 截图对照，记录实际 CDN 状态/类型/Cache-Control/缓存响应/下载 SHA 与接口版本。
- [x] **3.7 完成阶段交付。** 真实 R2 凭据未就绪时如实保留“存储替身通过，R2 联调未验收”，不要用静态预览替代。更新交接与实施证据，扫描个人信息，保留基线哈希、修改包、patch、验证原样输出与独立副本回退证据；源码保留新版。仅提交本阶段文件并推送功能分支，不合 main、不部署、不删除旧素材；提交说明 `feat: manage family covers across ops and public catalog`。

## 验证预算与执行顺序

顺序固定为 Task 1 → Task 2 → Task 3，数据/接口类型有直接依赖，首版不并行实现。Go 命令使用当前已配置 Go 1.27.1，在仓库根运行；npm 命令在 web 目录运行。DB URL 仅进程环境注入，不写命令行、仓库或输出。

新增回归总数 **1**（Task 2），新增框架/产品 fixture **0**。Task 1 的 3 个数据库用例、Task 2 的 3 个 HTTP/配置用例和 Task 3 的 6 个前端用例均直接对应改动；基线只运行其中已存在的用例。未运行用例不称为通过；基线、RED、GREEN、回退输出分开保存，不在每个子步骤重复整组验证。

回退源码包仅恢复本轮原文件并移除本轮新文件，迁移脚本的源码回退不等于生产删表。数据库中的默认图恢复走版本化确认流程，R2 对象和审计保留。共享请求逻辑已由 Task 3 的定向边界用例约束；如实施发现还需修改 BFF/session 核心，先给出新风险与新增命令再扩大验证。

## 执行交接

用户已选择 **Native／本会话直接实施，最后统一复核**。三个阶段分别提交为 `bc6b72b`、`b9bf5a1`、`b1513b0`。一次 `gpt-6-sol / max` 终审的三项重要发现已修复：上传回执在网络写入前读取、POSIX 密钥文件拒绝组/其他用户权限、明确旧二进制不兼容已升级的迁移历史。仍只新增一条回归声明；额外复用现有 portal CSP 用例，并在 Linux 中只运行既有密钥配置用例。

真实 Go/PostgreSQL 本地验收使用专用合成身份，不连接真实模型；已完成 R2 上传、CDN 原字节和长缓存验证、预览确认、匿名列表及同家族两个型号详情回读、刷新、恢复默认，版本 `2 → 3 → 4`。新对象返回 `200 / image/webp / HIT`，SHA-256 与原文件相同，`Cache-Control: public, max-age=31536000, immutable`。原图与审计保留。凭据仅存本机私有目录，不进入仓库；生产认证/迁移/授权/代理配置及正式部署仍按原发布流程另行执行。

执行中补充 `MOMIAO_ASSET_CDN_ORIGIN`：实际 Go portal 的默认 CSP 为 self-only，必须明确允许同一 HTTPS CDN origin（仅 img-src），否则静态预览成功也不代表真实服务器可显示。其它 CSP 与认证行为不变。

源码回退包仅作用于独立副本，不降级数据库。旧基线在单独匹配的基线测试库通过；旧二进制对已含 0040 的库明确拒绝启动，已在部署说明中记录。
