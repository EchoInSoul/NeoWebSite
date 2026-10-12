# NeoBlog 控制台访问控制设计（前端鉴权轮）

日期：2026-10-12
范围：`frontend/`（Next 16.4 App Router，`cacheComponents: true` + `partialPrefetching: true`）。后端零改动——`GET /api/v1/auth/me`、`POST /api/v1/auth/logout` 已存在且已由 `guard.RequireAuth` 保护。
状态：**已落地（2026-10-12，代码与文档均按用户要求暂未提交）**——plan `docs/superpowers/plans/2026-10-12-neoblog-console-auth.md` 的 Task 1–6 与 Task 8 已完成，Task 7（D 态取证）由用户决定推后。设计经用户确认：**不带 `?next=` 回跳**、**已登录不准停在 `/signin` / `/signup`**、**本轮一起做登出**。

> **v1.1（2026-10-12）修订**：两个独立只读审查代理逐条证伪后重写。改动最大的五处——① 权威判定从「非 200 一律当未登录」改为三态（后端故障必须抛错，否则后端一挂全员静默锁死）；② 状态表从四态补到八态并新增「脏 cookie 不自愈」；③ 重定向码从写死的 302 改为 307（`NextResponse.redirect` 默认值，源码 `frontend/node_modules/next/dist/server/web/spec-extension/response.js:95`）；④ §1 的现状描述更正（前端接线已由 `16e210a`–`22d272c` 六个提交收进分支，**上一轮的纯 CSR 门控已被撤销**）；⑤ 引 `authentication-with-cache-components.md:156` 与 `:1127` 指出本设计两处**故意**偏离官方建议并给理由。§4 的 403 分支、`cache:"no-store"`、`app/layout.tsx:34` 行号等 6 处事实错误一并删除。下列 file:line 均于 2026-10-12 在当前工作区核对过。

## 1. 意图与背景

`/console` 目前任何人直接访问都能拿到控制台外壳（顶栏 + 侧边栏 + 空内容区），没有门禁。本轮要求：**只有登录用户能进控制台**，暂不做角色区分。

现状登记（避免下一轮误判范围）：

- 前端登录/注册接线**已进分支**：`16e210a`（`/api` 转发 + `apiRequest` + Toaster）、`9e57fe2`（page 只留 metadata + 拆 `-view.tsx`）、`e4010f7`、`08b7848`、`aeecb9b`、`22d272c`。现有落点：`frontend/lib/api.ts`（浏览器侧请求底座）、`hooks/use-signin.ts` / `hooks/use-signup.ts`（逻辑）、`app/signin|signup/*-view.tsx` + `*-form.tsx`、`components/auth/field-error.tsx`。
- **但纯 CSR 门控已被撤销**：`hooks/use-is-client.ts` 不存在，`signin-view.tsx` / `signup-view.tsx` 里没有 `useSyncExternalStore` 门控，两个 `-view` 是普通 Client Component（服务端照常渲染 HTML 再水合）。本设计不依赖 CSR/SSR 之争，也不擅自恢复该门控；若要恢复另开一轮。
- 旧口径未更正：`docs/superpowers/plans/2026-10-11-neoblog-auth.md:13` 原文「按用户要求收窄为后端 only——Task 7 与 Task 9 不纳入本分支……这两节作废」，与现状不符。本文 §9 收尾步骤登记更正。
- 后端会话 cookie：名 `session`、HttpOnly、SameSite=Lax、`Path=/`、`Secure` 仅 `APP_ENV=production`，`SESSION_LIFETIME` 默认 168h（`backend/internal/app/app.go:107-117`、`backend/internal/config/config.go:70`）。cookie 值是 scs 的随机 token，**前端解不出内容**，真假只能问后端。
- `POST /auth/register` 与 `POST /auth/login` **都写会话**（`service.go:79-82`、`:105-115`）：注册即登录；注册成功 **201**（`controller.go:65`），不是 200。新注册用户落预设组「普通用户」、`permissions: []`（`internal/common/rbac/rbac.go`），`/auth/me` 对它返回 200——所以本轮门禁不会把新注册的人挡在控制台外。
- **二段验证 pending 态**：`POST /auth/login` 对已开启 TOTP 的账号 `RenewToken` 后**只写** `auth.pending_2fa_user_id`、**不写** `auth.user_id`，返回 401 `two_factor_required`（`service.go:109-113`），且该 401 响应**仍然下发会话 cookie**。前端上一轮拍板不接 `POST /auth/2fa/verify`，只显示一句「当前登录入口暂不支持」。于是存在第三种状态：**cookie 有效但不是登录态**。本设计的全部风险都围绕它。
- 控制台里只有一个用户菜单：`ConsoleTopBanner` 不含菜单，菜单在 `ConsoleSidebarFooter` → `ConsoleUserMenu`；`ConsoleUserMenu` 的 `displayName` prop 已存在但上游没传值（`components/console/console-sidebar.tsx` 渲染 `<ConsoleSidebarFooter />` 未带 props），右上角现在是空文本。
- `POST /auth/logout` 返回 **200 `{}`**（`controller.go:68-74`）。口径冲突登记：`backend/README.md:135` 与代码一致；**只有** `docs/DEVLOG.md:399` 写成 204。同类陈旧还有 `DEVLOG.md:335`、`specs/2026-10-11-neoblog-auth-layer2-design.md:48/85/100`、`specs/2026-10-11-neoblog-auth-design.md:86`（`password`/`logout` 都写 204，代码是 200 `{}`，`controller.go:73/130`）。

