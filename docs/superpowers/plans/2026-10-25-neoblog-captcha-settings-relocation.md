# 人机验证设置组归位与自检路由迁移实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** captcha 5 个设置键从 `auth` 组拆出独立 `captcha` 组；`POST /admin/captcha/verifications` 从 auth 搬进 captcha 模块；新增 `system.captcha.view/edit` 权限对；前端读点与权限矩阵同步。

**Architecture:** 设置组在 `settings/registry.go` 声明、迁移搬数据；captcha 模块从无路由能力模块升级为有路由模块（补 module/controller/dto），auth 与 captcha 彻底解耦；权限三路分流（settings 组接口）+ rbac 目录新对；前端只换 URL 与组名字段，形状零变化。

**Tech Stack:** Go 1.27 + gin + ent + Atlas + PostgreSQL；Next.js 16 + React 19 + next-intl。

**Spec:** [2026-10-25-neoblog-captcha-settings-relocation-design.md](../specs/2026-10-25-neoblog-captcha-settings-relocation-design.md)

## Global Constraints

- **绝不提交。** 每个任务结束只跑测试、核对 diff，改动留在工作区等用户检查。不要 `git add` / `git commit` / `git push`。
- **全仓零人工注释**：Go、TS、TSX 都不写解释性注释。
- **禁止兜底/降级**：失败按错误上抛。
- 不做（spec §1）：go-captcha 实现、`findUserByIdentifier` 拆分、自建组补授脚本。`builtin` 仍是"选了不拦"的现状。
- 登录/注册设置页（`use-login-settings-save`、`use-register-settings-save`、`admin-settings.ts` 的 register 读取）继续读写 `auth` 组，本轮不碰。
- 后端测试起手式（串行，不加 `-race`，不双开打同一个 `neoblog_test`）：
  ```bash
  cd backend
  export PATH="/f/software/go/bin:/f/software/gopath/bin:$PATH"
  export GOPROXY=https://goproxy.cn,direct
  set -a; source .env; set +a
  bash scripts/test.sh ./internal/...
  ```
- 静态门禁：`cd backend && bash scripts/check.sh`。
- 前端验证：`cd frontend && npx tsc --noEmit`；改动文件 `npx eslint <files>`、`npx prettier --check <files>`。浏览器验收用 `localhost`，不用 `127.0.0.1`。
- 手写迁移时：改完跑 `atlas migrate hash --dir "file://migrations"` 与 `atlas migrate validate --dir "file://migrations"`，再 `bash scripts/db-migrate-apply.sh`（先 `set -a; source .env; set +a`，同时应用 dev 与 test 库）。
- 实施前先 `git log -1` 现读 HEAD（工作区已含 5 个拆分提交，本计划引用的行号会漂，所有引用现读现用）。

## Review Focus

1. **老库现值丢失**：已配置的 Turnstile 密钥必须在迁移后原样出现在 `captcha` 组（Task 4 步骤 4 的实库核对；变异：删掉迁移①，②的默认值会把现值覆盖回空）。
2. **自建组权限突然失效是本轮明确接受的代价**，但 owner/admin 决不能受影响——`system.captcha.*` 必须进预设组运行时全量授权（Task 3 的 `TestPresetPermissions` 已覆盖；变异：catalog 新对忘了进 `Catalog()`，该测试红）。
3. **公开设置里 `turnstile_secret_key` 决不能出现**（非公开项），组搬家不得改变可见性语义（Task 6 的公开形状断言里显式断言它缺席）。
4. **auth 组公开值收敛到只剩 `totp_enabled`**：前端 profile 页读 `settings.auth?.totp_enabled`，若 auth 组公开项意外为空，两步验证入口会静默消失（Task 6 断言 auth 组公开值恰为 1 个）。
5. **前端 chunk 缓存**：`public-settings.ts` 改动后 dev server 若吐旧数据，重启而不是改代码；验证走 `localhost`。

---

### Task 1: rbac 新增 `system.captcha` 权限对

**Files:**
- Modify: `backend/internal/common/rbac/rbac.go`
- Test: `backend/internal/common/rbac/rbac_test.go`

**Interfaces:**
- Produces: 常量 `PermissionCaptchaView = "system.captcha.view"`、`PermissionCaptchaEdit = "system.captcha.edit"`，中文名「查看人机验证」/「管理人机验证」；`Catalog()` 顺序里排在 register 对之后、groups 对之前（不影响 `All()` 之外的消费方）。

