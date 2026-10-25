# 自研人机验证前端接入 Implementation Plan

> **For agentic workers:** Execute the tasks in order and check off every step. If the environment provides `superpowers:subagent-driven-development` or `superpowers:executing-plans`, use it; this workspace only ships the planning skill, so native task-by-task execution is the expected path.

**Goal:** 让登录 / 注册页在 `captcha_provider=builtin` 时弹出与 hao 博客同款的滑块验证，验证通过后把一次性 `captcha_token` 带回现有表单提交。

**Architecture:** 传输层（`lib/captcha-challenge.ts`）只负责出题 / 交卷请求与形状校验；`useCaptchaGate` 负责按 provider 预取题目、开窗、429 提示与关闭清理；`BuiltinCaptcha` 只负责滑块交互与六种状态；`CaptchaDialog` 按 provider 分派 Turnstile 或 builtin。后端、接口、迁移、设置键零改动。

**Tech Stack:** Next.js 16.4 / React 19 / TypeScript / next-intl / Tailwind v4 / base-ui Dialog / sonner / lucide-react

**Spec:** `docs/superpowers/specs/2026-10-26-neoblog-builtin-captcha-frontend-design.md`

## Global Constraints

- 只改 `frontend/` 代码、两份 `messages`、`docs/DEVLOG.md` 与 `backend/README.md` 的一句过时说明；`backend/` 生产代码、接口、迁移、设置键零改动。
- 不新增依赖；复用现有 base-ui Dialog、Button、sonner、lucide-react、next-intl。
- 滑块样式 / 状态 / 动画按 spec §4 的表逐项照抄 hao 博客 `selfhosted-widget.tsx`；只允许 spec §4.1 列出的四处协议偏离。
- 不在前端添加任何兜底 / 降级成功：出题失败显示失败卡，交卷失败显示失败条并换题，不放行、不伪造 token。
- 不写代码注释；用户可见文案中英两份同步；不新版本进位（保持 0.24.0）。
- 不新增前端测试框架；验证 = `tsc` / ESLint / Prettier + 浏览器实测。
- 开发 / 测试库的 captcha 设置只在验证期间临时改，验证完必须还原；不把测试账号或设置写进仓库。
- 改前端前先读 `frontend/AGENTS.md` 指向的 Next 16 文档；本轮只用现有 App Router 客户端组件模式，不新增服务端取数或新路由。

## Review Focus

1. **预取失败后的自动重试不能变成循环**：初始出题失败 → 开窗 → 组件自动再试一次 → 仍失败必须停在失败卡，只有点「重新加载」才再发请求；`begin()` 在弹窗已开时必须直接返回。验证：`begin()` 守卫在 Task 3 步骤 7，失败卡与自动重试停在失败卡在 Task 5 步骤 3。
2. **重复提交与成功期间关闭**：预取中连点只发一次出题；成功停留 1618ms 内点 X 必须取消提交，网络里不能出现登录 / 注册请求。验证在 Task 3 步骤 7。
3. **缩放后的坐标换算**：窗口宽度小于 300px 时，拖动距离必须按 `masterWidth / renderedWidth` 换回主图坐标，且 `x` 不能越过 `masterWidth - tileWidth`。验证在 Task 3 步骤 7 与 Task 5 步骤 3 的窄窗检查。
4. **错误路径不得成功**：403 / 429 / 5xx / 断网各自落到 toast 或失败卡 / 失败条，登录请求不能发出。验证在 Task 5 步骤 3。
5. **Turnstile 回归**：`CaptchaDialog` 的 props 改造后，Turnstile 仍能加载、拿 token、登录成功，设置页「验证配置」弹窗仍工作。验证在 Task 5 步骤 3。

---

## File Structure

