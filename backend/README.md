# website 后端（Go + Gin）

当前版本：0.4.0

设计规范对齐 NestJS：模块化组织、单向依赖、显式依赖注入；应用生命周期（监听、超时、优雅关闭）由 app 层掌管，对应 Nest 的 `app.listen` / `enableShutdownHooks`。Go 没有装饰器和反射式 DI 容器，对应物是构造函数传依赖、模块 `New` 暴露能力、`Register` 装配。

## 目录与 NestJS 对照

| 路径 | NestJS 对应 | 职责 |
| --- | --- | --- |
| `main.go` | `main.ts` | 启动引导：加载配置、创建应用、启动 |
| `internal/app/app.go` | `app.module.ts` + `setGlobalPrefix` | 组装模块、全局前缀 `/api/v1`、HTTP 服务器（超时 / 优雅关闭） |
| `internal/config` | `ConfigModule` | 读环境变量，产出 `Config` |
| `internal/modules/mail` | `@nestjs-modules/mailer`（对照位） | 邮件发送基础设施：`Mailer` 接口（Send / Close）+ SMTP / 日志双实现 + `templates/` 模板；`SMTP_HOST` 为空时降级为日志 |
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
| `internal/modules/auth/*` | `auth.module.ts` 等 | 登录 / 登出 / 当前用户；`Register`（公开）与 `RegisterProtected`（受保护） |
| `internal/common/guard` | `src/common`（Guards） | `RequireAuth` 会话守卫中间件；会话键常量 `auth.user_id` |
| `internal/common` 其余（待加） | `src/common` | pipes 校验 / interceptors / filters 统一错误（guards 已落地于 `internal/common/guard`） |

## 硬性规则

1. 一个功能一个模块目录 `internal/modules/<name>/`，自包含 module / controller / service / dto。
2. 依赖方向单向：controller → service → repository（需要 DB 时同目录 `repository.go`）。禁止反向调用，禁止跨层直连。
3. Gin 与 HTTP 细节只允许出现在 controller、module.go（挂载路由）、app 和 `internal/common/guard`（中间件）；service 一律不 import gin。
4. 路由由 controller 自己声明；全局前缀 `/api/v1` 只在 `app.go` 定义一次。
5. 模块只通过 `New(...)` 返回的 `*Module` 对外暴露能力（如 service）；跨模块依赖只在 `app.go` 里显式注入，不允许直接构造或调用别的模块的内部实现。唯一例外：为测试提供的包级入口（当前仅 `auth.CreateAdminUser`），业务代码不得调用。
6. 代码零注释（只有 Go 工具链自动生成的注释例外）；缩进一律走 gofmt。
7. 模块禁止自建数据库连接（禁止 `sql.Open` 等），只能用注入进来的连接。
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

应用启动绝不自动迁移；`ATLAS_DEV_URL` 缺省指向空的 `neoblog_atlas_dev`（脚本内默认值）。工具链细节与已验证事实见 `docs/superpowers/specs/2026-10-11-neoblog-data-layer-design.md` 第 6、7 节。

## 认证

只提供后端接口（前端由前端侧接线）。层 1：登录 / 登出 / 当前用户；层 2：公开注册与更改密码。

| 方法 路径 | 认证 | 说明 |
| --- | --- | --- |
| `POST /api/v1/auth/register` | 公开 | body `{"email","nickname","password"}` → 201 `{"email","nickname"}` + 会话 cookie（注册即登录）；400 `invalid_request` / `weak_password`；409 `email_taken` / `nickname_taken` / `already_taken`（并发竞态兜底） |
| `POST /api/v1/auth/login` | 公开 | body `{"identifier":"邮箱或昵称","password":"..."}` → 200 + 会话 cookie；401 `invalid_credentials`；400 `invalid_request` |
| `POST /api/v1/auth/logout` | 需登录 | 销毁会话，返回 204 |
| `GET /api/v1/auth/me` | 需登录 | 返回 `{"email","nickname"}`；未登录 401 `unauthorized` |
| `POST /api/v1/auth/password` | 需登录 | body `{"currentPassword","newPassword"}` → 204；旧密码错 401 `invalid_credentials`；400 `weak_password` / `password_unchanged` / `invalid_request` |

