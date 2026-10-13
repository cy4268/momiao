# 本地模型目录与运营

目录直接读取 `relay.channels`、`relay.prices`、`relay.groups`，与现有元数据做并集。
新增渠道模型立即出现在运营列表，默认未公开；仅剩元数据标为“无渠道”，渠道全停用标为“渠道已停用”。
配置支持不等于上游健康，不宣称所有非文本调用均可用。

## 身份与写入

- 个人报价使用本地 Redis 会话和 `identity.users` 中的实际分组，不接受请求指定分组。
- 运营使用本地 `role >= 10`；写请求经过现有 Origin / CSRF 校验。
- 一个事务保存数据及 `ops.audit_log`。没有操作编号、原因、预览、确认或版本比对。
- 批量公开/隐藏最多 100 个模型，任意一项失败则整批回退。
- 公开仍要求展示名称、家族、简介，沿用原纯文本、受控标签和素材字段校验。
  已明确按次/按秒定价的模型不再强制额外填写特殊计价说明。
- 未配置价格不视为免费；可以保存展示信息，但接入仍不可用。
- 元数据版本、身份历史和旧发布记录保留作历史读取，不作为保存前置条件。

## HTTP

统一使用现有 JSON envelope 和 no-store 响应。

| 方法 / 路径 | 作用 |
|---|---|
| GET `/platform/v1/models` | 公开列表、筛选、分页、同维度价格排序 |
| GET `/platform/v1/models/detail?model_id=...` | 公开详情、词表、API Base URL |
| GET `/platform/v1/models/access-config` | 显式部署的 API Base URL |
| GET `/platform/v1/models/personal-price?model_id=...` | 当前用户分组报价 |
| GET `/platform/v1/ops/models` | 全部本地模型；q / state / offset / limit |
| GET `/platform/v1/ops/models/detail?model_id=...` | `{item,price_settings}`，可编辑基础价格不含分组倍率 |
| PUT `/platform/v1/ops/models` | `{model_id,metadata,public,recommended,sort_order,price_settings}` |
| PUT `/platform/v1/ops/models/publication` | `{model_ids,public}`，批量公开/隐藏 |
| GET `/platform/v1/ops/models/family-covers` | 家族封面和上传配置状态 |
| POST `/platform/v1/ops/models/family-covers/upload` | multipart metadata + file，返回候选资产 |
| PUT `/platform/v1/ops/models/family-covers` | `{family,asset_id}`，空 asset_id 恢复默认 |

`price_settings: null` 保持原价格，缺失仍为未配置。`input`、`output`、`cache_read`、
`cache_write`、`cache_write_1h` 是十进制字符串，单位 Credit / 百万 token。
`per_request`、`per_second` 是可空十进制字符串且互斥；`default_seconds` 为非负整数。
SQL 写入精确乘 500000，读取精确除 500000，不经浮点数。
公开参考价乘 default 分组倍率；本人报价与结算只乘实际分组倍率一次。

公开查询保留 q、availability、family、tag、use_case、recommended、unknown_context、
min_context、price_dimension、min_price、max_price、sort、offset、limit、group_by=family。
价格条件必须明确维度，缺失价格不参与数值范围。默认公开分页24项，Ops50项，最大100项。
模型 ID 沿用不透明文本编码；`.` / `..` 的浏览器后缀为 `~Lg` / `~Li4`。

## 封面、配置与历史

复用 R2 上传与图片校验：静态 PNG/JPEG/WebP，8 MiB，每边8192，总像素16,777,216。
对象键由家族、内容 SHA256、真实扩展名组成；重复图片沿用已登记说明。
上传后素材地址公开，保存后绑定家族；保留来源归属字段和默认素材。

保留 `MOMIAO_API_BASE_URL`、`MOMIAO_ASSET_CDN_ORIGIN` 和既有 R2 配置。
删除目录 reader key、同步间隔、陈旧阈值、worker 与 native 目录解析器。
旧同步表及历史数据保留，不删除或重新导入。
0056/0057 把审计与维护操作者外键接到本地 `identity.users`，不要求先初始化 Master。

## 验收边界

源码与独立开发数据库验证不等于网站已发布。联合发布、浏览器、真实素材上传、游戏及中转验收
仍按迁移计划进行；完整开发验收前不切生产。源码回退不降库，保留已应用迁移及历史数据。
