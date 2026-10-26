# 自研人机验证新增「点选文字」形态 后端设计

日期：2026-10-26
状态：待审阅
前序：2026-10-26 自研人机验证（go-captcha 滑块）后端接入（`captcha_provider=builtin` 已入库，0.24.0）与前端接入（滑块组件已入库）；2026-10-26 回退「启用范围」（0.24.2，当前 HEAD `3dd55f5`）

## 1. 背景与目标

`builtin` provider 目前只有滑块（slide）一种形态。本轮新增第二种形态：**点选文字**（click）——大图背景上散落若干中文字，后端随机指定其中固定 3 个为答案并按序编号，提示图告知用户「请依次点击：验 证 码」，用户按顺序点 3 下后一次性交卷，后端按提交顺序逐位判分。

形态通过设置项 `captcha_mode`（`slide` / `click`）在管理端选择，`captcha_provider` 仍是 `builtin`。两种形态共用同一套挑战表、状态机、一次性凭证与限流。

范围**仅后端**：前端登录页、注册页、控制台设置页本轮零改动（现切到 `click` 会拦死前台，见 §9）。

**协议由本项目自己定**（延续滑块轮的既定口径）；开源库 `github.com/wenlng/go-captcha/v2` 的 `click` 子包只作引擎，不 fork。

### 1.1 本轮做

- `click` 形态的出题与判分：3 个目标字、按序判分、恰好 3 次点击。
- `captcha_mode` 设置项（文字、默认 `slide`、`Public`）+ 组校验（只认 `slide` / `click`）+ seed 迁移。
- 挑战表答案列统一改为 `answer_dots`（jsonb）：滑块答案也搬进去，`answer_x` / `answer_y` 删除。
- 出题 / 交卷两个端点的协议按形态分叉；错误码沿用既有，不新增。
- 点选判分、顺序敏感、数量校验、形态不匹配等测试；滑块链路回归。
- README / DEVLOG / 版本号。

### 1.2 明确不做

- 图形点选（`click.ModeShape`）、旋转、自由拖拽三种形态；难度、题量设置项（目标字数固定 3，是代码常量，不做设置）。
- 前端任何改动（点选组件、登录 / 注册页接线、控制台「验证方式」下拉）。
- 管理端自检接口的 builtin 形态（`POST /admin/captcha/verifications` 继续只服务 Turnstile）。
- 凭证绑定客户端 IP；`captcha_provider` 取值白名单（既有缺口，不扩大也不修）。
- 后台定时清理任务（过期行仍在出题时顺带清理）。
- 打乱提交顺序也能通过（顺序是本形态的核心语义，见 §5.2）。

## 2. 生效条件

在滑块轮既定条件之上多读一个 `captcha_mode`：

1. `captcha.captcha_enabled = true`；
2. `captcha.captcha_provider = "builtin"`；
3. `captcha.captcha_mode = "click"`（本形态专属；`slide` 时出题走滑块）；
4. 请求携带的 `captcha_token` 有效（凭证存在、未过期、未使用、scope 相符）。

条件 1～2 不满足时 `Verify` 放行、出题返回 `403 captcha_disabled`（既有语义不变）。**`captcha_mode` 只影响出题与判分，不影响 `Verify`**：凭证一旦签发，登录 / 注册侧只认凭证本身，不关心它是哪种形态换来的。条件 3 的读取失败（缺行、坏值）直接上抛，按项目「读取不做兜底」口径处理，不放行、不默认当 `slide`。

## 3. 协议

两个端点均为公开（免登录），沿用 captcha 包已有的 `requireJSONRequest` 守卫（Content-Type 必须 `application/json`，Origin 存在时必须与 Host 同源）。

### 3.1 出题

`POST /api/v1/captcha/challenges`

请求不变：

```json
{"scope": "login"}
```

- `scope` 必填，只接受 `login` / `register`；其它值 `400 invalid_request`（`param: "scope"`）。
- 未启用 / provider 不是 `builtin` → `403 captcha_disabled`。
- 超限 → `429 rate_limited`，响应带 `Retry-After`。
- `captcha_mode` 读失败 → `503 service_unavailable`；读到 `slide` / `click` 之外的值 → `500 internal_error`（数据被写坏，不是请求问题）。

