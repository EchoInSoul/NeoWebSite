# NeoBlog 邮件基础设施设计（SMTP 发送能力，只做后端）

日期：2026-10-11
范围：`backend/`（Go + Gin）
状态：设计分节经用户确认（2026-10-11 对话：范围＝只做基础设施、发件方＝有自有域名、包位置＝`internal/modules/mail`、日志＝slog）；本文为存档版，其后进 writing-plans 出实施计划

修订记录（编号自 v2 直跳 v4，v3 无对应条目）：
- v1（2026-10-11，初版）：选型与结论源自同日三路并行调研（Go 库与参考实现 / 服务商 / 投递合规），关键项均以源码或官方文档复核，证据摘要见 §13。
- v2（2026-10-11，审查后修订）：两路独立审查（技术事实核验 + 计划质量审查）发现的缺陷已并入——§5 补导出哨兵 `ErrClosed` 与「`Close` 后 `Send` 恒返回它」的确定性要求、补关闭窗口语义；§10 测试缝签名与真实签名对齐、补并发冒烟用例；§12 补「`Close` 返回错误 ≠ worker 已停」；§14 第 3、5 条改为有可执行触发手段的测试取证。
- v4（2026-10-11，归位审查后修订）：两路独立只读审查 + 实测坐实——归位把真实投递测试里写死的 `../../.env` 变成了 `backend/internal/.env`（不存在），且 `LoadDotEnvFile` 对缺文件静默，导致按文档操作只会 SKIP。已改为从包目录向上查找 `.env`（`findEnvFile`，深度上限 5）并补三个用例钉住（含「从包目录能找到 backend/.env」的守护用例）；§10 第 5 条与 §3 / §14 的版本表述同步；README 补「无路由模块」的规则说明。完整过程见 DEVLOG 同日条目。
- v5（2026-10-11，管理端发信接口轮修订）：模块升级为路由模块（兑现 v4 时期「若以后要形式统一」的预案）——`New(cfg)` 改返回 `*Module`（`m.Mailer` 暴露能力接口），新增 controller（`POST /admin/mail/messages`，body `{"to"}` → 201 `{"to","subject","sent":true}`；400 `invalid_request`/`param:to`、403 `forbidden`、500 `mail_delivery_failed`，状态码归并遵守 API 规范 §4）与 `Mailer.SendSync`（同步单次尝试、不重试、拨号死线 20s——go-mail 逐命令续期非总上限，专供管理端自检即时反馈；异步 `Send` 语义不变，业务流程一律走它）；新增守卫 `guard.RequirePresetGroup`（owner/admin 白名单，**不走权限目录**——SMTP 是服务器级配置，不属于可授出权限域；自建组不可授）。用户裁定：路由命名按 API 规范 §3 资源名词复数（`/messages`，创建即发送），拒绝 `/test` 动词尾巴。版本 0.8.0 → 0.9.0。
- v6（2026-10-23，设置接入）：新增 `mail` 设置组十二项并成为 SMTP 配置的首选来源；`smtp_host` 为空时回退 `SMTP_*` 环境变量，`enabled=false` 时强制日志实现。`New(cfg)` 改为 `New(cfg, settings)`，每次投递前读取设置组；读取失败直接报错，不静默回退。前端邮件设置页从纯 UI 改为服务端读取 + `PATCH /admin/settings/mail` 保存；后端仍不提供明文 SMTP 加密模式。

## 1. 意图与背景

目标：给后端一套**可被任何模块注入的发信能力**——SMTP 真实发送、开发环境降级为日志、模板机制、重试与优雅关闭、测试与示例邮件。本轮不接任何业务用例：目前后端没有任何发信消费者；将来注册验证 / 通知出现时，在 `app.go` 里注入即可（对齐 README 硬性规则 5 的跨模块注入方式）。

已定约束（用户）：
- 只做后端；不引入新服务、不引入额外界面。
- 代码零注释、gofmt；结构对齐现有 NestJS 风格约定。
- 发送走标准 SMTP 协议（不用各家专有 SDK）：将来换服务商只改环境变量，不动代码。
- 用户有自有域名，发件域名可配 SPF/DKIM/DMARC；域验类服务商自己签 DKIM，应用内不做签名。

