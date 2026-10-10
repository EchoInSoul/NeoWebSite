# NeoBlog 认证层 1 实施计划（登录 / 登出 / 会话守卫）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给 NeoBlog 后端加上管理员登录 / 登出 / 会话守卫，并把前端登录页接上接口。

**Architecture:** 会话用 `alexedwards/scs`（Postgres 存储，浏览器只拿一个不透明 token cookie）；密码 bcrypt；数据访问用 ent（schema 建模 → Atlas 生成迁移）；认证是标准模块（module / controller / service / repository / dto），守卫放 `internal/common/guard`，装配只在 `app.go`。

**Tech Stack:** Go 1.27 + Gin + ent + Atlas + scs(postgresstore) + bcrypt；前端 Next 16（rewrite 代理 + fetch）。

**Spec:** `docs/superpowers/specs/2026-10-11-neoblog-auth-design.md`

## Global Constraints

- 手写 Go 文件**零注释**（ent 生成物与 `//go:build`、`//go:embed` 这类工具链指令豁免）；`gofmt -l .` 必须为空。
- 依赖方向 controller → service → repository；`internal/database` 不得 import 任何业务模块；业务模块禁止自建 `sql.Open`。
- `/api/v1` 前缀只在 `app.go` 出现一次；模块能力只通过 `New(...) *Module` 暴露，装配只在 `app.go`。
- 本机 Git Bash 里先执行（`gopath/bin` 里是 `atlas` 等工具，必须在 PATH 里）：`export PATH="/f/software/go/bin:/f/software/gopath/bin:$PATH" GOPATH='F:/software/gopath' GOCACHE='F:/software/gocache' GOPROXY='https://goproxy.cn,direct'`
- Docker 只在 WSL 里：`wsl -d Ubuntu -- docker ...`（Windows 侧没有 docker CLI）；Atlas 二进制在 `F:/software/gopath/bin/atlas.exe`。
- 数据库：`neoblog`（`127.0.0.1:5434`）、测试库 `neoblog_test`、Atlas dev 库 `neoblog_atlas_dev`；`backend/.env` 已存在并通过 `.gitignore` 排除，**不要提交它**。
- 集成测试约定：读 `DATABASE_URL_TEST`，缺省 `t.Skip`；跑集成测试前先应用迁移。
- 提交信息中文、conventional 前缀（`feat(backend): ...` / `docs(backend): ...` / `chore(backend): ...`），每个任务一次提交。
- 本计划整体完成时执行 `./scripts/bump-version.sh minor`（0.1.0 → 0.2.0）。

## Review Focus

1. 邮箱大小写：`Admin@Example.COM` 也能登录（存 / 查统一转小写；nickname 原样匹配）——T6 覆盖。
2. 密码长度：超过 72 字节（bcrypt 上限）时建号被拒；登录返回 401 而不是 500——T5 / T6 覆盖。
3. 请求体异常：畸形 JSON、缺字段、空字符串 → 400 `{"error":"invalid_request"}`（不是 500、不是 401）——T6 覆盖。
4. 用户被删除但会话仍存在：`/auth/me` → 401 且会话被销毁——T8 覆盖。
5. Cookie 属性：HttpOnly、SameSite=Lax、Path=/ 恒有；`Secure` 仅当 `APP_ENV=production`——T8 覆盖。

---

### Task 1: ent 代码生成管线（users / sessions schema）

**Files:**
- Create: `backend/tools.go`
- Create: `backend/internal/database/ent/schema/user.go`
- Create: `backend/internal/database/ent/schema/session.go`
- Create: `backend/scripts/db-generate.sh`
- Modify: `backend/go.mod`（`golang.org/x/tools` 顶到 `v0.52.0`，ent 在 Go 1.27 下生成所需，见数据层 spec §7.1）
- Generated（提交进 Git）: `backend/internal/database/ent/**`

