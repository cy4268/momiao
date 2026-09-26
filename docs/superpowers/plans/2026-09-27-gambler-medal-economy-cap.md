# 赌怪勋章与资产封顶 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在保留原始游戏结果与账本的前提下实现 B 方案资产封顶、单人投入限制、永久「赌怪」勋章和可单独确认的一次性余额重置。

**Architecture:** 使用一份不可变经济政策及明确的封顶结算回执，让单人游戏、轮盘、德州和奖励共用金额计算与账户锁；不让通用账本静默修改调用者金额。一次性活动复用现有 Ops 授权、预览、确认及审计，排行榜读取永久持有记录。生产政策激活、发放和重置分别执行，开发阶段不开启生产写入。

**Tech Stack:** 现有 Go 1.27.1、PostgreSQL/pgx、React 19、TypeScript 6、Vite/Vitest、R2/CDN；不增加运行依赖或测试框架。

**Spec:** [用户已确认的设计规格](../specs/2026-09-27-gambler-medal-economy-cap-design.md)。计划基线 `83d3565c980fa1a19dfbc8c2f2cb7499fe4a7a64`；产品基线仍为 `5df845b1d114fe07db9ea38a0dec3d7e89dccbd4`。

状态：用户已批准本会话直接实施；六项实施、定向验证、双端核对与独立副本回退已完成；源码分阶段提交推送。一次 gpt-6-sol / max 复核发现 5 项，均已修正并定向验证。

## Global Constraints

- 勋章代码 `gambler-ruler-202609`，名称「赌怪」，资格为快照总资产严格 `> 500000000000000` raw；永久持有，一次性授予。
- 单人每局累计投入 `<= 500000000000` raw；含二十一点分牌和加倍。多人不套用该限额，保留原有房间和整数校验。
- 统一总资产上限 `50000000000000000` raw。B：只裁剪新增盈利，保留原始派彩、公平性、本金归还和独立的封顶差额。
- 后续重置 Reserve=`5000000000` raw、Available Chips=`0`；Native Active 不变。授予和重置是两个独立确认动作。
- 金额 JSON 为十进制字符串，前端 BigInt；SQL 跨账户合计用 numeric。Native 单次划转的 `2147483647` raw 上限不改。
- 规则沿用折叠展示，裁剪发生时结果区直接展示差额；排行榜数值仍每小时刷新，读取不触发重建。
- 图标复用既有 Ruler 透明 PNG，SHA-256=`e0c69ec268a86ee0230a62c5a51e4da1824639cd5ab513054fae77a45308d2c4`；通过 `VITE_ASSET_BASE_URL` 和 CDN 展示。
- 全计划最多新增 **1 个顶层回归测试**：`TestGamblerCampaignSnapshotResetLifecycle`；其余扩展下面点名的既有测试，不加新 fixture、框架或逐函数测试。
- 分支 `codex/gambler-medal-economy-cap`；每个完成阶段脱敏提交并推送，生产名单、密钥、截图内私有账户数据留在私有交付目录。开发不自动合 main、部署、授予或重置。
- 推荐本会话直接顺序实施，最后一次独立复核；如选择代理，统一 `gpt-6-sol / max`，不另开用户任务。本计划编写不派发代理。

## Review Focus

1. **跨玩法并发与锁反转**：一个账户同时收奖励、补足、结算和离桌，只消费一次剩余空间且不死锁 → Task 1 的既有 Economy/Exchange 测试、Task 3 的既有 Poker 钱包锁用例。
2. **钱仍在牌桌/托管/二十一点或 Native 状态不明**：既不漏算也不双计，失败前不揭晓新结果；本金回款保持原值 → Tasks 1–3 的既有真实 PG 用例。
3. **全额裁剪、未知提交与老回执**：零入账也有终态；同 key 返回原金额和政策，不按新余额重算 → Task 2 游戏/轮盘回放和 Task 5 唯一新增回归。
4. **德州独立进程及规则/入账差异**：只读额度调用不能变成写额度通道，底池原结果、实际桌上筹码与回放三者可核验 → Task 3 既有现金桌/历史用例，Task 5 唯一新增回归中的签名边界。
5. **重置中断、失效确认与公开隐私**：续跑不重置两次，Native 不变，旧奖励/划转不回流；公开只给勋章，不给名单余额 → Tasks 4–5 的既有榜单测试及唯一新增回归。

## 文件边界与依赖

