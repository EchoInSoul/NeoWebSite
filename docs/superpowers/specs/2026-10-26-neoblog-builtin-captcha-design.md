# 自研人机验证（go-captcha 滑块）后端接入 设计

日期：2026-10-26
状态：待审阅
前序：2026-10-25 人机验证设置组归位与自检路由迁移（`captcha` 模块与设置组已就位，`builtin` 仍是「选了不拦」）

## 1. 背景与目标

项目已有 Cloudflare Turnstile 一套人机验证（`captcha_provider=turnstile`），设置组、登录/注册拦截、管理端自检接口均在运行。本轮新增第二种 provider：`captcha_provider=builtin`，底层直接使用开源库 `github.com/wenlng/go-captcha/v2`（Go 自托管，题目由后端生成、后端判分，不依赖外部服务）。

范围**仅后端**：后端独立闭环「出题 → 交卷判分 → 发一次性凭证 → 登录/注册消费凭证」。前端登录页、注册页、控制台设置页本轮零改动。Turnstile 分支行为零改动，两种 provider 通过设置切换。

**协议由本项目自己定**（官方 `go-captcha-service` 的 `id/captchaKey/value` 与 `ok/failure` 只作参考，不照搬）；开源库只作引擎，不 fork。

### 1.1 本轮做

- 滑块（slide）一种形态；出题、判分能力的后端实现。
- 两个公开端点：出题、交卷。
- 一次性凭证（token），接入现有 `captcha_token` 语义。
- `captcha_challenges` 挑战表（ent schema + atlas 迁移）。
- `caption.Service.Verify` 按 provider 分派：`turnstile` 原样、`builtin` 新增、其余原样。
- 出题端点的 IP 限流。
- README / DEVLOG / 版本号。

### 1.2 明确不做

- 旋转 / 点选 / 拖拽三种形态；难度、题量、`captcha_mode` 等设置项（本轮不加任何新设置键）。
- 管理端自检接口（`POST /admin/captcha/verifications`）的 builtin 形态；它继续只服务 Turnstile。
- 前端任何改动（滑块组件、登录/注册页接线、控制台文案）。
- 凭证绑定客户端 IP。
- `captcha_provider` 取值白名单校验（记入已知缺口：拼错的 provider 仍会静默不拦）。
- 后台定时清理任务（过期行在出题时顺带清理）。

## 2. 生效条件

`builtin` 对某个入口生效的四条件与 Turnstile 完全一致（既有语义，见 `backend/README.md` 设置章节）：

1. `captcha.captcha_enabled = true`；
2. `captcha.captcha_provider = "builtin"`；
3. 当前入口出现在 `captcha.captcha_scopes` 里（`login` / `register`）；
4. 请求携带的 `captcha_token` 有效（Turnstile 那边是「私钥非空 + siteverify 通过」，builtin 这边是「凭证存在、未过期、未使用、scope 相符」）。

任一条不满足且属于「该入口不在保护范围」时，`Verify` 放行；属于「要保护但凭证无效」时返回 `400 captcha_failed`。出题端点只在 1～3 全满足且 `scope` 合法时出题，否则 `403 captcha_disabled`。

## 3. 协议

两个端点均为公开（免登录），复用 captcha 包已有的 `requireJSONRequest` 守卫（Content-Type 必须 `application/json`，Origin 存在时必须与 Host 同源）。

### 3.1 出题

`POST /api/v1/captcha/challenges`

请求：

```json
{"scope": "login"}
```

- `scope` 必填，只接受 `login` / `register`；其它值 `400 invalid_request`（`param: "scope"`）。
- 未启用 / provider 不是 `builtin` / scope 不在 `captcha_scopes` → `403 captcha_disabled`。
- 超限 → `429 rate_limited`，响应带 `Retry-After`。

响应 `201`：

```json
{
  "id": "0192f0c4-...",
  "master_image": "data:image/jpeg;base64,...",
  "tile_image": "data:image/png;base64,...",
  "master_width": 300,
  "master_height": 220,
  "tile_width": 64,
  "tile_height": 64,
  "tile_x": 12,
  "tile_y": 130,
  "expires_at": "2026-10-26T12:03:00Z"
}
```

