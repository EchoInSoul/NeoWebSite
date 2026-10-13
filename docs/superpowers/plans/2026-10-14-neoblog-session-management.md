# 会话管理实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans（本机可用；本轮执行方式按项目先例「我全程自己实现 + 末尾一次全新审查」）。

> **路径口径（0.16.0 起）：** `/auth/*` 已随 0.16.0 路由改名取消；本文原写的 `GET /auth/me` 已同步为 `GET /users/me`（见 DEVLOG 2026-10-14「路由改名」条目与 `backend/README.md` 接口表），其余路径未变，冒烟配方照跑即可。

**Goal:** 让登录用户能列出自己的活跃会话（设备 / IP / 建立与活跃时间）并按指纹吊销任意一条。

**Architecture:** 不建登记表——scs 的 `sessions` 表即会话本体。元数据以四个会话数据键随登录写入（零 DDL）；节流 5 分钟的 `TrackSessionActivity` 中间件维护 `last_seen_at`；两个接口走 `All() + Codec.Decode` 既有枚举手法。

**Tech Stack:** Go + gin + scs（postgresstore）+ ent；`internal/common/guard`（中间件）与 `internal/modules/auth`（模块）。

**Spec:** `docs/superpowers/specs/2026-10-14-neoblog-session-management-design.md`

## Global Constraints

- **零注释**：产品代码与测试代码都不写注释（工具链生成除外）。
- **零迁移**：`backend/migrations/` 与 `atlas.sum` 一字节不动。
- **不碰 `frontend/`**：本轮不新增、不修改任何前端文件。
- **gob 类型纪律**：会话数据只存内建类型（本轮四个元数据键一律 RFC3339 UTC 字符串）；不引入 `gob.Register` 副作用。
- **集成测试**：`DATABASE_URL_TEST` 未设置时用例 `t.Skip`；跑法 `cd backend && set -a; source .env; set +a && bash scripts/test.sh ./internal/...`。
- **门禁**：`go build ./... && go vet ./...`；格式判定写 `test -z "$(gofmt -l .)"`。
- **提交口径**：按文件清单逐个 `git add`，禁止 `git add -A`。
- **旧会话兼容**：本轮之前建立的会话没有元数据键 → 列表字段为空串，不报错、不隐藏。

## Review Focus

1. **超长 / 畸形请求头**：UA 或 `X-Forwarded-For` 塞到几十 KB（`MaxHeaderBytes` 默认 1MB）→ 必须截断（UA 512 / IP 64），不得把会话行撑大。→ Task 2 `TestLoginTruncatesOversizedHeaders`
2. **待验态污染**：只有 `auth.pending_2fa_user_id` 的会话不得出现在任何人的列表里、也不得带旧身份的元数据。→ Task 3 `TestSessionsListExcludesPendingSessions`
3. **他人会话的 id 与不存在的 id 响应同形**：都 404 `not_found`，且被瞄的会话仍可用（不构成存在性探测、不误杀）。→ Task 4 `TestSessionsRevokeUnknownOrForeignReturnsNotFound`
4. **每请求写库**：`last_seen_at` 距上次写入 <5 分钟时，`sessions` 行必须字节不变。→ Task 2 `TestSessionActivityTouchThrottled`
5. **已登录会话换身份**：同一会话再登录/注册时必须清 pending 两键、元数据整体覆盖为本次身份。→ Task 2 `TestEstablishSessionClearsPendingKeys`
6. **id 不可回推 token**：列表返回的 `id` 不得等于会话 cookie 值，也不得由它算出 token。→ Task 3 `TestSessionsListIncludesCurrentWithMetadata`

---

### Task 1: guard 会话元数据键 + 节流中间件

**Files:**
- Create: `backend/internal/common/guard/activity.go`
- Create: `backend/internal/common/guard/activity_test.go`
- Modify: `backend/internal/app/app.go`（protected 分组，`:70`）
- Modify: `backend/internal/modules/auth/auth_test.go`（`newTestAppWithSessionsAndSettings` 的 protected 分组，`:52`）

**Interfaces:**
- Produces: `guard.SessionUserAgentKey` / `guard.SessionIPKey` / `guard.SessionCreatedAtKey` / `guard.SessionLastSeenAtKey`（值 `auth.user_agent` / `auth.ip` / `auth.created_at` / `auth.last_seen_at`）；`func TrackSessionActivity(sm *scs.SessionManager) gin.HandlerFunc`。