**Interfaces:**
- Produces: 包 `ent`（`website/backend/internal/database/ent`）及生成类型 `ent.Client` / `ent.User` / `ent.Session`、查询构造器 `ent.User.Query()` 等。

- [ ] **Step 1: 写 `user.go` schema**

字段：`email`（String, Unique, NotEmpty）、`nickname`（String, Unique, NotEmpty）、`password_hash`（String, NotEmpty）、`created_at`（Time, Default now, Immutable）、`updated_at`（Time, Default now, UpdateDefault now）；id 用 `field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) })` 生成 UUIDv7（注意：ent 的 uuid 字段只有 `Default`，没有 `DefaultFunc`）；对 `created_at` 建索引。

- [ ] **Step 2: 写 `session.go` schema**

字段：`token`（String, Unique, NotEmpty）、`data`（[]byte, NotEmpty）、`expiry`（Time；ent 的 Time 字段没有 `NotEmpty`，不加即 NOT NULL），并对 `expiry` 建索引；**不要显式声明 `id`**——ent 会自动补一个自增主键列，scs 不读写它。（该表是 scs postgresstore 的存储约定，ent 只负责生成它的 DDL；用途说明写进 README，代码保持零注释。）

- [ ] **Step 3: 写 `tools.go` 并 `go mod tidy`**

`tools.go` 用 `//go:build tools` 标签 + `import _ "entgo.io/ent/cmd/ent"`（防止 `go mod tidy` 清掉生成依赖）。之后 `go build ./...` 应通过。

- [ ] **Step 4: 写 `scripts/db-generate.sh` 并执行**

脚本内容：`cd` 到 `backend/` 后执行 `go run entgo.io/ent/cmd/ent generate ./internal/database/ent/schema`。
Run: `bash scripts/db-generate.sh`
Expected: `internal/database/ent/` 出现生成物（`ent.go`、`user/`、`session/`、`migrate/` 等）。

- [ ] **Step 5: 全量检查**

Run: `gofmt -l . && go vet ./... && go build ./...`
Expected: 全绿（生成物带工具链注释，豁免零注释规则）。

- [ ] **Step 6: Commit**

```bash
git add backend/tools.go backend/internal/database/ent backend/scripts/db-generate.sh backend/go.mod backend/go.sum
git commit -m "feat(backend): ent 代码生成管线与 users/sessions schema"
```

---

### Task 2: `internal/database` 内建 ent Client

**Files:**
- Modify: `backend/internal/database/database.go`
- Modify: `backend/internal/database/database_test.go`

**Interfaces:**
- Consumes: Task 1 的 `ent` 包。
- Produces: `Database.Client *ent.Client`（导出字段）；`func (d *Database) SQL() *sql.DB`；`Open(ctx, cfg) (*Database, error)` 签名不变。

- [ ] **Step 1: 写失败的集成测试**

在 `database_test.go` 里沿用该文件已有的「读 `DATABASE_URL_TEST`、缺省 `t.Skip`」写法，新增 `TestOpenBuildsEntClient`：`Open` 成功后断言 `db.Client != nil`、`db.SQL().PingContext(ctx)` 为 nil。**不要**在此时查 users 表（表在 Task 3 才建）。

- [ ] **Step 2: 跑测试确认失败**

Run: `DATABASE_URL_TEST='postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_test?sslmode=disable' go test ./internal/database/ -count=1`
Expected: 编译失败（`db.Client` / `db.SQL` 未定义）。

- [ ] **Step 3: 实现**

在 `Open` 内：`drv := entsql.OpenDB(dialect.Postgres, db)`；`client := ent.NewClient(ent.Driver(drv))`（注意：`ent.NewClient` / `ent.Driver` 由生成代码提供，`ent` 指本地包 `website/backend/internal/database/ent`；另外 import `entgo.io/ent/dialect` 与 `entsql "entgo.io/ent/dialect/sql"`）；结构体加 `Client` 字段；`Close()` 先关 client 再关 `*sql.DB`；新增 `SQL()` 返回底层 `*sql.DB`。

