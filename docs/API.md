# DTool HTTP API 接口文档

> 面向前端开发者。以 `internal/handler/routes.go` 的路由注册为准（含手工扩展路由）。
> MCP 工具不在本文档范围内，见 [docs/MCP.md](./MCP.md)。

---

## 一、全局约定

### 1.1 统一响应信封

所有接口（除 SSE 流）返回统一 JSON 信封：

```json
{
  "code": 200,
  "msg": "success",
  "data": {}
}
```

- `code`：业务码（int）。**`resp.Code` 会被 handler 直接当作 HTTP 状态码写入**，因此大多数情况下 HTTP 状态码 === body.code。
- `msg`：提示文案（string），成功恒为 `"success"`（部分接口会附带更具体说明，如 "程序有更新"）。
- `data`：业务数据；无数据时为 `{}` 或 `null`；错误时通常为 `{}`，部分接口会携带补充字段（如 `failures`）。

**参数缺失 / 格式错误**（query/body 解析失败）时，服务端返回 **HTTP 400**，body 为同一信封且
`body.code = 400`：

```json
{ "code": 400, "msg": "<具体原因>", "data": {} }
```

### 1.2 前端判断约定

**`body.code === 200` 即成功**。非 200 一律按失败处理，失败文案从 `body.msg` 取；码值含义与默认文案应通过 `GET /api/meta` 动态获取，**禁止在前端硬编码任何码值含义或文案**。

### 1.3 错误码枚举表

| 业务码 | 含义 | 默认 msg |
|---|---|---|
| 200 | 成功 | `success` |
| 207 | 部分成功：多目标任务中存在失败项，详见 data 中的失败清单 | `部分成功` |
| 400 | 请求参数错误或目标资源不存在 | `请求参数错误` |
| 401 | 鉴权失败（两种来源见下） | 登录接口 `无效的secretKey`；JWT 中间件 `未授权` |
| 403 | 无权执行该操作（如程序自更新被禁用） | `禁止执行该操作` |
| 409 | 资源状态冲突（如镜像仍被容器引用而无法删除） | `资源状态冲突` |
| 429 | 登录尝试过于频繁（仅登录接口） | `尝试过于频繁，请稍后再试` |
| 500 | 服务端内部错误 | `服务端内部错误` |

> 以上枚举与文案可由 `GET /api/meta` 动态获取（见下文），本文表格仅供阅读。

### 1.4 鉴权

除 `POST /api/auth`、`GET /api/meta`（新增）和 `GET /`（健康检查）外，**所有接口都需要 JWT**。

**登录换 JWT：`POST /api/auth`**

请求（form 或 JSON 均可，字段名以代码 tag 为准 `form:"secretKey,optional" json:"secretKey,optional"`）：

```json
{ "secretKey": "your-secret-key" }
```

成功响应（**注意：data 里的字段名是 `jwt`**，来自 logic 中的 `JwtResponse{ Jwt string `json:"jwt"` }`，不是 `token`）：

```json
{ "code": 200, "msg": "success", "data": { "jwt": "eyJhbGciOiJIUzI1NiIs..." } }
```

失败响应（401，**登录接口专用**：secretKey 不匹配）：

```json
{ "code": 401, "msg": "无效的secretKey", "data": { "jwt": "" } }
```

注意区分另一种 401：**JWT 鉴权中间件**拒绝请求（token 缺失/过期/无效）时返回的是
不同 body（HTTP 401，由 `dtool.go` 的 `WithUnauthorizedCallback` 写出）：

```json
{ "code": 401, "msg": "未授权", "data": {} }
```

限流响应（HTTP 429，让客户端区分"口令错"与"被限流"）：

```json
{ "code": 429, "msg": "尝试过于频繁，请稍后再试" }
```

JWT 为 HS256，claims 含 `iat` / `exp`（`exp = iat + AccessExpire`）。

**JWT 携带方式**（go-zero `rest.WithJwt` 默认）：

```
Authorization: Bearer <jwt>
```

### 1.5 参数风格说明

- 路径参数：如 `/api/container/{id}/start` 中的 `{id}`。
- 查询参数（form tag）：GET/POST 的 query string。
- Body：JSON 或 multipart（仅图标上传）；每个接口的入参以 `internal/types` 中的 req struct tag 为准。

---

## 二、元数据（新增）

### GET /api/meta 【新增】

