# website 后端（Go + Gin）

当前版本：0.21.0

设计规范对齐 NestJS：模块化组织、单向依赖、显式依赖注入；应用生命周期（监听、超时、优雅关闭）由 app 层掌管，对应 Nest 的 `app.listen` / `enableShutdownHooks`。Go 没有装饰器和反射式 DI 容器，对应物是构造函数传依赖、模块 `New` 暴露能力、`Register` 装配。

模块内部的工程风格（文件职责、错误分层、DTO 归属、命名、测试与文档流程）见 [STYLE.md](STYLE.md)；本 README 继续只负责接口、环境与运维契约。

## 目录与 NestJS 对照

| 路径 | NestJS 对应 | 职责 |
| --- | --- | --- |
| `main.go` | `main.ts` | 启动引导：加载配置、创建应用、启动 |
| `internal/app/app.go` | `app.module.ts` + `setGlobalPrefix` | 组装模块、全局前缀 `/api/v1`、HTTP 服务器（超时 / 优雅关闭） |
| `internal/config` | `ConfigModule` | 读环境变量，产出 `Config`；`.env` 不存在时先自动生成一份（`EnsureDotEnv`，含随机 2FA 钥匙；**仅开发态 / `test`**，`production` 不自举） |
| `internal/database` | `core/data`（Neo 项目对照） | 数据连接的注入点与迁移入口：`Open` / `Ping` / `Close` / `Client`（ent）/ `SQL` / `StartupError`（首次探测失败原因，不致命） |
| `internal/version` | `package.json` 的 `version` | 后端版本号唯一来源 |
| `scripts/bump-version.sh` | `npm version` | 版本号进位工具 |
| `scripts/db-up.sh` / `scripts/dev.sh` | — | 起本地 Postgres 容器（等到 healthy） / 启动后端（先定位到 `backend/` 再 `go run .`，可在任意目录执行） |
| `scripts/build.sh` | `npm run build` | 发布构建：`CGO_ENABLED=0` + `-trimpath` + `-ldflags "-s -w -X website/backend/internal/config.DefaultEnvironment=production"`，把默认环境编译成 `production`；产物落 `bin/neoblog-<版本>-<GOOS>-<GOARCH>[.exe]` |
| `docker-compose.yml` + `deploy/postgres/` | — | 本地 Postgres 容器定义与首次初始化（建 `neoblog_test`、`neoblog_atlas_dev`） |
| `migrations/` + `scripts/db-generate.sh` / `db-migrate-diff.sh` / `db-migrate-apply.sh` | — | 版本化迁移：ent 代码生成、生成迁移（diff）、应用到 `neoblog` 与 `neoblog_test` |
| `internal/modules/health/module.go` | `health.module.ts` | 装配本模块，`New` 返回 `*Module` 并暴露对外能力 |
| `internal/modules/health/controller.go` | `health.controller.ts` | 本模块的路由声明与 HTTP 编解码（包内私有） |
| `internal/modules/health/service.go` | `health.service.ts` | 业务逻辑，模块对外暴露的能力 |
| `internal/modules/health/dto.go` | `dto/*.dto.ts` | 请求/响应数据结构 |
| `internal/modules/auth/*` | `auth.module.ts` 等 | 注册 / 登录（含 2FA 分叉）/ 登出 / 当前用户（`GET /users/me` 读、`PATCH /users/me` 改）/ 改密 / 两步验证；`Register`（公开）与 `RegisterProtected`（受保护）；`totp.go` + `secretbox.go` 承载 TOTP 窗匹配与 secret 加密 |
| `internal/modules/access/*` | `access.module.ts` 等 | 用户组与权限：组 CRUD、用户管理接口、生效权限解析；全部挂在权限守卫下 |
| `internal/modules/mail/*` | `@nestjs-modules/mailer`（对照位） | 邮件能力 + 管理端发信接口：`Mailer` 接口（Send / SendSync / Close）+ SMTP / 日志双实现 + `templates/` 模板；异步 `Send` 走后台队列重试，同步 `SendSync` 单次尝试（管理端即时反馈用）；`POST /admin/mail/messages` 挂守卫链 + 预设组守卫；`SMTP_HOST` 为空时降级为日志 |
| `internal/modules/settings/*` | `@nestjs/config` + `config.controller.ts`（对照位） | 有路由模块：设置清单定义在代码内（构造即自检）+ `setting_values` 复合键 upsert 存储；对外入口 `New(db) (*Module, error)`，能力挂 `m.Service`；管理侧 `GET / PATCH /admin/settings/:group`（按组挂权限点：`site` → `system.settings.view` / `.edit`，`register` / `auth` → `system.register.view` / `.edit`），公开侧 `GET /public/settings`（无鉴权，只出清单里标了 `Public` 的项）；响应统一为 `{group, values}` 记录、集合套 `items`；当前清单项 `site` 组六项（全公开）、`register` 组五项（`enabled` 公开；`email_domain_control_enabled` / `allowed_email_domains` 已有消费方）与 `auth` 组九项（`password_requirements_enabled` / `password_min_length` 已有消费方，其余非公开），加设置见「设置」章节配方 |
| `internal/common/guard` | `src/common`（Guards） | `RequireAuth`（会话）+ `LoadIdentity`（身份与生效权限）+ `RequirePermission`（权限点）+ `RequirePresetGroup`（预设组白名单，owner / admin 这类不入权限目录的服务器级操作用）；常量 `auth.user_id` / `auth.identity` |
| `internal/common/rbac` | `src/common`（权限目录） | 权限点目录与预设组常量（固定 UUID）；纯 Go，不依赖 gin / DB |
| `internal/common` 其余 | `src/common` | `apierror` 统一错误注册与写出、`apiresponse` 共享响应信封；pipes 校验 / interceptors / filters 仍待加（guards 已落地于 `internal/common/guard`） |

## 硬性规则

