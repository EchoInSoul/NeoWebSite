# NeoBlog 用户组与权限设计（访问控制层，层 2）

日期：2026-10-11
范围：`backend/`（Go + Gin + ent + Atlas）
状态：设计经用户逐项确认（2026-10-11 对话稿 + 两份调研报告）；本文为存档版，其后进 writing-plans 出实施计划

修订记录：
- v1（2026-10-11，本文）：首版。模型 = 单层用户组 + 组权限 + 用户额外权限；预设组 owner / admin / 普通用户；权限目录由代码定义；预设组权限由代码计算、不入库。
- v2（2026-10-11，双代理审阅后修订）：措辞对齐已落地的注册入口（认证层 2）与当前版本口径（0.3.1 → 0.4.0）；补 §6 规则 1 的差集口径与「摘除不受限」、规则 2/4 的顺序与并发可达性说明、规则 8（组侧无「自己所在组」限制）；补 §9 输入规则与响应口径、§13 四项已知取舍、§14 首个 owner 造数步骤。

## 1. 意图与背景

目标：给后端加一层「谁能做什么」。管理员能建用户组、给组勾权限、把用户挂到组上；接口按权限点拦截（未登录 401 / 权限不足 403）。

已定约束（用户）：

- 只有「用户组」一层：组直接挂权限；不做组继承、不做权限包。
- 一个用户只属于一个组。
- 用户身上可以单独加权限，**只能加不能减**；生效权限 = 所属组的权限 ∪ 额外权限。
- 三个预设组：owner / admin / 普通用户；此外可自建组。
- 后端只做后端：本轮出表、迁移、接口、守卫、测试、文档；控制台界面下一轮。
- 沿用既有规范：零注释、NestJS 式模块结构、ent + Atlas 迁移工作流、`DATABASE_URL_TEST` 集成测试约定。

对齐来源（不重新发明）：

- 参考实现 `F:\protect\NextJs_Blog`（HaoBlog，Next.js + Prisma）：`Role` / `RoleBinding` / `Permission` / `RolePermission` 四表、系统角色 owner / admin / user、权限清单在代码、一用户一角色、成套的保护规则。
- 本仓库既有设计：`docs/superpowers/specs/2026-10-11-neoblog-auth-design.md`（层 1：会话与守卫）、`docs/superpowers/specs/2026-10-11-neoblog-data-layer-design.md`（ent + Atlas 纪律）。

## 2. 与 HaoBlog 方案的对照（抄什么、不抄什么）

抄：

| 项 | 说明 |
| --- | --- |
| 权限清单在代码 + 「模块.动作」命名 + `edit` 蕴含 `view` | 他们经历过动作粒度反复（34→37→39→38 键）后收敛到 view/edit 两档，这个结论直接借用 |
| 一个用户一个组 | 他们用 `RoleBinding.userId @unique` 硬约束；本设计落成 `users.group_id` 非空外键 |
| 只能授出自己持有的权限 | 防管理员提权，他们用测试锁死了大量分支 |
| 不能改自己 | 改自己的组 / 额外权限一律拒绝 |
| 预设组锁死 | 不可删、不可改名、权限不可编辑 |
| 自建组有成员不许删 | 先迁人再删组 |
| 至少一个 owner 的保护 | 检查与写入必须在同一事务，否则并发下会漏判 |

不抄（及理由）：

| 项 | 他们的问题 | 本设计 |
| --- | --- | --- |
| 权限行入库 + 首建号一次性 seed | 新增权限点要手工补种，漏种即全站锁死（权限从库里读，fail-closed）；补种脚本未入库、只补 admin 还会清掉 owner 与自定义组 | **预设组权限由代码计算、不入库**（owner / admin = 全量、普通用户 = 空），只有自建组写 `group_permissions`；漂移面为零 |
| owner 完全不可指派 | 文档自认「owner 长期不可用时无法调整管理员阵容」是运维死角 | 允许 owner 指派 owner（「不能改自己」仍在，须由另一个 owner 操作） |
| slug 角色名 | slug 上限 48 与筛选校验上限 32 不一致，导致筛选静默失效 | 自建组以 UUID 为标识，组名唯一；不做 slug |
| 改角色 / 改用户后吊销全部会话 | 他们会话只存 userID、权限每请求现读，吊销只是 UX 动作 | 不做；权限每请求现算，改动下一次请求即生效（见 §13） |
| 审计留痕 | 建了又整体拆除 | 本轮不做（见 §15 插槽） |
| disabled 账号口径 | 保护判定必须区分「认定」与「早退」是否剔除禁用账号 | NeoBlog 尚无禁用概念，不涉及 |

