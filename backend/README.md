# website 后端（Go + Gin）

当前版本：0.0.2

设计规范对齐 NestJS：模块化组织、单向依赖、显式依赖注入；应用生命周期（监听、超时、优雅关闭）由 app 层掌管，对应 Nest 的 `app.listen` / `enableShutdownHooks`。Go 没有装饰器和反射式 DI 容器，对应物是构造函数传依赖、模块 `New` 暴露能力、`Register` 装配。

## 目录与 NestJS 对照

| 路径 | NestJS 对应 | 职责 |
| --- | --- | --- |
| `main.go` | `main.ts` | 启动引导：加载配置、创建应用、启动 |
| `internal/app/app.go` | `app.module.ts` + `setGlobalPrefix` | 组装模块、全局前缀 `/api/v1`、HTTP 服务器（超时 / 优雅关闭） |
| `internal/config` | `ConfigModule` | 读环境变量，产出 `Config` |
| `internal/version` | `package.json` 的 `version` | 后端版本号唯一来源 |
| `scripts/bump-version.sh` | `npm version` | 版本号进位工具 |
| `internal/modules/health/module.go` | `health.module.ts` | 装配本模块，`New` 返回 `*Module` 并暴露对外能力 |
| `internal/modules/health/controller.go` | `health.controller.ts` | 本模块的路由声明与 HTTP 编解码（包内私有） |
| `internal/modules/health/service.go` | `health.service.ts` | 业务逻辑，模块对外暴露的能力 |
| `internal/modules/health/dto.go` | `dto/*.dto.ts` | 请求/响应数据结构 |
| `internal/common`（待加） | `src/common` | guards 认证 / pipes 校验 / interceptors / filters 统一错误 |

## 硬性规则

1. 一个功能一个模块目录 `internal/modules/<name>/`，自包含 module / controller / service / dto。
2. 依赖方向单向：controller → service → repository（需要 DB 时同目录 `repository.go`）。禁止反向调用，禁止跨层直连。
3. Gin 与 HTTP 细节只允许出现在 controller、module.go（挂载路由）和 app；service 一律不 import gin。
4. 路由由 controller 自己声明；全局前缀 `/api/v1` 只在 `app.go` 定义一次。
5. 模块只通过 `New(cfg)` 返回的 `*Module` 对外暴露能力（如 service）；跨模块依赖只在 `app.go` 里显式注入，不允许直接构造或调用别的模块的内部实现。
6. 代码零注释（只有 Go 工具链自动生成的注释例外）；缩进一律走 gofmt。

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

## 运行

在 `backend/` 目录下：

```bash
go run .                # 默认 :5732
ADDR=:9090 go run .     # 自定义端口
```

健康检查：`GET /api/v1/healthy` → `{"status":"ok","service":"website-backend","version":"0.0.1","environment":"development","uptime":"3s","time":"2026-10-11T01:35:12+08:00"}`
