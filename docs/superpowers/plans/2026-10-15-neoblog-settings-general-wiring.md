# 「常规」设置页接后端 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.（执行方式由用户点名，见文末；本计划已按本仓既有口径写明：**运行期验收归用户、提交等点名**。）

**Goal:** 把 `/console/settings/general` 的六个设置行接上后端 `site` 组键值——读到真值、只提交改动键、保存后即真值；读失败（含无权限）不出可编辑表单。

**Architecture:** 服务端直读（`lib/admin-settings.ts`，cookie 转发 + 形状校验）→ props 下传 → 客户端受控表单 + 脏值保存（`hooks/use-site-settings-save.ts`，PATCH 只带改动键）。纯形状（解析 / PATCH 构造）独立在 `lib/site-settings.ts`，服务端与客户端共用。

**Tech Stack:** Next.js 16.4.0（App Router）、React 19.3.0、next-intl、sonner、既有 `ConsoleSettingsGroup` / `ConsoleSettingsRow` 与 `ConsoleFloatingSaveButton`、`lib/api.ts`。

**Spec:** `docs/superpowers/specs/2026-10-15-neoblog-settings-general-wiring-design.md`（本计划按 D1–D9 与 §4.1–§4.5 论证；两份一起读）

## Global Constraints

- 不新增依赖；不引入测试运行器；一次性断言脚本放 `frontend/.scratch/`（已 gitignore，`frontend/.gitignore:44-45`），**不入库**。
- **运行期验收归用户**：执行者不跑浏览器、不 curl 接口、不拉 SSR 页面；`tsc` / `eslint` / `prettier --check` 与离线纯函数断言照跑（用户口径，spec §8）。
- **不主动提交**：各任务的 Commit 步骤只是建议分批边界；实际提交等用户点名（0.20.1 轮先例）。
- 六键逐字：`title` / `subtitle` / `description` / `keywords` / `url` / `block_search_engines`；前端 camelCase 只有 `blockSearchEngines`，映射单点（`SITE_SETTINGS_PATCH_KEYS`）。
- 不做前端校验与 trim；**空串是合法值**（清空 = 写空串）。
- 不动：`components/ui/**`、`console-floating-save-button.tsx`、`console-settings-group.tsx` / `console-settings-row.tsx`、`lib/api.ts`、`lib/public-settings.ts`、`lib/auth-session.ts`、`console-sidebar-*.tsx`、register 页与其表单、`backend/**`。
- i18n 中文以 `zh-CN.json` 为准、`en.json` 同键位；**漏键不抛错**（只 `console.error` + 渲染键路径），每个加键任务必须跑键位比对自检。
- 校验命令统一在 `frontend/` 下：`npx tsc --noEmit && npm run lint && npm run format:check`。`.next` 类型过期导致 tsc 报"找不到已删除路由的模块"时，先跑 `npx next typegen`（本会话实测过一次，非本轮引入）。

## Review Focus

spec 隐含但无自动测试兜底、最可能咬人的六条，逐条在对应任务里钉检查步骤：

1. **读失败绝不给出可保存的表单**（403 / 401 / 断网 / 形状坏 → 只有提示行）→ Task 1 断言（坏形状 → `null`）+ Task 2 页面分支。
2. **空串是合法改动**：把某字段清空再保存必须发出该键（不能被"空值跳过"逻辑吃掉）→ Task 1 断言 + 变异①。
3. **开关双向都要发**（true→false 也要进 body）→ Task 1 断言。
4. **键名映射单点**：PATCH body 用后端蛇形 `block_search_engines`，映射只在 `SITE_SETTINGS_PATCH_KEYS` → Task 1 断言 + 变异②。
5. **`en.json` 漏键不报错**（把键路径渲染成文本而不是白屏）→ Task 2 / Task 3 的键位比对。
6. **读路径 cookie 转发只能由用户实机验收**：两种提示文案就是诊断信号（403 →「无权查看设置」= 转发通；「设置读取失败」= 转发 / 网络 / 解析坏）→ Task 4 交付清单。

---

## 文件结构

