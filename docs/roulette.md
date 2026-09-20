# 原生多人轮盘 v1

## 入口与共用能力

- 恶魔轮盘：`/roulette/devil-roulette`，2 人；加压轮盘：`/roulette/pressure-roulette`，3–6 人，默认 4 人。
- 房间：`/roulette/rooms/<UUIDv7>`；获授权历史：`/history/roulette/<UUIDv7>`。
- 复用原认证、游戏 Origin/CSRF、AVAILABLE_CHIPS 钱包、正式交易与账本、配置注册表、冻结公平流、History 与 Ops。普通单人游戏配置加载器没有扩展成多人引擎。
- 创建/加入免费；READY 确认配置 hash、下注策略 hash、服务器种子承诺和固定金额后托管。每人相同投入；零抽水；加压、道具、再次射击不追加扣款。
- 等待 300 秒；全局 1800 秒。认输的原投入留在池中；正常胜者获得全池。和局/时间上限按合资格座位升序分配整除余数。状态损坏或钱包溢出保留待核对，不换赢家。
- 仅在本轮盘域约束每账户一个未完结座位；淘汰/认输者直到全局结算才释放座位。重新开局是新 UUID、新种子、新 READY，不隐式沿用筹码。

## 固定规则与原版差异

来源：[The-Parliament-Bot](https://github.com/ESSEX-CV9/The-Parliament-Bot/tree/6bfbda58b19e045062f4f5939bb9223d810eddb3)，固定提交 `6bfbda58b19e045062f4f5939bb9223d810eddb3`，`devilRouletteEngine.js` 与 `pressureRouletteGame.js`。本项目沿用仓库已有 GNU AGPL-3.0 许可证；移植文件保留来源注释，许可证全文见根目录 LICENSE。UI 装饰枪械为本次生成素材，不含上游 Discord 图标、角色图或整屏截图。

### 恶魔：momiao-devil-rules-v1 / momiao-devil-rng-v1

4 HP；5–8 发弹。实弹数从 `round(0.4*n)..round(0.6*n)` 均匀取值，再洗牌。首手随机，后续装填周期先手交替。向自己打空弹保留行动，但换弹交替先手优先；手铐跳过行动；锯子在下一枪消耗，即使是空弹。啤酒最后一发触发换弹。

九道具固定顺序/权重：手铐 2、肾上腺素 2、锯子 2、放大镜 3、逆转器 4、手机 4、啤酒 4、香烟 5、过期药 5。每轮补发同数 2–3 件，容量 4，个别/分组上限由冻结规则控制。只从当前可用池抽取，避免无限重试。放大镜/手机情报只给本人；逆转更新已有私密情报，并遮蔽公众弹数直到弹被移走。过期药 40% 回 2 HP，否则减 1，立即判断死亡。

原版适配：双真人，无庄家 AI；肾上腺素必须明确选择对手仍持有的非肾上腺素道具，不用智能替代；偷取不重新套用直接使用门槛，末发手机会消耗但没有未来情报；认输放弃全部固定投入，不使用低 HP 禁用/惩罚减免。行动 62 秒超时由服务器向对手射击。

### 加压：momiao-pressure-rules-v1 / momiao-pressure-rng-v1

6 格弹巢；每波待发池 9 发、随机 1–3 哑弹；实弹淘汰，哑弹消耗但存活，空膛不消耗弹。开局随机打乱座位顺序、备池、装 1 发、旋转。

- FIRE 开枪，存活且无欠枪进入 CHOICE；继续开枪使蓄力 +1，传枪清零。
- 加压实际装填 `min(1+charge,6-loaded,poolRemaining)`，然后重转传枪。charge < 2 时下家 1 枪，否则欠 2 枪。欠枪未清不允许选择传枪/加压/反手。
- 每人一次退弹：随机扔掉枪里一发、不返池，重转。欠 2 枪时取消后续一枪但当前照开；欠 1 枪时直接取消这枪；普通退弹照开。存活后强制传枪，空枪先处理补弹/投票。
- 反手仅存活 CHOICE 持权者可用。枪交给加压者强制一枪，该阶段不给认输/退弹；随后按原座序跳过发起者，不让发起者补枪。若绕回加压者自己，这枪已算本回合，直接 CHOICE。
- 枪空池非空自动补 1 发到未验格，保留已验公开历史；没有未验格才重转。枪空池空进入 35 秒和局投票：任一反对立即备新波，全同意平分；到期未投票按同意。每人仅一票，已投票保存在加密快照，恢复不清零，后续同意票不延长截止时间。
- 玩家仅在自己 FIRE 且非反手强制枪时认输；1 名存活者立即结束，0 人保留 NEEDS_REVIEW。
- 超时档位按人累计 45/25/10 秒；FIRE 自动开枪，CHOICE 自动传枪，实际欠枪优先。人工行动不清掉已累计挂机档位。

原版适配：无 Discord 禁言、改名、角色处罚、赎罪状态、机器人或平台榜单副作用；“加压”只改变枪况，不改变筹码或钱包。以固定提交的实际函数/常量为准，不采用其过期中文注释的 12 发/60 秒等旧值。

## 公平协议：字节与调用顺序

所有整数编码为无符号大端；LP16(s) 为 `U16(UTF8字节长度)||UTF8(s)`。开房用 CSPRNG 生成 32 原始字节 S，公开 `SHA256(S)`，结束前不返回 S。

按座位排序最后有效的 READY 贡献，每座位贡献版本递增，种子为 UTF-8 1–128 字节、无控制字符：

```text
C = SHA256(
  "MOMIAO-ROULETTE-CLIENT-V1\0" || roundUUID16 || LP16(gameSlug) || configHash32 || U16(count)
  || for each seat: U16(seat) || U64(contributionVersion) || LP16(clientSeed)
)
effectiveClientSeed = lowercaseHex(C)
```

复用 `chaldea-pf-hmac-sha256-v1`，Nonce 固定 0，每个域从块号 0 开始：

```text
HMAC_SHA256(S,
 "CHALDEA-PF-HMAC-SHA256-V1\0" || LP16(gameSlug) || roundUUID16 || U64(0)
 || LP16(effectiveClientSeed) || LP16(algorithmVersion) || LP16(domain) || U64(blockIndex))
```

初始域 `roulette/init/v1`；已接受动作域 `roulette/action/v1/<十进制sequence>`。拒绝动作不增加 sequence、不消耗域。UniformInt 每次读 U32BE，拒绝不完整尾部，再 `%n`；Fisher–Yates 从尾到头，分别调用 UniformInt(i+1)。不是浮点 Math.random 的逐种子兼容实现。

恶魔初始 RNG 调用：弹数、实弹数、弹序洗牌、同数道具数量、逐座位逐件按固定权重池取道具、随机首手。重装沿用同样装填/道具顺序，不再随机先手。药品取 UniformInt(5)<2；手机取合法未知未来弹位。

加压初始：座次洗牌→哑弹数 UniformInt(3)+1→9 发池洗牌→队首抽一发→6 格位置洗牌。加压队首抽 actual_load 后全巢位置洗牌。退弹先 UniformInt(loaded)<gunDuds 判定扔掉哑弹，再全巢洗牌、按该动作完成自动击发/补弹。自动补弹从队首取一发，再洗牌未验格；全已验时改为全巢洗牌。投票反对按备池→抽一发→全巢洗牌。抽队首、正常击发、传枪、反手本身不额外随机。

动作摘要是冻结结构体按 Go encoding/json 编码的 SHA-256（字段顺序、omitempty、空集合形态也属于 v1），不对进行中玩家公开。读取证明以 100 动作一批重建状态，核对序号、域、actor、合法动作、时间/超时、每步摘要/公开事件、终局与资金。证明不把长局全帧放进单响应；详情默认 50、最多 100，游标绑定主体/局/version/sequence。

种子与快照复用已配公平密钥但使用独立 AES-GCM AAD：

```text
seed:  "MOMIAO-ROULETTE-SEED-AAD-V1\0"  || UUID16 || configHash32
state: "MOMIAO-ROULETTE-STATE-AAD-V1\0" || UUID16 || configHash32 || U64(snapshotVersion)
```

每次加密新 nonce；读取和保存都校验隐藏状态的结构、数组索引、座位、弹数和强制枪引用；读取到损坏状态时行动/worker 保留原密文并标记 NEEDS_REVIEW。缺密钥不造替代种子。保留历史密钥直到其所有历史验证/恢复需求结束。v1 冻结，后续规则变化须增加规则/算法版本分支，不能修改 v1 来解释旧局。

## 资金、恢复和管理员

房间行锁→按账户升序钱包锁；动作、加密快照、原请求回执同事务。终局先提交固定 outcome 和 SETTLING，再在独立事务尝试资金结算；失败由 worker 重试，不撤回已经确认的终局。钱包与 funding、FINISHED 同事务。`(actor,keyHash)` 幂等，语义包含 round/expected_version/typed input；相同 key 不同语义 409。晚到请求优先提交当前到期事件，再返回版本冲突；409 不总表示“数据库完全没动”。服务端 DB 时间为准，后台批量最多 50 局 SKIP LOCKED，每局每次只推进一个到期动作。

账本类型为 ROULETTE_ESCROW/REFUND/PAYOUT/VOID_REFUND，biz_id 为 `roulette/<round>/<user>/<ready_cycle>/<kind>`。资金守恒：累计托管 = 累计退回 + 累计派彩 + 当前托管。聚合为 numeric/十进制字符串，单钱包保持 int64；前端金额用 BigInt。

首次真实 ESCROW 捕获不可变标题/昵称快照。仅加入但未出资者不产生私有历史。History 仅授权实际出资者；Records 必须重新核验主体、权限、authz_epoch 并写访问审计。未结束证明返回 UNREVEALED；旁观者/其他账户完整证明 404。

Ops 游戏概览/详情沿用现有页面。发布/元数据/维护仍是 GAME_*。多人规则不提供任意概率编辑器；固定配置仅接受已登记且可信工件匹配的 VALIDATED→PREVIEWED→ACTIVE；SUPERSEDED 不重新激活。`ROULETTE_CONFIG_PREVIEW` 用 games.config.validate，`ROULETTE_CONFIG_ACTIVATE` 用 games.config.activate 且 SUPER_ADMIN/原因/typed confirmation/fresh auth。

`ROULETTE_SYSTEM_VOID` 用 games.runtime.write，同样 SUPER_ADMIN/原因/typed/fresh。目标为 roulette_round，输入 `{}`，没有管理员自由填金额/赢家。锁定未终结且未支付赢家的局，逐笔验证正式资金事实，只退各 READY 周期原净托管。原 outcome 不改写；退款写入独立 void_outcome。旧加密快照、种子和 action 保留，缺密钥也不修复/重抽。资金不一致或余额溢出则不执行退款。完整审计/回执与退款原子提交。

迁移 0037 建域，0038 追加既有 History/资金回链/Ops，0039 仅在目录精确前态匹配时开放加压。原迁移 0001–0036 不修改。运行时 grants 独立审阅应用，History reader/worker 保持原角色隔离。这里没有自动上传、迁移生产或部署步骤。

## 回退与后续维护

源码交付附基线和修改包 SHA-256、受影响路径逐文件 hash。ROLLBACK.sh 只接受本次独立验证树，检查标记和全部受影响文件当前 hash 后恢复基线源码；额外修改会停止。它不连接数据库、不删除账本、不执行 down migration。

一旦真实环境接受了轮盘资金，不应切回不认识轮盘的旧二进制：关闭新开房与 READY，保留旧规则、worker、退款、History/Records，等已有局完结。迁移和密钥须继续保留。源码验证树回退不等于生产数据回退。

可复用的维护提示：

```text
先读当前 Momiao 交接并执行 Git 只读核对，保留未提交改动。
仅处理本次明确的新需求；轮盘 v1 历史解释器、旧迁移和已上线游戏行为冻结。
优先复用认证/钱包/公平随机/History/Ops；默认最多新增1个回归，复用旧框架。
记录金额守恒、幂等、私密投影与真实页面证据；源码回退与有资金数据的前向维护分开。
不自动部署、上传、清理或修旧CI；子代理仅 gpt-5.6-sol/max。
```
