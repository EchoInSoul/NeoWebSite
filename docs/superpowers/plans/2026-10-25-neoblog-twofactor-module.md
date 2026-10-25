# 两步验证拆成独立能力模块 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 2FA（TOTP + 恢复码 + 二段验证）从 auth 拆成独立能力模块 `internal/modules/twofactor`，auth 收缩为账号与会话管理；路由、错误码、限流阈值、响应形状零变化。

**Architecture:** 照 login/registration/session 的既定模式：twofactor 为无路由能力模块（service/repository/totp/secretbox/ratelimit/dto 六文件），实现与仓库方法从 auth 剪切；6 条路由仍由 auth controller 声明、handler 改薄壳分发；共享动作（验密码/活跃用户/会话吊销）在 twofactor 包内自备副本（自包含裁定）；`TOTPEncryptionKey` 从 app 直达 twofactor。

**Tech Stack:** Go 1.27 + gin + ent + scs + pquerna/otp；测试用真实 Postgres（`DATABASE_URL_TEST`）。

**Spec:** [2026-10-25-neoblog-twofactor-module-design.md](../specs/2026-10-25-neoblog-twofactor-module-design.md)

## Global Constraints

- **绝不提交。** 每个任务结束只跑测试、核对 diff，改动留在工作区等用户检查。不要 `git add` / `git commit` / `git push`。
- **全仓零人工注释**；**禁止兜底/降级**（失败按错误上抛）。
- 路由路径、错误码、限流阈值（失败 10 次/15 分钟窗/15 分钟冷却；IP 30 次/15 分钟）、响应 JSON 形状**零变化**。
- `auth.totp_enabled` 设置键**留在 auth 组**；恢复码邮件接口保持 501 占位；前端零改动；零迁移。
- 不动 `findUserByIdentifier`（邮箱登录轮）。
- 后端测试起手式（串行、不加 `-race`、不双开打同一个 `neoblog_test`）：
  ```bash
  cd backend
  export PATH="/f/software/go/bin:/f/software/gopath/bin:$PATH"
  export GOPROXY=https://goproxy.cn,direct
  set -a; source .env; set +a
  bash scripts/test.sh ./internal/...
  ```
- 静态门禁：`cd backend && bash scripts/check.sh`。
- 实施前先 `git log -1` 现读 HEAD；本计划引用的行号会漂，所有代码引用现读现用。
- `TestVerifyEmailRegistrationConcurrent` 为既有偶发（在案），task 级验证允许 `-skip` 该用例；收尾全量跑一次记录结果。

## Review Focus

1. **响应形状漂移**：`/sessions/current/verify` 与 `me` 的 JSON 必须逐字段不变（`email`/`nickname`/`bio`/`two_factor_enabled`/`group`/`permissions`）——twofactor.User（无 ID）→ auth.UserResponse 的映射漏字段会静默改变 API（Task 5 的既有 flow 用例断言逐字段覆盖）。
2. **`totpNow` 替钟失效**：flow 测试大量用 `fakeTOTPClock` 控制 TOTP 窗口；若随迁后包级变量被改成局部或丢了 `var totpNow = time.Now`，重放/窗口边界测试会假绿或红（Task 4 迁移后全量跑即验证）。
3. **限流行为静默丢失**：拆分时 `recordFailure` / `checkFailure` 调用点若漏搬，登录二段验证将不再计数锁定（Task 4 的变异自检 #1 与既有 4 条 `TestVerifyRateLimit*` 兜底）。
4. **`me.two_factor_enabled` 映射断裂**：auth `toUser` 的 `TotpEnabledAt != nil` 映射若被顺手改掉，me 接口静默返回 false（Task 4 变异自检 #3，靶子断言在 Task 4 明确保留在 auth 包的两条用例里）。
5. **装配单例**：`app.go` 与 auth controller 必须共享同一个 `*twofactor.Service` 实例；若测试 app 手拼两个实例，设置读取（`totp_enabled` 开关）可能出现两份状态（Task 5 装配核对步骤明确检查）。

---