| 字段 | 说明 |
| --- | --- |
| `id` | 题目 ID（UUID v7），交卷路径参数 |
| `master_image` / `tile_image` | JPEG / PNG 的 data URL，前端直接塞 `<img src>` |
| `master_width` / `master_height` | 主图尺寸（库默认 300×220） |
| `tile_width` / `tile_height` | 拼图块尺寸 |
| `tile_x` / `tile_y` | 拼图块初始摆放位置（库的 display x/y） |
| `expires_at` | 题目过期时间（RFC3339 UTC） |

### 3.2 交卷

`POST /api/v1/captcha/challenges/:id/solutions`

请求：

```json
{"x": 123, "y": 45}
```

- `id` 不是合法 UUID → `400 invalid_request`（`param: "id"`）。
- `x` / `y` 缺失、非整数或为负 → `400 invalid_request`（`param: "x"` / `"y"`）。
- 题目不存在 / 已过期 / 已判负 / 已交过卷 → `400 captcha_failed`（`param: null`，不区分原因，不泄露题目存在性）。
- 答案不在容差范围内 → `400 captcha_failed`（`param: null`），同时该题标记为判负。
- 成功 `200`：

```json
{"captcha_token": "...", "expires_at": "2026-10-26T12:05:00Z"}
```

`captcha_token` 就是登录 / 注册请求体里现有的那个字段名，前端下一轮拿到后原样回传，登录 / 注册接口零改动。

### 3.3 错误码

| 场景 | 状态 | code | param |
| --- | --- | --- | --- |
| 请求体非法、scope 未知、坐标非法、id 非 UUID | 400 | `invalid_request` | 对应字段名 |
| 题目不存在 / 过期 / 已用 / 答案错 | 400 | `captcha_failed` | `null` |
| 未启用 / provider 非 builtin / scope 未勾选 | 403 | `captcha_disabled` | `null` |
| 限流 | 429 | `rate_limited` | `null`（带 `Retry-After`） |
| 数据库不可用 | 503 | `service_unavailable` | `null`（沿用集中映射） |
| 图片生成 / 编码失败 | 500 | `internal_error` | `null` |

新增两个错误常量（`internal/common/apierror`）：

- `ErrCaptchaDisabled`：`permission_error` / `captcha_disabled` / 403。
- `ErrCaptchaRejected`：`invalid_request_error` / `captcha_failed` / 400 / param 为 `nil`（与既有 `ErrCaptchaFailed` 同 code，后者 param 固定 `captcha_token`，供登录/注册使用）。`statusByCode` 已有 `captcha_failed` 条目，无需重复登记。

## 4. 数据模型

### 4.1 表 `captcha_challenges`

| 列 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | uuid | 主键，默认 UUID v7 | 题目 ID |
| `scope` | varchar(16) | 非空 | `login` / `register` |
| `answer_x` / `answer_y` | int | 非空 | 滑块正确答案，只存库，永不外发 |
| `status` | int | 非空，默认 0 | 0 待解 / 1 已解 / 2 已消费 / 3 判负 |
| `token_hash` | varchar(64) | 可空、唯一 | sha256 hex；交卷成功时写入 |
| `created_at` | timestamptz | 非空，默认当前时间 | |
| `expires_at` | timestamptz | 非空，建索引 | 题目 3 分钟；交卷成功改为当前时间 + 5 分钟（凭证有效期） |

索引：`expires_at`（对齐 `pending_registrations` 的清理方式）；`token_hash` 唯一。

### 4.2 状态机

```
待解(0) ──交卷通过──> 已解(1) ──登录/注册消费──> 已消费(2)
待解(0) ──交卷失败──> 判负(3)
```

