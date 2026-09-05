# Chaldea Platform 需求基线文档 v0.2.11

> **历史参考归档（已脱敏）**：本文件的 FINAL / FROZEN 是历史状态，不代表当前实现或当前强制流程；现行决策优先。见[归档索引](../README.md)与[决策 0001](../../../decisions/0001-pragmatic-baseline.md)。`examples/` 路径仅为说明占位，相关部署、图片和私有文件不随仓库提供。

> 状态：需求阶段基线 / v0.3.1 Information Architecture 已完成  
> 基于：Chaldea Platform 需求基线文档 v0.2.10  
> 用途：用于后续视觉设计、架构设计、Codex / Antigravity 实现时作为统一参考  
> 原则：本文优先记录已经明确确认的需求；尚未确认的内容统一放到“待后续设计”中，避免模型擅自补充。  
> v0.2.11 变更范围：将初始赠金统一调整为 1,000 API 额度，并扩展到迁移清零后的既有用户；将每日签到调整为固定 500 API 额度；将每小时签到奖励数量固定为 100；将救济金数量固定为 300；同步修订迁移批次、Migration Notice、Dashboard / Rewards、运营后台、里程碑、待确认事项、关键结论与 Art Direction 交接。每小时奖励和救济金的最终资产类型，以及每小时奖励时间口径等仍保持待确认。

---

## 1. 项目定位

本项目不是简单给 NewAPI 换皮，而是：

**以 NewAPI 作为底层 API / 模型 / 计费能力核心，在其上开发一个拥有独立品牌、统一视觉、完整用户前台、娱乐经济系统和真人德州扑克的一体化 Chaldea Platform。**

用户侧应感知为一个完整网站，而不是“NewAPI + 外挂小游戏”。

### 1.1 产品目标

- 面向小范围朋友社区使用。
- 第一阶段预计 **10–50 人同时在线**。
- 普通用户完全看不到 NewAPI 原始用户界面。
- 保留 NewAPI 作为底层核心，并尽量减少对其核心代码和原数据库结构的侵入。
- 项目视觉表现是一级需求，不是后期装饰。
- 主题以 **FGO / 迦勒底** 为核心视觉主轴。
- PC、平板、手机均需完整可用。
- 推荐生产环境为 **8GB 内存 VPS**，同时保持在 4GB 环境中可通过合理资源限制运行。

---

## 2. 整体架构方向

采用：

**NewAPI Core + Chaldea Platform + 独立 Poker Service**

长期允许向“Chaldea 独立平台、NewAPI 仅作为模型 Gateway”方向演进，但 V1 不做这种彻底解耦。

### 2.1 NewAPI 负责

- 用户账号
- Discord OAuth
- 密码登录
- API Key
- 渠道
- 模型
- 倍率 / 计费
- API 请求
- NewAPI quota
- API 调用记录
- NewAPI 原管理员后台

### 2.2 Chaldea Platform 负责

- 全新用户前端
  - Public Home
  - Login / Discord Registration
  - Unified Access Gate / Return-to-Intent
  - 全站错误、Loading、Processing 与 Empty State
- Personal Hub / My
- Master Profile / Master Identity
- Account & Security 前台
- Master Initialization 与迁移 Migration Notice
- 奖励中心
  - 每日签到
  - 每小时签到奖励
  - 救济金
- 用户可见 API 额度换算展示
- 娱乐筹码钱包
- API 额度 ↔ 娱乐筹码兑换
- 可扩展娱乐游戏目录与 Game Registry
- V1 首发直接游玩游戏及后续新增游戏接入框架
- 通用 Game Shell、游戏状态与恢复体验
- 游戏配置与运营后台
- Provably Fair
- 跨产品域 Rankings Center
  - 资产与游戏排行榜
  - RP Usage 排行榜
- 游戏历史
- 公告与活动
  - 公告列表与详情
  - 置顶公告
  - 未登录入口公告弹窗
  - 登录后公告弹窗
  - Acknowledgements / 致谢名单
  - 首页 Banner 与 Dashboard 摘要
- 平台账变
- Chaldea Operations
  - Operations Overview / Needs Attention
  - Chaldea Models Catalog 管理
  - Users & Identity / Account Support
  - Games 与 Poker Operations
  - Economy / Reconciliation / Rewards
  - Rankings / Records / Announcements
  - Operations / Maintenance / Incidents
  - Access Control / RBAC / Audit
- 业务审计

### 2.3 Poker Service 负责

- 德州扑克大厅
- 房间 / 桌子
- 座位
- Cash Game
- 买入 / 补充买入 / 离桌
- 牌桌筹码
- 实时行动
- WebSocket
- 掉线重连
- 自动托管
- 观战
- 牌桌聊天
- 战绩与牌局历史

---

## 3. 账号与注册

### 3.1 注册规则

沿用现有 NewAPI 账号体系。

新用户不能通过普通密码直接注册，必须：

1. 使用 Discord OAuth。
2. 属于指定 Discord 服务器。
3. 拥有指定 Discord Role。
4. 验证通过后才允许首次创建站内账号。

### 3.2 注册后的登录方式

注册成功后，用户可以：

- 设置密码。
- 使用密码登录。
- 继续使用 Discord OAuth 登录。

### 3.3 Discord Role 的作用

Discord Role **仅作为首次注册门槛**。

用户注册成功后，即使后续退出 Discord 服务器或失去指定 Role，也不会自动冻结或删除站内账号，仍可继续使用网站。

### 3.4 账号唯一绑定

Discord 身份和 NewAPI 用户必须保持一对一绑定。

数据库至少保证：

- 一个 Discord User ID 只能对应一个站内账号；
- 一个 NewAPI user_id 只能对应一个 Master Profile；
- OAuth 首次注册过程必须具有数据库唯一约束，而不能只依靠前端或应用层判断。

Chaldea Platform 不保存第二份用户密码。

密码哈希、密码修改和密码验证继续完全由 NewAPI 账号系统负责。

Chaldea 仅通过稳定的 `newapi_user_id` 与用户建立关联。

### 3.5 用户公开身份

用户公开展示使用独立的 **Master Profile / Master Identity**。

V1 公开身份字段仅包括：

- Master 昵称；
- 展示头像。

V1 不加入 Bio、个性签名、生日、地区、社交链接、主页背景、公开资产开关或其他扩展 Profile 字段。

Master 昵称和展示头像可以出现在：

- Rankings；
- Recent Wins / Featured Records；
- Poker Table；
- Table Chat；
- 其他已经确认的公开游戏区域。

V1 不提供其他用户的独立公开 Master Profile 页面，也不提供隐藏昵称、匿名排行或 Private Profile 模式。公开身份和内部稳定账号 ID 必须分离。

用户只能查看和编辑自己的 Master Profile。

### 3.6 Master Profile、Account Identity 与 Authentication 的职责分离

平台必须明确区分：

```text
Master Identity
用户在 Chaldea 中公开展示的身份

Account Identity
NewAPI 中稳定的内部账号身份

Authentication
用户证明账号所有权并完成登录的方式
```

Master Profile 负责：

- Master 昵称；
- 展示头像；
- 用户自己的公开身份预览。

Account & Security 负责：

- 只读账号标识；
- Discord 绑定状态；
- 密码状态；
- 密码设置、修改与受控重置；
- 当前会话和账号状态。

Master 昵称不得同时作为密码登录标识。修改 Master 昵称或头像不得改变：

- NewAPI 登录用户名；
- Password Login Identifier；
- Discord 绑定；
- API Key；
- Wallet；
- 历史记录归属；
- 排行榜内部用户 ID。

Chaldea Platform 不保存第二份用户密码。密码哈希、密码验证与密码更新继续由 NewAPI 账号系统负责。

### 3.7 首次进入 Chaldea

无论是新注册用户还是迁移后的现有 NewAPI 用户，当其第一次进入新的 Chaldea Platform、尚未完成 Master Profile 初始化时，进入一次性的 Master Initialization。

统一顺序为：

```text
Authentication
→ Account Status Gate
→ 幂等创建 INCOMPLETE Provisional Master Profile（如需要）
→ Master Initialization（如需要）
→ Migration Notice（如需要）
→ Role / Scope 与 Resource Availability Check
→ 合法 Return-to-Intent
→ 或 Dashboard / Safe Parent
→ Deferred Post-login Popup
```

新注册用户完成 Master Initialization 后，可以先显示 **1,000 API 额度初始赠金**的真实到账状态，再进入合法 Return-to-Intent 或 Dashboard。迁移用户的等额初始赠金由 Cutover 迁移批次幂等发放，不依赖其首次登录或重新注册。

该流程属于 Chaldea Profile 初始化，不属于重新注册。不得要求现有 NewAPI 用户重新创建账号、重新绑定 Discord、重新设置原密码或重新创建 API Key。

Master Initialization 和 Migration Notice 均完成后，后续正常登录不再重复进入对应流程，除非出现新的、明确版本化的迁移告知。

### 3.8 API Key Usage Purpose

Chaldea 在 NewAPI API Key 之上增加仅用于统计归类的 **Usage Purpose / 使用用途** 元数据。

V1 至少支持：

- `General`：普通 API 使用；
- `RP`：Roleplay / 酒馆角色扮演；
- `Unclassified`：仅用于迁移后的既有 API Key 初始状态。

规则：

- 新建 API Key 时必须选择 `General` 或 `RP`；
- 一个 Key 在任一时点只能拥有一个 Usage Purpose；
- 现有 API Key 迁移后初始标记为 `Unclassified`；
- `Unclassified` Key 可以继续使用，但其请求不进入 RP 排行榜；
- 修改 Key 用途只影响修改生效后的新请求，不追溯修改历史统计；
- 每条合格请求必须保存请求发生时的 `key_purpose_snapshot`，不能在之后根据 Key 当前标签重写历史；
- Usage Purpose 只用于统计与筛选，不改变 API Key 权限、模型访问、请求路由、计费或 Secret；
- 同一 Master 的多个 RP Key 在排行榜中聚合为一个用户排名。

### 3.9 Master 昵称

Master 昵称必须满足以下规则：

- 全平台唯一；
- 长度为 1–24 个可见 Unicode Grapheme；
- 允许 Unicode 文字、数字、普通空格、`_`、`-`、`·`；
- V1 不允许 Emoji；
- 禁止换行、控制字符、零宽字符、双向文本控制字符和注入内容；
- 保存前进行 Unicode NFKC、首尾空白移除、连续空格合并和 Unicode Case Fold，用于唯一性比较；
- `Alice`、`alice`、全角 `Ａｌｉｃｅ` 等规范化结果相同的昵称视为冲突；
- 用户实际展示形式可以保留合理大小写，但唯一性使用标准化值；
- 服务端执行最终唯一性、敏感词、骚扰内容与身份冒充校验；
- 维护 Admin、Administrator、Moderator、Official、System、Support、Chaldea、NewAPI、管理员、官方、系统、客服、迦勒底及等价冒充形式的保留名称列表。

发生昵称冲突时：

- 不自动替用户保存其他名称；
- 可以提供附带短随机后缀的可用建议；
- 用户必须主动选择并确认。

用户完成首次初始化后，每次主动修改 Master 昵称进入 7 天冷却。Avatar 修改不受昵称冷却影响。

管理员可以对违规昵称执行审计化的强制改名或 `Rename Required`，但必须记录原昵称、原因、操作者和时间，不得无审计静默修改。

停用账号的昵称默认继续保留。只有管理员通过专门、可审计的名称释放流程才能重新开放该名称。

### 3.10 Display Avatar

V1 Avatar Source 仅包括：

1. System Default Avatar；
2. Discord Avatar Snapshot。

规则：

- 所有用户始终拥有可用的 System Default Avatar；
- 具体系统头像、头像框与 FGO 映射留到 Art Direction v0.4；
- V1 不开放自定义头像上传，也不显示不可用的上传占位入口；
- Discord Avatar 采用用户主动 `Sync from Discord` 时生成的快照；
- Discord 头像变化不会自动改变 Chaldea 的公开头像；
- 用户同步后需要预览并明确保存；
- Discord Avatar 不可用时回退 System Default Avatar；
- V1 所有 Avatar 使用静态表现，不播放 Discord 动态头像；
- Avatar 可以随时修改，不设置独立冷却。

未来如增加自定义上传，必须作为独立需求确认文件类型、大小、裁剪、审核、对象存储、EXIF 清理、替换、删除与违规处理。

### 3.11 身份快照与历史归属

不同页面使用不同的身份时间口径：

- Rankings 使用当前 Master Profile；
- Recent Wins / Featured Records 与其他历史业务事件保存事件发生时的 Master Display Snapshot；
- Table Chat 保存消息发送时的昵称与头像快照；
- Poker Session 在第一次成功 Buy-in / 入座时冻结 Master Snapshot；
- Poker Session 进行中修改 Master Profile，不改变当前牌桌身份；
- 完成 Cash Out 后，下一次重新入座使用新的 Master Profile。

所有记录的真实归属始终使用稳定 `newapi_user_id`，不得使用昵称文本作为业务主键。

### 3.12 Account Identity 与 Discord Connection

Account & Security 至少显示：

- 只读 Password Login Identifier；
- 只读短 Account ID；
- Discord Connection；
- Password Status；
- Current Session；
- Account Status。

Password Login Identifier 与短 Account ID 都不得进入公开 Profile、排行榜、Poker 或 Recent Wins。

V1 不允许用户从 Chaldea 修改 NewAPI 登录用户名。

Discord 规则：

- 已绑定时显示 Connected 状态与 Discord Display Name；
- Discord 数字 User ID 不在普通页面默认展示；
- V1 不提供自助 Unlink；
- V1 不提供自助 Rebind / Change Binding；
- Discord 换绑、Legacy Account 补绑定与绑定冲突修复只允许通过管理员辅助流程完成，并记录完整审计；
- 新 Discord 必须未绑定其他账号；
- 禁止覆盖、自动合并账号或自动迁移资产；
- 用户失去 Discord Role 或退出服务器，不影响已存在账号；
- Legacy Account 没有 Discord Binding 时，不强制重新注册或绑定，可以继续使用密码登录；
- Legacy Account 的补绑定不得触发新用户初始赠金。

### 3.13 Password 与账号安全

密码继续完全由 NewAPI 保存和验证。Chaldea 不保存、记录或复制密码和密码哈希。

Account & Security 只显示：

- `Password Set`；
- `Password Not Set`。

不显示密码长度、提示、哈希或任何密码片段。

没有密码的用户可以执行：

```text
Set Password
→ Fresh Authentication
→ New Password
→ Confirm Password
→ NewAPI 保存
```

已有密码的用户执行：

```text
Change Password
→ Current Password
→ New Password
→ Confirm New Password
→ NewAPI 验证并更新
```

已绑定 Discord 的用户忘记密码时，可以执行：

```text
Fresh Discord OAuth Re-authentication
→ 校验返回的 Discord User ID 与原绑定一致
→ Set New Password
→ NewAPI 更新密码
```

设置或重置密码要求最近 10 分钟内完成 Fresh Authentication。长期未活动的旧 Session 不足以执行敏感密码操作。

如果 NewAPI 没有安全、受控的密码重置接口，后续技术设计必须补充适配层；不得通过 Chaldea 直接 SQL 修改密码字段。

无 Discord 且忘记密码的 Legacy Account 使用管理员辅助恢复。

Password Policy 使用 NewAPI 的真实规则，Chaldea 前端不得建立冲突规则。密码登录失败使用通用错误，不泄露账号是否存在。

V1 不新增：

- Email Recovery；
- Phone Recovery；
- TOTP；
- Passkey / WebAuthn；
- Backup Codes；
- 登录设备管理中心。

V1 只保证 Logout Current Session。撤销其他会话只有在 NewAPI 真实支持时才可以承诺。

### 3.14 账号停用、删除与异常状态

V1 不提供普通用户自助硬删除账号。

账号限制和停用继续由 NewAPI Account Status 控制。管理员需要限制账号时使用 Disable / Suspend，不得删除资产账本、API Usage、游戏历史、Poker 记录或审计数据来模拟封禁。

账号被停用时：

- 不进入普通 Dashboard；
- 显示受限状态、联系管理员入口与 Logout；
- 不显示管理员内部备注；
- 禁止新 API、Wallet 和游戏操作。

NewAPI 暂时不可用时：

- 已有 Master Profile 可以继续读取；
- Account & Security 显示账号服务不可用；
- Password 操作禁用；
- 不伪造密码状态或成功结果。

未来如增加账号删除请求，必须独立设计资产、API Key、未完成 Poker Session、账本保留、匿名化、审计与恢复窗口。

### 3.15 Personal Hub / My

`/me` 为 Mobile-first Personal Hub，不等同于 Dashboard。

Personal Hub 聚合：

- Master Summary 与 Edit Profile；
- Master Profile；
- Account & Security；
- API Keys；
- API Usage；
- RP Rankings；
- Game History；
- Announcements；
- Chaldea Operations（仅管理员）；
- Logout。

允许显示轻量状态提示，例如：

- Unclassified Key 数量；
- Unread Announcement 数量；
- Password Not Set。

V1 不允许用户自定义或重新排序 Personal Hub。

### 3.16 Master Initialization

第一次成功认证后，服务端幂等创建：

```text
Master Profile
status = INCOMPLETE
```

Provisional Master Profile：

- 绑定稳定 `newapi_user_id`；
- 使用系统默认头像或候选 Discord Avatar Snapshot；
- 尚未作为正式公开身份出现；
- 重复登录或重复提交不得创建第二份 Profile。

Master Initialization 使用一个紧凑单页，至少包括：

- Welcome；
- Master Nickname；
- Avatar Selection；
- Public Visibility Notice；
- Live Preview；
- Complete Initialization。

候选昵称优先级：

1. Discord Display Name；
2. NewAPI Login Username；
3. `Master-<Short Account ID>`。

候选昵称仍必须通过全部唯一性与内容校验。

头像预选：

- 有可用 Discord Avatar 时默认预选其快照；
- 否则使用 System Default Avatar。

用户必须主动确认昵称和头像。Master Initialization 不允许跳过；未完成时不能进入普通 Auth 页面、游戏或个人功能。

初始化不强制设置密码。完成后可以对 Password Not Set 用户显示非阻断式安全提示。

初始化保存必须幂等。保存失败时保持 `INCOMPLETE`，下次登录恢复同一流程，不得重复创建 Profile 或初始赠金。

新用户完成时可以显示 1,000 API 额度初始赠金的真实到账状态，不得只根据前端假定奖励成功。

### 3.17 Migration Notice、流程顺序与 Logout

迁移用户在完成必要的 Master Initialization 后，展示一次性、版本化的 Migration Notice。

Migration Notice：

- 是独立迁移 Interstitial，不是普通 Announcement；
- 不提供右上角直接关闭；
- 使用 `我已了解，继续` 完成确认；
- 服务端保存用户、迁移版本与确认时间；
- 每个迁移版本只需确认一次；
- 未确认时下次登录继续展示。

至少说明：

- 旧 API 额度已按开服公平规则清零；
- 清零后已通过迁移批次发放 **1,000 API 额度初始赠金**，使迁移用户与新用户采用同一开服起点；
- 该笔资产属于迁移触发的初始赠金，不表示重新注册，也不得重复触发新用户注册回调；
- 账号、Discord、密码和 API Key 没有删除；
- 历史 API Usage 保留；
- 现有 Key 初始为 `Unclassified`；
- Wallet、Rewards、API Keys 与 API Usage 的后续入口。

认证后顺序：