对齐来源（不重新发明）：
- 2026-10-11 调研结论（三路独立子代理：库与参考实现、服务商、投递合规；均要求逐条附出处，关键项由主会话抽查源码复核）。本 spec 已把结论固定下来，证据摘要见 §13。
- `backend/README.md` 硬性规则 1–8；`docs/superpowers/specs/2026-10-11-neoblog-auth-design.md` 的 spec → plan 流程、测试与文档约定。

## 2. 已定决策

| 项 | 决定 | 落地时点 |
| --- | --- | --- |
| 库 | `github.com/wneessen/go-mail` v0.8.1（2026-07-09 发布；依赖仅 `golang.org/x/crypto` + `x/text`，均已在依赖树） | 本轮 |
| 包位置 | `internal/modules/mail/`——按 README 硬性规则 1（一个功能一个模块目录）归入 modules；初版无路由（不含 controller / dto / Register），0.9.0（v5）升级为路由模块（module + controller，`RegisterProtected` 挂守卫链），对外入口 `New(cfg) *Module`，能力接口经 `m.Mailer` 暴露 | 本轮；v5 升级 |
| 对外形状 | 导出 `Mailer` 接口 + `Message` / `Body`；`New(cfg) *Module`（能力接口经 `m.Mailer` 暴露）、`Render(name, data)`；业务代码不 import go-mail | 本轮；v5 修订 |
| 投递模型 | 内存队列（缓冲 channel，容量 32）+ 单 worker 串行投递（一次只有一个 SMTP 连接）；**不做持久化 outbox** | 本轮 |
| 重试 | 每封最多 3 次尝试（首次 + 2 次重试），退避 1s / 5s，单次投递拨号死线 20s（go-mail 对每条 SMTP 命令续期，非总上限）；**服务端 4xx 或传输层错误重试，服务端 5xx 不重试**（判定依据见 §13） | 本轮 |
| 降级 | `SMTP_HOST` 为空 → log sender（把邮件写日志），启动不报错——这是 fail-fast 原则的唯一例外，开发机无凭据也能全流程跑通 | 本轮 |
| AUTH | go-mail `SMTPAuthAutoDiscover`（源码实证：优先列表不含 XOAUTH2；未加密连接只允许不裸传密码的机制）；用户名为空则 `SMTPAuthNoAuth`；不提供手工机制开关 | 本轮 |
| TLS | `SMTP_TLS=starttls`（默认，587）或 `ssl`（465）；一律加密，不提供明文模式 | 本轮 |
| 模板 | `html/template` + `embed.FS`（`internal/modules/mail/templates/<name>.html|txt` 成对）；渲染同步、错误在调用点暴露；每次 Render 解析、不做缓存 | 本轮 |
| 日志 | 标准库 `log/slog` 默认 logger（项目首次引入 slog） | 本轮 |
| 示例邮件 | `templates/example.{html,txt}`「邮件配置自检」，兼作真实投递测试载体 | 本轮 |
| 测试 | 包内 stub（重试策略单测）+ 进程内 `github.com/emersion/go-smtp` v0.25.0（SMTP 路径真发断言，自签证书）+ 可选真实投递（`MAIL_TEST_RECIPIENT` 门控） | 本轮 |
| DKIM 应用内签名 | 不做（腾讯云 SES / 阿里云 DirectMail / Resend 均为服务商侧签名） | 换自建 MTA 时 |
| 持久化 outbox（ent 表 + worker） | 不做；触发条件＝出现「邮件丢了不可恢复」的流程 | 触发时 |
| river 任务队列 | 不引入：`InsertTx` 需要 `*sql.Tx`，而 ent 的 `*ent.Tx` 不暴露底层事务，与业务写入同事务原子入队要绕非公开 API | 不适用 |
| 退信 / 投诉 / 抑制表 | 不做 | 开始往用户发信时 |
| 连接复用 | 不做（每封 Dial + Close；go-mail 本身没有连接池） | 发信量显著上升时 |

## 3. 范围

本轮做：