| 文件 | 动作 | 唯一职责 |
| --- | --- | --- |
| `frontend/lib/site-settings.ts` | Create | 纯形状：类型 / 解析 / PATCH 构造（零 import） |
| `frontend/lib/admin-settings.ts` | Create | 服务端读：`getSiteSettings` + `SiteSettingsRead` |
| `frontend/app/console/settings/general/page.tsx` | Modify | 服务端读 + 两分支渲染 |
| `frontend/app/console/settings/general/site-settings-form.tsx` | Modify | T2 接 props 初值 → T3 受控 + `<form>` + 悬浮保存 |
| `frontend/hooks/use-site-settings-save.ts` | Create | 脏值 + 保存 + toast + `router.refresh()` |
| `frontend/messages/zh-CN.json` / `en.json` | Modify | `settings` 命名空间 +5 键（T2 两个 / T3 三个） |
| `docs/DEVLOG.md` | Modify | 顶部倒序登记（T4） |

Task 2 只接"读"（表单显示真值、还不能存——自洽的中间态）；Task 3 接"写"。

---

### Task 1: 纯形状模块 `lib/site-settings.ts`

**Files:**
- Create: `frontend/lib/site-settings.ts`
- Create（一次性、不入库）: `frontend/.scratch/site-settings-check/check.cjs`

**Interfaces:**
- Consumes: 无（模块零 import）
- Produces（Task 2 / Task 3 按此签名调用，名称与类型逐字一致）:

```ts
export type SiteSettingsValues = {
  title: string;
  subtitle: string;
  description: string;
  keywords: string;
  url: string;
  blockSearchEngines: boolean;
};
export function parseSiteSettingsValues(payload: unknown): SiteSettingsValues | null;
export function buildSiteSettingsPatch(
  values: SiteSettingsValues,
  baseline: SiteSettingsValues,
): Record<string, string | boolean>;
```

- [ ] **Step 1: 写模块**

语义逐条照 spec §4.2：

- `parse`：`payload.values` 非对象、或五键非 `string` / `block_search_engines` 非 `boolean` → `null`；多余键忽略；返回 camelCase 对象（仅 `blockSearchEngines` 一处映射）。
- `buildSiteSettingsPatch`：**只含变化键**（`!==` 比较——空串变化、布尔双向都算变化）；无变化返回 `{}`；键名映射走模块内唯一一张 `SITE_SETTINGS_PATCH_KEYS`（`blockSearchEngines → block_search_engines`，其余同名）；比较覆盖全部六键。

- [ ] **Step 2: 写离线断言脚本**

`frontend/.scratch/site-settings-check/check.cjs`（先建目录），内容逐字：

```js
const assert = require("node:assert/strict");
const {
  parseSiteSettingsValues,
  buildSiteSettingsPatch,
} = require("./site-settings.js");

const payload = {
  group: "site",
  values: {
    title: "My Blog",
    subtitle: "s",
    description: "d",
    keywords: "k",
    url: "u",
    block_search_engines: false,
  },
};

const parsed = parseSiteSettingsValues(payload);
assert.deepEqual(parsed, {
  title: "My Blog",
  subtitle: "s",
  description: "d",
  keywords: "k",
  url: "u",
  blockSearchEngines: false,
});

// 形状坏 → null（读侧据此走「读取失败」，绝不放行半截数据）
assert.equal(parseSiteSettingsValues(null), null);
assert.equal(parseSiteSettingsValues({ group: "site" }), null);
assert.equal(
  parseSiteSettingsValues({
    ...payload,
    values: { ...payload.values, block_search_engines: "false" },
  }),
  null,
);
assert.equal(
  parseSiteSettingsValues({ ...payload, values: { ...payload.values, url: 1 } }),
  null,
);

// 多余键忽略
assert.deepEqual(
  parseSiteSettingsValues({ ...payload, values: { ...payload.values, extra: 1 } }),
  parsed,
);

// 无变化 → {}
assert.deepEqual(buildSiteSettingsPatch(parsed, parsed), {});

// 只包含变化键
assert.deepEqual(buildSiteSettingsPatch({ ...parsed, title: "New" }, parsed), {
  title: "New",
});

// 空串变化要发出去（清空是合法改动）
assert.deepEqual(
  buildSiteSettingsPatch({ ...parsed, title: "" }, { ...parsed, title: "My Blog" }),
  { title: "" },
);

// 布尔双向都发，键名映射为后端蛇形
assert.deepEqual(
  buildSiteSettingsPatch({ ...parsed, blockSearchEngines: true }, parsed),
  { block_search_engines: true },
);
assert.deepEqual(
  buildSiteSettingsPatch(
    { ...parsed, blockSearchEngines: false },
    { ...parsed, blockSearchEngines: true },
  ),
  { block_search_engines: false },
);

console.log("site-settings checks: PASS");
```

- [ ] **Step 3: 跑断言**

