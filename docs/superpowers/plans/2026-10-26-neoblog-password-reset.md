# 密码重置实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让已有账号通过邮件里的一次性链接自助设置新密码；后台「登录控制」的「启用密码重置」开关真正生效，关掉后入口、页面、接口一起失效。

**Architecture:** 新增 `password_resets` 表（一账号一条，令牌只存 SHA-256 指纹）与无路由能力模块 `internal/modules/passwordreset`；`auth` controller 暴露两个公开接口 `POST /users/password-resets` 与 `POST /users/password-resets/confirmations`。邮件复用现有 SMTP 与 `password_reset` 模板（改成链接），重置成功用现有 `auth_version` 机制踢掉所有旧会话，2FA 保持原样。

**Tech Stack:** Go 1.27 + gin + ent + Atlas + PostgreSQL；Next.js 16 + React 19 + next-intl + Tailwind/shadcn；后端测试用真实 Postgres（`DATABASE_URL_TEST`）。

**Spec:** [2026-10-26-neoblog-password-reset-design.md](../specs/2026-10-26-neoblog-password-reset-design.md)

## Global Constraints

- **绝不提交。** 每个任务结束只跑测试、核对 diff，改动留在工作区等用户检查。不要 `git add` / `git commit` / `git push`。
- **工作区有并行会话的 captcha 改动。** 不要 `git add -A`、不要回滚、不要清理不属于本计划的文件；每个任务只用 `git diff -- <本任务文件>` 核对。`git status` 里出现 captcha / settings 相关改动是正常的。
- **全仓零人工注释**：Go、TS、TSX 都不写解释性注释（工具生成的 `// indirect` 除外）。计划里的 `/* ... */` 只是说明占位，最终代码不得保留。
- **禁止兜底/降级**：失败按错误上抛；SMTP 未配置、`site.url` 非法、渲染/发送失败一律返回 `mail_unavailable`；不要加「失败改用默认值 / 跳过发信 / 日志发链接」的分支。唯一刻意的静默是「未知邮箱也回 202」，它已在 spec §2 D3 授权。
- **不扩范围**（spec §11）：不接人机验证、不改 `captcha_scopes`、不做重发按钮、不发「密码已修改」通知、不做审计表、不改 2FA 恢复码邮件、不加新设置键/环境变量/依赖。
- **版本号一个字都不提。**
- **后端测试起手式**（串行；不要 `-race`；不要同时开两个测试进程打同一个 `neoblog_test`）：

  ```bash
  cd backend
  export PATH="/f/software/go/bin:/f/software/gopath/bin:$PATH"
  export GOPROXY=https://goproxy.cn,direct
  set -a; source .env; set +a
  export DATABASE_URL_TEST="${DATABASE_URL_TEST:?missing}"
  bash scripts/test.sh ./internal/...
  ```

- **静态门禁**：`cd backend && bash scripts/check.sh`（gofmt / go vet / go build / repository 禁 `apierror` / whitespace）。
- **迁移**（改 ent schema 后）：

  ```bash
  cd backend
  export PATH="/f/software/go/bin:/f/software/gopath/bin:$PATH"
  export GOPROXY=https://goproxy.cn,direct
  set -a; source .env; set +a
  export ATLAS_DEV_URL="${ATLAS_DEV_URL:-postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_atlas_dev?sslmode=disable}"
  bash scripts/db-generate.sh
  atlas migrate diff password_resets --dir "file://migrations" --to "ent://internal/database/ent/schema" --dev-url "$ATLAS_DEV_URL"
  atlas migrate hash --dir "file://migrations"
  atlas migrate validate --dir "file://migrations"
  bash scripts/db-migrate-apply.sh
  ```

  生成后人工审阅 SQL；若 diff 夹带 `password_resets` 之外的漂移，停下报告，不要应用。
- **前端验证**：`cd frontend && npx tsc --noEmit`；改动文件跑 `npx eslint <files>` 与 `npx prettier --check <files>`。`npx prettier --check .` 的基线在 `components/ui/select.tsx` 就失败，不作为本轮门禁。
- **浏览器验收一律用 `localhost`**，不要用 `127.0.0.1`（会让前端 chunk 403、页面不 hydrate、登录表单退化成 GET）。静态门禁由实施者跑；真实收信与浏览器点按由用户执行。
- 实施前先 `git log -1` 现读 HEAD；spec / plan 里的行号会漂，所有引用现读现用。

## Review Focus

1. **未知邮箱不能成为探测 oracle**：不存在的邮箱也回 202、不发信、不写记录，且邮箱/IP 限流照常计数；SMTP 未配置或 `site.url` 非法时，未知邮箱也必须得到 503 而不是 202（Task 5 的 `TestRequestUnknownEmailNoSend`、`TestRequestUnknownEmailStillCountsRateLimit`、`TestRequestMailUnavailableBeforeLookup`；Task 7 的 HTTP 用例）。
2. **发信失败补偿只能删本次记录**：按 `id` + `token_hash` 删；按 `user_id` 删会误删并发重申请写入的新记录（Task 3 的 `TestDeleteResetRequiresTokenHash`、Task 5 的 `TestRequestSendFailureCompensates`）。
3. **重复申请后旧令牌必须失效**：`OnConflict` 覆盖 `token_hash`；认领删除条件必须同时带 `id` 与 `token_hash`；两个并发确认只能成功一个（Task 3 的 `TestCompleteResetStaleHash`、Task 6 的 `TestConfirmConcurrent`）。
4. **密码不合格不能浪费令牌**：弱密码与「与当前密码相同」都在认领之前判定，失败后令牌仍可重试（Task 6 的 `TestConfirmWeakPasswordKeepsToken`、`TestConfirmSamePasswordKeepsToken`）。
5. **重置后的会话与 2FA**：成功响应不写 cookie，旧会话下一次请求 401，TOTP secret / 启用时间 / 恢复码不变；开关关闭时两个接口都 403（Task 6 的 `TestConfirmKeepsTwoFactor`、Task 7 的 `TestPasswordResetConfirmHTTP`、`TestPasswordResetRequestDisabledHTTP`、`TestPasswordResetConfirmDisabledHTTP`）。

---

## File Structure

| 文件 | 动作 | 责任 |
| --- | --- | --- |
| `backend/internal/common/apierror/apierror.go` | 修改 | 两个新错误码与状态映射 |
| `backend/internal/database/ent/schema/password_reset.go` | 新建 | 重置记录表：`user_id` 唯一、`token_hash` 唯一、`expires_at` 索引 |
| `backend/internal/database/ent/schema/user.go` | 修改 | `passwordResets` 边与级联删除 |
| `backend/migrations/<timestamp>_password_resets.sql` | 新建 | 建表、唯一索引、外键级联 |
| `backend/internal/modules/passwordreset/service.go` | 新建 | `New`、`Request`、`Confirm`、密码策略读取 |
| `backend/internal/modules/passwordreset/repository.go` | 新建 | 用户查询、令牌 upsert、读取、补偿删除、原子认领改密 |
| `backend/internal/modules/passwordreset/token.go` | 新建 | 32 字节令牌、base64url、SHA-256、30 分钟常量 |
| `backend/internal/modules/passwordreset/ratelimit.go` | 新建 | 申请 IP / 邮箱、确认 IP 三条窗口 |
| `backend/internal/modules/passwordreset/email.go` | 新建 | `site` 组校验与邮件数据组装 |
| `backend/internal/modules/passwordreset/*_test.go` | 新建 | 令牌、仓库、申请、确认、表形状 |
| `backend/internal/modules/mail/controller.go` | 修改 | 导出 `TemplatePasswordReset`、默认主题、预览 `resetUrl` |
| `backend/internal/modules/mail/templates/password_reset.{html,txt}` | 修改 | 验证码改成重置链接 |
| `backend/internal/modules/mail/controller_test.go` | 修改 | 默认模板与预览断言 |
| `backend/internal/modules/auth/controller.go` | 修改 | 两个公开路由与 handler |
| `backend/internal/modules/auth/dto.go` | 修改 | 两个请求 DTO |
| `backend/internal/modules/auth/module.go` | 修改 | 构造参数增加重置服务 |
| `backend/internal/modules/auth/repository.go` | 修改 | 改密码 / 改邮箱时事务内删令牌 |
| `backend/internal/modules/auth/auth_test.go` + 三个 flow helper | 修改 | 装配参数同步 |
| `backend/internal/modules/auth/password_reset_flow_test.go` | 新建 | HTTP flow 与失效联动测试 |
| `backend/internal/modules/settings/registry.go` | 修改 | `password_reset_enabled` 标公开 |
| `backend/internal/modules/settings/registry_test.go` / `controller_test.go` | 修改 | 公开白名单与公开响应断言 |
| `backend/internal/app/app.go` | 修改 | 构造并注入重置服务 |
| `frontend/lib/public-settings.ts` | 修改 | `isPasswordResetEnabled` |
| `frontend/app/login/{page.tsx,signin-view.tsx,signin-form.tsx}` | 修改 | 「忘记密码？」入口 |
| `frontend/app/reset-password/page.tsx` | 新建 | 申请页（开关关闭 404） |
| `frontend/app/reset-password/reset-request-form.tsx` | 新建 | 邮箱表单与成功态 |
| `frontend/hooks/use-password-reset-request.ts` | 新建 | 申请校验、提交、错误映射 |
| `frontend/app/reset-password/confirm/page.tsx` | 新建 | 确认页（读 token、开关关闭 404） |
| `frontend/app/reset-password/confirm/reset-confirm-form.tsx` | 新建 | 密码 + 确认密码表单 |
| `frontend/hooks/use-password-reset-confirm.ts` | 新建 | 确认校验、提交、错误映射 |
| `frontend/app/console/settings/mail/mail-settings-form.tsx` | 修改 | 模板标签与占位符列表 |
| `frontend/messages/zh-CN.json` / `en.json` | 修改 | `auth.reset.*`、登录入口、邮件模板文案 |
| `backend/README.md` | 修改 | 模块表、接口表、邮件与设置描述、错误码 |
| `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md` | 修改 | 端点表与错误码表 |
| `docs/DEVLOG.md` | 修改 | 实施完成后的倒序条目 |

