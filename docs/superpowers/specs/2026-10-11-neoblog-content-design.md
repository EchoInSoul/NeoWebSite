# NeoBlog 内容模块设计（文章与自定义页面：存储与 API）

日期：2026-10-11
范围：`backend/`（Go + Gin + ent + Atlas，module 路径 `website/backend`，见 `backend/go.mod:1`）；本轮只覆盖文章（posts）与自定义页面（pages）的存储与 API 设计
状态：设计稿 v2.1，待用户逐项确认；确认后进 writing-plans 出实施计划

修订记录：
- v1（2026-10-11）：首版。双表（posts / pages）、单模块 `internal/modules/content/`、ProseMirror JSON 为唯一权威存储 + 客户端派生缓存列（content_html / content_text）、content_format 字符串标记、公开读 + 管理 CRUD/发布动作端点面、搜索 v1 = ILIKE、图片上传为非目标。
- v1.1（2026-10-11）：并入风险调研子代理的结论（均为注记级补充，不改冻结决策）：§14 增 Tiptap 默认静默剥离与 `enableContentCheck` 前端硬性要求、link 扩展 XSS 前科（CVE-2025-14284）与版本锁、`@tiptap/static-renderer` 现渲兜底；§10 增图片相对路径约定；§7.3 增 HardBreak 提取注记；§16 增 schema 指纹备选。
- v2（2026-10-11，本文）：**用户拍板推翻 v1 的「内容存文章行」方案，改为照抄 HaoBlog（`D:\protect\NextJs_Blog`，`prisma/schema.prisma` 的 Post / PostSnapshot / Page / PageSnapshot 四模型）的快照表 + 双指针设计**。正文四件套（content / content_html / content_text / content_format）移入 post_snapshots / page_snapshots（version 单调递增、行不可变），实体行留 head / release 两个 unique 外键指针与 title / 摘要类去规范化列；新增历史版本 API（快照列表 / 详情 / 恢复，posts 与 pages 同构）；publish 语义改为 release ← head，unpublish 保留 release 指针；乐观锁立场保持「不做」但理由改为「历史快照兜底，覆盖可找回」。受影响决策：D5 / D6 / D7 / D8 / D12 / D13 改写，新增 D16–D20；§2 非目标表更新（版本历史入范围、快照清理 / 版本对比入非目标）；§6 端点面增 6 条快照端点并补 §6.6 对照；§7 写入口径改快照语义；§9 搜索改打实体行 search_text；§12 数据模型重写为四表；§13 / §15 / §16 同步。其余 v1.1 决策（双实体组、单模块、slug 全表唯一、硬删、4 个权限点、错误码仅 slug_taken、图片相对路径、epoch 秒、limit/offset、snake_case）全部保留。
- v2.1（2026-10-11）：按用户拍板调整 slug——**公开详情标识段由 `:slug` 改回 UUID `:id`**（同时消解 v2 对 API 规范 §3 规则 2 的字面扩展，§6.6 相应行改为「符合」）；**slug 列改为可空可选**（创建缺省 / 空串落 NULL；提供时全表唯一 + 格式校验；Postgres 唯一索引对多个 NULL 天然放行，无需部分索引）；「/posts/hello-world」形式的 slug 公开网址整体延后（§16 第 7 条插槽）。端点面、响应形状（slug 可为 null）、写入口径、验收同步；其余决策不动。
- v2.2（2026-10-11）：事实修正两处——`internal/common/apierror` 实际已落地（哨兵 + 注册表 + Write/Abort），§6.5 时序注记与 §16 第 1 条改为「仅新增 slug_taken 登记」；`scripts/db-migrate-diff.sh` 实际仍硬编码迁移名，§13 第 1 条改为「先参数化再使用」。

## 1. 意图与背景

NeoBlog 是前后端分离的个人博客，本轮给后端加「内容」能力：站长（与管理员）能写文章、建自定义页面（如「关于」），内容先存草稿、编辑、发布、下线、删除；访客无需登录即可读已发布内容。

前端编辑器已定为 Tiptap，序列化格式为 ProseMirror JSON（`editor.getJSON()` 的输出）。本轮的存储基线（用户已拍板，本文只细化不推翻）：

- 后端以 ProseMirror JSON 为**唯一权威内容**，存**快照表**的 jsonb 列（v2 起，快照化）；
- `content_html`（供 RSS / SEO / 邮件）与 `content_text`（供摘要派生与搜索源）是**前端保存时随 JSON 一并提交的派生缓存**，只读缓存，永不回灌编辑器；
- 后端不引入 Tiptap 依赖、不做 JSON→HTML 转换，html/text 视为客户端派生的缓存数据；
- `content_format` 版本标记列随快照走（形态为字符串，见 D7）；
- **快照表 + 双指针**（v2，照 HaoBlog）：每次带内容的保存追加一条不可变快照，实体行用 `head_snapshot_id`（最新编辑版）与 `release_snapshot_id`（当前发布版）两个指针引用快照——历史天然保留，「改砸了随时翻回去」。

对齐来源（不重新发明）：

- `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md`（冻结的 API 规范，下称「API 规范」，引用按其章节号）——本文所有端点逐条对齐，对照见 §6.6；
- `docs/superpowers/specs/2026-10-11-neoblog-data-layer-design.md`（ent + Atlas 纪律：版本化迁移进 Git、应用启动绝不自动迁移）；
- `docs/superpowers/specs/2026-10-11-neoblog-access-design.md`（模块分层先例、权限目录纪律 §7「以后 `content.posts.edit` 等」的命名预留）；
- **参考实现 HaoBlog**：`D:\protect\NextJs_Blog\prisma\schema.prisma`——`Post`（:234-265）、`PostSnapshot`（:267-285）、`Page`（:404-419）、`PageSnapshot`（:421-438）。本轮只抄快照 + 双指针骨架（快照字段、`@@unique([postId, version])`、head / release 双 unique FK、级联删除）；HaoBlog 另有的 accessLevel / 加密 / scheduledPublishAt / 软删 / abbrlink slug 生成**一律不抄**（与 NeoBlog 无关）；
- 现有代码：`internal/modules/auth/` 与 `internal/modules/access/` 的分层与 ent schema 写法；`internal/common/guard` 守卫链（`backend/internal/app/app.go:60`）。

## 2. 范围与非目标

本轮做：

- ent schema 新增 `Post` / `PostSnapshot` / `Page` / `PageSnapshot` **四张表** + 一份 Atlas 版本化迁移；
- 新模块 `internal/modules/content/`：公开读端点（文章/页面的已发布列表与 slug 详情）+ 管理端点（CRUD、发布/下线动作）+ **历史版本端点（快照列表 / 详情 / 恢复）**；
- `internal/common/rbac` 权限目录新增 4 个权限点（§5）；
- 错误码新增 `slug_taken` 并登记到 `internal/common/apierror` 注册表（§6.5，该包按 API 规范 §7.4 落地）；
- 搜索的 v1 务实口径（§9）；图片/资源的 v1 口径（§10）；
- 测试（§13）、README 更新、版本进位 minor（0.6.0 起，执行时按当前版本顺延）。

本轮不做（非目标，逐条给出立场）：

| # | 非目标 | 立场 |
| --- | --- | --- |
| 1 | 标签 / 分类 | 明确不做；将来加表即可，不影响本轮表结构 |
| 2 | 评论 | 明确不做；将来独立设计 |
| 3 | 协同编辑、草稿自动保存 | 明确不做；单人博客无需求（版本历史已在范围内，不在此列） |
| 4 | 媒体上传 / 图片处理端点 | 明确不做（D11）；正文图片以相对路径 URL 内嵌于快照 JSON；`cover_image_url` 只是一列，不是上传 |
| 5 | 中文全文检索（tsvector / zhparser / pg_jieba）与 pg_trgm 索引 | 明确不做，列为后续演进（§9） |
| 6 | RSS / sitemap / SEO 专用端点 | 不做；快照的 content_html 列仅为它们备料，输出端点将来单独立项（RSS 为 XML，不在 API 规范的 JSON 词汇内） |
| 7 | 定时发布（scheduled 状态）、归档/按月归档端点 | 不做（HaoBlog 的 scheduledPublishAt 不抄）；status 只有 draft / published 两态 |
| 8 | 多作者 / 作者转移、代发 | 不做；`author_id` 创建时取操作者，之后不可改 |
| 9 | 页面树 / 导航排序（sort_order） | 不做，留插槽（§16） |
| 10 | 软删 / 回收站 | 不做（D3；HaoBlog 的 deletedAt 不抄） |
| 11 | 访问计数、点赞、阅读量 | 不做 |
| 12 | HTTP 缓存头 / ETag / CDN 策略 | 不做；将来属 v1 内向后兼容增量 |
| 13 | slug 变更后的 301 重定向表 | 不做（风险注记见 §14） |
| 14 | **快照清理 / 数量上限 / 存储压缩** | 不做（D19：v1 永久保留全部版本，个人博客量级） |
| 15 | **版本对比 diff** | 不做；快照 API 出「任意两版原文」已够，diff 属前端事务 |
| 16 | **excerpt 手工撰写 / 封面独立图床管理** | 不做；excerpt 恒为服务端派生（D12），手工摘要留插槽（§16） |
| 17 | **slug 形式的公开网址（`/posts/hello-world`）** | 暂不做（用户拍板 v2.1）：slug 列保留但可空、暂不填；公开详情走 UUID（D9）。将来启用时属 v1 内增量（§16 第 7 条） |
| 18 | 前端实现 | 不归本轮（前端 Next.js 在别处推进） |

