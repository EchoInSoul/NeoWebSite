# NeoBlog 密码重置（邮件链接 + 一次性令牌）

**日期**：2026-10-26 · **范围**：后端（`database` / `passwordreset` / `auth` / `mail` / `settings` / `apierror` / `app`）+ 前端（登录页、重置申请页、重置确认页、邮件设置页、`public-settings`、i18n） · **实施计划**：待本 spec 审阅通过后另写（`docs/superpowers/plans/`）

**状态**：v1.1 待用户审阅。方案按 2026-10-26 对话裁定：**链接式**；**未知邮箱 202、发信失败 503**；**保留 2FA**；接口 `POST /users/password-resets` + `POST /users/password-resets/confirmations`；**本轮不接人机验证**。

修订记录：
- v1（2026-10-26）：首版。
- v1.1（2026-10-26）：复核注册页后确认注册表单有「确认密码」框（`frontend/app/register/signup-form.tsx:134`），重置确认页因此改为同款双框；对话里口头说的「单框」作废。

---

## 1. 意图与背景

后台「登录控制」页里已有 `auth.password_reset_enabled`（`backend/internal/modules/settings/registry.go:48`，布尔、默认 `false`、非公开），开关能读能存，但**零消费方**。邮件侧的家底同样早已备好：

- `mail.password_reset_subject` / `password_reset_body` 两个设置键（`migrations/20261023120000_seed_mail_setting_values.sql:12-13`）；
- 内置模板 `mail/templates/password_reset.{html,txt}`，当前正文是「验证码 `{{code}}`」；
- 管理端测试发送 `POST /admin/mail/messages` 已支持 `template=password_reset`，预览数据里 `code=123456`、`expiresMinutes=30`（`mail/controller.go:145` 起）。

缺的是业务链路：没有发信调用、没有接口、没有存令牌的表、登录页没有入口。用户 2026-10-26 点名本轮做密码重置，并已逐条拍定：

1. 邮件里放**链接**，不放验证码；
2. 邮箱不存在时也回 202；账号存在但**发信失败要回 503**（不吞错）；
3. 重置**保留 2FA**；
4. 接口用 `POST /users/password-resets` + `POST /users/password-resets/confirmations`；
5. 本轮**不接人机验证**，只靠限流拦。

目标：忘记密码的已有账号，能靠注册邮箱自助设新密码并重新登录；管理员能在「登录控制」页一键开/关，关掉后入口、页面、接口一起失效。

## 2. 已定决策

| # | 决策 | 口径 |
| --- | --- | --- |
| D1 | 邮件形态 | 邮件里一个链接：`{site.url}/reset-password/confirm?token=...`。链接**点开页面不消费令牌**，必须在页面上点「重置密码」按钮发 POST 才消费——避免邮件安全扫描器预取链接时把一次性令牌吃掉（与注册验证页同口径） |
| D2 | 令牌 | 32 字节 `crypto/rand` → base64url（43 字符）；库里只存 SHA-256 十六进制指纹；30 分钟有效；一次性；一个账号同时最多一条，重复申请覆盖旧令牌（旧链接立即作废）；过期行在下次申请/确认时顺手清掉 |
| D3 | 防探测与错误 | 未知邮箱一律 202 `{}` 且不发信——这是刻意的「不披露」，不是兜底。邮件整体不可用（SMTP 未配置、`site.url` 非法）在账号查询**之前**判定 → 503 `mail_unavailable`；账号存在但渲染或发送失败 → 先删掉本次令牌（补偿）再 503。三条路径都不吞真实错误 |
| D4 | 开关 | 复用 `auth.password_reset_enabled`，默认 `false`；清单里标 `Public: true`（只暴露一个布尔，供登录页决定是否显示入口）。关闭期间：两个接口 403 `password_reset_disabled`、两个页面 404、登录页不显示入口；已发出的令牌**无法使用但不物理删除**，重新打开后未过期的仍可用（见 §7.5） |
| D5 | 接口 | `POST /users/password-resets`（申请，202 `{}`）+ `POST /users/password-resets/confirmations`（确认，200 `{}`）。令牌只放请求体，不放 URL |
| D6 | 密码策略 | 复用 `auth.password_requirements_enabled` / `auth.password_min_length` 与 `userinput.ValidPassword`（字母+数字、≤72 字节恒生效）；新密码与当前密码相同 → 400 `password_unchanged`。**不接受**请求体里的「确认密码」，二次输入只在前端校验 |
| D7 | 会话 | 不签发会话；重置成功在同一事务里把 `users.auth_version` 加一，所有已登录设备下一次请求被现有守卫判 401 并销毁会话（`guard/permission.go:49` 的版本比对），与「改密码」一致 |
| D8 | 2FA | 重置只改密码与 `auth_version`，**不碰** TOTP secret、启用时间、恢复码；重置后登录仍要二步验证。丢了验证器用恢复码 |
| D9 | 已认证一半的会话 | 「待二段」会话只存在 scs 会话里、没有独立表，无法精确吊销；重置后这类会话仍可用二因子完成登录。记为已知缺口，不额外加字段 |
| D10 | 限流 | 申请：IP 5 次/小时 + 邮箱 3 次/小时；确认：IP 30 次/15 分钟。key 前缀分别为 `passwordreset`、`passwordreset:email:`、`passwordreset-confirm`。**所有请求都计数**（不管邮箱存不存在），否则 429 本身会泄露存在性；超限 429 `rate_limited` + `Retry-After` |
| D11 | 人机验证 | 本轮不接：不改 `captcha_scopes` 合法值、不改设置页多选、申请页不弹验证窗 |
| D12 | 模块归属 | 新增无路由能力模块 `internal/modules/passwordreset`（照 twofactor / login / registration 的自包含口径）；路由与请求 DTO 放在 `auth` 的薄壳 controller；`app.go` 装配 |
| D13 | 邮件模板 | 内置 `password_reset` 改成链接模板，占位符用 `{{resetUrl}}`；数据里仍带一个空 `code`，让历史自定义正文里的 `{{code}}` 渲染为空而不是 `<no value>`。默认主题从「密码重置验证码」改成「密码重置」；管理端测试发送预览补 `resetUrl` |
| D14 | 失效联动 | 正常改密码（`POST /users/me/password`）或改邮箱（`PATCH /users/me` 带 `email`）时，在同一事务里删掉该账号未用的重置记录。否则一个泄露的重置链接在用户改完密码后仍然有效 |
| D15 | 通知与审计 | 不发「密码已修改」通知邮件、不做重置审计表、不做重发按钮/倒计时；需要再发一次就重新提交申请（限流兜底） |
| D16 | 前端形态 | 登录页「密码」标签同一行右侧加「忘记密码？」（位置经 2026-10-27 修订，见 §9.1）；新增 `/reset-password`（输邮箱）与 `/reset-password/confirm?token=...`（设新密码，**密码 + 确认密码两框**，与注册页一致）；成功后回登录页，不自动登录 |