| 任务 | 新建文件 | 主要修改文件 |
| --- | --- | --- |
| 1 共用经济边界 | `internal/platform/economy_policy.go`、`internal/platform/migrations/0041_economy_cap.sql`、`deploy/sql/runtime-grants-0041-economy-cap.psql` | `internal/platform/unified_assets.go`、`internal/platform/native_quota.go`、`internal/platform/api_chips_exchange.go`、`internal/platform/store.go`、`internal/platform/economy.go`、`internal/platform/rewards.go`、`internal/platform/admission.go`、`internal/platform/ops_economy.go` |
| 2 单人及轮盘 | 无 | `internal/games/seed.go`、`internal/games/service.go`、`internal/games/commitment.go`、`internal/games/registry.go`、`internal/games/service_types.go`、`internal/games/read.go`、`internal/games/blackjack_service.go`、`internal/games/history_detail.go`；`internal/roulette/service.go`、`internal/roulette/settlement.go`、`internal/roulette/config.go`、`internal/roulette/types.go`、`internal/roulette/store.go`、`internal/roulette/history.go`、`internal/roulette/ops.go` |
| 3 德州及进程接线 | `cmd/momiao/economy_cap_rpc.go`、`cmd/momiao/gambler_campaign_test.go`（全计划唯一新增回归） | `internal/poker/service.go`、`internal/poker/hand.go`、`internal/poker/funding.go`、`internal/poker/view.go`、`internal/poker/history_hand.go`、`internal/poker/history_types.go`；`cmd/momiao/config.go`、`cmd/momiao/main.go`、`cmd/momiao/poker_application.go`、`cmd/momiao/poker_process.go`、`cmd/momiao/poker_assertion.go`；`docs/deployment.md` |
| 4 勋章展示 | `internal/platform/migrations/0044_gambler_campaign.sql`、`deploy/sql/runtime-grants-0044-gambler-campaign.psql` | `internal/rankings/read.go`；`web/src/rankings/Rankings.tsx`、`web/src/rankings/rankings.css` |
| 5 活动及重置 | `internal/platform/gambler_campaign.go` | `internal/platform/ops_economy.go`、`internal/platform/ops_maintenance.go`、`internal/platform/active_quota_refill.go`、`internal/platform/admission.go`；`cmd/momiao/ops_economy_http.go`、`cmd/momiao/maintenance_worker.go`、`cmd/momiao/main.go`、`cmd/momiao/gambler_campaign_test.go`；`web/src/ops/OpsEconomy.tsx`、`web/src/ops/ops-api.ts` |
| 6 收执及交付 | 无 | `web/src/games-api.ts`、`web/src/Games.tsx`、`web/src/games/BlackjackGame.tsx`、`web/src/games/SlotGame.tsx`、`web/src/roulette/roulette-api.ts`、`web/src/roulette/RouletteRoom.tsx`、`web/src/roulette/RouletteRules.tsx`、`web/src/poker/LivePokerTable.tsx`、`web/src/poker/poker-ui-types.ts`、`web/src/history/api.ts`、`web/src/history/History.tsx`、`web/src/ops/OpsEconomyRecords.tsx`；相关既有测试与玩法文档 |

表内路径均从仓库根目录起算，任务内测试文件的缩写归属前一个完整目录。只改接入点，不顺手重构整套玩法；迁移编号实施前重查，已占用时顺延并更新两份授权脚本。0041 定义经济列、聚合与历史函数；定向 RED 需要的轮盘/Poker 守恒补充依次使用 0042/0043，活动数据顺延为 0044、排空能力 0045、验证游标 0046、未跟注本金 0047。0041–0045 已提交后保持 checksum 不变。新迁移中的政策默认未激活，未接完所有写路径前不打开开关。

## 定向验证约定

以下命令在仓库根目录执行；`go`/`npm` 使用已安装运行时。测试连接复用现有私有环境变量：`MOMIAO_TEST_DATABASE_URL`、`MOMIAO_GAMES_TEST_CONNECTION_FILE`、`POKER_TEST_CONNECTION_FILE`，仅指向现有 loopback `127.0.0.1:55432` 的隔离测试库。凭据不复制到计划；缺连接时先恢复既有配置，不把 SKIP 算 PASS。

本次触及共享钱包锁、跨进程额度读取及多人结算，定向验证覆盖这些实际调用方，比仅跑前端更广，但不执行 `go test ./...` 或无过滤的 `npm test`。每项先记录原有基线，再加断言得到 RED，最后记录 GREEN；新功能断言应因缺少行为失败，而非网络/配置故障。预期输出是命中测试全部 PASS、零 SKIP、退出 0；这些是将来验收标准，不是本轮结果。

### Task 1: 共用资产快照、经济政策与非游戏新增收入