Run:
```bash
cd frontend && npx tsc lib/site-settings.ts --outDir .scratch/site-settings-check --module commonjs --target es2022 --skipLibCheck && node .scratch/site-settings-check/check.cjs
```
Expected: `site-settings checks: PASS`（单文件 tsc 调用会忽略 tsconfig，flags 必须自足；这是纯函数级离线断言——不碰应用 / 浏览器 / 接口）。

- [ ] **Step 4: 变异自检①（空串）**

把 `buildSiteSettingsPatch` 的比较改成跳过空串（例如 `values[field] !== baseline[field] && values[field] !== ""`）→ 重跑 Step 3。
Expected: FAIL——"空串变化要发出去"那条红（`AssertionError`，实际 `{}`、期望 `{ title: '' }`）。还原 → 重跑绿。

- [ ] **Step 5: 变异自检②（映射）**

把 `SITE_SETTINGS_PATCH_KEYS.blockSearchEngines` 的值改成 `"blockSearchEngines"` → 重跑 Step 3。
Expected: FAIL——两条布尔断言红（实际键名 `blockSearchEngines`）。还原 → 重跑绿。

- [ ] **Step 6: 静态校验**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Expected: 全绿（新模块此刻无消费者，正常）。

- [ ] **Step 7: Commit（建议边界①，等用户点名）**

```bash
git add frontend/lib/site-settings.ts
git commit -m "feat(frontend): 设置页纯形状模块 site-settings——解析与 PATCH 构造"
```

`.scratch/` 不入库。

---

### Task 2: 服务端读取 + 页面接线（只读）

**Files:**
- Create: `frontend/lib/admin-settings.ts`
- Modify: `frontend/app/console/settings/general/page.tsx`（整页替换，见 Step 2）
- Modify: `frontend/app/console/settings/general/site-settings-form.tsx`（仅签名 + 初值，见 Step 3）
- Modify: `frontend/messages/zh-CN.json` / `frontend/messages/en.json`（`settings` 命名空间 +2 键）

**Interfaces:**
- Consumes: Task 1 的 `SiteSettingsValues`、`parseSiteSettingsValues`
- Produces: `SiteSettingsRead`（`ok` / `forbidden` / `failed` 判别联合）、`getSiteSettings(): Promise<SiteSettingsRead>`；表单新签名 `SiteSettingsForm({ settings }: { settings: SiteSettingsValues })`

- [ ] **Step 1: 写 `lib/admin-settings.ts`**

照 spec §4.2：读 cookie（名 `session`）→ `fetch(`${backendUrl}/api/v1/admin/settings/site`, { headers: { cookie }, cache: "no-store" })`；映射 403 → `forbidden`、无 cookie / 401 / 其它非 2xx / 网络异常 / `parse` 为 null → `failed`、2xx 且解析通过 → `ok`。`backendUrl` 从 `@/lib/backend-url` 取（照 `lib/auth-session.ts:57` 的用法）。

- [ ] **Step 2: 替换 `page.tsx`**

整页逐字（容器与 h1 不动，加读与两分支）：

```tsx
import { getTranslations } from "next-intl/server";

import { getSiteSettings } from "@/lib/admin-settings";

import { SiteSettingsForm } from "./site-settings-form";

export default async function SettingsGeneralPage() {
  const t = await getTranslations("settings");
  const read = await getSiteSettings();

  return (
    <div className="mx-auto flex w-full max-w-[1000px] flex-col gap-4 p-4 min-[1200px]:p-6">
      <h1 className="text-xl font-semibold tracking-tight">{t("general")}</h1>
      {read.status === "ok" ? (
        <SiteSettingsForm settings={read.values} />
      ) : (
        <p className="text-sm text-muted-foreground">
          {read.status === "forbidden" ? t("noPermission") : t("loadFailed")}
        </p>
      )}
    </div>
  );
}
```

- [ ] **Step 3: 表单最小接线（初值来自 props）**

`site-settings-form.tsx` 三处小改（其余不动，开关仍保持 `disabled`、没有保存）：

1. 加 `import type { SiteSettingsValues } from "@/lib/site-settings";`
2. `export function SiteSettingsForm() {` → `export function SiteSettingsForm({ settings }: { settings: SiteSettingsValues }) {`
3. 六个初值：`useState("")` → `useState(settings.title)`（`subtitle` / `description` / `keywords` / `url` 同理）；`useState(false)` → `useState(settings.blockSearchEngines)`

- [ ] **Step 4: 两份 messages 加 2 键**

