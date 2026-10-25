# NeoBlog 注册邮箱所有权认证（先验证后建号）

**日期**：2026-10-25 · **范围**：后端（`database` / `auth` / `mail` / `settings` / `access` / `app`）+ 前端（注册页、验证页、用户注册设置页、用户列表、i18n） · **实施计划**：待本 spec 审阅通过后另写（`docs/superpowers/plans/`）

**状态**：v1.1 待用户审阅。方案按 2026-10-25 对话裁定：用户在「先建号后激活」与「先验证后建号」之间选择**方案 A（先验证后建号）**；并裁定「不发重发入口、链接 30 分钟、改邮箱不做、老账号标不需要验证、开关前置条件含 SMTP 与 `site.url`」。

修订记录：
- v1（2026-10-25）：首版。把此前对话里针对「先建号」方案产生的覆盖、登录闸门、三种用户状态全部作废，改为待验证注册表 + 两种用户状态。
- v1.1（2026-10-25）：复核修复。验证消费的删除条件追加 `token_hash`：读取在事务外、重新提交会原地替换同一行的令牌，只按 id 删除会让旧链接消费新记录；§6.2 与 §8 I4 同步。

---

## 1. 意图与背景

后台「用户注册」设置页里已有 `register.email_verification_enabled`（`settings/registry.go:32`，布尔、默认 `false`、非公开），但**零消费方**：打开它今天什么都不会发生。邮件基础设施已就绪（SMTP 发送、日志降级、`signup_code` / `password_reset` 模板、管理端测试发送），但业务侧没有任何发信调用；`users` 表没有邮箱验证字段，也没有待验证注册表。

用户 2026-10-25 点名接入「邮箱所有权认证」：开关打开后，注册必须证明邮箱属于自己才算完成。最终形态选择**方案 A：先验证、后建号**——

- 提交注册时只写一条「待验证注册」记录，**不建用户**；
- 发一封 30 分钟有效的验证链接邮件；
- 点链接验证通过后，才真正创建用户，建出来就是「已验证」；
- 开关关着时，注册行为与今天逐字节一致。

选择 A 而不是「先建号后激活」的原因（用户已确认）：

- 未验证时**不占邮箱、不占用户名**，抢注问题从根上消失，不需要「覆盖未验证账号」补丁；
- 链接过期没有永久锁死：没有账号，用户重新提交注册即可；
- 登录逻辑不需要给真实账号加验证闸门；
- 用户状态只需要两种（已验证 / 不需要验证），不需要三种。

用户明确不要的东西：**任何重发入口**（发送提示页、登录页都不给），**改邮箱重新验证**（本轮完全不碰 `PATCH /users/me` 的邮箱修改），**验证码形式**（不要 6 位数字，只要链接）。

## 2. 已定决策

| # | 决策 | 口径 |
| --- | --- | --- |
| D1 | 建号时机 | 先验证、后建号。开关开着时，验证通过前 `users` 表没有这一行 |
| D2 | 设置项 | 复用 `register.email_verification_enabled`，默认 `false`；后台显示名「邮箱所有权认证」；**保持非公开**——公开注册页不需要预读它，202 响应本身就能驱动前端状态 |
| D3 | 开关前置条件 | 两条同时满足才允许在界面上打开：① SMTP 真正可用（`Mailer.Configured` 为真，即传输层会走 SMTP 而不是日志降级；设置组与环境变量回退走同一套格式校验）；② `site.url` 非空且是合法的 http/https 绝对地址（有 host、不带 query/fragment）。已打开时即使条件后来不满足，也允许关掉；页面显示一条显式警示。运行期任何一条不满足 → 注册直接失败 `mail_unavailable`，绝不跳过验证；注册发信拒绝日志降级 |
| D4 | 发送次数 | 只在注册提交时发**一封**；没有重发按钮、没有重发接口。链接 30 分钟有效。用户重新提交注册表单会覆盖待验证记录并发新链接（这是唯一的自救路径） |
| D5 | 令牌形态 | 32 字节 `crypto/rand` → base64url（43 字符）；库里只存 SHA-256 十六进制哈希；一次性消费；无错误次数上限；验证接口按 IP 限流 |
| D6 | 链接落点 | `{site.url}/signup/verify?token=...`。验证页是前端页面，**必须先点页面上的按钮才提交**，GET 本身不消费令牌——避免邮件安全扫描器预取链接时把一次性令牌吃掉 |
| D7 | 待验证记录 | 字段：邮箱、用户名、密码 bcrypt 哈希、令牌哈希、过期时间、创建时间；邮箱唯一、令牌哈希唯一、过期时间有索引。明文密码永不落库；`users.email_verification_status` 在 ent/数据库层只允许 `verified` / `not_required` 两个值 |
| D8 | 唯一性 | 提交时邮箱和用户名只跟**真实用户**比对；待验证记录不占用户名。同一邮箱重复提交 → 覆盖旧记录并重发。两个待验证记录可以同名，先验证成功者建号，后者得到 `nickname_taken` 并删掉自己的记录；若两者同时验证、检查后撞唯一约束，则后者回滚（记录还原）并得到 `already_taken` |
| D9 | 用户状态标记 | `users.email_verification_status` 两个值：`verified`（经链接验证后建号）、`not_required`（开关关着时注册、管理员建号、老账号）。**没有 `pending` 用户状态**；「未验证」的东西在 `pending_registrations` 表里，不是用户 |
| D10 | 登录 | 真实账号不加验证闸门。为保留用户要的「账户尚未验证」提示：登录时找不到用户、且登录名是邮箱时，查一次待验证记录并用其中的密码哈希核对；密码正确才返回 403 `email_not_verified`（前端文案「账户尚未验证」），密码错误仍走原来的 `invalid_credentials`。用用户名登录的待验证用户拿不到这个提示（待验证记录只认邮箱） |
| D11 | 接口形态 | `POST /users`：开关关 → 201 用户；开关开 → 202 `{"status":"verification_pending"}`，不含用户数据。新增 `POST /users/email-verifications`（body `{"token"}`）→ 201 用户。新增 `GET /admin/mail/status` → `{"smtp_configured","site_url_configured"}` |
| D12 | 邮件模板 | 沿用现有 `signup_code` 模板与 `signup_code_subject` / `signup_code_body` 两个设置键（不改键名、不加新键）。界面标签改「注册验证邮件」，默认正文改成验证链接，占位符用 `{{verifyUrl}}` 替换签名模板里的 `{{code}}`；管理端测试发送同步改用假链接。自定义正文若还写着 `{{code}}`，渲染出来就是空，不做兜底 |
| D13 | 发信能力 | `Mailer` 增加 `Configured(ctx) (bool, error)`；注册走 `SendSync`（同步拿结果，失败就整个注册失败），并给消息打 `RequireSMTP` 标记，传输层解析为日志降级时直接报错，绝不把含令牌的正文写进日志。发信失败 → 按本次写入行的 `id` + `token_hash` 删除待验证记录（补偿；不能按邮箱删，否则会误删并发重提交的新记录）→ 返回 `mail_unavailable`。`Compose` 从 mail controller 抽成包级导出函数，管理端测试发送与注册共用 |
| D14 | 开关保存校验 | 后端 `PATCH /admin/settings/register` **不做跨组校验**（settings 模块不依赖 mail/config，避免跨组不变量网）。前置条件只在管理界面强制 + 运行期 fail-closed；绕过界面直接调 API 可以存出「开着但 SMTP 没配」，此时注册按 `mail_unavailable` 失败，页面下次打开显示警示 |
| D15 | 注册后会话 | 不自动登录，和今天一样。验证成功后返回用户对象但不下发会话，前端引导去 `/signin` |
| D16 | 待验证列表 | 本轮**不做**后台「待验证注册」列表；管理端用户列表只显示真实用户及其两种状态。待验证记录不是用户，不进用户列表 |

