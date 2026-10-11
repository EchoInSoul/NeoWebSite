# NeoBlog 邮件基础设施实施计划（`internal/modules/mail`）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给后端一套可被注入的发信能力——SMTP 真实发送、开发环境降级为日志、模板机制、重试与优雅关闭、测试与示例邮件。

**Architecture:** 新增模块 `internal/modules/mail`（无路由，属被注入的能力；对外入口 `New(cfg) Mailer`，不含 controller / dto）：导出 `Mailer` 接口 + `Message` / `Body` + `New(cfg)` + `Render(name, data)`；内部是「缓冲 channel 队列 + 单 worker 串行投递 + 有界重试」；两个 transport 实现——SMTP（`wneessen/go-mail`）与日志（开发降级）。`SMTP_HOST` 为空即降级为日志、不报错。装配只在 `app.go`，关闭时 drain。

**Tech Stack:** Go 1.27 + `github.com/wneessen/go-mail` v0.8.1（运行时唯一新增依赖）、`github.com/emersion/go-smtp` v0.25.0（仅测试用假 SMTP 服务器）、`log/slog`（标准库）。

**Spec:** `docs/superpowers/specs/2026-10-11-neoblog-mail-design.md`

> **实施后修订（2026-10-11）：** 包位置由 `internal/mail/` 改为 `internal/modules/mail/`（用户裁定，对齐 README 硬性规则 1）；本文件各任务 Files 块中的路径已同步，其余照原样执行。
> **实施后修订 2（2026-10-11，归位审查）：** 归位使 `realdelivery_test.go` 写死的 `../../.env` 失效（且 `LoadDotEnvFile` 缺文件静默）——已改为从包目录向上查找 `.env`（`findEnvFile`，深度上限 5）+ 三个守护用例；Task 4 Step 5 的描述已同步。实际版本：0.3.1 → 0.4.0（minor）→ 0.4.1（归位 patch）。

## Global Constraints

- 手写 Go 文件**零注释**（`//go:embed`、`//go:build` 这类工具链指令豁免）；`gofmt -l .` 必须为空。
- 业务模块禁止自建 SMTP 客户端（对齐硬性规则 7 的类比）；`internal/modules/mail` 不得 import 其它业务模块（对齐规则 8 的类比）。
- 本机 Git Bash 里先执行：`export PATH="/f/software/go/bin:/f/software/gopath/bin:$PATH" GOPATH='F:/software/gopath' GOCACHE='F:/software/gocache' GOPROXY='https://goproxy.cn,direct'`
- 本计划的测试**不需要 Docker 与数据库**（唯一例外：Task 5 的 `TestShutdownClosesMailer` 用测试库，沿用 `DATABASE_URL_TEST` 缺省 skip 的既有约定）。
- 提交信息中文、conventional 前缀（`feat(backend): ...` / `docs(backend): ...` / `chore(backend): ...`），每个任务一次提交；不提交 `backend/.env`。
- 计划整体完成时执行 `./scripts/bump-version.sh minor`（计划写作 0.2.1 → 0.3.0；实际按执行时版本：0.3.1 → 0.4.0，归位后 patch 0.4.1）。
- 重试与容量的默认值（规范）：队列 32、每封最多 3 次尝试、退避 1s / 5s、单次投递 20s 超时。

## Review Focus

1. **传输层错误必须可重试**：连接被拒、超时这类错误在 go-mail 里 `IsTemp()` 为 false、`ErrorCode()` 为 0；若实现只信 `IsTemp()`，一次网络抖动就丢信——T3 的分类用例覆盖（5xx 永久 / 451 临时 / 连接被拒临时）。
2. **`Send` 在队列满与关闭时的行为**：`Close` 返回后 `Send` 必须确定性返回 `ErrClosed`（同时就绪分支的随机选择不得让它静默丢信）；队列满时阻塞等位不丢消息；并发 `Send`/`Close` 不 panic——T2 覆盖。
3. **中文主题的 RFC 2047 编码**：主题里有中文时收件方必须能正确解出——T3 集成用例在假服务器侧用 `mime.WordDecoder` 断言。
4. **log 模式不得因 SMTP 配置缺失或非法而启动失败**（fail-fast 的唯一例外），且邮件全文进日志——T1 / T2 覆盖。
5. **HTML 模板注入**：模板数据里的 `<script>` 必须被转义——T4 覆盖。

