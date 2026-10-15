# NeoBlog 控制台「用户组」页（表格视图 + 编辑弹窗，接 access 接口）

**日期**：2026-10-15 · **范围**：纯前端（新页面 + 侧栏入口 + i18n + `SessionUser` 权限解析；后端零改动） · **实施计划**：`docs/superpowers/plans/2026-10-15-neoblog-console-groups.md`（已同步为 v2，与本 spec 一并待审）

**状态**：v3（2026-10-16 实施记录）。v2 定稿的表格视图先以**纯 UI** 落地（b5ad652，空壳数据），随轮按用户点名修订两处——页面落**设置子项** `/console/settings/groups`（顶层 `/console/groups` 作废）、**删搜索框**（不做本地过滤）；2026-10-16 用户点名「开始执行」，接线轮按本 spec 接通真接口（读 / 建 / 改 / 删 / 门控 / 错码全落地，未提交）。请重点看 §2 的 D2–D4 与 D20–D22（表格视图、工具栏、行操作菜单）、D6 / D7（矩阵与联动）与 §7 文案表。

---

## 1. 意图与背景

用户 2026-10-15 点名：给前端做「权限组的 UI 编辑」，可参照 D 盘 HaoBlog 的实现原理。对话中已点头的口径：

- **范围**：只做「用户组」一页（组列表 + 组编辑弹窗、新建、删除）；「给用户改组 / 加额外权限」留给后续「用户」页轮。
- **做法**：直接接真接口（后端 access 8 条接口自 0.5.0 起就绪），不做「纯 UI 假数据」过渡。
- **列表形态（v2 定稿）**：先做了纯 HTML 演示并数轮打磨；用户定稿为**会话页同款表格视图**——工具栏（刷新 / 新建）+ 表头（组名 / 成员数 / 权限点数 / 操作）+ 行操作 **⋯ 下拉菜单**（编辑 / 删除）。原「设置页式行列表」方案作废；**搜索框后按用户点名移除**。

后端实读现状（本轮的消费面，全部已存在）：

| 接口 | 守卫 | 用途 |
| --- | --- | --- |
| `GET /api/v1/admin/permissions` | `system.groups.view` | 权限清单 `{items:[{key,name}]}`（现 8 键：users / settings / register / groups × view / edit） |
| `GET /api/v1/admin/groups` | `system.groups.view` | 组列表 `{items:[{id,system_key,name,is_preset,member_count,permissions}]}`，按 created_at 升序；`permissions` 已展开；预设组权限由代码现算（owner/admin=全量、user=空） |
| `POST /api/v1/admin/groups` | `system.groups.edit` | `{name,permissions}` → 201 组对象 |
| `PATCH /api/v1/admin/groups/:id` | `system.groups.edit` | `{name?,permissions?}`（permissions 整份替换）→ 200 组对象 |
| `DELETE /api/v1/admin/groups/:id` | `system.groups.edit` | → `{id,deleted:true}` |

关键行为（实读 `service.go` / `apierror.go`）：预设组 PATCH/DELETE → 409 `preset_group_locked`；组名空或超 64 字 → 400 `invalid_request`；重名 → 409 `group_name_taken`；有成员删除 → 409 `group_not_empty`；授出自己没有的权限 → 403 `grant_forbidden`；非法权限键 → 400 `invalid_permission`。错误信封只有 `{error:{type,code,param}}` 无文案，前端按 code 映射 i18n。

前端实读现状：

- 控制台无任何用户组 / 用户页面；侧栏 main 面板目前只有「设置」一项（`components/console/console-sidebar-nav.tsx`）。
- `SessionUser`（`lib/auth-session.ts`）未解析 `/users/me` 已下发的 `permissions` 字段——按钮显隐 / 入口显隐需要它，本轮补上。
- 可复用件（全部实读）：`ui/dialog`、`ui/alert-dialog`（`AlertDialogAction` 可传 `variant="destructive"`）、`ui/checkbox`（Base UI，支持 `indeterminate` 三态）、`ui/dropdown-menu`（行操作菜单）、列表三件套 `ConsoleListTable`（含排序表头 / 行操作槽 / `emptyState`）/ `ConsoleListToolbar`（搜索槽 + 刷新按钮）/ `ConsoleListSearchInput` / `ConsoleListTitleBlock`（会话页同款，`sessions-panel.tsx` 是用法参考）、`Card size="none"`、`lib/api.ts`（`apiRequest` / `ApiError` / `looksLikeServiceFailure`；401 自动触发全局会话失效弹窗）、sonner toast。