## 2. 已定决策

| 项 | 决定 |
| --- | --- |
| 守卫层次 | **两层**：`proxy.ts` 乐观短路（只看 cookie 存在）+ 服务端权威校验（问 `/auth/me`）。依据 `authentication.md:1026-1033`：proxy 每路由都跑（含 prefetch），**只读 cookie、禁止数据库校验**；`authentication.md:1121`：proxy **不得作为唯一防线**，校验要贴着数据源 |
| proxy 职责 | 只做一件事：无 `session` cookie 且目标是 `/console*` → 重定向 `/signin`；否则 `next()`。**不判定「已登录」，不做反向跳，不 import `lib/` 或 `hooks/` 下任何模块**（`proxy.md:19-21`） |
| matcher 收窄与官方建议的冲突 | 官方 Tips（`authentication.md:1126-1127`）建议鉴权时 proxy 跑**全部路由**；本项目**故意**收窄为 `["/console/:path*"]`。理由：反向跳不能放 proxy（§5 的 D/G 态会死环），而把 `/api` 纳入 matcher 会让门禁在转发前就重定向 API 请求。后人勿当遗漏改宽 |
| 「不冲突」的顺序依据 | proxy 执行在第 3 步、**先于** `beforeFiles` rewrites（`proxy.md:240-244`），所以 `/api/:path*` 转发与 `/console/:path*` 门禁互不相干——不是「 matcher 不含 /api 才不会打架」 |
| 权威判定的唯一来源 | `getAuthenticatedUser()` 一个函数。`/console` 的准入与 `/signin` `/signup` 的离开都问它，两处判定同源，不可能互相推诿成环 |
| 反向跳（已登录访问 `/signin`、`/signup`） | 做，落在**服务端 page 层**（不能放 `-view.tsx`：那是 Client Component，`instant` 禁用于 Client Component（`instant.md:24`），也不该在客户端判会话）。官方示例把这条写在 proxy（`authentication.md:1061-1069`），本项目**故意不照抄** |
| 后端故障 ≠ 未登录 | 权威判定是**三态**：401 才是「未登录」；5xx / 404 / 405 / 网络失败 / 响应体解析不出一律**抛错**（不 redirect、不降级）。否则后端一挂，`/signin` 也拿不到判定，用户被静默锁死且看不到原因。「非 200 一律 null」是 v1.0 的错误，已废 |
| 后端不可用时的界面 | 抛错即交给 Next 默认错误页（项目内暂无 `error.tsx`，本轮不新建，避免造兜底 UI）。**不做「后端挂了先放控制台外壳」的降级** |
| 未登录访问 `/console` 的去向 | 一律 `/signin`，**不带 `?next=`**；登录成功固定 `router.push("/console")` |
| 注册成功的去向 | 改为 `/console`。用户选了「已登录不准停在登录/注册页」，而注册即登录——仍 `push("/signin")` 会立刻被 page 层的权威判定弹走，白跳一次。成功 toast 保留 |
| 登出 | 一起做，落在 `ConsoleUserMenu`：`POST /api/v1/auth/logout` 成功 → `router.replace("/signin")`（`use-router.md:45`），失败 `toast.error` 并留在原地。逻辑抽 `hooks/use-signout.ts`，菜单只渲染——延续「UI 只管 UI，逻辑只做逻辑」；菜单里现有的主题/语言 handler 不是先例，它们不碰网络 |
| 会话数据缓存 | 不缓存。会话可被后端即时吊销（改密码会吊销其它全部会话，`service.go` 的 `revokeOtherSessions` 路径），缓存会让「吊销后仍进得去」变成 bug。cacheComponents 下 fetch 默认就不缓存（`migrating-to-cache-components.md:564`：不加 `use cache` 一律不缓存），因此 **v1.0 写的 `cache: "no-store"` 属多余，删掉** |
| 服务端请求层 | **不复用 `lib/api.ts`**（浏览器侧，相对路径 + `credentials: "same-origin"`）。落地偏离一处：v1.1 原计划给该模块加 `import "server-only"`，实测本机 **未安装 `server-only` 包**（`node_modules/server-only` 不存在），为一个防护性 import 引新依赖不值当——该模块 import 了 `next/headers`，被 Client Component 误引时 Next 自身就会构建失败，这层保护是白送的 |
| 后端地址字面量 | 新增 `lib/backend-url.ts` 导出 `backendUrl`，`next.config.ts` 与 `lib/auth-session.ts` 同引，消掉重复字面量 |
| 角色/权限 | **本轮不做**。`/auth/me` 的 `group` / `permissions` / `two_factor_enabled` 本轮不消费，窄 DTO 只取 `{email, nickname}` |
| 2FA verify | 本轮不接（上一轮拍板，非漏实现）。本设计只保证 pending 态**不被误当登录态** |
| 数据模型 / 后端改动 | 零（§8 另挂一条后端修复建议，不属本轮） |