---

### Task 1: 配置项与 dotenv 指定路径加载

**Files:**
- Modify: `backend/internal/config/config.go`
- Modify: `backend/internal/config/dotenv.go`
- Modify: `backend/internal/config/config_test.go`
- Modify: `backend/internal/config/dotenv_test.go`
- Modify: `backend/.env.example`

**Interfaces:**
- Produces:
  - `type SMTPTLSMode string`；`SMTPTLSStartTLS SMTPTLSMode = "starttls"`、`SMTPTLSSSL SMTPTLSMode = "ssl"`
  - `Config` 新增字段：`SMTPHost string`、`SMTPPort int`、`SMTPTLS SMTPTLSMode`、`SMTPUsername string`、`SMTPPassword string`、`SMTPFrom string`、`SMTPFromName string`
  - `func LoadDotEnvFile(path string) error`（文件不存在返回 nil）；`LoadDotEnv()` 改为 `LoadDotEnvFile(".env")`，**行为不变**

- [ ] **Step 1: 写失败的配置测试**

`config_test.go` 沿用既有写法（`t.Setenv`）新增：缺省值（Port 587 / TLS starttls / FromName "NeoBlog"、Host、Username、From 为空）；`SMTP_HOST` 为空时 `SMTP_PORT=abc`、`SMTP_TLS=bogus` 仍不报错（跳过校验）；`SMTP_HOST` 非空但 `SMTP_FROM` 空 → 报错；`SMTP_HOST` + `SMTP_USERNAME` 非空而 `SMTP_PASSWORD` 空 → 报错；`SMTP_HOST` + `SMTP_PORT=abc` 或 `SMTP_PORT=70000` → 报错（上限 65535，越界会让 `gomail.WithPort` 直接失败）；`SMTP_HOST` + `SMTP_TLS=bogus` → 报错、`SMTP_TLS=ssl` → `cfg.SMTPTLS == SMTPTLSSSL`。
`dotenv_test.go` 新增 `LoadDotEnvFile`：临时文件写入 `MAILTEST_KEY=value` → 调用后 `os.Getenv` 命中（`t.Cleanup` 里 `os.Unsetenv`）；不存在的路径 → 返回 nil。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/config/ -count=1`
Expected: 编译失败（字段与 `LoadDotEnvFile` 未定义）。

- [ ] **Step 3: 实现**

`config.go`：新增字段与常量；`Load()` 里按 spec §8 装配——`SMTP_HOST` 为空时跳过其余全部校验；非空时校验 `SMTP_FROM` 必填、`SMTP_USERNAME` 非空 ⇒ `SMTP_PASSWORD` 必填、`SMTP_PORT` 用既有 `getEnvInt`（正整数）并在其后加 `> 65535` 报错（`WithPort` 对越界端口会让 `NewClient` 立即失败，必须挡在配置层）、`SMTP_TLS` 必须在枚举内（非法值报错，报错文案含非法值本身）。
`dotenv.go`：抽出 `LoadDotEnvFile(path string) error`（原 `LoadDotEnv` 函数体，`os.ReadFile(path)`），`LoadDotEnv()` 变成一行包装。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/config/ -count=1`，Expected: PASS（含既有用例）。

- [ ] **Step 5: 追加 `.env.example`**

按 spec §8 的代码块逐行追加（腾讯云 SES 为主示例、阿里云 DirectMail 注释对照、`MAIL_TEST_RECIPIENT` 说明）。

- [ ] **Step 6: Commit**

```bash
git add backend/internal/config backend/.env.example
git commit -m "feat(backend): 邮件 SMTP 配置项与 dotenv 指定路径加载"
```

---

### Task 2: 邮件核心（接口 / 队列 / worker / 重试）与日志实现