借鉴与规避（D 盘原理）：

- **抄**：`edit`⇒`view` 的勾选联动（纯状态联动、不禁用）；预设组「编辑入口保留、弹窗只读 + 一句解释」；按资源分组的勾选矩阵；列表里的成员数 / 权限点数概览（我们落成两列数字）；删除二次确认。
- **规避**：前端硬编码权限清单（我们清单从接口拿，资源 / 动作列动态生成）；按名字判定（我们认 `system_key` / `is_preset` 字段）；漏映射错误码（我们逐码映射，见 §6）；「删除有成员」等提交后才报错（我们用行上的 `member_count` 提前禁用）。

## 2. 拟定决策（待审阅）

| # | 决策 | 口径 |
| --- | --- | --- |
| D1 | 入口与路由 | 页面落**设置子项** `/console/settings/groups`——侧栏「设置」子面板第三项（「常规」/「用户注册」之后；标签复用既有 `settings.groups`，图标 `Users`）；按 `canViewGroups` 显隐。原「顶层页面 + main 面板入口」口径作废（2026-10-16 用户点名并入设置区） |
| D2 | 列表形态 | **会话页同款表格视图**：复用 `ConsoleListTable` + `ConsoleListToolbar`（对齐 `sessions-panel.tsx` 用法）；**无搜索框**（2026-10-16 用户点名移除）；不做分页 / 批量选择（组少） |
| D3 | 展示排序 | 默认（未点表头）＝ 预设组按 `owner → admin → user` 在前、自建组保持接口顺序；点「成员数 / 权限点数」表头 → 按该字段**本地排序**（预设分区不再生效），再点切换升降；「组名」列不排序 |
| D4 | 行内容 | 单元格：`图标块 + 组名 +「预设」Badge（仅预设）` ｜ `成员数`（接口原值）｜ `权限点数`（2026-10-16 起只计矩阵可见资源的权限数）；操作列 = 右对齐单个 **⋯ 图标按钮**（`ellipsis`），点开 `ui/dropdown-menu`（`align="end"`）：**编辑**；**删除**（destructive，仅自建组渲染；`member_count > 0` 时该项 `disabled` + title 提示）。行本身不可点；任一请求在途（`isListLocked`）时 ⋯ 菜单与「新建」按钮禁用 |
| D5 | 编辑形态 | 弹窗（`ui/dialog`），新建 / 编辑共用同一组件；宽度 `sm:max-w-md`（实施可微调） |
| D6 | 权限矩阵 | 行 = 资源、列 = 动作，**全部由清单动态生成**（2026-10-16 用户点名后：按侧栏分三组加小标题——设置 / 注册与登录 / 权限组，组内行 = 常规 / 用户注册 / 用户组 × 查看 / 编辑；**「用户」行暂隐藏**，用户页未建）；某资源缺某动作渲染空格；**未列入显示分组的资源整行不渲染**（隐藏键保存时原样往返，不清不授），未知动作回退原始段。列头带三态批量勾选 |
| D7 | 勾选联动 | 勾 `edit` → 自动勾同资源 `view`；取消 `view` → 连带取消同资源 `edit`；取消 `edit` → `view` 保留；勾 `view` 不联动。列头批量复用同一套规则。前端仅硬编码这一条蕴含（与后端 `rbac.impliedActions` 对齐，双保险） |
| D8 | 提交口径 | 新建 `POST {name, permissions}`；编辑 `PATCH {name, permissions}` 整份提交（不做键级差异）；`permissions` = 原始勾选集合（UI 已保证 edit⇒view） |
| D9 | 预设组表现 | 行 ⋯ 菜单的「编辑」保留可点 → 弹窗整体只读（2026-10-16 用户点名：只读提示文案已删）；菜单不渲染「删除」项；服务端 409 锁定为最终防线 |
| D10 | 删除 | AlertDialog 二次确认（`AlertDialogAction variant="destructive"`，描述「删除后该组立即失效，无法恢复。」）；`member_count > 0` 时⋯ 菜单删除项 `disabled` + title 提示（不等提交报错） |
| D11 | 脏拦截 | 弹窗内容（组名 / 勾选）与打开时基线不同时，关闭（X / 遮罩 / Esc / 取消）先弹「放弃未保存的修改？」确认（AlertDialog，destructive「放弃」） |
| D12 | 权限门槛 | 页面需 `system.groups.view`（403 → 页面显示「无权查看用户组」，不渲染列表）；`system.groups.edit` 控制「新建」按钮与操作列显隐；侧栏入口按 view 显隐。前端显隐不是安全边界，服务端 403 兜底 |
| D13 | 读路径 | 服务端直读（`getAdminGroups` + `getPermissionCatalog`，cookie 转发 + `cache:"no-store"`），props 下传；不做客户端 useEffect 拉取（对齐 settings-general-wiring 口径） |
| D14 | 写后同步 | 用响应对象就地更新本地列表（create 追加 / patch 替换 / delete 移除），**不调 `router.refresh()`**（本页唯一服务端数据就是这份列表） |
| D15 | 错误文案 | 逐码映射（§6），一个码都不许落通用文案（D 盘漏 `grant_forbidden` 的教训） |
| D16 | i18n | 新命名空间 `groups`（zh-CN + en 同步）；侧栏标签加进 `sidebar` |
| D17 | `SessionUser` 前置改造 | 增 `permissions: string[]`，解析 `/users/me` 已下发字段；形状坏维持既有 malformed 抛错口径 |
| D18 | 本轮不做 | 用户列表 / 改组 / 额外权限 / 成员管理入口；分页 / 批量选择；权限点描述文字；拖拽；侧栏面板化。均为插槽（§10） |
| D19 | 工程口径 | 零新依赖、零后端改动、无版本进位（前端轮惯例，同 f512409） |
| D20 | 工具栏 | 左 = 空（**无搜索框**）；右 = 「新建用户组」（`canEdit` 才渲染，放 `end` 槽）+ 刷新按钮（组件内置、渲染在 `end` 槽之后，实际排布 [新建][刷新]；点了**客户端重读** `GET /admin/groups`，对齐会话页 `refreshSessions` 口径） |
| D21 | 表格实现 | 直接复用会话页三件套组件（不改源码）；行操作菜单用 `ui/dropdown-menu`（`DropdownMenuTrigger` 的 `render=` 用法对齐 `console-user-menu.tsx`）；`canEdit=false` 时不传 `rowActions` / `actionsHeader`（操作列整列不渲染） |
| D22 | 空态 | 列表空 → 表格区显示居中灰字「暂无用户组」（`emptyState` 只替换表格，工具栏保留）；接线后列表数据恒 ≥3 行（3 预设），空态仅覆盖异常情形 |

