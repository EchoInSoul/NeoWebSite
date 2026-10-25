# 自研人机验证（go-captcha 滑块）后端接入 实施计划

> **For agentic workers:** 本机只安装了 `superpowers:brainstorming` / `writing-plans` / `systematic-debugging` / `using-superpowers`，没有 `subagent-driven-development` 与 `executing-plans`。按 **Native** 方式执行：主会话逐任务实施、每个 Task 结束跑该 Task 的验证命令，全部完成后跑全量 `check.sh` + `test.sh`。Steps 用 checkbox（`- [ ]`）跟踪。

**Goal:** 给后端新增 `captcha_provider=builtin`：用开源库 `go-captcha` 生成滑块题、后端判分、发一次性凭证，接进登录/注册现有的 `captcha_token`。

**Architecture:** `captcha` 模块内新增三块互不越界的能力——`generator`（只有 go-captcha 引擎）、`repository`（只有挑战表存储）、`builtin`（自托管验证：出题/判分/凭证）；`service.go` 退成薄分派（读设置、选 turnstile 或 builtin），Turnstile 分支一行不动；HTTP 只在 `controller.go`。

**Tech Stack:** Go 1.27 / gin / ent / atlas / `github.com/wenlng/go-captcha/v2 v2.0.5` / `github.com/wenlng/go-captcha-assets v1.0.7` / PostgreSQL。

**Spec:** `docs/superpowers/specs/2026-10-26-neoblog-builtin-captcha-design.md`

## Global Constraints

- 只改 `backend/` 与 `docs/`；`frontend/` 零改动。
- **模块边界**：`login` / `registration` / `auth` / `settings` / `access` / `session` / `twofactor` 的生产代码零改动；`app.go` 只加装配，`apierror` 只加常量。不新增设置键，不做 provider 白名单，不做管理端 builtin 自检。
- **能力边界**：`generator` 不碰 HTTP / DB / 设置；`repository` 不碰 HTTP / `apierror` / 设置；`builtin` 只依赖 settings 窄接口 + repository + generator + limiter；`service` 只做分派；`controller` 只做 HTTP 编解码。
- Turnstile 判定链（`Service.Verify` 的 `turnstile` 分支）逐行保持现状。
- 无兜底：失败上抛；`provider=builtin` 而 builtin 未装配时返回 `503 service_unavailable`，不放行、不 panic；素材加载失败让 `app.New` 失败。
- 素材只用官方包，不改图、不自制。
- 固定值：题目 TTL `3m`、凭证 TTL `5m`、判分容差 `4px`、出题限流 `60 次/分钟/IP`、凭证一次性且只绑 `scope`（不绑 IP）。
- 协议：`POST /api/v1/captcha/challenges`、`POST /api/v1/captcha/challenges/:id/solutions`；字段 snake_case；错误信封 `{"error":{"type","code","param"}}`。
- 错误码只增不改义：新增 `captcha_disabled`（403）。交卷失败复用 code `captcha_failed`（400、`param: null`，新常量），登录/注册侧沿用既有 `ErrCaptchaFailed`（`param: captcha_token`）。
- 时间字段在 JSON 里是字符串，格式 `time.Now().UTC().Format(time.RFC3339)`。
- 不逐任务提交；最后统一一笔提交，且仅在用户确认后执行。

## Review Focus

1. **并发交卷 / 凭证并发消费**：同一题两次交卷、同一凭证两次调用 `Verify`，只能有一个成功；另一个必须 `400 captcha_failed`。→ Task 3、Task 4 的测试。
2. **过期边界**：题目过期后交卷、凭证过期后 `Verify`，一律 `400 captcha_failed`，不得因清理任务没跑而放行。→ Task 3、Task 4 的测试。
3. **scope 串用与未勾选放行**：`login` 凭证打 `register` 必须失败；入口不在 `captcha_scopes` 时 `Verify` 必须放行（既有语义）。→ Task 4 的测试。
4. **依赖不可用**：设置读不到（数据库不可用）时出题返回 `503`、不放行；`builtin` 未装配时 `Verify` 返回 `503`。→ Task 5、Task 7 的测试。
5. **非法输入**：`id` 非 UUID、`x`/`y` 缺失或为负、`scope` 未知 → `400` 且不改变题目状态（仍可用正确坐标交卷）。→ Task 6 的测试。

## 文件结构（模块边界）

```
internal/modules/captcha/
  module.go         装配：builtin.New、turnstile.NewService、两个 controller
  service.go        分派：读 enabled / provider，选并委托 turnstile 或 builtin
  verifier.go       Verifier 别名（指向 turnstile.Verifier）
  turnstile/
    turnstile.go    Cloudflare siteverify 客户端 + 三态结果；只依赖标准库
    verifier.go     消费方接口 + 不可用实现
    service.go      私钥 / scope / 凭证判定与 VerifyConfig
    controller.go   管理端自检路由（system.captcha.edit）
    dto.go          自检请求 / 响应
    turnstile_test.go
  builtin/
    generator.go    引擎：官方素材加载 + 出题；只依赖 go-captcha 库与标准库
    repository.go   存储：挑战表 ent 读写与原子状态迁移；不 import apierror
    service.go      能力：出题 / 交卷 / 凭证消费 / Policies
    controller.go   两条公开路由（出题 / 交卷）
    dto.go          请求 / 响应形状 + 挑战状态常量
    *_test.go       生成器 / 存储 / 能力 / 路由 / 表结构测试
```

