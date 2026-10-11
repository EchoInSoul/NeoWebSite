# NeoBlog 认证设计（层 2：注册与更改密码）

日期：2026-10-11
范围：`backend/`（Go + Gin），**只做后端**；前端接线由用户侧自行完成，契约以本文档第 3 节为准
状态：设计经用户确认（2026-10-11 对话）——公开注册、不验证邮箱、**暂不做限流**；改密码会话语义采用推荐 B（用户未否决，审阅本文时可改）

## 1. 意图与背景

在层 1（登录 / 登出 / me + 守卫，见 `2026-10-11-neoblog-auth-design.md`）之上补两个流程：

- **公开注册**：把前端已有的注册页接上后端；注册成功即登录。
- **更改密码**：已登录状态下用旧密码换新密码。

用户已定：
- 注册**公开**、**不验证邮箱**（没有 SMTP；也不做邮件找回密码）。
- **本轮不做限流**（原设计里的限流项移出，将来单独一轮；落点预留在 `internal/common/ratelimit`）。
- 只做后端；前端注册页 / 登录页由用户侧接线。
- 校验规则与注册页 UI 对齐：字段 `email` / `name`（映射为 `nickname`）/ `password`（`confirmPassword` 由前端自行校验）；密码页面上已写明「至少 8 位、含字母和数字」。

沿用层 1 的既有约定：模块五件套、零注释、错误形状 `{"error":"..."}`、非哨兵错误映射 500、`/api/v1` 前缀只在 `app.go` 定义一次、ent + Atlas 迁移、集成测试读 `DATABASE_URL_TEST` 缺省 skip。

## 2. 已定决策

| 项 | 决定 |
| --- | --- |
| 注册开放性 | 公开；不验证邮箱 |
| 限流 | **本轮不做**（用户明确；将来单独一轮） |
| 注册即登录 | 是：201 + 会话 cookie（复用层 1 的会话写入路径：`RenewToken` + `Put`） |
| 密码规则 | ≥8 字符、同时含字母与数字、≤72 字节（bcrypt 上限）；改密码时新密码不得与旧密码相同 |
| 昵称规则 | trim 后 2–32 个字符（按 rune 计）；不含空白、不含 `@`（避免与邮箱在同一次 identifier 查询里撞车）；大小写敏感、原样存 |
| 邮箱规则 | `net/mail.ParseAddress` 可解析、≤254 字符、入库统一小写（沿用层 1 规则） |
| 唯一性 | `email` / `nickname` 各自唯一；先查后插 + 唯一索引兜底，约束冲突映射为 409 |
| 改密码会话语义 | **保留当前会话、吊销该用户其他全部会话**（推荐 B） |
| 吊销实现 | 用 scs 公开 API：`Store.All()` 枚举 + `SessionManager.Codec.Decode` 解码，按会话中的 `auth.user_id` 定位、`store.Delete` 删除；当前会话的 token 从请求 cookie `session` 读取；**零 schema 改动** |
| 数据模型 | **无 schema 改动**（`users` 表沿用层 1；角色区分留到需要时） |
| 版本 | minor：0.2.1 → **0.3.0** |

已核实的事实（2026-10-11，scs v2.9.0 源码）：
- cookie 值就是会话 token（`session.go:144` 读取 `cookie.Value`、`:198` 写出 `Value: token`）；
- `HashTokenInStore` 默认 `false` 且本项目未开启，因此存储键 == cookie 值，可直接比较；若将来开启该开关，吊销比较需改为「哈希后再比」；
- `scs.Store` 接口含 `All() (map[string][]byte, error)`，`postgresstore` 已实现；`SessionManager.Codec`（`Encode` / `Decode`）为公开 API。

## 3. 接口

