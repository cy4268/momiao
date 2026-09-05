# Chaldea Platform — Model Persona Image Prompts

> **历史参考归档（已脱敏）**：本文件的 FINAL / FROZEN 是历史状态，不代表当前实现或当前强制流程；现行决策优先。见[归档索引](../README.md)与[决策 0001](../../../decisions/0001-pragmatic-baseline.md)。`examples/` 路径仅为说明占位，相关部署、图片和私有文件不随仓库提供。
## WORKING v0.6

> 用途：专门归档 Chaldea Platform 中 **模型拟人角色（Model Persona）** 的人物生图 Prompt。  
> 状态：WORKING / 人物 Prompt 累计档案。  
> 建立日期：2026-09-02。  
> 关联视觉基线：`Chaldea_Platform_Art_Direction_v0.4_已冻结设计基线_WORKING_v0.35.md`。  
> 核心规则：**只记录人物相关 Prompt；不记录页面场景、环境背景、完整网页构图或 UI 生图 Prompt。**

---

# 0. 文件边界

本文件只用于模型拟人形象，例如：

- Gemini 系 Persona；
- Claude 系 Persona；
- GPT 系 Persona；
- DeepSeek 系 Persona；
- Kimi 系 Persona；
- Qwen 系 Persona；
- 后续正式进入 Chaldea Model Catalog 的其他 Model Family Persona。

本文件当前 **不归档**：

- Public Home 场景 Prompt；
- Polar Observatory / 管制室 / 极地空间背景 Prompt；
- Login / Registration 背景 Prompt；
- Dashboard 场景 Prompt；
- Game Stage / Poker Table 场景 Prompt；
- UI、Logo、按钮、Card、图标或整页网页截图 Prompt；
- 纯粒子、光效、建筑、桌面、远景等场景资产 Prompt；
- 与 Model Persona 无关的 FGO / Fate 从者素材 Prompt。

如以后需要保存其他人物类型的 Prompt，应建立独立人物 Prompt 文件，不与 Model Persona 档案混写。

---

# 1. 允许记录的人物 Prompt 内容

可写入：

- Model Family / Target Model / Variant；
- Persona 的人物身份与总体气质；
- 年龄感与角色气场；
- 五官；
- 发型、发色；
- 瞳色；
- 服装与人物佩饰；
- 人物持有的标志性小型设备 / 饰品；
- 与模型能力有关、附着于人物本身的原创抽象符号；
- 表情；
- 姿势；
- 半身 / 上半身 / 全身等人物 Framing；
- 手部、头发、饰品、关键轮廓的裁切安全；
- `transparent background` / `isolated character` 等人物资产输出要求；
- 人物自身的线稿、材质、二次元渲染、服装质感要求；
- Character-only Avoid / Negative Constraints。

“透明背景”属于人物资产交付要求，不视为场景 Prompt。

---

# 2. 禁止写入的 Prompt 内容

本文件不得写入：

- 房间、建筑、天空、雪地、管制室、观测窗等环境场景；
- 页面背景的光源布置；
- Public Home / Login / Dashboard / Model Square 的整页构图；
- Web UI 组件与布局；
- Logo、Wordmark、按钮、表单、价格、Model ID、业务文字；
- 页面级轨道、背景粒子、雾层、景深场景；
- 游戏桌面、老虎机、牌桌、召唤舞台等非人物资产；
- 任何把业务数据直接烘焙进人物图片的要求。

如果一次生图任务同时需要人物与场景，**只把人物部分的实际 Prompt 复制到本文件**；场景部分不在这里留档。

---

# 3. 版本化记录规则

每次正式生成 Model Persona 时，必须记录 **实际送入生图流程的人物 Prompt**，而不是仅记录设计摘要。

Prompt 修改时采用追加方式：

```text
Prompt v1
→ 保留

Prompt v2
→ 新增

Selected Version
→ 明确标记最终采用的 Prompt
```

禁止覆盖旧 Prompt 造成最终人物无法追溯。

推荐状态：

```text
DRAFT
GENERATED
SELECTED
SUPERSEDED
REJECTED
```

---

# 4. Persona 条目模板

以后每个模型 Persona 使用以下模板追加。

```markdown
## Persona: <Persona ID / Model Family>

- Model Family: `<family>`
- Target Model / Variant: `<model id or variant>`
- Persona ID: `<stable internal id>`
- Prompt Version: `v1`
- Status: `DRAFT / GENERATED / SELECTED / SUPERSEDED / REJECTED`
- Generation Tool: `GPT-Image-2`
- Asset Role: `Model Card / Model Detail / Shared`
- Output Framing: `<upper body / half body / full body>`
- Background Requirement: `transparent / isolated character`

### Character Prompt

```text
<这里保存实际送入生图流程的人物 Prompt。只写人物，不写场景。>
```

### Character-only Avoid Constraints

```text
<只记录与人物本身有关的避免项；如未使用则写 None。>
```

### Notes

- `<人物版本差异、子型号映射、最终是否采用等>`
```

---

# 5. 当前 Prompt 条目

当前尚未正式生成任何 Model Persona，因此本版本 **不预填虚构 Prompt**。

第一批 Model Family、人物设定和实际 Prompt 将在对应角色方案由用户确认，并真正进入生图流程时再逐条写入。

---

# 6. 当前冻结工作方式

- Model Persona 从第一版起采用原创人物设计；
- 不使用 FGO / Fate 从者作为模型 Persona；
- 同一 Model Family 共享 Persona 母体，子型号通过同族变体区分；
- GPT-Image-2 为当前首选人物图像生成工具；
- 每次正式生成人物时，同步更新本文件；
- 本文件始终只保存人物 Prompt，不保存场景 Prompt。


