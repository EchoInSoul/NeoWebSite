# 控制台访问控制 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.（注：本机未安装这两个 skill——`C:\Users\Multu\.qoder` 下只有 brainstorming / writing-plans / systematic-debugging / using-superpowers。执行者按本文步骤逐条落地即可，不要因为找不到 skill 就停下。）

**Goal:** 给 `frontend/app/console/` 加上「只有登录用户能进」的门禁，并把登出与登录注册页的去向一并接顺。

**Architecture:** 两层。`frontend/proxy.ts` 只看 `session` cookie 存不存在做乐观短路（无 cookie 就把 `/console*` 弹到 `/signin`，它不授予任何访问权）；真正的准入判定集中在 `frontend/lib/auth-session.ts` 的 `getAuthenticatedUser()`，它带 cookie 问后端 `GET /api/v1/auth/me`，`/console` 布局与 `/signin` `/signup` 的反向跳共用这一个函数，因此不会互相推诿成环。

**Tech Stack:** Next 16.4 App Router（`cacheComponents: true` + `partialPrefetching: true`）、next-intl（cookie 定语言）、sonner toast、Go/Gin 后端（已存在，本轮零改动）。

**Spec:** `docs/superpowers/specs/2026-10-12-neoblog-console-auth-design.md`（v1.1；本计划的每条决策都从它论证，执行时两份一起读）

## Global Constraints

> **落地状态（2026-10-12）**：Task 1–6 与 Task 8 已完成并逐条取证（各任务的验证步骤实测值见 Task 2 Step 7 与各步 Expected）；**按用户要求暂未提交任何改动**；Task 7 推后。下文步骤保留原写法，供回滚或复跑时对照。

- 前端**没有测试运行器**（`package.json` 无 `test` 脚本），本轮不引入。每个验证步骤必须是「命令 + 期望输出」，命令一律在 `F:/protect/NeoBlog/frontend` 下跑。
- 代码**一律不写注释**（含一行「为什么」）。
- 两份语言包 `messages/zh-CN.json` 与 `messages/en.json` 的键位必须逐一对齐，新增键同步两份。
- 重定向状态码按 **307** 记（`NextResponse.redirect` 的默认值）；验收断言里不许写 302。
- 会话 cookie 名字面量 `session` 与后端 `backend/internal/app/app.go:111`（`sm.Cookie.Name = "session"`）耦合；本轮出现在 `proxy.ts` 与 `lib/auth-session.ts` 两处，改名要一起改。
- **状态类用例一律冷取证**：dev 下 Server Component 的 fetch 会跨 HMR 刷新复用，对 `cache:"no-store"` 同样生效，只在导航/整页刷新时清（`node_modules/next/dist/docs/01-app/03-api-reference/05-config/01-next-config-js/serverComponentsHmrCache.md:9-11`）。用 `curl` 冷请求，或浏览器整页刷新，别信热更画面。
- 不碰：`lib/api.ts`、`app/ui-check/`、`app/page.tsx`、两个 `-view.tsx`、后端全部。**不恢复纯 CSR 门控**（spec §8 插槽，等用户定）。
- 提交只 `git add` 实际改动文件，禁止 `git add -A`；提交信息风格照 `git log`（`feat(frontend): ...` / `docs: ...`）。
- 后端 dev 在 `:5732`、Next dev 在 `:3000`、Postgres 在 `:5432`，**由用户常驻运行**：不要重启它们；Task 2 的故障取证另起一个自己的 Next 实例并在取证后只杀自己起的那个。

## Review Focus

规格里写了但没有任何一个任务的测试会顺手覆盖、且最容易在真实使用咬人的五类，按可能性排序。每条后面标注它落在哪个任务。

