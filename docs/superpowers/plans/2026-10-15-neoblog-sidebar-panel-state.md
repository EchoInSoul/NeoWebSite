# 侧栏面板状态化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.（本机未安装这两个 skill，见文末「执行方式」——按 Native 逐任务实现。）

**Goal:** 把后台侧栏的「哪个面板在显示」从 URL 单一真源改为组件内 state，抽出一个可复用的子侧栏展示件，并在主侧栏加一个「设置」占位入口。

**Architecture:** 单文件内三层——纯判定（`resolvePanel`）→ 状态编排（`ConsoleSidebarNav` 持 `displayedPanel`）→ 展示（`ConsoleSidebarSection` 只认 props）。面板内容以 `PANEL_CONTENT` 注册表声明。面板切换为纯显隐，**不做任何动画与位移**。

**Tech Stack:** Next.js 16.4.0（App Router，`"use client"`）、React 19.3.0、next-intl 4.14.9、lucide-react 1.55.0、既有 `components/ui/sidebar.tsx` 底座、`cn`。

**Spec:** `docs/superpowers/specs/2026-10-15-neoblog-sidebar-panel-state-design.md`（本计划按该 spec 的 D1–D7、§4.1–§4.6、§5 B1–B6 论证；两份一起读）

## Global Constraints

- 不新增任何依赖；不引入测试运行器（spec §5 B6：`frontend/package.json` 无 `test` 脚本，全仓 0 个 `*.test.ts*`）。
- 不加滑动动画：不新增 `translate`/`transition` 常量，不引入叠层绝对定位、`outgoingPanel`、`setTimeout`、`onTransitionEnd`（D3）。
- `lib/sidebar-transition.ts` 只允许**沿用**既有 `SIDEBAR_LABEL_TRANSITION` 与 `sidebarLabelFade`（折叠态文字淡出），不得改动该文件。
- 样式一律 `cn(...)`，禁止字符串拼接；自建组件 props 全必填、不给视觉默认值（组件规范 §4.3/§4.4）。
- 不动这些文件：`console-sidebar.tsx`、`console-sidebar-footer.tsx`、`console-user-menu.tsx`、`components/ui/**`、`app/console/page.tsx`、后端全部。
- 中文文案以 `zh-CN.json` 为准，`en.json` 同键位同步；主侧栏只新增「设置」一个入口项。

## Review Focus

spec 隐含但无自动测试兜底、最可能咬人的六条，逐条在对应任务里钉检查步骤：

1. **直接访问 `/console/settings/general` 首屏必须就是 settings 面板**，不能先渲染主栏再纠正（hydration 一致）→ Task 3。
2. **返回后刷新会回到 settings 面板**（B1 的错位窗口），这是设计结果不是 bug，必须写进 DEVLOG「已知缺口」→ Task 3 行为 + Task 4 文档。
3. **uc 栏两项高亮贴合**（`31fac91` 的 `mb-1`）在注册表形态下最容易丢 → Task 2/3。
4. **折叠态进子面板**：返回行文字要随 `sidebarLabelFade` 淡出、tooltip 仍可及 → Task 2。
5. **`en.json` 漏键不会抛错，而是 `console.error` + 把键路径当文本渲染出来**（实测库源码：`use-intl/dist/esm/development/initializeConfig-CUsOI8u2.js:17-19` 的 `defaultOnError` 是 `console.error(error)`；`:95-103` 的 `getFallbackFromErrorAndNotify` 调完 `onError` 直接**返回** `defaultGetMessageFallback`，即 `joinPath(namespace, key)`，无 throw；本项目源码全文无 `onError` 覆盖）。后果是页面上出现 `settings.general` 这种字面量而不是白屏，**不会让任何校验命令变红**，所以必须靠显式的键位比对来兜 → Task 1。（本条原文写的是「会抛 `MissingMessageError`」，2026-10-15 经审查复验后更正。）
6. **移动端 Sheet 里点「设置」后抽屉是否自动关闭**spec 未定义（B5），实测并如实报告，不自作主张改 → Task 3。

---

## 文件结构

