# API 规范落地 Implementation Plan（v1.3 范围）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 按用户裁定范围（spec §0，v1.3）落地两块：①错误结构 `{"error":"token"}` → `{"error":{"type","code","param"}}`（无 message、映射 1:1、`preset_group_locked` 改 409）；②三处小修（`current_password` 改名、204→200、删除确认体）。

**Architecture:** 新建共享包 `internal/common/apierror`（哨兵 + 注册表 + JSON 写出），guard / auth / access 逐模块收敛到它；路由、分页、时间字段、health 全部不动。

**Tech Stack:** Go 1.27 + gin v1.12 + ent v0.14；测试为真 Postgres 集成测试（`DATABASE_URL_TEST`）。

**Spec:** `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md`——**以 §0 范围裁定为最高效力**，正文冲突处一律服从 §0。

## Global Constraints

- 所有 go 命令前先 `export PATH="/f/software/go/bin:$PATH"`（新 shell 不继承）。
- 集成测试需要 `DATABASE_URL_TEST`（在 `backend/.env`，跑前 `set -a && . ./.env && set +a`）；未设置时测试自动 skip——验证以 `go test` 输出 0 FAIL 且非全 SKIP 为准。
- 后端 Go 代码**零人工注释**；提交前 `gofmt -l .` 输出必须为空。
- 错误对象**只有 type/code/param 三个字段**，禁止出现任何报错文案（用户明令）。
- commit 信息格式：`type(backend): 中文描述`；不 push、不动 `frontend/`。
- 遵守 README 硬性规则：gin 只允许出现在 controller / module.go / app / guard / **apierror**（Task 5 同步进 README）。
- 每任务 TDD：先改测试到期望新行为→跑红→实现→跑绿→提交。

## Review Focus

1. 未知错误（含 wrap 链、未注册 error）→ 一律 500 `internal_error`，且 `param` 输出 JSON `null` 而非省略字段（Task 1）。
2. guard 中间件的 401/403 响应必须与业务端点错误**同构**（同一信封，Task 2）。
3. 改密旧密码错必须**保持 401 `invalid_credentials`**（范围裁定的 1:1 钉子，防止顺手"修"成 400 新码，Task 3）。
4. 删除确认体 `{"id","deleted":true}` 的 `id` 必须回显路径参数，状态码 200 非 204（Task 4）。
5. 并发竞态兜底 `already_taken` 仍 409 且 param=null；哨兵替换后并发测试必须全过（Task 3/4）。

---

### Task 1: `internal/common/apierror` 包（19 哨兵，无 message）

**Files:**
- Create: `backend/internal/common/apierror/apierror.go`
- Test: `backend/internal/common/apierror/apierror_test.go`

**Interfaces:**
- Consumes: 无（只依赖 gin）。
- Produces（后续任务依赖，签名逐字）:

```go
type Type string
const (TypeInvalidRequest Type = "invalid_request_error"; TypeAuthentication Type = "authentication_error"; TypePermission Type = "permission_error"; TypeNotFound Type = "not_found_error"; TypeConflict Type = "conflict_error"; TypeRateLimit Type = "rate_limit_error"; TypeServer Type = "server_error")

type Error struct {
	Type  Type    `json:"type"`
	Code  string  `json:"code"`
	Param *string `json:"param"`
}
func (e *Error) Error() string

// 19 个哨兵（*Error）：ErrInvalidRequest、ErrWeakPassword、ErrPasswordUnchanged、ErrInvalidPermission、ErrInvalidCredentials、ErrUnauthorized、ErrForbidden、ErrGrantForbidden、ErrOwnerProtected、ErrCannotModifySelf、ErrPresetGroupLocked、ErrNotFound、ErrEmailTaken、ErrNicknameTaken、ErrAlreadyTaken、ErrGroupNameTaken、ErrGroupNotEmpty、ErrLastOwner、ErrInternal
// 注意：没有 ErrCurrentPasswordMismatch（范围裁定撤销该拆分）。

func Body(err error) *Error
func Write(ctx *gin.Context, err error)
func Abort(ctx *gin.Context, err error)
func WithParam(err error, param string) error
```

- 注册表（code → HTTP / type / 默认 param），除 `preset_group_locked`=409/`conflict_error` 外与现状 1:1：
  - 400 `invalid_request_error`：`invalid_request`、`weak_password`、`password_unchanged`(param=`new_password`)、`invalid_permission`
  - 401 `authentication_error`：`invalid_credentials`、`unauthorized`
  - 403 `permission_error`：`forbidden`、`grant_forbidden`、`owner_protected`、`cannot_modify_self`
  - 404 `not_found_error`：`not_found`
  - 409 `conflict_error`：`preset_group_locked`、`email_taken`(param=`email`)、`nickname_taken`(param=`nickname`)、`already_taken`、`group_name_taken`(param=`name`)、`group_not_empty`、`last_owner`
  - 500 `server_error`：`internal_error`