1. **后端故障被当成未登录**：`/auth/me` 返回 5xx / 404 / 405 / 连不上时，期望**不是**「弹回 `/signin` 看起来像没登录」——否则后端一挂，已登录用户被静默锁死且看不到原因。**v1.2（2026-10-13）改了呈现**：故障仍抛错，但由三个路由自己 `catch`，`/console` 照常出外壳并弹一条 toast、`/signin` `/signup` 照常出表单；判定层三态不变，鉴别用例也不变（见 Task 2 Step 7 的订正）。（Task 2 Step 6/7）
2. **pending 会话**（cookie 有效但会话里没有 `auth.user_id`）访问 `/console` 与 `/signin`：期望两跳收敛到 `/signin` 并停住，**不出现 `/signin ⇄ /console` 循环**。**本轮无实测覆盖**（Task 7 已由用户决定推后，见下文）；当前只由 spec §5 的 I2/I3 与代码结构保证，接 2FA 那轮必须补测。
3. **取证被 HMR 缓存骗了**：登出或塞假 cookie 后，用热更新画面判断会得出「还像登录态」的错误结论；期望每条状态断言都出自冷请求或整页刷新。（Task 4 Step 5、Task 6 Step 6）
4. **matcher 收窄的覆盖盲区**：`/console` 下**新增子路由**天然被 `/console/:path*` 覆盖，但若将来引入 Server Function（POST 到同一路由），matcher 不覆盖它就静默失去门禁；期望知道「布局层的权威校验才是防线」。（Task 3 Step 5；spec §7 已登记）
5. **脏 cookie 不自愈**：C/D 态下 `session` cookie 不会被前端清掉（本项目未设 `IdleTimeout`，未改动的会话不回写 `Set-Cookie`），要等浏览器侧 `Max-Age` 到期或被成功登录覆盖；期望知道这是设计后果，不当 bug 修。（Task 4 Step 6 记录实测到的 `Set-Cookie` 有无）

---

### Task 1: 后端地址常量单一来源

**Files:**
- Create: `frontend/lib/backend-url.ts`
- Modify: `frontend/next.config.ts`

**Interfaces:**
- Consumes: 无
- Produces: `export const backendUrl: string`（值 = `process.env.BACKEND_URL ?? "http://127.0.0.1:5732"`）

- [ ] **Step 1: 建 `lib/backend-url.ts`**

导出上面那一行常量，不引任何其它模块（`proxy.ts` 不许引它，Task 3 会说明原因）。

- [ ] **Step 2: `next.config.ts` 改引它**

把文件顶部的 `const backendUrl = process.env.BACKEND_URL ?? "http://127.0.0.1:5732";` 换成 `import { backendUrl } from "@/lib/backend-url";`，`rewrites()` 内其余不动。

- [ ] **Step 3: 类型检查**

Run: `npx tsc --noEmit`
Expected: 退出码 0、无输出。（注意：`@/` 路径别名在 `tsconfig.json` 里是 `./*`，Next 配置文件本身不走别名解析——若 tsc 通过但 dev 起不来，就退回相对路径 `./lib/backend-url`。）

- [ ] **Step 4: 同源转发回归（证明没改坏 rewrites）**

Run: `curl -s -o resp.txt -w "%{http_code}" -X POST -H "Content-Type: application/json" --data-binary "{}" http://127.0.0.1:3000/api/v1/auth/register && cat resp.txt`
Expected: `400` + `{"error":{"type":"invalid_request_error","code":"invalid_request","param":null}}`。若 Next dev 未热加载 `next.config.ts`，让用户自己重启他的 `:3000`（不要替他重启）。

- [ ] **Step 5: Commit**

```bash
git add frontend/lib/backend-url.ts frontend/next.config.ts
git commit -m "refactor(frontend): 后端地址常量单一来源"
```

---

### Task 2: 权威校验模块与控制台门禁

**Files:**
- Create: `frontend/lib/auth-session.ts`
- Modify: `frontend/app/console/layout.tsx`
- Modify: `frontend/components/console/console-sidebar.tsx`
- Test: 无运行器，用 curl 取证（Step 4–7）

**Interfaces:**
- Consumes: `backendUrl`（Task 1）；后端 `GET /api/v1/auth/me`（200 → `{email, nickname, group, permissions, two_factor_enabled}`；401 → `{"error":{"code":"unauthorized"}}`）
- Produces:
  - `export type SessionUser = { email: string; nickname: string }`
  - `export async function getAuthenticatedUser(): Promise<SessionUser | null>` — 200→`SessionUser`；**401→`null`**；5xx/404/405/连不上/体解析不出→**`throw new Error("session lookup failed")`**
  - `ConsoleSidebar({ displayName }: { displayName?: string })`

