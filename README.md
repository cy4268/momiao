# momiao

## Current source

The current source includes local authentication adapted from official NewAPI, Redis sessions, the model portal, wallet and game services, operations UI, and illustrated homepage. The old Native/BFF authentication bridge, Native source/adapter/deployment packages and legacy SUPER_ADMIN bootstrap tool are removed. Official NewAPI attribution is retained in [ATTRIBUTION.md](ATTRIBUTION.md). The one-time importer and historical schema remain for migration; the integrated dev release and end-to-end acceptance are still pending. Runtime configuration and operational records are deliberately not stored here. Earlier feature notes below describe historical milestones, not the current deployment status or a claim that every acceptance case passed.

Build the Platform with `go build ./cmd/momiao`. In `web`, install from `package-lock.json` and run `npm run build`; login uses the local Cookie/Redis entry point, not a Native credential bridge. See [deployment configuration](docs/deployment.md#local-session-startup) for the remaining session settings. No compiled binaries, images, data or credentials are included. Public illustrations under `web/public` are application assets, not runtime records.

### Game hall artwork

The game hall loads responsive, versioned artwork directly from a CDN. Set `VITE_ASSET_BASE_URL` at build time or in an ignored `web/.env.production.local`, for example `VITE_ASSET_BASE_URL=https://assets.example.com`. This is a public asset URL, never a credential. Keep installation-specific domains and storage credentials out of committed source.

The artwork manifest uses relative, content-hashed keys; host matching files under the configured base URL. Apply long-lived `public, max-age=31536000, immutable` caching to versioned assets and retain older hashes while deployed pages reference them. If an image fails to load, game titles, availability and navigation remain usable. The illustration source files and private delivery records are not part of this repository.


一个从实际可用功能开始生长的开源 AI 平台。从 **登录 → 模型 → API 接入 → 调用记录** 建立真实连接，并管理自己的 API 密钥和上游渠道，而不是静态页面或演示数据。

## 当前功能

- **登录**：原生密码与二步验证流程、刷新后续期、退出登录；访问令牌只保存在内存中。
- **指挥台**：真实账户身份、原生可用/已用额度、请求数、密钥数量及最近个人记录。
- **密钥管理**：分页、创建、明确查看/复制、启停、确认删除；列表默认脱敏，离开弹窗清除页面中的明文。
- **调用记录**：个人记录分页、类型/模型/日期筛选，只展示调用元数据。
- **模型**：按已发布家族展示模型，支持搜索、详情中选择具体模型 ID 与 API 接入；不虚构价格或能力参数。
- **渠道管理**：管理员查看与启停；超级管理员新建、编辑基本 OpenAI 兼容渠道。已有密钥和未编辑的高级设置保持不变。
- **交付层**：React + TypeScript 界面；单个 Go 服务提供 SPA、固定 Unix Socket 上游代理及存活探针。

原生额度与 Reserve 分开记账，按显式单位换算。每日签到、本地兑换以及 Reserve → 原生可用额度的单向划转已接入；反向划转、自动补充、正式游戏结算与旧数据迁移尚未上线，V1 不做额度购买商城。旧服务与旧数据独立保留，迁移不再阻塞改造。见 [门户优先决策](docs/decisions/0002-usable-portal-first.md) 和 [模型闭环范围](docs/decisions/0003-model-workspace.md)。

## 平台钱包一期

`internal/platform` 提供独立 PostgreSQL 的身份锚点、双资产零值钱包、精确金额、幂等单钱包账变与追加式流水；包含显式版本迁移和真实数据库集成测试。钱包页面 `/wallet` 已接入可选的独立平台数据库：显式初始化、真实余额与流水查询。每日固定 500 Reserve 签到与本地 1:1 兑换见下方契约；没有默认赠金，也不携带生产数据库凭据。完整经济系统仍未实现。

见 [账本基础与验证](docs/wallet-foundation.md)、[钱包接口契约](contracts/wallet-api.md)、[隔离与可用性决策](docs/decisions/0005-wallet-portal.md)。

## 本地 Reserve

模型调用直接消耗 Reserve；Reserve 与可用筹码 1:1 双向兑换。`/wallet/activate`、Native 额度读取/划转接口及自动 refill 已删除；旧划转/跨库兑换行只作历史查询，不再由新程序启动 worker。上线前仍须处理历史在途并按计划一次性合并 Native 剩余额度。

## Master 展示身份

`/master-profile` 仅管理本人的昵称、静态系统默认头像与身份预览。服务端执行 Unicode 规范化、唯一性、保留名及7天改名冷却；首次初始化不消耗冷却。资料操作不更改登录用户名或钱包资产。现有门户不强制资料门禁；外部头像同步、上传、完整运营审核及其他用户公开页尚未开放。

见 [Master 契约](contracts/master-profile-api.md) 与 [切片裁定](docs/decisions/0006-master-profile.md)。

## 骰子体验

`/games/dice` 提供浏览器内的三骰模拟、大小/豹子判定、规则说明和最近 20 条本标签页记录。明确标注体验模式：不下注、不扣筹码、不发奖励、不改额度，不是正式游戏回执或服务端公平性证明。见 [体验范围](contracts/dice-experience.md)。

## 构建与验证

使用 Go **1.27.1**、Node.js **24.12.0**。在仓库根目录执行：

```sh
go vet ./...
go test -count=1 -timeout=30s ./...
go build -trimpath ./cmd/momiao
cd web
npm ci
npm run typecheck
npm test
npm run build
```

前端产物为 `web/dist/`。支持 race detector 的环境另执行 `go test -race -count=1 -timeout=30s ./...`。CI 分别验证 Linux、Windows Go 构建/测试、前端测试/构建及独立 PostgreSQL 钱包集成测试；不携带部署凭据、不自动部署。未配置 `MOMIAO_TEST_DATABASE_URL` 时，本地数据库集成测试明确跳过，不把普通 `go test` 通过当作数据库验收。

## 运行

### 去 NewAPI：本地身份迁移（开发中，尚未部署）

`internal/identity` 复用官方 `QuantumNous/new-api@f1164142` 的 bcrypt、TOTP/备用码和 Discord OAuth 逻辑，仅适配 pgx 与显式配置，不使用 VPS 的定制认证补丁。密码登录、五分钟 2FA 挑战、CSRF、退出与本地请求身份已接入源码中的主入口；复用平台 PG 连接池和现有 Redis，不再为这些请求签发 native 凭据。已用独立开发库的真实账号验证密码主流程。新增迁移 0048 和 `go build ./cmd/momiao-import-native`；默认 `-stage identity` 只导入用户与 2FA，保留 ID/密码哈希/备用码状态；Key 由独立 `-stage keys` 导入，分组/渠道/日志由 `-stage workspace` 导入；最终的 `-stage quota` 在旧服务停止并排空后按 1:1 units 合并剩余额度，复用原钱包账本与稳定业务号；不迁移旧会话。dev 目前只完成事务回退演练，尚未实际入账。使用方式见 [部署说明](docs/deployment.md#local-identity-migration-in-progress)。源码已接好本地 Discord 登录/注册、账号密码/2FA、前端与 self/Reserve 读取；网站镜像仍未切换，History、Bot、Poker 与各 Ops 模块已改为本地身份；Native 源码与启动依赖已清理，外部 Bot 接入及开发站整体验收仍待完成。

Discord 配置使用 `MOMIAO_DISCORD_CONFIG_FILE` 指向私有 JSON，包含 `client_id`、`client_secret`；开启 `MOMIAO_ADMISSION_ENABLED=true` 的注册还需要 `guild_id`、`role_id`、`policy_version`。回调地址固定为 `MOMIAO_PUBLIC_ORIGIN` + `/oauth/discord`。新用户验证服务器身份组后，在同一事务创建账号和本地注册回执，复用赠额 worker；已有绑定账号直接登录，不重复发赠额。0049 将回执读取迁入本地，不再读取 `MOMIAO_REGISTRATION_READER_KEY_FILE`。 Bot 也复用平台连接池查询 `identity.users`，不再读取 `MOMIAO_BOT_GAMES_NATIVE_DSN_FILE`；只保留 socket、token、quote key 三项私有监听配置。History 走同一本地 Cookie 鉴权，专用记录读写角色保持不变。Poker 复用现有连接票据，签发、接入和后续命令读取同一 Redis 本地会话与本地用户状态；删除 `MOMIAO_POKER_SESSION_READER_KEY_FILE`。独立 Poker 进程新增 `MOMIAO_POKER_SESSION_REDIS_CONFIG_FILE`，使用同一 Redis 的只读会话账号，配置与授权见部署说明。

运营维护页已改为本地管理员（role ≥ 10）的开关、可选提示语和范围直接保存；每次保存与一行 `ops.audit_log` 同事务提交。发布前应用 0050 及 `deploy/sql/runtime-grants-0050-local-ops.psql`。维护不再使用预约、预览或 fresh，已有业务准入锁继续复用。其他运营页面仍在迁移，整套开发验收前不切生产。 排行榜运营也已使用本地角色：直接创建重建任务、比较影子快照并确认发布；发布前应用 0059，将操作人外键改接本地用户，保留原聚合与发布历史。

用户运营页的 Master 资料已接本地角色与直接保存：设置改名要求或强制改名，不改变登录显示名和用户改名冷却。复用已有 Master 表、昵称规则和审计，无新迁移或授权；旧 Master Ops 绑定已删除，整站仍待开发验收后发布。

统一资产现只汇总本地 Reserve、可用筹码、Poker 桌面/底池、单人游戏在途和轮盘托管，救济与收益上限复用这套读取；本地兑换仅在 Reserve 与筹码间转移，不再向 Native 额度借支。经济运营页显示六项资产并直接调整可用筹码，复用原用户调账与审计。发布前应用 0064（新救济记录的历史 Native 时间戳允许为空），独立 Poker 无须 `MOMIAO_ECONOMY_READ_SOCKET`。

赌怪运营与批处理现使用本地 role >= 10，不依赖旧 Ops 预览、二次验证、epoch 或操作回执。0065 将活动执行人关联到 identity.users；历史资格、勋章和重置记录保持不可变，页面提供全局分页、时间/账户/资格/状态筛选与详情。重置仍单独确认，维护与游戏/资金在途检查保留；已受理的模型请求需由操作人先确认排空（没有新增自动请求计数器）。历史 Native 总额只供原资格展示，恢复只核对六项本地资产，不重算历史或强行接续变化的检查点。部署不会重新发奖或重置已完成活动；本批未发布网站，其余 Native 启动清理仍待完成。

操作日志 `/ops/audit` 使用本地管理员权限，统一显示直接保存日志与保留的历史审计，支持分页、时间、操作人和对象筛选；详情在弹窗查看。发布前应用 0051 和 `deploy/sql/runtime-grants-0051-ops-audit.psql`。`/ops/operations` 列表入口合并到此页；旧写流程所需的单条回执暂随未迁移页面保留。

API Key 管理已使用本地登录及 `relay.api_keys`，保留 `/api/token/` 的列表、创建、修改、查看与删除接口。用途随创建一起保存，修改用途不再经过 native 同步。发布前应用 0052 和 `deploy/sql/runtime-grants-0052-local-api-keys.psql`，再用 `momiao-import-native -stage keys` 单次导入未删除的旧 key；已导入的身份和余额不动。此阶段尚未接通本地模型中转与 Reserve 计费，须联合完成后发布。

调用日志、用户分组和可用模型接口已直接读取本地 `relay.logs/groups/channels`，self 的用量与请求数也由本地日志汇总；分组只返回账号所属组。发布前应用 0053 和 `deploy/sql/runtime-grants-0053-local-relay-workspace.psql`，用 `momiao-import-native -stage workspace` 向空的三张表单次导入全部日志元数据、分组和渠道配置。独立 dev 已导入 4 组、17 渠道、29279 条日志，保留模型映射、请求/头覆盖和多 Key 配置；不复制日志正文或 IP，不合并余额，不更换运行镜像。`/v1` 协议转发仍待接入。

定价与 Reserve 结算底层已接好：应用 0054 和 `deploy/sql/runtime-grants-0054-local-relay-billing.psql` 后，`momiao-import-native -stage prices` 单次把旧倍率转成 `relay.prices` 的最终单价。dev 已导入297条，其中32条按次、1条按视频时长；没有有效定价的模型不填免费价格。结算复用钱包事务，扣款不超过现有 Reserve，完整费用记入日志与Key用量，实际扣款写入归因；RP记录和排行榜同时读取历史与本地来源。协议适配和 `/v1` 路由尚未接线，本轮不发布网站镜像，也不进行余额合并。

本地中转前置现已提供 `PrepareRelay`：检查 Key/用户状态、期限、额度、模型限制、Reserve 与维护，再按所属组选择最高优先级渠道，同层按权重抽取；保留官方多 Key 状态/轮询语义、模型映射及覆盖配置。`GET /v1/models` 在 API-only 和 Web 路由中都只读本地数据，不接受浏览器身份或请求中的分组覆盖。小站暂逐次查库，不加 Key 缓存，停用立即生效。模型发现不扣费、不要求余额；实际转发必须先通过准入和定价检查。没有新迁移或重复导入；开发站仍运行旧镜像，待官方 relaykit 与扣费链路联合接通后发布。

**只运行存活探针**：不设置额外环境变量，执行构建出的 `momiao`（Windows 为 `momiao.exe`），默认监听 `127.0.0.1:8080`。`GET/HEAD /healthz` 返回 `{"status":"ok"}`；无后端配置时业务接口未就绪。存活不代表上游或业务就绪。

**本地登录界面**：需要已迁移的平台数据库与会话 Redis，Linux 示例（其余业务组件配置见部署说明）：

```sh
MOMIAO_WEB_DIR=/srv/momiao/web/dist \
MOMIAO_WALLET_DSN_FILE=/run/secrets/wallet.dsn \
MOMIAO_PUBLIC_ORIGIN=https://dev.momiao.win \
MOMIAO_SESSION_ENABLED=true \
MOMIAO_SESSION_REDIS_CONFIG_FILE=/run/secrets/session-redis.json \
MOMIAO_LISTEN_ADDR=127.0.0.1:8080 \
./momiao
```

打开 `/login`，HTTPS 入口使用本地 `__Host-chaldea_session` Cookie 与准确的 Public Origin。也可用 `MOMIAO_LISTEN_SOCKET` 替换 TCP 监听，供支持 Unix Socket 的入口直接连接，见 [部署说明](docs/deployment.md)。没有随仓库提供的账户、默认密码或数据库。门户已删除 Native Socket、身份投影与就绪探测；其余 Native 源码/工具与部署文件仍在清理，完整开发验收前不切生产。

| 环境变量 | 缺省 / 含义 |
|---|---|
| `MOMIAO_LISTEN_ADDR` | `127.0.0.1:8080`；数字 IP 与 `1..65535` 端口，IPv6 加方括号 |
| `MOMIAO_LISTEN_SOCKET` | 可选绝对路径；与显式设置的 TCP 地址互斥；新 Socket 为 0600，不覆盖已有路径 |
| `MOMIAO_WEB_DIR` | 可选绝对路径；必须存在且含 `index.html`，不再要求 Native Socket |
| `MOMIAO_SESSION_ENABLED` | `true` 启用本地登录，会话需要平台数据库、Public Origin 与 Redis |
| `MOMIAO_SESSION_REDIS_CONFIG_FILE` | 现有 Redis TLS 配置文件绝对路径 |
| `MOMIAO_WALLET_DSN_FILE` | 可选受限平台 DSN 文件绝对路径；钱包与 Master 共用，与 Public Origin 成对 |
| `MOMIAO_PUBLIC_ORIGIN` | 与平台 DSN 成对的准确 HTTPS Origin，用于显式写入验证 |
| `MOMIAO_SHUTDOWN_TIMEOUT` | `10s`，可设 `1s..30s`；超时后强制结束剩余连接 |

空值与缺省不同：非法配置在监听前失败。不默认监听公网；服务自身不终止 TLS。Windows 主要用于开发和测试，完整 Unix 传输部署在 Linux 上验收。

## 接口与范围

SPA 路由：`/login`、兼容入口 `/sign-in`、`/dashboard`、`/keys`、`/logs`、`/models`、`/admin/channels`、`/wallet`、`/master-profile`、`/games/dice`；`/` 根据登录状态跳转。静态文件不暴露源码、目录列表或 source map。

`GET /v1/models` 使用本地 Key 认证和分组/模型限制，`/v1/*` 由已接线的本地中转处理。未注册的 `/api/` 路径直接返回 404，不再兜底代理到 Native；旧登录、渠道等路径没有兼容代理。门户文本测试与 `/pg/chat/completions` 转发已移除。历史 [原生接口契约](contracts/native-api.md) 不代表当前认证入口。

密码、2FA、Discord 登录及注册准入使用本地身份模块；来源固定为官方 f1164142。开发站真实浏览器、第三方 OAuth、在途游戏等验收仍待完成，源码测试通过不等于网站已发布。

## 设计与贡献

- [六份设计归档](docs/design/archive/README.md)：保留长期产品与 Bright Moonlit 视觉方向，不等于当前实现清单。
- [迁移基线](docs/migration-baseline.md)：后续需要迁移时再继续；不包含生产数据。
- 保持改动小而完整：实现、契约和行为测试一起更新；不要提交凭据、数据库、原始业务日志、构建产物或第三方角色图片。

自有代码与文档使用 **AGPL-3.0-only**，见 [LICENSE](LICENSE)。外部 API、字体及素材说明见 [ATTRIBUTION.md](ATTRIBUTION.md)。仓库不包含 NewAPI 源码副本，也不提供第三方角色图像权利。

### Daily rewards and local exchange
The wallet page now supports an explicit daily 500 Reserve API Credit claim (Asia/Shanghai day), exact 1:1 Reserve/chips exchange, receipts and durable-key retry lookup. Native Active quota is unchanged. See [wallet action contract](contracts/wallet-actions-api.md). Initial grants, hourly/relief rewards, credit purchases and games are not enabled by this slice.

运营台已删除通用预览/执行/二次验证管道、访问控制页和操作回执页；所有模块及账户菜单的入口使用本地角色。历史审计和旧任务行保留只读，健康快照 worker 独立启动。认证启动已删除旧 Native/BFF 会话、独立会话数据库和 Native 凭据配置，直接复用平台连接池与 Redis；未注册的 `/api/` 路径返回 404。RP 记录与榜单直接读取本地归因，不再启动 Native 同步或检查其水位；部署前应用 0066，历史重复逻辑请求只计一次，原始行保留。quota/refill 启动与配置已删除，钱包只按本地余额兑换；其余 Native 投影/helper、transport/readiness 和部署文件仍待清理。0066 仅应用独立 dev，本轮无新增迁移或镜像发布。