## 3. 范围

### 3.1 后端

| 文件 | 动作 | 职责 |
| --- | --- | --- |
| `internal/database/ent/schema/user.go` | 改 | 新增 `email_verification_status` 字符串字段（非空、无默认） |
| `internal/database/ent/schema/pending_registration.go` | 新增 | 待验证注册表；邮箱唯一、令牌哈希唯一、过期时间索引 |
| `migrations/<timestamp>_email_verification.sql` | 新增 | 加列 + 回填老用户 `not_required` + 建表建索引；手工改造后重算 `atlas.sum` |
| `internal/modules/auth/repository.go` | 改 | `createUser` 增加状态参数；待验证记录的增删查；验证事务（删记录 + 建号）；管理员建号状态 |
| `internal/modules/auth/service.go` | 改 | `Register` 返回 `RegisterResult`；开关分支；待验证写入与发信；`VerifyEmailRegistration`；登录时的待验证提示 |
| `internal/modules/auth/dto.go` | 改 | `RegisterResult`、`EmailVerificationRequest`、`RegisterPendingResponse` |
| `internal/modules/auth/controller.go` | 改 | `POST /users` 的 202 分支；新增 `POST /users/email-verifications` |
| `internal/modules/auth/module.go` | 改 | `newService` 增加 `mail.Mailer` 参数 |
| `internal/modules/auth/ratelimit.go` | 改 | 新增验证接口的 IP 窗口策略 |
| `internal/modules/auth/admin.go` | 改 | `CreateAdminUser` 建号写 `not_required`，并清掉同邮箱待验证记录 |
| `internal/modules/auth/validate.go` 或新文件 | 改/新增 | 令牌生成与哈希、链接数据组装 |
| `internal/modules/mail/mailer.go` | 改 | `Mailer` / `transport` 增加 `Configured`；`mailer` 委托 |
| `internal/modules/mail/settings.go` / `smtp.go` / `logsender.go` | 改 | 三个 transport 各自实现 `Configured` |
| `internal/modules/mail/controller.go` | 改 | 导出 `Compose`；新增 `GET /admin/mail/status`；`previewData` 提供 `verifyUrl`；主题默认值改名 |
| `internal/modules/mail/templates/signup_code.html` / `.txt` | 改 | 默认正文改成验证链接 |
| `internal/modules/mail/dto.go` | 改 | 状态响应结构 |
| `internal/modules/settings/registry.go` | 不改定义 | 仍非公开；只改 README 描述其消费方 |
| `internal/modules/access/dto.go` | 改 | `UserSummary` 增加 `email_verification_status` |
| `internal/modules/access/repository.go` / `service.go` | 改 | `userRecord` 带状态；`ListUsers` 映射 |
| `internal/common/apierror/apierror.go` | 改 | 新增三个错误码与状态映射 |
| `internal/app/app.go` | 改 | `auth.New` 传入 `mailModule.Mailer` |
| `backend/README.md` | 改 | 目录表、接口表、认证与邮件章节、设置清单描述 |
| `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md` | 改 | 新端点与错误码登记 |
| `docs/DEVLOG.md` | 改 | 实施完成后顶部倒序条目 |

### 3.2 前端

