# momiao

一个从实际可用功能开始生长的开源 AI 平台。从 **登录 → 模型 → 在线测试 → 调用记录** 建立真实连接，并管理自己的 API 密钥和上游渠道，而不是静态页面或演示数据。

## 当前功能

- **登录**：原生密码与二步验证流程、刷新后续期、退出登录；访问令牌只保存在内存中。
- **指挥台**：真实账户身份、原生可用/已用额度、请求数、密钥数量及最近个人记录。
- **密钥管理**：分页、创建、明确查看/复制、启停、确认删除；列表默认脱敏，离开弹窗清除页面中的明文。
- **调用记录**：个人记录分页、类型/模型/日期筛选，只展示调用元数据。
- **模型**：按实际可用分组列出启用的模型，支持搜索、复制模型 ID 和进入在线测试；不虚构价格或能力参数。
- **在线测试**：使用原生登录态发送单轮文字请求，流式显示文本、停止生成和实际返回的用量；不额外创建永久 API 密钥，不在浏览器存储对话。
- **渠道管理**：管理员查看与启停；超级管理员新建、编辑基本 OpenAI 兼容渠道。已有密钥和未编辑的高级设置保持不变。
- **交付层**：React + TypeScript 界面；单个 Go 服务提供 SPA、固定 Unix Socket 上游代理及存活探针。

原生额度不是设计中的 Reserve 或 API Credit。充值、奖励、游戏与旧数据迁移尚未上线；本版不把这些功能替换成假数据。旧服务与旧数据独立保留，迁移不再阻塞改造。见 [门户优先决策](docs/decisions/0002-usable-portal-first.md) 和 [模型闭环范围](docs/decisions/0003-model-workspace.md)。

## 平台钱包一期

`internal/platform` 提供独立 PostgreSQL 的身份锚点、双资产零值钱包、精确金额、幂等单钱包账变与追加式流水；包含显式版本迁移和真实数据库集成测试。钱包页面 `/wallet` 已接入可选的独立平台数据库：显式初始化、真实余额与流水查询。没有开放资产变更接口或默认赠金，也不携带生产数据库凭据。完整经济系统仍未实现。

见 [账本基础与验证](docs/wallet-foundation.md)、[钱包接口契约](contracts/wallet-api.md)、[隔离与可用性决策](docs/decisions/0005-wallet-portal.md)。

## Master 展示身份

`/master-profile` 仅管理本人的昵称、静态系统默认头像与身份预览。服务端执行 Unicode 规范化、唯一性、保留名及7天改名冷却；首次初始化不消耗冷却。资料操作不更改登录用户名或钱包资产。现有门户不强制资料门禁；外部头像同步、上传、完整运营审核及其他用户公开页尚未开放。

见 [Master 契约](contracts/master-profile-api.md) 与 [切片裁定](docs/decisions/0006-master-profile.md)。

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

**只运行存活探针**：不设置额外环境变量，执行构建出的 `momiao`（Windows 为 `momiao.exe`），默认监听 `127.0.0.1:8080`。`GET/HEAD /healthz` 返回 `{"status":"ok"}`；该模式其他路径为 404。存活不代表上游或业务就绪。

**完整界面**：需要一个运行中的兼容 NewAPI，以及指向它的私有 Unix HTTP Socket。Linux 示例：

```sh
MOMIAO_WEB_DIR=/srv/momiao/web/dist \
MOMIAO_NEWAPI_SOCKET=/run/native-api/http.sock \
MOMIAO_LISTEN_ADDR=127.0.0.1:8080 \
./momiao
```

打开 `/login`。生产环境使用 HTTPS 入口并配置原生安全 Cookie 和准确的可信 Origin；原生认证 Cookie 的 Path 不作改写。也可用 `MOMIAO_LISTEN_SOCKET` 替换 TCP 监听，供支持 Unix Socket 的入口直接连接，见 [部署说明](docs/deployment.md)。没有随仓库提供的账户、默认密码或数据库。

| 环境变量 | 缺省 / 含义 |
|---|---|
| `MOMIAO_LISTEN_ADDR` | `127.0.0.1:8080`；数字 IP 与 `1..65535` 端口，IPv6 加方括号 |
| `MOMIAO_LISTEN_SOCKET` | 可选绝对路径；与显式设置的 TCP 地址互斥；新 Socket 为 0600，不覆盖已有路径 |
| `MOMIAO_WEB_DIR` | 可选绝对路径；必须存在且含 `index.html`，与上游 Socket 成对配置 |
| `MOMIAO_NEWAPI_SOCKET` | 固定上游绝对路径；浏览器不参与选择上游 |
| `MOMIAO_WALLET_DSN_FILE` | 可选受限平台 DSN 文件绝对路径；钱包与 Master 共用，与 Public Origin 成对 |
| `MOMIAO_PUBLIC_ORIGIN` | 与平台 DSN 成对的准确 HTTPS Origin，用于显式写入验证 |
| `MOMIAO_SHUTDOWN_TIMEOUT` | `10s`，可设 `1s..30s`；超时后强制结束剩余连接 |

空值与缺省不同：非法配置在监听前失败。不默认监听公网；服务自身不终止 TLS。Windows 主要用于开发和测试，完整 Unix 传输部署在 Linux 上验收。

## 接口与范围

SPA 路由：`/login`、兼容入口 `/sign-in`、`/dashboard`、`/keys`、`/logs`、`/models`、`/playground`、`/admin/channels`、`/wallet`、`/master-profile`；`/` 根据登录状态跳转。静态文件不暴露源码、目录列表或 source map。

`/api/`、`/v1/` 及确切的 `/pg/chat/completions` 原样转发到固定原生服务，前端不复制认证规则。适配版本和真实请求载荷见 [原生接口契约](contracts/native-api.md)。[OpenAPI](contracts/openapi.json) 声明 momiao 自有的 `/healthz` 、`/platform/v1/wallet` 与 `/platform/v1/master-profile` 接口，不把原生 API 冒充为自有实现。

代理 `/api/` 总上限 30 秒、`/v1/` 与在线测试总上限 5 分钟。它们是明确的工程上限，**不是并发容量或压测结论**；WebSocket 升级尚不支持。本版主动移除转发身份头，原生端看到的是内部代理地址，未宣称按真实客户端 IP 审计或限流。

登录实际入口使用密码。二步验证有源码契约和自动化覆盖，尚未使用真实已绑定账户验收；OAuth、Passkey、注册、密码找回和 CAPTCHA 控件不在本次范围。

## 设计与贡献

- [六份设计归档](docs/design/archive/README.md)：保留长期产品与 Bright Moonlit 视觉方向，不等于当前实现清单。
- [迁移基线](docs/migration-baseline.md)：后续需要迁移时再继续；不包含生产数据。
- 保持改动小而完整：实现、契约和行为测试一起更新；不要提交凭据、数据库、原始业务日志、构建产物或第三方角色图片。

自有代码与文档使用 **AGPL-3.0-only**，见 [LICENSE](LICENSE)。外部 API、字体及素材说明见 [ATTRIBUTION.md](ATTRIBUTION.md)。仓库不包含 NewAPI 源码副本，也不提供第三方角色图像权利。

### Daily rewards and local exchange
The wallet page now supports an explicit daily 500 Reserve API Credit claim (Asia/Shanghai day), exact 1:1 Reserve/chips exchange, receipts and durable-key retry lookup. Native Active quota is unchanged. See [wallet action contract](contracts/wallet-actions-api.md). Initial grants, hourly/relief rewards, credit purchases and games are not enabled by this slice.