- 语义：`Body` 用 `errors.As` 解 `*Error`（`WithParam` 包装须 `Unwrap` 回原哨兵）；解不出或 code 未注册 → `internal_error` 副本。`Write` 按 code 查状态码输出 `gin.H{"error": e}`；`Abort` = `Write` + `ctx.Abort()`。`WithParam` 本轮无生产调用点，仅作为注册表能力保留并测试。

- [ ] **Step 1: 写失败测试**（无 DB）：

```go
func TestRegistryMatchesSpec(t *testing.T)      // 表驱动 19 行：Write 后 status + 解码比较 type/code/param（含 preset_group_locked==409/conflict_error）
func TestBodyUnknownErrorFallsBack(t *testing.T) // errors.New 与 fmt.Errorf 包装 → internal_error
func TestWriteEnvelope(t *testing.T)             // 解码成 map 逐字段比较（不比原始串）；ErrInvalidRequest → param 键存在且为 JSON null
func TestAbortStopsChain(t *testing.T)           // ctx.IsAborted()==true
func TestWithParamOverrides(t *testing.T)        // errors.Is 保持；Body().Param=="new_password"；哨兵默认 param 不被污染
```

- [ ] **Step 2: 跑红** — `go test ./internal/common/apierror/`，期望编译失败（包不存在）。
- [ ] **Step 3: 实现** — `Param` 无 omitempty，nil 自然输出 `null`。
- [ ] **Step 4: 跑绿** — `go test ./internal/common/apierror/ -v` 全 PASS；`go build ./...`；`gofmt -l internal/common/apierror/` 无输出。
- [ ] **Step 5: Commit** — `feat(backend): 新增 apierror 错误注册表包`

### Task 2: guard 六处与 NoRoute 兜底接入 apierror

**Files:**
- Modify: `backend/internal/common/guard/guard.go:16`、`backend/internal/common/guard/permission.go:32,40,43,56,65`
- Modify: `backend/internal/app/app.go`（New 内加 `engine.NoRoute(...)`）
- Test: `backend/internal/common/guard/guard_test.go`、`backend/internal/app/app_test.go`

**Interfaces:**
- Consumes: Task 1 的 `ErrUnauthorized`/`ErrForbidden`/`ErrInternal`/`ErrNotFound`/`Write`/`Abort`。
- Produces: guard 失败响应为标准错误信封；未匹配路由（含方法不匹配）404 + `not_found` 信封；不启用 `HandleMethodNotAllowed`。

- [ ] **Step 1: 改测试到期望新行为** — guard_test 的 `decodeErrorBody`（现直接取 `body["error"]`）与 `body["error"] != "unauthorized"` 类扁平断言改解嵌套取 `error.code`；app_test 新增：

```go
func TestNoRouteReturnsErrorEnvelope(t *testing.T) // GET /api/v1/nope → 404，error.code=="not_found"
```

- [ ] **Step 2: 跑红** — `go test ./internal/common/guard/ ./internal/app/`。
- [ ] **Step 3: 实现** — 六处 `gin.H{"error": ...}` → `apierror.Abort(c, apierror.ErrUnauthorized|ErrForbidden|ErrInternal)`（LoadIdentity 销毁会话逻辑原位保留）；app.go 挂 `engine.NoRoute` 用 `apierror.Write(c, apierror.ErrNotFound)`。
- [ ] **Step 4: 跑绿** — 同 Step 2 全 PASS。

> 注（预期中间态）：auth_test / access_test 的扁平错误断言会短暂变红，由 Task 3/4 收口；Task 5 全量验证是首个全绿点。

- [ ] **Step 5: Commit** — `refactor(backend): guard 与 NoRoute 接入 apierror`

### Task 3: auth 错误收敛、current_password 改名、204→200

**Files:**
- Modify: `backend/internal/modules/auth/dto.go`（`PasswordChangeRequest` 改 json 标签 `current_password`/`new_password`；删 `ErrorResponse`）
- Modify: `backend/internal/modules/auth/controller.go`（错误分支收敛为 `apierror.Write`；logout/changePassword 204→200 `{}`）
- Modify: `backend/internal/modules/auth/service.go`（删本地哨兵 var 块，改返回 apierror 哨兵）
- Test: `backend/internal/modules/auth/auth_test.go`、`backend/internal/app/app_test.go`

**Interfaces:**
- Consumes: Task 1 auth 相关哨兵。
- Produces: `auth.Service` 方法签名不变；错误全为 apierror 哨兵。**映射 1:1**：改密旧密码错**仍为 `ErrInvalidCredentials`（401）**，不拆新码；请求体 `{"current_password":"...","new_password":"..."}`；logout/changePassword 成功 200 + `{}`。