- [ ] **Step 1: 写失败测试** —— `activity_test.go`，照 `guard_test.go` 既有装配（`scs.New()` + `sm.LoadAndSave(engine)`）。四条用例，探针 handler 回显 `sm.GetString(ctx, SessionLastSeenAtKey)`：

```go
func TestTrackSessionActivityWritesWhenStale(t *testing.T)   // 种 user_id + last_seen=now-6min → 探针返回值 ≠ 旧值，且解析后距现在 <1min
func TestTrackSessionActivitySkipsWhenFresh(t *testing.T)    // 种 user_id + last_seen=now-1min（仍新鲜；必须与任何新写入的字符串不同，否则「每次都写」的回归照样绿）→ 探针返回值与种下的字符串完全相等
func TestTrackSessionActivityWritesWhenMissing(t *testing.T) // 种 user_id、不种 last_seen → 探针返回非空
func TestTrackSessionActivityIgnoresAnonymous(t *testing.T)  // 只种 last_seen、不种 user_id → 探针返回原值不变
```

- [ ] **Step 2: 跑测试确认失败** —— `go test ./internal/common/guard/ -run TestTrackSessionActivity -count=1`，期望 `undefined: TrackSessionActivity`。

- [ ] **Step 3: 实现** —— `activity.go`：四个键常量（与 `guard.go` 的 `SessionUserIDKey` 同块语义，另立文件）；`const sessionActivityThrottle = 5 * time.Minute`；中间件逻辑：`user_id` 非空时读 `last_seen_at`，`time.Parse(time.RFC3339, …)` 失败或 `time.Since(seen) >= sessionActivityThrottle` 才 `sm.Put(ctx, SessionLastSeenAtKey, time.Now().UTC().Format(time.RFC3339))`，随后 `c.Next()`。

- [ ] **Step 4: 跑测试确认通过** —— 同一命令，四条全绿。

- [ ] **Step 5: 挂载** —— `app.go` 的 `protected` 分组在 `guard.RequireAuth(sessions)` 之后、`guard.LoadIdentity(...)` 之前插入 `guard.TrackSessionActivity(sessions)`；`auth_test.go` 的测试装配做同样插入（保持与生产装配一致，Task 2 的节流用例依赖它）。

- [ ] **Step 6: 门禁 + 提交**

```bash
cd backend && test -z "$(gofmt -l .)" && go vet ./... && go build ./...
git add internal/common/guard/activity.go internal/common/guard/activity_test.go internal/app/app.go internal/modules/auth/auth_test.go
git commit -m "feat(backend): 会话活跃追踪——guard 会话元数据键与 5 分钟节流中间件"
```

### Task 2: 登录 / 注册 / 二段验证落元数据

**Files:**
- Create: `backend/internal/modules/auth/session.go`
- Modify: `backend/internal/modules/auth/service.go`（`Register` `:90-93`、`Login` `:116-126`、`VerifySecondFactor` `:448-453`——均为各自成功分支里 `RenewToken` 到 `Put(user_id)` 的连续几行）
- Modify: `backend/internal/modules/auth/controller.go`（三个调用点 `:44` / `:60` / `:227` 传 meta；`requestSessionMeta` 需新 import `strings`）
- Test: `backend/internal/modules/auth/sessions_test.go`（新建；本轮后续任务的用例都放这里）

**Interfaces:**
- Consumes: Task 1 的四个键常量。
- Produces: `type SessionMeta struct { UserAgent string; IP string }`；`func (s *Service) establishSession(ctx context.Context, userID string, meta SessionMeta) error`；`func (s *Service) enterPendingSession(ctx context.Context, userID string)`；签名变更 `Register(ctx, email, nickname, password string, meta SessionMeta)` / `Login(ctx, identifier, password string, meta SessionMeta)` / `VerifySecondFactor(ctx, code string, meta SessionMeta)`。

- [ ] **Step 1: 写失败测试** —— `sessions_test.go` 先落两个测试助手与三条用例。助手：`sessionRow(t, sm, tsURL, client) (token string, deadline time.Time, values map[string]any)`（cookie 取 token → `sm.Store.Find` → `sm.Codec.Decode`）与 `rewriteSessionRow(t, sm, token, values, deadline)`（`sm.Codec.Encode` + `sm.Store.Commit`）。