- 交卷用条件更新落状态：`WHERE id = ? AND status = 0 AND expires_at > now()`，影响行数为 0 即判负 / 过期 / 已交卷，返回 `captcha_failed`。并发交卷只有一个成功。
- 消费用条件更新：`WHERE token_hash = ? AND status = 1 AND expires_at > now() AND scope = ?`，影响行数为 0 视为无效凭证。
- 读取即校验过期，不依赖清理是否跑过。

### 4.3 过期清理

每次出题先执行 `DELETE FROM captcha_challenges WHERE expires_at < now()`（`expires_at` 有索引），不额外起定时任务。清理失败按数据库错误上抛，不吞。

## 5. 判定与凭证

### 5.1 出题（service）

1. 读 `captcha_enabled` / `captcha_provider` / `captcha_scopes`；读失败直接上抛（`503`），不出题、不放行。
2. 三个条件与 scope 校验不通过 → `ErrCaptchaDisabled`。
3. 限流（见 §6）。
4. 调 generator 生成：主图、拼图块、尺寸、初始位置、正确答案；生成或编码失败 → `internal_error`。
5. 清理过期行 → 落库（`status=0`，`expires_at=now+3min`）→ 返回 §3.1 响应。

### 5.2 交卷（service）

1. 解析 `id`；查询题目（`status=0` 且未过期），查不到 → `captcha_failed`；查询本身报数据库错误 → 上抛。
2. `slide.Validate(x, y, answer_x, answer_y, padding=4)`。
3. 通过：生成 32 字节随机凭证（base64url），`token_hash = sha256 hex`，条件更新为已解（`status=1`、`expires_at=now+5min`）；更新影响行数为 0（并发交卷 / 恰好过期）→ `captcha_failed`；更新报数据库错误 → 上抛。返回 `captcha_token` 与过期时间。
4. 失败：条件更新为判负（`status=3`）；更新成功返回 `captcha_failed`，更新报数据库错误则上抛（不吞错、不留下可重试的待解题）。
5. 交卷不再读设置（题目存在即按题目判分）；provider 在出题后切换不影响已发题目。

### 5.3 登录 / 注册消费（`Service.Verify`）

`Verify(ctx, scope, token, ip)` 现有签名不变。分派：

- `captcha_enabled` 读失败或为 false → 放行（既有语义，与 Turnstile 相同）。
- `captcha_provider`：
  - `turnstile` → 现有判定链一行不动。
  - `builtin` → 读 `captcha_scopes`（读失败上抛）；scope 不在清单 → 放行（该入口未受保护）；`token` 为空 → `ErrCaptchaFailed`；原子消费 `WHERE token_hash=sha256(token) AND status=1 AND expires_at>now() AND scope=?`，消费不到 → `ErrCaptchaFailed`。
  - 其它值 → 放行（既有语义）。
- 凭证**一次性**：消费即置 `status=2`。凭证**只绑 scope**，不绑 IP：生产部署里后端直连对端是前端容器，所有访客的 `RemoteIP()` 相同，绑 IP 既是摆设又会误杀。
- 凭证 TTL 5 分钟，题目 TTL 3 分钟；均为常量，不做设置项。

## 6. 限流

- 只限出题端点：每 IP **60 次 / 分钟**（`ratelimit.WindowPolicy{Limit: 60, Window: time.Minute}`，key 用 `IPKey("captcha-challenges", ip)`），复用 `app.go` 现有的 `*ratelimit.Limiter` 注入。
- 交卷不单独限流：单题只能交一次（失败即判负），无止题就无从刷交卷；交卷本身只做一次主键查询 + 一次更新。
- **已知现实**：生产上后端看到的 IP 全是前端容器，该键实际是整站级配额。因此阈值按整站量级定为 60 次/分钟，不照抄邮箱验证的 30 次/15 分钟。此问题属于既有的「后端只取直连对端 IP」口径，不在本轮处理。

## 7. 模块与代码改动

### 7.1 `internal/modules/captcha`（有路由模块，按实现拆子包）

顶层只保留分派与 HTTP；两种 provider 各自成子包，互不 import：

