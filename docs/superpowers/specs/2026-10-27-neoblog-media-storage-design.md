# NeoBlog 媒体存储（任意格式上传 + 本地磁盘 + 静态直出）

**日期**：2026-10-27 · **范围**：后端（`database` / `media` / `app`）+ 文档 · **实施计划**：待本 spec 审阅通过后另写（`docs/superpowers/plans/`）

**状态**：v1.2 待用户审阅。口径来自 2026-10-25 与 2026-10-27 两轮对话裁定：**任意格式**、**只做后端**、**本地磁盘 + provider 预留**、**URL 不带 provider**、**静态直出**、**删除先删文件再删记录**、**大小上限只留插槽**、**上传/下载放宽服务器 deadline**。

修订记录：

- v1（2026-10-27）：首版，限定 jpeg/png/gif/webp。
- v1.1（2026-10-27）：用户裁定「适配所有格式、不做格式验证」。删除 MIME 白名单与内容嗅探，删除 `mime_type` 列与 `media_type_not_allowed` 错误码；新增静态服务的安全响应策略。
- v1.2（2026-10-27）：按复核结论修复——ent 边写法、测试注入点、超时、数据库不可用的状态码、删除顺序、「数据库挂了图还能看」的口径、原始文件名用途、ent 生成步骤、文档同步清单，以及一批并发与异常口径。

## 1. 意图与背景

文章模块（posts / pages）尚未动工，而文章正文要嵌图、附件要挂文件，所以媒体存储是它的前置能力。当前后端**完全没有**上传与静态文件能力：没有 media 模块、ent 里没有媒体表、`frontend/next.config.ts` 只代理 `/api/*`。

用户逐条拍定：

1. 存储后端**预留 provider 接口**，第一版只实现本地磁盘；
2. **删除不做引用检查**——删文件 + 删记录，历史快照里的旧文件直接变死链（与 WordPress 一致），这是用户知情接受的不可逆行为；
3. **任意格式都能上传，服务端不做类型验证**；
4. **URL 不带 provider**：`/storage/<yyyy>/<mm>/<uuid>.<ext>`；
5. `/storage/` 这个前缀**永久归本地存储**，接受由此带来的限制；
6. 文件**大小上限只预留接口**，本轮不设任何具体数值；
7. 路径**写死在代码里**，不读环境变量。

目标：登录的 owner/admin 能传任意文件并拿到一个可直接引用的相对 URL；前台取文件不经过应用路由与数据库。

## 2. 已定决策

