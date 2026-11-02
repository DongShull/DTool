# DTool 改造规划（ROADMAP）

> 本项目自 v2.2.0 起由 DongShull 独立维护（fork 自 onlyLTY/dockerCopilot v2.1.3，上游已停止更新）。
> 本文件记录 v2.1.3 基线体检结论与改造路线。

## 一、基线缺陷清单（v2.1.3）

### P0 · 崩溃/数据级
1. `containerslistlogic.go`：inspect 失败后仍解引用 nil，容器列表接口可被打崩
2. `updatecontainer.go`：更新失败（create/start）时旧容器停留在改名+停止状态，无回滚
3. `updatecontainer.go`：更新成功后不清理旧镜像，磁盘慢性泄漏

### P1 · 功能错误
4. 旧容器改名使用秒级时间戳，同秒二次更新必撞名
5. `checkupdate.go`：digest 比对循环覆盖，"最后一个说了算"，多 RepoDigest 时误报/漏报
6. `image.go`：`strings.Split(":")[1]` 解析 tag，带端口的私有仓库镜像名/tag 错位
7. 停止容器使用 SIGINT，业界惯例 SIGTERM
8. `getRemoteVersion.go`/`updateprogram.go` 硬编码上游 onlyLTY URL（已改可配置 UPDATE_REPO）
9. `decompressTarGz`：tar 条目名不校验，存在 zip-slip 路径穿越
10. `module.ImageUpdateData`：cron 写、HTTP 读共用 map，无锁数据竞争

### P2 · 卫生级
11. ProgressStore 只写不删（内存泄漏），重启丢进度，无任务取消
12. `checkImageInUsed` O(容器×镜像) 双重循环
13. restore 中永假的死代码分支
14. JWT 84 天有效、secretKey 兼作签名密钥（记录在案，暂无计划改动）
15. `Timeout: 1e14`、CORS `*`、`InsecureSkipVerify`（已加 REGISTRY_INSECURE 开关；Timeout 已改为 600000ms，SSE 路由带 `WithSSE()` 走 Accept 头豁免，见 `etc/DTool.yaml` 注释）

## 二、分期计划

| 期 | 内容 | 状态 |
|---|---|---|
| P0 | 基线整备：URL 可配置、CI（build/vet/test）、本规划入仓 | ✅ v2.2.0 |
| P1 | 修复批：缺陷 1/4/5/6/7/9/10/12/13 | ✅ v2.2.0 |
| P2 | 更新链重构：旧镜像清理、失败回滚、digest 并发检测、compose 标签感知 | ✅ v2.2.0 |
| P3 | 任务系统：进度落盘、TTL 清理、任务列表/取消 API、SSE 推送 | ✅ v2.2.0 |
| P4 | FnOS 适配：去 privileged 要求、HEALTHCHECK、备份目录运行时可配（Settings API） | ✅ v2.2.0 |
| P5 | 增强：webhook 通知、审计日志、定时自动更新白名单 | ✅ v2.2.0 |
| P6 | 稳定性：SSE 心跳、Docker 调用超时、进度文件并发落盘、Compose 导出保真度 | ✅ 待发版 |
| P7 | Agent 化：移除网页前端、保留 HTTP API、内置 MCP（Streamable HTTP）暴露全部能力 | ✅ 待发版 |

### P6 补充说明

- SSE 增加 15 秒心跳和断线重连提示，避免反向代理在任务长时间无进度时断开连接。
- 单次 Docker list/inspect/start/stop/rename/remove 调用增加 60 秒~2 分钟超时；镜像拉取仍由任务上下文控制，避免大镜像被误杀。
- 进度文件落盘与内存快照在同一把锁内完成，修复并发保存时旧快照覆盖新状态的问题。
- Compose 导出补齐 healthcheck、devices、cap_add/cap_drop、security_opt、ulimits、extra_hosts、dns、tmpfs、shm_size、user、group_add、logging、stop_signal、stop_grace_period、sysctls、资源限制等字段；命名卷改用 MountPoint.Name，避免导出宿主机卷路径。
- 恢复 `force=true` 改为“停止并临时改名旧容器 → 创建/启动新容器 → 成功后删除旧容器”；任一步失败都会清理半成品并把旧容器改回原名、恢复运行/暂停状态。
- 备份/更新/恢复统一清洗网络端点：只保留别名、链接、驱动参数和静态 IPAM，清掉运行期 NetworkID/EndpointID/IP/MAC/DNSNames，避免重建时继承失效端点或地址冲突。

### P7 补充说明

