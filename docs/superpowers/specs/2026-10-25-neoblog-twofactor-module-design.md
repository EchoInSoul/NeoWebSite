# 两步验证拆成独立能力模块 设计

日期：2026-10-25
状态：待审阅
前序：2026-10-25 人机验证设置组归位与自检路由迁移；登录/注册/会话拆分轮（session 模块已有 pending 2FA 会话能力）

## 1. 背景与目标

auth 包历经 captcha、login、registration、session 四次拆分后，最大的一块残留耦合是**两步验证（TOTP/2FA）**：实现（`totp.go` 138 行、`secretbox.go` 57 行）、6 个仓库方法、6 条路由、DTO 约一半、整个 `verifyPolicies` 限流、以及从 config 穿三层的 `totpKey` 加密钥匙，全部长在 auth 里。本轮把 2FA 拆成独立能力模块 `internal/modules/twofactor`，auth 收缩为账号与会话管理（me / 资料 / 改密 / 登出 / 会话列表）。

### 范围外（明确不做）

- `auth.totp_enabled` 设置键**不搬组**：它是 auth 域的站级开关（与密码策略、会话有效期同组），搬了要再变一轮公开设置形状，收益小。
- 路由路径、错误码、限流阈值、响应形状**零变化**。
- 恢复码邮件接口仍是 501 占位（随 handler 薄壳迁移，不实现）。
- 前端零改动。
- `findUserByIdentifier` 双用途（邮箱登录轮）。

## 2. 新模块 `internal/modules/twofactor`

无路由能力模块（STYLE.md §1），文件结构：

| 文件 | 内容 | 来源 |
| --- | --- | --- |
| `service.go` | `Setup` / `Enable` / `Disable` / `RegenerateRecoveryCodes` / `VerifySecondFactor` + 私有助手 | 从 auth/service.go 剪切 |
| `repository.go` | users 表 totp 列 6 方法（claimSetup / enable / disable / claimCounter）+ recovery_codes 表 3 方法（insert 批量 / replace / consume） | 从 auth/repository.go 剪切 |
| `totp.go` | TOTP 生成 / 重建 / 窗匹配 / 恢复码生成与哈希 / `totpNow` 变量 | auth/totp.go 原样搬 |
| `secretbox.go` | secret 加解密 | auth/secretbox.go 原样搬 |
| `ratelimit.go` | 验证限流：失败锁定（10 次 / 15 分钟窗 / 15 分钟冷却）+ IP 窗口（30 次 / 15 分钟），key 生成 `verify:user:<id>` 与 `verify:ip:<ip>` | 从 auth/ratelimit.go 剪切 |
| `dto.go` | `TwoFactorCodeRequest` / `TwoFactorSetupRequest` / `TwoFactorEnableRequest` / `TwoFactorDisableRequest` / `TwoFactorSetupResponse` / `RecoveryCodesResponse` | 从 auth/dto.go 剪切 |

### 2.1 构造与依赖

```go
func New(db *database.Database, sessions *session.Service, settingsService *settings.Service, limiter *ratelimit.Limiter, totpKey []byte) *Service
```

- pending 2FA 会话直接用共享 session 模块既有方法：`EnterPending2FA` / `PendingUserID` / `PendingAttempts` / `SetPendingAttempts` / `Establish` / `Destroy` / `RevokeOtherSessions` / `AuthVersion`——零新增共享能力。
- `totpKey` 从 `app.New` 直接注入 twofactor（不再经 auth 转手）。
- 服务方法对 外签名与现 auth 版一致（`Setup(ctx, userID, password)`、`Enable(ctx, userID, code, password, currentToken)`、`Disable(ctx, userID, password, currentToken)`、`RegenerateRecoveryCodes(ctx, userID, password)`、`VerifySecondFactor(ctx, code, meta session.Meta)`），返回类型用 twofactor 自己的 DTO。
- `totpNow` 包级变量随迁（测试替钟用）。

### 2.2 自包含裁定（共享动作在包内各有一份）

`Setup` / `Enable` / `Disable` / `RegenerateRecoveryCodes` 开头都要「读活跃用户（已删则销毁会话并 401）+ bcrypt 验密码」；`Enable` / `Disable` 还要「吊销其他会话」「读/写 auth version」。这些实现为 twofactor 包内私有助手（`activeUser`、`verifyPassword`，逻辑照抄 auth 现版；`activeUser` 整体随迁，`verifyPassword` 为副本），**不从 auth 导出，auth 也不向 twofactor 出接口**。twofactor 的依赖仅 session、settings、ent、common。

**已知代价（记档）**：bcrypt 密码校验与「用户已删销毁会话」语义在 auth（ChangePassword 的 `verifyPassword`）与 twofactor（自备副本）两处各一份，将来改语义需两处同步。

### 2.3 登录成功响应形状

`VerifySecondFactor` 现返回 auth 的 `User`（Email/Nickname/Bio）。拆出后返回 twofactor 自己的 `User` 值（同三字段，无 ID），auth 的 controller 薄壳直接映射到既有 `UserResponse`——响应 JSON 不变。

## 3. auth 侧收缩

### 3.1 删除清单