**`captcha_mode = "click"` 时响应 `201`**：

```json
{
  "id": "0192f0c4-...",
  "master_image": "data:image/jpeg;base64,...",
  "prompt_image": "data:image/png;base64,...",
  "prompt_text": "验 证 码",
  "master_width": 300,
  "master_height": 220,
  "expected_clicks": 3,
  "expires_at": "2026-10-26T12:03:00Z"
}
```

| 字段 | 说明 |
| --- | --- |
| `id` | 题目 ID（UUID v7），交卷路径参数 |
| `master_image` | 主图 JPEG data URL（300×220），字散布其上 |
| `prompt_image` | 提示图 PNG data URL（150×40），画出要点的 3 个字 |
| `prompt_text` | 提示文字，与 `prompt_image` 同内容，单空格分隔（便于无障碍与兜底显示） |
| `master_width` / `master_height` | 主图尺寸（库默认 300×220） |
| `expected_clicks` | 期望点击次数，恒为 `3` |
| `expires_at` | 题目过期时间（RFC3339 UTC） |

**不下发答案**：主图上每个字的位置、目标字是哪几个，都不在响应里。前端只渲染 `master_image` 与 `prompt_image`，用户点哪由人看，不由接口告知。

`captcha_mode = "slide"` 时响应与现在完全一致（`tile_image` / `tile_x` / `tile_y` 等字段），**两种形态的响应字段不重叠**：滑块不出 `prompt_image` / `prompt_text` / `expected_clicks`，点选不出 `tile_*`。前端按 mode 知道该读哪些。

### 3.2 交卷

`POST /api/v1/captcha/challenges/:id/solutions`

**滑块**（`captcha_mode` 与该题形态无关，按题目自身形状判）：

```json
{"x": 123, "y": 45}
```

- `id` 不是合法 UUID → `400 invalid_request`（`param: "id"`）。
- `x` / `y` 缺失、非整数或为负 → `400 invalid_request`（`param: "x"` / `"y"`）。

**点选**：

```json
{"clicks": [{"x": 10, "y": 50}, {"x": 92, "y": 80}, {"x": 170, "y": 110}]}
```

- `clicks` 缺失 → `400 invalid_request`（`param: "clicks"`）。
- 元素个数不等于 3 → `400 invalid_request`（`param: "clicks"`），**不改变题目状态**（仍是待解，可用正确坐标重交）。
- 元素缺失 `x` / `y`、非整数或为负 → `400 invalid_request`（`param: "clicks"`），同样不改变题目状态。
- 同时带 `x` 与 `clicks`，或所带形状与该题形态不符 → `400 invalid_request`（`param` 指到多余的那个字段）。

**判定结果**（两种形态一致）：

- 题目不存在 / 已过期 / 已判负 / 已交过卷 → `400 captcha_failed`（`param: null`，不区分原因，不泄露题目存在性）。
- 答案错（滑块超容差、点选任一位不对）→ `400 captcha_failed`（`param: null`），同时该题标记为判负，不可重试。
- 成功 `200`：

```json
{"captcha_token": "...", "expires_at": "2026-10-26T12:05:00Z"}
```

`captcha_token` 就是登录 / 注册请求体里现有的那个字段名，两种形态签发的凭证同构，登录 / 注册接口零改动。

### 3.3 错误码

沿用滑块轮既有映射，**不新增错误码、不改既有码义**：

| 场景 | 状态 | code | param |
| --- | --- | --- | --- |
| 请求体非法、scope 未知、坐标非法、id 非 UUID、clicks 数量或元素非法 | 400 | `invalid_request` | 对应字段名 |
| 题目不存在 / 过期 / 已用 / 答案错 / 顺序错 | 400 | `captcha_failed` | `null` |
| 未启用 / provider 非 builtin | 403 | `captcha_disabled` | `null` |
| 限流 | 429 | `rate_limited` | `null`（带 `Retry-After`） |
| 数据库不可用、`captcha_mode` 读失败 | 503 | `service_unavailable` | `null` |
| `captcha_mode` 值非法 | 500 | `internal_error` | `null` |
| 图片生成 / 编码失败 | 500 | `internal_error` | `null` |

## 4. 数据模型

