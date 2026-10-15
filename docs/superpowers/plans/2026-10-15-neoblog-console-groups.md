# 控制台「用户组」页（组列表 + 编辑弹窗）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.（执行方式由用户点名，见文末；本计划已按本仓既有口径写明：**运行期验收归用户、提交等点名**。）

**Goal:** 把 `/console/settings/groups` 建成可用页面——**会话页同款表格视图**（工具栏：刷新 / 新建；表头：组名 / 成员数 / 权限点数 / 操作；行操作 **⋯ 下拉菜单**：编辑 / 删除）；预设组只读可看；错误按码给中文；入口按 `system.groups.view` 显隐。

**口径修订（2026-10-16 实施时）**：本计划 v2 写于纯 HTML 演示定稿时，实际执行按用户随轮点名修订两处——① 页面落**设置子项** `/console/settings/groups`（非顶层；侧栏项在设置子面板、「常规」/「用户注册」之后，标签复用既有 `settings.groups`，`canViewGroups` 传参链同 Task 3）；② **搜索框删除**（Task 3 / Task 6 中搜索相关步骤与 `searchPlaceholder` 键不执行；工具栏无 `start` 槽；空态文案「暂无用户组」）。另：Task 1 的矩阵 / 联动 / 列三态 / 排序 / 标签映射随**纯 UI 批**（b5ad652）先行落地；接线轮补齐 Task 1 剩余（`parseAdminGroup` / `parseAdminGroups` / `parsePermissionCatalog` / `groupErrorKey`）并执行 Task 2–5 全链路。纯 UI 批 37 键 + 接线批 10 键 = 47 键（zh / en 同步）。执行记录见文末「执行结果」。

**2026-10-16 追加（按页拆行批）**：权限矩阵按用户点名拆成四行（用户 / 常规 / 用户注册 / 用户组）——后端加 `system.register.view` / `.edit` 并把 `register` / `auth` 组接口换挂该钥匙（目录顺序调整为 users → settings → register → groups，0.21.0）；前端 `system.settings` 行标签「设置」→「常规」、新增「用户注册」行（i18n 48 键）、矩阵表头勾选框与列对齐修复；侧栏「常规」「用户注册」按各自 view 权限显隐（三者全无时 main 面板「设置」入口也隐藏）；设置子面板拆三个分组（设置 / 注册与登录 / 权限组）带组间留白、空组不渲染。**再追加（同日，矩阵分组批）**：弹窗矩阵照侧栏分三组加小标题 + 组间留白、**「用户」行暂隐藏**（用户页未建）、列表「权限点数」只计可见资源；分组表在 `lib/groups.ts` 的 `MATRIX_GROUPS`，与目录顺序解耦，离线断言与变异自检同步。**三追加**：删预设组只读提示文案两键（`readOnlyPreset` / `readOnlyPrivileged` 退役，48 → 46 键）。**四追加**：工具栏按钮「新建用户组」由 `end` 槽移至 `start` 槽，随后按点名改文案「新建」、档位 `sm`→`default`（32 → 36px）（右侧只留刷新按钮）。详见 DEVLOG 同日条目。

**Architecture:** 服务端直读（`lib/admin-groups.ts`，cookie 转发 + 形状校验）→ props 下传 → 客户端面板（`groups-panel.tsx`：`ConsoleListTable` + `ConsoleListToolbar`（无搜索槽）+ ⋯ 行菜单 + 弹窗 / 删除确认挂载）+ 钩子（`use-groups.ts`：列表状态、刷新、写操作）→ 弹窗（`group-dialog.tsx`：组名 + 动态权限矩阵 + 联动 + 脏拦截）。纯形状（解析 / 矩阵推导 / 联动 / 分区排序 / 错误映射）全部独立在 `lib/groups.ts`（零 import）。排序为前端本地能力。

**Tech Stack:** Next.js 16.4.0（App Router）、React 19.3.0、next-intl、sonner、Base UI（`ui/dialog` / `ui/alert-dialog` / `ui/checkbox`）、既有 `ui/item` 原语、`Card size="none"`、`lib/api.ts`。

**Spec:** `docs/superpowers/specs/2026-10-15-neoblog-console-groups-design.md`（本计划按 D1–D19 与 §4–§7 论证；两份一起读）

## Global Constraints

- 不新增依赖；不引入测试运行器；一次性断言脚本放仓库根 `.superpowers/sdd/console-groups-check/`（gitignored、在 frontend 之外）。**不要放 `frontend/.scratch/`**——它虽被 gitignore，但仍被 `npm run lint` / `npm run format:check` 扫描会让门禁红（0.20.x 轮实测，本轮脚本目录同理不入库）。
- **运行期验收归用户**：执行者不跑浏览器、不 curl 接口、不拉 SSR 页面；`tsc` / `eslint` / `prettier --check` 与离线纯函数断言照跑（用户口径，spec §8）。
- **不主动提交**：各任务的 Commit 步骤只是建议分批边界；实际提交等用户点名。
- 工作区现状（写本计划时实读）：并行轮「设置页接后端」已入库（20b302f / d93127d / 96414b0 / 8e46911，HEAD = 8e46911），工作区干净，本轮从干净基线起步。暂存仍按本计划的文件清单分批，**严禁裸 `git add -A`**。
- 权限键逐字 6 个：`system.users.view|edit`、`system.groups.view|edit`、`system.settings.view|edit`；**前端零硬编码清单**（资源 / 动作全部从 `GET /admin/permissions` 推导）；唯一硬编码的一条规则是 `edit⇒view` 蕴含（对齐后端 `rbac.impliedActions`）。
- 列表组件**只组合、不改源码**：`ConsoleListTable` / `ConsoleListToolbar` / `ConsoleListSearchInput`（对齐 `sessions-panel.tsx` 用法）；行操作菜单用 `ui/dropdown-menu`（`DropdownMenuTrigger` 的 `render=` 用法对齐 `console-user-menu.tsx`）。
- 搜索（按组名，**2026-10-16 已按用户点名移除**）与排序（成员数 / 权限点数）为**前端本地**：不进接口参数；「刷新」= 客户端重读 `GET /api/v1/admin/groups`（对齐 `refreshSessions`）。
- 错误文案按 code 映射（`lib/groups.ts` 单点）；**一个码都不许落通用文案**（`grant_forbidden` 必须在列——D 盘就漏了它）。
- 组名不做前端校验 / trim（后端负责），只做 `maxLength=64` 输入截断；预设判定一律认 `system_key` / `is_preset`，**绝不按组名判断**。
- 展示计数用后端已展开的 `permissions`（**前端不重复展开**）；提交的是原始勾选集合。
- 不动：`backend/**`、`components/ui/**`、`components/console/console-list-*.tsx`（只组合，不改源码）、settings / uc 页面、`console-floating-save-button.tsx`、`console-settings-*.tsx`、`lib/api.ts`、既有 hooks、锁文件。
- i18n 中文以 `zh-CN.json` 为准、`en.json` 同键位；**漏键不抛错**（只 `console.error` + 渲染键路径），每个加键任务必须跑键位比对自检。
- 校验命令统一在 `frontend/` 下：`npx tsc --noEmit && npm run lint && npm run format:check`。`.next` 类型过期导致 tsc 报"找不到已删除路由的模块"时，先跑 `npx next typegen`。

## Review Focus

spec 隐含但无自动测试兜底、最可能咬人的九条，逐条在对应任务里钉检查步骤：