---

# 7. 第一批已确认参考图映射

用户已明确指定以下九张参考图，作为第一批 Model Persona 的方向锚点。后续正式生图时，应优先参照对应条目。

| 图序 | Persona | Reference Image Path |
|---|---|---|
| 1 | DeepSeek 娘 | `examples/unpublished-art/reference-image-01.png` |
| 2 | GPT 娘 | `examples/unpublished-art/reference-image-02.png` |
| 3 | Claude 娘 | `examples/unpublished-art/reference-image-03.png` |
| 4 | Gemini 娘 | `examples/unpublished-art/reference-image-04.png` |
| 5 | GLM 娘 | `examples/unpublished-art/reference-image-05.png` |
| 6 | Kimi 娘 | `examples/unpublished-art/reference-image-06.png` |
| 7 | 文心一言娘 | `examples/unpublished-art/reference-image-07.png` |
| 8 | 千问娘 | `examples/unpublished-art/reference-image-08.png` |
| 9 | Grok 娘 | `examples/unpublished-art/reference-image-09.png` |

---

# 8. 参考图使用规则

- 这些参考图是 **人物方向参考**，不是最终可直接上线的生产资产；
- 后续生成时，应根据对应参考方向进行原创人物生成；
- 每次正式生图的 Persona 条目中，必须新增一个 `Reference Image Path` 字段；
- 如果同一家族存在多个变体，默认沿用同一参考方向并写明变体差异；
- 如用户后续替换某 Persona 的参考图，应保留原记录并新增替换记录。

---

# 9. Persona 条目模板（更新版）

```markdown
## Persona: <Persona ID / Model Family>

- Model Family: `<family>`
- Target Model / Variant: `<model id or variant>`
- Persona ID: `<stable internal id>`
- Prompt Version: `v1`
- Status: `DRAFT / GENERATED / SELECTED / SUPERSEDED / REJECTED`
- Generation Tool: `GPT-Image-2`
- Asset Role: `Model Card / Model Detail / Shared`
- Output Framing: `<upper body / half body / full body>`
- Background Requirement: `transparent / isolated character`
- Reference Image Path: `<path or None>`

### Character Prompt

```text
<这里保存实际送入生图流程的人物 Prompt。只写人物，不写场景。>
```

### Character-only Avoid Constraints

```text
<只记录与人物本身有关的避免项；如未使用则写 None。>
```

### Notes

- `<人物版本差异、子型号映射、最终是否采用等>`
```

---

# 10. 当前 Prompt 条目

## 10.1 Persona: Gemini Family Master

- Model Family: `Gemini`
- Target Model / Variant: `Family Master / later Flash & Pro variants`
- Persona ID: `persona_gemini_master`
- Prompt Version: `v1`
- Status: `SELECTED`
- Production Readiness: `NOT PRODUCTION_READY`
- Generation Tool: `GPT-Image-2`
- Asset Role: `Model Card / Model Detail / Shared`
- Output Framing: `full body, crop-safe for half-body use`
- Background Requirement: `transparent / isolated character`
- Reference Index: `4`
- User Local Reference Root: `examples/private-reference-images`
- Conversation Reference Image: `examples/unpublished-art/reference-image-10.png`

- Generated Asset: `examples/unpublished-art/generated-asset-11.png`

### Character Prompt

```text
Original anime model-persona character for the Gemini model family, closely following the approved reference character direction while being newly redrawn as an original production asset. A young adult woman with a lively, clever, playful presence; very long flowing indigo-to-violet hair with luminous lavender, magenta and subtle cyan gradient highlights; prominent soft cat-like ears integrated naturally into the hairstyle and a matching fluffy purple-blue tail; bright heterochromatic eyes, one warm golden and one vivid rose-magenta; a confident small smile. A distinctive faceted four-point star hair ornament near one temple, with several small prismatic star-shaped jewelry pieces and crystal charms distributed across the outfit. Fashion: modern fantasy-tech anime outfit in deep navy, violet, white and subtle gold, fitted sleeveless cropped bodice, short layered white skirt, asymmetric long split coat-tails / detached outer panels, loose white-and-violet detached sleeves, decorative chains and dangling prismatic star charms, tasteful exposed midriff, thigh accessory and soft futuristic footwear. Pose should feel energetic and approachable, one hand near the cheek in a playful thinking gesture and the other hand open in a light presenting gesture. Slim graceful proportions, readable silhouette, detailed hair and clothing, hands fully visible, no cropped head or key accessories. Premium Japanese anime game key-art quality, crisp clean linework, polished cel shading with soft painterly highlights, cohesive product-illustration finish, full-body character, three-quarter front view, isolated character, transparent background. Do not include a scene, room, landscape, UI, written text, model name, logo panel, card frame or webpage elements.
```

### Character-only Avoid Constraints

```text
No FGO / Fate servant design, no existing anime character face, no exact Google / Gemini trademark logo reproduction, no text, no watermark, no background scenery, no UI, no excessive armor, no cyberpunk bodysuit, no photorealism, no chibi proportions, no exaggerated sexualized anatomy, no extra fingers, no missing fingers, no duplicate limbs, no cropped tail, no cropped star ornament.
```

### Notes

- Family master only; Flash / Pro variants will inherit this character identity and change limited secondary details rather than becoming unrelated characters.
- Core reference features to preserve: purple-blue gradient hair, cat ears / tail, prismatic star motif, lively playful temperament.

