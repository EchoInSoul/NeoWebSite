# NeoBlog 后端 API 设计规范（对标 OpenAI 公开 API 风格）

日期：2026-10-11
范围：`backend/`（Go + Gin，前缀 `/api/v1`，ent + Atlas，scs session 认证）；只约束 HTTP 表达层，不改业务语义
状态：冻结决策 D1–D11（含 D10 裁剪清单）经用户逐项确认（D1 于 v1.2 按用户指示修订）；本文为规范存档版，落地计划其后进 writing-plans；附录对照表已并入，定稿决策与遗留项见 §14

修订记录：
- v1（2026-10-11，本文）：首版。基于 OpenAI 公开 API 蓝本调研结论与后端现状核查（auth / access / health / guard / rbac 源码逐文件核对）撰写；所有冻结决策照录，个别与后端现实的衔接点在 §14 待定项标注。
- v1.1（2026-10-11）：并入端点规范化对照表（附录，含错误归类裁决与收敛点清单）；定稿 §14 决策 1–6（探针豁免与字段改名、分页一并接入、`invalid_credentials` 拆分、`not_found` 双语义拆分、`preset_group_locked` 改判 409、认证响应补 `id`）；正文 §4、§5、§7、§9、§10、§12 相应修订。
- v1.2（2026-10-11）：按用户指示修订 D1——去除 v2 升级机制，全部接口与后续演进固定在 `/api/v1` 内，本规范的规范化改造直接在 v1 执行；§2 重写版本化规则，§9、§14 同步措辞。同批复查修正：§7.3 收编口径表述、附录排序备注与交叉引用、状态行决策计数。
- v1.3（2026-10-11）：用户重新裁定实施范围并否决报错文案——新增 §0 范围裁定（效力高于正文冲突处）：本轮只实施错误结构（无 message 字段）与三处小修；分页、时间戳、补 id、`/healthy` 改造、`not_found`/`current_password_mismatch` 拆分、param 端点覆盖暂不实施。
- v1.4（2026-10-12）：用户改判 §4 规则 2——启用 Gin 的 `HandleMethodNotAllowed`，方法不匹配由「并入 404」改为 405 / `method_not_allowed` / `invalid_request_error`（响应带 `Allow` 头），路径不存在仍 404。§4 状态码清单、§7.2、§7.3 同步；实现随 0.11.0 入库。
- v1.5（2026-10-14）：用户裁定认证域路径整体改名——`/auth/*` 前缀取消，登录/登出/二步验证改挂会话资源，注册与自服务统一挂 `/users`：注册→`POST /users`、登录→`POST /sessions`、登出→`DELETE /sessions/current`、二步验证→`POST /sessions/current/verify`、`GET /auth/me`→`GET /users/me`、改密→`POST /users/me/password`、2FA 管理→`POST /users/me/2fa/*`。§3 规则 3 示例、D11 与端点表、§4 无负载体示例同步；实现见 0.16.0（后端 + 前端 4 个调用点）。§11 及以后与附录为历史批次存档，保留旧路径原文不改写。

## 0. 实施范围裁定（v1.3，2026-10-11，用户裁定）

经用户裁定，本轮落地**只实施以下两块**，正文任何与之冲突的表述以本节为准：

1. **错误结构（D3 收窄）**：`{"error":"<token>"}` → `{"error":{"type":...,"code":...,"param":...}}`。错误码 1:1 沿用现有 token，映射关系除 `preset_group_locked` 由 403 改判 409 外**全部维持现状**（含改密旧密码错仍为 401 `invalid_credentials`、畸形 UUID 仍 404 `not_found`）。**错误对象不设 message 字段**——任何报错文案不由本规范产出、不由实现写入。错误映射收敛到集中注册表 `internal/common/apierror`；guard 中间件错误（401/403/500）与未匹配路由的 404 兜底同属错误结构范畴，一并实施。
2. **三处小修**：`currentPassword`/`newPassword` → `current_password`/`new_password`（§5）；logout / changePassword 的 204 → 200 + `{}`、deleteGroup 的 204 → 200 + `{"id":"...","deleted":true}`（§4）；`preset_group_locked` 403 → 409（§7.3）。

本轮**不实施**（正文相应章节保留为将来参考，落地前需用户重新点名）：列表信封与分页（§9、D5）；响应补时间戳与认证响应补 id（D7 响应侧、§12 第 11 条、§14 决策 6）；`/healthy` 改造（§14 决策 1）；`not_found` 双语义拆分（§14 决策 4）；`current_password_mismatch` 拆分（§14 决策 3）；`weak_password`/`invalid_permission` 的端点级 param 覆盖（注册表默认 param 保留：`email_taken`=email、`nickname_taken`=nickname、`group_name_taken`=name、`password_unchanged`=new_password，其余为 null）。

## 1. 背景与目标

NeoBlog 后端已有一套能用的 API（认证层 1/2、用户组与权限、健康检查），但 HTTP 表达是各模块就地决定的：错误形状是扁平的 `{"error":"token"}`（`backend/internal/modules/auth/dto.go:43-45`、`backend/internal/modules/access/controller.go:14-16`），列表用 `items` 包裹（`backend/internal/modules/access/controller.go:18-20`），删除与登出返回 204 空体，字段命名残留 camelCase。这些约定散在 controller 里，没有一份成文规范，新模块只能靠模仿。

目标：给后端定一套**成文、集中、对标 OpenAI 公开 API** 的设计规范，覆盖 URL、方法与状态码、字段命名、响应信封、错误结构、认证表达、分页、时间与 ID。规范落地的杠杆点是两处收敛：

- 错误映射从各 controller 的本地 switch（`backend/internal/modules/access/controller.go:187-216`）收敛到共享包 `internal/common/apierror`；
- 列表信封从 `items` 收敛为 `data + total/limit/offset`。

对标对象是 OpenAI 公开 API 的**风格**（资源化 URL、snake_case、顶层 error 对象、裸资源响应、epoch 秒时间戳、确认体删除），不是照抄：OpenAI 的 object 自描述字段、ID 前缀、Bearer key、限流头、Idempotency-Key、SSE、游标分页在 §11 逐项裁剪并说明理由。

适用范围：

- `backend/` 全部 HTTP 端点（现有 14 个 + 以后的全部新端点）；
- guard 中间件产生的错误响应（401 / 403 / 500）；
- 健康检查端点的负载形状（探针豁免错误信封，见 §4 规则 5 与 §14 决策 1）。

不适用：前端消费逻辑、RBAC 目录与蕴含规则（本规范只在 §8 约定其 HTTP 表达，不重定义）、数据库 schema。

对齐来源（不重新发明）：

- OpenAI API 蓝本调研结论（2026-10-11，本文各节标注「对标」处）；
- 本仓库既有设计：`docs/superpowers/specs/2026-10-11-neoblog-auth-design.md`（层 1）、`2026-10-11-neoblog-access-design.md`（层 2）、`2026-10-11-neoblog-mail-design.md`；
- `backend/README.md` 硬性规则 1–8 与「认证」「用户组与权限」章节（本规范与其冲突处已在 §14 标注，README 待落地时同步）。

## 2. 总体风格与版本化

**D1（冻结，v1.2 修订）**：全部接口固定挂在 `/api/v1`，只在 v1 基础上演进；不设 v2，本规范的全部规范化改造直接在 v1 内执行。

规则：

1. 全部端点挂在 `/api/v1` 前缀下，路径版本而非 header（对标 OpenAI 的 `/v1/...`）。前缀在 `internal/app/app.go:26` 以常量 `apiPrefix` 定义一次（README 硬性规则 4），任何模块不得自行拼版本段。
2. **v1 是唯一版本**：不设 v2、不预留 `/api/v2` 升级路径。字段改名、字段类型或语义变更、删除字段或端点、状态码变更、请求参数语义收紧这类破坏性调整，只允许随本规范这类**显式的规范化批次**一次性执行（§12、附录对照表），日常迭代中一律禁止。
3. **v1 内允许的向后兼容增量**（规范化批次落地后的日常演进）：
   - 新增端点；
   - 响应新增字段（客户端必须容忍未知字段，见本节第 5 条）;
   - 新增错误 code（§7 词汇表**只增不改义**，与 rbac 权限目录同纪律）；
   - 新增可选查询参数；
   - 新增权限点（`internal/common/rbac` 目录逐条增加）。
