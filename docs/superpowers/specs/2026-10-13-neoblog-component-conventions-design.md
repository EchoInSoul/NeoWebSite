# NeoBlog 前端组件规范（底座 + 业务层 + ui-check 规范站）

日期：2026-10-13
范围：`frontend/components/`、`frontend/app/ui-check/`。不管 `hooks/`、`lib/`、`i18n/`、后端全部。
状态：设计经用户逐节确认（2026-10-13 对话：第 1–4 节全部通过）。**只落字，不改代码**——后续落地另开 plan。

下列 file:line 均于 2026-10-13 在当前工作区核对过。

## 1. 意图与背景

工作区现在是两套东西混放：`components/ui/` 48 个 shadcn 底座组件（Base UI 原语 + `data-slot` + 部分 `cva` 变体表），以及自建业务组件 7 个（`auth/` 2、`brand/` 1、`console/` 4）+ 页面内表单 2 个（`signin-form`、`signup-form`）。已查实的三处漏水：

- 自建组件三种 props 写法并存：`AuthFrame` 的槽模式（`components/auth/auth-frame.tsx:5-11`）、`ConsoleSidebar` 的透传模式（`components/console/console-sidebar.tsx:9`）、`HaoBlogWordmark` 的全可选带默认值（`components/brand/haoblog-wordmark.tsx:1-9`）。
- 同一个"字段报错"有两个实现：`components/ui/field.tsx:176-231`（`div` + `text-sm` + 多错误列表）与 `components/auth/field-error.tsx:1-11`（`p` + `text-xs font-medium` + 单 `message` + 必传 `id`）。登录/注册表单用的是 `auth` 版（`app/signin/signin-form.tsx:64,95,99`、`app/signup/signup-form.tsx:70,86,120,153,157`）。
- 业务真实引用的底座只有 7 种（`button`、`input`、`input-group`、`label`、`sidebar`、`dropdown-menu`、`sonner`），而 `app/ui-check/page.tsx`（1078 行）引用了全部 48 个。若按全量写规范站，四分之三暂时没人用。

既有硬约束（本 spec 不重复造，只引用）：改动只从 `ui/` 源码下手、禁覆盖写法、禁再包一层；禁 ring 泛光、状态只用边框颜色；`--radius` 5px；`ui-check` 长期保留。

## 2. 已定决策

| 项 | 决定 |
| --- | --- |
| 分层 | `ui/` 是底座，业务层是 `components/auth`、`components/console`、`components/brand` + 页面内表单。底座只改源码；业务只许用底座拼，不许自造基础样式 |
| 表单报错 | 全站只用 `auth` 版 `FieldError`；`ui` 版不动源码但业务不许用，`ui-check` 里只做展示 |
| 自建 props | 新增业务组件二选一：框（槽模式）或跑腿的（透传）。全可选带默认值以后不许再写；现有 `HaoBlogWordmark` 不动 |
| ui-check 升级 | 轻档：只给业务在用的 7 种加"什么时候用 / 什么时候别用"，剩下 41 个保持纯展示；`Section`/`Demo`（`ui-check/page.tsx:282-315`）保持页内本地，不升通用 |

## 3. 范围

管：`components/ui/` 的改法、`components/auth`、`components/console`、`components/brand` 的写法、`app/ui-check/` 的说明补齐。

不管：`hooks/`、`lib/`、`i18n/`、页面路由逻辑、后端全部。`HaoBlogWordmark` 现有实现不动（改它要动登录页，犯不上）。

## 4. 规范条文

### 4.1 底座（`ui/`）

- 底座文件是 shadcn 注册表文件（`components.json` 的 `style: "base-nova"`），`shadcn add -o` 会把上游样式原样带回。每次增改底座后必须跑 `ui-check` 目检 + `npm run format` + `npx tsc --noEmit` + `npm run lint`。
- 业务里不许用外层 `className` 硬压底座内部样式，不许新建 wrapper 包住底座。观感要改就改底座源码的 class 串 / 结构 / `cva` 变体表；全局视觉改 `:root` 令牌。
- 状态提示只用边框颜色（`focus-visible:border-ring`、`aria-invalid:border-destructive`），不许引入 `focus-visible:ring-*` / `aria-invalid:ring-*`。结构性 ring（弹层细边框 `ring-1 ring-foreground/10`、头像叠放 `ring-2 ring-background`、复位 `ring-0`）不在此限。
- 现状登记：48 个文件中 14 个引 `class-variance-authority`（`alert`、`attachment`、`badge`、`button`、`empty`、`field`、`input-group`、`item`、`marker`、`navigation-menu`、`sidebar`、`tabs`、`toggle`、`toggle-group`，其中 13 个定义 `cva()` 表），46 个带 `data-slot`（例外是 `badge.tsx` 与 `sonner.tsx`）。新底座照此二选一，不强求统一成一种。