## 3. 决策记录（总表）

| # | 决策 | 状态 |
| --- | --- | --- |
| D1 | posts 与 pages 为两套**独立实体组**（各含实体表 + 快照表，共四张），不用单表加 type 判别列 | 冻结（v2 改写：两表 → 四表） |
| D2 | 单一模块 `internal/modules/content/` 承载两实体组（对齐 access 模块同时承载组与用户管理的先例） | 冻结 |
| D3 | **硬删**，无软删列、无 deleted_at（对齐 auth / access 既有做法；HaoBlog 的软删不抄）；删实体行 → 快照级联删 | 冻结 |
| D4 | slug **可空可选**（v2.1，用户拍板暂不启用）：创建缺省 / 空串落 NULL；**提供时**全表唯一（不区分草稿/已发布状态），冲突 409 `slug_taken`；Postgres 唯一索引对多个 NULL 天然放行、无需部分索引；PATCH 可改可清（HaoBlog「创建后冻结不改」的做法已评估不采纳，对比见 §12.2） | 冻结（v2.1 改写） |
| D5 | ProseMirror JSON 是唯一权威内容，存**快照表** jsonb 列；实体行经 head / release 指针引用快照，正文不落实体行；后端**不做 schema 级校验**，渲染安全由前端 Tiptap schema 负责 | 冻结（v2 改写） |
| D6 | `content` / `content_html` / `content_text` **三列成组提交**并**落为新快照**（快照行不可变）；html/text 是客户端派生缓存，服务端不校验其与 content 的一致性；实体行上的 `excerpt` / `search_text` 是服务端从快照 content_text 派生的二级缓存，保存同事务刷新 | 冻结（v2 改写） |
| D7 | `content_format` 为**字符串**标记列（非整数），v1 唯一取值 `pm1`；演进规则「只标记不迁移」；列随快照走（每个版本自带方言标记） | 冻结（v2 改写：移入快照表；首值定名见 §16 待定项） |
| D8 | 状态机 `draft` / `published` 不变；发布/下线走**动作端点**（POST 子动作），PATCH 不收 status 不变；publish 幂等语义 v2 细化为「实际变更才刷新 published_at、纯重放不变」（D17） | 冻结（v2 微调） |
| D9 | 公开读：`GET /api/v1/posts`、`GET /api/v1/posts/:id`、`GET /api/v1/pages`、`GET /api/v1/pages/:id`（v2.1：标识段为 UUID，slug 网址延后）；管理面：`/api/v1/admin/posts` 与 `/api/v1/admin/pages` 的 CRUD + 发布动作 + **快照三端点** | 冻结（v2 扩充、v2.1 改写；位形说明见 §6.6） |
| D10 | 搜索 v1 = `q` 参数对 title / **search_text（实体行去规范化纯文本列）** 做 ILIKE 包含匹配；全文检索为非目标 | 冻结（v2 改写：搜索源落实体行） |
| D11 | 图片以 URL 内嵌于正文 JSON（存于快照）；上传端点为非目标 | 冻结 |
| D12 | 列表响应**排除大列且只读实体行、不 join 快照**；摘要字段 `excerpt` 为**实体行去规范化列**：服务端从当次快照 content_text 截取前 200 个 Unicode 字符、保存时同步刷新 | 冻结（v2 改写：excerpt 从「不入库纯派生」改为「入库去规范化列」） |
| D13 | 并发写**后写者获胜**，不做乐观锁 / 版本比对——**理由（v2 改写）：历史快照兜底，误覆盖可从旧版本恢复**；快照 version 分配用事务内 `FOR UPDATE` 锁实体行线性化（对齐 access 的 ForUpdate 先例，是实现机制而非乐观锁） | 冻结（v2 改写理由） |
| D14 | 权限点新增 4 个：`content.posts.view` / `content.posts.edit` / `content.pages.view` / `content.pages.edit`；快照端点映射进这 4 个点，**不新增 key** | 冻结 |
| D15 | 错误码仅新增 `slug_taken`（409 / `conflict_error` / param=`slug`），需在 apierror 注册表登记 | 冻结（登记动作依赖 apierror 包落地，时序见 §16） |
| D16 | **快照表 + 双指针模型**（照 HaoBlog）：快照行不可变（无 UPDATE 路径、无 updated_at）；version 每实体从 1 单调递增、`@@unique([实体id, version])`；恢复 = 把旧版内容**复制为新快照**、head 前移，历史线性追加、不销毁任何版本；pages 与 posts **完全同构**（含 head 指针——偏离 HaoBlog Page 仅 release 的做法，理由见 §12.3） | 冻结（v2 新增） |
| D17 | **发布语义 v2**：publish = release ← 当前 head + status=published，published_at 仅在实际变更（status 翻转或 release 指针移动）时刷新；unpublish = status=draft，**release 指针保留**（记录「上次发布版」），公开可见性只由 status 把关 | 冻结（v2 新增） |
| D18 | **历史版本 API**：快照列表（分页、不含大列）+ 快照详情（全字段）+ 恢复动作，posts 与 pages 同构各三条 | 冻结（v2 新增） |
| D19 | 快照**永久保留**（个人博客量级）；清理 / 数量上限 / 存储压缩 / 版本对比 diff 均非目标 | 冻结（v2 新增） |
| D20 | **读路径分配**：公开列表读实体行（不 join 快照）；公开详情读 **release** 快照内容；管理详情读 **head** 快照内容；快照端点按 :version 直读 | 冻结（v2 新增） |

## 4. 名词与模型

- **文章（post）**：博客正文，实体行存标题、slug、封面、状态、作者与两个快照指针；有草稿/已发布两态与发布时间。
- **页面（page）**：与文章同构的静态页（如「关于」「友链」），独立实体组、独立 URL 面（`/pages/...`）与独立权限点。
- **快照（snapshot）**：一次带内容的保存所产生的不可变行——ProseMirror JSON（`content`）+ 客户端派生的 `content_html` / `content_text` + 方言标记 `content_format`，带每实体单调递增的 `version`。**权威内容只存在于快照里**，实体行只有指针。
- **head**：最新编辑版快照；带内容组的保存与恢复都使 head 前移（指向新快照）。管理详情读 head。
- **release**：当前发布版快照；publish 使 release ← head。公开详情读 release。下线保留 release 指针（D17）。
- **excerpt**：实体行去规范化列，服务端从当次 head 快照的 content_text 截前 200 个 Unicode 字符（D12），列表卡直接用、免 join。
- **search_text**：实体行去规范化列，服务端从当次 head 快照的 content_text 全量复制，仅供 `q` 的 ILIKE（D10）；不出现在任何响应里。
- **发布（publish）**：release ← head + status=published；**下线（unpublish）**：status=draft，release 指针保留。published_at 记录「最近一次实际生效的发布时刻」（D17）。

## 5. 权限点

`internal/common/rbac` 目录新增 4 个点（命名对齐 access 设计 §7 的 `模块.动作` 纪律与该节预留的 `content.posts.edit` 形态；目录只增不改语义，每个 key 都有真实检查点）：

