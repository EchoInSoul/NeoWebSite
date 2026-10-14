# 两步验证设置复用窗口实施计划（setup 窗口内复用待验证密钥）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans（本机可用；本轮执行方式按项目先例「全程自己实现 + 末尾一次全新审查」）。Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `POST /api/v1/users/me/2fa/setup` 在待验证密钥生成后 **5 分钟**内的重复调用复用同一把密钥（并发调用下"至多一把"不变式成立），超过 5 分钟才重新生成——用户收起再展开设置面板不再换密钥。

**Architecture:** `users` 加一列 `totp_setup_at`（可空 timestamptz，语义 = 当前待验证密钥的生成时刻）。`SetupTOTP` 判定顺序：已启用 409 → 时刻新鲜则**读复用（零写入）** → 否则生成候选并走**原子条件更新认领**（照 `claimTOTPCounter` / `consumeRecoveryCode` 先例）；认领 0 行时**复读并以库内现状为准**（有密钥就复用、无密钥则再认领一次），不做盲写。复用响应由存量密文解密后经 `totp.Generate{Secret: 原始字节}` 重建（含空明文守卫），同一账号名下与首次响应逐字节相同。

**Tech Stack:** Go 1.27 + gin + ent（`sql/lock` feature 已开，本轮不用锁行）+ pquerna/otp v1.5.0 + Atlas + Postgres。**不引入任何新依赖**。

**Spec:** `docs/superpowers/specs/2026-10-14-neoblog-2fa-setup-reuse-window-design.md`（执行者两份都要读；窗口口径、判定顺序、边界在 spec §2/§4）

## Global Constraints

- **开工前先同步 HEAD**：仓库可能有并行会话；提交前 `git diff` 只应包含本任务的改动文件。
- 所有命令的工作目录是 `backend/`（`go test ./internal/...`、`bash scripts/*.sh`、`./scripts/bump-version.sh` 都是相对路径）。
- 本机 Git Bash 先执行：`export PATH="/f/software/go/bin:/f/software/gopath/bin:$PATH" GOPATH='F:/software/gopath' GOCACHE='F:/software/gocache' GOPROXY='https://goproxy.cn,direct' DATABASE_URL='postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog?sslmode=disable' DATABASE_URL_TEST='postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_test?sslmode=disable'`（`db-migrate-apply.sh` 有 `set -u`，两个 URL 必须导出，它自己不读 `.env`）。跑集成测试另 `cd backend && set -a; source .env; set +a`。
- 手写 Go 文件**零注释**；`test -z "$(gofmt -l .)"` 必须成立。
- **零新依赖**：`go.mod` / `go.sum` 一字节不动。
- **不碰 `frontend/`**：本轮不新增、不修改任何前端文件；`frontend/` 下若出现并行会话的未提交改动（会话列表页那批），一律不得 `git add` 进本轮提交。
- **controller / dto / 路由零改动**（响应形状与路径不变）；`guard` 零改动。
- 集成测试：读 `DATABASE_URL_TEST`，缺省 `t.Skip`；测试数据带随机后缀 + `t.Cleanup`；**时间一律经包内 `totpNow` 接缝固定，禁止真实 `time.Sleep` 等 5 分钟**；**用假钟的用例禁止 `t.Parallel`**（会串改全局接缝；现状全包零 `t.Parallel`，保持）。
- 迁移纪律：`atlas migrate diff` 名字面量换为 `totp_setup_at`；无论是否手改 SQL，之后必须 `atlas migrate hash` + `atlas migrate validate`；`db-migrate-apply.sh` 输出须覆盖 `neoblog` 与 `neoblog_test` 两个库。
- 版本：Task 3 执行 `./scripts/bump-version.sh minor`（当前 0.16.0 → 0.17.0；若届时已被其他轮进位，按当前版本顺延，DEVLOG 记实际版本）。
- 提交：按文件清单逐个 `git add`，**禁止 `git add -A`**；是否提交与提交时点按用户当轮点名（默认改动留工作区待检查）。
- 固定值（计划内唯一来源，代码 / 迁移 / 测试必须一致）：列名 `totp_setup_at`；常量 `totpSetupReuseWindow = 5 * time.Minute` 与 `totpSetupClaimAttempts = 2`；仓库方法 `claimTOTPSetup`（并附带 `updateTOTPSetup` 的**删除**——Go 不报未使用方法，删不干净会静默入库）；重建助手 `rebuildTOTPSetup`；新鲜判定助手 `isFreshTOTPSetup`；测试钟助手 `fakeTOTPClock`。

