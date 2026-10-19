# NeoBlog 鉴权入口限流设计

日期：2026-10-20
状态：待评审
范围：后端登录、注册、两步验证 verify 三个公开入口；前端 429 反馈

## 1. 背景

当前后端版本 0.21.1。2026-10-20 的鉴权审查确认三件事已在 HEAD 修复：登录/注册只收 JSON 并校验 Origin（`c66d9c0`）、2FA setup/enable 复核当前密码（`c74170c`）、会话绑定 `auth_version` 堵住在途登录竞态（`c02f951`）。限流仍未做。

`backend/README.md` 的「认证」已知缺口已把「登录、注册与两步验证 verify 暂无限流」列为待补；同章「两步验证」也明确承认二段「会话内连错 5 次」不是在线暴破防御，真正的闸门要等登录限流。本轮补这个闸门。

### 1.1 要解决的问题

1. 匿名者可以对 `POST /api/v1/sessions` 无限猜密码，bcrypt 校验没有失败计数、没有限速。
2. 知道密码的人可以对 `POST /api/v1/sessions/current/verify` 无限猜 6 位 TOTP。现有 5 次上限存在 scs 会话里（`auth.Service.failSecondFactor`），换一个新会话就重置；并发请求还会因会话整行覆盖写而丢计数。
3. 注册接口没有来源限制，可以被用来刷号。

### 1.2 已确认的决策

- 覆盖入口：只做登录、注册、2FA verify 三个公开入口。
- 部署形态：单实例，计数放进程内，不引入 Redis/Postgres 共享存储。
- IP 维度：做；配置可信代理，默认信任本机回环，生产按实际代理网段配置。
- 可配置性：先写死默认值，不在后台设置页暴露。
- 不搞永久锁号；只做临时冷却和临时窗口，响应 429 + `Retry-After`。

## 2. 目标与非目标

### 2.1 目标

- 让在线猜密码、猜 TOTP、刷注册在到达 bcrypt/TOTP 校验之前被挡下。
- 正常用户被挡时得到明确的等待时间，而不是笼统失败。
- 账号维度和 IP 维度同时生效：账号维度防针对单账号的暴破，IP 维度防单来源撞库/刷接口。
- 不改变现有错误信封、不泄漏账号是否存在、不破坏现有 2FA 会话内锁定的交互。
- 默认零配置可跑；单机部署不新增依赖、不新增迁移。

### 2.2 非目标

- 不做 CAPTCHA。后台 `captcha_enabled` / `captcha_provider` 仍是纯数据，另开轮次。
- 不做永久账号锁定。
- 不做全站 `/api/v1` 通用限流。
- 不给改密、2FA 启停、恢复码这些登录后接口加限流。
- 不改登录 CSRF、2FA 密码复核、auth_version 三项已修复的安全逻辑。
- 不做多实例共享计数；不在后台设置页提供次数/窗口配置。
- 不做密码重置。

## 3. 架构与边界

### 3.1 新增能力包

新增 `backend/internal/common/ratelimit`，纯 Go，不 import gin、不 import apierror、不碰数据库。

职责：

- 维护进程内、带互斥锁的计数表。
- 提供两类计数语义：
  - 窗口计数：在固定窗口内允许 N 次，超过即拒绝，返回剩余等待时间。
  - 失败冷却：连续失败达到 N 次后进入冷却期，冷却期内拒绝，返回剩余等待时间；成功时清零。
- 负责过期清理和键数上限，避免被海量不同键打爆内存。
- 时间源可注入，便于测试窗口边界和冷却。

不负责：

- 不认识 HTTP、gin、账号、密码、TOTP。
- 不认识错误码；由 auth 层把拒绝结果翻译成 `apierror`。
- 不负责取客户端 IP；由 controller 传入。

### 3.2 依赖方向