### 4.1 表 `captcha_challenges`

| 列 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | uuid | 主键，默认 UUID v7 | 题目 ID |
| `scope` | varchar(16) | 非空 | `login` / `register` |
| `answer_dots` | jsonb | 非空 | 唯一答案列，两种形态共用，形状见 §4.2 |
| `prompt_text` | text | 非空 | 点选的提示文字；滑块为空串 |
| `status` | int | 非空，默认 0 | 0 待解 / 1 已解 / 2 已消费 / 3 判负 |
| `token_hash` | varchar(64) | 可空、唯一 | sha256 hex；交卷成功时写入 |
| `created_at` | timestamptz | 非空，默认当前时间 | |
| `expires_at` | timestamptz | 非空，建索引 | 题目 3 分钟；交卷成功改为当前时间 + 5 分钟（凭证有效期） |

改动：`answer_x` / `answer_y` **删除**，`answer_dots` / `prompt_text` **新增**。索引与唯一约束不变（`expires_at` 索引、`token_hash` 唯一）。

**为什么删而不是留着**：留着会让「哪种形态该读哪列」变成仓库层的隐式判断。删掉后两种形态走同一条判分路径（读 `answer_dots` → 逐个比对），滑块只是 `dots` 长度为 1 的特例。

### 4.2 `answer_dots` 形状

**滑块**（数组长度恒为 1）：

```json
{"dots": [{"x": 123, "y": 45, "width": 64, "height": 64}]}
```

`x` / `y` 是库的正确答案坐标（`block.X` / `block.Y`，非可外传的 `DX` / `DY`）；`width` / `height` 是拼图块尺寸，只作记录，判分用 `slide.Validate` 与固定容差。

**点选**（数组长度恒为 3）：

```json
{
  "dots": [
    {"text": "验", "x": 10, "y": 50, "width": 30, "height": 32},
    {"text": "证", "x": 92, "y": 80, "width": 28, "height": 30},
    {"text": "码", "x": 170, "y": 110, "width": 31, "height": 32}
  ]
}
```

- `text` 是该字本身，只作记录与排查，判分不用（判分用坐标）。
- `x` / `y` 是该字的左上角，`width` / `height` 是其包围盒尺寸，来自库的 `Dot` 字段。
- **数组顺序就是要求用户点击的顺序**：第 `i` 次提交的点击比对 `dots[i]`。

### 4.3 状态机与过期清理

与滑块轮逐字一致，不改：条件更新落状态（`WHERE id = ? AND status = 0 AND expires_at > now()`，影响行数为 0 即判负 / 过期 / 已交卷）；消费条件更新（`WHERE token_hash = ? AND status = 1 AND expires_at > now() AND scope = ?`）；读取即校验过期，不依赖清理是否跑过；每次出题前 `DELETE ... WHERE expires_at < now()`，清理失败上抛不吞。

### 4.4 存量数据

`captcha_challenges` 现存行全部作废（上一轮 DEVLOG 记录 dev / test 库各有一批 10-25 遗留的一次性测试数据，且新表自 0.24.0 上线后产生的滑块行答案形状也随本轮改变）。迁移必须 `DELETE FROM captcha_challenges` 清空存量：这些行的 `answer_x` / `answer_y` 在新列里不存在，留着既判不了分又会让人误判。清空的是**挑战表**（一次性、分钟级过期），不涉及任何用户数据。

## 5. 判定与凭证

### 5.1 出题（service）

1. 校验 scope（只认 `login` / `register`）→ 限流（见 §6）。
2. 读 `captcha_enabled` / `captcha_provider`；不满足 → `ErrCaptchaDisabled`。读失败直接上抛（`503`），不出题、不放行。
3. 读 `captcha_mode`：`slide` 走滑块生成器，`click` 走点选生成器；读失败上抛（`503`）；值既非 `slide` 也非 `click` → `internal_error`（500）。
4. 调对应 generator 生成：主图 data URL、尺寸，以及滑块的拼图块 / 初始位置或点选的提示图 / 提示文字 / 3 个目标字答案；生成或编码失败 → `internal_error`。
5. 清理过期行 → 落库（`status=0`、`answer_dots`、`prompt_text`、`expires_at=now+3min`）→ 返回 §3.1 对应形态的响应。