1. 一个功能一个模块目录 `internal/modules/<name>/`。**有路由的模块必须自包含 module / controller / service / dto；需要 DB 时同目录 `repository.go`。** 无路由的能力模块可省 controller / dto / module.go，只提供实现文件 + `New` 入口 + 测试，不要为凑形状建空文件。
2. 依赖方向单向：controller → service → repository（需要 DB 时同目录 `repository.go`）。禁止反向调用，禁止跨层直连。**例外：`mail` 是无 service 的能力模块，controller 可直接调用注入的 `Mailer`。**
3. Gin 与 HTTP 细节只允许出现在 controller、module.go（挂载路由）、app、`internal/common/guard`（中间件）、`internal/common/apierror`（统一错误写出）和 `internal/common/apiresponse`（共享响应信封）；service 一律不 import gin。
4. 路由由 controller 自己声明；全局前缀 `/api/v1` 只在 `app.go` 定义一次。
5. 模块只通过 `New(...)` 返回的 `*Module` 对外暴露能力（如 service）；跨模块依赖只在 `app.go` 里显式注入，不允许直接构造或调用别的模块的内部实现。唯一例外：为测试提供的包级入口（当前仅 `auth.CreateAdminUser`），业务代码不得调用。**`settings.New(db) (*Module, error)` 仍是唯一返回 error 的模块构造器（清单自检不过启动即失败，错误沿启动链返回）；无状态的包级纯函数可直接导出（如 `mail.Render`）——不算对外暴露内部实现。**
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
| `APP_ENV` | 编译期注入（见「环境与构建」） | 只接受 `development` / `production` / `test`（首尾空白会 trim），其它值含空串**启动即失败**。决定 Gin 模式（`production`/`test` 走 release/test，不打 debug 横幅与路由表）与会话 cookie 的 `Secure`；出现在健康检查响应的 `environment` 字段 |
| `APP_VERSION` | 编译进二进制的版本号 | 仅作运行时临时覆盖（本地调试），常规部署不设置 |
| `SERVICE_NAME` | `website-backend` | 服务名 |
| `DATABASE_URL` | 无（必需） | Postgres 连接串；可写在 `backend/.env`（启动时自动加载，环境变量优先）；`.env` 整个不存在且环境为开发态 / `test` 时首次启动会自动生成一份（见「运行」）；有了文件仍取不到值时**启动即失败**，串本身不可达时照常启动、进降级态（见「运行」的降级启动） |
| `DATABASE_URL_TEST` | 无 | 集成测试专用连接串；不设置时集成用例自动 skip |
| `DB_MAX_OPEN_CONNS` | `10` | 连接池最大连接数 |
| `DB_MAX_IDLE_CONNS` | `5` | 连接池空闲连接数（不得超过最大连接数） |
| `DB_CONN_MAX_LIFETIME` | `30m` | 连接最长存活时间 |
| `SESSION_LIFETIME` | `168h` | 会话默认有效期（`auth.session_max_age_enabled` 关闭时生效；开启时由 `session_max_age_days` 决定）。浏览器 cookie 的 `Max-Age` 跟随当前会话 deadline |
| `SMTP_HOST` | 空 | SMTP 服务器；**为空时邮件只写日志、不影响启动** |
| `SMTP_PORT` | `587` | SMTP 端口（正整数 ≤ 65535；**25 不受支持**——会被 go-mail 视为未设置并改写成 587/465） |
| `SMTP_TLS` | `starttls` | `starttls`（配 587）或 `ssl`（配 465） |
| `SMTP_USERNAME` | 空 | 认证用户名；为空则不做 AUTH（自建中继场景） |
| `SMTP_PASSWORD` | 空 | 认证密码（QQ/163 为授权码）；用户名非空时必填 |
| `SMTP_FROM` | 无 | 发件地址；`SMTP_HOST` 非空时必填 |
| `SMTP_FROM_NAME` | `NeoBlog` | 发件人显示名 |
| `MAIL_TEST_RECIPIENT` | 空 | 仅测试用：与 `SMTP_*` 齐备时真实投递用例会向它发一封自检邮件 |
| `TOTP_ENCRYPTION_KEY` | 无（必需） | 两步验证密钥的 AES-256-GCM 加密钥匙；base64 且解码后恰 32 字节；**开发态 / `test` 下 `.env` 缺失时由后端随机生成并写进新建的 `.env`**（`production` 不自举，缺了就启动即失败），给了但非法仍启动即失败；丢失后已启用 2FA 的用户验码报 500，只能清列重置 |
| `POSTGRES_PASSWORD` | `neoblog` | 本地容器密码（compose 读 `.env`）；改动需同步 `DATABASE_URL` |

## 环境与构建

口径：**构建出来的产物一律是生产环境，只有本地开发才是开发环境**。环境不是构建出两份代码，而是编译期注入一个默认值——`internal/config` 里的 `var DefaultEnvironment`（源码值 `development`），`scripts/build.sh` 用 `-X website/backend/internal/config.DefaultEnvironment=production` 覆盖它。所以源码只有一份，差异全在编译命令。

| 入口 | 解析出的环境 | 启动日志 |
| --- | --- | --- |
| `go run .` / `bash scripts/dev.sh` / 裸 `go build` | `development` | 打印 `[GIN-debug]` 路由表（每条路由一行） |
| `bash scripts/build.sh` | `production` | 只有 `listening on :5732` 一行 |

```bash
bash scripts/build.sh                                  # 本机平台
GOOS=linux GOARCH=amd64 bash scripts/build.sh          # 交叉构建
bash scripts/build.sh bin/mynote.exe                   # 指定输出路径
```

产物开箱即生产态：不打路由表、会话 cookie 带 `Secure`、`GET /api/v1/healthy` 的 `environment` 为 `production`。`APP_ENV` 显式设置时**覆盖**编译期默认值（拿发布二进制本地调试就设 `development`），值非法（含空串）则启动失败并点名 `APP_ENV`。**`APP_ENV` 约定不写进 `.env` 与 `.env.example`**：写进 `.env` 会让同一目录下的发布二进制静默退回开发态，这正是环境开关最不该待的地方（代码仍会读，想在加载层硬拦是后续可选项）。

生产态还关掉了 `.env` 自举：解析出的环境为 `production` 时**不生成**任何配置形状文件，缺 `DATABASE_URL` / `TOTP_ENCRYPTION_KEY` 就按「启动即失败」报错，而不是凭空多出一份带全新随机 2FA 钥匙的开发版 `.env`。开发态与 `test` 保持原自举行为。

两条实测口径，免得再拿日志猜环境：`[GIN-debug]` 路由表**只在启动那一刻打印、且永不上色**；`[GIN] 时间 | 状态码 | 耗时 | IP | 方法 "路径"` 是每条请求都打的访问日志，**debug 与 release 都打**，带不带颜色只取决于 stdout 是不是终端（gin 用 isatty 判，与 `APP_ENV` 无关）。要确认某实例的环境，问 `/api/v1/healthy`。

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