| 文件 | 动作 | 责任 |
| --- | --- | --- |
| `frontend/lib/api.ts` | 修改 | `RequestOptions` 增加 `signal?: AbortSignal` 并透传 fetch |
| `frontend/lib/captcha-challenge.ts` | 新建 | 出题 / 交卷传输与响应形状校验 |
| `frontend/hooks/use-captcha-gate.ts` | 新建 | provider 分派、预取、开窗、429 toast、关闭清理 |
| `frontend/components/auth/builtin-captcha.tsx` | 新建 | 滑块六阶段、拖拽、坐标换算、交卷、失败重试、摆动 |
| `frontend/lib/dialog-nudge.ts` | 新建 | 弹窗快速缩放摆动 |
| `frontend/lib/public-settings.ts` | 修改 | `CaptchaConfig` 判别联合与 builtin 识别 |
| `frontend/components/auth/captcha-dialog.tsx` | 修改 | 按 provider 渲染、340px 外壳、成功定时器取消 |
| `frontend/app/login/signin-form.tsx` | 修改 | 接 `useCaptchaGate` |
| `frontend/app/register/signup-form.tsx` | 修改 | 接 `useCaptchaGate` |
| `frontend/app/console/settings/captcha/captcha-settings-form.tsx` | 修改 | 作用域行对 builtin 显示；Turnstile 弹窗改传 config |
| `frontend/messages/zh-CN.json` / `en.json` | 修改 | 7 条滑块文案 |
| `docs/DEVLOG.md` | 修改 | 本轮记录 |
| `backend/README.md` | 修改 | 去掉「前端未接 builtin」的过时说明 |

---

### Task 1: 传输层与 api signal

**Files:**
- Modify: `frontend/lib/api.ts`
- Create: `frontend/lib/captcha-challenge.ts`

**Interfaces:**
- Consumes: `apiRequest` from `@/lib/api`；`CaptchaScope` from `@/lib/captcha-settings`。
- Produces:
  ```ts
  export type CaptchaChallenge = {
    id: string;
    masterImage: string;
    tileImage: string;
    masterWidth: number;
    masterHeight: number;
    tileWidth: number;
    tileHeight: number;
    tileX: number;
    tileY: number;
    expiresAt: string;
  };
  export function requestCaptchaChallenge(scope: CaptchaScope, signal?: AbortSignal): Promise<CaptchaChallenge>;
  export function solveCaptchaChallenge(id: string, x: number, y: number, signal?: AbortSignal): Promise<string>;
  ```

- [ ] **Step 1: 给 `apiRequest` 增加 `signal`**

在 `RequestOptions` 增加 `signal?: AbortSignal`，解构后传给 `fetch`。其余逻辑（错误封装、401 上报、Retry-After）一行不动。

- [ ] **Step 2: 新建 `frontend/lib/captcha-challenge.ts`**

- `requestCaptchaChallenge(scope, signal?)` → `POST /api/v1/captcha/challenges`，body `{ scope }`，用 `apiRequest<unknown>`，把 snake_case 响应映射为上表 camelCase。
- `solveCaptchaChallenge(id, x, y, signal?)` → `POST /api/v1/captcha/challenges/${encodeURIComponent(id)}/solutions`，body `{ x, y }`，校验响应里的 `captcha_token` 是非空 string 后返回。
- 形状校验：对象、`id` / 两张图 / `expires_at` 是 string，六个尺寸 / 位置字段是有限 number；任何一项不符抛 `new Error("captcha challenge payload malformed")` 或 `new Error("captcha solution payload malformed")`。不填默认值、不吞错。
- 传输层不判断 429 / 403，统一让 `ApiError` 上抛给调用方。

- [ ] **Step 3: 静态门禁**

Run:
```bash
cd frontend
npx tsc --noEmit
npm run lint
npx prettier --check lib/api.ts lib/captcha-challenge.ts
```
Expected: tsc 0 错误；lint 0 error；prettier 通过。

- [ ] **Step 4: Commit**

```bash
git add frontend/lib/api.ts frontend/lib/captcha-challenge.ts
git commit -m "feat(frontend): 增加 builtin 人机验证传输层"
```

---

### Task 2: 滑块组件、摆动与文案

**Files:**
- Create: `frontend/lib/dialog-nudge.ts`
- Create: `frontend/components/auth/builtin-captcha.tsx`
- Modify: `frontend/messages/zh-CN.json`
- Modify: `frontend/messages/en.json`

**Interfaces:**
- Consumes: Task 1 的 `CaptchaChallenge`、`requestCaptchaChallenge`、`solveCaptchaChallenge`；`CaptchaScope`；`toast`；`useTranslations("auth")`。
- Produces:
  ```ts
  export function nudgeDialogNode(dialogNode: HTMLElement | null): void;
  export function BuiltinCaptcha(props: {
    scope: CaptchaScope;
    initialChallenge: CaptchaChallenge | null;
    onToken: (token: string) => void;
  }): React.ReactElement;
  ```

- [ ] **Step 1: 两份语言包加 7 条文案**