### Task 1: twofactor 包骨架——dto / totp / secretbox 随迁

**Files:**
- Create: `backend/internal/modules/twofactor/dto.go`、`backend/internal/modules/twofactor/totp.go`、`backend/internal/modules/twofactor/secretbox.go`
- Test: `backend/internal/modules/twofactor/totp_test.go`、`backend/internal/modules/twofactor/secretbox_test.go`（自 auth 包改包名随迁）

**Interfaces:**
- Produces: 类型 `TwoFactorCodeRequest{Code string \`json:"code"\`}`、`TwoFactorSetupRequest{Password}`、`TwoFactorEnableRequest{Code,Password}`、`TwoFactorDisableRequest{Password}`、`TwoFactorSetupResponse{Secret,OTPAuthURL}`（json `secret`/`otpauth_url`）、`RecoveryCodesResponse{RecoveryCodes}`（json `recovery_codes`）、`User{Email,Nickname,Bio *string}`；函数 `generateTOTPSetup(accountName) (totpSetup, error)`、`rebuildTOTPSetup`、`matchTOTPCounter(code, base32Secret, now) (int64, bool, error)`、`generateRecoveryCodes(count)`、`formatRecoveryCode`、`hashRecoveryCode(code)`、`isSixDigitCode`、`sealSecret(key, plain)` / `openSecret(key, sealed)`、常量 `totpIssuer="两步验证"`、`totpPeriodSeconds=30`、`totpSecretBytes=10`、`totpSkewWindows=1`、`totpSetupReuseWindow=5min`、`totpSetupClaimAttempts=2`、`recoveryCodeBytes=10`、`recoveryCodeGroupSize=4`、`recoveryCodesPerBatch=5`、变量 `totpNow = time.Now`。auth 包这些符号**本任务不删**（两包短暂并存，Task 4 删）。

- [ ] **Step 1: 随迁纯单测（RED）**

从 `backend/internal/modules/auth/` 复制 `totp_test.go`、`secretbox_test.go` 到 `backend/internal/modules/twofactor/`，`package auth` 改 `package twofactor`；引用的常量/函数随文件复制过来（见 Step 2）。先复制测试与实现，跑单测确认原值原样。

- [ ] **Step 2: 随迁实现**

剪切 auth 的 `totp.go`、`secretbox.go` 内容到 twofactor 新文件（`package twofactor`，内容逐行不变，仅包名与 6 处 `auth:` 内部错误字符串前缀改为 `twofactor:`——STYLE.md §3 要求本仓构造的错误带域前缀；API 错误哨兵不在此层）。auth 的 `dto.go` 里 6 个 2FA 类型剪切到 `twofactor/dto.go`，并新增 `type User struct { Email string; Nickname string; Bio *string }`。auth 侧文件本任务保留（Task 4 删）。

- [ ] **Step 3: 跑两包测试确认并存绿**

Run: `bash scripts/test.sh ./internal/modules/twofactor/ ./internal/modules/auth/`
Expected: 双包 PASS（twofactor 单测绿；auth 原测试未动仍绿）

- [ ] **Step 4: 核对 diff**（不提交）

---

### Task 2: twofactor repository + ratelimit 随迁

**Files:**
- Create: `backend/internal/modules/twofactor/repository.go`、`backend/internal/modules/twofactor/ratelimit.go`
- Test: `backend/internal/modules/twofactor/repository_test.go`（auth 的 `TestConsumeRecoveryCodeConcurrent` / `TestClaimTOTPCounterConcurrent` / `TestClaimTOTPCounterOrdering` / `TestClaimTOTPSetupRejectedWhenFresh` 等仓库级用例的归属判定见 Task 4；本任务先建实现与单测壳）

