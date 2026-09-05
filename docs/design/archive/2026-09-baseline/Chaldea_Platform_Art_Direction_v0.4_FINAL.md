# Chaldea Platform Art Direction v0.4 FINAL

> **历史参考归档（已脱敏）**：本文件的 FINAL / FROZEN 是历史状态，不代表当前实现或当前强制流程；现行决策优先。见[归档索引](../README.md)与[决策 0001](../../../decisions/0001-pragmatic-baseline.md)。`examples/` 路径仅为说明占位，相关部署、图片和私有文件不随仓库提供。

> 状态：**FINAL / 正式视觉设计基线**  
> 发布日期：2026-09-02  
> 上游产品基线：`Chaldea Platform 需求基线 v0.2.11`  
> 上游页面结构基线：`Chaldea Platform 全站页面结构设计 v0.3.1 FINAL`  
> Persona Prompt 档案：`Chaldea_Platform_Model_Persona_Image_Prompts_WORKING_v0.6.md`  
> 决策来源：`WORKING v0.37`，`AD-FRZ-001～582`  
> 文档用途：作为视觉实现、素材生产、前端 Design Token、页面表现、可访问性、媒体与 Rights Review 的唯一 Art Direction 基线。

## 0. FINAL 的含义

`v0.4 FINAL` 表示视觉设计决策阶段已经收口，不表示所有媒体资产已经生产完成，也不表示第三方 Rights / License 已自动取得。

后续实现不得重新打开已经冻结的：IA、路由、资产语义、Casino A、Bright Moonlit、Flat Button System、Royal Beacon、Semantic / Data Color、Responsive Token、Poker / Operations Shell、Layered Production Pack、九个 Model Persona Identity、Quiet Royal Motion 与 Accessible Production Gate。

如未来确实需要修改本 FINAL：

1. 不静默改写本文件；
2. 通过新的版本化 Art Direction Change Proposal；
3. 明确指出被替代条目、原因与影响；
4. 业务 / IA 变化先回到对应上游文档，不在视觉层偷偷修改产品规则。

---

# 1. 最终视觉定位

## 1.1 全站母方向

最终母方向为：

> **Chaldea Royal Observatory / 迦勒底皇家观测宫**

平台不是科技军控面板，不是普通企业 SaaS，也不是把 Fate 图片贴在 NewAPI 上。第一视觉印象应是：

- 明亮、洁净、皇家而克制；
- FGO / Chaldea 世界观存在感明显，但不依赖复制官方 UI；
- 现代 Web 产品结构清晰、长期可用；
- 技术语法只服务真实 API / Network / Fairness / Operations 职责；
- 角色、赌场与现象层负责情绪和世界观，业务信息继续由真实 DOM / SVG / Data UI 承担。

## 1.2 娱乐与游戏母方向

Entertainment / Direct Play / Poker 使用：

> **Casino Camelot Grand Resort / FGO Royal Casino Resort**

空间语言吸收澳门高端综合度假赌场的尺度与豪华感：Grand Atrium、Arch、Vaulted / Coffered Ceiling、Chandelier、Marble、Patterned Carpet、Mirror / Polished Metal、Warm White Lighting、Controlled Gold、Wine / Burgundy Scene Accent。

但借鉴的是**空间设计语法**，不是复制现实赌场品牌、Logo 或室内照片。

### 厅级映射

- Entertainment Hub → Grand Casino Atrium / Royal Casino Floor
- Dice → Lucky Dice Salon
- Scratch → Treasure Voucher / Prize Counter Salon
- Summon → Grand Manifestation Theatre
- Slot → King's Treasury Slot Gallery
- Blackjack → Casino Camelot VIP Royal Table
- Poker Lobby → High Roller Reception / Lounge
- Poker Table → Macau-style High Roller Poker Salon

Casino Luxury 由场景、物件、角色和灯光承担，不由 3D / Glossy / Bevel Web Button 承担。

## 1.3 三层表现体系

全站继续保留三层职责：

```text
Product / Command Structure
→ 信息、导航、表单、数据、状态、业务操作

Heroic / Character Layer
→ FGO-inspired 平台角色、游戏 Host、Model Persona Identity

Phenomenon Layer
→ Summon、Grand / Apex、短 Reaction、关键结果显现
```

人物和 Phenomenon 永远不能遮挡或改变权威业务结果。

---

# 2. Brand System — Royal Beacon

## 2.1 主 Logo

唯一主品牌图形为：

> **Royal Beacon / 皇家灵子观测环**

组成：

- 向右开放的不闭合双轨道；
- 轨道负形形成隐藏 `C`；
- 中央实心 Diamond Command Core；
- 外轨右上单一圆形 Arc Node。

Master Grid 使用 `100×100`：

```text
Outer Orbit stroke    5u
Outer opening         ~72°
Inner Orbit stroke    4.5u
Inner opening         ~92°
Outer optical rotate  -4°
Inner optical rotate  +3°
Command Diamond       14u × 14u
Arc Node diameter     8u
Arc Node angle        ~48°
```

双轨受控不对称，避免变成 Loading Spinner、Radar、Power Button 或网络拓扑。

## 2.2 Wordmark

正式 Wordmark：

> **CHALDEA PLATFORM**

- `CHALDEA`：基于 Marcellus 气质进行定制字标与光学校正；
- `PLATFORM`：Functional Sans 轻量大写，建议 Tracking 约 `0.18em`；
- 中文承担导航、页面标题、表单、状态和说明，不在常态 Header 重复堆完整中英文品牌名。

## 2.3 四级响应式 Logo

1. Primary Lockup：`[Royal Beacon] CHALDEA / PLATFORM`
2. Compact Lockup：`[Royal Beacon] CHALDEA`
3. Terminal Glyph：只保留 Beacon
4. Favicon：16px 主动简化为粗 C Arc + Diamond，Arc Node 可省略；32px 起恢复 Node

Clear Space：以 Diamond 宽度为 `X`，Logo 外部最小净空 `1.5X`；Beacon ↔ Wordmark 建议约 `1.25X`。

## 2.4 Logo 色彩与边界

- Bright UI：Royal Azure / Ink；
- 媒体或深色背景：Chaldea Ivory 单色；
- Logo 不依赖 Gradient / Glow / Particle；
- Casino / Event / Character Skin 不修改 Royal Beacon；活动另用 Command Crest / Ceremonial Sigil；
- 不复制官方 Chaldea、职阶、令咒、召唤阵、宗教符号、赌场徽章。

---

# 3. Color System

## 3.1 Bright Moonlit Core Neutral

```text
Ink             #17202D
Ink Soft        #596573
Paper           #FFFFFF
Surface         #FBFAF7
Surface Muted   #E9EBEE
Line            rgba(23, 32, 45, 0.16)
Line Strong     rgba(23, 32, 45, 0.34)
```

最终 UI 不再以墨蓝/黑色作为全站大面积主底。默认是明亮的月光白、象牙、浅灰蓝与清晰皇家蓝关系。

## 3.2 Button Palette — 只约束按钮

用户确认的“两种颜色 + 中间色”**只适用于 Button System**：

```text
Chaldea Ivory   #F4F0E8
Royal Azure     #3568B7
Moonlit Mid     #95ACD0
```

### Primary

```text
Default: Royal Azure fill + Ivory text
Hover:   Moonlit Mid fill + Ink text + Royal Azure border
```

### Secondary

```text
Default: Ivory/Transparent + Royal Azure border/text
Hover:   Moonlit Mid + Ink
```

Tertiary / Destructive / Game Action 仍继承相同按钮三色纪律。危险性通过文案、Icon、Impact Preview、Fresh Auth、Typed Confirmation、Audit 表达，而不是做红色拟物按钮。

所有按钮永久禁止：Gradient、Bevel、Emboss、Gloss、Inset Shadow、Hover Lift、Press Sink、Scale Press、Casino Chip Button。

## 3.3 Semantic Status Palette

| 语义 | Token | Hex |
|---|---|---:|
| Info / Link | Royal Azure | `#3568B7` |
| Running / Processing | Spiritron Teal | `#2B7783` |
| Success | Chaldea Jade | `#2F7D5A` |
| Warning / Maintenance | Royal Amber | `#9A620F` |
| Critical | Command Scarlet | `#B5484F` |
| Recovering / Compensating | Spirit Violet | `#6E5AA7` |
| Offline / Paused | Slate | `#6D7480` |

状态必须由 Icon + Shape + Text + Color 共同表达，Color-only 禁止。

Soft Status Surface 从主语义色以约 8–12% Alpha 派生，Border 约 25–35% Alpha；不为每个语义建立一组新的 Pale / Light Hue Token。

## 3.4 Data Visualization

Single Series 默认 Royal Azure。

Sequential Azure Scale：

```text
#E7EEF8
#C3D3EB
#95ACD0
#6F8FC3
#3568B7
```

Categorical 最多六个主色：

```text
#3568B7
#2B7783
#6E5AA7
#9A620F
#A34F78
#5C7A3D
```

超过六类优先用 Filter / Selector / Small Multiples，不继续扩张彩虹色。

Diverging 只有真实 Favorable / Unfavorable 或正负语义才使用 Scarlet ↔ Neutral ↔ Jade；“上升”不自动等于绿色，“下降”不自动等于红色。

---

# 4. Typography

```text
UI / Chinese Body       IBM Plex Sans SC
Technical / ID / Hash   IBM Plex Mono
Latin Display           Marcellus
Chinese Display         Noto Serif SC
```

全部 Self-host；生产优先 WOFF2，`font-display: swap`。

正文：

```text
Desktop 16px / line-height 1.6
Mobile  15px / line-height 1.6
```

数字资产、金额、游戏数值使用 Tabular Numerals；ID / Hash / Code 使用 Mono。中文为主要产品语言，英文只承担短技术词、Kicker 与适量仪式标题。

---

# 5. Surface / Component / Button System

## 5.1 Royal Minimal Frame

普通 Panel / Metric / Model Card / Game Card / Input Container / Navigation Surface：

```text
Solid Surface
+ 1px Outline
+ Spacing
```

普通内容不使用常态 `box-shadow`。Shadow 只用于 Drawer / Dialog / Popover 或真正 Overlay。

Radius：

```text
Small   8px
Medium  14px
Large   22px
```

赌场豪华材质由 Scene / Physical Object 承担，Web Card 不做金属浮雕或玻璃拟物。

## 5.2 Operation Hierarchy

保留 Primary / Secondary / Tertiary / Destructive / Game Action 五级职责；每个操作组只能有一个明确 Primary。Gold / Scarlet / Jade 等状态色不得扩张为普通按钮主题色。

## 5.3 Form / State

- 常驻 Label；
- Input 最小高度 46px；
- Focus 使用 3px Royal Azure Outline + 3px Offset；
- 错误必须有文字、关联字段和明确修复方式；
- Toast 不作为资产、奖励、游戏结算或危险操作唯一反馈；
- Network Uncertain 必须持久显示并阻止盲目重复提交。

---

# 6. Layout / Responsive / Navigation Tokens

## 6.1 Grid / Spacing

```text
Content Max Width        1200px
Desktop Gutter           24px / side
<=1100 Gutter            16px / side
<=720 Gutter             14px / side
Micro Rhythm             4px
Main Rhythm              8px
Page Stack Desktop       52px top / 28px gap
Page Stack Mobile        30px top / 20px gap
```

基础响应式网格：Desktop 12 列、Tablet 8 列、Mobile 4 列。

主断点：

```text
1100px
720px
420px
```

Tablet 在 Desktop / Mobile Pattern 之间切换，不建立第三套导航架构。

## 6.2 Navigation

```text
Global Header Desktop    72px
Global Header Mobile     60px
Context Navigation       46px
Mobile Bottom Navigation >=70px + safe-area
Standard Touch Target    >=44px
Input Height             >=46px
```

普通用户 PC：Global Navigation + Context Navigation + Local Navigation，不使用永久全站 Sidebar。

Mobile Bottom Navigation：

> 首页 / 模型 / 娱乐 / 资产 / 我的

Poker Table 隐藏普通 Header / Context / Bottom Navigation；Operations 使用独立后台 Shell。

## 6.3 Model Square Grid

```text
>1100px   3 columns
<=1100px  2 columns
<=420px   1 column
```

---

# 7. Data / Status / Feedback

数据优先真实性与可追溯：真实数值、单位、时区、更新时间、Freshness / Lag、状态、稳定 ID。

ID / Hash / Code 使用 Mono；负值不自动等同 Critical；类别颜色与状态颜色分离。

状态协议必须包含：Icon、形状、标题/Label、解释、颜色。关键写操作生命周期明确显示 Submitting / Processing / Completed / Failed / Recovering / Refunded 等对应语义。

Operations / Economy / Rankings / Jobs 等聚合模块高可见显示 Last Updated / Freshness / Lag。

---

# 8. Public / Auth

## 8.1 Public Home

Public Home 不永久绑定单一从者；以 Royal Observatory、Royal Beacon、Models & API / Entertainment 双路径与真实平台状态建立品牌记忆。

## 8.2 Login

- 角色：Artoria Saber，P3；
- Desktop：Scene 7 / Auth Panel 5；
- Auth Max Width：1360px；
- Desktop Scene Min Height：660px；
- <=1100：单列，Scene Min Height 500px；
- Mobile Scene Min Height：390px + 下方实体 Auth Panel。

Discord Login 位于密码登录上方作为主要方式；Password Login Identifier 不得伪装成 Email；Discord Registration 保持独立首次注册入口。

## 8.3 Registration

Mash P2–P3，聚焦 Required Server / Required Role / Discord OAuth；不收集 IA 未要求的用户名、Email、Phone、Master Nickname 等字段。

背景、人物、前景 Atmosphere 与真实 Web UI 永久分层，AI 图片不得烘焙按钮、Logo、表单、业务状态。

---

# 9. Dashboard

Dashboard 是任务与资产控制页，不使用巨大 Hero。

首屏优先：

1. Critical / Active Session Priority Dock（条件式）
2. Master Command Status
3. API Credit + Available Chips
4. Poker In Play / Processing（条件式）
5. Daily / Hourly / Relief Rewards
6. API Operations / Entertainment 双主线
7. Recent Activity / Announcements

Dashboard 对普通用户只展示统一 API Credit，不暴露 Active / Reserve 内部结构；Daily 500 API Credit，Hourly 100 数量但资产类型读取服务端，Relief 300 + Total Assets < 10 + rolling 4h，其他 TBD 不由视觉补全。

---

# 10. Models / Model Persona

Model Persona 是 Model Identity Asset，不是 FGO 从者、Master Avatar、Poker Avatar 或游戏 Host。

Model Card 固定：

```text
Identity
Summary
Attributes & Pricing
Persona Slot
Action Rail
```

Persona 不承担 Real Model ID、Availability、Pricing、Context 或 CTA；加载失败使用 Model Glyph / Family Geometry。

Model Detail：Desktop 信息 / Persona 约 7 / 5；Mobile 信息优先，空间不足先降低 Persona。

## 10.1 九个 Selected Family Master

全部已 `SELECTED / NOT PRODUCTION_READY`：

- DeepSeek
- GPT
- Claude
- Gemini
- GLM
- Kimi
- ERNIE / 文心一言
- Qwen / 千问
- Grok

当前 Selected Design Master v1 为 1024×1536 RGBA；正式生产前执行 Non-destructive Normalize，目标建议 2048×3072、2:3、Transparent Alpha、Full Body、Alpha Safety Margin >=5%，长发 / 尾巴 / 武器 / 宽裙摆建议 6–8%。

默认 Card / Detail 共享同一 Master，通过 Manifest focal point / crop safe 管理；不默认制作第二张半身 PNG。

Grok X-like Weapon 进入 Rights / Trademark Spot Check；如需要只微调敏感几何，不重做 Persona Identity。

---

# 11. Wallet / Rewards

Wallet 结构优先：Processing（条件）→ Total Assets → Asset Breakdown → Poker / Actions → Recent Transactions。

API Credit 与 Available Chips 等权呈现；Poker In Play / Processing 作为条件资产。Exchange 是双向 API Credit ↔ Entertainment Chips 1:1、Fee 0 的明确 Transfer Instrument，支持既有 atomic-unit 精度规则；确认前必须展示 Source / Target / Amount / Rate / Fee / After Balance。

Rewards Center 使用 Compact Reward Status Rail + 当前 Reward Detail，不做三张巨大领取卡；Reward History 位于本页局部区域。

---

# 12. API Usage

单一路径 `/api/usage`，内部使用 Overview / Request History。

Overview：所选周期 API Credit 消耗、Today Usage、一个真实趋势图与可切换 Breakdown；不堆图表墙。

Request History：真实筛选、时间、Model、API Key、Purpose、状态、Consumption 等；PC Detail 使用 Drawer / Side Panel，Mobile 使用 Full-screen Detail；不显示完整 API Secret，也不默认保存 Prompt / Response 私密内容。

---

# 13. Entertainment Hub / Rankings / History

Entertainment Hub 的视觉是 Grand Casino Atrium / Royal Casino Floor，但导航与 IA 不改变。

优先顺序：Active / Resume → Service / Assets → Featured / Continue / Browse → Multiplayer → Events / Rankings。无真实活动时不伪造在线人数、Jackpot Feed 或“刚刚有人中奖”。

Context Navigation：Hub / Games / Poker / Rankings / History。Games 进入动态 Game Catalog，不在导航中硬编码固定游戏名。

Rankings 使用真实指标、My Rank、Top 3 克制强调；History 使用真实 Round / Session / Hand Detail，不提供虚假 Replay。

---

# 14. Direct Play Shared Shell

Focused Game Layout 共享：

```text
Game Header
Status / Maintenance
Game Stage
Wager & Action
Current Round State
Result Summary
Rules / Transparency / Fairness
Personal History
```

精确 Stage Token：

```text
Entertainment Hero Desktop Min Height   620px
Entertainment Hero Mobile Min Height    560px
Game Stage Desktop Min Height            490px
Game Stage Mobile Min Height             340px
Console Overlap Desktop                  -72px
<=1100                                   -48px
<=720                                    -28px
```

权威业务 UI 留在 Safe Grid；媒体只调整 Crop / Focal Point，不破坏 Wager / Action / Result。

共享媒体槽：background_plate / character_idle / game_stage_asset / foreground_atmosphere / reaction_visual / reaction_audio / optional symbol/fx sets。

---

# 15. Dice — Lucky Dice Salon

- Astolfo：Stage Left，Lucky Companion，不是 Dealer；
- 三颗 Chaldea Ivory Dice；
- Big / Small 等权，不默认选择；
- Triple 只作为特殊结果表现，不伪造 Jackpot；
- Casino 氛围来自 Lucky Dice Salon，不改玩法为 Sic Bo；
- Roll / Settle 650–900ms，Result Reveal 180–240ms，常规总时长 <=1.1s；时长不得随结果价值变化。