在 `auth` 对象、现有 `captchaUnavailable` 之后按 spec §8 逐字加入：`captchaImageAlt`、`captchaSliderHint`、`captchaLoadFailed`、`captchaReload`、`captchaSuccess`、`captchaError`、`captchaRateLimited`。中英键名与顺序一致，JSON 合法；**不新增 `captchaLoading` / `captchaVerifying`**（用户 2026-10-26 点名移除加载文案与校验中文案）。

- [ ] **Step 2: 新建 `frontend/lib/dialog-nudge.ts`**

照抄 `D:\protect\NextJs_Blog\lib\console-dialog\dialog-nudge.ts` 的 `quick-scale` 预设：关键帧 `scale(1) → 0.985@0.32 → 1.006@0.68 → 1`，时长 140ms，`ease`；`prefers-reduced-motion: reduce` 时直接返回；节点为 null 时 no-op。

- [ ] **Step 3: 新建 `frontend/components/auth/builtin-captcha.tsx`**

- 文件首行 `"use client"`。
- Props / 阶段 / 状态按 spec §5.3、§6：
  - 阶段：`loading | ready | dragging | verifying | success | failed`。
  - 常量：`CAPTCHA_FAIL_RETRY_MS = 800`、`DEFAULT_CAPTCHA_CANVAS = { width: 300, height: 220 }`。
  - `initialChallenge` 非空 → 初始 `ready`；为空 → 初始 `loading` 并在挂载 effect 里请求一次。用 ref 记住「已采用初始题 / 已发起首次请求」，父组件因宽度变化重渲染时不能重复出题。
  - `requestChallenge`：abort 上一个 controller，调 `requestCaptchaChallenge(scope, signal)`；成功重置 `slidePx/elapsed` 并进 `ready`；失败时 `signal.aborted` 直接返回，`error instanceof ApiError && error.status === 429` 先 `toast.error(t("captchaRateLimited"))`，然后 `challenge=null`、`failed`。
  - `ResizeObserver` 量 `hostRef` 宽度；卸载时断开、abort、清 800ms 定时器。
- 拖拽：
  - `maxDrag = Math.max(0, (challenge.masterWidth - challenge.tileWidth - challenge.tileX) * scale)`；`scale = renderedWidth / challenge.masterWidth`。
  - `beginDrag` 仅 `ready` 且 `renderedWidth > 0`；`setPointerCapture` + 记录 `origin.clientX` + `dragStart=performance.now()`。
  - `moveDrag` 用 `clamp(event.clientX - origin.clientX, 0, maxDrag)` 写 state，同时写 `slidePxRef`。
  - `endDrag` 仅在 `dragging` 阶段执行，释放 pointer capture，并用 `slidePxRef` 计算；`onPointerCancel` 同样走 `endDrag`，重复触发不会二次交卷：
    ```ts
    const scale = renderedWidth / challenge.masterWidth;
    const x = Math.round(challenge.tileX + slidePxRef.current / scale);
    const y = challenge.tileY;
    ```
- 交卷：
  - `verifying` → `solveCaptchaChallenge(id, x, y)`。
  - 成功：`elapsed = max(0.1, round((performance.now()-dragStart)/100)/10).toFixed(1)`；`onToken(token)`；`success`。
  - 失败：`nudgeDialogNode(hostRef.current?.closest('[role="dialog"]') ?? null)`；`failed`；800ms 后无条件 `requestChallenge()`（后端已判负，不能复用原题）；429 由请求流程 toast。
- 渲染：
  - 图区 / 拼图块 / 滑轨 / 拇指 / 进度填充 / 成功条与耗时 / 失败条 / 失败卡，类名与状态逐项照 spec §4 表；拼图块位置 `translateX(tileX * scale + slidePx)`、`top = tileY * scale`；加载态（`challenge === null && phase !== "failed"`）渲染 `null`，不渲染占位 / 文案 / 转圈；`verifying` 不渲染遮罩 / 文案 / 转圈，只锁滑轨；失败卡用 `DEFAULT_CAPTCHA_CANVAS` 的宽高比预留并保留文案与「重新加载」。
  - 滑轨 `role="slider"`、`aria-valuemin=0`、`aria-valuemax=max(1, round(maxDrag))`、`aria-valuenow=round(slidePx)`、`aria-disabled=isLocked`、`tabIndex=0`；不加键盘处理；类名加 `outline-none`，不出现焦点光晕 / 边框变色。
  - `onToken` 只调用一次；所有 img 加 `draggable={false}`。

