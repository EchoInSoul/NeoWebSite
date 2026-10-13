# NeoBlog 前端组件规范（底座 + 业务层 + ui-check 规范站）

日期：2026-10-13
范围：`frontend/components/`、`frontend/app/ui-check/`。不管 `hooks/`、`lib/`、`i18n/`、后端全部。
状态：设计经用户逐节确认（2026-10-13 对话：第 1–4 节全部通过）。**只落字，不改代码**——后续落地另开 plan。

下列 file:line 均于 2026-10-13 在当前工作区核对过。

> **v1.1 修订（2026-10-12，按提交 `0e8a4be` 订正）**：本文原把「表单报错统一用 `auth/FieldError` + 输入框挂 `aria-invalid`」定成规则。该口径已被同日落地的实现推翻——业务表单现在**报错走 sonner，一次只弹第一条**（按字段顺序取第一个不合规项），输入框不标红、字段下方不放报错小字，`components/auth/field-error.tsx` 已作为零消费者死件删除。§1 / §2 / §4.2 / §5 / §6 / §8 相应条目已按现状改写。`ui` 版 `FieldError` 仍按原文保留不删。

> **v1.2 修订（2026-10-13，实现回填，不是新增设计）**：§4.5 补「设置列表」——通用设置页组件已随提交 `4099836` 落地（`components/console/console-settings-group.tsx` + `console-settings-row.tsx`，底座侧动了 `item` 的 `flush`/`lg`、`card` 的 `size="none"` 与外壳、`select` 弹层三处）。本节只把这些既成事实与踩过的约束登记成条文，供后面的设置页直接复用；当前**只有 ui-check 的 demo，没有接任何真实页面**。

## 1. 意图与背景

工作区现在是两套东西混放：`components/ui/` 48 个 shadcn 底座组件（Base UI 原语 + `data-slot` + 部分 `cva` 变体表），以及自建业务组件 7 个（`auth/` 2、`brand/` 1、`console/` 4）+ 页面内表单 2 个（`signin-form`、`signup-form`）。（v1.2 追加：业务层又多了设置列表两件 `console/console-settings-group.tsx`、`console-settings-row.tsx`，见 §4.5。）已查实的三处漏水：

- 自建组件三种 props 写法并存：`AuthFrame` 的槽模式（`components/auth/auth-frame.tsx:5-11`）、`ConsoleSidebar` 的透传模式（`components/console/console-sidebar.tsx:9`）、`HaoBlogWordmark` 的全可选带默认值（`components/brand/haoblog-wordmark.tsx:1-9`）。
- 同一个"字段报错"曾有**两个**实现：`components/ui/field.tsx:176-231`（`div` + `text-sm` + 多错误列表）与 `components/auth/field-error.tsx:1-11`（`p` + `text-xs font-medium` + 单 `message` + 必传 `id`），登录/注册表单当时用 `auth` 版。v1.1 现状：`auth` 版随 `0e8a4be` 删除（改口径后零消费者），业务报错统一走 `sonner`，`ui` 版仍在但业务不 import。
- 业务真实引用的底座只有 7 种（`button`、`input`、`input-group`、`label`、`sidebar`、`dropdown-menu`、`sonner`），而 `app/ui-check/page.tsx`（现 1105 行）引用了全部 48 个。若按全量写规范站，四分之三暂时没人用。

既有硬约束（本 spec 不重复造，只引用）：改动只从 `ui/` 源码下手、禁覆盖写法、禁再包一层；禁 ring 泛光、状态只用边框颜色；`--radius` 5px；`ui-check` 长期保留。

## 2. 已定决策

| 项 | 决定 |
| --- | --- |
| 分层 | `ui/` 是底座，业务层是 `components/auth`、`components/console`、`components/brand` + 页面内表单。底座只改源码；业务只许用底座拼，不许自造基础样式 |
| 表单报错 | 业务表单报错统一走 `sonner`：一次只弹**第一条**（按字段从上到下第一个不合规项），输入框不挂 `aria-invalid`、字段下方不放报错小字。`ui` 版 `FieldError` 不动源码但业务不许 import；`auth` 版已删除（v1.1） |
| 自建 props | 新增业务组件二选一：框（槽模式）或跑腿的（透传）。全可选带默认值以后不许再写；现有 `HaoBlogWordmark` 不动 |
| ui-check 升级 | 轻档：只给业务在用的 7 种加"什么时候用 / 什么时候别用"，剩下 41 个保持纯展示；`Section`/`Demo`（`ui-check/page.tsx:282-315`）保持页内本地，不升通用 |