- `internal/modules/mail/`：`mailer.go`（接口、队列、worker、重试）、`smtp.go`、`logsender.go`、`template.go`、`templates/example.{html,txt}`、测试文件。
- `internal/config`：新增 §8 的 7 个 `SMTP_*` 变量与校验（`MAIL_TEST_RECIPIENT` 仅测试读取，不进 `Config`）；`LoadDotEnv` 拆出 `LoadDotEnvFile(path)`（`LoadDotEnv` 变成其 `".env"` 包装，行为不变——现有实现写死读当前目录，包目录下测试用不了）；`config_test.go` 补正反用例。
- `.env.example`：追加 SMTP 段（自有域名示例，见 §8）。
- `internal/app/app.go`：装配 `mail.New(cfg)`、持有、关闭时 drain。
- `go.mod`：`github.com/wneessen/go-mail` v0.8.1（运行时）、`github.com/emersion/go-smtp` v0.25.0（仅测试）。
- README 更新（环境变量表、目录对照表加一行、新增「邮件」章节）；DEVLOG 一条；版本进位：按执行时的当前版本进位 minor（计划写作 0.2.1 → 0.3.0；实际 0.3.x 被并行落地的认证层 2 占用，落地为 0.3.1 → 0.4.0，模块归位后 patch 至 0.4.1）。

本轮不做：任何业务接入（无消费者、无 HTTP 端点）、outbox、退信处理、应用内 DKIM、List-Unsubscribe、连接复用、模板多语言、多收件人（`Message.To` 为单值）。

## 4. 数据模型

无 schema 变更、无迁移。原因：投递队列在内存（§2 已否决持久化 outbox）。

## 5. 接口

```go
package mail

type Body struct {
    Text string
    HTML string
}

type Message struct {
    To      string
    Subject string
    Body    Body
}

type Mailer interface {
    Send(ctx context.Context, msg Message) error
    SendSync(ctx context.Context, msg Message) error
    Close(ctx context.Context) error
}

func New(cfg config.Config) *Module

func Render(name string, data any) (Body, error)

var ErrClosed = errors.New("mailer closed")
```

语义（零注释风格下以本文为准）：

- `Send`：消息入队后立即返回，不等投递结果。`ctx` 只约束入队——队列满（容量 32）时阻塞等位，ctx 到期即返回其错误。**`Close` 之后调用 `Send` 必须确定性返回 `ErrClosed`**：实现上要在 `select` 前做一次非阻塞优先检查（`closing` 已关闭时「入队」与「已关闭」会同时就绪，Go 对同时就绪的分支随机选择，不做优先检查会有约一半概率静默返回 nil 而消息无人消费）。与 `Close` 真并发时，已入队的消息仍会被 drain 尽力投递；drain 判定排空退出后入队的边界消息不保证投递（关闭窗口内的在途消息）。投递失败不从这里返回，走后台重试 + 日志。
- `SendSync`（v5）：同步单次投递，不走队列、不重试；死线由实现套上（拨号 20s，go-mail 对每条 SMTP 命令续期，**非总上限**）；`Close` 之后调用必须确定性返回 `ErrClosed`，失败原样返回 transport 错误并记结构化日志。当前唯一消费方是管理端自检接口（结果即时反馈给操作者），业务流程一律走 `Send`。
- `Close`：停止接收新消息，把队列中剩余消息投完（受 ctx 限制）；**幂等**，可重复调用。
- `New`：返回 `*Module`，`SMTP_HOST` 为空 → log 实现；非空 → SMTP 实现。构造即启动 worker goroutine。
- `Render`：把 `templates/<name>.html` 与 `<name>.txt` 各渲染一份（HTML 用 `html/template`，纯文本用 `text/template`），返回 `Body`；模板不存在或执行出错即返回错误。
- 主题由调用方给出、不进模板：主题通常携带动态值（如验证码），放进模板反而绕。

用法（未来消费方的形态）：

```go
body, err := mail.Render("example", map[string]string{"Time": time.Now().Format(time.RFC3339)})
if err != nil {
    return err
}
return mailer.Send(ctx, mail.Message{To: addr, Subject: "NeoBlog 邮件配置自检", Body: body})
```

## 6. 装配与生命周期

