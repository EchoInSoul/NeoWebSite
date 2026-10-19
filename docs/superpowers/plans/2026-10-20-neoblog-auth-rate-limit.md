# 鉴权入口限流 Implementation Plan

> **For agentic workers:** Execute task-by-task with checkbox tracking. This environment has no `subagent-driven-development` / `executing-plans` skills; execution is native in this session after the user approves the plan.

**Goal:** 给登录、注册、2FA verify 三个公开入口加账号/标识与 IP 双维度限流，超限返回 429 + `Retry-After`；前端 sonner 只提示「请求过于频繁」，按钮暂时禁用并显示剩余时间。

**Architecture:** 新增 `internal/common/ratelimit` 进程内限流器（固定窗口 + 失败冷却），由 `app.New` 构造并注入 `auth.Service`；auth 层组合账号/IP 键，`apierror` 统一写出 429 与 `Retry-After`；`requestSessionMeta` 改用可信代理下的 `ClientIP()`；前端 `ApiError` 读取 `Retry-After`，429 sonner 只提示「请求过于频繁」、不显示等待时间，按钮文字显示剩余时间。零迁移、零新依赖、单实例。

**修订（实施后按用户要求）：** 429 前端 sonner 只显示「请求过于频繁」，不显示等待时间；提交按钮文字显示剩余时间；不保留表单内联提示。`formatRetryDuration` 保留用于按钮标签，`retryIn` 保留为按钮文案；版本定为 patch 0.21.2。下文 Task 7/8 中与内联倒计时相关的旧描述以本修订为准。

**Tech Stack:** Go + Gin + scs + ent；Next.js 16 + React 19 + next-intl + sonner；测试走 `scripts/test.sh`（`-p 1` 串行）。

**Spec:** `docs/superpowers/specs/2026-10-20-neoblog-auth-rate-limit-design.md`（本计划按该 spec 的 §3–§12 展开；两份一起读）

## Global Constraints

- **顺序**：Task 1（限流器）→ Task 2（apierror）→ Task 3（可信代理）→ Task 4（auth 注入 + 登录）→ Task 5（注册）→ Task 6（2FA verify）→ Task 7（前端登录）→ Task 8（前端注册）→ Task 9（文档/版本/全量验证）。Task 4 依赖 1、2、3；Task 5/6 依赖 4；Task 8 依赖 7；Task 9 依赖全部。
- **零迁移、零新依赖、零新路由**；版本按执行时现读 `backend/internal/version/version.go` 的下一档（本轮为 0.21.1 → patch 0.21.2；若并行轮已到 0.21.2，保持该值，不重复进位）。
- **不主动提交**：全部改动留在工作区；提交与否、批次由用户点名。禁止 `git add -A`；若提交，按文件清单逐个 `git add`。
- **开工预检**：`git status --short`。当前工作区只有本 spec/plan 两个未跟踪文件（并行侧栏改动已在 `442a7b3` 入库）；本轮目标文件若带非本轮未提交改动，停下报告，不硬改、不覆盖。
- **本机测试约束**：`export PATH="/f/software/go/bin:$PATH"`、`cd F:/protect/NeoBlog/backend`、`set -a; source .env; set +a` 导入 `DATABASE_URL_TEST`；一次只跑一个测试进程；**不用 `-race`**（本机 cgo 不可用，会挂起）。并发用例用确定性 goroutine + 互斥锁断言。
- **后端零注释、gofmt 缩进**；门禁 `bash scripts/check.sh`（含 gofmt / go vet / go build / 空白 / repository import 检查）。
- **实现遵循 `backend/STYLE.md` 现行条款**：零注释、错误码先登记 `apierror`、service 不碰 gin、测试同包打真库；§5 的「目标态」API 形状（分页、epoch、认证响应补 `id`）不得提前实施。
- **前端检查**：`npx tsc --noEmit`；改动文件跑 `npx eslint <files>` 与 `npx prettier --check <files>`（不要跑全仓 `format:check`，避免被无关文件干扰）。
- **限流默认值**（写死常量）：登录账号 5 次失败 / 15 分钟窗口 → 15 分钟冷却；登录 IP 30 次 / 15 分钟；注册邮箱 3 次 / 1 小时；注册 IP 5 次 / 1 小时；2FA 账号 10 次失败 / 15 分钟窗口 → 15 分钟冷却；2FA IP 30 次 / 15 分钟。
- **可信代理默认值**：`127.0.0.1/32,::1/128`；生产非回环代理必须显式配置 `TRUSTED_PROXIES`。
- **测试装配**：现有 auth 测试统一注入禁用策略（`disabledRateLimitPolicies()`），避免同一测试 app 内多次登录/注册撞额度；限流专用测试单独构造 app，用默认或短窗口策略。
- **不动的文件**：`backend/migrations/**`、ent schema、`scripts/**`、`guard/**`；不改登录 CSRF、2FA 密码复核、auth_version 三项已入库逻辑。

## Review Focus

spec 隐含、最可能咬人的五条，逐条钉到对应任务：

1. **账号枚举**：未知标识与已知标识在第 6 次必须同形 429，不能一个 429 一个 401。→ Task 4 `TestLoginRateLimitUnknownIdentifierParity`
2. **成功清零与冷却到期**：正确密码/验证码必须清零失败计数；冷却到期后必须恢复，不能永久锁号。→ Task 4 `TestLoginRateLimitResetsOnSuccess` / `TestLoginRateLimitCooldownExpires`
3. **并发二段失败不能丢计数**：单会话 5 次上限可被并发/重登绕过，账号级计数必须原子累加。→ Task 1 `TestConcurrentFailuresAreCountedOnce` + Task 6 `TestVerifyRateLimitConcurrentAcrossSessions`
4. **IP 键不能被伪造**：非可信来源的 `X-Forwarded-For` 不得改变限流键或会话 IP。→ Task 3 `TestClientIPIgnoresForwardedForFromUntrustedPeer`
5. **429 契约与测试互不污染**：429 必须是 429 / `rate_limit_error` / `rate_limited` + 正整数 `Retry-After`；现有 auth 测试共用 127.0.0.1，默认策略不得把旧用例打红。→ Task 2 `TestRateLimitedSetsRetryAfter` + Task 9 全量 `scripts/test.sh`