**Files:** 文件边界表第 1 行。测试复用 `internal/platform/economy_test.go`、`api_chips_exchange_test.go`、`ops_v08_economy_component_test.go`；原有 helper 保留。

**Interfaces:** 在 `economy_policy.go` 固定以下接口，其他任务只调用，不复制算法：

```go
type EconomicPolicy struct {
    Version, Hash string
    SinglePlayerMaxUnits, AssetCapUnits int64
}
type CapSettlement struct {
    PolicyVersion, PolicyHash string
    TotalBeforeUnits, StakeUnits, GrossPayoutUnits int64
    CreditedPayoutUnits, WithheldUnits, ActualNetUnits, TotalAfterUnits int64
    QuantumUnits int64
    AssetsObservedAt time.Time
}
func ResolveEconomicPolicyInTx(context.Context, pgx.Tx, string) (EconomicPolicy, error)
func CapPayout(EconomicPolicy, int64, int64, int64, int64) (CapSettlement, error)
// CapPayout 参数顺序：policy, totalIncludingStake, stake, grossPayout, quantum。
func LockEconomyUsersInTx(context.Context, pgx.Tx, ...int64) error
func ReadUnifiedAssetsInTx(context.Context, pgx.Tx, NativeQuotaObserver, int64) (UnifiedAssets, error)
func RecordCapSettlementInTx(context.Context, pgx.Tx, string, string, int64, CapSettlement) error
// Record 参数尾部：sourceKind, sourceID, userID, decision；只登记，不自行派款。
func (s *Store) ConfigureEconomicObserver(NativeQuotaObserver) error
```

`native_quota.go` 增加 `NativeQuotaObserver`，只含现有 `ReadNativeQuota` / `QueryQuotaOperation` 两个签名；原 `NativeQuotaOperator` 嵌入它，写方法原样保留。旧 Native port/SQL adapter 均满足该只读接口。统一读取和 `apiChipsInFlight` 的只读部分改为消费 Observer，不触碰 apply 路径。Store 在启动 worker 前一次性配置 observer，用于已有 Store 直达领取和注册奖励路径；不在处理请求时修改该依赖。CapSettlement 是私有账务结构，禁止直接作为多人公开 JSON。

既有 `TestEconomyIntegration` 内首例的固定断言（不另立函数测试）：

```go
policy := EconomicPolicy{Version: "economy-cap-v1", AssetCapUnits: 50000000000000000}
got, err := CapPayout(policy, 49999999950000000, 50000000, 250000000, 1)
if err != nil || got.CreditedPayoutUnits != 100000000 || got.WithheldUnits != 150000000 || got.TotalAfterUnits != 50000000000000000 {
    t.Fatalf("capped settlement mismatch: %+v, %v", got, err)
}
```

- [x] **1.1 扩展既有 RED 断言。** 在 `TestEconomyIntegration` 追加规格四例、整筹码向下对齐及并发争抢最后 `50000000` raw 容量的检查；在 `TestAPIChipsAmbiguousRecoveryAndRefillFence` 检查资金只计一次、UNKNOWN 不返回有效总额；在 `TestV08OpsEconomyAdjustment` 检查超限整笔拒绝且旧回执仍可读。期望分别为 `credited=100000000, withheld=150000000, totalAfter=50000000000000000`（规格首例），并发总增加不超过剩余容量，超限调账无账本腿。
- [x] **1.2 运行定向 RED。** `go test -count=1 -timeout=90s -v ./internal/platform -run '^(TestEconomyIntegration|TestAPIChipsAmbiguousRecoveryAndRefillFence|TestV08OpsEconomyAdjustment)$'`。
- [x] **1.3 新增 0041 契约。** 增加 `economy.policy_versions`（不可变 `version/hash/canonical_json`）、`economy.policy_runtime`（唯一生效指针，初始为空）及 `economy.cap_settlements`（`source_kind/source_id/user_id` 唯一、完整 CapSettlement、不可变）。政策名称固定 `economy-cap-v1`；常量来自 Global Constraints，hash 用 canonical JSON 的 SHA-256。NULL/空旧绑定明确代表 legacy；非空未知版本报错。为单人 commitment/round、轮盘 round、poker hand 增加可空政策绑定，旧行不重新绑定。
- [x] **1.4 补齐统一资产。** 在 UnifiedAssets 增加 `SinglePlayerInFlightUnits`、`RouletteEscrowUnits`（JSON string），通过新的窄 `SECURITY DEFINER` 聚合 `economy.cap_local_assets_read(bigint)` 读取受核验的本地余额/投入和在途编号；固定 search_path、限制 EXECUTE、不给 Poker 直接读全平台私有表的权限。即时局在扣款前取 T；长局在最终派款前取 T 且本金仍计一次；归还/入账后不再计原本金。原 UnifiedAssetReader 委托新事务内函数，公开只读接口保持。
- [x] **1.5 接入共同锁与奖励。** 锁顺序统一为维护准入锁 → 已需的房间/局锁 → 用户升序 `quota-transfer-user` 锁 → business/idempotency 锁 → 资产固定顺序的钱包锁。`ApplyInTx` 只补共同账户锁，不裁剪 delta；所有上层提前读钱包的入口先拿账户锁，沿用 relief 的业务/幂等锁前置规则。Daily/Hourly/Registration/Relief 发放、正向 Ops 调账分别显式计算，不遗漏 Store 直达或后台重试入口；政策生效时缺 Native 观察器不得绕过封顶。零奖励登记已处理回执，重复领取不重算或补领被裁剪部分。
- [x] **1.6 GREEN 与提交。** 重跑 1.2；再仅对该组使用 `-race` 检查锁竞争。扩展旧金额/交易读取，使零入账回执不依赖 JOIN 非零 ledger 才存在。新增授权脚本在既有真实 runtime role 上验证读聚合可用、越权写 Native/改历史不可用。提交 `feat: add versioned economy cap accounting`，阶段不激活政策。