## 3. 范围

本轮做：

- ent schema：新增 `Group` / `GroupPermission` / `UserPermission`，`User` 增加 `group_id`；生成代码提交。
- 新包 `internal/common/rbac`：权限目录、动作与蕴含、预设组常量（key、固定 UUID、展示名）。
- 新模块 `internal/modules/access/`：组 CRUD、用户管理接口、生效权限解析。
- `internal/common/guard/`：新增身份加载与权限守卫（`LoadIdentity` / `RequirePermission`）。
- `internal/modules/auth/`：`/auth/me` 响应扩展（group / permissions）。
- 迁移：一个迁移（建表 + 预设组 seed + `users.group_id` 回填）；`scripts/db-migrate-diff.sh` 迁移名去硬编码；`scripts/db-generate.sh` 开 `--feature sql/lock`。
- `internal/app/app.go` 装配。
- 测试（分层见 §12）、README 更新、DEVLOG 条目、版本进位 minor：0.3.1 → 0.4.0（执行时若已被其他层进位，按当前版本顺延）。

本轮不做：控制台界面与前端接线；首次建号入口；新用户创建路径的默认组指派（默认组由 ent 字段级默认值落实，见 §5）；用户禁用；会话吊销；审计；限流；多组 / 组继承；用户列表分页。

## 4. 名词与模型

- **权限点**：`模块.动作` 形式的字符串（如 `system.users.edit`），目录在代码里（§7）。
- **用户组**：权限的载体。预设组三个（owner / admin / 普通用户），权限由代码计算；自建组任意个，权限存库。
- **额外权限**：挂在用户身上的附加权限，只能加不能减。
- **生效权限** = 所属组的权限 ∪ 额外权限（含 `edit` 蕴含 `view` 的展开）。

## 5. 数据模型（ent schema 摘要）

groups：

- `id`：uuid、主键、默认 UUIDv7
- `system_key`：text、可空、唯一；预设组为 `owner` / `admin` / `user`，自建组为 NULL
- `name`：text、必填、唯一（预设组为「所有者」「管理员」「普通用户」；前端可按 `system_key` 本地化覆盖）
- `created_at` / `updated_at`：timestamptz

group_permissions（只有自建组会有行；预设组恒为空）：

- `id`：uuid、主键（ent 普通实体不支持复合主键，用代理主键 + 唯一索引）
- `group_id`：uuid、必填、外键 → `groups`，级联删除
- `permission`：text、必填
- 唯一索引 `(group_id, permission)`
- `created_at`

user_permissions：

- 结构同上，`user_id` → `users`，级联删除；唯一索引 `(user_id, permission)`

users 增量：

- `group_id`：uuid、非空、外键 → `groups`，删除行为默认（NO ACTION，删有成员的组被 DB 拒绝，服务层给出友好 409）
- `group_id` 的 ent 字段级默认值 = 「普通用户」组固定 UUID：不显式设组的创建路径自动落「普通用户」，**创建代码零改动**（ent 的 `defaults()` 先于必填校验执行、默认值经 edge setter 落入必填边；已核对 ent v0.14.6 源码，落地用一条集成测试实证）
- `group_id` 索引（Postgres 不会为外键自动建索引）

要点：

- 预设组用**固定 UUID**（常量在 `rbac` 包，迁移 SQL 里写字面量），保证代码随时能拿「普通用户」组 ID 做默认组。测试断言两处一致以兜底。
- 级联注解（`entsql.OnDelete(Cascade)`）挂在 `Group.permissions` / `User.permissions` 两个 `edge.To` 上。
- ent 的 Go 级默认值不落 DB DDL（现状 users 表同此），预设组行由迁移 INSERT 提供，不用 DB 默认值。

## 6. 预设组与规则

