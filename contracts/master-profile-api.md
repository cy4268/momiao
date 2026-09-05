# Master Profile API — A2b

路径使用 `/platform/v1` 避免覆盖原生 `/api`。仅本人操作；每个请求用已有 `Authorization`、`New-Api-User` 和可选 `X-Auth-Session` 经固定原生 Unix `/api/user/self` 核验身份，不接受 body/query 指定归属。

## 查询

`GET /platform/v1/master-profile` 不写数据库。成功 envelope 为 `{success:true,data:Profile}`：

```json
{
  "user_id": "1",
  "short_account_id": "CA-0123456789AB",
  "status": "INCOMPLETE",
  "display_name": "",
  "avatar_id": "system-default",
  "profile_version": "0",
  "nickname_changed_at": null,
  "next_rename_at": null,
  "suggested_name": "Master-CA-0123456789AB",
  "avatars": [{"id":"system-default","label":"系统默认头像","source":"SYSTEM"}]
}
```

示例短ID是占位示意，不是用户1的实际计算结果。真实 short ID 由 `SHA256("chaldea-short-account-id-v1\0" + 十进制稳定ID)` 的前12位大写hex加 `CA-` 得到，只是私有展示定位符，不用于认证/授权/FK。昵称建议不自动保存或保证可用。

未保存的资料投影为 INCOMPLETE/version0/空昵称，并不声称已有数据库 profile 行。保存后为 COMPLETE、正版本号；ID/版本均十进制字符串，时间为 UTC RFC3339 字符串或null。首次初始化的改名时间/下次可改时间均null。

## 初始化

`POST /platform/v1/master-profile/initialize`：

```json
{"expected_version":"0","display_name":"你的昵称","avatar_id":"system-default"}
```

必须准确配置 Origin 与 JSON Content-Type。服务端原子建立身份锚点、资料和初始名称历史；不初始化钱包或发资产。若 version1 且内容与已保存的初始资料完全相同，重复初始化返回原资料且无新历史；更改请求或已经改名则409。发生网络/5xx未知结果，客户端先GET核对，不自动重放。

## 修改

`PATCH /platform/v1/master-profile`：

```json
{"expected_version":"1","display_name":"新的昵称"}
```

`display_name`、`avatar_id` 至少出现一个，当前头像仅支持 `system-default`；expected_version必须正版本字符串。先锁行并检查版本。精确无变化不增版本/历史；实际显示名改变（含仅大小写）追加历史并更新DB时间，接下来7天内拒绝再改名。头像操作不消耗昵称冷却。客户端时钟或禁用按钮不是最终权威。

所有路径拒绝 query、重复/未知JSON字段和null字段，请求JSON读取上限8192字节。无 DELETE、任意用户查询或管理强制改名接口。

## 昵称校验和状态

服务端检查原始UTF-8及禁止字符，NFKC后移除首尾普通空格、合并U+0020，再校验1–24 grapheme、文字/数字/合法附着组合符/空格/`_`/`-`/`·`，最后 Unicode Case Fold 得到DB唯一值。原始Unicode Symbol及ℹ在规范化前拒绝，以免符号规范化为普通文字；原始与规范化后展示名上限4096字节，fold后上限8192字节。`Alice`、`alice`、`Ａｌｉｃｅ`冲突。保留名基线至少含设计列举的管理/官方/客服名称及本项目品牌，移除四种允许分隔符后进行规范化精确匹配；这不是完整的同形异义字或敏感词审核系统。

错误 envelope `{success:false,code,message}`；message是通用HTTP说明，客户端按code显示具体中文提示：

| HTTP | code / 含义 |
|---|---|
| 400 | INVALID_REQUEST、INVALID_NICKNAME、INVALID_AVATAR |
| 401 | AUTH_UNAUTHORIZED |
| 403 | AUTH_FORBIDDEN、ORIGIN_REJECTED、NICKNAME_RESERVED；不因此全局登出 |
| 404/405 | NOT_FOUND / METHOD_NOT_ALLOWED |
| 409 | NICKNAME_TAKEN、STALE_RESOURCE_VERSION、RENAME_COOLDOWN |
| 415 | INVALID_CONTENT_TYPE |
| 502 | AUTH_UNAVAILABLE |
| 503 | PROFILE_UNAVAILABLE |

响应 `Cache-Control:no-store`。身份核验和DB操作共享5秒上下文预算，请求读取另受HTTP服务器10秒读取超时约束。无额外数据库环境变量：与钱包共享受限平台连接；独立迁移升级到schema2，不在门户启动时迁移。