| 方法 路径 | 认证 | 请求 | 成功 | 失败 |
| --- | --- | --- | --- | --- |
| `POST /api/v1/auth/register` | 公开 | `{"email":"...","nickname":"...","password":"..."}` | 201 `{"email":"...","nickname":"..."}` + `Set-Cookie` | 400 `invalid_request`（JSON 畸形 / 缺字段 / 邮箱或昵称规则不符）；400 `weak_password`（密码规则不符）；409 `email_taken`；409 `nickname_taken`；409 `already_taken`（并发竞态兜底） |
| `POST /api/v1/auth/password` | 需登录 | `{"currentPassword":"...","newPassword":"..."}` | 204 | 401 `unauthorized`（无会话）；401 `invalid_credentials`（旧密码错）；400 `invalid_request`；400 `weak_password`；400 `password_unchanged` |

说明：
- 注册接口**区分**「邮箱已占用 / 昵称已占用」（注册场景的常规 UX，且用户需要知道换哪个）；登录接口维持层 1 的**不区分**（不泄露账号是否存在）。
- 错误形状沿用 `{"error":"..."}`；表外的服务端故障沿用 500 `internal_error`。
- 本层不新增环境变量。
- 注册与改密码沿用层 1 的同一个会话管理器：cookie 名与 HttpOnly / SameSite / Path / Secure 等属性完全一致，无新增配置。
- 已登录状态下调用 `register`：照常注册，并把当前会话切换为**新账号**（`RenewToken` + `Put` 新 userID），不做特殊拦截。
- 已登录但**账号已被删除**时调用 `password`：按「用户不存在」处理——销毁会话并返回 401 `unauthorized`（与 `/auth/me` 的删号路径一致）。

## 4. 实现要点（全部在既有落点内）

- `internal/modules/auth/dto.go`：新增 `RegisterRequest`、`PasswordChangeRequest`；复用 `UserResponse` / `ErrorResponse`。
- `internal/modules/auth/repository.go`：复用 `createUser` / `findUserByIdentifier` / `findUserByID`（层 1 已有）；新增 `updatePasswordHash(ctx, id, hash)`。
- `internal/modules/auth/service.go`：
  - `Register(ctx, email, nickname, password) (User, error)`：规范化（email 小写、trim）→ 校验规则 → 唯一性预查（命中 → `ErrEmailTaken` / `ErrNicknameTaken`）→ bcrypt → `createUser`；唯一索引兜住并发竞态，冲突时统一返回 `ErrAlreadyTaken`（对应 409 `already_taken`，不做索引名解析）→ 写会话（`RenewToken` + `Put`）。
  - `ChangePassword(ctx, userID, currentToken, currentPassword, newPassword) error`：取用户（不存在 → `ErrUnauthorized` 并销毁会话）→ 校验新密码（规则 + 不得与旧相同）→ 用 bcrypt 比旧密码（错则 `ErrInvalidCredentials`）→ 生成新哈希并更新 → `revokeOtherSessions(ctx, userID, currentToken)`。
  - `revokeOtherSessions(ctx, userID, currentToken)`：`store.All()` → 逐个 `Codec.Decode` → `values[guard.SessionUserIDKey]` 等于该 userID 且 token ≠ currentToken → `store.Delete(token)`；解码失败的行跳过（不因单行异常中断吊销）。
  - 新增哨兵错误：`ErrInvalidRequest`、`ErrEmailTaken`、`ErrNicknameTaken`、`ErrAlreadyTaken`、`ErrWeakPassword`、`ErrPasswordUnchanged`。
- `internal/modules/auth/controller.go`：注册两个新路由；`/auth/password` 从 guard 取 `userID`、从 `c.Cookie("session")` 取当前 token 传给 service（service 不碰 HTTP）；错误 → 状态码/错误码映射见第 3 节。
- `internal/modules/auth/module.go`：`Register` 增加 `POST /auth/register`；`RegisterProtected` 增加 `POST /auth/password`。
- `internal/app/app.go`：**无需改动**（两条路由都经由既有 `Register` / `RegisterProtected`）。
- `backend/README.md`：接口表补两行；认证章节补「公开注册」「昵称/密码规则」「改密码会话语义」；版本行由脚本同步。
- `docs/DEVLOG.md`：新增一条。

## 5. 测试

