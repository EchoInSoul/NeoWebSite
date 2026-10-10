# NeoBlog 数据层（层 0）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给 Go 后端接上 Postgres 连接层与健康检查：配置、`internal/database`、`/healthy` 纳入数据库状态、本地容器与脚本。

**Architecture:** 沿用 NeoBlog 既有模块结构（controller → service → repository，app.go 装配、模块 `New(cfg, …)` 注入）。本轮不引入 ent、不建任何业务表：`database.Open` 返回持有 `*sql.DB` 的 `Database`，下一轮加 `Client *ent.Client` 字段即可，签名不变。health 模块通过自己声明的消费方接口 `Ping(ctx) error` 检查数据库，不认识具体实现。

**Tech Stack:** Go 1.27.2、Gin、`github.com/jackc/pgx/v5`（stdlib 驱动）、PostgreSQL 17（WSL Docker 容器）、Bash 脚本。

**Spec:** `docs/superpowers/specs/2026-10-11-neoblog-data-layer-design.md`（v2；第 6、7 节是下一轮 ent 与迁移的施工依据，本轮不执行）

## Global Constraints

- 端口 `5434`；容器名 `neoblog-postgres`；镜像 `postgres:17-alpine`；库/用户 `neoblog`；密码 `${POSTGRES_PASSWORD:-neoblog}`。端口绑定 `127.0.0.1:5434:5432`。
- `DATABASE_URL` 是唯一必需环境变量；池参数默认 `DB_MAX_OPEN_CONNS=10`、`DB_MAX_IDLE_CONNS=5`、`DB_CONN_MAX_LIFETIME=30m`。
- 启动期 Ping 超时 5 秒、不重试、失败即进程退出；`/healthy` 的数据库 Ping 超时 2 秒。
- `/healthy` 语义：一切正常 → HTTP 200、`status:"ok"`、`database:"ok"`；数据库 Ping 失败 → HTTP 503、`status:"degraded"`、`database:"error"`。
- **Go 代码零注释**（工具链生成的注释除外）；缩进走 gofmt（`gofmt -l` 必须为空）。
- 错误与日志信息用英文（与现有 `log.Printf("listening on %s", ...)` 一致）；错误信息**不得包含完整 DSN**（避免日志泄露密码）。
- 模块禁止自建数据库连接（禁止 `sql.Open` 等），只能用注入的；`internal/database` 不得 import 任何业务模块；service 一律不 import gin（既有规则）。
- 本轮不做：ent 代码生成、`migrations/`、Atlas 脚本、任何业务表、auth、前端、部署形态、CI。
- 提交信息沿用现有风格：`feat(backend): 中文描述` / `chore(backend): …`。
- 验证命令注意：本机 `grep` 实为 ugrep、管道会吞退出码——跑验证命令时加 `set -o pipefail`，或直接看输出判断。

## Review Focus

1. 启动时数据库不可达或 DSN 写错：必须快速失败（秒级退出、非零码）且报错能定位原因，不允许"进程起来了但每次请求都 500"。→ Task 2（`TestOpenRejectsUnreachableDatabase`）、Task 4 验证步骤。
2. 池参数写成非法值（`0`、负数、非数字、idle > open）：启动即失败，报错必须点名是哪个环境变量。→ Task 1（`TestLoadRejectsInvalidPoolValues`）。
3. 运行中数据库挂掉：`/healthy` 必须在约 2 秒内返回 503 + `degraded`，不能挂住。→ Task 3（`TestCheckTimesOutSlowDatabase`、`TestControllerMapsDegradedTo503`）。
4. 错误信息泄露凭据：任何错误路径都不得把含密码的完整 DSN 打进日志。→ Task 2 实现约束（错误包装只带阶段名）。
5. 集成测试静默假绿：无库时必须以显式 `SKIP`（带原因）跳过；设置了 `DATABASE_URL_TEST` 但连不上时必须 FAIL 而不是 skip。→ Task 2（测试写法）、Task 5 第 6 步（真库跑通）。

---

### Task 1: 配置项（DATABASE_URL 与连接池参数）

**Files:**
- Modify: `backend/internal/config/config.go`
- Modify: `backend/main.go`（仅适配 `Load` 新签名，保证编译）
- Test: `backend/internal/config/config_test.go`

