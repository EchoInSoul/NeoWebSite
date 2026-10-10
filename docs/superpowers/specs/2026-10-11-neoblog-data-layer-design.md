# NeoBlog 数据层设计（ent + Atlas 版本化迁移）

日期：2026-10-11
范围：`backend/`（Go + Gin）的数据访问基础设施
状态：设计已与用户逐项确认（库选型 ent、独立容器 5434、Atlas 版本化迁移），待用户审阅本文档后进实施计划

## 1. 意图与背景

NeoBlog 后端（`backend/`，Go 1.27 + Gin，模块化组织对齐 NestJS）目前只有 health 模块，零数据库代码。本轮把数据层立起来：一个**连接注入点 + 迁移入口 + 代码生成管线**，让后续业务模块（auth、posts…）有统一、可迁移的数据访问底座。

对齐来源（不重新发明）：

- `F:\protect\Neo`（NestJS + Prisma 的平行项目）已定：Postgres + 版本化迁移进 Git；数据层定位为"数据连接的注入点与迁移入口"，**不隔仓储接口层**，业务边界靠出参 DTO 守。本设计沿用同一立场。
- 用户在选型对话中明确要 Prisma 那种"schema → 生成类型安全 client → 迁移"的体验。Go 侧没有 Prisma（其官方实验 Go client 已于 2025-07-28 归档、明确不再支持 Prisma v7+），最接近的是 **ent**。
- 数据库定为 PostgreSQL 17（与 neo-postgres 一致）。

## 2. 已定决策

| 项 | 决定 | 理由 |
| --- | --- | --- |
| 数据访问库 | `entgo.io/ent` | Go 里最接近 Prisma 的"schema → 生成 client"体验；关系、预加载、事务、迁移集成齐全 |
| 驱动 | `jackc/pgx/v5` 的 stdlib | 活跃维护；`lib/pq` 处于维护模式 |
| 迁移 | Atlas CLI + 版本化 SQL 进 Git | 对齐 Neo 的 `prisma/migrations` 习惯；应用启动不自动迁移 |
| 数据库 | PostgreSQL 17（`postgres:17-alpine`） | 与 Neo 一致 |
| 本地实例 | 独立容器 `neoblog-postgres`，端口 **5434** | 与 5432（D 盘项目）、5433（neo-postgres）互不干扰 |
| 数据层形态 | 连接注入点 + 迁移入口，**不做仓储接口层** | 对齐 Neo 已定的数据边界立场 |
| 启动行为 | 连不上库 fail fast | 数据是必需品，静默降级更危险 |
| 开发期建表 | 不用 ent 自动建表；唯一例外是测试里的 enttest | 避免与迁移文件双轨漂移 |

## 3. 范围

**做**：连接与配置（含连接池）、ent 代码生成管线、Atlas 版本化迁移管线、与 app / health 的装配、脚本与 README 更新、单元与集成测试骨架。

**不做**：任何业务表（第一张表随对应模块的设计单独出）、auth 实现、前端、部署形态、CI。

## 4. 目录与职责

```
backend/
├── docker-compose.yml              只含 db 服务（本地开发）
├── deploy/postgres/init.sql        首次初始化：建 neoblog_test 与 neoblog_atlas_dev
├── atlas.hcl                       Atlas 配置：ent schema → migrations
├── migrations/                     版本化 SQL + atlas.sum，进 Git
├── .env / .env.example             DATABASE_URL 等（.env 不进 Git）
├── .gitignore                      新增：.env
├── scripts/
│   ├── dev.sh                      读取 .env 后 go run .（Go 不自动读 .env）
│   ├── db-up.sh                    起本地 db 容器
│   ├── db-generate.sh              ent 生成代码
│   ├── db-migrate-diff.sh          从 ent schema 生成迁移 SQL
│   └── db-migrate-apply.sh         应用迁移
└── internal/
    ├── database/database.go        Open / Ping / Close，对外暴露 *ent.Client
    └── database/ent/               生成代码 + schema/（第一张表随业务模块来）
```

