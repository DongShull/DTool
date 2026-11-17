# DTool 改动报告（2026-11-18）

本报告覆盖自上一提交（5d2e9d6 只读控制面）之后的全部改动：三轮代码审查、全部修复、统一响应契约、完整文档与发布部署。

## 一、新增功能

### Compose 工程控制（第一轮需求）
- GET /api/compose/projects：列出全部 compose 工程（名称/workdir/服务数/运行状态）
- POST /api/compose/:project/{up,down}：工程级启停；拒绝作用于 DTool 自身所在工程（SelfComposeProject）；project/service 参数经 ComposeSafeToken 注入校验；执行超时 5 分钟；写审计（action=compose.up/compose.down）
- GET /api/compose/:project/logs：按服务分块聚合的工程日志
- MCP 新增 4 个对应工具：dt_list_compose_projects / dt_compose_up / dt_compose_down / dt_compose_logs（工具总数 37 → 41）

### 容器运维强化
- utiles.ContainerLogsRange：日志区间查询；HTTP logs 新增 until 参数（since/until 按 Docker daemon 主机时区解释，见 docs/API.md）
- dt_update_container 容器名可选 + 支持指定镜像 tag
- start/stop 幂等化（EnsureContainerStarted/EnsureContainerStopped）
- 自身保护强化：容器名传入也会被识别并拒绝（IsSelfTarget：ID 匹配 → Inspect 解析真实 ID → 容器名兜底，宁误拒不放行）

### 统一响应契约（前端零硬编码）
- 7 个业务码常量：200 成功 / 207 部分成功 / 400 参数错误 / 401 未授权 / 403 禁止 / 409 冲突 / 500 服务错误（meta 码表另含 429 登录限流）
- 构造器 types.OK(data, msg...) / types.Fail(code, msg, data...)，全量 logic 收敛，消灭手工三行赋值
- 新增 GET /api/meta（免鉴权）：返回版本 + 全部业务码的中文含义与默认文案，前端登录前即可获取错误文案，零硬编码
- 参数解析错误统一为 HTTP 400 + body {"code":400,"msg":<原因>,"data":{}}（原为 HTTP 200 + code:50000）
- handlerutil.ParseAndRun[REQ] / RunNoReq 泛型 helper：17 个模板 handler 收敛；9 个特殊语义 handler 保留并注释原因（SSE 流、下载头、207 部分失败、防降级文案等）

### 文档
- docs/API.md：完整接口文档（42 条目 = 41 路由 + meta），鉴权流程（POST /api/auth 返回字段名 jwt）、参数表、JSON 示例、SSE/异步任务特例、路由数对账
- docs/API-RESPONSE.md：响应信封规范与业务码枚举表
- docs/CODE-REVIEW-20261117.md：第二轮审查报告；docs/SMOKE-TEST-REPORT-20261117.md：上线冒烟报告（已脱敏）

## 二、代码质量修复（三轮审查）

### 结构与重复
- RunManagedTask（internal/utiles/taskwrapper.go）：统一异步任务包装（RegisterTask、占位进度、panic recover、通知、审计、FinishTask）；更新/恢复链路异步返回 taskID，MCP 同步等待返回最终进度 JSON，对外契约不变
- UpdateContainer 315 行拆分为 validateAndInspect / tryComposePath / containerRecreatePath + state struct 回滚方法
- dockerinfo.go 661 行拆为 6 个纯移动文件；超时常量统一收进 dockercontext.go
- 标签解析统一 ComposeLabelsOf；compose 容器名解析先判空（修复 Names[0] 越界 panic）
- renamehandler 复制走样修复；backup2compose/restore 两个漏网 handler 信封统一

### 正确性（第三轮发布前审查修复）
1.【高】自身保护可被容器名绕过（更新链可能误删自身容器）→ IsSelfTarget 三级识别
2.【高】/api/meta 需 JWT 但文档承诺免鉴权（前端登录前渲染依赖它）→ 移出 JWT 组
3.【高】参数解析错误 HTTP 200 + code:50000，文档码表无此码 → 统一 HTTP 400 + code:400
4.【高】Restore 全程持全局变更锁（镜像拉取期间全站启停/更新被阻塞）→ 恢复任务串行化独立锁 + 每条目粒度变更锁，ImagePull 移出锁外
5.【高】自更新并发触发写假失败历史 + 假 webhook 告警 → TryLock 前置，重入不再记录
6.【中】compose 多配置文件工程从不传 -f，用错配置重建 → 按标签追加 -f 并校验路径
7.【中】compose pull+up 持全局锁无超时 → 5 分钟超时兜底
8.【中】compose 更新失败静默降级为容器重建（脱离 compose 管控）→ 明确报错（含 stderr 摘要）；无 compose CLI 环境仍优雅兜底
9.【中】docker compose version 探测无超时 → 5s CommandContext
10.【中】UpdateContainer 缺 nil client 前置检查 → requireDocker 明确报错
11.【低】GetDigest 每次新建 Transport 空闲连接积压 → 包级复用 http.Client
12.【低】拉镜像进度硬编码回跳 → 基准百分比参数化

### 静态检查
- staticcheck：S1017 / SA1019（MacAddress→NetworkSettings）/ S1023 / SA4006 清零
- go vet、go build 全绿；go test -race ./... 14 包全 ok，新增回归测试 30+（自身保护三分支、compose -f、假告警跳过、锁窗口、信封构造、meta 路由等）

## 三、契约行为变化（升级注意）

| 场景 | 改前 | 改后 |
|---|---|---|
| GET /api/meta | 401（需 JWT） | 200 免鉴权 |
| 参数缺失/格式错误 | HTTP 200 + code:50000（无 data） | HTTP 400 + code:400（data:{}） |
| rename 容器名为空 | HTTP 400 | HTTP 200 + body.code 400（与其他接口一致，以 body.code 为准） |
| backup2compose/restore 失败 | code:50000 | code:400/500（原因文案不变） |
| meta 401 文案 | 无效的secretKey | 未授权（登录接口 401 仍为「无效的secretKey」，文档已区分） |
| compose 管理容器更新失败 | 静默回退容器重建 | 返回明确错误 |

## 四、依赖与仓库清洁

- go mod tidy + go mod verify：go.mod/go.sum 零变化，无多余依赖、无未使用依赖
- .gitignore 补充本地构建缓存目录（.home/ .gocache/ .gohome/）
- 提交前敏感信息扫描：secretKey/JWT 密钥/密码/token 实值、内外网 IP、服务器路径均未出现在待提交内容；冒烟报告中的生产 compose 工程名与服务名已脱敏
- 生产部署：/opt/dtool/DTool md5 323250e5c41bd11b5aaa686a94462b41，线上 9 项冒烟全部通过
- 回滚备份：/opt/dtool/DTool.bak-20261117-152851 及更早

## 五、遗留建议（不影响发布）

- 日志 since/until 客户端时区换算可考虑在 API 层做（当前按 daemon 主机时区解释，文档已注明）
- 并发更新任务可加全局 semaphore 限制资源峰值（当前仅 per-ID 互斥）
- progress SSE 心跳注释（: keep-alive）与 events SSE（: heartbeat）可统一
- DOCKER_TLS_VERIFY / DOCKER_CERT_PATH 可补进 README 环境变量表
