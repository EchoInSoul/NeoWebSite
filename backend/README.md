# website 后端（Go + Gin）

当前版本：0.5.0

设计规范对齐 NestJS：模块化组织、单向依赖、显式依赖注入；应用生命周期（监听、超时、优雅关闭）由 app 层掌管，对应 Nest 的 `app.listen` / `enableShutdownHooks`。Go 没有装饰器和反射式 DI 容器，对应物是构造函数传依赖、模块 `New` 暴露能力、`Register` 装配。

## 目录与 NestJS 对照

| 路径 | NestJS 对应 | 职责 |
| --- | --- | --- |
| `main.go` | `main.ts` | 启动引导：加载配置、创建应用、启动 |
| `internal/app/app.go` | `app.module.ts` + `setGlobalPrefix` | 组装模块、全局前缀 `/api/v1`、HTTP 服务器（超时 / 优雅关闭） |
| `internal/config` | `ConfigModule` | 读环境变量，产出 `Config` |
| `internal/database` | `core/data`（Neo 项目对照） | 数据连接的注入点与迁移入口：`Open` / `Ping` / `Close` / `Client`（ent）/ `SQL` |
| `internal/version` | `package.json` 的 `version` | 后端版本号唯一来源 |
| `scripts/bump-version.sh` | `npm version` | 版本号进位工具 |
| `scripts/db-up.sh` / `scripts/dev.sh` | — | 起本地 Postgres 容器（等到 healthy） / 启动后端（先定位到 `backend/` 再 `go run .`，可在任意目录执行） |
| `docker-compose.yml` + `deploy/postgres/` | — | 本地 Postgres 容器定义与首次初始化（建 `neoblog_test`、`neoblog_atlas_dev`） |
| `migrations/` + `scripts/db-generate.sh` / `db-migrate-diff.sh` / `db-migrate-apply.sh` | — | 版本化迁移：ent 代码生成、生成迁移（diff）、应用到 `neoblog` 与 `neoblog_test` |
| `internal/modules/health/module.go` | `health.module.ts` | 装配本模块，`New` 返回 `*Module` 并暴露对外能力 |
| `internal/modules/health/controller.go` | `health.controller.ts` | 本模块的路由声明与 HTTP 编解码（包内私有） |
| `internal/modules/health/service.go` | `health.service.ts` | 业务逻辑，模块对外暴露的能力 |
| `internal/modules/health/dto.go` | `dto/*.dto.ts` | 请求/响应数据结构 |
| `internal/modules/auth/*` | `auth.module.ts` 等 | 注册 / 登录 / 登出 / 当前用户 / 改密；`Register`（公开）与 `RegisterProtected`（受保护） |
| `internal/modules/access/*` | `access.module.ts` 等 | 用户组与权限：组 CRUD、用户管理接口、生效权限解析；全部挂在权限守卫下 |
| `internal/modules/mail` | `@nestjs-modules/mailer`（对照位） | 无路由模块（无 controller / dto / Register，对外入口 `New(cfg) Mailer` 与 `Render`）：`Mailer` 接口（Send / Close）+ SMTP / 日志双实现 + `templates/` 模板；`SMTP_HOST` 为空时降级为日志 |
| `internal/common/guard` | `src/common`（Guards） | `RequireAuth`（会话）+ `LoadIdentity`（身份与生效权限）+ `RequirePermission`（权限点）；常量 `auth.user_id` / `auth.identity` |
| `internal/common/rbac` | `src/common`（权限目录） | 权限点目录与预设组常量（固定 UUID）；纯 Go，不依赖 gin / DB |
| `internal/common` 其余（待加） | `src/common` | pipes 校验 / interceptors / filters 统一错误（guards 已落地于 `internal/common/guard`） |

## 硬性规则