```text
Account Status Gate
→ Master Initialization（如需要）
→ Migration Notice（如需要）
→ Permission / Resource Availability Check
→ 合法 Return-to-Intent
→ 或 Dashboard / Safe Parent
→ Deferred Post-login Popup（安全页面）
```

Return-to-Intent 只允许站内安全路径，使用后清除，不自动重放 Profile、Password、Wallet、API Key 或游戏操作。

存在 Active Poker Session 时，用户从普通页面 Logout 必须看到明确警告：Logout 不会执行 Safe Leave / Cash Out，行动计时和自动操作仍可能继续。提供 `Return to Poker Table` 与 `Logout Anyway`。

Logout 不取消 Wallet Exchange、Reward Claim、已接受 Game Round 或 API 的服务端处理。用户重新登录后查看最终状态。

### 3.18 Login、Discord Registration 与 Password Login Identifier

`/login` 仅用于已经存在的账号登录。Discord Login 与 Password Login 在同一页面同时可见，Discord 作为主要方式置于密码表单之前。

密码表单使用稳定的 **Password Login Identifier**。Master 昵称和短 Account ID 均不得作为普通密码登录标识。

Discord OAuth 创建的 NewAPI 账号必须拥有稳定、唯一、后续可用于密码登录的 Password Login Identifier。该标识必须在 Account & Security 与 Set Password 流程中明确展示。

如果技术核查发现 NewAPI 当前 OAuth 注册无法提供稳定 Identifier，技术设计必须增加受控适配或独立 Account Identifier 确认步骤；不得由前端猜测，也不得使用可修改的 Master 昵称替代。

`/register` 是 Discord 首次注册资格说明和 OAuth 启动页面，不是传统注册表单。页面不得提供普通用户名、密码、Email、Phone、Master 昵称或 Avatar 注册字段。

注册页显示后台当前配置的 Required Server 与 Required Role，使用人类可读名称；存在有效邀请链接时可以提供 Join Server。

已经绑定现有账号的 Discord 再次进入 Registration 时：

- 转为登录现有账号；
- 不创建第二个账号；
- 不重复发放新用户初始赠金；
- 异常 Binding Conflict 进入 Account Support；
- 不自动合并账号或迁移资产。

OAuth Callback、资格验证、账号创建、新用户初始赠金和 Provisional Master Profile 必须幂等。重复标签页、重复 Callback、刷新、超时或服务重启不得产生重复账号、奖励或 Profile。

服务器成员关系或 Role 验证失败时，不创建账号和新用户初始赠金。Discord 验证暂时不可用时必须显示临时故障，不得错误解释为用户缺少 Role。

账号已经创建但 1,000 API 额度新用户初始赠金仍在处理中时，不删除账号或重新注册；继续 Master Initialization，并在 Completion Summary 显示真实 Account、Profile 与 Reward 状态。

---

## 4. 用户前台原则

普通用户完全不接触 NewAPI 原始用户 UI。

全站统一在新的 Chaldea 前台中完成。

### 4.1 用户前台计划包含

- 首页 / Public Home
- Login / Discord Registration
- Unified Access Gate、Return-to-Intent 与全站状态
- Dashboard / 控制台总览
- 模型广场
- API Key 管理
- API 使用记录
- 钱包
- 奖励中心
  - 每日签到
  - 每小时签到奖励
  - 救济金
- 娱乐中心
- 游戏目录 / Game Catalog
- 实际游戏入口与大厅型游戏入口
- Rankings Center
  - 资产与游戏榜
  - RP Usage 榜
- 游戏历史
- 公告 / 活动
  - 置顶公告
  - 入口公告弹窗
  - Acknowledgements / 致谢名单
- Personal Hub / My
- Master 资料
  - Master 昵称
  - System / Discord Avatar
- Account & Security / 账号与安全
  - Discord Connection
  - Password Set / Change / Discord Reset
  - Account Status
- Master Initialization
- Migration Notice（迁移用户条件式 Interstitial）

### 4.2 不做完整开发者教程站

朋友本身有 AI/API 使用经验，因此 API 页面只提供基础信息：

- Base URL
- API Key
- 常用 Endpoint
- 模型 ID
- 简单 cURL 示例
- 一键复制

第一阶段不做大型文档中心、逐项客户端教程或在线 Playground。

---

## 5. 公开访问、Public Home 与认证入口

平台继续采用 **半公开式**。

### 5.1 未登录可访问

- Public Home；
- Model Square 与 Model Detail；
- 模型公开价格与状态；
- 平台介绍；
- Announcements 与公开 Announcement Detail；
- Entertainment Hub；
- Game Catalog 与已公开游戏介绍、分类和运行状态；
- Rankings Center 的公开聚合信息；
- Login；
- Discord Registration。

公开浏览不代表公开注册。新用户仍必须通过 Discord OAuth、指定 Server 与指定 Role 完成首次注册。

### 5.2 登录后才能访问

- Dashboard；
- API Keys、API Usage、API Access 中的个人能力；
- Wallet 与 Rewards Center；
- 实际 Direct Play Game；
- Poker Lobby / Table；
- Personal Hub / My；
- Master Profile；
- Account & Security；
- Master Initialization（首次进入时）；
- Migration Notice（符合迁移条件时）；
- 个人完整 Game History；
- Chaldea Operations（按 Role / Scope）。

### 5.3 Public Home

`/` 始终为 Public Home。登录用户访问 `/` 不强制重定向至 Dashboard，而是在同一公共首页上获得登录态增强。

Public Home 采用品牌与产品入口混合型结构，至少承担：

- Hero / Platform Identity；
- 真实 Platform Status；
- Models & API 与 Entertainment 两条主路径；
- Featured Models；
- Featured Games 与条件式 Poker Spotlight；
- Assets & Games / RP Usage Rankings Preview；
- Recent Public Wins；
- Announcements & Events 摘要；
- Acknowledgements 规范公告入口；
- Footer。

Public Home 不复制 Dashboard 的个人资产、签到、API Usage、Active Game Round 或 Active Poker Session。

Featured Models、Featured Games、Poker、Rankings、Recent Wins 与 Announcements 必须使用真实运营配置和真实数据。没有内容时隐藏模块，不生成虚构热门度、在线人数、中奖、模型或服务状态。

Recent Public Wins 使用事件发生时的 Master Identity Snapshot，不显示用户当前总资产。

Platform Status 只显示 Models / API、Entertainment 与 Poker 的真实 Operational / Degraded / Maintenance / Unavailable 状态和更新时间。V1 不新增独立 `/status` 页面。

Public Home 不使用自动轮播 Banner，不强制自动播放视频或音频。V1 同一时点最多显示一条主要 Home Banner。

Footer 只聚合已存在路由。V1 不因 IA-13 自动新增 Terms、Privacy、Cookie、Sponsor Payment 或 Public Status 页面。

### 5.4 Entry Popup、Critical Notice 与 Home Banner

未登录用户进入 `/` 或 `/login` 时可以检查当前有效 Entry Popup。普通 Entry Popup 不在 `/register` 强制弹出，也不强制覆盖具有明确访问意图的公共 Deep Link。

Entry Popup 与 Public Home 正文、Critical Notice、Home Banner、Post-login Popup 和 Dashboard Summary 为不同展示层。

- Critical Notice：用于真实严重故障、维护或安全事件；
- Entry Popup：按照公告展示版本规则在入口提示；
- Home Banner：用于普通公共活动或公告，V1 不自动轮播。

Entry Popup 加载失败不得阻止 Public Home、Login 或 Discord Registration。

### 5.5 Login

`/login` 只用于已有账号登录，同时显示：

- Continue with Discord；
- Password Login Identifier；
- Password；
- Forgot Password；
- Discord Registration 入口；
- 安全的 Return-to-Intent 摘要；
- 当前服务与错误状态。

Password Login 支持密码管理器、正常自动填充与 Show / Hide。登录失败保留 Identifier、清空 Password，并使用通用凭证错误避免账号枚举。V1 不新增 Chaldea Remember Me。

已绑定 Discord 用户忘记密码时使用 Fresh Discord Re-authentication；无 Discord 的 Legacy Account 进入 Account Support。

已注册用户使用 Discord Login 时不重新检查首次注册 Role。Discord Provider 故障必须与账号不存在区分。

已登录用户访问 `/login` 时不显示登录表单，直接进入认证后 Gate。

### 5.6 Discord Registration

`/register` 显示：

- 注册说明；
- Required Server；
- Required Role；
- 可选 Join Server；
- Continue with Discord；
- Existing User Login；
- 注册状态和错误。

页面不收集普通用户名、密码、Email、Phone、Master Nickname 或 Avatar。

服务端验证 OAuth State、Discord 身份、服务器成员关系、指定 Role 与现有绑定。用户取消 OAuth 后安全返回 Registration。

已绑定现有账号时转为登录，不创建第二账号或重复奖励。异常 Binding Conflict 进入支持流程。

### 5.7 Unified Access Gate

全站访问顺序：

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

受限账号不进入 Master Initialization 或普通 Dashboard。

Return-to-Intent 只允许安全站内路径，具有有效期，恢复前重新校验权限与资源状态，使用后清除。它只恢复页面和安全筛选，不自动执行下注、Buy-in、Cash Out、兑换、Reward Claim、API Key 变更、Profile / Password 保存或管理员写操作。

目标下线、维护或无权限时，显示原因和安全替代入口，不静默跳转到无关页面。

### 5.8 全站状态、错误与空状态

全站统一：

- 401：重新认证并保留安全 Return-to-Intent；
- 403：Access Denied；
- 404：不泄露私有资源存在性；
- 409：可恢复业务冲突；
- 429：安全重试状态；
- 503：说明受影响模块与仍可用模块。

部分服务降级时，不关闭未受影响的产品域。离线使用持续可见但非阻断提示。

读取操作使用 Skeleton 或明确 Loading，不允许永久 Spinner。写操作区分 Submitting、Processing、Confirmed 与异常状态，并锁定重复提交。

网络结果不确定时，先查询原 Business ID；资产、奖励与游戏结算不使用未经服务端确认的乐观余额。

Toast 不能作为资产或危险操作结果的唯一展示。用户错误页显示安全 Reference ID，不显示 Stack Trace。

空状态必须说明原因和下一步；“没有数据”与“加载失败”必须分离。所有表单、状态和错误需要支持键盘、明确 Focus、Label、文字错误与 Screen Reader。业务截止时间显示时区。

---

## 6. 模型广场

采用：**NewAPI 自动同步 + Chaldea 元数据增强**。

### 6.1 从 NewAPI 自动同步

- 模型 ID
- 可用状态
- 倍率 / 价格
- 渠道可用性等真实数据

### 6.2 Chaldea 平台额外维护

- 展示名称
- Logo / 图片
- 简介
- 标签
- 推荐用途
- 上下文长度
- 排序
- 是否推荐
- 是否在公开模型广场展示
- FGO 风格副标题

NewAPI 新增模型后，可先进入“待完善元数据”状态，完善后再公开。

---

## 7. 额度与钱包体系

### 7.1 资产层

用户侧存在两种可见资产：

1. **API 额度**
2. **娱乐筹码**

二者 **1:1 双向兑换**。

### 7.2 API 额度

API 额度是用户可见的统一计价单位。

系统内部采用：

- **1 API 额度 = 1 USD 标准模型计价单位**
- **1 API 额度 = `QuotaPerUnit` 个 NewAPI raw quota**
- 当前目标配置：**`QuotaPerUnit = 500,000`**
- 因此当前：
  - `1 API 额度 = 500,000 raw quota`
  - `1 raw quota = 0.000002 API 额度`

NewAPI 当前 raw quota 仍负责实际 API 请求的预扣费、结算和调用消费，但不再承担用户全部 API 资产的无限容量存储。

用户的完整 API 资产由两部分组成：

1. **Active API Quota**
   - 位于 NewAPI。
   - 用于实际 API 请求消费。
   - 使用 NewAPI 原生整数 quota。

2. **Reserve API Credit**
   - 位于 Chaldea Platform。
   - 用于保存超出 NewAPI Active Quota 范围的 API 额度。
   - 以 API 额度形式发放的初始赠金、每日签到、每小时签到奖励、救济金、管理员发放，以及娱乐筹码兑换回 API 额度，原则上先进入 Reserve。
   - 新用户与迁移用户的 1,000 初始赠金均为 API 额度，并进入 Reserve。
   - 如果每小时签到奖励或救济金最终配置为娱乐筹码，则其固定数量分别为 100 与 300，并直接进入娱乐钱包，不经过 Reserve。

用户前端展示的：

`API 总额度 = Reserve API Credit + Active NewAPI Quota ÷ QuotaPerUnit`

对普通用户隐藏 Active / Reserve 的技术区别，界面只展示一个统一的“API 额度”。

现有 NewAPI 用户迁移时：

- 现有账号、Discord 绑定、密码、API Key、渠道、模型、倍率和历史调用记录继续保留；
- 在正式切换维护窗口内，将迁移范围内现有用户的 `Active NewAPI Quota` 统一设置为 `0`；
- 在发放初始赠金前，将 `Reserve API Credit`、娱乐钱包与 Poker In Play 初始化为 `0`；
- 清零校验通过后，由同一 Cutover 迁移流程向每个迁移用户幂等发放 **1,000 API 额度初始赠金**；
- 迁移初始赠金进入 Reserve API Credit，不依赖用户首次登录，也不触发新用户注册回调；
- 发放完成且不存在其他账变时，迁移用户前端显示 `API Credit = 1,000`、`Available Chips = 0`、`Poker In Play = 0`、`Total Assets = 1,000`；
- API Key 继续有效，并可以在可用 API Credit 范围内继续完成付费模型调用；
- 现有 API Key 的 Usage Purpose 初始为 `Unclassified`，用户后续可以改为 `General` 或 `RP`；
- 迁移清零与初始赠金发放属于同一版本化开服迁移政策，不得通过普通管理员余额编辑逐个执行。

### 7.2.1 Quota 精度与整数规则

NewAPI raw quota 必须始终保持整数。

禁止在数据库或跨服务账务中使用 `float32` / `float64` 表示：

- raw quota
- API 资产原子单位
- 娱乐筹码原子单位
- 钱包余额
- 下注金额
- 派奖金额
- 转账金额

所有资产采用整数原子单位存储。

当前 `QuotaPerUnit = 500,000` 时：

`1 API额度 = 500,000 atomic units`

并定义：

`1 NewAPI raw quota = 1 Chaldea credit atomic unit`

因此：

`API额度 = atomic_units / 500,000`

例如：

- `1 API额度 = 500000 units`
- `0.1 API额度 = 50000 units`
- `0.0372 API额度 = 18600 units`
- `0.000002 API额度 = 1 unit`

所有“API额度 → raw quota”的转换必须使用 Decimal / 整数字符串解析后进行整数运算，禁止执行类似：

`int(float64(amount) * 500000)`

的直接浮点转换。

无法精确表示为 atomic unit 的输入必须由后端按照统一规则拒绝或规范化，不允许不同服务自行采用不同舍入规则。

`QuotaPerUnit` 是全平台关键账务参数，不允许在生产环境中随意修改。

如果未来需要修改 `QuotaPerUnit`：

1. 平台进入维护模式；
2. 停止钱包兑换和游戏结算；
3. 创建完整数据库备份；
4. 执行余额迁移；
5. 完成账务一致性校验；
6. 再恢复服务。

不得在平台运行期间直接改变该参数。

### 7.3 美元关系

- UI 不突出美元。
- 内部计价采用：**1 美元标准模型价格 = 1 API 额度**。
- 例如一次请求标准价格为 `$0.0372`，则扣除 `0.0372 API 额度`。
- 用户只看到“API 额度”，不显示 `$` 符号。
- 底层内部使用整数最小单位保存，禁止用浮点数直接记账。

### 7.4 娱乐筹码

- 娱乐筹码存储在 Chaldea Platform 数据库。
- API 额度与娱乐筹码 1:1 双向兑换。
- 无手续费。
- 不允许用户之间转账。
- 不允许赠送筹码。
- 不建立用户间筹码交易市场。

娱乐筹码同样不得使用浮点数作为数据库余额。

Chaldea 内部使用：

`chip_units BIGINT`

作为娱乐筹码原子余额。

当前：

`1 娱乐筹码 = 1 API额度 = 500,000 chip_units`

因此 API 额度和娱乐筹码之间可以进行无损 1:1 整数转换。

业务层可以限制老虎机、骰子、德州扑克等游戏只接受整数筹码或指定下注步长，但存储层始终使用统一原子单位。

所有资产相关 API 中，大整数原子值不得直接作为 JavaScript JSON Number 传输。

后端应优先使用：

- 十进制字符串
- 或明确的展示额度字符串

避免超过 JavaScript `Number` 安全整数范围后产生精度丢失。

### 7.5 兑换安全

每一笔兑换必须有：transfer_id、biz_id、direction、amount、status、created_at、confirmed_at。

必须实现：

- 幂等
- 防重复提交
- 防重复到账
- 失败补偿
- 未完成交易对账 / 修复能力

### 7.6 跨数据库账务一致性

由于 `newapi` 与 `chaldea_platform` 为两个独立 PostgreSQL Database，系统不得假定一次普通数据库事务能够同时覆盖两个数据库。

任何涉及 NewAPI quota 与 Chaldea 资产之间的跨库操作必须采用：

**Idempotent Saga / 状态机 + 对账补偿机制**

典型状态：

`PENDING`
→ `SOURCE_DEBITED`
→ `TARGET_CREDITED`
→ `CONFIRMED`

异常情况下可以进入：

`COMPENSATING`
→ `COMPENSATED`

每个操作必须拥有全局唯一 `biz_id / transfer_id`。

无论发生：

- HTTP 超时
- 服务崩溃
- Docker 重启
- 数据库短暂不可用
- 客户端重复提交

同一个 `biz_id` 都只能产生一次最终资产变化。

发生网络超时时，不得直接认为操作失败并重新执行，应首先查询原交易状态。

必须提供后台 Reconciliation Worker，定期扫描：

- 长时间 PENDING
- SOURCE_DEBITED 但未 TARGET_CREDITED
- TARGET_CREDITED 但未 CONFIRMED
- 补偿失败

并自动恢复或标记管理员处理。

### 7.7 钱包与账本不变量

所有涉及资产的操作必须遵守以下规则：

1. 钱包余额与对应 ledger 写入必须在同一数据库事务中完成。
2. 不允许先修改余额、后异步补写 ledger。
3. `wallet_ledger` 为 append-only 账本。
4. 已存在账变记录不得 UPDATE 或 DELETE。
5. 修正错误余额必须新增一笔反向 / 调整账变。
6. 每笔账变必须记录：
   - user_id
   - biz_type
   - biz_id
   - delta
   - balance_before
   - balance_after
   - created_at
   - metadata
7. `(biz_type, biz_id)` 或等价字段必须建立唯一约束。
8. 扣款必须使用数据库行锁或原子条件更新，禁止“先查询余额、再普通 UPDATE”。
9. 玩家余额不得因并发下注变成负数。
10. 管理后台不得提供直接修改 wallet.balance 的能力，必须通过统一账务服务生成调整流水。

---

## 8. 非游戏性奖励发行与周期补给

本节所称“非游戏性奖励发行”，指平台通过初始赠金、周期奖励、救济或管理员发放，向用户新增发放可以计入其平台总资产的 API 额度或娱乐筹码。

API 额度与娱乐筹码之间的 1:1 双向兑换属于用户既有资产在不同资产形态之间的转换，不属于新增发行。