### Task 2: 单人限注与单人/轮盘真实封顶结算

**Files:** 文件边界表第 2 行；测试复用 `internal/games/service_test.go`、`extra_service_test.go`、`blackjack_service_test.go`、`cmd/momiao/roulette_test.go`。

**Interfaces:** 消费 Task 1 全部接口。`games` 和 `roulette` 分别增加 `NewServiceWithEconomy(store *platform.Store, keys Keyring, observer platform.NativeQuotaObserver) (*Service,error)`；旧 NewService 代理并只允许 legacy 模式，避免隐式测试旁路。在 platform 定义 `PayoutCapView{PolicyVersion,PolicyHash string; GrossPayoutUnits,CreditedPayoutUnits,WithheldUnits,ActualNetUnits int64}` 和 `func (c CapSettlement) PublicView() PayoutCapView`；JSON 键为对应 snake_case，金额标注 `,string`。GameRound、轮盘/德州结果及历史结果增加可空 `economy_settlement`（PayoutCapView），不携带全账户余额/资产分项/资格快照。Bootstrap/新局准入投影增加 `economic_policy`（version、hash、single_player_max_units、asset_cap_units、cap_mode=`CLIP_PROFIT`），前端从该版本读取限额，legacy 为空；多人不将 single_player_max_units 用作房间限额。旧字段保留原游戏算法语义。

- [x] **2.1 扩展既有 RED。** `TestWagerValidationBeforeAnyResult`：`500000000000` raw 允许、加一步拒绝且未消耗 commitment；`TestBlackjackPersistentActionsAndOriginalReplay`：分牌/加倍总投入越界无扣款，重复动作金额不再变化；`TestBlackjackConcurrentCreateAndDouble`：并发追加仍遵守总投入；`TestDiceServiceIntegration`、`TestSlotDurableAtomicSettlement`：绑定结果原派彩不变，实际派彩按规格减少且重复 key 返回相同差额。`TestRouletteEscrowReplayLifecycle`：两种轮盘均可超过单人百万限额，裁剪后终态/守恒/历史有效，取消原额退回。
- [x] **2.2 运行 RED。** `go test -count=1 -timeout=90s -v ./internal/games -run '^(TestWagerValidationBeforeAnyResult|TestDiceServiceIntegration|TestSlotDurableAtomicSettlement|TestBlackjackPersistentActionsAndOriginalReplay|TestBlackjackConcurrentCreateAndDouble)$'`；`go test -count=1 -timeout=90s -v ./cmd/momiao -run '^TestRouletteEscrowReplayLifecycle$'`。
- [x] **2.3 实现入口及政策绑定。** 在 commitment/新局受理前绑定经济版本/hash；二十一点所有追加入口对 `currentTotalStake+additionalStake` 做同一检查。旧 commitment 不跨新政策受理，由既有刷新流程换新 commitment。新局在 RNG 揭晓前完成资产依赖和整数检查。轮盘继续 `maximum_mode=NONE`，不改共享 wagering policy 为单人限额。
- [x] **2.4 实现显式结算。** 保留 derive/引擎的原结果；调用 CapPayout、登记回执、只将 CreditedPayoutUnits 传给正式账本，再提交终态/揭晓。单人 supply_events 用 ActualNetUnits；轮盘保留 gross outcome，新增对应销毁事实，`verifyFunding` 同时核对 gross=actual+withheld 与总守恒。非零退款不裁剪；零派彩复用既有无账本腿的正式交易。源重复请求在重算前读取原回执。
- [x] **2.5 对齐 SQL 历史并 GREEN。** 使用 Task 1 的 0041 已定义的 `games.history_source_rows`、`games.history_record_snapshot`、`economy.history_transaction_read` 新语义：私有 typed result 保留原结果，历史列表/榜单净额使用实际金额；旧版本仍走旧核验。检查恢复 worker、system void 与用户正常结算共用该路径，不靠前端过滤。重跑 2.2，提交 `feat: enforce capped game settlements and solo wagers`。

