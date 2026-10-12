# 视觉 A Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 全站统一导航栏、统一按钮规范、清理内部术语与编号，并上线已完成的结算闪烁修复。

**Architecture:** 新增一个 `SiteHeader` 组件取代五份手写导航；在 `styles.css` 建立唯一的按钮规范并删除各页面 CSS 中的按钮外观；文案只改前端显示，后端与数据不动。分两批发布：批 1（闪烁 + 导航），批 2（按钮 + 文案 + 对比度）。

**Tech Stack:** React 19、react-router-dom 7、Vite 8、Vitest 4 + Testing Library、纯 CSS（无预处理器）。

**Spec:** `docs/superpowers/specs/2026-10-12-visual-polish-a-design.md`

## Global Constraints

- 工作目录 `web/`；所有命令在 `web/` 下运行。
- 不修改 `web/src/ops/**`、`web/src/Channels.tsx`（运营/管理员页面）。
- 不修改首页 `Home.tsx` 的布局与配色（仅批 2 的按钮规范自然作用于它）。
- 不改后端、Go 代码、数据库、游戏规则。
- 术语：保留 "Reserve"、"Master"；删除 "原生"；"原生额度/原生单位/原生可用" → "API 额度"（单位文字删除）。
- 按钮颜色：`--gold: #d9bf86; --gold-line: #d9bf8666; --ink-on-gold: #14243a; --text-strong: #f0dcae; --text-body: #e8dcc2; --text-soft: #c0d0e4; --control-h: 40px; --control-radius: 4px;`，≤720px 宽时 `--control-h: 44px`。
- 筹码"余额"只显示整数、向下取整，`title` 给完整值；下注额、派彩、净变化保持现有精度。
- 导航高度固定 68px；标志统一为 `Brand`。
- 提交作者：`git -c user.name=cy4268 -c user.email=325177413+cy4268@users.noreply.github.com commit`；提交信息末尾 `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`。
- 每批完成标准：`npx vitest run` 全绿、`npm run build` 成功。部署前单独征得用户同意。

## Review Focus

1. 从公共页（/games、/models、/rankings、/announcements）进入 Shell 页（/dashboard、/games/dice）时，头像菜单名字不能先显示"正在读取 Master"再变回名字——`SiteHeader` 在两种布局中重新挂载，必须用缓存的上次名字首帧渲染（Task 1 测试 `renders cached master name on remount`）。
2. 刷新页面会话未就绪时，右侧不能出现"登录账户"再变"个人中心"（Task 1 测试 `never shows sign-in text before session is ready`）。
3. 余额 `0.99999` 筹码显示为 `0`，"可用筹码不足"等判断仍以精确值进行（Task 6 测试 `floors balance display but keeps exact validation`）。
4. 删除各页面按钮外观后，选中态（`aria-pressed="true"`、`aria-current`）仍可分辨（Task 4 基础样式定义 + lint 测试允许清单）。
5. 未登录用户在公共页看到的子导航链接指向需要登录的页面时，进入登录门而不是空白（Task 2 测试沿用现有 `LoginRequired` 行为，断言 `/games/dice` 未登录显示登录入口）。

---

## 批 1：结算闪烁 + 统一导航

### Task 1: SiteHeader 组件

**Files:**
- Create: `web/src/SiteHeader.tsx`
- Create: `web/src/site-header.css`
- Test: `web/src/SiteHeader.test.tsx`

**Interfaces:**
- Produces:
  - `export type NavDomain = 'home'|'models'|'experience'|'rankings'|'announcements'|'assets'|'my'|null`
  - `export function navDomain(pathname: string): NavDomain`
  - `export function SiteHeader({ client }: { client: ApiClient }): JSX.Element`

