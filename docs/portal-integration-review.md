# 门户组合候选验收

2026-09-06。本批组合审定候选、解决实际冲突及接通合同。M3c、M4b/M4c/M4d 已导入，模型导航接入同一 AccessGate 与返回意图。最终 Web 28 文件 / 199 测试及构建通过；原 M4 执行者已完成实际低权限联合验收，最终 36 项生产输入逐一相符。模型登录返回链路、Kimi 四个画面与独立新站声明均获主控实际复审通过。本提交是待发布候选，尚未执行生产部署。

发布分支以 `d1c0145c182a2e7989962b5be0147333ed8f79e5` 为唯一父提交，将下面的内部组合记录压成一个提交。原候选及内部组合历史均保留，不改写其作者；新发布提交的 author 和 committer 使用 `cy4268 <325177413+cy4268@users.noreply.github.com>`。

## 精确输入

| 内容 | 审定来源 SHA | 内部组合 SHA |
|---|---|---|
| M2a 原生准入 | `733be2ec7b94b21924f1014484d700a922ac8fb7` | `4a0d4ff24c15ed281e7f5443d0ade88b68511b2e` |
| 原生模型来源 | `26604b681af246f0d98223f543e52d5b36c0f70c` | `d3a56064d9c7fb2975f577c397abeea282b18ce4` |
| 原生补丁联合组合 | `9520310d6d75feb0c30c70aaafe835e83b8ccff2` | 同左 |
| M3a / 0005 | `e988524ef0894f84c0c951682c7d9f35c9b4b660` | `1cee719bd9bdeadea0cdc4c0770c973e57980e5f` |
| M3a 公告应用 | `128fe7122bf6ad6996e772d94ae8d02ede466d6c` | `245999e278bdc3cc5d42ba820f959e086c99478d` |
| M3a 验收记录 | `744000ee37d5ee2583345a1f05c194a3811e50b4` | `cc873622bdfcf01096e79bf989b0ea13148760ea` |
| M2b / 0006 | `eea192e7bc7417a8325d4e59d4281106164d423f` | `77863511864a45c3b15abf8b3ccd776f9f586897` |
| M2b 平台准入 | `fa86bbd6f70fb703457b168724fa0688c0803292` | `5a6806ebbada3a54047428a3f3d01281332994b9` |
| M3c / 0007，仅迁移 | `60add8b4b876ba030871051425ea830f8ea3cba5` | `894d5d6e82d65c48e5ba332ba461e87c9e5a3cbd` |
| M4a / 0008 | `9d8013ab081ee285832d0af7c55b511d4fa20bd5` | `719e99ed47335d3723da36ec77c566184e032ac4` |
| M4a Bootstrap 与共享 native-self | `4b4da047fade53ed58cda3738c1ac7fef1c40cec` | `61a8304b76b62f967601bf3dac3ab63711dbeb5e` |
| 已审原生 Linux 发布输入 | `afff90266aa5e1aaffa1eed26dba8ce4f3996e18` | `795a69042c2ed3602ac9c77517c35bc360968aa3` |
| 固定 Discord L4 出站部署候选 | `6fce90032400ecdb8c88612ddad96376e24a364a` | `8a55b453877472a1e82a13cc637e7abd26fa1b6a` |
| 已审 Saber / Mash 透明角色层 | `a72fa876198ba11d9a04c22abc47a9b254c56aec` | `9e87c1c084180db7b3adf9cca70208f0c672e46b` |
| 原生部署与代码回退入口 | `8ca393f7704e05d5136d790b5a5921ed083aca50` | `b831cb3e014209f67b682d2989d72b936b78caab` |
| 部署入口固定站点 origin / callback | `1f944dfda0c30837f1168534ed99f497a3fbb735` | `37fd1de4c0e9e9c7c9ee58da298154748d7dffce` |
| Kimi 既有项目素材复用 | `07fc2d1976a03100a15724e56d64dd405cefc661` | `a85b1e8a8f96b6d7be78355743751f2d805212f5` |
| M4b / 0009 | `7fade5fe2a7d7a575bf455849fc590532e5c515a` | `69e0af1b041d0ad7dcecdf5d09aa0018074c73d9` |
| M4b 访问 Gate 与迁移公告 | `e3c2f716bf8120442128364cf52d98f2eb17fdd9` | `0c03c7d62fceb6063cf369870d6316cc61e9e559` |
| M4c 最小 runtime / Bootstrap grants 模板 | `87bce69311d46c6d807558ab70fa3e0167011075` | `cb480bb9ee75f118d0eb807972edad4bb377dd53` |
| M3c 来源读取与精确计价 | `70c9de29dc6694081d4ad0f93b632114ba7601a9` | `7fb52dc89f72153888ddedef850035ee292a1812` |
| M3c 存储与运营发布 | `56682ce5902a5e678b85582d4120c83ec0a09f8e` | `d685ea6f3a51c9d59c59fc840acdb04b9dcc91a4` |
| M3c 页面与浏览器夹具 | `2b4bc5120ad9400c28d7afe8561d3a2084b14730` | `b3d3e5c0b707d5f4b1cd17770fed52e4cabc01ed` |
| M3c 来源与文档收尾 | `a4c8d5b05bbe636ebdb36ab125351f83e6ea98a9` | `60f3c55c4173db8fe404d4ce6bb4f1fa5ec2315a` |
| M3c 已审遗漏依赖（仅三文件与配置调用） | `92a85c90cf0fc67c1a14973931345db326407f02` | `41a77ccd5e64f1a34f8d4d5736810d426a5682d2` |
| M4d 真实旧基线缺少的三个 profile 列授权 | `374951fcb6c325ecbd358a8e33e8e722ef88c53c` | `b570083a294f0c8eb2dd3f08901676e94343251a` |
| M4d 实际授权联合验收与旧基线 | `632ca349ce1ff578ae9b10d2e45c04f4f169b23d` | `17bbbd71e554c0b671acfeb793c4602087495707` |