`settings` 命名空间（zh-CN 在 `"siteGroup"` 附近加，en 同键位），值照 spec §4.5 表：`noPermission` / `loadFailed`。

- [ ] **Step 5: 键位比对自检（Review Focus 5）**

Run:
```bash
cd frontend && node -e "const a=require('./messages/zh-CN.json'),b=require('./messages/en.json');console.log(['noPermission','loadFailed'].every(k=>a.settings[k]&&b.settings[k]))"
```
Expected: `true`

- [ ] **Step 6: 静态校验**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Expected: 全绿。此时页面已直出真值（读成功时），但还不能保存。

- [ ] **Step 7: Commit（建议边界②，等用户点名）**

```bash
git add frontend/lib/admin-settings.ts frontend/app/console/settings/general/page.tsx frontend/app/console/settings/general/site-settings-form.tsx frontend/messages/zh-CN.json frontend/messages/en.json
git commit -m "feat(frontend): 常规设置页服务端读取与读失败分支"
```

---

### Task 3: 受控表单 + 悬浮保存（打通写路径）

**Files:**
- Create: `frontend/hooks/use-site-settings-save.ts`
- Modify: `frontend/app/console/settings/general/site-settings-form.tsx`
- Modify: `frontend/messages/zh-CN.json` / `frontend/messages/en.json`（+3 键）

**Interfaces:**
- Consumes: Task 1 的 `SiteSettingsValues` / `parseSiteSettingsValues` / `buildSiteSettingsPatch`；`lib/api.ts` 的 `ApiError` / `apiRequest` / `looksLikeServiceFailure`；Input / Textarea / Switch 受控件；`ConsoleFloatingSaveButton`
- Produces:

```ts
export function useSiteSettingsSave(initial: SiteSettingsValues): {
  values: SiteSettingsValues;
  isDirty: boolean;
  isSaving: boolean;
  updateText(
    field: "title" | "subtitle" | "description" | "keywords" | "url",
    value: string,
  ): void;
  setBlockSearchEngines(checked: boolean): void;
  save(): Promise<void>;
};
```

- [ ] **Step 1: 写 `hooks/use-site-settings-save.ts`**

语义逐条照 spec §4.3：

- state：`values` / `baseline` 两款都 `useState(initial)`（`baseline` 是脏值基准），`isSaving`。
- `isDirty = Object.keys(buildSiteSettingsPatch(values, baseline)).length > 0`（脏值判据与提交体共用同一个函数，不写第二遍比较）。
- `updateText` / `setBlockSearchEngines`：函数式 `setValues` 合并（`{ ...previous, [field]: value }`）。
- `save()`：`isSaving || !isDirty` 直接 return；body = `buildSiteSettingsPatch(values, baseline)`；`apiRequest("/api/v1/admin/settings/site", { method: "PATCH", body })`。
  - 成功：`parseSiteSettingsValues(回包)` 通过 → `setValues` 与 `setBaseline` 一起置为回包值、`toast.success(t("saved"))`、`router.refresh()`；解析为 `null`（不该发生）→ `toast.error(t("saveFailed"))` 且**不重置基线**。
  - 失败：`error instanceof ApiError && error.status === 401` → 直接 return（全局会话失效弹窗接管）；`looksLikeServiceFailure(error)` → `toast.error(tCommon("serviceUnavailable"))`；其余 → `toast.error(t("saveFailed"))`。
- imports：`useState`（react）、`useRouter`（next/navigation）、`useTranslations`（next-intl）、`toast`（sonner）、`ApiError` / `apiRequest` / `looksLikeServiceFailure`（`@/lib/api`）、`buildSiteSettingsPatch` / `parseSiteSettingsValues` / `type SiteSettingsValues`（`@/lib/site-settings`）。

- [ ] **Step 2: 表单改受控**

`site-settings-form.tsx`：

1. `import { useState } from "react";` → `import type { FormEvent } from "react";`（`useState` 不再用）。
2. 删六个 `useState`，换成钩子：`const { values, isDirty, isSaving, updateText, setBlockSearchEngines, save } = useSiteSettingsSave(settings);`
3. 五个文字行：`value={values.title}`、`onChange={(event) => updateText("title", event.target.value)}`、`disabled={isSaving}`（`subtitle` / `description`（Textarea）/ `keywords` / `url` 同理）。
4. 开关行：「屏蔽搜索引擎」的 `disabled` **删掉**，改 `checked={values.blockSearchEngines}` / `onCheckedChange={setBlockSearchEngines}` / `disabled={isSaving}`。Logo / 图标两行的 `disabled` **保留不动**。
5. 加 `handleSubmit`：`event.preventDefault(); void save();`（照 `profile-form.tsx:108-111`）。
6. 外层包 `<form onSubmit={handleSubmit} noValidate>`（`ConsoleSettingsGroup` 在里层；Logo / 图标行仍在卡内、不在可保存字段里）。
7. 表单末尾加 `<ConsoleFloatingSaveButton isDirty={isDirty} isPending={isSaving} label={t("save")} />`（组件在 `form` 内，Ctrl/Cmd+S 才会命中 owning form）。

