# NeoBlog 内容模块（文章/页面 + 快照历史）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 后端新增内容模块：文章（posts）与自定义页面（pages）的草稿/发布管理、快照式历史版本（每次保存正文追加一条不可变快照，实体行 head/release 双指针）、公开读 + 管理 CRUD/动作/快照三端点，共 24 条接口。

**Architecture:** 照 HaoBlog 的快照 + 双指针模型（spec D16）：正文四件套只存快照表，实体行存 title/slug/cover/excerpt/search_text + 两个 unique FK 指针；列表只读实体行、公开详情读 release、管理详情读 head；恢复 = 复制旧版为新快照。模块分层照抄 access：module → controller → service → repository（ent），错误统一走 `internal/common/apierror`。

**Tech Stack:** Go + Gin + ent（`--feature sql/lock`）+ Atlas 版本化迁移 + PostgreSQL 17（容器 `neoblog-postgres`，端口 5434）。

**Spec:** `docs/superpowers/specs/2026-10-11-neoblog-content-design.md`（v2.2，下称 spec；引用按其章节/决策号）

## Global Constraints（每个任务默认携带）

- 后端 Go 代码**零人工注释**，`gofmt` 规范缩进；每个任务收尾 `go build ./... && go vet ./... && gofmt -l .` 全干净。
- API 前缀 `/api/v1`（`backend/internal/app/app.go:27` 常量）；JSON 字段 snake_case；时间戳 epoch 秒、字段名 `_at` 结尾、未发生为 `null`（Go 侧 `time.Time.Unix()` / `*int64`）；ID 为 UUIDv7 字符串。
- 列表信封恒为 `{"data":[…],"total":n,"limit":n,"offset":n}`（不用 access 现在的 `{"items":…}`，spec §6.2）；`limit` 默认 20、超 100 钳制 100、非法值 400 param 指向参数；`offset` 默认 0；默认排序 `created_at` 倒序。
- 错误一律 `apierror.Write(ctx, err)` / `apierror.Abort`，禁止本地 writeError；本计划只新增 `slug_taken` 一个 code（409 / `conflict_error` / param=`slug`）。
- 阈值（spec §7.2）：title trim 后 1–200 runes；slug 可空，提供时 1–120 字符且 `^[a-z0-9]+(-[a-z0-9]+)*$`；cover_image_url ≤2048；content 为 JSON 对象、序列化 ≤256KB、嵌套深度 ≤64；content_html ≤1MB；content_text ≤256KB；content_format 非空 ≤32；excerpt 截 200 runes；q ≤100。
- q 的 ILIKE 拼接前必须转义 `\` `%` `_`。
- 集成测试连 `DATABASE_URL_TEST`（缺省 `postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_test?sslmode=disable`），未设置时 `t.Skip`——照抄 `backend/internal/modules/access/migration_test.go:17-36` 的 `testDB`。
- 迁移纪律：`scripts/db-migrate-diff.sh` 生成 → 人工审阅 SQL → `atlas migrate hash && atlas migrate validate` → `scripts/db-migrate-apply.sh` 应用到 `neoblog` 与 `neoblog_test`；应用启动绝不自动迁移。
- 提交信息格式对齐仓库历史：`type(backend): 中文描述`（feat/test/chore/docs）。
- 不引入任何前端/Tiptap 依赖；不动 auth/access/health/mail 现有行为（app.go 装配行除外）。

## Review Focus

1. **q 含 LIKE 通配符**（`q=%` / `q=_`）：应按字面匹配，不得膨胀为全表通配 → Task 6 集成测试钉死。
2. **content 深嵌套**（构造 >64 层嵌套对象）：必须 400 `invalid_request` param=`content`，不得栈溢出 → Task 3 单测钉死。
3. **三列成组**：PATCH 只带 `content` 缺 `content_html` → 400 param=`content_html`（缺哪指哪），不是 500 → Task 3 单测 + Task 5 集成钉死。
4. **公开侧防探测**：不存在的 `:id` 与未发布的 `:id` → 完全相同的 404 错误体 → Task 6 集成测试钉死。
5. **并发保存**：两个并发内容 PATCH 同一实体 → version 线性化为 v2/v3，不得泄漏 unique 冲突 500 → Task 5 并发集成测试钉死。

---

### Task 1: 注册表增补（apierror.ErrSlugTaken + rbac 4 权限点）

**Files:**
- Modify: `backend/internal/common/apierror/apierror.go`（哨兵区 + `statusByCode` 各加一行）
- Modify: `backend/internal/common/rbac/rbac.go`（常量区 + `catalog` 各加 4 条）
- Test: `backend/internal/common/apierror/apierror_test.go`（追加用例）、新建 `backend/internal/common/rbac/rbac_test.go`

**Interfaces:**
- Produces: `apierror.ErrSlugTaken`（`&Error{Type: TypeConflict, Code: "slug_taken", Param: strPtr("slug")}` + `statusByCode["slug_taken"] = http.StatusConflict`）；常量 `rbac.PermissionPostsView = "content.posts.view"`、`rbac.PermissionPostsEdit = "content.posts.edit"`、`rbac.PermissionPagesView = "content.pages.view"`、`rbac.PermissionPagesEdit = "content.pages.edit"`，catalog 条目展示名依次「查看文章 / 管理文章 / 查看页面 / 管理页面」。

- [ ] **Step 1: 写失败测试**——`apierror_test.go` 追加：`Body(apierror.ErrSlugTaken)` 的 Type=`conflict_error`、Code=`slug_taken`、Param=`slug`，`statusOf` 为 409。新建 `rbac_test.go`：`IsValid` 对 4 个新 key 为 true；`Expand([]string{PermissionPostsEdit})` 包含 `content.posts.view`（edit→view 蕴含，`rbac.go:28-30` 现成规则）；`len(Catalog())` 增加 4。
- [ ] **Step 2: 跑测试确认失败**——`cd backend && go test ./internal/common/... -v`，预期编译失败（常量未定义）。
- [ ] **Step 3: 实现**——按 Interfaces 的精确值加哨兵、注册行、4 常量、4 条 catalog。
- [ ] **Step 4: 跑测试确认通过**——同 Step 2 命令，全绿。
- [ ] **Step 5: 提交**——`git add -A && git commit -m "feat(backend): apierror 增补 slug_taken，rbac 目录增内容模块 4 权限点"`

### Task 2: ent 四实体 schema + 生成 + Atlas 迁移

**Files:**
- Create: `backend/internal/database/ent/schema/post.go`、`post_snapshot.go`、`page.go`、`page_snapshot.go`
- Modify: `backend/scripts/db-migrate-diff.sh`（迁移名参数化为 `"$1"`，缺省时报错退出）
- Create: `backend/internal/modules/content/migration_test.go`（本任务只有这个测试文件，包路径先建目录）
- Modify: `backend/migrations/`（生成 `*_content_posts_pages.sql` + `atlas.sum`）

**Interfaces:**
- Produces: ent 包 `post` / `postsnapshot` / `page` / `pagesnapshot` 及 Client 方法（后续所有任务依赖）。字段与边严格按下方 schema——它们就是 spec §12.2/§12.3 的落地值。

`post.go`（`page.go` 把 `slug`/`cover_image_url` 同款保留，`author` 边同样指向 User）：

```go
func (Post) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }),
		field.String("title").NotEmpty(),
		field.String("slug").Optional().Nillable().Unique(),
		field.String("cover_image_url").Optional().Nillable(),
		field.String("excerpt").NotEmpty().Default(""),
		field.Text("search_text").NotEmpty().Default(""),
		field.Enum("status").Values("draft", "published").Default("draft"),
		field.UUID("author_id", uuid.UUID{}),
		field.Time("published_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
func (Post) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("posts", User.Type),  // 声明在 User 侧：edge.To("posts", Post.Type)（user.go 的 Edges 追加一行）；Post 侧为 edge.From("author", User.Type).Ref("posts").Unique().Required().Field("author_id")——对齐 recovery_code.go:29 的 M2O 先例
		edge.To("snapshots", PostSnapshot.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.From("headSnapshot", PostSnapshot.Type).Ref("headOf").Unique().Field("head_snapshot_id"),
		edge.From("releaseSnapshot", PostSnapshot.Type).Ref("releaseOf").Unique().Field("release_snapshot_id"),
	}
}
func (Post) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "created_at"),
		index.Fields("created_at"),
		index.Fields("author_id"),
		index.Fields("head_snapshot_id").Unique(),
		index.Fields("release_snapshot_id").Unique(),
	}
}
```

`post_snapshot.go`（`page_snapshot.go` 同构换 `page_id`）：

```go
func (PostSnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }),
		field.UUID("post_id", uuid.UUID{}),
		field.Int("version"),
		field.JSON("content", map[string]any{}),
		field.Text("content_html"),
		field.Text("content_text"),
		field.String("content_format").Default("pm1"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}