M3c 仅导入上表精确输入；92a85c9 仅取 `catalog_reader.go`、对应测试、`catalog_source_test.go` 与 `loadConfig` 的 `catalogConfig` 调用，保留 70/a4 较新解析器、合同与全部 M2/M4 配置。原生补丁目录相对 `9520310` 无差异；本批没有重新构建或重复验收原生镜像。`native-release` 的镜像结论来自该独立候选自己的记录，不能替代门户运行时证据。

原 `3e2e7712bced6f6f717fc9ef074d15d9a246b282` 发布候选和内部组合历史均保留。后续出站候选仅增加 `native-release/discord-egress/` 下九个审定文件，逐一校验与精确来源的 Git blob 相同；门户代码、原生 ELF/镜像和迁移均无变化，因此本次沿用已完成的组合验收，不重复全套或镜像测试。新发布候选仍从公共基线创建独立的单提交分支。

主控已核对部署 guard 的分工：HTTP namespace guard 不固定镜像；quota guard 保留所有检查，只在部署包装中更新精确镜像预期；Discord reconciler 消费 HTTP READY 并校验 namespace inode。本次只收录部署输入，没有安装单元、变更 guard 或启动出站服务。该独立来源的零 capability、固定目标及合成 TLS 证据见 `native-release/discord-egress/acceptance.json`，真实 OAuth 仍待部署验收。

后续收录的部署入口保持离线计划为默认，实际执行和代码回退均需显式 `--apply`；本站 origin 与回调固定为 `https://momiao.win` 及 `/oauth/discord`。本批只导入并校验精确来源，没有执行入口、Docker、迁移或生产 apply，也未重复来源测试矩阵。已只读核对来源目录上层的 `artifacts/native-6d7062.tar`：130198528 bytes，SHA256 `c393696004c374f066d8e91a1f2b3dad1e4c6883a32b88835c7736808ba690af`；TAR 留在 Git 外，没有重建或重新导出。

Kimi 原文件和获取时清单保持不变：`WORKSPACE_PROVIDED`，1024×1536 RGBA、2632300 bytes，SHA256 `658da9f399818b7e18eac93095db8a602211628a12c7bd8fa47275af200e23ab`。本批不声称新生图或已有许可证证明。项目所有者明确批准项目内复用；registry 的 `LICENSED_OR_APPROVED` 记录此批准，不推断历史许可证或作者身份。