平台当前的非游戏性奖励来源仅包括：

1. 初始赠金
2. 每日签到
3. 每小时签到奖励
4. 救济金
5. 管理员发放

其中“初始赠金”是统一的产品奖励来源，包含两种互斥触发场景：

- 新用户通过 Discord 注册成功后的新用户初始赠金；
- 既有用户在 Cutover 清零完成后的迁移初始赠金。

两种场景金额一致，但必须使用不同的业务来源和幂等键，不得把迁移用户伪装成重新注册。

单人游戏中奖派奖属于游戏结算产生的娱乐筹码发行，按照第 9 章“平台无限庄家”和对应游戏结算规则单独记录与统计，不属于本节所称的非游戏性奖励来源。

不做充值商城。

不做普通用户购买额度。

由于娱乐筹码可以按照 1:1 兑换为 API 额度，因此通过每小时签到奖励或救济金新增发放的娱乐筹码，也必须计入平台非游戏性奖励发行统计。

所有奖励发放必须：

- 使用服务端权威时间；
- 使用服务端生成或确认的奖励数值；
- 保留完整领取记录；
- 生成唯一业务 ID；
- 具备幂等保护；
- 防止重复领取；
- 防止多标签页、多设备或网络重试导致重复发放；
- 将余额修改与账变写入放在同一数据库事务中；
- 通过正式 append-only ledger 记录资产变化。

### 8.1 初始赠金

V1 当前统一金额为：

**1,000 API 额度**

适用对象：

1. 新用户：首次通过 Discord 注册资格验证并成功创建站内账号后发放一次；
2. 迁移用户：旧 Active NewAPI Quota 清零并完成迁移校验后，通过 Cutover 批次发放一次。

规则：

- 新用户与迁移用户金额完全相同；
- 奖励资产类型固定为 API 额度；
- 奖励进入 Reserve API Credit；
- 不采用随机金额；
- 不允许按用户单独覆盖金额；
- V1 金额从 10,000 调整为 1,000；
- 未来如修改金额，必须形成新的版本化奖励政策和完整审计，不得静默改变历史记录。

建议业务 ID：

```text
新用户：initial_grant:registration:{user_id}
迁移用户：initial_grant:migration:{migration_batch_id}:{user_id}
```

同一用户在同一适用场景只能成功获得一次。OAuth Callback、页面刷新、重复登录、迁移脚本重跑或服务重启不得重复发放。

### 8.2 每日签到

每日签到奖励固定为：

**500 API 额度**

规则：

- 每个 `Asia/Shanghai` 自然日仅能成功领取一次；
- 每次成功领取固定获得 500 API 额度；
- 奖励进入 API 额度；
- 不采用 1,000–10,000 随机范围；
- 不使用随机数决定签到金额；
- 不允许普通运营人员为特定用户覆盖金额；
- 保留完整签到记录；
- 未来如修改金额，必须使用新的版本化奖励配置，并且只影响生效后的新领取。

数据库必须对：

`(user_id, checkin_date)`

建立唯一约束或采用等价数据库级防重复机制。

即使用户从 Dashboard、Rewards Center、多个标签页或多个设备同时提交签到请求，最终也只能产生：

- 一条每日签到记录；
- 一笔 500 API 额度奖励。

### 8.3 每小时签到奖励

每小时签到奖励的固定数量为：

**100**

当前已经确认：

- 每个用户每小时最多成功领取一次；
- 每次成功领取的数量固定为 100，不采用随机金额；
- 每次成功领取产生一笔新增平台资产；
- 必须保留完整领取记录；
- 必须写入正式资产账变；
- 必须具备幂等与并发防重复保护；
- 客户端时间不得作为是否可领取的判断依据；
- 未来如修改 100，必须通过新的版本化奖励政策，不得改写历史领取。

以下内容仍需后续确认，不得由开发模型或视觉设计模型自行决定：

- 100 发放为 API 额度还是娱乐筹码；
- “每小时”的具体口径：
  - 按 `Asia/Shanghai` 自然小时；
  - 或以上一次成功领取时间为起点的 60 分钟滚动冷却；
- 未及时领取的奖励是否累积；
- 是否设置每日领取次数或总量限制。

在资产类型确认前，页面可以显示固定数字 100，但必须由服务端返回实际资产类型和单位标签；不得在视觉稿或实现中擅自固定为 API Credit 或 Entertainment Chips。

如果采用自然小时口径，必须使用唯一领取周期标识，防止同一自然小时重复领取。

如果采用滚动冷却，必须由服务端原子更新并校验下一次可领取时间。

### 8.4 救济金

平台增加救济金机制，用于改善玩家进入破产状态后无法重新开始游戏的体验。

单次救济金固定数量为：

**300**

300 不采用随机金额。未来如修改该数量，必须形成新的版本化救济政策，不得改写历史领取。

#### 8.4.1 破产定义

用户满足以下条件时，视为处于平台破产状态：

`平台总资产 < 10`

资格判断采用第 17.1 节定义的统一总资产口径，至少包括：

- API 总额度；
- 娱乐钱包中的可用娱乐筹码；
- Poker In Play，包括 `table_stack` 以及当前 Hand 中已经投入 Pot、但尚未被其他玩家赢走或完成其他合法资产转移的资产；
- 跨库兑换处理中、尚未计入任一已结算目标余额且未与源余额重复计算的资产。

只有严格低于 10 才属于破产：

- 总资产为 `9.999998` 时符合破产条件；
- 总资产恰好为 `10` 时不符合破产条件；
- 总资产大于 `10` 时不符合破产条件。

在当前 `1 API 额度 = 1 娱乐筹码 = 500,000 atomic units` 的关系下，服务端可以使用以下等价原子单位条件判断：

`total_asset_atomic_units < 5,000,000`

破产资格必须由服务端基于权威资产快照计算。客户端不得只检查 Available Chips，也不得通过把资产兑换成 API 额度、转入 Poker、停留在 Pot 或制造处理中状态来错误获得救济资格。

#### 8.4.2 领取条件

用户领取救济金时，必须同时满足：

1. 当前平台总资产严格少于 10；
2. 当前没有尚未完成、会阻止新领取的同类救济金请求；
3. 从上一次成功领取救济金起已经经过至少 4 小时，或用户从未成功领取过救济金；
4. 救济金功能当前已启用且不处于维护状态。

如果用户从未成功领取过救济金，只要进入破产状态且功能可用，即可进行首次领取。

冷却已经结束但用户总资产仍为 10 或以上时，仍不得领取；当其之后再次进入总资产少于 10 的破产状态时，如果冷却已经结束，则可以立即领取。

救济金不会自动发放，用户必须在 Dashboard 或 Rewards Center 主动发起领取。

#### 8.4.3 滚动 4 小时冷却

救济金采用领取成功后开始计算的滚动冷却：

`next_claim_at = last_successful_claim_at + 4 hours`

例如用户在 09:37 成功领取，则下一次最早可领取时间为 13:37，而不是等待 12:00、16:00 等固定时间窗口。

明确不采用：

- 00:00 / 04:00 / 08:00 / 12:00 / 16:00 / 20:00 等固定四小时刷新点；
- 根据客户端本地时间判断冷却；
- 因页面刷新、重新登录或多设备切换而重置冷却。

只有已经成功发放资产的领取才开始或重置 4 小时冷却。以下情况不得开始或重置冷却：

- 不符合破产资格；
- 请求校验失败；
- 请求被拒绝；
- 未产生正式奖励记录；
- 未完成资产发放且最终被回滚或补偿为未领取。

服务端必须记录 `last_successful_claim_at` 与 `next_claim_at`，并以服务端时间进行判断。多标签页、多设备或并发请求最终只能成功领取一次，并且只能产生一个新的冷却起点。

#### 8.4.4 仍待后续确认

以下内容仍需后续确认，不得由开发模型或视觉设计模型自行决定：

- 300 发放为 API 额度还是娱乐筹码；
- 未领取的救济资格是否可以累积为多次；
- 用户存在 Active Poker Session 时能否领取；
- 是否允许管理员临时关闭救济金功能；
- 是否允许通过新的版本化政策临时调整已确认的 300 金额。

在资产类型确认前，页面可以显示固定数字 300，但必须由服务端返回实际资产类型和单位标签。

已经确认的单次数量 300、破产阈值和滚动冷却不得作为普通运营参数任意修改。如未来确需改变，必须进行新的需求确认并形成新的规则版本。

### 8.5 管理员发放

管理员可以手动增加或减少用户资产。

每笔管理员调整必须记录：

- 操作者；
- 用户；
- 资产类型；
- 变化数量；
- 原因；
- 修改前余额；
- 修改后余额；
- 时间；
- 唯一业务 ID。

管理员不得直接修改最终余额字段，必须通过统一账务服务生成正式账变。

---

## 9. 娱乐经济总体规则

### 9.1 完全自由经济

不设置：

- 每日最大盈利
- 每日最大兑换
- 人为提现上限
- 盈利冻结审核门槛

正常玩家赢多少都可以兑换回 API 额度。

但必须保留纯技术性安全机制：

- 幂等
- 防重复结算
- 事务
- 整数溢出保护
- 异常请求检测
- 完整账变
- Round ID 追踪

### 9.2 平台无限庄家

单人赌场游戏不维护有限庄家资金池。

玩家输：筹码回收 / 销毁。  
玩家赢：系统直接发行中奖筹码。

不出现“庄家余额不足无法赔付”。所有增发与销毁必须写入账变流水，并关联 round_id。

### 9.3 服务端权威原则

所有游戏均采用 Server Authoritative 模型。

客户端只能发送：

- 游戏类型
- 下注金额
- 玩家操作
- Client Seed
- 必要交互输入

客户端不得提交或决定：

- 开奖结果
- 随机数
- 派奖金额
- 输赢状态
- 钱包余额
- 游戏结算结果

所有下注验证、随机结果、赔率读取、派奖计算和最终结算必须在服务端完成。

一局游戏至少采用明确状态机：

`CREATED`
→ `BET_ACCEPTED`
→ `RESOLVING`
→ `SETTLED`

异常时：

`CANCELLED / REFUNDED`

`SETTLED` 游戏不得再次结算。

下注扣款与 Round 创建必须处于同一事务中。

派奖必须通过唯一 round_id 幂等执行。

---

## 10. V1 娱乐功能与可扩展游戏边界

### 10.1 V1 首发范围

第一版明确包含：

1. 老虎机 / Slot Machine
2. 骰子猜大小
3. 21 点 Blackjack
4. 刮刮乐
5. 扭蛋机
6. 德州扑克
7. 排行榜
8. 开奖记录 / 游戏历史

以上清单仅定义 V1 首发与验收范围，不构成 Chaldea Platform 的永久游戏数量上限。

### 10.2 可扩展游戏目录

平台必须提供独立、可公开浏览的 Game Catalog，用于承载全部已发布游戏。

Entertainment Hub 负责运营推荐、活动、Continue Playing 和重点入口；Game Catalog 负责完整游戏发现、搜索、分类、筛选和状态展示。

游戏目录、导航、历史、排行榜关联和运营后台不得在前端或后台导航中写死为固定五款或固定数量的游戏。

### 10.3 稳定游戏标识

每款接入平台的游戏必须拥有稳定的 `game_slug` 或等价稳定标识。

展示名称、主题包装和运营文案可以调整，但不得因为展示名称变化而导致：

- Deep Link 失效；
- 历史记录无法关联；
- Round / Session 数据迁移；
- 排行榜与配置关联变化。

直接游玩游戏统一使用类似：

`/games/:game_slug`

的逻辑入口。

### 10.4 游戏进入类型

Game Catalog 必须能够表达不同进入类型，而不是假定所有游戏都直接打开单人页面。

至少支持以下信息语义：

- Direct Play：直接进入游戏；
- Lobby：进入大厅、房间或桌子列表；
- Resume：恢复已有活动 Session；
- Maintenance：当前不可开始新局；
- Coming Soon：公开预告但暂不可玩。

Poker 在 V1 中使用独立 Lobby / Table 结构。未来麻将、斗地主等多人游戏可以拥有自己的 Lobby 或实时服务，而不需要套用单人游戏页面。

### 10.5 发布状态与运行状态

游戏平台必须区分：

1. 发布状态：决定是否对普通用户可见；
2. 运行状态：决定已发布游戏当前能否进入或开始新局。

尚未发布或元数据未完善的游戏不进入公开目录。

已发布但维护中的游戏继续保留介绍与历史入口，但禁止开始新 Round。

已经退役但存在历史数据的游戏不得删除其历史名称、配置版本、Round、Session 或公平验证数据。

### 10.6 通用 Game Shell 与能力适配

直接游玩游戏复用通用的信息职责，包括：

- Game Header 与返回入口；
- 资产摘要；
- 下注与操作区域；
- 当前 Round 状态；
- 结果与净变化；
- Rules 与 Transparency；
- Provably Fair；
- 个人历史入口；
- 维护、余额不足、处理中和恢复状态。

通用 Game Shell 不要求所有游戏使用相同视觉布局。

平台至少需要兼容以下交互能力：

- Instant Resolve：单次操作后结算；
- Reveal Sequence：服务端结果已确定，前端分阶段揭示；
- Multi-action Round：一局中需要继续提交玩家操作。

未来新增游戏按照自身规则选择所需能力组合，不得通过复制旧页面并写死旧游戏逻辑的方式接入。

### 10.7 新游戏接入边界

Game Registry 和运营后台负责游戏元数据、排序、推荐、发布、维护、透明度与配置管理。

但完整新游戏仍然必须经过：

1. 游戏规则与服务端逻辑实现；
2. Server Authoritative 结算；
3. 钱包与 Round / Session 集成；
4. 随机游戏的 Provably Fair 集成；
5. 前端交互实现；
6. 测试与审核；
7. 注册到 Game Registry；
8. 正式发布。

不得将“可扩展游戏目录”理解为管理员只填写表单即可无代码生成一款完整可玩的游戏。

### 10.8 Round 恢复与维护原则

如果服务端已经接受下注或创建有效 Round：

- 页面刷新、断线或重新进入必须恢复同一 Round；
- 不得因为客户端未收到响应而创建重复 Round；
- 已结算时返回同一结算结果；
- 已退款时返回正式退款状态。

游戏进入维护状态主要阻止新 Round。

已经接受下注的 Round 必须根据游戏能力正常完成、恢复、托管或正式退款，不得被维护切换直接遗弃。

### 10.9 Direct Play 全局统一下注规则

本节适用于所有使用娱乐主钱包、由玩家在开始一局前选择基础下注金额的 **Direct Play 游戏**。

V1 适用实例包括：

- Slot Machine；
- Dice；
- Blackjack 的初始下注；
- Scratch Card；
- Summon / Gacha 中由玩家选择的基础下注金额。

未来新增的同类 Direct Play 游戏默认复用本规则，除非后续需求明确批准新的全局策略或将该游戏定义为不同资金模型。

本规则不直接适用于：

- Poker；
- 使用 Buy-in、Blind、Ante、Base Score、Raise、Table Stack 或其他牌桌资金规则的大厅型多人游戏；
- 不允许玩家自由输入单局基础下注金额的特殊游戏模式。

Direct Play 游戏统一采用：

- 最低下注：**10 娱乐筹码**；
- 产品层固定最高下注：**不设置**；
- 快捷金额：**10 / 100 / 500 / 1000**。

“不设置产品层固定最高下注”表示平台不人为规定 1,000、10,000 或其他统一封顶金额，但实际提交金额仍不得超过：

- 用户当前可用娱乐筹码；
- 当前游戏操作允许扣除的余额；
- 资产原子单位、数据库字段与服务端整数溢出保护所允许的安全范围。

四个快捷金额仅用于选择或填入下注金额。点击快捷金额不得直接扣款、开始游戏或创建 Round；正式下注仍由游戏的主操作触发，并由服务端完成余额校验、扣款与 Round 创建。

单个 Direct Play 游戏不得自行覆盖为不同的最低下注，也不得自行增加固定最高下注。Blackjack 的 Double / Split 等由游戏操作产生的追加下注，以及未来游戏的特殊追加下注规则，在对应游戏规格中单独定义，但其初始基础下注仍遵守本节。

当 Available Chips 少于最低下注 10 时，不得创建有效付费 Round。此时页面应根据服务端返回的总资产与救济状态区分处理：

- 平台总资产严格少于 10：用户处于破产状态；在滚动 4 小时冷却已经结束时，可以进入 Rewards Center 领取救济金；
- Available Chips 少于 10、但平台总资产为 10 或以上：用户不属于破产，应优先通过 Wallet 将 API 额度兑换为娱乐筹码，或安全处理仍在 Poker In Play 中的资产；不得因此领取救济金；
- 从 Wallet 或 Rewards 返回游戏时可以恢复未提交的金额与选项，但不得自动下注。

Direct Play 主下注只接受整数娱乐筹码，允许任意不低于 10 的整数值；V1 五款 Direct Play 游戏均不启用游戏专属 Free Round / Free Summon。Blackjack Double / Split 等追加下注按照各自已经冻结的游戏规则执行。

---

## 11. 扭蛋机

采用 **纯筹码扭蛋**。

不做 Servant 收藏系统、灵基图鉴、卡牌背包、玩家交易、收藏品市场或养成系统。

扭蛋可以高度 FGO 化：单抽、十连、召唤阵、蓝光 / 金光 / 彩光、不同稀有度演出，但结果本质只产生筹码奖励。

概率、消耗、奖励、十连规则全部由后台配置。

---

## 12. Provably Fair

所有涉及随机结果的游戏采用 **Provably Fair 可验证公平**。

### 12.1 基础组成

- Server Seed
- Server Seed Hash
- Client Seed
- Nonce
- Round ID
- HMAC / SHA-256 等确定性随机过程

### 12.2 适用范围

Provably Fair 适用于所有由平台随机过程决定结果、牌序、符号、掉落或派奖映射的游戏。

V1 首发实例包括：

- 老虎机；
- 骰子；
- 21 点洗牌；
- 刮刮乐；
- 扭蛋；
- 德州扑克整副牌洗牌。

未来新增的随机结果游戏同样必须在正式发布前接入 Provably Fair，不得因为不在 V1 清单中而豁免。

### 12.3 原则

服务器在结果生成前提交 Seed Hash，结算后用户可以查看验证数据。德州扑克应对整副牌的洗牌顺序进行确定性生成，而不是每发一张牌临时随机。

### 12.4 Provably Fair 安全约束

Server Seed 必须：

- 使用密码学安全随机数生成器生成；
- 至少提供 256 bit 随机熵；
- 在接受下注前生成并提交 Server Seed Hash；
- 当前未公开 Server Seed 不得写入普通应用日志；
- 当前 Seed 应进行安全存储；
- Seed Reveal 后不得再次用于新游戏。

每个 Round 还必须绑定：

- algorithm_version
- game_config_version
- game_config_hash
- server_seed_hash
- client_seed
- nonce
- round_id

以避免管理员在玩家下注后：

- 修改随机算法
- 修改概率映射
- 修改赔率
- 修改游戏配置

然后仍然声称该局“Provably Fair”。

Nonce 必须保证不会在同一 Seed 生命周期中重复。

将哈希随机数映射为骰子、牌、老虎机符号等有限结果时，应避免 modulo bias（取模偏差）。

21 点和德州扑克应在一手牌开始前生成该手完整确定性牌序，同一手牌中不得重新生成 Seed 或临时重新洗牌。

---