**Files:**
- Create: `backend/internal/modules/mail/mailer.go`
- Create: `backend/internal/modules/mail/logsender.go`
- Create: `backend/internal/modules/mail/mailer_test.go`
- Create: `backend/internal/modules/mail/logsender_test.go`

**Interfaces:**
- Consumes: Task 1 的 `config.Config` 字段。
- Produces（导出）：
  ```go
  type Body struct { Text string; HTML string }
  type Message struct { To string; Subject string; Body Body }
  type Mailer interface {
      Send(ctx context.Context, msg Message) error
      Close(ctx context.Context) error
  }
  var ErrClosed = errors.New("mailer closed")
  ```
  （`ErrClosed` 是 spec §5「Close 后停止接收新消息」的落地细化：`Send` 在 `Close` 之后返回它，而不是 panic。）
- Produces（包内）：
  - `type transport interface { deliver(ctx context.Context, msg Message) error }`
  - `type deliveryError struct { err error; temporary bool }`（`Error()` 返回 `err.Error()`，并实现 `Unwrap() error`——否则 `errors.Is/As` 穿不透，Task 3 说的 `%w` 保留原始错误落不了地）
  - `func newMailer(tr transport, logger *slog.Logger) *mailer`（只构造，不启动；不需要 cfg——配置已在 transport 里）
  - `func (m *mailer) run()`；`func newLogSender(cfg config.Config, logger *slog.Logger) *logSender`
  - `mailer` 字段：`queue chan Message`（容量 32）、`transport`、`logger`、`closing chan struct{}`、`done chan struct{}`、`closeOnce sync.Once`、`retryBackoff []time.Duration`（默认 `[1s, 5s]`，测试可改短）；常量 `sendTimeout = 20 * time.Second`、`maxAttempts = 3`、`queueSize = 32`

- [ ] **Step 1: 写失败的测试**