---

## 文件结构

| 文件 | 动作 | 唯一职责 | 任务 |
| --- | --- | --- | --- |
| `backend/internal/common/ratelimit/ratelimit.go` | Create | 进程内固定窗口 + 失败冷却计数、清理、键上限 | Task 1 |
| `backend/internal/common/ratelimit/ratelimit_test.go` | Create | 窗口/冷却/并发/清理单测 | Task 1 |
| `backend/internal/common/apierror/apierror.go` | Modify | `ErrRateLimit`、429 映射、`Retry-After` 写出 | Task 2 |
| `backend/internal/common/apierror/apierror_test.go` | Modify | 429 注册表与响应头用例 | Task 2 |
| `backend/internal/config/config.go` | Modify | `TrustedProxies` 解析与默认值 | Task 3 |
| `backend/internal/config/config_test.go` | Modify | 默认/自定义/非法 `TRUSTED_PROXIES` 用例 | Task 3 |
| `backend/internal/app/app.go` | Modify | `SetTrustedProxies`、构造 limiter、注入 auth | Task 3/4 |
| `backend/internal/modules/auth/controller.go` | Modify | `requestSessionMeta` 改 `ClientIP()`、注册传 meta | Task 3/5 |
| `backend/internal/modules/auth/ratelimit.go` | Create | 策略常量、键规则、`allowWindow/checkFailure/recordFailure/resetFailure` | Task 4 |
| `backend/internal/modules/auth/service.go` | Modify | `Service` 字段、`newService`、登录/注册/verify 接线 | Task 4/5/6 |
| `backend/internal/modules/auth/module.go` | Modify | `New` 增加 limiter 参数 | Task 4 |
| `backend/internal/modules/auth/auth_test.go` | Modify | 测试 helper 注入禁用策略、可信代理参数 | Task 3/4 |
| `backend/internal/modules/auth/sessions_test.go` | Modify | XFF 断言适配可信代理、删 IP 截断断言 | Task 3 |
| `backend/internal/modules/auth/ratelimit_test.go` | Create | 登录/注册/2FA 限流集成用例 | Task 4/5/6 |
| `frontend/lib/api.ts` | Modify | `ApiError.retryAfter`，读取响应头 | Task 7 |
| `frontend/lib/rate-limit.ts` | Create | `parseRetryAfter`、`formatRetryDuration` | Task 7 |
| `frontend/hooks/use-signin.ts` | Modify | `rate_limited` 映射、禁用提交、按钮显示剩余时间 | Task 7 |
| `frontend/app/signin/signin-form.tsx` | Modify | 按钮文字显示剩余时间 | Task 7 |
| `frontend/hooks/use-signup.ts` | Modify | `rate_limited` 映射、禁用提交、按钮显示剩余时间 | Task 8 |
| `frontend/app/signup/signup-form.tsx` | Modify | 按钮文字显示剩余时间 | Task 8 |
| `frontend/messages/zh-CN.json` | Modify | 登录/注册限流文案 | Task 7/8 |
| `frontend/messages/en.json` | Modify | 登录/注册限流文案 | Task 7/8 |
| `backend/README.md` | Modify | 限流章节、env、错误表、已知缺口、IP 口径 | Task 9 |
| `backend/.env.example` | Modify | 可选 `TRUSTED_PROXIES` 示例 | Task 9 |
| `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md` | Modify | 429/`rate_limited`/`Retry-After` 从保留改为落地 | Task 9 |
| `docs/DEVLOG.md` | Modify | 顶部倒序条目 | Task 9 |
| `backend/internal/version/version.go` | Modify | 进位脚本代劳 | Task 9 |

---

### Task 1: 进程内限流器

**Files:**
- Create: `backend/internal/common/ratelimit/ratelimit.go`
- Create: `backend/internal/common/ratelimit/ratelimit_test.go`

**Interfaces:**
- Produces: `func New() *Limiter`；`func (l *Limiter) Allow(key string, limit int, window time.Duration) (bool, time.Duration)`；`func (l *Limiter) Blocked(key string) (bool, time.Duration)`；`func (l *Limiter) RecordFailure(key string, limit int, window, cooldown time.Duration)`；`func (l *Limiter) Reset(key string)`。
- 同包测试可直接设置未导出字段：`now func() time.Time`、`maxEntries int`、`sweepInterval time.Duration`、`entryTTL time.Duration`。
- 语义：`Allow` 是固定窗口计数；`Blocked` 查冷却；`RecordFailure` 在 `window` 内累计连续失败，达到 `limit` 后写入 `cooldown` 冷却；`Reset` 删除键；所有公开方法更新 `lastSeen` 并触发惰性清理/键上限淘汰。

- [ ] **Step 1: 写失败测试**

测试文件用假时钟，覆盖六条：

```go
func TestAllowRejectsAtLimitAndRecoversAfterWindow(t *testing.T)
func TestBlockedCooldownAndRecovery(t *testing.T)
func TestRecordFailureStaleWindowResets(t *testing.T)
func TestConcurrentFailuresAreCountedOnce(t *testing.T)
func TestResetClearsFailureState(t *testing.T)
func TestSweepExpiresEntriesAndCapsMap(t *testing.T)
```