1. 一个功能一个模块目录 `internal/modules/<name>/`，自包含 module / controller / service / dto。**无路由的能力模块（当前仅 `mail`）可省 controller / dto / module.go，只提供实现文件 + `New` 入口 + 测试，不要为凑形状建空文件。**
2. 依赖方向单向：controller → service → repository（需要 DB 时同目录 `repository.go`）。禁止反向调用，禁止跨层直连。
3. Gin 与 HTTP 细节只允许出现在 controller、module.go（挂载路由）、app 和 `internal/common/guard`（中间件）；service 一律不 import gin。
4. 路由由 controller 自己声明；全局前缀 `/api/v1` 只在 `app.go` 定义一次。
5. 模块只通过 `New(...)` 返回的 `*Module` 对外暴露能力（如 service）；跨模块依赖只在 `app.go` 里显式注入，不允许直接构造或调用别的模块的内部实现。唯一例外：为测试提供的包级入口（当前仅 `auth.CreateAdminUser`），业务代码不得调用。**另一个例外：无路由的能力模块由 `New` 直接返回能力接口（当前仅 `mail.New(cfg) Mailer`）；无状态的包级纯函数可直接导出（如 `mail.Render`）——都不算对外暴露内部实现。**
6. 代码零注释（只有 Go 工具链自动生成的注释例外）；缩进一律走 gofmt。
7. 模块禁止自建数据库连接（禁止 `sql.Open` 等），只能用注入进来的连接；**同理禁止自建 SMTP 客户端，发信一律用注入的 `mail.Mailer`**。
8. `internal/database` 不得 import 任何业务模块。

## 新增一个功能模块的顺序

`internal/modules/<name>/`：dto.go → repository.go（需要 DB 时）→ service.go → controller.go → module.go（`New` 返回 `*Module`，`Register` 挂路由），最后在 `internal/app/app.go` 里装配：

```go
postsModule := posts.New(cfg)
postsModule.Register(api)
```

## 环境变量

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `ADDR` | `:5732` | 监听地址（本机 8080 被 Steam 的 CEF 调试接口占用，默认改用 5732） |
| `APP_ENV` | `development` | 环境标识，出现在健康检查响应里；`production` 时自动切到 Gin release 模式（关闭 debug 横幅与路由表打印） |
| `APP_VERSION` | 编译进二进制的版本号 | 仅作运行时临时覆盖（本地调试），常规部署不设置 |
| `SERVICE_NAME` | `website-backend` | 服务名 |
| `DATABASE_URL` | 无（必需） | Postgres 连接串；可写在 `backend/.env`（启动时自动加载，环境变量优先）；缺失或不可用时启动即失败 |
| `DATABASE_URL_TEST` | 无 | 集成测试专用连接串；不设置时集成用例自动 skip |
| `DB_MAX_OPEN_CONNS` | `10` | 连接池最大连接数 |
| `DB_MAX_IDLE_CONNS` | `5` | 连接池空闲连接数（不得超过最大连接数） |
| `DB_CONN_MAX_LIFETIME` | `30m` | 连接最长存活时间 |
| `SESSION_LIFETIME` | `168h` | 会话有效期（浏览器 cookie 的 `Max-Age` 同此值） |
| `SMTP_HOST` | 空 | SMTP 服务器；**为空时邮件只写日志、不影响启动** |
| `SMTP_PORT` | `587` | SMTP 端口（正整数 ≤ 65535；**25 不受支持**——会被 go-mail 视为未设置并改写成 587/465） |
| `SMTP_TLS` | `starttls` | `starttls`（配 587）或 `ssl`（配 465） |
| `SMTP_USERNAME` | 空 | 认证用户名；为空则不做 AUTH（自建中继场景） |
| `SMTP_PASSWORD` | 空 | 认证密码（QQ/163 为授权码）；用户名非空时必填 |
| `SMTP_FROM` | 无 | 发件地址；`SMTP_HOST` 非空时必填 |
| `SMTP_FROM_NAME` | `NeoBlog` | 发件人显示名 |
| `MAIL_TEST_RECIPIENT` | 空 | 仅测试用：与 `SMTP_*` 齐备时真实投递用例会向它发一封自检邮件 |
| `POSTGRES_PASSWORD` | `neoblog` | 本地容器密码（compose 读 `.env`）；改动需同步 `DATABASE_URL` |