- [ ] **Step 1: 写 `lib/auth-session.ts`**

**不加** `import "server-only"`（落地时实测本机 `node_modules/server-only` 不存在，为一个防护 import 引新依赖不值当；该模块 import 了 `next/headers`，被 Client Component 误引时 Next 自身就会构建失败）。`getAuthenticatedUser()` 的三条分支按 Interfaces 块实现；从 `cookies()`（`next/headers`，需 `await`）取 `session`，**没有 cookie 直接返回 `null`，不发请求**；有则照下面的写法带 `Cookie` 头请求后端（参照 `node_modules/next/dist/docs/01-app/02-guides/data-security.md:36-44`）：

```ts
fetch(`${backendUrl}/api/v1/auth/me`, { headers: { cookie: `session=${value}` } })
```

**不设 `cache` 选项**（cacheComponents 下不加 `use cache` 本就不缓存，`migrating-to-cache-components.md:564`）。

- [ ] **Step 2: 布局层门禁 + nickname 透传**

`app/console/layout.tsx`：`const user = await getAuthenticatedUser(); if (!user) redirect("/signin");`（`redirect` 来自 `next/navigation`），然后 `<ConsoleSidebar displayName={user.nickname} />`。给 `components/console/console-sidebar.tsx` 加同名可选 prop 并透传给 `<ConsoleSidebarFooter displayName={displayName} />`（该组件已有此 prop，不用改）。

- [ ] **Step 3: 类型与格式**

Run: `npx tsc --noEmit && npx prettier --check lib/auth-session.ts app/console/layout.tsx components/console/console-sidebar.tsx`
Expected: 均退出码 0。若 prettier 报格式，跑 `npx prettier --write` 后复查。

- [ ] **Step 4: 冷请求取证——无 cookie 时由布局层挡（proxy 尚未存在）**

Run: `curl -s -o /dev/null -w "%{http_code} %{redirect_url}\n" http://127.0.0.1:3000/console`
Expected: `307 http://127.0.0.1:3000/signin`（此时还没有 `proxy.ts`，是 `redirect("/signin")` 生效；Task 3 之后期望值不变，改的是由谁产生）。

- [ ] **Step 5: 冷请求取证——假 cookie 也必须挡**

Run: `curl -s -o /dev/null -w "%{http_code} %{redirect_url}\n" -H "Cookie: session=garbage" http://127.0.0.1:3000/console`
Expected: `307 http://127.0.0.1:3000/signin`，且**无死循环**（`curl -s -L --max-redirs 3` 应停在 200 的 `/signin`，不是 `curl: (47) Maximum redirects`）。

- [ ] **Step 6: 真会话取证**

用 curl 注册一个临时账号拿 jar（注册即登录，成功码 **201**）：

```bash
printf '{"email":"gate-check@example.com","nickname":"gate_check","password":"abc12345"}' > /tmp/gate.json
curl -s -c /tmp/gate-jar -o /dev/null -w "%{http_code}\n" -X POST -H "Content-Type: application/json" --data-binary @/tmp/gate.json http://127.0.0.1:3000/api/v1/auth/register
curl -s -o /dev/null -w "%{http_code}\n" -b /tmp/gate-jar -L --max-redirs 3 http://127.0.0.1:3000/console
```
Expected: 第二行 `201`，第三行 `200`。浏览器打开 `http://localhost:3000/console`（整页刷新）应看到右上角显示 `gate_check`。

- [ ] **Step 7: 故障取证（Review Focus 1）——后端故障 ≠ 未登录**

用后台方式另起一个只属于自己的 Next 实例（本 harness 用 `run_in_background`），把后端指向空闲端口：

```bash
BACKEND_URL=http://127.0.0.1:59999 npx next dev -p 3100
```

等它起来（单次快速回查 `curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:3100/signin` 出 200 即就绪，别挂循环轮询），然后取证：

Run: `curl -s -o /dev/null -w "%{http_code} %{redirect_url}\n" http://127.0.0.1:3100/console`
#### Step 7 实际执行记录（2026-10-12，与原文不同）