- `internal/app/app.go` 构造一个 `ratelimit.Limiter`，通过构造函数注入 `auth.New`。
- `auth.Service` 持有 limiter，在 `Login`、`Register`、`VerifySecondFactor` 里组合键并调用。
- controller 继续只做请求解析和错误写出；`requestSessionMeta` 负责把可信客户端 IP 放进 `SessionMeta`。
- `internal/common/apierror` 负责 429 错误体与 `Retry-After` 头；ratelimit 包不依赖它。
- 与现有模块规则一致：service 不 import gin；跨层依赖只在 app 层显式注入。

### 3.3 为什么不放模块目录

`internal/modules/<name>` 主要承载业务功能；限流是鉴权链路的横切基础设施，和 `internal/common/guard` 同级。它没有路由、没有 DTO、没有数据库，放 `internal/common/ratelimit` 更贴现有边界。未来如果出现第二个消费者（例如全站限流），仍然是这个包，不需要再提层。

### 3.4 存储实现

- `map[string]*entry` + `sync.Mutex`。
- 每个 entry 记录窗口起点、窗口内次数、连续失败数、冷却截止时间、最后访问时间。
- 每次访问时惰性判断窗口/冷却是否过期并重置；不启动后台 goroutine，避免生命周期和测试清理问题。
- 每 1 分钟（按注入时钟）做一次惰性清扫；键数超过上限时先清过期项，仍超限则按 `lastSeen` 淘汰最旧一项。
- 键数上限默认 100000；对个人博客足够，同时把内存占用封顶。
- 不持久化：进程重启计数清零。攻击者不能重启服务，接受这个上限。

## 4. 限流策略

策略常量放在 auth 包内（例如 `auth/ratelimit_policy.go`），不放进 settings 清单。以下为第一版默认值。

| 入口 | 维度 | 键 | 规则 | 成功时 |
| --- | --- | --- | --- | --- |
| 登录 | 账号/标识 | `login:id:<sha256(trim+lower(identifier))>` | 连续 5 次失败后冷却 15 分钟 | 密码正确即清零 |
| 登录 | IP | `login:ip:<clientIP>` | 固定窗口 30 次 / 15 分钟 | 不清零 |
| 注册 | 邮箱 | `register:email:<sha256(normalized email)>` | 固定窗口 3 次 / 1 小时 | 不重置（成功即建号） |
| 注册 | IP | `register:ip:<clientIP>` | 固定窗口 5 次 / 1 小时 | 不清零 |
| 2FA verify | 账号 | `verify:user:<pending user id>` | 连续 10 次失败后冷却 15 分钟 | 验证通过即清零 |
| 2FA verify | IP | `verify:ip:<clientIP>` | 固定窗口 30 次 / 15 分钟 | 不清零 |

补充规则：

- 账号失败冷却只在连续失败时累积；成功一次立即清零。
- 登录密码正确但账号开了 2FA 时，登录失败计数清零；二段失败计数是独立键，不受影响。
- 未知标识和已知标识走同一套账号计数，避免通过 429 行为枚举账号。
- 账号键对 identifier 做 trim + lower 再哈希，避免大小写/空格变体拆出多条计数；实际用户查询逻辑本轮不改。
- 二段账号键使用会话里服务端写入的 pending user id，不用客户端输入。
- IP 键按入口分开，登录、注册、verify 的额度互不挤占。
- 现有的「会话内连错 5 次 → two_factor_locked」保留。它仍是单个 pending 会话的即时反馈；新的账号冷却负责跨会话、跨并发的总闸门。账号冷却阈值 10 高于会话阈值 5，现有锁定的测试和交互不会被提前改成 429。
- 不做滑动窗口。IP 维度用固定窗口，边界可能允许一次突发；账号冷却负责单账号的精确限制。若后续觉得边界突发不可接受，再换滑动窗口或令牌桶。

### 4.1 为什么账号维度要 15 分钟冷却

这是对「知道密码但不知道 TOTP」的兜底：对方可以不断重新登录拿新的 pending 会话，单会话 5 次限制拦不住。账号级 10 次/15 分钟冷却把跨会话尝试也收拢。代价是知道账号标识的人可以故意打满 5 次登录失败，让该账号短暂无法登录；这是限流的固有取舍，用 15 分钟短冷却、不永久锁定来降低影响。