1. **矩阵与列是动态推导**（不是写死 3×2）、缺格渲染空白、资源 / 动作标签未知时回退原始段 → Task 1 断言。
2. **联动三规则**（勾 edit 带 view；取消 view 连带清 edit；取消 edit 留 view）+ 列批量复用同一函数 → Task 1 断言 + 变异①。
3. **错误映射不漏码**：六个业务码逐一命中，其余落上下文兜底 → Task 1 断言 + 变异②。
4. **默认展示序**：未点表头时 = 预设 owner→admin→user 在前、自建组保持接口顺序；点表头后按字段本地排序（预设分区不生效）→ Task 1 断言 + 变异③ + Task 3 排序守卫。
5. **读失败绝不渲染列表**（403 →「无权查看用户组」；目录读失败 →「用户组读取失败」）→ Task 3 页面分支。
6. **`canEdit` 计算抛错按 `false`**（layout 兜底分支下页面仍会渲染，不能白屏）→ Task 3 / Task 4。
7. **预设组只读按 `system_key` 分档提示**（owner/admin 与 user 文案不同）、弹窗仍可打开看矩阵 → Task 4。
8. **脏拦截只拦用户发起的手动关闭**：`eventDetails.cancel()` + 放弃确认；保存成功与放弃确认后的程序性关闭必须直通 → Task 4。
9. **en.json 漏键不报错**（渲染键路径而非白屏）→ 每个加键任务的键位比对自检。
10. **⋯ 菜单三口径**：预设组菜单无「删除」项；`member_count > 0` 的删除项 `disabled` + title；`canEdit=false` 时操作列整列不渲染 → Task 4 / Task 5。
11. **搜索空态只替换表格区**（工具栏保留）、排序字段守卫只认 `memberCount / permissions` → Task 3。

---

## 文件结构

| 文件 | 动作 | 唯一职责 |
| --- | --- | --- |
| `frontend/lib/groups.ts` | Create | 纯形状（零 import）：类型 / 解析 / 矩阵推导 / 联动 / 排序 / 错误映射 |
| `frontend/lib/auth-session.ts` | Modify | `SessionUser.permissions` 解析 |
| `frontend/lib/admin-groups.ts` | Create | 服务端读：`getAdminGroups` / `getPermissionCatalog` |
| `frontend/app/console/settings/groups/page.tsx` | Create | 服务端壳：h1 + 读两接口 + 两分支 + `canEdit` |
| `frontend/app/console/settings/groups/groups-panel.tsx` | Create | 客户端：表格视图（T3 只读）→ T4 加工具栏新建 + ⋯ 菜单（编辑）→ T5 加菜单删除项 |
| `frontend/app/console/settings/groups/group-dialog.tsx` | Create | 客户端弹窗：组名 + 矩阵 + 联动 + 只读态 + 脏拦截（T4） |
| `frontend/hooks/use-groups.ts` | Create | 客户端状态：列表 + 刷新（T3 建）→ T4 加 create/update → T5 加 delete |
| `frontend/components/console/console-sidebar-nav.tsx` | Modify | 设置子面板加「用户组」项（活跃态 + `canViewGroups` 显隐） |
| `frontend/components/console/console-sidebar.tsx` | Modify | 透传 `canViewGroups` |
| `frontend/app/console/layout.tsx` | Modify | 计算 `canViewGroups` 下传 |
| `frontend/messages/zh-CN.json` / `en.json` | Modify | `settings.groups` 复用（侧栏标签）+ `groups` 命名空间（纯 UI 批 37 键 + 接线批 10 键） |
| `docs/DEVLOG.md` | Modify | 顶部倒序登记（T6） |

Task 3 结束时的中间态是自洽的：页面能看表格（含预设与自建组、数字列、搜索 / 排序 / 刷新），但还不能操作；T4 打通新建 / 编辑与 ⋯ 菜单；T5 打通删除。

---

### Task 1: 纯形状模块 `lib/groups.ts`

**Files:**
- Create: `frontend/lib/groups.ts`
- Create（一次性、不入库）: `.superpowers/sdd/console-groups-check/check.cjs`

**Interfaces:**
- Consumes: 无（模块零 import）
- Produces（Task 2–5 按此签名调用，名称与类型逐字一致）:

```ts
export type AdminGroup = {
  id: string;
  systemKey: string | null;
  name: string;
  isPreset: boolean;
  memberCount: number;
  permissions: string[];
};

export type PermissionCatalogItem = { key: string; name: string };

export type PermissionMatrix = {
  actions: string[];
  rows: Array<{
    resource: string;
    cells: Array<{ action: string; key: string } | null>;
  }>;
};

export function parseAdminGroups(payload: unknown): AdminGroup[] | null;
export function parseAdminGroup(payload: unknown): AdminGroup | null;
export function parsePermissionCatalog(payload: unknown): PermissionCatalogItem[] | null;
export function buildPermissionMatrix(catalog: PermissionCatalogItem[]): PermissionMatrix;
export function resourceLabelKey(resource: string): string | null;
export function actionLabelKey(action: string): string | null;
export function applyPermissionToggle(selected: string[], key: string, checked: boolean): string[];
export function togglePermissionColumn(selected: string[], keys: string[], checked: boolean): string[];
export function permissionColumnState(selected: string[], keys: string[]): "all" | "some" | "none";
export function samePermissionSet(left: string[], right: string[]): boolean;
export function sortGroupsForDisplay(groups: AdminGroup[]): AdminGroup[];
export function groupErrorKey(code: string, context: "create" | "edit" | "delete"): string;
```

- [ ] **Step 1: 写模块**

语义逐条照 spec §2 D3/D6/D7 与 §5/§6：

- **解析**（null 口径，对齐 `site-settings.ts`）：`parseAdminGroup`——`id` / `name` 为 `string`、`system_key` 为 `null | string`、`is_preset` 为 `boolean`、`member_count` 为 `number`、`permissions` 为 `string[]`，全部满足才返回 camelCase 对象，否则 `null`；多余键忽略。`parseAdminGroups` / `parsePermissionCatalog`——`payload.items` 为数组且逐项解析通过，否则 `null`。
- **矩阵**：`key` 按 `.` 切分——末段 = 动作，其余 = 资源前缀。`actions` = 清单中出现顺序去重；`rows` = 资源出现顺序去重，每行 `cells` 与 `actions` 对齐，该资源缺该动作 → `null`。资源 / 动作标签映射：`RESOURCE_LABEL_KEYS = { "system.users": "resourceUsers", "system.groups": "resourceGroups", "system.settings": "resourceSettings" }`、`ACTION_LABEL_KEYS = { view: "actionView", edit: "actionEdit" }`；查不到返回 `null`（调用方回退原始段）。
- **联动**：`IMPLIED_ACTIONS = { edit: ["view"] }`（模块内唯一硬编码规则）。
  - `applyPermissionToggle(selected, key, true)`：加入 `key`；若其动作有蕴含动作，一并加入同资源的蕴含键。
  - `applyPermissionToggle(selected, key, false)`：移除 `key`；再反向查——凡"蕴含列表包含本动作"的动作 A，移除同资源的 A 键（即取消 `view` 连带清 `edit`；取消 `edit` 只掉 `edit`）。
  - 返回新数组（不改入参、稳定去重）。
  - `togglePermissionColumn`：`keys` 为该列非空格键；**逐个复用 `applyPermissionToggle` 累积**，不得自己拼集合。
  - `permissionColumnState`：全在 `"all"`、全不在 `"none"`、否则 `"some"`（空 `keys` → `"none"`）。
  - `samePermissionSet`：忽略顺序与重复的集合相等（脏值判定用）。
- **排序**：`PRESET_ORDER = ["owner", "admin", "user"]`；稳定分区——`isPreset` 在前按 `systemKey` 在 `PRESET_ORDER` 的下标（未知 key 排已知之后、保持相对序），自建组保持输入顺序（`Array.prototype.sort` 稳定）。
- **错误映射**：`ERROR_KEYS = { preset_group_locked: "presetLocked", group_name_taken: "nameTaken", group_not_empty: "notEmpty", grant_forbidden: "grantForbidden", forbidden: "forbidden", not_found: "groupMissing" }`；`FALLBACK_KEYS = { create: "createFailed", edit: "saveFailed", delete: "deleteFailed" }`。`groupErrorKey` 命中返回映射值、否则返回兜底（`not_found` 的删除语境由调用方先按成功处理，不进映射）。

- [ ] **Step 2: 写离线断言脚本**

