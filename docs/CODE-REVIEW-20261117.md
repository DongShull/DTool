# DTool 代码审查报告（2026-11-17）

审查方式：两个只读子代理并行（review-reuse：重复/复用/死代码；review-quality：质量/屎山/race/staticcheck/API/测试）。
验证基线：go vet 干净；go test -race ./internal/... 通过；staticcheck v0.8.2 完成。未修改任何代码。

## 一、真实风险（优先修，均已验证存在）

R1. MCP 链路异步任务包装漂移 — internal/mcp/tools_container.go:133-150 的 dt_update_container 同步执行、无占位进度、无 panic recover，与 HTTP 链路（internal/logic/container/updatelogic.go:45-97）不一致：MCP 侧任务崩溃会把 dtool 进程带崩；MCP 侧恢复任务排队时前端查进度 404。同模式复制于 tools_backup.go:127-148、restorelogic.go:34-110 等 5 处（约 200 行样板）。
   → 建议：svc 层提取 RunManagedTask(ctx, name, kind, fn)，统一 RegisterTask/占位进度/recover/NotifyTaskDone/AuditLog。
R2. composeproject.go:98-100 潜在 panic — name := strings.TrimPrefix(it.Names[0], "/") 在 len(it.Names)==0 检查之前执行。
R3. composeSafeToken（composeupdate.go:108）与 ComposeSafeToken（composeproject.go:30）疑似重复实现，应合并为一个。
R4. renamehandler.go:21-24 if/else 两分支都是 WriteJson（else 丢了 OkJsonCtx，行为碰巧等价，属复制走样）。

## 二、重复与复用（合计可收敛约 600-700 行）

D1. Logic 层 resp.Code/Msg/Data 手工三行 ×~36 处（resp.Code=400 ×21、=200 ×27、Data=map ×36）→ 加 types.OK(data)/types.Fail(code,err) 构造器，约省 150-200 行并集中治理状态码。
D2. HTTP handler 解析+双分支模板 ×~30 个文件（stophandler.go 与 starthandler.go 逐字节相同）→ 泛型 Handle[Req,Resp] helper，约 300 行减到 100。
D3. Compose 标签解析三处重复（composeupdate.go:57-66、composeproject.go:93-106、:182）→ 抽 ComposeLabelsOf()。修 R2 时一并做。
D4. ContainerList(All:true)+dockerTimeout+错误包装样板 ×6 处 → 抽 listAllContainers()。
D5. HTTP 与 MCP 双链路各自维护同一业务编排（审计字符串/错误文案/响应形状漂移，如 image.cleanup：HTTP 侧 207 部分失败语义，MCP 侧失败直接报错无 207）→ 审计与结果组装下沉 utiles（backup 链路已是正确示范）。
D6. 低优先：RemoveImage 纯转发（removeimage.go:9-17）；MCP mustString 逐参数 try-chain 可改 mustStrings 批量取。

## 三、质量评审（P0-P2）

无严格 P0。接近 P0 边界两项：
- P0-a 错误码语义混乱：types.Resp.Code HTTP 语义与业务码混用，HTTP 层一律 200，失败只看 body code。→ HTTP 状态码表达传输层、业务码用 ErrCode 常量枚举 + 中间件映射状态码。
- P0-b UpdateContainer 约 315 行（updatecontainer.go:35-337），自保护/分流/7 步/45 行内联 rollback 闭包混在一函。→ 拆 validateAndInspect / tryComposePath / containerRecreatePath(rollback)，rollback 提为独立方法。

P1：
- fmt.Errorf %w 仅 29/123（24%），大量字符串拼接破坏 errors.Is/As；取消/超时已手工 ctx.Err() 补偿（做法正确但可统一为 errors.Is(err, context.Canceled)）。
- dockerinfo.go 661 行 31 函数杂物抽屉 → 按 domain 拆 6 个文件。
- 魔法超时散落（updatecontainer.go:341,346 的 90s/2s；composeupdate.go:83,89 的 15s；updateprogram.go:90,199 的 60s/10min）→ 收进 dockercontext.go:9-12 同一常量块。
- Resp.Data interface{} + 空 map 兜底 → 泛型 Resp[T] 渐进迁移。
- 错误消息兼 UI 文案（中文错误串，ST1005 ×5）→ 业务错误码 + 文案放前端/progress 层。

P2：
- staticcheck：removelogic.go:31 S1017；config2yaml.go:127-128 SA1019 deprecated Config.MacAddress（应 EndpointSettings.MacAddress）；config2yaml.go:162 S1023；types.go 8 处 SA5008 为 go-zero optional tag 误报（staticcheck.conf 说明或 CI 忽略）。
- 测试传 nil ctx ×9 处（containerops_test.go:118 等）；建议签名不容忍 nil。
- os.Getenv("CLEANUP_OLD_IMAGE")/("DelOldContainer") 与 settings 体系两套配置并存 → 统一。
- REST 语义：GET /container/backup 触发副作用、GET /container/listBackups 应为 GET /container/backups、GET /container/backup2compose（routes.go:66-92）。

## 四、做得好的

- 并发设计有据可查：oplock.go:9-21 解释全局锁理由；updatecontainer.go:25 per-ID 锁；镜像拉取在取锁前（:207-209）；go test -race 通过。
- 回滚测试是真实的：TestUpdateContainerRollbackOnStartFailure（updatecontainer_test.go:141-177）用 fake Docker server 验证回滚每一步 HTTP 调用；TestUpdateContainerRejectsSelf 覆盖自护栏。
- goroutine 任务均有 recover + FinishTask defer。
- 无死代码导出函数（抽查 GetRemoteVersion/EffectiveBackupDir/NotifyTaskDone/ComposeUpdateStrategy/CheckBackupDirSafe 均有真实调用点）。

## 总评分：7.5 / 10

总评：明显高于平均水平的 go-zero 项目——锁设计有理由文档、回滚/自护栏有真实测试、vet/-race 干净。短板在结构：UpdateContainer 过长、dockerinfo.go 职责堆积、错误 wrap 与错误码体系不规范、GET 副作用。无阻塞性缺陷，按 R1/R2 + P0 清单做一轮重构即可进入优秀行列。

## 建议执行顺序

1. R1 RunManagedTask 统一异步任务包装（唯一有运行时崩溃风险的项）
2. R2+D3 compose 标签解析统一 + panic 修复
3. P0-a 错误码枚举 + D1 types.OK/Fail 构造器（一次 PR 同时做）
4. P0-b 拆 UpdateContainer
5. D2 泛型 handler helper（可选，量大机械）
6. P1 %w 包装与超时常量、P2 按需