| 组 | 权限 | 备注 |
| --- | --- | --- |
| owner 所有者 | 全部（代码算，含以后新增的目录项） | 不可删 / 改名 / 改权限；成员只有 owner 能改 |
| admin 管理员 | 全部（与 owner 同权，只差保护规则） | 同上 |
| 普通用户 | 空（代码算） | 新用户默认组；成员由持 `system.users.edit` 的人改 |
| 自建组 | 勾什么是什么 | 可改名、可改权限、可删（无成员时）；唯一会写 `group_permissions` 的组 |

规则（全部为服务端强制）：

1. **只能授出自己持有的权限**：新建组、改组权限、给用户加额外权限、把用户设到某个组，凡目标会「新增而操作者没有」的权限点一律 403 `grant_forbidden`。差集口径：以**目标生效权限集的前后变化**为准——`next = 下一组权限 ∪ 下一额外权限`，`current = 当前组权限 ∪ 当前额外权限`，两侧先做蕴含展开，`added = next − current`；同一请求内组与额外权限的变更合并后只算一次差集；保持不变不算授予。**摘除不受限**：把权限从组或用户身上拿掉不做「必须自己持有」校验（删权限不会造成提权）。
2. **不能改自己**：任何人不能改自己的组、自己的额外权限 → 403 `cannot_modify_self`（连带效果：owner 不能给自己降级，交棒必须由另一个 owner 操作）。
3. **owner / admin 组的成员只有 owner 能改**（加入与移出都是）；普通用户组与自建组由持 `system.users.edit` 的人改 → 403 `owner_protected`。
4. **至少一个 owner**：移出 owner 组前在事务内校验（先 `FOR UPDATE` 锁 owner 组成员行再计数，见 §11）→ 409 `last_owner`。**这是并发保护**：串行路径下 owner 数不可能降到 0（规则 2 挡住自降、规则 3 挡住非 owner 操作 owner），所以 409 只在两个请求同时把两个 owner 各自降级时出现。检查顺序固定为 `cannot_modify_self`(403) → `owner_protected`(403) → `last_owner`(409)，即 403 优先于 409。
5. 权限检查统一按蕴含展开：勾了 `edit` 视为同时持有 `view`；读写接口口径见 §9。
6. 预设组锁死：改其名 / 权限 / 删除 → 403 `preset_group_locked`。
7. 新用户默认落「普通用户」（无权限组），由 ent 字段级默认值保证（§5）——创建路径不显式设组（公开注册已落地并遵循此约定）；以后不得把默认改成含权限的组（HaoBlog 曾因默认组带写权限出现提权 bug）。
8. 组侧没有「自己所在组」的限制：持 `system.groups.edit` 的人可以改任意自建组（含自己所在的组）的权限；因为只对新增权限做校验（规则 1）且摘除不受限，这里不存在借改组提权的路径。代价是能摘掉同组/其他自建组的权限——这是该权限点的正常语义（参照实现同样如此），不是漏洞。

## 7. 权限目录

v1 只有 4 个点（目录里每个 key 都必须有真实检查点，不预埋空权限）：

| key | 展示名 |
| --- | --- |
| `system.users.view` | 查看用户 |
| `system.users.edit` | 管理用户 |
| `system.groups.view` | 查看用户组 |
| `system.groups.edit` | 管理用户组 |

规范：

- 命名 `模块.动作`；动作白名单只有 `view` / `edit`；蕴含关系 `edit → view`（含传递闭包，当前只有一条边）。
- 目录是 Go 常量（`internal/common/rbac`），随模块落地逐条增加（以后 `content.posts.edit` 等）；目录只增不改语义，删除权限点会留下孤儿行（本轮不清理，见 §13）。
- 清单经 `GET /api/v1/admin/permissions` 吐给前端；前端不持有副本（Go 是唯一权威）。

## 8. 生效权限与守卫

生效权限计算（access 模块的 service）：

1. 取用户及其组的 `system_key`；
2. 预设组：owner / admin → 目录全量、普通用户 → 空（代码计算）；
3. 自建组：读 `group_permissions` 行；
4. 并入 `user_permissions` 行；
5. 做蕴含展开（`edit` → `view`）。

守卫（`internal/common/guard`）：

