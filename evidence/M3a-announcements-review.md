# M3a 公告候选交付

日期：2026-09-06。状态：本地实现和范围内验证完成，交主控复核；不是 M3/Beta 全量验收或生产发布。

## 提交与范围

- 基线：`d1c0145c182a2e7989962b5be0147333ed8f79e5`。
- 分支：`codex/m3-announcements-20260906`，独立 no-hardlinks clone。
- 冻结 schema 检查点：`e988524ef0894f84c0c951682c7d9f35c9b4b660`，**只含** `internal/platform/migrations/0005_announcements.sql`。
- 应用实现：`128fe7122bf6ad6996e772d94ae8d02ede466d6c`，在该 schema 检查点上追加 28 个文件。
- 本报告作为单独文档提交追加，候选最终 HEAD 由交付回执给出；上述实现 SHA 已固定。
- 全部应用差异相对基线为 29 文件，3291 行新增、21 行删除；另加本报告。提交作者/提交者均为公开 GitHub noreply 身份。

新增内容集中于公告 Go 文件、公告 React/CSS/API 文件、单份迁移、适配合同及测试。现有 config/server/main 只接公告 store、HTTP 路由与 worker；App/Home/API 只接公开页面、导航与首页 banner。既有 profile 测试只把 migration manifest 数量从 4 调整到 5。未修改钱包、额度、游戏实现、共享设计原文或其他候选目录；未集成、push、部署或操作正式账户。

## 已交付行为

- 六类公告、草稿/预约/已发布/过期/归档与独立撤回时间；身份、不可变内容版本、通知 revision、并发 row version 分开。
- 真实 Ops 草拟、编辑、受控 Markdown 净化预览、发布/预约、正文更新、独立展示渠道调整、重新通知、撤回和归档。Level2 使用服务器存储的影响预览、精确命令 hash、10 分钟有效期与显式确认。
- 每次 Ops 请求先通过固定 native transport 核验浏览器会话身份，再查 ACTIVE 平台 principal、base_role、ANNOUNCEMENTS scope 与 authz_epoch。原生 role 100 不自动成为平台运营者。
- 写事务串行核验权限、目标版本、展示区间和稳定 operation ID；业务变化、持久 job、append-only 成功审计同事务。精确重试返回原结果；不同命令重用编号明确冲突。
- 公告列表/稳定详情、类别/搜索/日期筛选、分页、人工置顶顺序、历史归档。所有公开查询使用数据库时间、可见窗口、身份和撤回条件，读时有效性不依赖 worker 是否已跑。
- ENTRY_POPUP 与 PRIMARY_HOME_BANNER 固定 guard 行锁，拒绝预约区间重叠，允许相邻半开区间。进程内 worker 从持久 job 恢复发布/过期，错过窗口不补发、不写阅读记录。
- 公共接口只输出服务器 canonical sanitized HTML 和安全字段；详情 GET 只读。登录详情渲染后显式 POST 当前响应的 notification revision；旧阅读不会标记新通知已读。
- 匿名入口关闭与会话展示去重按 ID/revision 保存；本地存储损坏/禁用有容错。首页 banner 独立位于原 M1 主视觉之后。
- canonical ACKNOWLEDGEMENTS 唯一；结构化文字致谢要求明确公开同意。未造贡献者、未开放未经验证的图片上传/外链图片。

适配细节见 `contracts/announcements.md`。设计 `/api/v1` 适配为现有 `/platform/v1/...`，保留 native `/api/*`；页面为 `/announcements`、`/announcements/:id`、`/ops/announcements`。

## 冻结迁移与数据库证据

0005 SHA-256：`e110e85e17b31318789e321dcfd89ff17ac7ca4cc9c9c8fa73f1ce56e93d515e`。主控已单独核验并仅批准作为稳定 schema 依赖；此后不重写 SQL、不 amend 该检查点。必要 schema 修订须先由主控协调编号。

| 文件 | SHA-256；与已发布基线逐字一致 |
| --- | --- |
| 0001_wallet.sql | `6db5d8fac468dbfac4eebe78dfd9af60f4c680b3206836898e61237b377b7cd9` |
| 0002_master_profile.sql | `c6024157351464f79a787bdb15f5a304f5d6878b6a3cb623c31b5ea64f6e26cb` |
| 0003_wallet_actions.sql | `1083def3f0297cf1d98f4130af4b4977e17607d172d35e710d86a06fd648265c` |
| 0004_quota_transfers.sql | `bb5b786cbc5f347eae0986e28dd5423609c304a56d37452cf994f60f75890f83` |