依赖方向：`controller → service → {turnstile, builtin} → {repository, generator}`；`builtin → settings（窄接口）`。两个子包互不 import，父包只依赖子包，没有反向依赖。

---

### Task 1: 挑战表（ent schema + 迁移 + 表测试）

**Files:**
- Create: `backend/internal/database/ent/schema/captcha_challenge.go`
- Generated: `backend/internal/database/ent/captchachallenge/*`（`scripts/db-generate.sh`）
- Create: `backend/migrations/<时间戳>_captcha_challenges.sql`、`backend/migrations/atlas.sum`（atlas diff）
- Test: `backend/internal/modules/captcha/schema_test.go`（同包可复用 `verify_route_test.go` 里的 `testDB` / `uniqueTag`）

**Interfaces:**
- Produces: ent 类型 `*ent.CaptchaChallenge`（字段 `Scope string`、`AnswerX int`、`AnswerY int`、`Status int`、`TokenHash *string`、`CreatedAt time.Time`、`ExpiresAt time.Time`），生成的查询包 `ent/captchachallenge`（`IDEQ` / `StatusEQ` / `TokenHashEQ` / `ScopeEQ` / `ExpiresAtGT` / `ExpiresAtLT`）。

- [ ] **Step 1: 写失败测试** `backend/internal/modules/captcha/schema_test.go`

```go
package captcha

import (
	"context"
	"testing"
	"time"

	"website/backend/internal/database/ent"
)

func TestCaptchaChallengeSchema(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	created, err := db.Client.CaptchaChallenge.Create().
		SetScope("login").
		SetAnswerX(12).
		SetAnswerY(34).
		SetExpiresAt(time.Now().Add(3 * time.Minute)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create challenge: %v", err)
	}
	t.Cleanup(func() { _ = db.Client.CaptchaChallenge.DeleteOneID(created.ID).Exec(context.Background()) })

	if created.Status != 0 {
		t.Fatalf("status = %d, want 0", created.Status)
	}
	if created.TokenHash != nil {
		t.Fatalf("token_hash = %v, want nil", created.TokenHash)
	}
	if created.CreatedAt.IsZero() {
		t.Fatal("created_at not set")
	}

	hash := "h" + uniqueTag(t)
	row, err := db.Client.CaptchaChallenge.Create().
		SetScope("login").SetAnswerX(1).SetAnswerY(2).
		SetTokenHash(hash).SetExpiresAt(time.Now().Add(time.Minute)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create with token hash: %v", err)
	}
	t.Cleanup(func() { _ = db.Client.CaptchaChallenge.DeleteOneID(row.ID).Exec(context.Background()) })

	if _, err := db.Client.CaptchaChallenge.Create().
		SetScope("login").SetAnswerX(1).SetAnswerY(2).
		SetTokenHash(hash).SetExpiresAt(time.Now().Add(time.Minute)).
		Save(ctx); !ent.IsConstraintError(err) {
		t.Fatalf("duplicate token_hash err = %v, want constraint error", err)
	}
}
```

- [ ] **Step 2: 跑测试确认变红**

Run: `cd backend && set -a && . ./.env && set +a && go test ./internal/modules/captcha/ -run TestCaptchaChallengeSchema -count=1`
Expected: 编译失败，`db.Client.CaptchaChallenge` 未定义。

- [ ] **Step 3: 写 schema**

```go
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type CaptchaChallenge struct {
	ent.Schema
}

func (CaptchaChallenge) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }),
		field.String("scope").MaxLen(16).NotEmpty(),
		field.Int("answer_x"),
		field.Int("answer_y"),
		field.Int("status").Default(0),
		field.String("token_hash").MaxLen(64).Optional().Nillable().Unique(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("expires_at"),
	}
}

func (CaptchaChallenge) Indexes() []ent.Index {
	return []ent.Index{index.Fields("expires_at")}
}
```

- [ ] **Step 4: 生成 ent 代码**

Run: `cd backend && bash scripts/db-generate.sh`
Expected: 生成 `internal/database/ent/captchachallenge/`（此前遗留的空目录会被写入），`ent/migrate/schema.go` 同步。

- [ ] **Step 5: 生成并应用迁移**

Run:
```bash
cd backend
set -a && . ./.env && set +a
bash scripts/db-up.sh
atlas migrate diff captcha_challenges \
  --dir "file://migrations" \
  --to "ent://internal/database/ent/schema" \
  --dev-url "postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_atlas_dev?sslmode=disable"
bash scripts/db-migrate-apply.sh
```
Expected: 新增一个 `migrations/<时间戳>_captcha_challenges.sql`（只有 `CREATE TABLE captcha_challenges` + 唯一索引 + `expires_at` 索引），`atlas.sum` 更新；两条库都输出已应用。
若 `neoblog_atlas_dev` 不存在：在 WSL 执行 `docker exec neoblog-postgres createdb -U neoblog neoblog_atlas_dev` 后重试。

- [ ] **Step 6: 跑测试确认变绿**

Run: `cd backend && set -a && . ./.env && set +a && go test ./internal/modules/captcha/ -run TestCaptchaChallengeSchema -count=1 -v`
Expected: PASS。

- [ ] **Step 7: 阶段检查**

Run: `cd backend && bash scripts/check.sh`
Expected: `check ok`。

---

### Task 2: generator（引擎：素材加载 + 出题）

**Files:**
- Modify: `backend/go.mod`、`backend/go.sum`
- Create: `backend/internal/modules/captcha/generator.go`
- Test: `backend/internal/modules/captcha/generator_test.go`