## 3. 范围

### 3.1 后端

| 文件 | 动作 | 职责 |
| --- | --- | --- |
| `internal/database/ent/schema/password_reset.go` | 新增 | 重置记录表：`user_id` 唯一、`token_hash` 唯一、`expires_at` 索引 |
| `internal/database/ent/schema/user.go` | 改 | 增加 `edge.To("passwordResets", PasswordReset.Type)`；删除用户时级联删记录 |
| `migrations/<timestamp>_password_resets.sql` | 新增 | 建表、唯一索引、外键级联；改 `scripts/db-migrate-diff.sh` 里的迁移名参数后生成，人工审阅 SQL，必要时 `atlas migrate hash` + `validate` |
| `internal/modules/passwordreset/service.go` | 新增 | `New(db, settings, limiter, mailer)`，缺依赖 panic；窄接口 `Settings`（`GetBool` / `GetNumber` / `GetGroup`）；`Request` / `Confirm` 两条主流程与顺序、开关、密码策略、发信、补偿 |
| `internal/modules/passwordreset/repository.go` | 新增 | 按邮箱查用户、过期清理 + upsert 令牌、按指纹读取、原子认领 + 改密码事务 |
| `internal/modules/passwordreset/token.go` | 新增 | 32 字节随机串、base64url、SHA-256 指纹、30 分钟常量 |
| `internal/modules/passwordreset/email.go` | 新增 | 组装 `siteTitle` / `siteUrl` / `username` / `expiresMinutes` / `sentAt` / `resetUrl` / `code=""` |
| `internal/modules/passwordreset/ratelimit.go` | 新增 | 三条窗口策略与 key 前缀 |
| `internal/modules/passwordreset/{service,repository,schema}_test.go` | 新增 | 单测、真实 Postgres 集成、表形状 |
| `internal/modules/auth/controller.go` | 改 | 两条公开路由 + 请求 DTO（`PasswordResetRequest` / `PasswordResetConfirmationRequest`）；两个 handler 都过 `requireJSONRequest`；把 `requestSessionMeta(ctx).IP` 传给服务 |
| `internal/modules/auth/dto.go` | 改 | 上述两个请求 DTO |
| `internal/modules/auth/module.go` | 改 | `New` 增加 `passwordResetService` 参数并传给 controller |
| `internal/modules/auth/auth_test.go` + `captcha` / `registration` / `session` 三个 `flow_helpers_test.go` | 改 | 所有直接构造 `auth.New` / `newController` 的测试同步传入重置服务（当前 5 处，不传会 nil 解引用） |
| `internal/modules/auth/repository.go` | 改 | `updatePasswordHash` 与 `updateProfile`（带 `email` 时）改为事务内同时删除 `password_resets` 行 |
| `internal/modules/mail/controller.go` | 改 | 导出 `TemplatePasswordReset`；默认主题改「密码重置」；`previewData` 增加 `resetUrl` |
| `internal/modules/mail/templates/password_reset.{html,txt}` | 改 | 正文从验证码改成重置链接，使用 `{{resetUrl}}` / `{{expiresMinutes}}` / `{{siteTitle}}` / `{{siteUrl}}` / `{{username}}` / `{{sentAt}}` |
| `internal/modules/mail/controller_test.go` | 改 | 预览与渲染断言改为链接；补「自定义正文里的 `{{code}}` 渲染为空」 |
| `internal/modules/settings/registry.go` | 改 | `password_reset_enabled` 标 `Public: true` |
| `internal/modules/settings/registry_test.go` | 改 | 公开白名单与 auth 组断言同步 |
| `internal/common/apierror/apierror.go` | 改 | 新增 `password_reset_disabled`（403）/ `password_reset_invalid`（400，param `token`） |
| `internal/common/apierror/apierror_test.go` | 改 | 两个新码的状态映射与 param |
| `internal/app/app.go` | 改 | 构造 `passwordreset.New(...)` 并传入 `auth.New` |
| `backend/README.md` | 改 | 模块表、接口表、认证章节、邮件模板说明、设置清单描述 |
| `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md` | 改 | §3 端点表与 §7.3 错误码表 |
| `docs/DEVLOG.md` | 改 | 实施完成后顶部倒序条目（本轮不动） |