当前迁移清单：`init_users_sessions`（users / sessions）→ `groups_and_permissions`（用户组与权限，含 seed 与回填）→ `totp_and_recovery_codes`（两步验证预留：`users` 加 `totp_secret` / `totp_enabled_at` 两个可空列 + 新表 `recovery_codes`，纯 DDL）→ `setting_values`（设置值单表：复合唯一 `(setting_group, name)` + jsonb `value`，纯 DDL 零数据，全仓第一列 `field.JSON`）→ `totp_last_counter`（`users` 加一列可空 bigint：TOTP 已认领的窗号，用于重放防护，纯 DDL 零数据）→ `totp_setup_at`（`users` 加一列可空 timestamptz：当前待验证密钥的生成时刻，setup 复用窗口用，纯 DDL 零数据）→ `user_bio`（`users` 加一列可空 varchar：用户简介，纯 DDL 零数据）→ `auth_version`（`users` 加一列非空 bigint 默认 0：认证版本，改密与启用 2FA 时 +1，旧版本会话在受保护请求上被 401 并销毁，纯 DDL 零数据）。

## 测试

静态检查（格式 / vet / 编译 / 空白 / repository 错误分层门禁）用 `bash scripts/check.sh`；它不是测试，不会替代下面的全仓测试。

```bash
bash scripts/check.sh                  # 静态检查
bash scripts/test.sh                   # 全仓：确认 DATABASE_URL_TEST 已导出后，串行跑所有包
bash scripts/test.sh ./internal/app/   # 只跑指定包（包名原样传给 go test）
```

**全仓必须经这个脚本跑**：`go test` 默认让包之间并行，而 `register/enabled` 是存在数据库里的**全局开关**——测注册控制的用例会临时把它关掉再打开，同库并行跑的其他包正好撞进这段窗口就会吃到 403（症状是不相干的用例偶发变红）。`scripts/test.sh` 用 `-p 1` 让包串行，消除这个竞态；直接敲 `go test ./...` 仍会偶发红。

脚本**始终**会确认 `DATABASE_URL_TEST` 已设置：不传包名时没设置直接退出；传了包名时只打一行警告再照跑（那种情况下该包的集成用例会静默 skip，`ok` 并不代表跑过）。避免集成用例静默 skip 后假装全绿。

## 认证

只提供后端接口（前端由前端侧接线）。层 1：登录 / 登出 / 当前用户；层 2：公开注册与更改密码。

所有端点的错误响应统一为 `{"error":{"type","code","param"}}`：HTTP 状态码照常；`code` 即下表中的 token；`type` 由状态码唯一决定（400 `invalid_request_error` / 401 `authentication_error` / 403 `permission_error` / 404 `not_found_error` / 405 `invalid_request_error` / 409 `conflict_error` / 500 `server_error` / 503 `server_error`）；`param` 指向出错字段或为 `null`；错误体不含任何文案。唯一例外：`/healthy` 为探针端点豁免本信封，503 降级时仍返回 `Status` 对象。

数据库连不上时，凡是要读写库的端点统一返回 503 `service_unavailable`（由 `internal/common/apierror` 在写出处识别连接级错误得出：`*pgconn.ConnectError`、`driver.ErrBadConn`、`*net.OpError`、`io.EOF` / `io.ErrUnexpectedEOF`，以及 SQLSTATE 类 08 / 53 / 57 / 58）；SQL 逻辑错误（如唯一约束 23505、语法 42601）仍是 500 `internal_error`，不会被降级掩盖。不需要碰库的端点不受影响：请求体校验仍是 400、路径不存在仍是 404、方法不匹配仍是 405。

请求方法不匹配（路径存在但该路径没注册这个方法）返回 405 `method_not_allowed`，并按 RFC 9110 带上 `Allow` 头列出该路径允许的方法；路径本身不存在仍是 404 `not_found`，且此时响应不带 `Allow`。这条分流由 `app.go` 的 `HandleMethodNotAllowed` + `NoMethod` 承担，在认证中间件之前生效，因此匿名请求也能拿到 `Allow`（即该端点存在）；要隐藏端点存在性就把 `HandleMethodNotAllowed` 关回 `false`，届时方法不匹配与路径不存在同为 404。`HEAD` 与 `OPTIONS` 没有任何端点注册，同样落进 405（`HEAD` 按 HTTP 语义响应体必为空，只有 `Allow` 头；将来接跨域时要先给 `OPTIONS` 预检让路）。