| # | 决策 | 口径 |
| --- | --- | --- |
| D1 | 文件类型 | **不做任何类型校验**：不嗅探内容、不查白名单、不拒收。视频、音频、压缩包、PDF、代码文件都能传。**唯一例外是服务时的处理**（见 D15），不影响上传 |
| D2 | 存储抽象 | `Provider` 接口：`Name` / `Save` / `Delete` / `URL` 四个方法。首版只实现 `local`。注册表由 `media.New` 构造（map），不做 `init()` 全局注册、不做插件热加载 |
| D3 | 读路径 | `/storage/**` 直接映射项目根下的 `data/storage`，**不查数据库、不经过任何业务 handler**。**注意适用范围**：只有**匿名请求**完全不受数据库影响；带会话 cookie 的请求仍会先经过 `sessions.LoadAndSave`，数据库不可用时由会话中间件返回 503，到不了静态处理器 |
| D4 | URL 形状 | `/storage/2026/10/27/<uuid>.<ext>`。**不带 provider 名**。`/storage/` 永久属于 `local`；将来加 S3 时，新文件的地址由该 provider 自己给出（`Provider.URL` 返回绝对地址），老文件与老 URL 不受影响 |
| D5 | 数据表 | 一张 `media_assets`：`id` / `provider` / `key` / `size_bytes` / `original_name` / `uploaded_by` / `created_at`。**不存 mime_type**、不加宽高、不加 sha1、不加缩略图 key |
| D6 | 删除语义 | **先删文件、再删记录**。读记录 → 删文件 → 删记录。文件删除失败（除「文件本来就不存在」）→ 500，记录保留，可直接重试；「文件本来就不存在」视为已删除、继续删记录。列出的两行并发删除：一个 200，另一个 404 |
| D7 | 大小上限 | 只留 `UploadPolicy` 接口插槽，v1 注入 `allowAll`。**不设任何数值**、不上 `http.MaxBytesReader`。按 AGENTS.md，必须在 `docs/DEVLOG.md` 记明「用户 2026-10-27 授权本轮不限制上传大小与文件类型，适用范围 `POST /api/v1/media`」 |
| D8 | 扩展名 | **取自客户端原始文件名**：转小写、去掉点、必须匹配 `^[a-z0-9]{1,16}$`；不匹配或没有扩展名 → `bin`。服务端不根据内容纠正扩展名 |
| D9 | 文件名 | 随机 UUID（与记录主键同一个），**从不使用原始文件名做路径**。`original_name` 只存库、只在上传响应里返回一次；它**不会出现在下载文件名里**（静态直出拿不到它） |
| D10 | 落盘 | `data/storage/<yyyy>/<mm>/<uuid>.<ext>`，年月按 UTC。先写同目录临时文件再 `rename`。目录日期与 `created_at` 用**同一个 `now`** |
| D11 | 权限 | 上传与删除：`RequireAuth` + `RequirePresetGroup(owner, admin)`。**不新增权限点**、不动 `rbac` 目录、不需要补种迁移 |
| D12 | 路由 | `POST /api/v1/media`（受保护）、`DELETE /api/v1/media/:id`（受保护）、`GET /storage/*key` 与 `HEAD /storage/*key`（公开，不在 `/api` 下）。**不做列表接口** |
| D13 | 目录列举与畸形路径 | 不改写 gin 的 `RedirectTrailingSlash`：`GET /storage` 会先被 301 跳到 `/storage/`，然后落到 404。key 形状校验不通过的路径直接 404，不碰文件系统、不列目录 |
| D14 | 缓存 | `X-Content-Type-Options: nosniff` 加在所有 handler 响应上。`Cache-Control` 按状态分档：**200 / 206 / 304 用 `public, max-age=31536000, immutable`；≥400（404 / 416 / 403 / 5xx）一律 `no-store`**。理由：`immutable` 承诺的是「这个 URL 的内容永不变化」，而错误状态不是内容——运维误删、磁盘故障、从备份恢复、目录权限配错都会让文件回来，缓存一年的错误会让它永远刷不出来。gin 在路由层生成的 `/storage` → `/storage/` 301 与 405 不经过 handler，不带这两个头 |
| D15 | 服务时的类型策略 | **内联表**里的扩展名按表中映射的类型返回、浏览器直接渲染；**其余一律** `application/octet-stream` + `Content-Disposition: attachment`，强制下载。内联表只收浏览器原生支持的图片/音视频，**不含 html / svg / js / xml / pdf**——否则任意文件上传会变成同源 XSS |
| D16 | 超时 | 上传入口清掉服务器读 deadline，`/storage` 入口清掉写 deadline（`http.ResponseController`）。**不设具体秒数**，`ReadHeaderTimeout` 与 `IdleTimeout` 保持原样 |

## 3. 与主流的对照

调研对象：WordPress、Halo 2、Ghost、Discourse。

| 维度 | 主流做法 | 本方案 |
| --- | --- | --- |
| 元数据 | WP 塞文章表；Halo / Discourse 独立表；Ghost 无表 | 独立表（Halo / Discourse 路线） |
| 存储抽象 | Halo 存储策略记在每条记录上；Ghost 适配器按功能分 | 记录级 `provider` 列 + 四方法接口 |
| 读文件 | 四家都是静态直出、不查库（Halo 是应用内静态资源映射 `/upload/**` → 磁盘） | 同 Halo：静态映射 `/storage/**` → `data/storage` |
| URL 与存储的关系 | WP 路径自带位置；Halo 各策略有各自的地址规则 | `/storage/` 固定给 `local`；其他 provider 将来给绝对地址 |
| 文件名 | WP 保留原名 + `-1`；Discourse 用内容哈希；Halo 可选随机名 | UUID，原名只存库 |
| **类型限制** | **四家都有白名单** | **不做**（用户裁定），代价由 D15 的服务策略兜 |
| 多尺寸 | 四家都有 | 不做 |
| 去重 | Discourse 内容哈希 | 不做 |
| 引用检查 | 只有 Discourse | 不做（用户已裁定） |

本方案在类型上是四家里最宽松的，因此**服务端的下载策略（D15）是本方案新增的一层，主流没有对应物**——它们靠上传时拒收来消除风险，我们靠响应头消除。

## 4. 范围

### 4.1 后端