- [ ] **Step 1: 写失败测试**

`rbac_test.go` 的 `TestCatalogMatchesFixedValues`：`want` 切片在 register 两项后插入

```go
{Key: PermissionCaptchaView, Name: "查看人机验证"},
{Key: PermissionCaptchaEdit, Name: "管理人机验证"},
```

常量字符串断言追加 `PermissionCaptchaView != "system.captcha.view" || PermissionCaptchaEdit != "system.captcha.edit"`。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/common/rbac/ -count=1`
Expected: FAIL（undefined: PermissionCaptchaView）

- [ ] **Step 3: 实现**

`rbac.go`：常量块加两行；`catalog` 切片在 `PermissionRegisterEdit` 之后插入两条 `Permission{Key, Name}`。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/common/rbac/ -count=1`
Expected: PASS

- [ ] **Step 5: 变异自检**

临时从 `catalog` 删掉新对（只留常量）→ `TestCatalogMatchesFixedValues` 必须红；恢复。

- [ ] **Step 6: 核对 diff**（不提交）

`git diff` 只含 rbac.go 与 rbac_test.go。

---

### Task 2: settings 清单拆出 captcha 组

**Files:**
- Modify: `backend/internal/modules/settings/registry.go`
- Test: `backend/internal/modules/settings/registry_test.go`

**Interfaces:**
- Produces: `builtinRegistry` 含 `captcha` 组（5 项，声明在 auth 之后）；`validateCaptchaGroup(values map[string]any, touched map[string]bool) error`（scopes 校验从 `validateAuthGroup` 原样搬出）；auth 组 `ValidateOn` 只剩 `password_min_length`、`session_max_age_days`、`session_max_age_enabled`。

- [ ] **Step 1: 改失败测试**

`registry_test.go` 的 `TestBuiltinRegistryShape`：auth 组 `assertDefinitions` 删掉 5 个 captcha 项、计数断言 `!= 12` 改 `!= 7`；紧随 auth 块后新增 captcha 组断言

```go
if len(reg.groups["captcha"]) != 5 {
    t.Fatalf("captcha items = %d, want 5", len(reg.groups["captcha"]))
}
assertDefinitions(t, reg.groups["captcha"], []wantItem{
    {name: "captcha_enabled", typ: FieldTypeBool, def: false, public: true},
    {name: "captcha_provider", typ: FieldTypeString, def: "none", public: true},
    {name: "captcha_scopes", typ: FieldTypeString, def: "login,register", public: true},
    {name: "turnstile_site_key", typ: FieldTypeString, def: "", public: true},
    {name: "turnstile_secret_key", typ: FieldTypeString, def: ""},
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/modules/settings/ -run TestBuiltinRegistryShape -count=1`
Expected: FAIL（captcha 组不存在）

- [ ] **Step 3: 实现 registry**

`registry.go`：auth 组删 5 项 captcha、`ValidateOn` 去掉 `captcha_scopes`；`validateAuthGroup` 删掉 `captcha_scopes` 分支；auth 组后新增

```go
{
    Name:       "captcha",
    ValidateOn: []string{"captcha_scopes"},
    Validate:   validateCaptchaGroup,
    Items: []Definition{
        {Name: "captcha_enabled", Type: FieldTypeBool, Default: false, Public: true},
        {Name: "captcha_provider", Type: FieldTypeString, Default: "none", Public: true},
        {Name: "captcha_scopes", Type: FieldTypeString, Default: "login,register", Public: true},
        {Name: "turnstile_site_key", Type: FieldTypeString, Default: "", Public: true},
        {Name: "turnstile_secret_key", Type: FieldTypeString, Default: ""},
    },
},
```

`validateCaptchaGroup` 从 `validateAuthGroup` 的 scopes 分支原样搬出（逻辑零变化）。

- [ ] **Step 4: 跑 settings 包测试**

Run: `bash scripts/test.sh ./internal/modules/settings/`
Expected: `TestBuiltinRegistryShape` PASS；以下 FAIL 属预期（后续 Task 修），本步确认 FAIL 原因不是编译错：`TestBuiltinPublicAllowlist`（want 表还有 5 个 `auth.captcha_*`、缺 `captcha.*`）、`TestSeedMigrationMatchesBuiltinRegistry`（seed 未改）、controller_test 公开形状（3 组断言）。`TestBuiltinPublicAllowlist` 的 `want` 表同步为移除 5 个 `auth.captcha_*`、按序插入 `captcha.captcha_enabled/captcha_provider/captcha_scopes/turnstile_site_key`——这一步顺手改掉，让它也 PASS。

