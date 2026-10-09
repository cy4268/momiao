# 广喜娘：首页 Live2D

## 操作与边界

- 仅首页 `/` 挂载；不读取账户或业务 API，不改变登录流程。默认左下角 16 CSS px，完整角色画布 200×300，工具栏 44 px。
- 拖动手柄移动；方向键每次 10 px，Shift 加方向键 40 px。复位回左下角；减号收起，“喜”展开。仅控件接收指针，角色和透明层允许点击穿透。
- 使用可见视口、安全区和完整组件边界夹紧位置；短屏按比例缩小。窄屏首次收起，已有偏好优先。
- 仅使用 `localStorage['guangxi-mascot:v1']`：`{version:1,collapsed:boolean,position:null|{left:number,bottom:number}}`。位置用 CSS px；存储损坏/抛错不影响首页。
- 页面 load 后在空闲时加载模型；收起、后台、减少动态效果时不启动新模型。现有模型暂停，恢复时不追赶离开期间的时间。减少动态效果显示静态预览。加载超过 15 秒、WebGL/资源失败保留静态预览及控件，不无限重试。
- Canvas 目标 30 fps，DPR 上限 2。Idle 只写头身和呼吸；本地 3–6 秒眨眼时序只写右眼，0.08 秒闭合/0.04 秒保持/0.12 秒睁开；物理只写头发和蝴蝶结。无相机、麦克风、语音或追视。

## 固定运行库与资产

采用用户提供的 `CubismSdkForWeb-5-r.5.zip`，SHA-256 `67064a7fb1812cf502f5c4a03bfe12cc638c75a621bb4acf06bb28763df06ba0`。配套 Core 实际报告 **6.0.1**。Framework 与公开 5-r.5 标签提交 `198a3769c26ca3d7b600e932590433badd392edd` 的 76 个源文件/Shader/构建配置/许可在换行归一化后完全一致。

Framework 使用 SDK 自带锁文件独立编译，未引入网站依赖、未降低 TypeScript strict。完整拷贝清单见 `web/vendor/cubism-r5/manifest.json`；原始许可在 `web/public/vendor/cubism-r5/`。SDK 与 renderer 在动态块中，首屏主块不执行它们。R5 枚举在导入时读取 Core，因此先载入 Core 再导入 Framework。

R5 异步 Shader loader 没有 HTTP 状态检查/AbortSignal。适配器使用可取消的同源请求填充公开的固定版本源字段，再调用原版编译器；只校验真实使用的程序索引，不把 Normal/Over 的三个保留槽误判为失败。首帧实际完成后才将状态设为 ready。

真实 WebGL 采样发现此细微、纯角度输入的物理组被 R5 的位移归零阈值吞掉。适配器仅在内存中将未用于平移输入的 Position normalization 设为 ±0.1，将阈值从 0.01 降至 0.0001；Angle normalization、输出倍率及源文件不变。原生模型的六个文件保持字节一致。

| 原生运行文件 | SHA-256 |
|---|---|
| guangxi-idle.model3.json | c74564511c9bd4869e8cacc5b2584792fd73cc1ec59df9212a22f064e839faaf |
| guangxi-idle.moc3 | c91ec26c05b9ac23a99e5507948773d1bb50ad3a80a21ff978915330926914e8 |
| guangxi-idle.cdi3.json | 44913a1709e9357baae4acbc690cef9f463befe099a55e5f6ca7037e8abb5900 |
| guangxi-idle.physics3.json | 806944bfde6a9dfb94629d0df38c1f8d05acf3acf7e3650e21ccdab7cf3e5bca |
| guangxi-idle.2048/texture_00.png | d0e3aa693c5cc7c3bf19c6d408a0df554f4cc63cb88a16e61d5b9ee5030d6db0 |
| motions/idle.motion3.json | 514b6149b6723bb697978d9fba06e7973749fd304f7772612fbbcd681c29aef6 |

同级 `preview.png` 是交付 v2 图原字节副本：`b1c18297238cbca2ff56c751e6cbfc61a6899297d9d352fc572decaa40b1541a`。

## 验证入口

工作目录为 `web`。仅在已有 `Home.test.tsx` 新增一个综合回归，不建立新测试框架：

```sh
npm test -- src/Home.test.tsx -t 'keeps guangxi movable, persistent and non-blocking across lifecycle changes'
npm test -- src/Home.test.tsx src/App.test.tsx
npm run build
```

实际执行日志、浏览器截图、临时诊断及副本回退证据保留在忽略目录 `evidence/guangxi-mascot-20261009/`；最终摘要见同目录 `VERIFICATION.txt`。JSDOM 只模拟 WebGL 适配边界，其通过结果不作为真实模型绘制证据。

`ROLLBACK.sh` 读取同目录基线和哈希清单，只恢复这次修改的原文件并移除这次新增的精确文件。执行前校验目标路径与当前文件哈希；存在后续修改时停止，不覆盖它们。副本演练不更改正式工作树。

## 浏览器实测（2026-10-09）

- Chromium 实际 WebGL：同一实例持续采样 112.33 秒、3335 次真实绘制，约 29.7 fps；眼睛实际达到 0/1，头身、呼吸与四路物理均产生非零变化。临时 console 采样已从交付源码移除；原始日志保留。
- 收起 100.18 秒期间没有新增采样；恢复后动画时钟不计入该停画时间。1440×900 下实际拖动、两角覆盖四边夹紧、方向键 10/40 px、复位、刷新记忆通过。
- 390×844、320×480 桌面视口下，画布与全部 44×44 控件留在边界内；新来源的窄屏首次收起，Core script 数为 0。使用浏览器桌面鼠标，不冒称真机触屏。
- 对真实生产构建分别返回 Core、moc3、纹理、Shader 的 404：四例均进入 failed 静态状态，首页标题、登录链接与控件保持可用。5 秒 Core 延迟下展开即收起，没有晚到模型；随后再展开完成真实绘制。
- 真实点击画布与登录链接的重叠区域，命中 `/sign-in`；离开首页后看板娘数量为 0。加载中再次离开也不附着旧实例。本地验证服务器未配置真实登录后端，认证本身不属于此验收。
- 损坏/禁用存储、pointercancel、隐藏/减少动态效果、迟到实例 dispose、15 秒截止由唯一 JSDOM 回归覆盖。当前浏览器接口未执行：真机触屏取消、安全区刘海、DPR 2 切换、系统减少动态效果切换、强制 WebGL 禁用/上下文丢失及 GPU 内存分析。这些不是已通过的浏览器结论。

原锁文件 `npm ci`/audit 报告既存 `source-map-js` 高等级公告，未新增依赖或顺手更新锁文件。生产 build 仍报告全站主块超过 500 kB；本功能的 Core/Framework 独立延迟加载，并未为此次功能拆改无关路由。