- `RequireAuth`（现有）：会话里有 `auth.user_id` 放行，否则 401。
- `LoadIdentity(resolver, sessions)`：读 `auth.user_id` → 调 resolver 拿身份（含 `GroupID` / `GroupKey` / `GroupName` / `Permissions`）→ 写入 gin context（键 `auth.identity`）；用户已不存在时销毁会话并 401（与 auth 现有语义一致）；其他错误 500。
- `RequirePermission("system.users.edit")`：从 context 读身份，权限不够 403 `{"error":"forbidden"}`；身份缺失视为装配错误，500。
- `resolver` 接口由 guard 定义（消费方定义接口，先例：health 模块的 `pinger`），access 模块的 `Service` 实现它，`app.go` 显式注入——不违反 README 规则 5，guard 不 import 业务模块。
- 每个受保护请求解析一次身份（约 2-3 条查询）；请求内多处检查不重复查库。不做进程内缓存。

原则变更记录：层 1 曾写「守卫不查库」（auth 设计 §5）。权限守卫必须查库，此原则被扩展为「守卫只依赖注入进来的 resolver，不 import 业务模块」；README 与 DEVLOG 同步更新。

## 9. 接口

| 方法 路径 | 权限 | 说明 |
| --- | --- | --- |
| `GET /api/v1/admin/permissions` | `system.groups.view` | 权限目录 |
| `GET /api/v1/admin/groups` | `system.groups.view` | 组列表（预设 + 自建） |
| `POST /api/v1/admin/groups` | `system.groups.edit` | 新建自建组 |
| `PATCH /api/v1/admin/groups/:id` | `system.groups.edit` | 改组名 / 组权限 |
| `DELETE /api/v1/admin/groups/:id` | `system.groups.edit` | 删自建组 |
| `GET /api/v1/admin/users` | `system.users.view` | 用户列表（按创建时间倒序，本轮不分页） |
| `GET /api/v1/admin/users/:id` | `system.users.view` | 用户详情（组 + 额外权限 + 生效权限） |
| `PATCH /api/v1/admin/users/:id` | `system.users.edit` | 设组 / 设额外权限 |
| `GET /api/v1/auth/me` | 登录即可 | 扩展为带 group 与 permissions |

响应形状（节选）：

- `GET /admin/permissions` → `{"items":[{"key":"system.users.edit","name":"管理用户"}]}`
- `GET /admin/groups` → `{"items":[{"id","system_key","name","is_preset","member_count","permissions":[...]}]}`
- `POST /admin/groups` 请求 `{"name":"编辑","permissions":["system.users.edit"]}` → 201 组对象
- `PATCH /admin/groups/:id` 请求 `{"name"?,"permissions"?}` → 200 组对象
- `DELETE /admin/groups/:id` → 204
- `GET /admin/users` → `{"items":[{"id","email","nickname","group":{"id","system_key","name"},"extra_permissions":[...]}]}`
- `GET /admin/users/:id` → 用户对象 + `"permissions":[...]`（生效集合，展开后）
- `PATCH /admin/users/:id` 请求 `{"group_id"?,"extra_permissions"?}`（至少给一个；`extra_permissions` 为全量替换）→ 200 用户详情
- `GET /auth/me` → `{"email","nickname","group":{"id","system_key","name"},"permissions":[...]}`

口径：

- **读接口返回展开后的权限集，写接口收原始勾选**（前端不需要理解蕴含规则）；写接口返回的组对象同样按读口径（展开后）。`GET /admin/groups` 里预设组的 `permissions` 是代码算出的全量 / 空集。
- 自建组的 `system_key` 输出 `null`（Go 侧 `*string`），预设组输出 `owner` / `admin` / `user`。
- 输入规则：组名 trim 后 1–64 字符（允许 Unicode、拒绝纯空白串），为空或超限 → 400 `invalid_request`；`permissions` / `extra_permissions` 数组先去重，含目录外 key → 400 `invalid_permission`；`PATCH /admin/groups/:id` 两个字段都不给 → 400 `invalid_request`；`PATCH /admin/users/:id` 的 `group_id` 为空串或畸形 UUID → 400，`null` / 缺省 = 不修改；`extra_permissions` 为 `[]` = 清空全部额外权限，`null` / 缺省 = 不修改。
- `/auth/logout` 对「用户已被删除但还持有会话」的请求现在返回 401（`LoadIdentity` 先于 handler 执行并销毁会话），不再是 204。

错误码（沿用 `{"error":"..."}` 形状）：