```go
func TestLoginStoresSessionMetadata(t *testing.T)      // 请求带 "User-Agent: probe/1.0" 与 "X-Forwarded-For: 203.0.113.7, 10.0.0.1" → 行内四键齐全：UA 原样、ip=203.0.113.7、created_at≈now、last_seen_at==created_at
func TestLoginTruncatesOversizedHeaders(t *testing.T)   // UA 送 2000 字节、XFF 送 200 字节 → 行内 UA 长度 512、ip 长度 64（Review Focus 1）
func TestEstablishSessionClearsPendingKeys(t *testing.T) // 造行 {pending_2fa_user_id: X} 并让客户端持该 token → 正常登录（另一账号）→ 新行内 pending 两键消失、user_id/元数据为本次身份（Review Focus 5）
```

- [ ] **Step 2: 跑测试确认失败** —— `go test ./internal/modules/auth/ -run 'TestLoginStoresSessionMetadata|TestEstablishSessionClearsPendingKeys' -count=1`，期望行内取不到元数据键。

- [ ] **Step 3: 实现 `session.go`** —— `SessionMeta`、四个键（引用 guard 常量）、`establishSession`（`RenewToken` → `Put(user_id)` → `Put` 四元数据（`created_at` 与 `last_seen_at` 同值）→ `Remove` pending 两键 → 返回 error）、`enterPendingSession`（`Remove` user_id + 四元数据键 → `Put(pendingUserIDKey)`）。

- [ ] **Step 4: 接线 service 与 controller** —— `Register` 成功的 `RenewToken + Put(user_id)` 两行换成 `establishSession(ctx, created.ID.String(), meta)`；`Login` 的无 2FA 分支同上、2FA 分支的 `Remove(user_id) + Put(pending)` 两行换成 `enterPendingSession(ctx, found.ID.String())`（`RenewToken` 保持在该分支之前调用）；verify 成功分支的 `RenewToken + Put(user_id) + Remove(pending) + Remove(attempts)` 四行换成 `establishSession(ctx, found.ID.String(), meta)`（pending 两键的清理已并入助手）。controller 加 `requestSessionMeta(c *gin.Context) SessionMeta`（XFF 首跳到逗号 → trim → `X-Real-IP` → `c.ClientIP()`；UA/IP 各按 512/64 截断）并在三个调用点传入。

- [ ] **Step 5: 补节流 e2e 用例并跑全绿** —— 追加 `TestSessionActivityTouchThrottled`（Review Focus 4）：登录先断言 `last_seen_at` 已写入；用 `rewriteSessionRow` 把 `last_seen_at` 改成 **1 分钟前**并以此为基准 → 立即 `GET /users/me` → 该值逐字不变；再把 `last_seen_at` 改成 6 分钟前 → 再 `GET /users/me` → 行内 `last_seen_at` 变成近期。命令：`go test ./internal/modules/auth/ -count=1`。

- [ ] **Step 6: 门禁 + 提交**

```bash
cd backend && test -z "$(gofmt -l .)" && go vet ./... && go build ./...
git add internal/modules/auth/session.go internal/modules/auth/service.go internal/modules/auth/controller.go internal/modules/auth/sessions_test.go
git commit -m "feat(backend): 会话元数据随建立落盘——UA/IP/建立时间/活跃时间 + 清 pending 残留"
```

### Task 3: 会话列表接口

**Files:**
- Modify: `backend/internal/modules/auth/session.go`（+`Session` 类型、`eachSession`、`sessionFingerprint`、`ListSessions`）
- Modify: `backend/internal/modules/auth/dto.go`（+`SessionResponse`）
- Modify: `backend/internal/modules/auth/controller.go`（`registerProtected` `:26` 内加路由；+`itemList[T]` 与 handler）
- Test: `backend/internal/modules/auth/sessions_test.go`

**Interfaces:**
- Produces: `type Session struct { ID string; Current bool; UserAgent, IP, CreatedAt, LastSeenAt, ExpiresAt string }`；`func (s *Service) ListSessions(ctx context.Context, userID string) ([]Session, error)`；`sessionFingerprint(token string) string`（`base64.RawURLEncoding(sha256(token))`）；`type SessionResponse`（json: `id/current/user_agent/ip/created_at/last_seen_at/expires_at`）。路由 `GET /users/me/sessions`，响应 `{"items":[SessionResponse…]}`。

- [ ] **Step 1: 写失败测试**

```go
func TestSessionsListIncludesCurrentWithMetadata(t *testing.T)  // 200；恰 1 条；current=true；UA/IP/三时间戳齐全；id ≠ sessionCookie 的 token（Review Focus 6）
func TestSessionsListIsolatedPerUser(t *testing.T)              // 两用户两客户端 → 各自 1 条且 id 不同
func TestSessionsListExcludesPendingSessions(t *testing.T)      // 造 {pending_2fa_user_id: 本用户} 的行 → 列表仍恰 1 条（Review Focus 2）
func TestSessionsListRequiresAuth(t *testing.T)                 // 无 cookie → 401 unauthorized
```