- [ ] **Step 4: 静态门禁**

Run:
```bash
cd frontend
npx tsc --noEmit
npm run lint
npx prettier --check lib/dialog-nudge.ts components/auth/builtin-captcha.tsx messages/zh-CN.json messages/en.json
```
Expected: 全部通过；`grep -c '"captchaSliderHint"' messages/zh-CN.json messages/en.json` 各为 1；`grep -rn "captchaLoading\|captchaVerifying" messages components app` 无结果。

- [ ] **Step 5: Commit**

```bash
git add frontend/lib/dialog-nudge.ts frontend/components/auth/builtin-captcha.tsx frontend/messages/zh-CN.json frontend/messages/en.json
git commit -m "feat(frontend): 添加 builtin 滑块组件与文案"
```

---

### Task 3: 闸门、配置联合、弹窗分派与登录 / 注册接线

**Files:**
- Create: `frontend/hooks/use-captcha-gate.ts`
- Modify: `frontend/lib/public-settings.ts`
- Modify: `frontend/components/auth/captcha-dialog.tsx`
- Modify: `frontend/app/login/signin-form.tsx`
- Modify: `frontend/app/register/signup-form.tsx`
- Modify: `frontend/app/console/settings/captcha/captcha-settings-form.tsx`

**Interfaces:**
- Consumes: Task 1 的 `CaptchaChallenge` / `requestCaptchaChallenge`；Task 2 的 `BuiltinCaptcha`。
- Produces:
  ```ts
  export type CaptchaConfig =
    | { provider: "turnstile"; siteKey: string }
    | { provider: "builtin"; scope: CaptchaScope };

  export function useCaptchaGate(config: CaptchaConfig | null): {
    isOpen: boolean;
    challenge: CaptchaChallenge | null;
    begin: () => void;
    cancel: () => void;
    handleOpenChange: (open: boolean) => void;
  };

  // CaptchaDialog props
  {
    open: boolean;
    config: CaptchaConfig;
    challenge?: CaptchaChallenge | null;
    onOpenChange: (open: boolean) => void;
    onSolved: (token: string) => void;
  }
  ```

- [ ] **Step 1: `public-settings.ts` 改判别联合**

`CaptchaConfig` 换成上面的联合。`getCaptchaConfig(settings, scope)`：
1. `captcha_enabled !== true` → null；
2. `captcha_scopes` 里没有当前 scope → null；
3. `captcha_provider` 为 `turnstile` 时要求 `turnstile_site_key` 非空，返回 `{provider:"turnstile", siteKey}`；
4. 为 `builtin` 时返回 `{provider:"builtin", scope}`；
5. provider 其它值 → null。

- [ ] **Step 2: 新建 `frontend/hooks/use-captcha-gate.ts`**

- state：`isOpen`、`challenge`；ref：`preparingRef`、`controllerRef`、`isOpenRef`（与 `isOpen` 同步，避免表单闭包拿到旧值）。
- `begin()`：`config === null || isOpenRef.current || preparingRef.current` 直接返回；Turnstile 只 `isOpenRef=true` + `setIsOpen(true)`；builtin 设 `preparingRef=true`，新 controller，`requestCaptchaChallenge(config.scope, signal)`：
  - 成功：`setChallenge`、`isOpenRef=true`、`setIsOpen(true)`；
  - `signal.aborted`：返回；
  - `error instanceof ApiError && error.status === 429`：`toast.error(t("captchaRateLimited"))`，不开窗；
  - 其它：`setChallenge(null)`、`isOpenRef=true`、`setIsOpen(true)`。
  - `finally` 仅当 `controllerRef.current === controller` 时清 ref 与 `preparingRef`。
- `cancel()`：abort、清 ref、`preparingRef=false`、`isOpenRef=false`、`setChallenge(null)`、`setIsOpen(false)`。
- `handleOpenChange(false)` → `cancel()`；`true` 时 `isOpenRef=true` 并 `setIsOpen(true)`。
- 卸载 effect：abort。
- 用 `useTranslations("auth")` 取 `captchaRateLimited`。

- [ ] **Step 3: 改 `CaptchaDialog`**