### 3.2 前端

| 文件 | 动作 | 职责 |
| --- | --- | --- |
| `lib/public-settings.ts` | 改 | `isPasswordResetEnabled(settings)`：`settings.auth?.password_reset_enabled === true`；读取失败返回 `{}` 时按关闭处理 |
| `app/login/page.tsx` | 改 | 读取开关并传给视图 |
| `app/login/signin-view.tsx` / `signin-form.tsx` | 改 | 与「密码」标签同一行右侧渲染「忘记密码？」（仅开关开启，见 §9.1 修订） |
| `app/reset-password/page.tsx` | 新增 | 申请页；开关关闭 `notFound()`；`instant = false`、noindex |
| `app/reset-password/reset-request-form.tsx` | 新增 | 邮箱表单、202 成功态、429 倒计时、错误 toast |
| `hooks/use-password-reset-request.ts` | 新增 | 校验、提交、状态与错误映射 |
| `app/reset-password/confirm/page.tsx` | 新增 | 读取 `token`；开关关闭 `notFound()`；`instant = false`、noindex、`referrer: no-referrer` |
| `app/reset-password/confirm/reset-confirm-form.tsx` | 新增 | 密码 + 确认密码（各带显示/隐藏）、按钮点击才 POST、错误 toast、成功回登录 |
| `hooks/use-password-reset-confirm.ts` | 新增 | 前端校验（空值、两次不一致、基线强度）、提交与错误映射 |
| `app/console/settings/mail/mail-settings-form.tsx` | 改 | 模板行标签「密码重置」；占位符列表加 `resetUrl` |
| `messages/zh-CN.json` / `messages/en.json` | 改 | `auth.reset.*`、`auth.signin.forgotPassword`；更新 `settings.mailTemplatePasswordReset` 与 `settings.mailTemplatePlaceholderHint` |

### 3.3 不动清单

- 注册、登录、邮箱验证、2FA、会话管理的既有行为与顺序。
- `captcha_scopes` 的合法值（仍只有 `login` / `register`）与设置页多选。
- `mail` 组其他键、SMTP 字段、管理端测试发送的接口形状（只改预览数据）。
- 管理员代改密码、用户列表、待验证注册表。
- 2FA 恢复码邮件（仍是 501 桩）。
- 公开设置的其他项；`password_reset_enabled` 是唯一新增的公开项。
- 版本号不在本 spec 讨论。

## 4. 现状核对（实施时现读现引，行号会漂）

- 开关：`settings/registry.go:48` `{Name: "password_reset_enabled", Type: FieldTypeBool, Default: false}`，当前非公开；种子行在 `migrations/20261023080025_seed_setting_values.sql:19`。后台「登录控制」页已在读写它（`frontend/lib/login-settings.ts`、`app/console/settings/login/login-control-form.tsx`）。
- 邮件：`mail/controller.go:21-29` 定义 `TemplateSignupCode`、未导出的 `passwordResetTemplate`、`previewSubjects`（密码重置默认主题「密码重置验证码」）；`Compose` 在 `:113`；`previewData` 在 `:145` 起，当前有 `code` / `verifyUrl`、没有 `resetUrl`。模板 `templates/password_reset.txt` 当前是「验证码」。
- 注册验证的令牌写法可直接对照：`registration/email_verification.go:10-27`（32 字节、SHA-256、30 分钟）；`registration/repository.go` 的 `replacePendingRegistration` 用 `OnConflict` 按邮箱覆盖；`registration/service.go:160-168` 是「Compose → SendSync(RequireSMTP) → 失败按 id+token_hash 删除」的补偿范式。
- 限流：`common/ratelimit/policy.go` 的 `AllowWindow` / `IPKey` / `HashedKey`；注册邮箱桶 3/小时、IP 桶 5/小时（`registration/ratelimit.go`）。
- 密码策略：`common/userinput/validate.go:51` `ValidPassword`、`:67` `EffectivePasswordMinLength`；auth 与 registration 各有一份读设置的 `passwordMinLength`，新模块按同样口径放第三份（自包含模块的既定代价，本轮不合并）。
- 会话失效：`guard/permission.go:49` 比较 scs 里的 `auth_version` 与数据库值，不一致就 `Destroy` + 401；`auth/repository.go:82` 的 `updatePasswordHash` 已经 `AddAuthVersion(1)`。
- 公开设置：`settings` 模块逐项白名单，`Public: true` 的项才会出现在 `GET /public/settings`；前端 `getPublicSettings` 服务器端 fetch、`revalidate: 60`、失败返回 `{}`。
- 前端认证页：登录页 `app/login/page.tsx` 读公开设置；注册验证页 `app/register/verify/page.tsx` 从 `searchParams` 取 token、`instant = false`、noindex；注册表单有「确认密码」框（`signup-form.tsx:134`）。

## 5. 数据模型与迁移

### 5.1 `password_resets`