| 状态 | error | 场景 |
| --- | --- | --- |
| 400 | `invalid_request` | 请求体形状 / 字段非法 |
| 400 | `invalid_permission` | 权限 key 不在目录 |
| 401 | `unauthorized` | 未登录 / 用户已被删除 |
| 403 | `forbidden` | 缺少所需权限点 |
| 403 | `grant_forbidden` | 授出操作者自己没有的权限 |
| 403 | `owner_protected` | 非 owner 改动 owner / admin 组成员 |
| 403 | `cannot_modify_self` | 改自己的组或额外权限 |
| 403 | `preset_group_locked` | 改 / 删预设组的锁死字段 |
| 404 | `not_found` | 组 / 用户不存在 |
| 409 | `group_not_empty` | 删有成员的自建组 |
| 409 | `group_name_taken` | 组名重复 |
| 409 | `last_owner` | 移出最后一个 owner |
| 500 | `internal_error` | 其他 |

## 10. 装配

- `app.go`：
  - `accessModule := access.New(db)`；
  - 保护组：`protected := api.Group("", guard.RequireAuth(sessions), guard.LoadIdentity(accessModule.Service, sessions))`；
  - `authModule.RegisterProtected(protected)`（登出、me、改密码）+ `accessModule.RegisterProtected(protected)`（admin 路由）。
- 模块边界：`access` 模块同时承载组 CRUD 与用户管理接口，经共享 ent client 读写 users（对齐数据层设计「不隔仓储接口层、边界靠 DTO」）；不引入 auth ↔ access 双向依赖。
- `/auth/me` 的 group / permissions 从 gin context（`auth.identity`）取，controller 已 import guard，无新增模块依赖。
- `internal/common/rbac` 是纯 Go 包（不 import gin、不 import 业务模块）：ent schema 与 auth 模块都可能需要预设常量（`internal/database` 不得 import 业务模块）。

## 11. 迁移与 seed（本轮最高风险步骤）

1. `scripts/db-generate.sh` 加 `--feature sql/lock`（「至少一个 owner」检查要在事务内 `ForUpdate` 锁 owner 组成员行；一次性全量重生成并提交）。
2. `scripts/db-migrate-diff.sh` 的迁移名去硬编码（现为 `init_users_sessions`），改为 `groups_and_permissions`。
3. 生成迁移后人工审阅，并手改三处：
   - `INSERT` 三个预设组（固定 UUID、`system_key`、中文名、显式时间戳、`ON CONFLICT DO NOTHING`）——必须落在 `CREATE TABLE "groups"` 之后（生成顺序不同就按实际位置调整）。**没有权限行 seed**——预设组权限由代码算。
   - `users.group_id` 回填三步：加可空列 → `UPDATE ... SET group_id = '<普通用户组 UUID>' WHERE group_id IS NULL` → `SET NOT NULL` + 建外键（直接加非空列在已有数据的库上必失败）。DDL 不设 DB 默认值；新行的默认落组由 ent 字段级默认值负责（§5），两处口径一致。
   - 索引与唯一索引（ent 生成部分核对）。
4. `atlas migrate hash` + `atlas migrate validate`（手改 SQL 后必须重算校验和）。
5. `scripts/db-migrate-apply.sh` 应用到 `neoblog` 与 `neoblog_test`。

## 12. 测试

| 层 | 连库 | 内容 |
| --- | --- | --- |
| rbac 单测 | 否 | key 唯一、格式 `模块.动作`、动作白名单、蕴含展开、预设组定义完整 |
| guard 单测 | 否 | 内存 scs + fake resolver：无会话 401、身份解析失败、权限不足 403、有权限放行、context 键写入 |
| access 集成（HTTP / service 级，`DATABASE_URL_TEST` 缺省 skip） | 是 | 组 CRUD（预设锁死、重名、非空拒删、未知权限 key、输入校验）；设组与额外权限（并集、组转移的 grant 差集、只能加不能减的实际语义、摘除不受限、`grant_forbidden`、`cannot_modify_self`、`owner_protected`、`last_owner`（并发））；`/auth/me` 新形状；401 / 403 |
| 迁移后置断言 | 是 | 三个预设组按固定 UUID 存在、`system_key` 与 rbac 常量一致、普通用户组无权限行、不显式设组建用户自动落「普通用户」 |

测试数据用唯一后缀 + `t.Cleanup` 清理；**不删预设组**。集成测试沿用「库已迁移」前置约定（README 写明）。