`.superpowers/sdd/console-groups-check/check.cjs`（先建目录；与编译产物同目录，`require("./groups.js")`），内容逐字：

```js
const assert = require("node:assert/strict");
const {
  parseAdminGroups,
  parseAdminGroup,
  parsePermissionCatalog,
  buildPermissionMatrix,
  resourceLabelKey,
  actionLabelKey,
  applyPermissionToggle,
  togglePermissionColumn,
  permissionColumnState,
  samePermissionSet,
  sortGroupsForDisplay,
  groupErrorKey,
} = require("./groups.js");

const groupFixture = {
  id: "g1",
  system_key: null,
  name: "内容编辑",
  is_preset: false,
  member_count: 2,
  permissions: ["system.settings.view"],
};

// --- 解析：camelCase 映射 + 畸形全落 null ---
assert.deepEqual(parseAdminGroups({ items: [groupFixture] }), [
  {
    id: "g1",
    systemKey: null,
    name: "内容编辑",
    isPreset: false,
    memberCount: 2,
    permissions: ["system.settings.view"],
  },
]);
assert.equal(parseAdminGroups(null), null);
assert.equal(parseAdminGroups({ items: "x" }), null);
assert.equal(parseAdminGroups({ items: [{ ...groupFixture, member_count: "2" }] }), null);
assert.equal(parseAdminGroups({ items: [{ ...groupFixture, system_key: 3 }] }), null);
assert.equal(parseAdminGroups({ items: [{ ...groupFixture, permissions: [1] }] }), null);
assert.equal(parseAdminGroups({ items: [{ ...groupFixture, is_preset: undefined }] }), null);
assert.equal(
  parseAdminGroups({ items: [{ ...groupFixture, extra: 1 }] }).length,
  1,
); // 多余键忽略
assert.equal(parseAdminGroup(groupFixture).name, "内容编辑");
assert.equal(parseAdminGroup({ ...groupFixture, permissions: null }), null);

const catalogItems = [
  { key: "system.users.view", name: "查看用户" },
  { key: "system.users.edit", name: "管理用户" },
  { key: "system.groups.view", name: "查看用户组" },
  { key: "system.groups.edit", name: "管理用户组" },
  { key: "system.settings.view", name: "查看设置" },
  { key: "system.settings.edit", name: "管理设置" },
];
assert.equal(parsePermissionCatalog({ items: catalogItems }).length, 6);
assert.equal(parsePermissionCatalog({ items: [{ key: "a" }] }), null);

// --- 矩阵：动态分组、列对齐、缺格 null、标签与回退 ---
const matrix = buildPermissionMatrix(parsePermissionCatalog({ items: catalogItems }));
assert.deepEqual(matrix.actions, ["view", "edit"]);
assert.deepEqual(
  matrix.rows.map((row) => row.resource),
  ["system.users", "system.groups", "system.settings"],
);
assert.deepEqual(matrix.rows[0].cells, [
  { action: "view", key: "system.users.view" },
  { action: "edit", key: "system.users.edit" },
]);
const partial = buildPermissionMatrix([
  { key: "system.users.view", name: "a" },
  { key: "system.users.edit", name: "b" },
  { key: "system.settings.view", name: "c" },
]);
assert.deepEqual(partial.actions, ["view", "edit"]);
assert.deepEqual(partial.rows[1].cells, [
  { action: "view", key: "system.settings.view" },
  null,
]);
assert.equal(resourceLabelKey("system.users"), "resourceUsers");
assert.equal(resourceLabelKey("system.unknown"), null);
assert.equal(actionLabelKey("edit"), "actionEdit");
assert.equal(actionLabelKey("delete"), null);

// --- 联动三规则 + 列批量复用 ---
const usersView = "system.users.view";
const usersEdit = "system.users.edit";
assert.deepEqual(
  applyPermissionToggle([], usersEdit, true).sort(),
  [usersEdit, usersView].sort(),
); // 勾 edit → 带 view
assert.deepEqual(applyPermissionToggle([usersView, usersEdit], usersView, false), []); // 取消 view → 清 edit
assert.deepEqual(applyPermissionToggle([usersView, usersEdit], usersEdit, false), [usersView]); // 取消 edit → 留 view
assert.deepEqual(applyPermissionToggle([], usersView, true), [usersView]); // 勾 view 不带 edit
assert.deepEqual(
  togglePermissionColumn([], ["system.users.edit", "system.settings.edit"], true).sort(),
  [
    "system.users.edit",
    "system.settings.edit",
    "system.users.view",
    "system.settings.view",
  ].sort(),
); // 列批量勾 edit 也带 view
assert.deepEqual(
  togglePermissionColumn(
    ["system.users.view", "system.users.edit", "system.settings.view"],
    ["system.users.view", "system.settings.view"],
    false,
  ),
  [], // 列取消 view → 连带清 edit
);
assert.equal(permissionColumnState([usersView], [usersView, "system.settings.view"]), "some");
assert.equal(permissionColumnState([], [usersView]), "none");
assert.equal(permissionColumnState([usersView], [usersView]), "all");
assert.equal(permissionColumnState([], []), "none");

// --- 集合相等（脏值判定） ---
assert.equal(samePermissionSet(["a", "b"], ["b", "a", "b"]), true);
assert.equal(samePermissionSet(["a"], ["a", "b"]), false);

// --- 排序：预设固定序、自建保输入序 ---
const makeGroup = (id, systemKey, isPreset) => ({
  id,
  systemKey,
  name: id,
  isPreset,
  memberCount: 0,
  permissions: [],
});
assert.deepEqual(
  sortGroupsForDisplay([
    makeGroup("c2", null, false),
    makeGroup("user", "user", true),
    makeGroup("c1", null, false),
    makeGroup("admin", "admin", true),
    makeGroup("owner", "owner", true),
  ]).map((group) => group.id),
  ["owner", "admin", "user", "c2", "c1"],
);

// --- 错误映射：六码逐一命中 + 兜底按上下文 ---
assert.equal(groupErrorKey("preset_group_locked", "edit"), "presetLocked");
assert.equal(groupErrorKey("group_name_taken", "create"), "nameTaken");
assert.equal(groupErrorKey("group_not_empty", "delete"), "notEmpty");
assert.equal(groupErrorKey("grant_forbidden", "edit"), "grantForbidden");
assert.equal(groupErrorKey("forbidden", "create"), "forbidden");
assert.equal(groupErrorKey("not_found", "edit"), "groupMissing");
assert.equal(groupErrorKey("invalid_request", "create"), "createFailed");
assert.equal(groupErrorKey("invalid_permission", "edit"), "saveFailed");
assert.equal(groupErrorKey("whatever", "delete"), "deleteFailed");

console.log("groups checks: PASS");
```


- [ ] **Step 3: 跑断言**

Run:
```bash
cd frontend && mkdir -p ../.superpowers/sdd/console-groups-check && npx tsc lib/groups.ts --outDir ../.superpowers/sdd/console-groups-check --module commonjs --target es2022 --skipLibCheck && node ../.superpowers/sdd/console-groups-check/check.cjs
```
Expected: `groups checks: PASS`（单文件 tsc 调用会忽略 tsconfig，flags 必须自足；这是纯函数级离线断言——不碰应用 / 浏览器 / 接口）。

- [ ] **Step 4: 变异自检①（联动）**

把 `applyPermissionToggle` 取消分支里的"反向蕴含清理"（取消 view 时连带移除 edit 的循环）删掉 → 重跑 Step 3。
Expected: FAIL——「取消 view → 清 edit」与「列取消 view → 连带清 edit」两条红（实际保留 `system.users.edit`）。还原 → 重跑绿。

- [ ] **Step 5: 变异自检②（映射）**

把 `ERROR_KEYS` 里的 `grant_forbidden` 一行删掉 → 重跑 Step 3。
Expected: FAIL——`groupErrorKey("grant_forbidden", "edit")` 期望 `grantForbidden`、实际 `saveFailed`。还原 → 重跑绿。