| 文件 | 动作 | 职责 |
| --- | --- | --- |
| `hooks/use-signup.ts` | 改 | 识别 202；保存待验证邮箱；只有 201 才跳 `/signin` |
| `app/signup/signup-form.tsx` | 改 | 待验证成功态（提示 + 返回登录），不出现重发按钮 |
| `app/signup/verify/page.tsx` | 新增 | 读取 `token`，渲染验证按钮页 |
| `app/signup/verify/verify-form.tsx` | 新增 | 点按钮 POST；成功/失败态；成功时写注册成功标记 |
| `hooks/use-signin.ts` | 改 | 映射 `email_not_verified` →「账户尚未验证」 |
| `lib/register-settings.ts` | 改 | 新增 `emailVerificationEnabled` 的解析/变更/补丁 |
| `hooks/use-register-settings-save.ts` | 改 | 新开关的 setter |
| `app/console/settings/register/register-settings-form.tsx` | 改 | 新增开关行；前置条件不满足时禁用打开；已开启但条件缺失时显示警示 |
| `app/console/settings/register/page.tsx` | 改 | 传入前置条件 |
| `lib/admin-settings.ts` | 改 | 读取 `/admin/mail/status`，与 register/auth/groups 一起返回 |
| `lib/users.ts` | 改 | `AdminUser` 增加 `emailVerificationStatus`；严格解析 |
| `app/console/settings/users/users-panel.tsx` | 改 | 新增「邮箱验证」列 |
| `app/console/settings/mail/mail-settings-form.tsx` | 改 | 模板行标签与占位符提示 |
| `messages/zh-CN.json` / `messages/en.json` | 改 | 新增文案键 |

### 3.3 不动清单

- `PATCH /users/me`（改邮箱）一行不动：不改状态、不要求重新验证。
- 登录/注册之外的所有邮件流程：密码重置、2FA 恢复码邮件（仍是 501 桩）、评论通知等。
- 注册默认组 `register.default_group` 仍不消费（既有缺口，不属本轮）。
- 公开设置负载：`register.email_verification_enabled` **不标 Public**，`TestBuiltinPublicAllowlist` 不变。
- 密码策略、域名白名单、人机验证、注册总闸、限流的既有行为与顺序。
- 邮件设置页的 SMTP 字段、密码重置模板、管理端测试发送的接口形状。
- 后台待验证注册列表（本轮不做）。
- 版本号不在本 spec 讨论。

## 4. 现状核对（实施时现读现引，行号会漂）

- `register.email_verification_enabled`：`internal/modules/settings/registry.go:32`，`FieldTypeBool`、默认 `false`、非公开；`registry_test.go` 的形状断言里也是 `public: false`，本轮不改。
- 现有注册：`auth.Service.Register`（`internal/modules/auth/service.go:60`）按「IP 桶 → 注册总闸 → 格式 → 人机验证 → 邮箱桶 → 密码策略 → 域名 → 邮箱/用户名占用 → bcrypt → 建号」执行；建号 `repository.createUser`（`repository.go:55`）只写邮箱、用户名、密码哈希，组走 schema 默认值；不建会话。
- 现有登录：`auth.Service.Login`（`service.go:119`）在用户不存在时做一次 dummy bcrypt，密码错误记失败；2FA 用户进 pending 会话。真实账号目前没有任何「邮箱验证」闸门。
- 邮件：`Mailer` 接口在 `internal/modules/mail/mailer.go:32`（`Send` / `SendSync` / `Close`）；`SendSync` 单次尝试、20 秒超时；传输层由 `settingsTransport` 在每次投递前 `resolveTransportConfig`（`mail/settings.go`），`mail.enabled=false` 或主机为空且环境变量为空时降级为 `logSender`（日志里有邮件全文）。管理端测试发送是 `POST /admin/mail/messages`，走 `SendSync`。
- 模板：`signup_code.html/.txt`、`password_reset.html/.txt`、`example.html/.txt`；`Render` 导出、`renderHTML` 包内私有；主题覆盖与自检数据组装目前在 `mail/controller.go` 的 `renderMessage` / `previewData` 里，不导出。
- 用户表：`ent/schema/user.go` 没有邮箱验证字段；`CreateAdminUser`（`auth/admin.go`）直接走 `createUser`。
- 管理端用户列表：`access.UserSummary`（`dto.go:18`）只有 id/email/nickname/group/extra_permissions；`listUsers`（`repository.go:162`）查 `User` + `Group`；前端 `lib/users.ts` 严格逐字段解析。
- 设置公开面：`settings.Service.PublicSettings`（`service.go:104`）只出 `Public` 项；`register.enabled` 是公开项，`register` 其余四项非公开。
- 前端注册：`hooks/use-signup.ts` 提交 `POST /users`，成功后 `markSignupSuccess()` + `router.push("/signin")`；`signup-form.tsx` 单步表单，无验证码输入。
- 前端登录：`hooks/use-signin.ts` 的 `toMessage` 未知码落 `failed`；新增 `email_not_verified` 映射即可。
- 前端设置页：`getRegisterSettings` 并行取 register / auth / groups；`register-settings-form.tsx` 的「注册」组现有「允许公开注册（内嵌注册默认组）」与「启用邮箱域名控制（内嵌域名清单）」两行；`ConsoleSettingsRow` 支持 `description` 与 `control`，没有现成的警示组件。
- 迁移流程：改 ent schema → `bash scripts/db-migrate-diff.sh` 生成 → 手工审阅/改造 SQL → 含数据改造时 `atlas migrate hash --dir "file://migrations"` + `atlas migrate validate`。`users` 老行回填属于数据改造，必须重算校验和。
- `frontend/lib/api.ts` 的 `apiRequest` 对 `response.ok`（含 202）返回解析后的 payload，因此 202 不需要改客户端。
- Next 16 / React 19；页面用 `export const instant = false` 读动态数据，`searchParams` 是 Promise。

## 5. 数据模型与迁移

### 5.1 `users.email_verification_status`

- 类型：`text NOT NULL`，取值只有两个：
  - `verified`：经验证链接建号；
  - `not_required`：开关关着时注册、管理员建号、本轮之前的老账号。