关键断言：

- `Allow("k", 2, time.Minute)` 前两次 `true`；第三次 `false` 且 `retry == time.Minute`；时钟前进 1 分钟后恢复 `true`。
- `RecordFailure("k", 2, time.Minute, 5*time.Minute)` 两次后 `Blocked("k") == true, retry == 5*time.Minute`；前进 5 分钟后 `false`；再记一次失败仍不 blocked（计数已清零）。
- 第一次失败后时钟前进 `window+1`，再记一次失败，`Blocked` 为 false（陈旧计数被窗口重置）。
- 100 个 goroutine 同时 `RecordFailure("k", 100, time.Hour, time.Hour)`；`WaitGroup.Wait()` 后 `Blocked` 必为 true（少计即失败）。不使用 `-race`。
- `Reset` 后一次失败不触发 limit=2 的冷却。
- 设置 `maxEntries=2`，写 3 个键后 `len(entries)==2` 且最早键被淘汰；设置小 `entryTTL`，时钟前进后调用任一公开方法，过期键被清掉。

- [ ] **Step 2: 跑测试确认失败**

Run: `cd F:/protect/NeoBlog/backend && go test ./internal/common/ratelimit/ -count=1`

Expected: 编译失败（`undefined: New`）。

- [ ] **Step 3: 实现限流器**

数据结构与签名固定：

```go
type entry struct {
    attempts     int
    windowStart  time.Time
    failures     int
    lastFailure  time.Time
    blockedUntil time.Time
    lastSeen     time.Time
}

type Limiter struct {
    mu            sync.Mutex
    entries       map[string]*entry
    now           func() time.Time
    maxEntries    int
    sweepInterval time.Duration
    entryTTL      time.Duration
    lastSweep     time.Time
}
```

默认值：`maxEntries = 100000`、`sweepInterval = time.Minute`、`entryTTL = 2*time.Hour`、`now = time.Now`。`Allow` 用 `attempts/windowStart`；`RecordFailure` 用 `failures/lastFailure/blockedUntil`；`Blocked` 在冷却到期时清空失败三字段；`sweepLocked` 按 `lastSeen` 清过期项，超上限时按 `lastSeen` 淘汰最旧项；所有公开方法先 `touch` 再按需 `sweepLocked`。返回值 `retry` 为剩余时间，可为 0 附近值，具体秒数取整由 apierror 负责。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/common/ratelimit/ -count=1`

Expected: `ok website/backend/internal/common/ratelimit`。

- [ ] **Step 5: 门禁**

Run: `gofmt -l internal/common/ratelimit && go vet ./internal/common/ratelimit/`

Expected: 无输出。

- [ ] **Step 6: 停在工作区**

不提交。`git status --short` 只应出现两个新文件。

---

### Task 2: apierror 429 与 Retry-After

**Files:**
- Modify: `backend/internal/common/apierror/apierror.go`
- Modify: `backend/internal/common/apierror/apierror_test.go`

**Interfaces:**
- Produces: `apierror.ErrRateLimit`（`rate_limit_error` / `rate_limited` / 429 / param null）；`apierror.RateLimited(retryAfter time.Duration) error`；`Write` / `Abort` 对带等待时间的错误设置 `Retry-After`（向上取整秒，最小 1）。
- Consumes: 现有 `Error`、`statusByCode`、`resolve`、`Body`、`statusOf`。

- [ ] **Step 1: 写失败测试**

`apierror_test.go`：

- `TestRegistryMatchesSpec` 的 `cases` 增加：

```go
{"rate_limited", ErrRateLimit, TypeRateLimit, "rate_limited", http.StatusTooManyRequests, nil},
```

- `TestRegistryIntegrity`：`all` 切片加 `ErrRateLimit`；两个计数 `32` 改 `33`；`statusTypes` 加 `http.StatusTooManyRequests: TypeRateLimit`。
- 新增：

```go
func TestRateLimitedSetsRetryAfter(t *testing.T) {
    rr := httptest.NewRecorder()
    ctx, _ := gin.CreateTestContext(rr)
    ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
    Write(ctx, RateLimited(1500*time.Millisecond))
    if rr.Code != http.StatusTooManyRequests { ... }
    if got := rr.Header().Get("Retry-After"); got != "2" { ... }
    // body: type rate_limit_error, code rate_limited, param null, no message
}
```

再加 `RateLimited(0)` → `Retry-After: 1`；`Abort` 同样设置头；包装错误经 `errors.As` 仍能取到 `ErrRateLimit`。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/common/apierror/ -count=1`

Expected: 编译失败（`undefined: ErrRateLimit` / `RateLimited`）。

- [ ] **Step 3: 实现**

- 哨兵：`ErrRateLimit = &Error{Type: TypeRateLimit, Code: "rate_limited"}`；`statusByCode["rate_limited"] = http.StatusTooManyRequests`。
- 包装：

```go
type rateLimitError struct {
    retryAfter time.Duration
}

func (e *rateLimitError) Error() string { return ErrRateLimit.Code }
func (e *rateLimitError) Unwrap() error { return ErrRateLimit }

func RateLimited(retryAfter time.Duration) error {
    if retryAfter < time.Second {
        retryAfter = time.Second
    }
    return &rateLimitError{retryAfter: retryAfter}
}
```

