# NeoBlog 认证设计（层 1：登录 / 登出 / 会话与守卫）

日期：2026-10-11
范围：`backend/`（Go + Gin）、`frontend/`（一条代理规则 + 登录表单接接口）
状态：设计经用户逐项确认（2026-10-11 对话稿）；本文为存档版，其后进 writing-plans 出实施计划

修订记录：
- v1（2026-10-11，对话稿）：认证 = 登录 / 登出 / 会话守卫；库 = scs + bcrypt（2FA 留层 2）；数据访问 = sqlc + goose。
- v2（2026-10-11，本文）：数据访问改为 **ent + Atlas**，对齐《数据层设计（层 0）》——该文档已定「第一张业务表落地时接 ent + Atlas」，认证的 users / sessions 正是第一张表。其余与 v1 一致。

## 1. 意图与背景

目标：给博客管理后台一套登录鉴权。第一版闭环 = 能登录、能退出、未登录访问受保护接口被拒（401）。

已定约束（用户）：
- 认证只由 Go 后端负责；Next 只做代理（BFF）；不引入额外服务、不引入额外界面。
- 后端代码零注释、NestJS 式模块结构（module / controller / service / dto）。
- 数据库与迁移按已定的数据层设计：`docs/superpowers/specs/2026-10-11-neoblog-data-layer-design.md`（ent + Atlas）。
- 账号只有管理员一个；不做注册页（已有 `frontend/app/signup` 保持纯 UI）；TOTP 二次验证留层 2。

对齐来源（不重新发明）：
- users 表字段从 `F:\protect\Neo`（NestJS + Prisma 平行项目）的 `backend/prisma/schema.prisma` 平移：`email`（唯一）、`nickname`（唯一）、`passwordHash`、`createdAt`、`updatedAt`。
- 前端已有登录页 `frontend/app/signin`（纯 UI，字段：`identifier`「用户名或邮箱」+ `password`）；本轮只接接口，不改版式。

## 2. 已定决策

| 项 | 决定 | 落地时点 |
| --- | --- | --- |
| 会话库 | `alexedwards/scs` v2 + `scs/postgresstore`（构造入参 `*sql.DB`，配现有 pgx stdlib 驱动；自带过期清理 goroutine，默认 5 分钟） | 本轮 |
| 会话表 | `sessions(token, data, expiry)` + expiry 索引（scs 存储的约定表）；用 ent schema 建模、由 Atlas 生成迁移 | 本轮 |
| 密码哈希 | bcrypt（`golang.org/x/crypto/bcrypt`，默认 cost） | 本轮 |
| 登录标识 | 单一字段 `identifier`，按 `email` 或 `nickname` 匹配（对齐前端表单文案） | 本轮 |
| Cookie | 名 `session`、HttpOnly、SameSite=Lax、Path=/、`Secure` 仅当 `APP_ENV=production`；有效期默认 7 天（`SESSION_LIFETIME` 可覆盖） | 本轮 |
| 会话固定防护 | 登录成功后 `RenewToken`；登出 `Destroy` | 本轮 |
| 会话内容 | 只存 `userID`（键 `auth.user_id`）；不存邮箱昵称快照，避免过期数据 | 本轮 |
| 数据访问 | ent（schema 在 `internal/database/ent/schema/`，生成代码在 `internal/database/ent/`）；用 `entsql.OpenDB(dialect.Postgres, db)` 复用现有 `*sql.DB`（ent 官方的既有连接接法）；模块内 `repository.go` 为具体类型、不抽接口（对齐数据层设计「不隔仓储接口层」） | 本轮 |
| users ID 方案 | Postgres `uuid` 类型、UUIDv7（`google/uuid`）；不可枚举、时间有序。与 Neo 的 cuid 形状不同但无实际影响：ID 不出现在 API，也无跨项目搬行需求 | 本轮 |
| 迁移 | Atlas（官方 Windows 二进制，已装）+ 版本化 SQL 进 `migrations/`；流程按数据层设计 §6（diff → 人工审阅 SQL → apply → validate） | 本轮 |
| 管理员创建 | 新工具 `cmd/create-admin`（应用入口仍在根 `main.go`）：读 `DATABASE_URL`，密码从 stdin 读（不进 shell 历史），bcrypt 后写 users | 本轮 |
| 登录限流 | 本轮不做（见 §9 已知缺口） | 层 2 |
| TOTP 2FA（+ 恢复码） | `pquerna/otp` + 自管恢复码 | 层 2 |
| API Token / Passkey / 社交登录 | `golang-jwt/jwt` / `go-webauthn/webauthn` / `markbates/goth` | 更后 |