**Interfaces:**
- Consumes: 既有 `getEnv(key, fallback string) string`。
- Produces: `config.Load() (Config, error)`；`Config.DatabaseURL string`、`Config.DBMaxOpenConns int`、`Config.DBMaxIdleConns int`、`Config.DBConnMaxLifetime time.Duration`。

- [ ] **Step 1: Write failing tests**（`backend/internal/config/config_test.go`）

```go
package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("expected error naming DATABASE_URL, got %v", err)
	}
}

func TestLoadDatabaseDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog?sslmode=disable")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL == "" || cfg.DBMaxOpenConns != 10 || cfg.DBMaxIdleConns != 5 || cfg.DBConnMaxLifetime != 30*time.Minute {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadReadsPoolOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	t.Setenv("DB_MAX_OPEN_CONNS", "20")
	t.Setenv("DB_MAX_IDLE_CONNS", "8")
	t.Setenv("DB_CONN_MAX_LIFETIME", "1h")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBMaxOpenConns != 20 || cfg.DBMaxIdleConns != 8 || cfg.DBConnMaxLifetime != time.Hour {
		t.Fatalf("unexpected overrides: %+v", cfg)
	}
}

func TestLoadRejectsInvalidPoolValues(t *testing.T) {
	cases := []struct {
		key, value, wantMention string
	}{
		{"DB_MAX_OPEN_CONNS", "0", "DB_MAX_OPEN_CONNS"},
		{"DB_MAX_OPEN_CONNS", "abc", "DB_MAX_OPEN_CONNS"},
		{"DB_MAX_IDLE_CONNS", "-1", "DB_MAX_IDLE_CONNS"},
		{"DB_CONN_MAX_LIFETIME", "soon", "DB_CONN_MAX_LIFETIME"},
	}
	for _, tc := range cases {
		t.Run(tc.key+"/"+tc.value, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), tc.wantMention) {
				t.Fatalf("expected error mentioning %s, got %v", tc.wantMention, err)
			}
		})
	}
}

func TestLoadRejectsIdleGreaterThanOpen(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	t.Setenv("DB_MAX_OPEN_CONNS", "5")
	t.Setenv("DB_MAX_IDLE_CONNS", "9")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when DB_MAX_IDLE_CONNS > DB_MAX_OPEN_CONNS")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd backend && go test ./internal/config/ -v`
Expected: FAIL —— `Load()` 只有一个返回值、`Config` 没有新字段（编译错误即算达成）。

- [ ] **Step 3: Implement**

`config.go` 增加字段与解析：

```go
type Config struct {
	Addr              string
	Environment       string
	Version           string
	ServiceName       string
	DatabaseURL       string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration
}

func Load() (Config, error)
func getEnvInt(key string, fallback int) (int, error)
func getEnvDuration(key string, fallback time.Duration) (time.Duration, error)
```

规则：`DatabaseURL` 为空 → `errors.New("DATABASE_URL is required")`；整数/时长解析失败或 ≤ 0 → `fmt.Errorf("%s must be a positive %s", key, kind)`；`DBMaxIdleConns > DBMaxOpenConns` → 点名两个变量的错误。`main.go` 适配为 `cfg, err := config.Load(); if err != nil { log.Fatal(err) }`。

- [ ] **Step 4: Run to verify pass**

Run: `cd backend && go test ./internal/config/ -v && go build ./...`
Expected: 全部 PASS，build 无输出。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/config backend/main.go
git commit -m "feat(backend): 配置项支持数据库连接串与连接池参数"
```

---

### Task 2: `internal/database` 连接层

**Files:**
- Create: `backend/internal/database/database.go`
- Create: `backend/internal/database/database_test.go`
- Modify: `backend/go.mod`（`go get github.com/jackc/pgx/v5`）

**Interfaces:**
- Consumes: `config.Config` 的 `DatabaseURL / DBMaxOpenConns / DBMaxIdleConns / DBConnMaxLifetime`。
- Produces: `database.Open(ctx context.Context, cfg config.Config) (*Database, error)`；`(*Database).Ping(ctx context.Context) error`；`(*Database).Close() error`。（下一轮：`Database` 增 `Client *ent.Client` 字段。）

- [ ] **Step 1: Add dependency**

Run: `cd backend && go get github.com/jackc/pgx/v5@latest`
（GOPROXY 已配 goproxy.cn；若新 shell 报找不到 go，先 `export PATH="/f/software/go/bin:$PATH"`。）

- [ ] **Step 2: Write failing tests**（`backend/internal/database/database_test.go`）

```go
package database