- [ ] **Step 5: 核对 diff**（不提交）

---

### Task 3: settings 接口权限三路分流

**Files:**
- Modify: `backend/internal/modules/settings/controller.go`
- Test: `backend/internal/modules/settings/controller_test.go`

**Interfaces:**
- Consumes: Task 1 的 `rbac.PermissionCaptchaView/Edit`。
- Produces: `requireGroupPermission` 的 switch 增加 `case "captcha"` 分支 → captcha 权限对。

- [ ] **Step 1: 写失败测试**

`controller_test.go` 新增 `TestSettingsPatchCaptchaGroupPermission`，写法照抄 `TestSettingsGroupPermissionsSplitByGroup` 的现成 helper：

```go
db := testDB(t)
resetBuiltin(t, db, "auth", "captcha")
ts := newSettingsTestApp(t, db)
captchaGroupID := createGroupWithPermissions(t, db, "cap", []string{rbac.PermissionCaptchaView, rbac.PermissionCaptchaEdit})
registerGroupID := createGroupWithPermissions(t, db, "reg", []string{rbac.PermissionRegisterView, rbac.PermissionRegisterEdit})
captchaUser := seedClient(t, ts, createUserInGroup(t, db, captchaGroupID))
registerUser := seedClient(t, ts, createUserInGroup(t, db, registerGroupID))
```

断言：captchaUser PATCH `/api/v1/admin/settings/captcha`（body `{"captcha_scopes": "register"}`）→ 200 且读回 `captcha_scopes == "register"`；registerUser PATCH 同路径 → 403 `forbidden`；captchaUser GET `/api/v1/admin/settings/auth` → 403（反向隔离，证明新权限不含 auth 组）。

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/settings/`
Expected: 新测试 FAIL（captcha 组走的是 settings 权限或 404）

- [ ] **Step 3: 实现**

`controller.go`：switch 加

```go
case "captcha":
    permission = captchaPermission
