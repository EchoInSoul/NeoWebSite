# NeoBlog 后台侧栏面板状态化（子侧栏复用件 + 设置入口）

**日期**：2026-10-15 · **范围**：纯前端，后端零改动 · **实施计划**：`docs/superpowers/plans/2026-10-15-neoblog-sidebar-panel-state.md`

---

## 1. 意图与背景

**现状（实读）**：主侧栏导航区在 `/console` 首页整片空白。原因是面板内容完全由 URL 决定——`components/console/console-sidebar-nav.tsx:30-32` 在路径不是 `/console/uc` 打头时 `return null`，一条都不渲染；而 `ui/sidebar.tsx:374-381` 的 `SidebarContent` 是 `flex-1`，容器撑满剩余高度，内容空了整片就是白的，只有 footer 贴在底部。点左下角头像里的「个人设置」是真跳转：`console-user-menu.tsx:110` 是 `<Link href="/console/uc">`，`app/console/uc/page.tsx:4` 再 redirect 到 profile，跳转后同一个 `SidebarContent` 里换成另一套条目。看起来像"进了一个新侧栏"，实质是同一个容器换了内容。

**本轮要什么**：

1. 主侧栏新增一个「设置」入口项，进入一个子侧栏；子侧栏与页面都**只做占位**，不接任何真实系统设置内容。
2. 把「返回行 + 分组标签 + 子项列表」这个形状抽成可复用组件，以后加二级侧栏不再重写实现。
3. 面板的显示改由组件内 state 掌管，URL 只作同步，不作唯一真源。

**与上一轮的关系**：2026-10-14 做过一版带 ±48px 对滑动画的子侧栏（`4f167bc` / `b1f27d9` / `4da316e`），已被 reflog `HEAD@{17}: reset: moving to 0ca0447` 甩掉，当前不在 `development` 上（`git branch --contains 4da316e` 为空、组件目录无该文件、DEVLOG 无该节）。本轮**不捞旧提交，只参考其形状，且动画整条作废**。

---

## 2. 已定决策

| # | 决策 | 口径 | 出处 |
| --- | --- | --- | --- |
| D1 | 状态模型 | state 为真源、URL 只同步：`displayedPanel` 由 `useState` 持有；仅当 `routePanel` 自身变化（前进后退 / 直接访问 / 刷新）时才接管显示 | 用户选 A 档：「URL 保留，显示由 state 定」 |
| D2 | 返回语义 | settings 面板的返回是**按钮**，只改 state、URL 不动；uc 面板的返回**保持 Link** 不改 | 上轮既定 + 本轮"最小化" |
| D3 | 动画 | **整条不做**：无绝对定位叠层、无新 transition 常量、无 `outgoingPanel` / `setTimeout` / `onTransitionEnd`；面板切换即显隐 | 用户原话「整个侧边栏先不做动画！不添上位移，最小化实现」 |
| D4 | 复用件边界 | `console-sidebar-section.tsx` 只渲染，**不持有面板可见性状态**，返回走 `onBack` 回调上报；props 全必填 | 组件规范 §4.3 |
| D5 | 设置内容 | 只要「设置」入口 + 一个占位子项「常规」+ 一页只有标题 | 用户上轮口径「我只要一个设置的占位项即可！不要真页」 |
| D6 | 落地方式 | 重写，不 `cherry-pick` 被甩的三笔 | 用户选"重新写" |
| D7 | 面板数量 | 三个：`main` / `uc` / `settings`，以 `Record` 注册表声明 | D5 范围 |

---

## 3. 范围

**改动文件（5 个）**：

| 文件 | 动作 | 职责 |
| --- | --- | --- |
| `frontend/components/console/console-sidebar-section.tsx` | 新建 | 复用展示件：返回行 + 分组标签 + 子项列表 |
| `frontend/components/console/console-sidebar-nav.tsx` | 重写 | 面板状态编排 + 注册表 + 纯判定函数 |
| `frontend/app/console/settings/general/page.tsx` | 新建 | 「常规」占位页，只有标题 |
| `frontend/messages/zh-CN.json` | 改 | 加 2 个键 |
| `frontend/messages/en.json` | 改 | 加 2 个键 |

**不动清单**：`lib/sidebar-transition.ts`（不加滑动常量）、`console-sidebar.tsx`、`console-sidebar-footer.tsx`、`console-user-menu.tsx`、`components/ui/sidebar.tsx`、后端全部。

---

## 4. 设计条文

### 4.1 分层与依赖方向

单文件内三层，依赖只能自上而下：

```
纯判定层      resolvePanel(pathname)          无 React、可独立推理
   ↓
状态编排层    ConsoleSidebarNav               持有 displayedPanel，产出「当前该显示哪个面板」
   ↓
注册表        PANEL_CONTENT                  面板 key → 返回契约 + 分组名 + 子项
   ↓
展示层        ConsoleSidebarSection          只认 props，不知道有"面板"这回事
```