func (PostSnapshot) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("post", Post.Type).Ref("snapshots").Unique().Required().Field("post_id").Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("headOf", Post.Type).Unique().Field("head_snapshot_id"),
		edge.To("releaseOf", Post.Type).Unique().Field("release_snapshot_id"),
	}
}
func (PostSnapshot) Indexes() []ent.Index {
	return []ent.Index{index.Fields("post_id", "version").Unique()}
}
```

注意：`head_snapshot_id` / `release_snapshot_id` 两列由 `edge.From(...).Field(...)` 声明在 posts 表（列可空、创建后回填），PostSnapshot 侧的 `edge.To(...).Field(...)` 只是命名同列，不要重复建列——生成后以 Atlas diff 出的 SQL 为准核对（spec §12.3 循环外键条目）。

- [ ] **Step 1: 写失败测试**——`content/migration_test.go`：`testDB` 照抄 access（`migration_test.go:17-36`）；`TestContentSchema` 先经 `Client.User.Create()` 造一个真实用户（author_id 有外键，必须用真实 user id），随后以 ent 原生 API 建 Post（title/slug/author/content 四件套/excerpt/search_text）+ 同事务外再建一条 PostSnapshot（version=1）并回填 head_snapshot_id，断言：`Client.Post.Query().WithHeadSnapshot().WithAuthor()` 能取到两边；`Client.Post.DeleteOneID(id)` 后 `Client.PostSnapshot.Query().Count()==0`（级联删）；重复 slug 二次创建返回 `ent.IsConstraintError`；同 post 重复 version 返回 `ent.IsConstraintError`；`content_format` 缺省落 `pm1`、`status` 缺省落 `draft`。
- [ ] **Step 2: 跑测试确认失败**——`go test ./internal/modules/content/ -run TestContentSchema -v`，编译失败（无 Post 实体）。
- [ ] **Step 3: 落 schema 并生成**——按上方代码写四个 schema 文件（user.go 的 Edges 追加 `edge.To("posts", Post.Type)`、`edge.To("pages", Page.Type)`），跑 `./scripts/db-generate.sh`。
- [ ] **Step 4: 参数化迁移脚本并出迁移**——`db-migrate-diff.sh` 改为 `NAME="${1:?usage: db-migrate-diff.sh <name>}"` 并用 `"$NAME"` 替换硬编码；跑 `./scripts/db-migrate-diff.sh content_posts_pages`。
- [ ] **Step 5: 人工审阅迁移 SQL**（spec §13 五点）：`content` 列为 jsonb；`status` 的 CHECK 表达；循环外键两段式 ALTER；快照 `post_id` 为 ON DELETE CASCADE、head/release FK 为 NO ACTION；实体表 6 索引 + 快照表 1 复合唯一在位。若 `status` 的 CHECK 形态不可接受，把 `field.Enum` 退为 `field.String("status").Default("draft")` 并重跑本步（spec §16.6）。
- [ ] **Step 6: hash + validate + 应用**——`atlas migrate hash && atlas migrate validate`；设好 `DATABASE_URL`、`DATABASE_URL_TEST` 后 `./scripts/db-migrate-apply.sh`。
- [ ] **Step 7: 跑测试确认通过**——Step 2 命令全绿。
- [ ] **Step 8: 提交**——`git commit -m "feat(backend): 内容模块四表 ent schema 与迁移（posts/pages + 快照表）"`

### Task 3: 输入校验与派生纯函数（无 DB 单测）

**Files:**
- Create: `backend/internal/modules/content/service.go`（先只有校验部分与常量）、`backend/internal/modules/content/dto.go`（先只有输入类型）
- Test: `backend/internal/modules/content/service_test.go`

**Interfaces:**
- Produces（后续任务按这些签名消费）:

```go
package content