## Review Focus

1. **复用必须零写入**：窗口内第二次调用不得改动行内任何列（密文与 `totp_setup_at` 逐字不变）。→ Task 2 `TestTwoFactorSetupReusesSecretImmediately`
2. **并发至多一把**：同刻 8 个并发调用不得各写各的密钥；认领 0 行的兜底必须是"以库内现状为准"，不得盲写。→ Task 2 `TestTwoFactorSetupConcurrentCallsShareSecret`
3. **边界闭合**：T+5m−1s 复用、**T+5m 重新生成**（`<` 复用 / `>=` 生成），不得差一个刻度。→ Task 2 `TestTwoFactorSetupReusesSecretJustInsideWindow` + `TestTwoFactorSetupRegeneratesAtWindowBoundary`
4. **复用响应必须等于库内待验证密钥**：不能返回密文当明文、不能"重建出来的码验不过"、**空明文必须报错而不是静默换一把**。→ Task 2 用例 1 的 enable 断言 + `TestTwoFactorSetupReuseEmptySecretIsServerError`
5. **disable 清四列、enable 保留时刻**：漏清 `totp_setup_at` 会让下次 setup 误判"新鲜"。→ Task 2 `TestTwoFactorSetupAtClearedByDisable` + `TestTwoFactorSetupAtKeptByEnable`
6. **遗留行与 `IsNil` 分支**：本部署前「设置中」的行（`totp_secret` 非空、`totp_setup_at` 为 NULL）必须照常重新生成。→ Task 2 `TestTwoFactorSetupLegacyPendingWithoutSetupAt`

## 文件结构

| 文件 | 职责 |
| --- | --- |
| `backend/internal/database/ent/schema/user.go`（改） | `users` 加 `totp_setup_at` 字段 |
| `backend/internal/database/ent/**`（生成，提交） | ent 生成代码（`TotpSetupAt`、`SetTotpSetupAt` / `ClearTotpSetupAt`、`TotpSetupAtIsNil` / `LTE` 谓词） |
| `backend/scripts/db-migrate-diff.sh`（改，`:7`） | 迁移名字面量 `totp_last_counter` → `totp_setup_at` |
| `backend/migrations/<ts>_totp_setup_at.sql`（新） | 单条 ADD COLUMN |
| `backend/internal/modules/auth/totp.go`（改） | 常量 `totpSetupReuseWindow` + 重建助手 `rebuildTOTPSetup`（含空明文守卫） |
| `backend/internal/modules/auth/repository.go`（改） | `claimTOTPSetup`（条件更新，含落时刻）、**删除** `updateTOTPSetup`（唯一调用点被取代）、`disableTOTP` 清时刻 |
| `backend/internal/modules/auth/service.go`（改，`SetupTOTP` `:256`） | 新鲜判定 / 读复用 / 认领 / 复读再认领 |
| `backend/internal/modules/auth/totp_schema_test.go`（改） | 迁移后置断言（**独立用例 `TestTOTPSetupAtColumn`**：可空读写/清空） |
| `backend/internal/modules/auth/totp_flow_test.go`（改） | 10 条新用例（含仓库级 `TestClaimTOTPSetupRejectedWhenFresh`）+ 2 条既有用例调整 + 测试钟助手 |
| `backend/README.md`、`docs/DEVLOG.md`、`backend/internal/version/version.go`（改） | 文档、开发日志、版本 |