## 版本号

格式 `x.y.z`，唯一来源是 `internal/version/version.go`，用脚本自动进位（minor 进位时 z 归零，major 进位时 y、z 归零）：

| 场景 | 命令 | 示例 |
| --- | --- | --- |
| 细节更改 | `./scripts/bump-version.sh patch` | 0.0.1 → 0.0.2 |
| 中等功能 | `./scripts/bump-version.sh minor` | 0.0.1 → 0.1.0（z 归零） |
| 大功能 | `./scripts/bump-version.sh major` | 0.0.1 → 1.0.0（y、z 归零） |
| 直接指定 | `./scripts/bump-version.sh set <x.y.z>` | 设为任意合法版本，如 `set 1.0.0` |

版本号随代码编译进二进制，出现在 `GET /api/v1/healthy` 的 `version` 字段；运行时可用 `APP_VERSION` 环境变量临时覆盖。进位脚本会同步更新本 README 顶部的「当前版本」，改完把 `internal/version/version.go` 和本次改动一起提交。

## HTTP 服务器

`app.go` 已配置读头 / 读 / 写 / 空闲超时；收到 SIGINT / SIGTERM 后停止接收新请求，等待在途请求完成（最多 10 秒）再退出。默认不信任任何代理头，将来放到反向代理后面时再在 `app.go` 配置真实代理 CIDR。

## 数据库

本地开发用 WSL 里的 Docker 起独立 Postgres 17 容器（端口 5434；5432 是 D 盘项目、5433 是 Neo 项目的库，互不干扰）：

```bash
bash scripts/db-up.sh   # 起容器并等到 healthy（容器名 neoblog-postgres）
```

在 WSL 终端里手动执行等价命令：

```bash
cd /mnt/f/protect/NeoBlog/backend && docker compose up -d
```

迁移工作流（Atlas，迁移文件在 `migrations/`，生成后**人工审阅 SQL** 再提交）：

```bash
bash scripts/db-up.sh                  # 起容器（等 healthy）
export DATABASE_URL='postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog?sslmode=disable'
export DATABASE_URL_TEST='postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_test?sslmode=disable'
bash scripts/db-migrate-apply.sh       # 把 migrations/ 应用到两个库（集成测试的前置）
bash scripts/db-generate.sh            # ent schema 有变更后重新生成代码
bash scripts/db-migrate-diff.sh        # ent schema → 生成新迁移（之后人工审阅 + atlas migrate hash/validate）
```

应用启动绝不自动迁移；`ATLAS_DEV_URL` 缺省指向空的 `neoblog_atlas_dev`（脚本内默认值）。迁移文件里含数据（预设组 seed）或手工改造过 SQL 时，必须重算校验和：`atlas migrate hash --dir "file://migrations"` 再 `atlas migrate validate`（用户组与权限迁移是第一个带 seed 的）。工具链细节与已验证事实见 `docs/superpowers/specs/2026-10-11-neoblog-data-layer-design.md` 第 6、7 节。

## 认证

只提供后端接口（前端由前端侧接线）。层 1：登录 / 登出 / 当前用户；层 2：公开注册与更改密码。

| 方法 路径 | 认证 | 说明 |
| --- | --- | --- |
| `POST /api/v1/auth/register` | 公开 | body `{"email","nickname","password"}` → 201 `{"email","nickname"}` + 会话 cookie（注册即登录）；400 `invalid_request` / `weak_password`；409 `email_taken` / `nickname_taken` / `already_taken`（并发竞态兜底） |
| `POST /api/v1/auth/login` | 公开 | body `{"identifier":"邮箱或昵称","password":"..."}` → 200 + 会话 cookie；401 `invalid_credentials`；400 `invalid_request` |
| `POST /api/v1/auth/logout` | 需登录 | 销毁会话，返回 204 |
| `GET /api/v1/auth/me` | 需登录 | 返回 `{"email","nickname","group":{"id","system_key","name"},"permissions":["..."]}`（自建组 `system_key` 为 `null`，`permissions` 是展开后的生效权限）；未登录 401 `unauthorized` |
| `POST /api/v1/auth/password` | 需登录 | body `{"currentPassword","newPassword"}` → 204；旧密码错 401 `invalid_credentials`；400 `weak_password` / `password_unchanged` / `invalid_request` |