| 方法 路径 | 认证 | 说明 |
| --- | --- | --- |
| `POST /api/v1/users` | 公开 | body `{"email","nickname","password"}` → 201 `{"email","nickname","bio"}`，新用户 `bio` 为 `null`；**不下发会话 cookie（注册只建号，要登录得另走 `POST /sessions`）**；400 `invalid_request` / `weak_password`；409 `email_taken` / `nickname_taken` / `already_taken`（并发竞态兜底） |
| `POST /api/v1/sessions` | 公开 | body `{"identifier":"邮箱或昵称","password":"..."}` → 200 + 会话 cookie；401 `invalid_credentials`；400 `invalid_request`；**已启用 2FA 的账号密码正确也不算登录完成**：换发会话后返回 401 `two_factor_required`，会话转入「待二段」状态 |
| `DELETE /api/v1/sessions/current` | 需登录 | 销毁当前会话，返回 200 `{}` |
| `GET /api/v1/users/me` | 需登录 | 返回 `{"email","nickname","bio","group":{"id","system_key","name"},"permissions":["..."],"two_factor_enabled":bool}`（`bio` 未设置时为 `null`；自建组 `system_key` 为 `null`，`permissions` 是展开后的生效权限）；未登录 401 `unauthorized` |
| `PATCH /api/v1/users/me` | 需登录 | body `{"nickname"?,"email"?,"bio"?}`（**字段可选、部分更新，三个都不给 400**）→ 200 `{"email","nickname","bio"}`（更新后的值）；昵称 / 邮箱规则同注册，简介 trim 后 ≤1000 字符（按 rune）、**提交空白值即清空（库中存 `NULL`，响应为 `null`）**，不符 400 `invalid_request`（`param` 指向出问题的字段）；撞名 409 `nickname_taken` / `email_taken`（并发竞态兜底 409 `already_taken`）；**提交与当前相同的值不算撞名**（自己排除在外）；**不要求当前密码**；未登录 401 `unauthorized` |
| `GET /api/v1/users/me/sessions` | 需登录 | 返回 `{"items":[{"id","current","user_agent","ip","created_at","last_seen_at","expires_at"}]}`——只含自己的**活跃**会话（已吊销 / 已过期 / 待二段的都不出现）；`id` 是 `base64url(sha256(token))` 指纹，**原始 token 永不出服务端**；`current` 标记发起请求的这条；时间为 RFC3339 UTC；没有元数据的老会话字段为空串；未登录 401 `unauthorized` |
| `DELETE /api/v1/users/me/sessions/:id` | 需登录 | 按指纹吊销单条（`id` 取自上一条列表）→ 200 `{"id","deleted":true}`；未知 / 他人的 `id` 一律 404 `not_found`（不泄露他人会话存在性）、同 id 二次删也是 404；**删当前这条等于登出**（响应清 cookie）；未登录 401 `unauthorized` |
| `POST /api/v1/users/me/password` | 需登录 | body `{"current_password","new_password"}` → 200 `{}`；旧密码错 401 `invalid_credentials`；400 `weak_password` / `password_unchanged` / `invalid_request` |
| `POST /api/v1/sessions/current/verify` | 公开（靠「待二段」会话） | body `{"code"}` → 200 与登录成功同形（验证码或恢复码皆可）；码错 401 `two_factor_invalid_code`（`param:"code"`）；连错 5 次 401 `two_factor_locked` 并销毁会话；无待验会话 401 `unauthorized`；空 body 400 `invalid_request`；密钥解密失败 500 `internal_error` |
| `POST /api/v1/users/me/2fa/setup` | 需登录 | body `{"password"}` → 200 `{"secret","otpauth_url"}`（明文 secret 仅经本端点暴露——首发与窗口内复用均可能返回明文，二维码由前端渲染）；**生成前校验当前密码**，密码错 401 `invalid_credentials`、空密码 400 `invalid_request`；**生成后 5 分钟内的重复调用复用同一把密钥**（同一账号名下响应逐字节相同、库内零写入；窗口内多设备 / 多标签页也拿到同一把；复用路径解密 / 空明文失败 500 `internal_error`）；超过 5 分钟重新生成并覆盖旧 secret（旧码作废、已认领窗号归零）；已启用 409 `two_factor_already_enabled`；站级 `auth.totp_enabled` 关闭时 403 `two_factor_disabled` |
| `POST /api/v1/users/me/2fa/enable` | 需登录 | body `{"code","password"}` → 200 `{"recovery_codes":[…共 10 条]}`（明文只回这一次）；**验码前校验当前密码**，密码错 401 `invalid_credentials`、空密码 400 `invalid_request`；未 setup 409 `two_factor_not_setup`；码错 401 `two_factor_invalid_code`；**成功吊销该用户其它设备的会话**；密钥解密失败 500 `internal_error`；站级 `auth.totp_enabled` 关闭时 403 `two_factor_disabled` |
| `POST /api/v1/users/me/2fa/disable` | 需登录 | body `{"password"}` → 200 `{}`；清空 secret / 启用时间 / 窗号 / 设置时刻并删除全部恢复码行；「设置中」也允许调（中途放弃）；未 setup 409 `two_factor_not_setup`；密码错 401 `invalid_credentials`；**同样吊销其它会话** |
| `POST /api/v1/users/me/2fa/recovery-codes` | 需登录 | body `{"password"}` → 200 `{"recovery_codes":[…共 10 条]}`；旧批（含已用过的）整体作废；未启用 409 `two_factor_not_enabled`；密码错 401 `invalid_credentials`；不吊销会话（要密码且已登录） |

**路径改名（2026-10-14，0.16.0）**：`/auth/*` 前缀已取消，上表即新口径——原 `POST /auth/register` → `POST /users`、`POST /auth/login` → `POST /sessions`、`POST /auth/logout` → `DELETE /sessions/current`、`GET /auth/me` → `GET /users/me`、`POST /auth/password` → `POST /users/me/password`、`POST /auth/2fa/verify` → `POST /sessions/current/verify`、`POST /auth/2fa/{setup,enable,disable,recovery-codes}` → `POST /users/me/2fa/*`。守卫链与错误契约不变。

规则：

- 注册**公开**、不验证邮箱（邮件基础设施见「邮件」章节，注册流程尚未接入；也没有找回密码）。
- **登录与注册只接受 `Content-Type: application/json`**：带 `Origin` 头的请求，Origin 的 host 必须与 `X-Forwarded-Host`（缺省用请求的 `Host`）一致；不满足一律 400 `invalid_request`。跨站表单发不出 `application/json`，跨站 fetch 会触发预检；非浏览器客户端不带 Origin 时不受影响。前端经 Next rewrite 转发时，`X-Forwarded-Host` 是浏览器访问的前端主机。
- 邮箱：`net/mail` 可解析、≤254 字符、入库统一小写；昵称：trim 后 2–32 字符（按 rune）、不含空白（含全角空格）与 `@`；密码：≥8 字符（按 rune）、同时含 ASCII 字母与数字、≤72 字节（bcrypt 上限）；简介：trim 后 ≤1000 字符（按 rune）、允许换行、可为空。
- 改自己的资料（`PATCH /users/me`）挂 `RequireAuth` + `LoadIdentity`、**不需要权限点**（改的是自己）；字段可选、只改给出的字段（缺省与 `null` 同义，都表示不修改）；昵称 / 邮箱 / 简介沿用注册那套归一与校验、同一批错误码；简介提交空串（或纯空白）即清空、库中存 `NULL`；**不校验当前密码**；会话只存 `userID`，因此改完无需重新登录，`/users/me` 与侧栏下次读取即是新值。
- **更改密码保留当前设备、吊销该用户其他所有设备的会话**（用 scs 的 `IterableStore.All()` + `Codec` 按会话中的 `auth.user_id` 定位删除；失败即 500 且密码不落盘，可用旧密码重试）；密码哈希与 `auth_version` 在同一条 UPDATE 里写入，当前会话随后同步新版本。
- **会话认证版本**：`users.auth_version` 在改密与启用 2FA 时 +1（与凭据写入同一条 UPDATE / 同一事务）；`establishSession` 把当时版本写进会话的 `auth.version`，`LoadIdentity` 每次请求与当前用户版本比对，不一致立即 401 并销毁会话。当前设备在改密 / 启用成功后同步写入新版本，不会踢掉自己；在途登录即使拿旧状态落了库，第一次访问受保护接口也会失效。
- **会话管理（`GET` / `DELETE /users/me/sessions`）**：会话元数据以四个会话数据键随建立写入——`auth.user_agent` / `auth.ip` / `auth.created_at` / `auth.last_seen_at`（RFC3339 UTC 字符串），**零迁移**（仍存 scs 的 `sessions` 表，元数据在 gob 编码的 `data` 里）；两处建立点（单段登录、2FA verify）统一走 `auth.establishSession`，进入「待二段」改走 `enterPendingSession`（清身份、认证版本与元数据，因此待验会话不进任何人的列表）。最后活跃由 `guard.TrackSessionActivity` 维护：**距上次写入不足 5 分钟不写库**（`Put` 不移动 scs 的 deadline，因此不产生滑动续期，7 天上限仍从登录时刻起算）。IP 取 `X-Forwarded-For` 首跳 → `X-Real-IP` → `RemoteAddr`，**仅供展示、不可用作鉴权或限流依据**（UA / IP 分别截断 512 / 64 字节）。列表与吊销共用 `eachSession` 迭代器（全表扫 + 逐条解码后按 `auth.user_id` 过滤）；删当前会话走 `Destroy`（避免本请求的节流写把已删的行提交回写）。