规则：

- 注册**公开**、不验证邮箱（邮件基础设施见「邮件」章节，注册流程尚未接入；也没有找回密码）。
- 邮箱：`net/mail` 可解析、≤254 字符、入库统一小写；昵称：trim 后 2–32 字符（按 rune）、不含空白（含全角空格）与 `@`；密码：≥8 字符（按 rune）、同时含 ASCII 字母与数字、≤72 字节（bcrypt 上限）。
- **更改密码保留当前设备、吊销该用户其他所有设备的会话**（用 scs 的 `IterableStore.All()` + `Codec` 按会话中的 `auth.user_id` 定位删除；失败即 500 且密码不落盘，可用旧密码重试）。

会话由 `scs` 管理、存 Postgres `sessions` 表；cookie 名 `session`、HttpOnly、SameSite=Lax、`Secure` 仅当 `APP_ENV=production`；登录与注册成功都会换发会话 token（防会话固定）。受保护路由在 `app.go` 里经 `guard.RequireAuth` 挂载。账号由注册流程创建；`auth.CreateAdminUser` 仅供测试使用。

**已知缺口**：登录与注册**暂无限流**（防刷号 / 防撞库，计划下一轮优先补）；部署时若前端经代理转发，需在 `app.go` 里用 `SetTrustedProxies` 配置真实代理 CIDR，否则 Go 看到的来源 IP 全是代理机、按 IP 限流会失效。所有登录用户当前权限等同（还没有管理员专属接口，角色区分留到需要时）。

## 邮件

只做后端基础设施，**当前没有任何模块消费它**（注册不验证邮箱）。`internal/modules/mail` 对外只暴露 `Mailer` 接口与 `Render`：

```go
body, err := mail.Render("example", map[string]string{"Time": "...", "Site": "NeoBlog"})
mailer.Send(ctx, mail.Message{To: addr, Subject: "NeoBlog 邮件配置自检", Body: body})
```

- `Send` 只保证入队（缓冲 32），投递由后台 worker 串行执行：每封最多 3 次尝试、退避 1s / 5s、单次 20s 超时；**服务端 4xx 与连接类错误重试，5xx 不重试**，失败记结构化日志（`log/slog`）。
- `Close(ctx)` 停收新消息并把队列投完（`app.Run` 关闭路径中先 `server.Shutdown` 再 `mailer.Close`，共用 10s 预算）；`Close` 之后 `Send` 返回 `mail.ErrClosed`。
- 模板：`internal/modules/mail/templates/<name>.html` 与 `<name>.txt` 成对（HTML 侧用 `html/template` 自动转义）；内置一封 `example`（邮件配置自检）。
- `SMTP_HOST` 为空时降级为日志实现：邮件全文写进日志但不发送，开发环境无需凭据即可跑通全流程。
- 真实投递测试（可选）：`backend/.env` 填好 `SMTP_*` 与 `MAIL_TEST_RECIPIENT` 后跑 `go test ./internal/modules/mail/ -run TestRealDelivery -v`。
- 上线前按所选服务商配好 SPF / DKIM / DMARC（域验类服务商侧签 DKIM）；发件域名必须与认证域名对齐，**不要用自有域名 From 搭配 QQ 个人邮箱认证**。细节见 `docs/superpowers/specs/2026-10-11-neoblog-mail-design.md` §11。

## 运行

在 `backend/` 目录下：

```bash
go run .                # 启动时自动加载工作目录下的 .env；默认 :5732（推荐）
bash scripts/dev.sh     # 同上（先定位到 backend/，可在任意目录执行）
ADDR=:9090 go run .     # 自定义端口
```

健康检查：`GET /api/v1/healthy` → `{"status":"ok","service":"website-backend","version":"0.3.0","environment":"development","database":"ok","uptime":"3s","time":"2026-10-11T01:35:12+08:00"}`。数据库 Ping 不通时返回 HTTP 503，`status` 为 `degraded`、`database` 为 `error`。