| key | 展示名 | 真实检查点 |
| --- | --- | --- |
| `content.posts.view` | 查看文章（后台） | `GET /admin/posts`、`GET /admin/posts/:id`、`GET /admin/posts/:id/snapshots`、`GET /admin/posts/:id/snapshots/:version` |
| `content.posts.edit` | 管理文章 | `POST` / `PATCH` / `DELETE /admin/posts/:id`、`publish` / `unpublish`、`POST /admin/posts/:id/snapshots/:version/restore` |
| `content.pages.view` | 查看页面（后台） | `GET /admin/pages`、`GET /admin/pages/:id`、`GET /admin/pages/:id/snapshots`、`GET /admin/pages/:id/snapshots/:version` |
| `content.pages.edit` | 管理页面 | `POST` / `PATCH` / `DELETE /admin/pages/:id`、`publish` / `unpublish`、`POST /admin/pages/:id/snapshots/:version/restore` |

- 蕴含规则 `edit → view` 已在目录定义（`backend/internal/common/rbac/rbac.go:28-30`），`content.posts.edit` 持有者自动满足 `content.posts.view` 检查点，无需重复授予。
- 公开读端点（`/posts`、`/pages`）不挂任何权限点，未登录可读；守卫链仍是现有 `RequireAuth → LoadIdentity`（`backend/internal/app/app.go:60`），管理路由逐条挂 `RequirePermission`（对齐 `backend/internal/modules/access/controller.go:46-53` 的挂法）。
- 快照端点**不新增权限 key**（D14）：读快照映射 `*.view`、恢复映射 `*.edit`——能改文章的人天然能翻并恢复它的历史，与「恢复产生一条新快照 = 一次编辑」的语义一致。

## 6. API 设计

### 6.1 端点面总表

公开（无守卫，只出已发布）：

| # | 方法 路径 | 说明 | 成功 |
| --- | --- | --- | --- |
| 1 | `GET /api/v1/posts` | 已发布文章列表（支持 `limit` / `offset` / `q`；只读实体行） | 200 列表信封 |
| 2 | `GET /api/v1/posts/:id` | 已发布文章详情（按 UUID；内容读 release 快照） | 200 详情对象 |
| 3 | `GET /api/v1/pages` | 已发布页面列表（支持 `limit` / `offset`；只读实体行） | 200 列表信封 |
| 4 | `GET /api/v1/pages/:id` | 已发布页面详情（按 UUID；内容读 release 快照） | 200 详情对象 |

管理（挂在 protected 组，逐条 `RequirePermission`）：

| # | 方法 路径 | 权限 | 说明 | 成功 |
| --- | --- | --- | --- | --- |
| 5 | `GET /api/v1/admin/posts` | `content.posts.view` | 全状态列表（支持 `limit` / `offset` / `status` / `q`；只读实体行） | 200 列表信封 |
| 6 | `POST /api/v1/admin/posts` | `content.posts.edit` | 创建（恒为草稿；同事务落快照 v1，head 指向它） | 201 详情对象 |
| 7 | `GET /api/v1/admin/posts/:id` | `content.posts.view` | 详情（全字段；内容读 head 快照） | 200 详情对象 |
| 8 | `PATCH /api/v1/admin/posts/:id` | `content.posts.edit` | 部分更新（带内容组 → 追加新快照、head 前移） | 200 详情对象 |
| 9 | `DELETE /api/v1/admin/posts/:id` | `content.posts.edit` | 硬删（快照级联删） | 200 确认体 `{"id":"…","deleted":true}` |
| 10 | `POST /api/v1/admin/posts/:id/publish` | `content.posts.edit` | 发布（release ← head；幂等） | 200 详情对象 |
| 11 | `POST /api/v1/admin/posts/:id/unpublish` | `content.posts.edit` | 下线（status=draft；release 保留；幂等） | 200 详情对象 |
| 12 | `GET /api/v1/admin/posts/:id/snapshots` | `content.posts.view` | 快照列表（分页；不含大列） | 200 列表信封 |
| 13 | `GET /api/v1/admin/posts/:id/snapshots/:version` | `content.posts.view` | 快照详情（全字段） | 200 快照对象 |
| 14 | `POST /api/v1/admin/posts/:id/snapshots/:version/restore` | `content.posts.edit` | 恢复（旧版复制为新快照、head 前移） | 200 新快照对象 |
| 15–24 | `/api/v1/admin/pages` 同构 10 条 | `content.pages.view` / `content.pages.edit` | 与 #5–#14 同构 | 同上 |

### 6.2 分页、排序与过滤参数

对齐 API 规范 §9（D5）：`limit` 默认 20、上限 100 超限钳制、`offset` 默认 0；非法值（负数、非整数、溢出）→ 400 `invalid_request`，`param` 指向 `limit` / `offset`；`total` 必返；信封恒为 `{"data":[…],"total":n,"limit":n,"offset":n}` 四字段；空列表 `"data": []` + `"total": 0`。

- 默认排序：`created_at` 倒序（公开与管理一致，API 规范 §9 规则 6；本轮不提供排序参数）。实现注记：为排序稳定性，建议次级键 `id` 倒序（同一秒内多篇时顺序确定），落地计划确认。
- 快照列表（#12）默认排序同为 `created_at` 倒序——快照的 version 单调递增，等价于 version 倒序（新版本在前）；稳定无歧义。
- `status`（仅实体列表端点，可选）：`draft` | `published`，其他值 → 400 `invalid_request` param=`status`；缺省 = 不过滤（全状态）。
- `q`（可选，§9 搜索口径，仅 posts 列表）：空串 = 不过滤；超过 100 字符 → 400 `invalid_request` param=`q`。
- 路径参数 `:version`：非整数 → 400 `invalid_request` param=`version`（对齐 API 规范 §14 决策 4「格式非法归 400」）；整数但不存在或越界 → 404 `not_found`。

### 6.3 响应形状

字段一律 snake_case（API 规范 §5 D2）；时间戳 epoch 秒整数、未发生为 `null`（§10 D7）；ID 为 UUIDv7 字符串（§10 D8）；单资源裸对象、列表四字段信封（§6 D4）；布尔用 `is_` 前缀（§5 规则 3）。

**实体列表项**（公开与管理一致，管理端每项多一个 `status`；**只读实体行、不含任何快照大列**——D12/D20）：

```json
{
  "id": "01936b2f-7a1e-7d5a-9f3b-2c8e4a6d1b90",
  "title": "第一篇文章",
  "slug": "hello-world",
  "status": "published",
  "excerpt": "这是 head 快照 content_text 的前 200 个字符…",
  "cover_image_url": "/uploads/2026/10/cover.webp",
  "author": { "id": "0193…", "nickname": "owner" },
  "published_at": 1760123456,
  "created_at": 1760123400,
  "updated_at": 1760123456
}
```

**实体详情对象**（管理端点与创建/更新/发布动作的响应；内容字段来自 head 快照，管理端由服务端 join 取出；公开详情同形但**无 `status` / `head_version` / `release_version` 字段**，公开类型是管理类型的子集）：

```json
{
  "id": "01936b2f-7a1e-7d5a-9f3b-2c8e4a6d1b90",
  "title": "第一篇文章",
  "slug": "hello-world",
  "status": "published",
  "content": { "type": "doc", "content": [ … ] },
  "content_html": "<p>…</p>",
  "content_text": "…",
  "content_format": "pm1",
  "cover_image_url": "/uploads/2026/10/cover.webp",
  "head_version": 3,
  "release_version": 2,
  "author": { "id": "0193…", "nickname": "owner" },
  "published_at": 1760123456,
  "created_at": 1760123400,
  "updated_at": 1760123456
}
```

**快照列表项**（#12；不含 content / content_html / content_text 大列）：

```json
{
  "version": 3,
  "content_format": "pm1",
  "is_head": true,
  "is_release": false,
  "created_at": 1760123500
}
```

**快照详情对象**（#13；全字段）与**恢复响应**（#14；复制出的新快照）：

```json
{
  "version": 2,
  "content": { "type": "doc", "content": [ … ] },
  "content_html": "<p>…</p>",
  "content_text": "…",
  "content_format": "pm1",
  "is_head": false,
  "is_release": true,
  "created_at": 1760123456
}
```

形状口径：

