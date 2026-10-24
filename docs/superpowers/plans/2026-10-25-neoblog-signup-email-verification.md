# 注册邮箱所有权认证实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 后台「用户注册」页新增「邮箱所有权认证」开关；开启后注册先写待验证记录并发一封 30 分钟有效的验证链接邮件，点链接验证通过后才建号。

**Architecture:** 方案 A（先验证后建号）。新增 `pending_registrations` 表存注册草稿（密码只存 bcrypt 哈希），`users` 增加 `email_verification_status` 两值状态（`verified` / `not_required`）。`POST /users` 在开关开时返回 202 并只写待验证记录；新增 `POST /users/email-verifications` 消费一次性令牌并建号；邮件层新增 `Configured` 与导出的 `Compose`。

**Tech Stack:** Go 1.27 + gin + ent + Atlas + PostgreSQL；Next.js 16 + React 19 + next-intl + Tailwind/shadcn；测试用真实 Postgres（`DATABASE_URL_TEST`）。

**Spec:** [2026-10-25-neoblog-signup-email-verification-design.md](../specs/2026-10-25-neoblog-signup-email-verification-design.md)

## Global Constraints

- **绝不提交。** 每个任务结束只跑测试、核对 diff，改动留在工作区等用户检查。不要 `git add` / `git commit` / `git push`。
- **全仓零人工注释**：Go、TS、TSX 都不写解释性注释（工具生成的 `// indirect` 除外）。计划里的 `/* ... */` 只是说明占位，最终代码不得保留。
- **禁止兜底/降级**：失败按错误上抛；邮件前置条件不满足就 fail-closed 返回 `mail_unavailable`；不要加"失败改用默认值/跳过验证"的分支。
- **不动非目标**（spec §11）：不接改邮箱验证、不做重发、不做密码重置、不做后台待验证列表、不接 `register.default_group`、不加新设置键/环境变量/依赖。
- `register.email_verification_enabled` **保持非公开**；公开设置负载与 `TestBuiltinPublicAllowlist` 不变。
- 实施前先 `git log -1` 现读 HEAD：仓库已有 `d56ab92`（站点 URL 前缀校验），spec/plan 里旧行号会漂；所有引用现读现用。
- 后端测试起手式：
  ```bash
  cd backend
  export PATH="/f/software/go/bin:/f/software/gopath/bin:$PATH"
  export GOPROXY=https://goproxy.cn,direct
  set -a; source .env; set +a
  export DATABASE_URL_TEST="${DATABASE_URL_TEST:?missing}"
  bash scripts/test.sh ./internal/...
  ```
  串行跑；不要加 `-race`（本机 cgo 不可用会挂起）。不要同时开两个测试进程打同一个 `neoblog_test`。
- 前端验证：`cd frontend && npx tsc --noEmit`；改动文件跑 `npx eslint <files>` 与 `npx prettier --check <files>`。`npx prettier --check .` 基线在 `components/ui/select.tsx` 就失败，不作为本轮门禁。浏览器验收一律用 `localhost`，不要用 `127.0.0.1`。
- 静态门禁：`cd backend && bash scripts/check.sh`（gofmt / go vet / go build / repository 禁 `apierror` / whitespace）。
- 迁移：改 ent schema 后跑 `bash scripts/db-generate.sh`；用 `atlas migrate diff email_verification --dir "file://migrations" --to "ent://internal/database/ent/schema" --dev-url "$ATLAS_DEV_URL"` 生成；手工改造 SQL 后跑 `atlas migrate hash --dir "file://migrations"` 与 `atlas migrate validate --dir "file://migrations"`；再 `bash scripts/db-migrate-apply.sh`（同时应用 dev 与 test 库，需先 `set -a; source .env; set +a`）。

## Review Focus

1. **同一邮箱并发注册**：两个请求同时提交，最终只能留一条待验证记录；发信失败补偿不能删掉并发重提交写入的新记录（Task 5 的 `TestRegisterVerificationConcurrentSameEmail`、`TestRegisterCompensationDoesNotDeleteReplacedPending`）。
2. **验证时邮箱/用户名被真实用户抢占**：提交后、点链接前被人注册走，验证必须返回具体 409 并删掉待验证记录，不能建出半个账号（Task 6 的 `TestVerifyPendingEmailTaken` / `TestVerifyPendingNicknameTaken` / `TestVerifyConcurrentSameNickname`）。
3. **邮件设置读取失败/坏配置**：`Configured` 或 `Compose` 读设置失败时注册必须失败，不能退回默认主题或日志发信；注册发信必须拒绝日志降级（Task 3 的 `TestSettingsTransportConfiguredReadError`、Task 4 的 `TestComposeSettingsReadError`、Task 5 的 `TestRegisterDoesNotUseLogSender`）。
4. **`site.url` 为空、带路径、带尾斜杠、带 query/fragment**：空/非法必须 fail-closed；带路径时链接必须正确拼在路径前缀下；query/fragment 必须拒绝（Task 3 的 `TestSiteURL`、Task 5 的 `TestRegisterWithoutSiteURLFailsClosed` / `TestRegisterBuildsVerifyLinkWithSitePath`）。
5. **待验证用户的登录提示**：正确密码 → `email_not_verified`；错误密码、过期记录、用户名登录都必须是 `invalid_credentials`，不能变成账号枚举 oracle（Task 7 的四个用例 + 失败计数用例）。

---

### Task 1: API 错误码

**Files:**
- Modify: `backend/internal/common/apierror/apierror.go`
- Modify: `backend/internal/common/apierror/apierror_test.go`

**Interfaces:**
- Consumes: 现有 `Error` / `statusByCode` 注册表。
- Produces:
  - `ErrMailUnavailable`（503 / `server_error` / `mail_unavailable`）
  - `ErrEmailVerificationInvalid`（400 / `invalid_request_error` / `email_verification_invalid` / `param:"token"`）
  - `ErrEmailNotVerified`（403 / `permission_error` / `email_not_verified`）

- [ ] **Step 1: 写失败测试**

在 `apierror_test.go` 的 `TestRegistryMatchesSpec` 表里加三条（现有表是 6 个字段：`name` / `sentinel` / `wantType` / `wantCode` / `wantCode2` / `wantParam`）：

```go
{"mail_unavailable", ErrMailUnavailable, TypeServer, "mail_unavailable", http.StatusServiceUnavailable, nil},
{"email_verification_invalid", ErrEmailVerificationInvalid, TypeInvalidRequest, "email_verification_invalid", http.StatusBadRequest, strPtr("token")},
{"email_not_verified", ErrEmailNotVerified, TypePermission, "email_not_verified", http.StatusForbidden, nil},
```

`TestRegistryIntegrity` 的 `all` 切片加这三个哨兵，`len(all)` 与 `len(statusByCode)` 都从 36 改成 39（现读现改，不猜）。

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/common/apierror/ -run TestRegistry -v`
Expected: FAIL（三个变量未定义）。

- [ ] **Step 3: 加哨兵与状态映射**

在 `apierror.go` 的 `var (...)` 里加三条，在 `statusByCode` 里加三行。其余不改。

- [ ] **Step 4: 跑测试确认通过**

Run: `bash scripts/test.sh ./internal/common/apierror/ -v`
Expected: PASS。

- [ ] **Step 5: 核对 diff，不提交**

Run: `git diff -- backend/internal/common/apierror`
Expected: 只有三个哨兵、三行状态映射、两处测试改动。

---

### Task 2: 数据层（用户状态列 + 待验证表 + 迁移）

**Files:**
- Modify: `backend/internal/database/ent/schema/user.go`
- Create: `backend/internal/database/ent/schema/pending_registration.go`
- Modify: `backend/internal/modules/auth/repository.go`
- Modify: `backend/internal/modules/auth/admin.go`
- Modify: `backend/internal/modules/auth/service.go`
- Modify: `backend/internal/modules/auth/totp_schema_test.go`
- Modify: `backend/internal/modules/access/service_test.go`
- Modify: `backend/internal/modules/access/migration_test.go`
- Modify: `backend/internal/modules/settings/controller_test.go`
- Modify: `backend/internal/app/app_test.go`
- Create: `backend/migrations/<timestamp>_email_verification.sql`
- Modify: `backend/migrations/atlas.sum`
- Create: `backend/internal/modules/auth/pending_registration_schema_test.go`

**Interfaces:**
- Produces:
  - `users.email_verification_status`（非空字符串，无默认值）
  - 状态常量（放 `auth/repository.go` 或新文件）：`emailVerificationVerified = "verified"`、`emailVerificationNotRequired = "not_required"`
  - `func (r *repository) createUser(ctx context.Context, email, nickname, passwordHash, emailVerificationStatus string) (*ent.User, error)`
  - `PendingRegistration` ent 类型，字段：`id` / `email` / `nickname` / `password_hash` / `token_hash` / `expires_at` / `created_at`

- [ ] **Step 1: 写失败测试**

新建 `pending_registration_schema_test.go`：

```go
func TestPendingRegistrationSchema(t *testing.T) {
    db := testDB(t)
    ctx := context.Background()
    row, err := db.Client.PendingRegistration.Create().
        SetEmail("p+" + uniqueTag(t) + "@test.local").
        SetNickname("p" + uniqueTag(t)).
        SetPasswordHash("hash").
        SetTokenHash("token-" + uniqueTag(t)).
        SetExpiresAt(time.Now().Add(30 * time.Minute)).
        Save(ctx)
    if err != nil { t.Fatalf("create pending: %v", err) }
    t.Cleanup(func() { _ = db.Client.PendingRegistration.DeleteOneID(row.ID).Exec(context.Background()) })

    if row.Email == "" || row.PasswordHash != "hash" { t.Fatalf("roundtrip = %+v", row) }
}