---

### Task 1: ent 列 `totp_setup_at` + 迁移 + 后置断言

**Files:**
- Modify: `backend/internal/database/ent/schema/user.go`（在 `totp_last_counter` 行后加一行）
- Modify: `backend/scripts/db-migrate-diff.sh:7`
- Create: `backend/migrations/<时间戳>_totp_setup_at.sql`
- Modify: `backend/internal/modules/auth/totp_schema_test.go`

**Interfaces:**
- Consumes: 无（起点任务）。
- Produces: `ent.User.TotpSetupAt`（`*time.Time`）、更新构造器 `SetTotpSetupAt(time.Time)` / `ClearTotpSetupAt()`、谓词 `user.TotpSetupAtIsNil()` / `user.TotpSetupAtLTE(time.Time)`；迁移 `<ts>_totp_setup_at.sql`（两库已应用）。

- [ ] **Step 1: 改 schema**

`user.go` 的 Fields 里 `totp_last_counter` 行后追加：`field.Time("totp_setup_at").Optional().Nillable(),`（与既有 `totp_enabled_at` 同写法；不加索引）。

- [ ] **Step 2: 重生成**

Run: `bash scripts/db-generate.sh`
Expected: 退出码 0；生成物里出现 `TotpSetupAt`（`user.go` / `user_create.go` / `user_update.go` / `user_query.go` / `where.go`）。

- [ ] **Step 3: 无库验证**

Run: `go build ./... && go vet ./... && test -z "$(gofmt -l .)"` → 全静默。
Run: `go test ./...`（不导出 `DATABASE_URL_TEST`）→ 全绿（集成用例缺省 skip）。

- [ ] **Step 4: 生成迁移并审阅 SQL**

`scripts/db-migrate-diff.sh` 第 7 行字面量 `totp_last_counter` → `totp_setup_at`（只换字符串）。
Run: `bash scripts/db-migrate-diff.sh`
Expected: `backend/migrations/` 出现 `<ts>_totp_setup_at.sql`，`atlas.sum` 更新。

审阅：内容**恰好**是一句 `ALTER TABLE "public"."users" ADD COLUMN "totp_setup_at" timestamptz NULL;`；**没有** NOT NULL、DEFAULT、回填 UPDATE、索引、数据语句。不符则人工修正 SQL（改过就必须重跑 hash）。

- [ ] **Step 5: hash + validate + 应用两库**

Run: `atlas migrate hash && atlas migrate validate` → 干净。
Run: `bash scripts/db-migrate-apply.sh` → 输出里 `neoblog` 与 `neoblog_test` 两个库都应用成功。

- [ ] **Step 6: 后置断言（`totp_schema_test.go` 新增独立用例 `TestTOTPSetupAtColumn`，形状对照既有 `TestTOTPLastCounterColumn`）**

三段断言：不带值建用户读回默认 nil；设值读回一致；Clear 后回 nil：

```go
setupAt := time.Now().UTC().Truncate(time.Microsecond)
if _, err := user.Update().SetTotpSetupAt(setupAt).Save(ctx); err != nil {
	t.Fatalf("set totp_setup_at: %v", err)
}
// 重查后：fetched.TotpSetupAt != nil 且 fetched.TotpSetupAt.Equal(setupAt)
// 再 user.Update().ClearTotpSetupAt() 后重查：TotpSetupAt == nil
```

断言一律用 `.Equal()` 比较时刻（时区/单调钟），不用 `==`。

- [ ] **Step 7: 带库验证**

Run: `bash scripts/test.sh ./internal/modules/auth/`（已 source `.env`）→ 全绿（此时行为未改，既有 2FA 用例应全绿）。

- [ ] **Step 8: Commit**

```bash
git add backend/internal/database backend/scripts/db-migrate-diff.sh backend/migrations backend/internal/modules/auth/totp_schema_test.go
git commit -m "feat(backend): users 加 totp_setup_at（两步验证设置复用窗口的数据插槽）"
```