使用本机 PostgreSQL 18.6，未触碰 M2 库、role 或数据：

- `m3_announcements_20260906`：开发中的真实事务/竞争/故障注入库，保留红绿过程合成历史。
- `m3_announcements_acceptance_20260906`：最后从空库创建，以已提交应用重新运行全套 Go 测试，迁移 0001–0005 自动按测试入口顺序执行，227 个测试/子测试通过。
- `m3_announcements_browser_20260906`：独立浏览器库。最终单条合成公告保留 1 个内容版本、1 个通知版本、1 条阅读，row version 4，`withdrawn_at` 非空；4 条审计依次为 SAVE、PUBLISH、UPDATE_PLACEMENTS、WITHDRAW。

## 实际验证

下列日志和 JSON 位于独立 checkout 的上层验收目录，不进入 Git。测试配置及密码只在进程内读取，报告不包含连接凭据。

| 验证命令/入口 | 实际结果 | 原始证据文件 |
| --- | --- | --- |
| `go test ./... -count=1 -json`，公告专用数据库 | 退出 0；227 通过、0 失败、9 明确跳过 | `go-test-final.jsonl`、`go-final-summary.json` |
| 同一命令，另建全新公告验收库 | 退出 0；227 通过、0 失败、9 明确跳过 | `go-test-clean-db.jsonl`、`go-clean-db-summary.json` |
| `go vet ./...` | 退出 0 | `go-vet-final.log` |
| `go mod verify` | 退出 0，all modules verified | 实际命令输出 |
| `npm --prefix web test` | 退出 0；19 文件、151 测试通过 | `web-test-final.log` |
| 最后表格布局/焦点改动后的 Ops 相关测试 | 退出 0；2 测试通过 | `web-ops-final.log` |
| `npm --prefix web run build`（内含 `tsc --noEmit`） | 退出 0；50 模块，JS 376.88 kB、CSS 50.40 kB | `web-build-final.log` |
| 本地实际 HTTP handler 验收 | 11 项通过 | `http-acceptance.json` |
| 撤回后只读 HTTP 检查 | 活跃/归档详情均 404；列表空；entry/banner 均 null | `withdrawal-http.json` |
| 数据库迁移、版本、阅读、审计和 job 查询 | 实际 PostgreSQL 数据 | `database-evidence.json`、`migration-hashes.json`、`baseline-migration-parity.json` |

9 个 Go skip 为手动浏览器 fixture、3 项 Windows/本地环境不适用的原有用例、5 项本批未启用连接的原有钱包/资料/划转数据库用例。**公告数据库测试没有跳过**。本批未重复 M1 生产全链验收。

有意义的红绿与回归包括：Markdown/raw HTML/图片/危险 URL；内容编辑与重新通知的阅读隔离；匿名/不同账户阅读隔离；同一版本并发写入；相同 operation 并发幂等；审计失败导致业务回滚；两位不同运营主体竞争同一未来展示窗口及相邻区间；断连前未提交的状态回滚，重新打开 Store 后恢复错过窗口/正常发布/过期；过期读取不等 worker；预览 body hash、确认、有效期；canonical 致谢唯一及公开默认渠道校验；列表置顶顺序/分页/身份/日期/类型筛选。

主控反馈与本地复查关闭的问题：

1. 权限撤销等待者猜中新 epoch 时，旧 scope 子查询快照可能被混用：实际观察 PostgreSQL 锁等待后提交撤权的测试先红，改为锁 principal 后在新 statement 读取 scope，Prepare/Execute 均绿。
2. 目标锁等待后的内容指针可变，而旧 join snapshot 看不到新 revision：先红，改为先锁身份行、再用新 statement 读取不可变内容；同一策略用于 worker。
3. 原生 UserAuth 同时允许 PAT：公告入口增加与固定 native 格式一致的浏览器凭据类别检查，完整 token 仍交 native 核验签名、到期和实时会话；opaque/dotted PAT、非标准 header/claim、伪造签名/撤销会话负例通过。
4. 写入成功但随后 GET 失败：UI 保留已确认回执，只提供 GET 恢复，测试证明不会再创建一次。
5. entry 关闭后离开首页再返回不复活；手机公告导航可见；Ops 表格整页溢出改为表格内部滚动并提供键盘焦点。

## 浏览器证据

使用真实本地门户/Go 公告 handler/PostgreSQL，固定 loopback `127.0.0.1:14211`；浏览器身份为独立合成 fixture，**不代表真实 native 服务的端到端认证验收**。原生 role 1 + 显式平台 SUPER_ADMIN 可操作；原生 role 100 + 无平台 principal 在页面显示无权限，HTTP 为 403。

