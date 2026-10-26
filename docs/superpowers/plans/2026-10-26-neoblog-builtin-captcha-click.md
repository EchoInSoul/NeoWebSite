# 自研人机验证新增「点选文字」形态 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给 `builtin` 人机验证新增「点选文字」形态：大图上散落中文字，后端指定 3 个目标字并按序判分，通过设置项 `captcha_mode` 在滑块与点选之间切换。

**Architecture:** `builtin` 包内拆出两个生成器（滑块 / 点选），service 读 `captcha_mode` 后分派；挑战表答案列统一改为 `answer_dots`（jsonb），滑块答案也搬进去，`answer_x` / `answer_y` 删除；判分统一走「一组提交坐标对一组答案 dot」的路径，滑块是长度为 1 的特例；凭证、状态机、限流全部复用。

**Tech Stack:** Go 1.27 / gin / ent / atlas / `github.com/wenlng/go-captcha/v2 v2.0.5`（`slide` + `click` 子包）/ `github.com/wenlng/go-captcha-assets v1.0.7`（`imagesv2` + `fonts/fzshengsksjw`）/ PostgreSQL。

**Spec:** `docs/superpowers/specs/2026-10-26-neoblog-builtin-captcha-click-design.md`

## Global Constraints

- 只改 `backend/` 与 `docs/`；`frontend/` 零改动。
- 无兜底：`captcha_mode` 缺行 / 坏值 → 503 / 500，不放行、不默认当 `slide`。
- 素材加载失败让 `app.New` 启动失败，不降级；素材只用官方包，不改图。
- 固定值：目标字数 **3**、判分容差 `padding=4`、题目 TTL 3 分钟、凭证 TTL 5 分钟、出题限流 5 次 / 10 分钟每 IP、失败共用桶 15 次 / 10 分钟。
- 顺序是硬语义：三个坐标集合正确但顺序打乱 → `captcha_failed`。
- `clicks` 必须恰好 3 个元素；元素 `x` / `y` 非负整数。形状非法 → `400 invalid_request`，且不改变题目状态。
- 不新增错误码、不改既有码义；不新增设置键以外的任何设置改动；零新依赖。
- 全仓零人工注释。
- 不逐任务提交；最终提交与否由用户决定（用户口径：绝不主动提交）。
- 版本号不提、不改（用户口径：不要你管版本号）。

## Review Focus

1. **提交顺序打乱但集合正确**（点选最核心的新语义）：必须 `400 captcha_failed` 且题目作废——Task 4 的顺序打乱用例。
2. **形状非法的交卷把题目误判负**（用户只是填错表单，却不能再交卷）：必须 `400 invalid_request` 且题目仍可交卷成功——Task 4、Task 6 的「invalid input keeps pending」用例。
3. **点选题被滑块判分函数处理**（形态分派漏了，`slide.Validate` 对 3 个点只会拿第一个比）：必须按形态路由判分——Task 3 的形态分派测试 + Task 7 的变异自检。
4. **存量行在新列下既判不了分又不清**（迁移只加列不清行，旧 `answer_x` 行变成永远 pending 的垃圾）：迁移必须 `DELETE FROM captcha_challenges`——Task 2 的迁移验证。
5. **`captcha_mode` 缺 seed 行时静默退回滑块**（违反项目「读取不兜底」口径）：必须 503——Task 3、Task 5 的缺行用例 + Task 7 的变异自检。

---

### Task 1: 点选生成器（素材加载 + 出题）

**Files:**
- Create: `backend/internal/modules/captcha/builtin/click_generator.go`
- Test: `backend/internal/modules/captcha/builtin/click_generator_test.go`

**Interfaces:**
- Consumes: 无（第一个任务，只用 go-captcha 库与官方素材）。
- Produces:
  - `newClickGenerator() (*clickGenerator, error)` — 启动期加载 `imagesv2.GetImages()` 背景 + `fzshengsksjw.GetFont()` 字体，构建 `click.NewBuilder()` 实例（`WithFonts`、`WithBackgrounds`、`WithRangeVerifyLen(option.RangeVal{Min: 3, Max: 3})`）；素材加载失败返回错误。
  - `(g *clickGenerator) generate() (clickGenerated, error)` — 返回 `clickGenerated{MasterImage, PromptImage, PromptText string, MasterWidth, MasterHeight int, Dots []answerDot}`。
  - `answerDot{Text string, X, Y, Width, Height int}` — 定义在 `dto.go`（Task 3 会用到，此处先建）。

- [ ] **Step 1: 写失败的测试 `TestClickGeneratorProducesChallenge`**