| 文件 | 动作 | 唯一职责 |
| --- | --- | --- |
| `frontend/messages/zh-CN.json` | Modify | 加 `sidebar.settings` + 顶层 `settings.general` |
| `frontend/messages/en.json` | Modify | 同上，英文 |
| `frontend/app/console/settings/general/page.tsx` | Create | 「常规」占位页，只有 `<h1>` |
| `frontend/components/console/console-sidebar-section.tsx` | Create | 子侧栏展示件：返回行 + 分组标签 + 子项列表 |
| `frontend/components/console/console-sidebar-nav.tsx` | Rewrite | 面板判定 + state 编排 + `PANEL_CONTENT` 注册表 |
| `docs/DEVLOG.md` | Modify | 顶部倒序登记本轮 |

Task 2 只造零件不接线（中间态自洽：Task 1、2 完成后主侧栏行为与现状一致，Task 3 才改变行为）。

---

### Task 1: 文案键与「常规」占位页

**Files:**
- Modify: `frontend/messages/zh-CN.json`（`sidebar` 块内 `backToConsole` 之后；顶层 `sidebar` 块之后新增 `settings`）
- Modify: `frontend/messages/en.json`（同键位）
- Create: `frontend/app/console/settings/general/page.tsx`

**Interfaces:**
- Consumes: 无
- Produces: i18n 键 `sidebar.settings`、`settings.general`；路由 `/console/settings/general`

- [ ] **Step 1: 加文案键**

两份文件同位置各加两键，值逐字照抄：

`zh-CN.json` — `sidebar.settings`: `"设置"`；新增顶层块 `"settings": { "general": "常规" }`
`en.json` — `sidebar.settings`: `"Settings"`；新增顶层块 `"settings": { "general": "General" }`

`sidebar` 块内 `settings` 排在 `backToConsole` 之后（保持 `profile`/`account`/`personalInfo`/`sessions`/`backToConsole` 顺序不动）。

- [ ] **Step 2: 建占位页**

`frontend/app/console/settings/general/page.tsx`：`"use client"` 不需要，用服务端组件 + `getTranslations("settings")`（照 `app/console/uc/profile/page.tsx` 的取词方式）。只渲染一个容器 div + `<h1>`，容器类名逐字用：

```
"mx-auto flex w-full max-w-[1000px] flex-col gap-4 p-4 min-[1200px]:p-6"
```

标题文本 = `t("general")`。除标题外不放任何元素（D5：不做真页）。

- [ ] **Step 3: 校验 JSON 与类型**

Run: `cd frontend && npx tsc --noEmit && npm run format:check`
Expected: 两条都无输出、退出码 0。JSON 语法错会让 `tsc` 通过但 `format:check` 报 prettier 差异；若报差异跑 `npm run format` 后复跑。

- [ ] **Step 4: 键位齐备自检（Review Focus 5）**

Run: `cd frontend && node -e "const a=require('./messages/zh-CN.json'),b=require('./messages/en.json');console.log(a.sidebar.settings&&a.settings.general&&b.sidebar.settings&&b.settings.general)"`
Expected: 打印 `Settings` 之类的真值，非 `undefined`。

- [ ] **Step 5: Commit**

```bash
git add frontend/messages/zh-CN.json frontend/messages/en.json frontend/app/console/settings/general/page.tsx
git commit -m "feat(frontend): 侧栏设置入口的文案键与「常规」占位页"
```

---

### Task 2: 可复用子侧栏展示件 `ConsoleSidebarSection`

**Files:**
- Create: `frontend/components/console/console-sidebar-section.tsx`

**Interfaces:**
- Consumes: `components/ui/sidebar`（`SidebarGroup` / `SidebarGroupLabel` / `SidebarMenu` / `SidebarMenuButton` / `SidebarMenuItem`）、`useSidebar()`、`lib/sidebar-transition`（`SIDEBAR_LABEL_TRANSITION`、`sidebarLabelFade`）、`cn`、lucide `ArrowLeft`
- Produces（Task 3 按此签名调用，名称与类型逐字一致）:

```ts
export type ConsoleSidebarSectionItem = {
  href: string;
  label: string;
  icon: ReactNode;
  className?: string;
};

export type ConsoleSidebarBack =
  | { kind: "link"; href: string }
  | { kind: "button"; onClick: () => void };

export type ConsoleSidebarSectionProps = {
  title: string;
  tooltip: string;
  back: ConsoleSidebarBack;
  groupLabel: string;
  items: ConsoleSidebarSectionItem[];
};

export function ConsoleSidebarSection(props: ConsoleSidebarSectionProps)
```

（不写返回类型标注：`@types/react` 19 已把全局 `JSX` 命名空间挪进 `React.JSX`，照仓库其余组件的写法省略即可。）

- [ ] **Step 1: 写组件**