---

### Task 2: 复用窗口实现 + 集成用例 + 变异自检

**Files:**
- Modify: `backend/internal/modules/auth/totp.go`（常量 + 重建助手）
- Modify: `backend/internal/modules/auth/repository.go`（`claimTOTPSetup` 新增；**删除** `updateTOTPSetup` `:110`；`disableTOTP` `:139` 清时刻）
- Modify: `backend/internal/modules/auth/service.go`（`SetupTOTP` `:256`）
- Test: `backend/internal/modules/auth/totp_flow_test.go`

**Interfaces:**
- Consumes: Task 1 的 `ent.User.TotpSetupAt` 与谓词。
- Produces: `const totpSetupReuseWindow = 5 * time.Minute`、`const totpSetupClaimAttempts = 2`；`func rebuildTOTPSetup(accountName, base32Secret string) (totpSetup, error)`；`func isFreshTOTPSetup(row *ent.User, now time.Time) bool`；`func (r *repository) claimTOTPSetup(ctx context.Context, id uuid.UUID, encryptedSecret string, now time.Time) (bool, error)`；**删除** `func (r *repository) updateTOTPSetup`（无替代者，认领路径接管；删后 `errUserNotFound` 仍有 7 处使用，不引孤儿）；服务侧复用助手 `func (s *Service) reuseTOTPSetup(accountName, sealed string) (TwoFactorSetupResponse, error)`。

- [ ] **Step 1: 写失败测试（10 条新用例 + 2 条既有用例调整 + 测试钟助手）**

`totp_flow_test.go` 顶部助手区新增（包内 `scs` 需补 import）：

```go
func fakeTOTPClock(t *testing.T, base time.Time) func(time.Time) {
	t.Helper()
	previous := totpNow
	now := base
	totpNow = func() time.Time { return now }
	t.Cleanup(func() { totpNow = previous })
	return func(at time.Time) { now = at }
}
```

各用例骨架（断言即验收口径，命名与 spec §7 一一对应；固定基准时刻用 `base := time.Unix(1700000001, 0)`，窗口 `W := totpSetupReuseWindow`）：