- [ ] **Step 1: Write the failing tests** in `SiteHeader.test.tsx`（参照 `App.test.tsx` 的 `ApiClient` + `getSnapshot` mock 方式）：
  - `navDomain maps every route family`：`/dashboard→home`；`/models`、`/models/x`、`/api/access`、`/keys`、`/logs→models`；`/games`、`/entertainment`、`/games/dice`、`/poker`、`/poker/t/1`、`/roulette/devil`、`/history`、`/history/rounds/x→experience`；`/rankings→rankings`；`/announcements`、`/announcements/1→announcements`；`/wallet`、`/wallet/activate`、`/wallet/transactions/x`、`/rewards→assets`；`/me`、`/account`、`/account/security`、`/master-profile→my`；`/` → `null`。
  - `signed-in header has the same links on every page`：对 `/dashboard`、`/games`、`/models`、`/rankings`、`/announcements` 分别渲染，主导航链接文字序列都等于 `['指挥台','模型目录','娱乐','排行榜','公告']`，且都有"资产"链接和"账户菜单"按钮。
  - `signed-out header`：主导航 `['首页','模型目录','娱乐','排行榜','公告']`，右侧有"登录"链接，无"资产"。
  - `marks the current domain`：在 `/games/dice` 时"娱乐"链接 `aria-current="page"`，其余无。
  - `never shows sign-in text before session is ready`：`ready:false` 时不出现"登录"/"个人中心"/"登录账户"文字，存在 `data-testid="account-placeholder"`。
  - `renders cached master name on remount`：首次渲染 mock `readProfile` 返回 `{status:'COMPLETE',display_name:'墨恒'}` 并等待出现；卸载后让 `readProfile` 返回永不 resolve 的 Promise 再渲染，账户按钮立即包含"墨恒"，且不包含"正在读取"。
  - `account menu hides internal identity`：展开菜单后不包含 "原生登录身份" 与 `user.username`。
  - `shows the domain sub-navigation`：在 `/games` 时页面导航包含 `['游戏目录','命运骰盅','星纹刮刮卡','圣晶召唤','月光回响','二十一点','Poker 大厅','游戏记录']`；在 `/models` 时包含 `['发现模型','API 接入','密钥管理','调用记录']`；在 `/rankings` 时无页面导航。

- [ ] **Step 2: Run** `npx vitest run src/SiteHeader.test.tsx` — Expected: FAIL（模块不存在）。

- [ ] **Step 3: Implement `SiteHeader.tsx`**
  - 会话：`useSyncExternalStore(client.subscribe, client.getSnapshot)`。
  - 账户菜单逻辑（展开、点外部关闭、Esc 归还焦点、ops 入口探测、退出登录）从 `App.tsx` `Shell`（约 172–206 行）原样迁入；菜单删去 "原生登录身份" 一行；按钮小字改为 `role(user.role)`。
  - Master 名字：`readProfile(client, String(user.id))`，结果写入模块级 `const masterNames = new Map<number,string>()`；渲染优先级：本次结果 → `masterNames.get(user.id)` → `user.display_name || user.username`。不渲染"正在读取 Master"等过程文字。
  - 子导航 `contextLinks` 从 `Shell` 迁入，按 `navDomain` 选择；`experience` 也用于 `/games`、`/entertainment`。
  - 结构/类名：`header.site-header > a.site-brand(Brand) + nav.site-nav[aria-label=主导航] + div.site-account`；子导航 `nav.site-subnav[aria-label=页面导航]`。会话未就绪时 `site-account` 内只渲染 `<span data-testid="account-placeholder" class="site-account-placeholder"/>`。
  - `site-header.css`：高度 68px、背景 `#081321`、底边 `1px solid #ffffff14`、内容宽 `min(1200px, calc(100% - 48px))` 居中；链接 13px、颜色 `--text-soft`、当前项金色 2px 下划线；占位宽 180px；≤720px 隐藏 `.site-nav`（沿用底部 `mobile-nav`）。新类名不与旧 `.portal-header` 等重名，旧页面覆盖样式不会命中它。

- [ ] **Step 4: Run** `npx vitest run src/SiteHeader.test.tsx` — Expected: PASS。

- [ ] **Step 5: Commit** `feat(nav): add shared SiteHeader`。

### Task 2: 五处导航替换为 SiteHeader，删除旧导航代码与样式

**Files:**
- Modify: `web/src/App.tsx`（`Shell`：删除内联 header、`contextLinks`、账户菜单；渲染 `<SiteHeader client={client}/>`；保留 `mobile-nav`、`master` 资源与 Outlet context、`document.title` 逻辑）
- Modify: `web/src/Games.tsx`（`GamesCatalog` 约 73–77 行）
- Modify: `web/src/Catalog.tsx`（`CatalogShell` 约 22–26 行，含其 `portal-context`）
- Modify: `web/src/rankings/Rankings.tsx`（约 71–75 行）
- Modify: `web/src/Announcements.tsx`（`AnnouncementShell` 的 `public-header`）
- Modify: 只服务于上述旧导航的 CSS 规则：`entry.css`、`styles.css`、`game-hall.css`、`catalog-workshop.css`、`rankings/rankings.css`、`announcements.css`、`announcement-chamber.css`、`command-personal.css`、`api-workbench.css`、`wallet-chamber.css`、`history/history.css`、`games/*-stage.css`、`poker/poker-salon.css`、`roulette/roulette-devil.css`、`admission.css`
- Test: `web/src/App.test.tsx`、`web/src/GamesCatalog.test.tsx`、`web/src/Catalog.test.tsx`、`web/src/Announcements.test.tsx`、`web/src/rankings/*.test.tsx`（更新断言旧导航文字的部分）