**Interfaces:**
- Consumes: Task 1 的常量（`totpSetupReuseWindow` 等）。
- Produces: `repository` 具体类型 + `newRepository(db)`；方法 `findUserByID(ctx, id string) (*ent.User, error)`（ent.IsNotFound → `errUserNotFound`）、`claimTOTPSetup(ctx, id uuid.UUID, encryptedSecret string, now time.Time) (bool, error)`、`enableTOTP(ctx, id, enabledAt, codeHashes []string) (int, error)`、`disableTOTP(ctx, id) error`、`replaceRecoveryCodes(ctx, userID, codeHashes) error`、`consumeRecoveryCode(ctx, userID, hash) (bool, error)`、`claimTOTPCounter(ctx, id, counter int64) (bool, error)`、`insertRecoveryCodes`（包内）；限流 `verifyPolicies{Account failurePolicy, IP windowPolicy}`、`DefaultVerifyPolicies()`、方法 `allowIP(ip) error` / `checkAccountFailure(key) error` / `recordAccountFailure(key)` / `resetAccountFailure(key)`、`accountKey(userID) string`（值 `verify:user:<id>`）、`ipKey("verify", ip)`（值 `verify:ip:<ip>`）。错误哨兵 `errUserNotFound`。

- [ ] **Step 1: 剪切实现**

从 auth 的 `repository.go` 剪切 6 个 2FA 方法 + `insertRecoveryCodes` 到 `twofactor/repository.go`（`errUserNotFound` 在 twofactor 重新声明；错误前缀 `auth:` → `twofactor:`）。从 auth 的 `ratelimit.go` 剪切 verify 常量与策略到 `twofactor/ratelimit.go`，对外形态按 Produces 重命名（`verifyAccountKey` → `accountKey` 等），逻辑与阈值逐值不变。

- [ ] **Step 2: 仓库级并发用例先搬三条（RED→GREEN 即搬即绿）**

把 auth `totp_flow_test.go` 的 `TestConsumeRecoveryCodeConcurrent`、`TestClaimTOTPCounterConcurrent`、`TestClaimTOTPCounterOrdering` 搬到 `twofactor/repository_test.go`（这些只打仓库方法与 `testDB`，不依赖 HTTP 装配），复制 `testDB`/`uniqueTag`/`createTestUser` 精简版（createTestUser 用 ent 直建 + `SetEmailVerificationStatus("not_required")`）。auth 侧这三条本任务删除。

Run: `bash scripts/test.sh ./internal/modules/twofactor/ ./internal/modules/auth/ -run "TestConsumeRecoveryCode|TestClaimTOTPCounter"`
Expected: 双包 PASS（同一行为在新旧两处短暂并存测试后，auth 侧删除）

- [ ] **Step 3: 核对 diff**（不提交）

---

### Task 3: twofactor Service——剪切与自包含助手

**Files:**
- Create: `backend/internal/modules/twofactor/service.go`
- Test: `backend/internal/modules/twofactor/service_test.go`（暂空壳或最小构造测试）

**Interfaces:**
- Consumes: Task 1/2 全部 Produces；`session.Service` 的 `EnterPending2FA(ctx, userID) error`、`PendingUserID(ctx) string`、`PendingAttempts(ctx) int`、`SetPendingAttempts(ctx, int)`、`Establish(ctx, userID string, authVersion int, meta session.Meta) error`、`Destroy(ctx) error`、`RevokeOtherSessions(ctx, userID, currentToken string) error`、`AuthVersion(ctx) int`；`settings.Service` 的 `GetBool`。
- Produces: `type Service struct` 未导出字段（repo、sessions、settings、limiter、policies、totpKey）；`New(db, sessions *session.Service, settingsService *settings.Service, limiter *ratelimit.Limiter, totpKey []byte) *Service`；方法 `Setup(ctx, userID, password string) (TwoFactorSetupResponse, error)`、`Enable(ctx, userID, code, password, currentToken string) (RecoveryCodesResponse, error)`、`Disable(ctx, userID, password, currentToken string) error`、`RegenerateRecoveryCodes(ctx, userID, password string) (RecoveryCodesResponse, error)`、`VerifySecondFactor(ctx, code string, meta session.Meta) (User, error)`；私有助手 `activeUser`、`verifyPassword`、`isFreshTOTPSetup`、`reuseTOTPSetup`、`twoFactorAvailable`、`matchTOTP`、`failSecondFactor`、`maxVerifyAttempts=5`。