- props 换成上面的契约；`config` 非空。
- `DialogContent` 加 `className="gap-3 p-5 sm:max-w-[340px]"`；保留 `blockOpenAutoFocus` 与 `disablePointerDismissal`；标题不变。
- 内容外层加 `<div className="grid max-w-full min-w-0 gap-4 py-1 [&>*]:max-w-full [&>*]:min-w-0">`（对照项目 AppDialog 同款内层）。
- provider 分派：
  ```tsx
  config.provider === "turnstile"
    ? <TurnstileCaptcha siteKey={config.siteKey} onToken={handleToken} onUnavailable={() => setIsUnavailable(true)} />
    : <BuiltinCaptcha scope={config.scope} initialChallenge={challenge ?? null} onToken={handleToken} />
  ```
- 保留 1618ms `CLOSE_DELAY_MS`；用户主动关闭（`next === false`）时清掉 `closeTimerRef` 并重置 `isUnavailable`，防止成功停留期间关窗后仍提交。

- [ ] **Step 4: 登录 / 注册表单接闸门**

两个表单同样处理：
```ts
const gate = useCaptchaGate(captcha);
const { ... } = useSigninForm(captcha !== null, gate.begin); // signup 同理
async function handleCaptchaSolved(token: string) {
  gate.cancel();
  focusField(await submit(token));
}
```
- 删除各自的 `isCaptchaOpen` state。
- 弹窗：
  ```tsx
  captcha === null ? null : (
    <CaptchaDialog
      open={gate.isOpen}
      config={captcha}
      challenge={gate.challenge}
      onOpenChange={gate.handleOpenChange}
      onSolved={handleCaptchaSolved}
    />
  )
  ```
- 其余字段、toast、pending / retry 逻辑一行不动。

- [ ] **Step 5: 设置页只先修编译**

`captcha-settings-form.tsx` 的自检弹窗改为：
```tsx
<CaptchaDialog
  open={isVerifyOpen}
  config={{ provider: "turnstile", siteKey: values.turnstileSiteKey }}
  onOpenChange={setIsVerifyOpen}
  onSolved={handleCaptchaSolved}
/>
```
作用域行的显示条件留给 Task 4。

- [ ] **Step 6: 静态门禁**

Run:
```bash
cd frontend
npx tsc --noEmit
npm run lint
npx prettier --check lib/public-settings.ts hooks/use-captcha-gate.ts components/auth/captcha-dialog.tsx app/login/signin-form.tsx app/register/signup-form.tsx app/console/settings/captcha/captcha-settings-form.tsx
```
Expected: 全部通过；`grep -rn "isCaptchaOpen" app/login app/register` 无结果。

- [ ] **Step 7: 浏览器主链路实测**

准备（本机已有 `neoblog-postgres` 容器，测试库注册开启、邮箱验证关闭）：

先备份测试库 captcha 组现值，并确认没有在跑 `scripts/test.sh` / 并行会话（测试库是共享的）：

```bash
wsl -e bash -lc 'docker exec neoblog-postgres psql -U neoblog -d neoblog_test -c "SELECT name, value FROM setting_values WHERE setting_group = '\''captcha'\'' ORDER BY name;"'
```

本计划编写时测试库原值为 `enabled=false / provider=none / scopes=login,register / 两个密钥空`；注册开关为开、邮箱验证为关、域名控制为关。若实际不同，以这次查询为准，验证后逐值还原。

```bash
# 临时开启 builtin（login + register）
printf '%s\n' \
  "UPDATE setting_values SET value = to_jsonb(true) WHERE setting_group = 'captcha' AND name = 'captcha_enabled';" \
  "UPDATE setting_values SET value = to_jsonb('builtin'::text) WHERE setting_group = 'captcha' AND name = 'captcha_provider';" \
  "UPDATE setting_values SET value = to_jsonb('login,register'::text) WHERE setting_group = 'captcha' AND name = 'captcha_scopes';" \
| wsl -e bash -lc 'docker exec -i neoblog-postgres psql -U neoblog -d neoblog_test -v ON_ERROR_STOP=1'

# 后端指向测试库，端口 5733
cd backend
DATABASE_URL='postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_test?sslmode=disable' ADDR=:5733 bash scripts/dev.sh

# 另开终端：前端指向 5733（先确认 3000 没有旧 dev server）
cd frontend
BACKEND_URL=http://127.0.0.1:5733 npm run dev
```

两条服务用后台任务启动并记下任务 ID；验证完用 TaskStop 停（或 netstat 取 PID → taskkill）。

用浏览器打开 `http://localhost:3000/register`，注册一个新账号（邮箱 / 昵称带时间戳，密码如 `Passw0rd!`）。