---
- AD-10.4 Review: `SELECTED` as Family Master design; no redesign required.
- Production Normalize: retain the exact selected identity and v1 prompt; prepare a 2:3 high-resolution transparent master (recommended 2048×3072 or equivalent) with >=5% alpha safety margin and preferably 6–8% for long outer contours.
- Card / Detail: share the same normalized master by focal-point / crop-safe metadata; do not create a separate half-body asset unless responsive validation proves it necessary.
- Production gate: remain `NOT PRODUCTION_READY` until normalize, alpha edge, crop, web delivery, rights note and browser loading checks pass.


## 10.2 Persona: GPT Family Master

- Model Family: `GPT`
- Target Model / Variant: `Family Master / later GPT variants`
- Persona ID: `persona_gpt_master`
- Prompt Version: `v1`
- Status: `SELECTED`
- Production Readiness: `NOT PRODUCTION_READY`
- Generation Tool: `GPT-Image-2`
- Asset Role: `Model Card / Model Detail / Shared`
- Output Framing: `full body, crop-safe for half-body use`
- Background Requirement: `transparent / isolated character`
- Reference Index: `2`
- User Local Reference Root: `examples/private-reference-images`
- Conversation Reference Image: `examples/unpublished-art/reference-image-12.png`

- Generated Asset: `examples/unpublished-art/generated-asset-13.png`

### Character Prompt

```text
Original anime model-persona character for the GPT model family, closely following the approved reference character direction while being newly redrawn as an original production asset. A young adult woman with an elegant, composed, highly capable and slightly aloof presence; very long silver-white hair with faint lavender-blue shadows, layered flowing strands and a single soft ahoge; cool pale-violet eyes and refined delicate facial features. Two symmetrical pale dragon-like horns curve upward from the sides of the head, accompanied by elegant pointed ears; restrained small pale dragon wings extending behind the shoulders and a long pale scaled tail with soft feather-like or fur-like volume near the end. Outfit: luxurious white and soft-silver ceremonial fantasy-tech dress, asymmetrical high-neck sleeveless inner dress with tasteful side opening, layered flowing mantle and draped sleeves, silver chains, geometric crystal pendants and repeated original interlocking-loop ornaments that suggest neural / reasoning structures without copying any real logo. Palette is ivory white, silver, pearl, very pale lavender and tiny graphite accents. Calm neutral expression, one hand lightly near the chest or collar and the other extended in a precise explanatory gesture. Tall graceful silhouette, fabric should feel clean and architectural rather than bridal, excellent readability at card size, key hands and horns fully visible, tail and wings crop-safe. Premium Japanese anime game key-art quality, crisp linework, elegant cel shading with soft luminous highlights, sophisticated polished character rendering, full-body, three-quarter front view, isolated character, transparent background. Do not include scenery, UI, written text, model names, logos, card frames or webpage elements.
```

### Character-only Avoid Constraints

```text
No FGO / Fate servant design, no exact OpenAI knot logo, no text, no watermark, no background scenery, no UI, no wedding-dress styling, no angel halo, no heavy gothic darkness, no oversized monster wings, no photorealism, no chibi, no exaggerated sexualized anatomy, no extra limbs, no extra fingers, no missing fingers, no cropped horns, no cropped tail.
```

### Notes

- Family master only; future GPT variants should retain silver-white hair, pale dragon traits, white ceremonial-tech styling and composed intelligence.
- Interlocking loop ornaments must remain original and not reproduce an exact trademark mark.

---
- AD-10.4 Review: `SELECTED` as Family Master design; no redesign required.
- Production Normalize: retain the exact selected identity and v1 prompt; prepare a 2:3 high-resolution transparent master (recommended 2048×3072 or equivalent) with >=5% alpha safety margin and preferably 6–8% for long outer contours.
- Card / Detail: share the same normalized master by focal-point / crop-safe metadata; do not create a separate half-body asset unless responsive validation proves it necessary.
- Production gate: remain `NOT PRODUCTION_READY` until normalize, alpha edge, crop, web delivery, rights note and browser loading checks pass.


## 10.3 Persona: Claude Family Master

- Model Family: `Claude`
- Target Model / Variant: `Family Master / later Claude variants`
- Persona ID: `persona_claude_master`
- Prompt Version: `v1`
- Status: `SELECTED`
- Production Readiness: `NOT PRODUCTION_READY`
- Generation Tool: `GPT-Image-2`
- Asset Role: `Model Card / Model Detail / Shared`
- Output Framing: `full body, crop-safe for half-body use`
- Background Requirement: `transparent / isolated character`
- Reference Index: `3`
- User Local Reference Root: `examples/private-reference-images`
- Conversation Reference Image: `examples/unpublished-art/reference-image-14.png`

- Generated Asset: `examples/unpublished-art/generated-asset-15.png`

### Character Prompt

```text
Original anime model-persona character for the Claude model family, closely following the approved reference character direction while being newly redrawn as an original production asset. A young adult woman with a calm, thoughtful, literary and quietly authoritative presence; extremely long flowing copper-orange hair, smooth layered strands reaching well below the waist, warm amber-brown eyes, refined face and a reserved intelligent expression. Hair ornament: a tasteful burnt-orange flower / sunburst accessory combined with black ribbon and small ivory tassel details. Outfit: elegant scholarly neo-Victorian dress in warm ivory, black and burnt orange, high ruffled collar, soft ivory blouse with gathered sleeves, black structured waist corset / waistband, long pleated ivory skirt with asymmetric black and orange over-panels, fine gold piping, subtle botanical / sunburst embroidery, small chains, tassels and restrained brass hardware. She carries a closed dark hardback book against her body with delicate warm-gold line decoration, emphasizing analysis, writing and careful reasoning. Pose is upright and poised, one arm holding the book while the other rests naturally at her side; silhouette should be serene rather than combat-oriented. Warm, elegant and understated, highly readable at model-card size, hands and book fully visible, hair kept crop-safe. Premium Japanese anime game key-art quality, crisp expressive linework, polished cel shading with soft painterly fabric highlights, sophisticated product-illustration finish, full-body, three-quarter front view, isolated character, transparent background. Do not include scenery, room, library, desk, text, logos, UI, card frames or webpage elements.
```