### 5.2 交卷（service）

1. 解析 `id`；按 §3.2 校验请求体形状（数量、元素、形态相符）。形状非法 → `invalid_request`，**不查库、不改题目状态**。
2. 查询题目（`status=0` 且未过期），查不到 → `captcha_failed`；查询本身报数据库错误 → 上抛。
3. 判分：
   - 滑块：`slide.Validate(x, y, dot.X, dot.Y, padding=4)`（`dots` 长度 1）。
   - 点选：对 `i ∈ [0,3)` 逐个 `click.Validate(clicks[i].X, clicks[i].Y, dots[i].X, dots[i].Y, dots[i].Width, dots[i].Height, padding=4)`。**任一位不符即整体失败**；三个坐标集合正确但顺序打乱同样失败（第 `i` 位比对的是第 `i` 个答案）。
4. 通过：生成 32 字节随机凭证（base64url），`token_hash = sha256 hex`，条件更新为已解（`status=1`、`expires_at=now+5min`）；更新影响行数为 0（并发交卷 / 恰好过期）→ `captcha_failed`；更新报数据库错误 → 上抛。返回 `captcha_token` 与过期时间。
5. 失败：条件更新为判负（`status=3`）；更新成功返回 `captcha_failed`，更新报数据库错误则上抛（不吞错、不留下可重试的待解题）。
6. 交卷不读设置：题目存在即按题目自身形状判分；provider / mode 在出题后切换不影响已发题目。

### 5.3 登录 / 注册消费（`Service.Verify`）

签名与分派逻辑**零改动**：仍只读 `captcha_enabled` / `captcha_provider`，`builtin` 分支原子消费凭证。点选与滑块签发的凭证同构、同 TTL、同一次性语义，`Verify` 不感知形态。

## 6. 限流

与滑块轮逐字一致，不改：出题每 IP **5 次 / 10 分钟**（key `captcha-challenges:ip:*`）；验证失败两种 provider 共用 IP 桶 **15 次 / 10 分钟**（key `captcha-fail:ip:*`，窗口与冷却均 10 分钟），builtin 在空 token 或凭证消费失败时计数；交卷不单独限流（单题只能交一次，失败即判负）。点选与滑块共用这两道限流，不因形态分桶。

## 7. 模块与代码改动

### 7.1 `internal/modules/captcha/builtin`

| 文件 | 改动 |
| --- | --- |
| `generator.go` | 拆成两个生成器：滑块生成器（现有代码，`imagesv2` + `tiles` 素材）与点选生成器（新增，`imagesv2` 背景 + `fzshengsksjw` TTF 字体 + 库内置 15 中文字库）。两者都在启动期加载素材，失败即返回错误（`app.New` 失败，不降级）。点选 `generate()` 返回主图 data URL、提示图 data URL、提示文字与 3 个目标字的答案。 |
| `repository.go` | `create` 写 `answer_dots`（jsonb）与 `prompt_text`；`findPending` / `markSolved` / `markFailed` / `consume` / `deleteExpired` 的谓词不变。答案的编解码（Go 结构 ↔ jsonb）在此层完成，不 import `apierror`。 |
| `service.go` | `Challenge` 读 `captcha_mode` 后分派到两个生成器；`Solve` 改为接收 `clicks []Point`（滑块由控制器把单 `{x,y}` 归一成 1 元素切片），按题目 `answer_dots` 里 `dots` 的长度走滑块或点选判分。`Policies` / `DefaultPolicies` 增补点选相关常量（目标字数 3、点选容差），默认值不变。 |
| `dto.go` | 新增 `Point`、`ClickSolveRequest`（`clicks []Point`）、点选出题响应字段；`SolveChallengeRequest` 容纳 `x` / `y` / `clicks` 两种形状；挑战状态常量不变。 |
| `controller.go` | 出题路由不变；交卷路由按题目形态分别解析 `x`/`y` 或 `clicks`，并做 §3.2 的形状校验（数量恒 3、元素非负整数、不得同时带两种形状）。 |
| `service_test.go` 等 | 见 §10。 |

### 7.2 其他