- [ ] **Step 6: 变异自检③（排序）**

让 `sortGroupsForDisplay` 直接返回 `groups`（去掉预设分区）→ 重跑 Step 3。
Expected: FAIL——排序断言红（实际 `c2, user, c1, admin, owner`）。还原 → 重跑绿。

- [ ] **Step 7: 静态校验**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Expected: 全绿（新模块此刻无消费者，正常）。

- [ ] **Step 8: Commit（建议边界①，等用户点名）**

```bash
git add frontend/lib/groups.ts
git commit -m "feat(frontend): 用户组纯形状模块——解析/矩阵/联动/排序/错误映射"
```

`.superpowers/` 不入库。

---

### Task 2: `SessionUser.permissions` + 服务端读 `lib/admin-groups.ts`

**Files:**
- Modify: `frontend/lib/auth-session.ts`
- Create: `frontend/lib/admin-groups.ts`

**Interfaces:**
- Consumes: Task 1 的 `AdminGroup` / `PermissionCatalogItem` / `parseAdminGroups` / `parsePermissionCatalog`
- Produces:

```ts
// auth-session.ts
export type SessionUser = { ...permissions: string[]; ... };

// admin-groups.ts
export type AdminGroupsRead =
  | { status: "ok"; groups: AdminGroup[] }
  | { status: "forbidden" }
  | { status: "failed" };
export function getAdminGroups(): Promise<AdminGroupsRead>;

export type PermissionCatalogRead =
  | { status: "ok"; catalog: PermissionCatalogItem[] }
  | { status: "failed" };
export function getPermissionCatalog(): Promise<PermissionCatalogRead>;
```

- [ ] **Step 1: `auth-session.ts` 加 `permissions`**

三处小改（其余不动）：

1. `SessionUser` 加 `permissions: string[];`（放 `group` 与 `twoFactorEnabled` 之间，与后端 dto 字段序一致）。
2. `MeResponse` 加 `permissions?: unknown;`。
3. 主校验条件补 `!Array.isArray(me.permissions) || !me.permissions.every((permission) => typeof permission === "string")` → 命中即 `throw new Error(malformedResponse)`；返回对象加 `permissions: me.permissions as string[]`。

现有消费者（`console/layout.tsx`、`uc/profile/page.tsx`、`profile-form.tsx`、`use-profile-save.ts`、`signin/signup` 页）只会多拿到一个未用字段，无感。

- [ ] **Step 2: 写 `lib/admin-groups.ts`**

照 `lib/admin-settings.ts` 的口径逐条对齐（读 `cookies()` 取 `session` → 带 cookie 转发 `fetch(`${backendUrl}/api/v1/admin/groups`, { headers: { cookie }, cache: "no-store" })`）：

- 无 cookie → `failed`；网络异常 → `failed`；`403` → `forbidden`；其它非 2xx → `failed`；2xx 但 `parseAdminGroups` 为 `null` → `failed`；通过 → `{ status: "ok", groups }`。
- `getPermissionCatalog` 同构，但**无 `forbidden` 分支**（与组列表同权限点；403 / 非 2xx / 畸形一律 `failed`，页面由组列表的三态决定文案）。路径 `/api/v1/admin/permissions`。
- `backendUrl` 从 `@/lib/backend-url` 取。

- [ ] **Step 3: 静态校验**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Expected: 全绿（`admin-groups.ts` 此刻无消费者，正常）。

取证 grep（确认解析口径没漏）:
```bash
cd frontend && grep -n "system_key\|is_preset\|member_count" lib/admin-groups.ts
```
Expected: 0 行（蛇形键名只许出现在 `lib/groups.ts` 的解析里）。

- [ ] **Step 4: Commit（建议边界②，等用户点名）**

```bash
git add frontend/lib/auth-session.ts frontend/lib/admin-groups.ts
git commit -m "feat(frontend): SessionUser 解析权限集 + 用户组服务端读取"
```

---

### Task 3: 侧栏入口 + 页面 + 表格视图（只读）

**Files:**
- Modify: `frontend/messages/zh-CN.json` / `frontend/messages/en.json`（`sidebar` 加 1 键 + `groups` 命名空间 9 键）
- Modify: `frontend/app/console/layout.tsx`（计算 `canViewGroups` + 传参）
- Modify: `frontend/components/console/console-sidebar.tsx`（透传）
- Modify: `frontend/components/console/console-sidebar-nav.tsx`（新增「用户组」项）
- Create: `frontend/app/console/settings/groups/page.tsx`
- Create: `frontend/app/console/settings/groups/groups-panel.tsx`（本任务先只读：表格 + 工具栏）
- Create: `frontend/hooks/use-groups.ts`（本任务先只有 `refreshGroups`）

**Interfaces:**
- Consumes: Task 2 的 `getAdminGroups` / `SessionUser.permissions`；Task 1 的 `AdminGroup` / `sortGroupsForDisplay`；既有 `ConsoleListTable` / `ConsoleListToolbar` / `ConsoleListSearchInput`；`lib/api.ts` 的 `ApiError` / `apiRequest` / `looksLikeServiceFailure`
- Produces（T4 / T5 再扩）:

```ts
// hooks/use-groups.ts
export function useGroups(initialGroups: AdminGroup[]): {
  groups: AdminGroup[];
  isRefreshing: boolean;
  refreshGroups(): Promise<void>;
};

// groups-panel.tsx（T4 扩签名）
export function GroupsPanel({ initialGroups }: { initialGroups: AdminGroup[] }): JSX.Element;
```

- [ ] **Step 1: 两份 messages 加键**

`sidebar` 命名空间：`groups` =「用户组」/ `"User groups"`（放 `settings` 旁）。
`groups` 命名空间（新建命名空间，放 `sessions` 旁；en 同键位）——本批 9 键，值照 spec §7：

| 键 | zh-CN |
| --- | --- |
| title | 用户组 |
| noPermission | 无权查看用户组 |
| loadFailed | 用户组读取失败，请刷新重试 |
| preset | 预设 |
| searchPlaceholder | 搜索组名 |
| empty | 没有匹配的用户组 |
| columnName / columnMembers / columnPermissions | 组名 / 成员数 / 权限点数（表头） |

en 直译（本批无 ICU 复数）。刷新按钮文案用既有 `common.refresh`，不加键。

- [ ] **Step 2: 链路传 `canViewGroups`**

1. `layout.tsx`：`ConsoleShell` 签名加 `canViewGroups?: boolean`（默认 `false`）→ `<ConsoleSidebar displayName={displayName} canViewGroups={canViewGroups} />`；主分支计算 `const canViewGroups = user.permissions.includes("system.groups.view");` 并传入；异常兜底分支不传（默认 false）。
2. `console-sidebar.tsx`：签名加 `canViewGroups = false`，透传给 `<ConsoleSidebarNav canViewGroups={canViewGroups} />`。
3. `console-sidebar-nav.tsx`：
   - 顶部加 `const GROUPS_HREF = "/console/settings/groups";`，`lucide-react` import 加 `UsersRound`；签名改 `export function ConsoleSidebarNav({ canViewGroups = false }: { canViewGroups?: boolean })`。
   - main 分支：在现有「设置」`SidebarMenuItem` **上方**新增（`canViewGroups` 为假时不渲染）：
     ```tsx
     {canViewGroups ? (
       <SidebarMenuItem>
         <SidebarMenuButton
           isActive={isUnder(pathname, GROUPS_HREF)}
           render={<Link href={GROUPS_HREF} />}
           tooltip={t("groups")}
         >
           <UsersRound />
           <span>{t("groups")}</span>
         </SidebarMenuButton>
       </SidebarMenuItem>
     ) : null}
     ```
   - 设置项 `isActive` 原样不动（`displayedPanel === "main" && routePanel === "settings"`）。

- [ ] **Step 3: `page.tsx`（本任务只读版）**

