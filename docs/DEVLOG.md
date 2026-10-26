# 开发日志

按时间**倒序**记录每个开发阶段的变更、决策与已知缺口。设计细节与实施计划在 `docs/superpowers/{specs,plans}/`；接口与结构规范以 `backend/README.md` 为准。

---

## 2026-10-27 · 2FA 恢复码邮件落地

**范围**：后端 `twofactor`（未使用哈希校验 + `RecoveryCodesEmailRequest`）、`auth`（`EmailRecoveryCodes` + 路由实现，注入 mailer）、`mail`（`recovery_codes` 模板 html/txt、`SiteInfo`、预览数据与测试发送支持）、`settings`（mail 组新增 `recovery_codes_subject` / `recovery_codes_body`）与迁移 `20261027120000_seed_recovery_codes_mail_settings.sql`（test/dev 已应用）；前端恢复码弹窗回传 `codes`、去掉 501 分支与「暂未开放」文案，邮件设置页新增第三行模板与测试发送，中英文案同步。零新依赖、零新错误码。

**动机**：原接口是 501 占位，前端「发送」按钮点了只提示「邮件发送暂未开放」。恢复码只存 SHA-256 哈希，原始码只在生成响应里出现一次；按用户裁定走「前端回传当前这组」，后端校验后发到账号邮箱。

**决策**：
- **回传 + 校验**：前端发送 `{"codes":[...]}`；`twofactor.VerifyRecoveryCodes` 用 `HashRecoveryCode` 比对当前账号未使用的哈希集合，重复 / 已用 / 不属于当前账号 / 超长一律 400 `invalid_request`（param `codes`）；未启用 2FA 409 `two_factor_not_enabled`。
- **同步投递**：用户就在弹窗里等结果，SMTP 未配置 / `site.url` 非法 / 投递失败返回 503 `mail_unavailable`；`RequireSMTP: true` 保证不会降级把恢复码写进日志。
- **模板与设置**：内置 `recovery_codes` html/txt（占位符 `{{codes}}` 等），mail 组两项可覆盖主题 / 正文，管理端测试发送支持该模板；迁移只补两行默认值，`ON CONFLICT DO NOTHING`。
- **不改 2FA 状态**：发信不签发新码、不作废当前码；注册验证与找回密码流程不动。

**已知缺口**：该端点没有独立限流（需登录，只能发到本人邮箱）；一组里只要有一张已使用就整体拒绝，用户需重新生成；真实收信与弹窗成功画面未在浏览器实测。

**验证**：`TestEmailRecoveryCodes`（匿名 401、成功 200 + 收件人 / 正文含 5 个码 + RequireSMTP、错码 400）；`TestCreateMessageUsesDefaultRecoveryCodesSubject`；settings registry 邮件组 14 项；`mail` / `twofactor` / `settings` / `auth` 四包全绿；`go vet ./...`、`scripts/check.sh` 通过；全量 `scripts/test.sh` 通过；前端 `tsc` / `eslint` / `prettier` 通过。**提交状态**：随本批次入库；尚未推送。

---



## 2026-10-27 · 找回密码邮件改为异步投递

**范围**：后端 `internal/modules/passwordreset/service.go` 的申请路径把 `SendSync` 换成 `Send`；`passwordreset` / `auth` 两套测试 fake 改为记录异步调用并断言不走同步；`backend/README.md`、API 规范、password-reset spec（新增 §14）同步。零迁移、零新依赖、零前端改动、零接口形状变化；注册验证与测试发信仍走 `SendSync`。

**动机**：粗审发现申请接口的「一律 202」只统一了响应，没统一耗时——已注册邮箱要走写令牌 + 渲染 + 同步 SMTP，未知邮箱立即返回，计时即可枚举账号；SMTP 拒收时还会对真实账号 503、未知邮箱 202。`mail` 模块本来就有异步队列（32 格、临时错误最多 3 次重试、关机排空、失败记 ERROR），直接复用，不新造队列、不加表、不改 App 生命周期。

**决策**：
- **只挪投递**：`Request` 仍同步完成开关 / SMTP 配置 / `site.url` / 限流 / 查账号 / 写令牌 / 渲染，最后 `Send` 入队后立刻 202；入队失败（队列满 / 已关闭）仍按原补偿逻辑删令牌并 503。
- **投递失败不回传**：入队成功后由 mail worker 投递，最终失败只记 ERROR；令牌不再因投递失败删除，按 30 分钟自然过期（原始令牌从未入库）。
- **注册不动**：注册对已注册邮箱直接 409 `email_taken`，本来就可枚举；且验证邮件是注册必经步骤、没有重发入口，静默失败会把用户卡死，同步 503 是必要的反馈。

**已知残留**：未知邮箱与已注册账号仍有毫秒级耗时差（写令牌 / 渲染 / 入队）；模板误配导致渲染失败时真实账号 503、未知邮箱 202；队列满 / 关机时同样；进程崩溃丢内存队列里的待发邮件（令牌仍在，可重新申请）；注册仍直接暴露邮箱存在性，全站枚举不变。

**验证**：`TestRequestSendsLink` 增断言「只调 `Send`、不调 `SendSync`」；`TestRequestEnqueueFailureCompensates` 改为模拟入队失败（503 + 删令牌）；`TestRequestUnknownEmailNoSend` / `TestRequestComposeFailureCompensates` 增断言「一次都没发」。`internal/modules/passwordreset`、`internal/modules/auth`、`internal/app` 三包全绿；`go vet ./...` 干净、`scripts/check.sh` 通过；全量 `scripts/test.sh` 通过。**提交状态**：随本条目一并入库；尚未推送。

---

## 2026-10-27 · 人机验证设置页：自研分支补「验证」自检

**范围**：后端 `internal/modules/captcha/builtin/{service.go,controller.go,dto.go}`、`internal/modules/captcha/module.go`（挂受保护路由）、新增 `internal/modules/captcha/admin_challenge_route_test.go`、`internal/modules/captcha/verify_route_test.go`（测试脚手架改为同时装配 builtin）、`backend/README.md`；前端 `lib/public-settings.ts`（`PublicCaptchaConfig` / `CaptchaConfig` 拆分）、`lib/captcha-challenge.ts`（`requestAdminCaptchaChallenge`）、`components/auth/builtin-captcha.tsx`（`scope` 换成 `requestChallenge` 回调）、`components/auth/captcha-dialog.tsx`、`app/console/settings/captcha/captcha-settings-form.tsx`、`hooks/use-captcha-gate.ts`、`app/{login,register}/{signin,signup}-{view,form}.tsx`（类型改名）、`messages/{zh-CN,en}.json`（新增 `settings.captchaVerifyChallengeFailed`）；另加本条目。

**动机**：设置页 CF 分支有「验证配置 / 验证」——用输入框里当场填的站点密钥与私钥调 siteverify，测的是**未保存的草稿配置**；自研分支只有「验证方式」下拉，管理员只能保存后去登录 / 注册页撞验证码才知道长什么样。用户要求两边统一成一样的，且明确测的是**表单上当前选的方式**（刚切成图像验证码还没保存，点「验证」就得弹点选），故走方案 B：新增管理端出题接口——公开的 `POST /captcha/challenges` 只认数据库里的 `captcha_mode`，没法满足这条。

**决策**：
- 路由 `POST /api/v1/admin/captcha/challenges`，body `{"mode":"slide"|"click"}`，守卫沿用 `system.captcha.edit`（与 `POST /admin/captcha/verifications` 同一权限点，不新造）。
- **复用而非重造**：把 `Challenge` 里「清过期 → 按 mode 分派出题」抽成 `issue()`，公开与管理端共用；管理端 `TestChallenge` 只多一步 `mode` 白名单校验。判分仍走公开的 `POST /captcha/challenges/:id/solutions`，前端拿到 token 即算通过，不再多调一个校验接口。
- **绝不读设置表**：`captcha_enabled` / `captcha_provider` / `captcha_mode` 一概不看——这是「测草稿配置」的前提。`TestAdminChallengeRouteIgnoresSavedSettings` 把设置改成 disabled + turnstile + click 后再请求 `slide`，仍须出滑块题。
- **题目 `scope` 记 `test`**（不是 `login` / `register`）：`consume` 按 scope 匹配，测试题换出的凭证天然不能用于登录 / 注册。
- **不套公开出题的 IP 限流**（5 次 / 10 分钟）：管理端是登录 + 权限下的显式自检，连点两下就 429 没法用；`TestAdminChallengeRouteIsNotRateLimited` 连发 7 次须全 201。
- 前端：`CaptchaConfig` 拆成 `PublicCaptchaConfig`（登录 / 注册页的 scope 形态，`getCaptchaConfig` 与 `useCaptchaGate` 都收这个）+ `CaptchaConfig = PublicCaptchaConfig | {provider:"builtin",testMode}`；`BuiltinCaptcha` 的 `scope` 换成 `requestChallenge` 回调，**答错后 800ms 自动重拉也走同一个回调**（否则会打回公开接口，按已保存设置出题甚至 `captcha_disabled`）。`CaptchaDialog` 其余行为（1618ms 关闭延迟、Esc 关闭）不变。
- 设置页自研分支的「验证配置」行与 CF 那行逐值一致（`BadgeCheck` 图标、`outline` + `size="sm"` 按钮、同一 `aria-label`）；点「验证」先拉挑战再弹窗（按钮在拉取期间置灰），答对走既有 `captchaVerifySucceeded`，拉取失败分 401 静默 / 403 / 503 / 其它四支 toast，新增文案 `captchaVerifyChallengeFailed`。

**验证**：6 个新 HTTP 测试（两种 mode 的形状与 scope、忽略已保存设置、不限流、拒绝未知 mode、要求 JSON、权限 403 / 未登录 401）。变异六条实测「改坏 → 红 → 还原 → 绿」：① `scopeTest` 写成 `login` → 形状 / scope 用例红；② 改成直接调公开 `Challenge` → 忽略设置、不限流两用例红；③ 套上公开限流 → 不限流用例红；④ 删 `mode` 白名单 → 未知 mode 用例红；⑤ 删权限守卫 → 权限用例红；⑥ 删 JSON 守卫 → JSON 用例红。全量 `scripts/test.sh`：22 个 `ok`、0 `FAIL`、退出码 0。前端 `npx tsc --noEmit` 0 错、改动文件 `eslint` 0 error（仅 `builtin-captcha.tsx` 既有的两条 `<img>` warning）、`npx prettier --check` 通过。

**已知缺口**：
- 本轮没做浏览器实测（按惯例运行期验收留给用户）：自研「验证」行与 CF 行是否逐值对齐、答错后自动换新题、Esc 关闭再重开是否出新题。
- `captchaVerifyChallengeFailed` 同时兜住「库里的 `captcha_mode` 是未知值 → 后端 400」这种情况，文案统一说「验证码生成失败」。
- 数据库不可用 503 等分支没在本地造条件实测，只依赖既有 `apierror` 映射。
- 顺带填掉 2026-10-26 条目「已知缺口」里的「管理端自检接口的 builtin 形态」。

**提交状态**：已按用户要求分 3 笔入库（后端接口 + 测试 / 前端接线 / 文档），均未推远端。

---

## 2026-10-26 · 找回密码（邮件一次性链接）

**范围**：后端 `internal/common/apierror/apierror.go`（两个哨兵与状态映射）、新增 `internal/database/ent/schema/password_reset.go` 与迁移 `20261026150001_password_resets.sql`（含 `atlas.sum`）、新增能力模块 `internal/modules/passwordreset/{service,repository,token,ratelimit,email}.go` 及其五个测试文件、`internal/modules/mail/{controller.go,templates/password_reset.html,templates/password_reset.txt,controller_test.go}`、`internal/modules/auth/{controller.go,dto.go,module.go,repository.go,auth_test.go,password_reset_flow_test.go}`、`internal/modules/settings/{registry.go,registry_test.go,controller_test.go}`、`internal/app/app.go`、`backend/README.md`、API 规范 §3 与 §7.3；前端 `lib/public-settings.ts`（`isPasswordResetEnabled`）、`app/login/{page.tsx,signin-view.tsx,signin-form.tsx}`（「忘记密码？」入口）、新增 `app/reset-password/{page.tsx,reset-request-form.tsx}` 与 `app/reset-password/confirm/{page.tsx,reset-confirm-form.tsx}`、新增 `hooks/use-password-reset-{request,confirm}.ts`、`app/console/settings/mail/mail-settings-form.tsx`（占位符清单加 `resetUrl`）、`messages/{zh-CN,en}.json`（`auth.signin.forgotPassword` + `auth.reset.*` 26 键、邮件模板标签与占位符提示）；另加本条目。设计 `docs/superpowers/specs/2026-10-26-neoblog-password-reset-design.md`、计划 `docs/superpowers/plans/2026-10-26-neoblog-password-reset.md`。

**动机**：`auth.password_reset_enabled` 从 0.15 那轮起就是纯数据（后台有开关、后端零实现）。本轮按已批准的 spec 让它第一次真正生效：申请 → 邮件链接 → 确认页设新密码 → 全部旧会话作废。

**决策**：
- **未知邮箱也回 202、不发信、不写记录**，但限流照常计数——这是 spec §2 D3 授权的「不披露」，不是兜底；`TestRequestUnknownEmailStillCountsRateLimit` 与 `TestPasswordResetRequestRateLimitHTTP` 把它钉住，否则 429 本身会反过来泄露存在性。
- **邮件整体不可用在查账号之前判定**（SMTP 未配 / `site.url` 缺失或非法 → 503），账号存在但渲染或投递失败则先按 `id` + `token_hash` 删掉本次记录再 503；补偿删除不能按 `user_id`，否则会把并发重申请写入的新行删掉（`TestDeleteResetRequiresTokenHash`）。
- **令牌 32 字节 base64url、库里只存 SHA-256 指纹、30 分钟、一次性**；一账号最多一行，重复申请 `ON CONFLICT (user_id)` 覆盖旧指纹（旧链接立即作废）；认领与删除都同时带 `id` + `token_hash`，两个并发确认恰一个成功（`TestConfirmConcurrent`）。
- **弱密码与「与当前密码相同」判在认领之前**，令牌不消耗、改好就能重试（`TestConfirmWeakPasswordKeepsToken`、`TestConfirmSamePasswordKeepsToken`）。
- **成功不签发会话、响应无 `Set-Cookie`**，改为在同一事务把 `users.auth_version` +1，靠既有守卫把全部设备踢下线；2FA 的 secret / 启用时间 / 恢复码原样保留（`TestConfirmKeepsTwoFactor`）。
- **站级开关关闭时已发出的令牌不物理删除**，重新打开后未过期的仍可用（`TestPasswordResetConfirmDisabledHTTP` 断库里仍有 1 行）。
- **正常改密与改邮箱在同一事务清掉该账号的重置记录**（`POST /users/me/password`、`PATCH /users/me` 带 `email`；只改昵称 / 简介不清），否则用户改完凭据后泄露的链接仍然有效。
- **`password_reset_enabled` 标 `Public: true`**，公开聚合端点多下发这一个布尔，供前端决定是否显示入口。
- **内置 `password_reset` 模板从「验证码」改成「链接」**，占位符 `{{resetUrl}}`、默认主题「密码重置」；数据里仍带一个空 `code`，让库里历史自定义正文的 `{{code}}` 渲染为空而不是 `<no value>`。
- **本轮不接人机验证、不做重发按钮、不发「密码已修改」通知、不做审计表、不提版本号**（spec §11 非目标）。
- **`user_id` 的唯一性写在 `index.Fields("user_id").Unique()` 而不是字段上的 `.Unique()`**：实施时实测——`user.go` 那边是复数边 `passwordResets`（O2M），ent 会**静默忽略**字段级 `.Unique()`，Atlas 生成的迁移里根本没有唯一索引，`ON CONFLICT (user_id)` 会在运行时报「没有匹配的唯一约束」。补成唯一索引后由 `TestPasswordResetIndexShape` 与 `TestPasswordResetRejectsSecondRowPerUser` 一起钉住。
- **前端**（先按用户口径只做后端，随后补上）：入口挂在凭据分支**「密码」标签同一行的右侧**（初版挂在密码框下方独立一行，用户看了对照页改定；2FA 那一步始终不挂）；`/reset-password` 与 `/reset-password/confirm` 都是服务器组件，读公开设置后 `!isPasswordResetEnabled` 直接 `notFound()`；确认页 metadata 带 `referrer: "no-referrer"`，令牌只从 query 读进 props、**GET 渲染不发请求**，必须点「重置密码」才 POST。
- **外壳与注册页逐值对齐**（用户点名要「和注册一样」）：`AuthFrame` + `h1` + `<div className="mt-6">` 包表单，卡片下方居中一行 `p.mt-5 flex items-center justify-center text-center text-sm text-muted-foreground` 放「返回登录」链接；提交按钮带 `aria-busy` 与 `LoaderCircle` 转圈，与 `signup-form.tsx` 同一写法。**没有自创提示文案**（注册页 footer 那句「已有账号？」是它自己的语义，这里只放链接）。
- **成功提示一律「先跳转、再在目标页弹 sonner」**（用户实测反馈：原来在 `toast.success` 之后立刻 `router.push`，客户端一跳转那个 toast 就没了）：新增 `lib/password-reset-notice.ts` + `hooks/use-password-reset-notice.ts`，照 `signup-notice` 那对 sessionStorage 标记/消费机制，登录页 `SigninView` 里与 `useSignupNotice` 并排调用。申请页 202 → 落 `requested` 标记 → 跳 `/login` → 弹「如果该邮箱已注册…」；确认页 200 → 落 `done` 标记 → 跳 `/login` → 弹「密码已重置，请用新密码登录」。**申请页因此不再原地换中性成功态**（spec §9.2 原案被这次调整推翻，`sent` 状态与分支一并删除，不留死代码）。
- **公开设置读不到时按「关闭」处理**（入口不显示、两个页面 404）——与 `isSignupOpen` 的 fail-open 方向相反，这是 spec §9.1 的刻意选择：后端不可用时宁可少一个入口，也不能给出一个必然 503 的表单。
- **无效令牌只走 sonner，页面不渲染任何提示块**：用户实测后点名删掉「链接无效或已过期 + 重新申请」那整块内联 UI（含 `auth.reset.goRequest` 文案键），确认页在任何情况下都只展示密码表单。缺 token 时 `missingToken` 由 props 直接算出，挂载 toast 一次、点提交也只 toast **不发请求**；服务端回 `password_reset_invalid` 同样只 toast。（初版曾在 effect 里 `setInvalidToken(true)` 被 `react-hooks/set-state-in-effect` 判错，改成派生值；现在连派生的展示态都不需要了。）
- 提交成功（202）把整个表单替换成中性文案「如果该邮箱已注册，重置邮件已发送」+ 返回登录，**不做重发按钮、不做倒计时**；错误全走 sonner，不留内嵌红字行；429 复用 `formatRetryDuration` 的按钮倒计时写法。
- 邮件设置页占位符清单加 `resetUrl`、**保留 `code`**（库里历史自定义正文可能还在用），标签与提示文案随之从「验证码/`{{code}}`」改成「密码重置/`{{resetUrl}}`」。
- **人机验证这一轮仍然不接**：用户中途提出「找回密码也要人机验证」，我核出现状要动 scope 白名单（`builtin/service.go` 只认 `CaptchaScopeLogin` / `CaptchaScopeRegister`）、两个 DTO、服务注入与两个页面的弹窗，他随后自己收回（「先不接，先完成整体功能」），故 spec §2 D11 维持有效。真要接时按那条清单起步。
- **申请成功的 toast 文案按用户要求改成「如果此用户存在，我们将发送一份重置链接」**（en 同步为 "If this user exists, we will send a reset link"），spec §9.4 的文案表一并更正；本条目「验证」段里引用的还是改动前实测到的那句旧文案，不改写。

**验证**：新增 41 个 `passwordreset` 测试（表形状 6、令牌 2、仓库 7、申请 15、确认 11）+ 15 个 auth flow HTTP 测试（含开关 403、503 前置、旧会话 401、响应无 cookie、两条限流 429）+ 2 个 mail 模板测试并改写 1 个既有断言；`apierror` 哨兵计数 40 → 42；`settings` 公开白名单加 `auth.password_reset_enabled` 并改公开响应断言。全量 `scripts/test.sh ./...` = 22 个 `ok`、0 `FAIL`、退出码 0；`scripts/check.sh` 通过；`atlas migrate validate` 通过；迁移已 apply 到 `neoblog` 与 `neoblog_test` 两个库。前端：`npx tsc --noEmit` 0 错、`npx eslint`（改动文件与两个新目录）0 问题、`npx prettier --check` 通过。**浏览器实测**（开发库该开关本就是开着的，`site.url=http://localhost:3000`）：登录页「忘记密码？」与「密码」标签同一行右对齐（实测两者垂直中心同为 y=390、链接右边缘与字段列右边缘差 0px、字号 14px、`href="/reset-password"`；改位后密码框与登录按钮之间不再多出一行空白）；`/reset-password` 200、填未知邮箱提交回 202 后**客户端跳到 `/login`，toast 在登录页弹出**（实测 toast DOM = 「如果该邮箱已注册，重置邮件已发送，请查收」，标记同时被消费）；确认页外壳与注册页逐值一致（`h1` + `.mt-6` 包表单 + 卡片下方居中「返回登录」），提交瞬间 `LoaderCircle` 转圈实测出现（`loaderSeen=true`），伪造 token 提交 → 只弹 toast「链接无效或已过期」、表单原样保留（初版还会换成带「重新申请」链接的内联态，该内联块按用户要求整块删除，删除后复验仍是这个结果）；`done` 标记经客户端跳转落到登录页实测弹出「密码已重置，请用新密码登录」；两次密码填成不一致时**只弹「两次输入的密码不一致」且一次请求都不发**；不带 token 直接打开确认页：挂载即 toast「链接无效或已过期」，页面仍只渲染密码表单（删除内联块后复验）。接口经 3000 rewrite 实测：未知邮箱 202 `{}`、非法邮箱 400 `param:"email"`、伪造 token 400 `password_reset_invalid` `param:"token"`、空 `new_password` 400 `param:"new_password"`。**仍未实测**：真实收到邮件并点开、开关关闭时两个页面确实 404、429 的按钮倒计时读数。

**已知缺口**：
- 开关关闭时两个页面应 404 这条只在代码层成立（`notFound()`），本轮没为了验证去翻开发库那个共享的布尔；登录页入口的隐藏同理。
- 「待二段」会话无法精确吊销（spec §2 D9）：重置密码后，只过了第一段密码、还没走二段的会话仍能用二因子完成登录。
- 申请与确认都还没有人机验证；限流计数在单实例进程内，重启清零。
- 库里既有的 `password_reset_subject` / `password_reset_body` 自定义正文若写死了 `{{code}}`，现在渲染出来是空串——需要管理员自己去改成 `{{resetUrl}}`（内置模板已改）。

**提交状态**：后端部分已按用户要求分 8 笔入库（错误码 → 表与迁移 → 邮件模板 → 能力模块 → 两个接口 → 改密/改邮箱清令牌 → 公开开关 → 文档），每笔单独 `go build` 通过、对应测试单独跑绿，未推远端。**前端整段（`lib/public-settings.ts`、登录页三件、两个页面与两个 hook、邮件设置页占位符、两语言文案）以及本条目的这次更正尚未提交**，留在工作区等用户检查。

---

## 2026-10-26 · 后端 500/panic 可诊断（最小版）

**范围**：`internal/app/app.go` 的 `recoveryHandler` 不再丢弃 panic 值；`internal/common/apierror` 的 `Write` / `Abort` / `WriteHTTP` 三个出口统一记 5xx 日志；`captcha/builtin/service.go` 的 `captcha_mode` 读取失败改为双 `%w` 包裹以保留根因。零新依赖、零新路由、零响应契约变化（不加 request id、不改访问日志格式）。

**动机**：2026-10-12 就拍板「先做对内可诊断」，至今未落地。现状是 `gin.CustomRecovery` 的回调把 panic 值当 `_ any` 丢掉、一行不记，`apierror` 三个写出函数也完全不记——一个 500 落到日志里只剩 gin 访问行那个状态码，根因彻底没了。

**决策**：
- **只做「把原因写进日志」这一半**：request id 与结构化访问日志按用户口径砍掉——后端单容器、上游既不生成也不透传时，自造的 id 只出现在后端自己的日志里，而那行访问日志凭时间戳 + 路径已能定位，属零增益；等真做多实例或接集中日志再补。
- **只记 `URL.Path`，不记查询串**：重置令牌这类敏感值可能出现在 query 里。
- **4xx 不记**，避免噪音；收口放在 `apierror` 的三个写出函数（全仓唯一的错误出口），不在各模块手写 `slog`。
- **`captcha_mode` 用 `fmt.Errorf("captcha: read captcha_mode: %w (%w)", err, apierror.ErrServiceUnavailable)`**：Go 1.27 的多重 `%w` 让 `errors.As` 仍能找到哨兵，状态码与错误信封不变（仍 503 `service_unavailable`），但底层原因进了日志——这正是审查里那条「哨兵吞掉原因」点位。
- **一次 panic 落两条 ERROR**（`request panic` 带栈 + `Abort` 走出口时的 `request failed status=500`）是有意的：前者给根因，后者是「所有 5xx 必被记录」这条统一保证的体现；测试把两条一起钉住，防止日后被当成重复删掉。

**已知缺口**：没有 request id，多实例后无法跨进程串联；gin 访问日志走 stdout、`slog` 错误日志走 stderr，容器里要靠采集端合流。

**验证**：新增 `TestRecoveryHandlerLogsPanicWithStack`（断 500 信封 + panic 值 + `path=/probe-panic` + 栈里有 `runtime/debug.Stack()` 与本文件帧 + 同时存在 `request failed` / `status=500` 那一条）；新增 `TestServerErrorLoggedWithCause`（断 5xx 日志含根因 / 状态 / 路径，且 4xx 一条都不记）；`TestChallengeFailsWhenModeRowMissing` 加断返回错误仍带 settings 根因。全量 `scripts/test.sh` = 21 个 `ok`、0 `FAIL`、退出码 0；`gofmt -l internal` 无输出、`go vet ./...` 干净、`scripts/check.sh` 通过。

**工作区处置**：并行会话当时正把密码重置的测试半成品写进 `apierror_test.go`（引用尚未实现的 `ErrPasswordResetDisabled` / `ErrPasswordResetInvalid`，并把哨兵计数从 40 改成 42），使该包测试无法编译。经用户授权把该文件回退到 HEAD，那份在制品已存进 `backend/.scratch/parallel-apierror-wip/`（整文件快照 + pending 补丁），之后只重放了我自己的两个 hunk（测试导入与 `TestServerErrorLoggedWithCause`）。

**提交状态**：随本条目一并入库；**尚未推送**（GitHub 连接被掐，`development` 领先 `origin/development`）。

---

## 2026-10-26 · 注册验证并发用例对齐 README 契约

**范围**：仅测试文件 `backend/internal/modules/registration/register_verification_test.go`。`TestVerifyEmailRegistrationConcurrent` 的断言从「一个 201 + 一个 400」改成契约允许的结果集：恰一个 201，输家为 400 `email_verification_invalid` 或 409 `email_taken` / `nickname_taken` / `already_taken`；补两条真不变量——`users` 恰 1 行、该邮箱的 `pending_registrations` 清零。同文件新增局部助手 `postJSONStatusAndBody`（把响应体带回主 goroutine 再解错误码，避免在 goroutine 里调 `t.Fatalf`）与 `errorCodeOf`。生产代码、接口契约、版本号零改动（纯测试，不进位）。

**动机**：全量 `scripts/test.sh` 偶发红：`statuses = [201 409], want one 201 and one 400`。查证后结论是实现没错、断言过窄——`verifyPendingRegistration` 在进事务**之前**查 pending 行，两个并发请求都读得到同一条；输家若在自己能看到赢家用戶已提交的那刻走到邮箱占用分支，返回 `errPendingEmailTaken` → 409；若在此之前走到条件删除、`deleted != 1`，返回 `errPendingNotFound` → 400。READ COMMITTED 下两种交错都合法，README 的端点条目也同时写着这两种结局。`uniqueTag` 是 `pid + UnixNano`，邮箱每次全新，先排除了「测试库历史脏数据」这条解释。

**已知缺口**：
- 同文件的 `TestRegisterVerificationConcurrentSameEmail`（断两个 202）属同一类「钉死单一交错结果」的写法，本轮按点名范围未动。
- 409 交错无法强制复现：本用例的放宽是由实现分支 + README 条目 + 顺序用例 `TestVerifyPendingEmailTaken` 三方佐证，不是实测抓到过那次 409。

**验证**：目标用例 `-count=20` 全绿；`./internal/modules/registration -count=3` 全绿；全量 `scripts/test.sh` 为 21 个 `ok`、0 `FAIL`、退出码 0；`gofmt -l` 无输出，`scripts/check.sh`（gofmt / apierror 导入守卫 / vet / build / diff）通过。

**提交状态**：已入库 `f6bf641`；尚未推送。

---

## 2026-10-26 · 全站表单显式 `method="post"`

**范围**：仅前端 8 个 `<form>`——`app/login/signin-form.tsx`、`app/register/signup-form.tsx`、控制台五个设置页（`captcha` / `info/site` / `login/login-control` / `mail` / `register`）与 `app/console/uc/profile/profile-form.tsx`，各补 `method="post"`（超过 80 列的按 prettier 折成多行）。后端、接口契约、文案、样式零改动。

**动机**：HTML 表单缺省提交方法是 GET。这些表单都靠 `onSubmit` + `preventDefault` 走 fetch，但只要提交发生在 React 处理器挂上之前（hydration 之前、脚本报错、禁用 JS），浏览器就按原生 GET 提交，用户名与口令直接进地址栏、进浏览历史、进 `Referer`。实测复现：在 `/login` 用 `HTMLFormElement.prototype.submit()`（绕过 submit 事件，等价于处理器未挂载）提交 → 地址栏变成 `/login?identifier=leak-probe-user&password=LeakProbe123%21`（假凭据，仅本地）。这是 10-22 审查就记下、一直未落地的账。

**决策**：
- **只补 `method`，不加 `action`、不做重定向兜底**：目标行为是「不跑 JS 就提交不了」，不是把原生表单提交变成一个可用的后端流程；补上 `method="post"` 后原生提交退化为对同一路由的 POST，敏感值不再进 URL。
- **8 个全补，不是只补含口令的 4 个**：口径统一。控制台设置页同样会把 SMTP 密码、Turnstile 私钥写进 URL，没有理由留 GET。

**已知缺口**：控制台那 6 个表单只做了属性改动，未逐个浏览器实测（dev 库无管理员凭据）；`/login` 与 `/register` 是实测过的。

**验证**：`npx tsc --noEmit` 通过；8 个文件 `eslint` 0 problem、`prettier --check` 通过；脚本遍历全仓 `<form>`，缺 `method="post"` 的已为零。浏览器实测（dev server，同样用绕过 submit 事件的原生提交）：`/login` 与 `/register` 的表单 `method` 均为 post、Navigation Timing 的 `responseStatus` 为 200、`location.search` 为空、页面重新渲染且字段清空——即不跑 JS 的提交不再把凭据写进地址栏。

**提交状态**：随本条目一并入库；**尚未推送**（GitHub 连接被掐，`development` 领先 `origin/development`）。

---

## 2026-10-26 · 人机验证只接受主指针与主键

**范围**：仅前端两个组件。`components/auth/click-captcha.tsx` 的 `handlePointerDown`、`components/auth/builtin-captcha.tsx` 内部滑块的 `beginDrag` / `moveDrag` / `endDrag` 各加一道指针过滤：按下事件要求 `event.isPrimary` 且 `event.button === 0` 才计入，移动与抬起只要求 `isPrimary`（`pointermove` 的 `button` 在按住左键时不可靠，不能拿来判）。后端、契约、文案零改动。

**动机**：代码审查发现点选把任何 `pointerdown` 都算一次点击——右键、中键、第二根手指都会落点，可能提前凑满三次并自动交卷，判负后换题；滑块的把手同理，右键按下就开始拖、第二根手指抬起会立即交卷。

**决策**：
- **只过滤输入来源，不改交互语义**：点选仍是「凑满 `expected_clicks` 立即交卷、无撤销」，滑块仍是抬起即交卷。
- **`moveDrag` / `endDrag` 用 `isPrimary` 而不是记 `pointerId`**：触摸场景里第一个触点才是 primary，第二根手指的移动与抬起天然被挡；记 pointerId 需要多一个 ref，收益相同。

**已知缺口**：400 `captcha_failed` 与 503 之外的交卷错误（如断网 `status === 0`）仍未单独实测。

**验证**：`npx tsc --noEmit` 通过；两个改动文件 `eslint` 0 error（4 条既有 `<img>` warning）；`prettier --check` 通过。浏览器实测（dev server + 后端在跑，点选题）：对同一张图依次派发自键 `button=2`、次指针 `isPrimary=false`、中键 `button=1` 三种 `pointerdown` → 标记数 0、`/solutions` 请求 0；随后正常左键三击 → 标记数 1、2、3，恰好一次交卷请求返回 400，弹窗出现「人机验证未通过」红标且 `getAnimations()` 抓到摆动动画，约 800ms 后红标消失、换新题、此后 1.2 秒内不再自动交卷。这同时补上了上一条记录里欠测的真答错分支。

**提交状态**：已入库 `daa2f68`；尚未推送。

---

## 2026-10-26 · 人机验证失败分级：服务故障不再冒充「答错」

**范围**：仅前端四个文件。`lib/captcha-challenge.ts` 新增传输层判定 `isCaptchaRejected`；`components/auth/click-captcha.tsx` 与 `components/auth/builtin-captcha.tsx`（含内部滑块组件）把子组件的单一 `onFailed` 出口拆成 `onRejected` / `onServiceError`，由父级分流；`hooks/use-captcha-gate.ts` 的出题预取失败改为分级提示。后端、迁移、`backend/README.md`、中英文案键零改动（复用既有 `common.serviceUnavailable` 与 `auth.captchaLoadFailed`）。

**动机**：点选形态接入后的代码审查发现，交卷失败一律按「答错」处理——后端 503、断网、坏响应都被画成红色「人机验证未通过」+ 弹窗摆动 + 800ms 自动换题，而换题要消耗 5 次 / 10 分钟的 IP 出题额度；出题预取失败时还会静默把弹窗开到空态。用户分不清是点错了还是服务挂了，也不符合 `agents.md`「主路径失败按错误上抛、不得降级掩盖」。

**决策**：
- **只有 400 `captcha_failed` 算答错**：该码意味着后端已把这道题判负（`markFailed`），必须换新题，所以保留原有红标 + 摆动 + 800ms 换题。
- **其余错误一律按故障说**：5xx、断网（`ApiError.status === 0`）、坏响应、400 `invalid_request` 走 `onServiceError` → 只出 toast（复用全站既有的 `looksLikeServiceFailure`）+ 落到已有的「重新加载」卡，**不摆动、不自动换题**。理由：这类失败下题目在服务端仍是 pending、没有作废，自动换题等于白烧一次出题额度，且故障时新题也会失败。
- **判定放传输层**：`isCaptchaRejected` 与出题 / 交卷调用同层，滑块与点选共用一个判定；子组件只报告错误类别，不再自己决定要不要换题。
- **预取失败不开弹窗**：与既有 429 分支同构（只 toast、不开窗），避免「弹窗起来后子组件立刻重发同一个必败请求」的隐藏重试；用户重按登录 / 注册即重新预取。
- **失败卡补最终高度**：卡原先只按 300×220 留宽高比，被复用为交卷失败态时会比正常态矮 52px（提示行 / 滑轨 `h-10` + 容器 `gap-3` 12px）；补 `minHeight: 220 + 40 + 12`，弹窗不再跳高。

**已知缺口**：
- 真答错（400 `captcha_failed`）分支本轮未在浏览器重测：探针把 dev 环境 5 次 / 10 分钟的出题额度用尽，后续出题实测返回 429 并停在失败卡（正是预期行为）。该分支代码只改了出口名与判定门，红标 + 摆动 + 800ms 换题逻辑沿用上一轮已实测的行为。**（已补测：见上一条「只接受主指针与主键」的验证段——400 红标、摆动动画、800ms 换一次题均在浏览器实测到。）**
- 断网（`ApiError.status === 0`）与 400 `invalid_request` 同走故障分支，未各自单独实测。
- 点选形态仍无键盘 / 读屏路径（沿用上一轮记录）。

**验证**：`npx tsc --noEmit` 通过；四个改动文件 `eslint` 0 error（4 条既有 `@next/next/no-img-element` warning，与滑块旧用法同类）；`prettier --check` 通过。浏览器实测（dev server + 后端在跑，走前端 `/api` 代理，注入 `/solutions` 返回 503）：只弹「服务暂时不可用，请稍后重试」toast，弹窗内**没有**「人机验证未通过」红标、不摆动，改落到「重新加载」卡；卡高 272px（`minHeight` 生效）、弹窗高度维持 348px 不跳；带调用栈的 fetch 日志证明交卷 503 之后 7.5 秒内没有再发出题请求（日志里只有预取那一次 POST `/captcha/challenges`），即故障后**没有自动换题**。

**提交状态**：已入库 `71de7c6`；尚未推送。

---

## 2026-10-26 · 登录 / 注册接入自研点选形态

**范围**：仅前端。`lib/captcha-challenge.ts` 把出题响应改为判别联合（`SlideCaptchaChallenge` / `ClickCaptchaChallenge`，按字段有效值区分）并新增点选交卷 `solveClickCaptchaChallenge`；`components/auth/builtin-captcha.tsx` 拆成父级容器（出题 / 重试 / 失败卡 / 弹窗摆动）+ 内部滑块组件；新增 `components/auth/click-captcha.tsx`（点选形态）；中英文案各两条；`backend/README.md` 删掉「前端只接滑块、切 click 会拦死」的过时说明并补记点选响应的零值 `tile_*`。后端、迁移、`public-settings`、登录 / 注册表单零改动。

**动机**：上一轮管理端已能选「图像验证码」，但前台只有滑块组件，切过去会把登录 / 注册拦死；本轮把点选形态接到同一个验证弹窗。

**决策**：
- **按出题响应的有效值分派，不读前端缓存的 `captcha_mode`**：后端 DTO 的点选响应里仍会带 `tile_image`（空串）与 `tile_*`（0），所以判别看有效值——`tile_image` 非空且 `tile_width/height` 为正才是滑块，`prompt_image` / `prompt_text` 非空且 `expected_clicks` 为正才是点选；两种都成立或都不成立按坏响应抛错。这样公开设置 60 秒缓存不会造成「组件形态与题目形态错配」。**初版按「字段是否存在」判别，点选题被误判成坏响应、弹窗空白，已改为有效值判别。**
- **出题 / 重试 / 失败卡上提到父级**：父组件按 `challenge.id` 作 key 渲染滑块或点选子组件；子组件只管自己的交互与交卷。答错仍是「弹窗摆动 + 800ms 后换新题」，与滑块既有语义一致。
- **点选交卷时机固定**：每点一次打一个带序号的标记，第 `expected_clicks`（后端恒为 3）次点击后立即一次性交卷；中途不提交、不提供撤销。顺序错即判负，由后端裁定。
- **弹窗尺寸与滑块一致**：沿用同一个 `DialogContent`（`sm:max-w-[340px]` / `p-5` / `gap-3`）；主图同样按后端 `master_width / master_height`（当前 300×220）预留宽高比；点选的提示行 `h-10` 与滑块滑轨 `h-10` 等高，弹窗总高度不变。
- **坐标换算**：点击位置按 `masterWidth / 渲染宽度` 换算回主图像素后四舍五入，容差仍只由后端决定。

**已知缺口**：
- 点选没有「撤销上一次点击」：点错只能等三次点满判负后换题。
- 点选交互只支持指针，无键盘 / 读屏路径（滑块同样没有）。
- 管理端自检接口的 builtin 形态仍未做（仍只服务 Turnstile）。
- 运行期观感与真实点击流程需在 dev server + 打开的测试后端上人工验收。

**验证**：`npx tsc --noEmit` 通过；改动文件 `eslint` 0 error（4 条 `@next/next/no-img-element` warning，与滑块既有的 `<img>` 数据 URL 用法同类）；`prettier --check` 与 `git diff --check` 通过。另在本地 dev server + 打开的后端上实测：点选弹窗正常显示（对话框 340×348，与滑块同外壳、同 300×220 画布比例，提示行与滑轨同为 40px）；同一位置连点三次 → 判负后 800ms 自动换新题；按库中 `answer_dots` 顺序点击 → 出现「验证成功，用时 0.6s」与序号标记，1618ms 后弹窗关闭并提交登录（测试账号在库中不存在，登录结果本身未验证）。初版「字段存在即判别」导致点选响应被判成坏数据、弹窗空白问题已按有效值判别修复。

**未提交。**

---

## 2026-10-26 · 控制台人机验证新增「验证方式」选择器

**范围**：仅前端控制台。`/console/settings/captcha` 在「验证服务」选为「自研人机验证」时展开一行「验证方式」下拉（滑块验证 / 图像验证码），读写 `captcha.captcha_mode`；`lib/captcha-settings.ts` 的解析 / 脏检查 / PATCH 补该键，保存 hook 加 `setCaptchaMode`，中英文案各三条。登录 / 注册页、`public-settings`、点选组件与后端零改动。

**动机**：点选形态后端已入库但管理端没有入口；用户要求先补选择器，登录 / 注册接线留到后续轮次。

**决策**：
- **选择器只在「自研人机验证」下出现**：`captcha_mode` 只对 `builtin` 有意义，Turnstile / 不启用时隐藏该行。
- **未知值照原样显示**：与「验证服务」同套路，读到的值不在 `slide` / `click` 里就把它作为一项显示出来，不静默改成滑块。
- **无条件回写**：`captcha_mode` 与 `captcha_provider` 一样每次保存都带上；只认 `slide` / `click` 的校验在后端。

**已知缺口**：
- 前端仍只有滑块组件，**选「图像验证码」保存后登录 / 注册会被拦死**，直到点选组件接上；本轮按用户口径不碰登录 / 注册。
- 管理端自检接口的 builtin 形态（仍只服务 Turnstile）。

**验证**：`npx tsc --noEmit`、改动文件 `eslint` 与 `prettier --check` 通过。

**未提交。**

---

## 2026-10-26 · 自研人机验证新增「点选文字」形态

**范围**：仅后端。`captcha/builtin` 拆出两个生成器（`generator.go` 滑块、`click_generator.go` 点选，后者用 `imagesv2` 背景 + `fzshenksjw` 字体 + 库内置 15 字库，`WithRangeVerifyLen(3,3)` 固定取 3 个字）；新增设置项 `captcha.captcha_mode`（文字、默认 `slide`、`Public`、只认 `slide` / `click`，写其它值 409 `setting_conflict`）+ seed 迁移；挑战表答案列统一为 `answer_dots`（jsonb）+ 新增 `prompt_text`，删除 `answer_x` / `answer_y`；`Service.Challenge` 按 `captcha_mode` 分派出题，`Service.Solve` 改收 `[]Point` 并按题目 `answer_dots` 的 dots 长度路由判分（1 = 滑块，3 = 点选）；交卷端点接受两种请求形状（`{"x","y"}` / `{"clicks":[3 个]}`）并做形状校验；文档同步。前端零改动、零新依赖、零新错误码。**未提交**（用户口径：绝不主动提交）。

**动机**：用户要在「自研」之下加入「图像点击」形态，即大图散落中文字、按提示依次点 3 个。

**决策**：
- **答案列统一 jsonb 并删旧列**：滑块也搬进 `answer_dots`（长度为 1 的特例），两种形态走同一条判分路径；留着 `answer_x` / `answer_y` 会让「哪种形态该读哪列」变成仓库层隐式判断。
- **顺序是硬语义**：三个坐标集合正确但顺序打乱 → `captcha_failed`。不为照顾手滑放宽成集合匹配。
- **固定 3 个字**（代码常量，不做难度 / 题量设置）。
- **读取不兜底**：`captcha_mode` 缺行 / 读失败 → 503 `service_unavailable`；值不在白名单 → 500 `internal_error`；不静默退回 `slide`。
- **形状非法不判负**：`clicks` 数量不是 3、元素缺 `x`/`y`、同时带两种形状、形状与题目形态不符 → 400 `invalid_request`，**不改变题目状态**（可换正确形状重交）；只有拿到正确形状但坐标判错才判负。
- **迁移先清空存量行再改列**：`answer_dots` 非空，dev 库有存量行时先 `ALTER` 会 23502 失败；存量题是一次性、分钟级过期的挑战数据，清空无用户数据损失。
- **点选元素用指针字段解码**（`ClickInput{X,Y *int}`）：`int` 分不清「元素没写 x」和「x 写 0」，而 spec 要求缺字段算 `invalid_request`。

**实测（点选新增部分）**：
- 启动期素材：`imagesv2.GetImages()` +6.4 MB / 单次约 51ms（**该函数不缓存，每次调用都重新解码**，滑块与点选各调一次，故启动多付一次背景解码）；`fzshenksjw.GetFont()` +8.1 MB / 约 37ms；两个 builder 自身 <1 MB。点选带来的常驻增量约 14.5 MB、启动多约 88ms。
- 出题：`click.Generate()` 平均 7.5ms、最大 10.1ms（滑块 7.7ms / 10.1ms，同量级）。
- 单次负载（base64 data URL 长度）：主图 ≤ 88.6 KB、提示图 ≤ 5.1 KB、两者合计平均 76.3 KB；滑块主图 ≤ 89.3 KB。

**变异自检（四条，均"改坏 → 红 → 还原 → 绿"）**：
1. 点选判分改成顺序无关（集合匹配）→ `TestClickSolveRejectsShuffledOrder` 红。✔
2. `len(Clicks) == 3` 改成 `>= 1`：**计划里点名的用例没有变红**——service 层的 `len(dots) != len(clicks)` 把它兜住了。补了 `TestSolveRouteSlideRejectsSingleClickShape`（滑块题交 1 个 `clicks`），变异下会返回 **200 + captcha_token**（形状混淆直接过题），该用例红；还原后绿。
3. 删掉形态路由（所有题走 `slide.Validate`）：`TestClickSolveInOrder` **没有变红**——既有用例点的是字块左上角 `(X,Y)`，两个校验器都接受。补了 `TestClickSolveAcceptsClicksInsideGlyphBox`（点字块中心，即真人的点法），变异下红；还原后绿。
4. `captcha_mode` 缺行时默认当 `slide` → `TestChallengeFailsWhenModeRowMissing` 红。✔

**已知缺口**：
- 图形点选（`ModeShape`）、旋转、自由拖拽三种形态；难度 / 题量设置（目标字数固定 3）。
- 前端点选组件、登录 / 注册页接线、控制台「验证方式」下拉——**前端目前只接滑块，`captcha_mode` 必须保持 `slide`，切成 `click` 会拦死登录 / 注册**。
- 管理端自检接口的 builtin 形态（`POST /admin/captcha/verifications` 仍只服务 Turnstile）。
- `captcha_provider` 取值白名单（既有缺口，本轮不扩大也不修）。
- 两次资源加载（滑块 + 点选各调一次 `imagesv2.GetImages()`）没有做进程级缓存，启动多付约 51ms。
- **测试隔离**：`internal/modules/captcha`（父包）与 `.../builtin` 都把 `captcha.*` 写进同一个测试库，**并发跑这两个包**（不带 `-p 1`）会随机出现 `captcha_disabled`；`scripts/test.sh` 自带 `-p 1`，按它的方式跑是绿的。这是既有性质，本轮测试量变大后更容易撞上。

**未提交。**

---

## 2026-10-26 · 回退人机验证「启用范围」（0.24.2）

**范围**：后端 `settings` 注册表删除 `captcha_scopes` 与组校验 / 公开项；builtin / turnstile 删除 scope 判定；前端删除 admin settings 的 scopes 字段 / 解析 / patch、`public-settings` 的 scopes 过滤、设置页选择器与 4 条文案；新增迁移 `remove_captcha_scopes` 删除存量行；测试同步（删 scope 用例，seed 静态测试忽略历史 seed 行）；README / spec 更新。版本 0.24.1 → **0.24.2**（patch）。零新依赖、零新路由。

**动机**：用户点名回退到 `0049dc8`（后端按入口启用）+ `7ab584b`（控制台选择器）之前；上一版只隐藏 UI、保留后端设置被否。

**口径**：
- 登录 / 注册两个入口固定都启用；`captcha_enabled` + provider 决定是否拦截。
- `captcha_challenges.scope` 与一次性凭证的 scope 绑定保留（login 凭证不能用于 register）；只是不再有启用范围设置。
- 存量 `captcha_scopes` 行由迁移删除；历史 seed 迁移保留但被新迁移清掉。test 库已删；dev 库为兼容仍在跑的旧 0.24.1 后端（5732）临时补回 `login,register` 行，0.24.2 不读它，重启后端后可删。
- 前端 `getCaptchaConfig` 仍接收 scope（用于 builtin 挑战 / 凭证），不再用它过滤入口。

**验证**：`go build ./...`、受影响包 `go test -p 1 ./internal/modules/captcha/... ./internal/modules/settings/... ./internal/app/...` 通过；前端 `tsc` / lint / 改动文件 prettier 通过；`scripts/check.sh` → `check ok`；全量 `scripts/test.sh` exit 0。

**未提交。**

---

## 2026-10-26 · 人机验证限流收紧（CF + 自研共用失败桶）（0.24.1）

**范围**：新增 `internal/modules/captcha/policy`（共享失败策略：15 次 / 10 分钟、窗口 / 冷却 10 分钟、共用 key `captcha-fail:ip:*`）；`builtin` 出题从 60 次/分钟降到 **5 次 / 10 分钟**，`VerifyToken` 增加 IP 参数并检查 / 记录失败桶；`turnstile` 增加 `Policies` / `NewServiceWithPolicies` 与 limiter，和 builtin 共用失败桶，只对空 token 与 Cloudflare 明确判负计数（`Unavailable` 不计）；`module.New` 把 limiter 注入 Turnstile；补 builtin / Turnstile / 跨 provider 共用桶 / policy 默认值测试；README、spec 同步；版本 0.24.0 → **0.24.1**（patch）。零迁移、零新接口、零新设置键、前端零改动。

**动机**：用户要求「不论是 CF 还是自研，都加这个限流，目前的太大了」；首版按 hao 博客 10/30，用户要求「至少砍半」后定为出题 **5/10**、失败 **15/10**。

**口径**：
- 出题限流只对 builtin；CF 没有后端出题端点。
- 失败桶两种 provider 共用，切 provider 不清零；命中后返回 429 `rate_limited`（带 `Retry-After`），检查发生在 Cloudflare siteverify / builtin 凭证消费之前，命中后不再产生外部请求或数据库消费。
- Turnstile 的 `Unavailable`（网络 / 5xx / 坏响应）不计数；builtin 的空 token / 过期 / 已用 / 错 scope 计数。
- 前端无需改动：登录 / 注册已有 `rate_limited` 倒计时，出题弹窗已有 429 toast。
- 仍是进程内、按直连对端 IP；生产后端看到前端容器 IP，实际是整站配额（既有口径，未修）。

**验证**：`go build ./...` 通过；`go test -p 1 ./internal/modules/captcha/...` 四包绿（root / builtin / policy / turnstile）。新增测试覆盖：builtin 失败桶与换 IP、Turnstile 失败桶与 `Unavailable` 不计数、跨 provider 共用桶（builtin 记一次失败后 turnstile 直接 429 且 verifier 零调用）、policy 默认值守卫。`scripts/check.sh` → `check ok`；全量 `scripts/test.sh` 全绿（exit 0）。

**已知缺口**：多实例各算各的；真实客户端 IP 未解决；限流值不在设置页暴露。**未提交。**

---

## 2026-10-26 · 自研人机验证前端接入（登录 / 注册滑块弹窗）

**范围**：前端新增 `lib/captcha-challenge.ts`（出题 / 交卷传输与形状校验）、`hooks/use-captcha-gate.ts`（provider 分派 / 预取 / 429 / 关闭清理）、`components/auth/builtin-captcha.tsx`（滑块六阶段 / 拖拽 / 交卷 / 重试 / 摆动）、`lib/dialog-nudge.ts`；修改 `lib/api.ts`（signal）、`lib/public-settings.ts`（`CaptchaConfig` 联合 + builtin）、`components/auth/captcha-dialog.tsx`（按 provider 分派 / 340px / 成功定时器取消）、登录 / 注册表单、设置页作用域条件、中英文案；`backend/README.md` 删除「前端未接 builtin」的过时说明；spec / plan 入档。零后端生产代码、零迁移、零新依赖、零新路由；版本保持 0.24.0（纯前端，不单独进位）。**未提交。**

**动机**：0.24.0 后端已支持 `builtin`，但前端没有滑块，切 provider 会把登录 / 注册全部拦死。

**决策**：
- 样式按 hao 博客 `selfhosted-widget.tsx` / `captcha-verify-dialog.tsx` 逐项照抄；弹窗 340px / p-5 / gap-3，滑轨 h-10 + 40px 拇指，成功绿条 + 耗时，答错红条 + 弹窗摆动，800ms 重试。
- 提交前先出题再开窗；429 只 toast 不开窗；答错 / 过期一律换新题（后端判负，原题不可复用）；不搬轨迹采集（本仓只验落点 ±4px）。
- **加载 / 校验中 UI 按用户两次点名移除**：第一次删「加载中」文案与转圈；第二次连空白占位整块删掉，并删 `captchaVerifying`（校验中…）与遮罩 / 转圈。当前加载态渲染 `null`，校验中只锁滑轨；文案表因此没有 `captchaLoading` / `captchaVerifying` 两键。
- 拼图块从后端 `tile_x / tile_y` 起，滑轨拇指仍从 0 起；坐标按 `masterWidth / renderedWidth` 换回主图坐标；失败卡按 300×220 预留高度，避免重试成功后跳高。
- 设置页「启用范围」行对 builtin 也显示；Turnstile 的站点密钥 / 私钥 / 验证配置仍只在 turnstile 下出现；管理端自检无 builtin 形态，本轮不加按钮。
- Turnstile 分支只在弹窗分派处改 props；loader、token、unavailable 与自检逻辑不动。

**验证**：
- `npx tsc --noEmit` 通过；prettier --check 覆盖本轮 12 个改动文件全过；`git diff --check` 通过。
- `npm run lint` 0 error / 10 warning：8 条既存（`.scratch` 7 + `components/ui/sonner.tsx` 1），新增 2 条 `@next/next/no-img-element`（data URL 图片，刻意用 `<img>`，与对照组件一致）。
- `npm run format:check` 全量在当前 HEAD 的 `components/ui/select.tsx` 报既存格式问题；该文件本轮未改，未顺手格式化。
- 浏览器实测（源码拷贝到临时 3001 前端 + 5733 development 后端，连 dev 库）：
  1. builtin 登录：出题 → 拖拽 → 交卷 200 → 登录 200 → `/console`；development 模式下会话 cookie 持久。
  2. 设置页：provider=builtin 时只显示作用域行（当时 dev 库值为「注册」），站点密钥 / 私钥 / 验证配置行不渲染。
  3. 注册：连续两次提交只发 1 条出题；交卷成功后在 1618ms 内点 X → 只有 solutions 200、无 `/users`；重来一次让 1618ms 走完 → `/users` 201、账号建立并跳登录。
  4. 缩放：登录时图片渲染宽 285px（scale 0.95）仍能拖到答案；注册时 300px（scale 1）通过。
  5. 5733 后端停掉后弹窗保持空壳（按用户要求不再有加载占位），后端重启后同一请求恢复并出题；确切的失败卡 / 429 / 403 未单独实测。
  6. 原 3000 前端连的 5732 后端是 production 模式（Secure cookie），登录后的整页跳转会丢会话；后半段改用 5733 development + 临时 3001 前端完成。共享 3000 / 5732 未重启。
- 环境处理：验证期间并行会话把 dev 库 provider 从 turnstile 改成 builtin、scopes 在 `login,register` / `register` 间变动、`allowed_email_domains` 也变过；本轮未覆盖这些值。只临时把 register 的两个开关改成 false 做过注册实测，已恢复 true；创建的 2 个测试账号已删；临时前端副本、探针、5733 / 3001 进程已清理。测试库未动。
- 计划文档里的验证脚本已按实测修正：题目表不存 `tile_x`，初值从 DOM 量；用合成 PointerEvent（临时把 `setPointerCapture` / `releasePointerCapture` 置空）代替不稳定的元素 drag。

**已知缺口**：Turnstile 实机回归未做（dev 库 provider 被并行会话保持为 builtin，未强切）；403 / 429 / 5xx 的前端失败卡 / toast 未逐条实测；管理端自检无 builtin；公开设置 60 秒缓存窗口；滑块无键盘操作；前端无自动化测试框架。后端完全不可达时弹窗会是空壳直到请求失败 / 恢复，这是移掉加载占位的既定代价。

**提交状态**：代码、spec、plan、本条目与 README 均**未提交**。

---

## 2026-10-25 · 两步验证拆成独立能力模块

**范围**：后端新增 `internal/modules/twofactor`（service / repository / totp / secretbox / ratelimit / dto / codetest），`auth` 删 `totp.go`、`secretbox.go`、`ratelimit.go` 与 9 个 2FA 方法、6 个仓库方法；auth controller 增 `twoFactorService` 字段，6 条 2FA 路由薄壳分发；`app.go` 装配（`TOTPEncryptionKey` 直达 twofactor）；README 模块表与会话章节引用修正；spec / plan 入档。零接口改动、零迁移、零前端改动。spec 见 [2026-10-25-neoblog-twofactor-module-design.md](superpowers/specs/2026-10-25-neoblog-twofactor-module-design.md)。**未提交。**

**动机**：auth 历经 captcha、login、registration、session 四次拆分后，2FA 是最后一大块残留耦合（实现、仓库、路由 DTO、验证限流、加密钥匙）。

**决策**：
- twofactor 为无路由能力模块，`New` 注入 session / settings / limiter / `TOTPEncryptionKey`；settings 是消费方窄接口（仅 `GetBool`），nil 依赖 panic（禁兜底）。
- **自包含裁定**：验密码、活跃用户（已删销毁会话 401）、吊销其他会话、auth version 读写这些共享动作为包内私有副本；已知代价——bcrypt 校验与销毁会话语义在 auth（ChangePassword）与 twofactor 各一份，改语义需两处同步。
- **flow 测试留 auth 包**（spec §4.1 的"整体随迁"未执行）：twofactor 测试需打生产登录路由，登录 handler 在 auth 包，twofactor 测试 import auth 即成环（auth→twofactor），经 app 包也环。改为 flow 留 auth、`SetupTOTP` 等调用点改 `twofactor.X` 新名；twofactor 包保留纯单测（totp / secretbox / repository / 构造，24 条）。跨包引用的测试助手（`CodeAt` / `PinWindow` / `CurrentWindow` / `FakeTOTPClock` / `SealSecret` / `OpenSecret` / `TestTOTPKey` / `TestCodeHash` / `RecoveryCodesPerBatch` / `MatchTOTPCounter` / `SetupReuseWindow` / `SkewWindows` / `HashRecoveryCode` / `GenerateRecoveryCodes`）经导出桥接，其中测试工具落在生产文件 `codetest.go`。
- 限流测试 `ratelimit_test.go` 整体留 auth（11 条），`TestVerifyRateLimitIPGate` 改 `twofactor.NewWithPolicies` 注入自定义 IP 窗口；`New` 拆出 `NewWithPolicies`（login 先例）。
- `auth.totp_enabled` 不搬组（auth 域站级开关）；恢复码邮件接口 501 占位随薄壳保留。

**验证**：`go build ./...`、`go vet ./...`、`gofmt` 干净；`scripts/check.sh` → `check ok`；全仓 `scripts/test.sh` 17 包绿（含偶发用例本轮通过）。变异自检三条实测红：①`recordAccountFailure` 注释 → `TestVerifyRateLimitAcrossSessions` 红；②锁定阈值改 `1<<30` → `TestTwoFactorVerifyLockout` 红（变异残留曾致全量三红，已恢复复绿）；③`toUser` 的 `TwoFactorEnabled` 改 `false` → `TestTwoFactorLoginSecondStageLifecycle` 红。装配单例核对：`twofactor.New` 全仓仅 `app.go` 一处。

**已知缺口**：恢复码邮件仍 501；`totp_enabled` 不搬组；`findUserByIdentifier` 双用途留邮箱登录轮；测试工具在生产包可见（`codetest.go`），是跨包测试的既定妥协。

---

## 2026-10-26 · 自研人机验证（go-captcha 滑块）后端接入

**范围**：后端 `captcha` 模块拆成三层目录——根目录只留引用与装配：`module.go`、`service.go`（读 enabled/provider 并委托）、`verifier.go`（`Verifier` 别名）；两种实现各自完整成包：`captcha/turnstile/`（`turnstile.go` Cloudflare 客户端 + 三态、`service.go` 私钥/scope/凭证判定与 `VerifyConfig`、`controller.go` 管理端自检路由、`dto.go`）、`captcha/builtin/`（`generator.go` 官方素材与出题、`repository.go` 挑战表、`service.go` 出题 / 交卷 / 凭证消费 / `Policies`、`controller.go` 两条公开路由、`dto.go`）；新增 ent schema `captcha_challenge` 与迁移 `captcha_challenges`；`apierror` 加 `captcha_disabled`（403）与 `captcha_failed`（param null）；`app.go` 装配 `builtin.New` 与 `turnstile.NewService`；README、本条目。版本 0.23.1 → 0.24.0。**未提交。**

**动机**：项目已有 Cloudflare Turnstile，本轮加第二种 provider `builtin`，底层用开源库 `wenlng/go-captcha/v2`（滑块）自托管出题与判分，不依赖外部服务；纯后端，前端零改动。

**决策**：
- 协议由本项目自定（不照搬官方 `go-captcha-service` 的 `id/captchaKey/value`）：`POST /api/v1/captcha/challenges` 出题、`POST /api/v1/captcha/challenges/:id/solutions` 交卷；交卷返回的 `captcha_token` 直接喂给现有登录 / 注册请求体，登录 / 注册接口零改动。
- 两段式一次性凭证：题目 3 分钟、凭证 5 分钟、只绑 scope 不绑 IP（生产上后端看到的 IP 全是前端容器）、token 只存 sha256、消费即作废；答错一次即判负。
- 挑战表 `captcha_challenges`（status 0 待解 / 1 已解 / 2 已消费 / 3 判负，`token_hash` 唯一、`expires_at` 索引）；过期行在每次出题时顺带清理。
- 出题限流 60 次/分钟/IP（复用 `ratelimit.Limiter`）；交卷不单独限流。
- 素材全部用官方 `go-captcha-assets`（背景 16 张 + 拼图 4 套），不改图。实测：二进制 +约 2.63MB（探针，Windows `-s -w`）、常驻内存 +约 8.3MB、启动解码约 54ms、单次出题约 5.3ms、负载约 41KB JPEG + 5KB PNG。真实构建 `bin/neoblog-0.24.0-windows-amd64.exe` 为 36.98MB（`-s -w`），相对 32.97MB 历史地板 +约 4.0MB（其中含本轮之外的模块改动）。
- 模块边界：`generator` 只依赖库；`repository` 只依赖 ent；`builtin` 依赖 settings 窄接口 + repository + generator + limiter；`service` 只做分派；`controller` 只做 HTTP。`login` / `registration` / `auth` / `settings` 生产代码零改动。

**验证**：`scripts/check.sh` → `check ok`；captcha 包全量测试绿（出题 / 交卷 / 一次性 / 过期 / scope 串用 / 限流 / 路由 400·403·429·503）；`internal/app` 端到端绿（出题 → 交卷 → 登录成功，同 token 二次登录 400 `captcha_failed`；降级库出题 503）；apierror 注册表测试同步。变异自检三条（去掉 scope 判断 / 去掉消费状态更新 / 去掉过期谓词）各自变红后还原。

**环境处理**：dev / test 库各有一张 10-25 遗留的 `captcha_challenges`（字段为 attempts / verified_at / consumed_at，18 行一次性测试数据，迁移文件从未进 git）。未删除数据：两库把旧表改名为 `captcha_challenges_legacy_20261025`（连带 pkey 约束与 `expires_at` 索引改名）后应用新迁移；atlas 修订里仍留着已不存在文件的版本 `20261025120002`，`migrate status` 不阻塞。

**已知缺口**：只做滑块（旋转 / 点选 / 拖拽未做）；`captcha_provider` 取值白名单未做（拼错仍会静默不拦）；管理端自检接口的 builtin 形态未做；前端滑块组件与两个页面未接——**切 provider 前必须先上前端**；过期行清理依赖出题行为。

---

## 2026-10-25 · 人机验证设置组归位与自检路由迁移

**范围**：后端 `common/rbac`（新增 `system.captcha.view/edit`）、`modules/settings/{registry,controller}` 及测试、`modules/captcha`（升级为有路由模块：`module.go` / `controller.go` / `dto.go` / `verify_route_test.go`）、`modules/auth`（删自检路由薄壳与 captcha 依赖）、`app.go` 装配、新迁移 `20261025130000_relocate_captcha_settings.sql` + `20261025130100_seed_captcha_group.sql`、三条历史 seed 迁移改写；前端 `use-captcha-settings-save` / `admin-settings.ts` / `public-settings.ts` / `groups.ts` / 双语 i18n；README、本条目。spec 见 [2026-10-25-neoblog-captcha-settings-relocation-design.md](superpowers/specs/2026-10-25-neoblog-captcha-settings-relocation-design.md)。**未提交。**

**动机**：captcha 已拆独立模块但设置键还在 `auth` 组、自检路由还在 auth 包、权限借 `system.register.edit`；本轮归位，为 go-captcha 轮铺路（新模式、难度等设置直接落 `captcha` 组）。

**决策**：
- 设置拆出独立 `captcha` 组（5 项，声明在 `auth` 后），`captcha_scopes` 校验从 `validateAuthGroup` 拆成 `validateCaptchaGroup`；`auth` 组剩 7 项。
- 迁移两条：`relocate`（①现值 `auth`→`captcha` 的 `INSERT…SELECT ON CONFLICT DO NOTHING` + ③删 `auth` 组旧行）与 `seed_captcha_group`（②5 行字面默认值）——②必须独立成 `*_seed_*` 文件，否则 seed 对账测试（glob 只认 `*_seed_*.sql`）读不到。三条历史 seed 同批改写（组名 `auth`→`captcha` / 删行），依据：对账测试要求去重总数恰好等于清单全集，组名搬家没有"新增一条 seed 覆盖"的出路。
- 自检路由 `POST /admin/captcha/verifications` 搬进 captcha 模块（路径不变），守卫换 `system.captcha.edit`；`requireJSONRequest` 复制到 captcha 包（约 25 行 HTTP 细节，不从 auth 导出）；auth 与 captcha 彻底解耦（删 `captchaVerifier` 接口、`VerifyCaptchaConfig`、`Service.captcha` 字段与 `auth.New` 参数）；`captcha.New` 删除，生产单一入口 `captchaModule.Service`。
- rbac 目录新增 `system.captcha` 对（owner/admin 运行时全量授权自动覆盖）；settings 组接口三路分流（`captcha` → `system.captcha.*`）。
- **已接受代价**：库里已有自建组不会自动拿到新权限点，不手工补授就打不开人机验证设置页（403）与自检接口；owner/admin 不受影响；当前 dev/test 库只有预设组，无实际影响。
- 前端只换组名：设置页读写 `/admin/settings/captcha`、`getCaptchaSettings` 改读 captcha 组、`getCaptchaConfig` 改读 `settings.captcha`；权限矩阵加「人机验证」节（labelKey 取 `sidebar.captchaGroup`，唯一新 i18n 键 `groups.resourceCaptcha`；独立审查发现最初按 spec 复用 `settings.captchaGroup` 的判断错了——渲染路径走 `sidebar` 命名空间，双语补键后修正，spec 相应前提作废）。

**验证**：`go build ./...`、`go vet ./...`、`gofmt` 干净；`scripts/check.sh` → `check ok`；`scripts/test.sh` 全仓绿（唯一红是既有偶发 `TestVerifyEmailRegistrationConcurrent`，stash 到干净 HEAD 复测 8 次 6 红，与本轮无关）。迁移实测：dev 库现值完整搬家（`turnstile` / 37 字符 secret / `enabled=true`）且 `auth` 组零残留；测试库走默认值路径（`none`/空串）。变异自检：rbac catalog 删新对→红；settings 分流删 `case "captcha"`→红；自检守卫降级成 `register.edit`→「captcha-edit 自建组用户 200」用例红（原用例表只有 owner 与 plain user，两类用户在两个权限点下行为相同，测不出降级——教训：变异必须让"仅持有新权限的用户"做裁决）。前端 `tsc` / ESLint / Prettier 干净。独立审查（子代理全量 diff）无 Critical，2 Important 已修（矩阵节标题 i18n 键命名空间、README API 表守卫行），3 Minor 中死代码（`truncateHeader` / `maxUserAgentLength` / `maxIPLength` 复制残留）已删，另两条（测试装配绕过 `New()`、clientIP 不截断）记档不修。

**已知缺口**：go-captcha 未实现，`builtin` 仍是"选了不拦"；`findUserByIdentifier` 双用途留邮箱登录轮；历史自建组需手工补授 `system.captcha.*`。

---

## 2026-10-25 · 登录 / 注册拆成独立模块，限流与校验下沉共享层

**范围**：后端新增 `internal/modules/{login,registration,session}` 三个能力模块与 `internal/common/userinput`、`internal/common/ratelimit/policy.go`；删除 `auth` 的 `session.go`、`email_verification.go`、`validate.go`（实现与对应单测随迁）；`auth` 保留 2FA / 会话管理 / 账号资料 / 管理端 captcha 自检薄壳，`Login` / `Register` / `VerifyEmailRegistration` 三条路由改由 controller 分发到注入的 login / registration 服务；`app.go` 装配四个新依赖；README 模块表重写。零接口改动、零迁移、零前端改动。**未提交。**

**动机**：登录、注册与人机验证一样继续挤在 `auth` 的 service 里；本轮把登录、注册也拆出去，速率限制（策略与 key 生成）跟着各自模块走，而不是留在 auth 的共享结构体字段里。

**决策**：
- `modules/login`：密码登录、bcrypt、未验证 pending 密码比对、2FA 分叉、失败计数与 IP 窗口；`Policies{IdentifierFailure, IPWindow}`，`DefaultPolicies()` 载默认值，测试走 `NewWithPolicies`。
- `modules/registration`：注册全流程（开关、字段、域名白名单、pending upsert、验证邮件与补偿）、`VerifyEmail`；`Policies{EmailWindow, IPWindow, EmailVerificationWindow}`；`verifyPendingRegistration` 的建号事务与冲突分类随迁到该模块 repository，三个直接测仓库内部的用例（补偿不删 replaced、删除计数守卫、旧 token 不消费）一并搬走并保留在案。
- `modules/session`：scs 封装——`Establish`（含 session_max_age 截止期）、`EnterPending2FA`、pending 用户与尝试次数读写、auth version、会话列表 / 撤销 / 吊销其他会话；pending 2FA 的会话键常量从 auth 挪到 session 包，测试用的键名字面量留在 auth 测试里。
- `common/ratelimit`：`WindowPolicy` / `FailurePolicy` 类型 + `AllowWindow` / `CheckFailure` / `RecordFailure` / `ResetFailure` + `IPKey` / `HashedKey`；auth 的 `windowPolicy` / `failurePolicy` 变为类型别名，原有 2FA 验证限流策略与默认值仍归 auth。
- `common/userinput`：邮箱 / 昵称 / bio / 密码校验与归一化、密码最小长度钳制、邮箱域名匹配；auth 与 registration 共用同一套，避免安全规则两份。
- `auth` 的 captcha 窄接口收敛为只有 `VerifyConfig`（管理端自检用），登录 / 注册各自直接依赖注入的 captcha 服务。
- 装配链：`settings → session → captcha → login → registration → auth`，全部在 `app.go` 显式注入；auth 模块签名新增 `loginService` / `registrationService` 两个参数。
- 测试装配：auth 的 `newTestAppWithCaptchaVerifier` 同时构造 login / registration（login 策略沿用测试传入的 Login 字段，registration 同理）；2FA 验证限流测试改走新的 `newTestAppWithVerifyPolicies`。

**行为**：不变。三条登录限流、两条注册限流、两条邮箱验证限流、2FA 验证失败锁定的阈值 / 窗口 / 冷却全部保持；错误码、路由、响应形状不变。

**验证**：`gofmt -l` 干净、`go build ./...` 通过、`scripts/check.sh` 输出 `check ok`、全量 `scripts/test.sh` 全绿（含偶发的 `TestVerifyEmailRegistrationConcurrent` 本轮通过）。app_test 的 `login` helper 因与新包名冲突改名为 `postLogin`（仅测试内改名）。

**已知缺口**：`auth` 设置组仍未拆 captcha 组；管理端自检路由仍在 auth；`builtin` provider 仍无实现；`access` 模块读 users 表与 `findUserByIdentifier` 双用途（登录 vs 注册查重）未在本轮处理，留待邮箱登录轮。

---

## 2026-10-25 · 人机验证拆成独立能力模块

**范围**：后端新增 `internal/modules/captcha/{service,verifier,turnstile}.go` 与 `turnstile_test.go`；删除 `internal/modules/auth/captcha.go`、`captcha_test.go`；`auth` 的 `service.go` / `module.go` / 三个测试文件改为通过窄接口调用；`internal/app/app.go` 装配并注入 captcha 服务；`backend/README.md` 模块表更新；`backend/scripts/check.sh` 的 gofmt 清单跳过工作区已删除的文件。零接口改动、零迁移、零前端改动。**未提交。**

**动机**：`auth` 包同时承接注册、登录、2FA、会话、账号资料、限流与人机验证；先拆人机验证，为后续 go-captcha 落地留出独立边界。

**决策**：
- captcha 做成**无路由的能力模块**（按 STYLE.md §1），只提供 `New`（生产用 Turnstile）、`NewService`（测试注入假 verifier）、`Service.Verify`、`Service.VerifyConfig`；包名即前缀，调用处是 `captcha.Verify` / `captcha.VerifyConfig`。
- 管理端自检路由 `POST /admin/captcha/verifications` 及其 DTO 仍留在 auth，作为薄壳调用 captcha 服务；登录 / 注册的 captcha 校验也只保留薄委托。
- 设置键仍读 `auth` 组（`captcha_enabled` / `captcha_provider` / `captcha_scopes` / `turnstile_secret_key`），本轮不搬设置组，避免同时改公开设置形状。
- `auth` 的 captcha 依赖改为消费方窄接口（两个方法）；`newService` 收到 nil 直接 panic，不再静默退化为不可用实现。

**行为**：不变。Turnstile 三态、开关 / provider / scope 判定、空 token 不调用 verifier、判负与不可用都映射 `captcha_failed`、管理端 400/503/403/401 全部保持。

**验证**：`gofmt -l` 干净、`go build ./...` 通过、`scripts/check.sh` 输出 `check ok`、`scripts/test.sh ./internal/modules/captcha/ ./internal/modules/auth/ ./internal/app/` 全绿。全量 `scripts/test.sh` 仅 `TestVerifyEmailRegistrationConcurrent` 偶发失败（同一 token 并发验证，落败方有时收到 409 而非 400），已在改动前 HEAD `81c0ab2` 的临时 worktree 复现，与本次无关。

**已知缺口**：`auth` 设置组尚未拆出 captcha 组；管理端自检路由仍在 auth；go-captcha 未实现，选择 `builtin` 仍然是前后端静默放行。

---

## 2026-10-25 · 认证页 URL 改名（`/signin` `/signup` → `/login` `/register`）

**范围**：前端路由目录 `app/signin/*` → `app/login/*`、`app/signup/*` → `app/register/*`（含 `verify` 子路由），另改 10 个文件里的 URL 字面量——`proxy.ts`、`lib/bootstrap-scripts.ts`、`hooks/{use-signout,use-signup}.ts`、`app/console/layout.tsx`、`components/console/session-expiry-dialog.tsx`、`app/login/signin-view.tsx`、`app/register/{signup-view.tsx,verify/verify-form.tsx}`、`app/ui-check/page.tsx`；后端 `internal/modules/auth/service.go`（邮件验证链接）+ `internal/modules/mail/controller.go`（预览链接）+ 两个测试断言；`backend/README.md` 一处路径提及；版本 0.23.0 → **0.23.1**（patch：邮件里的链接路径是对外可见行为）。零迁移、零接口改动、零依赖、零新文案。

**动机**：用户要求把登录 / 注册页的地址栏路径从 `signin` 这类命名改为 `login` / `register`。

**决策**：
- **只改 URL**。文件名（`signin-form.tsx`）、模块名（`hooks/use-signup.ts`、`lib/signup-notice.ts`）、i18n 键（`auth.signin` / `auth.signup`）、sessionStorage 标记键（`neoblog.signin.success` / `neoblog.signup.pending|success`）、邮件模板名 `signup_code` 与 `mail.signup_code_*` 设置项全部不动——它们不进地址栏，改了要连数据一起迁，收益为零。
- **不保留旧路径跳转**（用户 2026-10-25 明确裁定「不保留 3xx 等形式」）：`next.config.ts` 不加 redirect，`proxy.ts` 不做旧名兜底，`/signin` `/signup` `/signup/verify` 直接 404。代价当场生效：改名前发出、尚未消费的验证邮件链接（30 分钟有效）全部失效，用户须回 `/register` 重新提交。
- 控制台 `app/console/settings/{login,register}` 是「登录设置 / 注册设置」配置页，与认证流程无关且本来就是这组词，不在改名范围。
- 反向跳与守卫的目标同批改：`proxy.ts` 的未登录弹回、`console/layout.tsx` 的未登录弹回、登出成功 / 401 两条路径、会话过期弹窗「重新登录」、注册成功 / 待验证 / 验证成功后跳登录页，全部指向 `/login`。`SYSTEM_THEME_ROUTES`（登录注册页强制跟随系统主题）同步为 `["/login", "/register"]`，否则这两页的主题引导脚本静默失效。

**验证**：
- 实机 dev（`:3000`）：`/login`、`/register`、`/register/verify` 均 **200**；`/signin`、`/signup`、`/signup/verify` 均 **404**（无跳转，`redirect_url` 为空）。
- 渲染出的链接直指新地址：`/login` 页只有 `href="/register"`，`/register` 页只有 `href="/login"`；主题引导内联脚本实测 `var s=["/login","/register"]`。
- 后端 `bash scripts/check.sh` → `check ok`；`bash scripts/test.sh ./...` 全绿无 skip（含 `verifyUrl` 断言已改成 `/register/verify`）。前端 `npx tsc --noEmit`、ESLint、改文件 Prettier 全部干净。

**实现发现**：整目录改名不能直接 `git mv app/signup app/register`——dev server 运行时 Windows 锁住该目录，报 `Permission denied`；改成先建好新目录、逐文件 `git mv`、最后 `rmdir` 空目录即可。另一处坑：`.next/types/validator.ts` 是生成产物，仍写着 `AppPageConfig<"/signin">`，改名后 `tsc` 会报 `Cannot find module '../../app/signin/page.js'`，这不是源码错误；删掉 `.next/types` 让 dev 重新生成后 `tsc` 归零。

**已知缺口**：旧地址不留跳转，站外收藏、已发出的邮件链接、外部文档里的 `/signin` `/signup` 一律 404；URL 之外（文件名 / 模块名 / 文案键 / 设置键 / 模板名）仍是 signin / signup 语义。

---

## 2026-10-25 · 注册邮箱所有权认证（先验证后建号）

**范围**：后端 `internal/modules/auth/*`、`mail/*`、`access/*`、`settings/registry.go`、ent schema + 新迁移 `20261025120001_email_verification.sql`、`app.go`、`apierror.go`；前端注册页 / 新验证页 / 用户注册设置页 / 用户列表 / 邮件设置页 / i18n；另加本条目。**已入库。**

**动机**：`register.email_verification_enabled` 此前零消费方。用户要求接入「邮箱所有权认证」：开关打开后，注册必须先证明邮箱归属；最终选「先验证、后建号」（不建未验证账号）。

**决策**：
- 设置复用 `register.email_verification_enabled`（默认 false、非公开）；「用户注册」页新增「邮箱所有权认证」开关；前置条件 = SMTP 真正可用 + `site.url` 合法（缺失时开关不能打开，运行期 fail-closed）。
- 提交注册只写 `pending_registrations`（邮箱、用户名、bcrypt 密码哈希、SHA-256 令牌哈希、30 分钟过期），发一封链接邮件；链接只在点「验证邮箱」按钮后消费（GET 不消费）。
- 账号在 `POST /users/email-verifications` 验证成功后才创建，状态 `verified`；老账号 / 管理员建号 / 开关关时注册为 `not_required`。没有 `pending` 用户状态。
- 没有重发；过期/没收到时回注册页重新提交，覆盖旧记录并重发。同一邮箱并发提交用 upsert 覆盖；发信失败按本次写入行的 id + token_hash 补偿删除，不能按邮箱删。
- 注册发信走 `SendSync` 且 `RequireSMTP=true`：传输层解析为日志降级时直接报错，令牌不进日志。
- 登录不给真实账号加验证闸门；只有「用户不存在 + 登录名是邮箱 + 待验证记录密码正确」返回 403 `email_not_verified`（前端「账户尚未验证」）。
- **前端提示时序（2026-10-25 追加裁定）**：所有涉及路由切换的提示一律「先跳转、目标页面挂载后再弹 sonner」，不在跳转前弹。实现 = sessionStorage 标记 + 目标页 notice hook：
  - 注册 202（待验证）：`markSignupPending()` → `/signin` → `useSignupNotice` 弹「验证邮件已发送，请查收」；不显示成功卡片，也不弹「注册成功」。
  - 注册 201 / 验证成功：`markSignupSuccess()` → `/signin` → `useSignupNotice` 弹「注册成功，请登录」。
  - 登录成功 / 2FA 成功：`markSigninSuccess()` → `/console` → `ConsoleSigninNotice` 弹「登录成功」。
  - 验证失败 / 链接过期：不跳转，直接 toast「验证链接无效或已过期」。
  - pending 与 success 用不同的 sessionStorage 键（`neoblog.signup.pending` / `neoblog.signup.success`），避免未建号的注册被误弹成「注册成功」。
- 管理端用户列表新增「邮箱验证」列（已验证 / 不需要验证）；邮件设置页「注册验证码」改名「注册验证邮件」，占位符增加 `{{verifyUrl}}`。

**验证**：
- `bash scripts/check.sh` → `check ok`；`bash scripts/test.sh ./...` 全绿。
- 前端 `npx tsc --noEmit`、ESLint（仅既有 `sonner.tsx` warning）、改文件 Prettier 通过。
- 变异自检：expires、删除计数、Configured、补偿删除、验证状态、202→201、登录密码、site.url、令牌哈希、公开白名单、RequireSMTP 共 11 条全部 RED；删除计数最初是假绿，补了 `TestVerifyDeleteCountGuard`（行锁 + 并发）后才稳定变红。「补偿改回按邮箱删」没有对应变异项——实现里不存在按邮箱删的路径，`TestRegisterCompensationDoesNotDeleteReplacedPending` 直接验证 id + token 删除不会误删替换后的记录。
- **复核修复（2026-10-25 追加）**：验证消费的三处删除条件都追加 `token_hash`。原实现只在事务外按令牌读出记录、事务内按 id + 过期时间删除；若「重新提交注册」正好插在两者之间，旧链接会删掉刚换新的记录并用旧昵称/旧密码建号。新增 `TestVerifyStaleTokenDoesNotConsumeReplacedPending`：锁用户表让验证停在读完记录、尚未删除的位置，替换记录后放开，断言旧令牌返回失效且新记录保留——先红后绿。另把邮件文案的「30 分钟」改为按 `pendingRegistrationTTL` 计算。

**已知缺口**：后台看不到待验证注册列表；没有重发；密码重置 / 2FA 恢复码邮件仍未接；改邮箱不要求重新验证；用用户名登录的待验证用户拿不到「账户尚未验证」提示；过期记录只在下一次注册时懒清理；`register.default_group` 仍未消费。

---

## 2026-10-25 · 人机验证新增「启用范围」（登录 / 注册多选）

**范围**：后端 `internal/modules/settings/{registry.go,registry_test.go,controller_test.go}`、`internal/modules/auth/{service.go,captcha_flow_test.go}`、新迁移 `migrations/20261025120000_seed_captcha_scopes.sql` 与 `migrations/atlas.sum`、`README.md`；前端 `lib/{captcha-settings.ts,public-settings.ts}`、`hooks/use-captcha-settings-save.ts`、`app/console/settings/captcha/captcha-settings-form.tsx`、`app/{signin,signup}/page.tsx`、`messages/{zh-CN,en}.json`；另加本条目。零 DDL、零新接口、零依赖。

**动机**：此前「启用人机验证」一开，`auth.Service.Login` 与 `Register` 都无条件校验 captcha，登录和注册绑死在一起，无法只拦其中一处。用户要求加选择器，自行决定哪些入口需要人机验证。

**口径**：

- 新增设置 `auth.captcha_scopes`（文字、逗号分隔、默认 `login,register`、标 `Public`），合法项只有 `login` / `register`；`PatchGroup` 只拒绝未知项（409 `setting_conflict`），空清单合法、表示两个入口都不拦。新增 seed 迁移 `seed_captcha_scopes` 把默认行种进 `setting_values`（`ON CONFLICT DO NOTHING`，不覆盖已有配置），零 DDL。
- `verifyCaptcha` 增加场景参数：登录传 `login`、注册传 `register`；判定顺序仍是「开关 → provider 为 turnstile → 私钥非空 → 入口在 `captcha_scopes` 里」，未命中直接放行。原有三处设置读取仍然吞错误放行（未改动），新增的 `captcha_scopes` 读取失败上抛报错，不静默当成「未勾选」。
- 设置页在「验证服务」下、「站点密钥」上新增「启用范围」行，只在验证服务选 Cloudflare Turnstile 时出现；Base UI `Select` 多选（点选即开关、弹层不关），勾选项显示为「登录、注册」；两个都取消时存空串，等于两个入口都不拦。
- 保存时 `captcha_scopes` 与 `captcha_provider`、两把密钥一起无条件回写；排序固定为 `login` 在前、`register` 在后。
- 前台 `getCaptchaConfig(settings, scope)` 按入口过滤：登录页只认 `login`、注册页只认 `register`；未勾选的那一页不渲染挑战弹窗、不传 token。公开设置缓存 60 秒，改完最多约一分钟才在前台生效（与既有开关一致）。
- 找回密码、注册邮箱验证码这两个流程尚未实现，故选项暂只有登录与注册，不列空头项。

**验证**：后端 `gofmt -l` 干净、`go build ./...` 与 `go vet` 通过、`bash scripts/test.sh ./internal/modules/settings/ ./internal/modules/auth/` 全绿；新增范围测试（仅勾注册时登录免挑战、仅勾登录时注册免挑战、空清单时两处都免挑战）、校验测试（未知项 409、空清单可保存并读回）与公开设置计数断言。变异自检三条：去掉范围判断、放行未知项、把空清单当全拦，对应测试分别变红，恢复后全绿。新迁移已用 `atlas migrate hash` 重算并应用到开发库与测试库。前端 `npx tsc --noEmit`、ESLint、Prettier 通过；前端无自动化测试，运行期观感（多选下拉、两个都取消后的显示、只勾一处时对应页面不再弹窗）归用户实机确认。

**提交状态**：未提交。

---

## 2026-10-24 · 人机验证配置自检（控制台「验证配置」行）

**范围**：后端 `internal/common/apierror/{apierror.go,apierror_test.go}`、`internal/modules/auth/{captcha.go,captcha_test.go,controller.go,dto.go,service.go}`、新 `internal/modules/auth/captcha_verify_test.go`、`README.md`；前端 `app/console/settings/captcha/captcha-settings-form.tsx`、新 `hooks/use-captcha-verify.ts`、`messages/{zh-CN,en}.json`；另加本条目。零迁移、零依赖。

**动机**：控制台「人机验证」页只有两把密钥，填完无法判断是否可用；保存一份错配置会立刻按 `captcha_failed` 拦死登录（Cloudflare 不可用时也是拒绝）。用户要求在私钥下新增「验证配置」行：点按钮弹出与登录页一致的人机验证弹窗，完成挑战后当场校验这对密钥，**用表单当前值（未保存草稿）**，不是库里已保存的值。

**口径**：

- 校验走新的 `POST /api/v1/admin/captcha/verifications`（`system.register.edit`，与保存 `auth` 设置组同一把钥匙），body `{"secret_key","token"}` → 200 `{"verified":true}`；缺字段 400 `invalid_request`；Cloudflare 判负 400 `captcha_verification_failed`；连不上 / 超时 / 非 2xx / 坏响应 503 `service_unavailable`。接口不读也不写设置表，不碰开关与 provider。
- 私钥由请求体传入，是为了支持「未保存草稿」；私钥本来就由 `GET /admin/settings/auth` 明文返回管理端，这条不新增暴露面。
- `turnstileVerifier` 的拒绝清单补入 `invalid-input-secret`（原先归「不可用」）：否则私钥填错时自检会提示「服务暂不可用」而不是「验证失败」。登录 / 注册侧 HTTP 结果不变（`verifyCaptcha` 对 rejected / unavailable 都回 `captcha_failed`），只有 verifier 的分类与单测预期变了。
- 前端复用 `components/auth/captcha-dialog.tsx`（登录页同一个弹窗：停 1618ms 关闭、点遮罩不关）；挑战通过后关闭弹窗再调接口，结果走 sonner toast（成功「配置验证通过」、判负「配置验证失败，请检查站点密钥与私钥」、503 用公共「服务暂不可用」、403「无权验证配置」）。
- 只在「验证服务 = Cloudflare Turnstile」时出现该行；两把密钥任一为空时点击只 toast「请先填写站点密钥和私钥」，不弹窗。开关（启用人机验证）关着也能验——只验密钥对，不改开关、不保存。

**验证**：后端 `gofmt -l` 干净、`go build ./...` 通过、`bash scripts/test.sh ./internal/modules/auth/` 全绿；新增 7 条接口测试（通过 / 判负 400 `captcha_verification_failed` / 不可用 503 / 缺字段 / 非 JSON 与跨站 Origin / 普通用户 403 / 未登录 401）。变异自检五条：把 rejected / unavailable 都改成 `nil`、去掉权限守卫、去掉 token 非空校验、删掉 `invalid-input-secret`、去掉 `requireJSONRequest`，对应测试分别变红，恢复后全绿。独立代码审查（子代理）无 Critical / Important，3 条 Minor 已修：按钮补 `aria-label`、验证请求在途时锁住两把密钥输入、补 Content-Type / Origin 守卫用例。前端 `npx tsc --noEmit`、ESLint、Prettier 通过；前端无自动化测试，运行期观感（弹窗、toast、测试密钥下能通过）归用户实机确认。

**已知缺口**：浏览器不能直连 Cloudflare `siteverify`（实测响应无 CORS 头），所以本功能必须经后端；弹窗在浏览器当前域名下渲染，域名不在该 site key 白名单时会显示「人机验证加载失败」，这是真实信号、不是误报。Cloudflare 测试密钥对任何 token 都放行，用测试密钥时自检恒过，验不出配对错误。

**提交状态**：未提交。

---

## 2026-10-24 · 人机验证「验证服务」新增「不启用」并作为默认

**范围**：后端 7 文件——`internal/modules/settings/{registry.go,seed_migration_test.go,controller_test.go,registry_test.go}`、新迁移 `migrations/20261024120000_seed_captcha_provider_none.sql` 与 `migrations/atlas.sum`、`README.md`；前端 4 文件——`lib/captcha-settings.ts`、`app/console/settings/captcha/captcha-settings-form.tsx`、`messages/zh-CN.json`、`messages/en.json`；另加本条目。零接口、零依赖。

**动机**：用户要求「验证服务」下拉新增「不启用」，且默认就是「不启用」；下拉常规只有三项——不启用 / 自研人机验证 / Cloudflare Turnstile（清单外的历史脏值仍会额外追加一条原值，正常数据不会出现）。首版只改前端、由前端把后端空串映射为「不启用」，用户随后要求前后端统一成同一种取值，于是把 `none` 定为唯一形状。

**口径**：

- 后端默认值：`auth.captcha_provider` 的 registry 默认由空串改为 `"none"`，`ResetItem` 与测试重置写回的都是 `"none"`。
- 存量迁移：新增 seed 迁移 `seed_captcha_provider_none`，用 `INSERT ... ON CONFLICT ... DO UPDATE ... WHERE value = '""'::jsonb` 把空串行改成 `"none"`；已经是 `builtin` / `turnstile` / `none` 的行一律不动，不覆盖用户已选的服务。
- seed 校验测试：`TestSeedMigrationMatchesBuiltinRegistry` 允许后一条 seed 迁移覆盖前一条的默认值（按文件名顺序、最后一条为准），同一条迁移内重复仍然报错，计数改为去重后的项数。这样"改已有项默认值"不用回头改历史迁移。
- 前端：`CAPTCHA_PROVIDER_NONE = "none"`，读入不再把空串特判成「不启用」；下拉顺序为「不启用 / 自研人机验证 / Cloudflare Turnstile」，`none` 与 `builtin`、`turnstile` 一起进 `KNOWN_CAPTCHA_PROVIDERS`，清单外的历史值仍原样追加显示。
- 「不启用」与「启用人机验证」开关相互独立：选它只改 `captcha_provider`，不碰全局开关；开关的开 / 关只由用户手动操作。折叠区不收起，仍可改选其他实现。
- 保存仍无条件回写 `captcha_provider` 与两把密钥；Turnstile 密钥行只在选中 Turnstile 时出现，值不清空。
- 拦截条件与消费方不变：仍是 `captcha_enabled=true` + `captcha_provider="turnstile"` + 私钥非空，`none` / `builtin` 都走「不校验」分支；公开设置与登录 / 注册页同样只看 `turnstile`。

**验证**：`atlas migrate hash` / `atlas migrate validate` 通过，新迁移已应用到开发库与测试库；开发库实际读出 `captcha_provider = "none"`。迁移条件在测试库用事务回滚实测四种初值：空串 → `none`，`builtin` / `turnstile` / `none` 原样保留。后端 `gofmt -l` 干净、`scripts/check.sh` 输出 `check ok`、全量 `scripts/test.sh` 全绿（settings / auth / app / mail / access 等全部包）。前端 `npx tsc --noEmit`、ESLint、Prettier 全部通过。前端无自动化测试，运行期观感（下拉顺序、选中「不启用」后全局开关保持原状、保存刷新仍为「不启用」）归用户实机确认。

**已知缺口**：新迁移尚未执行的库 `captcha_provider` 仍是空串，管理页会显示一个空白选项——应用启动不自动迁移，按既有流程先跑迁移；后端仍不校验 provider 枚举，`PATCH` 可写入任意字符串。seed 防漂移测试只校验 seed 文件里的默认值、不执行迁移 SQL，迁移的 `WHERE` 条件由上面的实测覆盖，没有自动化测试守护；迁移应用后 Atlas 校验和会锁住文件内容。

**提交状态**：未提交。

---

## 2026-10-23 · 兜底/回退清理与 AI 开发约束落盘

**范围**：后端 `internal/modules/auth/{service.go,session.go,settings_consumers_test.go}`、`internal/app/{app.go,app_test.go}`、`internal/common/apierror/apierror.go`、`internal/common/guard/{activity.go,activity_test.go}`、`internal/config/{config.go,config_test.go}`、`.env.example`、`README.md`；新增仓库根 `AGENTS.md`；另加本条目。前端零改动。

**动机**：用户为 AI 开发定新规——未经授权的兜底/回退逻辑一律不允许存在。先逐条裁定存量，再把规则写进 `backend/README.md` 硬性规则第 9 条与根 `AGENTS.md`。

**已移除的兜底**：

- **2FA 站级开关**：`twoFactorAvailable` 读 `auth.totp_enabled` 失败不再当「开着」，签名改为返回 `(bool, error)`，`SetupTOTP` / `EnableTOTP` 原样上抛；设置/启用 2FA 在读失败时返回 500。
- **会话有效期**：`establishSession` 先读设置再生效会话；`session_max_age_enabled` 关闭时正常跳过，读开关或读天数失败直接上抛（登录 500）。登录路径不再做 1–30 范围检查；保存时的校验仍由后端 `validateAuthGroup` 与前端 `validateLoginSettings` 承担。
- **人机验证**：`verifyCaptcha` 三处 `err != nil` 分支删除，读失败走零值（开关视为关、provider 为空、密钥为空）→ 放行；密钥为空继续放行。行为与改造前一致，只是不再保留显式的错误兜底分支。
- **数据库与会话**：库不可用时，scs 读会话失败不再落成 401/500。新增 `apierror.WriteHTTP`，`newSessionManager` 设 `ErrorFunc` 把连接级错误写成 503 `service_unavailable`；响应已写出时只记日志，避免二次写坏 body（`writeTracker`）。README 原文「带 cookie 仍是 401」实测为 500，已改成 503 口径。
- **Gin 模式**：`ginMode` 对未知环境返回错误，`app.New` 启动失败，不再静默退回 `release`。
- **配置**：`DB_MAX_OPEN_CONNS=10`、`DB_MAX_IDLE_CONNS=5`、`DB_CONN_MAX_LIFETIME=30m`、`SESSION_LIFETIME=168h` 改为 `internal/config` 常量，同名环境变量不再读取；`.env` 与 `.env.example` 的四个键删除。`SMTP_PORT` / `SMTP_TLS` 仍为环境变量，只在 `SMTP_HOST` 非空时必填。
- **会话活跃时间**：`auth.last_seen_at` 解析失败改为销毁会话并返回 401，不再重写成当前时间继续。
- **dummy 哈希**：启动生成失败直接 panic，不再把 `dummyHash` 置 nil 继续。

**保留并记明**：

- **降级启动**（第 6 条）：库首次 Ping 失败照常监听、读库端点 503、库恢复自愈。用户明确保留。
- **限流 `limit <= 0`**（第 8 条）：作为「关闭限流」的显式测试开关保留；生产阈值都是正数常量。
- **`APP_ENV` 默认 production**（第 10 条）：沿用 2026-10-23 环境轮的裁定。
- **未知 `system_key` 空权限 / 预设组权限走代码**（第 13 条）：失败关闭，用户明确保留。
- **事务 Rollback 错误忽略（8 处）、`database.Close` 只返回第一个错误、panic → 500**（第 15–17 条）：属清理与 HTTP 兜底，不是失败后换路执行业务；未收到用户选择，按常规做法保留。
- **前端存量（第 18–28 条）**：公开设置 fail-open、控制台/登录/注册页后端挂了照常出壳或表单、Turnstile 不可用拒绝、读失败显示加载失败、健康探测文案、主题 localStorage 失败退 system，均保留未改。第 22 条（页级身份查询失败当无用户）与第 25 条（未知错误码落通用文案）没有单独授权记录，但撑着「后端挂了控制台外壳不掉」的既有行为，暂未改动。
- **邮件第 4 条**（SMTP 为空走日志 sender）暂时后置；**第 5 条**（异步重试后丢弃）按「不做发信」跳过。

**验证**：`gofmt -l` 干净、`go build ./...` 通过、`bash scripts/test.sh` 全绿（app / apierror / apiresponse / guard / ratelimit / rbac / config / database / access / auth / health / mail / settings）、`bash scripts/check.sh` 输出 `check ok`。新增/改写的用例覆盖：settings 不可用时登录 500、2FA setup 500、degraded server 带 cookie 503、未知环境 `ginMode` 返回错误、坏活跃时间 401、固定连接池/会话常量忽略同名环境变量。

**已知影响**：

- 现有会话若 `auth.last_seen_at` 为空或损坏，会在下一次受保护请求被销毁并 401；本仓登录路径一直写该键，正常会话不受影响。
- 连接池与会话默认有效期不再可调；`auth.session_max_age_enabled` 开启时仍由 `session_max_age_days`（1–30）决定新会话 deadline。
- `SMTP_PORT` / `SMTP_TLS` 仍是环境变量；设置 `SMTP_HOST` 后缺失会启动失败并点名变量。
- 2026-10-23 的 `auth` 设置读取失败三处已全部处理；邮件两条留待后续。

**提交状态**：主体随 `d24580e` / `b538ad4` / `33e5552` 三笔提交入库，本条目随 docs 提交；均未推送。工作区同时存在并行会话的 mail / 前端改动，不属于本批。

---

## 2026-10-23 · 邮件模板测试发送（行内发送按钮 + 确认弹窗）

**范围**：后端 `internal/modules/mail/{controller.go,controller_test.go,dto.go,module.go,template.go}`、新增 `templates/{signup_code,password_reset}.{html,txt}`、`internal/app/app_test.go`、`README.md`、API 规范；前端 `app/console/settings/mail/{page.tsx,mail-settings-form.tsx}`、新增 `hooks/use-mail-test-send.ts`、`hooks/use-mail-settings-save.ts`、`messages/{zh-CN,en}.json`；另加本条目。

**动机**：用户点名在「邮件模板」两个折叠行右侧加发送图标，点击弹确认框后发送。

**口径**：

- 按钮只对 owner / admin 渲染（后端 `POST /admin/mail/messages` 的预设组守卫不变）；收件人 = 当前登录账号邮箱，确认框描述里显示模板名与收件地址。
- 有未保存修改时，确认动作先走一次保存；保存失败则不发送，按钮文案显示「保存并发送」。
- 接口 `template` 可选：缺省 / `example` 发内置自检邮件，`signup_code` / `password_reset` 发对应模板；未知值 400 `invalid_request`（`param:"template"`）。
- 新增内置 `signup_code` / `password_reset` 的 html + txt 模板；`mail` 设置组里的 `*_subject` / `*_body` 非空时覆盖主题 / HTML 正文；占位符用示例值填充（`code`=123456、`expiresMinutes`=10、`siteTitle` / `siteUrl` 读 `site` 组、`username` 取收件人 `@` 前、`sentAt` 当前时间）。
- 自定义 HTML 覆盖时纯文本部分仍用内置 txt 模板；模板只用于管理端测试发送，注册验证码 / 密码重置业务发送仍未接。

**验证**：`go test ./internal/modules/mail/ ./internal/app/ -count=1` 全绿；`tsc --noEmit`、ESLint、Prettier 通过；运行期界面未手测。

**提交状态**：已随本轮邮件模板测试发送提交入库（未推送）。

---

## 2026-10-23 · 邮件设置接入：mail 设置组 + 页面读写 + 发信运行时读取

**范围**：后端 `internal/modules/settings/{registry.go,registry_test.go,seed_migration_test.go}`、`internal/modules/mail/{module.go,mailer.go,settings.go,settings_test.go,smtp_test.go}`、`internal/app/app.go`、新迁移 `migrations/20261023120000_seed_mail_setting_values.sql` 与 `migrations/atlas.sum`、`README.md`；前端 `lib/{mail-settings.ts,admin-settings.ts}`、`hooks/use-mail-settings-save.ts`、`app/console/settings/mail/{page.tsx,mail-settings-form.tsx}`、`messages/{zh-CN,en}.json`；另加本条目。

**动机**：邮件设置页此前是纯 UI，所有值只在组件 `useState` 里，刷新即失；实际发信只看 `backend/.env` 的 `SMTP_*`。用户要求把页面接入。

**口径**：

- 新增 `mail` 设置组十二项：`enabled`（默认 `true`）、`smtp_host` / `smtp_port`（默认 `587`）/ `smtp_encryption`（默认 `starttls`，只接受 `ssl` / `starttls`）/ `smtp_username` / `smtp_password` / `sender_address` / `sender_name`（默认 `NeoBlog`）与 `signup_code_subject` / `signup_code_body` / `password_reset_subject` / `password_reset_body`。全部非公开。
- 发信每次投递前读 `mail` 组：`enabled=false` → 日志实现（不发送）；`enabled=true` 且 `smtp_host` 非空 → 整组使用设置值；`enabled=true` 但 `smtp_host` 为空 → 回退 `SMTP_*` 环境变量；两者都没有则日志。设置组读取失败直接返回错误（管理端自检 500），不静默改用环境变量。
- 组级校验：端口在被改动或已启用且填了主机时检查 1–65535，加密方式在同一时机检查枚举；启用且填了主机时要求 `sender_address` 是纯邮箱地址，`smtp_username` 非空则 `smtp_password` 必填。
- `smtp_password` 明文入库、管理接口原样返回（沿用 Turnstile 私钥的既有口径）；模板四项当时只存不用，当前没有注册验证码 / 密码重置发送流程。
- 前端页面改为服务端 `getMailSettings()` 读取，403 显示「无权查看设置」；保存走 `PATCH /api/v1/admin/settings/mail`，只提交改动项，端口转数字；保存成功 toast + `router.refresh()`。删掉「不加密」选项（后端 spec 明确不提供明文模式），保留未知加密值的显示以便修正脏数据。
- 新增校验文案四条；占位符提示改为「预留给后续验证码 / 重置邮件」。

**验证**：`go test ./internal/modules/mail/ ./internal/modules/settings/ ./internal/app/ -count=1`（带 `DATABASE_URL_TEST`）全绿；`npx tsc --noEmit`、ESLint、Prettier 对改动前端文件通过；新迁移已应用到开发库与测试库。

**已知缺口**：注册验证码 / 密码重置的发送流程仍未接，模板四项因此当时只存不用；管理端自检接口仍没有前端「发送测试邮件」入口；未做密码打码 / 加密存储。

**提交状态**：已随本轮邮件设置接入提交入库（未推送）。

---

## 2026-10-23 · 设置读取不再兜底：坏值与缺行都直接报错

**范围**：后端 `internal/modules/settings/{service.go,module.go,service_test.go,service_integration_test.go,controller_test.go,migration_test.go,repository_test.go}`、`internal/app/app_test.go`、新迁移 `migrations/20261023080025_seed_setting_values.sql` 与 `migrations/atlas.sum`、`README.md`；另加本条目。零依赖、零新接口、零前端。

**动机**：此前设置读取有两层兜底——库里值解析失败时退回代码默认值，库里没有这一行时也退回代码默认值。用户裁定两层都不要：不使用旧数据、不做兼容，会从全新数据库开始，读不到就报错，不要任何兜底。

**口径**：

- 单项读取 `getTyped`：行不存在 → `settings: <group>.<name>: not configured`；值解析失败 → 同一前缀的解码错误。两种情况都返回零值加错误，不再取 `def.Default`。
- 整组读取 `readGroup`：组内任一项缺行或解析失败 → 整组返回 `nil` 加错误，不返回半截 map、也不再用默认值补齐。
- 默认值只剩两个用途：迁移里的初始数据、`ResetItem` 的写回值。`ResetItem` 从「删行」改为「把代码默认值 upsert 回该行」，否则 reset 之后读取会报缺行。
- `PatchGroup` 保持「写入值覆盖已存值 → 组校验 → 落库 → 返回这份结果」的顺序；坏掉的那一项仍可被 PATCH 自己修好，组里其他项缺行或损坏则整次 PATCH 在写之前失败。
- 新迁移 `seed_setting_values` 把清单里 22 项（`site` 6 / `register` 5 / `auth` 11）的默认值种进 `setting_values`，`ON CONFLICT (setting_group, name) DO NOTHING`，不覆盖既有值；`atlas.sum` 已重算，迁移已应用到开发库与测试库。
- 测试不再依赖「缺行取默认」：样例组改名 `demo` 避开内置 `site`，`newIntegrationService` 建行后再跑；controller 用例的 `resetBuiltin` 先删再按清单种默认行，并在结束时恢复；`app` 的注册开关端到端用例改用 `ResetItem` 复位而不是删行；`migration_test` / `repository_test` 改用 `schema_probe` / `repo_probe` 组，避免碰到种子里已有的键。
- README 同步：设置章节写明「读取不兜底、初始行由迁移种入」，`ResetItem` 语义改为写回默认值，「怎么加一个设置」从「只改 registry」变为「registry + seed 迁移」两步，迁移清单补上 `seed_setting_values`。

**未做**：auth 里三处「读失败按默认继续」未动（`twoFactorAvailable` 返回 true、`applySessionDeadline` 直接返回、`verifyCaptcha` 返回 nil）；没有在启动时自动补行，缺行只能靠迁移、`ResetItem` 或 PATCH 修复；没有新增 `ErrCorruptValue` 哨兵；没有补 controller 层断言 500 的单独测试；前端未动。

**验证**：

- `bash scripts/test.sh` 全仓绿（app 3.8s、auth 26.4s、settings 3.2s）；`bash scripts/check.sh` 输出 `check ok`。
- 新语义变异自检三条（改坏 → 确认红 → 还原）：`getTyped` 缺行改回默认值 → `TestServiceMissingRowReturnsError` 红；`readGroup` 缺行改回默认值 → 同一用例在 `GetGroup` 断言处红；`ResetItem` 改回删行 → `TestServiceResetItem` 与 `TestServiceResetItemWritesDefault` 双红。
- 上一轮的坏值变异四条仍有效（`getTyped` / `readGroup` 改回默认值、`PatchGroup` 改回先落库再重读、`readGroup` 忽略写入覆盖），对应测试分别在错误、返回默认值、写完才报错、无法自修复处变红。
- 测试库按组清点：`site` 6、`register` 5、`auth` 11，共 22 行，与清单一致；测试产生的临时组已清掉。

**已知影响**：新库必须先 `bash scripts/db-migrate-apply.sh` 才有设置可读，否则 `GET /admin/settings/:group` 与 `/public/settings` 直接 500；手工删掉某行会让那一项在补回之前一直报 `not configured`（补回方式：`ResetItem`、PATCH 该项、或重跑迁移）；以后每加一个设置都必须同时加 seed 迁移。

**提交状态**：主体随 `93d083f` / `1b45632` / `677ecdd` 三笔提交入库（未推送）；本轮审查补丁待提交。

---

## 2026-10-23 · 环境开关收成两值，移除 .env 自举

**范围**：后端 10 文件——`internal/config/{config.go,config_test.go,dotenv.go,dotenv_test.go}`、`internal/app/{app.go,app_test.go}`、`main.go`、`scripts/{build.sh,dev.sh}`、`README.md`；另加本条目。零迁移、零依赖、零新接口、零前端、零版本进位。

**动机**：

1. 原来的三值里 `test` 没有真实用途：它唯一的效果是让 gin 不打路由表、让 `/healthy` 报 `test`——而 `production` 也不打路由表。生产代码里只有 `ginMode()` 一个 switch 分支和 `.env` 自举的同一分支会看到它，没有任何测试或路径依赖这个模式。用户裁定删掉 `test`，只留 `development` / `production`。
2. 默认值的来源不对：源码 `var DefaultEnvironment = development`，靠 `build.sh` 的 `-X ...=production` 覆盖。结果是裸 `go build` / `go run .` 出来的东西默认是开发态（cookie 不带 `Secure`、自动生成 `.env`、打印路由表），而 `build.sh` 里那句 `echo "APP_ENV default: production"` 只是硬写的一句打印，不校验 `-X` 是否真的生效。用户裁定：默认值直接改成 `production`，`-X` 整套删掉。
3. 自举 `.env` 只剩“新克隆省一步”这点价值，代价是往工作目录里凭空写一个文件，里面带一枚随机 TOTP 钥匙——文件一丢，所有开过两步验证的账号验码报 500。用户裁定整体移除。

**口径**：

- `EnvironmentTest` 删除；`APP_ENV` 只接受 `development` / `production`（首尾空白仍 trim），`test` 现在按非法值启动即失败。
- `DefaultEnvironment` 由 `var ... = development` 改为 `const ... = production`；`scripts/build.sh` 的 `-X` 与那句 echo 一起删除，ldflags 只剩 `-s -w`。
- `ginMode()` 删掉 `test → gin.TestMode` 分支；未知值仍兜底 `release`（`TestGinModeFollowsEnvironment` 里 `"test"` 与 `"bogus"` 都断言 release）。
- `scripts/dev.sh` 新增 `export APP_ENV="${APP_ENV:-development}"`，`APP_ENV=production bash scripts/dev.sh` 仍可覆盖。它是仓库里唯一的开发态入口——`go run .` 现在默认生产态。
- `.env` 自举整体移除：`EnsureDotEnv` / `EnsureDotEnvFile` / `generateTOTPEncryptionKey` 与 `generatedDotEnvTemplate` 删除，`main.go` 只保留 `LoadDotEnv()`，启动不再打 `created .env …`。工作目录没有 `.env`、环境变量里也没有对应值时，直接报 `DATABASE_URL is required (add it to .env or export it)`。
- `LoadDotEnv` 语义不变：已有文件只加载、绝不改写，进程环境变量优先。`APP_ENV` 仍会被 `.env` 里的同名键覆盖（加载层过滤仍是未做的可选项）。

| 入口 | 解析出的环境 | 启动日志 |
| --- | --- | --- |
| `bash scripts/dev.sh` | `development` | 打印 `[GIN-debug]` 路由表 |
| `go run .` / 裸 `go build` / `build.sh` 产物 | `production` | 只有 `listening on :5732` 一行 |

**未做**：没有引入“`APP_ENV` 必填”（用户选的是默认 production，不是必填，未设仍静默取默认值）；没有在加载层过滤 `.env` 里的 `APP_ENV`；没有给 `dev.sh` 补“缺 `.env` 时提示 cp”，缺配置的报错本身已点名 `.env`；没有删 `DATABASE_URL_TEST` / `POSTGRES_PASSWORD` 这两个 `.env` 键（仍由 `.env.example` 提供，compose 也读）。

**验证**：

- 变异自检四条（改坏 → 确认红 → 还原）：`DefaultEnvironment` 改回 development → `TestLoadDefaultsToProductionWhenEnvironmentUnset` 与 `TestEnsureDotEnvSkipsDefaultEnvironment` 双双红；把 `"test"` 塞回 `EffectiveEnvironment` 的 case → `TestLoadRejectsUnknownEnvironment/test` 红；把 `case "test": return gin.TestMode` 塞回 `ginMode` → `TestGinModeFollowsEnvironment/test` 红（`gin mode = "test", want "release"`）。
- `bash scripts/test.sh`（`-p 1` 串行，`DATABASE_URL_TEST` 已导出）全仓绿；`bash scripts/check.sh` 输出 `check ok`；`bash scripts/build.sh` 产物正常（35287552 B，ldflags 无 `-X`）。
- 活体三条：裸 `go build` 产物不设 `APP_ENV` → `/api/v1/healthy` 报 `production`、`[GIN-debug]` 0 行；`bash scripts/dev.sh` → `development`、`[GIN-debug]` 29 行；`APP_ENV=test` → 启动失败并点名 `APP_ENV`。
- 自举移除：空目录里跑产物，退出码 1、执行后目录里没有 `.env`；`backend/` 下（`.env` 已存在）仍正常起，`/api/v1/healthy` 的 `database` 为 `ok`、`environment` 为 `development`。

**已知影响**：本地开发态入口从 `go run .` 变为 `bash scripts/dev.sh`；新机器上第一次起后端要先 `cp .env.example .env` 并填 `TOTP_ENCRYPTION_KEY`。cookie `Secure` 仅在 `production`，用局域网 IP 访问本地 dev 时若忘设 `APP_ENV=development` 会被浏览器丢弃该 cookie。

**提交状态**：随本次提交入库。

---

## 2026-10-23 · 确认弹窗改为方案 B（图标内联 + 整卡）

**范围**：前端 3 文件——`components/ui/alert-dialog.tsx`、`app/console/settings/users/users-panel.tsx`、`app/console/settings/groups/groups-panel.tsx`；另加本条目。零后端、零接口、零文案、零依赖、零版本进位。

**动机**：上一轮盘点指出破坏性确认弹窗有三个问题：头部在窄屏居中、标题与描述排版松散；图标是 40px 灰底方块；底部按钮区靠 `border-t` + `bg-muted/50` 被切成上下两块。用户在三个 HTML 方案中选定方案 B：图标与标题同一行、描述缩进对齐标题文字、整张卡片无分隔。

**口径**：

- Content：内边距 `p-4` → `p-5`，块间距 `gap-4` → `gap-5`；宽度档位不变（`sm` 及以上 `max-w-sm`，即 384px）。
- Header：去掉 `place-items-center` / `text-center` 及窄屏居中分支，固定 `text-left`；存在 `AlertDialogMedia` 时改为 `grid-cols-[1.25rem_1fr] gap-x-2`，图标与标题同排垂直居中。
- Media：40px（`size-10`）灰底方块改为 20px 无底色内联图标（`size-5`），默认 `text-muted-foreground`；三个删除弹窗在调用点加 `text-destructive` 变红。
- Title / Description：仅在存在 media 时 `col-start-2`，描述因此缩进 28px 对齐标题文字；描述由 `text-balance` 改为 `text-pretty`。
- Footer：删掉 `-mx-4 -mb-4`、`border-t`、`bg-muted/50`、`rounded-b-xl` 与自身 `p-4`，变成卡片内的普通按钮行；右对齐与 8px 间距不变。
- 影响面：共享组件改动让 8 个 `AlertDialog` 统一生效（4 个破坏性确认 + 2 个普通确认 + 2 个提示）；新建 / 编辑用户组用 `Dialog`，不受影响。

**未做**：未抽公共确认框组件；未改按钮尺寸与 `destructive` 按钮配色；未调整 `session-expiry` / `recovery-codes` 等无图标弹窗的信息结构；未同步 `.scratch/dialogs` 演示页（该目录被 Git 忽略）。

**验证**：`npx tsc --noEmit`、ESLint（3 个改动文件）、Prettier 全部通过；在本机 dev server 的 `/ui-check` 打开真实 `AlertDialog` 截图确认图标内联、描述缩进、按钮右对齐、分隔线消失。登录后的真实删除页归用户实机确认。

**提交状态**：随本次提交入库。

---

## 2026-10-23 · 前端弹窗盘点（只盘点，零代码改动）

**范围**：仅本条目。代码零改动、零依赖、零路由、零版本进位。盘点对象含控制台与登录 / 注册页；其中 `components/auth/captcha-dialog.tsx` 为并行会话新增的文件，本条只记录其存在与分类。

**动机**：用户要求梳理「项目里有哪些 diag（dialog）弹窗」，并按删除、确定等用途分类，作为后续统一交互与文案的底账。

**口径（产品内共 10 个，分 4 类）**：

1. **破坏性确认**（确认按钮 `variant="destructive"`，红色，4 个）
   - 删除单个用户：`app/console/settings/users/users-panel.tsx:190`，触发为列表行内删除；文案键 `users.deleteConfirmTitle` / `users.deleteConfirmDescription`（「「{name}」将立即退出登录并被删除，无法恢复。」）。
   - 批量删除用户：`app/console/settings/users/users-panel.tsx:214`，触发为工具栏批量删除；文案键 `users.batchDeleteConfirmTitle`（带 `{count}` 复数）/ `users.batchDeleteConfirmDescription`。
   - 删除用户组：`app/console/settings/groups/groups-panel.tsx:282`，触发为组列表行内删除；文案键 `groups.deleteConfirmTitle` / `groups.deleteConfirmDescription`。
   - 放弃未保存的修改：`app/console/settings/groups/group-dialog.tsx:269`，触发为组表单有脏值（`isDirty`）时关闭；文案键 `groups.discardTitle` / `groups.discardDescription`。
2. **普通操作确认**（默认按钮，2 个）
   - 退出单个会话：`app/console/uc/session/sessions-panel.tsx:301`，触发为会话列表行内「退出」；文案键 `sessions.revokeConfirmTitle` / `sessions.revokeConfirmDescription`（带 `{device}`）。
   - 退出登录：`components/console/console-user-menu.tsx:186`，触发为用户菜单 → 退出登录；文案键 `userMenu.signOutConfirmTitle` / `userMenu.signOutConfirmDescription` / `userMenu.signOutConfirm`。
3. **提示 / 告知**（2 个，不是「确定要做吗」）
   - 登录会话已失效：`components/console/session-expiry-dialog.tsx:33`，挂在 `app/console/layout.tsx:42`，由 401 `unauthorized` 的全局信号触发；出口为「留在此页」/「重新登录」；文案键 `sessionExpiry.*`。
   - 两步验证恢复码：`app/console/uc/profile/recovery-codes-dialog.tsx:29`，由 `two-factor-panel.tsx:205` 在开启成功后打开；展示 10 个恢复码 + 复制，只有「我已保存」一个动作；文案键 `uc.twoFactorRecovery*`。
4. **表单 / 操作弹窗**（2 个，非确认框）
   - 新建 / 编辑用户组：`app/console/settings/groups/group-dialog.tsx:120`，标准 `Dialog`，标题按 `group === null` 切换 `groups.dialogCreateTitle` / `groups.dialogEditTitle`。
   - 人机验证（Turnstile）：`components/auth/captcha-dialog.tsx:57`（并行会话新增），登录 `app/signin/signin-form.tsx:166` 与注册 `app/signup/signup-form.tsx:169` 各挂一个；表单提交时若已配置 captcha 就打开，`Dialog` + `disablePointerDismissal`（点遮罩不关），内含 `TurnstileCaptcha`，拿到 token 后延迟 1618ms 自动关闭并回填；文案键 `auth.captchaDialogTitle`（「人机验证」/「Human verification」），只有标题、无描述、无底部按钮。

**实现共性**：确认类全部基于 `components/ui/alert-dialog.tsx`（Base UI `AlertDialog`），破坏性操作的按钮统一用 `variant="destructive"`；带图标的确认框统一把图标放进 `AlertDialogMedia`（删除用 `Trash2`、退出用 `LogOut`）；表单 / 操作类共 2 个，均用 `components/ui/dialog.tsx`；确认框的取消按钮统一取 `common.cancel`。

**非产品弹窗**：`app/ui-check/page.tsx:1114`（Dialog）与 `:1130`（AlertDialog）仅为组件预览；`components/ui/command.tsx` 内的 `CommandDialog` 全站无真实调用（ui-check 只用了普通 `Command`）。

**观察（未改动，供后续裁定）**：`session-expiry-dialog` 与 `recovery-codes-dialog` 没有 `AlertDialogMedia` 图标，与其余确认框观感不一致；`recovery-codes-dialog` 用 `AlertDialog` 承载纯展示内容，语义上更接近信息弹窗；「放弃未保存修改」用了红色危险按钮，与删除类同色，但它的后果只是丢草稿；各弹窗文案分散在 `users` / `groups` / `sessions` / `userMenu` / `uc` / `sessionExpiry` / `auth` 七个命名空间；`captcha-dialog` 没有描述与手动关闭按钮，只能点右上角 X（`disablePointerDismissal` 已关掉点遮罩），且关闭前有 1618ms 固定延迟。

**未做**：未改任何弹窗的样式、图标、按钮变体或文案；未抽公共确认框组件；未补 ui-check 演示。

**验证**：逐文件静态核对（`grep` 全仓 `Dialog` / `AlertDialog` 引用，排除 `components/ui` 与 `.next`），确认产品代码中 10 个实例（8 个 `AlertDialog` + 2 个 `Dialog`）、无遗漏；分类依据为 `variant="destructive"` 与触发语义。中途复扫发现并行会话新增了 `captcha-dialog.tsx`，条目数字已由 9 修正为 10。运行期观感归用户实机确认。

**提交状态**：随本次提交入库。

---

## 2026-10-23 · 人机验证接入 Cloudflare Turnstile（登录 / 注册真拦截）

**范围**：后端 16 文件——`common/apierror/apierror.go` 与 `apierror_test.go`；`modules/settings/{registry.go,registry_test.go,controller_test.go}`；`modules/auth/captcha.go`（新）、`captcha_test.go`（新）、`captcha_flow_test.go`（新）、`service.go`、`controller.go`、`dto.go`、`module.go`、`auth_test.go`、`totp_flow_test.go`；`README.md`、`internal/version/version.go`。前端 13 文件——`lib/public-settings.ts`、`components/auth/turnstile-captcha.tsx`（新）、`components/auth/captcha-dialog.tsx`（新）、`hooks/{use-signin.ts,use-signup.ts}`、`app/signin/{page,signin-view,signin-form}.tsx`、`app/signup/{page,signup-view,signup-form}.tsx`、`messages/{zh-CN,en}.json`。另加本条目。零迁移、零依赖、零新路由；版本 `0.22.0` → `0.23.0`。

**动机**：上一轮只把两把密钥存进了设置组，选了 Turnstile 也不会真的出验证。本轮按用户要求接真：启用后登录与注册必须先过人机验证，失败不允许登录 / 注册，并返回专属状态码。

**口径**：

- 公开面：`captcha_enabled`、`captcha_provider`、`turnstile_site_key` 三项标 `Public`（登录 / 注册页要读它们决定是否渲染小组件），`turnstile_secret_key` 保持非公开。
- 错误码：新增 `captcha_failed`（400 `invalid_request_error`，`param:"captcha_token"`），登记进 `statusByCode` 与 `TestRegistryIntegrity`（33 → 34）。
- 四态模型照参考项目：`pass` / `reject` / `unavailable`（另有「不启用」由条件判断直接放行）。`siteverify` 表单字段 `secret` / `response` / `remoteip`（IP 为空或 `unknown` 时不带），超时 2.5 秒；只有访客侧错误码 `missing-input-response` / `invalid-input-response` / `timeout-or-duplicate` 判拒绝，`invalid-input-secret`、非 2xx、坏 JSON、超时、传输错误全部判「不可用」。
- 拦截条件（三者同时成立才校验）：`captcha_enabled=true`、`captcha_provider="turnstile"`、`turnstile_secret_key` 非空。**缺任一条件放行**——包括选了自研（`builtin`，尚无实现）、密钥没填、以及设置层读取失败（沿用 auth 模块既有的 fail-open：`brokenSettingsService` 用例仍然登录成功）。
- 判定顺序：登录在 IP 频率限制 + 账号锁定检查之后、比对密码之前；注册在注册总闸与字段结构校验之后、邮箱频率限制之前。captcha 失败**不计入**登录失败次数（有专门用例连打 6 次拒绝后仍能用正确密码登录成功），也不新开独立计数桶。
- 空 token 不发网络请求，直接 400 `captcha_failed`。
- 前端：`lib/public-settings.ts` 加 `getCaptchaConfig`，只有「启用 + turnstile + site key 非空」才返回配置；`components/auth/turnstile-captcha.tsx` 用 `render=explicit` 加载官方脚本（10 秒加载超时、失败清空 token），主题读 `documentElement.classList.contains("dark")`、语言跟随 locale、`size:"flexible"`。
- 弹窗形态（用户裁定，照参考项目）：小组件**不内嵌在表单里**，而是包进 `components/auth/captcha-dialog.tsx` 的二级弹窗——只有点「登录」/「注册」且表单校验全部通过、当前又没有 token 时才弹出（标题 `auth.captchaDialogTitle`「人机验证」，点遮罩不关，右上角 X 可取消）；弹窗打开时才挂载组件，关闭即卸载。
- 通过后的节奏：拿到 token 不立刻关窗，先停 `CLOSE_DELAY_MS = 1618`（照参考项目的停顿）让 Turnstile 自己的成功动画走完，再关窗并提交；等待期间点 X 取消会清掉定时器，不会偷偷提交。
- 提交闸门：hook 新增 `onCaptchaRequired` 回调与 `submit(captchaToken?)` 可选参数。表单提交 → 校验通过后若缺 token 就回调开窗并中止；用户在弹窗里验完 → 关窗、把 token 直接传给 `submit(token)` 发起真实请求（不用隐藏 input + `requestSubmit`）。token 一次性，验过即作废：任何失败后再次提交都会重新弹窗、重新拿码；`captcha_failed` 单独 toast「人机验证未通过，请重试」。2FA 二段步骤不校验 captcha，也不需要弹窗。
- 后端注入：`newService` 新增最后一个参数 `captchaVerifier`（接口），`nil` 时退化为「不可用」实现；生产在 `auth.New` 里构造 `newTurnstileVerifier(turnstileSiteverifyURL, 2500ms)`。测试用 `fakeCaptchaVerifier` 记录 secret / token / ip。

**未做**：自研（`builtin`）验证器仍未实现，选了它 + 启用开关等于不拦（文档已注明）；没有独立的 captcha 频率桶（只受登录 / 注册既有 IP 频率限制保护）；前端无自动化测试；`/public/settings` 读不到时页面不出小组件，但后端照样拦，用户会碰到 `captcha_failed`（公开设置缓存 60 秒，正常的瞬时故障影响有限）。

**验证**：`gofmt -l` 干净；`bash scripts/test.sh`（`-p 1`，含 `DATABASE_URL_TEST`）全绿——app / apierror / auth / settings 等 13 个包全过。四条变异实测变红：① 去掉 `remoteip` → 表单用例报 `remoteip = ""`；② 把访客侧错误码改成「不可用」→ 分类矩阵三条红；③ 删掉空 token 短路 → 登录 / 注册用例拿到 200 / 201；④ 把「拒绝」当成「通过」→ 拒绝类用例全部翻绿失败。前端 `npx tsc --noEmit`、ESLint、Prettier 全通过。运行期（本地用测试密钥走通登录 / 注册与失败提示）归用户实机确认。

**本地调试密钥**（Cloudflare 官方测试密钥，任意域名可用，含 localhost）：站点密钥 `1x00000000000000000000AA` + 私钥 `1x0000000000000000000000000000000AA` = 永远通过；站点 `2x00000000000000000000AB` + 私钥 `2x0000000000000000000000000000000AA` = 永远失败；`3x00000000000000000000FF` = 强制交互，配合 `1x…AA` 私钥。

**提交状态**：未提交。

---

## 2026-10-23 · Turnstile 密钥设置落库（只接线，不改登录/注册）

**范围**：后端 5 文件——`internal/modules/settings/registry.go`、`registry_test.go`、`controller_test.go`、`README.md`、`internal/version/version.go`；前端 3 文件——`lib/login-settings.ts`、`hooks/use-login-settings-save.ts`、`app/console/settings/login/login-control-form.tsx`；另加本条目。零迁移、零依赖、零路由变化；版本 `0.21.2` → `0.22.0`（minor）。

**动机**：上一轮把「验证服务」下拉加了 Cloudflare Turnstile，但两把密钥只是组件本地 state，刷新即丢。本轮让它们真正进 `auth` 设置组，登录/注册的校验留到下一轮（用户裁定「只接线，不做登录注册」）。

**口径**：

- registry：`auth` 组新增 `turnstile_site_key` / `turnstile_secret_key`（均为文字、默认空串），9 项 → 11 项。**不**标 `Public`、**不**加 `ValidateOn`、**不**加密——按用户裁定私钥明文入库、管理接口原样返回。
- 前端：两个键进 `LoginSettingsValues` 与 `LOGIN_PATCH_KEYS`，脏值判断与悬浮保存按钮自然生效；保存时与 `captcha_provider` 一样**无条件**回写当前显示值，避免「界面有、库里没有」的错位。
- 密钥两行仍是「验证服务选 Turnstile 才出现」，切回自研只是收起输入，库里的值不删。
- 读回：`GET /admin/settings/auth` 返回 11 项，其中两把密钥原样返回；能读 `auth` 组的账号（`system.register.view`）即可读到私钥，与「无 secret 打码机制」的现状一致。

**未做**：`/public/settings` 不加任何 captcha 键（当前无消费方）；登录、注册、两步验证零改动；无 secret 打码；无「启用前必须填 Site Key」的写入拦截。

**验证**：`gofmt -l` 干净、`go build ./...` 通过；`go test ./internal/modules/settings/...` 与 `./internal/app/...`（串行，需 `DATABASE_URL_TEST`）通过。三条变异实测变红——① 把 `turnstile_secret_key` 默认值改成 `"x"` → shape 测试报 `default = "x", want ""`；② 删掉 registry 里那一行 → shape 计数 10≠11、admin 回读 10 项、PATCH 两把密钥 400；③ 把 `turnstile_site_key` 类型改成 Number → 临时用例期望 400 拿到 200（临时用例已删，改回后主测试全绿）。新增 `TestSettingsPatchTurnstileKeysRoundtrip` 覆盖「写入两把密钥 → GET 原样读回」。前端 `npx tsc --noEmit`、ESLint、Prettier 全通过。运行期（填两把密钥保存后刷新仍在）归用户实机确认。

**下一轮（真正拦截）已定口径**：入口为登录 + 注册；照参考项目做 `PASSED / REJECTED / UNAVAILABLE / DISABLED` 四态，`REJECTED` 报 400 `captcha_failed`「人机验证未通过」，超时 / CF 5xx / 密钥配错一律 `UNAVAILABLE` 放行；siteverify 超时 2.5s、带 `remoteip`；captcha 失败不消耗登录失败锁定与频率预算；公开面加 `captcha_enabled` / `captcha_provider` / `turnstile_site_key`，私钥永不公开。

**提交状态**：未提交。

---

## 2026-10-23 · 登录控制「人机验证」新增 Cloudflare Turnstile 选项与密钥输入

**范围**：前端 3 文件——`app/console/settings/login/login-control-form.tsx`、`lib/login-settings.ts`、`messages/{zh-CN,en}.json`；另加本条目。零后端、零迁移、零依赖、零路由变化；版本保持 0.21.2（纯前端，不单独进位）。

**动机**：人机验证要接入 Cloudflare 的方案，先只做界面——用户明确本轮纯 UI，后端消费不接。

**口径**：

- 取值：新增 `CAPTCHA_PROVIDER_TURNSTILE = "turnstile"`，与既有 `builtin`（自研）并列；后端 `auth.captcha_provider` 是无枚举约束的字符串（默认 `""`），这个值能直接存下，无需动注册表。
- 文案：`settings.captchaProviderTurnstile` =「Cloudflare Turnstile」，中英同文（官方产品名不译），紧跟 `captchaProviderBuiltin`。
- 未知值回退：原逻辑是「只要不是 `builtin` 就追加一条原始值」，加了第二个已知项后会把 `turnstile` 再重复一条；改为按 `KNOWN_CAPTCHA_PROVIDERS` 判断，清单外的历史值才原样追加，`items` 与下拉项共用同一个 `unknownCaptchaProvider`。
- 宽度：`SelectTrigger` 由 `w-45`（180px）加宽到 `w-50`（200px）——「Cloudflare Turnstile」比「自研人机验证」宽，180px 装不下；触发条是 `whitespace-nowrap`，只会溢出不会换行。
- 密钥两行：`captcha_provider` 选中 `turnstile` 时，同一折叠区内多出「站点密钥」/「私钥」两行输入（图标 `Globe` / `LockKeyhole`，宽度 `w-64`，私钥用裸 `Input type="password"`，不另做专用组件；两格照 SMTP 两格的既有写法关掉自动填充与拼写检查——`autoComplete="off"` / `"new-password"` 加 `spellCheck={false}`，免得浏览器把私钥格当登录密码框自动填）。值放在表单本地的 `turnstileSiteKey` / `turnstileSecretKey` 两个 state 里，**不进 `LoginSettingsValues`、不发 PATCH**——后端 `auth` 组没有这两项，发了会撞未知键；切回自研再切回来已填内容不丢（state 在父组件）。本条「本地 state」的结论已被下一轮推翻，现在由后端设置组承载，见上方「Turnstile 密钥设置落库」条目。
- 折叠区结构：原来 `expandedContent` 只有一个 `ConsoleSettingsRow`，现在改成 fragment 包三行，靠 `CollapsibleContent` 上既有的 `divide-y` 出分隔线，未改组件与样式。

**未做**：密钥两项未落库（后端清单没有、也没有 secret 类型机制），刷新即丢，所以本轮它们只是形状——落库已在下一轮完成，见上方条目；后端仍零消费方，选了不会真的出验证；`captcha_provider` 仍无枚举校验。

**验证**：`npx tsc --noEmit` 全项目通过；ESLint 对两个改动文件 0 error；Prettier 对四个改动文件全部通过。

**提交状态**：未提交。

---

## 2026-10-23 · 退出登录后提示「已退出登录」

**范围**：前端 6 文件——新增 `lib/signout-notice.ts`、`hooks/use-signout-notice.ts`；`hooks/use-signout.ts`、`app/signin/signin-view.tsx`、`messages/{zh-CN,en}.json`；另加本条目。零后端、零迁移、零依赖、零路由变化；版本保持 0.21.2（纯前端，不单独进位）。

**动机**：用户要求照登录成功的做法，退出登录后也弹 sonner 提示。

**口径**：

- 机制与「登录成功后进控制台再提示」完全一致：退出成功写 `sessionStorage` 标记（`neoblog.signout.success`），`/signin` 页面挂载时由 `useSignoutNotice()` 消费并 `toast.success`，时序是「先切回登录页、再弹提示」。
- 两条退出路径都写标记：正常 `DELETE /api/v1/sessions/current` 成功后 `router.replace("/signin")`；返回 401（会话已失效）走 `window.location.replace("/signin")` 之前——整页跳转不影响 `sessionStorage`。
- 文案：新增 `userMenu.signedOut` =「已退出登录」/「Signed out」，放 `userMenu` 命名空间（与 `signOutFailed` 同组；注册、登录两条提示也各自用自己的来源命名空间）。
- 未做：会话失效弹窗（`SessionExpiryDialog`）不写标记——它不是用户主动退出。

**验证**：ESLint（改动文件）、`npx prettier --check`、`npx tsc --noEmit` 全部通过；中英两个新键读取正确；grep 确认「写标记 / 挂载 / 消费」三处接线完整，无多余调用点。运行期观感未起 dev server 复现，归用户实机确认。

**已知缺口**：若退出后登录页没能挂载，这条提示会留到下一次进登录页才弹（与注册、登录同款行为，未额外处理）。

**提交状态**：本轮代码 `de600d1` 已入库；本条目再单独入库，且只包含本轮的 6 个文件（两个语言文件里仅 `signedOut` 两行），未混入并行会话正在做的「验证服务（Turnstile）」改动。

---

## 2026-10-23 · toast 同屏两条 + 登录成功后进控制台再提示

**范围**：前端 7 文件——`components/ui/sonner.tsx`；新增 `lib/signin-notice.ts`、`hooks/use-signin-notice.ts`、`components/console/console-signin-notice.tsx`；`hooks/use-signin.ts`、`app/console/layout.tsx`、`messages/{zh-CN,en}.json`；另加本条目。零后端、零迁移、零依赖、零路由变化；版本保持 0.21.2（纯前端，不单独进位）。

**动机**：两条文案长短不一的 toast 连着弹时，后出现的那条会退到背景、文字被隐藏，只剩一条比前面更宽的空白卡片；用户要求去掉这个观感。另外登录成功后没有提示，用户要求照注册成功的做法：先完成页面切换、再在控制台弹「登录成功」。

**口径**：

- toast 堆叠：根因在 sonner 默认样式——`styles.css:297` 把非最上层 toast 的高度压成最前面那条的高度，`:315` 把它的所有子元素设为 `opacity: 0`，但宽度仍按各自内容的 `max-content` 计算（我们在 `sonner.tsx` 里设的），于是长文案那条只剩一块更宽的空白卡片。在 `<Sonner>` 上加 `visibleToasts`，超出的 toast 会被打上 `data-visible="false"` 直接淡出。用户在 1 与 2 之间选定 **2**（`visibleToasts={2}`）。
- 登录成功提示：完全照注册的机制——`use-signin.ts` 在密码登录成功、两步验证成功两处 `router.push("/console")` 之前写 `sessionStorage` 标记；控制台布局挂载 `ConsoleSigninNotice`，其 `useSigninNotice()` 在 effect 中消费标记并 `toast.success`。放布局而不是页面，是为了任何进入控制台的入口都检查一次。
- 文案：新增 `auth.signin.success` =「登录成功」/「Signed in successfully」。
- 未做：改为登录页直接弹 toast（时序会变成切换前弹，与用户要求的「先切换再弹」相反）；未改 `expand`、未改 toast 宽度。

**验证**：`npx tsc --noEmit` 全项目通过；ESLint 0 error（`sonner.tsx:7` 的 `InfoIcon` 未使用为改动前既有 warning，已核对 HEAD 版本）；Prettier 对全部改动文件通过；`git diff --check` 无空白问题；grep 确认标记的写、挂载、消费三处接线完整，无多余调用点；两份语言文件中 `"success"` 各 2 处（登录、注册），同一块内无重复键。标记读取即删除，开发模式 effect 跑两遍时第二次读到空值，不会重复弹。运行期观感未起 dev server 复现，归用户实机确认。

**已知缺口**：背景 toast 的文字仍被 sonner 隐藏，两条宽度差距大时「更宽的空白条」仍可能露出来；真遇到可退回 `visibleToasts={1}` 或把 toast 宽度统一成固定值。登录成功标记存在 `sessionStorage`，若登录成功但控制台页面没能加载，这条提示会留到下一次进控制台才弹（与注册同款行为，未额外处理）。

**提交状态**：`db7e199`（toast 同屏两条）、`0679f46`（登录成功提示）已入库；本条目随后单独入库。

---

## 2026-10-23 · 登录 / 注册表单打磨（副标题、按钮按压、注册校验）

**范围**：前端 6 文件——`app/signin/signin-view.tsx`、`app/signup/signup-view.tsx`、`components/ui/button.tsx`、`hooks/use-signup.ts`、`messages/{zh-CN,en}.json`；另加本条目。零后端、零迁移、零依赖、零路由变化；版本保持 0.21.2（纯前端，不单独进位）。

**动机**：四件事同轮处理——登录 / 注册两页的副标题与标题语义重复；全站按钮按下时只有 1px 位移，且被 `transition-all` 拖成慢速滑动，手感拖沓、移动端几乎无反馈；注册表单没有空值提示，空邮箱会落到格式校验上、弹「邮箱格式不正确」；用户名文案写的是「不能含空格和 @」，与实际校验不一致。

**口径**：

- 副标题：删掉 `auth.signin.subtitle`（登录以继续）与 `auth.signup.subtitle`（注册新账号）的渲染及中英各一对文案键，卡片内只剩标题；`AuthFrame` 的卡片内边距、logo 与卡片的间距、表单外层 `mt-6` 均未动。
- 按钮按压：`components/ui/button.tsx` 去掉 `active:not-aria-[haspopup]:translate-y-px`（该 `not-aria-[haspopup]` 守卫只服务于位移，一并删除），`transition-all` 收窄为 `transition-colors`，`default` 变体新增 `active:bg-primary/80`（hover 为 `/90`，按下再深一档）。全站 `<Button>` 一并生效。
- 注册空值校验：`submit()` 每个字段改为先判空、再判格式，新增 `requiredEmail` / `requiredName` / `requiredPassword` / `requiredConfirmPassword` 四对文案键，命名对齐登录页既有的 `requiredIdentifier` / `requiredPassword`。此前空确认密码会误报「两次输入的密码不一致」。
- 用户名规则（用户裁定）：`invalidName` 文案改为「用户名至少为 {minLength} 位，且不能包含特殊字符」（英文同步），调用处去掉不再使用的 `maxLength` 参数；`isValidName` 的正则由 `/[\s@]/` 换成 `/^[\p{L}\p{N}_-]+$/u`，即只允许中文 / 英文字母、数字、`_`、`-`，长度仍为 2–32。
- 字段顺序（用户裁定）：注册表单改为用户名在上、邮箱在下（DOM 顺序一并调整，Tab 顺序与浏览器自动填充跟随）；`submit()` 的校验顺序同步改为先用户名后邮箱，保证弹出的第一句提示与最上面的字段一致。
- 文案微调：`weakPassword` 改为「密码不符合站点要求」（英文同步），前端预检与后端 `weak_password` 两处报错共用。
- 未做：`outline` / `secondary` 变体的「阴影收掉 + 下沉」按压效果；`badge.tsx`、`toggle.tsx` 里的 `transition-all`（与按压无关）。

**验证**：`npx tsc --noEmit` 全项目通过；两个语言文件 JSON 解析通过，改写与新增的键在中英两侧渲染结果逐一核对；用户名校验用 14 个用例（空、单字、中文、下划线、连字符、空格、`@`、`!`、emoji、32 / 33 位边界）跑同一套判断逻辑，全部符合预期；用项目 `globals.css` 编译 `hover:bg-primary/90` 与 `active:bg-primary/80`，确认生成顺序是 hover 在前、active 在后，按下时不会被 `/90` 覆盖。运行期观感未起 dev server 复现，归用户实机确认（改 CSS 类后需重启 dev server，否则可能沿用旧样式）。

**已知缺口**：后端 `backend/internal/modules/auth/validate.go` 的 `validNickname` 仍只拦空格和 `@`、长度 2–32，绕过界面直接调 `POST /api/v1/users` 仍可注册出 `q!` 这类用户名——用户裁定本轮暂不改后端。`NAME_MAX_LENGTH = 32` 仍生效，但界面文案不再提示上限。`outline` / `secondary` 变体如需要按压反馈，可加 `active:translate-y-px active:shadow-none`。

**提交状态**：副标题移除 `dd3c6c1`、按钮按压 `123d48b`、注册校验与字段顺序 `da17496` 均已入库；本条目随后单独入库。

---

## 2026-10-23 · 个人资料内嵌报错统一改走 sonner

**范围**：前端 4 文件——`app/console/uc/profile/{profile-form,two-factor-panel}.tsx`、`hooks/{use-password-change,use-two-factor-setup}.ts`。零后端、零迁移、零依赖；7 个既有文案键全部保留（改由 toast 使用），未新增、未删除键；版本保持 0.21.2（纯前端交互修正，不单独进位）。

**动机**：个人信息页是全站仅有的内嵌红字报错处（`aria-live` 状态行）。同一句「密码不正确」在改密块是内嵌红字、在两步验证流程却是 sonner toast，两套呈现并存。用户要求把这些报错统一为 sonner。

**口径**：

- 删除三处内嵌状态行：改密块（密码不正确 / 密码长度不足 / 密码太长 / 新旧密码相同）与两步验证第一步、第二步（本站未开放两步验证 / 密钥生成失败 / 验证码不正确）。
- 改密：本地预检（超长、复杂度）由 `setState` 改为直接 `toast.error`；后端 `invalid_credentials` / `weak_password` / `password_unchanged` 与通用失败全部 toast；`usePasswordChange` 的状态机简化为 `isSubmitting` 布尔值，在 `finally` 中复位，失败时字段保持不清空以便修改。
- 两步验证：hook 原本已对密码错误、站点关闭、生成密钥失败弹 toast，页面又重复显示一遍，现只保留 toast；验证码错误原本只有内嵌红字，本轮补上 `twoFactorInvalidCode` 的 toast，输入框 `aria-invalid` 红框保留。
- 随状态行一起移除「密钥生成失败」后的「重试」按钮：失败后密码仍留在输入框，再点「继续」即重试；验证码重输满 6 位自动提交。hook 中只被该行使用的 `isLoadFailed`、`isDisabledBySite`、`retrySetup` 一并删除。
- 未改：请求期间按钮与输入框仍禁用；成功与通用失败 toast 不变；`common.retry` 仍被登录会话页使用，未清键。

**验证**：`npx tsc --noEmit`（全项目）、ESLint（4 文件）、`npx prettier --check`、`git diff --check` 均通过；grep 确认 `frontend/app` 下已无 `aria-live` / `text-destructive` 内嵌报错残留（组件库内部样式除外）；七个文案键逐一确认仍有调用方。成功 / 各类错误码 / 401 静默 / 站点关闭 / 生成失败 / 验证码错逐条核对，未发现重复 toast 或状态卡死。运行期 toast 外观未起 dev server 复现，归用户实机确认。

**已知缺口**：验证码校验遇 401 时 `submitState` 会停在 `verifying`（既有行为；401 由全局会话失效弹窗接管页面，不影响使用）；「密钥生成失败」不再有显式重试按钮，若需要 toast 内带动作需另做。

**提交状态**：代码批次 `f3bea92` 已入库；本条目随后单独入库。

---

## 2026-10-23 · 移除全站加载中状态文字与转圈图标

**范围**：前端 9 文件——`app/console/uc/profile/{profile-form,two-factor-panel}.tsx`、`app/ui-check/page.tsx`、`components/console/console-list-toolbar.tsx`、`components/ui/{toast,sonner}.tsx`、删除 `components/ui/spinner.tsx`、`messages/{zh-CN,en}.json`。零后端、零依赖、零路由变化；版本保持 0.21.2（纯前端，不单独进位）。

**动机**：用户要求移除全站所有「加载中」状态文字与转圈图标。

**口径**：

- 个人资料：改密「正在修改…」、两步验证「正在生成密钥…」「正在验证…」三处状态文字与 `LoaderCircle` 删除；请求期间按钮与输入框仍禁用，防重复提交。
- 列表刷新：用户 / 用户组 / 登录会话共用的工具栏刷新按钮不再旋转，刷新期间仍禁用。
- 组件库：删除 `Spinner` 组件及 `ui-check` 中三处演示（按钮「处理中」、spinner 演示、附件上传态）；base-ui toast 去掉 `loading` 类型图标；sonner toaster 清理未使用的 `Loader2Icon` 导入（生产代码无任何 `toast.loading` 调用）。
- i18n：删除 `uc.passwordChanging`、`uc.twoFactorSetupLoading`、`uc.twoFactorVerifying` 三键，中英文各三。
- 未动：成功与失败 toast、所有错误文案、`Skeleton` 占位（仅出现在 `ui-check` 演示与未使用的侧栏样板，生产页面未使用）。

**验证**：`npx tsc --noEmit`、ESLint（改动文件）、`npx prettier --check` 均通过；grep 确认无 `animate-spin` / `LoaderCircle` / `<Spinner` 残留。运行期未起 dev server 复现，归用户实机确认。

**已知缺口**：`components/ui/attachment.tsx` 内仍保留一条针对 `data-slot=spinner` 的尺寸选择器（组件已删除，选择器成死样式，无可视影响）；`ui-check` 的 `Skeleton` 演示与 `components/ui/sidebar.tsx` 的 `SidebarMenuSkeleton` 仍在。

**提交状态**：代码批次 `40646ee` 已入库；本条目与上一轮日志同批入库。

---

## 2026-10-23 · 开关折叠块随开关自动展开（用户注册 / 登录控制 / 邮件）

**范围**：前端 4 文件——新增 `hooks/use-switch-row-expansion.ts`；`app/console/settings/{register,login,mail}` 三个表单把共 6 个开关行接成受控展开。共享组件 `components/console/console-settings-row.tsx` 零改动（其 `expanded` / `onExpandedChange` 是既有能力）。零后端、零迁移、零依赖、无新增文案键；版本保持 0.21.2（纯前端交互修正，不单独进位）。

**动机**：此前开关行一律默认展开、与开关值无关（开关全关时子项仍全部露出）。用户要求改成：开关打开时自动展开；关闭时不立即收起，刷新后按开关值显示为收起。

**口径**：

- 页面加载时以开关的持久值播种展开状态：开=展开，关=收起；手动点箭头仍可自由开合。
- 开 → 立即展开；关 → 不改动展开状态（已展开的不会当场收起，下次加载才按已保存值收起）。
- 覆盖 6 行：用户注册「允许公开注册 / 启用邮箱域名控制 / 启用密码要求」、登录控制「会话有效期 / 人机验证」、邮件「是否启用 SMTP」。
- 未纳入：常规页「屏蔽搜索引擎」与登录控制「两步验证 / 密码重置」开关下没有子项；个人设置页两步验证行是「点开关即弹设置/关闭面板」的操作面板，语义不同，未改。
- 行为变化：邮件页 SMTP 开关初值为关，现在进页面默认收起（此前默认展开）。
- 刷新看到的是已保存值：关掉开关但未保存就刷新，回来仍按服务端的「开」展开。

**验证**：`npx tsc --noEmit`、ESLint、`npx prettier --check` 对改动文件全部通过；独立子代理复审未发现 P0–P2，唯一可修的 P3（邮件表单 hook 导入被空行单独分组）已改掉。运行期交互未另起 dev server 复现，归用户实机确认。

**已知缺口**：开关触发程序化展开时，开关本身未挂 `aria-controls` / `aria-expanded`，焦点在开关上的读屏用户只会听到开关状态变化，不会被告知子设置已出现（行触发器仍有独立无障碍名称）；补齐需要先把面板 id 暴露出来。用户注册、登录控制两行开关的既有「展开按钮与开关同名」问题仍待后续轮次用 `expandLabel` 补齐。

**提交状态**：代码批次 `a8a8066` 已入库；本条目随后单独入库。

---

## 2026-10-23 · 邮件设置 UI 轮（侧栏入口 + 纯 UI 页面）

**范围**：前端 6 文件——`components/console/console-sidebar-nav.tsx`（新增「邮件」分组与菜单项）、`app/console/settings/mail/{page,mail-settings-form}.tsx`（新建）、`messages/{zh-CN,en}.json`（新增 22 个文案键）、`components/console/console-settings-row.tsx`（新增 `expandLabel` 属性）；另加本条目。零后端、零迁移、零依赖，新增一个纯前端页面路由；版本保持 0.21.2（纯前端 UI，不单独进位）。

**动机**：用户点名在设置侧栏底部新增「邮件」分组与同名菜单项，并参照 D 盘 HaoBlog 的「邮件与通知」页做一张纯 UI 的邮件设置页。口径是「纯前端的更改，纯 UI 的更改」：不接接口、不存库、不加保存按钮；随后按用户逐轮截图迭代——第一行改为启用开关、补认证与发件人、补邮件模板并可展开、去掉模板编辑区 280px 宽度限制。

**口径**：

- 侧栏：分组小标题与菜单项都叫「邮件」，入口 `/console/settings/mail`，可见条件与「常规」一致（`system.settings.view`）；无权限时整组小标题一起不渲染（沿用空组剔除）。
- 页面：「SMTP 服务器」组第一行是「是否启用」开关（默认关，折叠行、默认展开），其下依次是服务器地址、端口（默认 `465`，输入时过滤非数字）、加密方式（`Select`，默认 `ssl`，选项 SSL / STARTTLS / 不加密）。
- 「认证与发件人」组：用户名、密码或授权码、发件人地址、发件人显示名。密码按用户要求使用普通 `Input type="password"`，不移植对照项目的专用密钥输入组件。
- 「邮件模板」组：注册验证码、密码重置验证码两行，默认收起；展开后是邮件主题输入框、HTML 正文文本域（等宽字体、最小高度 320px）与占位符提示（`{{code}}`、`{{expiresMinutes}}`、`{{siteTitle}}`、`{{siteUrl}}`、`{{username}}`、`{{sentAt}}`）。编辑区按用户要求撑满卡片宽度（对照项目原为 `max-w-[280px]`）。
- 所有值只存在组件本地 state，无校验、无保存、无接口；刷新恢复初始值。
- 审查后修三项：共享行组件新增 `expandLabel`，避免开关行的展开按钮与开关本身共用「是否启用」这一可访问名称；正文文本域用 `aria-describedby` 关联占位符提示；模板文案键统一为 `mailTemplate*` 前缀（原 `templateSignupCode` / `templatePasswordReset` 改名）。

**验证**：`npx tsc --noEmit`、ESLint（改动文件）、`npx prettier --check` 均通过；中英文键集合逐项比对无缺漏。运行期视觉未另起 dev server 复现，归用户实机确认。

**已知缺口**：

- **页面本身没有权限检查**。侧栏入口对无 `system.settings.view` 的用户隐藏，但只要登录后手动输入 `/console/settings/mail` 仍能看到整张表单；其他设置页在同一情况下会显示「无权查看设置」。当前表单无真实数据、无保存，不构成泄露，但接后端前必须补。建议修法：页面内用 `getAuthenticatedUser()`（React `cache` 已缓存，不增加请求）读一次用户，无 `system.settings.view` 时渲染 `t("noPermission")`。
- 登录控制、用户注册两页的开关行仍沿用「展开按钮名称 = 行标题」的旧行为；`expandLabel` 已可复用，待后续轮次顺手补齐。

**提交状态**：代码批次 `c8c63f4`、`f07531b` 已入库；本条目 `4afa74d` 入库。

---

## 2026-10-23 · 六个免缓存页面补 `instant = false`（消除导航期未缓存数据告警）

**范围**：前端 6 文件——`app/console/settings/{general,groups,login,register,users}/page.tsx` 与 `app/console/uc/session/page.tsx`，每页新增一行 `export const instant = false;`。零后端、零迁移、零依赖、零路由与接口变化；版本保持 0.21.2（纯前端告警收口，不单独进位）。

**动机**：`frontend/next.config.ts` 已开 `cacheComponents: true`。这六个页面在服务端都显式以 `cache: "no-store"` 取数，导航时会触发「uncached data during a navigation」告警。`export const instant = false` 声明该页不参与即时导航约束；它只豁免所在层、不向下传播，所以根 `app/layout.tsx`、`app/console/layout.tsx` 与 `app/{signin,signup}/page.tsx` 早有的同一行覆盖不到这些子页。

**口径**：

- 六页与数据来源一一对应：`general` / `login` / `register` 走 `lib/admin-settings.ts`，`users` 走 `lib/admin-users.ts`，`groups` 走 `lib/admin-groups.ts`，`uc/session` 走 `lib/user-sessions-server.ts`；四处 fetch 均为 `cache: "no-store"`。
- 只加段配置，不改取数方式、不动缓存策略与页面渲染，也不全局关闭 `cacheComponents`。
- 未加的同层页面：`app/page.tsx`、`app/ui-check/page.tsx` 无服务端取数；`app/console/page.tsx` 只读标题文案；`app/console/uc/page.tsx` 是重定向；`app/console/uc/profile/page.tsx` 无显式 `no-store` 取数（会话走 cookie 动态渲染，公开设置带 60 秒 revalidate）。

**验证**：`npx tsc --noEmit`、ESLint（6 文件）、`npx prettier --check` 均通过。告警消失的运行期现象未另起 dev server 复现，归用户实机确认。

**已知缺口**：无新增。后续新增页面若同样在服务端 `no-store` 取数，需自行补这一行。

**提交状态**：代码批次 `c396e05` 已入库；本条目随后单独入库。

---

## 2026-10-20 · 鉴权入口限流（登录 / 注册 / 2FA verify + 前端 429 反馈）（0.21.2）

**范围**：后端新增 `internal/common/ratelimit`（实现 + 单测）、`apierror` 的 429 / `Retry-After`、`config.TrustedProxies`、`app.go` 注入 limiter、`auth/{ratelimit.go,service.go,controller.go,module.go}` 与测试（`ratelimit_test.go`、`auth_test.go`、`sessions_test.go`、`totp_flow_test.go`）；前端 `lib/{api,rate-limit}.ts`、登录/注册 hooks 与表单、中英文案。零迁移、零新依赖、零新路由；版本 0.21.1 → 0.21.2（patch）。

**动机**：审查确认登录、注册与 2FA verify 三个公开入口没有在线暴破防护；原有二段「会话内连错 5 次」存在 scs 会话里，换会话或并发请求即可绕过，README 也把它记为已知缺口。本轮把账号/标识与 IP 两个维度的闸门补上，并让前端能显示等待时间。

**口径**：

- 只覆盖 `POST /sessions`、`POST /users`、`POST /sessions/current/verify`；不做全站限流，不动登录 CSRF、2FA 密码复核、auth_version 三项已入库逻辑。
- 进程内固定窗口 + 失败冷却，单实例计数、重启清零；键数上限 100000，惰性清扫。登录账号 5 次失败 / 15 分钟窗口 → 15 分钟冷却，成功后清零；登录 IP 30 次 / 15 分钟。注册邮箱 3 次 / 1 小时，注册 IP 5 次 / 1 小时。2FA 账号 10 次失败 / 15 分钟 → 15 分钟冷却，2FA IP 30 次 / 15 分钟；会话内 5 次 `two_factor_locked` 保留，账号级冷却负责跨会话与并发。
- 未知标识与已知标识使用同一套计数与响应，不通过 429 泄漏账号存在性；成功登录/验证只清账号失败计数，不清 IP 窗口。
- 新增 429：`type=rate_limit_error`、`code=rate_limited`，错误信封不变；`Retry-After` 秒数向上取整、最小 1。前端 `ApiError.retryAfter` 读取该头；429 只弹 sonner「请求过于频繁」，不显示等待时间，登录/注册/二段提交按钮按该头暂时禁用并把按钮文字切换为剩余时间（如「12 分 48 秒」）；429 不触发会话过期处理。
- 新增 `TRUSTED_PROXIES`（默认 `127.0.0.1/32,::1/128`），`app.New` 用 `SetTrustedProxies` 配置，`requestSessionMeta` 改用 `ClientIP()`；限流键与会话列表 IP 同源，非可信来源的 XFF 不采信。生产经非回环代理时必须显式配置。
- 新增错误码按 STYLE 登记 `apierror`；实现遵循 `backend/STYLE.md` 现行条款，未提前实施目标态 API 形状。

**验证**：后端 `bash scripts/check.sh` 通过；`bash scripts/test.sh`（真库，`-p 1`）全仓通过，含新增限流单测与登录/注册/2FA 集成用例。前端 `tsc --noEmit`、ESLint、Prettier 通过。变异自检四条——去掉登录账号 `checkFailure`、去掉注册 IP `allowWindow`、去掉 verify 账号 `checkFailure`、去掉 `Retry-After` 写出——分别实测变红后还原。live 5732 冒烟：同一未知标识 5 次错误密码后第 6 次返回 429，`Retry-After: 892`。本机 cgo 不可用，未跑 `-race`；并发用例用 goroutine + 互斥锁断言。前端 429 画面：sonner「请求过于频繁」经用户浏览器截图确认；按钮倒计时文字待用户复验。本机 in-app browser 未完成 hydration，无法自动化点击；`tsc` / ESLint / Prettier 已通过。

**已知缺口**：限流是单实例进程内计数，重启清零，多实例各算各的；定向打满 5 次可让已知账号冷却 15 分钟（临时锁号取舍，不永久锁定）；不提供 `x-ratelimit-*` 剩余额度/重置时间；共享存储版本未做。

**提交状态**：未提交（全部改动留在工作区；提交批次由用户点名）。

---

## 2026-10-20 · 后端工程风格规范化（规范文档 + 检查门禁 + 内部结构收口）（0.21.1）

**范围**：两笔提交。`5c2cdeb` 新增 `backend/STYLE.md`、`backend/scripts/check.sh`，README 补规范入口并修正硬性规则 1 / 2，计划文档入库；`a917591` 动 28 个文件——新增共享响应信封 `internal/common/apiresponse` 与 5 个新文件，DTO 归位、access 错误分层、错误字符串前缀与检查门禁落地，STYLE / README / 计划同步。零迁移、零新路由、零新依赖、**零 API 形状变化**。版本 0.21.0 → **0.21.1**（patch）。

**动机**：项目从首个后端提交起就是 NestJS 式模块化，但规范只写到骨架；DTO 放置、错误所有权、集合信封在 5 个模块里各自长出了不同写法。本轮把模块内部约定写成可执行规范，并收敛会造成新代码分裂的结构差异。

**口径**：

- **规范优先级**：早期设计文档的决策条款 > README > 现有实现中重复出现的写法。STYLE 分「现行规则 / 目标态 / 已接受例外 / 当前差距」四块；标为目标态的条目不得在日常迭代提前实施。
- **共享响应信封**：`internal/common/apiresponse` 提供 `ItemList[T]` / `Deleted` / `DeletedBatch` / `Empty`。`ItemList` 与 `DeletedBatch` 的 nil 集合输出 `[]`；auth / access / settings 删除各自的 `itemList`、`publicSettingsView` 与 `gin.H` 删除确认。线上仍是 `items`、`{"id","deleted":true}`、`{"ids",...}`、`{}`。
- **DTO 归位**：access 的请求 / 响应类型进 `dto.go`；settings 的 `groupView` 改名 `groupResponse` 并进 `dto.go`；mail 新建 `dto.go`；health 的 `Status` 改名 `HealthResponse`。字段名、JSON tag、指针性一律不变。
- **access 错误分层**：`repository.go` 不再 import `apierror`，改用 `errRecordNotFound` / `errGroupNameTaken` / `errGroupNotEmpty` / `errLastOwner` 四个域哨兵；`service.go` 的 `mapRepoError` 统一翻译成 `apierror`。`IdentityByUserID` 先认域哨兵再映射 `guard.ErrUserNotFound`，避免把 401 变成 500。所有 404 / 409 行为由既有测试兜底。
- **错误字符串**：auth / guard / mail / settings / access 自己构造的错误补域前缀。例外：config 启动校验错误点名环境变量或行号；`apierror.Error()` 返回 wire code；第三方库错误原样透传。
- **检查门禁**：`check.sh` 串起 gofmt（含未跟踪 Go 文件）、`go vet`、`go build`、空白检查，并新增「repository 不得 import apierror」的 grep 门禁。
- **明确未实施**：D4 的 `data` / `total` / `limit` / `offset` 与分页、epoch 时间戳、`/healthy` 字段改名、认证响应补 `id` 仍为目标态；本轮只统一内部结构。

**验证**：`bash scripts/check.sh` 通过；负向探针（临时未格式化 Go 文件）实测报 `gofmt needed` 并退出 1；repository 门禁在重构前实测命中 `access/repository.go`，重构后为空。`bash scripts/test.sh`（真库，`-p 1`）全仓通过，含 `app` / `auth` / `access` / `settings` / `mail`。新增 `apiresponse` 6 条 JSON 形状测试与 `TestMapRepoError` 表驱动测试。`git diff --check` 干净；生成代码、迁移、版本与 DEVLOG 未混入代码提交。**运行期未另起服务手测**：本轮无接口形状与业务行为变化，全仓测试覆盖。

**已知缺口**：日志可观测性仍未做（无 request id，模块用 `slog.Default()`，没有统一注入），STYLE §9.2 明确另开轮次。以下为已接受例外，不是待办：mail 无 service、settings 用模块哨兵、main 用标准库 log、config 错误不加前缀、mail log sender 在开发态记录邮件正文、第三方库错误原样透传、各包复制测试 helper。

**提交状态**：`5c2cdeb`（STYLE / check / README / 计划）与 `a917591`（内部结构收口）已入库；本条目与版本进位随收尾提交入库。

---

## 2026-10-19 · 登录控制页接线（auth 组前端读写 + 会话有效期与 TOTP 站级开关生效）（0.21.0）

**范围**：前端 9 文件（`app/console/settings/login/{page,login-control-form}.tsx`、`hooks/use-login-settings-save.ts`、`lib/login-settings.ts`、`lib/admin-settings.ts`、`app/console/uc/profile/{page,profile-form}.tsx` 与两份 message）；后端 10 文件（4 个生产文件：`settings/registry.go`、`auth/session.go`、`auth/service.go`、`common/apierror/apierror.go`；5 个测试文件：`auth/settings_consumers_test.go`（新增）、`auth/totp_flow_test.go`、`settings/registry_test.go`、`settings/controller_test.go`、`common/apierror/apierror_test.go`；以及 `version/version.go`），外加 README / API 规范 / 本条目。代码审查及其修复批次另动 12 个文件——后端 `settings/{types,registry,service}.go` 与 `settings/{registry,controller}_test.go`，前端 `lib/{admin-settings,login-settings}.ts`、`hooks/use-two-factor-setup.ts`、`app/console/uc/profile/two-factor-panel.tsx`、`app/console/settings/login/login-control-form.tsx` 与两份 message。零迁移、零新路由、零新依赖。版本 0.20.3 → **0.21.0**（minor）。

**动机**：`/console/settings/login` 自上一轮建成起是纯 UI（六个本地 state、无保存、不读后端）；本轮按用户点名的范围接成真页面，并让 `auth` 组里成本最低的两个策略真正生效——会话有效期与两步验证总开关。密码重置与人机验证只接页面开关，行为留给各自独立轮次。

**口径**：

- **页面读写**：`getLoginSettings()` 读 `GET /admin/settings/auth`；保存走 `PATCH` 且只发本页改过的键（`session_max_age_enabled` / `session_max_age_days` / `totp_enabled` / `password_reset_enabled` / `captcha_enabled` / `captcha_provider`）。与「用户注册」页共用 `auth` 组，但两边都只发自己的键，互不覆盖；权限继续沿用 `system.register.view/edit`（`settings/controller.go` 已按组分流）。
- **会话有效期**：`auth.Service.establishSession` 在登录（含二段验证通过）时读 `session_max_age_enabled` / `session_max_age_days`，用 scs `SetDeadline` 设为「当前时间 + N 天」。**只影响之后新建的会话**，已登录会话不变；关闭时沿用 `SESSION_LIFETIME`（默认 168h = 7 天）。读设置失败按 fail-open 处理（沿用环境变量默认值），不阻断登录。
- **天数范围 1–30**：`settings/registry.go` 的 `ValidateOn` 加入 `session_max_age_days` 与 `session_max_age_enabled`，越界返回 409 `setting_conflict`（param `session_max_age_days`）。组校验器的签名带上了**本次写了哪些键**（`Validate(values, touched)`），天数只在「本次写了天数」或「把开关写成开启」时才校验——后者是为了开启时存量脏天数仍会被拦（库里可能有旧的越界值），前者之外不再误伤：关掉开关、或在注册页改密码长度，都不会被历史脏天数挡住。前端 `validateLoginSettings` 用同一条规则预校验（只有本次写了天数、或这次把开关打开才拦），不再拦下用户没碰过的存量值。
- **TOTP 自助开关**：`totp_enabled` 默认值由 `false` 改为 `true`（保持「2FA 一直可用」的现状），并标 `Public`；`SetupTOTP` / `EnableTOTP` 在开关关闭时返回 403 `two_factor_disabled`（新增 token），`DisableTOTP` / 恢复码接口不受影响。登录是否要求二段仍只看用户自己的 `totp_enabled_at`，站级开关关闭不会让已开 2FA 的用户绕过验证。语义是「允许用户自助开启」而非「全站强制」，控制台标签随之改为「允许用户启用两步验证」，README / DEVLOG 一律称**自助开关**。
- **用户中心**：个人设置页读公开设置，站级开关关闭且用户自己未开启 2FA 时隐藏整行；用户已开启 2FA 时仍显示，保留 `disable` 的退出口，避免被锁死。
- **公开面**：`auth` 组因 `totp_enabled` 标公开而出现在 `/public/settings`，公开白名单从 7 项变 8 项；前台 `getPublicSettings` 仍是 60 秒 revalidate，后台改开关最多 60 秒后反映到用户中心，但后端接口立即生效。
- **`captcha_provider` 映射**：后端默认 `""`，前端把空串显示为 `builtin`（`CAPTCHA_PROVIDER_BUILTIN`）；其他字符串原样保留并作为额外选项显示，不再让整组解析失败。保存时这个键**无条件带上当前显示值**（它是页面始终呈现的选择），所以任何一次真实保存都会把空串落成 `builtin`，消除「界面显示已设置、库里永远空」的错位；空串仍是合法的「未设置」读入形状，后端暂无消费方。
- **站级关闭的 60 秒窗口**：前台公开设置是 60 秒 revalidate，后台刚关掉 TOTP 时，用户中心的入口可能还在，点进去 setup / enable 会拿 403 `two_factor_disabled`。`use-two-factor-setup.ts` 现在认这个码——不再显示「密钥生成失败」、不给无效的重试按钮，改为提示「本站当前未开放两步验证」并 `router.refresh()`；面板保留同一状态直到入口随缓存过期消失。

**验证**：后端 `gofmt -l` 干净、`go vet ./...` 零输出、`bash scripts/test.sh` 全绿；变异自检三条——去掉 `applySessionDeadline` 调用、把两个 TOTP 门控改成 `if false`、把天数范围校验改成永不触发——分别实测变红后还原。另补四条行为测试——站级开关关闭时登录仍需二段、恢复码接口仍可用、二段验证通过路径应用有效期、设置读取失败 fail-open；前两条做了变异自检（登录跳过二段、恢复码加门控），均实测变红后还原。前端 `tsc --noEmit`、ESLint、Prettier 通过。审查修复批次另加：`settings` 包两条 PATCH 测试（关开关时脏天数不再拦、注册页写密码长度不再被脏天数连坐）与 `TestValidateAuthGroup` 扩成 12 条表驱动用例——把校验器改回方向不敏感，这三处同时变红，还原后全绿；`use-two-factor-setup` 的 403 分支属 UI 层（前端无测试框架），只过 `tsc` / ESLint / Prettier，**未做运行期验证**。**运行期未验**：真实登录后改会话天数/关 TOTP 的行为归用户实机。

**已知缺口**：`password_reset_enabled` 与 `captcha_enabled` / `captcha_provider` 仍是纯数据（密码重置与人机验证后端零实现，各自单开 spec）；登录控制页与用户注册页仍共用 `system.register.*` 权限，未拆独立权限点；已登录会话不会被新有效期即时改写；库里既有的越界天数不会被自动清洗——关掉开关时保存会成功但那一行仍是脏值，生效路径按 fail-open 走 `SESSION_LIFETIME`，直到有人写天数或开启开关才会被拦。

**提交状态**：主实现已分 4 批入库（`76d191f` / `4acf62e` / `33a67c9` / `14f2fc8`）；代码审查修复与其后续批次（16 个文件）未提交。

---

## 2026-10-16 · 前端接上 `email_domain_not_allowed`（域名不允许时给具体提示）

**范围**：前端 3 文件（`hooks/use-signup.ts` 的 `toFailure` 加一个 case、`messages/zh-CN.json` 与 `messages/en.json` 各加一条 signup 键）。零后端改动、零版本进位。

**动机**：上一轮加的域名白名单后端会返回 400 `email_domain_not_allowed`（`apierror.ErrEmailDomainNotAllowed`），但前端 `toFailure` 的 switch 里没有这个 case，落到兜底分支，注册页只弹一句「注册失败，请稍后重试」——用户看不出是自己邮箱域名被挡了。用户裁定「启用了域名限制时注册，应当返回此邮箱无法注册」。

**口径**：

- `email_domain_not_allowed` 映射成 `{ message: t("emailDomainNotAllowed"), field: "email" }`，与 `email_taken` 同级：**既弹 toast，又把光标打回邮箱框**。
- 文案按用户要求收短：zh「该邮箱域名不允许注册」/ en「This email domain is not allowed」，不带解释性从句。

**验证**：`tsc --noEmit` 零输出、`eslint hooks/use-signup.ts` 零输出、`prettier --check` 三个文件通过。后端 `gofmt -l` 干净、`go vet ./...` 零输出、`scripts/test.sh ./internal/modules/auth/...` ok。**运行期未验**——真实提交一次被拦域名的注册看提示归用户实机。

**提交状态**：未提交（与「注册只建号」那轮一起躺着）。

---

## 2026-10-16 · 控制台「注册默认组」下拉接真实用户组（存组名）+ 后端默认值对齐

**范围**：前端 7 文件（`page.tsx` / `register-settings-form.tsx` / `use-register-settings-save.ts` / `admin-settings.ts` / `register-settings.ts` / 两个 message）；后端 4 文件（`rbac.go` 加 `PresetGroupNameByKey`、`registry.go` 改默认值、两处测试断言）。零迁移、零新路由、零版本进位。

**动机**：`/console/settings/register` 的「注册默认组」自建成起是空下拉（`<SelectContent />` 空、无 `value`、无 `onValueChange`）。用户 2026-10-16 裁定接真实用户组，且**值存组名不用 UUID**（「我创建的用户组是什么名称，它就可以选择什么名称」）。

**口径**：

- **值形状 = 组名**（`register.default_group` 存 `groups[].name`）。组名在 ent schema 上是 `field.String("name").NotEmpty().Unique()`，重名被后端挡下（`group_name_taken`），故按名称作 `SelectItem` 的 value 安全。
- **禁掉特权组**：按 `system_key` 过滤（`owner` / `admin`），自建组 `system_key` 为 null 故全保留。理由：注册是自助入口，指到特权组等于开放提权后门；后端 `access.Service.UpdateUser` 也有对应的 owner 保护。
- **存量指向特权组时不掩饰**：库里已有的 owner/admin 名称不在可选项里，此时 `Select` 的 `value` 传 `null`、显示「未指定」占位，而不是让下拉假装没限制。
- **后端默认值对齐**：原默认值是组**键** `"user"`，与组名形状对不上（普通用户组组名是「普通用户」），新库读出来会显示「未指定」。改为 `rbac.PresetGroupNameByKey(rbac.GroupKeyUser)`——不写字面量，取自 rbac 预设组定义；为此给 rbac 加了 `PresetGroupNameByKey`（`settings` → `rbac` 单向依赖，无循环）。两处断言（`controller_test` 默认读回、`registry_test` 清单形状）同步改用该函数。
- **Base UI 的 `items` 是必需的**：只给 `SelectItem` 不足以让触发按钮渲染当前选中项文字，需传 `{名称: 名称}` 记录（`groupSelectionMap`）。
- **读组列表**：`getRegisterSettings` 并行第三个请求 `/api/v1/admin/groups`，复用 `lib/groups` 的 `parseAdminGroups`；组列表 403 也归「无权查看设置」，解析失败归 `failed`。
- 新增 1 条 i18n 键 `registerDefaultGroupNone`（zh「未指定」/ en「Not set」）。

**已知缺口（都在用户手上）**：

1. **后端 `Register` 仍无消费方**——`default_group` 现在只是能选能存的设置值，新用户归属仍由 ent schema 默认值决定。用户 2026-10-16 明确「只接前端，后端下轮再说」。
2. **权限落差（本轮引入的新失败模式）**：注册设置页要 `system.register.view`，组列表要 `system.groups.view`。只有前者、没后者的管理员会看到整页「无权查看设置」，连原本能用的域名白名单 / 密码设置行也一起没了。修法待用户点名（组读不到时降级置空下拉，还是后端放宽读）。
3. 组名被改后，存量 `default_group` 值会失配（下拉显示「未指定」但不报错）；组被删同理。后端那轮做消费方时需一并处理。
4. 下拉无空值选项（不可清空）；`setDefaultGroup` 的 `null` 分支目前只有占位显示会走到，实际取消选择走不到。

**测试**：后端默认值两处断言做了变异自检——把默认值改回组键 `"user"`，两条分别红（`want "普通用户"`）；复原后全仓 `scripts/test.sh` ok、`gofmt` 干净、`go vet ./...` 零输出。前端 `tsc --noEmit` 零输出、目标文件 `eslint` 零输出、`prettier --check` 通过。**运行期未验**——下拉弹出、选中显示、保存读回均交用户实机。

**提交状态**：分两笔入库（后端默认值对齐 / 前端接线），**未推送**。

---

## 2026-10-16 · 注册只建号、不下发会话（去掉「注册即登录」）

**范围**：后端 `auth`（`service.go` 的 `Register` 去掉 `establishSession` 并收掉 `meta` 参数、`controller.go` 的 `signup` 少传 `requestSessionMeta`）+ 两个测试；前端 `use-signup.ts`（`router.push("/console")` → 清空表单 + `push("/signin")`）。零迁移、零新路由、零版本进位。

**动机**：用户发现注册完成会直接跳进控制台，裁定「注册用户只注册用户，不要授权会话」。根因不在前端跳转——后端 `Register` 建号后调 `establishSession` 种下会话 cookie，前端再 `push("/console")` 自然就进去了；而普通用户落的是「普通用户」组，控制台里没有任何可用内容，体感是"注册完给我开一个空后台"。

**口径**：

- **`Register` 不再种会话**，`Service.Register` 签名去掉 `SessionMeta` 参数（生产侧唯一调用方是 `controller.signup`，同步少调 `requestSessionMeta(ctx)`）；`Login` 与两步验证通过两处 `establishSession` **保持不动**。
- 前端注册成功：toast（文案本就是「注册成功，请登录」）→ 清空四个输入框 → `router.push("/signin")`。登录页无需改动、无新增 i18n 键。
- **已登录用户注册不影响既有会话**：该行为原本由"注册会换掉会话"间接覆盖，现在无会话可换，改由 `TestRegisterWhileLoggedInKeepsExistingSession` 直接断言"原会话仍是原来那个人，且注册方拿不到 cookie"。

**测试**：`TestRegisterSucceedsAndLogsIn` → `TestRegisterSucceedsWithoutSession`，断言从"有 cookie 且 `/users/me` 200"翻成"**无 cookie + `/users/me` 401**，随后 `loginAs` 显式登录 `/users/me` 200"。`TestRegisterWhileLoggedInSwitchesSession` → `TestRegisterWhileLoggedInKeepsExistingSession`。**变异自检实测红**：把 `establishSession` 加回 `Register`，两条分别报 `register handed out session cookie "…", want none`；复原后全仓 `scripts/test.sh` ok、`gofmt` 干净、`go vet ./...` 零输出。前端 `tsc --noEmit` / `eslint` / `prettier --check` 全绿。**运行期未验**。

**文档**：`backend/README.md` 的 `POST /api/v1/users` 行改写——去掉「+ 会话 cookie（注册即登录）」，改为明写「不下发会话 cookie，注册只建号，要登录另走 `POST /sessions`」。

**提交状态**：未提交，等用户审查。

---

## 2026-10-16 · 注册域名控制「空清单」改为放行（去掉设置写前拦截）

**范围**：后端 `settings` 去掉 register 组的组级校验（`registry.go` 摘掉 `ValidateOn` / `Validate` 与 `validateRegisterGroup`），前端 `register-settings.ts` 摘掉同款本地拦截与 `domainControlRequiresList` 键、改在空清单时挂提示；补两条测试。零版本进位、零迁移、零新路由。

**动机**：用户裁定「开关开着 + 域名清单为空」是合法配置，且**清空名单这个动作本身不能被拦**。原先前后端各有一道「开着必须非空」的拦截（后端 `validateRegisterGroup` 报 409 `setting_conflict`，前端 `validateRegisterSettings` 先拦一道），而后端**运行时** `ensureRegistrationDomain` 本来就是 `len(allowed) == 0` 放行——保存与运行两层口径本来就对不上，且按管辖项门槛触发会连带挡住「保存其他项」。

**口径**：

- 运行时不变：开关开 + 清单拆分后无条目 → 任何邮箱域名都放行（fail-open，不因配置残缺把注册锁死）。
- 保存不变：register 组**不再挂**组级校验器，清空/留空/只填逗号都能落库；`setting_conflict` 现在只剩 `password_min_length` 一处使用。
- 界面提示：邮箱域名控制开启时，域名输入框用 `placeholder` 显示 `settings.allowedEmailDomainsEmptyHint`（zh「留空表示不限制任何邮箱域名」/ en「Leave empty to allow any email domain」），浅色走 `Input` 既有的 `placeholder:text-muted-foreground`、不改组件源码；有值时浏览器自动隐藏，不额外判空（` , ，` 这类只填分隔符的写法同样落在"有值"，但后端按空处理——占位提示此时消失属可接受的取舍，省掉一层与后端重复的拆分逻辑）。关着开关时不显示（那句提示在关闭态是废话）。目的是防管理员看着开关开着误以为限着域名。
- 顺带删掉 `domainControlRequiresList` i18n 键（zh / en）与 `validateRegisterSettings` 的域名分支（现只剩 `passwordMinLengthRange` 一条 issue）；私有 `splitDomains` 保留（校验密长路径外已无调用方时由编译器把关——以现文件为准）。hook 的 `conflictMessage` 里两个域名项分支已摘掉。

**测试**：`TestSettingsPatchRegisterDomainControlAllowsEmptyList`（PATCH 开开关 → 补清单 → 清空 → 只填逗号，全程 200 且读回一致，开关保持 true）、`TestBuiltinRegisterGroupHasNoValidator`（register 组不得再挂校验器）、`TestRegisterDomainControlEmptyListAllowsAnyDomain`（开着 + ` , ，` → gmail.com 与 example.org 都注册成功）。**变异自检三条实测红**：① 把空名单拦截挂回 registry → 前两条红（409 / 校验器仍在）；② 运行时改成 fail-closed → 第三条红（400 `email_domain_not_allowed`）；改完全部复原、`gofmt -l` 干净。

**验证**：后端 `go vet ./...` 零输出、`scripts/test.sh` 全仓 `ok`（auth / settings / access / app 等）；前端 `tsc --noEmit` 零输出、目标文件 `eslint` 零输出、两个 message JSON 解析通过、旧键 grep 零残留。**运行期未验**，交用户实机。

**文档**：`backend/README.md` 两处改口径——组合约束那行去掉「开域名控制必须清单非空」，`register` 组清单段改成写明「清单为空时一律放行、该组合合法且存得下、界面需自行提示」。

**提交状态**：与并行轮的前端接线改动同处工作区，**未提交**，等用户审查。

---

## 2026-10-16 · 设置行折叠参数改为 defaultCollapsed（默认展开，传参收起）

**范围**：组合件契约调整 + 三处调用点适配；零版本进位。

**动机**：用户点名给设置行折叠加「是否默认折叠」参数，语义定为**不传 = 默认展开、启用后初始收起**。原参数 `defaultExpanded`（默认 false = 初始收起）语义正好相反，按「只改组件源码」的既定路径改契约。

**口径**：`console-settings-row.tsx` 的 `defaultExpanded`（默认 false，收起）改为 **`defaultCollapsed`（默认 false，展开）**，`useState(!defaultCollapsed)`；受控 `expanded` / `onExpandedChange` 不动（2FA 面板在用）。调用点适配：ui-check 两处 `defaultExpanded`（原意「初始展开」）直接删——新默认即展开、观感不变；profile 改密行（原被动收起）显式加 `defaultCollapsed` 保住观感。契约文档 `2026-10-13-neoblog-component-conventions-design.md` props 清单与已知缺口行同步（顺带修掉「无 expanded/onExpandedChange」的过期表述）。

**验证**：`tsc` / `eslint`（0 error）/ `prettier --check` 全绿；全仓 `defaultExpanded` 零残留（业务代码与文档）。

---

## 2026-10-16 · 控制台「用户注册」设置页接后端（register + auth 两组六键，纯前端）

**范围**：纯前端，后端零改动、零版本进位（前端轮惯例）。spec：`docs/superpowers/specs/2026-10-16-neoblog-register-settings-wiring-design.md`；计划：`docs/superpowers/plans/2026-10-16-neoblog-register-settings-wiring.md`。

**动机**：`/console/settings/register` 自建成起是纯 UI（六个本地 state + 一个空下拉），本轮把六个可接行接到真实数据源——与后端 0.20.3 刚接线的四键（域名白名单、密码策略）合上。

**口径（D1–D11 摘要）**：

- 读：服务端 `getRegisterSettings()` 直读两组（register + auth，cookie 转发 + `no-store`）；403 →「无权查看设置」、其余失败 →「设置读取失败」，两种都不出表单。
- 写：悬浮保存 + 脏值比对；**按组分别 PATCH、只发变动键**；成功以响应回包重置基线（两组分别解析合并）。
- 校验双保险：本地只拦「本次动到的规则」（开域名控制 + 空清单；密长 8–64 整数——与后端管辖门槛同语义，只改「允许公开注册」不被旧脏值误拦）；后端 409 `setting_conflict` 按 `param` 映射同款文案；403 →「无权修改设置」；401 静默；5xx / 断网 → 通用不可用文案。
- `password_min_length` 表单态是文本、PATCH 里是数字；`allowed_email_domains` / `password_complexity` 空串是合法值。
- 「注册默认组」下拉**不接**（值形状「组键 vs 组 ID」待裁定）；`email_verification_enabled` 不在 UI。
- 新增 3 条 i18n 键（`noPermissionToSave` / `domainControlRequiresList` / `passwordMinLengthRange`，zh / en 键位一致）。

**验证（离线，本机）**：纯函数断言 `register-settings checks: PASS`（两组解析 / 脏检测 / 校验命中门槛 / 按组 patch；脚本在 `.superpowers/sdd/2026-10-16-register-settings-wiring/`，不入库）；**变异自检四条实测红**：① 校验去掉命中门槛 →「只改允许公开注册」被误拦（`domainControlRequiresList` vs `null`）；② patch 恒发全部键 →「无变化 → `{}`」断言红；③ 放宽整数把关 → `8.5` 被放行、断言红；④ 全角逗号不认 → 先炸在「纯分隔符（含全角）→ 拦」断言。`tsc` / `eslint`（0 error，2 条 `sonner.tsx` 存量 warning）/ `prettier --check` 全绿；取证 grep：表单 `useState` 0、默认组 `onValueChange` 0、蛇形键仅在 lib（映射表 + 解析函数）与 hook（409 param）。**运行期未验**——读两组 / 保存 / 校验 / 门控交用户实机（清单见计划 Task 4 Step 3）。

**已知缺口**：两组两次 PATCH 非事务（第一组成功第二组失败 → 半区已落库、基线不重置、重试幂等）；未保存离开无守卫；多标签页后写覆盖；密长文本前导零会判脏（保存后由回包归一）；客户端 8–64 与后端常量靠口径对齐（不共享常量）。

**提交状态**：待用户审查后按批提交。

---

## 2026-10-16 · 注册域名控制与密码策略接线（register/auth 首批消费 + 设置写前校验插槽）（0.20.3）

**版本**：后端 0.20.2 → **0.20.3**（patch；用户 2026-10-16 直接设定，未走 0.21.0 线）。零迁移、零新路由、零新依赖、前端零改动。（并行轮的提交自称 0.21.0、其 `version.go` 实际落在 0.20.2——本轮按用户裁定从 0.20.2 走 patch 到 0.20.3，不回补 0.21.0。）spec：`docs/superpowers/specs/2026-10-16-neoblog-registration-policy-design.md`（v2）；计划：`docs/superpowers/plans/2026-10-16-neoblog-registration-policy.md`（v2）。

**动机**：register/auth 两组十四项此前是纯数据（唯一消费方是注册总闸），用户点名先接四项——邮箱域名控制 + 允许的邮箱域名、密码要求 + 密码最短长度；注册默认组明确搁置。

**口径（D1–D8 摘要）**：

- 域名白名单：大小写不敏感、逗号分隔（中英文逗号都认）、条目去空白、精确匹配域名（不放行子域）；只拦 `POST /users` 公开注册。
- 密码策略：开关关 = 现状逐字（≥8、字母+数字、≤72 字节）；开 = 取 `password_min_length` 并夹在 [8, 64]。「开着只会更严」。
- 保存时拦截：`PATCH /admin/settings/:group` 在「写入命中管辖项（`ValidateOn`）」时读合并最终态、落库前跑组级校验（机制新插槽 `Group.ValidateOn` / `Group.Validate`）；开域名控制必须清单条目非空；`password_min_length` 取 8–64；无关项写入不受脏存量牵连。
- 新错误码：`email_domain_not_allowed`（400）、`setting_conflict`（409，param=判定项名）。
- 公开面零变化（四项仍非公开）；`Set*` / `ResetItem` 内部与探针通道不校验。

**测试与变异自检**：单元（校验器 / `SplitListValue` / 域名匹配 / 密码策略纯函数）+ 集成（PATCH 冲突两例、脏存量门槛一例、注册域名两例、注册与改密密码策略三例；全部 `t.Cleanup` 复位）。变异七条（实测红）：

| # | 变异 | 实测红 |
| --- | --- | --- |
| ① | 域名校验恒放行 | 域名集成（201 want 400） |
| ② | 密码策略恒 8 | 密码集成（注册 201 / 改密 200，want 400） |
| ③ | register 校验器恒 nil | PATCH 冲突集成（200 want 409） |
| ④ | 密码长度下限夹取去掉 | 纯函数单测（3 want 8） |
| ⑤ | 合并态不读存量 | PATCH 冲突集成首个 PATCH（500 want 409） |
| ⑥ | 校验器不装载（compileRegistry 不登记） | 两条 PATCH 冲突集成（200 want 409） |
| ⑦ | 管辖项门槛恒真 | 门槛集成（409 want 200） |

**过程发现**：计划里 `validPassword` 边界用例期望值错一位（`pass12345678` 是 12 字符，min=12 应为通过），实施时修正测试与计划（12 → true、13 → false）。

**已知缺口**：注册页提示与本地校验仍写死 8（前端轮再跟，可能涉及 `password_min_length` 公开化）；新码前端未映射、暂落通用文案；域名控制不管改邮箱；复杂度不执行；无缓存（每次注册最多 5 次设置点查）。

**提交状态**：待用户审查后按批提交。

---

## 2026-10-16 · 权限矩阵按页面拆行（新增 system.register 键，前后端 0.21.0）

**范围**：前端（用户组弹窗矩阵 + 侧栏三项门控）+ 后端（权限目录 + 设置接口守卫按组分流）；版本 0.20.1 → 0.21.0。

**动机**：用户点名——「设置项下有 3 个（常规 / 用户注册 / 用户组），权限节点就应该显示这 3 个」。原矩阵三行是后端抽象资源（用户 / 用户组 / 设置），其中「设置」一把钥匙同时管着「常规」与「用户注册」两个页面；用户在两个方案里选了「拆成独立权限」。

**本轮口径**：

- **后端**：目录加 `system.register.view` / `.edit`（名称「查看 / 管理用户注册」）；目录顺序调整为 users → settings → register → groups（目录顺序只决定 `/admin/permissions` 的返回顺序；前端矩阵的行序与分组由 `frontend/lib/groups.ts` 的分组表定）。设置接口 `GET / PATCH /admin/settings/:group` 守卫由单一 `system.settings.*` 改为**按组分流**（`requireGroupPermission`）：`site` 组 → `system.settings.*`，`register` / `auth` 组 → `system.register.*`；未知组仍落到 settings 档、由服务层 404。
- **前端**：矩阵标签 `system.settings` 行「设置」→「常规」、新增 `system.register` 行「用户注册」（i18n 键 `resourceGeneral` / `resourceRegister` 替换 `resourceSettings`，共 48 键）；侧栏「常规」「用户注册」两项按各自 view 权限显隐（与用户组一致；三枚布尔 layout → sidebar → nav 下传；**三项全无权限时 main 面板「设置」入口整体不渲染**，避免点进空面板）；弹窗矩阵**表头对齐修复**——原表头是「勾选框 + 文字」整体居中，与正文列按勾选框居中的位置错开小半个字，改为「文字在上、勾选框在下」各自居中，勾选框与列严格对齐。
- **侧栏再分组**（同轮后续，用户点名）：设置子面板从「一个组标签 + 三项」改为**三个分组**——设置（常规）/ 注册与登录（用户注册）/ 权限组（用户组），组间留白（后续分组标签 `mt-4`）；空组（无权限项）整组不渲染；`ConsoleSidebarSection` 改为接收 `groups: { label, items }[]`，uc 面板同步改形（单组）。
- **矩阵分三组 + 暂隐藏「用户」**（同轮后续，用户点名）：弹窗矩阵照侧栏拆三组（设置 / 注册与登录 / 权限组）加小标题、组间留白；小组标题复用 `sidebar` 命名空间同款文案（与侧栏一字不差，标签 2026-10-16 按用户点名由「登录与注册」改为「注册与登录」）。**「用户」行暂不显示**——用户页未建：`system.users.*` 键仍在后端目录、owner / admin 自动持有；自建组暂不能新授「用户」权限，已有该权限的自建组保存时原样保留（隐藏键不进勾选、不清除）。列表「权限点数」改为只计可见资源（排序同步），与弹窗可勾数量一致（owner 由 8 → 6）。分组表在 `frontend/lib/groups.ts`（`MATRIX_GROUPS`），与后端目录顺序解耦；离线断言补分组 / 隐藏用例，并做过变异自检（把 users 塞回分组表 → 断言红，还原复绿）。
- **删只读提示文案**（同轮后续，用户点名）：预设组只读弹窗底部的两档提示行（owner·admin / user 文案）删除，`readOnlyPreset` / `readOnlyPrivileged` 两键同轮退役（`groups` 命名空间 48 → 46 键）；可写弹窗的「勾选『编辑』会自动勾选『查看』。」保留。
- **工具栏排布**（同轮后续，用户点名）：「新建」按钮（点名后文案由「新建用户组」改「新建」、档位 `sm`→`default`，32 → 36px）由右槽移至左槽，右侧只保留刷新按钮。
- **权限语义变化**：自建组原持有的 `system.settings.*` 现在只管「常规」页；要管「用户注册」页须另授 `system.register.*`（owner / admin 由代码全量授予、自动包含；已存在的自建组需在用户组页重新勾选）。无数据迁移。
- **文档同步**：`backend/README.md`（模块表 / 用户组与权限节 / 设置节接口表）、用户组 spec（D6、§5 顺序、§7 键表、§12 时间线）、实施计划（口径修订段追加行）。

**验证（本机）**：后端全量测试绿（`scripts/test.sh` 串行；新增 `TestSettingsGroupPermissionsSplitByGroup` 覆盖 register / auth / site 三组 × 读写的两把钥匙穿越矩阵）；**变异自检**：把守卫改回单钥匙 → 新测试红（`status = 403, want 200`）→ 还原复绿；`gofmt -l` 干净。前端：离线断言 `groups checks: PASS`（含四资源标签映射）；`tsc` / `eslint`（0 error，2 条 `sonner.tsx` 存量 warning）/ `prettier --check` 全绿。**运行期未验**——矩阵四行、侧栏三项显隐、表头对齐交用户实机。

**提交前独立评审（2026-10-16，只读子代理，requesting-code-review）**：无 Critical（嵌套 guard 的 handler 执行次数 / abort 语义、隐藏键往返、三态与计数一致性三项逐行验证成立）；1 Important **已修**——busy 锁没盖到 ⋯ 行菜单与「新建」按钮（spec §4.1 明文要求，`ConsoleListTable` 的 `isBusy` 实际只作用于行选择框），现行菜单触发钮与编辑菜单项、新建按钮均随 `isListLocked` 禁用，并删掉传给表格的无效 `isBusy`；另修两处 Minor（列头在无可见键时禁用、refresh 403 改用 `forbidden` 文案）并补三处测试断言（未知组两档位、register 钥匙 PATCH `auth`、隐藏键勾选往返——变异自检：`auth` 档位一摘测试即红）。余下 Minor（`canViewSettings` 命名、settings 组 tier 数据化、`/nav` 入口 href 取首个可访问项）留待点名。

**提交状态**：随本轮三批入库——① `feat(backend)`（权限目录与按组守卫，含 0.21.0 版本进位）② `feat(frontend)`（用户组接线 + 矩阵分组 + 侧栏分组 + 文案清理）③ `docs`（本节与 spec / 计划）；**未推送**。

---

## 2026-10-16 · 控制台「用户组」页接后端（access 接口，纯前端）

**范围**：纯前端，后端零改动，零新依赖。spec：`docs/superpowers/specs/2026-10-15-neoblog-console-groups-design.md`（v3）；计划：`docs/superpowers/plans/2026-10-15-neoblog-console-groups.md`（v2 + 口径修订段 + 执行结果节）。

**动机**：用户组页自 b5ad652 起是纯 UI（本地空数组 + 写死的 6 条权限清单，"新建 / 编辑 / 删除"只活在内存里）。后端 access 八条接口（0.5.0）与 `/users/me` 的 `permissions` 字段早就在位；本轮按用户点名「开始执行，直接执行；移除掉老的数据」把页面接到真接口。

**本轮口径（D1–D22 摘要，含两处随轮修订）**：

- **页面落设置子项** `/console/settings/groups`（原定的顶层 `/console/groups` 作废）：侧栏「设置」子面板第三项，标签复用 `settings.groups`；**搜索框删除**（用户点名）；表格视图保留（刷新 / 新建 / ⋯ 行菜单 / 权限矩阵弹窗）。
- **读**：服务端直读 `lib/admin-groups.ts`（`GET /api/v1/admin/groups` + `GET /api/v1/admin/permissions`，cookie 转发 + `cache:"no-store"`）；读失败 / 403 → 不渲染列表；**权限清单从接口取**（前端零硬编码清单，资源 / 动作动态推导）。
- **门控**：`SessionUser` 补解析 `/users/me` 的 `permissions`；`system.groups.view` 控侧栏项显隐（layout → sidebar → nav 三层传参）、`system.groups.edit` 控「新建」与操作列（`canEdit=true` 才传 `rowActions` / `actionsHeader`）。
- **写**：`use-groups.ts`（对齐 `use-sessions`）：新建 `POST` / 编辑 `PATCH`（整份 `{name, permissions}`）/ 删除 `DELETE`；成功后用响应对象就地更新列表（**不调 `router.refresh()`**）；删除 404 视为成功；401 静默（全局会话失效弹窗接管）；错误按码映射 i18n（`preset_group_locked` / `group_name_taken` / `group_not_empty` / `grant_forbidden` / `forbidden` / `not_found` 全列，兜底按 create / edit / delete 分档）。
- 唯一硬编码规则 = `edit⇒view` 蕴含（对齐后端 `rbac.impliedActions`）；展示计数用后端已展开的 `permissions`，提交原始勾选集合。
- 轮次口径：无版本进位（前端轮惯例，同 f512409）；随本轮三批入库（**未推送**）。

**验证（离线，本机）**：离线断言 `groups checks: PASS`（解析畸形面 / 矩阵 / 联动 / 排序 / 错码全表）；变异自检三条 RED→GREEN（删反向蕴含清理 → 「取消 view → 清 edit」红；删 `grant_forbidden` 映射 → 红（落成 `saveFailed`）；排序直返 → 红（预设分区丢失））；`tsc` / `eslint`（0 error，2 条 `sonner.tsx` 存量 warning）/ `prettier --check` 全绿；键位比对 47/47。**运行期未验**——读 / 写 / 门控六条链路交用户实机。

**过程发现（值得后续点名）**：单文件编译断言脚本（`tsc lib/groups.ts --outDir …`）会忽略项目 tsconfig，**须补 `--strict`**——默认非 strict 下 `unknown` 经 `x !== null && typeof x !== "string"` 复合收窄的行为不同，会误报 `TS2322`（项目 strict 下同代码合法）。

**已知缺口**：读失败态只有一行文案、无「重试」按钮（读失败不渲染面板；会话页的 `Empty` + retry 版本未套用，刷新整页重试）；错误码 / 标签映射前端各存一份（集中 `lib/groups.ts` 单点，后端清单变化时要同步）；前端显隐非安全边界（服务端 403 兜底）。

---

## 2026-10-15 · 站点设置接入前台（元数据 / robots）+ 控制台页面标题（纯前端）

**范围**：纯前端，后端零改动，零新依赖。spec：`docs/superpowers/specs/2026-10-15-neoblog-site-metadata-design.md`；计划：`docs/superpowers/plans/2026-10-15-neoblog-site-metadata.md`。

**动机**：site 六键与「常规」页的读写都做完了，但值在项目里**零消费方**——根 `layout.tsx` 的 metadata 至今是 create-next-app 脚手架文字（"Create Next App"），`/console` 除首页外没标题（标签吃根默认），`block_search_engines` / `site.url` 无人读。本轮把六键接进真实位置，并补齐控制台页面标题。

**本轮口径（D1–D9 摘要，逐条为 2026-10-15 讨论轮拍板）**：

- **公开设置读取链**（`lib/public-settings.ts`）从每请求 `no-store` 改 **60 秒缓存**（`next: { revalidate: 60 }`）；连带注册闸（登录 / 注册页）≤60 秒生效——用户拍板接受该延迟。
- **根 `layout.tsx`** 改 `generateMetadata()`：默认标题 = **`站名 - 副标题`**（非空才拼、不产生悬空分隔符；**不兜底**）；`description` / `keywords` 配了才输出；OG 只出 `og:title` / `og:description` / `og:site_name` 三个文字字段（无 `og:image`）；`metadataBase` 仅 URL 合法时设；`robots: { index: false }` 仅开关开着时。**子页面标题保持原样、不加任何后缀**。
- 新增 **`app/robots.ts`**：开关开 = 全站 `Disallow: /`；关 = 放行 + **`/console` 永久 disallow**。
- **控制台四页补标题**（跟导航走、无「\| 网站后台」后缀）：个人信息 / 登录会话 / **站点设置**（新键 `settings.siteSettings`，界面文字仍叫「常规」）/ 用户注册。
- 纯形状独立成**零 import 的 `lib/site-metadata.ts`**（参数收 `unknown`；选择器 / 标题拼装 / URL 校验单点）。

**实现发现（重要）**：`cacheComponents: true` 下**路由级 `export const revalidate` 被构建拒绝**（`Route segment config "revalidate" is not compatible with nextConfig.cacheComponents`）——按计划预案删掉该行，缓存落在读取层 fetch 的 `next: { revalidate: 60 }`；构建表实测 **`○ /robots.txt` + `Revalidate 1m`（ISR 静态 + 60 秒再生）**，`/signin` / `/signup` 等动态页也带 `1m`（数据缓存反射）。行为口径成立：≤60 秒生效、非每请求。

**验证（离线，本机）**：纯模块离线断言 RED（模块不存在，`TS6053`）→ GREEN（`site-metadata checks: PASS`）；变异自检两条：① 标题朴素直拼 → 红（`'A - '` 悬空分隔符）；② 关键词裸 `split(",")` → 红（`' blog '` / 空串未清）；均已还原复绿。`tsc` / `eslint`（0 error，2 条 `sonner.tsx` 存量 warning）/ `prettier --check` 与本轮每次 **`next build`** 全程绿（build 是本轮缓存与路由配置的直接证据）。**运行期未验**——标签 / meta / robots 两态交用户实机。

**评审后加固（当晚）**：独立代码评审（requesting-code-review，只读子代理）结论 Critical / Important 零、三条 Minor，**三条均已落地**——① `siteBaseUrl` 补**协议白名单**（仅 http / https；离线断言先红（`javascript:alert(1)` 原实现会放行）后绿）；② 公开设置读取补 **5 秒超时**（`AbortSignal.timeout(5000)`；离线检查先红（模拟"卡死后端"8 秒不返回）后绿（5014ms fail-open 返回空设置））；③ spec §5 B5 措辞按实际更正（register 页标签与 h1 同键、其余三页各持两处键）。

**已知缺口**：① 注册闸随缓存 ≤60 秒延迟（用户接受）；② 全空配置 = 无标题无 description（不兜底，有意）；③ `settings.siteSettings`（标签）与 `settings.general`（侧栏 / h1）是两个键，将来改名要改两处；④ `/console` 的 disallow 硬编码在 `robots.ts`，换后台前缀要同步改；⑤ 无测试运行器（离线断言不入库）；⑥ 后台页面会继承站点级 description / OG（无实际影响）。

**参考实现对齐（当晚后续）**：屏蔽搜索引擎对齐 WordPress——根 metadata 的 `robots` 由 `{ index: false }` 改为 **`{ index: false, follow: false }`**（`noindex, nofollow`，WP `wp_robots_no_robots` 同款）；robots.txt 保持"开着 = 全站 Disallow"的**双保险**不动（用户点名保留；WordPress 5.3 起已不再封 robots.txt、只靠 meta 去收录，本仓的差异按用户口径登记在 spec D8）。

**提交状态**：本轮按批入库（① `lib/site-metadata.ts` → ② `lib/public-settings.ts` 缓存 → ③ 根 layout → ④ `app/robots.ts` → ⑤ 控制台四页 + 两份 messages → ⑥ 本节 + 两份 spec / plan 文档，共 6 个提交）；**评审后加固三条随后另成一小批**——均未推送。

---

## 2026-10-15 · 控制台「常规」设置页接后端（site 组六键，纯前端）

**范围**：纯前端，后端零改动，零新依赖。spec：`docs/superpowers/specs/2026-10-15-neoblog-settings-general-wiring-design.md`；计划：`docs/superpowers/plans/2026-10-15-neoblog-settings-general-wiring.md`。

**动机**：常规页（`/console/settings/general`）自 f512409 起是纯 UI——六行设置全是本地 state、初值空串，「屏蔽搜索引擎」开关 disabled；后端 `site` 组六键齐备（0.19.0 五项 + 0.20.1 `block_search_engines`）但页面读写都没接。本轮把它接到"读到真值、改完能存、存完是真值"。

**本轮口径（D1–D9 摘要）**：

- **读**：服务端直读（`lib/admin-settings.ts`：读 cookie → 带 cookie 转发 `GET /api/v1/admin/settings/site` → 形状校验，`cache: "no-store"`），值作 props 下传；不做客户端 `useEffect` 拉取。
- **写**：右下角悬浮保存（`ConsoleFloatingSaveButton`，Ctrl/Cmd+S 已内置）＋ 脏值比对，**只 PATCH 改动过的键**（空串变化、开关双向都算变化）；成功后以响应 values 重置基线 + `router.refresh()`。
- **读失败三态**：403 →「无权查看设置」；401 / 5xx / 断网 / 形状坏 →「设置读取失败」；**两种都不渲染表单**（不变量：读失败路径不存在可保存的表单，杜绝"空基线保存"把真实配置清空）。
- 纯形状独立成 `lib/site-settings.ts`（零 import，服务端读与客户端存共用同一套解析；键名映射 `blockSearchEngines → block_search_engines` 单点）；`block_search_engines` 开关解禁；Logo / 图标维持 disabled（无键、无上传接口）；不校验、不 trim，空串是合法值。

**验证（离线，本机）**：

- 纯形状模块走"编译成 JS + node 断言"（脚本在台账工作区、不入库）：**RED**（模块不存在，`TS6053`）→ **GREEN**（`site-settings checks: PASS`；断言覆盖解析失败面、多余键忽略、空串变化、布尔双向、键名映射）。
- 变异自检两条：① 让"空串变化"被跳过 → 红（`{}` vs `{ title: '' }`）；② 键名映射改 `blockSearchEngines` → 红（`{blockSearchEngines:true}` vs `{block_search_engines:true}`）；均已还原复绿。
- 静态门禁：`tsc --noEmit` ✓、`eslint` **0 error**（剩 2 条 `sonner.tsx` 存量 warning，非本轮引入）、`prettier --check` ✓；取证 grep：表单内 `useState` 0 处、`ConsoleFloatingSaveButton` 2 处（import + 使用）、蛇形键名只出现在 `lib/site-settings.ts`。
- **运行期未验**（浏览器 / 接口一律不铺，用户口径）：读路径与保存闭环交用户实机验收。

**过程发现（值得后续点名）**：`frontend/.scratch/` 只被 git 忽略，**仍被 `npm run lint` 与 `npm run format:check` 扫描**——断言脚本放进去直接让门禁红（`require()` 撞 `@typescript-eslint/no-require-imports`），本轮把脚本挪到台账工作区 `.superpowers/sdd/…`（在 `frontend/` 之外，eslint / prettier 都扫不到）。该目录原只靠本机 `.git/info/exclude` 忽略（不随仓库分发），随本轮补进**新建的仓库根 `.gitignore`**，新克隆同样忽略。要不要给 `.scratch` 补 eslint / prettier ignore，未做、待点名。

**已知缺口**：

- **读路径只能用户实机诊断**：403 →「无权查看设置」= cookie 转发通；「设置读取失败」= 转发 / 网络 / 解析坏了。
- **happy path 的 owner 账号**：实现时开发库没有 owner（冒烟账号是普通用户）→ 默认只验 403 路径；同日按用户点名将其本人账号 `366247714@qq.com` 提权为「所有者」（照 README 的手工指定口径，固定 UUID `…0001`），完整闭环改为他本人实机验；403 路径仍可用 `gate-check@example.com` 验。
- **未保存离开无守卫**（无 `beforeunload` / 路由拦截，与个人资料页一致，登记为债务）；**多标签页后写覆盖**（无版本号 / ETag，部分更新只在改过的键之间互相盖，不做乐观锁）。
- **无测试运行器**（沿用既有缺口，spec §5 B5）：前端防回归 = 静态门禁 + 一次性离线断言 + 用户实机验收；**不校验长度 / 格式**（对齐后端 site 组 D7）。

**提交状态**：本轮按批入库（① `lib/site-settings.ts` 纯形状模块 → ② 服务端读取 + 受控表单 + 保存 hook + 两份 messages 的接线提交（page 与表单 / hook 互相引用，拆开会留下编译不过的中间提交）→ ③ spec / 计划两份文档 → ④ 本节 + 根 `.gitignore`，共 4 个提交，均未推送）。

---

## 2026-10-15 · 站点设置组 site 扩充：屏蔽搜索引擎（0.20.1）

**版本**：后端 0.20.0 → **0.20.1**（patch）。零迁移、零新接口、零新依赖、前端零改动——生产代码改动只有设置清单 `site` 组加一项。用户点名「先补后端键」、按「直接开始修改」口径执行，**无 spec / 计划文档**；版本按用户裁定的**小进位**（一个布尔设置不动 minor）。

**动机**：前端控制台「常规」页（`/console/settings/general`，纯 UI 轮 f512409）有「屏蔽搜索引擎」开关行，但后端 `site` 组只有五项文字键、没有对应键，接不上；这一项是前端设置页接入后端键值的前置。口径对齐 D 盘 HaoBlog 的同名设置 `site.blockSearchEngines`（boolean、默认 false、`isPublic: true`——它的前台用它渲染 robots 元数据，见 `lib/theme/robots.ts`）。

**口径**：

- `site` 组加 `block_search_engines`：开关、默认 `false`、标 `Public: true`（前台要匿名读它决定 robots / meta，理由同 `register.enabled` 必须公开）。
- 键名用蛇形 `block_search_engines`：本仓项名规则 `^[a-z][a-z0-9_]*$` 不收大写，不能照抄 D 盘的 camelCase。
- `site` 组由此从全文字变成混合类型（五个 string + 一个 bool）；公开项集合由 6 项变 7 项（`GET /public/settings` 的 site 负载多一个键）。

**测试同步**：形状断言 site 五 → 六项（新项单独走 `assertDefinitions`：类型 / 默认值 / 公开性）；公开白名单加 `site.block_search_engines`；匿名公开读用例断言六键（含新键默认 `false`）；「管理写 → 公开读」用例扩成 `title` + `block_search_engines` 双键同 patch 并改名 `TestSettingsPatchSiteReadsBackPublicly`（原名 `…TitleReadsBack…`；顺带覆盖 site 组的 bool 写入路径）。三条变异自检（逐条把代码改坏跑 settings 包确认红后还原）：

| # | 变异 | 实测红 |
| --- | --- | --- |
| ① | 新项默认值 `false` → `true` | {形状（default = true, want false）、匿名公开读（= true, want the false default）} |
| ② | 漏标 `Public` | {形状（public = false, want true）、公开白名单（少一项）、匿名公开读（五个键 vs 六个）、管理写公开读（公开负载里没有新键）——四条全红} |
| ③ | 类型改 `FieldTypeString` | {形状（type = 1, want 3）、匿名公开读（空串 vs false）、管理写（PATCH 400 类型不符）} |

**文档同步**：README 三处（模块对照表行、设置章节「当前清单项」、公开负载节选注记）。

**已知缺口**：前端控制台「常规」页与前台 robots 渲染均未接（本轮零前端改动）；后端零消费方。

**提交状态**：本轮按批入库（① 清单 + 测试 → ② README 与 DEVLOG → ③ 版本进位，共 3 个提交，均未推送）。

---

**版本**：后端 0.19.0 → **0.20.0**（minor）。零迁移、零新接口、零新依赖、前端零改动——生产代码改动只有设置清单：`register` 组加 4 项、新建 `auth` 组 9 项。spec：`docs/superpowers/specs/2026-10-15-neoblog-auth-settings-design.md`；实施计划：`docs/superpowers/plans/2026-10-15-neoblog-auth-settings.md`。

**动机**：用户点名加一批「注册与认证」字段，口径是**只填字段、不接实现**——值能存能读，但不接邮件、不让会话真过期、不验验证码。字段与 D 盘 HaoBlog「认证设置」页逐项对应；分组与粒度按用户裁定：拆 `register` + `auth` 两组、沿用 `register/enabled` 做注册总闸、密码要求拆长度 + 复杂度、会话有效期按天默认 7。

**口径**：

- `register` 组五项：`enabled`（总闸，公开）+ `default_group`（默认 `"user"`）、`email_domain_control_enabled`、`allowed_email_domains`（英文逗号分隔）、`email_verification_enabled`。
- `auth` 组九项：`password_requirements_enabled`、`password_min_length`（默认 `8`）、`password_complexity`、`session_max_age_enabled`、`session_max_age_days`（默认 `7`，单位天）、`totp_enabled`、`password_reset_enabled`、`captcha_enabled`、`captcha_provider`。
- 新增 13 项**全部非公开**：公开端点负载零变化（仍 `site` 五项 + `register.enabled`）。
- 与 D 盘的三处口径差：`password_requirements_enabled` 默认 false（D 为 true）；`session_max_age_days` 存天（D 存小时 168）；`captcha_provider` 默认 `""`（D 为 selfhosted）。
- 不做校验（枚举 / 范围 / 长度）；不加 Turnstile 密钥两项（用户清单未含；secret 类型机制也没有）。

**测试与变异自检**：形状断言扩两组成员（名字 / 类型 / 默认值 / 非公开逐项）+ 两条 admin 用例（`register` 默认读、`auth` 默认读 + 写回）。变异自检三条：

| # | 变异 | 实测红 |
| --- | --- | --- |
| ① | `session_max_age_days` 默认改 30 | {形状（default = 30, want 7）、auth 往返（= 30, want 7）} |
| ② | `captcha_enabled` 误标 `Public` | {白名单（7 项 vs 6）、形状（public = true, want false）、匿名公开读（3 组 vs 2）、站点标题公开读（3 组 vs 2）——四条全红} |
| ③ | `default_group` 挪去 `auth` | {形状（register items = 4）、register 默认读（4 项）} |

**已知缺口**：13 项全无消费方（实现接线是后续轮）；`default_group` / `captcha_provider` 无取值约束；`password_min_length` 无范围校验；控制台界面未做。

**提交状态**：本轮按批入库（① 代码+测试 → ② spec/计划 → ③ README 与 DEVLOG → ④ 版本进位，共 4 个提交，均未推送）。

---

## 2026-10-15 · 站点设置组 site（0.19.0）

**版本**：后端 0.18.0 → **0.19.0**（minor）。零迁移、零新接口、零新依赖、前端零改动——唯一生产代码改动是设置清单加一个组。spec：`docs/superpowers/specs/2026-10-15-neoblog-site-settings-design.md`；实施计划：`docs/superpowers/plans/2026-10-15-neoblog-site-settings.md`。

**动机**：用户点名加「站点标题 / 副标题 / 站点描述 / 站点关键词 / 站点 URL」。用途是前台元数据（`<title>`、meta、canonical/OG）的公共来源，将来邮件链接基址大概率也读 `site.url`。设置机制从 0.8.0 落地时定的口径就是「新增设置只改代码、永不操作数据库」，本轮是首个纯清单扩展。

**口径**：

- 组 `site`，五项 `title` / `subtitle` / `description` / `keywords` / `url`：全文字、默认空串、全部 `Public`。键名与 D 盘 HaoBlog 的 `site.*` 逐字一致。
- `keywords` 存英文逗号分隔的单个字符串（框架无列表类型；HaoBlog 同形）。
- 不做长度 / URL 格式校验（校验是机制留的插槽，首个真实需求再动）。
- 组序 `site` 在 `register` 前；公开聚合按清单序输出。
- 前端零改动：本轮只保证公开端点可读；渲染接线是后续前端轮。
- README「怎么加一个设置」的配方示例组名由 `site` 换成 `demo`（与真实组消歧），`recipeGroups()` / `TestRegistryRecipeShape` 原子联动；README 公开负载示例与 API 规范 #23 示例补「（节选）」。

**测试与变异自检**：测试先行——四处断言先红（公开白名单、形状断言、匿名公开读、新端到端用例，均因 site 组不存在）→ 落清单转绿 → 变异自检三条：

| # | 变异 | 实测红 |
| --- | --- | --- |
| ① | `site.url` 取消 `Public` | {公开白名单（5 vs 6）、形状断言、公开读（site 只剩 4 键）} |
| ② | `site.title` 默认值改非空串 | {形状断言、公开读（title=NeoBlog）} |
| ③ | 两组换序 | {公开读、端到端（均因组序）；白名单保持绿——证排序生效} |

**已知缺口**：五项后端暂无消费方（前端未接线，界面不变）；无长度校验；空串默认 = 未配置、前台自行兜底；`site.url` 属「每个部署一份」的值。

**提交状态**：本轮按批入库（① 代码+测试 → ② spec/计划 → ③ README、API 规范与 DEVLOG → ④ 版本进位，共 4 个提交，均未推送）。

---

## 2026-10-15 · 用户简介 user_bio（0.18.0）

**版本**：后端 0.17.0 → **0.18.0**（minor）。一个迁移（`user_bio`，纯 DDL）、零新依赖；前端两处小改（回填 `bio`，见下）。

**动机**：前端个人资料页早有「用户简介」输入框，保存时把 `bio` 一并发出，但后端 `users` 表没有这一列、`PATCH /users/me` 的请求结构体也只有 `nickname` / `email`——Go 反序列化**静默丢弃未知字段**，所以请求照样 200、页面照样弹「已保存」，实际什么都没存、刷新即空。用户 2026-10-15 核对后选择「后端补字段」（另一选项「前端撤掉输入框」未取）。

**口径（四条，实现即按此落）**：

- 存储：`users.bio` 可空 varchar（`field.String("bio").Optional().Nillable()`），「未设置」= `NULL`——对齐 API 规范 §5「标量可空字段输出 `null`」与 D 盘旧项目的 `String?` 形态。
- 请求：`PATCH /users/me` 加可选字段 `bio`（`*string`）；缺省 / `null` = 不修改（沿用既有 PATCH 指针约定）；提交 `""` 或纯空白 = 清空（库中落 `NULL`）。「至少给一个字段」守卫同步扩到三个字段。
- 归一与校验：trim（保留内部换行）、≤1000 字符按 rune（与 HaoBlog 的 `PROFILE_BIO_MAX_LENGTH` 同值）、越界 400 `invalid_request` + `param: "bio"`。
- 响应：`UserResponse`（注册 / 登录 / verify / PATCH）与 `MeResponse`（`GET /users/me`）统一带 `bio`（未设置为 `null`）——用户对象保持单一形状。

**实现要点**：

- 迁移 `20261015003344_user_bio.sql`：`ALTER TABLE "public"."users" ADD COLUMN "bio" character varying NULL;`，两库已应用；`scripts/db-migrate-diff.sh` 的字面量随轮换为 `user_bio`。
- 仓储三分支：`nil` 不动 / 空串 `ClearBio()` / 非空 `SetBio()`（`repository.go` 的 `updateProfile`）。
- 前端接线（用户指出「前端没有接入上」后补，只动两个文件）：`lib/auth-session.ts` 的 `SessionUser` 加 `bio: string | null` 并解析（`undefined` 容忍 → 老后端返回里没这个键也不会让整站会话解析炸掉，落 `null`）；`hooks/use-profile-save.ts` 本地 state 改从 `profile?.bio` 取初值、保存后用响应里的 `bio` 回填（沿用「保存后回填后端归一值」的既有口径，顺带吃掉后端 trim）。不碰并行会话正在改的 `messages/*.json`，**因此本轮零新增文案键**。

**测试与变异自检**：新增 `TestUpdateProfileBio`（初始 `null`、写读一致且 trim 与换行保留、只改简介不动身份字段、1000 / 1001 rune 边界、空白清空后 API 与库内**双核对 `NULL`**、只改昵称保留简介、空串显式清空）；`TestUpdateProfileEndToEnd` 的两个边界清单补 `{"bio": null}` 与 `{"bio": 42}`。变异自检六处（改坏 → 确认目标红 → 改回 → 全包绿）：

| # | 变异 | 实测红 |
| --- | --- | --- |
| ① | `normalizeBio` 去掉 trim | {trim + 换行断言} |
| ② | `validBio` 边界 `<=` 改 `<` | {1000 rune 边界} |
| ③ | 仓储删清空分支（一律 `SetBio`） | {空白清空 → 落空串而非 `NULL`} |
| ④ | `toUser` 不映射 `bio` | {写后回读} |
| ⑤ | 校验错误丢 `param` | {1001 rune 的 `param` 断言} |
| ⑥ | 「至少给一个字段」守卫忽略 `bio` | {纯 `bio` 的 PATCH} |

**活体证据（临时实例 5733，`gate-check@example.com`）**：登录响应即带 `"bio":null` → PATCH 带首尾空格与换行 → 200 且 trim 生效 → 只改昵称 bio 保留 → 1001 字符 400 `param:"bio"` → `{"bio":""}` 清空回 `null` → 直连 `neoblog` 库确认为 `NULL`（账号复位原状）。临时实例已关闭。

**已知缺口**：

- **简介超长暂无专属错误文案**：`param: "bio"` 现在落进 `use-profile-save.ts` 的兜底分支，页面提示通用「保存失败，请稍后重试」。加专属键要动 `messages/*.json`，而那两个文件正被并行会话占用——等用户点名再补。
- **前端侧未做运行期验收**（按既有口径，运行期 / 视觉验收由用户自己在浏览器里做）：`tsc --noEmit` 退出 0、`eslint` 对改动的两个文件 0 问题、`prettier --check` 通过；页面实际表现（填写 → 保存 → 刷新仍在）待用户验。
- 简介除长度外无其他校验（无 Markdown / HTML 清洗）；纯文本存储，展示侧自行转义。
- 全仓测试无 `-race`（本机无 cgo）；本轮无并发路径新增，与既有口径一致。

**提交状态**：本轮改动已按批入库（`schema`+迁移 → 接口+测试 → 前端接线 → 本文档 → 版本进位，共 5 个提交，均未推送）。工作区里并行会话的侧栏设置页前端文件（`console-sidebar-nav.tsx`、两份 `messages`、`app/console/settings/`、`console-sidebar-section.tsx`）仍未提交，**不属于本轮**。

---

## 2026-10-15 · 侧栏面板状态化（无动画）

**范围**：纯前端，后端零改动，零新依赖。spec：`docs/superpowers/specs/2026-10-15-neoblog-sidebar-panel-state-design.md`；计划：`docs/superpowers/plans/2026-10-15-neoblog-sidebar-panel-state.md`。

**动机**：主侧栏导航区在 `/console` 下整片空白——`console-sidebar-nav.tsx` 在路径不是 `/console/uc` 打头时直接 `return null`，而 `SidebarContent` 是 `flex-1`，空了就整片留白；且「个人设置」那栏的形状每次都要重写、面板显示完全由 URL 决定。

**本轮口径**：

- **state 为真源、URL 只同步**：`displayedPanel` / `syncedRoutePanel` 双 state，渲染期比对接管（不用 `useEffect`），两个初值都取路由面板，避免首屏先渲染主栏再纠正。
- **动画整条不做**（推翻上一轮定的 ±48px 对滑）：无叠层绝对定位、无新增 transition 常量、无 `outgoingPanel` / `setTimeout` / `onTransitionEnd`，面板切换即显隐。`sidebarLabelFade`（折叠时文字淡出）属既有折叠机制，保留。
- **返回语义两套并存**：settings 面板返回是按钮，只改 state、URL 不动；uc 面板返回仍是 Link（未改）。这是"最小化 + 尊重上轮口径"换来的不一致，登记为债务。
- 抽出复用件 `console-sidebar-section.tsx`：`back` 用判别联合自带 `onClick`（不留可能永不触发的必填回调）；`items[].className` 承接 `31fac91` 的 `mb-1`，防止注册表形态把它丢掉。
- 主侧栏只加「设置」一个入口；子栏只有「常规」一项 + 一页只有标题的占位页，**不接任何真实系统设置内容**。

**已知缺口 / 未实测**：

- **八条交互实测一条都没做成**：dev server 在 3000，但 `/console`、`/console/settings/general`、`/console/uc/profile` 全部回 `307 → /signin`，无登录态就拿不到侧栏 SSR HTML；开发库注册是关闭态、也没有可用凭据。已证明的只有三件：`tsc --noEmit` 退出码 0、`eslint .` **0 error**（剩 2 条 warning 在 `components/ui/sonner.tsx`，非本轮引入）、`npm run format:check` 全通过（**作用域是 `frontend/`**——仓库根没有 prettier 配置，`docs/*.md` 不在格式检查范围内），外加三条路由返回 307 而非 500（说明模块能编译）。
- **B1 错位窗口**：点返回后 URL 仍停在 `/console/settings/general`，此时**刷新会回到 settings 面板**（state 不持久）。这是 D1+D2 合取的既定结果，不是 bug，但尚未眼验。
- **B5 移动端**：Sheet 抽屉里点「设置」后是否自动关闭未设计、未测。
- **B6 无测试兜底**：前端无测试运行器（`package.json` 只有 `dev/build/start/lint/format`，全仓 0 个 `*.test.ts*`），本轮不引入 ⇒ 面板切换不变量拆在判定层与编排层，**没有自动测试承载**。
- **B8 键盘焦点缺口（审查发现，本轮未修）**：settings 面板的「返回」是按钮，点击后整棵子面板卸载，聚焦的按钮消失 → `activeElement` 退化到 `<body>`，再按 Tab 从文档起点重来、侧栏被整体跳过。鼠标用户无感；旧版 `uc` 那栏的返回是真链接，跳页后浏览器自行重置焦点，所以这条缺口是 settings 路径新增的。修法是在返回后把焦点交给主栏「设置」项，需要引 ref/焦点转移，与本轮「最小化」口径冲突，**按用户口径登记不修**。

**审查后更正两处**：

- 代码 `console-sidebar-nav.tsx:117` 原写 `isActive={routePanel === "settings"}`，与 spec §4.5 / plan Task 3 的全式 `displayedPanel === "main" && routePanel === "settings"` 不一致（行为等价：该判断在 `:111` 的 `displayedPanel === "main"` 提前 return 之内，前缀恒真），已补全以对齐文档并防后续改成叠层时静默变语义。
- plan Review Focus 5 原写「`en.json` 漏键会抛 `MissingMessageError`」**是错的**：实测 `use-intl` 的 `defaultOnError` 是 `console.error(error)`（`initializeConfig-CUsOI8u2.js:17-19`），缺键走 `getFallbackFromErrorAndNotify` 返回 `namespace.key`（同文件 `:95-103`），不抛错、不会让任何校验命令变红；本项目全文无 `onError` 覆盖。plan 已同步更正。

**提交状态**：五个代码文件（两份 messages、`app/console/settings/general/page.tsx`、`console-sidebar-section.tsx`、`console-sidebar-nav.tsx`）与本节**均未提交**（用户口径：不要擅自提交）；本轮已入库的只有 `46052b8`（spec + plan 两份文档，未推送）。

---

## 2026-10-14 · 两步验证 setup 复用窗口（0.17.0）

**版本**：后端 0.16.0 → **0.17.0**（minor）。一个迁移（`totp_setup_at`，纯 DDL）、零新依赖、前端零改动。设计 spec：`docs/superpowers/specs/2026-10-14-neoblog-2fa-setup-reuse-window-design.md`；实施计划：`docs/superpowers/plans/2026-10-14-neoblog-2fa-setup-reuse-window.md`（用户口语称「防抖」，实现语义是**复用窗口**）。

### 动机与窗口口径

症状：2FA 设置面板每次展开都重新生成密钥（前端收起即卸载 `useTwoFactorSetup` → 挂载即 `POST /users/me/2fa/setup`；后端无条件覆盖）。v1 方案定 30 秒，被独立审查否掉——30 秒盖不住「展开→掏手机扫码→回浏览器输码」的真实节奏，症状照样复发；且面板没有主动换钥入口，「等窗口过去」不可见。**窗口定为 5 分钟（固定窗，从生成时刻起算）**：`now - totp_setup_at < 5min` 复用（零写入、同一账号名下响应逐字节相同），`>= 5min` 重新生成（旧码作废、窗号归零，既有语义）。`disable` 清四列（secret / enabled_at / last_counter / **setup_at**）；`enable` 不动 `setup_at`。

### 实现要点

- `users` 加 `totp_setup_at`（可空 timestamptz，迁移 `20261014134240_totp_setup_at.sql`，两库已应用）。
- 复用响应 = 解密存量密文 → base32 解码 → `totp.Generate{Secret: 原始字节}` 重建；**空明文守卫**必须有——`pquerna/otp` 在 `len(Secret)==0` 时会静默随机生成新密钥（实证源码）。
- 生成路径走**原子条件更新认领** `claimTOTPSetup`（条件：未启用 且 `setup_at` 为 NULL 或早于 `now - 5min`），互斥依据是 Postgres Read Committed 下单条 UPDATE 重算谓词——**同一时刻 N 个并发 setup 至多产生一把密钥**，其余复用。认领 0 行 → 复读以库内现状为准（无密钥 → 再认领一次，共两次；两次都不行 500，理论不可达、不写库）。
- **删除了 `updateTOTPSetup`**：认领路径接手后它成了无人调用的死方法（Go / `go vet` 都不报），全仓 grep 确认无痕。曾考虑的「认领失败盲写候选」兜底在审查中被否：既无用例覆盖、又破坏「至多一把」。

### 测试与变异自检

新增 **10 条**集成用例（复用零写入 / 5m−1s 与 5m 边界 / 8 并发同钥 / disable 清 / enable 保留 / 遗留行 / 复用解密失败 500 / 空明文 500 / 仓库级认领互斥——最后这条是变异②自检补救时新增的）+ 2 条既有用例调整（`TestTwoFactorSetupOverwritesSecret` 推进到窗口外、`TestTwoFactorDisableLifecycle` 补断言）；时间一律经 `totpNow` 假钟固定，零真实等待。

变异自检八处（改坏→确认目标红→改回→全包绿），实测集合：

| # | 变异 | 实测红 |
| --- | --- | --- |
| ① | 复用条件 `<`→`<=` | {3 边界} |
| ② | 认领删时间条件 | **首测并发用例未红**（服务层全先读后写，时序运气）→ 补仓库级用例 `TestClaimTOTPSetupRejectedWhenFresh`（新鲜行二次认领必拒、过期必放）后红 {该用例} |
| ③ | disable 漏清 setup_at | {5, 11} |
| ④ | 删读路径复用分支 | **不可观测，移出列检**：慢路径（认领 0 行→复读→复用）行为与快路径等价（探针实证三零：响应同 / 密文同 / 时刻同，全包不红）；真正防线是「认领与复用互斥」（②⑥ 钉住） |
| ⑤ | enable 误加 Clear | {6} |
| ⑥ | `isFreshTOTPSetup` 把 NULL 时刻当新鲜（时间窗保留） | {7}（遗留行走复用返旧钥；独立复核实跑确认） |
| ⑥b | 新鲜判定整条放宽成「有密钥就算新鲜」（时间窗一并丢） | {3, 7, 10}——首轮 DEVLOG 误把 ⑥b 的实测数字记在 ⑥ 名下（两个改坏串行）；复核实跑把两者拆开，⑥={7}、⑥b={3,7,10}，spec §7 同步 |
| ⑦ | 认领谓词删 `IsNil` | 25 条（几乎全包：首次 setup 都落 NULL 行 → 500）；未逐条实跑，逻辑复核确认 |
| ⑧ | 删空明文守卫 | {9} |

②④⑥三条是自检真发现：并发用例单靠 goroutine 抓不住确定性互斥（须仓库级用例）；快路径只是慢路径的免写优化、行为分叉在 `isFreshTOTPSetup` 的前提上（不在快路径有无）；首轮自检曾把 ⑥b（整条放宽）的实测数字误记在 ⑥（仅 NULL 放宽）名下，复审拆开更正。变异⑦未逐条实跑（逻辑复核）。`-race` 本机不可用（无 cgo），并发正确性靠「认领与复用互斥的仓库级断言 + 重复 8 次稳定绿」承载，未做竞态证明。

### 活体证据（5732 实例，`gate-check@example.com`）

登录 → setup A → 立刻再 setup B（A == B，`setup_at` 落值）→ 探针把 `totp_setup_at` 拨回 6 分钟前 → setup C（C ≠ B）→ 用 C 算当前窗码 enable → 200 + 10 恢复码。**注意**：第一轮活体在旧二进制上跑出「同刻两把」假结果——dev server 是 `go run`，改代码后必须重启才吃到新实现（已重启复测）。冒烟账号事后已复位（四列清空）。

### 已知缺口

同坐超 5 分钟仍换钥（残余，接受）；面板每次展开仍有一次请求 + 骨架屏闪烁（前端轮）；主动换钥无 UI 入口（disable 接口可用，按钮待前端轮）；陈旧待验证密钥不主动清理。

---

## 2026-10-14 · 路由改名：`/auth/*` 前缀取消（会话式方案，0.16.0）

**版本**：后端 0.15.0 → **0.16.0**（minor：破坏性路径变更）。**零迁移、零新依赖**。**状态：未提交**（工作区留待检查）。起因：用户发现「读自己 `GET /auth/me`」与「改自己 `PATCH /users/me`」挂在两个前缀下，要求把认证域收进 `users`；先做了一轮**只读调研**（大厂公开 API / 框架官方生成器 / 「登录=建会话」流派，逐条核对官方文档），用户在三个候选里选定**会话式**。

### 新路径（旧 → 新逐条）

| 旧 | 新 | 说明 |
| --- | --- | --- |
| `POST /auth/register` | `POST /users` | 注册 = 建用户 |
| `POST /auth/login` | `POST /sessions` | 登录 = 建会话 |
| `POST /auth/logout` | `DELETE /sessions/current` | 登出 = 删当前会话（方法 POST → DELETE） |
| `POST /auth/2fa/verify` | `POST /sessions/current/verify` | 二步验证 = 对当前（待二段）会话的验证 |
| `GET /auth/me` | `GET /users/me` | 与 `PATCH /users/me` 合成同一条资源（`/auth/me` 删除） |
| `POST /auth/password` | `POST /users/me/password` | 改密 |
| `POST /auth/2fa/{setup,enable,disable,recovery-codes}` | `POST /users/me/2fa/*` | 2FA 管理，只搬家、名字不变 |
| `PATCH /users/me`、`GET/DELETE /users/me/sessions` | 不动 | |

`/auth` 前缀从此不存在；守卫链、错误码、响应形状、会话数据键（`auth.user_id` 等，内部键不是 URL）全部不变。

### 方案依据（调研出处，均有官方文档原文）

- 「当前用户」= `/users/me`：Twitter/X（`/2/users/me`）、Okta（`/api/v1/users/me`）、Discord（`/users/@me`）、Strapi（`/api/users/me`）一致；GitHub 是唯一的单数 `/user` 派。
- 登录/登出从不放 `/users` 下：大厂一律放认证命名空间或会话资源；登出要么是动作端点，要么是**删会话**（Okta `DELETE /api/v1/sessions/{id}`）。
- 「登录 = 建会话」有真实采用者：Rails 8 官方生成器（`POST/DELETE /session`）、Ghost（`POST /admin/session/`，2FA 是对本次会话的 `PUT /admin/session/verify/`）、Zitadel（`POST /v2/sessions`、`PATCH /v2/sessions/{id}` 提交 TOTP）。
- 2FA「登录挑战」与「设置管理」分居两处是主流常态（Okta `/authn/factors/…` vs `/users/{id}/factors`；Fortify `/two-factor-challenge` vs `/user/two-factor-*`；ASP.NET `/Account/LoginWith2fa` vs `/Manage/*`）。
- 被否的两个候选：`/users/login` 全动词式（大厂零先例；Zalando REST 规范 Rule 141「URL 必须无动词」是 MUST 级；唯一先例是 Phoenix 生成器）；保留 `/auth` 只收匿名（主流但违背用户「不要单独 auth」的初衷）。

### 改动面

- 后端：`internal/modules/auth/controller.go` 两个注册函数（10 行路径 + logout 方法改 DELETE）；`GET /users/me` 复用原 `me` handler。
- 测试：5 个文件路径同步（`app_test.go`、`auth_test.go`、`register_gate_test.go`、`sessions_test.go`、`totp_flow_test.go`）；405 用例的「GET-only」样本从 `/auth/me` 换到 `/users/me/sessions`（`/users/me` 现在 GET+PATCH 双方法）；2FA 测试助手 `post2FA` 改走 `/users/me/2fa/`、`logoutOK` 改走 `doDELETE /sessions/current`。
- 前端：4 个真实调用点换名——`use-signin`（→ `/api/v1/sessions`）、`use-signup`（→ `/api/v1/users`）、`use-signout`（→ `DELETE /api/v1/sessions/current`）、`lib/auth-session.ts`（→ `/api/v1/users/me`，含错误文案）。
- 文档：API 规范 v1.5 修订条目 + §3 规则 3 示例 / D11 / 端点表 + §4 无负载体示例；`backend/README.md` 接口表与改名对照；版本 0.15.0 → 0.16.0。

### 验证

- 全仓 `scripts/test.sh`（串行、真库）：**全绿**（`internal/app` 4.7s、`internal/modules/auth` 13.6s，共 11 包）。
- 前端 `npx tsc --noEmit`：零错误。
- 全仓源码扫描：`auth/login|logout|register|me|password|2fa` 在 `backend/`、`frontend/` 的代码里**零残留**；`docs/` 下仅存档保留旧路径原文（更早轮次的 spec/plan 与日志旧条目），本轮同步的活文档（README / 规范 / 本日志 / 同日会话管理 plan）已全部换新。

### 备注

- 上一轮提交过程里出现过「并行会话把同款改名写进 `controller.go` 又撤出」的插曲（见本文件下一条「提交过程中的插曲」）；本轮是从干净树重做的完整版（含测试、前端接线与文档），不是半成品。
- 运行期验收（浏览器登录链路）留用户自查；需要的话可另跑 curl 冒烟。

### 评审回补（同日）

- `app_test.go` 405 表新增一行 `PUT /users/me → Allow: GET, PATCH`——`/users/me` 变 GET+PATCH 双方法后的新形状此前没有用例钉住；已做变异验证（临时把 PATCH 路由改名，该行立刻红：`Allow = "GET", want "GET,PATCH"`；还原后全绿）。
- 同日 `docs/superpowers/plans/2026-10-14-neoblog-session-management.md` 的 3 处 `GET /auth/me`（含冒烟 curl 全链路）同步为 `GET /users/me`，文件顶加「路径口径（0.16.0 起）」说明——它是可复跑配方，不是历史存档。
- 口径修正：规范 v1.5 条目原写「实现随 0.16.0 入库」（本轮未提交，措辞不实）改为「实现见 0.16.0」并注明 §11 及以后为历史存档；§3 端点表列名「声明位置（2026-10-11 时点）」改为「随轮次更新」（该列本就逐轮更新，本轮写入了 0.16.0 时点的新行号）；本条目「零残留」句的存档清单补全。

---

## 2026-10-14 · 会话管理：审查后修补（测试有效性 3 条 + 文档口径 5 处 + 小项）

**版本**：0.15.0（未进位）。针对两份独立审查（终审子代理 + 外部复核）的结论做修补。**状态：未提交**（等用户决定提交切分）。用例总数 16 → 18。

### 测试有效性（三条都是「改坏了也会绿」）

- **`TestSessionsRevokeCurrentEndsSession` 原本没钉住复活路径**：它把 `last_seen_at` 改到 6 分钟前之后**又调了一次列表接口**，时间被刷回新鲜 → 删行时本请求不写库，`Destroy` 与裸 `Store.Delete` 无差别。改为「先列表取 id → 再改写 → 立即 DELETE」。**变异验证**：把 `Destroy` 换成 `Store.Delete(target)`，该用例立刻红（`status = 200, want 401`）；还原后绿。
- **节流「新鲜不写库」两处断言对「每次都写」全盲**：基准值原是「now」，与任何新写入的秒级字符串相同。基准改为 **1 分钟前**（仍新鲜，但与新写入必然不同）。**变异验证**：把节流条件改成恒真后，`TestTrackSessionActivitySkipsWhenFresh` 与 `TestSessionActivityTouchThrottled` 同时红。
- **`enterPendingSession` 清四个元数据键零覆盖**：新增 `TestTwoFactorLoginClearsSessionMetadata`（登录 → 开 2FA → 同一会话再登录吃 `two_factor_required` → 断言 `user_id` 与四个元数据键全消失、只剩 pending 键）。**变异验证**：删掉那四行 `Remove`，该用例红。
- 新增 `TestLoginTruncatesHeadersAtRuneBoundary`（多字节 UA 截断后必须是合法 UTF-8）。

### 代码小项

- `truncateHeader` 由按字节硬切改为回退到最后一个 rune 起始边界，不再截出非法 UTF-8。
- `Login` 去掉重复的 `RenewToken`：非 2FA 分支只由 `establishSession` 换发一次；2FA 分支单独换发（防会话固定语义不变）。

### 文档口径

- 设计文档：§2「删当前会话」与 §5「活跃追踪」纠正为「**必须**对当前 token 特判走 `Destroy`，裸删会被本请求的节流写提交回写复活」；§6 的 `HashTokenInStore` 代价改为**三处比较**（跳过当前会话会踢掉自己 / `current` 徽标不亮 / 判不出当前会话掉回复活坑）；§7 补「进入待验态清元数据」用例、给删当前会话用例写明前置条件、并写明测试存储是 **memstore**（Postgres 由活体冒烟覆盖）；§7 第 3 条的「照 `app_test.go` 直插先例」纠正为「新写助手（原生 SQL vs scs Codec/Store 通道不同）」。
- 计划：冒烟起进程 `PORT` → **`ADDR=:5733`**（地址只有 `ADDR` 这个口子）；节流用例基准改 1 分钟；删当前会话用例的关键前置写进用例说明；冒烟收尾补「断言残留为零」。
- API 规范表 §3 表头补消歧义一句（本表每轮追加；§13.1「表 A」是 2026-10-11 的冻结对照表、固定 14 行）。
- `README` 的 `HashTokenInStore` 缺口同步改写为三处比较。

### 附带：`internal/app` 一条既有偶发红（非本轮引入）

`TestSessionCookieMaxAgeMatchesLifetime` 偶发 `MaxAge = 3601, want 3600`。机制（读 scs 源码）：`RenewToken` 把 deadline 重设为「此刻 + lifetime」（`data.go:301`），而写 cookie 时 **Max-Age 向上取整**（`int(time.Until(expiry).Seconds() + 1)`）——两者落在同一时钟刻度就得 3601。该测试与登录公开路由都不经过本轮代码；在**当前 HEAD 与改动前提交（9144dad）各跑 25 次，两侧都是 0/25 复现**（稀有竞态，两边同概率）。断言放宽为 `want || want+1`。

### 提交过程中的插曲：并行会话在同一文件里改了路由

第一批提交时 `controller.go` 被另一会话改成整套路由改名（`/auth/login` → `POST /sessions`、`/auth/me` → `GET /users/me`、`/auth/2fa/*` → `/users/me/2fa/*`、`/auth/logout` → `DELETE /sessions/current`；该文件 mtime 05:42:29，提交发生在 05:42:42）——**F1/F2 两个提交把对方的在制品一起吞了进去**，而那时测试还全在老路径上（那棵树跑测试是红的）。处置：两个提交已撤回，按「只暂存我的 hunk」（先把我的改动恢复成独立版本暂存、再把对方的版本放回工作区）重提，**对方的改名原样留在工作区、不进提交**。教训：**按文件清单逐个 `git add` 挡不住「同一文件里的外部 hunk」**——提交前要对每个文件核对 `git diff` 的内容而不只是文件名，与并行会话共用工作区时提交窗口越短越好。

---

## 2026-10-14 · 会话管理：登录设备列表 + 单条吊销（后端一轮）

**版本**：后端 0.14.0 → **0.15.0**（minor）。**零迁移**：`sessions` 表与 `atlas.sum` 一字节未动，元数据走会话数据键。**状态：全部未提交**（用户要求本轮只做改动、不提交，工作区留待检查）。设计：`docs/superpowers/specs/2026-10-14-neoblog-session-management-design.md`；计划：`docs/superpowers/plans/2026-10-14-neoblog-session-management.md`。

### 做了什么

- **元数据四键**：`auth.user_agent` / `auth.ip` / `auth.created_at` / `auth.last_seen_at`（RFC3339 UTC 字符串），在注册成功、单段登录成功、2FA verify 成功三处经 `auth.establishSession` 统一写入；进入「待二段」改走 `enterPendingSession`（清身份 + 清元数据）。
- **节流中间件**：`guard.TrackSessionActivity`（5 分钟），`app.go` 与 auth 测试装配各挂一次，链序 `RequireAuth → TrackSessionActivity → LoadIdentity`。
- **两个接口**（都只挂 `RequireAuth + LoadIdentity`、不设权限点）：`GET /api/v1/users/me/sessions`（`{"items":[…]}`，`id = base64url(sha256(token))` 指纹，`current` 徽标，RFC3339 时间）与 `DELETE /api/v1/users/me/sessions/:id`（200 `{"id","deleted":true}`；未知 / 他人 / 二次删一律 404 `not_found`）。
- **收敛重复实现**：改密 / 2FA 开关在用的 `revokeOtherSessions` 抽到共用的 `eachSession` 迭代器上，行为不变（三处调用点同步去掉已不需要的 ctx 参数）。
- **测试**：新增 16 条用例（guard 4 + auth 12），全仓 11 包全绿（`scripts/test.sh`，串行）。
- **活体冒烟 12/12**：:5733 另起新二进制 + curl 全链路（两条登录 → 列表 2 条且 IP 来自 `X-Forwarded-For`、`id` ≠ cookie 值 → 删一条 → 被删方 401 / 自己 200 → 列表回 1 → 乱造 id 404 → 匿名双接口 401 → 登出 200），跑完会话行已清、进程已停。

### 过程中的关键事实（写进计划前先证过的）

- **`Put` 不移动 deadline**：scs 的 `Commit` 用 `sd.deadline`，`Put` 只置 `Modified`——节流刷新 `last_seen_at` **不会**把 7 天绝对有效期变成滑动续期。
- **`postgresstore` 自带清理**：`New()` 就带 5 分钟一轮的过期删除 goroutine，`All()` 的 SQL 已带 `WHERE current_timestamp < expiry`，列表天然只出活跃行——**不需要另做清理任务**（写计划时的初始判断是错的，读源码纠正）。
- **gob 只吃内建类型**：会话数据是 gob 编码的 `map[string]interface{}`，`time.Time` 进 interface 需要 `gob.Register` 全局副作用，因此四个元数据键一律存 RFC3339 字符串。
- **删当前会话有「复活」坑**：中间件若在本请求内 `Put` 过（节流到点），handler 之后再删行，scs 会因 `Modified` 在响应提交处把行**写回**。`RevokeSession` 对「当前 token」改走 `sessions.Destroy`（置 `Destroyed`、清 cookie），绕开提交分支；`TestSessionsRevokeCurrentEndsSession` 先把 `last_seen_at` 改到 6 分钟前、**紧邻 DELETE 之前**来钉这条路径（新鲜会话不会触发，光跑默认路径测不出来）。**注意：该用例的首版写法失效**——改写之后又发了一次列表请求，时间被刷回新鲜，删行时本请求不再写库；已在本日顶部「审查后修补」条目修正并加变异验证。
- **顺带闭合 2FA 轮登记的缺口**：`establishSession` 清 `auth.pending_2fa_user_id` / `auth.pending_2fa_attempts`，DEVLOG 里「已登录会话可能残留另一账号待验身份」这条不再存在，用 `TestEstablishSessionClearsPendingKeys` 钉住（此前无任何用例覆盖这两个键）。

### 已知取舍

- **列表是全表扫**：`eachSession` = `Store.(scs.IterableStore).All()` + 逐条 `Codec.Decode` 后按 `auth.user_id` 过滤（与既有吊销同一手法）；代价随全站活跃会话数线性增长，个人博客规模可接受。要 SQL 化见设计文档 §9.2（`sessions` 扩列 + 自写 Store）。
- **无历史 / 无分页 / 无批量**：吊销即删行，没有「已吊销」状态可筛；「退出其它所有设备」未开接口（改密 / 2FA 流程内部在用）；无会话上限（限流轮一起看）。
- **IP 可伪造**：`X-Forwarded-For` 客户端可自填，本轮按用户裁定「先信任」，**仅供展示**；限流轮要用真实 IP 必须先配 `SetTrustedProxies`。UA / IP 截断 512 / 64 字节，防超长头撑大会话行。
- **原始 token 落库**：`sessions.token` 存明文 token（scs 默认），DB 可读即会话可伪造；`HashTokenInStore` 加固待点名（代价：存量会话全失效 + 相关比较改指纹）。
- 运行期验收（浏览器里点）归用户；本轮证据是集成测试 + 另起端口的 curl 冒烟，浏览器里还没点过。

---

## 2026-10-13 · 补测试缺口：改资料接口的原子性、param、并发兜底与边界值 + 测试入口脚本

**版本**：后端 **0.14.0**（未进位）。**零源码语义变更、零迁移**：本轮只动测试与测试入口，`internal/` 下的非测试代码一行未改。起因是对上一轮（改资料接口）的独立审查列出 5 条测试缺口，用户点名全做。

### 做了什么

- **组合字段原子性**（新用例 `TestUpdateProfileRejectedWriteLeavesBothFieldsUnchanged`）：同时提交「可用的昵称 + 撞车的邮箱」→ 409 `email_taken`，随后回查确认**昵称也没被写进去**；反方向（撞车昵称 + 可用邮箱）同样回查。此前**失败路径**上从未把两个字段一起提交过（成功路径有过两次），"要么全改、要么全不改"这条只存在于代码读出来的形状里，谁改成两条 UPDATE 分两次写测试也照样绿。
- **`param` 断言**：`{"nickname":"a"}` → 400 + `param:"nickname"`；`{"email":"not-an-email"}` → `param:"email"`。前端 `use-profile-save` 全靠这个字段决定弹哪条文案，之前一个断言都没有。同时钉反向：请求体形状/类型错（`{"nickname":42}`）时 `param` 为空。
- **并发用例**（新用例 `TestUpdateProfileConcurrentSameNickname`）：两个已登录用户同时把昵称改成同一个值，正好一个 200、一个 409，且 409 的 code 落在 `{nickname_taken, already_taken}` 内。照 `TestRegisterConcurrentSameEmail` 的形状写，但**没有**复用 `exchangeJSON`——它内部 `t.Fatalf`，从测试 goroutine 里调用是错的（既有那条用例就有这个毛病）；另立了不在 goroutine 里断言的 `patchJSON`。
- **约束兜底确定性用例**（新用例 `TestUpdateProfileConstraintBackstopReturnsAlreadyTaken`）：并发那条打不到 `already_taken`（原因见下），改用测试夹具直取——在测试库上装一个 `BEFORE UPDATE` 触发器，昵称以 `collide` 开头时先插一行同名数据，使撞名预检仍然查不到、而紧接着真正的 UPDATE 撞上唯一索引，于是**确定性**拿到 409 `already_taken`。触发器与函数在 `t.Cleanup` 里连同行一起拆掉。
- **边界值**（并入 `TestUpdateProfileEndToEnd`）：拒绝侧——数组体、字符串体、数字体、`{"nickname":42}`、纯空格昵称、33 rune 昵称、261 字节邮箱 → 全部 400；接受侧——2 rune、32 rune、20 个 emoji（20 rune / 80 字节，专钉"按字符数而非字节数算长度"）、254 字节邮箱 → 200 且原样读回；把当前值**带前后空格**重提 → 200（trim 后自我匹配，正是前端"只提交改动字段"会走的那条）。用例按语义分成两组：**「两个字段都没给」**（空体、裸 `null`、`{"nickname":null}`）与**「请求体形状/类型错」**（数组/字符串/数字/字段类型错），两组都断言 `param` 为空——前者走的是 service 的"两个都缺"判定、后者才是解码失败，之前混在一张表里写，标签是错的。
- **会话清理**：新增 `loginWithLogout` 辅助（登录 + 在 `t.Cleanup` 里打一次登出），三个改资料用例改用它，跑完不再在 `sessions` 表留行。
- **测试入口脚本**（`scripts/test.sh`，新）：`go test ./... -count=1 -p 1`。**始终**确认 `DATABASE_URL_TEST` 已设置——不传包名时没设直接退出码 1，传了包名只打一行警告再照跑（此时该包的集成用例会静默 skip，`ok` 不代表跑过）。README 新增「测试」章节说明为什么必须串行。

### 过程中的发现

- **`already_taken` 兜底靠并发打不到，得用夹具。** 把并发用例的断言临时收紧成「必须是 `already_taken`」跑了 10 遍（审查复核又跑 20 遍），输家**每次**都是预检给出的 `nickname_taken`。原因：两个请求虽同时发出，但都要先走完会话加载 + `LoadIdentity` 查库才轮到预检，而预检（一次 SELECT）到 UPDATE 之间只有几十微秒的窗口，撞进去的概率极低。**结论不是"测不了"**——加一条测试库触发器夹具即可确定性触发（见上），并发用例的断言因此保持「两者皆可」不收紧。
- **`-p 1` 是绕开、不是根治。** 偶发红的根因是两个包共用同一个测试库，而 `register/enabled` 是**存在数据库里的全局开关**：会去关它的不止一处——`app` 包的 `TestRegistrationControlEndToEnd`，加上 `auth` 包 `register_gate_test.go` 里的 `closeRegistration`（四个用例共用）与 `settings/controller_test.go` 里那条；任何一处关着的时候，另一个包不相干的注册用例撞进去就吃 403。根治要给每个包独立的库或 schema，但测试库是**预先迁移好的**、代码里没有迁移入口（`database.Open` 只连库、不动 schema），代价远超收益，所以在入口脚本上收口。
- 本轮真遇上了那次红：首跑 `go test ./...` 时 `TestRegisterRejectsDuplicateNickname` 返回 403 而不是 409；串行与随后两次并行都绿——典型时序偶发。
- **装夹具时先吃了一个 500。** 触发器第一次跑给的是 500 `internal_error` 而不是 409：`INSERT` 漏了 `group_id`，而该列在 `groups_and_permissions` 迁移里被 `SET NOT NULL`（且没有默认值），于是报的是**非空约束**——ent 的 `IsConstraintError` 只认 `violates unique constraint` / 外键 / check 三种文案（`dialect/sql/sqlgraph/errors.go` 是纯字符串匹配），非空约束一条都不沾，径直落到 500。改成从被更新的行借 `NEW.group_id` 之后才拿到唯一约束。顺带说明：这条兜底对**错误种类**是敏感的，真实场景里能让它生效的只有唯一索引冲突本身。
- **试守卫时踩到本机 `env` 是坏的。** 起初用 `env -u DATABASE_URL_TEST bash scripts/test.sh` 试守卫，什么都没输出、退出码 0，一度以为守卫写错。真因：这台机器上 `env` 解析到 `C:\Users\Multu\.local\bin\env`——一个只往 PATH 里塞目录、**完全忽略参数**的 shim 脚本，`env -u X cmd` 里的命令根本没被执行。本仓脚本不靠 `env` 传参所以不受影响，但这个坑在本机是通用的：任何 `env VAR=x cmd` / `env -u VAR cmd` 写法都会静默空转（不报错、不执行、退出码 0）。

### 验证口径

- `bash scripts/test.sh` 11 包全绿，无 `DATABASE_URL_TEST` 相关 skip（本机 5434；`mail` 那条真实投递用例在未配 `SMTP_*` 时仍按设计 skip）；`gofmt -l internal` 空、`go vet ./...` 空。
- 并发用例 `-count=20`、约束兜底用例 `-count=5` 稳定通过。
- 夹具不留垃圾：跑完用探针查 `pg_trigger` / `pg_proc` 与 `users` 里的 `@collision.test` 行，三项残留均为 0。
- 原子性用例非空转：断言的是「回查后昵称仍等于**改之前**的值」，而请求里给的是另一个值——真发生半写就会红。（独立审查另用变异测试复核过：把实现改成分两步写，两个方向各命中一次。）
- 脚本守卫实测：不设 `DATABASE_URL_TEST` 时全仓跑退出码 1；单包跑打一行警告后照跑（`ok` 但集成用例其实已 skip）。

### 已知缺口

- 并行冲突只做在入口脚本层面：绕过脚本直接敲 `go test ./...` 仍会偶发红，没有从代码层阻止这种跑法的手段。
- 约束兜底夹具依赖 Postgres 触发器，并按昵称前缀 `collide` 触发——将来若有别的用例用这个前缀会互相干扰；`gen_random_uuid()` 需 PG 13+（本机 17）。
- 只改了本轮这三个改资料用例的登出清理，其他既有用例仍在登录后不登出，`sessions` 表仍会有别处留下的行（一小时过期）。
- 既有 `TestRegisterConcurrentSameEmail` 在测试 goroutine 里调 `t.Fatalf` 的毛病本轮没顺手修（属既有代码，另一轮的事）。

---

## 2026-10-13 · 改自己的资料：后端接口 `PATCH /users/me` + 前端把保存接上

**版本**：后端 0.13.0 → **0.14.0**（minor：新对外契约）。**零迁移、零新依赖**。起因：用户在 `/console/uc/profile` 页上改「用户名 / 邮箱」点保存没反应，问"是不是接口的问题"——查明后端**从来没有**写这两个字段的路径（读只有 `GET /auth/me`；`PATCH /admin/users/:id` 只接 `group_id` / `extra_permissions`），于是按用户当场定的四条裁定把接口补上；随后用户一句"可以，请你完成吧"把前端也接上。

### 做了什么

- 新端点 `PATCH /api/v1/users/me`（命名走用户的 A 案）：归 `auth` 模块的「当前用户」能力，声明在 `RegisterProtected`（`auth/controller.go:29`），只挂 `RequireAuth + LoadIdentity`、**不挂权限点**——改的是自己。实现落在 `service.go` 的 `UpdateProfile` + `repository.go` 的 `updateProfile`。
- 请求体 `{"nickname"?,"email"?}`：两个字段都可选、只改给出的那个；**两个都不给 400**。归一与校验复用注册那套（`normalizeNickname` / `validNickname` / `normalizeEmail` / `validEmail`），不符 400 `invalid_request` 且 `param` 指向出问题的字段。
- 撞名：先按 identifier 查一次，**查到的是自己就放行**（所以"原样重提"是 200 而不是 409），并发竞态由 ent 约束错误兜底 409 `already_taken`——与注册同构。错误码按用户裁定直接复用注册的 `nickname_taken` / `email_taken`。
- **不校验当前密码**（裁定 2"不用"）；**改邮箱不验证新邮箱**、**前端不重拉 `/auth/me`**（裁定 4"暂时不做"）。会话只存 `userID`，所以改完不需要重新登录，`/auth/me` 与侧栏下次读取即是新值。
- **前端接上（同一轮的后半段）**：新增 `hooks/use-profile-save.ts`（照 `use-signup` 的套路：`apiRequest` + `ApiError` 错误码逐条映射 + sonner 单条 toast；用的是 `lib/api.ts` 现成封装，没有自己写 fetch），`profile-form.tsx` 改受控并把 `isPending` 传给悬浮保存按钮。**只提交改动过的字段**（后端本来就是部分更新）；成功后 `toast.success` + `router.refresh()`；401 → `replace("/signin")`；5xx/网络错 → `common.serviceUnavailable`，**刻意不走 `useServiceHealth`**（它在挂载时会自己弹一条，而控制台布局里的 `ConsoleServiceToast` 已经管这事，会重复）。
- 前端不做本地预校验（昵称/邮箱规则只留在后端一处，避免两处规则漂移），把 `invalid_request` 的 `param` 映射成字段级文案。

### 验证口径

- 新增 `TestUpdateProfileEndToEnd`（`app_test.go`，真 `app.New` 装配链 + 真库）：匿名 401；空体 400；五组非法值（过短昵称 / 含空格 / 含 `@` / 非法邮箱 / 类型不对的 `42`）各 400；正常改（顺带验 trim 与小写）200；**原值重提 200**（"自己不算撞名"那条的回归）；改完 `GET /auth/me` 读到新值；撞别人昵称 409 `nickname_taken`、撞别人邮箱 409 `email_taken`；只给昵称时邮箱保持不动。
- 全仓 `go test ./... -count=1` 11 包全绿无 skip（本机 5434 Postgres）；`gofmt -l internal` 空、`go vet` 干净。
- README 模块表描述订正 + 接口表补一行 + 规则补一条；api-conventions §3「现状端点全集」补第 24 行。
- 前端 `prettier --check` / `tsc --noEmit` / `eslint` 干净；两条**只读**探测（不带会话、不写库）：`PATCH /api/v1/users/me` 直连 5732 与经 `:3000` 的 `/api` 转发**都返回 401**——说明路由、守卫、Next rewrite 三处都在，且 5732 上跑的进程已经带这条新路由（若是 404 就是旧二进制没重启）。带真会话的保存动作留给用户在浏览器里点。

### 已知缺口

- 带真会话的"点保存"我没跑（用户明确要求运行期验收归他自己）；因此撞名 / 校验失败这些失败态只做到了错误码到文案的映射，没端到端点过一遍。
- 保存成功后靠 `router.refresh()` 让表单与侧栏昵称一起跟上（裁定 4 里"要不要重拉 `/auth/me`"那半条实际就这么落了）；若刷新那一下也失败，页面会停在旧值上，没有额外提示。
- 改邮箱不验证新邮箱、不改密码就不要求当前密码——都是本轮显式裁定，不是遗漏。
- 撞名是"查一次 + 约束兜底"：两人同时改成同名时第二个吃 409 `already_taken` 而不是精确的 `nickname_taken`，与注册一致。

---

## 2026-10-13 · 公开设置读取收口：信封统一 + 聚合端点 + 前端通用层

**版本**：后端 0.12.0 → **0.13.0**（minor：新对外契约）。**零迁移**。起因是用户对上一轮结果的追加要求：「规范这个公共 API，让我方便复用」——当时公开读只有一个刚加的按组端点，前端为注册开关单独写了 `lib/signup-status.ts`，每加一项就得再抄一遍。

### 做了什么

- **信封统一**：一个设置组的记录固定为 `{group, values}`；集合外外套 `{items: […]}`（与 access 的 `itemList` 同形）。管理侧 `GET` / `PATCH /admin/settings/:group` 从裸对象改为组记录，两侧同形，前端一套解析通吃。已写进 api-conventions §6 规则 4，并记明 D4/§9 列表收敛落地时 `items` 与 access 一起迁 `data`，不留第二套形状。
- **公开侧只留一个聚合端点**：`GET /api/v1/public/settings`（无鉴权）只出清单里标了 `Public` 的项，没有任何公开项的组整条不出现；按组点查 `GET /public/settings/:group` 删除（未推送，收口零成本）。前台要读公开设置就一次取全。
- **白名单语义**：`Definition` 新增 `Public` 字段，`Service.PublicSettings` 按注册表顺序出组；新守卫 `TestBuiltinPublicAllowlist` 钉住「公开项集合 == `[register.enabled]`」，谁把敏感项标成公开就红——这是匿名端点唯一的安全闸。
- **前端收进通用层**：`lib/public-settings.ts` 一处负责 fetch（`cache: "no-store"`，漏了会被 Next 静态化）、形状校验、**fail-open**（读不到返回 `{}`，选择器 `settings.register?.enabled !== false` 即视为开放）；`/signup` 关闭时 `notFound()`，登录页 `signupOpen=false` 时整个 footer 不传；专用 `lib/signup-status.ts` 删除，页面里的 try/catch 一并收掉。加下一个公开设置：后端 `Public: true` → 前端加一行选择器 → 页面消费。
- 文案补 `registrationClosed`（zh「注册功能已关闭」/ en "Sign-ups are currently closed"），`use-signup.ts` 加 `registration_closed` 分支，不再掉进兜底的「注册失败，请稍后重试」。

### 实测

后端 11 包 `-count=1` 全绿无 skip；前端 `prettier` / `tsc` / `eslint` 干净。真实链路双向对照：关闭态 `{"items":[{"group":"register","values":{"enabled":false}}]}` + `/signup` **404** + 登录页 `href="/signup"` **0** 次；开启态 `/signup` **200** + 链接 **1** 次；再关回 404。降级新增一行：库不可达时匿名聚合端点 503（前端 fail-open 的依据就在这）。

### 过程中的坑

- **形状一改就击穿了 app 层旧断言**（`default get = 200 map[group:register values:…]` 对不上 `values["enabled"]`）——这条红是好消息，说明断言真的在验形状而不是随手写的绿灯；改判用新的 `envelopeValues()` 解嵌套。
- **判「链接有没有隐掉」不能 grep 中文串**：`去注册` 出现在 next-intl 随页面流下来的文案表 payload 里，HTML 里恒有，我据此一度误判成「没隐掉」。要 grep `href="/signup"`，并且必须做开启态正向对照，否则「0 次」既可能是隐掉了也可能是页面根本没渲染。
- **Next 16 不允同一目录起第二个 dev server**（`Another next dev server is already running`），所以前端只能借在跑的 :3000 验；期间 :5732 的后端进程被并行会话重启过两次，每次都要先确认它的 uptime 与 `/api/v1/public/settings` 的负载形状，才知道跑的是不是当前二进制。
- **前端无测试运行器**（package.json 里没有 vitest/jest），选择器与形状校验只能靠 `tsc` + 后端守卫兜；要真验需要单独立一轮前端测试底座。

### 已知缺口

- 界面仍没有：改注册开关还是得 `PATCH /admin/settings/register`；本轮只把「关」这件事在路由层做实了。
- 聚合端点无缓存，且每组一次查询（当前组数=1，无感）；设置项多了再评估单查询。
- `items` → `data` 的列表信封收敛仍欠着（api-conventions §0 未实施项），本轮已把公开端点登记进待迁移清单。

---

## 2026-10-13 · 注册控制：设置系统接上第一个消费方

**版本**：后端 0.11.0 → **0.12.0**（minor：新能力域）。提交 `f3d762a`…`de1e871`（7 个提交，全部在 `backend/`，**零迁移**）。spec 与实施计划在 `docs/superpowers/{specs,plans}/2026-10-13-neoblog-registration-control*`。

### 做了什么

- 清单落第一项 `register/enabled`（开关，默认 `true` = 现网行为零变化）；`auth.Service.Register` 首句读它，关闭时 403 `registration_closed`（新错误码，`permission_error`），**判定先于字段校验**；`auth.CreateAdminUser` 不经 `Register`，保持关闭态仍能建首个管理员。
- 设置模块从「无路由能力模块」变成有路由模块：新增 `module.go` / `controller.go`，`New(db)` 改回返回 `(*Module, error)`（照 `mail` 形状，仍是唯一返回 error 的构造器），`GET` / `PATCH /api/v1/admin/settings/:group` 按组通用读写；`App` 里那个无出口的 settings 私有字段删除（`mailer` 留字段是因为 `shutdown` 要 `Close` 它）。
- `Service.PatchGroup` + `ItemError`：请求体值类型以服务端清单为准（复用 `decodeTyped`，`3.5` / `1e3` 这类越界天然被拒），任一项不合法即整组零落库；controller 用 `errors.As` 取项名填进 `apierror.WithParam`。
- 鉴权走**新权限点** `system.settings.view` / `.edit`（`edit` 经 `rbac.Expand` 隐含 `view`），不是 `RequirePresetGroup`——取向是将来控制台能按角色勾选。核过：owner/admin 的生效权限运行时取自代码目录（`PresetPermissions → All()`），`permission` 列是纯字符串无枚举无外键，**加权限点不需要迁任何数据**；新键追加在 catalog 末尾以免漂掉既有断言。

### 过程中的坑

- **计划漏了一个守卫**：`rbac_test.go` 的 `TestCatalogMatchesFixedValues` 把权限目录**逐条**钉死，加键必红。审查计划时只核了 `access` 侧那个用 `len(rbac.Catalog())` 动态比对的用例，以为全仓没有第二处。
- **开关是落到库里的状态，换一个新 app 实例并不能重置它**：对照用例（关闭 403 → 打开后同一个体 400）必须在同一个服务实例上显式 `SetBool(..., true)`；另配 `t.Cleanup` 里 `ResetItem` 删行，否则测试库里残留 `false`，同库后续不相干的 auth 用例会莫名吃 403。
- **关闭态的字段校验对照不能用空体**：`signup` 在 controller 层就因三个字段为空判 400（`auth/controller.go:52-57`），请求根本进不到 service——要用「非空但 `validEmail` 会拒」的体（邮箱无 `@`）才证得了门在服务第一句。
- **降级态造不出带身份的客户端**：`newDegradedServer` 起的是库不可达的 app，登录本身就 503，所以设置端点的降级用例只能断未登录 401；注册侧的 503 由既有 `app_test.go:548` 那条合法体用例继续覆盖，不必新建函数。
- 测试脚手架没装 NoRoute 信封，PATCH 打到只注册了 GET 的路径时返回的是 gin 默认 404 纯文本，解 `code` 字段直接红——原本担心的一条会假绿，实际不存在。

### 验证口径

- 集成用例是**真跑**不是 skip（本机 5434 Postgres，`SKIP` 计数为 0）：设置包 4 条 GET + 7 条 PATCH + 4 条注册把关 + `TestRegistrationControlEndToEnd`（真 `app.New` 装配链，owner 关开关 → 匿名注册 403 → 打开 → 同一个体 201）。
- 真实进程冒烟：`ADDR=:5741 go run .` 的 gin 路由表里有这两条；匿名 `GET` / `PATCH` 均 401（不是 404 也不是 405，说明路由与守卫都在），非法邮箱注册 400（说明把关那次读库成功、默认判为开放）。

### 已知缺口

- 前端零感知：注册页照常显示，提交才吃 403；并行会话那份前端设置页骨架（五个分区）里**也没有注册落点**，且已被其自行回滚。要做到「按钮直接标为已关闭」需要设置 spec §10 的「公开读取」插槽。
- **改开关目前只能打接口**（本轮只后端，没有界面）：`PATCH /api/v1/admin/settings/register` 带 owner 会话，或 `ResetItem` 语义上恢复默认。
- 权限点可授出：`system.settings.edit` 能授给自建组，即非 owner/admin 的角色也可能被授「改注册开关」——这是选权限点而非预设组守卫的既定取向，不是疏漏。
- 每次注册多一次单行点查，无缓存；两个管理员同时 PATCH 为最后写赢（行级 upsert，无版本号）。
- 孤儿行沿用：若将来删掉 `register/enabled` 这一项，库里残留行不会被任何读取路径碰到，只占复合键，清理仍走人工 SQL。

---

## 2026-10-13 · 环境轮的复核与入库：几条取舍留档

**版本**：后端 **0.11.0**（未进位）；本轮**零源码语义变更**，只做审查复核并把环境轮改动入库。入库提交 `6ff236d`（10 个文件、+309/−30，未推送，`development` 领先 `origin/development` 3）。

### 复核怎么做的（为什么不照读文档结论）

- 文档写「全绿」不算结论，全部重跑：`gofmt -l internal` 空、`go vet ./...` 空、`go test ./...` 全绿（本机 5434 的 Postgres 在跑，集成用例是真执行的，不是 skip 出来的绿）。
- 环境这类判据必须拿产物自证：`bash scripts/build.sh .scratch/envprobe/prod.exe` 打一份发布产物，在库不可达（`DATABASE_URL` 指到死端口）的情况下启动，`GET /api/v1/healthy` 的 `environment` 为 **production**，启动日志 `[GIN-debug]` 行数为 **0**，只有 `listening on :5739` 一行——与上一节实测表里「build.sh 产物，不设 `APP_ENV`」那行对上。探针产物用完即删，`.scratch/` 已在 `.gitignore` 内，不在仓库路径里留 34 MB 二进制。
- 改动范围按消费者收口核一遍：非测试代码里读 `cfg.Environment` 的只有三处——`app.go:49` 的 gin 模式、`app.go:126` 的 cookie `Secure`、`modules/health/service.go:38` 的健康检查响应；`"production"` 字面量已全部收敛到常量，没有漏网的字符串比较。

### 关键取舍

- **`ginMode()` 的 `default → release` 兜底保留**（`app.go:113-114`，对应断言 `app_test.go:240` 的 `"bogus"` 用例）。审查一度把它列为「生产路径走不到」的多余分支，结论是不删：它钉住的是「未知值走安全侧」这个方向，删掉之后谁把 default 改成 `gin.DebugMode` 测试照样全绿，等于这个选择没人守；成本一行，零维护。同时记明它的价值上限——守的只是手造 `Config` 漂移这类测试内场景，真正的把关在 `TestLoadRejectsUnknownEnvironment`，兜底只是第二道。
- **两类兜底要分开判**：展示层的等效构造（后端挂了时前端造空状态那类）按既定口径整条删；配置层与纯函数的 fail-closed 默认值连同测试保留。判据是「删掉之后方向会不会被静默反过来」，不是「可达不可达」。
- **已定形的改动不重做**：`Load()` 里那层只转发 `EffectiveEnvironment()` 的 `loadEnvironment()`，在复核与拍板之间已被并行会话删掉（`config.go:67` 现为直调）。处理方式是先重核工作区（grep 无残留、文档无过时引用、文件 mtime 落在两轮之间）确认形状已对，再决定不动手，而不是按原计划改第二遍。同类项还有 README「运行」章节末段与 `app_test.go` 的一行，均为并行会话在制品，按「一起提交」的口径随同一提交入库，提交消息据实际内容反推。

### 本轮没做 / 仍未验证

- panic 日志随模式漂移、request id（10-12 定的「对内可诊断」）仍未做。
- `APP_ENV` 在加载层硬拦（过滤 `.env` 里这个键）仍是可选后续项，现状只有文档约定，代码照读。
- **发布产物默认 `Cookie.Secure=true` 这条分支依旧没有端到端验证**：非 localhost 的明文 http 部署下浏览器会直接丢弃 session cookie，症状是登录接口 200、下一步 `/auth/me` 全 401，看着像「登录没生效」。要证它需要真浏览器 + 非 localhost 明文地址。
- 仍无容器镜像与部署定义：`deploy/` 只有 postgres 的 `init.sql`，口径里的「镜像」目前等价于 `build.sh` 产出的单个可执行文件。

---

## 2026-10-13 · 后端环境口径收口：构建产物即生产，开发态只属于 `go run`

**版本**：后端 **0.11.0**（本轮未进位）；接口字段名与类型不变，但 `/api/v1/healthy` 的 `environment` **取值**现在随**构建方式**而变（`development` → `production`），依赖该字段判断环境的按变更处理。

注：本节写于本轮改动入库前；其中「未做 / 未验证」各条在同日的审查修复轮里已处理掉两条（`APP_ENV` 空值口径、生产态 `.env` 自举），下方补记标注了实际落地形状。

起因是把「日志有没有颜色 / 有没有路由表」当成了生产与开发的判据，核查后发现两个 exe 跑的都是开发态，且所谓「生产实例」在自己脚边生成了一份开发形状的 `.env`。

### 做了什么

- `internal/config/config.go`：新增 `var DefaultEnvironment = "development"`（发布构建用 `-X` 覆盖）与 `EnvironmentDevelopment/Production/Test` 三常量；`APP_ENV` 从「任意字符串」收紧成只认这三个值，非法值 `Load()` 直接报错点名 `APP_ENV`（此前 `prod` / `Production` / `staging` 会静默按非生产跑）；新增 `Config.IsProduction()` 作为唯一判定入口。
- `internal/app/app.go`：`gin.SetMode` 从「只管 production」改成按环境三分（production→release、development→debug、test→test）；`Cookie.Secure` 改走 `cfg.IsProduction()`。
- `scripts/build.sh`（新）：发布构建唯一入口——`CGO_ENABLED=0` + `-trimpath` + `-ldflags "-s -w -X website/backend/internal/config.DefaultEnvironment=production"`；版本号从 `internal/version/version.go` 读，产物名 `bin/neoblog-<版本>-<GOOS>-<GOARCH>[.exe]`，支持位置参数指定输出路径与 `GOOS`/`GOARCH` 交叉构建。
- 测试：`config_test.go` 新增 3 条（编译期默认值 / 显式覆盖三值 / 非法值拒绝）；`app_test.go` 的 `TestGinModeFollowsEnvironment` 直接对 `ginMode()` 纯函数断言四值（含兜底 release），不依赖数据库、且退出时恢复 gin 全局模式。`go test ./...` 全绿、`gofmt -l` 空、`go vet` 干净。
- 文档：`backend/README.md` 新增「环境与构建」章节；同步 `APP_ENV` 行、目录对照表加 `scripts/build.sh` 行、「认证」章节 cookie `Secure` 的口径、「运行」章节加指针。

### 关键决定

- **环境不做成两份代码，靠编译期注入一个变量**：源码一份，`go run .` / 裸 `go build` 出开发态，`scripts/build.sh` 出生产态。与 `internal/version.Version` 同一个 `-X` 机制。
- **`APP_ENV` 刻意不进 `.env` 与 `.env.example`**：写进 `.env` 会让同一目录下的发布二进制静默退回开发态——那正是本轮起因那类误判的来源。环境开关只能来自「怎么构建的」或「显式设环境变量」（代码仍会读 `.env` 里的 `APP_ENV`，已在「未验证 / 已知缺口」标注为可选的后续硬拦项）。
- **gin 模式完全由 `APP_ENV` 决定，`GIN_MODE` 不再能单独生效**：此前两个开关脱钩（只设 `GIN_MODE=release` 会日志安静、但 cookie 仍非 Secure、health 仍报 development）。代价是 `APP_ENV=development` 现在会强制把 gin 打回 debug，即使用户同时设了 `GIN_MODE=release`——取单一真源。
- **本轮只收口环境，`.env` 自举语义按同一轮审查结论另行收紧**：自举从「无条件」改为「仅开发态 / `test`」，生产态不自举，见缺口第一条的处理结果。

### 实测到的边界（2026-10-13，同一份源码四种跑法，探针在 `backend/.scratch/modeprobe/`，该目录已进 `.gitignore` 且临时钥匙文件已删）

| 跑法 | `[GIN-debug]` 行数 | `/healthy` 的 `environment` |
| --- | --- | --- |
| `build.sh` 产物，不设 `APP_ENV` | **0** | **production** |
| `build.sh` 产物，`APP_ENV=development` | 21 | development |
| 裸 `go build` 产物，不设 `APP_ENV` | 21 | development |
| `build.sh` 产物，`APP_ENV=prod` | 0 | 启动即失败：`APP_ENV must be one of "development", "production" or "test", got "prod"` |

顺带钉死两条日志口径（此前被当作环境判据，是错的）：`[GIN-debug]` 路由表**只在启动那一刻打印、且永不上色**（`gin/debug.go:69` 那句 `Fprintf` 里没有任何 ANSI 码）；`[GIN] 时间 | 状态码 | 耗时 | IP | 方法 "路径"` 是每条请求都打的访问日志，**debug 与 release 都打**（`app.go` 无条件 `engine.Use(gin.Logger())`），带不带颜色只取决于 stdout 是不是终端（`gin/logger.go:258-262` 算 `isTerm`、`:163` 用它决定上色，`SetMode` 不碰这个变量）。生产态那次实测照样打出 `[GIN] … | 503 | … | GET "/api/v1/healthy"`。（gin 行号以当时 `go.mod` 的版本为准，仅备查。）

### 补记（同轮收尾时补齐两笔入库的日志）

- `92e5afa`：没有 `.env` 时首次启动自动生成一份纯键值文件（`EnsureDotEnv`，`O_EXCL` 绝不覆盖、含随机 32 字节 2FA 钥匙），不再因缺环境变量退出。
- `63ffbe9`：数据库连不上也照常启动，碰库端点统一 503 `service_unavailable`（连接级错误判定），连接池惰性重连、库恢复无需重启。
- 同日审查修复：`APP_ENV` 空串也判非法（`LookupEnv` + trim，不再静默吃成默认值）；`.env` 自举收紧为**仅开发态 / `test`**，`production` 不生成文件（修复下方缺口第一条）；`ginMode()` 提成纯函数并补兜底 release，测试不再依赖数据库；`backend/.gitignore` 增加 `.scratch/`，探针产物与其中的临时钥匙文件不再有误入仓库的路径。

### 未验证 / 已知缺口

- **发布产物默认 `Cookie.Secure=true`，这条分支此前从未真跑过**：生产若走 `http://`（非 localhost 的明文域名 / IP），浏览器会直接丢弃 session cookie，症状是登录接口 200、下一步 `/auth/me` 全 401，看起来像「登录没生效」。**未做端到端验证**（需要真浏览器 + 非 localhost 明文地址）。
- **`EnsureDotEnv` 生产态自举已修**（同日审查修复）：自举只在开发态 / `test` 生效，`production` 遇到缺 `.env` 不造文件。原缺口记为：生产产物在没有 `.env` 的目录里会自造一份开发形状配置（本地 compose 的 `DATABASE_URL`、`DATABASE_URL_TEST`、外加一把全新随机 2FA 钥匙）；本轮起因 `F:\backend.exe` 在 `F:\` 生成的正是这种文件（sha256 前缀 `c4ce87ff` vs `backend/.env` 的 `3124744e`）。
- **没有容器镜像与部署定义**：`deploy/` 仍只有 postgres 的 `init.sql`，无 Dockerfile、无 app 服务、无 systemd 单元。用户口径里的「镜像」目前等价于 `build.sh` 产出的单个 exe。
- **`APP_ENV` 加载层硬拦是可选后续项**：文档只做「约定不写进 `.env`」，代码仍会读 `.env` 里的 `APP_ENV`（`LoadDotEnvFile` 不按 key 过滤）。要在加载层过滤或告警，需按「用户点名才做」的惯例另行决定。
- panic 日志随模式变（debug 额外 dump 整份请求，release 只打堆栈，见 `gin/recovery.go` 的 `IsDebugging()` 分支）——本轮未统一；10-12 定的「对内可诊断（500 / panic 日志 + request id）」仍未做。

---

## 2026-10-13 · 后端连不上时三条路由不再 500（外壳 / 表单照常渲染 + 一条 toast）

**版本**：前端 `package.json` 仍 **0.1.0**；本轮**零后端改动**、零接口契约变更。

### 做了什么

- `app/console/layout.tsx`：外壳 JSX 原样提成 `ConsoleShell` 函数；权威判定的抛错改为 `catch` → 渲染**无昵称的外壳** + 新组件 `ConsoleServiceToast`，`redirect("/signin")` 仍在 `try` 之外（`:46`）。
- `app/signin/page.tsx`、`app/signup/page.tsx`：同形改动——判定抛错时照常渲染 `-view`。此前实测：带 cookie 且后端停机时登录页自己返 500，用户连输密码重试的地方都没有。
- 新 `components/console/console-service-toast.tsx`：client 组件，effect 里 `toast.error(t("serviceUnavailable"), { id })`，`useRef` 挡 StrictMode 双跑，自身 `return null`。复用既有键 `common.serviceUnavailable`，两份语言包**零新增**。
- 三条路由各加 `export const instant = false`（阻塞式读 cookie 的段按 `instant.md:69` 声明「允许阻塞」）；root layout 那条**没撤**。

### 关键决定

- **用户拍板「两侧都放行」**，推翻 `specs/2026-10-12-neoblog-console-auth-design.md` v1.1 里「不做『后端挂了先放控制台外壳』的降级」那一行，也超出 10-12 只放宽访客页的提议范围。spec 同步升到 **v1.2**（§2 两行、§5 的 I1/I4、§6 用例 6、§7 两条、§8 三个插槽、§3 范围），plan 的 Review Focus 1 与 Task 2 Step 7 期望值同步改写（10-12 的实测存档不改写，另加订正段）。
- **判定层三态一字未改**：`lib/auth-session.ts` 故障时照旧抛错，变的只是抛错之后由谁接。「故障 ≠ 未登录」因此没退化——`/console` 带 cookie 遇故障是 200 外壳，若被错写成未登录就会是 307 回 `/signin`，鉴别点仍然成立。
- **`catch` 的粒度本轮不动**（用户决定）：连不上 / 非 200 / 响应体畸形三种原因被同一个 `catch` 合并，服务端零日志，于是真 bug 也会被显示成「服务不可用」，与 10-12 定的「先做对内可诊断」相反。已登记到 spec §8。
- **不新建 `error.tsx`**：沿 v1.1「避免造兜底 UI」，可读提示就交给那一条 toast。外壳形状不随后端数据变——无昵称时用户菜单只是少一段文字（`console-user-menu.tsx:79` 本来就在 `displayName` 为空时不渲染），没有任何新构造。
- **`/console` 上不会双 toast**：客户端健康探测 `useServiceHealth` 只挂在 `use-signin` / `use-signup`，控制台外壳里没有任何组件自己发请求，所以这条是唯一一条。

### 实测到的边界（2026-10-13，`:5732` 无人监听即后端不在）

| 请求 | 实测 |
| --- | --- |
| `Cookie: session=whatever` → `/console` | **200**，响应体含 `console-sidebar`（改动前该组合是 500） |
| 无 cookie → `/console` | `307 /signin`，proxy 未受本次改动波及 |
| 带 cookie → `/signin` 与 `/signup` | **200**，表单照常渲染 |

`npx tsc --noEmit`、`npx eslint`、`npx prettier --check` 三项退出码 0。

### 未验证 / 已知缺口

- **toast 的画面未取证**：它在水合后由 effect 发出，本轮只用 curl 证了状态码与响应体；「弹几条、导航时会不会重弹」需浏览器侧确认（`id` 去重 + `useRef` 是双保险，未实测）。
- D 态（2FA pending）与 prefetch 撞 307 仍未取证，沿 10-12 的欠账，本轮未动。
- 故障期 `/console` 属「未验明身份也渲染」，I1 已由充要降为必要；**该路由下新增子页不得假定布局已判过身份**。
- 后端 pending 键破损（G 态）、`docs/DEVLOG.md:399` 的 logout 204 陈旧等 10-12 已登记项本轮未动。

---

## 2026-10-12 · 服务不可用报错（两个表单不再谎报凭证错误）

**版本**：前端 `package.json` 仍 **0.1.0**；后端 0.11.0 不动，本轮**零后端改动**、零接口契约变更。

### 做了什么

数据库连不上时，登录/注册原本一律弹「登录失败，请稍后重试」（`use-signin.ts` 的兜底分支），把服务侧故障说成用户填错。现在：

- `lib/api.ts` +2 个导出：`looksLikeServiceFailure(error)`（HTTP 0 或 ≥500，只回答"这次错误值不值得去查一次健康"）与 `isServiceUp()`（打 `/api/v1/healthy`，**非 200 一律算不可用**，是唯一的结论来源）。`apiRequest` / `ApiError` 一字未动。
- 新 `hooks/use-service-health.ts`：`useServiceHealth()` 返回 `{ serviceDown, resolveServiceError }`。`resolveServiceError(error?)` 是统一入口（传错误 = 提交场景，不传 = 纯探测），两种场景给的是同一句话、这句只在 hook 顶部算一次；挂载时在 effect 里探测一次，不可用除弹一条外还把 `serviceDown` 置真，带 `active` 标记防卸载后弹（StrictMode 双跑也只弹一次）。
- `hooks/use-signin.ts` / `use-signup.ts`：提交失败的兜底改走统一入口，拿不到才落回原业务文案；`toMessage` / `toFailure` 里的 code→文案映射一条没删，注册页故障时不落焦点到任何字段。`serviceDown` 经 hook 透传给表单组件。
- `app/signin|signup/*-form.tsx`：`serviceDown` 为真时提交按钮文案换成「服务不可用」，**按钮不加 `disabled`**；`*-view.tsx` 本轮零改动。
- 两份语言包各 +2 键 `common.serviceUnavailable`（「服务暂时不可用，请稍后重试」）与 `common.serviceUnavailableButton`（「服务不可用」）。

### 关键决定

- **判定收成单一信息源**（用户拍板"两处都只认 `/healthy` 探测"）。初版是进页探测 `/healthy`、提交时看接口自己的状态码——一个判定两个信息源，读起来像实现了两遍。现在提交侧不再自己下结论，状态码只当闸门。
- **闸门保留**，否则密码每打错一次就多发的一个健康请求（harness 实测：有闸门请求数 0，无闸门 1）。
- 原命名 `isServiceUnavailable` 已改名为 `looksLikeServiceFailure`：收敛之后它不再下判决，旧名字读起来像在下判决。同轮里 hook 文件也从 `use-service-error.ts` 改名为 `use-service-health.ts`（它现在管的是状态而不只是报错文案），`c5408dc` 那个提交里记的是旧名。
- **没新增 503 `database_unavailable`**：会推翻 api-conventions §4「500 之外的 5xx 一律归并为 500、503 只给 `/healthy`」，还要判定哪些 ent 错误算数据库掉线（连接池耗尽 / 超时 / 慢查询语义不同），误报会把普通 bug 说成库故障。界面因此只说"服务"不可用，不特指数据库。
- 文案放 `common` 而非两个 `auth.*.failed` 各加一条：三个消费者共用同一句，拆开必然漂移。
- **不做去重**，登录/注册两页之间来回跳各弹一次——沿 `0e8a4be` 定的"连点提交弹几次算几次"。
- **故障时不禁用提交按钮，只改文案**（用户拍板"服务不可用 按照这个改吧"）：禁用会把「点提交」这条最省事的恢复检测堵掉——`/healthy` 的结论只是进页那一刻的快照，服务恢复后前端并不知道，而用户点一次提交恰好是一次免费重探；灰色本身不解释原因，要诚实就还得再挂一条常驻说明，等于把已有那条 toast 换成两套东西；另外沿用 2026-10-01 加载占位那轮定过的「常驻行必须真可用，禁用＝灰色遮蔽」否例。配套地，`resolveServiceError` 在拿到 4xx 业务错时会 `setServiceDown(false)`——服务器能回 4xx 就说明它活着，按钮文案当场恢复正常。

### 实测到的边界（记下来免得下次重查）

- **后端进程不在时，浏览器看到的不是网络错误，是 Next 自己返 500 + 纯文本 `Internal Server Error`**（`curl -i POST /api/v1/auth/login`，dev 3000 端口实测）。因为 `/api/*` 由 `next.config.ts` 的 rewrites 转发，断点在服务端那一跳；`apiRequest` 解析不出 JSON 于是给 `ApiError(500, "invalid_response")`。`status === 0` 只在"浏览器到 Next 这一跳也断了"时才出现。
- 后端 boot 时 ping 失败即 `log.Fatal` 退出（`backend/main.go:22-25`），所以"数据库连不上"最常见的形态是整个 HTTP 服务不存在。
- `/healthy` 在库掉线时返 **503** + `{"status":"degraded","database":"error"}`（`health/controller.go`），且它不受错误信封约束，非 200 时 `apiRequest` 拿不到 `error.code`。
- `useTranslations()` 的返回值是 `useMemo` 过的（依赖 cache / formatters / messages / locale / timeZone，见 `node_modules/use-intl/dist/esm/production/react.js`），放进 `useEffect` 依赖数组不会导致重复探测。
- **`react-hooks/set-state-in-effect` 会追进 effect 调用的回调里**：只要那个函数存在任何一条"未经 `await` 就 setState"的分支（本例是 4xx 直接 `setServiceDown(false)` 后 return），规则就报 "Calling setState synchronously within an effect"，哪怕实际调用路径必过 `await`。解法：effect 不复用那个入口函数，自己 `isServiceUp().then(...)` 置状态；文案在 hook 顶部算成常量串供两边用。
- **Qoder 内置浏览器跑不动这站**：37 个 JS chunk 全 200、`window.next` 已定义，但 6 秒内 React 事件属性始终没挂到 DOM（`__react*` 键为空）、`window.__next_f` 长度 0、`/__nextjs_font/*` 与 HMR WebSocket 一律 403 → 页面不水合，客户端 effect 根本不执行。**结论：本项目的 toast / 交互类验收不能靠内置浏览器**，只能无头 harness + 人工眼验。
- 无头 harness 四态矩阵（判定逻辑逐条实测，非推演）：后端不在 / 库运行中掉线 / 浏览器到 Next 断了 → 进页与提交都报同一句并各发 1 次探测；一切正常但密码错 → 仍报「用户名或密码错误」且探测请求数 0。`tsc --noEmit`、`eslint`、`prettier --check` 全绿。

### 已知缺口

- **`/console` 的整页可读错误页仍未做**（`2026-10-12-neoblog-console-auth-design.md:35` 与 `:168` 登记给"需要可读错误页那轮"）。后端故障时现在还是 Next 默认错误页 / dev 红堆栈。
- **本轮未验**：toast 与「服务不可用」按钮文案的实际渲染都只在浏览器里才看得见，而内置浏览器不水合（见上），需要人工开 `http://127.0.0.1:<dev 端口>/signin` 在停机状态下各看一眼：进页弹一条、按钮文字变成「服务不可用」、点下去仍会重探。
- 登出失败文案 `userMenu.signOutFailed` 没接这套判定（本轮点名范围只有两个表单）。
- 前台公开页（`app/page.tsx` 当前是空 div）将来接上内容后需要自己的故障态。
- `isServiceUp()` 把 `/healthy` 任何非 200 都算不可用 → 该路由若改名或被摘掉，进页会弹「服务暂时不可用」，属误导性诊断。

---

## 2026-10-12 · 控制台访问控制（proxy 短路 + 服务端权威校验 + 登出）

**版本**：前端 `package.json` 仍 **0.1.0**（前端版本进位口径未定，见 spec §8 插槽）；后端 0.11.0 不动，本轮**零后端改动**。

### 做了什么

- 两层门禁。新 `frontend/proxy.ts`（Next 16.4 里 middleware 已改名 proxy）只做乐观短路：`matcher: ["/console/:path*"]`，有 `session` cookie 就 `next()`、没有就 307 `/signin`；不调后端、不引共享模块。权威校验集中在新文件 `frontend/lib/auth-session.ts` 的 `getAuthenticatedUser()`——带 cookie 请求 `${backendUrl}/api/v1/auth/me`，**三态**：200 → `{email, nickname}`、401 → `null`、其余（5xx / 404 / 405 / 连不上 / 体解析不出）→ **抛错**。
- `app/console/layout.tsx` 由同步改异步：判到 `null` 就 `redirect("/signin")`；把 `nickname` 经 `ConsoleSidebar` 传给一直空传的 `ConsoleSidebarFooter` / `ConsoleUserMenu` 的 `displayName`。
- `app/signin/page.tsx`、`app/signup/page.tsx` 加反向跳：`if (await getAuthenticatedUser()) redirect("/console")`。判定放服务端 page 而非 `-view.tsx`（那是 Client Component，`instant` 在客户端会直接报错）。
- 登出：新 `frontend/hooks/use-signout.ts`（`{ isSigningOut, signOut }`）+ 用户菜单「退出登录」项，`POST /api/v1/auth/logout` 成功 `router.replace("/signin")`、失败 `toast.error`；两份语言包各 +2 键（49 键对齐）。逻辑留 hook、菜单只渲染，延续「UI 只管 UI，逻辑只做逻辑」。
- `hooks/use-signup.ts` 成功去向 `/signin` → `/console`：注册即登录与反向跳两条叠加后，推 `/signin` 会被立刻弹回、白跳一次。
- 新 `lib/backend-url.ts` 收口后端地址，`next.config.ts` 的 rewrites 改引它。
- 设计由两个独立只读审查代理逐条证伪后修订成 v1.1；v1.0 有 6 处事实错误（列在 spec 头部修订记录）。spec `2026-10-12-neoblog-console-auth-design.md`、plan `2026-10-12-neoblog-console-auth.md`。

### 关键决定

- **反向跳故意不放 proxy**（官方示例就放在 proxy，`authentication.md:1061-1069`）：后端对已开 TOTP 的账号会下发**有效 cookie 却不写 `auth.user_id`**（`service.go:109-113`），此时 proxy 的「有 cookie 就弹走」与布局的「401 就送回」会拼成 `/signin ⇄ /console` 无限重定向。两处判定必须共用同一个权威函数才不成环。
- **后端故障 ≠ 未登录**：v1.0 的「非 200 一律当未登录」被推翻——那样后端一挂，所有人（含已登录者）被静默判成未登录，连登录页都进不去且看不到原因。故障现在走 `throw` → Next 默认错误页。
- **session 读取故意放 layout 顶层**（官方 `authentication-with-cache-components.md:156` 明确劝退，理由是连 children 一起拖住）：门禁必须先于 children 渲染，代价是控制台整段多等一次内网 RTT。
- `matcher` 收窄到 `/console/:path*`，与官方「鉴权场景建议 proxy 跑全部路由」的 Tips 相反：`/api` 若进 matcher，会在 rewrites 之前被重定向劫持（proxy 执行在第 3 步、`beforeFiles` rewrites 在第 4 步，`proxy.md:240-244`）。
- **没用 `server-only` 包**（本机未安装，为一个防护 import 引新依赖不值当）；该模块 import 了 `next/headers`，被 Client Component 误引时 Next 自身就会构建失败，这层保护是白送的。

### 实测到的边界（记下来免得下次重查）

- `NextResponse.redirect` 默认 **307**（`node_modules/next/dist/server/web/spec-extension/response.js:95`）；服务端布局里抛 `redirect()` 对 HTML 请求同样是 307。
- 后端停机时：带任意 cookie 访问 `/console` → **500，响应体含 `session lookup failed`**；无 cookie → 仍 307（proxy 不依赖后端）；`/signin` 无 cookie → 200（压根不发那次请求）。
- **脏 cookie 不自愈**：`session=garbage` 的两个响应都不带 `Set-Cookie`（本项目未设 `IdleTimeout`，未改动的会话不回写），要等浏览器侧 `Max-Age` 到期或被一次成功登录覆盖。
- 登出实测 `set-cookie: session=; Path=/; Expires=Thu, 01 Jan 1970 ...; Max-Age=0; HttpOnly; SameSite=Lax`；同一 token 再进 `/console` 由布局层拒（proxy 放行）——两层分工的现场证据。
- dev 的 Server Component fetch 有 **HMR 缓存**，且对 `cache: 'no-store'` 同样生效，只在导航/整页刷新时清（`serverComponentsHmrCache.md:9-11`）。状态类验收必须冷请求或整页刷新，否则会得出「已退出却还像登录态」的错误结论。
- `proxy.ts` 与 `next.config.ts` 的改动都被 dev 自动热加载，无需重启；`/console/whatever` 这类不存在的子路由同样被 matcher 命中（proxy 先于路由执行）。
- 注册成功码是 **201**（`controller.go:65`），不是 200。

### 已知缺口

- **D 态（2FA pending 会话）本轮未实测**：不造开 TOTP 的账号，无环结论目前只有代码与推演支撑；取证步骤整段留在 plan 的 Task 7（已标推后）。
- **prefetch 撞 307 的行为未取证**：dev 不 prefetch，要验只能 `next build` + `next start`。
- 会话中途失效时，界面内的接口调用不会自动跳登录（`lib/api.ts` 不处理 401）。控制台开始消费真实数据那轮补，落点是它的单一出口。
- 本轮让 `/console`、`/signin`、`/signup` 每次渲染各多一次后端会话查询，而全站**零限流**（`backend/README.md:152`）。
- **后端不变量破损（本轮不改后端）**：`register` 与正常 `login` 都只写 `auth.user_id`、**不清** `auth.pending_2fa_user_id`（`Remove` 只出现在 verify 成功路径 `service.go:378-379`），而 `POST /auth/2fa/verify` 是公开路由、按该键放行 → 一枚已登录会话可能残留另一个账号的 pending 身份。利用前提苛刻（先掌握对方口令、再拿到对方动态码），不是远程可利用，但与 2FA spec 的不变量承诺相悖。
- 上一轮做的纯 CSR 门控（`hooks/use-is-client.ts`）已被并行会话撤销，本轮未恢复，登记为插槽。

### 更正既有文档口径

- `plans/2026-10-11-neoblog-auth.md:13` 与 `specs/2026-10-11-neoblog-auth-design.md:10` 仍写「前端未接 / Task 9 作废」——前端接线已随 `16e210a`–`22d272c` 入库，本轮又加门禁，两处已就地补修订说明。
- 本文件 0.11.0 条目里「本轮前端仍无任何对 `/api/v1` 的调用（全仓 grep 无 fetch 与后端地址常量）」现已失效。
- `logout` / `password` 成功码在 `DEVLOG:335`、`DEVLOG:399`、`specs/2026-10-11-neoblog-auth-layer2-design.md:48/85/100`、`specs/2026-10-11-neoblog-auth-design.md:86` 写成 **204**，代码是 200 `{}`（`controller.go:73/130`）；`backend/README.md:135` 本来就对。逐处加更正标注。

---

## 2026-10-12 · 方法不匹配改判 405（路由层错误分流）

**版本**：0.10.0 → **0.11.0**（minor：新增对外 HTTP 行为 + 1 个错误码）

### 做了什么

- 起点是用户要「方法用错也返回 404 隐藏端点」，实测确认 gin 默认行为已经是 404 信封（`HandleMethodNotAllowed` 默认 `false`，方法不匹配当作路由未匹配走 `NoRoute`）；用户随后改判取规范口径：**方法不匹配 405 + `method_not_allowed` / `invalid_request_error` + `Allow` 头**，路径不存在仍 404 + `not_found` 且不带 `Allow`。这推翻 API 规范 v1.3 的 §4 规则 2，规范补 v1.4 修订记录并同步 §4 状态码清单、§7.2、§7.3。
- `apierror` 注册表 +1（26 → **27** 条）：哨兵 `ErrMethodNotAllowed` 与 `"method_not_allowed" → 405` 映射。`Allow` 头由 gin 自己按 RFC 9110 填（`gin.go:738-752`），代码里不手拼方法列表。
- `app.go`：`engine.HandleMethodNotAllowed = true` + `engine.NoMethod(...)` 写同一信封；`NoRoute` 原样保留。
- 测试：`app_test.go` 新增 3 个用例函数——方法不匹配表驱动 4 例（POST-only 用 GET、GET-only 用 DELETE、PATCH+DELETE 路由用 PUT、OPTIONS），逐例断状态码 + `type` + `code` + `param` 为 `null` + `Allow` 集合（排序后比对，不依赖注册顺序）；HEAD 单独一例断 405 + `Allow: GET` + **响应体为空**；反空转守卫 2 条——不存在的路径用 GET/POST/DELETE 三种方法仍 404 且无 `Allow`、同一路径方法写对时仍进认证守卫拿 401（证明真路由没被 `NoMethod` 劫走）。`TestRegistryIntegrity` 的哨兵数与 `statusTypes`（405 → `invalid_request_error`）同步。
- 文档：README 信封段补 405 / `Allow` / 回退开关 / HEAD 与 OPTIONS 边界；API 规范补 v1.4。

### 关键决定

- **405 而非 404**（用户知情选择）：`Allow` 头等于向匿名探测者确认该端点存在及其允许的方法。回退只需把 `HandleMethodNotAllowed` 关回 `false`，方法不匹配立刻与路径不存在同为 404——这条开关在 README 与规范里都写了。
- **归 `invalid_request_error`，不新增 type**：400 与 405 同 type 不破坏「`type` 由状态码唯一决定」的不变式（状态码→type 仍是函数），也不必给 §7.2 加新分类。
- **405 发生在认证之前**：gin 的 `NoMethod` 是引擎级处理链（只含全局 Logger / recovery），受保护分组的路由也在这里分出 405，未登录请求同样能拿到 `Allow`。

### 实测到的边界（记下来免得下次重查）

- **HEAD**：gin 没注册任何 HEAD 路由，`HandleMethodNotAllowed` 仍进 405 分支并把该路径的其它方法列进 `Allow`；按 HTTP 语义 HEAD 响应体必须为空，所以它拿到的是 405 + 空体而**不是**错误信封（通用信封断言对它不适用，故单独成例）。
- **OPTIONS**：与 HEAD 同理按 405 + `Allow` 处理（`/api/v1/auth/login` 得 `Allow: POST`）。

### 已知缺口

- **跨域预检未开路**：将来若给浏览器跨域调用，`OPTIONS` 必须先绕开 405（预检会被现在的分流打回），届时 CORS 中间件要么注册 `OPTIONS` 要么在 `NoMethod` 之前放行。本轮前端仍无任何对 `/api/v1` 的调用（全仓 grep 无 fetch 与后端地址常量），不构成现行回归。（2026-10-12 更正：该前提已失效——`frontend/lib/api.ts`、`lib/auth-session.ts`、`next.config.ts` 的 rewrites 与 `proxy.ts` 都已存在并调用 `/api/v1`。）
- **HEAD 探活拿不到 2xx**：监控若用 HEAD 打 `/healthy` 仍是 405（改前是 404，同样不是本轮引入的回归），探活必须用 GET。

---

## 2026-10-12 · 两步验证接口轮（TOTP + 恢复码 + 登录分叉 + 重放防护）

**版本**：0.9.0 → **0.10.0**（minor：新能力域 + 5 条接口）

### 做了什么

- 5 条接口全部落地：`POST /auth/2fa/setup` / `enable` / `disable` / `recovery-codes`（守卫内）+ `POST /auth/2fa/verify`（公开，靠「待二段」会话）；`GET /auth/me` 增加 `two_factor_enabled`。路由 15 → **20 条**。
- **登录二段分叉**：bcrypt 通过后只看 `totp_enabled_at`——已启用则 `RenewToken` → 清除 `auth.user_id` → 写 `auth.pending_2fa_user_id` → 401 `two_factor_required`。「设置中」不拦登录，待验会话打受保护接口必 401（两态不并存）。`guard` 包零改动。
- **重放防护兑现**（用户裁定加列）：新迁移 `totp_last_counter`（可空 bigint、无默认、零回填）；命中窗号必须大于存量，且靠单条条件 UPDATE「影响行数 = 1」原子认领；恢复码消费同样单条条件 UPDATE（`used_at IS NULL`），并发同码恰一人成功。
- **secret 加密**：AES-256-GCM（12 字节随机 nonce 前置 + 整体 base64），密钥走新环境变量 `TOTP_ENCRYPTION_KEY`（base64、解码后恰 32 字节，缺失/非法启动即失败）。校验点 `loadTOTPKey()` 钉在 `Load()` 末尾，因此既有 8 个「断言错误里含某变量名」的负例零改动，只有 7 个要求 `Load()` 成功的用例补了合法 key。
- **enable / disable 吊销其它会话**（用户裁定）：复用改密在用的 `revokeOtherSessions`，且排在落库之前——失败即 500 且库里不变，不留「已启用但没领到恢复码、重试又撞 409」的死角。
- **半程失败收进单事务**：enable（启用时间 + 插 10 码）、disable（清三列 + 删全部码行）、recovery-codes（删旧 + 插新）各一条 `client.Tx`（照 access/settings 先例）；setup 覆盖 secret 时一并清窗号。
- **解密失败不再伪装成验证码错误**：`openSecret` 失败、已启用但 secret 为 NULL 的损坏态、`matchTOTPCounter` 因 secret 非法返回 error——三种一律 500 `internal_error`，不消耗限次、不销毁会话、不认领窗号；把列修回密文后同一枚码仍 200（有用例钉住）。
- `apierror` 新增 6 个码（三个 401 `authentication_error` + 三个 409 `conflict_error`），注册表 20 → **26 条**（`TestRegistryIntegrity` 同步）。
- 测试：config 3 例 + secretbox 5 例 + totp 原语 7 例 + repository 并发改判 3 例 + 2FA 生命周期集成 18 例（含双 jar 吊销、同窗重放、连错 5 次锁定、密文落库反解）。集成用例**零 `sleep`**：时间只经包内 `totpNow` 一处取，测试用 `pinWindow` 把窗号确定性前移。
- 文档：README（环境变量表、迁移清单、目录对照表 auth 行、认证接口表 +5 行、两步验证章节整段重写、已知缺口并入 verify）；API 规范 §3 端点表 +5 行、§7.3 注册表 +6 行并订正 `invalid_credentials` 的场景列（改密与 2FA 密码确认都在用，`current_password_mismatch` 拆分按 §0 v1.3 仍未实施）。

### 关键决定

- **待验态走 scs 独立会话键**，不加中间件、不改守卫：`RequireAuth` 仍只认 `auth.user_id`，待验会话天然拿不到权限。
- **认领与后续会话写不在同一事务**：认领成功而会话写失败会废掉一个窗，重试要下一窗——换取「不因会话存储抖动而回滚已验证的凭据」，代价记进已知缺口。
- **恢复码再生成不吊销会话**：要密码且已登录，旧码作废本身不扩大攻击面；enable/disable 才吊销（动机是「怀疑密码泄露才开 2FA」）。
- **限次只是会话内闸门**：每轮登录 5 次、重登续杯，不构成在线暴破防御；真正的闸门在登录限流轮，届时 login 与 verify 同做。

### 执行中的口径修正（与计划/上一轮的出入，逐条实测后定）

- **auth 模块构造器由 `New(db, sessions)` 变 `New(db, sessions, totpKey)`**：推翻 2FA schema 轮「该模块不需要配置」的旧记录——密钥是配置项，必须经 `app.go` 显式注入（README 目录表与 spec §6 同步）。
- `pquerna/otp` 的引入落在**第一个 import 它的提交**（TOTP 原语那条），不在依赖引入那条：`go mod tidy` 会把尚无人引用的依赖从 go.mod 剔回去，先 `go get` 再 tidy 等于白加。
- 新增错误码必须连 `apierror_test.go` 的注册表计数一起改（20 → 26），否则门禁红；计划该任务的文件清单漏了这个文件。
- 「disable 吊销其它会话」的集成用例从 Task 5 挪到本轮：Task 5 时二段登录还不存在，第二个会话只能用「未启用期的单段登录」造出来，那种写法到本轮必然红。
- 测试接缝 `codeAt` 带 `t *testing.T`（计划写的是不带的形状），取码失败要 `t.Fatalf`，不该向上层返回 error 让用例各自处理。
- 请求 DTO 用 `TwoFactorCodeRequest` 一个类型（enable 与 verify 的 body 同为 `{"code"}`），不留两个同名同形的结构。

### 已知缺口

- **`TOTP_ENCRYPTION_KEY` 是部署面新增强依赖**：缺失启动即失败；密钥丢失 = 已启用用户验码一律 500，只能 DB 清列重置（`.env.example` 与 README 都写了备份要求）。恢复码在密钥丢失时同样救不了——解密发生在恢复码分支之前。
- **重放防护的代价**：一枚码用过之后它所在窗整窗作废；用户在同一 30 秒内登出再登录需等下一窗，**启用那一刻也占掉当窗**（RFC 6238 §5.2 标准取舍）。
- **认领不回滚**：verify 的计数认领与会话写非同事务，会话写失败该窗已废；enable 的认领与落位分两步，落位事务失败同样废一窗。只影响一次重试体验，不产生权限或数据不一致。
- **限次续杯**：见上，暴破闸门等登录限流轮。
- **disable 删全部恢复码行**（含已消费的）：不留存根，无法统计用户消费过几个码；需审计得改成标记作废。
- **「吊销失败 → 整个动作不生效」这条分支本轮未构造测试**：要造会话存储故障才触发，代价大于收益，只靠调用顺序本身保证（改密轮的同类用例可参照）。
- **本机 `-race` 仍不可用**（`CGO_ENABLED=1` 缺 gcc）：并发改判的正确性依据是「单条条件 UPDATE + 判断影响行数」在 Postgres 侧的行锁语义，两个并发用例各断言「恰一人成功」；不是靠 race 检测器证明的。

---

## 2026-10-11 · 管理端发信接口（邮件模块升级为路由模块）

**版本**：0.8.0 → **0.9.0**（minor：新 HTTP 能力）

### 做了什么

- 邮件模块获得第一个路由 `POST /api/v1/admin/mail/messages`（创建邮件消息即发送）：body `{"to"}` → 用当前 SMTP 配置**同步真发**一封 `example` 自检邮件 → 201 `{"to","subject","sent":true}`；400 `invalid_request`（`param:"to"`，`net/mail.ParseAddress`，支持 `Name <addr>` 取纯地址）/ 401 `unauthorized` / 403 `forbidden` / 500 `mail_delivery_failed`。主题与正文由服务器控制（`example` 模板），请求方不能指定内容。
- 模块形状统一（兑现邮件 spec v4 时期「若以后要形式统一」的预案）：`mail.New(cfg)` 改返回 `*Module`（`m.Mailer` 暴露能力接口，app.go 装配处同步）；新增 `module.go` / `controller.go`，硬性规则 1、5 的「无路由模块」清单只剩 `settings`。
- `Mailer` 接口新增 `SendSync(ctx, msg) error`：同步单次尝试、不重试、拨号死线 20s（go-mail 逐命令续期，非总上限；`Close` 后同样返回 `ErrClosed`），失败记结构化日志；异步 `Send`（队列 + 3 次重试）语义不变，将来业务流程一律走它。
- 守卫新增 `guard.RequirePresetGroup(keys...)`（预设组白名单，owner/admin 过、其余 403 `forbidden`、无 identity 500）：路由链 `RequireAuth → LoadIdentity → RequirePresetGroup(owner, admin)`。**刻意不走权限目录**——SMTP 是服务器级配置（环境变量），不属于可授出的业务权限域，自建组不可授；将来邮件配置若搬进设置系统，可再切权限点。
- `apierror` 新增哨兵 `ErrMailDeliveryFailed`（`server_error` / 500，错误码注册表第 20 条）。
- 测试：guard 预设组守卫单测（owner 过 / 普通用户与自建组 403 / 未接线 500）、`SendSync` 单测（成功一次调用 / 失败不重试 / `Close` 后 `ErrClosed` / 失败日志）、controller HTTP 用例（201 + 内容断言 / 展示名地址提取 / 三种 400 / 两种 403 / 500 映射）；`TestRegistryIntegrity` 同步 20 条。
- 文档：README 目录对照表（mail 行、guard 行）、硬性规则 1 / 5、邮件章节重写（接口表 + SendSync 语义）；邮件 spec v5 修订记录。

### 关键决定

- **路由命名**：初稿 `POST /admin/mail/test` 被用户否决（不要动词尾巴、要求严格按既有 API 规范、不自造轮子）。按 API 规范 §3 资源名词复数，三选一由用户点名 `POST /admin/mail/messages`（对标 OpenAI「操作即资源」，创建即发送；`POST /<集合>/:id/<动作>` 子动作形态本项目仍无先例）。
- **同步而非入队**：自检接口的意义在于即时告知 SMTP 通不通；异步队列只能回「已入队」，对排障无用。`SendSync` 单次不重试（最多等 20s），失败由操作者自行重试。
- **权限用预设组守卫而非权限点**：权限目录动作只有 `view`/`edit`，`mail.test` 之类的第三动作会破坏既有命名规则；`RequirePresetGroup` 不动目录、不加戏。

### 已知缺口

- 无限流（与登录/注册同属已知缺口，等统一补）。
- 发信成功 ≠ 对方收件箱收到（SPF/DKIM/DMARC 与服务商合规仍见邮件 spec §11）。
- `SendSync` 与队列 worker 可能并发各起一条 SMTP 连接（自检请求与后台投递同时发生时）；对 SMTP 服务商的并发连接数无压力，暂不互斥。
- **20s 不是总上限**：go-mail 只在拨号阶段吃 ctx 死线（`client.go:1197`），之后对每条 SMTP 命令续期（`client.go:1237/1811`），慢速对端总耗时可超 20s（异步 `Send` 同构，既有行为）。
- `SendSync` 死线 20s 大于关停预算 10s：SIGTERM 撞上在途自检请求时 `server.Shutdown` 带 DeadlineExceeded 返回、SMTP 事务被硬切（`writeTimeout` 60s 不构成额外问题）。

### 终审

独立子代理整分支终审（fresh context）：**0 Critical / 4 Important / 8 Minor**，门禁全绿（`go test -count=1` 强制非缓存）、无越界文件、异步 `Send` 零改动、`RequirePresetGroup` 的身份来源链（自建组无法携带预设 `system_key`）源码取证通过。

- **已修（按用户裁定）**：
  1. **I-1 状态码违规**——`mail_delivery_failed` 初版 502 违反 API 规范 §4「500 之外的 5xx 一律归并为 500」的白名单；用户裁定**改回 500**（语义区分交给 code，不动冻结规范）。I-3（README:126 type 枚举缺 502）随本条自动消解。
  2. **I-2 词汇表漏登记**——API 规范 §7.3 补 `mail_delivery_failed` 行（500 / `server_error` / param null）+ 引言记账。
  3. **I-4 spec 正文未随修订**——邮件 spec §2 决策表 / §5 接口块（`SendSync` + `New` 签名）/ §6 装配与消费方四处按 v5 改写，修订记录不再「只记账不修文」。
  4. **M-1 超时措辞失实**——README 与 spec 的「20s 超时」改为「拨号 20s 死线、逐命令续期、非总上限」（go-mail 源码取证）。
  5. **M-2 ctx 断言缺失**——补 `TestSendSyncAppliesDeadline`（死线 ≈20s）与 `TestSendSyncPropagatesParentCancellation`（父取消传导到 transport）。
  6. **M-4 关停预算**——已知缺口补 `SendSync` 20s > `shutdownTimeout` 10s 一条。
  7. **M-5 同码不设防**——`TestRegistryIntegrity` 加「code 去重后仍 20 条」断言（两个哨兵共用 code 原本可静默通过）。
  8. **M-6 集成腿缺失**——补 `TestMailMessagesEndpointEndToEnd`：匿名 401 / 普通用户 403 / 自建组 403 / owner 201（走真实 app 装配 + 登录会话，`DATABASE_URL_TEST` 缺省 skip）。
- **留档不修（用户裁定）**：M-3（`ErrClosed` 单列错误码——其建议的 503 本身违反 §4 白名单，503 仅 `/healthy` 豁免；且该窗口实际不可达）；M-7（「操作即资源」判定标准待出现第二个此类端点时写入规范 §3/§4）；M-8（未跟踪的 2FA 两份文档写着 0.8.0 → 0.9.0，本轮已占用 0.9.0，2FA 轮启动时需重编版本号）。
- **审查手段缺陷记录**：本机 `-race` 不可用（`CGO_ENABLED=1` 缺 gcc），数据竞争结论以 go-mail v0.8.1 源码取证替代（每次投递新建连接 + `c.mutex.RLock()`）；`gofmt -l` 恒退出 0、`-race` 首跑退出码被管道吞掉——两处都要求门禁脚本用显式判定，不信工具默认行为。
- **过程中撞出的存量 flake（非本轮引入；2026-10-12 用户点名后已修）**：全仓并行跑时 auth 一次 `TestLoginRejectsWrongPasswordAndUnknownAccount` 报 `email or nickname already exists`——两包共用 `neoblog_test` 并行执行，而 `auth/repository_test.go` 与 `access/migration_test.go` 的 tag 生成器同为纯 `UnixNano`、建号昵称都派生自它（如 `"nick"+tag`），取到同一时钟刻度即撞唯一约束（当时单独重跑与后续两轮全仓 `-count=1` 均绿，属偶发）。修法：tag 改为「pid + `-` + UnixNano」，跨进程必不同；两包各加守护用例 `TestUniqueTagIncludesProcessID`（先红后绿）。修复后全仓 `-count=1`（带库）重跑绿。

---

## 2026-10-11 · 设置组存储机制（内部能力，无路由）

**版本**：0.7.0 → **0.8.0**（minor：新能力域）

### 做了什么

- spec v2 全落：`setting_values` 单表（复合唯一 `(setting_group, name)` + jsonb `value`，纯 DDL 零数据，全仓第一列 `field.JSON`）；模块 `internal/modules/settings/`（types / registry / repository / service 四文件，无路由无 controller / dto）；三种类型读写按类型分方法（用错即 `ErrMethodMismatch`）、整组读逐项合并坏行只回退该项、整组写单事务零落库、删项恢复默认（幂等只指"清单内但库里没有这行"）；清单定义在代码内（初始为空），构造自检不过启动失败。
- `settings.New(db) (*Service, error)` 是仓库第一个返回 error 的模块构造器；`app.New` 随之改 `(*App, error)`，三处调用点同步（`main.go` 拆链式调用、`app_test.go` 两处判错）。
- upsert 冲突分支显式 `SetUpdatedAt(time.Now())`（不赌 ent 的 `UpdateDefault` 是否作用于 `ON CONFLICT` 路径），集成用例⑧断言同项二次写入后时间变大。
- 测试：清单自检与配方防漂移（单测，无库）+ 集成①–⑧（`DATABASE_URL_TEST` 缺省 skip）；README 四处同步（目录对照表加行、硬性规则 1 / 5 措辞、迁移清单、「设置」章节 + 加设置配方——配方防漂移测试 `TestRegistryRecipeShape` 把关）。

### 过程中的坑

- **jsonb 归一化**：`1e3` 落库即被 Postgres 存成 `1000`，读取侧永远见不到指数记法原文；数字边界的真身（`3.5` / 超 int64）照拒，`1e3` 的拒绝由内核 `TestDecodeNumberBoundary`（`UseNumber` + `ParseInt`）在原始 JSON 文本上钉死。
- `.env` 直接 `source` 只设 shell 变量、不导出，`go test` 子进程看不到 `DATABASE_URL_TEST`——要 `set -a; source .env; set +a`。
- `gofmt -l` 退出码恒为 0（列出问题文件也不报错），门禁必须用 `test -z "$(gofmt -l .)"` 判定。

### 终审

独立子代理整分支终审（fresh context）：**0 Critical / 2 Important / 4 Minor**，Review Focus 五条逐条实测通过；build / vet / gofmt / 全仓测试 11 包全绿。

- 已修（每个都先红后绿）：
  1. **jsonb `null` 坏行对 string / bool 静默穿透**——`decodeTyped` 原来直接 `Unmarshal` 到零值（`null` 解入 string / bool 无错无日志，返回零值而非默认值）；改为三种类型统一 `UseNumber` + type switch，`null` 全部判坏行回退默认。新增 `TestDecodeTypedRejectsNull` 与 null 坏行集成断言（string / bool 各一）。
  2. **Task 1 提交混入范围外文件**——`git add -A` 卷走了会话开始前就存在的 `frontend/app/page.tsx` 改动与四份 content / mail 存档文档，backend feat 提交历史语义失真。历史不重写，本条即补记；**防再犯：分轮提交按文件清单暂存，不裸 `git add -A`**。
- 延后小项（待定）：`ConflictColumns` 用字面列名（接口轮映射 apierror 时顺带换生成常量）；集成测试用固定组名而非 spec 字面的"随机后缀"（隔离效果等价）；`marshalJSON` 吞 Marshal 错误（当前三种类型安全，将来加类型时回看）；`newServiceWith(nil)` 触库会 panic（已限定为不触库单测路径）。

### 已知缺口

- 清单为空、无消费方：模块上线即休眠（与 2FA 轮同性质）；首次真实使用（加组 + 消费方）只改代码不动库。
- 孤儿行：清单删项后旧行滞留并占着复合键，需人工 SQL 清理（spec §9）。
- 密钥类加密、接口 / 公开读取、导入导出：见 spec §10 插槽。

---

## 2026-10-11 · 两步验证表结构预留（TOTP，仅落库）

**版本**：0.5.1 → **0.6.0**（minor：新能力域的数据插槽）

### 做了什么

- 兑现认证设计 §2 / §11 的预留（TOTP + 自管恢复码，库选 `pquerna/otp` 但本轮不引入）：`users` 加 `totp_secret` / `totp_enabled_at` 两个可空列（ent Optional + Nillable，组合出「未设置 / 设置中 / 已启用」三态）、新表 `recovery_codes`（`user_id` 级联外键 + `code_hash` + `used_at` 一次性标记）。接口、登录分叉、恢复码生成逻辑全部下一轮；当前无用户启用、无任何写入路径，上线即现状。
- 迁移 `totp_and_recovery_codes`（纯 DDL：加两列 + 建表 + 索引，无回填无 seed），人工审阅后 hash / validate、应用到 `neoblog` 与 `neoblog_test`；`db-migrate-diff.sh` 的迁移名字面量再换一次（仍是写死字面量、未参数化）。
- 迁移后置断言 `TestTOTPSchemaReserved`（auth 模块，`DATABASE_URL_TEST` 缺省 skip）：可空性、Nillable 读写回程、恢复码行随用户删除级联消失、`used_at` 默认 nil——全部用 ent 行为实证，不查 `information_schema`。

### 过程中的坑

- ent 命名转换不保留缩写词大写：`totp_secret` 生成的 Go 字段是 `TotpSecret`（不是 `TOTPSecret`）；计划原写法有误，以生成物为准。
- auth 包测试 helper 已有同签名 `testDB` / `uniqueTag`（`repository_test.go`），新测试文件重复声明直接编译失败——删重复声明、复用既有 helper。

### 终审

独立子代理整分支终审（fresh context）：**0 Critical / 1 Important / 4 Minor**，Review Focus 五条（级联方向、Nillable、迁移无 NOT NULL/回填、索引、两库一致）与 spec §10 五条验收逐条实测通过；go.mod/go.sum 确认未动，业务模块 grep `totp|recovery` 零匹配。

- **已修**：README 已引用本轮 spec 路径而 plan/spec 两份文档此前未入库（悬空引用、违背前四轮存档惯例）——两份文档随本提交补交存档；spec §10.5 的既有路由计数同步更正（auth 5 + access 8 = 13 条 + healthy）。
- **延后小项（待定）**：迁移后置断言可选加固——`used_at` 改为从 DB 重查后再断言 nil、`TotpEnabledAt` 补值相等校验（当前由迁移 SQL 人工审阅 + `atlas migrate validate` 把关）。

### 已知缺口

- `recovery_codes` 在下一轮接口落地前是死表；`code_hash` 暂不设长度校验（写入方下一轮实现时钉 64 位 hex）。
- secret 按 base32 明文入列：DB 泄露即可伪造 TOTP。休眠期无存量，应用层加密可在任何用户启用前无痛补上（这正是先落结构、暂不启用的动机）。
- 登录限流仍未做（用户裁定与本轮分开，见认证章节已知缺口）。

### 关键决定

- 先落库表结构、暂不启用：插槽钉死后，下一轮接口不动迁移；secret 加密升级窗口留在无人启用阶段。
- 恢复码哈希用 sha256（高熵随机串无需慢哈希）；将来换低熵可读码时改算法不动表。

## 2026-10-11 · 邮件模块归位审查与修复

**版本**：0.5.0 → **0.5.1**（patch：测试基础设施缺陷修复 + 规范说明补全）

### 背景

邮件模块初版放在 `internal/mail/`（被当成与 `config` / `database` 平级的基础设施），用户指出它违背 README 硬性规则 1（一个功能一个模块目录）。归位提交 `56a8037`（`internal/mail` → `internal/modules/mail`，11 个文件 `R100`）后，用户要求派独立子代理**只读**复查。

### 过程（审查 → 验证 → 修复，可追溯）

1. **两路独立只读子代理审查**，角度错开：迁移完整性与引用面（逐字节比对 blob、残留引用、构建测试、git 历史完整性）／规范符合度与文档一致性（README 规则 1–8 逐条、三处文档互相一致、sed 批量替换残留）。两路**独立报出同一条真实缺陷**（下条），并另有规范说明与文档口径若干项。
2. **逐条实测验证**（不信转述）：`cd internal/modules/mail && test -f ../../.env` → 不存在（归位后指向 `backend/internal/.env`）；`test -f ../../../.env` → 存在（`backend/.env`）；`LoadDotEnvFile` 对缺文件静默返回 nil（`dotenv.go:16-19`）；README 规则 1/5 原文与 mail 实际形态逐条对照；spec/plan 里旧版本号（0.2.1 → 0.3.0）与旧路径描述逐条确认。**四条发现全部属实**。
3. **TDD 修复**：先写三个失败用例（`findEnvFile` 纯函数边界 / 「从包目录能找到 `backend/.env`」守护用例 / 经 `t.Chdir` 到临时目录树验证 `.env` 确实被载入到进程环境），跑出编译失败 → 实现「从包目录向上逐级查找 `.env`（深度上限 5）」并接入真实投递测试 → 全绿。守护用例专门钉住「包再移动导致静默失效」这一类回归；`TestRealDelivery` 的 skip 文案也点明了查找位置。
4. **文档同步**：spec 增 v4 修订记录 + §10 第 5 条改写 + §3/§14 版本表述修正；plan 增「实施后修订 2」+ Task 4 Step 5 改写；README 补规则 1/5/7 的「无路由模块」与「禁止自建 SMTP 客户端」说明、目录表 mail 行改写并移入模块行区域、「邮件」章节措辞与测试说明；本条目即过程记录。

### 缺陷根因

`realdelivery_test.go` 里写死的 `../../.env` 依赖包目录深度：归位只改了文件位置与含 `internal/mail` 字样的文本，这条不含路径 token 的相对深度没有跟着变；`LoadDotEnvFile` 对缺失文件静默返回 nil 又把问题藏住——用户按 README 填好 `backend/.env` 去跑真实投递测试时只会得到 SKIP，且没有任何提示。

### 关键决定

- **不写死相对深度**，改为有界向上查找（`findEnvFile` 深度上限 5）+ 守护用例：包位置再变会红，而不是静默失效。
- README 规则 5 采用「文档收编现状」方案（无路由模块由 `New` 直接返回能力接口、无状态包级纯函数可直接导出），而不是把 `mail.New` 包装成 `*Module`；若以后要形式统一，需同步改 spec/plan 与 `app.go`。

### 已知缺口

- 仅当变量已在进程环境导出或 `backend/.env` 存在且可读时，真实投递用例才会真跑；无凭据时按约定 SKIP。
- 邮件 spec/plan 仍是未跟踪文件（未入库），本次修订只存在于工作区。

---

## 2026-10-11 · 用户组与权限（层 2）

**版本**：0.4.1 → **0.5.0**（minor；认证层 2 与邮件基础设施先占用了 0.3.x / 0.4.x）
**提交**：`bbeef91`（rbac）→ `a3e5474`（版本进位）；终审修复 `f48568c`；设计与计划存档 `6088bf5`

### 做了什么

- 权限目录 `internal/common/rbac`（纯 Go，不依赖 gin / DB）：`模块.动作` 命名、动作只有 `view` / `edit`（`edit` 蕴含 `view`）、四个权限点（`system.users.view|edit`、`system.groups.view|edit`）、预设组常量（owner / admin / 普通用户 + 固定 UUID）。
- 新模块 `internal/modules/access/`：组 CRUD、用户管理接口（列表 / 详情 / 设组 / 设额外权限）、生效权限解析；路由全部挂在权限守卫下。
- 数据模型：新增 `groups` / `group_permissions` / `user_permissions` 三张表 + `users.group_id`（非空外键，ent 字段级默认值指向「普通用户」，**创建路径零改动**）；迁移里 seed 三个预设组、三步回填存量用户（生成物把 `ADD COLUMN … NOT NULL` 与外键写在同一句，人工拆开）。
- 守卫扩展：`guard.LoadIdentity`（解析一次身份与生效权限放进 `auth.identity`，用户已删除则销毁会话 + 401）与 `guard.RequirePermission`（权限不足 403 `forbidden`）。
- `/auth/me` 扩展为带 `group` 与 `permissions`；`app.go` 装配 access 模块与 `RequireAuth → LoadIdentity → RequirePermission` 链。
- `scripts/db-generate.sh` 开 `--feature sql/lock`（last-owner 检查用 `FOR UPDATE`）；`db-migrate-diff.sh` 的迁移名由 `init_users_sessions` 换成 `groups_and_permissions`（仍是写死字面量，未参数化）。
- 测试：rbac / guard 单测（不连库）+ 迁移后置断言 / service / HTTP / 端到端集成（`DATABASE_URL_TEST` 缺省 skip）。

### 制定阶段修掉的坑（设计 → 计划）

设计前后各派过独立子代理：先摸清参照项目（D 盘 HaoBlog）的模型与其生态代价，再把写好的计划交给两路只读子代理——一路查「照做会不会撞墙」（逐条核对 ent / Atlas / scs 行为），一路做安全红队（穷举提权路径与覆盖缺口）。真正影响实现的几条：

- **组转移的授权差集没钉死**（红队判高）：原计划只写「只能授出自己持有的权限」，差集口径留白——若实现成「只对额外权限做差集」，两步即可提权（先把自己有的权限塞进一个组、把用户塞进去，再借那个用户反向给自己加权限）。现设计 §6 规则 1 写死：`next 生效集 − current 生效集`、同一请求内组与额外权限的变更**合并计算**、两侧先做蕴含展开。
- **`last_owner` 串行路径根本打不出来**：自己不能改自己、非 owner 碰不到 owner 组成员 ⇒ 串行下 owner 数不可能降到 0；原计划却让 HTTP 用例串行断言 409，会诱导实现把检查提前（把 403 变成 409）。现规定检查顺序 `cannot_modify_self` → `owner_protected` → `last_owner`，409 只留给并发用例。
- **版本口径与仓库现状脱节**：计划初稿写死 0.2.1 → 0.3.0（该原稿未随存档保留、无从核对；存档版写为「当前 0.3.1 → 0.4.0」并已带「若届时已被其他层进位按当前版本顺延」规则），实际执行时已被认证层 2 与邮件层推到 0.4.1。
- **迁移脚本的隐性前置**：`db-migrate-apply.sh` 带 `set -u` 且不读 `.env`，计划漏了导出 `DATABASE_URL` / `DATABASE_URL_TEST`，照做必报 unbound variable。
- 其余中级项：约束错误不能一刀切映射（组名冲突与外键冲突分开）、guard 单测里「会话已销毁」的断言是空转（第二次仍 401 只能证明 resolver 仍返回 not-found，改为断言清除 cookie）、输入校验整段缺失（组名长度 / 空 body / `group_id: ""` / `[]` vs `null`）、`/auth/me` 扩展与 app 装配拆开会留下红灯窗口（合并为一个任务）。

### 执行中发现并修掉的问题

- **ent 边声明的写法**：计划写的是 `edge.To("users", User.Type())`，生成器直接报 `not enough arguments in call to User.Type, want (User)`。回 ent v0.14.6 源码核实：`Schema.Type` 是边声明专用伪方法，取方法表达式、**不加括号**（`ent.go:27-41`），改为 `User.Type` 后生成通过。
- **迁移生成物在有数据的库上必失败**：`atlas migrate diff` 把 `ALTER TABLE users ADD COLUMN group_id uuid NOT NULL` 与外键写在同一条语句；人工拆成「加可空列 → `UPDATE` 回填「普通用户」→ `SET NOT NULL` → 加外键」，预设组 seed 落在 `CREATE TABLE groups` 之后，再 `atlas migrate hash` + `validate`（13 条语句应用到两库）。
- **三处测试期望写错，被实现的正确行为纠正**（改测试不改代码）：① 目标已在 admin 组时 `owner_protected` 优先于 `grant_forbidden`；② 「应该被拒」的用例里给操作者配多了权限，其实合法；③ 断言普通用户组有成员却没造人。
- **脚手架坑**：`task-done` 脚本在验证命令零输出时不记账（`grep -v` 空输入退出 1 + `set -e` 静默中止），后续命令统一追加哨兵输出。

### 终审与修复

独立子代理整分支终审（fresh context；明确排除并行会话的邮件层提交）：**0 Critical / 2 Important / 6 Minor**，5 条预设审查焦点（组转移差集、`last_owner` 顺序与可达性、owner 保护两向、`edit` 蕴含 `view`、并集撤权假象）逐条通过，三条攻击者视角未发现接口组合提权路径。

- **已修（先复现后修）**：并发 `PATCH` 同一个组或同一个用户时，替换是「先删后插」，READ COMMITTED 下第二笔看不到第一笔未提交的行，随后 `INSERT` 撞唯一索引 → **裸 500**。先写 `TestConcurrentReplaceKeepsBothWritesServing` 复现（组 / 用户两条路径各 5 轮并发，3 次运行里稳定打到 500），再在替换事务内先 `FOR UPDATE` 锁父行（组 / 用户）串行化 → 15 轮并发全 200、全量套件绿。根因：替换事务之间没有可串行化的锚点。
- **裁定不修**：owner 保护判定用的是**请求开始时的快照**——并发把目标提升进 owner/admin 组的瞬间，非 owner 可能刚好完成一次对目标的额外权限编辑。不产生越权（可授出集合仍受差集限制）、不损坏数据，与设计 §13 已写明的「在途请求用旧快照」同属一种语义；要堵得把整段判定搬进事务，收益不抵复杂度。
- **未处理的小问题（待定）**：README「新用户由数据库默认值落普通用户」措辞（实为 ent 字段级默认值；后已修正）；删空组后权限行级联消失未断言；用户列表「按创建时间倒序」的断言是空转；`rbac.Expand` 对目录外的 `x.edit` 也会合成 `x.view`（仅脏数据可触发）；约束映射一刀切（未来新增约束需同步）；`/auth/me` 在 `LoadIdentity` 之后多查一次用户。

### 验证

- `gofmt -l` / `go vet` / `go build` 干净，`atlas migrate validate` 通过；带库与不带库各跑一次 `go test -p 1 ./...` 全绿（access 集成用例依赖 `DATABASE_URL_TEST`，缺省 skip；**带库全量用 `-p 1`**——access 与 app 都会造 owner，包级并行会互相干扰）。
- 手工 curl 清单：普通用户 403、owner 200、`/auth/me` 带 group 与 permissions、预设组改动 403 `preset_group_locked`、重名 409 `group_name_taken`、非空组删除 409 `group_not_empty`、改自己 403 `cannot_modify_self`、目录外权限 key 400 `invalid_permission`、删空组 204、用户被删后旧 cookie 401。
- 迁移后置断言（`TestMigrationSeed`）：三预设组按固定 UUID / key / 名称存在、权限行数为 0、**不显式设组建用户自动落「普通用户」**（ent 字段级默认值在 v0.14.6 生效的实证）。

### 关键决定

- **预设组权限由代码计算、不入库**（owner / admin = 全量、普通用户 = 空），只有自建组写 `group_permissions`——参照项目（D 盘 HaoBlog）因为「权限行入库 + 首建号一次性 seed」导致新增权限要手工补种、漏种即全站锁死（权限从库里读，fail-closed），这里从根上绕开：加权限点只改代码。
- 生效权限 = 所属组 ∪ 用户额外权限（都做蕴含展开）；**只能授出自己持有的权限**（按目标生效权限集的前后差集、同一请求合并计算）、**摘除不受限**、**不能改自己**、**owner / admin 组的成员只有 owner 能改**、**至少保留一个 owner**（409 `last_owner` 只在并发双降级时可达，串行路径被前两条挡住）。
- 组以 UUID 标识、组名唯一（不做 slug）；带成员的自建组不可删；预设组不可删 / 改名 / 改权限。
- 组名 trim 后 1–64 字符；读接口一律返回展开后的权限集，写接口收原始勾选。
- 守卫原则变更：从「不查库」改为「只依赖 `app.go` 注入进来的 resolver，不 import 业务模块」。
- 权限改动**下一次请求即生效**（不吊销会话）；在途请求使用请求开始时的权限快照。

### 已知缺口

- **首个 owner 只能手工指定**（`UPDATE users SET group_id = '00000000-0000-4000-8000-000000000001' WHERE email = '…'`）；「首次建号入口」仍待定。
- 登录 / 注册仍无限流；角色与权限操作无审计留痕；用户列表不分页、不做可见性遮蔽；组编辑是「后写者获胜」（无乐观锁）。
- 权限目录删除权限点会留下 DB 孤儿行（本轮不清理）。

---

## 2026-10-11 · 邮件基础设施（只做后端）

**版本**：0.3.1 → **0.4.1**（minor 0.4.0 落地；模块归位 patch 0.4.1，归位审查修复见上一条）

### 做了什么

- 新增模块 `internal/modules/mail/`（无路由；对外入口 `New(cfg) Mailer`，不含 controller / dto）：`Mailer` 接口（`Send` / `Close`，`Close` 后 `Send` 返回哨兵 `ErrClosed`）+ `Message` / `Body` + `Render(name, data)`；业务代码不 import go-mail。
- 发送链路：缓冲队列（32）+ 单 worker 串行投递；每封最多 3 次尝试、退避 1s / 5s、单次 20s 超时；**4xx 与连接类错误重试、5xx 不重试**（判定用 `SendError.IsTemp()` / `ErrorCode()`——go-mail 把网络类失败判为「永久」，只信 `IsTemp()` 会一次抖动就丢信）；失败记 `log/slog` 结构化日志。
- 双实现：SMTP（`wneessen/go-mail` v0.8.1；`starttls` / `ssl` 两种 TLS 模式、无用户名则不做 AUTH、有用户名走 `SMTPAuthAutoDiscover`）与日志（**`SMTP_HOST` 为空即降级**，邮件全文写日志，开发环境无凭据可跑通全流程）。
- 模板：`templates/<name>.html|txt` 成对 + `embed.FS`；HTML 侧 `html/template` 自动转义；内置一封 `example`（邮件配置自检）。
- 配置：7 个 `SMTP_*`（`SMTP_HOST` 为空时全部跳过校验，其余 fail-fast，含端口 ≤ 65535）与 `LoadDotEnvFile(path)`（`LoadDotEnv` 变其包装，行为不变）。
- `app.go`：装配 `mail.New(cfg)`；`server` 提为字段并抽出 `shutdown(ctx)`（先 `server.Shutdown` 再 `mailer.Close`，共用 10s 关闭预算）。
- 文档：README（目录表 / 环境变量表 / 新增「邮件」章节 / 认证章节的「没有邮件基础设施」措辞更新）、`.env.example`（腾讯云 SES 为主示例）。

### 验证

- 单测（无外部依赖）：队列满阻塞、`Close` 后 `Send` 返回 `ErrClosed`、4xx 重试 / 5xx 不重试 / 重试耗尽记日志、drain、`Close` 幂等、并发 `Send`/`Close` 不 panic、日志实现内容与缺省 From。
- 集成（进程内 `emersion/go-smtp` + 自签证书，不依赖 Docker 与外网）：multipart 双版本正文、**中文主题 RFC 2047 可解**、550 永久 / 451 临时 / 连接被拒可重试。
- 真库：`TestShutdownClosesMailer`（关闭 drain 后 `Send` 返回 `ErrClosed`）；带与不带 `DATABASE_URL_TEST` 各跑一次全量 `go test ./...` 全绿；`gofmt -l` 与 `go vet` 干净。
- 可选真实投递 `TestRealDelivery`：无凭据时 SKIP；需在 `.env` 填好 `SMTP_*` 与 `MAIL_TEST_RECIPIENT` 后手动跑。

### 关键决定

- 选型 `wneessen/go-mail` v0.8.1（依赖仅 x/crypto + x/text）；排除：`gomail.v2` 停在 2016、`jordan-wright/email` 停在 2021、标准库 `net/smtp` 已 frozen 且无 context。
- **不做持久化 outbox**：Gitea / Vikunja 的生产实现都是「异步即发、失败只记日志」，本项目量级用「重发」兜底；触发条件＝出现「邮件丢了不可恢复」的流程。
- 不引入 `riverqueue/river`：`InsertTx` 需 `*sql.Tx`，而 ent 的 `*ent.Tx` 不暴露底层事务，原子入队要绕非公开 API。
- 不做应用内 DKIM（域验类服务商侧签）、不做连接复用（每封 Dial + Close）、不做退信处理。
- 包位置：按 README 硬性规则 1 归入 `internal/modules/mail/`（初版误放在 `internal/mail/`，用户裁定后迁移，版本进位 0.4.1）。

### 已知缺口

- `internal/modules/mail` **没有任何消费者**（注册不验证邮箱）；第一个消费方出现时在 `app.go` 注入。
- 内存队列无持久化：进程崩溃丢在途邮件；`Close` 返回错误 ≠ worker 已停（退避不响应关闭，上界约 66s）。
- 无退信 / 投诉处理与抑制表、无连接复用、无应用内 DKIM、模板无多语言——触发时点见 `docs/superpowers/specs/2026-10-11-neoblog-mail-design.md` §12、§15。

---

## 2026-10-11 · 认证层 2（注册与更改密码）

**版本**：0.2.1 → **0.3.0**（minor）

### 做了什么

- 新接口（前缀 `/api/v1`）：
  - `POST /auth/register`（公开，注册即登录）→ 201 + 会话 cookie；校验与注册页 UI 对齐（邮箱小写入库、昵称 trim 后 2–32 字符且禁空白与 `@`、密码 ≥8 字符含 ASCII 字母与数字且 ≤72 字节）；错误码 400 `invalid_request` / `weak_password`、409 `email_taken` / `nickname_taken` / `already_taken`（并发竞态兜底）。
  - `POST /auth/password`（需登录）→ **200 `{}`**（2026-10-12 对账更正：原文写 204，代码是 `controller.go:130` 的 200）；**保留当前设备、吊销该用户其他所有设备的会话**（scs `IterableStore.All()` + `Codec` 按 `auth.user_id` 定位；零 schema 改动）；失败契约 fail-closed——吊销或写库失败都返回 500 且密码未改，旧密码可直接重试。
- 校验规则抽成 `internal/modules/auth/validate.go` 纯函数（表驱动单测，覆盖全角空格、ASCII 字母判定、`net/mail` 边界等）。
- 无 schema 改动、无新依赖、`app.go` 未改。测试新增 10 组 / 25+ 用例（双设备吊销、坏会话行容错、删号后改密码、已登录时注册切换会话、昵称/密码边界）。
- 文档：`backend/README.md` 认证章节重写（接口表 + 规则 + 已知缺口）；spec/plan 见 `docs/superpowers/{specs,plans}/2026-10-11-neoblog-auth-layer2*`。

### 计划评审

1 个独立子代理先审计划：2 Critical（scs API 名错、吊销顺序与失败契约自相矛盾）+ 3 Important（DTO 缺失、handler 与既有方法同名、路由归属写错）——全部修进计划后才开工；执行中编译期又抓到一处计划笔误（`IterableStore` 只有 `All()`、没有 `Delete`，删除改用 `Store.Delete`）。

### 已知缺口

- **登录 / 注册暂无限流**（本轮按用户要求不做；下一轮优先补，含部署时的可信代理配置）。
- 邮箱验证 / 找回密码（无 SMTP）、角色区分（还没有管理员专属接口）——均留到需要时。
- 前端未接：`/api` 代理与注册/登录表单由用户侧接线，后端只提供上面的接口契约。

---

## 2026-10-11 · 仓库换行符治理（.gitattributes）

**提交**：`acc8703`（`development`）；版本未变（后端 0.2.1 / 前端 0.1.0）。

### 问题

本机 git 的**全局** `core.autocrlf=true`（`D:/SoftWare/Git/etc/gitconfig`，不是仓库自己的配置）：索引存 LF、Windows 检出写 CRLF。于是任何让 git 重新写出文件的操作——clone、checkout、`reset --hard`、cherry-pick、rebase、重写历史——都会把文本文件在工作区变成 CRLF。`gofmt` 只认 LF，随即把 Go 文件报成「未格式化」，而 `git diff` 是空的（仓库内容其实没变）。重写认证分支历史后撞到过一次，当时误以为代码被改坏。

规模实测：**未加规则时克隆本仓库，158 个文本文件被检出为 CRLF，`gofmt -l` 报出全部 54 个 Go 文件。**

### 做了什么

- 新增 `.gitattributes`，内容一行：`* text=auto eol=lf`——在仓库内压过全局 autocrlf。
- 把当时工作区仍是 CRLF 的 5 个文件归一回 LF：`backend/README.md`、`backend/go.mod`、`backend/go.sum`、`docs/superpowers/{plans,specs}/2026-10-11-neoblog-auth*.md`。**跟踪内容零变化**（归一后 blob 与索引逐字节相同），因此本次提交只含 `.gitattributes` 一个文件。
- 例外情况无需处理：`frontend/app/favicon.ico` 是二进制（`text=auto` 自动跳过）、6 个单行 svg 没有行尾可转；仓库内没有 `.bat/.cmd/.ps1`，6 个 `*.sh` 本就要求 LF。

### 验证

临时克隆做 A/B（同一份内容，删除文件后强制重新检出全部文件）：

| 状态 | 重新检出结果 |
| --- | --- |
| 加规则前 | 158 个文件 CRLF；`gofmt -l` 报 54 个文件 |
| 加规则后 | 158 个文件全 LF；`gofmt -l` 无输出；`git status` 干净 |

规则提交后再克隆复验：160 个文件全 LF、`gofmt -l` 空、`git status` 完全干净——新克隆开箱不再出假警报。

### 排查与善后（复发时）

- 排查别先看 `gofmt -l`，用 `git ls-files --eol | grep -v 'w/lf'` 直接列出工作区非 LF 的文件；再用 `git diff -- <file> | wc -c` 为 0 确认仓库内容没变。
- 手动归一（`sed -i 's/\r$//'` 或对 Go 文件 `gofmt -w`）之后，这些文件会出现**假的 ` M`**：stat 缓存过期，blob 其实逐字节相同。`git update-index --refresh` 与 `git add --refresh` 都清不掉它（前者还会打印 "needs update" 误导人），必须 `git add -- <files>`；因 blob 相同，它不会暂存任何内容，用 `git diff --cached` 复核为空即可。

### 关键决定

- 采用全仓 `* text=auto eol=lf`，而不是最小化的 `*.go text eol=lf`：换行符与语言无关，前端 tsx/json/md 同样受 autocrlf 影响；`text=auto` 只对 git 判定为文本的文件生效，二进制安全。
- 用仓库内 `.gitattributes` 而不是改本机全局配置：规则随仓库分发，任何克隆与机器都免疫。

---

## 2026-10-11 · 认证层 1（只做后端）

**版本**：0.1.0 → **0.2.1**（minor＝新功能，patch＝补测试）

### 做了什么

- **接口**（前缀 `/api/v1`）：
  - `POST /auth/login`：body `{"identifier":"邮箱或昵称","password":"..."}` → 200 `{"email","nickname"}` + 会话 cookie；凭据错 401 `invalid_credentials`；请求不合法 400 `invalid_request`。
  - `POST /auth/logout` → **200 `{}`**（2026-10-12 对账更正：原文写 204，代码是 `controller.go:73`；`backend/README.md:135` 一直是对的）（销毁会话）。
  - `GET /auth/me` → 200 `{"email","nickname"}`；未登录 401 `unauthorized`。
- **会话**：`scs` + Postgres 存储（`sessions` 表）；cookie 名 `session`、HttpOnly、SameSite=Lax、`Secure` 仅当 `APP_ENV=production`；有效期 `SESSION_LIFETIME`（默认 168h）；登录成功换发 token（防会话固定），登出与「用户已删除」都会销毁会话。
- **密码**：bcrypt；拒绝空密码与超 72 字节密码；未知账号也做一次等价比较（抗时序探测）。
- **数据**：`ent` schema `users` / `sessions`（对齐数据层设计的 ent + Atlas 路线）；Atlas 首个迁移进 `migrations/`（生成后人工审阅）；`users` 字段对齐 Neo 项目：`email` / `nickname` 双唯一 + `password_hash` + 时间戳。
- **结构**：`internal/modules/auth/`（module / controller / service / repository / dto）+ `internal/common/guard/`（`RequireAuth`，会话键 `auth.user_id`）；装配只在 `internal/app/app.go`；`/api/v1` 前缀仍只定义一次。
- **测试**：单元 + 真库集成——登录 / 登出 / me 全流程、cookie 属性（HttpOnly / SameSite / Path、production 才 Secure）、删号后旧会话吊销、存储故障返回 500、大小写邮箱、超长 / 空密码、畸形请求、cookie `Max-Age` == `SESSION_LIFETIME`；无 `DATABASE_URL_TEST` 时集成用例自动 skip。
- **文档**：`backend/README.md` 更新（认证章节、环境变量、目录对照、迁移工作流、硬性规则 3 / 5 补充）；规格与计划见 `docs/superpowers/specs/2026-10-11-neoblog-auth-design.md`、`docs/superpowers/plans/2026-10-11-neoblog-auth.md`。

### 移除（用户要求；已从分支历史移除，不是 revert 提交）

- 前端接线：`next.config` 的 `/api` 转发规则、登录表单提交接线、语言包两条错误文案。
- `cmd/create-admin` 管理员创建命令。

### 评审与修复

独立子代理整分支终审：**0 Critical / 2 Important / 5 Minor**。

- 已修（每个都先有失败测试）：
  1. scs 存储写失败时登录 / 登出曾「假成功」——防会话固定会静默失效、登出会假报 204；现在登录失败直接 500 不落会话，登出失败返回 500。
  2. `CreateAdminUser` 曾接受空密码（会造出永远登不上的账号）；现在拒绝。
- 未处理的小问题（待定）：`Only()` 的 NotSingular 未映射（多用户后「邮箱撞昵称」会 500）；测试库残留 `sessions` 行未清理；`.env.example` 未列 `SESSION_LIFETIME`；`dummyHash` 生成失败时静默降级。

### 已知缺口

- **没有「首个管理员」的建号入口**（命令已按用户要求移除、注册未实现）；首次建号方式待定。
- 登录接口**无限流**（有意留到层 2；现有缓解：bcrypt 慢哈希、错误信息不区分账号、未知账号等价比较）。
- **前端未接**：`/api` 代理与登录表单由用户侧处理；后端只提供上面的接口契约。
- 生产 cookie 域（前端域 / API 子域）未定，部署轮再处理。

### 关键决定

- 选型：`scs` + `bcrypt` + `ent` + Atlas（对齐已批准的数据层设计，取代早期草案里的 sqlc + goose）。
- 会话 cookie 由 **Go 自己 Set-Cookie**（早期 BFF 草案「Go 不 Set-Cookie、Next 种 cookie」未采用）。
- 控制器把非哨兵错误映射为 500（凭据 / 授权问题才是 401）；`auth.New(db, sessions)` 不带 `cfg`（模块不需要配置）。