4. 新端点一律按本规范设计；存量端点在 v1 内只做「规范化改造」（负载与错误格式对齐，路径原则上保留，见 D11），改造明细由附录对照表逐端点给出。
5. **客户端义务**：忽略未知字段；错误分支只依据 `error.code`（而非 `error.message` 或 HTTP body 的其他自然语言内容）。

正例（v1 内增量——给组对象加一个新字段，老客户端不受影响）：

```json
{
  "id": "01936b2f-7a1e-7d5a-9f3b-2c8e4a6d1b90",
  "name": "编辑",
  "is_preset": false,
  "member_count": 3,
  "permissions": [],
  "description": null
}
```

禁止（把 `member_count` 改名 `members`、或把 `permissions` 从数组改成对象——字段改名与类型/语义变更属破坏性调整，日常迭代一律禁止，见规则 2）。

## 3. URL 与路由命名规则

**D11（冻结，v1.5 修订）**：现有路由路径原则上保留（`/admin/groups` 等符合「资源名词 + 动作 POST」风格），本轮只规范化负载与错误格式；个别违规路径由附录对照表指出。v1.5 按用户裁定对认证域整体改名：`/auth/*` 前缀取消，登录/登出/二步验证改挂会话资源、注册与自服务改挂 `/users`——逐条见下表。

规则（约束新端点；存量端点的逐条裁决在附录）：

1. 路径一律小写；多词段用连字符 `-` 分隔（现状无多词路径，规则为将来预留）。
2. 集合用**名词复数**（对标 OpenAI 的 `/files`、`/batches`）；资源标识段是 UUID 字符串（`:id`，§10）。资源路径段禁止动词。
3. 动作类端点允许动词，两种合法形态：
   - 域内动作：`POST /<域>/<动作>`（v1.5 起本项目无此形态，规则为将来预留）；
   - 资源子动作：`POST /<集合>/:id/<动作>`（对标 OpenAI 的 `POST /batches/{batch_id}/cancel`；现状：`POST /sessions/current/verify` 二步验证、`POST /users/me/2fa/setup|enable|disable|recovery-codes`）。
4. 嵌套从属资源用 `/<父集合>/:id/<子集合>` 表达从属关系，不另造顶层路径。
5. 路由由各模块 controller 自己声明（README 硬性规则 4），本规范不改变模块化归属。

现状端点全集（供对照，本文不做逐条裁决；附录对照表负责「保留 / 调整」结论；**本表随每轮实现追加新端点并标注版本**——§13.1 的「表 A」是 2026-10-11 时点的规范化对照表、固定 14 行不追加）：

| # | 方法 路径 | 声明位置（随轮次更新） |
| --- | --- | --- |
| 1 | `GET /api/v1/healthy` | `backend/internal/modules/health/controller.go:18` |
| 2 | `POST /api/v1/sessions` | `backend/internal/modules/auth/controller.go:28`（v1.5 改名：登录 = 建会话，原 `POST /auth/login`） |
| 3 | `POST /api/v1/users` | `backend/internal/modules/auth/controller.go:27`（v1.5 改名：注册 = 建用户，原 `POST /auth/register`） |
| 4 | `DELETE /api/v1/sessions/current` | `backend/internal/modules/auth/controller.go:33`（v1.5 改名：登出 = 删当前会话，原 `POST /auth/logout`） |
| 5 | `GET /api/v1/users/me` | `backend/internal/modules/auth/controller.go:34`（v1.5 改名：原 `GET /auth/me`，与 `PATCH /users/me` 同一条资源） |
| 6 | `POST /api/v1/users/me/password` | `backend/internal/modules/auth/controller.go:38`（v1.5 改名：原 `POST /auth/password`） |
| 7 | `GET /api/v1/admin/permissions` | `backend/internal/modules/access/controller.go:46` |
| 8 | `GET /api/v1/admin/groups` | `backend/internal/modules/access/controller.go:47` |
| 9 | `POST /api/v1/admin/groups` | `backend/internal/modules/access/controller.go:48` |
| 10 | `PATCH /api/v1/admin/groups/:id` | `backend/internal/modules/access/controller.go:49` |
| 11 | `DELETE /api/v1/admin/groups/:id` | `backend/internal/modules/access/controller.go:50` |
| 12 | `GET /api/v1/admin/users` | `backend/internal/modules/access/controller.go:51` |
| 13 | `GET /api/v1/admin/users/:id` | `backend/internal/modules/access/controller.go:52` |
| 14 | `PATCH /api/v1/admin/users/:id` | `backend/internal/modules/access/controller.go:53` |
| 15 | `POST /api/v1/admin/mail/messages` | `backend/internal/modules/mail/controller.go:36`（0.9.0 新增，域 `admin` 下资源 `mail/messages`，创建即发送） |
| 16 | `POST /api/v1/sessions/current/verify` | `backend/internal/modules/auth/controller.go:29`（0.10.0 新增，公开路由，靠「待二段」会话；v1.5 改名：原 `POST /auth/2fa/verify`——二步验证 = 对当前（待二段）会话的验证） |
| 17 | `POST /api/v1/users/me/2fa/setup` | `backend/internal/modules/auth/controller.go:39`（0.10.0 新增，守卫内；v1.5 改名：原 `POST /auth/2fa/setup`） |
| 18 | `POST /api/v1/users/me/2fa/enable` | `backend/internal/modules/auth/controller.go:40`（0.10.0 新增，守卫内；v1.5 改名：原 `POST /auth/2fa/enable`） |
| 19 | `POST /api/v1/users/me/2fa/disable` | `backend/internal/modules/auth/controller.go:41`（0.10.0 新增，守卫内；v1.5 改名：原 `POST /auth/2fa/disable`） |
| 20 | `POST /api/v1/users/me/2fa/recovery-codes` | `backend/internal/modules/auth/controller.go:42`（0.10.0 新增，守卫内；资源名 `recovery-codes` 保持连字符小写；v1.5 改名：原 `POST /auth/2fa/recovery-codes`） |
| 21 | `GET /api/v1/admin/settings/:group` | `backend/internal/modules/settings/controller.go:33`（0.12.0 新增，守卫链 `RequireAuth → LoadIdentity → RequirePermission(system.settings.view)`；0.13.0 起返回组记录 `{group, values}`，`values` 为整组「默认值 ⊕ 覆盖」合并，含未标公开的项） |
| 22 | `PATCH /api/v1/admin/settings/:group` | `backend/internal/modules/settings/controller.go:34`（0.12.0 新增，`RequirePermission(system.settings.edit)`；只写请求体里出现的项，值类型以服务端清单为准；0.13.0 起返回更新后的组记录） |
| 23 | `GET /api/v1/public/settings` | `backend/internal/modules/settings/controller.go:38`（0.13.0 新增，**无鉴权**聚合读：只出清单里标了 `Public` 的项，形如 `{"items":[{"group":"register","values":{"enabled":false}}]}`；无公开项的组整条不出现；不做按组点查，前台一次取全） |
| 24 | `PATCH /api/v1/users/me` | `backend/internal/modules/auth/controller.go:35`（0.14.0 新增，改自己的用户名 / 邮箱：`/users/me` 是「当前用户」这条单资源的自指路径，声明在 auth 模块的 `RegisterProtected` 里，只挂 `RequireAuth + LoadIdentity`、**不挂权限点**；字段可选、部分更新；撞名复用注册的 `nickname_taken` / `email_taken`；**不校验当前密码**——用户 2026-10-13 裁定"不用"，留待将来按需加） |
| 25 | `GET /api/v1/users/me/sessions` | `backend/internal/modules/auth/controller.go:36`（0.15.0 新增，`/users/me` 下的嵌套从属集合；只挂 `RequireAuth + LoadIdentity`、**不挂权限点**；响应 `{"items":[…]}`，`id` 为 `base64url(sha256(token))` 会话指纹、原始 token 永不出服务端；只见活跃会话，待二段 / 已过期 / 已吊销的都不出现） |
| 26 | `DELETE /api/v1/users/me/sessions/:id` | `backend/internal/modules/auth/controller.go:37`（0.15.0 新增，按指纹吊销单条；未知 / 他人 / 二次删一律 404 `not_found`，与不存在同形；删当前会话等于登出，响应清 cookie） |

正例（新端点命名，资源复数 + 子动作）：