| 列 | 类型 | 约束 |
| --- | --- | --- |
| `id` | uuid | 主键，ent 默认 UUIDv7 |
| `user_id` | uuid | 非空，**唯一**，外键 `users(id)`，`ON DELETE CASCADE` |
| `token_hash` | varchar | 非空，唯一（SHA-256 十六进制，64 字符） |
| `expires_at` | timestamptz | 非空，普通索引 |
| `created_at` | timestamptz | 非空，默认当前时间、不可变 |

ent 写法对照 `pending_registration.go`：`field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) })`；`User` 侧加 `edge.To("passwordResets", PasswordReset.Type).Annotations(entsql.OnDelete(entsql.Cascade))`（与现有 `permissions` / `recoveryCodes` 同级联写法一致），`PasswordReset` 侧用 `edge.From("user", User.Type).Ref("passwordResets").Field("user_id").Unique().Required()` 建立一对一关联与唯一的 `user_id`。

### 5.2 常量

- `tokenBytes = 32`、`resetTTL = 30 * time.Minute`；
- 原始令牌：`base64.RawURLEncoding.EncodeToString(buf)`；
- 指纹：`hex.EncodeToString(sha256.Sum256(raw))`。

### 5.3 迁移

`scripts/db-generate.sh` 重新生成 ent 代码 → `scripts/db-migrate-diff.sh` 生成 `migrations/<timestamp>_password_resets.sql` → 人工审阅 SQL（唯一索引、外键级联、`expires_at` 索引）→ 必要时 `atlas migrate hash --dir "file://migrations"` 与 `atlas migrate validate`。**无 seed 迁移**：开关与邮件四项库里已有。

## 6. 接口契约

两条都在 `/api/v1` 前缀下，都要求 `Content-Type: application/json` 且 Origin 与 Host 一致（`requireJSONRequest`）。

### 6.1 `POST /users/password-resets`（公开，新增）

请求：

```json
{"email":"user@example.com"}
```

响应：

| 情况 | 状态 | 体 |
| --- | --- | --- |
| 邮箱存在，已受理（异步投递） | 202 | `{}` |
| 邮箱不存在 | 202 | `{}`（不发信、不建记录） |
| 邮箱为空或格式非法 | 400 | `invalid_request`，`param="email"` |
| 开关关闭 | 403 | `password_reset_disabled` |
| 邮箱桶或 IP 桶超限 | 429 | `rate_limited` + `Retry-After` |
| `Configured` 读取/解析失败、SMTP 未配置、`site` 组读取失败或 `site.url` 非法 | 503 | `mail_unavailable` |
| 账号存在但模板渲染失败或入队失败 | 503 | `mail_unavailable`（令牌已补偿删除） |
| 账号存在，已入队但后台投递失败 | 202 | 不反馈；后台最多重试 3 次并记 ERROR 日志，令牌按 30 分钟自然过期 |
| 数据库错误、读 `auth.password_reset_enabled` 失败 | 500 | `internal_error` |

处理顺序（固定）：

1. IP 窗口限流（5 次/小时）——所有请求计数；
2. 读 `auth.password_reset_enabled`，关 → 403；
3. `NormalizeEmail` + `ValidEmail`，非法 → 400；
4. 邮箱窗口限流（3 次/小时，`ratelimit.HashedKey("passwordreset:email:", email)`）——所有请求计数；
5. `mailer.Configured` 为假或报错 → 503；读 `site` 组 `title` / `url` 并 `mail.SiteURL(url)`，失败 → 503；
6. 按规范化邮箱查用户：查不到 → 直接返回（controller 回 202）；
7. 生成令牌、在事务里清过期行并 upsert 本账号记录（`OnConflict` 按 `user_id` 覆盖 `token_hash` / `expires_at`）；
8. 组装邮件数据并 `mail.Compose(ctx, settings, mail.TemplatePasswordReset, data)`；
9. `mailer.Send`（异步入队，消息带 `RequireSMTP: true`）；
10. 第 8/9 步失败（渲染失败 / 入队失败）→ 按本次记录的 `id` + `token_hash` 删除（不能按 `user_id` 删，否则会误删并发重申请的新记录）→ 503；入队成功 → 202 `{}`。实际 SMTP 投递由 `mail` 模块的后台 worker 完成，失败不反馈请求方（见 §14）。

`data` 字段：`siteTitle`、`siteUrl`、`username`（用户昵称）、`expiresMinutes`（`"30"`）、`sentAt`、`resetUrl`（`siteUrl + "/reset-password/confirm?token=" + url.QueryEscape(rawToken)`）、`code`（空串，兼容旧自定义正文）。收件人用库里存的 `user.Email`，不用请求里的原始写法。

两个 handler 都以 `requireJSONRequest` 为第一道门，并把 `requestSessionMeta(ctx).IP` 传给服务；`passwordreset` 只接收 `ip string`，不依赖 `session` 包。

### 6.2 `POST /users/password-resets/confirmations`（公开，新增）

请求：

```json
{"token":"...","new_password":"..."}
```

响应：

| 情况 | 状态 | 体 |
| --- | --- | --- |
| 成功 | 200 | `{}` |
| `token` 为空 | 400 | `invalid_request`，`param="token"` |
| `new_password` 为空 | 400 | `invalid_request`，`param="new_password"` |
| 令牌不存在 / 已过期 / 已使用 | 400 | `password_reset_invalid`，`param="token"` |
| 密码不符合站点策略 | 400 | `weak_password`，`param="new_password"` |
| 新密码与当前密码相同 | 400 | `password_unchanged`，`param="new_password"` |
| 开关关闭 | 403 | `password_reset_disabled` |
| IP 桶超限 | 429 | `rate_limited` + `Retry-After` |
| 数据库或设置读取失败 | 500 | `internal_error` |

