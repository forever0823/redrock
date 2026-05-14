# CHANGELOG

## 2026-05-14 - controller 日志目录调整

### 修改
- `start_controller.sh` 默认将主控程序运行日志写入 `log/controller.log`。
- 新增 `CONTROLLER_LOG_DIR` 环境变量，可自定义日志目录。
- `logs` 命令会自动创建日志目录。
- `.gitignore` 忽略 `log/`、`bash_check/log/` 和 `*.pid`，避免运行产物进入版本管理。

### 修改文件
| 文件 | 修改内容 |
|------|----------|
| `start_controller.sh` | 默认日志路径改为 `log/controller.log` |
| `.gitignore` | 忽略日志目录和 pid 文件 |
| `CHANGELOG.md` | 记录本次日志目录调整 |

---

## 2026-05-14 - 新增 controller 启动管理脚本

### 功能
- 新增 `start_controller.sh`，用于 Ubuntu 主控机管理 controller 进程。
- 支持 `start`、`stop`、`restart`、`status`、`logs` 命令。
- 启动时自动写入 `controller.log` 和 `controller.pid`，方便查看主控程序运行日志和进程状态。
- 支持通过 `CONTROLLER_BIN`、`CONTROLLER_LOG`、`CONTROLLER_PID` 环境变量覆盖默认路径。

### 修改文件
| 文件 | 修改内容 |
|------|----------|
| `start_controller.sh` | 新增 controller 启动/停止/状态/日志管理脚本 |
| `CHANGELOG.md` | 记录本次启动脚本新增 |

---

## 2026-05-13 - 修复安全详情抽屉长日志溢出

### 修复
- 修复“查看详情”抽屉中长路径、挂载参数、归档路径等 monospace 日志行不换行，导致内容横向溢出并覆盖页面的问题。
- 详情抽屉宽度改为响应式 `min(760px, calc(100vw - 24px))`，并限制横向溢出。
- `.mono` 和 `.kv` 增加长文本自动换行能力，适配新增安全基线的大量路径类输出。

### 修改文件
| 文件 | 修改内容 |
|------|----------|
| `dashboard.html` | 调整详情抽屉、日志块和键值行 CSS，防止长文本撑破布局 |
| `CHANGELOG.md` | 记录本次 UI 修复 |

---

## 2026-05-13 - 前端适配新增安全基线指标

### 功能
- controller 的 `extractStructuredMetrics()` 新增安全基线结构化字段，覆盖本次扩展的账户、SSH、密码/PAM/sudo、文件权限、挂载、内核、网络、防火墙、审计、时间同步、暴露面和持久化风险。
- dashboard 安全详情抽屉新增分组展示：账户与身份认证、SSH 远程访问、密码/PAM/sudo、关键文件/目录/挂载、内核与系统加固、网络与防火墙、审计/日志/时间、服务暴露与持久化。
- agent 卡片新增安全风险摘要，展示风险项数量、权限异常、ROOT 登录、防火墙、PAM 复杂度、审计规则、时间同步和 PATH 风险。
- 安全详情的 `result_lines` 展示上限从 200 提升到 500，适配扩展后的基线输出。

### 修改文件
| 文件 | 修改内容 |
|------|----------|
| `controller/api_report.go` | 新增安全风险字段提取、风险计数、异常行收集，并提升详情结果行上限 |
| `dashboard.html` | 新增 badge/分组 UI 和安全指标详情展示逻辑 |
| `CHANGELOG.md` | 记录本次前端适配 |

---

## 2026-05-13 - 安全基线深度增强：参考青藤云合规基线与 Wazuh SCA

### 背景
- 参考青藤云主机安全/合规基线的等保、CIS、自定义基线巡检思路，以及 Wazuh SCA 按策略检查文件、进程、配置项和系统状态的实现方式。
- 目标是把原本偏基础的 Linux 安全基线扩展为更接近主机安全产品的多维度巡检脚本，同时保留现有 `[INFO]` / `[WARNING]` / `[ERROR]` 输出格式，兼容现有报告解析。