## 13. 游戏配置与运营后台

采用 **完整游戏运营后台**。

管理员无需改代码即可配置：

- 游戏启停
- 维护状态
- Direct Play 全局下注策略的版本与生效状态
- 赔率
- 概率
- RTP
- 活动倍率
- 免费次数
- 每日奖励
- 游戏公告
- 其他游戏参数

V1 的 Direct Play 全局下注策略已经冻结为：

- 最低下注 10 娱乐筹码；
- 不设置产品层固定最高下注；
- 快捷金额 10 / 100 / 500 / 1000。

运营后台不得允许单个 Direct Play 游戏独立覆盖上述最低下注、最高下注或快捷金额。未来如需调整，只能通过新的全局、版本化下注策略进行，并需要新的产品需求确认。

### 13.1 配置版本化

每局创建时必须锁定 `game_config_version`。管理员修改配置后，只影响之后创建的局。历史局必须保留当时真实使用的配置版本。

### 13.2 配置审计

每次修改记录：操作者、修改前、修改后、时间、配置版本。支持审计，后续可考虑回滚。

### 13.3 Game Registry 与目录运营

Chaldea 运营后台必须通过动态 Game Registry / Game List 管理已接入游戏，而不是在后台 Sidebar 中写死首发游戏名称。

每个已接入游戏至少需要维护：

- 稳定 `game_slug`；
- 展示名称；
- 图片 / Logo；
- 简介；
- 分类与标签；
- 游戏模式；
- 进入类型；
- 发布状态；
- 运行状态；
- 排序与推荐；
- 下注规则摘要：Direct Play 引用全局下注策略；大厅型游戏展示其买入、盲注、底注或其他适用门槛；
- 透明度设置；
- 配置版本。

新增完整游戏仍需完成代码、服务端结算、钱包集成和测试；Game Registry 只负责接入、管理与发布，不负责无代码生成游戏逻辑。

### 13.4 Direct Play 全局下注策略管理

Chaldea 运营后台需要能够查看当前生效的 Direct Play 全局下注策略及其历史版本。

当前 V1 策略为：

```text
最低下注：10 娱乐筹码
产品层固定最高下注：无
快捷金额：10 / 100 / 500 / 1000
```

管理要求：

- 单个 Direct Play 游戏不得独立覆盖该策略；
- 如果未来修改全局策略，必须创建新策略版本；
- 新版本只影响生效后创建的 Round；
- 历史 Round 必须保留当时使用的下注策略版本或完整策略快照；
- 策略修改必须记录操作者、修改前、修改后、生效时间和版本；
- Poker 与其他大厅型多人游戏不引用该 Direct Play 策略，继续使用各自配置版本中的资金与下注规则。

---

## 14. 游戏信息透明度

采用 **每个游戏独立控制公开程度**。

后台可分别设置是否展示：

- RTP
- 赔率
- 掉落 / 抽取概率
- 完整权重表
- Provably Fair 信息

历史局仍需保存实际使用的真实配置与公平性数据。

---

## 15. 德州扑克

### 15.1 产品目标

最终目标为完整真人多人赌场大厅。

支持方向：

- 公开房
- 私人密码房
- 不同盲注 / 底注
- 2–9 人
- 创建 / 加入房间
- 实时下注
- 牌桌聊天
- 观战
- 掉线重连
- 行动超时
- 自动托管
- 房主设置
- 战绩
- 历史记录

### 15.2 V1

第一阶段先做 **Cash Game**。

重点验收：多人正常开局、一手完整打完、正确分池 / 结算、正确更新牌桌筹码、掉线后可以恢复、离桌可以安全退回筹码。

### 15.3 后续 V2

增加 **Sit & Go 单桌锦标赛**：报名费、固定人数、满员开赛、淘汰、按名次派奖。

暂不规划复杂多桌大型锦标赛。

---

## 16. 德州扑克资金模型

采用 **牌桌买入制**。

娱乐钱包 50,000 → 买入 20,000 → 娱乐钱包剩余 30,000 → table_stack = 20,000。

牌局期间只操作 table_stack，不频繁修改娱乐主钱包。离桌时将剩余 table_stack 一次性退回娱乐钱包。

必须正确处理：

- 正常离桌
- 掉线
- 重连
- 服务重启
- 被踢
- 重复请求
- 重复结算

德州扑克属于玩家之间的零和筹码转移，与单人无限庄家游戏分开统计。

### 16.1 德州扑克资金守恒

德州扑克属于玩家之间的零和资产转移。

在当前 V1 不收取 Rake 的前提下，对于任意一张牌桌：

`玩家娱乐钱包减少金额`
=
`牌桌 Stack + 当前 Pot + 已结算返回金额`

牌局本身不得创建或销毁筹码。

必须保证：

- 买入扣款与 table_stack 创建原子完成；
- Cash Out 与 table_stack 清零原子完成；
- Pot 与 Side Pot 计算确定性；
- All-in 正确创建 Side Pot；
- 平局时正确 Split Pot；
- Odd Chip 采用固定规则分配；
- 同一 Hand 只能 Settlement 一次；
- Poker Service 重启后能够根据 PostgreSQL 重建未完成牌局；
- 任何尚未结算的真实资产不得只存在 Redis 中。

Redis 只能保存实时状态缓存，PostgreSQL 才是最终可恢复的资产与牌局记录。

### 16.2 V1 同时入座限制

V1 中，一个用户同时只能在一张 Poker Table 入座，并且只能持有一个 Active Poker Session 与一组有效 `table_stack`。

用户必须先完成当前牌桌的 Safe Leave / Cash Out，才能在另一张牌桌再次入座。

该限制用于避免多个并行牌桌资产状态、重连目标和移动端沉浸式牌桌体验产生歧义。用户已在一张 Poker Table 入座期间，不允许再加入或观战另一张牌桌；必须先完成当前 Session 的 Safe Leave / Cash Out。

---

## 17. Rankings Center

采用唯一、公开的跨产品域 **Rankings Center**：

`/rankings`

一级局部导航：

- `Assets & Games`
- `RP Usage`

不为每个榜单建立独立一级页面，也不将 Rankings 增加为 PC Global Header 一级入口。

### 17.1 Assets & Games 榜单

至少包含：

- Total Assets / 总资产；
- Game Profit / 游戏净盈利；
  - Today；
  - This Week；
  - All Time；
- Biggest Win / 单局最高净收益；
- Total Wagered / 累计下注；
- Poker Profit / Poker 已实现盈利。

原“今日盈利榜、本周盈利榜、历史净盈利榜”在页面中合并为同一个 `Game Profit` 指标，通过周期切换展示，指标本身不删除。

#### Game Profit

`Game Profit = Direct Play 已结算净变化 + Poker 已实现 Session P/L`

以下资产变化不计入 Game Profit：

- 初始赠金；
- 每日签到；
- 每小时签到奖励；
- 救济金；
- 管理员发放；
- API 额度与娱乐筹码之间的兑换；
- Poker Buy-in、Top-up 与 Cash Out 中不产生所有权变化的内部资产移动。

所有来源于 Poker 的正式公开排名数据，仅在对应 Poker Session 完成 Cash Out 后提交。

#### Biggest Win

Biggest Win 可以包含：

- Direct Play 单个 Round 的正净收益；
- Poker 单个 Hand 的正净收益。

Poker Hand 只有在父 Session 完成 Cash Out 后才进入公共 Biggest Win 榜。

Poker Hand 的单局收益按：

`该 Hand 最终获得金额 - 该 Hand 自身实际投入金额`

计算，不得把包含玩家自身本金的完整 Pot 当成净赢金额。

#### Total Wagered

累计下注包括：

- Direct Play 每个有效 Round 的实际下注；
- Poker 中实际投入 Pot 的 Blind、Ante、Call、Bet、Raise 与 All-in。

同一笔 Poker 投入只能累计一次，并在 Session Cash Out 后进入正式公共榜单。

#### Total Assets

Total Assets 为当前资产快照，不提供过去某日资产回放。

统一总资产至少包括：

- API 总额度；
- 娱乐钱包中的可用娱乐筹码；
- Poker In Play，包括 `table_stack` 以及当前 Hand 中已经投入 Pot、但尚未完成所有权转移的资产；
- 跨库兑换处理中、尚未计入任一已结算目标余额且未与源余额重复计算的资产。

迁移切换完成后，所有既有迁移用户的上述资产从清零后的新状态开始统计。

### 17.2 RP Usage 排行榜

RP Usage 包含三个榜单：

- Calls；
- Errors；
- Credits Consumed。

每个榜单以 Master 用户为排名对象；同一 Master 的多个 RP Key 聚合为一条排名。V1 不额外建立独立模型排行榜。

只有通过请求发生时 `key_purpose_snapshot = RP` 的合格模型推理/生成请求，才进入 RP 排行。

模型列表、余额查询、健康检查、认证接口、管理接口等非模型生成请求不计入。

平台内部对同一逻辑请求执行的多次渠道重试只算一个逻辑请求；客户端独立重复发送的请求分别计数。

#### Calls

Calls 按成功完成的合格 RP 逻辑请求数排序。

每行同时展示：

- 成功调用数；
- 实际消耗 API Credit；
- Error Rate；
- 主要调用模型。

#### Errors

Errors 按失败逻辑请求数量排序，同时展示：

- Error Count；
- Error Rate；
- Total Attempts；
- 主要报错模型。

不按 Error Rate 排序，避免一次请求一次失败的用户以 100% 报错率排在首位。

只有已经通过有效 API Key 完成用户归属、并正式进入模型调用流程的失败请求才计入。

包括：

- 请求参数或模型调用错误；
- 模型不可用；
- Rate Limit / 429；
- 上游错误；
- Timeout；
- 平台内部调用失败；
- 流式调用中途失败。

不包括：

- 完全无效、无法归属用户的 API Key 探测；
- 请求进入上游前由用户主动取消；
- 平台内部针对同一逻辑请求的重试次数。

公共榜单不展示原始错误文本或具体错误请求。

#### Credits Consumed

Credits Consumed 按最终实际结算的 API Credit 排序。

不使用：

- 预估价格；
- 美元；
- NewAPI raw quota；
- 请求提交时的预扣估算。

失败或中断请求：

- 最终没有实际扣费时记为 `0`；
- 实际产生部分扣费时按最终真实扣费计入。

公开额度最多显示 6 位小数，并移除无意义尾随零。

V1 不根据 RP 排名自动发放 API Credit、娱乐筹码或其他奖励。

### 17.3 模型展示与筛选

三个 RP 榜单均提供 Model Filter。

用户排名列表显示该统计维度下的 Top 3 Models 与 `Other`；展开后可以查看完整模型分布。

模型摘要使用 Chaldea Display Name；展开后可以查看用户请求的真实 Chaldea Model ID。

不公开：

- 内部渠道；
- 上游凭证；
- 实际路由渠道；
- 渠道优先级；
- Provider 错误详情。

退役模型必须保留历史 Display Name 与 Model ID 映射。

### 17.4 统计周期与名次

RP Calls、Errors 和 Credits Consumed 均支持：

- Today；
- This Week；
- All Time。

规则：

- 业务时区统一为 `Asia/Shanghai`；
- 一周从星期一 00:00 开始；
- 统计从明确的 `RP Ranking Activation Time` 开始；
- V1 不回溯旧日志推测历史 RP 调用；
- 历史日榜和周榜永久保留，并允许通过周期选择器浏览；
- 当前周期指标为 0 的用户不进入对应榜单；
- 同分用户共享排名，采用 `1、2、2、4`；
- 排行榜采用近实时聚合，目标在 5 分钟内更新；
- 页面必须显示 Last Updated。

RP Ranking Activation Time 应设置在迁移余额清零与正式切换完成之后，确保排行榜从 Chaldea 新经济与新统计规则生效点开始。

### 17.5 公共可见性与隐私

Rankings Center 允许未登录用户公开浏览聚合结果。

公开显示：

- Master 昵称；
- Master 头像；
- 排名；
- 聚合调用次数；
- 报错次数与报错率；
- 聚合消耗额度；
- 聚合模型分布；
- 统计周期；
- Last Updated。

绝不公开：

- API Key 名称、ID 或 Secret；
- Prompt；
- Response；
- Request Body；
- Request ID；
- 单次请求的具体时间；
- 原始错误消息；
- IP；
- User-Agent；
- 渠道与 Provider 凭证。

Master 昵称和头像不产生公开个人主页链接。

已登录用户在榜单中看到固定 My Rank 摘要，只能从自己的 My Rank 跳转到带 RP Filter 的个人 API Usage，不能查看其他用户的详细请求。

### 17.6 Poker 排行榜统计结算点

Poker 牌桌中的实时筹码变化属于尚未完成统计结算的牌桌内资产变化。

玩家仍在牌桌中时：

- Poker In Play 仍属于玩家资产；
- 牌桌可以展示实时 Stack 与 Unrealized Session Delta；
- 当前未结束 Session 的实时输赢不进入 Game Profit、Poker Profit、Biggest Win 或 Total Wagered 的正式公共统计。

当玩家完成 Safe Leave / Cash Out 后：

- Session Realized P/L 按 Cash Out 时间归入对应日榜或周榜；
- 该 Session 内的 Poker Hand Biggest Win 与 Total Wagered 才正式提交；
- 总资产始终以玩家仍然实际拥有的资产为准。

---

## 18. 游戏历史与公共记录

个人完整 Game History 仅本人和管理员可查看。

统一入口：

`/history`

默认 All Records 混合展示：

- Direct Play Round；
- Poker Session。

默认列表不平铺全部 Poker Hand，避免一场 Poker Session 产生大量 Hand 记录并淹没其他游戏历史。

Poker Hand 主要通过：

`Poker Session Detail → Hand List → Hand Detail`

进入，同时允许在高级 Record Type Filter 中单独查询。

Game History 至少支持：

- Record Type；
- Mode；
- Game；
- Time Range；
- Result；
- Status；
- Round / Session / Hand ID 搜索。

Result 至少包括：

- Win；
- Loss；
- Break-even；
- Cancelled；
- Refunded。

Status 至少包括：

- Processing；
- Settled；
- Cancelled；
- Refunded；
- Recovering。

从 Detail 返回列表时，必须保留原筛选条件与滚动位置。

V1：

- 不提供个人完整游戏历史公开分享；
- 不提供 CSV / JSON 导出；
- 手机端使用记录 Card 与 Filter Bottom Sheet，不压缩 PC 宽表格；
- 退役游戏继续保留历史名称、筛选项、配置版本和 Provably Fair 数据；
- RP API 请求记录不进入 Game History，继续由 `/api/usage` 承载。

公共区域可以展示：

- Rankings；
- 大额中奖；
- 最近大奖；
- 适合公开的精选记录。

公共 Recent Wins / Featured Records 与私人 Game History 必须分离，不公开每个人的完整输赢流水。

Rankings 使用用户当前 Master Profile；公共 Recent Wins / Featured Records 与历史业务事件保存事件发生时的昵称与头像快照。所有记录的真实归属继续使用稳定 `newapi_user_id`。

---

## 19. 公告与活动

采用 **完整 Announcements & Events 系统**。

用户侧继续使用：

- `/announcements`：公告列表；
- `/announcements/:id`：公告详情。

V1 不为致谢名单新增 `/sponsors` 或 `/acknowledgements` 一级页面。致谢名单继续作为标准公告内容，通过 Announcement Detail 展示。

### 19.1 公告类型

V1 支持以下公告类型：

- System / 系统公告；
- New Models / 新模型；
- Game Events / 游戏活动；
- Maintenance / 维护通知；
- Important / 重要提醒；
- Acknowledgements / 致谢与赞助鸣谢。

公告类型用于分类、筛选和内容语义，不自动决定置顶、弹窗或 Banner。

### 19.2 展示渠道 / Placement

每条公告可以独立配置以下展示渠道：

- Pinned in Announcement List：在公告列表中置顶；
- Entry Popup：未登录用户进入平台根入口或登录页时弹出；
- Post-login Popup：用户完成登录后，在安全页面弹出；
- Public Home Banner：首页 Banner；
- Dashboard Summary：Dashboard 公告摘要。

上述 Placement 相互独立：

- 置顶公告不会因为置顶而自动弹窗；
- Entry Popup 不会自动同时成为 Post-login Popup；
- 首页 Banner 与 Dashboard Summary 也不由公告类型或置顶状态自动推导。

公告列表允许同时存在多条置顶公告。V1 同一时点最多允许一条有效 Entry Popup；发布或排期操作必须阻止多个 Entry Popup 的有效时间相互重叠。

### 19.3 Entry Popup / 入口公告弹窗

Entry Popup 面向未登录用户。

触发范围：

- 未登录用户进入 `/`；
- 未登录用户进入 `/login`。

普通 Entry Popup 不强制覆盖：

- `/models/:model`；
- `/rankings`；
- `/announcements/:id`；
- 其他具有明确访问意图的公共 Deep Link。

入口公告加载失败时必须 Fail Open，不得阻止登录、Discord 注册或公共页面浏览。

Entry Popup 使用以下频率规则：

- 每个 `announcement_id + notification_revision`，每个浏览器默认只弹出一次；
- 同一浏览会话内无论发生刷新、登录失败返回或路由切换，最多弹出一次；
- 未登录关闭状态保存在浏览器本地，不提供跨设备同步；
- 普通文字修正不会自动重置关闭状态；
- 只有管理员明确执行 `Re-notify / 发布新通知展示版本`，才生成新的 `notification_revision` 并允许再次弹出。

Entry Popup 必须为非阻断式：

- 允许立即关闭；
- 不设置强制倒计时；
- 不要求滚动到底；
- 不要求勾选同意；
- PC 支持明确关闭按钮和 `Esc`；
- 手机支持始终可见的关闭入口；
- 提供“查看完整公告 / 查看完整致谢名单”并进入 Announcement Detail。

关闭 Entry Popup 只表示 `popup_dismissed`，不等于公告已经阅读。

### 19.4 Post-login Popup 与关键流程保护

Post-login Popup 与 Entry Popup 是不同展示渠道。

同一公告、同一通知展示版本在本次浏览会话已经作为 Entry Popup 展示后，登录成功时不得立即再弹一次相同内容。

普通 Post-login Popup 不得直接遮挡：

- Poker Table；
- 正在恢复或进行中的 Direct Play Round；
- Wallet Processing / 资产处理中页面；
- 其他需要用户立即处理的关键状态。

此类普通公告应延迟到 Dashboard 或下一个安全普通页面展示。紧急维护或安全事件继续使用独立 Critical Notice 语义，不应滥用普通公告弹窗。

### 19.5 置顶公告、列表与详情

`/announcements` 至少包含：

- Pinned Announcements；
- Latest Announcements；
- Type Filter；
- Search；
- Date Filter；
- Archive。

排序规则：

1. 当前有效的置顶公告，按照管理员手动顺序；
2. 其他当前公告，按照发布时间倒序。

过期或归档公告不进入 Entry Popup、首页 Banner 或 Latest，但可以根据公开范围进入 Archive。

Announcement Detail 承担完整正文、发布时间、更新时间、类型、必要附件或结构化内容。打开详情时，已登录用户将该通知版本标记为已读。

### 19.6 公告生命周期与时间

公告使用以下生命周期：

- Draft；
- Scheduled；
- Published；
- Expired；
- Archived。

至少支持：

- `publish_at`；
- `visible_from`；
- `visible_until`，可为空。

公告运营时间统一使用 `Asia/Shanghai`。

