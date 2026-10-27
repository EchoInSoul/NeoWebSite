# NeoBlog 媒体存储 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给后端加一套本地上传/存储能力：owner/admin 能传任意文件、拿到 `/storage/...` 相对 URL，公开读走静态映射且不查数据库。

**Architecture:** 新增 `internal/modules/media`（模块装配 + service + repository + local provider + controller），数据落 `media_assets` 一张表，文件落项目根 `data/storage/<yyyy>/<mm>/<uuid>.<ext>`，读路径用 `http.FileServer` 直接映射磁盘。

**Tech Stack:** Go 1.x + gin + ent（Atlas 迁移）+ Postgres；测试用 `httptest.NewServer` + 真实测试库。

**Spec:** `docs/superpowers/specs/2026-10-27-neoblog-media-storage-design.md`（v1.2，实施前必须通读）

## Global Constraints

- **全仓零人工注释**：Go 与 TS 都算，只有工具链生成的注释例外。
- **不执行 `git commit`、不推送**：每个任务结束时改动留在工作区，由用户自己提交。
- **不新增环境变量**：`internal/config` 与 `backend/.env.example` 不改。
- **不新增权限点**：`internal/common/rbac` 不改。
- **不做文件类型校验、不设大小上限**（本轮只留 `UploadPolicy` 插槽）。
- **错误响应一律走 `apierror` 信封**，不带 message；`repository.go` 不许 import `apierror`（`scripts/check.sh` 会拦）。
- **上传响应不带 `created_at`**。
- 固定值：媒体根 `../data/storage`（相对 `backend/` 工作目录）、公开前缀 `/storage/`、表名 `media_assets`。

## Review Focus

这些是 spec 没写测试、但用户一定会碰到的输入类；每条都要落到下面某个任务的测试里。

1. 客户端上传到一半断开（body 读错误）→ 必须报错，且磁盘上不能留下不完整文件。
2. 文件名是 `.`、`..`、空串或只有点 → 扩展名归一为 `bin`，不能报错也不能写出奇怪路径。
3. `Range: bytes=0-`（开放尾）与 `Range: bytes=999999999-`（超出文件长度）→ 分别 206 与 416，不能被我们的 key 校验或 404 逻辑吞掉。
4. 同一毫秒内两次上传 → 两个 key 不同、两份文件都在，不能互相覆盖。
5. 记录在、磁盘文件被外部删掉 → 读 404、删 200（spec §12）。

---

### Task 1: 媒体表、迁移与 Provider 接口

**Files:**
- Create: `backend/internal/database/ent/schema/media_asset.go`
- Modify: `backend/internal/database/ent/schema/user.go`
- Generated: `backend/internal/database/ent/**`（由脚本产出；**全部留在工作区，不要漏掉任何生成物**）
- Create: `backend/migrations/<timestamp>_media_assets.sql`、`backend/migrations/atlas.sum`（脚本产出）
- Create: `backend/internal/modules/media/provider.go`
- Test: `backend/internal/modules/media/schema_test.go`

**Interfaces:**
- Produces: `ent.MediaAsset`（字段 `id/provider/key/size_bytes/original_name/uploaded_by/created_at`）；`media.Provider` 接口、`media.UploadPolicy` 接口、`allowAll`。

- [ ] **Step 1: 建 ent schema**

`backend/internal/database/ent/schema/media_asset.go`：

```go
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type MediaAsset struct {
	ent.Schema
}

func (MediaAsset) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }),
		field.String("provider").NotEmpty(),
		field.String("key").NotEmpty().Unique(),
		field.Int64("size_bytes").NonNegative(),
		field.String("original_name"),
		field.UUID("uploaded_by", uuid.UUID{}).Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (MediaAsset) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("uploader", User.Type).Ref("mediaAssets").Unique().Field("uploaded_by"),
	}
}

func (MediaAsset) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("uploaded_by"),
	}
}
```

- [ ] **Step 2: 在 User 上加反向边**

`backend/internal/database/ent/schema/user.go` 的 `Edges()` 里，`passwordResets` 那一行后面加一行：

```go
		edge.To("mediaAssets", MediaAsset.Type).Annotations(entsql.OnDelete(entsql.SetNull)),
```

先确认该文件的 import 块里已经有 `"entgo.io/ent/dialect/entsql"`（现有 `permissions` / `recoveryCodes` / `passwordResets` 三条边都在用它，正常情况下已存在；缺失就补上，否则编译失败）。

- [ ] **Step 3: 生成 ent 代码**

Run: `cd backend && ./scripts/db-generate.sh`
Expected: 退出码 0；`internal/database/ent/mediaasset.go`、`mediaasset_create.go`、`mediaasset_query.go` 等文件出现。

- [ ] **Step 4: 生成迁移**

把 `backend/scripts/db-migrate-diff.sh` 里的 `user_bio` 改成 `media_assets`，然后：

Run: `cd backend && ./scripts/db-migrate-diff.sh`
Expected: `migrations/<timestamp>_media_assets.sql` 出现、`atlas.sum` 更新。跑完**立刻把脚本里的名字改回 `user_bio`**。

- [ ] **Step 5: 人工审阅迁移 SQL**