| 文件 | 改动 |
| --- | --- |
| `builtin/generator.go`（新） | 启动时加载官方素材并构建滑块实例：`imagesv2.GetImages()` 背景 16 张 + `tiles.GetTiles()` 拼图 4 套；素材加载失败返回错误（启动失败，不降级）。`generator.generate()` 返回主图/拼图块 data URL、尺寸、初始位置与正确答案。 |
| `builtin/repository.go`（新） | 挑战表的 ent 读写：`create`、`findPending`、`markSolved`、`markFailed`、`consume`、`deleteExpired`；只返回域哨兵或原始错误，不 import `apierror`。 |
| `builtin/service.go`（新） | 自托管能力：`Challenge(ctx, scope, ip)`、`Solve(ctx, id, x, y)`、`VerifyToken(ctx, scope, token)` 与 `Policies` / `DefaultPolicies`；`New(db, settings, limiter)` / `NewWithPolicies(...)` 加载素材并装配 repository。 |
| `builtin/dto.go`（新） | 出题/交卷响应类型与挑战状态常量。 |
| `service.go` | 薄分派：`Verify` 按 provider 选 turnstile 或 builtin；`Challenge` / `Solve` 委托 `*builtin.Service`。保留两参 `NewService(settings, verifier)`（Turnstile-only，既有测试继续用）；`provider=builtin` 而 builtin 未装配时返回 `503 service_unavailable`——明确报错，不放行、不 panic。 |
| `turnstile/`（新，原 `turnstile.go` 迁入并补全） | CF 整块能力：`turnstile.go` siteverify 客户端与三态结果（`Outcome` / `Passed` / `Rejected` / `Unavailable`）、`verifier.go` 消费方接口、`service.go` 私钥/scope/凭证判定与 `VerifyConfig`、`controller.go` 管理端自检路由（`system.captcha.edit`）、`dto.go`；`New(endpoint, timeout)` 便于测试指向假服务。 |
| `verifier.go` | 只留 `type Verifier = turnstile.Verifier` 别名（外部模块测试仍在用）。 |
| `builtin/controller.go`（新） | 两条公开路由与请求编解码（`requireJSONRequest`、`clientIP`、`apierror.Write`），`NewController(service)` 供 `module.go` 装配。 |
| `builtin/dto.go`（新） | `CreateChallengeRequest`、`SolveChallengeRequest`、两个响应类型与挑战状态常量。 |
| `module.go` | `New(db, settingsService, limiter) (*Module, error)`：构造 `builtin.New`（可失败）、`turnstile.NewService`，装配分派器与两个 controller；`RegisterProtected` 转挂 turnstile 自检路由。 |

### 7.2 其他

- `internal/database/ent/schema/captcha_challenge.go`（新）+ `scripts/db-generate.sh` 生成代码。
- `migrations/<时间戳>_captcha_challenges.sql`（新，atlas diff 生成）+ `atlas.sum`。
- `internal/common/apierror`：新增两个常量（§3.3）+ 注册表测试用例。
- `internal/app/app.go`：`captcha.New` 改为传 `db`、`settingsModule.Service`、`limiter`，处理返回的 error。
- 依赖：`go.mod` 新增 `github.com/wenlng/go-captcha/v2 v2.0.5`、`github.com/wenlng/go-captcha-assets v1.0.7`，间接带入 `golang.org/x/image`、`github.com/golang/freetype`。

### 7.3 构造顺序

`settings → session → captcha(db, settings, limiter) → login → registration → auth`；`module.New` 内的素材加载发生在启动期，失败即 `app.New` 返回错误、进程不启动。

## 8. 素材与开销（实测）

探针实测（Windows、`go build -ldflags "-s -w"`，仅导入滑块库 + `imagesv2` + `tiles`）：

| 项 | 数值 |
| --- | --- |
| 二进制体积 | hello-world 1.16MB → 3.79MB，**净增约 2.63MB**（真实后端二进制未实测，同量级推断） |
| 启动素材解码 | 约 54ms |
| 常驻内存 | 背景图 16 张（400×340）约 8.3MB |
| 单次出题 | 平均 5.3ms、最大 6.4ms |
| 单次负载 | 主图 JPEG 约 41KB + 拼图 PNG 约 5KB（base64 后约 62KB） |