- 没有数据库默认值；ent 字段 `field.String("email_verification_status").NotEmpty()`，所有建号路径显式赋值。这样做是为了让「忘记赋值」在测试里立刻暴露，而不是静默落成某个默认值。
- 迁移三步：加可空列 → `UPDATE users SET email_verification_status = 'not_required'` → 设 `NOT NULL`。
- `PATCH /users/me` 改邮箱不碰这一列（D 与不动清单）。
- 状态常量放 `auth` 包（`emailVerificationVerified` / `emailVerificationNotRequired`）；`access` 只透传字符串，不比较。

### 5.2 `pending_registrations`

| 列 | 类型 | 约束 |
| --- | --- | --- |
| `id` | uuid | 主键，v7 默认 |
| `email` | text | 非空、唯一 |
| `nickname` | text | 非空 |
| `password_hash` | text | 非空（bcrypt，提交时算好） |
| `token_hash` | text | 非空、唯一（SHA-256 十六进制） |
| `expires_at` | timestamptz | 非空，提交时间 + 30 分钟 |
| `created_at` | timestamptz | 非空，ent 默认 `time.Now` |

索引：`(email)` 唯一、`(token_hash)` 唯一、`(expires_at)` 普通。无外键、无 `user_id`、无 `used_at`——验证成功时整行删除，一次性由「删除计数必须为 1」保证。

ent schema：`PendingRegistration`，无 edge。旧行清理不做后台任务：每次写待验证记录前顺手删掉全局已过期行（`DELETE WHERE expires_at < now()`，走索引）。

### 5.3 常量

- `pendingRegistrationTTL = 30 * time.Minute`
- `pendingTokenBytes = 32`
- 状态值 `verified` / `not_required`
- 验证接口 IP 限流：30 次 / 15 分钟（键前缀 `emailverify:ip:`）

## 6. 接口契约

### 6.1 `POST /users`（公开，已有端点）

请求体不变：`{"email","nickname","password","captcha_token"?}`。校验顺序与今天一致，只在最后分叉：

- 开关关：bcrypt → 建号（`not_required`）→ 201 `{"email","nickname","bio"}`；同一事务里删掉该邮箱可能残留的待验证记录。与今天的外部行为一致（多一次删除，无可见差异）。
- 开关开：
  - `Mailer.Configured` 为假或报错 → 503 `mail_unavailable`，不写待验证记录（在 bcrypt 之前检查）。
  - `site.url` 缺失或不是合法 http/https 绝对地址（无 host、带 query/fragment 都算非法）→ 503 `mail_unavailable`，不写待验证记录。
  - 否则：bcrypt → 删旧待验证记录 + 写新记录（同一事务，返回写入行的 `id` 与 `token_hash`）→ 组装邮件 → `SendSync`（消息带「必须走 SMTP」标记）。
  - 发信失败：按本次写入行的 `id` + `token_hash` 条件删除待验证记录（影响行数为 0 说明已被并发重提交覆盖，不要动新记录），返回 503 `mail_unavailable`。
  - 成功：202 `{"status":"verification_pending"}`，响应体只有这一个键。
- 邮箱/用户名已被真实用户占用：仍按今天返回 409 `email_taken` / `nickname_taken`；待验证记录不算占用。
- 同一邮箱已有待验证记录：覆盖它（新密码哈希、新用户名、新令牌、新过期时间），旧链接立即失效，响应仍是 202——不构成「该邮箱是否在待验证」的额外 oracle。
- 人机验证失败、限流、域名、密码策略等错误与今天一致。

### 6.2 `POST /users/email-verifications`（公开，新增）

- 请求体：`{"token":"..."}`；非 JSON、缺 token、空 token → 400 `invalid_request`（`param:"token"`）。
- 先做 IP 窗口限流（30/15 分钟），再处理。
- 令牌哈希查记录：
  - 不存在 → 400 `email_verification_invalid`（`param:"token"`）。
  - 存在但 `expires_at <= now()` → 先按 id + `token_hash` 删除该行，再返回 400 `email_verification_invalid`。
  - 存在且未过期：进入验证事务——
    1. 按邮箱、用户名查真实用户占用；邮箱被占 → 删待验证记录并提交，返回 409 `email_taken`；用户名被占 → 删记录并提交，返回 409 `nickname_taken`。
    2. 用 id + `token_hash` + `expires_at > now()` 条件删除待验证记录（读取与删除之间可能被重新提交替换，必须再核对令牌）；删除计数不是 1 → 回滚，返回 400 `email_verification_invalid`（并发点击、重新提交与旧链接并发都只有一个赢家）。
    3. 在同一个事务里用 `tx.User.Create` 写记录里的邮箱、用户名、密码哈希，状态 `verified`；不能改用 `r.client.User.Create`。
    4. 提交，返回 201 `{"email","nickname","bio"}`；**不下发会话**。
  - 建号撞唯一约束（检查后被抢占）：回滚会把待验证记录还原，返回 409 `already_taken`；同一链接可重试，届时会走上面的具体占用错误。两个同名待验证记录并发验证，也必须只有一个成功，另一个走这条回滚路径。
- 不检查当前注册总闸与邮箱验证开关：待验证记录是「已经受理的注册」，链接在 30 分钟内有效，关开关/关注册不追溯作废。
- 不做错误次数上限；令牌 256 位熵，猜不中。

### 6.3 `GET /admin/mail/status`（新增，受保护）

- 鉴权：`system.register.view`（消费方是「用户注册」设置页；只返回布尔，不泄露 SMTP 细节或私钥）。
- 响应：`{"smtp_configured": true|false, "site_url_configured": true|false}`
  - `smtp_configured` = `Mailer.Configured(ctx)`；即传输层会走 SMTP（设置组或环境变量），而不是日志降级。判据是「配置齐全且格式合法」，不是「实测连通」；环境变量回退也必须过同一套 `validateEffectiveConfig`（发件人必填、纯邮箱地址、username/password 配套）。
  - `site_url_configured` = 读取 `site.url` 能通过 `mail.SiteURL` 校验（http/https、有 host）。