## 3. 范围

新增：`frontend/proxy.ts`、`frontend/lib/backend-url.ts`、`frontend/lib/auth-session.ts`、`frontend/hooks/use-signout.ts`

改动：`app/console/layout.tsx`、`app/signin/page.tsx`、`app/signup/page.tsx`、`components/console/console-sidebar.tsx`、`components/console/console-user-menu.tsx`、`hooks/use-signup.ts`、`next.config.ts`、`messages/zh-CN.json`、`messages/en.json`

不碰：`app/page.tsx`（首页公开）、`app/ui-check/`、两个 `-view.tsx`、`lib/api.ts`、`hooks/use-signin.ts` 的成功分支、后端全部。

## 4. 实现要点

**`proxy.ts`**

```ts
export default function proxy(request: NextRequest) {
  if (request.cookies.get("session")?.value) return NextResponse.next();
  return NextResponse.redirect(new URL("/signin", request.nextUrl));
}
export const config = { matcher: ["/console/:path*"] };
```

`/console/:path*` 里 `*` 是零或多，故 `/console` 本身也匹配。不指定状态码，接受 `NextResponse.redirect` 的默认 **307**（GET 场景与 302 行为等价）。不调后端、不引共享模块。

**`lib/auth-session.ts`（服务端专用，`import "server-only"`）**

`getAuthenticatedUser(): Promise<{ email: string; nickname: string } | null>`

1. `cookies()` 取 `session`；无 → 直接 `null`（省一次内网调用）。
2. 有 → 按官方示范带 `Cookie` 头请求后端（`data-security.md:36-44`）：`fetch(`${backendUrl}/api/v1/auth/me`, { headers: { cookie: `session=${value}` } })`。
3. 分类：
   - **200** → 解析 `{email, nickname}` 返回；
   - **401** → 返回 `null`（含三种成因：无会话 / pending 会话 / 用户已被删，`guard.go:14-16`、`permission.go:37-42`。`two_factor_required` 只出现在 login，`/auth/me` 一律 `unauthorized`）；
   - **其它（5xx / 404 / 405 / 网络失败 / JSON 解析失败）→ `throw`**。规则按状态码而非 code 分叉是有意的：故障侧不该被当成身份侧。
