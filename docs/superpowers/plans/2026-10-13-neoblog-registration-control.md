# 注册控制实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. 注意：本机这两个子技能都未安装（`C:\Users\Multu\.qoder\skills` 里只有 brainstorming / writing-plans / systematic-debugging / using-superpowers），执行方式在计划末尾的交付环节由用户点选，届时按「逐任务实现 + 一次全新审查」或「我全程自己实现 + 末尾一次全新审查」两种口径走。

**Goal:** 让管理员通过一个受保护的接口关掉注册——`POST /api/v1/auth/register` 在关闭态返回 403 `registration_closed`，其余行为零变化。

**Architecture:** 这是设置系统（0.8.0）的第一个真实消费方：清单里落第一个真实项 `register/enabled`，注册流程按它把关；同时把设置模块从「无路由能力模块」升级成带 controller 的模块，暴露按组通用的 `GET` / `PATCH /api/v1/admin/settings/:group`，鉴权走两个新权限点。零迁移——`setting_values` 表已存在，本轮第一次被真实读写。

**Tech Stack:** Go + gin + ent（无 schema 变化）+ scs 会话 + 既有 `internal/common/apierror` / `internal/common/guard` / `internal/common/rbac`。

**Spec:** `docs/superpowers/specs/2026-10-13-neoblog-registration-control-design.md`

## Global Constraints

- **零注释**：全仓口径，产品代码与测试代码都不写注释。
- **零迁移**：`backend/migrations/` 与 `migrations/atlas.sum` 一个字节都不动；不跑 `db-migrate-diff.sh`。
- **不碰 `frontend/`**：本轮不新增、不修改任何前端文件。写本计划时并行会话的前端设置页在制改动已被其自行回滚，盘上只剩一个空的 `frontend/app/console/settings/[section]/` 目录（git 不跟踪空目录，`git status` 里看不到）——不接管、不提交、也不删它。
- **集成测试约定**：`DATABASE_URL_TEST` 未设置时 `t.Skip`（`internal/modules/settings/migration_test.go:14` 的 `testDB`）。本机 Postgres 跑在 5434，`.env` 要用 `set -a; source .env; set +a` 才导出给子进程。
- **门禁命令**：`go build ./... && go vet ./...`；格式判定必须写 `test -z "$(gofmt -l .)"`（`gofmt -l` 有问题时退出码也恒为 0）。
- **提交口径**：按文件清单 `git add <逐个路径>`，**禁止 `git add -A`**；新文件首次要先 `git add`。
- **权限点顺序**：`rbac.catalog` 里两个新键**追加在末尾**（`internal/common/access` 的 `access_test.go:166` 断言 `items[0]` 是「查看用户」，插到前面会让它漂）。
- **测试清理硬要求**：凡写过 `register/enabled` 行的用例，必须在 `t.Cleanup` 里 `ResetItem`（或按组删行）。测试库里残留 `false` 会让同库后续 auth 用例莫名吃 403。

## Review Focus

spec 隐含、但默认任务形状不会自动覆盖，且真上线最容易咬人的五处；每条已在对应任务里钉了用例。

1. **请求体不是合法 JSON、或不是对象**（数组 / 字符串 / 垃圾字节）→ 必须 400 `invalid_request`，不能 500、不能在解码处 panic。→ Task 5 `TestSettingsPatchMalformedBody`
2. **组存在但库里零覆盖行**时 `GET` → 必须返回合并后的默认值 `{"enabled":true}`，不是 404、不是空对象。→ Task 4 `TestSettingsGetAsOwner`
3. **清单项被误删或改名**（`register/enabled` 不在了）→ 注册**不能静默放行**：形状测试先红；运行期 `GetBool` 报错原样上抛、表现为 500，这是 spec §5.3 定的响亮失败。→ Task 2 `TestBuiltinRegistryShape` + Task 6 拦截写法
4. **数据库不可用**时注册与设置端点 → 必须 503 `service_unavailable`（走 `apierror.isUnavailable` 现状映射），不能 500、更不能放行。→ 注册侧由既有 `app_test.go:548`「register needs the database」继续覆盖（注入 settings 后必须仍是 503，红了就是把关位置写错）；设置侧的新覆盖在 Task 7 Step 2。
5. **半写状态**：一次 PATCH 里一项合法 + 一项未知 → 必须整组零落库，合法那项也不能进库。→ Task 5 `TestSettingsPatchUnknownItem`