会话由 `scs` 管理、存 Postgres `sessions` 表；cookie 名 `session`、HttpOnly、SameSite=Lax、`Secure` 仅当解析出的环境为 `production`（发布构建默认即是，见「环境与构建」）；登录成功会换发会话 token（注册只建号，不下发会话）。受保护路由在 `app.go` 里经 `guard.RequireAuth → guard.TrackSessionActivity → guard.LoadIdentity` 挂载。账号由注册流程创建；`auth.CreateAdminUser` 仅供测试使用。

**已知缺口**：登录、注册与两步验证 verify **暂无限流**（防刷号 / 防撞库 / 防在线暴破，计划下一轮一起补）；部署时若前端经代理转发，需在 `app.go` 里用 `SetTrustedProxies` 配置真实代理 CIDR，否则 Go 看到的来源 IP 全是代理机、按 IP 限流会失效（会话列表里的 IP 同理只作展示）。另：`sessions.token` 列存**原始 token**（scs 默认），库可读即会话可伪造——scs 自带 `HashTokenInStore` 可改成只存哈希，代价是开启时存量会话全部失效（用户被登出一次），且**三处比较都要改成指纹口径**：① `revokeOtherSessions` 跳过当前会话比的是裸 token（哈希 vs 裸永不相等，会把自己也踢掉）；② 会话列表的 `current` 徽标永不亮；③ 吊销接口判不出「删的是当前会话」，会掉回裸 `Store.Delete` 的复活坑。待点名；会话列表**不保留历史**（吊销即删行，过期由存储层 5 分钟一轮清理），要状态筛选 / 历史需另设登记表（见 `docs/superpowers/specs/2026-10-14-neoblog-session-management-design.md` §9）。

**两步验证（TOTP + 自管恢复码）**：已实现（设计：`docs/superpowers/specs/2026-10-11-neoblog-2fa-design.md`；setup 复用窗口：`docs/superpowers/specs/2026-10-14-neoblog-2fa-setup-reuse-window-design.md`；表结构见上方迁移清单的 `totp_and_recovery_codes`、`totp_last_counter` 与 `totp_setup_at`）。

- 三态由两个可空列组合：`totp_secret` 空 = 未设置，非空且 `totp_enabled_at` 空 = 设置中，两者都非空 = 已启用。**登录分叉只看 `totp_enabled_at`**，「设置中」不拦登录（setup 可重复覆盖，disable 可中途放弃）。
- 参数：TOTP 固定 SHA1 / 6 位 / 30 秒 / ±1 窗；恢复码每批 10 条、每条 16 随机字节的小写 hex（128 bit 熵），库里只存 sha256 hex、无有效期；输入侧的 trim + 转小写只在 `hashRecoveryCode` 一处归一。
- secret 经 AES-256-GCM 加密后入列（密钥 `TOTP_ENCRYPTION_KEY`，随机 12 字节 nonce 前置、整体 base64）。**解密失败或 secret 本身非法 → 500 `internal_error`，且不消耗限次、不销毁会话**——密钥轮换或数据损坏不能伪装成验证码错误。
- **重放防护**：命中的时间窗号必须大于 `totp_last_counter`，并用单条条件 UPDATE 的「影响行数 = 1」原子认领（并发提交同一枚码恰一人成功）。代价：该码所在的 30 秒窗整窗作废——刚验完又登出再登录要等下一窗，启用那一刻也占掉当窗（RFC 6238 §5.2 的标准取舍）。
- 二段限次：会话内计数，连错 5 次销毁会话并返回 `two_factor_locked`；重走登录即续杯，**所以它不是在线暴破防御**，真正的闸门在登录限流那一轮（login 与 verify 同做）。
- 待验态用独立会话键 `auth.pending_2fa_user_id` / `auth.pending_2fa_attempts` 承载，**guard 包零改动**；进入待验态会清除 `auth.user_id`，因此待验会话打任何受保护接口都是 401；登录与 verify 各换发一次会话 token（防会话固定）。
- enable（验证码 + 当前密码）/ disable（当前密码）**在通过之后、落库之前**吊销其它设备会话（复用改密在用的 `revokeOtherSessions`）：怀疑密码泄露才开 2FA 时，攻击者的旧会话必须立刻失效。enable 在同一事务里把 `auth_version` +1，当前会话随后同步新版本；吊销失败则本次动作整体不生效（500、库里不变），不留「已启用但没领到恢复码」的死角。
- 多写操作各自单事务：enable（置启用时间 + 插 10 码）、disable（清三列 + 删全部码行）、recovery-codes（删旧 + 插新）；setup 覆盖 secret 时一并清窗号。

### 用户组与权限

三个预设组（不可删 / 不可改名 / 权限由代码计算，含以后新增的权限点）：`owner` 所有者 与 `admin` 管理员 = 全部权限，`user` 普通用户 = 空；自建组权限存库、可改名可删（有成员时不可删）。生效权限 = 所属组权限 ∪ 用户额外权限（两侧都做 `edit → view` 蕴含展开）；新用户由 ent 的字段级默认值落「普通用户」（迁移无 DB DEFAULT 子句，绕过 ent 的裸 SQL 建号不会自动落组），创建路径不显式设组。