无需鉴权。返回后端自描述元数据：版本信息 + 全部业务码枚举（含义与默认文案），供前端零硬编码渲染错误信息。

- 请求参数：无。
- 响应 data 结构（`MetaInfo`）：

| 字段 | 类型 | 说明 |
|---|---|---|
| version | string | 后端版本号 |
| buildDate | string | 构建时间 |
| codes | CodeInfo[] | 全部业务码枚举数组 |

`CodeInfo`：

| 字段 | 类型 | 说明 |
|---|---|---|
| code | int | 业务码数值 |
| meaning | string | 中文含义 |
| defaultMsg | string | 该码的默认 msg 文案 |

成功示例：

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "version": "1.2.0",
    "buildDate": "2026-01-01 00:00:00",
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

> 各接口实际 msg 以响应体为准（可能比 defaultMsg 更具体）。

---

## 三、健康检查

### GET /

无需鉴权。容器 HEALTHCHECK 入口，返回版本与服务入口信息。

```json
{
  "code": 200,
  "msg": "DTool API-only mode",
  "data": { "version": "1.2.0", "mcp": "/mcp", "api": "/api" }
}
```

---

## 四、容器管理

### GET /api/containers

列出全部容器（含停止的），并附带镜像更新检查结果。

响应 data 为数组，每项字段（`Info`）：

| 字段 | 类型 | 说明 |
|---|---|---|
| id | string | 容器 ID |
| status | string | 状态（running/exited/paused/created/restarting） |
| name | string | 容器名 |
| usingImage | string | 当前使用的镜像（含 tag） |
| createImage | string | 创建时的镜像 |
| createTime | string | 创建时间 `yyyy-MM-dd HH:mm:ss` |
| runningTime | string | Docker status 文案，如 "Up 2 minutes" |
| haveUpdate | bool | 是否有镜像更新 |
| isCompose | bool | 是否为 docker compose 创建 |
| imageName | string | 不带 tag 的镜像名 |
| imageTag | string | tag（`<none>` 表示无 tag） |
| ports | string[] | 形如 `"0.0.0.0:13000→3000/tcp"` |
| health | string | healthy/unhealthy/starting；无健康检查为空 |
| stateDetail | string | 同 status |
| composeProject | string | compose 工程名（非 compose 为空） |
| composeService | string | compose 服务名 |
| createdUnix | int64 | 创建时间（Unix 秒） |
| labelCount | int | 标签数量 |

成功示例：

```json
{
  "code": 200,
  "msg": "success",
  "data": [
    {
      "id": "demo0123456789abcdef",
      "status": "running",
      "name": "demo",
      "usingImage": "nginx:latest",
      "createImage": "nginx:latest",
      "createTime": "2026-01-01 08:00:00",
      "runningTime": "Up 2 minutes",
      "haveUpdate": false,
      "isCompose": true,
      "imageName": "nginx",
      "imageTag": "latest",
      "ports": ["0.0.0.0:80→80/tcp"],
      "health": "",
      "stateDetail": "running",
      "composeProject": "web",
      "composeService": "nginx",
      "createdUnix": 1767225600,
      "labelCount": 3
    }
  ]
}
```

### POST /api/container/{id}/start

启动容器。已在运行则幂等成功；paused 容器报错（code 400）。

- 路径参数：`id`（string，必填，容器 ID 或名称）。

成功：`{ "code": 200, "msg": "success", "data": {} }`
失败（400）：`{ "code": 400, "msg": "容器处于暂停状态...", "data": {} }`

### POST /api/container/{id}/stop

停止容器。已停止则幂等成功；paused 容器先 unpause 再 stop。

- 路径参数：`id`（string，必填）。

成功：`{ "code": 200, "msg": "success", "data": {} }`

### POST /api/container/{id}/restart

重启容器。每次都真正执行 restart（非幂等语义）；已停止的容器 restart 会把它启动。

- 路径参数：`id`（string，必填）。

成功：`{ "code": 200, "msg": "success", "data": {} }`

### POST /api/container/{id}/rename

重命名容器。

| 参数 | 位置 | 类型 | 必填 | 说明 | 示例 |
|---|---|---|---|---|---|
| id | path | string | 是 | 容器 ID 或名称 | `demo` |
| newName | query(form) | string | 是 | 新容器名，不能为空 | `demo-new` |