- [ ] **Step 2: 跑测试确认失败** —— `go test ./internal/modules/auth/ -run TestSessionsList -count=1`，期望 404（路由未注册）。

- [ ] **Step 3: 实现服务侧** —— `eachSession(fn func(token string, values map[string]any, deadline time.Time) error) error`（`Store.(scs.IterableStore).All()` → 逐条 `Codec.Decode`，解码失败跳过；不支持迭代的 store 返回既有错误文案）；`sessionFingerprint`；`ListSessions`：过滤 `values[guard.SessionUserIDKey] == userID` → 组装（`current = fingerprint(token) == fingerprint(s.sessions.Token(ctx))`，`ExpiresAt = deadline.UTC().Format(time.RFC3339)`）→ `sort.Slice`：`LastSeenAt` 降序、并列 `ID` 升序。

- [ ] **Step 4: 实现 controller 侧** —— `itemList[T]`（照 `access/controller.go:14` 同形）；`registerProtected` 里加 `r.GET("/users/me/sessions", c.listSessions)`；handler 从 `ctx.GetString(guard.SessionUserIDKey)` 取 userID → 调服务 → 映射 `SessionResponse` → `ctx.JSON(http.StatusOK, itemList[SessionResponse]{Items: items})`。

- [ ] **Step 5: 跑测试确认通过** —— 同 Step 2 命令，全绿。

- [ ] **Step 6: 门禁 + 提交**

```bash
cd backend && test -z "$(gofmt -l .)" && go vet ./... && go build ./...
git add internal/modules/auth/session.go internal/modules/auth/dto.go internal/modules/auth/controller.go internal/modules/auth/sessions_test.go
git commit -m "feat(backend): GET /users/me/sessions——列出自己的活跃会话（指纹 id + 当前徽标 + 元数据）"
```

### Task 4: 单条吊销接口

**Files:**
- Modify: `backend/internal/modules/auth/session.go`（+`RevokeSession`）
- Modify: `backend/internal/modules/auth/controller.go`（`registerProtected` `:26` 内加路由 + handler）
- Test: `backend/internal/modules/auth/sessions_test.go`

**Interfaces:**
- Produces: `func (s *Service) RevokeSession(ctx context.Context, userID, sessionID string) error`（命中即 `Store.Delete` 返回 nil；否则 `apierror.ErrNotFound`）。路由 `DELETE /users/me/sessions/:id` → 200 `{"id":"…","deleted":true}`。

- [ ] **Step 1: 写失败测试**

```go
func TestSessionsRevokeAnotherDevice(t *testing.T)                  // A/B 两客户端：A 列表 2 条 → A 删 B 的 id → 200 deleted；B 打 /users/me 变 401；A 仍 200 且列表回到 1 条
func TestSessionsRevokeUnknownOrForeignReturnsNotFound(t *testing.T) // 三人称 id / 乱造 id / 同 id 二次删 → 均 404 not_found；被瞄会话仍 200（Review Focus 3）
func TestSessionsRevokeCurrentEndsSession(t *testing.T)              // 删自己当前那条 → 200；随后同客户端 401。**关键**：`last_seen_at` 的改写必须紧邻 DELETE 之前（先列表取 id，再改写，中间不得再发请求），否则覆盖不到「本请求节流写 + 删行」的复活路径
func TestSessionsRevokeRequiresAuth(t *testing.T)                    // 无 cookie → 401
```

- [ ] **Step 2: 跑测试确认失败** —— `go test ./internal/modules/auth/ -run TestSessionsRevoke -count=1`，期望 404（路由未注册）。

- [ ] **Step 3: 实现** —— `RevokeSession`：`eachSession` 过滤 `user_id` 匹配且 `fingerprint(token) == sessionID` 的那条 → `Store.Delete`；遍历结束仍未命中 → `apierror.ErrNotFound`。controller：`r.DELETE("/users/me/sessions/:id", c.revokeSession)`；handler 出错走 `apierror.Write`，成功 `ctx.JSON(http.StatusOK, gin.H{"id": ctx.Param("id"), "deleted": true})`（照 `access/controller.go:122` 形状）。

- [ ] **Step 4: 跑测试确认通过** —— 同 Step 2 命令 + 整包 `go test ./internal/modules/auth/ -count=1`。