- 抽出 `writeWithHeaders(ctx, status, body, err)` 或在 `Write` / `Abort` 内用 `errors.As` 找 `*rateLimitError`，设置 `Retry-After = strconv.Itoa(int(math.Ceil(d.Seconds())))`；`ErrRateLimit` 的 `param` 为 nil，body 不带 message。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/common/apierror/ -count=1`

Expected: `ok website/backend/internal/common/apierror`。

- [ ] **Step 5: 停在工作区**

不提交。

---

### Task 3: 可信代理与客户端 IP

**Files:**
- Modify: `backend/internal/config/config.go`
- Modify: `backend/internal/config/config_test.go`
- Modify: `backend/internal/app/app.go`
- Modify: `backend/internal/modules/auth/controller.go`
- Modify: `backend/internal/modules/auth/auth_test.go`
- Modify: `backend/internal/modules/auth/sessions_test.go`

**Interfaces:**
- Produces: `Config.TrustedProxies []string`；`TRUSTED_PROXIES` 逗号分隔，默认 `127.0.0.1/32,::1/128`；非法项启动失败；`requestSessionMeta` 只使用 `ctx.ClientIP()`。
- Consumes: Gin 的 `SetTrustedProxies` / `ClientIP`。

- [ ] **Step 1: 写失败测试**

`config_test.go` 增加：

```go
func TestLoadTrustedProxiesDefaultsToLoopback(t *testing.T)
func TestLoadReadsTrustedProxies(t *testing.T)
func TestLoadRejectsInvalidTrustedProxies(t *testing.T)
```

- 默认：`t.Setenv("TRUSTED_PROXIES", "")`，`Load()` 后 `reflect.DeepEqual(cfg.TrustedProxies, []string{"127.0.0.1/32", "::1/128"})`。
- 自定义：`"10.0.0.0/8, 192.168.1.1 ,::1"` → 去空白后三项。
- 非法：`"not-an-ip"` → 错误文本含 `TRUSTED_PROXIES`。

`auth` 测试：

- `TestLoginStoresSessionMetadata` 的 `X-Forwarded-For` 改成单跳 `"203.0.113.7"`，期望不变。
- `TestLoginTruncatesOversizedHeaders` 改名为 `TestLoginTruncatesOversizedUserAgent`，删掉 XFF 请求头与 IP 长度断言，只保留 UA 512 截断。
- 新增 `TestClientIPIgnoresForwardedForFromUntrustedPeer`：用 `nil` 可信代理构造 server，登录时带 `X-Forwarded-For: 203.0.113.7`，会话 IP 必须是 `127.0.0.1`。
- 新增 `TestClientIPUsesForwardedForFromTrustedPeer`（可由 `TestLoginStoresSessionMetadata` 覆盖；若单列，期望 `203.0.113.7`）。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/config/ -run TrustedProxies -count=1`，Expected: `undefined: TRUSTED_PROXIES` 相关断言失败/编译失败。
Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestClientIP|TestLoginStoresSessionMetadata|TestLoginTruncates'`，Expected: 新用例失败。

- [ ] **Step 3: 实现配置**

`Config` 增加 `TrustedProxies []string`。`Load` 调 `getTrustedProxies()`：

```go
func getTrustedProxies() ([]string, error) {
    raw := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES"))
    if raw == "" {
        return []string{"127.0.0.1/32", "::1/128"}, nil
    }
    // split, trim; 每项必须是 IP 或 CIDR；空项/非法项返回点名 TRUSTED_PROXIES 的错误
}
```

不带 `/` 的项用 `net.ParseIP` 校验，带 `/` 的用 `net.ParseCIDR` 校验；保留原字符串交给 Gin。

- [ ] **Step 4: 实现 app 与 controller**

- `app.New`：把 `_ = engine.SetTrustedProxies(nil)` 换成 `if err := engine.SetTrustedProxies(cfg.TrustedProxies); err != nil { return nil, err }`。
- `requestSessionMeta`：删除手工 `X-Forwarded-For` / `X-Real-IP` 分支，改成：

```go
ip := truncateHeader(ctx.ClientIP(), maxIPLength)
```

`UserAgent` 逻辑不变；`strings` 仍被 `requireJSONRequest` 使用。

- [ ] **Step 5: 测试 helper 加可信代理参数**

`auth_test.go`：

- 增加 `var testTrustedProxies = []string{"127.0.0.1/32", "::1/128"}`。
- 把现有 `newTestAppWithSessionsAndSettings` 拆成薄包装 + `newTestAppWithProxySettings(t, db, proxies []string, settingsService *settings.Service)`；薄包装传 `testTrustedProxies`。
- 新增 `newTestAppWithTrustedProxies(t, db, proxies []string)`，解析 settings 后调用 `newTestAppWithProxySettings`。
- 在 `newTestAppWithProxySettings` 里 `engine.SetTrustedProxies(proxies)`，失败 `t.Fatalf`。

- [ ] **Step 6: 跑测试确认通过**

Run: `go test ./internal/config/ -count=1`
Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestClientIP|TestLoginStoresSessionMetadata|TestLoginTruncates|TestSessionsListIncludesCurrentWithMetadata'`

Expected: 全绿；`TestSessionsListIncludesCurrentWithMetadata` 的单跳 XFF 仍得到 `203.0.113.7`。

- [ ] **Step 7: 停在工作区**

不提交。

---

### Task 4: auth 策略、注入与登录限流

**Files:**
- Create: `backend/internal/modules/auth/ratelimit.go`
- Create: `backend/internal/modules/auth/ratelimit_test.go`
- Modify: `backend/internal/modules/auth/service.go`
- Modify: `backend/internal/modules/auth/module.go`
- Modify: `backend/internal/app/app.go`
- Modify: `backend/internal/modules/auth/auth_test.go`

**Interfaces:**
- Produces:

```go
type windowPolicy struct {
    Limit  int
    Window time.Duration
}

type failurePolicy struct {
    Limit    int
    Window   time.Duration
    Cooldown time.Duration
}

type rateLimitPolicies struct {
    LoginIdentifier failurePolicy
    LoginIP         windowPolicy
    RegisterEmail   windowPolicy
    RegisterIP      windowPolicy
    VerifyAccount   failurePolicy
    VerifyIP        windowPolicy
}

func defaultRateLimitPolicies() rateLimitPolicies
func disabledRateLimitPolicies() rateLimitPolicies
```

- 键：`loginIdentifierKey(identifier)`（trim + lower 后 SHA-256，前缀 `login:id:`）、`registerEmailKey(email)`（前缀 `register:email:`）、`ipKey(scope, ip)`（空 IP → `unknown`，前缀 `<scope>:ip:`）、`verifyAccountKey(userID)`（前缀 `verify:user:`）。
- `Service` 增加 `limiter *ratelimit.Limiter`、`policies rateLimitPolicies`。
- `newService(db, sessions, totpKey, settingsService, limiter, policies)`；`New(db, sessions, totpKey, settingsService, limiter)` 用 `defaultRateLimitPolicies()`。
- 辅助方法：`allowWindow(p windowPolicy, key string) error`、`checkFailure(p failurePolicy, key string) error`、`recordFailure(p failurePolicy, key string)`、`resetFailure(key string)`；`Limit <= 0` 时直接跳过。

- [ ] **Step 1: 写失败测试（登录）**

`ratelimit_test.go`。测试 helper 默认用 `disabledRateLimitPolicies()`；限流用例用 `newTestAppWithRatePolicies(t, db, policies)`（在 Task 3 的 `newTestAppWithProxySettings` 基础上增加 limiter/policies 参数，服务用 `newService` 直接构造）。

用例：

```go
func TestLoginRateLimitLocksIdentifier(t *testing.T)
func TestLoginRateLimitUnknownIdentifierParity(t *testing.T)
func TestLoginRateLimitResetsOnSuccess(t *testing.T)
func TestLoginRateLimitIPWindow(t *testing.T)
func TestLoginRateLimitCooldownExpires(t *testing.T)
```

断言：

- 默认策略：同一邮箱先 5 次错密码（均 401 `invalid_credentials`），第 6 次错密码与第 6 次正确密码都 429 `rate_limited`；`Retry-After` 正整数；body 无 message。
- 未知标识：5 次错密码后第 6 次 429；与已知标识的 429 code/body 同形。
- 成功清零：4 次错密码 + 正确登录 200 + 再 4 次错密码仍 401（不提前 429）；第 5 次错密码 401，第 6 次 429。
- IP 窗口：自定义 `LoginIP{Limit: 2, Window: time.Hour}`，账号策略禁用；同一 IP 下两个不同标识的错密码 401，第三个 429；带另一个可信 XFF（如 `203.0.113.8`）的请求仍是 401，证明 IP 键独立。
- 冷却到期：自定义 `LoginIdentifier{Limit: 2, Window: time.Minute, Cooldown: 50 * time.Millisecond}`，IP 禁用；两次失败后第三次 429；`time.Sleep(80 * time.Millisecond)` 后正确密码 200。

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestLoginRateLimit'`

Expected: 编译失败（`undefined: rateLimitPolicies` / `newTestAppWithRatePolicies`）。

- [ ] **Step 3: 实现策略与辅助方法**

`ratelimit.go` 常量按 spec 写死：

```go
const (
    loginFailureLimit    = 5
    loginFailureWindow   = 15 * time.Minute
    loginFailureCooldown = 15 * time.Minute
    loginIPLimit         = 30
    loginIPWindow        = 15 * time.Minute
    registerEmailLimit   = 3
    registerEmailWindow  = time.Hour
    registerIPLimit      = 5
    registerIPWindow     = time.Hour
    verifyFailureLimit    = 10
    verifyFailureWindow   = 15 * time.Minute
    verifyFailureCooldown = 15 * time.Minute
    verifyIPLimit         = 30
    verifyIPWindow        = 15 * time.Minute
)
```

`defaultRateLimitPolicies()` 组装；`disabledRateLimitPolicies()` 返回零值（所有 Limit 0）。键函数用 `sha256` + `hex`；`ipKey` 空值归一为 `unknown`。辅助方法调用 limiter，`Allow` / `Blocked` 拒绝时返回 `apierror.RateLimited(retry)`；`recordFailure` 不返回错误（当前这次仍返回认证错误，下一次才 429）。

- [ ] **Step 4: 接线 Service / Module / app**

- `Service` 加字段；`newService` 加参数；`New` 加 `limiter *ratelimit.Limiter`。
- `app.New`：`limiter := ratelimit.New()`，传给 `auth.New`。
- `Login` 顺序：

```go
if err := s.allowWindow(s.policies.LoginIP, ipKey("login", meta.IP)); err != nil { return User{}, err }
accountKey := loginIdentifierKey(identifier)
if err := s.checkFailure(s.policies.LoginIdentifier, accountKey); err != nil { return User{}, err }
// 现有长度检查、查用户、bcrypt
// 未知用户/密码错误：s.recordFailure(...)，返回 invalid_credentials
// 密码正确：s.resetFailure(accountKey)，再走 2FA / establishSession
```

- 未知用户仍先跑 dummy bcrypt 再记失败；数据库错误/500 路径不记失败。

- [ ] **Step 5: 测试 helper 改造**

- `newTestAppWithProxySettings` 增加 `limiter *ratelimit.Limiter, policies rateLimitPolicies` 参数；现有包装传 `ratelimit.New(), disabledRateLimitPolicies()`。
- 新增 `newTestAppWithRatePolicies(t, db, policies)`：使用 `testTrustedProxies` + `ratelimit.New()` + 给定 policies。
- 构造 module 时用 `newService` + `&Module{Service: service, controller: newController(service)}`，不经过 `New`（`New` 只给生产用）。