- `author` 输出嵌套对象 `{id, nickname}`（对齐 access 模块 `GroupRef` 嵌套先例，`backend/internal/modules/access/dto.go:3-7`）；不输出 `author_id` 冗余字段，**不输出 email**（公开端点防止邮箱泄漏）。
- `head_version` / `release_version`（仅管理详情）：head / release 指针当前指向的版本号，`release_version` 为 `null` 表示从未发布过。快照列表项的 `is_head` / `is_release` 是同一信息的标记形态，供前端渲染「当前版 / 发布版」角标。
- 公开详情不输出 `status` / `head_version` / `release_version`：公开端点只会返回已发布内容，status 恒定无信息量；版本号是后台管理概念，不对访客暴露。
- `slug` 可为 `null`（v2.1：slug 可空可选，用户暂不启用）；公开详情按 UUID 定位，slug 不参与公开路由。
- `content` / `content_html` / `content_text` / `content_format` 在实体详情里来自 head（管理）或 release（公开）快照，服务端原样回显、不重排不补字段；在快照端点里来自该 version 的行。
- `published_at` 为 `null` 表示从未发布过（与 `release_version` 为 null 同义）。
- 实体详情的响应字段与 v1 同形——快照化对 API 消费方透明（内容字段像列一样出现，来源是 head/release 快照这一事实不出现在响应里）。

### 6.4 状态机与发布语义（v2）

- 创建恒为草稿（请求不收 `status`）；创建 = 同事务「插实体行 → 插快照 v1 → head 指向 v1」，`release_version` 为 null、`published_at` 为 null。发布/下线只能经动作端点，PATCH 不收 `status` / `published_at` / `author` / `head_version` / `release_version`（服务端管辖，D8）。
- `publish`（D17）：事务内 release ← 当前 head + status ← published。published_at 仅在**实际变更**时刷新：status 从 draft 翻转为 published，或 release 指针实际移动（已发布状态下编辑后重新发布）；**纯重放**（status 已 published 且 release 已等于 head）→ 200、对象完全不变、published_at 不刷新，不报 409。该语义保证「已发布文章继续编辑 → 再点发布即上线新版」的最短路径。
- `unpublish`：status ← draft；**release 指针保留**（D17，取舍：保留则「上次发布的是什么」可追溯、再发布无需重选版本、与 published_at「最近一次发布」语义一致；清空方案被否决——它会丢失追溯且让「再发布」多一步版本选择）。公开可见性只由 status 把关，与指针无关。
- 多次「下线再发布」：status 翻转即实际变更 → published_at 刷新。
- 动作端点作用于不存在的资源 → 404 `not_found`。
- 删除不区分状态（草稿、已发布都可硬删），实体行删除 → 快照级联删（D3）。

### 6.5 错误码

对齐 API 规范 §7（D3）：body 统为 `{"error":{"type","message","code","param"}}`，type 由状态码唯一决定，`message` 中文仅供人读。content 模块只**新增一个 code**，其余全部复用既有词汇：

| code | HTTP | type | param | 场景 | 状态 |
| --- | --- | --- | --- | --- | --- |
| `invalid_request` | 400 | `invalid_request_error` | 按字段定位 | 请求体形状 / 字段非法（title、slug 格式、内容三列成组缺失、超限、`status` / `q` / `limit` / `offset` 参数非法、`:id` / `:version` 路径参数格式非法、全缺省 PATCH） | 复用（API 规范 §7.3） |
| `unauthorized` | 401 | `authentication_error` | null | 管理端点未登录（guard 写出） | 复用 |
| `forbidden` | 403 | `permission_error` | null | 缺少所需权限点（guard 写出） | 复用 |
| `not_found` | 404 | `not_found_error` | null | 资源不存在；公开端点命中未发布内容（与不存在**不可区分**，防探测）；快照 version 不存在/越界 | 复用 |
| `slug_taken` | 409 | `conflict_error` | `slug` | slug 与同表既有行冲突 | **新增，需在 apierror 注册表登记** |
| `internal_error` | 500 | `server_error` | null | 兜底 | 复用 |

- 新增 code 走注册表登记（HTTP + type + 默认 param + 默认 message），遵守「同一 code 不得跨 type」与「只增不改义」（API 规范 §7.3 规则 2）。建议默认 message：「该地址标识已被使用」。
- 路径参数 `:id` 格式非法（非 UUID）→ 400 `invalid_request` param=`id`，不是 404（API 规范 §14 决策 4）；`:version` 同理归 400 param=`version`。
- slug 冲突的判定来源是唯一索引：`ent.IsConstraintError` → `slug_taken`（对齐 `group_name_taken` 的处理先例，`backend/internal/modules/access/repository.go:173-179`）；posts 表除主键与两个指针 unique 外只有 slug 一个业务唯一约束，判定无歧义。
- 快照 version 由服务端单调分配，请求永不提交 version，因此不存在「版本号冲突」类错误码。
- **apierror 已落地（v2.2 修正）**：`internal/common/apierror` 已存在（哨兵 + `statusByCode` 注册表 + `Write` / `Abort` / `WithParam`，`backend/internal/common/apierror/apierror.go`），本轮只需新增 `slug_taken` 一个哨兵与一行注册。与冻结规范的两处既有差距属全库规范化批次、不在本轮：错误体尚无 `message` 字段（现形为 `{type, code, param}`）；access 等 controller 尚未收敛到 apierror。content 直接采用现状形态，不新写本地 switch。

### 6.6 与冻结规范（API 规范）的逐条对照

| 规范条款 | 本文落法 |
| --- | --- |
| §2 D1：全部端点挂 `/api/v1`，无 v2 | 全部 24 条端点挂 `/api/v1`；前缀沿用 `app.go` 常量（`backend/internal/app/app.go:26`） |
| §3 规则 2：集合名词复数、资源标识段是 UUID 字符串（`:id`） | 复数资源 `/posts`、`/pages` ✓；管理与公开详情的实体标识段一律 UUID `:id` ✓（v2.1 起公开详情改回 UUID，用户拍板 slug 暂不启用；slug 仅作可选 SEO 列保留，slug 网址延后，见 §2 非目标 17 与 §16 第 7 条） |
| §3 规则 3：子动作 `POST /<集合>/:id/<动作>` | `POST /admin/posts/:id/publish` / `unpublish` ✓（正是规范 §3 正例给的形态） |
| §3 规则 4：嵌套从属资源 `/<父集合>/:id/<子集合>` | `GET /admin/posts/:id/snapshots`、`GET /admin/posts/:id/snapshots/:version` ✓（v2 新增，规范明确允许的形态） |
| §3 规则 3+4 组合：嵌套子动作 | `POST /admin/posts/:id/snapshots/:version/restore`——规范无「嵌套资源上的子动作」现成先例，本形态为规则 3（子动作动词端点）与规则 4（嵌套从属资源）的自然组合，主动披露；备选形态是 `POST /admin/posts/:id/restore` + body 带 version，不如现形态直观 |
| §4 D6：创建 201、动作 200 带负载、DELETE 200 确认体、无 204 | #6 → 201；#10/#11/#14 → 200 带负载；#9 → 200 `{"id","deleted":true}` ✓ |
| §5 D2：snake_case、PATCH 指针可空、至少一字段 | §7 写入口径按此设计；全缺省 PATCH → 400 ✓；`is_head` / `is_release` 合 `is_` 前缀 ✓ |
| §6 D4：裸对象 + 四字段信封，无包装 | §6.3 ✓（实体列表与快照列表同用四字段信封） |
| §7 D3：错误信封 + 注册表 | §6.5 ✓（`:version` 格式非法归 400 param=`version`，§14 决策 4） |
| §8 D9：session cookie + guard 链 | 沿用现有链，不新增中间件形态 ✓ |
| §9 D5：limit/offset/total、默认 created_at 倒序 | §6.2 ✓（快照列表默认排序同为 created_at 倒序，与 version 倒序等价） |
| §10 D7/D8：epoch 秒、UUIDv7、无 object 字段、无 ID 前缀 | §6.3 ✓ |
| §11 D10 裁剪项 | 未引入限流头、幂等键、SSE、object、前缀、游标分页、Bearer 中任何一项 ✓ |

三处**位形说明**（非规则冲突，主动披露）：