- [ ] **Step 1: 剪切 Service**

从 auth `service.go` 剪切 9 个方法 + `isFreshTOTPSetup` + `activeUser` + `recoveryCodesPerBatch` + `maxVerifyAttempts` 到 `twofactor/service.go`；调用点按 Produces 重命名（`s.repo.claimTOTPSetup` 等签名不变；`s.sessions.RenewToken`+`enterPendingSession` 的旧组合已不存在——现版 `VerifySecondFactor` 之前已由 `login.EnterPending2FA` 处理，`VerifySecondFactor` 本身只读 pending，无需改）；`verifyPassword` 从 auth 复制一份为私有副本（auth 侧保留）。`twoFactorAvailable` 继续读 `auth` 组 `totp_enabled`（spec §1）。

- [ ] **Step 2: 最小构造测试（RED→GREEN）**

`service_test.go`：`TestNewRejectsNilSession`——`New(db, nil, settings, ratelimit.New(), key)` panic（`twofactor: session service is required`），非 nil 时构造成功。先写测试看红（函数不存在），实现 `New`（nil panic 守卫照 login 包先例）。

- [ ] **Step 3: 跑包测试**

Run: `bash scripts/test.sh ./internal/modules/twofactor/`
Expected: PASS

- [ ] **Step 4: 核对 diff**（不提交）

---

### Task 4: flow 测试随迁与 auth 收缩

**Files:**
- Create: `backend/internal/modules/twofactor/flow_test.go`（原 `totp_flow_test.go` 主体）、`backend/internal/modules/twofactor/schema_test.go`（原 `totp_schema_test.go`）
- Modify: `backend/internal/modules/auth/service.go`、`backend/internal/modules/auth/repository.go`、`backend/internal/modules/auth/ratelimit.go`、`backend/internal/modules/auth/dto.go`、`backend/internal/modules/auth/module.go`、`backend/internal/modules/auth/controller.go`、`backend/internal/app/app.go`
- Delete: `backend/internal/modules/auth/totp.go`、`backend/internal/modules/auth/secretbox.go`、`backend/internal/modules/auth/totp_test.go`、`backend/internal/modules/auth/secretbox_test.go`、`backend/internal/modules/auth/totp_schema_test.go`、`backend/internal/modules/auth/totp_flow_test.go`
- Test: `backend/internal/modules/auth/`（残留用例归位，见 Step 3）

**Interfaces:**
- Consumes: Task 3 的 `twofactor.Service` 全部方法。
- Produces: `auth.New(db, sessions *session.Service, settingsService *settings.Service, loginService *login.Service, registrationService *registration.Service, twoFactorService *twofactor.Service) *Module`；auth controller 增字段 `twoFactorService *twofactor.Service`；6 条 2FA 路由 handler 薄壳化（绑定 DTO 调 twofactor，`/sessions/current/verify` 成功映射 `twofactor.User` → `UserResponse`）；`auth.Service` 删 `totpKey`/`limiter`/`policies` 字段；auth 保留 `verifyPassword`、`toUser`（含 `TwoFactorEnabled` 映射）、`passwordMinLength`。

- [ ] **Step 1: flow 测试随迁（先搬测试，auth 实现仍在，双包绿）**

`totp_flow_test.go` 除以下留 auth 外整体搬 `twofactor/flow_test.go`（改包名 + helper 就地）：留 auth 的三条——`TestTwoFactorLoginSecondStageLifecycle` 中断言 `me.two_factor_enabled` 的主体拆分（该测试覆盖 登录→二段→me 全链，整体搬 twofactor 但把两条 `me.two_factor_enabled` 断言所在行为复制为 auth 包新用例 `TestMeTwoFactorEnabledFlag`：登录→开 2FA→me 断言 true；外加 `TestTwoFactorUnchangedForDisabledUsers` 的 me 断言部分保留为 `TestMeTwoFactorEnabledFalseForPlainUser`）；`TestTwoFactorLoginClearsSessionMetadata`（sessions_test.go 已有，不重复）。flow 测试依赖的 helper（`loginOK`/`loginRaw`/`logoutOK`/`assertErrorCode`/`decodeJSON`/`meBody`/`verify2FA`/`setup2FA`/`enable2FA`/`wrongCodesFor`/`nextWindowCode`/`fakeTOTPClock`）随迁到 twofactor 包；auth 包若仍有调用方（留下的用例）则 auth 侧保留所需子集。`totp_schema_test.go` 整体搬。