打开新迁移文件，确认包含：`CREATE TABLE "media_assets"`、`key` 上的唯一索引、`uploaded_by` 外键带 `ON DELETE SET NULL`。缺任何一条就回到 Step 1 检查 schema。

- [ ] **Step 6: 应用到开发库与测试库**

Run: `cd backend && source .env && ./scripts/db-migrate-apply.sh`
Expected: 两个库都报 applied 成功，再跑一次报 no changes。

- [ ] **Step 7: 写 provider 接口（让包可编译）**

`backend/internal/modules/media/provider.go`：

```go
package media

import (
	"context"
	"io"
)

type Provider interface {
	Name() string
	Save(ctx context.Context, key string, r io.Reader) (int64, error)
	Delete(ctx context.Context, key string) error
	URL(key string) string
}

type UploadPolicy interface {
	Check(size int64) error
}

type allowAll struct{}

func (allowAll) Check(int64) error { return nil }
```

- [ ] **Step 8: 写表结构测试**

先建 `backend/internal/modules/media/media_testhelpers_test.go`（**Task 5 会复用它**），里面三个函数：

```go
func testDB(t *testing.T) *database.Database
func uniqueTag(t *testing.T) string
func createTestUser(t *testing.T, db *database.Database, groupKey string) uuid.UUID
```

- `testDB`：照抄 `internal/modules/access/migration_test.go:15-35`（读 `DATABASE_URL_TEST`，未设置则 `t.Skip`，`database.Open` + `t.Cleanup` 关闭）。
- `uniqueTag`：`fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())`。
- `createTestUser`：直接操作 ent client —— `db.Client.User.Create()`，`email` 与 `nickname` 用 `uniqueTag` 拼唯一值，`password_hash` 填任意非空串，`email_verification_status` 填 `"not_required"`，`group_id` 按 `groupKey` 选 `rbac.GroupIDOwner` / `rbac.GroupIDAdmin` / `rbac.GroupIDUser`，返回 `row.ID`。不依赖 `auth` 模块的任何测试入口。

然后 `schema_test.go` 里断言四件事：

```go
func TestMediaAssetTable(t *testing.T)
func TestMediaAssetKeyIsUnique(t *testing.T)
func TestMediaAssetUploadedBySetNullOnUserDelete(t *testing.T)
func TestMediaAssetProviderIsPlainText(t *testing.T)
```

- `TestMediaAssetTable`：用 `db.SQL().QueryRow("SELECT count(*) FROM information_schema.columns WHERE table_name = 'media_assets'")` 断言列数是 7。
- `TestMediaAssetKeyIsUnique`：插两条相同 `key` 的记录，第二次必须返回错误。
- `TestMediaAssetUploadedBySetNullOnUserDelete`：建一个用户 + 一条 `uploaded_by` 指向它的记录，删用户，再查该记录，`uploaded_by` 必须为空。
- `TestMediaAssetProviderIsPlainText`：插入 `provider = "some-future-provider"` 必须成功（证明没有 DB 枚举约束）。

- [ ] **Step 9: 跑测试**

Run: `cd backend && source .env && ./scripts/test.sh ./internal/modules/media/`
Expected: PASS（4 个用例）。

### Task 2: 本地磁盘 Provider

**Files:**
- Create: `backend/internal/modules/media/local.go`
- Test: `backend/internal/modules/media/local_test.go`

**Interfaces:**
- Consumes: `Provider`（Task 1）。
- Produces: `newLocal(root string) *localProvider`，实现 `Name() string`、`Save(ctx, key, r) (int64, error)`、`Delete(ctx, key) error`、`URL(key) string`。

- [ ] **Step 1: 写失败测试**

`local_test.go`：

```go
func TestLocalSaveWritesFileAndReturnsSize(t *testing.T)
func TestLocalSaveCreatesNestedDirectories(t *testing.T)
func TestLocalSaveRemovesTempFileOnReadError(t *testing.T)
func TestLocalSaveRemovesTempFileOnRenameConflict(t *testing.T)
func TestLocalDeleteMissingFileReturnsErrNotExist(t *testing.T)
func TestLocalURL(t *testing.T)
```

- `TestLocalSaveWritesFileAndReturnsSize`：`root := t.TempDir()`，写入 `"hello"`，断言返回 5、`root/2026/10/27/a.txt` 内容为 `hello`。
- `TestLocalSaveCreatesNestedDirectories`：key 用 `2026/10/27/x.jpg`，断言目录被自动创建。
- `TestLocalSaveRemovesTempFileOnReadError`：传一个读时返回 `errors.New("boom")` 的 reader，断言 `Save` 返回错误、目标目录里**没有任何文件**（含 `.*.tmp`）。
- `TestLocalSaveRemovesTempFileOnRenameConflict`：预先在目标路径建一个目录占位，`Save` 失败后断言没有残留 `.tmp`。这条依赖 `os.Rename` 在目标已存在时的失败语义：Linux 上稳定，Windows 上是权限/占用错误；断言只写「返回错误 + 无 `.tmp` 残留」，两种平台都成立。
- `TestLocalDeleteMissingFileReturnsErrNotExist`：`errors.Is(err, fs.ErrNotExist)` 为真。
- `TestLocalURL`：`URL("2026/10/27/a.jpg")` 等于 `/storage/2026/10/27/a.jpg`。

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/modules/media/ -run 'TestLocal' -v`
Expected: 编译失败（`newLocal` 未定义）。

- [ ] **Step 3: 实现 `local.go`**

```go
package media

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