### 新增/增强检查类别
| 类别 | 新增能力 |
|------|----------|
| 账户与身份认证 | 空密码、UID0、重复 UID/GID/用户名/组名、可登录系统账户、root 锁定状态、账户密码最长使用期、到期提醒、登录用户家目录存在性/属主/other 写权限 |
| SSH 远程访问 | 优先读取 `sshd -T` 有效配置；扩展检查 root 登录、密码登录、空密码、公钥认证、HostbasedAuthentication、IgnoreRhosts、PermitUserEnvironment、X11/TCP/Agent 转发、GatewayPorts、PermitTunnel、UsePAM、交互式认证、MaxAuthTries、LoginGraceTime、ClientAliveInterval/CountMax、MaxSessions、MaxStartups、Allow/Deny 用户组访问控制、弱 Cipher/MAC/KEX 算法 |
| 密码/PAM/sudo | `login.defs` 中 PASS_MAX/MIN/WARN、口令算法、UMASK；PAM 密码复杂度、失败锁定、密码历史；`pwquality` minlen/minclass/credit；sudo 免密授权、`Defaults use_pty`、sudo/wheel 管理组成员 |
| 关键文件与挂载 | `/etc/passwd`、`/etc/shadow`、备份文件、`sudoers`、SSH 配置、GRUB、SSH host key、cron 文件/目录权限；`/tmp`、`/var/tmp`、`/dev/shm` 的 nodev/nosuid/noexec；全局可写目录 sticky bit；无属主/无属组文件；非标准路径 SUID/SGID |
| 内核与系统加固 | ASLR、SUID core dump、dmesg/kptr/ptrace 限制、当前 shell core dump、SELinux/AppArmor、非常用高风险内核模块、AIDE/Tripwire/Wazuh Agent 文件完整性组件 |
| 网络与防火墙 | IPv4/IPv6 转发、SYN Cookie、ICMP 广播/伪造响应、IPv4/IPv6 redirect/source route、secure/send redirects、rp_filter、log_martians；firewalld/ufw/nftables/iptables 检测；iptables INPUT 默认策略；高风险传统端口 21/23/69/512-514 |
| 审计日志与时间 | rsyslog/syslog-ng/journald、auditd、auditctl enabled、audit 规则覆盖 passwd/shadow/sudoers/关键系统调用、安全日志文件、journald 持久化、logrotate、chronyd/ntpd/systemd-timesyncd |
| 暴露面与持久化 | telnet/rsh/rexec/rlogin/tftp/xinetd 进程、FTP 匿名访问、`/etc/ld.so.preload`、PATH 劫持风险、cron 可写脚本、authorized_keys 权限、rc.local 启动项 |

### 兼容性
- 保留原有关键输出短语，例如 `ASLR 已完全启用`、`IP 转发已开启/关闭`、`TCP SYN Cookie 已开启`、`源路由已关闭/已开启`、`root 账户已锁定`、`PermitRootLogin=yes`、`PasswordAuthentication=yes` 等，避免现有 dashboard/controller 指标提取失效。
- 文件权限检查由“必须完全等于某个权限”改为“不得高于建议权限”，降低不同发行版如 `/etc/shadow` 使用 `0640 root:shadow` 时的误报。

### 修改文件
| 文件 | 修改内容 |
|------|----------|
| `agent/scripts/security_baseline_check.sh` | 重写为函数化、多类别安全基线巡检脚本，新增 8 大类 80+ 项检查，并保留原有报告输出格式 |
| `CHANGELOG.md` | 记录本次安全基线增强内容 |

---

## 2026-05-12 — 重构：按功能拆分文件

### agent (4 文件)
| 文件 | 行数 | 内容 |
|------|------|------|
| `main.go` | 80 | embed 指令、main() 心跳主循环 |
| `config.go` | 137 | Config 结构体、loadConfig()、generateCheckConfig() |
| `scripts.go` | 133 | extractScripts()、runSingleScript()、runChecks() |
| `network.go` | 90 | localIP()、hostname()、isoNow()、postJSON()、findConfigPath()、handleCommand() |