### Task 3: 德州每手封顶与独立进程只读额度

**Files:** 文件边界表第 3 行；复用 `internal/poker/service_integration_test.go`、`history_test.go`、`cmd/momiao/poker_config_test.go`。在此首次创建唯一新增回归 `TestGamblerCampaignSnapshotResetLifecycle`，先覆盖活动所需的只读 RPC；Task 5 只扩展同一个函数，补齐活动生命周期。

**Interfaces:** `poker.Options` 增加 `EconomyObserver platform.NativeQuotaObserver`，消费 Task 1。新 `cmd/momiao/economy_cap_rpc.go` 提供 `newEconomyQuotaReadHandler(observer platform.NativeQuotaObserver, peerKeys map[string]ed25519.PublicKey) http.Handler`；只支持精确 POST `/internal/v1/economy/quota/read` 与 `/internal/v1/economy/quota/query`。`newEconomyQuotaObserver(transport http.RoundTripper, signing pokerTicketKeys) (platform.NativeQuotaObserver,error)` 实现客户端。poker_assertion.go 抽出 `signServiceRequest(r *http.Request, body []byte, keys pokerTicketKeys, issuer, audience string) error` 及 `verifyServiceRequest(r *http.Request, body []byte, keys map[string]ed25519.PublicKey, issuer, audience string) error`；旧函数是固定 platform→poker 的薄包装，反向端口固定 poker→platform，方向不取自未验证请求头。

- [x] **3.1 编写 RED。** `TestRealPGCashTableFundingAndRecovery` 添加靠近上限的赢家，断言原底池分配不变、实际桌上筹码扣除 withheld、离桌只退实际额、重放不再次销毁；`TestPokerHistoryHandPG` 核对原底池证明、实际净利和差额；`TestRealPGBuyInLeaseLossWhileWalletLocked` 加共同账户锁路径，保持租约丢失整笔无效。`TestPokerApplicationConfigRequiresCompleteExplicitAuthority` 检查反向读取配置不完整时启动失败、Poker 仍拒绝 Native 写密钥配置。唯一新增回归先断言合法反向 read/query 成功、未签名/方向错误/改 body/apply 请求失败、旧正向签名仍有效，Native 写方法调用计数为 0。
- [x] **3.2 运行 RED。** `go test -count=1 -timeout=120s -v ./internal/poker -run '^(TestRealPGCashTableFundingAndRecovery|TestPokerHistoryHandPG|TestRealPGBuyInLeaseLossWhileWalletLocked)$'`；`go test -count=1 -timeout=120s -v ./cmd/momiao -run '^(TestPokerApplicationConfigRequiresCompleteExplicitAuthority|TestGamblerCampaignSnapshotResetLifecycle)$'`。
- [x] **3.3 实现只读接线。** Poker 通过新增 `MOMIAO_ECONOMY_READ_SOCKET` 连接平台私有监听器，复用既有 Service Assertion V1 canonical 编码与双方独立签名/公钥。原 signPokerRequest/verifyPokerRequest 硬编码了 platform→poker，不能原样用于反向调用；使用上述共享内部函数指定 poker→platform，并保留原包装和原验证严格度。无新签名协议，无 Native 写密钥共享。平台 handler 只校验 envelope 并调用 Native Observer，不再获取平台账户 SQL 锁，以免 Poker 持锁回调死锁。限固定路径、2 秒调用预算、16 KiB 回包、禁重定向/浏览器凭据；query 必须绑定原 operation/user。只读请求可按原参数重试，不生成写操作。
- [x] **3.4 每手结束原子调整。** 在 hand.go 结算持锁事务内，逐账户以手开始前绑定的政策计算 actual；保存原引擎快照、pots/awards/events，不改 RNG 或赢家。cap receipt 记录原结果与每人差额，实际 sessions.current_stack_units 使用已扣 withheld 的值；新一手从该实际余额开始。view/history 明确投影实际筹码且能回放到原结果，统计供应/销毁而非把钱送到别的座位；检查 uncalled return、平分底池与零利润均无裁剪。
- [x] **3.5 接线及 GREEN。** main.go 在启动注册/奖励/恢复 worker 前构造 observer、配置 Store 并注入单人/轮盘；独立 Poker 注入反向只读客户端，嵌入模式直接复用 observer。同步最小 SQL grants 和部署私有 socket 说明；复用 runtime 测试验证会话资金路径。重跑 3.2，只对钱包锁用例再用 `-race`。提交 `feat: cap poker winnings without changing pot outcomes`。