| 文件 | 动作 | 职责 |
| --- | --- | --- |
| `internal/database/ent/schema/media_asset.go` | 新增 | 媒体记录表定义 |
| `internal/database/ent/schema/user.go` | 改 | 增加 `edge.To("mediaAssets", MediaAsset.Type).Annotations(entsql.OnDelete(entsql.SetNull))` |
| `internal/database/ent/**` | 生成 | 跑 `scripts/db-generate.sh` 后提交全部生成物（实体、builder、`migrate/schema.go` 等）。**不跑生成，代码引用 `ent.MediaAsset` 无法编译** |
| `migrations/<timestamp>_media_assets.sql` | 新增 | 建表、`key` 唯一索引、`uploaded_by` 外键（`ON DELETE SET NULL`）。先改 `scripts/db-migrate-diff.sh` 的迁移名参数再生成，人工审阅 SQL 后把参数改回去，并 `atlas migrate hash`；`migrations/atlas.sum` 一并提交 |
| `internal/modules/media/module.go` | 新增 | **只做装配**：`New(db, root string) (*Module, error)` 调用 service 构造函数，把 controller 挂上；`RegisterStorage(engine *gin.Engine)` / `RegisterProtected(group *gin.RouterGroup)` |
| `internal/modules/media/service.go` | 新增 | 目录准备（`filepath.Abs`、`MkdirAll`、`os.CreateTemp` 写探针）、provider 注册表、扩展名归一与原始名校验、上传 / 删除两条主流程、`UploadPolicy` 调用 |
| `internal/modules/media/provider.go` | 新增 | `Provider` 接口、注册表类型、`UploadPolicy` 接口与 `allowAll` |
| `internal/modules/media/local.go` | 新增 | 本地磁盘实现：`Save`（建目录 / 临时文件 / rename）、`Delete`、`URL` |
| `internal/modules/media/repository.go` | 新增 | 两个方法：插入记录（带显式 `created_at`）、按 id 查询并删除 |
| `internal/modules/media/controller.go` | 新增 | 三条路由的 HTTP 编解码；静态处理器、deadline 调整、key 形状校验、D15 的响应头策略 |
| `internal/modules/media/dto.go` | 新增 | 上传响应结构 |
| `internal/modules/media/media_test.go` | 新增 | 集成测试：上传 / 删除 / 权限 / 静态读 / Range / 404 / 下载策略 / 补偿 |
| `internal/modules/media/service_test.go` | 新增 | 扩展名归一、原始名校验、上传失败补偿的表驱动单测 |
| `internal/modules/media/local_test.go` | 新增 | 本地 provider 单测：目录创建、临时文件清理、URL 拼接、删除不存在的文件 |
| `internal/modules/media/schema_test.go` | 新增 | 表与索引存在性、外键 `ON DELETE SET NULL` 行为 |
| `internal/app/app.go` | 改 | `const mediaRootPath = "../data/storage"`；`New(cfg, db)` 转调 `newWithRoot(cfg, db, mediaRootPath)`；新增 `newWithRoot(cfg, db, mediaRoot string)`；装配 `media.New(db, mediaRoot)`；给 `writeTracker` 加 `Unwrap()` |
| `internal/app/app_test.go` | 改 | 3 处 `New(cfg, db)`（第 63、566、1092 行）改成 `newWithRoot(cfg, db, t.TempDir())`，避免测试把目录建到仓库里 |
| `.gitignore`（仓库根） | 改 | 补 `/data/`，媒体目录不入口 |

`internal/config` 与 `backend/.env.example` **不动**：路径是代码常量，没有新增环境变量。

### 4.2 文档

| 文件 | 动作 | 职责 |
| --- | --- | --- |
| `backend/README.md` | 改 | 模块表新增 `internal/modules/media/*`；接口表新增三行（上传 / 删除 / 静态读）；新增「媒体存储」章节；**同时修正会被这次改动打成假话的三处**：第 174 行「`HEAD` 与 `OPTIONS` 没有任何端点注册」（现在注册了 `HEAD /storage/*key`）、第 170 行「唯一例外 `/healthy` 豁免错误信封」（`/storage` 是第二个）、第 150 行的迁移清单补 `media_assets` |
| `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md` | 改 | §3 端点表顺延新增两行（当前最大编号 31）；§4 规则 1「全部端点挂在 `/api/v1`」、规则 3「所有 body 一律 JSON」、规则 5「唯一例外 `/healthy`」三处补上 `/storage` 这条例外 |
| `docs/superpowers/specs/2026-10-11-neoblog-content-design.md` | 改 | 第 171、191、298、372 行的 `/uploads/...` 示例同步成 `/storage/...`，避免将来做内容模块时照抄旧前缀 |
| `docs/DEVLOG.md` | 改 | 功能条目 + 不限制大小与类型的授权记录（D7） |

### 4.3 前端

本轮**不接前端**。`/storage/*` 的 rewrite、媒体库界面、文章编辑器插图都不在本轮范围。接前台时需要：

- `frontend/next.config.ts` 的 `rewrites()` 增加 `{ source: "/storage/:path*", destination: `${backendUrl}/storage/:path*` }`；
- 前台渲染时直接用后端返回的相对 URL（与正文快照里存相对路径的口径一致）。

## 5. 数据模型