---

### Task 1: 新错误码 `registration_closed`

**Files:**
- Modify: `backend/internal/common/apierror/apierror.go`（哨兵区 `:36-65`、`statusByCode` `:67-96`）
- Test: `backend/internal/common/apierror/apierror_test.go`

**Interfaces:**
- Produces: `apierror.ErrRegistrationClosed`（`Type: TypePermission`、`Code: "registration_closed"`、`Param: nil`），`statusByCode["registration_closed"] = 403`。Task 6 用它。

- [ ] **Step 1: 写失败测试**——`apierror_test.go`：`TestRegistryMatchesSpec` 的 cases 里在 `cannot_modify_self` 之后加一行 `{"registration_closed", ErrRegistrationClosed, TypePermission, "registration_closed", http.StatusForbidden, nil}`；`TestRegistryIntegrity` 的 `all` 切片加入 `ErrRegistrationClosed`，并把 `:259` 与 `:262` 两处 `28` 改为 `29`。
- [ ] **Step 2: 跑到红**——`go test ./internal/common/apierror/ -run TestRegistry -v`，预期编译失败（`undefined: ErrRegistrationClosed`）。
- [ ] **Step 3: 实现**——`apierror.go` 在 `ErrOwnerProtected` / `ErrCannotModifySelf` 附近加哨兵一行；`statusByCode` 在 `forbidden` 一族之间加 `"registration_closed": http.StatusForbidden`。
- [ ] **Step 4: 跑到绿**——同 Step 2 命令全 PASS；`go test ./internal/common/apierror/` 整包绿。
- [ ] **Step 5: 提交**——`git add backend/internal/common/apierror/apierror.go backend/internal/common/apierror/apierror_test.go && git commit -m "feat(backend): 错误码 registration_closed（403/permission_error）"`

### Task 2: 内置清单落第一项 `register/enabled`

**Files:**
- Modify: `backend/internal/modules/settings/registry.go:8`
- Test: `backend/internal/modules/settings/registry_test.go`

**Interfaces:**
- Produces: `builtinRegistry` 含 `Group{Name: "register", Items: []Definition{{Name: "enabled", Type: FieldTypeBool, Default: true}}}`；`settings.New(db)` 构造出的 Registry 可通过 `reg.groups["register"]["enabled"]` 取到该定义。Task 4/5 的端点与 Task 6 的把关都按这个组名项名读写。

- [ ] **Step 1: 写失败测试**——`registry_test.go` 加 `TestBuiltinRegistryShape`：`reg, err := compileRegistry(builtinRegistry)` 断言 `err` 为 nil；`def, ok := reg.groups["register"]["enabled"]` 断言 `ok`、`def.Type == FieldTypeBool`、`def.Default.(bool) == true`（默认「开放注册」）。
- [ ] **Step 2: 跑到红**——`go test ./internal/modules/settings/ -run TestBuiltinRegistryShape -v`，预期 FAIL（register 组不存在）。
- [ ] **Step 3: 实现**——`registry.go` 把 `var builtinRegistry = []Group{}` 换成含上述一个组一项的字面量，形状照 README「怎么加一个设置」配方（`README.md:251` 起）。
- [ ] **Step 4: 跑到绿**——`go test ./internal/modules/settings/ -run 'TestBuiltin|TestRegistry' -v` 全 PASS（确认 `TestRegistryRecipeShape` / `TestRegistryEmpty` 用的本地字面量没受影响）。
- [ ] **Step 5: 提交**——`git add backend/internal/modules/settings/registry.go backend/internal/modules/settings/registry_test.go && git commit -m "feat(backend): 设置清单落地第一项 register/enabled（默认开放注册）"`