| 方法 路径 | 需要的权限 | 说明 |
| --- | --- | --- |
| `GET /api/v1/admin/permissions` | `system.groups.view` | 权限目录 `{"items":[{"key","name"}]}` |
| `GET /api/v1/admin/groups` | `system.groups.view` | 组列表（`is_preset` / `member_count` / 展开后的 `permissions`） |
| `POST /api/v1/admin/groups` | `system.groups.edit` | 新建自建组 → 201 |
| `PATCH /api/v1/admin/groups/:id` | `system.groups.edit` | 改组名 / 组权限；预设组 → 409 `preset_group_locked` |
| `DELETE /api/v1/admin/groups/:id` | `system.groups.edit` | 删自建组 → 200 `{"id","deleted":true}`；有成员 → 409 `group_not_empty` |
| `GET /api/v1/admin/users` | `system.users.view` | 用户列表（带所属组与额外权限） |
| `GET /api/v1/admin/users/:id` | `system.users.view` | 用户详情（另含 `permissions` = 生效权限） |
| `PATCH /api/v1/admin/users/:id` | `system.users.edit` | `{"group_id"?,"extra_permissions"?}`（额外权限为全量替换，`[]` 清空）；两个字段都不给 → 400 |

权限点命名 `模块.动作`，动作只有 `view` / `edit`（`edit` 蕴含 `view`）；目录是代码常量（`internal/common/rbac`），随模块落地逐条增加，不存在「目录里没有检查点」的空权限。当前目录四对：`system.users`、`system.settings`（设置页「常规」）、`system.register`（设置页「用户注册」）、`system.groups`，各含 `view` / `edit`（顺序仅决定 `/admin/permissions` 的返回顺序；前端矩阵的行顺序、分组与显隐在前端 `frontend/lib/groups.ts` 的分组表里定，与目录顺序解耦）；设置接口按组挂点：`site` → `system.settings.*`，`register` / `auth` → `system.register.*`。

规则：

- **只能授出自己持有的权限**：新建 / 改组、设用户组、加额外权限时，目标新增（按生效权限集的前后差集、同一请求合并计算）而操作者没有的点 → 403 `grant_forbidden`；**摘除不受限**（删权限不会提权）。
- **不能改自己**（自己的组与额外权限都算）→ 403 `cannot_modify_self`；**owner / admin 组的成员只有 owner 能改** → 403 `owner_protected`。
- **至少保留一个 owner**：并发双降级的第二个请求 → 409 `last_owner`（事务内 `FOR UPDATE` 锁 owner 成员行后计数；串行路径由上面两条规则挡住）。
- 组名 trim 后 1–64 字符；权限数组先去重、目录外的 key → 400 `invalid_permission`；组名重复 → 409 `group_name_taken`。
- 守卫链：`RequireAuth`（未登录 401）→ `LoadIdentity`（解析身份与生效权限放进 `auth.identity`；用户已删除则销毁会话 + 401）→ `RequirePermission`（权限不足 403 `forbidden`）。**原则变更**：守卫不再是「不查库」，而是「只依赖 `app.go` 注入进来的 resolver，不 import 业务模块」。
- 权限改动**下一次请求即生效**（不吊销会话）；已在处理中的请求使用请求开始时的权限快照。
- **首个 owner 只能手工指定**（迁移把存量用户全部回填为「普通用户」）：`UPDATE users SET group_id = '00000000-0000-4000-8000-000000000001' WHERE email = '你的邮箱';`——owner / admin / 普通用户的固定 UUID 依次是 `…0001` / `…0002` / `…0003`（与 `internal/common/rbac` 一致）。

## 邮件

后端发信能力 + 管理端发信接口。`internal/modules/mail` 对外暴露 `*Module`（`New(cfg) *Module`，`m.Mailer` 是 `Mailer` 接口）与 `Render`，以及 `Message` / `Body` 数据结构和哨兵 `ErrClosed`：

```go
body, err := mail.Render("example", map[string]string{"Time": "...", "Site": "NeoBlog"})
mailer.Send(ctx, mail.Message{To: addr, Subject: "NeoBlog 邮件配置自检", Body: body})
```

### 管理端接口

| 方法 路径 | 认证 | 说明 |
| --- | --- | --- |
| `POST /api/v1/admin/mail/messages` | 登录 + 预设组守卫 | body `{"to":"收件邮箱"}` → 用当前 SMTP 配置**同步真发**一封 `example` 自检邮件 → 201 `{"to","subject","sent":true}`；收件地址不合法 400 `invalid_request`（`param:"to"`）；非 owner / admin 403 `forbidden`；SMTP 投递失败 500 `mail_delivery_failed` |

规则：

- 守卫链 `RequireAuth` → `LoadIdentity` → `RequirePresetGroup(owner, admin)`。**不走权限目录**：SMTP 配置是服务器级设置（环境变量），不属于可授出的业务权限域；owner / admin 天然持有全部权限，自建组不可授。
- 投递走 `SendSync`：**单次尝试、不重试**；死线为拨号 20s（go-mail 对每条 SMTP 命令续期死线，**慢速对端总耗时可超 20s**，不是硬性总上限），失败时操作者可自行重试；失败细节看服务端结构化日志。收件地址经 `net/mail.ParseAddress` 解析（支持 `Name <addr>` 形式，取纯地址），主题与正文由服务器控制（`example` 模板），请求方不能指定内容。
- 开发环境（`SMTP_HOST` 为空）降级为日志实现，接口永远 201（邮件全文在日志里，不真发）。

### 发信能力

- `Send` 只保证入队（缓冲 32），投递由后台 worker 串行执行：每封最多 3 次尝试、退避 1s / 5s、单次投递拨号死线 20s（go-mail 逐命令续期，非总上限）；**服务端 4xx 与连接类错误重试，5xx 不重试**，失败记结构化日志（`log/slog`）。业务流程（如验证邮件）一律用它。
- `SendSync` 是同步单次投递，当前仅管理端自检接口使用。
- `Close(ctx)` 停收新消息并把队列投完（`app.Run` 关闭路径中先 `server.Shutdown` 再 `mailer.Close`，共用 10s 预算）；`Close` 之后 `Send` / `SendSync` 返回 `mail.ErrClosed`。
- 模板：`internal/modules/mail/templates/<name>.html` 与 `<name>.txt` 成对（HTML 侧用 `html/template` 自动转义）；内置一封 `example`（邮件配置自检）。
- `SMTP_HOST` 为空时降级为日志实现：邮件全文写进日志但不发送，开发环境无需凭据即可跑通全流程。
- 真实投递测试（可选）：`backend/.env` 填好 `SMTP_*` 与 `MAIL_TEST_RECIPIENT` 后跑 `go test ./internal/modules/mail/ -run TestRealDelivery -v`（测试会从包目录向上查找 `.env`，深度上限 5，不写死相对路径）。
- 上线前按所选服务商配好 SPF / DKIM / DMARC（域验类服务商侧签 DKIM）；发件域名必须与认证域名对齐，**不要用自有域名 From 搭配 QQ 个人邮箱认证**。细节见 `docs/superpowers/specs/2026-10-11-neoblog-mail-design.md` §11。

