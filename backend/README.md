# website 后端（Go + Gin）

当前版本：0.2.0

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

第一版（层 1）：登录 / 登出 / 当前用户，只做后端。账号只有管理员一个；本轮不提供注册或建号入口，`auth.CreateAdminUser` 目前仅供测试使用（首次建号方式另行决定）。

| 方法 路径 | 认证 | 说明 |
| --- | --- | --- |
| `POST /api/v1/auth/login` | 公开 | body `{"identifier":"邮箱或昵称","password":"..."}` → 200 `{"email","nickname"}` + 会话 cookie；凭据错 401 `invalid_credentials`；请求不合法 400 `invalid_request` |
| `POST /api/v1/auth/logout` | 需登录 | 销毁会话，返回 204 |
| `GET /api/v1/auth/me` | 需登录 | 返回 `{"email","nickname"}`；未登录 401 `unauthorized` |

会话由 `scs` 管理、存 Postgres `sessions` 表；cookie 名 `session`、HttpOnly、SameSite=Lax、`Secure` 仅当 `APP_ENV=production`；登录成功换发会话 token（防会话固定）。受保护路由在 `app.go` 里经 `guard.RequireAuth` 挂载。

## 运行

在 `backend/` 目录下：

```bash
go run .                # 启动时自动加载工作目录下的 .env；默认 :5732（推荐）
bash scripts/dev.sh     # 同上（先定位到 backend/，可在任意目录执行）
ADDR=:9090 go run .     # 自定义端口
```

健康检查：`GET /api/v1/healthy` → `{"status":"ok","service":"website-backend","version":"0.2.0","environment":"development","database":"ok","uptime":"3s","time":"2026-10-11T01:35:12+08:00"}`。数据库 Ping 不通时返回 HTTP 503，`status` 为 `degraded`、`database` 为 `error`。