- `service.go`：9 个 2FA 方法（SetupTOTP / reuseTOTPSetup / EnableTOTP / DisableTOTP / RegenerateRecoveryCodes / twoFactorAvailable / VerifySecondFactor / matchTOTP / failSecondFactor）+ `isFreshTOTPSetup` + `activeUser`（auth 侧无其他调用方，随迁）+ `recoveryCodesPerBatch` 常量。`verifyPassword` **保留在 auth**（`ChangePassword` 仍用），twofactor 包内另备一份私有副本（见 2.2）。
- `repository.go`：6 个 2FA 仓库方法（claimTOTPSetup / enableTOTP / disableTOTP / replaceRecoveryCodes / consumeRecoveryCode / claimTOTPCounter）+ `insertRecoveryCodes`。`toUser` 保留（`TwoFactorEnabled: u.TotpEnabledAt != nil` 是纯字段映射，供 `me` 接口，不构成对 2FA 模块依赖）。
- `totp.go`、`secretbox.go` 及其测试文件整体删除（随迁）。
- `ratelimit.go`：`verifyPolicies` / `defaultVerifyPolicies` / `verifyAccountKey` / 2FA 专属常量删除；`ipKey` 若仅剩 2FA 用则一并删。`Service` 的 `policies` 字段与 `limiter` 字段随之删除（auth 不再有限流）。
- `dto.go`：6 个 2FA 请求/响应类型剪切。
- `module.go` / `newService`：`totpKey`、`limiter`、`policies` 参数与字段删除。

### 3.2 controller 薄壳

6 条路由路径不变，仍由 auth 的 `controller.registerProtected` 声明，handler 改为绑定请求体后调用注入的 `*twofactor.Service`：

```go
type controller struct {
	service             *Service
	loginService        *login.Service
	registrationService *registration.Service
	twoFactorService    *twofactor.Service
}
```

`/sessions/current/verify` 成功后映射 `twofactor.User` → `UserResponse`（`me` 与登录响应同形）。`/users/me/2fa/recovery-codes/email` 的 501 占位随薄壳保留。

`TwoFactorEnabled`（`me` 接口）：`MeResponse` 与 auth `User` 结构不动，取值仍走 auth repository 的 `toUser`。

### 3.3 装配

`app.go`：

```go
twoFactorService := twofactor.New(db, sessionService, settingsModule.Service, limiter, cfg.TOTPEncryptionKey)
authModule := auth.New(db, sessionService, settingsModule.Service, loginService, registrationService, twoFactorService)
```

`cfg.TOTPEncryptionKey` 直达 twofactor；auth 的 `New` 去掉 `totpKey` / `limiter`。

## 4. 测试归属与迁移

### 4.1 整体随迁 twofactor 包

- `totp_flow_test.go`（34 条）——适配新装配 helper。
- `totp_test.go`（9）、`totp_schema_test.go`（3）、`secretbox_test.go`（5）——纯单测，改包名即走。
- `ratelimit_test.go` 中 verify 相关 4 条（AcrossSessions / IPGate / ResetsOnSuccess / ConcurrentAcrossSessions）。
- `settings_consumers_test.go` 中 2FA 相关 6 条（SetupTOTPBlocked / EnableTOTPBlocked / SecondFactorVerifyAppliesSessionMaxAge / TOTPSetupFailsWhenSettingsUnavailable / DisableTOTPStillWorks / RecoveryCodesStillWorkWhenSiteSwitchOff）。

### 4.2 留在 auth 包

- `TestLoginStillRequiresSecondFactorWhenSiteSwitchOff`——跨 login 与 2FA 的集成行为。
- `sessions_test.go` 的 `TestTwoFactorLoginClearsSessionMetadata`——验证 pending 会话键清理，属 session 语义。
- `totp_flow_test.go` 中断言 `me.two_factor_enabled` 的两条（`TwoFactorEnabled` 映射的守护用例，变异自检 #3 的靶子）——它们是 me 接口行为，若其所属测试主体随迁，则把断言部分留在 auth 包或整条留下（实现时按测试主体归属定，断言必须存在于 auth 包）。
- 登录分叉、改密吊销会话等用例。

### 4.3 测试装配

twofactor 包自建 `testDB` / `uniqueTag` / `newTestApp` 精简版：engine 挂 `/seed` 直种会话（captcha 轮先例）+ auth 模块公开/保护路由（登录建会话走真路由，flow 测试大量依赖 `loginOK`）。两包测试文件去重后各自持有 helper，不建共享 testutil（STYLE.md §7）。

### 4.4 变异自检（防假绿，至少三条）

1. twofactor 的 `recordFailure` 调用删除 → `TestVerifyRateLimit*` 红。
2. `failSecondFactor` 的锁定阈值改成超大 → 锁定用例红（`two_factor_locked` 不出现）。
3. auth `toUser` 的 `TwoFactorEnabled` 映射删掉 → `me` 接口断言 `two_factor_enabled` 的用例红（现读 auth 测试确认有此断言；无则补一条）。

## 5. 文档

- `backend/README.md`：模块表加 twofactor 行、auth 行瘦身（去 2FA 与 totp/secretbox 描述）；「认证」章节 2FA 小节的模块归属改写（接口契约本身不变）。
- `docs/DEVLOG.md`：本轮条目（动机、自包含裁定与代价、测试归属、验证、已知缺口——恢复码邮件仍 501、`totp_enabled` 不搬组）。

## 6. 验证

- `go build ./...`、`go vet ./...`、`gofmt -l` 干净；`scripts/check.sh` → `check ok`。
- `scripts/test.sh` 全仓绿（既有偶发 `TestVerifyEmailRegistrationConcurrent` 按在案口径处理）。
- 前端零改动，无需 tsc。