### controller (10 文件)
| 文件 | 行数 | 内容 |
|------|------|------|
| `main.go` | 122 | main()、ServeHTTP() 路由分发 |
| `types.go` | 53 | Config、AgentState、State、Server struct |
| `config.go` | 88 | loadConfig()、mergeTargets() |
| `state.go` | 70 | loadState()、saveState()、ensureAgent() |
| `auth.go` | 79 | checkAuth()、genToken()、handleLogin()、handleChangePassword() |
| `utils.go` | 136 | nowTS()、toString()、fixMojibake()、safeText()、coalesce()、parseFloat()、sendJSON()、readJSON() |
| `api_status.go` | 66 | buildStatusPayload()、handleStatus() |
| `api_heartbeat.go` | 47 | handleHeartbeat() |
| `api_report.go` | 326 | summarizeResults()、extractDetailInfo()、extractStructuredMetrics()、archiveReport()、handleReport() |
| `api_config.go` | 99 | handleProcessConfig()、handleAgentProcessConfig()、handleTriggerCheck() |

---

## 2026-05-12 — 新功能：登录认证

### 功能
- **登录页面**：访问 `GET /` 时，未登录显示 `login.html` 登录表单，登录成功跳转仪表盘
- **Session 认证**：登录后设置 `auth_token` cookie（HttpOnly，24 小时有效），所有 Web API 需携带有效 token
- **修改密码**：登录后可修改管理员密码，修改后所有旧 session 立即失效
- **默认账号**：`admin / changeme`，在 `config.env` 中配置 `ADMIN_PASSWORD`

### API
| 路由 | 认证 | 说明 |
|------|------|------|
| `POST /api/login` | 否 | 登录，body: `{"username":"admin","password":"xxx"}` |
| `POST /api/change-password` | 是 | 修改密码，body: `{"old_password":"x","new_password":"y"}` |
| `GET /` | 是 | 未登录返回登录页，已登录返回仪表盘 |
| `GET /api/status` | 是 | 401 未登录则拒绝 |
| `POST /api/process-config` | 是 | 401 未登录则拒绝 |
| `POST /api/agent-process-config` | 是 | 401 未登录则拒绝 |
| `POST /trigger-check` | 是 | 401 未登录则拒绝 |
| `POST /heartbeat` | 否 | agent 无需认证 |
| `POST /report` | 否 | agent 无需认证 |

### 修改文件
| 文件 | 修改内容 |
|------|----------|
| `controller/main.go` | `Config` 新增 `AdminPassword`；`Server` 新增 `adminPassword`/`sessionTokens`/`loginHTML`；`checkAuth()` / `genToken()` 辅助函数；新增 `/api/login` 和 `/api/change-password` 路由；Web API 统一加认证检查 |
| `config.env` | 新增 `ADMIN_PASSWORD=changeme` |
| `login.html` | 新建登录页面（表单 + 修改密码面板） |

---

## 2026-05-12 — 新功能：安全基线指标大幅扩充

### 新增检查类别

| 类别 | 新增指标 | 来源 |
|------|---------|------|
| 账户安全 | 可登录的系统账户、root 锁定状态、重复 UID | CIS Benchmark |
| SSH 深化 | PermitEmptyPasswords、X11Forwarding、AllowTcpForwarding、PermitUserEnvironment、UsePAM、MaxAuthTries | 云服务器安全基线 |
| 系统加固 | ASLR 状态、core dump 禁用、非标准路径 SUID 文件 | CIS / 等保2.0 |
| 网络加固 | IP 转发、TCP SYN Cookie、ICMP 重定向、源路由 | CIS Benchmark |
| 审计日志 | 系统日志服务运行状态、auditd 状态、安全日志文件存在性 | 等保2.0 |