### Task 3: `Service.PatchGroup` + `ItemError`

**Files:**
- Modify: `backend/internal/modules/settings/service.go`
- Test: `backend/internal/modules/settings/service_test.go`

**Interfaces:**
- Consumes: 包内既有 `decodeTyped(FieldType, json.RawMessage) (any, error)`（`:219`，返回 string / int64 / bool，数字走 `ParseInt` 天然拒 `3.5` 与 `1e3`）、`resolveGroupWrites`、`repository.upsertMany`、`GetGroup`。
- Produces:
  ```go
  type ItemError struct {
      Item string
      Err  error
  }
  func (e *ItemError) Error() string
  func (e *ItemError) Unwrap() error

  func (s *Service) PatchGroup(ctx context.Context, group string, raw map[string]json.RawMessage) (map[string]any, error)
  ```
  Task 5 的 controller 靠 `errors.As(&ItemError)` 取 `Item` 去填 `apierror.WithParam`。

- [ ] **Step 1: 写失败测试**——`service_test.go` 加 `TestPatchGroupValidation`（全部走 `newServiceWith(nil, sampleGroups(), testLogger())`，四条断言都不触库，所以 nil repo 安全）：未知组 → `errors.Is(err, ErrUnknownGroup)`；`raw` 长度为 0 → `errors.Is(err, ErrUnknownItem)` 且**不是** `ItemError`；`{"nope": json.RawMessage("1")}` → `errors.As` 命中 `*ItemError`、`Item == "nope"`、`errors.Is(err, ErrUnknownItem)`；`{"port": json.RawMessage("3.5")}` → `*ItemError{Item: "port"}` 且 `errors.Is(err, ErrTypeMismatch)`。
- [ ] **Step 2: 跑到红**——`go test ./internal/modules/settings/ -run TestPatchGroup -v`，预期编译失败。
- [ ] **Step 3: 实现**——`service.go` 加 `ItemError`（`Error()` 返回 `e.Err.Error()`，`Unwrap()` 返回 `e.Err`）与 `PatchGroup`：组不在清单 → `ErrUnknownGroup`；`len(raw)==0` → `ErrUnknownItem`；对项名 `sort.Strings` 后逐项处理——清单无此名 → `&ItemError{name, ErrUnknownItem}`，`decodeTyped(def.Type, raw)` 出错 → `&ItemError{name, ErrTypeMismatch}`；全部通过则组装 `[]ItemWrite`（`Type` 取 `def.Type`）交 `resolveGroupWrites` + `repo.upsertMany`，成功后 `return s.GetGroup(ctx, group)`。不新起第二条写路径。
- [ ] **Step 4: 跑到绿**——同 Step 2 命令 PASS；`go test ./internal/modules/settings/` 整包绿。
- [ ] **Step 5: 提交**——`git add backend/internal/modules/settings/service.go backend/internal/modules/settings/service_test.go && git commit -m "feat(backend): 设置服务 PatchGroup——按清单类型解码的整组部分更新"`

### Task 4: 设置模块长出路由（GET + 权限点）

**Files:**
- Create: `backend/internal/modules/settings/module.go`、`backend/internal/modules/settings/controller.go`、`backend/internal/modules/settings/controller_test.go`
- Modify: `backend/internal/common/rbac/rbac.go`（常量区 `:10-14`、`catalog` `:43-48`）
- Modify: `backend/internal/app/app.go`（`:36-97`）

**Interfaces:**
- Consumes: `Service.GetGroup`；`guard.RequirePermission`（`guard/permission.go:53`）；`access.New(db).Service` 供测试挂 `LoadIdentity`。
- Produces:
  ```go
  type Module struct {
      Service *Service
      controller *controller
  }
  func New(db *database.Database) (*Module, error)
  func (m *Module) RegisterProtected(r *gin.RouterGroup)

  const (
      PermissionSettingsView = "system.settings.view"
      PermissionSettingsEdit = "system.settings.edit"
  )
  ```
  Task 5 往同一 controller 加 PATCH；Task 6 用 `settingsModule.Service`。