- `internal/database/ent/schema/captcha_challenge.go`：删 `answer_x` / `answer_y`，加 `answer_dots`（`field.JSON`，非空）与 `prompt_text`（`field.String`，非空，默认空串）；`scripts/db-generate.sh` 重新生成。
- `migrations/<时间戳>_captcha_challenge_answer_dots.sql`（新，atlas diff 生成 + 人工审阅）：改列 + 清空存量行；含手工改造 SQL，跑完必须 `atlas migrate hash` + `atlas migrate validate`。
- `internal/modules/settings/registry.go`：`captcha` 组加 `captcha_mode`（文字、默认 `slide`、`Public`）与 `ValidateOn: ["captcha_mode"]` + `validateCaptchaGroup`（只认 `slide` / `click`，否则 `setting_conflict` 409）。
- `migrations/<时间戳>_seed_captcha_mode.sql`（新，`*_seed_*.sql` 命名）：种 `captcha.captcha_mode` 默认行 `slide`，`ON CONFLICT DO NOTHING`。加设置的三处同步（清单 / seed / atlas hash）照既有配方执行。
- `internal/app/app.go`：装配不变（点选生成器在 `builtin.New` 内构造，素材加载失败照旧让启动失败）。
- 零新依赖：`click` 子包、字体与素材都来自已引入的 `go-captcha/v2 v2.0.5` 与 `go-captcha-assets v1.0.7`。

### 7.3 依赖方向

`controller → service → {slideGenerator, clickGenerator, repository}`；`service → settings（窄接口）`。两个生成器互不 import，都只依赖 go-captcha 库与标准库。

## 8. 素材与开销（点选新增部分）

点选生成器新增加载：TTF 字体 1 个（`fzshengsksjw`，约数百 KB）+ 复用滑块轮已加载的 `imagesv2` 背景 16 张。素材全部来自官方 `go-captcha-assets`，不改图、不自制。

**实测回填（本机，2026-10-26）**：

| 指标 | 数值 |
| --- | --- |
| `imagesv2.GetImages()` | +6.4 MB，单次约 51ms（**不缓存，每次调用重新解码**；滑块与点选各调一次，启动多付一次） |
| `fzshengsksjw.GetFont()` | +8.1 MB，约 37ms |
| 两个 builder（`Make()`） | 合计 < 1 MB |
| 点选净增常驻内存 | 约 14.5 MB |
| 点选净增启动耗时 | 约 88ms（背景二次解码 + 字体） |
| `click.Generate()` | 平均 7.5ms / 最大 10.1ms |
| 主图 JPEG（base64 data URL） | ≤ 88.6 KB |
| 提示图 PNG（base64 data URL） | ≤ 5.1 KB |
| 单题响应合计 | 平均 76.3 KB |
| 对照：滑块 `Generate()` / 主图 | 平均 7.7ms / ≤ 89.3 KB |

## 9. 上线顺序

- 本轮交付后 `captcha_mode` 保持 `slide`，**不得切成 `click`**：前端没有点选组件，切过去登录 / 注册会被 `captcha_failed` 拦住。
- 切换时机在点选前端接线完成之后（形态选择器要做成管理端「自研」之下的二选一）。

## 10. 测试与验证计划

### 10.1 单元 / 集成

- generator（点选）：真实素材出题，断言主图 / 提示图非空、data URL 前缀正确（JPEG / PNG）、`expected_clicks` 为 3、答案 `dots` 长度为 3、每个 dot 的 `x` / `y` / `width` / `height` 为正且落在主图范围内、3 个目标字互不重复、提示文字与 3 个 dot 的 `text` 一致。
- 判分（service，测试库）：
  - 按正确顺序交 3 个正确坐标 → 拿到 token，且 token 是 32 字节 base64url。
  - **三个坐标集合正确但顺序打乱 → `captcha_failed`**（本轮最核心的新语义）。
  - 3 个坐标里错 1 个 → `captcha_failed`，随后用完全正确的坐标交同一题仍 `captcha_failed`（已判负不可重试）。
  - 同一题连续两次交卷（都对）→ 第二次 `captcha_failed`（一次性）。
  - 题目过期 / 凭证过期 → `captcha_failed`（直接改库时间）。
  - `login` 凭证用于 `register` scope → `captcha_failed`；凭证二次消费 → `captcha_failed`。