## 3. 范围

管：`components/ui/` 的改法、`components/auth`、`components/console`、`components/brand` 的写法、`app/ui-check/` 的说明补齐。

不管：`hooks/`、`lib/`、`i18n/`、页面路由逻辑、后端全部。`HaoBlogWordmark` 现有实现不动（改它要动登录页，犯不上）。注：v1.1 起「报错走 toast」这条口径的实现落在 `hooks/use-signin.ts` / `hooks/use-signup.ts`，本文只登记口径，不约束这两个文件的写法。

## 4. 规范条文

### 4.1 底座（`ui/`）

- 底座文件是 shadcn 注册表文件（`components.json` 的 `style: "base-nova"`），`shadcn add -o` 会把上游样式原样带回。每次增改底座后必须跑 `ui-check` 目检 + `npm run format` + `npx tsc --noEmit` + `npm run lint`。
- 业务里不许用外层 `className` 硬压底座内部样式，不许新建 wrapper 包住底座。观感要改就改底座源码的 class 串 / 结构 / `cva` 变体表；全局视觉改 `:root` 令牌。
- 状态提示只用边框颜色（`focus-visible:border-ring`、`aria-invalid:border-destructive`），不许引入 `focus-visible:ring-*` / `aria-invalid:ring-*`。结构性 ring（弹层细边框 `ring-1 ring-foreground/10`、头像叠放 `ring-2 ring-background`、复位 `ring-0`）不在此限。本条约束的是**底座源码**；业务表单自 v1.1 起不再给输入框挂 `aria-invalid`（报错改 toast，见 §4.2），底座里那套 `aria-invalid:border-destructive` 样式原样留着，供仍需要就地报错的场景使用。
- 现状登记：48 个文件中 14 个引 `class-variance-authority`（`alert`、`attachment`、`badge`、`button`、`empty`、`field`、`input-group`、`item`、`marker`、`navigation-menu`、`sidebar`、`tabs`、`toggle`、`toggle-group`，其中 13 个定义 `cva()` 表），46 个带 `data-slot`（例外是 `badge.tsx` 与 `sonner.tsx`）。新底座照此二选一，不强求统一成一种。

### 4.2 表单报错（统一走 toast）

- 业务表单报错一律 `toast.error(...)`，**一次只报一条**：按字段从上到下命中第一个不合规项就停（注册页顺序 邮箱 → 用户名 → 密码 → 确认密码），并把焦点落到那一格；修好再提交才轮到下一条。后端返回的错误同样走这一条 toast（占用类落回对应字段）。逻辑落点在 `hooks/use-signin.ts`、`hooks/use-signup.ts`——本文不约束 `hooks/` 的写法，只登记这条口径（见 §3 的注）。
- 输入框不挂 `aria-invalid`，字段下方不放报错 `<p>`；密码格上方那条常驻规则提示（`text-xs text-muted-foreground`）不属于报错文案，保留。表单保持 `noValidate`，挡掉浏览器原生气泡与红框。
- `components/ui/field.tsx:176-231` 的 `FieldError` 不动源码、不删除（注册表文件），但业务不许 import 它。`ui-check` 里那块 demo（`ui-check/page.tsx:124` import、`:596` 使用）保留作展示，不作为用法范例。
- 连点提交会叠同名 toast（实测过），按用户 2026-10-12 决定**不去重**，弹几次算几次。

### 4.3 自建业务组件 props（二选一）

- **框（槽模式）**：照 `AuthFrame`（`auth-frame.tsx:5-11`）——`children` 必填 + `footer` 这类可选槽。框只管摆位置，不管里面是啥。
- **跑腿的（透传）**：照 `ConsoleSidebar`（`console-sidebar.tsx:9`）——拿 `displayName?` 转手传给里面的孩子，自己不用。不传就是没有，不假装有个值。
- `HaoBlogWordmark` 式（`haoblog-wordmark.tsx:1-9`）的"全可选带默认值"以后不许再写。理由：默认值藏在组件肚子里，调用方在外面看不见，改字换色要翻源码才知道藏了什么。