- [ ] **Step 1: 写失败测试**——`controller_test.go` 先建脚手架（**复制 access 的形状**，本包 helper 非导出、跨包不可用）：`newSettingsTestApp(t, db)` 起 gin + `/seed` 路由（`sm.Put(ctx, guard.SessionUserIDKey, c.Query("user"))`，照 `access_test.go:22-43`）+ `settings.New(db)` 挂进 `api.Group("", guard.RequireAuth(sm), guard.LoadIdentity(access.New(db).Service, sm))` 的 protected 组；配 `seedClient` / `doJSON` / `decodeErrorCode` / `expectStatus` / `createTestUser(t, db, groupID)`（形状照 `access/service_test.go:23-40`，**连带它依赖的 `uniqueTag` 一起复制**；这些都是 access 包内非导出 helper，跨包用不了，只能在本包再写一份——与 settings 包当初复制 `testDB` 的做法一致）。用例四条：
  - `TestSettingsGetAsOwner`：owner（`rbac.GroupIDOwner`）GET `/api/v1/admin/settings/register` → 200，解码成 `map[string]any` 断言 `enabled == true`，且**库里此时零行**（Review Focus 2：默认值必须出现，不是 404、不是 `{}`）。跑前 `resetGroups(t, db, "register")`（同包 `service_integration_test.go:30` 已有此 helper）。
  - `TestSettingsGetUnknownGroup`：GET `/api/v1/admin/settings/nope` → 404 `not_found`。
  - `TestSettingsGetRequiresAuth`：无会话 → 401 `unauthorized`。
  - `TestSettingsGetForbiddenForPlainUser`：`rbac.GroupIDUser` 组用户 → 403 `forbidden`。
- [ ] **Step 2: 跑到红**——`go test ./internal/modules/settings/ -run TestSettingsGet -v`，预期编译失败（无 `New` 返回 `*Module`、无 `RegisterProtected`）。
- [ ] **Step 3: 加权限点**——`rbac.go` 常量区加 `PermissionSettingsView = "system.settings.view"` / `PermissionSettingsEdit = "system.settings.edit"`，`catalog` **末尾**追加 `{Key: PermissionSettingsView, Name: "查看设置"}`、`{Key: PermissionSettingsEdit, Name: "管理设置"}`（不动已有四条）。owner/admin 走 `PresetPermissions → rbac.All()` 运行时取全量，无需任何数据迁移。
- [ ] **Step 4: 写 module.go / controller.go**——`module.go` 照 `mail/module.go`：`New(db)` 内部走 `newServiceWith(db, builtinRegistry, slog.Default())`，返回 `&Module{Service: svc, controller: newController(svc)}`；`RegisterProtected` 转调 controller。`controller.go` 结构体固定为 `type controller struct { service *Service }`（对齐 `auth/controller.go:12-14`，Task 5 写 `c.service.PatchGroup` 要按这个名字），本任务只放 `registerProtected` 里的 `r.GET("/admin/settings/:group", guard.RequirePermission(rbac.PermissionSettingsView), c.getGroup)` 与 `getGroup`：`GetGroup` 成功 → `ctx.JSON(200, values)`（裸对象，无 `data` 信封）；`errors.Is(err, ErrUnknownGroup)` → `apierror.Write(ctx, apierror.ErrNotFound)`；其它错误 → `apierror.Write(ctx, err)`。请求/响应类型直接写在 controller.go 内（照 `mail/controller.go:16-24`，不建 dto.go）。
- [ ] **Step 5: 改 app.go 装配**——`settingsModule, err := settings.New(db)`（原 `settings.New` 返回 `*Service`）；`App` 结构体**删掉** `settings *settings.Service` 字段与 `:95` 的赋值（对照 `mailer`：那字段留着的理由是 `shutdown` 要 `a.mailer.Close`，`app.go:162`，settings 无生命周期动作）；在既有 `protected` 组那段（`:70-73`）加一行 `settingsModule.RegisterProtected(protected)`。本任务先不把它注入 auth（Task 6 做）。
- [ ] **Step 6: 编译与整包回归**——`go build ./... && go vet ./...` 通过；`go test ./internal/app/ ./internal/modules/settings/ ./internal/modules/access/ -v` 全绿（`access` 里 `TestPermissionsEndpoint` 用 `len(rbac.Catalog())` 动态比对，加键不影响；`TestIdentityByUserID` 比对 `rbac.All()` 同理）。
- [ ] **Step 7: 提交**——`git add` 五个文件（含新建三个）后 `git commit -m "feat(backend): 设置模块长出路由——GET /admin/settings/:group 与 system.settings.* 权限点"`