const (
	maxTitleRunes     = 200
	maxSlugRunes      = 120
	maxCoverURLRunes  = 2048
	maxContentBytes   = 256 * 1024
	maxContentDepth   = 64
	maxContentHTML    = 1024 * 1024
	maxContentText    = 256 * 1024
	maxFormatRunes    = 32
	excerptRunes      = 200
	maxQueryRunes     = 100
	defaultLimit      = 20
	maxLimit          = 100
)

type CreatePostInput struct {
	Title          string
	Slug           *string
	CoverImageURL  *string
	Content        map[string]any
	ContentHTML    string
	ContentText    string
	ContentFormat  *string
}

type UpdatePostInput struct {
	Title          *string
	Slug           *string
	CoverImageURL  *string
	Content        map[string]any
	ContentHTML    *string
	ContentText    *string
	ContentFormat  *string
}

func validateTitle(raw string) (string, error)
func validateOptionalSlug(raw *string) (*string, error)   // nil→nil；空串→nil（清空语义）；否则正则+长度
func validateOptionalCover(raw *string) (*string, error)  // nil→nil；空串→nil；否则 ≤2048
func validateContent(v map[string]any) error              // 非空对象、json.Marshal ≤maxContentBytes、jsonDepth ≤maxContentDepth
func jsonDepth(v any, depth int) (int, error)             // 递归 map/slice，超过 maxContentDepth 返回错误
func validateCached(name, raw string, max int) error      // content_html/content_text 用
func validateFormat(raw *string) error                    // 非 nil 时非空、≤32
func contentGroupComplete(in UpdatePostInput) error       // D6：任一出现→三者必须齐，缺哪返回 apierror.WithParam(ErrInvalidRequest, "content_html"/"content_text"/"content")；content_format 单独出现（内容组未出现）→ WithParam(ErrInvalidRequest, "content_format")（spec §7.1）
func excerptOf(text string, n int) string                 // 按 rune 截断
func escapeLike(q string) string                          // `\`→`\\`、`%`→`\%`、`_`→`\_`
func clampPage(limit, offset int) (int, int, error)       // 非法→apierror.WithParam(ErrInvalidRequest, "limit"/"offset")；钳制
```

- [ ] **Step 1: 写失败测试**——`service_test.go` 覆盖：title 空白/超长拒、trim 生效；slug 空→nil、`Hello` / `-a-` / 超长拒、合法过；content 数组/标量拒（借 json.Marshal 后 unmarshal 到 `map[string]any` 的入口断言）、257KB 拒、65 层嵌套拒且**不栈溢出**、64 层过；三列成组缺 html→param=`content_html`、缺 content→param=`content`、齐全通过、只有 title 通过；`content_format` 单独出现（内容组缺席）→ param=`content_format`；excerptOf 中文按 rune 截 200；escapeLike 三字符；clampPage limit=-1/0→400、limit=500→(100,…)、offset=-1→400。
- [ ] **Step 2: 跑测试确认失败**——`go test ./internal/modules/content/ -run TestValid -v`，编译失败。
- [ ] **Step 3: 实现**——按签名实现全部纯函数；深度递归只认 `map[string]any` 与 `[]any`。
- [ ] **Step 4: 跑测试确认通过**。
- [ ] **Step 5: 提交**——`git commit -m "feat(backend): 内容模块输入校验与派生纯函数"`

### Task 4: posts repository——创建/读/列表/删除（集成）

**Files:**
- Create: `backend/internal/modules/content/repository.go`、`backend/internal/modules/content/dto.go`（追加响应类型）
- Test: `backend/internal/modules/content/repository_test.go`

**Interfaces:**
- Consumes: Task 2 的 ent 包、Task 3 的校验函数。
- Produces:

```go
type authorRef struct{ ID, Nickname string }