- [ ] **Step 3: 两份 messages 加 3 键**

`settings` 命名空间：`save` / `saved` / `saveFailed`，值照 spec §4.5 表，zh / en 同键位。

- [ ] **Step 4: 键位比对自检（Review Focus 5）**

Run:
```bash
cd frontend && node -e "const a=require('./messages/zh-CN.json'),b=require('./messages/en.json');console.log(['noPermission','loadFailed','save','saved','saveFailed'].every(k=>a.settings[k]&&b.settings[k]))"
```
Expected: `true`

- [ ] **Step 5: 静态校验 + 取证 grep**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Expected: 全绿。

Run:
```bash
cd frontend && grep -c "useState" app/console/settings/general/site-settings-form.tsx && grep -c "ConsoleFloatingSaveButton" app/console/settings/general/site-settings-form.tsx && grep -c "block_search_engines" hooks/use-site-settings-save.ts
```
Expected: `0`（表单不再有自己的 state）、`2`（import + 使用）、`0`（蛇形键名只许出现在 `lib/site-settings.ts`）。

- [ ] **Step 6: Commit（建议边界③，等用户点名）**

```bash
git add frontend/hooks/use-site-settings-save.ts frontend/app/console/settings/general/site-settings-form.tsx frontend/messages/zh-CN.json frontend/messages/en.json
git commit -m "feat(frontend): 常规设置页接线——受控表单 + 悬浮保存（PATCH 改动键）"
```

---

### Task 4: DEVLOG 登记 + 交付清单

**Files:**
- Modify: `docs/DEVLOG.md`（顶部 `---` 之后倒序插入新一节）

**Interfaces:**
- Consumes: Task 1–3 的实现与变异自检记录
- Produces: 本轮决策与已知缺口入库；交用户的验收清单

- [ ] **Step 1: 写 DEVLOG 一节**

标题 `## 2026-10-15 · 控制台「常规」设置页接后端（site 组六键）`。必须包含：D1–D9 口径摘要；**变异自检两条的实测输出**（Task 1 Step 4 / 5）；**已知缺口**逐条（B1 读路径只能用户实机验收、B2 happy path 缺 owner 账号、B3 未保存离开无守卫、B4 多标签页后写覆盖、B5 无测试运行器、B6 不校验长度）；**提交批次**（按实际发生记录）。

- [ ] **Step 2: 全量静态校验 + 文件核对**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Run: `git -C . diff --stat HEAD`
Expected: 校验全绿；diff 只含本轮 7 个文件（+ DEVLOG）+ `.scratch`（ignored 不出现）。

- [ ] **Step 3: 交付（给用户的验收清单——不代跑）**

正文里逐条给出"打开哪条 URL 看什么"：

1. 普通用户账号（冒烟账号 `gate-check@example.com`）打开 `/console/settings/general` → 应显示「无权查看设置」（这同时证明 cookie 转发通——见 Review Focus 6）。
2. （D9(b) 升号后）owner 账号：页面六行显示库里真值；改标题 → 右下角浮出「保存」→ 点保存 → toast「已保存」→ 刷新后仍是新值。
3. 只动开关 → 保存 → 刷新仍为改后状态；再改回保存。
4. 把某字段清空 → 保存 → 刷新为空（空串是合法值）。
5. （可选）停掉后端进程 → 刷新 → 应显示「设置读取失败」而不是空表单。

- [ ] **Step 4: Commit（建议边界④，等用户点名）**

```bash
git add docs/DEVLOG.md
git commit -m "docs(devlog): 登记设置页常规组接线——读失败不变量与已知缺口"
```

---

## 执行方式

待用户点名：**Native**（本会话逐任务实现）或子代理驱动。两条既有约束不变：运行期验收归用户（执行者只跑静态校验与离线纯函数断言）；提交等用户点名（Commit 步骤里的分批边界照用）。