- 读取设置失败按现有错误上抛（DB 错误 → 503）；前端把非 200 视为「无法检测」，开关保持不可打开。

### 6.4 登录 `POST /sessions`（已有端点）

- 用户不存在、登录名含 `@` 时：按规范化邮箱查待验证记录。
  - 记录存在、未过期、密码与记录里的 bcrypt 哈希匹配 → 403 `email_not_verified`（前端文案「账户尚未验证」），不建会话、不记失败、重置该登录名的失败计数。
  - 密码不匹配 → 按今天的 `invalid_credentials` 记一次失败。
  - 记录过期 → 当作不存在，走今天的 dummy bcrypt + `invalid_credentials`。
  - 登录名是用户名（不含 `@`）→ 不查待验证记录，走今天的行为。
- 真实用户（`verified` / `not_required`）的登录路径与今天完全一致，没有任何验证闸门。

### 6.5 `GET /admin/users`（已有端点）

- `UserSummary` 增加 `email_verification_status`（字符串，`verified` / `not_required`）。`GET /admin/users` 与 `GET /admin/users/:id` 都带上。
- 权限、其它字段、排序不变。

### 6.6 错误码

| 码 | HTTP | Type | Param | 用途 |
| --- | --- | --- | --- | --- |
| `mail_unavailable` | 503 | `server_error` | — | 注册时 SMTP/site.url 不可用；发信失败 |
| `email_verification_invalid` | 400 | `invalid_request_error` | `token` | 令牌不存在/过期/已消费 |
| `email_not_verified` | 403 | `permission_error` | — | 登录命中未过期待验证记录且密码正确 |

`auth` 现有错误码与状态不变。`apierror` 的注册表测试（`TestRegistryIntegrity` 计数与逐码断言）同步加三条。

## 7. 时序与错误路径

### 7.1 开关关（与今天一致）

`POST /users` → 现有校验 → bcrypt → 事务内删该邮箱待验证记录 + 建号（`not_required`）→ 201 → 前端跳 `/signin`。

### 7.2 开关开：提交

`POST /users` → 现有校验（含人机验证）→ `configured` 与 `site.url` 检查 → bcrypt → 生成令牌 → 事务：删全局过期行、删该邮箱旧记录、插入新记录 → 组装邮件（`Compose`）→ `SendSync` → 202。

- 记录写入与发信是两步：先提交记录，再发信；发信失败走补偿删除。这样邮件里不会出现「链接指向一条根本没写进库的记录」，失败时也不会留下可用的待验证记录。
- `SendSync` 最长 20 秒；注册请求可能慢，前端按钮保持 pending 态。

### 7.3 开关开：点链接

邮件链接 → `/signup/verify?token=...` 页面（GET 不消费）→ 用户点「验证邮箱」→ `POST /users/email-verifications` → 验证事务 → 201 → 页面显示「注册成功，请去登录」→ `/signin`。

### 7.4 过期 / 重复提交 / 重复点击

- 过期：验证页显示「验证链接无效或已过期，请重新注册」；回到 `/signup` 重新提交，覆盖旧记录，发新链接。
- 重复提交（未过期）：旧链接立即失效，新链接可用。
- 重复点击同一链接：第一次建号并删记录；第二次 `email_verification_invalid`。
- 并发点击：事务里的条件删除保证只有一个成功。

### 7.5 开关中途变化

- 开关从开变关：已存在的待验证记录不主动作废；用户点链接仍会建号（已验证）。用户重新提交注册则直接建号（`not_required`），并删掉待验证记录。待验证用户的「账户尚未验证」提示**不看开关**，只看记录与密码：开关关掉后，待验证记录仍会被 `POST /sessions` 命中并返回 `email_not_verified`，这是为了让用户知道「你的注册还没完成」。若管理员想彻底放行，只能让用户重新提交注册（会转成 `not_required`）。本条是本轮接受的边界。
- 开关从关变开：老账号全是 `not_required`，不受影响；只有新提交走验证。

### 7.6 SMTP / site.url 失效

- 提交时 `Configured` 为假、或 `site.url` 非法 → 503 `mail_unavailable`，不写记录。
- 发信时失败 → 删记录 + 503。
- 管理界面：开关已开但前置条件不满足时，仍可关掉开关；页面顶部显示警示（见 §9.3）。

## 8. 不变量

- **I1 开关关 = 行为不变**：`POST /users` 的请求/响应/错误码/限流与今天一致；唯一新增副作用是同一事务里删除该邮箱的待验证记录（不可见）。既有 auth 测试的**行为断言**不改；允许为 `email_verification_status` 必填字段和测试 helper 签名做机械修改。
- **I2 验证前没有用户**：开关开着时，`users` 表只在验证事务里出现这一行；建号入口是 `register`（开关关）、`CreateAdminUser`、以及新增的验证事务——验证事务是唯一新增入口，且必须用 `tx.User.Create`。
- **I3 没有未验证的真实账号**：`email_verification_status` 只有两个值；登录不给真实账号加验证闸门；`email_not_verified` 只来自待验证记录。
- **I4 令牌一次性且不可猜**：32 字节随机、只存 SHA-256、验证时条件删除计数必须为 1（条件含 `token_hash`，重新提交后旧令牌删不到新记录）、30 分钟后失效。
- **I5 明文密码不落库**：待验证记录只存 bcrypt 哈希；明文不进日志、不进邮件、不进响应。
- **I6 发信失败不留下可用注册**：`Configured`/`site.url` 失败在建记录前拦截；`SendSync` 失败按本次写入行的 `id` + `token_hash` 补偿删除，不能按邮箱删（避免误删并发重提交的新记录）。极窄的例外是「`SendSync` 实际发出成功但报错，且补偿删除也失败」——此时记录仍在、邮件里的链接可用，用户仍能完成注册；这不产生无人持有的账号，接受。
- **I7 并发验证只有一胜**：条件删除计数为 1 才建号；建号失败回滚会还原记录。
- **I8 待验证记录不占用户名**：提交只与真实用户比对；两个同名待验证记录先验证者赢。
- **I9 状态字段不参与邮箱修改**：`PATCH /users/me` 不动 `email_verification_status`。
- **I10 没有重发**：没有重发接口、没有重发按钮；唯一补发路径是重新提交注册表单。
- **I11 老账号不被锁**：迁移回填 `not_required`；管理员建号也是 `not_required`。
- **I12 公开面不扩大**：`email_verification_enabled` 仍非公开；`GET /admin/mail/status` 只出两个布尔。
- **I13 邮件令牌不进入可观测面**：明文令牌只出现在邮件正文与浏览器链接；后端不记录令牌；验证页 `no-referrer` + `noindex`；注册发信带「必须走 SMTP」标记，传输层若解析为日志降级就报错，绝不把含令牌的正文写进日志。
- **I14 202 不含用户数据**：待验证响应只有 `{"status":"verification_pending"}`。