规则：

- 注册**公开**、不验证邮箱（邮件基础设施见「邮件」章节，注册流程尚未接入；也没有找回密码）。
- 邮箱：`net/mail` 可解析、≤254 字符、入库统一小写；昵称：trim 后 2–32 字符（按 rune）、不含空白（含全角空格）与 `@`；密码：≥8 字符（按 rune）、同时含 ASCII 字母与数字、≤72 字节（bcrypt 上限）。
- **更改密码保留当前设备、吊销该用户其他所有设备的会话**（用 scs 的 `IterableStore.All()` + `Codec` 按会话中的 `auth.user_id` 定位删除；失败即 500 且密码不落盘，可用旧密码重试）。

会话由 `scs` 管理、存 Postgres `sessions` 表；cookie 名 `session`、HttpOnly、SameSite=Lax、`Secure` 仅当 `APP_ENV=production`；登录与注册成功都会换发会话 token（防会话固定）。受保护路由在 `app.go` 里经 `guard.RequireAuth` 挂载。账号由注册流程创建；`auth.CreateAdminUser` 仅供测试使用。

**已知缺口**：登录与注册**暂无限流**（防刷号 / 防撞库，计划下一轮优先补）；部署时若前端经代理转发，需在 `app.go` 里用 `SetTrustedProxies` 配置真实代理 CIDR，否则 Go 看到的来源 IP 全是代理机、按 IP 限流会失效。

### 用户组与权限

三个预设组（不可删 / 不可改名 / 权限由代码计算，含以后新增的权限点）：`owner` 所有者 与 `admin` 管理员 = 全部权限，`user` 普通用户 = 空；自建组权限存库、可改名可删（有成员时不可删）。生效权限 = 所属组权限 ∪ 用户额外权限（两侧都做 `edit → view` 蕴含展开）；新用户由数据库默认值落「普通用户」，创建路径不显式设组。

| 方法 路径 | 需要的权限 | 说明 |
| --- | --- | --- |
| `GET /api/v1/admin/permissions` | `system.groups.view` | 权限目录 `{"items":[{"key","name"}]}` |
| `GET /api/v1/admin/groups` | `system.groups.view` | 组列表（`is_preset` / `member_count` / 展开后的 `permissions`） |
| `POST /api/v1/admin/groups` | `system.groups.edit` | 新建自建组 → 201 |
| `PATCH /api/v1/admin/groups/:id` | `system.groups.edit` | 改组名 / 组权限；预设组 → 403 `preset_group_locked` |
| `DELETE /api/v1/admin/groups/:id` | `system.groups.edit` | 删自建组 → 204；有成员 → 409 `group_not_empty` |
| `GET /api/v1/admin/users` | `system.users.view` | 用户列表（带所属组与额外权限） |
| `GET /api/v1/admin/users/:id` | `system.users.view` | 用户详情（另含 `permissions` = 生效权限） |
| `PATCH /api/v1/admin/users/:id` | `system.users.edit` | `{"group_id"?,"extra_permissions"?}`（额外权限为全量替换，`[]` 清空）；两个字段都不给 → 400 |

权限点命名 `模块.动作`，动作只有 `view` / `edit`（`edit` 蕴含 `view`）；目录是代码常量（`internal/common/rbac`），随模块落地逐条增加，不存在「目录里没有检查点」的空权限。

规则：