### 修改文件
| 文件 | 修改内容 |
|------|----------|
| `agent/scripts/security_baseline_check.sh` | 新增 5.系统加固、6.网络加固、7.审计日志三大节；SSH 检查从 3 项扩展到 9 项；账户检查从 2 项扩展到 5 项；新增 `ssh_check()` 复用函数 |
| `controller/main.go` | `extractStructuredMetrics` 中安全指标从 6 个布尔值扩展到 20+ 个 |
| `dashboard.html` | 卡片安全指标行扩展；详情抽屉按"账户风险/账户深度/SSH加固/SSH续/权限异常/系统加固/网络加固/审计日志"8 组展示 |

---

## 2026-05-12 — 新功能：巡检逾期告警

### 功能
- **巡检逾期检测**：agent 在每个心跳中预告下一次巡检的序号（`next_check_seq`）。controller 监控此预告，若当前心跳序号超过预告值 + 2 个容忍心跳且未收到巡检报告，则标记该 agent 为"巡检逾期"
- **报告到达自动清除**：收到巡检报告后，记录 `last_check_seq` 并清除逾期标记
- **前端展示**：卡片标题旁红色"巡检逾期"标签，最近巡检行显示逾期说明

### 数据流
```
agent seq=1 → 预告 next=40
agent seq=43 → controller: 43 > 40+2 且无报告 → overdue=true
agent report(seq=40) → controller: overdue=false, last_check_seq=40
```

### 容忍策略
容忍 2 个心跳周期（30 秒），避免巡检执行中（脚本最长 300 秒）被误报逾期。

### 修改文件
| 文件 | 修改内容 |
|------|----------|
| `agent/main.go` | 心跳包新增 `next_check_seq` 字段，预告下次巡检序号 |
| `controller/main.go` | `AgentState` 新增 `NextCheckSeq`/`LastCheckSeq`/`CheckOverdue`；`Config` 新增 `CheckEveryN`；心跳 handler 先检逾期再更新预告；报告 handler 清除逾期 |
| `dashboard.html` | 卡片标题加红色"巡检逾期"标签；最近巡检行加逾期说明 |

---

## 2026-05-12 — WARNING 加 PID + 进程名用全称

### 修改1：WARNING/ERROR 加上 PID
**效果**：WARNING 格式从 `[进程名]` 变为 `[进程全名:PID]`，例如 `[/usr/sbin/sshd -D:1320] CPU占用持续过低`

### 修改2：日志行使用进程全名而非配置短名
`check_one_pid` 中通过 `ps -p $pid -o cmd=` 获取完整命令行作为进程全名（截取前80字符）。避免配置短名 "agent" 匹配到 "agent_service"、"agent_helper" 等多个进程时无法区分。

### 前端适配
`parseProcessStatus` 的运行判定从精确匹配 `"检查进程: <name>, PID:"` 改为宽松匹配：行同时包含短名 + `", PID:"` 即判运行。

### 修改文件
| 文件 | 修改内容 |
|------|----------|
| `agent/scripts/process_check.sh` | `check_one_pid` 新增 `proc_name`（`ps -p $pid -o cmd=` 全名）；所有 log/warn/error 前缀从 `[$proc]` 改为 `[$proc_name:$pid]` |
| `dashboard.html` | `parseProcessStatus` 运行判定改为 `line.indexOf(name) !== -1 && line.indexOf(", PID:") !== -1` 宽松匹配 |

---

## 2026-05-11 — Bug 修复：WARNING 加进程名 + 前端独有列表去重

### 修复1：WARNING/ERROR 加上进程名前缀
**问题**：`process_check.sh` 输出的 `[WARNING] CPU占用持续过低（可能假死或无响应）` 没有标明是哪个进程，多进程巡检时无法区分。

**修复**：`check_one_pid` 中所有 WARNING/ERROR 加上 `[$proc]` 前缀，例如：
- `[WARNING] [sshd] CPU占用持续过低（可能假死或无响应）`
- `[ERROR] [nginx] 僵尸进程（Zombie）`
- `[WARNING] [mysqld] 文件描述符过多（可能资源泄漏），当前: 12000`