1. **管理面前缀 `/admin/*`**：API 规范 §3 的正例把「创建文章」画在 `POST /api/v1/posts` 下；本文按 `/api/v1/admin/posts` 落位。理由：规范正文没有任何规则规定管理面前缀，而现状端点全集的管理面全部在 `/admin/*`（API 规范 §3 现状表 #7–#14、access 设计 §9），公开读与特权写分离也使「同名集合的公开面 / 管理面」语义清晰。正例展示的是 URL 形态（复数 + 子动作），本文在 `/admin/posts` 上完整保留了该形态。
2. **可空标量的「显式清空」**：API 规范 §5 只规定了集合字段用 `[]` 清空、标量 `null` = 不修改，未规定标量如何清空。本文取 `cover_image_url: ""`（空串）= 清空为 NULL，缺省 / `null` = 不修改——与集合空数组清空同一精神；v2.1 起 `slug` 同口径。如规范所有者另有裁决，随规范化批次调整。

## 7. 写入口径

### 7.1 请求契约

创建（`POST /admin/posts`，201）：

```json
{
  "title": "第一篇文章",
  "cover_image_url": "/uploads/2026/10/cover.webp",
  "content": { "type": "doc", "content": [ … ] },
  "content_html": "<p>…</p>",
  "content_text": "…",
  "content_format": "pm1"
}
```

- `title` / `content` / `content_html` / `content_text` 必填；`slug` 可选（v2.1：缺省或空串落 NULL，暂不启用）；`cover_image_url` 可选（可空列）；`content_format` 可选，缺省落 `pm1`（v1 唯一方言）。
- 创建 = 同事务「插实体行（status=draft）→ 插快照 v1（四件套入行）→ head 指向 v1 → 派生列（excerpt / search_text）从 v1 刷新」；201 响应 `head_version=1`、`release_version=null`。
- 请求不收 `status`（恒落草稿）、不收 `author` / `author_id`（服务端从 guard 身份取操作者）、不收任何时间字段、不收 `version` / `head_version` / `release_version`（服务端单调分配，API 规范 §10 规则 3）。

部分更新（`PATCH /admin/posts/:id`，200）：全部字段指针可空、缺省与 `null` 同义为「不修改」（API 规范 §5 PATCH 规则）；至少给一个字段，否则 400。例外：`cover_image_url` 与 `slug` 的**空串** = 清空为 NULL（§6.6 位形说明 2）。

**内容三列成组规则（D6）**：`content` / `content_html` / `content_text` 视为一组——PATCH 中任一出现，三者必须同时出现，缺哪个 `param` 指向哪个（400 `invalid_request`）。带内容组的 PATCH = **追加新快照**：事务内锁实体行 → version = 当前最大 + 1 → 插入新快照行（四件套入行，`content_format` 缺省继承 head 快照的值）→ head 前移 → 派生列从新快照刷新。**只改 title / slug / cover_image_url 的 PATCH 不产生新快照**（元数据不随版本走，HaoBlog 同款）。`content_format` 只允许伴随内容组出现（单独出现 → 400 param=`content_format`）。

### 7.2 服务端校验边界

**能校验的（粗校验，全部服务端强制）**：

| 项 | 规则 | 违例 |
| --- | --- | --- |
| title | trim 后 1–200 个 Unicode 字符（拒绝纯空白） | 400 param=`title` |
| slug | 仅在提供时校验：1–120 字符，`^[a-z0-9]+(-[a-z0-9]+)*$`（小写字母数字与连字符，不以外连字符开头/结尾）；缺省 / 空串 = NULL | 400 param=`slug` |
| cover_image_url | 非空时 ≤ 2048 字符；相对路径约定见 §10 | 400 param=`cover_image_url` |
| content | 合法 JSON **对象**（非数组/标量）；序列化字节 ≤ 256 KB；嵌套深度 ≤ 64 层 | 400 param=`content` |
| content_html | 字节 ≤ 1 MB | 400 param=`content_html` |
| content_text | 字节 ≤ 256 KB | 400 param=`content_text` |
| content_format | 非空、≤ 32 字符 | 400 param=`content_format` |
| q / status / limit / offset | 见 §6.2 | 400 param=对应参数 |

长度与 trim 的校验风格对齐既有先例：服务层常量 + 手写校验（`backend/internal/modules/access/service.go:16` 的 `maxGroupNameLength`、`backend/internal/modules/auth/validate.go`）。

**不能校验的（明确声明）**：后端**不做 ProseMirror schema 级校验**——不验证节点/标记类型是否存在于某个 Tiptap schema、不验证文档结构（如 heading 的 content 表达式）、不认识任何节点语义。`content` 对后端是**不透明但格式合法的 JSON 对象**；结构合法性与渲染安全由前端 Tiptap schema 负责（D5）。理由：后端引入 Tiptap 依赖、跟随编辑器 schema 演进做双向校验，是持续维护负担且单人博客无对抗场景；粗校验已挡住体积与深度滥用。

### 7.3 缓存与派生列的地位（v2）

- `content_html` / `content_text` 是**客户端声明的事实**：服务端不验证它们与 `content` 的内容一致性（不做「text 是否约等于 content 的文字」这类检查），也不在任何读路径上从 content 重算它们。二者与 `content` / `content_format` 一起存于**快照行**，快照行不可变。
- `excerpt` / `search_text` 是**服务端二级派生**（从 head 快照的 content_text 派生）：excerpt = 前 200 个 Unicode 字符截断；search_text = 全量复制。写入时机 = 带内容组的保存 / 恢复的**同一事务**内刷新实体行（D12）；只改 title / slug / cover 的 PATCH 不触碰派生列。**历史快照的 content_text 不回刷派生列**——派生列永远只反映 head；head 前移后，旧版本保持原样（这正是快照不可变的意义）。
- 篡改与陈旧的容忍度（传导链）：前端提交的 content_text 失真 → 该快照的 content_text 失真 → 从它派生的 excerpt / search_text 跟着失真。**唯一代价**是列表摘要、搜索命中与 RSS/SEO/邮件（html）展示旧内容；权威内容（编辑器回灌读 head 快照的 content、公开详情读 release 快照的 content）不受影响。修复路径 = 前端重新保存（产生**新**快照并刷新派生列；失真的旧快照留在历史里不修复——历史是事实记录，不做清洗）。
- 二者永不回灌编辑器：编辑器只消费快照的 `content` 列（用户已拍板的基线）。
- 前端提取 `content_text` 的已知注意点（调研核实）：HardBreak 节点须转为 `\n`，否则摘要出现两段粘连；从 Word/网页粘贴的富文本按 schema 过滤后的形状处理。

## 8. content_format 版本策略（D7）

**形态：字符串（varchar），不用整数。**理由：

1. 标记的消费方是前端（与未来的迁移脚本），人类可读的方言名在排查时直接可辨，无需中央对照表；
2. 预期取值个位数，整数「可比较、可排序」的优势用不上；
3. ent 侧 `field.String` 与既有表风格一致，无额外类型映射。

取值与演进规则：

- v1 唯一取值 `pm1`（ProseMirror JSON 方言第 1 版；具体定名待与前端输出对齐确认，§16）。
- **何时升版本**：仅当编辑器产生的 JSON 出现「旧读取端无法合理解释」的**语义破坏性**结构变化（如既有节点/标记的含义重定义）。纯增量（新增节点/标记类型——旧内容不含它，读取端按未知节点忽略或保留）**不升版本**。
- **列随快照走（v2 顺带收益）**：每个快照自带方言标记，读取端按「这一版的 format」分派解释器即可，无需实体行上的「当前方言」推断——比 v1 的单列方案更自然。
- **旧版本怎么处理：只标记不迁移**。内容量小（个人博客），需要时由前端做一次性「打开—重存」（产生携带新 format 的新快照，旧快照原样保留），不做后端批量迁移作业。
- 服务端对 `content_format` **不做任何行为分支**：仅存储、回显、透传（新快照缺省时继承 head 的值）。

## 9. 搜索口径（D10）

v1 明确够用即可：