## 13. 已知缺口与风险

- **守卫开始查库**：每请求多 2-3 条查询；博客规模可接受，将来需要时再加请求内缓存之外的缓存层（会话里存快照的方案已被否决：会引入失效问题）。
- **不吊销会话**：改组 / 改权限后，目标用户下一次请求即生效；不做强制下线（要做得遍历 scs 会话，留作插槽）。
- **迁移回填**：`neoblog` / `neoblog_test` 里可能已有手工建的管理员数据；回填 SQL 必须人工审阅后执行。
- **固定 UUID 两处**：`rbac` 常量与迁移 SQL 字面量；靠集成测试断言兜底。
- **权限目录删除**：删点会留 DB 孤儿行；本轮不清理。
- **用户列表不分页**：博客尺度够用；接口形状（`items` 包裹）为分页预留。
- **列表不做可见性遮蔽**：持 `system.users.view` 的人能看到全部用户（含 owner）；行级操作按钮的呈现方式属前端轮的事，服务端已有 `owner_protected` 兜底。
- **额外权限只能加不能减**：从用户身上撤掉组已给的权限「没有效果」（并集语义），接口文档要写清，避免管理员误以为撤权成功。
- **首个 owner 只能手工产生**：迁移把存量用户全部回填为「普通用户」，而规则 3 只有 owner 能产生 owner——首个 owner 由运维在库里手工指定（`UPDATE users SET group_id = '<owner 组 UUID>' WHERE ...`），README 记录该步骤；「首次建号入口」仍留 §15 插槽。
- **在途请求使用请求开始时的权限快照**：并发降权时，已在处理中的请求仍以旧身份完成；改动对下一个请求生效。这是「不吊销会话 + 每请求解析」的固有语义，不是缺陷。
- **改组的并发写是「后写者获胜」**：两个管理者同时 PATCH 同一个组，后提交的全量替换覆盖前者的（没有版本号 / 乐观锁，博客规模下接受）。
- **`GET /admin/permissions` 要求 `system.groups.view` 是有意选择**：只持 `system.users.edit` 的人能改用户的额外权限但拿不到权限目录（也就列不出可选点），属可接受的保守取舍。

## 14. 验收标准

1. `scripts/db-generate.sh` 重生成后 `go build ./... && go vet ./... && gofmt -l .` 干净；生成代码已提交。
2. 迁移 SQL 人工审阅、`atlas migrate hash` / `validate` 通过；应用到两个库。
3. `go test ./...` 全绿（无库环境集成测试 skip；有库环境真跑并通过）。
4. 迁移后置断言通过：三个预设组存在且与 rbac 常量一致。
5. 手工 curl（前置：按 §13 用 SQL 把测试账号手工设为 owner）：普通用户 403、admin/owner 200、`/auth/me` 带 group 与 permissions、预设组改动被拒、非空组删除被拒、owner 组的成员只有 owner 能改。
6. README 更新（目录对照表、硬性规则补充、数据库章节、接口表、守卫原则变更、首个 owner 手工指定步骤）；DEVLOG 加条目；版本 `./scripts/bump-version.sh minor` → 0.4.0（按执行时的当前版本顺延）。

## 15. 插槽（本轮不定，定它的时点）

| 插槽 | 什么时候必须定 | 影响面 |
| --- | --- | --- |
| 首次建号入口（含首个 owner 的产生） | 与层 1 遗留项一起，下一轮 | auth / access 模块、运维脚本 |
| 控制台界面（用户列表左权限右组） | 下一轮（前端） | 接口消费方，不改本设计 |
| 会话吊销（改组后强制下线） | 出现「降权不生效」的实际痛点时 | scs 会话遍历或会话索引 |
| 权限缓存 | 查询量成为实际瓶颈时 | guard / access service |
| 审计留痕（谁改了谁的组 / 权限 diff） | 需要合规链时 | 新表 + 写路径 |
| 多组 / 组继承 | 出现真实需求时（本轮明确不做） | 数据模型重做 |
| 用户禁用 | 出现停用账号需求时 | 保护规则口径（参照 HaoBlog：认定与早退不同） |

## 16. 下一步

本文经用户审阅通过后，进 writing-plans 出实施计划（范围 = §3，验收 = §14）。