## 5. 请求流程

### 5.1 登录 `POST /api/v1/sessions`

当前入口：`backend/internal/modules/auth/controller.go` 的 `login`，服务：`backend/internal/modules/auth/service.go` 的 `Login`。

1. controller 先做 JSON/Origin 校验（已存在）。
2. controller 把 `requestSessionMeta(ctx)` 传给 `Login`。
3. service 先查 IP 窗口：超限返回 429。
4. 再查账号冷却：在冷却期返回 429。
5. 保持现有密码长度检查、用户查询、bcrypt 校验。
6. 未知用户/密码错误：记录账号失败，返回现有 `invalid_credentials`。
7. 密码正确：清零账号失败；有 2FA 进入 pending；否则建立会话。
8. IP 计数在成功登录时也不清零。

### 5.2 注册 `POST /api/v1/users`

当前入口：`backend/internal/modules/auth/controller.go` 的 `signup`，服务：`backend/internal/modules/auth/service.go` 的 `Register`。

1. controller 先做 JSON/Origin 校验（已存在）。
2. `Register` 新增 `SessionMeta` 参数，controller 传入客户端 IP。
3. service 先查 IP 窗口：超限返回 429。
4. 保持现有注册开关、字段归一与校验。
5. 归一后的邮箱查邮箱窗口：超限返回 429。
6. 保持现有唯一性检查、bcrypt、建号；注册仍然不建立会话。

### 5.3 二段验证 `POST /api/v1/sessions/current/verify`

当前入口：`backend/internal/modules/auth/controller.go` 的 `verifySecondFactor`，服务：`backend/internal/modules/auth/service.go` 的 `VerifySecondFactor`。

1. service 先查 IP 窗口：超限返回 429。
2. 读 pending user id；没有则保持现有 401 `unauthorized`。
3. 用 pending user id 查账号冷却：冷却期返回 429，不销毁 pending 会话。
4. 保持现有用户查询、TOTP/恢复码校验。
5. 失败：先记录账号失败，再走现有 `failSecondFactor`（会话内计数，达到 5 次返回 `two_factor_locked` 并销毁会话）。
6. 成功：清零账号失败，建立会话。

错误优先级固定为：IP 429 > 无 pending 的 401 > 账号 429 > 现有验证错误。测试按这个顺序断言。

### 5.4 计数发生点

- IP 窗口计数在请求真正进入 service 后发生；被 controller 的 JSON/Origin/空字段校验挡下的请求不计数。这些请求不跑 bcrypt/TOTP，成本低，不是本轮要挡的目标。
- 账号失败计数只在实际校验失败时发生；数据库故障、500 路径不记失败，避免把运维故障算到用户头上。
- 注册邮箱窗口在归一和格式校验之后计数，避免垃圾输入占用某个邮箱的额度。

## 6. 对外错误契约

### 6.1 新增错误

在 `backend/internal/common/apierror/apierror.go` 增加：

- `ErrRateLimit = &Error{Type: TypeRateLimit, Code: "rate_limited"}`。
- `statusByCode["rate_limited"] = http.StatusTooManyRequests`。
- 新增一个可携带等待时间的错误包装；`apierror.Write` / `Abort` 在写出前识别它并设置 `Retry-After` 响应头，取值为向上取整的秒数，最小 1。
- 错误体保持现有信封：`{"error":{"type":"rate_limit_error","code":"rate_limited","param":null}}`，不新增文案字段。
- `param` 固定为 `null`；限流不是字段错误。

### 6.2 响应头

- `Retry-After: <秒>`，整数，符合 RFC 9110。
- 如果等待时间来自账号冷却，就是冷却剩余秒数；来自固定窗口，就是窗口剩余秒数。
- 前端缺失该头时回退到 60 秒，不因为头缺失而显示“未知”。

### 6.3 不泄漏账号