处理顺序（固定）：

1. IP 窗口限流（30 次/15 分钟）；
2. 读开关，关 → 403；
3. 读密码策略 + `ValidPassword`，不合格 → `weak_password`；
4. 按 `hashToken(token)` 查记录（不删除）：不存在 → `password_reset_invalid`；已过期 → 删除该行并返回 `password_reset_invalid`；
5. 读用户并比对当前密码哈希：用户查不到 → `password_reset_invalid`（外键级联正常时记录已先消失，这里是竞态下的等价处理）；相同 → `password_unchanged`；
6. 事务内**原子认领**：`DELETE ... WHERE id=? AND token_hash=?`，影响行数必须为 1，否则 `password_reset_invalid`（并发/被新申请覆盖）；
7. 同一事务：`users.password_hash = bcrypt(new_password, bcrypt.DefaultCost)`、`auth_version = auth_version + 1`；
8. 提交 → 200 `{}`。不签发会话，不写 cookie。

第 4 步只读、第 6 步才删除，是为了「密码不合格时不浪费令牌」：用户输错密码可以重试，只有真正认领成功才作废。两人并发提交同一令牌时，只有一个事务能删到行，另一个得到 `password_reset_invalid`。

### 6.3 错误码

新增两个（`apierror.go` 与 `statusByCode` 同步）：

| code | HTTP | type | param | 触发 |
| --- | --- | --- | --- | --- |
| `password_reset_disabled` | 403 | `permission_error` | null | 站级 `auth.password_reset_enabled=false` 时调用任一重置接口 |
| `password_reset_invalid` | 400 | `invalid_request_error` | `token` | 令牌不存在 / 过期 / 已用 / 被新申请覆盖 / 账号已删 |

复用：`invalid_request`、`weak_password`、`password_unchanged`、`rate_limited`、`mail_unavailable`、`internal_error`。

## 7. 时序与错误路径

### 7.1 正常重置

1. 用户点「忘记密码？」→ `/reset-password` → 输邮箱 → 202 `{}`；
2. 邮件里点链接 → `/reset-password/confirm?token=...`（页面只渲染，不消费）；
3. 填两次新密码 → 点按钮 → 200 `{}`；
4. 前端提示「密码已重置，请用新密码登录」→ `/login`；
5. 该账号所有旧会话下一次请求 401，前端会话失效弹窗照常触发。

### 7.2 邮箱不存在

返回与正常申请完全相同的 202 `{}`，不发信、不写 `password_resets`、不产生可观察的差异（限流计数在查询之前，429 也不泄露）。前端提示统一为「如果该邮箱已注册，重置邮件已发送」。

### 7.3 发信失败

- 渲染失败（模板坏、站点设置坏）：删掉刚 upsert 的记录（按 `id` + `token_hash`）→ 503。旧令牌如果被覆盖，也一并没了；用户可重新申请。
- SMTP 投递失败：同上。日志保留结构化错误；接口不返回细节。
- `mail.enabled=false` 或传输层会降级为日志发送：`Configured` 返回 false → 503；`RequireSMTP` 兜第二层，绝不把含令牌的正文写进日志。

### 7.4 令牌过期 / 重复点击 / 重复申请

- 过期：确认时删除该行并回 400 `password_reset_invalid`；
- 第一次成功后行已删，第二次点击回 400；
- 重复申请：`OnConflict` 覆盖 `token_hash`，旧链接立即失效；旧的过期行在同一事务里清理；
- 重新申请不受「已发过一封」限制，只受邮箱 3 次/小时与 IP 5 次/小时约束。

### 7.5 开关中途变化

- 用户打开着申请页，管理员关掉开关 → 提交得到 403，前端 toast「密码重置已关闭」；
- 用户拿着已发出的链接，开关已关 → 确认页服务器端读到公开设置后 404；直接调接口 403；
- 开关重新打开后，未被消费且未过期的令牌**仍然有效**（开关不是令牌吊销器）；如果产品上不接受，只能等 30 分钟自然过期或用户重新申请。这是已知取舍。

### 7.6 正常改密码 / 改邮箱

- `POST /users/me/password` 成功：同一事务删掉该账号的 `password_resets` 行；
- `PATCH /users/me` 请求体带 `email`（无论是否与原值相同）：同一事务删掉该账号的 `password_resets` 行；
- 管理员删除用户：外键级联删除。

## 8. 不变量

1. 一个账号最多一条重置记录；`token_hash` 全局唯一。
2. 明文令牌只出现在邮件正文与用户请求里：不入库、不写日志、不出现在错误响应。
3. 消费是原子的：只有 `DELETE ... WHERE id=? AND token_hash=?` 删到行的那个请求才能改密码。
4. 重置不签发会话、不写 cookie。
5. 重置不改变 2FA、恢复码、邮箱、用户组、邮箱验证状态、昵称、简介。
6. 重置成功必然使 `auth_version` 加一，旧会话在下一次请求全部失效。
7. 开关是全局闸门：两个接口、两个页面、登录入口一起开/关。
8. 不吞真实错误：发信失败、渲染失败、数据库错误、设置读取错误都上抛；唯一「静默」的是未知邮箱的 202，这是已授权的防探测契约。

## 9. 前端形态

### 9.1 登录页