type localProvider struct {
	root string
}

func newLocal(root string) *localProvider {
	return &localProvider{root: root}
}

func (p *localProvider) Name() string {
	return "local"
}

func (p *localProvider) URL(key string) string {
	return "/storage/" + key
}
```

`Save`：`os.MkdirAll(filepath.Dir(target), 0o755)` → `os.CreateTemp(dir, ".upload-*")` → `io.Copy` → `Close` → `os.Rename`；任一步失败都要 `os.Remove` 临时文件并把原错误返回。

`Delete`：`os.Remove(filepath.Join(p.root, filepath.FromSlash(key)))`。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test ./internal/modules/media/ -run 'TestLocal' -v`
Expected: PASS（6 个用例）。

### Task 3: 上传服务（扩展名、原始名、补偿）

**Files:**
- Create: `backend/internal/modules/media/service.go`
- Create: `backend/internal/modules/media/repository.go`
- Test: `backend/internal/modules/media/service_test.go`

**Interfaces:**
- Consumes: `Provider`、`UploadPolicy`、`newLocal`。
- Produces:
  - `type mediaAsset struct { ID uuid.UUID; Provider string; Key string; SizeBytes int64; OriginalName string; UploadedBy *uuid.UUID; CreatedAt time.Time }`
  - `func newService(db *database.Database, root string, policy UploadPolicy) (*Service, error)`（内部 `filepath.Abs` 归一，并把归一后的值存进 `Service`）
  - `func (s *Service) Root() string`（返回归一后的绝对路径，供 controller 构造 `http.Dir`）
  - `func (s *Service) Upload(ctx context.Context, uploaderID uuid.UUID, name string, r io.Reader) (mediaAsset, error)`
  - `func (s *Service) URL(asset mediaAsset) (string, error)`（按 `asset.Provider` 查注册表，找不到返回 `errUnknownProvider`）
  - `var errNotFound`、`var errInvalidOriginalName`、`var errUnknownProvider`
  - `type store interface { Insert(context.Context, mediaAsset) error; Get(context.Context, uuid.UUID) (mediaAsset, error); Delete(context.Context, uuid.UUID) (bool, error) }`

- [ ] **Step 1: 写扩展名与原始名的表驱动测试**

`service_test.go`：

```go
func TestNormalizeExtension(t *testing.T)
func TestValidateOriginalName(t *testing.T)
```

用例（`want` 为扩展名）：

| name | want |
| --- | --- |
| `PHOTO.JPG` | `jpg` |
| `a.tar.gz` | `gz` |
| `photo.` | `bin` |
| `中文.png` | `png` |
| `` | `bin` |
| `.` | `bin` |
| `..` | `bin` |
| `abcdefghijklmnopqrstuvwxyz` | `bin` |
| `a.abcdefghijklmnopq` | `bin` |
| `.gitignore` | `gitignore` |

`TestValidateOriginalName`：`""` 合法；`"a\x00b"` 返回 `errInvalidOriginalName`；`string([]byte{0xff, 0xfe})` 返回 `errInvalidOriginalName`；`"素材包.zip"` 合法。

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/modules/media/ -run 'TestNormalizeExtension|TestValidateOriginalName' -v`
Expected: 编译失败（函数未定义）。

- [ ] **Step 3: 实现扩展名与原始名校验**

```go
var extensionPattern = regexp.MustCompile(`^[a-z0-9]{1,16}$`)

func normalizeExtension(name string) string