- 已知账号和未知账号在登录时使用相同的账号冷却键形式与相同阈值。
- 429 的响应体不包含剩余次数、账号是否存在、是 IP 还是账号维度触发。
- 成功登录清零账号失败；未知账号没有成功路径，计数自然累积到冷却。

## 7. 客户端 IP 与可信代理

### 7.1 现状

- `app.go` 的 `engine.SetTrustedProxies(nil)` 默认不信任代理。
- `controller.go` 的 `requestSessionMeta` 手工读 `X-Forwarded-For` 首跳，再 `X-Real-IP`，最后 `ctx.ClientIP()`。
- README 明确说这个 IP 仅供展示、不能用于鉴权或限流；因为手工信任 XFF 可伪造，直接用会把限流键交给攻击者。

### 7.2 方案

- `config.Config` 新增 `TrustedProxies []string`。
- 新增环境变量 `TRUSTED_PROXIES`，逗号分隔 IP 或 CIDR；默认 `127.0.0.1/32,::1/128`（同机 Next 代理的零配置场景）。
- 非法 IP/CIDR 或空项启动即失败，与现有配置的 fail-fast 口径一致。
- `app.New` 调用 `engine.SetTrustedProxies(cfg.TrustedProxies)` 并处理返回错误，替换现在的 `SetTrustedProxies(nil)`。
- `requestSessionMeta` 改为只取 `ctx.ClientIP()`；删除手工 XFF/X-Real-IP 分支。
- 效果：限流键和会话列表里的 IP 使用同一个可信来源。生产若经 Nginx/其他主机代理，必须把 `TRUSTED_PROXIES` 配成代理网段；否则所有请求会共用代理机 IP，IP 额度会误伤所有人。同机回环代理不需要配置。
- 本轮不改 `requireJSONRequest` 里无条件读 `X-Forwarded-Host` 的 Origin 比对；那是 CSRF 修复的既有逻辑，和限流无关，避免扩大范围。

### 7.3 测试影响

- `internal/modules/auth/sessions_test.go` 的 `TestLoginStoresSessionMetadata`、`TestLoginTruncatesOversizedHeaders`、`TestSessionsListIncludesCurrentWithMetadata` 直接断言 XFF 进入会话元数据。测试 helper 需要显式 `SetTrustedProxies` 为回环，保持这些用例代表“可信代理”的新语义。
- 新增用例：来自非可信来源的伪造 XFF 不改变限流键/会话 IP；来自可信回环的 XFF 生效。

## 8. 前端行为

### 8.1 API 客户端

- `frontend/lib/api.ts` 的 `ApiError` 增加可选 `retryAfter?: number`。
- `apiRequest` 在非 OK 响应里读取 `Retry-After`，解析为正整数后塞进 `ApiError`。
- 不改变现有 401 → 会话过期的处理；429 不触发 `reportSessionExpired`。

### 8.2 登录与二段

- `frontend/hooks/use-signin.ts` 的 `toMessage` 增加 `rate_limited` 分支，返回「请求过于频繁」。
- 登录步骤被 429：sonner 只显示「请求过于频繁」，不显示等待时间；保持凭据步骤；按 `retryAfter` 暂时禁用提交按钮，按钮文字显示剩余时间（如「12 分 48 秒」）。不要清空已填的 identifier。
- 二段步骤被 429：sonner 同样只显示「请求过于频繁」，保持 verify 步骤；不要调用 `backToCredentials`，因为 pending 会话没有被销毁。冷却结束前禁用验证按钮并把按钮文字切换为剩余时间。
- 现有 `two_factor_locked` 仍按旧逻辑回到凭据步骤；`rate_limited` 和它分开处理。

### 8.3 注册

- `frontend/hooks/use-signup.ts` 的 `toFailure` 增加 `rate_limited`，返回通用消息「请求过于频繁」、不指向具体字段。
- sonner 只显示「请求过于频繁」，不显示等待时间；按 `retryAfter` 暂时禁用提交按钮，按钮文字显示剩余时间；表单值保留。

### 8.4 文案