```

函数签名改四参 `requireGroupPermission(settingsPermission, registerPermission, captchaPermission string)`，调用处传 `rbac.PermissionSettingsView, rbac.PermissionRegisterView, rbac.PermissionCaptchaView`（PATCH 行同理 Edit）。

- [ ] **Step 4: 跑测试确认通过**

Run: `bash scripts/test.sh ./internal/modules/settings/`
Expected: 新测试 PASS；既有权限测试仍 PASS（`TestSettingsGetRequiresAuth` 等）。

- [ ] **Step 5: 变异自检**

临时删掉 `case "captcha"` 分支 → 新测试的 403 断言必须红；恢复。

- [ ] **Step 6: 核对 diff**（不提交）

---

### Task 4: 数据迁移与历史 seed 改写

**Files:**
- Create: `backend/migrations/20261025130000_relocate_captcha_settings.sql`（现编号已到 `20261025120001`，`20261025130000` 空闲；执行时仍以 migrations 目录现读为准）
- Modify: `backend/migrations/20261023080025_seed_setting_values.sql`、`backend/migrations/20261024120000_seed_captcha_provider_none.sql`、`backend/migrations/20261025120000_seed_captcha_scopes.sql`、`backend/migrations/atlas.sum`

**Interfaces:**
- Consumes: Task 2 的 captcha 组清单（默认值以此为准）。
- Produces: 老库 `captcha` 组 5 行现值 + `auth` 组无 captcha 残留；新库 seed 对账通过。

- [ ] **Step 1: 改三条历史 seed**

按 spec §2.4：`20261023…` 删 4 行 auth 组 captcha；`20261024…` 与 `20261025…` 组名 `auth` → `captcha`。

- [ ] **Step 2: 写新迁移**

`20261025130000_relocate_captcha_settings.sql` 三段（spec §2.3 原文 SQL）：① `INSERT…SELECT` 现值 `auth`→`captcha`（`ON CONFLICT DO NOTHING`）；② 5 行字面默认值 `INSERT … ON CONFLICT DO NOTHING`（值：`false`、`"none"`、`"login,register"`、`""`、`""`）；③ `DELETE` auth 组 5 键。顺序固定 ①②③。

- [ ] **Step 3: seed 对账 + hash + validate**

Run: `go test ./internal/modules/settings/ -run TestSeedMigrationMatchesBuiltinRegistry -count=1`
Expected: PASS（去重后恰好 35 项：site 6 + register 5 + auth 7 + captcha 5 + mail 12）
Run: `atlas migrate hash --dir "file://migrations" && atlas migrate validate --dir "file://migrations"`
Expected: 无报错

- [ ] **Step 4: 应用到 dev/test 库并实测现值保留**

```bash
bash scripts/db-migrate-apply.sh
```

用 `.scratch` + pgx（无 psql/docker，既有惯例）查两库：`captcha` 组 5 行中 `turnstile_secret_key` 等于迁移前 `auth` 组的值（迁移前先手工查一次存对照）；`auth` 组查无 5 键残留。

- [ ] **Step 5: 变异自检**

临时删掉迁移①段 → 重跑（换一个新文件名或先在测试库回滚该迁移）后 `captcha.captcha_provider` 变回 `none` 而非现值，Step 4 的对照断言红；恢复①并重算 hash。测试库验证即可，不动 dev 库。

- [ ] **Step 6: 核对 diff**（不提交）

---

### Task 5: captcha 服务与 auth 测试改读新组

**Files:**
- Modify: `backend/internal/modules/captcha/service.go`
- Test: `backend/internal/modules/auth/captcha_flow_test.go`、`backend/internal/modules/auth/captcha_verify_test.go`、`backend/internal/modules/captcha/turnstile_test.go`（无需改）

**Interfaces:**
- Consumes: Task 2 的 captcha 组、Task 4 的迁移（测试库已有 captcha 组行）。
- Produces: `captcha.Service` 四处读取组名 `"captcha"`。

- [ ] **Step 1: 改测试组名**

`captcha_flow_test.go` 的 `resetCaptchaSettings` / `setCaptchaProvider` / `setCaptchaScopes` 三 helper 里 `"auth"` → `"captcha"`（7 处，现读现改）；`captcha_verify_test.go` 无组名写入，确认无需改。

- [ ] **Step 2: 跑测试确认失败**

Run: `bash scripts/test.sh ./internal/modules/auth/ -run TestLogin -count=1`
Expected: captcha 流程用例 FAIL（服务还在读 auth 组，读不到新组的行）

- [ ] **Step 3: 实现**

`captcha/service.go` 四处 `"auth"` → `"captcha"`。

- [ ] **Step 4: 跑测试确认通过**

Run: `bash scripts/test.sh ./internal/modules/auth/ ./internal/modules/captcha/ -count=1`
Expected: PASS（含 2FA 验证限流等非 captcha 用例）

- [ ] **Step 5: 核对 diff**（不提交）

---

### Task 6: 后端测试形状同步（settings / seed / auth）

**Files:**
- Modify: `backend/internal/modules/settings/controller_test.go`
- Test: 同文件

**Interfaces:**
- Consumes: Task 2/3/4 完成后的新形状。

- [ ] **Step 1: 改公开设置形状断言**

`TestSettingsPublicGetAnonymously`：`len(items) != 3` → `!= 4`；`items[2]` 断言 auth 组——公开值**恰好 1 个**且是 `totp_enabled == true`（防 Review Focus 4）；新增 `items[3]` 断言 captcha 组公开 4 值（`captcha_enabled=false`、`captcha_provider="none"`、`captcha_scopes="login,register"`、`turnstile_site_key=""`）且**不含 `turnstile_secret_key`**（防 Review Focus 3）。`TestSettingsPatchSiteReadsBackPublicly` 的 `len(items) != 3` 同步 `!= 4`。

- [ ] **Step 2: 改 resetBuiltin 调用点**

出现 `"auth"` 组 reset 且用例涉及 captcha 值的（现读 `TestSettingsPatchAuthCaptchaScopes` 等），组名改/加 `"captcha"`；`TestSettingsPatchAuthCaptchaScopes` 改名 `TestSettingsPatchCaptchaScopes`、PATCH 路径与 reset 组改 captcha。

- [ ] **Step 3: 跑 settings 全包**

Run: `bash scripts/test.sh ./internal/modules/settings/`
Expected: 全 PASS

- [ ] **Step 4: 跑全仓测试**

Run: `bash scripts/test.sh`
Expected: 全 PASS（app/auth/access 等无组名依赖；若有 FAIL，逐个核对是不是漏改的组名，修掉）

- [ ] **Step 5: 核对 diff**（不提交）

---

### Task 7: 自检路由搬进 captcha 模块，auth 解耦

**Files:**
- Create: `backend/internal/modules/captcha/module.go`、`backend/internal/modules/captcha/controller.go`、`backend/internal/modules/captcha/dto.go`
- Modify: `backend/internal/modules/auth/controller.go`、`backend/internal/modules/auth/service.go`、`backend/internal/modules/auth/dto.go`、`backend/internal/modules/auth/module.go`、`backend/internal/modules/auth/captcha_verify_test.go`、`backend/internal/modules/auth/auth_test.go`、`backend/internal/app/app.go`
- Delete: `backend/internal/modules/auth/captcha_verify_test.go`（用例迁 captcha 包）
- Test: `backend/internal/modules/captcha/verify_route_test.go`（新）

**Interfaces:**
- Consumes: Task 3 的 `rbac.PermissionCaptchaEdit`；现有 `captcha.Service.VerifyConfig(ctx, secret, token, ip) error`。
- Produces: `captcha.Module`（`New(settings *settings.Service) *Module`、`RegisterProtected(r *gin.RouterGroup)`）；`POST /admin/captcha/verifications` 挂 `system.captcha.edit`；auth 的 `captchaVerifier` 接口与 `VerifyCaptchaConfig` 删除，`auth.New` 去掉 captcha 参数。
- 参考: `auth/controller.go` 现有 `verifyCaptchaConfig` handler 与 `requireJSONRequest`（原样逻辑，见下）。

- [ ] **Step 1: 写失败测试（接口级，放 captcha 包）**

新 `verify_route_test.go`：沿用 `auth/captcha_verify_test.go` 的 7 条用例形状（通过 200 / 判负 400 `captcha_verification_failed` / 不可用 503 / 缺字段 400 / 非 JSON 与跨站 Origin 400 / 无 `system.captcha.edit` 403 / 未登录 401），测试 app 自建：`scs` + `guard.RequireAuth/LoadIdentity`（identity resolver 直接 ent 查，参考 `access.Service` 或用 `access.New(db).Service`）+ `captchaModule.RegisterProtected`；登录建会话用 `POST /api/v1/sessions`（密码用 ent 直建用户时哈希好）；owner 走 `SetGroupID(rbac.GroupIDOwner)`，403 用户走无权限自建组或 `GroupIDUser`。verifier 用 `captcha.NewService(settings, fake)`，fake 定义在本文件。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/modules/captcha/ -count=1`
Expected: FAIL（Module 不存在）