允许长期公告不设置结束时间。Acknowledgements 可以长期保持 Published + Pinned。

### 19.7 已读、未读与展示版本

未登录用户：

- 不维护跨设备已读状态；
- 只在浏览器本地记录当前 Entry Popup 通知展示版本是否已关闭；
- 公告列表不提供匿名用户个人未读数量。

已登录用户：

- 阅读状态保存到服务端；
- 支持多设备同步；
- 打开 Announcement Detail 时标记当前通知版本已读；
- 管理员发布新的通知展示版本时，可以重新进入未读状态。

以下状态必须分离：

- `popup_dismissed`：关闭弹窗；
- `announcement_read`：打开并阅读公告详情。

### 19.8 Acknowledgements / 致谢名单

平台维护一条长期存在的规范 Acknowledgements 公告，而不是不断创建多条同名致谢公告。

默认规则：

- Visibility：Public；
- Pinned：Yes；
- Entry Popup：Yes；
- Post-login Popup：No；
- Public Home Banner：No；
- Dashboard Summary：可以独立启用并进入 Pinned 摘要；
- `visible_until`：为空。

致谢公告支持结构化 Sponsor / Contributor List。每个条目可以包含：

- Display Name：必填；
- Avatar / Logo：可选；
- External Link：可选；
- Acknowledgement Note：可选；
- Group：可选；
- Manual Order：必填排序语义。

支持：

- 公开昵称；
- Discord 昵称；
- 项目名；
- Anonymous / 匿名赞助者；
- 人工分组，例如“特别鸣谢”“赞助支持”“贡献者”。

V1 默认不公开具体赞助金额，也不根据金额自动生成 Gold / Silver / Bronze 等级。

公开真实姓名、头像、Logo 或外部链接前应取得对应赞助者同意。不得公开：

- Discord User ID；
- 邮箱；
- 支付账号；
- 付款截图；
- 交易流水号；
- 未经同意的真实姓名；
- 其他私人联系方式。

### 19.9 Markdown / Rich Text 与外部内容安全

公告支持 Markdown / 受控富文本，但必须统一清洗和安全渲染。

禁止：

- 任意 `<script>`；
- 自定义 JavaScript；
- 危险事件属性；
- 任意 iframe；
- 未经处理的原始 HTML；
- 通过赞助者链接、头像或 Logo 注入可执行内容。

外部链接必须采用安全打开方式。图片、头像和 Logo 使用受控上传或平台媒体资源。

### 19.10 发布、更新、归档与审计

后台至少支持：

- Create Draft；
- Preview；
- Schedule；
- Publish；
- Update；
- Withdraw / Archive。

已发布公告修改时，管理员必须明确选择：

1. `Update Content Only`：仅更新内容，不重新弹窗；
2. `Publish New Notification Revision`：更新内容，生成新的通知展示版本并允许重新提醒。

已经发布的公告不进行无审计硬删除。使用撤回、过期或归档处理，并保留历史版本。

审计至少记录：

- 操作者；
- 修改前后内容；
- 类型；
- Visibility；
- Placement；
- Pinned Order；
- 发布时间与可见时间；
- Re-notify 行为；
- 撤回或归档时间；
- Sponsor / Contributor List 变化。

第一阶段不做 Discord 自动同步。

---

## 20. 社区功能

不开发全站公共聊天室。

仅保留 **德州扑克牌桌内聊天**。其他社区交流继续使用 Discord。

每条 Table Chat 消息必须保存稳定用户 ID，以及消息发送时的 Master 昵称和头像快照。后续改名不得重写已经发送的历史消息身份。

---

## 21. 视觉设计

视觉方向：**FGO / 迦勒底主题 + 高级现代二次元 Web UI**。

不是简单贴 Fate 图片，而是以“迦勒底终端 / Master 系统”为整体设计语言。

### 21.1 视觉原则

- 视觉是一级需求
- 强 FGO / Chaldea 氛围
- 现代 Web 信息架构
- 不为了主题牺牲可用性
- 专业功能名词仍保留正常含义
- Fate 元素主要承担视觉与副标题表达

可能使用的视觉元素：Chaldea Terminal、Master、令咒、魔术回路、召唤阵、职阶元素、圣晶石式视觉、金卡 / 彩光、灵子演算感、FGO 风格 UI 副标题。

### 21.2 视觉细节暂不冻结

当前不确定：精确色板、字体、卡片半径、Hero 构图、角色选择、背景图、具体动画、图标系统、页面级动效。

这些放到独立的 **Art Direction / 视觉设计阶段** 再确定。

### 21.3 视觉开发流程

1. 收集参考网站 / FGO UI / 喜欢的截图
2. 做 Moodboard
3. 生成 3–5 套视觉方向
4. 浏览器实际渲染
5. 截图评审
6. 确认 Design System
7. 再批量开发页面

图片和动画可在开发阶段由 Codex / Antigravity / Gemini 图像与视频能力生成，再静态集成到项目中。不建议用户访问网站时实时调用 AI 生图 / 生视频。

---

## 22. 动画与媒体原则

复杂视觉建议混合使用：

- 静态图片：WebP / AVIF
- SVG
- CSS 动画
- Framer Motion / GSAP
- Canvas / WebGL
- Lottie / Rive
- WebM 视频

召唤演出可组合按钮动画、召唤阵旋转、粒子、光效、卡牌翻转和关键阶段视频演出，不建议整段交互全部用视频实现。

---

## 23. 移动端

采用 **完整响应式设计**。

要求：PC、平板、手机完整可用，不开发独立 App。

所有直接游玩游戏必须针对移动端重新排版，不能将 PC 游戏区域机械缩小。大厅型与沉浸式游戏应根据各自交互需求设计移动端布局。德州扑克需设计桌面完整牌桌、手机竖屏紧凑牌桌和手机横屏优化牌桌。

复杂动画根据设备性能和 `prefers-reduced-motion` 自动降级。

---

## 24. 管理后台与 Chaldea Operations

平台继续采用：

**NewAPI 原管理员后台 + Chaldea Operations 双后台。**

### 24.1 双后台边界

NewAPI Admin 继续负责：

- NewAPI 原生 Users；
- Discord OAuth 与密码账号体系；
- API Key 原生能力；
- Channels；
- 模型路由与底层模型状态；
- 倍率与真实计费；
- NewAPI quota；
- NewAPI 原生日志与管理功能。

Chaldea Operations 不重做、iframe 嵌入或复制完整 NewAPI Admin。

Chaldea Operations 负责：

- Overview / Needs Attention；
- Chaldea Models Catalog 元数据；
- Users & Identity / Master Moderation / Account Support；
- Games / Game Registry / 配置版本；
- Poker 实时运营、恢复和聊天管理；
- Economy / Wallet / Ledger / Transfer / Reconciliation / Adjustment；
- Rewards 配置、Claims 和发行统计；
- Rankings 聚合、历史快照和修复；
- Records / Incidents / Fairness Verification；
- Announcements & Events / Acknowledgements；
- Operations / Service Health / Maintenance / Incidents；
- Access Control / RBAC；
- append-only Audit。

NewAPI Admin 身份不会自动授予 Chaldea Operations 权限；Chaldea Super Admin 也不会自动获得 NewAPI Admin 权限。

### 24.2 Chaldea Operations 一级信息架构

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

分组标题不建立独立业务页面。

### 24.3 RBAC

V1 使用：

- `Super Admin`：完整 Chaldea Operations 权限；
- `Operator`：按照模块 Scope 获得日常运营权限；
- `Auditor`：全后台只读和 Audit 查看。

Operator Scope 至少包括：

- Models；
- Users & Identity；
- Games；
- Poker；
- Rewards；
- Rankings；
- Records；
- Announcements。

以下操作仅允许 Super Admin：

- 资产 Adjustment；
- Discord Rebind 最终执行；
- Access Control；
- 全站 Maintenance；
- Poker Emergency Pause；
- 概率、赔率、RTP 和其他经济配置激活；
- 关键修复结果发布。

权限必须由服务端校验。任何角色均不得查看密码、密码哈希、API Key Secret、未授权 Prompt / Response、未公开 Hole Cards 或未公开 Server Seed。

### 24.4 Operations Shell 与后台路由

`/ops` 默认进入 Overview。PC 使用持久 Sidebar；Tablet 使用可折叠 Sidebar；Mobile 使用 Drawer。

顶部至少显示：

- Production / Staging / Development 环境；
- Global Search；
- Needs Attention 数量；
- 返回普通用户站点；
- 当前管理员身份。

后台对象详情使用稳定 Deep Link。筛选、排序和分页写入 URL；从 Detail 返回时恢复原列表状态。列表默认使用明确分页，Drawer 只用于快速预览，复杂详情使用正式页面和 Breadcrumb。

Global Search 可以定位用户、Transfer、Transaction、Round、Table、Session、Hand、Announcement、Config 与 Audit ID，但不得搜索或展示 Secret、密码、Prompt / Response 或未公开牌面。

### 24.5 Overview 与 Needs Attention

Overview 以待处理问题为优先，不以图表为首要内容。

Needs Attention 聚合：

- 长时间 Pending Transfer；
- 补偿失败；
- 奖励异常；
- 未完成或恢复失败 Round；
- Paused / Recovering Poker Table；
- 未完成 Cash Out；
- Rankings 聚合延迟；
- Model Sync / Metadata 缺失；
- 公告排期冲突；
- Discord Binding Conflict；
- Account Support Case；
- Maintenance 状态。

Attention Item 使用 Critical / Warning / Info，必须链接到对象详情。Acknowledge 只代表已经查看，不代表问题解决。金融或牌局问题不得被永久隐藏或删除。

### 24.6 Models

Models 管理 NewAPI 同步模型和 Chaldea 前台元数据。

NewAPI Source Data 在 Chaldea Operations 中只读，包括 Model ID、底层状态、原始价格 / 倍率摘要、渠道摘要和最近同步时间。

Chaldea 可管理展示名称、Logo、简介、标签、推荐用途、上下文长度、副标题、排序、推荐和公开状态。

状态至少区分 Pending Metadata、Published、Hidden、Unavailable、Retired。发布前必须验证真实模型存在、元数据完整、价格展示与状态映射。展示字段不得改变真实计费。Retired 模型的历史 Usage 与 Rankings 名称继续保留。

### 24.7 Users & Identity

Users & Identity 提供 User Search、Master Moderation、Support Cases、Binding Conflict 和 Migration Acknowledgement。

User Detail 使用 Overview、Identity、Assets、Rewards、Activity、Support、Audit 分区。

支持 Rename Required、审计化 Forced Rename、Reserved Name 与昵称冷却查看。Discord Rebind 和 Legacy Binding 必须通过 Support Case，状态至少包括 Open、Verifying、Approved、Executed、Closed、Rejected。

不得在 Users 页面直接修改余额、NewAPI Account Status 或密码。不得无审计覆盖 Discord Binding、自动合并账号或迁移资产。Legacy Password Recovery 中管理员不得查看或指定最终密码。

### 24.8 Games

Games 由动态 Game Registry 驱动。

Game Detail 包含 Overview、Metadata、Publication、Runtime、Configuration、Fairness、History。

Active Config 不可直接编辑：

```text
Clone as Draft
→ Edit Allowed Fields
→ Validate
→ Preview
→ Activate as New Version
```

经济配置激活仅允许 Super Admin。普通 Metadata 可以由 Games Operator 管理。已冻结产品规则只读，不能通过通用 JSON Editor 绕过。

Maintenance 主要阻止新 Round，不遗弃已接受 Round。管理员不得修改已产生结果和资产。后台表单不能无代码生成完整可玩游戏。

### 24.9 Poker

Poker Operations 包含 Service Overview、Tables、Sessions、Hands、Recovery、Chat Moderation。

允许 Stop Accepting Players、Stop New Hands、Close After Hand、Remove After Hand、Remove Spectator、Mute、Pause、Request Recovery。

禁止修改 Stack、Pot、赢家、牌序、Settlement，禁止当前 Hand 强制 Cash Out，也禁止提前查看 Hole Cards、Server Seed 或完整牌序。

Emergency Pause 要求 Super Admin、Fresh Authentication 和原因。Recovery 只能执行状态机允许的 Resume、Pause、Safe Close、Escalate。

### 24.10 Economy、Reconciliation 与 Adjustment

Economy 包含 Asset Overview、Wallets、Ledger、Transfers、Reconciliation、Adjustments、Issuance & Burn、Migration Snapshots。

后台可查看 Active NewAPI Quota、Reserve、Available Chips、Poker In Play、Processing Assets 与 Total Assets。

Wallet 和 Ledger 默认只读，不提供直接编辑最终 Balance 的输入框。

Reconciliation Worker 优先自动处理。管理员只能执行状态机允许的 Retry、Resume、Compensate、Mark for Review，不得直接改为 CONFIRMED 或删除失败记录。

Admin Adjustment 仅允许 Super Admin，必须填写 Reason 和 Reference，并展示 Balance Before / Delta / After；提交要求 Fresh Authentication、Typed Confirmation、唯一 Operation ID、Ledger 与 Audit。负向调整不得产生负余额，普通 Adjustment 不得直接修改 Poker In Play。

V1 不提供面向全部用户的一键批量资产发放。

### 24.11 Rewards

Rewards 包含 Configurations、Claims、Issuance Analytics、Maintenance。

配置使用 Draft → Validate → Preview → Schedule / Activate → Version Locked。

Rewards 后台只读展示当前 V1 数值政策：初始赠金 1,000 API Credit、Daily 500 API Credit、Hourly 100、Relief 300；Relief Fund 的 `Total Assets < 10` 与成功领取后滚动 4 小时规则同样只读。Hourly / Relief 的资产类型及 Hourly 时间口径在确认前不得成为可编辑生产字段。

管理员不能把失败 Claim 改成 SUCCESS。安全重试复用原 Claim / Business ID；人工补发进入 Economy Adjustment；不能为特定用户覆盖固定奖励数量，也不能伪造一条签到或救济成功记录。

### 24.12 Rankings

Rankings 包含 Assets & Games、RP Usage、Aggregation Status、Historical Snapshots、Repair & Rebuild。

管理员不能直接编辑用户榜单分数。

修复流程采用 Shadow Snapshot → Compare Diff → Review → Publish。源记录排除、修复、取消排除和重新聚合均须填写原因并审计。后台提供与公共页面一致的 Preview。

### 24.13 Records

Records 统一承载 Direct Play Round、Poker Session、Poker Hand、Settlement / Refund Incident 与 Fairness Verification。

记录默认只读，不得修改下注、结果、Seed、牌序、Payout、Stack 或 Settlement。私有牌面与未公开 Seed 不因管理员权限提前显示。

异常通过 Create Incident 进入 Economy / Poker Repair。退役游戏和关闭牌桌的历史记录继续保留。V1 不提供批量导出全部用户敏感游戏记录。

### 24.14 Announcements & Events

继续沿用 Draft、Preview、Schedule / Publish、Pinned Order、Placements、Acknowledgements、Versions / Audit 工作流。

具有 Announcements Scope 的 Operator 可以发布普通公告。Entry Popup 同一时点最多一条有效。Re-notify 必须确认影响并生成新 Notification Revision。已发布公告不得无审计硬删除。发布前支持 PC、Mobile 与 Entry Popup Preview。

### 24.15 Operations 与 Maintenance

Operations 只提供业务级 Service Health、Background Jobs、Maintenance、Incidents。

不得提供 SSH、Docker Shell、任意命令、SQL Console、Redis Console、系统包升级或 VPS 防火墙管理。

Maintenance 支持 Chaldea User Writes、Wallet & Exchange、Rewards、Direct Play New Rounds、Poker New Tables / New Hands、Rankings Publishing、Announcements Scheduling 等范围。

Maintenance 要求 Reason、Impact Preview、可选 Schedule、Fresh Authentication 与 Confirm，并不得遗弃已经接受的 Round、Poker Hand、Transfer 或奖励发放。NewAPI 模型 API 的底层维护继续由 NewAPI Admin 负责。

### 24.16 Access Control、危险操作与 Audit

Access Control 管理 Administrators、Roles、Operator Scopes 和 Permission Audit。

危险操作分为：

- Level 1 Routine：普通查看、Draft 和未发布元数据；
- Level 2 Impactful：发布公告、非经济展示配置激活、单游戏 Maintenance、Rankings Rebuild、关闭 Poker 新玩家；
- Level 3 Critical：资产 Adjustment、Discord Rebind、Access Control、全站 Maintenance、Poker Emergency Pause、经济版本发布、手工补偿。

Level 3 必须使用：

```text
Fresh Authentication
+ Required Reason
+ Typed Confirmation
+ Impact Preview
+ Unique Operation ID
+ Append-only Audit
```

V1 不强制双人审批；未来管理员团队扩大后再考虑 Four-eyes Approval。

Audit 为 append-only，至少记录 Actor、Role / Scope、Action、Target、Before、After、Reason、Operation ID、Result、Timestamp、Related Business ID。Audit 不允许编辑或删除，也不得保存密码、Secret、未公开 Seed 或完整 Prompt / Response。

金融操作撤销必须通过新的反向账变或补偿操作，不能删除原 Ledger 或 Audit。

### 24.17 响应式后台

PC 使用持久 Sidebar、宽表格和高密度筛选。Tablet 使用可折叠 Sidebar。Mobile 使用 Drawer、管理 Card、全屏或 Bottom Sheet Filter 与全屏危险操作确认。

移动端不得依赖 Hover；排序不得只依赖拖拽，必须提供上移 / 下移等替代操作。关键管理能力仍需完整可用。

具体配色、图标、视觉密度和 FGO 装饰留到 Art Direction v0.4。

---

## 25. 数据库现状

现有 NewAPI 使用 **PostgreSQL**，不是 SQLite，也不是 MySQL。

当前已有：

- PostgreSQL 容器
- Redis 容器
- Docker Compose
- NewAPI 持久卷
- `examples/deployment/external-newapi` 部署目录

Redis 当前主要承担缓存 / 会话等，不作为最终资产账本。

---

## 26. 数据层设计

采用：**同一个 PostgreSQL 实例 + 两套逻辑数据库**。

### 26.1 NewAPI 数据库

保持现有 `newapi`，存 users、tokens、channels、logs 和 NewAPI 原有数据。

Discord OAuth 创建的账号必须具备稳定 Password Login Identifier。其生成、唯一性和密码登录兼容方式由后续技术设计核验；不得使用 Master 昵称替代。

### 26.2 Chaldea 数据库

新建 `chaldea_platform`，计划存：

- master_profiles
- master_profile_name_history / reserved_master_names
- master_profile_avatar_snapshots
- identity_display_snapshots
- migration_notice_versions / migration_notice_acknowledgements
- discord_binding_support_audit / account_support_audit
- api_key_purpose_metadata / api_key_purpose_history
- oauth_registration_operations / registration_idempotency_records
- public_service_status_snapshots / critical_notice_state
- wallets / chip balances
- wallet_ledger
- transfers
- reward_configs
- reward_claims
- checkins（如每日签到保留独立记录）
- announcements
- announcement_revisions / notification_revisions
- announcement_placements / pinned_order
- announcement_read_states
- acknowledgement_entries / sponsor_contributor_entries
- announcement_media_assets
- announcement_audit_logs
- model_catalog_metadata / model_catalog_publication
- model_sync_snapshots / model_availability_mappings
- ops_roles / ops_scopes / ops_admin_assignments
- ops_attention_items / ops_incidents
- ops_support_cases / ops_binding_cases
- ops_maintenance_windows / ops_background_jobs
- ops_admin_operations / ops_audit_logs
- ops_reconciliation_actions / ops_adjustments
- poker_recovery_incidents / poker_chat_moderation_actions
- record_incidents
- games / game_registry
- game_metadata
- game_categories / game_tags
- game_configs
- game_rounds
- game_bets
- game_results
- provably_fair_seeds
- poker_tables
- poker_seats
- poker_hands
- poker_actions
- ranking_snapshots / ranking_periods
- rp_usage_aggregates
- ranking_reaggregation_jobs / ranking_exclusions
- migration_batches / migration_balance_reset_audit
- 其他平台业务数据

