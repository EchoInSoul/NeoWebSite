# 前端组件规范站 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `ui-check` 升级成轻档规范站，给业务在用的 7 种底座补"什么时候用"说明，其余保持展示。

**Architecture:** 纯加文案，不改任何组件源码。每个任务改一个 `Section` 块内的说明文字，互不依赖，可独立驗收。

**Tech Stack:** Next 16.4 App Router, React 19, next-intl, Tailwind v4, Base UI / shadcn base-nova

**Spec:** `docs/superpowers/specs/2026-10-13-neoblog-component-conventions-design.md`

## Global Constraints

- 生成的代码一律不写注释，包括一行"为什么"。
- 只改 `frontend/app/ui-check/page.tsx` 一个文件，不碰任何 `components/` 源码、`hooks/`、`lib/`、后端。
- 文案中英文：中文为主，组件名/变体名用原文（如 `default`、`InputGroupInput`）。
- 每个任务结束跑：`npm --prefix frontend run format:check`、`node frontend/node_modules/typescript/bin/tsc -p frontend/tsconfig.json --noEmit`、`npm --prefix frontend run lint`，全绿才提交。
- 按文件清单 `git add`，禁止 `git add -A`；提交信息风格 `feat(frontend): ...`。

## Review Focus

- 说明块是否压住交互 demo（说明文字过长把 `Demo` 块挤到看不见）——预期说明块 `max-w-prose` 内收敛。
- 中英文案里的 `{minLength}` 这类插值括号被 prettier 或 JSX 转义吃掉——预期页面渲染出原文括号。
- `sonner` 说明块里的 toast 演示按钮与现有两个演示按钮互相覆盖——预期三个按钮并排，各弹各的。
- `sidebar` 说明写了"控制台专用"但 `ui-check` 里 sidebar demo 本身就在控制台外——预期说明写清"此 demo 仅展示，业务只许控制台用"。
- 改完 `ui-check` 整页 1078 行变 1200+ 行后首屏滚动定位漂移——预期纯文案追加，不动 `Section` 顺序与锚点。

---

### Task 1: 按钮说明块（button 6 变体 + 尺寸档）

**Files:**
- Modify: `frontend/app/ui-check/page.tsx`（"按钮与视觉标记" Section，约 :355-450）
- Test: 浏览器目检 `/ui-check` 该节

**Interfaces:**
- Consumes: 现有 `Section`（`:282-291`）、`Demo`（`:293-315`）本地组件，不改它们。
- Produces: 无（纯文案，后续任务不依赖）。

- [ ] **Step 1: 在"按钮与视觉标记" Section 顶部加一段说明块**

在 `frontend/app/ui-check/page.tsx` 的 `<Section title="按钮与视觉标记">` 内、第一个 `Demo` 之前，加一个 `<p className="max-w-prose text-sm text-muted-foreground">`，内容：`default` 主操作（每屏一个）、`secondary` 次要、`outline` 描边（对话框确认旁的取消）、`ghost` 幽灵（图标按钮）、`destructive` 危险（删除）、`link` 链接（跳转登录/注册那行字）；尺寸 `xs/sm/default/lg` 按信息密度选，图标按钮用 `icon` 系（`size-9` 为基准，与控制台 36px 按钮齐平）。

- [ ] **Step 2: 跑三项校验**

Run: `npm --prefix frontend run format:check && node frontend/node_modules/typescript/bin/tsc -p frontend/tsconfig.json --noEmit && npm --prefix frontend run lint`
Expected: 全绿。

- [ ] **Step 3: 浏览器目检该节**

整页刷新 `/ui-check`，确认说明块在 demo 上方、未压住按钮行。

- [ ] **Step 4: Commit**

```bash
git add frontend/app/ui-check/page.tsx
git commit -m "feat(frontend): ui-check 补按钮变体用法说明"
```

### Task 2: 输入组合说明块（input + input-group + label + auth/FieldError）

**Files:**
- Modify: `frontend/app/ui-check/page.tsx`（"输入与表单" Section，约 :454-635）
- Test: 浏览器目检 `/ui-check` 该节