---

### Task 1: API 错误码

**Files:**
- Modify: `backend/internal/common/apierror/apierror.go`
- Modify: `backend/internal/common/apierror/apierror_test.go`

**Interfaces:**
- Consumes: 现有 `Error` / `statusByCode` 注册表。
- Produces:
  - `ErrPasswordResetDisabled` = `{Type: TypePermission, Code: "password_reset_disabled"}`（403）
  - `ErrPasswordResetInvalid` = `{Type: TypeInvalidRequest, Code: "password_reset_invalid", Param: strPtr("token")}`（400）

- [ ] **Step 1: 写失败测试**

在 `apierror_test.go` 的 `TestRegistryMatchesSpec` 表里加两行：

```go
{"password_reset_disabled", ErrPasswordResetDisabled, TypePermission, "password_reset_disabled", http.StatusForbidden, nil},
{"password_reset_invalid", ErrPasswordResetInvalid, TypeInvalidRequest, "password_reset_invalid", http.StatusBadRequest, strPtr("token")},
```

在 `TestRegistryIntegrity` 的 `all` 切片加两个哨兵，并把 `len(all)` 与 `len(statusByCode)` 的期望值从当前实际值各加 2（现读现改，当前是 40 / 40，改完应为 42 / 42）。

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/common/apierror/ -run 'TestRegistry' -v`
Expected: FAIL（`ErrPasswordResetDisabled` / `ErrPasswordResetInvalid` 未定义）。

- [ ] **Step 3: 加哨兵与状态映射**

在 `apierror.go` 的 `var (...)` 里加两条哨兵；在 `statusByCode` 里加：

```go
"password_reset_disabled": http.StatusForbidden,
"password_reset_invalid":  http.StatusBadRequest,
```

- [ ] **Step 4: 跑测试确认通过**

Run: `bash scripts/test.sh ./internal/common/apierror/ -v`
Expected: PASS。

- [ ] **Step 5: 核对 diff，不提交**

Run: `git diff -- backend/internal/common/apierror`
Expected: 只有两个哨兵、两行状态映射、两处测试改动。

---

### Task 2: 数据层与模块骨架

**Files:**
- Create: `backend/internal/database/ent/schema/password_reset.go`
- Modify: `backend/internal/database/ent/schema/user.go`
- Create: `backend/migrations/<timestamp>_password_resets.sql`
- Create: `backend/internal/modules/passwordreset/service.go`
- Create: `backend/internal/modules/passwordreset/repository.go`
- Create: `backend/internal/modules/passwordreset/helpers_test.go`
- Create: `backend/internal/modules/passwordreset/schema_test.go`

**Interfaces:**
- Consumes: `database.Database`、`ent.Client`、`settings` 窄接口、`ratelimit.Limiter`、`mail.Mailer`。
- Produces:
  - ent 客户端 `db.Client.PasswordReset`，字段 `UserID uuid.UUID` / `TokenHash string` / `ExpiresAt time.Time` / `CreatedAt time.Time`。
  - `passwordreset.New(db *database.Database, settings Settings, limiter *ratelimit.Limiter, mailer mail.Mailer) *Service`（本任务只建立构造与空结构，不实现业务方法）。
  - `passwordreset.Settings` 接口：`GetBool(ctx, group, name string) (bool, error)`、`GetNumber(ctx, group, name string) (int64, error)`、`GetGroup(ctx, group string) (map[string]any, error)`。

- [ ] **Step 1: 写 ent schema**

`password_reset.go`：

```go
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type PasswordReset struct {
	ent.Schema
}

func (PasswordReset) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }),
		field.UUID("user_id", uuid.UUID{}),
		field.String("token_hash").NotEmpty().Unique(),
		field.Time("expires_at"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (PasswordReset) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("passwordResets").Field("user_id").Unique().Required(),
	}
}