**Interfaces:**
- Produces:
  - `type Generated struct { MasterImage, TileImage string; MasterWidth, MasterHeight, TileWidth, TileHeight, TileX, TileY, AnswerX, AnswerY int }`
  - `func NewGenerator() (*Generator, error)`
  - `func (g *Generator) Generate() (Generated, error)`

- [ ] **Step 1: 拉依赖**

Run: `cd backend && go get github.com/wenlng/go-captcha/v2@v2.0.5 github.com/wenlng/go-captcha-assets@v1.0.7`
Expected: `go.mod` 增加两条 require；暂不 `go mod tidy`（代码还没 import）。

- [ ] **Step 2: 写失败测试** `generator_test.go`

```go
package captcha

import (
	"strings"
	"testing"
)

func TestGeneratorProducesSlideChallenge(t *testing.T) {
	generator, err := NewGenerator()
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	got, err := generator.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if !strings.HasPrefix(got.MasterImage, "data:image/jpeg;base64,") {
		t.Fatalf("master image = %.40q", got.MasterImage)
	}
	if !strings.HasPrefix(got.TileImage, "data:image/png;base64,") {
		t.Fatalf("tile image = %.40q", got.TileImage)
	}
	if got.MasterWidth != 300 || got.MasterHeight != 220 {
		t.Fatalf("master size = %dx%d, want 300x220", got.MasterWidth, got.MasterHeight)
	}
	if got.TileWidth <= 0 || got.TileHeight <= 0 {
		t.Fatalf("tile size = %dx%d", got.TileWidth, got.TileHeight)
	}
	if got.AnswerX < 0 || got.AnswerX+got.TileWidth > got.MasterWidth {
		t.Fatalf("answer x = %d (tile %d, master %d)", got.AnswerX, got.TileWidth, got.MasterWidth)
	}
	if got.AnswerY < 0 || got.AnswerY+got.TileHeight > got.MasterHeight {
		t.Fatalf("answer y = %d (tile %d, master %d)", got.AnswerY, got.TileHeight, got.MasterHeight)
	}
	if got.TileX < 0 || got.TileX > got.MasterWidth {
		t.Fatalf("tile x = %d", got.TileX)
	}
	if got.TileY < 0 || got.TileY > got.MasterHeight {
		t.Fatalf("tile y = %d", got.TileY)
	}
}
```

- [ ] **Step 3: 跑测试确认变红**

Run: `cd backend && set -a && . ./.env && set +a && go test ./internal/modules/captcha/ -run TestGeneratorProducesSlideChallenge -count=1`
Expected: 编译失败，`NewGenerator` 未定义。

- [ ] **Step 4: 实现 generator.go**

- `NewGenerator` 内依次调用 `imagesv2.GetImages()` 与 `tiles.GetTiles()`，任一失败返回 `fmt.Errorf("captcha: load backgrounds: %w", err)` / `fmt.Errorf("captcha: load tiles: %w", err)`。
- 把 `[]*tiles.GraphImage` 逐个转成 `[]*slide.GraphImage`（字段同名：`OverlayImage` / `ShadowImage` / `MaskImage`）。
- 构建：`slide.NewBuilder()` → `SetResources(slide.WithBackgrounds(backgrounds), slide.WithGraphImages(graphs))` → `Make()`，存入 `Generator{captcha: built}`。
- `Generate()`：`data, err := g.captcha.Generate()`；`block := data.GetData()`；`master, err := data.GetMasterImage().ToBase64()`；`tile, err := data.GetTileImage().ToBase64()`；`size := g.captcha.GetOptions().GetImageSize()`。
- 返回值：`MasterWidth/MasterHeight = size.Width/Height`、`TileWidth/TileHeight = block.Width/Height`、`TileX/TileY = block.DX/DY`、`AnswerX/AnswerY = block.X/Y`。
- 库返回 `ToBase64()` 自带 `data:image/...;base64,` 前缀，不要自己拼。
- 本文件只 import 标准库与 go-captcha 两个包，不 import gin / ent / settings / apierror。

- [ ] **Step 5: 收口依赖并跑测试**

Run: `cd backend && go mod tidy && set -a && . ./.env && set +a && go test ./internal/modules/captcha/ -run TestGeneratorProducesSlideChallenge -count=1 -v`
Expected: PASS；`go.mod` 里 `golang.org/x/image`、`github.com/golang/freetype` 以 indirect 出现。

---

### Task 3: repository（存储：挑战表读写与原子状态迁移）

**Files:**
- Modify: `backend/internal/modules/captcha/dto.go`（加状态常量）
- Create: `backend/internal/modules/captcha/repository.go`
- Test: `backend/internal/modules/captcha/repository_test.go`

**Interfaces:**
- Produces:
  - 状态常量：`statusPending = 0`、`statusSolved = 1`、`statusConsumed = 2`、`statusFailed = 3`
  - `type challengeRecord struct { ID uuid.UUID; AnswerX int; AnswerY int }`
  - `var errChallengeNotFound = errors.New("captcha: challenge not found")`
  - `func newRepository(db *database.Database) *repository`
  - `func (r *repository) create(ctx context.Context, scope string, answerX, answerY int, expiresAt time.Time) (uuid.UUID, error)`
  - `func (r *repository) findPending(ctx context.Context, id uuid.UUID, now time.Time) (challengeRecord, error)`
  - `func (r *repository) markSolved(ctx context.Context, id uuid.UUID, tokenHash string, now, expiresAt time.Time) (int, error)`
  - `func (r *repository) markFailed(ctx context.Context, id uuid.UUID, now time.Time) (int, error)`
  - `func (r *repository) consume(ctx context.Context, scope, tokenHash string, now time.Time) (bool, error)`
  - `func (r *repository) deleteExpired(ctx context.Context, now time.Time) error`