用户选择「自己主动停后端」，因此**没有另起 3100 实例**（避开两个 dev 实例共用 `.next/dev` 缓存的风险）。`:5732` 停监听后在用户常驻的 `:3000` 上实测：

| 请求 | 实测 |
| --- | --- |
| `Cookie: session=whatever` → `/console` | **500**，响应体含 `session lookup failed`（不是 307） |
| 无 cookie → `/console` | `307 /signin`（proxy 不依赖后端） |
| 无 cookie → `/signin` | `200`（没 cookie 就不发那次请求） |

> **v1.2（2026-10-13）订正**：上表第一行是当时那次实测的存档，不作历史记录改写；**现状已变**。同一条件下现在实测 `/console` 带 cookie → **200** 且响应体含 `console-sidebar`，`/signin` `/signup` 带 cookie → **200** 表单照常（2026-10-13 于 `:5732` 无人监听状态 curl 复测）。鉴别点不变、只是期望值从 500 换成 200：故障若被错写成 `null`，`/console` 会 307 回 `/signin`，所以「200 且不出跳转」仍然证明 `throw` 分支没退化成未登录。不带 cookie → `/console` 仍是 307，proxy 那层没被这次改动波及。

Run（后端停机状态下）: `curl -s -o /dev/null -w "%{http_code} %{redirect_url}\n" -H "Cookie: session=whatever" http://127.0.0.1:3000/console`
Expected（v1.2 起）: `200` 且 `redirect_url` 为空。这条证明 `throw` 分支没被写成 `null`（写成 `null` 就会是 `307 /signin`）。访客页那两条自 v1.2 起不再具鉴别力——表单本来就返 200，判不出是「按故障放行」还是「当成未登录」。

- [ ] **Step 8: Commit**

```bash
git add frontend/lib/auth-session.ts frontend/app/console/layout.tsx frontend/components/console/console-sidebar.tsx
git commit -m "feat(frontend): 控制台服务端权威校验与门禁"
```

---

### Task 3: proxy 乐观短路

**Files:**
- Create: `frontend/proxy.ts`

**Interfaces:**
- Consumes: 无（**不得** import `lib/` 或 `hooks/` 下任何模块：`node_modules/next/dist/docs/01-app/03-api-reference/03-file-conventions/proxy.md:19-21`）
- Produces: `export default function proxy(request: NextRequest): NextResponse`、`export const config = { matcher: ["/console/:path*"] }`

- [ ] **Step 1: 写 `frontend/proxy.ts`**

有 `request.cookies.get("session")?.value` → `NextResponse.next()`；否则 `NextResponse.redirect(new URL("/signin", request.nextUrl))`。不指定状态码（接受默认 307）。文件放项目根（与 `app` 同级，`proxy.md:23`）；导出 default 或命名 `proxy` 均可（`proxy.md:58`）。

- [ ] **Step 2: 无 cookie 时由 proxy 产生重定向**

Run: `curl -s -o /dev/null -w "%{http_code} %{redirect_url}\n" http://127.0.0.1:3000/console`
Expected: `307 http://127.0.0.1:3000/signin`。断言与 Task 2 Step 4 相同，**含义变了**：现在短路来自 proxy。

- [ ] **Step 3: 有 cookie 时 proxy 不拦（它不授予访问权，也不否决）**

Run: `curl -s -o /dev/null -w "%{http_code}\n" -b /tmp/gate-jar http://127.0.0.1:3000/console`
Expected: `200`（jar 来自 Task 2 Step 6）。

- [ ] **Step 4: 转发与静态资源不被 proxy 干扰**

Run:
```bash
curl -s -o /dev/null -w "api=%{http_code}\n" -X POST -H "Content-Type: application/json" --data-binary "{}" http://127.0.0.1:3000/api/v1/auth/register
curl -s -o /dev/null -w "public=%{http_code}\n" http://127.0.0.1:3000/next.svg
curl -s -o /dev/null -w "favicon=%{http_code}\n" http://127.0.0.1:3000/favicon.ico
```
Expected: `api=400`、`public=200`、`favicon=200`。三者**都不能是 307**——出现 307 就说明 matcher 命中了非页面路由或 proxy 劫持了静态资源。