func validateOriginalName(name string) error
```

`normalizeExtension`：`strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")`，不匹配则 `bin`。
`validateOriginalName`：含 `\x00` 或 `!utf8.ValidString` 返回 `errInvalidOriginalName`。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test ./internal/modules/media/ -run 'TestNormalizeExtension|TestValidateOriginalName' -v`
Expected: PASS。

- [ ] **Step 5: 写 repository**

`repository.go`：`type repository struct { client *ent.Client }`、`newRepository(db *database.Database) *repository`。
`Insert` 用 `client.MediaAsset.Create().SetProvider(asset.Provider).SetKey(asset.Key).SetSizeBytes(asset.SizeBytes).SetOriginalName(asset.OriginalName).SetNillableUploadedBy(asset.UploadedBy).SetCreatedAt(asset.CreatedAt)`（显式设置 `created_at`，不用默认值）；`Get` 查不到返回 `errNotFound`；`Delete` 返回是否删到了行。

- [ ] **Step 6: 写上传服务测试（注入 fake store 与 fake provider）**

```go
func TestUploadWritesFileAndRecord(t *testing.T)
func TestUploadRejectsNULInName(t *testing.T)
func TestUploadLeavesNoFileWhenReaderFails(t *testing.T)
func TestUploadDeletesFileWhenInsertFails(t *testing.T)
func TestUploadDeletesFileWhenPolicyRejects(t *testing.T)
func TestUploadUsesSameTimestampForDirectoryAndRecord(t *testing.T)
```

- fake store 的 `Insert` 按需返回错误；fake provider 记录 `Save` / `Delete` 的调用与 key。
- **注入方式**：`newService` 内部硬编码 `newRepository(db)`，测试拿不到注入点，所以 service 测试**直接构造 `Service` 字面量**（同包、字段可访问），不要走 `newService`：

  ```go
  func newServiceForTest(root string, p Provider, st store) *Service {
  	return &Service{
  		store:         st,
  		providers:     map[string]Provider{"local": p},
  		writeProvider: p,
  		policy:        allowAll{},
  		root:          root,
  	}
  }
  ```

  `providers` 与 `writeProvider` 必须是**同一个实例**；需要真实文件行为（例如下面那条 reader 报错用例）时传 `newLocal(t.TempDir())`，只关心调用序列时传 fake provider。
- `TestUploadDeletesFileWhenInsertFails`：`Insert` 返回错误 → `Delete` 被调用一次，key 与 `Save` 的 key 相同。
- `TestUploadLeavesNoFileWhenReaderFails`：用**真实的 `newLocal(t.TempDir())`** 配一个读时返回 `errors.New("boom")` 的 reader，断言 `Upload` 返回错误、`store.Insert` 一次没被调用、根目录下没有任何文件（对应 Review Focus #1 的 service 层覆盖）。
- `TestUploadUsesSameTimestampForDirectoryAndRecord`：key 的 `yyyy/mm/dd` 与该记录的 `CreatedAt.UTC()` 日期一致。

- [ ] **Step 7: 实现 `service.go` 的上传路径**

```go
type Service struct {
	store         store
	providers     map[string]Provider
	writeProvider Provider
	policy        UploadPolicy
	root          string
}

func newService(db *database.Database, root string, policy UploadPolicy) (*Service, error)
```

`newService` 负责：`filepath.Abs(root)`、`os.MkdirAll(abs, 0o755)`、`os.CreateTemp(abs, ".probe-*")` 写探针（`Close` 与 `Remove` 的错误都要返回），然后构造 `map[string]Provider{"local": newLocal(abs)}`。探针的 `Remove` 失败会留下一个 `.probe-*` 文件在根目录，随机名、不影响功能，但错误要照常返回、启动照常失败。

`Upload` 按 spec §8 的顺序：校验原始名 → 归一扩展名 → `now := time.Now().UTC()` → `id := uuid.Must(uuid.NewV7())` → `key := now.Format("2006/01/02") + "/" + id.String() + "." + ext` → `Save` → `policy.Check(size)`（失败先 `Delete`）→ `Insert`（失败先 `Delete`）→ 返回 `mediaAsset`。

`asset.UploadedBy` 是 `*uuid.UUID`，而 `Upload` 收到的是值类型的 `uploaderID uuid.UUID`；构造记录时必须写 `asset.UploadedBy = &uploaderID`，再交给 repository 的 `SetNillableUploadedBy`。

- [ ] **Step 8: 跑测试确认通过**

Run: `cd backend && go test ./internal/modules/media/ -run 'TestUpload|TestNormalize|TestValidate' -v`
Expected: PASS。

### Task 4: 删除服务（先删文件、再删记录）

**Files:**
- Modify: `backend/internal/modules/media/service.go`
- Modify: `backend/internal/modules/media/repository.go`
- Test: `backend/internal/modules/media/service_test.go`

**Interfaces:**
- Produces: `func (s *Service) Delete(ctx context.Context, id uuid.UUID) error`

- [ ] **Step 1: 写失败测试**

```go
func TestDeleteRemovesFileBeforeRecord(t *testing.T)
func TestDeleteKeepsRecordWhenFileRemovalFails(t *testing.T)
func TestDeleteTreatsMissingFileAsAlreadyDeleted(t *testing.T)
func TestDeleteReturnsInternalErrorWhenProviderUnknown(t *testing.T)
func TestDeleteReturnsNotFoundWhenRecordMissing(t *testing.T)
func TestDeleteReturnsNotFoundWhenRowAlreadyGone(t *testing.T)
```

- `TestDeleteRemovesFileBeforeRecord`：断言**顺序**时不能只看"两个都调用了"，要用共享的 `[]string` 记录调用序列（fake provider 的 `Delete` 追加 `"file:"+key`，fake store 的 `Delete` 追加 `"record:"+id.String()`），然后断言 `file:` 那条的索引小于 `record:` 那条。
- `TestDeleteKeepsRecordWhenFileRemovalFails`：fake provider 的 `Delete` 返回 `errors.New("locked")` → 服务返回错误，fake store 的 `Delete` **一次都没被调用**。
- `TestDeleteTreatsMissingFileAsAlreadyDeleted`：fake provider 返回 `fs.ErrNotExist` → 服务成功，store 的 `Delete` 被调用。
- `TestDeleteReturnsInternalErrorWhenProviderUnknown`：记录里的 `provider` 是 `"s3"` 而注册表只有 `local` → 服务返回 `errUnknownProvider`，store 未被调用；**controller 层必须把它映射成 500 `internal_error`**（spec §12），不能落到 404，也不能只靠 `apierror.Write` 的默认分支。
- `TestDeleteReturnsNotFoundWhenRowAlreadyGone`：两个并发删除的第二种结局——`store.Delete` 返回 `false` → 服务返回 `errNotFound`（对应 spec §12 第一行）。

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/modules/media/ -run 'TestDelete' -v`
Expected: 编译失败（`Delete` 未定义）。

