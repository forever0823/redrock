
## 项目结构

`controller/`（主控）+ `agent/`（代理机）纯 Go + Bash。配置文件为根目录的 `config.env`。
原 Python 版仍保留在 `controller_node/`、`agent_node/` 目录作参考。

```
bash_check/
├── config.env               # 统一配置文件（纯文本 key=value）
├── build.sh / build.ps1     # 跨平台静态编译脚本
├── dashboard.html           # Web 面板
├── controller/
│   ├── main.go              # 主控端 HTTP 服务器
│   └── go.mod               # module controller, go 1.21
├── agent/
│   ├── main.go              # 代理端心跳 + 巡检（Bash 脚本编译时嵌入）
│   ├── go.mod               # module agent, go 1.21
│   └── scripts/             # Bash 巡检脚本（编译时通过 embed 内嵌到二进制）
│       ├── system_check.sh
│       ├── security_baseline_check.sh
│       ├── process_check.sh
│       ├── check_config.sh      # 由 agent 启动时从 config.env 自动生成
│       └── common_compat.sh
├── bin/                     # 编译输出目录
├── controller_node/         # 原 Python 版（参考）
└── agent_node/              # 原 Python 版（参考）
```

## 技术栈

Go 1.21+，纯标准库（`net/http`、`encoding/json`、`os/exec` 等），零外部依赖。
Bash 脚本通过 `//go:embed` 在编译时内嵌到 agent 二进制中，部署时只需单个可执行文件。
CGO_ENABLED=0 静态编译，跨平台直接运行。

## 编译

```bash
# Linux/macOS
chmod +x build.sh && ./build.sh

# Windows PowerShell
powershell -File build.ps1
```

一键编译所有目标，输出到 `bin/`，每个目标架构两个文件：
- `controller-{os}-{arch}[.exe]`
- `agent-{os}-{arch}[.exe]`

支持目标：linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64

## 配置

**统一配置文件** `config.env`（key=value 格式），controller 和 agent 共享：
- 巡检阈值、进程列表、心跳间隔等都在此文件配置
- agent 启动时会自动从 `config.env` 生成 `check_config.sh`（释放到临时目录）供 Bash 脚本 source

修改配置后重启对应程序即可生效。

## 启动

```bash
# 主控端（默认监听 0.0.0.0:8578）
./controller-linux-amd64

# 代理端（只需一个文件，脚本已内嵌）
./agent-linux-amd64
```

**注意**：controller 需 `dashboard.html` 在运行目录或上级目录下；agent 需 `config.env` 在运行目录或上级目录下。

## 端口默认值一致性问题

controller/main.go 默认端口 8578 与 config.env 保持同步——统一从 config.env 读取，不再有 Python 版的硬编码不一致问题。

## 关键 API

| 路径 | 方法 | 说明 |
|------|------|------|
| `/` | GET | 仪表盘页面 |
| `/api/status` | GET | 所有 agent 状态（JSON） |
| `/heartbeat` | POST | agent 上报心跳，返回待执行命令 |
| `/report` | POST | agent 上报巡检结果 |
| `/trigger-check` | POST | 手动触发指定 agent 巡检 |

手动触发：
```bash
curl -X POST http://<controller>:8578/trigger-check -H 'Content-Type: application/json' -d '{"agent_id": "主机名"}'
```

## 巡检脚本

按固定顺序执行：`system_check.sh` → `security_baseline_check.sh` → `process_check.sh`

check_config.sh 由 agent 在启动时从 config.env 自动生成，修改巡检参数请编辑 config.env。Bash 脚本通过 `source check_config.sh` 共享配置。

## 环境变量

config.env 中的配置项对应原有环境变量：

| 变量 | 默认值 | 位置 |
|------|--------|------|
| `CONTROLLER_PORT` | 8578 | controller |
| `CONTROLLER_BASE_URL` | `http://127.0.0.1:8578` | agent |
| `HEARTBEAT_INTERVAL_SECONDS` | 15 | agent |
| `CHECK_EVERY_N_HEARTBEATS` | 40 | agent |
| `PROCESS_NAME` | sshd | agent |
| `ALIVE_TIMEOUT_SECONDS` | 35 | controller |

添加监控进程：修改 `config.env` 中 `PROCESS_TARGETS` 的值。

## 归档

巡检原始输出保存到 `report_archive/<agent_id>/`，文件名 `YYYYmmdd_HHMMSS_<run_id>_<script>.log`。

## 测试

无测试框架，无测试代码。

## 已知问题

- `system_check.sh` 等脚本开头有 `set -u`，macOS 上因 `common_compat.sh` 触发 `unbound variable`（非目标平台，不修）
- 仪表盘每 5 秒自动刷新