### Task 5: PATCH 端点与错误映射

**Files:**
- Modify: `backend/internal/modules/settings/controller.go`
- Test: `backend/internal/modules/settings/controller_test.go`

**Interfaces:**
- Consumes: Task 3 的 `Service.PatchGroup` 与 `*ItemError`；Task 4 的 `PermissionSettingsEdit`、`newSettingsTestApp`；`apierror.WithParam`。
- Produces: 路由 `PATCH /api/v1/admin/settings/:group`；错误映射表（spec §5.2）。

- [ ] **Step 1: 写失败测试**——`controller_test.go` 加 `decodeError(t, resp) map[string]any`（返回整个 error 对象，本包要断言 `param`，`access` 的 `decodeErrorCode` 只给 code）。七条：
  - `TestSettingsPatchTurnsRegistrationOff`：owner PATCH `/api/v1/admin/settings/register` 体 `{"enabled":false}` → 200 且响应体 `enabled == false`；随后 GET 读回 `false`；`t.Cleanup` 里 `ResetItem(ctx, "register", "enabled")`。
  - `TestSettingsPatchUnknownItem`（Review Focus 5）：体 `{"enabled":false,"nope":1}` → 400 `invalid_request` 且 `param == "nope"`；**并断言 `register` 组此刻零行**——`enabled` 合法、`nope` 未知，零行才证明是「校验全过才落库」，没有把合法那项先写进去。只发 `{"nope":1}` 测不出这件事（本来就没有合法项可写）。
  - `TestSettingsPatchTypeMismatch`：体 `{"enabled":"no"}` → 400 且 `param == "enabled"`。
  - `TestSettingsPatchEmptyBody`：体 `{}` → 400 `invalid_request` 且 `param == nil`。
  - `TestSettingsPatchMalformedBody`（Review Focus 1）：raw `{not json` → 400 `invalid_request`；raw `["enabled"]`（数组）→ 400；两者都不能是 500，且进程不 panic。
  - `TestSettingsPatchUnknownGroup`：PATCH `/api/v1/admin/settings/nope` 体 `{"enabled":false}` → 404 `not_found`。
  - `TestSettingsPatchNeedsEditPermission`：`rbac.GroupIDUser` 用户 PATCH → 403 `forbidden`；owner 同请求 → 200。
- [ ] **Step 2: 跑到红**——`go test ./internal/modules/settings/ -run TestSettingsPatch -v`。测试脚手架的 engine 没开 `HandleMethodNotAllowed`（那是 `app.go:53` 才开的），所以 GET-only 路由上打 PATCH 会落进 NoRoute 拿 404，不是 405。预期**除 `TestSettingsPatchUnknownGroup` 外其余六条全红**——那一条期望的恰好也是 404，实现前就会假绿，所以判「红」按用例名点名，不要数顺序、也不要只看整包有没有红。
- [ ] **Step 3: 实现**——`registerProtected` 加 `r.PATCH("/admin/settings/:group", guard.RequirePermission(rbac.PermissionSettingsEdit), c.patchGroup)`。`patchGroup`：`var raw map[string]json.RawMessage`，`ShouldBindJSON` 失败（含数组/标量形状）→ `apierror.Write(ctx, apierror.ErrInvalidRequest)`；成功 → `c.service.PatchGroup(ctx, ctx.Param("group"), raw)`，失败走包内 `writeSettingError(ctx, err)`（同文件写一个 `errors.As` 出 `*ItemError` → `apierror.WithParam(ErrInvalidRequest, item)`；`errors.Is(…, ErrUnknownGroup)` → `ErrNotFound`；`errors.Is(…, ErrUnknownItem)` / `ErrTypeMismatch` → `ErrInvalidRequest`；default → `apierror.Write(ctx, err)`），成功 → `ctx.JSON(200, values)`。
- [ ] **Step 4: 跑到绿**——同 Step 2 命令全 PASS；`go test ./internal/modules/settings/` 整包绿。
- [ ] **Step 5: 提交**——`git add backend/internal/modules/settings/controller.go backend/internal/modules/settings/controller_test.go && git commit -m "feat(backend): PATCH /admin/settings/:group——项名与类型把关，错误映射带 param"`