`mailer_test.go`（`package mail`，stub transport 记录调用次数/消息、可脚本化返回错误）。**通用前提**：stub 的计数用 mutex 或 atomic 保护（worker goroutine 写、测试 goroutine 读）；除用例 1 外每个用例构造后 `go m.run()`；`retryBackoff` 必须在 `go m.run()` **之前**改短为 `[1ms, 1ms]`（默认 `[1s, 5s]` 会让重试用例多跑秒级）；每次 `Close` 都传 2s 超时 ctx 兜底，避免失败模式表现为挂死：
1. `TestSendBlocksWhenQueueFull`：**不启动 worker**，连续 `Send` 32 次全部返回 nil；第 33 次用 50ms 超时 ctx → 断言 `errors.Is(err, context.DeadlineExceeded)`。
2. `TestSendAfterCloseReturnsErrClosed`：启动 worker；`Close(ctx)` 后 `Send` → 断言 `errors.Is(err, ErrClosed)`（实现若漏了非阻塞优先检查，此用例约一半概率失败）。
3. `TestTemporaryErrorIsRetried`：stub 第 1 次返回 `&deliveryError{err: errors.New("451 4.0.0 try later"), temporary: true}`、第 2 次 nil；stub 每次调用向 channel 发信号；等够 2 次信号后加 50ms 静默窗口，断言没有第 3 次调用。
4. `TestPermanentErrorIsNotRetried`：stub 恒返回 `temporary: false` → 同法断言恰好 1 次调用。
5. `TestGivesUpAfterMaxAttempts`：stub 恒返回临时错误 → 同法断言恰好 3 次调用（`maxAttempts`），且日志缓冲里出现 Error 记录。
6. `TestCloseDrainsQueue`：启动 worker，`Send` 2 条（stub 立即成功），`Close(ctx)` → 断言返回 nil 且 2 条都已投递。
7. `TestCloseRespectsContext`：stub 阻塞在测试控制的 channel 上；`Send` 1 条；`Close(50ms ctx)` → 断言返回 `context.DeadlineExceeded`；随后释放 stub，并显式等 `<-m.done`（2s 兜底）确认 worker 已退出。
8. `TestCloseIsIdempotent`：连续两次 `Close(ctx)` → 都返回 nil。
9. `TestConcurrentSendCloseDoesNotPanic`：8 个 goroutine 各 `Send` 10 条，与 `Close(ctx)` 并发；断言不 panic、`Close` 在 ctx 内返回、每个 `Send` 的返回值只可能是 nil / `ErrClosed` / ctx 错误（不追求并发窗口内的消息必定投递——该边界的语义见 Step 3）。
`logsender_test.go`：10. `TestLogSenderWritesMailContent`：`newLogSender(cfg{SMTPFrom:"a@example.com", SMTPFromName:"NeoBlog"}, bufLogger)` + `newMailer(sender, bufLogger)`；启动 worker；`Send` 一条（To/Subject/Text 固定值）后 `Close`；断言日志缓冲同时含 `a@example.com`、收件人、主题与正文片段，以及 `to=` / `subject=` 两个字段名；另断言 From 为空时显示 `noreply@localhost`。
日志断言用注入的 `*slog.Logger`（`slog.New(slog.NewTextHandler(&buf, nil))`），不改全局 `slog.Default()`。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/modules/mail/ -count=1`
Expected: 编译失败（类型与函数未定义）。

- [ ] **Step 3: 实现 `mailer.go` 与 `logsender.go`**

要点（语义按 spec §5，界限如下）：
- `Send`：**先做一次非阻塞优先检查**（`select { case <-m.closing: return ErrClosed; default: }`），再进入三分支 `select`（`m.queue <- msg` → nil；`<-m.closing` → `ErrClosed`；`<-ctx.Done()` → `ctx.Err()`）。优先检查是必须的：`closing` 关闭后「入队」与「已关闭」两个分支同时就绪，Go 对同时就绪分支随机选择，会让约一半的关闭后调用静默返回 nil 而消息无人消费。关闭窗口语义（接受，并写进 spec §5）：与 `Close` 真并发时已入队的消息仍会被 drain 尽力投递；drain 判定排空退出后入队的边界消息不保证投递。
- `Close`：`closeOnce.Do(func(){ close(m.closing) })`，随后 `select` 等 `<-m.done` 或 `<-ctx.Done()`（重复调用安全）。
- `run`：主循环 `select`（收队列 / 收 `closing`）；收到 `closing` 后进入排空循环（`select` 收队列，`default` 时 `close(m.done)` 并返回）。
- `deliverWithRetry`：最多 `maxAttempts` 次；每次用 `context.WithTimeout(context.Background(), sendTimeout)` 调 transport；失败且 `temporary` 且还有尝试 → `logger.Warn` + `time.Sleep(backoff[i])` 后重试；否则 `logger.Error` 后丢弃（`Send` 侧早已返回，无回调对象）。
- 判定：仅 `errors.As` 命中 `*deliveryError` 且 `temporary` 为真才重试；其它错误视为永久。
- 日志字段固定：失败 Error 含 `to` / `subject` / `attempt` / `err`；重试等待 Warn 含 `attempt` / `backoff`。
- **`Close` 返回错误 ≠ worker 已停**：退避用的 `time.Sleep` 不响应 `Close`，worker 最坏继续跑完在途投递 + 2 次退避（上界约 3×20s + 1s + 5s）后自行 `close(m.done)`；无无界泄漏。
- `logSender.deliver`：`logger.Info` 打 `to` / `from`（`cfg.SMTPFrom`，空则 `noreply@localhost`）/ `from_name` / `subject`，正文用 `msg.Body.Text` 原样附上；永远返回 nil。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/modules/mail/ -count=1 -v`，Expected: 全 PASS（无挂起；`TestCloseRespectsContext` 结束时 worker 已退出）。

- [ ] **Step 5: 全量回归 + 静态检查**

Run: `gofmt -l . && go vet ./... && go test ./... -count=1`，Expected: 全绿。

- [ ] **Step 6: Commit**

```bash
git add backend/internal/modules/mail
git commit -m "feat(backend): 邮件核心（队列/worker/重试）与日志发送实现"
```

---

### Task 3: SMTP transport（go-mail）与 `New(cfg)`