1. `TestTwoFactorSetupReusesSecretImmediately`：`fakeTOTPClock` → 登录 → `first := setup2FA` → `before := userRow` → `second := setup2FA` → `after := userRow`。断言：`second.Secret == first.Secret && second.OTPAuthURL == first.OTPAuthURL`；`*after.TotpSecret == *before.TotpSecret`（密文逐字不变）且 `after.TotpSetupAt.Equal(*before.TotpSetupAt)`；最后 `enable2FA(t, client, ts.URL, first.Secret)` 成功（响应 = 库内待验证密钥）。
2. `TestTwoFactorSetupReusesSecretJustInsideWindow`：`advance := fakeTOTPClock(t, base)`；setup → `advance(base.Add(W - time.Second))` → setup → 两次 secret 相等。
3. `TestTwoFactorSetupRegeneratesAtWindowBoundary`：setup → `advance(base.Add(W))` → setup → secret **不同**；`row.TotpSetupAt.Equal(base.Add(W))`；旧 secret 当前窗码 enable → 401 `two_factor_invalid_code`（`assertErrorCode`）；新 secret `enable2FA` 成功。
4. `TestTwoFactorSetupConcurrentCallsShareSecret`（服务层，照 `TestClaimTOTPCounterConcurrent` 形状）：`fakeTOTPClock(t, base)` → `svc := newService(db, scs.New(), testTOTPKey, nil)`（`SetupTOTP` 不触 settings，传 nil）→ 8 个 goroutine `svc.SetupTOTP(context.Background(), u.ID)` → `wg.Wait()` 后断言全部 err 为 nil 且 8 个 `Secret` 全部相等；再 `openSecret(testTOTPKey, *userRow(t, db, id).TotpSecret)` 等于该 secret（至多一把不变式）。
5. `TestTwoFactorSetupAtClearedByDisable`：setup → 先断言 `userRow(...).TotpSetupAt != nil`（防空洞断言）→ `post2FA(..., "disable", TwoFactorDisableRequest{Password: password})` 200 → `TotpSetupAt == nil` 且 `TotpSecret == nil`。
6. `TestTwoFactorSetupAtKeptByEnable`：setup → 先断言 `before := userRow(...).TotpSetupAt` 非 nil → `enable2FA` → `TotpSetupAt.Equal(*before)`。
7. `TestTwoFactorSetupLegacyPendingWithoutSetupAt`：setup → `db.Client.User.UpdateOneID(id).ClearTotpSetupAt().Exec(ctx)`（手工模拟遗留「设置中」行）→ 同刻 setup → secret 不同且 `TotpSetupAt != nil`。
8. `TestTwoFactorSetupReuseDecryptFailureIsServerError`：setup → 把明文 base32 直接塞进列（照 `TestTwoFactorDecryptFailureIsServerError:794` 的手法：`UpdateOneID(id).SetTotpSecret(setup.Secret).Exec(ctx)`）→ 同刻 setup → 500 `internal_error`；`userRow(...).TotpSecret` 仍等于刚塞的明文值（未被替换成新密文）。
9. `TestTwoFactorSetupReuseEmptySecretIsServerError`：setup → `sealed, _ := sealSecret(testTOTPKey, "")` 后 `UpdateOneID(id).SetTotpSecret(sealed).Exec(ctx)`（合法密文、明文为空串）→ 同刻 setup → 500 `internal_error` 且列值未被替换。
10. `TestTwoFactorSetupOverwritesSecret`（改）：开头 `advance := fakeTOTPClock(t, base)`，第二次 `setup2FA` 前 `advance(base.Add(W + time.Second))`；其余断言不动。
11. `TestTwoFactorDisableLifecycle`（改）：结尾补 `if row.TotpSetupAt != nil { t.Fatalf(...) }`。
12. `TestClaimTOTPSetupRejectedWhenFresh`（仓库级）：同刻第一次 `claimTOTPSetup` 成 → 同刻第二次必拒 → 钟推进过窗后认领必成（变异②的唯一确定性钉子）。

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/auth/ -run 'TestTwoFactorSetup|TestTwoFactorDisable' -count=1`
Expected: 1–9 全红（复用分支不存在 → 每次 setup 都换密钥；`TotpSetupAt` 无写入 → nil 断言失败）；10、11 此刻仍是绿的（各自前提未变），确认无误后再进 Step 3。

- [ ] **Step 3: 实现**

`totp.go`：与 `totpPeriodSeconds` 并列加 `const totpSetupReuseWindow = 5 * time.Minute`（**独立常量，禁止复用它**）；助手（`encoding/base32` + 既有 `totp`/`otp` import）：

```go
func rebuildTOTPSetup(accountName, base32Secret string) (totpSetup, error) {
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(base32Secret)
	if err != nil {
		return totpSetup{}, fmt.Errorf("decode totp secret: %w", err)
	}
	if len(raw) == 0 {
		return totpSetup{}, fmt.Errorf("totp secret is empty")
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: accountName,
		Period:      totpPeriodSeconds,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
		Secret:      raw,
	})
	if err != nil {
		return totpSetup{}, fmt.Errorf("rebuild totp key: %w", err)
	}
	return totpSetup{Secret: key.Secret(), OTPAuthURL: key.URL()}, nil
}
```

（空明文守卫必须显式写，用 `fmt.Errorf` 而非 `errors.New`——`totp.go` 现有 import 块没有 `errors`，不为一行错误新增 import；语义同"静默换钥"：`pquerna/otp` 在 `len(opts.Secret) == 0` 时会随机生成新密钥。）

`repository.go`：**删除** `updateTOTPSetup`（其唯一调用点在下面的 service 改写里被认领路径取代；Go 不报未使用方法，必须手动删，删后 `errUserNotFound` 仍有 7 处使用、不留孤儿）；`disableTOTP` 的更新链追加 `.ClearTotpSetupAt()`；新增：

```go
func (r *repository) claimTOTPSetup(ctx context.Context, id uuid.UUID, encryptedSecret string, now time.Time) (bool, error) {
	claimed, err := r.client.User.Update().
		Where(
			user.IDEQ(id),
			user.TotpEnabledAtIsNil(),
			user.Or(user.TotpSetupAtIsNil(), user.TotpSetupAtLTE(now.Add(-totpSetupReuseWindow))),
		).
		SetTotpSecret(encryptedSecret).
		SetTotpSetupAt(now).
		ClearTotpLastCounter().
		Save(ctx)
	if err != nil {
		return false, err
	}
	return claimed == 1, nil
}
```

`service.go`：`SetupTOTP` 按 spec §4.2 改写（`isFreshTOTPSetup` 判定 + 认领 0 行复读、以库内现状为准、最多两次认领、末路 500）：

```go
func isFreshTOTPSetup(row *ent.User, now time.Time) bool {
	return row.TotpSecret != nil && row.TotpSetupAt != nil && now.Sub(*row.TotpSetupAt) < totpSetupReuseWindow
}

