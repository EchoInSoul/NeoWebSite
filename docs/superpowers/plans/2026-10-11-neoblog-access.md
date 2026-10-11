# NeoBlog 用户组与权限（层 2）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给 NeoBlog 后端加上用户组与权限：三个预设组（owner / admin / 普通用户）+ 自建组、组权限、用户额外权限、按权限点拦截的管理接口与守卫。

**Architecture:** 权限目录是 Go 常量（`internal/common/rbac`）；预设组权限由代码计算（owner/admin = 全量、普通用户 = 空），只有自建组把权限写进 `group_permissions`；一个用户一个组（`users.group_id` 非空外键 + ent 字段级默认值指向「普通用户」，所以创建路径零改动）；用户额外权限只能加不能减，生效权限 = 组 ∪ 额外（服务端做蕴含展开）。守卫分两级：`RequireAuth`（既有）+ `LoadIdentity`（查一次身份）/ `RequirePermission`（读 context 判定）；新模块 `internal/modules/access` 承载组 CRUD、用户管理接口与身份解析，装配只在 `app.go`。

**Tech Stack:** Go 1.27 + Gin + ent v0.14.6（`--feature sql/lock`）+ Atlas + scs + Postgres。

**Spec:** `docs/superpowers/specs/2026-10-11-neoblog-access-design.md`（本计划实现它；执行者两份都要读，含 v2 修订记录）

## Global Constraints

- **开工前先同步 HEAD**：仓库有并行会话在推进其他层（认证层 2 已落地：注册 / 改密码 / 邮件层；`app.go`、`README.md`、版本号可能同时被改动）。改动只加不删，提交前 `git diff` 只应包含本任务的改动文件，禁止 `git add -A`。
- 所有命令的工作目录是 `backend/`（`go test ./internal/...`、`bash scripts/*.sh`、`./scripts/bump-version.sh`、`atlas --dir "file://migrations"` 都是相对路径）。
- 本机 Git Bash 先执行：`export PATH="/f/software/go/bin:/f/software/gopath/bin:$PATH" GOPATH='F:/software/gopath' GOCACHE='F:/software/gocache' GOPROXY='https://goproxy.cn,direct' DATABASE_URL='postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog?sslmode=disable' DATABASE_URL_TEST='postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_test?sslmode=disable'`（`db-migrate-apply.sh` 有 `set -u`，两个 URL 必须导出，它自己不读 `.env`）。
- 手写 Go 文件**零注释**（ent 生成物与工具链指令豁免）；`gofmt -l .` 必须为空。
- 依赖方向 controller → service → repository；`internal/database` 不得 import 任何业务模块（`internal/common/rbac` 属 common，允许被 schema import）；业务模块禁止自建 `sql.Open`；service 不 import gin。
- `/api/v1` 前缀只在 `app.go` 出现一次；模块能力只通过 `New(...) *Module` 暴露，装配只在 `app.go`。
- **Task 2 与 Task 3 之间禁止带库跑测试**：schema 重生成后 ent 的 INSERT 会带上 `group_id`，而库里还没有该列——直到 Task 3 应用迁移为止，任何带 `DATABASE_URL_TEST` 的测试都会红。带库跑全量测试用 `go test -p 1 ./...`（access 与 app 的用例都会造 owner，包级并行会互相干扰）。
- `atlas migrate diff` 会临时创建 `backend/.entc/`（正常自删）；中断残留时不要 `git add` 它。手改迁移 SQL 后必须 `atlas migrate hash` + `atlas migrate validate`。
- 集成测试约定：读 `DATABASE_URL_TEST`，缺省 `t.Skip`；跑集成测试前先应用迁移；测试数据带随机后缀，`t.Cleanup` 清理；**不删预设组**。
- 提交信息中文、conventional 前缀（`feat(backend): ...` / `docs(backend): ...` / `chore(backend): ...`），每个任务一次提交。
- 版本：Task 8 执行 `./scripts/bump-version.sh minor`（当前 0.3.1 → 0.4.0；若届时已被其他层进位，按当前版本顺延，DEVLOG 记实际版本）。
- 固定值（计划内唯一来源，迁移 SQL 与代码必须一致）：预设组 UUID —— owner `00000000-0000-4000-8000-000000000001`、admin `...0002`、普通用户 `...0003`；预设组 key —— `owner` / `admin` / `user`；组显示名 —— 「所有者」「管理员」「普通用户」；权限点 —— `system.users.view` 查看用户、`system.users.edit` 管理用户、`system.groups.view` 查看用户组、`system.groups.edit` 管理用户组。

## Review Focus