**拖拽辅助脚本**：browser-use 的 `drag` 对滑轨子元素不稳定；本机实测可靠的是页面内合成 PointerEvent。脚本从 DOM 量出拼图块初始 x（`tileX`）与缩放，按 `answerX` 计算拖动距离；临时把滑轨的 `setPointerCapture` / `releasePointerCapture` 置空（仅验证用），三段事件之间留 60ms 让 React 完成阶段切换。把 `210` 换成 SQL 查到的 `answer_x`（错误用例用 `answer_x ± 30`）。题目表不存 `tile_x`，初始位置从 DOM 量：

```js
async () => {
  const answerX = 210;
  const bar = document.querySelector('[role="slider"]');
  const host = bar?.parentElement;
  const imgs = host ? host.querySelectorAll('img') : [];
  const master = imgs[0];
  const tile = imgs[1];
  if (!bar || !master || !tile) throw new Error('captcha widget not ready');
  const mr = master.getBoundingClientRect();
  const tr = tile.getBoundingClientRect();
  const br = bar.getBoundingClientRect();
  const scale = mr.width / master.naturalWidth;
  const tileX = (tr.left - mr.left) / scale;
  const dragCss = (answerX - tileX) * scale;
  bar.setPointerCapture = () => {};
  bar.releasePointerCapture = () => {};
  const x0 = br.left + 20;
  const y = br.top + br.height / 2;
  const ev = (type, x, buttons) =>
    new PointerEvent(type, {
      bubbles: true,
      cancelable: true,
      pointerId: 1,
      pointerType: 'mouse',
      isPrimary: true,
      clientX: x,
      clientY: y,
      button: 0,
      buttons,
    });
  bar.dispatchEvent(ev('pointerdown', x0, 1));
  await new Promise((resolve) => setTimeout(resolve, 60));
  bar.dispatchEvent(ev('pointermove', x0 + dragCss, 1));
  await new Promise((resolve) => setTimeout(resolve, 60));
  bar.dispatchEvent(ev('pointerup', x0 + dragCss, 0));
  return { answerX, tileX, dragCss, scale };
}
```

逐步验证：
1. 点击注册后先出现滑块弹窗，图已加载；**预取阶段连点两次注册**，网络里只有一条 `POST /api/v1/captcha/challenges`。
2. 弹窗保持打开时执行 `() => document.querySelector('form').dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }))`：表单 onSubmit 会走一遍，但 `begin()` 因弹窗已开直接返回，网络里不能新增出题请求。
3. 查答案并拖到错误位置：
   ```bash
   wsl -e bash -lc 'docker exec neoblog-postgres psql -U neoblog -d neoblog_test -At -F "|" -c "SELECT id, answer_x, answer_y, status FROM captcha_challenges WHERE status = 0 ORDER BY created_at DESC LIMIT 1;"'
   ```
   用上面脚本拖到偏离答案 ≥ 30px 的位置（右侧放不下就向左偏）：红色失败条 + 弹窗摆动；800ms 后出现新图；旧题 `status=3`；网络里没有 `POST /api/v1/users`。
4. 对新题重复，拖到 `answer_x`：成功绿条 + 耗时；1618ms 后注册请求带 `captcha_token` 并成功跳登录页。
5. 再走一遍注册（换新邮箱）：交卷后轮询 `[role="slider"]` 的 `data-state` 到 `success`，立即点击 `[data-slot="dialog-close"]`；等 2 秒，网络里没有新的 `POST /api/v1/users`。
6. 用刚注册账号登录：重复一次滑块，登录成功进入 `/console`。

Expected: 六条全部通过；测试库 `captcha_challenges` 里能查到对应 pending / solved / failed 行。

- [ ] **Step 8: Commit**

```bash
git add frontend/lib/public-settings.ts frontend/hooks/use-captcha-gate.ts frontend/components/auth/captcha-dialog.tsx frontend/app/login/signin-form.tsx frontend/app/register/signup-form.tsx frontend/app/console/settings/captcha/captcha-settings-form.tsx
git commit -m "feat(frontend): 登录注册接入 builtin 滑块验证"
```

---

### Task 4: 设置页作用域行对 builtin 显示

**Files:**
- Modify: `frontend/app/console/settings/captcha/captcha-settings-form.tsx`

**Interfaces:**
- Consumes: Task 3 的 `CaptchaConfig` / `CaptchaDialog`。
- Produces: 无新接口；仅渲染条件变化。