- [ ] **Step 6: 跑测试**

Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestLogin|TestLoginRateLimit|TestRegister'`

Expected: 新用例全绿；旧登录/注册用例不红。

- [ ] **Step 7: 停在工作区**

不提交。

---

### Task 5: 注册限流

**Files:**
- Modify: `backend/internal/modules/auth/service.go`
- Modify: `backend/internal/modules/auth/controller.go`
- Modify: `backend/internal/modules/auth/ratelimit_test.go`

**Interfaces:**
- Consumes: Task 4 的 `policies.RegisterIP` / `policies.RegisterEmail`、`allowWindow`、`registerEmailKey`、`ipKey`。
- Produces: `Register(ctx, email, nickname, password string, meta SessionMeta)`；controller 传 `requestSessionMeta(ctx)`。

- [ ] **Step 1: 写失败测试**

```go
func TestRegisterRateLimitIP(t *testing.T)
func TestRegisterRateLimitEmail(t *testing.T)
```

- IP：默认策略（注册 IP 5 次/小时）；同一 IP 下用 5 个不同邮箱各发一次弱密码注册（400 `weak_password`），第 6 次 429；带另一个可信 XFF（如 `203.0.113.8`）的请求仍返回 400，证明 IP 键独立；`Retry-After` 正整数。
- 邮箱：默认策略（邮箱 3 次/小时）；同一归一化邮箱连发 3 次弱密码 400，第 4 次 429；该 IP 计数为 4，低于 5，确保是邮箱闸门触发。
- 用 settings service 确保 `register.enabled=true`，`t.Cleanup` 复位，参照 `register_gate_test.go`。

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestRegisterRateLimit'`

Expected: 新用例失败（没有限流）。

- [ ] **Step 3: 实现**

- controller `signup`：`c.service.Register(ctx.Request.Context(), req.Email, req.Nickname, req.Password, requestSessionMeta(ctx))`。
- `Register` 开头查 IP 窗口；在 `normalizeEmail` + `validEmail/validNickname` 之后、密码策略之前查邮箱窗口；其余逻辑不动。

- [ ] **Step 4: 跑测试**

Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestRegister|TestLoginRateLimit'`

Expected: 全绿。

- [ ] **Step 5: 停在工作区**

不提交。

---

### Task 6: 2FA verify 限流

**Files:**
- Modify: `backend/internal/modules/auth/service.go`
- Modify: `backend/internal/modules/auth/ratelimit_test.go`

**Interfaces:**
- Consumes: Task 4 的 `policies.VerifyIP` / `policies.VerifyAccount`、`allowWindow`、`checkFailure`、`recordFailure`、`resetFailure`、`verifyAccountKey`、`ipKey`。
- Produces: 现有 `failSecondFactor` 保持不变；账号级冷却在它之前/之后都要生效。

- [ ] **Step 1: 写失败测试**

```go
func TestVerifyRateLimitAcrossSessions(t *testing.T)
func TestVerifyRateLimitIPGate(t *testing.T)
func TestVerifyRateLimitResetsOnSuccess(t *testing.T)
func TestVerifyRateLimitConcurrentAcrossSessions(t *testing.T)
```

- 跨会话：setup/enable 一个用户；会话 A 连错 5 次（第 5 次仍 `two_factor_locked`）；会话 B 再连错 5 次（第 5 次 `two_factor_locked`，账号计数到 10）；会话 C 登录得到 pending 后，用正确码 verify → 429，`Retry-After` 正整数。
- IP：`policies.VerifyIP = windowPolicy{Limit: 2, Window: time.Hour}`，账号禁用；无 pending 的两次 verify 401，第三次 429（IP 检查在 pending 检查之前）。
- 成功清零：4 次错误后正确码 200；重新登录再 4 次错误仍是 `two_factor_invalid_code`（账号计数已清零），第 5 次返回 `two_factor_locked`（会话内计数，不是 429）。
- 并发：10 个独立客户端各自登录拿 pending，goroutine 内直接 `http.Client.Do` 发错误码（不要在 goroutine 里调 `t.Fatalf`）；全部结束后新 pending + 正确码 → 429。此用例只依赖账号计数原子性，不依赖单会话 5 次。
- 现有 `TestTwoFactorVerifyLockout` / `TestTwoFactorReplayRejected` 不动。

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestVerifyRateLimit'`

Expected: 新用例失败。

- [ ] **Step 3: 实现**

`VerifySecondFactor` 顺序：

```go
if err := s.allowWindow(s.policies.VerifyIP, ipKey("verify", meta.IP)); err != nil { return User{}, err }
pendingUserID := s.sessions.GetString(ctx, pendingUserIDKey)
if pendingUserID == "" { return User{}, apierror.ErrUnauthorized }
accountKey := verifyAccountKey(pendingUserID)
if err := s.checkFailure(s.policies.VerifyAccount, accountKey); err != nil { return User{}, err }
// 现有查用户、TOTP/恢复码
// 失败：s.recordFailure(...)，再 return s.failSecondFactor(ctx)
// 成功：s.resetFailure(accountKey)，再 establishSession
```

账号冷却命中时返回 429，不销毁 pending 会话；解密/数据库等 500 路径不记失败。

- [ ] **Step 4: 跑测试**

Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestTwoFactor|TestVerifyRateLimit'`

Expected: 全绿，含旧锁定/重放用例。

- [ ] **Step 5: 停在工作区**

不提交。

---

### Task 7: 前端 429 契约与登录交互