1. 组转移的 grant 差集：单请求内「设组 + 改额外权限」合并成一次差集（`next 生效集 − current 生效集`，两侧先展开），非全量操作者把用户设到含其未持有权限的自建组必须 403 `grant_forbidden`——T5 / T6 覆盖。
2. `last_owner` 的顺序与可达性：检查顺序固定 `cannot_modify_self` → `owner_protected` → `last_owner`（403 优先于 409）；串行路径打不出 409，唯一可达是并发双降级——T5 覆盖。
3. owner 保护两向：非 owner 把别人设成 owner、非 owner 改 owner 组成员，都 403 `owner_protected`——T5 / T6 覆盖。
4. `edit` 蕴含 `view`：只勾 `edit` 的组能访问只要 `view` 的端点——T6 覆盖。
5. 并集语义下的「撤权假象」：从用户摘掉额外权限、但组里仍有该权限时，生效权限不变——T5 / T6 覆盖。

## 文件结构

| 文件 | 职责 |
| --- | --- |
| `backend/internal/common/rbac/rbac.go` | 权限目录、动作蕴含展开、预设组常量与固定 UUID（纯 Go，无 gin/DB） |
| `backend/internal/database/ent/schema/group.go` | 组表 |
| `backend/internal/database/ent/schema/group_permission.go` | 组-权限关联表 |
| `backend/internal/database/ent/schema/user_permission.go` | 用户额外权限表 |
| `backend/internal/database/ent/schema/user.go`（改） | `group_id` 外键（字段级默认值）+ 两条边 + 索引 |
| `backend/migrations/<ts>_groups_and_permissions.sql` | 建表 + 预设组 seed + `users.group_id` 回填（手工改造） |
| `backend/internal/common/guard/permission.go`（新） | `Identity` / `IdentityResolver` / `LoadIdentity` / `RequirePermission` / `IdentityFrom` |
| `backend/internal/modules/access/{dto,repository,service,controller,module}.go` | 组 CRUD、用户管理、身份解析与生效权限 |
| `backend/internal/modules/access/{migration_test,service_test,access_test}.go` | 迁移断言 / service 集成 / HTTP 集成 |
| `backend/internal/modules/auth/{dto,controller}.go`（改） | `/auth/me` 带 group + permissions |
| `backend/internal/app/app.go`（改） | 装配 access 模块与身份守卫 |

---

### Task 1: `internal/common/rbac` 权限目录包

**Files:**
- Create: `backend/internal/common/rbac/rbac.go`
- Create: `backend/internal/common/rbac/rbac_test.go`

**Interfaces:**
- Produces:
  - `const PermissionUsersView = "system.users.view"`、`PermissionUsersEdit = "system.users.edit"`、`PermissionGroupsView = "system.groups.view"`、`PermissionGroupsEdit = "system.groups.edit"`
  - `type Permission struct { Key string; Name string }`
  - `func Catalog() []Permission`（顺序：users.view、users.edit、groups.view、groups.edit）
  - `func All() []string`；`func IsValid(key string) bool`
  - `const GroupKeyOwner = "owner"`、`GroupKeyAdmin = "admin"`、`GroupKeyUser = "user"`
  - `type PresetGroup struct { ID uuid.UUID; Key string; Name string }`
  - `var GroupIDOwner / GroupIDAdmin / GroupIDUser uuid.UUID`（固定值见 Global Constraints）
  - `func PresetGroups() []PresetGroup`（顺序 owner、admin、user）；`func PresetGroupByKey(key string) (PresetGroup, bool)`；`func IsPresetGroupKey(key string) bool`
  - `func PresetPermissions(systemKey string) []string`——owner/admin 返回 `All()`，user 与其他返回空
  - `func Expand(keys []string) []string`——蕴含展开（`edit` → `view`），去重保序（先原序、再补展开项）

- [ ] **Step 1: 写失败单测**

