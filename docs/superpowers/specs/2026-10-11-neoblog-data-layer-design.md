# NeoBlog 数据层设计（层 0：连接与配置；ent 与迁移随首张表）

日期：2026-10-11
范围：`backend/`（Go + Gin）的数据访问基础设施
状态：设计经用户逐项确认；**v2 修订**已按真机验证结果收窄范围（就绪待用户审阅，之后进实施计划）

修订记录：
- v1（2026-10-11）：初版。选型 ent + Atlas 版本化迁移 + 独立容器。
- v2（2026-10-11）：经真机验证后修订——ent 代码生成必须有至少一张表才工作，故本轮范围为"层 0"（连接 + 健康检查 + 容器脚本），ent 与迁移工具链推迟到第一张业务表那轮一起落地；Atlas CLI 安装方式改为官方 Windows 二进制；新增第 7 节"已验证的工具链事实"。

## 1. 意图与背景

NeoBlog 后端（`backend/`，Go 1.27 + Gin，模块化组织对齐 NestJS）目前只有 health 模块，零数据库代码。数据层分两步落地：

- **本轮（层 0）**：连接的注入点与配置、健康检查集成、本地数据库容器与脚本。可独立验证、不依赖任何业务表。
- **下一轮（随第一张业务表，如 auth 的 users）**：ent 代码生成管线 + Atlas 版本化迁移 + 数据层对外提供 `*ent.Client`。施工依据是本文档第 6、7 节（已真机验证）。

对齐来源（不重新发明）：

- `F:\protect\Neo`（NestJS + Prisma 的平行项目）已定：Postgres + 版本化迁移进 Git；数据层定位为"数据连接的注入点与迁移入口"，**不隔仓储接口层**，业务边界靠出参 DTO 守。本设计沿用同一立场。
- 用户在选型对话中明确要 Prisma 那种"schema → 生成类型安全 client → 迁移"的体验。Go 侧没有 Prisma（其官方实验 Go client 已于 2025-07-28 归档、明确不再支持 Prisma v7+），最接近的是 **ent**（推迟到下一轮接入）。
- 数据库定为 PostgreSQL 17（与 neo-postgres 一致）。

## 2. 已定决策

| 项 | 决定 | 落地时点 |
| --- | --- | --- |
| 驱动 | `jackc/pgx/v5` 的 stdlib（`sql.Open("pgx", dsn)`） | 本轮 |
| 数据库 | PostgreSQL 17（`postgres:17-alpine`） | 本轮 |
| 本地实例 | 独立容器 `neoblog-postgres`，端口 **5434**（已确认空闲；5432 是 D 盘项目、5433 是 neo-postgres） | 本轮 |
| 连接形态 | `database.Open/Ping/Close`，`Database` 结构持有 `*sql.DB`；启动连不上即失败（fail fast） | 本轮 |
| 数据层形态 | 连接注入点 + 迁移入口，不做仓储接口层 | 本轮起 |
| 数据访问库 | `entgo.io/ent`（v0.14.6 起） | **下一轮**（第一张表） |
| 迁移 | Atlas CLI（官方 Windows 二进制）+ 版本化 SQL 进 Git；应用启动绝不自动迁移 | **下一轮**，依赖第 7 节已验证事实 |
| 开发期建表 | 不用 ent 自动建表；唯一例外是测试里 enttest（下一轮定） | 下一轮 |

## 3. 范围

**本轮做**：`internal/database`（Open / Ping / Close + 连接池）、`internal/config` 增 `DatabaseURL` 与池参数、health 模块的数据库健康检查、`docker-compose.yml` 与建库初始化、`scripts/db-up.sh` 与 `scripts/dev.sh`、`.env` 体系、单元与集成测试骨架、README 更新。

**本轮不做**：ent 代码生成与 `*ent.Client`（无表则无法生成，见第 7.2）、`migrations/` 与 Atlas 脚本（随第一张表）、任何业务表、auth 实现、前端、部署形态、CI。

## 4. 目录与职责

```
backend/
├── docker-compose.yml              只含 db 服务（本地开发）
├── deploy/postgres/init.sql        首次初始化：建 neoblog_test 与 neoblog_atlas_dev
├── .env / .env.example             DATABASE_URL 等（.env 不进 Git）
├── .gitignore                      新增：.env
├── scripts/
│   ├── dev.sh                      读取 .env 后 go run .（Go 不自动读 .env）
│   └── db-up.sh                    起本地 db 容器
└── internal/
    ├── config/config.go            + DatabaseURL 与池参数
    ├── database/database.go        Open / Ping / Close
    └── modules/health/*            /healthy 增加数据库检查
```

下一轮新增（已预留位置，不在本轮）：`internal/database/ent/`（生成代码 + `schema/`）、`migrations/`、`tools.go`、`scripts/db-generate.sh`、`scripts/db-migrate-diff.sh`、`scripts/db-migrate-apply.sh`。