### 26.3 权限隔离

建议独立数据库用户：NewAPI DB 用户只访问 newapi；Chaldea App 用户只访问 chaldea_platform。

平台服务不要随意直接 UPDATE NewAPI 核心表。

Chaldea Operations 的 RBAC、模块 Scope 与危险操作必须在服务端强制校验。需要读取 NewAPI 数据的后台页面优先使用受控只读接口或最小权限读取；任何跨库写操作继续使用既定业务服务、状态机、审计与补偿流程。

---

## 27. Redis

继续复用现有 Redis 实例，但使用明确 key namespace，例如：

- `newapi:*`
- `chaldea:session:*`
- `chaldea:poker:*`
- `chaldea:lock:*`
- `chaldea:cache:*`
- `chaldea:return-intent:*`
- `chaldea:auth-flow:*`

Redis 适合缓存、会话、WebSocket 状态、房间临时状态、锁和短期数据。最终余额、账变、正式牌局结果仍以 PostgreSQL 为准。

---

## 28. 现有数据迁移、余额清零与兼容

采用：

**保留现有账号与 API 使用身份，迁移切换时统一清零旧用户资产余额。**

### 28.1 必须保留

- 现有用户账号；
- Discord 绑定；
- 密码与密码哈希；
- API Key 与原 Key Secret / Token 关系；
- 渠道；
- 模型；
- 倍率与计费配置；
- 历史 API 调用记录；
- 管理员与系统配置；
- 为审计和回滚保存的迁移前用户 quota 快照。

### 28.2 清零与迁移初始赠金

在正式 Cutover 维护窗口内，对迁移范围内所有现有用户先执行：

```text
Active NewAPI Quota = 0
Reserve API Credit = 0
Entertainment Wallet = 0
Poker In Play = 0
```

Chaldea 上线前不存在的娱乐钱包与 Poker 资产按 0 初始化。

清零与校验完成后，同一迁移批次必须为每个迁移用户发放：

```text
Migration Initial Grant = 1,000 API Credit
```

迁移初始赠金：

- 进入 Reserve API Credit；
- 使用 `initial_grant:migration:{migration_batch_id}:{user_id}` 或等价唯一业务 ID；
- 不依赖用户首次登录；
- 不触发新用户注册回调；
- 同一迁移批次重复运行时不得重复发放。

发放完成且不存在其他新账变时：

```text
API Credit = 1,000
Available Chips = 0
Poker In Play = 0
Total Assets = 1,000
```

随后：

- 用户账号仍可登录；
- Discord 绑定和密码继续有效；
- 现有 API Key 继续有效，并可使用这 1,000 API Credit；
- 旧 API 调用记录继续可查，但不回溯进入 RP 排行；
- 现有 Key 的 Usage Purpose 初始为 `Unclassified`。

该政策与新用户 1,000 API Credit 初始赠金保持等额，但迁移触发与注册触发必须在账本中分开标识。

### 28.3 清零执行要求

迁移清零必须：

1. 在维护模式下执行，并暂停新注册、API 扣费、钱包兑换和游戏资产操作；
2. 在执行前完成 NewAPI PostgreSQL 全量备份与用户 quota 导出快照；
3. 使用唯一 `migration_batch_id`；
4. 为每个用户记录：
   - `newapi_user_id`；
   - 清零前 raw quota；
   - 清零后 raw quota；
   - 清零后的 Reserve 值；
   - 迁移初始赠金业务 ID、金额与发放结果；
   - 执行时间；
   - 迁移版本；
   - 执行结果；
5. 具备批次级幂等，重复运行同一批次不得产生第二次业务影响；
6. 执行后校验所有迁移用户的 Active、Reserve、Wallet、Poker In Play 与 1,000 API Credit 初始赠金；
7. 生成迁移校验报告并由管理员确认后再开放平台；
8. 不得通过普通用户管理页面逐个手工修改余额。

### 28.4 回滚边界

如果清零或校验失败：

- 在对用户重新开放 API、Wallet 和 Games 之前，使用完整备份回滚；
- 不允许在已经产生迁移初始赠金、其他 Chaldea 奖励、兑换、调用消费或游戏账变后，仅恢复旧 quota 字段形成混合经济状态；
- 平台正式开放后如发现个别迁移错误，必须通过专门的迁移修复流程与审计记录处理，不能直接修改最终余额。

### 28.5 用户体验

现有用户上线新站后：

- 不重新注册；
- 不强制重新绑定 Discord；
- 不重新创建 API Key；
- 首次进入时完成 Master Profile 初始化；
- 初始化完成后展示一次性、版本化 Migration Notice；
- Notice 明确旧额度已按开服公平规则清零，但账号、Discord、密码、API Key 和历史 API Usage 均未删除；
- Notice 明确已通过迁移批次发放 1,000 API Credit 初始赠金；
- 现有 Key 初始为 `Unclassified`；
- 迁移初始赠金不表示重新注册，也不得重复触发新用户初始赠金流程；
- 提供 Wallet、Rewards Center、API Keys 与 API Usage 入口；
- 用户可以继续通过 Daily、Hourly、Relief Fund、管理员发放或游戏 / 兑换规则获得或转换平台资产。

Migration Notice 是独立认证后 Interstitial，不作为普通 Announcement。它不提供右上角直接关闭，用户通过 `我已了解，继续` 确认。服务端保存迁移版本、用户和确认时间；未确认时下次登录继续展示。

完成确认后：

- 存在合法 Return-to-Intent 时前往原目标；
- 不存在有效目标时进入 Dashboard；
- 普通 Post-login Popup 排在 Master Initialization 与 Migration Notice 之后，并避免遮挡 Poker、活动 Round 或 Wallet Processing。

迁移告知不得暗示历史调用记录、账号或 API Key 被删除，也不得提供恢复旧额度按钮。


---

## 29. 代码与部署目录

保持完全分开。

### 29.1 现有 NewAPI

`examples/deployment/external-newapi`

尽量保持原样。

### 29.2 新平台

`examples/deployment/platform`

包含 frontend、platform-backend、poker-server、Chaldea Docker / Compose 配置、环境变量、迁移脚本等。

---

## 30. V1 服务形态

不做过度微服务拆分。

推荐：

### frontend

- React / Vite
- 静态构建

### platform-backend

优先 Go，内部模块化：auth bridge、master profile、wallet、transfer、rewards、games、provably fair、ranking、announcements、admin、audit。

### poker-server

独立 Go WebSocket 服务。

### 公共基础设施

- PostgreSQL
- Redis
- Nginx / Caddy
- Docker Compose

整体是：**模块化单体 + 独立实时 Poker 服务**。

---

## 31. 部署目标

V1 优先 **单台 VPS 一键部署**，使用 Docker Compose。

### 31.1 推荐生产配置

- 8GB RAM
- 4–8 vCPU
- 150 Mbps 带宽
- 800GB–1TB 月流量
- 推荐 50GB 以上系统盘
- 80GB 更舒适

### 31.2 兼容目标

项目应在 4GB VPS 上通过合理限制运行，但不作为推荐生产环境。

### 31.3 当前候选 VPS

候选 VPS 都为美国洛杉矶、150 Mbps、8GB RAM；CPU 候选为 4 vCPU 或 8 vCPU。

如果价差不大，优先 8C8G；如果价差明显，4C8G 对 10–50 人规模也足够。

---

## 32. 资源与性能原则

第一阶段 10–50 人并发，无需从第一天就做分布式部署。

后续如规模提升，可独立迁移：PostgreSQL、Redis、Poker Service、NewAPI、静态媒体 / CDN。

复杂 FGO 图片、视频建议以后可迁移对象存储 / CDN。

---

## 33. 明确不做 / 暂不做

当前明确排除或延期：

- 普通密码直接注册
- 注册码体系
- 用户间筹码赠送
- 用户间筹码转账
- 筹码交易市场
- 现金充值商城
- Fiat 提现
- 全站聊天室
- 完整社区论坛
- Servant 收藏系统
- 灵基图鉴
- 卡牌养成
- 扭蛋收藏品交易
- 在线 API Playground
- 大型 API 教程站
- NewAPI 管理后台全面重做
- 德州扑克大型多桌锦标赛
- V1 Sit & Go
- V1 多服务器分布式部署
- 其他用户公开 Master Profile 页面
- Profile Bio、签名、生日、地区和社交链接
- V1 自定义头像上传
- V1 昵称 Emoji
- 用户自助 Discord Unlink / Rebind
- Email / Phone Password Recovery
- V1 TOTP、Passkey、Backup Codes 与设备管理中心
- 普通用户自助硬删除账号
- Chaldea 保存第二份密码或直接 SQL 修改密码字段
- Chaldea Operations 内置 SSH、Docker Shell、任意命令执行、SQL Console 或 Redis Console
- 后台直接编辑最终 Wallet Balance、Poker Stack、Pot、开奖结果、牌序或 Rankings 分数
- 管理员提前查看未公开 Hole Cards、Server Seed 或完整牌序
- V1 完全自定义逐权限可视化权限设计器
- V1 强制双人审批 / Four-eyes Approval
- V1 面向全部用户的一键批量资产发放
- V1 批量导出全部用户敏感游戏记录
- 传统用户名 / 密码 / Email / Phone 自助注册表单
- 将 Master 昵称作为 Password Login Identifier
- Public Home 自动轮播 Banner
- Public Home 自动播放视频或声音
- 虚构热门度、在线人数、中奖记录或服务状态
- V1 独立 `/status` 页面
- 未经确认的 Terms / Privacy / Cookie / Sponsor Payment 页面

---

## 34. 当前版本里程碑建议

### Phase 0 — 安全、迁移与经济重置准备

- 完整备份现有 PostgreSQL
- 导出现有用户 quota 快照
- 冻结正式 Cutover 时间
- 编写带 `migration_batch_id` 的幂等清零脚本
- 在副本环境执行 Dry Run
- 验证 Active Quota / Reserve / Entertainment Wallet / Poker In Play 在初始赠金发放前全部归零
- 验证迁移初始赠金发放后，在没有其他账变时 API Credit 与 Total Assets 均为 1,000
- 准备全库回滚方案与迁移校验报告
- 记录现有 Compose / 环境变量
- 创建 `examples/deployment/platform`
- 创建 `chaldea_platform` 数据库
- 创建独立 DB 用户
- 建立开发 / 测试环境

### Phase 1 — 新 Chaldea 前台基础

- Public Home：Hero、真实 Platform Status、Featured Content、Rankings / Wins / Announcements Preview
- 新登录 / 注册流程 UI
- Discord / Password 同页登录与 Password Login Identifier 展示
- Discord Registration 资格说明、Server / Role 校验与既有账号识别
- OAuth Callback、账号、新用户 1,000 API Credit 初始赠金与 Profile 幂等
- Unified Access Gate 与安全 Return-to-Intent
- 全站 401 / 403 / 404 / 409 / 429 / 503、Loading、Processing 与 Empty State
- Dashboard
- Personal Hub / My
- Master Profile 与 Public Identity Preview
- Master 昵称唯一性、保留名称、敏感词和 7 天改名冷却
- System Default / Discord Avatar Snapshot
- Account & Security
- Set / Change Password 与 Discord Re-auth Reset
- Master Initialization
- Migration Notice Interstitial 与 Return-to-Intent 顺序
- 模型广场
- API Key 与 Usage Purpose
- API 使用记录与 Purpose Snapshot
- 基础 API Access 页面
- 公告列表与详情
- 置顶公告与人工排序
- `/` / `/login` Entry Popup
- Acknowledgements / 致谢名单基础版本
- 公告内容安全清洗

### Phase 2 — 钱包、奖励与周期补给

- API 额度显示适配
- 娱乐筹码钱包
- 1:1 双向兑换
- 账变流水
- 每日签到：固定 500 API 额度
- 每小时签到奖励：固定数量 100，资产类型与时间口径待确认
- 救济金：固定数量 300，资产类型待确认
- 破产资格判定：平台总资产严格少于 10
- 救济金领取成功后的滚动 4 小时冷却
- 奖励中心
- 管理员发放
- 奖励领取幂等
- 冷却与领取周期校验
- 资产发放统计
- 跨库兑换对账
- 异常奖励发放对账

### Phase 3 — 可扩展游戏目录与 V1 首发直接游玩游戏

- 公开 Game Catalog
- 动态 Game Registry
- 稳定 Game Slug 与 Entry Type
- 通用 Game Shell
- Direct Play 全局下注组件与策略版本
- Instant Resolve / Reveal Sequence / Multi-action Round 能力
- V1 首发：骰子
- V1 首发：刮刮乐
- V1 首发：扭蛋
- V1 首发：老虎机
- V1 首发：21 点
- Provably Fair
- 游戏配置版本
- 游戏运营后台
- 动态 Game History 筛选
- Round 恢复、维护与退款状态

### Phase 4 — Rankings、内容与 Chaldea Operations

- Assets & Games Rankings
- Game Profit 的 Today / This Week / All Time
- Biggest Win / Total Wagered / Poker Profit
- API Key General / RP / Unclassified Purpose
- RP Calls / Errors / Credits Consumed
- Model Filter 与模型分布
- RP Ranking Activation Time
- 5 分钟内近实时聚合与历史日/周快照
- My Rank 与个人 API Usage Cross-link
- 公共隐私边界
- Game History 筛选与 Session / Hand 层级
- 公共大奖播报
- Announcements & Events 完整工作流
- Chaldea Operations Shell、环境标识、Global Search 与 Needs Attention
- Super Admin / Operator / Auditor RBAC 与模块 Scope
- Chaldea Models Catalog 管理
- Users & Identity / Support Cases
- Games 配置 Draft / Validate / Preview / Activate
- Poker Tables / Sessions / Hands / Recovery / Chat Moderation
- Economy / Ledger / Transfer / Reconciliation / Admin Adjustment
- Rewards Configurations / Claims / Issuance Analytics
- Rankings Shadow Snapshot / Diff / Publish / Repair
- Records / Incidents / Fairness Verification
- Operations Service Health / Jobs / Maintenance / Incidents
- Access Control、危险操作分级与 append-only Audit
- PC / Tablet / Mobile Operations 响应式布局

### Phase 5 — 德州扑克 V1

- Cash Game
- 公开房
- 私人密码房
- 2–9 人
- 买入 / 离桌
- WebSocket
- 掉线重连
- 自动托管
- 观战
- 牌桌聊天
- 完整结算
- Provably Fair 洗牌
- 战绩

### Phase 6 — 视觉精修

可以与前面阶段并行推进，但正式定稿前进行独立 Art Direction：Moodboard、Design System、FGO / Chaldea 主题、页面动画、召唤演出、移动端、性能优化、浏览器截图评审。

### Phase 7 — 后续

- Sit & Go
- 更多游戏
- 更丰富的活动机制
- 媒体 CDN
- 服务拆分
- 管理后台统一化（如果未来确实需要）

---

## 35. 后续仍需继续讨论的内容

截至 v0.2.11，对应 v0.3.1 已完成 IA-01～IA-13，并已形成正式页面结构基线。

已完成的 Public Home、Login、Discord Registration、全站 Gate、跨模块流程、响应式页面家族和 IA 一致性审计，不再作为待设计项目。

以下内容仍不得由后续模型自行补全：

### 视觉与 Art Direction v0.4

- 精确色板；
- FGO / Fate 角色与素材选择；
- Logo；
- 字体；
- 图标；
- 背景图；
- 页面构图；
- 动画强度与分镜；
- 游戏视觉语言；
- 浏览器实渲染和截图评审；
- Design System。

### 奖励与救济剩余规则

以下数值已经冻结：

- 初始赠金：1,000 API Credit，新用户与迁移清零后的既有用户等额获得；
- Daily Check-in：固定 500 API Credit；
- Hourly Reward：固定数量 100；
- Relief Fund：固定数量 300。

以下内容仍待确认：

- Hourly Reward 发放 API Credit 或 Entertainment Chips；
- Hourly Reward 使用自然小时或滚动 60 分钟；
- Hourly Reward 是否累积与是否有每日限制；
- Relief Fund 发放 API Credit 或 Entertainment Chips；
- Relief Fund 未领取资格是否累积；
- Active Poker Session 中是否允许领取 Relief Fund；
- 奖励维护和临时关闭的最终业务规则；
- 是否允许通过新的版本化政策临时调整已冻结奖励金额；
- 奖励发行量运营预警阈值。

已冻结的奖励数量、`Total Assets < 10` 与救济金成功领取后滚动 4 小时冷却不得重新解释。

### 技术设计 v0.5

- OAuth 创建账号的稳定 Password Login Identifier 适配方式；
- NewAPI 受控密码重置接口；
- Session 与其他设备撤销能力核验；
- Route Gate、Return-to-Intent、OAuth State 与幂等实现；
- CSRF、XSS、Rate Limit、WebSocket 鉴权、请求签名与防重放；
- RBAC 技术权限矩阵和初始 Super Admin 建立方式；
- 数据库 Schema、迁移与回滚；
- 钱包一致性与 Reconciliation；
- Poker 状态恢复；
- 日志、监控、告警和数据保留；
- 域名、TLS、Nginx / Caddy、Compose、备份和资源限制。

### 后续产品扩展

- 新增游戏的独立规则与 IA；
- 公开用户主页；
- 自定义头像上传；
- 更高级账号安全；
- 法律 / 隐私 / Cookie 页面（如未来需要）；
- Public Status 页面（如未来需要）；
- 支付、充值或赞助支付页面（如未来需要）。

---

## 36. 当前已确认的关键结论速览

- NewAPI 是底层 Core，不深度塞入全部新业务。

- 普通用户完全使用新 Chaldea UI，不接触 NewAPI 原用户前台。

- FGO / 迦勒底是全站主视觉方向。

- Discord 指定服务器 + 指定 Role 才能首次注册。

- 注册成功后支持密码登录和 Discord OAuth 登录。

- Discord Role 仅作为首次注册门槛；注册后即使失去 Role 或退出服务器，也不影响已存在的站内账号。

- Discord 身份与 NewAPI 用户保持一对一绑定。

- Chaldea 不保存第二份用户密码；密码验证与密码修改继续由 NewAPI 账号系统负责。

- Master Identity、Account Identity 与 Authentication 必须分离；Master 昵称不是登录用户名或密码登录标识。

- V1 Master Profile 只包含 Master 昵称与展示头像，不加入 Bio、签名、生日、地区、社交链接或公开个人主页。

- Master 昵称全平台唯一，使用 NFKC、空白规范化和 Unicode Case Fold 进行唯一性比较，长度为 1–24 个可见 Grapheme，V1 不允许 Emoji。

- Master 昵称使用服务端敏感词、保留名称和身份冒充校验；用户主动改名后进入 7 天冷却，管理员强制改名必须通知并审计。

- V1 Avatar Source 仅为 System Default 与用户主动同步的 Discord Avatar Snapshot；不开放自定义头像上传，不自动跟随 Discord 头像变化，所有头像使用静态表现。