## 设置

模块 `internal/modules/settings/`：一张 `setting_values` 表只存"被显式写入过的值"，"有哪些设置"全部定义在代码内清单（`registry.go` 的 `builtinRegistry`）。构造入口 `settings.New(db) (*Module, error)`——清单自检不过启动即失败，能力挂 `m.Service`；错误哨兵 `ErrUnknownGroup` / `ErrUnknownItem` / `ErrTypeMismatch` / `ErrMethodMismatch`。三种类型（文字 / 数字 int64 / 开关）读写各按类型分方法，用错方法即报错；`GetGroup` 整组读取（默认值 ⊕ 覆盖逐项合并）；`PatchGroup` 整组部分更新（值类型以清单为准，任一项不合法即整组零落库）；`ResetItem` 删行恢复默认。库里没有某项的行 = 取代码里的默认值。

**响应信封（两侧统一）**：一个组的记录固定是 `{ "group": "<组名>", "values": { …整组合并结果… } }`；集合外面套 `{ "items": [ 组记录… ] }`，与 `access` 模块的 `items` 列表同形。`values` 里是「默认值 ⊕ 覆盖」的合并结果，键是项名。

**对外接口**：

| 端点 | 鉴权 | 返回 |
| --- | --- | --- |
| `GET /api/v1/admin/settings/:group` | `site` 组：`system.settings.view`；`register` / `auth` 组：`system.register.view` | 一条组记录（**全部项**，含未标公开的） |
| `PATCH /api/v1/admin/settings/:group` | 同上两把钥匙的 `edit` 档（`system.settings.edit` / `system.register.edit`，隐含 `view`） | 更新后的组记录 |
| `GET /api/v1/public/settings` | **无鉴权** | `{items: […]}`，只含**至少有一项标了 `Public` 的组**，且每组只出公开项 |

管理侧未知组 404；请求体非法、项名不在清单、值类型不符 → 400（带 `param` = 项名）；组级组合约束不满足（`password_min_length` 取 8–64、`session_max_age_days` 取 1–30）→ 409 `setting_conflict`（`param` = 判定项名，未必是要改的那一项）。owner / admin 由权限目录运行时全量授予全部权限点，无需数据迁移；自建组需显式授。公开侧按组点查**刻意不提供**——前台要读公开设置就一次取全，避免每加一项就多一个 fetcher。

**当前清单项**：① `site` 组六项——`title` / `subtitle` / `description` / `keywords` / `url`（均为文字、默认空串）与 `block_search_engines`（开关、默认 `false`）；六项全部标 `Public`（供前台读站名、SEO 与 robots 元数据；`keywords` 存英文逗号分隔的单个字符串；后端暂无消费方）；② `register` 组五项——`enabled`（开关，默认 `true`，已标 `Public`）是注册总闸：`auth.Service.Register` 的第一句读它，为 `false` 时注册返回 403 `registration_closed`，登录与 `auth.CreateAdminUser` 不受它影响；`email_domain_control_enabled`（开关，默认 `false`）与 `allowed_email_domains`（文字，逗号分隔、中英文逗号都认）是注册邮箱域名白名单：开关开启且清单条目非空时 `auth.Service.Register` 拒绝域名不在清单里的邮箱（400 `email_domain_not_allowed`；大小写不敏感、精确匹配域名、不放行子域），只拦公开注册，`CreateAdminUser` 与 `PATCH /users/me` 不受影响；清单为空（含只填逗号、全空白这类拆分后无条目的写法）时一律放行，也就是"开着但不限制任何域名"是一种合法且能存得下的配置，管理界面需自行提示这层含义；`default_group`（文字，默认**普通用户组的组名**「普通用户」，取自 `rbac.PresetGroupNameByKey(rbac.GroupKeyUser)`）是注册落到哪个用户组的设定，**值形状是组名不是 UUID 也不是组键**——控制台「用户注册」页的「注册默认组」下拉列的就是它，可选项按 `system_key` 排除 `owner` / `admin`（注册是自助入口，指到特权组等于开放提权后门）；它与 `email_verification_enabled` 一样**暂无消费方**（`auth.Service.Register` 不读它，新用户归属仍由 ent schema 默认值决定），组名被改或组被删会让存量值失配；两者均非公开；③ `auth` 组九项——`password_requirements_enabled`（开关，默认 `false`）与 `password_min_length`（默认 `8`，取值 8–64）是密码策略：开启后 `POST /users` 与 `POST /users/me/password` 按 `password_min_length` 校验（关闭时沿用写死的 8；字母+数字与 ≤72 字节恒生效），`PATCH /admin/settings/auth` 拒绝 8–64 之外的写入（409 `setting_conflict`）；`session_max_age_enabled`（开关，默认 `false`）与 `session_max_age_days`（默认 `7`，单位天，取值 1–30）是会话有效期：开启后 `auth.Service.establishSession` 在登录（含二段验证通过）时把会话 deadline 设为「当前时间 + N 天」，只影响之后新建的会话，已登录会话不变；关闭时沿用 `SESSION_LIFETIME`（默认 168h = 7 天）；`PATCH /admin/settings/auth` 拒绝 1–30 之外的写入（409 `setting_conflict`）；天数只在本次写了它、或把开关写成开启时才校验（开启时连同存量值一起校验），关掉开关、或写上组内其他键都不会被历史脏天数拦住。`totp_enabled`（开关，默认 `true`，已标 `Public`）是两步验证自助开关（控制台标签「允许用户启用两步验证」）：为 `false` 时 `POST /users/me/2fa/setup` 与 `/enable` 返回 403 `two_factor_disabled`；`/disable` 与恢复码接口不受影响；登录是否要求二段仍只看用户自己的 `totp_enabled_at`，已开启 2FA 的用户不会因站级开关关闭而绕过验证。`password_complexity`、`password_reset_enabled`、`captcha_enabled`、`captcha_provider` 仍非公开、纯数据、暂无消费方；其中 `captcha_provider` 由「登录控制」页每次保存**无条件**带上当前显示值（空串显示为 `builtin`），所以任何一次真实保存都会把它落成 `builtin`，而不是像早期那样永远停在空串。前端 `lib/public-settings.ts` 读聚合端点，`/signup` 在关闭时直接 404、登录页隐掉注册入口；**读不到该端点时按 fail-open 处理**（照常出页面），与「后端连不上照常出页 + toast」的既有口径一致。关掉注册的实际操作（暂无界面）：