- [ ] **Step 1: 写失败测试** `repository_test.go`

覆盖（每条用 `testDB(t)` + 直接写 ent 造数）：

```go
func TestChallengeRepositoryCreateAndFindPending(t *testing.T)      // create 后 findPending 拿到 answer；now 超过 expires_at → errChallengeNotFound
func TestChallengeRepositoryMarkSolvedIsSingleUse(t *testing.T)     // 第一次 markSolved 返回 1，第二次返回 0；markFailed 后 markSolved 返回 0
func TestChallengeRepositoryConsumeOnceAndScopeBound(t *testing.T)  // 正确 scope 第一次 true、第二次 false；错误 scope false；expires_at 改到过去后 false
func TestChallengeRepositoryDeleteExpired(t *testing.T)             // 造一条过期一条未过期，deleteExpired 后过期行消失、未过期行保留
```

关键断言写法：

```go
now := time.Now()
id, err := repo.create(ctx, "login", 10, 20, now.Add(3*time.Minute))
rec, err := repo.findPending(ctx, id, now)
if rec.AnswerX != 10 || rec.AnswerY != 20 { ... }
if _, err := repo.findPending(ctx, id, now.Add(4*time.Minute)); !errors.Is(err, errChallengeNotFound) { ... }
```

- [ ] **Step 2: 跑测试确认变红**

Run: `cd backend && set -a && . ./.env && set +a && go test ./internal/modules/captcha/ -run TestChallengeRepository -count=1`
Expected: 编译失败，`newRepository` 未定义。

- [ ] **Step 3: 实现 repository.go**

四个条件更新必须用谓词而不是先读后写：

```go
// markSolved
n, err := r.client.CaptchaChallenge.Update().
    Where(captchachallenge.IDEQ(id), captchachallenge.StatusEQ(statusPending), captchachallenge.ExpiresAtGT(now)).
    SetStatus(statusSolved).SetTokenHash(tokenHash).SetExpiresAt(expiresAt).
    Save(ctx)

// markFailed
n, err := r.client.CaptchaChallenge.Update().
    Where(captchachallenge.IDEQ(id), captchachallenge.StatusEQ(statusPending), captchachallenge.ExpiresAtGT(now)).
    SetStatus(statusFailed).
    Save(ctx)

// consume
n, err := r.client.CaptchaChallenge.Update().
    Where(captchachallenge.TokenHashEQ(tokenHash), captchachallenge.ScopeEQ(scope), captchachallenge.StatusEQ(statusSolved), captchachallenge.ExpiresAtGT(now)).
    SetStatus(statusConsumed).
    Save(ctx)
return n > 0, err
```

- `findPending`：`Where(IDEQ, StatusEQ(statusPending), ExpiresAtGT(now))`，`ent.IsNotFound` → `errChallengeNotFound`。
- `deleteExpired`：`Delete().Where(captchachallenge.ExpiresAtLT(now)).Exec(ctx)`。
- 不 import `apierror` / gin / settings；错误原样返回。

- [ ] **Step 4: 跑测试确认变绿**

Run: `cd backend && set -a && . ./.env && set +a && go test ./internal/modules/captcha/ -run TestChallengeRepository -count=1 -v`
Expected: 4 个用例 PASS。

---

### Task 4: builtin 能力（出题 / 交卷 / 凭证消费）+ 错误码

**Files:**
- Modify: `backend/internal/common/apierror/apierror.go`
- Modify: `backend/internal/common/apierror/apierror_test.go`
- Modify: `backend/internal/modules/captcha/dto.go`（请求 / 响应 DTO）
- Create: `backend/internal/modules/captcha/builtin.go`
- Test: `backend/internal/modules/captcha/builtin_test.go`

**Interfaces:**
- Consumes: Task 2 的 `Generator.Generate()`、Task 3 的 repository 方法与 `errChallengeNotFound`。
- Produces:
  - `type Policies struct { ChallengeLimit int; ChallengeWindow time.Duration; ChallengeTTL time.Duration; TokenTTL time.Duration; AnswerPadding int }`
  - `func DefaultPolicies() Policies`（60 / 1m / 3m / 5m / 4）
  - `const providerBuiltin = "builtin"`
  - `type builtinService struct { settings Settings; repo *repository; generator *Generator; limiter *ratelimit.Limiter; policies Policies }`
  - `func newBuiltinService(settings Settings, repo *repository, generator *Generator, limiter *ratelimit.Limiter, policies Policies) *builtinService`
  - `func (b *builtinService) Challenge(ctx context.Context, scope, ip string) (ChallengeResponse, error)`
  - `func (b *builtinService) Solve(ctx context.Context, id uuid.UUID, x, y int) (SolveChallengeResponse, error)`
  - `func (b *builtinService) VerifyToken(ctx context.Context, scope, token string) error`
  - 错误常量：`apierror.ErrCaptchaDisabled`、`apierror.ErrCaptchaRejected`

- [ ] **Step 1: 写失败测试** `builtin_test.go`

测试辅助：