---

# 16. Scratch — Treasure Voucher Salon

- Ishtar：Stellar Prize Host；
- 3×3 Scratch Card；
- Touch / Hold + Drag，单次点按不得直接揭示；
- Reveal All 固定 350–550ms，不按结果改变速度；
- 原创 Prize Sigils / Spiritron Foil，前端 Mask / SVG / Canvas，不烘焙结果；
- 0x / 1x / 2x / 3x / 5x / 10x / 25x / 100x 的正式业务语义继续服从上游奖表。

---

# 17. Summon — Grand Manifestation Theatre

- 中央 Manifestation Core 第一视觉；
- Da Vinci 左侧 Primary Operator；Romani 右后方 Observation Support；Mobile 可优先隐藏 Romani；
- Original Spiritron Manifestation Array，不复制官方召唤阵；
- Single / Tenfold 明确切换，默认 Single；
- T0～T5 / Multiplier / Payout 永远显示真实逻辑；
- 不使用 Servant 星级收藏语法、New / Owned / Inventory 心智；
- Single 常规 1.1–1.6s，T4/T5 可额外 400–700ms，非 Reaction 稳定结果硬上限 2.2s；
- Tenfold 每 Tile 90–140ms stagger，完整约 1.5–2.5s，支持 Skip / Reveal All。

---

# 18. Slot — King's Treasury Slot Gallery

- Gilgamesh Stage Right Rear，Sovereign Host；
- 5×3 Reel Grid，10 fixed paylines；
- Original Royal Treasury Sigils + Wild；
- Total Wager / Active Lines = 10 / Line Stake 必须同视野可理解；
- 不增加 Scatter、Bonus、Free Spin、Jackpot、Double-or-Nothing；
- 正常 Reel Motion 1.2–1.7s，Stop Interval 120–160ms / reel；Fast Stop 后剩余 <=350ms，结果不变。

---

# 19. Blackjack — Casino Camelot VIP Royal Table

旗舰 Casino A 页面。

- Artoria Ruler：Table Adjudicator / Royal Host，不是系统 Dealer；
- Royal Table 使用半月 / 马蹄轮廓、实体桌布、Dealer / Player Zone；
- Chaldea Ivory Playing Cards + 标准四花色高可读体系；
- READY：Initial Wager + 10 / 100 / 500 / 1000 + Deal；
- PLAYER_TURN：Hit / Stand；Double / Split 按服务端合法集合条件出现，并显示真实追加金额；
- Dealer Hole Card 在玩家阶段保持隐藏；
- Split / Natural / Push / Bust / Dealer Bust 等继续严格遵守上游规则；
- 不给策略建议；
- Deal 100–160ms / card，初始发牌 450–700ms，Hit <=300ms，Split separation 180–240ms；不制造慢悬念。

---

# 20. Poker — High Roller Poker Salon

Poker Lobby 仍属于普通 Chaldea Shell；Poker Table 使用 Full Immersive Shell。

## 20.1 Lobby

优先 Active Session → Service / Assets → Search / Filter → Table List → Create / Join → History。PC 使用高可读 Table Network List，不做巨大卡片墙。Public / Password 清晰可见；Preview → Seat → Reservation → Buy-in 保持短流程。

## 20.2 Table

- 无永久 Servant / Dealer Character；
- Arc Oval / High Roller Table + Chaldea Ivory Cards；
- Seat Node 清晰表达 Master / Stack / Bet / Hand / Sit Out / Disconnect / Waiting BB 等；
- Board / Main Pot / Side Pot 居中且 Side Pot 不合并为模糊总数；
- Action Tray 仅显示服务端合法 Fold / Check / Call / Bet / Raise / All-in；
- Amount quick options 只设置金额，不自动提交；
- All-in 是 High Commitment，不等于 Critical Red；
- Client reconnect Timer 继续，Service Paused 才暂停；
- Spectator 不得看到私有 Hole Cards；Folded / Mucked 不被动画泄漏。

## 20.3 Precision Tokens

```text
Desktop Control Bar      92px
Mobile Control Bar       112px
Mobile Action Tray       fixed bottom
Action Tray Max Height   258px + safe-area
Mobile Board Min Height  575px
Seat Main Text           11px
Seat Secondary           >=10px
```

Mobile 优先：自己的 Hole Cards → Current Action / Timer → Board → Pot → Current Actor → Own Stack → Other Seats。Mobile 不显示普通 Bottom Navigation。

---

# 21. Chaldea Operations

Operations 是管理后台，不赌场化、不角色化，P0 Heroic Persona。

PC：Persistent Sidebar + Persistent Operations Top Bar + High-density Workspace。

```text
Desktop Sidebar          252px
Desktop Top Bar          76px
<=1100 Sidebar           88px
<=720                    Drawer
Desktop Content Max      1260px
Mobile Content Gutter    12px
```

Top Bar 持续包含 Environment、Global Search、Needs Attention、Return to User Site、Current Admin。Environment 使用文字 + 结构/图形差异，不只依赖颜色。

Overview 是 Attention-first：Needs Attention Matrix + Service Status + Current Activity + Recent Administrative Activity，不做 KPI 图表墙。

Economy / Reconciliation / Adjustment / Games Config / Rankings Repair / Audit 必须继续遵守上游状态机、Version、Fresh Auth、Typed Confirmation、Operation ID、Append-only Audit 等规则。NewAPI Admin 是独立外部后台 Cross-link。

---

# 22. Layered Production Asset System

## 22.1 Media 与 UI 永久分层

```text
background_plate
character_layer
game_object_layer
foreground_atmosphere
真实 HTML / CSS / SVG UI
```

任何图片不得承载权威余额、按钮、结果、Poker 私牌、下注数值、菜单、排行榜、Login Form 等业务信息。

## 22.2 Character Master

建议 Production Master：2048×3072、2:3、Transparent Alpha、Full Body、完整轮廓与安全边距。

一个 Skin 默认一份 `idle_master`；Direct Play 额外一份 `reaction_visual + reaction_audio`。Summon Da Vinci / Romani Idle 必须是两张独立透明人物资产，便于 Mobile 降级；Reaction 可是双角色 Combined。

## 22.3 Background Plate

高身份场景建议：

```text
Desktop Master  >=2560×1440
Mobile Master   >=1440×1920
```

Mobile 不是 PC 中心硬裁。Tablet 优先从 Desktop Master 自适应。

Background 默认不画主要角色；人物单独分层。

## 22.4 Formats / Naming / Manifest

```text
Source Character    lossless alpha master
Web Character       WebP alpha + fallback
Background Web      AVIF preferred + WebP fallback
Vector              SVG
Reaction            transparent WebM + static WebP fallback
```

文件统一 lowercase kebab-case + `vNNN`，例如：

```text
char-artoria-ruler-blackjack-idle-v001.webp
bg-casino-blackjack-vip-table-desktop-v001.avif
obj-blackjack-royal-table-v001.svg
```

Asset Manifest 至少记录：asset_id / domain / scene / character / skin / role / version / source / fallback / focal points / safe_area / alpha / status / rights_note / prompt_archive_id。

状态：PLANNED → GENERATED → REVIEWED → SELECTED → PRODUCTION_READY / REJECTED。

---

# 23. Motion — Quiet Royal Motion

## 23.1 Levels

```text
M0 Static       0ms
M1 Micro        80–140ms
M2 Structural   160–240ms
M3 Event        280–600ms
M4 Phenomenon   700–1600ms
```

普通 UI 常态 <=240ms。

Easing：

```css
Standard: cubic-bezier(0.2, 0, 0, 1)
Enter:    cubic-bezier(0, 0, 0.2, 1)
Exit:     cubic-bezier(0.4, 0, 1, 1)
```

基础时长：Button 100ms、Tab 120ms、Tooltip 100ms、Focus 80ms、Dropdown 140ms、Dialog 180ms、Drawer / Bottom Sheet 220ms、Toast 180/140ms、Route 180ms。

Button 只变 Fill / Border / Text / Focus，不移动、不缩放。

Royal Beacon Intro 只在首次品牌进入使用一次 900–1200ms，常态 Header 静态。

## 23.2 Reaction

Character Reaction Visual / Audio 建议 1.2–1.8s，硬上限 2.2s；单次、非循环、可 Skip、不阻塞 Result，不覆盖 Critical / Network。

Reduced Motion 下 M3/M4 转为 Static Result + <=150ms Fade；Reduced Media 独立关闭视频和非必要赌场媒体。Reduced Motion 不自动等于 Mute。

---

# 24. Performance / Media Budget

```text
Card Persona              512–768px long edge   typical <=250KB
Detail Persona            1024–1536px            typical <=500KB
Desktop Hero AVIF                                  <=550KB target
Desktop Hero WebP                                  <=800KB fallback
Mobile Hero AVIF                                   <=400KB target
Mobile Hero WebP                                   <=600KB fallback
Transparent Reaction WebM                          <=1.5MB target
Reaction >2.5MB                                    mandatory review
Static Reaction WebP                               <=350KB target
Functional SVG                                     <=12KB typical
Complex Brand / Ritual SVG                         <=80KB target
```

首屏关键视觉传输目标：Mobile <=800KB，Desktop <=1.2MB。

只 Preload Functional UI Font、当前页面 LCP Hero / Background、必要时唯一首屏 Character；Reaction / 后续 Persona / 其他游戏 Scene Lazy Load。

V1 Background Plate 默认静态：禁止常驻 Casino Video、无限粒子、常驻 Live2D / WebGL Casino Lobby。

Core Web Vitals Production Target：

```text
LCP <= 2.5s
INP <= 200ms
CLS <= 0.1
```

---

# 25. Accessibility — WCAG 2.2 AA Production Gate

```text
普通文字                 >=4.5:1
大字号文字               >=3:1
UI Component / Icon      >=3:1
Focus Indicator          >=3:1
```

所有业务必须纯键盘完成；禁止 Hover-only。Dialog / Drawer / Sheet 正确管理 Focus Trap 与 Focus Return。Semantic HTML First，ARIA 只补充。

异步状态可使用适度 Live Region；Poker Timer 禁止每秒向 Screen Reader 连续报数。

游戏权威结果必须有文本等价：Dice values / total、Scratch match / multiplier、Slot line / payout、Blackjack totals / result、Poker board / pot / winner / settlement。

Decorative Casino / Ornament / Atmosphere / Decorative Host 使用空 Alt；Model Persona 可以使用简短家族身份 Alt，但不替代 Model Information。

支持 200% Zoom 与约 320 CSS px 核心 Reflow；游戏可以特殊重排，但 Wager / Action / Result 不依赖横向滚动。

Chart 必须同时提供 Label / Legend、Marker / Pattern / Direct Label、关键数值摘要；重要数据可通过表格或结构化摘要取得。

---

# 26. Rights / Trademark / Privacy Gate

## 26.1 Rights Manifest

每个生产资产记录 Rights Status：

```text
ORIGINAL_PLATFORM
ORIGINAL_GENERATED
THIRD_PARTY_CHARACTER_DERIVED
LICENSED_OR_APPROVED
REFERENCE_ONLY
RIGHTS_REVIEW_REQUIRED
```

并记录 source / creator-or-generator / reference / rights_status / rights_note / review_date。

## 26.2 Route B Boundary

自行生成 / 自行重绘只代表不直接复制官方素材，**不自动等于第三方 IP 权利已清除**。Artoria Ruler、Gilgamesh、Mash 等可识别第三方角色在公开运营或商业化前仍需由项目所有者确认合法使用基础。

禁止直接生产复用：FGO 官方立绘 / 背景 / UI / Logo / 召唤阵 / 音乐 / 语音、官方声优声音克隆、未授权字体、澳门真实赌场 Logo / 室内照片、第三方模型 Persona 参考原图。

## 26.3 Persona Rights Check

九个 Selected Model Persona 在 PRODUCTION_READY 前进行 Provider Trademark / Rights Spot Check；局部风险优先微调 Logo / Symbol / Weapon Geometry，不推翻 Persona Identity。

## 26.4 Review Privacy

Codex Screenshot / QA / Demo / Review Package 强制使用 Fixture / Demo Data；不得包含 API Secret、OAuth / Discord Token、完整真实 Account ID、真实用户资料、私密 Prompt / Response、未公开 Poker Hole Cards。

---

# 27. Production Release Gate

只有以下全部 PASS 才能标 `PRODUCTION_READY`：

```text
Visual                         PASS
IA / Business Contract        PASS
Responsive                     PASS
Keyboard                       PASS
Screen Reader Spot Check       PASS
Contrast                       PASS
Reduced Motion / Media         PASS
Performance Budget             PASS
Core Web Vitals Target         PASS
Asset Manifest                 PASS
Rights Review                  PASS
Fallback / Media Failure       PASS
```

任一 Gate FAIL 时，只能保持 `SELECTED / IMPLEMENTED / REVIEW_REQUIRED` 等中间状态。

媒体失败必须 Graceful：Persona→Glyph / Family Geometry；Casino BG→CSS Surface；Reaction→Static Frame / Stable Result；Audio→Silent Result；Character→无角色仍可完整操作。

---

# 28. 仍保持 OPEN 的生产项

以下不是视觉方向缺口，而是必须在真正素材生产 / 上线前完成的 Production Work：

- 最终 FGO-inspired Character Production Art、具体服装、姿势、表情、焦点与 Mobile Crop；
- Royal Observatory / Casino Camelot 各 Background Plate 的最终绘制与摄影机位；
- Blackjack Table、Poker Table、Dice Tray、Scratch Sigil、Summon Array、Slot Symbol 等最终生产资产精修；
- Character Reaction 的具体动作、台词、声线与业务触发阈值；
- 九个 Model Persona 的高分辨率 Normalize、Alpha Safety、Rights Review 与 Web Delivery 产物；
- 真实 Rights / License / Trademark 审核结论；
- 真实生产环境 Core Web Vitals / Accessibility / Fallback Gate 执行结果。

上述项目完成前可以是 `SELECTED` 或 `IMPLEMENTED`，不能因 Art Direction FINAL 自动标记 `PRODUCTION_READY`。

---

# 29. 实施权威关系

后续 Technical Design v0.5 / Implementation Spec v1.0 应按以下优先级读取：

```text
需求基线 v0.2.11
        ↓
IA v0.3.1 FINAL
        ↓
Art Direction v0.4 FINAL   ← 本文档
        ↓
Technical Design v0.5
        ↓
Implementation Spec v1.0
```

如代码、旧原型、旧 Prompt、旧 WORKING 文档与本文档冲突，以本文档当前生效视觉决策为准；如冲突涉及业务语义 / 路由 / 权限，以需求基线与 IA 为准。

---

# Appendix A — Decision Register / Supersession Traceability

以下 Decision Register 原样保留 `WORKING v0.37` 的全部 `AD-FRZ-001～582` 状态，用于审计历史。主文实现人员不应从已标记 `SUPERSEDED / PARTIALLY SUPERSEDED` 的历史条目恢复旧视觉方向。