```
POST /api/v1/posts                       创建文章
GET  /api/v1/posts/:id                   读单篇
GET  /api/v1/posts                       文章列表
POST /api/v1/posts/:id/publish           发布（资源子动作）
```

禁止（资源路径带动词、驼峰段、路径里再拼一层版本）：

```
POST /api/v1/createPost        禁止：动词 + 驼峰
GET  /api/v1/postList          禁止：驼峰 + 非复数
POST /api/v1/v1/posts          禁止：重复版本段
```

## 4. HTTP 方法与状态码约定

**D6（冻结）**：DELETE 资源返回 200 + `{"id":"...","deleted":true}` 确认体；动作类 POST（logout、改密码等）返回 200 + JSON 体（无有效负载时返回 `{}`），废除现有 204 空体用法。创建资源 201。

方法语义：

| 方法 | 用途 | 成功状态码 | 成功 body |
| --- | --- | --- | --- |
| `GET` | 读单资源 / 列表 | 200 | 裸对象 / 列表信封（§6） |
| `POST` | 创建资源 | 201 | 裸对象 |
| `POST` | 动作（登录、登出、改密等） | 200 | 有负载给负载，无负载给 `{}` |
| `PATCH` | 部分更新（指针可空字段，§5） | 200 | 更新后的裸对象 |
| `DELETE` | 删除资源 | 200 | 确认体 `{"id":"...","deleted":true}` |

状态码使用清单（本规范只使用以下状态码；500 之外的 5xx 一律归并为 500）：

| 状态码 | 场景 | 对应 error.type（§7） |
| --- | --- | --- |
| 200 | 读 / 动作 / 更新 / 删除成功 | — |
| 201 | 创建资源成功 | — |
| 400 | 请求形状或字段非法 | `invalid_request_error` |
| 401 | 未登录、会话失效、用户已删除、凭证错误 | `authentication_error` |
| 403 | 已登录但缺权限或触发保护规则 | `permission_error` |
| 404 | 资源不存在（含未匹配路由兜底） | `not_found_error` |
| 405 | 路径存在但该路径未注册此方法（响应带 `Allow` 头） | `invalid_request_error` |
| 409 | 与现状冲突（重名、非空、竞态、预设组锁定） | `conflict_error` |
| 429 | 触发限流（当前无产生点，词汇保留） | `rate_limit_error` |
| 500 | 服务器内部错误兜底 | `server_error` |
| 503 | 仅 `/healthy` 降级时（探针豁免，见规则 5） | — |

规则：

1. **废除 204**：不存在无 body 的成功响应，任何 2xx 都必须带 JSON body（至少 `{}`）。现状三处 204 待改：`backend/internal/modules/auth/controller.go:85`（logout）、`backend/internal/modules/auth/controller.go:158`（changePassword）、`backend/internal/modules/access/controller.go:126`（deleteGroup）。
2. 方法不匹配启用 Gin 的 `HandleMethodNotAllowed`（v1.4 改判，此前裁定为不启用）：路径存在但该路径未注册该方法 → 405 + `invalid_request_error` / `method_not_allowed`，`Allow` 头由 Gin 按 RFC 9110 列出该路径允许的方法；路径不存在仍走 `NoRoute` 兜底，404 + `not_found_error` / `not_found` 且不带 `Allow`。`NoMethod` 与 `NoRoute` 都在 `app.go` 装配、认证中间件之前生效，故匿名请求也能借 `Allow` 确认端点存在——用户知情后选定的取舍，代价与回退开关记在 `backend/README.md`。
3. 所有请求与响应 body 一律 `application/json`（UTF-8）；`ctx.JSON` 已满足，不需要额外头。
4. 401 响应不要求 `WWW-Authenticate` 头（session cookie 认证，无 challenge 语义；对标裁剪见 §11）。
5. **探针豁免**：`/healthy` 是机器探针端点，200 / 503 的 body 保持 `Status` 对象（`status` / `database` 等字段），不受 §7 错误信封约束；时间字段仍按 §10 规范化（§14 决策 1）。

正例（无有效负载的成功响应——`DELETE /api/v1/sessions/current`（登出））：

```
HTTP/1.1 200 OK
Content-Type: application/json

{}
```

禁止（现状，待废除——`backend/internal/modules/auth/controller.go:85`）：

```
HTTP/1.1 204 No Content
（空 body）
```

正例（DELETE 确认体——`DELETE /api/v1/admin/groups/:id`）：

```json
{
  "id": "01936b2f-7a1e-7d5a-9f3b-2c8e4a6d1b90",
  "deleted": true
}
```

正例（创建——`POST /api/v1/admin/groups`，现状已合规，`backend/internal/modules/access/controller.go:93`）：

```
HTTP/1.1 201 Created
```

## 5. JSON 字段约定

**D2（冻结）**：JSON 字段一律 snake_case（请求与响应），现有 camelCase 字段全部改名（如 `currentPassword`→`current_password`、`newPassword`→`new_password`）。

命名细则：

1. 一律小写 snake_case；连续缩写整体小写（`api_key`，不是 `apiKey` / `APIKey`）。
2. 后缀约定：时间戳 `_at`（`created_at` / `updated_at` / `deleted_at`）；计数 `_count`（`member_count`）；引用 `_id`（`group_id`、`user_id`）。
3. 前缀约定：布尔用 `is_` / `has_` / `can_`（现状 `is_preset` 合规）。
4. 时长秒数以 `_seconds` 结尾（如 `/healthy` 的 `uptime_seconds`，§10 规则 4）。
5. 集合字段（数组）为空时输出 `[]`，禁止输出 `null`；标量可空字段输出 `null`（现状：自建组 `system_key` 为 `null`，`backend/internal/modules/auth/controller.go:102-105`）。现状合规先例：`/users/me` 的 `permissions` 已做 nil→`[]` 处理（`backend/internal/modules/auth/controller.go:98-101`）。

唯一现存 camelCase 违例（全仓 grep 核实，2026-10-11 时点）：`backend/internal/modules/auth/dto.go:21-22`：

```go
CurrentPassword string `json:"currentPassword"`
NewPassword     string `json:"newPassword"`
```

正例（改名后）：

```json
{ "current_password": "old-pass-1a", "new_password": "new-pass-2b" }
```

禁止：

```json
{ "currentPassword": "old-pass-1a", "newPassword": "new-pass-2b" }
```

其余字段已合规（对照先例）：`backend/internal/modules/access/dto.go:9-24` 的 `system_key` / `is_preset` / `member_count` / `extra_permissions`；`backend/internal/modules/auth/dto.go:29-41` 的 `system_key` / `permissions`。

PATCH 指针可空字段（沿用 access 模块既有模式，引用为新端点的规范依据）：

1. PATCH 请求体的每个可选字段用**指针类型**接收：缺省与 `null` 同义，都表示「不修改」（Go 反序列化后均为 nil）。参照 `backend/internal/modules/access/controller.go:27-35` 的 `groupRequest` / `userRequest` 与 `backend/internal/modules/access/dto.go:31-39` 的 `UpdateGroupInput` / `UpdateUserInput`。
2. 至少给一个字段：全缺省的 PATCH → 400 `invalid_request`（现状 `backend/internal/modules/access/controller.go:106-108`、`:157-160`）。
3. 可空集合字段的「显式清空」用空数组表达：`extra_permissions: []` = 清空全部额外权限，`null` / 缺省 = 不修改（现状 `backend/internal/modules/access/controller.go:32-35`、README「用户组与权限」章节）。
4. 不可空字段的非法值 → 400 + `param` 指向该字段（现状：`group_id` 空串或畸形 UUID → 400，`backend/internal/modules/access/controller.go:161-165`）。

正例（PATCH 部分更新，只改组名不动权限）：

```json
{ "name": "新名字" }
```

禁止（PATCH 请求体里用 `null` 表达「清空集合」——清空必须用 `[]`）：

```json
{ "extra_permissions": null }
```

## 6. 响应格式

**D4（冻结）**：成功响应不加包装信封：单资源返回裸 JSON 对象；列表返回 `{"data":[...],"total":n,"limit":n,"offset":n}`（现有 `items` 字段改名 `data`）。

规则：