**Interfaces:**
- Consumes: `SiteHeader({client})`（Task 1）

- [ ] **Step 1: Write the failing test** in `App.test.tsx`：`one header across public and signed-in pages` —— 已登录状态依次渲染 `/dashboard`、`/games`、`/models`、`/rankings`、`/announcements`，每页恰好一个 `banner` 角色元素且其 `className` 为 `site-header`，主导航链接序列相同；未登录渲染 `/games/dice` 时出现现有 `LoginRequired` 的登录入口。

- [ ] **Step 2: Run** `npx vitest run src/App.test.tsx` — Expected: FAIL（各页仍是旧 header）。

- [ ] **Step 3: Replace the five headers** with `<SiteHeader client={client}/>`；删除 `GamesCatalog` 的 `hall-brand`/`hall-account`、`CatalogShell` 的 `portal-context`（由 SiteHeader 子导航取代）、`Rankings` 的 `ranking-account`、`AnnouncementShell` 的 `public-header`。`Home.tsx` 不动。

- [ ] **Step 4: Delete dead CSS**：删除选择器含 `.portal-header`、`.portal-global`、`.portal-account`、`.portal-context`、`.ranking-header`、`.ranking-account`、`.hall-brand`、`.hall-account`、`.asset-shortcut` 的规则；`.public-header` 规则仅在确认 `Home.tsx`/`Authentication.tsx` 不再使用后删除（`grep -rn "public-header" src --include=*.tsx`）。`.account-button`、`.account-menu`、`.avatar` 若被 SiteHeader 复用则保留。

- [ ] **Step 5: Update existing tests** that asserted removed text（"登录账户"、"个人中心"导航按钮、旧导航结构），只改断言对象，不删除其行为断言。

- [ ] **Step 6: Run** `npx vitest run` 与 `npm run build` — Expected: 全部 PASS，构建成功；`grep -rn "portal-header\|portal-global\|hall-account\|ranking-account" src` 无结果。

- [ ] **Step 7: Commit** `refactor(nav): render SiteHeader on every page and drop per-page headers`。

### Task 3: 批 1 发布