- `SigninPage` 读 `getPublicSettings()`，`isPasswordResetEnabled(settings)` 为真时把 `passwordResetEnabled` 传到 `SigninView` → `SigninForm`。
- 凭据分支（`step === "verify"` 的 2FA 步骤不渲染）渲染一行链接「忘记密码？」，`href="/reset-password"`；样式沿用登录页现有链接（`text-sm font-medium text-primary underline-offset-4 hover:underline`）。
  - **2026-10-27 实施后修订**：原案放在「密码框下方、右对齐独立一行」，用户看实际效果后要求改位置；出四方案对照页后改定为**与「密码」标签同一行、左右分置**（`flex items-center justify-between` 包住 `Label` 与链接），少占一行垂直空间、归属更清楚。
- 公开设置读取失败（`{}`）时按关闭处理：入口不显示。代价是后端不可用时入口消失，但不会有安全影响；与 `isSignupOpen` 的 fail-open 方向不同，这是刻意的。

### 9.2 `/reset-password`（申请页）

- 服务器组件：`getPublicSettings()` + `!isPasswordResetEnabled` → `notFound()`；`instant = false`；metadata 标题 + `robots: {index:false, follow:false}`。
- 表单：邮箱单字段，`autoComplete="email"`，用普通 `Input`（与注册页邮箱框一致）；客户端校验空值与基本格式。
- 提交成功（202）：把表单替换成中性成功态「如果该邮箱已注册，重置邮件已发送」+ 返回登录链接；不出现重发按钮、不做倒计时。要再发一次就重新打开页面提交。
  - **2026-10-27 实施后修订**：本条被用户实测反馈推翻——申请成功改为**先跳 `/login`、再在登录页弹 sonner**（与注册页同用的跨页标记机制），不再原地替换成功态。原因是在当前页 `toast` 后立刻客户端跳转会把提示带走；两个找回密码页面的外壳也一并改成与注册页逐值一致（居中「返回登录」链接 + 提交按钮转圈）。见 `docs/DEVLOG.md` 同日条目。
- 错误：
  - 429：按钮倒计时（复用 `formatRetryDuration` / 登录页的 `retryRemaining` 写法）+ toast；
  - 503 `mail_unavailable`：toast「邮件服务不可用，请稍后再试」；
  - 403 `password_reset_disabled`：toast「密码重置已关闭」；
  - 其他：toast 通用失败。
- 不使用内嵌 `aria-live` 红字行，全部走 sonner。

### 9.3 `/reset-password/confirm?token=...`（确认页）

- 服务器组件：开关关闭 → `notFound()`；`instant = false`；metadata 标题 + noindex + `referrer: "no-referrer"`（防止令牌经 Referer 外泄）。
- 读取 `searchParams.token`（字符串；缺省空串）。缺 token：toast「链接无效或已过期」；不发请求。
  - **2026-10-27 修订**：原案在此处还要「页面渲染提示 + 「重新申请」链接」，用户实测后要求**整块删除**——无效令牌只走 sonner，页面不渲染任何内联提示或跳转链接，表单照常展示。`auth.reset.goRequest` 文案键随之删除。
- 表单：新密码 + 确认密码两框，各带显示/隐藏按钮，`autoComplete="new-password"`；下方一行密码提示（沿用注册页 `passwordHint` 的基线 8 位写法）；客户端校验空值、两次不一致、`isStrongPassword` 基线；服务端策略才是最终权威。
- **GET 不消费令牌**：页面只渲染；必须点「重置密码」按钮才发 POST。
- 成功（200）：toast「密码已重置，请用新密码登录」→ `router.push("/login")`；不自动登录。
- 错误：
  - 400 `password_reset_invalid`：toast「链接无效或已过期」（2026-10-27 修订：不再展示「重新申请」链接）；
  - 400 `weak_password`：toast「密码不符合站点要求」；
  - 400 `password_unchanged`：toast「新密码不能与当前密码相同」；
  - 403 `password_reset_disabled`：toast「密码重置已关闭」；
  - 429：按钮倒计时 + toast；
  - 其他：toast 通用失败。
- 已登录用户不重定向：令牌是凭证，允许在登录状态下完成重置。

### 9.4 文案键

`auth.reset`（zh-CN / en 各一份）：

| key | zh-CN | en |
| --- | --- | --- |
| `title` | 重置密码 | Reset password |
| `email` | 邮箱 | Email |
| `submit` | 发送重置邮件 | Send reset email |
| `submitting` | 发送中… | Sending… |
| `requiredEmail` | 请输入邮箱 | Enter your email |
| `invalidEmail` | 邮箱格式不正确 | Invalid email address |
| `sent` | 如果此用户存在，我们将发送一份重置链接 | If this user exists, we will send a reset link |
| `backToLogin` | 返回登录 | Back to sign in |
| `confirmTitle` | 设置新密码 | Set a new password |
| `password` | 新密码 | New password |
| `confirmPassword` | 确认密码 | Confirm password |
| `passwordHint` | 密码长度至少 {minLength} 位，须包含：字母/数字 | At least {minLength} characters with letters and digits |
| `submitConfirm` | 重置密码 | Reset password |
| `requiredPassword` | 请输入新密码 | Enter a new password |
| `requiredConfirmPassword` | 请再次输入新密码 | Enter the new password again |
| `mismatch` | 两次输入的密码不一致 | Passwords do not match |
| `invalidToken` | 链接无效或已过期 | This link is invalid or expired |
| `weakPassword` | 密码不符合站点要求 | Password does not meet the site requirements |
| `passwordUnchanged` | 新密码不能与当前密码相同 | New password must differ from the current one |
| `disabled` | 密码重置已关闭 | Password reset is disabled |
| `mailUnavailable` | 邮件服务不可用，请稍后再试 | Mail service unavailable, try again later |
| `rateLimited` | 请求过于频繁 | Too many requests |
| `retryIn` | {duration} | {duration} |
| `failed` | 操作失败，请稍后重试 | Request failed, try again later |
| `success` | 密码已重置，请用新密码登录 | Password reset. Sign in with the new password |