| 决策编号 | 决策主题 | 当前状态 |
|---|---|---|
| AD-FRZ-001 | 最终主方向选择 D：Chaldea Arc Terminal | SUPERSEDED → AD-FRZ-429 |
| AD-FRZ-002 | Command / Heroic Persona / Phenomenon 三层视觉体系 | FROZEN |
| AD-FRZ-003 | 角色使用强度采用 P0–P4 分级 | FROZEN |
| AD-FRZ-004 | 角色与业务逻辑、核心组件和结果结算解耦 | FROZEN |
| AD-FRZ-005 | 素材路线采用 B：从第一版开始自行生成 / 制作 | FROZEN |
| AD-FRZ-006 | GPT-Image-2 作为首选图像资产生成工具 | FROZEN |
| AD-FRZ-007 | 首批页面与游戏默认角色映射 | FROZEN |
| AD-FRZ-008 | Blackjack 默认角色改为阿尔托莉雅 Ruler | FROZEN |
| AD-FRZ-009 | 从者插入细节、构图和具体特效暂不冻结 | FROZEN BOUNDARY |
| AD-FRZ-010 | 采用累计 Markdown 冻结稿工作流 | FROZEN |
| AD-FRZ-011 | Polar Command / Royal Arc / Spiritron Bloom 的全站基础配比与页面族视觉浓度 | SUPERSEDED → AD-FRZ-431 |
| AD-FRZ-012 | 全站色彩气质采用 A：Polar Observatory / 极地蓝银 | SUPERSEDED → AD-FRZ-433～435 |
| AD-FRZ-013 | V1 只维护完整深色主题，允许局部象牙白仪式材质 | SUPERSEDED → AD-FRZ-435 |
| AD-FRZ-014 | 品牌强调色与功能语义色采用明确职责分离 | SUPERSEDED → AD-FRZ-433 / 436 |
| AD-FRZ-015 | 字体方向采用 A：Command & Ceremony | FROZEN |
| AD-FRZ-016 | Functional Sans = IBM Plex Sans SC；Technical Mono = IBM Plex Mono | FROZEN |
| AD-FRZ-017 | Ceremonial English = Marcellus；Ceremonial Chinese = Noto Serif SC | FROZEN |
| AD-FRZ-018 | 中文为主要产品语言；英文承担技术词、Kicker 与仪式副标题 | FROZEN |
| AD-FRZ-019 | 字体视觉权重基准为 Functional 86% / Technical 11% / Ceremonial 3% | FROZEN |
| AD-FRZ-020 | 资产与游戏数字使用 Functional Sans + Tabular Numerals；ID / Hash / Code 使用 Mono | FROZEN |
| AD-FRZ-021 | V1 字体静态自托管、限制字重并按需加载仪式字体 | FROZEN |
| AD-FRZ-022 | 材质方向采用 A：Chaldea Precision Arc Frame | SUPERSEDED → AD-FRZ-437 |
| AD-FRZ-023 | 全站以克制圆角矩形为基础，重点 Command Panel 才使用局部单侧切角 | PARTIALLY SUPERSEDED → AD-FRZ-442 |
| AD-FRZ-024 | 建立 Polar Void / Canvas / Command Surface / Raised Surface / Command Overlay / Phenomenon Overlay 六级空间表面层级 | PARTIALLY SUPERSEDED → AD-FRZ-446 |
| AD-FRZ-025 | 关键正文、表单、数据、下注与状态使用实体表面；玻璃仅用于有限悬浮和场景过渡 | FROZEN |
| AD-FRZ-026 | 常态层级依靠明度、边框与空间关系；彩色发光仅用于 Focus、运行状态和短暂演出 | SUPERSEDED → AD-FRZ-438 |
| AD-FRZ-027 | 建立 Chaldea Frame 的结构线、局部编号、轨道节点和单侧激活线规范 | SUPERSEDED → AD-FRZ-432 / 442 |
| AD-FRZ-028 | 工具页克制、游戏舞台增强、Poker HUD 实体、Operations 高密度低装饰 | PARTIALLY SUPERSEDED → AD-FRZ-443 / 446 |
| AD-FRZ-029 | 移动端减少切角、阴影、模糊和背景纹理，同时保留同一品牌几何血统 | FROZEN |
| AD-FRZ-030 | 主 Logo 方向采用 A：Arc Beacon / 灵子观测环 | FROZEN |
| AD-FRZ-031 | 主标志由不闭合双轨道、C 负形、中央 Command Core 与单一 Arc Node 构成 | FROZEN |
| AD-FRZ-032 | 建立 Primary Lockup、Compact Lockup、Terminal Glyph 与 Favicon 四级响应式 Logo | FROZEN |
| AD-FRZ-033 | 基础 Logo 必须在单色、无辉光与 16–24px 小尺寸下成立 | FROZEN |
| AD-FRZ-034 | 主 Wordmark 使用 CHALDEA PLATFORM；中文承担界面与品牌说明 | FROZEN |
| AD-FRZ-035 | 图标分为 Functional、Navigation、Status、Product Domain、Ceremonial Sigil 五类 | FROZEN |
| AD-FRZ-036 | 普通功能沿用熟悉语义；复杂 FGO-inspired 符号只进入品牌、产品域和仪式层 | FROZEN |
| AD-FRZ-037 | 导航与高风险状态不得使用无文字抽象纹章，也不得只通过颜色表达 | FROZEN |
| AD-FRZ-038 | 主 Logo 与核心符号不得复制官方 Chaldea、职阶、令咒或召唤阵图形 | FROZEN |
| AD-FRZ-039 | GPT-Image-2 用于黑白概念探索与场景预览；最终 Logo / 核心图标必须矢量重构 | FROZEN |
| AD-FRZ-040 | 轨道角度、线宽、节点位置、字标字形与产品域图标轮廓暂不冻结 | BOUNDARY CLOSED → AD-FRZ-486～495 |
| AD-FRZ-041 | 全站 Motion 方向采用 A：Controlled Spiritron Motion / 受控灵子运动 | FROZEN |
| AD-FRZ-042 | 建立 M0 Static、M1 Micro、M2 Structural、M3 Event、M4 Phenomenon 五级动效强度 | FROZEN |
| AD-FRZ-043 | Command / Heroic Persona / Phenomenon 三层分别承担常态反馈、角色存在与关键显现 | FROZEN |
| AD-FRZ-044 | 普通交互使用短位移、边框、Arc Node 与局部状态线，不使用全站持续漂浮和夸张缩放 | FROZEN |
| AD-FRZ-045 | Route、Tab、Dialog、Drawer 与 Sheet 按空间来源进行轻量、连续转场 | FROZEN |
| AD-FRZ-046 | 工具与后台保持 M1–M2；奖励与普通游戏使用 M2–M3；Summon、Grand / Apex 才进入 M4 | FROZEN |
| AD-FRZ-047 | 所有强演出支持 Skip / Fast Reveal；Mute 与动态效果设置彼此分离 | FROZEN |
| AD-FRZ-048 | 完整支持 prefers-reduced-motion，并提供不依赖动画的静态结果与状态回退 | FROZEN |
| AD-FRZ-049 | 动画只揭示服务端已经锁定的结果，不影响资产、Round、牌序或操作时机 | FROZEN |
| AD-FRZ-050 | 精确时长、Easing、位移、缩放、粒子密度与媒体预算暂不冻结 | BOUNDARY CLOSED → AD-FRZ-544～562 |
| AD-FRZ-051 | Grand / Apex 不采用角色宝具或长战斗分镜，改用角色中奖响应演出 | FROZEN |
| AD-FRZ-052 | V1 每款游戏的默认角色 Skin 只要求一份中奖响应动画视觉资源与一份配套音频 | FROZEN |
| AD-FRZ-053 | Grand 与 Apex 可复用同一角色响应包，强度差异由全站复用的 UI / 灵子效果层承担 | FROZEN |
| AD-FRZ-054 | 角色响应为短时、单次、非循环 Cut-in；结束或跳过后立即回到稳定结果摘要 | FROZEN |
| AD-FRZ-055 | 角色动画与音频继续采用路线 B 自行生成 / 制作，不直接截取官方动画或官方语音 | FROZEN |
| AD-FRZ-056 | 具体动作、表情、台词、声线、触发阈值、时长与最终媒体格式仍逐游戏确认 | PARTIALLY CLOSED → AD-FRZ-557；动作 / 表情 / 台词 / 声线 / 触发阈值仍 OPEN |
| AD-FRZ-057 | 空间方向采用 A：Adaptive Command Grid / 自适应管制网格 | FROZEN |
| AD-FRZ-058 | 全站采用 4px 微单位与 8px 主节奏的间距体系 | FROZEN |
| AD-FRZ-059 | 响应式主网格采用 PC 12 列、Tablet 8 列、Mobile 4 列 | FROZEN |
| AD-FRZ-060 | 建立 Viewport Backdrop / Application Shell / Content Grid / Overlay Layer 四层空间结构 | FROZEN |
| AD-FRZ-061 | 建立 Reading / Standard / Wide / Stage / Immersive / Ops Fluid 六种容器模式 | FROZEN |
| AD-FRZ-062 | 建立 Standard Command / Compact Data / Stage 三种页面密度 | FROZEN |
| AD-FRZ-063 | 角色、背景与光效可越出 Content Grid；表单、牌面、资产、操作与结果必须留在 UI Safe Grid | FROZEN |
| AD-FRZ-064 | 移动端优先保证触摸目标、安全区与业务内容，空间不足时由角色与装饰主动降级 | FROZEN |
| AD-FRZ-065 | 精确 Breakpoint、Max Width、Gutter、Gap、Card Padding 与导航高度等待代表页面实渲染 | FROZEN BOUNDARY |
| AD-FRZ-066 | 基础组件方向采用 A：Command Instrument System / 管制仪器组件体系 | SUPERSEDED → AD-FRZ-437～440 |
| AD-FRZ-067 | 建立 Primary、Secondary、Tertiary、Destructive 与 Game Action 操作层级 | FROZEN |
| AD-FRZ-068 | 每个操作组只保留一个明确 Primary；Ritual Gold 不作为普通主按钮默认色 | SUPERSEDED → AD-FRZ-440 |
| AD-FRZ-069 | 表单采用常驻 Label、实体输入表面、高可见 Focus 与文字化错误反馈 | FROZEN |
| AD-FRZ-070 | Tabs、Segmented Control、Filter Chip、Status Badge、Checkbox、Radio 与 Toggle 各自保持明确职责 | FROZEN |
| AD-FRZ-071 | Card 分为 Information、Metric、Action 与 Clickable 四类 | FROZEN |
| AD-FRZ-072 | 整张 Card 仅在拥有单一目标时可点击；多操作 Card 使用明确按钮 | FROZEN |
| AD-FRZ-073 | Skeleton、Spinner、Processing 与 Network Uncertain 使用不同组件语义 | FROZEN |
| AD-FRZ-074 | Toast 不作为资产、奖励、游戏结算或危险操作的唯一反馈 | FROZEN |
| AD-FRZ-075 | Dialog、Drawer、Bottom Sheet 与 Full-screen Sheet 按任务复杂度和设备使用 | FROZEN |
| AD-FRZ-076 | 游戏与仪式 Skin 只强化外层表现，不改变控件语义、Focus、Disabled 或 Loading | PARTIALLY SUPERSEDED → AD-FRZ-438～440 |
| AD-FRZ-077 | 精确控件高度、Padding、Icon Size、Focus Offset 与状态参数等待浏览器实渲染 | FROZEN BOUNDARY |
| AD-FRZ-078 | 导航视觉采用 A：Layered Arc Rail Navigation / 分层弧轨导航 | SUPERSEDED → AD-FRZ-441 |
| AD-FRZ-079 | Global / Context / Local Navigation 使用三种不同视觉层级，不表现为三排相同 Tabs | FROZEN |
| AD-FRZ-080 | Global Header 以稳定实体 Command 表面为常态；Public / Stage 顶部可有限场景融合并在滚动后实体化 | FROZEN |
| AD-FRZ-081 | PC 一级导航以文字为主，使用短 Command Blue 激活线与 Arc Node 表达选中 | SUPERSEDED → AD-FRZ-441 |
| AD-FRZ-082 | Asset Summary 为进入 Wallet 的单一目标紧凑组件；Master Avatar Menu 保持轻量 | FROZEN |
| AD-FRZ-083 | Mobile 使用 Top Bar + 五项 Bottom Navigation，Active 同时由图标、标签与状态线表达 | PARTIALLY SUPERSEDED → AD-FRZ-441 |
| AD-FRZ-084 | Mobile Context Navigation 使用紧凑或横向滚动 Rail，不将全部同级入口隐藏进下拉菜单 | FROZEN |
| AD-FRZ-085 | Tablet 只在 Condensed Desktop 与 Mobile Pattern 间切换，不建立第三套导航 | FROZEN |
| AD-FRZ-086 | Direct Play 保留 Focused Shell；Poker Table 使用独立必要 Command HUD | FROZEN |
| AD-FRZ-087 | Chaldea Operations 使用低装饰实体 Sidebar、持续环境标记与顶部工具区 | PARTIALLY SUPERSEDED → AD-FRZ-441 / 446 |
| AD-FRZ-088 | 一级页面不使用 Breadcrumb；深层 Detail 提供 Breadcrumb 或明确 Parent Back | FROZEN |
| AD-FRZ-089 | Sticky、Badge、Focus 与 Motion 不得制造布局跳动或遮挡关键业务状态 | FROZEN |
| AD-FRZ-090 | 精确 Header、Context Rail、Bottom Navigation 与 Sidebar 尺寸等待代表页面实渲染 | FROZEN BOUNDARY |
| AD-FRZ-091 | 数据视觉采用 A：Command Data Matrix / 管制数据矩阵 | FROZEN |
| AD-FRZ-092 | 建立 Primary Metric / Supporting Metric / Operational Data / Reference Data 四级数据层级 | FROZEN |
| AD-FRZ-093 | 普通业务数字使用 Functional Sans + Tabular Numerals；ID / Hash / 审计字符串使用 Mono | FROZEN |
| AD-FRZ-094 | 金额与用量必须保留单位、正负号和真实业务精度，不伪造小数位或用颜色替代含义 | FROZEN |
| AD-FRZ-095 | 负数不自动等于错误；正常消耗、游戏盈亏与 Critical 故障使用不同语义 | FROZEN |
| AD-FRZ-096 | 建立 Standard / Compact Operational / Ledger & Audit / Leaderboard / Mobile Record 五类数据视图 | FROZEN |
| AD-FRZ-097 | 表格整行只在拥有单一详情目标时可点击；多操作行使用明确按钮或菜单 | FROZEN |
| AD-FRZ-098 | 图表仅用于真实趋势、比较和构成；明确数字与表格优先于装饰性可视化 | FROZEN |
| AD-FRZ-099 | Command Blue / Spiritron Cyan / Ritual Gold / Ether Violet 用作普通数据系列；功能状态色不作普通分类色板 | SUPERSEDED → AD-FRZ-507～510 |
| AD-FRZ-100 | Zero / Empty / Loading / Failure / Partial / Stale / Unknown 必须使用不同状态语义 | FROZEN |
| AD-FRZ-101 | 聚合与排行榜显示统计周期、业务时区、Last Updated 与延迟 / 数据来源状态 | FROZEN |
| AD-FRZ-102 | Leaderboard 使用克制的 Top 3 标记与独立 My Rank，不将完整列表做成领奖台 | FROZEN |
| AD-FRZ-103 | 移动端将宽表转换为 Record Card 与全屏详情，不机械压缩 PC 表格 | FROZEN |
| AD-FRZ-104 | Chaldea Operations 使用同一数据体系的高密度版本，Critical 行不使用大面积高饱和红底 | FROZEN |
| AD-FRZ-105 | 最终行高、Cell Padding、图表尺寸、Axis、Legend、Tooltip 与数据密度参数等待代表页面实渲染 | FROZEN BOUNDARY |
| AD-FRZ-106 | 状态视觉采用 A：Command Severity Protocol / 管制状态分级协议 | FROZEN |
| AD-FRZ-107 | 重要状态由图标、形状、标题、说明与颜色共同表达，不得只依赖颜色 | FROZEN |
| AD-FRZ-108 | 建立 Neutral / Running / Processing & Recovering / Success / Warning / Critical / Offline & Maintenance 状态语法 | PARTIALLY SUPERSEDED → AD-FRZ-498 / 500 / 502 / 503 |
| AD-FRZ-109 | 建立 Field / Inline / Module / Page Banner / Toast / Dialog / Full-page / Critical Layer 八级反馈层级 | FROZEN |
| AD-FRZ-110 | 同一业务事件只保留一个主要状态容器；高层级反馈出现时不重复制造 Toast 与弹窗 | FROZEN |
| AD-FRZ-111 | 401 / 403 / 404 / 409 / 429 / 503 使用不同页面语义，不套用同一红色错误页 | FROZEN |
| AD-FRZ-112 | 写操作采用 Submitting → Processing → Confirmed，并支持 Returned / Needs Attention / Not Executed | FROZEN |
| AD-FRZ-113 | Network Uncertain 使用独立持久状态，优先查询原 Business ID，不开放盲目重复提交 | FROZEN |
| AD-FRZ-114 | Maintenance 按 Component / Page / Product Domain / Global / Active Game Scope 分级表达 | FROZEN |
| AD-FRZ-115 | Critical Notice 与 Announcement / Entry Popup / Home Banner / 普通状态 Banner 保持职责与视觉分离 | FROZEN |
| AD-FRZ-116 | Critical、Network Uncertain 与业务结果摘要始终高于角色、大奖 Cut-in 与 Phenomenon Layer | FROZEN |
| AD-FRZ-117 | Full-page State 使用 Command Layer 与 P0 角色强度，保留安全父级、Reference ID 与恢复操作 | FROZEN |
| AD-FRZ-118 | 移动端合并低优先级状态；Critical 与 Processing 不得被 Bottom Sheet、键盘或游戏 HUD 遮挡 | FROZEN |
| AD-FRZ-119 | 状态支持键盘、Screen Reader、色觉差异与 Reduced Motion，禁止震屏、闪烁和循环告警辉光 | FROZEN |
| AD-FRZ-120 | 最终 Alert Padding、Banner Height、Toast Duration、Status Icon、Motion 与 Critical Layer 参数等待实渲染 | FROZEN BOUNDARY |
| AD-FRZ-121 | Public / Auth 页面家族采用 A：Polar Gate / 极地接入门 | PARTIALLY SUPERSEDED → AD-FRZ-429 / 443 |
| AD-FRZ-122 | Public Home、Login、Discord Registration 共享 Polar Observatory 场景骨架与同一 Global Header 血统 | PARTIALLY SUPERSEDED → AD-FRZ-429 / 443 |
| AD-FRZ-123 | Public Home 默认不永久绑定从者，以 Arc Beacon、极地观测空间和 Models / Entertainment 双路径为主 | FROZEN |
| AD-FRZ-124 | Login 使用阿尔托莉雅 Saber P3 角色层，PC 采用角色舞台 + 实体认证面板的 7 / 5 构图 | FROZEN |
| AD-FRZ-125 | Discord Registration 使用玛修 P2–P3，并以 Required Server / Role 与 Discord OAuth 为核心 | FROZEN |
| AD-FRZ-126 | Mobile Public / Auth 采用顶部视觉舞台 + 下方实体功能区，不机械缩小 PC 横向构图 | FROZEN |
| AD-FRZ-127 | 背景、角色、前景氛围与 Web UI 分层生成 / 集成，不在 AI 图片中烘焙文字、按钮、Logo 或业务状态 | FROZEN |
| AD-FRZ-128 | Entry Popup 使用可立即关闭的 Command Notice；PC 为 Dialog，Mobile 为 Bottom Sheet | FROZEN |
| AD-FRZ-129 | 角色与背景可 Full-bleed，但认证、资格、状态、错误和主要操作必须位于 UI Safe Grid | FROZEN |
| AD-FRZ-130 | 具体角色姿势、服装细节、背景物件、Hero 文案、精确列宽与素材尺寸等待生图和浏览器截图评审 | FROZEN BOUNDARY |
| AD-FRZ-131 | Dashboard 采用 A：Chaldea Command Deck / 迦勒底主控甲板 | FROZEN |
| AD-FRZ-132 | Dashboard 使用 Standard Command Density，不采用大型 Profile / Character Hero | FROZEN |
| AD-FRZ-133 | Dashboard 顶部采用轻量 Master Command Status，角色常态 P1、最高 P2，具体角色不固定 | FROZEN |
| AD-FRZ-134 | Critical / Active Session 使用条件式 Priority Dock，出现时动态插入且不保留永久空位 | FROZEN |
| AD-FRZ-135 | API Credit 与 Available Chips 构成首屏双资产核心；Poker In Play 与 Processing 作为条件状态表达 | FROZEN |
| AD-FRZ-136 | Daily / Hourly / Relief 组合为统一 Rewards Rail，同时保留各自独立资格与领取状态 | FROZEN |
| AD-FRZ-137 | PC Dashboard 中段采用 API Operations / Entertainment 约 6 / 6 双业务主线 | FROZEN |
| AD-FRZ-138 | Recent Activity / Announcements 降至第二视觉层，Discovery 不挤占 P0 / P1 | FROZEN |
| AD-FRZ-139 | Dashboard 常态背景使用 Polar Canvas + 极弱 Command 结构，不使用 Full-bleed 从者 | FROZEN |
| AD-FRZ-140 | Mobile Dashboard 按 Active → Identity → Assets → Rewards → API / Entertainment → Activity / Announcements 重排 | FROZEN |
| AD-FRZ-141 | Dashboard 不建立与 Bottom Navigation 重复的 Quick Action Grid | FROZEN |
| AD-FRZ-142 | 精确 Card 尺寸、首屏高度、资产字号、Rewards Rail 高度与 Operator 具体角色等待实渲染 | FROZEN BOUNDARY |
| AD-FRZ-143 | Models 页面采用 Chaldea 平台壳层 + Model Identity 内容层的双层视觉结构 | FROZEN |
| AD-FRZ-144 | Model Square 的模型 Card 在简要信息下方加入该模型对应的原创拟人立绘 | FROZEN |
| AD-FRZ-145 | 模型拟人立绘不使用 FGO 从者，与平台 / 游戏从者角色体系明确分离 | FROZEN |
| AD-FRZ-146 | 模型拟人立绘属于 Model Identity Asset，可用于 Card、Model Detail 与推荐模块 | FROZEN |
| AD-FRZ-147 | Model Square 采用 A：Catalog Card / 图鉴卡片式，以“上信息、下立绘”为基础双区结构 | FROZEN |
| AD-FRZ-148 | 模型 Card 立绘采用克制半身 / 上半身构图，信息优先、角色辅助，不做手游抽卡式满屏立绘 | FROZEN |
| AD-FRZ-149 | Model Detail 使用信息区 + 立绘展示区；PC 左右分栏，Mobile 上下重排 | FROZEN |
| AD-FRZ-150 | 同一模型家族共享统一拟人视觉母体，子型号通过同族变体建立区分 | FROZEN |
| AD-FRZ-151 | 模型拟人素材从第一版起采用原创、非从者化角色设计，不直接复用 FGO / Fate 角色 | FROZEN |
| AD-FRZ-152 | 各模型具体人物设定、发色、服装、符号、姿态与子型号映射在 Model Persona Mapping 阶段逐项确认 | FROZEN BOUNDARY |
| AD-FRZ-153 | 建立独立 Model Persona 生图 Prompt 归档文件，生成模型拟人形象时同步保存实际人物 Prompt | FROZEN |
| AD-FRZ-154 | Model Persona Prompt 归档只记录人物相关提示词与必要构图 / 透明背景要求，不记录页面场景、环境背景或整页构图 Prompt | FROZEN |
| AD-FRZ-155 | Persona Prompt 采用版本化追加记录；修改 Prompt 时保留旧版本并标明最终选用版本，避免后续无法复现 | FROZEN |
| AD-FRZ-156 | Model Square 使用紧凑 Page Header + Search / Filter / Sort，不采用大型 Persona Hero | FROZEN |
| AD-FRZ-157 | PC 目录以三列自适应为主要目标，空间不足转两列，Mobile 一列；不锁死像素断点 | FROZEN |
| AD-FRZ-158 | 推荐模型保持统一 Card 尺寸，以 Ordering / Badge 区分，不制作巨型 Featured Card | FROZEN |
| AD-FRZ-159 | Model Card 固定为 Identity → Summary → Attributes & Pricing → Persona Slot → Action Rail | FROZEN |
| AD-FRZ-160 | Persona 不承担 Model ID、Availability、Pricing、Context 或 CTA 等业务信息 | FROZEN |
| AD-FRZ-161 | 复杂价格不得压缩为虚假统一单价；Card 使用真实 Pricing Summary | FROZEN |
| AD-FRZ-162 | Persona 缺失或加载失败时使用 Model Glyph / Family Geometry 回退，不阻断模型展示 | FROZEN |
| AD-FRZ-163 | Model Card 不采用整卡隐式点击；使用明确查看详情与辅助 Copy 等操作 | FROZEN |
| AD-FRZ-164 | Model Detail Hero 采用约 7 / 5 的信息 + Persona 分栏，并在下方进入完整专业信息 | FROZEN |
| AD-FRZ-165 | Mobile 保持“信息在前、Persona 在后”的顺序，空间不足优先降低 Persona | FROZEN |
| AD-FRZ-166 | Persona 的最终比例、Card 高度、Detail Hero 高度与具体列宽等待真实人物素材和浏览器截图验证 | FROZEN BOUNDARY |
| AD-FRZ-167 | 第一批 Model Persona 视觉基准采用用户提供的九张参考立绘作为家族方向锚点 | FROZEN |
| AD-FRZ-168 | 图 1–9 固定映射为 DeepSeek、GPT、Claude、Gemini、GLM、Kimi、文心一言、千问、Grok | FROZEN |
| AD-FRZ-169 | 这些参考图用于锁定人物气质、服装轮廓、配色和配件方向；正式资产仍需自行原创生成 | FROZEN |
| AD-FRZ-170 | 后续生成 Persona 时，应将对应参考图路径、人物 Prompt 版本与最终采用结果一并写入 Prompt 档案 | FROZEN |
| AD-FRZ-171 | 如同一家族存在多个子型号，默认在当前参考方向上衍生同族变体，除非用户另行指定 | FROZEN |
| AD-FRZ-172 | AD-07.4 Model Persona Mapping 与九个 Family Master 首轮生图阶段完成并归档 | FROZEN |
| AD-FRZ-173 | 九个 Family Master 的生成目标统一为全身人物资产、透明背景要求、可安全裁成半身；实际透明度与裁切质量需在实渲染前逐项验证 | BOUNDARY CLOSED → AD-FRZ-531～543 |
| AD-FRZ-174 | 已生成 Persona 当前统一保持 GENERATED，不因“已出图”自动标记 SELECTED；最终采用版本需经后续页面实渲染与用户确认 | SUPERSEDED → AD-FRZ-531 |
| AD-FRZ-175 | `Chaldea_Platform_Model_Persona_Image_Prompts_WORKING_v0.5.md` 作为九个 Family Master 的人物 Prompt 配套归档，后续修改继续版本化追加 | FROZEN |
| AD-FRZ-176 | 每个视觉设计批次在用户确认完成后，必须先更新累计 Art Direction 及相关 Prompt / Asset 档案，再进入下一设计批次 | FROZEN |
| AD-FRZ-177 | Wallet / Rewards 视觉采用 A：Command Treasury / 管制资产库 | FROZEN |
| AD-FRZ-178 | Wallet Overview 采用 Processing 条件优先 → Total Assets → Asset Breakdown → Poker / Actions → Recent Transactions | FROZEN |
| AD-FRZ-179 | Total Assets 成为 Wallet D1 主指标，但不使用装饰性资产饼图替代明确数字 | FROZEN |
| AD-FRZ-180 | API Credit 与 Available Chips 使用等权专业资产面板；Poker In Play 与 Processing 采用条件状态 | FROZEN |
| AD-FRZ-181 | Exchange 采用 Dual-Asset Transfer Instrument，保持 Direction、Balance、Amount、Rate、Fee、Preview、Confirmation、Processing 完整层级 | FROZEN |
| AD-FRZ-182 | Exchange 最终确认使用实体 Dialog；提交后进入持久状态，不提供普通取消或盲目重试 | FROZEN |
| AD-FRZ-183 | Transactions 使用业务可理解的复合记录 + Standard Data Table；PC Drawer / Mobile Full-screen Detail | FROZEN |
| AD-FRZ-184 | Rewards Center 使用 Compact Reward Status Rail + 单个当前 Reward Detail，而不是三张巨大领取面板 | FROZEN |
| AD-FRZ-185 | Daily 视觉固定表现 500 API Credit | FROZEN |
| AD-FRZ-186 | Hourly 固定表现数量 100，但资产单位和时间算法动态读取服务端 | FROZEN |
| AD-FRZ-187 | Relief 固定表现数量 300、Total Assets < 10 与滚动四小时状态；资产单位和其他 TBD 读取服务端 | FROZEN |
| AD-FRZ-188 | Reward History 位于 Rewards Center 下部 / 局部区域，不新增全局一级路由 | FROZEN |
| AD-FRZ-189 | Wallet 不使用固定角色主视觉；Rewards 以 Reward Sigil / Arc 状态为主，避免资产页面过度角色化 | FROZEN |
| AD-FRZ-190 | Mobile Wallet 严格按 Processing → Total Assets → 可用资产 → 条件资产 → Actions → History 重排 | FROZEN |
| AD-FRZ-191 | Wallet / Rewards 精确 Card 高度、资产字号、Exchange 宽度、Reward Panel 高度与移动端间距等待实渲染 | FROZEN BOUNDARY |
| AD-FRZ-192 | API Usage 采用 A：Command Telemetry / 管制遥测台 | FROZEN |
| AD-FRZ-193 | `/api/usage` 内部使用 Overview / Request History Local Tabs，不改变 Context Navigation | FROZEN |
| AD-FRZ-194 | Overview 以周期 API Credit 消耗与 Today Usage 为核心；Request Count / Success / Error / Token 等仅在日志可靠时出现 | FROZEN |
| AD-FRZ-195 | Usage Trend 默认使用 API Credit Consumed 单主序列 Line Chart；真实条件指标通过 Selector 切换，不堆叠多张趋势图 | FROZEN |
| AD-FRZ-196 | Model / API Key / Purpose Breakdown 合并为单一可切换 Breakdown Module，优先使用水平 Bar | FROZEN |
| AD-FRZ-197 | API Usage 聚合时区采用 Asia/Shanghai，并显式显示周期、Last Updated 与数据状态 | FROZEN |
| AD-FRZ-198 | Request History Filter 固定覆盖时间、Model、API Key、Purpose、Status、Request ID 与 Sort | FROZEN |
| AD-FRZ-199 | PC Request Table 使用 Time / Model / Key / Purpose / Endpoint / Status / Usage / API Credit 核心列 | FROZEN |
| AD-FRZ-200 | Request Detail PC 使用 Drawer / Side Panel，Mobile 使用 Full-screen Detail | FROZEN |
| AD-FRZ-201 | Request Detail 只展示真实存在的日志元数据，不设计 Prompt / Request Body / 完整 Response 字段 | FROZEN |
| AD-FRZ-202 | Request Detail 中 API Key 只显示名称、Key ID 与遮罩标识，不显示完整 Secret | FROZEN |
| AD-FRZ-203 | 正常 API Consumption 不使用 Critical Scarlet；请求失败状态与实际 API Credit Consumption 分别表达 | FROZEN |
| AD-FRZ-204 | API Usage 中 Zero / Empty / Loading / Failure / Partial / Stale / Unknown 使用不同状态语义 | FROZEN |
| AD-FRZ-205 | Mobile Request History 转换为 Record Card，不机械压缩 PC 宽表格 | FROZEN |
| AD-FRZ-206 | Dashboard Today Usage、Wallet API Consumption Summary 与 API Usage 使用一致的视觉语义和真实统计来源 | FROZEN |
| AD-FRZ-207 | Chart Height、Table Row、Filter 高度、Drawer 宽度、Axis / Tooltip / Breakpoint 等精确参数等待浏览器实渲染 | FROZEN BOUNDARY |
| AD-FRZ-208 | Entertainment Hub / Game Catalog 视觉采用 A：Chaldea Recreation Deck / 迦勒底娱乐甲板 | SUPERSEDED → AD-FRZ-454 |
| AD-FRZ-209 | Entertainment Hub 采用 Status → Active / Resume → Featured → Continue / Browse → Multiplayer → Events / Rankings 的运营型视觉结构 | FROZEN |
| AD-FRZ-210 | Active / Resume 为条件式最高优先娱乐状态，无 Session 时完全移除 | FROZEN |
| AD-FRZ-211 | Featured Games 使用统一 Featured Shelf，不采用自动轮播 Banner Carousel | FROZEN |
| AD-FRZ-212 | Featured 数据由运营配置与 Game Registry 驱动，不锁死游戏数量 | FROZEN |
| AD-FRZ-213 | Continue Playing 由真实 History + Publication / Runtime State 动态生成，无历史时降级为 Explore Games | FROZEN |
| AD-FRZ-214 | Multiplayer Spotlight 单独承担 Poker 等大型多人入口，不将 Poker 简化为普通 Direct Play Card | FROZEN |
| AD-FRZ-215 | 在线人数、桌数、等待人数等仅在真实可靠时显示，不生成假活跃数据 | FROZEN |
| AD-FRZ-216 | `/games` 使用完整 Search / Dynamic Filter / Sort Catalog，不将 V1 游戏名称硬编码为导航 | FROZEN |
| AD-FRZ-217 | 建立统一 Game Access Card：Identity / Summary / Mode / Tags / Runtime State / Entry Summary / Main Action | FROZEN |
| AD-FRZ-218 | Game Catalog 明确区分 Direct Play / Lobby / Resume / Maintenance / Coming Soon CTA | FROZEN |
| AD-FRZ-219 | Maintenance 游戏保留介绍与历史入口但禁止新 Round；Coming Soon 不擅自增加预约功能 | FROZEN |
| AD-FRZ-220 | 游戏之间主要通过 Key Art、角色、Symbol、主题纹样和局部色彩区分，基础组件与状态语言保持统一 | FROZEN |
| AD-FRZ-221 | Game Key Art 可包含角色与游戏环境，但不得烘焙游戏名、余额、下注、倍率、按钮或业务状态 | FROZEN |
| AD-FRZ-222 | Anonymous Hub / Catalog 与登录态复用同一页面，Play 通过 Auth Gate + Return-to-Intent 返回原 Entry | FROZEN |
| AD-FRZ-223 | Mobile Entertainment 按 Active → Chips / Maintenance → Continue → Featured → Browse → Multiplayer → Events / Rankings 重排；Catalog Filter 使用 Bottom Sheet | FROZEN |
| AD-FRZ-224 | Rankings + Game History 页面族采用 A：Command Honors Archive / 管制荣誉档案 | FROZEN |
| AD-FRZ-225 | Rankings 保持唯一 `/rankings`，局部一级域为 Assets & Games / RP Usage | FROZEN |
| AD-FRZ-226 | Rankings 使用 Domain → Metric → Period / Filter → My Rank → Top 3 → Full List → Last Updated 层级 | FROZEN |
| AD-FRZ-227 | Top 3 使用克制金 / 银 / 铜荣誉强调，不采用大型赌场领奖台、金币雨或自动庆祝演出 | FROZEN |
| AD-FRZ-228 | My Rank 作为登录用户的独立高可见摘要，并允许滚动时快速定位 | FROZEN |
| AD-FRZ-229 | Total Assets 榜仅表现当前资产快照，不显示 Today / Week / Historical 回放控件 | FROZEN |
| AD-FRZ-230 | Game Profit / Biggest Win / Total Wagered / Poker Profit 按真实统计语义分别表现，不统一伪造成单一榜单逻辑 | FROZEN |
| AD-FRZ-231 | RP Usage 使用 Calls / Errors / Credits Consumed，并支持 Period、Model Filter、My Rank 与 Last Updated | FROZEN |
| AD-FRZ-232 | RP 排名行模型摘要 PC 默认 Top 3 + Other、Mobile 默认 Top 1；展开才显示 Model ID 分布 | FROZEN |
| AD-FRZ-233 | Rankings 公开 Master / Rank / Aggregate / Model Distribution，不公开 Key、Prompt、Response、Request、IP、UA 或 Provider | FROZEN |
| AD-FRZ-234 | Master Avatar / Nickname 在 Rankings 不建立其他用户公开 Profile 入口 | FROZEN |
| AD-FRZ-235 | Game History 使用唯一 `/history`，默认列表显示 Direct Play Round 与 Poker Session，不平铺全部 Poker Hand | FROZEN |
| AD-FRZ-236 | History Filter 固定覆盖 Record Type / Mode / Game / Time / Result / Status / ID Search | FROZEN |
| AD-FRZ-237 | Result 与 Status 使用独立视觉字段，不合并为一个含义模糊的 Badge | FROZEN |
| AD-FRZ-238 | Round / Session / Hand Detail 使用统一 Record Detail 语言，并提供 Wallet / Fairness / Game 等真实 Cross-link | FROZEN |
| AD-FRZ-239 | 历史 Detail 不重播中奖演出、不恢复可操作游戏控件；它是核验记录而不是游戏 Stage | FROZEN |
| AD-FRZ-240 | Mobile Rankings / History 使用紧凑榜单或 Record Card + Filter Bottom Sheet + Full-screen Detail，并保留列表筛选与滚动位置 | FROZEN |
| AD-FRZ-241 | Direct Play 共用视觉采用 Focused Arc Stage / 灵子专注舞台 | SUPERSEDED → AD-FRZ-455 |
| AD-FRZ-242 | 五款 Direct Play 共享 Focused Shell，同时允许 Stage Split / Stage Wide 两类内部 Arrangement | FROZEN |
| AD-FRZ-243 | 共用 Shell 固定包含 Game Header / Stage / Wager & Action / Round State / Result / Rules & Transparency / Fairness / History | FROZEN |
| AD-FRZ-244 | Game Stage 为角色、背景、游戏对象与氛围特效的主要自由区；业务 UI 必须留在 UI Safe Grid | FROZEN |
| AD-FRZ-245 | 建立 Stage Left / Stage Right / Stage Rear 三类 Character Anchor，具体游戏逐项选择 | FROZEN |
| AD-FRZ-246 | Wager 与创建 Round 明确分离；10 / 100 / 500 / 1000 只选择金额，不直接提交 | FROZEN |
| AD-FRZ-247 | 建立统一 Round State Ribbon，覆盖 Ready / Submitting / Accepted & Resolving / Waiting for Action / Settled / Recovering / Cancelled & Refunded | FROZEN |
| AD-FRZ-248 | 所有 Direct Play 使用稳定 Result Summary：Result / Bet / Payout / Net Change / Available Chips / Round ID / Detail | FROZEN |
| AD-FRZ-249 | 结果表现分 Normal / Significant / Grand-Apex 三档；Grand / Apex 延续 Character Reaction + Audio 体系 | FROZEN |
| AD-FRZ-250 | Skip / Fast Reveal / Mute 使用统一控制语法，但是否出现由具体 Game Capability 决定 | FROZEN |
| AD-FRZ-251 | Rules / Payout / Transparency / Fairness / History 使用共用 Utility Drawer / Sheet，不长期挤占 Stage | FROZEN |
| AD-FRZ-252 | Balance Insufficient 使用游戏内稳定解决状态，允许 Wallet / Rewards 往返但禁止自动 Replay / Bet | FROZEN |
| AD-FRZ-253 | Mobile 使用独立 Focused 重排，当前 Action 高于装饰与下注历史状态，Bottom Navigation 默认保留 | FROZEN |
| AD-FRZ-254 | Reduced Motion / 媒体失败时直接降级到静态确定结果，Result 与业务状态始终完整 | FROZEN |
| AD-FRZ-255 | 游戏媒体统一按 Background / Character / Game Stage Asset / Atmosphere / Reaction Visual / Reaction Audio 等可替换资产层组织 | FROZEN |
| AD-FRZ-256 | Stage Split / Wide 的具体游戏映射、角色精确位置、背景、游戏物件材质与游戏专属 Motion 留到 AD-08.2～08.6 逐项冻结 | FROZEN BOUNDARY |
| AD-FRZ-257 | Dice 专属视觉采用 A：Fortune Arc Deck / 幸运弧轨观测台 | SUPERSEDED → AD-FRZ-456 |
| AD-FRZ-258 | Dice PC 使用 Stage Split，Stage / Control 约 7 / 5 作为实渲染起点 | FROZEN |
| AD-FRZ-259 | 阿斯托尔福作为 Dice 默认角色；PC 使用 Stage Left、P3、约三分之二身，职责为 Lucky Companion 而非 Dealer | FROZEN |
| AD-FRZ-260 | Mobile 阿斯托尔福降为 P2 头肩 / 半身，骰子保持 Stage 第一视觉主角 | FROZEN |
| AD-FRZ-261 | Dice 使用 Arc Dice Tray，不采用传统赌场 Dice Cup / 骰宝桌 | PARTIALLY SUPERSEDED → AD-FRZ-456 |
| AD-FRZ-262 | 三颗骰子采用 Chaldea Ivory Dice：象牙白主体、深蓝 Pip、浅冰蓝倒角，以传统易读点数为核心 | FROZEN |
| AD-FRZ-263 | Big / Small 使用两个等权选择块，显示 4–10 / 11–17 与 1:1；首次进入不默认选择 | FROZEN |
| AD-FRZ-264 | V1 不显示可点击 Triple 下注 Card；Triple 只在结果阶段作为特殊结果表达 | FROZEN |
| AD-FRZ-265 | Big / Small 选中统一使用 Command Blue Selection，不以颜色暗示不同价值或概率 | FROZEN |
| AD-FRZ-266 | Roll 演出采用 Tray 激活 → 三骰翻滚 → 落定 → Total → Category → Win / Loss → Result Summary | FROZEN |
| AD-FRZ-267 | 骰子停顿节奏固定且与结果无关，不制造 Outcome-dependent Near-miss | FROZEN |
| AD-FRZ-268 | 结果阅读顺序固定为 Dice → Total → Actual Category → Player Choice → Win / Loss | FROZEN |
| AD-FRZ-269 | Triple 使用 Ether Violet 特殊结果语法，禁止 Ritual Gold / Jackpot 庆祝；必须明确 Big / Small 均判负 | FROZEN |
| AD-FRZ-270 | Dice V1 只使用 Normal Win / Loss + Triple Special Result，不人为制造 Grand / Apex 中奖等级 | FROZEN |
| AD-FRZ-271 | 建立 Astolfo Lucky Reaction 的 reaction_visual + reaction_audio 资产方向；具体 Win 触发频率暂不冻结 | FROZEN |
| AD-FRZ-272 | 常规 Roll 使用短灵子启动 + 实体骰碰撞音；Triple 使用特殊非庆祝共鸣音 | FROZEN |
| AD-FRZ-273 | Dice Background Plate 使用 Chaldea Fortune Observation Bay，不采用传统赌场、绿色赌桌、金币或 Las Vegas 风格 | SUPERSEDED → AD-FRZ-456 |
| AD-FRZ-274 | PC Rules / Probability / RTP / Fairness / History 使用共用 Utility Drawer | FROZEN |
| AD-FRZ-275 | Mobile 使用 Stage → Big / Small → Wager → Roll → Result 的单列重点布局，Bottom Navigation 默认保留 | FROZEN |
| AD-FRZ-276 | 阿斯托尔福具体服装、最终姿势细节、背景镜头、骰子精确尺寸、Roll 时长、Reaction 台词 / 声线等待素材生成与浏览器实渲染 | FROZEN BOUNDARY |
| AD-FRZ-277 | Scratch 专属视觉采用 A：Stellar Treasury Voucher / 星辉宝券台 | FROZEN |
| AD-FRZ-278 | Scratch PC 使用 Stage Wide，3 × 3 Scratch Card 为第一视觉主角 | FROZEN |
| AD-FRZ-279 | 伊什塔尔使用 Stage Right、P3、半身至三分之二身，职责为 Stellar Prize Host；Mobile 降为 P2 | FROZEN |
| AD-FRZ-280 | 卡片采用 Chaldea Stellar Prize Voucher：Ivory / Royal Blue / Ritual Gold 的高规格凭证材质 | FROZEN |
| AD-FRZ-281 | 九格统一使用 Spiritron Foil 覆盖；未刮状态不得因底层结果出现视觉差异或结果泄露 | FROZEN |
| AD-FRZ-282 | Scratch 使用 Original Prize Sigil + 明确 ×Multiplier 双重识别，不只依靠颜色 | FROZEN |
| AD-FRZ-283 | 1x / 2x / 3x / 5x / 10x / 25x / 100x 使用不同原创几何符号；未中奖卡不存在专门 0x Symbol | FROZEN |
| AD-FRZ-284 | READY 时显示 Wager + Buy Card；购买后 Wager 锁定降权，SCRATCHING 成为主要交互 | FROZEN |
| AD-FRZ-285 | PC / Touch 必须 Hold/Touch + Drag 才刮开，单击 / 单点不揭示单格 | FROZEN |
| AD-FRZ-286 | Reveal All 始终可发现，作为 Secondary Game Action，不改变结果或账务 | FROZEN |
| AD-FRZ-287 | Mobile 仅在刮奖区域发生有效 Drag 时接管触摸，页面其他区域保持正常滚动 | FROZEN |
| AD-FRZ-288 | 第三个匹配符号揭示后允许对三个匹配 Sigil 做局部强调，但不使用 Payline | FROZEN |
| AD-FRZ-289 | Scratch 结果表现映射：0x No Win；1x Break-even；2x/3x Normal；5x/10x Significant；25x Grand；100x Apex | FROZEN |
| AD-FRZ-290 | Scratch 结果等级只属于 Presentation Grade，不改变 Prize Tier、倍率、概率或结算语义 | FROZEN |
| AD-FRZ-291 | 1x 必须明确表现为回本 / Net Change = 0，不包装成盈利 Win | FROZEN |
| AD-FRZ-292 | 25x / 100x 使用同一 Ishtar reaction_visual + reaction_audio，Grand / Apex 差异由共享 UI Effect 强度表达 | FROZEN |
| AD-FRZ-293 | 伊什塔尔 Reaction 采用短时财富 / 宝石 / 星辉庆贺方向，不使用宝具、攻击或长战斗分镜 | FROZEN |
| AD-FRZ-294 | Scratch Background Plate 使用 Chaldea Stellar Treasury Counter，不使用现实彩票店、赌场、美元或官方 FGO 场景复刻 | SUPERSEDED → AD-FRZ-457 |
| AD-FRZ-295 | Prize Card、Foil Mask、Sigil 与 Scratch Residue 由真实前端 / SVG / Mask 系统实现，不烘焙进背景图片 | FROZEN |
| AD-FRZ-296 | Mobile 以 3 × 3 Card → Scratch / Reveal All → Result 为核心，伊什塔尔主动让位于触控安全区 | FROZEN |
| AD-FRZ-297 | Reduced Motion 保留手动 Scratch 与 Reveal All，但移除高密度碎屑、视差、大位移和强 Glow | FROZEN |
| AD-FRZ-298 | 卡片最终比例、Scratch Threshold、Mask 算法、伊什塔尔服装姿势、Background 镜头、Grand/Apex 精确时长和 Reaction 台词等待实渲染 / 素材阶段 | FROZEN BOUNDARY |
| AD-FRZ-299 | Summon 专属视觉采用 A：Spiritron Manifestation Chamber / 灵子显现管制室 | SUPERSEDED → AD-FRZ-458 |
| AD-FRZ-300 | Summon 使用 Stage Wide / Ritual Stage，以中央 Manifestation Core 为第一视觉主角 | FROZEN |
| AD-FRZ-301 | 达芬奇使用 Stage Left、P3、Primary Operator；罗马尼使用 Stage Right Rear、P2–P3、Observation Support | FROZEN |
| AD-FRZ-302 | Mobile 以 Manifestation Core 为第一主角，达芬奇降 P2、罗马尼降 P1 / 可优先隐藏 | FROZEN |
| AD-FRZ-303 | 使用原创 Spiritron Manifestation Array，继承 Arc Beacon DNA，不复制官方 FGO Summoning Circle | FROZEN |
| AD-FRZ-304 | Single / Tenfold 使用明确 Segmented Control，第一次进入默认 Single | FROZEN |
| AD-FRZ-305 | Base Wager / Draw Count / Total Cost / Available Chips / 最低预估余额必须同时清晰，主 Summon 按钮直接显示 Total Cost | FROZEN |
| AD-FRZ-306 | T0～T5 映射为原创 Manifestation Tier 视觉，同时始终显示逻辑 Tier、×Multiplier 或明确收益信息 | FROZEN |
| AD-FRZ-307 | V1 不使用 Servant 星级收藏语法，不让 Reward Tier 产生永久角色卡 / Inventory 心智 | FROZEN |
| AD-FRZ-308 | Reveal 对象使用 Manifestation Result Tile，不包含从者立绘、礼装、New、Owned 或收藏编号 | FROZEN |
| AD-FRZ-309 | Single Reveal 顺序为 Array 激活 → Core 升亮 → Spiritron Bloom → Result Tile → Tier / Multiplier → Summary | FROZEN |
| AD-FRZ-310 | 一旦出现预示 Reward Tier 的光效，必须忠实对应锁定 Tier；禁止假彩光和 Outcome-dependent Near-miss | FROZEN |
| AD-FRZ-311 | Tier 表现为 T0 No Manifestation、T1 Break-even、T2 Normal、T3 Significant、T4 Grand、T5 Apex | FROZEN |
| AD-FRZ-312 | T1 / 1x 必须明确表现为回本 / Net Change = 0，不包装成盈利 | FROZEN |
| AD-FRZ-313 | Tenfold 按 draw_index 1–10 自动 Reveal，并将每个 Result Tile 落入固定结果网格 | FROZEN |
| AD-FRZ-314 | PC Tenfold 完整结果使用 5×2 Result Grid 作为原型方向；Mobile 优先 2 列结果布局，最终比例待实渲染 | FROZEN |
| AD-FRZ-315 | Skip / Reveal All 只跳过表现，不改变 Tier、Draw 顺序、派彩、Round、扣款或派奖 | FROZEN |
| AD-FRZ-316 | Tenfold 的整体 Win / Loss / Break-even 严格由 Round Net Change 判断，Highest Tier 仅作为独立信息 | FROZEN |
| AD-FRZ-317 | Summon 只制作一份 Da Vinci + Romani 双角色 reaction_visual + 一份 reaction_audio | FROZEN |
| AD-FRZ-318 | Single T4/T5 可触发 Reaction；Tenfold 只在全部结果完成后根据 Highest Tier 最多触发一次 | FROZEN |
| AD-FRZ-319 | 双角色 Reaction 以达芬奇为主要动作、罗马尼为辅助反应，不使用宝具、战斗或官方从者显现 | FROZEN |
| AD-FRZ-320 | Background Plate 使用 Chaldea Spiritron Manifestation Chamber，不直接复刻官方 FGO 召唤室 | SUPERSEDED → AD-FRZ-458 |
| AD-FRZ-321 | Summon 可使用全站最高 M4 Spiritron Bloom，但 Skip、Critical、Network、Result Summary 始终高于演出 | FROZEN |
| AD-FRZ-322 | Tier 名称最终文案、Result Tile 比例、角色具体服装姿势、Manifestation Array 精细几何、粒子密度、音效、Reaction 台词和精确时长等待素材与浏览器实渲染 | FROZEN BOUNDARY |
| AD-FRZ-323 | Slot 专属视觉采用 A：King's Treasury Reel / 王之宝库转轴 | SUPERSEDED → AD-FRZ-459 |
| AD-FRZ-324 | Slot 正式采用 Stage Wide，5×3 Reel Grid 为第一视觉主角，吉尔伽美什为第二视觉主角 | FROZEN |
| AD-FRZ-325 | 吉尔伽美什 PC 使用 Stage Right Rear、P3、Sovereign Host；Mobile 降为 P2 | FROZEN |
| AD-FRZ-326 | 机台采用 Treasury Reel Console，不使用传统拉杆式赌场老虎机、投币口、现金槽或 777 Jackpot 顶灯 | FROZEN |
| AD-FRZ-327 | Reel 使用 Obsidian Spiritron Reel，高速状态可表现灵子纵向运动，停轮后必须恢复高可读 Symbol | FROZEN |
| AD-FRZ-328 | L1–H2 + W 映射为原创 Royal Treasury Sigils，不直接复制官方 Fate 职阶、圣晶石、令咒或现成宝具图标 | FROZEN |
| AD-FRZ-329 | Wild 使用高辨识 Sovereign Sun Sigil，但不得暗示 Scatter、Bonus、额外倍率或 Jackpot | FROZEN |
| AD-FRZ-330 | 10 条 Payline 常态只显示紧凑 Line Marker，不长期把完整彩线覆盖在 5×3 Grid 上 | FROZEN |
| AD-FRZ-331 | Settled 后先展示完整 Grid，再轮播 Current Winning Line；当前线高亮，其他合法 Winning Lines 低强度保留 | FROZEN |
| AD-FRZ-332 | 每条 Winning Line 同时显示 Line ID / Symbol / Length / Line Stake / Multiplier / Line Payout，不仅依赖颜色 | FROZEN |
| AD-FRZ-333 | Total Wager 为唯一可编辑下注主字段，Line Stake = Total Wager / 10 为只读派生值，Active Lines 固定显示 10 | FROZEN |
| AD-FRZ-334 | Spin 与 Fast Stop 分离；Fast Stop 只结束当前 Presentation，不允许玩家逐 Reel Skill Stop | FROZEN |
| AD-FRZ-335 | Reel 默认从左至右停止，使用固定 Presentation Curve；停顿和减速不得依据结果制造 Near-miss | FROZEN |
| AD-FRZ-336 | Slot 必须明确区分 No Win / Partial Return-Net Loss / Break-even / Win，存在中奖线不等于 Round Win | FROZEN |
| AD-FRZ-337 | Presentation Grade 按 Total Payout Multiplier 分档：0x No Win、0–1x Partial Loss、1x Break-even、>1–<5x Normal、5–<20x Significant、20–<100x Grand、≥100x Apex | FROZEN |
| AD-FRZ-338 | Slot Presentation Grade 阈值只控制视觉强度，不新增业务 Prize Tier、不改变 Paytable、RTP 或统计语义 | FROZEN |
| AD-FRZ-339 | Partial Return / Net Loss 可以高亮真实中奖线，但禁止 Big Win / Character Reaction / Jackpot 式庆祝 | FROZEN |
| AD-FRZ-340 | Grand / Apex 复用同一 Gilgamesh King's Approval reaction_visual + reaction_audio | FROZEN |
| AD-FRZ-341 | Gilgamesh Reaction 使用短时王之认可 / 皇家宝库几何光片，不采用 Gate of Babylon 攻击、宝具或长战斗分镜 | FROZEN |
| AD-FRZ-342 | Background Plate 使用 Chaldea Royal Treasury Simulation Bay，排除赌场大厅、Las Vegas、现金、金币瀑布和水果机风格 | SUPERSEDED → AD-FRZ-459 |
| AD-FRZ-343 | Slot Utility Drawer 重点覆盖 Paytable / 10 Paylines / Reel Strips / Wild / RTP / Fairness / History | FROZEN |
| AD-FRZ-344 | Mobile 优先保证 5×3 Grid → 10 Lines / Line Stake → Total Wager → Spin / Fast Stop → Result，角色主动让位 | FROZEN |
| AD-FRZ-345 | Reduced Motion 与 Fast Stop 均直接揭示同一已锁定 Stop / Grid / Payline 结果，不删除关键结算信息 | FROZEN |
| AD-FRZ-346 | Symbol 最终图案、机台比例、Gilgamesh 服装姿势、Payline 精确线型、Reel 曲线、Grand/Apex 精确时长和 Reaction 台词等待素材与浏览器实渲染 | FROZEN BOUNDARY |
| AD-FRZ-347 | Blackjack 专属视觉采用 A：Royal Adjudication Table / 王权裁决牌桌 | SUPERSEDED → AD-FRZ-460 |
| AD-FRZ-348 | Blackjack 正式采用 Stage Wide / Royal Table，Dealer / Player Hands 为第一视觉信息，阿尔托莉雅 Ruler 为第二视觉层 | FROZEN |
| AD-FRZ-349 | 阿尔托莉雅 Ruler 使用 Stage Right Rear、P3、Table Adjudicator，不担任实际 System Dealer；Mobile 降为 P2 | FROZEN |
| AD-FRZ-350 | Dealer Area 固定置于牌桌上方中央，清楚区分 Upcard / Hidden Hole Card / Dealer Total / Draw Sequence | FROZEN |
| AD-FRZ-351 | Dealer Hole Card 使用完全中性统一牌背；Peek 阶段不得通过翻牌、Glow、晃动或音效泄露结果 | FROZEN |
| AD-FRZ-352 | 扑克牌采用 Chaldea Ivory Playing Cards + 标准四花色高可读体系，牌背使用原创 Arc Pattern | FROZEN |
| AD-FRZ-353 | 牌桌采用 Royal Arc Table：深 Royal Blue / Obsidian + Ivory + restrained Gold，不使用传统 Casino Green | PARTIALLY SUPERSEDED → AD-FRZ-460～461 |
| AD-FRZ-354 | READY 使用 Initial Wager + Deal；Round 接受后 Wager 降级为锁定摘要，PLAYER_TURN 切换为 Action Dock | FROZEN |
| AD-FRZ-355 | Hit / Stand 为第一高频操作层；Double / Split 为条件式第二层，并直接显示真实追加 Stake | FROZEN |
| AD-FRZ-356 | Blackjack Action 不使用策略推荐色、Best Move、Basic Strategy Hint 或行为诱导 | FROZEN |
| AD-FRZ-357 | Player Hands 使用稳定 Hand Lane；Active Hand 以 Command Blue Arc + Node 高亮，其他手保持可读但降级 | FROZEN |
| AD-FRZ-358 | PC 最多 4 手使用分区 / 横向 Hand Lane；Mobile 使用 Active Hand 主视图 + Hand Navigator，不机械压缩四手 | FROZEN |
| AD-FRZ-359 | Split 动画只表现原手分离和确定性新牌进入，不洗牌、不重抽、不改变真实 hand_index | FROZEN |
| AD-FRZ-360 | Split Aces 使用专门 Auto Stand 状态；Split A + 10 仅为普通 21，不使用 Natural Blackjack 视觉 | FROZEN |
| AD-FRZ-361 | Natural Blackjack 仅用于原始未 Split 两张牌 A + 10-value，并采用 Significant Royal Gold / Ivory 特殊表现 | FROZEN |
| AD-FRZ-362 | Bust 使用游戏损失语义而非 Critical Error；Push 使用 Moon Silver 中性反馈；Dealer Bust 后仍逐手结算 | FROZEN |
| AD-FRZ-363 | 多手 Result 必须同时显示每 Hand Result / Stake / Payout / Net Change 与整个 Round Summary | FROZEN |
| AD-FRZ-364 | Round Overall Win / Loss 严格根据 Round Net Change，不因部分 Hand 获胜而包装成整体 Big Win | FROZEN |
| AD-FRZ-365 | Blackjack V1 不人为创建 Grand / Apex 业务奖级；Natural Blackjack 为 Significant Special Result | FROZEN |
| AD-FRZ-366 | 建立 Artoria Ruler Victory Acknowledgement reaction_visual + reaction_audio，但具体触发阈值等待实渲染 | FROZEN |
| AD-FRZ-367 | Artoria Reaction 采用克制皇家裁决 / 胜利认可，不使用宝具、战斗攻击或长动画 | FROZEN |
| AD-FRZ-368 | Background Plate 使用 Chaldea Royal Adjudication Lounge，排除赌场大厅、绿色赌桌、Las Vegas 与现实 Casino 环境 | SUPERSEDED → AD-FRZ-460 |
| AD-FRZ-369 | Dealer Peek 使用结果无关的统一预检查视觉 / 音效，真正结果差异只能在 Hole Card 合法 Reveal 后出现 | FROZEN |
| AD-FRZ-370 | Recovering 恢复同一 round_id、牌靴、Hands、Active Hand 与 Action；不设置短促决策倒计时 | FROZEN |
| AD-FRZ-371 | Utility Drawer 必须公开 6 Decks / Fresh Shuffle / S17 / 3:2 / Peek / Double / Split / DAS / Max 4 Hands / Split Aces / No Insurance 等冻结规则；参考 RTP 未验证前不得伪造精确百分比 | FROZEN |
| AD-FRZ-372 | Mobile 与 Reduced Motion 始终优先保留 Dealer、Active Hand、Stake、合法 Action、牌序和 Round Result；具体桌面比例、角色姿势、牌尺寸、动画时长与 Reaction 触发策略等待浏览器实渲染 | FROZEN BOUNDARY |
| AD-FRZ-373 | Poker 专属视觉采用 A：Chaldea Table Network / 迦勒底牌桌网络 | PARTIALLY SUPERSEDED → AD-FRZ-462 |
| AD-FRZ-374 | Poker Lobby 使用正常 Chaldea Shell，Poker Table 使用 Full Immersive Shell，并共享 Table Network 视觉母体 | FROZEN |
| AD-FRZ-375 | Poker Lobby 采用 Active Session → Service / Assets → Search / Filter → Table List → Create / Join → History 视觉层级 | FROZEN |
| AD-FRZ-376 | Active Poker Session 为 Lobby 条件式最高优先模块；存在时其它 Join / Spectate / Create 必须反映单桌 Session 限制 | FROZEN |
| AD-FRZ-377 | PC Poker Lobby 使用高可读 Table Network List，不采用大型房间卡墙 | FROZEN |
| AD-FRZ-378 | Public / Password Table 使用清楚 Access Badge；Password Table 保持 Lobby 可见，不制造 Unlisted / Invite-only 心智 | FROZEN |
| AD-FRZ-379 | 加桌使用 Table Preview → Seat Map → 30s Reservation → Buy-in 的内部短流程，不新增页面 | FROZEN |
| AD-FRZ-380 | Buy-in 明确表现 Available Chips → Table Stack / Poker In Play 的资产位置变化，并保持 40–100 BB 语义，不包装为消费 | FROZEN |
| AD-FRZ-381 | Create Table PC 使用 Side Drawer，Mobile 使用 Full-screen Sheet，仅展示 IA 已冻结字段 | FROZEN |
| AD-FRZ-382 | Poker Table 保持 P0–P1 Heroic Persona，不设置永久大型从者、Dealer 从者或 Winner Character Cut-in | FROZEN |
| AD-FRZ-383 | Poker Table 使用 Arc Oval Table + Polar Velvet，不使用传统 Casino Green | FROZEN |
| AD-FRZ-384 | Poker 与 Blackjack 共用 Chaldea Ivory Playing Cards / Arc Card Back 基础 Deck；Poker 使用更紧凑尺寸 | FROZEN |
| AD-FRZ-385 | 建立 Chaldea Table Token 作为筹码辅助视觉，Stack / Bet / Pot 数值始终为权威表达 | FROZEN |
| AD-FRZ-386 | Seat 使用统一 Command Seat Node，表达 Master / Stack / Bet / Hand / Sit Out / Disconnect / Waiting BB / Leave / Top-up 等状态 | FROZEN |
| AD-FRZ-387 | Current Action Player 使用 Command Blue / Cyan Active Arc + 明确数字 Timer；30 秒倒计时不采用高压闪烁或震屏 | FROZEN |
| AD-FRZ-388 | Board 为牌桌中央第一牌局信息，Main Pot / Side Pot 位于 Board 上方并保持独立 | FROZEN |
| AD-FRZ-389 | Side Pot 不得合并成不可解释的 Total Pot，多 Side Pot 必须逐项可识别 | FROZEN |
| AD-FRZ-390 | Action Tray 仅根据服务端合法 Action Set 展示 Fold / Check / Call / Bet / Raise / All-in | FROZEN |
| AD-FRZ-391 | Fold 不使用系统 Destructive Critical 风格；Check / Call 为高频主操作，Bet / Raise 根据当前合法状态进入金额控制 | FROZEN |
| AD-FRZ-392 | Bet / Raise Amount Rail 使用 Min / 1/2 Pot / 2/3 Pot / Pot / All-in / Slider / Integer Input；快捷值只设置金额，不直接提交 | FROZEN |
| AD-FRZ-393 | All-in 使用 High Commitment 视觉并明确金额，不与删除 / 系统危险的 Critical Red 混淆 | FROZEN |
| AD-FRZ-394 | Top Control Bar 固定提供 Lobby / Table-Blind-Ante / Wallet Chips / Table Stack / Network / Settings / Safe Leave | FROZEN |
| AD-FRZ-395 | Connected / Reconnecting / Disconnected / Server Paused 使用不同状态语义；Reconnecting 保留 Snapshot，Server Paused 同时暂停 Timer | FROZEN |
| AD-FRZ-396 | Take Over 必须由用户显式确认；新设备取得控制后旧设备转为只读，不自动多设备双控制 | FROZEN |
| AD-FRZ-397 | 返回大厅 / Leave / Browser Back 统一进入 Safe Leave；当前 Hand 中使用 Leave After Hand，Settlement 后才 Cash Out | FROZEN |
| AD-FRZ-398 | Spectator 使用同一桌面母体但无 Action Tray / 私人 Hole Cards，只显示合法公共信息与 Showdown | FROZEN |
| AD-FRZ-399 | Chat / Action Timeline / Rules / Fairness / Session Info 使用 Side Drawer / Panel；Mobile 使用 Sheet，绝不遮挡 Action Tray | FROZEN |
| AD-FRZ-400 | Showdown 只 Reveal 合法公开牌面，Folded / Mucked Cards 不因动画泄露 | FROZEN |
| AD-FRZ-401 | Winner 使用 Seat Victory Arc + 逐 Pot Settlement，不使用 Grand / Apex Character Reaction 或赌场式 Jackpot Celebration | FROZEN |
| AD-FRZ-402 | Poker 不建立 Direct Play 式 Payout Celebration Grade；正式 Poker Profit 继续以 Session Cash Out 后 Realized P/L 为准 | FROZEN |
| AD-FRZ-403 | Mobile Portrait 隐藏 Chaldea Bottom Navigation，并优先 Hole Cards → Action / Timer → Board → Pot → Action Player → Stack → Seats | FROZEN |
| AD-FRZ-404 | Mobile Landscape 与 Portrait 共享同一 Hand / Seat / Timer / Socket；旋转只改变布局，不触发新 Session | FROZEN |
| AD-FRZ-405 | Reduced Motion 使用短 Card / Token Motion 与静态 Winner Arc；精确桌面比例、Seat 坐标、牌尺寸、Chip 动画、Timer Motion、音效与背景镜头等待实渲染 | FROZEN BOUNDARY |
| AD-FRZ-406 | Chaldea Operations 采用 A：Command Operations Deck / 迦勒底运营管制甲板 | FROZEN |
| AD-FRZ-407 | Operations 固定使用 P0 Heroic Persona、M1–M2 常态 Motion，不使用从者主视觉或 Phenomenon Layer | FROZEN |
| AD-FRZ-408 | PC Operations Shell 使用 Persistent Sidebar + Persistent Operations Top Bar + High-density Workspace | FROZEN |
| AD-FRZ-409 | Production / Staging / Development 环境标识始终可见，并使用文字 + 图形 / 结构差异，不仅依赖颜色 | FROZEN |
| AD-FRZ-410 | Sidebar 按 Command / Catalog & Community / Economy & Data / Administration 分组；视觉隐藏入口不替代服务端 RBAC | FROZEN |
| AD-FRZ-411 | Global Search 使用全局 Command Search Pattern，可搜索稳定对象 ID，但不得展示 Secret / Password / Prompt / 未公开牌面 | FROZEN |
| AD-FRZ-412 | Operations Overview 采用 Attention-first，不以 KPI Card / 图表墙作为首要内容 | FROZEN |
| AD-FRZ-413 | Overview 使用 Needs Attention Matrix + Service Status Spine + Current Activity + Recent Administrative Activity 结构 | FROZEN |
| AD-FRZ-414 | Attention Item 固定显示 Severity / Object / Age / State / Reason / Deep Link；Acknowledge 只表示已查看 | FROZEN |
| AD-FRZ-415 | Overview / Economy / Rankings / Jobs 等聚合模块统一高可见显示 Last Updated / Data Freshness / Lag | FROZEN |
| AD-FRZ-416 | Economy 采用 Command Ledger Workspace，Wallet / Ledger 默认只读，不提供直接编辑最终 Balance 的输入框 | FROZEN |
| AD-FRZ-417 | Reconciliation 使用明确 State Queue + State Timeline，只展示当前状态机合法的 Retry / Resume / Compensate / Mark for Review | FROZEN |
| AD-FRZ-418 | Admin Adjustment 使用 Before / Delta / After + Reason / Reference + Fresh Auth + Typed Confirmation + Operation ID + Ledger / Audit 的 Critical Flow | FROZEN |
| AD-FRZ-419 | Games 使用 Registry + Stable Game Detail Tabs；Active Config 在视觉上明确 Locked | FROZEN |
| AD-FRZ-420 | Game Config 修改固定采用 Clone Active as Draft → Edit → Validate → Preview / Diff → Activate New Version | FROZEN |
| AD-FRZ-421 | Game Configuration 禁止通用 JSON Editor 绕过已冻结产品规则；只读规则使用明确 Baseline Lock 状态 | FROZEN |
| AD-FRZ-422 | Rankings Operations 采用 Aggregation Status / Historical Snapshots / Repair & Rebuild 控制台，不复制公共排行榜主视觉 | FROZEN |
| AD-FRZ-423 | Rankings Repair 使用 Shadow Snapshot → Old/New Diff → Review → Publish，禁止直接编辑用户 Score | FROZEN |
| AD-FRZ-424 | 管理操作建立 Routine / Impactful / Critical 三层视觉风险体系，Critical 只局部使用 Critical Scarlet | PARTIALLY SUPERSEDED → AD-FRZ-436 |
| AD-FRZ-425 | Audit 使用 Append-only Ledger / Audit Matrix，不提供 Edit / Delete；撤销通过新的反向或补偿 Operation | FROZEN |
| AD-FRZ-426 | Open NewAPI Admin ↗ 保持独立 External Admin Cross-link，并在新标签页打开 | FROZEN |
| AD-FRZ-427 | PC / Tablet / Mobile 分别使用 Persistent / Collapsible / Drawer Sidebar；Mobile 宽表转 Management Card、Critical Operation 使用全屏确认 | FROZEN |
| AD-FRZ-428 | Sidebar Width、Top Bar Height、Row Density、Attention Item Height、Drawer Width、Diff Layout、Management Card 和精确响应式参数等待浏览器实渲染 | FROZEN BOUNDARY |
| AD-FRZ-429 | 全站视觉母方向改为 Chaldea Royal Observatory / 迦勒底皇家观测宫；旧 Chaldea Arc Terminal 不再作为第一视觉印象 | FROZEN |
| AD-FRZ-430 | Entertainment / Direct Play / Poker 的娱乐子方向采用 Chaldea Royal Casino / 迦勒底皇家娱乐厅 | SUPERSEDED → AD-FRZ-449 |
| AD-FRZ-431 | 页面家族视觉权重重新校准为 Product Structure / Royal-FGO Atmosphere / Phenomenon，不再由 Polar Command 主导 | PARTIALLY SUPERSEDED → AD-FRZ-451 |
| AD-FRZ-432 | 普通前台显著削弱 HUD、坐标、模块编号、技术刻度、Arc Node 与装饰性 Mono；科技语法只保留给真实技术/数据职责 | FROZEN |
| AD-FRZ-433 | UI Chrome 的有彩配色严格限制为两个锚点色 + 二者中间色；不得再引入独立青、金、紫、绿、黄、红作为常态组件色 | SUPERSEDED → AD-FRZ-470～472 |
| AD-FRZ-434 | UI 三色关系固定为 Chaldea Ivory + Royal Azure + Moonlit Mid；角色/背景插画可保留自身自然色，但不得扩张为组件调色板 | PARTIALLY SUPERSEDED → AD-FRZ-470～473 |
| AD-FRZ-435 | V1 单一主题由“完整深色主题”改为 Bright Moonlit / 明亮月光主题；深色仅允许作为舞台或场景局部，不再铺满普通页面 | FROZEN |
| AD-FRZ-436 | Success / Warning / Critical / Running 等状态继续保留原业务语义，但不再依赖额外语义色；改用图标、标签、边框形态、填充反转与文字说明区分 | SUPERSEDED → AD-FRZ-472 |
| AD-FRZ-437 | 全站基础材质改为 Royal Minimal Frame / 皇家极简框架；去除“精密终端仪器”作为组件第一印象 | FROZEN |
| AD-FRZ-438 | 普通 Card / Input / Button / Navigation 使用 Flat Surface；禁止渐变、拟物高光、Bevel、Emboss、Inset Shadow、模拟凸起与持续 Glow | FROZEN |
| AD-FRZ-439 | 所有按钮采用纯色实体或透明底 + 1px 清晰描边的扁平语法；Hover / Pressed 不通过升起、下沉、3D 阴影或明显缩放表达 | FROZEN |
| AD-FRZ-440 | Primary / Secondary / Tertiary / Destructive / Game Action 继续保留业务层级，但全部使用同一三色扁平按钮体系；Destructive 依靠文案与确认流程而非红色按钮 | FROZEN |
| AD-FRZ-441 | Global / Context / Local Navigation 改用 Royal Line Navigation：纯色平面、文字、Underline / Border Active；普通导航不再使用 Arc Node、霓虹状态线或 HUD 装饰 | FROZEN |
| AD-FRZ-442 | 普通容器以简单圆角矩形、留白与细描边建立层级；技术编号、切角、轨道节点只在确有语义的少量对象使用，不作为默认 Card 装饰 | FROZEN |
| AD-FRZ-443 | Public/Auth、Entertainment、Direct Play 与 Poker 将 Background Plate / Atmosphere 作为一等视觉层；FGO / Casino 氛围主要由场景、角色与游戏物件承担，而非 UI Chrome | FROZEN |
| AD-FRZ-444 | Entertainment / Direct Play 提高角色可见强度；已冻结角色映射继续有效，角色必须成为真实构图成员而不是小型占位框；工具页与 Operations 仍保持 P0～P1 | FROZEN |
| AD-FRZ-445 | Casino 感通过牌桌、筹码、扑克牌、转轴、柜台、聚光关系与皇家休息厅空间建立，不使用 Las Vegas 霓虹、金币瀑布或额外金黑配色 | SUPERSEDED → AD-FRZ-449～467 |
| AD-FRZ-446 | 页面材质分层改为：工具/数据页明亮极简，Public/Auth 典雅场景化，Entertainment/Direct Play 皇家赌场化，Poker 高级牌桌化，Operations 明亮高密度低装饰 | PARTIALLY SUPERSEDED → AD-FRZ-454～464 |
| AD-FRZ-447 | 新视觉体系下所有旧页面专属色彩/材质描述若与 AD-FRZ-429～446 冲突，自动以新决策为准；其布局、角色、业务语义与交互流程保持冻结 | FROZEN |
| AD-FRZ-448 | 三色 Hex、Radius、Border、Spacing、Typography 与各页面亮度比例仍需新一轮 Codex 浏览器实渲染后冻结；本轮只提供 Prototype Seed，不直接升级为 FINAL Token | SUPERSEDED → AD-FRZ-471 / 473～485 |
| AD-FRZ-449 | 游戏区母方向升级为 A：Casino Camelot Grand Resort / FGO Royal Casino Resort，并以 FGO 皇家赌场世界优先于科技终端被感知 | FROZEN |
| AD-FRZ-450 | 游戏区的科技语法退居业务与公平验证功能层；第一视觉由赌场空间、角色、牌桌与游戏物件承担 | FROZEN |
| AD-FRZ-451 | Entertainment / Direct Play / Summon / Poker 的视觉权重进一步提高 FGO-Casino Atmosphere，并降低常态 Product Chrome 比例 | FROZEN |
| AD-FRZ-452 | Casino A 可借鉴澳门高端综合赌场的空间语言：挑高中庭、穹顶 / 格栅天花、吊灯、石材、纹样地毯、镜面金属、VIP Salon 与桌区聚光；禁止复制现实品牌标识与具体场景 | FROZEN |
| AD-FRZ-453 | UI Chrome 仍严格保持 Ivory / Azure / Mid 三色；赌场背景与角色资产可出现暖金、酒红、石材、木饰、水晶等自然场景色，但不得升级为 UI Component Token | PARTIALLY SUPERSEDED → AD-FRZ-470～473 |
| AD-FRZ-454 | Entertainment Hub / Game Catalog 的视觉场景升级为 Grand Casino Atrium / Royal Casino Floor；Table Games、Slot & Entertainment、Casino Camelot VIP、Poker Salon 仅作为场景分区，不新增 IA 导航 | FROZEN |
| AD-FRZ-455 | Direct Play 共用舞台升级为 Royal Casino Game Stage；Focused Shell、Wager、Round State、Result、Fairness 与 Mobile Bottom Navigation 业务结构继续冻结 | FROZEN |
| AD-FRZ-456 | Dice 场景升级为 Lucky Dice Salon：允许明显的高端赌场骰桌 / VIP Salon 氛围，但继续只提供 Big / Small 规则，不伪装成标准 Sic Bo 或新增赌场下注项 | FROZEN |
| AD-FRZ-457 | Scratch 保留 Stellar Prize Voucher 本体，背景升级为 Treasure Voucher / Prize Counter Salon；伊什塔尔置于高级赌场奖励休息区，不采用便利店彩票感 | FROZEN |
| AD-FRZ-458 | Summon 场景升级为 Grand Manifestation Theatre：作为 Casino Resort 内最 FGO / Ritual 化的娱乐表演厅，保留原有 Single / Tenfold 与 Manifestation Core 业务语义 | FROZEN |
| AD-FRZ-459 | Slot 场景升级为 King’s Treasury Slot Gallery：允许高端澳门式 Slot Gallery / Cabinet 空间感，吉尔伽美什作为 Royal Host；5×3 Grid 与所有 Reel 规则保持不变 | FROZEN |
| AD-FRZ-460 | Blackjack 旗舰场景升级为 Casino Camelot VIP Royal Table；阿尔托莉雅 Ruler 作为 Royal Host / Table Adjudicator，场景允许最明确的 FGO Casino Camelot 世界观联想但不得直接复制官方 UI / 场景素材 | FROZEN |
| AD-FRZ-461 | Blackjack Table 采用可识别的高端赌场半椭圆 / 马蹄形比例、软包桌边、Dealer / Player / Betting Arc 空间语法；这些仅为视觉结构，不新增 Side Bet、Insurance 或其它业务规则 | FROZEN |
| AD-FRZ-462 | Poker Table 场景升级为 Macau-style High Roller Poker Salon；Lobby 的 Table Network 信息架构继续保留，Table 不设置永久从者 Dealer，赌场感来自椭圆桌、高背座椅、灯具、地毯、VIP Lounge 与真实牌局物件 | FROZEN |
| AD-FRZ-463 | Blackjack / Poker 的 Playing Cards、Chips / Table Token、Dealer Button、Seat / Pot 物件提高实体与赌场存在感，但 Stack / Bet / Pot / Stake 数字仍是唯一权威资产表达 | FROZEN |
| AD-FRZ-464 | 游戏场景采用 Warm Casino Environment + Cool Moonlit UI Layer 的双层明暗关系；暖色只属于背景 / 角色 / 物件，业务 UI 继续使用三色 Flat Chrome | PARTIALLY SUPERSEDED → AD-FRZ-470～473 |
| AD-FRZ-465 | Casino Ornament 改为低频但大尺度的空间装饰：Arch、Chandelier、Ceiling、Carpet、Wall Panel、Drapery、Marble、Mirror 与 Serif Display；进一步删除微型 HUD、坐标、技术刻度与无业务 Mono 装饰 | FROZEN |
| AD-FRZ-466 | Casino Luxury 不得泄露到按钮拟物：Primary / Secondary / Game Action 继续严格纯色 + 描边，禁止 Gradient、Bevel、Emboss、Gloss、Inset Shadow、Hover 上浮与 Pressed 下沉 | FROZEN |
| AD-FRZ-467 | Casino A 强调 Casino Aesthetic 而非赌博刺激反馈：继续禁止结果无关 Near-miss、Coin Rain、虚假 Jackpot、将 Net Loss 包装成 Big Win 或用强刺激演出遮盖真实结果 | FROZEN |
| AD-FRZ-468 | Mobile 游戏页不能把赌场 / FGO 场景完全删除：通过安全裁切、角色 P2、桌面 / 灯具 / 背景局部保留空间身份；Action、Cards、Wager、Result 与 Poker Timer 始终高于场景装饰 | FROZEN |
| AD-FRZ-469 | 赌场背景最终相机、建筑细节、角色服装、场景暖色比例、桌面材质、Cabinet / Lounge 细节与媒体资产继续等待 Codex 浏览器占位实渲染；本阶段不生成图片，也不冻结最终图像 Prompt | FROZEN BOUNDARY |
| AD-FRZ-470 | “两个颜色 + 二者中间色”只约束 Button System 的填充、描边与交互色，不构成全站 UI Chrome、Status、Data Visualization、Domain Accent 或 Scene 的全局三色限制 | FROZEN |
| AD-FRZ-471 | Button System 的冻结三色为 Chaldea Ivory `#F4F0E8`、Royal Azure `#3568B7`、Moonlit Mid `#95ACD0`；Primary / Secondary / Tertiary / Destructive / Game Action 均保持 Flat Solid / Outline，文字可使用无彩 Ink / Ivory 保证可访问性 | FROZEN |
| AD-FRZ-472 | 全站除 Button System 外允许增加克制且有明确职责的功能色、状态色、数据色与产品域强调色；Success / Warning / Critical / Running 等不得仅依赖颜色，仍需图标、标签、形态与文字语义共同表达 | FROZEN |
| AD-FRZ-473 | Bright Moonlit 核心基底冻结为 Ink `#17202D`、Ink Soft `#596573`、Paper `#FFFFFF`、Surface `#FBFAF7`、Surface Muted `#E9EBEE`、Line `rgba(23,32,45,.16)`、Line Strong `rgba(23,32,45,.34)`；这些是中性/基底 Token，不限制后续语义色扩展 | FROZEN |
| AD-FRZ-474 | Button Default/State 冻结：Primary = Royal Azure + Ivory；Primary Hover = Moonlit Mid + Ink；Secondary = Ivory/Transparent + Royal Azure Border/Text；Secondary Hover = Moonlit Mid + Ink；Focus 使用 3px Royal Azure Outline；禁止 Gradient、Bevel、Emboss、Inset、Hover Lift、Scale 与 Press Sink | FROZEN |
| AD-FRZ-475 | Royal Minimal Frame 精确化：普通 Panel / Metric / Model Card / Game Card 使用纯色 Surface + 1px Outline，不使用常态 box-shadow；Shadow 仅限 Drawer / Dialog / Popover；Radius 冻结为 8 / 14 / 22px 三档 | FROZEN |
| AD-FRZ-476 | Typography Token 冻结：IBM Plex Sans SC 为 UI/中文正文，Noto Serif SC 为中文展示标题，Marcellus 为拉丁展示标题，IBM Plex Mono 为 ID / 数字技术信息；字体 V1 继续 Self-host；正文 Desktop 16px/1.6，Mobile 15px/1.6 | FROZEN |
| AD-FRZ-477 | Layout Token 冻结：主 Content Max Width = 1200px；Desktop 主水平 Gutter = 24px，≤1100px = 16px，≤720px = 14px；页面主节奏继续 4px Micro + 8px Main，Page Stack Desktop 52px Top / 28px Gap，Mobile 30px Top / 20px Gap | FROZEN |
| AD-FRZ-478 | Navigation / Control 尺寸冻结：Product Global Header Desktop 72px、Mobile 60px；Context Navigation 46px；Mobile Bottom Navigation 最小 70px + safe-area；常规交互最小高度 44px；Input 最小高度 46px | FROZEN |
| AD-FRZ-479 | Responsive 主断点冻结为 1100 / 720 / 420px；1100 以下进入压缩 Desktop/Tablet Pattern，720 以下进入 Mobile Pattern，420 以下执行超窄屏二次重排；不得以第三套独立 Tablet IA 替代既有规则 | FROZEN |
| AD-FRZ-480 | Model Square 代表网格冻结为 >1100px 三列、≤1100px 两列、≤420px 单列；Mobile Card 继续信息优先、Persona Crop 次之，不以横向压缩宽表替代响应式重排 | FROZEN |
| AD-FRZ-481 | Operations 精确 Shell Token 冻结：Desktop Sidebar 252px + Top Bar 76px；≤1100px Sidebar 88px；≤720px Sidebar 隐藏并进入 Drawer，Operations Content Mobile Gutter 12px；Desktop Operations Content Max Width 1260px | FROZEN |
| AD-FRZ-482 | Poker 精确 Shell Token 冻结：Desktop Control Bar 92px；Mobile Control Bar 112px；Mobile Action Tray 固定底部、Max Height 258px + safe-area；Mobile Poker Board Min Height 575px；Seat 主信息 11px、次级信息不低于 10px；Mobile 不显示普通 Bottom Navigation | FROZEN |
| AD-FRZ-483 | Casino / Direct Play 结构尺寸冻结为当前 v4 基准：Entertainment Hero Desktop Min Height 620px、Mobile 560px；Direct Play Game Stage Desktop Min Height 490px、Mobile 340px；Game Console Overlap Desktop -72px、≤1100px -48px、≤720px -28px；最终媒体只能在既有 Safe Grid 内调整 Crop，不得遮挡 Action / Wager / Result | FROZEN |
| AD-FRZ-484 | Login / Auth 代表布局冻结：Desktop 7/5 Scene-to-Panel Grid、最大 1360px、Scene Min Height 660px；≤1100px 改为单列且 Scene Min Height 500px；Mobile Scene Min Height 390px + 下方实体 Auth Panel；角色最终图片与背景仍保持媒体资产 OPEN | FROZEN |
| AD-FRZ-485 | Codex Browser Review v4 正式判定 PASS；上一轮 `Token-State Fix v1.3` 因“三色全局化”误解而取消执行。v4 已验证的业务/IA/响应式/Accessibility 结构成为 AD-09.2 Token Freeze 依据；最终角色、背景、Casino Costume、音频与媒体生产继续 OPEN | FROZEN |
| AD-FRZ-486 | Arc Beacon 最终几何收口采用 A：Royal Beacon / 皇家灵子观测环；保持唯一平台主品牌标志，不赌场化、不从者化 | FROZEN |
| AD-FRZ-487 | Royal Beacon 双轨道统一向右开放并形成隐藏 C / Forward Gateway 动势；100×100 Master Grid 中 Outer Gap 约 72°、Inner Gap 约 92° | FROZEN |
| AD-FRZ-488 | Outer / Inner Orbit 使用受控不对称，避免 Loading Spinner / Radar 心智；Master Geometry 采用约 -4° / +3° Optical Rotation，Outer Stroke 5u、Inner Stroke 4.5u | FROZEN |
| AD-FRZ-489 | Command Core 最终采用 14u×14u 实心 Diamond，位于 Optical Center；禁止十字、剑、盾、王冠、职阶、令咒或官方 Fate 标志 | FROZEN |
| AD-FRZ-490 | Arc Node 最终固定为单一实心圆节点，位于外轨右上约 48°，Master Diameter 8u；不得宝石化、筹码化或增加多个同权节点 | FROZEN |
| AD-FRZ-491 | 主 Wordmark 固定 `CHALDEA PLATFORM`；CHALDEA 采用 Marcellus 气质基础上的定制字标，PLATFORM 使用轻量 Functional Sans Uppercase，约为主字标 38–42% 视觉量并采用约 0.18em Tracking | FROZEN |
| AD-FRZ-492 | Logo 默认 Bright UI 优先使用 Royal Azure / Ink，媒体或深色场景使用 Chaldea Ivory 单色；允许局部 Arc Node Accent，但 Logo 识别不得依赖 Gradient / Glow / Particle | FROZEN |
| AD-FRZ-493 | Primary Lockup / Compact Lockup / Terminal Glyph / Favicon 四级响应式系统最终保留；Primary 建议最小宽度 140px，Glyph 使用 24/32/40/48px；16px Favicon 简化为粗 C Arc + Diamond 并省略 Arc Node | FROZEN |
| AD-FRZ-494 | Logo Clear Space 以 Command Core 宽度 X 为基准：四周最小净空 1.5X，Beacon 与 Wordmark 间距约 1.25X；角色、赌场装饰、吊灯、标题和背景高亮不得侵入净空 | FROZEN |
| AD-FRZ-495 | Casino / Event / Character Skin 不得修改主 Arc Beacon；活动与游戏可另用 Command Crest / Ceremonial Sigil，主品牌始终保持平台级中立性 | FROZEN |
| AD-FRZ-496 | Status / Functional / Data Visualization 功能色正式采用 A：Muted Semantic Accents / 克制皇家语义色；品牌色、按钮色、状态色与数据色保持职责分离 | FROZEN |
| AD-FRZ-497 | Info / Link 统一使用 Royal Azure `#3568B7`，作为普通信息、链接和非危险强调的首选有彩色 | FROZEN |
| AD-FRZ-498 | Running / Processing 统一使用 Spiritron Teal `#2B7783`；两者通过图标、形态与状态文字区分，不建立第二套运行色 | FROZEN |
| AD-FRZ-499 | Success / Completed / Healthy 统一使用 Chaldea Jade `#2F7D5A`，只承担真实成功与健康语义，不升级为普通按钮或奖励主题色 | FROZEN |
| AD-FRZ-500 | Warning / Maintenance 统一使用 Royal Amber `#9A620F`；两者通过 Icon / Label / Reason / Next Step 区分，不因同色合并业务语义 | FROZEN |
| AD-FRZ-501 | Critical / Severe Failure 统一使用 Command Scarlet `#B5484F`，只局部用于严重异常与高风险语义，不铺满页面、不作为普通 Destructive Button 主题 | FROZEN |
| AD-FRZ-502 | Recovering / Compensating / Reconnecting 等“从异常回到稳定”的中间态统一使用 Spirit Violet `#6E5AA7`，与 Processing / Warning / Success 分离 | FROZEN |
| AD-FRZ-503 | Offline / Paused / Unavailable 的中性不可用状态统一使用 Slate `#6D7480`；Paused 与 Offline 继续通过文字和图标区分 | FROZEN |
| AD-FRZ-504 | Semantic Soft Surface 不新增独立浅色 Hue Token；由权威语义色以约 8–12% Alpha 派生背景、25–35% Alpha 派生边框，文本 / 图标使用 Solid Semantic Color | FROZEN |
| AD-FRZ-505 | Solid Semantic Badge / Status Pill 在需要实底时采用 Solid Semantic Color + Paper White 文本；功能色在 Paper / Surface 上的普通文本对比需达到 WCAG AA 目标 | FROZEN |
| AD-FRZ-506 | 全站状态继续强制 Icon + Shape + Label / Text + Color 多通道表达；任何状态、风险、胜负或可用性不得只依赖颜色 | FROZEN |
| AD-FRZ-507 | 单序列 Data Visualization 默认使用 Royal Azure `#3568B7`；不得为了装饰性丰富而自动增加多色系列 | FROZEN |
| AD-FRZ-508 | Sequential / Intensity Data 使用 Azure 五级 Scale：`#E7EEF8 / #C3D3EB / #95ACD0 / #6F8FC3 / #3568B7`，只用于数量强度而非状态等级 | FROZEN |
| AD-FRZ-509 | Categorical Data 最多使用六个主 Series Color：`#3568B7 / #2B7783 / #6E5AA7 / #9A620F / #A34F78 / #5C7A3D`；颜色仅表示系列身份并必须配直接标签 / Legend / Marker，超过六类改用 Filter / Selector / Small Multiples | FROZEN |
| AD-FRZ-510 | Diverging Palette 仅用于真实存在 Favorable / Neutral / Unfavorable 或正负方向的数据；状态色不得扩张为 Button Theme，Game Win / Loss / Push / Break-even 仍以正式文字与 Net 数值为权威 | FROZEN |
| AD-FRZ-511 | Character / Background / Casino Scene 生产资产采用 **A：Layered Production Pack / 分层生产资产包**；媒体资产与真实 Web UI 永久分层，任何生产图片不得承担余额、按钮、状态、牌局结果或其他权威业务信息 | FROZEN |
| AD-FRZ-512 | 页面视觉资产统一拆为 `background_plate / character_layer / game_object_layer / foreground_atmosphere / Web UI` 五个职责层；前四层为媒体 / 图形资产，最后一层必须由真实 DOM / SVG / Canvas 等前端实现承担业务语义 | FROZEN |
| AD-FRZ-513 | `background_plate` 只承载建筑、空间、灯光、材质与非权威氛围，不烘焙 Logo、按钮、菜单、余额、牌面、下注、排行榜、结果或主要角色 | FROZEN |
| AD-FRZ-514 | Character Asset 以 **Character Skin** 为管理单位，而不是“一页一张完整插画”；每个 Skin 的 V1 最小常态资产为一份 `idle_master`，允许同一 Master 通过 Manifest 定义不同 Desktop / Mobile Focal Point 与 Crop Safe Area | FROZEN |
| AD-FRZ-515 | Direct Play 默认 Character Skin 在 `idle_master` 之外，仅额外要求已冻结的一份 `reaction_visual` + 一份 `reaction_audio`；V1 不为普通状态制作多套表情、多角度或每个结果一张独立角色图 | FROZEN |
| AD-FRZ-516 | Summon 的 Da Vinci 与 Romani 常态人物必须保存为两份独立透明角色资产，以支持 Mobile 独立降级 / 隐藏；Grand / Apex Reaction 仍允许使用一份双角色 Combined Reaction | FROZEN |
| AD-FRZ-517 | Character Master 生产基准冻结为 2:3 Portrait、建议 `2048×3072`、Transparent Alpha、Full Body、No Background / UI / Text / Logo；头顶、手部、武器、披风、脚底等主要轮廓不得被源画布裁切，并保留可安全裁成胸像 / 半身 / 3/4 身的 Alpha Margin | FROZEN |
| AD-FRZ-518 | 不默认导出大量 Desktop Crop / Mobile Crop 位图；优先由 Asset Manifest 保存 `desktop_focal_point / mobile_focal_point / desktop_crop_safe / mobile_crop_safe` 并由 CSS / 前端裁切，只有单一 Master 无法满足响应式构图时才制作第二 Composition Variant | FROZEN |
| AD-FRZ-519 | 具有明确场景身份的 P3 / Full-bleed Background Plate 生产基准冻结为 Desktop Master `≥2560×1440`、Mobile Master `≥1440×1920`；Mobile 必须允许独立构图，不以 PC 中心硬裁替代；Tablet 原则上从 Desktop Master 自适应 | FROZEN |
| AD-FRZ-520 | Public Home / Login / Registration / Master Initialization 使用 Royal Observatory 场景家族：Public Home 可选角色、Login 固定 Saber、Registration 固定 Mash、Master Initialization 使用 Mash + Da Vinci；Dashboard 不强制 Raster Background，仅允许低强度可选角色复用 | FROZEN |
| AD-FRZ-521 | Models / API / Wallet / Rewards / History 等长期工具页原则上不新增 Full-bleed Raster Scene；Model Persona 继续作为独立内容资产体系，Wallet / Rewards 不绑定固定角色主视觉 | FROZEN |
| AD-FRZ-522 | Entertainment 与五款 Direct Play 的正式场景资产矩阵冻结为：Grand Casino Atrium、Lucky Dice Salon、Treasure Voucher Salon、Grand Manifestation Theatre、King’s Treasury Slot Gallery、Casino Camelot VIP Royal Table；对应 Character / Game Object / Reaction 按各游戏已冻结角色映射独立管理 | FROZEN |
| AD-FRZ-523 | Poker Lobby 使用低强度 High Roller Reception / Lounge，Poker Table 使用 Macau-style High Roller Poker Salon；Poker 不绑定固定 Heroic Persona。Operations 不使用 Raster Scene 或角色，只保留 Royal Beacon、Functional Icon 与高密度管理 UI | FROZEN |
| AD-FRZ-524 | 全部 Casino Background 归属于同一 **Casino Camelot Grand Resort Architecture Family**，共享 Royal / Classical Architecture、Macau Integrated Resort Scale、Grand Atrium、Arch、Ceiling、Chandelier、Marble、Patterned Carpet、Mirror / Polished Metal、Warm White Lighting 与受控 Gold / Burgundy DNA，同时各厅保持明确空间身份 | FROZEN |
| AD-FRZ-525 | Casino 子场景差异冻结：Blackjack = 最皇家旗舰 VIP Table；Poker = 安静成熟 High Roller Salon；Slot = 明亮 King’s Treasury Gallery；Dice = Lucky Dice Salon；Scratch = Prize Counter / Treasure Lounge；Summon = Grand Manifestation Theatre，不强行赌场桌游化 | FROZEN |
| AD-FRZ-526 | `background_plate` 默认不含可辨识主要人物；允许极弱、不可识别的远景人群氛围，但 V1 推荐主 Background 无可辨识人物，以确保 Character Skin、Mobile Crop、Reduced Media 与加载失败回退可独立处理 | FROZEN |
| AD-FRZ-527 | Game Object 必须与 Background 分层：Blackjack / Poker Table、Dice Tray、Slot Cabinet、Scratch Card、Summon Array / Core 等不得把结果依赖内容烘焙进场景；牌面、Reel Symbol、Scratch Reveal、下注、Result 等继续由真实前端状态驱动 | FROZEN |
| AD-FRZ-528 | 源资产与 Web 交付资产分离：透明人物源使用 Lossless Alpha Master，网页人物优先 WebP Alpha + Fallback；背景 Web 交付优先 AVIF + WebP Fallback；Vector 使用 SVG；短 Reaction 可使用透明 WebM / 等价格式 + Static WebP Fallback；精确编码质量与 KB 上限留到 Media Budget 批次冻结 | FROZEN |
| AD-FRZ-529 | 所有生产媒体统一进入 Asset Manifest，至少记录 `asset_id / domain / scene / character / skin / role / version / source / fallback / desktop_focal_point / mobile_focal_point / safe_area / alpha / status / rights_note / prompt_archive_id`；状态统一为 `PLANNED / GENERATED / REVIEWED / SELECTED / PRODUCTION_READY / REJECTED` | FROZEN |
| AD-FRZ-530 | 物理文件统一 lowercase kebab-case + 语义前缀 + `vNNN` 版本号；FGO / Casino Camelot / 澳门赌场只作为方向参考，生产资产必须新生成 / 新绘制 / 新设计，不直接使用官方 FGO 图、官方活动背景 / UI、现实赌场 Logo 或标志性室内照片作为生产文件 | FROZEN |
| AD-FRZ-531 | Model Persona 最终审核采用 **Persona Review A — Select All Nine**；Gemini、GPT、Claude、DeepSeek、GLM、Kimi、文心一言 / ERNIE、千问 / Qwen、Grok 九个 Family Master 全部由 `GENERATED / REVIEWED` 推进为 `SELECTED` | FROZEN |
| AD-FRZ-532 | 九个 Family Master 的人物方向全部通过，不存在因角色设计、家族辨识或当前 Model Card / Detail 适配而需要 `REJECTED` 或重新选方向的 Persona | FROZEN |
| AD-FRZ-533 | 九张当前 Selected Design Master 均确认存在真实 RGBA Transparent Alpha，不属于黑底 / 灰底伪透明；人物可以继续作为独立 Model Identity Asset 使用 | FROZEN |
| AD-FRZ-534 | 当前九张 `1024×1536` 资产统一定义为 **Selected Design Master v1**，只确认人物设计与可用构图，不直接标记 `PRODUCTION_READY` | FROZEN |
| AD-FRZ-535 | Persona 正式进入生产前统一执行 **Non-destructive Normalize**：目标保持 2:3 Portrait，建议提升至 `2048×3072` 或等价高分辨率，保留人物身份、脸、服装、道具、姿势与 Prompt 设计，不把 Normalize 变成重新设计 | FROZEN |
| AD-FRZ-536 | Normalize 后 Alpha Safety Margin 目标为人物有效轮廓距画布四边至少约 `5%`；长发、尾巴、武器、宽裙摆等关键长轮廓优先争取 `6–8%`，避免浏览器裁切和缩放时贴边 | FROZEN |
| AD-FRZ-537 | 九个 Persona 默认不额外制作 Card 专用半身图；Model Card 与 Model Detail 继续共享同一 Master，通过 Asset Manifest 的 `desktop_focal_point / mobile_focal_point / crop_safe` 控制响应式裁切 | FROZEN |
| AD-FRZ-538 | ERNIE 轮椅、Grok 大型武器等宽轮廓不因横向占位较大自动触发第二 Composition Variant；只有同一 Normalize Master 无法同时满足信息安全区与人物识别时，才允许建立第二构图版本 | FROZEN |
| AD-FRZ-539 | Persona Normalize 必须保持非破坏性：不得借机改脸、换服装、改家族身份、删除核心道具或重写角色方向；如放大 / 清理后质量仍不足，只允许依据原 SELECTED Prompt 与人物方向重建同款 Master | FROZEN |
| AD-FRZ-540 | Grok Persona 保持 `SELECTED`，但其大型黑金武器内部的显著 X-like 几何进入最终 Rights / Trademark Audit；如判定过度接近品牌标志，只允许微调武器内部几何，不因此重新设计或 Reject Grok Persona | FROZEN |
| AD-FRZ-541 | Model Persona 的身份边界再次锁定：`Model Persona ≠ FGO / Fate Servant ≠ Master Avatar ≠ Poker Player Avatar ≠ Game Host`；九个 Selected Persona 只服务 Model Identity / Catalog / Detail 等模型识别场景 | FROZEN |
| AD-FRZ-542 | `Chaldea_Platform_Model_Persona_Image_Prompts_WORKING_v0.6.md` 作为本轮同步 Prompt 档案：九个 v1 Prompt 原文保持不变，仅将实际采用状态更新为 `SELECTED` 并追加 Normalize / Production Readiness Notes | FROZEN |
| AD-FRZ-543 | `PRODUCTION_READY` 必须晚于 Normalize、Alpha Margin、关键 Crop、Web 交付格式、Rights Note 与实际页面装载复核；本轮九个 Persona 统一为 `SELECTED / NOT PRODUCTION_READY` | FROZEN |
| AD-FRZ-544 | Motion 最终方向采用 A：Quiet Royal Motion / 克制皇家动效；普通产品交互快速简约，赌场氛围不以拖慢操作实现 | FROZEN |
| AD-FRZ-545 | M0～M4 时长冻结：M0=0ms；M1=80–140ms；M2=160–240ms；M3=280–600ms；M4=700–1600ms；普通 UI 常态不超过 240ms | FROZEN |
| AD-FRZ-546 | 全站 Motion Easing 仅保留 Standard `cubic-bezier(0.2,0,0,1)`、Enter `cubic-bezier(0,0,0.2,1)`、Exit `cubic-bezier(0.4,0,1,1)` 三条权威曲线，不使用 bounce / elastic / spring overshoot | FROZEN |
| AD-FRZ-547 | 基础组件时长冻结：Button 100ms、Tab/Segmented 120ms、Tooltip 100ms、Focus Ring 80ms、Dropdown 140ms、Dialog 180ms、Drawer/Bottom Sheet 220ms、Toast Enter 180ms / Exit 140ms、Route Content 180ms | FROZEN |
| AD-FRZ-548 | Flat Button Motion 固定只改变 Fill / Border / Text / Focus；Hover / Press 不使用 translateY、scale、浮起、下沉、Bevel 或弹性位移 | FROZEN |
| AD-FRZ-549 | 普通 Route 仅使用约 180ms Fade + 最多 4px 内容进入位移；工具页可只 Fade；Background Plate 不随页面做大幅 Parallax / Camera Travel | FROZEN |
| AD-FRZ-550 | Royal Beacon Motion 只允许在首次品牌进入 / Splash / 明确 Brand Intro 播放一次，约 900–1200ms；常态 Header Logo 永久静态 | FROZEN |
| AD-FRZ-551 | Dice 演出冻结：Roll / Settle 650–900ms、Result Reveal 180–240ms、常规总时长≤1.1s；时长不得随结果价值变化 | FROZEN |
| AD-FRZ-552 | Scratch 手动刮开保持 Pointer / Touch 用户驱动；Reveal All 统一 350–550ms，揭示曲线不得按中奖结果改变 | FROZEN |
| AD-FRZ-553 | Summon 演出冻结：Single 常规 1.1–1.6s，T4/T5 可追加 400–700ms Phenomenon，但非 Reaction 稳定结果硬上限 2.2s；Tenfold Tile stagger 90–140ms、完整约 1.5–2.5s，并始终支持 Skip / Reveal All | FROZEN |
| AD-FRZ-554 | Slot 正常 Reel Motion 1.2–1.7s，固定 Stop 间隔 120–160ms / Reel；Fast Stop 只缩短剩余演出，点击后剩余动画≤350ms，不改变锁定 Grid | FROZEN |
| AD-FRZ-555 | Blackjack 动效冻结：单张 Deal 100–160ms、初始发牌约 450–700ms、Hit 到稳定可读状态≤300ms、Split Hand Separation 180–240ms；不得用长停顿制造赌场悬念 | FROZEN |
| AD-FRZ-556 | Poker 动效冻结：Card Deal 100–160ms、Chip/Bet 120–180ms、Pot Reconcile 180–300ms、Winner Arc 250–450ms、Seat State 100–160ms；Action Timer 禁止闪烁、Shake 与大幅呼吸缩放 | FROZEN |
| AD-FRZ-557 | Character Reaction Visual 建议 1.2–1.8s、硬上限 2.2s；Reaction Audio 同步建议 1.2–1.8s、硬上限 2.2s；单次、非循环、可 Skip、不阻塞正式 Result，具体动作 / 台词 / 声线 / 业务触发阈值仍保持 OPEN | FROZEN |
| AD-FRZ-558 | `prefers-reduced-motion: reduce` 下 M1 只保留≤100ms Color/Border/Focus，M2 Overlay 改≤120ms Fade，M3/M4 移除长距离运动、粒子、Parallax 与 Cut-in Motion，结果以≤150ms Fade + 静态状态呈现；Reduced Motion 不自动等于 Mute | FROZEN |
| AD-FRZ-559 | 建立独立 Reduced Media Mode：不加载 Reaction Video / Foreground Atmosphere Video / 非必要 Casino Media，Character 与 Background 使用静态压缩资产；不改变任何业务能力 | FROZEN |
| AD-FRZ-560 | Web 媒体预算冻结：Card Persona 512–768px 长边、典型≤250KB；Detail Persona 1024–1536px 长边、典型≤500KB；Desktop Hero AVIF≤550KB / WebP≤800KB；Mobile Hero AVIF≤400KB / WebP≤600KB；Transparent Reaction WebM≤1.5MB target、>2.5MB 必须 Review；Static Reaction WebP≤350KB；功能 SVG 典型≤12KB、复杂品牌/仪式 SVG≤80KB | FROZEN |
| AD-FRZ-561 | 首屏关键视觉传输目标冻结：Mobile≤800KB、Desktop≤1.2MB；只 Preload Functional UI Font、当前页面真实 LCP Hero/Background 与必要时唯一首屏 Character；后续 Persona / Reaction / Casino Scene 使用 Lazy Load，不跨游戏批量预载 | FROZEN |
| AD-FRZ-562 | V1 Background Plate 默认静态；禁止常驻赌场视频背景、无限循环粒子视频、常驻 Live2D / WebGL Casino Lobby；短 Reaction / Phenomenon 可使用视频但必须有 Static Fallback | FROZEN |
| AD-FRZ-563 | 最终生产门采用 A：Accessible Production Gate / 可访问生产门；Art Direction 只有同时通过 Accessibility / Performance / Rights 三类 Gate 才可标记 PRODUCTION_READY | FROZEN |
| AD-FRZ-564 | Accessibility 基准采用 WCAG 2.2 AA；普通文字对比度≥4.5:1，大字号文字≥3:1，UI Component / Icon 与 Focus Indicator≥3:1；复杂 Casino / Character 背景不得降低真实 UI 可读性 | FROZEN |
| AD-FRZ-565 | 所有正式业务能力必须支持纯键盘完成；保留高可见 3px Royal Azure Focus Ring；Dialog / Drawer / Sheet 打开时正确管理 Focus Trap，关闭后返回触发控件；禁止 Hover-only 功能 | FROZEN |
| AD-FRZ-566 | Semantic HTML First：优先使用 button / a / input / select / table / heading / list / dialog 等原生语义；ARIA 只用于补充而非修复错误 DOM；字段错误必须与对应 Label / Description / Error 建立程序化关联 | FROZEN |
| AD-FRZ-567 | Submitting / Processing / Completed / Failed / Recovering 等异步状态使用适度 Live Region；Poker Action Timer 禁止每秒向 Screen Reader 播报倒计时，只在轮到玩家与关键阈值提供必要提示 | FROZEN |
| AD-FRZ-568 | Dice / Scratch / Slot / Blackjack / Poker 等游戏的权威结果必须存在完整文本等价物，至少覆盖结果对象、关键数值、Win/Loss/Push/Break-even、Payout / Net Change；动画、位置、颜色与光效只属于 Presentation | FROZEN |
| AD-FRZ-569 | Decorative Casino Background / Chandelier / Atmosphere / Ornament / Decorative Servant Host 默认使用空 Alt / aria-hidden；Model Persona 可使用简短家族身份 Alt，但不得替代 Model Name / ID / Availability / Pricing 等正式信息 | FROZEN |
| AD-FRZ-570 | 普通产品页面支持 200% Browser Zoom 与约 320 CSS px 宽度核心内容 Reflow；核心操作不得依赖横向滚动；游戏 Stage 可特殊重排但 Wager / Action / Result 必须保持可达；最低 Touch Target 继续≥44px | FROZEN |
| AD-FRZ-571 | Chart Accessibility 固定要求 Series Label / Legend + Marker / Pattern / Direct Label + 关键数值文本摘要；重要业务数据须能通过结构化摘要或对应表格取得，任何图表不得仅依赖颜色 | FROZEN |
| AD-FRZ-572 | AD-FRZ-558 / 559 的 Reduced Motion 与 Reduced Media 规则升级为 Production Gate：关闭长动画、Reaction Video 或复杂 Casino Media 时仍必须完整显示业务状态、结果、Action 与错误恢复能力 | FROZEN |
| AD-FRZ-573 | Performance Production Target 冻结为 LCP≤2.5s、INP≤200ms、CLS≤0.1；以真实生产环境主要用户体验为最终判定，不以 localhost / 高性能开发机成绩替代线上验证 | FROZEN |
| AD-FRZ-574 | Persona / Hero Background / Casino Stage / Game Object / Reaction Placeholder 必须预留稳定 width / height / aspect-ratio 或等价容器；Skeleton 与最终几何相近，禁止媒体加载完成后把 Pricing、Form、Wager、Action 或 Result 大幅顶移 | FROZEN |
| AD-FRZ-575 | 字体生产策略冻结为 Self-host WOFF2 Preferred + font-display: swap；只 Preload 首屏真正需要的 Functional Font，Serif / Decorative CJK 按需加载；任何 Subset 必须保留 License、必要字形并避免缺字导致巨大 Layout Shift | FROZEN |
| AD-FRZ-576 | 所有媒体失败必须 Graceful：Persona→Model Glyph / Family Geometry，Casino Background→Solid/CSS Surface，Reaction Video→Static Frame 或 Stable Result，Audio→Silent Result，Character→无角色仍可完整操作；媒体失败不得阻断业务 | FROZEN |
| AD-FRZ-577 | Asset Manifest 增加 Rights Status：ORIGINAL_PLATFORM / ORIGINAL_GENERATED / THIRD_PARTY_CHARACTER_DERIVED / LICENSED_OR_APPROVED / REFERENCE_ONLY / RIGHTS_REVIEW_REQUIRED；每个生产资产记录 source、creator/generator、reference、rights_status、rights_note、review_date | FROZEN |
| AD-FRZ-578 | Route B / 自行生成或自行重绘只代表“不直接复制官方素材”的生产方法，不自动等于法律权利已清除；可识别第三方 FGO / Fate 角色在公开运营或商业化前仍须由项目所有者确认合法使用基础 | FROZEN |
| AD-FRZ-579 | Production 明确禁止直接复用 FGO 官方角色立绘 / 活动背景 / UI / Logo / 召唤阵 / 音乐 / 角色语音、官方声优声音克隆、未授权字体、澳门真实赌场 Logo / 室内照片、用户提供的第三方模型娘参考原图；这些仅可作 Reference | FROZEN |
| AD-FRZ-580 | 九个 Selected Model Persona 进入 Provider Trademark / Rights Spot Check：检查 Provider Logo、胸章、能力符号、既有非官方二创相似度与 Grok X-like Weapon；如存在局部风险优先微调敏感 Symbol，不重做已 SELECTED 人物身份 | FROZEN |
| AD-FRZ-581 | QA / Codex Screenshot / Review Package 必须使用 Fixture / Demo Data；禁止包含 API Secret、OAuth / Discord Token、完整真实 Account ID、真实用户资料、私密 Prompt / Response 或未公开 Poker Hole Cards | FROZEN |
| AD-FRZ-582 | 统一 Production Release Gate 冻结：Visual、IA/Business Contract、Responsive、Keyboard、Screen Reader Spot Check、Contrast、Reduced Motion、Performance Budget、Core Web Vitals Target、Asset Manifest、Rights Review、Fallback/Media Failure 全部 PASS 后才能标 PRODUCTION_READY；任一 FAIL 时只能保持 SELECTED / IMPLEMENTED / REVIEW_REQUIRED | FROZEN |

---

# Appendix B — FINAL Release Note

- `WORKING v0.37` 是最后一个累计历史稿。
- 本 FINAL 主正文只描述当前生效体系；历史冲突只保留在 Appendix A。
- Casino A、Bright Moonlit、Button 三色范围、Royal Beacon、Semantic / Data Color、Layered Production Pack、Persona Select All Nine、Quiet Royal Motion、Accessible Production Gate 均为最终生效方向。
- 本 FINAL 不删除任何上游 IA / Requirement 边界，也不将未完成的媒体生产或 Rights Review 冒充为 `PRODUCTION_READY`。