type postRecord struct {
	Post     *ent.Post
	Head     *ent.PostSnapshot // 详情/派生用
	Author   authorRef
}

type repository struct{ client *ent.Client }

func (r *repository) createPost(ctx context.Context, authorID uuid.UUID, in CreatePostInput) (postRecord, error)
// 单事务：Post.Create(status=draft, excerpt/search_text 从 content_text 派生) → PostSnapshot.Create(version=1) → 回填 head_snapshot_id（spec §7.1）
func (r *repository) getPost(ctx context.Context, id uuid.UUID) (postRecord, error)      // WithAuthor + WithHeadSnapshot；不存在→apierror.ErrNotFound
func (r *repository) getPublishedPost(ctx context.Context, id uuid.UUID) (postRecord, error) // release==nil 或 status!=published→ErrNotFound（防探测：同一哨兵）
func (r *repository) listPosts(ctx context.Context, p listParams) ([]postRecord, int, error)  // p 含 PublishedOnly bool/Status 过滤/Q（视为已转义纯文本，转义发生在 parseListQuery）/limit/offset；只取实体列+WithAuthor，不取快照大列；created_at DESC, id DESC（spec §6.2 次级稳定键）
func (r *repository) deletePost(ctx context.Context, id uuid.UUID) error                 // 依赖 CASCADE；NotFound→ErrNotFound；若 NO ACTION 指针 FK 拦截级联（spec §16.10），退为事务内先置 NULL 两指针再删实体
type listParams struct { Limit, Offset int; Status *string; Q *string; PublishedOnly bool }
```

DTO 追加（响应形状 = spec §6.3，slug/cover/published_at 可空字段用指针）：

```go
type AuthorRef struct{ ID string `json:"id"`; Nickname string `json:"nickname"` }
type PostListItem struct {
	ID string `json:"id"`; Title string `json:"title"`; Slug *string `json:"slug"`
	Status string `json:"status"`; Excerpt string `json:"excerpt"`; CoverImageURL *string `json:"cover_image_url"`
	Author AuthorRef `json:"author"`; PublishedAt *int64 `json:"published_at"`
	CreatedAt int64 `json:"created_at"`; UpdatedAt int64 `json:"updated_at"`
}
type PostDetail struct {
	PostListItem
	Content map[string]any `json:"content"`; ContentHTML string `json:"content_html"`
	ContentText string `json:"content_text"`; ContentFormat string `json:"content_format"`
	HeadVersion int `json:"head_version"`; ReleaseVersion *int `json:"release_version"`
}
type PublicPostDetail struct { // PostDetail 去 status/head_version/release_version
	ID/Title/Slug/Excerpt/CoverImageURL/Author/PublishedAt/CreatedAt/UpdatedAt + Content 四件套
}
type ListEnvelope[T any] struct {
	Data []T `json:"data"`; Total int `json:"total"`; Limit int `json:"limit"`; Offset int `json:"offset"`
}
```

- [ ] **Step 1: 写失败测试**（`repository_test.go`，`testDB` 同 Task 2 的文件内复用）——创建：返回记录 status=draft、head.Version==1、excerpt==content_text 前 200、release 为 nil；slug 缺省落 NULL；getPost 不存在→ErrNotFound；getPublishedPost 对草稿与不存在返回**同一哨兵** ErrNotFound；listPosts PublishedOnly 只出已发布（先手动 ent 置 status）、Q 过滤命中 title、total 正确、limit/offset 分页正确且**响应结构里无快照列**（断言查询用 `.Select(...)` 字段集或直接断言返回 record 不含 snapshot）；deletePost 后快照计数 0、再删→ErrNotFound。
- [ ] **Step 2: 跑测试确认失败**——`go test ./internal/modules/content/ -run TestPostRepo -v`，编译失败。
- [ ] **Step 3: 实现**——事务模式照抄 `access/repository.go:167-196`（Tx + defer Rollback + Commit）；发布侧数据用 `ent.Post.Query().WithAuthor().WithHeadSnapshot()`；公开/未发布同哨兵；q 用 `post.TitleContains`/`post.SearchTextContains` 不存在——ent 的 ILIKE 用 `post.TitleLikeFold` 或手写 predicate `entsql.FieldContainsFold`，实现时取仓库内可行者（ent fold 谓词若不可用，用 `func(s *sql.Selector)` 自写 `ILIKE`，值两侧拼 `%`）。
- [ ] **Step 4: 跑测试确认通过**。
- [ ] **Step 5: 提交**——`git commit -m "feat(backend): posts 仓储——创建快照事务/读/列表/级联删除"`

### Task 5: posts 更新（追加快照）/publish/unpublish/恢复（集成）

**Files:**
- Modify: `backend/internal/modules/content/repository.go`（追加方法）、`backend/internal/modules/content/service.go`（Service 结构体 + 业务方法）
- Test: `backend/internal/modules/content/repository_test.go`（追加）、`backend/internal/modules/content/service_test.go`（追加集成段）

**Interfaces:**
- Produces:

```go
type Service struct{ repo *repository }
func newService(db *database.Database) *Service