1. 单资源：直接返回资源对象本身，**不套一层** `{"user": {...}}`、`{"result": ...}` 之类的包装。现状合规先例：`POST /admin/groups` 直接返回组对象（`backend/internal/modules/access/controller.go:93`）、`/users/me` 直接返回用户对象。
2. 列表：一律用四字段信封 `data` / `total` / `limit` / `offset`；`data` 为资源对象数组（可为空数组），`total` 为过滤后的总数，`limit` / `offset` 回显生效值。现状违例：`itemList[T]{Items: ...}` 的 `items` 字段（`backend/internal/modules/access/controller.go:18-20`、`:62`、`:71`、`:135`），改名为 `data` 并补齐信封。
3. 裸对象与信封内都不加 `object` 自描述字段（§11）。
4. **设置组记录**（0.13.0）：一个设置组是资源，其表示固定为 `{ "group": "<组名>", "values": { 项名: 值 } }`——`group` 是资源自身的字段，不算 §11 的自描述包装，也不违反规则 1。它的集合（`GET /public/settings`）沿用现状 `items` 数组信封（与 `access` 的 `itemList` 同形），**待 D4 / §9 的列表收敛落地时与 `access` 一起改名为 `data` 并补齐 `total` / `limit` / `offset`**，届时本轮端点一并迁移，不留第二套形状。

正例（单资源——`GET /api/v1/admin/groups/:id` 形态）：

```json
{
  "id": "01936b2f-7a1e-7d5a-9f3b-2c8e4a6d1b90",
  "system_key": null,
  "name": "编辑",
  "is_preset": false,
  "member_count": 3,
  "permissions": ["system.users.view", "system.users.edit"]
}
```

正例（列表信封——`GET /api/v1/admin/users?limit=20&offset=0` 形态）：

```json
{
  "data": [
    {
      "id": "01936b2f-7a1e-7d5a-9f3b-2c8e4a6d1b90",
      "email": "owner@example.com",
      "nickname": "owner",
      "group": {
        "id": "00000000-0000-4000-8000-000000000001",
        "system_key": "owner",
        "name": "所有者"
      },
      "extra_permissions": []
    }
  ],
  "total": 42,
  "limit": 20,
  "offset": 0
}
```

禁止（现状形状，待废除）：

```json
{ "items": [ ... ] }
```

禁止（包装信封）：

```json
{ "data": { "user": { "email": "..." } }, "success": true }
```

## 7. 错误格式

**D3（冻结）**：HTTP 状态码照常使用，body 统一为 `{"error":{"type":"...","message":"...","code":"...","param":null}}`。type 采用 OpenAI 分类并补充两类；code 沿用现有 snake_case 错误 token 词汇；message 为中文、仅供人读，不得作为客户端判断依据；param 指向出错的 JSON 字段路径或 null。错误映射必须收敛到集中的共享包（`internal/common/apierror`），废除各 controller 本地的 writeError/switch。

### 7.1 完整结构

```json
{
  "error": {
    "type": "conflict_error",
    "message": "该邮箱已被使用",
    "code": "email_taken",
    "param": "email"
  }
}
```

四个字段的口径：

- `type`：错误大类，由 HTTP 状态码唯一决定（见 7.2），客户端按它做粗分类。
- `code`：机器可判的 snake_case token，客户端分支**只**依据它。词汇表收敛于 7.3，只增不改义。
- `message`：中文、仅供人读（如直接展示给用户）；同一种 code 的 message 允许在不发版的情况下润色，客户端禁止解析它。
- `param`：出错位置的定位符。请求体字段用字段名，嵌套用点路径（如 `group.id`）；查询参数与路径参数用参数名（如 `limit`、`id`）；无法定位（如登录凭证错误、权限不足）为 `null`。

正例（字段级错误——`POST /api/v1/users` 邮箱已注册）：

```json
{
  "error": {
    "type": "conflict_error",
    "message": "该邮箱已被使用",
    "code": "email_taken",
    "param": "email"
  }
}
```

正例（中间件产生的错误——未登录访问受保护端点，guard 写出）：

```json
{
  "error": {
    "type": "authentication_error",
    "message": "请先登录",
    "code": "unauthorized",
    "param": null
  }
}
```

禁止（现状扁平形状，待废除——`backend/internal/modules/auth/dto.go:43-45`、`backend/internal/common/guard/guard.go:16` 等 6 处）：

```json
{ "error": "email_taken" }
```

禁止（客户端据 message 分支、或据 type 之外的顶层字段分支）。

### 7.2 type 分类表（状态码 → type 唯一映射）

| type | HTTP 状态码 | 说明 |
| --- | --- | --- |
| `invalid_request_error` | 400 / 405 | 请求体形状、字段值、查询参数非法；405 为方法不匹配（v1.4 改判） |
| `authentication_error` | 401 | 未登录、会话失效、凭证错误 |
| `permission_error` | 403 | 已登录但缺权限点或触发保护规则 |
| `not_found_error` | 404 | 资源不存在、路由未匹配（方法不匹配自 v1.4 起为 405，不再并入 404） |
| `conflict_error` | 409 | 与当前状态冲突（重名、非空、竞态、预设组锁定、保护计数） |
| `rate_limit_error` | 429 | 触发限流（**词汇保留**，当前无产生点，见 §11） |
| `server_error` | >=500 | 服务器内部错误（503 仅 `/healthy` 降级，探针豁免错误信封，§4 规则 5） |

### 7.3 code 词汇表（收编现有 token；此后只增不改义）

19 个现有 token 全量收编（来源：`backend/internal/modules/auth/service.go:15-24`、`backend/internal/modules/access/service.go:18-29`、guard 的 3 个字面量 token），另新增 `current_password_mismatch`（由 `invalid_credentials` 拆分而来，§14 决策 3），并将 `preset_group_locked` 由 403 / `permission_error` 改判为 409 / `conflict_error`（§14 决策 5）；0.9.0 起新增 `mail_delivery_failed`（管理端发信自检，走注册表登记）；0.10.0 起新增两步验证 6 个 token（`two_factor_*`：三个 401 / `authentication_error` + 三个 409 / `conflict_error`）；0.11.0 起新增 `method_not_allowed`（方法不匹配，405 改判见 §4 规则 2）；0.12.0 起新增 `registration_closed`（注册开关关闭，见设置 spec 与本轮 spec）；`param` 列为注册表默认值，端点可覆盖，如 `weak_password` 在注册时指 `password`、改密时指 `new_password`，设置端点 `PATCH` 的项名与类型错误覆盖为出错项名：