成功：`{ "code": 200, "msg": "success", "data": {} }`
失败（400）：`{ "code": 400, "msg": "新容器名不能为空", "data": {} }`

### POST /api/container/{id}/update 【异步】

更新容器镜像（"一键更新"）。**异步：立即返回 taskID，用进度接口轮询/SSE 订阅结果。**

| 参数 | 位置 | 类型 | 必填 | 说明 | 示例 |
|---|---|---|---|---|---|
| id | path | string | 是 | 容器 ID 或名称 | `demo` |
| imageNameAndTag | query(form) | string | 否 | 目标镜像；留空按容器当前镜像更新 | `nginx:1.27` |
| containerName | query(form) | string | 否 | 容器名（用于进度过滤/审计；不传则服务端反查） | `demo` |

成功：

```json
{ "code": 200, "msg": "success", "data": { "taskID": "b3a1..." } }
```

> 环境变量 `DelOldContainer=false` 可控制更新后是否保留旧容器（默认删除）。

---

## 五、容器详情 / 日志 / 资源 / 系统

### GET /api/system

系统总览：Docker 版本 + 主机信息 + 磁盘占用。data 结构（`SystemOverview`）：

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "docker": {
      "version": "27.0.0", "apiVersion": "1.46", "minApiVersion": "1.24",
      "os": "linux", "arch": "amd64", "kernelVersion": "6.1.0"
    },
    "info": {
      "name": "docker-host", "serverVersion": "27.0.0",
      "operatingSystem": "Debian GNU/Linux 12", "osType": "linux",
      "architecture": "x86_64", "nCPU": 4, "memTotal": 8589934592,
      "dockerRootDir": "/var/lib/docker", "storageDriver": "overlay2",
      "containers": 5, "containersRunning": 3, "containersPaused": 0,
      "containersStopped": 2, "images": 12, "warnings": 0
    },
    "disk": {
      "layersSize": 10737418240, "images": 12, "containers": 5,
      "volumes": 8, "buildCache": 0, "reclaimable": 5368709120,
      "detail": {
        "unusedImages": 3221225472,
        "stoppedContainersSize": 1073741824,
        "volumesSize": 1073741824
      }
    }
  }
}
```

### GET /api/container/{id}/detail

容器 inspect 摘要（`ContainerDetail`）。失败（容器不存在）返回 code 404。

data 字段逐一列出：

| 字段 | 类型 | 说明 |
|---|---|---|
| id | string | 容器 ID |
| name | string | 容器名 |
| image | string | 镜像引用 |
| imageId | string | 镜像 ID |
| state | string | 状态 |
| status | string | Docker status 文案 |
| health | string | 健康状态（omitempty） |
| created | string | 创建时间 |
| startedAt | string | 启动时间（omitempty） |
| restartCount | int | 重启次数 |
| restartPolicy | string | 重启策略 |
| platform | string | 平台（omitempty） |
| pid | int | 主进程 PID |
| exitCode | int | 退出码 |
| command | string[] | 启动命令（omitempty） |
| entrypoint | string[] | 入口（omitempty） |
| env | string[] | 环境变量（omitempty） |
| ports | PortMapping[] | 端口映射（omitempty） |
| mounts | MountInfo[] | 挂载（omitempty） |
| networks | ContainerNet[] | 网络（omitempty） |
| labels | object | 标签 map（omitempty） |
| compose | ComposeInfo | `{project,service,workingDir,configFiles}`（均 omitempty） |
| resources | ResourceInfo | `{memory,nanoCpus,cpuShares,pidsLimit,privileged,readonlyRootfs}` |
| user | string | 运行用户（omitempty） |
| workingDir | string | 工作目录（omitempty） |
| logPath | string | 日志路径（omitempty） |

`PortMapping`：`{containerPort, protocol, hostIp?, hostPort?}`
`MountInfo`：`{type, source, destination, mode?, rw, name?}`
`ContainerNet`：`{name, ipAddress?, gateway?, macAddress?, aliases?}`

### GET /api/container/{id}/logs

读取容器日志，支持区间查询。**一次性读取，不跟随。**

| 参数 | 位置 | 类型 | 必填 | 说明 | 示例 |
|---|---|---|---|---|---|
| id | path | string | 是 | 容器 ID 或名称 | `demo` |
| tail | query | int | 否 | 返回末尾行数；默认 200，≤0 取默认，上限 5000 | `200` |
| timestamps | query | bool | 否 | 是否在每行前带 Docker 时间戳，默认 false | `true` |
| since | query | string | 否 | 只返回该时间之后的日志（Unix 秒或 RFC3339，透传 Docker） | `1700000000` |
| until | query | string | 否 | 只返回该时间之前的日志；与 since 组成 **since..until 闭区间** | `1700003600` |

> **时区注意**：`since`/`until` 的时间串由 Docker daemon 按**其所在主机时区**解释。
> 客户端直接传浏览器本地时间字符串会产生偏移；建议传 Unix 秒，或带时区的
> RFC3339（如 `2026-01-01T00:00:00+08:00`），或 UTC 时间。

响应 data：

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "containerId": "demo",
    "tail": 200,
    "since": "1700000000",
    "until": "1700003600",
    "lines": ["2026-01-01T08:00:00.000Z access log line 1", "..."]
  }
}
```