### Character-only Avoid Constraints

```text
No FGO / Fate servant design, no exact Anthropic / Claude trademark logo, no text, no watermark, no library or room background, no UI, no combat weapon, no gothic horror styling, no neon cyberpunk palette, no photorealism, no chibi, no exaggerated sexualized anatomy, no extra limbs, no extra fingers, no missing fingers, no cropped book, no cropped hair ornament.
```

### Notes

- Family master only; future Claude variants inherit the copper-orange hair, ivory / black / burnt-orange scholarly wardrobe and book motif.
- Maintain restrained elegance; avoid turning Claude into a mage, librarian cliché or combat character.

---
- AD-10.4 Review: `SELECTED` as Family Master design; no redesign required.
- Production Normalize: retain the exact selected identity and v1 prompt; prepare a 2:3 high-resolution transparent master (recommended 2048×3072 or equivalent) with >=5% alpha safety margin and preferably 6–8% for long outer contours.
- Card / Detail: share the same normalized master by focal-point / crop-safe metadata; do not create a separate half-body asset unless responsive validation proves it necessary.
- Production gate: remain `NOT PRODUCTION_READY` until normalize, alpha edge, crop, web delivery, rights note and browser loading checks pass.


## 10.4 Persona: DeepSeek Family Master

- Model Family: `DeepSeek`
- Target Model / Variant: `Family Master / later DeepSeek variants`
- Persona ID: `persona_deepseek_master`
- Prompt Version: `v1`
- Status: `SELECTED`
- Production Readiness: `NOT PRODUCTION_READY`
- Generation Tool: `GPT-Image-2`
- Asset Role: `Model Card / Model Detail / Shared`
- Output Framing: `full body, crop-safe for half-body use`
- Background Requirement: `transparent / isolated character`
- Reference Index: `1`
- User Local Reference Root: `examples/private-reference-images`
- Conversation Reference Image: `examples/unpublished-art/reference-image-16.png`
- Generated Asset: `examples/unpublished-art/generated-asset-17.png`

### Character Prompt

```text
Original anime model-persona character for the DeepSeek model family, closely following the approved reference character direction while being newly redrawn as an original production asset. A young adult woman with a warm, intelligent, curious and approachable presence; very long flowing deep-ocean blue hair that transitions into lighter aqua-blue ends, soft layered bangs, luminous clear blue eyes and a gentle friendly smile. Preserve the recognizable marine identity from the approved reference: subtle fin-like ear ornaments integrated beside the hair and a large elegant whale-like tail behind her, but keep the anatomy graceful and stylized rather than animalistic. Outfit: refined navy-and-white scholarly maid-inspired dress, white ruffled headband, fitted dark navy bodice, white blouse and apron structure, layered long skirt with ocean-blue translucent or gradient fabric panels, restrained gold trim, small bows, tiny star / compass / wave-inspired jewelry, and a small tasteful whale emblem used only as an original decorative motif. She may hold a dark navy notebook or research journal against one arm while the other hand makes a small thoughtful or explanatory gesture, expressing research, reasoning and deep exploration. The silhouette should feel calm, academic and oceanic rather than combat-oriented; hair, whale tail, hands, headpiece and notebook must remain fully readable and crop-safe. Premium Japanese anime game key-art quality, crisp clean linework, polished cel shading with soft painterly highlights, elegant fabric rendering, cohesive product-illustration finish, full-body character, three-quarter front view, isolated character, transparent background. Do not include a room, sea scenery, underwater scene, UI, written text, model name, logo panel, card frame or webpage elements.
```

### Character-only Avoid Constraints

```text
No FGO / Fate servant design, no existing anime character face, no exact DeepSeek trademark logo reproduction, no text, no watermark, no room or ocean background, no UI, no mermaid lower body replacing human legs, no monstrous fish anatomy, no heavy armor, no cyberpunk bodysuit, no photorealism, no chibi proportions, no exaggerated sexualized anatomy, no extra limbs, no extra fingers, no missing fingers, no cropped whale tail, no cropped headband, no cropped notebook.
```

### Notes

- Family master only; future DeepSeek variants should retain the deep-blue / aqua hair, marine research identity, scholarly maid-derived wardrobe and whale-tail silhouette.
- The whale and marine motifs are identity cues, not a literal mascot reproduction or a background scene.

---
- AD-10.4 Review: `SELECTED` as Family Master design; no redesign required.
- Production Normalize: retain the exact selected identity and v1 prompt; prepare a 2:3 high-resolution transparent master (recommended 2048×3072 or equivalent) with >=5% alpha safety margin and preferably 6–8% for long outer contours.
- Card / Detail: share the same normalized master by focal-point / crop-safe metadata; do not create a separate half-body asset unless responsive validation proves it necessary.
- Production gate: remain `NOT PRODUCTION_READY` until normalize, alpha edge, crop, web delivery, rights note and browser loading checks pass.