非破坏性规范化仅增加四边透明留白，1152×1728 PNG 中原 1024×1536 区域的 RGBA 像素逐字节相同，原文件未动、没有放大。最小 Alpha 边距 5.56%。共享 768×1152 WebP 为 233224 bytes，规范化 PNG fallback 为 3673670 bytes；只有一个 Kimi registry 条目，卡片与详情共享同一构图。`../evidence/kimi-normalization-proof.json` 记录尺寸、参数与两文件 SHA256。原获取清单的 `REVIEW_PENDING` 作为历史事实保留；实际四画面、声明和最小 caption 修正获主控复审后，交付 registry 条目进入 `PRODUCTION_READY`。

1440×900 与 390×844 的实际卡片/详情均完整 contain，无横向溢出；详情说明使用独立 76px 底部区域，与图片盒不重叠。四张截图与 `kimi-layout-metrics.json` 在 `../evidence/final-model-browser/`。WebP 缺失时实际加载 PNG fallback；双图失败转几何标识由既有组件回归覆盖（11 项 Catalog 定向测试通过），未把浏览器缓存保留图片的尝试冒称双图实时验证。所有临时构建副本已原位恢复并核对哈希。高分辨率母版和生产环境 CWV 不由本地画面验收代替。

## 冲突与本批增量

- M2b 的六处冲突保留双方配置、公告与准入后台任务及退出等待、内部路径拒绝规则、API 路由、前端入口和回归断言；保留 M2b 的会话失效、跨账号隔离及账户敏感 proof 清理。
- M4a 的迁移数量断言改为至少八项，继续校验既有迁移字节；迁移 runner、连续性与 checksum 规则未放宽。
- M4b 与 M4c 无冲突导入，25 个应用/模板文件逐一核对 Git blob 与精确来源一致；认证角色代码、两张 PNG 与清单未变。0009 新增迁移公告版本、用户要求与幂等确认事实；不执行迁移、额度或账户切换。访问 Gate 保留未确认声明的 `UNVERIFIED` 状态。两份 grants 模板默认 `apply_grants=false`，仅收录文件，尚未执行 SQL。来源核对见 `../evidence/m4b-m4c-import-proof.json`。
- M3c 路由、配置及后台来源同步 worker 与现有准入/公告 worker 并存，退出时先取消并等待 worker 再关闭 store；内部路径拒绝规则仍在业务路由之前。
- 模型接入与密钥选择只允许单个有效 `model_id`，`intent=use` 仅属于 `/api/access`；重复、未知、非法编码及片段均拒绝，裸分号与 Go 查询解析保持一致。沿用 `chaldea.post-auth.route.v2`，不新增会话或返回存储。Gate 未 READY 时不读密钥；READY 后仅查询密钥数量并导航，不自动创建密钥或发起模型调用。`/ops/models` 校验真实平台 MODELS 权限，原生 role=100 不代替平台权限。切换路径、查询或会话时重建路由页面以清除过时状态。
- 最终模型夹具改用独立数据库 `m3_catalog_platform_browser_portal_20260906_01` 与随机回环端口，提供合成原生身份和已完成通知事实，真实门户/存储要求用户完成 Master 与显式通知确认；未更改生产默认的迁移 `UNVERIFIED`。
- 组合首批的新增代码是 opt-in 浏览器夹具组合：使用符合共享 dashboard 凭据语法的合成 JWT，通过精确 token 的内存会话验证；专用本地数据库种入公开/登录可见公告，并接入公告 store。该 `_test.go` 不进入生产二进制。夹具的 Bootstrap 种数不属于部署 CLI 或真实原生身份验证证据。

## 认证角色层接入

按 AD-FRZ-124/125/484/520，在既有 AuthFrame 中将登录 Saber P3、注册 Mash P2–P3 作为独立透明角色层接入 Royal Observatory。保留原始两张 PNG 和生成清单字节；不修改人物、认证逻辑、post-auth 或背景原图。清单的生成时审核状态作为来源记录保留，本节记录主控审图批准和实际应用验收。

桌面最大 1360px、7/5 场景/实体认证面板；1100px 以下单列，720px 以下场景最小 390px。角色使用 `object-fit: contain` 与布局留白，失败时移除装饰图片而保留字段及 CTA。背景的渐隐仅作用于独立背景层。