- [ ] **Step 5: 记录覆盖边界（Review Focus 4）**

确认 `/console` 的**任意子路由**（如 `/console/whatever`）也走 proxy：`curl -s -o /dev/null -w "%{http_code} %{redirect_url}\n" http://127.0.0.1:3000/console/whatever` → 期望 `307 .../signin`（该路径无对应 page，但 proxy 先于路由执行）。若得到 `404`，说明 matcher 没命中，改 matcher 为 `["/console", "/console/:path*"]` 复测。

- [ ] **Step 6: Commit**

```bash
git add frontend/proxy.ts
git commit -m "feat(frontend): console 路由 proxy 短路（只读 cookie）"
```

---

### Task 4: 登录/注册页反向跳

**Files:**
- Modify: `frontend/app/signin/page.tsx`
- Modify: `frontend/app/signup/page.tsx`

**Interfaces:**
- Consumes: `getAuthenticatedUser()`（Task 2）
- Produces: 无新接口；两个 page 的默认导出仍是服务端组件，`generateMetadata` 保持不动

- [ ] **Step 1: 两个 page 各加一行**

默认导出里，渲染 view 之前：`if (await getAuthenticatedUser()) redirect("/console");`。判定必须在服务端 page：`-view.tsx` 是 Client Component，`instant` 禁用于 Client Component（`instant.md:24`），且不能在浏览器上判会话。

- [ ] **Step 2: 类型与格式**

Run: `npx tsc --noEmit && npm run lint && npx prettier --check app`
Expected: 三项退出码 0。

- [ ] **Step 3: 无 cookie 时登录页照常 200**

Run: `curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:3000/signin`
Expected: `200`（Review Focus 3：这条与下一条都要冷请求）。

- [ ] **Step 4: 有真会话时登录页被弹走**

Run: `curl -s -o /dev/null -w "%{http_code} %{redirect_url}\n" -b /tmp/gate-jar http://127.0.0.1:3000/signin`
Expected: `307 http://127.0.0.1:3000/console`；注册页同理换 `/signup` 复测。

- [ ] **Step 5: 假 cookie 停在登录页（无环证明）**

Run: `curl -s -o /dev/null -w "%{http_code}\n" -L --max-redirs 5 -H "Cookie: session=garbage" http://127.0.0.1:3000/signin`
Expected: `200`。再对 `/console` 跑同一句 → 期望也是 `200`（路径 `/console → /signin → 停`，两跳收敛）。任何一条出现 `curl: (47)` 就是本设计最怕的环，回来重读 spec §5 的 I2/I3。

- [ ] **Step 6: 记录脏 cookie 是否自愈（Review Focus 5）**

Run: `curl -s -D - -o /dev/null -H "Cookie: session=garbage" http://127.0.0.1:3000/console | grep -i "^set-cookie" || echo "NO-SET-COOKIE"`
Expected: `NO-SET-COOKIE`（前端不主动清 HttpOnly 的会话 cookie，符合 spec §5 的 F 行）。把这一行实测结果贴进本步骤下面的记录，作为「不自愈」的凭据。

- [ ] **Step 7: Commit**

```bash
git add frontend/app/signin/page.tsx frontend/app/signup/page.tsx
git commit -m "feat(frontend): 已登录不准停在登录/注册页"
```

---

### Task 5: 注册成功去向改为控制台

**Files:**
- Modify: `frontend/hooks/use-signup.ts`

**Interfaces:**
- Consumes: 无新依赖
- Produces: 行为变化——注册成功后 `router.push("/console")`（原 `/signin`），toast 文案 `auth.signup.success` 不变

- [ ] **Step 1: 改成功分支**

`use-signup.ts` 里成功路径的 `router.push("/signin")` → `router.push("/console")`。理由见 spec §2（注册即登录 + 反向跳两条规则叠加，推 `/signin` 会被立刻弹回来，白跳一次）。

- [ ] **Step 2: 类型检查**

Run: `npx tsc --noEmit`
Expected: 退出码 0。

- [ ] **Step 3: 浏览器取证（MutationObserver，别信截图时序）**

装观察者后注册一个全新账号：