func (s *Service) SetupTOTP(ctx context.Context, userID string) (TwoFactorSetupResponse, error) {
	// 1) activeUser → 2) 已启用 409 → 3) now := totpNow()
	// 4) isFreshTOTPSetup → return s.reuseTOTPSetup(found.Email, *found.TotpSecret)
	// 5) candidate := generateTOTPSetup(found.Email); sealed := sealSecret(...)
	// 6) for attempt := 0; attempt < totpSetupClaimAttempts; attempt++ {
	//        claimTOTPSetup(...)；成功 → 返回候选
	//        复读 s.activeUser；行没了 → 401；已启用 → 409；row.TotpSecret != nil → s.reuseTOTPSetup(row.Email, *row.TotpSecret)
	//        （库内无密钥 → 循环再认领一次）
	//    }
	// 7) return TwoFactorSetupResponse{}, apierror.ErrInternal
}
```

```go
func (s *Service) reuseTOTPSetup(accountName, sealed string) (TwoFactorSetupResponse, error) {
	plain, err := openSecret(s.totpKey, sealed)
	if err != nil {
		return TwoFactorSetupResponse{}, apierror.ErrInternal
	}
	setup, err := rebuildTOTPSetup(accountName, plain)
	if err != nil {
		return TwoFactorSetupResponse{}, apierror.ErrInternal
	}
	return TwoFactorSetupResponse{Secret: setup.Secret, OTPAuthURL: setup.OTPAuthURL}, nil
}
```

`codeAt` / `currentWindow` / `enable2FA` 等既有助手不动（它们经 `totpNow` 取值，自动跟随固定钟）。

- [ ] **Step 4: 跑测试确认全绿**

Run: `bash scripts/test.sh ./internal/modules/auth/ -count=1` → 全绿（既有 `TestTwoFactor*` 共 **19 条**：`totp_flow_test.go` 18 + `sessions_test.go` 1；另有 claim/consume 并发 3 条、原语 7 条、迁移断言 2 条）。
Run: `go build ./... && go vet ./... && test -z "$(gofmt -l .)"` → 全静默。

- [ ] **Step 5: 变异自检（防假把式，八处改坏→确认目标用例红→改回）**

**判定口径**：目标用例必须红；**连带红属预期**（逐条记下实际红了哪几个），改回后全包绿即算通过——严禁为了让"其它用例不红"去改测试或往代码里加补丁。

| 改坏点 | 期望变红（目标 + 已知连带） |
| --- | --- |
| ① 复用条件 `now.Sub(*TotpSetupAt) < totpSetupReuseWindow` 改 `<=` | {3}（5 分钟边界） |
| ② `claimTOTPSetup` 里 `user.Or(TotpSetupAtIsNil(), TotpSetupAtLTE(...))` 条件整段删掉 | {4}（并发） |
| ③ `disableTOTP` 去掉 `.ClearTotpSetupAt()` | {5, 11}（两处都断 disable 后为 nil） |
| ④ 删掉读路径复用分支（总是生成） | {1, 2, 8, 9}（8/9 的 500 正是从复用分支来的） |
| ⑤ `enableTOTP` 误加 `.ClearTotpSetupAt()` | {6} |
| ⑥ `isFreshTOTPSetup` 把 `TotpSetupAt == nil` 也当新鲜 | {7}（唯一有区分度的钉法：遗留行走复用、返回旧密钥） |
| ⑦ `claimTOTPSetup` 谓词里删掉 `user.TotpSetupAtIsNil()` | 几乎全包 {1–11} + 多数既有用例（首次 setup 都落在 `setup_at` 为 NULL 的行上 → 两次认领 0 行 → 500）；只作"该防线会被抓住"的确认，区分度靠 ⑥ |
| ⑧ `rebuildTOTPSetup` 删掉空明文守卫 | {9}（`pquerna/otp` 静默随机生成，响应 ≠ 原密钥） |

每处改坏后跑**整包**：`bash scripts/test.sh ./internal/modules/auth/ -run 'TestTwoFactor|TestClaimTOTPCounter|TestConsumeRecoveryCode' -count=1`，逐条记下红了哪几个（对照上表），然后改回并复跑确认全包绿。结论（八处 × 期望集合 × 实测集合）记入 Task 3 的 DEVLOG。

- [ ] **Step 6: Commit**

```bash
git add backend/internal/modules/auth/totp.go backend/internal/modules/auth/repository.go backend/internal/modules/auth/service.go backend/internal/modules/auth/totp_flow_test.go
git commit -m "feat(backend): 两步验证 setup 5 分钟窗口内复用待验证密钥"
```

---

### Task 3: README、DEVLOG、版本进位与终验

**Files:**
- Modify: `backend/README.md`（`:143` 迁移清单 / `:177` setup 行 / `:179` disable 行 / `:196` 两步验证总述行）
- Modify: `docs/DEVLOG.md`（倒序插开头）
- Modify: `backend/internal/version/version.go` + `backend/README.md` 当前版本行（由脚本连带）

- [ ] **Step 1: README**

- `:143` 迁移清单末尾追加：`→ totp_setup_at`（`users` 加一列可空 timestamptz：当前待验证密钥的生成时刻，setup 5 分钟复用窗口用，纯 DDL 零数据）。
- `:177` setup 行：把"重复调用覆盖旧 secret（旧码作废、已认领窗号归零）"改写为「**生成后 5 分钟内的重复调用复用同一把密钥（同一账号名下响应逐字节相同、库内零写入；窗口内多设备/多标签页也拿到同一把）；超过 5 分钟重新生成并覆盖旧 secret（旧码作废、已认领窗号归零）**」；"明文 secret 只回这一次"改为"明文 secret 仅经本端点暴露（首发与窗口内复用均可能返回明文）"；补一句复用路径解密/空明文失败 → 500 `internal_error`。
- `:179` disable 行："清空 secret / 启用时间 / 窗号" → "清空 secret / 启用时间 / 窗号 / **设置时刻**"。
- `:196` 两步验证总述行：指路加本轮 spec（`docs/superpowers/specs/2026-10-14-neoblog-2fa-setup-reuse-window-design.md`）。

- [ ] **Step 2: DEVLOG 条目（`## 2026-10-14 · 两步验证 setup 复用窗口（0.17.0）`，倒序插在文件头）**