```go
type MediaAsset struct{ ent.Schema }

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

`User` 侧：

```go
edge.To("mediaAssets", MediaAsset.Type).Annotations(entsql.OnDelete(entsql.SetNull))
```

字段说明：

- `provider`：上传时生效的 provider 名（本轮恒为 `local`）。读文件不用它（静态映射），删除时才用它找实现。
- `key`：相对媒体根目录的逻辑路径，`2026/10/27/<uuid>.<ext>`。全局唯一。
- `original_name`：客户端给的原始文件名，Go 的 `Part.FileName()` 已剥掉路径。允许空串；**写入前拒绝含 NUL 或其他 `utf8.ValidString` 为假的值**。
- `uploaded_by`：可空；用户被删除时由外键置空（`ON DELETE SET NULL`），不阻塞用户删除。
- 没有 `mime_type`：类型由 `key` 的扩展名表达。
- `size_bytes` 的 `NonNegative()` 只是 Go 侧运行时校验，**数据库没有 CHECK 约束**；测试不要断言 DB 级非负。

**关于 `edge.From(...).Optional()`**：ent v0.14.6 的 `inverseBuilder` 没有这个方法（`schema/edge/edge.go`），写了编译不过。反向边默认就是可选，只保留 `Unique()` + `Field()`。

## 6. 接口

### 6.1 上传

```
POST /api/v1/media
Content-Type: multipart/form-data
字段：file（单文件，任何类型）
```

响应 201，裸对象：

```json
{
  "id": "0192f0c4-...",
  "url": "/storage/2026/10/27/0192f0c4-....zip",
  "size_bytes": 184320,
  "original_name": "素材包.zip"
}
```

不带 `created_at`：本轮没有消费方，且响应时间戳格式（API 规范 §14 的 D7 要求 epoch 秒，现状模块里混用 RFC3339）属于另一条未落地的规范项，不在本 spec 里开新口子。列本身存在，将来加列表接口时按 API 规范 D7 输出。

原始文件名里如果没有可用扩展名，URL 会是 `<uuid>.bin`。

### 6.2 删除

```
DELETE /api/v1/media/:id
```

响应 200：

```json
{"id":"0192f0c4-...","deleted":true}
```

`:id` 不是合法 UUID 或记录不存在 → 404 `not_found`。行已被并发删除（删除影响 0 行）→ 404 `not_found`。

### 6.3 读文件

```
GET /storage/2026/10/27/0192f0c4-....jpg
```

- 200 + 文件字节。
- `Content-Type` 与 `Content-Disposition` 由扩展名按 D15 决定。
- `Range` → 206；`If-Modified-Since` → 304。
- **多段 Range** 时 `Content-Type` 会被 `ServeContent` 改成 `multipart/byteranges`，这是标准行为、不在 D15 覆盖范围内。
- key 形状不合法 → 404，**空 body**，`Cache-Control: no-store`，不碰文件系统。
- key 形状合法但文件不存在 → 404，body 是 `http.FileServer` 的纯文本 `404 page not found`，`Cache-Control: no-store`。
- 两种 404 都**不是** JSON 错误信封——这是静态资源的例外，与 `/healthy` 探针同理，需在 README 记明。
- 200 / 206 / 304 带 `Cache-Control: public, max-age=31536000, immutable`；**≥400 的全部响应**（含 `ServeContent` 的 416 与 `FileServer` 的 403 / 5xx）由 `notFoundNoStore` 覆盖成 `no-store`。
- `GET /storage`（无尾斜杠）→ gin 先 301 到 `/storage/`，随后 404。这个 301 与 `POST /storage/x` 的 405 都由 gin 路由层产生、不经过 handler，因此不带上述响应头。

### 6.4 错误码

**不新增任何错误码。**

| 情况 | 状态 | code |
| --- | --- | --- |
| 不是 multipart / 缺 `file` 字段 / 文件名字段含 NUL | 400 | `invalid_request` |
| 未登录 | 401 | `unauthorized` |
| 非 owner/admin | 403 | `forbidden` |
| 删除时 id 非法或不存在（含并发删除输了） | 404 | `not_found` |
| 落盘失败、删除文件失败、非连接类的写库错误 | 500 | `internal_error` |
| 写库时数据库不可用（连接级错误，由 `apierror` 识别） | 503 | `service_unavailable` |

API 规范的状态码清单里没有 413，所以将来引入大小限制时也只能报 400（或先改规范）。

## 7. 存储层

### 7.1 Provider 接口

```go
type Provider interface {
    Name() string
    Save(ctx context.Context, key string, r io.Reader) (int64, error)
    Delete(ctx context.Context, key string) error
    URL(key string) string
}
```

- `Save` 返回实际写入字节数，供 `size_bytes` 使用；由实现负责建目录、临时文件、rename。
- `Open` **不定义**：读文件走静态映射，没有任何调用方。
- `URL` 让「对外地址长什么样」也归 provider 管：`local` 返回 `/storage/<key>`，将来的 S3 返回它自己的绝对地址。
- 注册表是 `media.New` 里的局部 map，不是全局变量。写入用哪个 provider：只有一个实现时就是 `local`，不设选择开关。
- 删除时记录里的 `provider` 在注册表里找不到 → 500，不回退到 `local`。

### 7.2 本地实现

- 根目录：由 `media.New(db, root)` 传入，service 里 `filepath.Abs` 归一后使用。
- `Save`：`os.MkdirAll(dir, 0o755)` → 在**同一目录**建 `.<uuid>.tmp` → 写入 → `Close` → `os.Rename` 到 `<uuid>.<ext>`。任何一步失败都删掉临时文件再把错误上抛。
- `Delete`：`os.Remove`，错误上抛。调用方把 `fs.ErrNotExist` 当作「已经删掉了」继续。
- `URL`：`"/storage/" + key`。

### 7.3 目录与 URL

```
data/storage/2026/10/27/0192f0c4-....zip
             └──────┬──────┘