```go
func newBuiltinHarness(t *testing.T, db *database.Database, policies Policies) (*builtinService, *settings.Service)

func setCaptchaSettings(t *testing.T, svc *settings.Service, enabled bool, provider, scopes string)
// 内部 SetBool/SetString，t.Cleanup 恢复 false / "none" / "login,register"

func challengeAnswer(t *testing.T, db *database.Database, id uuid.UUID) (int, int)
// 直接 ent 读 answer_x / answer_y
```

用例（全部直接打 `builtinService`，不经过 HTTP 与分派器）：

- `TestBuiltinChallengeAndSolveFlow`：builtin + `login,register`；`Challenge(ctx, "login", "1.2.3.4")` 返回字段合法（图片前缀、`ExpiresAt` 可解析且在未来）；`challengeAnswer` 取答案 → `Solve` 拿到非空 base64url 凭证与未来过期时间。
- `TestBuiltinChallengeRejectedWhenNotActive`：分别覆盖 `enabled=false`、`provider="turnstile"`、`scopes="register"` 且 scope=login → code `captcha_disabled`。
- `TestBuiltinChallengeRejectsUnknownScope`：`scope="signup"` → code `invalid_request`、`param == "scope"`。
- `TestBuiltinChallengeRateLimited`：`policies.ChallengeLimit=1`；第二次 → code `rate_limited`。
- `TestBuiltinSolveRejectsWrongAnswerAndBurnsChallenge`：错答案 → `captcha_failed`；随后正确答案 → 仍 `captcha_failed`。
- `TestBuiltinSolveIsSingleUse`：交卷两次，第二次 `captcha_failed`。
- `TestBuiltinVerifyTokenConsumesOnce`：交卷拿 token → `VerifyToken(ctx, "login", token)` 返回 nil；第二次 `captcha_failed`。
- `TestBuiltinVerifyTokenRejectsOtherScope`：login 的 token 调 `VerifyToken(ctx, "register", token)` → `captcha_failed`。
- `TestBuiltinVerifyTokenRejectsExpiredToken`：交卷后把 `expires_at` 改到过去 → `captcha_failed`。
- `TestBuiltinVerifyTokenPassesWhenScopeNotProtected`：`scopes="register"` 时 `VerifyToken(ctx, "login", "")` 返回 nil。

断言错误码统一用 `apierror.Body(err).Code` 与 `.Param`。

- [ ] **Step 2: 跑测试确认变红**

Run: `cd backend && set -a && . ./.env && set +a && go test ./internal/modules/captcha/ -run TestBuiltin -count=1`
Expected: 编译失败（`newBuiltinService` / 错误常量未定义）。

- [ ] **Step 3: 加错误常量**

`apierror.go`：

```go
ErrCaptchaDisabled = &Error{Type: TypePermission, Code: "captcha_disabled"}
ErrCaptchaRejected = &Error{Type: TypeInvalidRequest, Code: "captcha_failed"}
```

`statusByCode` 增加 `"captcha_disabled": http.StatusForbidden`（`captcha_failed` 已存在）。`apierror_test.go` 的 `TestRegistryMatchesSpec` 加两行用例（`captcha_disabled` 403、`captcha_failed` 400 且 `param == nil`）。

- [ ] **Step 4: 实现 builtin.go 与 DTO**

`builtin.go` 只依赖 settings 窄接口、repository、generator、limiter、policies：

- `Challenge` 顺序：
  1. `scope` 不是 `settings.CaptchaScopeLogin` / `settings.CaptchaScopeRegister` → `apierror.WithParam(apierror.ErrInvalidRequest, "scope")`。
  2. `ratelimit.AllowWindow(b.limiter, ratelimit.WindowPolicy{Limit: b.policies.ChallengeLimit, Window: b.policies.ChallengeWindow}, ratelimit.IPKey("captcha-challenges", ip))`，不允许 → `apierror.RateLimited(retryAfter)`。
  3. 读 `captcha_enabled` / `captcha_provider` / `captcha_scopes`，读失败直接上抛；`!enabled || provider != providerBuiltin || !slices.Contains(settings.SplitListValue(scopes), scope)` → `apierror.ErrCaptchaDisabled`。
  4. `b.generator.Generate()`；失败上抛。
  5. `now := time.Now()`；`b.repo.deleteExpired(ctx, now)` 失败上抛；`b.repo.create(ctx, scope, answerX, answerY, now.Add(b.policies.ChallengeTTL))` 失败上抛。
  6. 填 `ChallengeResponse`，`ExpiresAt` 用 `expiresAt.UTC().Format(time.RFC3339)`。
- `Solve` 顺序：
  1. `now`；`b.repo.findPending`：`errChallengeNotFound` → `ErrCaptchaRejected`；其它错误上抛。
  2. `slide.Validate(x, y, rec.AnswerX, rec.AnswerY, b.policies.AnswerPadding)` 为 false → `markFailed`（失败上抛）→ `ErrCaptchaRejected`。
  3. 生成凭证：`crypto/rand` 读 32 字节 → `base64.RawURLEncoding.EncodeToString`；`hashToken(token)` = `hex.EncodeToString(sha256.Sum256([]byte(token))[:])`。
  4. `markSolved(..., now.Add(b.policies.TokenTTL))`：错误上抛；返回 0 → `ErrCaptchaRejected`。
  5. 返回 `SolveChallengeResponse`。