```js
async () => {
  const log = [];
  new MutationObserver(() => document.querySelectorAll("[data-sonner-toast]").forEach((n) => {
    const x = n.textContent || "";
    if (x && !log.includes(x)) log.push(x);
  })).observe(document.body, { childList: true, subtree: true });
  const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set;
  const type = (n, v) => { const e = document.querySelector(`input[name="${n}"]`); set.call(e, v); e.dispatchEvent(new Event("input", { bubbles: true })); };
  type("email", "goto-console-check@example.com"); type("name", "goto_console_check");
  type("password", "abc12345"); type("confirmPassword", "abc12345");
  document.querySelector('button[type="submit"]').click();
  await new Promise((r) => setTimeout(r, 1200));
  return { url: location.href, log };
}
```
Expected: `url` 为 `http://localhost:3000/console`，`log` 含 `注册成功，请登录`。
若工具的真实鼠标事件送不进页面（本会话出现过 `pointerdown/mousedown/click` 计数全 0），照上面用 `button.click()` + 把真实点击这一步交回用户手点确认，并在提交信息里注明。

- [ ] **Step 4: Commit**

```bash
git add frontend/hooks/use-signup.ts
git commit -m "feat(frontend): 注册成功改跳控制台"
```

---

### Task 6: 登出

**Files:**
- Create: `frontend/hooks/use-signout.ts`
- Modify: `frontend/components/console/console-user-menu.tsx`
- Modify: `frontend/messages/zh-CN.json`, `frontend/messages/en.json`

**Interfaces:**
- Consumes: `apiRequest`（`lib/api.ts`，已有）；后端 `POST /api/v1/auth/logout` → **200 `{}`**（`backend/internal/modules/auth/controller.go:68-74`）
- Produces: `export function useSignout(): { isSigningOut: boolean; signOut: () => Promise<void> }`；新 i18n 键 `userMenu.signOut`、`userMenu.signOutFailed`

- [ ] **Step 1: 补两份语言包**

`userMenu.signOut` = 「退出登录」/ "Sign out"；`userMenu.signOutFailed` = 「退出失败，请稍后重试」/ "Sign out failed, please try again later"。两份键位必须一致。

- [ ] **Step 2: 写 `hooks/use-signout.ts`**

`signOut()`：`await apiRequest("/api/v1/auth/logout", { method: "POST" })` → 成功 `router.replace("/signin")`；catch → `toast.error(t("signOutFailed"))` 并留在原地。`isSigningOut` 用 `useState` 包住整个过程。菜单里的主题/语言 handler 不是分层先例（它们不碰网络），所以本任务的逻辑一律留在 hook 里，组件只渲染——延续「UI 只管 UI，逻辑只做逻辑」。

- [ ] **Step 3: 菜单加项**

`ConsoleUserMenu` 里，`访问前台` 之后、`主题色` 子菜单之前，插入 `DropdownMenuItem`：`render` 走按钮、图标 `LogOut`（lucide）、文案 `t("signOut")`、`disabled={isSigningOut}`、`onClick={signOut}`。照该文件既有项的写法，不要新造 variant。

- [ ] **Step 4: 类型/lint/格式**

Run: `npx tsc --noEmit && npm run lint && npm run format:check`
Expected: 全部退出码 0。

- [ ] **Step 5: 冷请求证明登出真的清 cookie**

```bash
curl -s -D - -o /dev/null -b /tmp/gate-jar -X POST http://127.0.0.1:3000/api/v1/auth/logout | grep -i "^set-cookie"
curl -s -o /dev/null -w "%{http_code} %{redirect_url}\n" -b /tmp/gate-jar http://127.0.0.1:3000/console
```
Expected: 第一行含 `session=` 且带 `Max-Age=0`（或 `Expires` 为过去时间）；第二行 `307 http://127.0.0.1:3000/signin`（jar 里的 cookie 已被后端作废）。

- [ ] **Step 6: 浏览器取证（Review Focus 3）**

整页刷新 `/console` → 点「退出登录」→ 期望落 `/signin`；再在地址栏输入 `http://localhost:3000/console` → 期望被弹回 `/signin`。这一步**必须整页刷新**后再看，别在 HMR 热更的画面里判断（服务端 fetch 有 HMR 缓存）。