`rbac_test.go`（不连库）：`Catalog()` 四项且 key/name 与 Global Constraints 一致；`All()` 与 `Catalog()` 同源；`IsValid` 对假 key（如 `content.posts.edit`）返回 false；`PresetGroups()` 三个、UUID 与 key 对应且互不相同；`PresetPermissions("owner")` 与 `("admin")` 都等于 `All()`、`("user")` 为空；`Expand(["system.users.edit","system.groups.edit"])` 含四个 key 且无重复；`Expand(["system.users.view"])` 原样。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/common/rbac/ -count=1`
Expected: 编译失败（标识符未定义）。

- [ ] **Step 3: 实现 `rbac.go`**

零注释。key 形如 `模块.动作`；动作 `edit` 蕴含 `view`（生成 `模块.view`）。固定 UUID 用 `uuid.MustParse` 构造。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/common/rbac/ -count=1`，Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/common/rbac
git commit -m "feat(backend): rbac 权限目录与预设组常量"
```

---

### Task 2: ent schema（groups / group_permissions / user_permissions + users.group_id）

**Files:**
- Create: `backend/internal/database/ent/schema/group.go`
- Create: `backend/internal/database/ent/schema/group_permission.go`
- Create: `backend/internal/database/ent/schema/user_permission.go`
- Modify: `backend/internal/database/ent/schema/user.go`
- Modify: `backend/scripts/db-generate.sh`
- Generated（提交）: `backend/internal/database/ent/**`

**Interfaces:**
- Consumes: Task 1 的 `rbac.GroupIDUser`。
- Produces: `ent.Group` / `ent.GroupPermission` / `ent.UserPermission`；`ent.User` 新增字段 `GroupID` 与边 `Group` / `Permissions`（`Query().WithGroup()` / `WithPermissions()`）；`ent.Group` 有边 `Users` / `Permissions`；查询构造器 `group.SystemKeyEQ`、`user.GroupIDEQ`、`grouppermission.GroupIDEQ` 等。

- [ ] **Step 1: 写三个新 schema（零注释）**

`group.go`：字段 `id`（UUIDv7 默认，同 `user.go` 写法）、`system_key`（String、Optional、Unique）、`name`（String、NotEmpty、Unique）、`created_at` / `updated_at`（同 `user.go`）；边 `edge.To("users", User.Type())`、`edge.To("permissions", GroupPermission.Type()).Annotations(entsql.OnDelete(entsql.Cascade))`（`entsql` 来自 `entgo.io/ent/dialect/entsql`，注解必须挂 `edge.To` 侧）。
`group_permission.go` / `user_permission.go`：字段 `id`（UUIDv7）、`group_id`（或 `user_id`，UUID）、`permission`（String、NotEmpty）、`created_at`；边 `edge.From("group", Group.Type()).Ref("permissions").Unique().Required().Field("group_id")`（`user_permission.go` 对 `User` 同理，字段 `user_id`）；索引 `index.Fields("group_id", "permission").Unique()`（`user_permission.go` 为 `user_id, permission`）。

- [ ] **Step 2: 改 `user.go`**

Fields 增加 `field.UUID("group_id", uuid.UUID{}).Default(func() uuid.UUID { return rbac.GroupIDUser })`（import `website/backend/internal/common/rbac`；UUID 字段的默认值**只能**用函数形式，这是 ent 要求的唯一写法）；Edges 增加 `edge.From("group", Group.Type()).Ref("users").Unique().Required().Field("group_id")` 与 `edge.To("permissions", UserPermission.Type()).Annotations(entsql.OnDelete(entsql.Cascade))`；Indexes 增加 `index.Fields("group_id")`。

- [ ] **Step 3: 改 `scripts/db-generate.sh` 并重新生成**

命令行改为 `go run entgo.io/ent/cmd/ent generate --feature sql/lock ./internal/database/ent/schema`（last-owner 事务要用 `ForUpdate`）。
Run: `bash scripts/db-generate.sh`
Expected: `internal/database/ent/` 出现 `group*.go`、`userpermission*.go`；`grep -r ForUpdate internal/database/ent | head` 有命中。

- [ ] **Step 4: 全量检查（不带库）**

Run: `gofmt -l . && go vet ./... && go build ./...`
Expected: 全绿。**本步不跑带库测试**（库里还没有 `group_id` 列，见 Global Constraints）。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/database backend/scripts/db-generate.sh
git commit -m "feat(backend): 用户组与权限 ent schema（含 group_id 默认组）"
```

---

### Task 3: 迁移（建表 + 预设组 seed + `users.group_id` 回填）

**Files:**
- Modify: `backend/scripts/db-migrate-diff.sh`
- Generated + 手工改造: `backend/migrations/<时间戳>_groups_and_permissions.sql`、`backend/migrations/atlas.sum`
- Create: `backend/internal/modules/access/migration_test.go`（本任务只建这一个测试文件：`testDB` 辅助 + 迁移后置断言；后续任务复用辅助函数）

**Interfaces:**
- Consumes: Task 1 的固定 UUID / key；Task 2 的 schema。
- Produces: 两个库含 `groups` / `group_permissions` / `user_permissions` 表、三个预设组行、`users.group_id` 非空外键。

- [ ] **Step 1: 改脚本迁移名**

`db-migrate-diff.sh` 第 7 行 `atlas migrate diff init_users_sessions ...` 改为 `atlas migrate diff groups_and_permissions ...`。

- [ ] **Step 2: 生成迁移并人工审阅**

Run: `bash scripts/db-migrate-diff.sh`
Expected: 出现 `migrations/<ts>_groups_and_permissions.sql`。逐行审阅：三张表的列、唯一索引、两处 `ON DELETE CASCADE`、`users.group_id` 与外键。

- [ ] **Step 3: 手工改造 SQL**

按生成文件的实际内容改（标识符带 `"public"` 限定，不要做字面匹配；`ADD COLUMN` 与 `ADD CONSTRAINT` 可能是两条独立语句）：
1）在 `CREATE TABLE "groups"` 之后、任何引用它之前，插入预设组 seed：

```sql
INSERT INTO "public"."groups" ("id", "system_key", "name", "created_at", "updated_at") VALUES
('00000000-0000-4000-8000-000000000001', 'owner', '所有者', now(), now()),
('00000000-0000-4000-8000-000000000002', 'admin', '管理员', now(), now()),
('00000000-0000-4000-8000-000000000003', 'user', '普通用户', now(), now())
ON CONFLICT ("id") DO NOTHING;
```

2）把 `ALTER TABLE ... ADD COLUMN "group_id" uuid NOT NULL` 替换成三步（确认 `UPDATE` 覆盖全表后再 `SET NOT NULL`）：

```sql
ALTER TABLE "public"."users" ADD COLUMN "group_id" uuid;
UPDATE "public"."users" SET "group_id" = '00000000-0000-4000-8000-000000000003' WHERE "group_id" IS NULL;
ALTER TABLE "public"."users" ALTER COLUMN "group_id" SET NOT NULL;
```

3）确认外键约束语句在回填之后（若被并进同一条 ALTER，就拆出来放到三步之后）。

- [ ] **Step 4: 写迁移后置断言（先失败）**

创建 `backend/internal/modules/access/migration_test.go`（`package access`，只有测试文件不影响 `go build`）：内含 `testDB(t *testing.T) *database.Database` 辅助（读 `DATABASE_URL_TEST`，缺省 `t.Skip`，`t.Cleanup` 关连接；供后续任务复用）与 `TestMigrationSeed`：
1. 三个预设组按 Global Constraints 的 UUID / `system_key` / 名称存在；
2. 三个预设组在 `group_permissions` 里的行数为 0（预设组权限由代码算）；
3. 用 ent 建一个**不设 group** 的用户（`email` / `nickname` / `password_hash` 三个必填字段都要给，邮箱带随机后缀）→ 落 `rbac.GroupIDUser`（Task 2 字段默认值的实证）；`t.Cleanup` 删掉该用户。

Run: `go test ./internal/modules/access/ -count=1`
Expected: FAIL（库里还没有表，报 `relation "groups" does not exist` 或 `column "group_id" does not exist` 都符合预期）。

- [ ] **Step 5: 重算校验和并应用，核对**

Run: `atlas migrate hash --dir "file://migrations" && atlas migrate validate --dir "file://migrations" && bash scripts/db-migrate-apply.sh`
Expected: hash / validate 通过；apply 对两个库各输出一个迁移 ok。核对：
`wsl -d Ubuntu -- docker exec neoblog-postgres psql -U neoblog -d neoblog_test -c "SELECT system_key, name FROM groups ORDER BY system_key;"` → 三行。

- [ ] **Step 6: 跑测试确认通过**

Run: 同 Step 4，Expected: PASS。

- [ ] **Step 7: Commit**

```bash
git add backend/migrations backend/scripts/db-migrate-diff.sh backend/internal/modules/access
git commit -m "feat(backend): 用户组与权限迁移（预设组 seed 与 group_id 回填）"
```

---

### Task 4: 守卫扩展 `LoadIdentity` / `RequirePermission`

**Files:**
- Create: `backend/internal/common/guard/permission.go`
- Modify: `backend/internal/common/guard/guard_test.go`（新增用例，保留原有）

**Interfaces:**
- Produces:
  - `const SessionIdentityKey = "auth.identity"`
  - `type Identity struct { UserID string; GroupID string; GroupKey string; GroupName string; Permissions []string }`（`Permissions` 为已展开的生效权限）
  - `var ErrUserNotFound = errors.New("user not found")`
  - `type IdentityResolver interface { IdentityByUserID(ctx context.Context, userID string) (Identity, error) }`
  - `func LoadIdentity(resolver IdentityResolver, sm *scs.SessionManager) gin.HandlerFunc`——无会话 → 401 `unauthorized`；resolver 返回 `ErrUserNotFound` → `sm.Destroy` + 401；其他错误 → 500 `internal_error`；成功 → `c.Set(SessionIdentityKey, identity)` 放行
  - `func RequirePermission(permission string) gin.HandlerFunc`——context 无身份 → 500 `internal_error`（装配错误）；权限不足 → 403 `forbidden`；通过 → 放行
  - `func IdentityFrom(ctx *gin.Context) (Identity, bool)`

- [ ] **Step 1: 写失败单测**

`guard_test.go` 新增（不连库，内存 scs + fake resolver + httptest）：
1. 未登录过 `LoadIdentity` → 401 `unauthorized`；
2. fake resolver 返回 `ErrUserNotFound` → 401，**且响应带清除会话的 cookie**（断言 `Set-Cookie` 的 `Max-Age < 0`；不要用「第二次请求仍 401」代替——那不能证明会话被销毁）；
3. resolver 返回普通错误 → 500 `internal_error`；
4. 不挂 `LoadIdentity` 只挂 `RequirePermission` → 500 `internal_error`；
5. `LoadIdentity` 通过后 `IdentityFrom(ctx)` 返回的身份与 resolver 一致（含 `Permissions` 内容）；对含 `system.users.edit` 的身份请求该权限 → 200；对不含的身份 → 403 `forbidden`。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/common/guard/ -count=1`，Expected: 编译失败。

- [ ] **Step 3: 实现 `permission.go`**

零注释；延续 `RequireAuth` 的写法（`c.AbortWithStatusJSON` + `gin.H{"error": ...}`）；`Destroy` 用请求 context。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/common/guard/ -count=1`，Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/common/guard
git commit -m "feat(backend): 身份加载与权限守卫（LoadIdentity/RequirePermission）"
```

---

### Task 5: access 模块 service 与 repository（全部规则）

**Files:**
- Create: `backend/internal/modules/access/dto.go`
- Create: `backend/internal/modules/access/repository.go`
- Create: `backend/internal/modules/access/service.go`
- Create: `backend/internal/modules/access/service_test.go`（复用 Task 3 的 `testDB` 辅助）

**Interfaces:**
- Consumes: Task 1 的 `rbac`、Task 2 的 ent 类型、Task 3 的迁移与预设组、Task 4 的 `guard.Identity` / `guard.ErrUserNotFound`。
- Produces（包 `access`，零注释）：

```go
type GroupRef struct { ID string; SystemKey *string; Name string }
type Group struct { ID string; SystemKey *string; Name string; IsPreset bool; MemberCount int; Permissions []string }
type UserSummary struct { ID string; Email string; Nickname string; Group GroupRef; ExtraPermissions []string }
type UserDetail struct { UserSummary; Permissions []string }
type UpdateGroupInput struct { Name *string; Permissions *[]string }
type UpdateUserInput struct { GroupID *string; ExtraPermissions *[]string }

var ErrNotFound, ErrGroupNotEmpty, ErrGroupNameTaken, ErrPresetGroupLocked,
    ErrInvalidName, ErrInvalidPermission, ErrGrantForbidden, ErrOwnerProtected,
    ErrCannotModifySelf, ErrLastOwner error

func newService(db *database.Database) *Service
func (s *Service) IdentityByUserID(ctx context.Context, userID string) (guard.Identity, error)
func (s *Service) ListGroups(ctx context.Context) ([]Group, error)
func (s *Service) CreateGroup(ctx context.Context, operator guard.Identity, name string, permissions []string) (Group, error)
func (s *Service) UpdateGroup(ctx context.Context, operator guard.Identity, groupID string, input UpdateGroupInput) (Group, error)
func (s *Service) DeleteGroup(ctx context.Context, groupID string) error
func (s *Service) ListUsers(ctx context.Context) ([]UserSummary, error)
func (s *Service) GetUser(ctx context.Context, userID string) (UserDetail, error)
func (s *Service) UpdateUser(ctx context.Context, operator guard.Identity, userID string, input UpdateUserInput) (UserDetail, error)
```

- [ ] **Step 1: 写失败的集成测试**

`service_test.go`（真库；复用 `testDB`；造用户用 `db.Client.User.Create().SetEmail(...).SetNickname(...).SetPasswordHash(...).SetGroupID(...)`——三个字段都是必填，邮箱带随机后缀；`t.Cleanup` 删用户与自建组）。用例清单（名称即测试函数名）：
1. `TestIdentityByUserID`：owner 组用户 → `GroupKey == "owner"`、`Permissions` 等于 `rbac.All()`；自建组用户（组勾 `system.users.edit`）→ 含 `edit` 与 `view`（展开）；含额外权限 → 并集；用户不存在 → `guard.ErrUserNotFound`。
2. `TestCreateGroup`：正常建组（勾 `system.users.view`）→ `IsPreset=false`；未知 key → `ErrInvalidPermission`；名字撞预设名「管理员」→ `ErrGroupNameTaken`；名字为空白串 / 超 64 字符 → `ErrInvalidName`；操作者只持 `system.users.view` 时勾 `system.users.edit` → `ErrGrantForbidden`。
3. `TestUpdateGroup`：预设组任意改动 → `ErrPresetGroupLocked`；自建组改名 / 改权限成功；只对**新增**权限做 grant 校验；**摘掉自己没有的权限允许**（摘除不受限）；**操作者改自己所在的自建组允许**；改权限后组成员生效权限立即变化。
4. `TestDeleteGroup`：预设组 → `ErrPresetGroupLocked`；有成员的自建组 → `ErrGroupNotEmpty`；空组删除成功且权限行级联消失。
5. `TestUpdateUser`：操作者是目标自己（`operator.UserID` 等于目标 ID）→ `ErrCannotModifySelf`（用唯一 owner 自降复现，断言是这个 403 而不是 `ErrLastOwner`——检查顺序 403 优先）；非 owner 把用户设到 owner / admin 组、非 owner 改 owner 组成员 → `ErrOwnerProtected`；owner 操作成功；把唯一 owner 移出 → **串行不可达**（被 `ErrCannotModifySelf` / `ErrOwnerProtected` 挡住，两条都断言）；**组转移的 grant 差集**：操作者只持 `system.users.edit`、把用户设到含 `system.groups.edit` 的自建组 → `ErrGrantForbidden`；同一请求同时设组 + 加额外权限时按合并差集判定（组给的 + 额外给的里有一条不是操作者的 → `ErrGrantForbidden`）；额外权限新增自己没有的点 → `ErrGrantForbidden`；摘掉额外权限总允许；未知 key → `ErrInvalidPermission`；`group_id` 不存在 → `ErrNotFound`。
6. `TestLastOwnerConcurrent`（Review Focus 2）：造两个 owner，两个 goroutine 同时把两人各自降级到普通用户 → 恰好一个成功、另一个 `ErrLastOwner`。
7. `TestEffectiveUnionRevoke`（Review Focus 5）：用户额外权限被移除、但组里仍有该权限 → 生效权限不变。
8. `TestListUsersAndGetUser`：列表含 group 与额外权限、按创建时间倒序；详情含生效权限；自建组的 `SystemKey` 为 `nil`。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/modules/access/ -count=1`（已导出 `DATABASE_URL_TEST`）
Expected: 编译失败（service 未实现）。

- [ ] **Step 3: 实现 `dto.go` / `repository.go` / `service.go`**

要点（不写 gin、零注释）：
- `IdentityByUserID`：查用户 + `WithGroup()` + `WithPermissions()`；组权限按 `system_key` 走 `rbac.PresetPermissions`（owner/admin 全量、user 空），自建组读 `group_permissions`；并入 `user_permissions`；`rbac.Expand` 后返回。
- **grant 差集（Review Focus 1）**：`next = 下一组权限 ∪ 下一额外权限`、`current = 当前组权限 ∪ 当前额外权限`（两侧先 `rbac.Expand`），`added = next − current`，凡 `added` 里不在 `operator.Permissions` 中的 → `ErrGrantForbidden`；同一请求里组与额外权限的变更**合并成一次差集**；摘除不校验。
- **检查顺序（Review Focus 2）**：`cannot_modify_self` → `owner_protected` → `last_owner`；`owner_protected` 的判据：目标当前组的 `SystemKey` 或目标下一组的 `SystemKey` 是 `owner`/`admin` 时，操作者的 `GroupKey` 必须是 `owner`。
- **last_owner 的精确规则**：`UpdateUser` 全程在一个事务里；若目标当前在 owner 组且目标组 ≠ owner 组，先在事务内 `tx.User.Query().Where(user.GroupIDEQ(rbac.GroupIDOwner)).ForUpdate().All(ctx)` 锁住 owner 成员行，锁到的行数 < 2 → `ErrLastOwner`。
- 约束映射（不要一刀切）：`groups.name` 唯一冲突 → `ErrGroupNameTaken`；`DeleteGroup` 触发 FK 冲突 → `ErrGroupNotEmpty`（先查成员数是快路径，FK 是兜底）；权限数组入库前**先去重**（避免 `(group_id, permission)` 唯一索引冲突）；其余 constraint error 不要吞。
- `DeleteGroup` 预设族判定按 `SystemKey != nil`；`ListGroups` 的 `MemberCount` 用一次 `groupBy`；`Permissions` 统一返回展开后的集合。

- [ ] **Step 4: 跑测试确认通过**

Run: 同 Step 2，Expected: PASS；再跑 `go test ./... -count=1`（不带库）确认集成用例 skip 且全绿。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/access
git commit -m "feat(backend): access 模块 service 与 repository（组/用户/权限规则）"
```

---

### Task 6: access 模块 HTTP 层（controller / module）

**Files:**
- Create: `backend/internal/modules/access/controller.go`
- Create: `backend/internal/modules/access/module.go`
- Create: `backend/internal/modules/access/access_test.go`

**Interfaces:**
- Consumes: Task 4 的 guard、Task 5 的 service 与哨兵错误。
- Produces：`func New(db *database.Database) *Module`（暴露 `Service *Service`）；`func (m *Module) RegisterProtected(r *gin.RouterGroup)`；路由见下。

- [ ] **Step 1: 写失败的 HTTP 测试**

`access_test.go`（httptest + 真库 + 内存 scs；仿 `auth_test.go` 的 mini 路由，挂真实 `guard.LoadIdentity` + `RequirePermission`，身份用真实用户）：
1. `GET /admin/permissions` → `{"items":[{"key","name"}...]}` 四项；
2. `GET /admin/groups` → 预设组 `is_preset=true`、`system_key` 非 null；owner / admin 的 `permissions` 等于目录全量、普通用户为空、`member_count` 正确；自建组 `system_key` 为 `null`；
3. `POST /admin/groups` 201 → **响应体按读口径（`permissions` 已展开：只勾 `system.users.edit` 返回含 `view`）**；重名 → 409 `group_name_taken`；未知 key → 400 `invalid_permission`；空名 / 超长名 → 400 `invalid_request`；非 `edit` 权限者 → 403 `forbidden`；
4. `PATCH /admin/groups/:id` 预设组 → 403 `preset_group_locked`；自建组改名 / 改组权限成功；空 body `{}` → 400 `invalid_request`；
5. `DELETE /admin/groups/:id` 成功 → 204；有成员 → 409 `group_not_empty`；不存在 → 404 `not_found`；
6. `GET /admin/users`、`GET /admin/users/:id` 形状与 spec §9 一致；未知 id → 404；
7. `PATCH /admin/users/:id`：设组成功；额外权限全量替换（Review Focus 5：`[]` 清空后组里还有的权限仍在）；**Review Focus 3**：非 owner 把用户设成 owner、非 owner 改 owner 组成员 → 403 `owner_protected`；改自己 → 403 `cannot_modify_self`；**Review Focus 1**：把用户设到含操作者未持有权限的自建组 → 403 `grant_forbidden`；只给一个字段也成功；两个字段都不给 → 400；`group_id: ""` → 400；`extra_permissions` 里含未知 key → 400 `invalid_permission`；
8. **Review Focus 4**：只持 `system.users.edit`（组里没勾 view）的操作者 `GET /admin/users` → 200；
9. 未登录任一 admin 端点 → 401；普通用户（无权限）→ 403；
10. 并发 `POST /admin/groups` 同名两次 → 恰一个 201、另一个 409。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/modules/access/ -count=1`（已导出 `DATABASE_URL_TEST`）
Expected: 编译失败（controller / module 未定义）。

- [ ] **Step 3: 实现 controller / module**

要点：`registerProtected` 给每条路由挂 `guard.RequirePermission(...)`（读端点用 `rbac.Permission*View`，写端点用 `...Edit`）；操作者身份用 `guard.IdentityFrom(ctx)`（缺失即 500）；请求体 `ShouldBindJSON`（`PATCH` 的「两个字段都不给」与 `group_id: ""` 在 controller 判 400——空串与畸形 UUID 都归 400，UUID 解析失败不算 404）；`writeError(ctx, err)` 映射：`ErrNotFound`→404 `not_found`、`ErrInvalidName`→400 `invalid_request`、`ErrInvalidPermission`→400 `invalid_permission`、`ErrGroupNameTaken` / `ErrGroupNotEmpty` / `ErrLastOwner`→409、`ErrGrantForbidden` / `ErrOwnerProtected` / `ErrCannotModifySelf` / `ErrPresetGroupLocked`→403 `forbidden`（细则见 spec §9 错误码表）、其余→500；响应 json tag 用 snake_case（`system_key`、`is_preset`、`member_count`、`extra_permissions`），`system_key` 用 `*string`（自建组输出 `null`）。

- [ ] **Step 4: 跑测试确认通过**

Run: 同 Step 2，Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/access
git commit -m "feat(backend): access 模块 HTTP 层（组与用户管理接口）"
```

---

### Task 7: `/auth/me` 扩展 + app 装配与端到端

（合并「改 me 形状」与「装配」为一个任务：me 需要 `LoadIdentity` 提供身份，拆开会让中间提交点的 app 级测试无谓变红。）

**Files:**
- Modify: `backend/internal/modules/auth/dto.go`
- Modify: `backend/internal/modules/auth/controller.go`
- Modify: `backend/internal/modules/auth/auth_test.go`
- Modify: `backend/internal/app/app.go`
- Modify: `backend/internal/app/app_test.go`

**Interfaces:**
- Consumes: Task 4 的 `guard.LoadIdentity` / `guard.IdentityFrom` / `SessionIdentityKey`、Task 5/6 的 access 模块。
- Produces: `GET /api/v1/auth/me` 响应 `{"email","nickname","group":{"id","system_key","name"},"permissions":[...]}`（自建组时 `system_key` 为 `null`；`permissions` 空时为 `[]` 而不是 `null`）；`POST /auth/login`、`POST /auth/register` 响应**不变**；保护组 = `api.Group("", guard.RequireAuth(sessions), guard.LoadIdentity(accessModule.Service, sessions))`。

- [ ] **Step 1: 写失败测试**

- `auth_test.go`：`/auth/me` 断言补 `group` 与 `permissions`；mini 路由改为挂**真实 access service**（`access.New(db).Service`，不写 fake——认证层 2 的注册 / 改密用例依赖运行时真实用户），`CreateAdminUser` 建的用户 → `group.system_key == "user"`、`permissions` 为 `[]`；已有的登录 / 注册 / 改密断言保持通过。
- `app_test.go` 追加：造 owner 用户与普通用户（`SetGroupID(rbac.GroupIDOwner / GroupIDUser)`，三个必填字段都给）：
  1. 普通用户登录后 `GET /api/v1/admin/users` → 403；`GET /api/v1/auth/me` → 200 且 `group.system_key == "user"`、`permissions == []`；
  2. owner 登录后 `GET /api/v1/admin/users` → 200；`PATCH /api/v1/admin/users/:id` 把普通用户设到自建组 → 200，该用户下一次请求生效权限变化；
  3. 用户被删除后带旧 cookie 访问 `/auth/me` → 401；
  4. 未登录访问任一 admin 端点 → 401；`GET /api/v1/healthy` 不受影响。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/app/ ./internal/modules/auth/ -count=1`（已导出 `DATABASE_URL_TEST`）
Expected: FAIL（me 缺字段 / admin 路由 404）。

- [ ] **Step 3: 实现**

- `auth/dto.go` 增加 `MeResponse`、`GroupResponse`（`SystemKey *string`，json tag `system_key`）；`auth/controller.go` 的 `me`：身份取 `guard.IdentityFrom(ctx)`（缺失即 500——装配保证有），用 `identity.UserID` 查 email / nickname（`LoadIdentity` 已保证用户存在），`permissions` 空时输出 `[]`。
- `app.go`：`accessModule := access.New(db)`；保护组改为 `api.Group("", guard.RequireAuth(sessions), guard.LoadIdentity(accessModule.Service, sessions))`；在保护组上注册 auth 与 access（`LoadIdentity` 必须挂在 `RequireAuth` 之后）。**只加不删**，保留并行会话对 `app.go` 的其他改动。

- [ ] **Step 4: 跑测试确认通过**

Run: 同 Step 2，Expected: PASS；再跑 `go test -p 1 ./... -count=1` 与带库的 `go test -p 1 ./... -count=1`，两次全绿。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/auth backend/internal/app
git commit -m "feat(backend): /auth/me 返回所属组与生效权限，装配权限守卫"
```

---

### Task 8: README、DEVLOG、版本进位与终验

**Files:**
- Modify: `backend/README.md`（**增量改**：当前文件已含认证层 2 与邮件层的内容，不要覆盖）
- Modify: `docs/DEVLOG.md`（顶部新条目）
- Modify: `backend/internal/version/version.go`（脚本改）

- [ ] **Step 1: 更新 README**

目录对照表加 `internal/modules/access/*` 与 `internal/common/rbac`（纯 Go 权限目录）；guard 行改为「会话守卫 + 身份加载 + 权限守卫」；数据库章节写：迁移内含预设组 seed、手改 SQL 后需 `atlas migrate hash` / `validate`、diff 脚本的迁移名；「认证」章节改「认证与权限」，补接口表（spec §9 的 9 条路由 + `/auth/me` 新形状 + `DELETE` 204）、401/403 与错误码语义、预设组与「额外权限只能加不能减 / 摘除不受限」的说明、守卫原则变更（原「守卫不查库」扩展为「守卫只依赖注入进来的 resolver，不 import 业务模块」）、以及**首个 owner 的手工指定步骤**（SQL 形态，见 spec §13）。

- [ ] **Step 2: 提交文档**

```bash
git add backend/README.md
git commit -m "docs(backend): 用户组与权限文档（接口/迁移/守卫原则）"
```

- [ ] **Step 3: DEVLOG 条目**

`docs/DEVLOG.md` 顶部（分隔线后）加 `## 2026-10-11 · 用户组与权限（层 2）`，含实际版本（0.3.1 → 0.4.0）、做了什么、已知缺口（首次建号未定、不吊销会话、无审计、列表不分页、在途请求用旧权限快照）、关键决定（预设组权限代码计算不入库、owner/admin 同权差在保护规则、额外权限只能加不能减、摘除不受限、首个 owner 手工指定）。
```bash
git add docs/DEVLOG.md
git commit -m "docs(backend): 新增开发日志（2026-10-11 用户组与权限）"
```

- [ ] **Step 4: 版本进位**

Run: `./scripts/bump-version.sh minor`（当前 0.3.1 → 0.4.0；若已被其他层进位则顺延，commit message 写实际版本）
```bash
git add backend/internal/version/version.go backend/README.md
git commit -m "chore(backend): 版本进位 0.4.0（用户组与权限）"
```

- [ ] **Step 5: 终验清单（spec §14）**

逐条执行并记录输出：`gofmt -l . && go vet ./... && go build ./...` 干净；`atlas migrate validate` 通过；`go test -p 1 ./...`（带库与不带库各一次）全绿；迁移后置断言通过；手工 curl——先用 SQL 把测试账号设为 owner（spec §13 的 `UPDATE users SET group_id = '00000000-0000-4000-8000-000000000001' ...`），再验：普通用户 403、owner 200、`/auth/me` 带 group 与 permissions、预设组改动被拒、非空组删除被拒、非空组改成员只有 owner 能改。

- [ ] **Step 6: 收尾提交（仅在终验发现问题时）**

修复后按层分别提交（只 `git add` 实际改动文件，禁止 `git add -A`）。无缺陷则跳过本步。