- [ ] **Step 1: 改测试** — auth_test `decodeError` 改解嵌套（名字保留）；204→200：`TestLogoutDestroysSession`（:308）、`TestChangePasswordSucceedsAndSwapsCredentials`（:621）、`TestChangePasswordRevokesOtherSessions`（:752）、app_test `TestAuthFlowEndToEnd` logout 断言（:160-161）与 `TestChangePasswordToleratesBadSessionRows`（:308）；JSON 字面量键改名：auth_test :799、app_test :299 的 `"currentPassword"`→`"current_password"`（结构体引用编译期跟随）；`TestChangePasswordRejectsBadInput/wrong current password` **保持 401 + `invalid_credentials` 不动**（Review Focus 3 的钉子）。
- [ ] **Step 2: 跑红** — `go test ./internal/modules/auth/ ./internal/app/`。
- [ ] **Step 3: 实现** — 五个 handler 错误路径统一 `if err != nil { apierror.Write(ctx, err); return }`（me 的 `guard.IdentityFrom` 失败分支 → `apierror.ErrInternal`）；logout/changePassword 成功 → `ctx.JSON(http.StatusOK, gin.H{})`；service 哨兵替换（`ErrInvalidCredentials`→`apierror.ErrInvalidCredentials` 等，无拆分）。
- [ ] **Step 4: 跑绿** — 同 Step 2 全 PASS（`TestLoginFailsWhenSessionStoreDeleteFails` 等 500 用例仍过）。
- [ ] **Step 5: Commit** — `refactor(backend): auth 接入 apierror 并规范化负载`

### Task 4: access 错误收敛、preset_group_locked 409、删除确认体

**Files:**
- Modify: `backend/internal/modules/access/service.go`（删本地 `Err*` 哨兵 var 块，改返回 apierror 哨兵；`ErrInvalidName` → `apierror.ErrInvalidRequest`）
- Modify: `backend/internal/modules/access/repository.go`（`ErrNotFound`/`ErrGroupNameTaken`/`ErrGroupNotEmpty` → apierror 哨兵）
- Modify: `backend/internal/modules/access/controller.go`（删 `errorResponse`/`writeInvalidRequest`/`writeError`/`operatorFrom` 错误分支；deleteGroup 确认体）
- Test: `backend/internal/modules/access/access_test.go`、`backend/internal/modules/access/service_test.go`

**Interfaces:**
- Consumes: Task 1 access 相关哨兵。
- Produces: `access.Service` 方法签名不变；`uuid.Parse` 失败路径**维持 `ErrNotFound`（404）不拆分**（范围裁定）；`preset_group_locked` 由注册表给出 **409**；`DELETE /admin/groups/:id` 成功 → `ctx.JSON(200, gin.H{"id": ctx.Param("id"), "deleted": true})`。

- [ ] **Step 1: 改测试** — access_test `decodeErrorCode` 改解嵌套（名字保留）；`TestGroupUpdateAndDelete`：`locked` 断言 403→409（code 不变），删除成功断言 200 + `deleted==true` + `id` 回显；service_test：`Err*`→`apierror.Err*`（`ErrInvalidName`→`apierror.ErrInvalidRequest`；import 调整）；`DeleteGroup(ctx, "not-a-uuid")` 的期望**保持 `ErrNotFound` 不动**（:267-268，范围裁定）；`guard.ErrUserNotFound` 断言不动。
- [ ] **Step 2: 跑红** — `go test ./internal/modules/access/`。
- [ ] **Step 3: 实现** — controller 所有 `writeError(ctx, err)` → `apierror.Write(ctx, err)`；`operatorFrom` 失败分支 → `apierror.ErrInternal`；service/repository 哨兵替换（parse 失败路径不动）。
- [ ] **Step 4: 跑绿** — 同 Step 2（`TestConcurrentReplaceKeepsBothWritesServing`、`TestGroupCreateConcurrentSameName` 必须仍过）。
- [ ] **Step 5: Commit** — `refactor(backend): access 接入 apierror，preset_group_locked 改判 409 并补删除确认体`

### Task 5: 版本 0.7.0、README 同步与全量验证

**Files:**
- Modify: `backend/internal/version/version.go`、`backend/README.md`（经 `scripts/bump-version.sh minor`）
- Test: 全仓

**Interfaces:**
- Consumes: Task 1–4 的最终 API 形态。
- Produces: 无代码接口。

- [ ] **Step 1: 版本** — `bash scripts/bump-version.sh minor`（同步 version.go → 0.7.0 与 README 顶部「当前版本」行）；`go test ./internal/config/ ./internal/modules/health/` 防回归。
- [ ] **Step 2: README 同步** — 只处理与本次范围相关的命中：硬性规则 3 的 gin 允许位置清单补 `internal/common/apierror`；「认证」「用户组与权限」章节错误示例改新信封、`204` 表述改 200、`currentPassword` → `current_password`、`preset_group_locked` 403→409；**不碰** healthy 示例与 `items`（本轮未改）。
- [ ] **Step 3: 全量验证** —

```bash
export PATH="/f/software/go/bin:$PATH"
cd backend && set -a && . ./.env && set +a && gofmt -l . && go vet ./... && go test ./...
```

期望：gofmt 无输出、vet 干净、无 FAIL 且非全 SKIP。
- [ ] **Step 4: Commit** — `chore(backend): 版本进位 0.7.0，README 按 v1.3 错误规范同步`