URL: /storage/2026/10/27/0192f0c4-....zip
```

`now := time.Now().UTC()` 同时用于目录日期与记录的 `created_at`（插入时显式 `SetCreatedAt(now)`），避免凌晨跨日时两者差一天。

### 7.4 静态映射与下载策略

```go
var inlineTypes = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png",
	".gif": "image/gif", ".webp": "image/webp", ".avif": "image/avif",
	".bmp": "image/bmp", ".ico": "image/x-icon", ".heic": "image/heic",
	".mp4": "video/mp4", ".webm": "video/webm", ".ogv": "video/ogg",
	".mov": "video/quicktime", ".m4v": "video/x-m4v",
	".mp3": "audio/mpeg", ".wav": "audio/wav", ".ogg": "audio/ogg",
	".oga": "audio/ogg", ".m4a": "audio/mp4", ".flac": "audio/flac",
	".aac": "audio/aac", ".opus": "audio/opus",
}

var keyPattern = regexp.MustCompile(
	`^/\d{4}/\d{2}/\d{2}/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.[a-z0-9]{1,16}$`,
)

type notFoundNoStore struct {
	http.ResponseWriter
}

func (w notFoundNoStore) WriteHeader(code int) {
	if code == http.StatusNotFound {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.ResponseWriter.WriteHeader(code)
}

fileServer := http.StripPrefix("/storage/", http.FileServer(http.Dir(root)))

handler := func(c *gin.Context) {
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "public, max-age=31536000, immutable")

	if err := http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{}); err != nil {
		c.Header("Cache-Control", "no-store")
		c.Status(http.StatusInternalServerError)
		return
	}

	key := c.Param("key")
	if !keyPattern.MatchString(key) {
		c.Header("Cache-Control", "no-store")
		c.Status(http.StatusNotFound)
		return
	}

	base := path.Base(key)
	if contentType, ok := inlineTypes[strings.ToLower(filepath.Ext(base))]; ok {
		c.Header("Content-Type", contentType)
	} else {
		c.Header("Content-Type", "application/octet-stream")
		c.Header("Content-Disposition", `attachment; filename="`+base+`"`)
	}
	fileServer.ServeHTTP(notFoundNoStore{c.Writer}, c.Request)
}