**Interfaces:**
- Consumes: Task 1 后的文件状态（同一文件，不同节，互不干扰）。
- Produces: 无。

- [ ] **Step 1: 在"输入与表单" Section 顶部加一段说明块**

同一文件 `<Section title="输入与表单">` 内、第一个 `Demo` 之前，加同样式 `<p>`，内容：业务表单照登录页组合——`Label` + `Input`（或 `InputGroupInput` + 密码可见切换）+ `auth/FieldError`，错误时输入框挂 `aria-invalid` 并用 `aria-describedby` 绑报错 `id`（范例 `app/signin/signin-form.tsx:59-64`）；业务不许用 `ui/field` 的 `FieldError`，`ui-check` 里 `field` 那块 demo 只做展示。

- [ ] **Step 2: 跑三项校验**

Run: 同 Task 1 Step 2。
Expected: 全绿。

- [ ] **Step 3: 浏览器目检该节**

确认说明块未压住输入 demo，`field` demo 原样保留。

- [ ] **Step 4: Commit**

```bash
git add frontend/app/ui-check/page.tsx
git commit -m "feat(frontend): ui-check 补输入组合与 FieldError 用法说明"
```

### Task 3: 剩余 3 处说明块（sidebar + dropdown-menu + sonner）

**Files:**
- Modify: `frontend/app/ui-check/page.tsx`（"布局与容器"、"导航"内 sidebar demo、"浮层与菜单"、"反馈与数据"四处）
- Test: 浏览器目检 `/ui-check` 全页

**Interfaces:**
- Consumes: Task 1、Task 2 后的文件状态。
- Produces: 无。

- [ ] **Step 1: 加 sidebar 说明**

`sidebar` demo（约 :842-875）上方加同样式 `<p>`，内容：控制台专用，别处不许用；此 demo 仅展示，业务落点见 `app/console/layout.tsx` + `components/console/`。

- [ ] **Step 2: 加 dropdown-menu 说明**

"浮层与菜单" Section 内 `command` demo 之后（或该节顶部）加同样式 `<p>`，内容：用户菜单顺序——访问前台 → 主题 → 语言 → 分隔线 → 退出登录（落点 `components/console/console-user-menu.tsx`）；分隔线用现成 `DropdownMenuSeparator`，不自造。

- [ ] **Step 3: 加 sonner 说明**

`sonner（点击按钮）` demo（约 :1049-1058）旁加同样式 `<p>`，内容：成功/失败 toast（注册成功、登出失败）；`sonner` 走全局 `Toaster`（`app/layout.tsx:7`），业务只调 `toast.success` / `toast.error`。

- [ ] **Step 4: 跑三项校验**

Run: 同 Task 1 Step 2。
Expected: 全绿。

- [ ] **Step 5: 浏览器全页目检**

整页刷新 `/ui-check`，从头滚到尾，确认四个说明块位置正确、无压住、无 JSX 转义问题。

- [ ] **Step 6: Commit**

```bash
git add frontend/app/ui-check/page.tsx
git commit -m "feat(frontend): ui-check 补 sidebar/菜单/sonner 用法说明"
```

### Task 4: 收尾核对（引用清零 + 全量校验）

**Files:**
- Modify: 无（只读核对）。
- Test: grep + 三项校验 + 全页目检。

**Interfaces:**
- Consumes: Task 1–3 的三个提交。
- Produces: 交付结论。

- [ ] **Step 1: 确认业务无 `ui/field` 引用**

Run: `grep -rn 'from "@/components/ui/field"' frontend/app frontend/components --include="*.tsx"`
Expected: 仅 `frontend/app/ui-check/page.tsx:131` 一处。

- [ ] **Step 2: 跑三项校验**

Run: 同 Task 1 Step 2。
Expected: 全绿。

- [ ] **Step 3: 全页目检 + 报完工**

整页刷新 `/ui-check` 全滚一遍，确认 7 种说明齐、41 种保持展示、`Section` 顺序未动。不提交以外的文件，`git status` 只剩本 plan 的三个提交（加 spec 的那次修正）。