**Files:**
- Modify: `frontend/lib/api.ts`
- Create: `frontend/lib/rate-limit.ts`
- Modify: `frontend/hooks/use-signin.ts`
- Modify: `frontend/app/signin/signin-form.tsx`
- Modify: `frontend/messages/zh-CN.json`
- Modify: `frontend/messages/en.json`

**Interfaces:**
- Produces: `ApiError.retryAfter?: number`；`parseRetryAfter(value: string | null): number | undefined`；`formatRetryDuration(seconds: number, locale: string): string`。
- `useSigninForm` 增加 `retryRemaining: number`、`retryMessage: string | null`。

- [ ] **Step 1: 改 API 客户端与共享格式化**

`frontend/lib/rate-limit.ts`：

```ts
export function parseRetryAfter(value: string | null): number | undefined
export function formatRetryDuration(seconds: number, locale: string): string
```

- `parseRetryAfter` 只接受正整数秒。
- `formatRetryDuration`：<60 秒显示“45 秒”/“45s”；≥60 显示“2 分 30 秒”/“2m 30s”；中文按 `locale.toLowerCase().startsWith("zh")` 判断。

`frontend/lib/api.ts`：

- `ApiError` 构造器增加第四参 `retryAfter?: number`，保存为 readonly。
- `apiRequest` 在非 OK 分支：`parseRetryAfter(response.headers.get("Retry-After"))` 传给 `ApiError`。
- 401 `unauthorized` 的 `reportSessionExpired` 不变；429 不触发它。

- [ ] **Step 2: 改登录 hook**

`use-signin.ts`：

- 引入 `useEffect`、`useLocale`、`formatRetryDuration`。
- 状态 `retryUntil` / `retryRemaining`；`useEffect` 每 1 秒更新剩余秒数，归零清空。
- `toMessage` 增加 `rate_limited`：`t("rateLimited", { duration: formatRetryDuration(error.retryAfter ?? 60, locale) })`。
- `submit` / `submitCode` 的 catch：若是 `ApiError` 且 `code === "rate_limited"`，记录 `retryUntil = Date.now() + (retryAfter ?? 60) * 1000`，toast 仍走现有 `resolveServiceError(error) ?? toMessage(error)`。
- `rate_limited` 不改 `step`、不调用 `backToCredentials`、不清空 identifier。
- 返回 `retryRemaining`、`retryMessage`（`retryRemaining > 0 ? t("retryIn", { duration: formatRetryDuration(retryRemaining, locale) }) : null`）。
- `submit` / `submitCode` 开头若 `retryRemaining > 0` 直接返回 undefined。

- [ ] **Step 3: 改登录表单**

`signin-form.tsx`：

- 解构 `retryRemaining`、`retryMessage`。
- 主提交按钮 `disabled={isPending || retryRemaining > 0}`。
- 返回凭据按钮保持 `disabled={isPending}`，冷却期间仍可返回。
- 主按钮下方渲染：

```tsx
{retryMessage ? (
  <p className="text-sm text-muted-foreground" aria-live="polite">{retryMessage}</p>
) : null}
```

- [ ] **Step 4: 文案**

`messages/zh-CN.json` / `en.json` 的 `auth.signin` 增加：

```json
"rateLimited": "尝试过于频繁，请 {duration}后再试",
"retryIn": "请 {duration}后再试"
```

```json
"rateLimited": "Too many attempts. Try again in {duration}.",
"retryIn": "Try again in {duration}."
```

- [ ] **Step 5: 检查**

Run:

```bash
cd F:/protect/NeoBlog/frontend
npx tsc --noEmit
npx eslint lib/api.ts lib/rate-limit.ts hooks/use-signin.ts app/signin/signin-form.tsx
npx prettier --check lib/api.ts lib/rate-limit.ts hooks/use-signin.ts app/signin/signin-form.tsx messages/zh-CN.json messages/en.json
```

Expected: 全部通过。运行期 429 画面归用户浏览器实测。

- [ ] **Step 6: 停在工作区**

不提交。

---

### Task 8: 前端注册交互

**Files:**
- Modify: `frontend/hooks/use-signup.ts`
- Modify: `frontend/app/signup/signup-form.tsx`
- Modify: `frontend/messages/zh-CN.json`
- Modify: `frontend/messages/en.json`

**Interfaces:**
- Consumes: `ApiError.retryAfter`、`formatRetryDuration`。
- Produces: `useSignupForm` 返回 `retryRemaining`、`retryMessage`。

- [ ] **Step 1: 改注册 hook**

- 引入 `useEffect`、`useLocale`、`formatRetryDuration`。
- 状态与登录 hook 相同：`retryUntil` / `retryRemaining` / `retryMessage`。
- `toFailure` 增加 `rate_limited`：通用消息、不指向字段（不返回 `field`）。
- catch 里若是 `rate_limited`，设置 `retryUntil`；toast 使用 `toFailure` 的消息。
- `submit` 开头若 `retryRemaining > 0` 直接返回 undefined；表单值保留。

- [ ] **Step 2: 改注册表单**

`signup-form.tsx`：

- 解构 `retryRemaining`、`retryMessage`。
- 提交按钮 `disabled={isPending || retryRemaining > 0}`。
- 按钮下方渲染与登录相同的 `retryMessage` 段落。

- [ ] **Step 3: 文案**

`auth.signup` 增加与 `auth.signin` 相同的 `rateLimited` / `retryIn` 两条（中文/英文）。

- [ ] **Step 4: 检查**

Run:

```bash
cd F:/protect/NeoBlog/frontend
npx tsc --noEmit
npx eslint hooks/use-signup.ts app/signup/signup-form.tsx
npx prettier --check hooks/use-signup.ts app/signup/signup-form.tsx messages/zh-CN.json messages/en.json
```