| code | HTTP | type | 默认 param | 场景 | 建议默认 message（注册表基线，可润色） |
| --- | --- | --- | --- | --- | --- |
| `invalid_request` | 400 | `invalid_request_error` | 按字段定位，定位不到为 null | 请求体形状 / 字段值非法（含路径参数与 UUID 格式非法）/ 全缺省 PATCH | 请求格式不正确 |
| `weak_password` | 400 | `invalid_request_error` | 按端点（`password` / `new_password`） | 密码强度不足 | 密码强度不足 |
| `password_unchanged` | 400 | `invalid_request_error` | `new_password` | 新旧密码相同 | 新密码不能与当前密码相同 |
| `invalid_permission` | 400 | `invalid_request_error` | `permissions` / `extra_permissions` | 权限 key 不在目录 | 包含未知的权限项 |
| `method_not_allowed` | 405 | `invalid_request_error` | null | 路径存在但该路径未注册此方法（响应带 `Allow`；路径不存在仍用 `not_found`） | 请求方法不被支持 |
| `invalid_credentials` | 401 | `authentication_error` | null | 登录凭证错误；改密的当前密码确认、2FA disable 与 recovery-codes 的密码确认也用它（`current_password_mismatch` 拆分按 §0 v1.3 暂不实施，见下） | 账号或密码不正确 |
| `current_password_mismatch` | 400 | `invalid_request_error` | `current_password` | 改密时当前密码不匹配（用户已认证，仅请求值错误） | 当前密码不正确 |
| `unauthorized` | 401 | `authentication_error` | null | 未登录 / 会话失效 / 用户已删除 / 无「待二段」会话却调 verify | 请先登录 |
| `two_factor_required` | 401 | `authentication_error` | null | 密码已对，等待二段验证（`two_factor_invalid_code` 之外唯一让前端跳验证码界面的 401；客户端按 code 分叉而非状态码） | 需要两步验证码 |
| `two_factor_invalid_code` | 401 | `authentication_error` | `code` | 验证码或恢复码不对（enable 与 verify 共用；同窗重放、认领竞败也用它） | 验证码不正确 |
| `two_factor_locked` | 401 | `authentication_error` | null | 二段连错 5 次，会话已销毁 | 验证码错误次数过多，请重新登录 |
| `forbidden` | 403 | `permission_error` | null | 缺少所需权限点 | 没有执行该操作的权限 |
| `grant_forbidden` | 403 | `permission_error` | null | 授出操作者自己没有的权限 | 不能授出自己不持有的权限 |
| `owner_protected` | 403 | `permission_error` | null | 非 owner 改动 owner / admin 组成员 | 该成员只能由所有者修改 |
| `cannot_modify_self` | 403 | `permission_error` | null | 改自己的组或额外权限 | 不能修改自己的组或额外权限 |
| `registration_closed` | 403 | `permission_error` | null | 注册开关（设置项 `register/enabled`）已关闭时提交注册；判定在字段校验之前，关闭态下连非法字段也返回本码而非 `invalid_request`（0.12.0 新增） | 当前已关闭注册 |
| `preset_group_locked` | 409 | `conflict_error` | null | 改 / 删预设组的锁死字段（锁来自资源状态，owner 亦被拒；现状 403，改判见 §14 决策 5） | 预设组不可修改 |
| `not_found` | 404 | `not_found_error` | null | 资源不存在（含路由未匹配兜底；方法不匹配自 v1.4 起归 `method_not_allowed`；路径 / 字段值**格式**非法归 `invalid_request`，§14 决策 4） | 资源不存在 |
| `email_taken` | 409 | `conflict_error` | `email` | 邮箱已注册 | 该邮箱已被使用 |
| `nickname_taken` | 409 | `conflict_error` | `nickname` | 昵称已占用 | 该昵称已被使用 |
| `already_taken` | 409 | `conflict_error` | null | 注册并发竞态兜底 | 该邮箱或昵称已被使用 |
| `group_name_taken` | 409 | `conflict_error` | `name` | 组名重复 | 该组名已被使用 |
| `group_not_empty` | 409 | `conflict_error` | null | 删有成员的自建组 | 组内仍有成员，无法删除 |
| `last_owner` | 409 | `conflict_error` | null | 移出最后一个 owner | 至少需要保留一名所有者 |
| `two_factor_already_enabled` | 409 | `conflict_error` | null | 已启用还 setup / enable（必须先 disable） | 两步验证已启用 |
| `two_factor_not_setup` | 409 | `conflict_error` | null | 没有 secret 就 enable / disable | 尚未生成两步验证密钥 |
| `two_factor_not_enabled` | 409 | `conflict_error` | null | 未启用就再生成恢复码 | 两步验证尚未启用 |
| `mail_delivery_failed` | 500 | `server_error` | null | SMTP 投递失败（管理端发信自检；不回传对端应答细节，0.9.0 新增） | 邮件发送失败 |
| `internal_error` | 500 | `server_error` | null | 一切未知错误兜底 | 服务器内部错误 |

规则：

1. 未知错误一律映射为 500 / `internal_error`，**不得把内部错误细节（SQL、栈）泄漏进 message**。
2. 新增 code 走注册表登记（HTTP + type + 默认 param + 默认 message）；每个 code 在注册表中唯一对应一组 HTTP 状态码与 type——同一 code 不得跨 type（§14 决策 3 的依据）。本轮规范化之后，词汇表只增不改义（§2）。
3. 保护规则类 403（`grant_forbidden` 等）不披露「操作者缺哪个权限点」——安全考虑，param 恒为 null。

### 7.4 集中映射：`internal/common/apierror`

约定（包名即冻结决策建议名）：

1. **哨兵错误与元数据集中**：`internal/common/apierror` 定义上表全部哨兵（对齐现有 token，如 `apierror.ErrEmailTaken`）及其元数据（HTTP 状态、type、code、默认 message、默认 param）。业务模块 service 直接返回 apierror 哨兵（替代现状 `auth` / `access` 各自的哨兵列表）；若落地时希望 service 对 API 词汇零依赖，可退为「模块哨兵 + apierror 注册映射」，二选一，落地计划定，词汇表只有一份。
2. **统一写出**：apierror 提供 `Write(ctx *gin.Context, err error)`（供 controller 与 guard 调用）与纯数据的 `Body(err)`（不 import gin，供测试与非 HTTP 场景）。controller 删除本地 `writeError` / `writeInvalidRequest` / `ErrorResponse` / `errorResponse`（`backend/internal/modules/auth/controller.go:34-153` 各 switch、`backend/internal/modules/access/controller.go:14-16`、`:187-216`），错误路径收敛为一行 `apierror.Write(ctx, err)`。
3. **guard 同样走该结构**：`backend/internal/common/guard/guard.go:16`、`backend/internal/common/guard/permission.go:32`、`:40`、`:43`、`:56`、`:65` 共 6 处 `gin.H{"error": ...}` 全部换为 apierror 输出（401 `unauthorized`、403 `forbidden`、500 `internal_error`）。
4. **param 覆盖**：注册表存默认 param，端点需要动态定位时用包装函数覆盖（如注册与改密共用 `weak_password` 但 param 不同）。
5. **README 同步**：硬性规则 3 的「Gin 与 HTTP 细节允许出现位置」清单需补 `internal/common/apierror`（落地时改）。

## 8. 认证与权限

**D9（冻结）**：认证维持 scs session cookie + `guard.RequireAuth` / `LoadIdentity` / `RequirePermission` 链；Bearer / API key 不做。

本节只规定认证与权限的 **HTTP 表达**，不重定义 RBAC（目录、蕴含、保护规则见 `docs/superpowers/specs/2026-10-11-neoblog-access-design.md` 与 `internal/common/rbac`）：

1. **凭证**：会话由 scs 管理、存 Postgres `sessions` 表；cookie 名 `session`、HttpOnly、SameSite=Lax、`Secure` 仅 production（`backend/internal/app/app.go:79-89`）。登录与注册成功换发会话 token。会话的写回（`Set-Cookie`）由 scs 的 `LoadAndSave` 中间件负责（`backend/internal/app/app.go:64`），与本规范的响应体约定互不干扰。
2. **守卫链**：受保护路由挂 `RequireAuth → LoadIdentity → RequirePermission`（`backend/internal/app/app.go:60`）。三者的失败点与错误表达：

| 失败点 | 场景 | HTTP | code | 声明位置（现状） |
| --- | --- | --- | --- | --- |
| `RequireAuth` | 会话无 `auth.user_id` | 401 | `unauthorized` | `backend/internal/common/guard/guard.go:12-22` |
| `LoadIdentity` | 用户已不存在（销毁会话后拒绝） | 401 | `unauthorized` | `backend/internal/common/guard/permission.go:36-42` |
| `LoadIdentity` | resolver 其他错误 | 500 | `internal_error` | `backend/internal/common/guard/permission.go:43` |
| `RequirePermission` | 身份缺失（装配错误） | 500 | `internal_error` | `backend/internal/common/guard/permission.go:52-58` |
| `RequirePermission` | 权限点不满足 | 403 | `forbidden` | `backend/internal/common/guard/permission.go:59-66` |

   以上 6 处现状均为扁平 `gin.H{"error": "..."}`，统一改走 `apierror.Write`（§7.4）。
3. **身份上下文**：gin context 键 `auth.user_id`（`guard.SessionUserIDKey`）与 `auth.identity`（`guard.SessionIdentityKey`）为内部装配约定，不出现在任何请求或响应 JSON 里；`/auth/me` 的 group / permissions 从 `auth.identity` 取（现状 `backend/internal/modules/auth/controller.go:88-123`）。
4. **权限点字符串**：`模块.动作`（`system.users.view` 等）由 `internal/common/rbac` 唯一定义（`backend/internal/common/rbac/rbac.go:9-14`），请求与响应里的权限数组原样输出这些字符串；本规范不新增、不改名任何权限点。
5. **不做 Bearer / API key**：唯一消费方是同源浏览器前端，cookie 已覆盖；无第三方集成场景（裁剪理由见 §11）。

## 9. 分页约定

**D5（冻结）**：分页参数 `limit`（默认 20，上限 100）+ `offset`（默认 0）；`total` 必返。游标分页只作为「保留扩展」提及，不实现。

