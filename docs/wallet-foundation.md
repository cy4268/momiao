# 账户与账本基础 A1

这是内部开发库，不是面向用户开放的资产服务。已运行的门户仍复用原生 API，本库没有接入 HTTP 路由、环境配置或自动启动迁移。

## 可调用的边界

包：`github.com/cy4268/momiao/internal/platform`。

| 接口 | 职责 |
|---|---|
| `ParseAmount(text)` / `FormatAmount(units)` | 精确正金额解析；零与有符号账变格式化 |
| `Open(ctx, databaseURL)` / `Close()` | 显式数据库连接；不自动建表或创建用户 |
| `Store.Migrate(ctx)` | 管理操作：事务、校验和及迁移锁；不供运行请求调用 |
| `Store.EnsureAccount(ctx, userID)` | 同事务确保身份 FK 锚点与两个零余额钱包 |
| `Store.ReadWallet(ctx, userID, asset)` | 读取一个明确的钱包；缺行返回错误而不是零 |
| `Store.Ledger(ctx, userID, asset, afterSeq, limit)` | 按用户、资产及序号读取流水，单页 1～100 条 |
| `Store.Apply(ctx, Mutation)` | 内部单钱包幂等账变；不接受未经认证的外部用户 ID |

`Mutation` 需要稳定的 `BizType/BizID`、`UserID`、`Asset`、`DeltaUnits`、`EntryType` 与 `IdempotencyKey`。身份核验和业务规则由将来的服务层完成，直接把浏览器请求映射到该结构并调用属于错误接入。

## 金额与一致性

- 两种本地资产：`RESERVE_API_CREDIT`、`AVAILABLE_CHIPS`。不保存第二份原生可写 quota。
- 一单位 Credit / Chip = 500000 atomic units。`0.0372` → `18600`，`0.000002` → `1`；`0.000001` 被拒绝，不舍入。
- 正金额输入不接受零、负号、正号、指数、空白和超过六位的小数；接受数字前导零并规范化输出。有符号流水和零可格式化。
- 持久化为 BIGINT；金额、序号和用户 ID 的 JSON 编码使用字符串，避免 JavaScript 整数精度丢失。
- 幂等键为 16～128 字节可见 ASCII、无空白；只持久化哈希。语义哈希使用版本化固定字段，不取任意原始 JSON。
- 同用户、同键、同语义返回原始流水。换键但业务身份相同且语义相同仍复用原始业务效果；键或业务身份复用于不同语义返回冲突。
- 余额、序号、交易、幂等记录与流水同事务；负余额及 int64 溢出拒绝。提交结果未知不自动重试，调用者必须使用原业务身份和键查询/重放核对，禁止换键当作新操作。
- 单钱包限制是真实能力边界：本批每个交易只有一条 leg。兑换、多资产事务和跨库 Saga 需要后续新增迁移与接口，不通过连续调用两次 Apply 假装原子兑换。

## 迁移

嵌入文件位于 `internal/platform/migrations/`。与原设计中的全量迁移编号不同，本仓库使用独立、从 1 开始的最小序列；不把这套编号当作原 IS 全量实现。

显式 `Migrate` 校验已应用版本与内容哈希；未知未来版本、缺失历史或内容篡改报错。已经应用的文件不改写，后续只加新版本。业务表位于 `identity`、`economy`，迁移和幂等元数据位于 `platform_meta`，不使用 public 业务表。

钱包流水、已确认交易和幂等记录禁止普通 UPDATE / DELETE / TRUNCATE。数据库所有者或超级用户仍有 DDL 能力；这些触发器不替代正式部署的最小权限角色。

## 真实数据库测试

仅指向专用、可丢弃、名字以 `momiao_test_` 开头的 PostgreSQL 16 测试库。测试会创建 schema、插入合成数据并暂时注入失败触发器/修改测试迁移记录。不要指向任何线上数据库，即使其名称恰好匹配前缀。

PowerShell 示例（测试库需要预先创建，凭据按本机测试配置设置）：

```powershell
$env:MOMIAO_TEST_DATABASE_URL = 'host=127.0.0.1 user=postgres dbname=momiao_test_local sslmode=disable'
go test -count=1 -timeout=90s -v ./internal/platform
# 在支持 race detector 的开发环境执行：
go test -race -count=1 -timeout=90s -v ./internal/platform
```

缺少环境变量时集成测试明确 SKIP；配置错误时失败，绝不退回内存模拟。CI 的 `wallet-postgres` job 使用独立短期服务和测试凭据，显式配置测试数据库，不访问部署环境。

连接使用 [pgx 官方 Go 驱动](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool)；CI 服务接入遵循 [GitHub PostgreSQL 服务容器说明](https://docs.github.com/en/actions/tutorials/use-containerized-services/create-postgresql-service-containers)。本批测试覆盖功能正确性，不证明容量、恢复或生产最小权限。

## A2 接入前仍需完成

服务端原生身份核验、Master 资料业务、正式平台库及受限运行/迁移角色、进程与数据库恢复验证、只读钱包与流水接口和页面。奖励、充值、quota 划转与游戏保持未开放。