- [ ] **Step 2: auth 实现收缩**

按 Produces 剪切：删 9 方法 + `isFreshTOTPSetup` + `activeUser` + `recoveryCodesPerBatch`；删仓库 6 方法 + `insertRecoveryCodes`（`recoverycode` import 清理）；删 `totp.go`/`secretbox.go`；`ratelimit.go` 删 verify 常量/策略/`verifyAccountKey`（`ipKey` 若无剩余调用方删）；`dto.go` 删 6 个 2FA 类型；`module.go`/`newService` 删 `totpKey`/`limiter`/`policies`；controller 增 `twoFactorService` 字段、6 个 handler 薄壳化；`app.go` 按装配式构造注入。

- [ ] **Step 3: 跑双包 + 全仓**

Run: `bash scripts/test.sh ./internal/modules/twofactor/ ./internal/modules/auth/ -skip TestVerifyEmailRegistrationConcurrent`
Expected: 双包 PASS
Run: `bash scripts/test.sh -skip TestVerifyEmailRegistrationConcurrent`
Expected: 全仓 PASS（app 包的 auth.New 签名调用同步修正）

- [ ] **Step 4: 变异自检三条**

①twofactor `recordAccountFailure` 调用注释掉 → `TestVerifyRateLimitAcrossSessions` 红；②`failSecondFactor` 阈值改 `math.MaxInt` → `TestTwoFactorVerifyLockout` 红；③auth `toUser` 的 `TwoFactorEnabled` 映射删 → `TestMeTwoFactorEnabledFlag` 红。逐条恢复。

- [ ] **Step 5: 核对 diff**（不提交）

---

### Task 5: 装配核对与门禁

**Files:**
- Modify: `backend/internal/app/app.go`（如 Task 4 未完成）、`backend/internal/app/app_test.go`（如有签名引用）

- [ ] **Step 1: 装配单例核对**

`app.go`：`twoFactorService := twofactor.New(db, sessionService, settingsModule.Service, limiter, cfg.TOTPEncryptionKey)` 仅构造一次，`auth.New(..., twoFactorService)` 传入同一实例；grep `twofactor.New` 全仓仅 app.go 一处。

- [ ] **Step 2: 静态门禁与全量**

Run: `bash scripts/check.sh && bash scripts/test.sh`
Expected: `check ok`；全仓 PASS（记录 `TestVerifyEmailRegistrationConcurrent` 实际结果）

- [ ] **Step 3: 核对 diff**（不提交）

---

### Task 6: README / DEVLOG 收尾

**Files:**
- Modify: `backend/README.md`、`docs/DEVLOG.md`

- [ ] **Step 1: README**

模块表加 `internal/modules/twofactor/*` 行（无路由能力模块：TOTP 生命周期、恢复码、二段验证、验证限流；`New` 注入 session/settings/limiter/totpKey）；auth 行删 2FA 与 totp/secretbox 描述；「认证」章节 2FA 小节的模块归属改写（接口契约不变）；装配描述同步。

- [ ] **Step 2: DEVLOG**

顶部倒序插入本轮条目：范围、动机、自包含裁定与代价（bcrypt 校验/销毁会话语义两处各一份）、测试归属（随迁清单与留 auth 清单）、变异自检三条结果、验证、已知缺口（恢复码邮件仍 501、`totp_enabled` 不搬组、`findUserByIdentifier` 留邮箱登录轮）。

- [ ] **Step 3: 全量验证**

Run: `bash scripts/check.sh && bash scripts/test.sh`
Expected: `check ok`，全仓绿（偶发按记录口径）

- [ ] **Step 4: 核对 diff**（不提交）