## 3. 范围

本轮做：

- `internal/modules/auth/`：dto / repository / service / controller / module 五件套。
- `internal/common/guard/`：`RequireAuth` Gin 中间件（`internal/common` 目录由此建立）。
- `internal/database`：新增 `Client *ent.Client`（在 `Open` 内构造，签名不变）；新增访问器 `SQL() *sql.DB`（供会话存储等构造使用）。
- ent schema：`users`、`sessions`；生成代码与 `tools.go`（ent 生成依赖）。
- 迁移：`migrations/` 首个迁移（users + sessions）。
- 脚本：`scripts/db-generate.sh`、`scripts/db-migrate-diff.sh`、`scripts/db-migrate-apply.sh`。
- `cmd/create-admin`（管理员创建工具）。
- 配置：新增 `SESSION_LIFETIME`（默认 `168h`）。
- 前端：`next.config.ts` 加 rewrite（`/api/:path*` → `${BACKEND_URL}/api/:path*`，默认 `http://127.0.0.1:5732`）；`app/signin/signin-form.tsx` 接登录接口（成功跳 `/console`，失败就地提示）。
- README 更新（接口、环境变量、目录对照表加 auth / guard / common、数据库章节补迁移工作流）。
- 版本进位 minor：0.1.0 → 0.2.0。

本轮不做：注册、找回密码、邮箱验证、2FA、API Token、登录限流、`/console` 的前端路由保护、生产 cookie 域方案、CORS（同源代理下不需要）。

## 4. 数据模型（ent schema 摘要）

users（字段对齐 Neo 的 prisma User）：

- `id`：uuid、主键、默认 UUIDv7
- `email`：text、唯一、必填
- `nickname`：text、唯一、必填
- `password_hash`：text、必填
- `created_at` / `updated_at`：timestamptz

sessions（scs postgresstore 的约定表，供其读写）：

- `token`：text、唯一、必填
- `data`：bytea、必填
- `expiry`：timestamptz、必填（建索引）

说明：ent 会为会话表附带默认 `id` 主键列；scs 只读写 token / data / expiry，多出的列不影响其行为。选择「ent 建模 + Atlas 生成」而不是手写迁移，是为保持单一迁移管线、零漂移（备选方案与代价见 §9）。

## 5. 接口

| 方法 路径 | 认证 | 请求 | 成功 | 失败 |
| --- | --- | --- | --- | --- |
| `POST /api/v1/auth/login` | 公开 | `{"identifier":"...","password":"..."}` | 200 `{"email":"...","nickname":"..."}` + Set-Cookie | 401 `{"error":"invalid_credentials"}` |
| `POST /api/v1/auth/logout` | 守卫 | 空 | 204 | 401 `{"error":"unauthorized"}` |
| `GET /api/v1/auth/me` | 守卫 | 空 | 200 `{"email":"...","nickname":"..."}` | 401 `{"error":"unauthorized"}` |

- 错误形状 `{"error":"..."}` 为本轮最小约定；common 层有统一 filters 时再收编。
- 登录失败对外统一 `invalid_credentials`，不区分「账号不存在 / 密码错」；账号不存在时也执行一次等价 bcrypt 比较（抗时序探测）。
- 守卫职责单一：会话里有 `userID` 就放进 gin.Context 放行，否则 401。不查库、不依赖任何业务模块。
- `/auth/me` 由 service 用 `userID` 查库返回 email / nickname；用户已被删除时返回 401（并清会话）。

## 6. 装配（对齐 NestJS 结构）

- `main.go`：`LoadDotEnv` → `config.Load` → `database.Open`（内含 ent client）→ `app.New(cfg, db)` → `Run`。
- `app.go`：
  - 构造 `scs.SessionManager`（cookie 策略、postgresstore、`SESSION_LIFETIME`）——会话管理器属应用生命周期，归 app 层。
  - `http.Server.Handler = sessions.LoadAndSave(engine)`（scs 是 net/http 中间件，包在引擎外）。
  - 公开组：`authModule.Register(api)` → 登录路由。
  - 保护组：`protected := api.Group("", guard.RequireAuth(sessions))`，`authModule.RegisterProtected(protected)` → 登出、me。以后每个模块的保护路由都这样挂。