- `frontend/messages/zh-CN.json` 与 `en.json` 增加 `rateLimited` 与按钮倒计时 `retryIn`：
  - `rateLimited` zh：`请求过于频繁`；en：`Too many requests.`
  - `retryIn` zh：`{duration}`；en：`{duration}`
- 等待时间只出现在按钮文字里（如「12 分 48 秒」），不出现在 sonner；前端把服务端秒数格式化为「45 秒」或「2 分 30 秒」再传入 `{duration}`。
- 不新增组件；沿用现有按钮 disabled 状态和 sonner toast。

## 9. 测试方案

### 9.1 限流器单测

`backend/internal/common/ratelimit/ratelimit_test.go`，注入假时钟：

- 窗口计数：第 N 次允许，第 N+1 次拒绝，窗口到期后恢复。
- 失败冷却：连续 N 次失败后进入冷却，冷却到期后恢复；成功清零。
- 剩余等待时间：窗口/冷却边界的取整和最小值。
- 并发：100 个 goroutine 同时记录失败，最终计数精确、冷却只触发一次。
- 清理：过期键被清扫；键数上限触发淘汰且 map 不无限增长。
- 并发用例用确定性 goroutine + 互斥锁断言；本机 cgo 不可用，**不跑 `-race`**；如需竞态检测在 Linux/WSL 另跑。

### 9.2 apierror 测试

- `rate_limited` 映射 429。
- 错误体仍是 `{"error":{"type","code","param"}}`。
- `Write` 设置 `Retry-After`，取整正确、最小 1。
- 包装错误可被 `errors.As` 识别，不破坏现有 sentinel 表。

### 9.3 鉴权集成测试

- 登录：同一标识 5 次错误 → 第 6 次 429 + `Retry-After`；冷却期正确密码也 429；冷却过后正确密码成功。
- 登录：4 次错误后正确密码清零，再错误 4 次仍不触发冷却。
- 登录：未知标识与已知标识同样在第 6 次 429。
- 登录 IP：不同标识、同一 IP 触发 IP 窗口 429；不同 IP 不互相影响。
- 注册：同一 IP 第 6 次 429；同一邮箱第 4 次 429；不同 IP 独立。
- 2FA：单会话 5 次失败仍是 `two_factor_locked`；重新登录后继续失败，跨会话累计到 10 次后 429；冷却期正确码也 429；冷却后正确码成功。
- 2FA：成功验证清零账号失败，之后可重新累计。
- 2FA：并发失败请求下账号计数原子增长，不会因为会话覆盖写而丢计数。
- 信任代理：可信来源的 XFF 改变 IP 键；非可信来源的 XFF 被忽略。
- 现有 `TestTwoFactorVerifyLockout`、`TestTwoFactorReplayRejected`、登录/注册/会话列表用例全部保持绿。

### 9.4 测试装配

- `auth.New` 签名增加 limiter 参数；`internal/modules/auth/auth_test.go` 的 `newTestAppWithSessionsAndSettings` 与 `totp_flow_test.go` 的测试构造器同步更新。
- 现有测试默认注入宽松 limiter（禁用或极高阈值），避免同一测试 app 内多次登录/注册撞上新额度；限流专用测试注入严格策略。
- `app.New` 构造默认 limiter；如果 app 层现有用例有多次鉴权请求，按同样方式在测试配置里换宽松 limiter。
- 前端无测试框架：`tsc --noEmit`、ESLint、Prettier 通过；429 的 toast/倒计时归用户浏览器实测。

### 9.5 变异自检

- 去掉登录的账号计数调用、去掉 verify 的账号冷却检查、把 IP 限制改成永远放行，分别确认新用例变红，再还原。
- 与项目现有 DEVLOG 的变异自检口径一致。

## 10. 文档与版本

- `backend/README.md`：
  - 从「已知缺口」删除限流条目，改为指向新的「限流」章节。
  - 认证章节增加限流策略表、键口径、429/`Retry-After` 契约、单实例/内存计数的边界。
  - 环境变量表增加 `TRUSTED_PROXIES`。
  - 错误表增加 429 `rate_limit_error`。
  - 会话管理段的 IP 来源改为「可信代理下的客户端 IP，限流与会话列表共用」。