**Files:**
- Create: `backend/internal/modules/mail/smtp.go`
- Create: `backend/internal/modules/mail/smtp_test.go`
- Modify: `backend/internal/modules/mail/mailer.go`（新增导出的 `New`）
- Modify: `backend/go.mod` / `backend/go.sum`

**Interfaces:**
- Consumes: Task 1 的配置字段；Task 2 的 `transport` / `deliveryError` / `newMailer` / `newLogSender`。
- Produces:
  - `func New(cfg config.Config) Mailer`——`SMTP_HOST` 空 → log transport，非空 → smtp transport；logger 一律传 `slog.Default()`；构造后 `go m.run()`
  - 包内 `func newSMTPTransport(cfg config.Config, opts ...gomail.Option) *smtpTransport`（测试缝：可追加 `gomail.WithTLSConfig`）
  - `smtpTransport.deliver` 的错误映射：**默认可重试**；仅当 `errors.As` 到 `*gomail.SendError` 且 `ErrorCode() != 0` 且 `!IsTemp()`（即服务端 5xx）时 `temporary = false`

- [ ] **Step 1: 写失败的集成测试**

`smtp_test.go`（`package mail`）：测试内自建**自签证书**（`crypto/ecdsa` 生成密钥 + `x509.CreateCertificate`，SAN 含 `127.0.0.1`）；用 `emersion/go-smtp` 起假服务器（实现 `Backend` / `Session`：`Mail` / `Rcpt` / `Data` 记录 from/to/原始字节，`Data` 可脚本化返回 `&smtpserver.SMTPError{Code: 550, EnhancedCode: smtpserver.EnhancedCode{5,1,1}, Message: "no such user"}` 之类），`Server.TLSConfig` 挂该证书，`net.Listen("tcp","127.0.0.1:0")` + `go srv.Serve(l)`，`t.Cleanup` 关闭；`Session` 五个方法都要实现（`Reset()` 与 `Logout() error` 空实现）；客户端经 `newSMTPTransport(cfg, gomail.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}))`。
用例：
1. `TestSMTPTransportDeliversMultipartWithChineseSubject`：`cfg` 用 `SMTPAuthNoAuth` 路径（Username 空）；投递 `Subject: "邮件配置自检"`、Text/HTML 各一段；断言假服务器收到的 from/to 正确；对原始消息用 `net/mail.ReadMessage` + `(&mime.WordDecoder{}).DecodeHeader` 解出 Subject 等于原文；正文用 `multipart.NewReader` 拆分，按各 part 的 `Content-Transfer-Encoding`（go-mail 默认 quoted-printable，会把中文与 `=` 改写成 `=XX`）解码后再断言 Text 与 HTML 两段内容都在——**不要对原始字节做 substring**。
2. `TestServer5xxIsPermanent`：`Data` 返回 550 的 `SMTPError` → 断言 `deliver` 返回 `*deliveryError` 且 `temporary == false`。
3. `TestServer4xxIsTemporary`：`Data` 返回 451 → 断言 `temporary == true`。
4. `TestDialFailureIsTemporary`：`net.Listen` 拿到端口后立即关闭，指向该端口 → 断言返回错误且 `temporary == true`。
5. `TestNewSelectsLogTransportWhenHostEmpty`：`New(config.Config{})`（不依赖 DB）→ `Send` 一条 → `Close` → 无错返回且不发生 SMTP 连接（log 分支）。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/modules/mail/ -count=1`
Expected: 编译失败（`newSMTPTransport` / `New` 未定义）。

- [ ] **Step 3: 实现 `smtp.go` 与 `New`**

要点（Option 名已对源码核过，见 spec §8）：go-mail 的包名也叫 `mail`、与本包冲突，导入必须起别名（全计划用 `gomail "github.com/wneessen/go-mail"`）。`gomail.NewClient(cfg.SMTPHost, opts...)`，opts 依次为 `WithPort(cfg.SMTPPort)`、`starttls` → `WithTLSPortPolicy(gomail.TLSMandatory)` / `ssl` → `WithSSLPort(false)`、`WithTimeout(sendTimeout)`、`WithUsername` + `WithPassword`（仅当 `SMTPUsername` 非空）、`WithSMTPAuth(gomail.SMTPAuthAutoDiscover)` 或 `WithSMTPAuth(gomail.SMTPAuthNoAuth)`；测试追加的 `opts` 排在其后生效。
消息装配：先 `msg := gomail.NewMsg()`（零值 `Msg` 的编码器未初始化，中文主题会被编成畸形 encoded-word），再 `FromFormat(cfg.SMTPFromName, cfg.SMTPFrom)` / `To(m.To)` / `Subject(m.Subject)` / `SetBodyString(gomail.TypeTextPlain, m.Body.Text)` / `AddAlternativeString(gomail.TypeTextHTML, m.Body.HTML)`；`FromFormat` 与 `To` 的错误包装成 **permanent** `deliveryError`（地址类错误不重试）；发送走 `client.DialAndSendWithContext(ctx, msg)`。
错误：包装进 `*deliveryError`，`Error()` 文案带上 `SendError` 的 `ErrorCode()` 与 `EnhancedStatusCode()`（可取得时），并用 `%w` 保留原始错误。`gomail.NewClient` 的错误（已被配置校验挡在端口上界之外，理论上只剩主机名非法）不 panic：存入 transport，`deliver` 直接返回 permanent `deliveryError` 并记 Error 日志。边界：`WithPort(25)` 会被 go-mail 视为「未设置」（等于默认端口）从而被 TLS Option 改写成 587/465，因此 **25 不受支持**——README 邮件章节注明（不加额外校验，保持校验面与 spec §8 一致）。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/modules/mail/ -count=1 -v`，Expected: 全 PASS。