`docker-compose.yml`：服务名 `db`、容器名 `neoblog-postgres`、`postgres:17-alpine`、端口绑定 `127.0.0.1:5434:5432`（只对本机开放）、`POSTGRES_USER=neoblog`、`POSTGRES_PASSWORD=${POSTGRES_PASSWORD:-neoblog}`、`POSTGRES_DB=neoblog`、卷 `neoblog-pgdata`、`restart: unless-stopped`、healthcheck 用 `pg_isready`。初始化脚本 `deploy/postgres/init.sql` 额外建 `neoblog_test`（集成测试用）与 `neoblog_atlas_dev`（下一轮 Atlas 生成迁移用的空回放库；Atlas 要求 dev 库专用且为空）。

本机 Docker 只存在于 WSL（`wsl -d Ubuntu -- docker`），没有 Windows 侧 Docker CLI，且 `wslpath` 不存在。脚本用 `pwd -W` 得到 Windows 路径再换算成 `/mnt/<盘>[/…]` 传给 WSL（已验证）；README 同时给出"在 WSL 终端里手动执行"的等价命令。

## 5. 连接与配置

- `internal/config` 的 `Config` 增加：`DatabaseURL`（`DATABASE_URL`，唯一必需项）；`DB_MAX_OPEN_CONNS`（默认 10）、`DB_MAX_IDLE_CONNS`（默认 5）、`DB_CONN_MAX_LIFETIME`（默认 30m）。非法数值在启动期报错（fail fast）。
- `database.Open(ctx, cfg) (*Database, error)`：`sql.Open("pgx", dsn)` → 设置连接池 → 启动期 `Ping`（超时 5s）。失败返回错误，`main.go` 终止启动；**不做重试**。
- `Database` 本轮形状：私有 `db *sql.DB` + `Ping(ctx) error` + `Close() error`。下一轮新增 `Client *ent.Client` 字段与构造逻辑，`Open` 签名不变（只增字段，不改契约）。
- 本地连接串：`postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog?sslmode=disable`（写进 `.env.example`）；生产由部署注入完整 DSN。
- 启动时自动加载工作目录下的 `.env`（`config.LoadDotEnv`：已存在的环境变量优先，文件缺失不算错误，非法行报错但不算致命）；`scripts/dev.sh` 保留为等价的 shell 方式；生产直接注入环境变量即可。
- `.env` 同时被 `docker-compose.yml` 读取（compose 默认读同目录 `.env`）：改密码要同时改 `POSTGRES_PASSWORD` 与 `DATABASE_URL` 两处，`.env.example` 里写明。

## 6. 迁移工作流（下一轮落地，命令已真机验证）

- 工具：Atlas CLI 官方 Windows 二进制（见 7.3，已安装于 `F:\software\gopath\bin\atlas.exe`）。
- 生成：`atlas migrate diff <名字> --dir "file://migrations" --to "ent://internal/database/ent/schema" --dev-url "$ATLAS_DEV_URL"`（`ATLAS_DEV_URL` 指向空的 `neoblog_atlas_dev`）。
- 应用：`atlas migrate apply --dir "file://migrations" --url "$DATABASE_URL"`；查看：`atlas migrate status ...`。
- 纪律：每次生成后**人工审阅 SQL** 再提交（含 `atlas.sum`）；质量门槛为审阅 + `atlas migrate validate`（lint 需登录 Atlas 账号，本地不可用，见 7.4）。
- schema 无变更时重跑 diff 会提示 "no changes to be made" 且不产生文件（已验证）。
- 应用启动绝不自动迁移；开发期不用 ent 自动建表。测试与迁移的关系（enttest 自建 schema 还是对迁移后的库测试）随第一张表那轮一起定。

## 7. 已验证的工具链事实（2026-10-11 真机跑出，供下一轮直接使用）

