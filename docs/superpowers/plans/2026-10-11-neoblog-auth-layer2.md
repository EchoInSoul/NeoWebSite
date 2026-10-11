# NeoBlog 认证层 2 实施计划（注册与更改密码）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在后端补上公开注册（注册即登录）与已登录状态下的更改密码（保留当前会话、吊销其他会话）。

**Architecture:** 全部落在既有的 `internal/modules/auth/` 五件套内：校验规则是模块内的纯函数，`Register` / `ChangePassword` 是 service 方法，两个新路由经由既有的 `Register` / `RegisterProtected` 挂载（`app.go` 不改）。吊销其他会话用 scs 的公开 API（`Store.All()` + `SessionManager.Codec.Decode`）按会话中的 `auth.user_id` 定位；当前会话 token 从请求 cookie `session` 读取。无 schema 改动、无新依赖、无新环境变量。

**Tech Stack:** Go 1.27 + Gin + ent + scs（含 postgresstore）+ bcrypt。

**Spec:** `docs/superpowers/specs/2026-10-11-neoblog-auth-layer2-design.md`

## Global Constraints

- 手写 Go 零注释；`gofmt -l .` 空；`go vet ./...`、`go build ./...` 每任务结束都要过。
- 错误形状 `{"error":"..."}`；非哨兵错误一律映射 500 `internal_error`（层 1 既定）。
- 密码规则：长度按 **rune** ≥8；**ASCII 判定**含至少一个 `[A-Za-z]` 与一个 `[0-9]`；字节数 ≤72（bcrypt 上限，沿用 `maxPasswordBytes`）。
- 昵称规则：trim 后按 **rune** 2–32；不含任何 `unicode.IsSpace`（含全角空格）；不含 `@`。
- 邮箱规则：`net/mail.ParseAddress` 可解析、≤254 字符；入库前 `strings.ToLower` + trim（与层 1 行为一致）。
- 只改 `backend/` 与 `docs/`；**不碰 `frontend/`**（前端由用户自己接）。
- 本机 Git Bash 先执行：`export PATH="/f/software/go/bin:/f/software/gopath/bin:$PATH" GOPATH='F:/software/gopath' GOCACHE='F:/software/gocache' GOPROXY='https://goproxy.cn,direct'`
- 集成测试：`DATABASE_URL_TEST='postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_test?sslmode=disable'`；缺省 `t.Skip`；测试数据带随机后缀并用 `t.Cleanup` 删除。
- 提交信息中文、conventional 前缀（`feat(backend):` / `docs(backend):` / `chore(backend):`）；每个任务一次提交，只 `git add` 本任务文件。
- 版本：本计划完成时 `./scripts/bump-version.sh minor`（0.2.1 → 0.3.0）。

## Review Focus

1. 昵称里的全角空格与不可见空白（`IsSpace` 覆盖全角）——T1 测试。
2. 密码「字母」判定为 ASCII：8 个中文字符 + 一个数字**不满足**规则——T1 测试。
3. `net/mail` 会放行的怪格式（如 `a@b`）按合法接受（与注册页 `type=email` 行为一致）——T1 测试固定该行为。
4. 改密码后**其他设备**的旧 cookie 再访问只得到 401、不会让会话复活——T4 测试。
5. 已登录状态下调用 `register`：成功后会话切换为新账号（`/me` 返回新账号）——T3 测试。

---

### Task 1: 校验规则纯函数（`validate.go`）

**Files:**
- Create: `backend/internal/modules/auth/validate.go`
- Create: `backend/internal/modules/auth/validate_test.go`

**Interfaces:**
- Produces（包内使用，全部纯函数；`validNickname` / `validEmail` 的入参是**已规范化**的值）：
  - `func normalizeEmail(raw string) string` —— `strings.ToLower(strings.TrimSpace(raw))`
  - `func normalizeNickname(raw string) string` —— `strings.TrimSpace(raw)`
  - `func validEmail(email string) bool` —— `net/mail.ParseAddress` 成功且 `len(email) <= 254`
  - `func validNickname(nickname string) bool` —— rune 数 2–32、无 `unicode.IsSpace` 字符（含全角空格）、不含 `@`
  - `func validPassword(password string) bool` —— rune 数 ≥8、`len(password) <= 72`、至少一个 ASCII 字母与一个 ASCII 数字

- [ ] **Step 1: 写失败测试（表驱动）**