**Files:**
- Create: `E:\Programs\vps\.codex-tmp\MOMIAO\publish\visual-polish-20261012\batch1\` 下 `MODIFIED_FILE.json`、`DIFF_FILE.patch`、`VERIFICATION.txt`、`ROLLBACK.sh`

- [ ] **Step 1:** 复读上次前端发布证据 `E:\Programs\vps\private\website-game-link-b-20261003\site\evidence\guangxi-deploy-20261009\`（VERIFICATION.txt、ROLLBACK.sh）与 `docs/deployment.md`，按同一方式：以当前生产镜像 `momiao:guangxi-20261009` 为底，只替换 `/app/web` 前端产物，后端二进制哈希不变；Compose 仅改 `services.platform.image`。
- [ ] **Step 2:** 生成新镜像标签 `momiao:visual-a1-20261012`，写四件套；在独立副本上执行一次 copy rollback，确认恢复原配置哈希。
- [ ] **Step 3: STOP — 向用户报告并征得部署同意。**
- [ ] **Step 4:** 部署（platform/edge 配对重建），验证：容器 healthy；公网首页与新 JS/CSS 返回 200；内置浏览器逐页实测 5 个页面导航高度均 68、链接位置一致；骰子页一局（10 筹码）逐帧记录结算卡节点不被移除、输局卡为深色。任一不符即执行回滚。
- [ ] **Step 5:** 推送分支 `codex/visual-polish-a` 到 GitHub（不合并 main，除非用户要求）。

---

## 批 2：按钮规范 + 文案清理 + 对比度

### Task 4: 按钮规范与各页面按钮外观清理

**Files:**
- Modify: `web/src/styles.css`（`:root` 变量；`button,.button`、`.primary`、`a` 文字链接、`:hover/:focus-visible/:disabled`、`[aria-pressed="true"]`、`[aria-current]` 基础样式；把 `.game-play`、`.extra-primary`、`.pk-primary` 并入 `.primary` 选择器组）
- Modify: 除 `ops/` 外含按钮外观声明的 CSS（约 28 个，清单：`poker/poker-ui.css`、`poker/poker-salon.css`、`roulette/roulette.css`、`roulette/roulette-devil.css`、`roulette/roulette-pressure.css`、`games/blackjack-stage.css`、`games/direct-extra-games.css`、`games/slot-stage.css`、`games/scratch-stage.css`、`games/summon-stage.css`、`games/dice-stage.css`、`games/direct-salon.css`、`wallet-chamber.css`、`entry.css`、`command-personal.css`、`catalog-workshop.css`、`api-workbench.css`、`announcement-chamber.css`、`rankings/rankings.css`、`games.css`、`identity-chamber.css`、`game-hall.css`、`history/history.css`、`catalog.css`、`announcements.css`、`admission.css`、`home.css`）
- Modify: `*.tsx`（除 `ops/`、`Channels.tsx`）按钮与链接文字中的 `→` 删除；外部链接保留 `↗`
- Test: Create `web/src/button-style.test.ts`

**Interfaces:**
- Produces: CSS 变量 `--gold --gold-line --ink-on-gold --text-strong --text-body --text-soft --control-h --control-radius`（Task 7 使用）

- [ ] **Step 1: Write the failing test** `button-style.test.ts`（node 环境读文件）：
  - `no page stylesheet restyles buttons`：遍历 `src/**/*.css`，排除 `styles.css`、`ops/**`、`site-header.css`；用 `/([^{}]+)\{([^{}]*)\}/g` 取规则；若选择器匹配 `/\bbutton\b|\.button\b|\.primary\b|\.game-play\b|\.extra-primary\b|\.pk-primary\b/` 且声明含 `/(^|;)\s*(background(-color|-image)?|border(-radius|-color)?|color|box-shadow|height|min-height|font(-size|-weight)?)\s*:/`，记为违规；允许清单（选择器包含即放行）：`.mobile-nav`、`.site-`、`.wager-quick`（仅允许 `min-height`）、`.game-choice`、`.spirit-`、`.crystal-`、`.roulette-chamber`。断言违规列表为空，失败时打印"文件: 选择器"。
  - `no decorative arrows in player-facing labels`：遍历 `src/**/*.tsx`，排除 `ops/**`、`Channels.tsx`、`*.test.tsx`；断言不存在 `/ →<\/|→<\/(a|Link|button)>| →'|→'/` 形式的按钮/链接尾箭头（`← 返回` 类前置箭头允许）。

- [ ] **Step 2: Run** `npx vitest run src/button-style.test.ts` — Expected: FAIL，列出违规。

- [ ] **Step 3: Implement** 基础样式于 `styles.css`；逐文件删除违规声明（只删外观，保留布局）；删除尾箭头；每屏只留一个 `.primary`（游戏页开局按钮、指挥台"选择模型并接入"；其余 `.primary` 改为默认次按钮）。
- [ ] **Step 4: Update tests** 中按钮可访问名包含 "→" 的查询（如 `'进入游戏 →'` → `'进入游戏'`）。
- [ ] **Step 5: Run** `npx vitest run` 与 `npm run build` — Expected: 全部 PASS。
- [ ] **Step 6: Commit** `style: one button system across player pages`。

### Task 5: 文案清理

**Files:**
- Modify: `App.tsx`（指挥台：额度卡、底部"原生登录身份"、"账户短 ID"、重复说明）、`PersonalHub.tsx`、`Account.tsx`（保留身份信息，改标为"登录名""账户编号"）、`Wallet.tsx`/`WalletChamber.tsx`/`WalletActions.tsx`/`QuotaActivation.tsx`/`Rewards.tsx`/`DailyReward.tsx`/`RewardPrograms.tsx`/`Keys.tsx`/`RPUsage.tsx`/`MasterProfile.tsx`/`history/*.tsx` 中含"原生""Reserve + Active""最小步长"的玩家可见文字；钱包交易列表删除 UUID 列内容（详情页保留）
- Modify: 113 处 `eyebrow`：每页仅保留页面大标题上方一行，去掉 "CHALDEA / "、"MY / " 等路径前缀，删除区块级 eyebrow 元素
- Modify: `Games.tsx` `RoundReceipt` 与 `economy-cap.tsx` `CapReceipt`：仅当 `receipt.capped_units`（以实际字段名为准，读 `economy-cap.tsx` 的 `CapSettlement` 类型）大于 0 时渲染经济回执
- Test: Create `web/src/copy.test.ts`；更新受影响的组件测试

- [ ] **Step 1: Write the failing tests**：
  - `copy.test.ts` → `player pages never say 原生`：遍历 `src/**/*.tsx`，排除 `ops/**`、`Channels.tsx`、`*.test.tsx`，断言源码字符串字面量中无 "原生"。
  - `Games.test.tsx` → `hides the economy cap row when nothing was capped`：`RoundReceipt` 传入未封顶的 `economy_settlement`，`queryByRole('group',{name:'经济结算回执'})` 为空；封顶时存在。
  - `App.test.tsx` / `PersonalHub.test.tsx` → 指挥台与个人中心不包含 "原生登录身份"、`user.username`、"账户短 ID"；`Account.test.tsx` → 包含"登录名"与 `user.username`。
  - `Wallet.test.tsx` → 交易列表不包含交易 UUID 文本，交易详情链接仍存在。
- [ ] **Step 2: Run** 上述测试 — Expected: FAIL。
- [ ] **Step 3: Implement** 文案替换与删除；`Reserve`、`Master` 保留。
- [ ] **Step 4: Update** 其余断言旧文案的测试。
- [ ] **Step 5: Run** `npx vitest run` 与 `npm run build` — Expected: 全部 PASS。
- [ ] **Step 6: Commit** `copy: hide internal identifiers and plain-language credit names`。

### Task 6: 余额整数显示

**Files:**
- Modify: `web/src/games-api.ts`（新增函数）
- Modify: `Games.tsx`（`game-balance`、`RoundReceipt` 的"结算后余额"）、`games/SlotGame.tsx`（`slot-balance`）、`games/BlackjackGame.tsx`（`blackjack-balance`）、钱包可用筹码显示处（`Wallet.tsx`/`WalletChamber.tsx`，以实际渲染 `6270.37079` 的位置为准）
- Test: `web/src/games-api.test.ts`（若无则创建）、`Games.test.tsx`

**Interfaces:**
- Produces:
  - `export function wholeChips(value: string): string` —— 输入为 units 字符串（同 `chips()`），向下取整到整筹码，`zh-CN` 千分位，如 `'3135185395'`（6,270.37079 筹码）→ `'6,270'`
  - `export function wholeDecimal(value: string): string` —— 输入十进制字符串，截断小数后千分位，如 `'6270.37079'` → `'6,270'`，`'0.99999'` → `'0'`，`'-3.5'` → `'-3'`

- [ ] **Step 1: Write the failing tests**：`wholeChips`/`wholeDecimal` 上述样例；`Games.test.tsx` → `floors balance display but keeps exact validation`：可用余额为 10.5 筹码时余额显示 `10`、`title` 为 `chips()` 完整值，下注 10 仍可开局；余额 9.99 时显示 `9` 且提示"可用筹码不足"。
- [ ] **Step 2: Run** — Expected: FAIL。
- [ ] **Step 3: Implement** 两函数（BigInt 运算，不用浮点）并替换上述余额显示为 `<strong title={完整值}>{整数}</strong>`。
- [ ] **Step 4: Run** `npx vitest run` — Expected: PASS。
- [ ] **Step 5: Commit** `feat: show whole-chip balances with exact value on hover`。

### Task 7: 文字阶梯与对比度

**Files:**
- Modify: 除 `ops/` 外 CSS 中 `font-size: 9px|10px|11px` 的玩家可见文字 → 说明文字 12px；正文 <13px 的 → 13px
- Modify: 深色页面次要文字颜色统一为 `var(--text-soft)`（替换 `#8fa3b8`、`#7f93a8`、`var(--muted)` 在深色容器内的用法，以实际出现为准）
- Test: 扩展 `button-style.test.ts` → `no player text below 12px`：排除 `ops/**`，断言无 `font-size:\s*(9|10|11)px`；允许清单 `.brand-wordmark small`（字标装饰）。

- [ ] **Step 1: Write the failing test**；**Step 2: Run** — FAIL。
- [ ] **Step 3: Implement**。
- [ ] **Step 4: Run** `npx vitest run`、`npm run build` — PASS。
- [ ] **Step 5: Commit** `style: readable type scale and contrast on night pages`。

### Task 8: 批 2 发布

- [ ] 同 Task 3 流程，镜像标签 `momiao:visual-a2-20261012`，四件套目录 `...\visual-polish-20261012\batch2\`。
- [ ] **STOP — 征得用户部署同意。**
- [ ] 部署后逐页实测：指挥台、个人中心、钱包、奖励、游戏大厅、五个游戏、排行榜、公告、模型目录——按钮仅三种样式、无"原生"、无 UUID/短 ID、余额为整数、每页仅一个英文小标题；截图交付用户。异常即回滚。