- [ ] **Step 4: 跑测试确认通过**

Run: 同 Step 2，Expected: PASS；再跑一次不带 `DATABASE_URL_TEST` 的 `go test ./internal/database/ -count=1`，Expected: PASS（用例 skip）。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/database
git commit -m "feat(backend): 数据连接内建 ent client"
```

---

### Task 3: 首个迁移（Atlas：users + sessions）

**Files:**
- Create: `backend/scripts/db-migrate-diff.sh`
- Create: `backend/scripts/db-migrate-apply.sh`
- Generated: `backend/migrations/<时间戳>_init_users_sessions.sql` + `backend/migrations/atlas.sum`

**Interfaces:**
- Consumes: Task 1 的 schema；Task 2 的可连库。
- Produces: `users`、`sessions` 两张表存在于 `neoblog` 与 `neoblog_test`；`migrations/` 目录成为后续一切表变更的唯一入口。

- [ ] **Step 1: 写两个脚本**

`db-migrate-diff.sh`：脚本内先取 `ATLAS_DEV_URL="${ATLAS_DEV_URL:-postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_atlas_dev?sslmode=disable}"`（该变量目前不在 `.env` / README 里，就在脚本里给默认值），再执行 `atlas migrate diff init_users_sessions --dir "file://migrations" --to "ent://internal/database/ent/schema" --dev-url "$ATLAS_DEV_URL"`。
`db-migrate-apply.sh`：对 `DATABASE_URL` 和 `DATABASE_URL_TEST` 各执行一次 `atlas migrate apply --dir "file://migrations" --url "<dsn>"`。两个脚本都先 `cd` 到 `backend/`（`file://` 与 `ent://` 都相对当前目录解析）。

- [ ] **Step 2: 生成迁移并人工审阅 SQL**

Run: `bash scripts/db-migrate-diff.sh`
Expected: `migrations/` 出现一个 `*_init_users_sessions.sql` + `atlas.sum`（幂等：schema 无变化时重跑输出 no changes 且不产生新文件）。**逐行审阅 SQL**：确认 `users`（email / nickname 唯一、password_hash、时间戳、created_at 索引）、`sessions`（token 唯一、data bytea、expiry + 索引）。若调整了 SQL 内容，在 `backend/` 下执行 `atlas migrate hash --dir "file://migrations"` 重新生成校验和。

- [ ] **Step 3: 应用到两个库并验证**

Run: `bash scripts/db-migrate-apply.sh`
Expected: 输出 1 migration / ok（两个库各一次）。验证（两个库都查）：
`wsl -d Ubuntu -- docker exec neoblog-postgres psql -U neoblog -d neoblog -c "SELECT table_name FROM information_schema.tables WHERE table_schema='public' ORDER BY 1;"`
`wsl -d Ubuntu -- docker exec neoblog-postgres psql -U neoblog -d neoblog_test -c "SELECT table_name FROM information_schema.tables WHERE table_schema='public' ORDER BY 1;"`
Expected: 两个库都含 `sessions`、`users`。

- [ ] **Step 4: 校验**

Run: `atlas migrate validate --dir "file://migrations"`
Expected: 通过。

- [ ] **Step 5: Commit**

```bash
git add backend/migrations backend/scripts/db-migrate-diff.sh backend/scripts/db-migrate-apply.sh
git commit -m "feat(backend): users/sessions 首个迁移与迁移脚本"
```

---

### Task 4: 会话守卫 `internal/common/guard`（TDD，不连库）

**Files:**
- Create: `backend/internal/common/guard/guard.go`
- Create: `backend/internal/common/guard/guard_test.go`
- Modify: `backend/go.mod` / `backend/go.sum`（引入 `github.com/alexedwards/scs/v2`）

**Interfaces:**
- Produces: `const SessionUserIDKey = "auth.user_id"`；`func RequireAuth(sm *scs.SessionManager) gin.HandlerFunc`——未登录：`401` + `{"error":"unauthorized"}`；已登录：把 userID `c.Set(SessionUserIDKey, ...)` 后放行。