- `app.New`：`mailModule := mail.New(cfg)`；`mailModule.Mailer` 存入 `App` 字段，`mailModule.RegisterProtected(protected)` 挂守卫链（v5）。
- `app.Run` 关闭路径：`server.Shutdown(ctx)` 之后调用 `mailer.Close(ctx)`，共用同一条 10s `shutdownTimeout` 预算；若 HTTP 关闭已耗尽预算，`Close` 拿到已到期 ctx 便立即返回（在途邮件放弃，可接受）。注意 `SendSync` 的拨号死线（20s）大于关停预算：SIGTERM 撞上在途自检请求时 SMTP 事务会被硬切。
- v5 起管理端自检接口是第一个消费方（controller 经注入的 `Mailer` 调 `SendSync`）；后续业务消费方一律在 `app.go` 注入（规则 5），例如 `auth.New(db, sessions, mailer)`。
- 两个方向约束：业务模块禁止自建 SMTP 客户端/连接（类比规则 7）；`internal/modules/mail` 不得 import 任何业务模块（类比规则 8）。

## 7. 模板机制与示例邮件

- 目录 `internal/modules/mail/templates/`，命名规则：`<name>.html` 与 `<name>.txt` 必须成对（HTML 必须同时提供纯文本版）。
- 加载：`//go:embed templates/*.html templates/*.txt` + `template.ParseFS`；每次 `Render` 时解析（每天几封的量级，不做缓存，也就没有缓存失效问题）。
- 转义：HTML 侧依赖 `html/template` 自动转义（测试覆盖 `<script>` 注入被转义）。
- 示例 `example`：主题「NeoBlog 邮件配置自检」，正文含渲染时间与一句说明；同时作为 §10 真实投递用例的载体。

## 8. 配置

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `SMTP_HOST` | 空 | **空 = 不报错**，降级为 log sender |
| `SMTP_PORT` | `587` | 正整数 |
| `SMTP_TLS` | `starttls` | `starttls`（配 587）或 `ssl`（配 465） |
| `SMTP_USERNAME` | 空 | 空 = 不做 AUTH（自建中继场景）；非空 = AUTH 自动协商 |
| `SMTP_PASSWORD` | 空 | |
| `SMTP_FROM` | 无 | `SMTP_HOST` 非空时必填 |
| `SMTP_FROM_NAME` | `NeoBlog` | 发件人显示名 |
| `MAIL_TEST_RECIPIENT` | 空 | **不进 `Config`**：由测试直接从进程环境读取（§10 第 5 条）；非空且 SMTP_* 齐备时真实投递用例才运行，否则 skip |

校验（fail-fast，沿用 `config.go` 现有 `getEnv*` 风格）：

- `SMTP_HOST` 为空时，以上全部跳过（不校验、不报错）。
- `SMTP_HOST` 非空时：`SMTP_FROM` 必填；`SMTP_USERNAME` 非空 ⇒ `SMTP_PASSWORD` 必填；`SMTP_PORT` 为正整数；`SMTP_TLS` ∈ {`starttls`, `ssl`}。
- log 模式打印用的 From：`SMTP_FROM` 未配置时显示 `noreply@localhost`。

客户端映射（Option 名已逐一对照 v0.8.1 源码，client.go:359–859）：`mail.NewClient(host, opts...)`，opts ＝ `WithPort(port)` ＋ `starttls` → `WithTLSPortPolicy(mail.TLSMandatory)` / `ssl` → `WithSSLPort(false)` ＋ `WithUsername` / `WithPassword` ＋（用户名非空 → `WithSMTPAuth(mail.SMTPAuthAutoDiscover)`，否则 `WithSMTPAuth(mail.SMTPAuthNoAuth)`）。消息侧：`Msg.FromFormat(fromName, fromAddr)`、`Msg.To(to)`、`Msg.Subject(subject)`、`msg.SetBodyString(mail.TypeTextPlain, text)` + `msg.AddAlternativeString(mail.TypeTextHTML, html)`（常量见 encoding.go:170/173）。测试注入自签证书用 `WithTLSConfig`。

`.env.example` 追加（示例值按自有域名给，站名占位 example.com）：