对照：`internal/database` 承担 Neo 里 `core/data` 的角色（连接的注入点与迁移入口），放在 `internal/` 下与 `modules`、将来的 `common` 平级。

`docker-compose.yml` 内容：服务名 `db`、容器名 `neoblog-postgres`、`postgres:17-alpine`、`5434:5432`、`POSTGRES_USER=neoblog`、`POSTGRES_PASSWORD=${POSTGRES_PASSWORD:-neoblog}`、`POSTGRES_DB=neoblog`、卷 `neoblog-pgdata`、`restart: unless-stopped`、healthcheck 用 `pg_isready`。容器初始化时挂载 `deploy/postgres/init.sql`，额外建 `neoblog_test`（集成测试用）与 `neoblog_atlas_dev`（Atlas 生成迁移时的空回放库）。

本机 Docker 只存在于 WSL（`wsl -d Ubuntu -- docker`），没有 Windows 侧 Docker CLI。`db-up.sh` 等脚本通过 `wsl -d Ubuntu --` 调用，路径用 `wslpath` 转换；README 同时写明"在 WSL 终端里手动执行"的等价命令。

## 5. 连接与配置

- `internal/config` 的 `Config` 增加 `DatabaseURL`（`DATABASE_URL`，唯一必需项）；可选连接池参数 `DB_MAX_OPEN_CONNS`（默认 10）、`DB_MAX_IDLE_CONNS`（默认 5）、`DB_CONN_MAX_LIFETIME`（默认 30m）。
- `database.Open(ctx, cfg) (*Database, error)` 的流程：`sql.Open("pgx", dsn)` → 设置连接池 → 用 `dialect/sql.OpenDB(Postgres, db)` 包成 ent driver → `ent.NewClient` → 启动期 `Ping`（超时 5s）。失败返回错误，由 `main.go` 终止启动；**不做重试**。
- `Database` 结构：`Client *ent.Client` 对外暴露，`db *sql.DB` 私有；方法 `Ping(ctx) error`、`Close() error`。
- 本地连接串：`postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog?sslmode=disable`（写进 `.env.example`）；生产由部署注入完整 DSN，`sslmode` 按环境。
- 本地开发读 `.env` 的途径只有 `scripts/dev.sh`（`set -a` 加载后 `go run .`）。生产不读 `.env`，环境变量直接注入。

## 6. 迁移工作流

- `atlas.hcl` 定义环境：`src = "ent://internal/database/ent/schema"`、`dev = neoblog_atlas_dev 连接串`、`migration.dir = file://migrations`、`url = $DATABASE_URL`。HCL 的确切写法（`getenv` 等）在实施计划第一步验证。
- 日常流程：改 ent schema → `scripts/db-migrate-diff.sh <名字>` 生成 SQL → **人工审阅** → 提交（含 `atlas.sum`）→ `scripts/db-migrate-apply.sh` 应用到本地。
- 质量门槛：`atlas migrate lint`；目录完整性 `atlas migrate validate`。
- 应用启动**绝不**自动迁移（不做 Prisma 的 `migrate deploy on boot`）。
- 测试与迁移解耦：模块测试用 `enttest` 自建 schema；迁移自身的正确性由 lint + 对 scratch 库的回放保证。
- 首次真实迁移随第一张业务表产生；在此之前，工具链的验证是 generate 可运行、validate 通过、apply 对空目录成功。

## 7. 代码生成

- `scripts/db-generate.sh`：在 `internal/database/ent` 下执行 `go run entgo.io/ent/cmd/ent generate --feature sql/upsert,sql/execquery ./schema`；其他 feature 到需要时再加。
- 生成代码**进 Git**（与 Neo 的 `src/generated` 同策略）：`go build` 不依赖先生成，diff 可审计。生成代码**禁止手工修改**，改 schema 后重生成。
- 不使用 `//go:generate` 指令注释（保持手写 Go 文件零注释），跑脚本替代。
- 幂等验收：连续两次生成后 `git diff` 为空。