素材全部来自官方 `go-captcha-assets`，不改图、不自制。

## 9. 上线顺序

- 本轮交付后 `captcha_provider` 保持现状（`none` 或 `turnstile`），**不得切到 `builtin`**：现有前端不会发出题 / 交卷请求，切过去等于全站登录 / 注册被 `captcha_failed` 拦住。
- 切换时机在前端接线完成之后；本轮只保证后端能力与测试可用。

## 10. 测试与验证计划

### 10.1 单元 / 集成

- generator：真实素材出题，断言两张图非空、data URL 前缀正确、尺寸与初始位置非零、答案落在主图范围内。
- 流程（测试库）：
  - 出题 → 交卷 → 消费成功（登录侧 `Verify` 放行）。
  - 同一凭证二次使用 → `captcha_failed`；同一题二次交卷 → `captcha_failed`。
  - 错答案 → `captcha_failed`，随后用正确答案交同一题仍 `captcha_failed`。
  - 题目过期 / 凭证过期 → `captcha_failed`（直接改库时间）。
  - `login` 凭证用于 `register` scope → `captcha_failed`。
  - `captcha_enabled=false`、scope 未勾选 → 放行；`provider=turnstile` 现有用例全部保持。
  - 空 token / 乱填 token → `captcha_failed`。
- 端点：出题 201 形状、`scope` 非法 400、未启用 / 非 builtin 403、`:id` 非 UUID 400、坐标负数 400、限流 429（带 `Retry-After`）。
- 端到端：设置切 `builtin` 后走「出题 → 交卷 → 用凭证登录成功」；切回 `turnstile` 后行为不变。
- 迁移：dev 库与测试库应用后表结构、索引、唯一约束正确；过期行在下次出题时被清掉。
- 变异自检（至少三条，必须让「仅 builtin 路径」的用例裁决）：去掉 scope 校验、去掉一次性消费、去掉过期判断，各变红一次。
- `scripts/check.sh` → `check ok`；`scripts/test.sh` 全量绿。

### 10.2 手工核对（用户执行）

- 用 curl / 浏览器地址栏确认出题端点返回图片（本轮前端未接，页面不可用属预期）。
- 确认切到 `builtin` 时登录被 `captcha_failed` 拦住——这是预期保护，非缺陷；核对完切回。

## 11. 文档与版本

- `backend/README.md`：API 表加两个端点；模块表 `captcha` 行补出题 / 交卷与 builtin 判定；迁移清单加 `captcha_challenges`；设置章节把「`builtin` 选了不拦」改为实际行为与凭证语义（含「切 provider 前必须先上前端」）。
- `docs/DEVLOG.md`：本轮条目（动机、决策、实测数字、已知缺口：三种形态、provider 白名单、前端未接）。
- 版本：`0.23.1 → 0.24.0`（minor：新端点 + 新 provider 行为）。

## 12. 已接受的代价与风险

- **二进制 +约 2.6MB、常驻内存 +约 8.3MB**：官方素材整体引入的直接代价，用户已选官方素材包。
- **出题限流实际是整站配额**：见 §6，拍板时已知情；修这个要先解决「后端取不到真实访客 IP」，属另一轮。
- **凭证不绑 IP**：中间人拿到凭证可在有效期内使用，但凭证一次性且 5 分钟过期，登录侧另有失败计数与限流；绑 IP 在本部署下无意义。
- **前端未接期间不能启用 builtin**：见 §9。
- **过期行清理依赖出题行为**：长期无人使用时会残留少量过期行，下次出题即清。

## 13. 已知缺口（本轮不做，已记录）

- 三种形态（旋转 / 点选 / 拖拽）与难度设置。
- `captcha_provider` 取值白名单：拼错（如 `builtln`）仍会静默不拦，属既有语义。
- 管理端自检接口的 builtin 形态。
- 前端滑块组件与登录 / 注册页接线。