```dotenv
# 邮件（SMTP）。不配置 SMTP_HOST 时邮件只写日志，不影响启动。
# 腾讯云 SES：smtp.qcloudmail.com，465 隐式 TLS（SMTP_TLS=ssl）或 587 STARTTLS
# SMTP_HOST=smtp.qcloudmail.com
# SMTP_PORT=465
# SMTP_TLS=ssl
# SMTP_USERNAME=noreply@example.com
# SMTP_PASSWORD=
# SMTP_FROM=noreply@example.com
# SMTP_FROM_NAME=NeoBlog
# 阿里云邮件推送对照：smtpdm.aliyun.com，465（SSL）或 80（STARTTLS）
# 真实投递测试用：非空且 SMTP_* 齐备时，测试会真发一封 example 邮件过去
# MAIL_TEST_RECIPIENT=
```

## 9. 日志

- 实现：`log/slog` 默认 logger（stderr）。
- log sender：每次发送打 Info，字段 `to` / `from` / `subject`，并附纯文本正文——开发环境从日志里就能拿到邮件全文。
- worker：投递失败打 Error（字段 `to` / `subject` / `attempt` / `err`，err 里含 `SendError` 的 `ErrorCode()` / `EnhancedStatusCode()` 可得信息）；进入重试等待打 Warn（字段 `attempt` / `backoff`）。
- 不打印 `SMTP_PASSWORD`；go-mail 的 `WithLogAuthData` 不开启。

## 10. 测试

1. **重试策略单测（无外部依赖）**：包内 stub transport 覆盖四种情形——4xx 后下次成功、5xx 不重试、传输层错误（`ErrorCode()==0`）重试、重试耗尽后记录失败日志；外加 `Close` 幂等与「并发 `Send`/`Close` 不 panic」两条冒烟用例。
2. **模板单测**：`Render` 缺模板名报错；`<script>` 注入被转义；text / html 两份分别由对应引擎渲染。
3. **SMTP 路径集成（进程内，不依赖 Docker / 外网）**：`emersion/go-smtp` 起假 server——实现 `Backend` / `Session` 两接口，`net.Listen("tcp","127.0.0.1:0")` + `Server.Serve`，server 侧 `TLSConfig` 挂测试自签证书以暴露 STARTTLS（v0.25.0 已核对接口）。smtpTransport 经客户端 `WithTLSConfig`（跳过自签校验）真发一封，断言收件人、双版本正文、以及**中文主题的 RFC 2047 编码**可被正确解出。
4. **config 单测**：§8 校验规则的正反用例。
5. **可选真实投递**：测试先从进程环境读取；若 `SMTP_HOST` 为空，从包目录向上逐级查找 `.env`（`findEnvFile`，深度上限 5），找到即用 `config.LoadDotEnvFile` 载入后再读一次——**不写死相对深度**（初版写死 `../../.env`，归位到 `internal/modules/mail` 后指向 `backend/internal/.env` 且静默失效，见 v4）。`SMTP_HOST` 与 `MAIL_TEST_RECIPIENT` 都非空时，用测试自建的 `config.Config` 字面量真发一封 `example` 到该地址（不用 `config.Load`——它要求 `DATABASE_URL`）；任一为空则 skip，skip 文案点明「environment or backend/.env」（沿用 `DATABASE_URL_TEST` 的缺省 skip 约定）。

测试缝（对齐 README 规则 5 的「为测试提供的包级入口」例外，业务代码不得调用）：包内 `newMailer(transport, logger)` 与可注入 `WithTLSConfig` 的 transport 构造入口，供测试构造 stub / 自签证书；不新增导出 API（`ErrClosed` 是 §5 的接口语义，不属测试缝）。

## 11. 部署前置（DNS 与服务商，非代码）

自有域名的最小合规动作（本项目量级不需要企业级方案；出处见 §13）：