func (s *Service) CreatePost(ctx context.Context, authorID uuid.UUID, in CreatePostInput) (PostDetail, error)
func (s *Service) UpdatePost(ctx context.Context, id uuid.UUID, in UpdatePostInput) (PostDetail, error)
// 元数据-only PATCH：不改快照；带内容组：contentGroupComplete→事务内 ForUpdate 锁实体行→version=max+1→插快照(content_format 缺省继承 head)→head 前移→刷新 excerpt/search_text/updated_at（spec D16/D6）
func (s *Service) DeletePost(ctx context.Context, id uuid.UUID) error
func (s *Service) PublishPost(ctx context.Context, id uuid.UUID) (PostDetail, error)
// 事务内 ForUpdate：draft→published 或 release!=head → release←head 且 published_at=now；纯重放返回现状不刷新（D17）
func (s *Service) UnpublishPost(ctx context.Context, id uuid.UUID) (PostDetail, error) // status←draft，release 保留
func (s *Service) ListPostSnapshots(ctx context.Context, postID uuid.UUID, limit, offset int) (ListEnvelope[SnapshotListItem], error)
func (s *Service) GetPostSnapshot(ctx context.Context, postID uuid.UUID, version int) (SnapshotDetail, error) // version<=0→WithParam(ErrInvalidRequest,"version")；不存在→ErrNotFound
func (s *Service) RestorePostSnapshot(ctx context.Context, postID uuid.UUID, version int) (SnapshotDetail, error)
// 复制旧版为新快照（version=max+1、head 前移、派生列刷新），绝不 UPDATE/DELETE 旧快照行（D16）
func (r *repository) appendSnapshot(ctx context.Context, postID uuid.UUID, content map[string]any, html, text, format string) (*ent.PostSnapshot, error)
type SnapshotListItem struct { Version int `json:"version"`; ContentFormat string `json:"content_format"`; IsHead bool `json:"is_head"`; IsRelease bool `json:"is_release"`; CreatedAt int64 `json:"created_at"` }
type SnapshotDetail struct { SnapshotListItem; Content map[string]any `json:"content"`; ContentHTML string `json:"content_html"`; ContentText string `json:"content_text"` }
```

- [ ] **Step 1: 写失败测试**——更新：元数据-only PATCH 后 head.Version 不变、updated_at 变；内容组 PATCH → 新快照 version+1、head 指向它、excerpt 刷新、format 缺省继承；只带 content 缺 html → param=`content_html`；publish：draft→published 且 published_at 刷新、release==head；已发布再 publish（无新编辑）→ published_at 不变；发布后编辑 → 公开读仍旧版（getPublishedPost 内容=旧 release）→ 再 publish → 刷新；unpublish → status=draft、release 保留；恢复 v1 → 产生 version=max+1 的新 head、历史快照数只增不减、is_head/is_release 标记正确；`GetPostSnapshot` version=0 → param=`version`、999 → ErrNotFound；**并发**：`sync.WaitGroup` 两 goroutine 各做一次内容组 PATCH → 成功数 2、最终 version=max(v1)+2、两响应 version 互异（锁线性化）。
- [ ] **Step 2: 跑测试确认失败**——`go test ./internal/modules/content/ -run "TestUpdate|TestPublish|TestRestore|TestSnapshot" -v`，编译失败。
- [ ] **Step 3: 实现**——version 分配：`tx.Post.Query().Where(post.IDEQ(id)).ForUpdate().All(ctx)` 锁行后 `tx.PostSnapshot.Query().Where(postsnapshot.PostIDEQ(id)).Order(ent.Desc(postsnapshot.FieldVersion)).First(ctx)` 取 max；事务模板照抄 `access/repository.go:209-238`。
- [ ] **Step 4: 跑测试确认通过**。
- [ ] **Step 5: 提交**——`git commit -m "feat(backend): posts 更新追加快照/发布状态机/版本恢复"`

### Task 6: 公开读端点 + 分页/搜索 + app 装配（集成）

**Files:**
- Create: `backend/internal/modules/content/controller.go`（先公开部分）、`backend/internal/modules/content/module.go`
- Modify: `backend/internal/app/app.go`（装配行）
- Test: `backend/internal/modules/content/controller_test.go`

**Interfaces:**
- Consumes: Task 4 的 `listParams` / `ListEnvelope` / `PostListItem` / `PublicPostDetail`。
- Produces:

```go
func New(db *database.Database) *Module          // Module{Service, controller}，对齐 access/module.go
func (m *Module) Register(r *gin.RouterGroup)    // 本任务只注册 GET /posts、GET /posts/:id；pages 两条路由由 Task 9 一并注册
func parseListQuery(ctx *gin.Context, withStatus bool) (listParams, error) // limit/offset 钳制、q≤100 且 escapeLike 转义、status 白名单
```

公开路由 handler：`:id` 非 UUID → `apierror.WithParam(ErrInvalidRequest, "id")`；查询失败一律 `apierror.Write`；200 返回 `ListEnvelope[PostListItem]` / `PublicPostDetail`（无 status/head_version/release_version）。app.go 在 `accessModule := access.New(db)` 之后追加：

```go
contentModule := content.New(db)
contentModule.Register(api)
```

- [ ] **Step 1: 写失败测试**——`controller_test.go` 建 httptest 骨架：照抄 `access/access_test.go:24-61`（scs + `/seed` + cookiejar + `doJSON`），公开路由**无守卫**直接可访问。用例：空库列表 → `{"data":[],"total":0,"limit":20,"offset":0}`；limit=500 钳制回显 100；limit=abc → 400 param=`limit`；q=`%` 只按字面匹配（造两篇 title 分别含字面 `%` 与任意文本，断言只有前者命中）；`:id=not-a-uuid` → 400 param=`id`；草稿 id 与不存在 id → **错误体逐字节相同**且 404；已发布详情返回 PublicPostDetail 形状（断言无 `status`/`head_version` 键）。
- [ ] **Step 2: 跑测试确认失败**——`go test ./internal/modules/content/ -run TestPublic -v`，编译失败。
- [ ] **Step 3: 实现**——controller/module/app 装配。
- [ ] **Step 4: 跑测试确认通过**；另跑 `go build ./...` 确认 app 装配无破坏。
- [ ] **Step 5: 提交**——`git commit -m "feat(backend): 内容模块公开读端点与装配"`

### Task 7: 管理端点（CRUD + publish/unpublish）+ 权限挂载（集成）

**Files:**
- Modify: `backend/internal/modules/content/controller.go`、`backend/internal/modules/content/module.go`（RegisterProtected）
- Test: `backend/internal/modules/content/controller_test.go`（追加）

**Interfaces:**
- Consumes: Task 1 的 4 个 rbac 常量、guard.RequirePermission（先例 `access/controller.go:42-50`）。
- Produces:

```go
func (m *Module) RegisterProtected(r *gin.RouterGroup) // 本任务注册 7 条 /admin/posts（CRUD 5 + publish/unpublish 2）；快照 3 条 Task 8、pages 10 条 Task 9
// GET  /admin/posts            content.posts.view
// POST /admin/posts            content.posts.edit    201，author 取 guard 身份
// GET  /admin/posts/:id        content.posts.view    PostDetail
// PATCH /admin/posts/:id       content.posts.edit    指针可空、至少一字段否则 400
// DELETE /admin/posts/:id      content.posts.edit    200 {"id":"…","deleted":true}
// POST /admin/posts/:id/publish|unpublish  content.posts.edit  200 PostDetail
```

- [ ] **Step 1: 写失败测试**——权限矩阵：未登录访问管理端点 401；登录无权限 403；`content.posts.edit` 用户（蕴含 view）全通；无权限用户 GET 列表 403。CRUD：创建 201 且 `head_version==1`、`release_version==null`、author 回显操作者、请求携带 `status`/`author_id` 被忽略；PATCH 全缺省 → 400；PATCH 改 title → 200；创建重复 slug → 409 `slug_taken` param=`slug`；DELETE → 200 确认体，再 GET → 404；publish/unpublish 幂等语义（复用 Task 5 断言口径走 HTTP）。
- [ ] **Step 2: 跑测试确认失败**——`go test ./internal/modules/content/ -run TestAdmin -v`，编译失败。
- [ ] **Step 3: 实现**——registerProtected 逐条挂 `guard.RequirePermission`；操作者取法照抄 access 的 `operatorFrom`（guard 身份）。
- [ ] **Step 4: 跑测试确认通过**。
- [ ] **Step 5: 提交**——`git commit -m "feat(backend): 内容管理端点与权限挂载"`

### Task 8: 快照三端点（集成）

**Files:**
- Modify: `backend/internal/modules/content/controller.go`（追加 3 条路由 ×2 实体占位）
- Test: `backend/internal/modules/content/controller_test.go`（追加）

**Interfaces:**
- Produces 路由：`GET /admin/posts/:id/snapshots`（view，分页信封，项=SnapshotListItem）、`GET /admin/posts/:id/snapshots/:version`（view，SnapshotDetail）、`POST /admin/posts/:id/snapshots/:version/restore`（edit，200 返回复制出的新快照）。`:version` 非整数 → 400 param=`version`（spec §6.2）。

- [ ] **Step 1: 写失败测试**——造 3 版历史：列表分页/total 正确、新版本在前、is_head/is_release 正确；version=abc → 400 param=`version`；999 → 404；restore 后列表多一条且 head 前移；无 edit 权限（仅 view）restore → 403。
- [ ] **Step 2: 跑测试确认失败**——`go test ./internal/modules/content/ -run TestSnapshotAPI -v`，编译失败。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 跑测试确认通过**。
- [ ] **Step 5: 提交**——`git commit -m "feat(backend): 快照历史三端点（列表/详情/恢复）"`

### Task 9: pages 同构充实（集成）

**Files:**
- Modify: `backend/internal/modules/content/repository.go`、`service.go`、`controller.go`（pages 全套方法，形态与 posts 逐一同构）
- Test: `backend/internal/modules/content/controller_test.go`（追加 pages 段）

**Interfaces:**
- Produces: `CreatePage/GetPage/UpdatePage/DeletePage/PublishPage/UnpublishPage/ListPages/ListPublishedPages/ListPageSnapshots/GetPageSnapshot/RestorePageSnapshot` 与 posts 同签名（`*ent.Page` / `page` 包替换）；无 `q`（spec §9：pages 不提供搜索，parseListQuery withStatus=false 且不解析 q）。
- 实现口径：**逐字镜像 posts 实现**，仅类型与路由替换（access 一模块承载两实体的先例；不抽象泛型，保持直白）。

- [ ] **Step 1: 写失败测试**——pages 全链路冒烟：创建→快照 v1→publish→公开 /pages/:id 可读且读 release→unpublish→公开 404→恢复→再发布；权限矩阵抽查（pages.view/pages.edit）；公开 /pages 列表无 q 参数（带 q= 被忽略不报错）。
- [ ] **Step 2: 跑测试确认失败**——`go test ./internal/modules/content/ -run TestPage -v`，编译失败。
- [ ] **Step 3: 实现**——镜像。
- [ ] **Step 4: 跑测试确认通过**。
- [ ] **Step 5: 提交**——`git commit -m "feat(backend): pages 全套同构实现"`

### Task 10: 文档、版本与全量验收

**Files:**
- Modify: `backend/README.md`（模块目录表、接口表 24 条、权限目录 +4）、`DEVLOG`（新条目：内容模块 + 快照方案）
- Modify: 版本号文件（`./scripts/bump-version.sh minor`，按执行时当前版本顺延——2026-10-11 时点为 0.7.0 → 0.8.0）

- [ ] **Step 1: 全量验证**——`cd backend && go build ./... && go vet ./... && gofmt -l .` 全干净；`go test ./...` 全绿（无 `DATABASE_URL_TEST` 时集成段 skip 属预期）。
- [ ] **Step 2: spec §15 手工抽查**——起 dev server（`scripts/dev.sh`，用 `Start-Process` 防随回合被杀），按 §15 第 5 条 curl 清单逐条过：公开列表信封、草稿详情 404、limit 钳制、创建 head_version=1、发布后编辑公开侧仍旧版→再发布更新、unpublish release 保留、恢复追加新 head、slug 冲突 409、无权限 403、删除后快照清零。
- [ ] **Step 3: README/DEVLOG/版本**——更新三处；`./scripts/bump-version.sh minor`。
- [ ] **Step 4: 提交**——`git commit -m "docs(backend): 内容模块 README/DEVLOG，版本进位"`