集成测试（真库，沿用唯一后缀 + `t.Cleanup` 清理约定）：

注册：
1. 成功 → 201 + `Set-Cookie`，随后 `/auth/me` 200（注册即登录）；
2. 重复邮箱 → 409 `email_taken`；重复昵称 → 409 `nickname_taken`；
3. 弱密码分支：<8 位、纯字母、纯数字、>72 字节 → 400 `weak_password`；
4. 邮箱非法（无 `@` / `net/mail` 拒绝）、昵称含 `@` / 含空白 / 超长 → 400 `invalid_request`；
5. 畸形 JSON / 缺字段 → 400 `invalid_request`；
6. 邮箱大小写：`A@B.com` 注册后，用 `a@b.com`（换一个不同昵称）再注册 → 第二次 409 `email_taken`。

改密码：
7. 成功 → 204；随后**新密码可登录**、**旧密码 401**；
8. 旧密码错 → 401 `invalid_credentials`；未登录 → 401 `unauthorized`；
9. 新密码弱 / 与旧相同 → 400 `weak_password` / `password_unchanged`；
10. 会话吊销：两个 client 登录同一账号 → A 改密码 → **A 仍 200**、**B 变 401**；B（已吊销）再访 `/me` 只返回 401（不复活）。

单元：
11. 密码 / 邮箱 / 昵称校验函数的分支表；
12. 吊销逻辑对「解码失败的行」不中断（构造一个坏行）。

回归：`go test ./...` 带库与无库各一遍；`gofmt -l .` 空；`go vet ./...`、`go build ./...` 干净。

## 6. 验收标准

1. `POST /auth/register` 真机成功：注册 → 响应带 cookie → `/auth/me` 返回本人；库里能看到该行（email 小写）。
2. 重复邮箱 / 昵称分别 409；弱密码与格式错误分别 400 且错误码正确。
3. `POST /auth/password` 成功 204；旧密码错 401；新密码可登录、旧密码不可。
4. 双设备验证吊销：B 设备被踢、A 设备保持登录。
5. 层 1 的既有测试与行为不回退（登录 / 登出 / me / cookie 属性 / 删号吊销）。
6. README 与 DEVLOG 更新；版本进位 0.2.1 → 0.3.0。
7. 无库环境 `go test ./...` 全绿（集成用例 skip）。

## 7. 已知缺口与风险

- **无限流**（本轮明确不做）：注册接口对脚本刷号、登录接口对撞库都没有速率防护。现有缓解：bcrypt 慢哈希、登录错误不区分账号、未知账号等价比较。**建议下一轮优先补**，并同时处理「前端经 Next 代理时 Go 看到的 IP 全为代理机」的问题（部署轮配置 `SetTrustedProxies` 真实 CIDR）。
- 公开注册意味着任何人都能开号；当前所有登录用户权限等同（还没有管理员专属接口）。**第一个管理员专属接口出现前必须先加角色区分**。
- 吊销实现依赖 scs 的公开 API（`Store.All` + `Codec`）与「存储键 == cookie 值」的前提；测试 10 与 12 用于锁住这两点，scs 升级破坏时会变红。
- 注册接口区分邮箱/昵称占用属于有意的 UX 取舍（可被用于探测某邮箱是否注册过）；如用户不接受，可改为统一错误码。

## 8. 插槽（本轮不定，定它的时点）

| 插槽 | 什么时候必须定 | 影响面 |
| --- | --- | --- |
| 限流方案与限额 | 下一轮（用户要求时） | 新包 `internal/common/ratelimit`、app 装配、部署可信代理 |
| 角色（管理员 / 普通用户） | 第一个管理员专属接口出现时 | `users` 表、guard、控制台 |
| 邮箱验证 / 找回密码 | 接入 SMTP 后 | `users`、新流程 |
| 注册开放度（关闭注册 / 邀请码） | 被刷或用户要求时 | register 端点 |

## 9. 下一步

本文经用户审阅通过后，进 writing-plans 出实施计划（范围 = 第 4 节，验收 = 第 6 节）。