1. **SPF**：按服务商控制台给出的 include 值添加，形如 `@ TXT "v=spf1 include:<服务商给的域> -all"`；同一域名只能有一条 SPF 记录。
2. **DKIM**：服务商控制台生成密钥后，按其给的主机名与值添加 TXT（2048 位、rsa-sha256）；应用内不签名。
3. **DMARC**：`_dmarc TXT "v=DMARC1; p=none; rua=mailto:dmarc@<你的域名>"`；先观察报告数月再收紧到 quarantine / reject。
4. **服务商选择**：腾讯云 SES（1000 封免费、不限有效期、仅发信不强制备案）为首选；阿里云 DirectMail（2000 封免费、每天免 200 封，官方明文「触发类邮件只能通过 API 或 SMTP 发送」）为对照；端点在 §8。
5. **验证闭环**：每改一次 DNS，用 mail-tester 发一封自测 + MXToolbox 查 SPF/DKIM/DMARC 与黑名单；再往 Gmail / QQ 各发一封，看原始头三项是否 pass。注意 QQ 对域名 DNS 记录最长缓存 12 小时，改完别急着判定失败。
6. 纯事务邮件（验证 / 重置）**不加** List-Unsubscribe；出现订阅类内容时再加（RFC 8058）。

## 12. 已知缺口与风险

- **内存队列无持久化**：进程崩溃或重启会丢在途邮件；`Send` 只保证入队。恢复路径＝业务侧提供「重发」。升级触发条件见 §2。
- **`Close` 返回错误 ≠ worker 已停**：退避等待不响应 `Close`，worker 最坏会继续跑完在途投递与 2 次退避（上界约 3×20s + 1s + 5s）后自行退出；无无界泄漏，进程退出场景等价于「在途邮件放弃」。
- **传输层错误一律重试的代价**：配置类错误（服务器不支持 STARTTLS、密码错等）也会白重试 2 次再记日志。这是刻意取舍——go-mail 把网络类失败判为「永久」而 4xx 才可重试，若只信 `IsTemp()`，一次网络抖动就会丢信。
- **每封一次 Dial**：无连接池与复用；QQ/163 类服务商对连接频率有限制（如 `550 Connection frequency limited`），本项目量级远低于阈值，量上来再按 §15 改造。
- **模板不缓存**：每封解析两个小文件（微秒级），可忽略；不要提前优化。
- **slog 为项目首次引入**：与 gin 自带的 logger 并存（gin 的仍在），统一日志层留待未来。
- **无退信感知**：发失败只进日志、不通知任何人；本轮无消费者所以风险为零，开始往用户发信时按 §15 补。
- 新增依赖后 `go mod tidy` 应只引入 go-mail 与（测试用）go-smtp 及其极少传递依赖，若出现意外传递依赖需人工确认（go-mail 仅依赖 x/crypto、x/text）。

## 13. 调研证据摘要（实现与评审时复核用）

- **go-mail v0.8.1 源码**（本地 `F:\software\gopath\pkg\mod\github.com\wneessen\go-mail@v0.8.1`）：`DialAndSendWithContext`（client.go:1375）；`SendError.IsTemp()` / `EnhancedStatusCode()` / `ErrorCode()`（senderror.go:147/196/211）；`isTempError` 只认错误首字符 `'4'`、`errorCode` 仅解析 4xx/5xx（senderror.go:264 起）——这是 §2 重试判定的依据；`authTypeAutoDiscover` 优先列表（client.go:1592）与 `auth()` 的未加密限制（client.go:1488）；Option 全表（client.go:359–859）；`TypeTextHTML` / `TypeTextPlain`（encoding.go:170/173）。
- **各家发布状态**（2026-10-11 用 `go list -m -json <mod>@latest`，GOPROXY=goproxy.cn 实测）：go-mail v0.8.1 / 2026-07-09；`jordan-wright/email` 停在 2021-01（无 context / DKIM）；`gopkg.in/gomail.v2` 停在 2016；标准库 `net/smtp` 本地源码包文档写明 "frozen and is not accepting new features"；river v0.49.0（活跃，但 §2 已否决）；`emersion/go-smtp` v0.25.0；`axllent/mailpit` v1.31.5（本地可视化收信，见 §15）。
- **参考实现**：Gitea `services/mailer`（Sender 接口 + DummySender + 异步队列，失败仅记日志）与 Vikunja `pkg/mail`（缓冲 channel + 单 daemon + 失败仅记日志）——两者都不做重试与持久化，印证 §2 的「不上 outbox」。
- **go-smtp v0.25.0 源码**（本地 `F:\software\gopath\pkg\mod\github.com\emersion\go-smtp@v0.25.0`）：`Backend` / `Session` 接口（backend.go:33/51）、`NewServer` / `Serve` / `Shutdown`（server.go:94/107/293）、`Server.TLSConfig` 字段——§10 第 3 条测试方案的依据；依赖仅 `go-sasl`。
- **服务商与合规**（官方文档 / RFC 实际抓取）：腾讯云 SES 额度、端口与备案口径（cloud.tencent.com/document/product/1288/47930、/55127、/65750、/52776）；阿里云 DirectMail（help.aliyun.com/zh/direct-mail/smtp-endpoints、product-overview/limits、getting-started/product-rules）；RFC 7208（SPF）/ 6376（DKIM）/ 7489（DMARC）/ 8301（密钥）/ 8058（一键退订）；QQ 邮箱群发指南与他域互通平台（help.mail.qq.com/detail/0/1042、/detail/110/1076）。
- **Gmail / Yahoo 批量发件人规则**：官方站点当时从用户网络不可达，相关数字为二手（Postmark / Mailgun 对官方公告的引述）；本项目量级不触发批量门槛（5000 封/天），仅作背景。