失败（400）：`{ "code": 400, "msg": "获取容器日志失败: ...", "data": null }`

### GET /api/container/{id}/stats

一次性资源采样（不持续流式）。data 结构（`ContainerStat`）：

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "cpuPercent": 2.5,
    "memUsage": 134217728,
    "memLimit": 2147483648,
    "memPercent": 6.25,
    "netRx": 1048576,
    "netTx": 524288,
    "blockRead": 2097152,
    "blockWrite": 1048576,
    "pids": 12,
    "sampledAt": 1767225600
  }
}
```

### GET /api/volumes

列出数据卷。data 为 `VolumeInfo[]`：`{name, driver, mountpoint, scope, createdAt, labels?, inUse, usedBy?}`。

### DELETE /api/volumes/{name}

删除数据卷（**卷里的数据不可恢复，前端应二次确认**）。

| 参数 | 位置 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| name | path | string | 是 | 卷名 |
| force | query | bool | 否 | 强制删除，默认 false |

成功：`{ "code": 200, "msg": "success", "data": { "name": "mydata", "force": false } }`
失败（400）：`{ "code": 400, "msg": "卷仍被容器使用...", "data": null }`

### GET /api/networks

列出网络。data 为 `NetworkInfo[]`：`{id, name, driver, scope, internal, attachable, created, subnet?, gateway?, labels?, containers}`。

### GET /api/events （SSE）

Docker 事件流，前端用它替代轮询做容器状态实时刷新。**需要鉴权（JWT）。**

| 参数 | 位置 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| since | query | string | 否 | 只推送该时间之后的事件 |

- 响应头：`Content-Type: text/event-stream`、`Cache-Control: no-cache`、`X-Accel-Buffering: no`。
- 连接先发 `retry: 3000`；每 20 秒无事件时发注释行 `: heartbeat` 防止代理掐断；单连接最长 10 分钟，到时发 `: stream deadline reached, reconnecting` 注释行后关闭，客户端按 retry 自动重连。
- 事件格式：`data: <EventSummary JSON>`（无 event 名，即默认 `message` 事件）。

`EventSummary` 字段：

| 字段 | 类型 | 说明 |
|---|---|---|
| type | string | Docker 事件类型（container/image/volume/network...） |
| action | string | 动作（start/stop/die/destroy...） |
| time | string | 事件时间 |
| id | string | Actor ID（有则携带） |
| shortId | string | ID 前 12 位（有则携带） |
| name | string | 对象名（有则携带） |
| image | string | 镜像名（有则携带） |

示例：

```
retry: 3000

: heartbeat

data: {"type":"container","action":"start","time":"2026-01-01T08:00:00Z","id":"demo0123456789abcdef","shortId":"demo01234567","name":"demo","image":"nginx:latest"}