```go
func TestClickGeneratorProducesChallenge(t *testing.T) {
	generator, err := newClickGenerator()
	if err != nil {
		t.Fatalf("newClickGenerator: %v", err)
	}

	generated, err := generator.generate()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if !strings.HasPrefix(generated.MasterImage, "data:image/jpeg;base64,") {
		t.Fatalf("master image = %.40q", generated.MasterImage)
	}
	if !strings.HasPrefix(generated.PromptImage, "data:image/png;base64,") {
		t.Fatalf("prompt image = %.40q", generated.PromptImage)
	}
	if generated.MasterWidth != 300 || generated.MasterHeight != 220 {
		t.Fatalf("master size = %dx%d, want 300x220", generated.MasterWidth, generated.MasterHeight)
	}
	if len(generated.Dots) != 3 {
		t.Fatalf("dots = %d, want 3", len(generated.Dots))
	}
	seen := map[string]bool{}
	for i, dot := range generated.Dots {
		if dot.Text == "" || seen[dot.Text] {
			t.Fatalf("dots[%d].Text = %q, want unique non-empty", i, dot.Text)
		}
		seen[dot.Text] = true
		if dot.X < 0 || dot.Y < 0 || dot.Width <= 0 || dot.Height <= 0 {
			t.Fatalf("dots[%d] = %+v, want positive box inside master", i, dot)
		}
		if dot.X+dot.Width > generated.MasterWidth || dot.Y+dot.Height > generated.MasterHeight {
			t.Fatalf("dots[%d] box %+v outside %dx%d", i, dot, generated.MasterWidth, generated.MasterHeight)
		}
	}
	want := strings.Join([]string{generated.Dots[0].Text, generated.Dots[1].Text, generated.Dots[2].Text}, " ")
	if generated.PromptText != want {
		t.Fatalf("prompt text = %q, want %q", generated.PromptText, want)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend && export PATH="/f/software/go/bin:$PATH" && go test ./internal/modules/captcha/builtin/ -run TestClickGeneratorProducesChallenge -count=1`
Expected: FAIL（`newClickGenerator` 未定义）。

- [ ] **Step 3: 实现 `click_generator.go` 与 `dto.go` 的 `answerDot`**

`click_generator.go`：`newClickGenerator()` 依次加载 `imagesv2.GetImages()`、`fzshengsksjw.GetFont()`，任一失败即 `fmt.Errorf("captcha: load click resources: %w", err)`；用 `click.NewBuilder()` + `SetResources(click.WithBackgrounds(...), click.WithFonts(...))` + `SetOptions(click.WithRangeVerifyLen(option.RangeVal{Min: 3, Max: 3}))` + `Make()` 建实例。

`generate()`：调 `g.captcha.Generate()`；`data.GetData()` 是 `map[int]*click.Dot`（**只有 3 个目标字**，其余字不在内），按 `Index` 排序后转成 `[]answerDot`；主图走 `data.GetMasterImage().ToBase64()`，提示图走 `data.GetThumbImage().ToBase64()`；尺寸走 `g.captcha.GetOptions().GetImageSize()`；`PromptText` 用 `strings.Join(dot.Text, " ")`。`block == nil` / 空 map / 长度不为 3 都返回错误。

`dto.go` 加：

```go
type answerDot struct {
	Text   string `json:"text"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend && export PATH="/f/software/go/bin:$PATH" && go test ./internal/modules/captcha/builtin/ -run TestClickGeneratorProducesChallenge -count=1 -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add backend/internal/modules/captcha/builtin/click_generator.go backend/internal/modules/captcha/builtin/click_generator_test.go backend/internal/modules/captcha/builtin/dto.go