```tsx
import { getTranslations } from "next-intl/server";

import { getAdminGroups } from "@/lib/admin-groups";

import { GroupsPanel } from "./groups-panel";

export default async function GroupsPage() {
  const t = await getTranslations("groups");
  const read = await getAdminGroups();

  return (
    <div className="mx-auto flex w-full max-w-[1000px] flex-col gap-4 p-4 min-[1200px]:p-6">
      <h1 className="text-xl font-semibold tracking-tight">{t("title")}</h1>
      {read.status === "ok" ? (
        <GroupsPanel initialGroups={read.groups} />
      ) : (
        <p className="text-sm text-muted-foreground">
          {read.status === "forbidden" ? t("noPermission") : t("loadFailed")}
        </p>
      )}
    </div>
  );
}
```

（目录读取、`canEdit`、矩阵下传都在 Task 4 加——本任务不引入未使用的变量。）

- [ ] **Step 4: 写 `hooks/use-groups.ts`（refresh 雏形）**

语义（对齐 `use-sessions.ts` 的 `refreshSessions`）：

- state：`groups`（`useState(initialGroups)`）、`isRefreshing`。
- `refreshGroups()`：`if (isRefreshing) return;`；`isRefreshing = true` 包裹全程（finally 复位）；`apiRequest("/api/v1/admin/groups")` → `parseAdminGroups`：
  - 成功 → `setGroups(parsed)`；
  - 401 → 静默 return（全局会话失效弹窗接管）；
  - `looksLikeServiceFailure` → `toast.error(tCommon("serviceUnavailable"))`；
  - 其余（含解析 `null`）→ `toast.error(t("loadFailed"))`。
- imports：`useState`、`useTranslations`（`groups` + `common`）、`toast`（sonner）、`ApiError` / `apiRequest` / `looksLikeServiceFailure`（`@/lib/api`）、`parseAdminGroups` / `type AdminGroup`（`@/lib/groups`）。

- [ ] **Step 5: `groups-panel.tsx`（只读版：表格）**

客户端组件，Props `{ initialGroups: AdminGroup[] }`；结构（对齐 `sessions-panel.tsx`）：

```tsx
<Card size="none">
  <ConsoleListToolbar
    start={
      <ConsoleListSearchInput
        value={searchText}
        onValueChange={setSearchText}
        placeholder={t("searchPlaceholder")}
      />
    }
    isBusy={isRefreshing}
    isRefreshing={isRefreshing}
    onRefresh={() => void refreshGroups()}
    refreshLabel={tCommon("refresh")}
  />
  <ConsoleListTable
    rows={visibleGroups}
    columns={columns}
    getRowKey={(group) => group.id}
    sortField={sortField ?? undefined}
    sortDirection={sortDirection}
    onToggleSort={toggleSort}
    isBusy={isRefreshing}
    emptyState={
      <p className="px-4 py-8 text-center text-sm text-muted-foreground">
        {t("empty")}
      </p>
    }
  />
</Card>
```

语义逐条：

- state：`searchText`、`sortField: string | null`、`sortDirection: ConsoleListSortDirection`（默认 `"desc"`）。
- `visibleGroups`：`sortGroupsForDisplay(groups)` → 搜索非空时按组名过滤（trim + 小写、包含匹配）→ `sortField` 非空时按数值排（`sortField === "memberCount" ? memberCount : permissions.length`，方向乘 `-1`；先过滤后排序、稳定）。
- `toggleSort(next: string)`：守卫 `if (next !== "memberCount" && next !== "permissions") return;`；同字段切方向、新字段置 `"desc"`（对齐 `sessions-panel` 的 `toggleSort`）。
- columns（3 列，`ConsoleListColumn<AdminGroup>[]`）：
  - `name`：`header: t("columnName")`、`headerClassName: "px-4"`、`cellClassName: "px-4"`；render = 图标块（`size-8` 圆角块 + `UsersRound className="size-4"`，对齐会话页设备块）+ `ConsoleListTitleBlock`（`title={group.name}`、`trailing={group.isPreset ? <Badge variant="secondary">{t("preset")}</Badge> : undefined}`）；
  - `members`：`header: t("columnMembers")`、`sortField: "memberCount"`、`cellClassName: "px-4 text-xs text-muted-foreground"`、render = `group.memberCount`；
  - `permissions`：`header: t("columnPermissions")`、`sortField: "permissions"`、`cellClassName: "px-4 text-xs text-muted-foreground"`、render = `group.permissions.length`。
- 行不可点；本任务不传 `rowActions` / `actionsHeader`（T4 起接，见 Review Focus 10）。
- 工具栏排布：T4 起「新建」放 `end` 槽——组件内置顺序为 **`end` 在刷新按钮左侧**（实际排布 [新建][刷新]）；演示页已对齐此序。
- imports：`useState`（react）、`useTranslations`（`groups` + `common`）、`Card`、三件套与 `ConsoleListColumn` / `ConsoleListSortDirection` / `ConsoleListTitleBlock`、`Badge`、`UsersRound`（lucide）、`useGroups`、`sortGroupsForDisplay` / `type AdminGroup`。

- [ ] **Step 6: 键位比对自检（Review Focus 9）**

Run:
```bash
cd frontend && node -e "const a=require('./messages/zh-CN.json'),b=require('./messages/en.json');const ks=['title','noPermission','loadFailed','preset','searchPlaceholder','empty','columnName','columnMembers','columnPermissions'];console.log(ks.every(k=>a.groups[k]&&b.groups[k]) && !!a.sidebar.groups && !!b.sidebar.groups)"
```
Expected: `true`

- [ ] **Step 7: 静态校验**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Expected: 全绿。此刻 `/console/settings/groups` 可看表格（3 预设 + 自建组、数字列、搜索 / 排序 / 刷新可用），无「新建」与操作列。

- [ ] **Step 8: Commit（建议边界③，等用户点名）**

```bash
git add frontend/messages/zh-CN.json frontend/messages/en.json frontend/app/console/layout.tsx frontend/components/console/console-sidebar.tsx frontend/components/console/console-sidebar-nav.tsx frontend/app/console/settings/groups/page.tsx frontend/app/console/settings/groups/groups-panel.tsx frontend/hooks/use-groups.ts
git commit -m "feat(frontend): 控制台用户组页——侧栏入口与表格视图（搜索/排序/刷新）"
```

---

### Task 4: 弹窗 + ⋯ 行菜单 + 新建 / 编辑接线

**Files:**
- Modify: `frontend/hooks/use-groups.ts`（已建，本任务扩 create/update）
- Create: `frontend/app/console/settings/groups/group-dialog.tsx`
- Modify: `frontend/app/console/settings/groups/groups-panel.tsx`（工具栏 `end` 加「新建」+ 行 ⋯ 菜单（编辑项）+ 弹窗挂载）
- Modify: `frontend/app/console/settings/groups/page.tsx`（加目录读取 + `canEdit` + 矩阵下传）
- Modify: `frontend/messages/zh-CN.json` / `frontend/messages/en.json`（`groups` 命名空间 +31 键）

**Interfaces:**
- Consumes: Task 1 的 `applyPermissionToggle` / `togglePermissionColumn` / `permissionColumnState` / `samePermissionSet` / `groupErrorKey` / `resourceLabelKey` / `actionLabelKey` / `parseAdminGroup`；Task 2 的 `getPermissionCatalog` / `SessionUser.permissions`；`ui/dropdown-menu`；`lib/api.ts` 的 `ApiError` / `apiRequest` / `looksLikeServiceFailure`
- Produces:

```ts
// hooks/use-groups.ts（T5 再扩 delete 字段）
export function useGroups(initialGroups: AdminGroup[]): {
  groups: AdminGroup[];
  isRefreshing: boolean;
  refreshGroups(): Promise<void>;
  isSaving: boolean;
  isListLocked: boolean; // = isSaving || isRefreshing（T5 再并入删除在途）
  submitGroup(input: {
    groupId: string | null;
    name: string;
    permissions: string[];
  }): Promise<boolean>; // true = 成功（调用方据此关弹窗）
};

// groups-panel.tsx（扩签名）
export function GroupsPanel(props: {
  initialGroups: AdminGroup[];
  matrix: PermissionMatrix;
  canEdit: boolean;
}): JSX.Element;

// group-dialog.tsx
export function GroupDialog(props: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  group: AdminGroup | null; // null = 新建
  matrix: PermissionMatrix;
  isPending: boolean;
  onSubmit: (input: { name: string; permissions: string[] }) => Promise<boolean>;
}): JSX.Element;
```