- 形状校验（controller / service）：
  - `clicks` 缺失、2 个元素、4 个元素 → `400 invalid_request`（`param: "clicks"`），且**题目仍可用正确坐标交卷成功**（证明没被误判负）。
  - 元素缺 `x` / `y`、坐标为负 → `400 invalid_request`，题目状态同样不变。
  - 滑块题交 `clicks`、点选题交 `x`/`y`、同时带两种 → `400 invalid_request`。
- 形态切换：
  - `captcha_mode=slide` 时出题响应不带点选字段、交卷走 `x`/`y`，滑块既有用例全部保持。
  - `captcha_mode=click` 时出题响应带 `prompt_image` / `prompt_text` / `expected_clicks`，不带 `tile_*`。
  - `captcha_mode` 缺行 / 值为 `foo` → 出题失败（503 / 500），不放行。
- 设置：`captcha_mode` 写 `slide` / `click` → 200；写 `foo` / 空串 → 409 `setting_conflict`；seed 对账测试同步。
- 端到端：`captcha_mode=click` 走「出题 → 按序交卷 → 用凭证登录成功」，再用同 token 登录 400 `captcha_failed`。
- 迁移：dev 库与测试库应用后列、索引、唯一约束正确；存量行被清空。
- 变异自检（至少三条，必须让「仅点选路径」的用例裁决）：
  - 判分改成顺序无关（集合匹配）→ 顺序打乱用例必须变红。
  - 去掉 `expected_clicks` 数量校验（改成 `>= 1` 就收）→ 4 元素用例必须变红。
  - 去掉形态判断（点选题也走 `slide.Validate`）→ 点选流程用例必须变红。
  - 删掉 `answer_dots` 非空约束或让滑块继续写 `answer_x` → 编译 / 迁移 / 用例必须红。
- `scripts/check.sh` → `check ok`；`scripts/test.sh` 全量绿。

### 10.2 手工核对（用户执行）

- 用 curl 出题两次，肉眼比对两次的目标字与位置不同（随机性）。
- 按提示图顺序点 3 下，确认拿到 token；故意打乱顺序，确认 400。
- 核对完把 `captcha_mode` 切回 `slide`。

## 11. 文档与版本

- `backend/README.md`：API 表补点选形态的出题 / 交卷字段差异；模块表 `captcha` 行补「按 `captcha_mode` 分派两种形态、答案统一存 `answer_dots`」；迁移清单加新迁移两条；设置章节 `captcha` 组从四项改五项并说明 `captcha_mode` 取值与校验；删掉「点选未做」的缺口表述。
- `docs/DEVLOG.md`：本轮条目（动机、决策：答案统一 jsonb 与删除旧列、顺序语义、固定 3 字、实测数字、已知缺口）。
- 版本：`0.24.2 → 0.24.3`（patch：新增形态与一个设置键，无破坏性接口变更）。

## 12. 已接受的代价与风险

- **答案列形状变更**：`answer_x` / `answer_y` 删除后，`captcha_challenges` 存量行全部作废（一次性、分钟级过期，无用户数据损失）；外部若有人直连该表读答案列，本轮不兼容。
- **点选限流与滑块共用**：出题桶 5 次 / 10 分钟对点选同样偏紧（点选一次交互要点 3 下，但只出一次题，不额外消耗），未按形态分别设阈值。
- **顺序语义不可配置**：固定「必须按顺序」，不接受乱序；这是本轮的核心选择，不为照顾手滑而放宽成集合匹配。
- **未启用期间的风险照旧**：前端未接点选前不能切 `click`，见 §9。
- **`captcha_mode` 读失败会拦死出题**：这是「读取不兜底」口径的必然结果——缺 seed 行时切到 `builtin` 立刻报错，而不是静默退回滑块。

## 13. 已知缺口（本轮不做，已记录）

- 图形点选（`ModeShape`）、旋转、自由拖拽三种形态。
- 难度 / 题量设置（目标字数固定 3 是代码常量）。
- 前端点选组件、登录 / 注册页接线、管理端「验证方式」下拉。
- 管理端自检接口的 builtin 形态（含点选）。
- `captcha_provider` 取值白名单（既有缺口，本轮不扩大）。