登录页新增 `auth.signin.forgotPassword`：zh-CN「忘记密码？」，en "Forgot password?"。

### 9.5 邮件设置页

- `settings.mailTemplatePasswordReset`：「密码重置验证码」→「密码重置」/ "Password reset"。
- `settings.mailTemplatePlaceholderHint`：说明注册验证用 `{{verifyUrl}}`、密码重置用 `{{resetUrl}}`。
- `MAIL_TEMPLATE_PLACEHOLDERS` 增加 `"resetUrl"`（保留 `"code"`，历史自定义正文可能还在用）。
- 管理端测试发送选 `password_reset` 时，预览数据使用示例链接 `https://example.com/reset-password/confirm?token=preview-token`。

## 10. 测试计划

### 10.1 后端

真实 Postgres（`DATABASE_URL_TEST` 必须 export，串行 `scripts/test.sh`）；邮件用假 Mailer，记录调用与消息内容。

申请：

1. 已存在账号 → 202、记录写入、`SendSync` 调用一次、收件人是库里邮箱、正文含 `resetUrl` 与 30 分钟；
2. 未知邮箱 → 202、零发信、零记录；
3. 开关关闭 → 403 `password_reset_disabled`、零发信；
4. 邮箱为空/格式非法 → 400、零发信；
5. SMTP 未配置 → 503、零记录；
6. `site.url` 非法 → 503、零记录；
7. 渲染失败 → 503，记录被补偿删除；
8. 发送失败 → 503，记录被补偿删除；
9. 邮箱桶第 4 次 → 429（前 3 次无论账号是否存在都计数）；
10. IP 桶第 6 次 → 429；
11. 重复申请 → 记录仍只有一条，`token_hash` 变了，旧令牌确认失败；
12. 过期行在下次申请时被清理；
13. 假 Mailer 断言消息带 `RequireSMTP=true`（日志降级拒绝发送由邮件模块既有测试覆盖）。

确认：

1. 成功 → 密码哈希可被新密码通过、`auth_version` 加一、记录被删、响应 200 `{}`、无 `Set-Cookie`；
2. 同一令牌第二次 → 400 `password_reset_invalid`；
3. 过期 → 400 且行被删；
4. 未知令牌 → 400；
5. 空 token / 空密码 → 400 `invalid_request` 且 param 正确；
6. 弱密码 → 400 `weak_password`（param `new_password`），令牌**未**被消费（还能用正确密码再来一次）；
7. 与当前密码相同 → 400 `password_unchanged`，令牌未消费；
8. 开关关闭 → 403；
9. IP 第 31 次 → 429；
10. 并发两个确认同一令牌 → 恰好一个 200，另一个 400，密码只改一次；
11. 用户已删（记录因外键级联已消失）→ 400；
12. 重置后 2FA 字段与恢复码不变；
13. 仓库层：同一行 `token_hash` 被新申请替换后，用旧指纹认领返回 0 行。

会话与联动：

1. 重置前登录的会话，重置后 `GET /users/me` → 401，会话被销毁；
2. 正常改密码后，之前申请的重置令牌确认 → 400；
3. 改邮箱后，之前申请的重置令牌确认 → 400；
4. 删除用户 → `password_resets` 行级联消失。

表形状：

1. `password_resets` 建表、`user_id` 唯一、`token_hash` 唯一、`expires_at` 有索引；
2. 同一用户插入第二条 → 约束错误；
3. 删除用户 → 行被级联删除。

设置与邮件：

1. `password_reset_enabled` 出现在 `GET /public/settings`，默认 `false`；
2. 公开白名单测试更新后仍只包含已批准的项；
3. `password_reset` 模板渲染出链接、不再出现验证码语义；
4. 自定义正文仍写 `{{code}}` 时渲染为空串（不是 `<no value>`）；
5. 管理端测试发送 `password_reset` → 201，正文含预览链接。

### 10.2 前端

- `npx tsc --noEmit`、`npm run lint`，并对本轮改动的文件单独跑 `prettier --check`（全量 `format:check` 会撞既存文件问题）。
- `localhost` + development 后端手动走：
  1. 开关关：登录页无入口；直接访问两个页面 404；
  2. 开关开：登录页出现入口；申请存在的邮箱 → 中性成功态；申请不存在的邮箱 → 同样成功态；
  3. 从邮件链接进确认页，输两次不一致 → 前端拦截；弱密码 → toast；正确密码 → toast + 回登录；旧密码登录失败、新密码登录成功；
  4. 同一链接第二次打开 → 「链接无效或已过期」；缺 token → 提示；
  5. 重置后另一浏览器里已登录的会话下一次点击 → 会话失效弹窗；
  6. 429 倒计时、503、403 各至少实测一次；
  7. 邮件设置页标签与占位符提示更新；测试发送 `password_reset` 收到链接版预览。