```

---

## 六、镜像管理

### GET /api/images

列出全部镜像。data 为数组，每项字段（`Info`）：`{id, name, tag, size, inUsed, createTime}`（size 为格式化字符串如 `"187MB"`，createTime 为 `yyyy-MM-dd HH:mm:ss`）。

```json
{
  "code": 200,
  "msg": "success",
  "data": [
    { "id": "sha256:abc123...", "name": "nginx", "tag": "latest", "size": "187MB", "inUsed": true, "createTime": "2026-01-01 08:00:00" }
  ]
}
```

### DELETE /api/image/{id}

删除单个镜像。

| 参数 | 位置 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| id | path | string | 是 | 镜像 ID（可带 `sha256:` 前缀，服务端会去掉） |
| force | query(form) | bool | 否 | 强制删除，默认 false |

成功：`{ "code": 200, "msg": "success", "data": {} }`
失败（**409 冲突**，如镜像仍被容器引用）：`{ "code": 409, "msg": "image is being used by ...", "data": {} }`

### DELETE /api/images

批量清理镜像。

| 参数 | 位置 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| filter | query(form) | string | 是 | `untagged`=删除无 TAG 镜像；`unused`=删除未被任何容器引用的镜像 |

成功响应 data：`{ "filter": "untagged", "removed": ["sha256:a..."], "failed": ["sha256:b..."] }`。
部分失败时 code=**207**，msg 如 `"清理部分失败：2 个镜像未删除（详见 failed 字段）"`。
参数错误（400）：`{ "code": 400, "msg": "filter 参数错误，只支持 untagged（无 TAG 镜像）或 unused（未使用镜像）", "data": {} }`

---

## 七、备份系列

### GET /api/container/backup

备份容器配置。`include` 为空备份全部容器；非空时只备份名称（或 ID）命中的容器（可多值：`?include=a&include=b`）。

成功（全部成功）：

```json
{ "code": 200, "msg": "success", "data": { "failures": [] } }
```

部分失败（**code 207**）：`{ "code": 207, "msg": "备份部分失败：1 个容器未写入备份文件", "data": { "failures": ["demo"] } }`
全部失败（500）：`{ "code": 500, "msg": "<错误信息>", "data": { "failures": [...] } }`

### GET /api/container/backup2compose

把现有容器配置导出为 compose 文件（备份目录内生成 `backup-*.yaml`）。

成功：`{ "code": 200, "msg": "success", "data": null }`

### GET /api/container/listBackups

列出备份文件（只列本工具生成的 `backup-*.json` / `backup-*.yaml`）。

```json
{ "code": 200, "msg": "success", "data": ["backup-2026-01-01-080000.json", "backup-2026-01-02-090000.yaml"] }
```

### DELETE /api/container/backups

删除一个备份文件（**不可恢复**）。只允许删除 `backup-*.json` / `backup-*.yaml`，防路径穿越。

| 参数 | 位置 | 类型 | 必填 | 说明 | 示例 |
|---|---|---|---|---|---|
| filename | query(form) | string | 是 | 备份文件名 | `backup-2026-01-01-080000.json` |

成功：`{ "code": 200, "msg": "success", "data": {} }`
失败（400）：`{ "code": 400, "msg": "非法文件名：只允许删除 backup-*.json / backup-*.yaml", "data": {} }`

### POST /api/container/backups/restore 【异步】

从 JSON 配置备份恢复容器。**异步：立即返回 taskID，全局串行执行（排队期间进度可查可取消）。**

Body（JSON，`ContainerRestoreReq`）：

| 字段 | 类型 | 必填 | 说明 | 示例 |
|---|---|---|---|---|---|
| filename | string | 是 | 备份文件名（必须 .json，目前仅支持 config 备份恢复） | `"backup-xxx.json"` |
| start | bool | 否 | 为空按备份记录的状态恢复；true/false 强制启动或不启动 | `true` |
| force | bool | 否 | true 时同名容器先删除后重建；默认跳过同名容器 | `false` |
| nameMap | object | 否 | 容器名重定向：键=备份中的容器名，值=恢复后的容器名（不改备份原名） | `{"old":"new"}` |

成功：

```json
{ "code": 200, "msg": "success", "data": { "taskID": "r8c2..." } }
```

失败（400）：`{ "code": 400, "msg": "目前仅支持config备份恢复", "data": {} }`

### GET /api/container/backups/compare

对比两个 JSON 配置备份的差异（old 为基线，new 为当前）。

| 参数 | 位置 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| old | query(form) | string | 是 | 旧备份文件名 |
| new | query(form) | string | 是 | 新备份文件名 |

成功 data 结构（`BackupDiffResult`）：

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "oldFile": "backup-a.json",
    "newFile": "backup-b.json",
    "added": ["nginx"],
    "removed": ["redis"],
    "changed": [
      { "name": "demo", "status": "changed", "fields": ["Image"], "oldImage": "old:latest", "newImage": "new:latest" }
    ],
    "unchangedCount": 3
  }
}
```