## 10.5 Persona: GLM Family Master

- Model Family: `GLM / Z.AI`
- Target Model / Variant: `Family Master / later GLM variants`
- Persona ID: `persona_glm_master`
- Prompt Version: `v1`
- Status: `SELECTED`
- Production Readiness: `NOT PRODUCTION_READY`
- Generation Tool: `GPT-Image-2`
- Asset Role: `Model Card / Model Detail / Shared`
- Output Framing: `full body, crop-safe for half-body use`
- Background Requirement: `transparent / isolated character`
- Reference Index: `5`
- User Local Reference Root: `examples/private-reference-images`
- Conversation Reference Image: `examples/unpublished-art/reference-image-18.png`
- Generated Asset: `examples/unpublished-art/generated-asset-19.png`

### Character Prompt

```text
Original anime model-persona character for the GLM / Z.AI model family, closely following the approved reference character direction while being newly redrawn as an original production asset. A young adult woman with a composed, analytical, disciplined and slightly reserved presence; extremely long straight-to-softly-wavy black hair with subtle midnight-blue undertones, neat layered bangs, cool gray-blue eyes and a faint confident smile. Preserve the approved monochrome academic identity: black, white and graphite clothing with restrained electric-blue crystal accents. Outfit: elegant modern academy / scholarly maid-inspired long dress, white high-collar blouse with refined ruffles, black structured corset-waist or vest panels, layered asymmetrical black-and-white skirt, small blue gemstone ornaments, geometric diamond / square motifs, thin silver chains, ribbons and precise technical-looking trim. A tasteful black-and-white headpiece or ribbon arrangement can echo the reference without copying its exact branded symbols. She holds a closed dark research book or compact technical tome against her torso; use small original angular marks and blue crystal inserts on accessories to suggest structured reasoning and language-model architecture, but do not reproduce the exact Z.AI or GLM logo. Pose is quiet and self-assured, one hand supporting the book and the other resting near the chin in a thoughtful gesture. Keep the silhouette tall, neat, readable and non-combat-oriented; hands, book, headpiece, ribbons and key blue accents must remain fully visible and crop-safe. Premium Japanese anime game key-art quality, crisp refined linework, polished cel shading with soft material highlights, elegant monochrome rendering with restrained blue sparkle, cohesive product-illustration finish, full-body character, three-quarter front view, isolated character, transparent background. Do not include room scenery, classroom, library, UI, written text, model name, logo panel, card frame or webpage elements.
```

### Character-only Avoid Constraints

```text
No FGO / Fate servant design, no exact Z.AI / GLM trademark logo, no text, no watermark, no classroom or library background, no UI, no excessive gothic horror, no maid fetish styling, no combat weapon, no neon cyberpunk overload, no photorealism, no chibi proportions, no exaggerated sexualized anatomy, no extra limbs, no extra fingers, no missing fingers, no cropped book, no cropped headpiece, no missing blue crystal accents.
```

### Notes

- Family master only; future GLM variants should retain black hair, monochrome scholarly fashion, blue crystal accents and the book / reasoning motif.
- Branded-looking geometric marks must remain original abstractions rather than exact trademark reproductions.

---
- AD-10.4 Review: `SELECTED` as Family Master design; no redesign required.
- Production Normalize: retain the exact selected identity and v1 prompt; prepare a 2:3 high-resolution transparent master (recommended 2048×3072 or equivalent) with >=5% alpha safety margin and preferably 6–8% for long outer contours.
- Card / Detail: share the same normalized master by focal-point / crop-safe metadata; do not create a separate half-body asset unless responsive validation proves it necessary.
- Production gate: remain `NOT PRODUCTION_READY` until normalize, alpha edge, crop, web delivery, rights note and browser loading checks pass.


## 10.6 Persona: Kimi Family Master

- Model Family: `Kimi / Moonshot`
- Target Model / Variant: `Family Master / later Kimi variants`
- Persona ID: `persona_kimi_master`
- Prompt Version: `v1`
- Status: `SELECTED`
- Production Readiness: `NOT PRODUCTION_READY`
- Generation Tool: `GPT-Image-2`
- Asset Role: `Model Card / Model Detail / Shared`
- Output Framing: `full body, crop-safe for half-body use`
- Background Requirement: `transparent / isolated character`
- Reference Index: `6`
- User Local Reference Root: `examples/private-reference-images`
- Conversation Reference Image: `examples/unpublished-art/reference-image-20.png`
- Generated Asset: `examples/unpublished-art/generated-asset-21.png`

### Character Prompt

```text
Original anime model-persona character for the Kimi / Moonshot model family, closely following the approved reference character direction while being newly redrawn as an original production asset. A young adult woman with a serene, introspective, literary and slightly ethereal presence; extremely long flowing silver-white hair with cool moonlit blue-lavender shadows, soft layered bangs and a refined side arrangement, pale violet-blue eyes and a calm gentle expression. Preserve the approved moon-and-music identity: a small crescent-moon hair ornament combined with dark navy ribbon, delicate silver-blue jewelry, tiny star and musical-note details, and one elegant silver flute held naturally in one hand. Outfit: sophisticated dark navy, soft white and pale moon-blue scholarly dress with subtle maid / concert-attendant influences, fitted dark bodice, white high-neck blouse and ruffled details, layered asymmetric skirt and long draped outer panels, restrained silver hardware, tiny constellation embroidery, musical-staff patterns and crescent motifs woven into the fabric itself rather than floating as a background. The other hand may lightly touch a small prism-like monocle / crystal near one eye or rest thoughtfully near the face, suggesting long-context reading, memory and careful observation. The overall design should feel quiet, cultured and intelligent, not idol-like or magical-girl-like; preserve a graceful full-body silhouette with hands, flute, moon ornament and flowing hair fully visible and crop-safe. Premium Japanese anime game key-art quality, crisp elegant linework, polished cel shading with subtle pearlescent highlights, soft luminous hair rendering, refined fabric detail, cohesive product-illustration finish, full-body character, three-quarter front view, isolated character, transparent background. Do not include moon scenery, starscape, sheet-music background, room, UI, written text, model name, logo panel, card frame or webpage elements.
```