`"use client"`。结构照已被甩的 `4f167bc` 版本形状（`git show 4f167bc:frontend/components/console/console-sidebar-section.tsx` 可查），三点差异：**不留 `onBackToMain` 这个可能永不触发的必填回调**（改为 `back` 自带 `onClick`）；**子项 `SidebarMenuItem` 用 `item.className`**；不做任何面板/动画逻辑。

行为规则：
- 返回行 `SidebarMenuButton` 的 `render` 只在 `back.kind === "link"` 时传 `<Link href={back.href} />`；`button` 形态不传 `render`，改传 `onClick={back.onClick}`。
- `tooltip={props.tooltip}`；返回行文字 `<span>` 挂 `cn("flex-1 text-center", SIDEBAR_LABEL_TRANSITION, sidebarLabelFade(isCollapsed))`。
- 返回行尾部保留既有占位 `<span aria-hidden className={cn("size-4 shrink-0", sidebarLabelFade(isCollapsed))} />`（对齐主栏项的右侧留白）。
- 分组：`<SidebarGroupLabel className="pointer-events-none">{groupLabel}</SidebarGroupLabel>`。
- 子项高亮判据逐字沿用现状：`pathname === item.href || pathname.startsWith(\`${item.href}/\`)`。

- [ ] **Step 2: 校验**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Expected: 全绿。`tsc` 会抓到 `back` 判别联合未收窄就取 `href`/`onClick` 的错——必须用 `back.kind === "link"` 分支内取值。

- [ ] **Step 3: 折叠态类名取证（Review Focus 4）**

Run: `cd frontend && grep -c "sidebarLabelFade(isCollapsed)" components/console/console-sidebar-section.tsx`
Expected: `2`（返回行文字与尾部占位各一处；`grep` 用调用形式而不是裸名，否则 import 行也计入）。组件此刻无消费者，运行时取证留到 Task 3 Step 5。

- [ ] **Step 4: Commit**

```bash
git add frontend/components/console/console-sidebar-section.tsx
git commit -m "feat(frontend): 抽出可复用子侧栏展示件 ConsoleSidebarSection"
```

---

### Task 3: `ConsoleSidebarNav` 重写——三面板 + state 接管

**Files:**
- Rewrite: `frontend/components/console/console-sidebar-nav.tsx`
- Consumes: `ConsoleSidebarSection`（Task 2 签名）

**Interfaces:**
- Produces: 默认导出的组件名与文件路径不变（`console-sidebar.tsx:18` 仍 `<ConsoleSidebarNav />`，不改调用方）。模块内新增：

```ts
const PANEL_PREFIXES = { uc: "/console/uc", settings: "/console/settings" } as const;
type PanelKey = "main" | keyof typeof PANEL_PREFIXES;   // "main" | "uc" | "settings"
const PANEL_KEYS: PanelKey[] = ["main", "uc", "settings"];
function resolvePanel(pathname: string): PanelKey       // 纯判定，无 React
```

- [ ] **Step 1: 判定层**

`resolvePanel`：依次判 `pathname === base || pathname.startsWith(base + "/")`，`uc` 先、`settings` 次、否则 `"main"`。**必须返回 `"main"` 而不是 `null`**——现状 `console-sidebar-nav.tsx:30-32` 的 `return null` 是空白根因，本轮整条删掉。

反空转自检：改完后 `grep -c "return null" components/console/console-sidebar-nav.tsx` 期望 `0`。

- [ ] **Step 2: 编排层**

状态与同步逐字按 spec §4.3：两个 state 初值都取 `routePanel`（`useState<PanelKey>(routePanel)`），渲染期 `if (routePanel !== syncedRoutePanel)` 才接管；**不用 `useEffect`**。切换只显示/隐藏，不做动画：`PANEL_KEYS` 只用于枚举，渲染时 `switch (displayedPanel)` 出**唯一一块**内容。

- [ ] **Step 3: 注册表与三个面板**

取词与上下文先定名（表格里用的就是这四个）：`const t = useTranslations("sidebar")`、`const ts = useTranslations("settings")`、`const pathname = usePathname()`、`const { state } = useSidebar()`。

`PANEL_CONTENT: Record<Exclude<PanelKey, "main">, ConsoleSidebarPanelContent>`（类型含 `title`/`tooltip`/`groupLabel`/`back`/`items`）。取值：