### 4.4 样式写法

- 业务组件样式只用 `cn` + Tailwind 类名。需要变体时用 `cva` 变体表（照 `button.tsx:5-38`），不许字符串拼接（`a + (x ? b : c)`）。
- 尺寸与间距优先用底座已有的档（按钮 `h-9/px-4` 系、图标按钮 `size-9` 系）；控制台尺寸（顶栏 58px、侧栏 259px、rail 52px）收在组件源码常量，不做第二处硬编码。

### 4.5 设置列表（通用设置页组件，v1.2 补）

- **分层**：业务件是 `components/console/console-settings-group.tsx`（分组）与 `console-settings-row.tsx`（行）；视觉全部来自底座，业务件里没有一行覆盖式 className。动过的底座与改动点：
  - `item`：新增 `variant="flush"`（`item.tsx:45`，`relative rounded-none`，**不写任何 border 类**）与 `size="lg"`（`item.tsx:51`，`gap-3.5 px-5 py-3.5`）；配套 lg 规则：图标 `size-5` + `text-muted-foreground`（`:91`）、说明 `text-xs leading-4`（`:148`）、内容块 `gap-0.5`（`:122`）、控件列 `max-w-70`（280px）且 **`relative z-10`**（`:161`）。
  - `card`：新增 `size="none"`（`card.tsx:8,14`，即 `[--card-spacing:0px]`）让行整行铺满；外壳同期换成 `rounded-lg border border-border`。
  - `collapsible`：注册表原件（`shadcn add`），不手写。
  - `select`：弹层默认落触发器下方（`select.tsx:65` 的 `alignItemWithTrigger=false`）、弹层描边与底色与触发器统一（`:86`）、滚动条 `bg-inherit`（`:162,:180`）。
- **用法**：
  - 分组：`<ConsoleSettingsGroup title="屏幕">…</ConsoleSettingsGroup>`——"框"（槽模式，§4.3）：`title` 必填 + `children` 槽 + 可选 `className`（`console-settings-group.tsx:6-21`）。
  - 行：`<ConsoleSettingsRow … />`——`label` 必填，其余可选：`description` / `icon` / `control` / `href` / `expandedContent` / `defaultExpanded` / `className`（`console-settings-row.tsx:22-30`）。三种形态：纯控件行（只给 `control`）、整行可点行（给 `href`，尾巴出箭头）、可展开行（给 `expandedContent`，走 `collapsible`，箭头 `rotate-90`）。props 形状按"跑腿的"走：**`label` 必填**、不给视觉默认值；`defaultExpanded` 是非受控行为默认值，不属于 §4.3 禁的"全可选带默认值"。
- **复用时的硬约束**（每条都踩过或被评审抓出，别绕）：
  1. 行只许用 `flush` 变体，且**行上不能带任何 border-color 类**。行间分隔线靠容器的 `divide-y divide-border`，而 Tailwind v4 的 `divide-*` 编译成 `:where(…)`、特异性为 0——行上任何 `border-*` 都会把分隔线吞成透明（症状：行与行之间完全没有线，`border-border` 的显式线却正常）。这也是 `item` 的 `border` 从基类挪进各变体的原因。
  2. 整行可点的行里**控件必须还能点**：覆盖层是 `absolute inset-0 z-0`，未定位的兄弟控件按绘制顺序会画在它下面，所以控件列在底座里是 `relative z-10`（`item.tsx:161`）。谁把它删了，`Button` / `SelectTrigger` / `Input` 就全点不动（`Switch` / `Checkbox` 自带 `relative` 反而能点，表现为"同排控件有的能点有的不能"）。`ui-check/page.tsx:950` 的「可点行里的控件（层级回归）」组专门守这条。
  3. 展开出来的内容**就是普通行**（直接排 `<ConsoleSettingsRow … />`），不缩进、不换底色——与对照项目 HaoBlog 一致。展开**没有动画**是有意的（对照项目也是直接显隐）。
  4. 行里的下拉要用 `Select` 且**传 `items`**（`<Select items={{ srgb: "sRGB" }}>`），否则触发器显示原始值（`srgb` 而不是 `sRGB`）——Base UI 的约定，不算 bug。
  5. 已知缺口（别依赖）：`href` 与 `expandedContent` 同时给时按可展开行处理、`href` 静默失效；展开只有非受控 `defaultExpanded`，无 `expanded`/`onExpandedChange`；行标签点击不切换控件（对照项目有 `labelFor`/`labelId`）；`description` 会落进 `<p>`（`item.tsx:143`），别传块级元素。