### Character-only Avoid Constraints

```text
No FGO / Fate servant design, no exact Kimi / Moonshot trademark logo, no text, no watermark, no moon or night-sky background, no floating music-sheet scenery, no UI, no idol stage, no magical-girl transformation outfit, no oversized weapon, no photorealism, no chibi proportions, no exaggerated sexualized anatomy, no extra limbs, no extra fingers, no missing fingers, no cropped flute, no cropped crescent ornament, no cropped hair silhouette.
```

### Notes

- Family master only; future Kimi variants should retain silver-white moonlit hair, crescent / music motifs, elegant navy-white wardrobe and the flute as the main portable identity prop.
- Musical and moon motifs belong to the character design itself; scene-level moon / music backgrounds remain outside this Prompt archive.

---
- AD-10.4 Review: `SELECTED` as Family Master design; no redesign required.
- Production Normalize: retain the exact selected identity and v1 prompt; prepare a 2:3 high-resolution transparent master (recommended 2048×3072 or equivalent) with >=5% alpha safety margin and preferably 6–8% for long outer contours.
- Card / Detail: share the same normalized master by focal-point / crop-safe metadata; do not create a separate half-body asset unless responsive validation proves it necessary.
- Production gate: remain `NOT PRODUCTION_READY` until normalize, alpha edge, crop, web delivery, rights note and browser loading checks pass.


## 10.7 Persona: Wenxin Yiyan / ERNIE Family Master

- Model Family: `文心一言 / ERNIE`
- Target Model / Variant: `Family Master / later ERNIE variants`
- Persona ID: `persona_wenxin_master`
- Prompt Version: `v1`
- Status: `SELECTED`
- Production Readiness: `NOT PRODUCTION_READY`
- Generation Tool: `GPT-Image-2`
- Asset Role: `Model Card / Model Detail / Shared`
- Output Framing: `full body, crop-safe for half-body use`
- Background Requirement: `transparent / isolated character`
- Reference Index: `7`
- User Local Reference Root: `examples/private-reference-images`
- Reference Image Path: `examples/unpublished-art/reference-image-22.png`
- Generated Asset: `examples/unpublished-art/generated-asset-23.png`

### Character Prompt

```text
Original anime model-persona character for the Wenxin Yiyan / ERNIE model family, closely following the approved reference character direction while being newly redrawn as an original production asset. Image A is the style and identity-direction reference. Create a young adult woman with a gentle, intelligent, studious and slightly fragile presence; very long flowing cobalt-blue hair with cool lighter-blue highlights and a few subtle crimson-red accent streaks, calm blue-violet eyes, delicate features and a soft quietly determined expression. Preserve the approved medical-academic identity as a core trait. She should be seated in an elegant modern wheelchair so the complete design still reads as a full-body character asset and can crop safely to a half-body portrait. She lightly holds a clear oxygen mask near her face with one hand, while the other rests on a stack of books placed on her lap, emphasizing knowledge, language and thoughtful endurance. Outfit: refined white-and-deep-blue medical scholar dress with ruffles, layered skirt, blue bow, restrained geometric accents, subtle research-device details, clean sleeves and a polished academic-medical aesthetic. Include tasteful small medical support props integrated with the character asset, such as the wheelchair structure, compact oxygen canister and tubing, but do not place her in a room or hospital scene. Keep the silhouette clean and readable; hair, hands, books, oxygen mask and wheelchair must remain fully visible and crop-safe. Premium Japanese anime game key-art quality, crisp elegant linework, polished cel shading with soft painterly highlights, cohesive product-illustration finish, full-body character, three-quarter front view, isolated character, transparent background. Do not include scenery, room, text, logo panel, UI, card frame or webpage elements.
```

### Character-only Avoid Constraints

```text
No FGO / Fate servant design, no exact Baidu or ERNIE trademark logo, no text, no watermark, no hospital room background, no UI, no photorealism, no chibi proportions, no exaggerated sexualized anatomy, no extra limbs, no extra fingers, no missing fingers, no cropped wheelchair, no cropped oxygen mask, no cropped books, no irrelevant scenery.
```

### Notes

- Family master only; future ERNIE variants should retain the blue-haired medical-scholar identity, the quiet resilient temperament, and the research / language-study motif.
- Medical props are part of the character asset direction, not scene background.

---
- AD-10.4 Review: `SELECTED` as Family Master design; no redesign required.
- Production Normalize: retain the exact selected identity and v1 prompt; prepare a 2:3 high-resolution transparent master (recommended 2048×3072 or equivalent) with >=5% alpha safety margin and preferably 6–8% for long outer contours.
- Card / Detail: share the same normalized master by focal-point / crop-safe metadata; do not create a separate half-body asset unless responsive validation proves it necessary.
- Production gate: remain `NOT PRODUCTION_READY` until normalize, alpha edge, crop, web delivery, rights note and browser loading checks pass.