`PANEL_KEYS` 是唯一的枚举源；`resolvePanel` 的返回类型由 `keyof typeof PANEL_PREFIXES` 推导，不重复声明 union——加一个面板只改两处字面量，类型自动跟上。

### 4.2 契约

```ts
// 导航子项（className 为布局入参，供 `mb-1` 这类间距，不给视觉默认值）
type ConsoleSidebarSectionItem = {
  href: string;
  label: string;
  icon: ReactNode;
  className?: string;
};

// 返回行两态（D2 的落点）：行为自带在分支里，不留永不触发的回调
type ConsoleSidebarBack =
  | { kind: "link"; href: string }              // uc 面板：真跳转
  | { kind: "button"; onClick: () => void };    // settings 面板：只上报

type ConsoleSidebarSectionProps = {
  title: string;          // 返回行的文字（面板名）
  tooltip: string;        // 折叠态 tooltip
  back: ConsoleSidebarBack;
  groupLabel: string;
  items: ConsoleSidebarSectionItem[];
};
```

注册表类型：

```ts
type ConsoleSidebarPanelContent = {
  title: string;
  tooltip: string;
  groupLabel: string;
  back: ConsoleSidebarBack;
  items: ConsoleSidebarSectionItem[];
};
const PANEL_CONTENT: Record<Exclude<PanelKey, "main">, ConsoleSidebarPanelContent>
```

`main` 面板不进注册表——它不是子侧栏，没有返回行，直接由 `<SidebarGroup>` + `<SidebarMenu>` 渲染唯一的「设置」入口项。

**面板清单（本轮终态）**：

| 面板 | 返回形态 | 分组名 | 子项 |
| --- | --- | --- | --- |
| `main` | 无 | 无 | 仅「设置」一项（进 settings 面板） |
| `uc` | `link → /console` | `sidebar.account` | 个人信息 `/console/uc/profile`、登录会话 `/console/uc/session` |
| `settings` | `button` | `sidebar.settings` | 常规 `/console/settings/general` |

`uc` 面板两个子项之间的 `className="mb-1"` 是 `31fac91` 为高亮贴合加的修复，注册表形态下必须一并保留，落在「个人信息」子项的 `className` 上（见 §4.4）。

### 4.3 状态与同步规则

```ts
const routePanel = resolvePanel(pathname);
const [displayedPanel, setDisplayedPanel] = useState<PanelKey>(routePanel);
const [syncedRoutePanel, setSyncedRoutePanel] = useState<PanelKey>(routePanel);

if (routePanel !== syncedRoutePanel) {
  setSyncedRoutePanel(routePanel);
  setDisplayedPanel(routePanel);   // 路由自己变了 → 路由接管显示
}
```

四条规则：

- **初值即路由面板**（两个 state 都用 `routePanel` 初始化）。若初值写死 `"main"`，SSR 首屏会先渲染主栏再纠正，与 `/console/uc/profile` 的客户端表现不一致 → hydration 风险。
- **同步只在 `routePanel` 变化时发生**，用"渲染期比对上一个已同步值"而不是 `useEffect`，避免多一帧的旧面板。
- **点击不改 URL 的面板切换**只走 `setDisplayedPanel`（settings 的返回按钮）。
- **点击既改 URL 又改显示**的入口（主栏「设置」项）：`render={<Link href="/console/settings/general" onClick={() => setDisplayedPanel("settings")} />}`。URL 与 state 同时前进，刷新/深链仍然正确（D1 的 A 档语义）。

不做 `dismissedPanel`。对照项目 `D:\protect\NextJs_Blog\components\console\console-sidebar-shell.tsx:100-116` 用第三个 state `dismissedPanel` 加 `resolveSidebarPanelDismissal` 表达"返回但 URL 没退"；本模型里 `displayedPanel !== routePanel` 本身已经表达了同一事实，再加一态是冗余。

### 4.4 复用件规则

- **高亮**：`pathname === item.href || pathname.startsWith(`${item.href}/`)`（沿用 `console-sidebar-nav.tsx:34-39` 既有判据）。
- **折叠态**：返回行文字沿用 `SIDEBAR_LABEL_TRANSITION` + `sidebarLabelFade(isCollapsed)`；子项沿用现状直接 `<span>{label}</span>`，不额外加淡出。折叠淡出是侧栏 `collapsible="icon"` 的既有机制，不在 D3 作废范围内。
- **样式写法**：一律 `cn()`，禁止字符串拼接（组件规范 `2026-10-13-neoblog-component-conventions-design.md` §4.4）。
- **props 写法**：全必填、不给视觉默认值（同规范 §4.3 禁「全可选带默认值」）。`items` 子项多一个可选 `className`，用于 `mb-1` 这类间距，属布局入参而非视觉默认值。