- [ ] **Step 1: 扩 `hooks/use-groups.ts`（create/update）**

语义逐条照 spec §4.1/§4.2 与 §6（模式对齐 `use-sessions.ts`）：

- state：在既有 `groups` / `isRefreshing` 基础上加 `isSaving`；`isListLocked = isSaving || isRefreshing`。
- `submitGroup({ groupId, name, permissions })`：
  - `if (isSaving) return false;`；`isSaving = true` 包裹全程（finally 复位）。
  - `groupId === null` → `apiRequest("/api/v1/admin/groups", { method: "POST", body: { name, permissions } })`；否则 `PATCH /api/v1/admin/groups/${encodeURIComponent(groupId)}`，body 同样整份 `{ name, permissions }`。
  - 回包 `parseAdminGroup`：`null` → `toast.error(t(groupId === null ? "createFailed" : "saveFailed"))`、return false（不重置列表）。
  - 成功：create → `setGroups((previous) => [...previous, group])` + `toast.success(t("created"))`；edit → `setGroups((previous) => previous.map((row) => (row.id === group.id ? group : row)))` + `toast.success(t("saved"))`；return true。
  - 失败：401 → return false（静默，全局会话失效弹窗接管）；`looksLikeServiceFailure` → `toast.error(tCommon("serviceUnavailable"))`；`ApiError` 且 `code === "not_found"` 且是 edit → **移除该行** + `toast.error(t("groupMissing"))` + return true（关弹窗，行已不存在）；其余 → `toast.error(t(groupErrorKey(code, groupId === null ? "create" : "edit")))`。
  - 不调 `router.refresh()`（D14：响应对象即真相）；列表顺序 / 搜索 / 排序都由面板计算，hook 只存原始数组。
- imports 增：`groupErrorKey` / `parseAdminGroup`（其余沿用 T3）。

- [ ] **Step 2: 写 `group-dialog.tsx`**

结构（spec §4.2）：

- `Dialog open={open} onOpenChange={handleOpenChange}` + `DialogContent className="sm:max-w-md"` + `DialogHeader`（`DialogTitle` = `group === null ? t("dialogCreateTitle") : t("dialogEditTitle")`）。
- 草稿 state：`name` / `selected` / `baseline`（`{ name, permissions }`）/ `isDiscardOpen`；`useEffect` 在 `open` 变 true 时按 `group` 重置三者（依赖 `[open, group]`）。
- `isDirty = name.trim() !== baseline.name.trim() || !samePermissionSet(selected, baseline.permissions)`。
- `isReadOnly = group?.isPreset ?? false`；`isPrivileged = group?.systemKey === "owner" || group?.systemKey === "admin"`。
- **关闭拦截**（Review Focus 8）：`handleOpenChange(next, eventDetails)`：
  - `next === false && isPending` → `eventDetails.cancel()`（保存中不许关）。
  - `next === false && isDirty` → `eventDetails.cancel()` + `setIsDiscardOpen(true)`。
  - 其余 → `onOpenChange(next)`。
- footer：
  - 只读 → 单个 `DialogClose` render `Button variant="outline"`（文案 `t("close")`）。
  - 可写 → 取消按钮 `DialogClose` render `Button variant="outline"`（文案 `tCommon("cancel")`）+ 保存按钮 `Button`（`t("save")`，`disabled = isPending || name.trim() === "" || !isDirty`，`onClick` → `const ok = await onSubmit({ name, permissions: selected }); if (ok) onOpenChange(false);`）。
- 放弃确认：`AlertDialog open={isDiscardOpen} onOpenChange={setIsDiscardOpen}`——`AlertDialogTitle` `t("discardTitle")`、`AlertDialogDescription` `t("discardDescription")`；`AlertDialogCancel`（common.cancel）；`AlertDialogAction variant="destructive"`（`t("discard")`）→ `setIsDiscardOpen(false); onOpenChange(false);`。
  （Base UI 弹层支持嵌套；如实测焦点 / 遮罩异常，退路是把放弃确认改为弹窗内部的一段内联确认区——实施时定。）
- 组名行：`Input value={name} maxLength={64} placeholder={t("namePlaceholder")} aria-label={t("nameLabel")} disabled={isReadOnly || isPending} onChange={...}`；旁边 `Label` 风格小标签用 `t("nameLabel")`（照 `ui/field`/`Label` 既有用法，实施时对齐 profile 表单）。
- 权限区：`t("permissionsLabel")` 小标题 + 网格，**列数动态**：
  ```tsx
  <div
    className="grid items-center gap-x-2 gap-y-1"
    style={{ gridTemplateColumns: `minmax(0, 1fr) repeat(${matrix.actions.length}, 3.5rem)` }}
  >
  ```
  - 首行：空首格 + 每个动作的列头——`<Checkbox checked={state === "all"} indeterminate={state === "some"} disabled={isPending} onCheckedChange={(checked) => setSelected((previous) => togglePermissionColumn(previous, columnKeys, checked))} aria-label={...} />`（`columnKeys` = 该列所有非空格键；`state` = `permissionColumnState(selected, columnKeys)`）；只读态列头整块不渲染。
  - 每行：资源名（`resourceLabelKey(resource)` 命中 `t(key)`，否则原文）+ 按 `matrix.actions` 对齐的格子：`null` → 空 `<span />`；否则 `<Checkbox checked={selected.includes(cell.key)} disabled={isReadOnly || isPending} onCheckedChange={(checked) => setSelected((previous) => applyPermissionToggle(previous, cell.key, checked))} aria-label={`${resourceLabel} ${actionLabel}`} />`。
- 说明行：`<p className="text-xs text-muted-foreground">{t("hintEditImpliesView")}</p>`；只读态替换为 `t(isPrivileged ? "readOnlyPrivileged" : "readOnlyPreset")`。
- 所有 aria-label 走翻译文案（**不硬编码中文**）。

- [ ] **Step 3: 面板接线 + 页面补目录读取**

`groups-panel.tsx`：

- 签名扩为 `{ initialGroups, matrix, canEdit }: { initialGroups: AdminGroup[]; matrix: PermissionMatrix; canEdit: boolean }`。
- hook 解构扩为 `const { groups, isRefreshing, refreshGroups, isSaving, isListLocked, submitGroup } = useGroups(initialGroups);`。
- 工具栏：`end`（`canEdit` 才传）= `<Button type="button" variant="outline" size="sm" onClick={() => setDialog({ group: null })}><Plus aria-hidden="true" />{t("newGroup")}</Button>`；`isBusy` / `isRefreshing` / `onRefresh` 改传 `isListLocked` / `isRefreshing` / `refreshGroups`。
- 表头 / 表格：`rowActions` / `actionsHeader`（`canEdit` 才传，见 Review Focus 10）——`rowActions={(group) => <RowActions group={group} />}`、`actionsHeader={t("actions")}`、`isBusy={isListLocked}`。
- 弹窗状态：`const [dialog, setDialog] = useState<{ group: AdminGroup | null } | null>(null);`。
- 行内子组件 `RowActions`（写在 `groups-panel.tsx` 内，不新增文件）——⋯ 菜单（对齐 `console-user-menu.tsx` 用法）：

  ```tsx
  <DropdownMenu>
    <DropdownMenuTrigger
      render={<Button type="button" variant="ghost" size="icon-sm" />}
      aria-label={t("actionsMenu", { name: group.name })}
      title={t("actionsMenu", { name: group.name })}
    >
      <Ellipsis aria-hidden="true" />
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end">
      <DropdownMenuItem onClick={() => setDialog({ group })}>
        <Pencil aria-hidden="true" />
        {t("edit")}
      </DropdownMenuItem>
      {/* 删除项 T5 加 */}
    </DropdownMenuContent>
  </DropdownMenu>
  ```

  （`RowActions` 需要 `setDialog`——把它定义在面板组件内部的闭包里，或作为 prop 传入，实施时任选。）