- [ ] **Step 5: 依赖清单核对 + 全量回归**

Run: `go mod tidy && git diff --stat go.mod go.sum`（在 `backend/` 下执行），Expected: 新增 `github.com/wneessen/go-mail v0.8.1`（直接）与 `github.com/emersion/go-smtp v0.25.0`（直接）及其传递依赖（`go-sasl` 等），无其它意外引入；`gofmt -l . && go vet ./... && go test ./... -count=1` 全绿。

- [ ] **Step 6: Commit**

```bash
git add backend/internal/modules/mail backend/go.mod backend/go.sum
git commit -m "feat(backend): SMTP transport 与按配置装配的 New"
```

---

### Task 4: 模板机制与示例邮件（含可选真实投递测试）

**Files:**
- Create: `backend/internal/modules/mail/template.go`
- Create: `backend/internal/modules/mail/templates/example.html`
- Create: `backend/internal/modules/mail/templates/example.txt`
- Create: `backend/internal/modules/mail/template_test.go`
- Create: `backend/internal/modules/mail/realdelivery_test.go`

**Interfaces:**
- Consumes: Task 3 的 `newSMTPTransport`；Task 1 的 `config.LoadDotEnvFile`。
- Produces: `func Render(name string, data any) (Body, error)`——从 `//go:embed templates/*.html templates/*.txt` 里取 `<name>.html`（`html/template`）与 `<name>.txt`（`text/template`）各渲染一份；模板缺失或执行出错原样返回错误。

- [ ] **Step 1: 写失败的模板测试**

`template_test.go`：
1. `TestRenderExample`：`Render("example", map[string]string{"Time": "...", "Site": "..."})` → 两份都非空、都含注入值。
2. `TestRenderMissingTemplate`：`Render("nope", nil)` → 非 nil 错误。
3. `TestRenderEscapesHTML`：data 里放 `<script>alert(1)</script>` → HTML 结果**不含**该字面量、含 `&lt;script&gt;`；Text 结果原样保留（text/template 不转义）。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/modules/mail/ -count=1`，Expected: 编译失败（`Render` 未定义）。

- [ ] **Step 3: 实现 `template.go` 与 `example` 模板**

`example.html`：极简内联样式的中文邮件（标题 + 「如果你收到这封邮件，说明 NeoBlog 的邮件配置可用」+ 渲染时间 + 站名）。`example.txt`：同义的纯文本版。两份都只用 `{{.Time}}` / `{{.Site}}` 两个占位符。`Render` 每次调用现场 `template.ParseFS` 解析（不做缓存，spec §7）。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/modules/mail/ -count=1`，Expected: 全 PASS。