engine.GET("/storage/*key", handler)
engine.HEAD("/storage/*key", handler)
```

要点：

- **`notFoundNoStore` 是给 `FileServer` 自己写的 404 兜缓存用的**：文件不存在时是 `FileServer` 自己 `WriteHeader(404)`，我们在包装器里把它刚设好的 `Cache-Control` 改成 `no-store`。这样不用额外 `os.Stat`（多一次系统调用，且 stat 与 open 之间有竞态）。
- 形状非法的 404 是我们自己写的，直接在分支里覆盖 `Cache-Control`。
- 包装器只覆盖 `WriteHeader`；`Header()` 与 `Write()` 靠内嵌的 `http.ResponseWriter` 透传，`FileServer` 只需要这三个方法。
- 包装器**不要**放在 `http.NewResponseController` 之前——清 deadline 用的是 gin 的原始 writer。
- 不用 `engine.Static`：它在目录无 `index.html` 时会输出文件清单，也给不了 D15 的响应头策略。
- 预先设置 `Content-Type` 后，`http.ServeContent` 会尊重它、不再按扩展名或内容嗅探改写（多段 Range 除外，见 §6.3）。
- 内联表以外的类型（`.html`、`.svg`、`.js`、`.xml`、`.pdf`、`.zip`、无扩展名……）一律下载，浏览器不会把它们当本页脚本执行。
- 清掉写 deadline 意味着慢速客户端可以一直占着连接；个人博客量级接受，不额外加并发连接限制。
- 这条路由注册在 `engine` 上（不是 `api` 分组），仍处在 `sessions.LoadAndSave` 与 `trackWrites` 的包装内（`backend/internal/app/app.go:113`）。匿名取图没有会话 cookie，scs 不碰数据库；带 cookie 的请求见 D3 的适用范围。
- `HandleMethodNotAllowed` 生效：`POST /storage/x` → 405 + `Allow: GET, HEAD`。

## 8. 上传流程

1. 清掉读 deadline：`http.NewResponseController(ctx.Writer).SetReadDeadline(time.Time{})`；返回错误 → 500 `internal_error`，不继续。
2. `ctx.Request.MultipartReader()`；失败 → 400 `invalid_request`。**不用** `FormFile`（它会把整包落到系统临时目录，等于写两遍盘）。
3. 遍历 part，取第一个 `FormName() == "file"` 的；在它之前的 part 读完丢弃（`file` 之后的 part 不再读取，避免为复用连接读完超大 body）。找不到 → 400 `invalid_request`。
4. 原始名校验：`part.FileName()` 为空则允许（存空串）；含 NUL 或不是合法 UTF-8 → 400 `invalid_request`。
5. 扩展名归一：`strings.ToLower(filepath.Ext(name))` → 去前导点 → 必须匹配 `^[a-z0-9]{1,16}$`，否则用 `bin`。
6. `now := time.Now().UTC()`；`id := uuid.NewV7()`；`key := now.Format("2006/01/02") + "/" + id + "." + ext`。
7. `size, err := provider.Save(ctx, key, part)`；失败 → 500。
8. `policy.Check(size)`；失败 → `provider.Delete(key)`（补偿）后报错。本轮 `allowAll` 永不触发。
9. 插库（显式 `SetCreatedAt(now)`）；失败 → `provider.Delete(key)`（补偿）后按 §6.4 返回 503 或 500。补偿删除再失败只记日志，原始错误照常返回。
10. 201 + 裸对象。

失败路径的口径：**任何一步都不静默降级**。文件写成功而库没写进去，必须删文件；反过来不存在。不对内容做任何读取、嗅探或解码：流式从 part 复制到磁盘。

## 9. 大小限制插槽

```go
type UploadPolicy interface {
    Check(size int64) error
}

type allowAll struct{}

func (allowAll) Check(int64) error { return nil }
```

- 注入点在 `media.New`，本轮固定 `allowAll`。
- **本轮没有任何拒绝路径**，所以不新增「文件过大」错误码；将来引入具体数值时，同一处提交里加 `MaxBytes` 实现 + 错误码 + 400 映射。
- `docs/DEVLOG.md` 必须记：本轮不限制上传大小与类型是用户 2026-10-27 明确授权，适用范围仅 `POST /api/v1/media`，`data/storage` 所在分区需自行监控剩余空间。

## 10. 路径与启动

媒体根目录**写死在代码里**，不读环境变量、不进 `config.Config`：

```go
const mediaRootPath = "../data/storage"
```

为什么是 `../data/storage`：后端一直从 `backend/` 目录启动（`scripts/dev.sh` 会 `cd backend`，`.env` 也是从那里读的），所以相对工作目录往上一级才是项目根。这是后端既有的运行契约，不是新加的假设。

```
NeoBlog/
  backend/                     ← 进程工作目录（go run . / 二进制）
  frontend/
  data/                        ← 项目数据总目录（整棵不入库）
    storage/                   ← 媒体根目录
      2026/10/27/<uuid>.<ext>