- 弹窗挂载：

  ```tsx
  <GroupDialog
    open={dialog !== null}
    onOpenChange={(open) => { if (!open) setDialog(null); }}
    group={dialog?.group ?? null}
    matrix={matrix}
    isPending={isSaving}
    onSubmit={(input) => submitGroup({ groupId: dialog?.group?.id ?? null, ...input })}
  />
  ```

`page.tsx`：加 `getPermissionCatalog` 读取（`Promise.all`）、`canEdit` 计算（`try { const user = await getAuthenticatedUser(); canEdit = user?.permissions.includes("system.groups.edit") ?? false; } catch { canEdit = false; }`，Review Focus 6）、渲染条件改 `groupsRead.status === "ok" && catalogRead.status === "ok"`，传 `matrix={buildPermissionMatrix(catalogRead.catalog)}` 与 `canEdit`。两个读任一非 ok 时文案：组列表 403 → `t("noPermission")`，其余 → `t("loadFailed")`。

- [ ] **Step 4: 两份 messages 加 31 键**

`groups` 命名空间（值照 spec §7，en 直译）：`newGroup`（新建按钮）/ `actions`（操作列头）/ `actionsMenu`（`「{name}」的操作`，⋯ 按钮 aria / title）/ `edit`（菜单项）/ `dialogCreateTitle` / `dialogEditTitle` / `nameLabel` / `namePlaceholder` / `permissionsLabel` / `hintEditImpliesView` / `readOnlyPreset` / `readOnlyPrivileged` / `save` / `close` / `created` / `saved` / `createFailed` / `saveFailed` / `groupMissing` / `presetLocked` / `nameTaken` / `grantForbidden` / `forbidden` / `discardTitle` / `discardDescription` / `discard` / `actionView` / `actionEdit` / `resourceUsers` / `resourceGroups` / `resourceSettings`（共 31 键）。

- [ ] **Step 5: 键位比对自检（Review Focus 9）**

Run:
```bash
cd frontend && node -e "const a=require('./messages/zh-CN.json'),b=require('./messages/en.json');const ks=['title','noPermission','loadFailed','preset','searchPlaceholder','empty','columnName','columnMembers','columnPermissions','newGroup','actions','actionsMenu','edit','dialogCreateTitle','dialogEditTitle','nameLabel','namePlaceholder','permissionsLabel','hintEditImpliesView','readOnlyPreset','readOnlyPrivileged','save','close','created','saved','createFailed','saveFailed','groupMissing','presetLocked','nameTaken','grantForbidden','forbidden','discardTitle','discardDescription','discard','actionView','actionEdit','resourceUsers','resourceGroups','resourceSettings'];console.log(ks.every(k=>a.groups[k]&&b.groups[k]))"
```
Expected: `true`（T3 的 9 个列表键与 T4 的 31 个键一并复核，共 40）

- [ ] **Step 6: 静态校验 + 取证 grep**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Expected: 全绿。

Run:
```bash
cd frontend && grep -rn '"system\.' app/console/settings/groups/ hooks/use-groups.ts; echo "exit=$?"
```
Expected: 无命中（`exit=1`）——权限键字符串只许出现在 `lib/groups.ts` 与 messages。

- [ ] **Step 7: Commit（建议边界④，等用户点名）**

```bash
git add frontend/hooks/use-groups.ts frontend/app/console/settings/groups/group-dialog.tsx frontend/app/console/settings/groups/groups-panel.tsx frontend/app/console/settings/groups/page.tsx frontend/messages/zh-CN.json frontend/messages/en.json
git commit -m "feat(frontend): 用户组页写路径——⋯ 行菜单与编辑弹窗（新建/编辑）"
```

---

### Task 5: 删除路径（⋯ 菜单删除项 + 二次确认）

**Files:**
- Modify: `frontend/hooks/use-groups.ts`（扩 delete 字段）
- Modify: `frontend/app/console/settings/groups/groups-panel.tsx`（⋯ 菜单加删除项 + AlertDialog）
- Modify: `frontend/messages/zh-CN.json` / `frontend/messages/en.json`（+7 键）

**Interfaces:**
- Consumes: Task 1 的 `groupErrorKey`；Task 4 的 `GroupsPanel` / `useGroups` / `RowActions`
- Produces（`useGroups` 返回值扩展）:

```ts
  isDeleting: boolean;
  isListLocked: boolean; // = isSaving || isDeleting || isRefreshing
  deleteTarget: AdminGroup | null;
  isDeleteOpen: boolean;
  setDeleteOpen(open: boolean): void;
  requestDelete(group: AdminGroup): void;
  confirmDelete(): Promise<void>;
```

- [ ] **Step 1: hook 扩删除**

- `requestDelete(group)` → `setDeleteTarget(group); setIsDeleteOpen(true);`；`confirmDelete()` → 先 `setIsDeleteOpen(false)`，再执行删除（对齐 `use-sessions` 的 `confirmRevoke`）。
- `confirmDelete`：`apiRequest(`/api/v1/admin/groups/${encodeURIComponent(target.id)}`, { method: "DELETE" })`：
  - 成功 → `setGroups(remove)` + `toast.success(t("deleted"))`。
  - `ApiError` 且 `status === 404` → **视为成功**（同 `use-sessions` 404 口径）：移除 + `toast.success(t("deleted"))`。
  - 401 → 静默；service failure → `common.serviceUnavailable`；其余 → `toast.error(t(groupErrorKey(code, "delete")))`。
- `isDeleting` 包裹全程；`isListLocked = isSaving || isDeleting || isRefreshing`。

- [ ] **Step 2: 面板加删除**

- ⋯ 菜单加删除项（`!group.isPreset` 才渲染，Review Focus 7 / 10）：

  ```tsx
  {group.isPreset ? null : (
    <DropdownMenuItem
      variant="destructive"
      disabled={group.memberCount > 0}
      title={group.memberCount > 0 ? t("deleteBlocked") : undefined}
      onClick={() => requestDelete(group)}
    >
      <Trash2 aria-hidden="true" />
      {t("delete")}
    </DropdownMenuItem>
  )}
  ```

  （`disabled` 用 Base UI Item 的既有 `data-disabled` 样式；`title` 走原生属性。`Trash2` 加进 `groups-panel.tsx` 的 lucide imports；`requestDelete` 从 hook 解构传入 `RowActions`，同 `setDialog` 的传递方式。）
- AlertDialog（照 `sessions-panel.tsx` 的删除确认结构）：`open={isDeleteOpen}` / `onOpenChange={setDeleteOpen}`；`AlertDialogMedia` 放 `Trash2`；`AlertDialogTitle` `t("deleteConfirmTitle")`；`AlertDialogDescription` `t("deleteConfirmDescription")`；footer = `AlertDialogCancel`（common.cancel）+ `AlertDialogAction variant="destructive"`（`t("delete")`，`onClick={() => void confirmDelete()}`）。

- [ ] **Step 3: 两份 messages 加 7 键**

`delete` / `deleteBlocked` / `deleteConfirmTitle` / `deleteConfirmDescription` / `deleted` / `deleteFailed` / `notEmpty`（值照 spec §7；`notEmpty` 与 `deleteBlocked` 文案相同但键分开——一个 toast 一个 tooltip）。

- [ ] **Step 4: 键位比对 + 静态校验**

Run:
```bash
cd frontend && node -e "const a=require('./messages/zh-CN.json'),b=require('./messages/en.json');const ks=['delete','deleteBlocked','deleteConfirmTitle','deleteConfirmDescription','deleted','deleteFailed','notEmpty'];console.log(ks.every(k=>a.groups[k]&&b.groups[k]))"
```
Expected: `true`
Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Expected: 全绿。至此 `/console/settings/groups` 四条操作链（看 / 建 / 改 / 删）全部打通。

