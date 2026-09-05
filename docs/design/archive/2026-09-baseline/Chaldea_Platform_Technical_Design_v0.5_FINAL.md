# Chaldea Platform — Technical Design v0.5 FINAL

> **历史参考归档（已脱敏）**：本文件的 FINAL / FROZEN 是历史状态，不代表当前实现或当前强制流程；现行决策优先。见[归档索引](../README.md)与[决策 0001](../../../decisions/0001-pragmatic-baseline.md)。`examples/` 路径仅为说明占位，相关部署、图片和私有文件不随仓库提供。

> 版本：`v0.5 FINAL`  
> 状态：`FINAL / TECHNICAL DESIGN COMPLETE`  
> 当前已冻结批次：`TD-01 — System Topology & Service Boundaries`、`TD-02 — Identity / Auth / Session / Return-to-Intent`、`TD-03 — Data Model / DB Ownership / Migration Framework`、`TD-04 — Economy / Ledger / Transfer / Reconciliation`、`TD-05 — Rewards / Initial Grant / Migration Grant`、`TD-06 — Game Platform / Registry / Config / Round Engine`、`TD-07 — V1 Direct Play Game Contracts`、`TD-08 — Poker Service / Realtime / Recovery`、`TD-09 — Rankings / History / Announcements / Jobs`、`TD-10 — Chaldea Operations / RBAC / Audit / Maintenance`、`TD-11 — Frontend Technical Architecture`、`TD-12 — Security / Observability / Deployment / Backup / DR`、`TD-13 — API Contract & Technical Design FINAL Audit`
> 冻结技术决定：`TD-FRZ-001 ～ TD-FRZ-552`  
> 下一阶段：`Implementation Spec v1.0`  
> 下一批：`TD-13 — API Contract & Technical Design FINAL Audit`  
> 上游权威链：`需求基线 v0.2.11 → IA v0.3.1 FINAL → Art Direction v0.4 FINAL → Technical Design v0.5`  
> 原则：本文件只累计已经由用户明确确认的技术设计决定；后续如修改已冻结决定，不静默覆盖，必须保留旧条目并标记 `SUPERSEDED`，同时新增新的 `TD-FRZ-*` 决策。

---

# 0. 文档工作规则

## 0.1 技术设计职责

Technical Design v0.5 的职责是实现并约束已经冻结的需求、IA 与视觉设计，不重新发明业务规则、页面结构、游戏数学、Poker 规则或视觉母方向。

如果技术实现发现上游不可实现、互相矛盾或存在严重安全问题：

1. 明确指出冲突；
2. 不静默修改业务语义；
3. 给出技术原因和替代方案；
4. 等用户确认是否回到对应上游文档进行版本化变更。

## 0.2 累计冻结规则

采用：

```text
完整设计一批
→ 用户确认
→ 立即写入累计 Technical Design Markdown
→ 更新 Change Log
→ 更新下一批
→ 继续
```

冻结编号连续使用：

```text
TD-FRZ-001
TD-FRZ-002
...
```

如以后修改：

```text
旧 TD-FRZ
→ 保留
→ 标记 SUPERSEDED

新决定
→ 新增新的 TD-FRZ
→ 写明替代对象、原因和影响范围
```

---

# 1. 已冻结系统基线

V1 总体继续采用：

```text
NewAPI Core
+
Chaldea Platform（模块化单体）
+
独立 Poker Realtime Service
```

技术栈基线：

```text
Frontend
React + Vite

Platform Backend
Go

Poker Realtime Service
独立 Go WebSocket Service

Database
PostgreSQL

Cache / Session / Lock / Realtime Ephemeral State
Redis

Reverse Proxy / TLS
Nginx 或 Caddy

Deployment
Docker Compose
```

V1 优先单 VPS 部署，推荐 8GB RAM，并保持在合理限额下 4GB RAM 环境仍可运行。

不为当前约 10–50 人同时在线规模提前引入 Kubernetes、Kafka、Service Mesh、大量微服务或其他不必要的分布式复杂度。

---

# 2. TD-01 — System Topology & Service Boundaries

> 状态：`FROZEN`  
> 用户确认：`方案 B 全部通过`

## 2.1 TD-01 目标

本批冻结：

- Frontend / Platform Backend / Poker Service / NewAPI / PostgreSQL / Redis / Reverse Proxy 拓扑；
- Public / Internal Trust Boundary；
- Browser 与各服务之间的公开接口边界；
- Chaldea Backend ↔ NewAPI 边界；
- Poker REST / WebSocket 边界；
- Session 在 Platform / Poker 之间的传递原则；
- Service-to-Service Authentication 基线；
- Docker Compose Network Boundary；
- Health / Readiness 基线；
- Failure Isolation；
- 未来拆分 Poker / PostgreSQL / Redis / NewAPI 时保持前端 Contract 稳定的原则。

本批不冻结：

- NewAPI 当前实际 Endpoint / Table / Field 名称；
- Auth Cookie / Token 精确格式；
- Poker Connect Ticket 精确 TTL / 签名算法；
- Wallet ↔ Poker 的最终事务与 Saga；
- PostgreSQL 具体 Schema / GRANT；
- WebSocket 消息协议；
- Health Endpoint 的最终 URL 与 Metric 细节。

这些分别进入后续 TD-02 / TD-03 / TD-04 / TD-08 / TD-12。

---

# 3. 拓扑方案比较

## 3.1 方案 A — 单域名单 Origin

示意：

```text
chaldea.example.com
├── Frontend
├── Platform API
├── Poker WS
└── NewAPI API
```

优点：

- DNS / TLS 最简单；
- 几乎无跨 Origin CORS 问题；
- V1 部署非常直接。

问题：

- Chaldea Web 与 NewAPI API 暴露面耦合；
- 容易误暴露 NewAPI 原始用户 UI；
- Cookie、安全策略与路由边界混杂；
- 长期拆分 NewAPI 时 Contract 较不清晰。

结论：`可行，但不采用。`

## 3.2 方案 B — Web Origin + API Origin

示意：

```text
chaldea.example.com
→ Chaldea Web / BFF / Poker WS

api.chaldea.example.com
→ NewAPI Model API
```

优点：

- Chaldea Web 保持单一 Web Origin；
- 普通网页与 NewAPI 外部模型 API 边界清楚；
- NewAPI Core 可以继续独立演进；
- API Base URL 对外稳定；
- 将来 NewAPI / Poker 拆机时无需改变 Browser Product Routes；
- 最符合“NewAPI Core 尽量少侵入 + Chaldea 是完整独立产品”的上游方向。

成本：

- 比方案 A 多一个 Host / DNS / TLS Routing；
- 需要明确 NewAPI Public API Allowlist。

结论：`FROZEN / SELECTED`

## 3.3 方案 C — 每服务独立 Subdomain

示意：

```text
app.example.com
platform-api.example.com
poker.example.com
api.example.com
ops.example.com
...
```

优点：

- 服务隔离最明显；
- 适合未来大规模独立部署。

问题：

- V1 CORS / Cookie / WS Auth / DNS / TLS / 运维明显复杂化；
- 对 10–50 人同时在线规模属于过度设计。

结论：`V1 不采用。`

---

# 4. TD-01 最终推荐拓扑

```text
                         PUBLIC INTERNET
                              │
                    HTTPS / WSS :443
                              │
                    ┌──────────────────┐
                    │ Edge Reverse     │
                    │ Proxy            │
                    │ Nginx / Caddy    │
                    └────────┬─────────┘
                             │
        ┌────────────────────┼────────────────────────┐
        │                    │                        │
        ▼                    ▼                        ▼

chaldea.example.com    api.chaldea.example.com   NewAPI Admin
        │                    │                  独立入口 / Host
        │                    │
        │                    └───────────────► NewAPI Core
        │                                      │
        │                                      ├─ newapi DB
        │                                      └─ Redis / newapi:*
        │
        ├── /                  → React / Vite Static Frontend
        │
        ├── /ops/*             → 同一 Frontend / Operations Shell
        │
        ├── /api/*             → Chaldea Platform Backend
        │                         │
        │                         ├─ chaldea_platform DB
        │                         ├─ Chaldea Redis namespaces
        │                         ├─ NewAPI Adapter
        │                         │      └──► NewAPI Core
        │                         │
        │                         └─ Poker Internal API
        │                                └──► Poker Service
        │
        └── /ws/poker          → Poker Service
                                      │
                                      ├─ chaldea_platform DB
                                      │   authoritative poker data
                                      │
                                      └─ Redis
                                          realtime / locks /
                                          ephemeral state
```

公网暴露基线：

```text
Host 80/443
→ Reverse Proxy only
```

默认不直接向公网发布：

```text
Platform Backend Port
Poker Internal HTTP Port
PostgreSQL
Redis
NewAPI Internal Bridge
```

---

# 5. Public Origin Contract

## 5.1 Chaldea Web Origin

推荐逻辑 Host：

```text
https://chaldea.example.com
```

负责：

```text
/...
→ React Frontend / IA Routes

/ops/*
→ Chaldea Operations Frontend Shell

/api/*
→ Chaldea Platform Backend BFF

/ws/poker
→ Poker WebSocket Service
```

`chaldea.example.com` 是普通用户 Chaldea Platform 的统一 Web Origin。

现有 IA 路由语义保持不变，例如：

```text
/
/dashboard
/models
/wallet
/rewards
/games/:game_slug
/poker
/poker/table/:id
/rankings
/history
/ops/*
```

物理服务拆分不得改变这些产品级路由。

## 5.2 NewAPI Model API Origin

推荐逻辑 Host：

```text
https://api.chaldea.example.com
```

只承担：

- SillyTavern；
- SDK；
- CLI；
- 代码；
- 其他外部 API Client

对 NewAPI Model API 的调用。

它不是 Chaldea Browser 的页面业务 Backend。

Chaldea Frontend 不应为了读取用户 Wallet、Rewards、Games、Master、Operations 等页面数据而直接调用该 Origin。

## 5.3 NewAPI Admin

NewAPI Admin 保持独立入口、独立权限、独立认证边界。

Chaldea Operations 可以保留 `Open NewAPI Admin ↗` Cross-link，但：

- 不 iframe 嵌入完整 NewAPI Admin；
- 不将 Chaldea Operations Session 自动视为 NewAPI Admin Session；
- 不因为用户是 Chaldea Super Admin 就自动授予 NewAPI Admin；
- 不因为用户是 NewAPI Admin 就自动授予 Chaldea Operations 权限。

---

# 6. Browser → Backend BFF Boundary

除 Poker WebSocket 外，Chaldea Browser 的业务请求统一经过 Platform Backend。

正式方向：

```text
Browser
   │
   ▼
Chaldea Platform Backend
   │
   ├── Chaldea Modules
   ├── NewAPI Adapter
   ├── Chaldea DB
   ├── Redis
   └── Poker Internal API
```

Browser 禁止直接访问：

```text
NewAPI Database
Chaldea Database
Redis
NewAPI Internal Admin API
Poker Internal REST
```

Platform Backend 作为 Chaldea Web 的 Business / BFF Boundary，后续统一承载：

- Authentication Context；
- Authorization；
- Return-to-Intent；
- Fresh Authentication；
- CSRF；
- Rate Limit；
- Error Mapping；
- Request ID；
- Audit Context；
- Chaldea Business Validation。

这些细节进入 TD-02 / TD-10 / TD-12。

---

# 7. NewAPI Integration Boundary

## 7.1 核心原则

Chaldea 继续通过稳定 `newapi_user_id` 关联 NewAPI 用户。

Chaldea Runtime 不得因为实现方便而获得 NewAPI 核心数据库的通用 UPDATE 权限。

NewAPI Adapter 的优先调用顺序：

```text
Verified NewAPI HTTP / Internal API
        ↓
受控 Narrow Internal Bridge
        ↓
必要时最小权限 Read-only DB Access
```

## 7.2 Runtime Read

可用于后续核验的读取领域包括：

- Account State；
- API Key Metadata；
- Model / Pricing / Availability；
- API Usage Metadata；
- NewAPI Raw Quota。

是否使用 HTTP API、Bridge 或 Read-only DB，必须以用户实际部署的 NewAPI 版本源码核验结果为准。

不得凭模型记忆写死 Endpoint / Field / Table。

## 7.3 Runtime Write

正常运行时：

```text
Chaldea Module
   ↓
NewAPI Adapter
   ↓
Verified NewAPI API
   or
Narrow Internal Bridge
   ↓
NewAPI owns final write
```

禁止：

```text
Wallet Module
→ direct UPDATE newapi tables

Rewards Module
→ direct UPDATE newapi tables

Auth Module
→ direct password table mutation
```

如果实际 NewAPI 缺少必要受控写接口，优先设计窄范围 Internal Bridge，而不是向所有 Chaldea 模块授予 DB Write。

## 7.4 Migration Exception

Cutover / Migration 是独立维护场景。

如 TD-03 核验后确认某些迁移步骤必须直接操作 NewAPI 数据，可以使用：

```text
temporary migration credential
+
maintenance window
+
migration_batch_id
+
pre-check / backup
+
audit
+
post-check
+
rollback boundary
```

Migration Credential：

- 平时禁用；
- 不作为 Platform Backend Runtime Credential；
- 权限只覆盖迁移所需对象；
- 使用后撤销 / 关闭。

---

# 8. PostgreSQL Ownership Boundary

V1 使用：

```text
同一 PostgreSQL Instance

newapi
→ NewAPI Core

chaldea_platform
→ Chaldea Platform
```

推荐角色基线：

| Role | Ownership / Access | Baseline Permission |
|---|---|---|
| `newapi_app` | `newapi` | NewAPI 正常 RW |
| `chaldea_app` | `chaldea_platform` | Platform Backend 正常 RW |
| `chaldea_newapi_ro` | 经核验允许读取的 NewAPI 对象 | SELECT only |
| `chaldea_poker` | Chaldea 中 Poker-owned data | Poker scoped RW |
| Migration Role | 迁移批次专用 | 临时最小权限 |

具体 Schema、GRANT、View、Ownership 与 Migration Role 在 TD-03 冻结。

---

# 9. Poker Data / Economy Boundary

Poker Service 是独立实时服务，但不能形成第二套资产真相。

Poker Service 负责的正式领域包括：

- Poker Lobby；
- Table；
- Seat；
- Poker Session；
- Hand；
- Action；
- Action Timer；
- Reconnect；
- Take Over；
- Spectator；
- Table Chat；
- Hand / Session Recovery；
- Poker Settlement Orchestration。

Poker Service 可以对自己拥有的 Poker 数据进行正式持久化。

但是 Poker Service 不获得对普通 Wallet Balance / Ledger 的任意修改权限。

涉及：

```text
Available Chips
↕
Poker In Play
```

的：

- Buy-in；
- Top-up；
- Safe Leave；
- Cash Out；
- Settlement Boundary

必须进入正式 Economy Service / Ledger / Transaction Boundary。

最终数据库事务、锁、Saga、Exactly-once 语义由 TD-04 + TD-08 冻结。

---

# 10. Redis Boundary

V1 使用同一个 Redis Instance，但通过 Namespace / ACL 进行逻辑隔离。

建议 Namespace：

```text
newapi:*

chaldea:session:*
chaldea:auth-flow:*
chaldea:return-intent:*
chaldea:cache:*
chaldea:lock:*
chaldea:poker:*
```

Redis 允许保存：

- Cache；
- Session；
- Auth Flow；
- Return-to-Intent 短期状态；
- Business / Distributed Lock；
- WebSocket Presence；
- Poker Realtime Ephemeral State；
- Reconnect 辅助状态；
- 非最终房间缓存。

Redis 不得成为以下事实的唯一来源：

- Wallet Final Balance；
- Wallet Ledger；
- Transfer Final State；
- Reward Final Claim；
- Direct Play Final Round；
- Poker Final Hand；
- Poker Final Settlement；
- Poker Final Session Asset State。

正式原则：

```text
Redis
= acceleration / coordination / ephemeral

PostgreSQL
= durable authority
```

---

# 11. Poker REST / WebSocket Boundary

## 11.1 Durable / Lower-frequency Poker Operations

Browser 通过：

```text
Browser
   ↓
Chaldea /api/*
   ↓
Platform Backend
   ↓
Poker Internal API
```

处理适合 HTTP / durable command 的领域，例如：

- Lobby Discovery；
- Table Discovery；
- Table Preview；
- Create Table；
- Seat Reservation；
- Active Session Discovery；
- History；
- Durable Management Commands。

具体 Endpoint 不在 TD-01 冻结。

## 11.2 Realtime Table Operations

牌桌实时路径：

```text
Browser
   ↓
wss://chaldea.example.com/ws/poker
   ↓
Reverse Proxy
   ↓
Poker Service
```

WebSocket 领域包括：

- Realtime Action；
- Timer；
- Table Snapshot；
- Reconnect；
- Take Over；
- Spectator Updates；
- Table Chat；
- Realtime Table Events。

WebSocket Message Contract 在 TD-08 冻结。

---

# 12. Platform Session → Poker Authentication Boundary

Poker Service 不直接依赖或解析完整 Browser Session 内部状态。

采用：

```text
Browser
   │
   │ normal authenticated HTTP
   ▼
Platform Backend
   │
   │ mint short-lived Poker Connect Credential
   ▼
Browser
   │
   │ WebSocket handshake
   ▼
Poker Service
```

Poker Connect Credential 至少表达：

- stable user identity；
- Platform session binding / session identity；
- `purpose = poker`；
- expiry；
- nonce / replay boundary；
- connection authorization context。

精确：

- TTL；
- 签名 / MAC / key strategy；
- single-use 与 reconnect 语义；
- Take Over 交互；
- Rotation；
- Revocation

分别进入 TD-02 与 TD-08。

目标：

> Poker 未来迁移到独立主机时，不需要重新设计 Browser 的主 Session Contract。

---

# 13. Service-to-Service Authentication

即使所有服务位于同一 Docker Host，也不得把“Docker 内网不可被普通公网直接访问”当作唯一认证。

V1 基线：

```text
Docker Network Isolation
+
Service Identity / High-entropy Service Credential
+
Endpoint Allowlist
```

至少覆盖：

```text
Platform Backend
→ Poker Internal API

Platform Backend
→ NewAPI Internal Bridge
```

V1 不要求引入完整 Service Mesh。

未来真正多主机拆分后，可以升级为：

```text
Private Network
+
TLS / mTLS
```

上层业务 Contract 不因此改变。

---

# 14. Docker Compose / Network Boundary

保持两个独立部署项目：

```text
examples/deployment/external-newapi
examples/deployment/platform
```

不把整个 NewAPI 项目强行揉入 Chaldea 的单一超级 Compose 生命周期。

允许二者通过受控 External Docker Network / Shared Infrastructure Network 互联。

逻辑 Network 分层：

```text
edge
application
data
```

示意：

```text
              Host
               │
        only 80 / 443
               │
        Reverse Proxy
          │         │
          │ app_net │
          ▼         ▼
   Platform       Poker
   Backend        Service
      │              │
      ├──── data_net ┤
      │              │
      ▼              ▼
 PostgreSQL        Redis
      ▲              ▲
      │              │
      └──── NewAPI ──┘
```

默认：

```text
PostgreSQL
→ no host published port

Redis
→ no host published port

Poker Internal HTTP
→ no host published port

Platform Backend
→ no host published port

NewAPI Internal Bridge
→ no host published port
```

公网请求必须先经过 Edge Reverse Proxy。

---

# 15. NewAPI Public Route Allowlist

普通用户不能看到 NewAPI 原始用户 UI。

因此禁止：

```text
api.chaldea.example.com/*
→ unrestricted entire NewAPI application
```

采用：

```text
api.chaldea.example.com
   │
   ├── verified public model/API routes
   │       → NewAPI Core
   │
   └── unrelated UI/admin/internal routes
           → reject / safe handling
```

具体 Public API Path Allowlist 必须读取实际部署 NewAPI 源码后确认。

TD-01 不猜测真实 `/v1`、`/api` 或其他 Endpoint。

---

# 16. Failure Isolation

## 16.1 Platform Backend Failure

影响：

- Chaldea 个性化业务 API；
- Wallet / Rewards / Games / Operations 等 Platform Backend 能力。

原则：

- NewAPI Model API 应保持独立；
- 不因 Platform Backend 故障主动终止已经在 Poker Service 内处于正式恢复逻辑中的 Poker Hand；
- 前端显示 Platform Domain Degraded / Unavailable。

## 16.2 Poker Service Failure

影响：

- Poker Lobby / Table / WS；
- Poker Realtime Operations。

原则：

- Poker 进入 Paused / Recovering；
- Wallet；
- Rewards；
- Direct Play；
- Models / API；
- NewAPI Model API

不得因为 Poker Service 单模块故障被整体关闭。

## 16.3 NewAPI Core Failure

影响：

- Model API；
- Account/Auth 依赖 NewAPI 的能力；
- API Key / Model runtime 等依赖项。

原则：

- 不自动关闭 Chaldea 所有内容或不依赖 NewAPI 的产品域；
- 根据实际 Dependency Map 做 Partial Degradation；
- 不伪造账号、余额或 API 状态。

## 16.4 Redis Failure

允许造成：

- Session / Cache / Lock / Realtime State 降级；
- Poker Reconnect / Presence 需要恢复；
- 部分业务暂时不可执行。

不得造成：

- Wallet Ledger 永久丢失；
- Final Balance 永久丢失；
- Poker Hand / Settlement 永久丢失。

## 16.5 Chaldea Database Failure

原则：

- Chaldea 正式业务写入停止；
- Poker 不得把 Redis 升级为临时最终账本；
- 进入安全 Degraded / Paused / Recovering；
- 恢复后以 PostgreSQL 正式事实继续。

## 16.6 Shared PostgreSQL Instance Failure

V1 因同一 PostgreSQL Instance 承载两个逻辑数据库：

```text
newapi
chaldea_platform
```

实例级故障会同时影响两套系统。

这是 V1 当前规模接受的单点风险。

## 16.7 Reverse Proxy Failure

Edge Proxy 为 V1 公网入口单点。

V1 接受该风险。

HA Edge / Multi-node Reverse Proxy 不在 V1 范围内。

---

# 17. Health / Readiness Baseline

所有应用服务区分：

```text
Liveness
Readiness
Dependency Status
```

## 17.1 Liveness

回答：

> 进程是否还活着？

不代表业务已经安全可接流量。

## 17.2 Readiness

回答：

> 当前实例是否可以安全承担其职责对应的新请求？

例如 Poker：

```text
process alive
+
PostgreSQL recovery not complete
=
live = true
ready = false
state = recovering
```

此时 Reverse Proxy / Service Discovery 不应把正常新牌桌流量恢复给该实例。

## 17.3 Dependency Status

各服务应能表达依赖项：

```text
healthy
degraded
unavailable
recovering
```

具体 Health Endpoint、Metrics、Alert Threshold 在 TD-12 冻结。

---

# 18. Future Split Strategy

外部 Contract 与物理部署解耦。

V1：

```text
api.chaldea.example.com
→ local Docker NewAPI

/ws/poker
→ local Docker Poker Service
```

未来：

```text
api.chaldea.example.com
→ private-network NewAPI host

/ws/poker
→ private-network Poker host
```

Browser / API Client 继续只认识：

```text
https://chaldea.example.com
https://api.chaldea.example.com
wss://chaldea.example.com/ws/poker
```

可以逐步独立迁移：

1. PostgreSQL；
2. Redis；
3. Poker Service；
4. NewAPI；
5. Static Media / Object Storage / CDN。

迁移不应要求重写 IA Route 或普通前端页面 Contract。

---

# 19. TD-01 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| TD-FRZ-001 | V1 采用模块化 Chaldea Platform Backend + 独立 Poker Realtime Service + NewAPI Core。 | FROZEN |
| TD-FRZ-002 | 采用方案 B：Chaldea Web Origin 与 NewAPI API Origin 分离。 | FROZEN |
| TD-FRZ-003 | Chaldea Web 采用统一 BFF；Browser 不直接访问 NewAPI 内部接口、DB、Redis 或 Poker Internal REST。 | FROZEN |
| TD-FRZ-004 | External Model / API Client 通过独立 API Origin 访问 NewAPI Core。 | FROZEN |
| TD-FRZ-005 | NewAPI Admin 与 Chaldea Operations 保持独立认证与权限域。 | FROZEN |
| TD-FRZ-006 | Runtime NewAPI 集成采用 Verified API / Narrow Internal Bridge 优先，DB 访问仅允许最小权限 Read Only；Runtime 禁止任意 NewAPI DB Write。 | FROZEN |
| TD-FRZ-007 | Poker 普通 REST 由 Platform BFF 进入；实时牌桌通信由 Edge 直接转发至 Poker WebSocket Service。 | FROZEN |
| TD-FRZ-008 | Poker 不直接继承 / 解析普通 Browser Session；采用 Platform Backend 签发的短生命周期 Poker Connect Credential，细节进入 TD-02 / TD-08。 | FROZEN |
| TD-FRZ-009 | PostgreSQL 为正式资产、Ledger 与 Poker Persistence Authority；Redis 永远不得成为这些事实的唯一来源。 | FROZEN |
| TD-FRZ-010 | Platform、Poker、NewAPI 使用独立 DB Identity 与最小权限；精确 Schema / Grant 在 TD-03 冻结。 | FROZEN |
| TD-FRZ-011 | `examples/deployment/external-newapi` 与 `examples/deployment/platform` 保持独立部署项目，通过受控 Docker Network 互联；仅 Edge Proxy 发布公网入口。 | FROZEN |
| TD-FRZ-012 | 公网 Hostname / Path Contract 与物理部署解耦；未来 PostgreSQL、Redis、Poker、NewAPI 或媒体迁移主机时不改变用户前端 Contract。 | FROZEN |
| TD-FRZ-013 | Service-to-Service 通信同时使用网络隔离与 Service Identity；不把 Docker 内网当作唯一认证机制。 | FROZEN |
| TD-FRZ-014 | V1 采用 Failure Isolation / Partial Degradation，不因 Poker 或 NewAPI 单模块故障无条件关闭整个 Chaldea Platform。 | FROZEN |
| TD-FRZ-015 | NewAPI 公网 Route 使用实际源码核验后的 Allowlist，不公开 NewAPI 原始普通用户 UI。 | FROZEN |

---

# 20. TD-01 Open Technical Dependencies

以下不是未完成的产品决定，而是明确推迟到后续技术批次的依赖：

## 20.1 NewAPI Source Verification

必须核验用户实际部署 NewAPI 版本后才能确定：

- Auth API；
- Password Login Identifier；
- OAuth Callback；
- User Schema；
- API Key Schema；
- Quota Update Capability；
- Logs Schema；
- Model API Public Path；
- Admin API；
- 是否存在适合 Chaldea 使用的 Internal / HTTP API。

在核验前不写死 Endpoint / Field / Table 名称。

## 20.2 Poker Connect Credential

进入：

- TD-02；
- TD-08。

待冻结：

- Credential format；
- signing；
- expiry；
- nonce；
- reconnect reuse；
- Take Over；
- revocation。

## 20.3 Database Permission Matrix

进入 TD-03。

待冻结：

- Schema；
- Role；
- GRANT；
- View；
- RLS（如需要）；
- migration credential；
- ownership。

## 20.4 Economy ↔ Poker Transaction Boundary

进入：

- TD-04；
- TD-08。

待冻结：

- Buy-in atomicity；
- Top-up；
- Cash Out；
- Safe Leave；
- Hand settlement interaction；
- ledger linkage；
- lock strategy；
- crash recovery；
- exactly-once semantics。

---

# 21. Change Log

## WORKING v0.1

### Added

- 建立 `Chaldea Platform Technical Design v0.5 WORKING` 累计技术设计文档；
- 完成 `TD-01 — System Topology & Service Boundaries`；
- 比较方案 A / B / C；
- 用户正式确认 **方案 B 全部通过**；
- 冻结 `TD-FRZ-001 ～ TD-FRZ-015`；
- 冻结 Web Origin / API Origin 分离；
- 冻结 Browser BFF Boundary；
- 冻结 NewAPI Adapter / Runtime Write Boundary；
- 冻结 PostgreSQL / Redis Authority Boundary；
- 冻结 Poker REST / WS Boundary；
- 冻结 Poker Connect Credential 技术方向；
- 冻结 Service-to-Service Authentication Baseline；
- 冻结 Docker Network / Public Port Boundary；
- 冻结 NewAPI Public Route Allowlist；
- 冻结 Failure Isolation；
- 冻结 Health / Readiness Baseline；
- 冻结 Future Split Strategy。

### Not Changed

本批没有改变：

- 需求基线；
- IA；
- Art Direction；
- Rewards TBD；
- 游戏数学；
- Poker 规则；
- NewAPI 实际源码事实。

---

# 22. 下一批 — TD-02

下一批正式进入：

> **TD-02 — Identity / Auth / Session / Return-to-Intent**

计划一次设计清楚：

1. Discord OAuth Login；
2. Discord First Registration Gate；
3. Required Server / Role Verification；
4. Password Login 与 NewAPI Bridge；
5. Password Login Identifier；
6. Account Identity ↔ Master Identity；
7. Browser Session Strategy；
8. Cookie / Token Boundary；
9. CSRF；
10. CORS；
11. Session Refresh / Expiry；
12. Fresh Authentication；
13. Unified Access Gate；
14. Account Status Gate；
15. Master Initialization；
16. Migration Notice Gate；
17. Return-to-Intent；
18. Logout；
19. Poker Connect Credential；
20. WebSocket Authentication；
21. Take Over 与 Session Relationship；
22. Rate Limit；
23. OAuth / Auth Failure Recovery；
24. Auth Audit / Secret Logging Boundary。

仍然不在 TD-02 猜测 NewAPI 实际 Endpoint / Table / Field。凡依赖当前 NewAPI 实现的部分，以 `SOURCE VERIFICATION REQUIRED` 标记，直到读取实际源码。



---

# 23. TD-02 — Identity / Auth / Session / Return-to-Intent

> 状态：`FROZEN`  
> 用户确认：`TD-02 整体采用方案 B，并同时批准冲突解析 R-A`

## 23.1 TD-02 总体目标

本批冻结：

- Authentication Identity / Account Identity / Master Identity 三层边界；
- Discord Login；
- Discord First Registration Gate；
- Existing Binding 与 Server / Role 校验顺序；
- OAuth State / Auth Flow；
- Password Login；
- Password Login Identifier 边界；
- Chaldea Browser Session；
- Session Cookie；
- Fresh Authentication；
- Unified Access Gate；
- Account Status Gate；
- Master Initialization Gate；
- Migration Notice Gate；
- Return-to-Intent；
- CSRF；
- CORS；
- Logout；
- Poker Connect Credential；
- Poker WebSocket Authentication；
- Multi-device / Take Over；
- Auth Rate Limit；
- Auth Audit / Secret Logging Boundary。

以下仍需用户当前实际 NewAPI 版本源码核验，TD-02 不猜测：

- NewAPI Password Authentication Endpoint；
- Password Login Identifier 实际字段；
- Password Set / Change / Reset Capability；
- NewAPI Account Status API / 数据来源；
- NewAPI Session 实际 Idle / Absolute Lifetime；
- OAuth Account Creation 的真实内部字段和流程。

---

# 24. Auth / Session 方案比较

## 24.1 方案 A — NewAPI Session Pass-through

做法：

```text
Browser
→ 直接持有 / 复用 NewAPI Login Session
→ Chaldea 根据 NewAPI Session 识别用户
```

优点：

- 实现层较少；
- Credential Authority 与 Browser Session 都由 NewAPI 处理。

问题：

- Chaldea Web 与 NewAPI Web Session 高度耦合；
- Chaldea Gate / Fresh Auth / Poker Ticket 均被 NewAPI Cookie 结构约束；
- Future Split 较困难；
- 不符合 TD-01 已冻结的 Chaldea BFF Boundary 方向。

结论：`V1 不采用。`

## 24.2 方案 B — Chaldea Opaque Server Session

做法：

```text
Discord / NewAPI
→ 验证 Authentication Identity
→ resolve stable newapi_user_id
→ Chaldea 创建自己的 opaque web session
→ Browser 只持有 HttpOnly Session Cookie
```

这不建立第二套用户账号，也不复制密码。

NewAPI 继续拥有：

- Password；
- Password Hash；
- Password Validation；
- Account Identity；
- Account Status；
- API Key；
- 原生账号能力。

Chaldea Session 只表示：

> 当前 Browser 已在 Chaldea 中成功证明自己对应哪个稳定 `newapi_user_id`。

结论：`FROZEN / SELECTED`

## 24.3 方案 C — Stateless Long-lived JWT

做法：

```text
Login
→ Browser receives long-lived JWT
→ each request self-authenticates
```

问题：

- Logout / Revocation 较复杂；
- Suspension / RBAC 变化传播更复杂；
- Fresh Auth / Take Over / Critical Operation 不利于集中控制；
- 容易诱导实现把 Token 存入 localStorage。

结论：`V1 不采用。`

---

# 25. Identity 三层分离

正式身份模型：

```text
Authentication Identity
Discord OAuth / Password Proof
        │
        ▼
Account Identity
stable newapi_user_id
Password Login Identifier
NewAPI Account Status
Discord Binding
        │
        ▼
Master Identity
Master Nickname
Display Avatar
```

业务归属使用：

```text
newapi_user_id
```

不得使用：

```text
Master Nickname
Discord Display Name
Short Account ID
```

作为：

- Password Login Identifier；
- Wallet 主键；
- Poker 主键；
- Ledger 主键；
- History 主键；
- Rankings 内部业务主键。

---

# 26. Chaldea Browser Session

## 26.1 Cookie

推荐：

```text
__Host-chaldea_session=<opaque random session id>

Secure
HttpOnly
SameSite=Lax
Path=/
No Domain
```

`No Domain` 的目标：

```text
chaldea.example.com
→ receives Chaldea Session

api.chaldea.example.com
→ does not receive Chaldea Session
```

Browser 不将 Auth Token 保存到：

- `localStorage`；
- `sessionStorage`；
- IndexedDB 作为长期认证凭证。

## 26.2 Server-side Session Store

Redis namespace：

```text
chaldea:session:<sid_hash>
```

Session 逻辑状态至少支持：

```text
session_id_hash
newapi_user_id

created_at
last_seen_at

login_method
authenticated_at

fresh_auth_at
fresh_auth_method

account_status_snapshot
account_status_checked_at

csrf_secret

session_version
revoked_at
```

具体字段进入 TD-03 Schema。

Redis Session 丢失：

```text
→ User re-authentication required
```

但不能造成：

- Account 丢失；
- Master Profile 丢失；
- Wallet / Ledger 丢失；
- Poker Hand / Settlement 丢失。

---

# 27. Session Lifetime / Refresh

V1 不增加独立 `Remember Me`。

Chaldea Session 支持：

- Idle expiration；
- Absolute expiration；
- Rolling touch / refresh capability；
- Server-side revoke；
- Session rotation。

但具体：

```text
idle_ttl
absolute_ttl
rolling_refresh_interval
```

不得在未核验当前 NewAPI 版本真实认证策略前猜测。

状态：

```text
SOURCE VERIFICATION REQUIRED
```

约束原则：

> Chaldea 不应凭空承诺比实际底层账号安全策略更长期、更宽松的 Authentication Lifetime。

---

# 28. Session Fixation / Rotation

成功 Authentication：

```text
anonymous / pre-auth context
→ credential verified
→ generate new authenticated SID
```

不得把 pre-auth SID 原地升级为 authenticated SID。

Fresh Authentication 成功后：

```text
current authenticated SID
→ rotate
→ preserve safe session context
→ update fresh_auth_at
```

旧 SID 被撤销。

---

# 29. OAuth Auth Flow

OAuth 不使用裸 Return URL 作为 `state`。

Redis：

```text
chaldea:auth-flow:<flow_id>
```

逻辑字段：

```text
flow_id
purpose
state_hash

session_id          optional
return_intent_id    optional

created_at
expires_at
consumed_at

provider_config_version
```

`purpose` 至少区分：

```text
LOGIN
REGISTRATION
FRESH_AUTH
PASSWORD_RESET
AVATAR_SYNC
```

不同 purpose 的 OAuth Callback 不允许交叉复用。

---

# 30. OAuth State

使用：

```text
CSPRNG random state
>= 128 bits entropy
single-use
recommended TTL = 10 minutes
```

Callback：

```text
receive state
→ hash
→ lookup auth_flow
→ verify purpose
→ verify not expired
→ verify not consumed
→ atomically consume
→ continue
```

OAuth State 不保存：

- External URL；
- raw Password；
- Browser Session Token；
- Discord Access Token；
- 任意可执行 Return Target。

---

# 31. Discord Credential Boundary

Discord OAuth Code / Access Token：

```text
Callback
→ Server-side exchange / verify
→ Resolve Discord Identity
→ Perform required provider checks
→ Discard transient provider credential
```

普通日志不记录：

- OAuth Code；
- OAuth Access Token；
- Refresh Token；
- Browser Cookie；
- Raw Session ID；
- CSRF Token。

V1 当前没有要求长期保存用户 Discord OAuth Token。

默认：

> Discord Provider Credential 不作为普通长期 Chaldea User Credential 持久化。

需要重新同步 Avatar 等 Provider Action 时，发起新的 purpose-bound OAuth Flow。

---

# 32. Discord Login

已有账号登录流程：

```text
/login
→ Discord OAuth purpose=LOGIN
→ State Validation
→ Resolve Discord Identity
→ Existing Binding Lookup
→ Bound newapi_user_id
→ Account Status
→ Create Chaldea Session
→ Unified Access Gate
```

已经注册的账号：

- 不重新验证首次注册 Server；
- 不重新验证首次注册 Role；
- 退出 Server / 失去 Role 不自动导致账号冻结。

---

# 33. TD-02-C01 — Registration Flow 上游冲突

## 33.1 冲突

冻结 Requirement：

> Discord Role 仅作为首次注册门槛。注册完成后，即使退出 Server 或失去 Role，仍可以继续登录。

同时冻结：

> 已绑定现有账号的 Discord 再次进入 Registration 时，应转为登录现有账号，不创建第二账号，不重复发 Initial Grant。

但 IA 一处 Registration 顺序写成：

```text
Discord Identity
→ Server Membership Validation
→ Required Role Validation
→ Existing Binding Check
```

若机械实现，会导致：

```text
already registered
+
later lost Role
+
opens /register
→ blocked before Existing Binding Check
```

与上游业务规则冲突。

## 33.2 用户确认解析 R-A

正式技术执行顺序：

```text
Discord Identity
        ↓
Existing Binding Pre-check
        │
        ├── BOUND
        │      ↓
        │   Existing Account Login
        │   do not re-check registration Role
        │
        └── UNBOUND
               ↓
        Server Membership Validation
               ↓
        Required Role Validation
               ↓
        New Registration
```

该解析不改变“哪些新用户有资格注册”。

它只保证：

> Server / Role Gate 仅作用于尚未注册的新身份。

状态：`FROZEN`

---

# 34. Discord Registration State Machine

推荐逻辑状态：

```text
STARTED
→ OAUTH_STARTED
→ CALLBACK_VERIFIED
→ IDENTITY_RESOLVED
→ BINDING_CHECKED

BOUND
→ EXISTING_ACCOUNT_LOGIN

UNBOUND
→ ELIGIBILITY_CHECKING
→ ELIGIBILITY_VERIFIED
→ ACCOUNT_CREATING
→ ACCOUNT_CREATED
→ INITIAL_GRANT_REQUESTED
→ SESSION_ISSUED
→ PROFILE_ENSURED
→ POST_AUTH_GATE
```

Server / Role 验证失败：

```text
→ stop before Account Creation
```

不得产生：

- NewAPI Account；
- Initial Grant；
- Provisional Master Profile。

Discord Provider 暂时不可用：

```text
→ PROVIDER_UNAVAILABLE
```

不得错误映射为：

```text
ROLE_MISSING
```

---

# 35. Registration Idempotency

Registration 必须同时使用：

- OAuth Flow ID；
- Registration Operation ID；
- Discord Identity Unique Constraint；
- stable NewAPI User relation；
- Provisional Master Profile Unique Constraint；
- Initial Grant stable Business ID。

并发：

```text
Tab A Callback ─┐
Tab B Callback ─┼─► uniqueness / lock / state lookup
Retry Callback ─┘
```

最终只允许：

```text
1 NewAPI Account
1 Discord Binding
1 Master Profile
1 Initial Grant
```

如果：

```text
Account created
→ Initial Grant pending / recovering
```

禁止：

```text
delete account
re-register
create second grant
```

允许继续 Master Initialization，并显示真实 Reward State。

---

# 36. Password Login Boundary

流程：

```text
Browser
→ TLS
→ Chaldea Platform Backend
→ NewAPI Adapter
→ Verified NewAPI Password Authentication Capability
```

Password 仅允许瞬时存在于：

- 当前 Request Memory；
- NewAPI Authentication Request Memory。

禁止：

- Chaldea Password DB；
- Password Hash Copy；
- Redis Password Cache；
- Audit Password；
- Debug Log Password；
- Trace Request Body 中记录 Password。

成功：

```text
NewAPI credential verification
→ stable newapi_user_id
→ Chaldea opaque session
```

失败：

```text
generic INVALID_CREDENTIALS
```

不得进行 Account Enumeration。

---

# 37. Password Login Identifier

冻结：

```text
Password Login Identifier
≠ Master Nickname
≠ Short Account ID
≠ fabricated Email
```

实际字段：

```text
SOURCE VERIFICATION REQUIRED
```

必须读取用户当前 NewAPI 版本确认：

- OAuth Account 创建后实际稳定 Identifier；
- Password Login 使用的真实字段；
- Identifier 是否允许修改；
- 是否有现成安全 API。

如果当前 NewAPI 无法提供稳定 Identifier：

```text
→ NewAPI Adapter
or
→ controlled Account Identifier Confirmation / Provisioning
```

不得由 Frontend 猜测或使用 Master Nickname 替代。

---

# 38. Forgot Password

## 38.1 已绑定 Discord

```text
Forgot Password
→ Discord OAuth purpose=PASSWORD_RESET
→ OAuth State Validation
→ Resolve Discord User ID
→ Must equal current bound Discord User ID
→ Mint one-time Reset Authorization
→ Set New Password through controlled NewAPI capability
→ Consume authorization
```

不得允许：

```text
Discord Account A
→ reset Account B
```

## 38.2 Legacy Account 无 Discord

继续使用：

```text
Account Support
```

V1 不新增：

- Email Recovery；
- Phone Recovery；
- TOTP；
- Passkey；
- Backup Codes。

---

# 39. Fresh Authentication

全平台统一：

```text
FRESH_AUTH_WINDOW = 10 minutes
```

至少适用于：

- Set Password；
- Reset Password；
- Critical Operations；
- Access Control；
- Asset Adjustment；
- Discord Rebind Final Execution；
- Poker Emergency Pause；
- 其他明确标记为需要 Fresh Auth 的 Level 3 操作。

Session 保存：

```text
fresh_auth_at
fresh_auth_method
```

判定：

```text
server_now - fresh_auth_at <= 10 minutes
```

Browser 不允许自行提交 `fresh=true` 作为证明。

普通登录成功视为一次 Fresh Authentication。

超过窗口：

```text
Sensitive Action
→ FRESH_AUTH_REQUIRED
→ re-authenticate
→ rotate session
→ return to safe confirmation context
```

不会自动执行敏感操作。

---

# 40. Fresh Auth Methods

已设置 Password：

```text
Current Password Re-verification
```

无 Password、已绑定 Discord：

```text
Discord OAuth purpose=FRESH_AUTH
```

Password Reset：

```text
Discord Re-auth
→ verify same binding
```

长期有效 Session 本身不自动等于 Fresh Session。

---

# 41. Unified Access Gate

正式 Gate：

```text
Requested Route
        ↓
Route Classification
        ↓
Anonymous Entry Popup Check
        ↓
Authentication
        ↓
Account Status Gate
        ↓
Master Initialization
        ↓
Migration Notice
        ↓
Role / Scope
        ↓
Resource Availability
        ↓
Return-to-Intent
        ↓
Target / Safe Parent
        ↓
Deferred Post-login Popup
```

优先级：

```text
Account Status
> Master Initialization
> Migration Notice
> Role / Scope
> Resource Availability
> Return-to-Intent
```

---

# 42. Route Classification

服务端至少支持：

```text
PUBLIC
AUTH_OPTIONAL
PROTECTED
ADMIN
IMMERSIVE
```

示例：

```text
/
→ PUBLIC / AUTH_OPTIONAL

/models
/rankings
→ AUTH_OPTIONAL

/dashboard
/wallet
/games/:slug
/poker
→ PROTECTED

/ops/*
→ ADMIN

/poker/table/:id
→ IMMERSIVE + PROTECTED
```

Frontend Route Guard 只负责 UX。

Backend API 必须独立进行 Auth / Authorization。

---

# 43. Account Status Gate

NewAPI Account Status 为 Authority。

逻辑：

```text
ACTIVE
→ continue

DISABLED / SUSPENDED
→ Restricted Account
```

Restricted Session 可允许：

- Restricted Account 页面；
- Account Support 入口；
- Logout。

禁止：

- Wallet Write；
- Rewards Claim；
- Game Round；
- Poker Join；
- API Management Write；
- Chaldea Operations。

受限账号不进入普通 Dashboard 或 Master Initialization。

---

# 44. NewAPI Dependency Degradation

## 44.1 Login 阶段

NewAPI 不可用：

```text
Password Login
→ AUTH_DEPENDENCY_UNAVAILABLE
```

不得误报：

```text
INVALID_PASSWORD
```

## 44.2 已存在有效 Chaldea Session

NewAPI 短暂不可用时：

- 不立即销毁整个 Chaldea Session；
- 可继续读取安全的 Chaldea-owned 数据；
- 依赖 NewAPI 的能力标记 `DEGRADED / UNAVAILABLE`；
- 不伪造 Account / Password / API Key / Model Runtime 状态。

---

# 45. Master Initialization Gate

认证后：

```text
stable newapi_user_id
→ ensure Provisional Master Profile
```

幂等保证：

```text
status = INCOMPLETE
```

已存在时：

```text
→ resume same profile
```

Gate：

```text
INCOMPLETE
→ /onboarding/master

COMPLETE
→ continue
```

保存失败：

```text
→ remain INCOMPLETE
```

不得：

- 创建第二 Profile；
- 创建第二 Initial Grant；
- 提前暴露半完成 Master Identity。

Initial Grant State 独立存在，可以：

```text
PROCESSING
CONFIRMED
RECOVERING
FAILED
```

Completion Summary 必须读取真实状态。

---

# 46. Migration Notice Gate

正式 Migration Acknowledgement 至少表达：

```text
required_migration_version
acknowledged_migration_version
acknowledged_at
```

如果：

```text
required > acknowledged
```

进入：

```text
Migration Notice
```

提交：

```text
Acknowledge(version)
```

必须幂等。

Migration Notice：

> 只确认已经完成的 Migration / Cutover 事实，不在用户点击确认时执行资产迁移。

---

# 47. Return-to-Intent 方案

## 47.1 方案 A — Raw Return URL

```text
?return=https://...
```

Open Redirect 风险高。

结论：`不采用。`

## 47.2 方案 B — Entire URL in OAuth State

可以签名，但会让 OAuth State 同时承担多个职责。

结论：`不采用。`

## 47.3 方案 C — Server-side Opaque Intent ID

选用。

Redis：

```text
chaldea:return-intent:<intent_id>
```

逻辑信息：

```text
intent_id

relative_path
safe_query

route_class
created_at
expires_at
consumed_at

source
```

推荐：

```text
TTL = 30 minutes
single-use
```

30 分钟属于 TD-02 技术安全参数，未来如调整需保持短期、single-use 与安全重新验证语义。

Intent 只存安全站内 Navigation State。

禁止保存：

- External URL；
- `javascript:`；
- `data:`；
- Password；
- POST Body；
- Exchange Submission；
- Bet Submission；
- Admin Write Payload。

---

# 48. Return-to-Intent Recovery

流程：

```text
Authentication
→ Account Status
→ Master Initialization
→ Migration Notice
→ Role / Scope
→ Resource Availability
→ load intent
→ revalidate current target
→ atomically consume
→ navigate
```

恢复时重新检查：

- 当前 Permission；
- Resource Existence；
- Publication；
- Maintenance；
- Current Account Status。

如果目标已经：

- 下线；
- 删除；
- 维护；
- 无权限；

则进入安全父页面，并说明原因。

---

# 49. Navigation Resume ≠ Action Replay

Return-to-Intent 可以恢复：

- Route；
- Safe Query；
- Filter；
- Sort；
- Pagination；
- Detail Location。

绝不自动重放：

- Wallet Exchange；
- Reward Claim；
- Direct Play Bet；
- Poker Buy-in；
- Poker Cash Out；
- API Key Create / Delete；
- Password Save；
- Profile Save；
- Admin Write。

---

# 50. CSRF

Cookie-authenticated BFF 写请求采用：

```text
SameSite=Lax Session Cookie
+
Synchronizer CSRF Token
+
Origin / Fetch Metadata Validation
```

Session 创建：

```text
csrf_secret
```

Frontend 通过安全 Bootstrap 获得当前 CSRF Token，并保存在运行时内存。

写请求：

```text
POST
PUT
PATCH
DELETE
```

必须带：

```text
X-CSRF-Token
```

并通过：

- expected Origin；
- Fetch Metadata / same-origin policy。

失败：

```text
403 CSRF_FAILED
```

不执行业务写操作。

---

# 51. CORS

Chaldea Web BFF：

```text
Browser
→ same origin
→ chaldea.example.com/api/*
```

因此 credentialed CORS：

```text
deny by default
```

禁止：

```text
Access-Control-Allow-Origin: *
+
Access-Control-Allow-Credentials: true
```

NewAPI External API Origin：

```text
api.chaldea.example.com
```

不接收 Chaldea Browser Session Cookie。

---

# 52. Poker WebSocket Authentication

流程：

```text
Authenticated Browser
→ Platform Backend
→ mint Poker Connect Ticket
→ Browser
→ wss://chaldea.example.com/ws/poker
→ Poker validates ticket
```

Connect Ticket 推荐：

```text
TTL = 60 seconds
single-use
```

语义至少绑定：

```text
ticket_id / jti
newapi_user_id
chaldea_session_id
purpose = poker_connect

issued_at
expires_at

optional table/session target
control intent
```

Ticket 不是长期 Session。

---

# 53. Poker Ticket Trust Model

采用：

```text
Platform Backend
→ signs

Poker Service
→ verifies
```

目标：

> Poker Service 不获得签发任意 Chaldea 用户身份的同等权限。

Ticket 必须：

```text
short-lived
purpose-bound
session-bound
user-bound
single-use
signed
```

具体算法与 Key Rotation 留 Implementation Spec / TD-12。

WebSocket 同时校验：

```text
Origin == https://chaldea.example.com
```

普通 Browser Session Cookie 不作为 Poker WebSocket 的直接 Auth Credential。

---

# 54. Multi-device / Poker Control

普通 Chaldea Web：

```text
User
├── Laptop Session
├── Phone Session
└── Tablet Session
```

允许。

V1 不承诺完整设备会话管理中心。

只保证：

```text
Logout Current Session
```

Poker：

```text
same user
→ one Active Control Connection
```

普通多 Session 与 Poker 单控制连接是不同层级规则。

---

# 55. Poker Take Over

采用：

```text
control_epoch
```

例如：

```text
epoch = 17
Device A = controller
```

Device B：

```text
CONTROL_ALREADY_EXISTS
→ explicit Take Over
→ new authenticated takeover authorization
→ atomic control_epoch = 18
→ Device B = controller
→ Device A = read-only
```

旧设备后续 Action：

```text
epoch 17
→ reject
```

具体 Action Message / Ack / Sequence 进入 TD-08。

---

# 56. Logout

普通：

```text
Logout Current Session
```

执行：

- revoke current Chaldea session；
- invalidate current CSRF context；
- invalidate unused Poker Connect Tickets bound to Session；
- terminate / revoke Poker control connection owned by that Session；
- expire Session Cookie。

Logout 不：

- Cash Out；
- Safe Leave；
- Cancel accepted Round；
- Cancel Wallet Transfer；
- Cancel Reward Claim；
- Cancel already accepted API server operation。

---

# 57. Active Poker Logout

存在 Active Poker Session：

```text
Logout
→ warning
→ Return to Poker Table
or
→ Logout Anyway
```

`Logout Anyway`：

```text
Web Session revoked
→ Poker client disconnect
→ Poker seat / session continues
→ Action Timer continues
→ Auto Check / Fold follows Poker rules
→ later Login + Reconnect
```

Logout 本身不得伪装成 Safe Leave / Cash Out。

---

# 58. Session Expiry

Protected Route：

```text
401 SESSION_EXPIRED
→ create safe Return-to-Intent
→ Login
→ Unified Gate
→ return
```

可恢复：

- Filter；
- Sort；
- Safe Draft；
- Page / Detail Location。

不恢复：

- Password；
- OAuth Token；
- API Secret；
- Typed Confirmation Secret；
- Pending Action Submission。

---

# 59. Auth Rate Limit

TD-02 冻结以下初始安全基线：

| Flow | Initial Baseline |
|---|---:|
| Password Login / IP | 10 attempts / 5 min |
| Password Login / Identifier | 5 failed attempts / 10 min |
| OAuth Start / IP | 20 / 10 min |
| Invalid OAuth Callback / IP | 20 / 10 min |
| Password Reset / User | 5 / 15 min |
| Fresh Auth / Session | 10 / 10 min |
| Poker Ticket Mint / Session | 30 / min |

触发：

```text
429
retry_after
```

不使用永久账号锁定。

普通 UI 可以显示安全重试时间，但不公开完整内部风控算法。

Rate Limit Key 不直接保存 Password Login Identifier 原文。

推荐：

```text
rate:login:identifier:<HMAC(normalized_identifier)>
```

---

# 60. Auth Secret / XSS Boundary

TD-02 最低安全边界：

- No auth token in localStorage；
- No Password Persistence；
- No OAuth Token in normal browser app state after callback；
- No raw API Key Secret in Auth / Session log；
- No raw Session ID in Analytics；
- No unsanitized Return URL；
- No Password / API Secret in Trace body。

完整 CSP / Trusted Types / Security Headers 进入 TD-12。

---

# 61. Auth Audit

至少记录：

```text
LOGIN_SUCCESS
LOGIN_FAILURE
LOGOUT

OAUTH_STARTED
OAUTH_DENIED

REGISTRATION_ELIGIBILITY_FAILED
REGISTRATION_SUCCESS

FRESH_AUTH_SUCCESS
FRESH_AUTH_FAILED

PASSWORD_RESET_AUTHORIZED

SESSION_EXPIRED
ACCOUNT_RESTRICTED

POKER_TICKET_ISSUED
POKER_TAKEOVER
```

禁止记录：

```text
Password
Password Hash
OAuth Code
OAuth Access Token
Raw Session Cookie
CSRF Secret
API Key Secret
```

---

# 62. TD-02 Failure Mapping

## Discord Provider unavailable

```text
PROVIDER_UNAVAILABLE
```

不得映射为：

```text
ROLE_MISSING
```

## NewAPI unavailable during Password Login

```text
AUTH_DEPENDENCY_UNAVAILABLE
```

不得映射为：

```text
INVALID_CREDENTIALS
```

## Discord Binding Conflict

```text
ACCOUNT_SUPPORT_REQUIRED
```

禁止：

- 自动覆盖；
- 自动 Rebind；
- 自动 Merge；
- 自动迁移资产。

## Registration Network Uncertain

重新读取：

```text
Auth Flow State
Registration Operation
Current Session
```

根据 durable fact Resume。

不得直接创建第二账号。

---

# 63. TD-02 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| TD-FRZ-016 | Authentication Identity、Account Identity、Master Identity 三层严格分离；业务归属使用稳定 `newapi_user_id`。 | FROZEN |
| TD-FRZ-017 | Chaldea Web Session 采用方案 B：Server-side Opaque Session；不直接把 NewAPI Session 当 Browser Product Session，也不采用长期 Browser JWT。 | FROZEN |
| TD-FRZ-018 | Session Cookie 使用 Host-only、Secure、HttpOnly、SameSite=Lax；不发送给 NewAPI API Origin；Auth Token 不进入 localStorage / sessionStorage。 | FROZEN |
| TD-FRZ-019 | Chaldea Session Ephemeral State 保存在 Redis；Redis Session 丢失允许重新登录，但不得影响正式账号、资产或 Poker 事实。 | FROZEN |
| TD-FRZ-020 | V1 不增加 Remember Me；Session Idle / Absolute TTL 根据当前 NewAPI 实际认证能力源码核验后确定，不得猜测。 | FROZEN |
| TD-FRZ-021 | OAuth 使用 purpose-bound、single-use、server-side Auth Flow；OAuth State 使用 CSPRNG，推荐 10 分钟 TTL，不承载原始 Return URL。 | FROZEN |
| TD-FRZ-022 | Discord OAuth Provider Credential 默认只短期使用，不作为普通长期用户凭证持久化。 | FROZEN |
| TD-FRZ-023 | TD-02-C01 采用冲突解析 R-A：Discord Identity 后先 Existing Binding Pre-check；已绑定账号直接登录，只有未绑定身份才执行 Server / Role 首次注册资格验证。 | FROZEN |
| TD-FRZ-024 | Registration 使用可恢复、幂等状态机；重复 Tab / Callback / Retry / Restart 只能产生一个账号、Binding、Profile 与 Initial Grant。 | FROZEN |
| TD-FRZ-025 | Password 只通过受控 NewAPI Authentication Capability 验证；Chaldea 永不保存第二份 Password / Hash，也不直接 SQL 修改密码。 | FROZEN |
| TD-FRZ-026 | Password Login Identifier 必须来自当前 NewAPI 实际稳定账号能力；Master Nickname / Short Account ID 不得替代；具体字段 `SOURCE VERIFICATION REQUIRED`。 | FROZEN |
| TD-FRZ-027 | Fresh Authentication 全平台统一采用 10 分钟 Window；Fresh Auth Evidence 只由服务端 Session 保存。 | FROZEN |
| TD-FRZ-028 | Unified Access Gate 顺序固定为 Account Status → Master Initialization → Migration Notice → Role / Scope → Resource Availability → Return-to-Intent。 | FROZEN |
| TD-FRZ-029 | Disabled / Suspended Account 使用 Restricted Gate，不进入普通 Dashboard 或业务写操作。 | FROZEN |
| TD-FRZ-030 | Return-to-Intent 采用 Server-side Opaque Intent Record，推荐 TTL 30 分钟、Single-use，仅允许安全站内 Navigation State。 | FROZEN |
| TD-FRZ-031 | Return-to-Intent 永不自动重放有副作用操作；恢复时重新验证 Permission、Resource、Publication 与 Maintenance State。 | FROZEN |
| TD-FRZ-032 | Cookie-authenticated BFF 写请求采用 SameSite + Synchronizer CSRF Token + Origin / Fetch Metadata 联合保护。 | FROZEN |
| TD-FRZ-033 | Chaldea Web BFF 使用 Same-origin；Credentialed CORS 默认拒绝；NewAPI API Origin 不接收 Chaldea Browser Session。 | FROZEN |
| TD-FRZ-034 | Poker WS 使用 Platform Backend 签发的短期、purpose-bound、session-bound、user-bound、single-use Connect Ticket；推荐 TTL 60 秒。 | FROZEN |
| TD-FRZ-035 | Poker Connect Ticket 使用 Platform 签名 / Poker 验证的单向信任模型，并校验 WebSocket Origin。 | FROZEN |
| TD-FRZ-036 | 普通 Chaldea 允许多个 Browser Session；Poker 同一用户只允许一个 Active Control Connection。 | FROZEN |
| TD-FRZ-037 | Poker Take Over 必须显式确认并原子推进 `control_epoch`；旧设备转为 read-only，旧 epoch Action 被拒绝。 | FROZEN |
| TD-FRZ-038 | Logout V1 只保证 Current Session；Logout 不 Cash Out、不 Safe Leave、不取消已接受服务端业务；Active Poker Logout 必须先警告。 | FROZEN |
| TD-FRZ-039 | Auth / Registration / Password Reset / Fresh Auth / Poker Ticket 采用分层 Rate Limit；不使用永久账号锁定。 | FROZEN |
| TD-FRZ-040 | Auth Audit 永不保存 Password、Hash、OAuth Secret、Raw Session、CSRF Secret 或 API Secret。 | FROZEN |

---

# 64. TD-02 Source Verification Dependencies

以下正式保持：

```text
SOURCE VERIFICATION REQUIRED
```

## 64.1 NewAPI Password Authentication

需要核验：

- 当前 Login API；
- Identifier Field；
- Password Validation behavior；
- Account Status behavior；
- Error mapping；
- Rate Limit interaction。

## 64.2 Password Set / Change / Reset

需要核验：

- NewAPI 是否已有安全 API；
- 是否允许 OAuth account 设置 Password；
- Change Password 如何验证 Current Password；
- Reset Password 是否有可安全复用的 internal capability。

如没有安全接口：

```text
→ design narrow NewAPI Adapter / Bridge
```

不得直接 SQL 修改 Password Hash。

## 64.3 Account Status

需要确认：

- Disabled / Suspended 的实际字段 / API；
- 状态刷新方式；
- Session 中允许 Cache 多久；
- NewAPI 不可用时如何安全降级。

## 64.4 Session Policy

需要核验真实 NewAPI：

- Idle Lifetime；
- Absolute Lifetime；
- Refresh behavior；
- Logout capability；
- 是否支持 revoke other sessions。

V1 当前只承诺：

```text
Logout Current Session
```

---

# 65. Change Log — WORKING v0.2

## Added

- 用户正式确认 `TD-02 整体采用方案 B`；
- 用户正式确认 `TD-02-C01 冲突解析 R-A`；
- 冻结 `TD-FRZ-016 ～ TD-FRZ-040`；
- 冻结 Identity 三层模型；
- 冻结 Chaldea Opaque Server Session；
- 冻结 Host-only HttpOnly Session Cookie；
- 冻结 OAuth Auth Flow / State Boundary；
- 冻结 Existing Binding Pre-check；
- 冻结 Discord Registration Idempotency；
- 冻结 Password Login Adapter Boundary；
- 冻结 Password Login Identifier `SOURCE VERIFICATION REQUIRED`；
- 冻结全平台 Fresh Auth Window = 10 minutes；
- 冻结 Unified Access Gate；
- 冻结 Restricted Account Gate；
- 冻结 Return-to-Intent Opaque Record；
- 冻结 CSRF / CORS Baseline；
- 冻结 Poker Connect Ticket；
- 冻结 Poker WebSocket Origin / Auth Boundary；
- 冻结 Multi-device 与 Poker Single Control Connection；
- 冻结 Take Over `control_epoch`；
- 冻结 Logout / Active Poker Logout；
- 冻结 Auth Rate Limit；
- 冻结 Auth Audit Secret Boundary。

## Supersession

本批没有直接将既有 TD-FRZ 标记为 `SUPERSEDED`。

TD-02-C01 属于：

> 对冻结 Requirement 与 IA 流程顺序冲突进行显式 Technical Resolution。

该解析不修改首次注册资格本身，不改变已确认的 Server / Role Requirement，只改变实现时对 **Existing Binding Check 与 First-registration Eligibility Check 的执行顺序**。

## Not Changed

本批没有改变：

- Discord Role 仅首次注册验证；
- 新用户必须 Discord 注册；
- Password Login 只用于已有账号；
- Master Nickname 不作为 Login Identifier；
- Master Initialization / Migration Notice UX；
- Wallet / Rewards / Poker 业务规则；
- NewAPI 实际代码事实；
- IA Route；
- Art Direction。

---

# 66. 下一批 — TD-03

下一批正式进入：

> **TD-03 — Data Model / DB Ownership / Migration Framework**

计划冻结：

1. `newapi` / `chaldea_platform` Database Ownership；
2. PostgreSQL Role / Permission Matrix；
3. Schema 分区；
4. Global ID / Business ID / Public ID；
5. `newapi_user_id` Reference Strategy；
6. Master Profile 数据模型；
7. Discord Binding / Auth Support Metadata；
8. API Key Usage Purpose Metadata；
9. Model Catalog Metadata；
10. Wallet / Ledger 表边界（只定义 Ownership，不提前完整做 TD-04 State Machine）；
11. Rewards / Games / Poker / Rankings / Announcements / Audit 顶层 Schema Ownership；
12. Migration Tooling；
13. Schema Migration Versioning；
14. NewAPI Read Views / Adapter Boundary；
15. Backup / Restore；
16. Cutover Batch；
17. Existing User Reset + Migration Initial Grant；
18. Retry / Resume；
19. Partial Failure；
20. Rollback Boundary；
21. Migration Notice Version；
22. Post-cutover Verification；
23. Data Retention / Delete Boundary；
24. PostgreSQL / Redis authority mapping。

特别注意：

- TD-03 不直接设计完整 Economy Saga；
- TD-03 不直接设计完整 Poker Realtime Protocol；
- NewAPI 真实 Table / Field / API 仍需以用户当前部署源码核验为准；
- 迁移不得写成无审计的一次性 SQL。



---

# 67. TD-03 — Data Model / DB Ownership / Migration Framework

> 状态：`FROZEN`  
> 用户确认：`采用上述方案 B + Hybrid ID 策略 + Durable Cutover State Machine`

## 67.1 TD-03 总体结论

本批正式冻结：

- 同一 PostgreSQL Instance + `newapi` / `chaldea_platform` 两个 Database；
- `chaldea_platform` 内部按业务域使用 PostgreSQL Schema 分区；
- Hybrid ID Strategy；
- stable `newapi_user_id` 作为用户真实业务归属；
- Chaldea 本地 Account Reference Anchor；
- NewAPI / Chaldea 跨库边界；
- Database Role / Ownership / DDL Boundary；
- Master Profile / Identity Snapshot 顶层模型；
- API Key Usage Purpose 顶层数据边界；
- Model Catalog 顶层数据边界；
- Wallet / Rewards / Games / Poker / Content / Ranking / Ops / Audit / Migration Schema Ownership；
- Schema Migration Framework；
- Product Cutover Migration Framework；
- Durable Migration Batch State Machine；
- Durable Per-user Migration State；
- Backup / Verification / Resume / Rollback Boundary；
- Migration Initial Grant 与 Existing API Key Migration；
- Migration Notice Version / Acknowledgement；
- NewAPI Source Verification Dependency。

本批不提前完成：

- Economy / Ledger 完整 State Machine；
- Transfer Saga；
- Compensation Worker；
- Reward Claim State Machine；
- Game Round / Provably Fair 完整设计；
- Poker Realtime Protocol / Settlement State Machine。

这些分别进入 TD-04 / TD-05 / TD-06 / TD-08。

---

# 68. PostgreSQL Database Topology

继续采用：

```text
PostgreSQL Instance
│
├── newapi
│   └── NewAPI-owned data
│
└── chaldea_platform
    └── Chaldea-owned data
```

V1 不进一步拆分多个 Chaldea Database。

## 68.1 `chaldea_platform` 内部 Schema 分区

采用方案 B：

```text
chaldea_platform
│
├── platform_meta
├── identity
├── api
├── catalog
├── economy
├── rewards
├── games
├── poker
├── content
├── ranking
├── ops
├── audit
└── migration
```

目的：

- 强化 Ownership；
- 强化最小权限；
- Poker 可以获得更窄的 DB Role；
- 减少所有表堆入 `public` 后的权限和维护混乱；
- 不增加额外数据库、跨库事务或额外部署复杂度。

PostgreSQL Schema 分区不代表微服务拆分。

---

# 69. Cross-database Boundary

`newapi` 与 `chaldea_platform` 是两个独立 Database。

正式禁止使用以下机制把两边伪装成一个普通事务数据库：

```text
cross-database FK
dblink-based business write
FDW-based business write
cross-database trigger
```

Chaldea 保存稳定 NewAPI External ID。

引用合法性通过：

```text
stable external id
+
NewAPI Adapter / verified source
+
application validation
+
idempotency
+
reconciliation
```

保证。

跨库资产事务不能依赖普通 PostgreSQL 单事务覆盖两个 Database。

---

# 70. Database Role / Ownership

## 70.1 `newapi_app`

NewAPI Runtime Role。

只负责 NewAPI 自身正常数据库能力。

## 70.2 `chaldea_owner`

```text
NOLOGIN
```

拥有 Chaldea：

- Schemas；
- Tables；
- Sequences；
- Views；
- Functions。

普通 Runtime 不使用 Owner Credential。

## 70.3 `chaldea_migrator`

专门用于 Schema Migration / Deploy。

允许：

- CREATE / ALTER；
- controlled data migration；
- schema evolution。

普通 Runtime 不拥有 DDL。

## 70.4 `chaldea_app`

Platform Backend Runtime Role。

允许访问 Platform-owned Schemas。

不允许：

- Runtime DDL；
- arbitrary NewAPI database write；
- arbitrary Poker authoritative mutation bypassing Poker Service。

## 70.5 `chaldea_poker`

Poker Service Runtime Role。

直接 RW：

```text
poker
```

不直接获得：

```text
UPDATE economy.wallets
INSERT arbitrary economy.wallet_ledger
```

Poker 与 Economy 通过正式业务边界交互。

## 70.6 `chaldea_newapi_ro`

仅在实际源码核验后确有需要时启用。

目标：

```text
SELECT only
```

且仅授予经确认需要读取的 Table / View / Column。

不使用：

```text
GRANT SELECT ON ALL TABLES
```

作为默认方案。

## 70.7 `chaldea_cutover`

Product Cutover 专用特殊身份。

仅在 Maintenance Window 激活。

可以拥有：

- 必要的 Chaldea Migration Write；
- 经源码核验的 NewAPI Quota Reset Capability。

Cutover 完成后撤销特殊 Write Capability。

---

# 71. Poker Database Boundary

正常运行：

```text
Platform Backend
        │
        └── Poker Internal API
                │
                └── poker schema
```

以及：

```text
Poker Service
        │
        └── Economy Internal API
                │
                └── economy schema
```

避免：

```text
Platform Backend
→ arbitrary poker authoritative update

Poker
→ arbitrary economy authoritative update
```

Poker Session 首次成功 Buy-in / 入座时持久化 Master Display Snapshot。

当前 Poker Session 后续恢复依赖已冻结 Snapshot，不因用户中途修改 Master Profile 而改变牌桌身份。

---

# 72. Hybrid ID Strategy

采用 Hybrid ID。

## 72.1 Chaldea Entity ID

Chaldea 新生成实体默认使用：

```text
UUIDv7
```

PostgreSQL 类型：

```text
uuid
```

典型对象：

```text
transfer_id
round_id
poker_session_id
hand_id
announcement_id
operation_id
migration_batch_id
snapshot_id
```

## 72.2 保留 Domain / External ID

以下不强制改成 UUID：

```text
newapi_user_id
newapi_key_id
real model_id
game_slug
```

展示名称变化不得改变稳定 Domain Identity。

## 72.3 NewAPI ID Type

`newapi_user_id` 的真实 DB 类型继续：

```text
SOURCE VERIFICATION REQUIRED
```

不得提前猜测为 `BIGINT`、`TEXT` 或其他具体类型。

---

# 73. Local Account Reference Anchor

在：

```text
identity.account_refs
```

建立 Chaldea 内部 Reference Anchor。

逻辑职责：

```text
newapi_user_id
created_at
first_seen_at
migration_batch_id nullable
```

它不是第二套账号。

不保存：

```text
password
password_hash
authoritative username
authoritative account status
```

Chaldea 自有表可以：

```text
FOREIGN KEY newapi_user_id
→ identity.account_refs.newapi_user_id
```

从而避免跨 Database Foreign Key。

---

# 74. Entity ID vs Business ID

Entity ID：

```text
round_id
transfer_id
claim_id
session_id
```

代表实体。

`biz_id`：

> 表示一次业务影响的稳定幂等身份。

两者不得混用。

建议基础语义：

```text
biz_type
biz_id
```

并在业务域建立：

```text
UNIQUE(biz_type, biz_id)
```

或更严格的等价唯一约束。

例如：

```text
initial_grant:migration:{migration_batch_id}:{newapi_user_id}
```

属于 Business Identity。

完整资产 Idempotency 设计进入 TD-04。

---

# 75. Time Storage

正式业务时间：

```text
TIMESTAMPTZ
UTC storage
```

业务时区：

```text
Asia/Shanghai
```

用于：

- Daily Period；
- Ranking Period；
- Announcement Scheduling；
- UI Display。

不得使用 Naive Local Timestamp 作为跨服务正式事实时间。

---

# 76. Typed Columns vs JSONB

权威业务状态优先使用：

```text
typed relational columns
```

JSONB 只适合：

- metadata；
- immutable snapshot extension；
- audit context；
- provider snapshot；
- non-critical extension；
- diagnostic context。

禁止使用单个 JSONB 替代：

- Wallet Balance；
- Ledger；
- Poker Hand Authority；
- Settlement；
- Claim Authority；
- Critical State Machine。

JSONB Extension 如可能演进，应保存：

```text
schema_version
```

---

# 77. Schema Ownership Map

## `platform_meta`

负责：

```text
schema_migrations
platform_versions
runtime configuration references
service-state metadata
```

## `identity`

负责：

```text
account_refs

master_profiles
master_profile_name_history
reserved_master_names

master_profile_avatar_snapshots
identity_display_snapshots

registration_operations
registration_idempotency_records
```

以及经源码核验后确有需要的 Registration Guard / Support Metadata。

## `api`

负责：

```text
api_key_purpose_metadata
api_key_purpose_history
request_attributions
```

`request_attributions` 是否由 NewAPI Log 派生或 Minimal Hook 生成取决于源码核验。

## `catalog`

负责：

```text
model_catalog_metadata
model_catalog_publication
model_sync_snapshots
model_availability_mappings
historical_model_identity
```

## `economy`

顶层 Ownership：

```text
wallets
wallet_ledger
transfers
reconciliation records
asset snapshots
adjustments
issuance / burn records
```

完整字段和状态机进入 TD-04。

## `rewards`

负责：

```text
reward_configs
reward_config_versions
reward_claims
daily_checkins
reward_issuance_records
```

完整 Claim State Machine 进入 TD-05。

## `games`

负责：

```text
game_registry
game_metadata
game_categories
game_tags

game_config_versions

game_rounds
game_bets
game_results

fairness commitments / seed records
```

完整 Round / Fairness Architecture 进入 TD-06 / TD-07。

## `poker`

负责：

```text
poker_tables
poker_seats
poker_sessions
poker_hands
poker_actions
poker_settlements
poker_recovery_state
poker_chat
```

PostgreSQL 为 Poker Durable Authority。

## `content`

负责：

```text
announcements
announcement_revisions
notification_revisions
announcement_placements
announcement_read_states

acknowledgement_entries
announcement_media_assets
```

Published Revision 不做无审计硬删除。

## `ranking`

负责：

```text
ranking_periods
ranking_snapshots
ranking_entries

rp_usage_aggregates

ranking_reaggregation_jobs
ranking_exclusions
```

管理员不直接编辑用户 Score。

## `ops`

负责：

```text
roles
scopes
admin_assignments

attention_items
incidents
support_cases
binding_cases

maintenance_windows
background_jobs
admin_operations
```

## `audit`

建立统一：

```text
audit_events
```

所有正式 Admin Operation 至少可以关联：

```text
audit_event_id
operation_id
actor
target
reason
before
after
result
timestamp
related_biz_id
```

Audit append-only。

## `migration`

负责：

```text
cutover_batches
cutover_user_states

pre_cutover_quota_snapshots
balance_reset_audit
grant_results
validation_reports

migration_notice_versions
migration_notice_acknowledgements
```

---

# 78. Master Profile Data Model Boundary

`identity.master_profiles` 至少支持：

```text
newapi_user_id
status

display_name
normalized_name

current_avatar_snapshot_id

nickname_changed_at
rename_required

profile_version

created_at
updated_at
completed_at
```

数据库必须对：

```text
normalized_name
```

建立 Unique Constraint。

昵称唯一性不能只靠应用层：

```text
SELECT
→ then INSERT
```

---

# 79. Name History / Reserved Name

`master_profile_name_history`：

Append-only 保存：

```text
user
old_display_name
new_display_name
old_normalized
new_normalized
change_type
reason
actor
changed_at
```

`reserved_master_names`：

使用标准化形式存储。

Name Release 必须 Audit。

---

# 80. Avatar / Identity Snapshot

V1 Avatar Source：

```text
SYSTEM
DISCORD_SNAPSHOT
```

`master_profile_avatar_snapshots` 保存静态 Snapshot。

不得长期引用 Discord 当前头像作为自动公开 Profile。

`identity_display_snapshots`：

```text
snapshot_id
newapi_user_id
display_name_snapshot
avatar_snapshot_id
created_at
```

为 Immutable Identity Event Snapshot。

Poker Session / Table Chat / Recent Wins 等可以引用该 Snapshot。

---

# 81. Discord Binding Authority

Discord Binding 的账号级 Source of Truth 继续属于 NewAPI Account System。

Chaldea 可以保存：

- Registration Operation；
- Support Case；
- Binding Audit；
- Registration Guard。

但不得建立第二套普通可编辑 Binding Authority 与 NewAPI 竞争。

实际一对一 Unique Constraint 的落点：

```text
SOURCE VERIFICATION REQUIRED
```

优先级：

1. 使用 / 加强 NewAPI 现有 Binding Unique Constraint；
2. 如真实实现无法安全满足，再设计 Chaldea Registration Guard；
3. 永远避免两个可写 Authority 同时存在。

---

# 82. API Key Usage Purpose Metadata

Chaldea 不保存 API Key Secret 副本。

只保存：

```text
newapi_key_id
newapi_user_id
current_purpose
effective_version
updated_at
```

Purpose：

```text
GENERAL
RP
UNCLASSIFIED
```

History：

```text
api_key_purpose_history
```

Append-only：

```text
key_id
old_purpose
new_purpose
effective_at
changed_by
version
```

---

# 83. Request Purpose Snapshot

每条合格 API Request 必须形成不可变：

```text
key_purpose_snapshot
```

禁止后续通过 Key 当前 Purpose 重写历史。

## 83.1 优先方案 A — Existing NewAPI Log Attribution

如果真实 NewAPI Log 可以稳定提供：

```text
stable key id
user id
logical request identity
request-start timestamp
model id
final status
final cost
```

则：

```text
NewAPI Log
+
immutable purpose history
→ Attribution Worker
→ api.request_attributions
```

优先采用此方案。

## 83.2 方案 B — Minimal Request Attribution Hook

如果 NewAPI Log 缺少关键事实：

增加最小 Request Attribution Hook。

只记录：

```text
logical_request_id
newapi_user_id
stable key id
key_purpose_snapshot / purpose version
model id
request-start time
final status / cost
```

不改变：

- Auth；
- Model Routing；
- Billing；
- Provider invocation。

## 83.3 禁止方案

```text
historical request
→ later lookup current key purpose
```

永久禁止。

---

# 84. Model Catalog Identity

真实：

```text
model_id
```

为稳定 Domain Identity。

Display Name / Persona / Metadata 变化不改变 Model Identity。

Retired Model：

```text
retired
≠ delete
```

历史 Usage / Rankings 必须仍可恢复：

```text
model id
historical display identity
```

---

# 85. Asset Data Type Baseline

TD-03 冻结：

```text
asset balance
wager
payout
transfer amount
→ BIGINT atomic units
```

禁止使用：

```text
float32
float64
REAL
DOUBLE PRECISION
```

作为账务事实。

当前：

```text
1 API Credit
= 500,000 atomic units

1 NewAPI raw quota
= 1 Chaldea credit atomic unit
```

娱乐筹码同样使用 BIGINT Atomic Units。

完整 Wallet / Ledger / Transfer State Machine 进入 TD-04。

---

# 86. Durable Record Delete Policy

V1 Durable Business Records 默认使用：

```text
ON DELETE RESTRICT
```

而不是跨域级联删除。

特别适用于：

```text
wallet ledger
reward claims
game rounds
poker sessions / hands
ranking source
audit
migration records
```

业务退役 / 隐藏优先使用：

```text
RETIRED
ARCHIVED
DISABLED
RELEASED through audited operation
```

而不是 Hard Delete。

User Disabled 不删除历史资产、Usage、Game、Poker 或 Audit。

---

# 87. Schema Migration vs Product Cutover

必须区分两个子系统。

## 87.1 Schema Migration

用于版本化数据库结构演进。

## 87.2 Product Cutover Migration

用于现有 NewAPI 用户正式迁移到 Chaldea Launch Economy。

两者：

- 不共享含糊状态；
- 不使用相同执行脚本语义；
- 不把 Product Cutover 当普通 DDL Migration。

---

# 88. Schema Migration Framework

采用：

```text
versioned immutable SQL migrations
```

逻辑命名例如：

```text
000001_identity_base
000002_economy_base
000003_game_registry
...
```

Applied Migration 保存：

```text
version
name
checksum
applied_at
app_build
execution_result
```

规则：

1. 已应用 Migration File 不修改；
2. Checksum mismatch 阻止 Deploy；
3. Runtime App 无 DDL；
4. App Startup 不任意 ALTER TABLE；
5. Deploy 前运行 Dedicated Migration Job；
6. Migration 成功后 Application 才 Ready。

---

# 89. Database Rollback Philosophy

采用：

> Forward-compatible / Forward-fix 为主。

只有真正可逆的 DDL 才提供安全 Down Migration。

不可逆 Data Migration：

```text
backup
→ migrate
→ verify
```

不能假装一条 Down SQL 可以恢复历史账务。

---

# 90. Durable Product Cutover State Machine

Batch State：

```text
PLANNED
↓
PRECHECKED
↓
BACKUP_READY
↓
MAINTENANCE_LOCKED
↓
SNAPSHOT_COMPLETE
↓
RESET_COMPLETE
↓
GRANT_COMPLETE
↓
VERIFYING
↓
VERIFIED
↓
READY_TO_OPEN
↓
COMPLETED
```

异常：

```text
FAILED_RETRYABLE
ROLLBACK_REQUIRED
ROLLED_BACK
REPAIR_REQUIRED
```

`READY_TO_OPEN`：

> 表示迁移事实已经通过验证，但用户流量尚未恢复。

从：

```text
READY_TO_OPEN
→ COMPLETED
```

必须经过明确 Open Gate。

---

# 91. Per-user Cutover State

每个迁移用户拥有 durable state：

```text
PENDING
↓
SNAPSHOTTED
↓
RESET_VERIFIED
↓
GRANT_CONFIRMED
↓
VERIFIED
```

异常：

```text
FAILED
NEEDS_REVIEW
```

至少记录：

```text
migration_batch_id
newapi_user_id

pre_cutover_raw_quota
post_reset_raw_quota

reserve_initialized_units
chip_initialized_units
poker_in_play_initialized_units

initial_grant_biz_id
initial_grant_units
initial_grant_state

started_at
updated_at
verified_at

failure_code
```

---

# 92. Cutover Maintenance Boundary

正式 Cutover 前必须停止相关新写入。

至少包括：

```text
new registrations
API charging / relevant NewAPI writes
Wallet / Exchange
Rewards claims
new Game rounds
Poker asset-changing activity
```

待业务进入可恢复安全点后才开始 Snapshot / Reset。

真实 NewAPI 如何暂停相关写入：

```text
SOURCE VERIFICATION REQUIRED
```

---

# 93. Pre-cutover Backup Set

至少保存：

```text
newapi database backup
chaldea_platform database backup

PostgreSQL role / grant metadata

NewAPI user quota export snapshot

schema migration version manifest
application build / version manifest

migration scope manifest
migration batch id
```

因为两个 Database 没有一个普通跨库事务，所以必须：

```text
Maintenance
→ freeze related writes
→ capture both authorities at stable cutover point
```

Redis Backup 不作为 Financial Recovery Authority。

---

# 94. Per-user Reset Sequence

推荐执行：

```text
1. Ensure cutover_user row
2. Save pre-cutover NewAPI quota snapshot
3. Ensure Chaldea account_ref
4. Initialize / verify Chaldea asset rows = 0
5. Reset Active NewAPI Quota = 0
6. Verify NewAPI Quota = 0
7. Mark RESET_VERIFIED
8. Request Migration Initial Grant
9. Query Grant by stable biz_id
10. Confirm Grant
11. Verify final asset invariants
12. Mark user VERIFIED
```

第 4 / 5 步不构成普通跨库 Atomic Transaction。

一致性依赖：

```text
durable migration state
+
maintenance
+
verification
+
resume / rollback
```

---

# 95. NewAPI Quota Reset

具体 API / SQL：

```text
SOURCE VERIFICATION REQUIRED
```

语义先冻结：

首次执行：

```text
expected pre-cutover quota
→ reset to 0
→ record before / after
```

Retry：

```text
already 0
+
same migration batch
+
matching snapshot
→ verify and continue
```

如果当前 Quota 与 Snapshot 不一致：

```text
→ NEEDS_REVIEW
```

禁止盲目覆盖。

---

# 96. Chaldea Zero Initialization

Chaldea 上线前不存在的：

```text
Reserve
Entertainment Wallet
Poker In Play
```

初始化为：

```text
0 atomic units
```

并保留 Migration Initialization / Reset Audit。

之后的 1,000 API Credit 必须由正式 Grant / Ledger 路径产生。

禁止：

```text
direct set final wallet balance = grant amount
```

而不产生 Grant / Ledger Record。

---

# 97. Migration Initial Grant

稳定业务 ID：

```text
initial_grant:migration:{migration_batch_id}:{newapi_user_id}
```

或完全等价语义。

金额：

```text
1,000 API Credit
= 500,000,000 atomic units
```

目标：

```text
Reserve API Credit
```

由正式 Reward / Economy Boundary 执行。

Cutover Orchestrator 不自行直接修改最终余额。

---

# 98. Cutover Batch Idempotency

同一 `migration_batch_id` 重跑：

- 不创建第二份 snapshot；
- 不重复 Reset；
- 不重复 Grant；
- 不重复 Profile；
- 不重复 API Key Purpose Initialization；
- 不重复 Migration Notice Ack。

恢复：

```text
read durable fact
→ verify last confirmed state
→ continue next legal step
```

不从头盲目重跑。

---

# 99. Existing API Key Migration

NewAPI Key：

```text
preserve unchanged
```

只补 Chaldea Metadata：

```text
purpose = UNCLASSIFIED
```

并建立：

```text
UNIQUE(newapi_key_id)
```

仅在不存在 Purpose Metadata 时初始化。

不得覆盖 Cutover 后用户已经修改成：

```text
GENERAL
RP
```

的记录。

---

# 100. Migration Notice

`migration_notice_versions`：

保存发布版本。

Acknowledgement：

```text
newapi_user_id
migration_notice_version
acknowledged_at
```

并建立等价：

```text
UNIQUE(user, version)
```

用户点击：

```text
我已了解，继续
```

只创建 Ack。

它不执行：

- Reset；
- Grant；
- Cutover。

资产迁移应在用户首次登录前由 Batch 完成。

---

# 101. Post-cutover Verification Gate

进入 `READY_TO_OPEN` 前至少验证：

## Account Preservation

```text
user scope / count
Discord binding integrity
Password / account reference integrity
API Key reference integrity
```

## NewAPI Quota

所有迁移用户：

```text
Active quota = 0
```

## Chaldea Assets

无其他新账变时：

```text
API Credit = 1000
Available Chips = 0
Poker In Play = 0
Total Assets = 1000
```

## Initial Grant

```text
exactly one migration grant per in-scope user
```

## API Keys

```text
preserved
purpose = UNCLASSIFIED
```

仅适用于此前没有 Purpose Metadata 的迁移记录。

## Historical Logs

仍可查询。

## Migration State

不得存在未处理：

```text
FAILED
NEEDS_REVIEW
unverified user
duplicate biz_id
```

必须生成 Batch Verification Report。

---

# 102. Rollback Boundary

## 102.1 Platform 尚未重新开放

发生：

```text
reset integrity uncertain
grant integrity uncertain
verification failed
```

保持 Maintenance。

优先：

```text
resume same batch
```

若无法安全 Resume：

```text
ROLLBACK_REQUIRED
→ restore pre-cutover backups
→ verify restored system
→ ROLLED_BACK
```

## 102.2 Platform 已开放并产生新经济事实

如果已经产生：

```text
API consumption
Daily / Hourly / Relief
Wallet Exchange
Game Round
Poker asset change
Admin Adjustment
```

则禁止：

```text
restore old quota only
```

进入：

```text
REPAIR_REQUIRED
```

通过：

```text
migration repair
+
ledger adjustment / compensation
+
audit
```

修复。

---

# 103. Migration Evidence Retention

至少持久保留：

```text
migration batch
pre-cutover quota snapshot
balance reset audit
initial grant reference
verification report
migration notice version
```

具体 Retention 期限、Backup Rotation 和 Restore Drill 进入 TD-12。

---

# 104. NewAPI Source Verification Checklist

进入 Implementation Spec 前必须核验实际部署版本：

1. User PK 实际类型；
2. Quota Field / Update Semantics；
3. Discord Binding 存储位置；
4. Discord User ID Unique Constraint；
5. Password Login Identifier；
6. API Key Stable ID；
7. API Key Secret Storage；
8. Request Log 是否有 Stable Key ID；
9. Request-start Timestamp；
10. Logical Request / Internal Retry 可识别性；
11. Model ID；
12. Final Quota / Cost / Status；
13. Account Status；
14. Password Auth / Set / Change / Reset API；
15. Available Read API；
16. 是否需要 Narrow Internal Bridge。

在读取源码前，禁止写死具体 NewAPI Column / Endpoint。

---

# 105. TD-03 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| TD-FRZ-041 | 保持一个 PostgreSQL Instance + `newapi` / `chaldea_platform` 两个 Database，不进一步拆分 V1 数据库。 | FROZEN |
| TD-FRZ-042 | `chaldea_platform` 采用按业务域划分的 PostgreSQL Schema 方案 B，不把全部业务表堆在 `public`。 | FROZEN |
| TD-FRZ-043 | 禁止通过跨库 FK、dblink、FDW 或 Trigger 将 NewAPI 与 Chaldea 伪装为单一事务数据库；跨库引用使用稳定 External ID + Adapter + Reconciliation。 | FROZEN |
| TD-FRZ-044 | 使用独立 `chaldea_owner / migrator / app / poker / newapi_ro / cutover` 权限边界；Runtime App 不拥有 DDL。 | FROZEN |
| TD-FRZ-045 | Poker Runtime 只直接 RW Poker-owned Schema，不直接写 Economy；Platform Backend 不绕过 Poker Service 任意修改 Poker Authority。 | FROZEN |
| TD-FRZ-046 | Chaldea 新实体默认使用 UUIDv7；已有自然稳定标识如 `newapi_user_id`、real `model_id`、`game_slug` 保持原语义。 | FROZEN |
| TD-FRZ-047 | `newapi_user_id` 保持用户真实归属，并通过 `identity.account_refs` 建立本地 FK Anchor；真实 DB 类型必须源码核验，不提前强制转换。 | FROZEN |
| TD-FRZ-048 | Entity ID 与 `biz_id` 分离；`biz_id` 是业务幂等身份，并按业务域建立唯一约束。 | FROZEN |
| TD-FRZ-049 | 正式时间使用 PostgreSQL `TIMESTAMPTZ` / UTC；`Asia/Shanghai` 等业务时区只用于周期和显示。 | FROZEN |
| TD-FRZ-050 | 权威业务状态使用强类型 Column；JSONB 仅用于 Metadata / Snapshot / Extension，不作为 Balance、Hand、Settlement 等核心事实的替代结构。 | FROZEN |
| TD-FRZ-051 | `chaldea_platform` 使用 `platform_meta / identity / api / catalog / economy / rewards / games / poker / content / ranking / ops / audit / migration` 顶层 Schema 分区。 | FROZEN |
| TD-FRZ-052 | Master Profile 使用标准化昵称唯一约束、Name History、Reserved Name、Avatar Snapshot 与 Immutable Display Snapshot。 | FROZEN |
| TD-FRZ-053 | Discord Binding 不在 Chaldea 与 NewAPI 形成两个可编辑 Authority；一对一 Unique Constraint 的实际落点继续 `SOURCE VERIFICATION REQUIRED`。 | FROZEN |
| TD-FRZ-054 | Chaldea API Key Purpose Metadata 只保存 stable NewAPI Key ID、User、Purpose 与 History，不复制 Key Secret。 | FROZEN |
| TD-FRZ-055 | `key_purpose_snapshot` 必须形成不可变 Request Attribution；优先复用真实 NewAPI Log，若信息不足才增加最小 Request Attribution Hook，禁止后来用当前 Purpose 重写历史。 | FROZEN |
| TD-FRZ-056 | Model Catalog 保留真实 `model_id` 为稳定 Domain Identity；Retired Model 不删除历史映射。 | FROZEN |
| TD-FRZ-057 | 所有资产字段继续采用 BIGINT Atomic Units；完整 Wallet / Ledger / Transfer State Machine 推迟至 TD-04。 | FROZEN |
| TD-FRZ-058 | V1 Durable Business Records 默认禁止级联硬删除；User Disabled 不删除 Ledger、Usage、Game、Poker、Migration 或 Audit，历史对象优先 Archive / Retire / Status。 | FROZEN |
| TD-FRZ-059 | Schema Migration 使用版本化不可变 Migration + Checksum + Dedicated Migrator；Runtime Startup 不自动执行任意 DDL。 | FROZEN |
| TD-FRZ-060 | Production Database Migration 以 Forward-fix 为主；不可逆 Data Migration 不伪装成普通 Down Migration。 | FROZEN |
| TD-FRZ-061 | Schema Migration 与 Chaldea Product Cutover Migration 是两个独立子系统，不共享含糊状态模型。 | FROZEN |
| TD-FRZ-062 | Cutover 使用 Durable Batch State Machine：Precheck → Backup → Maintenance → Snapshot → Reset → Grant → Verify → Ready-to-open → Complete。 | FROZEN |
| TD-FRZ-063 | 每个迁移用户拥有独立 Durable Cutover State，可从最后确认步骤 Resume，不从头盲目重跑。 | FROZEN |
| TD-FRZ-064 | Cutover 前必须停止相关写入并取得 NewAPI + Chaldea Authority Backup、Quota Snapshot、Schema / Build / Migration Manifest。 | FROZEN |
| TD-FRZ-065 | Migrated User 严格执行 Active NewAPI Quota / Reserve / Chips / Poker In Play 清零后，再通过正式 Grant Boundary 发放 1,000 API Credit。 | FROZEN |
| TD-FRZ-066 | Migration Initial Grant 使用稳定 `initial_grant:migration:{batch}:{user}` 等价 Biz ID，进入 Reserve，并产生正常 Ledger / Grant Record，不直接编辑最终 Balance。 | FROZEN |
| TD-FRZ-067 | Existing API Keys 完整保留；只在不存在 Chaldea Purpose Metadata 时初始化 `UNCLASSIFIED`，不得覆盖用户后续修改。 | FROZEN |
| TD-FRZ-068 | 同一 `migration_batch_id` 所有步骤必须幂等；Network / Crash Recovery 通过查询 Durable Fact 继续。 | FROZEN |
| TD-FRZ-069 | 平台开放前失败允许 Resume 或完整 Backup Restore；平台开放产生新经济事实后禁止恢复单独旧 Quota，必须走 Migration Repair / Compensation。 | FROZEN |
| TD-FRZ-070 | Migration Notice Version / Acknowledgement 只确认已完成的迁移事实，不触发 Reset 或 Grant；每用户每 Version Ack 一次。 | FROZEN |
| TD-FRZ-071 | Batch 必须通过 Post-cutover Verification Report 且所有用户无 unresolved failure 后才能进入 `READY_TO_OPEN`。 | FROZEN |
| TD-FRZ-072 | Migration Snapshot、Reset Audit、Grant Reference、Verification Report 属于 Durable Audit Evidence；具体 Retention 周期在 TD-12 冻结。 | FROZEN |
| TD-FRZ-073 | 所有依赖实际 NewAPI Table / Field / API / Log Capability 的细节继续标记 `SOURCE VERIFICATION REQUIRED`，不得凭模型记忆填充。 | FROZEN |

---

# 106. Change Log — WORKING v0.3

## Added

- 用户正式确认 `TD-03`；
- 正式采用 PostgreSQL Schema 方案 B；
- 正式采用 Hybrid ID Strategy；
- 正式采用 Durable Cutover State Machine；
- 冻结 `TD-FRZ-041 ～ TD-FRZ-073`；
- 冻结 Chaldea Database Schema Ownership；
- 冻结 Cross-database Boundary；
- 冻结 Database Roles；
- 冻结 UUIDv7 + Domain ID Hybrid Strategy；
- 冻结 `identity.account_refs`；
- 冻结 Master Profile / Name / Avatar / Display Snapshot 顶层模型；
- 冻结 API Key Purpose Metadata / Request Attribution 边界；
- 冻结 Model Catalog Identity；
- 冻结 BIGINT Atomic Unit 数据类型基线；
- 冻结 Durable Record Delete Policy；
- 冻结 Schema Migration Framework；
- 冻结 Product Cutover Batch / User State；
- 冻结 Backup / Resume / Rollback Boundary；
- 冻结 Existing API Key Migration；
- 冻结 Migration Notice Ack Boundary；
- 冻结 Post-cutover Verification Gate；
- 更新 NewAPI Source Verification Checklist。

## Not Changed

本批没有改变：

- NewAPI Core 作为 Account / Password / Key / Model / Billing Authority；
- TD-01 Service Boundary；
- TD-02 Auth / Session Boundary；
- Wallet / Ledger 业务不变量；
- Reward 数值；
- Game 数学；
- Poker 规则；
- IA Route；
- Art Direction。

---

# 107. 下一批 — TD-04

下一批正式进入：

> **TD-04 — Economy / Ledger / Transfer / Reconciliation**

这是 v0.5 第一批真正的高风险核心设计。

计划完整冻结：

1. Asset Types / Atomic Units；
2. Wallet Authority；
3. Reserve API Credit；
4. Available Chips；
5. Active NewAPI Quota Mapping；
6. Poker In Play 边界；
7. Processing Assets；
8. Total Assets 计算；
9. Wallet Row Model；
10. Append-only Ledger；
11. Balance Before / Delta / After；
12. `biz_type / biz_id`；
13. Ledger Exactly-once；
14. Database Lock / Conditional Update；
15. Negative Balance Prevention；
16. API Credit ↔ Chips 1:1 Exchange；
17. Cross-database Transfer Saga；
18. Transfer Intent；
19. State Machine；
20. Source Debit；
21. Target Credit；
22. Retry / Resume；
23. Compensation；
24. Network Uncertain；
25. Reconciliation Worker；
26. Crash Point Analysis；
27. Admin Adjustment；
28. Issuance / Burn；
29. Poker Buy-in / Top-up / Cash Out Economy Boundary；
30. Migration Grant Economy Linkage；
31. Audit；
32. Metrics / Alerts；
33. Concurrency Tests；
34. Recovery Tests。

TD-04 必须给出：

- State Transition Table；
- Sequence Diagram；
- Transaction Boundary；
- Lock Strategy；
- Crash Point Analysis；
- Exactly-once / At-least-once Semantics。



---

# 108. TD-04 — Economy / Ledger / Transfer / Reconciliation

> 状态：`FROZEN`  
> 用户确认：`可以`  
> 技术解析：批准 `TD-04-C01 — Bounded Active Buffer + Reactive Refill`

## 108.1 TD-04 总体结论

本批正式冻结：

- Reserve API Credit / Active NewAPI Quota / Available Chips / Poker In Play / Processing Assets Authority；
- Total Assets 统一原子单位口径；
- Materialized Wallet + Append-only Ledger；
- `asset_transactions` / Ledger Legs；
- Request / Idempotency / Business / Transaction ID 分离；
- Wallet Row Lock / Non-negative Invariant；
- Chips → API Credit 本地原子转换；
- API → Chips Reserve-first / Active-shortfall；
- Cross-DB Idempotent Saga；
- Target-local Idempotency；
- Transfer Legs / Processing Assets；
- At-least-once + Idempotent Effects；
- Unknown Result Query-first；
- Reconciliation Worker；
- Active Liquidity Controller；
- Reserve → Active Refill；
- Poker Buy-in / Top-up / Cash Out 原子资金边界；
- Direct Play Wager / Settlement 顶层 Economy Boundary；
- Reward / Grant / Migration Grant Economy Boundary；
- Admin Adjustment；
- Asset API Integer / Decimal String Contract；
- Fresh Authoritative Asset Snapshot；
- Crash Point Analysis；
- Metrics；
- Ledger Reconciliation；
- TD-04 高风险测试 Gate。

本批不决定：

- Hourly Reward 的资产类型；
- Hourly Reward 的时间口径 / 累积 / Daily Limit；
- Relief Fund 的资产类型；
- Relief Fund 的资格累积与 Active Poker 行为；
- NewAPI Quota API / Field / Operation Journal 的具体实现名称；
- Active Liquidity Low / Target / Max Watermark 具体数值；
- Direct Play 各游戏数学；
- Poker Hand / Pot / Side Pot 详细算法。

---

# 109. Asset Authority Model

用户侧仍然只理解：

```text
API Credit
Entertainment Chips
Poker In Play
Processing Assets
Total Assets
```

内部 Authority：

| Position | Authority | Durable Store | Primary Use |
|---|---|---|---|
| Reserve API Credit | Chaldea Economy | `chaldea_platform.economy` | API 资产长期持有、兑换来源 |
| Active API Quota | NewAPI Core | `newapi` | API 请求实际消费 |
| Available Chips | Chaldea Economy | `chaldea_platform.economy` | Direct Play、Poker Buy-in、Exchange |
| Table Stack | Poker | `chaldea_platform.poker` | Poker 行动 |
| Committed This Hand | Poker | `chaldea_platform.poker` | 当前 Hand 未结算投入 |
| Processing Assets | Economy Transfer State | `economy.transfers / legs` | 未完成跨 Authority 资产 |

Poker In Play：

```text
Table Stack
+
Committed This Hand
```

NewAPI Active Quota 不在 Chaldea 建立第二个可写 Balance Authority。

---

# 110. Total Assets

统一以 Atomic Units 计算：

```text
Total Assets Units
=
Reserve API Credit Units
+
Active NewAPI Raw Quota
+
Available Chip Units
+
Poker In Play Units
+
Unrepresented Processing Units
```

关键原则：

> Processing 只包含已经离开来源、又尚未存在于任何 Target Settled Position 的资产。

同一资产单位不得同时计入：

```text
Source
Processing
Target
```

多个 Bucket。

---

# 111. Wallet Balance Model

`economy.wallet_balances` 只保存 Chaldea-owned Materialized Positions。

V1 主要：

```text
RESERVE_API_CREDIT
AVAILABLE_CHIPS
```

逻辑结构：

```text
newapi_user_id
asset_type
balance_units BIGINT
ledger_seq BIGINT
version BIGINT
updated_at

UNIQUE(newapi_user_id, asset_type)
CHECK(balance_units >= 0)
```

不把以下对象硬塞入普通 Wallet Balance：

```text
ACTIVE_API_QUOTA
POKER_IN_PLAY
PROCESSING_ASSETS
```

---

# 112. Materialized Balance + Append-only Ledger

采用：

```text
wallet_balances
→ Current Materialized State

wallet_ledger
→ Immutable Financial History
```

Balance Mutation 与 Ledger Entry 必须同一个 DB Transaction：

```text
BEGIN
→ lock wallet
→ validate
→ update balance
→ insert ledger
→ COMMIT
```

任何一步失败：

```text
ROLLBACK
```

不得：

```text
update balance
→ async append ledger later
```

---

# 113. Wallet Ledger

建议逻辑字段：

```text
ledger_entry_id UUIDv7
transaction_id UUIDv7

newapi_user_id
asset_type

delta_units BIGINT
balance_before_units BIGINT
balance_after_units BIGINT

ledger_seq
leg_key

biz_type
biz_id

reverses_entry_id nullable

created_at
metadata limited JSONB
```

不变量：

```text
balance_after = balance_before + delta
```

同一 Wallet：

```text
entry[n].balance_before
=
entry[n-1].balance_after
```

`ledger_seq` 单调递增。

---

# 114. Ledger Append-only Permission

Runtime Role：

```text
INSERT wallet_ledger  ✓
SELECT wallet_ledger  ✓

UPDATE wallet_ledger  ✗
DELETE wallet_ledger  ✗
```

错误修正：

```text
original entry
+
new REVERSAL / ADJUSTMENT entry
```

不得改写原账。

---

# 115. Business Transaction / Ledger Leg

引入：

```text
economy.asset_transactions
```

逻辑：

```text
transaction_id
biz_type
biz_id
newapi_user_id
operation_type
status
created_at
confirmed_at

UNIQUE(biz_type, biz_id)
```

一个 Business Operation 可以拥有多个 Ledger Leg：

```text
wallet_ledger
→ transaction_id
→ leg_key
```

并建立：

```text
UNIQUE(transaction_id, leg_key)
```

用于支持：

```text
CHIPS -X
RESERVE +X
```

等复合业务。

---

# 116. Four ID Classes

严格分离：

```text
request_id
→ Observability / Trace

idempotency_key
→ Client Duplicate Submission Guard

biz_id
→ Stable Business Fact Identity

transaction_id
→ Database Entity ID
```

同一个 `idempotency_key` 若绑定不同规范化 Payload：

```text
409 IDEMPOTENCY_CONFLICT
```

不得创建第二笔资产事实。

---

# 117. Wallet Concurrency / Lock Order

扣款使用：

```text
SELECT ... FOR UPDATE
```

或等价 Atomic Conditional Update。

禁止：

```text
SELECT balance
→ no lock
→ ordinary UPDATE
```

涉及多个 Wallet Row 时按固定 Asset Order 锁定。

数据库继续使用：

```text
CHECK(balance_units >= 0)
```

作为最终不变量。

---

# 118. Chips → API Credit

由于：

```text
Available Chips
Reserve API Credit
```

都在 `chaldea_platform`，

因此 Chips → API 不使用 Saga。

单 DB Transaction：

```text
BEGIN
→ lock Chips
→ lock Reserve
→ validate Chips
→ debit Chips
→ ledger
→ credit Reserve
→ ledger
→ mark asset_transaction CONFIRMED
→ COMMIT
```

失败：

```text
no partial effect
```

API Credit 目标固定进入 Reserve。

---

# 119. API → Chips Source Strategy

采用：

> **Reserve-first / Active-shortfall**

例如：

```text
Exchange = 100
Reserve  = 70
Active   = 80
```

内部：

```text
Reserve Debit = 70
Active Debit  = 30
Chips Credit  = 100
```

如果 Reserve 足够：

```text
entire operation local
```

只有不足部分触碰 NewAPI。

---

# 120. Cross-DB Consistency Strategy

正式采用：

```text
Idempotent Saga
+
Durable Transfer State
+
Target-local Idempotency
+
Reconciliation
+
Compensation
```

明确禁止：

```text
Naive Dual Write
```

V1 不采用：

```text
PostgreSQL 2PC / Prepared Transaction
```

作为 NewAPI / Chaldea 跨库业务一致性机制。

---

# 121. Cross-DB Transfer State Machine

正式状态：

```text
PENDING
↓
SOURCE_DEBITING
↓
SOURCE_DEBITED
↓
TARGET_CREDITING
↓
TARGET_CREDITED
↓
CONFIRMED
```

异常：

```text
PENDING
→ FAILED_NO_EFFECT
```

Source 已产生资产影响、Target 确定无法完成：

```text
SOURCE_DEBITED
→ COMPENSATING
→ COMPENSATED
```

无法自动确定下一步：

```text
→ NEEDS_REVIEW
```

终态：

```text
CONFIRMED
COMPENSATED
FAILED_NO_EFFECT
```

`NEEDS_REVIEW` 是人工/自动系统无法安全判断下一步，不表示资产可以被忽略。

---

# 122. Transfer Legs

引入：

```text
economy.transfer_legs
```

每个 Leg 至少表达：

```text
leg_id
transfer_id

authority
position
direction

amount_units

effect_operation_id
effect_state

applied_at
reversed_at
```

例如：

```text
API → Chips 100

Leg A
CHALDEA / RESERVE / DEBIT / 70

Leg B
NEWAPI / ACTIVE / DEBIT / 30

Leg C
CHALDEA / AVAILABLE_CHIPS / CREDIT / 100
```

---

# 123. Processing Assets

Processing 不维护独立易漂移的 Wallet Balance。

从 Durable Transfer / Leg Facts 推导。

原则：

```text
每一个资产原子单位
在 Transfer 生命周期中
恰好属于以下一个 Bucket：

Source Settled
Processing
Target Settled
Restored Source
```

不得重复计入。

---

# 124. User-visible Transfer State Mapping

内部：

```text
PENDING
SOURCE_DEBITING
SOURCE_DEBITED
TARGET_CREDITING
TARGET_CREDITED
```

映射：

```text
Processing / 处理中
```

```text
CONFIRMED
→ Completed / 已完成
```

```text
COMPENSATING
→ Returning Assets / 退回中
```

```text
COMPENSATED
→ Returned / 已退回
```

```text
NEEDS_REVIEW
→ Needs Attention / 需要处理
```

```text
FAILED_NO_EFFECT
→ Not Executed / 未执行
```

普通用户不显示内部 Saga State Name。

---

# 125. No Ordinary Cancel

用户确认并产生正式 Transfer Intent 后：

```text
no normal user cancel
```

用户可以离开页面。

服务端继续：

```text
finish
compensate
needs review
```

网络超时后：

```text
query original transfer
```

禁止盲目创建第二笔。

---

# 126. Target-local Idempotency

任何跨 Authority Effect 必须拥有稳定：

```text
effect_operation_id
```

Target 对同一个 Operation ID：

```text
apply once
or
return original outcome
```

不能重复产生资产变化。

---

# 127. NewAPI Quota Mutation Boundary

优先：

```text
verified NewAPI idempotent quota operation capability
```

如果不存在，则使用窄：

```text
Quota Bridge
+
Target-side Operation Journal
```

该 Journal 必须与 NewAPI Quota Mutation 处于同一个 NewAPI-local DB Transaction。

逻辑：

```text
operation_id
newapi_user_id
delta_raw_quota
before_quota
after_quota
result
created_at
```

是否需要该 Bridge：

```text
SOURCE VERIFICATION REQUIRED
```

但 Target-local Idempotency Requirement 本身已冻结。

---

# 128. Cross-DB Delivery Semantics

正式表述：

> **At-least-once Execution Attempts + Idempotent Effects = Effectively Exactly-once Business Outcome**

不得声称：

```text
HTTP / Network Delivery
= exactly once
```

Retry / Restart 可以重复发送同一个 Operation ID。

Target 必须返回同一原始 Outcome。

---

# 129. Unknown Result Query-first

例如：

```text
NewAPI debit sent
→ response timeout
```

禁止：

```text
assume failed
→ compensate immediately
```

也禁止：

```text
assume success
→ credit target
```

必须：

```text
query effect_operation_id
```

结果：

```text
APPLIED
→ continue

NOT_APPLIED
→ legal retry / compensation path

UNKNOWN
→ wait / NEEDS_REVIEW
```

---

# 130. TD-04-C01 — Active Liquidity Gap

已确认采用：

> **Bounded Active Buffer + Reactive Refill**

目的：

```text
Reserve
= Long-term API Asset Holding

Active
= Bounded API Consumption Liquidity Layer
```

用户不需要手动执行 Active Top-up。

---

# 131. Active Quota Liquidity Controller

维护内部参数：

```text
LOW_WATERMARK
TARGET_WATERMARK
MAX_ACTIVE_BUFFER
```

精确数值：

```text
NOT YET FROZEN
```

属于：

```text
INTERNAL TECHNICAL CONFIG
```

最终需基于：

- NewAPI 预扣费语义；
- 单请求最大合理消费；
- 并发；
- 模型价格；
- 实际负载测试。

确定。

---

# 132. Liquidity Refill Behavior

正常：

```text
Active > Low
→ no action

Active <= Low
AND Reserve > 0
→ Reserve → Active Refill
→ toward Target
```

并保持：

```text
Target <= Max Active Buffer
```

如果某次请求需要更多 Active：

```text
Reactive Refill
```

如何嵌入 NewAPI Admission / Insufficient Quota Path：

```text
SOURCE VERIFICATION REQUIRED
```

---

# 133. Reserve → Active Refill Accounting

Refill 是资产 Position Movement，不是：

- Issuance；
- Reward；
- Consumption。

示例：

```text
Reserve 1000 / Active 0
→
Reserve 900 / Active 100

Total API unchanged
```

因为跨两个 Database：

```text
Reserve → Active
```

本身必须走正式 Cross-DB Saga。

异常中资产通过 Processing Bucket 保持可解释。

---

# 134. Cutover / Liquidity Boundary

TD-03 的：

```text
Active Quota = 0
```

继续作为 Cutover Reset Verification Fact。

Runtime 顺序：

```text
Cutover Reset
→ verify Active = 0
→ Migration Grant to Reserve
→ Batch Verification
→ READY_TO_OPEN
→ Liquidity Controller may prime Active
→ Resume normal API writes
```

Active Refill 不改变 Cutover Reset Evidence。

---

# 135. Chips → API Destination

用户执行：

```text
Chips → API Credit
```

内部固定：

```text
Available Chips
→ Reserve API Credit
```

不会在同一用户 Exchange 操作中强制直接 Credit Active。

Active 由 Liquidity Controller 独立管理。

---

# 136. API → Chips Sequence

示例：

```text
requested = 100
Reserve = 70
Active = 80
```

流程：

```text
Client
→ Confirm + Idempotency Key

Economy
→ create Transfer PENDING
→ plan Reserve=70 / Active=30 / Chips=100

Local TX
→ lock Reserve
→ debit 70
→ Ledger
→ COMMIT

NewAPI Idempotent Effect
→ debit Active 30

Local TX
→ lock Chips
→ credit 100
→ Ledger
→ COMMIT

→ TARGET_CREDITED
→ CONFIRMED
```

若 NewAPI Active 在提交时不足：

```text
Active Debit
→ INSUFFICIENT
```

则已扣 Reserve 进入：

```text
COMPENSATING
→ credit Reserve back
→ append reversal ledger
→ COMPENSATED
```

不透支。

---

# 137. API Consumption vs Exchange Concurrency

API Consumption 继续由 NewAPI Authority 处理。

Exchange Active Debit 同样通过 NewAPI 原子 Quota Mutation。

两者竞争 Active Quota：

```text
NewAPI transaction order decides
```

必须保证：

```text
Active quota never < 0
```

页面预览不构成最终资金预留。

提交时必须重新验证。

---

# 138. NewAPI Active Event Mirror

Chaldea 不维护第二个 Active Balance。

可建立：

```text
economy.external_asset_events
```

作为 Append-only Mirror：

```text
authority
source_event_id
newapi_user_id
event_type
delta_raw_quota
occurred_at
observed_at
logical_request_id
```

用途：

- Audit；
- Transaction Composition；
- Reconciliation；
- Usage Attribution。

但：

```text
NewAPI quota
```

仍然是 Active Balance Authority。

---

# 139. Local Exactly-once

单个 Chaldea Database 内：

```text
Unique Business Identity
+
Row Lock
+
Balance / Ledger Same Transaction
```

实现：

> Exactly-once committed business effect.

重复客户端请求只返回原业务结果。

---

# 140. Reconciliation Worker

扫描：

```text
non-terminal transfer
+
next_attempt_at <= now
```

通过：

```text
FOR UPDATE SKIP LOCKED
```

领取任务。

持久化：

```text
attempt_count
last_attempt_at
next_attempt_at
last_error_category
```

采用：

```text
exponential backoff + jitter
```

精确 Retry Timing 进入 Implementation Spec。

---

# 141. Reconciliation Decision Principle

Worker 只根据 Durable Facts 决策：

```text
Chaldea ledger leg exists?
NewAPI operation applied?
Target wallet leg exists?
Compensation applied?
```

不能根据内存或“推测应该执行到了哪里”恢复。

---

# 142. Unresolved Asset Transfer Guard

同一个用户存在：

```text
NEEDS_REVIEW
or
economically ambiguous transfer
```

时：

新 Exchange / Rebalance 如果会触碰同一 Asset Position：

```text
Fail Closed
```

不相关业务可以继续。

API Request 是否继续取决于实际可用 NewAPI Active Quota，不因单个 Wallet Saga 异常自动冻结整个账号。

---

# 143. Poker Funding Boundary

Poker 与 Economy 都位于：

```text
chaldea_platform
```

因此 Buy-in / Top-up / Cash Out 不使用跨服务 Saga。

采用窄范围：

```text
Atomic Transaction Gateway / Stored Procedure Contract
```

目标：

```text
Poker Service
→ EXECUTE approved economy/poker funding contract
```

而不是直接获得任意 Economy Table DML。

---

# 144. Poker Buy-in Atomicity

Buy-in：

```text
BEGIN

validate funding/session/seat state
lock Available Chips wallet

debit Available Chips
insert wallet ledger

create / activate Poker Session
create table stack
insert Poker funding record

COMMIT
```

任何一步失败：

```text
ROLLBACK
```

不允许：

```text
wallet debit without stack
stack without wallet debit
```

语义：

```text
Available Chips -X
Poker In Play +X
Total Assets unchanged
```

---

# 145. Poker Top-up

只在合法 Hand Boundary 生效。

执行时重新验证 Wallet。

单 Local DB Transaction：

```text
lock wallet
→ debit chips
→ ledger
→ increase stack
→ funding record
→ COMMIT
```

余额不足：

```text
NO EFFECT
```

原 Stack 不变。

---

# 146. Poker Cash Out

同一 Local DB Transaction：

```text
lock Poker Session / Seat
lock Wallet

verify Safe Leave eligible
verify not already cashed out

final_stack = authoritative stack

set stack = 0
close funding position

credit Available Chips
insert wallet ledger

mark cash out confirmed
mark session settlement

COMMIT
```

重复：

```text
same cashout biz_id
→ original result
```

禁止 Partial Cash Out。

---

# 147. Poker Hand Economy Isolation

Hand 内部只移动：

```text
Table Stack
Committed This Hand
Pot / Side Pot
Winner Stack
```

主 Wallet 不因：

```text
Fold
Call
Raise
Win Pot
```

而逐次更新。

主 Wallet 只在：

```text
Buy-in
Top-up
Cash Out
```

发生资产边界移动。

---

# 148. Poker Zero-sum

V1 无 Rake。

Table / Session 资金必须守恒：

```text
Total successful Buy-in / Top-up
=
Current Poker In Play
+
Total successful Cash Out
```

Poker Hand 不：

```text
issue chips
burn chips
```

系统不能通过 Poker 产生或销毁用户筹码。

---

# 149. Direct Play Economy Boundary

Direct Play Paid Round 与 Wager Debit：

```text
same Chaldea DB transaction
```

示意：

```text
BEGIN
→ lock Chips Wallet
→ validate
→ create Round
→ debit Wager
→ ledger
→ COMMIT
```

Round Settlement：

```text
stable round_id / biz_id
→ exactly-once settlement
```

完整 Round State Machine 进入 TD-06 / TD-07。

---

# 150. Direct Play Issuance / Burn

玩家输：

```text
GAME_BURN
```

玩家赢：

```text
GAME_ISSUANCE
```

都关联：

```text
round_id
game_config_version
biz_id
```

Poker 不使用无限庄家 Issuance / Burn。

---

# 151. Reward / Grant Economy Boundary

TD-05 决定 Claim Eligibility / Claim State。

Economy 负责真正资产效果。

API Credit 类型：

```text
→ credit Reserve API Credit
```

如后续 Hourly / Relief 被确认是 Chips：

```text
→ credit Available Chips
```

TD-04 不决定尚未冻结的 Hourly / Relief Asset Type。

---

# 152. Migration Initial Grant Economy Linkage

业务 ID：

```text
initial_grant:migration:{migration_batch_id}:{newapi_user_id}
```

目标：

```text
Reserve API Credit
```

使用标准：

```text
Wallet + Ledger same transaction
```

Migration Script 不直接设置最终余额。

---

# 153. Admin Adjustment

仅通过正式 Economy Service / Contract。

正向 API Credit：

```text
credit Reserve
```

正向 / 负向 Chip：

```text
Available Chips only
```

普通 Adjustment 不直接修改：

```text
Poker In Play
```

Poker 资产异常走 Poker Settlement Repair。

---

# 154. Negative API Credit Adjustment

用户侧统一 API Credit：

```text
Reserve + Active
```

负向 Adjustment 采用：

```text
Reserve-first
+
Active-shortfall
```

Reserve 足够：

```text
local
```

否则：

```text
cross-DB adjustment transfer
```

Unified API Credit 不得变为负数。

---

# 155. Adjustment Safety

必须：

```text
Super Admin
Fresh Auth <= 10min
Reason
Reference
Before
Delta
After
Typed Confirmation
Operation ID
Ledger
Audit
```

由于 Economy / Audit 同库，要求：

```text
Balance
+
Ledger
+
Admin Operation
+
Audit Event
```

同一 DB Transaction。

Audit 失败则 Adjustment 回滚。

---

# 156. Asset Serialization

所有资产 API：

```text
amount
atomic_units
```

使用 Decimal String / Integer String。

示例：

```json
{
  "amount": "0.0372",
  "atomic_units": "18600"
}
```

禁止依赖 JavaScript JSON Number 表示大整数资产值。

---

# 157. Decimal Parsing

客户端传金额：

```text
string
```

服务端严格 Decimal → Atomic Unit。

```text
0.0372
→ 18600 units
```

```text
0.000002
→ 1 unit
```

```text
0.000001
→ not representable
→ reject
```

禁止：

```text
int(float64(amount) * 500000)
```

不做静默舍入。

---

# 158. Poker Integer Chip Constraint

Wallet 存储精度仍然是 Atomic Units。

Poker V1 只接受整数 Chip。

因此 Poker Funding / Blind / Bet / Pot / Stack / Cash Out 要求：

```text
amount_units % 500000 = 0
```

Wallet 中不足一枚整 Chip 的小数余额继续留在 Wallet，不进入 Poker。

---

# 159. Fresh Authoritative Asset Snapshot

以下高风险操作使用 Fresh Authoritative Revalidation：

- Relief Eligibility；
- Exchange Submit；
- Negative Adjustment；
- Poker Buy-in；
- 其他资产安全判断。

必须组合：

```text
NewAPI Active
Chaldea Wallets
Poker Authority
Processing Transfers
```

如果某 Authority：

```text
Unavailable
Too stale
Economically ambiguous
```

则：

```text
Fail Closed
```

不得把不可用数据当作 0。

---

# 160. Reconciliation Allowed Actions

Worker / Operator 可以：

```text
Retry same idempotent effect
Resume
Verify
Compensate
Mark for Review
```

不得：

```text
direct UPDATE final balance
direct force transfer CONFIRMED
delete failed transfer
```

Reconciliation 是恢复 State Machine，不是人工覆盖资产事实。

---

# 161. Crash Point Analysis

| Crash Point | Durable Fact | Recovery |
|---|---|---|
| Intent created, source not touched | `PENDING` | Retry same transfer |
| Local source debit committed, state not advanced | Ledger leg exists | Detect fact, advance state |
| NewAPI debit applied, response lost | Local uncertain / Target Journal durable | Query same operation ID |
| Source fully debited, target not credited | `SOURCE_DEBITED` + Processing | Resume target or compensate |
| Target applied, response lost | Target operation exists | Query then mark `TARGET_CREDITED` |
| Target complete, final state not written | Durable Target Fact | Mark `CONFIRMED` |
| Compensation applied, response lost | Compensation Fact exists | Query then `COMPENSATED` |
| Worker crashes | Durable State persists | Next Worker resumes |

Recovery 不依赖进程内存。

---

# 162. Economy Metrics

至少监控：

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

以下属于 Critical：

```text
negative wallet
ledger chain mismatch
duplicate economic effect
compensation impossible
```

---

# 163. Wallet Ledger Reconciliation

除 Cross-DB Worker 外，建立定期：

```text
Wallet Ledger Reconciliation
```

校验：

```text
materialized balance
vs
ledger chain / derived balance
```

发现不一致：

```text
Incident
→ Needs Attention
```

不得自动修改 Balance 或 Ledger 以掩盖异常。

---

# 164. TD-04 Test Gate

Implementation Spec 必须包含至少：

1. 100 并发相同 Biz ID，只产生一次资产效果；
2. 100 并发不同扣款，余额不得负；
3. API↔Chips 反向并发无死锁 / 超发；
4. Client Timeout + Retry 返回原 Transaction；
5. Target Applied + Response Lost 不重复 Target；
6. Source Debited + Target Unavailable 时 Processing 正确；
7. Compensation Response Lost 不重复补偿；
8. API Consumption 与 Active Debit 竞争，NewAPI Quota 不负；
9. Refill 与 API→Chips 并发保持 API 总资产守恒；
10. Poker 双设备 / 重复 Buy-in 只产生一次资产效果；
11. Duplicate Cash Out 只到账一次；
12. Poker Service Crash During Funding 不产生半状态；
13. Ledger Insert Failure 导致 Balance Mutation Rollback；
14. Worker Crash 后 Resume Same Transfer；
15. Redis Loss 不丢正式资产；
16. Ledger Reconciliation 能检测故意制造的不一致。

---

# 165. TD-04 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| TD-FRZ-074 | Economy 明确区分 Reserve API Credit、Active NewAPI Quota、Available Chips、Poker In Play 与 Processing Assets，各自 Authority 不混用。 | FROZEN |
| TD-FRZ-075 | `Total Assets` 使用统一 Atomic-unit 口径，由 Reserve + Active + Chips + Poker In Play + 未在其他已结算 Position 表示的 Processing Assets 构成。 | FROZEN |
| TD-FRZ-076 | Chaldea `wallet_balances` 只保存 Chaldea-owned Materialized Positions，V1 主要为 Reserve API Credit 与 Available Chips。 | FROZEN |
| TD-FRZ-077 | Wallet Balance 与对应 Ledger Entry 必须在同一 DB Transaction；`wallet_ledger` append-only，Runtime 禁止 UPDATE / DELETE。 | FROZEN |
| TD-FRZ-078 | Economy 使用 `asset_transactions` 聚合唯一 Business Operation；一个 Transaction 可以拥有多个 Ledger Legs。 | FROZEN |
| TD-FRZ-079 | `request_id / idempotency_key / biz_id / transaction_id` 四种身份严格分离。 | FROZEN |
| TD-FRZ-080 | Wallet 扣款使用 DB Row Lock 或 Atomic Conditional Update，并保持固定多 Wallet Lock Order；数据库同时强制非负余额。 | FROZEN |
| TD-FRZ-081 | Chips → API Credit 全部使用同一 Chaldea DB 原子事务，并统一 Credit 到 Reserve。 | FROZEN |
| TD-FRZ-082 | API → Chips 使用 Reserve-first / Active-shortfall；Reserve 足够时完全本地事务，只有 Shortfall 涉及 NewAPI。 | FROZEN |
| TD-FRZ-083 | Cross-DB Asset Movement 采用 Idempotent Saga；禁止 Naive Dual Write，也不采用 PostgreSQL 2PC 作为 V1 业务一致性方案。 | FROZEN |
| TD-FRZ-084 | Cross-DB Transfer 正式状态为 PENDING / SOURCE_DEBITING / SOURCE_DEBITED / TARGET_CREDITING / TARGET_CREDITED / CONFIRMED / COMPENSATING / COMPENSATED / FAILED_NO_EFFECT / NEEDS_REVIEW。 | FROZEN |
| TD-FRZ-085 | Cross-DB Transfer 使用独立 Legs 表达多来源 / 多目标 Effect；Processing Assets 从 Durable Leg Facts 推导，不维护易漂移的独立 Processing Balance。 | FROZEN |
| TD-FRZ-086 | 每一单位在 Transfer 中只能属于 Source / Processing / Target / Restored Source 中一个 Accounting Bucket，禁止重复计算。 | FROZEN |
| TD-FRZ-087 | 用户确认 Exchange 后不提供普通 Cancel；HTTP Timeout / Refresh / Disconnect 后必须查询原 Transfer，不盲目创建第二笔。 | FROZEN |
| TD-FRZ-088 | 任何 NewAPI Quota Mutation 必须具备 Target-local Idempotency；优先复用真实 NewAPI 能力，缺失时使用窄 Quota Bridge / Target-side Operation Journal。 | FROZEN |
| TD-FRZ-089 | Cross-DB 网络调用采用 At-least-once Attempts + Idempotent Effects，实现 Effectively Exactly-once Business Outcome；不得声称 Network Delivery 本身 exactly-once。 | FROZEN |
| TD-FRZ-090 | 外部 Effect 结果不确定时必须按 Operation ID 查询 Authority；在结果 Unknown 时既不盲目 Retry，也不盲目 Compensate。 | FROZEN |
| TD-FRZ-091 | TD-04-C01 采用 Bounded Active Buffer + Reactive Refill；Reserve → Active 自动化，用户不感知手动 Active Top-up。 | FROZEN |
| TD-FRZ-092 | Active Liquidity Low / Target / Max Watermark 属于内部版本化技术参数，精确值由 NewAPI 源码核验与负载测试确定。 | FROZEN |
| TD-FRZ-093 | Reserve → Active Refill 本身是 Cross-DB Saga，不改变用户总资产；异常期间使用 Processing Assets 保持资产可解释。 | FROZEN |
| TD-FRZ-094 | Cutover Reset 的 Active=0 是迁移验证点；Liquidity Controller 只在 Reset / Verification 完成后开始 Prime Active，不改变 TD-03 迁移事实。 | FROZEN |
| TD-FRZ-095 | NewAPI Active Quota 继续以 NewAPI 为 Source of Truth；Chaldea 可保存 Append-only External Asset Events / Bridge Journal Mirror，但不得建立第二个 Active Balance Authority。 | FROZEN |
| TD-FRZ-096 | Poker Buy-in / Top-up / Cash Out 不使用跨服务 Saga；因 Economy/Poker 同属 `chaldea_platform` Database，使用窄范围原子 Transaction Gateway / DB Procedure 同时修改 Wallet Ledger 与 Poker Funding State。 | FROZEN |
| TD-FRZ-097 | Poker Runtime 不取得任意 Economy DML 权限，只能 EXECUTE 被批准的 Funding Transaction Contract。 | FROZEN |
| TD-FRZ-098 | Poker Hand 内部只移动 Poker-owned Stack / Committed / Pot；主 Wallet 只在 Buy-in / Top-up / Cash Out 边界发生账变。 | FROZEN |
| TD-FRZ-099 | Poker V1 无 Rake，Hand 不发行或销毁资产；Table / Session 必须保持零和资金守恒。 | FROZEN |
| TD-FRZ-100 | Direct Play Paid Round 的 Wager Debit 与 Round Acceptance 必须在同一 Chaldea DB Transaction；Settlement 通过 `round_id` 幂等执行。 | FROZEN |
| TD-FRZ-101 | Direct Play 单人游戏输赢分别记录 GAME_BURN / GAME_ISSUANCE；Poker 不使用无限庄家 Issuance/Burn。 | FROZEN |
| TD-FRZ-102 | Reward / Initial Grant / Migration Grant 的真实资产写入统一由 Economy Boundary 完成；API Credit 进入 Reserve；Hourly / Relief 的 OPEN Asset Type 保持不决定。 | FROZEN |
| TD-FRZ-103 | Admin Adjustment 不直接编辑余额；API 正向调整进入 Reserve，Chip Adjustment 只影响 Available Chips，普通 Adjustment 永不直接修改 Poker In Play。 | FROZEN |
| TD-FRZ-104 | Negative Unified API Credit Adjustment 复用 Reserve-first / Active-shortfall 机制，并且 Total API Credit 不得变为负数。 | FROZEN |
| TD-FRZ-105 | Asset API 的 Atomic Units 与金额使用 Decimal String Contract，不把大整数作为 JavaScript JSON Number。 | FROZEN |
| TD-FRZ-106 | Exchange 使用严格 Decimal → Integer Atomic Unit Parser；无法精确对应 Atomic Unit 的金额直接拒绝，不静默舍入。 | FROZEN |
| TD-FRZ-107 | Poker 金额继续存 Atomic Units，但 V1 Poker 资金必须整 Chip，数据库 / 业务层保证金额是 500,000 Units 的整数倍。 | FROZEN |
| TD-FRZ-108 | Relief / Exchange / Negative Adjustment / Poker Buy-in 等高风险判断使用 Fresh Authoritative Asset Snapshot；Authority 不可用或经济状态不确定时 Fail Closed。 | FROZEN |
| TD-FRZ-109 | Reconciliation Worker 只根据 Durable Facts 执行合法 Retry / Resume / Compensation；使用持久化 Retry State、Backoff 与 `SKIP LOCKED`，不能自动改余额掩盖差异。 | FROZEN |
| TD-FRZ-110 | TD-04 Implementation 必须通过 Concurrency / Idempotency / Crash-point / Compensation / Redis-loss / Poker Funding Atomicity / Ledger Reconciliation 测试 Gate。 | FROZEN |

---

# 166. Change Log — WORKING v0.4

## Added

- 用户正式确认 TD-04；
- 冻结 `TD-FRZ-074 ～ TD-FRZ-110`；
- 冻结 Economy Asset Authority；
- 冻结 Total Assets Atomic Formula；
- 冻结 Materialized Wallet + Append-only Ledger；
- 冻结 Asset Transaction / Ledger Leg；
- 冻结 Wallet Row Lock / Non-negative Invariant；
- 冻结 Chips → API 本地原子事务；
- 冻结 API → Chips Reserve-first / Active-shortfall；
- 冻结 Cross-DB Saga；
- 冻结 Target-local Idempotency；
- 冻结 Transfer Leg / Processing Asset；
- 冻结 Effectively Exactly-once Business Semantics；
- 冻结 Unknown Result Query-first；
- 冻结 Reconciliation Worker；
- 冻结 `TD-04-C01 — Bounded Active Buffer + Reactive Refill`；
- 冻结 Reserve → Active Cross-DB Refill；
- 冻结 Poker Atomic Funding Gateway；
- 冻结 Poker Zero-sum；
- 冻结 Direct Play Economy Boundary；
- 冻结 Reward / Grant Economy Boundary；
- 冻结 Admin Adjustment；
- 冻结 Asset String Serialization / Decimal Parsing；
- 冻结 Fresh Authoritative Asset Snapshot；
- 冻结 Crash Point Analysis；
- 冻结 Economy Metrics / Ledger Reconciliation；
- 冻结 TD-04 Test Gate。

## Not Changed

本批没有改变：

- Hourly Reward 资产类型；
- Hourly Reward 时间口径；
- Relief Fund 资产类型；
- Relief Fund Active Poker 行为；
- Reward 固定数值；
- Game 数学；
- Poker 业务规则；
- NewAPI 实际代码事实；
- TD-03 Cutover Reset Policy。

---

# 167. 下一批 — TD-05

下一批正式进入：

> **TD-05 — Rewards / Initial Grant / Migration Grant**

计划完整冻结：

1. Reward Domain Ownership；
2. Reward Config / Version；
3. Registration Initial Grant；
4. Migration Initial Grant；
5. Daily Check-in；
6. Hourly Reward；
7. Relief Fund；
8. Claim Record；
9. Claim Biz ID；
10. Claim State Machine；
11. Eligibility Snapshot；
12. Server-authoritative Time；
13. `Asia/Shanghai` Daily Boundary；
14. Unique Constraints；
15. Multi-tab / Multi-device Concurrency；
16. Pending / Processing / Confirmed / Failed / Recovering；
17. Retry / Resume；
18. Economy Credit Linkage；
19. Claim UI State；
20. Reward Maintenance；
21. Operations；
22. Audit；
23. Metrics / Alerts；
24. Migration / Registration Initial Grant Separation；
25. OPEN / TBD Feature Flags；
26. Config Version Lock；
27. Relief Fresh Asset Snapshot；
28. Crash Point Analysis；
29. Reward Test Gate。

特别注意：

- `Initial Grant = 1000 API Credit` 已冻结；
- `Daily = 500 API Credit` 已冻结；
- `Hourly = 100` 数量固定，但资产类型 / 时间口径 / 累积 / Daily Limit 仍 OPEN；
- `Relief = 300` 数量固定，条件 `Total Assets < 10`、成功后滚动 4 小时已冻结，但资产类型 / Eligibility Accumulation / Active Poker 行为仍 OPEN；
- TD-05 不得擅自替用户决定这些 OPEN 项；
- 可以把 OPEN 项设计成版本化 Policy / Feature Flag，但生产默认值不能伪装成已经确认的产品规则。



---

# 168. TD-05 — Rewards / Initial Grant / Migration Grant

> 状态：`FROZEN`  
> 用户确认：`按上述方案通过`

## 168.1 TD-05 总体结论

本批正式冻结：

- Versioned Reward Policy；
- Durable Reward Claim；
- Registration / Migration Initial Grant；
- Daily Check-in；
- Hourly Reward 的多口径技术支持但保持产品 OPEN；
- Relief Fund Eligibility / Cooldown / Snapshot；
- Reward Claim State Machine；
- Reward Biz ID / Policy Version；
- Claim / Wallet / Ledger 同库原子提交；
- Server-authoritative Time；
- `Asia/Shanghai` Daily Boundary；
- Claim Cursor；
- Claim Unique Constraint；
- 多 Tab / 多设备 / Retry 幂等；
- Reward Recovery；
- Reward Maintenance 技术 Gate；
- Operations / Audit / Metrics；
- Registration / Migration Grant exactly-once；
- Reward Crash Point / Test Gate；
- OPEN / UNRESOLVED Policy 激活保护。

仍保持 OPEN：

```text
Hourly.asset_type
Hourly.window_mode
Hourly.accumulation
Hourly.daily_limit

Relief.asset_type
Relief.accumulation
Relief.active_poker_policy

Reward Maintenance 的最终产品权限与临时关闭规则
未来是否允许产品层版本化修改当前冻结金额
Reward Issuance Alert 的最终运营阈值
```

技术设计允许表达上述字段，但不得填入假默认产品答案。

---

# 169. Reward Kind

正式 Reward Kind：

```text
INITIAL_GRANT_REGISTRATION
INITIAL_GRANT_MIGRATION
DAILY
HOURLY
RELIEF
```

Initial Registration / Migration 属于同一产品家族，但：

- Trigger 不同；
- Biz ID 不同；
- Audit Source 不同；
- Claim Origin 不同；
- 不得互相替代。

管理员资产调整仍属于 Economy Adjustment，不伪装成 Reward Claim。

Game Payout 属于 Game Settlement，不属于非游戏 Reward Claim。

---

# 170. Reward Policy Version

使用：

```text
rewards.policy_versions
```

逻辑字段：

```text
policy_version_id UUIDv7
reward_kind

version_number
status

asset_type nullable
amount_units

business_timezone nullable

window_mode nullable
cooldown_seconds nullable
eligibility_threshold_units nullable

accumulation_mode nullable
daily_limit nullable
active_poker_policy nullable

effective_from
effective_until

created_at
validated_at
activated_at
retired_at

policy_hash
```

状态：

```text
DRAFT
→ VALIDATED
→ SCHEDULED
→ ACTIVE
→ RETIRED
```

Active Version Immutable。

历史 Claim 永远引用实际生效：

```text
policy_version_id
```

---

# 171. Product-locked Rule Boundary

以下当前属于：

```text
PRODUCT_LOCKED
```

而不是普通后台参数：

```text
Registration Initial Grant = 1000 API Credit
Migration Initial Grant    = 1000 API Credit
Daily                      = 500 API Credit
Hourly                     = quantity 100
Relief                     = quantity 300
Relief Threshold           = Total Assets < 10
Relief Cooldown            = 4 hours after successful claim
```

普通 Operations 不允许通过 JSON / DB Editor 修改。

Policy Version Framework 只表示：

> 未来如果经过新的产品需求确认，可以新增新版本，而不是改写历史。

---

# 172. OPEN Policy Activation Gate

如果 Policy 存在产品必填字段：

```text
UNRESOLVED
```

则不能进入 Production `ACTIVE`。

例如 Hourly：

```text
amount = 100                confirmed
asset_type                  unresolved
window_mode                 unresolved
accumulation                unresolved
daily_limit                 unresolved
```

允许：

```text
DRAFT / CONFIG_INCOMPLETE
```

禁止：

```text
ACTIVE
```

Relief 同理。

因此技术层明确区分：

```text
CONFIG_INCOMPLETE
```

与：

```text
MAINTENANCE / SERVICE_UNAVAILABLE
```

---

# 173. Durable Reward Claim

统一使用：

```text
rewards.claims
```

逻辑字段：

```text
claim_id UUIDv7

newapi_user_id
reward_kind
claim_origin

policy_version_id

biz_type
biz_id

period_key nullable

asset_type
amount_units

status

server_requested_at
confirmed_at

eligibility_snapshot_id nullable

economy_transaction_id nullable

failure_code nullable
failure_detail_safe nullable

created_at
updated_at
```

Claim Origin 至少：

```text
REGISTRATION
MIGRATION
USER_DASHBOARD
USER_REWARDS_CENTER
SYSTEM_RECOVERY
```

Initial Grant 即使不是 User-click Claim，也使用 Durable Claim Record 表达 Reward Entitlement 被履行。

---

# 174. Reward Claim State Machine

正式状态：

```text
PENDING
↓
ELIGIBILITY_VERIFIED
↓
ISSUING
↓
CONFIRMED
```

无资产影响终态：

```text
REJECTED_NO_EFFECT
```

异常恢复：

```text
PENDING
ELIGIBILITY_VERIFIED
ISSUING
→ RECOVERING
→ CONFIRMED
```

自动无法安全完成：

```text
→ NEEDS_REVIEW
```

---

# 175. Reward Issuance Atomicity

Reward 当前最终都 Credit Chaldea-owned Position：

```text
API Credit
→ Reserve API Credit

Entertainment Chips
→ Available Chips
```

因此 Reward Issue 不使用 Cross-DB Saga。

Claim / Economy / Ledger：

```text
BEGIN

validate claim uniqueness
validate policy
validate eligibility / period

ensure Claim

credit Wallet
insert Wallet Ledger
create / link Economy Transaction

set Claim = CONFIRMED

COMMIT
```

任意失败：

```text
ROLLBACK
```

不得出现：

```text
balance changed
but claim not confirmed
```

或：

```text
claim confirmed
but no ledger
```

---

# 176. Confirmed Claim Immutability

一旦：

```text
CONFIRMED
```

不得：

```text
UPDATE → FAILED
DELETE Claim
DELETE Ledger
```

发现错误：

```text
Economy Reversal / Adjustment
+
Incident
+
Audit
```

原 Claim 继续保留其历史事实。

---

# 177. Reward / Economy Linkage

Reward：

```text
claim_id
```

Economy：

```text
transaction_id
```

共享：

```text
biz_type
biz_id
```

形成：

```text
Reward Claim
↕
Economy Transaction
↕
Wallet Ledger
```

完整追踪链。

---

# 178. Registration Initial Grant

固定：

```text
1000 API Credit
→ Reserve API Credit
```

Biz ID：

```text
initial_grant:registration:{newapi_user_id}
```

除 Business ID Unique 外，再建立领域级：

```text
one INITIAL_GRANT_REGISTRATION per user
```

避免错误更换 Biz ID 后重复赠送。

---

# 179. Registration Grant Flow

```text
Discord Registration
→ NewAPI Account Created
→ stable newapi_user_id
→ ensure identity.account_refs
→ ensure Registration Grant Claim
→ Local Economy TX
     Reserve +1000
     Ledger
     Claim CONFIRMED
→ Completion Summary
```

如果 Account 创建成功但 Grant 暂未完成：

```text
Account remains
Claim = PENDING / RECOVERING
```

继续 Master Initialization。

不得：

```text
delete account
re-register
create another grant
```

---

# 180. Existing Binding Cannot Trigger Registration Grant

任何 Existing Bound Account：

```text
/register
OAuth retry
duplicate callback
refresh
new device
```

最终均：

```text
login existing account
```

不得再次创建：

```text
INITIAL_GRANT_REGISTRATION
```

---

# 181. Migration Initial Grant

固定：

```text
1000 API Credit
→ Reserve API Credit
```

Biz ID：

```text
initial_grant:migration:{migration_batch_id}:{newapi_user_id}
```

同时建立领域：

```text
one INITIAL_GRANT_MIGRATION per user
```

防止 accidental second migration batch 再发一笔。

如果未来产品确实决定新的迁移奖励，应创建新的 Reward Kind / Policy，而不是复用当前开服 Migration Initial Grant。

---

# 182. Migration Grant Flow

与 TD-03 一致：

```text
Cutover User
→ SNAPSHOTTED
→ RESET_VERIFIED
→ Ensure Migration Reward Claim
→ Local Economy TX
     Reserve +1000
     Ledger
     Claim CONFIRMED
→ Cutover User = GRANT_CONFIRMED
→ Post-cutover Verification
```

Migration Notice 不触发 Grant。

---

# 183. Migration Grant Blocking Gate

如果：

```text
RESET_VERIFIED
+
Grant not CONFIRMED
```

则：

```text
Cutover User
≠ VERIFIED
```

Batch：

```text
≠ READY_TO_OPEN
```

Grant Recovery 必须完成后继续。

---

# 184. Daily Business Period

Daily 固定使用：

```text
Asia/Shanghai Business Date
```

保存：

```text
business_date
period_start_at
period_end_at
```

数据库等价唯一：

```text
UNIQUE(
  newapi_user_id,
  reward_kind,
  business_date
)
```

---

# 185. Daily Biz ID

推荐：

```text
reward:daily:{newapi_user_id}:{YYYY-MM-DD}
```

日期只能由 Server 依据：

```text
Asia/Shanghai
```

生成。

客户端不能提交自定义 Claim Date。

---

# 186. Daily No Make-up Claim

V1 不支持：

```text
补签
Streak
七日大奖
连续签到倍率
```

如果某自然日已经结束：

```text
missed Daily
→ no cross-day make-up entitlement
```

Reward Maintenance 并不会自动产生补签。

---

# 187. Daily Concurrency

多 Tab / Device：

```text
Device A
Device B
Tab C
→ same Daily Claim Period
```

依赖 DB Unique Constraint。

只有一个 Claim 可成为该自然日正式资产事实。

其他请求：

```text
read existing Claim
→ return original status/result
```

不会重复 +500。

---

# 188. Policy Version Does Not Reset Daily

Daily Period Unique Key 不包含：

```text
policy_version_id
```

同一 Business Date 激活新 Policy：

```text
does not create second claim opportunity
```

Policy Change 不重置当前日领取次数。

---

# 189. Hourly Technology Model

当前固定：

```text
amount = 100
server-authoritative time
at most one successful claim per selected window
```

支持：

```text
NATURAL_HOUR
ROLLING_60_MINUTES
```

但：

```text
window_mode = UNRESOLVED
```

Production 不自行选择。

---

# 190. Future Hourly Natural Hour Mode

如果产品选择：

```text
NATURAL_HOUR
```

则使用：

```text
Asia/Shanghai hour_key
```

例如：

```text
2026-09-03T21
```

等价唯一：

```text
UNIQUE(user, HOURLY, hour_key)
```

Biz ID：

```text
reward:hourly:{user}:{hour_key}
```

---

# 191. Future Hourly Rolling Mode

如果产品选择：

```text
ROLLING_60_MINUTES
```

使用：

```text
rewards.claim_cursors
```

逻辑：

```text
newapi_user_id
reward_kind

last_successful_claim_at
next_claim_at

claim_sequence
version
```

Claim：

```text
BEGIN
→ lock cursor
→ if now < next_claim_at: cooldown
→ issue Reward
→ update successful time / next_claim_at
→ COMMIT
```

---

# 192. Hourly Accumulation Capability

预留：

```text
rewards.entitlements
```

用于未来可能的 Accumulation。

逻辑：

```text
entitlement_id
user
reward_kind
window_key
policy_version_id

status
consumed_by_claim_id
created_at
expires_at nullable
```

当前不启用任何：

```text
accumulated hourly entitlement
```

语义。

---

# 193. Hourly Daily Limit Semantics

当前：

```text
daily_limit = UNRESOLVED
```

不得使用：

```text
NULL = Unlimited
```

这种错误语义。

产品如果未来明确：

```text
no daily limit
```

才设置：

```text
limit_mode = NONE
```

否则配置具体 Limit。

---

# 194. Relief Threshold

固定：

```text
Total Assets < 10
```

当前 Atomic Unit：

```text
total_asset_atomic_units < 5,000,000
```

```text
9.999998
→ eligible

10
→ ineligible
```

Total Assets 必须复用 TD-04 完整资产 Authority。

---

# 195. Relief Eligibility Snapshot

使用：

```text
rewards.eligibility_snapshots
```

逻辑保存：

```text
eligibility_snapshot_id

newapi_user_id
reward_kind

server_time

reserve_units
active_quota_units
available_chip_units
poker_in_play_units
processing_units

total_asset_units

last_successful_claim_at
next_claim_at

active_poker_session_present

source_observed_at
source_freshness

policy_version_id

eligible
reason_code
```

用于记录 Claim 当时服务端真实判断依据。

---

# 196. Relief Fresh Authority

Claim 必须读取：

```text
Reserve
Active NewAPI Quota
Available Chips
Poker In Play
Processing Assets
```

如果任一 Required Authority：

```text
Unavailable
Too stale
Economically ambiguous
```

返回：

```text
ELIGIBILITY_TEMPORARILY_UNAVAILABLE
```

Fail Closed。

不得：

```text
unknown = 0
```

---

# 197. Relief Claim Cursor

`rewards.claim_cursors`：

```text
user + RELIEF
```

保存：

```text
last_successful_claim_at
next_claim_at
```

只有：

```text
CONFIRMED
```

Claim 才更新：

```text
next_claim_at = confirmed_at + 4 hours
```

---

# 198. Relief Claim Flow

```text
Client
→ Claim Relief
→ serialize per-user Relief Claim
→ read Fresh Asset Snapshot
→ validate:
     operational gate
     Total Assets < 10
     cooldown expired
     no conflicting same reward claim
     Active Poker Policy ← OPEN
→ BEGIN
     lock Relief Cursor
     revalidate local claim state
     insert Claim
     save Eligibility Snapshot
     credit configured Reward Asset
     insert Ledger
     set Claim CONFIRMED
     update last_successful_claim_at
     update next_claim_at = confirmed_at + 4h
  COMMIT
→ Return Result
```

---

# 199. Relief Cooldown Only on Successful Issuance

以下不推进 Cooldown：

```text
ineligible
request validation failure
maintenance rejection
DB rollback
asset issuance not committed
claim compensated to no-success fact
```

只有：

```text
CONFIRMED asset issuance
```

创建新的 4h 冷却起点。

---

# 200. Relief Concurrency

多个并发 Claim：

```text
cursor row lock
+
claim uniqueness
+
wallet atomic issuance
```

保证最终只一笔成功。

后续并发请求读取：

```text
last_successful_claim_at
next_claim_at
```

并返回 Cooldown / Existing Claim 状态。

---

# 201. Relief Active Poker Rule Remains OPEN

Eligibility Snapshot 保存：

```text
active_poker_session_present
```

Policy 可以表达：

```text
ALLOW
DENY
```

但当前：

```text
UNRESOLVED
```

不得自行选择。

---

# 202. Relief Accumulation Remains OPEN

Policy 可以表达：

```text
eligibility_accumulation_mode
```

但当前：

```text
UNRESOLVED
```

不得解释成：

```text
multiple accumulated 300 claims
```

---

# 203. Claim Origin Is Not Claim Authority

Dashboard 与 Rewards Center：

```text
claim_origin
```

不同。

但：

```text
same Reward Period
same Cursor
same Claim Authority
```

不能在两个入口分别领取一次。

---

# 204. Policy Change / History

Claim 固定快照：

```text
policy_version_id
asset_type
amount_units
```

新 Policy：

```text
only affects future Claims after effective time
```

历史 Claim 不重算。

Policy 激活默认不重置：

```text
Daily Period
Hourly Cursor
Relief Cursor
```

除非未来产品明确要求新的资格重置语义。

---

# 205. Reward Maintenance Capability

技术层支持：

```text
AVAILABLE
CLAIMS_PAUSED
```

作为 Reward Claim Operational Gate。

但以下继续 OPEN：

- 谁能暂停；
- 哪些 Reward 可暂停；
- 是否需要公告；
- 临时关闭最终产品策略。

相关权限在 TD-10 收口。

---

# 206. Existing Claim During Maintenance

Maintenance 不删除已有：

```text
PENDING
ELIGIBILITY_VERIFIED
ISSUING
RECOVERING
```

Claim。

系统仍必须恢复原 Claim 至合法终态：

```text
CONFIRMED
REJECTED_NO_EFFECT
NEEDS_REVIEW
```

---

# 207. Initial Grant Entitlement vs Maintenance

Registration Initial Grant：

```text
maintenance / transient failure
→ Claim pending / recovering
```

不得永久跳过。

Migration Initial Grant：

```text
not confirmed
→ Cutover cannot finish
```

Maintenance 只影响执行时机，不改变应得权益。

---

# 208. Daily During Maintenance

如果 Daily Period 尚未结束：

```text
maintenance ends
→ normal same-day claim still possible
```

自然日结束后：

```text
no make-up Daily
```

保持 V1 无补签规则。

---

# 209. Hourly During Maintenance

最终行为取决于仍 OPEN：

```text
Hourly Accumulation Policy
```

技术模型已预留 Entitlement，但当前不自行决定遗漏窗口是否积累。

---

# 210. Server Time Contract

Reward Status API 返回：

```text
server_time
business_timezone

daily.business_date
daily.status

hourly.status
hourly.next_claim_at nullable

relief.status
relief.next_claim_at
```

Client 倒计时可以根据：

```text
server_time offset
```

显示。

但 Claim Submit 永远由 Server 重新验证。

---

# 211. Reward History

展示：

```text
Reward Type
Asset Type
Amount
Claim Time
Status
Business ID
Balance After
```

Initial Grant 必须区分：

```text
Registration Initial Grant
Migration Initial Grant
```

不能合并成无法审计来源的单一 “Reward +1000”。

---

# 212. Operations Claim Detail

至少可查看：

```text
claim_id
user
reward_kind
policy_version
biz_id
status

eligibility_snapshot
asset
amount

economy_transaction
ledger reference

recovery state
created_at
confirmed_at
failure_code
```

管理员不能直接：

```text
FAILED / REJECTED
→ SUCCESS
```

---

# 213. Reward Retry / Recovery

安全 Retry 必须继续：

```text
same claim_id
same biz_id
same policy_version
same asset
same amount
```

Worker：

```text
read Claim
→ read Economy Transaction / Ledger
→ derive durable fact
→ continue legal state
```

Ledger 已存在且 Claim 对应事务已 Commit：

```text
return / converge to CONFIRMED
```

不得再次发放。

---

# 214. Manual Compensation

人工补发不创建假的：

```text
Daily SUCCESS
Relief SUCCESS
```

进入：

```text
Economy Adjustment
```

并关联：

```text
original claim_id
incident / reference
```

统计必须区分正常 Reward Issuance 与 Manual Adjustment。

---

# 215. Reward Rate Limit

初始技术安全基线：

```text
Daily Claim        10 / min / user
Hourly Claim       10 / min / user
Relief Claim        6 / min / user
Claim Status Query 60 / min / user
```

精确值可以在 Implementation Spec 调优。

防重复的核心不是 Rate Limit，而是：

```text
DB Unique
Claim Cursor Lock
Biz ID
Atomic Economy Transaction
```

---

# 216. Reward Metrics

至少采集：

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

运营 Alert Threshold 当前继续 OPEN。

---

# 217. Reward Audit

至少：

```text
REWARD_POLICY_CREATED
REWARD_POLICY_VALIDATED
REWARD_POLICY_ACTIVATED
REWARD_POLICY_RETIRED

REWARD_CLAIM_CREATED
REWARD_CLAIM_REJECTED
REWARD_CLAIM_CONFIRMED
REWARD_CLAIM_RECOVERY_STARTED
REWARD_CLAIM_NEEDS_REVIEW

INITIAL_GRANT_REGISTRATION
INITIAL_GRANT_MIGRATION
```

Audit 不记录敏感 Secret。

---

# 218. TD-05 Crash Point Analysis

| Crash Point | Durable Fact | Recovery |
|---|---|---|
| Request 到达、Claim 未创建 | No effect | Client retry |
| Claim created, pre-eligibility crash | `PENDING` | Resume same Claim |
| Eligibility verified, pre-wallet crash | Claim durable, no Ledger | Resume issuance |
| Wallet/Ledger/Claim commit before crash | Transaction atomic | Read final fact |
| DB COMMIT success, response lost | `CONFIRMED` + Ledger | Query same Claim/Biz ID |
| Registration Account created, Grant pending | Existing account + pending Claim | Resume Grant |
| Migration Reset verified, Grant pending | Cutover incomplete | Resume Grant; no open |
| Relief Snapshot becomes stale | Old snapshot not trusted for issue | Recalculate |
| Relief Wallet issue rolls back | No Confirmed Claim | Cooldown unchanged |
| Confirmed Claim later disputed | Immutable historical success | Adjustment / Incident |

---

# 219. Reward Exactly-once

Reward Asset Target 当前全部属于 Chaldea DB，因此：

> Reward Business Effect 使用 DB Unique + Wallet Lock + Claim/Wallet/Ledger same transaction，形成 Exactly-once Committed Effect。

网络请求本身仍可以 at-least-once。

Duplicate Request：

```text
same Claim / Biz ID
→ same final result
```

---

# 220. TD-05 Test Gate

Implementation 必须覆盖：

## Registration Initial Grant

```text
duplicate OAuth callback
multiple tabs
refresh
network timeout
service restart
```

最终：

```text
1 Account
1 Claim
1 Economy Transaction
1 Wallet Credit
1 Ledger Effect
```

## Migration Grant

```text
same batch rerun
worker crash
response lost
duplicate resume
accidental second batch
```

同一用户仍：

```text
1 Migration Initial Grant
```

## Daily

100 并发请求同一 User / Shanghai Date：

```text
1 Claim
1 × 500 API Credit
```

测试：

```text
23:59:59
00:00:00
```

日期边界。

## Relief

```text
9.999998 → eligible
10       → ineligible
>10      → ineligible
```

并测试不同资产组合计算相同 Total。

50 并发 Claim：

```text
1 successful 300 reward
```

## Cooldown

成功：

```text
09:37
```

则：

```text
13:36:59 denied
13:37:00 cooldown satisfied
```

仍需重新满足：

```text
Total Assets < 10
```

Maintenance / rejected / rollback：

```text
cooldown unchanged
```

---

# 221. TD-05 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| TD-FRZ-111 | Reward Domain 使用 Versioned Policy + Durable Claim + Economy Atomic Issuance；Initial Registration / Migration / Daily / Hourly / Relief 是独立 Reward Kind。 | FROZEN |
| TD-FRZ-112 | 所有 Reward Claim / Grant 必须具有 stable `claim_id`、`biz_id`、Policy Version、资产快照、状态与 Economy Transaction Reference。 | FROZEN |
| TD-FRZ-113 | Reward Policy 使用 Draft → Validated → Scheduled → Active → Retired，Active Version Immutable；历史 Claim 永远引用实际 Policy Version。 | FROZEN |
| TD-FRZ-114 | 当前冻结的 1000 / 500 / 100 / 300、Relief `<10` 与 4h 等规则属于 Product-locked，不因存在配置表而成为普通运营可编辑字段。 | FROZEN |
| TD-FRZ-115 | Hourly / Relief 未确认字段保持 `UNRESOLVED`；Policy Validator 禁止含 Required OPEN Field 的版本进入 Production ACTIVE，不使用假默认值。 | FROZEN |
| TD-FRZ-116 | Reward Claim 状态采用 PENDING / ELIGIBILITY_VERIFIED / ISSUING / CONFIRMED / REJECTED_NO_EFFECT / RECOVERING / NEEDS_REVIEW。 | FROZEN |
| TD-FRZ-117 | Reward 资产 Effect、Wallet Balance、Ledger 与 Claim `CONFIRMED` 必须在同一个 Chaldea PostgreSQL Transaction 内提交。 | FROZEN |
| TD-FRZ-118 | 已 CONFIRMED Reward Claim 不得改写为失败或删除；错误修正通过 Economy Adjustment / Reversal + Audit。 | FROZEN |
| TD-FRZ-119 | Registration Initial Grant 固定 1000 API Credit → Reserve，使用稳定 Registration Biz ID，并有每用户一次的领域唯一约束。 | FROZEN |
| TD-FRZ-120 | 已绑定 Existing Account 的 Registration/OAuth Flow 永远不得重新触发 Registration Initial Grant。 | FROZEN |
| TD-FRZ-121 | Account 已创建但 Registration Grant 未完成时保留账号并恢复原 Claim；不得删号或重新注册。 | FROZEN |
| TD-FRZ-122 | Migration Initial Grant 固定 1000 API Credit → Reserve，使用 Batch-aware Biz ID，同时以每用户一次的领域唯一约束防止跨 Batch 误重复发放。 | FROZEN |
| TD-FRZ-123 | Migration Grant 只有在 Cutover User `RESET_VERIFIED` 后执行；Grant 未 CONFIRMED 时 Batch 不得完成 Post-cutover Verification / READY_TO_OPEN。 | FROZEN |
| TD-FRZ-124 | Daily 使用服务端 `Asia/Shanghai` Business Date，并建立 `(user, DAILY, business_date)` 等价 DB Unique Constraint。 | FROZEN |
| TD-FRZ-125 | Daily Biz ID 基于服务端 Business Date；客户端不能选择 Claim Date，V1 不支持跨日补签。 | FROZEN |
| TD-FRZ-126 | Policy Version 切换不重置 Daily / Hourly / Relief Claim Window 或 Cooldown；周期唯一性和 Claim Cursor 不包含 Policy Version。 | FROZEN |
| TD-FRZ-127 | Hourly 技术模型同时支持 NATURAL_HOUR 与 ROLLING_60_MINUTES，但当前 `window_mode` 保持 OPEN、不可自行激活。 | FROZEN |
| TD-FRZ-128 | 若未来选 Natural Hour，使用服务端 Hour Key + DB Unique Constraint；若选 Rolling 60min，使用 Locked Claim Cursor / `next_claim_at`。 | FROZEN |
| TD-FRZ-129 | `reward_entitlements` 预留 Hourly Accumulation 能力，但在产品确认前不启用、不推断累积行为。 | FROZEN |
| TD-FRZ-130 | Hourly `daily_limit=NULL` 表示 UNRESOLVED，不表示 Unlimited；只有产品明确后才能配置 NONE 或具体 Limit。 | FROZEN |
| TD-FRZ-131 | Relief Eligibility 必须按 TD-04 Total Assets 完整口径计算，Atomic Threshold 固定为 `< 5,000,000 units`。 | FROZEN |
| TD-FRZ-132 | Relief Claim 必须保存 Fresh Authoritative Eligibility Snapshot；任何 Required Authority 不可用或经济事实不明确时 Fail Closed。 | FROZEN |
| TD-FRZ-133 | Relief 使用 per-user Claim Cursor 保存 `last_successful_claim_at / next_claim_at`；只有正式 CONFIRMED Issuance 才推进 4h Cooldown。 | FROZEN |
| TD-FRZ-134 | Relief Claim 提交时必须重新计算 Total Assets、Cooldown 与 Policy，并通过 DB Serialization 防止多 Tab / 多设备重复领取。 | FROZEN |
| TD-FRZ-135 | Relief Asset Type、Accumulation、Active Poker Policy 保持 OPEN；Schema / Policy 可表达这些字段，但 Production 不填假答案。 | FROZEN |
| TD-FRZ-136 | Dashboard 与 Rewards Center 只是不同 `claim_origin`；同一个 Reward Period / Cursor 共用同一 Claim Authority，不形成两个领取渠道。 | FROZEN |
| TD-FRZ-137 | Reward Policy 新版本只影响生效后的新 Claim，不修改历史 Claim；版本激活默认不重置 Cursor / Cooldown。 | FROZEN |
| TD-FRZ-138 | Reward 系统支持 AVAILABLE / CLAIMS_PAUSED 等技术 Operational Gate，但最终 Maintenance 业务权限和临时关闭规则继续 OPEN，不在 TD-05 擅自补全。 | FROZEN |
| TD-FRZ-139 | Maintenance 不删除或伪造已有 Claim；已接受 Claim 必须恢复到合法终态。Initial Grant 权益不得因维护被永久跳过。 | FROZEN |
| TD-FRZ-140 | Reward Status API 返回 Server Time / Business Time 与服务端计算的 Eligibility / Next Claim 状态；客户端倒计时不是 Authority。 | FROZEN |
| TD-FRZ-141 | Admin 不得把 Failed / Rejected Claim 改成 SUCCESS；人工补发通过 Economy Adjustment，并关联原 Claim / Incident。 | FROZEN |
| TD-FRZ-142 | Reward Recovery 永远复用原 `claim_id / biz_id / policy_version`；HTTP Timeout 先查询原 Claim，不能创建第二份奖励。 | FROZEN |
| TD-FRZ-143 | Reward Issuance Analytics / Metrics 全量采集，但产品运营 Alert Threshold 当前继续 OPEN，不写死伪阈值。 | FROZEN |
| TD-FRZ-144 | Reward Audit 记录 Policy / Claim / Grant / Recovery 生命周期，且不记录密码、API Key Secret、OAuth Secret 等敏感凭证。 | FROZEN |
| TD-FRZ-145 | TD-05 Implementation 必须通过 Registration / Migration Exactly-once、Daily Timezone / Concurrency、Relief Threshold / Cooldown / Concurrency、Maintenance Recovery、DB Crash / Response-loss 等测试 Gate。 | FROZEN |

---

# 222. Change Log — WORKING v0.5

## Added

- 用户正式确认 TD-05；
- 冻结 `TD-FRZ-111 ～ TD-FRZ-145`；
- 冻结 Versioned Reward Policy；
- 冻结 Product-locked Rule Boundary；
- 冻结 OPEN / UNRESOLVED Policy Activation Gate；
- 冻结 Durable Reward Claim；
- 冻结 Reward Claim State Machine；
- 冻结 Claim + Wallet + Ledger Atomic Issuance；
- 冻结 Registration Initial Grant exactly-once；
- 冻结 Migration Initial Grant exactly-once；
- 冻结 Daily `Asia/Shanghai` Natural Day Unique Period；
- 冻结 Hourly Natural / Rolling 双技术能力但产品字段继续 OPEN；
- 冻结 Reward Entitlement Extension；
- 冻结 Relief Total Assets / Eligibility Snapshot；
- 冻结 Relief 4h Cursor；
- 冻结 Relief Fail Closed；
- 冻结 Reward Maintenance Technical Gate；
- 冻结 Reward Retry / Recovery；
- 冻结 Reward Operations / Audit / Metrics；
- 冻结 TD-05 Crash Analysis / Test Gate。

## Not Changed

本批没有改变：

```text
Hourly Asset Type
Hourly Window Mode
Hourly Accumulation
Hourly Daily Limit

Relief Asset Type
Relief Accumulation
Relief Active Poker Rule

Reward Maintenance Final Product Rule
Product-level Future Amount Changes
Operational Alert Threshold
```

上述仍为 OPEN。

---

# 223. 下一批 — TD-06

下一批正式进入：

> **TD-06 — Game Platform / Registry / Config / Round Engine**

计划冻结：

1. Game Registry Ownership；
2. Game Capability Model；
3. Direct Play vs Lobby / Realtime Game Type；
4. V1 五款 Direct Play 与未来扩展边界；
5. Game Metadata；
6. Publication / Runtime State；
7. Config Version；
8. Draft → Validate → Preview → Activate；
9. Active Config Immutability；
10. Policy Version Lock；
11. Direct Play Common Contract；
12. Round ID / Biz ID；
13. Wager Acceptance；
14. Round State Machine；
15. Wager + Round Atomicity；
16. Settlement exactly-once；
17. Refund；
18. Round Recovery；
19. Maintenance；
20. Client Retry；
21. Provably Fair Common Architecture；
22. Server Seed Commitment；
23. Client Seed；
24. Nonce / Round Sequence；
25. History / Verification Contract；
26. Runtime Capability；
27. Future 斗地主 / 麻将 / 炸金花 等扩展模型；
28. Operations Boundary；
29. Audit；
30. Metrics / Test Gate。

注意：

- TD-06 设计公共 Game Platform，不重新设计 Dice / Scratch / Summon / Slot / Blackjack 数学；
- 五个 V1 Direct Play 的专有状态机进入 TD-07；
- Poker 已是独立 Service，不强塞进 Direct Play Round Engine；
- Game Registry 不是 No-code Game Generator；
- 所有 Direct Play Round 必须继承 TD-04 的 Atomic Wager / Ledger / Settlement 不变量。



---

# 224. TD-06 — Game Platform / Registry / Config / Round Engine

> 状态：`FROZEN`  
> 用户确认：`按上述方案通过`

## 224.1 TD-06 总体结论

本批正式冻结：

- Dynamic Game Registry；
- Stable `game_slug`；
- Code-owned Runtime Manifest；
- Capability Model；
- Direct Play / Lobby Adapter 边界；
- Publication / Runtime 双状态；
- Game Config Versioning；
- Config Schema / Validator；
- Global Direct Play Wager Policy；
- Round Creation / Idempotency；
- Round Lifecycle；
- Multi-action Round；
- Settlement / Refund Exactly-once；
- Recovery / Maintenance Race；
- Provably Fair Commitment；
- Server Seed / Client Seed / Nonce；
- Versioned Deterministic Random Stream；
- Bias-free Mapping；
- History / Fairness Verification；
- Runtime / Operations；
- Crash Point / Test Gate。

本批不重新设计：

- Dice 数学；
- Scratch 数学；
- Summon / Gacha 数学；
- Slot Reel / Paytable 数学；
- Blackjack 详细 Hand / Action / Shuffle State；
- Poker Realtime / Side Pot / Settlement；
- 斗地主 / 麻将 / 炸金花具体规则和服务形态。

上述分别进入 TD-07 / TD-08 或未来需求版本。

---

# 225. Game Platform Topology

采用：

```text
Chaldea Platform Backend
│
├── Game Registry
│   ├── Catalog Metadata
│   ├── Publication State
│   ├── Runtime State
│   ├── Capability Manifest
│   └── Runtime Adapter Reference
│
├── Direct Play Platform
│   ├── Wager Policy
│   ├── Config Versioning
│   ├── Round Engine
│   ├── Action Engine
│   ├── Recovery
│   ├── Settlement / Refund
│   └── Provably Fair
│
└── Lobby / Realtime Adapters
    ├── Poker → independent Poker Service
    └── Future Lobby Games
```

V1 Direct Play：

```text
dice
scratch
summon
slot
blackjack
```

这些只是首发注册实现，不构成永久固定枚举。

---

# 226. Stable Game Identity

每款游戏拥有稳定：

```text
game_slug
```

用于：

- Deep Link；
- Registry；
- Game History；
- Round / Session；
- Config；
- Rankings；
- Audit；
- Operations。

Display Name、FGO 副标题、图片、主题包装可以变化，但不得改变稳定 `game_slug`。

---

# 227. Registry vs Runtime Implementation

Registry 只描述：

```text
what game exists
how it is published
how it is entered
what capabilities it declares
what implementation it maps to
```

Registry 不保存可任意执行的 Runtime Code。

禁止：

```text
DB Script
→ eval / dynamic execute
→ becomes a game
```

采用：

```text
game_registry.implementation_key
→ Code-owned registered implementation
```

只有随正式 Application Build 部署并注册的 Implementation 可以运行。

---

# 228. Runtime Definition

每个 Direct Play Implementation 注册：

```text
implementation_key

supported_config_schema_versions
supported_algorithm_versions

interaction_capabilities

randomness_requirement
fairness_implementation

config_validator
round_resolver
action_handler optional
recovery_handler

result_serializer
history_serializer
```

DB Registry 不替代代码实现。

---

# 229. Capability Model

至少支持组合：

```text
interaction.instant_resolve
interaction.reveal_sequence
interaction.multi_action

runtime.direct_play
runtime.lobby
runtime.resume

history.round
history.session
history.hand

fairness.random_required

presentation.skippable
```

Capability 为组合能力，不是永久固定 Game Type。

---

# 230. Entry Mode / Effective Entry Action

技术分离：

## Base Entry Mode

```text
DIRECT_PLAY
LOBBY
```

## Effective Entry Action

由运行状态 / 当前用户 Session 动态决定：

```text
PLAY
OPEN_LOBBY
RESUME
MAINTENANCE
TEMP_UNAVAILABLE
COMING_SOON
RETIRED
```

例如 Blackjack 本质：

```text
DIRECT_PLAY
```

但用户有 Active Round 时：

```text
Effective Entry = RESUME
```

---

# 231. Publication / Runtime State

Publication：

```text
DRAFT
PUBLISHED
```

Runtime：

```text
AVAILABLE
MAINTENANCE
TEMPORARILY_UNAVAILABLE
COMING_SOON
RETIRED
```

两者独立。

系统再计算：

```text
effective_runtime_state
```

综合：

- configured runtime state；
- implementation availability；
- global maintenance；
- dependent service health。

如果 Registry 标记 AVAILABLE 但 Implementation 不存在：

```text
effective_runtime_state = TEMPORARILY_UNAVAILABLE
```

不得运行未知实现。

---

# 232. Game Registry Data Boundary

`games.game_registry` 至少逻辑包含：

```text
game_slug
implementation_key

base_entry_mode

publication_state
configured_runtime_state

display_name
short_description
logo/media reference

mode
category/tags

recommendation_order
is_featured

capability_manifest_version

created_at
updated_at
```

Category / Tag / Mode 为动态目录元数据，不写死永久词表。

---

# 233. Lobby / External Adapter

未来大厅型游戏通过：

```text
runtime_adapter_key
```

引用正式代码注册 Adapter。

Poker 当前：

```text
entry = /poker
```

未来 Lobby Route / Adapter 必须由代码注册 / allowlist。

禁止后台录入任意外部 URL 作为游戏入口。

---

# 234. Direct Play Global Wager Policy

当前 V1 固定：

```text
Minimum Base Wager = 10 Chips
Product Fixed Maximum = NONE
Quick Amounts = 10 / 100 / 500 / 1000
Input Step = 1 whole Chip
```

建立：

```text
games.direct_play_wager_policy_versions
```

每个 Round 锁定：

```text
wager_policy_version_id
wager_policy_hash
```

当前策略为 Product-locked。

单个 Game Config 不允许覆盖：

```text
minimum base wager
global fixed max
quick amounts
```

---

# 235. Game-specific Derived Cost

全局 Wager 只表示 Base Wager。

Game Implementation 可以按已冻结规则派生：

```text
Summon
→ Base Wager × draw count

Slot
→ Total Wager → paylines

Blackjack
→ Initial Wager + derived Double / Split stake
```

这些属于 Game-specific Logic / Config。

不能用它绕过 Global Minimum / Base Policy。

---

# 236. Game Config Version

使用：

```text
games.game_config_versions
```

逻辑字段：

```text
config_version_id UUIDv7
game_slug

version_number
parent_version_id

status

config_schema_version
algorithm_version

config_payload JSONB
config_hash

created_by
created_at

validated_at
activated_at
superseded_at
```

`config_payload` 允许按游戏不同而结构不同，但必须经过：

```text
Code-owned Config Schema
+
Game-specific Validator
```

不能把 JSONB 当作任意规则引擎。

---

# 237. Game Config Lifecycle

正式：

```text
ACTIVE
→ Clone
→ DRAFT
→ VALIDATED
→ PREVIEWED
→ ACTIVE NEW VERSION
```

旧 Active：

```text
→ SUPERSEDED / HISTORICAL
```

历史引用版本 Immutable。

恢复旧配置：

```text
Clone historical
→ Validate
→ Activate as new version
```

不回写旧版本。

---

# 238. Config Hash

每个有效版本保存：

```text
game_config_hash
=
SHA-256(canonical validated config representation)
```

Active 后：

```text
config payload
config hash
schema version
algorithm version
```

全部不可变。

Round Creation 固定保存：

```text
game_config_version_id
game_config_hash
```

结算不能重新读取“当前 Active Config”代替历史锁定版本。

---

# 239. Config Activation Atomicity

Activation：

```text
BEGIN

lock game/config active pointer

verify:
    candidate valid
    implementation exists
    config schema supported
    algorithm supported
    Product Locks satisfied
    Fairness requirements satisfied

old active → SUPERSEDED
candidate → ACTIVE
registry.active_config_id → candidate

Audit

COMMIT
```

Round Creation 与 Activation 对同一 Active Pointer 建立有序并发。

一个 Round 要么完整引用旧版本，要么完整引用新版本。

---

# 240. Economic Config Permission Boundary

会影响以下参数的版本激活：

```text
Probability
Odds
Paytable
Prize Mapping
RTP
Random Mapping
Economic Game Rule
```

继续属于：

```text
Super Admin only
```

普通 Games Scope Operator 可以管理：

```text
Metadata
Catalog
Recommendation
Safe Runtime Operations
```

精确 RBAC 在 TD-10 收口。

---

# 241. V1 Free Round Boundary

虽然 Config Framework 可以未来表达：

```text
free_round
free_summon
```

但 V1 五款 Direct Play 当前禁止生产激活：

```text
free_round_enabled = true
```

除非未来正式需求版本确认。

---

# 242. Round Command vs Durable Round

用户态：

```text
READY
→ SUBMITTING
→ BET_ACCEPTED
```

技术：

```text
READY
= UI / pre-round state

SUBMITTING
= command / idempotency request state

BET_ACCEPTED
= first durable paid-round state
```

只有 `BET_ACCEPTED` 原子事务成功，才存在有效付费 Round。

---

# 243. Round Create Idempotency

Client 创建 Round 使用：

```text
Idempotency-Key
```

服务端保存：

```text
newapi_user_id
game_slug
idempotency_key
normalized_request_hash
round_id
```

等价唯一：

```text
UNIQUE(user, game_slug, idempotency_key)
```

行为：

```text
same key + same payload
→ original Round

same key + different payload
→ 409 IDEMPOTENCY_CONFLICT
```

---

# 244. Active Round Uniqueness

同一用户同一 Direct Play `game_slug`：

```text
at most one non-terminal Round
```

用户重新进入游戏时优先：

```text
Resume existing Round
```

而不是创建新 Round。

不同 Direct Play 游戏之间可以拥有各自独立 Active Round。

---

# 245. BET_ACCEPTED Atomic Transaction

Round Acceptance：

```text
BEGIN

lock runtime gate
lock active config pointer
lock active wager policy
lock wallet
lock/consume fairness commitment

validate:
  publication/runtime
  implementation
  no same-game active round
  wager
  balance
  config
  fairness commitment

insert Round
state = BET_ACCEPTED

debit wager
insert Economy Transaction
insert Wallet Ledger

consume Fairness Commitment

COMMIT
```

因此：

```text
BET_ACCEPTED
↔
Initial Wager Debit
```

保持同一原子事实。

---

# 246. Round Core Data

建议：

```text
round_id UUIDv7

newapi_user_id
game_slug
implementation_key

lifecycle_state
recovery_state
round_version

interaction_profile

game_config_version_id
game_config_hash

wager_policy_version_id
wager_policy_hash

algorithm_version

initial_wager_units
total_stake_units
payout_units
net_change_units

result_class
result_payload
result_schema_version

initial_funding_transaction_id
settlement_transaction_id
refund_transaction_id

game_display_name_snapshot

created_at
bet_accepted_at
settled_at
refunded_at
```

---

# 247. Durable Round Lifecycle

核心：

```text
BET_ACCEPTED
      │
      ├── RESOLVING
      │       └── SETTLING
      │              ↓
      │           SETTLED
      │
      └── PLAYER_TURN
              ├── PLAYER_TURN
              └── SETTLING
                     ↓
                  SETTLED
```

异常：

```text
BET_ACCEPTED / RESOLVING / PLAYER_TURN
→ REFUNDING
→ REFUNDED
```

---

# 248. CANCELLED Semantic

对 V1 Paid Direct Play：

```text
BET_ACCEPTED
```

之后不可仅以：

```text
CANCELLED
```

结束并不解释资产。

`CANCELLED` 只适合：

```text
no accepted stake
no financial effect
```

的零资产尝试 / Future Mode。

已接受付费 Round 的无法完成终态必须：

```text
REFUNDED
or
SETTLED
```

---

# 249. Recovery State Separation

不把生命周期覆盖成：

```text
RECOVERING
```

而使用：

```text
lifecycle_state = PLAYER_TURN / RESOLVING ...
recovery_state =
    NORMAL
    RECOVERING
    NEEDS_REVIEW
```

UI 可以显示 Recovering，但 PostgreSQL 保留真实业务阶段。

---

# 250. Server Authoritative

Client 可以提交：

```text
game action
wager
game-specific selection
client seed preference
necessary action input
```

Client 不能提交/决定：

```text
result
random value
payout
win/loss
final wallet balance
server seed
effective config selection
```

正式下注接受、结果、概率映射、派奖与 Wallet Settlement 全部由 Server 完成。

---

# 251. Presentation / Business Separation

例如：

```text
SPINNING
SCRATCHING
SUMMON_ANIMATION
DICE_ROLLING
CARD_FLIP
```

只属于 Presentation。

Presentation：

- 可 Skip；
- 可加速；
- 可降级；
- 可重新播放静态结果。

不得：

- reroll；
- re-settle；
- re-debit；
- change payout；
- change config；
- create new round。

---

# 252. Reveal Sequence

Reveal Game 可以：

```text
business_result = fixed
presentation_progress = partial
```

Presentation Progress 可 Local 或 Optional Synced，但不是资产 Authority。

Progress 丢失最多改变演出位置。

不能改变：

```text
result
payout
round state
```

---

# 253. Multi-action Round

使用：

```text
games.round_actions
```

逻辑字段：

```text
action_id UUIDv7
round_id
action_sequence

client_action_id

expected_round_version

action_type
action_payload

resulting_round_version
result_payload

additional_stake_units

created_at
applied_at
```

Unique：

```text
UNIQUE(round_id, action_sequence)
UNIQUE(round_id, client_action_id)
```

---

# 254. Multi-action Concurrency

Client Action 携带：

```text
expected_round_version
```

Server：

```text
lock Round

duplicate action?
→ return original result

stale version?
→ return authoritative current state

legal action?
→ apply
→ round_version + 1
```

并发 Hit / Stand 等不能同时覆盖同一 Round Version。

---

# 255. Additional Stake Atomicity

需要追加 Stake 的 Action：

```text
BEGIN

lock Round
lock Wallet

validate action
validate wallet

debit additional stake
insert Ledger

insert Action
update Round / Version

COMMIT
```

不能出现：

```text
Action applied
but stake missing
```

或相反。

---

# 256. Settlement Exactly-once

稳定业务 ID：

```text
game_settlement:{round_id}
```

或等价。

Settlement：

```text
BEGIN

lock Round

assert not SETTLED
assert not REFUNDED

derive deterministic authoritative result

calculate payout

credit Wallet
insert Economy Transaction
insert Ledger

persist immutable result
set payout/net change
state = SETTLED

COMMIT
```

重复调用返回原 Settlement。

---

# 257. Result / Settlement Atomicity

最终：

```text
result_payload
payout
wallet credit
ledger
round SETTLED
```

处于同一 PostgreSQL Transaction。

Result 计算后但 COMMIT 前崩溃：

```text
rollback
```

Recovery 使用同样：

```text
Seed
Config
Actions
Algorithm
```

确定性重算同一结果。

---

# 258. Refund Exactly-once

稳定：

```text
game_refund:{round_id}
```

退款：

```text
BEGIN

lock Round

assert not SETTLED
assert not REFUNDED

calculate refundable accepted stake

credit Wallet
insert Refund Ledger

state = REFUNDED
record reason

COMMIT
```

重复请求返回原 Refund。

---

# 259. Settlement / Refund Mutual Exclusion

每个 Round 只能有一个：

```text
terminal_financial_outcome =
    SETTLEMENT
    REFUND
```

永远禁止：

```text
SETTLED
+
REFUNDED
```

Recovery / Maintenance / Operator 均不得绕过该约束。

---

# 260. Round Recovery Worker

扫描：

```text
non-terminal Round
+
recovery due
```

使用：

```text
FOR UPDATE SKIP LOCKED
```

只根据 Durable Facts：

- Round State；
- Locked Config；
- Fairness；
- Actions；
- Funding；
- Settlement / Refund Transaction。

决定下一步。

---

# 261. Recovery Priority

通用：

```text
1. Reconstruct same deterministic Round
2. Resume legal business state
3. Apply game-specific timeout automation
4. Only if legal deterministic completion is impossible:
   Refund
```

Maintenance 不自动等于 Refund。

具体各 V1 游戏的 Recovery / Timeout 进入 TD-07。

---

# 262. Maintenance / Round Acceptance Race

Round Create 与 Maintenance 共享同一个 Runtime Gate 顺序化锁。

如果 Round 先合法 Commit：

```text
accepted before maintenance
→ must finish / recover
```

如果 Maintenance 先 Commit：

```text
new round
→ rejected
```

避免切换瞬间的模糊 Round。

---

# 263. Maintenance Semantics

Maintenance：

```text
blocks new Direct Play Round
```

不遗弃：

```text
BET_ACCEPTED
RESOLVING
PLAYER_TURN
SETTLING
```

Round。

已有 Round 必须：

```text
complete
recover
game-specific timeout action
or
formal refund
```

---

# 264. Provably Fair Coverage

所有由平台随机过程决定：

- 结果；
- 掉落；
- Reel；
- Deck；
- Prize；
- Payout Mapping；

的游戏在正式发布前必须接入 Provably Fair。

未来新随机游戏不能豁免。

---

# 265. Fairness Commitment

引入：

```text
games.fairness_commitments
```

用于 Next Round Pre-commit。

逻辑：

```text
commitment_id UUIDv7
reserved_round_id UUIDv7

newapi_user_id
game_slug

server_seed_hash
server_seed_ciphertext

client_seed
client_seed_version

nonce

algorithm_version

status

created_at
consumed_at
revealed_at
```

玩家在下注前可以获得当前 Next Round：

```text
Server Seed Hash
Client Seed
```

---

# 266. Preallocated Round ID

Fairness Commitment 可以预分配：

```text
reserved_round_id
```

但该 UUID 的存在：

```text
≠ paid Round exists
```

只有 BET_ACCEPTED 事务创建 `games.game_rounds` 后才形成正式 Round。

---

# 267. Client Seed Preference

使用：

```text
games.client_seed_preferences
```

按：

```text
user + game
```

保存：

```text
next-round client seed
client_seed_version
```

BET_ACCEPTED 后：

```text
client_seed
client_seed_version
```

锁定。

修改只影响下一 Round。

---

# 268. Server Seed

Random Direct Play 每 Round 使用独立：

```text
Server Seed
```

要求：

```text
CSPRNG
>= 256-bit entropy
```

终态前：

```text
encrypted at rest
not included in normal logs
```

终态 Reveal 后：

```text
never reused
```

---

# 269. Nonce

推荐：

```text
monotonic per-user/game nonce
```

DB 强制：

```text
UNIQUE(user, game, nonce)
```

Commitment 过期可产生 Gap。

Nonce 永不复用。

---

# 270. Deterministic Random Stream

V1 公共 Random Primitive：

```text
Server Seed
+
Client Seed
+
Nonce
+
Round ID
+
Algorithm Version
+
Stream Counter
```

通过版本化：

```text
HMAC-SHA-256 deterministic stream
```

生成字节流。

精确 Canonical Encoding 在 Implementation Spec 固定。

---

# 271. Game-specific Random Mapping

公共 Fairness Engine：

```text
deterministic random bytes
```

Game Handler：

```text
bytes
→ game result
```

映射算法必须版本化。

禁止隐藏未版本化变化：

```text
probability mapping
paytable
reel strip
shuffle
prize table
```

---

# 272. Bias-free Mapping

有限范围 Mapping 不得直接使用明显有偏：

```text
random_uint % N
```

采用：

```text
rejection sampling
```

或等价无偏方案。

牌组使用版本化 Deterministic Fisher–Yates 或等价无偏 Shuffle。

---

# 273. Round Fairness Binding

随机 Round 至少绑定：

```text
round_id

server_seed_hash
client_seed
nonce

algorithm_version

game_config_version_id
game_config_hash

wager_policy_version_id
wager_policy_hash

game-specific resource versions
```

例如：

```text
paytable_version
reel_strip_version
prize_table_version
shuffle_algorithm_version
```

具体进入 TD-07。

---

# 274. Seed Reveal Boundary

Direct Play：

```text
BET_ACCEPTED / RESOLVING / PLAYER_TURN
→ Server Seed secret
```

到：

```text
SETTLED
REFUNDED
```

才 Reveal。

Multi-action Game 未完成前不得泄露可推导未来随机状态的数据。

---

# 275. Fairness Historical Evidence

终态后保留：

```text
server_seed
server_seed_hash
client_seed
nonce

algorithm_version
config version/hash

game-specific deterministic inputs
game-specific outputs
```

Retired Game 仍能验证历史 Round。

---

# 276. Fairness Verification Contract

Round Detail 应能：

```text
1. hash(server_seed) == server_seed_hash
2. rebuild deterministic stream
3. run game-specific mapper
4. recreate result
5. recalculate payout using locked config
6. compare with stored result / payout
```

同一输入必须得到同一结果。

---

# 277. Golden Test Vectors

每个：

```text
algorithm_version
```

都必须拥有内部 Golden Test Vector：

```text
Server Seed
Client Seed
Nonce
Round ID
Config
→ Expected Random Bytes
→ Expected Result
```

新版本必须创建新 Algorithm Version。

不得修改旧 Algorithm Version 计算结果。

---

# 278. Transparency vs Historical Truth

当前用户公开程度可以按 Game Config 决定：

```text
RTP
Odds
Probability
Weights
```

但不公开不等于可以删除。

历史 Round 必须保留真实 Config / Fairness Evidence。

---

# 279. Common Result Contract

每个 Direct Play Round 保存：

```text
total_stake_units
total_payout_units

net_change_units =
payout - total stake
```

Result Class：

```text
WIN
LOSS
BREAK_EVEN
```

异常用户语义：

```text
CANCELLED
REFUNDED
```

Game-specific Result 存在 Immutable Typed/Versioned Payload。

---

# 280. History Snapshot

Round 预先保存：

```text
game_slug
game_display_name_snapshot

config version/hash
wager policy version/hash
algorithm version
```

终态保存：

```text
stake
payout
net change
result
fairness
refund/cancel reason
```

Game Display Name 后续变化不重写历史。

---

# 281. Retired Game

`RETIRED` 后：

```text
No New Round
```

但继续：

```text
/history
/history/round/:id
historical config
ranking source
fairness verification
```

可读。

---

# 282. Game API Contract Family

精确 Endpoint 延后 TD-13。

逻辑 Contract 固定：

```text
Catalog Query

Game Bootstrap
→ Registry
→ Runtime State
→ Config Summary
→ Wager Policy
→ Available Chips
→ Active Round
→ Next Fairness Commitment

Create Round
Get Active Round
Get Round

Submit Round Action
Fairness Preference
Fairness Verification
```

Browser 只对 Platform Backend Game Contract 操作，不直接拼装 Economy / Config / Fairness Internal API。

---

# 283. Game Bootstrap Resume-first

进入：

```text
/games/:game_slug
```

Backend 先查：

```text
active non-terminal round
```

存在：

```text
effective action = RESUME
```

不展示可直接创建第二 Round 的新下注状态。

---

# 284. Balance Insufficient

如果服务器确认：

```text
Available Chips < required stake
```

则：

```text
No Round
No Wallet Debit
```

可进入：

```text
/wallet
/rewards
```

返回：

```text
restore unsubmitted UI state
refresh balance
no action replay
```

---

# 285. No Auto-play Economic Loop

V1 禁止：

```text
Auto Roll
Auto Spin
Auto Summon
Auto Buy
Auto Blackjack Strategy
```

保存上一局参数只用于填充 UI。

每个新 Round 都必须由用户再次主动提交。

---

# 286. Game Operations Boundary

Operations Game Detail：

```text
Overview
Metadata
Publication
Runtime
Configuration
Fairness
History
```

允许：

- Metadata；
- Publish / Hide；
- Safe Runtime Change；
- Clone Config；
- Validate；
- Preview Diff；
- Activate；
- Inspect History / Fairness。

禁止：

```text
edit settled result
edit payout
change historical seed
change historical config
force accepted Round to new Config
```

---

# 287. Round Incident / Repair

游戏异常不直接修改 History。

采用：

```text
Round Detail
→ Incident
→ Recovery / Economy Repair
```

原 Round / Result / Ledger / Fairness Record 保持只读。

---

# 288. TD-06 Audit

至少：

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
ROUND_RECOVERY_STARTED
ROUND_SETTLED
ROUND_REFUNDED
ROUND_NEEDS_REVIEW

FAIRNESS_COMMITMENT_CREATED
FAIRNESS_COMMITMENT_CONSUMED
FAIRNESS_SEED_REVEALED
```

当前未 Reveal Server Seed 不进入普通 Audit Payload。

---

# 289. TD-06 Metrics

至少：

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
```

---

# 290. TD-06 Crash Point Analysis

| Crash Point | Durable Fact | Recovery |
|---|---|---|
| Client Play before server acceptance | No Round / No debit | Safe retry |
| Fairness Commitment exists only | No asset effect | Expire / replace commitment |
| Before BET_ACCEPTED COMMIT | No paid Round | Retry same idempotency key |
| After BET_ACCEPTED COMMIT, response lost | Round + wager ledger | Return same Round |
| After BET_ACCEPTED, before result | Locked fairness/config/stake | Deterministic recovery |
| Result computed, settlement not committed | No terminal effect | Recompute same result |
| SETTLED COMMIT, response lost | Round + payout ledger | Return original result |
| Multi-action committed, response lost | Action durable | Return original action result |
| Maintenance / Create race | Runtime gate lock orders | Accepted continues, later rejected |
| REFUNDED COMMIT, process crash | Refund durable | Return original refund |
| Worker crash | DB Round state | Next Worker resumes |

---

# 291. Exactly-once Semantics

## Round Acceptance

Network request can be retried.

Business outcome:

```text
one accepted Round
```

through：

```text
Idempotency Key
DB Unique
Wager + Round same transaction
```

## Settlement

Worker may attempt multiple times.

Stable:

```text
game_settlement:{round_id}
```

means one final payout.

## Action

Repeated:

```text
client_action_id
```

returns original action result.

---

# 292. TD-06 Test Gate

Implementation 必须覆盖：

## Registry

- 第六款测试游戏不要求改 Catalog 固定枚举；
- Stable Slug 允许 Display Name 变化；
- Missing Implementation 不能进入 Available；
- DB 无法执行任意脚本成为游戏。

## Config

- Active Config 无法直接编辑；
- Config Hash 篡改检测；
- Historical Config Immutable；
- Activation / Round Creation Race 只产生完整版本引用；
- Global Wager Product Lock 无法被单 Game Config 绕过。

## Round

- 100 个相同 Idempotency Key → 1 Round；
- 同用户同 Game 并发 Create → 最多 1 Active Round；
- Ledger failure → Round Acceptance Rollback；
- Response lost → 同 Round；
- SETTLED Retry → 无第二 Payout；
- REFUNDED → 无后续 Settlement。

## Multi-action

- Duplicate Action → 原结果；
- Concurrent same Version Action → 只有一个成功；
- Additional Stake + Action Atomic。

## Maintenance

- Maintenance / Create Race 无模糊状态；
- Accepted Round 不遗弃；
- Deterministically Recoverable Round 不应直接 Refund。

## Fairness

- Server Seed ≥256-bit；
- Hash 在 BET_ACCEPTED 前存在；
- 未 Reveal Seed 不进日志；
- Revealed Seed 不复用；
- Nonce 不重复；
- Same Input → Same Output；
- Golden Vector 稳定；
- Bias Mapping Property Test；
- Config / Seed 篡改 Verification Fail。

## Recovery

- Redis Clear 不丢 Round；
- Backend Restart 恢复同 Round；
- Resolve Crash 重建同结果；
- Settlement Crash 无重复资产 Effect。

---

# 293. TD-06 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| TD-FRZ-146 | Game Platform 采用 Dynamic Game Registry + Code-owned Runtime Manifest + Direct Play Round Engine + Lobby Adapter，不把 V1 五款游戏写死为永久平台结构。 | FROZEN |
| TD-FRZ-147 | 每款游戏使用稳定 `game_slug` 作为路由、历史、配置、统计与 Registry Identity；Display Name 可版本化变化。 | FROZEN |
| TD-FRZ-148 | Registry 技术上区分稳定 Base Entry Mode 与动态 Effective Entry Action；用户侧继续支持 Direct Play / Lobby / Resume / Maintenance / Coming Soon 语义。 | FROZEN |
| TD-FRZ-149 | Publication State 与 Runtime State 独立；Retired / Maintenance / Unavailable 不删除历史 Round、配置或 Fairness 数据。 | FROZEN |
| TD-FRZ-150 | Game Registry 不加载或执行数据库中的任意游戏代码 / 脚本；DB 只能引用已部署、已注册的 `implementation_key`。 | FROZEN |
| TD-FRZ-151 | Direct Play 实现通过 Code-owned Capability Manifest 声明 Interaction、Fairness、Config、Recovery、History 等能力；Capability 是组合而非固定游戏分类。 | FROZEN |
| TD-FRZ-152 | Future Lobby Games 通过注册 Adapter 接入 Registry；TD-06 不擅自确定斗地主、麻将、炸金花的具体服务形态。 | FROZEN |
| TD-FRZ-153 | Direct Play 共用版本化 Global Wager Policy：最低 10、无产品层固定最高下注、Quick Amount 10/100/500/1000、基础下注整数 Chip；当前为 Product-locked。 | FROZEN |
| TD-FRZ-154 | 每个 Round 同时锁定 `game_config_version`、`game_config_hash` 与 `wager_policy_version`；单游戏 Config 不得覆盖全局 Direct Play 基础下注规则。 | FROZEN |
| TD-FRZ-155 | Game Config 使用 Immutable Version + Code-owned Schema / Validator；Game-specific Config Payload 可以版本化保存，但不得通过通用 JSON Editor 绕过产品规则。 | FROZEN |
| TD-FRZ-156 | Config Activation 使用 Clone → Validate → Preview → Activate New Version；历史 Active Version 不直接编辑，恢复旧配置必须发布新版本。 | FROZEN |
| TD-FRZ-157 | 概率、赔率、RTP、奖表、随机映射等经济配置的正式激活继续仅允许 Super Admin；普通 Metadata 与安全 Runtime 操作按 Games Scope 管理。 | FROZEN |
| TD-FRZ-158 | V1 Direct Play Free Round / Free Summon 能力保持关闭；即使未来 Schema 能表达，当前 Validator 不允许生产激活。 | FROZEN |
| TD-FRZ-159 | READY / SUBMITTING 属于 Round Creation Command/UI 阶段；只有 Wager + Round Acceptance 原子提交成功后才产生正式 `BET_ACCEPTED` Paid Round。 | FROZEN |
| TD-FRZ-160 | Round Create 使用 Client Idempotency Key + Request Payload Hash；重复请求返回原 Round，不同 Payload 复用同 Key 返回冲突。 | FROZEN |
| TD-FRZ-161 | 同一用户同一 `game_slug` 最多拥有一个非终态 Direct Play Round；进入 Game Entry 时优先 Resume 原 Round。 | FROZEN |
| TD-FRZ-162 | `BET_ACCEPTED` 与 Initial Wager Debit / Economy Ledger 必须在同一 Chaldea DB Transaction，并同时锁定 Runtime Gate、Config、Wager Policy 与 Fairness Commitment。 | FROZEN |
| TD-FRZ-163 | Durable Round Lifecycle 使用 BET_ACCEPTED / RESOLVING / PLAYER_TURN / SETTLING / SETTLED / REFUNDING / REFUNDED；Recovery 状态独立保存，不覆盖真实生命周期。 | FROZEN |
| TD-FRZ-164 | V1 已 BET_ACCEPTED 的付费 Round 不能以无资产解释的 CANCELLED 结束；无法完成合法结算时必须正式 REFUND。 | FROZEN |
| TD-FRZ-165 | 所有 Direct Play 使用 Server Authoritative Model；客户端 Presentation / Animation 不决定结果、随机数、派彩或 Wallet。 | FROZEN |
| TD-FRZ-166 | Reveal Sequence 的 Presentation Progress 不属于资产 / 结果 Authority；Skip、Refresh 或动画失败只影响表现，不改变已确定业务结果。 | FROZEN |
| TD-FRZ-167 | Multi-action Round 使用唯一 `action_id / action_sequence` + `round_version`；重复 Action 返回原结果，并发 Stale Action 不得覆盖当前状态。 | FROZEN |
| TD-FRZ-168 | 任何需要追加 Stake 的 Round Action 必须将 Wallet Debit、Ledger、Action 与 Round Version Update 在同一数据库事务提交。 | FROZEN |
| TD-FRZ-169 | Direct Play Settlement 使用稳定 Round Settlement Biz ID，并将 Result、Payout、Wallet / Ledger 与 `SETTLED` 同事务提交，实现 Exactly-once Settlement。 | FROZEN |
| TD-FRZ-170 | Refund 使用稳定 Round Refund Biz ID；同一 Round 的 Settlement 与 Refund 互斥，永远不得既退款又派奖。 | FROZEN |
| TD-FRZ-171 | Round Recovery 只依赖 PostgreSQL Durable Facts；优先恢复同一确定性结果 / 行动状态，仅在无法恢复合法结果时进入正式退款。 | FROZEN |
| TD-FRZ-172 | Maintenance 通过与 Round Creation 共用 Runtime Gate Lock 决定并发顺序；只阻止新 Round，不遗弃已接受 Round。 | FROZEN |
| TD-FRZ-173 | 所有随机结果游戏在发布前必须接入 Provably Fair；未来新增随机游戏不得因不属于 V1 而豁免。 | FROZEN |
| TD-FRZ-174 | Random Direct Play 使用 Next-round Fairness Commitment，在下注接受前产生并持久化 Server Seed Hash、Reserved Round ID、Client Seed、Nonce 与 Algorithm Version。 | FROZEN |
| TD-FRZ-175 | Preallocated `round_id` 仅为 Fairness Commitment Identity；只有 BET_ACCEPTED 原子事务成功后才形成正式 Game Round。 | FROZEN |
| TD-FRZ-176 | Direct Play Random Round 使用独立 ≥256-bit CSPRNG Server Seed；终态前加密保存且不写普通日志，Reveal 后 Seed 永不复用。 | FROZEN |
| TD-FRZ-177 | Client Seed 使用 per-user/game Next-round Preference；BET_ACCEPTED 后当前 Round Client Seed 永久锁定。 | FROZEN |
| TD-FRZ-178 | Nonce 使用数据库保证的不重复序列，推荐 per-user/game 单调递增；未使用 Commitment 可以造成 Gap，但 Nonce 不复用。 | FROZEN |
| TD-FRZ-179 | V1 Provably Fair 公共随机流采用 Versioned HMAC-SHA-256 Deterministic Stream；精确 Canonical Encoding 在 Implementation Spec 固定。 | FROZEN |
| TD-FRZ-180 | Game-specific Random Mapping 必须版本化且无偏；有限映射禁止明显 Modulo Bias，牌组使用确定性无偏 Shuffle。 | FROZEN |
| TD-FRZ-181 | 每个随机 Round 绑定 Server Seed Hash、Client Seed、Nonce、Round ID、Algorithm Version、Game Config Version/Hash、Wager Policy Version 及游戏特有 Paytable/Strip/Shuffle 等版本。 | FROZEN |
| TD-FRZ-182 | Direct Play Server Seed 只在 Round `SETTLED` / `REFUNDED` 后 Reveal；Multi-action Round 完成前不得泄露可推导未来随机状态的数据。 | FROZEN |
| TD-FRZ-183 | Round Fairness Verification 必须能够重新校验 Hash、重建随机流、重算 Game-specific Result 与 Payout；每个 Algorithm Version 必须有 Golden Test Vector。 | FROZEN |
| TD-FRZ-184 | Game History 保留 Game Display Snapshot、Config / Policy / Algorithm Versions、Stake / Payout / Result 与 Fairness Evidence；Retired Game 继续可查询和验证。 | FROZEN |
| TD-FRZ-185 | Game API 统一通过 Platform Backend 提供 Catalog / Bootstrap / Create Round / Active Round / Action / Round Detail / Fairness Contract；精确 REST Path 在 TD-13 收口。 | FROZEN |
| TD-FRZ-186 | TD-06 Implementation 必须通过 Registry Extensibility、Config Immutability、Round Idempotency、Settlement / Refund Exclusivity、Maintenance Race、Fairness Determinism / Bias、Crash Recovery 与 Redis-loss 测试 Gate。 | FROZEN |

---

# 294. Change Log — WORKING v0.6

## Added

- 用户正式确认 TD-06；
- 冻结 `TD-FRZ-146 ～ TD-FRZ-186`；
- 冻结 Dynamic Game Registry；
- 冻结 Stable Game Slug；
- 冻结 Code-owned Runtime Manifest；
- 冻结 Capability Model；
- 冻结 Direct Play / Lobby Adapter；
- 冻结 Publication / Runtime 双状态；
- 冻结 Global Direct Play Wager Policy；
- 冻结 Immutable Game Config Version；
- 冻结 Config Activation Atomicity；
- 冻结 Round Command / BET_ACCEPTED Boundary；
- 冻结 Round Idempotency / Active Round Uniqueness；
- 冻结 Durable Round Lifecycle；
- 冻结 Multi-action Concurrency；
- 冻结 Settlement / Refund Exactly-once；
- 冻结 Recovery / Maintenance Race；
- 冻结 Provably Fair Commitment；
- 冻结 Server Seed / Client Seed / Nonce；
- 冻结 Versioned HMAC-SHA-256 Deterministic Stream；
- 冻结 Bias-free Mapping；
- 冻结 Fairness Golden Test Vector；
- 冻结 Game History / Retired Game Fairness；
- 冻结 TD-06 Crash Point / Test Gate。

## Not Changed

本批没有改变：

- Dice / Scratch / Summon / Slot / Blackjack 数学；
- V1 五款专有 Action / Result State；
- Poker Realtime / Side Pot / Settlement；
- Future 斗地主 / 麻将 / 炸金花规则；
- TD-04 Economy Invariants；
- TD-05 Reward OPEN Fields；
- IA Route；
- Art Direction。

---

# 295. 下一批 — TD-07

下一批正式进入：

> **TD-07 — V1 Direct Play Game Contracts**

计划完整冻结：

1. Dice Contract；
2. Scratch Contract；
3. Summon / Gacha Contract；
4. Slot Contract；
5. Blackjack Contract；
6. 各游戏 Config Schema；
7. 各游戏 Algorithm Version；
8. 各游戏 Wager Derivation；
9. Result Payload；
10. Game-specific Round Lifecycle；
11. Multi-step Blackjack Action；
12. Blackjack Additional Stake；
13. Dice Fairness Mapping；
14. Scratch Prize Mapping；
15. Summon Prize Pool / Multi-draw；
16. Slot Reel Strip / Payline / Stop Index；
17. Blackjack deterministic Shoe；
18. Game-specific Refund / Recovery；
19. Game-specific History Detail；
20. Game-specific Transparency；
21. Fairness Verification Contract；
22. Server/client payload；
23. Error Codes；
24. Config Validation；
25. Test Vectors；
26. Property Tests；
27. Economy / Ranking Linkage；
28. Presentation Hooks；
29. Mobile / Reduced Motion contract boundaries。

注意：

- TD-07 不重新讨论已冻结数学和玩法；
- 所有规则必须从 Requirement / IA 已确认内容技术化；
- 如果某个字段上游没有明确，就标记 `SOURCE / PRODUCT RULE NOT FOUND`，不能用常识补；
- Blackjack 是本批最复杂的 Direct Play Contract，需要完整 Action / Split / Double / Shoe / Recovery 状态；
- Poker 继续留在 TD-08。



---

# 296. TD-07 — V1 Direct Play Game Contracts

> 状态：`FROZEN`  
> 用户确认：`按上述方案整体通过`

## 296.1 TD-07 总体结论

本批将以下五款 V1 Direct Play 的上游已冻结玩法正式技术契约化：

```text
dice
scratch
summon
slot
blackjack
```

统一继承 TD-06：

- Dynamic Game Registry；
- Stable `game_slug`；
- Global Direct Play Wager Policy；
- `ruleset_version` / `algorithm_version` / `game_config_version` / `game_config_hash`；
- Round Create Idempotency；
- Wallet / Ledger 原子资金边界；
- Server Authoritative；
- Provably Fair；
- Settlement / Refund 互斥；
- Recovery / Maintenance；
- History / Fairness Verification；
- Retired Game History 保留。

本批不修改任何上游已冻结数学与玩法，也不涉及 Poker Realtime。

---

# 297. TD-07-C01 — Atomic Fast Settlement

V1 以下四款游戏：

```text
Dice
Scratch
Summon
Slot
```

在服务端接受 Initial Wager 后不等待新的玩家业务决策。

因此正常路径采用：

> **Atomic Fast Settlement**

概念事务：

```text
BEGIN

Validate:
  Auth
  Runtime
  Config
  Wager
  Idempotency
  Fairness Commitment

Lock Wallet
Consume Fairness Commitment

Create Round
Debit Wager
Write Economy / Ledger

Generate Deterministic Result
Calculate Payout

Credit Payout
Write Economy / Ledger

Persist Game-specific Result
Round = SETTLED

COMMIT
```

逻辑生命周期仍表达：

```text
BET_ACCEPTED
→ RESOLVING
→ SETTLING
→ SETTLED
```

但正常路径无需人为拆成多个 Commit。

如果 Commit 之前故障：

```text
No Paid Round
No Wager Effect
No Payout Effect
```

如果 Commit 成功但 HTTP Response 丢失：

```text
SETTLED Round
+
Complete Economy / Ledger
```

Retry 返回同一 Round。

---

# 298. Blackjack Settlement Model

Blackjack 不使用 Fast Settlement。

Blackjack 是正式 Durable Multi-action Round：

```text
BET_ACCEPTED
→ INITIAL_DEAL
→ DEALER_PEEK optional
→ PLAYER_TURN
→ DEALER_TURN
→ SETTLING
→ SETTLED
```

可能跨越多个独立事务：

```text
Hit
Stand
Double
Split
Re-split
System Auto Stand
Dealer Draw
Settlement
```

所有 Action 与 Round State 必须 Durable。

---

# 299. Ruleset Version

五款游戏新增固定：

```text
ruleset_version
```

用于区分：

## Code-owned Product Rules

例如：

```text
Dice Triple kills Big/Small
Scratch exactly one winning triple
Summon Tenfold = 10 independent draws
Slot fixed payline / Wild semantics
Blackjack S17 / 3:2 / Split rules
```

## Configurable Data

例如：

```text
Scratch Prize Table
Summon Prize Table
Slot Reel Strip / Paytable
```

每个 Round 锁定：

```text
ruleset_version
algorithm_version
game_config_version
game_config_hash
wager_policy_version
```

---

# 300. Game-specific Durable Authority

统一 Common Round：

```text
games.game_rounds
```

Game-specific Authority：

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

games.round_actions
```

Game-specific API DTO 可以使用结构化 JSON，但数据库不能只保存一个任意 JSONB 作为唯一结果 Authority。

Common Financial Columns：

```text
initial_wager_units
total_stake_units
payout_units
net_change_units
```

继续使用 BIGINT Atomic Units。

---

# 301. Dice Contract

## 301.1 Identity

```text
game_slug = dice
entry = /games/dice
entry_type = DIRECT_PLAY
interaction = INSTANT_RESOLVE + REVEAL_SEQUENCE
```

## 301.2 Frozen Rule

每 Round：

```text
3 × fair d6

player_choice =
  BIG
  SMALL
```

Small：

```text
sum = 4..10
AND not Triple
```

Big：

```text
sum = 11..17
AND not Triple
```

Triple：

```text
1-1-1
2-2-2
3-3-3
4-4-4
5-5-5
6-6-6
```

任何 Triple：

```text
BIG loses
SMALL loses
```

V1 不允许 Triple 单独下注或其他骰宝投注区。

---

# 302. Dice Math v1

固定：

```text
Win total payout = wager × 2
Loss total payout = 0
```

即 1:1 Net Odds。

三颗 d6：

```text
6 × 6 × 6 = 216
```

精确结果：

```text
Small Win = 105
Big Win   = 105
Triple    = 6
```

单侧赢率：

```text
105 / 216
```

理论 RTP：

```text
2 × 105 / 216
=
97.222222...%
```

House Edge：

```text
2.777777...%
```

当前赔率、Triple 规则属于 Product-locked。

---

# 303. Dice Config Validator

Dice v1 Validator 必须验证：

```text
dice_count = 3
faces_per_die = 6

allowed_choices = BIG / SMALL

small_range = 4..10
big_range = 11..17

triple_rule = BOTH_LOSE

win_total_payout_multiplier = 2
```

并枚举全部：

```text
216 outcomes
```

必须得到：

```text
105 / 105 / 6
RTP = 210 / 216
```

否则 Config / Ruleset 不得 Production Activate。

---

# 304. Dice Fairness

从 TD-06 Deterministic Stream 分别生成：

```text
d1 ∈ [1,6]
d2 ∈ [1,6]
d3 ∈ [1,6]
```

使用无偏有限范围映射。

禁止明显有偏：

```text
random_uint % 6
```

如果 Random Space 不可整除。

同一：

```text
Server Seed
Client Seed
Nonce
Round ID
Algorithm Version
```

必须重建：

```text
same d1/d2/d3
same Triple/Big/Small
same Payout
```

---

# 305. Dice Result Authority

```text
round_id
player_choice

die_1
die_2
die_3

total
is_triple

resolved_side
outcome

total_payout_units
net_change_units
```

`resolved_side`：

```text
BIG
SMALL
TRIPLE
```

但 `TRIPLE` 不是可投注 Selection。

---

# 306. Dice Round Flow

```text
READY
→ choose BIG / SMALL
→ choose Wager
→ Roll
→ SUBMITTING
→ Atomic Fast Settlement
→ SETTLED
→ Reveal Presentation
→ Result Summary
```

Skip Reveal：

```text
Presentation only
```

不得：

- reroll；
- re-settle；
- re-debit；
- create second Round。

---

# 307. Dice History / Transparency

Round Detail 至少保存：

```text
Round ID
Player Choice
Wager
d1 / d2 / d3
Total
Triple
Outcome
Total Payout
Net Change
Balance Before / After

1:1 Rule
RTP

ruleset_version
algorithm_version
game_config_version
game_config_hash
Fairness Evidence

Refund / Cancel Detail if applicable
```

以下始终公开：

```text
Big / Small Rule
Triple Rule
1:1
Probability
RTP
```

普通 Transparency Toggle 不得隐藏。

---

# 308. Scratch Contract

## 308.1 Identity

```text
game_slug = scratch
entry = /games/scratch
entry_type = DIRECT_PLAY
interaction = REVEAL_SEQUENCE
```

## 308.2 Card Model

每张卡：

```text
3 × 3
9 logical cells
```

Winning Card：

```text
Exactly one prize-tier functional symbol
appears exactly 3 times
```

位置任意，不要求连线。

剩余 6 Cell 不得形成第二组三同。

Losing Card：

```text
No functional symbol appears >= 3 times
```

一张卡最多一个奖级。

---

# 309. Scratch Prize Table v1

| Tier | Total Payout Multiplier | Weight / 100,000 |
|---|---:|---:|
| LOSS | 0x | 54,000 |
| BREAK_EVEN | 1x | 19,500 |
| T2 | 2x | 18,500 |
| T3 | 3x | 5,000 |
| T5 | 5x | 2,000 |
| T10 | 10x | 800 |
| T25 | 25x | 180 |
| TOP | 100x | 20 |

Weight Sum：

```text
100,000
```

RTP：

```text
96%
```

1x：

```text
Payout = Wager
Net Change = 0
Result = BREAK_EVEN
```

不是 Win。

---

# 310. Scratch Config Validation

Prize Table Activation 必须验证：

```text
sum(weights) = 100000
weight >= 0
payout_multiplier >= 0
```

并自动计算：

```text
RTP
Loss Probability
Break-even Probability
Positive Profit Probability
Top Prize Probability
```

当前 v1 必须得到：

```text
RTP = 96%
```

---

# 311. Scratch Result Generation

禁止：

```text
randomize 9 cells independently
→ then decide whether card won
```

正式顺序：

```text
1. Weighted Sample Prize Tier
2. Build valid functional 9-cell multiset
3. Winning:
     exactly one matching 3-symbol prize group
4. Losing:
     every symbol count <= 2
5. Deterministically shuffle cell positions
6. Compute Payout
7. Atomic Fast Settlement
```

Prize Sample 与 Position Shuffle 都来自同一 Round 的确定性无偏 Fairness Stream。

---

# 312. Scratch Typed Authority

```text
scratch_results
  round_id
  prize_tier
  payout_multiplier

scratch_cells
  round_id
  cell_index 0..8
  functional_symbol_id
  is_matching_symbol
```

Pixel Scratch Mask：

```text
NOT financial authority
NOT required durable history
```

---

# 313. Scratch Reveal State

资产 Round 可以已经：

```text
SETTLED
```

同时 Presentation：

```text
UNREVEALED
PRESENTING
COMPLETE
```

`COMPLETE`：

```text
all 9 logical cells revealed
OR
Reveal All
```

用户在当前卡 Presentation 未完成前不能购买下一张。

Reveal All：

```text
no new Round
no new debit
no new payout
no result change
```

---

# 314. Scratch Refresh

服务端不保存像素级刮痕。

Refresh：

```text
same round_id
same prize_tier
same logical 9-cell layout
same Payout
```

如果此前 Presentation 未完成：

```text
overlay may be restored
```

用户可以重新刮或 Reveal All。

不能因此第二次派彩。

---

# 315. Scratch Fairness / History

验证：

```text
Seed + Nonce + Algorithm + Config
→ same Prize Tier
→ same functional symbols
→ same positions
→ same payout
```

History 至少：

```text
Prize Tier
Multiplier
9-cell layout
Matching Symbol
Wager
Payout
Net
Balance Before / After

prize_table_version
ruleset_version
algorithm_version
game_config_version/hash
Fairness Evidence
```

当前 Prize Table / Probabilities / RTP / Matching Rules 始终公开。

---

# 316. Summon Contract

## 316.1 Identity

```text
game_slug = summon
entry = /games/summon
entry_type = DIRECT_PLAY
interaction = REVEAL_SEQUENCE
```

结果只产生：

```text
Logical Reward Tier
Entertainment Chips Payout
```

不产生 Servant、礼装、收藏品、背包或交易物。

---

# 317. Summon Logical Tiers

固定逻辑 Tier：

```text
T0
T1
T2
T3
T4
T5
```

美术层可映射为不同稀有度、卡面、光效等。

更换 Presentation 不能改变：

```text
Tier
Probability
Multiplier
Historical Result
Fairness
```

---

# 318. Summon Modes

V1：

```text
SINGLE
draw_count = 1

TENFOLD
draw_count = 10
```

Base Wager：

```text
per Draw
```

Single Cost：

```text
base_wager
```

Tenfold Cost：

```text
base_wager × 10
```

Tenfold 是：

```text
one round_id
one total debit
one total settlement
```

不是十个独立付费 Round。

---

# 319. Summon Prize Table v1

| Tier | Total Payout Multiplier | Weight / 100,000 |
|---|---:|---:|
| T0 | 0x | 59,850 |
| T1 | 1x | 25,000 |
| T2 | 2x | 10,000 |
| T3 | 5x | 4,000 |
| T4 | 20x | 1,050 |
| T5 | 100x | 100 |

理论：

```text
RTP = 96%
```

---

# 320. Summon Tenfold Rules

Tenfold 为：

```text
10 independent deterministic samples
using same prize_table_version
```

V1 没有：

```text
Discount
11th Draw
Guaranteed Tier
Pity
Rate-up
Reroll
Cross-round Probability State
```

Round：

```text
Total Stake =
base_wager × draw_count

Total Payout =
sum(draw payouts)

Net Change =
Total Payout - Total Stake
```

---

# 321. Summon Typed Authority

```text
summon_results
  round_id
  summon_mode
  draw_count
  base_wager_units
  total_cost_units
  pool_id
  prize_table_version
  highest_tier

summon_draw_results
  round_id
  draw_index
  reward_tier
  payout_multiplier
  payout_units
```

Unique：

```text
UNIQUE(round_id, draw_index)
```

Single：

```text
draw_index = 1
```

Tenfold：

```text
draw_index = 1..10
```

---

# 322. Summon Deterministic Domain Separation

每个 Draw 由：

```text
Round Fairness Inputs
+
draw_index
```

确定性产生独立 Sample。

语义：

```text
Seed + Nonce + Round + draw_index
→ deterministic sample
→ Reward Tier
```

Exact Canonical Byte Encoding 进入 Implementation Spec。

Draw N 不得依赖此前 Draw 的 Tier 或输赢。

---

# 323. Summon Pool

V1 只有：

```text
one functional Active Pool
```

Pool 绑定：

```text
stable pool_id
prize_table_version
game_config_version
effective time
publication state
tenfold rules
```

主题包装可以变化。

V1 不提供并行功能性 Banner / Pool。

---

# 324. Summon Result Classification

Single：

```text
T0 → LOSS
T1 → BREAK_EVEN
T2/T3/T4/T5 → WIN
```

Tenfold：

```text
Round Net < 0 → LOSS
Round Net = 0 → BREAK_EVEN
Round Net > 0 → WIN
```

即使存在高 Tier 子 Draw，Round-level Result 仍由整轮 Net Change 决定。

---

# 325. Summon Recovery / Fairness

BET_ACCEPTED 后必须固定：

```text
summon_mode
draw_count
pool_id
prize_table_version
all draw_index results
total payout
net change
```

Refresh 默认可以进入 Result Summary。

Replay Presentation：

```text
no re-settlement
```

Fairness 必须验证：

```text
same Seed/Nonce/Algorithm/Config/draw_index
→ same Draw Tier
→ same multiplier
→ same draw payout

all draws
→ same Total Payout / Net
```

---

# 326. Summon Transparency

始终公开：

```text
T0-T5
Tier Probability
Tier Multiplier
RTP
Single / Tenfold Rule
No Pity
No Guarantee
No Discount
No Rate-up
```

当前 Active Version 必须可识别。

---

# 327. Slot Contract

## 327.1 Identity

```text
game_slug = slot
entry = /games/slot
entry_type = DIRECT_PLAY
interaction = INSTANT_RESOLVE + REVEAL_SEQUENCE
```

## 327.2 Structural Rules

```text
5 Reels × 3 Rows
10 fixed paylines
all paylines always enabled
left-to-right
minimum winning run = 3
Wild substitution
multiple paylines can pay
```

---

# 328. Slot Wager

玩家输入：

```text
Total Wager
```

Line Stake：

```text
total_wager / 10
```

以 Atomic Units 无损计算。

因为：

```text
1 Chip = 500,000 units
```

所以：

```text
1 Chip / 10
= 50,000 units
```

任何整数 Chip Total Wager 都可精确均分。

不要求 Total Wager 是 10 的倍数。

---

# 329. Slot Logical Symbols v1

稳定 Symbol：

```text
L1
L2
L3
M1
M2
H1
H2
W
```

其中：

```text
W = Wild
```

Art Direction 可以改变图片 / 名称，但不得改变 stable `symbol_id`、Reel Stop、概率或历史。

---

# 330. Slot Reel Frequency v1

每个 Reel：

```text
32 stops
uniform stop selection
```

频数：

| Symbol | Count / Reel |
|---|---:|
| L1 | 8 |
| L2 | 7 |
| L3 | 5 |
| M1 | 4 |
| M2 | 3 |
| H1 | 2 |
| H2 | 2 |
| W | 1 |

总计：

```text
32
```

---

# 331. Exact Reel Strip v1

五条 Reel Strip 的**精确顺序**已在冻结 IA 中定义。

Implementation Seed Data 必须逐项使用该 frozen order。

禁止仅保持相同 Symbol Frequency 然后重新随机生成“等价 Strip”。

原因：

```text
3-row visible window
depends on symbol adjacency
```

更换 Strip 顺序会改变数学结果与 RTP。

Round 必须锁定：

```text
reel_strip_version
```

---

# 332. Slot Visible Grid

每个 Reel：

```text
stop_index ∈ [0,31]
```

Visible：

```text
Top    = stop_index - 1 mod 32
Middle = stop_index
Bottom = stop_index + 1 mod 32
```

五个 Reel Stop 独立、无偏生成。

---

# 333. Slot Paylines v1

固定：

```text
1  M-M-M-M-M
2  T-T-T-T-T
3  B-B-B-B-B
4  T-M-B-M-T
5  B-M-T-M-B
6  T-T-M-B-B
7  B-B-M-T-T
8  M-T-T-T-M
9  M-B-B-B-M
10 B-M-M-M-T
```

只：

```text
Reel 1 → Reel 5
```

至少连续 3 个。

不支付：

```text
right-to-left
middle-start
```

---

# 334. Slot Wild Rule

W：

```text
substitutes:
L1/L2/L3/M1/M2/H1/H2
```

同时拥有自身 3/4/5 连 Payout。

如果同一个 Payline 中 Wild 可以解释成多个普通 Symbol：

```text
evaluate all legal interpretations
+
pure Wild interpretation
→ choose highest payout exactly once
```

同一 Payline 不重复支付多个解释。

---

# 335. Slot Paytable v1

| Symbol | 3 | 4 | 5 |
|---|---:|---:|---:|
| L1 | 4x | 15x | 50x |
| L2 | 8x | 25x | 80x |
| L3 | 10x | 40x | 150x |
| M1 | 15x | 60x | 250x |
| M2 | 25x | 100x | 500x |
| H1 | 50x | 250x | 1000x |
| H2 | 100x | 500x | 2500x |
| W | 125x | 1000x | 5000x |

Multiplier 相对于：

```text
Line Stake
```

并表示包含本金的 Total Payout Multiplier。

同一 Payline：

```text
5-chain
```

不得再叠加支付同线的 3-chain / 4-chain。

---

# 336. Slot Frozen Mathematics

基于 frozen：

```text
Reel Strip v1
Payline v1
Paytable v1
Wild interpretation rule
```

理论：

```text
RTP
= 96.0033118724823%

Any non-zero payout
= 41.4642333984375%

Round Net Profit
= 19.3924903869629%

Break-even
= 3.46889495849609%

Round Net Loss
= 77.1386146545410%

Theoretical Max Round Total Payout
= 516.4 × Total Wager
```

---

# 337. Slot Exhaustive Verifier

因为：

```text
32^5
=
33,554,432
```

Stop Combination 有限。

任何新的：

```text
reel_strip_version
paytable_version
payline_version
```

Production Activate 前必须跑完整 Exhaustive Math Verifier。

至少输出：

```text
RTP
Non-zero Payout Probability
Net Profit Probability
Break-even Probability
Net Loss Probability
Max Payout
```

Validation Report Hash 必须绑定被激活 Config。

不允许管理员手工输入“RTP 96%”作为数学真相。

---

# 338. Slot Typed Authority

```text
slot_results
  round_id

  stop_1
  stop_2
  stop_3
  stop_4
  stop_5

  full_grid

  total_wager_units
  line_stake_units

  reel_strip_version
  payline_version
  paytable_version

  total_payout_units
  net_change_units

slot_line_results
  round_id
  line_number
  interpreted_symbol
  match_length
  multiplier
  line_stake_units
  line_payout_units
```

---

# 339. Slot Round Result

正式 Result：

```text
Payout = 0
→ LOSS / NO_WIN

0 < Payout < Wager
→ LOSS / PARTIAL_RETURN

Payout = Wager
→ BREAK_EVEN

Payout > Wager
→ WIN
```

不能因为存在某条 Winning Payline 就把净亏损 Round 标为 Win。

---

# 340. Slot Fairness / Presentation

必须重建：

```text
same Seed + Nonce + Algorithm
→ same 5 Stop Indexes
→ same 5×3 Grid
→ same Line Results
→ same Total Payout
→ same Net
```

Fast Stop / Skip：

```text
Presentation only
```

不能改变 Stop / Grid / Payout。

---

# 341. Blackjack Contract

## 341.1 Identity

```text
game_slug = blackjack
entry = /games/blackjack
entry_type = DIRECT_PLAY
interaction = MULTI_ACTION + REVEAL_SEQUENCE
```

资金：

```text
Initial Wager
+
optional Split / Double Additional Stake
```

---

# 342. Blackjack Frozen Ruleset v1

```text
Deck Count = 6
Cards = 312
New Shoe Every Round

American Hole Card
Dealer Peek

Dealer = S17

Blackjack = 3:2

Hit
Stand
Double
Split

Double Any Two
Double After Split

Max Hands = 4

Split Aces:
  one additional card
  auto complete
  no Hit
  no Double
  no Re-split Aces

No:
  Insurance
  Even Money
  Surrender
  Side Bets
  Persistent Shoe
  Cut Card
  Auto Play
```

---

# 343. Blackjack Shoe

每 Round 生成：

```text
canonical 312-card input deck
↓
versioned deterministic unbiased shuffle
↓
shoe[0..311]
```

必须保存：

```text
shuffle_algorithm_version
shoe_hash
shoe_index
```

以及足够的受保护 Authority 以确保重启后恢复完全相同的 Shoe。

Canonical Card Encoding 与原始 Deck Order 在 Implementation Spec 固定。

---

# 344. Blackjack Durable Tables

## `blackjack_round_state`

```text
round_id

ruleset_version
shuffle_algorithm_version

deck_count = 6
shoe_index

dealer_phase
active_hand_id

last_player_action_at
auto_resolve_at

initial_wager_units
total_stake_units
total_payout_units
net_change_units
```

## `blackjack_hands`

```text
hand_id
round_id
hand_index
parent_hand_id nullable

stake_units

is_from_split
is_split_aces
is_natural

hand_state

hard_total
best_total
is_soft

result
payout_units
net_change_units
```

## `blackjack_dealt_cards`

```text
round_id
shoe_index
card_instance_id

recipient_kind
  PLAYER_HAND
  DEALER

hand_id nullable
recipient_sequence

public_state
```

---

# 345. Blackjack Hand Values

Card Value：

```text
2..9 → face value
10/J/Q/K → 10
A → 11 if possible, otherwise 1
```

Soft Hand：

```text
at least one Ace currently counted as 11
```

Hard Hand：

```text
no Ace counted as 11
```

---

# 346. Natural Blackjack

Natural 仅：

```text
Original
Unsplit
Initial two cards
A + any 10-value card
```

Dealer 无 Blackjack：

```text
Total Payout
= 2.5 × Initial Hand Stake

Net
= +1.5 × Initial Hand Stake
```

Split 后的两张牌 21：

```text
ordinary 21
not Natural
```

---

# 347. Initial Deal

正式顺序：

```text
1. Player first card
2. Dealer Upcard
3. Player second card
4. Dealer Hole Card
```

四张牌全部从已锁定 Shoe 顺序消费并保存对应 `shoe_index`。

---

# 348. Dealer Hole Card Privacy

Hole Card 是已经存在的 Durable Authority，但状态：

```text
NOT_YET_PUBLIC
```

合法 Reveal 前：

- Browser 不得获得；
- Operations 不得提前看到；
- 普通 Logs / Audit 不得输出；
- Refresh / Reconnect 不改变；
- 不允许后续重新生成 / 替换。

---

# 349. Dealer Peek

如果 Upcard：

```text
A
or
10/J/Q/K
```

在任何玩家行动前 Peek。

Dealer Blackjack：

```text
Player Natural
→ PUSH

Player not Natural
→ Original Wager LOSS

→ Round immediate settlement
```

此时没有 Double / Split 机会。

Dealer 无 Blackjack：

```text
continue PLAYER_TURN
```

但 Hole Card 内容继续保密。

---

# 350. Blackjack Hand States

建议：

```text
ACTIVE
STOOD
BUST
DOUBLED_COMPLETE
SPLIT_ACES_COMPLETE
NATURAL_COMPLETE
```

只有：

```text
ACTIVE
```

能接受 Player Action。

---

# 351. Hit

```text
validate Active Hand
consume next shoe card
shoe_index++
append dealt card
recalculate total
```

如果：

```text
total > 21
→ BUST

total = 21
→ STOOD

otherwise
→ ACTIVE
```

---

# 352. Stand

```text
ACTIVE
→ STOOD
```

然后选择下一 left-to-right 未完成 Hand。

没有额外 Wallet Effect。

---

# 353. Double

允许：

```text
Original Unsplit Two-card Hand
OR
Non-Aces Split Two-card Hand
```

DAS 开启。

Split Aces 不允许。

Transaction：

```text
BEGIN

lock Round
lock Hand
lock Wallet

validate action
validate current balance

debit additional stake
= current Hand Stake

write Ledger

Hand Stake *= 2

consume exactly one next Shoe card
record card

Hand = DOUBLED_COMPLETE

write Action
round_version++

COMMIT
```

余额不足：

```text
Double unavailable
```

但 Hit / Stand 继续合法。

---

# 354. Split Eligibility

两张牌按：

```text
Blackjack Point Value
```

相同判断。

因此：

```text
8 + 8
A + A
10 + J
Q + K
10 + Q
```

均可按规则 Split。

---

# 355. Split Transaction

```text
BEGIN

lock Round
lock Active Hand
lock Wallet

validate:
  pair value
  hand_count < 4
  balance

debit additional stake
= current Hand Stake
write Ledger

split original two cards into left/right hands

draw one Shoe card to left
draw one Shoe card to right

assign stable hand_id / hand_index

write Action
round_version++

COMMIT
```

后续按：

```text
left → right
```

操作。

---

# 356. Re-split

非 Aces Split Hand：

```text
if again splittable
AND total hands < 4
AND balance sufficient
→ may Re-split
```

每次 Split / Re-split：

```text
unique action_id
additional stake ledger
same round_id
```

Round 最多：

```text
4 hands
```

---

# 357. Split Aces

```text
A + A
→ Split

each child:
  exactly one new card
  then auto complete

No Hit
No Double
No Re-split Aces
```

Split Aces 子手：

```text
A + 10-value
→ ordinary 21
not Natural
```

---

# 358. Hand Ordering

稳定：

```text
hand_index
```

操作：

```text
left → right
```

一次只：

```text
active_hand_id
```

可以接受 Action。

当前手结束后进入下一手。

全部玩家手结束后：

```text
DEALER_TURN
```

---

# 359. Dealer Engine S17

固定：

```text
Hard <=16 → Hit
Soft <=16 → Hit

Hard >=17 → Stand
Soft 17   → Stand
Soft >=18 → Stand
```

如果所有玩家手已经 Bust：

```text
Reveal Hole Card
No additional Dealer Draw required
→ SETTLING
```

---

# 360. Blackjack Hand Payout

基于实际 Hand Stake：

```text
LOSS / BUST
→ payout = 0

PUSH
→ payout = 1 × stake

NORMAL_WIN
→ payout = 2 × stake

NATURAL
→ payout = 2.5 × initial hand stake
```

Natural 3:2 允许半 Chip。

例如：

```text
Initial Wager = 11
Natural Total Payout = 27.5
Net = +16.5
```

必须用 Atomic Units 精确支付，禁止任何舍入。

---

# 361. Blackjack Round Settlement

```text
Total Stake
=
Initial Wager
+
all Split Stakes
+
all Double Stakes

Total Payout
=
sum(all Hand Payouts)

Round Net Change
=
Total Payout - Total Stake
```

Round Result：

```text
Net < 0 → LOSS
Net = 0 → BREAK_EVEN
Net > 0 → WIN
```

不能用部分 Hand Win 替代 Round-level Result。

---

# 362. Blackjack Action Idempotency

每个 Player / System Action：

```text
action_id
action_sequence
expected_round_version
hand_id
```

要求：

```text
same action_id
→ original action result
```

Stale：

```text
STALE_ROUND_VERSION
+
authoritative current snapshot
```

不得重复：

- Deal Card；
- Double Debit；
- Split Debit；
- Hand Creation。

---

# 363. Blackjack Page Leave / Recovery

离开页面：

```text
No Auto Stand
No Refund
```

Round 保持可恢复。

Refresh / Backend Restart：

必须恢复：

```text
same round_id
same 312-card shoe
same shoe_index
same dealer hole card
same player hands
same hand indexes
same stakes
same actions
same active hand
same legal actions
```

不能重新洗牌或发牌。

---

# 364. Blackjack 24h Auto Resolution

从：

```text
last successful Player Action
```

开始计算 24 小时。

超过：

```text
24h
```

系统：

```text
for every unfinished Player Hand:
  SYSTEM_AUTO_STAND
```

然后：

```text
Dealer executes S17
→ normal settlement
```

必须写入 System Action Record。

Worker 使用稳定 Idempotency：

```text
blackjack:auto-resolve:{round_id}
```

不能重复自动完成。

---

# 365. Blackjack Fairness

Round 至少锁定：

```text
server_seed_hash
server_seed
client_seed
nonce
round_id

ruleset_version
algorithm_version
shuffle_algorithm_version

game_config_version
game_config_hash

deck_count = 6

complete Shoe Authority
all shoe_index consumption
```

Round 完成前：

```text
Server Seed secret
Future Shoe order secret
Dealer Hole Card secret
```

Round 完成后：

```text
Reveal Server Seed
Rebuild full 312-card Shoe
Verify all dealt shoe_index
Verify Hand Results / Payout
```

---

# 366. Blackjack RTP Validation Artifact

Blackjack 不允许在后台手工填入一个目标 RTP 当作真相。

上线前必须建立：

```text
blackjack_validation_artifact
```

至少：

```text
ruleset_version
shuffle_algorithm_version
reference_strategy_version

validation_method
sample_count / enumeration metadata

computed_rtp
computed_house_edge

validation_build
artifact_hash
verified_at
```

正式发布必须基于：

```text
Frozen Ruleset
+
Reference Basic Strategy
+
Reproducible Enumerator / Large Simulation
```

验证后公开参考 RTP / House Edge。

TD-07 不提前写死百分比。

---

# 367. Configuration Validation Matrix

| Game | Activation Validation |
|---|---|
| Dice | 216 exact outcomes, 105/105/6, 1:1, RTP |
| Scratch | Weight Sum, RTP, Tier Multiplier, Layout Invariant |
| Summon | Weight Sum, RTP, Single/Tenfold Cost, 10 independent Draws |
| Slot | Exact Reel Strip, Payline, Paytable, 32^5 Exhaustive Math |
| Blackjack | Frozen Rules, Shuffle Determinism, RTP Validation Artifact |

---

# 368. History / Rankings Contract

五款 Direct Play：

```text
Total Wagered
→ Round Total Stake

Game Profit Source
→ Round Net Change
```

### Scratch

```text
1x
= BREAK_EVEN
```

不算 Positive Profit。

### Summon

Tenfold：

```text
one Round
```

十个 Draw 不分别成为十个 Round。

### Slot

Winning Payline 存在：

```text
≠ Round WIN
```

仍按 Net Change 分类。

### Blackjack

多个 Hand：

```text
one Round
```

正式游戏统计使用 Round Aggregate。

Game Layer 必须同时保存足够的：

```text
Total Stake
Total Payout
Net
sub-result details
```

供 TD-09 Rankings 使用。

---

# 369. Transparency Contract

以下核心数学/规则始终公开。

## Dice

```text
Big / Small
Triple Rule
1:1 Odds
Probabilities
RTP
```

## Scratch

```text
Prize Tiers
Payout Multipliers
Probabilities
RTP
Matching Rule
```

## Summon

```text
T0-T5
Probabilities
Multipliers
RTP
Single / Tenfold
No Guarantee / Pity / Rate-up
```

## Slot

```text
5×3 Grid
10 Paylines
Line Stake Rule
Full Paytable
Wild Rule
Reel Frequencies
Exact Reel Strip Order
RTP / Probability Metrics
Version IDs
```

## Blackjack

```text
6 Decks
Per-round Shuffle
American Hole Card
Dealer Peek
S17
3:2
Hit / Stand / Double / Split
DAS
Max 4 Hands
Split Aces
No Insurance / Surrender / Side Bets
RTP Strategy Dependency
```

这些不受普通 Transparency Toggle 隐藏。

---

# 370. Game-specific Error Semantics

公共：

```text
GAME_NOT_AVAILABLE
GAME_MAINTENANCE

INVALID_WAGER
INSUFFICIENT_CHIPS

ACTIVE_ROUND_EXISTS
IDEMPOTENCY_CONFLICT

FAIRNESS_COMMITMENT_INVALID
CONFIG_VERSION_UNAVAILABLE
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

当存在 Round 时，Error Contract 应附：

```text
round_id
current authoritative state
safe next action / resume information
```

精确 HTTP Mapping 在 TD-13 收口。

---

# 371. Crash / Recovery Matrix

| Game | Normal Crash Protection | Deterministic Recovery |
|---|---|---|
| Dice | Fast Settlement single TX | Same 3 dice |
| Scratch | Fast Settlement single TX | Same Tier / 9 cells |
| Summon | Fast Settlement single TX | Same 1/10 Draws |
| Slot | Fast Settlement single TX | Same Stops / Grid / Lines |
| Blackjack | Durable Multi-action | Same Shoe / Hands / Actions |

四款 Fast Settlement：

```text
Commit failed
→ no paid Round

Commit succeeded
→ complete SETTLED Round
```

Blackjack 可长期处于：

```text
PLAYER_TURN
```

并由 PostgreSQL 恢复。

---

# 372. TD-07 Deployment Impact

不新增独立 Service。

Platform Backend 增加：

```text
dice runtime
scratch runtime
summon runtime
slot runtime
blackjack runtime

game-specific config validators
```

以及：

```text
Slot Exhaustive Math Verifier
Blackjack RTP Validation Tool / Job
Blackjack 24h Recovery Worker
```

Production Readiness 必须匹配：

```text
Active Config
↔ implementation_key
↔ ruleset_version
↔ algorithm_version
↔ required validation artifact
```

不匹配时：

```text
effective runtime = TEMPORARILY_UNAVAILABLE
```

而不是继续开局。

---

# 373. TD-07 Test Gate

## Dice

必须：

```text
Enumerate all 216 outcomes
Small = 105
Big = 105
Triple = 6

RTP exact

Triple always loses BIG/SMALL
same Fairness inputs → same dice
```

## Scratch

必须：

```text
Weight Sum = 100000
RTP = 96%

Winning Card:
  exactly one symbol count = 3

Losing Card:
  every symbol count <= 2

never second winning triple

Reveal All no asset effect
Refresh same layout
```

## Summon

必须：

```text
Weight Sum = 100000
RTP = 96%

Single = exactly 1 Draw
Tenfold = exactly 10 indexed Draws

Tenfold:
  one Round
  one total debit
  one total settlement

No pity / state dependence
same inputs → same indexed Draws
```

## Slot

必须：

```text
5 strips length = 32
frequency vector exact
exact Strip order

10 paylines exact
Paytable exact

enumerate all 32^5 stop combinations

RTP expected
hit/profit/break-even/loss expected
max payout expected

Wild highest interpretation exactly once

same line:
5-chain does not additionally pay 3/4-chain

Fast Stop cannot alter result
```

## Blackjack

必须：

```text
312-card Shoe permutation valid

Initial Deal order exact
Dealer Peek exact

Natural 3:2
Dealer Blackjack Push/Loss

Double:
  one added card
  auto stand
  correct stake

DAS

Split Ten-value cards
Max 4 Hands

Split Aces:
  exactly one additional card
  no Hit
  no Double
  no Re-split

left-to-right hand order

Action Idempotency
Stale Version rejection

S17 Dealer Engine

all player Bust
→ no unnecessary Dealer draw

odd wager Natural
→ exact 0.5 Chip settlement

24h Auto Stand

Service Restart:
same Shoe / Hands / Active Hand

same Seed:
same 312-card Shoe

no unrevealed Hole Card / Seed leakage
```

---

# 374. TD-07 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| TD-FRZ-187 | 五款 Direct Play Round 在现有 Config / Algorithm 之外固定保存 `ruleset_version`，区分 Code-owned Product Rules 与可版本化 Config。 | FROZEN |
| TD-FRZ-188 | Game-specific 正式结果使用 Typed Tables / Structured Authority；JSON DTO 可以作为 API 表现，但不得成为唯一资产 / 结果真相。 | FROZEN |
| TD-FRZ-189 | Dice / Scratch / Summon / Slot V1 正常路径采用 Atomic Fast Settlement：Round Acceptance、Wager、Result、Payout、Ledger、SETTLED 在单一 Chaldea DB Transaction 完成。 | FROZEN |
| TD-FRZ-190 | Blackjack 采用 Durable Multi-action Round，不使用 Fast Settlement；每个玩家 Action 独立事务持久化。 | FROZEN |
| TD-FRZ-191 | Dice v1 固定三颗公平 d6、Big / Small、Triple 通杀、1:1；105 / 105 / 6 Outcome 分布与 97.222222…% RTP 作为精确 Validator Gate。 | FROZEN |
| TD-FRZ-192 | Dice 使用三个确定性无偏 1–6 Sample，同一 Fairness Input 必须重建同一组三骰、结果与派彩。 | FROZEN |
| TD-FRZ-193 | Scratch v1 固定 3×3 九格、中奖卡恰好一个三同奖级、未中奖卡无任何 3+ 同符号、一卡最多一个奖项。 | FROZEN |
| TD-FRZ-194 | Scratch Prize Table v1 使用冻结的 0x / 1x / 2x / 3x / 5x / 10x / 25x / 100x 权重并由 Validator 计算确认 RTP=96%。 | FROZEN |
| TD-FRZ-195 | Scratch 先确定 Prize Tier，再确定性生成合法功能布局并洗牌位置；禁止九格独立随机后再判断输赢。 | FROZEN |
| TD-FRZ-196 | Scratch Pixel Mask 不属于服务端金融 Authority；服务端仅保存逻辑九格及轻量 Reveal Completion，Refresh 可重置覆盖层但不能改变结果或派奖。 | FROZEN |
| TD-FRZ-197 | Summon v1 仅使用逻辑 T0–T5 Chip Reward Tier；Presentation Artwork 不构成玩家永久资产。 | FROZEN |
| TD-FRZ-198 | Summon 只提供 Single=1 Draw 与 Tenfold=10 Draw；Tenfold 为一个 Round、一次总成本扣款、一次总 Settlement。 | FROZEN |
| TD-FRZ-199 | Summon Prize Table v1 使用冻结的 0x / 1x / 2x / 5x / 20x / 100x 权重，Validator 必须确认 RTP=96%。 | FROZEN |
| TD-FRZ-200 | Tenfold 的 10 个 `draw_index` 使用同一 Prize Table、独立确定性 Sample，不存在折扣、保底、Pity、Rate-up、额外 Draw 或跨 Round 概率状态。 | FROZEN |
| TD-FRZ-201 | Summon V1 只有一个功能性 Active Pool；Pool 使用稳定 `pool_id` + Prize Table / Config Version，主题包装不得改变逻辑结果。 | FROZEN |
| TD-FRZ-202 | Summon Single / Tenfold 的 Win / Loss / Break-even 最终均按整个 Round Net Change；Tenfold 子 Draw 不作为 10 个独立 Round。 | FROZEN |
| TD-FRZ-203 | Slot v1 固定 5×3 Grid、10 条 Fixed Payline、5×32 Stop Reel Strip、L1/L2/L3/M1/M2/H1/H2/W 逻辑符号。 | FROZEN |
| TD-FRZ-204 | Slot Reel Strip v1 必须逐项使用上游已冻结的五条精确顺序，不得只按相同 Symbol Frequency 重新生成“等价 Strip”。 | FROZEN |
| TD-FRZ-205 | Slot Total Wager 为整 Spin 基础下注；Line Stake = Total Wager / 10，并使用 Atomic Units 无损计算，不要求 Total Wager 为 10 的倍数。 | FROZEN |
| TD-FRZ-206 | Slot 10 条 Payline、左到右判定、至少 3 连、Wild 最高单一解释、每线只支付一个最高组合及 Paytable v1 作为 Versioned Mathematical Contract。 | FROZEN |
| TD-FRZ-207 | Slot Production Config Activation 必须执行 32^5 全 Stop Exhaustive Verifier，并验证 RTP、Hit / Profit / Break-even / Loss、Max Payout 等公开数学。 | FROZEN |
| TD-FRZ-208 | Slot Result 保存 5 个 Stop、完整 Grid、逐中奖线解释与 Total Payout / Net Change；Round Win 只由 Net Change 决定。 | FROZEN |
| TD-FRZ-209 | Blackjack v1 固定每 Round 新六副牌 Shoe、American Hole Card、Dealer Peek、S17、Natural 3:2、Hit / Stand / Double / Split。 | FROZEN |
| TD-FRZ-210 | Blackjack 使用确定性 312-card Shoe + `shoe_index`；所有 Initial Deal / Hit / Double / Split / Dealer Draw 只能依次消费锁定 Shoe，不允许重新抽牌或洗牌。 | FROZEN |
| TD-FRZ-211 | Dealer Hole Card 是已存在但未公开的 Authority；Player / Operations / Logs 在合法 Reveal 前均不得获得其值。 | FROZEN |
| TD-FRZ-212 | Dealer Upcard 为 A 或 10-value 时必须在任何 Player Action 前 Peek；Dealer Blackjack 时 Natural Push，否则原下注 Loss 并立即结算。 | FROZEN |
| TD-FRZ-213 | Natural 仅限 Original Unsplit Two-card A+10-value，Dealer 无 Blackjack 时支付 3:2；Split 后 21 只按普通 Win。 | FROZEN |
| TD-FRZ-214 | Blackjack Double 允许 Original Two-card 与 Non-Aces Split Two-card，DAS 开启；追加 Stake 与发一张牌 / Auto Stand 在同一 Action Transaction。 | FROZEN |
| TD-FRZ-215 | Blackjack Split 按 Blackjack Point Value 判定，因此全部 Ten-value Card 可以互 Split；每次 Split / Re-split 追加同等 Stake，最多 4 Hands。 | FROZEN |
| TD-FRZ-216 | Split Aces 每手只补一张后自动完成，不允许 Hit、Double 或 Re-split Aces；其 A+10 不属于 Natural。 | FROZEN |
| TD-FRZ-217 | Blackjack Hands 使用稳定 `hand_id / hand_index` 并左到右依次行动；一次仅 Active Hand 可接受 Player Action。 | FROZEN |
| TD-FRZ-218 | Blackjack Dealer Engine 固定 S17；Hard / Soft ≤16 Hit、Hard ≥17 Stand、Soft 17+ Stand；所有玩家 Bust 时无需继续 Dealer Draw。 | FROZEN |
| TD-FRZ-219 | Blackjack Hand Payout 使用 0x / 1x / 2x / Natural 2.5x Total Payout；3:2 产生的半筹码使用 Atomic Units 精确支付，不舍入。 | FROZEN |
| TD-FRZ-220 | Blackjack Round Total Stake 包含 Initial + Split + Double，Round Result 由总 Payout−总 Stake 判断，不以部分 Hand Win 替代 Round 结果。 | FROZEN |
| TD-FRZ-221 | Blackjack Action 使用 Unique Action ID + Sequence + Round Version；重复 Action 返回原结果，Stale Action 不得重复发牌或追加扣款。 | FROZEN |
| TD-FRZ-222 | Blackjack 页面离开不触发 Auto Stand / Refund；最后成功玩家 Action 后 24h 未操作才由幂等 Recovery Worker 写入 System Auto Stand，并按同一 Shoe 完成正常结算。 | FROZEN |
| TD-FRZ-223 | Blackjack Fairness 必须能够从 Server Seed / Client Seed / Nonce / Shuffle Algorithm 重建完整 312-card Shoe，并验证所有 `shoe_index`。 | FROZEN |
| TD-FRZ-224 | Blackjack Production 不手写目标 RTP；必须生成绑定 Ruleset / Reference Strategy / Validator Version 的可重复 RTP / House Edge Validation Artifact 后发布。 | FROZEN |
| TD-FRZ-225 | Dice / Scratch / Summon / Slot 的概率 / 奖表数学以及 Blackjack 核心规则按照上游透明度要求持续公开，不能被普通 Transparency Toggle 隐藏。 | FROZEN |
| TD-FRZ-226 | 五款游戏的 History / Rankings 使用 Round-level Total Stake、Total Payout、Net Change，并保存子结果细节；子 Win 不得错误改变 Round-level Result。 | FROZEN |
| TD-FRZ-227 | Game-specific Error Contract 必须返回稳定错误语义及 Authoritative Round State / Resume Information；HTTP 映射留 TD-13。 | FROZEN |
| TD-FRZ-228 | TD-07 Implementation 必须通过五款游戏各自的 Exact Math / Distribution / Config / Idempotency / Fairness / Recovery / Privacy / Economy Property Test Gate。 | FROZEN |

---

# 375. Change Log — WORKING v0.7

## Added

- 用户正式确认 TD-07；
- 冻结 `TD-FRZ-187 ～ TD-FRZ-228`；
- 冻结 `ruleset_version`；
- 冻结 Typed Game-specific Result Authority；
- 冻结 Dice / Scratch / Summon / Slot Atomic Fast Settlement；
- 冻结 Dice Exact 216 Outcome Validator；
- 冻结 Scratch 3×3 Functional Layout Generator；
- 冻结 Scratch Prize Table v1 / RTP 96%；
- 冻结 Summon T0–T5 / Single / Tenfold Contract；
- 冻结 Summon Prize Table v1 / RTP 96%；
- 冻结 Summon Single Active Pool / No Pity；
- 冻结 Slot Exact Reel Strip / Payline / Paytable；
- 冻结 Slot 32^5 Exhaustive Math Verifier；
- 冻结 Blackjack Six-deck Deterministic Shoe；
- 冻结 American Hole Card / Dealer Peek / S17 / Natural 3:2；
- 冻结 Blackjack Double / DAS / Split / Re-split / Split Aces / Max 4 Hands；
- 冻结 Blackjack Action Idempotency / Round Version；
- 冻结 Blackjack 24h System Auto Stand；
- 冻结 Blackjack RTP Validation Artifact；
- 冻结五款游戏 Transparency / History / Error / Recovery / Test Gate。

## Not Changed

本批没有改变：

- V1 五款已冻结数学；
- Direct Play Global Wager Policy；
- Poker 产品 / Realtime；
- Game Registry；
- Economy Invariants；
- Reward OPEN Fields；
- IA Route；
- Art Direction。

---

# 376. 下一批 — TD-08

下一批正式进入：

> **TD-08 — Poker Service / Realtime / Recovery**

该批为另一高风险核心批次，计划完整冻结：

1. Poker Service Actor / Room Model；
2. Table / Seat / Session / Hand Authority；
3. PostgreSQL Durable State；
4. Redis Ephemeral Boundary；
5. WebSocket Protocol；
6. Connect Ticket / Session Binding；
7. Active Control Connection；
8. Take Over；
9. Table Lifecycle；
10. Seat / Buy-in；
11. Hand Lifecycle；
12. Dealer Button / Blind / Ante；
13. Action Sequence；
14. Action Timer；
15. Disconnect vs Service Failure；
16. Auto Check / Auto Fold；
17. Sit Out；
18. Leave After Hand；
19. Top-up / Rebuy；
20. No-limit Bet Validation；
21. All-in；
22. Main / Side Pot；
23. Eligible Player Set；
24. Showdown；
25. Split Pot；
26. Odd Chip；
27. Hand Settlement Exactly-once；
28. Session / Cash Out；
29. Safe Leave；
30. Recovery Snapshot；
31. Service Restart；
32. Table Pause / Reconnect Grace；
33. Spectator；
34. Chat；
35. Host / Operations Boundary；
36. Poker Provably Fair；
37. Seed Privacy / Delayed Reveal；
38. History；
39. Metrics / Alert；
40. Crash-point Analysis；
41. Concurrency / Property / Recovery Tests。

特别注意：

- Poker 是独立 Go WebSocket Service；
- PostgreSQL 仍是正式 Table / Session / Hand / Asset / Settlement Authority；
- Redis 永远不能成为未结算真实资产的唯一真相；
- Client Disconnect 与 Poker Service Failure 必须分开建模；
- Buy-in / Top-up / Cash Out 继续继承 TD-04 的同库原子 Funding Contract；
- V1 一个用户只能拥有一个 Active Poker Session / Seated Table；
- Hand Settlement 必须 Exactly-once；
- Poker V1 无 Rake，必须保持零和；
- Operations 绝不能直接改 Stack、Pot、Winner、Deck、Settlement 或提前查看 Hole Cards / 未公开 Seed。



---

# 377. TD-08 — Poker Service / Realtime / Recovery

> 状态：`FROZEN`  
> 用户确认：`整体按上述方案通过`

## 377.1 TD-08 总体结论

本批正式冻结：

- Single-writer Table Actor；
- PostgreSQL Durable Authority；
- Redis Ephemeral Boundary；
- Runtime Epoch Fencing；
- Table / Seat / Session / Hand / Action / Pot / Fairness / Chat Durable Objects；
- Password Table Access Grant；
- Seat Reservation；
- Active Poker Session 全局唯一；
- Owned Table 唯一；
- First Buy-in Config Lock；
- Buy-in / Top-up / Rebuy / Cash Out；
- 5s Hand Boundary；
- Two-phase Hand Start；
- Dealer Button / Blind / Action Ordering；
- No-limit Action Engine；
- Full Raise / Short All-in；
- Server Action Timer；
- Client Disconnect vs Service Failure；
- 30s Recovery Grace；
- Sit Out / Auto Safe Leave；
- Deterministic Side Pot；
- Uncalled Excess Return；
- Viewer-specific Projection；
- Hand Settlement Exactly-once；
- Odd Chip；
- Zero-sum Invariant；
- Catastrophic Hand Refund；
- Safe Leave；
- WS Authentication / Envelope / Snapshot；
- Active Control / Take Over；
- Restart Recovery；
- Poker Provably Fair；
- 24h Fairness Release；
- Spectator / Chat；
- Host / Operations / Maintenance Boundary；
- Crash-point / Property / Privacy Test Gate。

本批同时批准技术解析：

```text
TD-08-C01
Service Failure Recovery
→ 30s Reconnect Grace
→ fresh 30s current-action window

TD-08-C02
Uncalled Excess
→ return to original player's Table Stack before Pot Settlement

TD-08-C03
Catastrophic Hand Refund
→ restore each participant to Hand-start Stack
```

继续明确保留：

```text
POKER-PROD-GAP-01
Ante Posting Mode

POKER-PROD-GAP-02
Post BB Now live/dead semantics

POKER-PROD-GAP-03
Initial Dealer Button rule

POKER-PROD-GAP-04
Hand Evaluator edge/tie rules

POKER-PROD-GAP-05
Pot Shortcut exact raise formula
```

以上不得在 Implementation 阶段以“标准 Poker 一般如此”为由自行补全。

---

# 378. Poker Runtime Model

Poker Service：

```text
Lobby Coordinator
Table Actor Registry

Table Actor A
Table Actor B
Table Actor C
...

Recovery Worker
Timer Scheduler
Fairness Release Worker
Lobby Projection Publisher
```

每张活动 Table 使用：

```text
one Table Actor
+
one command mailbox
```

串行处理：

```text
Join
Hand Start
Action
Timeout
Boundary Top-up
Sit Out
Leave
Take Over
Host Operation
Settlement
```

---

# 379. PostgreSQL Authority / Commit-before-Broadcast

Table Actor 只是：

```text
serialization coordinator
```

不是 Source of Truth。

正式状态：

```text
PostgreSQL
= Table / Session / Hand / Asset / Pot / Settlement Authority
```

每个正式状态变化：

```text
Receive Command
→ Validate
→ PostgreSQL Transaction
→ COMMIT
→ update in-memory projection
→ broadcast WS event
```

必须：

```text
Commit Before Broadcast
```

禁止：

```text
broadcast action
→ DB write later
```

---

# 380. Runtime Epoch

每桌维护：

```text
runtime_epoch BIGINT
```

Table Actor Recovery / Claim：

```text
BEGIN
lock poker.tables
runtime_epoch++
COMMIT
```

之后正式 Write 必须带：

```text
expected_runtime_epoch
```

旧 Actor：

```text
epoch mismatch
→ rejected
```

Redis Lock 不作为唯一 Split-brain Fencing Authority。

---

# 381. Redis Ephemeral Boundary

Redis 只用于：

```text
poker:connection:*
poker:seat-reservation:*
poker:table-access:*
poker:lobby-delta:*
poker:event-buffer:*
```

例如：

- WS Connection Lease；
- Seat Reservation；
- Password Access Grant；
- Lobby Pub/Sub；
- Recent Event Buffer。

以下不得 Redis-only：

```text
Table Stack
Committed Chips
Pot / Side Pot
Hole Cards
Full Deck
Hand State
Settlement
Session Cash Out
```

---

# 382. Durable Poker Objects

至少：

```text
poker.tables
poker.seats
poker.sessions

poker.hands
poker.hand_players
poker.actions
poker.dealt_cards

poker.pots
poker.pot_eligible_players
poker.pot_awards

poker.funding_operations

poker.fairness_commitments
poker.client_seed_contributions
poker.fairness_releases

poker.chat_messages
poker.chat_mutes
```

---

# 383. Poker Table

`poker.tables` 至少：

```text
table_id UUIDv7
owner_newapi_user_id

table_name

access_mode
password_hash nullable

max_seats

blind_preset_version_id
poker_ruleset_version
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

---

# 384. Table Lifecycle

```text
CREATED
↓
WAITING
↓
IN_HAND
↓
INTERMISSION
↓
WAITING
```

故障：

```text
IN_HAND
→ PAUSED
→ RECOVERING
→ IN_HAND
```

关闭：

```text
WAITING / INTERMISSION / IN_HAND
→ CLOSING
→ CLOSED
```

独立 Gate：

```text
accepting_players
allow_new_hands
```

---

# 385. Empty Table Auto Close

完全：

```text
No seated players
No spectators
```

持续：

```text
30 minutes
```

并且不存在：

```text
unfinished Hand
unfinished Buy-in
unfinished Cash Out
Poker In Play
```

才允许自动 Closed。

Auto-close Operation 使用稳定 Idempotency Identity。

---

# 386. Password Table

Password Table 继续在 Lobby 可见：

```text
Table Name
Blind
Seats
Status
Lock
```

Password 同时保护：

```text
Seat
Spectate
Table Chat Access
```

Password 错误：

```text
No Seat Reservation
No Buy-in
No Active Session
No Table Visibility beyond allowed lobby metadata
```

数据库只保存：

```text
Argon2id Password Hash
```

不得保存 / Log 明文。

---

# 387. Table Access Grant

Password 验证成功后创建：

```text
session-bound ephemeral access grant
```

例如：

```text
poker:table-access:{chaldea_session}:{table_id}
```

Redis 丢失：

```text
may require re-entering password
```

但不会影响：

```text
Seat
Session
Stack
Hand
Assets
```

---

# 388. Seat Reservation

Seat Reservation：

```text
Redis SET NX
TTL = 30s
```

最终 Buy-in Transaction 再次确认：

```text
seat empty
user eligible
no active session conflict
```

30 秒未成功 Buy-in：

```text
reservation expires
no Wallet debit
no Stack
no Session
```

---

# 389. Poker Session

`poker.sessions` 至少：

```text
session_id UUIDv7

newapi_user_id
table_id
seat_no

identity_display_snapshot_id

state

initial_buyin_units
total_topup_units

current_stack_units

final_cashout_units nullable
realized_pl_units nullable

control_epoch

started_at
ended_at
end_reason
```

Session：

```text
first successful Buy-in
→ ACTIVE

Safe Leave + Cash Out
→ SETTLED
```

Realized P/L：

```text
Final Cash Out
-
all successful Buy-in / Top-up
```

---

# 390. Active Poker Session Uniqueness

数据库强制：

```text
one Active Poker Session
per newapi_user_id
```

存在 Active Session：

```text
cannot seat another Table
cannot create another Table
cannot spectate another Table
```

Lobby 优先显示 Reconnect。

---

# 391. Owned Table Uniqueness

独立约束：

```text
one non-closed owned Table
per owner
```

Create Table：

```text
does not create Seat
does not create Session
```

成功 Buy-in 后才形成 Active Session。

---

# 392. First Buy-in Locks Settings

第一笔成功 Buy-in 的同一事务中：

```text
if settings_locked_at IS NULL:
    lock max_seats
    lock blind/ante preset
    lock buy-in bounds
    lock spectator policy
    lock running economic config
```

之后 Host 不可修改上述运行中经济配置。

---

# 393. Blind / Ante Presets

V1 首发 SB / BB：

```text
5 / 10
10 / 20
25 / 50
50 / 100
100 / 200
500 / 1000
```

每档存在：

```text
NO_ANTE
or
ANTE_10_PERCENT_BB
```

Buy-in：

```text
minimum = 40 BB
maximum = 100 BB
```

每个 Hand 锁定：

```text
blind_preset_version_id
poker_ruleset_version
algorithm_version
game_config_version
config_hash
```

---

# 394. POKER-PROD-GAP-01 — Ante Posting Mode

当前只冻结：

```text
Ante amount = 10% BB
```

没有冻结：

```text
PER_PLAYER_ANTE
or
BIG_BLIND_ANTE
```

因此：

```text
ante_mode
= PRODUCT RULE NOT FOUND
```

该 Preset 在此字段确认前不得进入正式生产激活。

---

# 395. Poker Integer Chip Rule

所有：

```text
Blind
Ante
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

底层都使用 Atomic Units，但要求：

```text
units % 500000 = 0
```

钱包小数筹码留在 Wallet。

---

# 396. Buy-in Atomic Funding

Buy-in：

```text
BEGIN

lock Wallet
lock Table / Seat

validate Reservation
validate Active Session uniqueness
validate Buy-in range
validate Wallet

debit Available Chips
write Wallet Ledger

create Poker Session
occupy Seat
create Table Stack

if first Buy-in:
    lock Table settings

COMMIT
```

语义：

```text
Available Chips -X
Poker In Play +X
Total Assets unchanged
```

---

# 397. Seat State

建议：

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

Connection 状态独立：

```text
CONNECTED
DISCONNECTED
```

---

# 398. Joining Running Table

运行中 Table 新玩家：

```text
WAIT_FOR_BIG_BLIND
or
POST_BIG_BLIND_NOW
```

不能进入当前 Hand 中途。

至少：

```text
2 seated
2 ready
not sit out
stack > 0
```

才允许新 Hand。

---

# 399. POKER-PROD-GAP-02 — Post BB Now

已经冻结：

```text
Post one entering Big Blind
at next eligible Hand
```

但：

```text
live blind / dead blind semantics
betting rights
```

未冻结。

技术层保留：

```text
entry_blind_rule_version
```

不得自行补。

---

# 400. Hand Intermission

每 Hand 之间：

```text
5 seconds
```

Boundary 顺序：

```text
1. Finish previous Settlement
2. Apply Safe Leave / Remove After Hand
3. Apply Sit Out
4. Apply eligible Pending Top-up / Rebuy
5. Activate newly bought-in Seats
6. Resolve waiting blind states
7. Select next participants
8. Prepare next Hand
```

---

# 401. Top-up / Rebuy

Hand 中：

```text
Top-up Request
→ PENDING
```

不改变当前 Hand Stack。

Boundary：

```text
revalidate Wallet
revalidate Stack
revalidate <=100BB cap
→ Atomic Wallet Debit + Stack Credit
```

失败：

```text
NO EFFECT
```

通过牌局赢得 Stack >100BB：

```text
no forced reduction
```

V1 无 Auto Rebuy。

---

# 402. Stack = 0 / Rebuy Window

Stack 归零：

```text
state = REBUY_WINDOW
rebuy_deadline_at = now + 60s
```

成功 Rebuy：

```text
continue same Session
```

超时：

```text
Cash Out 0
→ Seat Leave
→ Session Settlement
```

---

# 403. Two-phase Hand Start

每 Hand Start 使用两个 Durable Commit。

## Phase A — Commitment

```text
BEGIN

lock Table

select participants
lock starting stacks
lock config/rules

resolve next Dealer Button

freeze Client Seed Contributions

generate Server Seed
persist Server Seed Hash

derive Effective Client Seed
generate / lock full 52-card Deck

create Hand
state = COMMITTED

COMMIT
```

然后 Broadcast：

```text
HAND_COMMITTED
server_seed_hash
hand_id
participants
versions
```

## Phase B — Forced Bets / Deal

```text
BEGIN

lock Hand

post Ante / Blind / Entry Blind
record forced-bet Actions

deal Hole Cards from locked Deck

street = PREFLOP
set first actor
set 30s deadline

COMMIT
```

确保：

```text
Seed Hash durable
before dealing
```

---

# 404. Hand Lifecycle

```text
COMMITTED
↓
POSTING_FORCED_BETS
↓
DEALING_HOLE_CARDS
↓
PREFLOP
↓
FLOP
↓
TURN
↓
RIVER
↓
SHOWDOWN
↓
SETTLING
↓
SETTLED
```

Early Winner：

```text
only one non-folded player
→ SETTLING
```

All-in：

```text
no further decision
→ ALL_IN_RUNOUT
→ SHOWDOWN
→ SETTLING
```

故障：

```text
PAUSED
RECOVERING
```

灾难性不可恢复：

```text
REFUNDING
REFUNDED
```

---

# 405. Hand Player Snapshot

`poker.hand_players` 至少：

```text
hand_id
session_id
seat_no

starting_stack_units

street_committed_units
total_committed_units

is_folded
is_all_in
is_showdown_eligible

hole_card_index_1
hole_card_index_2

timeout_streak_at_hand_start
```

Hand 开始后的 Participant Set 不受：

```text
Disconnect
Take Over
Profile Change
```

影响。

---

# 406. Dealer Button / Order

三人及以上：

```text
Button 左侧第一位 = SB
SB 左侧第一位 = BB

Preflop:
BB 左侧第一位仍有行动权玩家先

Postflop:
Button 左侧第一位仍在局玩家先
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

---

# 407. POKER-PROD-GAP-03 — Initial Dealer Button

后续 Button 顺时针移动已经冻结。

但：

```text
first Hand initial Button assignment
```

没有找到冻结规则。

因此：

```text
initial_button_rule
= PRODUCT RULE NOT FOUND
```

Implementation 不可自行选择随机 / 最小 Seat / 创建者等规则。

---

# 408. Player Action Command

Client 发送：

```text
action_id
hand_id
expected_hand_version
control_epoch

action_type
requested amount / raise-to if needed
```

Server Authoritative：

```text
legal_actions
to_call
minimum_bet
minimum_raise_to
maximum_raise_to
current_pot
current_stack
raise_rights
```

Client 不自行决定最终 Legal Action Set。

---

# 409. Action Types

Player：

```text
FOLD
CHECK
CALL
BET
RAISE
ALL_IN
```

System：

```text
POST_ANTE
POST_SB
POST_BB
POST_ENTRY_BB

AUTO_CHECK
AUTO_FOLD

RETURN_UNCALLED

DEAL_FLOP
DEAL_TURN
DEAL_RIVER

SHOWDOWN_REVEAL
SYSTEM_SETTLEMENT
```

---

# 410. Poker Action Record

`poker.actions`：

```text
action_id UUIDv7
hand_id
action_sequence

actor_session_id nullable
actor_seat_no nullable

action_type

requested_to_units nullable
applied_delta_units

street

hand_version_before
hand_version_after

is_timeout
is_system

created_at
```

Unique：

```text
UNIQUE(hand_id, action_id)
UNIQUE(hand_id, action_sequence)
```

---

# 411. Hand Version

每次正式 Action：

```text
hand_version++
```

Client 需要：

```text
expected_hand_version
```

Stale：

```text
STALE_HAND_VERSION
+
authoritative snapshot
```

旧设备不能覆盖新状态。

---

# 412. Full Raise Tracking

保存：

```text
current_bet_to_units
last_full_raise_increment_units
full_raise_sequence
```

每玩家：

```text
last_acted_full_raise_sequence
```

完整 Raise：

```text
full_raise_sequence++
last_full_raise_increment = actual increment
```

Short All-in：

```text
does not increment full_raise_sequence
```

因此不会错误重新开放已行动玩家 Raise 权。

---

# 413. Bet Shortcuts

Server 提供：

```text
Min
1/2 Pot
2/3 Pot
Pot
All-in
```

只是 Suggested Legal Target。

Fraction：

```text
floor
```

且不得低于合法 Minimum。

---

# 414. POKER-PROD-GAP-05 — Pot Shortcut Formula

已经冻结按钮存在和向下取整。

但 Raise 场景：

```text
1/2 Pot
2/3 Pot
Pot
```

精确计算公式仍未冻结。

因此：

```text
bet_shortcut_formula_version
= PRODUCT RULE NOT FOUND
```

正式 UI 不能在没有公式版本时自行计算。

---

# 415. Action Transaction

例如 Raise：

```text
BEGIN

lock Hand
lock Acting Player / Seat

verify:
  runtime_epoch
  control_epoch
  expected_hand_version
  action_id
  current actor
  deadline
  legal action
  legal amount

move:
  Table Stack
  → Hand Commitment

write Action

update:
  current bet
  raise increment / rights
  pot projection
  next actor / street
  timer
  hand version

COMMIT
```

不触碰主 Wallet。

---

# 416. Action Timer

每个需要玩家决策节点：

```text
30 seconds
```

无 Time Bank。

PG 保存：

```text
action_started_at
action_deadline_at
current_actor_seat
action_sequence
```

Client Timer 只是 Server Deadline Projection。

---

# 417. Timeout Race

Timer System Action ID：

```text
timeout:{hand_id}:{action_sequence}
```

到期：

```text
BEGIN
lock Hand

if action_sequence advanced:
    no-op

if now < deadline:
    no-op

if Check legal:
    AUTO_CHECK
else:
    AUTO_FOLD

COMMIT
```

User Action 与 Timeout 同时发生：

```text
database transaction ordering decides
```

只能一条合法 Action Commit。

---

# 418. Consecutive Timeout

Seat / Session 保存：

```text
consecutive_timeout_count
```

Timeout：

```text
+1
```

人工成功 Action：

```text
reset = 0
```

连续：

```text
2
```

后：

```text
sit_out_next_hand = true
reason = CONSECUTIVE_TIMEOUT
```

当前 Hand 继续依法完成。

---

# 419. No Advanced Pre-action

V1 不提供：

```text
Time Bank
Check/Fold Pre-action
Call Any
Auto Call
Auto Bet
Auto Raise
Strategy Preset
```

全部 All-in 且无决策时自动 Runout。

---

# 420. Client Disconnect vs Service Failure

## Client Disconnect

```text
Poker Service healthy
socket lost
```

行为：

```text
Timer continues
Timeout → Auto Check/Fold
Hand continues
Boundary while still disconnected → Sit Out
15min continuous disconnect + no unsettled hand
→ Auto Safe Leave
```

## Service Failure

```text
Poker Service unavailable
```

行为：

```text
Table Paused
Timer paused semantically
PostgreSQL Recovery
30s Reconnect Grace
Resume same Hand
```

---

# 421. TD-08-C01 — Service Failure Timer

正式冻结：

```text
Service Restored
→ 30s Reconnect Grace
→ if current Action still required:
     fresh full 30s Action Window
→ Resume
```

正常 Client Disconnect 不适用。

---

# 422. Disconnect Sit-out

Socket Close：

```text
persist disconnected_at
```

当前 Hand：

```text
responsibility remains
```

如果在 Hand Boundary 仍断线：

```text
Sit Out
```

如果 Boundary 前成功 Reconnect：

```text
clear disconnect-induced next-hand sit-out
```

不影响连续 Timeout 产生的 Sit Out。

---

# 423. Sit Out

`Sit Out Next Hand`：

```text
does not cancel current Hand
```

下一 Hand：

```text
Seat remains
Stack remains
Session remains
No Hole Cards
```

恢复时：

```text
WAIT_FOR_BB
or
POST_BB_NOW
```

---

# 424. 15-minute Auto Safe Leave

保存：

```text
sit_out_since
disconnected_since
```

Continuous：

```text
>=15m
AND no unsettled Hand
```

才触发：

```text
Auto Safe Leave
→ Cash Out
→ Session Settlement
```

若 Hand 未结算：

```text
defer until Settlement
```

---

# 425. Pot Authority

正式投入 Authority：

```text
street_committed_units
total_committed_units
```

Pot 从所有 Commitment 确定性构建。

不能只把：

```text
pot_total
```

作为唯一来源。

---

# 426. Deterministic Side Pot Builder

对所有：

```text
total_committed_units > 0
```

提取 Unique Commitment Levels：

```text
L1 < L2 < L3 ...
```

每层：

```text
layer_amount
=
(level - previous_level)
×
count(players with commitment >= level)
```

Eligible Set：

```text
commitment >= level
AND not folded
```

Fold Player：

```text
contribution included
winner eligibility excluded
```

形成：

```text
Main Pot
Side Pot 1
Side Pot 2
...
```

---

# 427. TD-08-C02 — Uncalled Excess

正式冻结：

> 没有第二位玩家匹配的最顶层 Uncalled Excess，在 Pot Settlement 前原子退回原玩家 Table Stack。

写入：

```text
RETURN_UNCALLED
```

它：

```text
does not become settled Pot
does not become ownership transfer
does not count as wagered settlement amount
```

---

# 428. Pot Durable Model

`poker.pots`：

```text
pot_id
hand_id
pot_index
pot_type

amount_units
contribution_floor
contribution_ceiling

state
settled_at
```

`poker.pot_eligible_players`：

```text
pot_id
session_id
seat_no
```

`poker.pot_awards`：

```text
pot_id
winner_session_id

base_share_units
odd_chip_units
final_award_units
```

同一 Pot：

```text
settle once
```

---

# 429. Hand Evaluator

输入：

```text
2 Hole Cards
+
0–5 Community Cards
```

最终：

```text
up to 7 cards
→ best 5-card hand
```

Evaluator 输出：

```text
category
primary_rank_vector
best_five_card_indices
hand_evaluator_version
```

Comparison 使用 Versioned Canonical Rank Vector。

---

# 430. POKER-PROD-GAP-04 — Hand Evaluator Edge Rules

上游只冻结牌型大类，没有逐项冻结：

```text
A2345 Wheel semantics
Suit tie semantics
all kicker / tie-break edges
```

因此：

```text
hand_evaluator_version
```

必须有 Canonical Poker Ruleset + Test Vectors 后才 `PRODUCTION_READY`。

不得以模型常识补写并伪装成上游已确认规则。

---

# 431. Early Winner

如果：

```text
only one non-folded player remains
```

直接：

```text
winner receives all pots for which eligible
→ SETTLING
```

不继续无意义 Board。

未公开 Hole Card 不因 Early Win 强制公开。

---

# 432. All-in Runout

如果所有仍在局玩家：

```text
ALL_IN
```

且无 Action：

```text
Reveal remaining live Hole Cards
Deal remaining Board
Showdown
Pot Settlement
```

---

# 433. Ordinary Showdown

实时公开：

```text
cards required to determine winner
```

Loser 可以 Muck。

Folded / Mucked Hole Cards：

```text
remain hidden in realtime
```

V1 不提供主动 Show Folded Cards。

---

# 434. Viewer-specific Projection

正式 Server Projection：

```text
Authoritative State
├── Player A Projection
├── Player B Projection
├── Spectator Projection
└── Operations Projection
```

Player：

```text
own Hole Cards
+
public information
```

Spectator / Host / Operations：

```text
authorized public state only
```

禁止：

```text
send all Hole Cards to browser
→ hide with UI
```

---

# 435. Hand Settlement Exactly-once

稳定 Biz ID：

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

return Uncalled Excess

build deterministic Pots

evaluate eligible hands

calculate split shares
assign Odd Chips

increase winner Table Stacks

close all commitments

persist Pot / Awards / Result

Hand = SETTLED

COMMIT
```

不触碰 Wallet。

---

# 436. Odd Chip

并列 Winner：

```text
base_share = floor(pot / winner_count)
```

余数：

```text
Odd Chip
```

依次从：

> Dealer Button 左侧开始顺时针查找对该 Pot 有资格的并列 Winner。

必须记录最终 Odd Chip Receiver。

---

# 437. Zero-sum Invariant

V1：

```text
Rake = 0
```

Hand 必须满足：

```text
Total Commitments
=
Pot Awards
+
Returned Uncalled Excess
```

Poker Hand 不：

```text
issue
burn
rake
```

---

# 438. TD-08-C03 — Catastrophic Hand Refund

正式冻结：

如果已开始 Hand 确实无法合法恢复：

```text
Automatic deterministic refund
=
restore each participant
to hand_start_stack_units
```

也就是本 Hand 的 Commitment 全部退回原所有者。

Refunded Hand：

```text
No Winner
No Game Profit
No Biggest Win
No Wagered ranking input
```

历史保留：

```text
Action Timeline
Refund Reason
Incident
Audit
Before / After
```

如果无法证明每一单位来源：

```text
NEEDS_REVIEW
```

不猜余额。

---

# 439. Safe Leave

统一覆盖：

```text
Return Lobby
Leave Table
Browser Back
Host Remove Player
Auto Leave
```

未参与 Hand：

```text
→ immediate Safe Leave / Cash Out
```

正在参与 Hand：

```text
→ LEAVE_AFTER_HAND
→ Settlement
→ Cash Out
```

即使已 Fold：

```text
wait Settlement
```

---

# 440. Cash Out Atomicity

Biz ID：

```text
poker_cashout:{session_id}
```

事务：

```text
BEGIN

lock Session
lock Seat
lock Wallet

assert no unsettled owned Hand
assert not already cashed out

final_stack = Table Stack

set Table Stack = 0

credit Available Chips
write Ledger

final_cashout = final_stack

realized_pl =
final_cashout
- all Buy-in / Top-up

Session = SETTLED
Seat = LEFT

COMMIT
```

重复：

```text
return original result
```

不允许 Partial Cash Out。

---

# 441. Rankings Commit Point

Hand Settlement 时可以保存：

```text
hand_net_change
hand_total_wagered
```

但 Poker Public Ranking 正式输入只有：

```text
parent Session Cash Out
```

之后才提交：

```text
Session Realized P/L
eligible Hand Biggest Win
Total Wagered
```

---

# 442. Poker WebSocket Auth

采用：

```text
Sec-WebSocket-Protocol:
chaldea-poker.v1
```

不把 Secret 塞入 Subprotocol。

WSS Upgrade 后第一个 Frame：

```text
auth.connect
{
  poker_connect_ticket
}
```

这样 Secret 不进入 URL Query。

---

# 443. WS Authentication Flow

```text
Browser
→ BFF mint Poker Connect Ticket
→ WSS Upgrade

Poker validates:
  Origin
  protocol

connection = AUTH_PENDING

→ auth.connect(ticket)

validate:
  signature
  user
  chaldea session
  purpose
  expiry
  single-use
  target binding if present

→ AUTHENTICATED
```

Auth Deadline 精确秒数进入 Implementation Spec。

---

# 444. No Secret in WS URL

明确禁止：

```text
wss://.../ws/poker?token=<secret>
```

避免 Ticket 进入：

```text
Proxy access logs
URL traces
Screenshots
Debug history
```

---

# 445. WebSocket Command Envelope

Client：

```text
type
request_id

table_id
hand_id nullable

action_id nullable

expected_table_version
expected_hand_version

control_epoch

payload
```

Server Event：

```text
type
event_id
event_seq

table_id
table_version

hand_id nullable
hand_version nullable

server_time

payload
```

---

# 446. WS Delivery Semantics

WS Frame：

```text
may duplicate
may be lost
```

正式语义：

```text
Business state exactly-once / idempotent
+
WS at-least-once / loss-tolerant
```

Client：

```text
event_seq <= last_seen
→ ignore duplicate
```

Gap：

```text
→ resync
```

---

# 447. Snapshot-first Reconnect

Reconnect：

```text
authoritative snapshot
→ snapshot version
→ live delta after version
```

Redis Event Buffer 可以补小 gap。

Event Buffer 不完整：

```text
full snapshot
```

Redis 丢失不影响正式恢复。

---

# 448. Slow Client Backpressure

每 Connection：

```text
bounded send queue
```

持续过慢：

```text
disconnect socket
→ reconnect
→ snapshot
```

不得无限缓冲拖垮 Table Actor。

---

# 449. Active Control Connection

Active Session：

```text
one Active Control Connection
```

Connection Lease：

```text
ephemeral
```

真正 Action Authority：

```text
session.control_epoch
```

Durable in PostgreSQL。

---

# 450. Take Over

第二设备：

```text
Authenticate
→ ACTIVE_CONTROLLER_EXISTS
→ explicit Take Over
```

事务：

```text
BEGIN
lock Session

control_epoch++
Audit

COMMIT
```

新设备：

```text
epoch = N+1
```

旧设备：

```text
epoch = N
→ Action rejected
→ READ_ONLY
```

---

# 451. Service Restart / Control

Restart 后 Connection Lease 丢失。

`control_epoch` 保留。

第一个合法 Reconnect：

```text
claim controller lease
```

两个设备同时：

```text
first wins
second requires explicit Take Over
```

---

# 452. Reconnect Snapshot

至少：

```text
Table / Table Version

Seat
Session

Connection State
Control State
control_epoch

Current Hand
Hand Version

Own Hole Cards
Community Cards

Main Pot
Side Pots

Table Stack
Committed This Hand

Action Timeline

Current Actor
Legal Action Set
Action Deadline / Remaining Time

Sit Out
Leave After Hand
Remove After Hand
Pending Top-up
Rebuy Window
```

---

# 453. Poker Service Restart Recovery

Startup：

```text
find non-closed Tables
find Active Sessions
find nonterminal Hands
```

每 Table：

```text
lock Table
runtime_epoch++
mark RECOVERING

load:
  Table
  Seats
  Sessions
  Hand
  Players
  Actions
  Stack
  Commitments
  Deck
  Pots
  Boundary Operations
```

重建 Actor。

Redis Cache 后重建。

然后进入 Recovery Grace。

---

# 454. Recovery Timer

采用 TD-08-C01：

```text
Service Recovery
→ 30s Reconnect Grace
→ if current player decision exists:
     new full 30s Action Window
→ Resume
```

不会使用故障前已经过期的 Deadline 立即 Auto Fold。

---

# 455. Client Seed Contributions

每 Seat 保存：

```text
next_client_seed_contribution
version
```

Hand COMMITTED：

```text
freeze current contribution
```

之后修改只影响下一 Hand。

---

# 456. Effective Client Seed

至少包含：

```text
Table ID
Hand ID
all participant contributions
fixed canonical ordering
```

推荐 Canonical Ordering：

```text
seat_no ascending
```

Effective Seed：

```text
Hash(
  canonical(
    table_id,
    hand_id,
    [(seat_no, contribution)...]
  )
)
```

Exact Encoding 进入 Implementation Spec。

---

# 457. Poker Server Seed / Deck

每 Hand：

```text
Server Seed
CSPRNG
>=256-bit entropy
```

保存：

```text
server_seed_hash
encrypted_server_seed
```

完整 52-card Deck：

```text
deterministically generated once
```

推荐：

```text
HMAC-SHA-256 deterministic stream
+
unbiased Fisher-Yates
```

---

# 458. Deck Authority

保存：

```text
deck_version
deck_hash
encrypted/reproducible deck authority
next_deck_index
```

每张 Deal：

```text
deck_index
card_instance
recipient
public_visibility
```

不能在需要牌时临时重新调用随机数。

---

# 459. Private Card / Seed Logging Boundary

未公开：

```text
Hole Cards
Server Seed
Future Deck
```

不得进入：

```text
ordinary logs
traces
analytics
operations search
error payload
```

Host / Operator / Super Admin 均无提前查看权限。

---

# 460. Fairness Reveal Timeline

Hand Settlement 后立即公共：

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
Complete 52-card Deck
Verification Data
```

只在：

```text
settled_at + 24h
```

以后，向 Durable Hand Participant Set 开放。

Spectator / Public 永不获得完整 Seed。

---

# 461. Fairness Release Worker

Hand 保存：

```text
full_fairness_reveal_at
=
settled_at + 24h
```

Worker 只控制：

```text
release availability
```

不修改历史。

请求者必须：

```text
∈ durable Hand participant set
```

不是依靠当前 Seat / 当前 Table Membership。

---

# 462. Spectator

Spectator：

```text
real-time
no artificial delay

no Poker Session
no Stack
no Control
```

可见：

```text
Master
Seat
Public Stack
Button
Blind / Ante
Public Action
Board
Pots
Public Showdown
Chat
```

不可见：

```text
Private Hole Cards
Folded private cards
raw Client Seed Contribution
Future Deck
Full Server Seed
Private Wallet
```

---

# 463. Spectator vs Active Session

用户已有 Active Session：

```text
cannot spectate another Table
```

Server 必须验证，不能只隐藏 UI。

---

# 464. Table Chat

只支持：

```text
USER_TEXT
SYSTEM
```

不支持：

```text
image
file
voice
DM
global chat
```

Durable：

```text
poker.chat_messages
```

至少：

```text
message_id
table_id
sender
message_type
text
moderation_state
created_at
hidden_at nullable
```

---

# 465. Chat Moderation

Host / Operations：

```text
Mute
Hide according to scope
```

但：

```text
original message remains durable
moderation operation audited
```

Local Block：

```text
client preference
```

不等于服务端删除。

---

# 466. Host Permissions

Host 可以：

```text
Pause accepting players
Resume accepting players
Remove Player After Hand
Remove Spectator
Mute Chat User
Close Table
```

Host 不得：

```text
view hidden Hole Cards
change Deck
change Pot
change Stack
change Winner
change locked Blind / Buy-in
confiscate Chips
skip Cash Out
re-settle Hand
```

---

# 467. Host Close Table

```text
OPEN
→ CLOSING

accepting_players = false
allow_new_hands = false
```

若 Active Hand：

```text
finish / recover same Hand
```

之后所有 Seat：

```text
Safe Leave
→ Cash Out
```

只有：

```text
no Poker In Play
no pending funding
no unsettled Hand
```

才：

```text
CLOSED
```

---

# 468. Operations Boundary

Operations 允许：

```text
Stop / Resume Accepting Players
Stop / Resume New Hands
Close After Current Hand
Remove After Hand
Remove Spectator
Mute
Pause Table
Request Recovery
```

Emergency Pause：

```text
Super Admin
Fresh Auth
Reason
```

绝不允许：

```text
edit Stack
edit Pot
choose Winner
edit Deck
edit Settlement
force current-Hand Cash Out
peek Hole Cards
peek unrevealed Seed
```

---

# 469. Poker Maintenance

Global Poker Maintenance：

```text
No new Table
No new Seat
No new Buy-in
No new Hand
```

Active Hand：

```text
prefer normal finish
or
Paused / Recovery
```

不得因 Maintenance 直接 Delete / Refund。

关闭前必须：

```text
all Cash Outs finished
or
explicit unresolved incident
```

---

# 470. Hand History

V1 使用：

```text
Action Timeline
```

不做动画 Replay。

至少：

```text
Hand ID
Table / Rules Version

Seat / Button
Blind / Ante

Starting Stacks

authorized Hole Cards
Community Cards

Action Timeline

Main / Side Pots
Eligible Players

Showdown
Pot Winners
Odd Chip

Settlement
Ending Stacks

Algorithm / Config
Fairness
```

---

# 471. Session History

至少：

```text
Session ID

Table ID / Name
Access Type

Blind / Ante

Start / End

Initial Buy-in
Top-ups / Rebuys

Final Cash Out
Realized P/L

Hand Count
Hand List

End Reason
```

End Reason 例如：

```text
SAFE_LEAVE
AUTO_CASHOUT
KICKED
TABLE_CLOSED
BUST_NO_REBUY
```

---

# 472. Realtime Action Sequence

```text
Browser
→ WS command
   action_id / expected_hand_version / control_epoch

WS Gateway
→ authenticate controller

Table Actor
→ DB Transaction

PostgreSQL
→ Action
→ Stack / Commitment
→ Hand Version
→ COMMIT

Table Actor
→ viewer-specific WS events
→ public spectator events
→ safe lobby delta
```

---

# 473. Hand Start Sequence

```text
Intermission ends
↓
Select eligible Seats
↓
Hand Commitment TX
  freeze participants
  freeze config
  freeze seed contributions
  create Server Seed Hash
  lock full Deck
↓ COMMIT
Broadcast HAND_COMMITTED
↓
Forced Bet / Deal TX
  post Ante / Blind
  deal Hole Cards
  enter Preflop
  start 30s timer
↓ COMMIT
Broadcast viewer-specific state
```

---

# 474. Safe Leave Sequence

```text
Client Leave
↓
Table Actor
├─ not in Hand
│   → Atomic Cash Out
└─ in Hand
    → Leave After Hand
    → continue Hand
    → unique Hand Settlement
    → Atomic Cash Out
    → Session Settlement
```

---

# 475. Crash Point Analysis

| Crash Point | Durable Fact | Recovery |
|---|---|---|
| Seat Reservation only | Redis Reservation | Expire; no asset |
| Buy-in before Commit | No Session / Stack / debit | Retry |
| Buy-in committed, response lost | Session + Stack + Ledger | Return same result |
| Hand COMMITTED before deal | Hash / Deck / participants | Resume forced bets/deal |
| Forced Bets committed | Commitment durable | Resume same Deck |
| Player Action committed, broadcast lost | Action / Stack / Version durable | Snapshot |
| Timer vs User Action | DB ordering | Exactly one Action |
| Service crash during action | Hand durable | Pause → Recover → Grace |
| Street transition crash | Deck index / Action durable | Reconstruct |
| Settlement before Commit | No final settlement | Deterministic rerun |
| Settlement Commit, broadcast lost | Awards / Stack / Settled | Snapshot |
| Pending Top-up crash | Pending op durable | Apply at legal boundary |
| Cash Out before Commit | Stack still Poker-owned | Retry |
| Cash Out Commit, response lost | Stack=0 + Wallet Ledger + Session settled | Return original |
| Fairness release worker crash | `reveal_at` durable | Resume worker |

---

# 476. Exactly-once / Delivery Matrix

| Area | Semantics |
|---|---|
| WS Frame | At-least-once / loss possible |
| Player Command | Retry allowed |
| Action ID | Effectively-once |
| Poker Action DB Effect | Exactly-once committed |
| Hand Settlement | Exactly-once |
| Pot Award | Exactly-once |
| Buy-in | Exactly-once |
| Top-up | Exactly-once |
| Cash Out | Exactly-once |
| Safe Leave | Idempotent |
| Take Over | Atomic epoch transition |
| Fairness Release | Idempotent |

---

# 477. Poker Metrics

至少：

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

poker_hand_duration
poker_hand_recovery_count
poker_paused_tables
oldest_recovering_hand_age

poker_side_pot_count
poker_settlement_failure
poker_duplicate_action_conflict

poker_buyin_failure
poker_topup_failure
poker_cashout_failure

poker_fairness_release_lag
poker_seed_verification_failure

poker_asset_conservation_failure
```

---

# 478. Critical Poker Invariants

以下立即 Critical：

```text
negative Table Stack

duplicate Hand Settlement

same Chip in multiple Pot layers

Pot Awards
!=
Settled Pot Total

Poker Asset Conservation mismatch

duplicate Cash Out

Deck index reused

Hole Card sent to unauthorized viewer

Server Seed leaked before reveal boundary

two Active Control Connections same epoch

Redis treated as durable truth
```

---

# 479. TD-08 Test Gate

## Actor / Concurrency

```text
100 concurrent commands same Table
→ deterministic serial ordering

duplicate action_id
→ one effect

two actions same hand_version
→ one success / one stale

old runtime_epoch
→ rejected
```

## Funding

```text
duplicate Buy-in
duplicate Top-up
duplicate Cash Out

crash before / after commit

Wallet / Stack atomicity
```

## Timer

```text
30s Server Timer
User Action vs Timeout Race

Check legal
→ Auto Check

Check illegal
→ Auto Fold

2 consecutive timeout
→ Next Hand Sit Out

manual success
→ timeout streak reset
```

## Disconnect

```text
socket loss
→ Timer continues

Reconnect before deadline
→ same action state

Reconnect after timeout
→ actual auto action visible
```

## Service Failure

```text
Restart
→ no timeout during outage
→ PG Recovery
→ 30s Grace
→ fresh 30s current action window
```

## Take Over

```text
Device A controller
Device B Take Over

control_epoch++
A rejected
B accepted
```

## Poker Rules

Property Tests：

```text
Button / Blind Order
Heads-up Order

Full Raise
Short All-in Reopening

Side Pot Layers
Eligible Players
Fold Contribution

Split Pot
Odd Chip
All-in Runout
```

## Asset Conservation

大量随机合法 Hand：

```text
Starting Poker Assets
=
Ending Poker Assets
```

Rake = 0。

## Settlement

```text
100 duplicate Settlement attempts
→ one final Settlement

REFUNDED
→ cannot SETTLE later
```

## Recovery

在每个 Action Sequence 强制 Crash：

```text
restart
→ same Stack
→ same Commitment
→ same Board
→ same Hole Cards
→ same next Actor
```

## Privacy

自动测试 Viewer：

```text
own player
other player
spectator
host
operator
auditor
```

必须保证：

```text
no unauthorized Hole Card
no unrevealed Seed
no future Deck
```

## Fairness

```text
Seed Hash match
Effective Client Seed deterministic
same input → same 52-card Deck
no duplicate cards
unbiased shuffle vectors
24h release authorization
spectator cannot obtain full Seed
```

---

# 480. Poker Product Gaps Register

| ID | Missing Product Rule | Status |
|---|---|---|
| POKER-PROD-GAP-01 | Ante = 10% BB 时采用 Per-player Ante 还是 Big Blind Ante。 | OPEN |
| POKER-PROD-GAP-02 | Post Big Blind Now 的 Live / Dead Blind 以及完整 betting-right 语义。 | OPEN |
| POKER-PROD-GAP-03 | Table 第一手 Initial Dealer Button 如何确定。 | OPEN |
| POKER-PROD-GAP-04 | Hand Evaluator 的 Wheel、Suit Tie、完整 Kicker / Tie-break 边界。 | OPEN |
| POKER-PROD-GAP-05 | Raise 场景下 1/2 Pot、2/3 Pot、Pot 快捷金额的精确 Target Raise-To 公式。 | OPEN |

以上进入后续 Technical Final Audit / Implementation Spec 前补齐，禁止未经用户/上游确认自动写成冻结规则。

---

# 481. TD-08 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| TD-FRZ-229 | Poker Service 使用 Single-writer Table Actor / Command Mailbox 串行化每桌实时操作；PostgreSQL 仍是正式 Authority。 | FROZEN |
| TD-FRZ-230 | Actor 必须 Commit-before-Broadcast；内存状态仅为 Projection，任何正式 Action / Stack / Pot / Settlement 先持久化再发送 WS。 | FROZEN |
| TD-FRZ-231 | 每桌维护 Durable `runtime_epoch` 防止旧 Actor / Recovery Actor Split-brain；Redis Lock 不作为唯一 Fencing Authority。 | FROZEN |
| TD-FRZ-232 | Redis 只承载 Connection、Seat Reservation、Table Access Grant、Lobby Delta 与短 Event Buffer；未结算 Poker 资产和 Hand 状态不得 Redis-only。 | FROZEN |
| TD-FRZ-233 | Table / Seat / Session / Hand / Action / Pot / Fairness / Chat 使用独立 Durable Object，不合并成单一 Room JSON。 | FROZEN |
| TD-FRZ-234 | Table 使用 CREATED / WAITING / IN_HAND / INTERMISSION / PAUSED / RECOVERING / CLOSING / CLOSED 等生命周期，并把 Accepting Players / Allow New Hands 作为独立 Gate。 | FROZEN |
| TD-FRZ-235 | Password Table 仅保存安全 Hash，密码不进普通日志；验证成功产生 Session-bound Ephemeral Table Access Grant，Redis 丢失最多要求重新验证密码。 | FROZEN |
| TD-FRZ-236 | Seat Reservation 使用 Redis 30s Ephemeral Lock，但正式占座只在 Buy-in PG Transaction 后成立，最终依靠 DB Unique / Row Lock 防双占座。 | FROZEN |
| TD-FRZ-237 | Poker Session 从成功 Buy-in 开始，到 Safe Leave / Cash Out 完成；数据库强制每用户最多一个 Active Poker Session。 | FROZEN |
| TD-FRZ-238 | 每用户最多拥有一张 Non-closed Owned Table；Create Table 本身不创建 Seat / Active Session。 | FROZEN |
| TD-FRZ-239 | 第一笔成功 Buy-in 原子锁定 Table 的 Max Seats、Blind/Ante Preset、Buy-in Bounds、Spectator 与正式经济配置。 | FROZEN |
| TD-FRZ-240 | Blind / Ante 使用版本化 Preset，每 Hand 锁定 Preset / Ruleset / Algorithm / Config / Hash；Ante Posting Mode 当前保持 `PRODUCT RULE NOT FOUND`。 | FROZEN |
| TD-FRZ-241 | Poker 所有正式金额使用整数 Chip，底层继续使用 Atomic Units，并强制 500,000 units 整倍数。 | FROZEN |
| TD-FRZ-242 | Buy-in 使用 TD-04 同库 Atomic Funding Contract，一次事务完成 Wallet Debit、Ledger、Session、Seat 与 Table Stack。 | FROZEN |
| TD-FRZ-243 | 加入运行中 Table 使用 WAIT_FOR_BB / POST_BB_NOW Boundary State，新玩家永远不能 Hand 中途获得 Hole Cards；Post BB Now 的 Live/Dead 细节继续显式待产品确认。 | FROZEN |
| TD-FRZ-244 | 每 Hand 之间固定 5s Intermission，Boundary 顺序处理 Safe Leave、Sit Out、Top-up/Rebuy、新 Seat 与下一 Hand 准备。 | FROZEN |
| TD-FRZ-245 | Top-up 只在 Hand Boundary 生效并重新验证 Wallet/100BB 上限；失败不产生 Partial Effect，赢得 Stack >100BB 不强制降低。 | FROZEN |
| TD-FRZ-246 | Stack=0 进入 60s Rebuy Window；成功 Rebuy 继续原 Session，超时 Cash Out 0 + Session Settlement；V1 无 Auto Rebuy。 | FROZEN |
| TD-FRZ-247 | Hand Start 使用两阶段 Commit：先锁参与者/版本/Seed Hash/完整 Deck，再单独 Posting Forced Bets + Deal，确保 Fairness Commitment 先于发牌。 | FROZEN |
| TD-FRZ-248 | Hand 使用 COMMITTED / POSTING_FORCED_BETS / DEALING_HOLE_CARDS / PREFLOP / FLOP / TURN / RIVER / SHOWDOWN / SETTLING / SETTLED，并支持 PAUSED / RECOVERING / REFUNDING / REFUNDED。 | FROZEN |
| TD-FRZ-249 | Hand Participant / Starting Stack / Client Seed Contribution / Hole Card Index 在 Hand Commitment 时锁定，Disconnect / Take Over / Profile Change 不改变参与集合。 | FROZEN |
| TD-FRZ-250 | Dealer Button 后续每 Hand顺时针移动并严格使用已冻结 Heads-up / 3+ 行动顺序；第一手 Initial Button Rule 保持 `PRODUCT RULE NOT FOUND`。 | FROZEN |
| TD-FRZ-251 | Player Action 由 Table Actor + PostgreSQL 执行，使用 `action_id / action_sequence / expected_hand_version / control_epoch`；客户端不得决定 Legal Action Set。 | FROZEN |
| TD-FRZ-252 | Full Raise 使用 Previous Full Raise Increment；Short All-in 不增加 Full Raise Sequence，因此不重新开放已经行动玩家 Raise 权。 | FROZEN |
| TD-FRZ-253 | Min / 1/2 Pot / 2/3 Pot / Pot / All-in 全由服务端产生与校验；精确 Pot Shortcut Raise Formula 当前保持 `PRODUCT RULE NOT FOUND`。 | FROZEN |
| TD-FRZ-254 | 每个玩家决策使用 PostgreSQL 持久化的 30s Server Deadline；Timeout System Action 使用稳定 ID，与人工 Action 通过 DB Lock / Version Race 保证只能一个生效。 | FROZEN |
| TD-FRZ-255 | Timeout 时可 Check 则 Auto Check，否则 Auto Fold；连续两次 Timeout 后下一 Hand 自动 Sit Out，成功人工操作重置连续 Timeout 计数。 | FROZEN |
| TD-FRZ-256 | Client Disconnect 与 Poker Service Failure 是两类不同状态；前者 Timer 继续，后者 Timer 语义暂停并走 PostgreSQL Recovery。 | FROZEN |
| TD-FRZ-257 | TD-08-C01：Poker Service Failure 恢复后先给 30s Reconnect Grace；Grace 后若仍需当前玩家决策，为其创建新的完整 30s Action Window。 | FROZEN |
| TD-FRZ-258 | Disconnect 中当前 Hand 继续；若到 Hand Boundary 仍断线则 Sit Out，若 Boundary 前成功 Reconnect 则清除仅由本次 Disconnect 引起的 Next-hand Sit-out。 | FROZEN |
| TD-FRZ-259 | Continuous Sit Out / Disconnect ≥15min 且无未结算 Hand 时触发幂等 Auto Safe Leave / Cash Out。 | FROZEN |
| TD-FRZ-260 | V1 不提供 Time Bank、高级 Pre-action、Auto Call、Auto Bet、Auto Raise 或策略预设；全部 All-in 无决策后自动 Runout。 | FROZEN |
| TD-FRZ-261 | Poker 通过 per-player Street / Total Commitment 构建确定性 Main / Side Pot，Fold Player 的贡献进入 Pot 但不进入 Eligible Winner Set。 | FROZEN |
| TD-FRZ-262 | TD-08-C02：无人匹配的最顶层 Uncalled Excess 在 Pot Settlement 前原子退回原玩家 Table Stack，并记录 System Action，不作为有效 Pot Award / Ownership Transfer。 | FROZEN |
| TD-FRZ-263 | Pot / Eligible Player / Award 使用 Durable Tables；同一筹码不能重复进入多个 Pot，同一 Pot 只能 Settlement 一次。 | FROZEN |
| TD-FRZ-264 | Poker Hand Evaluator 必须版本化并输出可比较 Rank Vector；当前缺失的 Wheel / Suit / Kicker 边界在 Canonical Ruleset Test Vector 确认前不得假装已冻结。 | FROZEN |
| TD-FRZ-265 | 所有仍在局玩家 All-in 且无决策时自动公开这些玩家 Hole Cards、发完剩余 Board 并进入确定性 Showdown / Settlement。 | FROZEN |
| TD-FRZ-266 | 普通 Showdown 只公开确定 Pot Winner 必需的牌；Folded / Mucked Card 实时继续隐藏，V1 无主动 Show Folded Cards。 | FROZEN |
| TD-FRZ-267 | Poker Snapshot / Event 必须按 Viewer 服务端投影；绝不向 Browser / Spectator / Host 发送无权查看的 Hole Cards 再依赖前端隐藏。 | FROZEN |
| TD-FRZ-268 | Hand Settlement 使用稳定 `poker_hand_settlement:{hand_id}` 等价 Biz ID，在同一 PG Transaction 完成 Pot Build、Winner、Odd Chip、Table Stack Award 与 Hand SETTLED。 | FROZEN |
| TD-FRZ-269 | Odd Chip 固定给 Dealer Button 左侧顺时针第一位对该 Pot 有资格的并列 Winner，并持久化至 Hand History。 | FROZEN |
| TD-FRZ-270 | Poker V1 无 Rake；每 Hand 强制资产守恒，Total Commitments = Pot Awards + Returned Uncalled Excess，不允许 Issuance / Burn。 | FROZEN |
| TD-FRZ-271 | TD-08-C03：无法合法恢复 Hand 时的自动确定性 Refund 仅允许恢复所有参与者至各自 Hand-start Stack；无法证明完整来源时进入 NEEDS_REVIEW，不猜余额。 | FROZEN |
| TD-FRZ-272 | Safe Leave 统一覆盖 Return Lobby / Leave Table / Browser Back / Host Removal / Auto Leave；Hand 中使用 Leave After Hand，Fold 后也等待 Settlement。 | FROZEN |
| TD-FRZ-273 | Cash Out 与 Table Stack 清零、Wallet Credit、Ledger、Session Settlement 同事务，使用稳定 Session Biz ID；重复请求返回原结果且不允许 Partial Cash Out。 | FROZEN |
| TD-FRZ-274 | Session Realized P/L 只在最终 Cash Out 计算为 Final Cash Out − 全部 Buy-in / Top-up；未 Cash Out 的 Session Delta 仅为 Unrealized。 | FROZEN |
| TD-FRZ-275 | Poker Hand 指标可以先 Durable 保存，但 Poker Profit / Biggest Win / Total Wagered 的正式排名输入只在 Parent Session Cash Out 后提交。 | FROZEN |
| TD-FRZ-276 | Poker WS 使用固定 `chaldea-poker.v1` Subprotocol，单次 Connect Ticket 在 Upgrade 后第一个 Auth Frame 中传输；Secret 不放 URL Query。 | FROZEN |
| TD-FRZ-277 | WS 使用 Versioned Command / Event Envelope、Table/Hand Version 与 Event Sequence；WS Delivery 可重放/丢失，业务 Authority 通过 Snapshot + Idempotency 收敛。 | FROZEN |
| TD-FRZ-278 | 所有正式状态 Commit-before-Broadcast；Commit 后 Broadcast 丢失通过 Authoritative Snapshot 恢复，不重新执行 Command。 | FROZEN |
| TD-FRZ-279 | Slow WS Client 使用有界 Send Queue，过慢则断开并要求 Snapshot Reconnect，不允许无限缓存拖垮 Table Actor。 | FROZEN |
| TD-FRZ-280 | Active Control Connection Lease 是 Ephemeral，但 `control_epoch` Durable；Take Over 原子递增 Epoch，旧 Connection 变 Read-only 并失去行动权限。 | FROZEN |
| TD-FRZ-281 | Reconnect Snapshot 必须包含 Table/Seat/Session/Hand、自己的 Hole Cards、Board、Pot、Stack/Committed、Action History、Current Actor/Timer、Sit-out/Leave/Top-up 等正式状态。 | FROZEN |
| TD-FRZ-282 | Poker Service Restart 从 PostgreSQL 枚举 Non-closed Table / Active Session / Nonterminal Hand，递增 Runtime Epoch 重建 Actor；Redis 只重建 Cache。 | FROZEN |
| TD-FRZ-283 | Effective Client Seed 使用 Hand Participant Contributions 的固定 Canonical Order；推荐 `seat_no` 升序，精确 Encoding 在 Implementation Spec 固定。 | FROZEN |
| TD-FRZ-284 | 每 Hand 使用独立安全 Server Seed 与一次性确定性 52-card Deck；未公开 Seed / Deck 加密保存且永不进入普通 Log / Trace。 | FROZEN |
| TD-FRZ-285 | 完整 Server Seed / Effective Client Seed 组合 / 52-card Deck 只在 Settlement 24h 后向 Durable Participant Set 开放；Spectator / Public 永不获得完整 Seed。 | FROZEN |
| TD-FRZ-286 | Spectator 使用实时 Public Projection、不创建 Active Session；已有另一桌 Active Session 的用户不得观战其他桌。 | FROZEN |
| TD-FRZ-287 | Table Chat 仅支持文字 / System Message，并 Durable 保存原记录；Mute / Hide 不删除历史，Local Block 不是服务端删除。 | FROZEN |
| TD-FRZ-288 | TD-08 Implementation 必须通过 Actor Concurrency、Action / Timer Race、Side Pot / Odd Chip / Asset Conservation、Funding Exactly-once、Disconnect / Restart / Take Over、Fairness / Privacy 和 Crash-point Property Tests。 | FROZEN |

---

# 482. Change Log — WORKING v0.8

## Added

- 用户正式确认 TD-08；
- 冻结 `TD-FRZ-229 ～ TD-FRZ-288`；
- 冻结 Single-writer Table Actor；
- 冻结 PostgreSQL Poker Durable Authority；
- 冻结 Runtime Epoch Fencing；
- 冻结 Redis Ephemeral Boundary；
- 冻结 Table / Seat / Session / Hand / Action / Pot Durable Model；
- 冻结 Password Table Access Grant；
- 冻结 Seat Reservation；
- 冻结 Buy-in / Top-up / Rebuy / Cash Out；
- 冻结 5s Hand Intermission；
- 冻结 Two-phase Hand Start；
- 冻结 Full Raise / Short All-in；
- 冻结 Action Timer / Timeout Race；
- 冻结 Client Disconnect / Service Failure 分离；
- 冻结 `TD-08-C01` 30s Grace + Fresh Action Window；
- 冻结 deterministic Side Pot；
- 冻结 `TD-08-C02` Uncalled Excess Return；
- 冻结 Viewer-specific State Projection；
- 冻结 Hand Settlement Exactly-once；
- 冻结 Odd Chip；
- 冻结 Poker Zero-sum；
- 冻结 `TD-08-C03` Catastrophic Hand Refund；
- 冻结 Safe Leave / Session Settlement；
- 冻结 WS Auth / Envelope / Snapshot / Backpressure；
- 冻结 Take Over / Control Epoch；
- 冻结 Service Restart Recovery；
- 冻结 Poker Fairness / 24h Participant-only Reveal；
- 冻结 Spectator / Chat / Host / Operations Boundary；
- 冻结 Poker Crash / Property / Privacy Test Gate；
- 新增 Poker Product Gaps Register。

## Explicitly Still Open

```text
POKER-PROD-GAP-01 Ante Posting Mode
POKER-PROD-GAP-02 Post BB Now Live/Dead Semantics
POKER-PROD-GAP-03 Initial Dealer Button Rule
POKER-PROD-GAP-04 Hand Evaluator Edge/Tie Rules
POKER-PROD-GAP-05 Pot Shortcut Exact Raise Formula
```

---

# 483. 下一批 — TD-09

下一批正式进入：

> **TD-09 — Rankings / History / Announcements / Jobs**

计划冻结：

1. Rankings Source of Truth；
2. Assets & Games Ranking Aggregation；
3. Poker Cash-out Commit Point；
4. Direct Play Round Aggregation；
5. Biggest Win Source；
6. Total Wagered；
7. Poker Profit；
8. RP Usage Aggregation；
9. Logical Request Identity；
10. Key Purpose Snapshot；
11. Model Aggregation；
12. Today / Week / All-time；
13. Asia/Shanghai Period；
14. Historical Snapshot；
15. Tie Ranking；
16. Feature Activation Time；
17. 5min Freshness Target；
18. Ranking Source Exclusion；
19. Shadow Rebuild；
20. Publish Snapshot；
21. Game History Unified Record Projection；
22. Session / Hand Parent-child；
23. Retired Game / Model History；
24. Public Recent Wins / Featured Records；
25. Announcement Revisions；
26. Entry Popup Arbitration；
27. Schedule / Publish / Expire；
28. Acknowledgement；
29. Notification Revision / Re-notify；
30. Background Jobs Framework；
31. Job Idempotency；
32. Job Lease / `SKIP LOCKED`；
33. Retry / Backoff；
34. Dead Letter / Needs Attention；
35. Maintenance / Scheduling Interaction；
36. Audit；
37. Metrics / Test Gate。

注意：

- Ranking 不重新计算经济事实，只聚合 Durable Source；
- 管理员不能直接改分数；
- Poker 数据必须等 Session Cash Out 才成为正式排行榜输入；
- RP Usage 必须使用冻结 `key_purpose_snapshot` / logical request 口径；
- History 记录默认只读；
- Announcement 与 Notification Revision 必须区分；
- Jobs 不能成为无审计的“后台脚本垃圾桶”。



---

# 484. TD-09 — Rankings / History / Announcements / Jobs

> 状态：`FROZEN`  
> 用户确认：`整体按上述方案通过`

## 484.1 TD-09 总体结论

本批正式冻结：

- Rankings Durable Source → Derived Facts → Aggregate Set → Published View；
- Ranking Projection 不成为业务第二 Authority；
- Direct Play / Poker / RP Ranking Source；
- Poker Cash-out Ranking Commit Point；
- Total Assets Current Snapshot；
- Asia/Shanghai Period Engine；
- RP Feature Activation Time；
- Logical Request / Key Purpose Snapshot / Model Attribution；
- Versioned Aggregate Set；
- Tie Ranking；
- Routine Aggregation；
- Shadow Rebuild / Diff / Review / Publish；
- Source Exclusion；
- Historical Snapshot；
- Rankings Publishing Maintenance；
- Unified Game History Read Index；
- Round / Session / Hand Detail Source Authority；
- Recent Public Wins Safe Projection；
- Announcement Identity / Content Version / Notification Revision；
- Update Content Only / Re-notify；
- Announcement Lifecycle / Time / Placement；
- Entry Popup / Post-login Popup / Read / Dismissal；
- Acknowledgements；
- Markdown / Rich Text Sanitization；
- Announcement Scheduler；
- PostgreSQL-backed Durable Job Framework；
- Job Allowlist / Lease / Retry / Attempts / Maintenance；
- Crash Analysis / Test Gate。

本批明确保留：

```text
TD-09-PROD-GAP-01
Recent Public Wins / Featured Records
合格门槛、数量与精选算法尚未冻结
```

规则未确认时，对应模块直接隐藏，不生成虚假内容。

---

# 485. Ranking Authority Model

Rankings 只从正式 Durable Source 聚合。

| Ranking | Durable Source |
|---|---|
| Total Assets | TD-04 Fresh Asset Authorities |
| Direct Play Profit | Final `games.game_rounds` Settlement |
| Direct Play Wagered | Round `total_stake_units` |
| Direct Play Biggest Win | Round `net_change_units` |
| Poker Profit | Settled Poker Session |
| Poker Biggest Win | Settled Poker Hand, released after Parent Session Cash Out |
| Poker Wagered | Actual Hand Pot contributions, released after Parent Session Cash Out |
| RP Calls / Errors / Credits | Finalized API Request Attribution |

Ranking 表不允许作为可手工修改的 Score Authority。

---

# 486. Ranking Pipeline

```text
Authoritative Domain Sources
│
├── Direct Play Round
├── Poker Session / Hand
├── Current Asset Authorities
└── API Request Attribution
        │
        ▼
Incremental Source Scanner
        │
        ▼
Derived Ranking Facts
        │
        ▼
Aggregate Builder
        │
        ├── Current Period
        ├── Closed Historical Period
        └── Shadow Rebuild
        │
        ▼
Published Aggregate Set
        │
        ▼
/rankings
Public Home Preview
Operations Preview
```

V1 使用：

```text
PostgreSQL
+
Background Worker
```

不引入 Kafka。

---

# 487. Derived Ranking Facts

建议：

```text
ranking.source_facts
```

逻辑：

```text
fact_id UUIDv7
source_type
source_id
source_version
newapi_user_id
metric_family
game_slug nullable
model_id nullable
event_at
value_units nullable
value_count nullable
identity_snapshot_id nullable
eligibility_state
created_at
```

等价 Unique：

```text
UNIQUE(source_type, source_id, metric_family, dimension_key)
```

该表是 `Rebuildable Projection`，不是最终业务 Authority。Full Rebuild 必须能直接重新读取 Round / Session / Hand / API Request Attribution。

---

# 488. Ranking Ingestion Cursor

使用：

```text
ranking.ingestion_cursors
```

按 Source 保存 `(timestamp, stable source_id)` 或等价 Cursor。

Worker：

```text
read committed sources
→ insert derived facts idempotently
→ advance cursor
```

如果 Fact 已插入但 Cursor 未推进就崩溃，下次重扫由 DB Unique 去重。

---

# 489. Direct Play Ranking Facts

最终 `SETTLED` Round 至少产生：

```text
DIRECT_PLAY_PROFIT = net_change_units
DIRECT_PLAY_WAGERED = total_stake_units
```

若 `net_change_units > 0`，同时成为 `BIGGEST_WIN` Candidate。

Refunded Round：

```text
profit contribution = 0
wagered contribution = 0
biggest-win contribution = none
```

---

# 490. Poker Ranking Eligibility

Hand Settlement 可以先保存：

```text
hand_net_change
hand_total_wagered
ranking_eligibility = HELD
```

直到 Parent Session `Cash Out → SETTLED` 后才变为 `ELIGIBLE`。

未 Cash Out 的 Poker 只允许显示 Unrealized Session Delta，不进入正式公共排名。

---

# 491. Poker Profit

```text
Poker Profit = Poker Session Realized P/L
```

事件时间使用 `cashout_at`，日 / 周归属按 Cash Out Time。

---

# 492. Game Profit

```text
Game Profit
=
Σ Direct Play settled net_change
+
Σ Poker settled Session realized_pl
```

排除：

```text
Reward
Initial Grant
Admin Adjustment
Exchange
Poker Buy-in
Poker Cash Out
Poker internal Stack movement
```

---

# 493. Biggest Win

候选：

```text
Direct Play Round Net > 0
Poker Hand Net > 0
```

Poker Biggest Win 使用该玩家在该 Hand 的 positive net profit，不使用 Pot Size，并且 Parent Session 必须已经 Cash Out。

---

# 494. Total Wagered

Direct Play 使用 Round `total_stake_units`。

Poker 使用实际：

```text
Blind
Ante
Call
Bet
Raise
All-in
```

同一 Poker Chip 投入只统计一次。TD-08 `RETURN_UNCALLED` 不计为完成 Ownership Transfer 的正式 Wager。

---

# 495. Total Assets Ranking

Total Assets 只做 Current Snapshot，不做历史某日资产回放。

使用：

```text
ranking.current_asset_snapshots
```

而不是历史 Event Sum。

---

# 496. Total Assets Completeness Gate

Snapshot 必须完整读取：

```text
Reserve API Credit
Active NewAPI Quota
Available Chips
Poker In Play
Processing Assets
```

任一 Required Authority 不可用、过旧或经济事实不明确时禁止发布新 Snapshot。

行为：

```text
keep last complete published snapshot
mark STALE / DEGRADED
show Last Updated
```

Operations 展示 Data Completeness / Aggregation Lag / Source Error。

---

# 497. Ranking Period Engine

统一业务时区：

```text
Asia/Shanghai
```

Daily：`00:00 → next 00:00`

Weekly：`Monday 00:00 → next Monday 00:00`

All Time：从 feature-specific activation boundary 开始。

---

# 498. RP Feature Activation

保存：

```text
ranking_feature_activation.activation_at
```

只有 `request_at >= activation_at` 才进入 RP Ranking。

禁止通过旧日志推测历史 Key Purpose 后回填 RP 排名。

---

# 499. API Request Attribution

Ranking Aggregator 只消费 Finalized Attribution，逻辑至少：

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

实际 NewAPI Schema / Endpoint：

```text
SOURCE VERIFICATION REQUIRED
```

---

# 500. RP Logical Request

统计单位：

```text
logical_request_id
```

Platform Internal Retry 使用同一个 `logical_request_id`，只算一次；用户独立提交的请求分别统计。

---

# 501. RP Eligibility

只统计：

```text
key_purpose_snapshot = RP
```

且是真实 inference / generation request。

排除：

```text
model list
balance
health
auth
admin
```

General / Unclassified 不进入 RP Ranking。

---

# 502. RP Calls

```text
Calls = count(successful eligible RP logical requests)
```

不计 Provider / Channel Retry Attempt。

---

# 503. RP Errors

主排序使用 Error Count，同时展示 Error Rate / Total Attempts / Top Error Models。

计入有效调用流程后的：

```text
Timeout
429
Upstream Error
Platform Call Error
Stream Interruption
```

不计：

```text
invalid key probe
cancel before upstream
internal retry attempt count
```

Public 不显示 Raw Error。

---

# 504. RP Credits Consumed

使用：

```text
final actual settled API Credit
```

禁止使用 Estimated Cost / USD / Raw Quota。

Failed Request 只有实际产生扣费才计入。UI 最多显示 6 位小数并去尾零。

---

# 505. Model Aggregation

聚合：

```text
user + period + metric + model_id
```

All Models 使用 Sum；Model Filter 使用 stable Chaldea Model ID。

V1 用户量 10–50，不需要为每个 Model Filter 预生成独立榜单。

---

# 506. Retired Model History

Fact 保存 stable `model_id`。Historical Display Name 从版本化 Catalog 解析。

Retired Model 继续可查询历史 Ranking，不公开 Provider / Channel / Credential / Internal Route。

---

# 507. Ranking Aggregate Set

```text
ranking.aggregate_sets
```

至少：

```text
aggregate_set_id UUIDv7
domain
metric
period_type
period_start
period_end
version
status
source_watermark
data_completeness
built_at
published_at
aggregate_hash
previous_published_set_id nullable
```

状态：

```text
BUILDING
SHADOW
PUBLISHED
SUPERSEDED
FAILED
```

---

# 508. Ranking Aggregate Rows

例如：

```text
ranking.user_aggregates
```

至少：

```text
aggregate_set_id
newapi_user_id
model_id nullable
success_calls
error_count
attempt_count
credits_consumed_units
game_profit_units
total_wagered_units
poker_profit_units
biggest_win_units
biggest_win_source_id nullable
```

不提供人工 Score Editor。

---

# 509. Tie Ranking

使用 SQL `RANK()` 等价语义：

```text
100 → 1
90  → 2
90  → 2
80  → 4
```

禁止 Dense Rank 的 `1,2,2,3`。

---

# 510. Ranking Identity

Rankings 展示 Current Master Nickname / Avatar。用户改名后 Ranking Identity 随 Current Profile 变化，真实归属仍为 `newapi_user_id`。

Recent Wins / Featured Records 使用 event-time Identity Snapshot，不随改名重写。

---

# 511. My Rank

Public Ranking 匿名可读；登录用户额外获得 My Rank。

只有当前用户自己的 My Rank 可以进入 `/api/usage?purpose=rp...`，不能通过他人榜单行查看对方详细 API Usage。

---

# 512. Routine Aggregation

目标：

```text
public aggregation lag <= 5 minutes
```

系统显示真实 Last Updated / Source Watermark / Aggregation Lag。

---

# 513. Routine vs Repair Build

Routine：

```text
Incremental Source Scan
→ Candidate Aggregate
→ Automated Validation
→ Publish if complete
```

Repair：

```text
Select Period / Scope
→ Full Authoritative Rebuild
→ SHADOW
→ Diff
→ Human Review
→ Publish
```

---

# 514. Ranking Source Exclusion

```text
ranking.source_exclusions
```

至少：

```text
exclusion_id
source_type
source_id
reason
created_by
created_at
revoked_by nullable
revoked_at nullable
```

Exclusion 不删除 Source Record；Revoke 也是独立审计状态变化，之后 Rebuild。

---

# 515. Shadow Diff

至少报告：

```text
old source count
new source count
old user count
new user count
metric total delta
Top-N changes
Rank changes
newly included sources
newly excluded sources
data completeness
source watermark
```

Repair Diff 必须 Review。

---

# 516. Ranking Publish Pointer

```text
BEGIN
lock metric / period publish pointer
candidate = verified SHADOW
old PUBLISHED → SUPERSEDED
candidate → PUBLISHED
update published pointer
Audit
COMMIT
```

不能出现一部分读新 Snapshot、一部分读旧 Snapshot。

---

# 517. Historical Closed Snapshots

Closed Daily / Weekly Aggregate Set Immutable。

Repair 创建新 Aggregate Set Version 并切换 Published Pointer；旧 Published Version 保留 Audit。

---

# 518. Rankings Publishing Maintenance

Maintenance Scope = Rankings Publishing 时，可以继续 Source Ingestion / Shadow Build / Validation，但暂停 Published Pointer Swap。

Public 页面继续使用 Last Good Published Snapshot + Stale Last Updated。

---

# 519. Unified History Index

建立可重建：

```text
games.history_index
```

至少：

```text
record_type
source_id
parent_source_id nullable
newapi_user_id
game_slug
mode
result_class
display_status
occurred_at
ended_at nullable
identity_snapshot_id nullable
source_version
updated_at
```

它只服务 `/history` List / Filter / Search，不是业务 Authority。

---

# 520. History Detail Authority

Round Detail 读取：

```text
games.game_rounds
+ game-specific typed result
+ fairness
```

Session Detail 读取 Poker Session / Funding / Hand List。

Hand Detail 读取 Poker Hand / Actions / Pots / Awards / Fairness。

History Index 错误只能重建 Index，不能修改正式 Source。

---

# 521. History Default List

默认：

```text
Direct Play Round
Poker Session
```

Poker Hand 主要从 Session Detail → Hand List → Hand Detail，也允许高级 Record Type Filter 查询。

---

# 522. History Authorization

完整私人 History 仅 Record Owner 或 Authorized Records Scope 可读。

Records 默认只读，禁止直接修改：

```text
wager
result
seed
deck
payout
stack
settlement
```

异常：

```text
Create Incident
→ Economy / Poker Repair
```

Private Poker Data 继续遵守 Reveal Boundary。

---

# 523. History Retention / Cross-link

允许 Cross-link：

```text
Wallet Transaction
Provably Fair
Game Entry
Poker Session
Historical Poker Table Metadata
```

Retired Game、Closed Poker Table 历史继续保留。

V1 不提供公共完整个人 History Share、CSV Export、JSON Export。

---

# 524. Public Game Event Projection

Public Recent Wins / Featured Records 与 Private History 分离。

使用：

```text
content.public_game_events
```

至少：

```text
public_event_id
source_type
source_id
game_slug
identity_display_snapshot_id
wager_units
net_win_units
occurred_at
public_policy_version
```

不保存 Current Assets、Full Private History、Private Cards、API Details。

---

# 525. TD-09-PROD-GAP-01

当前未冻结：

```text
Recent Win minimum net
Big Win threshold
maximum displayed count
automatic selection formula
Featured Record promotion rule
```

因此：

```text
PUBLIC_RECORD_SELECTION_POLICY
= PRODUCT RULE NOT FOUND
```

规则确认前不虚构 Recent Wins；没有明确合格记录时模块隐藏。


---

# 526. Announcement Identity Layers

必须区分：

```text
announcement_id
content_version
notification_revision
```

三者不能混用。

---

# 527. Announcement ID

`announcement_id` 表示同一条逻辑公告。

长期维护的规范 Acknowledgements：

```text
same announcement_id
```

名单更新通过 Content Version 维护，不重复创建多条同名公告。

---

# 528. Content Version

任何正文或结构化内容变化：

```text
new immutable content_version
```

至少：

```text
content_version_id
announcement_id
version_no
title
body_markdown
sanitized_html
announcement_type
visibility_policy
sanitizer_policy_version
created_by
created_at
```

Published Historical Version 不直接 UPDATE。

---

# 529. Notification Revision

```text
notification_revision
```

只有 Re-notify 或明确创建新通知展示版本时增加。

Entry Popup 展示身份：

```text
announcement_id
+
notification_revision
```

---

# 530. Update Content Only

```text
content_version++
notification_revision unchanged
```

效果：

```text
Detail content updated
existing popup dismissal preserved
existing read-state semantics preserved
```

---

# 531. Re-notify

```text
notification_revision++
```

效果：

```text
Entry Popup eligible again
Logged-in current revision can become unread
```

必须经过 Impact Preview 与 Audit。

---

# 532. Announcement Lifecycle

主生命周期：

```text
DRAFT
SCHEDULED
PUBLISHED
EXPIRED
ARCHIVED
```

允许 `withdrawn_at` 作为审计化 Visibility Stop。

Withdraw 不硬删除版本或 Audit。

---

# 533. Announcement Time

业务输入：

```text
Asia/Shanghai
```

数据库：

```text
TIMESTAMPTZ UTC
```

字段：

```text
publish_at
visible_from
visible_until nullable
```

长期公告允许 `visible_until = NULL`。

---

# 534. Effective Visibility

用户侧有效可见必须同时满足：

```text
state = PUBLISHED
viewer allowed by visibility policy
visible_from <= DB now()
visible_until IS NULL OR DB now() < visible_until
withdrawn_at IS NULL
```

因此 Scheduler 故障也不能让已经过期的内容继续展示。

---

# 535. Announcement Placement

独立 Placement：

```text
PINNED_LIST
ENTRY_POPUP
POST_LOGIN_POPUP
PUBLIC_HOME_BANNER
DASHBOARD_SUMMARY
```

Pinned 不自动等于 Popup。

同一 Announcement 可以拥有多个 Placement。

---

# 536. Entry Popup Non-overlap

V1：

```text
at most one effective ENTRY_POPUP
at any instant
```

Schedule / Publish Transaction 必须序列化并检查时间窗口冲突。

冲突时拒绝，不由 Runtime 临时随机挑选一条。

---

# 537. Home Banner Non-overlap

V1 同一时点最多一条 Primary Home Banner。

同样执行 Placement Schedule Conflict Validation。

---

# 538. Entry Popup Trigger

只主动检查：

```text
anonymous first entry:
/
or
/login
```

普通 Entry Popup 不强制覆盖：

```text
/models/:model
/rankings
/announcements/:id
other intentional public deep links
```

内容加载失败：

```text
fail open
```

不能阻止 Login / Registration。

---

# 539. Anonymous Popup Dismissal

匿名用户 Browser Local State：

```text
announcement_id
+
notification_revision
```

不建立跨设备已读状态。

清除 Browser Storage / Incognito / New Browser 后可能再次展示。

---

# 540. Popup Dismissed vs Read

严格区分：

```text
POPUP_DISMISSED
!=
ANNOUNCEMENT_READ
```

关闭 Popup 不等于 Detail Read。

---

# 541. Logged-in Read State

建立：

```text
content.announcement_reads
```

Unique：

```text
(newapi_user_id, announcement_id, notification_revision)
```

打开 Detail 时标记当前 Notification Revision 已读。

新 Notification Revision 创建新的未读状态，多设备同步。

---

# 542. Post-login Popup Safety

顺序：

```text
Login
→ Master Initialization
→ Migration Notice
→ Return-to-Intent
→ optional Post-login Popup
```

如果当前 Critical Flow：

```text
Poker
Active Direct Play Round
Wallet Processing
```

普通 Post-login Popup 必须 defer。

延迟到 Dashboard 或下一个 Safe Normal Page。

严重安全 / 维护事件继续使用独立 Critical Notice。

---

# 543. Acknowledgements

规范长期公告：

```text
Type = ACKNOWLEDGEMENTS
Visibility = PUBLIC
Pinned = YES
Entry Popup = YES
Post-login Popup = NO
Home Banner = NO
visible_until = NULL
```

Dashboard Summary 可独立配置。

---

# 544. Structured Acknowledgement Entry

绑定 Content Version：

```text
display_name required
avatar_or_logo nullable
external_link nullable
acknowledgement_note nullable
group nullable
manual_order required
anonymous
```

---

# 545. Sponsor Privacy

禁止公开：

```text
payment account
transaction record
Discord User ID
email
payment screenshot
unconsented real identity
private contact data
```

公开真实身份、头像、Logo 或 External Link 前必须有同意依据。

---

# 546. Markdown / Rich Text Security

保存 Controlled Markdown Source，并生成 Sanitized Canonical HTML。

Save / Render 使用同一：

```text
sanitizer_policy_version
```

禁止：

```text
script
event handler
custom JS
arbitrary iframe
untrusted raw HTML
```

External Link 使用安全属性。

---

# 547. Announcement Scheduler

Scheduler 只负责 Durable Lifecycle Transition：

```text
SCHEDULED → PUBLISHED
PUBLISHED → EXPIRED
```

它不直接“推送 Popup”。

Popup Eligibility 每次根据：

```text
published revision
placement
visibility
time
dismissal/read/session state
```

计算。

---

# 548. Scheduler Idempotency

Publish：

```text
announcement:publish:{announcement_id}:{content_version}
```

Expire：

```text
announcement:expire:{announcement_id}:{notification_revision}
```

重复执行时查询 Target Durable State 并收敛原结果。

---

# 549. Announcements Scheduling Maintenance

Maintenance Scope = Announcements Scheduling 时暂停新的 Scheduled Publication Transition。

但 User Query 继续实时检查 `visible_until`，所以已经过期内容不能继续曝光。

Maintenance 解除后：

```text
if publish window still valid
→ publish

if visible window already ended
→ missed window / expired
```

不得把错过窗口的公告突然补弹给所有人。

---

# 550. Durable Background Job Framework

建立：

```text
ops.jobs
ops.job_runs
ops.job_schedules
```

关键业务 Job 不使用 Shell Script、Ad-hoc Cron 或 In-memory Goroutine 作为唯一执行事实。

---

# 551. Job Type Allowlist

`job_type` 只能来自代码注册，例如：

```text
RANKING_INCREMENTAL
RANKING_REBUILD

ANNOUNCEMENT_PUBLISH
ANNOUNCEMENT_EXPIRE

REWARD_RECOVERY

ECONOMY_RECONCILIATION

POKER_RECOVERY
POKER_FAIRNESS_RELEASE
```

Payload 必须 Schema-validated Typed Payload。

Operations 永不支持 Arbitrary Command Job。

---

# 552. Job State Machine

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

无法自动安全完成：

```text
→ NEEDS_ATTENTION
```

其他：

```text
CANCELLED
BLOCKED_MAINTENANCE
```

---

# 553. Job Idempotency

每 Job：

```text
job_id
job_type
dedupe_key
```

Unique：

```text
UNIQUE(job_type, dedupe_key)
```

Handler 本身仍必须 Target-idempotent。

---

# 554. Job Claim / Lease

Worker：

```text
BEGIN
SELECT due job
FOR UPDATE SKIP LOCKED

set:
  lease_owner
  lease_expires_at
  state = RUNNING
COMMIT
```

长 Job 使用 Heartbeat。

Worker Crash：

```text
lease expires
→ another worker resumes
```

Redis 不是 Job Queue Authority。

---

# 555. Target Effect Commit / Worker Crash

场景：

```text
target business effect COMMIT
→ worker crashes
→ job still RUNNING
```

Lease Recovery 后重试：

```text
handler queries target durable business identity
→ sees effect already done
→ marks SUCCEEDED
```

绝不重复资产、Publication 或 Settlement。

---

# 556. Job Retry

使用：

```text
exponential backoff
+
jitter
```

精确 Initial Delay / Max Delay / Max Automatic Attempts 进入 Implementation Config。

超过自动安全边界：

```text
NEEDS_ATTENTION
```

---

# 557. Job Attempt History

`ops.job_runs` Append-only：

```text
job_run_id
job_id
attempt_no
worker_id
started_at
finished_at
result
safe_error_category
safe_error_detail
target_business_id nullable
```

旧 Attempt 永不覆盖。

---

# 558. Jobs and Maintenance

每种 `job_type` 声明：

```text
affected_maintenance_scopes
```

例如：

```text
RANKING_INCREMENTAL
→ may continue under Rankings Publishing Maintenance

RANKING_PUBLISH
→ blocked

ANNOUNCEMENT_PUBLISH
→ blocked under Announcements Scheduling
```

已经接受的 Asset Transfer / Poker Hand / Reward Claim / Round Recovery 不得因 Maintenance 被遗弃。

---

# 559. DB Time Authority

Scheduler / Job Due 判断使用：

```text
PostgreSQL now()
```

Browser Time 不作为业务 Authority。

业务显示可转换为 Asia/Shanghai，Durable Timestamp 继续使用 UTC。

---

# 560. Manual Job Operations

允许：

```text
View
Retry
Resume
Cancel pending safe job
Create approved Rebuild Job
Mark Needs Attention
```

禁止：

```text
arbitrary payload editing
arbitrary command
fake SUCCESS
delete failed runs
```

---

# 561. Ranking Rebuild Job

`RANKING_REBUILD` 必须明确：

```text
domain
metric
period
scope
reason
operation_id
```

输出：

```text
Shadow Aggregate Set
Diff Report
```

Job Success 不等于自动 Publish Repair Snapshot。

---

# 562. Announcement Schedule Atomicity

Schedule Action 同一事务保存：

```text
Announcement SCHEDULED state
+
Durable Job Schedule
```

Job Schedule 创建失败则整个 Scheduling Operation Rollback。

禁止 UI 显示 Scheduled 而没有 Durable Scheduler Fact。

---

# 563. TD-09 Metrics

Rankings：

```text
ranking_last_source_scan_at
ranking_last_publish_at
ranking_aggregation_lag_seconds
ranking_data_completeness
ranking_source_fact_conflict
ranking_rebuild_running
ranking_publish_failure
```

Jobs：

```text
job_pending_count
job_running_count
job_retry_wait_count
job_needs_attention_count
job_oldest_pending_age
job_lease_recovery_count
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

History：

```text
history_index_lag
history_index_rebuild_count
history_source_mismatch
record_access_denied
fairness_access_denied
```

---

# 564. TD-09 Crash Point Analysis

| Crash Point | Recovery |
|---|---|
| Source committed, scanner 尚未读取 | Cursor next scan |
| Ranking Fact inserted, cursor not advanced | Unique derived fact |
| Aggregate built, pre-publish crash | Shadow persists |
| Publish pointer crash | PostgreSQL all-or-nothing |
| Source exclusion created, rebuild crash | Resume rebuild |
| History Source committed, index absent | Rebuild index |
| Announcement schedule worker crash | Durable Job lease recovery |
| Publish committed, response lost | Query announcement state |
| Content update committed, no re-notify | Same Notification Revision remains |
| Re-notify committed, popup path crash | Revision durable |
| Job target effect committed, job state not success | Target-idempotent retry |
| Worker dies holding lease | Lease expiry recovery |

---

# 565. TD-09 Test Gate

## Rankings

```text
same source scanned 100 times
→ one aggregate effect

internal API retry
→ one logical request

Key Purpose changes
→ historical attribution unchanged

Poker Active Session
→ no formal public Poker profit

Poker Cash Out
→ held ranking facts become eligible
```

## Tie

```text
100 / 90 / 90 / 80
→ 1 / 2 / 2 / 4
```

## Period

```text
Asia/Shanghai 23:59:59 / 00:00:00
Sunday → Monday 00:00
```

## Snapshot

```text
Required Source unavailable
→ no incomplete publish

Publish crash
→ old or new complete pointer only
```

## Repair

```text
exclude source
→ Shadow Diff
→ public score changes only after Publish

revoke exclusion
→ rebuild restores source
```

## RP Privacy

Public never returns：

```text
Key ID
Request ID
Prompt
Response
Raw Error
IP
UA
Provider
Channel
```

## History

```text
Index deleted
→ rebuild from source

Retired Game
→ history accessible

Wrong user
→ denied

Operator
→ no unrevealed Poker private data
```

## Announcement

```text
Update Content Only
→ same notification revision

Re-notify
→ revision++

same revision refresh
→ no repeat Entry Popup

overlap Entry Popup window
→ rejected

expired
→ invisible even if scheduler down
```

## Jobs

```text
two workers
→ one lease

worker crash
→ lease recovery

target effect commit then crash
→ no duplicate effect

maintenance block
→ safe resume
```

---

# 566. TD-09 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| TD-FRZ-289 | Rankings 永远由 Economy / Game / Poker / API Usage Durable Facts 聚合产生；Ranking Tables 是可重建 Projection，不成为第二业务 Authority。 | FROZEN |
| TD-FRZ-290 | Ranking 增量摄取使用 Source-native Stable ID + Durable Cursor + Unique Derived Fact，Worker 重扫不会重复计分。 | FROZEN |
| TD-FRZ-291 | Direct Play 排名来源仅使用最终 SETTLED Round；Refunded Round 不贡献 Profit / Wagered / Biggest Win。 | FROZEN |
| TD-FRZ-292 | Poker Hand 指标可以先持久化，但正式 Public Ranking Eligibility 只有 Parent Session Cash Out 后才释放。 | FROZEN |
| TD-FRZ-293 | Poker Profit 使用 Session Realized P/L，并按照 Cash Out Time 归属正式统计周期。 | FROZEN |
| TD-FRZ-294 | Game Profit = Direct Play settled net + Poker settled Session realized P/L；Reward、Adjustment、Exchange 与 Poker Funding Movement 排除。 | FROZEN |
| TD-FRZ-295 | Biggest Win 使用 Direct Play Round / Poker Hand 的正 Net Profit，而不是 Gross Payout / Poker Pot；Poker Hand 必须等待 Session Cash Out。 | FROZEN |
| TD-FRZ-296 | Total Wagered 使用 Direct Play Total Stake 与 Poker 实际 Pot 投入；Poker 同一投入只统计一次，Returned Uncalled Excess 不作为有效 Wager Ownership Transfer。 | FROZEN |
| TD-FRZ-297 | Total Assets Ranking 只提供 Current Complete Snapshot，不建立历史资产回放；Required Authority 不完整时保留 Last Good Published Snapshot 并标记 Stale / Degraded。 | FROZEN |
| TD-FRZ-298 | Ranking Period Engine 使用 Asia/Shanghai；Weekly 从 Monday 00:00 开始；RP 只从 Feature Activation Time 起统计且不回溯推测旧 Purpose。 | FROZEN |
| TD-FRZ-299 | RP Ranking 只消费 Finalized Request Attribution，要求 Stable Logical Request ID、Key Purpose Snapshot、Model ID、Final Status、Error Category、Actual Credit、Timestamp；具体 NewAPI 字段继续 SOURCE VERIFICATION REQUIRED。 | FROZEN |
| TD-FRZ-300 | RP Calls 只统计成功的合格 RP Logical Request；内部渠道 Retry 仍只是一条 Logical Request。 | FROZEN |
| TD-FRZ-301 | RP Errors 以 Error Count 排序，并按已冻结 Error Eligibility 统计；公共输出永远不暴露 Raw Error。 | FROZEN |
| TD-FRZ-302 | RP Credits Consumed 使用 Final Actual Settled API Credit；不得使用 Estimated Cost、USD 或 Raw Quota 排名。 | FROZEN |
| TD-FRZ-303 | RP Model Filter 基于稳定 Chaldea Model ID 的 per-model aggregate；Retired Model 继续解析历史 Catalog Metadata，不公开 Provider / Channel。 | FROZEN |
| TD-FRZ-304 | Ranking Published Data 使用 Versioned Aggregate Set；Routine Aggregation 自动验证完整后发布，Repair 使用独立 Shadow Set。 | FROZEN |
| TD-FRZ-305 | 同分排名使用 SQL `RANK()` 等价语义，产生 1、2、2、4，而非 Dense Rank。 | FROZEN |
| TD-FRZ-306 | Rankings 展示 Current Master Profile；Recent Wins / Featured Records 使用事件发生时 Identity Snapshot，真实归属始终为稳定 `newapi_user_id`。 | FROZEN |
| TD-FRZ-307 | Ranking Repair 只允许 Source Exclusion / Repair / Rebuild；Exclusion 不删除 Source，启用与撤销都必须 Reason + Audit。 | FROZEN |
| TD-FRZ-308 | Ranking Rebuild 采用 Shadow → Diff → Review → Atomic Publish Pointer Swap；Job 成功不等于自动发布 Repair Snapshot。 | FROZEN |
| TD-FRZ-309 | Historical Closed Aggregate Set 通过新 Version 修复，旧 Published Version 保留 Audit；RP 日榜 / 周榜永久保留。 | FROZEN |
| TD-FRZ-310 | Rankings Publishing Maintenance 可以暂停 Publish Pointer Swap，但不要求停止 Source Ingestion / Shadow Build；公共页面继续使用 Last Good Published Snapshot。 | FROZEN |
| TD-FRZ-311 | Game History 使用 Rebuildable Unified History Index；Round / Session / Hand Detail 永远读取各自 Durable Source，而不是 Index Payload。 | FROZEN |
| TD-FRZ-312 | `/history` 默认只列 Direct Play Round 与 Poker Session；Poker Hand 通过 Parent Session 或高级 Record Filter 查询。 | FROZEN |
| TD-FRZ-313 | 完整个人 Game History 仅 Owner / Authorized Records Scope 可见；Records 默认只读，异常通过 Incident / Repair Flow，不直接编辑正式历史。 | FROZEN |
| TD-FRZ-314 | Retired Game / Closed Poker Table 继续保留 History、Config/Fairness Cross-link 与稳定 ID；V1 不提供完整私人 History 公共分享或 CSV/JSON 导出。 | FROZEN |
| TD-FRZ-315 | Public Recent Wins / Featured Records 使用与 Private History 分离的安全 Public Event Projection，只保存已允许公开字段与事件时身份快照。 | FROZEN |
| TD-FRZ-316 | `PUBLIC_RECORD_SELECTION_POLICY` 当前保持 PRODUCT RULE NOT FOUND；未定义合格门槛/精选算法前不得虚构 Recent Wins，模块无合格记录时隐藏。 | FROZEN |
| TD-FRZ-317 | Announcement 技术身份严格区分 `announcement_id / content_version / notification_revision`；Content Update 与 Re-notify 是不同操作。 | FROZEN |
| TD-FRZ-318 | Published Announcement Content 使用 Immutable Content Version；Update Content Only 创建新 Content Version 但不改变 Notification Revision。 | FROZEN |
| TD-FRZ-319 | Re-notify 创建新 Notification Revision，只有该动作才重新赋予 Entry Popup / Unread 资格，并必须 Impact Preview + Audit。 | FROZEN |
| TD-FRZ-320 | Announcement 主生命周期保持 Draft / Scheduled / Published / Expired / Archived；Withdraw 作为审计化 Visibility Stop，不硬删除历史。 | FROZEN |
| TD-FRZ-321 | Announcement 时间使用 Asia/Shanghai 业务语义 + TIMESTAMPTZ 存储；Effective Visibility 每次查询都重新验证 State / Visibility / Time / Withdraw。 | FROZEN |
| TD-FRZ-322 | Pinned / Entry Popup / Post-login Popup / Home Banner / Dashboard Summary 是独立 Placement；Schedule Validation 强制 Entry Popup 同一时点最多一条，并遵守 V1 单一 Primary Home Banner。 | FROZEN |
| TD-FRZ-323 | Anonymous Entry Popup Dismissal 使用 `(announcement_id, notification_revision)` Browser-local State；Popup Dismissed 与 Announcement Read 永远分离。 | FROZEN |
| TD-FRZ-324 | Logged-in Read State 使用 Server-side `(user, announcement, notification_revision)`，跨设备同步；新 Notification Revision 可重新进入 Unread。 | FROZEN |
| TD-FRZ-325 | Post-login Popup 永远位于 Master Initialization / Migration Notice 后，并在 Poker、Active Direct Play、Wallet Processing 等关键流程中延迟至 Safe Page。 | FROZEN |
| TD-FRZ-326 | Acknowledgements 使用单一长期规范 Announcement + Versioned Structured Contributor Entries；Public identity/media/link 必须满足同意与隐私要求。 | FROZEN |
| TD-FRZ-327 | Announcement Markdown / Rich Text 在 Save 与 Render 使用同一 Versioned Sanitization Policy，禁止 Script、Event Handler、Arbitrary iframe / JS。 | FROZEN |
| TD-FRZ-328 | Announcement Scheduler 只负责 Durable Lifecycle Transition；用户可见性始终由数据库状态与实时 Time Window 计算，Scheduler 故障不能令 Expired 内容继续可见。 | FROZEN |
| TD-FRZ-329 | TD-09 引入 PostgreSQL-backed Durable Background Job Framework；关键业务 Job 不使用无状态 Cron/Shell 作为唯一执行事实。 | FROZEN |
| TD-FRZ-330 | Job Type 必须 Code-allowlisted、Payload Schema-validated；Operations 不提供任意命令、SQL、Redis 或 Shell Job。 | FROZEN |
| TD-FRZ-331 | Job 使用 Durable State、`dedupe_key`、`FOR UPDATE SKIP LOCKED` Lease、Heartbeat 与 Lease Expiry Recovery；Redis 不是 Job Queue Authority。 | FROZEN |
| TD-FRZ-332 | Job Delivery 可 at-least-once，但 Handler 必须 Target-idempotent；Target Effect Commit 后 Worker Crash 的 Retry 只能收敛原 Effect。 | FROZEN |
| TD-FRZ-333 | Job Retry 使用可配置 Exponential Backoff + Jitter；超过安全自动恢复边界进入 NEEDS_ATTENTION，不无限高速重试。 | FROZEN |
| TD-FRZ-334 | Job Attempt History Append-only；Manual Retry / Resume 不能删除失败 Run、不能伪造 Success、不能修改 Payload 为任意命令。 | FROZEN |
| TD-FRZ-335 | Job 声明 Maintenance Scope；Publishing/Scheduling Job 可被对应 Maintenance 阻断，而已经接受的资产/Hand/Transfer/Reward Recovery 必须继续按原状态机完成。 | FROZEN |
| TD-FRZ-336 | Ranking Rebuild / Announcement Publish 等 Job 的 Schedule/Target Fact 必须与业务状态一致；Scheduled UI State 不能在不存在 Durable Job/Schedule Fact 时单独成功。 | FROZEN |
| TD-FRZ-337 | TD-09 Implementation 必须通过 Ranking Exactly-once / Period / Privacy / Shadow Repair、History Rebuild/Auth、Announcement Revision/Popup/Sanitization、Job Lease/Crash/Maintenance 等测试 Gate。 | FROZEN |

---

# 567. Change Log — WORKING v0.9

## Added

- 用户正式确认 TD-09；
- 冻结 `TD-FRZ-289 ～ TD-FRZ-337`；
- 冻结 Rankings Source → Derived Facts → Aggregate Set → Published View；
- 冻结 Direct Play / Poker / RP Ranking Authority；
- 冻结 Poker Cash-out Eligibility Gate；
- 冻结 Total Assets Current Complete Snapshot；
- 冻结 Asia/Shanghai Daily / Weekly Period；
- 冻结 RP Feature Activation；
- 冻结 Logical Request / Purpose Snapshot / Model Attribution；
- 冻结 Aggregate Set / Tie Rank；
- 冻结 Routine Aggregation / Shadow Rebuild；
- 冻结 Source Exclusion / Diff / Atomic Publish；
- 冻结 Unified Game History Index；
- 冻结 Record Source Authority / Authorization；
- 冻结 Public Game Event Projection；
- 冻结 Announcement ID / Content Version / Notification Revision；
- 冻结 Update Content Only / Re-notify；
- 冻结 Entry Popup / Post-login Popup / Read / Dismissal；
- 冻结 Acknowledgements / Privacy / Sanitization；
- 冻结 Announcement Scheduler；
- 冻结 PostgreSQL Durable Job Framework；
- 冻结 Job Allowlist / Lease / Retry / Attempts / Maintenance；
- 冻结 TD-09 Crash / Test Gate；
- 新增 `TD-09-PROD-GAP-01`。

## Explicitly Still Open

```text
TD-09-PROD-GAP-01
Recent Public Wins / Featured Records
Eligibility Threshold / Count / Selection Algorithm
```

---

# 568. 下一批 — TD-10

下一批正式进入：

> **TD-10 — Chaldea Operations / RBAC / Audit / Maintenance**

计划完整冻结：

1. Operations Shell；
2. Environment Identity；
3. Global Search；
4. Needs Attention；
5. Super Admin / Operator / Auditor；
6. Module Scopes；
7. Permission Evaluation；
8. Stable `newapi_user_id` Admin Binding；
9. NewAPI Admin 与 Chaldea Operations 分离；
10. Fresh Auth；
11. Dangerous Operation Levels；
12. Reason / Typed Confirmation / Impact Preview；
13. Operation ID；
14. Audit Append-only；
15. Before / After Snapshot；
16. Secret Redaction；
17. Incident Model；
18. Support Case；
19. Economy / Poker / Ranking Repair Boundaries；
20. Service Health；
21. Maintenance Scopes；
22. Maintenance Schedule；
23. Maintenance State Machine；
24. Maintenance Impact Preview；
25. Accepted-work Protection；
26. Global vs Module Maintenance；
27. Operations Job Controls；
28. No Shell / SQL / Redis Console；
29. Access Control Changes；
30. Session Revocation / Permission Refresh；
31. Mobile / PC Operations Boundary；
32. Audit Retention；
33. Security / Concurrency / Test Gate。

特别注意：

- Operations 绝不变成 VPS 控制台；
- NewAPI Admin 权限不会自动映射成 Chaldea Super Admin；
- Admin Adjustment、Discord Rebind、Access Control、全站 Maintenance、Poker Emergency Pause、关键经济发布继续属于 Critical；
- Audit 只能 Append，不允许管理员删除历史；
- Maintenance 必须保护已经接受的 Round、Poker Hand、Transfer、Claim 与 Cash Out；
- TD-10 将正式收口前面各批留下的 Operations 权限边界，但不会改写资产/游戏/Poker 业务真相。


---

# 569. TD-10 — Chaldea Operations / RBAC / Audit / Maintenance

> 状态：`FROZEN`  
> 用户确认：`整体按上述方案通过`

## 569.1 TD-10 总体结论

本批正式冻结：

- Chaldea-side Admin Principal；
- Super Admin / Operator / Auditor；
- Operator 固定模块 Scope；
- Code-owned Permission Matrix；
- Server-side Authorization；
- `authz_epoch` 即时撤权；
- NewAPI Admin / Chaldea Operations 双后台隔离；
- Last Super Admin Safety Guard；
- Level 1 / 2 / 3 Operation Guard；
- Fresh Authentication；
- Server-generated Impact Preview；
- Typed Confirmation；
- Production Environment Binding；
- Durable Admin Operation；
- Same-DB Audit Atomicity；
- Cross-DB Admin Operation Recovery；
- Audit-safe Serializer；
- Append-only Audit；
- Global Search Security Boundary；
- Stable Deep Link；
- Needs Attention Projection；
- Incident / Support Case；
- Discord Rebind / Legacy Recovery Session Security Reset；
- Economy / Games / Poker / Rankings / Records / Announcements Operations Boundary；
- Service Health Projection；
- Seven Maintenance Scopes；
- Maintenance Durable Authority / Lifecycle；
- Maintenance Scope Union；
- Same-scope Overlap Protection；
- Accepted-work Protection；
- Scheduled Maintenance；
- No Infrastructure Console；
- Responsive Operations Security；
- Crash / Permission / Maintenance / Audit Test Gate。

本批新增安全技术决定：

```text
TD-10-C01
Role / Scope / Enabled 状态修改后
通过 authz_epoch 立即使旧 Operations Authorization 失效

TD-10-C02
Discord Rebind / Legacy Account Recovery 成功后
递增目标用户 security_epoch
撤销既有 Chaldea Web Sessions / Poker Connect Ticket 能力
要求重新认证
```

继续保留既有 OPEN：

```text
TD-05 Hourly / Relief OPEN fields
POKER-PROD-GAP-01..05
TD-09-PROD-GAP-01
```

---

# 570. Operations Authorization Topology

```text
NewAPI Authentication
        │
        ▼
stable newapi_user_id
        │
        ▼
Chaldea Ops Principal
        │
        ├── Base Role
        │     SUPER_ADMIN
        │     OPERATOR
        │     AUDITOR
        │
        ├── Operator Scopes
        │
        ├── Authz Epoch
        │
        └── Enabled State
                │
                ▼
Server-side Permission Engine
                │
                ▼
Operation Guard
        ┌───────┼────────┐
        ▼       ▼        ▼
     Level 1  Level 2  Level 3
        │       │        │
        └───────┴────────┘
                │
                ▼
Domain Admin Contract
                │
                ▼
Business Transaction + Append-only Audit
```

NewAPI 负责身份认证，Chaldea Operations 权限独立保存在 Chaldea。

---

# 571. Admin Principal

建议 `ops.admin_principals`：

```text
admin_principal_id UUIDv7
newapi_user_id
base_role
status
authz_epoch BIGINT
created_at
created_by
updated_at
updated_by
disabled_at nullable
disabled_by nullable
version
```

`base_role`：

```text
SUPER_ADMIN
OPERATOR
AUDITOR
```

V1 一个 Principal 同时只拥有一个 Base Role。

---

# 572. Base Role Boundaries

## Super Admin

拥有完整 Chaldea Operations 业务权限，但仍禁止：

```text
Password / Password Hash
API Key Secret
Unauthorized Prompt / Response
Unrevealed Poker Hole Cards
Unrevealed Server Seed / Future Deck
Arbitrary SQL / Redis / Shell
```

Super Admin 也必须走正式 Adjustment / Rebind / Recovery / Maintenance / Publish Contract。

## Operator

只拥有 Assigned Module Scopes 内已批准的日常动作。

## Auditor

全后台只读与 Audit 查看，仍受秘密数据边界限制。

---

# 573. V1 Operator Scope Catalog

固定：

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

TD-10 不新增普通 Operator：

```text
ECONOMY
ACCESS_CONTROL
AUDIT_ADMIN
SYSTEM_SHELL
DATABASE
```

Scope。

Cross-domain Summary Read 不等于目标域 Write Permission。

---

# 574. Code-owned Permission Matrix

Permission Key 由 Backend 代码拥有，例如：

```text
models.metadata.read
models.metadata.write
models.publish

users.read
users.master.moderate
users.support.write

games.metadata.write
games.runtime.pause
games.config.draft

poker.read
poker.accepting_players.write
poker.recovery.request

rewards.read
rewards.claim.retry

rankings.read
rankings.rebuild

records.read

announcements.write
announcements.publish
```

Super Admin-only 示例：

```text
economy.adjust
identity.discord_rebind.execute
access_control.write
maintenance.global
poker.emergency_pause
economic_config.activate
critical_repair.publish
```

数据库不能凭空添加任意 Permission String 就让 Backend 获得新能力。

---

# 575. Server-side Authorization

所有 Ops REST / WS Admin Command / Background-triggering Admin Action 都必须重新做服务端权限检查。

以下只用于 UX：

```text
Hide Sidebar
Disable Button
Frontend Route Guard
```

不能替代授权。

---

# 576. Authz Epoch

Admin Session 保存：

```text
session.admin_authz_epoch
```

Request 对比当前：

```text
ops.admin_principals.authz_epoch
```

Role / Scope / Enabled 状态变化：

```text
authz_epoch++
```

旧 Session：

```text
AUTHORIZATION_STALE
```

---

# 577. TD-10-C01 — Immediate Ops Revocation

正式冻结：

```text
Role Change
Scope Change
Admin Disable
→ increment authz_epoch
→ old Ops authorization immediately invalid
```

旧 Tab 即使显示旧 UI，下一次 Server Write 也必须拒绝。

移除 Ops 权限默认不撤销普通用户站点登录。

---

# 578. NewAPI Admin Separation

NewAPI Admin 与 Chaldea Super Admin：

```text
no automatic mapping
```

“Open NewAPI Admin”只对真实拥有 NewAPI Admin 权限的用户显示。

实际 NewAPI Admin Role / Permission Detection：

```text
SOURCE VERIFICATION REQUIRED
```

---

# 579. Last Super Admin Invariant

强制：

```text
active_super_admin_count >= 1
```

禁止移除、禁用或降级最后一位 Active Super Admin。

---

# 580. Access Control Mutation

建议：

```text
ops.admin_scope_assignments
ops.admin_role_history
```

修改：

```text
BEGIN
lock target principal
validate actor = Super Admin
validate target/version/scope
validate last-super-admin invariant
apply role/scope
target.authz_epoch++
Audit
COMMIT
```

Access Control = Level 3 Critical。

---

# 581. Risk Levels

固定：

```text
LEVEL_1_ROUTINE
LEVEL_2_IMPACTFUL
LEVEL_3_CRITICAL
```

Level 1：查看、Draft、未发布普通元数据。  
Level 2：普通公告发布、非经济配置激活、单游戏 Maintenance、Ranking Rebuild、关闭 Poker 新玩家。  
Level 3：资产 Adjustment、Discord Rebind、Access Control、全站 Maintenance、Poker Emergency Pause、经济配置发布、手工资产补偿。

---

# 582. Operation Guards

Level 1 Write：

```text
permission → transaction → audit
```

Level 2：

```text
Server Impact Summary
+ Explicit Confirmation
+ Audit
```

Level 3：

```text
Fresh Authentication
+ Required Reason
+ Typed Confirmation
+ Impact Preview
+ Unique Operation ID
+ Append-only Audit
```

---

# 583. Fresh Authentication

复用 TD-02：

```text
Fresh Auth Window = 10 minutes
```

真正 Submit 时必须仍在窗口内；旧 Preview 放置超过窗口必须重新 Fresh Auth。

---

# 584. Scheduled Critical Operation

Schedule Creation 时完成 Fresh Auth / Reason / Typed Confirmation / Impact Preview。

真正到点由 Durable Scheduler 按已经授权的 Durable Fact 执行，不要求管理员届时在线。

---

# 585. Admin Operation Record

建议：

```text
ops.admin_operations
```

至少：

```text
operation_id UUIDv7
operation_type
risk_level
actor_newapi_user_id
actor_role
actor_scopes_snapshot
target_type
target_id
reason nullable
impact_preview
impact_hash
confirmation_challenge_hash nullable
fresh_auth_verified_at nullable
state
related_business_id nullable
created_at
executed_at
completed_at
```

---

# 586. Admin Operation State

```text
PREPARED
→ AUTHORIZED
→ EXECUTING
→ SUCCEEDED
```

异常：

```text
FAILED_NO_EFFECT
NEEDS_REVIEW
RECOVERING
```

Critical Retry 必须复用原 `operation_id`。

---

# 587. Impact Preview / TOCTOU

Impact Preview 必须由 Server 从当前权威事实生成并绑定 Hash。

真正 Submit：

```text
lock target
re-read facts
revalidate permission
revalidate constraints
```

事实已变化：

```text
PREVIEW_STALE
```

要求重新 Preview。

---

# 588. Typed Confirmation

Server 生成 Operation-specific Challenge，例如：

```text
PRODUCTION <target-short-id>
```

Challenge 绑定：

```text
operation_id
target
environment
```

不使用全站永久单一 `CONFIRM`。

---

# 589. Environment Identity

环境来自部署配置：

```text
PRODUCTION
STAGING
DEVELOPMENT
```

不能被 localStorage / query string / frontend label 改变。

Operation / Audit / Maintenance / Job 均保存 Environment Snapshot。

Production Level 3 Typed Confirmation 必须显式包含 Production Context。

---

# 590. Audit Authority

建议：

```text
audit.admin_events
```

至少：

```text
audit_id UUIDv7
actor_newapi_user_id
actor_role
actor_scopes_snapshot
action
target_type
target_id
before_snapshot
after_snapshot
reason nullable
operation_id
result
timestamp
related_business_id nullable
request_id nullable
environment
```

---

# 591. Same-DB Mutation + Audit Atomicity

同库管理写操作：

```text
BEGIN
Business Mutation
Admin Operation State
Audit
COMMIT
```

Audit Insert 失败：

```text
whole mutation rollback
```

---

# 592. Cross-DB Critical Admin Operation

涉及 NewAPI 的 Critical Operation 使用 Durable Operation State + Audit Timeline，不假装跨库原子。

例如 Rebind：

```text
AUTHORIZED
→ TARGET_VALIDATING
→ NEWAPI_EFFECT_EXECUTING
→ NEWAPI_EFFECT_CONFIRMED
→ CHALDEA_SUPPORT_UPDATED
→ SUCCEEDED
```

NewAPI Binding Write：

```text
SOURCE VERIFICATION REQUIRED
```

未知结果先查询正式状态。

---

# 593. Audit-safe Serializer

Before / After 永久剔除：

```text
Password
Password Hash
API Key Secret
OAuth Secret
Private Prompt / Response
Unrevealed Poker Seed
Future Deck
Unauthorized Hole Cards
```

---

# 594. Audit Immutability

Application Runtime 对 Audit：

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

V1 不设置业务 TTL 删除 Audit。

金融错误通过新的 Reversal / Compensation + Ledger + Audit 修正。

---

# 595. Global Search

允许定位：

```text
Master Nickname
Short Account ID
newapi_user_id
Discord User ID
API Key ID
Transfer ID
Transaction ID
Round ID
Poker Table ID
Poker Session ID
Poker Hand ID
Announcement ID
Config Version
Audit ID
```

绝不索引 / 返回：

```text
API Key Secret
Password
Password Hash
Prompt
Response
Hole Card
Server Seed
Future Deck
```

Search 必须在 Server Permission Filter 后返回 Redacted DTO。

---

# 596. Stable Deep Links

Operations Detail 使用稳定 URL，供 Needs Attention / Audit / Incident / Operator Collaboration / Refresh。

Drawer 只承担 Preview；复杂 Repair 必须进入正式 Detail。

---

# 597. Needs Attention

建议：

```text
ops.attention_items
```

作为可重建 Operational Projection。

聚合：

```text
Pending Transfer
Compensation Failure
Reward Failure
Recovering Round
Paused Poker
Cash Out Failure
Ranking Lag
Model Sync
Announcement conflict
Binding conflict
Support case
Maintenance
```

身份：

```text
attention_source_type
attention_source_id
attention_reason_code
```

等价 Unique。

状态：

```text
OPEN
ACKNOWLEDGED
RESOLVED
```

`RESOLVED` 必须由底层问题消失或正式 Repair/Incident Outcome 驱动。

---

# 598. Attention Acknowledge

Acknowledge 只保存：

```text
acknowledged_by
acknowledged_at
```

语义：

```text
seen != fixed
```

未解决金融 / Poker / Settlement 问题不得永久隐藏。

---

# 599. Incident Model

建议：

```text
ops.incidents
```

至少：

```text
incident_id UUIDv7
severity
category
title
safe_summary
state
source_type
source_id
assigned_to nullable
created_by
created_at
resolved_at nullable
closed_at nullable
```

状态建议：

```text
OPEN
→ TRIAGED
→ IN_PROGRESS
→ MONITORING
→ RESOLVED
→ CLOSED
```

False Positive 可直接 Closed，但必须记录理由。

---

# 600. Incident Timeline

使用 Append-only：

```text
ops.incident_events
```

记录：

```text
state change
assignment
comment
linked operation
linked job
linked audit
linked business fact
```

Incident 本身不直接修改业务数据。

---

# 601. Support Case

延续：

```text
OPEN
→ VERIFYING
→ APPROVED
→ EXECUTED
→ CLOSED
```

拒绝：

```text
VERIFYING
→ REJECTED
→ CLOSED
```

Evidence 只保存最少必要安全元数据：

```text
verification_method
verified_facts
verified_by
verified_at
reference
```

不保存 Password / Password Hash / Full API Key / Secret。

---

# 602. Discord Rebind

必须：

```text
Support Case = APPROVED
Original ownership verified
New Discord uniqueness verified
Super Admin
Reason
Fresh Auth
```

实际 NewAPI Binding Write：

```text
SOURCE VERIFICATION REQUIRED
```

禁止自动 Account Merge、Asset Move、API Key Move、History Ownership Rewrite。

---

# 603. TD-10-C02 — Security Epoch after Identity Recovery

成功：

```text
Discord Rebind
or
Legacy Account Recovery
```

后：

```text
target user security_epoch++
```

撤销既有：

```text
Chaldea Web Sessions
Poker Connect Ticket capability
```

要求重新认证。

不改变余额、API Keys、History。

---

# 604. Legacy Password Recovery

Operations 不允许管理员查看现有密码、Hash 或指定用户最终密码。

实际 Reset 走 Source-verified NewAPI Auth Contract。

Chaldea 只保存 Support Authorization / Audit。

---

# 605. Economy Operations Boundary

Wallet / Ledger 默认只读。

Reconciliation 只允许：

```text
Retry
Resume
Compensate
Mark for Review
```

禁止 Force CONFIRMED、Delete Failure、Manual Balance Patch。

Admin Adjustment 仅 Super Admin，继承 TD-04 的 Before/Delta/After、Reason/Reference、Fresh Auth、Typed Confirm、Ledger/Audit 不变量。

---

# 606. Games / Poker Operations Boundary

Games Operator 可处理：

```text
Metadata
Catalog
Safe Runtime
Draft Config
Validate
Preview
```

经济参数 Activation = Super Admin-only；Active/Historical Config Immutable。

Poker 允许：

```text
Stop / Resume Accepting Players
Stop / Resume New Hands
Close After Current Hand
Remove Player After Hand
Remove Spectator
Mute
Pause
Request Recovery
```

禁止 Stack/Pot/Winner/Deck/Settlement Edit、Current-hand Cash Out、Hole Card / Seed Peek。

Poker Emergency Pause = Level 3 Super Admin Operation。

---

# 607. Rankings / Records / Announcements Boundary

Ranking Operator 可 Build / Inspect Repair Shadow。

Critical Repair Snapshot Final Publish：

```text
Super Admin only
```

Records Scope 仅 Read/Search/Cross-link/Create Incident，不修改 History。

Announcements Operator 可 Create/Edit/Ordinary Publish/Schedule/Placement；普通发布为 Level 2。Re-notify 必须 Impact Preview + Explicit Confirmation + Audit。



# 608. Service Health Projection

建议：

```text
ops.service_health
```

至少覆盖：

```text
Chaldea Frontend
Chaldea Backend
NewAPI Connectivity
Poker Service
PostgreSQL / Chaldea
PostgreSQL / NewAPI
Redis
Reconciliation Worker
Ranking Aggregator
Announcement Scheduler
Reward Jobs
```

状态：

```text
OPERATIONAL
DEGRADED
MAINTENANCE
UNAVAILABLE
UNKNOWN
```

每项：

```text
service_key
status
observed_at
safe_summary
source_check
```

Health 是 Operational Projection，不是业务 Authority。

---

# 609. Health Failure Boundary

Health Check Failure 不自动：

```text
refund round
change wallet
settle poker
set missing asset = 0
```

它只影响 Status / Attention / 明确定义的 Admission Gate。

Operations 可以显示 Safe Error Category / Last Successful Check / Latency / Dependency，但仍不得暴露 Secret / Credential / Seed。

---

# 610. Maintenance Authority

建议：

```text
ops.maintenance_windows
```

至少：

```text
maintenance_id UUIDv7
scope
state
reason
impact_snapshot
impact_hash
scheduled_start_at nullable
activated_at nullable
ended_at nullable
created_by
activated_by nullable
ended_by nullable
operation_id
created_at
```

PostgreSQL 是 Durable Authority；Redis / Memory 只能 Cache。

---

# 611. Frozen Maintenance Scopes

固定：

```text
CHALDEA_USER_WRITES
WALLET_EXCHANGE
REWARDS
DIRECT_PLAY_NEW_ROUNDS
POKER_NEW_TABLES_NEW_HANDS
RANKINGS_PUBLISHING
ANNOUNCEMENTS_SCHEDULING
```

NewAPI 模型 API Maintenance 不纳入 Chaldea Maintenance。

---

# 612. Maintenance Lifecycle

建议：

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

---

# 613. Maintenance Guard

统一：

```text
Select Scope
→ Required Reason
→ Server Impact Preview
→ Optional Schedule
→ Fresh Authentication
→ Confirm
→ Durable Maintenance
→ Backend Gate Active
```

所有 Maintenance Create / Activate 都要求 Fresh Auth。

---

# 614. Maintenance Risk

建议：

Level 2：

```text
DIRECT_PLAY_NEW_ROUNDS
REWARDS
RANKINGS_PUBLISHING
ANNOUNCEMENTS_SCHEDULING
ordinary Poker stop-new-work
```

Level 3：

```text
CHALDEA_USER_WRITES
WALLET_EXCHANGE
POKER_EMERGENCY_PAUSE
```

多 Scope Maintenance：

```text
risk = max(selected scope risk)
```

---

# 615. Backend Maintenance Gate

所有 New-work Admission Path 必须查询 Effective Maintenance State。

统一错误：

```text
MAINTENANCE_ACTIVE
scope
safe_message
started_at
estimated_end nullable
announcement_id nullable
```

Frontend Disable 不能替代 Backend Gate。

---

# 616. Multiple Maintenance Windows

不同 Scope 可以同时 Active。

Effective Gate：

```text
union(active scopes)
```

结束一个 Window 不清除其他 Scope。

V1 禁止两个未完成 Window 对同一个 exact Scope 时间重叠。

---

# 617. CHALDEA_USER_WRITES

阻止新的普通 Chaldea Mutation，例如：

```text
Master Profile edit
new Exchange
new Reward Claim
new Direct Play Round
new Poker Join/Table mutation
```

但继续：

```text
existing Transfer Reconciliation
accepted Round Settlement
existing Poker Hand Recovery
Safe Leave / Cash Out
existing Reward Issuance Recovery
```

---

# 618. WALLET_EXCHANGE Maintenance

阻止：

```text
new Exchange
new user-initiated asset transfer
```

继续：

```text
existing Saga Recovery
Compensation
Reconciliation
Poker Cash Out
accepted Game Settlement
```

---

# 619. REWARDS Maintenance

阻止新的普通 Daily / Hourly / Relief Claim Admission。

已有：

```text
PENDING
ISSUING
RECOVERING
Initial Grant entitlement
```

继续完成。

---

# 620. DIRECT_PLAY_NEW_ROUNDS Maintenance

阻止新的：

```text
BET_ACCEPTED
```

已接受 Round 继续 Settle / Recover / Refund as required by original state machine。

---

# 621. POKER_NEW_TABLES_NEW_HANDS Maintenance

阻止：

```text
new Table
new Seat Reservation / Buy-in
new Hand
```

现有 Active Hand 继续 Finish / Pause / Recover。

Safe Leave / Cash Out 必须继续可用。

---

# 622. RANKINGS_PUBLISHING Maintenance

允许：

```text
source ingestion
shadow build
rebuild
validation
```

暂停：

```text
Published Pointer Swap
```

Public 继续读取 Last Good Snapshot。

---

# 623. ANNOUNCEMENTS_SCHEDULING Maintenance

暂停未来：

```text
SCHEDULED → PUBLISHED
```

Transition。

实时 `visible_until / withdrawn_at` 仍然生效。

已经错过整个窗口的公告不在解除维护后补弹。

---

# 624. Accepted-work Protection Registry

每个 Domain Adapter 明确声明：

```text
new_work_gate
accepted_work_recovery
safe_exit
```

| Scope | New Work | Accepted Work | Safe Exit |
|---|---|---|---|
| Wallet | Block | Reconciliation continues | Compensation |
| Rewards | Block new Claim | Existing issuance continues | Claim terminal |
| Direct Play | Block new Round | Settle/recover | Refund if required |
| Poker | Block new Table/Hand | Continue/recover | Cash Out |
| Rankings | Block publish | Build continues | Last good snapshot |
| Announcements | Block scheduling transition | Visibility enforced | Expire |

---

# 625. Maintenance No Automatic Compensation

进入 Maintenance 不自动：

```text
refund all rounds
cash out all poker
compensate all transfers
cancel all rewards
```

各 Domain 继续使用原状态机。

---

# 626. Scheduled Maintenance

与 TD-09 Durable Job Framework 集成。

同一事务保存：

```text
maintenance fact
activation schedule/job
audit
```

Schedule 创建失败则全部 Rollback。

---

# 627. Scheduled Activation / End

到点 Worker：

```text
load scheduled maintenance
verify DB time
verify scope conflict
activate durable gate
write system audit
```

失败：

```text
ACTIVATION_FAILED
→ Needs Attention
```

Active End：

```text
ACTIVE
→ ENDING
→ clear only this window
→ COMPLETED
```

Scheduled 未激活可：

```text
→ CANCELLED
```

均必须 Audit。

---

# 628. Maintenance Impact Preview

至少展示：

```text
Scope
Environment
Current active maintenance
Affected Users
Pending Transfers
Active Direct Play Rounds
Active Poker Tables / Hands
Pending Reward Claims
Scheduled Ranking Publishes
Scheduled Announcements
New Work Blocked
Accepted Work Continuing
Critical Notice / Announcement Link
```

无法读取的指标显示：

```text
Unavailable
```

不得伪造成 `0`。

---

# 629. Critical Notice Link

Maintenance 可关联：

```text
announcement_id
critical_notice_id
```

用户侧只显示安全业务影响，不暴露内部 Error Stack。

---

# 630. Operations Job Controls

沿用 TD-09：

```text
View
Retry
Resume
Approved Rebuild
Needs Attention
```

每次执行 Module Permission + Risk Guard。

不提供 Arbitrary JSON / Command Job。

---

# 631. No Infrastructure Console

正式冻结：

```text
NO SSH
NO Docker Shell
NO Command Exec
NO SQL Console
NO Redis Console
NO Package Upgrade
NO VPS Firewall Editor
```

Cockpit / Portainer 等始终是独立 Infrastructure Tool。

---

# 632. Environment Isolation

Operation / Audit / Maintenance / Job 均保存 Environment。

Production / Staging / Development 使用独立服务身份与数据库配置。

Operations 不提供：

```text
input database URL
select arbitrary server
```

能力。

---

# 633. Responsive Operations

PC：

```text
Persistent Sidebar
Wide Tables
Multi-column Filter
Detail Page + optional Drawer
```

Tablet：

```text
Collapsible Sidebar
Responsive Detail
```

Mobile：

```text
Sidebar → Drawer
List → Admin Cards
Filter → Fullscreen / Bottom Sheet
Dangerous Operation → Fullscreen Confirm
No Hover dependency
Move Up / Move Down fallback for ordering
```

Mobile 不得降低 Critical Guard：Fresh Auth / Reason / Impact Preview / Typed Confirmation 全部保留。

---

# 634. TD-10 Metrics

至少：

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
```

---

# 635. Critical Alerts

至少：

```text
Audit insert failure on Level 3
last Super Admin invariant threatened
unauthorized successful operation
stale authz_epoch but write succeeded
maintenance gate disagreement across instances
accepted economic work blocked past recovery SLA
Poker Cash Out blocked by maintenance
secret / unrevealed poker data leakage
arbitrary command surface detected
```

---

# 636. TD-10 Crash Point Analysis

| Crash Point | Recovery |
|---|---|
| Level 3 Preview only | No effect |
| Fresh Auth success, no submit | No effect |
| Same-DB mutation before Commit | Rollback incl. Audit |
| Commit success, HTTP lost | Query same `operation_id` |
| Access Control changed, response lost | Role / epoch durable |
| Discord Rebind result unknown | Query authoritative binding |
| Attention scanner crash | Rebuild projection |
| Incident event committed, notification lost | Incident durable |
| Maintenance scheduled, worker crash | Durable Job recovery |
| Maintenance activated, broadcast/cache lost | Gate reloads PostgreSQL |
| Maintenance ended, one cache stale | Version/cache refresh |
| Audit UI unavailable | Audit-required mutation cannot bypass Audit |

---

# 637. TD-10 Test Gate

## Permission

```text
Operator without Scope → 403
raw HTTP bypass → 403
Auditor write → 403
NewAPI Admin only → no Chaldea Ops
Chaldea Super Admin only → no automatic NewAPI Admin
```

## Access Control

```text
demote active Operator → old tab write rejected immediately
disable Admin → old Ops authorization revoked
remove last Super Admin → rejected
concurrent role update → serialized final version
stale form → version conflict
```

## Critical Operation

```text
expired Fresh Auth → reject
wrong Typed Confirm → reject
stale Preview → re-preview
same operation_id retry → one effect
Audit failure → same-DB mutation rollback
```

## Audit

```text
UPDATE Audit → DB denied
DELETE Audit → DB denied
secret serializer input → redacted/rejected
financial reversal → original Audit remains + new Audit
```

## Maintenance

每 Scope：

```text
new work blocked
accepted work continues
safe exit remains possible
```

重点：

```text
Wallet Maintenance → compensation/reconciliation continues
Rewards Maintenance → Initial Grant recovery continues
Direct Play Maintenance → accepted Blackjack/recovery continues
Poker Maintenance → active Hand settles + Safe Leave/Cash Out
Rankings Publishing → ingestion continues + old snapshot visible
Announcements Scheduling → expired content still invisible
```

## Secret Boundary

Super Admin / Operator / Auditor / Normal User / Disabled Admin / Stale Session 均不能读取：

```text
Password Hash
API Key Secret
Prompt / Response
Unrevealed Hole Card
Unrevealed Seed
Future Deck
```



---

# 638. TD-10 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| TD-FRZ-338 | Chaldea Operations 权限完全保存在 Chaldea 侧并以稳定 `newapi_user_id` 关联；NewAPI Authentication 只提供身份，不自动授予 Ops 权限。 | FROZEN |
| TD-FRZ-339 | V1 Admin Principal 使用单一 Base Role：Super Admin / Operator / Auditor；Super Admin 为完整 Chaldea 业务管理权限、Operator 按 Scope、Auditor 全后台只读。 | FROZEN |
| TD-FRZ-340 | V1 Operator 可授权 Scope 固定为 Models / Users & Identity / Games / Poker / Rewards / Rankings / Records / Announcements；TD-10 不擅自新增 Economy / Access Control 等普通 Operator Scope。 | FROZEN |
| TD-FRZ-341 | Permission Matrix 为 Code-owned；V1 不建设任意字符串权限设计器，数据库 Scope 只能映射 Backend 已注册 Permission Key。 | FROZEN |
| TD-FRZ-342 | 所有 Ops REST / Command 权限必须服务端重新检查；隐藏 Sidebar、Disabled Button、Frontend Guard 均不能代替授权。 | FROZEN |
| TD-FRZ-343 | Admin Principal 使用 Durable `authz_epoch`；TD-10-C01 冻结 Role / Scope / Enabled 改动后立即递增 Epoch，使旧 Operations Authorization 失效。 | FROZEN |
| TD-FRZ-344 | Operations 权限撤销默认不撤销普通用户站点登录，除非是身份恢复 / Rebind 等安全操作触发 User Security Epoch。 | FROZEN |
| TD-FRZ-345 | NewAPI Admin 与 Chaldea Super Admin 权限继续双向独立；实际 NewAPI Admin 判断机制必须按部署源代码 `SOURCE VERIFICATION REQUIRED`。 | FROZEN |
| TD-FRZ-346 | Access Control 禁止移除 / 降级最后一位 Active Super Admin，保持至少一名可恢复 Chaldea Operations 的 Super Admin。 | FROZEN |
| TD-FRZ-347 | 风险等级保持 Level 1 Routine / Level 2 Impactful / Level 3 Critical；Level 3 继续强制 Fresh Auth + Reason + Typed Confirmation + Impact Preview + Unique Operation ID + Audit。 | FROZEN |
| TD-FRZ-348 | Level 3 Fresh Authentication 必须在实际提交时仍处于 TD-02 10 分钟窗口；过期 Preview 不可继续执行。 | FROZEN |
| TD-FRZ-349 | 所有管理写操作建议统一生成 `operation_id`；Critical Operation 通过 Durable `ops.admin_operations` 支持重试、结果查询和跨系统恢复。 | FROZEN |
| TD-FRZ-350 | Impact Preview 必须由服务器从当前权威事实生成并绑定 Hash；提交时重新验证事实，发生 TOCTOU 时返回 PREVIEW_STALE。 | FROZEN |
| TD-FRZ-351 | Production Level 3 Typed Confirmation 必须绑定 Environment / Target / Operation，不使用全站永久单一 `CONFIRM` 作为确认语义。 | FROZEN |
| TD-FRZ-352 | Operations Environment Identity 来自部署配置并持续显示 Production / Staging / Development；前端参数不得改变操作目标环境。 | FROZEN |
| TD-FRZ-353 | 同库 Admin Mutation 与 Audit 必须处于同一 DB Transaction；Audit 写入失败时 Critical Mutation 回滚。 | FROZEN |
| TD-FRZ-354 | 跨 NewAPI / Chaldea 的 Critical Operation 使用 Durable Operation State，而不假装跨库原子；未知远端结果先查询权威状态，禁止盲目重复执行。 | FROZEN |
| TD-FRZ-355 | Audit Before / After 必须通过安全序列化器脱敏；任何 Role 均不得使密码、API Key Secret、未授权 Prompt/Response、未公开 Hole Card / Seed 进入 Audit。 | FROZEN |
| TD-FRZ-356 | Audit 为 Append-only；Runtime / Operations 无 UPDATE / DELETE / TRUNCATE 能力，V1 不配置业务 TTL 删除管理员审计。 | FROZEN |
| TD-FRZ-357 | 金融撤销必须通过新的 Reversal / Compensation Operation + Ledger + Audit，不能删除原 Mutation / Ledger / Audit。 | FROZEN |
| TD-FRZ-358 | Global Search 只索引已冻结安全 Object Keys / Stable IDs，并在服务端 Permission Filter 后返回；秘密字段永不进入 Search Index。 | FROZEN |
| TD-FRZ-359 | Operations Detail 使用稳定 Deep Link；Needs Attention / Audit / Incident 均通过 Stable Target ID Cross-link，复杂详情不得只存在临时 Drawer。 | FROZEN |
| TD-FRZ-360 | Needs Attention 是可重建 Operational Projection，按 Source ID + Reason 去重，并使用 Critical / Warning / Info；它不成为底层问题 Authority。 | FROZEN |
| TD-FRZ-361 | Attention Acknowledge 仅表示已查看；Resolved 必须由底层问题实际消失或正式 Repair / Incident Outcome 驱动，未解决金融 / Poker 问题不得永久隐藏。 | FROZEN |
| TD-FRZ-362 | Incidents 使用 Durable State + Append-only Timeline；Incident 本身不直接编辑资产、Round、Poker Hand 或 Ranking Source。 | FROZEN |
| TD-FRZ-363 | Support Case 延续 Open → Verifying → Approved/Rejected → Executed → Closed，并只保存完成验证所需的最少安全证据。 | FROZEN |
| TD-FRZ-364 | Discord Rebind 必须基于 Approved Support Case、原账号 Ownership、新 Discord Uniqueness、Super Admin、Reason 与 Fresh Auth；实际 NewAPI Binding Write 需 SOURCE VERIFICATION REQUIRED。 | FROZEN |
| TD-FRZ-365 | TD-10-C02：成功 Discord Rebind / Legacy Account Recovery 后递增目标用户 Security Epoch，撤销现有 Chaldea Sessions / Poker Connect 能力并要求重新认证，不改变余额、Keys 或历史归属。 | FROZEN |
| TD-FRZ-366 | Legacy Password Recovery 不允许管理员查看、指定或保存用户最终密码；Chaldea 只保存 Support / Authorization / Audit，实际 Auth Reset 走 Source-verified NewAPI Contract。 | FROZEN |
| TD-FRZ-367 | Economy Wallet / Ledger 在 Operations 默认只读；Reconciliation 只允许状态机合法 Retry / Resume / Compensate / Mark for Review，不允许强制 Confirm 或直接 Balance Patch。 | FROZEN |
| TD-FRZ-368 | Admin Adjustment 保持 Super Admin-only，并继承 TD-04 Before/Delta/After、Reason/Reference、Fresh Auth、Typed Confirm、Ledger/Audit 不变量。 | FROZEN |
| TD-FRZ-369 | Games Operator 只处理 Metadata / Catalog / Safe Runtime / Draft-Validate-Preview；经济配置 Activation 继续 Super Admin-only，Active / Historical Config 不直接编辑。 | FROZEN |
| TD-FRZ-370 | Poker Operations 只能执行 Stop/Resume Admission、Boundary Remove、Mute、Pause、Recovery 等正式动作；Stack/Pot/Winner/Deck/Settlement/Hole Card/Seed 永远不可编辑或提前查看。 | FROZEN |
| TD-FRZ-371 | Poker Emergency Pause 是 Level 3 Super Admin Operation，不改变 Hand 事实，并继续从 PostgreSQL Authority 恢复。 | FROZEN |
| TD-FRZ-372 | Ranking Operator 可以构建 / 检查 Repair Shadow，但关键 Repair Snapshot 最终 Publish 归 Super Admin；Records Scope 仍默认只读并通过 Incident 进入 Repair。 | FROZEN |
| TD-FRZ-373 | Service Health 是业务级 Operational Projection，覆盖 Chaldea/NewAPI/Poker/PG/Redis/Workers；Health Failure 不自动修改资产或牌局正式状态。 | FROZEN |
| TD-FRZ-374 | Maintenance Durable Authority 为 PostgreSQL；缓存可用于加速但 Redis / Frontend State 不得成为维护开关真相。 | FROZEN |
| TD-FRZ-375 | Maintenance Scope 固定为 Chaldea User Writes / Wallet & Exchange / Rewards / Direct Play New Rounds / Poker New Tables & New Hands / Rankings Publishing / Announcements Scheduling；NewAPI 模型 API Maintenance 不纳入。 | FROZEN |
| TD-FRZ-376 | Maintenance 使用 Draft / Scheduled / Active / Ending / Completed，并支持 pre-activation Cancelled 与显式 Failed 状态；所有 Maintenance Create / Activate 均要求 Reason / Impact Preview / Fresh Auth。 | FROZEN |
| TD-FRZ-377 | Maintenance Risk 采用 Scope 最大风险：全站 User Writes、Wallet/Exchange 与 Poker Emergency Pause 按 Critical；普通单游戏/发布型 Scope 为 Impactful，同时所有 Maintenance 仍要求 Fresh Auth。 | FROZEN |
| TD-FRZ-378 | Maintenance Gate 必须由 Backend 在 New-work Admission 时检查；Frontend 禁用按钮不能替代维护权限控制。 | FROZEN |
| TD-FRZ-379 | 不同 Maintenance Scope 可以重叠，Effective Gate 为所有 Active Scope 并集；结束一个 Window 不清除其他 Window。V1 禁止同一 exact Scope 的 Maintenance 时间窗口相互重叠。 | FROZEN |
| TD-FRZ-380 | `CHALDEA_USER_WRITES` 只阻断新的普通 Chaldea Mutations；Accepted-work Recovery、Settlement、Compensation、Safe Leave / Cash Out 等安全完成路径继续运行。 | FROZEN |
| TD-FRZ-381 | Wallet Maintenance 阻止新 Exchange，但 Existing Saga / Reconciliation / Compensation 继续；Rewards Maintenance 阻止新普通 Claim，但已有 Claim / Initial Grant Recovery 继续。 | FROZEN |
| TD-FRZ-382 | Direct Play Maintenance 只阻止新 Round；Poker Maintenance 阻止新 Table / Seat / Buy-in / Hand，但 Active Hand Recovery、Safe Leave 与 Cash Out 继续。 | FROZEN |
| TD-FRZ-383 | Rankings Publishing Maintenance 只暂停 Publish Pointer；Source Ingestion / Shadow Build 可继续。Announcements Scheduling Maintenance 暂停未来 Publish Transition，但实时 Expiry / Withdraw 可见性继续执行。 | FROZEN |
| TD-FRZ-384 | Maintenance 不自动 Refund Round、Cash Out 全部 Poker、Compensate 全部 Transfer 或 Cancel Reward；Accepted Business 继续由原 Domain State Machine 决定。 | FROZEN |
| TD-FRZ-385 | Scheduled Maintenance 使用 TD-09 Durable Job Framework；Schedule + Maintenance Fact + Audit 原子创建，到点由 Server DB Time 激活。 | FROZEN |
| TD-FRZ-386 | Maintenance Impact Preview 必须展示真实受影响业务数量与“将阻断 / 将继续”清单；不可用指标显示 Unavailable，不得伪造为零。 | FROZEN |
| TD-FRZ-387 | Chaldea Operations 永不提供 SSH、Docker Shell、任意 Command、SQL Console、Redis Console、Package Upgrade 或 VPS Firewall Management。 | FROZEN |
| TD-FRZ-388 | PC / Tablet / Mobile 使用同一 RBAC 与 Operation Guard；Mobile 不能因界面限制省略 Critical Operation Fresh Auth / Impact / Typed Confirmation。 | FROZEN |
| TD-FRZ-389 | TD-10 Implementation 必须通过 Role/Scope Bypass、Immediate Revocation、Last Super Admin、Critical Operation Idempotency、Audit Immutability/Redaction、Maintenance Accepted-work Protection、Incident/Attention 与 Secret-boundary Test Gate。 | FROZEN |

---

# 639. Change Log — WORKING v1.0

## Added

- 用户正式确认 TD-10；
- 冻结 `TD-FRZ-338 ～ TD-FRZ-389`；
- 冻结 Chaldea Admin Principal；
- 冻结三角色与八 Operator Scope；
- 冻结 Code-owned Permission Matrix；
- 冻结 `authz_epoch` 即时撤权；
- 冻结 Last Super Admin Guard；
- 冻结 Level 1/2/3 Operation Guard；
- 冻结 Fresh Auth / Impact Preview / Typed Confirmation；
- 冻结 Production Environment Binding；
- 冻结 Durable Admin Operation；
- 冻结 Same-DB Audit Atomicity；
- 冻结 Cross-DB Critical Operation Recovery；
- 冻结 Audit Redaction / Append-only；
- 冻结 Global Search Security；
- 冻结 Needs Attention / Incident / Support Case；
- 冻结 `TD-10-C02` Rebind/Recovery Session Reset；
- 冻结 Economy / Games / Poker / Rankings / Records / Announcements Operations Boundary；
- 冻结 Service Health Projection；
- 冻结七种 Maintenance Scope；
- 冻结 Maintenance Durable Authority / Lifecycle / Scope Union；
- 冻结 Same-scope Maintenance Overlap Protection；
- 冻结 Accepted-work Protection；
- 冻结 Scheduled Maintenance；
- 冻结 No Infrastructure Console；
- 冻结 TD-10 Crash / Security / Permission / Maintenance Test Gate。

## Existing Open Items Preserved

```text
TD-05 Reward OPEN fields

POKER-PROD-GAP-01
POKER-PROD-GAP-02
POKER-PROD-GAP-03
POKER-PROD-GAP-04
POKER-PROD-GAP-05

TD-09-PROD-GAP-01
```

---

# 640. 下一批 — TD-11

下一批正式进入：

> **TD-11 — Frontend Technical Architecture**

计划冻结：

1. React + Vite App Architecture；
2. Public / Auth / Operations App Boundary；
3. Route Families；
4. BFF Contract Consumption；
5. Query / Server State；
6. Client UI State；
7. Auth Bootstrap；
8. Global Gate Bootstrap；
9. Return-to-Intent；
10. Error Boundary；
11. Loading / Processing State；
12. Asset Amount Serialization；
13. Form Architecture；
14. Mutation Idempotency Key；
15. Data Freshness；
16. Optimistic Update Policy；
17. Realtime Poker Client；
18. Reconnect / Snapshot / Event Sequence；
19. Take Over Client State；
20. Direct Play Round Resume；
21. Animation vs Business State；
22. Responsive Layout Contract；
23. PC / Tablet / Mobile Shell；
24. Operations Frontend Permission Projection；
25. Accessibility；
26. Reduced Motion；
27. Media / Asset Loading；
28. Performance Budget；
29. Security Headers / CSP Frontend Impact；
30. Testing；
31. Deployment / Cache Busting；
32. Art Direction Token Mapping。

特别注意：

- Frontend 不成为任何资产 / Reward / Game / Poker Authority；
- React Query / client cache 只能是 Projection；
- 不能用 JS `Number` 保存资产 BIGINT 字符串；
- Poker WebSocket State 必须从 Server Snapshot 恢复；
- Skip Animation 不能改变游戏结算；
- Mobile 和 PC 使用同一正式业务状态，不建立两套业务逻辑；
- Operations 前端隐藏权限只是 UX，服务端授权仍是 Authority；
- 视觉继续严格读取 Art Direction v0.4 FINAL，不重新讨论已冻结视觉方向。



---

# 641. TD-11 — Frontend Technical Architecture

> 状态：`FROZEN`  
> 用户确认：`整体按上述方案通过`

## 641.1 TD-11 总体结论

本批正式冻结：

- Single React + Vite SPA；
- Route-level Lazy Chunk；
- Route / Feature / Design System / Realtime / Generated Contract 分层；
- Public / Protected / Admin / Immersive Route Metadata；
- Public / Auth 同路由增强；
- Dynamic Game Runtime Adapter；
- URL State / Server State / Form State / Presentation State / Poker Realtime State 分离；
- Chaldea BFF Typed Client；
- TanStack Query 等价 Server Cache；
- Mutation Idempotency / Unknown Result Recovery；
- Asset String / BigInt Boundary；
- Unified Gate Controller；
- Direct Play Resume-first；
- Fast-settlement Presentation Separation；
- Blackjack Authoritative Action；
- Dedicated Poker Realtime Store；
- Snapshot / Event Sequence / Reconnect / Take Over；
- No Offline Mutation Queue / No Service Worker；
- Responsive Token Engineering；
- Design Token / CSS / Font / Media Engineering；
- Reduced Motion / Reduced Media；
- WCAG 2.2 AA；
- Asset Manifest / Rights / Media Failure；
- Static Build Cache / Chunk Mismatch Recovery；
- Test Pyramid / A11y / Visual Review；
- Frontend Production Release Gate。

本批不重新设计：

- IA 路由；
- PC / Mobile Navigation；
- Casino A / Bright Moonlit；
- Royal Beacon；
- Button System；
- Poker / Operations Shell；
- 游戏数学；
- Poker 规则；
- TD-05 / TD-08 / TD-09 仍 OPEN 的产品规则。

---

# 642. Frontend Application Shape

V1 采用：

```text
Single React + Vite SPA
+
Route-level Code Splitting
```

同一 Application 承载：

```text
Public Routes
Authenticated Routes
Direct Play Routes
Poker Immersive Route
Operations Routes
```

不拆第二套 Operations Frontend。

不引入 SSR / Next 类架构。

原因：

- 上游已经冻结 React + Vite Static Build；
- 当前 10–50 人规模没有 SSR 必要性；
- 复用 Auth / Gate / Design System / Error / Accessibility；
- Operations / Poker 可通过 Lazy Chunk 获得足够隔离。

---

# 643. Frontend Source Structure

建议：

```text
frontend/
├── src/
│   ├── app/
│   │   ├── bootstrap/
│   │   ├── router/
│   │   ├── gates/
│   │   └── providers/
│   │
│   ├── routes/
│   │   ├── public/
│   │   ├── authenticated/
│   │   ├── games/
│   │   ├── poker/
│   │   └── operations/
│   │
│   ├── features/
│   │   ├── auth/
│   │   ├── master/
│   │   ├── models/
│   │   ├── api/
│   │   ├── wallet/
│   │   ├── rewards/
│   │   ├── games/
│   │   ├── rankings/
│   │   ├── history/
│   │   ├── announcements/
│   │   └── operations/
│   │
│   ├── realtime/
│   │   └── poker/
│   │
│   ├── design-system/
│   ├── media/
│   ├── generated/
│   ├── shared/
│   └── styles/
│
└── public/
```

原则：

```text
Route owns composition
Feature owns business-facing UI logic
Design System owns primitives/tokens
Realtime owns Poker live projection
Generated owns protocol/API types
Shared owns truly generic infrastructure
```

---

# 644. Frontend State Categories

不采用单一巨型 Global Store。

正式拆分：

```text
1. URL / Navigation State
2. HTTP Server State
3. Form / Draft State
4. Local Presentation State
5. Poker Realtime Projection State
6. User Media / Accessibility Preferences
```

不同类别使用不同工具和生命周期。

---

# 645. Route Authority

严格保持 IA FINAL：

```text
/
 /dashboard

/login
/register
/onboarding/master

/models
/models/:model
/api/keys
/api/usage
/api/access

/wallet
/rewards

/entertainment
/games
/games/:game_slug

/poker
/poker/table/:id

/rankings

/history
/history/round/:id
/history/session/:id
/history/hand/:id

/announcements
/announcements/:id

/me
/master-profile
/account/security

/ops/*
```

Frontend 不擅自创建第二套路由。

---

# 646. Route Classification Metadata

Router 维护 Code-owned Metadata：

```text
route_class:
  PUBLIC
  PROTECTED
  ADMIN
  IMMERSIVE

product_domain
safe_parent_route
required_capability
required_ops_permission
shell_type
```

例如：

```text
/models/:model
→ PUBLIC

/wallet
→ PROTECTED

/ops/rankings
→ ADMIN

/poker/table/:id
→ IMMERSIVE + PROTECTED
```

不只按路径前缀推断权限。

---

# 647. Router Technology

推荐 React Router 等价实现，负责：

```text
Nested Routes
Lazy Route Modules
Search Params
Navigation
Route Error Boundaries
Scroll Restoration
```

具体版本在 Implementation Spec 锁定。

---

# 648. Public / Auth Route Reuse

例如 `/models`：

```text
Anonymous
→ Public Model Square

Authenticated
→ Same Page + Auth Capabilities / Context Navigation
```

不建立：

```text
/public/models
/app/models
```

两套页面。

---

# 649. Dynamic Game Frontend Adapter

`/games/:game_slug` 不以固定五款枚举定义 Catalog 容量。

流程：

```text
Game Registry Bootstrap
→ implementation_key
→ Code-owned Frontend Runtime Adapter
```

V1 注册：

```text
dice.v1
scratch.v1
summon.v1
slot.v1
blackjack.v1
```

DB 不能加载任意前端脚本成为游戏。

---

# 650. URL State

以下进入 Search Params：

```text
Tab
Filter
Sort
Pagination
Period
Model Filter
History Filter
Ranking Metric
Search Query
```

便于：

```text
share
back/forward
detail return
restore list context
```

---

# 651. State Forbidden from URL

禁止：

```text
Password
Typed Confirmation
Wallet Exchange Draft
Reward Submit State
Poker Action Input
Poker Connect Ticket
Private Hole Cards
Server Seed
```

也不把整棵 React State JSON 放入 Query String。

---

# 652. Scroll / Focus Restoration

List → Detail：

```text
URL preserves filter/page
history entry preserves scroll key
```

Back：

```text
restore safe list scroll
restore focus to originating item if still present
```

Route Navigation：

```text
focus main heading / main region
```

Dialog / Drawer / Sheet：

```text
focus trap
focus return
```

---

# 653. Shell Families

## Normal Product Shell

```text
Global Header
Context Navigation
Page
Mobile Bottom Navigation
```

## Poker Immersive Shell

```text
Poker Top Controls
Poker Table
Action Area
No normal Global Header
No Mobile Bottom Navigation
```

## Operations Shell

```text
Operations Sidebar
Operations Top Bar
Operations Content
```

Direct Play 默认使用 Normal / Focused Shell；Poker Table 使用 Immersive。

---

# 654. HTTP BFF Boundary

Browser 普通 HTTP 业务只调用：

```text
Chaldea Platform Backend
```

禁止 Browser 直接：

```text
NewAPI internal admin API
NewAPI database
PostgreSQL
Redis
```

Poker WS 继续：

```text
Browser → Poker WebSocket Service
```

Connect Ticket 由 Platform Backend 签发。

---

# 655. Public Runtime Config

Frontend Build / Runtime 只允许公开配置：

```text
WEB_ORIGIN
API_ORIGIN
POKER_WS_ORIGIN
BUILD_ID
ENVIRONMENT
```

任何 Secret：

```text
DB password
OAuth secret
provider credential
signing key
```

不得进入 Vite Client Environment。

---

# 656. Typed BFF Client

最终 TD-13 Contract 生成：

```text
TypeScript Request Types
Response Types
Error Envelope
Enums
```

建议：

```text
Go/OpenAPI Contract
→ generated TS types
→ thin domain client
→ feature hooks
```

协议层禁止 `any`。

---

# 657. Typed Poker Protocol

Poker WS 同样生成：

```text
Command Envelope
Event Envelope
Snapshot
Domain Errors
Version Fields
```

JSON Parse 后必须经过 Typed/Schema Boundary。

---

# 658. HTTP Server State

推荐 TanStack Query 等价方案承载：

```text
Read Cache
Request Deduplication
Stale / Fetch State
Invalidation
Background Refetch
```

Query Cache：

```text
Projection only
```

不是业务 Authority。

---

# 659. Query Cache Persistence

V1 不把整个 Query Cache 持久化到 LocalStorage / IndexedDB。

Refresh：

```text
refetch Server
```

理由：

- 避免长期保存敏感个人数据；
- 避免过时资产 / 权限 / Reward 状态；
- 减少 Client Schema Migration。

---

# 660. Query Key Discipline

例如：

```text
["wallet","summary"]
["wallet","transactions",filters]

["reward","status"]

["game",slug,"bootstrap"]
["game",slug,"round",roundId]

["rankings",domain,metric,period,model]

["history",filters]

["ops","attention",filters]
```

禁止使用无法精确 Invalidate 的通用 `["data"]`。

---

# 661. Read Retry

Read Query 可使用有限：

```text
retry
+
backoff
```

只针对瞬时 Network / selected 5xx。

不得自动重试：

```text
401
403
domain 404
409 conflict
```

具体次数进入 Implementation Config。

---

# 662. Mutation Automatic Retry

禁止 Generic Client 自动重发：

```text
Exchange
Reward Claim
Round Create
Blackjack Action
Poker Buy-in
Admin Operation
Admin Adjustment
```

Network Result Unknown：

```text
query original business identity
```

之后再按 Domain Contract 收敛。

---

# 663. Client Idempotency Key

每个新的 User Intent：

```text
generate random idempotency key
```

在：

```text
SUBMITTING
PROCESSING
UNKNOWN_RESULT
retry same intent
```

期间保持不变。

只有终态后用户明确发起新动作才生成新 Key。

---

# 664. Mutation UI Lifecycle

统一：

```text
IDLE
→ SUBMITTING
→ ACCEPTED / PROCESSING
→ CONFIRMED
```

异常：

```text
FAILED
RETURNED
NEEDS_ATTENTION
UNKNOWN_RESULT
```

`UNKNOWN_RESULT` 是正式状态，不等于 Failed。

---

# 665. Unknown Result Recovery

Timeout / Connection Loss 后：

```text
UNKNOWN_RESULT
→ query by operation_id / round_id / transfer_id / claim_id / action identity
→ CONFIRMED / PROCESSING / NO_EFFECT
```

禁止直接创建第二笔操作。

---

# 666. Optimistic Truth Policy

以下绝不做未经 Server Confirm 的乐观真相更新：

```text
Wallet Balance
Reward Balance
Total Assets
Round Result
Poker Stack
Pot
Ranking Score
Admin Permission
Maintenance State
```

允许的 Optimistic UI 仅限低风险 Presentation，如：

```text
Accordion
Tab animation
Local media preference
Input selection
Local chat block
```

---

# 667. Asset Transport

所有资产金额 API 使用：

```text
string
```

例如：

```json
{
  "units": "500000000",
  "display": "1000"
}
```

禁止使用 JS Number 作为资产协议类型。

---

# 668. Frontend Asset Types

建议 Branded Types：

```text
AtomicUnitString
DecimalAmountString
```

协议缓存优先保留 String。

精确整数比较可受控使用：

```text
BigInt(atomicUnitString)
```

但 JSON Transport 不使用 BigInt。

---

# 669. JS Number Prohibition

资产路径禁止：

```text
Number(balance)
parseFloat(balance)
Math.round(balance * ratio)
```

覆盖：

```text
API Credit
Chips
Wager
Payout
Stack
Pot
Admin Adjustment
```

---

# 670. Amount Parser / Formatter

建立唯一：

```text
parseDisplayAmountToAtomic()
formatAtomicAmount()
```

Parser 必须拒绝：

```text
scientific notation
NaN / Infinity
excess precision
invalid separators
illegal negative
```

Formatter 支持：

```text
API Credit precision
Chip display
Poker whole-chip
trailing-zero trim
large grouping
```

不经 JS Number。

---

# 671. Poker Amount Input

Poker 输入保持：

```text
integer chip string
```

Frontend 只做明显格式检查。

合法最小 / 最大 / Raise-to / Stack 限制最终由 Server 判断。

---

# 672. Form Architecture

推荐统一：

```text
Form State
+
Schema Validation
+
Server Validation
+
Domain Error Mapping
```

可采用 React Hook Form + Schema Validator 等价组合。

Client Validation 不替代：

```text
Balance
Permission
Eligibility
Maintenance
Version
```

---

# 673. Safe Draft Registry

允许 Session Expired 后恢复的非敏感 Draft 通过显式 Allowlist：

```text
SafeDraftRegistry
```

最多暂存于 Session Storage，并带短 TTL。

---

# 674. Never-persist Form Fields

禁止写入 URL / LocalStorage / SessionStorage：

```text
Password
OAuth token
Poker Connect Ticket
API Key Secret
Critical Typed Confirmation
Private Server Seed
Hole Cards
```

Level 3 Admin Form 也不自动持久化。

---

# 675. Auth Bootstrap

Root Session Bootstrap 返回最低必要：

```text
anonymous / authenticated
account status
master initialization status
migration notice pending
safe user summary
resource/maintenance summary
Ops principal summary if applicable
```

Exact Endpoint 留 TD-13。

---

# 676. Public Route Bootstrap

Public Route 不被 Auth Bootstrap 阻塞。

例如：

```text
/
 /models
 /games
 /rankings
 /announcements
```

先允许 Public Content 正常读取。

Session Bootstrap 只增强登录体验。

---

# 677. Protected Route Bootstrap

Protected Route：

```text
Route Classification
→ Session Bootstrap
→ Unified Gate
→ Feature Bootstrap
```

Gate 完成前不渲染 Protected Business Data。

---

# 678. Unified Frontend Gate

严格：

```text
Requested Route
→ Route Classification
→ Entry Popup Check where applicable
→ Public/Protected/Admin/Immersive Access
→ Authentication
→ Account Status
→ Master Initialization
→ Migration Notice
→ Role / Scope
→ Resource Availability
→ Return-to-Intent
→ Deferred Post-login Popup
```

不在各 Feature 重复实现完整 Auth Gate。

---

# 679. Return-to-Intent

使用 Server-side Safe Intent。

Frontend 只恢复：

```text
safe route
safe filters
safe page position
```

永不自动重放：

```text
Wager
Poker Buy-in / Cash Out
Exchange
Reward Claim
API Key Mutation
Profile Save
Password Change
Admin Write
```

---

# 680. Error Taxonomy

统一：

```text
401 → Reauthenticate + Safe Intent
403 → Access Denied + Safe Parent
404 → privacy-preserving Not Found
409 → recoverable domain conflict
429 → retryable state without threshold leak
503 → affected module + available modules + maintenance info
```

部分服务降级时不关闭无关产品域。

---

# 681. React Error Boundaries

Render / Lazy Chunk Error 与 HTTP Domain Error 分离。

至少：

```text
App Root Boundary
Route Family Boundary
Poker Boundary
Operations Boundary
```

Poker Frontend 崩溃不得伪造 Cash Out / Leave。

---

# 682. Loading / Empty / Notification

Read：

```text
Skeleton
or
explicit Loading
```

禁止永久 Fullscreen Spinner。

Empty State：

```text
why empty
+
safe next action
```

必须和 Load Failure 区分。

Notification 层：

```text
Inline
Toast
Persistent Banner
Dialog / Sheet
Full-page / Interstitial
```

Toast 不能作为资产 / Critical Operation 的唯一成功结果。

---

# 683. Direct Play Bootstrap

`/games/:game_slug` 使用单一 Bootstrap Contract 返回：

```text
Registry
Runtime State
Active Config Summary
Wager Policy
Wallet Summary
Active Round
Fairness Commitment
```

Browser 不自行拼多个 Endpoint 来推断可操作状态。

---

# 684. Resume-first

若：

```text
active_round != null
```

Frontend 必须优先 `RESUME`。

不能先给 New Wager UI，再晚到发现已有 Round。

---

# 685. Create Round Flow

```text
READY
→ explicit user action
→ create idempotency key
→ SUBMITTING
→ Create Round
```

Fast-settlement Game：

```text
SETTLED authoritative result
→ presentation
```

Blackjack：

```text
BET_ACCEPTED / PLAYER_TURN
→ authoritative interactive state
```

---

# 686. Fast-settlement Presentation Separation

Dice / Scratch / Summon / Slot：

```text
Business Result
!=
Animation State
```

Frontend 保存：

```text
result DTO
+
presentation phase
```

Skip / Reduced Motion / Media Failure 只改变 Presentation。

不得：

```text
reroll
re-settle
re-credit
re-debit
```

---

# 687. Result Accessibility before Media

即使 Reaction / Animation 未加载，Authoritative Result 必须有稳定 DOM / Text Equivalent。

媒体失败不能让结果“消失”。

---

# 688. Scratch Presentation

Pixel Scratch Mask：

```text
local presentation state
```

可用 Canvas / SVG。

但正式：

```text
9 logical cells
prize tier
payout
```

来自 Server。

Refresh：

```text
same result
new local overlay allowed
```

Canvas 不作为 Accessibility Authority。

---

# 689. Blackjack Client

保存 Server Projection：

```text
round_id
round_version
hands
cards
dealer public state
active hand
legal actions
result
```

Action：

```text
action_id
expected_round_version
```

Frontend 不预测下一张牌。

Stale Action：

```text
discard pending local command
apply authoritative snapshot
```

---

# 690. Poker Realtime Store

Poker Live State 不放进普通 Query Cache 当作实时 Authority。

建立：

```text
PokerRealtimeStore
```

推荐：

```text
useSyncExternalStore
+
typed internal reducer/store
```

以细粒度 Subscription 避免高频全树重渲染。

---

# 691. Poker Connection State

客户端连接态：

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

它和 Poker Hand 生命周期严格分离。

---

# 692. Poker Connect Ticket

流程：

```text
HTTP BFF
→ mint single-use Connect Ticket
→ WSS Upgrade
→ first auth frame
```

Ticket 只存在内存。

禁止：

```text
URL
LocalStorage
Console
Error Telemetry
```

---

# 693. Snapshot-first Poker

初次连接 / Reconnect：

```text
AUTHENTICATED
→ SYNCING
→ authoritative Snapshot
→ LIVE
```

第一个 Snapshot 之前 Player Action 禁用。

---

# 694. Poker Event Sequence

Store 保存：

```text
last_event_seq
table_version
hand_version
```

Event：

```text
expected seq
→ apply

duplicate
→ ignore

gap
→ disable action
→ SYNCING
→ fetch authoritative snapshot
```

---

# 695. Poker Projection Rule

Event Handler 只能 Apply Server Facts。

禁止从：

```text
"Player raises"
```

本地自己计算正式：

```text
Pot
Stack
Legal Actions
```

这些都由 Server Projection 更新。

---

# 696. Poker Action

Client Command：

```text
action_id
expected_hand_version
control_epoch
action_type
requested amount
```

点击后本地只进入 Pending。

真正结果由 Server Event / Snapshot 决定。

---

# 697. Poker Unknown Delivery

Action Send 后断线：

```text
do not generate new Action ID automatically
```

Reconnect + Snapshot 后检查正式 Hand/Action 状态。

必要 Retry 复用原 Action Identity。

---

# 698. Poker Timer

Authority：

```text
server action_deadline_at
```

Snapshot / Event 同时含：

```text
server_time
```

Client 只显示估算剩余时间。

本地倒计时归零不执行 Fold/Check。

---

# 699. Background Tab / Sleep

恢复可见性时：

```text
visibilitychange
→ verify socket/state
→ sync if necessary
```

不能因浏览器 Timer 暂停就认为服务端 Timer 也暂停。

---

# 700. Poker Reconnect / Service Recovery

Reconnect：

```text
bounded exponential backoff + jitter
```

连接未知时禁用 Action，但保留最后 Snapshot 并显示 Network State。

Service Failure 后：

```text
PAUSED / RECOVERING
→ reconnect grace
→ new server deadline
```

Frontend 不自行补时间。

---

# 701. Take Over

第二设备显示显式 Take Over。

Server 增加 `control_epoch` 后：

```text
new device = controller
old device = read-only
```

旧设备立即移除可操作 Controls。

---

# 702. Poker Orientation

Portrait / Landscape：

```text
layout only
```

不创建：

```text
new socket
new session
new hand snapshot identity
new timer
```

Mobile Action Tray 始终保持高可见，不被 Chat Sheet 覆盖。

---

# 703. Offline Mutation Boundary

V1 不实现：

```text
PWA offline business mutation queue
```

禁止离线自动排队后提交：

```text
Wager
Exchange
Reward Claim
Poker Action
Admin Operation
```

---

# 704. Service Worker

V1 不启用 Service Worker。

理由：

- 避免旧 `index.html` / Chunk 混用；
- 避免业务 Request 意外缓存；
- 避免 Offline Replay；
- 当前无离线产品需求。

普通 HTTP Cache 足够。



# 705. Responsive Breakpoints

严格：

```text
1100px
720px
420px
```

1100 以下 Compact Desktop/Tablet，720 以下 Mobile，420 以下 Ultra-narrow Reflow。

Tablet 不形成第三套业务 IA。

---

# 706. Product Layout Tokens

实现：

```text
Content Max Width = 1200px

Desktop Gutter = 24px
<=1100 = 16px
<=720 = 14px

Page Top:
Desktop = 52px
Mobile = 30px

Page Gap:
Desktop = 28px
Mobile = 20px
```

Navigation：

```text
Global Header Desktop = 72px
Global Header Mobile = 60px
Context Nav = 46px
Mobile Bottom Nav >=70px + safe-area
```

---

# 707. Operations Shell Tokens

实现：

```text
Desktop Sidebar = 252px
Top Bar = 76px
<=1100 Sidebar = 88px
<=720 Sidebar hidden → Drawer
```

继续使用已冻结 Operations Shell。

---

# 708. Shared Business Logic across Responsive Variants

禁止长期维护：

```text
DesktopWallet.tsx
MobileWallet.tsx
```

两套业务逻辑。

推荐：

```text
WalletRoute
→ shared query/mutation/business hooks
→ responsive presentation
```

必要时拆 Desktop / Mobile Presentation，但 Contracts / Hooks 共用。

---

# 709. Mobile Navigation

登录 Mobile 固定：

```text
首页 → /dashboard
模型 → /models
娱乐 → /entertainment
资产 → /wallet
我的 → /me
```

Direct Play 默认保留普通 Shell / Bottom Navigation。

Poker Table 始终 Full Immersive、无 Bottom Navigation。

---

# 710. Design Token Engineering

建立单一版本化 Token Source：

```text
src/design-system/tokens/
```

生成：

```text
CSS Custom Properties
TypeScript Token Metadata
Test / Story Fixtures
```

Feature 不散落未批准的颜色、断点、Shadow、Radius。

---

# 711. Button Three-color Boundary

用户确认的：

```text
Chaldea Ivory   #F4F0E8
Royal Azure     #3568B7
Moonlit Mid     #95ACD0
```

三色纪律只作用于 Button System。

不能将其错误提升为全站只允许三色。

Semantic / Data Colors 继续使用 Art Direction FINAL 定义。

---

# 712. CSS Architecture

推荐：

```text
CSS Custom Properties
+
Static / Scoped Component CSS
```

避免大量 Runtime CSS-in-JS。

原因：

- CSP 更简单；
- Runtime 成本更低；
- 更直接读取 Design Tokens；
- Vite Static Build 友好。

---

# 713. Surface / Shadow

普通：

```text
Solid Surface
1px Outline
No normal box-shadow
```

Shadow 只用于：

```text
Dialog
Drawer
Popover
```

Radius 使用冻结：

```text
8 / 14 / 22px
```

---

# 714. Fonts

Self-host：

```text
IBM Plex Sans SC
Noto Serif SC
Marcellus
IBM Plex Mono
```

Web：

```text
WOFF2 Preferred
font-display: swap
```

只 Preload 当前首屏真正需要的 Functional Font。

Display / Decorative 字体按需加载。

---

# 715. Media/UI Layer Separation

永久：

```text
background_plate
character_layer
game_object_layer
foreground_atmosphere
real HTML / CSS / SVG UI
```

图片 / 视频不得承载：

```text
Balance
Button
Authoritative Result
Poker Private Card
Wager
Menu
Ranking
Login Form
```

---

# 716. Asset Manifest

组件不直接散落物理媒体路径。

统一：

```text
asset_id
→ generated asset manifest
→ source / fallback / focal point / rights / status
```

至少继承：

```text
asset_id
domain
scene
character
skin
role
version
source
fallback
desktop_focal_point
mobile_focal_point
safe_area
alpha
status
rights_note
prompt_archive_id
```

---

# 717. Production Asset Gate

Production 页面正式资产不得引用：

```text
REJECTED
REFERENCE_ONLY
RIGHTS_REVIEW_REQUIRED
```

作为已批准生产文件。

非生产 Review Environment 可按显式规则使用中间状态。

---

# 718. Media Formats

优先：

```text
Background → AVIF + WebP fallback
Character → WebP alpha + fallback
Vector → SVG
Reaction → transparent WebM + static WebP fallback
```

业务在所有媒体失败时仍必须完整可操作。

---

# 719. Media Geometry

所有：

```text
Persona
Hero
Background
Casino Stage
Game Object
Reaction Placeholder
```

预留：

```text
width / height
aspect-ratio
```

Skeleton 几何接近最终内容，防止媒体加载后把 Pricing/Form/Wager/Action/Result 大幅顶移。

---

# 720. Media Preload / Lazy Load

只 Preload：

```text
Functional UI Font
Current Route true LCP Hero / Background
optional one first-screen Character
```

Lazy Load：

```text
later Persona
Reaction
other Casino Scenes
other Game assets
```

进入 Entertainment Hub 不得预载所有游戏媒体。

---

# 721. Frozen Media Budget

严格继承：

```text
Card Persona              <=250KB typical
Detail Persona            <=500KB typical

Desktop Hero AVIF         <=550KB target
Desktop Hero WebP         <=800KB fallback

Mobile Hero AVIF          <=400KB target
Mobile Hero WebP          <=600KB fallback

Transparent Reaction WebM <=1.5MB target
>2.5MB                    mandatory review

Static Reaction WebP      <=350KB

Functional SVG            <=12KB typical
Complex Brand SVG         <=80KB target
```

首屏关键视觉：

```text
Mobile <=800KB
Desktop <=1.2MB
```

---

# 722. Reduced Motion

`prefers-reduced-motion: reduce`：

```text
M1 <=100ms Color/Border/Focus
M2 <=120ms Fade
M3/M4 remove long travel / particles / parallax / cut-in
Result <=150ms Fade + Static State
```

Reduced Motion 不自动等于 Mute。

---

# 723. Reduced Media

独立 Browser Preference：

```text
reduced_media
```

可本地持久化。

启用：

```text
do not load reaction video
do not load unnecessary casino media
use static compressed assets
```

不改变业务能力。

---

# 724. Media Failure Fallback

强制：

```text
Persona → Model Glyph / Family Geometry
Casino Background → Solid / CSS Surface
Reaction Video → Static Frame / Stable Result
Audio → Silent Result
Character → No-character layout
```

媒体失败不得阻断业务。

---

# 725. Accessibility Baseline

Production：

```text
WCAG 2.2 AA
```

对比度：

```text
Normal text       >=4.5:1
Large text        >=3:1
UI / Icon         >=3:1
Focus Indicator   >=3:1
```

---

# 726. Semantic HTML First

优先原生：

```text
button
a
input
select
table
heading
list
dialog
```

ARIA 只补充。

禁止大量用 `<div role="button">` 重造标准控件。

---

# 727. Keyboard Completeness

全部正式业务必须纯键盘完成：

```text
Wallet
Rewards
Direct Play
Blackjack
Poker
Operations
Dialogs
Sheets
```

禁止 Hover-only。

---

# 728. Focus

继续高可见：

```text
3px Royal Azure Focus Ring
```

Dialog / Drawer / Sheet：

```text
Focus Trap
Focus Return
```

Casino 背景不能覆盖或削弱 Focus Indicator。

---

# 729. Async Live Region

适度播报：

```text
Submitting
Processing
Completed
Failed
Recovering
```

Poker Timer 禁止每秒 Screen Reader 倒计时。

只在：

```text
your turn
important threshold
timeout/action result
```

提供必要提示。

---

# 730. Game / Poker Text Equivalent

必须有真实 DOM Text Equivalent：

```text
Dice:
values / total / result / payout

Scratch:
match / multiplier / payout

Summon:
tiers / total payout

Slot:
grid summary / winning lines / payout

Blackjack:
hand totals / dealer / result

Poker:
board / pots / current action / winner / settlement
```

动画、位置和颜色不得成为唯一信息渠道。

---

# 731. Zoom / Reflow / Touch

普通产品页面必须支持：

```text
200% Browser Zoom
~320 CSS px core reflow
```

游戏可特殊重排，但 Wager / Action / Result 不依赖横向滚动。

Touch Target：

```text
>=44px
```

Input：

```text
>=46px
```

---

# 732. Trusted Rich Text Boundary

Announcement Rich Text 只通过受信：

```text
TrustedRichText
```

组件渲染。

输入必须是 Server 已 Sanitized Payload，并携带版本信息。

普通 Feature 不允许随意 `dangerouslySetInnerHTML`。

Poker Chat 按纯文本渲染。

---

# 733. Operations Permission Projection

Ops Bootstrap 可返回：

```text
role
scopes
permission projection
authz_epoch
environment
```

Frontend 只用于 UX：

```text
navigation
action availability
confirmation level
```

Server Authorization 仍是 Authority。

---

# 734. AUTHORIZATION_STALE

如果 Ops Request 返回：

```text
AUTHORIZATION_STALE
```

Frontend：

```text
invalidate Ops Bootstrap
refetch role/scopes/epoch
rebuild navigation/actions
```

若已无 Ops 权限：

```text
leave /ops
→ safe user route
```

---

# 735. Level 3 Client State

以下不自动持久化：

```text
Admin Adjustment
Discord Rebind Execution
Access Control
Global Maintenance
Poker Emergency Pause
Economic Config Activation
Critical Repair Publish
```

离开页面销毁 Typed Confirmation Challenge。

重新进入必须重新 Preview。

---

# 736. Frontend Safe Logging

禁止：

```text
console.log(password)
console.log(apiKeySecret)
console.log(pokerConnectTicket)
console.log(privatePokerSnapshot)
```

Production Error Telemetry 不发送敏感 payload。

---

# 737. Error Telemetry Redaction

允许：

```text
build_id
route_id
safe error code
request reference id
browser capability
own frontend stack
```

不得 Dump：

```text
whole Query Cache
React State
Poker Snapshot
Form Payload
```

---

# 738. CSP Compatibility

Frontend 工程必须兼容：

```text
No eval
No arbitrary dynamic script injection
No third-party inline-script dependency
No arbitrary HTML execution
```

Exact CSP / HSTS / Permissions-Policy 在 TD-12 收口。

---

# 739. Static Build Cache

Vite Build：

```text
index.html
→ no-cache / must-revalidate

hashed JS/CSS/media
→ long immutable cache
```

新 Build 通过新的 Content Hash 生效。

---

# 740. Public Runtime Config

如需要：

```text
/config/public.json
```

只能包含：

```text
origins
build id
environment
public feature availability
```

永不包含 Secret。

---

# 741. Lazy Route Chunks

至少 Lazy：

```text
Operations
Poker Table
Each Direct Play Runtime
History Deep Detail
Large Admin Editors
```

Public Home 不下载全部 Poker / Operations / Casino Runtime Code。

---

# 742. Chunk Mismatch Recovery

旧页面请求已被新部署移除的旧 Chunk：

```text
detect chunk-load mismatch
→ one controlled full reload
```

只执行一次。

仍失败：

```text
show version/cache error
```

禁止 Reload Loop。

---

# 743. Core Web Vitals

Production Target：

```text
LCP <= 2.5s
INP <= 200ms
CLS <= 0.1
```

最终以真实生产环境为判定。

---

# 744. JS Bundle Budget

上游没有冻结具体 JS KB 数字。

TD-11 不随意写死。

Implementation Spec：

```text
measure production baseline
→ establish route-specific CI regression budgets
```

媒体预算仍严格使用 Art Direction v0.4 FINAL。

---

# 745. Rendering Performance

避免：

```text
whole app rerender on Poker timer
global Context containing all server data
ranking list animated at frame rate
```

Poker Timer 只更新 Timer Presentation。

Realtime Store 使用细粒度 selector / subscription。

---

# 746. Pagination

数据密集列表使用明确 Pagination。

不以 Infinite Scroll 作为唯一浏览方式。

URL 保存：

```text
page
page_size
sort
filters
```

---

# 747. Randomness Boundary

Browser Random 只允许：

```text
visual particles
non-authoritative decoration
idempotency IDs
```

不能用于：

```text
Dice Result
Scratch Prize
Summon Tier
Slot Stop
Blackjack Shoe
Poker Deck
```

---

# 748. Testing Pyramid

## Unit

```text
Amount parser/formatter
Route classifier
Gate resolver
Query key builders
Error mapper
Poker reducer
Event sequence
Timer projection
Token helpers
```

## Component

```text
Forms
Dialogs
Shell
Game Results
Poker Action Tray
Ops Confirmation
```

## Integration

```text
Typed HTTP mocks
Auth Bootstrap
Mutation Idempotency
Game Resume
Poker Snapshot/Event
```

## E2E

推荐 Playwright 等价方案覆盖 Browser Flow。

---

# 749. Accessibility Testing

CI：

```text
semantic lint
automated a11y checks
keyboard smoke
contrast token validation where feasible
```

推荐 axe-core 等价工具。

Automated Test 不能替代 Screen Reader Spot Check。

---

# 750. Manual Accessibility Gate

Production 前人工验证：

```text
Keyboard
Focus Order
Dialog Trap/Return
Screen Reader key flows
200% Zoom
320px Reflow
Reduced Motion
Reduced Media
Poker Action accessibility
```

---

# 751. Poker Realtime Test Harness

必须模拟：

```text
Duplicate Event
Lost Event
Out-of-order Event
Snapshot Gap
Disconnect
Reconnect
Take Over
Service Pause
Timer Race
Old control_epoch
Old hand_version
```

---

# 752. Game Presentation Test

验证：

```text
Animation Skip
Reduced Motion
Reaction asset failure
Refresh after settlement
Back/Forward
```

均不会：

```text
reroll
repay
redebit
```

---

# 753. Visual Regression / Review Privacy

使用 Fixture / Demo Data 截图：

```text
Desktop
Tablet
Mobile
420px
320px reflow
Reduced Motion
Reduced Media
Media Failure
```

Review Package 禁止真实：

```text
API Secret
OAuth / Discord Token
full real Account ID
private user profile
private Prompt / Response
unrevealed Poker Hole Cards
```

---

# 754. Asset Manifest CI

CI 检查：

```text
referenced asset exists in manifest
fallback exists
geometry metadata exists
rights status valid for environment
budget valid
production-ready status valid
```

---

# 755. Design Token CI

防止 Feature CSS 随意扩散：

```text
random hex
random breakpoint
random shadow
random radius
```

新增视觉值必须进入正式 Token Definition。

---

# 756. Production Release Gate

Frontend Release 必须承接：

```text
Visual
IA / Business Contract
Responsive
Keyboard
Screen Reader Spot Check
Contrast
Reduced Motion / Media
Performance Budget
Core Web Vitals
Asset Manifest
Rights Review
Fallback / Media Failure
```

全部 PASS 才能标记：

```text
PRODUCTION_READY
```

---

# 757. TD-11 Crash / Recovery Matrix

| Scenario | Frontend Recovery |
|---|---|
| Public Auth Bootstrap fails | Public page continues; auth enhancement unknown |
| Protected Bootstrap fails | Explicit error/retry; no fake protected state |
| Wallet response lost | Query original Transfer/Operation |
| Reward response lost | Query original Claim |
| Fast Game response lost | Query original Round / idempotency identity |
| Blackjack refresh | Resume same Round |
| Poker event gap | Stop action → Snapshot Sync |
| Poker disconnect after action | Reconnect + Snapshot; no new Action ID replay |
| Poker Take Over | Old device read-only |
| Lazy Chunk mismatch | One controlled reload |
| Persona media failure | Glyph / Geometry fallback |
| Casino background failure | CSS Surface |
| Reaction failure | Stable Result |
| Query Cache lost | Refetch Server |
| Orientation change | Layout only; Poker Session/Hand remains |

---

# 758. TD-11 Test Gate

## Route / Gate

```text
Public route works without successful Auth Bootstrap
Protected route does not flash protected data
Admin raw URL cannot bypass permission
Poker Immersive hides normal navigation
Return-to-Intent never replays mutation
```

## Amount

```text
very large atomic string remains exact
0.000002 API Credit exact
Poker whole-chip enforcement
no Number / parseFloat asset path
```

## Mutation

```text
double click → one idempotency key
timeout → original business lookup
no optimistic wallet/game balance
```

## Direct Play

```text
active Round → Resume
Skip Animation → same result
Reaction failure → same result
Blackjack stale action → Server Snapshot wins
```

## Poker

```text
duplicate event
event gap
disconnect
background tab
orientation change
Take Over
service recovery
```

均保持 Server Authority。

## Responsive

```text
>1100
<=1100
<=720
<=420
320 CSS px
200% Zoom
```

## Accessibility

```text
keyboard complete
focus trap/return
live region appropriate
Poker timer no per-second SR spam
game result text equivalent
```

## Media / Release

```text
Reduced Motion
Reduced Media
AVIF failure
WebP fallback
Character failure
Reaction failure

hashed immutable asset cache
index no-cache
chunk mismatch controlled reload
no secret in Vite env
no sensitive fixture in screenshots
```

---

# 759. TD-11 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| TD-FRZ-390 | V1 Frontend 使用单一 React + Vite Client SPA，通过 Route-level Lazy Chunk 隔离 Public/Auth/Games/Poker/Operations；不拆第二套 Operations App，也不引入 SSR。 | FROZEN |
| TD-FRZ-391 | Frontend 采用 Feature/Route/Design-System/Realtime/Generated Contract 分层，不建立单一巨型全局业务 Store。 | FROZEN |
| TD-FRZ-392 | Route Classification 为 Code-owned Public / Protected / Admin / Immersive Metadata，并严格实现 IA FINAL 已冻结路由与 Safe Parent。 | FROZEN |
| TD-FRZ-393 | Public 与 Auth 状态继续复用同一路由/页面；登录态只做能力增强，不开发重复 Public/Auth 页面树。 | FROZEN |
| TD-FRZ-394 | `/games/:game_slug` 由 Dynamic Registry + Code-owned Frontend Runtime Adapter 驱动，目录容量不写死 V1 五款游戏。 | FROZEN |
| TD-FRZ-395 | 可分享的 Tab/Filter/Sort/Pagination/Period/Model 状态进入 URL；Password、资产提交状态、Poker Action、Critical Confirmation 等敏感/副作用状态不得进入 URL。 | FROZEN |
| TD-FRZ-396 | List→Detail 使用 URL 状态 + History/Scroll Restoration 恢复筛选、分页、滚动和安全 Focus，不把旧业务数据缓存成 Authority。 | FROZEN |
| TD-FRZ-397 | Frontend 仅通过 Chaldea BFF 消费普通业务 HTTP Contract；Browser 不直连 NewAPI Internal API/DB/Redis，Poker WS 继续按 TD-08 独立直连。 | FROZEN |
| TD-FRZ-398 | TD-13 API Contract 生成 TypeScript Request/Response/Error Types；Poker WS Command/Event/Snapshot 同样使用版本化 Typed Contract，禁止 `any` 作为协议层。 | FROZEN |
| TD-FRZ-399 | HTTP Server State 使用 TanStack Query 等价 Query Cache；Query Cache 只是 Projection，V1 不将完整 Query Cache 持久化到 LocalStorage。 | FROZEN |
| TD-FRZ-400 | Read Query 允许有限瞬时 Retry；Asset/Game/Reward/Poker/Admin Mutation 禁止 Generic Automatic Retry，未知结果必须查询原业务 ID。 | FROZEN |
| TD-FRZ-401 | 每次新的用户 Mutation Intent 生成稳定随机 Idempotency Key；同一 Intent 的网络 Retry 复用原 Key，只有新的显式用户动作才生成新 Key。 | FROZEN |
| TD-FRZ-402 | Frontend Mutation 状态统一为 Idle → Submitting → Accepted/Processing → Confirmed，以及 Failed/Returned/Needs Attention/Unknown Result；Unknown Result 不能伪装成失败。 | FROZEN |
| TD-FRZ-403 | Wallet、Reward、Game Result、Poker Stack/Pot、Ranking、Permission、Maintenance 均禁止未经 Server Confirm 的 Optimistic Truth Update。 | FROZEN |
| TD-FRZ-404 | 所有资产 / Wager / Payout / Stack / Pot API 值使用字符串；Frontend 资产路径永远禁止 JS `Number` / `parseFloat`，仅在精确整数运算时受控使用 BigInt。 | FROZEN |
| TD-FRZ-405 | 建立唯一 Decimal↔Atomic Parser/Formatter；拒绝科学计数、精度溢出和非法数值，Poker 继续只接受整 Chip。 | FROZEN |
| TD-FRZ-406 | Frontend Form 使用统一 Schema + Server Validation 架构；Client Validation 只改善 UX，不替代 Balance/Permission/Eligibility/Maintenance/Version 检查。 | FROZEN |
| TD-FRZ-407 | Session Expired 可通过显式 Safe Draft Allowlist 恢复非敏感草稿；Password、Ticket、API Secret、Critical Confirmation 等永不持久化到 Browser Storage。 | FROZEN |
| TD-FRZ-408 | Public Route 不因 Session Bootstrap 故障被阻塞；Protected/Admin Route 在 Unified Gate 完成前不渲染受保护业务数据。 | FROZEN |
| TD-FRZ-409 | Frontend Global Gate Controller 严格执行 Account Status → Master Initialization → Migration Notice → Role/Scope → Resource Availability → Return-to-Intent 优先级，不在各 Feature 重写认证流程。 | FROZEN |
| TD-FRZ-410 | Return-to-Intent 使用 Server-side Safe Intent，只恢复站内 Route/安全筛选/位置，永不自动重放下注、Buy-in、Exchange、Claim、Key/Profile/Password/Admin Mutation。 | FROZEN |
| TD-FRZ-411 | HTTP Domain Error 与 React Render Error 分离；401/403/404/409/429/503 使用统一 Domain UX，部分服务故障不得关闭未受影响产品域。 | FROZEN |
| TD-FRZ-412 | Read Loading 使用 Skeleton/明确 Loading，禁止永久 Fullscreen Spinner；Empty State 与 Load Failure 必须语义分离，Toast 不能作为资产/危险操作唯一结果。 | FROZEN |
| TD-FRZ-413 | Direct Play Game Bootstrap 一次返回 Registry/Runtime/Wager/Wallet/Active Round/Fairness 等可操作状态；存在 Active Round 时 Frontend 必须 Resume-first。 | FROZEN |
| TD-FRZ-414 | Dice/Scratch/Summon/Slot 的 Business Result 与 Presentation State 严格分离；Skip/Reduced Motion/Media Failure 不得 reroll、re-settle 或改变 Wallet。 | FROZEN |
| TD-FRZ-415 | Scratch Pixel Reveal 仅为 Local Presentation；Server 九格/奖级/Payout 才是 Authority，并必须提供非 Canvas 的文本等价结果。 | FROZEN |
| TD-FRZ-416 | Blackjack Frontend 永远不预测 Card / Result；Action 使用 `action_id + expected_round_version`，Stale 时完全回到 Server Authoritative Snapshot。 | FROZEN |
| TD-FRZ-417 | Poker Realtime 不放入普通 HTTP Query Cache 作为 Authority，而使用专用 Typed External Store，并区分 Connection State 与 Hand Business State。 | FROZEN |
| TD-FRZ-418 | Poker Connect Ticket 只存在内存，使用 TD-08 First Auth Frame，不进入 URL/LocalStorage/Console；Snapshot 完成前禁止 Player Action。 | FROZEN |
| TD-FRZ-419 | Poker Event Store 使用 event_seq/table_version/hand_version；Duplicate Ignore，Gap 立即进入 SYNCING 并禁止行动，直到 Authoritative Snapshot 收敛。 | FROZEN |
| TD-FRZ-420 | Poker Client 不本地计算正式 Stack/Pot/Legal Actions；Action Delivery 不确定时先 Reconnect/Snapshot，禁止生成新 Action ID 自动重放。 | FROZEN |
| TD-FRZ-421 | Poker Timer 只投影 Server Deadline；本地倒计时归零不触发 Check/Fold，Browser Sleep/Visibility Change 后按 Server State 重新校准。 | FROZEN |
| TD-FRZ-422 | Poker Reconnect 使用有界 Backoff/Jitter；Take Over 后旧 Device 立即 Read-only；Portrait/Landscape 只改变 Layout，不重建 Socket/Session/Hand/Timer。 | FROZEN |
| TD-FRZ-423 | V1 不实现 Service Worker/PWA Offline Mutation Queue；Wager/Exchange/Claim/Poker/Admin Operation 永不因离线而排队后自动提交。 | FROZEN |
| TD-FRZ-424 | Responsive Engineering 严格采用 1100/720/420px Tokens；Tablet 不形成第三套业务 IA，PC/Mobile 共享业务 Hooks/Contracts。 | FROZEN |
| TD-FRZ-425 | Mobile 登录导航固定 Dashboard/Models/Entertainment/Wallet/My；Direct Play 默认保留普通 Shell/Bottom Nav，Poker Table 始终使用 Full Immersive Shell。 | FROZEN |
| TD-FRZ-426 | Design Tokens 建立单一版本化 Source 并生成 CSS Variables / Typed Metadata；Feature 不得散落未经 Token 化的颜色、断点、Shadow、Radius。 | FROZEN |
| TD-FRZ-427 | Button System 严格使用 Ivory/Royal Azure/Moonlit Mid 三色纪律，但该“两色+中间色”限制只作用 Button，不限制全站 Semantic/Data Colors。 | FROZEN |
| TD-FRZ-428 | CSS 采用 CSS Custom Properties + Static/Scoped CSS，避免大量 runtime CSS-in-JS；普通 Panel 继续 Solid Surface + 1px Outline，无常态 Shadow。 | FROZEN |
| TD-FRZ-429 | 字体 Self-host WOFF2 + `font-display: swap`；只 Preload 首屏必要 Functional Font，Display/Decorative 字体按需加载。 | FROZEN |
| TD-FRZ-430 | Media 与真实 DOM/CSS/SVG UI 永久分层；任何图片/视频不得承载余额、按钮、权威结果、Poker 私牌、下注、菜单、排行榜或 Login Form。 | FROZEN |
| TD-FRZ-431 | 所有媒体通过 Versioned Asset Manifest 解析，并执行 Production Status/Rights/Fallback/Focal Point/Geometry Gate；Production 不得引用未通过 Rights/Production Ready 的正式资产。 | FROZEN |
| TD-FRZ-432 | Media 加载严格遵守 v0.4 FINAL Budget、LCP Preload 与 Lazy Load；所有媒体容器预留稳定几何，禁止媒体导致核心 Form/Wager/Action/Result CLS。 | FROZEN |
| TD-FRZ-433 | `prefers-reduced-motion` 与独立 Reduced Media Mode 都是正式 Production Path；关闭 Motion/Video/Character 媒体后必须保留全部 Business Action/Result/Recovery。 | FROZEN |
| TD-FRZ-434 | Accessibility 基准为 WCAG 2.2 AA、Semantic HTML First、Keyboard Complete、Focus Trap/Return、3px Focus Ring、200% Zoom/约320px Reflow 与≥44px Touch Target。 | FROZEN |
| TD-FRZ-435 | Game/Poker 权威结果必须有完整文本等价；Async Live Region 适度播报状态，Poker Timer 禁止每秒 Screen Reader 倒计时。 | FROZEN |
| TD-FRZ-436 | Announcement Rich Text 只通过受信 Server-sanitized `TrustedRichText` Render Boundary；Poker Chat 作为纯文本渲染，禁止任意 HTML/Script。 | FROZEN |
| TD-FRZ-437 | Operations Frontend Permission 只是 UX Projection；`AUTHORIZATION_STALE` 必须刷新 Ops Bootstrap，Level 3 Form/Confirmation 不允许持久化到 Browser Storage。 | FROZEN |
| TD-FRZ-438 | Vite Deploy 使用 `index.html` no-cache/must-revalidate + hashed immutable assets；Dynamic Chunk Mismatch 最多执行一次 Controlled Reload，禁止 Reload Loop；Vite Runtime/Public Config 永不包含 Secret。 | FROZEN |
| TD-FRZ-439 | TD-11 Implementation 必须通过 Route/Gate、Exact Amount、Mutation Idempotency、Game Resume、Poker Realtime/Take Over、Responsive/A11y/Reduced Media、Asset Manifest、Performance/Cache 和 Sensitive Review Fixture Test Gate。 | FROZEN |

---

# 760. Change Log — WORKING v1.1

## Added

- 用户正式确认 TD-11；
- 冻结 `TD-FRZ-390 ～ TD-FRZ-439`；
- 冻结 Single React + Vite SPA；
- 冻结 Route-level Lazy Chunk；
- 冻结 Frontend Feature / Route / Realtime / Design System / Generated Contract 分层；
- 冻结 Route Classification / Public-Auth Route Reuse；
- 冻结 Dynamic Game Frontend Adapter；
- 冻结 URL State / Query State / Form / Presentation / Poker Realtime State 分离；
- 冻结 Typed BFF / Poker Protocol；
- 冻结 Query Cache Projection Boundary；
- 冻结 Mutation Idempotency / Unknown Result；
- 冻结 Asset String / BigInt Boundary；
- 冻结 Safe Draft / No-sensitive-persistence；
- 冻结 Unified Frontend Gate；
- 冻结 Direct Play Bootstrap / Resume-first；
- 冻结 Fast Settlement Presentation Separation；
- 冻结 Blackjack Authoritative Client；
- 冻结 Poker Snapshot / Event Sequence / Reconnect / Take Over；
- 冻结 No Service Worker / No Offline Mutation Queue；
- 冻结 1100/720/420 Responsive Tokens；
- 冻结 Design Token / CSS / Font Engineering；
- 冻结 Button 三色正确作用范围；
- 冻结 Layered Media / Asset Manifest / Rights Gate；
- 冻结 Media Budget / Lazy Load / Geometry；
- 冻结 Reduced Motion / Reduced Media；
- 冻结 WCAG 2.2 AA Frontend Gate；
- 冻结 Safe Rich Text / Telemetry Redaction；
- 冻结 Vite Cache / Chunk Mismatch Recovery；
- 冻结 Frontend Test / Production Release Gate。

## Existing Open Items Preserved

```text
TD-05 Reward OPEN fields

POKER-PROD-GAP-01
POKER-PROD-GAP-02
POKER-PROD-GAP-03
POKER-PROD-GAP-04
POKER-PROD-GAP-05

TD-09-PROD-GAP-01
```

---

# 761. 下一批 — TD-12

下一批正式进入：

> **TD-12 — Security / Observability / Deployment / Backup / DR**

计划完整冻结：

1. Threat Model；
2. Trust Boundaries；
3. TLS / Reverse Proxy；
4. Security Headers；
5. CSP；
6. CSRF / Origin；
7. Cookie Hardening；
8. Rate Limit / Abuse；
9. Secret Management；
10. Service-to-Service Authentication；
11. DB Role Hardening；
12. Redis Security；
13. NewAPI Integration Security；
14. Poker WS Security；
15. Data Classification；
16. Log Redaction；
17. Metrics / Structured Logging；
18. Trace / Request ID；
19. Alerting；
20. Health / Readiness；
21. Docker Compose Production Layout；
22. Container Isolation；
23. Resource Limits；
24. Deployment Strategy；
25. Schema Migration Gate；
26. Backup Policy；
27. NewAPI / Chaldea PostgreSQL Backup；
28. Redis Backup Boundary；
29. Media / Config Backup；
30. Restore Procedure；
31. RPO / RTO；
32. Disaster Scenarios；
33. Poker / Economy Recovery；
34. Secret Rotation；
35. Incident Response；
36. Dependency / Image Supply Chain；
37. Production Security Gate；
38. Backup / Restore Drill；
39. Deployment / DR Test Gate。

特别注意：

- TD-12 不改变业务状态机；
- PostgreSQL 仍是资产 / Poker 正式恢复来源；
- Redis 数据丢失必须可恢复，不把 Redis Backup 伪装成金融备份；
- NewAPI 相关权限与真实 Endpoint 继续以实际部署源码核验；
- V1 单 VPS + Docker Compose，不引入 Kubernetes；
- Backup 的重点是“真的能 Restore”，不是只写一个 `pg_dump` Cron；
- Observability 永远不得泄露 Password、API Key Secret、Prompt/Response、未公开 Seed/Hole Cards。



---

# 762. TD-12 — Security / Observability / Deployment / Backup / DR

> 状态：`FROZEN`  
> 用户确认：`可以，按你的来`

## 762.1 TD-12 总体结论

本批正式冻结：

- Threat Model；
- Data Classification；
- Edge-only Exposure；
- TLS / HSTS / CSP / Security Headers；
- Session Fixation / CSRF / CORS / XSS Boundary；
- Request / Parser Limits；
- SSRF Allowlist；
- Layered Rate Limit；
- Platform↔Poker Service Identity；
- NewAPI Integration Security Boundary；
- Secret File Injection；
- AES-256-GCM Fairness Seed Encryption；
- Versioned Encryption Key Ring / Rotation；
- PostgreSQL / Redis Hardening；
- Poker WS Security；
- Debug / pprof / Metrics Exposure Boundary；
- Structured Logging / Metrics / Trace Correlation；
- Out-of-band Alerting；
- 8GB / 4GB Monitoring Profile；
- NTP / Time Sync；
- `examples/deployment/external-newapi` / `examples/deployment/platform` Deployment Separation；
- Container / Docker Network Hardening；
- Resource Governance / Emergency Swap；
- Graceful Shutdown；
- Immutable Release Manifest / Image Digest；
- High-risk Deployment Gate；
- Expand / Migrate / Contract Migration；
- Application Rollback Boundary；
- Poker Deployment Recovery；
- PostgreSQL Physical Backup + Continuous WAL + PITR；
- Encrypted Off-host Backup；
- Logical Secondary Backup；
- Recovery Kit；
- Backup Retention；
- RPO / RTO；
- Restore Drill；
- `DR_RECOVERY_LOCK`；
- Full DR Sequence；
- Post-restore Invariants；
- Redis-loss / VPS-loss / Host-compromise Recovery；
- PITR vs Business Repair Boundary；
- Supply-chain Security / SBOM；
- Host Baseline；
- Deployment Preflight / Post-deploy Verification；
- Security Incident / Secret Leak Playbooks；
- TD-12 Security / DR Test Gate。

本批没有新增产品规则缺口。

保留实施核验依赖：

```text
DEPLOYMENT-VERIFY-01
Current VPS Edge / DNS / Ports / Docker Network
must be verified before implementation.

NEWAPI-SOURCE-VERIFY
NewAPI current source/deployment must be inspected for:
auth/admin routes
Redis auth
runtime secrets
persistent volume
backup-relevant non-DB data
```

---

# 763. Threat Model

V1 至少覆盖：

```text
Anonymous Internet Attacker
Authenticated Malicious User
Stolen Browser Session
Stolen API Key
XSS / CSRF / Replay
Credential Stuffing / Login Abuse
Malicious Input / Oversized Payload
Poker Protocol Abuse
Compromised Application Container
Compromised Service Credential
Malicious / Mistaken Administrator
Dependency / Container Supply-chain Issue
Backup Repository Theft / Deletion
Redis Complete Loss
PostgreSQL Crash / Disk Corruption
Single VPS Total Loss
Operator Deployment Error
```

外部依赖：

```text
Discord Unavailable
NewAPI Unavailable
Upstream Model Unavailable
DNS / Certificate Failure
```

---

# 764. Security Objective Priority

优先级：

```text
1. Financial / Poker Integrity
2. Authentication / Authorization Integrity
3. Secret Confidentiality
4. Private Game Information Confidentiality
5. Durable Recoverability
6. Product Availability
7. Operational Diagnosability
```

当 Availability 与资产/牌局正确性冲突：

```text
Prefer explicit DEGRADED / MAINTENANCE
over guessed state
```

---

# 765. V1 Security Non-goals

V1 不承诺：

```text
Multi-region HA
Kubernetes HA
Active-active DB
Volumetric DDoS absorption
Zero-trust service mesh
Hardware HSM
TOTP / Passkey / Backup Codes
Public Status Service
```

---

# 766. Data Classification

| Level | Examples |
|---|---|
| SECRET | DB Password、OAuth Secret、Service Private Key、Backup Key、Unrevealed Server Seed |
| RESTRICTED | Unrevealed Hole Cards、Future Deck、Support Evidence、Private API Usage Content |
| SENSITIVE | Wallet、Ledger、Account Binding、Game History、Full Audit、IP/Security Metadata |
| INTERNAL | Build ID、Service Name、Config Hash、Detailed Health |
| PUBLIC | Public Model Metadata、Published Games、Public Rankings、Announcements |

Logs / Metrics / Traces 默认禁止自动采集 SECRET / RESTRICTED Payload。

---

# 767. Edge-only Exposure

Production：

```text
Internet
   │
   ▼
Nginx / Caddy Edge
   ├── Chaldea Web
   ├── Chaldea BFF
   ├── Poker WS
   └── allowlisted NewAPI public API
```

应用层公网只开放：

```text
80 / 443
```

给 Edge。

不得直接公开：

```text
Platform Backend
Poker Internal HTTP
PostgreSQL
Redis
Internal NewAPI Bridge
Metrics
Debug / pprof
```

---

# 768. TLS

Production：

```text
HTTP → HTTPS permanent redirect
TLS 1.2 / 1.3 only
```

Edge 负责：

```text
Certificate Obtain / Renew
TLS Termination
Security Headers
Request Size Limits
Public Route Allowlist
```

Certificate Renewal Failure 必须告警。

---

# 769. HSTS

Production 建议：

```text
Strict-Transport-Security:
max-age=31536000
```

确认相关子域全部稳定 HTTPS 后再：

```text
includeSubDomains
```

V1 不直接加入 Browser HSTS Preload List。

---

# 770. Content Security Policy

Production Baseline：

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

关键：

```text
NO unsafe-eval
NO arbitrary third-party scripts
NO arbitrary iframe
NO arbitrary inline script
```

先 Staging：

```text
Content-Security-Policy-Report-Only
```

再 Production Enforcement。

---

# 771. Other Security Headers

至少：

```text
X-Content-Type-Options: nosniff
Referrer-Policy: strict-origin-when-cross-origin
X-Frame-Options: DENY
```

Permissions Policy 默认关闭 V1 无业务需求的：

```text
camera
microphone
geolocation
payment
usb
bluetooth
serial
accelerometer
gyroscope
magnetometer
```

---

# 772. Session Cookie Hardening

继续 TD-02：

```text
Host-only
Secure
HttpOnly
SameSite=Lax
```

建议正式 Cookie Name：

```text
__Host-chaldea_session
```

要求：

```text
Secure
Path=/
No Domain attribute
```

---

# 773. Session Fixation Defense

成功：

```text
Login
OAuth Authentication
Identity Recovery
```

后必须创建新的 Opaque Session Identity。

匿名 Session ID 不原样提升为认证 Session。

Role/Scope 变化继续使用 `authz_epoch`。

Identity Recovery/Rebind 继续使用 `security_epoch`。

---

# 774. CSRF / CORS

Cookie-auth BFF Write 继续：

```text
SameSite=Lax
+
Synchronizer CSRF Token
+
Origin Validation
+
Fetch Metadata Validation
```

BFF：

```text
same-origin
credentialed cross-origin CORS denied by default
```

NewAPI External Model API Origin 不接收 Chaldea Browser Session。

---

# 775. Browser Request Signature

V1 不引入通用 Browser Request Signature。

Browser Mutation 安全由：

```text
TLS
Session
CSRF
Origin
Permission
Idempotency Key
Business State
```

共同完成。

Service-to-Service 才使用专用 Service Assertion。

---

# 776. XSS Boundary

组合：

```text
React Output Escaping
CSP
TrustedRichText
Server Sanitization
No arbitrary HTML
No arbitrary JS
```

Announcement Rich Text 仅通过 TD-11 TrustedRichText。

Poker Chat 纯文本渲染。

---

# 777. Request / Parser Limits

Edge + Backend 对：

```text
body size
header size
JSON nesting / parse cost
```

设上限。

V1 无普通用户文件上传，大多数 BFF JSON 必须保持小体积。

Exact Byte Limits 在 Implementation Spec 基于真实 Payload 锁定。

---

# 778. SSRF Boundary

Backend Outbound Destination 只来自 Allowlist：

```text
Discord API
Verified NewAPI Internal Endpoint
Approved Backup Target
Approved Alert Sink
```

Announcement External Link 不由 Backend 自动 Fetch。

---

# 779. Rate Limit Architecture

至少：

```text
Edge Connection Limit
Edge Request Rate

Auth IP Bucket
Auth Account Bucket
Registration / OAuth Bucket
Password Login Bucket
Fresh Auth Bucket

Poker Ticket Mint Bucket
Poker WS Connect Bucket
Poker Message / Action Bucket

Reward Claim Bucket
Game Create Round Bucket

Operations Critical Action Bucket
```

阈值为隐藏 Implementation Config。

---

# 780. Rate-limit Failure Mode

Limiter 故障：

### Public Read

可以：

```text
degrade to local emergency limit
```

### Auth / Fresh Auth / Critical Security Operation

采用：

```text
fail closed
```

---

# 781. Service-to-Service Identity

Platform ↔ Poker Internal HTTP：

```text
Network Isolation
+
Ed25519-signed short-lived Service Assertion
```

Assertion 绑定：

```text
issuer
audience
issued_at
expires_at
request_id
HTTP method
path
body_sha256
```

Mutation 继续要求正式 Biz / Operation ID。

---

# 782. Service Signing Keys

Platform 和 Poker 各自拥有：

```text
private signing key
```

Peer 保存 Public Verification Key。

不共享一个长期 Static Bearer Secret。

---

# 783. Service Assertion Lifetime

Assertion 使用短 TTL。

推荐：

```text
<= 60 seconds
```

验证：

```text
audience
clock window
body hash
request identity
```

Exact Clock Skew 在 Implementation Spec 锁定。

---

# 784. NewAPI Integration Authentication

Chaldea → NewAPI 不能假定支持同一 Service Assertion。

必须按用户当前部署：

```text
Verified API Credential
or
Narrow Bridge Authentication
```

设计。

继续：

```text
SOURCE VERIFICATION REQUIRED
```

---

# 785. Secret Storage

Production Secret 禁止进入：

```text
Git
Dockerfile
Container Image
Frontend Bundle
Compose YAML Plaintext
Asset Manifest
Audit
Ordinary Logs
```

推荐：

```text
examples/deployment/platform/secrets/
```

Host Directory：

```text
0700 root-owned
```

Secret File：

```text
0600
```

通过 Compose Secret / Read-only File Mount 注入。

---

# 786. Environment Variables

普通非 Secret Config 可使用 Environment Variable。

高价值长期 Secret 优先 File-mounted，降低 Process/Debug/Compose 扩散。

---

# 787. Fairness Seed Encryption

未 Reveal Game / Poker Server Seed：

```text
AES-256-GCM
```

每对象保存：

```text
key_version
nonce
ciphertext
```

AAD 至少：

```text
domain
round_id / hand_id
algorithm_version
```

---

# 788. Encryption Nonce

每次 AES-GCM 加密：

```text
unique random 96-bit nonce
```

禁止固定 Nonce。

Seed Encryption Key 不存数据库。

---

# 789. Encryption Key Ring

状态：

```text
ACTIVE
DECRYPT_ONLY
RETIRED
```

新写入只用 ACTIVE。

旧记录通过 `key_version` 找对应 Key。

---

# 790. Key Rotation

无停机：

```text
add new key → ACTIVE
old ACTIVE → DECRYPT_ONLY
new writes → new key
```

删除旧 Key 前必须证明：

```text
no retained DB object
no retained backup
```

仍依赖它。

---

# 791. Recovery Key Requirement

DR Recovery Kit 必须包含仍受保留期影响的历史 Fairness Encryption Key Version。

否则数据库 Restore 后可能无法恢复合法 Round / Poker Hand。

---

# 792. PostgreSQL Hardening

继续 TD-03：

```text
owner
migrator
app
poker
newapi_ro
cutover
```

Runtime Role：

```text
NO SUPERUSER
NO CREATEROLE
NO CREATEDB
NO RUNTIME DDL
```

并撤销不需要的 PUBLIC Privilege。

---

# 793. PostgreSQL Authentication / Network

Production：

```text
SCRAM-SHA-256
```

禁止 Runtime `trust`。

`pg_hba.conf` 只允许必要：

```text
network
role
database
```

PostgreSQL Port 不公网映射。

---

# 794. Backup Role

Backup Executor 使用独立 Backup Credential。

Runtime App / Poker Role 不因备份需要获得 Cluster-level Backup Privilege。

---

# 795. Redis Hardening

同 Redis Instance 使用：

```text
Namespace
+
ACL
```

逻辑：

```text
NewAPI → newapi:*
Platform → chaldea:session/auth-flow/return-intent/cache/lock/*
Poker → chaldea:poker/*
```

NewAPI ACL 兼容性仍需 SOURCE VERIFICATION。

---

# 796. Redis Public Boundary

Redis：

```text
No public host port
```

Application ACL User 禁止：

```text
CONFIG
FLUSHALL
FLUSHDB
DEBUG
MODULE
```

等高危管理命令。

---

# 797. Redis Persistence Boundary

Redis Persistence 仅是 Operational Convenience。

不得计入：

```text
Financial RPO
Poker RPO
```

DR 必须支持：

```text
start Redis empty
```

然后重新登录 / 重建 Cache / PG Recovery。

---

# 798. Poker WS Security

继续 TD-08，并增加：

```text
max frame size
schema validation
message rate limit
bounded send queue
ping/pong idle detection
```

Ticket / Origin / Version / Control Epoch 继续保持。

---

# 799. WS Compression

V1 默认：

```text
permessage-deflate disabled
```

以后根据真实带宽/CPU数据再评估。

---

# 800. Debug Endpoint Boundary

Production：

```text
/debug/*
pprof
internal metrics
internal diagnostics
```

不通过 Public Edge 发布。

pprof 默认 Disabled 或 Internal-only Temporary Enable。

---

# 801. Observability Contract

每个 Chaldea Service 至少：

```text
Structured Logs
Prometheus-compatible Metrics
Liveness
Readiness
Request / Trace Correlation
```

---

# 802. Structured Log Schema

统一 JSON：

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
biz_id nullable
operation_id nullable
round_id nullable
hand_id nullable
job_id nullable
```

---

# 803. Log Content Rules

默认永远不写：

```text
raw HTTP body
raw response body
Authorization
Cookie
CSRF token
OAuth token
API Key Secret
Password
Prompt / Response
Server Seed
Full Deck
Private Hole Cards
```

Debug Level 也不自动解禁。

---

# 804. Log Identity

需要定位用户时优先：

```text
newapi_user_id
or
pseudonymous stable internal ID
```

不默认记录 Master Nickname / Discord Display Name。

---

# 805. Edge Access Logs

默认不记录完整 Query String。

记录：

```text
method
host
safe path/route
status
duration
bytes
safe client network metadata
request_id
```

不记录 Authorization / Cookie。

---

# 806. Metrics Cardinality

允许低基数 Label：

```text
service
route_template
method
status_class
game_slug
state
result_class
```

禁止：

```text
user_id
round_id
hand_id
request_id
operation_id
IP
```

作为 Metric Label。

---

# 807. Trace / Request Correlation

Edge 接受安全有效 Request ID 或生成新值。

传播：

```text
request_id
trace_id
```

用户错误页可显示 Reference ID，但不显示 Stack Trace。

---

# 808. Distributed Tracing

V1 不强制部署 Jaeger / Tempo。

代码保持：

```text
W3C Trace Context
+
OpenTelemetry-compatible
```

未来可接 Trace Backend。

---

# 809. Health Endpoints

每服务：

```text
/health/live
/health/ready
/metrics
```

`live` 只判断进程是否健康，不把外部依赖失败当成必须自杀重启。

---

# 810. Readiness

Readiness 表示 Service 是否能安全接受核心职责。

例如 Platform Own DB 不可用：

```text
NOT READY
```

NewAPI 单独不可用：

```text
Platform may stay alive
unaffected domains remain available
```

避免 Restart Loop。

---

# 811. Dependency Health

Dependency Degradation 通过 TD-10 Service Health Projection 表达。

不把所有 Dependency 强绑 Container Liveness。

---

# 812. Out-of-band Alerting

Production 至少配置一个 Chaldea 之外的 Alert Sink。

例如：

```text
Email
Discord Webhook
Generic Webhook
Other approved channel
```

Exact Sink 进入 Implementation Config。

---

# 813. Alert Categories

至少：

```text
SERVICE_DOWN
HIGH_5XX
DB_UNAVAILABLE

BACKUP_STALE
WAL_ARCHIVE_FAILED
RESTORE_CHECK_FAILED

DISK_LOW
MEMORY_PRESSURE

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

---

# 814. Alert Deduplication

Alert 保存/计算：

```text
fingerprint
first_seen
last_seen
state
```

支持：

```text
FIRING
RESOLVED
```

不能每秒重复刷同一 Incident。

---

# 815. Monitoring Profiles

## 8GB Recommended

```text
Prometheus
Alertmanager
Grafana optional
```

## 4GB Compatibility

允许：

```text
shorter metrics retention
omit local Grafana
reduced collector resource use
```

不降低核心业务正确性。

---

# 816. Logs Stack

V1 不强制 Loki / Elasticsearch。

基线：

```text
Structured Container Logs
+
Host Log Rotation
```

以后可用 Vector / Fluent Bit 等 Ship 到 Remote Sink。

---

# 817. Observability Retention

V1 运维默认：

```text
Application / Edge Runtime Logs
→ 14-day target
→ hard size cap

Prometheus Metrics
8GB → 30d
4GB → 7d

Audit / Ledger / Migration Evidence
→ no V1 business TTL
```

硬容量上限优先，避免日志写满磁盘。

---

# 818. Time Synchronization

VPS 必须运行可靠 NTP：

```text
chrony
or
systemd-timesyncd
```

监控：

```text
sync state
clock drift
```

OAuth / Fairness / Job / Session / DB 时间依赖正确 Host Time。

---

# 819. Deployment Directory Boundary

保持：

```text
examples/deployment/external-newapi
examples/deployment/platform
```

两套 Compose Project 独立。

Chaldea 不自动接管 NewAPI Deployment。

---

# 820. Edge Proxy Verification

实际采用 Nginx 或 Caddy 由 Implementation 前核验当前 VPS：

```text
DEPLOYMENT ENVIRONMENT VERIFICATION REQUIRED
```

行为 Contract 冻结，但避免两个 Edge 同时争抢 80/443。

---

# 821. Docker Network Design

建议：

```text
edge network
application/internal network
data network
```

每个 Container 只加入真正需要的 Network。

Frontend Static Server 无需 DB Network。

---

# 822. Container Hardening

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

---

# 823. Docker Socket

任何 Chaldea Service 均不得挂载：

```text
examples/runtime/docker.sock
```

---

# 824. Restart Policy

生产服务使用明确：

```text
restart unless-stopped
```

或等价策略。

Restart 不代替 Durable Recovery。

---

# 825. Resource Governance

每 Container 需要：

```text
memory observation
CPU observation
connection limits
worker concurrency config
```

关键非 DB Service 建议设置合理 Memory Limit。

---

# 826. 8GB Profile

目标：

```text
leave ~1–1.5GB host / burst headroom
```

PostgreSQL / NewAPI 的精确内存按实际 Load Test 调整。

---

# 827. 4GB Profile

只作为兼容目标：

```text
lower worker concurrency
shorter local metrics retention
optional no local Grafana
conservative DB memory
smaller cache
```

不改变业务语义。

---

# 828. Emergency Swap

建议：

```text
1–2GB
```

Emergency Swap。

持续 Swap 视为 Memory Pressure，而不是正常容量。

---

# 829. Graceful Shutdown — Platform

SIGTERM：

```text
stop accepting new requests
allow bounded in-flight completion
release leases cleanly
flush logs/metrics
exit
```

未完成 Durable Job 由 PG Lease Recovery。

---

# 830. Graceful Shutdown — Poker

SIGTERM：

```text
stop new WS connections / commands
finish currently committing DB transaction
close sockets with reconnectable reason
exit
```

不要求 Deploy 前完成所有 Poker Hand。

重启从 PostgreSQL 恢复同一 Hand。

---

# 831. Release Manifest

每次 Production Release 保存：

```text
release_id
git_commit
build_id
frontend artifact hash
platform image digest
poker image digest
schema migration version/checksum
asset manifest hash
config version
deployed_at
deployed_by
environment
```

---

# 832. Immutable Images

Production 禁止：

```text
image: latest
```

使用不可变 Image Digest。

Rollback 指定明确 Previous Digest。

---

# 833. Deployment Categories

区分：

```text
Frontend-only
Compatible Backend
Schema Migration
Poker Runtime
Security/Auth
Economy/Migration High-risk
```

风险不同，Deployment Gate 不同。

---

# 834. Frontend-only Deploy

允许：

```text
build hashed assets
upload release
atomic index switch
```

通常无需业务 Maintenance。

保留上一版本静态 Asset 一段窗口，降低旧 Tab Chunk Failure。

---

# 835. Compatible Backend Deploy

无不兼容 Schema/Data Migration 时：

```text
pull immutable image
health check
restart backend
verify
```

单 VPS 不强制 Blue/Green。

---

# 836. High-risk Deploy

涉及：

```text
Auth
Economy
Database Migration
Migration/Cutover
Poker State Logic
Critical Security Boundary
```

必须：

```text
Fresh Backup/PITR Check
→ Maintenance
→ Migration
→ Deploy
→ Verification
→ Open Gate
```

---

# 837. Schema Migration

继续 TD-03 Immutable / Checksum / Migrator / Forward-fix。

增加：

```text
Expand
→ Migrate
→ Contract
```

策略。

---

# 838. Destructive Schema Change

禁止同一 Release：

```text
drop old structure
+
deploy first code requiring new structure
```

Contract 必须等：

```text
old code no longer used
backup verified
migration verified
```

再执行。

---

# 839. Application Rollback

只在当前 Schema Backward-compatible 时直接回滚 Application Image。

如果 Data/Schema 已不可逆变化：

```text
Forward Fix
```

不是伪造 Down Migration。

真正灾难才做 Restore。

---

# 840. Poker Deployment

升级：

```text
Stop New Hands
→ active Hand finish or safe Pause
→ restart Poker
→ PG Recovery
→ 30s Reconnect Grace
→ Resume
```

不改 Pot / Stack / Hand。

---

# 841. NewAPI Deployment Boundary

Chaldea Release 不自动 Upgrade / Restart NewAPI。

NewAPI 版本变化后需重新执行 Source Compatibility Verification。

---

# 842. Primary PostgreSQL Backup

主策略：

> **Cluster-level Physical Backup + Continuous WAL Archive + Point-in-Time Recovery**

原因：

```text
newapi
+
chaldea_platform
```

同一 PostgreSQL Cluster，需要恢复至同一 Timeline / Restore Point。

每日独立 `pg_dump` 不作为唯一金融恢复方案。

---

# 843. Backup Engine Contract

要求：

```text
PITR-capable
WAL-aware
checksum verification
encrypted off-host repository
retention management
restore validation
```

推荐实现：

```text
pgBackRest
```

或满足同等 Contract 的工具。

---

# 844. WAL Archive

连续归档 WAL 至 Off-host Repository。

低流量环境需配置合理 WAL Switch / Archive Timeout，避免 Segment 长期未归档。

工程目标：

```text
maximum archive gap ≈ 5 minutes
```

Exact PG 参数按部署版本锁定。

---

# 845. Off-host Mandatory

以下均不算正式 DR Backup：

```text
same VPS examples/deployment/backups
same disk volume copy
same filesystem second directory
```

Production 必须至少：

```text
one encrypted off-host backup repository
```

---

# 846. Backup Transport

使用：

```text
TLS
+
encrypted repository
```

Backup Repository 不公开。

Runtime Application 不持有 Restore Admin Credential。

---

# 847. Secondary Logical Backup

除 PITR 外每天生成：

```text
pg_dump newapi
pg_dump chaldea_platform
roles / grants manifest
```

作为 Portable Secondary Recovery / Forensics。

不是首选 Financial RPO Source。

---

# 848. Backup Scope

至少：

```text
PostgreSQL Cluster
newapi
chaldea_platform
roles / grants

Schema Migration Manifest
Release Manifest
Migration / Cutover Manifests

Chaldea Runtime Config
NewAPI Runtime Config needed for recovery

Asset Manifest
Rights Manifest

Critical non-reconstructible persistent volume data
```

NewAPI Persistent Volume 是否含不可重建数据：

```text
SOURCE VERIFICATION REQUIRED
```

---

# 849. Secret Recovery Kit

普通 DB Backup Repository 不直接塞明文 Secret。

建立独立加密 Recovery Kit：

```text
Fairness Encryption Key Ring
Backup Repository Recovery Key
required OAuth/NewAPI recovery secrets
service identity recovery material where needed
```

---

# 850. Session Secrets after DR

无需恢复：

```text
old Chaldea Session
old CSRF state
old Poker Connect Ticket
Redis session state
```

DR 后可以全部失效并要求重新登录。

---

# 851. Backup Retention

V1 目标：

```text
PITR Window → 30 days
Weekly Restore Points → 8 weeks
Monthly Archive Restore Points → 6 months

Logical Dumps:
7 daily
4 weekly
```

---

# 852. RPO / RTO

PostgreSQL Durable Facts：

```text
RPO <= 5 minutes
RTO <= 2 hours
```

RTO 指 DR 宣告并开始恢复后，到核心服务可以安全重新开放的技术目标。

---

# 853. Redis Recovery Objective

Redis 不承担 Durable RPO。

允许完全丢失。

基础设施就绪后：

```text
new empty Redis <=15 minutes
```

业务随后从 PG/重新登录恢复。

---

# 854. Static Artifact Recovery

Immutable Release 通过 Git / Registry / Release Manifest 恢复。

RPO = Last Deployed Release Manifest。

---

# 855. Backup Freshness Alerts

至少：

```text
WAL archive age > RPO
→ CRITICAL

physical backup overdue
→ CRITICAL

logical backup overdue
→ WARNING

repository check fail
→ CRITICAL
```

High-risk Deploy / Cutover 在 Backup Freshness 不合格时阻断。

---

# 856. Backup Verification

Backup Success 不只看 Exit Code。

必须定期：

```text
repository check
backup manifest validation
checksum validation
WAL continuity validation
```

---

# 857. Restore Drill

冻结：

```text
Before Initial Production Launch
→ mandatory full restore drill

Monthly
→ isolated PostgreSQL restore drill

Quarterly
→ full application DR drill

After Backup/Encryption Mechanism Change
→ immediate restore test
```

---

# 858. Restore Drill Isolation

必须：

```text
isolated network
no public routing
no real model-provider call
no Discord mutation
no scheduled business jobs
```

不能恢复生产副本后自动执行 Reward / Announcement / API / Poker Side Effects。

---

# 859. DR Recovery Lock

正式新增：

```text
DR_RECOVERY_LOCK
```

它属于 Deployment Safety，不是普通 Product Maintenance。

Full Restore 启动后：

```text
DR_RECOVERY_LOCK = ON
```

阻断：

```text
new Chaldea user writes
new game round
new Poker table / hand
ranking publish
scheduled announcement publish
```

直到验证完成。

---

# 860. DR Lock Independence

不能只依赖恢复出来的 Maintenance State。

Backup Point 可能早于 Maintenance Activation。

因此 DR Lock 独立于业务 DB 中的 Maintenance。

---

# 861. Full DR Sequence

```text
1. Provision clean host
2. Install pinned Docker/Edge baseline
3. Restore Recovery Kit
4. Restore PostgreSQL cluster to selected PITR
5. Verify roles/grants/migrations
6. Start Redis empty
7. Start services with DR_RECOVERY_LOCK
8. Keep user writes closed
9. Inspect/reconstruct incomplete durable states
10. Economy reconciliation
11. Reward recovery
12. Direct Play recovery
13. Poker recovery
14. Verify Rankings/Jobs/Announcements
15. Verify NewAPI integration
16. Run global invariant report
17. Operator approval
18. Remove DR_RECOVERY_LOCK
19. Reopen traffic
20. Elevated monitoring
```

---

# 862. Post-restore Invariant Report

至少：

```text
No negative wallet balance
Ledger/materialized balance consistency
No duplicate transfer terminal effect
No Settlement + Refund same Round
No duplicate Reward issuance

Poker asset conservation
No duplicate Poker Settlement
Active Poker Session uniqueness

Fairness encrypted seed decryptability

Migration checksum consistency
Schema migration checksum consistency

Ranking published pointer integrity
Admin/Audit readable
NewAPI User/Key/Quota integration readable
```

---

# 863. Redis Total Loss

丢失：

```text
Sessions
OAuth/Auth Flow temporary state
Return Intent temporary state
Locks
Poker event buffer / presence
Cache
```

恢复：

```text
users reauthenticate
cache rebuild
workers use PG leases
Poker actors reconstruct from PG
```

Wallet / Stack / Round Result 不从 Redis 恢复。

---

# 864. Platform Service Loss

Binary/Container Loss、PG 正常：

```text
pull same immutable image
restart
```

Accepted Transfer / Reward / Round / Job 从 Durable State 继续。

---

# 865. Poker Service Loss

继续 TD-08：

```text
PG Hand durable
actor recreated
runtime_epoch++
30s grace
same Hand resumes
```

无需 DB Restore。

---

# 866. PostgreSQL Process Crash

Disk/WAL 正常：

```text
restart PostgreSQL
→ native crash recovery
→ service readiness
→ domain recovery workers
```

---

# 867. VPS / Disk Total Loss

正式路径：

```text
Clean Host
→ Immutable Release
→ Recovery Kit
→ Off-host PITR
→ DR_RECOVERY_LOCK
→ Verification
→ Reopen
```

---

# 868. Host Compromise

疑似 Host compromise：

```text
isolate old host
preserve forensic evidence
provision clean host
restore known-good backup
rotate credentials
rotate service signing keys
rotate DB/Redis credentials
revoke sessions
verify fairness key integrity
redeploy pinned artifacts
```

不原地“清理后继续生产”。

---

# 869. Backup Repository Compromise

必须：

```text
revoke credential
rotate auth
verify object integrity/version history
verify backup checksums
create new trusted baseline
```

如果 Provider 支持：

```text
Versioning
Object Lock / Immutable Retention
```

建议启用。

---

# 870. PITR vs Business Repair

PITR 不是单条业务错误撤销按钮。

平台已经产生后续经济事实后，单记录错误使用：

```text
Incident
→ Reconciliation
→ Compensation
→ Repair
```

不能恢复整个 Cluster 到过去时间覆盖后续真实业务。

---

# 871. PITR Decision Boundary

Full PITR 仅用于：

```text
catastrophic cluster loss
broad corruption
failed deployment before safe reopening
security incident requiring clean restore point
```

必须记录：

```text
restore point
expected lost window
RPO impact
external effects requiring reconciliation
```

---

# 872. Backup Manifest

至少：

```text
backup_id
cluster timeline
start/end time
WAL range
backup type
tool version
repository
encryption key id
checksum result
release/schema manifest reference
```

不能使用无法追溯 Release / Schema 的随意压缩包作为正式 Backup。

---

# 873. Supply-chain Security

Production：

```text
dependency lockfile
go.sum
pinned base image
immutable image digest
```

禁止：

```text
latest
runtime curl | sh
floating production npm install
```

---

# 874. CI Security Gate

Production Artifact 至少：

```text
unit/integration tests
dependency vulnerability scan
container image scan
secret scan
SBOM generation
migration checksum verification
frontend asset/rights gate
```

Critical Vulnerability 必须 Block 或显式 Review。

---

# 875. SBOM

每 Release 生成：

```text
Frontend dependency SBOM
Platform Go module SBOM
Poker Go module SBOM
Container base image SBOM
```

并绑定 `release_id`。

---

# 876. Host Baseline

Infrastructure Runbook 至少：

```text
dedicated admin account
SSH key authentication
direct root SSH disabled after emergency path verification
host firewall default deny
only intended ports exposed
regular security updates
time synchronization
disk monitoring
```

不进入 Chaldea Operations Shell。

---

# 877. Infra/Application Firewall Boundary

“Only Edge Public”指应用业务服务端口。

SSH/Cockpit 等 Infra 管理入口独立控制，不由 Chaldea Compose 自动暴露。

---

# 878. Disk Monitoring

必须监控：

```text
PostgreSQL volume
Docker images/logs
Backup staging
Redis volume if persistence enabled
Root filesystem
```

Disk Low 是 Critical Operational Risk。

---

# 879. Backup Staging

如需要 Local Staging：

```text
bounded temporary staging
```

成功上传/验证后清理。

Local Staging 不计正式 Backup。

---

# 880. Deployment Preflight

High-risk Release 前：

```text
Correct Environment
Disk free
DB health
Redis health
Latest backup freshness
WAL archive healthy
Migration checksums
Current Release Manifest
Target Release Manifest
Maintenance impact
No unresolved critical invariant
```

关键 Gate Fail：

```text
do not deploy
```

---

# 881. Post-deploy Verification

至少：

```text
Frontend build ID
Platform build ID
Poker build ID
DB migration version
Health
Session bootstrap
Public routes
Wallet read
Reward status read
Game registry read
No duplicate job
Poker recovery
Audit write test
Metrics/alerts connected
```

---

# 882. Security Incident Workflow

```text
Detect
→ Alert
→ Create Incident
→ Contain
→ Preserve Evidence
→ Revoke / Rotate
→ Recover
→ Verify
→ Reopen
→ Post-incident Review
```

Incident Timeline 继续 TD-10 Append-only。

---

# 883. Secret Leak Playbook

## Browser Session

```text
revoke session / security_epoch
```

## Service Signing Key

```text
remove verification key
rotate signer
redeploy
```

## DB / Redis Password

```text
rotate
restart dependents
```

## Backup Credential

```text
revoke/rotate
verify repository
```

## Fairness Encryption Key

```text
contain
rotate ACTIVE key
preserve required historical decrypt key securely
investigate unrevealed seed exposure
```

---

# 884. Observability as Data Egress

Log Aggregator / Metrics / Error Reporter / Alert Webhook 均视为独立 Data Egress。

只发送 Minimum Safe Metadata。

不得把完整 Request/Response/Poker Snapshot 发到第三方观测系统。

---

# 885. Restore Privacy

Production Backup Restore Drill 使用 Private Isolated Environment。

如果之后用于开发 / UI QA：

```text
anonymize
or
fixture conversion
```

Production DB 不长期作为 Staging 数据源。

---

# 886. DR Public Traffic Gate

DR 验证期间 Public Read 是否开放由 Incident 判断。

但：

```text
user writes
model API charging path
Poker new work
```

在对应 Authority 未验证前必须关闭。

---

# 887. RPO Breach

如果：

```text
last recoverable WAL gap > 5m
```

触发：

```text
CRITICAL Alert
Needs Attention
Block high-risk deploy/cutover
```

不要求自动关闭整个网站。

---

# 888. Backup Failure Boundary

Backup Failure 不改 Wallet / Poker / Business State。

它只是 Critical Operational Condition，并阻止 High-risk Deployment。

---

# 889. TD-12 Crash Matrix

| Failure | Formal Outcome |
|---|---|
| Redis Lost | Re-login / Cache Rebuild；PG business facts survive |
| Platform Crash | Restart immutable build；Jobs/business recover from PG |
| Poker Crash | Same Hand recovered from PG |
| Edge Crash | Restart Edge；DB unaffected |
| DB Process Crash | PostgreSQL native recovery |
| VPS Loss | Off-host PITR restore |
| Backup Job Crash | Retry backup operation / repository check |
| WAL Archive Failure | Alert；RPO degraded |
| Deploy Response Lost | Inspect Release Manifest / actual build |
| Migration App Crash | Migration record/checksum decides |
| Audit/Log Collector Fails | Audit DB still authoritative |
| Metrics Collector Fails | Business continues；monitoring degraded |
| Secret Rotation Interrupted | Versioned dual-key state resumes |

---

# 890. TD-12 Test Gate

## Edge

```text
PostgreSQL not Internet reachable
Redis not Internet reachable
internal metrics not public
pprof not public
```

## Web Security

```text
CSRF blocked
wrong Origin blocked
CSP blocks inline script
frame embedding blocked
session cookie host-only/HttpOnly
```

## Service Identity

```text
wrong issuer
wrong audience
expired assertion
modified body/path
→ rejected
```

## Secret

```text
secret scan
log scan
audit scan
frontend bundle scan
```

无真实 Secret。

## Fairness Encryption

```text
wrong AAD → fail
wrong key → fail
old/new key rotation works
seed never logged
```

## Redis Loss

```text
empty Redis test
→ assets intact
→ Poker rebuild
→ sessions safely lost
```

## Backup / DR

```text
PITR restore
logical restore
roles/grants
key recovery
WAL continuity
clean host full DR
DR_RECOVERY_LOCK
invariant report
```

## Deployment

```text
old image rollback
forward-only migration
frontend chunk mismatch
Poker restart
```

## Observability

```text
request ID propagation
metric cardinality
alert fire/resolve
backup stale alert
no sensitive payload in logs
```

---

# 891. TD-12 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| TD-FRZ-440 | TD-12 采用 Edge-only Exposure + Defense-in-depth Security + Privacy-safe Observability + Immutable Compose Release + PostgreSQL PITR + Off-host DR；资产/身份/恢复完整性优先于瞬时可用性。 | FROZEN |
| TD-FRZ-441 | 数据分为 SECRET / RESTRICTED / SENSITIVE / INTERNAL / PUBLIC；Logs/Metrics/Trace 默认不得采集 Secret 或 Restricted Payload。 | FROZEN |
| TD-FRZ-442 | Production 应用层只有 Edge Proxy 对公网提供入口；Platform/Poker Internal HTTP/PostgreSQL/Redis/Metrics/Debug 均不得直接公网发布，TLS 只允许 1.2/1.3。 | FROZEN |
| TD-FRZ-443 | Production 启用强 CSP、Frame Deny、nosniff、Referrer/Permissions Policy；Script 禁止 `unsafe-eval` 与任意第三方执行，CSP 先在 Staging Report-only 验证后 Enforcement。 | FROZEN |
| TD-FRZ-444 | TD-02 Host-only/Secure/HttpOnly/SameSite=Lax Session Cookie 进一步使用 `__Host-` Cookie Contract；Login/Recovery 创建新 Session，防止 Session Fixation。 | FROZEN |
| TD-FRZ-445 | Cookie-auth BFF 继续使用 Synchronizer CSRF + Origin/Fetch Metadata，Credentialed CORS 默认拒绝；V1 不增加通用 Browser Request Signature。 | FROZEN |
| TD-FRZ-446 | Edge/Backend 强制 Request Size/Parser Limits，Backend Outbound Fetch 使用目的地 Allowlist，普通 Announcement External Link 不触发 Server-side arbitrary fetch。 | FROZEN |
| TD-FRZ-447 | Rate Limit 扩展至 Edge/Auth/Fresh Auth/Poker Ticket/WS/Game/Reward/Critical Ops；阈值为隐藏 Implementation Config，关键 Auth Security Limiter 故障采用 Fail-closed。 | FROZEN |
| TD-FRZ-448 | Platform↔Poker Internal Service 使用 Network Isolation + Ed25519 Short-lived Service Assertion，绑定 Issuer/Audience/Time/Request/Method/Path/Body Hash；Mutation 仍要求业务 Idempotency。 | FROZEN |
| TD-FRZ-449 | Chaldea→NewAPI Authentication 继续按实际部署能力 SOURCE VERIFICATION REQUIRED，不假定 NewAPI 支持 Chaldea Service Assertion。 | FROZEN |
| TD-FRZ-450 | Production Secret 不进入 Git/Image/Frontend/Compose Plaintext/Logs/Audit；高价值 Secret 使用 root-owned Secret File / Compose Secret Read-only Mount。 | FROZEN |
| TD-FRZ-451 | 未 Reveal Game/Poker Server Seed 使用 AES-256-GCM 加密，保存 key_version/nonce/ciphertext，并使用 Round/Hand/Algorithm Identity 作为 AAD。 | FROZEN |
| TD-FRZ-452 | Fairness Encryption 使用版本化 Key Ring，支持 ACTIVE→DECRYPT_ONLY Rotation；旧 Key 在所有依赖数据与 Backup 退出 Retention 前不得删除。 | FROZEN |
| TD-FRZ-453 | PostgreSQL Runtime Roles 保持最小权限、无 Superuser/Createdb/Createrole/Runtime DDL，使用 SCRAM-SHA-256 和限制性 pg_hba；DB Port 不公网发布。 | FROZEN |
| TD-FRZ-454 | Redis 使用 Namespace + ACL，禁用应用用户高危管理命令且不公网发布；Redis Persistence 永远不计入 Financial/Poker RPO，DR 必须支持 Empty Redis Start。 | FROZEN |
| TD-FRZ-455 | Poker WS 增加 Frame Size/Schema/Message Rate/Backpressure/Ping-Pong Security，V1 默认不启用 permessage-deflate；Ticket/Origin/Version/Control Epoch 继续继承 TD-08。 | FROZEN |
| TD-FRZ-456 | Production Debug/pprof/Metrics Endpoint 仅 Internal，可临时受控启用但不得通过 Chaldea Operations 或 Public Edge 暴露。 | FROZEN |
| TD-FRZ-457 | 每个 Chaldea Service 必须提供 Structured JSON Logs、Prometheus-compatible Metrics、Liveness/Readiness 与 Request/Trace Correlation。 | FROZEN |
| TD-FRZ-458 | Structured Logs 使用 Safe Schema；Request/Response Body、Authorization/Cookie/CSRF/OAuth/API Secret/Password/Prompt/Seed/Deck/Hole Cards 永不进入普通日志。 | FROZEN |
| TD-FRZ-459 | Metric Label 严格低 Cardinality，禁止 user_id/round_id/hand_id/request_id/IP 等高 Cardinality Identity；业务 ID 只进入安全 Logs。 | FROZEN |
| TD-FRZ-460 | V1 传播 W3C-compatible Trace/Request Context，但不强制部署 Jaeger/Tempo；Code 保持 OpenTelemetry-compatible 以便未来无协议重构接入。 | FROZEN |
| TD-FRZ-461 | `/health/live` 与 `/health/ready` 分离；外部依赖单点故障不得触发无意义 Restart Loop，Dependency Degradation 通过 Service Health 单独表达。 | FROZEN |
| TD-FRZ-462 | Production 必须配置至少一个 Out-of-band Alert Sink；告警覆盖 Service/DB/Backup/WAL/Disk/Audit/Economy/Poker/Jobs/Cert 等 Critical Condition，并具有 Dedup/Resolved 语义。 | FROZEN |
| TD-FRZ-463 | 8GB 推荐使用 Prometheus-compatible Collector + Alertmanager、Grafana 可选；4GB Profile 可缩短 Metrics Retention/省略本地 Grafana，但不降低核心业务正确性。 | FROZEN |
| TD-FRZ-464 | V1 不强制 Loki/Elasticsearch；Structured Container Logs + Rotation 为基线，可后续增加 Log Shipper，不改变 Log Contract。 | FROZEN |
| TD-FRZ-465 | V1 默认 Retention：Runtime Logs 14-day Target + Hard Size Cap；Metrics 8GB=30d、4GB=7d；Audit/Ledger/Migration Evidence 不配置 V1 Business TTL。 | FROZEN |
| TD-FRZ-466 | VPS 必须保持可靠 NTP Time Sync 并监控 Clock Drift；业务 Server Time/DB Time/OAuth/Fairness/Jobs 不依赖失准主机时钟。 | FROZEN |
| TD-FRZ-467 | `examples/deployment/external-newapi` 与 `examples/deployment/platform` 继续独立 Compose Project；Edge 行为 Contract 冻结，但实际 Nginx/Caddy 选择在部署前核验现有 Host，避免双 Edge 抢占 80/443。 | FROZEN |
| TD-FRZ-468 | Chaldea 自有容器默认 Non-root、Read-only Root FS where possible、Cap Drop、No-new-privileges、No Privileged/Host Network/Docker Socket，Secret 只读挂载。 | FROZEN |
| TD-FRZ-469 | Production 使用 Resource Governance 与 Host Headroom；8GB 保留约 1–1.5GB Burst/OS Headroom，4GB 降低 Worker/Monitoring/Cache 配置，精确 Container Cap 经 Load Test 在 Implementation Spec 锁定。 | FROZEN |
| TD-FRZ-470 | 建议 1–2GB Emergency Swap 仅防瞬时 OOM，不作为常规 RAM；持续 Swap 进入 Memory Pressure Alert。 | FROZEN |
| TD-FRZ-471 | Platform/Poker 都必须支持 Graceful SIGTERM；未完成业务靠 Durable PG State/Lease/Recovery 收敛，Container Restart 不得成为业务恢复算法。 | FROZEN |
| TD-FRZ-472 | 每个 Production Release 保存不可变 Release Manifest：Git/Build/Image Digest/Schema Checksum/Asset Manifest/Config/Actor/Environment，并禁止 `latest` 镜像。 | FROZEN |
| TD-FRZ-473 | Deployment 分 Frontend-only / Compatible Backend / Schema / Poker / Security-Economy High-risk；High-risk Release 强制 Backup Freshness + Maintenance + Verification。 | FROZEN |
| TD-FRZ-474 | Database Change 采用 Immutable Migration + Expand/Migrate/Contract；破坏性 Contract 不与依赖新结构的首个 Release 同时执行，保持合理 Application Rollback Window。 | FROZEN |
| TD-FRZ-475 | Application Rollback 只在当前 Schema Backward-compatible 时切回 Previous Digest；不可逆 Data/Schema Change 使用 Forward Fix，不伪造 Down Migration。 | FROZEN |
| TD-FRZ-476 | Frontend 使用 Hashed Asset + Atomic Index Release；Poker Upgrade 先 Stop New Hands/安全完成或 Pause，再 Restart→PG Recovery→Reconnect Grace，不修改 Hand/Pot/Stack。 | FROZEN |
| TD-FRZ-477 | PostgreSQL 主备份采用整个 Cluster 的 PITR-capable Physical Backup + Continuous WAL Archive，确保 `newapi` 与 `chaldea_platform` 恢复到同一时间线；每日独立 `pg_dump` 不得作为唯一金融备份。 | FROZEN |
| TD-FRZ-478 | Production 至少拥有一个加密 Off-host Backup Repository；Same-VPS Directory/Volume Copy 永远不能计作正式 DR Backup。 | FROZEN |
| TD-FRZ-479 | 每日 Logical Dump + Role/Grant/Schema/Release Manifest 作为 Portable Secondary Backup；NewAPI 非 DB Volume 是否需要纳入 Backup 必须 SOURCE VERIFICATION REQUIRED。 | FROZEN |
| TD-FRZ-480 | Backup Repository 与 Secret Recovery Kit 分离；Recovery Kit 必须安全保存 Fairness Historical Key Ring、Backup Recovery Key 和无法重新生成的运行 Secret。 | FROZEN |
| TD-FRZ-481 | V1 Backup Retention Target：30-day PITR、8 Weekly Restore Points、6 Monthly Restore Points；Portable Logical Backup 7 Daily + 4 Weekly。 | FROZEN |
| TD-FRZ-482 | V1 PostgreSQL Durable Fact 工程目标冻结为 RPO≤5min、技术 RTO≤2h；Redis 不提供 Durable RPO，允许从 Empty State 重建。 | FROZEN |
| TD-FRZ-483 | WAL/Physical/Logical Backup Freshness 与 Repository Integrity 均必须监控；超出 RPO Target 触发 Critical，且阻止 High-risk Deploy/Cutover。 | FROZEN |
| TD-FRZ-484 | Backup 成功必须包含 Checksum/WAL Continuity/Repository Verification，不以单纯 Process Exit 0 判断可恢复。 | FROZEN |
| TD-FRZ-485 | Restore Drill：Production 上线前必须完整执行；之后每月隔离 DB Restore、每季度 Full Application DR Drill，Backup/Encryption Mechanism 变化后立即补测。 | FROZEN |
| TD-FRZ-486 | Production Backup Restore Drill 必须隔离公网和外部业务副作用；不得让 Production 数据副本自动运行 Reward/Announcement/API/Poker Side Effects。 | FROZEN |
| TD-FRZ-487 | Full DR 启动必须启用 Deployment-level `DR_RECOVERY_LOCK`，独立于恢复出来的 Maintenance State，在 Integrity Verification 完成前阻断所有新用户写入/新游戏/新 Poker/发布型 Job。 | FROZEN |
| TD-FRZ-488 | Full DR 顺序固定为 Clean Host→Recovery Keys→PG PITR→Redis Empty→Services Recovery-lock→Domain Recovery/Reconciliation→Invariant Report→Operator Approval→Open Traffic。 | FROZEN |
| TD-FRZ-489 | Post-restore 必须验证 Wallet/Ledger/Transfer/Reward/Round/Poker/Fairness/Migration/Schema/Ranking/Audit/NewAPI Integration Invariants 后才能解除 DR Lock。 | FROZEN |
| TD-FRZ-490 | Redis Complete Loss 只导致 Session/Cache/Presence/Short State 丢失；Wallet/Game/Poker/Jobs 必须从 PostgreSQL Authority 恢复，不得从 Redis Backup 恢复金融事实。 | FROZEN |
| TD-FRZ-491 | VPS/Disk Total Loss 的正式路径是 Clean Host + Immutable Release + Off-host PITR + Recovery Kit；疑似 Host Compromise 必须在 Clean Host 恢复并轮换 Credentials，不原地“清理后继续生产”。 | FROZEN |
| TD-FRZ-492 | PITR 只用于灾难性 Cluster Loss/Broad Corruption/Safe Pre-open Rollback；平台已产生后续经济事实时，单记录错误使用 Incident/Reconciliation/Compensation，不使用全库时间倒退。 | FROZEN |
| TD-FRZ-493 | Backup Credential 泄露时必须 Revoke/Rotate/Integrity Verify/New Baseline；若 Backup Provider 支持 Versioning/Object Lock，应启用以降低恶意删除风险。 | FROZEN |
| TD-FRZ-494 | Production Artifact Supply Chain 使用 Dependency Lock/Go Sum/Image Digest、Secret Scan、Vulnerability Scan、SBOM 与 Release Manifest；禁止 Runtime `curl|sh` 和 Floating `latest`。 | FROZEN |
| TD-FRZ-495 | Host Infrastructure Runbook 必须涵盖 Key-based Admin Access、Firewall、Security Updates、Time Sync、Disk Monitoring；这些属于 Infra，不进入 Chaldea Operations Shell。 | FROZEN |
| TD-FRZ-496 | TD-12 Implementation 必须通过 Edge Exposure、CSP/CSRF/Service Identity、Secret/Seed Rotation、Redis-loss、PITR/Restore/DR Lock、Deployment Rollback、Observability Redaction/Cardinality、Supply-chain 和 Full Restore Test Gate。 | FROZEN |

---

# 892. Change Log — WORKING v1.2

## Added

- 用户正式确认 TD-12；
- 冻结 `TD-FRZ-440 ～ TD-FRZ-496`；
- 冻结 Threat Model / Data Classification；
- 冻结 Edge-only Public Exposure；
- 冻结 TLS / CSP / HSTS / Security Headers；
- 冻结 Session Fixation / CSRF / SSRF / Rate Limit；
- 冻结 Platform↔Poker Service Identity；
- 冻结 Secret File Injection；
- 冻结 AES-256-GCM Seed Encryption / Key Rotation；
- 冻结 PostgreSQL / Redis Hardening；
- 冻结 Poker WS Security / Debug Endpoint Boundary；
- 冻结 Structured Logs / Metrics / Request Correlation；
- 冻结 Out-of-band Alerts；
- 冻结 8GB / 4GB Monitoring Profile；
- 冻结 NTP / Time Sync；
- 冻结 Compose / Docker Network / Container Hardening；
- 冻结 Resource Governance / Emergency Swap；
- 冻结 Graceful Shutdown；
- 冻结 Immutable Release Manifest / Digest；
- 冻结 High-risk Deploy / Expand-Migrate-Contract；
- 冻结 PostgreSQL Physical Backup + WAL + PITR；
- 冻结 Off-host Encrypted Backup；
- 冻结 Secondary Logical Dump；
- 冻结 Recovery Kit；
- 冻结 Backup Retention / RPO / RTO；
- 冻结 Restore Drill；
- 冻结 `DR_RECOVERY_LOCK`；
- 冻结 Full DR / Post-restore Invariant Gate；
- 冻结 Redis-loss / VPS-loss / Host-compromise Recovery；
- 冻结 PITR vs Business Repair Boundary；
- 冻结 Supply-chain / SBOM / Host Baseline；
- 冻结 TD-12 Security / DR Test Gate。

## Existing Open Items Preserved

```text
TD-05 Reward OPEN fields

POKER-PROD-GAP-01
POKER-PROD-GAP-02
POKER-PROD-GAP-03
POKER-PROD-GAP-04
POKER-PROD-GAP-05

TD-09-PROD-GAP-01
```

## Implementation Verification Dependencies

```text
DEPLOYMENT-VERIFY-01
Actual VPS Edge / DNS / Port / Docker Network inspection.

NEWAPI-SOURCE-VERIFY
Actual NewAPI source/deployment verification for integration, auth,
Redis and backup-relevant non-DB persistence.
```

---

# 894. TD-13 — API Contract & Technical Design FINAL Audit

> 状态：`FROZEN / FINAL AUDIT PASSED`  
> 用户确认：`可以`

## 894.1 TD-13 总体结论

TD-13 不增加新的产品规则，而是完成以下最终收口：

- Chaldea Browser BFF Versioning；
- REST Success / Error Envelope；
- HTTP Status Mapping；
- Request / Idempotency / Business / Resource Identity；
- Amount / ID / Time / Enum Serialization；
- Pagination / Filter / Cache Contract；
- Session / Auth / Master / Account API；
- Models / API Keys / API Usage API；
- Wallet / Rewards API；
- Game Registry / Direct Play / Blackjack API；
- Poker HTTP + WS Final Protocol Map；
- Rankings / History / Announcements API；
- Operations / Maintenance / Jobs Command Contract；
- OpenAPI / Poker WS Schema；
- Initial Super Admin Bootstrap；
- NewAPI `SV-01 ～ SV-16` Source Verification Register；
- Product OPEN Register；
- Implementation-only Config Register；
- Production Verification Register；
- State Machine / Exactly-once / Schema / Auth / Security / IA / Art / DR Cross-audit；
- FINAL Test Gate；
- Technical Design v0.5 FINAL Packaging。

审计结果：

```text
Frozen Decision Register
TD-FRZ-001 ～ 496
→ continuous / no duplicate register entry

Existing Decision requiring SUPERSEDED
→ none

Missing Technical Decision
→ Initial Super Admin Bootstrap
→ closed in TD-13

Cross-batch contradiction requiring supersession
→ none

Product OPEN
→ preserved explicitly

NewAPI implementation unknowns
→ consolidated into SV-01 ～ SV-16
```

---

# 895. Final Web / API Origin Boundary

页面仍使用 IA FINAL 已冻结 Route。

Chaldea Browser Application：

```text
https://<chaldea-web-origin>
```

同一 Web Origin 下：

```text
/
→ React / Vite Frontend

/api/v1/*
→ Chaldea Platform Backend BFF

/ws/poker
→ Poker WebSocket Service
```

外部模型调用继续使用独立 API Origin：

```text
https://<api-origin>
→ NewAPI Core
```

External Model API 不属于 Cookie-auth BFF。

页面 Route 与 BFF Route 永久分开：

```text
/wallet
!=
/api/v1/wallet
```

---

# 896. BFF Versioning

Chaldea Browser BFF 固定：

```text
/api/v1/*
```

V1 内允许向后兼容 Additive Change，例如新增可选字段、Endpoint 或明确可忽略的新能力。

以下属于 Breaking Change：

```text
remove/rename required field
change field meaning
change amount semantics
change enum meaning
remove required endpoint
change terminal state semantics
```

Breaking Change 必须进入新的 Major API Version，例如：

```text
/api/v2/*
```

不能在 `/api/v1` 静默改变语义。

---

# 897. Common Success Envelope

普通 JSON Read / Command 成功：

```json
{
  "data": {},
  "meta": {
    "request_id": "01...",
    "server_time": "2026-09-03T17:22:11.123Z"
  }
}
```

List：

```json
{
  "data": {
    "items": []
  },
  "meta": {
    "request_id": "01...",
    "server_time": "2026-09-03T17:22:11.123Z",
    "pagination": {}
  }
}
```

OAuth Browser Callback 等 Redirect Endpoint 不强制套 JSON Envelope。

---

# 898. Common Error Envelope

统一：

```json
{
  "error": {
    "code": "INSUFFICIENT_CHIPS",
    "message_key": "errors.insufficient_chips",
    "message": "Insufficient entertainment chips.",
    "request_id": "01...",
    "retryable": false,
    "details": {},
    "current_state": {},
    "next_actions": []
  }
}
```

`code` 是稳定机器语义。

`message` 是安全 fallback，不是 Stack Trace。

`details` 必须为按 Error Code 定义的 Typed Safe DTO。

禁止放入：

```text
raw exception
SQL error
raw upstream body
secret
private game state
```

---

# 899. Field Validation Error

Semantic Validation 使用 `422`：

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "field_errors": [
      {
        "field": "master_nickname",
        "code": "NICKNAME_RESERVED"
      }
    ]
  }
}
```

Frontend 不通过自然语言错误字符串猜字段。

---

# 900. Final HTTP Status Mapping

| HTTP | Contract |
|---|---|
| `200` | Read / successful command / idempotent replay |
| `201` | Durable Resource created |
| `202` | Durable request accepted and still Processing |
| `400` | Malformed / unsupported request syntax |
| `401` | No valid authenticated Session |
| `403` | Authenticated but forbidden / Fresh Auth or security gate required |
| `404` | Not found or intentionally concealed private resource |
| `409` | State / version / idempotency / concurrency conflict |
| `422` | Well-formed but semantic field validation failed |
| `429` | Rate limited |
| `503` | Maintenance or required dependency temporarily unavailable |
| `500` | Unexpected internal failure |

NewAPI Dependency Error 不向 Browser 直接透传 Raw Upstream Body。

优先映射为安全 Domain Error，例如：

```text
503
NEWAPI_TEMPORARILY_UNAVAILABLE
```

---

# 901. `202 Accepted`

`202` 只有在 Durable Business / Resource Identity 已存在时才允许返回。

例如：

```json
{
  "data": {
    "transfer_id": "...",
    "state": "SOURCE_DEBITING"
  }
}
```

适用：

```text
Cross-DB Exchange
Pending Safe Leave
Pending Poker Top-up
Ranking Rebuild
Cross-system Admin Operation
```

禁止：

```text
202
→ server has persisted nothing
```

---

# 902. Request Correlation and Identity Classes

Client 可发送：

```http
X-Request-ID: ...
```

Server 接受安全合法值或生成新值，并返回最终 Request ID。

以下四类身份永久分离：

```text
request_id
idempotency_key
biz_id / operation_id
resource_id
```

例如 Exchange：

```text
request_id
→ one transport attempt

Idempotency-Key
→ one user intent

biz identity
→ one business effect

transfer_id
→ durable resource identity
```

---

# 903. Idempotency Header

所有用户发起的 Durable Business Mutation 使用：

```http
Idempotency-Key: <opaque-random-value>
```

包括：

```text
Master initialize / save
API Key create / manage
Exchange
Reward Claim
Round Create / Action
Poker Buy-in / Top-up / Safe Leave / Take Over
Announcement Read
Operations Mutation
```

GET、Login、Logout、OAuth Start 等不要求此 Header，除非具体 Domain Contract 另有规定。

Server Scope：

```text
authenticated actor
+
HTTP method
+
route/business action
+
Idempotency-Key
```

并绑定 Normalized Request Hash。

---

# 904. Idempotency Conflict / Replay

同 Key + 同 Request：

```text
return same business resource/effect
```

可返回：

```http
Idempotent-Replayed: true
```

若资源后续状态已前进，可返回同一资源当前 Authoritative Representation，但不能创建第二份。

同 Key + 不同 Request：

```text
409 IDEMPOTENCY_CONFLICT
```

---

# 905. Unknown Network Result

Timeout / Reset / Refresh 后：

```text
query original resource / business identity
```

禁止生成新 Idempotency Key 自动重放 Financial/Game/Poker/Admin Mutation。

---

# 906. Identifier Serialization

所有外部 JSON ID：

```text
string
```

包括：

```text
newapi_user_id
newapi_key_id
model_id
Discord User ID when authorized
UUIDv7 resources
```

Chaldea UUID：

```text
canonical lowercase string
```

这样 BFF 不暴露 NewAPI 内部 ID 数据类型。

---

# 907. Amount / Decimal Serialization

金融：

```text
*_units
```

永远使用 Decimal Integer String。

例如：

```json
{
  "available_chip_units": "500000000",
  "available_chip_display": "1000"
}
```

需要精确数学语义的：

```text
API Credit Decimal
RTP
Multiplier
Ratio
Precise Rate
```

同样优先 String。

普通计数、Rank、Version、Event Sequence 可使用安全整数。

---

# 908. Time / Date / Enum Contract

Instant：

```text
RFC3339 UTC
2026-09-03T17:22:11.123Z
```

Business Date：

```text
YYYY-MM-DD
```

Asia/Shanghai 周期必须明确：

```text
period_start
period_end
business_timezone = "Asia/Shanghai"
```

Enum：

```text
SCREAMING_SNAKE_CASE
```

---

# 909. Null / Optional Contract

```text
field absent
→ capability / optional data not provided

field = null
→ domain explicitly models null/empty
```

不把所有 Response 塞成几十个无意义 `null` 字段。

---

# 910. Pagination

## Page Pagination

适用：

```text
Models
Game Catalog
Rankings
Operations configuration lists
```

参数：

```text
page
page_size
sort
```

Response：

```text
page
page_size
total_items
total_pages
```

## Cursor Pagination

适用：

```text
API Usage Requests
Wallet Transactions
Game History
Audit
Jobs
Incident Events
```

稳定 Keyset：

```text
occurred_at DESC
+
stable_id DESC
```

返回：

```text
next_cursor
has_more
```

Default / Maximum Page Size 进入 Implementation Spec。

---

# 911. Filter / Sort Allowlist and Cache

Sort / Filter 都必须 Code-allowlisted。

未知字段：

```text
400 INVALID_FILTER
```

禁止 Raw SQL Sort Expression。

Chaldea BFF JSON 默认：

```http
Cache-Control: no-store
```

未来若建立 Session-independent Public Cache，必须单独定义 Cache-safe Contract。

---

# 912. Session Bootstrap

```http
GET /api/v1/session/bootstrap
```

匿名也可调用。

至少提供：

```text
authentication state
CSRF token
account status
Master initialization state
Migration Notice state
safe identity summary
maintenance/resource summary
Operations principal summary if authorized
```

Protected Data 只在 Auth/Gate 允许后返回。

Cookie-auth Durable Write 使用：

```http
X-CSRF-Token: ...
```

Token 来源为 Server Session Contract。

---

# 913. Authentication API

```text
POST /api/v1/auth/password/login
POST /api/v1/auth/logout
POST /api/v1/auth/fresh/password
```

Fresh Password：

```text
Current Password
→ NewAPI Verify
→ rotate Session if required
→ update fresh_auth_at
```

Discord OAuth Start 按固定 Purpose 分开：

```text
POST /api/v1/auth/discord/login/start
POST /api/v1/registration/discord/start
POST /api/v1/auth/discord/fresh/start
POST /api/v1/auth/discord/password-reset/start
```

共享 Callback：

```text
GET /api/v1/auth/discord/callback
```

Purpose 只从 Server-side OAuth Flow State 取得，不信任 Client Query Parameter。

Registration Status：

```text
GET /api/v1/registration/status
```

---

# 914. Master / Account API

```text
GET   /api/v1/master-profile
POST  /api/v1/master-profile/initialize
PATCH /api/v1/master-profile

GET   /api/v1/account/security

GET   /api/v1/migration-notice
POST  /api/v1/migration-notice/acknowledge
```

Password：

```text
POST /api/v1/account/password/set
POST /api/v1/account/password/change
POST /api/v1/account/password/reset
```

Password 永远由 NewAPI Authority 保存/验证；Chaldea 不保存第二份密码或 Hash。

底层 NewAPI Password Contract 继续 Source Verification。

---

# 915. Composite Read APIs

允许：

```text
GET /api/v1/home
GET /api/v1/dashboard
GET /api/v1/me
GET /api/v1/entertainment
```

它们仅组合 Read Projection。

不得隐式执行：

```text
Reward Claim
Exchange
Game Round
Poker Action
```

---

# 916. Models / API Access

```text
GET /api/v1/models
GET /api/v1/models/{model_id}

GET /api/v1/api-access
```

`api-access` 可使用 `model_id` 查询，返回：

```text
External API Base URL
Model ID
Supported invocation information
Safe cURL / compatible examples
```

不返回用户真实 API Secret，不提供 Web Playground。

---

# 917. API Keys BFF

```text
GET    /api/v1/api-keys
POST   /api/v1/api-keys

PUT    /api/v1/api-keys/{key_id}/purpose

POST   /api/v1/api-keys/{key_id}/disable
POST   /api/v1/api-keys/{key_id}/enable

DELETE /api/v1/api-keys/{key_id}
```

New Key 必须选择：

```text
GENERAL
or
RP
```

`UNCLASSIFIED` 仅用于迁移既有 Key。

Usage Purpose 只影响 Chaldea Attribution，不改变 Key 权限、路由和计费。

Native Key Capabilities 继续由 NewAPI Source Verification 决定。

---

# 918. API Key Reveal / One-time Secret

只有真实 NewAPI 能力支持时才开放：

```text
POST /api/v1/api-keys/{key_id}/reveal
```

如果 NewAPI 只保存不可逆摘要：

```text
Reveal capability unavailable
```

One-time Secret Create Response 丢失时：

```text
Key exists
Secret unavailable
```

Chaldea 不为重放 Response 保存第二份 API Key Secret。

用户只能显式撤销/删除后创建新 Key。

---

# 919. API Usage

```text
GET /api/v1/api-usage/summary
GET /api/v1/api-usage/requests
GET /api/v1/api-usage/requests/{logical_request_id}
```

Request Detail 仅 Owner / Authorized Operations 可读。

RP Attribution 不为排行榜新增 Prompt / Full Response 采集。

---

# 920. Wallet / Economy API

```text
GET /api/v1/wallet

GET /api/v1/wallet/transactions
GET /api/v1/wallet/transactions/{transaction_id}

POST /api/v1/wallet/exchanges
GET  /api/v1/wallet/exchanges/{transfer_id}
```

不提供用户：

```text
Manual Active Quota Top-up Endpoint
```

Exchange 如进入跨库 Processing：

```text
202
+
same transfer_id
+
current state
```

---

# 921. Rewards API

```text
GET  /api/v1/rewards
POST /api/v1/rewards/claims
GET  /api/v1/rewards/claims/{claim_id}
```

Client 只提交：

```text
reward_kind
```

不提交：

```text
reward amount
asset type
eligibility decision
```

Server 读取 Active Validated Policy。

`UNRESOLVED` Required Policy 不得 Production Active。

---

# 922. Reward Maintenance Clarification

TD-10 的：

```text
REWARDS Maintenance Scope
```

仅是平台技术 Admission Gate：

```text
block new Claims
continue accepted Claim recovery
```

它不等于已经解决 TD-05 仍 OPEN 的：

```text
Reward Product Maintenance / Temporary Disable Policy
```

因此 Reward Product OPEN 保持 OPEN，不需要 Supersede TD-10。

---

# 923. Game Catalog / Bootstrap

```text
GET /api/v1/games

GET /api/v1/games/{game_slug}/bootstrap
```

Catalog 由 Dynamic Registry 驱动。

Bootstrap 一次返回：

```text
Registry Metadata
Effective Runtime State

Ruleset Version
Config Summary / Hash
Wager Policy

Available Chips
Active Round
Effective Entry Action

Next Fairness Commitment
Client Seed Preference
```

存在 Active Round 时 Resume-first。

---

# 924. Direct Play Round API

```text
POST /api/v1/games/{game_slug}/rounds

GET /api/v1/games/{game_slug}/rounds/active

GET /api/v1/game-rounds/{round_id}

POST /api/v1/game-rounds/{round_id}/actions

GET /api/v1/games/{game_slug}/client-seed
PUT /api/v1/games/{game_slug}/client-seed

GET /api/v1/game-rounds/{round_id}/fairness
```

Create Round Body 使用 Game-specific Typed Union。

Blackjack Action 使用：

```text
action_id
expected_round_version
```

Client Seed 修改只影响下一合法 Round。

---

# 925. Poker HTTP BFF

Lobby / Table：

```text
GET  /api/v1/poker

GET  /api/v1/poker/tables
POST /api/v1/poker/tables
GET  /api/v1/poker/tables/{table_id}

POST /api/v1/poker/tables/{table_id}/access
POST /api/v1/poker/tables/{table_id}/seat-reservations
POST /api/v1/poker/tables/{table_id}/buy-ins

POST /api/v1/poker/tables/{table_id}/commands
```

Session：

```text
GET  /api/v1/poker/sessions/active
GET  /api/v1/poker/sessions/{session_id}

POST /api/v1/poker/sessions/{session_id}/top-ups
POST /api/v1/poker/sessions/{session_id}/safe-leave
POST /api/v1/poker/sessions/{session_id}/take-over
```

Connection / Fairness：

```text
POST /api/v1/poker/connect-tickets
GET  /api/v1/poker/hands/{hand_id}/fairness
```

Host Command 必须是 Code-allowlisted Typed Union，不接受 arbitrary command string。

---

# 926. Poker WS Final Contract

Path：

```text
WSS /ws/poker
```

Subprotocol：

```text
chaldea-poker.v1
```

Breaking Protocol 使用新 Major，例如：

```text
chaldea-poker.v2
```

Client Envelope：

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

Server Envelope：

```json
{
  "type": "hand.action_applied",
  "event_id": "...",
  "event_seq": 105,
  "table_id": "...",
  "table_version": 13,
  "hand_id": "...",
  "hand_version": 58,
  "server_time": "2026-09-03T17:22:11.123Z",
  "payload": {}
}
```

---

# 927. Poker WS Message Families

Client 至少：

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

Server 至少：

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

Low-frequency Host/Ops Durable Command 使用 HTTP Contract。

---

# 928. Poker Viewer Projection

所有 WS Payload 先执行 Server-side Viewer Projection。

Browser 永不接收：

```text
unauthorized Hole Cards
Future Deck
Unreleased Seed
```

后再依靠 UI 隐藏。

Client 不根据 Event 文案自己推算正式 Pot / Stack / Legal Action。

---

# 929. Rankings / Historical Snapshot API

```text
GET /api/v1/rankings

GET /api/v1/rankings/snapshots
GET /api/v1/rankings/snapshots/{snapshot_id}
```

Current Ranking 支持：

```text
domain
metric
period
model_id
page/page_size
```

Response：

```text
entries
period
data_freshness
last_updated
my_rank nullable
```

---

# 930. History API

```text
GET /api/v1/history

GET /api/v1/history/rounds/{round_id}
GET /api/v1/history/sessions/{session_id}
GET /api/v1/history/hands/{hand_id}
```

Detail 永远读取 Durable Domain Source，不以 History Index Payload 充当 Authority。

Owner / Authorized Records Scope 才能读取完整私人 History。

Poker Reveal Boundary 不因 History / Admin 权限被绕过。

---

# 931. Announcements API

```text
GET /api/v1/announcements
GET /api/v1/announcements/{announcement_id}

GET /api/v1/announcements/current-entry-popup
GET /api/v1/announcements/current-post-login-popup

POST /api/v1/announcements/{announcement_id}/reads
```

Read Body 绑定：

```text
notification_revision
```

Anonymous Entry Popup Dismissal 继续 Browser-local。

Published Update 继续严格区分：

```text
CONTENT_UPDATE
→ content_version++
→ notification_revision unchanged

RE_NOTIFY
→ notification_revision++
```

---

# 932. Operations Bootstrap / Reads

```text
GET /api/v1/ops/bootstrap

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

业务模块 Read Family 继续位于：

```text
/api/v1/ops/models
/api/v1/ops/users
/api/v1/ops/games
/api/v1/ops/poker/*
/api/v1/ops/economy/*
/api/v1/ops/rewards/*
/api/v1/ops/rankings/*
/api/v1/ops/records
/api/v1/ops/announcements
```

全部 Server-side RBAC Filter。

---

# 933. Operations Command Contract

Level 2 / Level 3 写入统一：

```text
POST /api/v1/ops/operations
```

创建 Prepared Operation。

Request：

```json
{
  "operation_type": "ECONOMY_ADJUSTMENT",
  "target": {},
  "input": {},
  "reason": "..."
}
```

Server 决定：

```text
required permission
risk level
impact preview
typed confirmation challenge
```

Client 不允许自行降低 Risk。

执行：

```text
POST /api/v1/ops/operations/{operation_id}/execute
```

状态：

```text
GET /api/v1/ops/operations/{operation_id}
```

Level 1 Write 可以使用同一 Engine，由 Server 原子 Prepare + Execute。

不存在 Generic State PATCH 后门。

---

# 934. Maintenance / Jobs Operations

仍通过 Code-owned Operation Type：

```text
MAINTENANCE_CREATE
MAINTENANCE_CANCEL
MAINTENANCE_END

JOB_RETRY
JOB_RESUME
JOB_CANCEL

RANKING_REBUILD_CREATE
RANKING_REPAIR_PUBLISH

POKER_EMERGENCY_PAUSE

DISCORD_REBIND
ACCESS_CONTROL_CHANGE
```

不能提交 arbitrary SQL / Shell / command payload。

---

# 935. Concurrent Mutation Versioning

需要防止 Stale UI 的 Resource Mutation 使用：

```text
expected_version
```

例如：

```text
Master Profile
Config Draft
Announcement Draft
Admin Principal
Blackjack Round
Poker Hand
```

Mismatch：

```text
409 STALE_RESOURCE_VERSION
```

返回 Safe Current State。

正式状态机使用 Typed Command，不允许：

```text
PATCH {"state":"SETTLED"}
```

绕过流程。

---

# 936. Safe Retry Metadata

Server 确定可重试时：

```text
retryable = true
retry_after_seconds
```

429 / 503 在已知时也可返回：

```http
Retry-After
```

业务 Cooldown 返回：

```text
eligible_at
```

Client 不自己累计权威时间。

---

# 937. Machine-readable Contract Files

最终建立：

```text
contracts/
├── openapi/
│   └── chaldea-bff-v1.yaml
│
└── poker-ws/
    ├── envelope.schema.json
    ├── client-messages/
    └── server-events/
```

OpenAPI 生成：

```text
TypeScript DTOs
TypeScript Client Types
Enums
Error Codes
Pagination Types
```

Poker WS 使用 JSON Schema / 等价 Typed Schema 独立管理。

---

# 938. Contract CI

Pull Request 必须检查：

```text
OpenAPI syntax/semantic validation
Poker WS schema validation
Generated Client freshness
Breaking-change diff
Example / Fixture validation
```

Contract Example 只使用 Demo Data。

不得放入 Production：

```text
Account ID
API Secret
Prompt / Response
Private Poker Card
```

---

# 939. Initial Super Admin Bootstrap

补齐此前唯一明确缺失的技术决策。

采用：

> **Deployment-only One-shot Super Admin Bootstrap**

逻辑：

```text
Deployment Operator
→ one-shot bootstrap tool
→ provide stable newapi_user_id
→ verify target user exists
→ verify admin_principal count = 0
→ create SUPER_ADMIN principal
→ write role history
→ write SYSTEM_BOOTSTRAP audit
→ COMMIT
```

Bootstrap 仅在：

```text
admin principal count = 0
```

时允许。

一旦存在管理员：

```text
bootstrap refuses permanently
```

后续管理员全部通过 TD-10 Access Control Level 3 创建/修改。

---

# 940. Bootstrap Security

Bootstrap 输入只需要：

```text
stable newapi_user_id
```

不要求：

```text
password
Discord OAuth Secret
API Key Secret
```

目标用户存在性必须通过 Source-verified NewAPI Read/Adapter 验证。

Audit Actor：

```text
SYSTEM_BOOTSTRAP
```

记录：

```text
environment
target newapi_user_id
created principal
timestamp
release/build
```

绝不提供：

```text
/api/v1/bootstrap-super-admin
```

公网 Endpoint。

具体 CLI/One-shot Container 命令留 Implementation Spec。



# 941. NewAPI Source Verification Inventory

TD-13 将此前散落的 `SOURCE VERIFICATION REQUIRED` 项统一收口为 `SV-01 ～ SV-16`。

## SV-01 — Session / Authentication

核验：

```text
NewAPI password verification capability
session / auth lifetime
other-session revoke ability
```

## SV-02 — Password Identifier

核验：

```text
OAuth-created account stable login identifier
actual login field
mutability
```

## SV-03 — Password Set / Change / Reset

核验：

```text
safe NewAPI API / bridge
real password policy
```

## SV-04 — Discord Binding

核验：

```text
binding storage
one-to-one uniqueness
existing-binding lookup
admin rebind capability
```

## SV-05 — NewAPI Identity Types

核验：

```text
user ID
API Key ID
model/log identifiers
```

BFF 外部 JSON 继续统一 String。

## SV-06 — API Key Native Operations

核验：

```text
create fields
disable / enable
delete
reveal capability
one-time secret behavior
native metadata
```

## SV-07 — NewAPI Admin Detection

核验真实：

```text
admin role / permission
NewAPI Admin access
```

## SV-08 — Cutover Write Freeze

核验：

```text
how to stop new registration
how to stop quota charging / relevant writes
```

## SV-09 — Raw Quota

核验：

```text
read
reset to zero
delta mutation
before / after
```

## SV-10 — Quota Idempotency

核验：

```text
native idempotency available?
```

若没有，使用已经冻结的：

```text
Quota Bridge
+
NewAPI-local operation journal
```

## SV-11 — Reactive Refill Hook

核验如何在：

```text
insufficient Active quota
or
request admission
```

触发 Reserve → Active Refill。

## SV-12 — API Request Attribution

核验 NewAPI Log 是否可提供：

```text
logical request
user
key
requested model
status
charge
timestamp
```

## SV-13 — Chaldea→NewAPI Authentication

核验：

```text
internal API credential
bridge authentication
least-privilege mechanism
```

## SV-14 — NewAPI Redis

核验：

```text
current Redis auth
namespace
ACL compatibility
```

## SV-15 — NewAPI Persistent Volume / Backup

核验：

```text
which non-DB files are non-reconstructible
what must enter DR backup
```

## SV-16 — Public NewAPI Route Allowlist

核验：

```text
api origin
formal model API paths
```

禁止把整个 NewAPI Web/Admin Surface 直接公开反代。

---

# 942. FINAL Product OPEN Register — Rewards

以下继续 OPEN：

```text
Hourly:
- Asset Type
- Natural Hour vs Rolling 60m
- Accumulation
- Daily Limit

Relief:
- Asset Type
- Eligibility Accumulation
- Active Poker Session behavior

Rewards:
- Final product maintenance / temporary disable policy
- Whether future versioned policy may alter current fixed amounts
- Issuance alert threshold
```

已冻结不变：

```text
Initial Grant = 1000
Daily = 500
Hourly Amount = 100
Relief Amount = 300
Relief Total Assets < 10
Relief success → rolling 4h cooldown
```

Implementation 不得自行补 Product OPEN。

---

# 943. FINAL Product OPEN Register — Poker

继续：

```text
POKER-PROD-GAP-01
Ante Posting Mode

POKER-PROD-GAP-02
Post BB Now live/dead semantics

POKER-PROD-GAP-03
Initial Dealer Button rule

POKER-PROD-GAP-04
Hand Evaluator edge/tie rules

POKER-PROD-GAP-05
Pot Shortcut exact raise formula
```

在全部确认并形成正式 Ruleset Version 前：

```text
Poker Ruleset != PRODUCTION_READY
```

允许先实现不依赖这些 Gap 的 Actor / Persistence / Recovery / Protocol。

禁止用开发者/模型常识补 Production Rule。

---

# 944. FINAL Product OPEN Register — Public Records

继续：

```text
Recent Win minimum net
Big Win threshold
display count
automatic selection
Featured Record promotion
```

直到产品确认前：

```text
PUBLIC_RECORD_SELECTION_POLICY = UNRESOLVED
```

因此 Public Recent Wins / Featured Records 模块没有合格 Policy 时直接隐藏。

不得生成虚假中奖记录填充首页。

---

# 945. Implementation-only Config Register

以下属于 Implementation Spec / Load Test，而不是 Product OPEN：

```text
Session correlated Idle / Absolute TTL
Active Quota LOW_WATERMARK
Active Quota TARGET_WATERMARK
Active Quota MAX_ACTIVE_BUFFER

Read Retry Count
HTTP Retry Backoff
Poker WS Reconnect Backoff

Rate Limit thresholds
Request Body/Header limits

Service Assertion Clock Skew

Container CPU/RAM hard limits
PostgreSQL memory tuning
Redis memory/cache sizing

JS Bundle Regression Budget
Runtime Log hard-size cap
Generic operational alert thresholds
```

这些值必须通过实现、压测或部署现场核验确定。

---

# 946. Production Verification Register

Production Readiness 继续包含：

```text
DEPLOYMENT-VERIFY-01

Actual VPS:
- Edge Proxy
- DNS
- 80/443 ownership
- Docker Networks
- current NewAPI Compose topology
```

以及：

```text
NEWAPI-SOURCE-VERIFY
SV-01 ～ SV-16
```

以及 Art Direction 已冻结的：

```text
Rights Review
Production Asset Gate
Accessibility Gate
Performance Gate
Media Fallback Gate
```

这些是 Production Verification，不是架构未决定。

---

# 947. State Machine Cross-audit

已审计：

```text
Auth / Session
Migration Batch / User
Economy Transfer
Reward Claim
Game Round
Blackjack
Poker Table
Poker Hand
Poker Session
Ranking Aggregate
Announcement
Background Job
Admin Operation
Maintenance
```

结论：

```text
No conflicting terminal definitions found.
```

未发现需要把既有 TD-FRZ 标记为 `SUPERSEDED` 的状态机冲突。

---

# 948. Settlement / Refund Cross-audit

Direct Play：

```text
SETTLED
xor
REFUNDED
```

Poker Hand：

```text
SETTLED
xor
REFUNDED
```

严格互斥。

任何 Round / Hand 不允许：

```text
payout
+
refund
```

同时成为正式终态。

---

# 949. Exactly-once Cross-audit

统一：

> **Transport / Worker Delivery 可以 At-least-once；Durable Business Effect 通过 Stable Identity + Idempotency + Unique Constraint + Transaction 实现 Effectively/Committed Exactly-once。**

适用：

```text
Wallet Mutation
Exchange Saga
Reward Claim
Game Round
Game Settlement
Poker Action
Poker Hand Settlement
Poker Funding
Cash Out
Admin Operation
Background Job
```

没有任何技术决定宣称 HTTP / WS / Network 自带 Exactly-once。

---

# 950. Economy Authority Cross-audit

正式 Authority：

```text
NewAPI
→ Active Raw Quota

Chaldea Economy
→ Reserve API Credit
→ Available Chips

Poker
→ Table Stack / Commitments

PostgreSQL
→ Durable Business Truth
```

Redis / Frontend / Ranking Projection 均不成为资产 Authority。

无冲突。

---

# 951. Poker Authority Cross-audit

正式：

```text
Table Actor
→ serialization coordinator

PostgreSQL
→ Table / Seat / Session / Hand / Action / Pot / Settlement Authority

Redis
→ ephemeral connection/cache/presence
```

Frontend：

```text
projection only
```

无冲突。

---

# 952. Schema Ownership Cross-audit

保持：

```text
newapi
→ NewAPI Core Authority

chaldea_platform
→ Chaldea Authority
```

没有引入：

```text
cross-DB foreign key
cross-DB trigger
FDW pretending atomicity
distributed 2PC
```

跨库资产继续 Saga / Reconciliation。

---

# 953. Authentication Cross-audit

保持：

```text
NewAPI Account Identity
+
Chaldea Opaque Web Session
```

并继续：

```text
No second password database
Master nickname != login identifier
Discord binding stable
```

无冲突。

---

# 954. Web Origin / CSP Cross-audit

最终：

```text
Chaldea BFF
→ same Web Origin /api/v1/*

Poker WS
→ same Web Origin /ws/poker

External Model API
→ separate API Origin
```

因此 TD-12 Browser `connect-src 'self'` Baseline 与 TD-01 / TD-11 不冲突。

External Model API 由用户的 API Client 使用，不是 Chaldea Browser BFF 调用。

---

# 955. Reward Maintenance Cross-audit

容易产生歧义的两条：

```text
TD-05:
Reward Product Maintenance Policy = OPEN

TD-10:
REWARDS Technical Maintenance Scope = FROZEN
```

最终解释：

```text
technical claim-admission gate
!=
product reward policy
```

因此：

- TD-10 Maintenance 不需要 Supersede；
- TD-05 Product OPEN 继续保留。

---

# 956. Operations Authority Cross-audit

NewAPI Admin 与 Chaldea Operations 双向独立。

Operations：

```text
No arbitrary shell
No SQL Console
No Redis Console
No direct final balance edit
No stack/pot/winner/deck edit
No score edit
```

所有写操作仍调用正式 Domain Contract。

---

# 957. Security / Privacy Cross-audit

以下：

```text
Password
Password Hash
API Key Secret
OAuth Secret
Prompt / Response beyond authorized use
Unrevealed Server Seed
Future Deck
Unauthorized Hole Cards
```

在：

```text
Frontend
Logs
Metrics
Trace
Audit
Operations
Review Fixture
```

都有明确禁止边界。

没有后续批次重新开放。

---

# 958. Backup / Recovery Cross-audit

Durable Recovery Authority：

```text
PostgreSQL
```

DR：

```text
Physical Backup
+
Continuous WAL
+
PITR
+
Encrypted Off-host Repository
+
Recovery Kit
+
DR_RECOVERY_LOCK
```

与 Economy / Poker 状态机一致。

Redis Backup 不用于覆盖正式业务事实。

---

# 959. IA Route Coverage Audit

IA Canonical Routes：

```text
/
 /dashboard

/login
/register
/onboarding/master

/models
/models/:model
/api/keys
/api/usage
/api/access

/wallet
/rewards

/entertainment
/games
/games/:game_slug

/poker
/poker/table/:id

/rankings

/history
/history/round/:id
/history/session/:id
/history/hand/:id

/announcements
/announcements/:id

/me
/master-profile
/account/security

/ops/*
```

全部存在对应：

```text
Frontend Route Family
+
BFF / Realtime Contract
```

以下继续是 Conditional Surface，而非一级 Route：

```text
Migration Notice
Entry Popup
Critical Notice
Restricted Account
Post-login Popup
```

---

# 960. Art / Asset Contract Coverage Audit

已经覆盖：

```text
Design Tokens
Responsive Tokens
Button Scope
Fonts
Layered Media
Asset Manifest
Rights Review
Reduced Motion
Reduced Media
Media Budget
Performance
Accessibility
Fallback
```

TD-13 不新增或修改 Art Direction 决定。

---

# 961. API Key IA Consistency Audit

Backend Resource API 可以存在：

```text
/api/v1/api-keys/{key_id}/...
```

但 Frontend 继续没有：

```text
/api/keys/:id
```

独立页面。

因此：

```text
Backend Resource Path
!=
Frontend Page Route
```

不构成 IA 冲突。

Reveal 只有 Source Verification 确认能力存在时才开放。

---

# 962. FINAL Blocker Classes

FINAL 之后剩余工作必须严格分四类：

## A — Product Decision Blocker

```text
Reward OPEN
Poker 5 Gaps
Recent Wins Selection
```

## B — Source Verification Blocker

```text
NewAPI SV-01 ～ SV-16
```

## C — Implementation Configuration

```text
Rate limits
Watermarks
Timeouts / Backoff
Resource Caps
Bundle Budget
etc.
```

## D — Production Readiness

```text
Rights
Final Assets
Restore Drill
Load Test
Accessibility
Deployment Verification
Security Scan
```

不再用一个模糊 `TBD` 混在一起。

---

# 963. Technical Design FINAL Meaning

`Technical Design v0.5 FINAL` 表示：

> 架构、服务边界、数据 Authority、状态机、事务/幂等、安全、接口 Contract、恢复/DR 与技术审计已经收口。

它不表示：

```text
all Product OPEN resolved
all NewAPI adapter verified
all media rights ready
production deployed
load test finished
restore drill finished
```

这些由对应 Final Registers 继续约束。

---

# 964. Implementation Spec Gate

Implementation Spec v1.0 / Codex 不得：

```text
invent Poker gap
invent Reward policy
invent NewAPI table or endpoint
invent financial retry semantics
invent hidden admin override
invent asset precision behavior
```

必须显式读取：

```text
TD-FRZ-001 ～ 552
Product OPEN Register
SV-01 ～ SV-16
Implementation Config Register
Production Readiness Register
```

---

# 965. FINAL Contract Test Gate

## REST

```text
OpenAPI valid
every handler conforms
every error code documented
every amount field string-typed
every durable mutation idempotency documented
```

## WS

```text
every Client Message typed
every Server Event typed
invalid/unknown type rejected
viewer projection privacy tested
```

## HTTP Error

```text
401
403
404
409
422
429
503
```

均有安全 DTO。

## Serialization

```text
large ID
large amount
UTC time
Asia/Shanghai period
enum
optional/null
```

## Idempotency

```text
same key same payload
same key different payload
response lost
202 polling
```

## Operations

```text
risk determined by server
Level 3 cannot skip prepare/execute guard
authz stale rejected
```

## Generation

```text
OpenAPI
Go implementation
TS generated client
Poker WS schemas
```

不得漂移。

---

# 966. TD-13 Frozen Decision Register

| ID | Decision | Status |
|---|---|---|
| TD-FRZ-497 | 补齐 Initial Super Admin：使用 Deployment-only One-shot Bootstrap，仅在 Admin Principal=0 时接受稳定 `newapi_user_id`，验证目标账号后原子创建首位 Super Admin + Role History + `SYSTEM_BOOTSTRAP` Audit；不提供 Web Bootstrap Endpoint。 | FROZEN |
| TD-FRZ-498 | TD-10 `REWARDS` Maintenance Scope 仅为平台技术 Admission Gate，不解决 TD-05 仍 OPEN 的 Reward Product Maintenance / Temporary Disable Policy；Reward Product OPEN 原样保留。 | FROZEN |
| TD-FRZ-499 | Chaldea Browser BFF 统一使用 Same-origin `/api/v1/*`；页面 Route 不加 `/api/v1`，External Model API 继续使用独立 NewAPI API Origin。 | FROZEN |
| TD-FRZ-500 | `/api/v1` 只允许向后兼容 Additive Change；Required Field/Meaning/Endpoint Removal 等 Breaking Change 必须进入新 API Major Version。 | FROZEN |
| TD-FRZ-501 | 普通 BFF JSON Success 使用 `data + meta(request_id/server_time)` Envelope；Error 使用稳定 Code/Message Key/Safe Message/Request ID/Retryable/Typed Details/Current State/Next Actions。 | FROZEN |
| TD-FRZ-502 | HTTP Status 统一为 200/201/202/400/401/403/404/409/422/429/503/500 语义；NewAPI Dependency Error 不向 Browser 透传 Raw Upstream Body。 | FROZEN |
| TD-FRZ-503 | `202 Accepted` 必须已经拥有 Durable Business/Resource Identity，不允许“后台也没落库”的伪 Accepted。 | FROZEN |
| TD-FRZ-504 | `request_id / idempotency_key / biz_id(or operation_id) / resource_id` 四种身份永久分离，Request Correlation 不代替业务幂等。 | FROZEN |
| TD-FRZ-505 | 所有用户发起的 Durable Business Mutation 使用 `Idempotency-Key`；同 Key/同 Request Hash 返回同一业务资源，不产生第二 Effect；同 Key/不同 Payload 返回 409 `IDEMPOTENCY_CONFLICT`。 | FROZEN |
| TD-FRZ-506 | Unknown Network Result 永远 Query Original Resource/Business ID 后再恢复；Frontend/HTTP Client 不生成新 Key 自动重放金融/游戏/Poker/Admin Mutation。 | FROZEN |
| TD-FRZ-507 | 所有外部 JSON ID 均序列化为 String；Chaldea UUID 使用 canonical lowercase，屏蔽 NewAPI 内部 ID DB Type。 | FROZEN |
| TD-FRZ-508 | Asset/Wager/Payout/Stack/Pot 使用 decimal integer String `*_units`；精确 Decimal/RTP/Multiplier 需要时也用 String；普通计数/版本可用安全整数。 | FROZEN |
| TD-FRZ-509 | Instant 使用 RFC3339 UTC，业务自然日期使用 `YYYY-MM-DD`，Asia/Shanghai Period 明确返回 Period Boundary/Timezone；Enum 统一 `SCREAMING_SNAKE_CASE`。 | FROZEN |
| TD-FRZ-510 | List Contract 固定 Page Pagination 与 Cursor Pagination 两种 Profile；Chronological Feed 使用稳定 Keyset Cursor，Sort/Filter 均为 Code Allowlist。 | FROZEN |
| TD-FRZ-511 | Chaldea BFF JSON 默认 `Cache-Control: no-store`；未来 Session-independent Public Cache 必须使用单独明确 Cache-safe Contract。 | FROZEN |
| TD-FRZ-512 | `GET /api/v1/session/bootstrap` 同时服务 Anonymous/Auth Session Bootstrap 并提供 CSRF / Account / Master / Migration / Resource / Ops Safe Context；Protected Data 在 Gate 完成前不返回。 | FROZEN |
| TD-FRZ-513 | Auth API 冻结 Password Login/Logout/Fresh Password 与四个独立 Discord OAuth Start Purpose；共享 Callback 只信 Server-side OAuth Flow State。 | FROZEN |
| TD-FRZ-514 | Master/Account API 冻结 Profile Get/Initialize/Patch、Migration Notice Get/Acknowledge、Account Security 与 Set/Change/Reset Password Contract；底层 Password 能力继续 NewAPI Source Verify。 | FROZEN |
| TD-FRZ-515 | Public/App Composite API 允许 `/home /dashboard /me /entertainment` Read Projection，但 Composite 永远不隐式执行 Claim/Exchange/Game/Poker 副作用。 | FROZEN |
| TD-FRZ-516 | Model/API Access BFF 冻结 Models List/Detail 与 API Access Read Contract；API Access 只展示 External API 使用信息，不提供在线模型调用。 | FROZEN |
| TD-FRZ-517 | API Keys BFF 冻结 List/Create/Purpose/Disable/Enable/Delete；Native Key Capability 继续 NewAPI Source Verify，Usage Purpose 属于 Chaldea Metadata。 | FROZEN |
| TD-FRZ-518 | API Key Reveal 只有真实 NewAPI 能力存在时才开放；One-time Secret Create Response 丢失时 Chaldea 不保存第二份 Secret，需安全提示并由用户显式重建 Key。 | FROZEN |
| TD-FRZ-519 | API Usage 冻结 Summary/Requests/Request Detail Contract；RP Attribution 只使用已冻结 Safe Metadata，不为排行榜新增 Prompt/Response 采集。 | FROZEN |
| TD-FRZ-520 | Wallet BFF 冻结 Wallet/Transactions/Exchange Create/Transfer Query；不提供用户 Manual Active Quota Top-up Endpoint。 | FROZEN |
| TD-FRZ-521 | Rewards BFF 冻结 Reward Status/Claim/Create Claim Query；Client 永不提交 Reward Amount/Asset，Server 只读取 Active Validated Policy，UNRESOLVED Kind 不可 Production Active。 | FROZEN |
| TD-FRZ-522 | Game BFF 冻结 Catalog/Bootstrap/Create Round/Active Round/Canonical Round/Action/Client Seed/Fairness Paths；存在 Nonterminal Round 时 Bootstrap Resume-first。 | FROZEN |
| TD-FRZ-523 | Blackjack 继续使用通用 `game-rounds/{round_id}/actions` + Action ID + Expected Round Version，不新增客户端牌局计算 Endpoint。 | FROZEN |
| TD-FRZ-524 | Poker HTTP BFF 冻结 Lobby/Table/Access/Reservation/Buy-in/Active Session/Top-up/Safe Leave/Take Over/Connect Ticket/Fairness 与 Typed Host Command Family。 | FROZEN |
| TD-FRZ-525 | Poker WS 固定 `/ws/poker` + `chaldea-poker.v1`，使用 Versioned Client/Server Envelope；Breaking Protocol Change 使用新 Subprotocol Major。 | FROZEN |
| TD-FRZ-526 | Poker V1 Client WS Message 固定 Auth/Sync/Hand Action/Sit-out/Resume/Next Client Seed/Chat/Ping Family；Host/Ops Durable Commands 留在 HTTP Control Contract。 | FROZEN |
| TD-FRZ-527 | Poker V1 Server WS Event 固定 Auth/Snapshot/Table/Seat/Session/Hand/Timer/Control/Chat/Service/Error Family，并携带 Event/Table/Hand Version。 | FROZEN |
| TD-FRZ-528 | Poker WS Payload 永远由 Server Viewer-specific Projection 生成；Browser 永远不接收无权 Hole Cards/Future Deck 后再靠 UI 隐藏。 | FROZEN |
| TD-FRZ-529 | Rankings API 冻结 Public Current Ranking + Historical Snapshot Contract，Period/Metric/Model Filter 使用 TD-09 Aggregate Authority。 | FROZEN |
| TD-FRZ-530 | History API 冻结 Unified List + Round/Session/Hand Detail，Detail 始终读取 Durable Domain Source，权限与 Poker Reveal Boundary 不被 History Index 绕过。 | FROZEN |
| TD-FRZ-531 | Announcements API 冻结 List/Detail/Current Entry Popup/Current Post-login Popup/Read Revision Contract；Anonymous Popup Dismissal 继续 Browser-local。 | FROZEN |
| TD-FRZ-532 | Operations Read API 冻结 Bootstrap/Search/Attention/Incident/Support/Health/Jobs/Maintenance/Audit/Admin + 各业务模块 Read Family，并始终 Server RBAC Filter。 | FROZEN |
| TD-FRZ-533 | Operations Level 2/3 Write 统一通过 Durable `/ops/operations` Prepare + `/{operation_id}/execute` Command Contract，Server 根据 Code-owned Operation Type 决定 Risk/Permission/Impact/Challenge。 | FROZEN |
| TD-FRZ-534 | Level 1 Admin Write 可复用同一 Operation Engine 并由 Server 原子 Prepare+Execute；所有 Ops Write 都获得 Operation ID/Audit，不存在任意 State PATCH 后门。 | FROZEN |
| TD-FRZ-535 | Resource Concurrent Mutation 使用 `expected_version`；Mismatch 返回 409 `STALE_RESOURCE_VERSION` + Safe Authoritative State，Client 不自行 Merge 正式状态。 | FROZEN |
| TD-FRZ-536 | Server Error Contract 可返回 Safe `retry_after_seconds / eligible_at / current_state / next_actions`；429/503 在已知时使用标准 `Retry-After`，不公开内部限流阈值。 | FROZEN |
| TD-FRZ-537 | 机器可读 Contract 建立 `OpenAPI chaldea-bff-v1` + 独立 Poker WS JSON Schema/等价 Schema，作为 Implementation Spec 权威接口文件。 | FROZEN |
| TD-FRZ-538 | OpenAPI/WS Contract 生成 Frontend TypeScript DTO/Client/Enums；CI 检查 Schema Validity、Generated Client Freshness 与 Breaking Change。 | FROZEN |
| TD-FRZ-539 | Contract Example/Fixture 永远使用 Demo Data，不得把 Production Account/API Secret/Prompt/Poker Private Card 放进 OpenAPI 或 Review Package。 | FROZEN |
| TD-FRZ-540 | TD-13 正式建立 NewAPI `SV-01～SV-16` Source Verification Inventory，覆盖 Auth/Identifier/Password/Discord/API Key/Admin/Cutover/Quota/Logs/Auth Bridge/Redis/Backup/Public Route Allowlist。 | FROZEN |
| TD-FRZ-541 | NewAPI Source Verification 只决定 Adapter 实现细节，不允许静默改变 Chaldea 已冻结身份、资产、Key Purpose、权限或历史语义；若源码能力不满足必须回到 Versioned Technical Change。 | FROZEN |
| TD-FRZ-542 | FINAL Product Open Register 必须永久显式保留 Reward OPEN、Poker 5 Gaps 与 Public Record Selection Gap；Implementation 不得使用假 Default 填补。 | FROZEN |
| TD-FRZ-543 | Poker 五项 Product Gap 在全部确认并形成正式 Ruleset Version 前阻止 Poker Ruleset 标记 PRODUCTION_READY；架构代码可以实现，但生产规则不可猜。 | FROZEN |
| TD-FRZ-544 | Recent Wins Selection 未确认时 `PUBLIC_RECORD_SELECTION_POLICY=UNRESOLVED`，模块隐藏；不得为填首页生成伪中奖数据。 | FROZEN |
| TD-FRZ-545 | FINAL 另设 Implementation-only Config Register，将 Watermark/Rate Limit/Retry/Backoff/Resource Cap/Bundle Budget 等与 Product OPEN 严格分离。 | FROZEN |
| TD-FRZ-546 | TD-01～12 State Machine Cross-audit 通过：主要 Domain 无互相冲突的 Terminal Definition；Settlement/Refund 继续严格互斥。 | FROZEN |
| TD-FRZ-547 | Economy/Reward/Game/Poker/Admin/Jobs Exactly-once Cross-audit 统一为 At-least-once Delivery + Idempotent Durable Effect，不声明 Network Exactly-once。 | FROZEN |
| TD-FRZ-548 | Schema Ownership Cross-audit 通过：NewAPI 与 Chaldea 两逻辑 DB Authority 不变，无 Cross-DB FK/Trigger/2PC 偷渡，Redis 不成为正式 Ledger/Poker Authority。 | FROZEN |
| TD-FRZ-549 | Auth/Security/Privacy Cross-audit 通过：第二密码库不存在，NewAPI/Chaldea Admin 不互通，Secret/Prompt/Unrevealed Poker Data 不通过 Frontend/Logs/Audit/Ops 泄露。 | FROZEN |
| TD-FRZ-550 | IA Route Coverage Audit 通过：IA FINAL 全部 Canonical Routes 均有 Frontend Family/BFF Contract；Migration Notice/Popup/Critical Notice 继续保持 Conditional Surface 而非伪一级 Route。 | FROZEN |
| TD-FRZ-551 | Art/Deployment/DR Cross-audit 通过：Design Token/Media/Rights/A11y/Performance Gate 与 Immutable Release/PITR/Off-host Backup/DR Recovery Lock 均已获得技术覆盖。 | FROZEN |
| TD-FRZ-552 | TD-13 通过后 Technical Design v0.5 可整理为 FINAL；FINAL 明确包含 Frozen Decisions、Product Open Register、Source Verification Register、Implementation Config Register 与 Production Readiness Gate，然后才进入 Implementation Spec v1.0。 | FROZEN |

---

# 967. FINAL Change Log

## Technical Design v0.5 FINAL

- TD-01 ～ TD-13 全部通过并冻结；
- 冻结编号连续至 `TD-FRZ-552`；
- 未发现需要 SUPERSEDED 的既有技术决定；
- 补齐 Initial Super Admin Deployment-only Bootstrap；
- 完成 Same-origin `/api/v1/*` BFF Contract；
- 完成 Common Success / Error / Status / Serialization / Pagination Contract；
- 完成 Auth / Master / Account API；
- 完成 Models / API Key / API Usage API；
- 完成 Wallet / Rewards API；
- 完成 Games / Blackjack API；
- 完成 Poker HTTP / WS Final Protocol；
- 完成 Rankings / History / Announcements API；
- 完成 Operations Command Contract；
- 完成 OpenAPI / Poker WS Schema Contract；
- 建立 `SV-01 ～ SV-16` NewAPI Source Verification Register；
- 建立 FINAL Product OPEN Register；
- 建立 Implementation-only Config Register；
- 建立 Production Verification Register；
- 完成 State Machine / Exactly-once / Schema / Auth / Security / IA / Art / DR Cross-audit；
- Technical Design 状态从 WORKING 正式升级为 FINAL。

---

# 968. FINAL Status

```text
Chaldea Platform Technical Design v0.5
Status:
FINAL / TECHNICAL DESIGN COMPLETE

Frozen Decisions:
TD-FRZ-001 ～ TD-FRZ-552

Frozen Batches:
TD-01 ～ TD-13

Existing Decisions Superseded:
None
```

## Product OPEN

```text
Reward OPEN fields
Poker Product Gaps 01～05
Public Recent Wins / Featured Records Selection Policy
```

## Source Verification

```text
NewAPI SV-01 ～ SV-16
Deployment Environment Verification
```

## Implementation-only Configuration

```text
Watermarks
Rate Limits
Retry / Backoff
Resource Caps
Timeouts
Bundle Regression Budget
other measured runtime settings
```

## Production Readiness

```text
NewAPI source verification
VPS deployment verification
Rights / Asset Production Gate
Accessibility Gate
Load / Performance Gate
Security / Supply-chain Gate
Backup / Restore Drill
DR Verification
```

---

# 969. Next Stage

Technical Design 阶段至此正式结束。

下一阶段为：

> **Chaldea Platform — Implementation Spec v1.0**

Implementation Spec 必须以本 FINAL 为技术权威，并与：

```text
Requirements v0.2.11
IA v0.3.1 FINAL
Art Direction v0.4 FINAL
Technical Design v0.5 FINAL
```

保持追溯关系。

不得重新打开已冻结 Technical Decision，除非通过新的版本化 Technical Change Proposal。