规则：

1. 参数为查询串整数：`?limit=20&offset=0`；缺省取默认值（`limit=20`、`offset=0`）。
2. **非法值**（负数、非整数、溢出）→ 400 `invalid_request_error` / `invalid_request`，`param` 指向对应参数名（`limit` 或 `offset`）。
3. **超上限钳制**：`limit` 超过 100 时钳制到 100，响应信封的 `limit` 回显生效值（不报错）。
4. `total` = 当前过滤条件下的总数，不受 `limit` / `offset` 影响；列表信封四字段（`data` / `total` / `limit` / `offset`）恒齐。
5. **现状无分页参数的列表端点**（`GET /admin/groups`、`GET /admin/users`、`GET /admin/permissions`）：信封与 `total` 必须补齐（D5 的硬要求），且三个端点在本轮规范化中**一并接入** `limit` / `offset`（`/admin/permissions` 为静态目录，对目录切片即可）；逐端点明细见附录表 A。
6. **排序**：分页端点必须声明默认排序并保证稳定；默认 `created_at` 倒序（对齐 access 用户列表现状）。本轮不提供排序参数。
7. 空列表返回 `"data": []` + `"total": 0`，不是 404、不是 `null`。

正例（请求与响应）：

```
GET /api/v1/admin/users?limit=20&offset=40

{
  "data": [ ... ],
  "total": 42,
  "limit": 20,
  "offset": 40
}
```

禁止（负数 offset、`limit=0`、超上限报错之外的处置——负数与非整数必须 400）。

游标分页（**保留扩展，本轮不实现**）：出现深分页需求时，以新增可选查询参数（`after` / `before` 或 `cursor`）+ 信封附加字段（`next_cursor` / `has_more`）的方式扩展——新增字段与可选参数属 §2 允许的 v1 内向后兼容增量，届时直接在 v1 上扩展。触发条件：单个列表的总量或翻页深度造成 offset 扫描成为实际瓶颈。

## 10. 时间与 ID

**D7（冻结）**：时间戳一律 unix epoch 秒整数（未发生的用 null）；不再用 RFC3339 或 Go 时长字符串。

**D8（冻结）**：ID 为 UUIDv7 字符串，不加类型前缀；不引入 OpenAI 的 object 自描述字段。

时间规则：

1. 响应时间戳字段以 `_at` 结尾，值为 epoch 秒整数（Go 侧 `time.Time.Unix()`），UTC 语义，不做本地化字符串。
2. 「未发生」用 `null`（如将来的 `deleted_at`——资源未被删除时）。
3. 请求侧当前不接收任何时间字段；将来接收时同样用 epoch 秒整数。
4. 禁止 RFC3339 字符串、Go `time.Duration` 字符串出现在 JSON 里。**现状违例**：`/healthy` 的 `time`（RFC3339）与 `uptime`（Go 时长字符串 `"3s"`），`backend/internal/modules/health/service.go:40-41`——按 D7 一并规范化：`time` → `reported_at`（epoch 秒）、`uptime` → `uptime_seconds`（整数秒，§14 决策 1）。

正例：

```json
{ "created_at": 1760123456, "deleted_at": null }
```

禁止：

```json
{ "created_at": "2026-10-11T01:35:12+08:00", "uptime": "3s" }
```

ID 规则：

1. 资源 ID 是 UUIDv7 字符串；生成侧已合规——`users.id` 用 `uuid.NewV7()`（`backend/internal/database/ent/schema/user.go:22`），新表 schema 沿用。
2. 不加 OpenAI 式类型前缀（禁止 `user_0193...`）；不在资源对象里加 `object` 字段。
3. ID 在代码与客户端里按字符串比较，不解析内部位。
4. **预设组固定 UUID 例外**：`internal/common/rbac/rbac.go:23-25` 的三个固定 UUID（`…0001` / `…0002` / `…0003`）是配置常量而非生成 ID，格式为 v4 形态，保持不变（迁移 seed 与代码常量的一致性由既有集成测试兜底）。

正例：

```json
{ "id": "01936b2f-7a1e-7d5a-9f3b-2c8e4a6d1b90", "group_id": "00000000-0000-4000-8000-000000000001" }
```

禁止：

```json
{ "id": "user_01936b2f-7a1e-7d5a-9f3b-2c8e4a6d1b90", "object": "user" }
```

## 11. 明确裁剪项与理由

**D10（冻结）**：以下 OpenAI 机制明确裁剪并说明理由。

| # | 项 | OpenAI 做法 | 本规范决定 | 理由 |
| --- | --- | --- | --- | --- |
| 1 | 限流响应头 | 429 附 `x-ratelimit-*`、`retry-after` 头 | 裁剪；`rate_limit_error` / 429 词汇保留 | 后端尚无限流实现（README「认证」章节已知缺口：登录注册暂无限流）；限流落地时再一并定头字段，届时属 v1 内增量 |
| 2 | Idempotency-Key | 写操作幂等键（重放保护） | 裁剪 | 现有写操作（注册 / 登录 / 改密 / 组管理）低频且重复提交无资金类风险；引入需存储与并发去重，成本大于收益 |
| 3 | SSE 流式 | `stream=true`、`text/event-stream` | 裁剪 | 无生成式输出或服务端推送场景；出现时另立设计，不进本规范 |
| 4 | `object` 自描述字段 | 每个资源带 `"object": "chat.completion"` | 裁剪（D8） | 前端是强类型消费方，无动态反序列化需求；去掉后每个 DTO 少一个恒定字段与维护负担 |
| 5 | ID 类型前缀 | `chatcmpl-`、`file-` 等前缀 | 裁剪（D8） | UUIDv7 已具全局唯一性；前缀会进入 DB 主键 / 外键并占用存储，调试收益有限 |
| 6 | 游标分页 | `after` / `before` + `has_more` | 保留扩展，不实现（D5） | 博客数据规模下 `limit` / `offset` 足够；扩展路径已预留（§9「游标分页」段），属向后兼容增量 |
| 7 | Bearer / API key | `Authorization: Bearer sk-...` | 裁剪（D9） | 唯一消费方是同源浏览器前端，scs session cookie 已覆盖；无第三方 API 消费者，密钥管理面为零 |

另有两处「对标但不引入」：`WWW-Authenticate` 头（401 无 challenge 语义，§4）与列表的 `first_id` / `last_id` 字段（limit/offset 信封下无意义，将来上游标分页时随扩展一并评估）。

## 12. 落地改造清单摘要

以下为规范落地涉及的改造面汇总（**详细逐端点对照见附录，内容由端点调研路并入**）：

1. **新包 `internal/common/apierror`**：哨兵 + 注册表（7.3 全表）+ `Write` / `Body`（§7.4）；README 硬性规则 3 的 Gin 允许位置清单同步补该包。
2. **废除本地错误形状**：`auth/dto.go:43-45` 的 `ErrorResponse`、`auth/controller.go` 各错误 switch（`:34`、`:41`、`:44`、`:62-72`、`:98`、`:101`、`:143-153`）、`access/controller.go:14-16` 的 `errorResponse` 与 `:187-216` 的 `writeInvalidRequest` / `writeError`，收敛为 `apierror.Write`。
3. **guard 6 处** `gin.H{"error": ...}` 换 apierror（§8 第 2 条表）。
4. **字段改名**：`currentPassword` / `newPassword` → `current_password` / `new_password`（`backend/internal/modules/auth/dto.go:21-22`，全仓唯一 camelCase 违例）。
5. **列表信封与分页**：`items` → `data` + `total` / `limit` / `offset`（`backend/internal/modules/access/controller.go:18-20`、`:62`、`:71`、`:135`）；三个列表端点一并接入 `limit` / `offset`（§9、附录表 A）。
6. **废除 204**：logout（`backend/internal/modules/auth/controller.go:85`）与 changePassword（`:158`）→ 200 + `{}`；deleteGroup（`backend/internal/modules/access/controller.go:126`）→ 200 + 确认体（§4）。
7. **`/healthy` 规范化**：`time` → `reported_at`（epoch 秒）、`uptime` → `uptime_seconds`（整数秒），503 body 保持 `Status` 对象（探针豁免；`backend/internal/modules/health/service.go:40-41`、§14 决策 1）。
8. **404 JSON 兜底**：`app.go` 挂 `NoRoute` 输出标准错误结构（§4 第 2 条）。
9. **测试与文档同步**：受影响集成测试断言（错误形状、信封、状态码、字段名）全量更新；README 接口表（「认证」「用户组与权限」「运行」章节的健康检查示例）同步改写；版本按执行时当前版本进位 minor。
10. **service 哨兵迁移**：`auth/service.go:15-24`、`access/service.go:18-29` 的哨兵改为直接使用或映射到 apierror 注册表（二选一方案见 §7.4 第 1 条）。
11. **响应补字段（透传即可，ent 字段已就绪、无迁移）**：login / register / `/auth/me` 补 `id`（`/auth/me` 另补 `updated_at`）；`/admin/users`、`/admin/groups` 及其详情补 `created_at` / `updated_at`——需在 `auth/repository.go` 的 `toUser` 与 `access/repository.go` 的 `groupRecord` / `userRecord` 及相应 DTO 透传（附录表 A、§14 决策 6）。