- `q` 参数（可选）出现在 `GET /api/v1/posts`（公开）与 `GET /api/v1/admin/posts`（管理）；
- 匹配规则：`title ILIKE '%q%' OR search_text ILIKE '%q%'`——**两列都在实体行上，查询不 join 快照表**（大小写不敏感；对中文无大小写概念，等价包含匹配）；
- **为什么在实体行上去规范化一份纯文本列（search_text）**：列表与搜索是最高频的读路径，语义上属于「元数据查询」；快照化后 content_text 在快照表里，若 ILIKE 直接打在快照表，每个搜索都变成跨表 join 或子查询，且要在（post_id, version）里挑 head/release 那一条，复杂度无谓上升。实体行上的 search_text 在保存事务里与 excerpt 一起刷新（D6），读路径零 join；个人博客量级下这份冗余（每篇一份纯文本副本 + 快照里的原件）完全可接受。
- `total` 按 q 过滤后的总数计算（§6.2 通用口径）；
- 实现注记：q 中的 LIKE/ILIKE 特殊字符（`%` `_` `\`）必须转义后拼接，防通配符注入式的全表通配；
- **不做**：tsvector / GIN 全文索引、分词（zhparser / pg_jieba）、pg_trgm、相关度排序、搜索结果高亮。理由：百篇量级的顺序扫描 ILIKE 是毫秒级，为它建全文索引属过度设计；中文全文检索列为后续演进（届时属新增可选参数与索引的 v1 内增量，或另立设计）。
- pages **不提供搜索**（数量个位数，非目标）。

## 10. 图片与资源（D11）

- v1 口径：图片以 **URL 内嵌于正文 JSON**（Tiptap image 节点的属性，存于快照），后端不解析、不代理、不改写、不校验可达性；`content_html` 里的 `<img>` 同理原样缓存。
- **URL 约定（写给前端轮的契约）**：正文图片 URL 与 `cover_image_url` 一律存**相对路径**（如 `/uploads/2026/10/x.webp`），不存带域名的绝对 URL——换域名/CDN 时免改存量 JSON（对快照 jsonb 做批量改写属自找的迁移；快照不可变更放大了改写的代价）。
- **上传端点为非目标**（本轮不做）：上传涉及存储选型（本地目录 / 对象存储）、体积与类型白名单、URL 生成策略，是一块独立设计；届时作为新增端点属 v1 内增量，不影响本轮表结构（URL 已在 JSON / cover 列里，无需新列）。
- 引用外部图床（或将来本站上传服务）的防盗链、失效、HTTP 升级问题属内容运营与前端渲染事务，后端不处理。

## 11. 装配与模块组织（D2）

```
backend/internal/modules/content/
├── module.go        New(db) *Module；Register(api) 挂公开路由；RegisterProtected(protected) 挂管理路由
├── controller.go    公开读 + 管理 CRUD/动作 + 快照三端点的 handler；管理路由逐条挂 RequirePermission
├── service.go       输入校验、三列成组规则、快照追加与恢复、发布状态机
├── repository.go    ent 查询与写入（posts / pages 两套方法 + 各自快照方法，对齐 access/repository.go 承载多实体先例）
└── dto.go           请求/响应 DTO（实体列表项、实体详情、快照列表项、快照详情、author 嵌套）
```

- 单模块承载两实体组而非 `posts` / `pages` 两个模块：两实体组同构、共享校验与发布/快照工作流，拆两模块会复制一套形状相同的 service/dto 而无边界收益；权限命名 `content.*`（access 设计 §7 预留）也已预设「content」是一个功能域。
- `app.go` 装配：`contentModule := content.New(db)`；`contentModule.Register(api)`（公开，无守卫）；`contentModule.RegisterProtected(protected)`（现有 protected 组已带 `RequireAuth → LoadIdentity`，`backend/internal/app/app.go:60`）。
- 依赖方向遵守 README 硬性规则 2/3/5：controller → service → repository；service 不 import gin；模块间只经 `app.go` 注入。
- rbac 目录增 4 行（`backend/internal/common/rbac/rbac.go:43-48` 的 catalog 与 `:9-14` 的常量同步扩展）。
- 多步写（创建、带内容组的保存、恢复、发布）走 `client.Tx(ctx)` 事务，version 分配前 `FOR UPDATE` 锁实体行（对齐 `backend/internal/modules/access/repository.go:214` 的先例）。

## 12. 数据模型（D1、D3、D4、D16）

### 12.1 两套实体组的权衡

v1 已裁决 posts 与 pages 分表（不用单表 type 判别列），v2 不推翻，只把裁决对象从「两张表」扩为「两套实体组（各含实体表 + 快照表）」：

| 方案 | 优点 | 缺点 |
| --- | --- | --- |
| 单表 + `type` 判别列（含快照共表） | 一套 schema/DTO/查询 | 判别列进入每个查询的 WHERE；slug 与 (实体id, version) 唯一性都要按 type 分区；将来两实体分化时列集合稀疏；ent 对单表继承建模别扭；权限点与 URL 面本来就要分开 |
| **两套实体组（本文采用）** | 每张表列集紧凑、约束直白；两实体可独立演进；ent 建模自然；权限与 URL 天然分离 | 列定义重复一遍；跨两实体的搜索将来需 UNION |

个人博客规模下「列重复」是零成本，而判别列方案把复杂度前置到每个查询与唯一索引里。

### 12.2 实体表字段清单

posts（pages 同构，逐字段相同）：

| 列 | 类型 | 可空 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | uuid | 否 | UUIDv7 | 主键；ent `field.UUID` + `uuid.NewV7()` 默认（对齐 `backend/internal/database/ent/schema/user.go:22`） |
| `title` | varchar | 否 | — | **放实体行（跟 HaoBlog），不随版本走**；理由：标题是列表卡 / 搜索 / 排序的元数据，随版本走会逼所有列表查询 join 快照表；改标题不产生新快照（§7.1）。服务层校验 1–200 字符；DB 不设长度（对齐现状列风格，`backend/migrations/20261010223715_init_users_sessions.sql`） |
| `slug` | varchar | 是 | NULL | **可空（v2.1，用户拍板暂不启用）**；提供时全表唯一（D4）——Postgres 唯一索引对多个 NULL 天然放行，无需部分唯一索引；格式校验在服务层且仅在有值时。**关于 HaoBlog 的「创建后冻结不改」（`prisma/schema.prisma:238` "frozen forever"）**：HaoBlog 的 slug 是机器生成的 abbrlink（crc32），人工不可读、改了也无意义，冻结合理；NeoBlog 的 slug 是人工撰写的 SEO 标识，保留 PATCH 可改可清（风险与 301 非目标见 §14 / §2） |
| `cover_image_url` | varchar | 是 | NULL | 封面（HaoBlog 同款去规范化列，`Post.coverImageUrl`）；列表卡 / 将来 og:image 用；可空；PATCH 空串清空（§6.6 位形说明 3）；相对路径约定同 §10 |
| `excerpt` | varchar | 否 | `''` | **去规范化摘要列（D12，v2 入库）**：服务端从 head 快照 content_text 截前 200 个 Unicode 字符，内容组保存 / 恢复同事务刷新；列表直接读、免 join。HaoBlog 的 excerpt 是作者手写的可空字段，NeoBlog v2 取「恒派生、非空、无手工编辑」（手工摘要留 §16 插槽） |
| `search_text` | text | 否 | `''` | **去规范化搜索列（D10，v2 新增）**：服务端从 head 快照 content_text 全量复制，仅供 ILIKE；不出现在任何响应里；刷新时机同 excerpt |
| `status` | varchar | 否 | `'draft'` | `draft` / `published`；ent `field.Enum`（迁移时人工核对 Atlas 生成的枚举表达；若生成形态不理想，退为 `field.String` + 服务层白名单校验，见 §16） |
| `author_id` | uuid | 否 | — | 外键 → `users.id`（users 主键为 uuid，`backend/internal/database/ent/schema/user.go:22`）；创建时取操作者，不可更新 |
| `head_snapshot_id` | uuid | 是 | NULL | **head 指针（D16）**：unique 外键 → `post_snapshots.id`（unique FK 表达 1:1，HaoBlog 同款）；事务中间态为 NULL，创建完成后恒非空（不变量：创建即落 v1 快照），服务层保证 |
| `release_snapshot_id` | uuid | 是 | NULL | **release 指针（D16/D17）**：unique 外键 → `post_snapshots.id`；NULL = 从未发布过（与 published_at NULL 同义）；unpublish 不清空 |
| `published_at` | timestamptz | 是 | NULL | 最近一次实际生效的发布时刻（D17）；可空时间对齐 `used_at` 先例（`backend/internal/database/ent/schema/recovery_code.go:23`，`Optional().Nillable()`） |
| `created_at` | timestamptz | 否 | now | ent `Default(time.Now).Immutable()`（对齐 `user.go:29`） |
| `updated_at` | timestamptz | 否 | now | ent `Default(time.Now).UpdateDefault(time.Now)`（对齐 `user.go:30`）；快照追加（指针 + 派生列更新）与 publish / unpublish 都刷新它，语义 =「最后变更时刻」 |

索引（实体表 ×2）：

| 索引 | 列 | 用途 |
| --- | --- | --- |
| 唯一 | `slug` | D4；slug 网址延后（v2.1），当前仅作唯一性保险，公开详情走 UUID |
| 复合 | `(status, created_at)` | 公开列表 `WHERE status='published' ORDER BY created_at DESC` 的覆盖路径 |
| 普通 | `created_at` | 管理列表全状态排序（对齐 users 先例 `user.go:44`） |
| 普通 | `author_id` | 外键手动建索引——Postgres 不为外键自动建索引（access 设计 §5 同款注记） |
| 唯一 | `head_snapshot_id` | unique FK 的唯一约束（1:1） |
| 唯一 | `release_snapshot_id` | 同上 |

### 12.3 快照表字段清单

post_snapshots（page_snapshots 同构，`post_id` 换 `page_id`）：

| 列 | 类型 | 可空 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | uuid | 否 | UUIDv7 | 主键 |
| `post_id` | uuid | 否 | — | 外键 → `posts.id`，**ON DELETE CASCADE**（删实体 → 快照级联删，D3；HaoBlog 同款 `onDelete: Cascade`） |
| `version` | int | 否 | — | 每实体从 1 单调递增；服务端在事务内分配（FOR UPDATE 锁实体行后取 max+1，§11）；请求永不提交 |
| `content` | jsonb | 否 | — | ProseMirror JSON 对象（权威内容；HaoBlog 名为 `raw`，NeoBlog 沿用 v1 的 `content` 命名）；ent `field.JSON`（Postgres 方言生成 jsonb，落地时以迁移 SQL 人工核对为准） |
| `content_html` | text | 否 | — | 客户端派生缓存（RSS/SEO/邮件）；`field.Text` |
| `content_text` | text | 否 | — | 客户端派生缓存（excerpt / search_text 的派生源）；`field.Text` |
| `content_format` | varchar | 否 | `'pm1'` | 方言标记（D7）；随快照走 |
| `created_at` | timestamptz | 否 | now | 快照产生时刻 = 该版本的保存时刻；**无 updated_at**——快照行不可变（D16），没有任何 UPDATE 路径 |

索引（快照表 ×2）：

| 索引 | 列 | 用途 |
| --- | --- | --- |
| 唯一复合 | `(post_id, version)` | 版本定位（快照详情 `WHERE post_id AND version`）与按版本倒序的历史列表；HaoBlog 同款 `@@unique([postId, version])` |
| （外键索引由 CASCADE 级联与 `(post_id, version)` 复合索引最左前缀覆盖，不单建） | | |

- **HaoBlog 对快照 raw jsonb 建了 GIN（schema 外手工加的，其注释警告 Prisma diff 会报 drift）——NeoBlog 不抄**：v2 没有任何 jsonb 包含查询（`@>`）场景（搜索走实体行 search_text，§9），内容读取永远是主键 / unique 定位；GIN 在百行级数据上是纯写放大。
- **pages 与 posts 完全同构（含 head 指针）——对 HaoBlog 的有意偏离**：HaoBlog 的 Page 只有 `releaseSnapshotId` 没有 head（其注释口径：页面直接编辑即生效、status 单独把关、页面不进列表卡）；NeoBlog v1.1 已冻结「pages 与 posts 同构、共用草稿/发布工作流」，管理详情读 head、恢复走 head 前移都需要 head 指针，故 pages 同样配齐双指针。
- **循环外键**：`posts.head_snapshot_id → post_snapshots.id` 与 `post_snapshots.post_id → posts.id` 构成环。写入顺序 = 同事务「先插实体行（指针 NULL）→ 插快照 → 回填指针」；ent 侧同类型间多条边（history / head / release）需显式命名，生成代码后核对。删除路径见 §14 注记。

**软删与否（D3）**：auth / access 全库无软删先例（无任何 `deleted_at` 列；access 的组删除是硬删 + 外键保护，`backend/internal/modules/access/repository.go:239-247`），内容模块跟随：**硬删**（HaoBlog 的 `deletedAt` 不抄）。因此 slug 唯一性无「软删行占坑」问题，无需部分唯一索引；时间戳输出也无需 `deleted_at` 字段。

**外键删除行为**：`author_id` → users 用默认 NO ACTION——但博客场景作者即博主，用户删除功能本身不存在（access 无 DELETE /users），该约束仅兜底；head / release 两个 unique FK 用 NO ACTION（HaoBlog 同款），依赖「删实体先于级联删快照、被删行自身带走了引用」的语义，核对点见 §13 / §14。

## 13. 迁移与测试

**迁移**（对齐数据层设计 §6 与 access 设计 §11 的工作流）：

1. 一个迁移，建议名 `content_posts_pages`，产出四张表。`scripts/db-migrate-diff.sh` 现仍硬编码 `totp_and_recovery_codes`（v2.2 修正：access 轮并未去硬编码），本轮先把它参数化为接收 `$1` 再使用。
2. 无 seed 数据（不像 groups 需要预设行）。
3. 人工审阅核对五点：`content` 列为 jsonb（非 json）；`status` 的枚举表达（CHECK 约束或 varchar）与 ent schema 一致；**循环外键的两段式 ALTER**（实体表先建、指针列后加）与唯一索引在位；级联删除方向正确（快照 `post_id` CASCADE，指针 FK NO ACTION）；索引在位（实体表各 6 个、快照表各 1 个唯一复合）。
4. **删除路径核对**：`DELETE FROM posts WHERE id=…` 应在同一语句内级联删快照且不受指针 FK 阻拦（被删行自身带走了 head/release 引用）；若 Postgres 的 NO ACTION 检查时序拦截，退为事务内「先置 NULL 指针 → 删实体行」两步，用集成测试实证（见 §16）。
5. 手改后 `atlas migrate hash` + `atlas migrate validate`；应用到 `neoblog` 与 `neoblog_test`。
6. 应用启动绝不自动迁移（数据层设计 §2 冻结纪律）。

**测试**（分层对齐 access 设计 §12）：

| 层 | 连库 | 内容 |
| --- | --- | --- |
| service 单测 | 否 | slug 格式、title 长度、三列成组规则、content 体积/深度粗校验、状态机动作语义（含幂等与 published_at 刷新条件）、恢复语义（复制为新版本、不销毁历史） |
| repository/service 集成（`DATABASE_URL_TEST` 缺省 skip） | 是 | CRUD（创建落 v1、带内容组 PATCH 落 v2+head 前移、只改 title 不产生快照）；slug 冲突 409；公开列表只出已发布且只读实体行；公开详情读 release（发布后编辑，公开侧仍是旧版，直至再 publish）；分页钳制与非法值；`total` 在 status/q 过滤下正确；q 转义；publish/unpublish 的 release 与 published_at 语义；恢复后 head 前移且历史全保留；快照列表分页与 is_head/is_release 正确；删除实体 → 快照级联删 |
| 权限矩阵集成 | 是 | 公开端点未登录可读；管理端点（含快照三端点）无权限 403、未登录 401；`content.posts.edit` 蕴含 `view` |
| 并发集成 | 是 | 两个并发内容保存线性化为 v2 / v3（FOR UPDATE），不出现 unique 冲突错误泄漏 |
| 迁移后置断言 | 是 | 四表存在、索引在位、`content` 列为 jsonb、级联删可行 |

## 14. 风险对冲注记

- **html 缓存陈旧 / 被篡改**：`content_html` 失真只影响 RSS/SEO/邮件，不影响编辑与正文阅读；spec 的立场是**容忍 + 重存自愈**（§7.3），不做服务端一致性校验，也不做「检测失真」的机制（成本远超单人博客收益）。v2 的传导链：content_text 失真会连带 excerpt / search_text 失真，修复路径同样是重存（新快照 + 派生列刷新）；失真快照留在历史里不清洗。
- **ProseMirror schema 演进**：编辑器升级可能产生旧读取端不认识的节点。立场：增量演化不升 `content_format`（未知节点由前端解释器忽略或保留）；破坏性语义变更才升版本，且只标记不迁移（§8）。风险敞口是「升版本判断失误导致旧内容渲染异常」——由前端渲染层兜底，后端不介入。**前端硬性要求**：编辑器加载存量 JSON 必须显式开启 `enableContentCheck`——Tiptap 默认**静默剥离** schema 不匹配的内容（官方文档可证），升级或删扩展后旧文章会被无声删块，且只在下一次保存时永久丢失；v2 的缓解：被剥离前的原快照仍在历史里，可从快照详情取出原文重救——快照化把「静默丢失」降级为「可找回的误编辑」，但 `enableContentCheck` 仍是前端轮验收项（剥离发生在**新快照写入前**，坏内容一旦保存即成新事实）。挂靠对象：快照 JSON。
- **并发覆盖**：两个写者同时 PATCH 同一篇，后写者获胜，无乐观锁 / 版本比对（D13）。v2 理由改写：**历史快照兜底**——误覆盖最多丢「最新一次编辑」，旧版本都在，恢复即可找回；因此「长会话编辑期间他人保存，本端保存时静默覆盖」从数据丢失降级为体验问题。version 分配的并发正确性由事务内 FOR UPDATE 线性化保证（实现机制，非乐观锁）；基于 head version 的 If-Match 类互踩提示留 §16 插槽（价值因历史兜底而降低）。
- **公开 404 不可区分**：未发布与不存在同响应（§6.5），防草稿存在性探测；副作用是 slug 拼错无法与「尚未发布」区分——可接受。
- **XSS 面**：后端不 sanitize `content_html`，公开端点原样输出（§7.2 的「不做 schema 级校验」同理延伸到 HTML）。这是明确立场：存储层不消毒，渲染安全由**前端消费侧**（Tiptap schema 渲染、RSS 模板）负责。风险注记：未来任何「直接 dangerouslySetInnerHTML 消费 content_html」的实现必须配 sanitize 策略，属前端轮事务。**已知前科（2026-10-11 调研核实）**：`@tiptap/extension-link` < 2.10.4 存在 `javascript:` href XSS（CVE-2025-14284 / GHSA-vhrc-hgrq-x75r），前端 Tiptap 依赖必须锁定 ≥ 2.10.4；另需注意安全过滤来自渲染端 renderHTML 而非「JSON 本身干净」——JSON 加载路径不经过 parseHTML，attrs 原样进文档。需要从 JSON 免编辑器渲染时，官方 `@tiptap/static-renderer` 走 schema renderHTML 路径（白名单外 href 被剥除），可作为快照 content_html 陈旧时的现渲兜底。历史快照同样未经消毒，快照详情端点的消费方与公开详情同等对待。
- **slug 修改使外链失效**：已设置 slug 的文章可改 slug，v1 不做 301 表（§2 非目标 13）；单人博客里由站长自行权衡。v2.1 起 slug 暂不启用，该风险当前休眠。
- **大负载滥用**：三列体积上限（§7.2）是防滥用粗闸；Gin 默认无 body 限制，强制位置（服务层校验 vs `http.MaxBytesReader` 中间件）留实施计划（§16）。
- **ILIKE 无索引**：q 搜索在百篇级是顺序扫描，毫秒级；数据量到万篇再议 pg_trgm（§9 非目标）。
- **快照增长（v2 新增）**：每次带内容的保存追加一行（上限约 content 256KB + html 1MB + text 256KB）。量级估算：日更 10 版 × 10 年 ≈ 3.6 万行、数百 MB 内，个人博客完全可接受；v1 立场 = 永久保留（D19），清理/上限/压缩列非目标，将来需要时属独立演进。注意「无脑全选保存」习惯会加速增长——真实增量通常远小于上限。
- **循环外键的删除路径（v2 新增）**：`posts ↔ post_snapshots` 双向引用下，删实体依赖「被删行自身带走指针引用 + CASCADE 删快照」在同一语句内成立；若 Postgres 检查时序拦截（NO ACTION 先于级联评估），退为事务内两步删（§13 核对点 4）。
- **apierror 包未落地**：§6.5 注记的时序依赖，若 content 先行需用过渡形态，词汇表仍只有一份。

## 15. 验收标准

1. ent 重生成后 `go build ./... && go vet ./... && gofmt -l .` 干净；生成代码提交。
2. 迁移人工审阅、hash / validate 通过；应用到 `neoblog` 与 `neoblog_test`。
3. `go test ./...` 全绿（无库环境集成测试 skip）。
4. 迁移后置断言通过（四表、索引在位、jsonb 列型、级联删可行）。
5. 手工 curl 抽查：公开列表仅含已发布且信封四字段齐；未发布 / 不存在的 `:id` 公开详情 404；创建不带 slug → 响应 `slug=null`；`limit=500` 钳制 100 且回显生效值；创建后 `head_version=1`；带内容组编辑产生 v2 且公开侧（已发布时）仍读旧 release 版，再 publish 后公开侧更新；unpublish 后 release 保留；恢复 v1 追加为新 head 版且历史三条俱全；快照列表分页与 `is_head` / `is_release` 正确；`:version=abc` → 400 param=`version`；slug 冲突 409 `slug_taken`（param=`slug`）；无权限访问管理端点（含快照端点）403；删除文章后快照行清零。
6. README 更新（模块目录表、接口表、权限目录）；DEVLOG 条目；版本 `./scripts/bump-version.sh minor`（0.6.0 起，按执行时当前版本顺延）。

## 16. 遗留问题（留给实施计划的开放决策）

1. **apierror 形态（v2.2 已裁决）**：包已存在，content 直接加哨兵与注册行并复用 `Write`；错误体 `message` 字段与旧 controller 的收敛留给全库规范化批次。
2. **`content_format` 首值定名**：`pm1` 为本文建议值，实施时与前端 `getJSON()` 输出对齐后定稿。备选方案是存「schema 指纹」（扩展清单 + 顺序的 hash，可程序化判定哪些存量快照受某次扩展变更影响）；个人博客规模下简单标记已够，指纹方案在出现第二套方言时再议。
3. **粗校验阈值的最终数值**：title 200 字符、slug 120 字符、content 256 KB / 深度 64 层、content_html 1 MB、content_text 256 KB、excerpt 200 字符、cover_image_url 2048 字符、q 100 字符——均取合理默认，实施时可调，落地后在 README/接口文档固化。
4. **深度校验的实现位置**：service 递归计数 vs 请求中间件统一拦——实现细节，不影响契约。
5. **大负载上限的强制位置**：服务层校验 vs `http.MaxBytesReader`（§14 注记）。
6. **`status` 的 ent 表达**：`field.Enum`（DB 级 CHECK）vs `field.String` + 服务层白名单——取决于 Atlas 对 ent enum 的生成形态，迁移审阅时定（§12.2 注记）。
7. **slug 公开网址（延后插槽，v2.1 用户拍板本期不做）**：将来启用「/posts/hello-world」形式时属 v1 内增量——注意按 UUID 与按 slug 取详情共用同一路径形态，届时用单端点「先按 UUID 解析、失败再按 slug 查找」的口径实现，避免双路由歧义。
8. **管理列表排序的次级稳定键**（`id` 倒序）确认。
9. **pages 的 `sort_order`（导航排序）**：出现真实菜单需求时加列，属 v1 内增量。
10. **循环外键删除路径**：迁移审阅与集成测试实证「同语句级联删」可行；若被拦，退为事务内两步删（先置 NULL 指针再删实体），repository 形态随之定（§13 核对点 4、§14 注记）。
11. **并发互踩提示插槽**：基于 head version 的 If-Match 类检查（PATCH 带期望版本，不匹配 409）——历史快照已兜底数据安全，仅在出现真实误覆盖痛点时再议（§14）。
12. **自定义 excerpt 插槽**：手工撰写摘要覆盖派生值（HaoBlog 的作者手写模式）——出现真实需求时给 PATCH excerpt 或独立字段，属 v1 内增量。
13. **快照清理策略插槽**：量级变化（如引入大文件附件、多作者高频保存）时再议保留策略 / 归档 / 压缩（D19）。
14. **公开列表摘要的取材源**：excerpt 恒从 head 快照派生（D12），而公开详情正文读 release——两者在「发布后编辑、未再发布」窗口内不一致（列表摘要露新内容、正文仍旧版）。个人博客影响有限，v2.1 按现状（head）实施；若在意，改为发布动作时从 release 快照派生，属小改。

## 17. 下一步

本文经用户逐项确认（§6.6 两处位形说明）后，进 writing-plans 出实施计划（范围 = §2「本轮做」，验收 = §15）。slug 网址已由用户拍板延后（§16 第 7 条）。