定向 `Authentication.test.tsx` 四项通过，包含两张角色的对应入口及图片失败后表单可用；TypeScript/Vite 最终构建退出 0。未运行全 Go/全 Web 或再次提交合成注册。本次浏览器只访问实际构建的 `/login`、`/register`；现有 Go 测试宿主提供本地配置，未提交登录/注册。四个视口均实测无横向溢出，图片 natural size 为 1024×1536 且完整包含于场景；另保存移动登录页下方表单截图。浏览器尺寸已恢复，临时宿主已停止。

有效应用截图与 DOM 尺寸证据：`../evidence/auth-character-app/` 中的 `login-1440.jpg`、`register-1440.jpg`、`login-390.jpg`、`register-390.jpg`、`login-390-form.jpg` 及同名 JSON。原始测试/构建输出为 `auth-art-tests-green.log`、`auth-art-build.log`。整页截图接口曾产生缩放留白异常，已丢弃错误捕获，交付的是实际视口截图。

尺寸限制：Saber PNG 1,919,110 bytes，Mash PNG 1,294,414 bytes，均为 1024×1536，上下原始透明留白约 1%；没有放大或冒充 2048 母版。当前保留原 PNG 传输，没有新增编码依赖；高分辨率母版及首屏传输体积优化仍待后续媒体验收。

## 本轮实际检查

原始输出位于本独立工作目录的 `../evidence/`。下表是先于 M3c/M4b 的首批组合历史证据，后续增量结果另列；不将历史结果冒称最终组合全量通过。

| 检查 | 结果 | 原始输出 |
|---|---|---|
| 全 Go，含全新独立 PostgreSQL 数据库 | 335 pass，0 fail，5 skip；退出 0 | `combined-go-tests.jsonl` |
| Go vet | 退出 0 | `combined-go-vet.log` |
| Web | 24 文件 / 177 测试全部通过 | `combined-web-tests.log` |
| TypeScript + Vite 生产构建 | 退出 0 | `combined-web-build.log` |
| 本地组合夹具 | 启动成功；使用实际构建的门户与 PostgreSQL | `combined-browser-harness.log` |
| 匿名公告 | 页面仅出现合成公开公告，不出现登录可见公告 | 本任务浏览器观察及截图 |
| 注册回调 → Master → 登录公告 | 合成注册成功；v0 保存进入指挥台；受限公告可见，详情阅读后已读 | 本任务浏览器观察及截图 |
| 敏感 proof | 交到账户页并开启首个密码表单；刷新后清除，重新要求验证 | 本任务浏览器观察及截图 |
| 赠额恢复 | 从处理中恢复为到账；钱包 1000 Reserve / 500000000 units，一条已确认交易和账本 | `combined-browser-db-proof.json` |
| 登出隔离 | 受限详情不可访问，列表仅公开公告 | 本任务浏览器观察 |
| 浏览器后只读数据库对账 | Master v1、昵称历史/注册来源/领取/发行/交易/账本/已读记录各一条；领取 CONFIRMED 且完整关联 | `combined-browser-db-proof.json` |

五个 skip 为两个 opt-in 浏览器夹具，以及 Windows 缺少符号链接权限/Unix socket 能力的三项条件测试。组合夹具另行启动并实际完成上述浏览器交互；本报告不把原 M2b、M3a 的单模块浏览器证据改称组合证据。未重复原密码/2FA 全矩阵，本批也没有提交设置或修改密码。

本地合成注册回调在用户明确授权后经原 CUA 工具完成。主控随后直接读取发布 commit proof、浏览器 PASS 及数据库对账证据，并批准收录上述出站候选。

最终 M3c/M4b 组合 Web：`final-composed-web-tests.log` 为 28 文件 / 199 测试通过，`final-composed-web-build.log` 与 `final-composed-web-exits.json` 显示 TypeScript/Vite 构建退出 0。随后仅修正裸分号查询与 Go 不一致的边界，`model-query-semicolon-red.log` 记录失败重现、`model-query-semicolon-green.log` 为该五项测试通过。模型 Gate 定向 Go 测试及最终夹具编译通过，见 `final-gate-and-browser-compile.log`。未因此重复 PostgreSQL 或原生镜像测试。