### Task 4: 永久勋章、资格存储与公开榜单图标

**Files:** 文件边界表第 4 行；测试扩展 `internal/poker/v07_rankings_regression_test.go` 与 `web/src/rankings/Rankings.test.tsx`。固定图标使用现有原稿，不再生图。

**Interfaces:** `rankings.Entry` 增加 `Badges []PublicBadge`，`PublicBadge{Code,Name,IconPath,Description string}` 对应 `code/name/icon_path/description`；旧数据返回空数组。前端只接受已登记的本枚代码和固定相对对象路径，使用已有 assetUrl；不接受客户端提交的授予字段。

- [x] **4.1 扩展既有 RED。** `TestV07RankingsPublicationAndRecovery` 加持有人/非持有人、昵称修改、跨榜单、刷新不改快照指针及响应无 user_id/资格余额断言。Rankings 现有旅程增加姓名旁“赌怪”、点击/键盘说明、图片失败文字保留和大金额 string/BigInt 断言，不新增 it。
- [x] **4.2 运行 RED。** `go test -count=1 -timeout=90s -v ./internal/poker -run '^TestV07RankingsPublicationAndRecovery$'`；`npm --prefix web test -- src/rankings/Rankings.test.tsx`。
- [x] **4.3 新增 0044 数据。** 增加 `economy.gambler_campaigns`（活动 ID、阶段/版本、资格截止点、重置检查点、政策、目标集合摘要）、`economy.gambler_campaign_accounts`（campaign/user 唯一、资格资产分项/状态、重置前后及事务 ID）和 `identity.account_badges`（user/code 唯一、活动与授予时间）。资格快照封存后不可改；执行进度字段可按版本前进。授予记录不可删改。只预置活动类型/代码，不在迁移中查询生产名单或授予。
- [x] **4.4 实现读取与图标。** 在 orderedEntries 中按稳定用户 ID 关联勋章，公开 DTO 不带资格快照；分页、历史榜、每小时发布不变。R2 对象键固定为 `ui/badges/gambler-ruler.e0c69ec268a86ee0.png`，上传前核对完整 SHA，保留旧图，Cache-Control 沿用 public/max-age=31536000/immutable；上传不会授予。前端使用真实 PNG、28–32px 视觉尺寸与足够点击区域，真实文字说明。
- [x] **4.5 GREEN 与提交。** 重跑 4.2；核对 CDN 响应、文件 hash 和清晰度，匿名用户看不到后台数据。提交 `feat: show permanent gambler medals on public rankings`。

### Task 5: 一次性资格快照、授予与可恢复重置

**Files:** 文件边界表第 5 行；扩展 Task 3 已建立的唯一新增 `TestGamblerCampaignSnapshotResetLifecycle(t *testing.T)`，仍放在 `cmd/momiao/gambler_campaign_test.go`。复用 `gameBrowserStores`、既有 Ops 公开构造器/确认流程和现有合成用户；不新增测试 fixture。

**Interfaces:** `platform.NewGamblerCampaignService(store *Store, assets *UnifiedAssetReader) (*GamblerCampaignService,error)`；服务提供 `OpsBindings() []OpsOperationBinding`、`Read(ctx context.Context, actor int64, campaignID string) (GamblerCampaignView,error)` 和 `RunBatch(ctx context.Context, campaignID string, limit int) (bool,error)`。GamblerCampaignView 包含 `id/version/phase/cutoff_at/target_count/completed_count/eligible_count/blocking_facts/native_unchanged`，数量为字符串，私有账户细目经 Ops 权限分页读取。

动作固定 `GAMBLER_SNAPSHOT_PREPARE`、`GAMBLER_MEDALS_GRANT`、`GAMBLER_RESET_PREPARE`、`GAMBLER_RESET_START`、`ECONOMY_POLICY_ACTIVATE`；均复用 Ops operation ID/version/epoch、SUPER_ADMIN、`economy.adjust`、原因、fresh auth 和 typed confirmation。对应授权不从客户端选择。既有 Ops HTTP mutation 路由承接动作；只补 GET `/api/v1/ops/economy/gambler-campaigns/{id}` 的私有读取。

