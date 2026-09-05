# Chaldea Platform 全站页面结构设计文档 v0.3.1

> **历史参考归档（已脱敏）**：本文件的 FINAL / FROZEN 是历史状态，不代表当前实现或当前强制流程；现行决策优先。见[归档索引](../README.md)与[决策 0001](../../../decisions/0001-pragmatic-baseline.md)。`examples/` 路径仅为说明占位，相关部署、图片和私有文件不随仓库提供。

> 状态：FINAL / 正式页面结构基线（奖励数值修订）  
> 上游基线：Chaldea Platform 需求基线文档 v0.2.11  
> 当前阶段：Information Architecture / Sitemap / UX Flow — FINAL  
> 当前已确认范围：IA-01～IA-13 全部完成；涵盖 Sitemap、Navigation、Dashboard、Models & API、Assets / Wallet / Rewards、Extensible Entertainment、V1 Direct Play、Poker、Rankings / History、Announcements、Master / Account / Onboarding、Chaldea Operations、Public Home、Login / Discord Registration、统一 Access Gate 与全站 UX Flow；v0.3.1 进一步冻结初始赠金 1,000、Daily 500、Hourly 100、Relief 300 及迁移用户等额初始赠金 UX。
> 原则：本文只记录已经完成讨论并得到确认的页面架构设计。任何改变已冻结路由、页面职责、权限、资产语义或 UX Flow 的内容，必须通过版本化变更重新确认。  
> 视觉色板、字体、角色素材、背景图、动画、FGO 具体视觉映射等内容不属于本阶段，将在后续 Art Direction v0.4 中设计。

---

# 1. 文档目标

本阶段负责将已经确认的 Chaldea Platform 产品需求转换为完整的信息架构和用户流程。

本阶段重点设计：

- 全站 Sitemap；
- Public Home；
- Login / Discord Registration；
- 统一 Access Gate、Return-to-Intent、错误与空状态；
- 顶层导航；
- 未登录 / 已登录信息架构；
- Dashboard；
- 模型广场；
- API Key；
- API Usage；
- API Access；
- Wallet；
- Rewards Center（每日签到、每小时签到奖励、救济金）；
- Entertainment Hub；
- Game Catalog；
- 可扩展 Game Entry / Game Shell；
- V1 首发直接游玩游戏页面；
- Poker Lobby；
- Poker Table；
- Rankings；
- Game History；
- Announcements / Events；
- Master Profile；
- Account & Security；
- Chaldea Operations；
- PC / Tablet / Mobile 导航差异；
- 页面之间的完整 UX Flow。

本阶段不负责：

- 编写实际业务代码；
- 深入数据库实现；
- 深入 API 实现；
- 确定具体前端技术实现；
- 确定最终色板；
- 确定字体；
- 确定 FGO 角色素材；
- 确定具体背景；
- 确定页面级视觉稿；
- 确定具体动画表现。

---

# 2. 已冻结的信息架构原则

## 2.1 登录后的主要入口

Dashboard 为已登录用户的主要 Home / Command Center。

Public Home 与 Dashboard 是两个不同概念：

- `/` = 平台公共首页；
- `/dashboard` = 登录后的个人 Command Center。

用户登录以后主要通过 Dashboard 开始个人操作，但登录用户仍可正常访问 Public Home。

不强制将已登录用户访问 `/` 时自动重定向至 Dashboard。

---

## 2.2 普通用户 PC 导航模式

PC 普通用户前台采用：

**Global Header + 产品域 Context Navigation**

不采用传统后台系统式的“全站永久左侧 Sidebar”作为主要导航结构。

Global Header 负责产品域之间的切换。

进入具体产品域后，由 Context Navigation 负责域内部页面切换。

Chaldea Operations 运营后台不受此原则限制，可以采用传统 Sidebar 管理后台布局。

---

## 2.3 手机端主导航

登录后的手机端采用 Bottom Navigation。

当前冻结五个一级入口：

1. 首页
2. 模型
3. 娱乐
4. 资产
5. 我的

对应关系：

### 首页
进入 Dashboard。

### 模型
进入 Model Square。

### 娱乐
进入 Entertainment Hub。

### 资产
进入 Wallet，并提供 Rewards 入口。

### 我的
进入 Personal Hub，继续访问：

- API Keys；
- API Usage；
- RP Rankings；
- Announcements；
- Master Profile；
- Account & Security；
- 其他个人低频功能。

手机端不简单使用 PC Header 缩小后的 Hamburger Menu 作为唯一导航方式。

---

## 2.4 Poker Table 沉浸模式

Poker Lobby 属于普通 Chaldea Platform 页面结构。

进入实际 Poker Table 后切换到独立的沉浸式布局。

Poker Table 中隐藏：

- 普通 Global Header；
- Entertainment Context Navigation；
- 手机 Bottom Navigation。

仅保留必要的全局牌桌控件：

- 返回大厅；
- 当前娱乐钱包 / 牌桌筹码；
- 网络连接状态；
- 设置；
- 退出牌桌。

实际座位、Pot、下注操作区、聊天区、观战、重连与 Safe Leave 等规则已经在 IA-08 中完成冻结；具体视觉表现仍留到 Art Direction v0.4。

---

## 2.5 Wallet 与 Rewards

Wallet 与 Rewards 同属于 Assets 产品域，但保持为两个独立页面。

Wallet 负责：

- API 总额度；
- 娱乐筹码；
- API 额度 ↔ 娱乐筹码兑换；
- 资产流水；
- 交易记录。

Rewards Center 负责周期奖励与救济体验，包括：

- 每日签到；
- 每小时签到奖励；
- 救济金；
- 奖励状态与领取历史。

Rewards Center 不降级为 Wallet 页面中的一个普通按钮。

---

## 2.6 Master Identity、Account Identity 与 Authentication

平台必须区分三层身份：

```text
Master Identity
用户在 Chaldea 中公开展示的昵称和头像

Account Identity
NewAPI 中稳定的账号、登录标识与账号状态

Authentication
Discord OAuth 与密码
```

Master Profile 与 Account & Security 保持为两个独立页面：

- `/master-profile` 负责 Master 昵称、展示头像和公开身份预览；
- `/account/security` 负责只读账号身份、Discord Connection、Password 与 Account Status。

Master 昵称不得作为密码登录标识。Profile 修改不改变 NewAPI 用户名、Discord 绑定、API Key、Wallet 或历史归属。

Chaldea 不保存第二份密码，实际密码体系继续由 NewAPI 负责。

---

## 2.7 V1 不建立公开用户主页

V1 不提供：

`/masters/:user_id`

一类可浏览的公开 Master Profile 页面。

排行榜、大额中奖记录等公共区域可以展示：

- Master 昵称；
- Master 展示头像。

但昵称和头像本身不产生公开个人主页跳转。

用户只能查看和编辑自己的 Master Profile。

---

## 2.8 首次进入 Chaldea

认证后统一使用以下顺序：

```text
Authentication
→ Account Status Gate
→ Master Initialization（如需要）
→ Migration Notice（如需要）
→ Permission / Resource Availability Check
→ 合法 Return-to-Intent
→ 或 Dashboard / Safe Parent
→ Deferred Post-login Popup（安全页面）
```

Master Initialization 同时适用于：

- 新 Discord 注册用户；
- 已存在并迁移至 Chaldea Platform 的 NewAPI 用户。

该过程只是 Chaldea Master Identity 初始化，不属于重新注册。不得要求已有用户重新注册、重新绑定 Discord、重新设置原密码或重新创建 API Key。

合法 Return-to-Intent 的优先级高于默认 Dashboard，但只恢复站内页面，不自动重放副作用操作。

---

## 2.9 可扩展游戏架构

V1 首发游戏清单与平台长期游戏容量分离。

平台不得将游戏目录、Context Navigation、历史筛选、Dashboard Recently Played 或 Chaldea Operations Games Sidebar 写死为固定五款游戏。

Entertainment Hub 负责运营推荐与重点入口；独立 Game Catalog 负责完整游戏目录。

直接游玩游戏采用稳定：

`/games/:game_slug`

入口。大厅型游戏可以拥有独立 Lobby / Table 路径。

Game Registry 动态提供游戏元数据、分类、标签、进入类型、发布状态、运行状态、排序与推荐。

新增完整游戏仍需代码、服务端权威结算、钱包集成、必要的 Provably Fair、前端交互和测试，不能只通过后台表单生成。

## 2.10 既有用户迁移余额清零

正式 Cutover 时：

- 既有账号、Discord 绑定、密码和 API Key 保留；
- 既有用户 Active NewAPI Quota 统一清零；
- Reserve API Credit、Entertainment Wallet 与 Poker In Play 先初始化为 0；
- 清零校验通过后，通过迁移批次向每个既有用户幂等发放 **1,000 API Credit 初始赠金**；
- 发放完成且无其他账变时，迁移用户 `API Credit = 1,000`、`Available Chips = 0`、`Poker In Play = 0`、`Total Assets = 1,000`；
- 既有 API Key 初始 Usage Purpose 为 `Unclassified`；
- 历史 API Usage 继续保留，但不回溯进入 RP Rankings；
- 迁移初始赠金与新用户赠金等额，但使用独立迁移业务 ID，不表示重新注册。

迁移用户完成必要的 Master Initialization 后，进入独立、版本化 Migration Notice。Notice 明确“旧额度已清零、1,000 API Credit 初始赠金已按迁移批次发放、账号与 Key 未删除”。用户通过 `我已了解，继续` 确认，服务端保存确认版本与时间。确认后恢复合法 Return-to-Intent；没有有效目标时进入 Dashboard。Migration Notice 不是普通公告，也不提供恢复旧额度操作。

---

## 2.11 Announcements Entry Popup 与致谢名单

Public Home 与 Login 的既有路由保持不变：

```text
/       = Public Home
/login  = Login
```

未登录用户首次进入 `/` 或 `/login` 时，平台可以按照当前有效 Placement 显示非阻断式 Entry Popup。

普通 Entry Popup 不强制打断 `/models/:model`、`/rankings`、`/announcements/:id` 等具有明确访问意图的公开 Deep Link。

Pinned、Entry Popup、Post-login Popup、Public Home Banner 与 Dashboard Summary 彼此独立配置。多条公告可以置顶，但同一时点最多启用一条 Entry Popup。

Acknowledgements / 致谢名单属于公告类型，使用标准 Announcement Detail，不建立新的一级页面。其默认行为为公开、长期置顶、入口弹出，不启用 Post-login Popup 或 Home Banner；Dashboard Summary 作为独立可选 Placement。

具体内容视觉、FGO 装饰与弹窗转场仍留到 Art Direction v0.4。

---

## 2.12 Chaldea Operations、双后台与 RBAC

平台继续采用 NewAPI Admin + Chaldea Operations 双后台。

NewAPI Admin 管理 NewAPI 原生 Users、Channels、模型路由、倍率与计费；Chaldea Operations 管理 Chaldea 业务域、运营、资产对账、Poker 运维、内容、身份支持、Access Control 与 Audit。

两套管理员权限不自动互通。

Chaldea Operations V1 使用：

- Super Admin；
- Operator + Module Scope；
- Auditor。

普通用户前台继续使用 Global Header + Context Navigation；Chaldea Operations 使用独立 Sidebar。后台不提供 Shell、SQL Console 或 Redis Console。

## 2.13 Public Home、Authentication 与全站 UX Final

`/` 保持 Public Home，`/dashboard` 保持登录后 Command Center。登录用户访问 `/` 不强制跳转 Dashboard。

`/login` 仅用于已有账号登录；`/register` 为 Discord 首次注册资格说明与 OAuth 启动页。

认证后使用统一 Gate：

```text
Authentication
→ Account Status
→ Master Initialization（如需要）
→ Migration Notice（如需要）
→ Role / Scope
→ Resource Availability
→ Return-to-Intent
→ 或 Dashboard / Safe Parent
→ Deferred Post-login Popup
```

Return-to-Intent 只恢复安全站内页面，不自动重放副作用操作。全站统一错误、Loading、Processing、Empty State 与响应式页面家族规则在 IA-13 完成冻结。

---

# 3. Sitemap IA-01

当前确认的全站信息架构如下：

```text
Chaldea Platform
│
├── 01 Public / 公共层
│   ├── Home
│   ├── Models
│   │   └── Model Detail
│   ├── Entertainment Hub
│   ├── Game Catalog
│   ├── Rankings
│   ├── Announcements
│   │   └── Announcement Detail
│   ├── Login
│   ├── Discord Registration
│   └── Conditional Surfaces
│       ├── Critical Notice
│       ├── Restricted Account
│       └── Global Error States
│
├── 02 Onboarding
│   ├── Master Initialization
│   └── Migration Notice（条件式 Interstitial）
│
├── 03 Command Center
│   └── Dashboard
│
├── 04 Models & API
│   ├── Model Square
│   │   └── Model Detail
│   ├── API Keys
│   ├── API Usage
│   └── API Access
│
├── 05 Assets
│   ├── Wallet
│   │   ├── Overview
│   │   ├── Exchange
│   │   └── Transactions
│   └── Rewards Center
│       ├── Daily Check-in
│       ├── Hourly Reward
│       └── Relief Fund
│
├── 06 Entertainment
│   ├── Entertainment Hub
│   ├── Game Catalog
│   │   └── Game Entry /games/:game_slug
│   ├── Lobby-based Games
│   │   └── Poker
│   │       ├── Poker Lobby
│   │       └── Poker Table
│   ├── Rankings
│   └── Game History
│       ├── All Records
│       ├── Mode Filter
│       ├── Game Filter
│       ├── Round Detail
│       ├── Session Detail
│       └── Hand Detail
│
├── 07 Information
│   └── Announcements & Events
│       ├── Announcement List
│       ├── Announcement Detail
│       └── Delivery Surfaces（非独立路由）
│           ├── Entry Popup
│           ├── Post-login Popup
│           ├── Public Home Banner
│           └── Dashboard Summary
│
├── 08 Master & Account
│   ├── Personal Hub / My
│   ├── Master Profile
│   └── Account & Security
│
└── 09 Administration
    ├── Chaldea Operations
    │   ├── Overview / Needs Attention
    │   ├── Models
    │   ├── Users & Identity
    │   ├── Games
    │   ├── Poker
    │   ├── Economy
    │   ├── Rewards
    │   ├── Rankings
    │   ├── Records
    │   ├── Announcements & Events
    │   ├── Operations
    │   ├── Access Control
    │   └── Audit
    └── NewAPI Admin
```

---

# 4. Public 与 Auth 页面复用原则

以下页面采用同一页面、不同登录状态增强的设计，不开发 Public 与 Auth 两套重复页面：

- Public Home；
- Model Square；
- Model Detail；
- Entertainment Hub；
- Game Catalog；
- Rankings；
- Announcements；
- Announcement Detail。

例如：

`/models`

未登录用户可以查看公开模型信息。

登录用户进入同一 `/models` 页面后，可以额外看到与自身账号相关的功能入口。

不得设计：

`/public/models`

与：

`/app/models`

两套重复页面体系。

---

# 5. 页面权限表

| 页面 | 逻辑路径 | 未登录 | 已登录 |
|---|---|---:|---:|
| Public Home | `/` | ✓ | ✓ |
| Model Square | `/models` | ✓ | ✓ |
| Model Detail | `/models/:model` | ✓ | ✓ |
| Entertainment Hub | `/entertainment` | ✓ | ✓ |
| Game Catalog | `/games` | ✓ | ✓ |
| Rankings | `/rankings` | ✓ | ✓ |
| Announcements | `/announcements` | 按公告公开配置 | ✓ |
| Announcement Detail | `/announcements/:id` | 按公告公开配置 | ✓ |
| Login | `/login` | ✓ | 进入认证后 Gate |
| Discord Registration | `/register` | ✓ | 不允许创建第二账号，进入认证后 Gate |
| Restricted Account | 条件式 Full-page State | — | 受限账号 |
| Global Error States | 条件式 | 按资源 | 按资源 / 权限 |
| Master Initialization | `/onboarding/master` | — | 首次进入 |
| Migration Notice | 条件式 Interstitial | — | 迁移用户首次确认 |
| Dashboard | `/dashboard` | — | ✓ |
| API Keys | `/api/keys` | — | ✓ |
| API Usage | `/api/usage` | — | ✓ |
| API Access | `/api/access` | — | ✓ |
| Wallet | `/wallet` | — | ✓ |
| Rewards Center | `/rewards` | — | ✓ |
| Game Entry | `/games/:game_slug` | — | ✓ |
| Poker Lobby | `/poker` | — | ✓ |
| Poker Table | `/poker/table/:id` | — | ✓ |
| Game History | `/history` | — | ✓ |
| Round Detail | `/history/round/:id` | — | ✓ |
| Session Detail | `/history/session/:id` | — | ✓ |
| Hand Detail | `/history/hand/:id` | — | ✓ |
| Personal Hub | `/me` | — | ✓ |
| Master Profile | `/master-profile` | — | ✓ |
| Account & Security | `/account/security` | — | ✓ |
| Chaldea Operations | `/ops` | — | Chaldea Admin |
| Chaldea Operations Subpages | `/ops/*` | — | 按 Role / Module Scope |
| NewAPI Admin | 独立入口 | — | NewAPI Admin |

---

# 6. Models & API 信息架构

Models & API 产品域包含：

```text
Models & API
│
├── Models
│   ├── Model Square
│   └── Model Detail
│
├── API Keys
│
├── API Usage
│
└── API Access
```

Model Square 与 Model Detail 为公共页面。

API Keys、API Usage、API Access 仅登录后访问。

---

## 6.1 API Usage

API Usage 当前作为一个完整页面存在。

内部可以包含：

- Usage Overview；
- Request History。

暂不将 Usage Overview 与 Request History 拆为两个一级页面。

具体使用 Tab、Section 或其他方式，在 API Usage 页面设计阶段继续确定。

---

## 6.2 API Access

API Access 为轻量 API 使用信息页。

其职责范围为：

- Base URL；
- 常用 Endpoint；
- Model ID；
- 基础 cURL 示例；
- 一键复制。

V1 不将其发展为：

- 大型文档站；
- 在线 Playground；
- 完整客户端教程中心。

---

# 7. Assets 信息架构

```text
Assets
│
├── Wallet
│   ├── Overview
│   ├── Exchange
│   └── Transactions
│
└── Rewards Center
    ├── Daily Check-in
    ├── Hourly Reward
    └── Relief Fund
```

Wallet 与 Rewards Center 均为登录后主页面：

```text
/wallet
/rewards
```

Overview、Exchange、Transactions 属于 Wallet 内部功能视图，不视为三个全局一级页面。

Daily Check-in、Hourly Reward、Relief Fund 属于 Rewards Center 内部 Local Navigation，不分别建立三个全局一级页面。

最终采用 Tab、Section、Drawer、Bottom Sheet 或其他表现方式，在对应页面设计阶段确定。

---

# 8. Entertainment 信息架构

```text
Entertainment
│
├── Entertainment Hub
│
├── Game Catalog
│   └── Game Entry /games/:game_slug
│
├── Lobby-based Games
│   └── Poker
│       ├── Poker Lobby
│       └── Poker Table
│
├── Rankings
│
└── Game History
```

Entertainment Hub 是娱乐域的运营型首页，负责推荐、活动、Active / Resume、Continue Playing 和重点入口。

Game Catalog 是完整、动态、可扩展的游戏目录，负责全部已发布游戏的搜索、分类、筛选、状态展示和进入目标。

未登录用户可以访问 Entertainment Hub 与 Game Catalog。

实际 Game Entry、Poker Lobby 和 Poker Table 必须登录后才能进入。

V1 首发游戏清单继续有效，但不构成目录容量或前端导航数量上限。

---

# 9. 未登录进入游戏的 Return-to-Intent 原则

用户在未登录状态下从 Entertainment Hub 或 Game Catalog 点击某项实际游戏时：

`Entertainment Hub / Game Catalog`

→ `点击开始游戏`

→ `Login / Discord Registration`

→ `Authentication`

→ `Account Status Gate`

→ `Master Initialization（如需要）`

→ `Migration Notice（如需要）`

→ `Resource Availability Check`

→ `返回用户原本想进入的游戏`

不得无条件将该用户送回 Dashboard，从而丢失原始操作意图。

该原则后续同样适用于其他需要登录才能继续的深层入口。

---

# 10. 可扩展 Game Entry 原则

V1 首发的老虎机、骰子、21 点、刮刮乐和扭蛋均属于首发 Game Entry 实例，但平台不为其写死固定数量的导航或页面结构。

直接游玩游戏统一使用：

`/games/:game_slug`

其中 `game_slug` 为稳定标识，展示名称变化不得破坏路由、历史或配置关联。

不额外为每款游戏开发一套独立的公共 Game Detail 页面。

公共介绍、状态、分类、下注摘要和透明度摘要由 Game Catalog 条目承担。

实际 Game Entry 属于 Auth 区域。

Game Catalog 支持不同进入目标：

- Direct Play；
- Lobby；
- Resume；
- Maintenance；
- Coming Soon。

不是所有未来游戏都必须套用单人直接游玩页面。Poker 和其他适合房间制的多人游戏可以使用独立 Lobby / Table 架构。

---

# 11. Poker Sitemap

Poker V1 页面级结构保持简洁：

```text
Poker
│
├── Poker Lobby
└── Poker Table
```

Poker Lobby 内部完成：

- 浏览 Public Tables；
- 浏览 / 加入 Private Tables；
- Create Table；
- Join Private Table；
- 输入房间密码；
- 选择并进入牌桌。

Create Table 和 Join Private Table 当前不单独建立独立页面。

它们属于 Poker Lobby 内部短流程，可使用 Modal、Sheet、Panel 等交互形式。

其具体信息结构与 UX Flow 已在 IA-08 中完成冻结；Modal、Drawer、Sheet 的最终视觉表现留到后续页面布局与 Art Direction 阶段。

---

# 12. Rankings Center 信息架构

Rankings 始终保持一个主页面：

`/rankings`

从纯 Entertainment 子功能升级为跨产品域 Rankings Center。

一级局部导航：

```text
Assets & Games | RP Usage
```

Assets & Games 包含：

- Total Assets；
- Game Profit；
- Biggest Win；
- Total Wagered；
- Poker Profit。

Game Profit 使用：

```text
Today | This Week | All Time
```

RP Usage 包含：

```text
Calls | Errors | Credits Consumed
```

三个 RP 榜单均支持：

- Today / This Week / All Time；
- Model Filter；
- My Rank；
- 可分享的页面筛选状态。

不为每个榜单建立独立一级页面，也不将 Rankings 增加为 PC Global Header 一级入口。

完整统计口径、公开字段、移动端行为与运营后台规则在 IA-09 中冻结。

---

# 13. Game History 信息架构

Game History 使用统一入口：

`/history`

默认 All Records 主要展示：

- Direct Play Round；
- Poker Session。

不在默认列表中平铺所有 Poker Hand。

页面采用动态筛选：

```text
Record Type
Mode
Game
Time Range
Result
Status
Round / Session / Hand ID
```

Poker Hand 主要通过：

```text
Poker Session Detail
→ Hand List
→ Hand Detail
```

进入，也允许在高级 Record Type Filter 中单独查询。

## 13.1 Round Detail

Round-based Game 记录进入：

`/history/round/:id`

至少承担：

- Round 信息；
- 下注；
- 结果与净变化；
- 结算前后余额；
- 配置版本；
- Provably Fair；
- Cancelled / Refunded 信息。

## 13.2 Session Detail

Session-based Game 记录进入：

`/history/session/:id`

Poker Session 是 V1 主要实例，包含 Buy-in、Top-up、Cash Out、Realized P/L 与 Hand List。

## 13.3 Hand Detail

Hand-based Game 记录进入：

`/history/hand/:id`

Poker Hand 是 V1 主要实例。

从任何 Detail 返回列表时，需要保留原筛选条件与滚动位置。

RP API 请求不进入 Game History，继续由 `/api/usage` 承载。

---

# 14. Announcements & Events

Announcements 与 Events 属于同一个内容系统，继续使用：

```text
/announcements
/announcements/:id
```

公告类型包括：

- System；
- New Models；
- Game Events；
- Maintenance；
- Important；
- Acknowledgements。

类型用于列表筛选，不建立独立一级页面。

公告使用以下彼此独立的 Delivery Placement：

- Pinned Announcement List；
- Entry Popup；
- Post-login Popup；
- Public Home Banner；
- Dashboard Summary。

Acknowledgements 使用标准 Announcement Detail，不新增 `/sponsors` 路由。Entry Popup 是展示层，不是独立页面。

公告列表、详情、生命周期、展示版本、已读状态、致谢名单和运营流程在 IA-10 中完成冻结。

---

# 15. Personal Hub / My

`/me` 为 Mobile-first Personal Hub，不等同于 Dashboard。

页面结构：

```text
Personal Hub
│
├── Master Summary
│   ├── Avatar
│   ├── Master Nickname
│   └── Edit Profile
│
├── Master & Account
│   ├── Master Profile
│   └── Account & Security
│
├── API
│   ├── API Keys
│   ├── API Usage
│   └── RP Rankings
│
├── Activity
│   ├── Game History
│   └── Announcements
│
├── Administration
│   └── Chaldea Operations（仅管理员）
│
└── Logout
```

入口可以显示轻量状态提示：

- Unclassified API Key 数量；
- Unread Announcement 数量；
- Password Not Set；
- Account 或 Discord 需要管理员处理的状态。

Personal Hub 只聚合入口，不复制完整 API Usage、Wallet、公告、历史或账号设置。

PC 端 `/me` 不要求成为 Global Header 一级入口；PC 用户仍可以主要通过 Master Avatar Menu 进入。V1 不允许用户自定义或重新排序 Personal Hub。

---

# 16. Chaldea Operations 一级架构

IA-12 已冻结以下一级结构：

```text
Chaldea Operations
│
├── Command
│   └── Overview
│       ├── Platform Status
│       └── Needs Attention
│
├── Catalog & Community
│   ├── Models
│   ├── Users & Identity
│   ├── Games
│   ├── Poker
│   └── Announcements & Events
│
├── Economy & Data
│   ├── Economy
│   ├── Rewards
│   ├── Rankings
│   └── Records
│
└── Administration
    ├── Operations
    ├── Access Control
    ├── Audit
    └── Open NewAPI Admin ↗
```

当前模块：

- Overview / Needs Attention；
- Models；
- Users & Identity；
- Games；
- Poker；
- Economy；
- Rewards；
- Rankings；
- Records；
- Announcements & Events；
- Operations；
- Access Control；
- Audit。

Games 继续由动态 Game Registry 驱动。Models 只管理 Chaldea 前台元数据。Users & Identity 不重做 NewAPI Users。Poker 拥有独立实时运营与 Recovery 模块。

### 16.1 NewAPI Admin

NewAPI Admin 与 Chaldea Operations 保持独立。

Chaldea Operations 可以提供 `Open NewAPI Admin` 新标签页入口，但仅对真实拥有 NewAPI Admin 权限的用户显示。

V1：

- 不 iframe 嵌入完整 NewAPI Admin；
- 不在 Chaldea Operations 内重新实现完整 NewAPI Admin；
- 不重做 Channels、底层 Models、Users、倍率和真实计费体系；
- NewAPI Admin 权限与 Chaldea RBAC 不自动互通。

---

# 17. Sitemap 与 Navigation 的区别

Sitemap 描述平台完整的信息空间。

Navigation 描述用户最常用的访问路径。

因此虽然 Sitemap 中存在大量页面，普通用户顶层导航不会直接展示全部页面。

当前 PC 顶层结构方向：

```text
[Chaldea]

Dashboard
Models
Entertainment
Announcements

                 API额度 / 娱乐筹码
                 Master
```

当前手机顶层结构：

```text
[首页] [模型] [娱乐] [资产] [我的]
```

Global Header、Context Navigation、Avatar Menu 与移动端导航已经在 IA-02 完成冻结；Public Home、Login / Registration 与最终全站 Gate 在 IA-13 完成收束。

---

# 18. Sitemap IA-01 已冻结结论

以下内容已经确认：

1. Public 与 Auth 共用 Model Square。
2. Public 与 Auth 共用 Model Detail。
3. Public 与 Auth 共用 Entertainment Hub。
4. Public 与 Auth 共用 Rankings。
5. Public 与 Auth 共用 Announcements。
6. 公共页面采用登录态增强，不建立两套重复页面。
7. Dashboard 为登录后的主要 Command Center。
8. Public Home 与 Dashboard 同时存在。
9. API 功能主要包含 API Keys、API Usage、API Access。
10. API Usage 暂不拆为多个一级页面。
11. Wallet 与 Rewards Center 为独立页面。
12. Wallet 内包含 Overview、Exchange、Transactions 内部视图。
13. V1 首发直接游玩游戏使用稳定 `/games/:game_slug` 入口，但不构成平台游戏数量上限。
14. 新增公开 Game Catalog `/games`；不为每款游戏额外建立独立公共详情页。
15. Poker 页面级 Sitemap 主要为 Lobby + Table。
16. Create Table / Join Private Table 在 Lobby 内完成。
17. Poker Table 采用独立沉浸模式。
18. Rankings 为唯一跨产品域 Rankings Center，包含 Assets & Games 与 RP Usage。
19. Game History 为统一历史页，并通过动态 Mode / Game Filter 适配后续新增游戏。
20. Round-based Game 历史支持 Round Detail。
21. Session-based 与 Hand-based Game 支持 Session / Hand Detail。
22. Announcements / Events 为统一信息系统。
23. Master Profile 与 Account & Security 分离。
24. V1 不提供公开 Master 个人主页。
25. `/me` 作为 Mobile-first Personal Hub。
26. Master Initialization 为一次性 Onboarding 页面。
27. Chaldea Operations 与 NewAPI Admin 保持双后台模式。
28. Announcements 使用统一 List / Detail；Entry Popup、Post-login Popup、Home Banner 与 Dashboard Summary 为非独立路由的 Delivery Surface。
29. Acknowledgements 属于公告类型，不建立独立 Sponsor 页面。
30. Chaldea Operations 增加 Models、Users & Identity、Poker、Records、Operations 与 Access Control 一级模块。
31. Chaldea Operations 与 NewAPI Admin 权限独立，不自动互通。
32. Chaldea Operations Subpage 使用 `/ops/*` 稳定 Deep Link，并按照 Role / Module Scope 授权。
33. Public Home 使用 `/`，Dashboard 使用 `/dashboard`，登录用户访问 `/` 不强制重定向。
34. `/login` 只服务已有账号，`/register` 只服务 Discord 首次注册资格与 OAuth。
35. Critical Notice、Restricted Account、Migration Notice 与 Global Error State 为条件式状态，不建立新的普通产品域。
36. Public Home、Login / Registration 与统一 Access Gate 已在 IA-13 完成冻结。

---

# 19. IA-02 — Navigation Architecture

本节负责定义 Chaldea Platform 的全站导航架构。

本节只冻结：

* 导航层级；
* PC 登录前 / 登录后 Global Navigation；
* 产品域 Context Navigation；
* 手机 Bottom Navigation；
* Tablet 导航行为；
* 页面返回逻辑；
* Breadcrumb 使用范围；
* Deep Link；
* Return-to-Intent；
* Poker 沉浸模式下的导航语义；
* Admin 导航模式。

本节不确定具体视觉样式，例如：

* Header 高度；
* 字体；
* 图标；
* Tab 外观；
* 胶囊样式；
* 动画；
* Hover 效果；
* FGO 视觉包装。

上述内容属于后续 Art Direction / Design System 阶段。

---

# 20. 全站导航层级

普通用户前台统一使用三层导航模型：

## Level 1 — Global Navigation

负责全站主要产品域之间的切换。

例如：

* Dashboard；
* Models；
* Entertainment；
* Announcements；
* Assets 快捷入口；
* Master / Account。

## Level 2 — Context Navigation

负责当前产品域内部不同功能页面之间的切换。

例如 Models & API：

`Models | API Keys | Usage | API Access`

例如 Assets：

`Wallet | Rewards`

例如 Entertainment：

`Hub | Games | Poker | Rankings | History`

## Level 3 — Local Navigation

负责单个页面内部的：

* Tab；
* Filter；
* Sort；
* Detail；
* Back；
* 页面内部视图切换。

例如 Game History：

`Mode Filter | Game Filter | Time Filter | Result Filter`

全站不得无必要同时叠加多套重复导航系统。

普通用户前台不采用“Global Header + 永久 Sidebar + 页面 Tab”三套重复一级导航。

---

# 21. PC Global Navigation

## 21.1 未登录状态

PC 未登录状态的 Global Header 当前冻结为以下信息结构：

```text
[Chaldea Logo]

Models
Entertainment
Announcements

Login
Discord Registration
```

### Logo

点击 Chaldea Logo：

`→ Public Home /`

### Models

进入：

`/models`

### Entertainment

进入：

`/entertainment`

### Announcements

进入：

`/announcements`

### Login

进入登录流程。

### Discord Registration

进入 Discord OAuth 首次注册流程。

由于普通密码不能用于新用户直接注册，因此注册入口必须明确指向 Discord Registration，不设计容易让用户误解为普通账号密码注册的独立传统注册入口。

---

## 21.2 Rankings 不作为 PC Global Header 一级入口

Rankings 已升级为跨产品域 Rankings Center，同时承载 Assets & Games 与 RP Usage。

即使其产品归属跨域，PC Global Header 仍不单独长期增加 Rankings 一级位置，以避免顶层导航膨胀。

Rankings 可以通过以下位置进入：

* Entertainment Context Navigation；
* Entertainment Hub；
* API Usage；
* Personal Hub；
* Public Home 的排行榜内容模块；
* 其他合理 Cross-link。

---

# 22. PC 已登录 Global Navigation

已登录状态冻结以下 Global Header 信息结构：

```text
[Chaldea Logo]

Dashboard
Models
Entertainment
Announcements

Asset Summary
Master Avatar
```

各部分职责如下。

---

## 22.1 Chaldea Logo

点击：

`→ Public Home /`

登录用户仍然可以访问平台公共首页。

访问 `/` 不强制重定向至 Dashboard。

---

## 22.2 Dashboard

Dashboard 是登录用户的主要 Command Center。

进入：

`/dashboard`

---

## 22.3 Models

进入：

`/models`

登录状态下同时进入 Models & API 产品域。

---

## 22.4 Entertainment

进入：

`/entertainment`

---

## 22.5 Announcements

进入：

`/announcements`

---

## 22.6 Asset Summary

Global Header 中提供统一的资产摘要入口。

Asset Summary 至少表达：

* API 总额度；
* 娱乐筹码。

其具体视觉形式后续确定。

点击 Asset Summary：

`→ /wallet`

Asset Summary 是 Assets 产品域的全局快捷入口。

Wallet 不额外长期占用一个 PC Global Header 一级文字导航位置。

Rewards 也不作为 Global Header 一级导航。

---

# 23. Master Avatar Menu

PC 已登录用户通过 Master Avatar 打开个人菜单。

当前冻结菜单职责：

```text
Master Avatar
│
├── My / Personal Hub
├── Master Profile
├── Account & Security
├── Chaldea Operations     ← 仅管理员
└── Logout
```

Master Avatar Menu 必须保持轻量。

以下功能不要求全部重复塞入 Avatar Menu：

* API Keys；
* API Usage；
* Wallet；
* Rewards Center；
* Game History；
* Entertainment；
* Rankings。

这些功能通过各自正常产品域导航和 Personal Hub 的 Cross-link 访问。

Avatar Menu 不作为第二套完整 Sitemap。

---

# 24. 产品域 Context Navigation

## 24.1 Models & API

登录用户进入 Models & API 产品域后，使用：

```text
Models | API Keys | Usage | API Access
```

对应页面：

```text
/models
/api/keys
/api/usage
/api/access
```

未登录用户访问 `/models` 时，不展示大量不可访问的锁定 API 功能。

未登录状态保持公共 Model Square 体验。

登录后，同一 `/models` 页面增加 Models & API Context Navigation。

---

## 24.2 Assets

Assets Context Navigation：

```text
Wallet | Rewards
```

对应：

```text
/wallet
/rewards
```

Wallet 内部继续使用 Local Navigation：

```text
Overview | Exchange | Transactions
```

因此 Assets 形成：

```text
Global
Asset Summary

Context
Wallet | Rewards

Local
Overview | Exchange | Transactions
```

---

## 24.3 Entertainment

Entertainment Context Navigation：

```text
Hub | Games | Poker | Rankings | History
```

其中：

### Hub

进入：

`/entertainment`

### Games

进入完整 Game Catalog：

`/games`

Games 不再是写死五个首发游戏名称的下拉集合。

新增游戏发布后，由 Game Registry 自动进入 Game Catalog，不要求修改 Global Header 或 Context Navigation 的基本结构。

### Poker

进入 Poker Lobby：

`/poker`

Poker 在 V1 中保留独立 Context 入口，因为它具有完整多人大厅、房间、牌桌和活动 Session。

未来其他大型多人游戏是否获得独立 Context 快捷入口，需要根据实际重要性单独确认，不自动提升所有多人游戏。

### Rankings

进入：

`/rankings`

### History

进入：

`/history`

---

## 24.4 Announcements

Announcements 不建立单独的复杂 Context Navigation。

以下内容属于页面内部内容分类：

* System；
* New Models；
* Game Events；
* Maintenance；
* Important；
* Acknowledgements。

这些分类使用 Local Navigation / Filter 实现。

---

## 24.5 Master & Account

Master / Account 产品域包含：

```text
My
Master Profile
Account & Security
```

对应：

```text
/me
/master-profile
/account/security
```

PC 用户主要通过 Master Avatar Menu 进入。

该产品域不作为 PC Global Header 一级主导航。

---

# 25. 直接游玩游戏与 Poker 的 Shell 区别

## 25.1 直接游玩游戏 — Focused Game Layout

直接游玩游戏默认采用：

**Focused Game Layout**

进入 Direct Play Game Entry 后：

* 保留 Chaldea 普通用户 Shell；
* PC 保留必要 Global Navigation；
* 手机默认保留 Bottom Navigation；
* 大型 Entertainment Context Navigation 可以简化或收起；
* 页面提供明确返回 Entertainment 的方式。

直接游玩游戏不默认进入完全脱离网站导航的 Full Immersive 模式。

具体某一游戏如果后续确认需要 Lobby 或 Full Immersive，可以单独设计，不在 IA 阶段对全部新增游戏统一强制。

---

## 25.2 Poker Table — Full Immersive Layout

Poker Table 使用：

**Full Immersive Layout**

进入：

`/poker/table/:id`

以后隐藏：

* PC Global Header；
* Entertainment Context Navigation；
* 手机 Bottom Navigation。

只保留牌桌需要的必要全局控件：

* 返回大厅；
* 当前娱乐钱包 / Table Stack；
* 网络连接状态；
* 设置；
* 退出牌桌。

Poker Lobby 仍然属于正常 Chaldea 页面 Shell。

---

# 26. Mobile Navigation

## 26.1 Bottom Navigation

登录后的手机端冻结五个 Bottom Navigation 一级入口：

```text
首页 | 模型 | 娱乐 | 资产 | 我的
```

对应：

### 首页

`→ /dashboard`

### 模型

`→ /models`

### 娱乐

`→ /entertainment`

### 资产

`→ /wallet`

### 我的

`→ /me`

手机端不简单依赖 PC Header 缩小后的 Hamburger Menu 作为唯一主要导航。

---

## 26.2 Mobile Context Navigation

手机端产品域内部不使用永久 Sidebar。

使用页面顶部的紧凑 Context Navigation。

例如：

### Models & API

```text
Models | Keys | Usage | Access
```

### Assets

```text
Wallet | Rewards
```

### Entertainment

```text
Hub | Games | Poker | Rankings | History
```

具体使用：

* 横向滚动 Tab；
* Compact Tabs；
* Segmented Control；
* 其他形式

由后续页面与视觉设计阶段确定。

IA 只冻结其导航职责。

---

## 26.3 Mobile Personal Hub

`/me`

作为 Mobile-first Personal Hub。

主要聚合低频个人功能入口：

```text
Master Summary
├── Avatar / Master Nickname
└── Edit Profile

Master & Account
├── Master Profile
└── Account & Security

API
├── API Keys
├── API Usage
└── RP Rankings

Information
└── Announcements

Personal Records
└── Game History
```

Game History 在 `/me` 中属于 Cross-link。

其信息架构归属仍然是 Entertainment / Records，不因此改变产品域归属。

---

## 26.4 Mobile Direct-play Games

手机进入直接游玩游戏时，默认：

* 保留最小页面顶部栏；
* 提供返回 Entertainment；
* 保留 Bottom Navigation。

概念结构：

```text
← Entertainment       Game Name

Game Content

首页 | 模型 | 娱乐 | 资产 | 我的
```

具体某款游戏如果 Bottom Navigation 会明显干扰操作，可以在对应游戏页面设计阶段单独确认隐藏策略。

---

## 26.5 Mobile Poker Table

手机进入 Poker Table 后：

* 隐藏 Bottom Navigation；
* 隐藏普通 Global Navigation；
* 隐藏普通 Context Navigation。

手机竖屏与横屏采用同一信息架构，只改变 Poker Table 的具体排版。

---

# 27. Tablet Navigation

Tablet 不建立独立第三套导航体系。

采用响应式导航行为：

### 较宽 / 横屏 Tablet

使用 Condensed Desktop Navigation。

### 较窄 / 竖屏 Tablet

使用 Mobile Navigation Pattern。

是否采用 PC 或 Mobile Pattern 根据实际可用布局空间决定。

IA 阶段不锁死具体：

* 768px；
* 1024px；
* 或其他 Breakpoint。

具体 Breakpoint 在 Design System / Frontend Implementation 阶段确定。

---

# 28. Breadcrumb 规则

Chaldea Platform 不全站强制使用 Breadcrumb。

Breadcrumb 主要用于深层 Detail 页面。

当前建议：

| 页面                      | Breadcrumb |
| ----------------------- | ---------- |
| Public Home             | 不需要        |
| Dashboard               | 不需要        |
| Model Square            | 不需要        |
| Model Detail            | 需要         |
| API Keys                | 不需要        |
| API Usage               | 不需要        |
| Wallet                  | 不需要        |
| Rewards Center         | 不需要        |
| Entertainment Hub       | 不需要        |
| Game Catalog            | 不需要        |
| Game Entry              | 不需要        |
| Poker Lobby             | 不需要        |
| Poker Table             | 不使用        |
| Rankings                | 不需要        |
| Game History            | 不需要        |
| Round Detail            | 需要         |
| Session Detail          | 需要         |
| Hand Detail             | 需要         |
| Announcements           | 不需要        |
| Announcement Detail     | 需要         |
| Master Profile          | 不需要        |
| Account & Security      | 不需要        |
| Chaldea Operations 深层页面 | 按需使用       |

手机端深层页面优先使用：

```text
← Parent Page
Current Page
```

而不是长期展示完整多级 Breadcrumb。

---

# 29. Back Navigation

普通页面返回遵循两个原则。

## 29.1 正常逐层浏览

当用户正常从父页面进入子页面时：

Back 优先尊重实际浏览历史。

例如：

`Models → Model Detail → Back → Models`

---

## 29.2 Deep Link 直接进入

如果用户通过 Deep Link 直接进入 Detail 页面，页面自身仍必须提供明确的 Parent Back。

例如直接打开：

`/history/round/:id`

页面仍应提供：

`← Game History`

不能完全依赖浏览器 Back。

---

# 30. Deep Link

Deep Link 属于正式支持的导航方式。

Public Deep Link 包括：

- Model Detail；
- Game Catalog；
- Rankings；
- Announcement Detail。

Protected / Admin Deep Link 在认证前保存安全 Return-to-Intent，并在统一 Gate 完成后重新检查：

- Account Status；
- Master Initialization；
- Migration Notice；
- Role / Scope；
- Resource Existence；
- Publication / Maintenance 状态。

普通 Entry Popup 只在匿名用户进入 `/` 或 `/login` 时检查，不强制覆盖 `/register` 或其他公共 Deep Link。

---

# 31. Return-to-Intent

## 31.1 基本原则

Protected Route 在认证前保存安全的站内页面意图。

统一流程：

```text
Protected / Admin Target
→ Login / Discord Registration
→ Authentication
→ Account Status Gate
→ Master Initialization（如需要）
→ Migration Notice（如需要）
→ Role / Scope Permission Check
→ Resource Availability Check
→ Original Target
```

没有合法目标时进入 Dashboard；Admin 目标无权限时进入 Access Denied 或 `/ops` 安全父页面。

## 31.2 安全约束

Return-to-Intent：

- 只允许 Chaldea 站内安全路径；
- 禁止外部 URL 和危险 Scheme；
- 必须具有有效期；
- 恢复前重新校验权限与资源；
- 使用一次后清除；
- 无效、过期、下线或维护时进入安全父页面并说明原因。

普通 Post-login Popup 排在 Master Initialization、Migration Notice 与关键恢复流程之后，不遮挡 Poker Table、活动 Round 或 Wallet Processing。

## 31.3 Navigation Resume ≠ Action Replay

Return-to-Intent 只恢复页面、筛选或详情位置，不自动执行：

- API Credit / Chips Exchange；
- Reward Claim；
- 游戏下注；
- Poker Buy-in / Cash Out；
- API Key 创建 / 删除；
- Profile / Password 保存；
- Admin 写操作。

网络结果不确定时查询原 Business ID，而不是依靠导航恢复重放请求。

---

# 32. Poker Navigation Safety

Poker Table 涉及真实娱乐筹码，因此普通页面导航规则不能直接应用。

---

## 32.1 Spectator 状态

用户未入座，仅观战时：

点击：

`返回大厅`

可以直接：

`→ Poker Lobby`

---

## 32.2 Seated 状态

用户已经 Buy-in 并拥有 `table_stack` 时：

“返回大厅”不能直接执行普通页面跳转。

其语义为：

```text
返回大厅
→ 发起 Safe Leave
→ 完成必要离桌处理
→ Cash Out
→ Seat Leave
→ Poker Lobby
```

不得出现玩家 UI 已经返回 Lobby，但真实 `table_stack` 仍处于无法理解的悬空状态。

---

## 32.3 返回大厅与退出牌桌

Poker Table 保留：

* 返回大厅；
* 退出牌桌。

两者语义不同。

### 返回大厅

表示：

**Navigation Intent**

用户的目标是回到 Poker Lobby。

### 退出牌桌

表示：

**Seat / Asset Operation**

用户明确要求离开当前牌桌。

当玩家已入座时，“返回大厅”最终也必须先安全完成对应的离桌 / Cash Out 流程，才能真正完成导航。

---

## 32.4 Browser Back

如果用户正在 Poker Table 中且已经 Buy-in：

浏览器 Back 不得被简单视为普通页面跳转。

应被视为：

**用户产生离开牌桌意图。**

必须进入 Safe Leave 语义。

如果用户只是 Spectator：

可以正常返回 Poker Lobby。

具体浏览器拦截和 WebSocket 实现不属于 IA 阶段，在后续技术设计中确定。

---

# 33. Chaldea Operations Navigation

Chaldea Operations 使用独立的管理后台 Sidebar。

Sidebar 分组：

```text
Chaldea Operations
│
├── Command
│   └── Overview
│       ├── Platform Status
│       └── Needs Attention
│
├── Catalog & Community
│   ├── Models
│   ├── Users & Identity
│   ├── Games
│   ├── Poker
│   └── Announcements & Events
│
├── Economy & Data
│   ├── Economy
│   ├── Rewards
│   ├── Rankings
│   └── Records
│
└── Administration
    ├── Operations
    ├── Access Control
    ├── Audit
    └── Open NewAPI Admin ↗
```

顶部持续显示：

- Environment；
- Global Search；
- Needs Attention；
- Return to User Site；
- Current Admin。

PC 使用持久 Sidebar；Tablet 使用可折叠 Sidebar；Mobile 使用 Drawer。

筛选、排序、分页写入 URL；详情页面使用稳定 Deep Link 和 Breadcrumb。NewAPI Admin 使用新标签页 Cross-link，不嵌入 Chaldea Operations。

权限由服务端 RBAC 校验。Sidebar 只展示当前角色可访问模块，但隐藏入口本身不构成授权。

---

# 34. IA-02 已冻结结论

以下 Navigation Architecture 内容已经确认：

1. 普通用户前台采用 Global Navigation + Context Navigation + Local Navigation 三层结构。
2. 普通用户 PC 前台不采用永久全站 Sidebar。
3. 未登录 PC Header 包含 Models、Entertainment、Announcements、Login、Discord Registration。
4. Discord Registration 明确作为首次注册入口。
5. Rankings Center 不作为 PC Global Header 独立一级入口，并通过 Entertainment、API Usage、Personal Hub 与 Public Home 提供 Cross-link。
6. 登录后 PC Header 包含 Dashboard、Models、Entertainment、Announcements、Asset Summary、Master Avatar。
7. Asset Summary 为 Wallet 的全局快捷入口。
8. Wallet 与 Rewards Center 不长期占据 PC Global Header 一级导航。
9. Master Avatar Menu 保持轻量，不复制完整 Sitemap。
10. Models & API Context Navigation = Models / API Keys / Usage / API Access。
11. Assets Context Navigation = Wallet / Rewards。
12. Entertainment Context Navigation = Hub / Games / Poker / Rankings / History。
13. Games 固定进入独立 Game Catalog `/games`，不在导航中罗列固定游戏名称。
14. Game Catalog、分类、标签和游戏条目由 Game Registry 动态生成。
15. Announcements 类型（含 Acknowledgements）属于页面内部 Local Navigation / Filter；Entry Popup、Banner 与 Dashboard Summary 是 Delivery Surface，不是导航页面。
16. Direct Play 游戏默认采用 Focused Game Layout。
17. Poker Table 采用 Full Immersive Layout。
18. 手机端使用 Bottom Navigation。
19. 手机 Bottom Navigation = 首页 / 模型 / 娱乐 / 资产 / 我的。
20. 手机产品域内部使用顶部 Context Navigation，不使用永久 Sidebar。
21. `/me` 为 Mobile-first Personal Hub。
22. Tablet 根据实际布局空间采用 Desktop 或 Mobile Pattern，不设计独立第三套导航。
23. Breadcrumb 只在深层 Detail 和部分 Admin 页面按需使用。
24. Deep Link 属于正式支持的导航方式。
25. Game Entry 使用稳定 `/games/:game_slug` Deep Link。
26. Protected Deep Link 登录后恢复原页面意图。
27. 首次用户存在合法 Return-to-Intent 时，在 Account Status、Master Initialization、Migration Notice 与 Resource Availability Gate 完成后返回原目标。
28. 无有效 Return-to-Intent 时，在全部认证后 Gate 完成后进入 Dashboard 或安全父页面。
29. Return-to-Intent 只恢复 Navigation，不自动重放产生副作用的业务操作。
30. Poker Spectator 可以直接返回 Lobby。
31. Poker 已入座玩家返回 Lobby 前必须完成 Safe Leave。
32. Poker Browser Back 在已 Buy-in 状态下同样视为离桌意图。
33. Chaldea Operations 使用适合后台管理的 Sidebar。
34. Chaldea Operations Games 使用动态 Game Registry，不写死首发游戏名称。
35. NewAPI Admin 继续作为独立管理入口。
36. Chaldea Operations Sidebar 按 Command、Catalog & Community、Economy & Data、Administration 分组。
37. 后台顶部持续显示环境、Global Search、Needs Attention 和返回用户站点入口。
38. Chaldea Operations 使用 Super Admin / Operator + Scope / Auditor RBAC。
39. 后台详情使用稳定 Deep Link，列表筛选、排序和分页写入 URL。
40. NewAPI Admin 与 Chaldea Operations 权限互相独立。

---

# 35. IA-03 — Dashboard Information Architecture

Dashboard 是已登录用户进入 Chaldea Platform 后的主要 **Command Center**。

Dashboard 的目标不是复制各个功能页面，而是在一个页面中快速回答：

1. 我当前是什么状态；
2. 我现在拥有什么资产；
3. 有什么需要立即处理的事情；
4. API 使用情况如何；
5. 可以继续进行哪些娱乐活动；
6. 最近发生了什么；
7. 平台当前有什么重要信息。

Dashboard 不等同于：

* Public Home；
* Wallet；
* API Usage；
* Entertainment Hub；
* Game History；
* Announcements；
* Chaldea Operations。

各专业功能的完整操作仍应进入对应独立页面。

---

# 36. Dashboard 产品定位

Chaldea Platform 同时包含：

* API Platform；
* Entertainment Platform。

Dashboard 必须同时服务这两条核心产品主线。

不得将 Dashboard 设计成传统 NewAPI / Developer Console 式页面，仅突出：

* API Key；
* 调用量；
* 模型；
* API 日志。

也不得将其设计成单纯娱乐平台首页，仅突出：

* 游戏；
* Poker；
* 排行榜；
* 大奖。

Dashboard 中：

**Assets 是 API 与 Entertainment 两条业务主线之间的核心桥梁。**

API 额度既用于 API 消费，也可以通过资产转换进入娱乐体系。

娱乐筹码同样可以转换回 API 额度。

因此资产状态属于 Dashboard 中最高优先级的信息之一。

---

# 37. Dashboard 信息职责

Dashboard 统一承担四种信息职责：

## 37.1 Status

告诉用户当前状态。

包括：

* Master 身份；
* 资产状态；
* 重要系统状态；
* Active Game Session（V1 主要为 Poker）等当前活动状态。

---

## 37.2 Action

提供当前最可能执行的高频行动。

例如：

* Daily Check-in；
* Hourly Reward；
* Relief Fund；
* Wallet；
* API Keys；
* Models；
* Continue Playing；
* Poker Reconnect。

---

## 37.3 Activity

展示用户近期活动摘要。

例如：

* API 使用活动；
* 已结算游戏记录；
* Poker Session 结算结果。

Dashboard Activity 仅为摘要，不代替完整记录页面。

---

## 37.4 Information

展示平台当前重要信息。

主要来自：

* Announcements；
* Events；
* Maintenance；
* New Models；
* Important Notices。

---

# 38. Dashboard 核心逻辑区域

Dashboard 当前冻结以下七个逻辑区域：

```text
Dashboard
│
├── 01 Command Status / Master Identity
├── 02 Assets & Rewards
├── 03 API Operations
├── 04 Entertainment
├── 05 Active / Resume
├── 06 Recent Activity
└── 07 Announcements & Events
```

上述内容表示信息架构中的逻辑区域。

不代表最终必须制作七张独立 Card。

具体：

* Card 数量；
* Grid；
* 左右栏；
* 卡片大小；
* Header 样式；
* FGO 视觉语言

均留到后续 Page Layout / Art Direction 阶段确定。

---

# 39. Command Status / Master Identity

Dashboard 顶部显示轻量 Master 身份信息。

当前冻结：

* Master Avatar；
* Master 昵称；
* 简单欢迎信息。

例如其信息语义可以表达：

```text
Master Avatar

Master Name

Welcome back / 简单当前状态信息
```

该区域不得演变为大型 Profile Banner。

以下信息不属于 Dashboard 顶部身份区域的核心内容：

* Discord User ID；
* NewAPI user_id；
* 注册时间；
* 完整 Account 信息；
* 完整安全设置。

这些内容继续归属于：

* Master Profile；
* Account & Security。

---

# 40. Assets & Rewards

Assets & Rewards 属于 Dashboard 中最高优先级的信息区域之一。

至少展示：

* API 总额度；
* 可用娱乐筹码；
* 必要的 Poker In Play 状态；
* Daily Check-in 状态；
* Hourly Reward 状态；
* Relief Fund 状态。

---

## 40.1 API 总额度

Dashboard 对普通用户只展示统一的：

**API Credit / API 总额度**

不得向普通用户拆分显示：

* Active API Quota；
* Reserve API Credit。

Active / Reserve 属于内部资产结构。

用户侧继续保持一个统一的 API 总额度概念。

---

## 40.2 娱乐筹码

Dashboard 显示用户当前可用娱乐筹码。

Poker 中已经进入 `table_stack` 的资产与娱乐钱包余额属于不同运行状态。

当用户存在 Active Poker Session 时，Dashboard 可以额外显示紧凑的 Poker In Play 状态，但不得因为筹码已经进入 Poker Table 而将其错误视为已经不属于用户资产。

完整资产拆分进入 Wallet。

---

## 40.3 Wallet 入口

Dashboard 提供进入 Wallet 的入口。

资产兑换的完整流程继续在：

`/wallet`

中完成。

Dashboard 不直接提供完整 API Credit ↔ Entertainment Chips 兑换表单。

---

## 40.4 Rewards Center 入口

Dashboard 提供进入：

`/rewards`

的入口。

Dashboard 负责奖励状态摘要与符合条件时的快速领取；Rewards Center 负责完整规则、冷却状态、领取历史和维护状态。

---

# 41. Dashboard Rewards

Dashboard 中的 Rewards 区域同时覆盖：

```text
Daily Check-in
Hourly Reward
Relief Fund
```

三个奖励入口共用统一的服务端资格判断、正式领取记录、幂等保护和资产到账逻辑。

Dashboard 只展示服务端返回的可领取状态，不根据设备本地时间或前端余额自行推断资格。

---

## 41.1 Daily Check-in

Daily Check-in 继续作为 Dashboard 高优先级 Daily Action。

### 未签到

Dashboard 明确显示：

`今日未签到`

并允许用户直接完成 Daily Check-in，不要求先进入 Rewards Center。

### 签到成功

Dashboard 直接反馈：

* 签到成功；
* 本次固定获得 500 API Credit；
* 更新后的 API 总额度；
* 今日状态已完成。

### 已签到

显示：

* 今日已签到；
* 本次签到奖励。

已完成后可以降低视觉优先级。

---

## 41.2 Hourly Reward

Hourly Reward 每个用户每小时最多成功领取一次，单次奖励数量固定为 **100**，不采用随机金额。

### 可领取

Dashboard 显示直接领取操作。

领取成功后显示：

* 本次奖励数量；
* 实际发放资产类型；
* 更新后的对应资产余额；
* 下一次可领取状态。

### 冷却中

Dashboard 显示下一次可领取时间或倒计时，并降低操作优先级。

### 已确认数量与待确认规则

Dashboard 可以固定展示奖励数量 **100**，但实际单位图标和资产名称必须读取服务端返回值。

以下内容仍以需求基线中的 TBD 为准，Dashboard 不得自行决定：

* 100 发放为 API 额度或娱乐筹码；
* 自然小时或滚动 60 分钟冷却；
* 是否累积；
* 是否存在每日领取上限。

---

## 41.3 Relief Fund

Relief Fund 用于让处于破产状态的玩家重新开始游戏，单次奖励数量固定为 **300**。

### 固定破产资格

破产固定定义为：

`Total Assets < 10`

Total Assets 使用 Wallet 已冻结的统一口径，包括 API Credit、Available Chips、Poker In Play 与未重复计算的 Processing Assets。

Dashboard 不得只根据 Available Chips 判断资格。

边界规则：

- 总资产低于 10：满足破产资产条件；
- 总资产等于 10：不满足；
- 总资产高于 10：不满足。

### 滚动 4 小时冷却

Relief Fund 不按固定自然时段刷新。

成功领取后：

`next_claim_at = last_successful_claim_at + 4 hours`

Dashboard 使用服务端返回的下一次可领取时间或倒计时。

- 从未成功领取过、且当前总资产低于 10：可以立即领取；
- 已领取但仍在 4 小时冷却中：即使再次破产也不能领取；
- 冷却结束但总资产为 10 或以上：仍不可领取；
- 冷却已经结束后，用户随后重新跌至总资产低于 10：立即恢复可领取；
- 失败、被拒绝或未形成成功记录的请求不开始或重置倒计时。

### Dashboard 状态

#### 可领取

同时满足：

- 服务端当前总资产低于 10；
- 从未成功领取，或滚动 4 小时冷却已经结束；
- 功能启用且非维护；
- 不存在尚未确认的重复领取。

Dashboard 显示高可见直接领取操作。

#### 破产但冷却中

显示下一次可领取时间或倒计时，并说明当前仍满足破产资产条件，但冷却尚未结束。

#### 冷却已结束但未破产

显示当前不符合资格，并明确说明：

`总资产必须严格少于 10`

不显示新的固定刷新点。

### 提交时重新校验

页面展示的资格可能因 API 消费、兑换、游戏结算或 Poker 资产变化而过期。

用户点击领取时，服务端必须重新计算总资产并重新校验滚动冷却。前端不得根据旧余额自行保证领取成功。

### 待确认规则

以下内容仍不得由 IA、视觉或实现模型自行补全：

- 固定数量 300 发放为 API 额度或娱乐筹码；
- 未领取或长期未使用的资格是否累积；
- Active Poker Session 中总资产仍低于 10 时是否允许领取。

单次数量 300、破产阈值、统一总资产口径与滚动 4 小时冷却已冻结。

## 41.4 Dashboard 直接领取原则

Dashboard 允许对服务端明确判定为可领取的奖励直接发起领取。

领取操作必须：

* 使用服务端权威状态；
* 提交后防止重复点击；
* 网络超时后先查询原领取结果；
* 不因页面刷新或重新登录自动重放；
* 成功后同步刷新资产摘要和奖励状态。

---

## 41.5 Rewards Center 独立页面继续保留

Dashboard 的快速领取不取代：

`/rewards`

Rewards Center 用于承载：

* Daily Check-in；
* Hourly Reward；
* Relief Fund；
* 当前服务器 / 业务时间；
* 下一次可领取时间；
* 资格说明；
* 奖励历史；
* 规则与维护状态。

当前需求没有确认：

* 连续签到奖励；
* 七日签到；
* 签到补签；
* 连续签到倍率。

因此不得通过页面视觉暗示上述机制已经存在。

---

# 42. API Operations

Dashboard 保留轻量 API Operations Summary。

主要承担：

```text
API Operations
│
├── API Usage Summary
├── API Keys Summary
└── Models Quick Access
```

完整功能继续进入各自独立页面。

---

## 42.1 API Usage Summary

Dashboard 显示：

**当日 API 消耗数字。**

当前冻结至少需要表达：

`Today API Usage`

以及对应消耗的 API Credit。

例如：

```text
Today API Usage

123.45 API Credit
```

具体统计还可以在后续 API Usage 页面设计中讨论：

* Request Count；
* Token Count；
* 最近调用；
* 模型分布；
* 输入 / 输出 Token。

上述附加指标当前暂不冻结。

---

## 42.2 当日统计口径

Dashboard 已确定需要显示“当日 API 消耗”。

但当前 IA 阶段不单独确定：

* API Usage 的业务日时区；
* 请求跨日归属；
* 流式请求计入时间；
* 失败请求统计方式。

这些属于后续 API Usage 产品设计与技术设计需要统一确认的内容。

不得由 Dashboard 单独制定与 Usage 页面不同的统计规则。

---

## 42.3 API Keys Summary

Dashboard 提供 API Key 状态摘要。

例如可以表达：

* 当前存在多少个有效 API Key；
* 是否尚未创建 API Key；
* Manage API Keys 入口。

Dashboard 不直接完整展示 API Key Secret。

完整查看、创建和管理进入：

`/api/keys`

---

## 42.4 API Key Empty State

如果用户尚未拥有 API Key：

Dashboard API 区域显示对应 Empty State。

例如信息语义：

```text
No API Key

Create your first API Key
```

并进入：

`/api/keys`

---

## 42.5 Models Quick Access

Dashboard 可以展示少量：

* Recommended Models；
* New Models。

其作用仅为模型快捷发现。

完整模型浏览继续进入：

`/models`

Dashboard 不复制完整 Model Square。

展示数量、排序方式和视觉形式在 Models 页面设计阶段继续确定。

---

# 43. Entertainment

Dashboard 提供 Entertainment Quick Access，但不复制 Entertainment Hub。

娱乐区域主要承担：

* Recently Played；
* Continue Playing；
* Poker 入口；
* Entertainment Hub 入口；
* Rankings 快捷入口。

---

## 43.1 Recently Played / Continue Playing

Dashboard 显示：

**Continue Playing / Recently Played**

但该区域为条件模块。

只有用户确实存在游戏历史时才显示。

---

### 有游戏历史

可以显示用户最近玩过的已注册游戏。

例如：

```text
Continue Playing

由用户历史与 Game Registry 动态返回的最近游戏
```

具体显示：

* 一个；
* 三个；
* 或其他数量

后续页面设计继续确定。

---

### 无游戏历史

不展示空的 Recently Played 列表。

可以降级为：

`Explore Games`

引导进入：

`/games`

---

## 43.2 Recently Played 不等于 Active / Resume

已经完成、退出或没有持续 Session 的游戏属于 Recently Played。

仍在持续中的可恢复 Game Session 属于：

**Active / Resume**

V1 主要实例为 Poker Session。

拥有更高优先级。

不得将两者混合成普通“最近游戏”。

---

## 43.3 Rankings

Dashboard 可以提供：

`View Rankings`

快捷入口。

V1 Dashboard 不强制展示完整排行榜，也不强制展示用户当前个人排名。

完整 Rankings 继续进入：

`/rankings`

---

## 43.4 Recent Jackpot / Big Win

大额中奖、最近大奖和公共中奖记录不作为 Dashboard 核心信息模块。

其主要展示位置优先为：

* Public Home；
* Entertainment Hub；
* Rankings。

后续如果需要在 Dashboard 增加低优先级娱乐内容，可重新讨论，但不得挤占 Dashboard 核心状态区域。

---

# 44. Active / Resume

Dashboard 支持条件式：

**Active / Resume**

模块。

其当前 V1 最高优先级实例是 Active Poker Session；未来其他可恢复游戏 Session 也可以进入该模块。

---

## 44.1 Active Game Session / Active Poker Session

如果用户仍然存在有效、可恢复的游戏 Session，Dashboard 应显示：

* 游戏名称；
* Session 状态；
* 必要的继续信息；
* Resume / Reconnect 操作。

V1 首个主要实例为 Poker Table Session。Poker 额外显示：

* Table 信息；
* 当前 Table Stack；
* 连接状态；
* Reconnect to Table。

例如信息语义：

```text
Active Poker Session

Table: XXXXX
Stack: 18,500

Reconnect to Table
```

---

## 44.2 Active Game 优先级

Active Game Session 属于 P0 状态。

其优先级高于普通：

* Recently Played；
* Entertainment 推荐；
* Rankings 快捷入口。

因为该 Session 可能包含：

* 实时游戏状态；
* 尚未完成的玩家操作；
* 仍需恢复的 Session 资产或进度。

Poker 还可能包含 Seat、table_stack 与未完成牌局。

---

## 44.3 不自动强制恢复 Session

登录后如果检测到 Active Game Session，不得默认强制进入对应游戏。

应当：

`Dashboard → Active Session Notice → User chooses Resume`

对于 Poker：

`Dashboard → Active Poker Notice → User chooses Reconnect`

用户保留主动决定是否恢复 Session 或重新进入牌桌的权利。

---

## 44.4 Poker 未实现盈亏

Active Poker Session 可以显示：

* Current Stack；
* Table 状态。

但是玩家仍在桌上时的实时筹码变化不得直接作为：

* 今日 Poker 盈利；
* 本周 Poker 盈利；
* 历史净盈利；
* Poker 盈利榜正式结果

展示。

正式 Poker 盈亏继续以完成离桌 / Cash Out 后的 Session 结算结果为准。

---

# 45. Recent Activity

Dashboard 提供统一的 Recent Activity Summary。

其作用是回答：

> 最近在 Chaldea 做了什么？

而不是提供完整业务流水。

---

## 45.1 可以包含

例如：

### API Activity

* 最近模型调用；
* 最近 API 消耗摘要。

### Game Activity

* 最近已完成游戏；
* 已结算游戏输赢。

### Poker Activity

* 已完成并结算的 Poker Session。

---

## 45.2 Recent Activity 不替代专业记录页

必须区分：

### Dashboard Recent Activity

全站近期行为摘要。

### API Usage

完整 API 调用记录。

### Wallet Transactions

完整资产账变。

### Game History

完整娱乐游戏历史。

因此 Dashboard Recent Activity 不需要展示全部技术字段或资产字段。

---

## 45.3 条件降级

如果用户完全没有 Recent Activity：

该区域可以：

* 显示轻量 Empty State；
* 降低视觉优先级；
* 或在特定布局下暂时不出现。

Dashboard 不为了填满页面强制制造无意义的空 Card。

---

# 46. Announcements & Events

Dashboard 显示 Announcements & Events 摘要。

主要展示：

* Important；
* Pinned；
* Latest；
* Acknowledgements 摘要（默认进入 Pinned Summary）。

内容可能包括：

* 系统公告；
* 新模型通知；
* 游戏活动；
* 维护通知；
* 重要提醒；
* 致谢名单。

完整内容继续进入：

`/announcements`

---

## 46.1 Entry Popup、Post-login Popup 与 Dashboard Summary

必须区分：

### Entry Popup

用于未登录用户首次进入 `/` 或 `/login` 时展示当前有效入口公告。

Entry Popup 为非阻断式，关闭后继续公共浏览或登录。Acknowledgements 默认使用该渠道。

### Post-login Popup

用于登录后的普通公告提醒，但不能直接遮挡 Poker Table、活动 Round 或 Wallet Processing。需要展示时延迟到 Dashboard 或其他安全普通页面。

同一公告展示版本已在本次 Entry Popup 中出现时，登录后不重复弹出。

### Dashboard Summary

属于 Dashboard 中持续可访问的公告摘要。Acknowledgements 可以作为 Pinned 摘要进入 Dashboard，但不强制占据大型区域。

### Announcement List / Detail

属于长期可访问的完整内容渠道，不因弹窗被关闭而消失。

Pinned、Entry Popup、Post-login Popup、Home Banner 和 Dashboard Summary 彼此独立配置。

关闭 Entry Popup 不等于将 Announcement Detail 标记为已读。

---

# 47. Dashboard 信息优先级

## 47.1 P0 — Immediate / Critical

最高优先级：

* Critical Notice；
* Active Game Session（V1 主要为 Poker）；
* API Credit；
* Entertainment Chips；
* 未完成或异常资产状态；
* Daily Check-in；
* 当前可领取的 Hourly Reward；
* 当前符合资格且可领取的 Relief Fund。

---

## 47.2 P1 — Primary Operations

主要业务：

* API Operations；
* Entertainment；
* Continue Playing。

---

## 47.3 P2 — Recent Information

次级信息：

* Recent Activity；
* Announcements & Events。

---

## 47.4 P3 — Discovery

较低优先级：

* Recommended Models；
* New Models；
* 其他发现型内容。

P3 内容不得挤压 P0 / P1 的首屏可用性。

---

# 48. PC Dashboard 信息结构原则

PC Dashboard 当前建议的信息逻辑关系为：

```text
┌──────────────────────────────────────────────┐
│ Master Identity / Command Status             │
├──────────────────────────────────────────────┤
│ API Credit │ Chips │ Rewards Status          │
├─────────────────────┬────────────────────────┤
│ API Operations      │ Entertainment          │
│                     │                        │
│ Today Usage         │ Continue Playing       │
│ API Keys            │ Poker                  │
│ Models              │ Rankings               │
├─────────────────────┴────────────────────────┤
│ Active / Resume（条件出现，优先级动态提升） │
├─────────────────────┬────────────────────────┤
│ Recent Activity     │ Announcements          │
└─────────────────────┴────────────────────────┘
```

该结构仅表达信息关系。

不冻结实际：

* 两栏比例；
* Card 数量；
* Card 高度；
* 是否跨栏；
* 模块左右位置。

具体 Layout 后续设计。

---

# 49. Mobile Dashboard

Mobile Dashboard 不采用单纯：

`Desktop Grid → 单列折叠`

作为完整设计策略。

手机端应重新按照移动使用场景排列信息优先级。

当前建议顺序：

```text
1. Critical Notice / Active Game Session
2. Master Identity
3. Assets
4. Rewards
5. API Usage Summary
6. Continue Playing / Entertainment
7. API Operations
8. Recent Activity
9. Announcements
```

实际设计阶段可根据页面高度继续优化，但必须保持：

* Active State 优先；
* Assets 优先；
* Rewards 高可见；
* 内容发现低于当前状态。

---

# 50. Mobile Dashboard 不建立重复 Quick Action Grid

由于 Mobile Bottom Navigation 已经提供：

```text
首页
模型
娱乐
资产
我的
```

Dashboard 不再建立一套重复的通用：

```text
模型
钱包
娱乐
我的
API
...
```

九宫格快捷导航。

各功能入口直接融入对应 Dashboard 模块。

避免出现：

**Bottom Navigation + Quick Action Grid**

两套高度重复的入口体系。

---

# 51. Dashboard Empty / Conditional State

Dashboard 模块必须允许根据用户状态变化。

不得假定所有用户都有：

* API Key；
* API Usage；
* Game History；
* Active Game Session；
* Announcement；
* Recently Played。

---

## 51.1 无 API Key

显示：

`Create your first API Key`

---

## 51.2 无 API Usage

可以显示：

`No API activity yet`

并提供：

`Explore Models`

---

## 51.3 无 Game History

Recently Played 不强制显示空列表。

改为：

`Explore Games`

并进入：

`/games`

---

## 51.4 无 Active Game Session

Active / Resume 模块不占据永久空白区域。

---

## 51.5 无重要公告

Announcements 区域可降低权重。

---

## 51.6 Migrated User Reset-and-Grant State

对于迁移前已存在的 NewAPI 用户，首次进入 Chaldea 时：

- Master Profile 初始化继续正常进行；
- 旧 Active NewAPI Quota 已清零；
- 迁移批次已幂等发放 1,000 API Credit 初始赠金；
- 在没有其他账变时，Dashboard 显示 `API Credit = 1,000`、`Available Chips = 0`、`Total Assets = 1,000`；
- 现有 API Key 仍显示在 API Keys 中并可使用新的初始赠金；
- 页面显示一次性迁移告知，明确“旧额度已按开服公平规则重置，1,000 初始赠金已发放，账号与 Key 未删除”；
- 提供 Rewards Center、Wallet、API Keys 与 API Usage 入口；
- 不把迁移用户当作重新注册，也不重复执行新用户注册回调。

该告知不提供恢复旧余额的操作。

---

# 52. Administrator Dashboard 行为

管理员登录后：

`/dashboard`

仍然展示普通用户 Dashboard。

管理员不会因为拥有管理员权限而自动进入运营统计页面。

管理员额外通过：

* Master Avatar Menu；
* 其他明确 Admin Entry

进入：

`/ops`

Chaldea Operations Dashboard 与普通 User Dashboard 保持独立。

这样管理员仍然可以从普通用户视角正常使用和检查 Chaldea Platform。

---

# 53. IA-03 已冻结结论

以下 Dashboard Information Architecture 已经确认：

1. Dashboard 是已登录用户的 Command Center。
2. Dashboard 与 Public Home 独立存在。
3. Dashboard 不复制其他完整专业页面。
4. Dashboard 同时服务 API 与 Entertainment 两条业务主线。
5. Assets 是两条业务之间的核心桥梁。
6. Dashboard 使用 Status / Action / Activity / Information 四类信息职责。
7. Dashboard 包含 Command Status、Assets & Rewards、API Operations、Entertainment、Active / Resume、Recent Activity、Announcements 七个逻辑区域。
8. Dashboard 顶部显示轻量 Master Identity。
9. Master Identity 包含头像、Master 昵称和简单欢迎信息。
10. Dashboard 不使用大型 Master Profile Banner。
11. API 总额度属于 Dashboard P0 信息。
12. 可用娱乐筹码属于 Dashboard P0 信息。
13. 普通用户不在 Dashboard 感知 Active / Reserve 技术拆分。
14. Dashboard Rewards 同时覆盖 Daily Check-in、Hourly Reward 和 Relief Fund。
15. Dashboard 允许对服务端判定为可领取的三类奖励直接发起领取。
16. Daily Check-in 固定 500 API Credit，并继续按 `Asia/Shanghai` 自然日每天最多成功领取一次。
17. Hourly Reward 单次固定数量为 100，每个用户每小时最多成功领取一次。
18. Relief Fund 单次固定数量为 300；破产条件固定为统一 Total Assets 严格少于 10，等于 10 时不符合资格。
19. Relief Fund 使用 Wallet 的统一总资产口径，不得只根据 Available Chips 判断。
20. Relief Fund 采用成功领取后开始计算的滚动 4 小时冷却，不使用固定自然时段刷新。
21. Dashboard 必须区分可领取、破产但冷却中、冷却结束但未破产等状态。
22. Hourly Reward 数量 100 和非随机规则已冻结；资产类型、冷却口径、累积规则和每日限制仍为 TBD。
23. Relief Fund 数量 300 已冻结；资产类型、累积规则和 Active Poker 行为仍为 TBD，资格阈值与冷却口径不再是 TBD。
24. 奖励成功后直接反馈奖励数量、资产类型和更新后余额。
25. `/rewards` 作为独立 Rewards Center 继续保留。
26. Dashboard 不直接承担完整资产兑换流程。
27. Dashboard 显示当日 API 消耗。
28. API Usage 的完整统计口径在后续 API Usage 设计阶段统一确定。
29. Dashboard API Key 只显示状态摘要和入口，不完整显示 Secret。
30. 无 API Key 时提供 Empty State。
31. Dashboard Models 只展示少量推荐 / 新模型内容。
32. Dashboard Entertainment 不复制完整 Entertainment Hub。
33. Dashboard 支持由用户历史与 Game Registry 动态生成的 Continue Playing / Recently Played。
34. Recently Played 只有用户存在实际游戏记录时才显示。
35. 无游戏历史时降级为 Explore Games，并进入 `/games`。
36. Active / Resume Session 与 Recently Played 分离。
37. Active Game Session 属于 P0 Conditional State，V1 主要实例为 Poker。
38. Active Session 显示对应 Session 状态和 Resume；Poker 额外显示 Table / Stack / Reconnect。
39. Active Game Session 不自动强制恢复；Poker 不自动强制跳转牌桌。
40. Poker 未完成 Cash Out 前的实时变化不作为正式排行榜盈亏。
41. Recent Activity 只做摘要。
42. Recent Activity 不替代 API Usage、Wallet Transactions 或 Game History。
43. Dashboard 展示 Important / Pinned / Latest Announcements 摘要。
44. Entry Popup、Post-login Popup、Dashboard Summary 与 Announcement List / Detail 属于不同展示通道；同一展示版本在同一入口流程不重复弹出。
45. 完整 Rankings 不进入 Dashboard。
46. Dashboard 不强制开发个人排名摘要。
47. 大奖 Feed 不属于 Dashboard 核心区域。
48. Mobile Dashboard 根据移动场景重新排列优先级。
49. Mobile Dashboard 不建立与 Bottom Navigation 重复的快捷九宫格。
50. Dashboard 模块允许 Empty / Conditional State。
51. 奖励领取发生网络异常时先查询原领取结果，不自动重放。
52. 管理员继续使用普通 `/dashboard`。
53. Chaldea Operations 独立通过 `/ops` 进入。
54. 迁移既有用户首次进入时展示一次性余额清零告知，账号与 API Key 继续保留。
55. Dashboard 不新增大型 RP 排行模块；RP Rankings 从 API Usage 与 Personal Hub Cross-link。
56. Acknowledgements 可以进入 Dashboard Pinned 摘要，但不强制形成大型模块。
57. 普通 Post-login Popup 不得遮挡关键业务流程。

---

# 54. IA-04 — Models & API Information Architecture

本节负责定义 Chaldea Platform 中 Models & API 产品域的信息架构、页面职责、登录状态差异以及主要用户流程。

本节冻结：

* Model Square；
* Model Detail；
* API Keys；
* API Usage；
* Request History；
* Request Detail；
* API Access；
* 页面之间的完整接入流程；
* PC 与移动端页面行为原则。

本节不确定：

* 模型卡片最终视觉；
* 模型 Logo 风格；
* 图表颜色；
* 代码块视觉；
* FGO 副标题的具体文案；
* 页面动效；
* 最终 Design System；
* 具体前端组件实现。

---

# 55. Models & API 产品职责

Models & API 产品域应支持用户完成以下完整过程：

```text
Discover
发现可用模型

↓  

Evaluate
了解模型用途、状态、上下文与价格

↓

Authorize
创建或选择 API Key

↓

Integrate
获得 Base URL、Endpoint、模型 ID 与调用示例

↓

Observe
查看 API 消耗与请求记录
```

对应主要页面：

```text
Model Square
      ↓
Model Detail
      ↓
API Keys
      ↓
API Access
      ↓
API Usage
```

该流程表示页面职责关系，不要求用户每次都必须严格按照固定顺序操作。

---

# 56. Models & API Sitemap

当前冻结以下页面结构：

```text
Models & API
│
├── Model Square
│   └── Model Detail
│
├── API Keys
│
├── API Usage
│   ├── Overview
│   ├── Request History
│   └── Request Detail
│
└── API Access
```

逻辑路径：

```text
/models
/models/:model

/api/keys
/api/usage
/api/access
```

权限关系：

| 页面           | 未登录用户 │       已登录用户 |
| ------------ | ----: | ----------: |
| Model Square |   可访问 | 可访问并提供登录态增强 |
| Model Detail |   可访问 | 可访问并提供登录态增强 |
| API Keys     |  不可访问 │         可访问 |
| API Usage    |  不可访问 │         可访问 |
| API Access   |  不可访问 │         可访问 |

Model Square 与 Model Detail 使用同一组页面路径。

不得建立：

```text
/public/models
/app/models
```

两套重复模型页面。

---

# 57. Models & API Context Navigation

已登录用户进入 Models & API 产品域后，使用：

```text
Models | API Keys | Usage | API Access
```

对应：

```text
Models
→ /models

API Keys
→ /api/keys

Usage
→ /api/usage

API Access
→ /api/access
```

未登录用户访问 `/models` 或 `/models/:model` 时，不展示大量不可使用的锁定导航项。

未登录状态保持完整、自然的公共 Model Square 体验。

---

# 58. Model Square 产品职责

Model Square 负责回答：

> Chaldea 当前提供哪些模型，这些模型适合什么用途、价格如何、当前是否可用？

Model Square 的真实业务数据由 NewAPI 与 Chaldea 两部分共同组成。

## 58.1 NewAPI 数据

包括：

* 模型 ID；
* 可用状态；
* 倍率；
* 价格；
* 渠道可用性等真实运行数据。

## 58.2 Chaldea 元数据

包括：

* 展示名称；
* Logo / 图片；
* 简介；
* 标签；
* 推荐用途；
* 上下文长度；
* 排序；
* 是否推荐；
* 是否公开展示；
* FGO 风格副标题。

Model Square 不得变成与底层状态脱节的静态手工模型列表。

---

# 59. Model Square 页面结构

Model Square 当前冻结以下五个逻辑区域：

```text
Model Square
│
├── Page Introduction
├── Search
├── Filter
├── Sort
├── Model Catalog
└── Empty / Error / Maintenance State
```

这些区域是信息职责，不代表最终必须分别表现为独立 Card。

---

## 59.1 Page Introduction

页面顶部可以简要表达：

* 当前公开模型数量；
* 平台模型整体状态；
* 必要的模型维护提示；
* 必要的渠道或服务异常提示。

不在 IA 阶段设计大型 Hero。

---

## 59.2 Search

模型搜索至少支持：

* Chaldea 展示名称；
* 真实模型 ID。

例如用户既可以搜索模型的易读展示名称，也可以直接搜索实际用于 API 调用的模型 ID。

模型的最终技术识别必须以真实模型 ID 为准。

---

## 59.3 Filter

Model Square 当前规划以下筛选维度：

### Availability

按照模型当前可用状态筛选。

具体状态枚举暂不在 IA 阶段锁死，最终必须根据平台可以稳定判断的真实状态定义。

### Recommended

只查看被 Chaldea 标记为推荐的模型。

### Tags / Recommended Uses

根据模型标签或推荐用途筛选。

具体标签词表需要后续单独确认，不在 IA 阶段直接写死。

### Context Length

根据上下文长度筛选。

### Price

根据模型价格范围或计价水平筛选。

价格筛选必须适配不同计价结构，不能假设所有模型只有一个统一单价。

---

## 59.4 Sort

Model Square 至少支持：

* Chaldea 推荐排序；
* 价格从低到高；
* 价格从高到低；
* 上下文长度；
* 名称排序。

默认排序采用：

**Chaldea 推荐排序。**

“最新上线”只有在平台拥有可靠的模型公开时间字段后才可以加入。

不得仅根据模型 ID、同步顺序或数据库插入顺序猜测发布时间。

---

## 59.5 不建立独立模型分类页面

当前不建立：

```text
/models/coding
/models/reasoning
/models/roleplay
/models/cheap
```

等独立分类页面。

模型分类、用途和标签通过 `/models` 内部的 Search、Filter 与 Sort 完成。

避免标签调整后出现大量重复或过时页面。

---

# 60. Model Catalog 信息层级

Model Square 中的每个模型目录项至少表达：

* 展示名称；
* 真实模型 ID；
* 当前可用状态；
* 简短简介；
* 标签或推荐用途；
* 上下文长度；
* 价格摘要；
* 查看详情或使用模型的主要操作。

模型目录项不展示过多完整技术信息。

以下内容优先进入 Model Detail 或 API Access：

* 完整价格表；
* 全部价格维度；
* 所有 Endpoint；
* 完整 cURL 示例；
* 大段模型说明；
* 内部渠道细节；
* 完整调用文档。

Model Square 的主要职责是：

**发现、筛选与比较。**

---

# 61. Model Square 价格摘要

模型广场需要公开价格信息。

Model Square 与 Model Detail 采用两级价格信息结构：

```text
Model Square
→ 紧凑价格摘要

Model Detail
→ 完整价格结构
```

如果一个模型存在多种价格维度，例如：

* 输入；
* 输出；
* 缓存；
* 图片；
* 按次调用；
* 其他特殊计价；

不得为了让目录项更简短，而错误地将其合并成一个可能误导用户的统一价格。

普通用户界面继续使用：

**API Credit / API 额度**

作为价格和消耗单位。

用户界面不突出美元符号。

---

# 62. 模型公开与可用状态

模型需要区分：

* 已公开但暂时不可用；
* 尚未公开或元数据待完善。

两者不得混为同一状态。

---

## 62.1 已公开但暂时不可用

已经公开、但当前暂时不可用的模型仍然保留在 Model Square。

其行为为：

* 模型信息仍可查看；
* 明确显示当前不可用或维护状态；
* 不提供误导性的立即使用操作；
* 使用 CTA 禁用或转换为状态说明；
* Model Detail 仍然可访问；
* 模型 ID 和价格信息仍可以查看。

模型暂时不可用时，不应直接从用户视野中完全消失。

---

## 62.2 待完善元数据

NewAPI 新同步但尚未完成 Chaldea 元数据配置的模型进入：

**待完善元数据**

状态。

此类模型：

* 不进入公开 Model Square；
* 不进入公共搜索结果；
* 不向普通用户展示；
* 由管理员完善展示名称、简介、标签、价格表达等元数据；
* 正式发布后才进入公开模型目录。

---

## 62.3 平台级可用状态

普通用户主要看到：

> 该模型当前是否可以通过 Chaldea Platform 使用。

普通用户页面不直接暴露：

* 上游渠道凭证；
* 渠道内部优先级；
* 渠道轮询策略；
* 渠道错误细节；
* NewAPI 内部配置。

如果模型存在部分渠道可用、部分渠道不可用的复杂情况，需要由后续状态映射规则统一生成用户级状态。

---

# 63. Model Square 登录状态差异

## 63.1 未登录用户

未登录用户可以：

* 浏览模型；
* 搜索模型；
* 使用筛选；
* 使用排序；
* 查看模型状态；
* 查看价格；
* 查看模型 ID；
* 复制公开模型 ID；
* 进入 Model Detail。

当用户准备实际接入模型时，引导进入：

* Login；
* Discord Registration。

---

## 63.2 已登录用户

登录用户在相同 Model Square 基础上额外获得：

* Use This Model；
* API Access 快捷入口；
* API Keys 快捷入口；
* 当前账号相关的接入提示。

不得在每个模型目录项中同时塞入：

* Create Key；
* Copy Endpoint；
* Copy cURL；
* View Usage；
* Manage Key；

等大量并列操作。

每个模型目录项只保留一个清晰的主要动作：

* 查看详情；
* 或使用模型。

完整接入流程进入 Model Detail 与 API Access。

---

# 64. Model Detail 产品职责

Model Detail 负责回答：

> 这个模型是什么、适合什么、当前能否使用、如何计费，以及怎样接入。

当前冻结以下逻辑区域：

```text
Model Detail
│
├── Identity & Status
├── Overview
├── Recommended Uses & Tags
├── Context & Model Information
├── Pricing
├── Availability
└── Access Actions
```

---

## 64.1 Identity & Status

至少包含：

* 展示名称；
* 模型 ID；
* 当前状态；
* 推荐标记；
* Logo / 图片位置；
* FGO 风格副标题位置。

具体视觉样式后续确定。

---

## 64.2 Overview

展示由 Chaldea 维护的简明模型介绍。

主要帮助用户理解：

* 模型主要能力；
* 推荐用途；
* 适合什么场景；
* 何时建议选择该模型。

V1 不需要复制上游厂商完整长文档。

---

## 64.3 Recommended Uses & Tags

显示：

* 推荐用途；
* 标签。

这些标签同时服务：

* Model Square 筛选；
* Model Detail 认知；
* Dashboard 推荐模型等发现入口。

标签必须由平台元数据统一维护，不在前端独立写死。

---

## 64.4 Context & Model Information

至少展示：

* 真实模型 ID；
* 上下文长度；
* 其他已经由 Chaldea 元数据确认的信息。

当前不得在没有可靠数据来源的情况下擅自加入：

* 知识截止日期；
* 模型训练数据；
* Benchmark 分数；
* 模型发布时间；
* 推理等级；
* 工具调用能力；
* 多模态能力细节。

这些字段需要后续正式纳入模型元数据后才能展示。

---

# 65. Model Detail Pricing

Model Detail 提供完整 Pricing 区域。

Pricing 必须能够适配：

* 输入价格；
* 输出价格；
* 缓存读取；
* 缓存写入；
* 图片价格；
* 按请求价格；
* 其他实际计价维度。

Pricing 区域不得提前假定所有模型都采用：

```text
每 1M Token 一个统一价格
```

价格展示最终必须来源于经过标准化的真实计价数据。

详细的价格归一化、舍入和文案规则将在后续：

**Model Pricing Presentation Rules**

中继续确定。

---

# 66. Model Detail Availability

Model Detail 显示模型当前真实状态。

如果模型当前不可用：

* 页面仍然允许访问；
* 明确展示不可用或维护状态；
* 停用 Use This Model 主操作；
* 保留模型信息、价格和模型 ID；
* 不向用户承诺无法实际完成的调用。

Model Detail 不暴露普通用户不需要理解的内部渠道配置。

---

# 67. Use This Model 用户流程

Model Detail 中的：

**Use This Model**

不是在线 Playground。

该操作表示：

> 开始 API 接入流程。

---

## 67.1 未登录用户

```text
Model Detail
→ Use This Model
→ Login / Discord Registration
→ Authentication
→ Account Status Gate
→ Master Initialization（如需要）
→ Migration Notice（如需要）
→ Resource Availability Check
→ Return to Model Detail
```

之后再根据用户是否存在 API Key 继续流程。

---

## 67.2 已登录但没有 API Key

```text
Model Detail
→ Use This Model
→ API Keys
→ Create API Key
→ Creation Success
→ API Access
```

进入 API Access 后自动预选原模型。

---

## 67.3 已登录且已有 API Key

```text
Model Detail
→ Use This Model
→ API Access
```

API Access 自动带入：

* 当前模型 ID；
* 适用 Endpoint；
* 对应 cURL 示例。

该过程只恢复页面选择状态，不发送实际 API 请求。

---

## 67.4 不提供在线调用

Model Detail 不提供：

* 在线聊天；
* Prompt 输入框；
* Run Request；
* Test Model；
* 在线响应结果；
* Playground。

用户需要在自己的客户端或工具中使用 API。

---

# 68. API Keys 产品职责

API Keys 页面负责：

* 展示用户已有 API Key；
* 创建 API Key；
* 复制 API Key；
* 查看基础状态；
* 执行底层真实支持的管理操作；
* 引导用户进入 API Access；
* 维护 General / RP Usage Purpose。

现有 NewAPI 用户的原 API Key 必须继续保留并维持原使用方式，不要求用户重新创建。

---

# 69. API Keys 页面结构

当前冻结：

```text
API Keys
│
├── Page Header
├── Key Summary
├── Key List
├── Create Key Flow
├── Manage Key Actions
├── Usage Purpose
└── Empty State
```

---

## 69.1 Page Header

提供：

* 页面用途说明；
* Create API Key 主操作；
* API Access 快捷入口。

---

## 69.2 Key Summary

轻量展示：

* 当前 API Key 总数；
* 当前有效 API Key 数量。

API Key Summary 不承担完整 API 使用统计。

完整使用情况进入：

`/api/usage`

---

## 69.3 Key List

每个 Key 至少需要表达：

* Key 名称或标签；
* 遮罩后的 Key；
* 当前状态；
* 创建时间；
* Usage Purpose：General、RP 或迁移后的 Unclassified；
* 可用管理操作。

如果底层真实支持，也可以进一步展示：

* 最近使用时间；
* 到期时间；
* 其他 NewAPI 原生字段。

这些附加字段只有在真实数据可获得时才展示。

---

## 69.4 API Key Usage Purpose

API Key Usage Purpose 用于 RP Usage 统计归类，不改变 Key 权限、路由、模型访问或计费。

状态：

- `General`；
- `RP`；
- `Unclassified`，仅用于迁移后的既有 Key 初始状态。

规则：

- 新建 Key 时必须选择 General 或 RP；
- 一个 Key 同一时间只能有一个 Purpose；
- 修改 Purpose 只影响之后的新请求；
- 每条请求保存 `key_purpose_snapshot`；
- Unclassified 请求不进入 RP Rankings；
- 多个 RP Key 按 Master 聚合；
- Key 页面说明 Purpose 只是统计标签。

现有 API Key 在迁移中继续有效，但初始为 Unclassified。

---

# 70. API Key Secret 安全原则

API Key 页面冻结以下安全表现：

1. Key List 默认只显示遮罩值；
2. Dashboard 不显示完整 Secret；
3. 完整 Key 只通过明确的 Reveal 或 Copy 操作取得；
4. 页面不在无必要情况下同时长期暴露多个完整 Key；
5. 移动端不因为空间不足而直接换行展示完整 Key；
6. 删除、禁用等破坏性操作必须二次确认；
7. Request History 和 Request Detail 不显示完整 Key；
8. 代码示例不自动嵌入用户真实 Key。

如果 NewAPI 底层只保存不可逆摘要、无法在创建后重新读取完整 Secret，则完整 Key 只能在创建成功阶段展示一次。

页面不得虚构底层不存在的 Reveal 能力。

该技术能力需要在后续 NewAPI 适配阶段验证，但不改变 API Keys 页面的整体信息架构。

---

# 71. Create API Key Flow

Create API Key 不建立独立一级页面：

```text
/api/keys/create
```

创建流程在 API Keys 页面中通过以下形式之一完成：

* Modal；
* Drawer；
* Mobile Bottom Sheet。

具体视觉形式后续确定。

标准流程：

```text
API Keys
→ Create API Key
→ 填写底层真实支持的创建字段
→ 选择 General 或 RP Usage Purpose
→ Confirm
→ Created
→ Copy Key
→ Go to API Access
```

当前不得擅自加入未确认功能，例如：

* IP 白名单；
* 模型白名单；
* 每日预算；
* 独立模型权限；
* 高级 Scope；
* Key 自动轮换；
* 自定义风控规则。

是否支持这些能力需要以后作为独立需求确认。

---

# 72. API Key Detail

V1 不建立独立：

```text
/api/keys/:id
```

API Key Detail 页面。

API Key 的以下操作在 Key List 与 Modal / Drawer 中完成：

* 查看；
* 复制；
* 修改底层真实支持的字段；
* 禁用；
* 恢复；
* 删除。

如果后续 API Key 引入复杂权限、独立额度、模型限制或专属 Usage 分析，再重新评估是否建立 Key Detail 页面。

---

# 73. API Keys Empty State

如果用户没有 API Key：

页面显示明确 Empty State。

信息语义：

```text
No API Key

Create your first API Key
```

同时可以提供：

* Explore Models；
* API Access 基础说明；
* Create API Key。

但没有 Key 时，API Access 中不得误导用户认为已完成认证配置。

---

# 74. API Usage 产品职责

API Usage 负责回答：

> 在某段时间内消耗了多少 API Credit，以及每次 API 请求发生了什么？

保持单一路径：

```text
/api/usage
```

页面内部使用：

```text
Overview | Request History
```

Overview 与 Request History 不拆成两个 Context Navigation 页面。

---

# 75. Usage Overview

Usage Overview 当前规划以下逻辑区域：

```text
Usage Overview
│
├── Date Range
├── Total API Credit Consumed
├── Today API Usage
├── Request Count
├── Usage Trend
├── Model Breakdown
├── Key Breakdown
├── Purpose Breakdown
└── RP Rankings Cross-link
```

其中当前明确需要支持：

* 所选周期 API Credit 消耗；
* 当日 API Credit 消耗；
* Request History 入口。

Dashboard 显示的 Today API Usage 必须与 API Usage 页面采用同一统计来源和口径。

---

## 75.1 条件指标

以下指标只有在 NewAPI 日志真实提供并且数据可靠时展示：

* 请求数量；
* 成功请求数量；
* 失败请求数量；
* 输入 Token；
* 输出 Token；
* 缓存 Token；
* 响应延迟；
* 按模型消耗；
* 按 API Key 消耗。

IA 可以预留这些信息能力，但不得将无法获得的数据伪装为可用指标。

---

## 75.2 API Usage 不统计资产发放

API Usage 只统计：

**API 请求产生的实际 API Credit 消耗。**

以下内容不进入 API Usage：

* Initial Grant / 初始赠金；
* Daily Check-in；
* Hourly Reward；
* Relief Fund；
* 管理员发放；
* API Credit 与娱乐筹码之间的兑换；
* 游戏派奖；
* Poker 盈亏；
* Wallet 资产调整。

上述数据进入：

* Wallet Transactions；
* Game History；
* Poker Records；
* 其他对应账务页面。

---

# 76. Request History

Request History 当前支持以下查询维度：

* 时间范围；
* 模型筛选；
* API Key 筛选；
* Usage Purpose 筛选；
* 请求状态筛选；
* Request ID 搜索；
* 排序；
* 分页或连续加载。

每条请求记录的核心信息建议包含：

* 请求时间；
* 模型；
* API Key 名称或遮罩标识；
* Key Purpose Snapshot；
* Endpoint；
* 请求状态；
* Token 或用量摘要，如数据存在；
* 消耗的 API Credit。

---

# 77. Request Detail

V1 不将 Request Detail 建立为独立一级 Sitemap 页面。

推荐表现：

### PC

使用：

* Drawer；
* Side Panel。

### 手机

使用：

* Full-screen Detail Layer；
* Full-screen Sheet。

Request Detail 可以展示：

* Request ID；
* 请求时间；
* 模型；
* Endpoint；
* API Key 遮罩标识；
* Key Purpose Snapshot；
* 请求状态；
* 用量；
* API Credit 消耗；
* 错误摘要；
* 其他实际存在的日志元数据。

---

## 77.1 不默认保存 Prompt 与 Response

当前需求没有确认平台保存：

* 用户 Prompt；
* 完整 Request Body；
* 完整模型 Response；
* 完整对话内容。

因此 Request Detail 当前不得擅自设计这些字段。

如果后续确实需要记录这些内容，必须单独确认：

* 隐私边界；
* 存储空间；
* 保留期限；
* 加密要求；
* 用户访问权限；
* 管理员访问权限；
* 敏感信息脱敏；
* 删除策略。

---

## 77.2 API Key 展示

Request Detail 只显示：

* API Key 名称；
* Key ID；
* 遮罩标识。

不得显示完整 Secret。

---

## 77.3 Model Cross-link

Request Detail 中的模型可以链接至：

```text
/models/:model
```

帮助用户从异常或消费记录回查模型信息。

API Key 标识可以帮助用户定位对应 Key，但不得因此暴露完整 Key。

---

# 78. API Usage 时间规则

用户侧以下 API Usage 聚合统一采用：

```text
Asia/Shanghai
```

作为业务时区：

* 今日；
* 本周；
* 按自然日聚合；
* Dashboard Today API Usage；
* RP Rankings Today / This Week。

这与 Daily Check-in 的业务自然日保持一致。Daily Check-in 已确定按照 `Asia/Shanghai` 计算自然日。

Hourly Reward 的冷却窗口口径仍以需求基线中的 TBD 为准。Relief Fund 已固定为成功领取后开始计算的滚动 4 小时冷却，不使用 `Asia/Shanghai` 自然时段或固定四小时窗口。

原始 Request History 仍保存并展示精确请求时间。

用户侧 `This Week` 从 Asia/Shanghai 星期一 00:00 开始。

前端可以根据统一规则格式化显示，但不得让 Dashboard 与 API Usage 使用不同日界线。

---

## 78.1 尚未冻结的时间细节

当前只冻结业务时区。

以下内容仍需在后续技术设计中确认：

* 跨日长请求归属开始时间还是结算时间；
* 流式请求的统计归属；
* 延迟结算记录的日期归属；
* 夏令时以外的显示规则；
* 原始时间存储格式。

不得由不同页面分别定义不一致的规则。

---

# 79. API Access 产品职责

API Access 是一个轻量、任务型接入页面。

其目标是让已有 API 使用经验的用户快速获得：

* Base URL；
* Authentication 说明；
* 常用 Endpoint；
* Model ID；
* cURL 示例；
* 一键复制能力。

API Access 不发展为：

* 大型开发者文档中心；
* 完整 API Reference；
* 各客户端逐项教程；
* 在线 Playground；
* 在线 Prompt 测试工具。

---

# 80. API Access 页面结构

当前冻结：

```text
API Access
│
├── Quick Start
├── Base URL
├── Authentication
├── Endpoint
├── Model Selector
├── cURL Example
└── Related Links
```

---

## 80.1 Quick Start

使用简明步骤说明：

```text
1. 创建 API Key
2. 选择模型
3. 配置 Base URL
4. 发送请求
```

不展开成面向完全零基础用户的大型教程。

---

## 80.2 Base URL

展示 Chaldea Platform 的统一 API Base URL。

提供一键复制。

具体生产域名、子域名和代理拓扑由后续部署设计决定，IA 阶段不写死。

---

## 80.3 Authentication

展示认证头的基本用法。

代码示例统一使用：

```text
<YOUR_API_KEY>
```

不得自动将用户真实 API Key 注入代码块。

用户需要从 API Keys 页面明确复制自己的 Key。

这样可以降低：

* 截图泄露；
* 屏幕共享泄露；
* 复制到公开聊天；
* 粘贴到公开 Issue；
* 页面长期暴露 Secret；

等风险。

---

## 80.4 Endpoint

展示平台实际支持的常用 Endpoint。

具体 Endpoint 列表必须根据 NewAPI 当前兼容能力确认。

不在 IA 阶段擅自承诺底层未支持的接口。

---

## 80.5 Model Selector

API Access 提供模型选择能力。

当用户从 Model Detail 的：

`Use This Model`

进入 API Access 时，自动预选对应模型。

该选择仅改变：

* Model ID；
* 示例；
* 相关 Endpoint 信息。

不会自动发送 API 请求。

---

## 80.6 cURL Example

cURL 示例根据当前选择动态更新：

* Base URL；
* Endpoint；
* Model ID。

API Key 始终使用：

```text
<YOUR_API_KEY>
```

用户可以分别复制：

* Base URL；
* Model ID；
* cURL 示例。

---

## 80.7 Related Links

API Access 提供：

* Manage API Keys；
* Explore Models；
* View API Usage。

这些只是 Cross-link。

API Access 不复制完整：

* API Key 管理；
* Model Square；
* API Usage。

---

# 81. Models & API 核心用户流程

## 81.1 未登录用户发现模型

```text
Public Home / Model Square
→ Model Detail
→ Use This Model
→ Login / Discord Registration
→ Authentication
→ Account Status Gate
→ Master Initialization（如需要）
→ Migration Notice（如需要）
→ Resource Availability Check
→ Return to Model Detail
```

完成登录后：

```text
已有 API Key
→ API Access

没有 API Key
→ API Keys
→ Create API Key
→ API Access
```

---

## 81.2 已登录用户使用模型

```text
Model Square
→ Model Detail
→ Use This Model
→ API Access
→ 当前模型自动预选
```

---

## 81.3 第一次创建 API Key

```text
Dashboard API Key Empty State
→ API Keys
→ Create API Key
→ Creation Success
→ Copy Key
→ API Access
```

---

## 81.4 查看 API 使用情况

```text
Dashboard Today API Usage
→ API Usage
→ Overview
→ Request History
→ Request Detail
```

---

## 81.5 从请求记录回查模型

```text
Request Detail
→ Model ID
→ Model Detail
```

---

## 81.6 API Access 回到 Key 管理

```text
API Access
→ Manage API Keys
→ API Keys
```

Return-to-Intent 只恢复页面和所选模型状态，不自动：

* 创建 Key；
* 删除 Key；
* 发送 API 请求；
* 重放失败请求。

---

# 82. PC 页面行为原则

## 82.1 Model Square

PC 端提供完整：

* Search；
* Filter；
* Sort；
* Model Catalog。

最终使用 Card、List 或混合布局，在后续 Page Layout 阶段确定。

---

## 82.2 Model Detail

PC 端使用清晰的信息分区。

保证以下内容容易定位：

* 模型 ID；
* 状态；
* 上下文；
* 用途；
* Pricing；
* Use This Model。

---

## 82.3 API Keys

PC 端以 Key List 为核心。

Create 与 Manage 使用：

* Modal；
* Drawer。

不建立额外 Key Detail 页面。

---

## 82.4 API Usage

PC 端使用：

```text
Overview | Request History
```

Request Detail 使用侧边详情层。

---

## 82.5 API Access

PC 端提供：

* 可复制信息区域；
* Model Selector；
* 代码示例；
* Related Links。

---

# 83. Mobile 页面行为原则

## 83.1 Model Square

手机端：

* 保留 Search；
* Filter 进入 Bottom Sheet；
* Sort 使用紧凑菜单；
* 模型目录项重新排版；
* 不将 PC 宽表格直接压缩到手机宽度。

---

## 83.2 Model Detail

使用单列信息流。

主要操作保持明显：

* Copy Model ID；
* Use This Model；
* 查看状态；
* 查看价格。

---

## 83.3 API Keys

每个 Key 使用移动端列表项或 Card。

完整 Key 不直接长期显示。

管理操作通过：

* Action Menu；
* Bottom Sheet；
* Confirm Dialog。

---

## 83.4 API Usage

Usage Overview 根据手机空间降低指标密度。

Request History 转换为移动端可读记录项，不强制使用横向宽表格。

Request Detail 使用全屏详情层。

---

## 83.5 API Access

代码块：

* 支持横向滚动；
* 提供一键复制；
* 不为了适配手机宽度而随意换行破坏命令语义。

---

# 84. IA-04 依赖与风险

以下内容不会阻止 IA-04 冻结，但在后续产品数据设计与技术设计中必须继续处理。

---

## 84.1 Model Pricing Presentation Rules

需要统一定义不同模型价格维度的展示方法，包括：

* 输入；
* 输出；
* 缓存；
* 图片；
* 按次调用；
* 倍率；
* 特殊计价。

不得只用一个简单价格数字覆盖所有模型。

---

## 84.2 Model Availability Mapping

需要根据 NewAPI 的真实状态定义：

* 全部渠道可用；
* 部分渠道可用；
* 全部渠道不可用；
* 管理员维护；
* 临时异常；
* 已下线；

如何映射为普通用户看到的平台级状态。

---

## 84.3 API Key Capability Verification

需要确认当前 NewAPI 实际支持：

* 创建；
* 命名；
* 编辑；
* 禁用；
* 恢复；
* 删除；
* 到期时间；
* 独立额度；
* Secret 再次读取。

Chaldea UI 不得虚构底层不存在的 Key 操作。

---

## 84.4 API Usage Log Capability

需要确认 NewAPI Logs 实际提供：

* Request ID；
* 模型；
* Endpoint；
* Key 标识；
* Token；
* 消耗；
* 请求状态；
* 延迟；
* 错误信息。

Request Detail 只展示真实、可靠且允许用户访问的数据。

---

## 84.5 Usage 时间边界

`Asia/Shanghai` 已冻结为业务时区，`This Week` 已冻结为从星期一 00:00 开始。

仍需在技术设计中统一：

* 跨日请求归属；
* 流式请求归属；
* 延迟结算归属；
* 原始时间字段。

---

# 85. IA-04 已冻结结论

以下 Models & API Information Architecture 已确认：

1. Models & API 产品域包括 Model Square、Model Detail、API Keys、API Usage 和 API Access。
2. Model Square 与 Model Detail 对未登录用户公开。
3. API Keys、API Usage 和 API Access 仅登录后访问。
4. Public 与 Auth 共用同一套模型页面。
5. Model Square 使用统一模型目录。
6. Model Square 支持 Search、Filter 和 Sort。
7. 不建立多个独立模型分类页面。
8. 搜索支持展示名称和真实模型 ID。
9. 默认排序采用 Chaldea 推荐排序。
10. 标签词表暂不在 IA 阶段写死。
11. Model Square 显示紧凑价格摘要。
12. Model Detail 显示完整价格结构。
13. 不得错误合并复杂模型计价。
14. 已公开但暂时不可用的模型仍保留展示。
15. 不可用模型明确显示状态，并禁止误导性使用操作。
16. 待完善元数据的模型不进入公开 Model Square。
17. 普通用户看到平台级模型可用状态，不暴露内部渠道配置。
18. Model Square 负责发现与比较，不承载完整技术信息。
19. Model Detail 包含身份、简介、用途、上下文、价格、状态和接入操作。
20. Model Detail 不擅自展示没有可靠来源的模型属性。
21. Use This Model 表示开始 API 接入流程。
22. Use This Model 不表示在线调用模型。
23. 未登录用户登录后返回原 Model Detail。
24. 已登录且没有 Key 的用户先进入 Create API Key。
25. 已有 Key 的用户直接进入 API Access。
26. API Access 自动预选来源模型。
27. V1 不提供在线 Playground。
28. API Keys 页面使用 Key List。
29. V1 不建立独立 Key Detail 页面。
30. Create Key 使用 Modal、Drawer 或 Mobile Sheet。
31. Key 默认遮罩。
32. 完整 Key 仅通过明确 Reveal / Copy 操作取得。
33. 不在 Dashboard、Request History 或代码示例中显示完整 Secret。
34. 如果底层不能再次读取 Secret，则只在创建成功阶段展示完整值。
35. 删除、禁用等破坏性 Key 操作需要确认。
36. 不擅自加入底层未确认的 Key 高级能力。
37. API Usage 保持单一路径 `/api/usage`。
38. API Usage 内部使用 Overview 与 Request History。
39. Request Detail 不建立独立一级页面。
40. PC Request Detail 使用 Drawer / Side Panel。
41. 手机 Request Detail 使用全屏详情层。
42. API Usage 只统计 API 请求实际消耗。
43. Initial Grant、Daily Check-in、Hourly Reward、Relief Fund、兑换和游戏账变不进入 API Usage。
44. Dashboard Today API Usage 与 Usage 页面采用相同统计口径。
45. 用户侧今日和本周 API Usage 使用 `Asia/Shanghai`。
46. 原始 Request History 保留精确时间。
47. Request Detail 不默认规划保存 Prompt 或完整 Response。
48. Request Detail 不显示完整 API Key。
49. API Access 为轻量任务型接入页面。
50. API Access 提供 Base URL、Authentication、Endpoint、Model ID 和 cURL。
51. API Access 不发展为大型文档中心。
52. API Access 代码示例使用 `<YOUR_API_KEY>`。
53. 用户真实 Key 不自动嵌入代码示例。
54. Model Selector 可以根据 Model Detail 入口自动预选模型。
55. Return-to-Intent 不自动重放 API 请求或 Key 管理操作。
56. 手机 Model Square 的 Filter 使用移动端紧凑交互。
57. 手机 Request History 不直接压缩 PC 宽表格。
58. 手机代码块允许横向滚动并支持复制。
59. Model Pricing、Availability Mapping、Key 能力和日志字段继续作为后续技术依赖确认。
60. API Key 增加 General / RP / Unclassified Usage Purpose。
61. 新建 Key 必须选择 General 或 RP；既有 Key 初始为 Unclassified。
62. Purpose 修改不追溯历史，每条请求保存 Purpose Snapshot。
63. API Usage 提供 Purpose Filter 与 RP Rankings Cross-link。

---

# 86. IA-05 — Assets / Wallet / Rewards Information Architecture

本节负责定义 Chaldea Platform 中 Assets 产品域的用户心智模型、Wallet、资产兑换、用户资产记录、Rewards Center、Poker 牌桌中资产以及 PC / Mobile 行为。

本节冻结：

* 用户可见总资产结构；
* API Credit、Available Chips、Poker In Play 与 Processing Assets 的关系；
* Wallet Overview；
* Exchange；
* Wallet Transactions；
* Transaction Detail；
* Rewards Center；
* Daily Check-in；
* Hourly Reward；
* Relief Fund；
* Poker 资产位置与正式盈亏的区别；
* V1 单用户同时入座限制；
* 主要资产与奖励 UX Flow。

本节不深入：

* 数据库表结构；
* Saga 代码实现；
* 行锁与事务具体实现；
* Poker Pot 计算算法；
* 最终视觉样式；
* 已确认奖励数值对应的资产类型展示、Hourly Reward 冷却口径，以及 Relief Fund 的累积和 Active Poker 行为；不得重新讨论已冻结的 1,000 / 500 / 100 / 300 数量。

---

# 87. Assets 产品域的用户心智模型

用户只需要理解以下资产状态：

```text
Total Assets
│
├── API Credit
├── Available Entertainment Chips
├── Poker In Play
└── Processing Assets（仅存在时）
```

## 87.1 API Credit

API Credit 用于 API 请求消费，也可以按照 1:1 兑换为娱乐筹码。

普通用户只看到统一 API 总额度，不感知：

* Active API Quota；
* Reserve API Credit；
* NewAPI raw quota；
* atomic units。

## 87.2 Available Entertainment Chips

Available Chips 表示当前仍在娱乐主钱包中、可以立即用于以下操作的筹码：

* 直接游玩游戏及其他支持主钱包下注的游戏；
* Poker Buy-in；
* 兑换回 API Credit。

## 87.3 Poker In Play

Poker In Play 表示已经从娱乐主钱包转入 Active Poker Session 的资产。

该资产暂时不能直接用于其他游戏或兑换，但在尚未被其他玩家赢走或合法转移前仍属于用户总资产。

## 87.4 Processing Assets

Processing Assets 表示跨库兑换等正式资产操作中，已经离开源余额、但尚未进入目标已结算余额的资产。

该资产：

* 不得从用户总资产中无解释消失；
* 必须只计算一次；
* 不得同时重复计入源余额和目标余额；
* 仅在真实存在未完成交易时显示。

---

# 88. Assets Sitemap 与导航

Assets 产品域保持两个主页面：

```text
Assets
│
├── Wallet
│   ├── Overview
│   ├── Exchange
│   └── Transactions
│
└── Rewards Center
    ├── Daily Check-in
    ├── Hourly Reward
    ├── Relief Fund
    └── Reward History
```

逻辑路径：

```text
/wallet
/rewards
```

权限：两者均仅登录后访问。

Assets Context Navigation：

```text
Wallet | Rewards
```

Wallet Local Navigation：

```text
Overview | Exchange | Transactions
```

Rewards Local Navigation：

```text
Daily | Hourly | Relief
```

Reward History 可以作为 Rewards Center 内部公共历史区域或局部视图，不建立新的全局一级页面。

---

# 89. Total Assets 定义

Wallet Overview 显示：

**Total Assets / 总资产**

用户可见逻辑：

```text
Total Assets
=
API Credit
+
Available Entertainment Chips
+
Poker In Play
+
尚未计入任一已结算余额的 Processing Assets
```

处理中资产只有在未与其他余额重复计算时才单独加入。

例如：

```text
API Credit                 10,000
Available Chips            30,000
Poker In Play              20,000
Processing Assets               0

Total Assets               60,000
```

总资产只表达 Chaldea 平台内资产，不显示美元符号或法币估值。

---

# 90. Wallet 产品职责

Wallet 负责回答：

> 我当前拥有多少资产，哪些可以立即使用，哪些在 Poker 牌桌中，哪些正在处理中，以及最近发生了哪些资产变化？

Wallet 不提供与当前产品边界冲突的主要操作：

* 充值；
* 购买；
* 法币提现；
* 用户间转账；
* 用户间赠送；
* 收款。

Wallet 的主要操作保持为：

* Exchange；
* View Transactions；
* Rewards Center；
* Poker 资产状态入口。

---

# 91. Wallet Overview

Wallet Overview 当前冻结以下逻辑区域：

```text
Wallet Overview
│
├── Total Assets
├── Asset Breakdown
├── Processing / Attention State
├── Primary Actions
├── Active Poker Assets
└── Recent Transactions
```

## 91.1 Total Assets

页面最高层显示：

* 总资产；
* 最近更新时间；
* 必要的资产处理状态。

## 91.2 Asset Breakdown

至少拆分：

```text
API Credit
Available Chips
Poker In Play
Processing Assets（存在时）
```

## 91.3 API Credit 区域

可以提供：

* 当前统一余额；
* Today API Usage 快捷摘要；
* View API Usage；
* Exchange to Chips。

不复制完整 Request History。

## 91.4 Available Chips 区域

显示当前能够立即用于直接游玩游戏、其他支持主钱包下注的游戏、Poker Buy-in 或兑换的娱乐筹码。

Poker In Play 不计入 Available Chips。

## 91.5 Processing / Attention State

存在未完成兑换、退回中或需要处理的资产操作时，Wallet Overview 必须高可见提示。

用户可以：

* 查看状态；
* 查看金额与方向；
* 复制 Transaction ID；
* 进入 Transaction Detail；
* 刷新真实状态。

不得在状态未知时直接创建一笔看似相同的重复操作。

## 91.6 Primary Actions

主要操作保持克制：

```text
Exchange
View Transactions
Rewards
```

## 91.7 Recent Transactions

Overview 只展示少量近期资产业务记录。

完整列表进入 Transactions。

---

# 92. Global Asset Summary

PC Global Header 的 Asset Summary 与 Wallet 使用相同资产语义。

默认显示：

```text
API Credit
Available Chips
```

如果存在 Poker In Play，应提供紧凑但明确的牌桌中状态或标记。

不得只显示一个合并后的“娱乐总资产”，却让用户无法理解为什么其中一部分不能立即下注或兑换。

点击 Asset Summary：

`→ /wallet`

完整 Total Assets 与 Processing Assets 主要在 Wallet Overview 展示。

---

# 93. Poker 资产展示

Poker 使用牌桌买入制，必须区分：

```text
Available Chips
≠
Poker In Play
```

## 93.1 Buy-in

示例：

```text
Buy-in 前
Available Chips          50,000
Poker In Play                 0
Total Entertainment      50,000
```

Buy-in 20,000 后：

```text
Available Chips          30,000
Poker In Play            20,000
Total Entertainment      50,000
```

Buy-in 是资产位置变化，不是消费或亏损。

## 93.2 Active Session 实时资产

Wallet 可以反映 Active Poker Session 中已经由服务端正式结算到玩家牌桌资产状态的变化，并明确标注：

* Session 进行中；
* 尚未完成最终 Cash Out；
* 正式 Poker 盈亏尚未实现。

## 93.3 当前 Hand 与 Pot

Poker In Play 不能只读取一个简单桌面 Stack 后让已投入 Pot 的资产从 Wallet 中消失。

Poker In Play 的展示必须基于 Poker Service 可恢复的完整资产状态，考虑：

* 当前桌面 Stack；
* 当前 Hand 已投入金额；
* Pot / Side Pot 关联状态；
* 已完成 Hand 的结算结果。

IA-08 将 Poker In Play 的用户口径冻结为 `Table Stack + 当前 Hand 尚未结算的自身投入`；最终账务字段、快照读取与恢复算法在技术设计阶段实现，但 UX 必须始终保持资产可解释。

## 93.4 正式盈亏确认

Wallet 当前资产状态与排行榜正式盈亏分离：

```text
Wallet
→ 可以展示当前 Poker In Play

Rankings / Formal P&L
→ 仅在 Safe Leave + Cash Out + Session Settlement 后更新
```

## 93.5 V1 同时入座限制

V1 中，一个用户同时只能在一张 Poker Table 入座并持有一个 Active Poker Session。

必须先完成：

```text
Safe Leave
→ Cash Out
→ Session Closed
```

才能在另一张牌桌再次入座。

---

# 94. Exchange 产品职责

Exchange 支持：

```text
API Credit → Entertainment Chips
Entertainment Chips → API Credit
```

两种方向均为：

* 1:1；
* 零手续费；
* 存量资产形态转换；
* 不创造或销毁用户总资产。

Poker In Play 不能直接用于兑换。用户必须先完成 Safe Leave / Cash Out，使资产回到 Available Chips。

---

# 95. Exchange 页面结构

当前冻结：

```text
Exchange
│
├── Direction
├── Source Balance
├── Amount Input
├── Quick Amount
├── Rate & Fee
├── Result Preview
├── Confirmation
└── Processing / Result
```

## 95.1 Direction

用户明确选择兑换方向，页面始终保持源资产和目标资产可辨认。

## 95.2 Source Balance

显示当前真正可兑换的来源余额。

## 95.3 Amount Input

兑换输入遵守平台 atomic unit 精度：

* 最多接受并显示 6 位小数；
* 最小精确单位为 `0.000002`；
* 自动去除无意义尾随零；
* 无法精确对应 atomic unit 的输入明确拒绝；
* 不静默采用页面各自不同的舍入规则。

例如：

```text
0.0372
```

有效。

```text
0.000001
```

无法精确对应一个 atomic unit，应明确拒绝。

## 95.4 Quick Amount

可以提供：

```text
25% | 50% | Max
```

`Max` 以当前服务端确认的可兑换余额为准。

## 95.5 Rate & Fee

确认前始终显示：

```text
Rate
1 API Credit = 1 Entertainment Chip

Fee
0
```

## 95.6 Result Preview

提交前展示：

* 源资产扣除；
* 目标资产增加；
* 兑换金额；
* 手续费；
* 预计兑换后余额。

---

# 96. Exchange Confirmation 与 Processing

## 96.1 明确确认

每笔兑换需要一次简明确认：

```text
Confirm Exchange

5,000 API Credit
→
5,000 Entertainment Chips

Rate 1:1
Fee 0
```

确认后才产生正式唯一交易意图。

## 96.2 Processing State

用户可见主状态：

```text
Submitting
→ Processing
→ Completed
```

异常可能进入：

```text
Processing
→ Returning Assets
→ Returned
```

或：

```text
Processing
→ Needs Attention
```

## 96.3 提交后不提供普通取消

确认提交后，不提供普通“取消兑换”按钮。

用户可以安全离开页面，但交易状态由服务端记录继续决定。

## 96.4 禁止盲目重试

发生 HTTP 超时、刷新、断网或前端未收到响应时：

* 先查询原 Transaction / Transfer 状态；
* 不立即重新执行相同兑换；
* 只有确认原交易未产生资产变化或已经完整退回后，才允许创建新交易。

## 96.5 用户可见状态

普通用户看到：

| 状态 | 含义 |
|---|---|
| 处理中 | 正式交易仍在执行 |
| 已完成 | 两侧资产均已正确更新 |
| 退回中 | 系统正在恢复源资产 |
| 已退回 | 交易未完成，资产已经恢复 |
| 需要处理 | 自动处理尚未完成 |
| 未执行 | 未产生正式资产变化 |

不要求普通用户理解内部 Saga 状态名。

---

# 97. Wallet Transactions

Wallet Transactions 回答：

> 我的平台资产为什么增加、减少或移动？

它不是：

* API Request History；
* 完整 Game History；
* Poker Hand History；
* 管理员原始 ledger 浏览器。

## 97.1 页面结构

```text
Transactions
│
├── Balance Context
├── Search / Filter
├── Transaction List
└── Transaction Detail
```

## 97.2 Filter

支持按以下维度筛选：

### Asset

* API Credit；
* Entertainment Chips；
* Poker Assets。

### Type

* Initial Grant（Registration / Migration）；
* Daily Check-in；
* Hourly Reward；
* Relief Fund；
* Administrator Adjustment；
* Exchange；
* Game Settlement；
* Poker Buy-in；
* Poker Cash Out；
* Refund / Compensation；
* API Consumption Summary。

### Status

* Processing；
* Confirmed；
* Returned；
* Needs Attention。

### Time

* 时间范围。

---

# 98. 用户记录与底层 Ledger 的关系

底层 ledger 保持 append-only。

用户界面可以按照统一业务标识将多个组成账变组合成可理解的一条业务记录，但不得修改或合并底层账本事实。

## 98.1 Exchange

API → Chips 显示为一条复合业务记录：

```text
Exchange Completed

API Credit             -5,000
Entertainment Chips    +5,000
Fee                          0
Net Total Change             0
```

## 98.2 Reward Claim

奖励记录显示：

* Reward Type；
* 发放资产；
* 发放数量；
* 领取时间；
* 状态；
* Business ID；
* 领取后余额。

## 98.3 Game Round Settlement

按 `round_id` 组合显示：

```text
Bet
Payout
Net Change
```

完整过程和 Provably Fair 进入 Round Detail。

## 98.4 Poker Buy-in

显示为内部资产转移：

```text
Available Chips       -20,000
Poker In Play         +20,000
Net Total Change            0
```

不得显示为普通消费或亏损。

## 98.5 Poker Cash Out

显示：

```text
Poker In Play         -24,000
Available Chips       +24,000
Session Realized P/L   +4,000
```

## 98.6 API Consumption

Wallet 不复制每一条 API 请求。

按 `Asia/Shanghai` 业务日显示 API Consumption Summary：

```text
2026-09-01 API Usage
-123.45 API Credit
357 Requests

View API Usage →
```

逐请求模型、Endpoint、Token 和错误信息继续进入 `/api/usage`。

---

# 99. Transaction Detail

V1 不建立独立：

```text
/wallet/transactions/:id
```

页面。

表现方式：

* PC：Drawer / Side Panel；
* 手机：Full-screen Detail Sheet。

根据交易类型显示：

* 业务类型；
* 状态；
* 创建时间；
* 完成时间；
* 资产变化；
* Balance Before；
* Balance After；
* Fee；
* Transaction ID / Business ID；
* Round ID；
* Poker Session；
* 管理员调整原因；
* 补偿或退回状态；
* 相关专业页面链接。

Cross-link 示例：

```text
Daily / Hourly / Relief Reward
→ Rewards Center

Game Round
→ Round Detail

Session / Hand Game
→ Session / Hand Detail

API Consumption Summary
→ API Usage
```

---

# 100. Rewards Center 产品职责

Rewards Center 负责回答：

> 当前有哪些基础奖励可以领取、下一次什么时候可领取、我是否符合救济资格，以及过去领取了什么？

页面结构：

```text
Rewards Center
│
├── Current Server / Business Time
├── Daily Check-in
├── Hourly Reward
├── Relief Fund
├── Reward History
└── Rules / Maintenance State
```

Rewards Center 不引入第三种隐藏奖励资产。

如果某项奖励发放娱乐筹码，由于娱乐筹码可以 1:1 兑换为 API Credit，该奖励仍属于平台新增可兑换资产。

## 100.1 Initial Grant / 初始赠金状态

Initial Grant 不是可循环领取的 Rewards Center 卡片，而是一次性开服资产来源：

- 新用户完成 Discord 注册与账号创建后，获得一次 **1,000 API Credit** 初始赠金；
- 迁移用户的旧额度完成清零和校验后，由迁移批次获得一次 **1,000 API Credit** 初始赠金；
- 两种场景金额相同，但使用不同的幂等业务 ID，不得重复发放，也不得把迁移用户解释为重新注册；
- 新用户的真实到账状态在 Completion Summary 中显示；
- 迁移用户的真实到账状态在 Migration Notice、Dashboard / Wallet 与 Transactions 中显示；
- 初始赠金进入 API Credit，不在 Daily / Hourly / Relief 局部导航中增加第四个循环领取入口；
- Wallet Transactions 与后台 Issuance Analytics 必须能够区分 Registration Initial Grant 与 Migration Initial Grant。

---

# 101. Daily Check-in

Daily Check-in 保持已确认规则：

* 每个 `Asia/Shanghai` 自然日最多成功领取一次；
* 每次固定获得 **500 API Credit**；
* 奖励进入 API Credit；
* 不再采用 1,000–10,000 随机范围；
* Dashboard 与 Rewards Center 共用同一领取行为；
* 多标签页、多设备和重试只能产生一条记录与一笔奖励。

页面展示：

* 当前业务日期；
* 今日状态；
* Claim；
* 本次奖励；
* 更新后 API Credit；
* 固定奖励值 500 API Credit；
* 最近领取记录。

不加入：

* Streak；
* 七日大奖；
* 补签；
* 连续签到倍率。

---

# 102. Hourly Reward

Hourly Reward 已确认：

* 每个用户每小时最多成功领取一次；
* 单次奖励数量固定为 **100**；
* 不采用随机金额；
* 必须使用服务端时间；
* 必须生成正式领取记录和资产账变；
* Dashboard 与 Rewards Center 可以直接领取；
* 冷却中显示下一次可领取状态。

以下仍为 TBD：

* 100 发放为 API Credit 或 Entertainment Chips；
* `Asia/Shanghai` 自然小时或滚动 60 分钟；
* 是否累积；
* 是否存在每日次数或总量上限。

页面可以写明固定数字 100，但资产图标、单位名称和倒计时算法必须读取服务端配置，不得自行假设。

---

# 103. Relief Fund

Relief Fund 用于让统一总资产已经低于 Direct Play 最低下注 10 的玩家重新开始游戏，单次奖励数量固定为 **300**。

## 103.1 破产定义

固定资格条件：

`Total Assets < 10`

Total Assets 必须复用 Wallet Overview 已冻结的统一资产汇总：

- API Credit；
- Available Entertainment Chips；
- Poker In Play；
- 未与其他余额重复计算的 Processing Assets。

页面和服务端不得只检查 Available Chips。

总资产等于 10 时不属于破产，只有严格低于 10 才满足资产条件。

页面必须显示固定救济数量 300；实际资产图标和单位名称由服务端返回，因为 API Credit / Entertainment Chips 类型仍待确认。

## 103.2 滚动冷却

Relief Fund 使用领取成功后开始计算的滚动 4 小时冷却：

`next_claim_at = last_successful_claim_at + 4 hours`

不使用 00:00、04:00、08:00 等固定时段刷新。

- 从未成功领取且已经破产：立即可领取；
- 成功领取后：开始 4 小时倒计时；
- 冷却中再次破产：仍需等待；
- 冷却结束但未破产：仍不可领取；
- 冷却结束后再次破产：立即可领取；
- 失败、拒绝、维护或未形成成功领取记录：不开始或重置冷却。

## 103.3 领取与状态

符合服务端资格且可领取时，可以在 Dashboard 或 Rewards Center 直接领取。

领取提交时，服务端必须重新计算 Total Assets 和冷却状态，并记录资格判断快照。多标签页、多设备、刷新和网络重试不得产生重复奖励。

页面至少需要表达：

- 可领取；
- 领取中；
- 破产但冷却中；
- 冷却结束但未破产；
- 奖励处理中；
- 已领取；
- 维护中；
- 需要处理。

## 103.4 仍待确认

救济金单次数量已经固定为 **300**。以下内容仍为 TBD：

- 300 发放为 API Credit 或 Entertainment Chips；
- 未领取或长期未使用的资格是否累积；
- Active Poker Session 中 Total Assets 仍低于 10 时是否允许领取；
- 管理员临时关闭功能，以及未来通过版本化政策调整金额的具体规则。

300 的固定数量、破产阈值、总资产口径和滚动 4 小时冷却均已经冻结。

# 104. Reward Common States

三类奖励共用清晰状态语义：

| 状态 | 用户含义 |
|---|---|
| 可领取 | 当前可以提交 Claim |
| 领取中 | 服务端正在处理，禁止重复提交 |
| 已领取 | 当前周期已经成功领取 |
| 冷却中 | 尚未到下一次领取时间 |
| 不符合资格 | 当前 Total Assets 为 10 或以上，或其他服务端资格条件不满足 |
| 奖励处理中 | 领取记录已存在，资产仍在确认 |
| 维护中 | 当前奖励被运营暂停 |
| 需要处理 | 自动恢复尚未完成 |

网络超时时，应先查询原领取结果。

如果奖励已经成功，返回原奖励而不是再次发放或显示笼统失败。

---

# 105. PC 与 Mobile 行为

## 105.1 PC Wallet

```text
Assets Context
Wallet | Rewards

Wallet Local
Overview | Exchange | Transactions
```

* Overview 展示完整资产拆分；
* Exchange 展示预览、确认与 Processing；
* Transactions 使用完整列表；
* Transaction Detail 使用 Drawer；
* Active Poker 与 Processing 状态高可见。

## 105.2 Mobile Wallet

手机 Bottom Navigation 的“资产”默认进入：

`/wallet`

顶部 Context Navigation：

```text
Wallet | Rewards
```

Wallet Local Navigation：

```text
Overview | Exchange | Transactions
```

移动端 Overview 优先顺序：

```text
1. Processing / Attention Notice
2. Total Assets
3. API Credit
4. Available Chips
5. Poker In Play
6. Primary Actions
7. Recent Transactions
```

Transactions 使用移动记录项，不压缩 PC 宽表格。

## 105.3 Mobile Rewards

Rewards Center 使用：

```text
Daily | Hourly | Relief
```

不在同一手机首屏纵向堆叠三套大型领取面板。

Reward History 与 Rules 可作为页面下部或独立局部视图。

---

# 106. Assets 与 Rewards 核心 UX Flow

## 106.1 Dashboard 查看资产

```text
Dashboard Asset Summary
→ Wallet Overview
```

## 106.2 API Credit 兑换 Chips

```text
Wallet
→ Exchange
→ API Credit → Chips
→ Enter Amount
→ Preview
→ Confirm
→ Processing
→ Completed
```

## 106.3 Chips 兑换 API Credit

```text
Wallet
→ Exchange
→ Chips → API Credit
→ Enter Amount
→ Preview
→ Confirm
→ Processing
→ Completed
```

## 106.4 Dashboard Daily Check-in

```text
Dashboard
→ Claim Daily Check-in
→ Reward Result: +500 API Credit
→ API Credit Updated
```

## 106.5 Dashboard Hourly Reward

```text
Dashboard
→ 服务端显示可领取
→ Claim Hourly Reward
→ Reward Result: +100（资产类型由服务端返回）
→ 对应资产 Updated
```

## 106.6 Dashboard Relief Fund

```text
Dashboard
→ 服务端返回 Total Assets 与滚动冷却状态
→ Total Assets < 10 且冷却结束 / 从未领取
→ Claim Relief Fund
→ 服务端提交时重新校验资格
→ Reward Result: +300（资产类型由服务端返回）
→ 成功领取时间写入
→ next_claim_at = claimed_at + 4 hours
→ 对应资产 Updated
```

如果 Total Assets 为 10 或以上，或 4 小时滚动冷却尚未结束，不创建成功领取记录，也不启动新的冷却。

## 106.7 查看完整奖励

```text
Dashboard Reward Status
→ Rewards Center
→ Daily / Hourly / Relief
→ Reward History
```

## 106.8 Poker Buy-in

```text
Available Chips
→ Poker Lobby
→ Buy-in
→ Available Chips Decrease
→ Poker In Play Increase
→ Total Assets Unchanged
→ Poker Table
```

## 106.9 Poker Cash Out

```text
Poker Table
→ Safe Leave
→ Cash Out
→ Poker In Play Decrease
→ Available Chips Increase
→ Session P/L Realized
→ Poker Lobby / Wallet
```

---

# 107. IA-05 依赖与风险

以下内容不阻止 IA-05 冻结，但必须继续保留为待确认事项。

## 107.1 Reward Economics

理论上高频领取会持续新增可兑换资产。

运营后台需要至少能够查看：

* 每日奖励总发行量；
* 按 Daily / Hourly / Relief 分类的发行量；
* 人均领取量；
* 发放资产类型；
* 兑换回 API Credit 的数量；
* 异常高频或重复请求。

## 107.2 Hourly Reward Rules

已冻结：固定数量 100、非随机。仍需确认：资产类型、时间窗口、累积与每日限制。

## 107.3 Relief Fund Rules

已冻结：统一总资产严格少于 10 的破产阈值，以及成功领取后开始计算的滚动 4 小时冷却。

已冻结：固定数量 300。仍需确认：资产类型、累积规则与 Active Poker 行为。后续技术设计还需保证跨 NewAPI / Chaldea 资产组成的资格快照一致且不会重复计入。

## 107.4 Processing Assets Calculation

后续技术设计必须确保处理中资产只计入一次，不能产生总资产重复计算。

## 107.5 Poker In Play Calculation

后续 Poker 技术设计必须明确 Stack、已投入 Pot、Side Pot 与未结算 Hand 的资产展示口径。

---

# 108. IA-05 已冻结结论

以下 Assets / Wallet / Rewards Information Architecture 已确认：

1. Assets 产品域包含 Wallet 与 Rewards Center 两个主页面。
2. Wallet 路径为 `/wallet`。
3. Rewards Center 路径为 `/rewards`。
4. Wallet Local Navigation = Overview / Exchange / Transactions。
5. Rewards Local Navigation = Daily / Hourly / Relief。
6. 用户侧总资产包括 API Credit、Available Chips、Poker In Play 和未重复计算的 Processing Assets。
7. 普通用户不感知 Active / Reserve 技术拆分。
8. Poker In Play 属于用户资产，但不属于可立即兑换或用于其他游戏的 Available Chips。
9. Processing Assets 必须可见、可追踪且只计入一次。
10. Wallet 不提供充值、购买、法币提现、用户转账或赠送。
11. Wallet Overview 包含 Total Assets、Asset Breakdown、Processing State、Primary Actions、Active Poker 与 Recent Transactions。
12. Global Asset Summary 主要显示 API Credit 与 Available Chips。
13. 存在 Poker In Play 时，Global Asset Summary 提供明确的牌桌中状态。
14. Poker Buy-in 是内部资产位置变化，不是消费或亏损。
15. Wallet 可以展示 Active Poker 当前资产状态。
16. Poker 正式盈亏仅在 Safe Leave / Cash Out / Session Settlement 后确认。
17. 当前 Hand 已投入 Pot 的资产不能从用户资产视图中无解释消失。
18. V1 一个用户同时只能在一张 Poker Table 入座并持有一个 Active Poker Session。
19. 完成当前 Safe Leave / Cash Out 后才能在另一张桌入座。
20. Exchange 支持 API Credit 与 Entertainment Chips 1:1 双向兑换。
21. Exchange 零手续费。
22. Poker In Play 不能直接兑换。
23. Exchange 最多接受 6 位小数。
24. Exchange 最小精确单位为 `0.000002`。
25. 无法对应 atomic unit 的输入明确拒绝，不静默舍入。
26. Exchange 确认前显示方向、金额、汇率、手续费和预计余额。
27. 每笔 Exchange 需要一次明确确认。
28. 提交后的 Exchange 不提供普通取消。
29. 网络异常时先查询原交易状态，不盲目重试。
30. Wallet 使用用户可理解的业务记录，不直接平铺所有底层 ledger entry。
31. Exchange 组合显示为一笔双资产记录。
32. Reward Claim 按奖励类型显示资产、数量、时间、状态和 Business ID。
33. Round-based Game 按 Round 分组展示 Bet、Payout 与 Net Change。
34. Poker Buy-in 显示为内部资产转移。
35. Poker Cash Out 显示 Session Realized P/L。
36. Wallet 不复制逐请求 API History。
37. API 消耗在 Wallet 中按 `Asia/Shanghai` 业务日聚合，并链接至 API Usage。
38. V1 不建立独立 Transaction Detail 路径。
39. PC Transaction Detail 使用 Drawer / Side Panel。
40. 手机 Transaction Detail 使用 Full-screen Detail Sheet。
41. Rewards Center 覆盖 Daily Check-in、Hourly Reward、Relief Fund 与 Reward History。
42. Daily Check-in 每个 `Asia/Shanghai` 自然日最多成功领取一次。
43. Daily Check-in 固定获得 500 API Credit。
44. Hourly Reward 每个用户每小时最多成功领取一次，单次固定数量为 100。
45. Relief Fund 单次固定数量为 300；破产资格固定为统一 Total Assets 严格少于 10，等于 10 时不符合资格。
46. Relief Fund 资格复用 Wallet 的统一总资产口径，包括 API Credit、Available Chips、Poker In Play 与未重复计算的 Processing Assets。
47. Relief Fund 采用成功领取后开始计算的滚动 4 小时冷却，不采用固定自然时段刷新。
48. 从未成功领取且已经破产的用户可以立即领取；失败或被拒绝的请求不开始或重置冷却。
49. Dashboard 与 Rewards Center 可以直接领取服务端明确判定为可领取的奖励。
50. 奖励领取必须使用服务端状态、幂等记录与正式资产账变。
51. 多标签页、多设备、刷新和网络重试不得重复发放奖励。
52. Hourly Reward 数量 100 与非随机规则已冻结；资产类型、冷却口径、累积和每日限制仍为 TBD。
53. Relief Fund 数量 300 已冻结；资产类型、累积和 Active Poker 行为仍为 TBD，资格阈值、资格口径与冷却口径已经冻结。
54. 当前不加入连续签到、七日大奖、补签或连续签到倍率。
55. Mobile Rewards 使用 Daily / Hourly / Relief 的局部导航，不在同一首屏堆叠三套大型面板。

---

# 109. IA-06 — Entertainment Hub & Extensible Game Architecture

本节负责冻结 Entertainment Hub、Game Catalog、Game Entry、通用 Game Shell、动态 Game History、Game Registry 以及未来新增游戏的接入边界。

本节确认：

- V1 首发清单与平台长期游戏容量分离；
- 娱乐目录、导航、历史与后台不得绑定固定游戏数量；
- `/games` 为独立、公开可浏览的 Game Catalog；
- `/games/:game_slug` 为直接游玩游戏的稳定逻辑入口；
- Poker 保留独立 Lobby / Table；
- 后续游戏可以使用 Direct Play、Lobby 或其他适配结构；
- 游戏恢复、维护与历史记录必须保持资产和 Round 可解释。

本节不确定：

- V1 各游戏的详细规则、默认 RTP、赔率与奖表；
- 未来麻将、斗地主、炸金花等具体游戏的规则与服务形态；
- 最终游戏卡片与 Game Stage 视觉；
- 动画、音效与 FGO 包装；
- Game Registry 的数据库与 API 实现。

---

# 110. V1 首发范围与平台容量

V1 首发仍然包括：

- Slot Machine；
- Dice；
- Blackjack；
- Scratch Card；
- Summon / Gacha；
- Poker；
- Rankings；
- Game History。

该清单只定义第一阶段的产品范围与验收对象。

它不代表：

- 平台永久只有五款直接游玩游戏；
- Context Navigation 需要逐项列出所有游戏；
- Game History 永久只区分 Solo 与 Poker；
- 运营后台 Sidebar 永久写死 V1 游戏名称；
- 新增游戏需要重构 Entertainment Hub。

未来新增游戏应通过 Game Registry、Game Catalog、通用 Game Entry 与动态历史筛选接入。

---

# 111. Entertainment Sitemap

Entertainment 产品域冻结为：

```text
Entertainment
│
├── Entertainment Hub
│   └── /entertainment
│
├── Game Catalog
│   └── /games
│
├── Game Entry
│   └── /games/:game_slug
│
├── Lobby-based Games
│   └── Poker
│       ├── /poker
│       └── /poker/table/:table_id
│
├── Rankings
│   └── /rankings
│
└── Game History
    └── /history
```

Entertainment Hub 与 Game Catalog 对未登录用户开放。

Game Entry、Poker Lobby、Poker Table 与个人完整 Game History 需要登录。

---

# 112. Entertainment Context Navigation

Entertainment Context Navigation 保持稳定：

```text
Hub | Games | Poker | Rankings | History
```

对应：

```text
Hub
→ /entertainment

Games
→ /games

Poker
→ /poker

Rankings
→ /rankings

History
→ /history
```

Games 不表现为无限膨胀的游戏名称下拉菜单。

新增游戏正式发布后进入 `/games`，不修改 Global Header 和 Context Navigation 的基本结构。

Poker 在 V1 中保留独立入口，因为它拥有完整多人大厅、房间、桌子、实时 Session 和沉浸式牌桌。

未来其他大型多人游戏是否增加独立快捷入口，必须单独评估，不自动将所有新增游戏提升为 Context 一级项。

---

# 113. Entertainment Hub 产品职责

Entertainment Hub 负责回答：

> Chaldea 当前有什么值得玩、可以继续什么、正在进行什么，以及有哪些娱乐活动与重要状态？

其逻辑区域为：

```text
Entertainment Hub
│
├── Entertainment Status
├── Active / Resume
├── Continue Playing
├── Featured Games
├── Browse Games
├── Multiplayer Spotlight
└── Events, Rankings & Public Wins
```

上述内容为信息职责，不代表最终必须制作七张 Card。

## 113.1 Entertainment Status

轻量展示：

- 娱乐系统整体状态；
- Available Chips 摘要；
- 游戏维护通知；
- 重要娱乐活动。

不复制完整 Wallet。

## 113.2 Active / Resume

当用户存在尚未结束、可以恢复的游戏 Session 时高优先级显示。

V1 首个主要实例为：

`Active Poker Session → Reconnect`

未来其他可恢复实时游戏也可以进入该模块。

模块不得在数据结构上命名为只支持 Poker。

## 113.3 Continue Playing

Continue Playing 根据：

- 用户历史；
- Game Registry；
- 当前发布状态；
- 当前运行状态

动态生成。

不写死 Slot、Dice、Summon 或固定显示数量。

如果最近游戏维护中：

- 仍可显示最近玩过；
- CTA 改为维护状态或查看历史；
- 不提供无法完成的 Play 操作。

没有历史时降级为：

`Explore Games → /games`

## 113.4 Featured Games

由运营配置：

- 推荐游戏；
- 新上线游戏；
- 活动游戏；
- 编辑精选。

前台可根据布局展示有限数量，但数据与架构不得假定平台只有这些游戏。

## 113.5 Browse Games

Entertainment Hub 只提供：

- 少量分类入口；
- 部分目录预览；
- Browse All Games。

完整目录进入 `/games`。

## 113.6 Multiplayer Spotlight

V1 重点展示 Poker Lobby、活动牌桌与 Active Session。

未来增加其他多人游戏后，该区域可以扩展为多个多人入口，但不改变 Game Catalog 的统一归属。

只有真实可获得的数据才可以展示在线人数、公开桌数量或等待房间数量。

## 113.7 Events、Rankings 与公共记录

Entertainment Hub 可以展示少量：

- 当前游戏活动；
- Rankings 摘要；
- 最近大奖；
- 精选开奖记录。

完整内容分别进入：

- `/announcements`；
- `/rankings`；
- `/history` 或对应公共记录入口。

不公开所有用户的完整输赢流水。

---

# 114. Entertainment Hub 登录状态差异

## 114.1 未登录用户

可以查看：

- 娱乐中心介绍；
- Game Catalog 预览；
- 已公开游戏；
- 游戏状态；
- 游戏简介、分类与标签；
- Poker 产品介绍；
- Rankings；
- 公共大奖；
- 公开活动。

点击实际游戏时：

```text
Entertainment Hub / Game Catalog
→ Play
→ Login / Discord Registration
→ Authentication
→ Account Status Gate
→ Master Initialization（如需要）
→ Migration Notice（如需要）
→ Resource Availability Check
→ Return-to-Intent
→ 原目标 Game Entry 或 Lobby
```

## 114.2 已登录用户

在同一页面基础上增加：

- Available Chips；
- Active / Resume；
- Continue Playing；
- 个性化推荐；
- Poker Lobby 入口；
- Wallet / Rewards Cross-link。

Public 与 Auth 共用同一个 Entertainment Hub。

---

# 115. Game Catalog 产品职责

Game Catalog 使用：

`/games`

负责回答：

> 当前已经发布哪些游戏，它们是什么类型、是否可用，以及点击后会进入哪里？

页面逻辑区域：

```text
Game Catalog
│
├── Page Introduction
├── Search
├── Category / Tag Filter
├── Mode Filter
├── Availability Filter
├── Sort
├── Game List
└── Empty / Maintenance State
```

---

# 116. Game Catalog Search、Filter 与 Sort

## 116.1 Search

搜索至少支持：

- 展示名称；
- 稳定 Game Slug；
- 运营配置的别名或关键词，如后续正式支持。

最终技术识别以稳定 `game_slug` 为准。

## 116.2 Category / Tag Filter

分类和标签由 Game Registry / 运营元数据动态提供。

可能的概念包括 Card、Chance、Table、Casual、Strategy、Instant、Multiplayer，但这些仅为示例，不在 IA 阶段冻结词表。

前端不得写死永久分类。

## 116.3 Mode Filter

可以表达：

- All；
- Solo；
- Multiplayer；
- 其他未来正式模式。

Mode 是目录筛选维度，不是固定 Sitemap 层级。

## 116.4 Availability Filter

按照用户可见运行状态筛选。

## 116.5 Sort

建议支持：

- Chaldea 推荐排序；
- 名称；
- 最近上线；
- 下注 / 买入门槛（仅在不同游戏之间存在可比较差异时）；
- 热度。

其中“最近上线”和“热度”只有存在可靠字段和统计口径时才显示。

默认使用 Chaldea 推荐排序。

---

# 117. Game Catalog 条目信息

每个游戏条目至少能够表达：

- 稳定 Game Slug；
- 展示名称；
- 图片 / Logo；
- 简短介绍；
- 分类与标签；
- 游戏模式；
- 当前状态；
- 下注规则摘要：Direct Play 显示全局最低下注 10 与无产品层固定最高下注；大厅型游戏显示其买入、盲注、底注或其他适用门槛；
- 透明度信息摘要；
- 主要进入操作。

RTP、赔率、概率和完整权重表是否显示，继续服从该游戏独立透明度配置。

如果某项未公开，不显示虚假占位值。

---

# 118. Stable Game Slug

每款游戏必须拥有稳定 `game_slug`。

例如 V1 可以存在：

```text
slot
dice
blackjack
scratch
summon
```

这些仅是 V1 实例，不是固定枚举上限。

展示名称、FGO 副标题和主题包装可以调整，但稳定标识不得随意变化。

稳定 Slug 用于：

- Deep Link；
- Game Registry；
- Game History；
- Round / Session 关联；
- 配置版本；
- Rankings 与运营统计。

---

# 119. Game Entry Type

游戏目录项支持不同进入类型：

| Entry Type | 用户行为 |
|---|---|
| Direct Play | 进入 `/games/:game_slug` |
| Lobby | 进入该游戏大厅或房间列表 |
| Resume | 恢复已有活动 Session |
| Maintenance | 展示状态，禁止开始新局 |
| Coming Soon | 仅展示预告 |

不是所有游戏都必须进入直接游玩页面。

Poker 当前进入 `/poker`。

未来麻将、斗地主等可以进入自己的 Lobby，不需要套用 Slot / Dice 的 Game Shell。

---

# 120. 发布状态与运行状态

游戏平台区分两类状态。

## 120.1 Publication State

决定游戏是否进入公共目录：

- Draft / Unpublished；
- Published。

Draft / Unpublished：

- 不进入公共目录；
- 不进入公共搜索；
- 仅管理员可见；
- 用于开发和元数据准备。

## 120.2 Runtime State

决定已发布游戏当前行为：

- Available；
- Maintenance；
- Temporarily Unavailable；
- Coming Soon；
- Retired。

最终状态名可以在状态模型设计时调整，但 Publication 与 Runtime 不得合并为一个模糊字段。

## 120.3 Retired Game

退役游戏可以从主目录隐藏，但：

- 历史记录继续可读；
- 当时名称与配置保留；
- Round / Session 不删除；
- Provably Fair 验证数据不丢失。

---

# 121. 新游戏接入边界

动态 Game Catalog 不等于无代码游戏生成器。

完整新游戏仍需：

1. 规则设计与确认；
2. 服务端权威逻辑；
3. 钱包扣款与结算；
4. Round / Session 状态；
5. 随机游戏的 Provably Fair；
6. 前端 Game Stage 与操作；
7. 断线、刷新与异常恢复；
8. 历史与运营统计；
9. 测试和审核；
10. 注册到 Game Registry；
11. 完善元数据并发布。

运营后台负责配置与发布，不负责凭空生成游戏规则和结算代码。

---

# 122. 通用 Game Page Shell

Direct Play 游戏复用以下信息职责：

```text
Game Page
│
├── Game Header
├── Status / Maintenance Notice
├── Game Stage
├── Wager & Action Area
├── Current Round State
├── Result
├── Rules & Transparency
├── Provably Fair
└── Personal Game History
```

通用职责不意味着所有游戏拥有完全相同的视觉布局。

## 122.1 Game Header

至少提供：

- 返回 Game Catalog / Entertainment；
- 游戏名称；
- 当前状态；
- Available Chips；
- Wallet / Rewards 入口；
- 游戏信息入口。

## 122.2 Game Stage

承载游戏专有表现，例如转轴、牌区、骰子、刮奖区或召唤演出。

## 122.3 Wager & Action Area

根据游戏承担：

- 下注金额；
- 下注选项；
- 玩家操作；
- 开始 Round；
- 多步骤行动。

所有允许玩家从娱乐主钱包选择单局基础下注金额的 Direct Play 游戏，共用 IA-07 冻结的全局下注策略：

- 最低下注 10 娱乐筹码；
- 不设置产品层固定最高下注；
- 快捷金额 10 / 100 / 500 / 1000；
- 快捷金额只用于选择或填入金额，不直接创建 Round；
- 实际下注不得超过可用娱乐筹码与服务端安全范围。

单个游戏不得独立覆盖为不同最低下注或固定最高下注。Poker 与其他大厅型多人游戏使用自己的资金和牌桌规则。

## 122.4 Current Round State

UI 必须表达 Ready、Submitting、Bet Accepted、Resolving、Waiting for Action、Settled、Cancelled、Refunded、Recovering 等对应语义。

不要求普通用户直接看到内部英文枚举。

## 122.5 Result

展示：

- 游戏结果；
- Bet；
- Payout；
- Net Change；
- 更新后的 Available Chips；
- Round ID；
- 查看详情。

结果来自服务端；动画只负责揭示，不决定结果。

---

# 123. Game Interaction Capabilities

通用 Game Shell 至少兼容三种能力。

## 123.1 Instant Resolve

单次操作后直接结算。

V1 可能实例：Dice、部分 Slot 行为。

## 123.2 Reveal Sequence

服务端结果已经确定，但前端通过交互或动画逐步揭示。

V1 可能实例：Scratch、Summon、部分 Slot 演出。

## 123.3 Multi-action Round

一局中需要玩家继续提交动作。

V1 主要实例：Blackjack。

这些是能力组合，不是永久固定游戏分类。

---

# 124. Game Layout Modes

平台支持：

```text
Standard
Focused
Immersive
```

- Direct Play 默认使用 Focused；
- Poker Table 使用 Immersive；
- 未来游戏是否使用 Lobby 或 Immersive，按照具体交互重新确认；
- 不因 Poker 使用沉浸模式而让全部新增游戏自动隐藏导航。

---

# 125. Rules、Transparency 与 Provably Fair

随机游戏页面需要能够访问：

- Rules；
- Payout / Odds；
- RTP / Probability，如配置公开；
- Provably Fair；
- My History。

具体可以使用 Drawer、Tabs、Modal、Sheet 或页面信息区，留到 Layout 阶段确定。

如果后台不公开某项：

- 不显示伪造值；
- 不通过空占位暗示必然可见；
- 历史 Round 仍保留真实配置；
- 公平验证继续遵守需求基线。

---

# 126. 余额不足与 Rewards Flow

服务端判断 Available Chips 不足时：

```text
Play
→ Balance Insufficient
→ 不创建有效下注 Round
→ 提供解决路径
```

解决路径：

- Exchange API Credit → `/wallet`；
- Claim Rewards → `/rewards`；
- Return to Game。

从 Wallet 或 Rewards 返回后：

- 恢复原 Game Entry；
- 可以保留尚未提交的下注输入；
- 刷新真实余额；
- 不自动提交下注。

继续遵守：

**Navigation Resume ≠ Action Replay。**

---

# 127. Round 提交、重复操作与恢复

用户提交下注后：

```text
Submit
→ Lock duplicate action
→ Server accepts or rejects
→ Return authoritative Round state
```

如果服务端已经接受下注：

- 刷新不能重新下注；
- 断线重进恢复同一 Round；
- 已结算返回同一结果；
- 处理中显示 Recovering；
- 已退款显示正式 Refund 状态。

如果服务端确认没有创建有效 Round，用户才可以重新提交。

前端响应超时不得直接创建另一局。

---

# 128. Maintenance 与已接受 Round

Maintenance 主要阻止新 Round。

已经被服务端接受的 Round 不得被维护切换直接遗弃。

必须根据游戏能力：

- 正常完成；
- 恢复后继续；
- 超时托管；
- 或正式退款。

具体采用何种策略，在每款游戏规则与技术设计中确定，但资产与历史状态必须可解释。

---

# 129. Dynamic Game History

Game History 统一使用：

`/history`

筛选：

- Mode；
- Game；
- Time；
- Result。

Game Filter 根据 Game Registry 动态生成。

不同游戏支持不同详情粒度：

```text
Round-based Game
→ Round Detail

Session-based Game
→ Session Detail

Hand-based Game
→ Hand Detail
```

Poker 可以保留快捷筛选，但不能成为永久写死的唯一 Multiplayer 分类。

---

# 130. Chaldea Operations Games

运营后台 Games 采用：

```text
Games
│
├── Global Direct-play Wager Policy
│   ├── Minimum Bet: 10
│   ├── Product Maximum: Unbounded
│   └── Quick Amounts: 10 / 100 / 500 / 1000
├── Game Registry / Game List
├── Select Game
│   ├── Metadata
│   ├── Publication
│   ├── Availability
│   ├── Wager Policy Reference / Game-specific Additional Rules
│   ├── Odds / Probability / RTP
│   ├── Transparency
│   ├── Configuration Versions
│   ├── Events
│   └── Audit
└── Ordering & Recommendation
```

增加新游戏后，通过 Game List 管理，不增加固定 Sidebar 项。

Direct Play 游戏引用同一全局下注策略，不提供单游戏最低下注、固定最高下注或快捷金额覆盖。Blackjack Double / Split 等追加下注，以及其他游戏特有的追加金额规则，在对应游戏规格和配置版本中管理。

每次配置修改继续遵守版本化和审计要求。

---

# 131. PC 与 Mobile 行为

## 131.1 PC Entertainment Hub

允许更丰富的运营区、目录预览和 Multiplayer Spotlight，但不得复制完整 Game Catalog。

## 131.2 Mobile Entertainment Hub

优先级建议：

1. Active / Resume；
2. Available Chips 与重要维护；
3. Continue Playing；
4. Featured Games；
5. Browse All Games；
6. Multiplayer；
7. Events / Rankings。

## 131.3 Mobile Game Catalog

- Search 保留；
- Filter 使用 Bottom Sheet；
- Sort 使用紧凑菜单；
- 条目重新排版；
- 不压缩 PC 宽表格。

## 131.4 Mobile Direct Play

默认使用最小顶部栏与 Focused Layout。

某款游戏如需隐藏 Bottom Navigation，必须在该游戏页面设计时单独确认。

---

# 132. IA-06 已冻结结论

1. V1 首发清单与平台游戏容量分离。
2. V1 首发游戏不构成永久数量上限。
3. 新增独立公开 Game Catalog `/games`。
4. Entertainment Hub 负责推荐和运营，Game Catalog 负责完整目录。
5. Entertainment Context 的 Games 固定进入 `/games`。
6. Context Navigation 不罗列固定游戏名称。
7. 直接游玩游戏使用稳定 `/games/:game_slug`。
8. Game Slug 与展示名称分离。
9. 分类、标签、模式、推荐和条目由 Game Registry 动态提供。
10. Game Catalog 支持 Direct Play、Lobby、Resume、Maintenance、Coming Soon。
11. 不为每款游戏增加独立公共 Game Detail 页面。
12. 实际 Game Entry 需要登录。
13. Poker 保留独立 Lobby / Table。
14. 未来多人游戏可以拥有自己的 Lobby，不强制套用单人页面。
15. Publication State 与 Runtime State 分离。
16. Draft / Unpublished 游戏不进入公共目录。
17. Maintenance 游戏保留介绍与历史，但禁止新 Round。
18. Retired 游戏不得丢失历史、配置和公平验证数据。
19. Direct Play 游戏复用通用 Game Shell 信息职责。
20. 通用 Game Shell 不要求完全相同的视觉布局。
21. Game Shell 兼容 Instant Resolve、Reveal Sequence、Multi-action Round。
22. Direct Play 默认使用 Focused Layout。
23. Poker Table 使用 Immersive Layout。
24. 余额不足时进入 Wallet / Rewards，返回后恢复页面但不自动下注。
25. 服务端已接受的下注在刷新、断线或重进后恢复同一 Round。
26. 客户端超时不得创建重复 Round。
27. Maintenance 不得遗弃已接受 Round。
28. Game History 使用动态 Mode / Game / Time / Result Filter。
29. Game History 支持 Round、Session、Hand 不同详情粒度。
30. Dashboard Continue Playing 由用户历史与 Game Registry 动态生成。
31. Active / Resume 作为通用可恢复 Session 模块，Poker 是 V1 主要实例。
32. Chaldea Operations Games 使用动态 Game Registry / Game List。
33. 新增完整游戏仍需代码、服务端结算、钱包集成、必要的 Provably Fair、前端交互、测试和发布。
34. Game Registry 不提供无代码生成完整游戏逻辑的能力。

---

---

# 133. IA-07 — V1 Direct-play Game Instances

IA-07 将 V1 五款直接游玩游戏的已确认规则正式并入页面结构文档：

- Dice / 骰子猜大小；
- Scratch Card / 刮刮乐；
- Summon / Gacha / 扭蛋机；
- Slot Machine /老虎机；
- Blackjack / 21 点。

五款游戏均属于 Game Registry 中的首发 `Direct Play` 实例，不构成平台永久游戏数量上限。未来新增游戏继续遵守 IA-06 的可扩展 Game Catalog、稳定 `game_slug`、动态历史筛选与通用 Game Shell 原则。

本章冻结玩法、数值、页面信息职责、操作流程、Round 状态、恢复行为、透明度、Provably Fair、PC / Mobile 行为及表现能力插槽；不冻结精确色板、字体、FGO 角色、背景、图标、材质、音效、动画分镜和具体时长。上述视觉内容继续进入 Art Direction v0.4。

---

# 134. IA-07 共享产品边界

## 134.1 服务端权威

所有五款游戏均采用 Server Authoritative 模型：

- 客户端只提交游戏类型、基础下注、玩家选择、Client Seed 与必要动作；
- 客户端不得决定随机结果、牌序、奖级、派彩、输赢或最终余额；
- 下注接受、Round 创建、随机结果、派奖和账变由服务端完成；
- 同一个 `round_id` 只能完成一次最终结算；
- 已经 `SETTLED` 的 Round 不得重复扣款、重复派奖或重复写入资产变化。

## 134.2 视觉与业务状态分离

服务端结果与正式账务结算不依赖动画完成。Spin、Roll、Scratch、Summon、Deal、发牌、翻牌和中奖演出只负责表现已经锁定的业务状态。

跳过、加速、刷新、断线或重播表现不得：

- 改变结果；
- 重新抽取或重新洗牌；
- 创建新的 Round；
- 产生第二次扣款；
- 产生第二次派奖；
- 修改已经锁定的配置版本。

## 134.3 V1 不提供自动连续扣款

五款游戏均不提供会持续产生新资产操作的 Auto Play：

- Dice 不提供 Auto Roll；
- Scratch 不提供 Auto Buy；
- Summon 不提供 Auto Summon；
- Slot 不提供 Auto Spin；
- Blackjack 不提供 Auto Play / Auto Strategy。

保留上一次参数仅用于减少重复输入，用户仍必须再次主动点击当前游戏主操作。

## 134.4 Free Round 边界

V1 五款游戏均不启用游戏专属 Free Round / Free Summon。后续若引入免费局，必须先统一确认名义下注额、派奖基准、RTP、累计下注与排行榜统计，不能把 `0` 筹码输入直接视为普通付费 Round。

---

# 135. IA-07 共享下注规则最终版

## 135.1 适用范围

适用于使用娱乐主钱包、由玩家在一局开始前选择基础下注金额的 Direct Play 游戏。

Poker、斗地主、麻将、炸金花及其他使用 Buy-in、Blind、Ante、Base Score、Raise 或 Table Stack 的大厅型游戏不套用本节。

## 135.2 全局数值

```text
最低下注：10 娱乐筹码
产品层固定最高下注：不设置
快捷金额：10 / 100 / 500 / 1000
下注步长：1 个整数娱乐筹码
```

“不设置产品层固定最高下注”不表示允许透支。实际金额仍不得超过：

- 当前 `Available Chips`；
- 当前动作所需可扣余额；
- 服务端、数据库与整数溢出保护允许的安全范围。

## 135.3 统一下注组件

所有适用游戏提供：

```text
[自定义整数下注金额]

[10] [100] [500] [1000]
```

快捷按钮只填入金额，不直接扣款、创建 Round 或开始游戏。正式提交由当前游戏的 `Roll`、`Buy Card`、`Summon`、`Spin` 或 `Deal` 触发。

## 135.4 默认值与记忆

- 首次进入某款游戏时，基础下注默认值为 `10`；
- 一局完成后保留该游戏上一局的有效基础下注；
- 金额记忆仅在同一游戏内生效；
- Dice、Scratch、Summon、Slot 与 Blackjack 不共享最近下注金额；
- 当快捷金额超过当前可用余额或模式总成本上限时，对应按钮禁用；
- V1 不提供 `Max`、`50%`、百分比或 `All-in` 快捷按钮。

## 135.5 游戏特有派生金额

共享输入仅表示基础下注：

- Slot：输入为整个 Spin 的 `Total Wager`，再平均分配至 10 条固定 Payline；
- Summon：输入为每个 Draw 的 `Base Wager`，十连总成本为 `Base Wager × 10`；
- Blackjack：输入为 `Initial Wager`，Double / Split 追加下注由当前 Hand Stake 派生；
- Blackjack Natural 3:2 派彩可以产生 `0.5` 娱乐筹码，使用原子单位精确支付；
- 玩家主动输入仍只接受整数娱乐筹码。

---

# 136. IA-07 共享 Round、恢复与维护规则

## 136.1 通用业务状态

不同游戏可以增加特有表现状态，但业务层至少表达：

```text
READY
→ SUBMITTING
→ BET_ACCEPTED
→ RESOLVING / PLAYER_TURN
→ SETTLED
```

异常与恢复状态包括：

```text
RECOVERING
CANCELLED
REFUNDED
```

服务端接受下注后，当前 Round 的基础参数被锁定，输入与快捷按钮进入不可编辑状态；Round 完成或正式退款后才重新开放。

## 136.2 刷新、断线与重复请求

如果服务端尚未接受下注：

- 不产生正式资产变化；
- 用户可以重新提交。

如果服务端已经接受下注：

- 页面刷新不得再次扣款；
- 网络超时不得创建第二个 Round；
- 重新进入游戏必须查询并恢复同一 `round_id`；
- 已结算时返回同一结果；
- 处理中时显示恢复状态；
- 已退款时显示正式退款状态。

多步骤 Blackjack 的每个动作还必须使用唯一 `action_id` 与 `action_sequence`，重复动作请求返回同一结果。

## 136.3 维护状态

Maintenance 主要阻止创建新 Round，不得遗弃已经接受的 Round：

- 即时游戏优先完成原确定性结果；
- Reveal 类游戏恢复同一结果并允许继续揭示；
- Blackjack 保留原牌靴与行动状态；
- 只有无法恢复合法确定性结果或无法完成正式结算时才退款；
- 不允许同一 Round 既退款又派奖。

## 136.4 余额不足与 Return-to-Intent

当 `Available Chips < 10`，普通付费 Round 不可创建。页面提供 Wallet 与 Rewards Center 入口。

用户返回游戏后：

- 刷新最新余额；
- 恢复尚未提交的选项与金额；
- 不自动下注；
- 不自动重复此前失败的动作。

继续遵守：**Navigation Resume ≠ Action Replay**。

---

# 137. IA-07 共享 Provably Fair、透明度与版本化

## 137.1 每局绑定数据

随机游戏 Round 至少绑定：

- `server_seed_hash`；
- 按既定时点公开的 `server_seed`；
- `client_seed`；
- `nonce`；
- `round_id`；
- `algorithm_version`；
- `game_config_version`；
- `game_config_hash`；
- 游戏特有的 Paytable、Reel Strip、Prize Table、Pool 或 Shuffle 版本。

Client Seed 默认由系统生成，用户可以在 Provably Fair 面板查看并修改下一 Round 的 Client Seed；已经 `BET_ACCEPTED` 的 Round 不可修改。

## 137.2 无偏随机映射

有限结果映射必须避免 modulo bias，可使用 rejection sampling、无偏 Fisher–Yates 或其他经过验证的确定性无偏算法。

21 点在初始发牌前一次性确定完整 312 张牌顺序；玩家动作只决定从既定牌序中消耗多少张牌，不触发重新洗牌。

## 137.3 历史与配置版本

管理员可以创建、验证并发布新配置版本，但不得：

- 修改已经发布且被历史 Round 引用的版本；
- 修改已经接受下注的 Round；
- 在不创建新版本时改变概率、赔率、牌序映射或奖表；
- 删除历史验证所需的数据。

每个 Round Detail 必须保留当时实际使用的规则、配置、算法和公平验证数据。

## 137.4 透明度

Dice、Scratch、Summon 与 Slot 的已确认概率、奖表或数学规则始终公开。Blackjack 始终公开牌靴、副数、S17、3:2、Peek、Double、Split、Split Aces 与无边注等核心规则；其参考 RTP / House Edge 必须由冻结规则和参考 Basic Strategy 经验证器计算后发布，不得手工伪造固定百分比。

---

# 138. IA-07 共享页面、历史与响应式原则

## 138.1 通用 Game Entry

五款游戏均使用稳定逻辑入口：

```text
/games/dice
/games/scratch
/games/summon
/games/slot
/games/blackjack
```

并采用 `Focused Game Layout`。页面至少具备：

- 返回 Game Catalog / Entertainment；
- 游戏名称、运行状态和 Available Chips；
- Game Stage；
- Wager / Action Area；
- Current Round State；
- Result Summary；
- Rules、Transparency、Provably Fair 与 History 入口。

## 138.2 Game History

五款游戏均为 Round-based Game，记录进入：

```text
/history/round/:id
```

Round Detail 至少保存：

- Round ID 与时间；
- 下注、追加下注及模式；
- 服务端结果与派彩；
- 结算前后余额；
- 算法、配置与奖表版本；
- Provably Fair 验证数据；
- Cancelled / Refunded 信息。

每款游戏再增加其特有结果字段。

## 138.3 PC 与 Mobile

- PC 保持 Game Stage、关键下注或行动区在同一视野内可理解；
- 手机竖屏必须完整可玩，不强制横屏；
- 不能把 PC 游戏区域机械缩小；
- 手机默认保留 Bottom Navigation，但不得遮挡主操作、结果或行动区；
- 若真实浏览器测试证明某款游戏受到严重干扰，必须返回页面设计阶段重新确认，不能由实现模型自行隐藏；
- Round Detail 在手机使用全屏详情层；
- `prefers-reduced-motion` 或低性能设备仍须显示完整静态结果。

## 138.4 Presentation Hooks

IA-07 只冻结演出阶段、跳过 / 加速、恢复与结果不受表现影响。角色、背景、材质、图标、光效、音效、镜头和具体时长留到 Art Direction v0.4。


# 139. Dice / 骰子猜大小

## 139.1 游戏身份

展示名称：骰子猜大小 / Dice Big or Small
稳定 game_slug：dice
逻辑入口：/games/dice
Entry Type：Direct Play
主要交互能力：Instant Resolve + Reveal Sequence
资金模型：娱乐主钱包付费 Round
页面 Shell：Focused Game Layout

## 139.2 核心玩法

每局由服务器通过 Provably Fair 确定性随机过程生成三颗公平的六面骰结果。

Small / 小：
- 三颗骰子点数总和为 4–10；
- 任何三同号不属于 Small。

Big / 大：
- 三颗骰子点数总和为 11–17；
- 任何三同号不属于 Big。

Triple / 三同号：
- 1-1-1；
- 2-2-2；
- 3-3-3；
- 4-4-4；
- 5-5-5；
- 6-6-6。

出现任何三同号时：
- 选择 Big 判负；
- 选择 Small 判负。

V1 仅开放：
- Big；
- Small。

V1 不开放：
- Triple 单独下注；
- 指定总点数；
- 指定骰子点数；
- 单双；
- 围骰；
- 其他骰宝下注区域。

## 139.3 选择规则

每个 Round：
- 必须且只能选择 Big 或 Small 中的一项；
- Big 与 Small 为互斥单选；
- 不允许同时押 Big 与 Small；
- 未选择任何一项时不能提交 Roll。

第一次进入 Dice：
- 不默认选择 Big；
- 不默认选择 Small；
- 必须由用户主动选择。

一局结算后：
- 保留上一局的 Big / Small 选择；
- 用户仍必须再次点击普通 Roll 才能开始下一局；
- 不自动下注；
- 不自动创建新 Round。

V1 不提供独立 Repeat Bet 按钮。
上一局参数保留后，普通 Roll 即承担再次提交的入口。

V1 不提供 Auto Roll / 连续下注。

## 139.4 赔率与数学口径

Big / Small 基础赔率固定为 1:1。

1:1 的明确含义：
- 胜利时净盈利等于下注金额；
- 含返还本金的总派彩为下注金额的 2 倍；
- 失败时总派彩为 0。

三颗六面骰共有：

6 × 6 × 6 = 216

种等概率有序结果。

Small 获胜结果：
105 种。

Big 获胜结果：
105 种。

三同号结果：
6 种。

单侧获胜概率：

105 / 216
≈ 48.611111%

理论 RTP：

2 × 105 / 216
≈ 97.222222%

理论平台优势：

1 - 97.222222%
≈ 2.777778%

V1 中：
- 1:1 赔率作为固定游戏规则；
- 不允许管理员日常调整该赔率；
- 概率、赔率、RTP 与三同号通杀规则始终公开；
- 不受通用透明度开关隐藏。

如未来改变赔率或结果映射：
- 必须创建新的游戏规则版本；
- 必须创建新的配置版本；
- 必须重新计算并公开 RTP；
- 历史 Round 继续保留当时真实赔率和规则版本。

## 139.5 全局下注规则

Dice 引用 Direct Play 全局下注策略：

最低下注：
10 娱乐筹码

产品层固定最高下注：
不设置

固定快捷金额：
10 / 100 / 500 / 1000

下注金额规则：
- 只接受整数娱乐筹码；
- 10、11、1527 等整数均可；
- 不接受小数下注；
- 实际下注不得超过 Available Chips；
- 实际下注不得超过服务端与数据库安全范围；
- “不设最高下注”不表示允许透支。

第一次进入 Dice：
- 默认下注金额为 10。

一局结算后：
- 保留上一局 Dice 下注金额；
- Dice 的金额记忆只属于 Dice；
- 不与 Slot、Blackjack 或其他游戏共享最近下注金额。

快捷金额行为：
- 点击后只填入金额；
- 不直接 Roll；
- 不直接扣款；
- 不直接创建 Round。

当快捷金额高于 Available Chips：
- 对应快捷按钮禁用；
- 自定义输入仍可输入不超过余额的合法整数。

V1 不增加：
- Max；
- 50%；
- 百分比快捷按钮；
- All-in。

## 139.6 单局用户流程

```text
进入 /games/dice
→ 查看 Available Chips、最低下注、规则摘要
→ 下注输入默认显示 10，或恢复该用户上一次 Dice 有效下注金额
→ 用户主动选择 Big 或 Small
→ 用户输入自定义整数金额，或点击 10 / 100 / 500 / 1000
→ 点击 Roll
→ 前端进入 SUBMITTING 并锁定重复操作
→ 服务端校验登录、游戏状态、Big / Small 选择、下注金额、余额和幂等键
→ 服务端接受下注，并创建唯一 round_id
→ 下注金额与选择被锁定
→ 服务端通过 Provably Fair 数据确定三颗骰子
→ 服务端完成结果判定、派彩和账务结算
→ 页面执行可跳过的 Reveal Sequence
→ 页面展示骰子、总点数、胜负、派彩、净变化与更新后余额
→ 用户可以再次点击普通 Roll、查看 Round Detail 或离开游戏
```

## 139.7 Round 状态语义

页面至少表达以下状态：

READY
- 等待用户选择 Big / Small 与确认下注金额。

SUBMITTING
- 请求正在提交；
- 禁止重复点击；
- 不允许修改当前提交参数。

BET_ACCEPTED
- 服务端已经接受下注；
- 下注金额和 Big / Small 选择被锁定；
- 不得再次扣款。

RESOLVING
- 服务端正在生成或读取确定性结果；
- 前端不得自行生成骰子结果。

REVEALING
- 结果和结算已经锁定；
- 前端只负责表现；
- 用户可以跳过 Reveal。

SETTLED
- 派彩与余额更新完成；
- 同一 Round 不得再次结算或派奖。

RECOVERING
- 页面刷新、断线或网络异常后查询并恢复原 Round。

CANCELLED / REFUNDED
- 无法完成正常结算时的正式取消或退款状态。

内部状态枚举可以在技术设计阶段统一，但用户界面必须表达等价语义。

## 139.8 结果判定

输入：
- player_choice：BIG 或 SMALL；
- d1、d2、d3：三颗骰子结果；
- bet_amount：下注金额。

处理：

1. 判断 d1、d2、d3 是否全部相等；
2. 如为三同号，BIG 与 SMALL 均失败；
3. 如非三同号，计算 total = d1 + d2 + d3；
4. total 为 4–10 时，结果为 SMALL；
5. total 为 11–17 时，结果为 BIG；
6. 玩家选择与结果一致则获胜，否则失败。

派奖：

获胜：
- total_payout = bet_amount × 2；
- net_change = +bet_amount。

失败：
- total_payout = 0；
- net_change = -bet_amount。

## 139.9 页面信息结构

Dice Game Entry
```text
│
├── Game Header
│   ├── 返回 Game Catalog / Entertainment
│   ├── 游戏名称
│   ├── 运行状态
│   ├── Available Chips
│   └── Rules / Provably Fair / History 入口
│
├── Game Stage
│   ├── 三颗骰子展示区域
│   ├── 总点数
│   └── 当前结果状态
│
├── Bet Choice
│   ├── Big
│   └── Small
│
├── Wager Area
│   ├── 自定义整数金额
│   └── 10 / 100 / 500 / 1000
│
├── Primary Action
│   └── Roll
│
├── Current Round State
├── Result Summary
└── Rules / Transparency / Provably Fair / Recent History
```

## 139.10 结果展示内容

每局结算后至少展示：

- 三颗骰子结果；
- 总点数；
- 是否为三同号；
- 玩家选择 Big / Small；
- 下注金额；
- 胜 / 负；
- 总派彩；
- 净变化；
- 更新后的 Available Chips；
- Round ID；
- 查看 Round Detail；
- 查看 Provably Fair。

动画、光效和音效不得：
- 决定结果；
- 改变结果；
- 改变派彩；
- 改变 Round 状态；
- 阻止用户最终看到已经结算的结果。

## 139.11 Reveal 与动画行为

Reveal 动画允许跳过。

跳过动画：
- 只结束表现；
- 不改变服务端结果；
- 不改变派彩；
- 不创建新 Round；
- 不重新结算。

逐个揭示还是同时揭示：
- 不在 IA-07 中锁死；
- 留到 Art Direction v0.4 决定。

必须支持：
- 低性能设备降级；
- prefers-reduced-motion；
- 动画中断后恢复最终结果；
- 跳过后立即显示完整静态结果。

## 139.12 Provably Fair

每局至少绑定：

- server_seed_hash；
- server_seed（按照既定 Reveal 规则公开）；
- client_seed；
- nonce；
- round_id；
- algorithm_version；
- game_config_version；
- game_config_hash。

三颗骰子必须由同一 Round 的确定性随机过程生成。

将哈希输出映射至 1–6 时：
- 必须避免 modulo bias；
- 可以使用 rejection sampling 或其他经过验证的无偏映射方法。

Client Seed：
- 默认由系统生成；
- 用户可以在 Provably Fair 面板查看；
- 用户可以修改下一局 Client Seed；
- 已经 BET_ACCEPTED 的 Round 不允许修改 Client Seed；
- Client Seed 不要求在主游戏区手动填写。

Round Detail 必须能够验证：

同一组 Seed + Nonce + Algorithm Version
→ 同一组三颗骰子
→ 同一个 Big / Small / Triple 结果
→ 同一个派彩结果

## 139.13 刷新、断线与幂等

如果服务端尚未接受下注：
- 不产生正式资产变化；
- 用户可以重新提交。

如果服务端已经接受下注：
- 页面刷新不得重新扣款；
- 网络超时不得新建另一局；
- 重新进入 /games/dice 时查询并恢复原 round_id；
- 已结算时返回同一结果；
- 处理中时显示 RECOVERING；
- 已退款时显示 REFUNDED。

SETTLED Round：
- 不得再次结算；
- 不得再次派奖；
- 不得因重复请求产生第二条资产变化。

## 139.14 维护行为

游戏进入维护状态后：
- 阻止创建新的 Dice Round。

已经 BET_ACCEPTED 的 Round：
- 优先完成同一确定性结果和结算；
- 不能因为维护切换直接遗弃；
- 只有服务端无法获得确定性结果或无法完成正式结算时，才进入正式退款流程。

V1 不采用“一进入维护就将所有已接受 Round 全部退款”的规则。

## 139.15 余额不足

当 Available Chips < 10：
- 无法创建有效付费 Round；
- Roll 不可执行；
- 显示最低下注为 10；
- 提供 Wallet 入口；
- 提供 Rewards Center 入口。

从 Wallet / Rewards Center 返回后：
- 刷新余额；
- 恢复 Dice 页面；
- 可以保留尚未提交的 Big / Small 选择；
- 可以保留尚未提交的下注金额；
- 不自动 Roll；
- 不自动重放此前失败的下注。

## 139.16 Free Round 与扩展玩法边界

V1 不启用 Dice 专属 Free Round。

在全平台 Free Round 的名义下注额、派奖基准、历史记录和排行榜统计规则统一前，不接入 Dice 免费局。

V1 不提供：
- Auto Roll；
- 独立 Repeat Bet；
- 一键直接重下注；
- Double or Nothing；
- 赢后翻倍；
- 随机 Jackpot；
- 额外倍率事件；
- Triple 单独下注；
- 其他骰宝下注区域。

如未来增加 Jackpot、活动倍率或新的下注区域：
- 必须创建新的配置版本；
- 必须重新计算概率与 RTP；
- 必须更新公开规则；
- 历史 Round 继续保留旧规则。

## 139.17 Game History / Round Detail

Dice Round Detail 至少包含：

- Round ID；
- 创建时间；
- 接受下注时间；
- 结算时间；
- 玩家选择；
- 下注金额；
- 三颗骰子；
- 总点数；
- 是否三同号；
- 胜负；
- 总派彩；
- 净变化；
- 结算前余额；
- 结算后余额；
- 固定 1:1 赔率；
- 理论 RTP；
- algorithm_version；
- game_config_version；
- game_config_hash；
- Provably Fair 验证数据；
- Cancelled / Refunded 信息（如适用）。

## 139.18 PC 与 Mobile

PC：
- 使用 Focused Game Layout；
- Game Stage、Big / Small、下注区和 Roll 保持在同一视野内可理解；
- Rules、History 与 Provably Fair 可以使用 Drawer / Panel；
- 完整概率、赔率、RTP 与三同号规则始终可访问。

Mobile：
- 使用单列或紧凑分区；
- Big / Small 必须适合单手点击；
- 四个快捷金额必须清晰可区分；
- 余额不足的快捷按钮直接禁用；
- 结果区不能被 Bottom Navigation 遮挡；
- Round Detail 使用全屏详情层；
- 低性能和 reduced-motion 环境仍须显示完整静态结果。

## 139.19 Presentation Hooks

IA-07 只冻结以下演出能力：

- 三颗骰子的开始前状态；
- Roll 过程；
- 结果已经锁定后的 Reveal；
- 总点数显示；
- Big / Small 结果强调；
- Triple 特殊结果强调；
- Win / Loss 反馈；
- 可跳过；
- reduced-motion 静态结果；
- 动画中断后恢复最终结果。

留到 Art Direction v0.4：

- 骰子具体材质；
- 骰子颜色；
- FGO 角色或职阶映射；
- 背景；
- 光效；
- 粒子；
- 按钮视觉；
- 音效；
- 逐个或同时揭示；
- 镜头运动；
- 动画具体时长。

# 140. Scratch Card / 刮刮乐

## 140.1 游戏身份

展示名称：刮刮乐 / Scratch Card
稳定 game_slug：scratch
逻辑入口：/games/scratch
Entry Type：Direct Play
主要交互能力：Reveal Sequence
资金模型：娱乐主钱包付费 Round
页面 Shell：Focused Game Layout

## 140.2 玩法方向

V1 采用经典的：

“九格刮开，任意位置出现三个相同奖级符号，即获得对应倍数派彩”

玩法。

每张刮刮乐包含 3 × 3，共 9 个覆盖区域。

玩家购买一张卡后：
- PC 使用鼠标左键按住并拖动刮开覆盖层；
- 触屏设备使用手指在卡面上滑动刮开覆盖层；
- 触控板如能产生等价的按住拖动指针事件，可以按同一 Pointer Drag 逻辑工作；
- 可以按照任意顺序揭示九个区域；
- 可以使用 Reveal All / 全部揭晓跳过手动刮开。

结果在服务端接受购买时已经锁定。
刮开行为只负责揭示结果，不决定结果。

## 140.3 中奖结构

Winning Card / 中奖卡：
- 恰好存在一个中奖三连组；
- 某个奖级符号在九格中恰好出现 3 次；
- 三个相同符号可以出现在任意位置；
- 不要求横向、纵向或斜线相连；
- 剩余六格不得再组成第二个三同符号；
- 每张卡最多获得一个奖级。

Losing Card / 未中奖卡：
- 九格中不存在任何出现 3 次或以上的相同奖级符号；
- 不产生派彩。

V1 不生成：
- 一张卡多组中奖；
- 奖项叠加；
- 连线叠加；
- 四个或更多相同符号带来的额外奖项；
- Bonus Symbol；
- Wild Symbol；
- 二次抽奖入口。

## 140.4 默认奖表

V1 默认采用以下总派彩倍数与概率：

| 结果级别 | 总派彩倍数 | 概率 | 100,000 权重 |
|----------|------------|------|--------------|
| 未中奖   | 0x         | 54.000% | 54,000 |
| 回本     | 1x         | 19.500% | 19,500 |
| 奖级 2  | 2x         | 18.500% | 18,500 |
| 奖级 3  | 3x         | 5.000%  | 5,000 |
| 奖级 5  | 5x         | 2.000%  | 2,000 |
| 奖级 10 | 10x        | 0.800%  | 800 |
| 奖级 25 | 25x        | 0.180%  | 180 |
| 最高奖   | 100x       | 0.020%  | 20 |

概率总和：
100%

理论 RTP：

0 × 54.000%
+ 1 × 19.500%
+ 2 × 18.500%
+ 3 × 5.000%
+ 5 × 2.000%
+ 10 × 0.800%
+ 25 × 0.180%
+ 100 × 0.020%

= 96.000%

对应：

全额损失概率：
54.000%

回本概率：
19.500%

净盈利概率：
26.500%

任意非零派彩概率：
46.000%

最高 100x 概率：
0.020%
即平均约 1 / 5,000 张。

## 140.5 派彩倍数含义

所有倍数均表示：

“包含本金的总派彩倍数”

例如下注 100：

0x：
- total_payout = 0；
- net_change = -100。

1x：
- total_payout = 100；
- net_change = 0；
- 结果标记为 Break-even / 回本。

2x：
- total_payout = 200；
- net_change = +100。

10x：
- total_payout = 1,000；
- net_change = +900。

100x：
- total_payout = 10,000；
- net_change = +9,900。

1x 不计为游戏净盈利，也不计为单局正收益。
其业务结果为回本，而不是正盈利。

## 140.6 结果生成原则

刮刮乐不能通过九个符号彼此独立随机后，再临时判断是否中奖。

采用以下确定性过程：

1. 使用 Provably Fair 随机输出选择 Prize Tier；
2. 根据已锁定的 Prize Tier 生成符合规则的九格功能布局；
3. 对九格位置进行确定性洗牌；
4. 绑定 algorithm_version、game_config_version 与 game_config_hash；
5. 服务端完成派彩结算；
6. 前端只负责揭示已经锁定的九格。

中奖卡：
- 生成一个且仅一个三同奖级；
- 其余符号不得再形成三同。

未中奖卡：
- 任意符号最多出现 2 次；
- 不得意外形成中奖组合。

符号位置洗牌同样必须来自该 Round 的确定性随机过程。

## 140.7 全局下注规则

本游戏继承已确认的 Direct Play 全局下注规则：

最低下注：
10 娱乐筹码

产品层固定最高下注：
不设置

固定快捷金额：
10 / 100 / 500 / 1000

下注金额：
- 只接受整数娱乐筹码；
- 不得超过 Available Chips；
- 不得超过服务端安全数值范围。

第一次进入 Scratch：
- 默认下注金额为 10。

一张卡完成后：
- 保留上一张 Scratch Card 的下注金额；
- Scratch 金额记忆只属于 Scratch；
- 不与 Dice、Slot、Blackjack 或其他游戏共享。

快捷金额高于 Available Chips 时：
- 对应按钮禁用。

V1 不提供：
- Max；
- 50%；
- 百分比下注；
- All-in。

## 140.8 单局用户流程

```text
进入 /games/scratch
→ 查看 Available Chips、最低下注、规则与奖表摘要
→ 输入自定义整数金额，或点击 10 / 100 / 500 / 1000
→ 点击 Buy Card / 购买刮刮乐
→ 前端进入 SUBMITTING，并锁定重复购买
→ 服务端校验登录、游戏状态、下注金额、余额和幂等键
→ 服务端接受下注，创建唯一 round_id
→ 服务端通过 Provably Fair 选择 Prize Tier 并生成九格布局
→ 服务端完成派彩与账务结算
→ 页面进入 SCRATCHING / REVEALING
→ 用户手动刮开九格，或点击 Reveal All
→ 全部揭示后显示中奖级别、派彩、净变化和更新后余额
→ 用户可以购买下一张、查看 Round Detail 或离开游戏
```

## 140.9 Round 状态语义

READY
- 等待用户确认下注金额并购买新卡。

SUBMITTING
- 购买请求正在提交；
- 禁止重复点击。

BET_ACCEPTED
- 服务端已经接受下注；
- 下注金额被锁定；
- 不允许取消或修改该卡。

RESOLVING
- 服务端正在生成或读取确定性 Prize Tier 与九格布局。

SCRATCHING / REVEALING
- 结果已经锁定并完成正式结算；
- 前端只负责揭示九格。

SETTLED
- 派彩与余额已经完成；
- 结果摘要已经可展示。

RECOVERING
- 页面刷新、断线或网络异常后恢复同一 round_id 和同一张卡。

CANCELLED / REFUNDED
- 无法完成正式结果或结算时的取消 / 退款状态。

## 140.10 结算与揭示时点

已确认规则：

服务端接受购买后：
- 立即锁定结果；
- 立即完成正式派彩结算；
- Scratch 动画不控制结算。

手动刮开或 Reveal All：
- 只控制何时在当前页面显示完整结果；
- 不改变钱包；
- 不改变派彩；
- 不改变 Round。

为避免动画提前剧透：
- 当前游戏页面可以在完整揭示前暂缓刷新“本局结果摘要”；
- 完整揭示后再显示本局派彩与净变化；
- 但服务端真实余额与 ledger 已经完成更新。

该反剧透表现不构成安全边界。
用户通过其他页面、其他标签页或开发者工具可能提前看到已结算后的余额变化，但不能改变结果或重复获利。

## 140.11 手动刮开行为

九格允许按照任意顺序揭示。

逐格手动揭示只支持真实的按住拖动刮开操作：

PC / 鼠标：
- 用户必须按住鼠标左键；
- 在覆盖层上连续拖动；
- 只有按住并移动时才产生刮痕；
- 单击某一格不会直接揭示该格。

Mobile / 触屏：
- 用户使用手指在刮奖区域内连续滑动；
- 手指移动轨迹产生刮痕；
- 单次点按某一格不会直接揭示该格。

触控板：
- 如果设备能够产生等价的按住主按键并拖动指针事件，可沿用同一 Pointer Drag 逻辑；
- 不为触控板单独提供点击揭示模式。

V1 不提供：
- 点击单格直接揭示；
- 键盘逐格揭示；
- 通过方向键逐格移动并揭示；
- 自动逐格揭示。

每个格子达到既定刮开覆盖比例后，视为该格已经揭示。

具体覆盖比例、笔刷尺寸、遮罩算法、粒子与刮痕材质属于后续技术设计与 Art Direction，不在 IA-07 写死。

手机端：
- 仅在用户手指实际落于刮奖区域并开始刮动时抑制该区域的页面滚动；
- 离开刮奖区域后页面仍可正常滚动；
- 不得因为触摸事件冲突导致整页无法操作。

可访问性边界：
- 选择 16B 后，V1 不提供逐格键盘或点击式替代；
- Reveal All 仍作为整张卡的非拖动快速揭晓入口保留；
- Reveal All 不等于逐格可访问模式，也不改变已经锁定的结果；
- 这是已经接受的 V1 产品取舍，不应在实现阶段被误写成“支持完整键盘逐格操作”。

## 140.12 Reveal All

V1 提供 Reveal All / 全部揭晓。

Reveal All：
- 在购买成功后即可使用；
- 不需要二次确认；
- 立即揭示剩余全部格子；
- 不创建新 Round；
- 不产生第二次扣款；
- 不改变服务端已经锁定的结果；
- 承担快速跳过刮奖表现的功能。

Reveal All 不等于：
- Auto Buy；
- Repeat Purchase；
- 自动购买下一张。

## 140.13 结果摘要显示

在以下任一条件成立时显示完整 Result Summary：

1. 九个格子全部揭示；
2. 用户点击 Reveal All。

即使用户较早刮出了三个相同符号：
- 可以即时强调匹配；
- 但完整结果摘要仍在全部揭示后显示。

每局结果至少展示：

- 九格完整布局；
- 匹配的中奖符号；
- 中奖倍数；
- 下注金额；
- 总派彩；
- 净变化；
- 更新后的 Available Chips；
- Round ID；
- 查看 Round Detail；
- 查看 Provably Fair。

## 140.14 下一张卡与批量购买

V1 每个 Round 只购买一张卡。

一张卡未完成揭示时：
- 不允许购买下一张；
- 用户可以通过 Reveal All 快速完成当前卡。

完整结果显示后：
- 主操作变为 New Card / 购买下一张；
- 保留上一次 Scratch 下注金额；
- 用户必须再次主动点击；
- 不自动购买。

V1 不提供：
- 一次购买 5 张；
- 一次购买 10 张；
- 批量刮开；
- Auto Buy；
- 无限连续购买；
- 一键直接重复扣款。

## 140.15 页面刷新与断线恢复

如果服务端尚未接受下注：
- 不产生正式资产变化；
- 用户可以重新购买。

如果服务端已经接受下注：
- 页面刷新不得重新扣款；
- 网络超时不得创建第二张卡；
- 重新进入 /games/scratch 时恢复同一 round_id；
- 恢复同一个 Prize Tier；
- 恢复同一九格布局；
- 已结算时返回同一派彩；
- 已退款时显示正式退款状态。

V1 不要求服务端保存像素级刮痕。

刷新后的表现：
- 必须恢复同一张卡和同一 Round；
- 已完整揭示的卡保持结果已完成；
- 未完整揭示时，可以重新显示覆盖层；
- 页面明确提示“已恢复未完成揭示的卡”；
- 用户可以重新刮开或使用 Reveal All；
- 不能因此获得第二次派彩。

## 140.16 Provably Fair

每局至少绑定：

- server_seed_hash；
- server_seed（按照既定 Reveal 规则公开）；
- client_seed；
- nonce；
- round_id；
- algorithm_version；
- game_config_version；
- game_config_hash。

Provably Fair 必须验证：

同一组 Seed + Nonce + Algorithm Version + Config Version
→ 同一个 Prize Tier
→ 同一组九格功能符号
→ 同一组格子位置
→ 同一个总派彩
→ 同一个净变化

奖级选择必须避免 modulo bias。

九格位置洗牌也必须使用确定性、可验证且无明显偏差的过程。

Client Seed：
- 默认由系统生成；
- 可以在 Provably Fair 面板查看；
- 可以修改下一张卡的 Client Seed；
- 已经 BET_ACCEPTED 的卡不可修改。

## 140.17 奖表透明度

V1 始终公开：

- 当前奖级；
- 每个奖级的总派彩倍数；
- 每个奖级概率；
- 理论 RTP；
- 全额损失概率；
- 回本概率；
- 净盈利概率；
- 最高奖概率；
- 三同匹配规则；
- 每张卡最多一个奖项的规则。

奖表不受通用透明度开关隐藏。

管理员发布新奖表版本时：
- 只能影响之后创建的卡；
- 历史卡保留当时的真实奖表；
- 当前公开页面同步显示生效版本。

## 140.18 运营配置与版本化

V1 提供一个默认 96% RTP 奖表。

管理员可以：
- 创建新的奖表草稿；
- 调整各奖级概率；
- 调整可用总派彩倍数；
- 预览概率总和与理论 RTP；
- 发布新的版本；
- 停用旧版本用于新 Round。

管理员不可以：
- 修改已经发布并被历史 Round 引用的版本；
- 修改已经购买卡片的 Prize Tier；
- 修改已经结算的派彩；
- 在不更新版本的情况下改变概率；
- 隐藏当前生效奖表的真实 RTP。

每张卡创建时锁定：
- game_config_version；
- prize_table_version；
- game_config_hash。

## 140.19 1x 回本的统计语义

1x 结果：
- total_payout = bet_amount；
- net_change = 0；
- 业务状态显示为 Break-even / 回本。

1x：
- 可以计入“产生派彩的卡片数量”；
- 不计入正盈利局数；
- 不增加今日盈利；
- 不增加历史净盈利；
- 不进入大额中奖或最高净盈利记录；
- 下注金额仍计入累计下注榜。

## 140.20 维护行为

维护状态：
- 阻止购买新卡。

已经 BET_ACCEPTED 的卡：
- 优先恢复并完成同一确定性结果；
- 允许用户继续揭示；
- 如果结果和结算已经完成，不得退款后再派奖；
- 只有服务端无法得到确定性结果或无法完成正式结算时，才进入正式退款流程。

维护切换不得遗弃已经购买的卡。

## 140.21 余额不足

当 Available Chips < 10：
- 无法购买付费卡；
- 显示最低下注为 10；
- 提供 Wallet；
- 提供 Rewards Center。

从 Wallet / Rewards Center 返回后：
- 刷新余额；
- 恢复 Scratch 页面；
- 保留尚未提交的下注金额；
- 不自动购买新卡。

## 140.22 Game History / Round Detail

Scratch Round Detail 至少包含：

- Round ID；
- 创建时间；
- 接受下注时间；
- 结算时间；
- 下注金额；
- Prize Tier；
- 总派彩倍数；
- 九格功能布局；
- 匹配符号；
- 总派彩；
- 净变化；
- 结算前余额；
- 结算后余额；
- prize_table_version；
- algorithm_version；
- game_config_version；
- game_config_hash；
- Provably Fair 验证数据；
- Cancelled / Refunded 信息（如适用）。

像素级刮痕不属于正式历史资产数据。

## 140.23 PC 与 Mobile

PC：
- 使用 Focused Game Layout；
- 九格卡片、下注区与购买操作保持清楚；
- Rules、Prize Table、History 与 Provably Fair 可使用 Drawer / Panel；
- 鼠标左键按住并拖动刮开必须流畅；
- 单击某格不得直接揭示该格；
- Reveal All 始终可发现。

Mobile：
- 九格必须适合手指连续滑动；
- 刮奖区域内正确处理触摸与页面滚动冲突；
- 页面其余区域仍可正常滚动；
- 单次点按某格不得直接揭示该格；
- 四个快捷金额不得拥挤；
- Reveal All 必须易于点击；
- Round Detail 使用全屏详情层；
- V1 不提供点击单格或键盘逐格揭示；
- reduced-motion 下仍可通过手动刮开或 Reveal All 查看完整静态结果。

## 140.24 Presentation Hooks

IA-07 只冻结以下演出能力：

- 未购买状态；
- 新卡出现；
- 覆盖层；
- 手动刮开；
- 单格揭示；
- Reveal All；
- 三同匹配强调；
- Break-even；
- 普通中奖；
- 高倍中奖；
- 100x 最高奖；
- 失败反馈；
- reduced-motion 静态揭示；
- 动画中断后恢复同一卡片结果。

留到 Art Direction v0.4：

- 卡片形状；
- 覆盖层材质；
- FGO 主题映射；
- 奖级符号图案；
- 角色；
- 背景；
- 粒子；
- 光效；
- 音效；
- 100x 演出；
- 具体动画分镜与时长。

## 140.25 V1 暂不包含

V1 暂不包含：

- 多奖项叠加；
- 一张卡多个中奖三连；
- 4 个或更多相同符号特殊奖；
- 连线奖；
- Wild；
- Bonus Symbol；
- 二次抽奖；
- Jackpot；
- 累积奖池；
- Double or Nothing；
- Free Round；
- 批量购卡；
- Auto Buy；
- 实物奖品；
- 收藏品；
- 卡片交易。

# 141. Summon / Gacha / 扭蛋机

## 141.1 游戏身份

展示名称：
Summon / Gacha / 扭蛋机

稳定 game_slug：
summon

逻辑入口：
/games/summon

Entry Type：
Direct Play

主要交互能力：
Reveal Sequence

资金模型：
娱乐主钱包付费 Round

页面 Shell：
Focused Game Layout

## 141.2 产品定位

Summon / Gacha V1 采用：

“纯娱乐筹码抽取”

每次抽取只产生：
- 逻辑 Reward Tier；
- 对应娱乐筹码派彩；
- 本 Round 的结果与公平验证数据。

V1 不产生：
- Servant 所有权；
- 礼装；
- 卡牌背包；
- 灵基图鉴；
- 收藏品；
- 养成材料；
- 玩家交易物；
- 可出售物品；
- 实物奖励。

页面可以使用高度 FGO 化的召唤表现，但视觉对象不构成玩家永久资产。

## 141.3 逻辑奖级与美术分离

每次抽取使用稳定逻辑奖级：

T0
T1
T2
T3
T4
T5

每个逻辑奖级绑定：
- total_payout_multiplier；
- probability_weight；
- presentation_tier；
- prize_table_version。

具体美术可以在 Art Direction v0.4 映射为：
- 不同卡面；
- 不同光效；
- 不同召唤阵反馈；
- 不同稀有度称呼；
- 蓝光 / 金光 / 彩光；
- 其他 FGO 风格表现。

更换美术、名称或角色素材不得改变：
- 逻辑 Tier；
- 倍数；
- 概率；
- 历史结果；
- Round Detail；
- Provably Fair 验证。

## 141.4 抽取模式

V1 提供两种模式：

Single Summon / 单抽
- 产生 1 个抽取结果。

Tenfold Summon / 十连
- 产生 10 个抽取结果。

V1 不提供：
- 2 连；
- 5 连；
- 20 连；
- 100 连；
- 自定义抽取数量；
- 自动无限抽取。

## 141.5 全局下注与抽取成本

本游戏继承 Direct Play 全局下注规则。

基础下注 / Base Wager：
- 表示每一个 Draw 的基础娱乐筹码金额；
- 最低为 10；
- 产品层固定最高下注不设置；
- 只接受整数娱乐筹码；
- 不得超过服务端安全范围。

固定快捷金额：
10 / 100 / 500 / 1000

快捷按钮：
- 只填入 Base Wager；
- 不自动抽取；
- 不扣款；
- 不创建 Round。

Single Summon 总成本：

single_total_cost = base_wager

Tenfold Summon 总成本：

tenfold_total_cost = base_wager × 10

例如 Base Wager = 100：

Single Summon：
- 总成本 100。

Tenfold Summon：
- 总成本 1,000。

页面必须在正式提交前明确显示：
- Base Wager；
- Draw Count；
- Total Cost；
- 当前 Available Chips；
- 扣除 Total Cost 后、尚未计入未知派彩的最低预估余额。

## 141.6 默认奖表

V1 已确认默认奖表：

| 逻辑奖级 | 总派彩倍数 | 概率 | 100,000 权重 |
|----------|------------|------|--------------|
| T0       | 0x         | 59.850% | 59,850 |
| T1       | 1x         | 25.000% | 25,000 |
| T2       | 2x         | 10.000% | 10,000 |
| T3       | 5x         | 4.000%  | 4,000 |
| T4       | 20x        | 1.050%  | 1,050 |
| T5       | 100x       | 0.100%  | 100 |

概率总和：
100%

理论 RTP：

0 × 59.850%
+ 1 × 25.000%
+ 2 × 10.000%
+ 5 × 4.000%
+ 20 × 1.050%
+ 100 × 0.100%

= 96.000%

对应：

全额损失概率：
59.850%

单次回本概率：
25.000%

单次净盈利概率：
15.150%

单次 20x 或以上概率：
1.150%

单次 100x 概率：
0.100%
即平均约 1 / 1,000 Draw。

## 141.7 派彩倍数含义

所有倍数表示：

“相对于该 Draw Base Wager、包含本金的总派彩倍数”

例如 Base Wager = 100：

T0 / 0x：
- payout = 0；
- net_change_for_draw = -100。

T1 / 1x：
- payout = 100；
- net_change_for_draw = 0；
- 显示为 Break-even / 回本。

T2 / 2x：
- payout = 200；
- net_change_for_draw = +100。

T4 / 20x：
- payout = 2,000；
- net_change_for_draw = +1,900。

T5 / 100x：
- payout = 10,000；
- net_change_for_draw = +9,900。

## 141.8 Single Summon 数学口径

Single Summon：
- 总下注 = base_wager；
- 总派彩 = 单个 Draw 的 payout；
- Round 净变化 = total_payout - base_wager；
- 理论 RTP = 96%。

Single Round 结果分类：

Loss：
- 0x；
- Round 净变化小于 0。

Break-even：
- 1x；
- Round 净变化等于 0。

Win：
- 2x / 5x / 20x / 100x；
- Round 净变化大于 0。

## 141.9 Tenfold Summon 规则

V1 已确认采用：

“10 个彼此独立、使用同一奖表的 Draw”

Tenfold：
- 不打折；
- 不附加额外 Draw；
- 不保证 T1；
- 不保证 T2 或以上；
- 不保证高稀有度；
- 不使用保底；
- 不使用 Pity；
- 不重新抽取最低结果；
- 不修改任何单抽概率。

每个 Draw 使用：
- 同一个 prize_table_version；
- 独立确定性随机取样；
- 固定 draw_index：1–10。

Tenfold 总派彩：

tenfold_total_payout
=
sum(draw_1_payout ... draw_10_payout)

Tenfold 净变化：

tenfold_net_change
=
tenfold_total_payout - base_wager × 10

由于 10 个 Draw 使用同一 96% RTP 奖表：

Tenfold 理论 RTP：
96%

当前奖表下，十连中：

至少一个非 0x 结果的概率：
约 99.410286%

至少一个净盈利奖级（2x 或以上）的概率：
约 80.657239%

至少一个 20x 或以上奖级的概率：
约 10.922763%

至少一个 100x 奖级的概率：
约 0.995512%

以上只是组合概率，不是保底承诺。

## 141.10 Round 与 Sub-result 结构

Single Summon：
- 一个 round_id；
- draw_count = 1；
- 一个 draw_index = 1；
- 一个抽取子结果。

Tenfold Summon：
- 一个 round_id；
- draw_count = 10；
- draw_index = 1–10；
- 十个抽取子结果。

Tenfold 不拆成十个彼此独立的可重复扣款 Round。

服务端必须：
- 一次校验 Tenfold 总成本；
- 一次原子扣除总成本；
- 生成十个确定性子结果；
- 计算总派彩；
- 一次完成正式结算；
- 保存每个 draw_index 的逻辑 Tier、倍数和派彩；
- 保存 Round 汇总结果。

## 141.11 结果生成原则

服务端接受抽取后：

1. 锁定 summon_mode；
2. 锁定 base_wager；
3. 计算并锁定 total_cost；
4. 锁定 prize_table_version；
5. 锁定 game_config_version；
6. 通过 Provably Fair 生成 1 个或 10 个 Draw；
7. 计算每个 Draw 的 Tier 与派彩；
8. 计算 Round 总派彩与净变化；
9. 完成钱包与 ledger 结算；
10. 前端开始召唤和揭示表现。

前端动画：
- 不选择 Tier；
- 不改变概率；
- 不决定奖励；
- 不重新抽取；
- 不延迟或重复结算。

## 141.12 Single Summon 用户流程

```text
进入 /games/summon
→ 默认处于 Single Summon
→ 查看 Available Chips、奖表和规则摘要
→ Base Wager 默认 10，或恢复上一次 Summon 有效金额
→ 输入整数金额，或点击 10 / 100 / 500 / 1000
→ 页面显示 Total Cost = Base Wager
→ 点击 Summon
→ 前端进入 SUBMITTING，锁定重复操作
→ 服务端校验登录、运行状态、金额、余额、配置版本和幂等键
→ 服务端接受下注并创建唯一 round_id
→ 服务端生成 Draw、完成派彩和账务结算
→ 页面进入 SUMMONING / REVEALING
→ 用户观看或跳过表现
→ 页面显示 Tier、倍数、派彩、净变化和更新后余额
→ 用户可以再次主动 Summon、切换 Tenfold、查看 Round Detail 或离开
```

## 141.13 Tenfold Summon 用户流程

```text
进入 /games/summon
→ 选择 Tenfold Summon
→ 查看 Base Wager 与 Total Cost = Base Wager × 10
→ 输入整数金额，或点击 10 / 100 / 500 / 1000
→ 页面实时更新十连总成本，以及未计派彩时的扣款后余额
→ 点击 Tenfold Summon
→ 前端进入 SUBMITTING，锁定重复操作
→ 服务端一次校验总成本与 Available Chips
→ 服务端一次扣除总成本并创建一个 round_id
→ 服务端生成 draw_index 1–10 的十个确定性子结果
→ 服务端完成总派彩和账务结算
→ 页面执行十连召唤表现
→ 结果按照 draw_index 顺序揭示
→ 用户可以加速或 Skip / Reveal All
→ 页面显示十个结果和 Round 汇总
→ 用户可以再次主动抽取、查看 Round Detail 或离开
```

## 141.14 业务状态与表现状态分离

业务 Round 状态至少表达：

READY
- 等待用户选择模式和下注金额。

SUBMITTING
- 请求正在提交；
- 禁止重复点击。

BET_ACCEPTED
- 服务端已经接受总下注；
- 模式、金额和 Draw Count 被锁定。

RESOLVING
- 服务端正在生成或读取确定性 Draw 结果。

SETTLED
- 总派彩、净变化与余额更新已经完成；
- 同一 Round 不得再次结算。

RECOVERING
- 刷新、断线或网络异常后恢复原 Round。

CANCELLED / REFUNDED
- 无法完成正式结果或结算时的取消 / 退款状态。

前端 Presentation 状态可以另外表达：

PREPARING
SUMMONING
REVEALING
SUMMARY

Presentation 状态不得反向改变已经完成的业务结算。

## 141.15 页面信息结构

Summon Game Entry
```text
│
├── Game Header
│   ├── 返回 Game Catalog / Entertainment
│   ├── 游戏名称与运行状态
│   ├── Available Chips
│   └── Rules / Rates / Provably Fair / History
│
├── Summon Mode
│   ├── Single
│   └── Tenfold
│
├── Summon Stage
│   ├── 未抽取状态
│   ├── Summoning
│   ├── Reveal Sequence
│   └── Result Presentation
│
├── Wager Area
│   ├── Base Wager
│   ├── 10 / 100 / 500 / 1000
│   ├── Draw Count
│   ├── Total Cost
│   └── 预计剩余 Available Chips
│
├── Primary Action
│   ├── Single Summon
│   └── Tenfold Summon
│
├── Reveal Controls
│   ├── Accelerate
│   └── Skip / Reveal All
│
├── Result Summary
└── Rules / Prize Table / Provably Fair / Recent History
```

## 141.16 Reveal Sequence

已确认：

Single：
- 进行一次召唤表现；
- 揭示一个结果；
- 显示 Single Result Summary。

Tenfold：
- 按 draw_index 1–10 自动顺序揭示；
- 不要求玩家手动点击每一个结果；
- 用户可以通过点击 / 触摸加速当前表现；
- 用户可以使用 Skip / Reveal All 立即进入完整十连结果；
- 精确镜头、光效、停顿时间和卡面样式留到 Art Direction v0.4。

Skip / Reveal All：
- 不创建新 Round；
- 不改变任何 Tier；
- 不改变派彩；
- 不改变概率；
- 不产生第二次扣款；
- 不产生第二次派奖；
- 只结束或跳过表现。

## 141.17 Result Summary

Single Result Summary 至少显示：

- Reward Tier；
- 总派彩倍数；
- Base Wager；
- 总派彩；
- Round 净变化；
- 更新后的 Available Chips；
- Round ID；
- 查看 Provably Fair；
- 查看 Round Detail。

Tenfold Result Summary 至少显示：

- 十个结果；
- 每个 draw_index；
- 每个 Reward Tier；
- 每个总派彩倍数；
- 每个 Draw 派彩；
- Base Wager；
- Draw Count = 10；
- Total Cost；
- Total Payout；
- Round Net Change；
- 最高 Reward Tier；
- 更新后的 Available Chips；
- Round ID；
- 查看 Provably Fair；
- 查看 Round Detail。

Tenfold 的 Win / Loss / Break-even 按整个 Round 汇总净变化判断。

## 141.18 下一次抽取

当前 Round 尚未进入 Summary 时：
- 不允许开始下一次抽取；
- 用户可以使用 Skip / Reveal All 快速结束表现。

进入 Summary 后：
- 重新开放下注与模式控件；
- 保留上一次 Summon Mode；
- 保留上一次 Summon Base Wager；
- 用户必须再次主动点击 Summon；
- 不自动创建下一 Round；
- 不自动扣款。

V1 不提供：
- 一键直接 Repeat Summon；
- Auto Summon；
- 无限连续抽取。

## 141.19 下注记忆与按钮状态

第一次进入 Summon：
- 默认 Single；
- Base Wager 默认 10。

后续进入：
- 恢复用户上一次 Summon Mode；
- 恢复用户上一次 Summon Base Wager；
- 只在 Summon 游戏内记忆；
- 不与 Dice、Scratch、Slot 或 Blackjack 共享。

快捷按钮状态：

Single：
- 当快捷金额高于 Available Chips 时，对应按钮禁用。

Tenfold：
- 当快捷金额 × 10 高于 Available Chips 时，对应按钮在 Tenfold 模式下禁用。

例如 Available Chips = 3,000：

Single：
- 10 / 100 / 500 / 1000 均可用。

Tenfold：
- 10 / 100 可用；
- 500 / 1000 禁用。

自定义输入仍可输入不超过对应模式总成本上限的合法整数 Base Wager。

V1 不提供：
- Max；
- 50%；
- 百分比按钮；
- All-in。

## 141.20 余额不足

Single：
- Available Chips < 10 时无法发起。

Tenfold：
- Available Chips < 100 时，即使 Base Wager 最低为 10，也无法发起十连。

页面提供：
- Wallet；
- Rewards Center；
- 当前最低所需总成本；
- 当前可用余额。

从 Wallet / Rewards 返回后：
- 刷新 Available Chips；
- 恢复 Summon 页面；
- 可以保留尚未提交的模式与金额；
- 不自动抽取；
- 不重放之前失败的提交。

## 141.21 页面刷新、断线与恢复

服务端尚未接受抽取：
- 不产生正式资产变化；
- 用户可以重新提交。

服务端已经接受抽取：
- 页面刷新不得再次扣款；
- 网络超时不得创建另一个 Round；
- 必须恢复同一 round_id；
- 必须恢复同一 Single / Tenfold 模式；
- 必须恢复同一个 prize_table_version；
- 必须恢复相同 draw_index 与 Tier；
- 必须返回相同 Total Payout 与 Net Change。

V1 不要求保存召唤动画播放到哪一帧。

已确认恢复行为：
- 默认进入 Result Summary；
- 可以提供 Replay Presentation；
- Replay 只重播表现，不重新结算。

## 141.22 Provably Fair

每个 Round 至少绑定：

- server_seed_hash；
- server_seed（按既定 Reveal 规则公开）；
- client_seed；
- nonce；
- round_id；
- algorithm_version；
- game_config_version；
- game_config_hash；
- prize_table_version；
- summon_mode；
- draw_count。

Single：
- 使用 draw_index = 1。

Tenfold：
- 使用 draw_index = 1–10。

确定性验证应满足：

同一组 Seed + Nonce + Algorithm Version + Config Version + draw_index
→ 同一个随机样本
→ 同一个 Reward Tier
→ 同一个倍数
→ 同一个 Draw Payout

全部 draw_index 汇总
→ 同一个 Total Payout
→ 同一个 Round Net Change

奖级映射必须避免 modulo bias。
不得因为十连动画顺序、Skip 或页面刷新改变任何 Draw。

Client Seed：
- 默认由系统生成；
- 可以在 Provably Fair 面板查看；
- 可以修改下一 Round；
- 已经 BET_ACCEPTED 的 Round 不可修改。

## 141.23 奖表透明度

V1 已确认始终公开：

- 每个逻辑 Reward Tier；
- 每个 Tier 的总派彩倍数；
- 每个 Tier 的概率；
- 理论 RTP；
- 全额损失概率；
- 回本概率；
- 净盈利概率；
- 20x+ 概率；
- 100x 概率；
- Single 与 Tenfold 的规则；
- Tenfold 不存在保底或折扣；
- Tenfold 的组合概率说明。

管理员发布新奖表版本时：
- 只影响之后创建的 Round；
- 历史 Round 保留原奖表；
- 页面必须明确当前生效版本。

## 141.24 奖表、Pool 与版本化

V1 确认只提供一个功能性 Active Pool。

该 Pool 可以使用不同主题包装，但其逻辑必须绑定：

- stable pool_id；
- prize_table_version；
- game_config_version；
- 生效时间；
- 发布状态；
- 当前奖表；
- 当前 Tenfold 规则。

管理员可以：
- 创建新奖表草稿；
- 调整 Tier 概率；
- 调整 Tier 倍数；
- 预览总概率与理论 RTP；
- 发布新版本；
- 停止旧版本用于新 Round。

管理员不可以：
- 修改已发布并被历史 Round 引用的版本；
- 修改已接受 Round 的结果；
- 在不创建新版本时改变概率；
- 隐藏当前生效奖表的真实概率和 RTP。

V1 不提供多个并行功能性 Banner / Pool。

## 141.25 保底、Rate-up 与跨 Round 状态

V1 确认不提供：

- Tenfold 保底；
- 至少一个高稀有度保证；
- Pity Counter；
- N 抽必出；
- 歪 / 不歪；
- Rate-up；
- 指定目标奖励；
- 跨 Round 累积概率；
- 抽数继承；
- Pool 间保底转移。

理由：
- V1 没有 Servant 或收藏品目标；
- 结果只产生娱乐筹码；
- 引入保底会产生跨 Round 账户状态；
- 会改变理论 RTP；
- 会显著增加历史、配置和 Provably Fair 解释复杂度。

未来如增加保底：
- 必须单独设计；
- 必须版本化；
- 必须公开触发规则；
- 必须重新计算并公开 RTP；
- 必须明确是否跨 Pool 继承；
- 不得在不公开规则的情况下暗中调整。

## 141.26 Free Summon、Bonus 与 Jackpot

V1 确认不启用：

- Free Summon；
- 每日免费单抽；
- 首抽免费；
- Tenfold 赠送第 11 抽；
- 购买十次只扣九次；
- Bonus Draw；
- Jackpot；
- 累积奖池；
- Double or Nothing。

如果未来启用 Free Summon：
- 必须定义名义 Base Wager；
- 必须定义派奖基准；
- 必须定义排行榜和累计下注统计；
- 必须定义奖表与 RTP；
- 不能把 0 筹码下注直接当作普通付费 Round。

## 141.27 统计与排行榜语义

Single：
- 一个 Round；
- cumulative_wager 增加 base_wager；
- Round Net Change 按该 Draw 计算。

Tenfold：
- 一个 Round；
- cumulative_wager 增加 base_wager × 10；
- Round Net Change 使用十个 Draw 汇总结果；
- 单局最高中奖 / 单局最高净盈利按整个 Round 汇总判断；
- 十个子结果不分别计为十个独立 Round；
- 100x 子结果可以在 Round Detail 和表现中突出，但排行榜记录仍关联该 Tenfold round_id。

T1 / 1x：
- 单 Draw 为 Break-even；
- 不计为正盈利 Draw。

Tenfold：
- 即使包含若干 1x 或高 Tier，也以 Round 总净变化判断 Win / Loss / Break-even。

## 141.28 Tenfold 结果顺序

Tenfold 的 draw_index 1–10 是公平验证与历史记录的一部分。

已确认：
- Reveal Sequence 按 draw_index 顺序进行；
- Result Summary 默认保留 draw_index 顺序；
- 不因为某个 Tier 较高而在结算后重新排序；
- 可以通过视觉强调高 Tier，但不改变原始索引；
- Round Detail 始终保留原始 draw_index。

如未来提供“按稀有度查看”：
- 只能作为额外展示视图；
- 默认公平验证视图仍按原始顺序；
- 不得覆盖原始顺序。

## 141.29 Game History / Round Detail

Summon Round Detail 至少包含：

- Round ID；
- 创建时间；
- 接受下注时间；
- 结算时间；
- Summon Mode；
- Base Wager；
- Draw Count；
- Total Cost；
- 每个 draw_index；
- 每个 Reward Tier；
- 每个倍数；
- 每个 Draw Payout；
- Total Payout；
- Round Net Change；
- 结算前余额；
- 结算后余额；
- pool_id；
- prize_table_version；
- algorithm_version；
- game_config_version；
- game_config_hash；
- Provably Fair 验证数据；
- Cancelled / Refunded 信息（如适用）。

具体动画播放进度不属于正式资产历史数据。

## 141.30 维护行为

维护状态：
- 阻止创建新的 Single 或 Tenfold Round。

已经 BET_ACCEPTED 的 Round：
- 优先恢复并完成同一确定性结果；
- 已结算结果不得退款后再次派奖；
- 只有服务端无法生成确定性结果或无法完成正式结算时，才进入退款流程。

维护切换不得遗弃已经接受的抽取。

## 141.31 PC 与 Mobile

PC：
- 使用 Focused Game Layout；
- 模式、Base Wager、Total Cost、Available Chips 和主操作保持清晰；
- Tenfold 结果应在一个可理解的结果区域内展示；
- Rates、History 和 Provably Fair 可使用 Drawer / Panel；
- Skip / Reveal All 始终可发现。

Mobile：
- Single / Tenfold 切换必须清晰；
- Base Wager 与 Tenfold Total Cost 不得混淆；
- 四个快捷金额不得拥挤；
- 十个结果使用适合手机的 Grid、列表或分页表现；
- 不要求用户横向查看 PC 宽表格；
- Skip / Reveal All 易于单手操作；
- Round Detail 使用全屏详情层；
- reduced-motion 下仍完整显示 Tier、倍数、派彩与 Round 汇总。

## 141.32 Presentation Hooks

IA-07 只冻结所需表现能力：

- 未抽取状态；
- Single Summon 启动；
- Tenfold Summon 启动；
- 召唤过程；
- Reward Tier 差异化提示；
- T0 / 0x；
- T1 / 1x Break-even；
- 普通净盈利；
- 20x 高奖；
- 100x 最高奖；
- 十连逐个揭示；
- 加速；
- Skip / Reveal All；
- 十连结果汇总；
- reduced-motion 静态结果；
- 刷新后 Replay Presentation 的能力。

留到 Art Direction v0.4：

- Reward Tier 最终名称；
- 稀有度星级；
- 蓝光 / 金光 / 彩光具体映射；
- 召唤阵；
- 卡面；
- 角色；
- 背景；
- 粒子；
- 音效；
- 镜头；
- 具体动画分镜与时长。

## 141.33 V1 暂不包含

V1 暂不包含：

- Servant 收藏；
- 礼装收藏；
- 卡牌背包；
- 灵基图鉴；
- 养成；
- 物品交易；
- 多 Banner；
- Rate-up；
- Pity；
- Tenfold 保底；
- 折扣十连；
- 第 11 抽；
- Auto Summon；
- 自定义批量抽取；
- Free Summon；
- Bonus Draw；
- Jackpot；
- Double or Nothing；
- 实物奖品。

# 142. Slot Machine /老虎机

## 142.1 游戏身份

展示名称：
Slot Machine /老虎机

稳定 game_slug：
slot

逻辑入口：
/games/slot

Entry Type：
Direct Play

主要交互能力：
Instant Resolve + Reveal Sequence

资金模型：
娱乐主钱包付费 Round

页面 Shell：
Focused Game Layout

## 142.2 V1 产品定位

V1 采用经典固定 Payline 视频老虎机：

- 5 个 Reel；
- 3 行可见区域；
- 10 条固定 Payline；
- 所有 Payline 始终启用；
- 从最左 Reel 向右连续判定；
- 每条线至少连续 3 个有效符号才中奖；
- 支持 Wild 替代；
- 多条中奖线可以同时派彩。

V1 重点是先完成：
- 清楚的总下注；
- 可验证的 Reel Stop；
- 固定 Payline；
- 稳定奖表与 RTP；
- 服务端权威结算；
- 刷新 / 断线恢复；
- PC 与手机端完整操作。

V1 不加入复杂 Bonus Feature，避免把第一版变成多层状态机。

## 142.3 Reel Grid

可见 Grid：

5 Reels × 3 Rows

行标识：
- T = Top；
- M = Middle；
- B = Bottom。

概念结构：

Reel 1 | Reel 2 | Reel 3 | Reel 4 | Reel 5
   T   |    T   |    T   |    T   |    T
   M   |    M   |    M   |    M   |    M
   B   |    B   |    B   |    B   |    B

每个 Spin 只创建一个 round_id。

## 142.4 Direct Play 全局下注与 Line Stake

本游戏继承 Direct Play 全局下注策略：

最低总下注：
10 娱乐筹码

产品层固定最高下注：
不设置

固定快捷金额：
10 / 100 / 500 / 1000

下注输入表示：
“当前整个 Spin / Round 的 Total Wager”

不是：
- 单条 Payline 金额；
- 每个 Reel 金额；
- 单个 Symbol 金额。

V1 10 条 Payline 始终启用，因此：

line_stake = total_wager / 10

例如：

Total Wager = 10
→ 每条线 Line Stake = 1

Total Wager = 100
→ 每条线 Line Stake = 10

Total Wager = 11
→ 每条线 Line Stake = 1.1

由于 1 娱乐筹码 = 500,000 atomic units，任何整数 Total Wager 均可无损均分为 10 份：

1 chip / 10 = 50,000 atomic units

因此无需强制 Total Wager 必须为 10 的倍数。

快捷按钮：
- 只填入 Total Wager；
- 不自动 Spin；
- 不扣款；
- 不创建 Round。

## 142.5 逻辑符号与美术分离

V1 使用 8 个稳定逻辑符号：

L1
L2
L3
M1
M2
H1
H2
W / Wild

语义：
- L = Low Tier；
- M = Mid Tier；
- H = High Tier；
- W = Wild。

每个逻辑符号绑定：
- stable symbol_id；
- Reel Strip 位置；
- Paytable；
- presentation_tier；
- config_version。

Art Direction v0.4 可以将其映射为：
- 职阶符号；
- 圣晶石或素材；
- 令咒；
- 礼装式图案；
- 角色剪影；
- 其他 FGO 风格元素。

更换图案、名称或素材不得改变：
- symbol_id；
- Reel Stop；
- 概率；
- 派彩；
- 历史记录；
- Provably Fair 验证。

## 142.6 Virtual Reel 模型

V1 使用 5 条固定 Virtual Reel Strip。

每条 Reel：
- 32 个 Stop；
- 每个 Stop 被均匀选择；
- 5 个 Reel 的 Stop 独立生成；
- 使用无偏映射避免 modulo bias；
- Reel Strip 与顺序属于版本化配置。

每条 Reel 的符号频数相同：

| Symbol | 每 Reel 数量 | 单行边际概率 |
|--------|--------------|--------------|
| L1     | 8            | 25.000%      |
| L2     | 7            | 21.875%      |
| L3     | 5            | 15.625%      |
| M1     | 4            | 12.500%      |
| M2     | 3            | 9.375%       |
| H1     | 2            | 6.250%       |
| H2     | 2            | 6.250%       |
| W      | 1            | 3.125%       |

总计：
32 Stops。

## 142.7 Reel Strip v1

以下 Strip 索引为 0–31，循环首尾相接。

Reel 1：
L3, L2, L3, M1, L2, L1, L3, L2, M2, H2, L1, M2, L3, M1, H1, L2, L1, H1, L1, M2, L2, L3, L1, M1, W, L1, L2, L1, L2, H2, M1, L1

Reel 2：
L2, M2, L1, L2, L3, L2, W, L1, H1, L1, M1, L3, M1, H2, M1, L2, L1, L3, L1, L3, H1, L1, H2, L3, L1, L2, M2, L2, L1, L2, M1, M2

Reel 3：
L2, L3, L1, M2, L1, H1, L3, L1, M1, M2, L2, M1, L3, L1, L2, L3, H2, L2, L1, M1, L2, L1, H1, L2, L1, H2, M1, L3, L1, L2, M2, W

Reel 4：
M1, L3, W, L2, L1, H2, L2, M2, L1, M1, L1, L3, H1, L2, L3, M2, H1, L1, L2, L3, M1, L1, L3, L1, L2, H2, L1, M1, M2, L2, L1, L2

Reel 5：
L1, H2, L2, L1, L3, L2, L3, L1, H1, L2, L1, L2, M1, L1, W, H2, M1, L2, H1, L1, L2, L1, M1, M2, L3, M1, L2, L1, M2, L3, M2, L3

可见窗口由 Stop Index 计算：
- Top = stop_index - 1；
- Middle = stop_index；
- Bottom = stop_index + 1；
- 超出范围时对 32 取循环。

Reel Strip 的完整顺序必须绑定：
- reel_strip_version；
- game_config_version；
- game_config_hash；
- algorithm_version。

## 142.8 10 条固定 Payline

所有 Payline 始终启用：

Line 1：M-M-M-M-M
Line 2：T-T-T-T-T
Line 3：B-B-B-B-B
Line 4：T-M-B-M-T
Line 5：B-M-T-M-B
Line 6：T-T-M-B-B
Line 7：B-B-M-T-T
Line 8：M-T-T-T-M
Line 9：M-B-B-B-M
Line 10：B-M-M-M-T

每条线：
- 从 Reel 1 开始；
- 只按从左到右判定；
- 至少连续 3 个有效相同符号 / Wild 才派彩；
- 不支付右到左组合；
- 不支付中间起始组合。

## 142.9 单条 Payline 判定

对每条 Payline：

1. 从 Reel 1 向 Reel 5 读取符号；
2. 对每个普通符号 L1–H2 检查连续相同符号或 Wild；
3. 连续长度达到 3、4、5 时读取对应 Paytable；
4. 对纯 Wild 连续组合读取 Wild Paytable；
5. 如果同一序列可以被 Wild 解释为多个符号，选择派彩最高的单个解释；
6. 同一 Payline 只支付一个最高有效组合；
7. 5 连不会再叠加支付同一线的 3 连和 4 连。

多条 Payline 同时中奖时：
- 每条线独立计算；
- 所有 Line Payout 相加；
- 同一个 Grid Cell 可以同时参与多条不同 Payline；
- 不因共享 Cell 而取消其他合法 Line Win。

## 142.10 Wild 规则

W / Wild：
- 可以替代 L1、L2、L3、M1、M2、H1、H2；
- 可以出现在任意 Reel；
- 不承担 Scatter 功能；
- 没有额外 Multiplier；
- 没有扩展、黏性、堆叠或移动能力；
- 自身存在 3 / 4 / 5 连派彩。

如果 Payline 以一个或多个 Wild 开始：
- 服务端枚举所有合法普通符号解释；
- 同时检查纯 Wild 组合；
- 只支付金额最高的一个解释；
- 不重复派彩。

## 142.11 Paytable v1

下表倍数均相对于单条 Payline 的 Line Stake，并且表示包含本金的总派彩倍数：

| Symbol | 3 连 | 4 连 | 5 连 |
|--------|------|------|------|
| L1     | 4x   | 15x  | 50x  |
| L2     | 8x   | 25x  | 80x  |
| L3     | 10x  | 40x  | 150x |
| M1     | 15x  | 60x  | 250x |
| M2     | 25x  | 100x | 500x |
| H1     | 50x  | 250x | 1000x|
| H2     | 100x | 500x | 2500x|
| W      | 125x | 1000x| 5000x|

示例：

Total Wager = 100
Line Stake = 10

某条线获得 H2 五连：
Line Payout = 10 × 2500 = 25,000

所有中奖线汇总后得到 Round Total Payout。

## 142.12 数学口径

在上述：
- 5 × 3 Grid；
- 10 条固定 Payline；
- Reel Strip v1；
- Paytable v1；
- Wild 最高解释；
- 每条线只支付一个最高组合；

条件下，理论值为：

理论 RTP：
96.0033118724823%

任意一条或多条 Payline 产生非零派彩的概率：
41.4642333984375%

完全没有任何 Line Payout 的概率：
58.5357666015625%

Round 最终净盈利概率：
19.3924903869629%

Round 恰好回本概率：
3.46889495849609%

Round 最终净亏损概率：
77.1386146545410%

其中“有部分派彩但仍净亏损”的概率：
18.6028480529785%

当前 Reel Strip v1 与 Payline v1 下的理论最高 Round Total Payout：
516.4 × Total Wager

说明：
- Paytable 中 Wild 五连为 5000 × Line Stake；
- 因 Line Stake = Total Wager / 10，单条纯 Wild 五连相当于 500 × Total Wager；
- 同一 Grid 还可能产生其他 Line Win，因此当前全 Round 最大值为 516.4 × Total Wager；
- 不设置额外人工 Payout Cap。

## 142.13 Result 语义

Round 结果必须按照 Total Payout 与 Total Wager 比较：

No Win：
- Total Payout = 0。

Partial Return / Net Loss：
- 0 < Total Payout < Total Wager；
- 虽然存在 Line Win，但整个 Round 仍为净亏损。

Break-even：
- Total Payout = Total Wager。

Win：
- Total Payout > Total Wager。

页面不得仅因为出现一条派彩线，就把净亏损 Round 误标为“Win”。

## 142.14 单局用户流程

```text
进入 /games/slot
→ 查看 Available Chips、固定 10 Lines、当前 Paytable 与 RTP 摘要
→ Total Wager 默认 10，或恢复上一次 Slot 有效金额
→ 输入整数金额，或点击 10 / 100 / 500 / 1000
→ 页面显示 Total Wager 与 Line Stake
→ 点击 Spin
→ 前端进入 SUBMITTING，锁定重复操作
→ 服务端校验登录、运行状态、Total Wager、Available Chips、配置版本与幂等键
→ 服务端接受下注并创建唯一 round_id
→ 服务端生成 5 个确定性 Reel Stop
→ 服务端生成 5 × 3 Grid
→ 服务端计算 10 条 Payline、Total Payout 与 Net Change
→ 服务端完成钱包与 ledger 结算
→ 页面进入 SPINNING / REVEALING
→ 用户观看、加速或跳过表现
→ 页面展示 Grid、中奖线、Total Payout、Net Change 与更新后余额
→ 用户可以再次主动 Spin、查看 Round Detail 或离开游戏
```

## 142.15 业务状态与表现状态分离

业务状态至少表达：

READY
- 等待 Total Wager 与 Spin。

SUBMITTING
- 请求正在提交；禁止重复点击。

BET_ACCEPTED
- 服务端已经接受下注；Total Wager 与策略版本被锁定。

RESOLVING
- 服务端正在生成或读取确定性 Reel Stop，并计算派彩。

SETTLED
- Grid、Line Payout、Total Payout、Net Change 与余额更新均已完成。

RECOVERING
- 刷新、断线或网络异常后恢复同一 Round。

CANCELLED / REFUNDED
- 无法完成正式结果或结算时的取消 / 退款状态。

前端 Presentation 状态可以表达：

PREPARING
SPINNING
STOPPING
SHOWING_LINES
SUMMARY

Presentation 状态不得反向修改已经完成的业务结算。

## 142.16 服务端结算时点

服务端接受 Spin 后立即：

1. 锁定 Total Wager；
2. 锁定 10 条 Payline；
3. 锁定 Reel Strip Version；
4. 锁定 Paytable Version；
5. 锁定 Game Config Version；
6. 生成 5 个确定性 Stop Index；
7. 构建完整 Grid；
8. 计算每条中奖线；
9. 计算 Total Payout 与 Net Change；
10. 完成钱包与 ledger 结算；
11. 前端开始 Reel 动画。

前端 Reel 动画：
- 不生成 Stop；
- 不选择 Symbol；
- 不改变中奖线；
- 不改变派彩；
- 不重新抽取；
- 不延迟正式结算。

## 142.17 页面信息结构

Slot Game Entry
```text
│
├── Game Header
│   ├── 返回 Game Catalog / Entertainment
│   ├── 游戏名称与运行状态
│   ├── Available Chips
│   └── Rules / Paytable / Provably Fair / History
│
├── Slot Stage
│   ├── 5 × 3 Reel Grid
│   ├── Payline Overlay
│   ├── Spin / Stop Presentation
│   └── Win Presentation
│
├── Wager Area
│   ├── Total Wager
│   ├── 10 / 100 / 500 / 1000
│   ├── Active Lines = 10
│   └── Line Stake
│
├── Primary Action
│   └── Spin
│
├── Presentation Control
│   └── Fast Stop / Skip Current Presentation
│
├── Current Round State
├── Result Summary
└── Rules / Paytable / Reel Strips / Provably Fair / Recent History
```

## 142.18 Spin 与 Reel 表现

点击 Spin 后：
- Spin 按钮立即进入不可重复提交状态；
- 结果先由服务端确定并结算；
- Reel 动画按照已确定 Stop Index 展示；
- Reel 默认从左至右停止，但精确节奏留到 Art Direction v0.4。

允许：
- 点击 Fast Stop / Skip；
- 或在动画期间再次点击 Reel Stage 加速；
- 立即结束当前 Reel 表现并进入中奖线 / Summary。

不允许：
- 玩家分别手动决定每个 Reel 的停止点；
- Stop 操作影响结果；
- Hold；
- Nudge；
- Skill Stop；
- 动画结束后改变已确定 Grid。

V1 不提供持久 Turbo Mode。
Fast Stop 只作用于当前 Round 的表现。

## 142.19 中奖线展示

Round Settled 后：
- 先显示完整 Grid；
- 明确显示 Total Payout 与 Net Change；
- 对中奖 Payline 进行高亮；
- 支持查看每条中奖线的 Symbol、连续长度、Line Stake、Multiplier 与 Line Payout；
- 多条中奖线可以自动轮播高亮；
- 用户可以直接 Skip 到汇总；
- 不要求用户看完每条线动画才能开始下一 Round。

具体颜色、线条形状、轮播时长与 Big Win 表现留到 Art Direction v0.4。

## 142.20 下注记忆与按钮状态

第一次进入 Slot：
- Total Wager 默认 10。

后续进入：
- 恢复上一次 Slot Total Wager；
- 只在 Slot 内记忆；
- 不与 Dice、Scratch、Summon 或 Blackjack 共享。

一局结算后：
- 保留当前 Total Wager；
- 用户必须再次主动点击 Spin；
- 不自动创建下一 Round。

快捷按钮：
- 高于 Available Chips 时禁用；
- 自定义输入仍可输入不超过余额的合法整数；
- 不提供 Max、50%、百分比或 All-in。

## 142.21 余额不足

当 Available Chips < 10：
- Spin 不可创建有效付费 Round；
- 显示最低 Total Wager 为 10；
- 提供 Wallet；
- 提供 Rewards Center。

从 Wallet / Rewards 返回后：
- 刷新 Available Chips；
- 恢复 Slot 页面；
- 可以保留尚未提交的 Total Wager；
- 不自动 Spin；
- 不重放之前失败的提交。

## 142.22 页面刷新、断线与幂等

服务端尚未接受 Spin：
- 不产生正式资产变化；
- 用户可以重新提交。

服务端已经接受 Spin：
- 页面刷新不得再次扣款；
- 网络超时不得创建另一个 Round；
- 必须恢复同一 round_id；
- 必须恢复相同 Reel Stop、Grid、中奖线、Total Payout 与 Net Change；
- 已 SETTLED 时默认进入 Result Summary；
- 可以提供 Replay Presentation；
- Replay 不得重新结算。

V1 不保存 Reel 动画播放到哪一帧。

## 142.23 Provably Fair

每个 Round 至少绑定：

- server_seed_hash；
- server_seed（按既定 Reveal 规则公开）；
- client_seed；
- nonce；
- round_id；
- algorithm_version；
- game_config_version；
- game_config_hash；
- reel_strip_version；
- paytable_version；
- payline_version；
- total_wager；
- active_lines = 10。

确定性验证应满足：

同一组 Seed + Nonce + Algorithm Version
→ 同一组 5 个 Reel Stop
→ 同一个 5 × 3 Grid
→ 同一组 Payline 结果
→ 同一个 Total Payout
→ 同一个 Net Change

每个 Stop Index 必须通过无偏映射生成。

Client Seed：
- 默认由系统生成；
- 可以在 Provably Fair 面板查看；
- 可以修改下一 Round；
- 已 BET_ACCEPTED 的 Round 不可修改。

## 142.24 透明度与版本化

V1 始终公开：

- 5 × 3 Grid；
- 10 条 Payline；
- Line Stake 规则；
- 完整 Symbol Paytable；
- Wild 规则；
- Reel Strip 频数；
- 完整 Reel Strip 顺序；
- 理论 RTP；
- 非零派彩概率；
- 净盈利、回本与净亏损概率；
- 当前理论最高 Round Payout；
- 当前 reel_strip_version；
- 当前 paytable_version；
- 当前 payline_version。

管理员可以创建新版本，但：
- 新版本只影响之后创建的 Round；
- 已发布并被历史 Round 引用的版本不得修改；
- 当前 Active 版本的真实数学信息不得隐藏；
- 历史 Round 必须保留原版本与验证数据。

## 142.25 Maintenance 行为

维护状态：
- 阻止创建新 Spin。

已经 BET_ACCEPTED 的 Round：
- 优先恢复并完成同一确定性 Grid 与结算；
- 已结算结果不得先退款后再次派奖；
- 只有服务端无法生成确定性结果或无法完成正式结算时，才进入退款流程。

维护切换不得遗弃已接受 Spin。

## 142.26 Game History / Round Detail

Slot Round Detail 至少包含：

- Round ID；
- 创建时间；
- 接受下注时间；
- 结算时间；
- Total Wager；
- Active Lines = 10；
- Line Stake；
- 5 个 Reel Stop Index；
- 完整 5 × 3 Grid；
- 每条中奖 Payline；
- 每条线的 Symbol Interpretation；
- 连续长度；
- Line Multiplier；
- Line Payout；
- Total Payout；
- Round Net Change；
- 结算前余额；
- 结算后余额；
- reel_strip_version；
- paytable_version；
- payline_version；
- algorithm_version；
- game_config_version；
- game_config_hash；
- Provably Fair 验证数据；
- Cancelled / Refunded 信息（如适用）。

具体 Reel 动画进度不属于正式资产历史数据。

## 142.27 统计与排行榜语义

每个 Spin：
- 计为一个 Round；
- cumulative_wager 增加 Total Wager；
- Total Payout 为所有 Line Payout 之和；
- Round Net Change = Total Payout - Total Wager。

排行榜与统计必须记录：
- Total Wager；
- Total Payout；
- Net Change；
- Total Payout Multiplier；
- 最高单线 Multiplier；
- 中奖线数量。

单局是否为 Win：
- 以 Round Net Change 是否大于 0 判断；
- 不能仅以是否存在非零 Line Payout 判断。

“单局最高中奖榜”的最终排序使用 Total Payout 还是 Net Profit，继续由 Rankings 阶段统一定义；Slot 必须同时保存两者。

## 142.28 PC 与 Mobile

PC：
- 使用 Focused Game Layout；
- Grid、Total Wager、Line Stake、Spin 与 Result Summary 保持同一视野内可理解；
- Paytable、Reel Strips、History 和 Provably Fair 可以使用 Drawer / Panel；
- 中奖线 Detail 可以轮播或展开；
- Fast Stop 始终可发现。

Mobile：
- 支持手机竖屏完整游玩；
- 不强制横屏；
- 5 × 3 Grid 必须保持符号可辨识；
- 四个快捷金额不得拥挤；
- Total Wager 与 Line Stake 不得混淆；
- Spin 与 Fast Stop 适合单手操作；
- 中奖线 Detail 使用 Sheet / 全屏详情；
- 默认保留 Bottom Navigation，但不得遮挡 Spin、余额或 Result Summary；
- reduced-motion 下仍完整显示 Grid、中奖线、Total Payout 与 Net Change。

## 142.29 Presentation Hooks

IA-07 只冻结所需表现能力：

- Idle Reel Grid；
- Spin Start；
- Reel Movement；
- Reel Stop；
- Fast Stop / Skip；
- 完整 Grid Reveal；
- 单条 / 多条 Payline Highlight；
- No Win；
- Partial Return / Net Loss；
- Break-even；
- Win；
- High Payout Presentation；
- Result Summary；
- reduced-motion 静态结果；
- 刷新后的 Replay Presentation。

明确禁止：
- 动画改变 Stop；
- 假停后重新抽取；
- 通过额外减速制造与真实 Grid 无关的 Near-miss；
- 将净亏损 Round 以误导性的“Big Win”表现包装；
- 隐藏 Total Wager 或 Net Change。

留到 Art Direction v0.4：
- 机台外观；
- Symbol 最终图案；
- Reel 材质；
- Payline 颜色；
- 背景；
- 角色；
- 光效；
- 粒子；
- 音效；
- Big Win 视觉级别；
- Reel 动画曲线；
- 精确停顿顺序与时长。

## 142.30 V1 暂不包含

V1 暂不包含：

- 可选择 Payline 数量；
- 可调整 Coin Denomination；
- Scatter；
- Free Spins；
- Bonus Game；
- Bonus Buy；
- Jackpot / Progressive Jackpot；
- Multiplier Wild；
- Expanding Wild；
- Sticky Wild；
- Stacked Wild；
- Cascading Reels；
- Respin；
- Hold；
- Nudge；
- Skill Stop；
- Auto Spin；
- 持久 Turbo Mode；
- Max / All-in；
- Double or Nothing；
- 免费 Round。

# 143. Blackjack / 21 点

## 143.1 游戏身份

展示名称：
Blackjack / 21 点

稳定 game_slug：
blackjack

逻辑入口：
/games/blackjack

Entry Type：
Direct Play

主要交互能力：
Multi-action Round + Reveal Sequence

资金模型：
娱乐主钱包付费 Round；Split 与 Double 可以在同一 Round 中产生额外下注。

页面 Shell：
Focused Game Layout

## 143.2 V1 产品定位

V1 采用经典单人对庄家 Blackjack：

- 玩家与系统庄家对战；
- 目标是在不超过 21 点的前提下高于庄家；
- 使用公平、完整、预先确定顺序的牌靴；
- 支持 Hit、Stand、Double、Split；
- Blackjack 支付 3:2；
- 庄家 Soft 17 停牌；
- 不加入保险、投降、边注、五小龙或其他变体规则。

V1 重点是先完成：
- 清楚的初始下注与追加下注；
- 完整多手 Split 状态；
- 确定性牌靴与 Provably Fair；
- 服务端权威行动与结算；
- 刷新、断线和长时间离开后的恢复；
- PC 与手机竖屏完整操作。

## 143.3 牌靴与基础牌组

V1 每个 Round 使用一副全新洗牌的六副牌牌靴：

- 6 × 52 = 312 张牌；
- 不使用 Joker；
- 每个新 Round 都重新生成一套完整、确定性的 312 张牌顺序；
- 不跨 Round 延续同一个 Shoe；
- 不设置 Cut Card 或 Shoe Penetration；
- 不提供依赖跨局牌靴状态的记牌体验。

每个 Round 的完整牌序必须在首张牌发出前确定。

Hit、Double、Split、庄家补牌等操作只从该预先确定的牌序中按顺序取下一张牌，不得在玩家操作后重新生成随机牌。

## 143.4 牌面点数

牌面点数：

- 2–9：按牌面数字计算；
- 10、J、Q、K：均为 10 点；
- A：优先按 11 点计算；若会爆牌，则一个或多个 A 自动改按 1 点计算。

Soft Hand：
- 当前至少有一个 A 仍按 11 点计算。

Hard Hand：
- 没有 A 按 11 点计算，或所有 A 都必须按 1 点计算。

玩家与庄家区域都需要显示当前可公开的总点数，并在适用时表达 Soft Total。

## 143.5 Blackjack / Natural 定义

Natural Blackjack 只指：

- 原始、未 Split 的玩家手牌；
- 最初两张牌；
- 一张 A + 一张任意 10 点牌。

Natural Blackjack：
- 在庄家没有 Blackjack 时，净盈利为初始下注的 1.5 倍；
- 含本金的总派彩为初始下注的 2.5 倍。

Split 后任意一手以两张牌达到 21 点：
- 只视为普通 21；
- 不视为 Natural Blackjack；
- 获胜时按 1:1 普通胜利结算。

## 143.6 初始发牌与庄家 Hole Card

采用 American Hole-card 结构。

初始发牌顺序：

1. 玩家第一张牌；
2. 庄家明牌 / Upcard；
3. 玩家第二张牌；
4. 庄家暗牌 / Hole Card。

庄家 Hole Card：
- 在玩家行动阶段保持隐藏；
- 已经从预先确定的牌靴中取出；
- 不得在玩家行动后重新生成或替换；
- 在庄家行动或 Round 提前结算时公开。

## 143.7 Dealer Peek

当庄家 Upcard 为：

- A；
- 10、J、Q、K 中任意 10 点牌；

服务端在玩家进行 Hit、Stand、Double 或 Split 前检查庄家是否为 Blackjack。

如果庄家为 Blackjack：
- 玩家也是 Natural Blackjack：Push；
- 玩家不是 Natural Blackjack：原始下注失败；
- Round 立即结算；
- 不允许玩家再产生 Double 或 Split 等额外下注。

如果庄家没有 Blackjack：
- 继续正常玩家行动；
- 不向玩家公开 Hole Card 内容。

V1 不提供 Insurance，因此 Dealer Peek 之前不出现保险购买流程。

## 143.8 玩家基础行动

V1 支持：

Hit：
- 当前手从牌靴获得下一张牌；
- 若超过 21 点，该手立即 Bust；
- 若恰好达到 21 点，该手自动 Stand。

Stand：
- 当前手停止补牌；
- 进入下一手或庄家阶段。

Double：
- 在符合条件时追加与当前 Hand Stake 相同的下注；
- 当前手只再获得一张牌；
- 随后自动 Stand。

Split：
- 在符合条件时追加与当前 Hand Stake 相同的下注；
- 将一手拆为两手；
- 两手分别结算。

V1 不提供：
- Insurance；
- Even Money；
- Surrender；
- Side Bets；
- Five-card Charlie；
- Early Cash Out；
- Auto Play。

## 143.9 Double Down 规则

Double Down 允许在：

- 任意未 Split 原始手的前两张牌；
- 非 Aces Split 手的前两张牌；

执行。

不限制为特定点数，例如 9、10、11；是否 Double 由玩家决定。

Double 成功后：

1. 从娱乐主钱包原子扣除与该 Hand Stake 相同的追加下注；
2. Hand Stake 变为原来的 2 倍；
3. 发出且只发出一张新牌；
4. 该手自动 Stand；
5. 后续不允许 Hit、再次 Double 或 Split。

Double After Split / DAS：
- V1 允许，但 Split Aces 除外。

如果可用娱乐筹码不足以支付追加下注：
- Double 按钮禁用；
- 玩家仍可选择 Hit 或 Stand。

## 143.10 Split 资格

V1 允许两张 Blackjack 点数相同的牌 Split。

因此：
- 8 + 8 可以 Split；
- A + A 可以 Split；
- 10、J、Q、K 都属于 10 点牌，任意两张 10 点牌可以互相 Split。

执行 Split 时：

1. 服务端原子扣除一笔与当前 Hand Stake 相同的追加下注；
2. 原两张牌分成两手；
3. 第一手和第二手各从预定牌靴获得一张牌；
4. 按从左到右的顺序依次操作每一手；
5. 每手独立计算 Stake、结果与派彩。

如果可用娱乐筹码不足以支付追加下注：
- Split 按钮禁用；
- 玩家仍可使用其他合法行动。

## 143.11 Re-split 与最大手数

非 Aces Split 手如果再次获得可 Split 的相同点数两张牌：
- 可以继续 Re-split；
- 前提是可用筹码足够；
- 整个 Round 最多同时形成 4 手。

达到 4 手后：
- 不再允许继续 Split；
- 仍可 Hit、Stand 或在适用时 Double。

每个 Split / Re-split 都必须：
- 拥有唯一 action_id；
- 产生独立、可审计的追加下注账变；
- 关联同一个 round_id；
- 防止重复请求造成重复扣款或重复建手。

## 143.12 Split Aces

Split Aces 使用特殊规则：

- A + A 可以 Split；
- Split 后每一手只获得一张额外牌；
- 获得该牌后自动 Stand；
- 不允许继续 Hit；
- 不允许 Double；
- 不允许 Re-split Aces；
- A + 10 点牌只按普通 21 计算，不属于 Natural Blackjack。

## 143.13 多手操作顺序

存在多手时：

- 使用稳定 hand_index；
- 按从左到右顺序操作；
- 一次只允许对 Active Hand 提交行动；
- 当前手进入 Stand、Bust、Double Complete 或 Split Aces Complete 后，切换到下一手；
- 所有玩家手完成后才进入庄家阶段。

页面必须清楚显示：
- 当前 Active Hand；
- 每手当前点数；
- 每手 Stake；
- 每手可用行动；
- 已完成手的状态。

## 143.14 庄家规则

庄家采用 Stand on Soft 17 / S17：

- Hard 16 或以下：Hit；
- Soft 16 或以下：Hit；
- Hard 17 或以上：Stand；
- Soft 17：Stand；
- Soft 18 或以上：Stand。

庄家行动完全由服务端规则执行，玩家不能影响庄家决策。

如果所有玩家手都已经 Bust：
- 庄家公开 Hole Card；
- 不需要继续补牌；
- Round 直接完成失败结算。

## 143.15 单手结果与派彩

以下倍数均基于该手实际 Hand Stake，且总派彩包含返还本金。

Bust / Loss：
- Total Payout = 0；
- Net Change = -Hand Stake。

Push：
- Total Payout = 1 × Hand Stake；
- Net Change = 0。

普通 Win：
- Total Payout = 2 × Hand Stake；
- Net Change = +Hand Stake。

Natural Blackjack：
- Total Payout = 2.5 × Initial Hand Stake；
- Net Change = +1.5 × Initial Hand Stake。

Dealer Bust：
- 所有未 Bust、未提前完成 Natural 的玩家手按普通 Win 结算。

玩家与庄家相同点数：
- Push。

## 143.16 小数派彩

Direct Play 的主动下注仍只接受整数娱乐筹码。

由于 Natural Blackjack 支付 3:2：
- 奇数下注可能产生 0.5 娱乐筹码派彩；
- 该小数可以由现有 Atomic Units 无损表示；
- 不进行向下取整、向上取整或随机舍入。

例如初始下注 11：

Natural Blackjack 总派彩：
27.5

Round 净变化：
+16.5

页面、钱包、历史和排行榜都必须准确显示该小数结果。

## 143.17 Round 总结算

一个 Blackjack Round 可以包含多手，但只拥有一个 round_id。

每手拥有：
- hand_id；
- hand_index；
- stake；
- cards；
- actions；
- final_total；
- result；
- payout；
- net_change。

Round 汇总：

Total Stake
= 初始下注 + 所有 Split 追加下注 + 所有 Double 追加下注

Total Payout
= 所有手 Total Payout 之和

Round Net Change
= Total Payout - Total Stake

Round 最终分类：
- Net Loss：Round Net Change < 0；
- Break-even：Round Net Change = 0；
- Net Win：Round Net Change > 0。

即使多手中存在部分胜利，也必须根据整个 Round 的 Net Change 显示最终汇总，不能只要有一手获胜就把整个 Round 标为 Win。

## 143.18 Direct Play 初始下注

本游戏继承 Direct Play 全局下注策略。

最低初始下注：
10 娱乐筹码

产品层固定最高下注：
不设置

固定快捷金额：
10 / 100 / 500 / 1000

下注输入表示：
当前 Blackjack Round 的 Initial Wager。

快捷按钮：
- 只填入 Initial Wager；
- 不自动 Deal；
- 不扣款；
- 不创建 Round。

实际初始下注不得超过当前 Available Chips。

Split 与 Double 的追加下注不重新显示四个快捷金额，而是始终等于对应 Hand 当前 Stake 的规则金额。

## 143.19 下注记忆与按钮状态

第一次进入 Blackjack：
- Initial Wager 默认 10。

后续进入：
- 恢复上一次 Blackjack 有效 Initial Wager；
- 只在 Blackjack 内记忆；
- 不与 Dice、Scratch、Summon 或 Slot 共享。

Round 结算后：
- 保留当前 Initial Wager；
- 用户必须再次主动点击 Deal；
- 不自动创建下一 Round。

快捷按钮：
- 高于 Available Chips 时禁用；
- 自定义输入仍可输入不超过余额的合法整数；
- 不提供 Max、50%、百分比或 All-in。

## 143.20 Deal 与追加下注确认

开始 Round：
- 不增加额外确认弹窗；
- Deal 主按钮显示或紧邻显示 Initial Wager，例如“Deal · 100”；
- 点击 Deal 即正式提交初始下注。

Double / Split：
- 不额外弹出确认窗口；
- 按钮必须清楚显示本次追加成本，例如：
  - Double · +100；
  - Split · +100；
- 点击后即提交对应追加下注与行动；
- 提交中立即锁定重复点击。

## 143.21 单局用户流程

```text
进入 /games/blackjack
→ 查看 Available Chips、规则摘要与 Initial Wager
→ 输入整数金额，或点击 10 / 100 / 500 / 1000
→ 点击 Deal · Amount
→ 前端进入 SUBMITTING，锁定重复操作
→ 服务端校验登录、运行状态、余额、配置版本与幂等键
→ 服务端原子扣除 Initial Wager 并创建唯一 round_id
→ 服务端生成完整确定性六副牌牌靴
→ 完成初始发牌
→ 如庄家 Upcard 为 A / 10 点牌，先执行 Dealer Peek
→ 如 Round 未提前结算，进入 PLAYER_TURN
→ 玩家对当前 Active Hand 执行 Hit / Stand / Double / Split
→ 所有玩家手完成后进入 DEALER_TURN
→ 庄家按 S17 规则补牌
→ 服务端逐手计算结果并完成汇总结算
→ 页面展示每手结果、Total Stake、Total Payout、Round Net Change 与更新后余额
→ 用户可以再次 Deal、查看 Round Detail 或离开游戏
```

## 143.22 业务状态

页面至少表达以下业务语义：

READY
- 等待 Initial Wager 与 Deal。

SUBMITTING
- 初始下注请求提交中，禁止重复点击。

BET_ACCEPTED
- 初始下注已扣除，Round 与牌靴已经锁定。

INITIAL_DEAL
- 初始四张牌已经从确定牌靴中取出。

DEALER_PEEK
- 服务端检查庄家 Blackjack，不公开 Hole Card。

PLAYER_TURN
- 等待当前 Active Hand 的玩家行动。

ACTION_SUBMITTING
- Hit / Stand / Double / Split 正在提交，禁止重复行动。

DEALER_TURN
- 所有玩家手完成，庄家按规则行动。

SETTLING
- 服务端计算逐手结果与 Round 汇总。

SETTLED
- 账务、派彩和余额更新完成。

RECOVERING
- 刷新、断线或重新进入后恢复同一 Round。

AUTO_RESOLVING
- 超过允许的长期未操作时间后，系统按冻结规则自动完成未结束手。

CANCELLED / REFUNDED
- 无法恢复合法牌序或完成结算时的正式取消 / 退款状态。

内部枚举可以在技术设计阶段统一，但用户界面必须表达等价状态。

## 143.23 服务端权威与 Action 幂等

客户端只能提交：
- round_id；
- hand_id；
- action_type；
- action_id / idempotency_key；
- 当前可见的必要交互输入。

客户端不得提交或决定：
- 下一张牌；
- 庄家 Hole Card；
- 牌靴顺序；
- Hand Total；
- 是否 Bust；
- 是否 Blackjack；
- 派彩；
- 最终余额。

每个玩家行动必须：
- 具有唯一 action_id；
- 使用服务端当前 action_sequence 校验；
- 重复请求返回同一已接受结果；
- 不得重复发牌；
- 不得重复扣除 Split / Double 追加下注；
- 不得对非 Active Hand 执行动作。

## 143.24 刷新、断线与离开页面

如果服务端尚未接受初始 Deal：
- 不产生正式资产变化；
- 用户可以重新提交。

如果服务端已经接受 Round：
- 页面刷新不得重新扣除 Initial Wager；
- 网络超时不得创建另一个 Round；
- 必须恢复同一 round_id、牌靴、发牌指针、所有 Hand 与 Action；
- 若等待玩家行动，恢复到同一 Active Hand；
- 若已经结算，返回同一 Result Summary；
- 若正式退款，显示 Refunded。

离开 Blackjack 页面：
- 不自动 Stand；
- 不自动退款；
- Round 保持可恢复；
- Dashboard / Entertainment Hub 的 Active / Resume 可以显示该 Round。

## 143.25 长时间未操作

V1 不设置短促的每步倒计时。

为了避免 Round 永久悬空：
- 从最后一次成功接受的玩家行动开始计算 24 小时；
- 24 小时内可以随时恢复并继续；
- 超过 24 小时后，系统对所有尚未完成的玩家手执行 Auto Stand；
- 随后庄家按 S17 完成行动并正常结算；
- Auto Stand 与自动结算必须写入明确的系统 Action 记录；
- 不能通过超时重新洗牌、换牌或退款规避已确定结果。

Split Aces 等已经自动完成的手不受影响。

## 143.26 并行游戏与可用余额

每个用户同时最多拥有一个未完成 Blackjack Round。

存在 Active Blackjack Round 时：
- 不能再创建第二个 Blackjack Round；
- 可以导航到平台其他页面；
- 不对整个 Chaldea 账号实施全局游戏锁；
- 用户仍可以进行其他合法操作或游戏；
- Double / Split 的可用性始终按动作提交时的最新 Available Chips 判断。

如果并发操作导致余额不再足以追加下注：
- 服务端拒绝该 Double / Split；
- 刷新 Available Chips；
- 当前 Hand 仍可继续选择其他合法行动；
- 不自动改变为 Stand 或 Hit。

## 143.27 余额不足

Initial Wager 阶段 Available Chips < 10：
- 不能创建付费 Round；
- 显示最低下注 10；
- 提供 Wallet；
- 提供 Rewards Center。

Round 进行中余额不足以 Double / Split：
- 只禁用对应追加下注动作；
- Hit / Stand 等不需要追加下注的合法动作继续可用；
- 不为了补充余额自动离开 Round；
- 用户自行离开页面并补充余额后，可以在 24 小时恢复期限内回到原 Round；
- 返回后重新由服务端计算 Double / Split 是否可用；
- 不自动提交原动作。

## 143.28 Provably Fair

每个 Round 至少绑定：

- server_seed_hash；
- server_seed（Round 完成后按规则 Reveal）；
- client_seed；
- nonce；
- round_id；
- algorithm_version；
- shuffle_algorithm_version；
- game_config_version；
- game_config_hash；
- deck_count = 6；
- 完整 312 张初始牌靴；
- 所有发牌的 shoe_index。

在接受初始下注前：
- 页面可以查看当前 Server Seed Hash；
- 玩家可以查看或修改下一 Round 的 Client Seed。

在 Round 完成前：
- 不公开 Server Seed；
- 不公开尚未发出的牌序；
- 不允许玩家从验证数据推导庄家 Hole Card 或未来牌。

Round 完成后：
- Reveal Server Seed；
- 允许重新生成完整六副牌顺序；
- 验证初始发牌、Hit、Double、Split 与庄家补牌都按同一顺序消费；
- 验证所有 Hand Result 与 Payout。

## 143.29 Shuffle Algorithm

V1 使用确定性、版本化的 Fisher–Yates Shuffle 或等价无偏洗牌算法。

要求：
- 输入来自 Server Seed、Client Seed、Nonce 与版本化派生规则；
- 每次交换索引使用无偏随机映射；
- 不直接使用存在 modulo bias 的简单取模；
- 不对牌面、花色、庄家牌或玩家牌设置隐藏权重；
- 不根据玩家下注金额、历史盈亏或行为改变牌序；
- 同一输入必须重现同一 312 张牌顺序。

## 143.30 RTP 与策略依赖

Blackjack 的实际 RTP 取决于玩家决策，不能像 Slot 或 Scratch 一样只用单一固定概率表描述。

V1 冻结：
- 公平六副牌；
- S17；
- Blackjack 3:2；
- Dealer Peek；
- Double Any Two；
- Double After Split；
- 最多 4 手；
- Split Aces 一张牌；
- 无 Insurance、Surrender 与 Side Bets。

平台不得：
- 通过隐藏牌面权重人工调低 RTP；
- 在后台直接输入一个与规则不一致的“目标 RTP”；
- 将休闲玩家的实际结果伪装成规则固有 RTP。

正式上线前必须：
- 对冻结规则建立参考 Basic Strategy；
- 使用可重复的枚举或大规模模拟验证器计算参考 RTP / House Edge；
- 发布所采用的规则版本、参考策略和验证结果；
- 不在验证完成前手工写死一个看似精确的百分比。

普通玩家实际 RTP 可能因为决策偏离参考策略而下降。

## 143.31 透明度

V1 始终公开：

- 牌靴副数；
- 每 Round 重新洗牌；
- Blackjack 3:2；
- Dealer S17；
- Dealer Peek；
- Hit / Stand / Double / Split 规则；
- Double After Split；
- 最大 4 手；
- Split Aces 规则；
- 无 Insurance、Surrender 与 Side Bets；
- Push 与逐手派彩规则；
- RTP 的策略依赖说明；
- Provably Fair 算法版本与结算后完整验证数据。

这些核心规则不跟随通用透明度开关隐藏。

## 143.32 页面信息结构

Blackjack Game Entry
```text
│
├── Game Header
│   ├── 返回 Game Catalog / Entertainment
│   ├── 游戏名称与运行状态
│   ├── Available Chips
│   └── Rules / Provably Fair / History
│
├── Dealer Area
│   ├── Dealer Upcard
│   ├── Hidden Hole Card
│   ├── Dealer Total（可公开时）
│   └── Dealer Draw Sequence
│
├── Player Hands Area
│   ├── Hand Cards
│   ├── Hard / Soft Total
│   ├── Hand Stake
│   ├── Active Hand
│   └── Hand Result
│
├── Wager Area（READY 时）
│   ├── Initial Wager
│   ├── 10 / 100 / 500 / 1000
│   └── Deal · Amount
│
├── Action Area（PLAYER_TURN 时）
│   ├── Hit
│   ├── Stand
│   ├── Double · +Amount
│   └── Split · +Amount
│
├── Current Round State
├── Result Summary
└── Rules / Strategy-independent RTP Note / Provably Fair / Recent History
```

## 143.33 Result Summary

Round 结算后至少显示：

- Dealer 最终手牌与总点数；
- 每个玩家 Hand 的牌、Stake、行动记录和最终点数；
- 每手 Result；
- 每手 Payout；
- Initial Wager；
- Split 追加下注；
- Double 追加下注；
- Total Stake；
- Total Payout；
- Round Net Change；
- 更新后的 Available Chips；
- Round ID；
- 查看 Round Detail；
- 查看 Provably Fair。

多手 Round 必须同时提供逐手结果和 Round 汇总。

## 143.34 Game History / Round Detail

Blackjack Round Detail 至少包含：

- Round ID；
- 创建、初始下注接受、最后行动与结算时间；
- Initial Wager；
- 所有追加下注；
- Total Stake；
- Dealer Upcard、Hole Card 与补牌；
- 每个 hand_id / hand_index；
- 每手全部牌；
- 每手 Hard / Soft Total 变化；
- 每个玩家 Action 与 action_sequence；
- 自动 Stand 或系统动作；
- 每手 Result / Payout / Net Change；
- Round Total Payout / Net Change；
- 结算前后余额；
- 完整牌靴顺序与所有 shoe_index；
- server_seed_hash / server_seed / client_seed / nonce；
- algorithm_version；
- shuffle_algorithm_version；
- game_config_version；
- game_config_hash；
- Cancelled / Refunded 信息（如适用）。

## 143.35 Maintenance 行为

Maintenance：
- 阻止创建新 Blackjack Round。

已经 BET_ACCEPTED 的 Round：
- 优先保留并恢复原 Round；
- 不因为维护重新洗牌或替换牌；
- 用户在服务恢复后继续原 Active Hand；
- 如果 Round 达到 24 小时未操作期限，按既定 Auto Stand 规则完成；
- 只有无法恢复合法牌靴、行动状态或正式结算时，才退款全部仍归该 Round 的有效 Stake；
- 不允许既退款又再次派奖。

## 143.36 PC 与 Mobile

PC：
- 使用 Focused Game Layout；
- Dealer、当前 Active Hand、行动按钮和 Stake 保持同一视野内可理解；
- Split 后最多 4 手可横向或分区展示；
- Rules、History 与 Provably Fair 可以使用 Drawer / Panel；
- Double / Split 的追加成本始终可见。

Mobile：
- 手机竖屏完整可玩，不强制横屏；
- Dealer 区保持紧凑但 Hole Card 状态清楚；
- 多手时以 Active Hand 为中心，其他手使用可读的横向切换或缩略导航；
- Hit / Stand 为最高频主操作；
- Double / Split 明确显示追加成本与不可用原因；
- 行动区不得被 Bottom Navigation 遮挡；
- 默认保留 Bottom Navigation，如实测严重干扰再返回页面设计阶段确认；
- Round Detail 使用全屏详情层；
- reduced-motion 下仍清楚显示发牌顺序、手牌与最终结果。

## 143.37 Presentation Hooks

IA-07 只冻结所需表现能力：

- Idle Table；
- Initial Deal；
- Dealer Hole Card；
- Dealer Peek 状态；
- Hit Deal；
- Stand；
- Double；
- Split 与多手重排；
- Split Aces；
- Bust；
- Natural Blackjack；
- Dealer Reveal；
- Dealer Draw；
- Push / Loss / Win；
- 多手逐项结算；
- Round Summary；
- Refresh / Recover；
- reduced-motion 静态或简化表现。

允许：
- 加速或跳过纯发牌 / 结算动画；
- 结算后查看完整行动时间线。

不允许：
- 跳过玩家必须作出的决策并自动选择策略；
- 动画改变牌序；
- 假抽牌或重新洗牌；
- 在 Hole Card 应隐藏时通过表现泄露；
- 将部分手获胜但 Round 净亏损包装成整体 Big Win。

留到 Art Direction v0.4：
- 牌桌样式；
- 扑克牌与牌背；
- 庄家形象；
- FGO 角色与职阶映射；
- 背景；
- 光效；
- 音效；
- 动画节奏；
- Blackjack / Bust / Win 的最终视觉分镜。

## 143.38 V1 暂不包含

V1 暂不包含：

- Insurance；
- Even Money；
- Early Surrender；
- Late Surrender；
- Side Bets；
- Perfect Pairs；
- 21+3；
- Five-card Charlie；
- European No-hole-card；
- Persistent Shoe；
- Cut Card；
- Card Counting UI；
- Strategy Hint；
- Basic Strategy 自动提示；
- Auto Play；
- Repeat Deal 一键自动下注；
- Early Cash Out；
- Free Round；
- Jackpot；
- Double or Nothing；
- 多人同桌 Blackjack。

# 144. IA-07 最终冻结结论

1. 五款 V1 Direct Play 游戏的规则、数值、页面职责、恢复行为与公平验证已经完成确认。
2. 五款游戏均引用统一整数下注策略：最低 10、无产品层固定最高上限、快捷金额 10 / 100 / 500 / 1000。
3. 首次进入默认下注 10；每款游戏只记忆自身最近有效金额；不同游戏之间不共享。
4. 超过余额或模式总成本的快捷金额禁用；不提供 Max、百分比或 All-in。
5. 五款游戏均不提供自动连续扣款和游戏专属 Free Round。
6. 所有正式结果由服务端权威确定，动画和用户揭示行为不得决定或改变结果。
7. 已接受 Round 在刷新、断线、重复请求与维护切换后必须恢复同一结果或进入正式退款，不得创建重复 Round。
8. Dice 使用三颗六面骰的 Big / Small，三同号通杀，固定 1:1，理论 RTP 97.222222%。
9. Scratch 使用 3 × 3 九格任意位置三同匹配，默认奖表 RTP 96%，支持按住拖动刮开和 Reveal All。
10. Summon 提供 Single / Tenfold，使用 0x / 1x / 2x / 5x / 20x / 100x 奖表，默认 RTP 96%，不提供保底、Pity、Rate-up 或并行功能 Pool。
11. Slot 使用 5 × 3 Grid、10 条固定 Payline、7 个普通符号加 1 个 Wild、五条 32 Stop Reel Strip，当前理论 RTP 约 96.0033119%。
12. Blackjack 使用每 Round 全新六副牌、American Hole-card、Dealer Peek、S17、Blackjack 3:2，并支持 Hit / Stand / Double / Split。
13. Blackjack 的参考 RTP / House Edge 必须通过冻结规则与参考 Basic Strategy 的可重复验证器计算后发布。
14. 五款游戏的具体视觉资产和演出分镜仍未冻结，统一进入 Art Direction v0.4。
15. 后续增加游戏时继续遵守 IA-06 可扩展 Game Registry 与通用 Game Shell，不把五款首发游戏写成永久上限。

---

---

# 145. IA-08 — Poker Lobby & Poker Table Information Architecture

本节定义 Poker V1 的产品边界、Lobby、Table、资金语义、牌局操作、观战与聊天、房主能力、断线恢复、Safe Leave、历史记录、Provably Fair 以及 PC / Mobile 响应式行为。

本节已经按照产品确认的 `1A–78A` 全部冻结。

本阶段继续不冻结：

- Poker 牌桌的最终美术构图；
- 扑克牌、牌背和筹码的具体样式；
- FGO 角色、职阶和世界观映射；
- 色板、字体、粒子、光效与音效；
- 牌桌动画的具体时长与镜头。

这些内容统一进入 Art Direction v0.4，不得反向改变本节已经冻结的规则、资产语义、操作流程和恢复行为。

---

# 146. Poker V1 产品边界

Poker V1 定义为：

**No-Limit Texas Hold’em Cash Game / 无限注德州扑克现金桌。**

V1 使用真人玩家对战，不使用系统机器人。

V1 不包含：

- Sit & Go；
- 多桌锦标赛；
- Limit Hold’em；
- Pot-Limit Hold’em；
- Short Deck；
- Omaha；
- Straddle；
- Bomb Pot；
- Run It Twice；
- Rabbit Hunt；
- Poker Insurance；
- Side Bet；
- Rake；
- 真实货币结算。

Poker V1 继续只使用两个主要页面：

```text
/poker
→ Poker Lobby

/poker/table/:table_id
→ Poker Table
```

Create Table、Private Password、Seat Selection、Buy-in 与 Rebuy 均在 Lobby 或 Table 内部短流程中完成，不增加独立创建页面。

---

# 147. Poker 核心业务对象

Poker 页面和历史记录必须区分 Table、Seat、Hand 与 Session，不能将四者混为一个“房间记录”。

## 147.1 Poker Table

Poker Table 是持续存在的一张牌桌，至少拥有：

- Table ID；
- Table Name；
- Public / Password 属性；
- Max Seats；
- Blind / Ante Preset；
- Buy-in 范围；
- Spectator 配置；
- Table Chat 配置；
- 接受新玩家状态；
- 运行状态；
- 创建者 / 房主。

## 147.2 Poker Seat

Poker Seat 表达玩家在牌桌中的占位和实时状态，至少包括：

- Master 身份；
- Seat Position；
- Table Stack；
- 是否参与当前 Hand；
- 是否 Ready；
- 是否 Sit Out；
- 是否掉线；
- 是否等待 Big Blind；
- 是否申请 Leave After Hand；
- 是否等待 Top-up / Rebuy 生效。

## 147.3 Poker Hand

Poker Hand 从一手牌开始发牌，到所有 Pot 完成唯一 Settlement 为止。

阶段包括：

```text
Preflop
→ Flop
→ Turn
→ River
→ Showdown
→ Settlement
```

如果所有仍在局玩家提前只剩一人，或全部 All-in 后不再需要行动，可以提前进入确定性的发牌、Showdown 或 Settlement 流程。

## 147.4 Poker Session

Poker Session 从用户第一次成功 Buy-in 并正式入座开始，到 Safe Leave / Cash Out 完成为止。

第一次成功 Buy-in / 入座时，Poker Session 冻结当时的 Master Nickname 与 Display Avatar Snapshot。Session 进行中修改 Master Profile 不改变当前牌桌身份；完成 Cash Out 后，下一次入座使用新的 Profile。真实归属始终使用 `newapi_user_id`。

一个 Session 可以包含：

- 多个 Hand；
- 多次 Top-up / Rebuy；
- Sit Out；
- 掉线与重连；
- Take Over Control；
- Leave After Hand。

正式 Session 盈亏：

```text
Session Realized P/L
=
Final Cash Out
-
全部成功 Buy-in 与 Top-up
```

仍在桌上时可以显示 Current Session Delta，但必须标记为 Unrealized，不进入正式 Poker 盈利榜。

---

# 148. Poker Lobby 信息架构

Poker Lobby 是正常 Chaldea 页面，不使用 Poker Table 的沉浸式 Shell。

页面包含：

```text
Poker Lobby
│
├── Active Session / Reconnect
├── Poker Service Status & Asset Summary
├── Table Search / Filter / Sort
├── Public & Password Table List
├── Create Table
├── Join by Table ID / Deep Link
└── Poker History Entry
```

## 148.1 Active Session / Reconnect

如果用户已经在某张桌成功 Buy-in，Lobby 最高优先级显示唯一 Active Poker Session：

- Table Name；
- Table ID；
- Blind / Ante；
- Current Table Stack；
- Poker In Play；
- Connection State；
- Reconnect to Table。

V1 一个用户同时只能在一张 Poker Table 入座。

存在 Active Session 时：

- 不允许加入另一张桌；
- 不允许创建另一张桌；
- 不允许观战另一张桌；
- 点击其他桌时，引导用户 Reconnect 或先完成 Safe Leave。

## 148.2 Poker Status & Asset Summary

Lobby 轻量展示：

- Poker Service 当前状态；
- Available Entertainment Chips；
- Poker In Play；
- Wallet 入口；
- Poker History 入口。

Lobby 不复制完整 Wallet，也不把未实现 Poker 盈亏包装成正式盈利。

## 148.3 Table Search / Filter / Sort

Lobby 支持：

- Table ID 搜索；
- Table Name 搜索；
- Public / Password 筛选；
- 有空座筛选；
- Max Seats 筛选；
- Blind Preset 筛选；
- Waiting / In Hand / Paused 等状态筛选；
- Allow Spectators 筛选；
- 低盲注优先；
- 高盲注优先；
- 接近满桌优先。

桌况应实时更新，不要求用户刷新整页。

## 148.4 Table List Item

每张桌至少展示：

- Table Name；
- Table ID；
- Public / Password；
- SB / BB / Ante；
- Minimum / Maximum Buy-in；
- 当前人数 / Max Seats；
- 当前状态；
- 是否允许观战；
- 是否启用聊天；
- Join / Spectate / Reconnect 等状态化操作。

---

# 149. Public Table、Password Table 与访问控制

## 149.1 Public Table

Public Table：

- 在 Lobby 正常显示；
- 可以通过 Table ID 或 Deep Link 进入；
- 有空座时可以申请入座；
- 满桌但允许观战时可以 Spectate；
- 不需要房间密码。

## 149.2 Password Table

V1 的 Private Table 使用“大厅可见、密码保护”的方式：

- 在 Lobby 显示锁定状态；
- 可以显示 Table Name、Blinds、Seats 和运行状态；
- 用户点击后输入密码；
- 密码同时保护入座、观战和牌桌聊天访问。

密码错误时：

- 不创建 Seat Reservation；
- 不产生 Buy-in；
- 不创建 Active Session；
- 不进入可见牌桌状态。

## 149.3 V1 不提供 Unlisted Table

V1 不提供完全从 Lobby 隐藏的：

- Invite-only Table；
- Unlisted Table；
- 一次性秘密链接房间。

以后如增加，需要作为新的隐私与邀请机制单独设计。

---

# 150. Create Table

Create Table 在 Lobby 中通过 Modal、Drawer 或 Mobile Sheet 完成，不建立 `/poker/create`。

## 150.1 创建资格

发起创建时，用户必须：

- 已登录；
- 已完成 Master Profile 初始化；
- 当前没有 Active Poker Session；
- 当前没有另一张由其拥有且尚未关闭的 Poker Table。

每名用户同时最多拥有一张未关闭牌桌。

## 150.2 创建字段

创建字段冻结为：

- Table Name；
- Public / Password；
- Password（仅 Password Table）；
- Max Seats：2–9；
- Blind / Ante Preset；
- Allow / Disable Spectators；
- Enable / Disable Table Chat。

房主不能直接输入任意 Blind、Ante 或 Buy-in 范围。

## 150.3 参数锁定

第一笔 Buy-in 成功后：

- Max Seats 锁定；
- Blind / Ante Preset 锁定；
- Buy-in 范围锁定；
- Spectator 开关锁定；
- 已经运行的经济配置不得修改。

房主仍可以暂停或恢复接受新玩家，但不能借此改变已锁定的牌局经济规则。

## 150.4 空桌自动关闭

完全无人入座且无人观战的牌桌持续 30 分钟后自动关闭。

自动关闭前：

- 不存在未完成 Hand；
- 不存在未完成 Buy-in / Cash Out；
- 不存在真实 Poker In Play 资产。

---

# 151. Blind、Ante、Buy-in 与 Top-up

## 151.1 Blind / Ante Preset

Blind 和 Ante 使用运营后台发布的完整 Preset。

V1 首发 SB / BB：

```text
5 / 10
10 / 20
25 / 50
50 / 100
100 / 200
500 / 1000
```

每档可以存在：

```text
No Ante
```

或：

```text
Ante = 10% BB
```

例如：

```text
50 / 100，Ante 0
50 / 100，Ante 10
```

房主不能创建任意非标准档位。

## 151.2 Buy-in 范围

每张桌统一采用：

```text
Minimum Buy-in = 40 BB
Maximum Buy-in = 100 BB
```

例如 `5 / 10`：

```text
Minimum Buy-in = 400
Maximum Buy-in = 1,000
```

## 151.3 默认 Buy-in

默认选择 100 BB。

用户余额不足 100 BB、但至少拥有 40 BB 时，默认选择其当前可负担且不低于 40 BB 的最大整数金额。

用户余额低于 40 BB 时：

- 不能 Buy-in；
- 显示最低 Buy-in；
- 提供 Wallet；
- 提供 Rewards Center；
- 返回后刷新余额，但不自动提交 Buy-in。

## 151.4 Poker 金额精度

Poker V1 以下金额只使用整数娱乐筹码：

- Blind；
- Ante；
- Buy-in；
- Top-up / Rebuy；
- Bet；
- Call；
- Raise；
- Pot；
- Side Pot；
- Stack；
- Cash Out。

钱包中的小数筹码继续留在娱乐钱包，不进入 Poker Table。

## 151.5 Top-up / Rebuy

允许 Top-up，但规则为：

- 只在 Hand 边界正式生效；
- 当前 Hand 中申请时显示 Pending Top-up；
- Hand 结束后再次由服务端校验钱包余额；
- 成功扣除钱包后才增加 Table Stack；
- Top-up 后最多补到 100 BB；
- 玩家通过牌局获胜使 Stack 超过 100 BB 时，不强制降低；
- V1 不提供 Auto Rebuy。

如果 Pending Top-up 生效时钱包余额不足：

- Top-up 不执行；
- 不透支；
- 不产生部分到账；
- 明确显示失败原因；
- 原有 Stack 不受影响。

## 151.6 Partial Cash Out

V1 不允许玩家保持 Seat 的同时提取部分 Table Stack。

任何 Cash Out 都必须通过 Safe Leave 完成。

---

# 152. Seat Selection、Seat Reservation 与加入运行中牌桌

## 152.1 Seat Selection

标准流程：

```text
打开牌桌或 Join Flow
→ 查看空座
→ 选择一个空座
→ Seat Reserved
→ 选择 Buy-in
→ Confirm Buy-in
→ 钱包扣款与 table_stack 创建原子完成
→ 正式入座或等待下一个 Hand 生效
```

## 152.2 Seat Reservation

Seat Reservation 有效期为 30 秒。

30 秒内未完成成功 Buy-in：

- Reservation 自动释放；
- 不扣钱包；
- 不创建 table_stack；
- 不创建 Active Poker Session。

Seat Reservation 不应长期阻止牌桌继续开局。

## 152.3 加入尚未开始第一手的牌桌

牌桌尚未开始第一手时：

- 玩家正常入座；
- 第一手开始时按照 Dealer Button 分配 Blind；
- 不额外收取进入 Big Blind。

## 152.4 加入正在运行的牌桌

牌桌已经运行时，新玩家选择：

```text
Wait for Big Blind
```

或：

```text
Post Big Blind Now
```

### Wait for Big Blind

- 玩家保持等待状态；
- 自然轮到 Big Blind 时开始参与；
- 等待期间不发 Hole Cards。

### Post Big Blind Now

- 在下一次可加入的 Hand 补发一个进入 Big Blind；
- 该金额属于牌桌投入；
- 不属于 Raise；
- 由服务端决定 Pot 归属；
- 不允许客户端跳过或伪造。

## 152.5 Hand Start 条件

至少存在两名：

- 已正式入座；
- Ready；
- 未 Sit Out；
- Stack > 0；

的玩家时，才能开始新 Hand。

## 152.6 Hand Intermission

Hand 之间保留 5 秒 Intermission，用于：

- 展示上一手结算；
- 使 Pending Top-up 生效；
- 处理 Sit Out；
- 处理 Safe Leave；
- 激活已完成 Buy-in 的新 Seat；
- 准备下一手。

新玩家不得在 Hand 中途获得 Hole Cards。

---

# 153. No-Limit Texas Hold’em 核心规则

Poker V1 使用：

- 52 张标准扑克牌；
- 不使用 Joker；
- 每名玩家 2 张 Hole Cards；
- 最多 5 张 Community Cards；
- 从最多 7 张牌中选取最佳 5 张构成最终牌型。

牌型从高到低：

```text
Royal Flush
Straight Flush
Four of a Kind
Full House
Flush
Straight
Three of a Kind
Two Pair
One Pair
High Card
```

下注轮：

```text
Preflop
Flop
Turn
River
```

## 153.1 Dealer Button 与三人以上牌桌

Dealer Button 每 Hand 顺时针移动。

三人及以上：

```text
Button 左侧第一位 = Small Blind
Small Blind 左侧第一位 = Big Blind
Preflop = Big Blind 左侧第一位仍有行动权玩家先行动
Flop / Turn / River = Button 左侧第一位仍在局玩家先行动
```

## 153.2 Heads-up

Heads-up：

```text
Button = Small Blind
另一名玩家 = Big Blind
```

行动顺序：

```text
Preflop：Button / Small Blind 先行动
Flop / Turn / River：Big Blind 先行动
```

---

# 154. 玩家行动、Bet 控件与 Action Timer

## 154.1 合法操作

根据服务端当前状态动态提供：

- Fold；
- Check；
- Call；
- Bet；
- Raise；
- All-in。

不合法操作必须隐藏或禁用，并说明不可用原因。

客户端不得自行决定合法 Action Set。

## 154.2 Bet / Raise 控件

提供：

```text
Min
1/2 Pot
2/3 Pot
Pot
All-in
Slider
Integer Input
```

所有金额由服务端根据以下状态计算或校验：

- Current Pot；
- To Call；
- Current Stack；
- Minimum Legal Bet；
- Minimum Legal Raise；
- Previous Full Raise Increment；
- Pot / Side Pot 状态。

`1/2 Pot` 与 `2/3 Pot` 产生非整数时向下取整，但结果不得低于合法最小 Bet / Raise。

## 154.3 Minimum Raise

No-Limit 最小 Raise 采用上一笔完整 Raise 增量规则。

服务端必须返回：

- Minimum Raise To；
- Maximum Raise To；
- 当前合法操作集合。

## 154.4 Short All-in

玩家可以 All-in 到低于最小完整 Raise 的金额。

Short All-in：

- 是合法投入；
- 可以形成 Main Pot / Side Pot；
- 不构成新的完整 Raise；
- 不重新开放已经行动玩家的 Raise 权利。

## 154.5 Action Timer

每个需要玩家决策的节点使用 30 秒服务端倒计时。

V1 不提供 Time Bank。

超时行为：

```text
当前可以 Check
→ Auto Check

必须投入更多筹码才能继续
→ Auto Fold
```

V1 不自动 Call、不自动 Bet、不自动 Raise。

## 154.6 连续超时

连续两次由超时触发 Auto Check / Auto Fold 后：

- 当前 Hand 仍按服务端状态正确完成；
- 下一手自动 Sit Out。

玩家在倒计时内成功完成一次人工操作后，连续超时计数重新开始。

## 154.7 V1 不提供高级 Pre-action

V1 不提供：

- Check/Fold Pre-action；
- Call Any；
- Auto Call；
- Auto Raise；
- 预设策略。

所有仍在局玩家已经 All-in 且不再存在决策时，服务端自动发完剩余 Board。

---

# 155. Sit Out、Stack 归零与 Rebuy

## 155.1 Sit Out Next Hand

提供 `Sit Out Next Hand`。

玩家当前正在参与 Hand 时：

- 当前 Hand 不取消；
- 当前行动责任仍存在；
- Sit Out 从下一手开始生效。

## 155.2 Sit Out 状态

Sit Out 后：

- Seat 保留；
- Table Stack 保留；
- Active Poker Session 保留；
- 用户不能加入或观战另一张桌；
- 用户可以回到当前桌恢复；
- 恢复时根据错过 Blind 状态选择 Wait for BB 或 Post BB Now。

## 155.3 Sit Out / Disconnect 自动离桌

连续 Sit Out 或掉线达到 15 分钟，并且当前不存在未结算 Hand 时：

```text
Auto Safe Leave
→ Cash Out
→ Session Settlement
```

## 155.4 Stack 归零

玩家 Table Stack 变为 0 后：

- Seat 保留 60 秒；
- 可以发起 Rebuy；
- 可以前往 Wallet；
- 可以前往 Rewards Center；
- 返回时恢复同一桌与 Session。

60 秒内没有成功 Rebuy：

```text
自动离座
→ Cash Out 0
→ Poker Session Settlement
```

V1 不提供 Auto Rebuy。

---

# 156. Main Pot、Side Pot、Showdown 与 Odd Chip

## 156.1 Pot 生成

服务端必须：

- 按实际投入创建 Main Pot；
- 按 All-in 投入层级创建 Side Pot；
- 为每个 Pot 保存 Eligible Players；
- Fold 玩家不再具有对应 Pot 获胜资格；
- 每个 Pot 独立判定获胜者；
- 同一筹码不能重复进入多个 Pot；
- 同一 Pot 只能 Settlement 一次。

## 156.2 Split Pot

同一 Pot 出现并列最佳牌型时，按照有资格的并列获胜者平均分配。

## 156.3 Odd Chip

整数筹码无法平均分配时，Odd Chip 按固定规则分配：

> 从 Dealer Button 左侧开始，顺时针找到第一位对该 Pot 有资格的并列获胜者。

Hand History 必须记录 Odd Chip 最终归属。

## 156.4 All-in Showdown

所有仍在局玩家均已 All-in，且后续不存在下注决策时：

- 自动公开所有仍在局玩家的 Hole Cards；
- 自动发完剩余 Community Cards；
- 按 Main Pot / Side Pot 分别结算。

## 156.5 普通 Showdown 与 Muck

普通 Showdown：

- 公开确定 Pot 获胜者所必需的手牌；
- 输家可以 Muck；
- 已 Fold 的牌在实时牌桌中保持隐藏；
- V1 不提供主动 Show Folded Cards。

24 小时后参与者可能通过 Provably Fair 完整牌序重建 Folded Cards，属于第 163 章明确披露的验证边界，不改变实时牌桌中的 Muck / Hidden 语义。

## 156.6 Hand 配置锁定

每个 Hand 创建时必须锁定：

- Blind Preset；
- Ante；
- Poker Rule Version；
- Algorithm Version；
- Game Config Version；
- Config Hash；
- Server Seed Hash；
- Effective Client Seed。

---

# 157. Spectator 与 Table Chat

## 157.1 Spectator 开关

房主创建桌子时选择 Allow Spectators 或 Disable Spectators。

第一笔 Buy-in 成功后，该设置锁定。

## 157.2 Spectator 可见信息

允许观战时，Spectator 可以看到：

- 玩家 Master 昵称与头像；
- Seat；
- Public Table Stack；
- Dealer Button；
- Blind / Ante；
- 玩家公开 Action；
- Community Cards；
- Main Pot / Side Pot；
- 已公开的 Showdown Cards；
- 牌桌聊天与系统消息。

Spectator 不能看到：

- 未公开 Hole Cards；
- 已 Fold 且未公开的 Hole Cards；
- Client Seed 原始贡献；
- 尚未公开的完整牌序；
- 玩家私有 Wallet；
- 房主管理私有信息。

V1 使用实时观战，不增加观战延迟。

## 157.3 Table Chat

V1 只支持：

- 文字消息；
- 系统牌局消息。

不支持：

- 图片；
- 文件；
- 语音；
- 私聊；
- 全站聊天室。

Table Chat 默认开启，房主可在创建时关闭。

启用时：

- 已入座玩家可以发言；
- 已通过访问控制的 Spectator 可以发言；
- 用户可以在本地屏蔽其他聊天用户；
- 系统消息与用户消息必须明确区分；
- 手机端聊天不得遮挡 Action Timer 或 Action Area。

---

# 158. 房主权限与关闭牌桌

## 158.1 房主可以执行

房主可以：

- 暂停接受新玩家；
- 恢复接受新玩家；
- 将某位已入座玩家标记为 Hand 结束后移除；
- 安全移除 Spectator；
- Mute 聊天用户；
- 发起关闭牌桌。

## 158.2 房主不得执行

房主不能：

- 查看其他玩家未公开 Hole Cards；
- 修改牌序；
- 修改 Pot / Side Pot；
- 修改玩家 Stack；
- 修改已经锁定的 Blind / Ante / Buy-in；
- 指定赢家；
- 没收筹码；
- 跳过正式 Cash Out；
- 对已经结算的 Hand 再次 Settlement。

## 158.3 移除已入座玩家

如果被移除玩家正在当前 Hand 中：

```text
当前 Hand 正常完成
→ 唯一 Settlement
→ Safe Leave
→ Cash Out
→ Seat Removed
```

不得直接删除 Seat 或遗失 Table Stack。

## 158.4 房主离开

房主离开 Seat 或关闭浏览器后：

- 牌桌不立即关闭；
- 已有玩家继续正常游戏；
- 房主身份不自动转移；
- 房主重新进入后仍保留管理权限；
- 完全空置后适用 30 分钟自动关闭。

## 158.5 主动关闭牌桌

房主发起关闭后：

- 牌桌进入 Closing；
- 禁止新玩家进入；
- 禁止创建新 Hand；
- 当前 Hand 正常完成；
- 所有 Seat 依次 Safe Leave / Cash Out；
- 所有资产处理完成后进入 Closed。

---

# 159. 掉线、重连、服务重启与多设备控制

## 159.1 网络状态

Poker Table 必须明确显示：

- Connected；
- Reconnecting；
- Disconnected；
- Server Paused。

具体中文和视觉由后续设计确定，但语义必须清楚。

## 159.2 客户端掉线

客户端掉线时：

- Action Timer 继续；
- 到期后按 Auto Check / Auto Fold；
- 当前 Hand 完成后自动 Sit Out；
- 15 分钟内允许恢复；
- 超时且不存在未结算 Hand 时自动 Safe Leave / Cash Out。

## 159.3 Reconnect Snapshot

重连后，从 Poker Service 获取服务端权威 Snapshot，恢复：

- Table；
- Seat；
- Poker Session；
- 当前 Hand；
- 自己的 Hole Cards；
- Community Cards；
- Main Pot / Side Pot；
- Table Stack；
- Committed This Hand；
- Action History；
- 当前 Action Player；
- 剩余 Action Time；
- Sit Out / Leave / Top-up 状态。

如果 Action 已经超时，显示服务端实际执行的自动操作，不重放旧 Action。

## 159.4 Poker Service 故障或重启

Poker Service 故障时：

- Table 进入 Paused；
- Action Timer 暂停；
- 不根据客户端本地时间自动 Fold；
- 从 PostgreSQL 恢复正式资产和牌局状态；
- Redis 只作为缓存与辅助恢复；
- 恢复后给予玩家 30 秒 Reconnect Grace；
- Grace 结束后继续原 Hand。

任何真实资产、Pot、Settlement 与正式 Hand 状态不得只保存在 Redis。

## 159.5 单一控制连接

同一已入座账号同时只能拥有一个 Active Control Connection。

第二台设备进入时：

```text
检测已有控制连接
→ 显示 Take Over Table
→ 用户明确确认
→ 新设备取得控制权
→ 旧设备转为只读并失去行动权限
```

所有行动继续使用唯一：

- action_id；
- action_sequence；
- Hand / Seat 版本；

防止多设备重复提交。

---

# 160. Safe Leave 与 Cash Out

## 160.1 未参与当前 Hand

玩家未参与当前 Hand 时：

```text
Leave Table
→ Confirm
→ Cash Out
→ Seat Leave
→ Session Settlement
→ Poker Lobby
```

## 160.2 正在参与当前 Hand

玩家正在参与当前 Hand 时：

```text
Leave Table / 返回大厅 / Browser Back
→ Leave After Hand
→ 当前 Hand 正常完成
→ Settlement
→ Cash Out
→ Session Settlement
→ Poker Lobby
```

## 160.3 已经 Fold

即使玩家已经 Fold：

- 当前 Hand 的自身投入仍在 Pot；
- 必须等待当前 Hand Settlement；
- Settlement 后才能最终 Cash Out。

## 160.4 关闭浏览器

直接关闭浏览器：

- 不取消 Hand；
- 不退款当前已合法投入；
- 到行动时按 Auto Check / Auto Fold；
- Hand 完成后 Sit Out；
- 后续按 15 分钟规则自动 Safe Leave。

## 160.5 返回大厅与 Browser Back

已入座时：

- 返回大厅；
- Leave Table；
- Browser Back；

全部进入统一 Safe Leave 语义，不允许绕过资产处理。

Spectator 未入座时可以直接返回 Lobby。

## 160.6 Cash Out 幂等

Cash Out 必须：

- 与 Table Stack 清零原子完成；
- 使用唯一业务 ID；
- 重复请求只能返回原结果；
- 不允许 Partial Cash Out；
- 不允许重复到账；
- Session 只完成一次最终 Settlement。

---

# 161. Poker 资产语义与排行榜结算点

Poker Table 和 Wallet 统一使用以下资产语义：

```text
Wallet Chips
= 尚未 Buy-in、仍可在主钱包使用的筹码

Table Stack
= 当前尚可用于后续行动的桌面筹码

Committed This Hand
= 已投入当前 Hand、尚未完成 Settlement 的自身筹码

Poker In Play
= Table Stack + Committed This Hand
```

## 161.1 Buy-in

Buy-in：

```text
Available Chips Decrease
Poker In Play Increase
Total Assets Unchanged
```

Buy-in 不是消费，也不是 Poker 亏损。

## 161.2 Hand 中投入 Pot

玩家下注后：

- Table Stack 减少；
- Committed This Hand 增加；
- Poker In Play 在 Hand Settlement 前保持资产可解释；
- 不允许用户总资产无说明地减少。

## 161.3 Hand Settlement

Hand Settlement 后：

- 获胜者获得其有资格 Pot；
- 失败者相应资产完成所有权转移；
- 新 Table Stack 成为下一手基础；
- 当前 Session 仍未正式 Cash Out。

## 161.4 Unrealized 与 Realized P/L

Session 进行中：

- 可以显示 Current Session Delta；
- 必须标记 Unrealized；
- 不进入今日 Poker 盈利、本周 Poker 盈利、历史净盈利或 Poker 盈利榜。

完成 Safe Leave / Cash Out 后：

```text
Realized P/L
=
Final Cash Out
-
全部 Buy-in / Top-up
```

此时才进入正式排行榜与统计。

---

# 162. Poker History

Poker History 使用两层详情：

```text
Session Detail
└── Hand Detail List
    └── Hand Detail
```

## 162.1 Session Detail

至少展示：

- Session ID；
- Table ID / Name；
- Public / Password 标识；
- Blind / Ante；
- 入座时间；
- 离桌时间；
- Initial Buy-in；
- 所有 Top-up / Rebuy；
- Final Cash Out；
- Realized P/L；
- 参与 Hand 数量；
- Hand List；
- 结束原因：Safe Leave、Auto Cash Out、Kicked、Table Closed 等。

## 162.2 Hand Detail

至少展示：

- Hand ID；
- Table 与规则版本；
- Seat 与 Dealer Button；
- Blind / Ante；
- 每位玩家起始 Stack；
- 当前用户有权限查看的 Hole Cards；
- Community Cards；
- 完整 Action Timeline；
- Main Pot / Side Pot；
- Eligible Players；
- Showdown；
- 各 Pot 获胜者；
- Odd Chip；
- Settlement；
- Hand 前后 Stack；
- Algorithm / Config Version；
- Provably Fair 信息。

## 162.3 V1 不制作动画 Hand Replay

V1 使用可审计的 Action Timeline，不制作复杂动画 Replay。

后续若增加动画回放，必须以已保存的正式 Action 与牌序重建，不得生成与历史不一致的演出。

---

# 163. Poker Provably Fair

## 163.1 Hand 级公平承诺

每个 Hand：

- 发牌前提交 Server Seed Hash；
- 锁定参与玩家；
- 锁定每位参与玩家的 Client Seed Contribution；
- 生成 Effective Client Seed；
- 一次性确定完整 52 张牌顺序；
- 发牌过程中不得重新洗牌或临时生成下一张牌。

## 163.2 Effective Client Seed

Effective Client Seed 至少确定性组合：

```text
Table ID
Hand ID
所有该 Hand 参与者的 Client Seed Contribution
固定排序规则
```

用户可以在 Fairness 面板查看并修改**下一手**使用的 Client Seed Contribution。

Hand 开始后：

- 当前 Hand 的 Client Seed 不可修改；
- 离桌或重连不能改变当前牌序；
- 多设备 Take Over 不能生成新牌序。

## 163.3 完整牌序验证与隐私边界

完整可验证牌序与永久隐藏 Folded Cards 在简单 Seed Reveal 模型下无法同时实现。

V1 冻结以下折中：

### Hand 结束后立即公开

- Server Seed Hash；
- Hand ID；
- Algorithm Version；
- Config Version；
- Config Hash；
- 公共 Board；
- 公开 Action；
- Public Settlement 数据。

### Hand Settlement 24 小时后

仅向该 Hand 的实际参与玩家开放：

- Server Seed；
- Effective Client Seed 组合信息；
- 完整 52 张牌顺序；
- 完整验证工具所需数据。

Spectator 与普通公共记录不获得完整 Server Seed。

## 163.4 Folded Cards 披露提示

参与 Hand 即表示接受：

> Hand Settlement 24 小时后，其他该 Hand 参与者可能通过完整验证数据重建当时已 Fold 或 Muck 的 Hole Cards。

平台应在 Fairness 规则中明确披露该限制。

完整 Seed 一旦向参与者开放，平台无法保证参与者不会另行分享。

V1 不实现：

- 零知识发牌证明；
- 按玩家加密牌序证明；
- 永久隐藏 Folded Cards 的高级密码学协议。

---

# 164. PC Poker Table 信息结构

PC Poker Table 使用 Full Immersive Layout，隐藏普通 Chaldea Global Header、Entertainment Context Navigation 与普通页面 Bottom Navigation。

建议的信息职责结构：

```text
Poker Table — PC
│
├── Top Control Bar
│   ├── 返回大厅
│   ├── Table / Blind / Ante
│   ├── Wallet Chips / Table Stack
│   ├── Network State
│   ├── Settings
│   └── Leave Table
│
├── Main Table
│   ├── 2–9 Seats
│   ├── Dealer Button
│   ├── Hole Cards
│   ├── Community Cards
│   ├── Main Pot / Side Pot
│   ├── Current Action Player
│   └── Hand State
│
├── Player Action Area
│   ├── Action Timer
│   ├── Fold / Check / Call
│   ├── Bet / Raise Controls
│   ├── All-in
│   └── Sit Out Next Hand
│
└── Side Drawer / Panel
    ├── Table Chat
    ├── Action Timeline
    ├── Rules
    ├── Fairness
    └── Session Info
```

PC 需要确保：

- 9 人桌仍能识别所有 Seat；
- Action Area 始终高可见；
- Side Pot 不因空间不足被折叠成不可理解的一个总 Pot；
- Chat 不遮挡当前行动；
- Take Over、Reconnect、Leave After Hand 等状态明确。

---

# 165. Mobile Poker Table 信息结构

手机竖屏和横屏采用同一业务状态，不重建 WebSocket Session，不重置 Action Timer，不创建新的 Hand Snapshot。

## 165.1 Portrait

手机竖屏必须完整可玩，不强制横屏。

优先级：

1. 自己的 Hole Cards；
2. 当前 Action 与倒计时；
3. Board；
4. Pot / Side Pot；
5. 当前行动玩家；
6. 自己的 Table Stack；
7. 其他 Seat 状态；
8. Chat / Rules / Fairness。

Portrait 行为：

- 2–9 Seat 均可识别；
- Action Tray 固定在安全区域；
- Chat、Rules、Fairness 使用 Sheet；
- Chat 不得遮挡行动按钮；
- 不显示 Chaldea Mobile Bottom Navigation；
- 网络与 Leave 状态始终可发现。

## 165.2 Landscape

横屏：

- 扩大牌桌与座位空间；
- 可以同时显示更多 Action History 或 Chat；
- 保留同一 Hand、Seat、Timer 与 Socket；
- 旋转屏幕不得触发重连为新 Session。

## 165.3 Mobile Lobby

Mobile Poker Lobby 不压缩 PC 宽表格。

每张 Table 使用可阅读的列表项或 Card，重点显示：

- Table Name / ID；
- Lock；
- Blind / Ante；
- Seats；
- Buy-in；
- Status；
- Join / Spectate。

Filter、Create Table、Password、Buy-in、Seat Selection 使用 Mobile Sheet 或全屏短流程。

---

# 166. Maintenance、Pause 与异常关闭

## 166.1 Poker Maintenance

进入维护后：

- 禁止创建新 Table；
- 禁止新 Buy-in；
- 禁止新 Seat；
- 禁止开始新 Hand；
- 已经开始的 Hand 优先正常完成；
- 无法继续时进入 Paused / Recovery；
- 不得通过删除 Table 遗弃资产。

## 166.2 已开始 Hand

已经开始的 Hand：

- 不因维护切换直接退款；
- 优先从 PostgreSQL 恢复并继续；
- 只有无法重建正式状态时，才进入确定性退款或管理员处理流程；
- 同一 Hand 不得既退款又派奖。

## 166.3 Table Closing

Table Closing 与平台 Maintenance 均必须：

- 阻止新 Hand；
- 完成或恢复当前 Hand；
- 完成所有 Cash Out；
- 确认 Poker In Play 归零或有明确异常记录；
- 再进入 Closed。

---

# 167. Poker 核心 UX Flow

## 167.1 浏览并加入 Public Table

```text
Poker Lobby
→ Search / Filter Table
→ Open Public Table
→ Select Empty Seat
→ Seat Reserved 30s
→ Select Buy-in 40–100 BB
→ Confirm
→ Wallet Debit + table_stack Create
→ Seat Active / Waiting Next Hand
→ Poker Table
```

## 167.2 加入 Password Table

```text
Poker Lobby
→ Open Locked Table
→ Enter Password
→ Server Verify
→ View Seats
→ Select Seat
→ Buy-in
→ Poker Table
```

错误密码不创建 Seat、Session 或资产变化。

## 167.3 创建牌桌

```text
Poker Lobby
→ Create Table
→ Name / Publicity / Seats / Blind Preset / Spectator / Chat
→ Confirm
→ Table Created
→ Lobby / Table Preview
→ First Buy-in locks economic and seat settings
```

## 167.4 加入运行中牌桌

```text
Seat + Buy-in Success
→ Choose Wait for BB / Post BB Now
→ Wait for Hand Boundary
→ Join Eligible Hand
```

## 167.5 正常牌局

```text
At least 2 Ready Players
→ Hand Created
→ Seed Hash Committed
→ Full Deck Locked
→ Blind / Ante Posted
→ Hole Cards
→ Preflop
→ Flop
→ Turn
→ River
→ Showdown
→ Main / Side Pot Settlement
→ 5s Intermission
→ Next Hand
```

## 167.6 Top-up

```text
Request Top-up
→ Pending
→ Current Hand Settlement
→ Revalidate Wallet
→ Wallet Debit + Stack Credit
→ Next Hand
```

## 167.7 掉线恢复

```text
Disconnected
→ Timer Continues
→ Auto Check / Fold if Needed
→ Reconnect
→ Fetch Authoritative Snapshot
→ Resume Same Table / Seat / Hand
```

## 167.8 Poker Service 恢复

```text
Service Failure
→ Table Paused
→ Timer Paused
→ PostgreSQL Recovery
→ 30s Reconnect Grace
→ Resume Same Hand
```

## 167.9 Safe Leave

```text
Leave Intent
→ If In Hand: Leave After Hand
→ Settlement
→ Cash Out
→ Session Realized P/L
→ Poker Lobby
```

## 167.10 Stack 归零

```text
Stack = 0
→ 60s Rebuy Window
→ Rebuy Success: Continue Session
→ No Rebuy: Auto Seat Leave + Session Settlement
```

---

# 168. IA-08 最终冻结结论

以下 78 项与用户确认的 `1A–78A` 一一对应：

1. Poker V1 为 No-Limit Texas Hold’em Cash Game。
2. 使用 52 张标准牌，不使用 Joker。
3. 每张桌的 Max Seats 在创建时选择 2–9 人。
4. 每人两张 Hole Cards，使用最多七张牌组成最佳五张。
5. 使用 Preflop、Flop、Turn、River、Showdown 流程。
6. V1 不收 Rake。
7. V1 不加入系统机器人。
8. V1 不加入 Straddle、Bomb Pot、Run It Twice、Rabbit Hunt、保险或 Side Bet。
9. 一个用户同时只能在一张桌入座，入座期间不能观战另一桌。
10. Blind / Ante 使用运营后台发布的完整 Preset。
11. 首发 SB/BB 为 `5/10、10/20、25/50、50/100、100/200、500/1000`。
12. 每档可以有 No Ante 或 Ante = 10% BB 的版本。
13. 房主不能自行输入任意 Blind / Ante。
14. Minimum Buy-in = 40 BB，Maximum Buy-in = 100 BB。
15. 默认 Buy-in = 100 BB；余额不足时选择可负担且不少于 40 BB 的最大整数金额。
16. Poker Blind、Bet、Stack、Pot、Buy-in 和 Cash Out 只使用整数筹码。
17. Top-up 只在 Hand 边界生效，最多补至 100 BB；赢到超过 100 BB 不强制降低。
18. V1 不提供 Auto Rebuy，也不允许保持 Seat 时 Partial Cash Out。
19. Stack 归零后保留 Seat 60 秒等待 Rebuy，超时自动结算 Session。
20. Active Poker Session 是 Lobby 的最高优先级 Reconnect 模块。
21. V1 提供 Public Table 与 Lobby 可见的 Password Table。
22. V1 不提供完全隐藏的 Unlisted / Invite-only Table。
23. 当前未入座用户可以创建桌子；每人同时最多拥有一张未关闭牌桌。
24. Create Table 在 Lobby Modal / Drawer / Sheet 中完成。
25. 创建字段为名称、公开性、密码、座位数、Blind Preset、Spectator 与 Chat。
26. 第一笔 Buy-in 成功后锁定经济与人数设置。
27. 完全空置 30 分钟的牌桌自动关闭。
28. V1 不做 Waitlist；满桌时仅在允许观战时进入 Spectator。
29. Lobby 提供实时桌况、Filter、Sort、Table ID Search 和 Deep Link。
30. Password Table 密码同时保护入座和观战。
31. Seat Reservation 为 30 秒，Buy-in 成功后才正式占座。
32. 加入运行中桌子时可以选择 Wait for BB 或 Post BB Now。
33. 尚未开始第一手的桌子使用正常 Button / Blind 分配，不额外收费。
34. 提供 Sit Out Next Hand。
35. Sit Out 返回时根据错过 Blind 状态选择 Wait for BB 或 Post BB Now。
36. Sit Out 期间保留 Seat 与 Stack，并阻止用户加入或观战其他桌。
37. Sit Out 或掉线连续 15 分钟且无未结算 Hand 时自动 Safe Leave。
38. 至少两名 Ready 且 Stack > 0 的玩家才开始 Hand。
39. Hand 之间保留 5 秒；入座、Top-up 和离桌在 Hand 边界生效。
40. 房主可以暂停或恢复接受新玩家。
41. 房主移除已入座玩家时，只能在 Hand 结束后 Safe Leave / Cash Out。
42. 房主可以移除 Spectator 和 Mute 聊天用户。
43. 房主不能查看底牌、修改牌序、Pot、Stack、结果或已锁定经济配置。
44. 房主离开后牌桌继续，不自动转移房主身份。
45. Spectator 开关在创建时设置，第一笔 Buy-in 后锁定。
46. V1 Spectator 使用实时观战，不增加延迟。
47. Spectator 只能看到公共信息和正式公开的 Showdown Cards。
48. Table Chat 只支持文字和系统消息。
49. Chat 默认开启，房主创建时可关闭；启用时玩家与获准 Spectator 均可发言。
50. 用户可本地屏蔽聊天用户，Mobile Chat 不得遮挡 Action Area。
51. Dealer Button 顺时针移动，Heads-up 使用标准 Blind 与行动顺序。
52. 每次玩家决策 30 秒，V1 无 Time Bank。
53. 超时可 Check 时 Auto Check，否则 Auto Fold。
54. 连续两次超时后，下一手自动 Sit Out。
55. 操作集为 Fold、Check、Call、Bet、Raise、All-in。
56. Bet 控件提供 Min、1/2 Pot、2/3 Pot、Pot、All-in、Slider 与整数输入。
57. 所有合法金额由服务端计算；比例金额向下取整但不得低于合法最小值。
58. Minimum Raise 使用上一笔完整 Raise 增量规则。
59. Short All-in 不重新开放已经行动玩家的 Raise 权利。
60. V1 不提供高级 Pre-action；全部 All-in 后自动发完 Board。
61. Main Pot 与 Side Pot 按投入层级和 Eligible Players 确定性生成。
62. 平局平均 Split Pot；Odd Chip 给 Button 左侧顺时针第一位有资格的并列赢家。
63. 所有仍在局的 All-in 手牌在下注结束后公开。
64. 普通 Showdown 公开确定赢家所需手牌，输家可以 Muck。
65. 已 Fold 手牌在实时牌桌隐藏，V1 不提供主动 Show Folded Cards。
66. 每个 Hand 锁定 Blind、Ante、规则、算法与配置版本。
67. 客户端掉线时 Action Timer 继续，当前 Hand 后自动 Sit Out。
68. Poker Service 故障时暂停 Timer，从 PostgreSQL 恢复后给予 30 秒 Reconnect Grace。
69. 同一已入座账号只允许一个控制连接，第二设备需要明确 Take Over。
70. 返回大厅、Leave Table 和 Browser Back 全部进入统一 Safe Leave。
71. 当前 Hand 中离桌采用 Leave After Hand；已经 Fold 也等待 Settlement。
72. Hand 之间可以立即 Cash Out；Cash Out 幂等且不允许 Partial Cash Out。
73. Session P/L = Final Cash Out − 全部 Buy-in / Top-up，并只在 Session 结束后进入正式排行榜。
74. Poker In Play = Table Stack + 当前 Hand 尚未结算的自身投入。
75. 每个 Hand 发牌前提交 Seed Hash，并一次性确定完整 52 张牌顺序。
76. Effective Client Seed 由参与玩家的 Seed Contribution 确定性组合，玩家只能修改下一手。
77. 完整 Server Seed 与牌序在 Hand 结束 24 小时后仅向实际参与者开放；参与者接受届时可能重建彼此 Folded Cards，Spectator 和公共记录不获得完整 Seed，V1 不做零知识发牌系统。
78. Poker History 使用 Session Detail + Hand Detail；V1 使用 Action Timeline、不做动画 Replay；PC 使用沉浸牌桌，手机竖屏完整可玩、横屏优化；Maintenance 阻止新桌和新 Hand，但不得遗弃已开始 Hand。

---

# 169. IA-09 — Rankings Center & Game History

本阶段冻结跨产品域 Rankings Center、RP Usage 排行榜、Game History 与迁移余额清零相关 UX。

本阶段不设计最终榜单视觉、奖杯图标、FGO 称号、排行榜动画或卡片美术。

# 170. Rankings Center 产品定位

唯一入口：

`/rankings`

一级局部导航：

```text
Assets & Games | RP Usage
```

Rankings Center 对未登录用户公开浏览聚合结果，对已登录用户增加 My Rank 与个人详细记录 Cross-link。

Rankings 不增加为 PC Global Header 一级入口。

# 171. RP 身份归类：API Key Usage Purpose

RP 请求通过 API Key 的 `Usage Purpose = RP` 判断。

用途：

- General；
- RP；
- Unclassified，仅用于迁移后的既有 Key。

新建 Key 必须选择 General 或 RP。

用途修改：

- 只影响修改后的请求；
- 不追溯重写旧请求；
- 每条请求保存 `key_purpose_snapshot`；
- 不改变权限、路由、计费或模型访问。

同一 Master 的多个 RP Key 聚合为一条排名。

# 172. 合格 RP 逻辑请求

仅统计实际模型推理 / 生成请求。

不统计：

- 模型列表；
- 余额查询；
- 健康检查；
- 认证接口；
- 管理接口。

调用次数使用逻辑请求口径：

- 客户端独立发送的请求分别计数；
- 平台内部对同一逻辑请求执行的渠道重试只计一次；
- 模型归属使用用户请求的 Chaldea Model ID；
- 不公开内部实际渠道。

# 173. Rankings Center 页面结构

```text
Rankings Center
│
├── Assets & Games
│   ├── Total Assets
│   ├── Game Profit
│   ├── Biggest Win
│   ├── Total Wagered
│   └── Poker Profit
│
└── RP Usage
    ├── Calls
    ├── Errors
    └── Credits Consumed
```

页面状态包含：

- 一级榜单域；
- 指标；
- Today / This Week / All Time；
- Model Filter；
- 历史日期 / 周选择器；
- My Rank；
- Last Updated。

筛选状态允许通过 URL 状态分享，但不拆成多个一级页面。

# 174. Assets & Games 排行榜

## 174.1 Total Assets

展示当前资产快照。

不提供历史某日 Total Assets 回放。

迁移既有用户从 Cutover 清零后的资产状态开始。

## 174.2 Game Profit

使用：

```text
Today | This Week | All Time
```

计算：

`Direct Play 已结算净变化 + Poker 已实现 Session P/L`

排除奖励、管理员发放、兑换和 Poker 内部资金移动。

## 174.3 Biggest Win

包含：

- Direct Play Round 正净收益；
- Poker Hand 正净收益。

Poker Hand 必须等父 Session Cash Out 后才进入正式榜单。

## 174.4 Total Wagered

包含 Direct Play 实际下注，以及 Poker 中实际投入 Pot 的 Blind、Ante、Call、Bet、Raise、All-in。

同一投入只统计一次，Poker 数据在 Session Cash Out 后提交。

## 174.5 Poker Profit

以 Session Realized P/L 排名，并按 Cash Out 时间归入对应日榜或周榜。

# 175. RP Calls 排行榜

主排序：

`成功完成的合格 RP 逻辑请求数`

每行展示：

- Master；
- 成功调用数；
- 消耗额度；
- Error Rate；
- Top Models。

# 176. RP Errors 排行榜

主排序：

`Error Count`

每行展示：

- Error Count；
- Error Rate；
- Total Attempts；
- Top Error Models。

不按 Error Rate 排序。

计入已经归属有效用户并进入模型调用流程后的 Timeout、429、上游错误、平台调用错误和流中断。

不计入无效 Key 探测、上游前主动取消和内部多次渠道重试次数。

公共榜单不显示原始错误文本。

# 177. RP Credits Consumed 排行榜

主排序：

`最终实际结算 API Credit`

每行展示：

- 消耗额度；
- 成功调用数；
- Top Consumed Models。

失败请求只有实际产生扣费时才计入消耗。

额度最多显示 6 位小数，移除尾随零。

V1 不按排名自动发奖。

# 178. 模型展示与 Model Filter

三个 RP 榜单都提供 Model Filter。

PC 排名行摘要显示 Top 3 Models + Other；展开后显示完整模型分布和真实 Chaldea Model ID。

手机默认只显示 Top 1 Model，其余在展开层查看。

不公开 Provider、渠道、凭证或路由信息。

退役模型继续保留历史名称与 ID。

# 179. 周期、更新与并列名次

周期：

- Today；
- This Week；
- All Time。

规则：

- 时区 Asia/Shanghai；
- 周一 00:00 为一周起点；
- RP 从 Feature Activation Time 开始；
- 不回溯旧日志；
- 历史日榜与周榜永久保留；
- 指标为 0 不进入榜单；
- 同分共享排名，使用 1、2、2、4；
- 近实时聚合目标 5 分钟内；
- 显示 Last Updated。

# 180. 公开性、隐私与 My Rank

公开：

- Master 昵称和头像；
- 名次；
- 聚合指标；
- 模型分布；
- 周期和更新时间。

不公开：

- Key 信息；
- Prompt / Response；
- Request ID；
- 单次请求时间；
- 原始错误；
- IP / User-Agent；
- 渠道 / Provider。

Master 不链接至公开个人主页。

登录用户看到固定 My Rank。

用户只可从自己的 My Rank 进入带 RP Filter 的个人 API Usage，不能查看其他用户请求详情。

Dashboard 不新增大型 RP 排行卡片；API Usage 与 Personal Hub 提供入口。

# 181. Rankings Center PC / Mobile

PC：

- 榜单域、指标、周期、Model Filter 清晰分层；
- 排名列表可以展开模型分布；
- My Rank 在滚动时仍可快速定位；
- Last Updated 高可见。

Mobile：

- 使用紧凑榜单 Tab；
- 筛选进入 Bottom Sheet；
- 每行默认只显示核心指标和 Top 1 Model；
- 展开层查看完整分布；
- 不压缩 PC 宽表格。

# 182. Game History 产品职责

唯一入口：

`/history`

仅本人和管理员可以查看完整历史。

默认 All Records 展示 Direct Play Round 与 Poker Session，不平铺全部 Poker Hand。

RP API 请求继续由 `/api/usage` 承载。

# 183. Game History 筛选

支持：

- Record Type；
- Mode；
- Game；
- Time Range；
- Result；
- Status；
- ID Search。

Result：Win、Loss、Break-even、Cancelled、Refunded。

Status：Processing、Settled、Cancelled、Refunded、Recovering。

Game Filter 根据 Game Registry 动态生成，并保留退役游戏。

# 184. Game History 列表与详情层级

默认列表：

- Direct Play Round；
- Poker Session。

详情：

```text
Round Detail
Session Detail
Hand Detail
```

Poker Hand 主要从 Session Detail 的 Hand List 进入，也可以通过高级 Record Type Filter 单独查询。

从 Detail 返回时保留筛选与滚动位置。

# 185. History Cross-link 与公开记录

Round / Session / Hand Detail 可以 Cross-link：

- Wallet Transaction；
- Provably Fair；
- Game Entry；
- Poker Table / Session 信息。

V1 不提供：

- 完整个人历史公开分享；
- CSV / JSON 导出。

公共 Recent Wins / Featured Records 与私人 Game History 分离。

Rankings 使用当前 Master Profile；Recent Wins / Featured Records 与其他历史业务事件保存事件发生时的 Master Nickname / Avatar Snapshot。后续改名不会重写历史事件身份，但真实归属始终使用稳定 `newapi_user_id`。

# 186. Game History PC / Mobile

PC：

- 使用可筛选记录列表；
- Detail 可以使用页面或 Drawer，深层 Hand 仍有明确 Parent Back；
- 支持稳定 ID Search。

Mobile：

- 使用记录 Card；
- Filter Bottom Sheet；
- Detail 使用全屏层；
- 不压缩 PC 宽表格。

# 187. Chaldea Operations Rankings

后台同时管理 Game/Economy 与 RP Rankings。

至少展示：

- Enabled；
- Activation Time；
- Last Aggregate；
- Aggregation Lag；
- Data Completeness；
- Error Category；
- Reaggregation Jobs；
- Exclusion / Repair Audit。

管理员不能直接编辑用户分数。

错误数据通过有审计记录的排除、修复和重聚合处理。

准确统计至少需要：

- Stable User ID；
- Logical Request ID；
- Key ID；
- Key Purpose Snapshot；
- Request Model ID；
- Final Status；
- Error Category；
- Actual Credit Consumed；
- Timestamp。

不为 RP 排行新增 Prompt 或完整 Response 采集。

# 188. 迁移余额清零 UX

迁移用户首次进入：

```text
Authentication
→ Account Status Gate
→ Master Initialization（如需要）
→ Migration Notice（如需要）
→ Permission / Resource Availability Check
→ 合法 Return-to-Intent
→ 或 Dashboard / Safe Parent
→ Deferred Post-login Popup（安全页面）
```

Migration Notice 是独立迁移 Interstitial，不作为普通 Announcement。

至少表达：

- 旧 API 额度已按 Chaldea 开服公平规则重置为 0；
- 迁移批次已发放 1,000 API Credit 初始赠金；
- 账号、Discord、密码和 API Key 没有删除；
- 历史 API Usage 继续保留；
- 现有 Key 初始为 Unclassified；
- 该笔迁移初始赠金不表示重新注册，也不得重复触发新用户注册回调；
- 可以进入 Rewards、Wallet、API Keys 和 API Usage。

交互规则：

- 不提供右上角直接关闭；
- 使用 `我已了解，继续`；
- 服务端保存 Migration Notice Version、确认时间与用户；
- 每个迁移版本只确认一次；
- 未确认时下次登录继续展示；
- 不提供恢复旧额度按钮；
- 不重复发放迁移初始赠金；
- 确认后保留合法 Return-to-Intent，不无条件送往 Dashboard。


# 189. IA-09 核心 UX Flow

## 189.1 浏览 RP Calls

```text
Rankings
→ RP Usage
→ Calls
→ Select Period / Model
→ View Master Aggregates
→ Logged-in User sees My Rank
→ Open Own RP-filtered API Usage
```

## 189.2 浏览 RP Errors

```text
Rankings
→ RP Usage
→ Errors
→ View Error Count / Rate / Models
→ No raw error detail for other users
```

## 189.3 修改 Key Purpose

```text
API Keys
→ Select Key
→ Change Unclassified / General / RP
→ Confirm
→ Future Requests use new Purpose Snapshot
→ Historical Requests unchanged
```

## 189.4 查看 Poker 历史

```text
Game History
→ Poker Session
→ Session Detail
→ Hand List
→ Hand Detail
→ Wallet / Fairness Cross-link
→ Back preserves filters and scroll
```

# 190. IA-09 最终冻结结论

以下内容已按用户确认的 1A–84A 冻结：

1. RP 请求通过 API Key Usage Purpose = RP 判断。
2. API Key 用途至少为 General、RP、Unclassified。
3. 新建 Key 必须选择用途，一个 Key 同时只有一个用途。
4. Key 用途只影响统计，不影响权限、路由或计费。
5. Unclassified 不进入 RP 排行。
6. Purpose 修改不追溯历史。
7. 同一 Master 多个 RP Key 聚合排名。
8. V1 不建立独立模型排行榜。
9. 只统计模型推理 / 生成请求。
10. 模型列表、余额、健康、认证和管理请求不计入。
11. Calls 只统计成功逻辑请求。
12. 客户端独立请求分别计数，内部重试只算一个逻辑请求。
13. 模型归属使用用户请求的 Chaldea Model ID。
14. 所有通过 RP Key 的合格模型 Endpoint 均可统计。
15. Errors 统计可归属用户的失败逻辑请求。
16. 无效 Key 探测不进入个人报错榜。
17. 上游前主动取消不计报错。
18. 进入调用流程后的 Timeout、429、上游错误和流中断计报错。
19. 内部多次重试最终失败只计一次。
20. Errors 按 Error Count 排序，同时显示 Rate 与 Attempts。
21. 不按 Error Rate 排序。
22. 公共 Error 榜不显示原始错误文本。
23. Credits 使用最终实际结算 API Credit。
24. 不使用预估、美元或 raw quota 排名。
25. 失败请求仅在实际扣费时计入 Credits。
26. 额度最多显示 6 位小数并移除尾随零。
27. V1 不根据 RP 排名自动发奖。
28. RP 榜支持 Today / This Week / All Time。
29. 使用 Asia/Shanghai。
30. 周一 00:00 为周起点。
31. 从 Feature Activation Time 开始。
32. 不回溯旧日志推测 RP。
33. 历史日榜和周榜永久保留。
34. 指标为 0 不进入榜单。
35. 同分共享名次 1、2、2、4。
36. 聚合目标 5 分钟内并显示 Last Updated。
37. 继续使用唯一 `/rankings`。
38. Rankings 升级为跨产品域 Rankings Center。
39. 不增加 PC Global Header 一级入口。
40. 一级局部导航为 Assets & Games / RP Usage。
41. RP 内为 Calls / Errors / Credits Consumed。
42. 三个 RP 榜均支持 Model Filter。
43. 每行摘要展示 Top Models 并可展开。
44. Calls 显示成功调用、消耗和报错率。
45. Errors 显示报错数、报错率、总请求和主要模型。
46. Credits 显示消耗、成功调用和主要模型。
47. 模型摘要显示 Display Name，展开显示 Model ID。
48. 手机默认显示 Top 1 Model。
49. 页面筛选状态可分享但不拆一级页面。
50. RP 榜允许未登录公开浏览聚合结果。
51. 公开 Master 昵称与头像，不建立公开主页。
52. 不公开 Key、Prompt、Response、Request、IP、UA 或渠道。
53. 已登录用户看到 My Rank。
54. 只能从自己的 My Rank 进入个人 API Usage。
55. Dashboard 不增加大型 RP 排行卡片。
56. 今日、周、历史净盈利合并为 Game Profit 周期视图。
57. Game Profit = Direct Play 净变化 + Poker 已实现 P/L。
58. 奖励、管理员发放、兑换和 Poker 内部移动排除。
59. Poker 公共排名数据在 Session Cash Out 后提交。
60. Biggest Win 包含 Direct Play Round 与 Poker Hand。
61. Poker Hand 等父 Session Cash Out 后才进入 Biggest Win。
62. Poker 单手按净收益而非完整 Pot。
63. Total Wagered 包括 Direct Play 与 Poker 实际投入。
64. Poker 投入只统计一次并在 Session 结束后提交。
65. Total Assets 是当前快照，不提供历史资产回放。
66. Poker Profit 按 Cash Out 时间归期。
67. Game History 仅本人和管理员可见。
68. 默认列表展示 Direct Play Round 与 Poker Session。
69. Poker Hand 从 Session Detail 进入，也可高级筛选。
70. History 支持 Record Type、Mode、Game、Time、Result、Status、ID。
71. 返回列表保留筛选和滚动位置。
72. V1 不公开分享个人完整历史。
73. V1 不提供 CSV / JSON 导出。
74. 手机使用 Card 与 Filter Bottom Sheet。
75. 公共 Recent Wins 与私人 History 分离。
76. RP 请求记录不进入 Game History。
77. Operations Rankings 同时管理 Game 与 RP。
78. 后台显示启用、Activation、刷新和完整性。
79. 管理员不能直接编辑排名分数。
80. 错误数据通过审计化排除、修复和重聚合。
81. Error Category 可在后台和个人 Usage 查看，不在公共榜展示。
82. 统计需要用户、逻辑请求、Key、Purpose Snapshot、Model、状态、错误、消耗和时间。
83. 不为 RP 排行新增 Prompt / Response 采集。
84. API Keys 增加 Usage Purpose，API Usage 增加 Purpose Filter 和 RP Rankings Cross-link。

另行冻结迁移 UX：既有用户余额在 Cutover 统一清零，随后幂等发放 1,000 API Credit 初始赠金；账号与 API Key 保留，首次进入显示一次性告知。

# 191. IA-10 — Announcements & Events Information Architecture

本节按照用户确认的 1A–68A，冻结公告列表、详情、置顶、Entry Popup、Post-login Popup、Acknowledgements、阅读状态、内容安全、运营后台和响应式行为。

本阶段仍不冻结具体色板、FGO 装饰、弹窗材质、插画、转场与动画，这些内容继续留到 Art Direction v0.4。

# 192. Announcements & Events 产品定位

Announcements & Events 是 Chaldea 的统一内容发布与投放系统。

核心页面保持：

```text
/announcements
/announcements/:id
```

公告类型、Placement、阅读状态与展示版本都是同一系统内的能力，不创建多套重复页面。

Public 与 Auth 用户共用同一 Announcement List / Detail，具体可见性由公告 Visibility 决定。

# 193. 公告类型

V1 支持：

```text
System
New Models
Game Events
Maintenance
Important
Acknowledgements
```

这些类型用于列表筛选与内容语义，不分别建立一级路由。

`Acknowledgements` 继续使用 `/announcements/:id`，不建立 `/sponsors` 或 `/acknowledgements` 一级页面。

# 194. 公告生命周期与时间

生命周期：

```text
Draft
Scheduled
Published
Expired
Archived
```

## 194.1 Draft

- 仅运营后台可见；
- 支持 PC、Mobile 与 Entry Popup Preview；
- 不进入任何用户侧 Placement。

## 194.2 Scheduled

- 已配置未来发布时间；
- 到达服务端时间后自动发布；
- 发布前仍可修改或取消计划。

## 194.3 Published

- 用户根据 Visibility 可以访问；
- 按当前 Placement 进入列表、弹窗、Banner 或 Dashboard。

## 194.4 Expired

- 超过 `visible_until`；
- 不再进入 Entry Popup、Home Banner 或 Latest；
- 可以在 Archive 中查看。

## 194.5 Archived

- 管理员主动归档；
- 保留历史版本与审计；
- 默认不进入活跃列表。

运营时间字段：

```text
publish_at
visible_from
visible_until（可为空）
```

统一使用 `Asia/Shanghai`。长期公告允许不设置结束时间。

# 195. Announcement List

`/announcements` 包含：

```text
Pinned Announcements
Latest Announcements
Type Filter
Search
Date Filter
Archive
```

排序规则：

1. 当前有效置顶公告按照管理员手动顺序；
2. 其他当前公告按照发布时间倒序。

公告列表允许多条置顶公告同时存在。

过期或归档内容不进入 Latest，但可以通过 Archive 查看。

# 196. Announcement Detail

`/announcements/:id` 至少展示：

- 标题；
- 公告类型；
- 完整正文；
- 发布时间；
- 最近更新时间；
- 当前有效状态；
- 必要的活动开始 / 结束时间；
- 结构化 Acknowledgements 内容，如适用。

已登录用户打开 Detail 时，将当前公告展示版本标记为已读。

Announcement Detail 支持公开 Deep Link。关闭入口弹窗不影响详情继续存在。

# 197. Placement / Delivery Surface

以下 Placement 独立配置：

```text
Pinned in Announcement List
Entry Popup
Post-login Popup
Public Home Banner
Dashboard Summary
```

规则：

- Pinned 不自动等于 Popup；
- 同一公告可以使用多个 Placement；
- 多条置顶公告可以共存；
- V1 同一时点最多一条 Entry Popup；
- Placement 不产生新的一级页面；
- Home Banner 与 Entry Popup 可以分别启停。

# 198. Entry Popup 触发范围

Entry Popup 面向未登录入口：

```text
未登录首次进入 /
或
未登录首次进入 /login
```

普通 Entry Popup 不强制打断：

```text
/models/:model
/rankings
/announcements/:id
以及其他具有明确访问意图的公开 Deep Link
```

内容加载失败时：

- 不阻止 Public Home；
- 不阻止 Login；
- 不阻止 Discord Registration；
- 不将入口页面置于永久 Loading。

# 199. Entry Popup 的非阻断行为

Entry Popup 必须：

- 提供始终可见的关闭按钮；
- PC 支持 `Esc`；
- 手机支持明确关闭操作；
- 不设置强制倒计时；
- 不要求滚动到底；
- 不要求勾选同意；
- 关闭后继续访问平台；
- 提供“查看完整公告”入口。

普通致谢公告不是法律协议，也不是登录许可条件。

# 200. 展示版本、频率与 Re-notify

入口弹窗使用：

```text
announcement_id + notification_revision
```

作为展示版本。

默认规则：

- 每个展示版本在每个浏览器配置中最多主动弹出一次；
- 刷新不重复弹出同一版本；
- 同一版本在同一浏览会话中最多弹出一次；
- 普通内容修正不自动清除用户关闭状态；
- 只有新通知展示版本或明确 Re-notify 才再次主动弹出。

匿名用户关闭状态保存在浏览器本地。清除浏览器数据、无痕模式或更换浏览器后可能再次展示。

关闭弹窗只产生 `Popup Dismissed`，不等于 `Announcement Read`。

# 201. Post-login Popup

Post-login Popup 与 Entry Popup 分离。

规则：

- 排在 Master Initialization 与 Migration Notice 之后；
- 同一公告版本已经在本次入口流程出现后，登录成功不立即重复；
- 普通 Post-login Popup 不遮挡 Poker Table；
- 不遮挡活动中的 Direct Play Round；
- 不遮挡 Wallet Processing；
- 需要展示时延迟到 Dashboard 或下一个安全普通页面；
- 紧急维护 / 安全提醒使用独立 Critical Notice 机制。

Acknowledgements 默认不启用 Post-login Popup。

# 202. Acknowledgements 默认配置

建立一条长期维护的规范致谢公告：

```text
Type                  = Acknowledgements
Visibility            = Public
Pinned                 = Yes
Entry Popup            = Yes
Post-login Popup       = No
Public Home Banner     = No
Dashboard Summary      = Optional / 独立配置
Visible Until          = None
```

名单通过同一公告的版本更新维护，不重复创建多条同名致谢公告。

入口弹窗可以显示标题、感谢说明、名单、更新时间和完整详情入口。

# 203. Sponsor / Contributor List

每项支持：

```text
display_name           必填
avatar_or_logo         可选
external_link          可选
acknowledgement_note   可选
group                  可选
manual_order           必填排序语义
anonymous              可选
```

支持：

- 公开昵称；
- Discord 昵称；
- 项目名；
- Anonymous / 匿名赞助者；
- 人工分组，例如特别鸣谢、赞助支持、贡献者；
- 管理员手动组内排序。

V1 默认不公开具体赞助金额，也不按照金额自动生成 Gold / Silver / Bronze 等级。

# 204. Sponsor 隐私与同意

公开真实姓名、头像、Logo 或外部链接前，需要取得赞助者同意。

不得公开：

- 支付账号；
- 交易流水；
- Discord User ID；
- 邮箱；
- 付款截图；
- 未获同意的真实身份；
- 私人联系方式。

外部链接使用安全打开方式；头像与 Logo 使用受控媒体资源。

# 205. Read / Unread 与 Popup Dismissal

## 205.1 未登录用户

- 不建立跨设备已读状态；
- 浏览器本地保存 Entry Popup 展示版本关闭状态；
- 不显示个人未读数量。

## 205.2 已登录用户

- 已读状态保存到服务端；
- 多设备同步；
- 打开 Detail 时标记已读；
- 新通知展示版本可以重新进入未读。

Pinned 与 Read 状态互相独立。

# 206. Dashboard、Public Home 与入口整合

## 206.1 Dashboard Summary

Dashboard 显示 Important / Pinned / Latest 摘要，并允许 Acknowledgements 进入 Pinned 摘要。

致谢摘要不强制占据大型模块，完整内容进入 Announcement Detail。

## 206.2 Public Home Banner

Home Banner 与 Entry Popup 独立配置。

Acknowledgements 默认：

```text
Entry Popup = Yes
Home Banner = No
```

管理员后续可以发布其他适合 Banner 的公告，不影响致谢弹窗。

# 207. Markdown / Rich Text 安全

公告正文采用受控 Markdown / Rich Text。

必须：

- 统一安全清洗；
- 禁止任意 `<script>`；
- 禁止自定义 JavaScript；
- 禁止危险事件属性；
- 禁止任意 iframe；
- 使用允许列表处理 HTML；
- 对外部链接使用安全属性；
- 受控处理图片、头像和 Logo；
- 不直接信任粘贴的原始 HTML。

保存与渲染阶段都必须遵守同一安全策略。

# 208. PC 与 Mobile

## 208.1 PC Entry Popup

- 居中 Modal；
- 长内容区域可以滚动；
- 关闭按钮持续可见；
- 支持 `Esc`；
- 可以进入完整 Detail；
- 不阻止登录控件最终可用。

## 208.2 Mobile Entry Popup

- 使用接近全屏或全屏 Sheet；
- 顶部固定标题与关闭；
- 中间滚动名单；
- 提供完整详情入口；
- 不与系统 Back 产生无法退出的冲突。

## 208.3 Announcement List / Detail

- PC 可使用列表 / 卡片与筛选区；
- 手机使用单列内容与紧凑筛选；
- 长名单在 Detail 中保持可读；
- 最终色板、FGO 装饰与转场留到 v0.4。

# 209. Chaldea Operations Announcements

后台结构：

```text
Announcements & Events
│
├── All Content
├── Drafts
├── Scheduled
├── Published
├── Expired / Archived
├── Create Announcement
├── Pinned Order
├── Delivery Placements
├── Acknowledgements Editor
└── Audit Log
```

发布流程：

```text
Create Draft
→ Edit
→ Select Type
→ Set Visibility
→ Set Publish / Visible Time
→ Set Placements
→ Preview PC / Mobile / Entry Popup
→ Publish or Schedule
```

修改已发布公告时必须选择：

```text
Update Content Only
```

或：

```text
Publish New Notification Revision / Re-notify
```

已发布公告不进行无审计硬删除，使用 Expire、Withdraw 或 Archive。

审计至少记录：

- 操作者；
- 内容版本；
- 修改前后；
- Visibility；
- Placement；
- Pinned Order；
- Publish / Withdraw Time；
- Re-notify。

# 210. IA-10 核心 UX Flow

## 210.1 未登录入口查看致谢弹窗

```text
Open /
→ Check Active Entry Popup
→ Show Acknowledgements Revision
→ Dismiss or Open Full Detail
→ Continue Public Home / Login
```

## 210.2 刷新入口页

```text
Refresh /
→ Read Browser Dismissal State
→ Same Revision Already Dismissed
→ Do Not Reopen
```

## 210.3 致谢名单更新但不重新提醒

```text
Operations
→ Edit Published Acknowledgements
→ Update Content Only
→ Detail Content Updated
→ Existing Popup Dismissal State Preserved
```

## 210.4 致谢名单更新并重新提醒

```text
Operations
→ Edit Acknowledgements
→ Publish New Notification Revision / Re-notify
→ New notification_revision
→ Entry Popup Can Show Again
→ Logged-in Read State Can Reset for New Revision
```

## 210.5 登录后安全展示普通公告

```text
Login Success
→ Return-to-Intent = Poker / Active Round / Wallet Processing
→ Do Not Block Critical Flow
→ Defer Post-login Popup
→ Show on Dashboard or Safe Normal Page
```

## 210.6 查看公告列表

```text
Announcements
→ Pinned / Latest
→ Filter / Search / Date
→ Open Detail
→ Logged-in User Marks Current Revision Read
```

# 211. IA-10 最终冻结结论

以下内容已按用户确认的 1A–68A 冻结：

1. `/` 保持 Public Home，`/login` 保持独立 Login。
2. 未登录用户进入 `/` 或 `/login` 时检查 Entry Popup。
3. 普通致谢弹窗不强制打断公开 Deep Link。
4. Entry Popup 与 Post-login Popup 分离。
5. Entry Popup 加载失败不阻止入口使用。
6. Pinned、Entry Popup、Post-login Popup、Home Banner、Dashboard Summary 独立配置。
7. Pinned 不自动获得 Popup 资格。
8. 公告列表允许多条置顶。
9. 同一时点最多一条 Entry Popup。
10. 同一公告展示版本在同一会话最多弹出一次。
11. 新增 Acknowledgements 类型。
12. Acknowledgements 使用标准 Detail，不新增 Sponsor 一级页面。
13. 致谢公告对 Public / Auth 公开。
14. 致谢公告长期置顶，不设置结束时间。
15. 致谢公告默认启用 Entry Popup。
16. 致谢公告默认不启用 Post-login Popup。
17. 致谢公告默认不进入 Home Banner。
18. 致谢公告可以进入 Dashboard Pinned Summary。
19. Entry Popup 按每个展示版本、每个浏览器一次处理。
20. 刷新不重复弹出同一版本。
21. 只有 Re-notify / 新展示版本才再次弹出。
22. 普通文字修正不重置关闭状态。
23. 致谢弹窗允许立即关闭。
24. 不要求滚动到底或勾选同意。
25. 提供完整致谢名单入口。
26. Popup Dismissed 与 Announcement Read 分离。
27. 支持结构化 Sponsor / Contributor List。
28. 每项 Display Name 必填。
29. Avatar / Logo 可选。
30. External Link 可选。
31. Acknowledgement Note 可选。
32. 支持 Anonymous。
33. 支持人工分组。
34. 支持人工组内排序。
35. V1 不公开具体赞助金额。
36. V1 不自动生成赞助等级。
37. 公开身份、头像或链接前取得同意。
38. 不公开支付、交易、Discord ID、邮箱或付款截图。
39. Announcement List 分为 Pinned 与 Latest。
40. Pinned 手动排序，普通内容按发布时间倒序。
41. 支持类型、搜索和日期筛选。
42. Expired 进入 Archive，不再进入 Popup / Latest。
43. 致谢名单使用单一规范公告持续维护。
44. 生命周期为 Draft / Scheduled / Published / Expired / Archived。
45. 公告运营时间使用 Asia/Shanghai。
46. 公告可以没有结束时间。
47. 匿名用户不维护跨设备已读状态。
48. 登录用户已读状态服务端跨设备同步。
49. 打开 Detail 标记当前版本已读。
50. 新通知展示版本可以重新标记未读。
51. Entry 已展示的同版本不在本次登录后重复弹出。
52. 普通 Post-login Popup 不遮挡 Poker、活动 Round 或 Wallet Processing。
53. Post-login Popup 延迟到安全页面。
54. Acknowledgements 主要使用 Entry Popup。
55. Markdown / Rich Text 统一安全处理。
56. 不允许任意 Script、事件属性或 iframe。
57. 外部链接使用安全打开方式。
58. Avatar / Logo 使用受控媒体资源。
59. 后台支持 Draft、Preview、Schedule、Publish、Update、Archive。
60. 后台提供 PC、Mobile、Entry Popup Preview。
61. 已发布修改区分只更新与 Re-notify。
62. 已发布公告不进行无审计硬删除。
63. 保存公告版本和完整操作审计。
64. Placement、Pinned Order、Visibility、Re-notify 全部审计。
65. PC 使用可滚动 Modal，关闭持续可见。
66. 手机使用接近全屏或全屏 Sheet。
67. 长名单弹窗可滚动并始终能进入 Detail。
68. 具体视觉与动画留到 Art Direction v0.4。

# 212. IA-11 — Master Profile、Account & Security 与 Onboarding Information Architecture

本节冻结 Chaldea Platform 的 Master Identity、个人功能中心、账号安全、首次初始化、迁移告知、Return-to-Intent 顺序与相关异常 UX。

本节不深入数据库或 API 实现，也不冻结 Master 卡片、头像框、FGO 装饰和初始化动画。具体视觉继续留到 Art Direction v0.4。

# 213. 产品边界与路由

继续使用：

```text
/me
Personal Hub

/master-profile
Master Profile

/account/security
Account & Security

/onboarding/master
Master Initialization
```

Migration Notice 为条件式认证后 Interstitial，不新增固定一级路由。

V1 不建立：

- 其他用户公开 Profile；
- 独立 Profile Detail 子页面；
- 独立设备管理页面；
- 独立邮箱找回密码页面；
- 普通用户账号删除页面。

Master Identity、Account Identity 与 Authentication 三层分离。Master Nickname 不作为 Password Login Identifier。

# 214. Personal Hub / My

`/me` 为 Mobile-first Personal Hub，不替代 Dashboard。

```text
Personal Hub
│
├── Master Summary
│   ├── Avatar
│   ├── Master Nickname
│   └── Edit Profile
├── Master & Account
│   ├── Master Profile
│   └── Account & Security
├── API
│   ├── API Keys
│   ├── API Usage
│   └── RP Rankings
├── Activity
│   ├── Game History
│   └── Announcements
├── Administration
│   └── Chaldea Operations（仅管理员）
└── Logout
```

入口可以显示 Unclassified Key、Unread Announcement、Password Not Set 等轻量状态。V1 不允许用户自定义或重新排序模块。

# 215. Master Profile

页面结构：

```text
Master Profile
│
├── Public Identity Preview
├── Master Nickname
├── Display Avatar
├── Public Visibility Explanation
├── Save / Cancel
└── Update Status
```

V1 Profile 字段仅包括 Master Nickname 与 Display Avatar。

Public Identity Preview 说明该身份可能出现在 Rankings、Recent Wins、Poker Table 与 Table Chat，但不产生公开个人主页。

Profile 使用明确 Save / Cancel。离开有未保存修改的页面时提示是否放弃；不采用逐字段自动保存。

# 216. Master 昵称

规则：

- 全平台唯一；
- 1–24 个可见 Unicode Grapheme；
- 允许文字、数字、普通空格、`_`、`-`、`·`；
- V1 不允许 Emoji；
- 禁止换行、控制字符、零宽字符、双向控制字符和注入内容；
- 使用 NFKC、首尾空白移除、连续空格合并与 Unicode Case Fold 进行唯一性比较；
- 服务端执行最终唯一性、敏感词、骚扰内容、保留名称和身份冒充检查。

`Alice`、`alice` 与全角 `Ａｌｉｃｅ` 视为冲突。

冲突时可以提供带短后缀的建议，但不自动保存。

初始化后，用户主动改名进入 7 天冷却。管理员可以执行审计化强制改名或 `Rename Required`。停用账号的名称继续保留，除非通过专门审计流程释放。

# 217. Display Avatar

V1 Avatar Source：

```text
System Default Avatar
Discord Avatar Snapshot
```

Discord Avatar 只在用户主动 `Sync from Discord`、预览并保存后更新，不自动跟随 Discord 变化。

Discord 资源不可用时回退系统默认头像。V1 使用静态头像，不播放 Discord 动态头像；Avatar 可随时修改。

V1 不开放自定义上传，也不显示不可用上传入口。相关文件处理与审核留到以后独立确认。

# 218. 身份快照

身份时间口径：

- Rankings 使用当前 Master Profile；
- Recent Wins / Featured Records 与历史业务事件使用事件发生时快照；
- Table Chat 使用发送时昵称与头像快照；
- Poker Session 在第一次 Buy-in / 入座时冻结 Master Snapshot；
- 当前 Poker Session 中途改名不改变牌桌身份；
- 完成 Cash Out 后的下一次入座使用新 Profile。

真实归属始终使用 `newapi_user_id`。

# 219. Account & Security

```text
Account & Security
│
├── Account Identity
├── Discord Connection
├── Password
├── Current Session
└── Account Status
```

Account Identity 显示只读 Password Login Identifier 与短 Account ID。V1 不允许从 Chaldea 修改 NewAPI 登录用户名。

这些私有信息不进入公开 Profile、Rankings、Poker 或 Recent Wins。

# 220. Discord Connection

已绑定时显示 Connected 状态与 Discord Display Name；普通页面不默认展示数字 Discord User ID。

V1 不提供自助 Unlink 或 Rebind。换绑、Legacy Account 补绑定和冲突修复使用管理员辅助流程，必须验证原账号所有权、新 Discord 唯一性并完整审计。

失去 Role 或退出服务器不影响既有账号。

Legacy Account 无 Discord 时可继续密码登录，不强制重新注册；补绑定不触发新用户初始赠金。Discord 已绑定其他账号时禁止覆盖、自动合并或迁移资产。

# 221. Password

密码完全由 NewAPI 保存和验证。Account 页面只显示 Password Set / Not Set。

Set Password：

```text
Fresh Authentication
→ New Password
→ Confirm Password
→ NewAPI Save
```

Change Password：

```text
Current Password
→ New Password
→ Confirm New Password
→ NewAPI Validate & Update
```

Forgot Password（已绑定 Discord）：

```text
Fresh Discord OAuth Re-authentication
→ Verify Discord User ID equals existing binding
→ Set New Password
→ NewAPI Update
```

设置或重置要求最近 10 分钟内 Fresh Authentication。无 Discord 的 Legacy Account 采用管理员辅助恢复。

Password Policy 以 NewAPI 真实规则为准；登录失败使用通用错误，不泄露账号是否存在。

V1 不加入 Email / Phone Recovery、TOTP、Passkey、Backup Codes 或设备管理中心。只保证 Logout Current Session；其他会话撤销以 NewAPI 实际能力为准。

# 222. Account Status 与删除边界

V1 不提供普通用户自助硬删除账号。

账号停用由 NewAPI Account Status 控制。受限用户不进入普通 Dashboard，只看到必要状态、联系管理员和 Logout，不显示内部管理备注，也不能发起新的 API、Wallet 或游戏操作。

NewAPI 账号服务暂时不可用时，Profile 可以继续读取，但 Password 操作禁用并显示真实服务状态。

未来增加账号删除请求时，必须另行处理资产、API Key、未完成 Poker、账本、匿名化、审计与恢复窗口。

# 223. Master Initialization

第一次认证后，服务端幂等创建 `INCOMPLETE` Provisional Master Profile。

该 Profile 已绑定 `newapi_user_id`，但尚不作为正式公开身份出现。

初始化使用紧凑单页：

```text
Welcome
→ Master Nickname
→ Avatar Selection
→ Public Visibility Notice
→ Live Preview
→ Complete Initialization
```

候选昵称优先：Discord Display Name → NewAPI Username → `Master-<Short ID>`。候选值仍须通过完整校验。

有 Discord Avatar 时默认预选快照，否则使用系统头像。用户必须主动确认昵称与头像，不能跳过。未完成时不能进入普通 Auth 页面或游戏。

初始化不强制设置密码。完成后可以显示 Password Not Set 的非阻断提示。

保存必须幂等；失败保持 INCOMPLETE，下次登录恢复，不重复创建 Profile 或初始赠金。

# 224. 新注册用户流程

```text
Public Home / Login
→ Entry Popup（如需要）
→ Discord Registration
→ Server / Role Validation
→ NewAPI Account Created
→ Initial Grant Created: +1,000 API Credit
→ Authentication
→ Account Status Gate
→ INCOMPLETE Provisional Master Profile
→ Master Initialization
→ Completion Summary / Real Reward Status
→ Return-to-Intent or Dashboard
→ Deferred Post-login Popup（安全页面）
```

Completion Summary 读取真实奖励状态，不假定前端提交即到账。

# 225. Migration Notice

迁移用户在 Master Initialization 后进入版本化 Migration Notice。

它是独立迁移 Interstitial，不是普通 Announcement。

规则：

- 不提供右上角直接关闭；
- 使用 `我已了解，继续`；
- 服务端保存用户、Migration Version 与确认时间；
- 每个迁移版本确认一次；
- 未确认时下次登录继续显示。

至少说明旧额度清零、迁移批次已发放 1,000 API Credit 初始赠金、账号 / Discord / 密码 / API Key 保留、历史 Usage 保留、Key 初始 Unclassified；并说明该赠金不表示重新注册，提供 Wallet、Rewards、API Keys、API Usage 入口。

# 226. 登录后顺序与 Return-to-Intent

```text
Authentication
→ Account Status Gate
→ Master Initialization（如需要）
→ Migration Notice（如需要）
→ Permission / Resource Availability Check
→ 合法 Return-to-Intent
→ 或 Dashboard / Safe Parent
→ Deferred Post-login Popup（安全页面）
```

Return-to-Intent 只接受允许访问的站内路径，禁止外部 URL 与危险 Scheme，使用一次后清除；无效或过期时回退 Dashboard。

只恢复页面，不自动重放 Profile 保存、密码操作、Wallet Exchange、API Key 修改、游戏下注或 Poker 操作。

# 227. Logout 与服务端进行中操作

Active Poker Session 存在时，从普通页面 Logout 显示：

```text
You still have an active Poker session.
Logging out will not Cash Out.
Action timers and automatic actions may continue.
```

操作：

```text
Return to Poker Table
Logout Anyway
```

Logout 不自动 Safe Leave / Cash Out，也不取消 Wallet Exchange、Reward Claim、已接受 Game Round 或 API 服务端处理。

# 228. 错误、空状态与重新认证

- Account Disabled：进入受限页面，不进入 Dashboard；
- NewAPI Unavailable：Password 操作禁用，不伪造状态；
- Profile Save Failed：不改变既有公开身份，不产生半更新或重复 Profile；
- Session Expired：重新认证后返回原页面；可安全恢复非敏感草稿，密码字段不恢复；
- Discord Binding Conflict：禁止覆盖或合并，转管理员辅助；
- Discord Avatar Unavailable：回退系统默认头像。

# 229. PC、Mobile 与 Accessibility

PC：

- 使用 `My | Master Profile | Account & Security` Context Navigation；
- Profile 可以采用宽屏 Preview + Edit 结构；
- Account 页面保持只读身份、连接状态和敏感操作清晰分区。

Mobile：

- 从 `/me` 进入单列 Profile 与 Account；
- Save、头像选择、错误提示和敏感确认适合触屏；
- 不长期显示内部长 ID；
- 密码字段支持系统密码管理器。

Accessibility：

- 键盘可操作；
- 明确 Focus；
- 表单 Label；
- 错误使用文本，不只依赖颜色；
- Avatar Alt；
- Screen Reader 可理解状态变化；
- Reduced Motion。

具体 Master 卡片、头像框和初始化演出留到 v0.4。

# 230. IA-11 核心 UX Flow

## 230.1 编辑 Master Profile

```text
Personal Hub / Avatar Menu
→ Master Profile
→ Edit Nickname / Avatar
→ Preview
→ Save
→ Server Validation
→ Success or Field Error
```

## 230.2 同步 Discord Avatar

```text
Master Profile
→ Sync from Discord
→ Fresh Avatar Snapshot
→ Preview
→ Save
→ Future Public Surfaces use new avatar
→ Active Poker Session keeps old snapshot
```

## 230.3 设置或修改密码

```text
Account & Security
→ Set / Change Password
→ Fresh Authentication as required
→ NewAPI Validate / Save
→ Success
```

## 230.4 Discord 重置密码

```text
Forgot Password
→ Discord OAuth Re-authentication
→ Verify bound Discord User ID
→ Set New Password through controlled NewAPI capability
→ Login
```

## 230.5 迁移用户首次进入

```text
Login
→ Account Status Gate
→ Master Initialization（如需要）
→ Migration Notice
→ Acknowledge
→ Resource Availability Check
→ Return-to-Intent or Dashboard / Safe Parent
```

## 230.6 Active Poker 时 Logout

```text
Logout
→ Active Poker Warning
→ Return to Table
或
→ Logout Anyway without Cash Out
```

# 231. IA-11 最终冻结结论

以下内容已按用户确认的 1A–129A 冻结：

1. 继续使用 `/me`、`/master-profile`、`/account/security`、`/onboarding/master` 四个路径。
2. Master Identity、Account Identity、Authentication 三层分离。
3. Master 昵称不得作为密码登录标识。
4. 修改 Master Profile 不改变账号、Discord、API Key 或历史归属。
5. V1 不建立其他用户的公开 Profile 页面。
6. V1 不建立独立 Profile Detail 子页面。
7. PC 使用 `My | Master Profile | Account & Security` Context Navigation。
8. `/me` 保持 Mobile-first Personal Hub，不替代 Dashboard。
9. Personal Hub 顶部显示 Avatar、Master Nickname 和 Edit Profile。
10. Personal Hub 按 Master & Account、API、Activity 分组入口。
11. API 区包含 Keys、Usage 与 RP Rankings。
12. Activity 区包含 Game History 与 Announcements。
13. Personal Hub 允许显示 Unclassified Key、Unread Announcement、Password Not Set 等状态提示。
14. 管理员在 `/me` 中额外看到 Chaldea Operations 入口。
15. Logout 位于 Personal Hub 底部。
16. V1 不支持用户自定义或重新排序 Personal Hub。
17. V1 Profile 只包含 Master Nickname 与 Display Avatar。
18. V1 不加入 Bio、签名、生日、地区或社交链接。
19. Master Profile 页面提供 Public Identity Preview。
20. 页面明确告知昵称和头像会显示在 Rankings、Poker、Chat 与公开中奖记录。
21. V1 不提供隐藏昵称、匿名排行或 Private Profile 模式。
22. Profile 使用明确 Save / Cancel，不采用逐字段自动保存。
23. 离开存在未保存修改的页面时进行提醒。
24. Master 昵称全平台唯一。
25. 唯一性使用 NFKC、空白规范化与 Unicode Case Fold 后的结果。
26. 昵称长度为 1–24 个可见 Unicode Grapheme。
27. 允许文字、数字、普通空格、`_`、`-`、`·`。
28. V1 不允许昵称包含 Emoji。
29. 禁止控制字符、零宽字符、双向控制字符、换行和注入内容。
30. `Alice`、`alice` 和全角 `Ａｌｉｃｅ` 视为冲突。
31. 维护 Admin、Official、System、Chaldea 等保留名称列表。
32. 昵称需要敏感词与身份冒充检查。
33. 服务端负责最终昵称校验，前端校验只用于即时反馈。
34. 昵称冲突时提供带短后缀的建议，但不自动保存。
35. 首次初始化必须确认一个合法昵称。
36. 用户主动改名后使用 7 天冷却。
37. Avatar 修改不受昵称冷却影响。
38. 管理员可以强制要求违规用户改名，但必须通知并审计。
39. 管理员不得无审计静默修改昵称。
40. 停用账号的昵称继续保留，除非管理员通过专门审计流程释放。
41. V1 Avatar Source 只有 System Default 与 Discord Avatar。
42. V1 不开放自定义头像上传。
43. 不显示不可用的“上传头像”占位入口。
44. System Default Avatar 的具体美术留到 v0.4。
45. Discord Avatar 使用用户主动同步时的快照。
46. Discord 头像变化不会自动改变 Chaldea 头像。
47. Master Profile 提供 `Sync from Discord`。
48. Discord Avatar 不可用时回退系统默认头像。
49. V1 所有头像使用静态表现，不播放 Discord 动态头像。
50. Avatar 可以随时修改，无独立冷却。
51. Rankings 使用当前 Master Profile。
52. Recent Wins 与历史业务事件保存事件发生时的身份快照。
53. Table Chat 保存发送时昵称与头像快照。
54. Poker Session 入座时冻结 Master Snapshot。
55. Poker Session 中途改名不改变当前牌桌身份。
56. 完成 Cash Out 后下一次入座使用新 Profile。
57. 所有记录的真实归属始终使用稳定 `newapi_user_id`。
58. Account & Security 包含 Account Identity、Discord、Password、Current Session 与 Account Status。
59. 显示只读 Password Login Identifier。
60. 显示只读短 Account ID，但不作为公开信息。
61. 不允许在 Chaldea 修改 NewAPI 登录用户名。
62. Discord 已绑定时显示连接状态与 Discord Display Name。
63. Discord 数字 User ID 不在普通页面默认展示。
64. V1 不提供自助 Discord Unlink。
65. V1 不提供自助 Discord Rebind / Change Binding。
66. Discord 换绑与冲突修复只能由管理员辅助并完整审计。
67. 失去 Discord Role 或退出服务器不影响已存在账号。
68. Legacy Account 没有 Discord Binding 时不强制重新注册或绑定。
69. 无 Discord 的 Legacy Account 可以继续密码登录。
70. Legacy Account 需要绑定 Discord 时采用管理员辅助流程。
71. Discord 已绑定其他账号时禁止覆盖、自动合并或迁移资产。
72. 密码继续完全由 NewAPI 保存和验证。
73. Chaldea 不保存、记录或复制密码与密码哈希。
74. Account 页面只显示 `Password Set / Not Set`。
75. 没有密码的用户可以执行 Set Password。
76. 已有密码的用户修改时输入 Current Password、New Password 和 Confirm。
77. 已绑定 Discord 的用户可通过 Fresh Discord Re-authentication 重置忘记的密码。
78. Discord 重置密码时必须验证返回的 Discord User ID 与原绑定一致。
79. 设置或重置密码要求 10 分钟内的 Fresh Authentication。
80. 无 Discord 且忘记密码的 Legacy Account 使用管理员辅助恢复。
81. 密码规则以 NewAPI 的真实校验为准，Chaldea 不建立冲突规则。
82. 密码登录失败使用通用错误，不泄露具体账号是否存在。
83. V1 不增加 Email / Phone Recovery。
84. V1 不增加 TOTP、Passkey、Backup Code 或设备管理中心。
85. V1 不提供普通用户自助硬删除账号。
86. 账号限制和停用继续由 NewAPI Account Status 控制。
87. V1 只保证 Logout Current Session；其他会话撤销以真实 NewAPI 能力为准。
88. 首次登录幂等创建 `INCOMPLETE` Provisional Master Profile。
89. 未完成 Profile 不作为正式公开身份出现。
90. 初始化使用一个紧凑单页，而不是多步长 Wizard。
91. 候选昵称优先使用 Discord Display Name。
92. 没有可用 Discord 名称时使用 NewAPI Username，再使用 `Master-<Short ID>`。
93. 候选昵称仍须通过唯一性和敏感内容校验。
94. 有 Discord Avatar 时默认预选其快照，否则使用系统头像。
95. 用户必须主动确认昵称和头像。
96. Master Initialization 不允许跳过。
97. 初始化未完成时不能进入普通 Auth 页面或游戏。
98. 初始化不强制设置密码。
99. 初始化完成后对未设置密码用户显示非阻断式安全提示。
100. 初始化保存必须幂等，不重复创建 Profile 或初始赠金。
101. 刷新或中断后，下次登录恢复初始化。
102. 新用户完成时可以显示真实 1,000 API Credit 初始赠金到账状态。
103. 迁移用户同样先完成 Master Initialization。
104. Master Initialization 后展示一次性 Migration Notice。
105. Migration Notice 是独立迁移 Interstitial，不作为普通公告。
106. Migration Notice 不提供右上角直接关闭，使用 `我已了解，继续`。
107. 服务端保存 Migration Notice 版本、确认时间和用户。
108. 每个迁移版本只需确认一次。
109. 未确认时下次登录继续展示。
110. Notice 明确说明余额清零但账号、Discord、密码和 API Key 保留。
111. Notice 明确说明历史 Usage 保留、Key 初始 Unclassified，并已通过迁移批次发放 1,000 API Credit 初始赠金。
112. Notice 提供 Wallet、Rewards、API Keys 与 Usage 后续入口。
113. 有合法 Return-to-Intent 时，确认 Notice 后前往原目标。
114. 无 Return-to-Intent 时进入 Dashboard。
115. 普通 Post-login Popup 排在 Initialization 与 Migration Notice 之后。
116. Return-to-Intent 只允许站内安全路径，使用后清除。
117. Return-to-Intent 不自动重放任何 Profile、Password、Wallet 或游戏操作。
118. 存在 Active Poker Session 时 Logout 显示明确警告。
119. Logout 不自动执行 Poker Safe Leave 或 Cash Out。
120. Logout 不取消 Wallet、Reward、Game Round 或 API 的服务端处理。
121. 账号被停用时进入受限状态，不继续普通 Dashboard。
122. 受限页面只显示必要状态、联系管理员和 Logout，不显示内部管理备注。
123. NewAPI 不可用时，Password 操作禁用并显示真实服务状态。
124. Profile 保存失败时不产生半更新或重复 Profile。
125. Session 失效后重新认证并返回原页面；密码字段不恢复。
126. PC 使用 Context Navigation 和适合宽屏的预览 / 编辑结构。
127. 手机从 `/me` 进入单列 Profile 与 Account 页面。
128. 表单支持键盘、屏幕阅读器、明确 Label、Focus 和文字错误提示。
129. Master 卡片、头像框、初始化演出与 FGO 视觉留到 Art Direction v0.4。

# 232. IA-12 — Chaldea Operations Information Architecture

本节负责冻结 Chaldea Operations 的后台边界、信息架构、管理员角色、模块工作流、危险操作保护、审计、维护与响应式原则。

本节不负责：

- 重做 NewAPI 原管理员后台；
- 编写实际后台代码；
- 确定数据库表结构细节；
- 确定最终后台视觉、配色、图标或 FGO 装饰；
- 提供 SSH、Shell、SQL Console 或 Redis Console。

# 233. 双后台边界与 Operations Sitemap

平台继续采用：

```text
NewAPI Admin
+
Chaldea Operations
```

NewAPI Admin 继续负责：

- NewAPI 原生 Users；
- Channels；
- 模型路由与底层模型能力；
- 倍率与真实计费；
- NewAPI quota；
- 其他 NewAPI 原生管理功能。

Chaldea Operations 负责 Chaldea 业务域：

```text
Chaldea Operations
│
├── Command
│   └── Overview
│       ├── Platform Status
│       └── Needs Attention
│
├── Catalog & Community
│   ├── Models
│   ├── Users & Identity
│   ├── Games
│   ├── Poker
│   └── Announcements & Events
│
├── Economy & Data
│   ├── Economy
│   ├── Rewards
│   ├── Rankings
│   └── Records
│
└── Administration
    ├── Operations
    ├── Access Control
    ├── Audit
    └── Open NewAPI Admin ↗
```

`Command`、`Catalog & Community`、`Economy & Data`、`Administration` 仅作为 Sidebar 分组标题，不建立独立业务页面。

NewAPI Admin 与 Chaldea Operations 的权限体系互相独立。NewAPI Admin 身份不自动授予 Chaldea Operations 权限；Chaldea Super Admin 也不自动获得 NewAPI Admin 权限。

# 234. Operations 路由与稳定 Deep Link

当前冻结以下逻辑路由：

```text
/ops
/ops/overview

/ops/models
/ops/models/:model_id

/ops/users
/ops/users/:user_id

/ops/games
/ops/games/:game_slug

/ops/poker
/ops/poker/tables/:table_id
/ops/poker/sessions/:session_id
/ops/poker/hands/:hand_id

/ops/economy
/ops/economy/transfers/:transfer_id
/ops/economy/transactions/:transaction_id

/ops/rewards
/ops/rewards/claims/:claim_id

/ops/rankings

/ops/records
/ops/records/rounds/:round_id
/ops/records/sessions/:session_id
/ops/records/hands/:hand_id

/ops/announcements
/ops/announcements/:announcement_id

/ops/operations
/ops/access
/ops/audit
/ops/audit/:audit_id
```

后台对象详情使用稳定 URL，以支持：

- 管理员之间共享问题链接；
- Needs Attention 跳转；
- Audit 回查目标对象；
- 页面刷新后恢复当前工作位置；
- 从对象详情反查相关 Audit、Incident 和业务记录。

Drawer 可以用于快速预览，但复杂详情必须具有正式页面与稳定 Deep Link。

# 235. RBAC 与权限边界

V1 使用三种基础角色：

| 角色 | 权限定位 |
|---|---|
| Super Admin | 完整 Chaldea Operations 权限与关键操作能力 |
| Operator | 按模块 Scope 获得日常运营能力 |
| Auditor | 全后台只读与 Audit 查看 |

Operator Scope 至少包括：

- Models；
- Users & Identity；
- Games；
- Poker；
- Rewards；
- Rankings；
- Records；
- Announcements。

资产调整、Discord Rebind 最终执行、Access Control、全站 Maintenance、关键经济版本激活和 Poker Emergency Pause 仅允许 Super Admin。

所有权限必须由服务端校验。隐藏菜单、禁用按钮或前端路由守卫不能代替授权检查。

任何角色均不得查看：

- 用户密码或密码哈希；
- API Key Secret；
- 未授权 Prompt / Response；
- 当前未公开 Hole Cards；
- 当前未公开 Server Seed 或完整牌序。

# 236. Operations Shell、环境标识与列表规范

PC 使用持久 Sidebar。顶部区域至少包含：

```text
Environment
Global Search
Needs Attention Count
Return to User Site
Current Admin
```

环境标识必须持续、明显地区分：

- Production；
- Staging；
- Development。

后台列表采用明确分页，不以无限滚动作为唯一浏览方式。筛选、排序、分页写入 URL；从 Detail 返回时恢复筛选、排序、页码与滚动位置。

深层详情使用 Breadcrumb。复杂详情使用正式页面；Drawer 只承担快速预览。

# 237. Global Search

Global Search 可以按以下稳定对象定位：

- Master Nickname；
- Short Account ID；
- `newapi_user_id`；
- Discord User ID；
- API Key ID；
- Transfer ID；
- Transaction ID；
- Round ID；
- Poker Table ID；
- Poker Session ID；
- Poker Hand ID；
- Announcement ID；
- Config Version；
- Audit ID。

Global Search 不索引或返回：

- API Key Secret；
- 密码或密码哈希；
- Prompt / Response；
- 当前未公开牌面；
- 当前未公开 Server Seed。

# 238. Operations Overview 与 Needs Attention

Overview 的首要职责是回答：

> 当前是否存在需要管理员立即处理的异常？

页面结构：

```text
Operations Overview
│
├── Environment & Service Status
├── Needs Attention
├── Current Activity
├── Economy Snapshot
├── Jobs & Data Freshness
└── Recent Administrative Activity
```

Needs Attention 至少聚合：

- 长时间 Pending 的资产转换；
- 补偿失败；
- 奖励发放异常；
- 未完成或恢复失败的游戏 Round；
- Paused / Recovering Poker Table；
- 未完成 Cash Out；
- Rankings 聚合延迟；
- Model Sync 或元数据缺失；
- Entry Popup 排期冲突；
- Discord Binding Conflict；
- Account Support Case；
- 服务维护状态。

Attention Item 使用 `Critical / Warning / Info`，并链接到对应对象详情。

`Acknowledge` 只表示管理员已经查看，不表示问题已经解决。未解决的金融、资产、牌局或结算问题不得被永久隐藏或删除。

业务摘要至少可以显示在线用户、活动 Round、活动 Poker Table、Pending Transfer、当日资产发行与回收，并标记更新时间或数据新鲜度。

# 239. Models 管理

Models 是 Chaldea Model Catalog 管理模块，不取代 NewAPI 模型后台。

```text
Models
│
├── Synced Models
├── Pending Metadata
├── Published
├── Hidden / Unavailable / Retired
└── Model Detail
    ├── NewAPI Source Data
    ├── Chaldea Metadata
    ├── Publication
    ├── Pricing Presentation
    ├── Availability Mapping
    ├── User Preview
    └── Audit
```

NewAPI Source Data 只读展示：

- Model ID；
- 底层状态；
- 原始价格和倍率摘要；
- 渠道可用性摘要；
- 最近同步时间。

Chaldea Metadata 可以管理：

- 展示名称；
- Logo；
- 简介；
- 标签；
- 推荐用途；
- 上下文长度；
- FGO 风格副标题；
- 推荐状态；
- 排序；
- 是否公开。

模型发布前必须校验元数据完整性、真实模型存在性、价格展示与状态映射。Chaldea 展示字段不得改变真实计费结果。

Retired 模型可以从目录隐藏，但历史 API Usage 与 Rankings 中的历史模型名称和 Model ID 必须继续保留。

# 240. Users & Identity

```text
Users & Identity
│
├── User Search
├── Master Moderation
├── Account Support Cases
├── Binding Conflicts
└── Migration Acknowledgements
```

User Detail 使用：

```text
Overview | Identity | Assets | Rewards | Activity | Support | Audit
```

允许：

- 查看 Master Profile 与账号状态摘要；
- 查看 Discord Binding 状态；
- 标记 `Rename Required`；
- 执行有审计的 Forced Rename；
- 查看昵称冷却和 Profile Status；
- 创建、验证和处理 Support Case；
- 处理 Legacy Account 与 Discord Binding Conflict；
- 查看 Migration Notice Acknowledgement；
- Cross-link 到 Economy、API Usage、Game History 和 NewAPI Admin。

Support Case 状态：

```text
Open
→ Verifying
→ Approved / Rejected
→ Executed
→ Closed
```

Discord Rebind 必须验证原账号所有权、新 Discord 唯一性、操作原因和 Fresh Authentication。

Users & Identity 不得：

- 查看密码、密码哈希或 API Key Secret；
- 查看 Prompt / Response；
- 直接编辑余额；
- 无审计覆盖 Discord Binding；
- 自动合并账号；
- 自动迁移 API Key、资产或历史归属；
- 直接改变 NewAPI Account Status。

Legacy Password Recovery 中，管理员不得查看或指定用户最终密码。

# 241. Games

Games 由动态 Game Registry 驱动，不在 Sidebar 中写死首发游戏。

Game Detail：

```text
Overview
Metadata
Publication
Runtime
Configuration
Fairness
History
```

配置工作流：

```text
Active Version
→ Clone as Draft
→ Edit Allowed Fields
→ Validate
→ Preview
→ Confirm Activation
→ New Active Version
```

已生效并被历史 Round 引用的版本不得直接编辑。恢复历史配置时，应复制或重新激活为新的版本，不回写旧版本。

概率、赔率、RTP 和其他经济配置的激活仅允许 Super Admin。具有 Games Scope 的 Operator 可以管理普通 Metadata、目录展示和安全的 Runtime 操作。

已冻结产品规则在后台只读展示，不能通过通用 JSON Editor 绕过。

Maintenance 主要阻止新 Round，不遗弃已经接受的 Round。Games Operator 可以暂停新 Round，但不能修改已产生的结果或资产。

Game Registry 不能只凭后台表单无代码生成完整可玩游戏。

# 242. Poker Operations

Poker 使用独立运营模块：

```text
Poker
│
├── Service Overview
├── Tables
├── Sessions
├── Hands
├── Recovery
└── Chat Moderation
```

Table Detail 至少展示：

- Table ID；
- 房主；
- Blind / Ante；
- Buy-in 规则；
- 当前玩家与 Spectator；
- 当前 Hand；
- Pot / Side Pot 摘要；
- Session 状态；
- Connection 状态；
- Safe Leave / Cash Out 状态；
- Recovery 状态。

允许的安全操作：

- Stop / Resume Accepting Players；
- Stop / Resume New Hands；
- Close After Current Hand；
- Remove Player After Hand；
- Remove Spectator；
- Mute Chat User；
- Pause Table；
- Request Recovery。

禁止：

- 修改 Stack 或 Pot；
- 指定赢家；
- 修改牌序或 Settlement；
- 当前 Hand 强制 Cash Out；
- 提前查看 Hole Cards；
- 提前查看完整 Server Seed 或牌序。

聊天内容可以被隐藏，用户可以被 Mute，但原始记录和管理动作继续保留审计。

Poker Recovery 只能执行状态机允许的 `Resume / Pause / Safe Close / Escalate`。

Emergency Pause 仅允许 Super Admin，并要求 Fresh Authentication 和原因。

# 243. Economy

```text
Economy
│
├── Asset Overview
├── User Wallets
├── Transactions & Ledger
├── Transfers
├── Reconciliation
├── Adjustments
├── Issuance & Burn
└── Migration Snapshots
```

管理员可以查看：

- Active NewAPI Quota；
- Reserve API Credit；
- Available Chips；
- Poker In Play；
- Processing Assets；
- Total Assets。

Wallet 与 Ledger 默认只读。后台不得提供直接编辑最终 Balance 的输入框。

Transfer Detail 显示完整业务状态、资产变化与关联 ID。

# 244. Reconciliation 与 Admin Adjustment

Reconciliation Queue 按状态组织：

```text
Pending
Source Debited
Target Credited
Compensating
Failed
Resolved
```

Reconciliation Worker 优先自动处理。管理员只能执行当前状态机允许的：

- Retry；
- Resume；
- Compensate；
- Mark for Review。

不得直接把交易状态改为 `CONFIRMED`，不得删除失败记录，也不得手工改余额来掩盖差异。

Admin Adjustment 仅允许 Super Admin：

```text
Select User
→ Select Asset
→ Increase / Decrease
→ Enter Amount
→ Required Reason
→ Required Reference
→ Preview Before / Delta / After
→ Fresh Authentication
→ Typed Confirmation
→ Submit
→ Ledger + Audit Result
```

负向调整不得产生负余额。普通 Adjustment 不得直接修改 Poker In Play；Poker 资产错误进入 Poker Settlement Repair。

V1 不提供面向全部用户的一键批量资产发放。开服补助或活动批量奖励必须作为独立、版本化 Grant / Campaign 需求设计。

# 245. Rewards Operations

```text
Rewards
│
├── Configurations
│   ├── Daily
│   ├── Hourly
│   └── Relief Fund
├── Claims
├── Issuance Analytics
└── Maintenance
```

配置工作流：

```text
Draft
→ Validate
→ Preview
→ Schedule / Activate
→ Version Locked
```

固定产品规则只读展示：

```text
Initial Grant
1,000 API Credit（Registration / Migration 各自一次）

Daily Check-in
500 API Credit / Asia/Shanghai 自然日

Hourly Reward
固定数量 100，非随机

Relief Fund
固定数量 300
Total Assets < 10
成功领取后滚动 4 小时
```

Hourly Reward 尚未确认的资产类型、时间口径、累积和每日限制，以及 Relief Fund 尚未确认的资产类型、累积和 Active Poker 行为，不得提前作为可编辑生产字段。

管理员不能把失败 Claim 直接改成 `SUCCESS`。安全重试必须复用原 Claim / Business ID。人工补发资产进入 Economy Adjustment，不伪造签到记录，也不能为特定用户覆盖 Initial Grant 1,000、Daily 500、Hourly 100 或 Relief 300 的固定数量。

配置变化只影响生效后的新 Claim。

# 246. Rankings Operations

```text
Rankings
│
├── Assets & Games
├── RP Usage
├── Aggregation Status
├── Historical Snapshots
└── Repair & Rebuild
```

管理员不得直接编辑用户分数。

标准重建流程：

```text
Select Period / Scope
→ Rebuild Shadow Snapshot
→ Compare Old vs New
→ Review Diff
→ Publish New Snapshot
```

错误数据通过源记录排除、修复和重新聚合处理。排除与取消排除均须填写原因并形成新的 Audit 操作。

后台提供与公共页面一致的 Rankings Preview。

# 247. Records

Records 统一承载：

- Direct Play Round；
- Poker Session；
- Poker Hand；
- Settlement / Refund Incident；
- Fairness Verification。

支持按用户、游戏、时间、状态和稳定 ID 查询。

记录详情默认只读，不得修改：

- 下注；
- 结果；
- Seed；
- 牌序；
- Payout；
- Stack Before / After；
- 历史 Settlement。

当前私有牌面和未公开 Seed 不因管理员角色而提前显示。

发现异常时：

```text
Record Detail
→ Create Incident
→ Economy / Poker Repair Workflow
```

退役游戏和已关闭牌桌的历史记录继续保留。V1 不提供批量导出全部用户敏感游戏记录。

# 248. Announcements & Events Operations

沿用 IA-10 已冻结结构：

```text
All Content
Drafts
Scheduled
Published
Expired / Archived
Pinned Order
Delivery Placements
Acknowledgements Editor
Versions / Audit
```

具有 Announcements Scope 的 Operator 可以创建、编辑和发布普通公告。

Entry Popup 排期必须保证同一时点最多一条有效。Re-notify 必须经过影响确认，并生成新的 Notification Revision。

已发布公告不允许无审计硬删除。Sponsor / Contributor 数据继续遵守同意与隐私要求。

发布前提供 PC、Mobile 与 Entry Popup Preview。

# 249. Operations、Service Health 与 Maintenance

Operations 不替代 Cockpit、Portainer 或服务器终端。

```text
Operations
│
├── Service Health
├── Background Jobs
├── Maintenance
└── Incidents
```

Service Health 可以展示：

- Chaldea Frontend；
- Chaldea Backend；
- NewAPI Connectivity；
- Poker Service；
- PostgreSQL / Chaldea；
- PostgreSQL / NewAPI；
- Redis；
- Reconciliation Worker；
- Ranking Aggregator；
- Announcement Scheduler；
- Reward Jobs。

不得提供：

- SSH；
- Docker Shell；
- 任意命令执行；
- PostgreSQL SQL Console；
- Redis Command Console；
- 系统包升级；
- VPS 防火墙管理。

支持范围化 Maintenance：

- Chaldea User Writes；
- Wallet & Exchange；
- Rewards；
- Direct Play New Rounds；
- Poker New Tables / New Hands；
- Rankings Publishing；
- Announcements Scheduling。

流程：

```text
Select Scope
→ Required Reason
→ Preview Impact
→ Optional Schedule
→ Fresh Authentication
→ Confirm
→ Maintenance Active
```

Maintenance 不得遗弃已接受 Round、进行中 Poker Hand、处理中 Transfer 或已经开始的奖励发放。NewAPI 模型 API 的底层维护继续由 NewAPI Admin 负责。

# 250. Access Control

```text
Access Control
│
├── Administrators
├── Roles
├── Operator Scopes
└── Permission Audit
```

Super Admin 专属能力至少包括：

- 管理 Chaldea Operations 角色和权限；
- Admin Adjustment；
- Discord Rebind 最终执行；
- Legacy Account Recovery；
- 经济与奖励配置激活；
- 全站 Maintenance；
- Poker Emergency Pause；
- 关键数据修复的最终发布。

Operator 只能在被授予的模块 Scope 内执行日常操作，不能直接改变资产、账号绑定或管理员权限。

Auditor 可以查看业务数据、配置版本与 Audit，但不能执行任何写操作。

V1 不建设完全自定义的逐权限可视化权限设计器；采用三种基础角色和有限模块 Scope。

# 251. 危险操作分级

## Level 1 — Routine

例如查看详情、保存 Draft、修改筛选、编辑未发布普通元数据。无需额外确认，但写操作仍记录普通审计。

## Level 2 — Impactful

例如发布普通公告、激活非经济展示配置、开启单游戏 Maintenance、触发 Rankings Rebuild、关闭 Poker 新玩家加入。

要求：

- Impact Summary；
- Explicit Confirmation；
- Audit。

## Level 3 — Critical

例如：

- 资产 Adjustment；
- Discord Rebind；
- Access Control 修改；
- 全站 Maintenance；
- Poker Emergency Pause；
- 发布经济参数版本；
- 手工补偿资产交易。

要求：

```text
Fresh Authentication
+
Required Reason
+
Typed Confirmation
+
Impact Preview
+
Unique Operation ID
+
Append-only Audit
```

V1 不强制双人审批。管理员团队扩大后，可以再引入 Four-eyes Approval。

# 252. Audit

Audit 为 append-only。

每条管理员写操作至少记录：

```text
Actor
Actor Role / Scope
Action
Target Type
Target ID
Before
After
Required Reason
Operation ID
Result
Timestamp
Related Business ID
```

Audit 不保存明文密码、API Key Secret、当前未公开 Seed、完整 Prompt / Response 或其他不必要的敏感信息。

Audit：

- 不允许编辑；
- 不允许删除；
- 支持筛选和精确 ID 搜索；
- 可以跳回目标对象；
- 对象详情可以查看相关 Audit。

金融操作的撤销必须通过新的反向账变或补偿操作完成，不能删除原 Ledger 或 Audit。

# 253. PC、Tablet 与 Mobile

PC：

- 持久 Sidebar；
- 宽表格和多列筛选；
- Detail Page + Optional Drawer；
- 适合高密度管理。

Tablet：

- 可折叠 Sidebar；
- 保留主要筛选；
- Detail 使用单列或双列响应式布局。

Mobile：

- Sidebar 变为 Drawer；
- 列表变为管理 Card；
- Filter 使用全屏层或 Bottom Sheet；
- 危险操作使用全屏确认；
- 不依赖 Hover；
- 排序不能只依赖拖拽，必须提供上移 / 下移等替代方式；
- 关键管理能力仍可完成。

具体后台配色、图标、视觉密度和 FGO 装饰留到 Art Direction v0.4。

# 254. IA-12 核心 UX Flow

## 254.1 处理长时间 Pending Transfer

```text
Needs Attention
→ Transfer Detail
→ 查看 State Timeline / Asset Delta / Related IDs
→ Retry / Resume / Compensate / Mark for Review
→ Fresh Authentication（Critical 时）
→ Operation Result
→ Ledger / Audit Cross-link
```

## 254.2 激活游戏配置

```text
Games
→ Game Detail
→ Clone Active Version as Draft
→ Edit Allowed Fields
→ Validate
→ Preview
→ Super Admin Confirm Activation
→ New Active Version
→ Audit
```

## 254.3 Discord Rebind Support Case

```text
Users & Identity
→ Create / Open Support Case
→ Verify Original Account Ownership
→ Verify New Discord Uniqueness
→ Approve
→ Super Admin Fresh Authentication
→ Execute Rebind
→ Close Case
→ Audit
```

## 254.4 Poker 故障恢复

```text
Poker Overview / Attention
→ Table Detail
→ Pause / Request Recovery
→ Load Authoritative State
→ Resume / Safe Close / Escalate
→ Preserve Hand / Pot / Stack / Settlement
→ Audit
```

## 254.5 Rankings 重建

```text
Rankings
→ Select Period / Scope
→ Build Shadow Snapshot
→ Compare Diff
→ Review Exclusions / Repairs
→ Publish New Snapshot
→ Audit
```

## 254.6 发布 Entry Popup

```text
Announcements
→ Draft
→ Set Entry Popup Placement
→ Validate No Overlap
→ PC / Mobile / Popup Preview
→ Publish / Schedule
→ Audit
```

# 255. IA-12 最终冻结结论

以下内容已按用户确认的 1A–130A 冻结：

1. 继续采用 NewAPI Admin + Chaldea Operations 双后台。
2. Chaldea Operations 不重做 NewAPI 原生用户、渠道、模型路由、倍率和计费后台。
3. 新增 Chaldea `Models` 一级模块，管理模型广场元数据。
4. 新增 `Users & Identity` 一级模块，管理 Master Moderation 与 Account Support。
5. 新增独立 `Poker` 一级运营模块。
6. 将 `Game Records` 扩展并重命名为通用 `Records`。
7. 将含义模糊的 `Operations Analytics` 调整为 `Operations`，承载 Health、Jobs、Maintenance 和 Incidents。
8. 新增 `Access Control` 一级模块。
9. V1 使用 Super Admin、Operator、Auditor 三种基础角色。
10. Operator 再按照 Models、Users、Games、Poker、Rewards、Rankings、Records、Announcements 等模块 Scope 授权。
11. Chaldea Operations 权限保存在 Chaldea 侧，并通过稳定 `newapi_user_id` 关联。
12. NewAPI Admin 身份不会自动授予 Chaldea Operations 权限。
13. Chaldea Super Admin 也不会自动获得 NewAPI Admin 权限。
14. Super Admin 拥有完整 Chaldea Operations 权限。
15. Operator 只拥有被授予 Scope 的日常运营能力。
16. Auditor 为全后台只读角色。
17. 资产调整、Discord 换绑、Access Control 和全站维护仅允许 Super Admin。
18. 权限必须由服务端校验，隐藏 Sidebar 入口不能代替权限检查。
19. `/ops` 默认进入 Operations Overview。
20. PC 使用持久 Sidebar。
21. Sidebar 按 Command、Catalog & Community、Economy & Data、Administration 分组。
22. 顶部显示 Production / Staging / Development 环境标识。
23. 顶部提供 Global Search。
24. 顶部提供 Needs Attention 数量。
25. 顶部提供返回普通用户站点的入口。
26. Open NewAPI Admin 在新标签页打开，并只对真正拥有 NewAPI Admin 权限的人显示。
27. 后台详情对象使用稳定、可分享的 Deep Link。
28. Global Search 支持用户、Transfer、Transaction、Round、Table、Session、Hand、Announcement、Config 和 Audit ID。
29. 可以通过 Master 昵称、Account ID、newapi_user_id、Discord ID 和 API Key ID 定位用户。
30. 不允许搜索或展示 API Key Secret、密码、Prompt、Response 或未公开牌面。
31. 筛选、排序、分页状态写入 URL。
32. 后台列表默认采用明确分页，不以无限滚动作为唯一浏览方式。
33. 从 Detail 返回列表时恢复筛选、排序、页码和滚动位置。
34. 复杂详情使用正式页面，Drawer 只作为快速预览。
35. 深层后台页面使用 Breadcrumb。
36. Overview 首要展示 Needs Attention，而不是先展示大量图表。
37. Attention 聚合资产、奖励、游戏、Poker、排行榜、模型同步、公告和账号支持异常。
38. Attention Item 使用 Critical / Warning / Info 严重级别。
39. Acknowledge 只代表管理员已经查看，不代表底层问题已解决。
40. 未解决的金融或牌局问题不能被永久隐藏或删除。
41. Attention Item 必须链接到对应对象详情。
42. Overview 展示在线用户、活动 Round、活动牌桌、Pending Transfer 和当日发行/回收等业务摘要。
43. 所有摘要显示数据更新时间或数据新鲜度。
44. Models 管理 NewAPI 同步模型与 Chaldea 展示元数据。
45. Model ID、底层状态、原始价格和渠道摘要在 Chaldea Operations 中只读。
46. 展示名称、Logo、简介、标签、推荐用途、上下文、排序、推荐和公开状态由 Chaldea 管理。
47. 模型状态至少区分 Pending Metadata、Published、Hidden、Unavailable、Retired。
48. 模型发布前必须通过元数据、真实模型存在性、价格展示和状态映射校验。
49. Chaldea 不允许用展示字段修改真实计费结果。
50. 退役模型从目录隐藏，但历史 API Usage 和排行榜模型名称继续保留。
51. Users & Identity 提供 User Search、Master Moderation、Support Cases、Binding Conflict 和 Migration Acknowledgement。
52. 用户详情使用 Overview、Identity、Assets、Rewards、Activity、Support、Audit 分区。
53. 用户详情不显示密码、密码哈希、API Key Secret、Prompt 或 Response。
54. 支持 Rename Required 和有审计的 Forced Rename。
55. 支持查看 Reserved Name、昵称冷却和 Profile Status。
56. Discord Rebind 与 Legacy Binding 使用正式 Support Case。
57. Support Case 使用 Open、Verifying、Approved、Executed、Closed、Rejected 等状态。
58. Discord Rebind 必须验证原账号所有权、新 Discord 唯一性、原因和 Fresh Authentication。
59. Legacy Password Recovery 中管理员不能查看或指定用户最终密码。
60. Account Disable / Suspend 继续进入 NewAPI Admin，不在 Chaldea 用户详情中直接修改。
61. 用户资产调整必须进入 Economy，不在 Users 页面直接改余额。
62. Games 继续由动态 Game Registry 驱动，不在 Sidebar 中写死首发游戏。
63. 游戏详情包含 Overview、Metadata、Publication、Runtime、Configuration、Fairness 和 History。
64. 当前 Active Config 不可直接编辑，只能 Clone as Draft。
65. 配置采用 Draft → Validate → Preview → Activate。
66. 概率、赔率、RTP 或其他经济配置激活仅允许 Super Admin。
67. 普通 Metadata 和目录展示可以由具有 Games Scope 的 Operator 管理。
68. 已经冻结的产品规则在后台只读，不能通过通用 JSON Editor 绕过。
69. 进入 Maintenance 主要阻止新 Round，不遗弃已接受 Round。
70. Games Operator 可以暂停新 Round，不能修改已经产生的结果或资产。
71. 恢复历史配置时发布一个新版本，不回写旧版本。
72. 后台表单不能无代码生成一款完整可玩的新游戏。
73. Poker 独立于 Games Registry 日常配置，拥有专门运营模块。
74. Poker 模块包含 Service Overview、Tables、Sessions、Hands、Recovery、Chat Moderation。
75. 管理员页面不得提前显示未公开 Hole Cards。
76. 管理员页面不得提前显示完整 Server Seed 或牌序。
77. 允许 Stop Accepting Players、Stop New Hands、Close After Hand、Remove After Hand 和 Mute。
78. Emergency Pause 要求 Super Admin、Fresh Authentication 和原因。
79. 不得直接修改 Stack、Pot、赢家、牌序或 Settlement。
80. 不得在当前 Hand 中强制 Cash Out。
81. 聊天消息可以隐藏或用户可以被 Mute，但原记录继续保留审计。
82. Poker Recovery 只能执行状态机允许的 Resume、Pause、Safe Close 或 Escalate。
83. Economy 包含 Asset Overview、Wallets、Ledger、Transfers、Reconciliation、Adjustments、Issuance & Burn、Migration。
84. 管理员可查看 Active NewAPI Quota、Reserve、Available Chips、Poker In Play 和 Processing Assets。
85. 管理员同时看到统一 Total Assets 和各来源拆分。
86. Wallet 与 Ledger 默认只读。
87. 不提供任何直接编辑最终 balance 的输入框。
88. Transfer Detail 显示完整业务状态、资产变化和关联 ID。
89. Reconciliation Queue 优先由 Worker 自动处理。
90. 管理员只能执行状态机允许的 Retry、Resume、Compensate 或 Mark for Review。
91. 不得手工把交易状态直接改成 CONFIRMED。
92. Admin Adjustment 仅允许 Super Admin。
93. Admin Adjustment 必须填写原因和 Reference。
94. 提交前展示 Balance Before / Delta / Balance After。
95. Admin Adjustment 要求 Fresh Authentication 和 Typed Confirmation。
96. 负向调整不得产生负余额。
97. 不得通过普通 Adjustment 直接修改 Poker In Play。
98. Poker 资产错误进入 Poker Settlement Repair。
99. V1 不提供面向全部用户的一键批量资产发放。
100. Rewards 包含 Configurations、Claims、Issuance Analytics 和 Maintenance。
101. 奖励配置采用 Draft、Validate、Preview、Schedule / Activate、Version Locked。
102. Initial Grant 1,000 API Credit、Daily 500 API Credit、Hourly 100、Relief 300，以及 Relief Fund 的总资产小于 10和滚动 4 小时规则均以只读政策形式展示。
103. Hourly Reward 尚未确认的资产类型、时间口径、累积和每日限制，以及 Relief Fund 尚未确认的资产类型、累积和 Active Poker 行为，不能提前作为可编辑生产字段。
104. 管理员不能把失败 Claim 直接改成 SUCCESS。
105. 安全重试必须继续使用原 Claim / Business ID。
106. 人工补发资产进入 Economy Adjustment，不伪造签到记录。
107. 奖励配置变化只影响生效后的新 Claim。
108. 后台不能为特定用户覆盖 Initial Grant 1,000、Daily 500、Hourly 100 或 Relief 300 的固定数量，也不能伪造签到或救济结果。
109. Rankings 包含 Assets & Games、RP Usage、Aggregation Status、Historical Snapshots、Repair & Rebuild。
110. 管理员不能直接编辑某个用户的榜单分数。
111. 重建采用 Shadow Snapshot → Compare Diff → Publish。
112. 错误数据通过源记录排除、修复和重新聚合处理。
113. 排除记录必须填写原因并保留审计。
114. 取消排除也通过新的审计操作完成。
115. 后台提供与公共页面一致的 Rankings Preview。
116. Records 统一承载 Direct Play Round、Poker Session 和 Poker Hand。
117. 支持按用户、游戏、时间、状态和稳定 ID 查询。
118. 记录详情默认只读。
119. 不得修改下注、结果、Seed、牌序、Payout 或历史 Settlement。
120. 当前私有牌面和未公开 Seed 不因管理员权限提前显示。
121. 发现异常时创建 Incident，并进入 Economy / Poker Repair。
122. 退役游戏和关闭牌桌的历史记录继续保留。
123. V1 不提供批量导出所有用户敏感游戏记录。
124. Announcements Operations 继续沿用 IA-10 已冻结流程。
125. 具有 Announcements Scope 的 Operator 可以创建、编辑和发布普通公告。
126. Entry Popup 排期继续保证同一时间最多一条有效。
127. Re-notify 必须经过影响确认并生成新 Notification Revision。
128. 已发布公告不允许无审计硬删除。
129. Sponsor 信息继续遵守同意和隐私规则。
130. PC、Mobile 和 Entry Popup Preview 在发布前均可查看。

# 256. IA-13 — Public Home、Login / Registration 与全站 UX Flow 最终整合

本节是 v0.3 Information Architecture 的最终收尾阶段。

IA-13 负责：

- 冻结 Public Home 的完整内容职责；
- 冻结 Login 与 Discord Registration 页面；
- 统一 Entry Popup、Authentication、Account Status、Master Initialization、Migration Notice、Permission、Resource Availability 与 Return-to-Intent 的顺序；
- 建立全站错误、Loading、Processing、通知和空状态体系；
- 汇总关键跨模块流程；
- 完成 PC / Tablet / Mobile 页面家族总审计；
- 对 IA-01～IA-12 的术语、路由、权限和 Cross-link 做最终一致性检查；
- 将 v0.3 从 DRAFT 升级为正式页面结构基线。

本节不新增普通用户一级产品域，也不冻结最终色板、FGO 角色、背景、图标、动画或页面视觉稿。

每小时奖励数量 100 与救济金数量 300 已通过 v0.3.1 修订冻结；其资产类型、Hourly 时间口径、累积和 Active Poker 行为仍读取服务端实际配置。

# 257. Public Home 产品定位

Public Home 使用：

**品牌入口 + 产品发现 + 公共社区内容**

的混合结构。

```text
/          = Public Home
/dashboard = 登录后的个人 Command Center
```

登录用户访问 `/` 时仍然看到同一公共首页，不强制重定向至 Dashboard；页面通过登录态增强调整 CTA，但不复制 Dashboard 的个人资产、奖励领取、API Usage、Active Game Round 或 Active Poker Session。

Public Home 需要回答：

1. Chaldea Platform 是什么；
2. 当前可以使用哪些主要能力；
3. Models / API、Entertainment 与 Poker 是否可用；
4. 有哪些值得发现的模型、游戏、排行榜和公告；
5. 当前用户下一步可以前往哪里。

# 258. Public Home 信息架构

Public Home 使用以下逻辑区域：

```text
Public Home
│
├── Critical Notice（条件式）
├── Hero / Platform Identity
├── Public Home Banner（条件式）
├── Platform Status
├── Main Pathways
│   ├── Models & API
│   └── Entertainment
├── Featured Models（条件式）
├── Featured Games & Poker（条件式）
├── Rankings Preview（条件式）
├── Recent Public Wins（条件式）
├── Announcements & Events
├── Acknowledgements Entry
└── Footer
```

这些是信息职责，不表示最终必须渲染为固定数量的卡片，也不限制 Art Direction 对构图、节奏或模块组合方式的设计。

所有数据型区域必须来自真实配置或真实聚合。没有真实内容时隐藏对应区域，不用假数据填充版面。

# 259. Critical Notice、Entry Popup 与 Home Banner

三者必须分离：

## Critical Notice

用于平台级故障、登录系统不可用、钱包维护、Poker 紧急暂停或安全事件。它表达真实服务影响，不属于普通营销或公告轮播。

## Entry Popup

只在未登录用户进入 `/` 或 `/login` 时按照 IA-10 规则检查。它与 Public Home 正文独立；加载失败不得阻止首页或登录。

普通 Entry Popup 不强制覆盖 `/register`，也不强制覆盖 Model Detail、Rankings、Announcement Detail 等公共 Deep Link。

## Public Home Banner

来自 Announcement Placement，用于新模型、游戏活动、普通重要公告等公共内容。

V1 同一时点最多展示一条主要 Home Banner，不使用自动轮播。其他公告进入 Announcements 摘要或公告列表。

# 260. Hero 与登录状态增强

Hero 负责表达平台名称、简洁定位和 Models / Entertainment 两条主路线。具体文案、角色素材和视觉构图留到 v0.4。

未登录 CTA：

```text
Explore Models
Explore Games
Login
Register with Discord
```

已登录 CTA：

```text
Open Dashboard
Explore Models
Explore Games
```

登录用户仍使用同一个 Public Home，不额外开发 Auth Home。

# 261. Platform Status 与主产品路径

Public Home 提供轻量公开状态：

```text
Models / API
Entertainment
Poker
```

状态至少可以表达：

- Operational；
- Degraded；
- Maintenance；
- Unavailable。

状态必须来自真实服务检查或正式运营状态，并显示更新时间。不得公开 Provider 凭证、服务器 IP、内部数据库名称或不必要的故障原文。

V1 不新增独立 `/status` 页面；更详细的维护说明进入 Announcement Detail。

主产品路径：

- Models & API → `/models`；
- Entertainment → `/entertainment`；
- Browse All Games → `/games`。

Public Home 不复制 API Access 的 Base URL、cURL 或完整接入内容。

# 262. Featured Models、Featured Games 与 Poker

Featured Models 由 Chaldea Operations Models 的真实推荐与公开状态动态生成，可以展示 Display Name、Logo、简短介绍、推荐用途、价格摘要和可用状态。

Featured Games 由 Game Registry 的真实发布、推荐、活动和运行状态动态生成，不写死首发游戏数量。

Poker Spotlight 只有 Poker 已正式发布且允许公开发现时才出现，并使用真实 Poker Service 状态。

未登录用户点击需要认证的实际游戏或 Poker 入口时，保存安全 Return-to-Intent，完成统一 Gate 后返回原目标；系统不能自动替用户下注或 Buy-in。

# 263. Rankings、Recent Wins、Announcements 与 Acknowledgements

## Rankings Preview

Public Home 可以同时预览：

- Assets & Games；
- RP Usage。

只显示公开聚合数据、榜单名称、少量排名、指标和 Last Updated；完整筛选仍进入 `/rankings`。

## Recent Public Wins

使用事件发生时的 Master Identity Snapshot，展示游戏、下注、净赢金额和发生时间，不展示用户当前资产。没有合格记录时隐藏整个模块。

## Announcements & Events

只展示当前重要和最新摘要，完整内容进入 `/announcements/:id`。

## Acknowledgements

首页只提供规范致谢公告入口、简短说明和更新时间，不复制完整 Sponsor / Contributor List。入口、弹窗和详情共用同一规范公告来源。

# 264. Public Home 内容策略与 Footer

首页禁止：

- 虚构热门度；
- 虚构在线人数；
- 假服务状态；
- 为填空而生成的模型、游戏或中奖数据；
- 强制自动播放视频；
- 自动播放声音；
- 自动轮播 Banner；
- 无限滚动作为唯一浏览方式。

Footer 只聚合已经存在的产品路由：

- Models；
- Entertainment；
- Games；
- Rankings；
- Announcements；
- Login；
- Discord Registration；
- 登录状态下的 Dashboard 与 Personal Hub。

IA-13 不擅自创建 Terms、Privacy、Cookie、Sponsor Payment 或独立 Public Status 页面。这些内容如以后需要，必须独立确认。

# 265. Login 产品职责与页面结构

`/login` 只服务已经存在的账号。

页面结构：

```text
Login
│
├── Entry Popup（条件式、仅未登录）
├── Login Introduction
├── Continue with Discord
├── Password Login
│   ├── Password Login Identifier
│   ├── Password
│   ├── Show / Hide
│   └── Login
├── Forgot Password
├── Discord Registration Entry
├── Return-to-Intent Summary
└── Service / Error State
```

Discord Login 与 Password Login 在同一页面同时可见，Discord 位于上方作为主要方式，不使用隐藏式 Tab。

已登录用户访问 `/login` 时不再次显示登录表单，直接进入认证后 Gate。

# 266. Password Login Identifier 与密码交互

密码输入使用：

```text
Password Login Identifier
```

不得使用 Master 昵称、短 Account ID 或并不存在的 Email 字段替代。

Discord OAuth 创建的 NewAPI 账号必须拥有稳定、唯一、后续可用于密码登录的 Password Login Identifier。该标识在 Account & Security 和 Set Password 流程中明确展示。

如果技术核查发现 NewAPI 当前 OAuth 注册无法提供稳定 Identifier，必须在技术设计中增加受控适配或独立 Account Identifier 确认步骤；不得静默使用可修改的 Master 昵称。

密码交互：

- 支持密码管理器与正常自动填充；
- 支持 Show / Hide；
- Enter 可以提交；
- 提交期间锁定重复操作；
- 登录失败后保留 Identifier；
- 登录失败后清空密码字段；
- V1 不增加 Chaldea 自己的 Remember Me；
- Session 生命周期沿用真实 NewAPI 能力。

忘记密码：

- 已绑定 Discord → Fresh Discord Re-authentication；
- 无 Discord 的 Legacy Account → Account Support。

# 267. Login 状态与错误

至少支持：

- Invalid Credentials：通用“账号登录名或密码不正确”，不进行账号枚举；
- Too Many Attempts：显示安全重试时间，不公开内部阈值；
- Discord OAuth Denied：安全返回 Login；
- Discord Provider Unavailable：明确是 Provider 暂时不可用；
- Account Disabled / Suspended：进入 Restricted Account；
- Network Failure：保留非敏感输入并允许重试；
- Session Expired：重新认证后返回原安全页面，密码字段不恢复。

已经完成注册的用户使用 Discord Login 时只验证 Discord 身份与绑定，不重新检查首次注册所需 Role。

# 268. Discord Registration 产品职责与页面结构

`/register` 是 Discord 首次注册资格说明与 OAuth 启动页面，不是传统注册表单。

```text
Discord Registration
│
├── Registration Introduction
├── Eligibility Requirements
├── Required Discord Server
├── Required Discord Role
├── Join Server（有有效邀请链接时）
├── Continue with Discord
├── Existing User Login
└── Registration Status / Error
```

页面不提供：

- 普通用户名注册；
- 密码注册；
- Email / Phone 注册；
- Master 昵称或 Avatar 输入。

Required Server 与 Required Role 使用人类可读名称；Guild ID、Role ID 和 OAuth Scope 等技术 ID 不作为普通用户主要文案。

# 269. Discord Registration 验证、幂等与既有账号

注册流程：

```text
/register
→ Discord Authorization
→ OAuth State Validation
→ Discord Identity
→ Server Membership Validation
→ Required Role Validation
→ Existing Binding Check
→ NewAPI Account Creation
→ Initial Grant Creation: +1,000 API Credit
→ Authentication
→ INCOMPLETE Provisional Master Profile
→ Master Initialization
→ Completion Summary / Real Reward Status
→ Migration Notice（仅适用时）
→ Return-to-Intent or Dashboard
→ Deferred Post-login Popup
```

验证失败规则：

- 不在服务器 → 明确提示并在有配置时提供邀请链接；
- 缺少 Role → 明确提示，但平台不能自动授予 Role；
- Discord API 暂时不可用 → 显示临时验证故障，不得伪装成缺少 Role；
- 服务器或 Role 验证未通过 → 不创建账号和新用户初始赠金。

既有账号：

- Discord 已稳定绑定现有账号 → 转为登录现有账号；
- 不创建第二个账号；
- 不重复发放新用户初始赠金；
- 异常绑定冲突 → Account Support；
- 不自动合并账号或迁移资产；
- Legacy Account 不因访问注册流程自动换绑。

OAuth Callback、账号创建、奖励创建和 Provisional Profile 必须幂等。重复标签页、重复回调、刷新、超时或服务重启不得产生重复账号或奖励。

账号已创建但 1,000 API Credit 初始赠金仍在处理中，不删除账号，也不重新注册；继续 Master Initialization，并在 Completion Summary 显示真实奖励状态。

# 270. 统一访问 Gate

全站使用统一 Gate：

```text
Requested Route
→ Route Classification
→ Anonymous Entry Popup Check（仅 / 与 /login）
→ Public / Protected / Admin / Immersive Access Check
→ Authentication（如需要）
→ Account Status Gate
→ Master Initialization（如需要）
→ Migration Notice（如需要）
→ Role / Scope Permission Check
→ Resource Availability / Maintenance Check
→ Valid Return-to-Intent
→ 或 Dashboard / Safe Parent
→ Deferred Post-login Popup
```

优先级：

1. Account Status；
2. Master Initialization；
3. Migration Notice；
4. Role / Scope；
5. Resource Availability；
6. Return-to-Intent。

受限账号不进入 Master Initialization 或普通 Dashboard。

# 271. Return-to-Intent

Return-to-Intent 必须：

- 只允许 Chaldea 站内安全路径；
- 使用允许列表或受控 Route 解析；
- 拒绝外部 URL 和危险 Scheme；
- 具有有效期；
- 恢复前重新检查权限和资源状态；
- 使用一次后清除；
- 无效时进入 Dashboard 或安全父页面。

可以恢复页面位置与安全筛选状态，但不得自动执行：

- 游戏下注；
- Poker Buy-in / Cash Out；
- Wallet Exchange；
- Reward Claim；
- API Key 创建 / 删除；
- Profile 保存；
- Password 修改；
- 管理员写操作。

目标已经下线、维护或无权限时，页面说明原因并提供 Game Catalog、Dashboard、父级详情或其他安全入口，不自动替用户选择另一个目标。

# 272. 全站错误、Loading、Processing 与通知

统一错误状态：

- 401：重新认证并保留安全页面意图；
- 403：Access Denied，提供安全父页面；
- 404：不泄露私有资源是否存在；
- 409：可恢复业务冲突并提供下一步；
- 429：安全重试状态，不公开内部阈值；
- 503：说明受影响模块、仍可使用模块和维护信息。

部分服务降级时，不关闭未受影响的产品域。

读取：

- 使用 Skeleton 或明确 Loading；
- 超时后显示错误与重试；
- 不允许永久全屏 Spinner。

写操作：

```text
Idle
→ Submitting
→ Accepted / Processing
→ Confirmed
或
→ Failed / Returned / Needs Attention
```

提交后锁定重复操作。网络结果不确定时，先查询原 `operation_id / round_id / transfer_id / claim_id`，不能直接重放。

资产、奖励和游戏结算不采用未经服务端确认的乐观余额。

通知层级：

- Inline Message：字段错误、资格或余额问题；
- Toast：复制成功和普通非关键反馈；
- Persistent Banner：离线、服务降级和模块维护；
- Dialog / Sheet：删除 Key、Safe Leave、Active Poker Logout 和危险操作；
- Full-page / Interstitial：Master Initialization、Migration Notice、Restricted Account 和全站不可用。

Toast 不作为资产或危险操作最终结果的唯一展示。用户错误页可以提供支持 Reference ID，但不显示 Stack Trace。

# 273. 全站空状态

所有空状态必须说明：

```text
为什么为空
+
下一步可以做什么
```

标准示例：

- No API Keys → Create API Key；
- No API Usage → Open API Access；
- No Wallet Transactions → 当前尚无资产记录；
- No Game History → Browse Games；
- No Poker Tables → Create Table；
- No Announcements → 当前没有公告；
- No Ranking Entries → 当前周期尚无符合条件的数据；
- No Attention Items → 当前没有待处理问题。

“没有数据”与“加载失败”必须使用不同状态和文案。

# 274. 关键跨模块 UX Flow

## 274.1 发现并使用模型

```text
Public Home / Model Square
→ Model Detail
→ Use This Model
→ Login / Registration（如需要）
→ Unified Gate
→ API Keys（如没有 Key）
→ API Access
```

## 274.2 发现并开始游戏

```text
Public Home / Entertainment / Game Catalog
→ Game Entry
→ Login / Registration（如需要）
→ Unified Gate
→ Available Chips Check
→ Wallet / Rewards（如不足）
→ Return to Game
→ 玩家再次主动提交下注
```

## 274.3 新用户注册

```text
Public Home / Login / Register
→ Discord Registration
→ OAuth / Eligibility
→ Account + Reward
→ Master Initialization
→ Completion Summary
→ Return-to-Intent or Dashboard
```

## 274.4 迁移用户

```text
Login
→ Account Status
→ Master Initialization（如需要）
→ Migration Notice
→ Return-to-Intent or Dashboard
```

## 274.5 Poker 恢复

```text
Login
→ Unified Gate
→ Dashboard / Poker Lobby Active Session
→ Reconnect
→ 同一 Table / Seat / Session / Hand
```

## 274.6 资产处理中重新登录

```text
Wallet Processing
→ Session Expired
→ Login
→ Unified Gate
→ Return to Existing Transaction
→ Query Existing Transfer
```

## 274.7 Admin Deep Link

```text
/ops/...
→ Login
→ Account Status
→ Master Initialization / Migration Notice（如需要）
→ Chaldea Role / Scope
→ Resource Availability
→ Target Admin Page
```

# 275. PC、Tablet 与 Mobile 页面家族总表

| 页面家族 | PC | Tablet | Mobile |
|---|---|---|---|
| Public Home | 多区域宽屏布局 | 自适应双列 / 单列 | 单列、关键 CTA 优先 |
| Login / Registration | 集中窄内容区 | 居中或宽单列 | 全宽单列表单 |
| Model / Game Catalog | 完整筛选与列表 | 折叠筛选 | Filter Sheet |
| Dashboard / Wallet | 多列信息区 | 自适应区域 | 按优先级纵向排列 |
| Profile / Account | Preview + Edit | 双列或单列 | 单列 |
| Direct Play | Focused Layout | 自适应 | 保留 Bottom Navigation |
| Poker Table | Immersive | Immersive | Immersive，无 Bottom Navigation |
| History / Detail | 表格 + Detail | 响应式列表 | Card + 全屏详情 |
| Chaldea Operations | Sidebar | 折叠 Sidebar | Drawer + 管理 Card |

Tablet 按实际可用空间采用 Desktop 或 Mobile Pattern，不建立第三套独立导航。

复杂筛选在手机使用 Sheet；危险确认可以使用全屏层。所有表单与状态支持键盘、明确 Focus、Label、文字错误和 Screen Reader。业务截止时间明确显示时区。

# 276. 规范路由与术语最终审计

正式路由：

```text
Public Home             /
Dashboard               /dashboard

Login                   /login
Discord Registration    /register

Master Initialization   /onboarding/master
Migration Notice        条件式 Interstitial

Model Square            /models
Model Detail            /models/:model
API Keys                /api/keys
API Usage               /api/usage
API Access               /api/access

Wallet                  /wallet
Rewards Center          /rewards

Entertainment Hub       /entertainment
Game Catalog            /games
Game Entry              /games/:game_slug
Poker Lobby             /poker
Poker Table             /poker/table/:id

Rankings Center         /rankings
Game History            /history
Round Detail            /history/round/:id
Session Detail          /history/session/:id
Hand Detail             /history/hand/:id

Announcements           /announcements
Announcement Detail     /announcements/:id

Personal Hub            /me
Master Profile          /master-profile
Account & Security      /account/security

Chaldea Operations      /ops/*
```

最终清理以下旧语义：

- 独立 `/check-in`；
- 固定五个游戏的永久导航容量；
- `/history/poker/:id`；
- 把 `Game Records` 作为后台最终名称；
- 把 `Operations Analytics` 作为后台最终名称；
- 把 Master 昵称当作 Password Login Identifier；
- 认证成功后一律丢弃 Return-to-Intent 并进入 Dashboard；
- 把 Entry Popup、Migration Notice 或 Critical Notice 当作普通一级路由；
- Public / Auth 为同一公共内容开发两套路由。

筛选、Tab、分页和可分享状态写入 URL。从 Detail 返回列表时，尽量恢复筛选、页码和滚动位置。

# 277. v0.3 Final 状态

完成 IA-13 后：

- IA-01～IA-13 全部冻结；
- v0.3 从 DRAFT 升级为 FINAL；
- 本文成为后续 Art Direction、Page Layout、技术设计和实施规格的正式页面结构基线；
- 初始赠金 1,000、Daily 500、Hourly 100、Relief 300 已冻结；其余未确认的资产类型和周期规则仍由需求基线约束；
- 任何改变已冻结路由、页面职责、权限、资产语义或用户流程的需求，必须通过版本化变更重新确认。

下一阶段不是继续增加 IA 编号，而是由用户先审阅 v0.3 Final，再进入 Art Direction v0.4。

# 278. IA-13 最终冻结结论

以下内容已按用户确认的 1A–144A 冻结：

1. IA-13 作为 v0.3 Information Architecture 的最终收尾阶段。
2. 本轮不新增新的普通用户一级产品域。
3. 公共页面继续采用同一路由的登录态增强，不建立两套页面。
4. 具体色板、FGO 角色、背景、图标和动画继续留到 v0.4。
5. 奖励数值的后续版本化修订不改变 IA 页面结构；当前已冻结 1,000 / 500 / 100 / 300。
6. Public Home 采用品牌与产品入口混合型结构。
7. `/` 始终保持 Public Home。
8. 登录用户访问 `/` 不强制跳转 Dashboard。
9. Public Home 不复制 Dashboard 的个人资产、奖励和 Active Session。
10. Hero 表达平台定位与 Models / Entertainment 两条主路线。
11. 未登录 Hero 提供 Explore Models、Explore Games、Login、Discord Registration。
12. 登录后 Hero 的主要 CTA 改为 Open Dashboard。
13. 登录用户仍然看到同一个公共首页内容。
14. Entry Popup 与 Public Home 正文保持独立。
15. Critical Notice 与普通 Home Banner 分离。
16. V1 同一时点最多展示一条主要 Home Banner。
17. Public Home 不使用自动轮播 Banner。
18. Platform Status 只显示真实状态与更新时间。
19. V1 不新增独立 `/status` 页面。
20. 首页提供 Models & API 与 Entertainment 两个主入口区。
21. Featured Models 由 Chaldea Models 运营配置动态生成。
22. Featured Games 由 Game Registry 动态生成。
23. Poker Spotlight 只有 Poker 已发布时出现。
24. Rankings Preview 同时支持 Assets & Games 与 RP Usage 摘要。
25. 首页 Rankings 只显示公开聚合数据。
26. Recent Public Wins 使用事件发生时身份快照。
27. Recent Public Wins 不显示用户当前总资产。
28. Announcements 只做摘要并进入标准详情页。
29. Acknowledgements 在首页只做规范公告入口，不复制完整名单。
30. 没有真实内容的首页模块直接隐藏。
31. 首页不使用虚构热门度、虚构在线人数或假状态。
32. V1 不强制自动播放视频或音频。
33. Footer 只聚合已经存在的产品路由。
34. IA-13 不擅自创建未确认的 Terms、Privacy、Cookie 或 Sponsor Payment 页面。
35. `/login` 仅用于已经存在的账号登录。
36. Discord Login 和 Password Login 在同一页面同时可见。
37. Discord Login 放在密码表单上方作为主要方式。
38. 密码表单使用 Password Login Identifier。
39. Master 昵称不能用于密码登录。
40. 短 Account ID 不能用于普通密码登录。
41. 账号输入支持密码管理器和正常自动填充。
42. 密码字段支持显示 / 隐藏。
43. 登录失败后保留账号登录名。
44. 登录失败后清空密码字段。
45. V1 不新增独立的 Chaldea Remember Me 开关。
46. 忘记密码通过 Discord Fresh Re-authentication 处理。
47. 无 Discord 的 Legacy Account 使用 Account Support。
48. Login 页面明确提供 Discord Registration 入口。
49. Login 可以显示安全的 Return-to-Intent 目标摘要。
50. Login 不显示或接受外部 Return URL。
51. 已经登录的用户访问 Login 时直接进入认证后 Gate。
52. 账号或密码错误使用通用提示，避免账号枚举。
53. 登录限流显示可重试状态，但不公开内部阈值。
54. Disabled / Suspended Account 进入受限状态。
55. Discord Provider 故障与账号不存在必须区分。
56. 网络失败保留非敏感输入并允许重试。
57. Discord 登录不重新检查已经注册用户的 Role。
58. 登录成功后统一进入 Account Status、Onboarding 与 Return-to-Intent Gate。
59. `/register` 是 Discord 注册说明页，不是传统注册表单。
60. 页面不提供普通用户名、密码、邮箱或手机号注册。
61. 页面不提前要求填写 Master 昵称和头像。
62. 页面显示当前配置的 Required Server 与 Required Role。
63. 服务器和 Role 使用人类可读名称。
64. 配置了有效邀请链接时可以显示 Join Server。
65. 页面提供 Existing User Login 入口。
66. 首次注册只通过 Discord OAuth。
67. 用户取消 OAuth 后安全返回 Registration 页面。
68. 注册时服务端验证服务器成员关系。
69. 注册时服务端验证指定 Role。
70. Discord 校验暂时不可用时不得错误显示为缺少 Role。
71. 已经绑定现有账号的 Discord 再次注册时转为登录现有账号。
72. Registration-to-Login 不创建新账号或重复新用户初始赠金。
73. 异常 Discord Binding Conflict 进入支持流程。
74. V1 不自动合并两个账号。
75. Legacy Account 不因注册流程自动绑定新的 Discord。
76. OAuth Callback、账号创建和新用户初始赠金必须幂等。
77. 重复标签页和重复 Callback 不产生重复账号。
78. 账号已创建但奖励仍在处理时不删除账号。
79. 奖励处理中仍可继续 Master Initialization，并显示真实奖励状态。
80. 初始化中断后恢复同一个 `INCOMPLETE` Profile。
81. OAuth 创建的 NewAPI 账号必须拥有稳定 Password Login Identifier。
82. 该 Identifier 在 Account & Security 和 Set Password 中展示。
83. 不得使用可修改的 Master 昵称替代 Password Login Identifier。
84. 技术核查无法提供稳定 Identifier 时，必须回到技术设计补适配，而不是前端自行猜测。
85. 注册完成后显示真实 Account、Profile 与 Reward 状态。
86. 注册流程继续保留合法 Return-to-Intent。
87. 已经登录的用户不能通过 `/register` 创建第二个账号。
88. 服务器或 Role 校验失败时不创建账号或新用户初始赠金。
89. 所有路由先分类为 Public、Protected、Admin 或 Immersive。
90. 匿名 Entry Popup 只在 `/` 和 `/login` 检查。
91. 普通 Entry Popup 不在 `/register` 强制弹出。
92. 公共 Deep Link 不强制显示普通入口致谢弹窗。
93. Protected Route 在认证前保存安全 Return-to-Intent。
94. 认证后首先检查 Account Status。
95. 受限账号不进入 Master Initialization 或普通 Dashboard。
96. 活动账号未初始化时进入 Master Initialization。
97. 迁移用户初始化后进入 Migration Notice。
98. 管理员页面在上述 Gate 后再校验 Role / Scope。
99. 恢复目标前重新检查资源是否存在和可用。
100. 目标有效时恢复原目标。
101. 没有有效目标时进入 Dashboard。
102. 普通 Post-login Popup 排在 Initialization、Migration Notice 和关键恢复流程之后。
103. Return-to-Intent 只允许安全站内路径。
104. Return-to-Intent 具有有效期并在使用后清除。
105. Return-to-Intent 不自动重放任何副作用操作。
106. 目标下线或维护时显示原因和安全替代入口。
107. Session Expired 后重新登录并返回原安全页面。
108. 重新认证后不恢复密码字段。
109. Poker、活动 Round 和 Wallet Processing 不被普通公告遮挡。
110. 401 进入重新认证并保留安全页面意图。
111. 403 使用统一 Access Denied 页面。
112. 404 不泄露私有资源是否存在。
113. 409 用于可恢复业务冲突并提供下一步。
114. 429 显示安全的重试状态。
115. 503 显示受影响模块与仍可使用的模块。
116. 部分服务降级时不关闭未受影响的产品域。
117. 离线状态使用持续可见但非阻断的网络提示。
118. 读取操作使用 Skeleton 或明确 Loading，不允许永久 Spinner。
119. 写操作明确区分 Submitting、Processing、Confirmed 和异常状态。
120. 提交后锁定重复操作。
121. 网络结果不确定时先查询原业务状态。
122. 资产、奖励和游戏结算不采用未经服务端确认的乐观余额。
123. Toast 不作为资产或危险操作结果的唯一展示。
124. 用户错误页显示支持 Reference ID，但不显示 Stack Trace。
125. 表单字段错误就近展示。
126. 危险操作使用明确确认层。
127. 离开有未保存修改的表单时提示。
128. 空状态必须说明原因并提供合理下一步。
129. “没有数据”与“加载失败”严格区分。
130. 业务时间与截止时间明确显示时区。
131. 全站状态、错误和表单支持键盘与屏幕阅读器。
132. Public Home PC 使用宽屏多区域，手机按重要性单列排列。
133. Login / Registration PC 使用集中内容区，手机使用全宽单列。
134. 列表页的复杂筛选在手机使用 Sheet。
135. Tablet 继续自适应 Desktop / Mobile Pattern，不建立第三套导航。
136. Direct Play、Poker Table 和 Chaldea Operations 继续沿用其已冻结 Shell。
137. 筛选、Tab、页码和可分享状态写入 URL。
138. 从 Detail 返回时尽量保留列表筛选、页码和滚动位置。
139. IA-13 合并时执行 IA-01～IA-12 全量术语、路由、权限和 Cross-link 审计。
140. 清除旧 `/check-in`、固定游戏数量和旧 Poker History 路由等冲突表述。
141. IA-13 当时将需求基线升级为 v0.2.10；当前奖励数值修订后的上游基线为 v0.2.11。
142. IA-13 原封版将页面结构由 `v0.3 DRAFT` 升级为 `v0.3 FINAL`；本次奖励数值点修订后，当前页面结构基线为 `v0.3.1 FINAL`。
143. 同时输出独立 IA-13 Final 与修订说明。
144. 完成 v0.3 / v0.3.1 Final 后，先由用户审核，再进入 Art Direction v0.4。

# 279. v0.3.1 Final 后续阶段

v0.3.1 Information Architecture 已完成并作为正式页面结构基线；本次点版本仅修订奖励数值和迁移初始赠金 UX，不重开 IA。

下一步顺序：

```text
用户审阅 v0.3.1 FINAL
→ 如有变更，使用版本化修订
→ Art Direction v0.4
→ Page Layout / Visual Design
→ 技术设计 v0.5
→ 实施规格 v1.0
```

Art Direction v0.4 负责 Moodboard、精确色板、字体、FGO / Chaldea 主题映射、角色与素材、图标、背景、动画、游戏视觉演出和浏览器实渲染评审，不得无确认改变 v0.3 已冻结的页面职责、路由、权限、资产语义与 UX Flow。

# 280. v0.3.1 奖励数值与迁移初始赠金修订

本节记录 v0.3 FINAL 完成后的点版本修订。该修订不改变 IA-01～IA-13 已冻结的 Sitemap、路由、权限、页面职责、导航或关键 UX Flow，只冻结奖励数值以及迁移用户的开服初始资产展示。

## 280.1 已冻结奖励数量

```text
Initial Grant / 初始赠金
= 1,000 API Credit

Daily Check-in
= 500 API Credit

Hourly Reward
= 100

Relief Fund
= 300
```

其中：

- Initial Grant 和 Daily 的资产类型已经固定为 API Credit；
- Hourly 与 Relief 的数量已固定，但资产类型仍须由服务端配置返回；
- Hourly 的自然小时 / 滚动 60 分钟、累积与每日限制仍待确认；
- Relief 的累积、Active Poker 行为和临时关闭规则仍待确认。

视觉稿可以显示 100 与 300，但在资产类型确认前不得固定对应图标或单位名称。

## 280.2 新用户初始赠金

新用户成功创建账号后获得一次 1,000 API Credit 初始赠金。Completion Summary 显示真实到账状态；重复 OAuth Callback、刷新、并发注册或服务重启不得重复发放。

## 280.3 迁移用户初始赠金

迁移用户的旧 Active NewAPI Quota 仍先统一清零，Reserve、Entertainment Wallet 与 Poker In Play 先归零。迁移校验成功后，同一 Cutover 批次向每个迁移用户幂等发放 1,000 API Credit 初始赠金。

无其他账变时，首次进入的资产摘要为：

```text
API Credit       1,000
Available Chips      0
Poker In Play        0
Total Assets     1,000
```

Migration Notice 必须同时说明旧额度清零与 1,000 初始赠金已经发放，不能继续显示“从 0 开始”或“迁移用户没有开服赠金”。

# 281. v0.3.1 修订冻结结论

1. 新用户初始赠金固定为 1,000 API Credit。
2. 迁移用户在旧额度清零后获得等额 1,000 API Credit 初始赠金。
3. 新用户与迁移用户使用同一产品奖励来源，但账本触发类型和幂等业务 ID分离。
4. 每日签到固定为 500 API Credit，不再随机。
5. 每小时签到奖励固定数量为 100，不再随机。
6. 救济金固定数量为 300。
7. Hourly 与 Relief 的资产类型仍未冻结，视觉和实现不得擅自决定。
8. Hourly 时间口径、累积、每日限制，以及 Relief 累积和 Active Poker 行为继续留在需求基线。
9. 本修订不改变 Rewards Center 路由、Dashboard 职责、Wallet 资产语义或任何游戏规则。
10. `v0.3.1 FINAL` 取代 `v0.3 FINAL`，成为 Art Direction v0.4 的页面结构基线。