最终模型浏览器：`final-model-browser-harness.log` PASS（443.947s），匿名选模 → 原登录 → Master → 独立通知 ACK → 精确 `keys?model_id=demo%2Faurora`，原生 role 100 在 `/ops/models` 被拒；Master/history/notice ACK 各一条，0 key_creates、0 model_calls。`final-model-browser/flow-receipt.json` 与三张截图已由主控直接核验。Kimi 仅图像夹具 `kimi-model-browser-harness.log` PASS（611.952s），未重复登录/权限矩阵；`kimi-caption-catalog-tests.log` 11 pass，最终 `kimi-browser-build.log` 构建退出 0。两个夹具和临时浏览器均已关闭。

M4d 联合验收由原执行者在真实低权限角色与 77 列旧基线下完成：首轮四项通过、两项失败；仅补三列 profile 授权并修正公告测试前置后，两项定向重跑全部通过，无 skip。主控审阅原日志和完整 diff 后批准；本任务不重跑 PostgreSQL。`../evidence/m4d-final-composition-inputs.json` 记录导入后 36 个生产文件哈希与执行者 manifest 全部相符，九份迁移均未修改。

## 冻结迁移

0001–0009 与各精确来源的 Git blob 及 SHA256 一致。只增加审定迁移，没有修改旧 SQL：

| 迁移 | SHA256 |
|---|---|
| 0001 | `6db5d8fac468dbfac4eebe78dfd9af60f4c680b3206836898e61237b377b7cd9` |
| 0002 | `c6024157351464f79a787bdb15f5a304f5d6878b6a3cb623c31b5ea64f6e26cb` |
| 0003 | `1083def3f0297cf1d98f4130af4b4977e17607d172d35e710d86a06fd648265c` |
| 0004 | `bb5b786cbc5f347eae0986e28dd5423609c304a56d37452cf994f60f75890f83` |
| 0005 | `e110e85e17b31318789e321dcfd89ff17ac7ca4cc9c9c8fa73f1ce56e93d515e` |
| 0006 | `6466e21dfc6332dffd785016f704269d95d687a13677468076f0ef82f8e8a4e9` |
| 0007 | `728b47053eb5ae1745cb076991c0efb60ff9f16d4d530b0a161558d953126eb1` |
| 0008 | `0d726b6cdcce8d740aea21ec37c0d98b3442aa2843d0696c92ffe27cb2566893` |
| 0009 | `3b8563aa4aa915644e5f13205c48fb0c0d6941389df518ec3b883e8ca95881ad` |

## 剩余边界

本地组合收尾完成，剩余是既有 CI、双库备份及真实发布验收。当前迁移为 0001–0009，低权限存储与授权基于已审 M4d manifest 冻结；后续只做字节核对，发现真实 SQL 问题才精确补验。认证角色本地接入已验收，高分辨率母版与其传输优化仍保留为后续媒体事项。

用户已授权联合验收、M4 低权限通过及主控复审后的第三台 `momiao.win` 条件发布，包括平台迁移/最小权限、已核原生镜像、固定 Discord 443 出站与指定原生 ID 1 的首次平台管理员初始化。本轮明确按独立新站开放注册/登录，旧站账号和额度另批迁移；正式声明 `deploy/portal-access-declaration-20260906.json` 使用 `NO_MIGRATION_APPLICABLE`，五个已交付域 AVAILABLE、EXPERIENCE UNAVAILABLE，不创建虚构迁移事实/requirements/ACK。

当前未 apply。发布保留双库备份与回滚，旧站不动，不代领奖励或消费额度。迁移以真实平台库的 `momiao_owner` 执行，runtime 为 `momiao_wallet`；现有名为 `momiao_bootstrap` 的初始 superuser 不得作为 CLI deployer。Bootstrap 使用独立最小 LOGIN、窄函数 EXECUTE、原生 ID 1 的实时 native-self 会话私有输入与生产 TTY 确认，回执核对后撤销授权并删凭据。私有 reader 配对文件沿用主控已核原值，不输出或纳入 Git；真实 OAuth 留待发布后用户核验。数据库回退遵循既有前滚/备份恢复边界，不能删除已应用历史 SQL。