func (PasswordReset) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("expires_at"),
	}
}
```

`user.go` 的 `Edges()` 增加：

```go
edge.To("passwordResets", PasswordReset.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
```

（`entsql` 已在 `user.go` 导入。）

- [ ] **Step 2: 生成 ent 代码与迁移**

按 Global Constraints 的迁移命令执行 `db-generate.sh` 与 `atlas migrate diff password_resets ...`。

审阅生成的 SQL，必须包含：

```sql
CREATE TABLE "public"."password_resets" (
  "id" uuid NOT NULL,
  "user_id" uuid NOT NULL,
  "token_hash" character varying NOT NULL,
  "expires_at" timestamptz NOT NULL,
  "created_at" timestamptz NOT NULL,
  PRIMARY KEY ("id")
);
CREATE UNIQUE INDEX ... ON "public"."password_resets" ("user_id");
CREATE UNIQUE INDEX ... ON "public"."password_resets" ("token_hash");
CREATE INDEX ... ON "public"."password_resets" ("expires_at");
ALTER TABLE "public"."password_resets" ADD CONSTRAINT ... FOREIGN KEY ("user_id") REFERENCES "public"."users" ("id") ON DELETE CASCADE;
```

索引/外键名字以 Atlas 生成为准；只要唯一约束、`expires_at` 索引、`ON DELETE CASCADE` 三者都在即可。然后 `atlas migrate hash`、`atlas migrate validate`、`bash scripts/db-migrate-apply.sh`。

- [ ] **Step 3: 建模块骨架**

`service.go`：

```go
package passwordreset

import (
	"context"

	"website/backend/internal/common/ratelimit"
	"website/backend/internal/database"
	"website/backend/internal/modules/mail"
)

type Settings interface {
	GetBool(ctx context.Context, group, name string) (bool, error)
	GetNumber(ctx context.Context, group, name string) (int64, error)
	GetGroup(ctx context.Context, group string) (map[string]any, error)
}

type Service struct {
	repo     *repository
	settings Settings
	limiter  *ratelimit.Limiter
	mailer   mail.Mailer
}

func New(db *database.Database, settings Settings, limiter *ratelimit.Limiter, mailer mail.Mailer) *Service {
	if settings == nil {
		panic("passwordreset: settings is required")
	}
	if limiter == nil {
		panic("passwordreset: limiter is required")
	}
	if mailer == nil {
		panic("passwordreset: mailer is required")
	}
	return &Service{
		repo:     newRepository(db),
		settings: settings,
		limiter:  limiter,
		mailer:   mailer,
	}
}
```

`repository.go` 先只放结构：

```go
package passwordreset

import (
	"errors"

	"website/backend/internal/database"
	"website/backend/internal/database/ent"
)

var (
	errUserNotFound  = errors.New("passwordreset: user not found")
	errResetNotFound = errors.New("passwordreset: reset not found")
)

type repository struct {
	client *ent.Client
}

func newRepository(db *database.Database) *repository {
	return &repository{client: db.Client}
}
```

- [ ] **Step 4: 写测试助手**

`helpers_test.go`（`package passwordreset`）：

```go
func testDB(t *testing.T) *database.Database {
	t.Helper()
	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("SKIP: DATABASE_URL_TEST is not set")
	}
	db, err := database.Open(context.Background(), config.Config{
		DatabaseURL:       url,
		DBMaxOpenConns:    10,
		DBMaxIdleConns:    5,
		DBConnMaxLifetime: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func uniqueTag(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
}

func createTestUserWithPassword(t *testing.T, db *database.Database, tag, password string) *ent.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	created, err := db.Client.User.Create().
		SetEmail("reset+"+tag+"@example.com").
		SetNickname("reset"+tag).
		SetPasswordHash(string(hash)).
		SetEmailVerificationStatus("not_required").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { _ = db.Client.User.DeleteOneID(created.ID).Exec(context.Background()) })
	return created
}

func createTestUser(t *testing.T, db *database.Database, tag string) *ent.User {
	t.Helper()
	return createTestUserWithPassword(t, db, tag, "Initial123")
}
```

- [ ] **Step 5: 写表形状测试**

`schema_test.go`（`package passwordreset`）至少四条：

```go
func TestPasswordResetSchemaRoundTrip(t *testing.T)
func TestPasswordResetRejectsSecondRowPerUser(t *testing.T)
func TestPasswordResetRejectsDuplicateTokenHash(t *testing.T)
func TestPasswordResetCascadesOnUserDelete(t *testing.T)
```

断言写死：

- round trip：`SetTokenHash("h-"+tag)`、`SetExpiresAt(time.Now().Add(30*time.Minute))`、`SetUserID(user.ID)` 保存后读回，三个字段相等。
- 第二行同 `user_id` → `ent.IsConstraintError(err)`。
- 两个不同用户同 `token_hash` → `ent.IsConstraintError(err)`。
- 删除用户后 `db.Client.PasswordReset.Query().Where(pwreset.UserIDEQ(user.ID)).Count(ctx)` 为 0（导入 alias `pwreset "website/backend/internal/database/ent/passwordreset"`）。
- 额外查 `information_schema.columns`：`user_id` 的 `is_nullable = 'NO'`；查 `pg_indexes`：存在 `indexdef LIKE '%expires_at%'` 的索引。

- [ ] **Step 6: 跑测试**

Run: `bash scripts/test.sh ./internal/modules/passwordreset/ -run 'TestPasswordReset' -v`
Expected: PASS（迁移已应用到 test 库）。

- [ ] **Step 7: 静态门禁与 diff**

Run:

```bash
bash scripts/check.sh
git diff --stat -- backend/internal/database/ent backend/migrations backend/internal/modules/passwordreset
```

Expected: `check ok`；diff 只含 schema、生成代码、迁移与本模块骨架。

---

### Task 3: 令牌与仓库

**Files:**
- Create: `backend/internal/modules/passwordreset/token.go`
- Modify: `backend/internal/modules/passwordreset/repository.go`
- Create: `backend/internal/modules/passwordreset/token_test.go`
- Create: `backend/internal/modules/passwordreset/repository_test.go`

**Interfaces:**
- Consumes: Task 2 的 ent 客户端与 `repository` 骨架。
- Produces:

  ```go
  const (
      tokenBytes = 32
      resetTTL   = 30 * time.Minute
  )
  func newToken() (raw string, hash string, err error)
  func hashToken(raw string) string

  func (r *repository) findUserByEmail(ctx context.Context, email string) (*ent.User, error)
  func (r *repository) findUserByID(ctx context.Context, id uuid.UUID) (*ent.User, error)
  func (r *repository) replaceReset(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) (uuid.UUID, error)
  func (r *repository) findReset(ctx context.Context, tokenHash string) (*ent.PasswordReset, error)
  func (r *repository) deleteReset(ctx context.Context, id uuid.UUID, tokenHash string) (int, error)
  func (r *repository) completeReset(ctx context.Context, id uuid.UUID, tokenHash string, userID uuid.UUID, passwordHash string) error
  ```

- [ ] **Step 1: 写令牌与仓库的失败测试**

`token_test.go`：

```go
func TestNewTokenShape(t *testing.T) {
	raw, hash, err := newToken()
	if err != nil { t.Fatalf("newToken: %v", err) }
	if len(raw) != 43 { t.Fatalf("raw length = %d, want 43", len(raw)) }
	if len(hash) != 64 { t.Fatalf("hash length = %d, want 64", len(hash)) }
	if hash != hashToken(raw) { t.Fatal("returned hash does not match hashToken") }
	raw2, _, _ := newToken()
	if raw == raw2 { t.Fatal("two tokens are identical") }
}

func TestHashTokenIsSHA256Hex(t *testing.T) {
	sum := sha256.Sum256([]byte("abc"))
	if got := hashToken("abc"); got != hex.EncodeToString(sum[:]) {
		t.Fatalf("hashToken(abc) = %q", got)
	}
}
```

`repository_test.go`：

```go
func TestReplaceResetUpsertsSingleRow(t *testing.T)
func TestReplaceResetClearsExpiredRows(t *testing.T)
func TestFindResetMissing(t *testing.T)
func TestDeleteResetRequiresTokenHash(t *testing.T)
func TestCompleteResetConsumesOnce(t *testing.T)
func TestCompleteResetStaleHash(t *testing.T)
func TestCompleteResetBumpsAuthVersion(t *testing.T)
```

断言写死：

- upsert：同一用户连续两次 `replaceReset`，`Count` 为 1；第二次的 `token_hash` 可查到，第一次的 `findReset` 返回 `errResetNotFound`；`expires_at` 取第二次的值。
- 过期清理：为用户 A、B 各插一条 `expires_at = now-1h`，再为 C `replaceReset`；A/B 的行数为 0。
- 缺失：`findReset("nope")` 返回 `errResetNotFound`。
- 删除条件：同一行先 `replaceReset(hashA)`，再 `replaceReset(hashB)`；用 `deleteReset(id, hashA)` 返回 0，行还在；用 `deleteReset(id, hashB)` 返回 1。
- 单次消费：`completeReset` 第一次 nil，第二次 `errResetNotFound`；用户密码哈希变成传入值，`auth_version` 从 0 变 1。
- 陈旧指纹：读取行后把 `token_hash` 改成另一个值，再用旧指纹 `completeReset` → `errResetNotFound`，密码不变。

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/passwordreset/ -run 'TestNewToken|TestHashToken|TestReplaceReset|TestFindReset|TestDeleteReset|TestCompleteReset' -v`
Expected: FAIL（方法未定义）。

- [ ] **Step 3: 实现 token.go**

```go
package passwordreset

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"
)

const (
	tokenBytes = 32
	resetTTL   = 30 * time.Minute
)

func newToken() (string, string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	return raw, hashToken(raw), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 4: 实现 repository.go**

导入 `entgo.io/ent/dialect/sql`、`github.com/google/uuid`、`pwreset "website/backend/internal/database/ent/passwordreset"`。

`replaceReset` 事务步骤：

1. `tx.PasswordReset.Delete().Where(pwreset.ExpiresAtLT(time.Now())).Exec(ctx)`；
2. `tx.PasswordReset.Create().SetUserID(userID).SetTokenHash(tokenHash).SetExpiresAt(expiresAt).OnConflict(sql.ConflictColumns("user_id")).Update(func(u *ent.PasswordResetUpsert) { u.SetTokenHash(tokenHash); u.SetExpiresAt(expiresAt) }).Exec(ctx)`；
3. `tx.PasswordReset.Query().Where(pwreset.UserIDEQ(userID)).Only(ctx)` 取 ID；
4. `tx.Commit()`，返回 ID。

`completeReset` 事务步骤：

1. `n, err := tx.PasswordReset.Delete().Where(pwreset.IDEQ(id), pwreset.TokenHashEQ(tokenHash)).Exec(ctx)`；
2. `n != 1` → `errResetNotFound`；
3. `tx.User.UpdateOneID(userID).SetPasswordHash(passwordHash).AddAuthVersion(1).Save(ctx)`；`ent.IsNotFound` → `errUserNotFound`；
4. `tx.Commit()`。

`findReset` / `findUserByEmail` / `findUserByID` 用 `Only`，`ent.IsNotFound` 转成对应哨兵。

- [ ] **Step 5: 跑测试确认通过**

Run: `bash scripts/test.sh ./internal/modules/passwordreset/ -run 'TestNewToken|TestHashToken|TestReplaceReset|TestFindReset|TestDeleteReset|TestCompleteReset' -v`
Expected: PASS。

- [ ] **Step 6: 静态门禁**

Run: `bash scripts/check.sh`
Expected: `check ok`（尤其 repository 不得 import `apierror`）。

---

### Task 4: 邮件模板、默认主题与预览

**Files:**
- Modify: `backend/internal/modules/mail/controller.go`
- Modify: `backend/internal/modules/mail/templates/password_reset.html`
- Modify: `backend/internal/modules/mail/templates/password_reset.txt`
- Modify: `backend/internal/modules/mail/controller_test.go`

**Interfaces:**
- Consumes: 现有 `Compose` / `previewData` / `previewSubjects`。
- Produces:
  - `mail.TemplatePasswordReset = "password_reset"`（导出常量）。
  - 默认主题 `"密码重置"`。
  - 预览数据新增 `resetUrl = "https://example.com/reset-password/confirm?token=preview-token"`。

- [ ] **Step 1: 写失败测试**

在 `controller_test.go` 新增：

```go
func TestComposePasswordResetRendersLink(t *testing.T) {
	body, err := Render(TemplatePasswordReset, map[string]string{
		"siteTitle": "My Blog", "siteUrl": "https://blog.example.com",
		"username": "alice", "expiresMinutes": "30", "sentAt": "2026-10-26 12:00:00",
		"resetUrl": "https://blog.example.com/reset-password/confirm?token=abc", "code": "",
	})
	if err != nil { t.Fatalf("Render: %v", err) }
	if !strings.Contains(body.Text, "https://blog.example.com/reset-password/confirm?token=abc") {
		t.Fatalf("text = %q, want reset link", body.Text)
	}
	if strings.Contains(body.Text, "验证码") {
		t.Fatalf("text = %q, still mentions a code", body.Text)
	}
}

func TestPasswordResetCustomBodyKeepsEmptyCode(t *testing.T) {
	settings := &controllerSettingsStub{groups: map[string]map[string]any{
		"mail": {"password_reset_body": "<p>{{.code}}</p>"},
	}}
	_, body, err := Compose(context.Background(), settings, TemplatePasswordReset, map[string]string{"code": ""})
	if err != nil { t.Fatalf("Compose: %v", err) }
	if strings.Contains(body.HTML, "<no value>") { t.Fatalf("html = %q", body.HTML) }
}
```

把 `TestCreateMessageUsesBuiltinTemplateDefaults` 改成断言：主题 `"密码重置"`；正文含 `https://example.com/reset-password/confirm?token=preview-token`；不含 `"123456"`。

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/mail/ -run 'TestComposePasswordReset|TestPasswordResetCustomBody|TestCreateMessageUsesBuiltinTemplateDefaults' -v`
Expected: FAIL（模板/常量/预览未改）。

- [ ] **Step 3: 改常量、主题与预览**

`controller.go`：

```go
const (
	selfCheckSubject      = "邮件配置自检"
	TemplateSignupCode    = "signup_code"
	TemplatePasswordReset = "password_reset"
	exampleTemplate       = "example"
	signupCodeTemplate    = TemplateSignupCode
	passwordResetTemplate = TemplatePasswordReset
)

var previewSubjects = map[string]string{
	signupCodeTemplate:    "注册验证邮件",
	passwordResetTemplate: "密码重置",
}
```

`previewData` 返回值增加：

```go
"resetUrl": "https://example.com/reset-password/confirm?token=preview-token",
```

保留 `code: "123456"`，给历史自定义正文用。

- [ ] **Step 4: 改模板**

`password_reset.txt`：

```text
{{.siteTitle}} 密码重置

你好 {{.username}}，

请在 {{.expiresMinutes}} 分钟内打开下面的链接设置新密码：

{{.resetUrl}}

如果不是你本人操作，请忽略这封邮件。

{{.siteTitle}} · {{.siteUrl}} · {{.sentAt}}
```

`password_reset.html` 用同样的文案与 `{{.resetUrl}}` 链接，保留现有卡片样式；把「验证码」段落换成链接段落：

```html
<p style="margin:0 0 12px;line-height:1.7">请在 <strong>{{.expiresMinutes}} 分钟内</strong>打开下面的链接设置新密码：</p>
<p style="margin:0 0 12px"><a href="{{.resetUrl}}" style="color:#2563eb">重置密码</a></p>
<p style="margin:0 0 12px;color:#666666;font-size:13px">如果不是你本人操作，请忽略这封邮件。</p>
```

- [ ] **Step 5: 跑测试确认通过**

Run: `bash scripts/test.sh ./internal/modules/mail/ -v`
Expected: PASS（既有 mail 测试全绿）。

- [ ] **Step 6: 核对 diff**

Run: `git diff -- backend/internal/modules/mail`
Expected: 常量、主题、预览、两份模板、三处测试。

---

### Task 5: 申请流程 `Request`

**Files:**
- Create: `backend/internal/modules/passwordreset/ratelimit.go`
- Create: `backend/internal/modules/passwordreset/email.go`
- Modify: `backend/internal/modules/passwordreset/service.go`
- Create: `backend/internal/modules/passwordreset/service_request_test.go`
- Modify: `backend/internal/modules/passwordreset/helpers_test.go`

**Interfaces:**
- Consumes: Task 3 的 `newToken` / `hashToken` / `replaceReset` / `deleteReset` / `findUserByEmail`；Task 4 的 `mail.TemplatePasswordReset`。
- Produces:
  - `func (s *Service) Request(ctx context.Context, email, ip string) error`
  - `func (s *Service) allowRequestIP(ip string) error`
  - `func (s *Service) allowRequestEmail(email string) error`
  - `func (s *Service) siteInfo(ctx context.Context) (title, baseURL string, err error)`
  - `func resetEmailData(title, baseURL string, user *ent.User, rawToken string) map[string]string`

- [ ] **Step 1: 扩展测试助手**

`helpers_test.go` 增加：

```go
type fakeMailer struct {
	mu            sync.Mutex
	configured    bool
	configuredErr error
	sendSyncErr   error
	messages      []mail.Message
}

func (f *fakeMailer) Configured(context.Context) (bool, error) { return f.configured, f.configuredErr }
func (f *fakeMailer) Send(context.Context, mail.Message) error { return nil }
func (f *fakeMailer) SendSync(_ context.Context, msg mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendSyncErr != nil {
		return f.sendSyncErr
	}
	f.messages = append(f.messages, msg)
	return nil
}
func (f *fakeMailer) sent() []mail.Message { ... }
func (f *fakeMailer) Close(context.Context) error { return nil }

func newResetService(t *testing.T, db *database.Database, mailer *fakeMailer) (*Service, *settings.Service) {
	t.Helper()
	settingsModule, err := settings.New(db)
	if err != nil { t.Fatalf("settings.New: %v", err) }
	return New(db, settingsModule.Service, ratelimit.New(), mailer), settingsModule.Service
}

func enablePasswordReset(t *testing.T, svc *settings.Service) {
	t.Helper()
	ctx := context.Background()
	if err := svc.SetBool(ctx, "auth", "password_reset_enabled", true); err != nil {
		t.Fatalf("enable password reset: %v", err)
	}
	t.Cleanup(func() { _ = svc.ResetItem(ctx, "auth", "password_reset_enabled") })
}

func setSite(t *testing.T, svc *settings.Service, title, url string) {
	t.Helper()
	ctx := context.Background()
	if err := svc.SetString(ctx, "site", "title", title); err != nil {
		t.Fatalf("set site.title: %v", err)
	}
	if err := svc.SetString(ctx, "site", "url", url); err != nil {
		t.Fatalf("set site.url: %v", err)
	}
	t.Cleanup(func() {
		_ = svc.ResetItem(ctx, "site", "title")
		_ = svc.ResetItem(ctx, "site", "url")
	})
}

func extractResetToken(t *testing.T, body string) string {
	t.Helper()
	match := regexp.MustCompile(`token=([A-Za-z0-9_-]+)`).FindStringSubmatch(body)
	if match == nil {
		t.Fatalf("no reset token in %q", body)
	}
	return match[1]
}
```

- [ ] **Step 2: 写申请流程的失败测试**

`service_request_test.go`（每条单独用例）：

```go
func TestRequestSendsLink(t *testing.T)
func TestRequestUnknownEmailNoSend(t *testing.T)
func TestRequestUnknownEmailStillCountsRateLimit(t *testing.T)
func TestRequestDisabled(t *testing.T)
func TestRequestInvalidEmail(t *testing.T)
func TestRequestMailUnavailable(t *testing.T)
func TestRequestMailUnavailableBeforeLookup(t *testing.T)
func TestRequestSiteURLInvalid(t *testing.T)
func TestRequestSendFailureCompensates(t *testing.T)
func TestRequestComposeFailureCompensates(t *testing.T)
func TestRequestReplacesToken(t *testing.T)
func TestRequestEmailRateLimit(t *testing.T)
func TestRequestIPRateLimit(t *testing.T)
func TestRequestUsesStoredEmail(t *testing.T)
func TestRequestClearsExpiredRows(t *testing.T)
```

关键断言：

- `TestRequestSendsLink`：建用户、`enablePasswordReset`、`setSite("My Blog","https://blog.example.com")`；`Request(ctx, user.Email, "203.0.113.10")` 为 nil；`mailer.sent()` 恰好 1 条，`To == user.Email`，`RequireSMTP == true`，正文含 `https://blog.example.com/reset-password/confirm?token=`；用户行数 1；`extractResetToken` 的 `hashToken` 与库里 `token_hash` 相等；`expires_at` 在 `now+29m` 与 `now+31m` 之间。
- `TestRequestUnknownEmailNoSend`：`Request(ctx, "nobody@example.com", ip)` 为 nil；零发信；`PasswordReset` 行数 0。
- `TestRequestUnknownEmailStillCountsRateLimit`：同一 typos 邮箱连续 4 次，每次换一个 IP（`10.0.0.1` 到 `10.0.0.4`）避开 IP 桶；第 4 次必须返回非 nil（邮箱桶 3 次/小时），证明不存在也计数。
- `TestRequestDisabled`：不 enable；`errors.Is(err, apierror.ErrPasswordResetDisabled)` 为真（该哨兵是指针，直接用 `err == apierror.ErrPasswordResetDisabled` 断言），零发信。
- `TestRequestInvalidEmail`：空串与 `"not-an-email"` 各返回非 nil，零发信。
- `TestRequestMailUnavailable`：`fakeMailer{configured:false}`；`err == apierror.ErrMailUnavailable`；零记录。
- `TestRequestMailUnavailableBeforeLookup`：`configured:false` + 未知邮箱；必须是 `ErrMailUnavailable`，不能是 nil。
- `TestRequestSiteURLInvalid`：表驱动 `""` / `"example.com"` / `"https://example.com/?a=1"` / `"https://example.com/#x"`；全是 `ErrMailUnavailable`，零记录。
- `TestRequestSendFailureCompensates`：`sendSyncErr` 设置后请求 → `ErrMailUnavailable`，该用户行数 0；旧记录（如果存在）也被本次 upsert 覆盖后删除，用户可重新申请。补偿只按 `id` + `token_hash` 命中本次记录，这一点的仓库级断言在 Task 3 的 `TestDeleteResetRequiresTokenHash`。
- `TestRequestComposeFailureCompensates`：`settings.SetString(ctx,"mail","password_reset_body","{{.broken")`（非法模板）→ `ErrMailUnavailable`，行数 0，之后 `ResetItem`。
- `TestRequestReplacesToken`：连续两次请求；行数 1；两次邮件 token 不同；第二次的 hash 在库，第一次的 `findReset` 返回 `errResetNotFound`。
- `TestRequestEmailRateLimit`：同一邮箱 3 次 nil，第 4 次非 nil。
- `TestRequestIPRateLimit`：不同邮箱、同一 IP 5 次 nil，第 6 次非 nil。
- `TestRequestUsesStoredEmail`：用户邮箱小写；请求传大写；`To` 等于库里小写邮箱。
- `TestRequestClearsExpiredRows`：为两个用户插入过期行；为第三个用户请求后，过期行数为 0。

- [ ] **Step 3: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/passwordreset/ -run 'TestRequest' -v`
Expected: FAIL（`Request` 未定义）。

- [ ] **Step 4: 实现 ratelimit.go**

```go
package passwordreset

import (
	"time"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/ratelimit"
)

const (
	requestIPLimit    = 5
	requestIPWindow   = time.Hour
	requestEmailLimit = 3
	requestEmailWindow = time.Hour
	confirmIPLimit    = 30
	confirmIPWindow   = 15 * time.Minute
)

func (s *Service) allowRequestIP(ip string) error {
	allowed, retryAfter := ratelimit.AllowWindow(s.limiter, ratelimit.WindowPolicy{
		Limit: requestIPLimit, Window: requestIPWindow,
	}, ratelimit.IPKey("passwordreset", ip))
	if !allowed {
		return apierror.RateLimited(retryAfter)
	}
	return nil
}

func (s *Service) allowRequestEmail(email string) error {
	allowed, retryAfter := ratelimit.AllowWindow(s.limiter, ratelimit.WindowPolicy{
		Limit: requestEmailLimit, Window: requestEmailWindow,
	}, ratelimit.HashedKey("passwordreset:email:", email))
	if !allowed {
		return apierror.RateLimited(retryAfter)
	}
	return nil
}

func (s *Service) allowConfirmIP(ip string) error {
	allowed, retryAfter := ratelimit.AllowWindow(s.limiter, ratelimit.WindowPolicy{
		Limit: confirmIPLimit, Window: confirmIPWindow,
	}, ratelimit.IPKey("passwordreset-confirm", ip))
	if !allowed {
		return apierror.RateLimited(retryAfter)
	}
	return nil
}
```

- [ ] **Step 5: 实现 email.go**

```go
package passwordreset

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"website/backend/internal/database/ent"
	"website/backend/internal/modules/mail"
)