git commit -m "feat(captcha): 实现 builtin 点选文字生成器"
```

---

### Task 2: 挑战表答案列改为 jsonb（ent + 迁移）

**Files:**
- Modify: `backend/internal/database/ent/schema/captcha_challenge.go`
- Modify: `backend/internal/modules/captcha/builtin/helpers_test.go`（`challengeAnswer` 改读 `answer_dots`）
- Create: `backend/migrations/<时间戳>_captcha_challenge_answer_dots.sql`（atlas diff 生成）
- Modify: `backend/migrations/atlas.sum`

**Interfaces:**
- Consumes: `answerDot`（Task 1）。
- Produces:
  - ent schema `CaptchaChallenge`：`answer_dots`（`field.JSON("answer_dots", json.RawMessage{})`，非空）+ `prompt_text`（`field.String("prompt_text").Default("")`，非空），`answer_x` / `answer_y` 删除。
  - 迁移文件清空存量行（`DELETE FROM captcha_challenges`）+ 改列。

- [ ] **Step 1: 改 ent schema 并重新生成**

`schema/captcha_challenge.go` 的 `Fields()` 改为：

```go
field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }),
field.String("scope").MaxLen(16).NotEmpty(),
field.JSON("answer_dots", json.RawMessage{}),
field.String("prompt_text").Default(""),
field.Int("status").Default(0),
field.String("token_hash").MaxLen(64).Optional().Nillable().Unique(),
field.Time("created_at").Default(time.Now).Immutable(),
field.Time("expires_at"),
```

（`encoding/json` 要 import。）

Run: `cd backend && bash scripts/db-generate.sh`
Expected: 退出 0，`internal/database/ent/captchachallenge*` 与 `captchachallenge/` 重新生成，出现 `SetAnswerDots` / `AnswerDots` / `SetPromptText`，`AnswerX` / `AnswerY` 消失。

- [ ] **Step 2: 生成迁移并补清空语句**

Run: `cd backend && ATLAS_DEV_URL="postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_atlas_dev?sslmode=disable" atlas migrate diff captcha_challenge_answer_dots --dir "file://migrations" --to "ent://internal/database/ent/schema"`

（`scripts/db-migrate-diff.sh` 里的迁移名是写死的 `user_bio`，不能复用，必须照上面这样手写完整命令；`ATLAS_DEV_URL` 取脚本内同款默认值。）
Expected: 新增 `migrations/<时间戳>_captcha_challenge_answer_dots.sql`，内容为 `ALTER TABLE ... DROP COLUMN answer_x, DROP COLUMN answer_y, ADD COLUMN answer_dots jsonb NOT NULL, ADD COLUMN prompt_text text NOT NULL DEFAULT ''` 一类语句。

在生成的 SQL 末尾追加（存量行答案形状已变，全部作废）：

```sql
DELETE FROM "public"."captcha_challenges";
```

Run: `cd backend && atlas migrate hash --dir "file://migrations" && atlas migrate validate --dir "file://migrations"`
Expected: 两条都退出 0。

- [ ] **Step 3: 改测试 helper 并跑 builtin 包编译**

`helpers_test.go` 的 `challengeAnswer` 改为返回 `[]answerDot`：读 `row.AnswerDots`，`json.Unmarshal` 进 `struct{ Dots []answerDot \`json:"dots"\` }`，返回 `Dots`。同步修正 `service_test.go` / `controller_test.go` 里所有 `x, y := challengeAnswer(...)` 的调用点（滑块答案是 `dots[0].X` / `dots[0].Y`）。

Run: `cd backend && export PATH="/f/software/go/bin:$PATH" && go build ./... && go vet ./internal/modules/captcha/...`
Expected: 退出 0。

- [ ] **Step 4: 应用到两个库并验证表结构**

Run: `cd backend && source .env && bash scripts/db-migrate-apply.sh`
Expected: 退出 0。用 `.scratch` 里的 pgx 探针查 `information_schema.columns`：`answer_dots` / `prompt_text` 在、`answer_x` / `answer_y` 不在、`token_hash` 唯一索引与 `captchachallenge_expires_at` 索引仍在、`SELECT count(*) FROM captcha_challenges` 为 0。

- [ ] **Step 5: 提交**

```bash
git add backend/internal/database/ent/schema/captcha_challenge.go backend/internal/database/ent/ backend/migrations/ backend/internal/modules/captcha/builtin/
git commit -m "refactor(db): captcha_challenges 答案列统一为 answer_dots"
```

---

### Task 3: `captcha_mode` 设置项与形态分派

**Files:**
- Modify: `backend/internal/modules/settings/registry.go`
- Modify: `backend/internal/modules/settings/registry_test.go`
- Create: `backend/migrations/<时间戳>_seed_captcha_mode.sql`
- Modify: `backend/migrations/atlas.sum`
- Modify: `backend/internal/modules/captcha/builtin/service.go`
- Modify: `backend/internal/modules/captcha/builtin/dto.go`
- Test: `backend/internal/modules/captcha/builtin/service_test.go`

**Interfaces:**
- Consumes: `answerDot`、`newClickGenerator` / `(*clickGenerator).generate`（Task 1）。
- Produces:
  - 设置键 `captcha.captcha_mode`：文字、默认 `"slide"`、`Public`；校验只认 `slide` / `click`。
  - `modeSlide = "slide"`、`modeClick = "click"` 常量（`builtin` 包内）。
  - `Service.Challenge(ctx, scope, ip) (ChallengeResponse, error)` — 读 `captcha_mode` 后分派；`click` 时响应带 `PromptImage` / `PromptText` / `ExpectedClicks`，`slide` 时保持原字段。
  - `Service.Solve(ctx, id, clicks []Point) (SolveChallengeResponse, error)` — 按题目 `answer_dots` 里 dots 的长度路由判分（长度 1 走 slide，长度 3 走 click）。
  - `Point{X, Y int}`；`ClickChallengeResponse`（点选响应字段）。

- [ ] **Step 1: 写失败的设置校验测试**

`registry_test.go` 加（照同文件 `TestValidateMailGroup` 的表驱动风格，直接测 `validateCaptchaGroup`）：

```go
func TestValidateCaptchaGroup(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mode    string
		touched bool
		wantErr bool
	}{
		{"slide", "slide", true, false},
		{"click", "click", true, false},
		{"unknown value", "foo", true, true},
		{"empty string", "", true, true},
		{"untouched invalid value", "foo", false, false},
	} {
		touched := map[string]bool{"captcha_mode": tc.touched}
		err := validateCaptchaGroup(map[string]any{"captcha_mode": tc.mode}, touched)
		if (err != nil) != tc.wantErr {
			t.Fatalf("%s: err = %v, wantErr = %v", tc.name, err, tc.wantErr)
		}
	}
}
```

同文件 `TestBuiltinRegistryShape` 的 captcha 断言从 4 项改 5 项并补 `captcha_mode` 定义（`FieldTypeString`、默认 `"slide"`、`public: true`）。

Run: `cd backend && export PATH="/f/software/go/bin:$PATH" && go test ./internal/modules/settings/ -run 'TestCaptchaMode|TestRegistry' -count=1`
Expected: FAIL（`captcha_mode` 未定义）。

- [ ] **Step 2: 加设置键与校验**

`registry.go` 的 `captcha` 组加：

```go
{
	Name:       "captcha",
	ValidateOn: []string{"captcha_mode"},
	Validate:   validateCaptchaGroup,
	Items: []Definition{
		{Name: "captcha_enabled", Type: FieldTypeBool, Default: false, Public: true},
		{Name: "captcha_provider", Type: FieldTypeString, Default: "none", Public: true},
		{Name: "captcha_mode", Type: FieldTypeString, Default: "slide", Public: true},
		{Name: "turnstile_site_key", Type: FieldTypeString, Default: "", Public: true},
		{Name: "turnstile_secret_key", Type: FieldTypeString, Default: ""},
	},
},
```

新增 `validateCaptchaGroup(values map[string]any, touched map[string]bool) error`：`touched["captcha_mode"]` 时读字符串值，不是 `slide` / `click` 即返回 `&ItemError{Item: "captcha_mode", Err: apierror.ErrSettingConflict}`。

Run: `cd backend && export PATH="/f/software/go/bin:$PATH" && go test ./internal/modules/settings/ -run 'TestCaptchaMode|TestRegistry' -count=1`
Expected: PASS。

- [ ] **Step 3: 加 seed 迁移**

创建 `backend/migrations/<时间戳>_seed_captcha_mode.sql`（文件名必须含 `_seed_`，否则 seed 对账测试读不到；行内容形状必须匹配 `seedRowPattern`，即 `(gen_random_uuid(), '<group>', '<name>', '<json>'::jsonb, now())`）：

```sql
INSERT INTO "public"."setting_values" ("id", "setting_group", "name", "value", "updated_at")
VALUES (gen_random_uuid(), 'captcha', 'captcha_mode', '"slide"'::jsonb, now())
ON CONFLICT ("setting_group", "name") DO NOTHING;
```

`seed_migration_test.go` 的 `obsoleteSeedItems` 不动（那是对 `captcha_scopes` 的既有豁免）。

Run: `cd backend && atlas migrate hash --dir "file://migrations" && atlas migrate validate --dir "file://migrations" && source .env && bash scripts/db-migrate-apply.sh`
Expected: 三条都退出 0。随后 `go test ./internal/modules/settings/ -run TestSeedMigrationMatchesBuiltinRegistry -count=1` PASS（新键必须有 seed 行，否则这条测试红）。

- [ ] **Step 4: 写失败的形态分派测试**

`service_test.go` 加 `TestChallengeDispatchesByMode`：`captcha_mode=click` 时 `Challenge` 返回的 `PromptImage` / `PromptText` 非空、`ExpectedClicks == 3`、`TileImage == ""`；`captcha_mode=slide` 时 `TileImage` 非空、`PromptImage == ""`。helper `setCaptchaSettings` 扩一个 `mode` 参数（同步改所有既有调用点）。

Run: `cd backend && export PATH="/f/software/go/bin:$PATH" && source .env && go test ./internal/modules/captcha/builtin/ -run TestChallengeDispatchesByMode -count=1`
Expected: FAIL（新字段不存在 / 未分派）。

- [ ] **Step 5: 实现 service 分派**

`service.go`：

- `Service` 结构加 `clickGen *clickGenerator`；`newService(...)` 增参 `clickGen`；`NewWithPolicies` 里两个生成器都构造（点选素材加载失败让 `New` 失败）。
- **测试侧同步**：`newService` 增参会让 `helpers_test.go` 的 `newServiceHarness` 与 `controller_test.go` 的 `newChallengeRouteTestApp` 两处调用点编译失败——这两个 helper 改为多建一个 `mustClickGenerator(t)`（与既有 `mustGenerator(t)` 同风格）并传入。
- `Challenge` 在 provider 校验之后读 `captcha_mode`：`GetString(ctx, "captcha", "captcha_mode")` 失败 → 原样上抛（503）；值不是 `modeSlide` / `modeClick` → `apierror.ErrInternal`（500，`internal_error`）。`modeClick` 走 `clickGen.generate()`，落库时 `answer_dots` 序列化 `{"dots":[...]}`、`prompt_text` 写 `PromptText`。
- `Solve` 签名改为 `Solve(ctx context.Context, id uuid.UUID, clicks []Point) (SolveChallengeResponse, error)`：查出题目后按 `answer_dots` 里 dots 长度路由——1 走 `slide.Validate(clicks[0].X, clicks[0].Y, dots[0].X, dots[0].Y, s.policies.AnswerPadding)`，3 走逐个 `click.Validate(clicks[i].X, clicks[i].Y, dots[i].X, dots[i].Y, dots[i].Width, dots[i].Height, s.policies.AnswerPadding)`，任一不符即失败。

`dto.go`：`ChallengeResponse` 加 `PromptImage string \`json:"prompt_image,omitempty"\``、`PromptText string \`json:"prompt_text,omitempty"\``、`ExpectedClicks int \`json:"expected_clicks,omitempty"\``；加 `type Point struct { X, Y int }`。

**父包同步**：`captcha/service.go` 的 `Challenge` / `Solve` 委托签名跟着改（`Solve` 收 `[]builtin.Point`），`dispatch_test.go` 里 `service.Solve(ctx, challenge.ID, x, y)` 的调用点改为 `[]builtin.Point{{X: x, Y: y}}`；`app/captcha_test.go` 的同形调用点一并改。`dispatch_test.go` 的 `challengeAnswer` 返回 `[]answerDot` 后，那里的 `x, y := challengeAnswer(...)` 也要改成取 `dots[0]`。

Run: `cd backend && export PATH="/f/software/go/bin:$PATH" && source .env && go test ./internal/modules/captcha/builtin/ -count=1`
Expected: PASS（既有滑块用例因 helper 签名变化已同步更新）。

- [ ] **Step 6: 提交**

```bash
git add backend/internal/modules/settings/ backend/migrations/ backend/internal/modules/captcha/builtin/
git commit -m "feat(captcha): 新增 captcha_mode 设置与形态分派"
```

---

### Task 4: 点选判分与顺序语义

**Files:**
- Modify: `backend/internal/modules/captcha/builtin/service.go`
- Modify: `backend/internal/modules/captcha/builtin/dto.go`
- Test: `backend/internal/modules/captcha/builtin/service_test.go`

**Interfaces:**
- Consumes: `Service.Solve(ctx, id, []Point)`、`answerDot`、`modeClick`（Task 3）。
- Produces: 完整的点选判分行为（顺序敏感、错一个即作废、并发只一次成功）。

- [ ] **Step 1: 写失败的测试**

`service_test.go` 加（`setCaptchaSettings` 已带 mode 参数）：

```go
func TestClickSolveInOrder(t *testing.T)          // 按序交 3 个正确坐标 → 200 + 32 字节 token
func TestClickSolveRejectsShuffledOrder(t *testing.T) // 集合对、顺序打乱 → captcha_failed，再用正确顺序交仍 captcha_failed
func TestClickSolveRejectsOneWrong(t *testing.T)  // 3 个里错 1 个 → captcha_failed，再交正确坐标仍 captcha_failed
func TestClickSolveRejectsReplay(t *testing.T)    // 成功后同题再交一次 → captcha_failed
func TestClickSolveExpired(t *testing.T)          // 直接改库把 expires_at 改到过去 → captcha_failed
func TestClickSolveScopeMismatch(t *testing.T)    // login 题的凭证用于 register → captcha_failed
```

每个用例都用 `challengeAnswer` 拿 `[]answerDot` 构造请求；「顺序打乱」用例必须用**集合完全相同、仅顺序不同**的坐标（按项目变异纪律：顺序类需求要造「两种顺序结果不同」的输入，用固定正确顺序永远测不出顺序）。

Run: `cd backend && export PATH="/f/software/go/bin:$PATH" && source .env && go test ./internal/modules/captcha/builtin/ -run 'TestClickSolve' -count=1`
Expected: FAIL。

- [ ] **Step 2: 实现点选判分**

`service.go` 的 `Solve` 里点选分支已由 Task 3 接通；本步补足失败路径：判分失败时先 `markFailed`（条件更新为 `status=3`），再返回 `apierror.ErrCaptchaRejected`；`markFailed` 报错上抛。确认「已判负题再交」走 `findPending` 查不到 → `ErrCaptchaRejected`。

Run: `cd backend && export PATH="/f/software/go/bin:$PATH" && source .env && go test ./internal/modules/captcha/builtin/ -run 'TestClickSolve' -count=1 -v`
Expected: PASS。

- [ ] **Step 3: 变异自检（顺序语义）**

把 `Solve` 的点选分支临时改成顺序无关（三个点击与三个答案做集合匹配），跑 `TestClickSolveRejectsShuffledOrder`：

Run: `cd backend && export PATH="/f/software/go/bin:$PATH" && source .env && go test ./internal/modules/captcha/builtin/ -run TestClickSolveRejectsShuffledOrder -count=1`
Expected: **必须 FAIL**。通过后立刻还原代码，再跑一次确认 PASS。

- [ ] **Step 4: 提交**

```bash
git add backend/internal/modules/captcha/builtin/
git commit -m "feat(captcha): 实现点选按序判分与作废语义"
```

---

### Task 5: 交卷端点的形状校验与形态路由

**Files:**
- Modify: `backend/internal/modules/captcha/builtin/controller.go`
- Modify: `backend/internal/modules/captcha/builtin/dto.go`
- Test: `backend/internal/modules/captcha/builtin/controller_test.go`

**Interfaces:**
- Consumes: `Service.Solve(ctx, id, []Point)`、`Point`（Task 3）。
- Produces: 交卷路由对两种请求形状的解析与校验；`clicks` 数量恒 3、元素非负整数、不得混带。

- [ ] **Step 1: 写失败的测试**

`controller_test.go` 加：

```go
func TestClickSolveRouteRequiresThreeClicks(t *testing.T)      // 2 个 / 4 个元素 → 400 invalid_request param "clicks"，随后按正确坐标交卷仍 200
func TestClickSolveRouteRejectsMalformedClicks(t *testing.T)   // 元素缺 x / 负坐标 → 400 param "clicks"，题目状态不变
func TestClickSolveRouteRejectsMixedShapes(t *testing.T)       // 同时带 x 与 clicks → 400
func TestClickSolveRouteWrongShapeForSlideChallenge(t *testing.T) // 滑块题交 clicks → 400
func TestClickSolveRouteWrongShapeForClickChallenge(t *testing.T)  // 点选题交 x/y → 400
func TestChallengeRouteClickShape(t *testing.T)                // mode=click 出题响应带 prompt_image/prompt_text/expected_clicks=3、无 tile_image
```

helper `createChallengeViaRoute` 增 `mode` 参数。

Run: `cd backend && export PATH="/f/software/go/bin:$PATH" && source .env && go test ./internal/modules/captcha/builtin/ -run 'TestClickSolveRoute|TestChallengeRouteClick' -count=1`
Expected: FAIL。

- [ ] **Step 2: 实现路由解析与校验**

`controller.go` 的 `solveChallenge`：解析 `id` 后先 `ShouldBindJSON` 到扩展后的 `SolveChallengeRequest`（含 `X *int` / `Y *int` / `Clicks []Point`）。用 `json.RawMessage` 或双字段判形状：`Clicks == nil && X == nil` → `invalid_request`（param `x`）；两者都非 nil → `invalid_request`（param 指到 `clicks`）。`Clicks` 非 nil 时校验 `len(Clicks) == 3`（否则 param `clicks`）、每个元素 `X >= 0 && Y >= 0`（否则 param `clicks`）——**这一层只做形状校验，不查库、不碰题目状态**；通过后调 `service.Solve(ctx, id, clicks)`，由 service 按题目自身形状判「形态相符」（点选题收到 1 个点、滑块题收到 3 个点都返回 `ErrCaptchaRejected`）。

Run: `cd backend && export PATH="/f/software/go/bin:$PATH" && source .env && go test ./internal/modules/captcha/builtin/ -run 'TestClickSolveRoute|TestChallengeRouteClick' -count=1 -v`
Expected: PASS。

- [ ] **Step 3: 跑 builtin 包全量 + 父包**

Run: `cd backend && export PATH="/f/software/go/bin:$PATH" && source .env && go test -p 1 ./internal/modules/captcha/... -count=1`
Expected: PASS（root / builtin / policy / turnstile 四包）。

- [ ] **Step 4: 提交**

```bash
git add backend/internal/modules/captcha/builtin/
git commit -m "feat(captcha): 交卷端点支持 clicks 形状校验"
```

---

### Task 6: 端到端、缺行/坏值与形态切换

**Files:**
- Modify: `backend/internal/app/captcha_test.go`
- Modify: `backend/internal/modules/captcha/dispatch_test.go`

**Interfaces:**
- Consumes: 全部前述任务的公开行为。
- Produces: 端到端与边界用例：点选登录全流程、`captcha_mode` 缺行 503、坏值 500、滑块链路不回归。

- [ ] **Step 1: 写失败的测试**

`app/captcha_test.go` 加 `TestBuiltinClickCaptchaLoginFlow`：设置 `captcha_enabled=true` / `captcha_provider=builtin` / `captcha_mode=click` → 出题（201，带 `prompt_image`）→ 从库里读 `answer_dots` 构造 3 个坐标 → 交卷拿 token → 登录 200 → 同 token 再登录 400 `captcha_failed`。

`dispatch_test.go` 加：

```go
func TestChallengeFailsWhenModeRowMissing(t *testing.T)   // 删掉 captcha_mode 行 → 出题 503 service_unavailable
func TestChallengeFailsWhenModeValueInvalid(t *testing.T) // 直接写坏值 'foo' → 出题 500 internal_error
func TestChallengeSlideUnchanged(t *testing.T)            // mode=slide 时响应字段与现在逐字一致
```

- [ ] **Step 2: 运行确认失败**

Run: `cd backend && export PATH="/f/software/go/bin:$PATH" && source .env && go test -p 1 ./internal/app/ ./internal/modules/captcha/ -run 'Click|ModeRow|ModeValue|SlideUnchanged' -count=1`
Expected: FAIL。

- [ ] **Step 3: 补齐行为并跑绿**

按需修 `service.go` / `dispatch_test.go` 的装配（坏值分支若走了 `settings.PatchGroup` 的校验而写不进坏值，测试改用直连 `db.Client.SettingValue` 写坏行，制造「库里被写坏」的真实状态）。

Run: `cd backend && export PATH="/f/software/go/bin:$PATH" && source .env && go test -p 1 ./internal/app/ ./internal/modules/captcha/... -count=1`
Expected: PASS。

- [ ] **Step 4: 提交**

```bash
git add backend/internal/app/ backend/internal/modules/captcha/
git commit -m "test(captcha): 点选端到端与 mode 缺行坏值用例"
```

---

### Task 7: 变异自检、素材实测与文档

**Files:**
- Modify: `backend/README.md`
- Modify: `docs/DEVLOG.md`
- Modify: `docs/superpowers/specs/2026-10-26-neoblog-builtin-captcha-click-design.md`（§8 回填实测数字）

**Interfaces:**
- Consumes: 无。
- Produces: 变异结论、实测数字、文档更新。

- [ ] **Step 1: 三条变异自检**

每条改坏 → 跑指定用例确认**变红** → 还原 → 确认恢复绿：

| 变异 | 用例 | 期望 |
| --- | --- | --- |
| 点选判分改成顺序无关（集合匹配） | `TestClickSolveRejectsShuffledOrder` | 红 |
| 去掉 `len(Clicks) == 3` 校验（改成 `>= 1`） | `TestClickSolveRouteRequiresThreeClicks` | 红 |
| `Solve` 里删掉形态路由（所有题走 `slide.Validate`） | `TestClickSolveInOrder` | 红 |

第四条（`captcha_mode` 缺行默认当 `slide`）：把缺行分支改成 `return modeSlide, nil`，跑 `TestChallengeFailsWhenModeRowMissing` 必须红；还原。

Run: `cd backend && export PATH="/f/software/go/bin:$PATH" && source .env && go test ./internal/modules/captcha/builtin/ ./internal/modules/captcha/ -count=1`
Expected: 每条变异后红、还原后绿。

- [ ] **Step 2: 点选素材与出题实测**

写一个 `.scratch` 探针（或临时测试）测：点选生成器加载后的常驻内存增量、单次 `generate()` 平均/最大耗时、主图 JPEG 与提示图 PNG 的 base64 后字节数、启动期额外解码耗时。把数字回填 spec §8 与 DEVLOG。

Expected: 得到四组数字；确认提示图 PNG 体积可接受（量级参考：滑块主图约 41KB）。

- [ ] **Step 3: 更新 README**

`backend/README.md`：

- API 表：`POST /api/v1/captcha/challenges` 与 `.../solutions` 两行补点选形态的字段差异（`prompt_image` / `prompt_text` / `expected_clicks`、`clicks` 恰好 3、按序判分）。
- 模块表 `captcha` 行：补「按 `captcha_mode`（`slide` / `click`）分派两种形态，答案统一存 `answer_dots`（jsonb）」。
- 迁移清单：加 `captcha_challenge_answer_dots` 与 `seed_captcha_mode` 两条，前者注明清空存量行。
- 设置章节：`captcha` 组从四项改五项，写清 `captcha_mode` 取值、默认 `slide`、`Public`、只认两值否则 409；把「点选未做」从缺口表述里删掉。

- [ ] **Step 4: 写 DEVLOG 条目**

`docs/DEVLOG.md` 顶部（倒序）加本轮条目：范围、动机（用户要「图像点击」即中文字点选）、决策（答案统一 jsonb 并删旧列、顺序硬语义、固定 3 字、缺行不兜底）、实测数字、已知缺口（图形点选 / 旋转 / 拖拽、难度设置、前端未接、管理端 builtin 自检、provider 白名单）。**不写版本号。**

- [ ] **Step 5: 全量验证**

Run: `cd backend && export PATH="/f/software/go/bin:$PATH" && source .env && bash scripts/check.sh && bash scripts/test.sh`
Expected: `check ok`；`test.sh` exit 0。

- [ ] **Step 6: 汇报，不提交**

把改动清单、验证输出、变异结论摆给用户，等用户决定是否提交（用户口径：绝不主动提交）。

---

## Self-Review 记录

**1. Spec coverage：**

| spec 章节 | 落在哪个任务 |
| --- | --- |
| §1.1 / §1.2 范围 | Global Constraints + 各任务边界 |
| §2 生效条件（多读 `captcha_mode`，不影响 `Verify`） | Task 3 Step 5（只在 `Challenge` 读）、Task 6 |
| §3.1 出题协议与字段 | Task 3 Step 4/5、Task 5 Step 1（`TestChallengeRouteClickShape`） |
| §3.2 交卷协议与形状校验 | Task 5 |
| §3.3 错误码（不新增） | Task 3（500 哨兵）、Task 5、Task 6 |
| §4.1 / §4.2 表结构与 jsonb 形状 | Task 2、Task 1（`answerDot`） |
| §4.3 状态机与清理（不改） | Task 4 复用，无新步骤 |
| §4.4 存量清空 | Task 2 Step 2 |
| §5.1 出题分派 | Task 3 Step 5 |
| §5.2 交卷判分（顺序、作废） | Task 4 |
| §5.3 `Verify` 零改动 | 无任务改动即覆盖；Task 6 端到端隐式验证 |
| §6 限流（不改） | 无任务改动即覆盖 |
| §7.1 / §7.2 / §7.3 模块与依赖 | Task 1/2/3/5 |
| §8 素材实测 | Task 7 Step 2 |
| §9 上线顺序（不得切 click） | Task 7 Step 4（DEVLOG 记明） |
| §10 测试计划 | Task 3/4/5/6/7 |
| §11 文档 | Task 7 Step 3/4 |
| §12 / §13 代价与缺口 | Task 7 Step 4 |

无遗漏。

**2. Step scan：** 每个步骤只给签名、测试名与断言、验证命令与期望输出；函数体只在「签名与测试定不了」处出现（如 `WithRangeVerifyLen` 的具体取值、seed SQL 的 `ON CONFLICT` 子句）。

**3. Type consistency：** `answerDot`（Task 1 建）→ Task 2/3/4 一致使用；`Point`（Task 3 建）→ Task 4/5 一致使用；`Service.Solve(ctx, id, []Point)` 在 Task 3 定型，Task 4/5 不再改签名；`setCaptchaSettings` / `createChallengeViaRoute` / `challengeAnswer` 三个 helper 的签名变化在 Task 3 / Task 5 显式要求同步所有既有调用点。

**4. Review Focus：** 五条各有归属——顺序打乱（Task 4 Step 1/3）、形状非法不判负（Task 5 Step 1）、形态路由（Task 3 Step 4 + Task 7 Step 1 变异三）、存量清空（Task 2 Step 4 验证 count=0）、mode 缺行 503（Task 6 Step 1 + Task 7 Step 1 变异四）。

**5. Proportion：** 计划以签名、测试名、命令与期望为主，代码块仅 5 处且都是接口定型或 SQL 定稿，未写成程序转录。