```go
func TestValidEmail(t *testing.T) {
	cases := []struct {
		in    string
		valid bool
	}{
		{"a@b.com", true},
		{"a@b", true},
		{"", false},
		{"no-at", false},
		{"a@@b", false},
		{strings.Repeat("a", 250) + "@b.com", false},
	}
	for _, tc := range cases {
		if got := validEmail(tc.in); got != tc.valid {
			t.Fatalf("validEmail(%q) = %v, want %v", tc.in, got, tc.valid)
		}
	}
}

func TestValidNickname(t *testing.T) {
	cases := []struct {
		in    string
		valid bool
	}{
		{"ab", true},
		{"中文名", true},
		{"a", false},
		{strings.Repeat("字", 33), false},
		{"a b", false},
		{"a\u3000b", false},
		{"a@b", false},
	}
	for _, tc := range cases {
		if got := validNickname(tc.in); got != tc.valid {
			t.Fatalf("validNickname(%q) = %v, want %v", tc.in, got, tc.valid)
		}
	}
}

func TestValidPassword(t *testing.T) {
	cases := []struct {
		in    string
		valid bool
	}{
		{"pass1234", true},
		{strings.Repeat("a", 71) + "1", true},
		{"short1", false},
		{"abcdefgh", false},
		{"12345678", false},
		{strings.Repeat("a", 72) + "1", false},
		{"密码密码密码密码1", false},
	}
	for _, tc := range cases {
		if got := validPassword(tc.in); got != tc.valid {
			t.Fatalf("validPassword(%q) = %v, want %v", tc.in, got, tc.valid)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got := normalizeEmail("  A@B.COM "); got != "a@b.com" {
		t.Fatalf("normalizeEmail = %q, want a@b.com", got)
	}
	if got := normalizeNickname("  ab  "); got != "ab" {
		t.Fatalf("normalizeNickname = %q, want ab", got)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/modules/auth/ -count=1 -run 'TestValid|TestNormalize'`
Expected: 编译失败（四个函数未定义）。

- [ ] **Step 3: 实现 `validate.go`**

要点：`net/mail` 的 `ParseAddress`；rune 计数用 `utf8.RuneCountInString` 或 `[]rune`；空白判定 `strings.ContainsFunc(s, unicode.IsSpace)`；字母/数字判定限定 ASCII（`(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')` 与 `c >= '0' && c <= '9'`）；72 字节上限直接复用现有 `maxPasswordBytes`。

- [ ] **Step 4: 跑测试确认通过**

