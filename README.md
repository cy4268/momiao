# momiao

一个按可验证的小阶段推进的平台项目。当前提交是 **Phase 3 / IS-01 的最小后端基础**，不是完整平台、完整 IS-01 验收或生产发布。

## 已实现 / 待实现

| 已实现 | 尚未实现 |
|---|---|
| 单个 Go 标准库 HTTP 进程；无第三方运行时依赖 | 登录、Session、OAuth 与 NewAPI Adapter |
| `GET /healthz`、`HEAD /healthz` 进程存活探针 | 数据库、资产账本、奖励、游戏与 Poker |
| loopback 默认、配置校验、HTTP 超时与有期限的优雅退出 | 前端、正式部署、TLS、迁移与生产监控 |
| 可重复运行的测试、仅真实端点的 OpenAPI、测试/构建 CI | 全部来源核验、并发与故障验收 |

历史页面预览、设计图或原生 NewAPI 页面都不是此后端的已实现功能。`/healthz` 成功仅表示进程可以处理该请求；它不检查数据库、上游或业务就绪状态。其他路径为 `404`，该路径的其他方法为 `405`；没有占位登录接口。

## 本地运行

先安装 [Go 官方发行版](https://go.dev/dl/)。本基础使用并核验了 **Go 1.27.1**；`go.mod` 声明版本，CI 按该文件安装。工具版本更新先核对官方实际可用版本，再更新并重跑测试。当前没有前端，因而不要求 Node、npm、Make、Docker 或数据库。

以下命令在仓库根目录运行，Windows PowerShell 与常见 POSIX shell 均适用：

```text
go version
go vet ./...
go test -count=1 -timeout=30s ./...
go build -trimpath ./cmd/momiao
```

Windows PowerShell：

```powershell
.\momiao.exe
# 另一个终端执行：
Invoke-RestMethod http://127.0.0.1:8080/healthz
```

Linux / macOS：

```sh
./momiao
# 另一个终端执行：
curl --fail http://127.0.0.1:8080/healthz
```

响应为 `{"status":"ok"}`。终端按 `Ctrl+C` 退出；Unix 服务管理器也可发送 `SIGTERM`。进程停止接收新连接并等待活动请求，超出退出期限后关闭连接并以非零状态退出。强制终止进程不等于优雅退出。

### 配置

| 环境变量 | 默认 | 校验 |
|---|---|---|
| `MOMIAO_LISTEN_ADDR` | `127.0.0.1:8080` | 必须显式指定数字 IP 与 `1..65535` 端口；IPv6 加方括号 |
| `MOMIAO_SHUTDOWN_TIMEOUT` | `10s` | Go duration，范围 `1s..30s` |

设置为空与缺省不同：空值、非法 IP、缺失主机/端口或越界退出时间会在监听前失败；错误只标识配置项，不回显配置原值。没有 DNS 主机名解析，也不会默认绑定所有网卡。

PowerShell 示例：

```powershell
$env:MOMIAO_LISTEN_ADDR = '127.0.0.1:8090'
$env:MOMIAO_SHUTDOWN_TIMEOUT = '15s'
.\momiao.exe
```

POSIX 示例：

```sh
MOMIAO_LISTEN_ADDR=127.0.0.1:8090 MOMIAO_SHUTDOWN_TIMEOUT=15s ./momiao
```

显式设为 `0.0.0.0` 或其他非 loopback 地址会扩大可达范围。本程序只有明文 HTTP 存活探针，不内置 TLS 或访问控制；本地开发保留 loopback，正式对外入口需要另行设计与验证。

固定 HTTP 上限：读请求头 `5s`、读请求 `10s`、写响应 `10s`、空闲连接 `60s`，最大请求头 `16 KiB`。这些是当前小服务的工程默认值，不是容量或生产调优结论。

## 项目结构与下一阶段

- `cmd/momiao/`：入口、配置、HTTP 与退出流程、真实行为测试。
- [OpenAPI](contracts/openapi.json)：只描述 `GET/HEAD /healthz`。
- [决策 0001](docs/decisions/0001-pragmatic-baseline.md)：当前实现范围及对历史流程的具体简化。
- [迁移基线](docs/migration-baseline.md)：保留项、迁移规则与证据缺口，不含生产数据或执行脚本。
- [六份设计文档归档](docs/design/archive/README.md)：历史需求、IA、视觉与技术参考，不等于实现状态。

下一阶段选择一个有实际使用价值的纵向功能；在其依赖入口完成来源与行为核验后再实现。先做无关基础不必等待所有 SV；涉及身份、凭据、资产或迁移的真实验证仍然保留。奖励数值与视觉意图目前不作调整。

## 验证与贡献

提交前运行上面的 `go vet`、`go test`、`go build`，用 `gofmt -w cmd` 格式化 Go 文件。支持 race detector 的环境另运行 `go test -race -count=1 -timeout=30s ./...`。CI 在 Linux 与 Windows 测试、构建，在 Linux 增加 race 检查；它没有部署步骤或项目 Secret 访问。已配置 CI 不代表远端 CI 已运行通过。

保持改动小而完整：实现、边界测试与契约一起更新；新依赖要有当前需求。不要提交 `.env`、凭据、数据库、原始核验日志、私有目录、工具二进制或第三方角色图片。`.gitignore` 只是第一层防护，发布前仍应审阅实际文件与 Git diff。

## 许可证与外部来源

本仓库自有代码与文档使用 **AGPL-3.0-only**，见 [LICENSE](LICENSE)。[ATTRIBUTION.md](ATTRIBUTION.md) 说明外部 NewAPI 参考来源和第三方角色、商标及素材边界；该许可证不授予第三方角色或美术资产权利。仓库不包含 NewAPI 源码副本。
