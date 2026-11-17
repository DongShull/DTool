# DTool 新版本功能级冒烟测试报告

**日期**：2026-11-17　**被测对象**：宿主机 /opt/dtool/DTool（新版构建，md5 91aa307571ea6cd9741d549f88a7b02d，本次改造代码编译）
**部署方式**：systemd 服务 dtool（端口 12713）；旧版已备份为 /opt/dtool/DTool.bak-<时间戳>
**结论**：24 项检查 22 项通过，2 项为环境限制（非缺陷），无功能性回归。测试未触碰生产容器与 compose 工程，临时资源已全部清理。

## 一、鉴权与安全（5/5 通过）

| # | 检查项 | 结果 |
|---|---|---|
| 1 | JSON body 登录换 JWT | ✅ 200 返回 jwt |
| 2 | 无 token 访问 /api/containers | ✅ 401 |
| 3 | 无 token 调 /api/compose/<工程名>/up（高危写） | ✅ 401 |
| 4 | 不存在路由 | ✅ 404 |
| 5 | compose up 参数注入（project="foo;rm"） | ✅ 500 明确拒绝"project 名含非法字符或以 - 开头"，未执行任何命令 |

## 二、容器管理（8 项）

| # | 检查项 | 结果 |
|---|---|---|
| 6 | 容器列表（7 台真实容器，含 compose 标记） | ✅ 200 |
| 7 | 容器 detail（wxchat：端口/挂载/网络/compose 归属） | ✅ 200 |
| 8 | 容器 logs（tail=3，解复用干净） | ✅ 200 |
| 9 | 容器 stats（CPU%/内存/网络实时采样） | ✅ 200 |
| 10 | 临时容器 stop → start → rename | ✅ 全部 200 |
| 11 | 更新任务提交（指定 imageName+imageTag） | ✅ 200 返回 taskID，异步语义正确 |
| 12 | 更新负路径：目标容器不存在 | ✅ 任务正确落盘失败态 "No such container"，旧容器未误删 |
| 13 | 更新负路径：registry 不可达 | ✅ 进度明确记录"拉取镜像失败：context deadline exceeded"（环境限制见第四节） |

## 三、Compose 工程控制（新功能，4 项）

| # | 检查项 | 结果 |
|---|---|---|
| 14 | GET /api/compose/projects | ✅ 200：六个 compose 工程（生产工程名已脱敏），含 workdir、服务数与运行状态 |
| 15 | GET /api/compose/<工程名>/logs?tail=1 | ✅ 200，按服务分块聚合日志 |
| 16 | 生产工程 up/down 实测 | ⏭ 未执行（守则禁止动生产；逻辑由 10+ mock 单测覆盖：参数构造、token 校验、状态推导、审计） |
| 17 | 自护栏（拒绝作用于自身工程） | ⏭ 静态确认代码在位 + 单测覆盖 |

## 四、镜像 / 卷 / 备份 / 进度 / 系统（7 项）

| # | 检查项 | 结果 |
|---|---|---|
| 18 | 镜像列表 | ✅ 200（含 inUsed 标记） |
| 19 | 测试镜像删除（dtool-test-img:tmp） | ✅ 200 |
| 20 | 测试卷创建 + DELETE /api/volumes | ✅ 200，卷已删 |
| 21 | 备份创建 GET /api/container/backup | ✅ 200，failures=[] |
| 22 | 备份列表 /api/container/listBackups | ✅ 200（本次生成 json+yaml 双备份，纳秒文件名） |
| 23 | 备份导出 compose | ✅ 200 |
| 24 | 进度查询（含失败任务明细） | ✅ 200 |

其余：/api/system、/api/networks、/api/settings、/api/version、/api/icons 均 ✅ 200。

## 五、MCP 服务

- 握手（Streamable HTTP + Bearer）：✅ 200 + Mcp-Session-Id
- tools/list：✅ **41 个 dt_* 工具**全部注册（含 4 个新 compose 工具）

## 六、环境限制（非缺陷，2 项）

1. **镜像拉取超时**：本机无法直连 registry-1.docker.io（context deadline exceeded）。更新链对本地已有镜像场景不受影响；远端拉取需给 Docker 守护进程配 registry mirror 或代理。
2. **生产 compose 工程 up/down 未实测**：遵守不动生产守则，由 mock 单测覆盖。

## 七、发现的问题（1 个，轻微）

- **契约不一致（存量，非本次引入）**：GET /api/containers 成功返回 code:0，其余接口 code:200。已在 docs/ROADMAP.md 契约治理待办中记录，建议下一轮统一。

## 八、遗留资源

无。临时容器 dtool-test-nginx / dtool-test-nginx2、测试镜像 dtool-test-img:tmp、测试卷 dtool-test-vol 均已删除。

## 九、回退方式

    systemctl stop dtool
    cp /opt/dtool/DTool.bak-<时间戳> /opt/dtool/DTool
    systemctl start dtool

## 附：顺手发现的真实环境隐患（与 DTool 无关）

某缓存服务容器日志反复出现 "Possible SECURITY ATTACK detected ... Cross Protocol Scripting"——其端口疑似对公网暴露且未设密码（已在后续运维中处理，细节从略）。