`changed` 项（`BackupEntryDiff`）：`{name, status(added/removed/changed/unchanged), fields?, oldImage?, newImage?}`。
失败（400）：`{ "code": 400, "msg": "旧备份读取失败: ...", "data": {} }`

---

## 八、任务进度（异步任务）

> 容器更新、备份恢复均为异步任务：调用接口立即返回 `taskID`，随后用本节接口轮询或 SSE 订阅进度。
> 任务状态（`TaskProgress`）归一化为：`running`（进行中）/ `cancelled`（被主动取消）/ `done`（正常完成）。

### GET /api/progress

任务进度列表。

| 参数 | 位置 | 类型 | 必填 | 说明 | 示例 |
|---|---|---|---|---|---|
| status | query | string | 否 | running / cancelled / done，为空返回全部 | `running` |
| name | query | string | 否 | 按任务名子串匹配（不区分大小写） | `demo` |

data 为 `TaskProgress[]`，字段：

| 字段 | 类型 | 说明 |
|---|---|---|
| taskID | string | 任务 ID |
| percentage | int | 进度百分比 0~100 |
| message | string | 当前消息 |
| name | string | 任务名（一般即容器名） |
| detailMsg | string | 详细消息 |
| isDone | bool | 是否已结束 |
| startedAt | int64 | 开始时间（Unix 秒） |
| finishedAt | int64 | 结束时间（Unix 秒，未结束为 0） |
| cancelled | bool | 是否被主动取消（omitempty，进行中恒 false） |
| cancelRequestedAt | int64 | 发出取消请求的时间（Unix 秒，未取消为 0，omitempty） |

```json
{
  "code": 200,
  "msg": "success",
  "data": [
    { "taskID": "b3a1...", "percentage": 40, "message": "拉取新镜像中", "name": "demo", "detailMsg": "Pulling nginx:1.27", "isDone": false, "startedAt": 1767225600, "finishedAt": 0 }
  ]
}
```

### GET /api/progress/{taskid}

查询单个任务进度。

- 路径参数：`taskid`（string，必填）。

成功（注意 msg 会带上任务当前 message）：

```json
{
  "code": 200,
  "msg": "拉取新镜像中",
  "data": {
    "taskID": "b3a1...",
    "percentage": 40,
    "message": "拉取新镜像中",
    "name": "demo",
    "detailMsg": "Pulling nginx:1.27",
    "isDone": false
  }
}
```

失败（400）：`{ "code": 400, "msg": "taskID 未找到", "data": {} }`

### GET /api/progress/export

按条件导出任务进度为 JSON（含汇总头），过滤条件与列表一致，另支持时间范围（按开始时间，Unix 秒）。

| 参数 | 位置 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| status | query | string | 否 | 同列表 |
| name | query | string | 否 | 同列表 |
| from | query | int64 | 否 | 开始时间下限（Unix 秒） |
| to | query | int64 | 否 | 开始时间上限（Unix 秒） |

data 结构（已格式化 JSON）：

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "exportedAt": "2026-01-01T08:00:00Z",
    "filter": { "status": "", "name": "", "from": 0, "to": 0 },
    "summary": { "total": 2, "byStatus": { "running": 1, "cancelled": 0, "done": 1 } },
    "tasks": [ { "...": "TaskProgress 同列表字段" } ]
  }
}
```

### POST /api/progress/{taskid}/cancel

取消一个进行中的任务。

- 路径参数：`taskid`（string，必填）。

成功：`{ "code": 200, "msg": "success", "data": { "taskID": "b3a1..." } }`
失败（400，任务不存在或已结束）：`{ "code": 400, "msg": "<原因>", "data": {} }`

### GET /api/progress/{taskid}/events （SSE）

以 Server-Sent Events 推送单个任务进度，替代高频轮询。**需要鉴权（JWT）。**

- 路径参数：`taskid`（string，必填）。
- **任务不存在时在写 200 之前返回 404**：`{ "code": 404, "msg": "taskID 未找到" }`（不会建立流）。
- 连接先发 `retry: 3000`；每 500ms 检查一次进度，**内容有变化时**发 `data: <TaskProgress JSON>`（无 event 名）；每 15 秒无变化发注释行 `: keep-alive`（注意与 Docker 事件流 `/api/events` 的 `: heartbeat` 文案不同，两个接口各自实现）；单连接最长 30 分钟后正常关闭，EventSource 按 retry 自动重连。
- 任务完成（`isDone=true`）时推送最后一条进度后关流。

进度事件示例：

```
retry: 3000