Run: 同 Step 2，Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/auth/validate.go backend/internal/modules/auth/validate_test.go
git commit -m "feat(backend): 注册与改密码的校验规则纯函数"
```

---

### Task 2: repository 更新密码哈希

**Files:**
- Modify: `backend/internal/modules/auth/repository.go`
- Modify: `backend/internal/modules/auth/repository_test.go`

**Interfaces:**
- Consumes: 层 1 的 `repository` / `errUserNotFound` / `createTestUser` 测试助手。
- Produces: `func (r *repository) updatePasswordHash(ctx context.Context, id string, passwordHash string) error` —— 按 UUID 更新 `password_hash`；用户不存在 → `errUserNotFound`；含 `updated_at` 由 ent 的 `UpdateDefault` 自动刷新。

- [ ] **Step 1: 写失败测试**

在 `repository_test.go` 追加：

```go
func TestUpdatePasswordHash(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, oldPassword := createTestUser(t, db, tag)
	repo := newRepository(db)

	newHash, err := bcrypt.GenerateFromPassword([]byte("new-pass-123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := repo.updatePasswordHash(context.Background(), u.ID, string(newHash)); err != nil {
		t.Fatalf("update: %v", err)
	}

	found, err := repo.findUserByID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(found.PasswordHash), []byte("new-pass-123")); err != nil {
		t.Fatalf("new password does not verify: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(found.PasswordHash), []byte(oldPassword)); err == nil {
		t.Fatal("old password still verifies after update")
	}

	missing := uuid.Must(uuid.NewV7()).String()
	if err := repo.updatePasswordHash(context.Background(), missing, string(newHash)); !errors.Is(err, errUserNotFound) {
		t.Fatalf("missing user: err = %v, want errUserNotFound", err)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `DATABASE_URL_TEST=... go test ./internal/modules/auth/ -count=1 -run TestUpdatePasswordHash`
Expected: 编译失败（`updatePasswordHash` 未定义）。

- [ ] **Step 3: 实现**

`uuid.Parse` → `r.client.User.UpdateOneID(parsed).SetPasswordHash(passwordHash).Save(ctx)`；`ent.IsNotFound` → `errUserNotFound`。

- [ ] **Step 4: 跑测试确认通过**

Run: 同 Step 2，Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/auth/repository.go backend/internal/modules/auth/repository_test.go
git commit -m "feat(backend): repository 支持更新密码哈希"
```

---

### Task 3: 注册（service + controller + module 路由）

**Files:**
- Modify: `backend/internal/modules/auth/service.go`（新增哨兵错误与 `Register`）
- Modify: `backend/internal/modules/auth/dto.go`（新增 `RegisterRequest`）
- Modify: `backend/internal/modules/auth/controller.go`（新增 `c.signup` handler，并在 `controller.register` 里声明路由）
- Modify: `backend/internal/modules/auth/auth_test.go`（HTTP 级测试）

**Interfaces:**
- Consumes: T1 的 `normalizeEmail` / `validEmail` / `validNickname` / `validPassword`；层 1 的 `repository.createUser`、`findUserByIdentifier`、`toUser`、`guard.SessionUserIDKey`、`maxPasswordBytes`。
- Produces:
  - `type RegisterRequest struct { Email string \`json:"email"\`; Nickname string \`json:"nickname"\`; Password string \`json:"password"\` }`（前端注册页的 `name` 字段映射到这里；`confirmPassword` 由前端自行校验，不进请求体）
  - 哨兵错误（与既有 `ErrInvalidCredentials` / `ErrUnauthorized` 同处）：`ErrInvalidRequest`、`ErrEmailTaken`、`ErrNicknameTaken`、`ErrAlreadyTaken`、`ErrWeakPassword`
  - `func (s *Service) Register(ctx context.Context, email, nickname, password string) (User, error)`
  - 路由 `POST /api/v1/auth/register`

- [ ] **Step 1: 写失败测试（HTTP 级，沿用 `newTestApp` / `postJSON` / `testClient` 助手）**

用例（各自一个测试函数或子测试）：
1. `TestRegisterSucceedsAndLogsIn`：合法注册 → 201、响应体 `{"email","nickname"}` 且 email 为小写、带 `session` cookie；随后 `/auth/me` 200 且返回同一账号。子用例：昵称两边带空格（`  ab  `）→ 201 且响应与库中均为 trim 后的 `ab`。
2. `TestRegisterRejectsDuplicateEmail`：同邮箱（其中一个为大写变体、换不同昵称）→ 第二次 409 `email_taken`。
3. `TestRegisterRejectsDuplicateNickname`：同昵称、换不同邮箱 → 409 `nickname_taken`。
4. `TestRegisterRejectsWeakPasswords`：子测试覆盖 `<8 位`、`纯字母`、`纯数字`、`>72 字节` → 400 `weak_password`。
5. `TestRegisterRejectsInvalidFields`：子测试覆盖 `邮箱缺 @`、`邮箱超长`、`昵称 1 字符`、`昵称含 @`、`昵称含空格` → 400 `invalid_request`。
6. `TestRegisterRejectsMalformedBody`：`{not json` 与缺 `password` 字段 → 400 `invalid_request`。
7. `TestRegisterWhileLoggedInSwitchesSession`：client 先注册账号 A；同一个 client 再注册账号 B → 201，随后 `/auth/me` 返回账号 B。

- [ ] **Step 2: 跑测试确认失败**

Run: `DATABASE_URL_TEST=... go test ./internal/modules/auth/ -count=1 -run TestRegister`
Expected: 编译失败（`Register` / 路由未定义）。

- [ ] **Step 3: 实现**

- `service.go`：`Register` 流程 = `normalizeEmail` + `normalizeNickname` → `validEmail`/`validNickname`（失败 → `ErrInvalidRequest` → 400 `invalid_request`）→ `validPassword`（失败 → `ErrWeakPassword`）→ 唯一性预查：**先用规范化后的 email 调 `findUserByIdentifier` 一次**（命中 → `ErrEmailTaken`）、**再用 trim 后的 nickname 调一次**（命中 → `ErrNicknameTaken`）——昵称不含 `@`，两次查询不会互相误报 → bcrypt → `createUser`（`ent.IsConstraintError` → `ErrAlreadyTaken`）→ `RenewToken` + `Put(userID)`。
- `controller.go`：新增 `c.signup` handler（**不要叫 `register`**——`controller.register` 已是「声明路由」的方法名，同名会编译冲突）——`ShouldBindJSON` 失败/空字段 → 400；按哨兵错误映射 400（`weak_password` / `invalid_request`）、409（`email_taken` / `nickname_taken` / `already_taken`）→ 其余 500；成功 201 + `UserResponse`。路由声明加进既有的 `controller.register`：`r.POST("/auth/register", c.signup)`。
- `module.go`：无需改动（`Register` 已委托 controller 声明路由）。

- [ ] **Step 4: 跑测试确认通过**

Run: 同 Step 2，Expected: PASS。再跑 `go test ./internal/modules/auth/ -count=1`（全部用例）Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/auth/service.go backend/internal/modules/auth/dto.go backend/internal/modules/auth/controller.go backend/internal/modules/auth/auth_test.go
git commit -m "feat(backend): 公开注册（注册即登录）"
```

---

### Task 4: 更改密码 + 吊销其他会话

**Files:**
- Modify: `backend/internal/modules/auth/service.go`（`ChangePassword` / `revokeOtherSessions` / `ErrPasswordUnchanged`）
- Modify: `backend/internal/modules/auth/dto.go`（新增 `PasswordChangeRequest`）
- Modify: `backend/internal/modules/auth/controller.go`（新增 `c.changePassword` 与路由声明）
- Modify: `backend/internal/modules/auth/auth_test.go`（HTTP 级测试）
- Modify: `backend/internal/app/app_test.go`（真 postgresstore 的吊销与坏行容错）

**Interfaces:**
- Consumes: T2 的 `updatePasswordHash`；层 1 的 `findUserByID`、`toUser`、`guard.SessionUserIDKey`、`ErrInvalidCredentials`、`ErrUnauthorized`；scs 的 `scs.IterableStore`（`All()`，由 `SessionManager.Store` 类型断言取得——`scs.Store` 本身只有 `Delete/Find/Commit`）与 `SessionManager.Codec.Decode`（签名 `(time.Time, map[string]interface{}, error)`）。
- Produces:
  - 哨兵错误 `ErrPasswordUnchanged`
  - `type PasswordChangeRequest struct { CurrentPassword string \`json:"currentPassword"\`; NewPassword string \`json:"newPassword"\` }`
  - `func (s *Service) ChangePassword(ctx context.Context, userID, currentToken, currentPassword, newPassword string) error`
  - `func (s *Service) revokeOtherSessions(ctx context.Context, userID, currentToken string) error`（包内方法；`All()` 失败或**命中行的 `Delete` 失败**都返回错误；只有「解码失败、无法判定归属」的行跳过）
  - 路由 `POST /api/v1/auth/password`（受保护，声明在 `controller.registerProtected`）

- [ ] **Step 1: 写失败测试**

auth_test.go（memstore）：
1. `TestChangePasswordSucceedsAndSwapsCredentials`：登录 → 改密码 204 → 旧密码登录 401、新密码登录 200。
2. `TestChangePasswordRejectsBadInput`：子测试——旧密码错 → 401 `invalid_credentials`；新密码弱 → 400 `weak_password`；新旧相同 → 400 `password_unchanged`；未登录 → 401 `unauthorized`。
2b. `TestChangePasswordOnDeletedUserReturnsUnauthorized`：登录后删除该用户行（`db.Client.User.DeleteOneID(...).Exec`）→ 改密码 → 401 `unauthorized`，且响应带清除会话的 `Set-Cookie`（会话被销毁）。
3. `TestChangePasswordRevokesOtherSessions`：client A、B 各登录同一账号 → A 改密码 → A 的 `/auth/me` 仍 200、**B 的 `/auth/me` 401**，且 B 再请求仍是 401（不复活）。
4. `TestChangePasswordRejectsMalformedBody`：`{not json` 与缺 `newPassword` → 400 `invalid_request`。

app_test.go（真 postgresstore）：
5. `TestChangePasswordToleratesBadSessionRows`：经 `db.SQL().Exec("INSERT INTO sessions (token, data, expiry) VALUES ($1, $2, now() + interval '1 hour')", "junk-<随机>", []byte{0})` 插入一行坏数据；然后登录 → 改密码 204（吊销过程不因坏行中断）→ 清理该行。

- [ ] **Step 2: 跑测试确认失败**

Run: `DATABASE_URL_TEST=... go test ./internal/modules/auth/ ./internal/app/ -count=1 -run 'TestChangePassword'`
Expected: 编译失败（`ChangePassword` / 路由未定义）。

- [ ] **Step 3: 实现**

- `service.go`：`ChangePassword` = `findUserByID`（`errUserNotFound` → 销毁会话 + `ErrUnauthorized`）→ `validPassword(newPassword)` 失败 → `ErrWeakPassword` → `bcrypt.CompareHashAndPassword(旧哈希, currentPassword)` 失败 → `ErrInvalidCredentials` → `bcrypt.CompareHashAndPassword(旧哈希, newPassword)` 成功 → `ErrPasswordUnchanged` → **先 `revokeOtherSessions`（失败 → 返回错误 → 500；此时密码未改，用旧密码重试即可，重试时吊销幂等）→ 再生成新哈希并 `updatePasswordHash`（失败 → 500；其他会话已吊销、密码未改，旧密码重试仍有效）**。
- `revokeOtherSessions`：`its, ok := s.sessions.Store.(scs.IterableStore)`（`!ok` → 返回错误）→ `its.All()`（失败 → 返回错误）→ 遍历 `token → data`：`token == currentToken` 跳过；`_, values, err := s.sessions.Codec.Decode(data)` 失败的行**跳过并继续**（无法判定归属）；`values[guard.SessionUserIDKey]` 断言为 `string` 且等于 userID → `its.Delete(token)`，**Delete 失败直接返回错误**（不静默跳过——否则会 204 谎报已吊销）。
- `controller.go`：新增 `c.changePassword` handler —— 从 `ctx.GetString(guard.SessionUserIDKey)` 取 userID、从 `ctx.Cookie("session")` 取当前 token（取不到 → 401 `unauthorized`，正常路径下 guard 已保证有会话）；映射 401（`unauthorized` / `invalid_credentials`）、400（`weak_password` / `password_unchanged` / `invalid_request`）→ 成功 204。**路由声明加在既有的 `controller.register` / `registerProtected` 里**（与层 1 相同；`module.go` 无需改动）。
- `module.go`：无需改动（`Register` / `RegisterProtected` 已委托给 controller 的路由声明）。

- [ ] **Step 4: 跑测试确认通过**

Run: 同 Step 2，Expected: PASS；再全量 `go test ./... -count=1`（带 `DATABASE_URL_TEST`）Expected: 全绿。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/auth/service.go backend/internal/modules/auth/dto.go backend/internal/modules/auth/controller.go backend/internal/modules/auth/auth_test.go backend/internal/app/app_test.go
git commit -m "feat(backend): 更改密码（保留当前会话，吊销其他会话）"
```

---

### Task 5: 文档、版本进位与终验

**Files:**
- Modify: `backend/README.md`
- Modify: `docs/DEVLOG.md`（文件已存在，顶部追加一条）
- Modify: `backend/internal/version/version.go`（脚本改）

**Interfaces:**
- Produces: 文档与实现一致；spec §6 六条验收全部跑过。

- [ ] **Step 1: 更新 README 与 DEVLOG**

README：认证章节接口表补 `POST /auth/register`（公开，201；错误码全列）与 `POST /auth/password`（需登录，204）；补「公开注册、不验证邮箱」「昵称/密码规则」「改密码保留当前会话、吊销其他会话」；已知缺口里注明「登录/注册暂无限流（下一轮优先补；部署时需配置真实代理 CIDR 才能按 IP 限流）」。
DEVLOG：文件顶部追加 `## 2026-10-11 · 认证层 2（注册与更改密码）` 条目，格式对齐既有条目（做了什么 / 已知缺口 / 关键决定）。

- [ ] **Step 2: 提交文档**

```bash
git add backend/README.md docs/DEVLOG.md
git commit -m "docs(backend): 认证层 2 文档（注册/改密码接口与规则）"
```

- [ ] **Step 3: 版本进位**

Run: `cd backend && ./scripts/bump-version.sh minor`（0.2.1 → 0.3.0）
```bash
git add backend/internal/version/version.go backend/README.md
git commit -m "chore(backend): 版本进位 0.3.0（认证层 2）"
```

- [ ] **Step 4: 终验清单（spec §6）**

逐条执行并记录输出：
1. 真机注册 → 201 + cookie → `/auth/me` 返回本人；库里该行 email 为小写。
2. 重复邮箱 / 昵称 → 409（错误码各自正确）；弱密码 / 格式错 → 400（错误码正确）。
3. 改密码 204；旧密码登录 401、新密码 200。
4. 双设备吊销：B 401、A 仍 200。
5. 层 1 回归（登录/登出/me/cookie 属性/删号吊销）全绿。
6. `go test ./...` 带库与无库各一遍；`gofmt -l .` 空；`go vet ./...`、`go build ./...` 干净。

- [ ] **Step 5: 收尾提交（仅在终验发现问题时）**

修复后按文件归属提交；无缺陷则跳过。