### Task 6: 注册按 `register/enabled` 把关

**Files:**
- Modify: `backend/internal/modules/auth/module.go:16`、`backend/internal/modules/auth/service.go`（结构体 `:24-29`、`newService` `:31-42`、`Register` `:44-53`）、`backend/internal/modules/auth/auth_test.go:30`
- Modify: `backend/internal/app/app.go:66`
- Create: `backend/internal/modules/auth/register_gate_test.go`

**Interfaces:**
- Consumes: Task 4 的 `settingsModule.Service` 与 `settings.Service.GetBool`；Task 1 的 `apierror.ErrRegistrationClosed`；Task 2 的组名项名 `register/enabled`。
- Produces: `auth.New(db *database.Database, sessions *scs.SessionManager, totpKey []byte, settingsService *settings.Service) *Module`；`Service` 多一个 `settings *settings.Service` 字段。

- [ ] **Step 1: 改测试脚手架**——`auth_test.go`：新增 `newTestAppWithSessionsAndSettings(t, db) (*httptest.Server, *scs.SessionManager, *settings.Service)`，内部 `sm := scs.New()`、`svc, err := settings.New(db)`（判错 `t.Fatalf`）、`New(db, sm, testTOTPKey, svc.Service)`，其余照原样；原 `newTestAppWithSessions` 改为调用它并丢弃第三个返回值（其余用例一行不改）。
- [ ] **Step 2: 写失败测试**——新建 `register_gate_test.go`，每条都用 `newTestAppWithSessionsAndSettings` 拿 settings 服务，并在 `t.Cleanup` 里 `ResetItem(ctx, "register", "enabled")`（**Global Constraints 的清理硬要求**）：
  - `TestRegisterBlockedWhenSettingOff`：`SetBool(ctx,"register","enabled",false)` → 记下 `db.Client.User.Query().Count(ctx)` → POST `/api/v1/auth/register`（合法体）→ 403 且 `decodeErrorCode == "registration_closed"` → 再数一次用户行数不变。
  - `TestRegisterGatePrecedesFieldValidation`（Review Focus 3 的运行面对偶）：关闭态下 POST 体 `{"email":"no-at","nickname":"nick","password":"pass1234"}` → **403 `registration_closed`**；同一个体在开启态 → **400 `invalid_request`**。**别用空体 `{}`**：`signup` 在 controller 层就因三个字段为空判 400（`auth/controller.go:52-57`），请求根本进不到 service，测不出门在不在最前。这里选的体三个字段都非空、能过 controller，但 `validEmail`（`validate.go:18`，走 `mail.ParseAddress`）会拒——正好把「门在服务第一句」钉死。
  - `TestLoginUnaffectedWhenRegistrationClosed`：先注册一个用户拿凭证 → 关闭开关 → POST `/auth/login` → 200。
  - `TestCreateAdminUserUnaffectedWhenClosed`：关闭态调用 `CreateAdminUser(...)` → 成功建号（`t.Cleanup` 删该用户），证明初始化路径没被开关堵死。