func (s *Service) siteInfo(ctx context.Context) (string, string, error) {
	site, err := s.settings.GetGroup(ctx, "site")
	if err != nil {
		return "", "", err
	}
	title, ok := site["title"].(string)
	if !ok {
		return "", "", errors.New("passwordreset: site.title is not a string")
	}
	rawURL, ok := site["url"].(string)
	if !ok {
		return "", "", errors.New("passwordreset: site.url is not a string")
	}
	base, err := mail.SiteURL(rawURL)
	if err != nil {
		return "", "", err
	}
	return title, base, nil
}

func resetEmailData(title, baseURL string, user *ent.User, rawToken string) map[string]string {
	return map[string]string{
		"siteTitle":      title,
		"siteUrl":        baseURL,
		"username":       user.Nickname,
		"expiresMinutes": fmt.Sprintf("%d", int(resetTTL/time.Minute)),
		"sentAt":         time.Now().Format("2006-01-02 15:04:05"),
		"resetUrl":       baseURL + "/reset-password/confirm?token=" + url.QueryEscape(rawToken),
		"code":           "",
	}
}
```

- [ ] **Step 6: 实现 `Request`**

在 `service.go` 增加导入 `errors`、`log/slog`、`time`、`github.com/google/uuid`、`website/backend/internal/common/apierror`、`website/backend/internal/common/userinput`、`website/backend/internal/modules/mail`。

```go
func (s *Service) Request(ctx context.Context, email, ip string) error {
	if err := s.allowRequestIP(ip); err != nil {
		return err
	}
	enabled, err := s.settings.GetBool(ctx, "auth", "password_reset_enabled")
	if err != nil {
		return err
	}
	if !enabled {
		return apierror.ErrPasswordResetDisabled
	}
	email = userinput.NormalizeEmail(email)
	if !userinput.ValidEmail(email) {
		return apierror.WithParam(apierror.ErrInvalidRequest, "email")
	}
	if err := s.allowRequestEmail(email); err != nil {
		return err
	}
	configured, err := s.mailer.Configured(ctx)
	if err != nil || !configured {
		return apierror.ErrMailUnavailable
	}
	title, baseURL, err := s.siteInfo(ctx)
	if err != nil {
		return apierror.ErrMailUnavailable
	}
	user, err := s.repo.findUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			return nil
		}
		return err
	}
	rawToken, tokenHash, err := newToken()
	if err != nil {
		return err
	}
	resetID, err := s.repo.replaceReset(ctx, user.ID, tokenHash, time.Now().Add(resetTTL))
	if err != nil {
		return err
	}
	subject, body, err := mail.Compose(ctx, s.settings, mail.TemplatePasswordReset,
		resetEmailData(title, baseURL, user, rawToken))
	if err != nil {
		s.compensate(ctx, resetID, tokenHash)
		return apierror.ErrMailUnavailable
	}
	if err := s.mailer.SendSync(ctx, mail.Message{
		To: user.Email, Subject: subject, Body: body, RequireSMTP: true,
	}); err != nil {
		s.compensate(ctx, resetID, tokenHash)
		return apierror.ErrMailUnavailable
	}
	return nil
}