实际完成：草拟、服务器净化预览、发布影响确认、登录详情阅读、正常退出后的匿名详情地址刷新、入口弹窗关闭、首页 banner 独立保留、已发布公告独立取消置顶、撤回影响确认及公开不可访问。

CSS viewport 为 1440×1000 和 390×844。已查看保存文件的 9 张有效 JPEG 截图列于 `browser-acceptance.json` 和 `screenshot-manifest.json`，覆盖桌面列表/详情、手机列表/匿名刷新/首页 banner/无权限/Ops 表格/撤回预览/撤回详情。部分导出为 375×812（浏览器滚动条及导出尺寸），没有把导出尺寸当成 CSS viewport。手机 Ops 实测 `document.scrollWidth=clientWidth=375`；表格本身宽 351、内容宽 730，可独立横向滚动。

入口弹窗实时图像及关闭交互已观察；保存的 `03-entry-390.jpg` 是 viewport 切换时的异常缩小拼图，已排除并保留失败样本。其余旧导航/溢出截图明确标为 superseded，不充当最终通过证据。没有使用 fullPage 导出。未宣称完成 axe、浏览器缩放或全站 M1 重验。验收后浏览器尺寸已恢复，本地 fixture listener 已停止，合成数据库与证据保留供复核。

## 依赖与真实接线边界

保持冻结直接依赖 [goldmark v1.8.5](https://github.com/yuin/goldmark/releases/tag/v1.8.5) 和 [bluemonday v1.0.26](https://github.com/microcosm-cc/bluemonday/releases/tag/v1.0.26)。新增前核对官方 release 与 Go 漏洞数据库：goldmark 已知记录 [GO-2026-5320](https://vuln.go.dev/ID/GO-2026-5320) 修复于 1.7.17；bluemonday 的 [GO-2022-0588](https://vuln.go.dev/ID/GO-2022-0588) / [GO-2022-0762](https://vuln.go.dev/ID/GO-2022-0762) 分别修复于 1.0.16 / 1.0.5，冻结版本不在这些已知受影响范围。

bluemonday 拉入的旧 x/net 0.17.0 存在官方已知问题，因此必要间接依赖采用 x/net 0.56.0（包含官方 html 修复及 [GO-2026-5942](https://vuln.go.dev/ID/GO-2026-5942) 修复）；其 go.mod 要求 x/text 0.38.0，后者要求 x/sync 0.21.0。其余新增间接依赖为 douceur 0.2.0、gorilla/css 1.0.0；未新增 Web 依赖。原始核对结果保存在 `dependency-security.json`，这是已知公告核对，不是完整供应链安全审计。

native 凭据语法依据本地只读核验的 `054df389178025a34bc9f65964056ad8d6cc7d22` 中 `service/auth_token.go` 与 `middleware/auth.go`：两文件 SHA-256 分别为 `c38c71bc750d05bba7080026391a0d415b75c4f876facc808db3268e8fe3aeb7`、`83c654aa64a1693683f8ebc84c2ac633105f91bfcdbec5b32311d3f5520427d9`。这两份文件未由本批修改。

明确保留的缺口：

- 登录后自动弹窗尚未接全局 Master 完成、迁移提示完成、回跳已解决及关键业务状态的组合契约。已提供正确的已登录未读候选排序接口；不会伪造“没有活跃对局/未知钱包操作”的判断。
- managed asset 验证/同意流程尚未交付，因此本批只支持文字致谢并拒绝 media ID/图片上传。
- 生产初始 SUPER_ADMIN bootstrap、生产 runtime 最小 grants 和真实 native 会话端到端验收仍需独立运维接线。测试 seed 仅编入 `_test.go`，限定本机 `m3_announcements_*`，没有“首访者管理员”或通用 bootstrap CLI。
- 最小 Ops 列表展示最近更新 100 条；完整角色管理、公开模型、其他 M3 功能不在此候选范围。

## 集成和回滚

当前只交本地候选，主控复核后决定集成。0005 已交给后续迁移依赖，后续提交不得重写它；应用回滚可撤回本批应用接线并恢复基线行为，但保留 0005 表、不可变内容、阅读、审计及 job 历史，不执行 DROP 或 down migration。正式 runtime grants 与初始 principal 接线必须按合同另行验证，不能因使用 owner 权限的本地测试通过就宣称生产已就绪。

跨任务交付发送曾被自动审批拒绝；随后主控通过正常复审确认已取得 schema 检查点，并明确要求在本任务完成本地交付。本批没有重试被拒发送或改变通信机制。