## 3. 范围（文件清单）

| 文件 | 动作 | 职责 |
| --- | --- | --- |
| `frontend/lib/auth-session.ts` | 改 | `SessionUser` 增 `permissions: string[]` + 解析 |
| `frontend/lib/groups.ts` | 新 | **纯形状**（零 import）：类型、`parseAdminGroups` / `parsePermissionCatalog`（畸形回 null）、矩阵推导（资源 / 动作分组）、联动规则、列三态、展示排序、错误码→文案键映射 |
| `frontend/lib/admin-groups.ts` | 新 | 服务端读：`getAdminGroups()` / `getPermissionCatalog()`（cookie 转发 + `no-store` + 三态 union，对齐 `admin-settings.ts`） |
| `frontend/hooks/use-groups.ts` | 新 | 客户端状态：列表、刷新（`refreshGroups`）、busy 锁、create / update / delete（toast 映射、401 静默、DELETE 404 视为成功）、删除确认目标（对齐 `use-sessions.ts` 模式） |
| `frontend/app/console/settings/groups/page.tsx` | 新 | 服务端壳：`max-w-[1000px]` 页壳 + h1 + 读两接口（403 / 失败文案）+ canEdit |
| `frontend/app/console/settings/groups/groups-panel.tsx` | 新 | 客户端：表格 + 工具栏（刷新 / 新建）+ ⋯ 行菜单 + 挂编辑弹窗与删除确认 |
| `frontend/app/console/settings/groups/group-dialog.tsx` | 新 | 客户端：组名 + 权限矩阵 + 联动 + 只读态 + 脏拦截 + footer |
| `frontend/components/console/console-sidebar-nav.tsx` | 改 | 设置子面板加「用户组」项（活跃态、`canViewGroups` 显隐） |
| `frontend/components/console/console-sidebar.tsx` | 改 | 透传 `canViewGroups` |
| `frontend/app/console/layout.tsx` | 改 | 计算 `canViewGroups` 下传 |
| `frontend/messages/zh-CN.json` / `en.json` | 改 | `sidebar` 分组标签两键（`signInAndRegister` / `permissionGroups`）+ `groups` 命名空间（§7）；侧栏子面板项标签沿用 `settings.*` |
| `docs/DEVLOG.md` | 改 | 顶部倒序登记本轮（实施时） |