- [ ] **Step 3: 实现 `Service.Delete`**

顺序固定：`store.Get` → 查 provider（不存在返回 `errUnknownProvider`）→ `provider.Delete(key)`（`errors.Is(err, fs.ErrNotExist)` 视为成功）→ `store.Delete(id)`；返回 `false` 时返回 `errNotFound`。

**所有 sentinel 错误都不许丢链**：直接 `return errUnknownProvider`，或 `fmt.Errorf("media: %w", errUnknownProvider)`。绝不能写成 `fmt.Errorf("unknown provider: %s", name)`——`writeServiceError` 用的是 `errors.Is`，链断了就会落进 `default`，状态码变成不确定。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test ./internal/modules/media/ -run 'TestDelete' -v`
Expected: PASS。

### Task 5: 上传/删除的 HTTP 层与模块装配

**Files:**
- Create: `backend/internal/modules/media/dto.go`
- Create: `backend/internal/modules/media/controller.go`
- Create: `backend/internal/modules/media/module.go`
- Test: `backend/internal/modules/media/media_test.go`

**Interfaces:**
- Consumes: `Service.Upload`、`Service.Delete`。
- Produces: `func New(db *database.Database, root string) (*Module, error)`、`func (m *Module) RegisterProtected(r *gin.RouterGroup)`、`*Module` 上的 `Service` 字段。

- [ ] **Step 1: 写集成测试骨架与上传/删除用例**

`media_test.go` 复用 `access/access_test.go` 的模式：`scs.New()` + `/seed` 设会话 + `guard.RequireAuth` + `guard.LoadIdentity` + `httptest.NewServer(sm.LoadAndSave(engine))`。

**`LoadIdentity` 需要一个 `IdentityResolver`**：生产里是 `accessModule.Service`（`app.go:98`），测试里用 `access.New(db).Service`（`media` 的测试文件 import `internal/modules/access`，只用于测试，不构成包依赖环）。没有它 `LoadIdentity` 编译不过，`RequirePresetGroup` 也拿不到身份。

辅助函数：

```go
func newMediaTestApp(t *testing.T) (*httptest.Server, *database.Database, string)
func seedClient(t *testing.T, ts *httptest.Server, userID uuid.UUID) *http.Client
func uploadFile(t *testing.T, client *http.Client, url, filename string, content []byte) *http.Response
```

`testDB`、`uniqueTag`、`createTestUser` 已在 Task 1 的 `media_testhelpers_test.go` 里，这里直接用，不要重复定义。

`newMediaTestApp` 里必须设 `gin.SetMode(gin.TestMode)` 与 `engine.HandleMethodNotAllowed = true`（不设的话 `POST /storage/x` 会返回 404 而不是 405，测不出 405 用例）。`gin.SetMode` 是全局副作用，但仓库现有测试（`internal/modules/access/access_test.go`）也这么用，保持一致即可，不要中途切回别的模式。`media.New` 的错误要 `t.Fatalf`。

注意：测试必须用 `httptest.NewServer`（真实 ResponseWriter），**不能用 `httptest.NewRecorder`**——`ResponseRecorder` 不支持 `SetWriteDeadline`/`SetReadDeadline`，上传与下载 handler 会走 500 分支。

用例：

```go
func TestUploadStoresFileAndRecord(t *testing.T)
func TestUploadRequiresOwnerOrAdmin(t *testing.T)
func TestUploadRequiresAuthentication(t *testing.T)
func TestUploadRejectsMissingPart(t *testing.T)
func TestUploadRejectsNonMultipart(t *testing.T)
func TestUploadAcceptsZip(t *testing.T)
func TestUploadAcceptsEmptyFile(t *testing.T)
func TestConcurrentUploadsUseDistinctKeys(t *testing.T)
func TestDeleteRemovesFileAndRecord(t *testing.T)
func TestDeleteWithMalformedIDReturns404(t *testing.T)
```

- `TestUploadStoresFileAndRecord`：201 返回 `url` 形如 `/storage/YYYY/MM/DD/<uuid>.jpg`；磁盘文件字节一致；库里 `size_bytes` 正确，`created_at` 的 **UTC** 日期与 URL 里的年月日一致（两边都先 `.UTC()` 再比）。
- `TestUploadRejectsNonMultipart`：`Content-Type: application/json` → 400 `invalid_request`。
- `TestUploadAcceptsZip`：上传 `素材包.zip`，URL 以 `.zip` 结尾。
- `TestUploadAcceptsEmptyFile`：0 字节上传 → 201，`size_bytes` 为 0（本轮不做类型校验）。
- `TestConcurrentUploadsUseDistinctKeys`：两个 goroutine 同时上传，断言两个 `url` 不同、两份文件都在（Review Focus #4）。
- 权限用例：普通用户 `groupKey="user"` → 403；无 cookie → 401。
- **NUL 文件名不做端到端测试**：HTTP 头不允许 NUL 字节，`multipart.Writer` 写不进去，`Part.FileName()` 也拿不到这种值。这条只由 Task 3 的 `TestValidateOriginalName` 在 service 层覆盖。

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && source .env && ./scripts/test.sh ./internal/modules/media/ -run 'TestUpload|TestDelete'`
Expected: 编译失败（`Module` 未定义）。