| 面板 | `title` | `tooltip` | `groupLabel` | `back` | `items` |
| --- | --- | --- | --- | --- | --- |
| `uc` | `t("profile")` | `t("backToConsole")` | `t("account")` | `{ kind: "link", href: "/console" }` | `[{ href: "/console/uc/profile", label: t("personalInfo"), icon: <CircleUserRound />, className: "mb-1" }, { href: "/console/uc/session", label: t("sessions"), icon: <MonitorSmartphone /> }]` |
| `settings` | `t("settings")` | `t("backToConsole")` | `t("settings")` | `{ kind: "button", onClick: () => setDisplayedPanel("main") }` | `[{ href: "/console/settings/general", label: ts("general"), icon: <SlidersHorizontal /> }]` |

`main` 面板不进注册表，直接渲染：`SidebarMenuButton` `isActive={displayedPanel === "main" && routePanel === "settings"}`（spec §4.5 显式判据）、`render={<Link href="/console/settings/general" onClick={() => setDisplayedPanel("settings")} />}`、`tooltip={t("settings")}`、图标 `<Settings />`。

- [ ] **Step 4: 校验**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Expected: 全绿。

- [ ] **Step 5: 起 dev server 手测（Review Focus 1 / 2 / 3 / 6）**

Run: `cd frontend && npx next dev -p 3000`（后台跑；已在跑则直接复用。取 PID/停止按 [[verify-kill-background-windows-process]]）

逐条实测并记录实际值，不写「应该可以」：

1. `/console` → 导航区出现「设置」一项（不再是空白）。
2. 点「设置」→ URL 变 `/console/settings/general`，导航区显示「常规」子项，内容区显示标题「常规」。
3. 点返回 → `location.pathname` 仍是 `/console/settings/general`（用 `evaluate_script` 取，别只看截图），面板回主栏且「设置」项高亮。
4. 该状态下刷新 → 回到 settings 面板（B1 既定行为，确认后交 Task 4 登记）。
5. 浏览器后退 → 回 `/console`，面板由同步接管为 `main`。
6. 头像菜单 →「个人设置」→ uc 面板显示两项，用 `evaluate_script` 取两项 `getBoundingClientRect().bottom/top` 差值确认 `mb-1`（4px）仍在。
7. 折叠侧栏（footer 的折叠按钮）后进 settings 面板 → 返回行文字 `opacity:0` 且 tooltip 可及。
8. 视口 < 1200px 走 Sheet：点「设置」后抽屉是否自动关闭，实测并记录为未定义行为（B5）。

- [ ] **Step 6: Commit**

```bash
git add frontend/components/console/console-sidebar-nav.tsx
git commit -m "feat(frontend): 侧栏面板改 state 接管——主栏设置入口 + 子侧栏复用件接线"
```

---

### Task 4: DEVLOG 登记与收尾

**Files:**
- Modify: `docs/DEVLOG.md`（顶部 `---` 之后倒序插入新一节）

**Interfaces:**
- Consumes: Task 3 Step 5 的实测记录
- Produces: 本轮决策与已知缺口入库

- [ ] **Step 1: 写 DEVLOG 一节**

标题 `## 2026-10-15 · 侧栏面板状态化（无动画）`。必须包含：D1–D7 口径摘要；**故意不做**清单（动画整条、uc 返回仍走链接、`/console` 首页仍空白）；**已知缺口**逐条：B1 错位与「返回后刷新回到子面板」、B3 两套返回语义并存、B5 移动端抽屉未设计、B6 无自动测试兜底；Task 3 Step 5 八条里任何一条与预期不符的实测值。

- [ ] **Step 2: 全量校验**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Run: `git -C . diff --stat HEAD` 
Expected: 校验全绿；diff 只含 `docs/DEVLOG.md`。

- [ ] **Step 3: Commit**

```bash
git add docs/DEVLOG.md
git commit -m "docs(devlog): 登记侧栏面板状态化——无动画取舍与 B1/B3/B5/B6 已知缺口"
```

- [ ] **Step 4: 汇报**

正文里逐条给出：四笔提交 hash、Task 3 Step 5 八条的**实测值**（哪些眼验不了或没测，明说）。

---

## 执行方式

本机 skill 清单里没有 `subagent-driven-development` 与 `executing-plans`（只装了 brainstorming / writing-plans / systematic-debugging / using-superpowers），所以本计划按 **Native** 执行：本会话内逐任务实现、逐任务提交、Task 3 的交互项用浏览器 `evaluate_script` 取实测值，最后一次性汇报。