func (s *Service) compensate(ctx context.Context, id uuid.UUID, tokenHash string) {
	if _, err := s.repo.deleteReset(ctx, id, tokenHash); err != nil {
		slog.Error("passwordreset: delete reset", "err", err)
	}
}
```

- [ ] **Step 7: 跑测试确认通过**

Run: `bash scripts/test.sh ./internal/modules/passwordreset/ -run 'TestRequest' -v`
Expected: PASS。

- [ ] **Step 8: 静态门禁**

Run: `bash scripts/check.sh`
Expected: `check ok`。

---

### Task 6: 确认流程 `Confirm`

**Files:**
- Modify: `backend/internal/modules/passwordreset/service.go`
- Create: `backend/internal/modules/passwordreset/service_confirm_test.go`

**Interfaces:**
- Consumes: Task 3 的 `findReset` / `findUserByID` / `completeReset` / `deleteReset`；Task 5 的 `Request`（测试里拿令牌）。
- Produces:
  - `func (s *Service) Confirm(ctx context.Context, token, newPassword, ip string) error`
  - `func (s *Service) passwordMinLength(ctx context.Context) (int, error)`

- [ ] **Step 1: 写确认流程的失败测试**

`service_confirm_test.go`：

```go
func TestConfirmChangesPasswordAndVersion(t *testing.T)
func TestConfirmSingleUse(t *testing.T)
func TestConfirmExpiredDeletesRow(t *testing.T)
func TestConfirmUnknownToken(t *testing.T)
func TestConfirmWeakPasswordKeepsToken(t *testing.T)
func TestConfirmSamePasswordKeepsToken(t *testing.T)
func TestConfirmDisabled(t *testing.T)
func TestConfirmRateLimit(t *testing.T)
func TestConfirmConcurrent(t *testing.T)
func TestConfirmKeepsTwoFactor(t *testing.T)
func TestConfirmConfiguredMinLength(t *testing.T)
```

关键断言：

- 成功：先 `Request` 拿令牌（`extractResetToken`），记录用户 `AuthVersion`；`Confirm(ctx, token, "NewPass123", ip)` 为 nil；重新查用户，`bcrypt.CompareHashAndPassword(newHash, []byte("NewPass123")) == nil`；`AuthVersion == before+1`；该用户 `PasswordReset` 行数 0。
- 单次：第二次 `Confirm` 返回 `apierror.ErrPasswordResetInvalid`。
- 过期：直接插一行 `SetExpiresAt(now.Add(-time.Minute))`、`SetTokenHash(hashToken("expired"))`；`Confirm(ctx,"expired",...)` 返回 `ErrPasswordResetInvalid`，行数 0。
- 未知令牌：`ErrPasswordResetInvalid`。
- 弱密码：`Confirm(ctx, token, "abc")` 返回非 nil；行仍存在；再用 `"NewPass123"` 成功。
- 同旧密码：用户初始密码 `"Initial123"`；`Confirm(ctx, token, "Initial123", ip)` 返回 `apierror.ErrPasswordUnchanged`；行仍存在。
- 开关关闭：先 Request，再 `SetBool(...,false)`；Confirm 返回 `ErrPasswordResetDisabled`，行仍在。
- 限流：用未知令牌连续 30 次（每次 `ErrPasswordResetInvalid`），第 31 次返回非 nil（限流）；每次用不同 IP 之外的同一 IP。
- 并发：两个 goroutine 同时 `Confirm` 同一令牌，密码分别 `"Winner123"` / `"Loser123"`；收集两个 error，恰好一个 nil、一个 `ErrPasswordResetInvalid`；最终哈希只匹配其中一个密码。
- 2FA 不变：给用户 `SetTotpSecret("secret").SetTotpEnabledAt(now).SetTotpLastCounter(7).SetTotpSetupAt(now)`；Request + Confirm 后这些字段逐项相等。
- 配置最小长度：`SetBool("auth","password_requirements_enabled",true)`、`SetNumber("auth","password_min_length",12)`；10 位字母+数字被拒，令牌仍在；12 位成功；测试后 `ResetItem` 两键。

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/passwordreset/ -run 'TestConfirm' -v`
Expected: FAIL（`Confirm` 未定义）。

- [ ] **Step 3: 实现 `passwordMinLength` 与 `Confirm`**

`passwordMinLength`（照 auth / registration 的第三份自包含副本）：

```go
func (s *Service) passwordMinLength(ctx context.Context) (int, error) {
	requirementsEnabled, err := s.settings.GetBool(ctx, "auth", "password_requirements_enabled")
	if err != nil {
		return 0, err
	}
	configured := int64(0)
	if requirementsEnabled {
		configured, err = s.settings.GetNumber(ctx, "auth", "password_min_length")
		if err != nil {
			return 0, err
		}
	}
	return userinput.EffectivePasswordMinLength(requirementsEnabled, configured), nil
}
```

`Confirm`：

```go
func (s *Service) Confirm(ctx context.Context, token, newPassword, ip string) error {
	if err := s.allowConfirmIP(ip); err != nil {
		return err
	}
	enabled, err := s.settings.GetBool(ctx, "auth", "password_reset_enabled")
	if err != nil {
		return err
	}
	if !enabled {
		return apierror.ErrPasswordResetDisabled
	}
	minLength, err := s.passwordMinLength(ctx)
	if err != nil {
		return err
	}
	if !userinput.ValidPassword(newPassword, minLength) {
		return apierror.WithParam(apierror.ErrWeakPassword, "new_password")
	}
	tokenHash := hashToken(token)
	reset, err := s.repo.findReset(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, errResetNotFound) {
			return apierror.ErrPasswordResetInvalid
		}
		return err
	}
	if !reset.ExpiresAt.After(time.Now()) {
		if _, err := s.repo.deleteReset(ctx, reset.ID, tokenHash); err != nil {
			return err
		}
		return apierror.ErrPasswordResetInvalid
	}
	user, err := s.repo.findUserByID(ctx, reset.UserID)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			return apierror.ErrPasswordResetInvalid
		}
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(newPassword)) == nil {
		return apierror.ErrPasswordUnchanged
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.repo.completeReset(ctx, reset.ID, tokenHash, user.ID, string(newHash)); err != nil {
		if errors.Is(err, errResetNotFound) || errors.Is(err, errUserNotFound) {
			return apierror.ErrPasswordResetInvalid
		}
		return err
	}
	return nil
}
```

在 `service.go` 增加 `golang.org/x/crypto/bcrypt` 导入。

- [ ] **Step 4: 跑测试确认通过**

Run: `bash scripts/test.sh ./internal/modules/passwordreset/ -run 'TestConfirm' -v`
Expected: PASS。

- [ ] **Step 5: 全模块测试与静态门禁**

Run:

```bash
bash scripts/test.sh ./internal/modules/passwordreset/ -v
bash scripts/check.sh
```

Expected: 全绿 + `check ok`。

---

### Task 7: HTTP 接口与装配