- **只能授出自己持有的权限**：新建 / 改组、设用户组、加额外权限时，目标新增（按生效权限集的前后差集、同一请求合并计算）而操作者没有的点 → 403 `grant_forbidden`；**摘除不受限**（删权限不会提权）。
- **不能改自己**（自己的组与额外权限都算）→ 403 `cannot_modify_self`；**owner / admin 组的成员只有 owner 能改** → 403 `owner_protected`。
- **至少保留一个 owner**：并发双降级的第二个请求 → 409 `last_owner`（事务内 `FOR UPDATE` 锁 owner 成员行后计数；串行路径由上面两条规则挡住）。
- 组名 trim 后 1–64 字符；权限数组先去重、目录外的 key → 400 `invalid_permission`；组名重复 → 409 `group_name_taken`。
- 守卫链：`RequireAuth`（未登录 401）→ `LoadIdentity`（解析身份与生效权限放进 `auth.identity`；用户已删除则销毁会话 + 401）→ `RequirePermission`（权限不足 403 `forbidden`）。**原则变更**：守卫不再是「不查库」，而是「只依赖 `app.go` 注入进来的 resolver，不 import 业务模块」。
- 权限改动**下一次请求即生效**（不吊销会话）；已在处理中的请求使用请求开始时的权限快照。
- **首个 owner 只能手工指定**（迁移把存量用户全部回填为「普通用户」）：`UPDATE users SET group_id = '00000000-0000-4000-8000-000000000001' WHERE email = '你的邮箱';`——owner / admin / 普通用户的固定 UUID 依次是 `…0001` / `…0002` / `…0003`（与 `internal/common/rbac` 一致）。

## 邮件

只做后端发信能力（无路由模块，无 controller / dto），**当前没有任何模块消费它**（注册不验证邮箱）。`internal/modules/mail` 对外只暴露 `Mailer` 接口与 `Render`：

```go
body, err := mail.Render("example", map[string]string{"Time": "...", "Site": "NeoBlog"})
mailer.Send(ctx, mail.Message{To: addr, Subject: "NeoBlog 邮件配置自检", Body: body})
```

- `Send` 只保证入队（缓冲 32），投递由后台 worker 串行执行：每封最多 3 次尝试、退避 1s / 5s、单次 20s 超时；**服务端 4xx 与连接类错误重试，5xx 不重试**，失败记结构化日志（`log/slog`）。
- `Close(ctx)` 停收新消息并把队列投完（`app.Run` 关闭路径中先 `server.Shutdown` 再 `mailer.Close`，共用 10s 预算）；`Close` 之后 `Send` 返回 `mail.ErrClosed`。
- 模板：`internal/modules/mail/templates/<name>.html` 与 `<name>.txt` 成对（HTML 侧用 `html/template` 自动转义）；内置一封 `example`（邮件配置自检）。
- `SMTP_HOST` 为空时降级为日志实现：邮件全文写进日志但不发送，开发环境无需凭据即可跑通全流程。
- 真实投递测试（可选）：`backend/.env` 填好 `SMTP_*` 与 `MAIL_TEST_RECIPIENT` 后跑 `go test ./internal/modules/mail/ -run TestRealDelivery -v`（测试会从包目录向上查找 `.env`，深度上限 5，不写死相对路径）。
- 上线前按所选服务商配好 SPF / DKIM / DMARC（域验类服务商侧签 DKIM）；发件域名必须与认证域名对齐，**不要用自有域名 From 搭配 QQ 个人邮箱认证**。细节见 `docs/superpowers/specs/2026-10-11-neoblog-mail-design.md` §11。

## 运行

在 `backend/` 目录下：

```bash
go run .                # 启动时自动加载工作目录下的 .env；默认 :5732（推荐）
bash scripts/dev.sh     # 同上（先定位到 backend/，可在任意目录执行）
ADDR=:9090 go run .     # 自定义端口
```

健康检查：`GET /api/v1/healthy` → `{"status":"ok","service":"website-backend","version":"0.5.0","environment":"development","database":"ok","uptime":"3s","time":"2026-10-11T01:35:12+08:00"}`。数据库 Ping 不通时返回 HTTP 503，`status` 为 `degraded`、`database` 为 `error`。