### 修复2：单机独有列表去重
**问题**：若单机独有配置的进程名与全局重复，`agentConfigBlock` 中会同时出现在全局标签（灰色）和独有标签（彩色）中，造成混淆。`addAgentTag` 也未拦截。

**修复**：
- `agentConfigBlock` 渲染时 `extraList` 过滤掉 `globalList` 中已有的项
- `addAgentTag` 检查全局列表，拒绝添加重复进程
- `saveAgentExtra` 保存时用 `Set` 去重

### 修改文件
| 文件 | 修改内容 |
|------|----------|
| `agent/scripts/process_check.sh` | `check_one_pid` 中所有 WARNING/ERROR 加上 `[$proc]` 前缀 |
| `dashboard.html` | `agentConfigBlock` 用 `globalSet` 过滤 `extraList`；`saveAgentExtra` 用 `Set` 去重；`addAgentTag` 检查全局列表 |

---

## 2026-05-11 — Bug 修复：全局配置删除失效 + 执行顺序调换

### Bug 1：全局配置删除立即被刷新回滚（竞态条件）
**根因**：`saveProcessConfig()` 内部读的是共享变量 `currentTargets.join(",")`。而 `removeTag` 在 `splice` 后、`fetch` 前，5 秒 `setInterval` 可能恰好触发 `render()` → `renderTags(data.process_targets)` 把 `currentTargets` 重置为服务器旧值，导致发给 API 的是旧数据。

**修复**：`removeTag` 和 `procInputEl` 回调中，`splice`/`push` 后立即用 `const newTargets = currentTargets.join(",")` 快照值，作为参数传给 `saveProcessConfig(newTargets)`。`saveProcessConfig` 改用参数接收而非读取共享变量。

### Bug 2：全局配置 API 完全失效（空壳 case 拦截）
**根因**：controller 的 `ServeHTTP` switch 中 `/api/process-config` 出现了**两个 case**——第一个是空壳 body（匹配后不执行任何代码），第二个才是真正的 handler。Go switch 匹配到第一个就退出，所以所有全局配置请求都被空壳吞掉、不执行任何逻辑。这也是单机配置一直正常的原因（它的 case 在空壳之后、真 handler 之前）。

**修复**：删除空壳 case。现在 `/api/agent-process-config` 和 `/api/process-config` 各有一个唯一的 handler。

### Feature：先更新配置再执行命令
agent 收到心跳响应时，处理顺序从"先 command 后 process_targets"调换为"先 process_targets 后 command"。效果：当 Web 面板同时修改进程配置 + 点"立即检测"时，一次心跳中先更新 `check_config.sh`，再立刻按最新配置执行巡检。

### 修改文件
| 文件 | 修改内容 |
|------|----------|
| `controller/main.go` | 删除重复的空壳 `case "/api/process-config"` |
| `agent/main.go` | 心跳响应处理顺序：先 process_targets 更新，后 command 执行 |
| `dashboard.html` | `saveProcessConfig` 改为参数传入；`removeTag`/回车回调在修改后立即快照 `currentTargets` |

---

## 2026-05-11 — Bug 修复：进程状态误判 + null 防护

### Bug 1：进程明明运行中却显示"未运行"
**根因**：`parseProcessStatus` 先判停（warningLines 中的"进程未运行"）再判跑（resultLines 中的 PID），且停优先于跑。若某次巡检中某进程确实停止，后续巡检即使进程恢复运行，前次 WARNING 仍可能干扰判断。

**修复**：改为 **PID 优先**——先搜 PID 信息，找到即判定运行中；仅在无 PID 时才检查"进程未运行" WARNING。
结果：只要巡检发现 PID，即使 warningLines 中有历史停止信号，也会正确显示运行中。

### Bug 2：全局配置删除进程不触发 API
**根因**：前端 `saveProcessConfig` 中 `cfgStatusEl.textContent = ...` 若 `cfgStatusEl` 为 null（DOM 元素缺失）则抛异常，后续 `fetch()` 调用被阻断。