- [ ] **Step 1: 移动作用域行**

把现有「启用范围」`ConsoleSettingsRow` 从 `values.captchaProvider === CAPTCHA_PROVIDER_TURNSTILE` 的 fragment 里移出来，改成：
```tsx
{values.captchaProvider === CAPTCHA_PROVIDER_TURNSTILE ||
 values.captchaProvider === CAPTCHA_PROVIDER_BUILTIN ? (
  <ConsoleSettingsRow ... />
) : null}
{values.captchaProvider === CAPTCHA_PROVIDER_TURNSTILE ? (
  <>站点密钥 / 私钥 / 验证配置</>
) : null}
```
- 不改保存 hook、字段名、默认值、权限。

- [ ] **Step 2: 静态门禁**

Run:
```bash
cd frontend
npx tsc --noEmit
npm run lint
npx prettier --check app/console/settings/captcha/captcha-settings-form.tsx
```
Expected: 全部通过。

- [ ] **Step 3: 浏览器核对**

用 Task 3 注册的账号在测试库提升为 owner：
```bash
printf '%s\n' \
  "UPDATE users SET group_id = '00000000-0000-4000-8000-000000000001' WHERE email = '<Task 3 注册邮箱>';" \
| wsl -e bash -lc 'docker exec -i neoblog-postgres psql -U neoblog -d neoblog_test -v ON_ERROR_STOP=1'
```
回到浏览器（必要时刷新）进入 `/console/settings/captcha`，现有会话即可（权限按请求重读，不必重新登录）：
- provider 选 `builtin`：作用域行可见、可保存；站点密钥 / 私钥 / 验证配置不出现。
- provider 选 `turnstile`：作用域行、站点密钥、私钥、验证配置都出现。
- provider 选 `none`：作用域、密钥与验证配置行都不出现。
- 保存后刷新，值仍在。

- [ ] **Step 4: Commit**

```bash
git add frontend/app/console/settings/captcha/captcha-settings-form.tsx
git commit -m "feat(frontend): 人机验证设置页对 builtin 显示作用域"
```

---

### Task 5: 文档、错误路径与 Turnstile 回归

**Files:**
- Modify: `docs/DEVLOG.md`
- Modify: `backend/README.md`
- Add: `docs/superpowers/specs/2026-10-26-neoblog-builtin-captcha-frontend-design.md`（已存在）
- Add: `docs/superpowers/plans/2026-10-26-neoblog-builtin-captcha-frontend.md`（本文件）

**Interfaces:**
- Consumes: 前四个任务的全部产物。
- Produces: 文档与最终验证记录。

- [ ] **Step 1: DEVLOG**

新增 `## 2026-10-26 · 自研人机验证前端接入（登录 / 注册滑块弹窗）（纯前端，版本保持 0.24.0）`，记录：
- 动机：后端 builtin 已可用，前端未接时切 provider 会把登录 / 注册全部拦住。
- 范围：文件清单、零后端生产代码、零新依赖、零新路由、不升版本。
- 决策：样式照 hao 博客；预取后开窗；答错换题；失败卡预留高度；设置页作用域行；加载态与校验中按用户 2026-10-26 点名移除文案 / 转圈（加载态也不渲染占位）。
- 验证：静态门禁 + 浏览器主链路 / 错误路径 / Turnstile 回归的实测结果；未实测项明确标注。
- 已知缺口：管理端自检无 builtin、公开设置 60 秒缓存窗口、无键盘操作、无前端自动化测试。
- 提交状态：按实际填写。

- [ ] **Step 2: 改 backend README 过时说明**

把 `backend/README.md` 中「前端尚未接 builtin，**切 provider 前必须先上前端滑块组件**，否则登录 / 注册会被 `captcha_failed` 拦住」替换为：
「前端已接 builtin（登录 / 注册弹窗按 provider 渲染 Turnstile 或自研滑块）；切换 provider 后公开设置最多缓存 60 秒，期间仍可能下发旧 provider。」
其余 API 表与设置章节不动。

- [ ] **Step 3: 浏览器错误路径与回归矩阵**

在前端 / 后端仍指向测试库的前提下，先把测试库恢复为 `enabled=true / provider=builtin / scopes=login,register`；若刚在 Task 4 保存过设置，重启前端或等 60 秒让公开设置缓存失效。