**Files:**
- Modify: `backend/internal/modules/auth/dto.go`
- Modify: `backend/internal/modules/auth/controller.go`
- Modify: `backend/internal/modules/auth/module.go`
- Modify: `backend/internal/modules/auth/auth_test.go`
- Modify: `backend/internal/modules/captcha/flow_helpers_test.go`
- Modify: `backend/internal/modules/registration/flow_helpers_test.go`
- Modify: `backend/internal/modules/session/flow_helpers_test.go`
- Modify: `backend/internal/app/app.go`
- Create: `backend/internal/modules/auth/password_reset_flow_test.go`

**Interfaces:**
- Consumes: `passwordreset.Service.Request` / `Confirm`；`auth` 现有 `requireJSONRequest` / `requestSessionMeta` / `apiresponse.Empty`。
- Produces:
  - `type PasswordResetRequest struct { Email string \`json:"email"\` }`
  - `type PasswordResetConfirmationRequest struct { Token string \`json:"token"\`; NewPassword string \`json:"new_password"\` }`
  - `newController(service, loginService, registrationService, twoFactorService, passwordResetService)`（5 个参数）
  - `auth.New(db, sessions, settings, login, registration, twofactor, passwordReset)`（7 个参数）

- [ ] **Step 1: 写 HTTP 失败测试**

`password_reset_flow_test.go`（`package auth`）加辅助：

```go
func enableReset(t *testing.T, svc *settings.Service) {
	t.Helper()
	ctx := context.Background()
	if err := svc.SetBool(ctx, "auth", "password_reset_enabled", true); err != nil {
		t.Fatalf("enable reset: %v", err)
	}
	t.Cleanup(func() { _ = svc.ResetItem(ctx, "auth", "password_reset_enabled") })
}

func setResetSite(t *testing.T, svc *settings.Service) {
	t.Helper()
	ctx := context.Background()
	if err := svc.SetString(ctx, "site", "title", "My Blog"); err != nil {
		t.Fatalf("set title: %v", err)
	}
	if err := svc.SetString(ctx, "site", "url", "https://example.com"); err != nil {
		t.Fatalf("set url: %v", err)
	}
	t.Cleanup(func() {
		_ = svc.ResetItem(ctx, "site", "title")
		_ = svc.ResetItem(ctx, "site", "url")
	})
}

func decodeErrorFields(t *testing.T, resp *http.Response) (string, string) {
	t.Helper()
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	errObj, _ := body["error"].(map[string]any)
	code, _ := errObj["code"].(string)
	param, _ := errObj["param"].(string)
	return code, param
}
```

测试清单：

```go
func TestPasswordResetRequestHTTPAccepted(t *testing.T)
func TestPasswordResetRequestDisabledHTTP(t *testing.T)
func TestPasswordResetConfirmDisabledHTTP(t *testing.T)
func TestPasswordResetRequestInvalidEmailHTTP(t *testing.T)
func TestPasswordResetRequestMailUnavailableHTTP(t *testing.T)
func TestPasswordResetConfirmHTTP(t *testing.T)
func TestPasswordResetConfirmInvalidTokenHTTP(t *testing.T)
func TestPasswordResetConfirmWeakPasswordHTTP(t *testing.T)
func TestPasswordResetConfirmSamePasswordHTTP(t *testing.T)
func TestPasswordResetConfirmMissingFieldsHTTP(t *testing.T)
func TestPasswordResetConfirmRateLimitHTTP(t *testing.T)
func TestPasswordResetRequestRateLimitHTTP(t *testing.T)
```

关键断言：

- `TestPasswordResetRequestHTTPAccepted`：`newTestAppWithMailer` + `enableReset` + `setResetSite`；对已有用户与未知邮箱各 POST 一次，均 202 且 body 为 `{}`；已有用户恰好 1 封，未知 0 封。
- `TestPasswordResetRequestDisabledHTTP`：不开开关；403，code `password_reset_disabled`，param 空。
- `TestPasswordResetConfirmDisabledHTTP`：先开开关拿到 token，再关开关；确认接口 403 `password_reset_disabled`，令牌行仍在（开关不物理删除）。
- `TestPasswordResetRequestInvalidEmailHTTP`：400，code `invalid_request`，param `email`。
- `TestPasswordResetRequestMailUnavailableHTTP`：`fakeMailer{configured:false}`；503，code `mail_unavailable`；未知邮箱同样 503（证明前置检查在查询之前）。
- `TestPasswordResetConfirmHTTP`：先在另一个 client 上 `loginAs` 拿旧会话；`Request` 拿 token；POST confirmations 200、body `{}`、响应无 `Set-Cookie`；旧会话 `GET /users/me` → 401；旧密码登录 401 `invalid_credentials`；新密码登录 200。
- `TestPasswordResetConfirmInvalidTokenHTTP`：400，code `password_reset_invalid`，param `token`。
- `TestPasswordResetConfirmWeakPasswordHTTP`：400，code `weak_password`，param `new_password`；随后用合格密码成功。
- `TestPasswordResetConfirmSamePasswordHTTP`：400，code `password_unchanged`，param `new_password`。
- `TestPasswordResetConfirmMissingFieldsHTTP`：空 token → param `token`；空 new_password → param `new_password`。
- `TestPasswordResetRequestRateLimitHTTP`：同一未知邮箱第 4 次 → 429 + `Retry-After` 头。
- `TestPasswordResetConfirmRateLimitHTTP`：同一 IP 第 31 次 → 429 + `Retry-After`。

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestPasswordReset' -v`
Expected: FAIL（路由 404 / 字段未定义）。

- [ ] **Step 3: 加 DTO 与 handler**

`dto.go` 加两个结构体（字段名与 tag 如上）。

`controller.go`：

- `controller` 增加 `passwordResetService *passwordreset.Service`；
- `newController` 增加第 5 个参数并赋值；
- `register(r)` 增加：

```go
r.POST("/users/password-resets", c.requestPasswordReset)
r.POST("/users/password-resets/confirmations", c.confirmPasswordReset)
```

- handler：

```go
func (c *controller) requestPasswordReset(ctx *gin.Context) {
	if !requireJSONRequest(ctx) {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}
	var req PasswordResetRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Email == "" {
		apierror.Write(ctx, apierror.WithParam(apierror.ErrInvalidRequest, "email"))
		return
	}
	if err := c.passwordResetService.Request(ctx.Request.Context(), req.Email, requestSessionMeta(ctx).IP); err != nil {
		apierror.Write(ctx, err)
		return
	}
	ctx.JSON(http.StatusAccepted, apiresponse.Empty{})
}

func (c *controller) confirmPasswordReset(ctx *gin.Context) {
	if !requireJSONRequest(ctx) {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}
	var req PasswordResetConfirmationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Token == "" {
		apierror.Write(ctx, apierror.WithParam(apierror.ErrInvalidRequest, "token"))
		return
	}
	if req.NewPassword == "" {
		apierror.Write(ctx, apierror.WithParam(apierror.ErrInvalidRequest, "new_password"))
		return
	}
	if err := c.passwordResetService.Confirm(ctx.Request.Context(), req.Token, req.NewPassword, requestSessionMeta(ctx).IP); err != nil {
		apierror.Write(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, apiresponse.Empty{})
}
```

- [ ] **Step 4: 装配与测试助手同步**

`module.go`：`New` 增加 `passwordResetService *passwordreset.Service` 参数，传给 `newController`。

`app.go` 在 `auth.New` 之前：

```go
passwordResetService := passwordreset.New(db, settingsModule.Service, limiter, mailModule.Mailer)
```

并加到 `auth.New(...)` 末尾。

三个 flow helper 与 `auth_test.go` 的两处 `newController` 调用同步传入；直接构造：

```go
passwordResetService := passwordreset.New(db, settingsService, limiter, mailer)
```

（session flow helper 没有 `mailer` 变量时用 `&fakeMailer{configured: true}`。）

- [ ] **Step 5: 跑测试确认通过**

Run:

```bash
bash scripts/test.sh ./internal/modules/auth/ -run 'TestPasswordReset' -v
bash scripts/test.sh ./internal/modules/auth/ ./internal/modules/captcha/ ./internal/modules/registration/ ./internal/modules/session/ -v
```

Expected: 两组全绿（装配改动没有破坏既有 flow）。

- [ ] **Step 6: 静态门禁**

Run: `bash scripts/check.sh`
Expected: `check ok`。

---

### Task 8: 改密码 / 改邮箱时清令牌

**Files:**
- Modify: `backend/internal/modules/auth/repository.go`
- Modify: `backend/internal/modules/auth/password_reset_flow_test.go`

**Interfaces:**
- Consumes: Task 2 的 `password_resets` 表；ent 字段 `pwreset.UserIDEQ`。
- Produces: `updatePasswordHash` 与 `updateProfile` 在同一事务里删除该用户的 `password_resets` 行（签名不变）。

- [ ] **Step 1: 写失败测试**

在 `password_reset_flow_test.go` 加：

```go
func TestChangePasswordDeletesResetToken(t *testing.T)
func TestUpdateProfileEmailDeletesResetToken(t *testing.T)
func TestUpdateProfileWithoutEmailKeepsResetToken(t *testing.T)
```

关键断言：

- 建用户、登录、给该用户插一条 `PasswordReset`；`POST /users/me/password` 200 后，`PasswordReset` 行数 0。
- 建用户、登录、插一条；`PATCH /users/me` body `{"email":"new+<tag>@example.com"}` 200 后，行数 0。
- 建用户、登录、插一条；`PATCH /users/me` body `{"bio":"hello"}` 200 后，行数仍为 1。

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestChangePasswordDeletesResetToken|TestUpdateProfile.*ResetToken' -v`
Expected: FAIL（令牌还在）。

- [ ] **Step 3: 改 `updatePasswordHash` 为事务**