- `VerifyToken`：
  1. 读 `captcha_scopes`（失败上抛）；scope 不在清单 → nil。
  2. token 空 → `ErrCaptchaFailed`。
  3. `b.repo.consume(ctx, scope, hashToken(token), time.Now())`：错误上抛；false → `ErrCaptchaFailed`；true → nil。
- DTO（`dto.go`，与状态常量同文件）：

```go
type CreateChallengeRequest struct{ Scope string `json:"scope"` }

type ChallengeResponse struct {
	ID           uuid.UUID `json:"id"`
	MasterImage  string    `json:"master_image"`
	TileImage    string    `json:"tile_image"`
	MasterWidth  int       `json:"master_width"`
	MasterHeight int       `json:"master_height"`
	TileWidth    int       `json:"tile_width"`
	TileHeight   int       `json:"tile_height"`
	TileX        int       `json:"tile_x"`
	TileY        int       `json:"tile_y"`
	ExpiresAt    string    `json:"expires_at"`
}

type SolveChallengeRequest struct {
	X *int `json:"x"`
	Y *int `json:"y"`
}

type SolveChallengeResponse struct {
	CaptchaToken string `json:"captcha_token"`
	ExpiresAt    string `json:"expires_at"`
}
```

- `builtin.go` 里不出现 gin、`net/http`、SQL 语句；除 `captcha_enabled` / `captcha_provider` / `captcha_scopes` 外不读别的设置键。

- [ ] **Step 5: 跑测试确认变绿**

Run: `cd backend && set -a && . ./.env && set +a && go test ./internal/modules/captcha/ ./internal/common/apierror/ -count=1 -v`
Expected: 全部 PASS；既有 Turnstile 用例不受影响。

---

### Task 5: 分派器（service.go 薄改，Turnstile 不动）

**Files:**
- Modify: `backend/internal/modules/captcha/service.go`
- Test: `backend/internal/modules/captcha/dispatch_test.go`

**Interfaces:**
- Consumes: Task 4 的 `builtinService` 三个方法与 `newBuiltinService`。
- Produces:
  - `Service` 结构变为 `settings Settings; verifier Verifier; builtin *builtinService`
  - `NewService(settings, verifier)` 保持两参（`builtin` 为 nil，既有测试继续用）
  - 包内 `newService(settings Settings, verifier Verifier, builtin *builtinService) *Service`
  - `func (s *Service) Challenge(ctx, scope, ip) (ChallengeResponse, error)` / `func (s *Service) Solve(ctx, id, x, y) (SolveChallengeResponse, error)`：`builtin == nil` → `ErrServiceUnavailable`，否则委托
  - `Verify`：`turnstile` 分支原文不动；`builtin` → `builtin == nil` 时 `ErrServiceUnavailable`，否则 `s.builtin.VerifyToken(ctx, scope, token)`；其余保持放行

- [ ] **Step 1: 写失败测试** `dispatch_test.go`

- `TestDispatchChallengeUnavailableWhenBuiltinNotWired`：`NewService(settingsSvc, nil)` → `Challenge` → code `service_unavailable`。
- `TestDispatchVerifyUnavailableWhenBuiltinNotWired`：同样构造，`provider=builtin`、token `"x"` → code `service_unavailable`。
- `TestDispatchVerifyPassesWhenCaptchaDisabled`：`enabled=false` → `Verify(ctx, "login", "", "")` 返回 nil。
- `TestDispatchVerifyPassesWhenProviderUnknown`：`provider="none"` 与 `provider="geetest"` 各一次 → 返回 nil。
- `TestDispatchVerifyRoutesTurnstileToVerifier`：用现有 `fakeVerifier` 注入 outcome；`provider=turnstile`、secret 非空、`scopes="login"`，断言 verifier 被调用且 `Passed → nil`、`Rejected → captcha_failed`。
- `TestDispatchVerifyRoutesBuiltin`：用 `newService(settingsSvc, nil, newBuiltinService(...))`；走完出题 + 交卷后用 `Verify(ctx, "login", token, "")` → nil（分派器与 builtin 串起来的组合测试）。

- [ ] **Step 2: 跑测试确认变红**

Run: `cd backend && set -a && . ./.env && set +a && go test ./internal/modules/captcha/ -run TestDispatch -count=1`
Expected: 编译失败（`newService` 三参签名未就位）。

- [ ] **Step 3: 改 service.go**

- 结构体只留 `settings` / `verifier` / `builtin` 三个字段；删除任何挑战表、令牌、限流相关代码（它们已在 `builtin.go`）。
- `NewService`：`return &Service{settings: settings, verifier: verifier}`（`builtin` 为 nil）。
- `newService`：三参直填；`verifier` 为 nil 时沿用现有 `unavailableVerifier` 语义。
- `Verify`：
  - `enabled` / `provider` 读取与提前返回逻辑保持现状；
  - `case providerTurnstile:` 现有判定链（secret 空、scope 过滤、token 空、verifier 三态）逐行保留；
  - `case providerBuiltin:` 直接委托 `s.builtin`（nil → 503）；
  - `default: return nil`。
- `Challenge` / `Solve` 只做 nil 判断与委托。

- [ ] **Step 4: 跑测试确认变绿**

Run: `cd backend && set -a && . ./.env && set +a && go test ./internal/modules/captcha/ -count=1 -v`
Expected: 包内全部 PASS（Turnstile 既有用例逐条通过，证明分支未动）。

---

### Task 6: 路由与装配（HTTP + module + app）