### 4.2 表单报错（只留一个）

- 全站表单报错统一用 `components/auth/field-error.tsx:1-11`：`p` + `text-xs font-medium text-destructive` + `role="alert"`，单 `message`，必须传 `id` 并与输入框的 `aria-describedby` 绑定（范例见 `app/signin/signin-form.tsx:59-64`）。
- `components/ui/field.tsx:176-231` 的 `FieldError` 不动源码、不删除（注册表文件），但业务不许 import 它。`ui-check` 里那块 demo（`ui-check/page.tsx:124,582`）保留作展示，不作为用法范例。

### 4.3 自建业务组件 props（二选一）

- **框（槽模式）**：照 `AuthFrame`（`auth-frame.tsx:5-11`）——`children` 必填 + `footer` 这类可选槽。框只管摆位置，不管里面是啥。
- **跑腿的（透传）**：照 `ConsoleSidebar`（`console-sidebar.tsx:9`）——拿 `displayName?` 转手传给里面的孩子，自己不用。不传就是没有，不假装有个值。
- `HaoBlogWordmark` 式（`haoblog-wordmark.tsx:1-9`）的"全可选带默认值"以后不许再写。理由：默认值藏在组件肚子里，调用方在外面看不见，改字换色要翻源码才知道藏了什么。

### 4.4 样式写法

- 业务组件样式只用 `cn` + Tailwind 类名。需要变体时用 `cva` 变体表（照 `button.tsx:5-38`），不许字符串拼接（`a + (x ? b : c)`）。
- 尺寸与间距优先用底座已有的档（按钮 `h-9/px-4` 系、图标按钮 `size-9` 系）；控制台尺寸（顶栏 58px、侧栏 259px、rail 52px）收在组件源码常量，不做第二处硬编码。

## 5. ui-check 轻档升级清单

保持一页结构不动，只给以下 7 种补"什么时候用 / 什么时候别用"说明块：

- `button`：6 种变体（`default` / `secondary` / `outline` / `ghost` / `destructive` / `link`）各用在哪；尺寸档 `xs` / `sm` / `default` / `lg` / `icon` 系。
- `input` + `input-group` + `label`：登录表单那套组合（`Label` + `Input` / `InputGroupInput` + 密码可见切换 + `auth/FieldError` + `aria-describedby` 绑定）。
- `sidebar`：控制台专用，别处不许用。
- `dropdown-menu`：用户菜单那套（访问前台 → 主题 → 语言 → 分隔线 → 退出登录）。
- `sonner`：成功 / 失败 toast（注册成功、登出失败）。

剩下 41 个保持纯展示，用到了再补说明。`Section`（`:282-291`）与 `Demo`（`:293-315`）保持页内本地组件，不升成通用。

## 6. 明确不做

- `HaoBlogWordmark` 现有实现不动。
- `ui` 版 `FieldError` 不删、不改源码。
- 剩下 41 个底座的用法说明现在不写。
- 不碰 `hooks/`、`lib/`、后端。

## 7. 落地步骤（给 plan 用，本 spec 不执行）

1. `ui-check` 给 7 种补说明块（纯加文案，不改组件源码）。
2. 全仓 grep 确认业务无 `from "@/components/ui/field"` 引用（预期只有 `ui-check` 一处）。
3. 跑 `npm run format` + `npx tsc --noEmit` + `npm run lint` 全绿。
4. 浏览器目检 `ui-check` 一次。

## 8. 依据

- `components/auth/auth-frame.tsx:5-11`、`components/auth/field-error.tsx:1-11`、`components/brand/haoblog-wordmark.tsx:1-9`、`components/console/console-sidebar.tsx:9`、`components/ui/field.tsx:176-231`、`components/ui/button.tsx:5-38`、`app/signin/signin-form.tsx:59-64`、`app/ui-check/page.tsx:282-315,579-583`。
- 记忆：只改 shadcn 源码（`neoblog-style-port-from-haoblog`）、禁 ring 泛光（`neoblog-no-ring-glow`）、工具链与令牌基线（`neoblog-frontend-tooling`）、`ui-check` 长期保留（同上）。