```go
func (r *repository) updatePasswordHash(ctx context.Context, id string, passwordHash string) (*ent.User, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, errUserNotFound
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	updated, err := tx.User.UpdateOneID(parsed).
		SetPasswordHash(passwordHash).
		AddAuthVersion(1).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errUserNotFound
		}
		return nil, err
	}
	if _, err := tx.PasswordReset.Delete().Where(pwreset.UserIDEQ(parsed)).Exec(ctx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return updated, nil
}
```

导入 `pwreset "website/backend/internal/database/ent/passwordreset"`。

- [ ] **Step 4: 改 `updateProfile` 为事务**

把现有 `r.client.User.UpdateOneID(...)` 改成 `tx.User.UpdateOneID(...)`，`Save` 之后：

```go
if email != nil {
	if _, err := tx.PasswordReset.Delete().Where(pwreset.UserIDEQ(parsed)).Exec(ctx); err != nil {
		return nil, err
	}
}
```

再 `tx.Commit()`。`ent.IsNotFound` 的返回逻辑保持不变。

- [ ] **Step 5: 跑测试确认通过**

Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestChangePasswordDeletesResetToken|TestUpdateProfile.*ResetToken' -v`
Expected: PASS。

- [ ] **Step 6: 回归**

Run: `bash scripts/test.sh ./internal/modules/auth/ -v`
Expected: 全绿（改密码 / 资料更新既有用例都在）。

---

### Task 9: 设置公开开关

**Files:**
- Modify: `backend/internal/modules/settings/registry.go`
- Modify: `backend/internal/modules/settings/registry_test.go`
- Modify: `backend/internal/modules/settings/controller_test.go`

**Interfaces:**
- Consumes: 现有设置清单。
- Produces: `auth.password_reset_enabled` 带 `Public: true`，匿名 `GET /public/settings` 会下发这个布尔。

- [ ] **Step 1: 写失败测试**

`registry_test.go`：

- `assertDefinitions` 里 `password_reset_enabled` 改成 `{name: "password_reset_enabled", typ: FieldTypeBool, def: false, public: true}`；
- `TestBuiltinPublicAllowlist` 的 `want` 增加 `"auth.password_reset_enabled"`（排序后位于 `"auth.totp_enabled"` 之前）。

`controller_test.go` 的公开设置用例：

```go
if !ok || len(authValues) != 2 {
	t.Fatalf("public auth values = %v, want totp_enabled and password_reset_enabled", items[2]["values"])
}
if authValues["password_reset_enabled"] != false {
	t.Fatalf("public auth.password_reset_enabled = %v, want false", authValues["password_reset_enabled"])
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/settings/ -run 'TestBuiltinPublicAllowlist|TestSettingsPublicGetAnonymously' -v`
Expected: FAIL（公开项数量 / 标志不符）。

- [ ] **Step 3: 改 registry**

```go
{Name: "password_reset_enabled", Type: FieldTypeBool, Default: false, Public: true},
```

- [ ] **Step 4: 跑测试确认通过**

Run: `bash scripts/test.sh ./internal/modules/settings/ -run 'TestBuiltinPublicAllowlist|TestSettingsPublicGetAnonymously' -v`
Expected: PASS。

- [ ] **Step 5: 全设置包回归**

Run: `bash scripts/test.sh ./internal/modules/settings/ -v`
Expected: 全绿。

---

### Task 10: 前端公开标记、登录入口与文案

**Files:**
- Modify: `frontend/lib/public-settings.ts`
- Modify: `frontend/app/login/page.tsx`
- Modify: `frontend/app/login/signin-view.tsx`
- Modify: `frontend/app/login/signin-form.tsx`
- Modify: `frontend/app/console/settings/mail/mail-settings-form.tsx`
- Modify: `frontend/messages/zh-CN.json`
- Modify: `frontend/messages/en.json`

**Interfaces:**
- Consumes: Task 9 的公开设置项。
- Produces:
  - `export function isPasswordResetEnabled(settings: PublicSettings): boolean`
  - `SigninView` / `SigninForm` 新增 `passwordResetEnabled: boolean` prop。
  - `auth.signin.forgotPassword` 与完整 `auth.reset.*` 文案键（后续两个页面直接使用）。

- [ ] **Step 1: 加公开设置读取**

`lib/public-settings.ts`：

```ts
export function isPasswordResetEnabled(settings: PublicSettings): boolean {
  return settings.auth?.password_reset_enabled === true;
}
```

- [ ] **Step 2: 登录页接线**

`app/login/page.tsx`：

```ts
import {
  getCaptchaConfig,
  getPublicSettings,
  isPasswordResetEnabled,
  isSignupOpen,
} from "@/lib/public-settings";

const passwordResetEnabled = isPasswordResetEnabled(settings);
```

两个 `return <SigninView ...>` 都加 `passwordResetEnabled={passwordResetEnabled}`。

`signin-view.tsx`：props 增加 `passwordResetEnabled: boolean`，传给 `SigninForm`。

`signin-form.tsx`：props 增加 `passwordResetEnabled`；在凭据分支（`step !== "verify"` 的 `fieldset` 内）密码框的 `div` 之后加：

```tsx
{passwordResetEnabled ? (
  <div className="flex justify-end">
    <Link
      href="/reset-password"
      className="text-sm font-medium text-primary underline-offset-4 hover:underline"
    >
      {t("forgotPassword")}
    </Link>
  </div>
) : null}
```

并在文件顶部 `import Link from "next/link";`。

- [ ] **Step 3: 加文案键**

`messages/zh-CN.json` 的 `auth.signin` 增加：

```json
"forgotPassword": "忘记密码？"
```

`en.json` 对应 `"Forgot password?"`。

在 `auth` 下新增完整 `reset` 对象（spec §9.4 的 26 个键，逐一照抄；`retryIn` 用 `{duration}` 占位）。

`settings` 命名空间：

- zh-CN：`mailTemplatePasswordReset` → `"密码重置"`；`mailTemplatePlaceholderHint` → `"可用占位符（注册验证邮件用 '{{verifyUrl}}'，密码重置用 '{{resetUrl}}'）"`。
- en：`"Password reset"`；`"Available placeholders (use '{{verifyUrl}}' for registration verification, '{{resetUrl}}' for password reset)"`。

- [ ] **Step 4: 邮件设置页占位符列表**

`app/console/settings/mail/mail-settings-form.tsx` 的 `MAIL_TEMPLATE_PLACEHOLDERS` 增加 `"resetUrl"`；保留 `"code"`（历史自定义正文可能还在用）：

```ts
const MAIL_TEMPLATE_PLACEHOLDERS = [
  "code",
  "verifyUrl",
  "resetUrl",
  "expiresMinutes",
  "siteTitle",
  "siteUrl",
  "username",
  "sentAt",
] as const;
```

标签与提示文案走 Step 3 的两个 i18n 键，组件里不硬编码。

- [ ] **Step 5: 静态门禁**

Run:

```bash
cd frontend
npx tsc --noEmit
npx eslint app/login/page.tsx app/login/signin-view.tsx app/login/signin-form.tsx app/console/settings/mail/mail-settings-form.tsx lib/public-settings.ts
npx prettier --check app/login/page.tsx app/login/signin-view.tsx app/login/signin-form.tsx app/console/settings/mail/mail-settings-form.tsx lib/public-settings.ts messages/zh-CN.json messages/en.json
```

Expected: 0 错；prettier 通过。

- [ ] **Step 6: 核对 diff**

Run: `git diff -- frontend/lib/public-settings.ts frontend/app/login frontend/app/console/settings/mail frontend/messages`
Expected: 只有入口、公开读取、占位符与文案。

---

### Task 11: 申请页 `/reset-password`

**Files:**
- Create: `frontend/app/reset-password/page.tsx`
- Create: `frontend/app/reset-password/reset-request-form.tsx`
- Create: `frontend/hooks/use-password-reset-request.ts`

**Interfaces:**
- Consumes: Task 10 的 `isPasswordResetEnabled` 与 `auth.reset.*` 文案；`apiRequest` / `ApiError` / `useServiceHealth` / `formatRetryDuration`。
- Produces: `usePasswordResetRequest()` 返回：

  ```ts
  {
    email: string;
    sent: boolean;
    isPending: boolean;
    serviceDown: boolean;
    retryRemaining: number;
    retryLabel: string | null;
    changeEmail(value: string): void;
    submit(): Promise<void>;
  }
  ```

- [ ] **Step 1: 写服务器页面**

`page.tsx`：

```tsx
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";

import {
  getPublicSettings,
  isPasswordResetEnabled,
} from "@/lib/public-settings";
import { ResetRequestForm } from "./reset-request-form";

export const instant = false;

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("auth.reset");
  return {
    title: t("title"),
    robots: { index: false, follow: false },
  };
}

export default async function ResetPasswordPage() {
  const settings = await getPublicSettings();
  if (!isPasswordResetEnabled(settings)) {
    notFound();
  }
  return <ResetRequestForm />;
}
```

- [ ] **Step 2: 写 hook**

`use-password-reset-request.ts`：

- 本地 `EMAIL_MAX_LENGTH = 254`、`EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/`（与注册页同一套，不抽公共模块）。
- 状态：`email`、`sent`、`isPending`、`retryUntil`、`retryRemaining`；`useEffect` 每秒倒计时（照 `use-signup.ts`）。
- `submit()`：
  1. `retryRemaining > 0` 直接返回；
  2. trim 后空 → `toast.error(t("requiredEmail"))`；
  3. 不匹配 → `toast.error(t("invalidEmail"))`；
  4. `apiRequest("/api/v1/users/password-resets", { method: "POST", body: { email } })`；
  5. 成功 → `setSent(true)`；
  6. `rate_limited` → `applyRateLimit(error.retryAfter ?? 60)` + toast；
  7. `mail_unavailable` → toast `t("mailUnavailable")`；
  8. `password_reset_disabled` → toast `t("disabled")`；
  9. 其余先 `resolveServiceError`，再 `t("failed")`。
- `retryLabel` 用 `formatRetryDuration(retryRemaining, locale)`。