内容：动机（用户症状 + 两处根因）、窗口口径（**5 分钟**由用户裁定，附"30 秒盖不住扫码节奏、症状会复发"的否决理由；固定窗、从生成时刻起算、边界 5 分钟整重生成）、`claimTOTPSetup` 原子认领与互斥依据（Read Committed 重算谓词）、认领失败的"以库内现状为准"兜底与两次上限、`updateTOTPSetup` 的删除（认领路径接管的死方法清理）、迁移名、测试面（9 新 + 2 改）、**变异自检八处结论（目标集合 / 连带集合逐条记）**、活体 curl 证据（复用同刻同钥 + 探针把 `totp_setup_at` 拨回 6 分钟前验"陈旧换新"，**不等 5 分钟**）、已知缺口指路（spec §9）。术语用「setup 复用窗口」，可注一句"用户口语称防抖"。

- [ ] **Step 3: 版本进位**

Run: `./scripts/bump-version.sh minor` → `0.17.0`（`version.go` + README 当前版本行；若已被其他轮进位则顺延并记实际值）。

- [ ] **Step 4: 终验**

Run: `go build ./... && go vet ./... && test -z "$(gofmt -l .)"` → 干净。
Run: `bash scripts/test.sh ./...`（带库）→ 全绿。
Run: `git status --short go.mod go.sum` → 空（依赖未动）；`git status --short` 里 `frontend/` 的改动全部属于并行会话，本轮任何一次提交都只 `git add` 本节清单里的后端与文档文件。
Run: `grep -rn "totpSetupReuseWindow\|claimTOTPSetup\|isFreshTOTPSetup" backend/internal/modules/auth/*.go | grep -v _test.go` → 实现点齐全（totp.go / repository.go / service.go）。
Run: `grep -rn "updateTOTPSetup" backend/` → **无输出**（死方法已删、不留痕）。
Run: 活体 curl 全链路（起 `bash scripts/dev.sh` 后端，或对本地起好的实例）：登录 → setup 取 A → 立刻再 setup 取 B（须 A == B，复用零写入）→ 用 `backend/.scratch` 的 Go 探针把该行 `totp_setup_at` 拨回 **6 分钟前**（照既有"跑 SQL 走 .scratch + pgx"手法）→ setup 取 C（须 C != B）→ 用 C 的码 enable 200。**不阻塞等 5 分钟**——边界已由用例 3 用假钟钉死，活体只证"同刻同钥 + 陈旧换新"。