- [x] **5.1 扩展唯一回归的活动 RED。** 在原测试库内通过真实 Ops Prepare/Execute 链建立两名资格边界用户及一个零余额账户；断言 `500000000000000` 不授予、`500000000000001` 授予、重复 GRANT 一条记录、重置后仍持有。每人 Reserve 精确 `5000000000`、chips=0、Native 快照完全相同。第一次 batch 后模拟进程中断，重启调用原活动 ID，已处理者无新账本腿；新 operation 对已完成活动也不能二次重置。
- [x] **5.2 在同一回归中补真实边界。** 过期预览/epoch/非管理员无资金效果；有旧在途、牌桌资金或 Native 不明时 RESET_START 被阻止；只读 RPC 验证未签名/改 path/改 body/错误 peer/错误 issuer-audience/apply 请求均无效果，合法 read/query 只观察，旧 platform→poker 调用仍通过；测试用户 ID 符合既有 Native int32 契约。公开排名响应包含勋章但无资格余额，多人局回执没有 TotalBefore/TotalAfter 等全账户信息。重置结果不进入游戏盈利或投注统计。测试不连接生产 Native/R2。
- [x] **5.3 运行 RED。** `go test -count=1 -timeout=120s -v ./cmd/momiao -run '^TestGamblerCampaignSnapshotResetLifecycle$'`。
- [x] **5.4 实现固定活动状态。** 单一流程 `DRAFT → SNAPSHOT_PREPARING → SNAPSHOT_READY → GRANTED → RESET_PREPARING → RESET_READY → RESET_RUNNING → COMPLETED`；遇到未决项记录阻塞事实并停在当前阶段，不跳状态。Snapshot/Reset 的受理只登记持久任务，由既有维护 worker 循环调用 RunBatch，每批最多 50 人。外层每阶段重新检查维护状态、账户集合摘要、操作授权；原已受理批次恢复使用原确认。固定资格截止点，不重新用当前余额推翻旧资格。
- [x] **5.5 正确冻结与排空。** 新局/资金/奖励使用既有维护 scope；补齐自动补足和注册奖励的维护准入缺口，已受理的恢复与退款继续。Native 模型流量不由 Go 平台维护 scope 自动阻止：重置手册要求在实际 API 入口暂停新消费准入并等旧请求结束，Native 只读额度接口继续可达。后台重置前后对每个目标再次核对 Native，证据变化时停止下一批并报告，不宣称原生不变。只在封存快照/全部重置完成后恢复相关准入。
- [x] **5.6 实现差额账本。** 原状态锁定后，`RESET campaign/user/asset` 稳定业务键通过 ApplyInTx 记实际差额；已等于目标写完成回执不写零 ledger。目标集合包括已有管理员/零余额用户；新注册不混进集合，旧未决奖励必须先排空。重置非游戏交易不会成为利润或新资格。所有目标完成且 Native/在途/账本核对通过后，独立激活已接通的政策并通过既有排行榜构建入口发布一次新资产快照。
- [x] **5.7 Ops UI、GREEN、提交。** 在 OpsEconomy 增加本活动面板，明确“预览/授予”和“重置”两组动作，展示人数/截止点/未决项/原生保留/差额及审计回执。继续复用现有 typed 确认组件；按钮不是前端直接写余额。重跑 5.3，提交 `feat: add audited gambler grants and resumable balance reset`。

### Task 6: 真实回执展示、独立副本演练与最终交付

**Files:** 文件边界表第 6 行。测试只扩展既有 `web/src/Games.test.tsx`、`games/BlackjackGame.test.tsx`、`roulette/Roulette.test.tsx`、`poker/LivePokerTable.test.tsx`、`history/History.test.tsx`；Task 4 的榜单测试不重复新增。

**Interfaces:** 统一解析新增 `economy_settlement` 字段，德州可选 `neutral_return_units` 单列中性本金；存在时使用 CreditedPayoutUnits/ActualNetUnits 显示入账，原派彩单列，缺失表示历史 legacy 而不是伪造为零。Ops 资产分项解析同时包含新单人/轮盘在途字段，核对总额时不遗漏。