4. 不重试、不缓存、不抛业务错误类型（本轮调用方只区分「有用户 / 没用户 / 判不了」三种结果，故障走 error boundary）。

**`app/console/layout.tsx`**

- `const user = await getAuthenticatedUser(); if (!user) redirect("/signin");`
- `<ConsoleSidebar displayName={user.nickname} />`（`ConsoleSidebarFooter` 已有同名 prop，透传即可）。
- 依赖 root layout 的 `export const instant = false`（`app/layout.tsx:27`）免于静态壳校验；`redirect()` 在 cacheComponents 下无 Suspense 要求。

**`app/signin/page.tsx` / `app/signup/page.tsx`**

- 保留现有 `generateMetadata`；默认导出里先 `if (await getAuthenticatedUser()) redirect("/console")`，再渲染 view。

**`hooks/use-signout.ts` + 菜单项**

- `signOut()`：`apiRequest("/api/v1/auth/logout", { method: "POST" })` → 成功 `router.replace("/signin")`；catch → `toast.error(t("signOutFailed"))`。`isSigningOut` 期间禁用该项。
- 菜单位置：`访问前台` 之后、`主题色` 之前；图标 `LogOut`。

**`hooks/use-signup.ts`**

- 成功分支 `push("/signin")` → `push("/console")`；toast 文案不动。

**`lib/backend-url.ts` + `next.config.ts`**

- 常量单一来源，`next.config.ts` 里的 rewrites 目标改引它。

## 5. 不变量与状态机

- **I1** 进入 `/console` 渲染的充要条件是 `getAuthenticatedUser()` 返回非 null。proxy 只会在「完全没有 cookie」时挡人，它**不授予**任何访问权。
- **I2** 从 `/signin` / `/signup` 跳往 `/console` 的唯一触发是同一个函数返回非 null。
- **I3** 同一 cookie 在同一会话状态下，`getAuthenticatedUser()` 给出同一结论。
- 由 I2/I3：判定为 null 时 `/signin` 是终点；判定为真时 `/signin → /console` 后 `/console` 判定同真，不会弹回。**不存在由两处互相推诿造成的环**。
- **I4**（v1.1 新增）后端故障**不产生重定向**，只产生错误页。故故障期间不会出现「反复跳转」，也不会出现「看起来没登录」的假象。

已知状态（v1.0 称「穷举」，不准确；下列为已核实的八态，未覆盖的后端新状态出现时需回来补表）：

| 状态 | 请求 `/console` | 请求 `/signin` | 终态 |
| --- | --- | --- | --- |
| A 无 cookie | proxy 307 → `/signin`；page 判 null → 停 | 渲染 | `/signin` |
| B 有效会话（未开 2FA） | proxy 放行；权威真 → 渲染 | 权威真 → 307 `/console` → 渲染 | `/console` |
| C 伪造 / 已过期 cookie | proxy 放行；`/auth/me` 401 → null → `/signin` → 停 | 停 | `/signin`（2 跳收敛） |
| D 2FA pending 会话（cookie 有效、无 `auth.user_id`） | proxy 放行；401 → `/signin` → 停 | **停**（这正是反向跳不能交给 proxy 的原因） | `/signin` |
| E 用户已被删（cookie 有效、会话行还在） | proxy 放行；`/auth/me` 401 且后端**下发清 cookie**（`permission.go:37-42`）→ `/signin` → 停，浏览器 cookie 消失，退化为 A | 停 | `/signin` |
| F 脏 cookie 的存续 | C/D 态的 cookie **不会自动清**：本项目未设 `IdleTimeout`，未改动的会话不回写 `Set-Cookie` → 脏 cookie 留到浏览器侧 `Max-Age` 到期，或被一次成功登录覆盖 | 同左 | 需登录覆盖（不自愈） |
| G `auth.user_id` 与 pending 键并存（脏态，可达） | 见 §7：`register` / 正常 `login` 只写 `auth.user_id`，**不清** pending（`service.go:79-82`、`:105-115`；`Remove` 只在 verify 成功的 `:378-379`）→ 同一会话两键并存，而 `POST /auth/2fa/verify` 是**公开路由**（`controller.go:23`）读 pending | 不破坏本节的无环结论（门禁不看 pending） | 属**后端不变量破损**，见 §7/§8 |
| H 改 `SESSION_LIFETIME` | 只影响新签发 cookie 的 `Max-Age`；存量 cookie 按已写入的过期时间走，改短不提前踢人 | 同左 | 无环风险 |