- [ ] **Step 5: 写可选真实投递测试**

`realdelivery_test.go`：`TestRealDelivery`——先从进程环境读 `SMTP_*` 与 `MAIL_TEST_RECIPIENT`；`SMTP_HOST` 为空时从包目录向上逐级查找 `.env`（`findEnvFile`，深度上限 5）并用 `config.LoadDotEnvFile` 载入后再读一次（**不写死相对深度**；初版写死 `../../.env`，归位后静默失效，见头部实施后修订）；`SMTP_HOST` / `SMTP_FROM` / `MAIL_TEST_RECIPIENT` 任一为空 → `t.Skip`（`SMTP_USERNAME` / `SMTP_PASSWORD` 允许为空；`SMTP_TLS` 为空按 `starttls` 处理）。用读到的值构造 `config.Config` 字面量（`SMTP_PORT` 用 `strconv.Atoi`，`SMTP_TLS` 映射到枚举），Subject 固定「NeoBlog 邮件配置自检」，**直接调 `newSMTPTransport(cfg).deliver(ctx, msg)`**（不走异步 Mailer）并用 20s ctx 真发一封 `Render("example", ...)` 的邮件到 `MAIL_TEST_RECIPIENT`；`err != nil` → `t.Fatalf`。

- [ ] **Step 6: 跑测试（两个分支各一次）**

Run: `go test ./internal/modules/mail/ -count=1 -run TestRealDelivery -v`，Expected: 无凭据环境 → SKIP。
用户手动分支（有真实 SMTP 与收件地址时）：在 `backend/.env` 填好 `SMTP_*` 与 `MAIL_TEST_RECIPIENT` 后从 `backend/` 跑同一条命令 → Expected: PASS，且收件箱收到「邮件配置自检」。

- [ ] **Step 7: Commit**

```bash
git add backend/internal/modules/mail
git commit -m "feat(backend): 邮件模板机制与示例邮件（含可选真实投递测试）"
```

---

### Task 5: app 装配与关闭 drain

**Files:**
- Modify: `backend/internal/app/app.go`
- Modify: `backend/internal/app/app_test.go`

**Interfaces:**
- Consumes: Task 3 的 `mail.New(cfg)`、Task 2 的 `ErrClosed`。
- Produces: `App` 内部新增字段 `server *http.Server` 与 `mailer mail.Mailer`（均为未导出）；包内 `func (a *App) shutdown(ctx context.Context) error`（先 `a.server.Shutdown(ctx)`，再 `a.mailer.Close(ctx)`，返回先出现的错误）；`New(cfg, db)` / `Handler()` / `Run(addr)` **签名与对外行为不变**。

- [ ] **Step 1: 写失败的测试**

`app_test.go` 新增 `TestShutdownClosesMailer`（真库，沿用 `newTestServer` 的 skip 约定，测试配置沿用 `testConfig`——`SMTPHost` 为空即 log 模式）：构造 `a := New(cfg, db)`；`a.shutdown(context.Background())` → 断言返回 nil；随后 `a.mailer.Send(ctx, mail.Message{...})` → 断言 `errors.Is(err, mail.ErrClosed)`。（`http.Server.Shutdown` 对未启动的 server 直接返回 nil。）

- [ ] **Step 2: 跑测试确认失败**

Run: `DATABASE_URL_TEST='postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_test?sslmode=disable' go test ./internal/app/ -count=1`
Expected: 编译失败（`shutdown` / `mailer` 未定义）。

- [ ] **Step 3: 实现**

`New` 里构造 `mailer := mail.New(cfg)` 与 `server := &http.Server{Handler: sessions.LoadAndSave(engine), ReadHeaderTimeout: ..., ...}`（超时常量原样搬入），存入结构体；`Run(addr)` 里**先 `a.server.Addr = addr`**（漏了这行会去监听 `:http` 即 80 端口），再保留信号处理与 `ListenAndServe`，关闭段改为 `return a.shutdown(shutdownCtx)`。注意别写成新方法 `run()`——`Run` 是 main.go 调用的既有导出方法。

