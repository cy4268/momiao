# momiao

## Current source

The current source includes local authentication adapted from official NewAPI, Redis sessions, the model portal, wallet and game services, operations UI, and illustrated homepage. The old Native/BFF authentication bridge, Native source/adapter/deployment packages and legacy SUPER_ADMIN bootstrap tool are removed. Official NewAPI attribution is retained in [ATTRIBUTION.md](ATTRIBUTION.md). The one-time importer and historical schema remain for migration; the restricted dev release is deployed, while full acceptance and production cutover remain pending. Runtime configuration and operational records are deliberately not stored here. Earlier feature notes below describe historical milestones, not the current deployment status or a claim that every acceptance case passed.

Build the Platform with `go build ./cmd/momiao`. In `web`, install from `package-lock.json` and run `npm run build`; login uses the local Cookie/Redis entry point, not a Native credential bridge. See [deployment configuration](docs/deployment.md#local-session-startup) for the remaining session settings. No compiled binaries, images, data or credentials are included. Public illustrations under `web/public` are application assets, not runtime records.

### Game hall artwork

The game hall loads responsive, versioned artwork directly from a CDN. Set `VITE_ASSET_BASE_URL` at build time or in an ignored `web/.env.production.local`, for example `VITE_ASSET_BASE_URL=https://assets.example.com`. This is a public asset URL, never a credential. Keep installation-specific domains and storage credentials out of committed source.

The artwork manifest uses relative, content-hashed keys; host matching files under the configured base URL. Apply long-lived `public, max-age=31536000, immutable` caching to versioned assets and retain older hashes while deployed pages reference them. If an image fails to load, game titles, availability and navigation remain usable. The illustration source files and private delivery records are not part of this repository.


一个从实际可用功能开始生长的开源 AI 平台。从 **登录 → 模型 → API 接入 → 调用记录** 建立真实连接，并管理自己的 API 密钥和上游渠道，而不是静态页面或演示数据。

## 当前功能

- **登录**：本地密码、二步验证与 Discord 登录；HttpOnly Cookie 对应 Redis 会话，支持续期与退出。
- **指挥台**：本地账户身份、Reserve、请求数、密钥数量及最近个人记录。
- **密钥管理**：分页、创建、明确查看/复制、启停、确认删除；列表默认脱敏，离开弹窗清除页面中的明文。
- **调用记录**：个人记录分页、类型/模型/日期筛选，只展示调用元数据。
- **模型**：按已发布家族展示模型，支持搜索、详情中选择具体模型 ID 与 API 接入；不虚构价格或能力参数。
- **渠道管理**：本地管理员直接新建、编辑和测试渠道及分组，保存与审计同事务；已有密钥留空不修改。
- **交付层**：React + TypeScript 界面；单个 Go 服务提供 SPA、固定 Unix Socket 上游代理及存活探针。

模型调用直接扣 Reserve；签到、本地筹码兑换与服务端游戏结算保留。Native 额度划转和 refill 已删除。开发站已完成旧数据导入，生产旧服务与数据保留用于最终切换后的回退。见 [门户优先决策](docs/decisions/0002-usable-portal-first.md) 和 [模型闭环范围](docs/decisions/0003-model-workspace.md)。

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

使用 Go **1.27.3**、Node.js **24.22.1**。安全版本与上游合并检查见 [构建安全基线](docs/security-baseline.md)。在仓库根目录执行：

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

### 去 NewAPI：开发站已部署，生产待验收后切换

`internal/identity` 复用官方 `QuantumNous/new-api@f1164142` 的 bcrypt、TOTP/备用码和 Discord OAuth，只适配本地 PG 与 Redis，不使用 VPS 定制登录补丁。History、Bot 身份查询、Poker 和运营台均使用本地身份；旧 Native 源码、桥接和启动依赖已移除。

一次性导入器按 `identity` → `keys` → `workspace` → `prices` → `quota` 执行，各阶段独立事务，不是跨阶段事务。前四阶段要求目标表为空；最终额度按 1:1 units 合并到 Reserve，旧会话不迁移。开发站已完成导入与镜像切换，勿重跑。冻结、迁移、授权和运行配置见 [部署说明](docs/deployment.md#local-identity-migration-in-progress)。

Discord 通过 `MOMIAO_DISCORD_CONFIG_FILE` 提供私有 `client_id`、`client_secret`；注册还需 `guild_id`、`role_id`、`policy_version` 和 `MOMIAO_ADMISSION_ENABLED=true`。回调为 `MOMIAO_PUBLIC_ORIGIN` + `/oauth/discord`。新注册验证身份组后原子创建账号与准入回执；已有绑定账号不重复发赠额。独立 Poker 通过 `MOMIAO_POKER_SESSION_REDIS_CONFIG_FILE` 只读同一 Redis 会话。

运营台按本地角色直接保存，写入与审计同事务；删除、封禁、扣余额等操作有确认。模型中转使用本地 Key/分组/价格与官方协议适配，结束后扣 Reserve、更新 Key 用量、日志和归因；不预扣，余额不足扣到零。旧预览/fresh/操作回执与 Native 划转/refill 均已删除，历史记录保留。

开发验收尚待 2FA 登录、新 Discord 注册、改密/封禁会话失效、Poker 完整一局及可独立接入 dev 的 Bot。已通过的密码/Discord老账户登录退出、中转扣费、Key/Ops、排行榜、骰子/21点不重复执行。生产尚未切换，原 Native 容器和数据库保留用于回退。

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

打开 `/login`，HTTPS 入口使用本地 `__Host-chaldea_session` Cookie 与准确的 Public Origin。也可用 `MOMIAO_LISTEN_SOCKET` 替换 TCP 监听，供支持 Unix Socket 的入口直接连接，见 [部署说明](docs/deployment.md)。没有随仓库提供的账户、默认密码或数据库。门户已删除 Native Socket、身份投影与就绪探测；旧 Native 服务源码/工具与部署文件已清理，完整开发验收前不切生产。

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

`GET /v1/models` 使用本地 Key 认证和分组/模型限制，`/v1/*` 由已接线的本地中转处理。未注册的 `/api/` 路径直接返回 404，不再兜底代理到 Native；旧登录、渠道等路径没有兼容代理。门户文本测试与 `/pg/chat/completions` 转发已移除。旧原生接口契约随已移除的服务一起删除。

密码、2FA、Discord 登录及注册准入使用本地身份模块；来源固定为官方 f1164142。开发站已部署并完成部分实际验收；待验收项见运行节，源码测试不替代真实流程验收。

## 设计与贡献

- [六份设计归档](docs/design/archive/README.md)：保留长期产品与 Bright Moonlit 视觉方向，不等于当前实现清单。
- [迁移基线](docs/migration-baseline.md)：后续需要迁移时再继续；不包含生产数据。
- 保持改动小而完整：实现、契约和行为测试一起更新；不要提交凭据、数据库、原始业务日志、构建产物或第三方角色图片。

自有代码与文档使用 **AGPL-3.0-only**，见 [LICENSE](LICENSE)。外部 API、字体及素材说明见 [ATTRIBUTION.md](ATTRIBUTION.md)。仓库不包含 NewAPI 源码副本，也不提供第三方角色图像权利。

### Daily rewards and local exchange
The wallet page now supports an explicit daily 500 Reserve API Credit claim (Asia/Shanghai day), exact 1:1 Reserve/chips exchange, receipts and durable-key retry lookup. Native Active quota is unchanged. See [wallet action contract](contracts/wallet-actions-api.md). Initial grants, hourly/relief rewards, credit purchases and games are not enabled by this slice.

运营台已删除通用预览/执行/二次验证管道、访问控制页和操作回执页；所有模块及账户菜单的入口使用本地角色。历史审计和旧任务行保留只读，健康快照 worker 独立启动。认证启动已删除旧 Native/BFF 会话、独立会话数据库和 Native 凭据配置，直接复用平台连接池与 Redis；未注册的 `/api/` 路径返回 404。RP 记录与榜单直接读取本地归因，不再启动 Native 同步或检查其水位；部署前应用 0066，历史重复逻辑请求只计一次，原始行保留。quota/refill 启动与配置已删除，钱包只按本地余额兑换；旧 Native 投影/helper、transport/readiness 和部署文件已清理；开发站已使用本地镜像，生产保持原状。

### Local-only runtime image

`deploy/Dockerfile` consumes a build context with `bin/` (Linux amd64 `momiao`, `momiao-migrate`, `momiao-import-native`), `web/` (the Vite production output) and `licenses/` (this repository LICENSE/ATTRIBUTION.md plus `new-api/` from `docs/upstream/new-api`). Build Go with `CGO_ENABLED=0`, and the web with `VITE_MOMIAO_SESSION_MODE=opaque` and `VITE_ASSET_BASE_URL=https://assets.momiao.win`. Pass the committed source ID as `--build-arg REVISION=...`. The fresh Alpine image contains no Native-auth frontend, bootstrap binary or Native service; platform and Poker use the same image with their existing process-role configuration.

The current VPS uses the legacy Docker builder; the Dockerfile does not require BuildKit. Use `docker build --network host --build-arg REVISION=... -t IMAGE CONTEXT` there for package downloads; this build option does not change the runtime compose networks.