## 8. 装配与健康检查

- `main.go`：`config.Load` → `database.Open`（失败 `log.Fatal`）→ `app.New(cfg, db)` → `Run`；`defer db.Close()`。
- `app.go`：把 db 注入需要的模块；health 模块先做示范（`health.New(cfg, db)`）。
- health 模块：只依赖本模块声明的消费方接口（`Ping(ctx) error`），不认识 ent；`/healthy` 出参增加 `database` 字段（`ok` / `error`），Ping 超时 2s；失败时返回 503、`status` 为 `degraded`。
- README 硬性规则新增两条：① 模块禁止自建数据库连接（禁止 `sql.Open` / `ent.Open`），只能用注入的；② `internal/database` 不得 import 任何业务模块。跨域访问数据仍只能走对方 service，与既有规则一致。

## 9. 测试

- 单测：`config`（`DATABASE_URL` 与连接池参数默认值）；`health`（假 pinger 覆盖 ok / error 两个分支与状态码），不连真库。
- 集成测试：读 `DATABASE_URL_TEST`，设置了就真连（`Open` / `Ping` / `Close`），没设置 `t.Skip`；保证 `go test ./...` 在无库环境下全绿。
- 迁移：脚本里跑 `atlas migrate lint` 与 `validate`，本轮不做自动回放测试。

## 10. 验收标准

1. `bash scripts/db-up.sh` 后容器 healthy，`127.0.0.1:5434` 可连（端口已确认空闲）。
2. `bash scripts/db-generate.sh` 成功且幂等（重跑 `git diff` 为空）。
3. `atlas migrate validate` 通过；`atlas migrate status` 能连上库。
4. `go build ./...` 与 `go test ./...` 全绿（无库时集成测试自动 skip）。
5. `bash scripts/dev.sh` 启动后 `GET /api/v1/healthy` 返回 200 且 `database` 为 `ok`。
6. 停掉容器后 health 返回 503 且 `database` 为 `error`；恢复容器后回到 200。
7. README 更新完成：环境变量表、目录对照表、数据库章节（工作流与命令）、两条新硬性规则。

## 11. 风险与注意

- ent 生成代码带注释，属工具链产物，与 `go.mod` 的 `// indirect` 同理豁免零注释规则；手写文件仍零注释。
- Atlas 的 `--dev-url` 在 Windows 侧不能用 `docker://` 形式（本机无 Docker CLI），必须用真库（`neoblog_atlas_dev`）。
- 容器卷在 WSL 的 vhdx 内（C 盘）。博客数据量小，接受；与 D 盘项目同款特性。
- Docker 只在 WSL 内，脚本与 README 都要覆盖"WSL 终端手动执行"的等价命令，避免脚本在无 WSL 环境下失效时无路可走。
- 完成后按 README 的版本规则进位（本项属中等功能，minor）。

## 12. 插槽（本轮不定，及定它的时点）

| 插槽 | 什么时候必须定 | 影响面 |
| --- | --- | --- |
| ID 方案（Neo 用 cuid 字符串，ent 侧对齐还是自增） | 第一张业务表设计时 | ent 生成代码形状、DTO |
| 时间戳列约定（createdAt / updatedAt） | 同上 | schema、迁移 SQL |
| 表名 / 列名风格（snake_case 等） | 同上 | 迁移 SQL、手写 SQL |
| 事务的使用点与形状 | 第一个跨表原子写的 service | service 内部 |
| 数据表 ownership 划分（哪个域拥有哪张表） | 每个域模块设计时 | 模块边界 |

## 13. 下一步

本文档经用户审阅后，进 writing-plans 出实施计划。实施计划的第一步是验证 Atlas HCL 与 ent 生成命令的确切形状，再铺目录、写代码、按第 10 节验收。