## 9. 前端形态

### 9.1 注册页（`/signup`）

- 表单结构与今天一致：用户名、邮箱、密码、确认密码；人机验证照旧。
- 提交成功且返回 202：**不显示成功卡片**；清空表单并跳 `/signin`，由登录页挂载后的 `useSignupNotice` 用 sessionStorage 标记弹「验证邮件已发送，请查收」（和 201 成功 notice 同一机制，**不在跳转前弹**；账号还没建，所以不弹「注册成功」）。没有重发按钮。
- 提交成功且返回 201（开关关）：保持今天的行为——`markSignupSuccess()` + 跳 `/signin`。
- `mail_unavailable`（503）：toast「邮件服务不可用，请稍后再试」，留在注册页可重试。

### 9.2 验证页（`/signup/verify`）

- 服务端组件读取 `searchParams.token`，`instant = false`，metadata：`referrer: "no-referrer"`、`robots: { index: false, follow: false }`。
- 页面首屏只显示「验证邮箱」按钮，不自动提交——GET 不消费令牌。
- 点击后 POST：
  - 成功：`markSignupSuccess()` + `router.push("/signin")`；登录页的既有 notice 弹出「注册成功，请登录」。**不在验证页显示成功文案。**
  - `email_verification_invalid` / 令牌缺失：sonner「验证链接无效或已过期」，留在验证页。
  - `email_taken` / `nickname_taken` / `already_taken`：sonner「该邮箱或用户名刚被占用，请重新注册」。
  - 其他失败：sonner「服务暂时不可用，请稍后再试」。
- 令牌缺失时只显示一个去 `/signup` 的链接（不做校验尝试）。

### 9.3 「用户注册」设置页

- 「注册」组在「启用邮箱域名控制」之后新增一行：
  - 图标：`MailCheck`（新增 import；与现有图标风格一致）；
  - 标签：「邮箱所有权认证」；
  - 控件：`Switch`，`disabled` 规则：
    - `emailVerificationEnabled === true`：永远可操作（允许关掉）；
    - `emailVerificationEnabled === false`：仅当 `smtp_configured === true && site_url_configured === true` 时可打开；任一为假或状态未知 → 禁用。
  - `description`：前置条件不满足时显示一行小字，缺哪项说哪项；两项都缺就两项都列（「请先在邮件设置里配置 SMTP」「请先在站点设置里填写站点 URL」/「无法读取邮件配置，暂时无法开启」）。
- 已开启但前置条件不满足时，在表单顶部加一条显式警示（边框/底色，不用 ring），文案走 `settings.emailVerificationWarning`：「邮箱所有权认证已开启，但邮件服务或站点 URL 未就绪，当前注册会失败。」
- 保存仍走现有的 `PATCH /admin/settings/register`，只发改过的键；新开关在 `REGISTER_PATCH_KEYS` 里加一项。
- 设置页的数据加载：`getRegisterSettings` 增加 `GET /admin/mail/status`；非 200/解析失败 → `prerequisites: null`，开关按「无法检测」处理（禁用打开）。站点 URL 的判断由该端点返回的 `site_url_configured` 提供，不在前端重算，避免 60 秒公开设置缓存带来的延迟。

### 9.4 邮件设置页

- 模板行「注册验证码」→「注册验证邮件」。
- 占位符提示增加 `{{verifyUrl}}`；`{{code}}` 保留（密码重置模板仍在用）。
- 默认主题「注册验证邮件」；默认正文改成链接文案（`templates/signup_code.html/.txt`）。
- 自定义主题/正文的覆盖逻辑不变；自定义正文里残留的 `{{code}}` 会渲染为空，不做兜底。

### 9.5 用户列表

- 「用户」表格在「用户组」列之后新增「邮箱验证」列：
  - `verified` → 「已验证」
  - `not_required` → 「不需要验证」
- 前端解析严格：字段缺失或值不在两者之内 → 整页 `loadFailed`（不猜、不兜底）。
- 待验证注册不在此列表（D16）。

### 9.6 文案键（zh-CN / en 各一份）

- `settings.emailVerification`、`settings.emailVerificationSmtpHint`、`settings.emailVerificationSiteUrlHint`、`settings.emailVerificationUnknownHint`、`settings.emailVerificationWarning`
- `settings.mailTemplateSignupCode` 改为「注册验证邮件」
- `auth.signup.verificationSent`（sonner 文案，无占位符）
- `auth.signup.verifyTitle`、`auth.signup.verifyAction`、`auth.signup.verifyInvalid`、`auth.signup.verifyTaken`、`auth.signup.verifyGoSignup`
- `auth.signin.emailNotVerified` =「账户尚未验证」
- `auth.signup.mailUnavailable` =「邮件服务不可用，请稍后再试」
- `auth.signup.verifyFailed` =「验证失败，请稍后再试」
- `users.columnEmailVerification`、`users.emailVerificationVerified`、`users.emailVerificationNotRequired`