- `docs/DEVLOG.md`：新增本轮条目，记录范围、决策、验证、未实测项。
- `.env.example`：增加可选 `TRUSTED_PROXIES` 示例与一行说明，默认值不强制写入。
- 版本：0.21.1 → 0.21.2（patch），跑 `scripts/bump-version.sh patch`，同步 README 顶部。
- 零迁移。

## 11. 风险与取舍

- **定向锁号**：知道账号标识的人可以打满 5 次登录失败，使该账号冷却 15 分钟。接受；不永久锁定，冷却短。若以后不能接受，改用 CAPTCHA 或更细的渐进延迟。
- **共享出口 IP**：公司/学校 NAT 下 30 次/15 分钟可能被多人共用；个人博客场景可接受，数值是常量，后续可调。
- **单实例/重启清零**：用户已确认单实例；重启后计数清零，攻击者无法重启服务。多实例时需要换共享存储，limiter 接口留出替换空间。
- **固定窗口边界突发**：IP 维度在窗口交界可能放行两倍突发；账号冷却负责精确限制。若不可接受，后续换滑动窗口。
- **键数上限淘汰**：大规模分布式攻击可能把计数表打满并淘汰部分键，使个别键的限额失效；内存封顶优先，个人博客可接受。
- **可信代理配置错误**：生产代理不在默认回环范围时，IP 键会变成代理机 IP，可能误伤所有人。README 和环境变量说明必须写清；IP 阈值留得较宽。
- **429 的可观测性**：429 只说明被限流，不说明账号是否存在；这是刻意的。运维需要区分真实攻击和误伤时，只能看日志/指标，本轮不加。
- **性能**：每次鉴权请求多一次互斥锁操作和少量 map 访问；相对 bcrypt 可忽略。

## 12. 验收标准

1. 同一标识连续 5 次错误登录后，第 6 次返回 429 `rate_limited`，`Retry-After` 为正整数；冷却后正确密码可登录。
2. 未知标识和已知标识的 429 行为一致，响应不泄漏账号存在性。
3. 登录成功清零账号失败计数；IP 计数不清零。
4. 同一 IP 超限返回 429；可信代理下的 XFF 生效，非可信来源的伪造 XFF 不生效。
5. 注册 IP/邮箱窗口生效，返回 429 + `Retry-After`。
6. 2FA 单会话 5 次仍返回 `two_factor_locked`；跨会话累计到账号冷却后返回 429；成功验证清零。
7. 错误体保持现有信封；前端能读到 `Retry-After`，登录/注册/二段分别显示等待提示并暂时禁用提交。
8. 现有后端测试全绿；新增限流单测/集成测试通过；并发用例在现有测试环境可重复绿（本机 cgo 不可用，不跑 `-race`）。
9. README、DEVLOG、`.env.example` 更新；版本 0.21.2；零迁移。

## 13. 实施顺序（供后续 plan 展开）

1. `internal/common/ratelimit` 包与单测。
2. `apierror` 429 + `Retry-After` 包装与测试。
3. `config.TrustedProxies` + `app.New` 可信代理 + `requestSessionMeta` 改 `ClientIP()`，同步会话测试。
4. auth service 接入 limiter、三入口策略、测试装配与集成测试。
5. 前端 `ApiError.retryAfter`、登录/注册/二段错误映射、倒计时与中英文案。
6. README/DEVLOG/.env.example/版本号。

## 14. 待评审确认

- 限流数值（5 次/15 分钟、30 次/15 分钟、注册 5 次/小时、2FA 10 次/15 分钟）是否按第一版执行。
- 前端是否接受“toast + 按 `Retry-After` 禁用提交并倒计时”，还是第一版只弹提示。
- 生产代理网段是否已经有固定值，便于写入部署说明；如果没有，默认回环 + README 说明即可。