data: {"taskID":"b3a1...","percentage":40,"message":"拉取新镜像中","name":"demo","detailMsg":"Pulling nginx:1.27","isDone":false,"startedAt":1767225600,"finishedAt":0}

data: {"taskID":"b3a1...","percentage":100,"message":"更新完成","name":"demo","detailMsg":"","isDone":true,"startedAt":1767225600,"finishedAt":1767225900}

```

---

## 九、Compose 系列

### GET /api/compose/projects

聚合列出全部 compose 工程。data 为 `ComposeProjectInfo[]`：

| 字段 | 类型 | 说明 |
|---|---|---|
| project | string | 工程名 |
| workdir | string | 工作目录 |
| configFiles | string | compose 文件路径（omitempty） |
| services | int | 服务数 |
| running | int | 运行中容器数 |
| stopped | int | 停止容器数 |
| status | string | 状态描述 |
| containers | ComposeContainerBrief[] | 容器摘要：`{name, service, state, image}` |

```json
{
  "code": 200,
  "msg": "success",
  "data": [
    {
      "project": "web",
      "workdir": "/data/compose/web",
      "configFiles": "/data/compose/web/docker-compose.yml",
      "services": 2,
      "running": 1,
      "stopped": 1,
      "status": "partial",
      "containers": [
        { "name": "web-nginx-1", "service": "nginx", "state": "running", "image": "nginx:latest" }
      ]
    }
  ]
}
```

### POST /api/compose/{project}/up

启动 compose 工程（等价 `docker compose up -d`）。写操作，服务端统一落审计日志。

- 路径参数：`project`（string，必填，工程名）。

成功：

```json
{ "code": 200, "msg": "success", "data": { "project": "web", "output": " Network web_default Created ..." } }
```

失败（500）：`{ "code": 500, "msg": "<错误信息>", "data": "<compose 输出>" }`

### POST /api/compose/{project}/down

停止并移除 compose 工程（等价 `docker compose down`）。请求/响应结构同 up。

### GET /api/compose/{project}/logs

聚合工程日志。

| 参数 | 位置 | 类型 | 必填 | 说明 | 示例 |
|---|---|---|---|---|---|
| project | path | string | 是 | 工程名 | `web` |
| service | query | string | 否 | 只看某服务；为空聚合全部服务 | `nginx` |
| tail | query | int | 否 | 每服务行数；为 0 按默认行数取 | `200` |

data 为 `ComposeServiceLogs[]`：

```json
{
  "code": 200,
  "msg": "success",
  "data": [
    { "service": "nginx", "container": "web-nginx-1", "lines": ["...", "..."] }
  ]
}
```

---

## 十、设置

### GET /api/settings

读取设置。data：

```json
{ "code": 200, "msg": "success", "data": { "backupDir": "/data/backups", "effectiveBackupDir": "/data/backups" } }
```

（`backupDir` 为用户配置值，`effectiveBackupDir` 为实际生效目录：env `BACKUP_DIR` > Settings > 默认。）

### PUT /api/settings

更新设置。Body（JSON）：

| 字段 | 类型 | 必填 | 说明 | 示例 |
|---|---|---|---|---|---|
| backupDir | string | 是 | 备份目录，必须为绝对路径且可写，不能指向配置/日志目录 | `"/data/backups"` |

成功：响应同 GET /api/settings 的 data。
失败（400）示例：`{ "code": 400, "msg": "备份目录必须是绝对路径", "data": {} }`

---

## 十一、图标

### POST /api/icons

上传自定义镜像图标（multipart，≤10MB）。

| 参数 | 位置 | 类型 | 必填 | 说明 | 示例 |
|---|---|---|---|---|---|
| file | form-data | file | 是 | 图片文件，仅 png/jpg/jpeg/webp/gif，且内容须与扩展名匹配 | — |
| imageName | form-data | string | 是 | 镜像名 key，须匹配 `^[A-Za-z0-9][A-Za-z0-9._/:-]*$` | `nginx` |

成功（data 为服务端生成的存储文件名）：

```json
{ "code": 200, "msg": "success", "data": "3f2a1b8c-....png" }
```

失败（400）：`{ "code": 400, "msg": "failed to get file", "data": {} }`

### GET /api/icons

读取全部自定义图标映射（镜像名 → 图标路径）。配置文件不存在时返回空 map（与正常路径同型，前端无需两套解析）。

```json
{ "code": 200, "msg": "success", "data": { "nginx": "/src/config/image/3f2a1b8c-....png" } }
```

---

## 十二、版本与程序自更新

### GET /api/version

查询版本。

| 参数 | 位置 | 类型 | 必填 | 说明 | 示例 |
|---|---|---|---|---|---|
| type | query | string | 否 | `local`（默认）/ `remote` | `remote` |

type=local（或省略）：

```json
{ "code": 200, "msg": "success", "data": { "version": "1.2.0", "buildDate": "2026-01-01 00:00:00" } }
```

type=remote 且有更新：`{ "code": 200, "msg": "程序有更新", "data": { "remoteVersion": "1.3.0" } }`
type=remote 且无更新：`{ "code": 200, "msg": "程序无更新", "data": { "remoteVersion": "1.2.0" } }`
type=remote 平台托管版（FnOS/UGREEN 等，不走通用更新链）：`{ "code": 200, "msg": "<平台说明>", "data": { "remoteVersion": "1.2.0", "platform": "fnos", "managed": "true", "fnos": "true" } }`（`fnos` 仅 FnOS 时携带，兼容旧字段）
type 非法（400）：`{ "code": 400, "msg": "type 参数错误", "data": {} }`
获取失败（500）：`{ "code": 500, "msg": "获取版本错误...", "data": { "remoteVersion": "1.2.0" } }`

### PUT /api/program

触发程序自更新（成功后进程约 10 秒退出并由守护脚本换入新版本，期间接口不再可用）。

Body（JSON 或 form）：

| 字段 | 类型 | 必填 | 说明 | 示例 |
|---|---|---|---|---|---|
| target | string | 否 | 为空更新到最新版；非空强制更新到指定版本（允许降级） | `"v1.3.0"` |

成功：`{ "code": 200, "msg": "success", "data": {} }`
被禁用（403，env `UPDATE_PROGRAM_ENABLED=false`）：`{ "code": 403, "msg": "程序自更新已被禁用（UPDATE_PROGRAM_ENABLED=false）", "data": {} }`
target 非法（400）：`{ "code": 400, "msg": "<校验错误>", "data": {} }`

### GET /api/program/history

程序自更新历史（最新在前）。

| 参数 | 位置 | 类型 | 必填 | 说明 | 示例 |
|---|---|---|---|---|---|
| limit | query | int | 否 | 返回条数；默认 20，最大 50 | `20` |

data：

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "currentVersion": "1.2.0",
    "buildDate": "2026-01-01 00:00:00",
    "history": [
      {
        "time": "2026-01-01T08:00:00Z",
        "fromVersion": "1.1.0",
        "toVersion": "1.2.0",
        "ok": true,
        "manual": true,
        "message": "更新成功"
      }
    ]
  }
}
```