- [ ] **Step 3: 实现 captcha 三件套**

- `dto.go`：`CaptchaVerificationRequest`（`secret_key`/`token` JSON tag）、`CaptchaVerificationResponse`（`verified`）——从 `auth/dto.go` 剪切。
- `controller.go`：handler 逻辑从 `auth/controller.go` 原样搬（含 `requireJSONRequest` 同款 Content-Type/Origin 守卫，**复制到本包**，不从 auth 导出）；路由 `r.POST("/admin/captcha/verifications", guard.RequirePermission(rbac.PermissionCaptchaEdit), c.verify)`。
- `module.go`：`Module{Service *Service, controller *controller}`；`New(settingsService *settings.Service) *Module` 内部 `Service: NewService(settingsService, newTurnstileVerifier(turnstileSiteverifyURL, turnstileTimeout))`——与现 `captcha.New` 等价；`RegisterProtected(r)` 委托 `controller.registerProtected(r)`。`captcha.New` 函数**删除**（唯一调用方 app.go 改用 `captchaModule.Service`，避免两个生产入口）。

- [ ] **Step 4: auth 侧解耦**

`auth/controller.go`：删 handler 与路由行、`CaptchaVerificationRequest/Response` 从 dto 删、`login` import 清理；`auth/service.go`：删 `VerifyCaptchaConfig` 与 `captchaVerifier` 接口、`Service.captcha` 字段删；`auth/module.go`：`New` 去掉 verifier 参数；`auth/captcha_verify_test.go` **删除**（7 条用例已迁 captcha 包）；`auth_test.go` 的 `newTestAppWithCaptchaVerifier` 简化为 `newTestAppWithCaptcha`（只构造 `captcha.NewService(settingsService, verifier)` 注入 login/registration，auth service 不再收 captcha）。

- [ ] **Step 5: app 装配**