1. **403**：登录页已加载后，把测试库 provider 改成 `none`，提交账号 → 弹窗出现、组件自动重试一次、失败卡 +「重新加载」；网络无登录请求。测完改回 `builtin`。
2. **断网 / 5xx**：登录页加载后停掉 5733 后端，提交 → 失败卡；重启后端后点「重新加载」能出题。
3. **429**：连续 61 次 `curl -X POST http://127.0.0.1:5733/api/v1/captcha/challenges -H 'Content-Type: application/json' -d '{"scope":"login"}'` 触发限流，网页提交 → 只弹 toast、不打开弹窗；等 60 秒或重启后端恢复。
4. **窄窗坐标**：浏览器窗口缩到 < 360px（无法缩窗时用 `() => { const el = document.querySelector('[data-slot="dialog-content"]'); el.style.width = '280px'; el.style.maxWidth = '280px'; }` 模拟），完整过一次滑块；提交的 `x` 仍落在答案 ±4 内（数据库对应行变为 solved）。刷新页面即恢复样式。
5. **Turnstile 回归**：测试库设为 `enabled=true / provider=turnstile / turnstile_site_key=1x00000000000000000000AA / turnstile_secret_key=1x0000000000000000000000000000000AA / scopes=login,register`（Cloudflare 官方 always-pass 测试钥匙，已对官方文档核实），重启前端让公开设置缓存失效。登录 → 弹窗渲染 Cloudflare 组件并自动通过；登录请求成功。再进设置页点「验证配置」→ 自检成功 toast。测完还原 `builtin` 或原始 `none`；外网不可达则标注未验证，不伪造通过。
6. **弹窗尺寸 / 样式**：对照 spec §4 表逐一核对 340px 宽、滑轨 40px、六种状态；截图交用户确认。

Expected: 1–5 全部符合；任何一条失败先修代码再重跑该条。

- [ ] **Step 4: 还原环境**

```bash
printf '%s\n' \
  "UPDATE setting_values SET value = to_jsonb(false) WHERE setting_group = 'captcha' AND name = 'captcha_enabled';" \
  "UPDATE setting_values SET value = to_jsonb('none'::text) WHERE setting_group = 'captcha' AND name = 'captcha_provider';" \
  "UPDATE setting_values SET value = to_jsonb('login,register'::text) WHERE setting_group = 'captcha' AND name = 'captcha_scopes';" \
  "UPDATE setting_values SET value = to_jsonb(''::text) WHERE setting_group = 'captcha' AND name = 'turnstile_site_key';" \
  "UPDATE setting_values SET value = to_jsonb(''::text) WHERE setting_group = 'captcha' AND name = 'turnstile_secret_key';" \
| wsl -e bash -lc 'docker exec -i neoblog-postgres psql -U neoblog -d neoblog_test -v ON_ERROR_STOP=1'
```
- 停掉 5733 后端与前端 dev server。
- 删除页面里插入的临时标记（刷新即消失）；确认 `.scratch` 里没有本轮新增的未忽略文件。
- 按 Task 3 开始时的备份逐值还原测试库 captcha 组，并再查一次确认与备份逐字节一致；不要盲信上面的默认值（共享测试库可能已被其它会话改过）。
- 删除本轮注册的测试账号（换成实际邮箱）：
  ```bash
  printf '%s\n' "DELETE FROM users WHERE email = '<Task 3 注册邮箱>';" \
  | wsl -e bash -lc 'docker exec -i neoblog-postgres psql -U neoblog -d neoblog_test -v ON_ERROR_STOP=1'
  ```

- [ ] **Step 5: 全量静态门禁**

```bash
cd frontend
npx tsc --noEmit
npm run lint
npm run format:check
cd ..
git diff --check
git status --short
```
Expected: tsc / lint / prettier / whitespace 全过；`git status` 只剩本文档改动。

- [ ] **Step 6: Commit 文档**

```bash
git add docs/DEVLOG.md docs/superpowers/specs/2026-10-26-neoblog-builtin-captcha-frontend-design.md docs/superpowers/plans/2026-10-26-neoblog-builtin-captcha-frontend.md backend/README.md
git commit -m "docs: 记录自研人机验证前端接入轮"
```

- [ ] **Step 7: 最终核对**

```bash
git log --oneline -5
git status --short
```
Expected: 5 笔提交（4 个 feat + 1 个 docs）；工作区干净；未推送。若用户要求一次性提交，则把上面 5 笔压成 1 笔或按用户口径拆，不要擅自推送。