- [ ] **Step 3: 写 dto 与 controller（受保护两条路由）**

`dto.go`：

```go
type mediaResponse struct {
	ID           string `json:"id"`
	URL          string `json:"url"`
	SizeBytes    int64  `json:"size_bytes"`
	OriginalName string `json:"original_name"`
}

type deleteResponse struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}
```

`controller.go` 的 `registerProtected`：

```go
r.POST("/media", guard.RequirePresetGroup(rbac.GroupKeyOwner, rbac.GroupKeyAdmin), c.upload)
r.DELETE("/media/:id", guard.RequirePresetGroup(rbac.GroupKeyOwner, rbac.GroupKeyAdmin), c.remove)
```

controller 结构体必须**自己持有媒体根目录**，Task 6 的静态 handler 要用它构造 `http.Dir`，而 `Service` 不对外暴露根路径：

```go
type controller struct {
	service *Service
	root    string
}

func newController(service *Service, root string) *controller
```

`newController` 收到的 `root` 必须与 `newService` 用的**同一个归一后路径**。固定做法：`Service` 里存归一后的绝对路径并暴露 `Root()`，`Module.New` 写 `newController(service, service.Root())`。不要两边各自 `filepath.Abs`——`data/storage` 与 `<abs>/data/storage` 会指向不同目录。

`upload`：先 `http.NewResponseController(ctx.Writer).SetReadDeadline(time.Time{})`（失败 → 500），再 `ctx.Request.MultipartReader()`，循环 `NextPart()` 找 `FormName()=="file"`，用 `guard.IdentityFrom(ctx)` 的 `UserID` 解析 UUID，调 `service.Upload`，再调 `service.URL(asset)` 拿地址，201 返回 `mediaResponse`。`service.URL` 出错也按 500 处理。

`remove`：`uuid.Parse(ctx.Param("id"))` 失败 → 404 `not_found`；成功 → 200 `deleteResponse`。

错误映射统一走一个私有函数，**四条都要显式列出**：

```go
func writeServiceError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, errNotFound):
		apierror.Write(ctx, apierror.ErrNotFound)
	case errors.Is(err, errInvalidOriginalName):
		apierror.Write(ctx, apierror.ErrInvalidRequest)
	case errors.Is(err, errUnknownProvider):
		apierror.Write(ctx, apierror.ErrInternal)
	default:
		apierror.Write(ctx, err)
	}
}
```

`errUnknownProvider` 必须是 500（spec §12），漏掉它会落进 `default`，状态码取决于 `apierror` 对该错误的判定，不可控。
`apierror.Write` 的 503 映射（连接类数据库错误）已由 `internal/common/apierror/apierror_test.go` 覆盖，本轮不再在 HTTP 层重复测。

- [ ] **Step 4: 写 module.go**

```go
type Module struct {
	Service *Service

	controller *controller
}

func New(db *database.Database, root string) (*Module, error) {
	service, err := newService(db, root, allowAll{})
	if err != nil {
		return nil, err
	}
	return &Module{Service: service, controller: newController(service, service.Root())}, nil
}

func (m *Module) RegisterProtected(r *gin.RouterGroup) {
	m.controller.registerProtected(r)
}
```

- [ ] **Step 5: 跑测试确认通过**

Run: `cd backend && source .env && ./scripts/test.sh ./internal/modules/media/`
Expected: PASS。

### Task 6: 静态读、下载策略与 deadline

**Files:**
- Modify: `backend/internal/modules/media/controller.go`
- Modify: `backend/internal/modules/media/module.go`
- Test: `backend/internal/modules/media/media_test.go`

**Interfaces:**
- Produces: `func (m *Module) RegisterStorage(engine *gin.Engine)`

- [ ] **Step 1: 写失败测试**

```go
func TestStorageServesUploadedFile(t *testing.T)
func TestStorageHeadHasNoBody(t *testing.T)
func TestStorageServesOpenEndedRange(t *testing.T)
func TestStorageRejectsRangeBeyondEnd(t *testing.T)
func TestStorageSends304OnIfModifiedSince(t *testing.T)
func TestStorageForcesDownloadForHTML(t *testing.T)
func TestStorageReturns404ForMalformedKey(t *testing.T)
func TestStorageReturns404ForMissingFile(t *testing.T)
func TestStorageRedirectsBarePrefix(t *testing.T)
func TestStorageRejectsPost(t *testing.T)
```