- 前端（`go:embed front/*`、`/manager` 静态路由、CI 的前端构建 job）整体移除，`GET /` 改为返回 JSON 状态，同时满足容器 HEALTHCHECK 的 200 要求。
- 保留全部 `/api/*` 路由：脚本、CI 与旧客户端不受影响，MCP 与 API 共用同一套 `internal/utiles` / `internal/logic` 实现。
- 新增 `internal/mcp/`：基于 `mark3labs/mcp-go` 的 Streamable HTTP 服务，挂载 `/mcp`，注册 26 个 `dt_*` 工具，覆盖容器、镜像、备份恢复、进度、设置与自更新。
- `/mcp` 默认鉴权：`MCP_TOKEN` 优先，未设置时回退 `secretKey`；支持 `Authorization: Bearer`、`X-MCP-Token`、`?token=` 三种携带方式，比较使用常量时间算法。`MCP_TOKEN=off` 可显式关闭（仅隔离内网）。
- 破坏性 MCP 操作复用现有审计与通知；长任务复用 `RegisterTask`/`CancelTask`/`FinishTask` 与进度持久化。

## 三、配置项一览（环境变量）

| 变量 | 默认 | 说明 |
|---|---|---|
| `secretKey` | （必填） | 登录密钥，同时用于 JWT 签名，不少于 8 位且非纯数字 |
| `DOCKER_HOST` | unix:///var/run/docker.sock | Docker 守护进程地址 |
| `BACKUP_DIR` | /data/backups | 容器备份目录（优先级：env > Settings API 配置 > 默认） |
| `DelOldContainer` | true | 更新成功后是否删除旧容器（"false" 保留） |
| `CLEANUP_OLD_IMAGE` | true | 更新成功后是否清理不再被引用的旧镜像 |
| `UPDATE_REPO` | DongShull/DTool | 版本检查与自更新仓库 |
| `UPDATE_BRANCH` | latest | 版本检查分支 |
| `githubProxy` | 空 | GitHub 代理前缀 |
| `REGISTRY_INSECURE` | false | 设为 `true` 时 registry 检测跳过 TLS 校验（默认严格校验） |
| `NOTIFY_WEBHOOK_URL` | 空 | 任务完成通知 webhook（POST JSON） |
| `AUTOUPDATE_CRON` | 空 | 自动更新周期（cron 表达式，空=关闭） |
| `AUTOUPDATE_WHITELIST` | 空 | 自动更新容器白名单（逗号分隔，`*`=全部有更新的容器） |
| `MCP_TOKEN` | 空 | `/mcp` 访问令牌；空则回退 `secretKey`，`off` 显式关闭鉴权 |

## 四、验收标准（v2.2.0）

- `go build ./...` / `go vet ./...` / `go test ./...` 全绿
- 更新一个容器后 `docker images` 不新增悬空镜像
- 模拟更新失败（拉取超时）后旧容器自动回到原名并恢复运行
- 重启进程后任务历史仍可查询；进行中任务可取消
- 50 个镜像的更新检测从串行分钟级进入 10 秒级
- 非 privileged 容器（仅挂载 docker.sock + 数据卷）可跑通全部功能
- `go test ./internal/mcp/` 覆盖工具注册、鉴权与 Streamable HTTP 握手
- MCP 客户端可完成「列容器 → 更新 → 查进度」全流程，`/api/*` 行为不变

## 五、P8 · 安全与契约修复（进行中，代码已入仓）

依据 2026-11-02 的后端审计（静态通读 + 对运行实例只读实测），先止血再补能力。

### 第 0 批（P0 止血，已实现）

| # | 问题 | 修复 |
|---|---|---|
| 1 | 恢复/更新/自动更新会作用于 DTool 自身容器，进程随容器死在半路、回滚代码执行不到 | 新增 `internal/utiles/selfid.go`：从 `/proc/self/mountinfo`、`/proc/self/cgroup`、`HOSTNAME` 推断自身容器 ID；`UpdateContainer` 拒绝自身、`RunAutoUpdate` 跳过自身（改按容器 ID，不再依赖镜像名含 `dtool`，旧判断在镜像叫 `dockercopilot:*` 时失效）、`Restore` 把自身容器名计入 `skipped` 并拒绝 nameMap 重定向到自身 |
| 2 | `POST /api/auth` 只认 form/query，JSON body 登录恒失败并误报"无效的secretKey" | `LoginReq` 增加 `json:"secretKey,optional"`，form/JSON 双通 |
| 3 | 登录接口无限流，CORS 全域放开时可在浏览器侧爆破 | `/api/auth` 进程内限流：同 IP 连续 5 次失败锁定 15 秒，命中返回 429 |
| 4 | 裸 `GET /api/version` 返回 `code:50000 field "type" is not set` | `VersionReq.Type` 改可选，缺省按 `local` 处理 |
| 5 | `MCP_TOKEN=off` 在任何监听地址上都关闭 `/mcp` 鉴权 | `off` 仅在回环监听（127.0.0.1/localhost/::1）时允许，否则拒绝启动（start.sh 失败计数会自动回退上一版） |
| 6 | 备份文件名秒级 + 备份无互斥，同秒并发备份互相覆盖 | 文件名加纳秒；`BackupContainer` 全流程加 `backupMu` 串行化 |