对照记录：若照官方示例把反向跳写进 proxy（只看 cookie 存在），D 态与 G 态下 `/signin ⇄ /console` 就是无限重定向。本设计刻意偏离该示例，理由记此，勿被后人改回去。

## 6. 测试与验收

前端**没有测试运行器**（`package.json` 无 `test`），本轮不引入；以下实测为准。curl 一律走 `http://127.0.0.1:3000`（经同源转发），用 cookie jar（`-c` / `-b`）。

> **取证纪律（新增）**：dev 下 Server Component 的 fetch 结果会跨 HMR 刷新复用，且该缓存**对 `cache: "no-store"` 同样生效**，只在导航或整页刷新时清（`serverComponentsHmrCache.md:9-11`）。因此下表所有涉及会话状态的用例，**必须以 curl 冷请求或整页刷新取证**，不许看 HMR 热更后的画面。

| # | 用例 | 期望 |
| --- | --- | --- |
| 1 | 无 cookie `curl -i /console` | **307**，`Location: /signin`；无 cookie 下发 |
| 2 | `POST /auth/register` 拿到 jar 后 `curl -i -b jar /console` | 注册响应是 **201**；`/console` 返回 200，不再重定向 |
| 3 | 同一 jar 请求 `/signin` | 307 → `/console` |
| 4 | 假 cookie（`-b "session=garbage"`）请求 `/console`，再请求 `/signin` | 前者 307 → `/signin`；后者 200 停住。**顺带自证 F 态**：记录这两个响应里有没有 `Set-Cookie` |
| 5 | D 态：先造一个开 2FA 的账号（`POST /auth/2fa/setup` + `/enable`，验证码按返回 secret 现算 TOTP），登录得 401 `two_factor_required` 并拿到 jar，再请求 `/console` 与 `/signin` | 前者 307 → `/signin`，后者 200 停住。此步不可省——它正是偏离官方示例的根据 |
| 6 | 后端故障（把 `BACKEND_URL` 指向空闲端口起一次 dev，或停后端进程）访问 `/console` | **不是**跳 `/signin`，而是错误页 / 500。验 I4 |
| 7 | 浏览器登录 | 地址栏 `/console`，右上角显示 nickname，`/auth/me` 200 |
| 8 | 点「退出登录」 | 请求 200；响应 `Set-Cookie: session=; Max-Age=0`；界面落 `/signin`；再进 `/console` 被挡 |
| 9 | 注册成功 | toast「注册成功，请登录」出现（MutationObserver 取证），地址栏 `/console` |
| 10 | 转发与静态资源回归 | `curl -i -X POST /api/v1/auth/register`（空体）仍 400 `invalid_request`；`/_next/static/*` 不受 proxy 影响 |
| 11 | 门禁 | `npx tsc --noEmit`、`npm run lint`、`npm run format:check` 全过 |

## 7. 已知缺口与风险