- 先上传一个 `.jpg` 与一个 `.html`（内容 `<script>alert(1)</script>`）作为夹具。
- `TestStorageServesUploadedFile`：200，`Content-Type: image/jpeg`，字节一致，且 `Cache-Control` 是 `public, max-age=31536000, immutable`。
- `TestStorageServesOpenEndedRange`：`Range: bytes=0-` → 206，`Content-Length` 等于文件长度、`Content-Range` 为 `bytes 0-4/5`，`Cache-Control` 仍是 `immutable`（Review Focus #3）。
- `TestStorageRejectsRangeBeyondEnd`：`Range: bytes=999999999-` → 416，`Cache-Control` 为 `no-store`。
- `TestStorageSends304OnIfModifiedSince`：拿第一次响应的 `Last-Modified` 再请求 → 304，`Cache-Control` 仍是 `immutable`。
- `TestStorageForcesDownloadForHTML`：`Content-Type: application/octet-stream`、`Content-Disposition` 以 `attachment; filename="` 开头、`X-Content-Type-Options: nosniff`。
- `TestStorageReturns404ForMalformedKey`：`/storage/2026/` → 404、body 为空，**`Cache-Control` 必须是 `no-store`**（不是 `immutable`），`nosniff` 在。
- `TestStorageReturns404ForMissingFile`：形状合法但不存在的 key → 404，body 含 `404 page not found`，**`Cache-Control` 必须是 `no-store`**，`nosniff` 在。
- `TestStorageRedirectsBarePrefix`：`GET /storage`，`CheckRedirect` 关掉 → 301，`Location` 以 `/storage/` 结尾。这个 301 是 **gin 在路由层生成的**，不经过我们的 handler，所以**不要断言 `nosniff` / `Cache-Control`**（spec D14 的「所有响应」只覆盖 handler 产生的响应）。
- `TestStorageRejectsPost`：POST → 405，`Allow` 含 `GET` 与 `HEAD`。同样是路由层响应，不断言我们的两个头。

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && source .env && ./scripts/test.sh ./internal/modules/media/ -run 'TestStorage'`
Expected: 全部 404（路由未注册）。

- [ ] **Step 3: 实现静态 handler**

按 spec §7.4 的代码：响应头先设 `nosniff` 与 `Cache-Control: public, max-age=31536000, immutable`；再清写 deadline（失败 → 改 `no-store` 后 500）；再校验 `keyPattern`（不匹配 → 改 `no-store` 后 404）；再按 `inlineTypes` 决定 `Content-Type` / `Content-Disposition`；最后交给 `notFoundNoStore{c.Writer}` 包着的 `http.StripPrefix("/storage/", http.FileServer(http.Dir(root)))`。注册 `engine.GET` 与 `engine.HEAD`。

**`notFoundNoStore` 不能省**：文件不存在时是 `FileServer` 自己写响应，必须靠这个包装器在 `WriteHeader(code)` 且 `code >= 400` 时把 `Cache-Control` 改成 `no-store`（覆盖 404、416 以及 `FileServer` 的 403 / 5xx），否则浏览器会把错误缓存一年。包装器只覆盖 `WriteHeader`，`Header`/`Write` 靠内嵌接口透传；它包在 `FileServer` 外面、但清 deadline 用的是没包装的 `c.Writer`。

`RegisterStorage(engine *gin.Engine)` 在 module.go 里转调 controller；**同时把测试辅助 `newMediaTestApp` 改成也调用 `module.RegisterStorage(engine)`**，否则 Task 6 的用例全部 404。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && source .env && ./scripts/test.sh ./internal/modules/media/`
Expected: PASS（全部用例）。

- [ ] **Step 5: 补两条 spec §12 的异常口径测试**

```go
func TestStorageReturns404WhenFileMissingButRecordExists(t *testing.T)
func TestDeleteSucceedsWhenFileAlreadyGone(t *testing.T)
```

第一条：直接往库里插一条记录（不写文件）→ GET 404，`Cache-Control` 为 `no-store`，`nosniff` 在。记录的 `key` 必须**符合 `keyPattern`**，否则会先被形状校验挡掉、测不出「文件不存在」这条路径：

```go
now := time.Now().UTC()
key := now.Format("2006/01/02") + "/" + uuid.Must(uuid.NewV7()).String() + ".jpg"
```

用 `db.Client.MediaAsset.Create()` 插这条记录（`provider="local"`、`size_bytes=1`、`original_name="x.jpg"`、`created_at=now`）。第二条：上传后手工删掉磁盘文件 → DELETE 200、记录消失（Review Focus #5）。

- [ ] **Step 6: 变异自检（404 缓存策略）**

把 `fileServer.ServeHTTP(notFoundNoStore{c.Writer}, c.Request)` 临时改成 `fileServer.ServeHTTP(c.Writer, c.Request)`（去掉包装器），跑：

Run: `cd backend && source .env && ./scripts/test.sh ./internal/modules/media/ -run 'TestStorageReturns404'`
Expected: 两个 404 用例变红（`Cache-Control` 拿到 `immutable` 而不是 `no-store`）。确认后把包装器改回来，再跑一次确认绿。

### Task 7: app 装配与超时穿透

**Files:**
- Modify: `backend/internal/app/app.go`
- Modify: `backend/internal/app/app_test.go`
- Test: `backend/internal/app/app_test.go`

**Interfaces:**
- Consumes: `media.New(db, root)`、`RegisterStorage`、`RegisterProtected`。
- Produces: `const mediaRootPath`、`func newWithRoot(cfg config.Config, db *database.Database, mediaRoot string) (*App, error)`、`(*writeTracker).Unwrap() http.ResponseWriter`。

- [ ] **Step 1: 给 `writeTracker` 加 Unwrap**

`app.go` 里加：

```go
func (w *writeTracker) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
```