- Rankings 使用当前 Master Profile；Recent Wins、历史业务事件、Table Chat 与 Poker Session 使用对应时点的身份快照；真实归属始终使用 `newapi_user_id`。

- Account & Security 显示只读 Password Login Identifier、短 Account ID、Discord Connection、Password Status、Current Session 与 Account Status。

- V1 不提供自助 Discord Unlink 或 Rebind；Legacy Account 无 Discord 时可继续密码登录，补绑定和冲突处理必须由管理员辅助并完整审计。

- 已绑定 Discord 的用户可以通过 Fresh Discord Re-authentication 重置忘记的密码，必须校验返回 Discord User ID 与原绑定一致；设置或重置密码要求 10 分钟内 Fresh Authentication。

- V1 不增加 Email / Phone Recovery、TOTP、Passkey、Backup Codes、设备管理中心或普通用户自助硬删除账号。

- 首次认证后幂等创建 `INCOMPLETE` Provisional Master Profile；Master Initialization 使用不可跳过的单页流程确认昵称与头像，保存失败后下次登录恢复，不能重复创建 Profile 或初始赠金。

- 迁移用户在 Master Initialization 后确认版本化 Migration Notice；确认后恢复合法 Return-to-Intent，否则进入 Dashboard；普通 Post-login Popup 排在其后。

- 存在 Active Poker Session 时 Logout 必须警告，但不得自动 Safe Leave / Cash Out；Logout 也不得取消 Wallet、Reward、Game Round 或 API 的服务端处理。

- 初始赠金固定为 1,000 API 额度；新注册用户获得一次，迁移既有用户在旧额度清零后通过迁移批次等额获得一次。

- 每日签到固定获得 500 API 额度，不再使用 1,000–10,000 随机范围。

- 每小时签到奖励固定数量为 100，不采用随机金额；每个用户每小时最多成功领取一次。

- 救济金固定数量为 300，仅在用户平台总资产严格少于 10 时允许领取；总资产恰好为 10 不属于破产。

- 救济金采用领取成功后开始计算的滚动 4 小时冷却；不采用 00:00 / 04:00 / 08:00 等固定时间窗口。

- 只有成功发放救济金才开始或重置冷却；资格不符、请求失败、回滚或未实际发放不得启动新的冷却。

- 每小时签到奖励仍待确认资产类型、自然小时或滚动 60 分钟口径、累积规则及每日限制。

- 救济金仍待确认发放资产类型、未领取资格累积规则、Active Poker Session 行为和管理员临时关闭 / 版本化调整能力；开发模型不得自行决定。

- 初始赠金、每日签到、每小时签到奖励、救济金和管理员发放均必须具备唯一业务 ID、正式领取记录和幂等保护，禁止重复发放。

- 每日签到按统一业务时区计算，默认采用 `Asia/Shanghai`；数据库必须通过唯一约束保证同一用户同一自然日只能成功签到一次。

- 每小时签到奖励和救济金必须使用服务端权威时间；多标签页、多设备、网络重试和并发请求不得造成重复领取。

- API 计价与标准美元模型价格保持 1:1，但普通用户界面不显示美元符号。

- 当前目标换算关系：
  - `1 API 额度 = 1 USD 标准模型计价单位`
  - `1 API 额度 = 500,000 NewAPI raw quota`
  - `1 NewAPI raw quota = 0.000002 API 额度`

- `QuotaPerUnit = 500,000` 属于核心账务参数，不允许在生产环境运行期间随意修改。

- NewAPI raw quota 必须始终为整数，不允许出现浮点 quota。

- API 资产、NewAPI raw quota、娱乐筹码、下注金额、派奖金额和钱包余额均禁止使用 `float32 / float64` 作为最终账务存储格式。

- 所有资产统一使用整数原子单位进行存储和计算。

- API 额度采用 **Active + Reserve 双层结构**：
  - NewAPI quota 作为 **Active API Quota**，负责实际 API 请求预扣、结算和消费；
  - Chaldea Platform 保存 **Reserve API Credit**，用于保存超出 NewAPI Active Quota 安全范围的 API 资产；
  - 用户前端只看到统一的“API 总额度”，不感知 Active / Reserve 的内部区别。

- 用户 API 总资产由：
  - `Reserve API Credit`
  - 加上 `Active NewAPI Quota ÷ QuotaPerUnit`
  共同构成。

- NewAPI quota 不再承担用户全部 API 资产的无限容量存储，仅作为实际 API 消费层。

- 现有 NewAPI 用户迁移时，账号、Discord、密码、API Key、渠道、模型、倍率和历史调用记录继续保留；迁移范围内旧用户的 Active NewAPI Quota 统一清零，Reserve、娱乐钱包与 Poker In Play 先归零。

- 清零校验后，迁移批次向每个既有用户幂等发放 1,000 API Credit 初始赠金；发放完成且无其他账变时，用户 API Credit 与 Total Assets 均为 1,000。该笔赠金与新用户等额，但使用独立迁移业务 ID。

- 清零必须在维护窗口中通过唯一 migration_batch_id、迁移前快照、幂等脚本、全量校验和回滚方案执行，不允许通过普通后台逐个手工改余额。

- API 额度与娱乐筹码按 1:1 双向兑换。

- API 额度与娱乐筹码之间兑换不收取手续费。

- 不允许用户之间转账娱乐筹码。

- 不允许用户之间赠送娱乐筹码。

- 不建立娱乐筹码交易市场。

- 跨 NewAPI 与 Chaldea 的资产操作必须使用全局唯一 `biz_id / transfer_id`。

- 跨数据库资产操作必须采用幂等状态机、补偿机制和对账机制，不能假定两个 PostgreSQL Database 之间的普通事务具有跨库原子性。

- 跨库资产交易至少需要支持：
  - `PENDING`
  - `SOURCE_DEBITED`
  - `TARGET_CREDITED`
  - `CONFIRMED`
  - `COMPENSATING`
  - `COMPENSATED`
  等状态。

- 必须提供 Reconciliation / 对账机制，能够发现并修复长时间未完成的资产交易。

- 所有资产账变必须进入 append-only ledger。

- 钱包余额修改与对应 ledger 写入必须处于同一数据库事务中。

- 已完成的历史账变不得直接 UPDATE 或 DELETE。

- 错误账务必须通过新增反向账变或调整账变进行修正。

- 所有资产操作必须具备幂等性，禁止同一 `biz_id` 重复扣款、重复派奖或重复到账。

- 扣款必须采用数据库行锁、原子条件更新或等价并发安全机制，禁止仅依赖“先查询余额、再普通 UPDATE”的逻辑。

- 玩家钱包不得因并发请求出现负数余额。

- 管理后台不得直接修改最终 wallet balance 字段，管理员资产调整也必须通过统一账务服务生成正式流水。

- 平台游戏使用独立娱乐筹码体系。

- 单人游戏采用无限庄家模型。

- 单人游戏玩家输钱时由系统回收筹码，玩家中奖时由系统发行对应筹码。

- 不设置正常玩家每日最大盈利。

- 不设置正常玩家每日最大兑换。

- 不人为限制玩家通过游戏获得的娱乐筹码兑换回 API 额度。

- 所有允许玩家从娱乐主钱包选择单局基础下注金额的 Direct Play 游戏，统一使用全局下注策略。

- Direct Play 最低下注统一为 10 娱乐筹码。

- Direct Play 不设置产品层固定最高下注，但实际下注不得超过可用娱乐筹码和服务端安全范围。

- Direct Play 固定提供 10 / 100 / 500 / 1000 四个快捷金额。

- 快捷金额只负责选择或填入下注值，不直接扣款、开始游戏或创建 Round。

- 单个 Direct Play 游戏不得自行覆盖为不同最低下注或固定最高下注；如未来调整，必须使用新的全局、版本化下注策略。

- Poker 与其他使用 Buy-in、Blind、Ante、Base Score、Raise 或 Table Stack 的大厅型多人游戏不套用 Direct Play 全局下注策略。

- 平台当前非游戏性奖励来源包括初始赠金、每日签到、每小时签到奖励、救济金和管理员发放。

- API 额度 ↔ 娱乐筹码兑换属于存量资产形态转换，不属于新增发行。

- 单人游戏中奖派奖属于游戏结算产生的娱乐筹码发行，单独按照游戏 Round 和平台无限庄家规则记录，不属于非游戏性奖励来源。

- 由于娱乐筹码可以 1:1 兑换为 API 额度，通过每小时签到奖励或救济金发放的娱乐筹码仍属于平台新增可兑换资产。

- 总资产包括 API 总额度、娱乐钱包筹码、仍归玩家所有的 Poker table_stack，以及未与其他余额重复计算的处理中资产。

- Poker table_stack 从钱包转入牌桌不构成资产减少。

- Poker 正式盈亏排行榜统计以玩家离桌并完成 Cash Out 为确认点，牌桌内实时波动属于未实现盈亏。

- V1 一个用户同时只能在一张 Poker Table 入座并持有一个 Active Poker Session；完成 Safe Leave / Cash Out 后才能在另一张桌入座。

- V1 不提供其他用户的公开 Master Profile 页面。

- Master Profile 与 Account & Security 为两个独立页面。

- 首次进入 Chaldea 且尚未初始化 Master Profile 的用户，需要完成 Master 昵称 / 头像初始化后进入 Dashboard。

- Poker Table 使用独立沉浸式布局，不显示普通网站完整导航。

- 所有游戏采用 **Server Authoritative** 模型。

- 客户端不得决定：
  - 开奖结果
  - 随机数
  - 派奖金额
  - 输赢状态
  - 最终钱包余额
  - 游戏结算结果

- 所有下注验证、随机结果、赔率读取、派奖计算和最终结算必须在服务端完成。

- 每个游戏 Round 必须拥有唯一 `round_id`。

- 同一个 Round 只能完成一次最终结算。

- 游戏采用 Provably Fair 可验证公平机制。

- Provably Fair 至少包含：
  - Server Seed
  - Server Seed Hash
  - Client Seed
  - Nonce
  - Round ID
  - algorithm_version
  - game_config_version
  - game_config_hash

- Server Seed 必须使用密码学安全随机数生成。

- Server Seed 必须在接受下注前提交 Hash。

- 当前尚未 Reveal 的 Server Seed 不得写入普通日志。

- Seed Reveal 后不得继续用于新的游戏 Round。

- Nonce 在同一 Seed 生命周期内不得重复。

- 随机数映射必须避免明显的 modulo bias。

- 21 点和德州扑克必须在一手开始前确定整副牌的洗牌顺序，不允许发牌过程中临时重新生成随机结果。

- 游戏运营参数支持完整后台配置。

- 游戏运营后台至少支持：
  - 游戏启停
  - 维护状态
  - Direct Play 全局下注策略与版本
  - 赔率
  - 概率
  - RTP
  - 活动倍率
  - 免费次数
  - 每日奖励
  - 游戏公告

- 游戏配置必须版本化。

- 每个游戏 Round 创建时锁定对应 `game_config_version`。

- 后台修改游戏参数后，只影响之后创建的 Round。

- 历史 Round 必须保留当时真实使用的配置版本。

- 游戏透明度按游戏独立配置。

- 后台可以分别控制是否向用户展示：
  - RTP
  - 赔率
  - 掉落概率
  - 完整权重表
  - Provably Fair 验证信息

- V1 首发游戏清单只定义第一阶段范围，不构成平台永久游戏数量上限。

- 平台提供独立、公开可浏览的 Game Catalog；Entertainment Hub 负责运营推荐，Game Catalog 负责完整目录。

- 游戏目录、导航、历史和运营后台通过 Game Registry 动态生成，不得在代码或后台 Sidebar 中写死固定五款游戏。

- 每款游戏使用稳定 `game_slug`；展示名称变化不得破坏 Deep Link、历史记录或配置关联。

- Game Catalog 支持 Direct Play、Lobby、Resume、Maintenance、Coming Soon 等不同进入语义。

- 游戏发布状态与运行状态必须分离；维护主要阻止新 Round，已接受 Round 必须完成、恢复、托管或退款。

- 直接游玩游戏复用通用 Game Shell，但按照 Instant Resolve、Reveal Sequence、Multi-action Round 等能力适配，不要求相同视觉布局。

- 页面刷新、断线或网络超时后，已被服务端接受的下注必须恢复同一 Round，不得创建重复 Round。

- Game History 使用动态 Mode / Game Filter，并支持 Round、Session、Hand 等不同记录粒度。

- 新增完整游戏仍需实现规则、服务端权威结算、钱包集成、必要的 Provably Fair、前端交互与测试；Game Registry 不提供无代码生成完整游戏的能力。

- V1 首发游戏范围包括：
  - 老虎机
  - 骰子猜大小
  - 21 点
  - 刮刮乐
  - 扭蛋
  - 德州扑克

- 扭蛋机采用纯筹码奖励模式。

- V1 不做 Servant 收藏系统、灵基图鉴、卡牌养成和收藏品交易。

- Poker V1 = Cash Game。

- Poker V2 = Sit & Go 单桌锦标赛。

- Poker V1 暂不实现大型多桌锦标赛。

- Poker 采用买入锁定筹码模型。

- 玩家进入 Poker 牌桌时，从娱乐钱包扣除买入金额并转入独立 `table_stack`。

- Poker 牌局进行期间只修改牌桌筹码，不频繁修改娱乐主钱包。

- 玩家离桌时，将最终 `table_stack` 统一退回娱乐钱包。

- Poker 属于玩家之间的零和筹码转移。

- Poker V1 不收取 Rake。

- 在不收取 Rake 的前提下，Poker 牌局本身不得凭空创建或销毁筹码。

- Poker 必须正确处理：
  - Main Pot
  - Side Pot
  - All-in
  - Split Pot
  - 平局
  - Odd Chip
  - 掉线重连
  - 服务重启
  - 重复结算

- 同一 Poker Hand 只能进行一次最终 Settlement。

- Poker Service 重启后必须能够根据 PostgreSQL 数据恢复未完成牌局的关键状态。

- 任何真实资产和正式牌局结算结果不得只保存在 Redis。

- Redis 只用于缓存、会话、WebSocket 状态、牌桌临时状态和锁。

- PostgreSQL 是娱乐资产、账变和正式牌局数据的最终持久化来源。

- Rankings 使用唯一 `/rankings` 跨产品域页面，一级局部导航为 `Assets & Games | RP Usage`。

- Assets & Games 至少包含：
  - Total Assets
  - Game Profit（Today / This Week / All Time）
  - Biggest Win
  - Total Wagered
  - Poker Profit

- RP Usage 至少包含：
  - Calls
  - Errors
  - Credits Consumed

- RP 请求通过 API Key 的 Usage Purpose = RP 判断；新 Key 必须选择 General 或 RP，既有 Key 初始为 Unclassified。

- Key Purpose 只影响统计归类，不改变权限、路由或计费；用途修改不追溯历史请求。

- RP 排行从明确 Activation Time 开始，不回溯旧日志；周期统一使用 Asia/Shanghai，一周从星期一 00:00 开始。

- RP 排行允许未登录公开浏览聚合结果，但不公开 Key、Prompt、Response、Request ID、具体时间、原始错误、IP、User-Agent 或渠道信息。

- 排行榜中的游戏盈利只统计游戏净收益，不包含：
  - 初始赠金
  - 每日签到
  - 每小时签到奖励
  - 救济金
  - 管理员发放
  - API 额度与娱乐筹码兑换
  - Poker Buy-in / Cash Out 中不产生所有权变化的内部资产移动

- 用户公开展示 Master 昵称 + 展示头像。

- 用户默认参与排行榜。

- 完整个人游戏历史只有本人和管理员可查看。

- 公共区域只展示排行榜、大额中奖、最近大奖和适合公开的精选记录。

- 不做全站公共聊天室。

- 只保留 Poker 牌桌内聊天。

- 模型广场采用 NewAPI 自动同步 + Chaldea 元数据增强。

- NewAPI 负责提供真实模型 ID、状态、倍率、价格等数据。

- Chaldea 负责补充展示名称、简介、Logo、标签、推荐用途、排序等前台元数据。

- API Access 页面只做基础版。

- 不做大型 API 教程中心。

- 不做在线 Playground。

- 全站必须完整响应式。

- PC、平板和手机均需完整可用。

- Poker 必须针对手机竖屏和横屏设计专门布局。

- 复杂动画需要根据设备性能和 `prefers-reduced-motion` 自动降级。

- Announcements & Events 采用完整内容系统。

- 公告类型包括：
  - System
  - New Models
  - Game Events
  - Maintenance
  - Important
  - Acknowledgements / 致谢

- Pinned、Entry Popup、Post-login Popup、Public Home Banner 与 Dashboard Summary 为彼此独立的 Placement。

- 未登录用户进入 `/` 或 `/login` 时可以展示一条当前有效 Entry Popup；普通公共 Deep Link 不强制弹出致谢公告，入口公告加载失败不得阻止页面访问。

- V1 同一时间最多启用一条 Entry Popup；公告列表可以存在多条置顶公告，并由管理员手动排序。

- Entry Popup 按 `announcement_id + notification_revision` 在每个浏览器默认只显示一次；刷新、登录失败返回或同会话路由切换不得重复弹出。

- Entry Popup 为非阻断式，不要求倒计时、滚动到底或确认同意；关闭弹窗不等于已读。

- 已登录阅读状态由服务端保存并跨设备同步；匿名关闭状态只保存在当前浏览器。

- Acknowledgements 使用一条长期、公开、置顶的规范公告维护结构化 Sponsor / Contributor List，不新增独立 `/sponsors` 页面。

- V1 默认不公开赞助金额，不自动生成赞助等级，不公开支付账号、交易流水、Discord ID、邮箱或未经同意的真实身份信息。

- Markdown / Rich Text 必须经过统一安全清洗，不允许任意 Script、事件属性或 iframe。

- 已发布公告更新必须明确选择“只更新内容”或“发布新通知展示版本并 Re-notify”；已发布公告不进行无审计硬删除。

- 管理员继续使用 NewAPI 原管理员后台管理 NewAPI 原生能力。

- 新建 Chaldea 运营后台，管理游戏、奖励中心、娱乐筹码、公告、排行榜、游戏记录、账变和业务审计。

- 现有 NewAPI 账号身份、配置和历史记录原则上保留；用户旧 quota 余额是唯一明确要求在迁移切换时统一清零的数据。

- 现有用户无需重新注册。

- 现有 Discord 绑定无需重新绑定。

- 现有密码继续有效。

- 现有 API Key 继续有效。

- 现有渠道、模型、倍率和调用记录原则上全部保留。

- 当前基础设施采用 PostgreSQL + Redis + Docker Compose。

- 数据层采用同一 PostgreSQL 实例、逻辑数据库隔离的设计：
  - `newapi`
  - `chaldea_platform`

- NewAPI 原业务数据继续保存在 `newapi`。

- Chaldea 新业务数据保存在 `chaldea_platform`。

- Chaldea 与 NewAPI 通过稳定的 `newapi_user_id` 建立关联。

- NewAPI 与 Chaldea 建议使用不同数据库账号和最小权限原则。

- `examples/deployment/external-newapi` 保持现有 NewAPI 项目与部署结构。

- `examples/deployment/platform` 存放新的 Chaldea Platform 前端、后端、Poker Service、配置和迁移脚本。

- V1 优先采用单 VPS 部署。

- 不在 V1 过度拆分微服务。