`app.go`：`captchaService := captcha.New(settingsModule.Service)` 改 `captchaModule := captcha.New(settingsModule.Service)`，`captchaService := captchaModule.Service`；`captchaModule.RegisterProtected(protected)` 挂进保护组；`auth.New` 调用去掉 captchaService 实参。login / registration 继续收 `captchaService`。

- [ ] **Step 6: 跑测试确认通过**

Run: `bash scripts/test.sh ./internal/modules/captcha/ ./internal/modules/auth/ ./internal/app/ -count=1`
Expected: 全 PASS

- [ ] **Step 7: 变异自检**

临时把 captcha 路由守卫改回 `rbac.PermissionRegisterEdit` → captcha 包新测试的 403 用例红；恢复。

- [ ] **Step 8: 静态门禁**

Run: `bash scripts/check.sh`
Expected: `check ok`

- [ ] **Step 9: 核对 diff**（不提交）

---

### Task 8: 前端读点切换与权限矩阵

**Files:**
- Modify: `frontend/hooks/use-captcha-settings-save.ts:51`、`frontend/lib/admin-settings.ts`（`getCaptchaSettings`）、`frontend/lib/public-settings.ts`（`getCaptchaConfig`）、`frontend/lib/groups.ts`、`frontend/messages/zh-CN.json`、`frontend/messages/en.json`

**Interfaces:**
- Consumes: 后端 captcha 组（`/admin/settings/captcha`、公开设置 `settings.captcha`）与 `system.captcha` 权限点（经 `/admin/permissions` 自动下发目录）。

- [ ] **Step 1: 读点切换**

`use-captcha-settings-save.ts` PATCH URL `auth` → `captcha`；`admin-settings.ts` `getCaptchaSettings` 的 fetchAdminPayload 路径改 `captcha`；`public-settings.ts` `getCaptchaConfig` 内 `const auth = settings.auth` 改 `const captcha = settings.captcha`（4 处字段前缀同步；`turnstile_secret_key` 是非公开项不会出现，逻辑本就只读 site key，无需处理）。`login-settings.ts` / `register-settings.ts` / profile 页 `totp_enabled` 不动。

- [ ] **Step 2: 权限矩阵**

`groups.ts`：`RESOURCE_LABEL_KEYS` 加 `"system.captcha": "resourceCaptcha"`；`MATRIX_GROUPS` 在 `signInAndRegister` 节后加 `{ labelKey: "captchaGroup", resources: ["system.captcha"] }`。`messages/zh-CN.json` 的 `groups` 命名空间加 `"resourceCaptcha": "人机验证"`；`messages/en.json` 加 `"resourceCaptcha": "Human verification"`。

- [ ] **Step 3: 前端验证**

Run: `cd frontend && npx tsc --noEmit && npx eslint hooks/use-captcha-settings-save.ts lib/admin-settings.ts lib/public-settings.ts lib/groups.ts && npx prettier --check hooks/use-captcha-settings-save.ts lib/admin-settings.ts lib/public-settings.ts lib/groups.ts messages/zh-CN.json messages/en.json`
Expected: 全部无报错

- [ ] **Step 4: 核对 diff**（不提交）

---

### Task 9: README / DEVLOG 收尾与全量验证

**Files:**
- Modify: `backend/README.md`、`docs/DEVLOG.md`

- [ ] **Step 1: README**

设置清单章节：`auth` 组 7 项（公开只剩 `totp_enabled`）、新增 `captcha` 组 5 项（四公开一非公开）；权限点目录四对→五对（含 `system.captcha`）；模块表：auth 行删自检路由描述，captcha 行改「有路由模块」并写路由与守卫；权限分流描述加 captcha 分支。

- [ ] **Step 2: DEVLOG**

顶部倒序插入本轮条目：范围、动机、决策（组名搬家、三步迁移、历史 seed 改写理由、`requireJSONRequest` 复制不共享、`system.captcha` 新权限点与自建组手工补授的已接受代价）、验证、已知缺口（`builtin` 未实现、`findUserByIdentifier` 双用途、自建组需手工补授）。

- [ ] **Step 3: 全量验证**

Run: `cd backend && bash scripts/check.sh && bash scripts/test.sh`
Expected: `check ok`，全仓 PASS

- [ ] **Step 4: 手工实机核对清单（归用户）**

- 设置页人机验证：保存/读回走新组；已有 Turnstile 密钥仍在。
- 登录/注册页：配置 captcha 后正常弹窗（site key 从新组下发）。
- 权限矩阵出现「人机验证」节。

- [ ] **Step 5: 核对 diff**（不提交）