- [ ] **Step 4: 跑测试确认通过**

Run: 同 Step 2 → PASS；再 `go test ./... -count=1`（不带 `DATABASE_URL_TEST`）→ 全绿（新用例 skip、既有 app 用例全过）。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/app
git commit -m "feat(backend): app 装配邮件模块并在关闭时 drain"
```

---

### Task 6: README、DEVLOG、版本进位与终验

**Files:**
- Modify: `backend/README.md`
- Modify: `docs/DEVLOG.md`
- Modify: `backend/internal/version/version.go`（脚本改）

**Interfaces:**
- Produces: 文档与实现一致；spec §14 六条验收全部跑过。

- [ ] **Step 1: 更新 README**

环境变量表新增 7 个 `SMTP_*`（默认值与说明照 spec §8）+ `MAIL_TEST_RECIPIENT`（仅测试读取，列法同 `DATABASE_URL_TEST`）；目录对照表新增一行 `internal/modules/mail`（基础设施：邮件发送，SMTP / log 双实现）；新增「邮件」章节：`Mailer` 接口与 `Render` 用法示例、`SMTP_HOST` 空即日志降级、`mailer.Send` 只保证入队（投递异步 + 有界重试）、真实投递测试怎么跑（`MAIL_TEST_RECIPIENT` + `backend/.env`）、`SMTP_PORT=25` 不受支持（go-mail 会视为未设置）、部署前置一句话（SPF/DKIM/DMARC，细节指向 spec §11）。

- [ ] **Step 2: 提交文档**

```bash
git add backend/README.md
git commit -m "docs(backend): 邮件基础设施文档（环境变量/目录/用法）"
```

- [ ] **Step 3: 版本进位**

Run: `cd backend && ./scripts/bump-version.sh minor`（实际按执行时版本：0.3.1 → 0.4.0）
```bash
git add backend/internal/version/version.go backend/README.md
git commit -m "chore(backend): 版本进位 0.3.0（邮件基础设施）"
```

- [ ] **Step 4: 写 DEVLOG 条目**

在 `docs/DEVLOG.md` 顶部新增「2026-10-11 · 邮件基础设施（只做后端）」：做了什么（`internal/modules/mail` 接口/队列/重试/模板/日志降级、配置项、app 装配 drain）、测试（单测 + 进程内 go-smtp 集成 + 可选真实投递）、已知缺口（无 outbox / 无退信处理 / 无连接复用 / 无消费者，均指向 spec §12、§15）、关键决定（go-mail v0.8.1、不上 outbox、不上 river 的理由一句话）。
```bash
git add docs/DEVLOG.md
git commit -m "docs(repo): 新增开发日志（2026-10-11 邮件基础设施）"
```

- [ ] **Step 5: 终验（spec §14 逐条）**

按修订后的 spec §14 逐条取证（第 3、5 条本轮无消费者，以测试取证代替人工触发）：
- `gofmt -l . && go vet ./... && go build ./...` 干净；`go test ./...`（带与不带 `DATABASE_URL_TEST` 各一次）全绿。
- §14.3 日志降级：`go test ./internal/modules/mail/ -run 'TestLogSenderWritesMailContent|TestNewSelectsLogTransportWhenHostEmpty' -v` 通过。
- §14.5 失败行为：`go test ./internal/modules/mail/ -run 'TestDialFailureIsTemporary|TestTemporaryErrorIsRetried|TestGivesUpAfterMaxAttempts' -v` 通过；`go test ./internal/app/ -run TestShutdownClosesMailer -v`（带 `DATABASE_URL_TEST`）通过。
- `/api/v1/healthy` 的 `version` 字段为 0.3.0。

- [ ] **Step 6: 收尾提交（仅在终验发现问题时）**

修复后按层分别提交（只 `git add` 实际改动的文件，禁止 `git add -A`）；无问题则跳过。