- V1 推荐采用：
  - React / Vite 前端
  - Go Platform Backend
  - 独立 Go Poker WebSocket Service
  - PostgreSQL
  - Redis
  - Nginx / Caddy
  - Docker Compose

- 推荐生产环境为 **8GB RAM VPS**。

- 架构应保持在 **4GB RAM 环境下经过合理资源限制仍可运行**。

- 第一阶段预计 **10–50 人同时在线**。

- 第一阶段无需多服务器分布式部署。

- 后续规模增长时，可以逐步独立迁移：
  - PostgreSQL
  - Redis
  - Poker Service
  - NewAPI
  - 静态媒体 / CDN

- FGO / 迦勒底视觉属于一级产品需求，不是上线后再补的装饰。

- 精确色板、字体、背景、角色素材、动效、Hero 构图和 Design System 暂不在需求阶段锁死。

- 正式开发前需要单独进行 Art Direction / 视觉设计阶段。

- 视觉设计流程建议采用：
  - 参考素材收集
  - Moodboard
  - 多套视觉方案
  - 浏览器真实渲染
  - 截图评审
  - Design System 定稿
  - 再进行批量页面开发


- 平台继续采用 NewAPI Admin + Chaldea Operations 双后台，两套管理员身份和权限不自动互通。

- Chaldea Operations 一级模块为 Overview、Models、Users & Identity、Games、Poker、Economy、Rewards、Rankings、Records、Announcements & Events、Operations、Access Control、Audit。

- V1 使用 Super Admin、Operator、Auditor 三种基础角色；Operator 按模块 Scope 授权，Auditor 只读。

- 资产 Adjustment、Discord Rebind、Access Control、全站 Maintenance、Poker Emergency Pause 和经济配置激活仅允许 Super Admin。

- 所有后台权限必须由服务端校验，隐藏菜单不能代替授权。

- Operations Shell 必须持续显示 Production / Staging / Development 环境，支持 Global Search、Needs Attention 和稳定 Deep Link。

- Needs Attention 的 Acknowledge 只代表已查看，不代表问题已经解决；金融和牌局异常不得被永久隐藏或删除。

- Chaldea Models 只管理前台元数据和发布状态，NewAPI 原始模型、渠道、倍率和计费继续由 NewAPI Admin 管理。

- Users & Identity 负责 Master Moderation、Support Case、Binding Conflict 和 Migration Acknowledgement，不直接编辑余额、密码或 NewAPI Account Status。

- Active Game Config 不可直接编辑；必须 Clone as Draft、Validate、Preview 并发布新版本。

- Poker Operations 不得修改 Stack、Pot、赢家、牌序或 Settlement，也不得提前查看未公开 Hole Cards 和 Server Seed。

- Wallet 与 Ledger 在后台默认只读，不提供直接修改最终 Balance 的输入框。

- Reconciliation 优先由 Worker 自动处理；管理员只能执行状态机允许的 Retry、Resume、Compensate 或 Mark for Review。

- Admin Adjustment 只允许 Super Admin，必须有 Reason、Reference、Before / Delta / After、Fresh Authentication、Typed Confirmation、Ledger 和 Audit。

- Rewards 不能把失败 Claim 直接改成 SUCCESS；人工补发进入 Economy Adjustment，不伪造签到记录。

- Rankings 不能直接编辑用户分数；使用 Shadow Snapshot、Diff Review 和 Publish 流程修复。

- Records 默认只读；异常通过 Incident 进入 Economy / Poker Repair，不直接修改历史结果。

- Operations 不提供 SSH、Shell、SQL Console、Redis Console 或 VPS 管理能力。

- Maintenance 必须范围化、展示影响并保护已接受 Round、Poker Hand、Transfer 和奖励发放。

- 危险操作分为 Routine、Impactful、Critical；Critical 必须 Fresh Authentication、Reason、Typed Confirmation、Impact Preview、Operation ID 和 append-only Audit。

- Audit 不允许编辑或删除，金融撤销通过新的反向账变或补偿操作完成。

- V1 不强制双人审批；未来管理员团队扩大后再评估 Four-eyes Approval。


- Public Home 使用品牌与产品入口混合型结构；`/` 保持公共首页，登录用户访问时不强制跳转 Dashboard。

- Public Home 不复制个人资产、奖励或 Active Session；Featured Models、Featured Games、Poker、Rankings、Recent Wins 与 Status 只使用真实配置和真实数据，没有内容时隐藏模块。

- Critical Notice、Entry Popup、Home Banner、Post-login Popup 和 Dashboard Summary 为不同展示层；V1 不使用自动轮播 Home Banner，也不自动播放视频或声音。

- `/login` 只服务已有账号，同页展示 Discord Login 与 Password Login；密码登录使用稳定 Password Login Identifier，不使用 Master 昵称或短 Account ID。

- `/register` 是 Discord 首次注册说明和 OAuth 启动页，不提供传统用户名、密码、Email、Phone、Master 昵称或 Avatar 注册字段。

- 已绑定现有账号的 Discord 再次进入注册流程时转为登录现有账号，不创建第二个账号或重复奖励；异常 Binding Conflict 进入支持流程。

- OAuth Callback、Server / Role 验证、账号创建、新用户初始赠金和 Provisional Profile 必须幂等；验证失败不创建账号或奖励。

- Discord OAuth 创建的 NewAPI 账号必须拥有稳定 Password Login Identifier；如果现有能力不足，技术设计必须补适配，不得由前端猜测。

- 全站访问顺序统一为 Route Classification → Authentication → Account Status → Master Initialization → Migration Notice → Permission → Resource Availability → Return-to-Intent / Safe Parent → Deferred Post-login Popup。

- Return-to-Intent 只允许安全站内路径，具有有效期，使用后清除，只恢复页面和筛选，不自动重放任何副作用操作。

- 全站统一 401 / 403 / 404 / 409 / 429 / 503、Loading、Processing、Network Uncertain、Empty State 与通知层级；资产结果不能只依赖 Toast 或未经确认的乐观余额。

- PC / Tablet / Mobile 页面家族、复杂筛选、详情返回状态和可分享 URL 状态已完成统一审计。

- v0.3 Information Architecture 已完成 IA-01～IA-13 并升级为 FINAL；后续先由用户审阅，再进入 Art Direction v0.4。

## 37. 已确认的页面架构前置决策

以下内容已经完成产品确认，后续 Information Architecture、UX Flow、页面设计和实现不得擅自改变。

### 37.1 登录后的主入口与统一 Gate

Dashboard 为已登录用户的主要 Home / Command Center；未登录用户访问 `/` 时进入 Public Home。登录用户访问 `/` 不强制重定向 Dashboard。

全站认证后顺序统一为：

```text
Authentication
→ Account Status Gate
→ Master Initialization（如需要）
→ Migration Notice（如需要）
→ Role / Scope Permission Check
→ Resource Availability / Maintenance Check
→ 合法 Return-to-Intent
→ 或 Dashboard / Safe Parent
→ Deferred Post-login Popup（安全页面）
```

受限账号不进入普通 Dashboard 或 Master Initialization。

合法 Return-to-Intent 的优先级高于默认 Dashboard，但只恢复安全站内页面，不自动重放任何副作用操作。目标已下线、维护或无权限时，显示原因并进入安全父页面。

---

### 37.2 PC 全站导航模式

普通用户前台采用：

**Global Header + 产品域 Context Navigation**

不采用传统后台式的全站永久固定左侧 Sidebar 作为普通用户主要导航方式。

Global Header 负责产品域级别导航。

进入 API、Assets、Entertainment 等产品域后，再通过 Context Navigation 访问该产品域内部页面。

Chaldea Operations 管理后台不受该限制，可以采用更适合后台管理的 Sidebar 导航。

---

### 37.3 Wallet 与 Rewards

Wallet 与 Rewards 同属于 Assets / 资产产品域。

两者保持为独立页面。

Wallet 负责：

* API 额度展示；
* 娱乐筹码展示；
* API 额度 ↔ 娱乐筹码兑换；
* 资产账变与交易记录。

Rewards Center 负责周期奖励与救济体验，包括：

* 每日签到；
* 每小时签到奖励；
* 救济金；
* 奖励状态与领取历史。

救济金页面与 Dashboard 必须使用同一服务端资格结果：平台总资产严格少于 10 且滚动 4 小时冷却结束时才可领取。领取成功后重新开始 4 小时倒计时。

不得仅将 Rewards Center 降级为 Wallet 页面中的一个普通小按钮。

---

### 37.4 Master Profile、Account & Security 与 Onboarding

Master Profile、Account Identity 与 Authentication 为三个不同概念。

Master Profile 只负责 Master 昵称、展示头像和用户自己的公开身份预览；Master 昵称不改变 NewAPI 登录名、Discord 绑定、API Key、Wallet 或历史归属。

V1 Master 昵称全平台唯一，使用 Unicode 规范化进行唯一性校验，长度 1–24 个可见 Grapheme，不允许 Emoji；用户主动改名有 7 天冷却。

V1 Avatar Source 仅为 System Default 与 Discord Avatar Snapshot，不开放自定义头像上传。

Account & Security 负责只读账号身份、Discord Connection、Password Set / Change / Discord Reset、Current Session 与 Account Status。Chaldea 不保存第二份密码。

V1 不提供其他用户公开 Profile、自助 Discord 解绑 / 换绑、邮箱或手机号找回、TOTP、Passkey、设备中心或用户自助硬删除。

Master Initialization 为不可跳过、幂等的单页流程。迁移用户完成初始化后确认版本化 Migration Notice，再恢复合法 Return-to-Intent 或进入 Dashboard。

---

### 37.5 Poker Table 沉浸模式

Poker Lobby 属于正常 Chaldea 网站信息架构。

用户进入实际 Poker Table 后切换至独立的沉浸式牌桌布局。

Poker Table 不继续展示普通网站完整 Global Header、Context Navigation 或移动端 Bottom Navigation。

牌桌仅保留必要全局控件，包括：

* 返回大厅；
* 当前娱乐钱包 / 牌桌筹码；
* 网络连接状态；
* 牌桌设置；
* 退出牌桌。

Poker Table 的座位、Pot / Side Pot、行动按钮、聊天、观战、重连、Safe Leave 与 PC / Mobile 布局已经在 IA-08 完成冻结；具体视觉表现留到 Art Direction v0.4。

---

### 37.6 手机端一级导航

手机端不简单将 PC Global Header 缩减为汉堡菜单作为唯一导航方式。

登录后的主要移动端导航采用 Bottom Navigation。

当前确认的五个一级入口为：

1. 首页
2. 模型
3. 娱乐
4. 资产
5. 我的

其中：

**首页**
→ Dashboard

**模型**
→ Model Square

**娱乐**
→ Entertainment Hub

**资产**
→ Wallet，并通过 Assets Context Navigation 提供 Rewards Center 入口

**我的**
→ API Key、API Usage、Announcements、Master Profile、Account & Security 等个人和低频功能入口。

Poker Table 进入沉浸模式后隐藏 Bottom Navigation。

未登录手机端不显示登录后的 Bottom Navigation，使用紧凑 Public Header / Menu 访问 Public Home、Models、Entertainment、Announcements、Login 与 Discord Registration。

---

### 37.7 页面产品域

普通用户信息架构原则上按照以下产品域组织：

* Public
* Dashboard / Command Center
* Models & API
* Assets
* Entertainment
* Information / Announcements
* Master / Account

管理员区域独立为：

* Chaldea Operations
* NewAPI Admin

普通用户不得进入 NewAPI 原始用户前台。

---

### 37.8 可扩展游戏架构

V1 首发清单与平台长期游戏容量分离。

平台采用：

- Entertainment Hub：运营推荐、活动、Continue Playing 与重点入口；
- Game Catalog：全部已发布游戏的搜索、分类、筛选和状态展示；
- Game Entry：按照稳定 `game_slug` 进入直接游玩游戏；
- Lobby / Table：用于 Poker 及未来适合大厅型结构的多人游戏。

Entertainment Context Navigation 中的 Games 固定指向 `/games`，不直接罗列固定游戏名称。

分类、标签、游戏列表、推荐、发布状态与运行状态由 Game Registry 动态提供。

直接游玩游戏默认使用 Focused Game Layout；Poker Table 使用 Full Immersive Layout。未来其他游戏是否使用大厅或沉浸模式，必须按照对应游戏重新确认。

Game History 使用动态 Mode 与 Game Filter，不把 `All / Solo / Poker` 作为永久固定结构。

维护状态主要阻止新 Round；已经接受下注的 Round 必须完成、恢复、托管或退款。

新增完整游戏仍需代码、服务端结算、钱包集成、前端交互和测试，不能只通过运营后台表单生成。

---

### 37.9 Direct Play 全局下注体验

所有使用娱乐主钱包、由玩家选择单局基础下注金额的 Direct Play 游戏共享统一下注体验：

- 最低下注 10 娱乐筹码；
- 不设置产品层固定最高下注；
- 固定提供 10 / 100 / 500 / 1000 四个快捷金额；
- 快捷金额只用于选择或填入金额；
- 正式下注仍通过游戏主操作提交；
- 实际下注不得超过可用娱乐筹码与服务端安全范围；
- 单个 Direct Play 游戏不得覆盖为不同最低下注或固定最高下注。

Poker 与其他大厅型多人游戏继续使用各自的 Buy-in、Blind、Ante、Base Score、Raise、Table Stack 或等价规则。

Direct Play 主下注仅接受整数娱乐筹码；V1 不启用游戏专属 Free Round / Free Summon；Blackjack 等追加下注行为按 IA-07 已冻结的对应游戏规则执行。

---

### 37.10 Rankings Center 与 RP Usage

Rankings 保持唯一 `/rankings` 页面，但从单纯 Entertainment 榜单升级为跨产品域 Rankings Center。

一级局部导航：

- Assets & Games；
- RP Usage。

RP Usage 内部包含 Calls、Errors、Credits Consumed，并支持 Today / This Week / All Time 与 Model Filter。

公开榜单仅展示 Master 聚合信息，不公开任何 Key、Prompt、Response 或单次请求细节。

Rankings 不新增为 PC Global Header 一级入口；Entertainment、API Usage、Personal Hub 和 Public Home 可以提供 Cross-link。

---

### 37.11 迁移余额清零用户体验

既有用户首次进入 Chaldea 时，账号、Discord、密码和 API Key 保持可用，但旧 API 额度已经在 Cutover 中统一清零。

迁移用户先完成必要的 Master Initialization，再进入版本化 Migration Notice。Notice 使用 `我已了解，继续`，服务端保存确认版本和时间，不提供恢复旧额度按钮。

Notice 明确说明：

- 账号、Discord、密码、API Key 和历史 Usage 未删除；
- 旧额度已经清零，并已通过迁移批次发放 1,000 API Credit 初始赠金；
- 现有 Key 初始为 Unclassified；
- 该笔初始赠金不表示重新注册，也不得重复触发注册回调；
- 可以进入 Rewards、Wallet、API Keys 与 API Usage。

确认后存在合法 Return-to-Intent 时返回原目标，否则进入 Dashboard。普通 Post-login Popup 排在 Migration Notice 之后。

---

### 37.12 Announcements、Entry Popup 与 Acknowledgements

Announcements 保持统一的 `/announcements` 列表与 `/announcements/:id` 详情页面。

公告类型增加 Acknowledgements / 致谢，但不新增独立 Sponsor 一级页面。

Pinned、Entry Popup、Post-login Popup、Public Home Banner 与 Dashboard Summary 必须独立配置。置顶不自动等于弹窗。

未登录用户进入 `/` 或 `/login` 时可以看到当前有效的非阻断式 Entry Popup；普通公共 Deep Link 不强制展示致谢弹窗。V1 同一时间最多一条 Entry Popup。

致谢名单使用一条长期、公开、置顶的规范公告维护，并支持结构化 Sponsor / Contributor List、人工分组与排序、匿名赞助者以及可选头像、Logo、链接和致谢说明。

匿名用户的弹窗关闭状态保存在浏览器本地；登录用户的公告阅读状态保存到服务端。关闭弹窗与打开详情阅读必须作为不同状态。

普通 Post-login Popup 不遮挡 Poker Table、活动 Round 或 Wallet Processing，应延迟到 Dashboard 或安全普通页面。

具体色板、FGO 装饰、弹窗转场和视觉动效仍留到 Art Direction v0.4。

---

### 37.13 Chaldea Operations

Chaldea Operations 与 NewAPI Admin 保持双后台和独立权限体系。

Chaldea Operations 使用 Sidebar，按 Command、Catalog & Community、Economy & Data、Administration 分组，并包含：

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

V1 角色为 Super Admin、Operator、Auditor。Operator 通过模块 Scope 授权，Auditor 只读。关键资产、账号绑定、权限和全站维护操作仅允许 Super Admin。

后台列表采用稳定 URL、明确分页、可恢复筛选与 Breadcrumb。复杂详情使用正式页面，Drawer 仅用于快速预览。

所有经济、游戏、Poker、排行榜和记录对象默认不可直接修改历史事实。修复必须通过版本、状态机、补偿、Incident、重聚合或反向账变完成。

Operations 只提供业务级 Service Health、Jobs、Maintenance 和 Incidents，不提供服务器 Shell 或数据库 Console。

### 37.14 Public Home、Login 与 Discord Registration

Public Home 使用品牌与产品入口混合型结构，包括 Hero、真实 Platform Status、Models / Entertainment 路径、条件式 Featured Models / Games / Poker、Rankings Preview、Recent Public Wins、Announcements 和 Acknowledgements Entry。

`/login` 只服务已有账号，同页显示 Discord Login 与 Password Login；Password Login 使用稳定 Identifier。

`/register` 只负责 Discord 首次注册说明和 OAuth，不接受普通密码、Email、Phone、Master Profile 等注册输入。Server / Role 资格由服务端验证。

已绑定现有账号的 Discord 再次注册时转为登录现有账号。注册、奖励和 Profile 创建必须幂等。

---

### 37.15 全站状态、错误与响应式

全站统一 401、403、404、409、429、503，区分 No Data 与 Load Failure，并使用明确的 Loading / Processing / Confirmed / Needs Attention 状态。

网络结果不确定时查询原 Business ID，不直接重放写操作；资产、奖励和游戏余额以服务端确认为准。

Public Home、Login / Registration、Catalog、Dashboard / Wallet、Profile / Account、Direct Play、Poker、History 和 Chaldea Operations 均已冻结 PC / Tablet / Mobile 页面家族行为。Tablet 根据空间采用 Desktop 或 Mobile Pattern，不建立第三套导航。

---

### 37.16 v0.3 页面结构基线状态

截至 v0.2.11，对应页面结构文档已经完成 IA-01～IA-13，并通过奖励数值补丁升级为 `v0.3.1 FINAL`。

v0.3.1 FINAL 是后续 Art Direction、Page Layout、技术设计和实施规格的页面结构基线。任何改变已冻结路由、页面职责、权限、资产语义或关键 UX Flow 的内容，必须通过版本化变更重新确认。

每小时奖励数量 100 与救济金数量 300 已经冻结；其资产类型、Hourly 时间口径、累积 / 每日限制、Active Poker 行为等剩余运营参数继续保留在需求基线，不由视觉或实现模型擅自决定。

下一阶段：

```text
用户审阅 v0.3.1 FINAL
→ Art Direction v0.4
→ 技术设计 v0.5
→ 实施规格 v1.0
```

视觉色板、字体、角色素材、背景、动画、具体 FGO 视觉映射和 Design System 仍属于 Art Direction 阶段。