- [ ] **Step 3: 跑到红**——`go test ./internal/modules/auth/ -run 'TestRegister|TestLoginUnaffected|TestCreateAdminUserUnaffected' -v`，预期编译失败（`New` 参数不匹配）。
- [ ] **Step 4: 实现**——`Service` 加 `settings` 字段、`newService` 多收一个参数、`module.go` 的 `New` 多收 `settingsService *settings.Service` 并透传（auth 需要 import settings 包；settings 不 import auth，无环）。`Register` 第一句（在 `normalizeEmail` 之前）：
  ```go
  enabled, err := s.settings.GetBool(ctx, "register", "enabled")
  if err != nil {
      return User{}, err
  }
  if !enabled {
      return User{}, apierror.ErrRegistrationClosed
  }
  ```
  错误**原样上抛**：不吞、不回退默认值（spec §5.3；清单项缺失属编程错误，响亮失败）。
- [ ] **Step 5: 装配**——`app.go:66` 改 `authModule := auth.New(db, sessions, cfg.TOTPEncryptionKey, settingsModule.Service)`。`go build ./...` 驱动的调用点应只有 `app.go` 与 `auth_test.go` 两处，出现第三处说明漏看。
- [ ] **Step 6: 跑到绿**——`go test ./internal/modules/auth/ -v` 全绿（含既有注册用例，证明默认「开」时行为零变化）；`go test ./internal/app/` 绿。
- [ ] **Step 7: 提交**——`git add` 四个文件后 `git commit -m "feat(backend): 注册受 register/enabled 把关（关闭即 403，登录与建号不受影响）"`

### Task 7: app 级组合与降级验证

**Files:**
- Test: `backend/internal/app/app_test.go`

**Interfaces:**
- Consumes: 真实 `app.New(cfg, db)` 装配链（settings 模块、auth 模块、protected 组全在）。
- Produces: 无产品代码——本任务是全链路的证据。

- [ ] **Step 1: 写组合测试**——`TestRegistrationControlEndToEnd`。用 `newTestServer(t, config.EnvironmentDevelopment)`（`app_test.go:50`）起服务——它走的就是 `New(cfg, db)` 生产装配链，auth 与设置端点必然共用同一个 settings 实例，这正是本任务要证的连接点。身份照 `TestAccessEndToEnd`（`:680-700`）的现成三件套：`createGroupUser(t, db, rbac.GroupIDOwner)`（`:662`，返回 `auth.User` 与密码）+ `newClient(t)`（`:84`，带 cookie jar）+ `login(t, client, ts.URL, email, password)`（`:94`，**函数名是 `login`，不是 `loginAs`**）。步骤：owner `GET /api/v1/admin/settings/register` 断 200 且 `enabled == true` → owner `PATCH` 体 `{"enabled":false}` 断 200 → 换一个**新的** `newClient(t)`（匿名）POST `/api/v1/auth/register`，体 `{"email":"regctl<tag>@test.local","nickname":"regctl<tag>","password":"pass-12345"}`（`<tag>` 按既有写法用 `fmt.Sprintf("%d", time.Now().UnixNano())`）断 403 + `code == "registration_closed"` → owner `PATCH` 体 `{"enabled":true}` 断 200 → **同一个体**再注册断 201，收尾删号。两次注册必须同体：403 与 201 之差才能归因到开关。`t.Cleanup` 清 `register` 组行。
- [ ] **Step 2: 降级只补设置端点那一条**（Review Focus 4）——**别新建测试函数**：`app_test.go:548` 已有「register needs the database」用合法体断 503，auth 注入 settings 后它自然改由把关那次读库拿到 503、结果不变（若它变红，说明把关写在了读库之后或吞了错）。只在那张表（`:546-552`）补一行：`{"settings read needs the database", http.MethodGet, "/api/v1/admin/settings/register", "", http.StatusUnauthorized, "unauthorized"}`——与 `:549` 的 `admin list needs the database` 同型，未登录被 `RequireAuth` 先挡下。**降级态造不出带身份的客户端**：`newDegradedServer`（`:514`）起的是库不可达的 app，`login` 本身就 503，表里现有六条全是匿名请求，所以不要写「降级态 owner 打 PATCH」这种写不出来的用例。
- [ ] **Step 3: 跑到绿**——`go test ./internal/app/ -run TestRegistrationControl -v` 两条 PASS。若 Step 1 红在 403/201 顺序上，优先查 auth 注入的是不是同一个实例。
- [ ] **Step 4: 提交**——`git add backend/internal/app/app_test.go && git commit -m "test(backend): 注册控制跨模块组合与库不可用降级用例"`

