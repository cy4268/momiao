# 构建安全基线（2026-10-22）

## Go 证书验证修复

本仓库要求 **Go 1.27.3**。对应 CVE-2026-111943 / GO-2026-6809：Go 1.27 分支的修复版本从 1.27.3 开始，1.26 分支从 1.26.10 开始。本项目选择 1.27.3，或后续受支持的已修复安全版本。

`go.mod` 是工具链要求的唯一入口；现有 CI 的 `actions/setup-go` 读取 `go-version-file: go.mod`，并使用 `GOTOOLCHAIN=local`。合并上游后保留该版本下限。升级机器上的 SDK 之后，仍须重新编译并发布，已运行的旧二进制不会自动获得修复。

前端构建使用 **Node 24.22.1**，README 与 CI 的 Node 固定版本保持一致；本次源码同步不触发无关的全量 CI。

现用生产主服务的业务基线为 main `f8ea90d`（保留 Claude 的非流式保活改进），edge 为 `4294ed4 + edge-errors-360`，poker 为 `35af2de`。三者已经分别用 Go 1.27.3 编译；不要用主服务当前源码替换另两个组件的独立业务基线。开发副本的旧镜像不据此记作已升级。

最小现有业务回归，仅在重建相关组件时按改动选择：

```sh
go version
GOTOOLCHAIN=local go test -p 2 ./internal/relay -run '^TestDelayedJSONResponse$' -count=1 -timeout=60s
```

部署后核对实际二进制 Go 版本、原业务修订、就绪状态及匿名认证边界。本次源码同步只新增说明与工具链要求，未重建主站或执行上述业务测试。

官方记录：[GO-2026-6809](https://github.com/golang/vulndb/blob/master/data/osv/GO-2026-6809.json)。

## 相邻服务的版本边界

- Antigravity Gateway：Go **1.27.3**，显式 `UPSTREAM_TIMEOUT_MS=330000`（330 秒）；三个部署共享同一源码，凭据和环境文件保持独立。
- Python aistudio-api：保留 2026-10-22 当日更新，部署参数 `AISTUDIO_TIMEOUT_REPLAY=330`、`AISTUDIO_TIMEOUT_STREAM=330`。
- Node AIStudioToAPI：当前 **v1.4.0 + empty-response 补丁 / Node 24.22.1**。它与 Python aistudio-api 是不同项目；旧 v1.3.7 补丁目录仅作历史记录，不作为现用重建基线。
- mail-hub：Node **22.23.4**，两个部署变体独立保留业务补丁和锁文件。
- Antigravity Manager：应用 **4.9.6**，`rustls-webpki` **0.103.16**；上游合并后检查实际 Cargo.lock，而不只检查 Rust 编译器版本。

这些基线仅对应本次公告。相邻服务的源码和构建说明各归所属项目；同步本仓库不替代它们的实际运行时验证。