- `internal/modules/auth/module.go`：`New(cfg, db, sessions) *Module`；`Register`（公开）与 `RegisterProtected`（受保护）；对外暴露 `Service`（供其他模块使用，本轮暂无消费方）。
- 会话键 `auth.user_id` 的常量由 `internal/common/guard` 提供；auth 写入时引用同一常量。

## 7. 前端（本轮的一点点）

- rewrite：`/api/:path*` → `${BACKEND_URL}/api/:path*`。开发下浏览器与 Next 同源（localhost:3000），cookie 自然带上，无需 CORS。
- `signin-form.tsx`：提交 `identifier` / `password` → `POST /api/v1/auth/login` → 200 跳 `/console`；401 显示「用户名或密码错误」；网络错误就地提示。
- 其余页面不动；`/console` 的前端路由保护（未登录跳 `/signin`）放下一轮。

## 8. 测试

- 迁移：`scripts/db-migrate-apply.sh` 把迁移应用到 `DATABASE_URL` 与 `DATABASE_URL_TEST`（neoblog_test）；集成测试假设库已迁移（README 写明前置步骤）。
- 集成测试（HTTP 级、真库，沿用 `DATABASE_URL_TEST` + 缺省 skip 的既有约定）：
  1. 登录成功 → 200 + Set-Cookie（HttpOnly）
  2. 密码错 → 401
  3. 未知账号 → 401（同一错误形状）
  4. 未登录访问 `/auth/me` → 401
  5. 登录后 `/auth/me` → 200 且返回 email / nickname
  6. 登出后 `/auth/me` → 401
  7. 带着旧会话 cookie 再次登录 → 会话 token 变化（RenewToken 生效）
  测试数据用唯一后缀，测试自行清理。
- 守卫单测（不连库）：scs 内存 store + httptest，覆盖已登录 / 未登录两个分支与 401 形状。
- 密码哈希：bcrypt 往返单测。

## 9. 已知缺口与风险

- **登录接口无限流**（本轮有意不做）：缓解 = bcrypt 慢哈希、错误信息不区分账号、未知账号也做等价比较。建议层 2 加最简单的按 IP 内存限流，或交给反向代理；用户可在审阅本文时要求本轮加。
- 生产 cookie 域：开发同源无问题；部署时若前端与 API 不同子域，需设 cookie `Domain` 或由 Next 路由处理器统一种 cookie——部署轮定。
- scs/postgresstore 文档以 `pq` 驱动举例，本设计用 pgx stdlib（官方说明可替换）；落地时确认依赖清单没有把未使用的 `lib/pq` 带进直接依赖。
- 会话表由 ent 建模会多一个 `id` 列（见 §4）；若审阅时倾向「表形状完全对齐 scs 文档」，可改为手写迁移 + Atlas 忽略该表，代价是引入一处需人工维护的漂移点。
- `/console` 前端无路由保护（其数据尚未接入，风险为零），下一轮补。

## 10. 验收标准

1. `bash scripts/db-up.sh` → 容器 healthy；`scripts/db-migrate-apply.sh` 将迁移应用到 `neoblog` 与 `neoblog_test`。
2. 迁移 SQL 人工审阅过、`atlas migrate validate` 通过；`go build ./... && go vet ./... && gofmt -l .` 干净。
3. `go test ./...` 全绿（无库环境集成测试 skip；有库环境真跑并通过）。
4. `go run ./cmd/create-admin` 能建出管理员；重复邮箱 / 昵称有明确报错。
5. 真机：`/signin` 登录成功 → 跳 `/console`；curl 带 cookie 访问 `/api/v1/auth/me` 200；登出后 401；不带 cookie 访问 401；`/api/v1/healthy` 不受影响。
6. README 更新完成；版本进位 0.1.0 → 0.2.0（`./scripts/bump-version.sh minor`）。

## 11. 插槽（本轮不定，定它的时点）

| 插槽 | 什么时候必须定 | 影响面 |
| --- | --- | --- |
| 限流方案（内存 / 反代 / 边缘） | 层 2 开头（或审阅本文时要求提前） | 登录 controller / service |
| 会话有效期与「记住我」策略 | 首次出现多设备使用痛点时 | cookie 策略 |
| 多用户与角色 | 出现评论 / 多人后台时 | users 表与守卫 |
| 2FA 的表结构与恢复码 | 层 2 | users / 新表、登录流程 |
| 生产 cookie 域与 TLS 部署形态 | 部署轮 | app.go、Next 代理 |

## 12. 下一步

本文经用户审阅通过后，进 writing-plans 出实施计划（范围 = §3，验收 = §10）。