- [ ] **Step 7: Commit**

```bash
git add frontend/hooks/use-signout.ts frontend/components/console/console-user-menu.tsx frontend/messages/zh-CN.json frontend/messages/en.json
git commit -m "feat(frontend): 控制台登出入口"
```

---

### Task 7: pending 会话取证（D 态）——**本轮不做**

> 用户 2026-10-12 决定推后：不在本轮造开 2FA 的账号。下面整段保留给接 2FA 那轮直接取用，届它是 spec §5 D 行与「反向跳为什么不放进 proxy」唯一的实测凭据。本计划其余任务的验证与提交不依赖它。

**Files:**
- Create: `.scratch/totp.mjs`（一次性探针，按用户惯例留在 `.scratch/` 不删）

**Interfaces:**
- Consumes: 后端 `POST /auth/2fa/setup`（→ `{secret, otpauth_url}`）、`POST /auth/2fa/enable`（→ 200 + 恢复码，**会吊销其它会话**）、`POST /auth/login`
- Produces: 一条写在提交信息/DEVLOG 里的实测结论：pending 会话下 `/console` 与 `/signin` 的收敛路径

- [ ] **Step 1: 写 TOTP 生成小工具**

`.scratch/totp.mjs`：对 RFC 6238 参数 **SHA-1 / 30 秒窗口 / 6 位**，输入是 setup 响应里的 base32 `secret`（需先 base32 解码成字节），取当前窗口的计数器值做 HMAC、动态截断取模。后端用的就是这套参数（`internal/modules/auth/totp.go`），对不上就打脸，所以先拿一次 enable 成功当自证。别去装新依赖。

- [ ] **Step 2: 给一个账号开启 2FA**

用 Task 2 的 jar（`gate_check`）依次：`POST /auth/2fa/setup` → 用 Step 1 算出的 code `POST /auth/2fa/enable` → 记录返回的恢复码。**注意 enable 会吊销其它会话**，随后必须重新登录。

- [ ] **Step 3: 重新登录，拿到 pending 态 cookie**

先把登录体写成文件（口令与 Task 2 Step 6 里 `printf` 出去的那个 `password` 字段同值，本文不重复字面量以免被脱敏层改写）：

```bash
printf '{"identifier":"gate_check","password":"***"}' > /tmp/gate-login.json   # 手工把 <口令> 换成 Task 2 Step 6 用的值
```

Run: `curl -s -c /tmp/2fa-jar -o /dev/null -w "%{http_code}\n" -X POST -H "Content-Type: application/json" --data-binary @/tmp/gate-login.json http://127.0.0.1:3000/api/v1/auth/login`
Expected: `401`，响应体 `code` 为 `two_factor_required`，且响应头里**确实有** `Set-Cookie: session=...`（这就是 D 态可达的证据）。

- [ ] **Step 4: D 态门禁收敛断言**

Run:
```bash
curl -s -o /dev/null -w "console=%{http_code} %{redirect_url}\n" -b /tmp/2fa-jar http://127.0.0.1:3000/console
curl -s -o /dev/null -w "signin=%{http_code} %{redirect_url}\n" -b /tmp/2fa-jar http://127.0.0.1:3000/signin
curl -s -o /dev/null -w "followed=%{http_code}\n" -L --max-redirs 5 -b /tmp/2fa-jar http://127.0.0.1:3000/console
```
Expected: `console=307 →/signin`、`signin=200`（**不跳**）、`followed=200` 且无 `curl: (47)`。这三行就是 spec §5 D 行与「反向跳故意不放 proxy」的实测凭据。

- [ ] **Step 5: 关掉该账号的 2FA，恢复环境**

用 pending jar 现算一个 code 走 `POST /auth/2fa/verify` 完成登录，随后立刻 `POST /auth/2fa/disable` 把该账号恢复成未开 2FA，避免留下一个手上没有验证器就永远登不上的账号。

- [ ] **Step 6: Commit（只有探针与文档）**

```bash
git add .scratch/totp.mjs
git commit -m "test(frontend): D 态会话门禁取证探针"
```
如果 `.scratch/` 被 `.gitignore` 排除，就不提交，改为把三条实测输出贴进 Task 8 的 DEVLOG 条目。