### Task 8: 登记、进位与全量回归

**Files:**
- Modify: `backend/README.md`（`:28` 目录对照表、`:35` 规则 1、`:39` 规则 5、`:247` 起「设置」章节）
- Modify: `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md`（§3 路由表、§7.3 code 词汇表及其前言）
- Modify: `docs/DEVLOG.md`
- Modify: `backend/internal/version/version.go`（走脚本）

- [ ] **Step 1: README 四处**——① `internal/modules/settings` 那行改成「有路由模块：`New(db) (*Module, error)`，能力挂 `m.Service`，`GET/PATCH /admin/settings/:group`」；② 规则 1 的「无路由的能力模块（当前 `settings`）」改为空清单或删掉该括注；③ 规则 5 里「`settings.New(db) (*Service, error)` 是唯一返回 error 的模块构造器」的措辞更新为「返回 `(*Module, error)`，仍是唯一返回 error 的构造器」；④「设置」章节：删掉过期话「本轮清单不含任何真实设置」（核对命令 `grep -n "本轮清单不含任何真实设置" backend/README.md` 应无输出），补两个端点、两个权限点与 `register/enabled` 实例；迁移清单不动（本轮零迁移）。
- [ ] **Step 2: API 规范登记**——§3 路由表按现有编号续两条（21 `GET /api/v1/admin/settings/:group`、22 `PATCH /api/v1/admin/settings/:group`，注明守卫链）；§7.3 表加一行 `registration_closed` | 403 | `permission_error` | null；前言那句「0.11.0 起新增 `method_not_allowed`」按惯例续写本轮。两处行号会漂，现读现改。
- [ ] **Step 3: 版本进位**——`bash scripts/bump-version.sh minor`（`0.11.0` → `0.12.0`），确认 `internal/version/version.go` 已改。
- [ ] **Step 4: 手工冒烟**——`bash scripts/dev.sh` 起服务（库可达），用 owner 会话：`GET /api/v1/admin/settings/register` → `{"enabled":true}`；`PATCH` 关 → 注册吃 403；`PATCH` 开 → 注册 201。**测完必须把开关恢复为开**，并停掉服务（`netstat -ano | findstr :3000` 取 PID → `taskkill /PID <pid> /F` → 回验端口释放）。
- [ ] **Step 5: 全量回归**——`cd backend && go build ./... && go vet ./... && test -z "$(gofmt -l .)"`；`set -a; source .env; set +a; go test ./...` 全绿（有库；集成不是 skip 出来的绿）。
- [ ] **Step 6: DEVLOG 条目**——`docs/DEVLOG.md` 顶部加一节：做了什么、坑、已知缺口（前端无落点 / 改开关只能打接口 / 权限点可授出给自建组）。**只加不删**。
- [ ] **Step 7: DEVLOG 绕开不提交（用户 2026-10-13 已定）**——`docs/DEVLOG.md` 此刻带着并行会话未提交的环境轮条目，而本 harness 不能交互式部分暂存（`git add -p` 不可用），一提就是把两份条目捆进同一个提交。所以：**Step 6 的条目照写，但 `docs/DEVLOG.md` 一律不 `git add`**，让它继续悬在工作区，由并行会话或用户自己入库。提交前用 `git status --short` 复核 `M docs/DEVLOG.md` 确实没进暂存区。
- [ ] **Step 8: 提交**——逐个 `git add`：`backend/README.md`、`docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md`、`backend/internal/version/version.go`；`git commit -m "chore(backend): 注册控制轮 README/API 规范登记与版本进位"`。提交后 `git status --short` 应只剩 `M docs/DEVLOG.md`（并行会话的）与两个空目录之类的未跟踪残留；`migrations/` 必须一个字节没动（`git diff --stat HEAD -- backend/migrations` 为空）。