**不动清单**：`backend/**`（零改动）；`components/ui/**` 与 `components/console/console-list-*.tsx`（只组合、不改源码）；settings / uc 页面；现有 hooks；锁文件。

## 4. 页面行为

### 4.1 列表页（会话页同款表格视图）

```
<div 页壳 max-w-[1000px]>
  <h1「用户组」>
  <Card size="none">
    <工具栏：                          [新建用户组] [刷新]>
    <表头：组名 | 成员数 | 权限点数 | 操作>
    <行 × N>
  </Card>
</div>
```

- 工具栏（`ConsoleListToolbar`）：**无 `start` 槽（搜索框已按用户点名移除）**；`end` = 「新建用户组」（`canEdit` 才渲染）；刷新按钮组件内置（`isRefreshing` / `onRefresh`），点了**客户端重读** `GET /api/v1/admin/groups`（排序状态保留）。
- 表头 / 单元格：组名单元格 = 32px 图标块 + 组名 +「预设」Badge；「成员数」取接口原值、「权限点数」只计矩阵可见资源（2026-10-16 起，与弹窗可勾数量一致）；「操作」列右对齐单个 ⋯ 按钮。
- 排序：默认无排序（＝ D3 预设分区序）；点「成员数 / 权限点数」表头切换降→升（箭头指示）。
- 空态：列表空 → 表格区显示「暂无用户组」（工具栏保留）。
- ⋯ 菜单（`ui/dropdown-menu`，`align="end"`）：**编辑** → 打开弹窗（基线 = 该行数据）；**删除**（destructive，仅自建组渲染；`member_count > 0` 时置灰 + title 提示）→ 二次确认 → `DELETE` → 成功移除该行 + toast。
- busy 锁：任一请求（含刷新）在途时行菜单 / 提交按钮 / 工具栏禁用（参照 `use-sessions` 的 `isListLocked` 思路）。
- 读失败两态（对齐 settings 页口径，不渲染列表）：403 →「无权查看用户组」；其余失败 →「用户组读取失败，请刷新重试」。
- canEdit：页面里再取一次 `getAuthenticatedUser()`（React `cache` 去重；该调用抛错时按 `false` 处理——layout 的兜底分支下页面仍会渲染）；`canEdit=false` 时不渲染「新建」按钮与操作列（整列不出现）。

### 4.2 编辑弹窗

```
新建用户组 / 编辑用户组
┌──────────────────────────────┐
│ 组名  [内容编辑            ]  │
│                              │
│           查看   编辑         │
│ 用户       ☐     ☐           │
│ 用户组     ☑  ☐（列头三态）   │
│ 设置       ☑     ☑           │
│ 勾选「编辑」会自动勾选「查看」。│
│                 [取消] [保存] │
└──────────────────────────────┘
```