### 4.5 主栏与占位页

- 主栏在 `main` 面板只渲染「设置」一项（图标 `Settings`）。这同时修掉 §1 的空白现象：`/console` 首页的导航区不再 `return null`。
- **「设置」项的选中判据必须显式定义**（否则 B1 的错位窗口会出现"面板已退回、入口却不高亮"的歧义）：`isActive = displayedPanel === "main" && routePanel === "settings"`。即仅当"URL 在 settings 里、但面板已经退回主栏"时点亮，表示"从这里能回到刚才那栏"。其余情况（URL 不在 settings）一律不亮。
- `frontend/app/console/settings/general/page.tsx` 只渲染 `<h1>常规</h1>`，容器沿用现有控制台页的 `mx-auto flex w-full max-w-[1000px] flex-col gap-4 p-4 min-[1200px]:p-6`。
- `/console` 首页自身仍 `return null`（`app/console/page.tsx:12-14`），本轮不改；点「设置」后内容区直接进 `/console/settings/general`，用户不会落在空白首页上。

### 4.6 文案键

`sidebar.settings`、`settings.general` 两个键，`zh-CN.json`（"设置" / "常规"）与 `en.json`（"Settings" / "General"）各一份。

---

## 5. 边界条件与已知风险

诚实登记，不藏：

- **B1 返回后 URL 与面板错位**。点 settings 面板返回 → 面板回 `main`，URL 停在 `/console/settings/general`。此时**刷新会回到 settings 面板**（`resolvePanel` 只看 URL，state 不持久）。该错位窗口只在「点了返回但没刷新」期间存在，用户要离开只能点已高亮的「设置」或直接操作浏览器。这是 D1（URL 只同步）+ D2（返回不改 URL）合取的固有结果，对照项目同样如此（它的 `handleBackToMain` 也只 `setDismissedPanel`）。
- **B2 返回再进入不产生新历史**。高亮的「设置」项点击时 URL 相同，不新增历史记录、前进后退栈不变，仅 `displayedPanel` 变。
- **B3 uc 与 settings 两套返回语义并存**。uc 走链接、settings 走按钮（D2），是"最小化 + 尊重上轮口径"换来的不一致，登记为债务：将来若把 uc 接进同一套，需要 §4.3 提到的 `dismissedPanel` 类语义。
- **B4 折叠态 × 面板切换未做组合设计**。`displayedPanel` 与 `collapsible="icon"` 正交，未验证两者同时变化时的观感。
- **B5 移动端未单独设计**。面板内容挂在同一个 `SidebarContent`，Sheet 抽屉里结构一致；抽屉是否在点击「设置」后自动关闭，沿用 `console-sidebar.tsx` 现状（本轮没传 `handleMenuLinkClick` 一类的关闭回调），未实测。
- **B6 无测试运行器**。`frontend/package.json` 只有 `dev/build/start/lint/format`，全仓 `0` 个 `*.test.ts*`。引入 vitest 属于本轮未 requested 的新依赖，**不做**。防回归只靠：`eslint`、`prettier --check`、`next build` 的类型检查，加手测四条路径（进入 / 返回 / 刷新 / 后退）。因此"面板切换不变量拆在判定层与编排层"这一风险**没有自动测试兜底**。

---

## 6. 明确不做

滑动动画与叠层结构（含 ±48px）、真实系统设置内容、uc 面板接进 state、测试底座、`/console` 首页内容填充、后端与权限改动、`components/ui/` 底座改动、除「设置」外的其他导航入口。

---

## 7. 落地步骤

见 `docs/superpowers/plans/2026-10-15-neoblog-sidebar-panel-state.md`。本 spec 不执行。

---

## 8. 依据（全部为本次实读）

- 空白来源：`frontend/components/console/console-sidebar-nav.tsx:30-32`、`frontend/components/ui/sidebar.tsx:374-381`
- 头像入口与路由：`frontend/components/console/console-user-menu.tsx:110`、`frontend/app/console/uc/page.tsx:4`
- 挂载链：`frontend/app/console/layout.tsx:21-26` → `frontend/components/console/console-sidebar.tsx:12-23`
- 折叠淡出常量：`frontend/lib/sidebar-transition.ts:4-11`
- 高亮贴合修复：`git show 31fac91`（`console-sidebar-nav.tsx` 的 `mb-1`）
- 组件规范：`docs/superpowers/specs/2026-10-13-neoblog-component-conventions-design.md` §4.3 / §4.4
- 对照项目的 dismissal 模型：`D:\protect\NextJs_Blog\components\console\console-sidebar-shell.tsx:96-116`、`:184-190`
- 被甩提交：`git reflog` `HEAD@{17}`、`git show --stat 4f167bc` / `b1f27d9` / `4da316e`
- 测试底座缺失：`frontend/package.json:4-10`、`find frontend -name "*.test.ts*"` 为空