- **G 态（后端不变量破损）**：会话可同时持 `auth.user_id` 与 `auth.pending_2fa_user_id`，公开的 `/auth/2fa/verify` 会按 pending 键完成登录，从而把身份切到那个 2FA 账号。触发前提是攻击者已掌握对方口令（pending 只在密码校验通过后写入）**且**能拿到对方的动态码/恢复码，所以不是远程可利用；但它与 `specs/2026-10-11-neoblog-2fa-design.md` 里「同一会话不可能既已登录又在等二段」的承诺相悖。本轮前端不改后端，登记到 §8/§9。
- **`/signin`、`/signup`、`/console` 每次渲染各多一次到后端的会话查询**，而全站**零限流**（`backend/README.md:152`、layer2 spec:108 已登记），本轮把读放大带到登录页本身。生产 `BACKEND_URL` 必须指内网且不对浏览器暴露。
- **故意违反一条官方建议**：`authentication-with-cache-components.md:156` 明确劝「别把 session 读放 layout 顶层，会连 `children` 一起拖住」。本设计**故意**放在顶层——门禁必须先于 children 渲染，否则未登录时控制台子树会先出画面。代价是整段多等一次内网 RTT（开发环境同机约 1ms）。后人勿按文档把它下移进 Suspense。
- **依赖 `instant = false` 的位置**：`instant.md:87-89` 说明 root layout 上的 `false` 关掉**整站**静态壳校验，同时官方建议把它尽量下移。一旦被下移或删除，layout 顶层的 `cookies()` + 外部 fetch 会立刻变成构建错。此依赖写进本 spec 正文，改动 root layout 时必须回来核。
- **prefetch 与 307 的交互无文档覆盖**：App Shell 在 prefetch 命中重定向后如何缓存，随包文档没说；且 `partialPrefetching.md:55` 提到读 cookie 的路由按 session 缓存壳。`partialPrefetching: true` 下需在验收时观察 `/console` 的 prefetch 行为，不能只信 dev 表现（dev 不 prefetch）。
- **C/D 态的脏 cookie 不自愈**（§5 的 F 行）：脏 cookie 会留到浏览器 `Max-Age` 到期或被成功登录覆盖；本轮不主动清 cookie（前端读不到 HttpOnly 值，proxy 也无权改写后端会话）。
- **Server Function 覆盖盲区**：`proxy.md:249-251`——Server Function 不是独立路由，matcher 收窄会静默失去对其的覆盖，重构挪路由也一样。本项目当前没有 Server Function（表单全走客户端 fetch），若将来引入，必须在其内部独立鉴权，不得依赖 proxy。
- 后端故障时用户看到的是错误页而非「请重试」提示（项目暂无 `error.tsx`，本轮不新建）。
- `docs/DEVLOG.md:399` 的 logout 204 与代码不符（`backend/README.md:135` 是对的）；`password` 的同类陈旧见 §1 列表。

## 8. 插槽（本轮不定，定它的时点）

| 项 | 触发时点 |
| --- | --- |
| 401 `unauthorized` 在界面内接口调用时统一跳登录（落点 `lib/api.ts` 单一出口） | 控制台开始消费真实数据那轮 |
| **客户端改按 `error.code` 分叉而非状态码**（本轮 `getAuthenticatedUser` 按状态码分叉是有意的） | 接 2FA 那轮；依据 `specs/2026-10-11-neoblog-api-conventions-design.md:399` 的客户端义务 |
| **后端修复 G 态**：`register` / 正常 `login` 成功路径显式清 pending 键，并把不变量写回 2FA spec | 后端下一轮（不依赖前端功能） |
| 角色/权限门禁（`permissions` 的前端表达、菜单按权显隐） | 做角色区分那轮 |
| `?next=` 原位续作 | 出现多个受保护子页且需回原页时 |
| 恢复纯 CSR 门控 | 你决定要不要恢复（上一轮的它已被撤销） |
| `error.tsx` / 后端不可用的用户可读提示 | 需要可读错误页那轮 |
| 前端版本号进位口径（`package.json` 仍是 0.1.0） | 你定前端版本规则那轮 |
| 首个 owner 账号来源 | 管理端/建号那轮 |

## 9. 下一步

1. 你批这份 spec（或改它）。
2. 我按它出逐任务 plan（`docs/superpowers/plans/2026-10-12-neoblog-console-auth.md`：每任务落点、验证命令、提交划分），plan 批准后再动代码。
3. 落地收尾：`docs/DEVLOG.md` 补一条；更正 `plans/2026-10-11-neoblog-auth.md:13` 与相关 spec 里「前端未接 / Task 9 作废」的口径；把 `DEVLOG.md:335/399` 的 204 改成与代码一致的 200 `{}`。
4. 后端修复建议（G 态）单列一条，等你要动后端时一起做。