- [ ] **Step 5: Commit**

```bash
git add backend/README.md
git commit -m "docs(backend): 两步验证 setup 5 分钟复用窗口口径（README 四处）"
git add docs/DEVLOG.md
git commit -m "docs(backend): DEVLOG 记录 setup 复用窗口轮（含变异自检结论）"
git add backend/internal/version/version.go backend/README.md
git commit -m "chore(backend): 版本进位 0.17.0（两步验证 setup 复用窗口）"
```

---

## 收尾：终审与修复

全部任务完成后做一次整分支终审（fresh context 只读子代理），对照 Review Focus 六项 + spec §10 验收标准逐条核对，重点复核：

- 复用路径确实零写入（读代码路径 + 用例 1 的密文逐字断言）；
- 复用响应与被 enable 的密钥同源（用例 1 的 enable 断言），空明文走报错而非静默换钥（用例 9）；
- 认领 0 行的兜底以库内现状为准（无盲写）、认领至多两次、末路不写库；
- `>= W` 生成 / `< W` 复用，边界不多不少一个刻度；
- `disable` 清四列、`enable` 不动 `totp_setup_at`、遗留行（`setup_at` 为 NULL）照常重新生成；
- `updateTOTPSetup` 已删除、全仓 grep 无引用（不留死方法）；
- 无库/有库两套测试全绿，`frontend/` 与 `go.mod`/`go.sum` 零 diff。

发现缺陷先复现再修，修复按文件清单分别 `git add`（禁止 `git add -A`）；**是否提交、提交时点由用户点名**。审查结论（含变异自检八处结果）报给用户，修补条目等用户点名。