**修复**：所有 DOM 引用（`cfgStatusEl`、`tagListEl`、`procInputEl`）加 null 守卫，API 调用不再依赖可选 DOM 元素存在。

### 修改文件
| 文件 | 修改内容 |
|------|----------|
| `dashboard.html` | `parseProcessStatus` 改为 PID 优先逻辑；`saveProcessConfig`/`renderTags`/`procInputEl` 加 null 守卫 |

---

## 2026-05-11 — 单机独有进程配置

### 功能
- **单机进程配置**：每个 agent 卡片上新增"单机独有进程配置"区域，可为该机器额外指定监控进程，不影响全局配置
- **合并逻辑**：`最终进程 = 全局列表 ∪ 单机独有列表`（去重）。修改全局配置会同步影响所有机器的基础列表，但不会覆盖单机独有部分

### 新增 API
| 路由 | 说明 |
|------|------|
| `POST /api/agent-process-config` | 设置指定 agent 的独有进程列表（`{"agent_id":"x","extra_targets":"d,e"}`） |

### 修改文件
| 文件 | 修改内容 |
|------|----------|
| `controller/main.go` | State/Server 新增 `AgentExtraTargets` 字段；`mergeTargets()` 合并函数；`POST /api/agent-process-config` 路由；心跳响应下发合并后的 `process_targets`；`buildStatusPayload` 每个 agent 返回 `extra_process_targets` 和 `process_targets` |
| `dashboard.html` | 每个 agent 卡片增加单机配置区域（全局灰色标签 + 独有可编辑 tag）；`saveAgentExtra()`/`addAgentTag()`/`removeAgentTag()` 函数；回车添加/点击×删除均即时保存 |

### 数据流
```
全局配置 (process_targets)     = a,b,c
机器 X 独有 (extra_targets)    = d
机器 X 最终 (heartbeat 下发)   = a,b,c,d  (mergeTargets 去重)
修改全局为 a,b,x → 机器 X 最终 = a,b,x,d  (c 被全局移除，d 保留)
```

---

## 2026-05-11 — Bug 修复：Web 保存失效 + 进程状态检测增强

### Bug 1：回车保存不生效
**根因**：`saveProcessConfig()` 中引用了已删除的 `document.getElementById("saveBtn")`，返回 `null`，`.disabled` 赋值直接抛异常，`fetch()` 调用从未执行。用户在输入框中回车 → 进程 tag 只更新了本地 UI → 5 秒刷新后 `renderTags` 从服务器数据重置 → 修改丢失。

**修复**：删除 `saveBtn` 相关代码，`saveProcessConfig` 直接调用 API。

### Bug 2：进程全部显示"未运行"
**根因分析**：`parseProcessStatus` 依赖 `resultLines` 中的 `检查进程: <name>, PID:` 行判断运行态。若 agent 尚未提交新巡检报告、或 `resultLines` 被截断/不完整，则所有进程被判定为 `false`（未运行）。

**修复**：改为主等策略——
- **警告中明确标记停止** → 进程停止（权威信号）
- **未标记停止，且找到 PID** → 进程运行中
- **两者均无** → 未知（尚未巡检到，灰色显示）

同时增加 `proc-unknown` CSS 类（灰色圆点），前端展示区分三种状态。

### 修改文件
| 文件 | 修改内容 |
|------|----------|
| `dashboard.html` | `saveProcessConfig` 删除 `saveBtn` 引用；`parseProcessStatus` 重写为优先判停、辅以 PID 判别、兜底未知态；新增 `proc-unknown` 样式；3 态展示（绿色=运行 / 橙色=停止 / 灰色=未检查） |

---

## 2026-05-11 — Bug 修复 + 交互优化

