# 钱包 API A2a

基路径 `/platform/v1/wallet`，返回 `{success:true,data:...}`；失败为 `{success:false,code,message}`。响应均 `Cache-Control: no-store`。

身份头：`Authorization: Bearer ...`、`New-Api-User` 必需；`X-Auth-Session` 可选，浏览器沿用当前登录会话携带。服务端每次通过固定原生 Unix `/api/user/self` 核验令牌、真实 ID 与启用状态，不信任客户端 ID，不转发 Cookie/来源身份头，不跟随重定向。401 可触发刷新登录，403 保留当前会话，502/503 表示服务故障。

## GET /platform/v1/wallet

无参数、无副作用。

```json
{"success":true,"data":{"initialized":true,"user_id":"1","wallets":[{"asset":"RESERVE_API_CREDIT","balance_units":"0","amount":"0","ledger_seq":"0","version":"1"},{"asset":"AVAILABLE_CHIPS","balance_units":"0","amount":"0","ledger_seq":"0","version":"1"}],"scope":"LOCAL_WALLETS_ONLY","total_assets":null}}
```

示例中的零是初始化状态，不是实际账户数据。未初始化返回 `initialized:false,wallets:[]`；只存在一个钱包属于不完整状态，返回503，不把缺行当作零。金额、ID、序号均为字符串。

## POST /platform/v1/wallet/initialize

必须带配置中准确的 HTTPS `Origin` 与 `Content-Type: application/json`，body 为 `{}`，不接受 user_id、资产值或其他字段。结果形状同 GET。只为当前已验证用户确保两行零余额，重复提交不发放资产。

网络失败或 5xx 可能发生在提交之后，客户端必须先显式刷新核对；不得自动重试 POST。返回成功不代表注册赠金或奖励领取完成。

## GET /platform/v1/wallet/ledger

参数：`asset` 必需，为两种本地资产之一；`after_seq` 默认0，非负 int64 十进制；`limit` 默认20，范围1～50。未知、重复或无效参数返回400。

`data`：`items`、`has_more`、`next_after_seq`。按序号升序；有更多时游标为本页最后序号字符串，否则为 null。未初始化时返回空列表，不创建钱包。

每项沿用 A1 LedgerEntry：`id,transaction_id,user_id,asset,ledger_seq,wallet_version,entry_type,biz_type,biz_id,delta_units,balance_before_units,balance_after_units,created_at`；增加精确字符串 `delta_amount,balance_after_amount`。本版没有生成业务流水的公共接口，空流水是正常状态。

## 配置与验证

`MOMIAO_WALLET_DSN_FILE`（绝对路径、普通小文件）与 `MOMIAO_PUBLIC_ORIGIN` 必须成对，且要求完整门户配置；未启用时钱包 API 返回503。DSN 文件内容不写日志或公开仓库。运行时数据库故障只影响钱包，不阻止门户启动；原生核验与数据库操作共享5秒上下文预算，请求读取另受 HTTP 服务器10秒读取超时限制。这些是工程截止时间，不是压测结论。

独立 `cmd/momiao-migrate` 只接受 `MOMIAO_MIGRATION_DSN_FILE`，不接受命令行参数，不在门户启动或请求中调用。正式运行与迁移使用不同凭据/权限。OpenAPI 同步包含上述自有接口，原生 API 仍由原生契约描述。