- [ ] **Step 3: 写表单**

`reset-request-form.tsx`：

- `AuthFrame` 包裹；`h1` 用 `t("title")`；
- `sent` 为真时渲染 `t("sent")` + `<Link href="/login">{t("backToLogin")}</Link>`，不渲染表单；
- 否则 `<form>` + `Label` + `Input type="email" autoComplete="email"` + `Button type="submit"`；
- 按钮文案：`retryLabel ?? (serviceDown ? tCommon("serviceUnavailableButton") : isPending ? t("submitting") : t("submit"))`；
- `disabled={isPending || retryRemaining > 0}`。

- [ ] **Step 4: 静态门禁**

Run:

```bash
cd frontend
npx tsc --noEmit
npx eslint app/reset-password hooks/use-password-reset-request.ts
npx prettier --check app/reset-password hooks/use-password-reset-request.ts
```

Expected: 0 错。

---

### Task 12: 确认页 `/reset-password/confirm`

**Files:**
- Create: `frontend/app/reset-password/confirm/page.tsx`
- Create: `frontend/app/reset-password/confirm/reset-confirm-form.tsx`
- Create: `frontend/hooks/use-password-reset-confirm.ts`

**Interfaces:**
- Consumes: Task 10 的 `auth.reset.*` 文案；`isStrongPassword` / `PASSWORD_MIN_LENGTH`；`apiRequest` / `ApiError`。
- Produces: `usePasswordResetConfirm(token)` 返回：

  ```ts
  {
    values: { password: string; confirmPassword: string };
    isPending: boolean;
    serviceDown: boolean;
    retryRemaining: number;
    retryLabel: string | null;
    changeField(field: "password" | "confirmPassword", value: string): void;
    submit(): Promise<void>;
    invalidToken: boolean;
  }
  ```

- [ ] **Step 1: 写服务器页面**

`confirm/page.tsx`：

```tsx
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";

import {
  getPublicSettings,
  isPasswordResetEnabled,
} from "@/lib/public-settings";
import { ResetConfirmForm } from "./reset-confirm-form";

export const instant = false;

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("auth.reset");
  return {
    title: t("confirmTitle"),
    referrer: "no-referrer",
    robots: { index: false, follow: false },
  };
}

export default async function ResetConfirmPage({
  searchParams,
}: {
  searchParams: Promise<{ token?: string }>;
}) {
  const settings = await getPublicSettings();
  if (!isPasswordResetEnabled(settings)) {
    notFound();
  }
  const { token } = await searchParams;
  return <ResetConfirmForm token={typeof token === "string" ? token : ""} />;
}
```

- [ ] **Step 2: 写 hook**

`use-password-reset-confirm.ts`：

- 状态：`values`、`isPending`、`retryUntil`、`retryRemaining`、`invalidToken`。
- 组件挂载时若 `token === ""`：`toast.error(t("invalidToken"))` 并 `setInvalidToken(true)`（`useEffect`，照 `verify-form.tsx`）。
- `submit()`：
  1. `retryRemaining > 0 || invalidToken` 直接返回；
  2. 空密码 → `requiredPassword`；空确认 → `requiredConfirmPassword`；两次不一致 → `mismatch`；
  3. `!isStrongPassword(password)` → `weakPassword`（含 72 字节上限）；
  4. `apiRequest("/api/v1/users/password-resets/confirmations", { method: "POST", body: { token, new_password: password } })`；
  5. 成功 → `toast.success(t("success"))` + `router.push("/login")`；
  6. `password_reset_invalid` → `setInvalidToken(true)` + toast `invalidToken`；
  7. `weak_password` → toast `weakPassword`；
  8. `password_unchanged` → toast `passwordUnchanged`；
  9. `password_reset_disabled` → toast `disabled`；
  10. `rate_limited` → apply retry + toast；
  11. 其余 `resolveServiceError` → `t("failed")`。

- [ ] **Step 3: 写表单**

`reset-confirm-form.tsx`：

- `AuthFrame` 包裹；`h1` 用 `t("confirmTitle")`。
- `invalidToken` 为真时渲染 `t("invalidToken")` + `<Link href="/reset-password">{t("goRequest")}</Link>`，不渲染密码表单。
- 否则两个 `InputGroup` 密码框：
  - 第一个 `name="password"`、`autoComplete="new-password"`、`Label` 用 `t("password")`；
  - 第二个 `name="confirmPassword"`、`autoComplete="new-password"`、`Label` 用 `t("confirmPassword")`；
  - 各自 `Eye` / `EyeOff` 切换，`aria-label` 用 `tCommon("showPassword")` / `tCommon("hidePassword")`；
- 密码提示：`{t("passwordHint", { minLength: PASSWORD_MIN_LENGTH })}`，样式 `text-xs text-muted-foreground`。
- 提交按钮：`retryLabel ?? (serviceDown ? tCommon("serviceUnavailableButton") : isPending ? t("submitting") : t("submitConfirm"))`；`disabled={isPending || retryRemaining > 0}`。
- **不自动提交**：只有表单 `onSubmit` 才调用 `submit()`；页面 GET 不消费令牌。

- [ ] **Step 4: 静态门禁**

Run:

```bash
cd frontend
npx tsc --noEmit
npx eslint app/reset-password hooks/use-password-reset-confirm.ts
npx prettier --check app/reset-password hooks/use-password-reset-confirm.ts
```

Expected: 0 错。

- [ ] **Step 5: 核对 diff**

Run: `git diff --stat -- frontend/app/reset-password frontend/hooks/use-password-reset-*.ts`
Expected: 只含两个页面与两个 hook。

---

### Task 13: 文档、全量验证与变异自检

**Files:**
- Modify: `backend/README.md`
- Modify: `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md`
- Modify: `docs/DEVLOG.md`

- [ ] **Step 1: 更新 README**

- 目录表：加 `internal/modules/passwordreset/*`（无路由能力模块，申请/确认、令牌、限流、发信补偿）。
- 认证接口表：加 `POST /users/password-resets` 与 `POST /users/password-resets/confirmations`，逐条写 202/200、400/403/429/503 与两个新错误码。
- 设置清单：`auth.password_reset_enabled` 改成「默认 `false`、已标 `Public`、消费方为密码重置两接口与登录页入口」。
- 邮件章节：`password_reset` 模板改成链接版、占位符 `{{resetUrl}}`、默认主题「密码重置」、预览链接；业务发送已接。
- 错误码表：加 `password_reset_disabled` / `password_reset_invalid`。

- [ ] **Step 2: 更新 API 规范**

- §3 端点表按现有编号追加两个端点（放在 #29 之后），注明公开、请求体、状态与错误码。
- §7.3 错误码表加两个新码（403 `permission_error` / 400 `invalid_request_error`，param `token`）。
- 不写版本号。

- [ ] **Step 3: 更新 DEVLOG**

在 `docs/DEVLOG.md` 顶部倒序加一条，按现有格式写动机、范围、决策、验证、已知缺口、**提交状态（未提交，待用户检查）**。决策里至少记：未知邮箱 202 / 发信失败 503、令牌 30 分钟单次、开关公开、2FA 保留、本轮不接人机验证、正常改密码/改邮箱清令牌。

- [ ] **Step 4: 全量后端门禁**

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

Expected: `check ok` + 全绿。若有失败，先确认不是并行会话或残留行导致；串行重跑。

- [ ] **Step 5: 全量前端检查**

Run:

```bash
cd frontend
npx tsc --noEmit
npx eslint app/login app/reset-password app/console/settings/mail hooks/use-password-reset-request.ts hooks/use-password-reset-confirm.ts lib/public-settings.ts
npx prettier --check app/login app/reset-password app/console/settings/mail/mail-settings-form.tsx hooks/use-password-reset-request.ts hooks/use-password-reset-confirm.ts lib/public-settings.ts messages/zh-CN.json messages/en.json
```

Expected: 0 error；prettier 通过。eslint 既有 warning 不计入。

- [ ] **Step 6: 变异自检（spec §10.3）**

逐条把代码改坏、跑对应测试、确认变红，再改回：

1. 未知邮箱分支改成返回 404 / `ErrNotFound` → `TestRequestUnknownEmailNoSend` 或 HTTP 用例红；
2. 删除 `AddAuthVersion(1)` → `TestConfirmChangesPasswordAndVersion` / 旧会话 401 用例红；
3. 补偿删除改成按 `user_id` → `TestDeleteResetRequiresTokenHash` / `TestRequestSendFailureCompensates` 红；
4. 认领条件去掉 `token_hash` → `TestCompleteResetStaleHash` 红；
5. 去掉开关判断 → `TestRequestDisabled` / `TestPasswordResetRequestDisabledHTTP` 红；
6. 弱密码校验挪到认领之后 → `TestConfirmWeakPasswordKeepsToken` 红；
7. 去掉 `Public: true` → `TestBuiltinPublicAllowlist` / 公开设置用例红。

每条记录「改了哪行、哪条测试红、已改回」。

- [ ] **Step 7: 汇总，不提交**

报告：改了哪些文件、测试命令与结果、变异自检结果、已知缺口（照 spec §12）。**不要 `git add` / `git commit` / `git push`。** 真实收信、浏览器点按与运行期验收由用户执行；计划里的手动清单如下，供用户核对：

1. 开关关：登录页无入口，`/reset-password` 与 `/reset-password/confirm` 404；
2. 开关开：登录页出现「忘记密码？」；申请存在的邮箱与不存在的邮箱都显示中性成功态；
3. 邮件链接打开确认页，两次密码不一致被前端拦截；弱密码 toast；正确密码成功后回登录页；
4. 旧密码登录失败、新密码登录成功；另一浏览器里的旧会话下一次点击触发会话失效；
5. 同一链接第二次打开显示「链接无效或已过期」；
6. 429 / 503 / 403 各至少实测一次；
7. 邮件设置页标签与占位符提示更新，测试发送 `password_reset` 收到链接版预览。