1. **ent 代码生成在 Go 1.27 上需要更高版本的 x/tools**：默认会报 `entc/load: package "context" without types`。修法（已验证通过）：go.mod 里 `golang.org/x/tools` 顶到 v0.52.0，并加一个 `tools.go`（`//go:build tools` + `import _ "entgo.io/ent/cmd/ent"`）让 `go mod tidy` 不清理生成工具的依赖（cobra 等）。
2. **ent 必须有至少一张表**：空 schema 目录报 `no Go files in ...`；只有 package 声明报 `no schema found in: ./schema`。故本轮无法生成 ent client，与本轮范围的收窄互相印证。
3. **Atlas CLI 的 `go install` 路径不可用**：v1.3.x 的 Go 模块不含 CLI 命令，`go install` 会回退到 2023 年的旧代码，其依赖的 `x/tools v0.8.1` 在 Go 1.27 下编译失败。官方 Windows 装法是下载预编译二进制（`https://release.ariga.io/atlas/atlas-windows-amd64-latest.exe`，约 138MB），已验证 v1.3.4 可用，且内置 `ent://` 提供者。
4. **`atlas migrate lint` 自 v0.38 起需要登录 Atlas 账号**（Cloud 功能），本地不可用；用 `atlas migrate validate` + 人工审阅替代。
5. **Atlas 的 dev 库必须专用且为空**（官方要求，Atlas 会自行清理且要求独占）→ `neoblog_atlas_dev` 专库专用。
6. **端到端已验证**（临时 Postgres 容器 + 探针表，验证后已清理）：`atlas migrate diff` 从 ent schema 生成标准 Postgres DDL 的迁移文件 + `atlas.sum`；`atlas migrate apply` 成功（1 migration / 1 statement / ok）；`atlas migrate status` 显示已到最新。
7. **本机 Docker 只在 WSL**（`wsl -d Ubuntu -- docker`，compose v5.3.1 可用）；`wslpath` 不存在，路径换算用 `pwd -W` + sed（已验证输出 `/mnt/f/...`）。
8. 端口占用：5432（D 盘项目）、5433（neo-postgres）、**5434 空闲**（本轮采用）。

## 8. 装配与健康检查

- `main.go`：`config.Load` → `database.Open`（失败 `log.Fatal`）→ `app.New(cfg, db)` → `Run`；`defer db.Close()`。
- `app.go`：把 db 注入需要的模块；health 模块先做示范（`health.New(cfg, db)`）。
- health 模块：只依赖本模块声明的消费方接口（`Ping(ctx) error`），不认识具体数据库实现；`/healthy` 出参增加 `database` 字段（`ok` / `error`），Ping 超时 2s；失败时返回 503、`status` 为 `degraded`（现状：无人消费该接口，前端、脚本、CI 均未调用，故影响面为零，只需同步 README 示例）。
- README 硬性规则新增两条：① 模块禁止自建数据库连接（禁止 `sql.Open` 等），只能用注入的；② `internal/database` 不得 import 任何业务模块。跨域访问数据仍只能走对方 service，与既有规则一致。

## 9. 测试

- 单测：`config`（`DATABASE_URL` 与池参数默认值、非法值报错）；`health`（假 pinger 覆盖 ok / error 两个分支与状态码），不连真库。
- 集成测试：读 `DATABASE_URL_TEST`，设置了就真连（`Open` / `Ping` / `Close`），没设置 `t.Skip`；保证 `go test ./...` 在无库环境下全绿。
- 本轮不做迁移相关自动化测试（无表可迁移，随第一张表那轮定）。

## 10. 验收标准

1. `bash scripts/db-up.sh` 后容器 healthy，`127.0.0.1:5434` 可连。
2. `go build ./...` 与 `go test ./...` 全绿（无库时集成测试自动 skip）。
3. `bash scripts/dev.sh` 启动后 `GET /api/v1/healthy` 返回 200 且 `database` 为 `ok`。
4. 停掉容器后 health 返回 503 且 `database` 为 `error`；恢复容器后回到 200。
5. 未设置或设错 `DATABASE_URL` 时启动即失败，报错信息指明原因。
6. README 更新完成：环境变量表、目录对照表、数据库章节（含下一轮迁移工作流的预告）、两条新硬性规则。
7. `.env` / `.env.example` / `.gitignore` 就位，`.env` 未进 Git。

## 11. 风险与注意

- 容器卷在 WSL 的 vhdx 内（C 盘）。博客数据量小，接受；与 D 盘项目同款特性。
- Docker 只在 WSL 内，脚本与 README 都要覆盖"WSL 终端手动执行"的等价命令。
- 下一轮 ent 生成代码带注释，属工具链产物（与 `go.mod` 的 `// indirect` 同理豁免零注释规则）；手写文件保持零注释，生成用脚本而非 `//go:generate` 指令注释。
- 本轮完成后按 README 的版本规则进位（中等功能，minor）。

## 12. 插槽（本轮不定，及定它的时点）

| 插槽 | 什么时候必须定 | 影响面 |
| --- | --- | --- |
| 第一张表（候选：users，字段从 Neo 的 prisma.schema 平移） | 下一轮开头 | ent schema、迁移 SQL |
| ID 方案（Neo 用 cuid 字符串，ent 侧对齐还是自增） | 同上 | 生成代码形状、DTO |
| 时间戳列与表名 / 列名风格 | 同上 | 迁移 SQL |
| 测试与迁移的关系（enttest 自建 schema vs 对迁移后的库测试） | 同上 | 集成测试写法 |
| 事务的使用点与形状 | 第一个跨表原子写的 service | service 内部 |
| 数据表 ownership 划分 | 每个域模块设计时 | 模块边界 |

## 13. 下一步

本文档经用户审阅后，进 writing-plans 出实施计划（范围＝第 3 节的"本轮做"，验收＝第 10 节）。