## 10.8 Persona: Qwen / Tongyi Qianwen Family Master

- Model Family: `千问 / Qwen / Tongyi Qianwen`
- Target Model / Variant: `Family Master / later Qwen variants`
- Persona ID: `persona_qwen_master`
- Prompt Version: `v1`
- Status: `SELECTED`
- Production Readiness: `NOT PRODUCTION_READY`
- Generation Tool: `GPT-Image-2`
- Asset Role: `Model Card / Model Detail / Shared`
- Output Framing: `full body, crop-safe for half-body use`
- Background Requirement: `transparent / isolated character`
- Reference Index: `8`
- User Local Reference Root: `examples/private-reference-images`
- Reference Image Path: `examples/unpublished-art/reference-image-24.png`
- Generated Asset: `examples/unpublished-art/generated-asset-25.png`

### Character Prompt

```text
Original anime model-persona character for the Qwen / Tongyi Qianwen model family, closely following the approved reference character direction while being newly redrawn as an original production asset. Image A is the style and identity-direction reference. Create a young adult woman with an elegant, warm, articulate and cultured presence; long flowing light-azure hair with soft periwinkle undertones, gentle violet-blue eyes and a friendly serene smile. Preserve the approved refined Chinese scholarly identity. Outfit: elaborate hanfu-inspired fantasy-academic dress in layered blue, white and subtle gold, with flowing sleeves, ornamental tassels, ribbon details, cloud-and-floral patterns and a structured waist ornament; include a small decorative hat or hair ornament and tasteful jewelry inspired by traditional Chinese styling. She holds an open decorative folding fan in one hand and a small ornate handbag or pendant purse in the other, matching the approved direction. The design should feel intelligent, composed and graceful rather than combative, with a luxurious but readable silhouette suitable for model cards. Keep hair, fan, purse, hands and the key hanging ornaments fully visible and crop-safe. Premium Japanese anime game key-art quality, crisp clean linework, polished cel shading with soft painterly fabric highlights, cohesive product-illustration finish, full-body character, three-quarter front view, isolated character, transparent background. Do not include architecture, landscape, moon gate, room, text, logo panel, UI, card frame or webpage elements.
```

### Character-only Avoid Constraints

```text
No FGO / Fate servant design, no exact Alibaba / Qwen trademark logo, no text, no watermark, no scenic background, no UI, no combat weapon, no photorealism, no chibi proportions, no exaggerated sexualized anatomy, no extra limbs, no extra fingers, no missing fingers, no cropped fan, no cropped purse, no cropped tassels.
```

### Notes

- Family master only; future Qwen variants should retain the blue-white-gold hanfu-derived identity, the fan motif, and the articulate elegant temperament.
- Traditional Chinese motifs must remain tasteful and character-centered rather than becoming a scene illustration.

---
- AD-10.4 Review: `SELECTED` as Family Master design; no redesign required.
- Production Normalize: retain the exact selected identity and v1 prompt; prepare a 2:3 high-resolution transparent master (recommended 2048×3072 or equivalent) with >=5% alpha safety margin and preferably 6–8% for long outer contours.
- Card / Detail: share the same normalized master by focal-point / crop-safe metadata; do not create a separate half-body asset unless responsive validation proves it necessary.
- Production gate: remain `NOT PRODUCTION_READY` until normalize, alpha edge, crop, web delivery, rights note and browser loading checks pass.


## 10.9 Persona: Grok Family Master

- Model Family: `Grok`
- Target Model / Variant: `Family Master / later Grok variants`
- Persona ID: `persona_grok_master`
- Prompt Version: `v1`
- Status: `SELECTED`
- Production Readiness: `NOT PRODUCTION_READY`
- Generation Tool: `GPT-Image-2`
- Asset Role: `Model Card / Model Detail / Shared`
- Output Framing: `full body, crop-safe for half-body use`
- Background Requirement: `transparent / isolated character`
- Reference Index: `9`
- User Local Reference Root: `examples/private-reference-images`
- Reference Image Path: `examples/unpublished-art/reference-image-26.png`
- Generated Asset: `examples/unpublished-art/generated-asset-27.png`

### Character Prompt

```text
Original anime model-persona character for the Grok model family, closely following the approved reference character direction while being newly redrawn as an original production asset. Image A is the style and identity-direction reference. Create a young adult woman with a mischievous, sharp, confident and slightly rebellious presence; long voluminous blonde twin-tails, vivid bright blue eyes, a teasing grin with a small fang, and pointed elf-like ears. Preserve the approved dark-gothic identity with a playful edge. Outfit: luxurious black dress with deep red inner layers, subtle gold trim, chains, circular ornaments and a dramatic high collar, combining gothic nobility with stylish fantasy-tech accents. Include black ribbon bows, a small ornate crown-like headpiece and tasteful bat-wing motifs near the hair or shoulders. She wields an oversized ornate black-and-gold polearm / axe as a signature prop, matching the approved direction, while posing with lively self-assured body language. The design should read as dangerous, witty and charismatic rather than horrific. Keep the silhouette readable and crop-safe, with hair, weapon, hands and key ornaments fully visible. Premium Japanese anime game key-art quality, crisp expressive linework, polished cel shading with rich fabric and metal highlights, cohesive product-illustration finish, full-body character, three-quarter front view, isolated character, transparent background. Do not include scenery, smoke, text, logo panel, UI, card frame or webpage elements.
```

### Character-only Avoid Constraints