import (
	"context"
	"os"
	"testing"
	"time"

	"website/backend/internal/config"
)

func testConfig(url string) config.Config {
	return config.Config{
		DatabaseURL:       url,
		DBMaxOpenConns:    10,
		DBMaxIdleConns:    5,
		DBConnMaxLifetime: 30 * time.Minute,
	}
}

func TestOpenRejectsUnreachableDatabase(t *testing.T) {
	_, err := Open(context.Background(), testConfig("postgresql://nobody:nobody@127.0.0.1:1/nowhere?sslmode=disable"))
	if err == nil {
		t.Fatal("expected error for unreachable database")
	}
}

func TestOpenPingClose(t *testing.T) {
	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("SKIP: DATABASE_URL_TEST is not set")
	}
	db, err := Open(context.Background(), testConfig(url))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := db.Ping(context.Background()); err == nil {
		t.Fatal("expected ping to fail after close")
	}
}
```

- [ ] **Step 3: Run to verify failure**

Run: `cd backend && go test ./internal/database/ -v`
Expected: FAIL —— `Open` 未定义（编译错误即算达成）。

- [ ] **Step 4: Implement `database.go`**

```go
package database

const pingTimeout = 5 * time.Second

type Database struct {
	db *sql.DB
}

func Open(ctx context.Context, cfg config.Config) (*Database, error)
func (d *Database) Ping(ctx context.Context) error
func (d *Database) Close() error
```

要点：`sql.Open("pgx", cfg.DatabaseURL)`（副作用导入 `_ "github.com/jackc/pgx/v5/stdlib"`）；随后 `SetMaxOpenConns / SetMaxIdleConns / SetConnMaxLifetime`；用 `context.WithTimeout(ctx, pingTimeout)` 做 `PingContext`；失败时先 `db.Close()` 再返回 `fmt.Errorf("database: connect: %w", err)`（**错误里不带 DSN**）。

- [ ] **Step 5: Run to verify pass (unit) and skip (integration)**

Run: `cd backend && go test ./internal/database/ -v && go test ./...`
Expected: `TestOpenRejectsUnreachableDatabase` PASS；`TestOpenPingClose` 显示 `SKIP: DATABASE_URL_TEST is not set`；`go test ./...` 全绿。

- [ ] **Step 6: Commit**

```bash
git add backend/internal/database backend/go.mod backend/go.sum
git commit -m "feat(backend): 数据库连接层（pgx + 连接池 + 启动期探活）"
```

---

### Task 3: `/healthy` 纳入数据库状态

**Files:**
- Modify: `backend/internal/modules/health/dto.go`、`service.go`、`controller.go`、`module.go`
- Modify: `backend/README.md`（健康检查示例 + 两条硬性规则）
- Test: `backend/internal/modules/health/service_test.go`、`backend/internal/modules/health/controller_test.go`

**Interfaces:**
- Consumes: 任何实现 `Ping(ctx context.Context) error` 的值（Task 2 的 `*database.Database` 满足）。
- Produces: `health.New(cfg config.Config, db pinger) *Module`；`(*Service).Check(ctx context.Context) Status`；`Status.Database string`（`ok` / `error`）。

- [ ] **Step 1: Write failing tests**

`service_test.go`：

```go
package health

import (
	"context"
	"errors"
	"testing"
	"time"

	"website/backend/internal/config"
)

type fakePinger struct {
	err   error
	delay time.Duration
}

func (f fakePinger) Ping(ctx context.Context) error {
	if f.delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(f.delay):
		}
	}
	return f.err
}