### 10.3 变异自检

写完测试后把代码故意改坏，确认对应测试变红：

1. 未知邮箱分支改成 404 → 防探测测试红；
2. 删除 `auth_version + 1` → 会话失效测试红；
3. 补偿删除改成按 `user_id` → 并发重复申请测试红（旧记录被误删）；
4. 认领条件只按 `id` 不按 `token_hash` → 仓库层「旧指纹认领返回 0 行」测试红；
5. 去掉开关判断 → 403 测试红；
6. 把弱密码校验挪到认领之后 → 「弱密码不消费令牌」测试红；
7. 去掉 `public` 标记 → 公开设置测试红。

## 11. 非目标

- 不发「密码已修改」通知邮件、不做审核日志、不做重置历史列表。
- 不做重发按钮 / 冷却倒计时 / 发送频率提示。
- 不做管理员代用户重置密码。
- 不接人机验证、不改 `captcha_scopes`。
- 不改 2FA 恢复码邮件（仍是 501）。
- 不做「重置后强制下线所有待二段会话」（无表可查）。
- 不改注册验证、登录、改邮箱的既有文案与流程（只加失效联动）。
- 不引入新设置键、新环境变量、新依赖。
- 不讨论版本号。

## 12. 已知残留与风险

- **待二段会话**：重置无法吊销「已输对旧密码、停在 2FA 页」的会话；它仍可凭二因子完成登录。风险低（需要二因子），本轮不修。
- **开关重开**：关掉再打开，未过期且未消费的旧令牌仍有效；要彻底作废只能等过期或重新申请。
- **开关与 SMTP 未联动**：开关开着但 SMTP 没配时，登录页入口仍然显示，用户提交后才收到 503 toast；本轮不加前置探测或公开状态（`GET /admin/mail/status` 仍需登录）。
- **限流是进程内、按直连对端 IP**：多实例各算各的；经 Next rewrite 转发后所有前台流量共用代理桶（项目既有问题，非本轮引入）。
- **公开设置缓存 60 秒**：开关切换后入口/页面状态最多延迟 60 秒。
- **申请入口没有验证码**：跨 IP 的定向邮件轰炸只受「每邮箱 3 次/小时」约束，攻击者换 IP 仍可继续；本轮已按用户裁定不接人机验证，接受这个风险。
- **令牌在 URL query 里**：可能进入浏览器历史；确认页已设 `no-referrer`，与注册验证同口径。
- **邮件发送与账号查询之间的极窄竞态**：查询后、写记录前用户恰好改邮箱，邮件仍发往旧地址；本次记录可能落在改邮箱清理之后而继续有效，直到过期。窗口极窄，不额外加锁。
- **异步投递的残留**：见 §14——进程崩溃丢内存队列、毫秒级耗时差、模板误配 / 队列满时的极小错误路径差异，均为低风险取舍。

## 13. 实施顺序（交接给 writing-plans）

1. ent schema + `db-generate.sh` + 迁移 + 表形状测试；
2. `apierror` 两个新码 + 映射测试；
3. `passwordreset` 模块（token / repository / service / ratelimit / email）+ 申请与确认的服务层测试；
4. `auth` 路由与 DTO、`module.go` / `app.go` 装配 + HTTP flow 测试；
5. `auth` 改密码 / 改邮箱的令牌失效联动 + 测试；
6. 邮件模板、导出常量、默认主题、预览数据 + 管理端文案；
7. `settings` 公开标记与测试；
8. 前端 `public-settings`、登录入口、两个页面、hook、i18n；
9. README、API 规范、DEVLOG；
10. `scripts/check.sh` + 全量 `scripts/test.sh` + `tsc` / lint + `localhost` 手动验收。

按项目规矩：不主动提交；spec 与代码改完都躺在工作区等用户检查。

## 14. 2026-10-27 修订：找回密码邮件改为异步投递

**动机**：申请接口原本「一律 202」以防枚举，但同步发送让已注册邮箱明显更慢（写令牌 + 渲染 + SMTP），计时即可区分；SMTP 拒收时还会对真实账号 503、未知邮箱 202。`mail` 模块已有异步队列（32 格、临时错误最多 3 次重试、关机排空、失败记 ERROR），无需新造。

**改动**：`passwordreset.Service.Request` 的 `SendSync` 改为 `Send`；渲染与入队仍在请求内，资格判定（开关 / SMTP 配置 / `site.url` / 邮箱桶 / IP 桶）不变。入队失败（队列满 / 已关闭）按原补偿逻辑删令牌并 503；入队成功后由 mail worker 投递，失败只记日志、不回传请求方，令牌不再因投递失败删除（30 分钟自然过期）。注册验证与测试发信仍走 `SendSync` 单次。

**残留（已知取舍）**：

- 未知邮箱 vs 已注册账号仍有毫秒级耗时差（写令牌 / 渲染 / 入队），不追求恒定时间。
- 模板误配导致渲染失败时，真实账号 503、未知邮箱 202（管理员用测试发信可发现）。
- 队列满 / 关机时真实账号 503、未知邮箱 202（运营态，极罕见）。
- 进程崩溃会丢内存队列里的待发邮件（令牌仍在，可重新申请）；正常关机由 `mailer.Close` 排空。
- 注册接口仍直接返回 409 `email_taken`，全站枚举依旧成立——本修订只保证找回密码接口自己的 202 口径，不改变全站可枚举性。