```

- service 构造函数里 `filepath.Abs(root)` 归一，得到开发机上的 `F:\protect\NeoBlog\data\storage`；启动日志打印这个绝对路径。
- 启动时 `MkdirAll` + 写探针（`os.CreateTemp` 建一个临时文件再删，不用固定文件名，避免多进程互删），任一失败 → 启动失败。
- 仓库根 `.gitignore` 加 `/data/`（整棵 `data/` 都不入库）。
- 因为 `data/` 在仓库内：`git clean -xdf` 会连它一起删掉，备份脚本也必须显式包含它。
- 不要放在 `frontend/public/` 或 `backend/bin/`。
- 备份口径：`data/` 必须与数据库一起备份。只有记录没有文件 → 死链；只有文件没有记录 → 孤儿文件。

### 10.1 测试注入

`root` 是 `media.New` 的参数，不是模块内部常量：

- `internal/app/app.go` 里 `const mediaRootPath = "../data/storage"`，`New(cfg, db)` 转调 `newWithRoot(cfg, db, mediaRootPath)`，`newWithRoot` 把 `mediaRoot` 传给 `media.New(db, mediaRoot)`。
- `media` 自己的测试直接 `media.New(db, t.TempDir())`。
- `internal/app/app_test.go` 现有 3 处 `New(cfg, db)` 改成 `newWithRoot(cfg, db, t.TempDir())`。**不改的话，测试会把目录建到 `backend/internal/data/storage`**：`go test` 的工作目录是包目录，`../data/storage` 因此指错地方，既污染仓库又不被根 `.gitignore` 覆盖。

## 11. 权限

```go
r.POST("/media", guard.RequirePresetGroup(rbac.GroupKeyOwner, rbac.GroupKeyAdmin), c.upload)
r.DELETE("/media/:id", guard.RequirePresetGroup(rbac.GroupKeyOwner, rbac.GroupKeyAdmin), c.remove)
```

`protected` 分组已挂 `RequireAuth` + `TrackSessionActivity` + `LoadIdentity`，所以未登录是 401、普通用户是 403，不需要额外代码。

因为校验只剩权限，**这条权限边界就是唯一的防线**：任何能上传的人都能在站点域名下放任意文件。

## 12. 并发与异常口径

| 场景 | 行为 |
| --- | --- |
| 两个 DELETE 同时到达同一个 id | 都读到记录；文件删除方一串行执行，后一个得到 `fs.ErrNotExist` 视为已删；记录删除影响 0 行的一方返回 404 |
| 上传过程中上传者被删除 | 插库外键失败 → 补偿删文件 → 500 `internal_error` |
| 记录在、磁盘文件被人在服务器上手动删掉 | 读 → 404（FileServer）；DELETE → `fs.ErrNotExist` 视为已删，继续删记录 → 200 |
| 记录里的 `provider` 当前未注册 | 读不受影响（静态映射）；DELETE → 500 `internal_error`，不回退 |
| 插库超时、结果未知 | 按错误返回 500/503，并补偿删文件。极端情况下可能删掉一个已提交记录对应的文件（记录仍在、文件没了），下次读取 404 |
| 磁盘写满 | `Save` 失败 → 500，不留下半截文件（临时文件已清理） |

## 13. 测试

集成测试（`media_test.go`，需要 `DATABASE_URL_TEST`；媒体根用 `t.TempDir()`）：

| 用例 | 断言 |
| --- | --- |
| 上传 jpeg | 201；URL 形如 `/storage/YYYY/MM/DD/<uuid>.jpg`；盘上文件存在且字节一致；库里有行且 `size_bytes` 正确；`created_at` 的日期与目录一致 |
| 上传 `.zip` | 201，扩展名保留 `.zip` |
| 上传 `.html`（内容是 `<script>`） | 201；取回时 `Content-Type: application/octet-stream`、`Content-Disposition: attachment`、`X-Content-Type-Options: nosniff` |
| 上传无扩展名的文件 | 201，URL 以 `.bin` 结尾 |
| 上传文件名带大写扩展名（`PHOTO.JPG`） | key 里是小写 `.jpg` |
| 上传文件名含 NUL | 400 `invalid_request`，盘上没有残留 |
| 上传空文件 | 201（不做类型校验），`size_bytes` 为 0 |
| 未登录上传 | 401 |
| 普通用户上传 | 403 |
| 非 multipart 请求 / 缺 `file` 字段 | 400 `invalid_request` |
| 插库失败 | 500，盘上文件已被补偿删除 |
| `GET` 刚上传的 jpg | 200；`Content-Type: image/jpeg`；字节一致；`Cache-Control: public, max-age=31536000, immutable` |
| `HEAD` 同一 URL | 200，无 body，头一致 |
| 带 `Range: bytes=0-9` | 206；`Content-Length: 10`；`Content-Range` 正确 |
| `If-Modified-Since` 命中 | 304 |
| `GET` 不存在的 key（形状合法） | 404，纯文本，`Cache-Control: no-store`，`X-Content-Type-Options: nosniff` |
| `GET /storage/2026/`（形状非法） | 404，空 body，`Cache-Control: no-store`，`X-Content-Type-Options: nosniff` |
| `GET /storage`（无尾斜杠） | 301 到 `/storage/` |
| `POST /storage/x` | 405，`Allow` 含 GET、HEAD |
| 删除 | 200 确认体；行没了；盘上文件没了；再 GET → 404 |
| 删除不存在的 id / 畸形 id | 404 |
| 删除时文件已被外部删除 | 200，记录删掉 |
| 删除用户后 | 该用户的媒体记录 `uploaded_by` 变 NULL，记录本身保留 |

单测：

- `service_test.go`（表驱动）：扩展名归一（`PHOTO.JPG`→`jpg`、`a.tar.gz`→`gz`、`photo.`→`bin`、`中文.png`→`png`、超长扩展名→`bin`、空名→`bin`）；原始名含 NUL/非法 UTF-8 被拒；上传各阶段失败时的补偿调用；**删除时 `provider.Delete` 返回普通错误 → 服务返回错误且记录仍在；返回 `fs.ErrNotExist` → 继续删记录并成功**。
- `local_test.go`：`Save` 自动建多层目录、写失败时临时文件被清掉、`URL` 拼接、`Delete` 不存在的文件返回 `fs.ErrNotExist`。
- `schema_test.go`：表与唯一索引存在、`uploaded_by` 外键 `ON DELETE SET NULL` 实际生效（不存在的 provider 字符串也能插入，证明没做 DB 枚举）。

变异自检（照着改坏代码，对应用例必须变红）：

1. 去掉 `keyPattern` 校验 → 目录路径与非法路径用例红（而且会暴露出文件清单）；
2. 把 `URL` 改成 `"/media/" + key` → URL 形状用例红；
3. 去掉 `RequirePresetGroup` → 普通用户 403 用例红；
4. 把扩展名改成直接拼原始文件名 → 大小写扩展名用例与 key 形状用例红；
5. 删掉 `Content-Disposition` 那一行 → `.html` 下载策略用例红；
6. 删掉 `nosniff` → 对应用例红；
7. 删掉 `notFoundNoStore` 包装器（把 `c.Writer` 直接交给 `ServeHTTP`）→ 两个 404 的 `no-store` 用例红（它们会拿到 `immutable`）；
8. 调换删除顺序（先删记录再删文件）→「文件删除失败时记录仍在」的 service 单测红；
9. 去掉 `keyPattern` 里的扩展名段 → 畸形 key 用例红。

## 14. 文档同步

- `backend/README.md`：模块表、接口表（上传 / 删除 / 静态读三行）、新增「媒体存储」章节（含「`/storage` 的 404 是纯文本或空 body，不是错误信封」「非媒体类型强制下载」「`/storage` 不在 `/api/v1` 下、不返回 JSON」三条）；修正第 170 / 174 行与迁移清单。
- `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md`：§3 端点表两行；§4 规则 1 / 3 / 5 补 `/storage` 例外。
- `docs/superpowers/specs/2026-10-11-neoblog-content-design.md`：四处 `/uploads/...` 示例改 `/storage/...`。
- `docs/DEVLOG.md`：功能条目 + 不限制大小与类型的授权记录（D7）。

## 15. 明确不做

- 类型白名单、内容嗅探、扩展名纠正（用户明确要求）
- 病毒扫描、内容安全检测
- 缩略图、多尺寸、`srcset`
- 内容哈希去重
- 引用检查 / 回收站
- 附件分组、存储策略管理界面
- 列表接口、分页
- 磁盘配额、剩余空间检查、大小上限的具体数值
- 私有附件、签名 URL
- PDF 或其他文档的内联预览（需要时把扩展名加进 `inlineTypes`，但要自行评估浏览器阅读器的风险）
- 前端接线（rewrite、媒体库界面、编辑器）
- 把 `/storage` 移到 session 中间件外层的结构调整
- 并发连接数限制 / 慢速攻击防护

## 16. 已知代价与风险

1. **删文件即死链**：历史快照里的旧 URL 404 且不可恢复。用户已明确接受。
2. **不限大小**：owner/admin 账号被拿去灌文件能写满分区。缓解手段只有权限收窄与自行监控磁盘。
3. **`/storage/` 被本地存储独占**：将来整体迁到 S3 时，老文件仍必须留在本地磁盘（或做重定向），否则老文章全死链。这是 D4 的直接后果，用户已确认。
4. **可以托管任意内容**：站点域名下可能出现任何文件。D15 只保证「不会在浏览器里当本页脚本执行」，不保证内容合法合规。
5. **`data/` 在仓库内**：`git clean -xdf` 会删掉整个 `data/`，备份必须显式包含它。
6. **扩展名以客户端为准**：把一张 PNG 改名成 `.jpg` 上传，浏览器会因为 `nosniff` 拒绝渲染。服务端不纠正、不嗅探。
7. **清掉写 deadline 后公开下载可被慢速占用**：个人博客量级接受，不额外防护。
8. **`/storage` 不是无状态**：带会话 cookie 的请求仍依赖数据库（D3）。数据库宕机时登录用户看到的图会 503。
9. **没有缩略图**：文章列表页在手机上加载原图，流量偏大。以后补 `thumb_key` 列 + 回填脚本即可。
10. **原始文件名不参与下载**：下载文件名恒为 `<uuid>.<ext>`。
11. **Windows 上删不掉正在被读取的文件**：`os.Remove` 会失败，按 D6 返回 500，记录保留，可重试。