---

### Task 8: 口径收尾（文档一致性）

**Files:**
- Modify: `docs/DEVLOG.md`
- Modify: `docs/superpowers/plans/2026-10-11-neoblog-auth.md`
- Modify: `docs/superpowers/specs/2026-10-11-neoblog-auth-layer2-design.md`, `docs/superpowers/specs/2026-10-11-neoblog-auth-design.md`
- Modify: `docs/superpowers/specs/2026-10-12-neoblog-console-auth-design.md`

**Interfaces:**
- Consumes: 前面各任务的实际落地结果与取证输出
- Produces: 无代码接口

- [ ] **Step 1: DEVLOG 补一条本轮**

照该文件既有格式（日期 + 版本 + 做了什么 + 已知缺口 + 关键决定）。内容至少包含：两层门禁与职责边界、两处故意偏离官方（反向跳不入 proxy、session 读取放 layout 顶层）及理由、Task 2 Step 7 的故障取证结果，以及「D 态（2FA pending 会话）本轮未实测取证」这条欠账（对照 Task 7 抬头）。

- [ ] **Step 2: 更正「前端未接」旧口径**

`plans/2026-10-11-neoblog-auth.md:13` 现写「按用户要求收窄为后端 only——Task 7 与 Task 9 不纳入本分支，这两节作废」。前端接线已在 `16e210a`–`22d272c` 落地，追加一行修订说明（保留原句，加「2026-10-12 更正：Task 9 前端接线已另行落地，见 specs/2026-10-12-neoblog-console-auth-design.md」），别抹掉历史。

- [ ] **Step 3: 更正 204 陈旧码口径**

`docs/DEVLOG.md:335` 与 `:399`、`specs/2026-10-11-neoblog-auth-layer2-design.md:48/85/100`、`specs/2026-10-11-neoblog-auth-design.md:86` 里 `logout` / `password` 写的 **204**，与代码不符（`controller.go:73/130` 是 200 `{}`）。逐处改成 200 并注明一次「2026-10-12 对账代码更正」。（`backend/README.md:135` 已正确，不动。）

- [ ] **Step 4: spec 落地回写**

在 `2026-10-12-neoblog-console-auth-design.md` 头部加一行「状态：已落地（YYYY-MM-DD）」，并把实测与 spec 有出入的地方就地订正（例：若 matcher 改成了 `["/console", "/console/:path*"]`，§4 的代码块要跟着改）。

- [ ] **Step 5: 全仓门禁复跑**

Run: `npx tsc --noEmit && npm run lint && npm run format:check`（在 `frontend/`）
Expected: 三项退出码 0。

- [ ] **Step 6: Commit**

```bash
git add docs/DEVLOG.md docs/superpowers/plans/2026-10-11-neoblog-auth.md docs/superpowers/specs/2026-10-11-neoblog-auth-layer2-design.md docs/superpowers/specs/2026-10-11-neoblog-auth-design.md docs/superpowers/specs/2026-10-12-neoblog-console-auth-design.md
git commit -m "docs: 控制台鉴权轮 DEVLOG 登记与旧口径更正"
```

---

## 落地后仍然欠着的（不属于本计划）

- **D 态（2FA pending 会话）的重定向收敛未实测**：Task 7 整段留档，接 2FA 那轮照它跑（需造一个开 TOTP 的 dev 账号）。本轮该结论只有 spec §5 的推演支撑。
- **prefetch 下的 307 行为没取证**：spec §7 说这块无文档覆盖，而 dev 不做 prefetch，要验只能 `next build` + `next start`（会动他的 `.next` 与端口）。本计划不做，留给下一轮单独决定。
- 后端修复 G 态：`register` / 正常 `login` 成功路径显式清 `auth.pending_2fa_user_id`，并把不变量写回 2FA spec（spec §7/§8）。
- 界面内接口调用 401 统一跳登录（`lib/api.ts` 单一出口）——控制台开始消费真实数据那轮。
- 恢复纯 CSR 门控、`error.tsx`、角色/权限门禁、`?next=` 回跳、前端版本号口径、首个 owner 建号。