- 标题：新建「新建用户组」/ 编辑「编辑用户组」。
- 组名：`Input`，`maxLength=64`（对齐后端上限），占位「例如：内容编辑」；编辑态预填 `group.name`。
- 权限矩阵（`Checkbox`）：
  - 列头：动作名 + 三态批量勾选（全选 / 部分=横杠 / 全不选）；点击「部分」→ 全选，点击「全选」→ 全不选；批量走 D7 同一套联动。
  - 行：资源名；单元格 = 该资源该动作的复选框；清单中不存在的组合渲染空白。
  - 每个复选框的 `aria-label` 用翻译文案（资源+动作），**不硬编码中文**（会话页组件硬编码中文的雷不复用）。
- 编辑态勾选初值 = `group.permissions`（后端已展开，不再二次展开）。
- 预设组只读态：组名输入与全部复选框 `disabled`，列头批量不渲染，footer 只留「关闭」；只读提示文案 **2026-10-16 按用户点名删除**（原按 `system_key` 分 owner·admin / user 两档）。
- 保存按钮禁用条件：`isPending || 组名 trim 为空 || 无改动`（新建基线 = 空名 + 空集）。
- 提交成功：关闭弹窗 + toast（新建「用户组已创建」/ 编辑「已保存」）+ 列表就地更新；失败：弹窗保持打开，按 §6 映射 toast。
- 脏拦截：见 D11（关闭动作被拦下时改为弹「放弃未保存的修改？」，确认才关）。

### 4.3 侧栏

- 设置子面板三项各按自己的 view 权限显隐：**「常规」`system.settings.view`、「用户注册」`system.register.view`、「用户组」`system.groups.view`**（2026-10-16 按页拆行批落地）；三者全无时 main 面板的「设置」入口也不渲染。
- 子面板按主题分三组（组标签 + 下设项）：**设置 → 常规、注册与登录 → 用户注册、权限组 → 用户组**；组间留白（后续分组标签 `mt-4`）；空组整组不渲染（2026-10-16 用户点名）。`ConsoleSidebarSection` 的 props 从单组（`groupLabel` + `items`）改为 `groups: { label, items }[]`。
- `layout.tsx` 的异常兜底分支（后端不可达）三枚布尔全按 false。
- 传参链：`layout.tsx`（三枚布尔）→ `ConsoleSidebar` → `ConsoleSidebarNav`。

## 5. 数据形状与解析口径

- `AdminGroup`：`{ id, systemKey: string | null, name, isPreset, memberCount, permissions: string[] }`（snake→camel 手工映射，同 `user-sessions.ts` 风格）。
- `PermissionCatalogItem`：`{ key, name }`。
- 解析：`null` = 畸形（对齐 `site-settings.ts` 的 null 口径）；列表项缺字段 / 类型不对 → null；写路径响应畸形 → toast 失败。
- 展示用 `permissions.length` 直接取（后端已展开，**前端不重复展开**）。
- 资源 / 动作分组：`key` 按 `.` 切分，末段 = 动作、其余 = 资源前缀；**行顺序、显示分组与显隐由前端分组表决定**（`lib/groups.ts` 的 `MATRIX_GROUPS`，与后端目录顺序解耦）；`system.users` 暂不显示（用户页未建，键仍在后端目录）。
- 列表一次性全量读取（接口无分页 / 排序参数）；**排序在前端完成**（搜索框已删）；「刷新」= 客户端重新 GET 同一接口并整体替换列表。

## 6. 错误码映射（前端唯一映射点：`lib/groups.ts`）

| code | HTTP | 场景 | toast 文案（zh） |
| --- | --- | --- | --- |
| `preset_group_locked` | 409 | 改 / 删预设组 | 系统内置组不能修改 |
| `group_name_taken` | 409 | 组名重名 | 这个组名已被使用 |
| `group_not_empty` | 409 | 删有成员的组 | 组里还有成员，先移走成员再删除 |
| `grant_forbidden` | 403 | 授出自己没有的权限 | 不能授出你自己没有的权限 |
| `forbidden` | 403 | 无管理权限 | 你没有管理用户组的权限 |
| `invalid_request` / `invalid_permission` | 400 | 名空 / 超长、非法键 | 落各自泛化失败文案（正常不可达） |
| `not_found` | 404 | 组已被删（并发） | 编辑：关闭弹窗 + 移除该行 + 「该组已不存在」；删除：视为成功（同 `use-sessions` 404 口径） |
| `unauthorized` | 401 | 会话失效 | 静默（全局会话失效弹窗接管，`apiRequest` 既有行为） |
| `service_unavailable` 等 5xx / 0 | — | 服务不可用 | `common.serviceUnavailable` |
| 其余 | — | 兜底 | 新建「创建失败，请稍后重试」/ 编辑「保存失败，请稍后重试」/ 删除「删除失败，请稍后重试」 |