- [ ] **Step 2: 加路径常量与 `newWithRoot`**

把现有 `func New(cfg, db) (*App, error)` 的函数体整体搬进 `func newWithRoot(cfg config.Config, db *database.Database, mediaRoot string) (*App, error)`，`New` 变成：

```go
func New(cfg config.Config, db *database.Database) (*App, error) {
	return newWithRoot(cfg, db, mediaRootPath)
}
```

常量与 `apiPrefix` 放在同一组：

```go
const mediaRootPath = "../data/storage"
```

- [ ] **Step 3: 在 `newWithRoot` 里装配 media**

`engine` 建好之后、`authModule.Register(api)` 之前插入：

```go
	mediaModule, err := media.New(db, mediaRoot)
	if err != nil {
		return nil, err
	}
```

`protected` 建好之后加 `mediaModule.RegisterProtected(protected)`；`engine.NoRoute` 之前加 `mediaModule.RegisterStorage(engine)`。

- [ ] **Step 4: 改 app_test 的三处构造**

`app_test.go` 里**三处** `New(cfg, db)` 调用（当前在 helper `newTestServer`、降级服务器 helper、`TestShutdownClosesMailer` 里）改成 `newWithRoot(cfg, db, t.TempDir())`。按调用点定位，不要按行号。

- [ ] **Step 5: 跑 app 测试**

Run: `cd backend && source .env && ./scripts/test.sh ./internal/app/`
Expected: PASS；跑完后仓库里**没有** `backend/internal/data/` 目录（用 `ls backend/internal/data` 确认不存在）。如果目录存在，先确认它是不是改动前的旧运行留下的残留（删掉再跑一遍），不要把旧残留言当成这次改动的问题。

- [ ] **Step 6: 跑全量测试**

Run: `cd backend && source .env && ./scripts/test.sh`
Expected: 所有包 PASS。

### Task 8: 文档、忽略规则与授权记录

**Files:**
- Modify: `.gitignore`（仓库根）
- Modify: `backend/README.md`
- Modify: `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md`
- Modify: `docs/superpowers/specs/2026-10-11-neoblog-content-design.md`
- Modify: `docs/DEVLOG.md`

- [ ] **Step 1: 加忽略规则**

仓库根 `.gitignore` 末尾加：

```
/data/
```

- [ ] **Step 2: 改 README**

- 模块表加 `internal/modules/media/*`（装配 + service + repository + local provider + controller + 三条路由）。
- 接口表加三行：`POST /api/v1/media`、`DELETE /api/v1/media/:id`、`GET|HEAD /storage/*key`。
- 新增「媒体存储」章节：目录 `data/storage`、URL 前缀 `/storage/`、路径写死在 `app.go` 的 `mediaRootPath`、非媒体类型强制下载、`/storage` 不在 `/api/v1` 下且不返回 JSON、404 是纯文本或空 body。
- 修正三处失效表述（**按文字定位，不要按行号**，README 会被其他改动挪动）：含「唯一例外 `/healthy`」那一段补上 `/storage`；含「`HEAD` 与 `OPTIONS` 没有任何端点注册」那一段改成「`OPTIONS` 没有端点，`HEAD /storage/*key` 已注册」；迁移清单里补 `media_assets` 这一行。

- [ ] **Step 3: 改 API 规范**

`docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md`：

- §3 端点表顺延加两行（编号接当前最大 31）。
- §4 规则 1 补「`/storage/**` 是静态资源例外」、规则 3 补「`/storage/**` 返回二进制」、规则 5 补「`/storage` 的 404 同样不受错误信封约束」。

- [ ] **Step 4: 改 content spec 的四处示例**

`docs/superpowers/specs/2026-10-11-neoblog-content-design.md` 里所有 `/uploads/...` 示例改成 `/storage/...`（**按文字定位，不要按行号**）。

- [ ] **Step 5: 写 DEVLOG**

`docs/DEVLOG.md` 开头插入本轮条目（倒序），必须包含一行授权记录：

> 用户 2026-10-27 明确授权：媒体上传本轮**不限制文件大小、不做文件类型校验**，适用范围仅 `POST /api/v1/media`；`data/storage` 所在分区的剩余空间需自行监控。

同时记：删除语义为「先删文件再删记录，删不掉返回 500」；`/storage` 的静态路由带自有响应头策略。

- [ ] **Step 6: 跑格式与静态检查**

Run: `cd backend && ./scripts/check.sh`
Expected: 退出码 0（gofmt、repository 不许 import apierror、go vet）。

- [ ] **Step 7: 全量测试收尾**

Run: `cd backend && source .env && ./scripts/test.sh`
Expected: 全绿。

---

## 完成标准

- `POST /api/v1/media` 能传任意文件，返回 `/storage/...` 相对 URL；磁盘在 `data/storage/<yyyy>/<mm>/<uuid>.<ext>`。
- 匿名 `GET /storage/...` 不查数据库，支持 Range、304、HEAD；非内联类型强制下载且带 `nosniff`。
- `DELETE /api/v1/media/:id` 先删文件再删记录，删不掉返回 500。
- 全量测试绿，`scripts/check.sh` 绿，工作区不含 `backend/internal/data/`。
- 改动全部留在工作区，未提交。