## 10. 测试计划

### 10.1 后端单元/集成（真实 Postgres；`DATABASE_URL_TEST` 必须 export，串行 `scripts/test.sh`）

- 静态门禁：`bash scripts/check.sh`（gofmt / go vet / go build / repository 禁 `apierror` 门禁）。

注册：

1. 开关关：建号成功、状态 `not_required`、没有待验证记录、`Mailer` 零调用。
2. 开关开 + `Configured=true` + site.url 合法：返回 202，响应体只有 `{"status":"verification_pending"}`；`users` 零新增；恰好一封邮件；待验证行 `password_hash` 可用原密码通过 bcrypt、`token_hash` 是 64 位 hex 且不等于明文令牌、`expires_at` 在 29–31 分钟之间；邮件正文含 `https://.../signup/verify?token=...`。
3. 开关开 + `Configured=false`（设置组关闭 / 主机空 / 环境变量空）：503 `mail_unavailable`，无待验证、无用户、零发信。
4. 开关开 + site.url 缺失/非法（空、无 scheme、`ftp://`、`javascript:`、无 host、带 query/fragment）：503 `mail_unavailable`，不写待验证。
5. 开关开 + `SendSync` 返回错误：503 `mail_unavailable`，刚写入的待验证行被删除，无用户；并发重提交已覆盖成新记录时，补偿删除不能删掉新记录。
6. 开关开 + 环境变量回退配置不完整（如 `SMTP_HOST` 有、`SMTP_FROM` 空）：`Configured=false`，注册 503。
7. 同一邮箱重复提交：旧记录被替换、旧令牌失效、新令牌有效、只多一封邮件。
8. 开关开时邮箱/用户名已被真实用户占用：`email_taken` / `nickname_taken`，不写待验证、零发信。
9. 另一个待验证记录用了同名用户名：提交成功（不占名），两条待验证并存。
10. 注册发信带「必须走 SMTP」标记：传输层解析为日志降级时 `SendSync` 报错，日志里没有正文/令牌。

验证：

11. 有效令牌：建号成功、状态 `verified`、待验证记录删除、响应 201、不下发会话。
12. 未知/过期/已消费令牌：400 `email_verification_invalid`。
13. 过期令牌验证时记录被清理（按 id 删除后再返回 400）。
14. 并发两个请求用同一令牌：恰好一个 201，另一个 400；只有一个用户。
15. 验证时邮箱/用户名被真实用户抢占：409 具体码、待验证记录删除、用户零新增。
16. 验证建号撞唯一约束（检查后抢占，用测试触发器强制）：409 `already_taken`、记录仍在（回滚）、重试可成功或得到具体占用码；两个同名待验证并发验证也只能有一个成功。
17. 开关关掉后验证仍成功（已受理的注册不被追溯作废）。

登录：

18. 待验证记录 + 正确密码 → 403 `email_not_verified`，无会话。
19. 待验证记录 + 错误密码 → `invalid_credentials`，计一次失败（低限流策略下下一次请求 429）。
20. 待验证记录 + 用户名登录 → `invalid_credentials`。
21. 已验证/不需要验证账号的登录路径与今天一致（既有行为断言不改）。

状态与列表：

22. 管理员建号状态 `not_required`；`CreateAdminUser` 清掉同邮箱待验证记录。
23. `GET /admin/users` 与 `/admin/users/:id` 带 `email_verification_status`。
24. 迁移后老用户全部 `not_required`；列 `NOT NULL`。
25. `PATCH /users/me` 改邮箱后 `email_verification_status` 不变（I9）。
26. 状态列拒绝第三个值（`pending` 或未知值），只有 `verified` / `not_required` 能入库。

设置与邮件：

27. `email_verification_enabled` 仍非公开；`TestBuiltinPublicAllowlist` 不变；管理端 GET 能读到该键。
28. `GET /admin/mail/status`：SMTP 配齐时 true；`mail.enabled=false`/主机空/环境变量空时 false；site.url 缺失/非法时 false；读设置失败时 503；无 `system.register.view` 时 403。
29. `Mailer.Configured`：设置组 SMTP、环境变量回退、日志降级三种路径的布尔值；环境变量回退缺 sender 时必须 false/报错。
30. `Compose`：默认模板渲染 `verifyUrl`；自定义主题/正文覆盖生效；自定义正文坏模板报错（不兜底）；管理端测试发送的 `signup_code` 行用假链接、`expiresMinutes=30`。
31. `D14`：绕过界面直接 PATCH `email_verification_enabled=true` 成功（后端不做跨组校验）；随后注册 503。
32. `apierror` 注册表：三个新码的 type/status/param，`all` 列表与计数同步 39。
33. 既有测试同步：`controller_test.go` 的旧文本断言（`注册验证码`/`123456`/`10`）、`app_test.go` 的 `注册验证码` 主题、`app_test.go` 的裸 SQL `INSERT INTO users` 都要更新。

### 10.2 前端

- `npx tsc --noEmit`、ESLint、Prettier 对**改动文件**通过；`npx prettier --check .` 在基线就有 `components/ui/select.tsx` 失败，不作为本轮门禁。
- 手工 E2E（需要真实 SMTP 或本地 SMTP sink；日志降级不算 configured）：
  1. 配好 SMTP 与站点 URL → 「用户注册」页开关可开 → 保存。
  2. `/signup` 注册 → 页面显示「验证邮件已发送」→ 收信 → 点链接 → 点「验证邮箱」→ 成功 → `/signin` 登录成功。
  3. 用未验证的邮箱+正确密码登录 → 「账户尚未验证」。
  4. 30 分钟后点旧链接 → 失效提示 → 回注册页重提 → 新链接可用（过期路径由集成测试覆盖，手工可选）。
  5. 关闭开关 → 注册回到一步完成。
  6. 把 SMTP 主机清空 → 设置页显示警示、注册报「邮件服务不可用」。