Expected: 全部通过。运行期画面归用户实测。

- [ ] **Step 5: 停在工作区**

不提交。

---

### Task 9: 文档、版本与全量验证

**Files:**
- Modify: `backend/README.md`
- Modify: `backend/.env.example`
- Modify: `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md`
- Modify: `docs/DEVLOG.md`
- Modify: `backend/internal/version/version.go`（脚本代劳）

**Interfaces:**
- Consumes: 前八个任务的全部实现与测试。

- [ ] **Step 1: README**

- 在「两步验证」之后、「用户组与权限」之前新增「### 限流」：策略表（六个键）、键规则、429/`Retry-After`、单实例/内存边界、测试 helper 说明。
- 环境变量表加 `TRUSTED_PROXIES`（默认 `127.0.0.1/32,::1/128`，生产非回环代理必须显式配置）。
- 错误契约表加 429 `rate_limit_error` / `rate_limited`。
- 「已知缺口」删除限流与代理配置两条，保留 `sessions.token` 原始存储与会话历史缺口。
- 会话管理段的 IP 来源改为「`ClientIP()` 在可信代理下解析，限流与会话列表共用」。
- 2FA 段的「真正的闸门在登录限流那一轮」改为「登录限流已落地，见『限流』；会话内 5 次保留」。

- [ ] **Step 2: API 规范文档**

`docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md`：把 429 / `rate_limit_error` 的「词汇保留、当前无产生点」更新为已落地；补 `rate_limited` token 与 `Retry-After` 头；同步 §4 状态表、§7.2 type 表、§11/§13 的相关行。

- [ ] **Step 3: `.env.example`**

增加可选示例：

```
# 可信代理（逗号分隔 IP/CIDR）。默认 127.0.0.1/32,::1/128；生产经非回环代理时改成代理网段。
# TRUSTED_PROXIES=127.0.0.1/32,::1/128
```

- [ ] **Step 4: DEVLOG**

在 `docs/DEVLOG.md` 顶部新增本轮条目（日期 2026-10-20，若执行日不同写实际日期）：范围、决策（三个入口、进程内、可信代理、写死默认值）、验证结果、已知缺口（单实例/重启清零、定向锁号取舍、前端 429 画面运行期未验、本机未跑 `-race`）。

- [ ] **Step 5: 版本进位**

- 先 `grep 'Version =' backend/internal/version/version.go` 读当前值。
- 若仍是 `0.21.1`，执行一次 `bash backend/scripts/bump-version.sh patch`；若并行轮已是 `0.21.2`，保持 `0.21.2`，不要重复进位。
- 只跑一次；验证 `version.go` 与 README 顶部一致。

- [ ] **Step 6: 全量验证**

```bash
export PATH="/f/software/go/bin:$PATH"
cd F:/protect/NeoBlog/backend
bash scripts/check.sh
set -a; source .env; set +a
bash scripts/test.sh
```

Expected: `check ok`；`scripts/test.sh` 全绿。

前端：

```bash
cd F:/protect/NeoBlog/frontend
npx tsc --noEmit
npm run lint
npx prettier --check lib/api.ts lib/rate-limit.ts hooks/use-signin.ts hooks/use-signup.ts app/signin/signin-form.tsx app/signup/signup-form.tsx messages/zh-CN.json messages/en.json
```

Expected: 全部通过。`npm run lint` 若被无关文件的既有问题挡住，改用改动文件定向 eslint，并在 DEVLOG 标注。

- [ ] **Step 7: 变异自检**

依次做，做完立即还原：

1. 删除 `Login` 里的 `s.checkFailure(s.policies.LoginIdentifier, accountKey)` → `TestLoginRateLimitLocksIdentifier` 必红。
2. 删除 `Register` 里的 IP `allowWindow` → `TestRegisterRateLimitIP` 必红。
3. 删除 `VerifySecondFactor` 里的账号 `checkFailure` → `TestVerifyRateLimitAcrossSessions` 必红。
4. 把 `RateLimited` 的 `Retry-After` 写出去掉 → `TestRateLimitedSetsRetryAfter` 必红。

每条还原后重跑对应测试，确认回绿。

- [ ] **Step 8: 停在工作区**

- `git status --short` 核对：只应有本轮目标文件与 spec/plan；本计划开始时并行侧栏改动已在 `442a7b3` 入库，不在工作区。
- **不提交**。用户点名后再按文件清单逐项 `git add`；若用户要单次提交，建议 message：`feat(auth): 登录注册与二段验证限流——账号/IP 双维度 + 429 Retry-After`。

---

## 计划自审记录

- **spec 覆盖**：§3 架构 → Task 1/4；§4 策略 → Task 4/5/6；§5 流程 → Task 4/5/6；§6 契约 → Task 2/7；§7 可信代理 → Task 3；§8 前端 → Task 7/8；§9 测试 → 各任务 + Task 9；§10 文档/版本 → Task 9；§12 验收 → Task 9 Step 6/7。无遗漏。
- **类型一致**：`Allow/Blocked/RecordFailure/Reset`、`windowPolicy/failurePolicy/rateLimitPolicies`、`allowWindow/checkFailure/recordFailure/resetFailure`、`newService`/`New` 的 limiter 参数在各任务中同名同签名。
- **测试互不污染**：现有 auth helper 默认注入禁用策略；限流用例单独构造 app；全量 `scripts/test.sh` 作为最终门禁。
- **本机约束**：不跑 `-race`；并发用例用 `WaitGroup` + 互斥锁断言；所有测试命令带 `.env` 导入。
- **提交口径**：全程不主动提交，符合用户默认单次提交偏好；Task 9 只给建议 message。