```
GET   /api/v1/public/settings                →  {"items":[{"group":"register","values":{"enabled":false}}]}
PATCH /api/v1/admin/settings/register        →  {"group":"register","values":{"enabled":false}}
```

（两行响应都是节选：`GET` 的完整负载里 `site` 组六项、`register` 组的 `enabled` 和 `auth` 组的 `totp_enabled` 都在；`PATCH` 回的是整组合并结果，`register` 另四项非公开策略一并回显。）

**怎么加一个设置**：在 `internal/modules/settings/registry.go` 的 `builtinRegistry` 里添加（组不存在就新增一个 `Group` 字面量）：

```go
var builtinRegistry = []Group{
	{
		Name: "demo",
		Items: []Definition{
			{Name: "title", Type: FieldTypeString, Default: "我的博客"},
			{Name: "port", Type: FieldTypeNumber, Default: int64(8080)},
			{Name: "maintenance", Type: FieldTypeBool, Default: false},
		},
	},
}
```

保存后重启即生效：新设置立刻可读（默认值），**不需要任何数据库操作**。名字规则 `^[a-z][a-z0-9_]*$`；数字默认值必须写 `int64(...)`。防漂移测试 `TestRegistryRecipeShape` 按本配方字面形状把关，配方改了它就红。

**怎么加一个公开设置**（前台匿名要读的）：在上面的写法里多带一个 `Public: true`——

```go
{Name: "title", Type: FieldTypeString, Default: "我的博客", Public: true},
```

它会自动出现在 `GET /api/v1/public/settings` 里，**后端不需要加端点、不需要加 service 方法**。前端只动两处：`lib/public-settings.ts` 里加一个纯选择器（照 `isSignupOpen` 的写法，读 `settings.<组>[.<项>] ?? 默认`），页面消费它。守卫测试 `TestBuiltinPublicAllowlist` 钉住「公开项集合 == 期望清单」，把敏感项误标成 `Public` 会让它先红。

**怎么加一条组级保存校验**（保存时拦住组合非法的写入）：给 `Group` 字面量同时加 `ValidateOn`（管辖项：只有改动这些项的 `PATCH` 才触发校验）与 `Validate`（拿到「存量 ⊕ 本次写入」的合并最终态；不合规返回 `*ItemError{Item: 判定项名, Err: apierror.ErrSettingConflict}`）：

```go
{
	Name:       "demo",
	ValidateOn: []string{"enabled", "list"},
	Validate:   validateDemoGroup,
	Items: []Definition{
		{Name: "enabled", Type: FieldTypeBool, Default: false},
		{Name: "list", Type: FieldTypeString, Default: ""},
	},
}
```

两者必须成对（只写 `Validate` 构造即报错，管辖项不存在也报错）。校验只覆盖 `PATCH /admin/settings/:group`（唯一 HTTP 写路径）；`Set*` / `SetGroup` / `ResetItem` 直写不校验（测试与探针通道）。冲突回 409 `setting_conflict`，`param` 指向**判定项**（未必是要改的那一项）；未触碰管辖项的写入不读合并态、零开销。

## 运行

在 `backend/` 目录下（`go run .` 与 `dev.sh` 解析出的都是**开发环境**，发布产物见「环境与构建」）：

```bash
go run .                # 启动时自动加载工作目录下的 .env；默认 :5732（推荐）
bash scripts/dev.sh     # 同上（先定位到 backend/，可在任意目录执行）
ADDR=:9090 go run .     # 自定义端口
```

**首次启动自动建 `.env`（仅开发态 / `test`）**：`main.go` 在读配置之前先调 `config.EnsureDotEnv()`——工作目录没有 `.env` 就写一份（`DATABASE_URL` / `DATABASE_URL_TEST` 按 compose 的本地库、`POSTGRES_PASSWORD`、以及随机生成的 32 字节 `TOTP_ENCRYPTION_KEY`），并打一条 `created .env …` 日志。创建用 `O_EXCL`，**已有文件绝不覆盖**，第二次启动只是照常加载。所以「忘了配 `.env`」不再让进程退出：`go run .` / `bash scripts/dev.sh` 开箱即跑。文件按 0600 请求创建，Windows 上由 ACL 决定实际权限（本机显示为 644）。**发布产物（默认 `production`）不自举**：没有 `.env` 就不造文件，缺配置按「启动即失败」报错，避免在运行目录里凭空长出一份带全新随机 2FA 钥匙的开发配置（见「环境与构建」）。

两件事要跟着一并记住：生成的那把 2FA 钥匙只活在这份文件里，文件没了等于钥匙丢了（已启用 2FA 的账号验码一律 500），所以开发 / 测试容器每次重建都会换新钥匙（发布产物不自举，不会凭空长出新钥匙）。生产的配置只能来自持久化的 `.env`（已存在就照常加载、绝不覆盖）或显式环境变量；自动写入的连接串是本地 compose 的形状，只适合本机，串本身不可达时不会退出，而是按下面的降级启动跑。

健康检查：`GET /api/v1/healthy` → `{"status":"ok","service":"website-backend","version":"<当前版本>","environment":"development","database":"ok","uptime":"3s","time":"2026-10-11T01:35:12+08:00"}`。数据库 Ping 不通时返回 HTTP 503，`status` 为 `degraded`、`database` 为 `error`。

**降级启动**：`database.Open` 只在驱动 / DSN 语法层面失败才返回错误；首次 Ping 失败被记在 `Database.StartupError()` 里，`main.go` 打一条 `degraded start: …` 日志后照常监听端口，所有路由照常注册。要读写库的端点在此期间统一 503 `service_unavailable`（判定见「认证」章节的错误信封段），连不上那一刻起的请求不会退化成 401 或 HTML 错误页。连接池是惰性的，库恢复后无需重启进程：下一次取连接即成功（已实测：进程不变，`/healthy` 从 503 翻回 200、登录从 503 回到 401 `invalid_credentials`）。

已知取舍：会话本身存在 Postgres（scs 的 `postgresstore`），库不可用时 scs 把「读不到会话」与「没有会话」一视同仁，带 cookie 打受保护端点仍是 401 `unauthorized`，不是 503。