### 审计误报更正

- **`GET /api/progress/export` 并未被 `/progress/:taskid` 抢路由**：go-zero 的 patrouter 按注册顺序取首个匹配，导出路由本就注册在参数路由之前。实测返回"taskID 未找到"是因为线上镜像构建于 10-31，而导出路由是 11-01 的提交 —— 属**部署落后于代码**。本轮补充了回归测试 `TestProgressExportRouteNotShadowed` 守住该行为。

### 待办（第 1~4 批）

- 契约治理：统一 `code`（`/api/containers` 现返回 0，其余 200）、错误结构带真实 HTTP 状态码、`dtool.api` 导出 OpenAPI
- SSE 鉴权：`EventSource` 无法带 `Authorization`，需支持短时 ticket / `?token=`
- 只读控制面：logs、stats、events(SSE 桥接)、volumes list、networks list、容器 inspect 详情
- 写能力：volume prune、network create/connect、image prune/tag、pause/unpause、exec
- 恢复链路：保留镜像 digest 与 platform、默认恢复比对完整配置而非仅镜像名
- 备份对比：补齐 privileged/cap/security_opt/healthcheck/资源限制/`HostConfig.Mounts`/端口 `HostIP`

## 六、P9 · 第二轮审计修复（2026-11-02）

第二轮审计（`go test -race` 全绿、`staticcheck 2026.2.2` 无实质问题、handler/logic 薄层逐文件通读、自更新链全文、真实容器实测）发现 19 项，已全部修复。

### P0

| 问题 | 修复 |
|---|---|
| **版本链断裂**：`UPDATE_BRANCH` 默认 `latest`，但仓库默认分支是 `main` 且远程已无 `latest`。实测 `HEAD` 代码 `GET /api/version?type=remote` → `code:500 404 Not Found`；7 个发布 workflow 也全锚 `latest`，等于全部不触发 | 默认分支改 `main`；7 个 workflow 补齐 `main`；404 报错文案带上 `UPDATE_REPO`/`UPDATE_BRANCH` |
| **`PUT /api/program` 静默降级**：handler `_ = httpx.Parse` 吞掉解析错误，`Target` 无 json tag。JSON 客户端请求"更新到 vX"实际执行"更新到最新版 + 退出进程" | 解析失败返回 400；补 json tag；退出码改 0（避免被 start.sh 记成启动失败而误回退）；MCP 通道同样调度退出 |

### P1

- **可远程删配置**：`backupDir` 可设为任意路径 + 删除接口只校验扩展名 → 能删 `progress.json`/`settings.json`。现在只允许 `backup-*`，目录白名单拒绝配置/日志/程序目录，删除写审计。
- **compose 备份未纳入上一轮修复**：仍是秒级文件名 + 无锁；补齐纳秒 + 共用 `backupMu`。
- **MCP 取消语义弱于 HTTP**：`CancelTask` → `CancelActiveTask`。
- **任务提交后无进度记录**：查进度/SSE/取消都会 `taskID 未找到`；提交即写"任务已排队"占位。
- **更新/恢复无跨操作锁**：新增 `LockContainerMutation` 全局锁，覆盖更新、恢复、改名、启停、重启。
- **部分失败报成功**：备份与镜像清理改用 207。
- **审计可绕过**：HTTP 通道的备份/导出 compose/对比/删除/图标上传补齐审计。
- **UGREEN 专用版无护栏**：`ManagedUpdatePlatform` 同时拦 FnOS 与 UGREEN（大小写不敏感）。

### P2

- `start.sh`：`137`(OOM) 不再算稳定；`DTool.bak` 改临时文件 + 大小校验 + 原子 mv。
- SSE 未知 taskID 在写 200 之前返回 404（消除无限重连风暴）。
- 更新历史：唯一 tmp 名、损坏时保留 `.corrupt-*` 现场、`RemoveLastUpdateHistory` 按"刚追加的键"精确删除、`SliceStable` 排序。
- 审计：所有错误路径改为记日志，不再静默。
- webhook：校验 scheme、检查响应码、失败重试一次并记日志。
- 自更新：换入路径改用二进制所在目录；解压增加条目数/单文件/累计大小上限；禁止 https→http 降级重定向；`SanitizeUpdateTarget` 拒绝纯点串。
- `/api/icons` 返回类型统一为 map，上传补审计。
- 更新任务的 `containerName` trim + 空值用 ID 反查真实名字。

### 验收

- `go build ./...` / `go vet ./...` / `go test ./...` / `go test -race ./...` 全绿
- 容器内实测：版本检查 200（v2.2.0）、`backupDir=/data/config` 与 `/` 被拒、删除 `progress.json` 被拒、SSE 未知任务 404、备份文件名带纳秒、审计出现 backup.create/compose/delete、更新自身被拒、JSON `target` 正确解析（`..` 被拒）、非法 JSON 返回 400、任务占位进度可查