## 13. 附录：现有端点规范化对照表

> 已由端点调研并入并定稿（v1.1）。含四部分：13.1 端点逐条对照、13.2 错误 token 出处与归类裁决、13.3 错误写出收敛点清单、13.4 前端受影响清单。file:line 为 2026-10-11 时点，落地时如行号漂移以符号名为准。

口径：`变更点` 从固定词表取值：`字段改名` / `信封` / `状态码` / `错误结构` / `时间戳` / `路径调整` / `无变更`；「错误结构」指改走 §7 错误信封（全部业务端点均涉及，`/healthy` 探针豁免见行 1；逐行不重复展开）。

### 13.1 表 A：端点规范化对照表

| # | 方法 路径 | 现状要点（file:line） | 规范化后 | 变更点 | 路径处置 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | `GET /healthy` | 200 / 503；`time`=RFC3339、`uptime`=时长串（health/service.go:40-41）；503 body 为 Status 对象，非错误信封（health/controller.go:24-27） | 200 / 503 保持；`time`→`reported_at`（epoch 秒）、`uptime`→`uptime_seconds`（整数秒）；503 body 保持 Status 对象 | 字段改名、时间戳 | 保留 | 探针豁免错误信封（§4 规则 5、§14 决策 1） |
| 2 | `POST /auth/login` | 请求 `{identifier, password}`（auth/dto.go:9-12）；响应 `{email, nickname}`（dto.go:25-28；controller.go:48） | 请求不变；响应补 `id`、`created_at` | 错误结构、时间戳 | 保留 | 补 `id` 以关联 /admin/users 条目（§14 决策 6） |
| 3 | `POST /auth/register` | 请求 `{email, nickname, password}`（dto.go:14-18）；201（controller.go:77） | 响应补 `id`、`created_at` | 错误结构、时间戳 | 保留 | — |
| 4 | `POST /auth/logout` | 204 空体（auth/controller.go:85） | 200 + `{}` | 状态码、错误结构 | 保留 | 废除 204（§4） |
| 5 | `GET /auth/me` | 裸 MeResponse `{email, nickname, group, permissions}`（dto.go:36-41；controller.go:113-122） | 字段不变；补 `id`、`created_at`、`updated_at` | 错误结构、时间戳 | 保留 | `toUser` 现丢弃 ent 时间字段，需透传（auth/repository.go:65-70） |
| 6 | `POST /auth/password` | 请求 `{currentPassword, newPassword}`（dto.go:20-23）；204（controller.go:158）；旧密码错复用 `invalid_credentials`（controller.go:145；service.go:131-133） | 请求改名 `current_password` / `new_password`；200 + `{}`；旧密码错拆为 `current_password_mismatch`（400） | 字段改名、状态码、错误结构 | 保留 | `invalid_credentials` 仅保留在登录（§14 决策 3） |
| 7 | `GET /admin/permissions` | `{"items":[{key,name}]}`，数据来自静态目录、恒 4 条（access/controller.go:62；rbac/rbac.go:43-48） | `{"data":[…],"total","limit","offset"}`；`limit`/`offset` 对目录切片，`total`=len(目录) | 信封、错误结构 | 保留 | 无 DB 查询；目录增长后走同一切片逻辑；无 `created_at`，排序为目录固定顺序（§9 规则 6 的声明例外） |
| 8 | `GET /admin/groups` | `{"items":[Group]}`，created_at 升序全量（access/controller.go:71；repository.go:105-109） | 信封 + `limit`/`offset`（默认 20/0，上限 100）；Group 补 `created_at`/`updated_at` | 信封、错误结构、时间戳 | 保留 | `total` 用 COUNT；record/DTO 需补时间字段（repository.go:17-21）；排序统一为 `created_at` 倒序（现状升序，随规范化调整，§9 规则 6） |
| 9 | `POST /admin/groups` | 201 + 裸 Group（controller.go:93） | 补 `created_at`/`updated_at` | 错误结构、时间戳 | 保留 | 创建路径现状已合规（裸对象、201） |
| 10 | `PATCH /admin/groups/:id` | 指针可空部分更新；200 + 裸 Group（controller.go:118）；畸形 `:id` 归 not_found（service.go:107-111） | 补 `created_at`/`updated_at`；`:id` 格式非法 → 400 `invalid_request`（param=`id`） | 错误结构、时间戳 | 保留 | 格式非法与不存在拆分（§14 决策 4） |
| 11 | `DELETE /admin/groups/:id` | 204 空体（controller.go:126）；`preset_group_locked` 现状 403（controller.go:212） | 200 + `{"id":"…","deleted":true}`；`preset_group_locked` → 409 `conflict_error` | 状态码、错误结构 | 保留 | 确认体（§4）；改判理由（§14 决策 5） |
| 12 | `GET /admin/users` | `{"items":[UserSummary]}`，created_at 降序全量（controller.go:135；repository.go:141-146） | 信封 + `limit`/`offset` + COUNT；UserSummary 补 `created_at` | 信封、错误结构、时间戳 | 保留 | 唯一增长型列表，分页收益最大；userRecord 需补时间字段（repository.go:23-28） |
| 13 | `GET /admin/users/:id` | 200 + 裸 UserDetail（=UserSummary+permissions）（dto.go:26-29；controller.go:144） | 补 `created_at`/`updated_at`；`:id` 同 #10 | 错误结构、时间戳 | 保留 | — |
| 14 | `PATCH /admin/users/:id` | 请求 `{group_id?, extra_permissions?}`（controller.go:32-35）；200 + 裸 UserDetail（controller.go:175） | 补 `created_at`/`updated_at`；`:id` 同 #10 | 错误结构、时间戳 | 保留 | 请求字段现状已 snake_case |

### 13.2 表 B：错误 token 出处与归类裁决

现状统一形态为扁平 `{"error":"<token>"}`；规范后形态与词汇表见 §7.3，本表只记录**出处**与**归类裁决**，不重复 message 基线。