## 14. 验收标准

1. `go build ./... && go vet ./... && gofmt -l .` 干净；零人工注释。
2. `go test ./...` 全绿，且**不依赖外网与 Docker**（stub 单测 + 进程内 go-smtp 集成；`MAIL_TEST_RECIPIENT` 未设置时真实投递用例 skip；`DATABASE_URL_TEST` 未设置时既有集成用例照旧 skip）。
3. 日志降级（空 `SMTP_HOST`）取证：`go test ./internal/modules/mail/ -run 'TestLogSenderWritesMailContent|TestNewSelectsLogTransportWhenHostEmpty' -v` 通过——日志里出现邮件全文与 `to` / `from` / `subject` 字段（本轮无消费者、无 HTTP 端点，故以测试取证代替人工触发）。
4. 真实投递（用户手动）：`.env` 填真实 SMTP + `MAIL_TEST_RECIPIENT`，`go test ./internal/modules/mail/ -run TestRealDelivery -v`，收件箱收到「邮件配置自检」。
5. 投递失败行为取证：`go test ./internal/modules/mail/ -run 'TestDialFailureIsTemporary|TestTemporaryErrorIsRetried|TestGivesUpAfterMaxAttempts' -v` 通过（不可达服务器判为可重试、有界重试后放弃并记日志）；`go test ./internal/app/ -run TestShutdownClosesMailer -v`（需 `DATABASE_URL_TEST`）通过。
6. README 更新完成；DEVLOG 一条；版本按执行时当前版本进位 minor（实际 0.3.1 → 0.4.0，归位 patch 至 0.4.1）。

## 15. 插槽（本轮不定，定它的时点）

| 插槽 | 什么时候必须定 | 影响面 |
| --- | --- | --- |
| 第一个真实消费方（注册验证 / 通知） | 各自立 spec 时 | auth 模块、`app.go` 注入 |
| 本地可视化收信（`axllent/mailpit` 加进 compose，1025 收 / 8025 看，带 HTTP API） | 出现第一个真实消费方、需要肉眼看信时 | docker-compose、测试断言方式 |
| 持久化 outbox 与投递重试持久化 | 出现「邮件丢了不可恢复」的流程时 | ent schema、Atlas 迁移、`internal/modules/mail` 改造 |
| 多收件人 / CC / Reply-To | 出现通知多人的需求时 | `Message` 结构 |
| 模板多语言 | 出现非中文收件人时 | templates 目录结构、`Render` 签名 |
| 应用内 DKIM 签名 | 换自建 MTA 时 | smtp.go（消息级 `Msg.SetDKIM` 或客户端级 `WithAlwaysDKIMSign`，签名器 `NewDKIMSigner`） |
| 退信 / 投诉处理与抑制表 | 开始往用户发信后 | 新表、服务商 webhook 或日志解析 |
| 连接复用（Dial + Send + Close 手动管理） | 发信量显著上升时 | smtp.go |

## 16. 下一步

本文经用户审阅通过后，进 writing-plans 出实施计划（范围 = §3，验收 = §14）。