func TestCheckReportsDatabaseOK(t *testing.T) {
	st := newService(config.Config{ServiceName: "s"}, fakePinger{}).Check(context.Background())
	if st.Status != "ok" || st.Database != "ok" {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestCheckReportsDatabaseErrorAndDegrades(t *testing.T) {
	st := newService(config.Config{ServiceName: "s"}, fakePinger{err: errors.New("boom")}).Check(context.Background())
	if st.Status != "degraded" || st.Database != "error" {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestCheckTimesOutSlowDatabase(t *testing.T) {
	start := time.Now()
	st := newService(config.Config{ServiceName: "s"}, fakePinger{delay: 10 * time.Second}).Check(context.Background())
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("check took %s, expected ~2s timeout", elapsed)
	}
	if st.Status != "degraded" || st.Database != "error" {
		t.Fatalf("unexpected status: %+v", st)
	}
}
```

`controller_test.go`：

```go
package health

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"website/backend/internal/config"
)

func newTestRouter(t *testing.T, p pinger) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	New(config.Config{ServiceName: "s"}, p).Register(engine.Group("/api/v1"))
	return engine
}

func TestHealthyReturns200WhenDatabaseOK(t *testing.T) {
	w := httptest.NewRecorder()
	newTestRouter(t, fakePinger{}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/healthy", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"database":"ok"`) {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

func TestHealthyReturns503WhenDatabaseDown(t *testing.T) {
	w := httptest.NewRecorder()
	newTestRouter(t, fakePinger{err: errors.New("boom")}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/healthy", nil))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), `"database":"error"`) {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"status":"degraded"`) {
		t.Fatalf("expected degraded status, got %s", w.Body.String())
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd backend && go test ./internal/modules/health/ -v`
Expected: FAIL —— `newService` 参数不匹配 / `Status.Database` 不存在（编译错误即算达成）。

- [ ] **Step 3: Implement**

- `dto.go`：`Status` 增加 `Database string \`json:"database"\``。
- `service.go`：定义 `type pinger interface { Ping(ctx context.Context) error }`；`Service` 增 `db pinger`；`newService(cfg, db pinger)`；`Check(ctx context.Context) Status` 里用 `context.WithTimeout(ctx, 2*time.Second)` 调 `s.db.Ping`，失败则 `Status="degraded"`、`Database="error"`，成功两者均为 `"ok"`。
- `controller.go`：`c.service.Check(ctx.Request.Context())`；`code := http.StatusOK; if status.Status != "ok" { code = http.StatusServiceUnavailable }`；`ctx.JSON(code, status)`。
- `module.go`：`func New(cfg config.Config, db pinger) *Module`，透传给 `newService`。
- `README.md`：更新健康检查示例（加入 `database` 字段并说明 503 语义）；「硬性规则」追加两条：① 模块禁止自建数据库连接（禁止 `sql.Open` 等），只能用注入的；② `internal/database` 不得 import 任何业务模块。

- [ ] **Step 4: Run to verify pass**

Run: `cd backend && go test ./internal/modules/health/ -v && go test ./...`
Expected: 全部 PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/health backend/README.md
git commit -m "feat(backend): 健康检查纳入数据库状态（库挂返回 503 degraded）"
```

---

### Task 4: 启动装配（fail fast）

**Files:**
- Modify: `backend/main.go`
- Modify: `backend/internal/app/app.go`

**Interfaces:**
- Consumes: `config.Load() (Config, error)`、`database.Open(ctx, cfg) (*Database, error)`、`health.New(cfg, db)`。
- Produces: `app.New(cfg config.Config, db *database.Database) *App`。

- [ ] **Step 1: Implement wiring**

`main.go`：`cfg, err := config.Load()` → `log.Fatal(err)`；`db, err := database.Open(context.Background(), cfg)` → `log.Fatal(err)`；`defer db.Close()`；`app.New(cfg, db).Run(cfg.Addr)`。`app.go`：`New(cfg config.Config, db *database.Database) *App`，内部 `health.New(cfg, db)`。

- [ ] **Step 2: Verify build and fail-fast**

```bash
cd backend
set -o pipefail
go build ./...
env -u DATABASE_URL go run . 2>&1 | tail -2
env -u DATABASE_URL go run . >/dev/null 2>&1; echo "rc=$?"
DATABASE_URL="postgresql://nobody:nobody@127.0.0.1:1/x?sslmode=disable" go run . 2>&1 | tail -2
```

Expected: build 无输出；两次 `go run .` 都**秒级退出**，错误信息分别点名 `DATABASE_URL` 与连接失败，`rc` 非 0（`log.Fatal` 退出码 1）；不出现监听端口日志。（`.env` 即使存在也不影响——`go run .` 不读它。）

- [ ] **Step 3: Commit**

```bash
git add backend/main.go backend/internal/app/app.go
git commit -m "feat(backend): 启动时装配数据库连接，连不上即失败"
```

---

### Task 5: 本地容器、脚本与端到端验证

**Files:**
- Create: `backend/docker-compose.yml`、`backend/deploy/postgres/init.sql`、`backend/scripts/db-up.sh`、`backend/scripts/dev.sh`、`backend/.env.example`、`backend/.env`、`backend/.gitignore`
- Modify: `backend/README.md`（目录对照表、数据库章节、运行章节）

**Interfaces:**
- Consumes: Task 4 的启动路径；`DATABASE_URL` / `DATABASE_URL_TEST` 命名。
- Produces: `scripts/db-up.sh`（起库并等到 healthy）、`scripts/dev.sh`（載入 `.env` 后 `go run .`）。

- [ ] **Step 1: Create files**

`docker-compose.yml`：

```yaml
services:
  db:
    image: postgres:17-alpine
    container_name: neoblog-postgres
    restart: unless-stopped
    environment:
      POSTGRES_USER: neoblog
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-neoblog}
      POSTGRES_DB: neoblog
    ports:
      - '127.0.0.1:5434:5432'
    volumes:
      - neoblog-pgdata:/var/lib/postgresql/data
      - ./deploy/postgres/init.sql:/docker-entrypoint-initdb.d/01-databases.sql:ro
    healthcheck:
      test: ['CMD-SHELL', 'pg_isready -U neoblog -d neoblog']
      interval: 3s
      timeout: 3s
      retries: 20

volumes:
  neoblog-pgdata:
```

`deploy/postgres/init.sql`：

```sql
CREATE DATABASE neoblog_test;
CREATE DATABASE neoblog_atlas_dev;
```

`scripts/db-up.sh`（本机 Docker 只在 WSL；`wslpath` 不存在，用 `pwd -W` + sed 换算）：

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
win_path="$(pwd -W)"
wsl_path="/mnt/$(printf '%s' "$win_path" | sed -E 's#^([A-Za-z]):#\L\1#')"
wsl -d Ubuntu -- bash -lc "cd '$wsl_path' && docker compose up -d"
for _ in $(seq 1 30); do
  status="$(wsl -d Ubuntu -- docker inspect -f '{{.State.Health.Status}}' neoblog-postgres 2>/dev/null || true)"
  [ "$status" = "healthy" ] && { echo "neoblog-postgres is healthy"; exit 0; }
  sleep 2
done
echo "neoblog-postgres did not become healthy in time" >&2
exit 1
```

`scripts/dev.sh`：

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [ -f .env ]; then
  set -a
  . ./.env
  set +a
fi
exec go run .
```

`.env.example`（`.env` 内容相同；`.env` 不进 Git）：

```
DATABASE_URL=postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog?sslmode=disable
DATABASE_URL_TEST=postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_test?sslmode=disable
POSTGRES_PASSWORD=neoblog
# 改密码要同时改 POSTGRES_PASSWORD 与两条 DATABASE_URL（compose 与后端读同一个 .env）。
# DB_MAX_OPEN_CONNS=10
# DB_MAX_IDLE_CONNS=5
# DB_CONN_MAX_LIFETIME=30m
```

`.gitignore`：内容 `.env`。两个脚本 `chmod +x`。

- [ ] **Step 2: Start the database**

Run: `cd backend && bash scripts/db-up.sh`
Expected: 输出 `neoblog-postgres is healthy`。
若 Windows 侧连不上 5434（WSL localhost 转发异常），把端口改回 `'5434:5432'`（与既有容器绑定方式一致）后重试。

- [ ] **Step 3: Verify databases exist**

Run: `wsl -d Ubuntu -- docker exec neoblog-postgres psql -U neoblog -d neoblog -c "SELECT datname FROM pg_database ORDER BY datname;"`
Expected: 列表含 `neoblog`、`neoblog_atlas_dev`、`neoblog_test`。

- [ ] **Step 4: Integration test against the real database**

Run: `cd backend && DATABASE_URL_TEST="postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_test?sslmode=disable" go test ./internal/database/ -run TestOpenPingClose -v`
Expected: PASS（不是 SKIP）。

- [ ] **Step 5: End-to-end health check (acceptance)**

```bash
cd backend
set -o pipefail
(ADDR=:5733 bash scripts/dev.sh >/tmp/neoblog-dev.log 2>&1 &)
code=""
for _ in $(seq 1 40); do
  code="$(curl -s -o /tmp/h1.json -m 2 -w '%{http_code}' http://127.0.0.1:5733/api/v1/healthy || true)"
  [ "$code" = "200" ] && break
  sleep 1
done
echo "healthy: $code"; cat /tmp/h1.json
wsl -d Ubuntu -- docker stop neoblog-postgres
sleep 2
code="$(curl -s -o /tmp/h2.json -m 5 -w '%{http_code}' http://127.0.0.1:5733/api/v1/healthy || true)"
echo "down: $code"; cat /tmp/h2.json
wsl -d Ubuntu -- docker start neoblog-postgres
for _ in $(seq 1 30); do
  code="$(curl -s -o /dev/null -m 2 -w '%{http_code}' http://127.0.0.1:5733/api/v1/healthy || true)"
  [ "$code" = "200" ] && break
  sleep 2
done
echo "restored: $code"
pid="$(netstat -ano | rg ':5733\s+.*LISTENING' | head -1 | awk '{print $NF}')"
[ -n "$pid" ] && taskkill //F //PID "$pid"
```

Expected: 40 秒内出现 `healthy: 200` 且 body 含 `"database":"ok"`（首次 `go run` 要现场编译，慢属正常）；停库后 `down: 503` 且 body 含 `"status":"degraded"`、`"database":"error"`（5 秒内返回）；重启库后 `restored: 200`；`taskkill` 后 5733 不再被监听。（用 5733 而非默认 5732，避免与其它调试实例撞端口；按端口找 PID 而不是 `pkill go run`，因为 `go run` 的子进程不会被连带杀死。）

- [ ] **Step 6: Verify `.env` is untracked**

Run: `cd backend && git status --short`
Expected: 不出现 `.env`（出现则修 `.gitignore`）。

- [ ] **Step 7: Update README**

目录对照表加 `internal/database` 与 `docker-compose.yml` / `scripts/` 行；环境变量表加 `DATABASE_URL`、`DATABASE_URL_TEST`、`POSTGRES_PASSWORD`、`DB_MAX_OPEN_CONNS`、`DB_MAX_IDLE_CONNS`、`DB_CONN_MAX_LIFETIME`；新增「数据库」章节：起库（`scripts/db-up.sh` 与 WSL 手动等价命令）、跑后端（`scripts/dev.sh`）、下一轮迁移工作流预告（指向 spec 第 6、7 节）；「运行」章节补 `scripts/dev.sh`。

- [ ] **Step 8: Commit**

```bash
git add backend/docker-compose.yml backend/deploy backend/scripts backend/.env.example backend/.gitignore backend/README.md
git commit -m "feat(backend): 本地 Postgres 容器、初始化脚本与开发脚本"
```

---

### Task 6: 版本进位与全量验收

**Files:**
- Modify: `backend/internal/version/version.go`、`backend/README.md`（经进位脚本）

- [ ] **Step 1: Bump version**

Run: `cd backend && ./scripts/bump-version.sh minor`
Expected: 版本 `0.0.2 → 0.1.0`，README 顶部「当前版本」同步更新。

- [ ] **Step 2: Full acceptance sweep（spec 第 10 节）**

```bash
cd backend
set -o pipefail
gofmt -l .                 # 期望：无输出
rg -n "//" --type go .     # 期望：无匹配（本轮无工具链生成代码）
go vet ./...
go build ./...
go test ./...              # 无 DATABASE_URL_TEST：集成用例 SKIP，其余全绿
DATABASE_URL_TEST="postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_test?sslmode=disable" go test ./...
bash scripts/db-up.sh
env -u DATABASE_URL go run . 2>&1 | tail -1   # 期望：点名 DATABASE_URL 的错误
```

Expected: 全部符合括注；`go test` 两次全绿。

- [ ] **Step 3: Commit**

```bash
git add backend/internal/version/version.go backend/README.md
git commit -m "chore(backend): 版本进位 0.1.0（数据层层 0：连接与健康检查）"
```

---

## 完成标准

spec 第 10 节 7 条逐条成立：容器 healthy 且 5434 可连；`go build ./...`、`go test ./...` 全绿（无库时集成用例显式 SKIP）；`/healthy` 在有库时 200 + `database:"ok"`、无库时 503 + `degraded`；`DATABASE_URL` 缺失/错误时启动即失败且信息可定位；README 更新到位；`.env` 未进 Git。