### 10.3 变异自检（写完测试后把代码故意改坏，必须变红）

1. 去掉 `expires_at` 判断 → 过期用例红。
2. 去掉「删除计数必须为 1」 → 并发/重复点击用例红。
3. 跳过 `Mailer.Configured` → 未配置用例红。
4. 跳过发信失败后的补偿删除 → 记录残留断言红。
5. 验证建号写成 `not_required` → 状态断言红。
6. 202 改成 201 → 接口用例红。
7. 登录待验证分支不比密码 → 错误密码用例红。
8. 去掉 `site.url` 校验 → 缺 URL 用例红。
9. 令牌以明文入库 → 哈希断言红。
10. 把 `email_verification_enabled` 标成 Public → 公开白名单测试红。

## 11. 非目标

- 不做重发按钮、重发接口、登录页重发入口。
- 不做改邮箱重新验证；`PATCH /users/me` 一行不动。
- 不做密码重置邮件、2FA 恢复码邮件、评论/订阅通知。
- 不做后台「待验证注册」列表、不做待验证记录的手工清理界面。
- 不做后台可配置的链接有效期、令牌长度、限流参数。
- 不做异步发信队列；注册走同步 `SendSync`。
- 不做验证码形式的注册验证（6 位数字、错误次数上限均不引入）。
- 不做注册后自动登录。
- 不做 `register.default_group` 接线（既有缺口）。
- 不做后端跨组保存校验（D14）。
- 不做邮件正文 i18n；默认模板中文，管理端可自定义。
- 不引入新依赖、新环境变量、新设置键。
- 不在本 spec 讨论版本号。

## 12. 已知残留与风险

- **过期只能重新提交**：没有重发入口；用户需要回 `/signup` 重新填一遍。若用户忘了原密码也无妨，重新提交会覆盖。
- **同一邮箱在待验证表里最多一条记录**：这不是「占用」——本人重新提交会覆盖旧记录并立刻发新链接；它只保证同一时刻不会有多条有效令牌。
- **无后台清理任务**：只在每次写待验证记录前删全局过期行；如果站点长期无人注册，过期行会留到下次注册。
- **SMTP 慢时注册请求会等**：`SendSync` 超时 20 秒；前端按钮需保持 pending 态。
- **发信失败补偿删除本身可能失败**：记录会留到过期，但不会产生可用账号；下次注册会覆盖。
- **`site.url` 变更会让已发出的旧链接失效**（链接里写死了发出时的域名）。
- **自定义注册验证正文里的 `{{code}}` 会渲染为空**：不做兜底，需要管理员重新编辑。
- **用用户名登录的待验证用户看不到「账户尚未验证」**：只提示 `invalid_credentials`；用邮箱登录才能得到提示。
- **登录待验证分支多一次查询 + bcrypt**：只发生在「用户不存在且登录名含 @」的路径，失败登录本来就有 dummy bcrypt。
- **验证页依赖用户再点一次**：这是为了防邮件扫描器；代价是多一次点击。
- **令牌会出现在邮件正文与浏览器地址栏**：一次性、30 分钟、`no-referrer`、`noindex`；邮件服务商与浏览器历史仍可能留存。注册发信必须走 SMTP，传输层若解析为日志降级就报错，避免令牌进日志。
- **管理端测试发送的站点读取**：`previewData` 在 settings 非 nil 且 `site` 组读取失败时上抛（不再静默回退 `https://example.com`）；settings 为 nil 的测试路径用空字符串。`signup_code` 预览的 `expiresMinutes` 为 30。
- **`site.url` 的 query/fragment 会被拒绝**：`mail.SiteURL` 只接受 http/https、有 hostname、无 query/fragment 的地址；设置页保存时的前缀校验不能替代运行期校验。
- **既有测试要同步改**：`controller_test.go` 的旧文本断言（`注册验证码` / `123456` / `10`）、`app_test.go` 的 `TestMailSettingsEndToEnd` 主题、`app_test.go` 的裸 SQL `INSERT INTO users`。
- **管理端看不到待验证注册**：D16；需要时另开一轮。
- **人机验证只在提交注册那一步**：验证接口不挂 captcha（令牌本身就是高熵凭据）。
- **限流是单实例内存态**：与站内既有限流同局限。

## 13. 实施顺序（交接给 writing-plans）

1. 数据层：ent schema（用户状态列 + 待验证表）→ 生成/手改迁移 → 重算 atlas 校验和 → 跑迁移测试。
2. 邮件层：`Mailer.Configured`（三个 transport）→ 导出 `Compose` → 模板与预览数据 → 邮件测试。
3. 认证层：`RegisterResult` 与开关分支 → 待验证记录读写 → 验证事务 → 登录待验证提示 → 新端点与错误码 → 认证测试。
4. 设置/访问层：`GET /admin/mail/status` → `access.UserSummary` 字段 → 对应测试。
5. 应用装配：`auth.New` 传入 mailer；修所有直接建用户的测试助手——`auth_test.go`、`totp_schema_test.go`、`access/service_test.go`、`access/migration_test.go`、`settings/controller_test.go`、`app/app_test.go` 里所有 `db.Client.User.Create()` 都要补 `email_verification_status`。
6. 前端：注册页 202/成功态 → 验证页 → 登录错误映射 → 设置页开关与前置条件 → 用户列表列 → 邮件页标签与占位符 → i18n。
7. 文档：README、API 规范、DEVLOG。
8. 全量验证：`scripts/test.sh`、`npx tsc --noEmit`、ESLint、Prettier、变异自检。