- [ ] **Step 1: 写失败测试**

用 `scs.New()`（内存 store）+ httptest 包一个测试路由：未登录 GET → 断言 401 与 body `{"error":"unauthorized"}`；预置会话用两个请求完成——请求 A：handler 里 `sm.Put(ctx, guard.SessionUserIDKey, "u1")`，响应会带会话 cookie；请求 B：带上 A 的 cookie，断言放行且 `c.GetString(guard.SessionUserIDKey) == "u1"`。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/common/guard/ -count=1`
Expected: 编译失败（`RequireAuth` 未定义）。

- [ ] **Step 3: 实现**

`RequireAuth`：`userID := sm.GetString(c.Request.Context(), SessionUserIDKey)`；空 → `c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})`；否则 `c.Set(SessionUserIDKey, userID)` + `c.Next()`。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/common/guard/ -count=1`，Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/common/guard backend/go.mod backend/go.sum
git commit -m "feat(backend): 会话守卫中间件 internal/common/guard"
```

---

### Task 5: auth 模块数据访问与建号入口（真库）

**Files:**
- Create: `backend/internal/modules/auth/dto.go`
- Create: `backend/internal/modules/auth/repository.go`
- Create: `backend/internal/modules/auth/admin.go`
- Create: `backend/internal/modules/auth/repository_test.go`
- Modify: `backend/go.mod` / `backend/go.sum`（`golang.org/x/crypto` 变为直接依赖）

**Interfaces:**
- Consumes: Task 2 的 `*database.Database`（字段 `Client`）、Task 3 的迁移。
- Produces:
  - `dto.go`：`type User struct { ID, Email, Nickname string }`、`type LoginRequest struct { Identifier string \`json:"identifier"\`; Password string \`json:"password"\` }`、`type UserResponse struct { Email string \`json:"email"\`; Nickname string \`json:"nickname"\` }`、`type ErrorResponse struct { Error string \`json:"error"\` }`
  - `repository.go`（包内私有）：`newRepository(db *database.Database) *repository`、`findUserByIdentifier(ctx, identifier string) (*ent.User, error)`（email 转小写后与 email 或 nickname 匹配）、`findUserByID(ctx, id string) (*ent.User, error)`、`createUser(ctx, email, nickname, passwordHash string) (*ent.User, error)`；哨兵错误 `errUserNotFound`
  - `admin.go`：`func CreateAdminUser(ctx context.Context, db *database.Database, email, nickname, password string) (User, error)`（email 统一小写、bcrypt 哈希、超 72 字节密码返回错误、重复唯一键返回明确错误）——`cmd/create-admin` 与集成测试共用此入口
  - 约定：ent 类型只在模块内部流转，对外一律出 `User`

- [ ] **Step 1: 写失败的集成测试**

`repository_test.go`（真库，沿用 skip 约定）：`CreateAdminUser` 建号 → `findUserByIdentifier` 用**小写邮箱**、**大写变体邮箱**、**昵称**三种写法都能命中；`findUserByID` 命中；建号后用 `bcrypt.CompareHashAndPassword` 校验哈希（原密码 → nil，错误密码 → 非 nil）；重复 email 与重复 nickname → 返回非 nil 错误且不是 `errUserNotFound`；不存在的 UUID → `errUserNotFound`；73 字节密码 → 返回错误。测试账号标识一律带随机后缀（如 `admin+<随机>@test.local`），`t.Cleanup` 删除建出的用户行。

- [ ] **Step 2: 跑测试确认失败**

Run: `DATABASE_URL_TEST='postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_test?sslmode=disable' go test ./internal/modules/auth/ -count=1`
Expected: 编译失败（函数未定义）。

- [ ] **Step 3: 实现 `dto.go` / `repository.go` / `admin.go`**

要点：`findUserByIdentifier` 用 `Query().Where(user.Or(user.EmailEQ(strings.ToLower(identifier)), user.NicknameEQ(identifier)))` + `Only`，`ent.IsNotFound` → `errUserNotFound`；`createUser` 返回 ent 错误原样包装（唯一键冲突交给调用方判断）；`CreateAdminUser` 里 `len(password) > 72` → `errors.New("password must be at most 72 bytes")`。

- [ ] **Step 4: 跑测试确认通过**

Run: 同 Step 2，Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/auth backend/go.mod backend/go.sum
git commit -m "feat(backend): auth 模块数据访问与管理员账号创建入口"
```

---

### Task 6: auth 模块业务与 HTTP 层（内存会话冒烟）

**Files:**
- Create: `backend/internal/modules/auth/service.go`
- Create: `backend/internal/modules/auth/controller.go`
- Create: `backend/internal/modules/auth/module.go`
- Create: `backend/internal/modules/auth/auth_test.go`

**Interfaces:**
- Consumes: Task 4 的 `guard.SessionUserIDKey`、Task 5 的 repository / `CreateAdminUser`、scs。
- Produces:
  - `var ErrInvalidCredentials = errors.New("invalid credentials")`、`var ErrUnauthorized = errors.New("unauthorized")`
  - `func newService(cfg config.Config, db *database.Database, sessions *scs.SessionManager) *Service`
  - `func (s *Service) Login(ctx context.Context, identifier, password string) (User, error)`——成功时 `sessions.RenewToken(ctx)` + `sessions.Put(ctx, guard.SessionUserIDKey, id)`；凭据错 → `ErrInvalidCredentials`
  - `func (s *Service) Logout(ctx context.Context)`——`sessions.Destroy(ctx)`
  - `func (s *Service) CurrentUser(ctx context.Context, userID string) (User, error)`——用户不存在 → `ErrUnauthorized` 并 `Destroy`
  - `type Module struct { Service *Service; controller *controller }`；`func New(cfg config.Config, db *database.Database, sessions *scs.SessionManager) *Module`；`Register(r *gin.RouterGroup)`（`POST /auth/login`）、`RegisterProtected(r *gin.RouterGroup)`（`POST /auth/logout`、`GET /auth/me`）

- [ ] **Step 1: 写失败的 HTTP 冒烟测试**

`auth_test.go`（`package auth`，httptest + `scs.New()` 内存 store + 真库）：自建 mini 路由（gin.New + `sessions.LoadAndSave(engine)` + 公开组 + `guard.RequireAuth` 保护组）。
断言清单：
  1. 正确凭据 → 200 `{"email","nickname"}` + `Set-Cookie`；2. 密码错 → 401 `invalid_credentials`；3. 未知账号 → 401 同形状；4. 畸形 JSON / 缺字段 / 空字段 → 400 `{"error":"invalid_request"}`；5. 73 字节密码 → 401（不是 500；bcrypt 只比较前 72 字节，测试用短口令保证断言稳定）；6. 大写邮箱能登录；7. 带旧 cookie 再登录 → cookie 值变化（RenewToken）；8. 带 cookie GET `/auth/me` → 200；不带 → 401；9. `POST /auth/logout` → 204，随后 me → 401。测试账号带随机后缀，`t.Cleanup` 删除建出的行。

- [ ] **Step 2: 跑测试确认失败**

Run: `DATABASE_URL_TEST='postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_test?sslmode=disable' go test ./internal/modules/auth/ -count=1`
Expected: 编译失败。

- [ ] **Step 3: 实现 service / controller / module**

要点：service 里账号不存在时也跑一次等价的 bcrypt 比较（抗时序探测）；`len(password) > 72` 直接返回 `ErrInvalidCredentials`（bcrypt 只取前 72 字节，显式拒绝更清楚）；controller 用 `ctx.ShouldBindJSON` 绑定（gin 没有 `json.BindJSON`），绑定失败或字段为空一律映射为 400 `invalid_request`，把 `ErrInvalidCredentials` 映射为 401；`/auth/me` 从 gin.Context 取 `guard.SessionUserIDKey`。

- [ ] **Step 4: 跑测试确认通过**

Run: 同 Step 2，Expected: PASS。

- [ ] **Step 5: 无库环境回归**

Run: `go test ./... -count=1`（不带 `DATABASE_URL_TEST`），Expected: 全绿（集成用例 skip）。

- [ ] **Step 6: Commit**

```bash
git add backend/internal/modules/auth
git commit -m "feat(backend): auth 模块（登录/登出/me）与守卫接线"
```

---

### Task 7: 管理员创建工具 `cmd/create-admin`

**Files:**
- Create: `backend/cmd/create-admin/main.go`

**Interfaces:**
- Consumes: `config.LoadDotEnv` / `config.Load`、`database.Open`、`auth.CreateAdminUser`。
- Produces: `go run ./cmd/create-admin --email <e> --nickname <n>`，密码从 stdin 读一行；成功打印创建的账号；失败非零退出。

- [ ] **Step 1: 实现**

flag 解析 `--email` / `--nickname`（都必填，缺失给用法并非零退出）；提示 `password: ` 写到 stderr；`bufio` 读 stdin 一行；走 `LoadDotEnv`（允许从 `.env` 拿库地址）→ `database.Open` → `auth.CreateAdminUser`。

- [ ] **Step 2: 对测试库真机验证**

Run（用一次性账号，避免污染测试库）：
```bash
STAMP=$(date +%s)
printf 'pass1234\n' | DATABASE_URL='postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_test?sslmode=disable' go run ./cmd/create-admin --email "admin+$STAMP@test.local" --nickname "admin$STAMP"
wsl -d Ubuntu -- docker exec neoblog-postgres psql -U neoblog -d neoblog_test -c "SELECT email, nickname FROM users WHERE email LIKE 'admin+%@test.local';"
```
Expected: 第一次成功并打印账号；库中可见该行。

- [ ] **Step 3: 重复与缺参验证**

Run: 用**同一个 `STAMP`** 再执行一次 Step 2 的 create-admin 命令 → Expected: 非零退出 + 明确的重复错误；`go run ./cmd/create-admin` 不带参数 → 用法提示 + 非零退出。验证完清理：
```bash
wsl -d Ubuntu -- docker exec neoblog-postgres psql -U neoblog -d neoblog_test -c "DELETE FROM users WHERE email LIKE 'admin+%@test.local';"
```

- [ ] **Step 4: Commit**

```bash
git add backend/cmd
git commit -m "feat(backend): 管理员创建工具 cmd/create-admin"
```

---

### Task 8: app 装配（真 Postgres 会话存储）与端到端测试

**Files:**
- Modify: `backend/internal/config/config.go`（新增 `SessionLifetime`）
- Modify: `backend/internal/config/config_test.go`
- Modify: `backend/internal/app/app.go`
- Create: `backend/internal/app/app_test.go`
- Modify: `backend/go.mod` / `backend/go.sum`（引入 `github.com/alexedwards/scs/postgresstore`）

**Interfaces:**
- Consumes: Task 4 的 guard、Task 6 的 auth 模块、`scs/postgresstore`。
- Produces: `App` 新增 `func (a *App) Handler() http.Handler`（返回 `sessions.LoadAndSave(engine)` 包装后的处理器，供 `Run` 与测试使用）；`app.New(cfg, db)` 内构造会话管理器并完成「公开组 + 保护组」装配。

- [ ] **Step 1: 加 `SESSION_LIFETIME` 配置（TDD）**

`config.Load()` 里用现有 `getEnvDuration` 增加 `SessionLifetime`（`SESSION_LIFETIME`，默认 `168h`）；`config_test.go` 补三条断言：缺省为 168h、设 `SESSION_LIFETIME=1h` 时为 1h、非法值报错。
Run: `go test ./internal/config/ -count=1`，Expected: PASS。

- [ ] **Step 2: 写失败的端到端测试**

`app_test.go`（真库 + 真 postgresstore）：测试构造的 `config.Config` 显式设 `SessionLifetime: time.Hour`（否则 Lifetime=0 会让会话立即过期，测试会以很迷惑的方式失败）；用 `auth.CreateAdminUser` 造账号，经 `app.New(cfg, db).Handler()` 走完整流程：
  1. `POST /api/v1/auth/login` → 200 + cookie；2. cookie 断言 `HttpOnly`、`SameSite=Lax`、`Path=/`、development 下无 `Secure`；3. 带 cookie `GET /api/v1/auth/me` → 200；4. `POST /api/v1/auth/logout` → 204，随后 me → 401；5. 删除该用户后（用 repository 或直接 `db.Client.User.DeleteOneID(id).Exec(ctx)`）重启会话再用旧 cookie 访问 me → 401 且响应带清除会话的 `Set-Cookie`（Max-Age=-1 / 过期时间在过去）；6. `GET /api/v1/healthy` → 200 且 `database` 为 `ok`；7. 另起一个 `Environment: "production"` 的 app，断言登录响应的 cookie 带 `Secure`；8. 未登录 `POST /api/v1/auth/logout` → 401；9. 账号邮箱带随机后缀，`t.Cleanup` 删除建出的用户行。

- [ ] **Step 3: 跑测试确认失败**

Run: `DATABASE_URL_TEST=... go test ./internal/app/ -count=1`
Expected: 编译失败（`Handler` 未定义）。

- [ ] **Step 4: 实现 app 装配**

要点：`sm := scs.New()`；`sm.Lifetime = cfg.SessionLifetime`；`sm.Store = postgresstore.New(db.SQL())`；`sm.Cookie.Name = "session"`、`HttpOnly = true`、`SameSite = http.SameSiteLaxMode`、`Path = "/"`、`Secure = cfg.Environment == "production"`（scs 字段名以包文档为准）；`authModule := auth.New(cfg, db, sm)`；公开组注册 login；`protected := api.Group("", guard.RequireAuth(sm))`；`a.handler = sm.LoadAndSave(engine)`；`Run` 的 `http.Server.Handler` 改用 `a.handler`。

- [ ] **Step 5: 跑测试确认通过**

Run: `DATABASE_URL_TEST=... go test ./internal/app/ ./internal/modules/... ./internal/common/... -count=1`，Expected: PASS；再全量 `go test ./... -count=1`，Expected: 全绿。

- [ ] **Step 6: Commit**

```bash
git status
git add backend/internal/config backend/internal/app backend/go.mod backend/go.sum
git commit -m "feat(backend): app 装配会话存储与认证模块"
```

---

### Task 9: 前端接线（/api 转发 + 登录表单）

**Files:**
- Modify: `frontend/next.config.ts`
- Modify: `frontend/app/signin/signin-form.tsx`

**Interfaces:**
- Consumes: Task 6/8 的 `POST /api/v1/auth/login`（同源经 rewrite）。
- Produces: 浏览器 `localhost:3000` 上登录成功后跳 `/console`；cookie 由同源代理自然写入。

- [ ] **Step 1: 加 rewrite**

`const backendUrl = process.env.BACKEND_URL ?? "http://127.0.0.1:5732";` + `async rewrites()` 返回 `[{ source: "/api/:path*", destination: \`${backendUrl}/api/:path*\` }]`。

- [ ] **Step 2: 接登录表单**

`handleSubmit` 改为：`FormData` 取 `identifier` / `password` → `fetch("/api/v1/auth/login", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ identifier, password }), credentials: "same-origin" })`；200 → `router.push("/console")`（`useRouter` from `next/navigation`）；401 → 就地显示「用户名或密码错误」；其他/网络错误 → 「登录失败，请稍后重试」。提交中禁用按钮。

- [ ] **Step 3: 类型检查**

Run: `cd frontend && npx tsc --noEmit`，Expected: 通过。

- [ ] **Step 4: 真机端到端**

前置：`bash backend/scripts/db-up.sh`、迁移已应用、管理员已建（Task 7）。
Run: 后端起（`cd backend && bash scripts/dev.sh`），前端 `npm run dev`，浏览器打开 `http://localhost:3000/signin`，用 Task 7 的账号登录。
Expected: 跳转 `/console`；DevTools 里 cookie `session` 为 HttpOnly；在地址栏访问 `http://localhost:3000/api/v1/auth/me` 显示当前用户 JSON。退出与未登录态用 cookie jar 的 curl 验证（把 `<账号>`/`<密码>` 换成本步登录用的值）：
```bash
curl -s -c /tmp/jar -X POST -H 'Content-Type: application/json' -d '{"identifier":"<账号>","password":"<密码>"}' http://127.0.0.1:5732/api/v1/auth/login
curl -s -b /tmp/jar http://127.0.0.1:5732/api/v1/auth/me
curl -s -o /dev/null -w 'logout=%{http_code}\n' -b /tmp/jar -X POST http://127.0.0.1:5732/api/v1/auth/logout
curl -s -o /dev/null -w 'me_after_logout=%{http_code}\n' -b /tmp/jar http://127.0.0.1:5732/api/v1/auth/me
```
Expected: 第二条显示当前用户；`logout=204`；`me_after_logout=401`。

- [ ] **Step 5: Commit**

```bash
git add frontend/next.config.ts frontend/app/signin/signin-form.tsx
git commit -m "feat(frontend): 登录表单接后端与 /api 转发"
```

---

### Task 10: README、版本进位与终验

**Files:**
- Modify: `backend/README.md`
- Modify: `backend/internal/version/version.go`（脚本改）

**Interfaces:**
- Produces: 文档与版本号与实现一致；spec §10 六条验收全部跑过。

- [ ] **Step 1: 更新 README**

补：三个接口一览；环境变量表加 `SESSION_LIFETIME`（默认 `168h`）；目录对照表加 `internal/modules/auth`、`internal/common/guard`、`cmd/create-admin`、`migrations/`；数据库章节补「迁移工作流（diff → 审阅 → apply）与首次应用步骤」；硬性规则 3 补充「`internal/common/guard`（中间件）允许出现 Gin」（代码事实要求，规则文本与之一致）；硬性规则 5 补一条例外说明：包级导出只允许「为工具与测试提供的入口」（当前仅 `auth.CreateAdminUser`），业务代码一律走 `*Module`。

- [ ] **Step 2: 提交 README 文档（先提文档，不含版本行变化）**

```bash
git add backend/README.md
git commit -m "docs(backend): 认证层 1 文档（接口/环境变量/迁移工作流）"
```

- [ ] **Step 3: 版本进位**

Run: `cd backend && ./scripts/bump-version.sh minor`（0.1.0 → 0.2.0；脚本同时改写 `version.go` 与 README 顶部的版本行）
```bash
git add backend/internal/version/version.go backend/README.md
git commit -m "chore(backend): 版本进位 0.2.0（认证层 1）"
```

- [ ] **Step 4: 终验清单（spec §10）**

逐条执行并记录输出：容器 healthy + 迁移已应用；`atlas migrate validate`；`gofmt -l . && go vet ./... && go build ./...`；`go test ./...`（带与不带 `DATABASE_URL_TEST` 各一次）；`cmd/create-admin` 建号与重复报错；真机登录 → me → 退出全流程；`/api/v1/healthy` 200 且 `database: ok`。

- [ ] **Step 5: 收尾提交（仅在终验发现问题时）**

若终验发现缺陷：修复后按改动所在层分别提交（`backend/` 与 `frontend/` 分开；只 `git add` 实际改动的文件，禁止 `git add -A`）。若无缺陷，跳过本步。