- [ ] **Step 5: 门禁 + 提交**

```bash
cd backend && test -z "$(gofmt -l .)" && go vet ./... && go build ./...
git add internal/modules/auth/session.go internal/modules/auth/controller.go internal/modules/auth/sessions_test.go
git commit -m "feat(backend): DELETE /users/me/sessions/:id——按指纹吊销单条会话（他人/未知一律 404）"
```

### Task 5: 活体冒烟（另起端口，不碰在跑的进程）

**Files:** 无代码改动；证据写入 Task 6 的 DEVLOG。

- [ ] **Step 1: 构建并另起实例** —— `cd backend && go build -o bin/neoblog-smoke .`；`set -a; source .env; set +a` 后用 `ADDR=:5733 APP_ENV=development ./bin/neoblog-smoke` 另起进程（地址只有一个口子 `ADDR`，默认 `:5732`；**没有 `PORT` 这个变量**），确认启动日志与 `/api/v1/healthy` 200。**不动 5732 上可能在跑的并行会话进程。**
- [ ] **Step 2: curl 全链路** —— 用开发库既有冒烟账号（`gate-check@example.com`）。① 带 `X-Forwarded-For: 198.51.100.9` 与自定义 UA 登录，存 jar1 → ② `GET /users/me/sessions`：1 条、`current=true`、`ip=198.51.100.9`、`id` 与 cookie 值不同 → ③ jar2 再登录一次 → jar1 列表 2 条 → ④ jar1 `DELETE` jar2 那条 id → 200；jar2 `/users/me` 401；jar1 `/users/me` 200、列表 1 条 → ⑤ 乱造 id 删除 → 404 → ⑥ 无 cookie 两个接口 → 401。
- [ ] **Step 3: 收尾** —— 停掉本轮起的进程；两条会话都要清干净（jar2 那条由 §Step 2 的 DELETE 删除、jar1 那条由登出 `Destroy` 删除），**并断言残留为零**（对 jar1 再打一次受保护接口应为 401，或直接查库确认 `sessions` 表无本轮 token）。开发库没有 psql 入口，查库要么走 `.scratch` + pgx，要么就用上面这条接口断言。

### Task 6: 文档收口 + 版本

**Files:**
- Modify: `backend/README.md`（`:3` 版本、认证章节规则、端点表、已知缺口）
- Modify: `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md`（端点表追加两行，标 0.15.0）
- Modify: `docs/DEVLOG.md`（倒序插开头）
- Modify: `backend/internal/version/version.go`（走 `scripts/bump-version.sh minor`）

- [ ] **Step 1: 版本进位** —— `cd backend && bash scripts/bump-version.sh minor`（0.14.0 → 0.15.0）；README `:3` 同步。
- [ ] **Step 2: README** —— 认证规则区补：会话元数据四键与节流的 5 分钟口径、`X-Forwarded-For` 仅供展示（与既有「代理/限流」缺口段呼应）、两个新端点的形为；端点表加两行；「已知缺口」补 `HashTokenInStore`（原始 token 落库）与「会话历史不做」。
- [ ] **Step 3: API 规范表** —— 追加 25/26 两行：`GET /api/v1/users/me/sessions`、`DELETE /api/v1/users/me/sessions/:id`，标注「0.15.0 新增，挂 `RequireAuth + LoadIdentity`、不挂权限点；id 为会话指纹，token 永不出服务端」。
- [ ] **Step 4: DEVLOG** —— 倒序插开头：本轮做了什么（四键 / 中间件 / 两接口 / 测试数）、过程中确认的关键事实（`Put` 不改 deadline ⇒ 非滑动续期；postgresstore 自带 5 分钟清理；gob 只吃内建类型 ⇒ 时间戳存字符串）、活体冒烟结果、顺带闭合的 2FA pending 缺口、已知取舍（全表扫描代价、伪造 XFF、token 明文落库、无历史与分页）。
- [ ] **Step 5: 门禁 + 提交**

```bash
cd backend && test -z "$(gofmt -l .)" && go vet ./... && go build ./...
git add README.md internal/version/version.go ../docs/DEVLOG.md ../docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md
git commit -m "docs(backend): 会话管理收口——README/API 规范表/DEVLOG 与版本 0.15.0"
```

### Task 7: 独立终审（发现先汇报，不擅自修）

- [ ] 派一个全新子代理按 spec 复核本轮 diff（含测试有效性、Review Focus 六条、与 README/规范表一致性），把发现与我的处置建议整理汇报给用户；**未经点名不改代码、不提交**。