- **什么时候别用**：单条设置不必为它拉一整分组；控制台以外的页面要复用时，先确认那片页面也接受"整行铺满 + 行间一根线"的 `card size="none"` 观感。
- **样例**：`app/ui-check/page.tsx:800` 起的「设置列表（Win11 风格）」一节。

## 5. ui-check 轻档升级清单

保持一页结构不动，只给以下 7 种补"什么时候用 / 什么时候别用"说明块：

- `button`：6 种变体（`default` / `secondary` / `outline` / `ghost` / `destructive` / `link`）各用在哪；尺寸档 `xs` / `sm` / `default` / `lg` / `icon` 系。
- `input` + `input-group` + `label`：登录表单那套组合（`Label` + `Input` / `InputGroupInput` + 密码可见切换）；报错不在字段上，走 `sonner`（见 §4.2），输入框不标红。
- `sidebar`：控制台专用，别处不许用。
- `dropdown-menu`：用户菜单那套（访问前台 → 主题 → 语言 → 分隔线 → 退出登录）。
- `sonner`：成功 / 失败 toast（注册成功、登出失败，以及 v1.1 起的**表单报错**——一次只弹第一条，见 §4.2）。

剩下 41 个保持纯展示，用到了再补说明。`Section`（`:282-291`）与 `Demo`（`:293-315`）保持页内本地组件，不升成通用。

## 6. 明确不做

- `HaoBlogWordmark` 现有实现不动。
- `ui` 版 `FieldError` 不删、不改源码。（`auth` 版 `components/auth/field-error.tsx` 已随 v1.1 的报错改 toast 而删除，不在此列。）
- 剩下 41 个底座的用法说明现在不写。
- 不碰 `hooks/`、`lib/`、后端。

## 7. 落地步骤（给 plan 用，本 spec 不执行）

1. `ui-check` 给 7 种补说明块（纯加文案，不改组件源码）。
2. 全仓 grep 确认业务无 `from "@/components/ui/field"` 引用（预期只有 `ui-check` 一处）。
3. 跑 `npm run format` + `npx tsc --noEmit` + `npm run lint` 全绿。
4. 浏览器目检 `ui-check` 一次。

## 8. 依据

- `components/auth/auth-frame.tsx:5-11`、`components/brand/haoblog-wordmark.tsx:1-9`、`components/console/console-sidebar.tsx:9`、`components/ui/field.tsx:176-231`、`components/ui/button.tsx:5-38`、`app/signin/signin-form.tsx`（组合范例，v1.1 后全文 94 行、无 `aria-invalid`）、`app/ui-check/page.tsx:282-315`（`Section`/`Demo`）与 `:570-600`（`ui/field` 那块 demo，仅展示）；报错口径的实现在 `hooks/use-signin.ts`、`hooks/use-signup.ts`、`hooks/use-signout.ts`。
- 设置列表（v1.2 追加）：`components/console/console-settings-group.tsx:6-21`、`components/console/console-settings-row.tsx:22-38,86-101`、`components/ui/item.tsx:45,51,91,122,148,161`、`components/ui/card.tsx:8,14`、`components/ui/select.tsx:65,86,162,180`、`app/ui-check/page.tsx:800,950`。落地提交 `4099836`；同轮独立评审结论 With fixes（控件被覆盖层挡住、弹层滚动条色差两处已修）。
- ~~`components/auth/field-error.tsx:1-11`~~（v1.1 已删除）、~~`app/signin/signin-form.tsx:59-64`~~（该区间已不存在）。
- 记忆：只改 shadcn 源码（`neoblog-style-port-from-haoblog`）、禁 ring 泛光（`neoblog-no-ring-glow`）、工具链与令牌基线（`neoblog-frontend-tooling`）、`ui-check` 长期保留（同上）。