```text
No FGO / Fate servant design, no exact xAI or Grok trademark logo, no text, no watermark, no background scenery, no UI, no gore, no horror monster transformation, no photorealism, no chibi proportions, no exaggerated sexualized anatomy, no extra limbs, no extra fingers, no missing fingers, no cropped weapon, no cropped twin-tails, no cropped crown.
```

### Notes

- Family master only; future Grok variants should retain the blonde twin-tails, black-red-gold gothic styling, playful fang, and oversized polearm identity.
- The tone should remain mischievous and witty, not grimdark or purely villainous.
- AD-10.4 Review: `SELECTED` as Family Master design; no redesign required.
- Production Normalize: retain the exact selected identity and v1 prompt; prepare a 2:3 high-resolution transparent master (recommended 2048×3072 or equivalent) with >=5% alpha safety margin and preferably 6–8% for long outer contours.
- Card / Detail: share the same normalized master by focal-point / crop-safe metadata; do not create a separate half-body asset unless responsive validation proves it necessary.
- Production gate: remain `NOT PRODUCTION_READY` until normalize, alpha edge, crop, web delivery, rights note and browser loading checks pass.


# 11. 本轮生成记录

- `persona_gemini_master / v1`：AD-10.4 已审核并标记 `SELECTED`；保持 `NOT PRODUCTION_READY`，进入 Non-destructive Normalize / Alpha / Crop / Rights / Web Delivery Gate。
- `persona_gpt_master / v1`：AD-10.4 已审核并标记 `SELECTED`；保持 `NOT PRODUCTION_READY`，进入 Non-destructive Normalize / Alpha / Crop / Rights / Web Delivery Gate。
- `persona_claude_master / v1`：AD-10.4 已审核并标记 `SELECTED`；保持 `NOT PRODUCTION_READY`，进入 Non-destructive Normalize / Alpha / Crop / Rights / Web Delivery Gate。
- `persona_deepseek_master / v1`：AD-10.4 已审核并标记 `SELECTED`；保持 `NOT PRODUCTION_READY`，进入 Non-destructive Normalize / Alpha / Crop / Rights / Web Delivery Gate。
- `persona_glm_master / v1`：AD-10.4 已审核并标记 `SELECTED`；保持 `NOT PRODUCTION_READY`，进入 Non-destructive Normalize / Alpha / Crop / Rights / Web Delivery Gate。
- `persona_kimi_master / v1`：AD-10.4 已审核并标记 `SELECTED`；保持 `NOT PRODUCTION_READY`，进入 Non-destructive Normalize / Alpha / Crop / Rights / Web Delivery Gate。

- `persona_wenxin_master / v1`：AD-10.4 已审核并标记 `SELECTED`；保持 `NOT PRODUCTION_READY`，进入 Non-destructive Normalize / Alpha / Crop / Rights / Web Delivery Gate。
- `persona_qwen_master / v1`：AD-10.4 已审核并标记 `SELECTED`；保持 `NOT PRODUCTION_READY`，进入 Non-destructive Normalize / Alpha / Crop / Rights / Web Delivery Gate。
- `persona_grok_master / v1`：AD-10.4 已审核并标记 `SELECTED`；保持 `NOT PRODUCTION_READY`，进入 Non-destructive Normalize / Alpha / Crop / Rights / Web Delivery Gate。

本轮仍未生成页面场景或背景 Prompt。


---

# 12. WORKING v0.4 — 2026-09-02

补录第二批已生成 Model Persona 的人物 Prompt：

- DeepSeek Family Master v1；
- GLM / Z.AI Family Master v1；
- Kimi / Moonshot Family Master v1。

本次仅补写 Prompt 与生成记录，**没有重新生成图片**。继续遵守：只记录人物 Prompt，不记录场景 Prompt。


---

# 13. WORKING v0.5 — 2026-09-02

新增第三批已生成 Model Persona 的人物 Prompt：

- 文心一言 / ERNIE Family Master v1；
- 千问 / Qwen Family Master v1；
- Grok Family Master v1。

本次已同步记录：

- Reference Image Path；
- 实际 Character Prompt；
- Character-only Avoid Constraints；
- Generated Asset 路径；
- `GENERATED` 状态。

继续遵守：只记录人物 Prompt，不记录场景 Prompt。


---

# 14. WORKING v0.6 — 2026-09-02

AD-10.4 Persona Review A 已完成：

- Gemini、GPT、Claude、DeepSeek、GLM、Kimi、文心一言 / ERNIE、千问 / Qwen、Grok 九个 Family Master v1 全部由 `GENERATED / REVIEWED` 更新为 `SELECTED`；
- 本轮没有任何 Persona 进入 `REJECTED`；
- 当前九张 `1024×1536 RGBA` 图定义为 `Selected Design Master v1`，不直接标记 `PRODUCTION_READY`；
- 九个实际 Character Prompt 与 Character-only Avoid Constraints **原文保持不变**；
- 每项新增 `Production Readiness: NOT PRODUCTION_READY` 以及 Normalize / Alpha / Crop Notes；
- Normalize 推荐保持 2:3、提升至 `2048×3072` 或等价高分辨率，并建立最低约 5% Alpha Safety Margin，关键长轮廓优先 6–8%；
- Card / Detail 默认共享同一 Master，不默认生成独立半身图；
- Grok 武器 X-like 几何进入最终 Rights / Trademark Audit，但 Grok Persona 本身保持 `SELECTED`；
- 后续高分辨率重建如发生，必须作为新的 Production Variant / Version 追加，不覆盖 v1 Prompt 记录。

本文件继续只归档人物 Prompt 与人物生产元数据，不记录页面场景、Casino Background、UI 或整页构图 Prompt。