- [ ] **Step 5: Commit（建议边界⑤，等用户点名）**

```bash
git add frontend/hooks/use-groups.ts frontend/app/console/settings/groups/groups-panel.tsx frontend/messages/zh-CN.json frontend/messages/en.json
git commit -m "feat(frontend): 用户组页删除路径——⋯ 菜单删除项与二次确认"
```

---

### Task 6: DEVLOG 登记 + 全量核对 + 交付清单

**Files:**
- Modify: `docs/DEVLOG.md`（顶部 `---` 之后倒序插入新一节）

**Interfaces:**
- Consumes: Task 1–5 的实现与变异自检记录
- Produces: 本轮决策与已知缺口入库；交用户的验收清单

- [ ] **Step 1: 写 DEVLOG 一节**

标题 `## 2026-10-15 · 控制台「用户组」页（表格视图 + 编辑弹窗，接 access 接口）`。必须包含：D1–D22 口径摘要（含 v2 表格视图修订）；**变异自检三条的实测输出**（Task 1 Step 4/5/6）；**已知缺口**逐条（spec §9 全列 + 实施中新增发现）；**提交批次**（按实际发生记录，未提交状态如实写"等用户点名"）。

- [ ] **Step 2: 全量静态校验 + 文件核对**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Expected: 全绿（`sonner` 存量 warning 不算）。
Run: `git -C . status --short` + `git -C . diff --stat HEAD`
Expected: 本轮只新增 / 改动计划内 12 个文件（`.superpowers/` 下的断言脚本不出现——已 gitignore）；基线干净（并行轮已入库），可逐文件核对。

- [ ] **Step 3: 交付（给用户的验收清单——不代跑）**

正文里逐条给出"打开哪条 URL 看什么"：

1. 管理员 / 所有者账号打开 `/console/settings/groups` → **表格视图**：工具栏（[新建] [刷新]，无搜索框）+ 表头（组名 / 成员数 / 权限点数 / 操作）；三预设组带「预设」徽章、行尾 ⋯ 按钮，数字列与库一致。
2. 点「成员数」「权限点数」表头 → 排序生效 + 箭头切换（降 / 升）；点刷新 → 列表重读。
3. 点任意行 ⋯ → 菜单「编辑 / 删除」；预设组的菜单只有「编辑」；有成员的组删除项置灰（悬停提示）；点「编辑」→ 弹窗预填该组。
4. 「新建」→ 填名 + 勾权限（勾「编辑」看是否自动带上「查看」）→ 保存 → toast「用户组已创建」+ 表格出现新行；重开该组编辑弹窗，勾选与保存一致。
5. 编辑自建组改名 / 改勾选 → 保存 → 行内即时更新；改动过再点 × → 弹「放弃未保存的修改？」。
6. 预设组（所有者 / 普通用户）菜单点「编辑」→ 弹窗只读（输入与勾选全灰）+ 两档提示文案。
7. 重名新建 → toast「这个组名已被使用」。
8. 删一个空自建组 → 菜单「删除」→ 确认弹窗 → 行消失 + toast。
9. 普通用户账号（冒烟账号 `gate-check@example.com`）→ 侧栏不见「用户组」入口；直连 `/console/settings/groups` → 「无权查看用户组」；（可选）若手头有一个"有查看、无编辑"权限的账号 → 页面无「新建」、无操作列。
10. 亮 / 暗主题 + 中 / 英切换各看一遍文案。

- [ ] **Step 4: Commit（建议边界⑥，等用户点名）**

```bash
git add docs/DEVLOG.md
git commit -m "docs(devlog): 登记控制台用户组页——联动规则与已知缺口"
```

---

## 执行方式

~~待用户点名：**Native**（本会话逐任务实现）或子代理驱动。~~ 2026-10-16 用户点名「开始执行，直接执行」→ **Native**（本会话逐任务实施）。两条既有约束不变：运行期验收归用户（执行者只跑静态校验与离线纯函数断言）；提交等用户点名（严禁裸 `git add -A`）——2026-10-16 已点名，按 backend / frontend / docs 三批入库。

## 执行结果（2026-10-16 接线轮，Native）

**落地文件：代码 11（9 改 + 2 新）+ 文档 3。**

| 文件 | 动作 |
| --- | --- |
| `frontend/lib/groups.ts` | 改（+102 行：`parseAdminGroup` / `parseAdminGroups` / `parsePermissionCatalog` + `ERROR_KEYS` / `FALLBACK_KEYS` / `groupErrorKey`；矩阵 / 联动 / 排序等纯 UI 批已有） |
| `frontend/lib/admin-groups.ts` | 新（`getAdminGroups` / `getPermissionCatalog`，三态 union，对齐 `admin-settings.ts`） |
| `frontend/lib/auth-session.ts` | 改（`SessionUser.permissions` 解析，三处小改） |
| `frontend/hooks/use-groups.ts` | 新（`refreshGroups` / `submitGroup` / `requestDelete` / `confirmDelete`；`isListLocked = isSaving ∥ isDeleting ∥ isRefreshing`） |
| `frontend/app/console/settings/groups/page.tsx` | 改（两接口服务端读 + `canEdit`；读失败 / 403 文案分支） |
| `frontend/app/console/settings/groups/groups-panel.tsx` | 改（删本地 CATALOG 与内存增删改；接 hook；`canEdit` 门控新建与操作列） |
| `frontend/components/console/console-sidebar-nav.tsx` | 改（设置子面板「用户组」项按 `canViewGroups` 显隐） |
| `frontend/components/console/console-sidebar.tsx` | 改（透传 `canViewGroups`） |
| `frontend/app/console/layout.tsx` | 改（算 `canViewGroups` 下传；异常兜底按 false） |
| `frontend/messages/zh-CN.json` / `en.json` | 改（`groups` +10 键，共 47；`sidebar.groups` 未新增——标签走 `settings.groups`） |
| 文档 | 改：本计划（路径 / 搜索 / 修订与执行记录）、spec v3、`docs/DEVLOG.md` 顶部一节 |

`frontend/app/console/settings/groups/group-dialog.tsx` **零改动**（纯 UI 批的组件接口已满足接线）。

**验证（离线，本机）**：

- 离线断言：`groups checks: PASS`（脚本 `.superpowers/sdd/console-groups-check/`，不入库；覆盖解析畸形面、矩阵分组与缺格、联动三规则 + 列批量复用、列三态、预设分区排序、错码全表）。
- 变异自检三条实测 RED→GREEN：① 删反向蕴含清理 → 「取消 view → 清 edit」红（actual `['system.users.edit']` vs expected `[]`）；② 删 `grant_forbidden` 映射 → 红（`saveFailed` vs `grantForbidden`）；③ 排序直返 `groups` → 红（`c2,user,c1,admin,owner` vs `owner,admin,user,c2,c1`）；均还原复绿。
- 静态门禁：`tsc --noEmit` ✓、`eslint` 0 error（2 条 `sonner.tsx` 存量 warning）、`prettier --check` ✓（`hooks/use-groups.ts` / `lib/admin-groups.ts` 首跑 warn → `prettier --write` 后复绿）。
- 键位比对：zh / en 各 47 键 ✓。取证 grep：`CATALOG` 0 处；权限键字面量仅剩 `layout.tsx` / `page.tsx` 门控各 1 处（计划内 gating）；蛇形键名只出现在 `lib/groups.ts` 解析里。
- **本机环境备注**：单文件 `tsc lib/groups.ts`（编译断言脚本用）**必须补 `--strict`**——文件参数调用不读项目 tsconfig，默认非 strict 下 `unknown` 的复合收窄行为不同，会误报 `TS2322`。

**状态**：按批入库（backend / frontend / docs 三批，**未推送**，含提交前独立评审的修补）；运行期验收归用户。