**Files:**
- Modify: `backend/internal/modules/captcha/controller.go`
- Modify: `backend/internal/modules/captcha/module.go`
- Modify: `backend/internal/app/app.go`
- Test: `backend/internal/modules/captcha/challenge_route_test.go`

**Interfaces:**
- Consumes: Task 5 的 `newService(settings, verifier, builtin)`、Task 4 的 `newBuiltinService` / `Policies`。
- Produces: `Module.Register(r *gin.RouterGroup)` 挂两条公开路由；`captcha.New(db, settingsService, limiter) (*Module, error)`。

- [ ] **Step 1: 写失败测试** `challenge_route_test.go`

辅助 `newChallengeRouteTestApp(t, db, policies)`：`settings.New(db)` → `newService(settingsSvc, nil, newBuiltinService(settingsSvc, newRepository(db), mustGenerator(t), ratelimit.New(), policies))` → `gin.New()` + `api := engine.Group("/api/v1")` + `module.Register(api)` + `httptest.NewServer`。辅助 `mustGenerator(t)` = `NewGenerator()`，失败即 `t.Fatal`。

用例：

- `TestChallengeRouteRequiresJSON`：`Content-Type: text/plain` → 400 `invalid_request`。
- `TestChallengeRouteCreatesChallenge`：POST `{"scope":"login"}` → 201；解析 `ChallengeResponse`，断言 `id` 可解析、两张图前缀、`expires_at` 可被 `time.RFC3339` 解析。
- `TestChallengeRouteRejectsUnknownScope`：`{"scope":"signup"}` → 400、`param == "scope"`。
- `TestChallengeRouteRejectsWhenInactive`：`enabled=false` → 403 `captcha_disabled`；`provider=turnstile` → 403。
- `TestChallengeRouteRateLimited`：`policies.ChallengeLimit=1`；第二次 → 429、`code == "rate_limited"`、`Retry-After` 头非空。
- `TestSolveRouteRejectsMalformedID`：`/captcha/challenges/not-a-uuid/solutions` → 400、`param == "id"`。
- `TestSolveRouteRejectsMissingAndNegativeCoordinates`：`{}` → 400 `param == "x"`；`{"x":1}` → 400 `param == "y"`；`{"x":-1,"y":2}` → 400 `param == "x"`。
- `TestSolveRouteInvalidInputKeepsChallengePending`：先用 `{"x":-1,"y":2}` 打已存在的题目 → 400；再从库里读正确答案交卷 → 200（非法输入没有把题目判负）。
- `TestSolveRouteWrongAnswer`：错误坐标 → 400 `captcha_failed`、`param == nil`；随后库内该行 `status == 3`。

- [ ] **Step 2: 跑测试确认变红**

Run: `cd backend && set -a && . ./.env && set +a && go test ./internal/modules/captcha/ -run 'TestChallengeRoute|TestSolveRoute' -count=1`
Expected: 编译失败（`Module.Register` 未定义）。

- [ ] **Step 3: 写 controller 与 module**

controller：

```go
func (c *controller) register(r *gin.RouterGroup) {
	r.POST("/captcha/challenges", c.createChallenge)
	r.POST("/captcha/challenges/:id/solutions", c.solveChallenge)
}
```

- `createChallenge`：`requireJSONRequest` → 绑 `CreateChallengeRequest`（绑定失败 → `ErrInvalidRequest`）→ `c.service.Challenge(ctx, req.Scope, clientIP(ctx))` → `201`。
- `solveChallenge`：`requireJSONRequest` → `uuid.Parse(c.Param("id"))` 失败 → `apierror.WithParam(apierror.ErrInvalidRequest, "id")` → 绑 `SolveChallengeRequest`；`req.X == nil || *req.X < 0` → `WithParam(..., "x")`；`req.Y` 同理 → `c.service.Solve(...)` → `200`。
- 所有错误走 `apierror.Write`；controller 不碰 DB、不拼 JSON 字符串。

module：

```go
func New(db *database.Database, settingsService *settings.Service, limiter *ratelimit.Limiter) (*Module, error) {
	generator, err := NewGenerator()
	if err != nil {
		return nil, err
	}
	builtin := newBuiltinService(settingsService, newRepository(db), generator, limiter, DefaultPolicies())
	service := newService(settingsService, newTurnstileVerifier(turnstileSiteverifyURL, turnstileTimeout), builtin)
	return &Module{Service: service, controller: newController(service)}, nil
}

func (m *Module) Register(r *gin.RouterGroup) { m.controller.register(r) }
```

保留 `RegisterProtected` 与 `NewService` 两参构造函数（既有测试在用）。

- [ ] **Step 4: 接进 app.go**

```go
captchaModule, err := captcha.New(db, settingsModule.Service, limiter)
if err != nil {
	return nil, err
}
captchaService := captchaModule.Service
```

并在 `authModule.Register(api)` 旁加 `captchaModule.Register(api)`。`app.go` 本轮只改这几行。

- [ ] **Step 5: 跑测试确认变绿**

Run: `cd backend && set -a && . ./.env && set +a && go test ./internal/modules/captcha/ -count=1 -v && go build ./...`
Expected: 全 PASS；`go build ./...` 通过。

---

### Task 7: 端到端与降级

**Files:**
- Create: `backend/internal/app/captcha_test.go`（package `app`）

**Interfaces:**
- Consumes: Task 6 的公开路由、`newTestServer` / `newDegradedServer` / `createTestUser`（均在同包 `app_test.go`）。

- [ ] **Step 1: 写测试** `captcha_test.go`