## 7. i18n 键清单（新命名空间 `groups`；en 同步）

| 键 | zh-CN |
| --- | --- |
| title | 用户组 |
| noPermission | 无权查看用户组 |
| loadFailed | 用户组读取失败，请刷新重试 |
| newGroup | 新建用户组 |
| preset | 预设 |
| empty | 暂无用户组 |
| columnName / columnMembers / columnPermissions / actions | 组名 / 成员数 / 权限点数 / 操作（表头） |
| actionsMenu | `「{name}」的操作`（⋯ 按钮 aria-label / title） |
| edit / delete | 编辑 / 删除（⋯ 菜单两项） |
| deleteBlocked | 组里还有成员，先移走成员再删除（⋯ 菜单删除项禁用态的 title） |
| deleteConfirmTitle | 删除该用户组？ |
| deleteConfirmDescription | 删除后该组立即失效，无法恢复。 |
| dialogCreateTitle / dialogEditTitle | 新建用户组 / 编辑用户组 |
| nameLabel / namePlaceholder | 组名 / 例如：内容编辑 |
| permissionsLabel | 权限 |
| hintEditImpliesView | 勾选「编辑」会自动勾选「查看」。 |
| save / close | 保存 / 关闭 |
| created / saved / deleted | 用户组已创建 / 已保存 / 用户组已删除 |
| createFailed / saveFailed / deleteFailed | 创建失败，请稍后重试 / 保存失败，请稍后重试 / 删除失败，请稍后重试 |
| groupMissing | 该组已不存在 |
| presetLocked / nameTaken / notEmpty / grantForbidden / forbidden | 见 §6 文案列 |
| discardTitle / discardDescription / discard | 放弃未保存的修改？ / 关闭后这些修改会丢失。 / 放弃 |
| selectAllColumn | `全选「{action}」`（列头批量勾选 aria-label） |
| actionView / actionEdit | 查看 / 编辑（列头；未知动作回退原始段） |
| resourceUsers / resourceGeneral / resourceRegister / resourceGroups | 用户 / 常规 / 用户注册 / 用户组（行名；未列入显示分组的资源不呈现） |

侧栏子面板项标签沿用既有 `settings.*`（`settings.general` / `settings.register` / `settings.groups`）；分组小标题 2026-10-16 新增 `sidebar.signInAndRegister` / `sidebar.permissionGroups` 两键（侧栏与弹窗矩阵共用，保证一字不差）；刷新按钮文案复用既有 `common.refresh`，不入 `groups` 命名空间。实现批注：纯 UI 批入库 37 键（b5ad652）、接线批补 10 键（`noPermission` / `loadFailed` / `createFailed` / `saveFailed` / `deleteFailed` / `groupMissing` / `presetLocked` / `grantForbidden` / `forbidden` / `notEmpty`）、按页拆行批改标签（`resourceSettings` 退役 → `resourceGeneral` + `resourceRegister`）、删只读提示两键（`readOnlyPreset` / `readOnlyPrivileged` 退役），当前共 46 键、zh / en 同步。

en 侧全部直译（本页无 ICU 复数键）。

## 8. 测试与验证