- [x] **6.1 扩展既有 UI RED。** 复用现有 it，断言“原规则派彩 / 实际到账 / 封顶未入账”三项对应不同数值；21点追加越界按钮/说明与服务器一致；多人不显示百万上限；历史旧回执仍正常。点击折叠规则可读政策，实际裁剪差额在结果区可见；不恢复手动核对横幅。
- [x] **6.2 实现并 GREEN。** `npm --prefix web test -- src/Games.test.tsx src/games/BlackjackGame.test.tsx src/roulette/Roulette.test.tsx src/poker/LivePokerTable.test.tsx src/history/History.test.tsx`，随后 `npm --prefix web run build`。这组命令只在全部相关 UI 完成后统一跑一次，失败只重跑命中文件。
- [x] **6.3 独立副本演练。** 复用既有隔离数据库与脱敏合成账户，40→47 迁移校验旧 checksum、真实最小 runtime grants、未激活时旧游戏回执可读；执行完整授予→中断重置→原 ID 续跑→新政策激活。比较 Native、原账本行、旧游戏配置及历史 hash，只有新增合法记录/明确余额差额发生变化。记录本金守恒与封顶销毁；不以 owner-role 成功冒充 runtime-role 验收。
- [x] **6.4 双端截图与复核。** 实际浏览器桌面 1440×900/手机 390×844 核对姓名旁图标、点击说明、真实回执与 Ops 确认，遮挡私有账号资料后留证。按所选执行方式做一次独立整分支复核；若有代理仅 gpt-6-sol/max。只针对发现的风险补最小验证，超出本计划命令前先说明风险。
- [x] **6.5 打包回退并提交。** 私有交付目录保留基线 SHA、`MODIFIED_FILE.zip`、`DIFF_FILE.patch`、`VERIFICATION.txt`、可执行 `ROLLBACK.sh`；记录每条实际 BASELINE/MODIFIED/ROLLBACK 命令、输入、原样输出和退出码。回退仅对另一个源码/数据库副本演练并重新打开交付文件；生产维持现状。已结算数据以审计补偿处理，回退旧 UI 使用兼容新回执的后端，不旧库覆盖新流水。提交 `feat: display capped payouts and verify gambler campaign`，推送该阶段并核对远端 SHA；不把 Git 推送或 CI 绿当生产已上线。

## 计划自查与执行交接

| 规格条目 | 对应任务 |
| --- | --- |
| 统一资产、精度、B、本金与并发 | 1–3 |
| 百万单人累计、多人不限该值、版本/公平性/回放 | 2–3、6 |
| 永久勋章、严格十亿资格、CDN/公开隐私/小时榜 | 4–5 |
| 独立授予和重置、Native 不变、维护/排空/断点续跑 | 5–6 |
| 最多新增一条回归、真实权限/双端、版本交付与回退 | 全局约束、各任务命令、6 |

本计划创建时只做文档自查；后续用户已选择本会话直接实施并完成。原供选择的方式如下，实际执行采用第一项：

- **推荐：本会话直接实施，最后统一复核。** 六项共用经济政策/锁/回执接口，顺序实施可减少交接偏差；完成后一次独立整分支复核。
- **逐任务代理实施与复核。** 每项单独实现和检查，隔离更强但上下文与审查轮次更多；仍统一 gpt-6-sol/max。

实现与隔离验收已完成。生产发放、生产政策激活和余额重置仍需真实影响预览，分别得到明确执行指令；本次开发确认不代替这些最终操作。

## 实施偏差与最终核对记录

- 固定 current24 manifest 的 `TestPokerHistoryHandPG` 明确 SKIP，不计通过。原定历史验证合并到现有 `TestRealPGCashTableFundingAndRecovery`，使用当前真实迁移与权限覆盖公开历史、原始底池、实际现金退出及重放。
- 增加同一确认框架下的 `GAMBLER_REAUTHORIZE`：管理员 epoch/维护窗口变更可续跑原活动；保留旧审计、重新核对固定检查点、不二次重置。
- 每批及最终封存的 Native 核对均使用持久有界游标，现有唯一新增回归验证 limit=1 时最多两次 Native 读取。
- 重置影响以 numeric 精确汇总并输出字符串；后台可核对增加、减少与人数，Native 差额为零。UTC 输出修正也并入同一活动回归，避免客户端拒绝有效的时区时间。
- 前端五个点名文件共 57 条既有用例通过；终审后仅重跑命中文件。保留初次失败原因及原日志，不把失败/跳过伪称通过。没有新增第二个回归、依赖或测试框架。
- 独立数据库副本由 40 升至 47；原 40 checksum 与 130 张既有表行 hash 一致；另一个恢复副本保持 schema 40 和相同数据。所有生产步骤仍需各自明确确认。

源码回退：独立副本的 851 个修改后文件通过校验，逆向补丁恢复 830 个基线文件逐字节一致，原有 TestEconomyIntegration 在恢复副本通过；原工作源码保留改动。四件交付文件及完整命令/输出/退出码保留在任务私有交付目录。