func TestUserRequiresEmailVerificationStatus(t *testing.T) {
    db := testDB(t)
    _, err := db.Client.User.Create().
        SetEmail("nostatus+" + uniqueTag(t) + "@test.local").
        SetNickname("ns" + uniqueTag(t)).
        SetPasswordHash("hash").
        Save(context.Background())
    if err == nil { t.Fatal("user without email_verification_status saved, want validation error") }
}

func TestUserRejectsUnknownEmailVerificationStatus(t *testing.T) {
    db := testDB(t)
    _, err := db.Client.User.Create().
        SetEmail("badstatus+" + uniqueTag(t) + "@test.local").
        SetNickname("bs" + uniqueTag(t)).
        SetPasswordHash("hash").
        SetEmailVerificationStatus("pending").
        Save(context.Background())
    if err == nil { t.Fatal("unknown email_verification_status saved, want validation error") }
}

func TestEmailVerificationStatusColumnShape(t *testing.T) {
    db := testDB(t)
    ctx := context.Background()
    var nullable string
    if err := db.SQL().QueryRowContext(ctx,
        `SELECT is_nullable FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'email_verification_status'`,
    ).Scan(&nullable); err != nil { t.Fatalf("column lookup: %v", err) }
    if nullable != "NO" { t.Fatalf("is_nullable = %q, want NO", nullable) }
    empty, err := db.Client.User.Query().Where(user.EmailVerificationStatusEQ("")).Count(ctx)
    if err != nil { t.Fatalf("count empty status: %v", err) }
    if empty != 0 { t.Fatalf("users with empty status = %d, want 0", empty) }
}