| code | 现状出处（file:line） | 规范 HTTP / type | 裁决与说明 |
| --- | --- | --- | --- |
| `unauthorized` | guard/guard.go:16；guard/permission.go:32,40；auth/controller.go:98,135,143 | 401 / `authentication_error` | 无争议；写出走 apierror（13.3） |
| `invalid_credentials` | auth/controller.go:41（登录） | 401 / `authentication_error` | 保留于登录；改密处拆出为 `current_password_mismatch`（§14 决策 3） |
| `current_password_mismatch` | 新增，承接 auth/controller.go:145（触发于 service.go:131-133） | 400 / `invalid_request_error` | 用户已认证，仅请求值错误；param=`current_password` |
| `invalid_request` | auth/controller.go:34,54,64,128,151；access/controller.go:188,196 | 400 / `invalid_request_error` | 含路径 `:id` / UUID 格式非法（原归 not_found，§14 决策 4） |
| `weak_password` | auth/controller.go:62,147（触发于 service.go:51-53,127-129） | 400 / `invalid_request_error` | param 随端点：`password` / `new_password` |
| `password_unchanged` | auth/controller.go:149（触发于 service.go:135-137） | 400 / `invalid_request_error` | 维持 400：对请求值的约束，客户端改输入即可解决，不随资源状态 |
| `email_taken` | auth/controller.go:66（触发于 service.go:55-59） | 409 / `conflict_error` | param=`email` |
| `nickname_taken` | auth/controller.go:68（触发于 service.go:60-64） | 409 / `conflict_error` | param=`nickname` |
| `already_taken` | auth/controller.go:70（触发于 service.go:72-77 并发兜底） | 409 / `conflict_error` | 并发竞态兜底，param=null 合法（§7.1） |
| `not_found` | access/controller.go:194（触发于 service.go:19,107-111,163-165,202-206,236-239,251-255；repository.go:218-220,274-277） | 404 / `not_found_error` | 仅「引用对象不存在」；格式非法拆至 `invalid_request`（§14 决策 4） |
| `invalid_permission` | access/controller.go:198（触发于 service.go:386-400） | 400 / `invalid_request_error` | param=`permissions` / `extra_permissions` |
| `group_name_taken` | access/controller.go:200（触发于 repository.go:173-179,197-205） | 409 / `conflict_error` | param=`name` |
| `group_not_empty` | access/controller.go:202（触发于 service.go:174-180；repository.go:239-247） | 409 / `conflict_error` | — |
| `last_owner` | access/controller.go:204（触发于 service.go:304-311；repository.go:256-266） | 409 / `conflict_error` | — |
| `grant_forbidden` | access/controller.go:206（触发于 service.go:96-98,138-147,300-302,402-409） | 403 / `permission_error` | 取决于调用者权限边界而非资源状态 |
| `owner_protected` | access/controller.go:208（触发于 service.go:264-268） | 403 / `permission_error` | 非 owner 触碰特权组成员，属权限不足 |
| `cannot_modify_self` | access/controller.go:210（触发于 service.go:240-242） | 403 / `permission_error` | 动作授权规则，非状态冲突 |
| `preset_group_locked` | access/controller.go:212（触发于 service.go:116-118,171-173） | **409 / `conflict_error`（现状 403，改判）** | 锁定来自资源状态（`system_key` 非空），owner 亦被拒（§14 决策 5） |
| `forbidden` | guard/permission.go:65 | 403 / `permission_error` | — |
| `internal_error` | auth/controller.go:44,72,82,91,101,153；access/controller.go:181,214；guard/permission.go:43,56 | 500 / `server_error` | 兜底；param 恒 null，不泄漏内部细节 |
| `rate_limit_error` / 429 | （现状无产生点，全代码无 429 路径） | 429 / `rate_limit_error` | 词汇保留（§11）；限流落地时再新增 code 与响应头 |

### 13.3 表 C：错误写出收敛点清单

目标是 controller / guard 不再自行拼错误 JSON，全部经 `internal/common/apierror` 的 `Respond` / `Abort` 写出（§7.4）。全部替换点：

| 位置 | 现状 | 替换形态 |
| --- | --- | --- |
| guard/guard.go:16 | `AbortWithStatusJSON(401, gin.H{"error":"unauthorized"})` | `apierror.Abort(ctx, err)`（401 `unauthorized`） |
| guard/permission.go:32 | 同上（LoadIdentity 未登录） | 同上 |
| guard/permission.go:40 | 同上（身份解析时用户已消失，销毁会话） | 同上（销毁逻辑保留在中间件内） |
| guard/permission.go:43 | `AbortWithStatusJSON(500, gin.H{"error":"internal_error"})` | 同上（500 `internal_error`） |
| guard/permission.go:56 | 同上（IdentityFrom 失败） | 同上 |
| guard/permission.go:65 | `AbortWithStatusJSON(403, gin.H{"error":"forbidden"})` | 同上（403 `forbidden`） |
| auth/dto.go:43-45 | 重复的 `ErrorResponse` struct | 删除，改用 `apierror.Error` |
| auth/controller.go:34,54,128 | `ErrorResponse{Error:"invalid_request"}` 内联 | `apierror.Respond(ctx, err)`（bind 失败转 `invalid_request`） |
| auth/controller.go:40-45（login switch） | 逐 case JSON | 统一 `apierror.Respond(ctx, err)`，哨兵映射集中在注册表 |
| auth/controller.go:60-73（signup switch） | 6 个 case | 同上 |
| auth/controller.go:82（logout） | internal_error 内联 | 同上 |
| auth/controller.go:91,97-102（me） | unauthorized / internal_error | 同上 |
| auth/controller.go:128,135,141-154（changePassword） | 6 case；旧密码错 case 改挂新 code | 同上 |
| access/controller.go:14-16 | 重复的 `errorResponse` struct | 删除，改用 `apierror.Error` |
| access/controller.go:178-185（operatorFrom） | internal_error 内联 | 同上 |
| access/controller.go:187-189（writeInvalidRequest） | invalid_request 内联 | 删除，改 `apierror.Respond` |
| access/controller.go:191-216（writeError 大 switch，10 case） | 模块内错误映射 | 整体迁入 apierror 注册表（哨兵 `access.Err*` 自 service.go:18-29）；controller 只剩 `apierror.Respond(ctx, err)` |
| health/controller.go:24-27 | 非 ok 时 503 + 普通 Status JSON（非错误信封） | 保持探针语义不套信封（§4 规则 5、§14 决策 1） |

### 13.4 前端受影响清单（仅列出）

> 前端同步不在本规范范围（后端先行），落地时需与前端对齐的 breaking 面如下：

- 请求改名：`currentPassword`→`current_password`、`newPassword`→`new_password`（POST /auth/password）。
- 列表信封：三个 admin 列表 `items`→`data`，新增 `total`/`limit`/`offset` 响应字段与 `limit`/`offset` 请求参数。
- 错误格式全量变化：token 从 `error` 字符串移至 `error.code`；前端错误分支全部改依据 `error.code`（§2 客户端义务）。
- 204→200：logout、POST /auth/password、DELETE /admin/groups/:id（后者新增 `{"id","deleted":true}` 确认体）。
- 状态码变化：`preset_group_locked` 403→409（前端按状态码分支处需改）。
- /healthy：`time`→`reported_at`（epoch 秒）、`uptime`→`uptime_seconds`（整数秒）。
- 新增响应字段：login/register/me 的 `id`（me 另有 `created_at`/`updated_at`）；users/groups 及其详情的 `created_at`/`updated_at`。

## 14. 决策记录与遗留项

以下决策在 v1.1 并入对照表时定稿（正文各节已同步；落地前可推翻重议，推翻须同步修订正文对应节）：

1. **`/healthy` 探针豁免与字段改名**：200 / 503 的 body 保持 `Status` 对象，不受 §7 错误信封约束（§4 规则 5）；`time`→`reported_at`（epoch 秒）、`uptime`→`uptime_seconds`（整数秒）（§10 规则 4）。
2. **分页一并接入**：三个无分页列表端点本轮同时补齐信封、`total` 与 `limit`/`offset` 参数（§9 规则 5）；access 设计 §13 的「用户列表本轮不分页」由本规范取代。
3. **`invalid_credentials` 拆分**：登录保留原 code（401 / `authentication_error`）；改密旧密码错新增 `current_password_mismatch`（400 / `invalid_request_error` / param=`current_password`）——同一 code 不得跨 type（§7.3）。
4. **`not_found` 双语义拆分**：路径参数 / 字段值**格式**非法 → 400 `invalid_request`（param 指向该参数名）；引用对象不存在 → 404 `not_found`（§7.1、§7.3）。
5. **`preset_group_locked` 改判 409**：锁定来自资源状态（`system_key` 非空）而非调用者权限，owner 亦被拒，归 `conflict_error`（§7.3；现状 403，前端分支需同步，13.4）。
6. **认证响应补 `id`**：login / register / `/auth/me` 输出用户 `id`（me 另补 `created_at`/`updated_at`）；users / groups 响应补 `created_at`/`updated_at`（§12 第 11 条、附录表 A）。
7. **版本策略修订（v1.2，按用户指示）**：D1 改为 `/api/v1` 唯一版本，不设 v2、不预留升级路径；本规范全部规范化改造直接在 v1 内执行，破坏性调整仅随显式规范化批次发生（§2 规则 2）。

遗留（实现期事务，不阻塞本规范定稿）：

- §7.3「建议默认 message」中文文案逐条定稿（当前为注册表基线草案）。
- §7.4 第 1 条 apierror 哨兵方案二选一（service 直用哨兵 vs 模块哨兵 + 注册映射），由落地计划定。
- `rate_limit_error` / 429 无现状产生点，限流功能落地时再补 code 与响应头字段（§11）。