- 静态门禁（frontend 下）：`npx tsc --noEmit`、`eslint` 0 error（存量 `sonner` warning 不算）、`prettier --check`。
- 纯形状断言（`lib/groups.ts`；脚本放台账工作区、不入库；先 RED 后 GREEN）：解析畸形面；矩阵推导（分组 / 列 / 空格）；联动三规则 + 批量复用；列三态；展示排序稳定分区；错误映射全表逐一命中。
- 变异自检（逐条改坏确认会红再还原）：① 删「取消 view 连带清 edit」→ 红；② 错误映射删 `grant_forbidden` → 红；③ 预设排序去掉 → 红。
- 运行期验收归用户实机（需管理员账号；冒烟账号 `gate-check` 是普通用户，命中 403 路径可用它验「无权」态）：建组 → 表格出现；改权限 → 重开弹窗一致；预设组只读；**点「成员数 / 权限点数」表头排序（降 / 升 + 箭头）、⋯ 菜单（预设组无删除项、有成员删除项置灰）**；重名 / 预设改的 toast；无 view 账号不见入口、直连 URL 显无权文案。
- 后端零改动 → 后端测试不涉；**不提交**（按既有约定等用户点名）。

## 9. 已知缺口与风险

- 错误码→文案、资源 / 动作标签、`edit⇒view` 蕴含，前端各存一份小映射；后端清单 / 规则变化时要同步（集中 `lib/groups.ts` 单点，风险可控）。
- 前端显隐非安全边界（服务端 403 兜底）。
- 预设组权限展示依赖后端现算；前端只读不重算（owner/admin 若后端扩展，页面自动跟上）。
- 组名 64 字上限前端只做输入截断，最终判定在后端。
- 权限点数基于后端展开值、再按矩阵可见资源过滤（「用户」行隐藏时只计可见）；后端若改「读原始」或矩阵加回隐藏资源，计数语义要跟着改。
- **排序为前端本地能力**（搜索已按用户点名移除，工具条左槽为空）：接口无相应参数、列表一次性全量返回（组数少可接受；组数变多要加服务端参数，见 §10）；排序字段守卫只认 `memberCount / permissions`。
- 列表数据恒 ≥3 行（3 预设），空态 =「暂无用户组」（仅异常情形触发）；读失败在页面层处理。

## 10. 插槽

| 插槽 | 时点 | 影响面 |
| --- | --- | --- |
| 用户页（列表 / 改组 / 额外权限） | 用户点名 | 新页 + 侧栏面板化决策（本轮不做） |
| 组页看成员入口 | 需求出现 | 行展开或跳用户页 |
| 权限点描述 / 模块显示名下沉后端 | 清单变长时 | catalog 结构小改 + 本页资源标签映射退役 |
| 分页 / 服务端搜索排序 | 组数变多时 | 接口加参数 + 复用既有 `ConsoleListPagination` |
| 侧栏面板化（用户组 / 用户子项） | 用户页轮 | `console-sidebar-nav.tsx` |

## 11. 验收标准

1. `tsc --noEmit` / `eslint` / `prettier --check` 干净；零新依赖；`backend/**` 零 diff。
2. 纯形状断言 RED→GREEN；变异自检三条逐条红后回绿。
3. 运行期（用户实机）清单全过（§8 末条）。
4. i18n zh / en 同步；侧栏入口与活跃态正确。
5. DEVLOG 顶部条目（实施时）；**不提交**（等用户点名）。

## 12. 下一步

本 spec（v3）与配套实施计划（v2，含 2026-10-16 口径修订段）为同轮活文档。时间线：2026-10-15 表格视图定稿 → 纯 UI 版落地（b5ad652：模型 / 面板 / 弹窗 / 设置子项 / i18n；两处随轮修订＝落设置子项、删搜索框）→ 2026-10-16 用户点名「开始执行」，接线轮按本 spec 落地读 / 建 / 改 / 删 / 门控 / 错码六条链路 → 同日按用户点名**权限矩阵按页拆行**（后端加 `system.register` 键、设置接口守卫按组分流、`system.settings` 行改名「常规」、表头勾选框对齐修复；后端 0.21.0）→ 再按用户点名**矩阵分三组**（照侧栏分组加小标题、组间留白；「用户」行暂隐藏；列表「权限点数」只计可见资源）→ 同日删预设组只读提示文案（`readOnlyPreset` / `readOnlyPrivileged` 两键退役）→ 提交前独立评审（只读子代理）修 busy 锁补齐与三处断言补强。各批已按 ① backend（0.21.0）② frontend ③ docs 三批入库（**未推送**）。