func TestCreateAdminUserMarksNotRequiredAndClearsPending(t *testing.T) {
    db := testDB(t)
    tag := uniqueTag(t)
    email := "adminpending+" + tag + "@example.com"
    pending, err := db.Client.PendingRegistration.Create().
        SetEmail(email).SetNickname("ap"+tag).SetPasswordHash("hash").
        SetTokenHash("token-"+tag).SetExpiresAt(time.Now().Add(30 * time.Minute)).Save(context.Background())
    if err != nil { t.Fatalf("seed pending: %v", err) }
    t.Cleanup(func() { _ = db.Client.PendingRegistration.DeleteOneID(pending.ID).Exec(context.Background()) })

    if _, err := CreateAdminUser(context.Background(), db, email, "ap"+tag, "pass1234"); err != nil {
        t.Fatalf("CreateAdminUser: %v", err)
    }
    t.Cleanup(func() { _ = db.Client.User.Delete().Where(user.EmailEQ(email)).Exec(context.Background()) })

    row, err := db.Client.User.Query().Where(user.EmailEQ(email)).Only(context.Background())
    if err != nil { t.Fatalf("find user: %v", err) }
    if row.EmailVerificationStatus != "not_required" { t.Fatalf("status = %q, want not_required", row.EmailVerificationStatus) }
    count, err := db.Client.PendingRegistration.Query().Where(pendingregistration.EmailEQ(email)).Count(context.Background())
    if err != nil { t.Fatalf("count pending: %v", err) }
    if count != 0 { t.Fatalf("pending rows = %d, want 0", count) }
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestPendingRegistrationSchema|TestUserRequiresEmailVerificationStatus|TestUserRejectsUnknownEmailVerificationStatus|TestEmailVerificationStatusColumnShape|TestCreateAdminUserMarksNotRequired' -v`
Expected: FAIL（`PendingRegistration` 未定义 / 用户无状态也能保存 / 未知状态也能保存）。

- [ ] **Step 3: 改 ent schema 并生成代码**

`user.go` 在 `password_hash` 后加（`Validate` 用字面量，避免 ent schema 反向依赖 auth 包）：

```go
field.String("email_verification_status").NotEmpty().Validate(func(value string) error {
    if value != "verified" && value != "not_required" {
        return fmt.Errorf("invalid email_verification_status %q", value)
    }
    return nil
}),
```

新建 `pending_registration.go`：

```go
type PendingRegistration struct{ ent.Schema }

func (PendingRegistration) Fields() []ent.Field {
    return []ent.Field{
        field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }),
        field.String("email").NotEmpty().Unique(),
        field.String("nickname").NotEmpty(),
        field.String("password_hash").NotEmpty(),
        field.String("token_hash").NotEmpty().Unique(),
        field.Time("expires_at"),
        field.Time("created_at").Default(time.Now).Immutable(),
    }
}

func (PendingRegistration) Indexes() []ent.Index {
    return []ent.Index{index.Fields("expires_at")}
}
```

Run: `bash scripts/db-generate.sh`
Expected: `internal/database/ent/` 下生成 `PendingRegistration` 相关文件，`user_create` 出现 `SetEmailVerificationStatus`。

- [ ] **Step 4: 生成并手改迁移**

Run:

```bash
ATLAS_DEV_URL="${ATLAS_DEV_URL:-postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_atlas_dev?sslmode=disable}"
atlas migrate diff email_verification --dir "file://migrations" --to "ent://internal/database/ent/schema" --dev-url "$ATLAS_DEV_URL"
```

把生成的「加列」语句改成三步，**列类型保留 diff 生成的实际类型（通常是 `character varying`），不要手写 `text`**：

```sql
ALTER TABLE users ADD COLUMN email_verification_status character varying;
UPDATE users SET email_verification_status = 'not_required' WHERE email_verification_status IS NULL;
ALTER TABLE users ALTER COLUMN email_verification_status SET NOT NULL;
```

保留生成的 `CREATE TABLE pending_registrations` 与两个唯一索引、一个 `expires_at` 索引。然后：

```bash
set -a; source .env; set +a
atlas migrate hash --dir "file://migrations"
atlas migrate validate --dir "file://migrations"
bash scripts/db-migrate-apply.sh
```

- [ ] **Step 5: 改所有建号路径**

`repository.createUser` 增加第五个参数，并把它包进一个事务：先按邮箱删掉 `pending_registrations` 里可能残留的同邮箱记录，再 `SetEmailVerificationStatus(emailVerificationStatus).Save`。这样开关关时的注册、`CreateAdminUser` 都自动清掉旧待验证记录。调用点：

- `auth/admin.go` 的 `CreateAdminUser` → `emailVerificationNotRequired`；
- `auth/service.go` 的 `Register` → 暂时传 `emailVerificationNotRequired`（Task 5 再改分叉）。

所有测试里直接 `db.Client.User.Create()` 的位置补 `.SetEmailVerificationStatus("not_required")`：`auth/totp_schema_test.go`（3 处）、`access/service_test.go`、`access/migration_test.go`、`settings/controller_test.go`、`app/app_test.go`（2 处）。同时处理 `app_test.go:1025` 的裸 SQL：

```sql
INSERT INTO users (id, email, nickname, password_hash, email_verification_status, group_id, created_at, updated_at)
VALUES (..., 'not_required', ...)
```

核对命令：

```bash
grep -rn "User.Create()\|INSERT INTO users" internal --include=*.go | grep -v "ent/"
```

在 `settings/controller_test.go` 再加 `TestRegisterSettingAllowsEnableWithoutSMTP`：owner PATCH `/admin/settings/register` 写 `{"email_verification_enabled": true}` 返回 200（D14：后端不做跨组校验，运行期由注册 503 兜住）。

- [ ] **Step 6: 跑测试确认通过**

Run: `bash scripts/test.sh ./internal/modules/auth/ ./internal/modules/access/ ./internal/modules/settings/ ./internal/app/ -run 'Schema|Migration|TestCreateAdminUser|TestUserRejectsUnknownEmailVerificationStatus|TestEmailVerificationStatusColumnShape|TestUpdateProfileConstraintBackstop|TestRegisterSettingAllowsEnableWithoutSMTP' -v`
Expected: PASS。

- [ ] **Step 7: 核对 diff，不提交**

Run: `git status --short && git diff --stat`
Expected: ent 生成文件、迁移、auth/access/settings/app 测试改动；没有业务行为变化。

---

### Task 3: 邮件层 `Configured` + `SiteURL`

**Files:**
- Modify: `backend/internal/modules/mail/mailer.go`
- Modify: `backend/internal/modules/mail/settings.go`
- Modify: `backend/internal/modules/mail/smtp.go`
- Modify: `backend/internal/modules/mail/logsender.go`
- Create: `backend/internal/modules/mail/link.go`
- Modify: `backend/internal/modules/mail/controller_test.go`（`stubMailer` 补方法）
- Create: `backend/internal/modules/mail/link_test.go`
- Modify: `backend/internal/modules/mail/settings_test.go` / `smtp_test.go` / `mailer_test.go`（`stubTransport` 与 `funcTransport` 也要补 `configured`）

**Interfaces:**
- Produces:
  - `Mailer.Configured(ctx context.Context) (bool, error)`
  - `transport.configured(ctx context.Context) (bool, error)`
  - `func SiteURL(raw string) (string, error)`：trim 空白、只接受 http/https、要求 host、允许 path、去掉尾斜杠
- Consumes: 现有 `resolveTransportConfig`、`newLogSender`、`newSMTPTransport`。

- [ ] **Step 1: 写失败测试**

`link_test.go`：

```go
func TestSiteURL(t *testing.T) {
    cases := []struct{ in, want string; ok bool }{
        {"https://example.com", "https://example.com", true},
        {"https://example.com/", "https://example.com", true},
        {"https://example.com/blog/", "https://example.com/blog", true},
        {"http://localhost:3000", "http://localhost:3000", true},
        {"", "", false},
        {"example.com", "", false},
        {"ftp://example.com", "", false},
        {"javascript:alert(1)", "", false},
        {"https://", "", false},
        {"http://:8080", "", false},
        {"https://example.com/?a=1", "", false},
        {"https://example.com/#x", "", false},
    }
    for _, c := range cases {
        got, err := SiteURL(c.in)
        if (err == nil) != c.ok || got != c.want {
            t.Fatalf("SiteURL(%q) = (%q, %v), want (%q, ok=%v)", c.in, got, err, c.want, c.ok)
        }
    }
}
```

`settings_test.go` 加：

```go
func TestSettingsTransportConfiguredReadError(t *testing.T) {
    tr := newSettingsTransport(config.Config{}, &controllerSettingsStub{errs: map[string]error{"mail": errors.New("db down")}}, discardLogger())
    ok, err := tr.configured(context.Background())
    if err == nil || ok { t.Fatalf("configured = (%v, %v), want error", ok, err) }
}
```

再加 `TestSettingsTransportConfiguredEnvFallback`：设置组 SMTP 主机为空、环境变量 `SMTP_HOST` 有但 `SMTP_FROM` 空时，`configured` 必须为 false 或返回错误（环境变量回退也要过 `validateEffectiveConfig`）。

`mailer_test.go` 加：日志 sender 的 `Configured` 为 `false`；SMTP transport 的 `Configured` 在 `initErr == nil` 时为 `true`；`stubTransport` 与 `funcTransport` 补 `configured() (bool, error)`（分别返回 true / 用例指定值）。

`settings_test.go` 再加 `TestSettingsTransportRejectsRequireSMTPWhenLog`：`Message{RequireSMTP: true}` + 设置解析为日志降级时，`deliver` 必须返回错误，且日志 buffer 里不能出现正文。

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/mail/ -run 'TestSiteURL|TestSettingsTransportConfigured|Configured' -v`
Expected: FAIL（方法/函数未定义）。

- [ ] **Step 3: 实现**

- `Mailer` 接口加 `Configured(ctx context.Context) (bool, error)`；`mailer` 委托 `transport.configured`。
- `transport` 接口加 `configured(ctx context.Context) (bool, error)`。
- `Message` 增加 `RequireSMTP bool`；`settingsTransport.deliver` 解析出 `send=false` 且 `msg.RequireSMTP` 为真时直接返回错误（例如 `errSMTPRequired`），不要走 `logSender`；日志里不能出现正文。
- `settingsTransport.configured`：调 `resolveTransportConfig`，返回它的 `send` 与错误（错误原样上抛）。环境变量回退分支在返回 `send=true` 前也必须跑 `validateEffectiveConfig(env)`；缺 sender / sender 非法 / username 有但 password 空都要返回错误，不能报「已配置」。
- `smtpTransport.configured`：`return t.initErr == nil, nil`。
- `logSender.configured`：`return false, nil`。
- `link.go` 实现 `SiteURL`（`net/url`；scheme 必须 `http`/`https`；`u.Hostname() != ""`；`u.RawQuery == ""` 且 `u.Fragment == ""`；用 `u.Scheme + "://" + u.Host + strings.TrimRight(u.Path, "/")` 重建，允许 path 前缀）。
- `stubMailer` 加 `configured bool` / `configuredErr error` 与 `Configured` 方法；把 `mail/controller_test.go` 里 8 处 `&stubMailer{}` 逐个补 `configured: true`（status 的 false 用例单独写）。`mailer_test.go` 的 `stubTransport` / `funcTransport` 也补 `configured()`。

- [ ] **Step 4: 跑测试确认通过**

Run: `bash scripts/test.sh ./internal/modules/mail/ -v`
Expected: PASS。

- [ ] **Step 5: 核对 diff，不提交**

---

### Task 4: `Compose` + 模板改链接 + 邮件状态端点

**Files:**
- Modify: `backend/internal/modules/mail/controller.go`
- Modify: `backend/internal/modules/mail/dto.go`
- Modify: `backend/internal/modules/mail/templates/signup_code.html`
- Modify: `backend/internal/modules/mail/templates/signup_code.txt`
- Modify: `backend/internal/modules/mail/controller_test.go`
- Modify: `backend/internal/modules/mail/template_test.go`
- Modify: `backend/internal/app/app_test.go`（`TestMailSettingsEndToEnd` 的旧主题断言）

**Interfaces:**
- Produces:
  - `func Compose(ctx context.Context, settings Settings, templateName string, data map[string]string) (string, Body, error)`
  - `const TemplateSignupCode = "signup_code"`
  - `GET /admin/mail/status` → `{"smtp_configured":bool,"site_url_configured":bool}`，守卫 `system.register.view`
- Consumes: Task 3 的 `Mailer.Configured` / `SiteURL`。

- [ ] **Step 1: 写失败测试**

`controller_test.go`：

- 更新 `TestCreateMessageRendersSelectedTemplate`：
  - 自定义主题断言保留（`自定义注册验证码`）。
  - 删掉旧的 `注册验证码` / `123456` / `10` 文本断言，改成默认 txt 模板里的链接：
    ```go
    if !strings.Contains(mailer.got.Body.Text, "https://example.com/signup/verify?token=preview-token") { ... }
    ```
  - 自定义 HTML 正文里的 `{{.code}}` 仍渲染 `123456`（该用例的 custom body 用 `code`，保留；要测 `verifyUrl` 另加一条）。
- 新增 `TestCreateMessageUsesDefaultSignupSubject`：不带 mail 设置时 `signup_code` 的主题是「注册验证邮件」。
- 新增 `TestMailStatusSMTPFalseAndReadError`：`stubMailer{configured:false}` → `smtp_configured=false`；`stubMailer{configuredErr: ...}` 或 `site` 组读设置失败 → 503/500 错误响应，不能返回 200。
- 新增 `TestComposeSettingsReadError`：

```go
func TestComposeSettingsReadError(t *testing.T) {
    settings := &controllerSettingsStub{errs: map[string]error{"mail": errors.New("db down")}}
    if _, _, err := Compose(context.Background(), settings, TemplateSignupCode, map[string]string{}); err == nil {
        t.Fatal("Compose returned nil error when mail settings read failed")
    }
}
```

- 新增 `TestComposeRejectsBadCustomBody`：自定义正文是坏模板（如 `{{`）→ `Compose` 返回错误，不回退默认正文。
- `backend/internal/app/app_test.go:1372` 的 `TestMailSettingsEndToEnd` 把主题断言从 `注册验证码` 改成 `注册验证邮件`。
- `controller_test.go` 里现有 `&stubMailer{}` 补 `configured: true`（status 的 false 用例除外）。

新增：

```go
func TestMailStatus(t *testing.T) {
    settings := &controllerSettingsStub{groups: map[string]map[string]any{
        "site": {"url": "https://example.com"},
    }}
    mailer := &stubMailer{configured: true}
    ts := newControllerTestServer(t, mailer, rbac.GroupKeyOwner, settings)
    resp, err := http.Get(ts.URL + "/admin/mail/status")
    if err != nil { t.Fatalf("get: %v", err) }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK { t.Fatalf("status = %d", resp.StatusCode) }
    var body struct{ SMTPConfigured bool `json:"smtp_configured"`; SiteURLConfigured bool `json:"site_url_configured"` }
    _ = json.NewDecoder(resp.Body).Decode(&body)
    if !body.SMTPConfigured || !body.SiteURLConfigured { t.Fatalf("body = %+v", body) }
}
```

再写两个断言：`site.url` 为空时 `site_url_configured=false`；非 owner/admin 组 GET 返回 403。

新增 `TestComposeSettingsReadError`：mail 组读失败时 `Compose` 返回错误（不能回退默认主题）。

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/mail/ -run 'TestCreateMessage|TestMailStatus|TestCompose' -v`
Expected: FAIL。

- [ ] **Step 3: 实现**

- 把 `controller.renderMessage` 的主体抽成包级 `Compose`；mail 组读失败直接返回错误（不再 `slog.Warn` 后回退）。**保留 `settings == nil` 分支**：nil 时只做默认模板渲染，不读 mail/site——现有 `TestCreateMessageDeliversSelfCheck`、`TestCreateMessageUsesBuiltinTemplateDefaults`、`TestCreateMessageMapsDeliveryFailure` 都依赖它。
- `controller.create` 改调 `Compose`。
- `previewSubjects[TemplateSignupCode] = "注册验证邮件"`。
- `previewData` 改成 `(map[string]string, error)`：settings 非 nil 时 `site` 组读失败必须上抛，不再 `slog.Warn` + `https://example.com` 回退；settings 为 nil 时 `siteTitle` / `siteUrl` 用空字符串。`expiresMinutes` 用 `"30"`。返回值增加 `"verifyUrl": "https://example.com/signup/verify?token=preview-token"`（保留 `"code": "123456"` 给密码重置）。
- 模板正文改成链接文案，占位符 `{{.verifyUrl}}` + `{{.expiresMinutes}}` + `{{.siteTitle}}` + `{{.siteUrl}}` + `{{.username}}` + `{{.sentAt}}`。
- `dto.go` 加 `statusResponse{SMTPConfigured bool; SiteURLConfigured bool}`（JSON 键见 Interfaces）。
- `registerProtected` 加 `r.GET("/admin/mail/status", guard.RequirePermission(rbac.PermissionRegisterView), c.status)`。
- `status`：先调 `c.mailer.Configured`（错误上抛），再读 `site` 组并 `SiteURL` 判 `site_url_configured`（读失败上抛；settings 为 nil 时 `site_url_configured=false`）。
- `newControllerTestServer` 的身份补 `Permissions: rbac.PresetPermissions(groupKey)`，否则 `RequirePermission` 拿不到权限。

- [ ] **Step 4: 跑测试确认通过**

Run: `bash scripts/test.sh ./internal/modules/mail/ -v`
Expected: PASS。

- [ ] **Step 5: 核对 diff，不提交**

---

### Task 5: 注册分叉（202 + 待验证记录 + 发信）

**Files:**
- Modify: `backend/internal/modules/auth/repository.go`
- Modify: `backend/internal/modules/auth/service.go`
- Modify: `backend/internal/modules/auth/dto.go`
- Modify: `backend/internal/modules/auth/controller.go`
- Modify: `backend/internal/modules/auth/module.go`
- Modify: `backend/internal/modules/auth/auth_test.go`
- Modify: `backend/internal/modules/auth/totp_flow_test.go`
- Modify: `backend/internal/modules/auth/captcha_flow_test.go`（如直接调 helper）
- Modify: `backend/internal/app/app.go`
- Create: `backend/internal/modules/auth/register_verification_test.go`

**Interfaces:**
- Consumes: Task 2 的 `createUser` / `PendingRegistration`；Task 3 的 `Mailer.Configured` / `SiteURL`；Task 4 的 `mail.Compose` / `TemplateSignupCode`。
- Produces:
  - `type RegisterResult struct { User *User; VerificationPending bool }`
  - `func (s *Service) Register(...) (RegisterResult, error)`
  - `func newService(..., policies rateLimitPolicies, verifier captchaVerifier, mailer mail.Mailer) *Service`
  - `func (r *repository) replacePendingRegistration(ctx context.Context, in pendingRegistrationInput) (pendingRegistrationRecord, error)`
  - `func (r *repository) deletePendingRegistration(ctx context.Context, id uuid.UUID, tokenHash string) error`
  - `type pendingRegistrationRecord struct { ID uuid.UUID; TokenHash string }`
  - `func newPendingToken() (raw string, hash string, err error)`
  - `func hashPendingToken(raw string) string`
  - `type pendingRegistrationInput struct { Email, Nickname, PasswordHash, TokenHash string; ExpiresAt time.Time }`
  - 常量 `pendingRegistrationTTL = 30 * time.Minute`、`pendingTokenBytes = 32`
  - `mail.Message` 增加 `RequireSMTP bool`（注册发信置 true；传输层解析为日志降级时直接报错）
  - `func (s *Service) registrationEmailData(ctx context.Context, nickname string) (map[string]string, error)`
  - `func (s *Service) compensatePending(ctx context.Context, record pendingRegistrationRecord)`

- [ ] **Step 1: 写失败测试**

新建 `register_verification_test.go`，至少覆盖：

```go
func TestRegisterCreatesPendingInsteadOfUser(t *testing.T) {
    db := testDB(t)
    tag := uniqueTag(t)
    mailer := &fakeMailer{configured: true}
    ts, _, settings := newTestAppWithMailer(t, db, mailer)
    enableEmailVerification(t, settings)
    setSiteURL(t, settings, "https://example.com")

    before := userCount(t, db)
    email := "verify+" + tag + "@example.com"
    resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users",
        RegisterRequest{Email: email, Nickname: "v" + tag, Password: "pass1234"})
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusAccepted { t.Fatalf("status = %d, want 202", resp.StatusCode) }
    rawBody, _ := io.ReadAll(resp.Body)
    if strings.Contains(string(rawBody), email) { t.Fatalf("202 leaked email: %s", rawBody) }
    var body map[string]any
    _ = json.Unmarshal(rawBody, &body)
    if len(body) != 1 || body["status"] != "verification_pending" { t.Fatalf("body = %+v", body) }
    if after := userCount(t, db); after != before { t.Fatalf("users = %d, want %d", after, before) }
    if pendingCount(t, db, email) != 1 { t.Fatal("want one pending registration") }
    row, err := db.Client.PendingRegistration.Query().Where(pendingregistration.EmailEQ(email)).Only(context.Background())
    if err != nil { t.Fatalf("find pending: %v", err) }
    if bcrypt.CompareHashAndPassword([]byte(row.PasswordHash), []byte("pass1234")) != nil { t.Fatal("pending password hash does not verify") }
    if len(row.TokenHash) != 64 { t.Fatalf("token hash length = %d, want 64", len(row.TokenHash)) }
    if row.ExpiresAt.Before(time.Now().Add(29*time.Minute)) || row.ExpiresAt.After(time.Now().Add(31*time.Minute)) {
        t.Fatalf("expires_at = %v, want ~30m", row.ExpiresAt)
    }
    mails := mailer.sent()
    if len(mails) != 1 { t.Fatalf("mails = %d, want 1", len(mails)) }
    match := regexp.MustCompile(`token=([A-Za-z0-9_-]+)`).FindStringSubmatch(mails[0].Body.Text)
    if match == nil { t.Fatal("mail has no verify token") }
    if row.TokenHash != hashPendingToken(match[1]) { t.Fatal("stored token hash does not match mailed token") }
    if !mails[0].RequireSMTP { t.Fatal("verification mail must require SMTP") }
}
```

再写：

- `TestRegisterVerificationOffCreatesNotRequiredUser`：开关关 → 201、查 ent 用户状态 `not_required`、零发信、零待验证。
- `TestRegisterWithoutSMTPFailsClosed`：`configured=false` → 503 `mail_unavailable`、零用户、零待验证、零发信。
- `TestRegisterWithoutSiteURLFailsClosed`：`site.url=""` → 503、零记录。
- `TestRegisterInvalidSiteURL`：表驱动 `example.com`、`ftp://example.com`、`javascript:alert(1)`、`http://:8080`、`https://example.com/?a=1`、`https://example.com/#x` → 全部 503、零记录。
- `TestRegisterBuildsVerifyLinkWithSitePath`：

```go
func TestRegisterBuildsVerifyLinkWithSitePath(t *testing.T) {
    db := testDB(t)
    tag := uniqueTag(t)
    mailer := &fakeMailer{configured: true}
    ts, _, settings := newTestAppWithMailer(t, db, mailer)
    enableEmailVerification(t, settings)
    setSiteURL(t, settings, "https://example.com/blog/")
    email := "path+" + tag + "@example.com"
    postJSON(t, testClient(t), ts.URL+"/api/v1/users", RegisterRequest{Email: email, Nickname: "p" + tag, Password: "pass1234"}).Body.Close()
    mails := mailer.sent()
    if len(mails) != 1 { t.Fatalf("mails = %d, want 1", len(mails)) }
    if !strings.Contains(mails[0].Body.Text, "https://example.com/blog/signup/verify?token=") {
        t.Fatalf("body = %q", mails[0].Body.Text)
    }
}
```
- `TestRegisterSendFailureDeletesPending`：`sendSyncErr` → 503、零用户、零待验证。
- `TestRegisterCompensationDoesNotDeleteReplacedPending`（Review Focus 1）：先写入记录 A，再用 B 覆盖同邮箱，最后调 `deletePendingRegistration(A.ID, A.TokenHash)`；断言 B 的记录仍在、B 的 token 仍可用。
- `TestRegisterResubmitReplacesPending`：同邮箱两次 → 202、待验证仍一条、两封邮件的 token 不同、库里 token_hash 等于第二封。
- `TestRegisterVerificationConcurrentSameEmail`（Review Focus 1）：参考现有 `auth_test.go` 的 `TestRegisterConcurrentSameEmail` 写法（不要在新 goroutine 里调用会 `t.Fatalf` 的 helper）；两个 goroutine 同时 POST 同邮箱；两个都 202；待验证 1 条；最终 token_hash 等于其中一封信里的令牌。
- `TestRegisterWithPendingAndSwitchOffCreatesUser`：先造待验证，关开关，再 POST 同邮箱 → 201 `not_required`，待验证 0，库里旧 token_hash 被替换（旧 token 的 400 断言放 Task 6）。
- `TestRegisterVerificationEnabledRejectsExistingEmail` / `...Nickname`：开关开 + 真实用户已占用 → 409、零待验证、零发信。
- `TestRegisterAllowsSameNicknameAcrossPending`：两个不同邮箱、同一昵称 → 两次 202、待验证 2 条、用户 0。
- `TestRegisterDoesNotUseLogSender`：断言发出的 `mail.Message.RequireSMTP == true`；传输层拒绝日志降级的行为在 Task 3 的 settings transport 用例覆盖。
（环境变量回退不完整由 Task 3 的 `TestSettingsTransportConfiguredEnvFallback` 覆盖；auth 层只测 `Configured=false`。）

测试助手（放 `register_verification_test.go`）：

```go
type fakeMailer struct {
    mu sync.Mutex
    configured bool
    configuredErr error
    sendSyncErr error
    messages []mail.Message
}
func (f *fakeMailer) Configured(context.Context) (bool, error) { return f.configured, f.configuredErr }
func (f *fakeMailer) Send(context.Context, mail.Message) error { return nil }
func (f *fakeMailer) SendSync(_ context.Context, m mail.Message) error {
    f.mu.Lock()
    defer f.mu.Unlock()
    if f.sendSyncErr != nil { return f.sendSyncErr }
    f.messages = append(f.messages, m)
    return nil
}
func (f *fakeMailer) sent() []mail.Message {
    f.mu.Lock()
    defer f.mu.Unlock()
    return append([]mail.Message(nil), f.messages...)
}
func (f *fakeMailer) Close(context.Context) error { return nil }

func enableEmailVerification(t *testing.T, svc *settings.Service) { /* SetBool + t.Cleanup ResetItem */ }
func setSiteURL(t *testing.T, svc *settings.Service, value string) { /* SetString + t.Cleanup ResetItem */ }
func pendingCount(t *testing.T, db *database.Database, email string) int { /* Query Count */ }
```

`auth_test.go` 的测试助手改成这条链：

- `newTestAppWithCaptchaVerifier(..., verifier captchaVerifier, mailer mail.Mailer)`；
- 新增 `newTestAppWithSettingsAndMailer(t, db, settingsService, limiter, policies, mailer)`，它调上面的 helper（`verifier=nil`）；
- 新增 `newTestAppWithMailer(t, db, mailer)`：`settings.New(db)` → `newTestAppWithSettingsAndMailer(t, db, svc, ratelimit.New(), disabledRateLimitPolicies(), mailer)`（必须用 `disabledRateLimitPolicies`，否则注册邮箱 3 次/小时会把并发/重复提交用例互相污染）；
- 现有 `newTestAppWithSettings` 传 `&fakeMailer{configured: true}`。

`totp_flow_test.go` 的直接 `newService` 调用补一个 `&fakeMailer{configured: true}`。所有调用点跟着改。

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestRegister(CreatesPending|VerificationOff|WithoutSMTP|WithoutSiteURL|InvalidSiteURL|BuildsVerifyLink|SendFailure|Compensation|Resubmit|VerificationConcurrent|WithPendingAndSwitchOff|VerificationEnabledRejects|AllowsSameNickname|DoesNotUseLogSender)' -v`
Expected: FAIL（202 未实现、mailer 参数不存在）。

- [ ] **Step 3: 实现**

- `Service` 加 `mailer mail.Mailer`；`newService` 末尾加参数；`auth.New` 末尾加参数；`app.go` 传 `mailModule.Mailer`。
- `Register` 在现有「邮箱/用户名占用」检查之后分叉：

```go
verificationEnabled, err := s.settings.GetBool(ctx, "register", "email_verification_enabled")
if err != nil { return RegisterResult{}, err }
if !verificationEnabled {
    hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    if err != nil { return RegisterResult{}, err }
    created, err := s.repo.createUser(ctx, email, nickname, string(hash), emailVerificationNotRequired)
    if err != nil {
        if ent.IsConstraintError(err) { return RegisterResult{}, apierror.ErrAlreadyTaken }
        return RegisterResult{}, err
    }
    user := toUser(created)
    return RegisterResult{User: &user}, nil
}
configured, err := s.mailer.Configured(ctx)
if err != nil || !configured { return RegisterResult{}, apierror.ErrMailUnavailable }
data, err := s.registrationEmailData(ctx, nickname)
if err != nil { return RegisterResult{}, apierror.ErrMailUnavailable }
hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
if err != nil { return RegisterResult{}, err }
raw, tokenHash, err := newPendingToken()
if err != nil { return RegisterResult{}, err }
expiresAt := time.Now().Add(pendingRegistrationTTL)
record, err := s.repo.replacePendingRegistration(ctx, pendingRegistrationInput{
    Email: email, Nickname: nickname, PasswordHash: string(hash), TokenHash: tokenHash, ExpiresAt: expiresAt,
})
if err != nil { return RegisterResult{}, err }
data["verifyUrl"] = data["siteUrl"] + "/signup/verify?token=" + url.QueryEscape(raw)
subject, body, err := mail.Compose(ctx, s.settings, mail.TemplateSignupCode, data)
if err != nil {
    s.compensatePending(ctx, record)
    return RegisterResult{}, apierror.ErrMailUnavailable
}
if err := s.mailer.SendSync(ctx, mail.Message{To: email, Subject: subject, Body: body, RequireSMTP: true}); err != nil {
    s.compensatePending(ctx, record)
    return RegisterResult{}, apierror.ErrMailUnavailable
}
return RegisterResult{VerificationPending: true}, nil
```

`compensatePending` 只按本次写入行的 id + token_hash 删除；影响行数为 0 说明记录已被并发重提交覆盖，不要动新记录。删除失败只 `slog.Error("auth: delete pending registration", "err", err)`（不得带 token/明文密码），响应仍是 `mail_unavailable`。

- `replacePendingRegistration`：事务里先删全局过期行，再 `Create` + `OnConflict(sql.ConflictColumns("email"))` 显式更新 `nickname` / `password_hash` / `token_hash` / `expires_at`（不更新 `id` / `created_at`）；提交前按 email 回查该行，返回 `pendingRegistrationRecord{ID, TokenHash}`。`sql/upsert` 特性已在 `db-generate.sh` 开启。
- `deletePendingRegistration`：`WHERE id = ? AND token_hash = ?` 删除，返回影响行数或错误；补偿删除只能用这条，不能用邮箱。
- `newPendingToken`：`crypto/rand` 32 字节 → `base64.RawURLEncoding`；`hashPendingToken` 返回 `hex.EncodeToString(sha256.Sum256([]byte(raw)))`。
- `registrationEmailData` 读 `site` 组，要求 `title`/`url` 是字符串；`mail.SiteURL(url)` 校验并拿到规范化 base；返回 `{"siteTitle": title, "siteUrl": base, "username": nickname, "expiresMinutes": "30", "sentAt": time.Now().Format("2006-01-02 15:04:05"), "code": "", "verifyUrl": ""}`。`verifyUrl` 在拿到 token 后拼，`code` 留空。
- 开关关的建号路径直接走 Task 2 的 `createUser`——它已经在同一事务里删同邮箱待验证记录。
- Controller：

```go
result, err := c.service.Register(...)
if err != nil { apierror.Write(ctx, err); return }
if result.VerificationPending {
    ctx.JSON(http.StatusAccepted, RegisterPendingResponse{Status: "verification_pending"})
    return
}
ctx.JSON(http.StatusCreated, UserResponse{Email: result.User.Email, Nickname: result.User.Nickname, Bio: result.User.Bio})
```

- `dto.go` 加 `RegisterResult`（非 JSON）与 `RegisterPendingResponse{Status string \`json:"status"\`}`。

- [ ] **Step 4: 跑测试确认通过**

Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestRegister' -v`
Expected: PASS；既有注册/登录测试也全绿。

- [ ] **Step 5: 核对 diff，不提交**

---

### Task 6: 验证接口（`POST /users/email-verifications`）

**Files:**
- Modify: `backend/internal/modules/auth/repository.go`
- Modify: `backend/internal/modules/auth/service.go`
- Modify: `backend/internal/modules/auth/controller.go`
- Modify: `backend/internal/modules/auth/dto.go`
- Modify: `backend/internal/modules/auth/ratelimit.go`
- Modify: `backend/internal/modules/auth/register_verification_test.go`

**Interfaces:**
- Consumes: Task 5 的待验证表、`pendingRegistrationInput`、`hashPendingToken`。
- Produces:
  - `func (s *Service) VerifyEmailRegistration(ctx context.Context, token string, meta SessionMeta) (User, error)`
  - `type EmailVerificationRequest struct { Token string \`json:"token"\` }`
  - `POST /users/email-verifications`（body `{"token":"..."}`）→ 201 `UserResponse`
  - `rateLimitPolicies.EmailVerificationIP`（30 / 15 分钟）
  - 域哨兵（repository 只返回这些，不 import `apierror`）：`errPendingNotFound`、`errPendingEmailTaken`、`errPendingNicknameTaken`、`errPendingConflict`

- [ ] **Step 1: 写失败测试**

在 `register_verification_test.go` 加（下面的注释是场景说明，落成代码时必须删掉）：

```go
func TestVerifyEmailRegistration(t *testing.T) {
    db := testDB(t)
    tag := uniqueTag(t)
    mailer := &fakeMailer{configured: true}
    ts, _, settings := newTestAppWithMailer(t, db, mailer)
    enableEmailVerification(t, settings)
    setSiteURL(t, settings, "https://example.com")
    email := "verifyok+" + tag + "@example.com"
    postJSON(t, testClient(t), ts.URL+"/api/v1/users", RegisterRequest{Email: email, Nickname: "vo" + tag, Password: "pass1234"}).Body.Close()
    token := extractToken(t, mailer.sent()[0].Body.Text)
    client := testClient(t)
    resp := postJSON(t, client, ts.URL+"/api/v1/users/email-verifications", map[string]string{"token": token})
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusCreated { t.Fatalf("status = %d, want 201", resp.StatusCode) }
    row, err := db.Client.User.Query().Where(user.EmailEQ(email)).Only(context.Background())
    if err != nil { t.Fatalf("find user: %v", err) }
    if row.EmailVerificationStatus != "verified" { t.Fatalf("status = %q, want verified", row.EmailVerificationStatus) }
    if count := pendingCount(t, db, email); count != 0 { t.Fatalf("pending = %d, want 0", count) }
    if len(client.Jar.Cookies(mustURL(t, ts.URL))) != 0 { t.Fatal("verify must not create a session") }
}

func TestVerifyEmailRegistrationRejectsUnknownAndExpired(t *testing.T) {
    /* 未知 token → 400 email_verification_invalid；
       手工插一条 expires_at = now-1m 的记录 → 400，且该行被删掉 */
}

func TestVerifyEmailRegistrationConcurrent(t *testing.T) {
    /* 两个 goroutine 同上 token：恰好一个 201、一个 400；用户恰好 1 个 */
}

func TestVerifyPendingEmailTaken(t *testing.T) {
    db := testDB(t)
    tag := uniqueTag(t)
    mailer := &fakeMailer{configured: true}
    ts, _, settings := newTestAppWithMailer(t, db, mailer)
    enableEmailVerification(t, settings)
    setSiteURL(t, settings, "https://example.com")
    email := "taken+" + tag + "@example.com"
    postJSON(t, testClient(t), ts.URL+"/api/v1/users", RegisterRequest{Email: email, Nickname: "tk" + tag, Password: "pass1234"}).Body.Close()
    token := extractToken(t, mailer.sent()[0].Body.Text)
    _, err := db.Client.User.Create().SetEmail(email).SetNickname("other"+tag).SetPasswordHash("hash").
        SetEmailVerificationStatus("not_required").Save(context.Background())
    if err != nil { t.Fatalf("seed user: %v", err) }
    t.Cleanup(func() { _ = db.Client.User.Delete().Where(user.EmailEQ(email)).Exec(context.Background()) })
    resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users/email-verifications", map[string]string{"token": token})
    if resp.StatusCode != http.StatusConflict { t.Fatalf("status = %d, want 409", resp.StatusCode) }
    if code := decodeError(t, resp); code != "email_taken" { t.Fatalf("code = %q, want email_taken", code) }
    if pendingCount(t, db, email) != 0 { t.Fatalf("pending = %d, want 0", pendingCount(t, db, email)) }
}

func TestVerifyPendingNicknameTaken(t *testing.T) {
    db := testDB(t)
    tag := uniqueTag(t)
    mailer := &fakeMailer{configured: true}
    ts, _, settings := newTestAppWithMailer(t, db, mailer)
    enableEmailVerification(t, settings)
    setSiteURL(t, settings, "https://example.com")
    email := "nt+" + tag + "@example.com"
    nickname := "nt" + tag
    postJSON(t, testClient(t), ts.URL+"/api/v1/users", RegisterRequest{Email: email, Nickname: nickname, Password: "pass1234"}).Body.Close()
    token := extractToken(t, mailer.sent()[0].Body.Text)
    _, err := db.Client.User.Create().SetEmail("other+"+tag+"@example.com").SetNickname(nickname).SetPasswordHash("hash").
        SetEmailVerificationStatus("not_required").Save(context.Background())
    if err != nil { t.Fatalf("seed user: %v", err) }
    t.Cleanup(func() { _ = db.Client.User.Delete().Where(user.NicknameEQ(nickname)).Exec(context.Background()) })
    resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users/email-verifications", map[string]string{"token": token})
    if resp.StatusCode != http.StatusConflict { t.Fatalf("status = %d, want 409", resp.StatusCode) }
    if code := decodeError(t, resp); code != "nickname_taken" { t.Fatalf("code = %q, want nickname_taken", code) }
    if pendingCount(t, db, email) != 0 { t.Fatalf("pending = %d, want 0", pendingCount(t, db, email)) }
}

func TestVerifyConcurrentSameNickname(t *testing.T) {
    /* 两条不同邮箱、同一用户名的待验证记录；在 users 上给目标昵称装临时 BEFORE INSERT 触发器强制 unique_violation；
       并发验证两个 token；恰好一个 201、一个 409（nickname_taken 或 already_taken）；用户只有 1 个；
       失败那条的待验证记录仍在（回滚），drop 触发器后重试可成功 */
}

func TestVerifyOldTokenInvalidAfterResubmit(t *testing.T) {
    /* 两次提交取两个 token；第一个 → 400，第二个 → 201 */
}

func TestVerifyEmailRegistrationAfterSwitchOff(t *testing.T) {
    /* 开关开时创建待验证，关开关，验证 token → 201、状态 verified */
}

func TestVerifyRateLimit(t *testing.T) {
    /* newTestAppWithRatePolicies 把 EmailVerificationIP 设成 {Limit:1, Window:time.Hour}；
       用任意假 token 请求两次：第一次 400、第二次 429（限流发生在查库之前） */
}
```

`extractToken` / `mustURL` 是测试小助手（前者用 `token=([A-Za-z0-9_-]+)` 正则，后者封装 `url.Parse`）。

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestVerify' -v`
Expected: FAIL（路由 404 / 方法未定义）。

- [ ] **Step 3: 实现**

- `controller.register` 加 `r.POST("/users/email-verifications", c.verifyEmailRegistration)`。
- `verifyEmailRegistration`：`requireJSONRequest` → 绑定 `EmailVerificationRequest{Token string}` → 空 token → 400 `invalid_request`（param token）→ 调 service → 201 `UserResponse`。
- `VerifyEmailRegistration` 开头做 IP 限流（`ipKey("emailverify", meta.IP)`）。
- `repository.verifyPendingRegistration(ctx, tokenHash) (*ent.User, error)`（repository 不 import `apierror`，只返回域哨兵）：
  1. 按 `token_hash` 查行；不存在 → `errPendingNotFound`；存在但 `expires_at <= now()` → 先按 id 删除该行，再返回 `errPendingNotFound`。
  2. 开事务。
  3. 查真实用户邮箱/用户名占用：邮箱 → 删该待验证行 + 提交 + `errPendingEmailTaken`；用户名 → 同上 + `errPendingNicknameTaken`。
  4. `DELETE WHERE id = row.ID AND expires_at > now()`；`count != 1` → 回滚 + `errPendingNotFound`。
  5. `tx.User.Create(...).SetEmailVerificationStatus(emailVerificationVerified).Save(ctx)`；**必须用事务内的 `tx`，不能用 `r.client`**，否则 409 回滚会留下半个账号。
  6. 提交，返回用户。
  7. 唯一约束错误 → 回滚（记录恢复）+ `errPendingConflict`。
- Service 做映射：`errPendingNotFound` → `apierror.ErrEmailVerificationInvalid`；`errPendingEmailTaken` → `ErrEmailTaken`；`errPendingNicknameTaken` → `ErrNicknameTaken`；`errPendingConflict` → `ErrAlreadyTaken`。
- `rateLimitPolicies` 加 `EmailVerificationIP`，`defaultRateLimitPolicies` 加 30/15min；`disabledRateLimitPolicies` 保持零值。

- [ ] **Step 4: 跑测试确认通过**

Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestVerify' -v`
Expected: PASS。

- [ ] **Step 5: 核对 diff，不提交**

---

### Task 7: 登录「账户尚未验证」提示

**Files:**
- Modify: `backend/internal/modules/auth/repository.go`
- Modify: `backend/internal/modules/auth/service.go`
- Modify: `backend/internal/modules/auth/register_verification_test.go`（或 `sessions_test.go`）

**Interfaces:**
- Consumes: Task 5 的 `findPendingRegistrationByEmail`；Task 1 的 `ErrEmailNotVerified`。
- Produces: `func (r *repository) findPendingRegistrationByEmail(ctx context.Context, email string) (*ent.PendingRegistration, error)`（不存在返回 `errPendingNotFound`）

- [ ] **Step 1: 写失败测试**

```go
func TestLoginPendingRegistrationCorrectPassword(t *testing.T) {
    db := testDB(t)
    tag := uniqueTag(t)
    mailer := &fakeMailer{configured: true}
    ts, _, settings := newTestAppWithMailer(t, db, mailer)
    enableEmailVerification(t, settings)
    setSiteURL(t, settings, "https://example.com")
    email := "pend+" + tag + "@example.com"
    postJSON(t, testClient(t), ts.URL+"/api/v1/users", RegisterRequest{Email: email, Nickname: "pd" + tag, Password: "pass1234"}).Body.Close()
    resp := postJSON(t, testClient(t), ts.URL+"/api/v1/sessions", LoginRequest{Identifier: email, Password: "pass1234"})
    if resp.StatusCode != http.StatusForbidden { t.Fatalf("status = %d, want 403", resp.StatusCode) }
    if code := decodeError(t, resp); code != "email_not_verified" { t.Fatalf("code = %q, want email_not_verified", code) }
}

func TestLoginPendingRegistrationWrongPassword(t *testing.T) {
    /* 同 setup；密码改成错误值 → 401 invalid_credentials */
}

func TestLoginPendingRegistrationExpired(t *testing.T) {
    /* 同 setup；把 pending 的 expires_at 改成过去 → 401 invalid_credentials */
}

func TestLoginPendingRegistrationByNickname(t *testing.T) {
    /* 同 setup；identifier 用 nickname + 正确密码 → 401 invalid_credentials */
}

func TestLoginPendingRegistrationWrongPasswordCountsFailure(t *testing.T) {
    /* 直接插一条待验证记录（bcrypt 哈希），用 LoginIdentifier limit=1 的策略；
       第一次错误密码 → 401，第二次 → 429（计一次失败） */
}

func TestUpdateProfileEmailKeepsVerificationStatus(t *testing.T) {
    /* 建一个已验证用户并登录，PATCH /users/me 改邮箱；
       重新查库断言 email_verification_status 仍是 verified（I9：改邮箱不重置状态） */
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestLoginPending' -v`
Expected: FAIL（返回 401 而不是 403）。

- [ ] **Step 3: 实现**

`Login` 在 `findUserByIdentifier` 返回 `errUserNotFound` 的分支里：

```go
if strings.Contains(identifier, "@") {
    pending, pendingErr := s.repo.findPendingRegistrationByEmail(ctx, normalizeEmail(identifier))
    if pendingErr == nil && pending.ExpiresAt.After(time.Now()) {
        if bcrypt.CompareHashAndPassword([]byte(pending.PasswordHash), []byte(password)) == nil {
            s.resetFailure(accountKey)
            return User{}, apierror.ErrEmailNotVerified
        }
        s.recordFailure(s.policies.LoginIdentifier, accountKey)
        return User{}, apierror.ErrInvalidCredentials
    }
    if pendingErr != nil && !errors.Is(pendingErr, errPendingNotFound) {
        return User{}, pendingErr
    }
}
_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
s.recordFailure(s.policies.LoginIdentifier, accountKey)
return User{}, apierror.ErrInvalidCredentials
```

过期记录不删（懒清理留给下次注册）。

- [ ] **Step 4: 跑测试确认通过**

Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestLoginPending|TestLogin|TestUpdateProfileEmailKeeps' -v`
Expected: PASS；既有登录测试不改。

- [ ] **Step 5: 核对 diff，不提交**

---

### Task 8: 管理端用户列表带验证状态

**Files:**
- Modify: `backend/internal/modules/access/dto.go`
- Modify: `backend/internal/modules/access/repository.go`
- Modify: `backend/internal/modules/access/service.go`
- Modify: `backend/internal/modules/access/service_test.go`

**Interfaces:**
- Produces: `UserSummary.EmailVerificationStatus string`，JSON `email_verification_status`，值来自 `users.email_verification_status`。

- [ ] **Step 1: 写失败测试**

在 `service_test.go` 加：创建一个用户（状态 `not_required`），`ListUsers` 返回的对应项 `EmailVerificationStatus == "not_required"`；`GetUser` 同样带出。

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/access/ -run 'TestListUsers|TestGetUser' -v`
Expected: FAIL（字段为空）。

- [ ] **Step 3: 实现**

- `userRecord` 加 `EmailVerificationStatus`；`toUserRecord` 复制。
- `UserSummary` 加字段。
- `ListUsers` / `GetUser` 映射。

- [ ] **Step 4: 跑测试确认通过**

Run: `bash scripts/test.sh ./internal/modules/access/ -v`
Expected: PASS。

- [ ] **Step 5: 核对 diff，不提交**

---

### Task 9: 前端注册 202 + 验证页

**Files:**
- Modify: `frontend/hooks/use-signup.ts`
- Modify: `frontend/app/signup/signup-form.tsx`
- Modify: `frontend/lib/signup-notice.ts`
- Modify: `frontend/hooks/use-signup-notice.ts`
- Create: `frontend/app/signup/verify/page.tsx`
- Create: `frontend/app/signup/verify/verify-form.tsx`
- Modify: `frontend/messages/zh-CN.json`
- Modify: `frontend/messages/en.json`

**Interfaces:**
- Consumes: `POST /users` 的 202 `{"status":"verification_pending"}`；`POST /users/email-verifications` 的 201/400/409；现有 `markSignupSuccess()`。
- Produces: 注册成功 sonner 提示；`/signup/verify?token=...` 页面。

- [ ] **Step 1: 改 hook**

`useSignupForm` **不保存待验证邮箱，也不让表单切成功态**。`submit` 里（先跳转、页面挂载后再弹，不在跳转前弹）：

```ts
const payload = await apiRequest<{ status?: string }>("/api/v1/users", { method: "POST", body: {...} });
if (payload?.status === "verification_pending") {
  markSignupPending();
  setValues(EMPTY_VALUES);
  router.push("/signin");
  return undefined;
}
markSignupSuccess();
setValues(EMPTY_VALUES);
router.push("/signin");
```

同时在 `toFailure` 加 `case "mail_unavailable": return { message: t("mailUnavailable") };`。

`lib/signup-notice.ts` 增加 `markSignupPending()` / `consumeSignupPending()`（独立的 sessionStorage 键）；`hooks/use-signup-notice.ts` 在登录页挂载后先 `consumeSignupPending()` 弹 `verificationSent`，否则 `consumeSignupSuccess()` 弹 `success`。201 与验证成功继续用 `markSignupSuccess()`，也是跳转后由登录页弹。

- [ ] **Step 2: 注册表单不显示成功态**

`signup-form.tsx` 保持单步表单：不要加 `verificationEmail`、不要加成功卡片、不要加 `Link` import。202 的提示与跳转完全由 Step 1 的 sonner + `router.push("/signin")` 负责。

- [ ] **Step 3: 新建验证页**

`page.tsx`（记得 `import type { Metadata } from "next";`）：

```tsx
export const instant = false;
export const metadata: Metadata = {
  referrer: "no-referrer",
  robots: { index: false, follow: false },
};

export default async function VerifyPage({ searchParams }: { searchParams: Promise<{ token?: string }> }) {
  const { token } = await searchParams;
  return <VerifyForm token={typeof token === "string" ? token : ""} />;
}
```

`verify-form.tsx`（client）：

- 首屏按钮「验证邮箱」；`token === ""` 时只显示去 `/signup` 的链接，并在挂载时 sonner `verifyInvalid`。
- 成功：`markSignupSuccess()` + `router.push("/signin")`；登录页的既有 notice 弹出「注册成功，请登录」，**验证页不显示成功文案**。
- `email_verification_invalid` → sonner `verifyInvalid`。
- `email_taken` / `nickname_taken` / `already_taken` → sonner `verifyTaken`。
- 其他 → sonner `serviceUnavailable` / `verifyFailed`。
- 用 `AuthFrame` 包住，保持与注册/登录页一致。

- [ ] **Step 4: 加文案**

`messages/zh-CN.json` 与 `en.json` 的 `auth.signup` 下：`verificationSent`（sonner，无占位符）、`verifyTitle`、`verifyAction`、`verifyInvalid`、`verifyTaken`、`verifyGoSignup`、`verifyFailed`、`mailUnavailable`。删掉成功卡片用的 `verificationSentTitle` / `verificationExpires` / `verificationNoResend` / `verifySuccess` / `verifyIncomplete` / `verifyGoSignin`。

- [ ] **Step 5: 静态检查**

Run:

```bash
cd frontend
npx tsc --noEmit
npx eslint app/signup hooks/use-signup.ts
npx prettier --check app/signup hooks/use-signup.ts messages/zh-CN.json messages/en.json
```

Expected: 0 错。

- [ ] **Step 6: 核对 diff，不提交**

---

### Task 10: 前端登录错误映射

**Files:**
- Modify: `frontend/hooks/use-signin.ts`
- Modify: `frontend/messages/zh-CN.json`
- Modify: `frontend/messages/en.json`

- [ ] **Step 1: 加映射**

`toMessage` 的 switch 加：

```ts
case "email_not_verified":
  return t("emailNotVerified");
```

`messages` 的 `auth.signin` 下加 `emailNotVerified`：「账户尚未验证」/ `"Account not verified"`。

- [ ] **Step 2: 静态检查**

Run:

```bash
cd frontend
npx tsc --noEmit
npx eslint hooks/use-signin.ts
npx prettier --check hooks/use-signin.ts messages/zh-CN.json messages/en.json
```

- [ ] **Step 3: 核对 diff，不提交**

---

### Task 11: 「用户注册」设置页开关 + 前置条件

**Files:**
- Modify: `frontend/lib/register-settings.ts`
- Modify: `frontend/hooks/use-register-settings-save.ts`
- Modify: `frontend/app/console/settings/register/register-settings-form.tsx`
- Modify: `frontend/app/console/settings/register/page.tsx`
- Modify: `frontend/lib/admin-settings.ts`
- Modify: `frontend/messages/zh-CN.json`
- Modify: `frontend/messages/en.json`

**Interfaces:**
- Consumes: `GET /admin/mail/status` 的 `{"smtp_configured","site_url_configured"}`。
- Produces:
  - `RegisterGroupValues.emailVerificationEnabled: boolean`
  - `RegisterSettingsRead` 增加 `prerequisites: { smtpConfigured: boolean; siteUrlConfigured: boolean } | null`
  - `useRegisterSettingsSave` 增加 `setEmailVerificationEnabled(checked: boolean)`

- [ ] **Step 1: 改类型与解析**

`lib/register-settings.ts`：

- `RegisterGroupValues` 加 `emailVerificationEnabled`；
- `REGISTER_PATCH_KEYS` 加 `emailVerificationEnabled: "email_verification_enabled"`；
- `parseRegisterGroupValues` 要求 `email_verification_enabled` 是 boolean；
- 新增：

```ts
export type MailStatus = { smtpConfigured: boolean; siteUrlConfigured: boolean };

export function parseMailStatus(payload: unknown): MailStatus | null {
  const raw = payload as { smtp_configured?: unknown; site_url_configured?: unknown } | null;
  if (typeof raw?.smtp_configured !== "boolean" || typeof raw?.site_url_configured !== "boolean") return null;
  return { smtpConfigured: raw.smtp_configured, siteUrlConfigured: raw.site_url_configured };
}
```

- [ ] **Step 2: 改数据加载**

`lib/admin-settings.ts` 的 `getRegisterSettings` 并行增加：

```ts
fetchAdminPayload("/api/v1/admin/mail/status"),
```

`RegisterSettingsRead` 的 ok 分支加 `prerequisites`：register / auth / groups 三条都 ok 后，单独解析 mail status；mail status 请求失败或解析失败 → `prerequisites: null`；register/auth/groups 任一 forbidden → 整页 forbidden（现状）。

- [ ] **Step 3: 改保存 hook**

`useRegisterSettingsSave` 增加：

```ts
function setEmailVerificationEnabled(checked: boolean) {
  setValues((previous) => ({ ...previous, emailVerificationEnabled: checked }));
}
```

并返回它。

- [ ] **Step 4: 改表单**

- 从 props 接 `prerequisites`。
- `const canEnable = prerequisites?.smtpConfigured === true && prerequisites?.siteUrlConfigured === true;`
- 缺项提示两项都列，不要只报一项：

```tsx
const missingHints = prerequisites === null
  ? [t("emailVerificationUnknownHint")]
  : [
      prerequisites.smtpConfigured ? null : t("emailVerificationSmtpHint"),
      prerequisites.siteUrlConfigured ? null : t("emailVerificationSiteUrlHint"),
    ].filter((value): value is string => value !== null);

const description = values.emailVerificationEnabled ? undefined : missingHints.join(" ");
```

- 在「启用邮箱域名控制」行之后加：

```tsx
<ConsoleSettingsRow
  control={
    <Switch
      aria-label={t("emailVerification")}
      checked={values.emailVerificationEnabled}
      onCheckedChange={setEmailVerificationEnabled}
      disabled={isSaving || (!values.emailVerificationEnabled && !canEnable)}
    />
  }
  description={description}
  icon={<MailCheck />}
  label={t("emailVerification")}
/>
```

- 表单顶部：`values.emailVerificationEnabled && !canEnable` 时渲染一条带边框/底色的警示文字（`text-sm`，不用 ring），文案用 `t("emailVerificationWarning")`，不要硬编码中文。
- `page.tsx` 把 `read.prerequisites` 传给表单。

- [ ] **Step 5: 加文案**

`settings` 下加：`emailVerification`（邮箱所有权认证）、`emailVerificationSmtpHint`、`emailVerificationSiteUrlHint`、`emailVerificationUnknownHint`、`emailVerificationWarning`。

- [ ] **Step 6: 静态检查**

Run:

```bash
cd frontend
npx tsc --noEmit
npx eslint app/console/settings/register hooks/use-register-settings-save.ts lib/register-settings.ts lib/admin-settings.ts
npx prettier --check app/console/settings/register hooks/use-register-settings-save.ts lib/register-settings.ts lib/admin-settings.ts messages/zh-CN.json messages/en.json
```

- [ ] **Step 7: 核对 diff，不提交**

---

### Task 12: 前端用户列表状态 + 邮件模板标签

**Files:**
- Modify: `frontend/lib/users.ts`
- Modify: `frontend/app/console/settings/users/users-panel.tsx`
- Modify: `frontend/app/console/settings/mail/mail-settings-form.tsx`
- Modify: `frontend/messages/zh-CN.json`
- Modify: `frontend/messages/en.json`

- [ ] **Step 1: 用户列表**

- `AdminUser` 加 `emailVerificationStatus: "verified" | "not_required"`。
- `parseAdminUser` 读取 `raw.email_verification_status`，只接受两个值，否则返回 null。
- `users-panel.tsx` 在 group 列后加：

```tsx
{
  key: "emailVerification",
  header: t("columnEmailVerification"),
  headerClassName: "px-4",
  cellClassName: "px-4 text-xs text-muted-foreground",
  render: (user) => t(user.emailVerificationStatus === "verified" ? "emailVerificationVerified" : "emailVerificationNotRequired"),
}
```

- `users` 文案加 `columnEmailVerification`、`emailVerificationVerified`、`emailVerificationNotRequired`。

- [ ] **Step 2: 邮件模板行**

- `mail-settings-form.tsx` 的 `MAIL_TEMPLATE_PLACEHOLDERS` 加 `"verifyUrl"`（保留 `"code"`）。
- `settings.mailTemplateSignupCode` 改成「注册验证邮件」/ `"Registration verification email"`。
- `settings.mailTemplatePlaceholderHint` 改成列出 `code`（密码重置）与 `verifyUrl`（注册验证）两套。

- [ ] **Step 3: 静态检查**

Run:

```bash
cd frontend
npx tsc --noEmit
npx eslint app/console/settings/users app/console/settings/mail lib/users.ts
npx prettier --check app/console/settings/users app/console/settings/mail lib/users.ts messages/zh-CN.json messages/en.json
```

- [ ] **Step 4: 核对 diff，不提交**

---

### Task 13: 文档与全量验证

**Files:**
- Modify: `backend/README.md`
- Modify: `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md`
- Modify: `docs/DEVLOG.md`

- [ ] **Step 1: 更新 README**

- 目录表：`auth` / `mail` / `access` / `settings` 行补新能力。
- 接口表：`POST /users` 的 201/202 分叉、`POST /users/email-verifications`、`GET /admin/mail/status`、`POST /sessions` 的 403 `email_not_verified`。
- 设置清单：`register.email_verification_enabled` 从「暂无消费方」改成实际行为；注明仍非公开。
- 邮件章节：`Configured` 的判定口径（设置组与环境变量回退都要求配置齐全且格式合法，不是实测连通）；注册发信带 `RequireSMTP`，日志降级直接报错；模板 `signup_code` 改成链接、预览 `expiresMinutes=30`。
- 已知缺口：后台待验证列表、重发、改邮箱验证仍未做。

- [ ] **Step 2: 更新 API 规范**

在错误码表加三条；在端点表加两个新端点与 202；登记 `GET /admin/mail/status` 的权限点。

- [ ] **Step 3: 更新 DEVLOG**

在 `docs/DEVLOG.md` 顶部（倒序）加一条，按现有格式写：动机、范围、决策、验证、已知缺口、提交状态（写明「未提交，待用户检查」）。

- [ ] **Step 4: 全量后端测试与静态门禁**

Run:

```bash
cd backend
export PATH="/f/software/go/bin:/f/software/gopath/bin:$PATH"
export GOPROXY=https://goproxy.cn,direct
set -a; source .env; set +a
export DATABASE_URL_TEST="${DATABASE_URL_TEST:?missing}"
bash scripts/check.sh
bash scripts/test.sh ./...
```

Expected: `check ok` + 全绿。若有 failure，先确认是不是并行/残留行导致；串行重跑。

- [ ] **Step 5: 全量前端检查**

Run:

```bash
cd frontend
npx tsc --noEmit
npx eslint app components hooks lib
npx prettier --check <改动的文件>
```

Expected: 0 错。`npx prettier --check .` 基线在 `components/ui/select.tsx` 就失败，不作为本轮门禁；eslint 的既有 `components/ui/sooner` 警告也不计入。

- [ ] **Step 6: 变异自检（spec §10.3）**

逐条把代码改坏、跑对应用例、确认变红，再改回。spec §10.3 的十条全做：

1. 去掉 `expires_at` 判断 → `TestVerifyEmailRegistrationRejectsUnknownAndExpired` 红；
2. 去掉删除计数判断 → `TestVerifyEmailRegistrationConcurrent` 红；
3. 跳过 `Configured` → `TestRegisterWithoutSMTPFailsClosed` 红；
4. 跳过发信失败补偿删除 → `TestRegisterSendFailureDeletesPending` 红；
5. 验证建号写成 `not_required` → `TestVerifyEmailRegistration` 的状态断言红；
6. 202 改成 201 → `TestRegisterCreatesPendingInsteadOfUser` 红；
7. 登录待验证分支不比密码 → `TestLoginPendingRegistrationWrongPassword` 红；
8. 去掉 `site.url` 校验 → `TestRegisterWithoutSiteURLFailsClosed` / `TestRegisterInvalidSiteURL` 红；
9. 令牌以明文入库 → `TestRegisterCreatesPendingInsteadOfUser` 的 `token_hash` 断言红；
10. 把 `email_verification_enabled` 标成 Public → `TestBuiltinPublicAllowlist` 红。

另加两条本轮新增的：补偿删除改回按邮箱删 → `TestRegisterCompensationDoesNotDeleteReplacedPending` 红；去掉 `RequireSMTP` → `TestSettingsTransportRejectsRequireSMTPWhenLog` 或 `TestRegisterDoesNotUseLogSender` 红。

- [ ] **Step 7: 汇总，不提交**

报告：改了哪些文件、测试命令与结果、变异自检结果、已知残留。**不要 git add/commit/push。** 运行期 E2E（真实 SMTP、收信、点链接）由用户执行；「清空 SMTP 主机后注册失败」这条只有在环境变量 `SMTP_HOST` 也未配置时才成立。