---

## 十三、路由统计

- 依据 `internal/handler/routes.go` 当前注册的路由（含手工扩展部分）共 **41** 条：
  - `GET /`（健康检查）×1
  - `/api/auth` ×1
  - 容器组（JWT）×12：rename/restart/start/stop/update/backup/backup2compose/backups(DELETE)/backups/restore/listBackups/backups/compare/containers
  - 图标组（JWT）×2：POST+GET `/api/icons`
  - 镜像组（JWT）×3：DELETE /api/image/{id}、GET+DELETE /api/images
  - 进度组（JWT）×4：progress、progress/export、progress/{taskid}、progress/{taskid}/cancel
  - 进度 SSE ×1：GET /api/progress/{taskid}/events（WithSSE）
  - 设置 ×2、系统只读组 ×7（system/volumes/volumes/{name}/networks/container detail/logs/stats）
  - Compose ×4、Docker 事件 SSE ×1（GET /api/events，WithSSE）
  - 版本组 ×3（program/version/program/history）
- 文档另收录 **GET /api/meta（新增）** 1 条（已注册于 routes.go 的独立免鉴权路由组）。
- 合计文档接口条目 **42** 条 = routes.go 已注册 41 + 新增 1。

> MCP 工具不在本文档范围，见 [docs/MCP.md](./MCP.md).