- `TestBuiltinCaptchaLoginFlow`：
  1. `ts, db := newTestServer(t, config.EnvironmentDevelopment)`；用 `settings.New(db)` 得到 service，`SetBool("captcha","captcha_enabled",true)`、`SetString(...,"captcha_provider","builtin")`、`SetString(...,"captcha_scopes","login")`，`t.Cleanup` 恢复 `false` / `none` / `login,register`。
  2. `createTestUser(t, db)` 拿用户与密码。
  3. POST `/api/v1/captcha/challenges` `{"scope":"login"}` → 201，解析 `id`。
  4. 从 `db.Client.CaptchaChallenge.Get(ctx, id)` 读 `answer_x/answer_y`。
  5. POST `/api/v1/captcha/challenges/{id}/solutions` `{"x":..,"y":..}` → 200，拿 `captcha_token`。
  6. POST `/api/v1/sessions`，body 带该 token → 200 且下发会话 cookie。
  7. 用同一 token 再登录一次 → 400 `captcha_failed`（一次性在真实链路生效）。
- `TestBuiltinCaptchaChallengeFailsWhenDatabaseUnavailable`：`newDegradedServer(t)` → POST `/api/v1/captcha/challenges` → 503 `service_unavailable`。

- [ ] **Step 2: 跑测试**

Run: `cd backend && set -a && . ./.env && set +a && go test ./internal/app/ -run 'TestBuiltinCaptcha' -count=1 -v`
Expected: 两条 PASS（端到端回归，不是失败优先）。若为红，按失败点回查 Task 6 的装配或 Task 5 的分派，不在本任务里加兜底。

- [ ] **Step 3: 全仓回归**

Run: `cd backend && set -a && . ./.env && set +a && bash scripts/test.sh`
Expected: 全绿；既有登录/注册/Turnstile 用例不变。

---

### Task 8: 文档、版本与最终验证

**Files:**
- Modify: `backend/README.md`
- Modify: `docs/DEVLOG.md`
- Modify: `backend/internal/version/version.go`（脚本改）

- [ ] **Step 1: README**

- API 表新增两行（放在 `POST /api/v1/users/email-verifications` 之后），写清请求/响应/错误码与「凭证一次性、5 分钟、只绑 scope」。
- 模块表 `internal/modules/captcha/*` 行：补「出题 / 判分 / 一次性凭证 / `builtin` 判定」与 `Register` 公开路由。
- 迁移清单追加 `captcha_challenges`。
- 设置章节 `captcha` 组段落：把「`builtin` 选了不拦」改为实际行为（四条件、凭证语义、3/5 分钟、不绑 IP），并加「切 provider 前必须先上前端」。
- 版本行由 bump 脚本自动改。

- [ ] **Step 2: DEVLOG 条目**

按倒序插在 `---` 之后：范围、动机、决策（协议魔改、一次性凭证、不绑 IP、限流阈值、素材实测数字）、验证、已知缺口（三种形态、provider 白名单、前端未接、过期行清理依赖出题）。标注**未提交**。

- [ ] **Step 3: 版本号（只跑一次）**

Run: `cd backend && bash scripts/bump-version.sh minor`
Expected: `0.23.1 -> 0.24.0`；随后只读 `internal/version/version.go` 核对，不重跑脚本。

- [ ] **Step 4: 变异自检（三条，验完立即还原）**

1. 把 `builtin.VerifyToken` 里的 scope 清单判断删掉（`if !slices.Contains(...) { return nil }` 整行）→ 跑 `go test ./internal/modules/captcha/ -run TestBuiltinVerifyTokenPassesWhenScopeNotProtected -count=1`，Expected: FAIL。
2. 把 repository `consume` 的 `SetStatus(statusConsumed)` 删掉（只查不改）→ 跑 `-run TestBuiltinVerifyTokenConsumesOnce`，Expected: FAIL（第二次仍放行）。
3. 把 `consume` 的 `captchachallenge.ExpiresAtGT(now)` 谓词删掉 → 跑 `-run TestBuiltinVerifyTokenRejectsExpiredToken` 或 `-run TestChallengeRepositoryConsumeOnceAndScopeBound`，Expected: FAIL。
每条验完恢复原状，再跑一次对应用例确认回到 PASS。

- [ ] **Step 5: 最终验证**

```bash
cd backend
bash scripts/check.sh
set -a && . ./.env && set +a
bash scripts/test.sh
cd ..
git status --short
```
Expected: `check ok`；测试全绿；`git status` 只包含本轮文件 + 并行会话既有改动（不得误动 `auth` / `twofactor` 的在制品）。

- [ ] **Step 6: 汇报并等待提交指令**

向用户汇报：改动清单、实测数字（二进制 / 内存 / 出题耗时）、未提交状态。**用户确认后再执行一次提交**（单笔，只暂存本轮文件；`app.go` 等与并行会话共享的文件先 `git diff` 核对，必要时 `git add -p` 只取本轮 hunk）：

```bash
git add backend/go.mod backend/go.sum \
        backend/internal/common/apierror \
        backend/internal/database/ent \
        backend/internal/modules/captcha \
        backend/internal/app \
        backend/internal/version \
        backend/migrations \
        backend/README.md \
        docs/DEVLOG.md \
        docs/superpowers/specs/2026-10-26-neoblog-builtin-captcha-design.md \
        docs/superpowers/plans/2026-10-26-neoblog-builtin-captcha.md
git commit -m "feat(captcha): 接入 go-captcha 滑块自研人机验证"
```
