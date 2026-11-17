# DTool 统一响应规范（给前端）

> 后端唯一响应信封契约。前端**不得硬编码**任何文案、码值含义或枚举：一切展示信息以 `/api/meta` 与各接口返回的 `msg` / `data` 为准。

## 1. 响应信封结构

所有 HTTP 接口（含错误）的响应体都是同一个 JSON 信封：

```json
{
  "code": 200,          // int 业务码，见下方枚举表
  "msg":  "success",    // string 提示文案，错误时可直接展示给用户
  "data": {}            // T 业务数据；成功无数据时为 null 或 {}，错误时通常为 {}
}
```

构造入口（后端）：`types.OK(data)` / `types.Fail(code, msg, data...)`，定义在 `internal/types/types.go`。

## 2. 业务码枚举表

| 码值 | 常量 | 含义 | 默认 msg | 典型场景 |
|---|---|---|---|---|
| 200 | `CodeOK` | 成功 | `success` | 所有正常响应 |
| 207 | `CodePartial` | 部分成功（多目标任务存在失败项） | "部分成功" | 批量备份、批量镜像清理中有失败项，失败清单在 `data.failures` / `data.failed` |
| 400 | `CodeBadRequest` | 请求参数错误或目标资源不存在 | "请求参数错误" | 参数为空/非法、taskID 未找到、文件名越界等 |
| 401 | `CodeUnauthorized` | 鉴权失败 | 登录接口 `无效的secretKey`；JWT 中间件 `未授权` | 登录 secretKey 不匹配，或 token 缺失/无效 |
| 429 | - | 登录尝试过于频繁 | "尝试过于频繁，请稍后再试" | 仅登录接口的进程内限流 |
| 403 | `CodeForbidden` | 无权执行该操作 | "禁止执行该操作" | 程序自更新被配置禁用 |
| 409 | `CodeConflict` | 资源状态冲突 | "资源状态冲突" | 删除仍被容器引用的镜像 |
| 500 | `CodeServerError` | 服务端内部错误 | "服务端内部错误" | Docker daemon 调用失败、磁盘/配置异常 |

枚举的自描述元数据（含版本号）由 `/api/meta` 返回，前端应优先使用其动态渲染，而不是维护本地副本。

## 3. JSON 示例

成功：

```json
{ "code": 200, "msg": "success", "data": { "version": "1.2.3", "buildDate": "2026-01-01" } }
```

参数错误（400）：

```json
{ "code": 400, "msg": "备份目录不能为空", "data": {} }
```

部分成功（207，批量备份有失败项）：

```json
{ "code": 207, "msg": "备份部分失败：1 个容器未写入备份文件", "data": { "failures": ["web"] } }
```

服务端错误（500）：

```json
{ "code": 500, "msg": "获取版本错误dial tcp: ...", "data": { "remoteVersion": "1.2.3" } }
```

`/api/meta`（自描述元数据，走同一信封）：

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "version": "1.2.3",
    "buildDate": "2026-01-01T00:00:00Z",
    "codes": [
      { "code": 200, "meaning": "成功", "defaultMsg": "success" },
      { "code": 207, "meaning": "部分成功：多目标任务中存在失败项，详见 data 中的失败清单", "defaultMsg": "部分成功" },
      { "code": 400, "meaning": "请求参数错误或目标资源不存在", "defaultMsg": "请求参数错误" },
      { "code": 401, "meaning": "鉴权失败（登录 secretKey 不匹配，或 JWT 缺失/无效）", "defaultMsg": "未授权" },
      { "code": 403, "meaning": "无权执行该操作（如程序自更新被禁用）", "defaultMsg": "禁止执行该操作" },
      { "code": 409, "meaning": "资源状态冲突（如镜像仍被容器引用而无法删除）", "defaultMsg": "资源状态冲突" },
      { "code": 500, "meaning": "服务端内部错误", "defaultMsg": "服务端内部错误" },
      { "code": 429, "meaning": "登录尝试过于频繁", "defaultMsg": "尝试过于频繁，请稍后再试" }
    ]
  }
}
```

## 4. 前端判断约定

- **HTTP 状态码大多与 `body.code` 一致**（handler 把 `resp.Code` 写入 HTTP 状态）；例外：参数解析错误恒为 HTTP 400 + `code=400`；JWT 中间件 401；登录限流 429。前端统一按 body 判断。
- **成败判定**：`body.code === 200` 即完全成功；`code === 207` 为部分成功（有数据但要提示失败项）；其余一律按失败处理。
- **文案**：`msg` 为可直接展示的中文文案；错误时直接展示 `msg`，含义说明取 `/api/meta` 的 `codes[].meaning`。
- **不要硬编码**：码值含义、默认文案、版本号一律来自后端。

## 5. 例外清单（不走统一信封的响应）

截至当前，`internal/logic/**` 所有响应均已收敛到 OK/Fail 信封，暂无流式/文件下载等例外接口。若后续新增（如日志流 SSE、备份文件下载），必须在此处登记并说明其响应格式。
