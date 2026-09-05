# 设计历史档案

此目录仅归档六份历史设计 Markdown 文本，版本集合为 `2026-09-baseline`。原有文件名、业务数值、路由与视觉意图保留；**归档中的 FINAL / FROZEN 不再直接覆盖现行决策记录，也不表示相应功能已经实现。**

当前实现以 [README](../../../README.md)、[决策 0001](../../decisions/0001-pragmatic-baseline.md)与[已实现接口契约](../../../contracts/openapi.json)为准。历史计划与当前决策冲突时，当前决策优先；后续按功能逐步实现和验证，而不是先构造全部目录、服务和流程。

## 参考文档

- [Chaldea_Platform_Art_Direction_v0.4_FINAL.md](2026-09-baseline/Chaldea_Platform_Art_Direction_v0.4_FINAL.md)
- [Chaldea_Platform_Implementation_Spec_v1.0_FINAL.md](2026-09-baseline/Chaldea_Platform_Implementation_Spec_v1.0_FINAL.md)
- [Chaldea_Platform_Model_Persona_Image_Prompts_WORKING_v0.6.md](2026-09-baseline/Chaldea_Platform_Model_Persona_Image_Prompts_WORKING_v0.6.md)
- [Chaldea_Platform_Technical_Design_v0.5_FINAL.md](2026-09-baseline/Chaldea_Platform_Technical_Design_v0.5_FINAL.md)
- [Chaldea_Platform_全站页面结构设计_v0.3.1_FINAL.md](2026-09-baseline/Chaldea_Platform_全站页面结构设计_v0.3.1_FINAL.md)
- [Chaldea_Platform_需求基线_v0.2.11_奖励数值与迁移初始赠金修订版.md](2026-09-baseline/Chaldea_Platform_需求基线_v0.2.11_奖励数值与迁移初始赠金修订版.md)

## 公开副本的处理

- 每份增加历史参考提示；统一 UTF-8/LF，不改变原始私有文件。
- 工作站绝对路径、会话图片位置与图片内部标识替换为 `examples/private-reference-images` 或 `examples/unpublished-art/` 下的描述性占位路径。
- 历史部署/运行时文件绝对路径替换为 `examples/deployment/`、`examples/runtime/` 等说明路径；它们不是真实部署位置或可直接执行的部署指令。
- HTTP 页面/API 路由、官方公开链接、保留示例域名、业务规则与数值不因文件路径脱敏而改写。
- 不包含图像、参考素材、原始源码核验材料、操作记录、账号数据、Secret 或运行环境。图片占位引用没有对应附件；设计文本及 Prompt 本身不证明素材权利。
- 归档文本里关于前端、数据库、服务、鉴权和工具的表格是历史规划，不是本仓库具备那些能力或依赖的清单。

## 公开文本 SHA-256

下列哈希对应仓库中的脱敏后文件，不对应受限原稿。

- `Chaldea_Platform_Art_Direction_v0.4_FINAL.md`: `0a6cea5e46f4706507edb31eeb477a6220ae4824741c2a72be4111575c9d319e`
- `Chaldea_Platform_Implementation_Spec_v1.0_FINAL.md`: `85ad1031c2c9b16bb47d81636888a8a34a1ad9e3f0eb606d70966997122da7bb`
- `Chaldea_Platform_Model_Persona_Image_Prompts_WORKING_v0.6.md`: `d609c88679cb48e74fb6afdf701fcc8be847a20162069d2c8fd175b73b3e9e19`
- `Chaldea_Platform_Technical_Design_v0.5_FINAL.md`: `5ec995e792f61737bb87dec8b0d3b87efa483c3e29c4c0881f0090e9b1403e67`
- `Chaldea_Platform_全站页面结构设计_v0.3.1_FINAL.md`: `0881cc8918750da6c71e1313f28a15ad8c2bc85934630c09c807f86326295a08`
- `Chaldea_Platform_需求基线_v0.2.11_奖励数值与迁移初始赠金修订版.md`: `1b4c80eba8115f6f368444839e9844e8ce780d861df42c10f762b06cfe36e8a9`