### Bug 修复：进程只展示 1 个的问题
**根因**：`agent` 默认 `ProcessName` 为 `"sshd"`，每次调用 `process_check.sh` 时都传入了 `sshd` 作为命令行参数。`process_check.sh` 中的 `collect_targets()` 检测到非空参数后，**仅返回该单个进程名**，完全忽略 `check_config.sh` 中的 `PROCESS_TARGETS` 列表。导致无论配置多少进程，实际只巡检 `sshd` 一个。

**修复**：
- `agent/main.go`：`ProcessName` 默认值从 `"sshd"` 改为 `""`（空字符串）
- `config.env`：`PROCESS_NAME=` 清空
- 效果：`process_check.sh` 不收到命令行参数，正确使用 `PROCESS_TARGETS` 列表

### 交互优化：回车即保存
**问题**：前端每 5 秒自动刷新，用户在进程配置面板中增删进程后如果没及时点"保存"按钮，刷新后修改会丢失。

**修复**：
- 删除"保存到所有巡检机"按钮
- 回车添加进程 → 立即调用 `POST /api/process-config` 保存
- 点击 × 删除进程 → 立即保存
- 面板 title 更新为"回车即可保存，下次巡检生效"

### 修改文件
| 文件 | 修改内容 |
|------|----------|
| `agent/main.go` | `ProcessName` 默认值改为空字符串 |
| `config.env` | `PROCESS_NAME=` 清空 |
| `dashboard.html` | 删除保存按钮；回车和删除操作立即调用 API 保存 |

---

## 2026-05-11 — Web 面板进程配置 + 进程展示增强

### 新增功能
- **Web 面板进程配置**：在仪表盘顶部新增进程监控配置面板，支持动态添加/删除进程名，保存后通过心跳下发给所有 agent，下次巡检生效
  - 新增 `POST /api/process-config` 接口（controller）
  - `GET /api/status` 返回中增加 `process_targets` 字段（controller）
  - 心跳响应 `POST /heartbeat` 返回中增加 `process_targets` 字段（controller）
  - agent 收到 `process_targets` 后自动重新生成 `check_config.sh`（agent）

- **进程展示增强**：前端按进程名逐个展示运行状态（绿色=运行中 / 橙色=未运行），解决之前只展示进程名列表不区分状态的问题

### 修改文件

| 文件 | 修改内容 |
|------|----------|
| `controller/main.go` | Config 新增 `ProcessTargets` 字段；State 新增 `ProcessTargets` 持久化字段；`/heartbeat` 响应携带 `process_targets`；`/api/status` 响应携带 `process_targets`；新增 `POST /api/process-config` 路由 |
| `agent/main.go` | 心跳成功时检测 `process_targets` 字段，值与当前不同则动态重写 `check_config.sh` |
| `dashboard.html` | 新增进程配置面板（tag 增删 + 保存按钮）；`parseProcessStatus()` 函数解析各进程运行态；卡片中展示各进程带状态指示灯的列表；进程详情中展示各进程状态 |

### 新增文件
- `CHANGELOG.md` — 项目变更记录

---

## 2026-05-06 — Go 重构初始版本

### 功能
- Python 代码全部用 Go 重写（controller + agent），纯标准库零依赖
- 统一配置文件 `config.env`（key=value 格式）
- Bash 脚本通过 `//go:embed` 编译时嵌入 agent 二进制
- 跨平台构建脚本 `build.sh` / `build.ps1` / `build.bat`
- 一键部署打包脚本 `publish.bat`
- 详细中文注释（含 Go 语法教学说明）

### 文件列表
| 文件 | 说明 |
|------|------|
| `config.env` | 统一配置（中文注释） |
| `controller/main.go` + `go.mod` | 主控端 HTTP 服务器 |
| `agent/main.go` + `go.mod` | 代理端心跳 + 巡检 |
| `agent/scripts/*.sh` | Bash 巡检脚本（编译时 embed） |
| `dashboard.html` | Web 仪表盘 |
| `build.sh` `build.ps1` `build.bat` | 跨平台编译 |
| `publish.bat` | 一键部署打包 |
| `AGENTS.md` | 项目文档 |
