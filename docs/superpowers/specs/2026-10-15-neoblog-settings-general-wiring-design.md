# NeoBlog 控制台「常规」设置页接后端（site 组六键）

**日期**：2026-10-15 · **范围**：纯前端（Next.js），后端零改动 · **实施计划**：`docs/superpowers/plans/2026-10-15-neoblog-settings-general-wiring.md`

**状态**：v1 草案，待用户审阅。D3（读失败形态）与 D9（happy-path 验收的 owner 账号）为「按推荐拟定」——请重点看这两处。

---

## 1. 意图与背景

用户 2026-10-15 点名：把 `/console/settings/general`（常规）页的设置行接上后端设置键值，**Logo / 图标两行除外**（无键、无上传接口，维持现状）。先决条件已就绪：后端 `site` 组六键齐备（0.19.0 五项 + 0.20.1 `block_search_engines`，均未推送）。

**现状（实读）**：

- 页面是纯 UI：`frontend/app/console/settings/general/page.tsx:1-14`（服务端组件：`getTranslations` + `<h1>` + `<SiteSettingsForm />`）；`site-settings-form.tsx:51-58` 六个 `useState` 初值全空串，`:92` / `:163` / `:171` 的 Logo 与图标按钮、`:225` 的「屏蔽搜索引擎」开关都是 `disabled`——页面上没有任何读写后端的链路。
- 后端接口现成：`GET / PATCH /api/v1/admin/settings/:group`（`backend/internal/modules/settings/controller.go:33-34`，挂 `system.settings.view` / `.edit`；owner / admin 由 `rbac.PresetPermissions`（`rbac.go:103`）运行时全量授予，普通用户两者都没有）。`PATCH` 是**部分更新**：只写请求体里出现的键（`service.go:203` `PatchGroup`），任一项不合法整组零落库；返回更新后的完整组记录。
- `site` 组六项（`registry.go:10-18`）：`title` / `subtitle` / `description` / `keywords` / `url`（文字、默认空串）+ `block_search_engines`（开关、默认 `false`）；六项全 `Public`。
- 唯一先例：个人资料页——服务端读（`lib/auth-session.ts:47-96`：读 cookie → 带 cookie 转发后端 → 形状校验，401 归 null）＋ 客户端保存（`hooks/use-profile-save.ts:15-99`：脏值比对 → `apiRequest` PATCH → 以响应重置基线 → toast → `router.refresh()`）＋ 右下角悬浮保存按钮（`console-floating-save-button.tsx`，仅脏时出现、Ctrl/Cmd+S 已内置）。
- 客户端请求器 `lib/api.ts:50-84`：相对路径走 `next.config.ts:10-16` 的 `/api/:path*` rewrite（同源、cookie 自动带）；`:73-75` 的 401 交给全局会话失效弹窗。
- 侧栏「设置」入口**不按权限过滤**（`console-sidebar-nav.tsx` 未做），普通用户会走到这个页面。

**本轮要什么**：页面从"能看能改但不能存"变成"读到后端真值、改完能存、存完是真值"；读失败（含无权限）不出可编辑表单。

---

## 2. 拟定决策

| # | 决策 | 口径 | 出处 |
| --- | --- | --- | --- |
| D1 | 读路径 | **服务端直读**：新增 `lib/admin-settings.ts`（照 `auth-session.ts` 的「读 cookie → 带 cookie 转发 → 形状校验」写法），`page.tsx` 取值作 props 下传；不做客户端 `useEffect` 拉取（直出无闪烁、无加载态） | 用户问答轮确认「照个人资料页那套」；先例 `auth-session.ts:47-96` |
| D2 | 模块分层 | 纯形状（类型 / 解析 / PATCH 构造）独立成 **`lib/site-settings.ts`**（零 import、客户端安全）；服务端读取独立成 **`lib/admin-settings.ts`**（引 `next/headers`，**不允许**被客户端组件引用） | 保存回包要在客户端解析，不能把 `next/headers` 拖进客户端 bundle |
| D3 | 读失败形态（**待重点看**） | 403 → 「无权查看设置」；401 / 5xx / 断网 / 形状坏 → 「设置读取失败」；**两种情形都不渲染表单**（页面只有 h1 + 一行提示）。**不变量：读失败路径不存在可保存的表单**——绝不允许在"空基线"上保存把真实配置清空。不做重试按钮（刷新即重试） | 按推荐拟定（备选：渲染禁用表单——更花哨，且容易出"灰表单还能提交"的洞，未取） |
| D4 | 保存交互 | 右下角 `ConsoleFloatingSaveButton`（沿用，不改）+ 脏值比对；**只 PATCH 改动过的键**；成功后**以响应 values 重置基线**；保存中输入/开关 `disabled` | 照 profile 先例（`use-profile-save.ts:56-86`） |
| D5 | 字段口径 | 六键逐字：`title` / `subtitle` / `description` / `keywords` / `url` / `block_search_engines`；前端 camelCase 只有 `blockSearchEngines`，映射单点（`SITE_SETTINGS_PATCH_KEYS`）；**不做前端校验与 trim**；**空串是合法值**（清空 = 写空串） | 后端 site 组 spec D7「类型之外不校验」；`registry.go:10-18` |
| D6 | 哪些行动 | 「屏蔽搜索引擎」开关**解禁**（去 `disabled`）接 `block_search_engines`；Logo / 图标两行维持 `disabled` 现状；关键词行按逗号分隔字符串直读直写（不做 chips） | 用户点名「除了 logo 和图片」 |
| D7 | 未保存离开 | **不做**守卫（无 `beforeunload`、无路由拦截），与个人资料页一致；登记为已知缺口 | 最小化 |
| D8 | 文案键 | `settings` 命名空间新增 5 键（页面级 `noPermission` / `loadFailed`；保存级 `save` / `saved` / `saveFailed`），不复用 `uc.*`（命名空间自包含）；5xx / 断网复用 `common.serviceUnavailable` | 与 register 轮同款做法 |
| D9 | happy-path 验收与 owner 账号（**待重点看**） | 运行期验收归用户（既有口径）。happy path 需要 owner 账号，**开发库当前没有**（0.19.0 轮已证）、冒烟账号是普通用户 → 默认走 (a)：本轮实机只覆盖「403 提示」路径 + 静态校验，happy path 待有 owner 账号再眼验；(b) 按 README 的 `UPDATE users SET group_id='00000000-0000-4000-8000-000000000001' …` 动开发库升号 | 用户口径（「我手动检验」）+ `plans/2026-10-15-neoblog-site-settings.md` 的账号事实 |

---

## 3. 范围

**改动文件（7 个 + 一次性脚本）**：

| 文件 | 动作 | 职责 |
| --- | --- | --- |
| `frontend/lib/site-settings.ts` | 新建 | 纯形状：`SiteSettingsValues` / `parseSiteSettingsValues` / `buildSiteSettingsPatch`（零 import） |
| `frontend/lib/admin-settings.ts` | 新建 | 服务端读取：`getSiteSettings(): Promise<SiteSettingsRead>` |
| `frontend/hooks/use-site-settings-save.ts` | 新建 | 脏值 + 保存（PATCH 改动键）+ toast + `router.refresh()` |
| `frontend/app/console/settings/general/page.tsx` | 改 | 服务端读 + 两分支渲染（表单 / 提示行） |
| `frontend/app/console/settings/general/site-settings-form.tsx` | 改 | props 接初值 → 受控 + `<form>` + 悬浮保存；开关解禁 |
| `frontend/messages/zh-CN.json` | 改 | settings 命名空间 +5 键 |
| `frontend/messages/en.json` | 改 | 同键位 +5 键 |
| `frontend/.scratch/site-settings-check/`（gitignore） | 一次性 | 纯函数断言 + 变异自检用，**不入库** |

**不动清单**：`components/ui/**`、`console-floating-save-button.tsx`、`console-settings-group.tsx` / `console-settings-row.tsx`、`lib/api.ts`、`lib/public-settings.ts`、`lib/auth-session.ts`、`console-sidebar-*.tsx`、register 页与其表单、`backend/**` 全部。

---

## 4. 设计条文

### 4.1 分层与依赖方向

```
服务端   page.tsx ──► lib/admin-settings.ts ──► lib/site-settings.ts（纯，零 import）
                        （next/headers + backendUrl）      ▲
客户端   site-settings-form.tsx ──► hooks/use-site-settings-save.ts
                        └────────────────────────► lib/site-settings.ts（解析回包 / 构造 patch）
                                                       └─► lib/api.ts（PATCH）
```

- `lib/site-settings.ts` 零 import：服务端（读）与客户端（写）共用一个解析器，形状口径单点。
- `lib/admin-settings.ts` 只被服务端组件引用；写路径不经过它（客户端走 `apiRequest` 相对路径 + rewrite）。

### 4.2 契约

```ts
// lib/site-settings.ts
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

- `parse`：`payload.values` 非对象、或五键非 string / `block_search_engines` 非 boolean → `null`；多余键忽略；返回 camelCase 对象（仅 `blockSearchEngines` 一处映射）。
- `buildSiteSettingsPatch`：**只含变化键**（`!==` 比较——空串变化、布尔双向都算变化）；无变化返回 `{}`（调用方据此短路）；键名映射走模块内唯一一张 `SITE_SETTINGS_PATCH_KEYS`（`blockSearchEngines → block_search_engines`，其余同名）。

```ts
// lib/admin-settings.ts
export type SiteSettingsRead =
  | { status: "ok"; values: SiteSettingsValues }
  | { status: "forbidden" }
  | { status: "failed" };

export async function getSiteSettings(): Promise<SiteSettingsRead>;
```

- 读 cookie（名 `session`）→ `fetch(`${backendUrl}/api/v1/admin/settings/site`, { headers: { cookie }, cache: "no-store" })`（`no-store` 照 `public-settings.ts:40-43` 先例，漏了会被静态化）。
- 映射：403 → `forbidden`；无 cookie / 401 / 其它非 2xx / 网络异常 / `parse` 为 null → `failed`；2xx 且解析通过 → `ok`。

```ts
// hooks/use-site-settings-save.ts
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

### 4.3 保存路径

- `save()`：`isSaving || !isDirty` 直接 return；body = `buildSiteSettingsPatch(values, baseline)`；`apiRequest("/api/v1/admin/settings/site", { method: "PATCH", body })`。
- 成功：`parseSiteSettingsValues(回包)` 通过 → `values` 与 `baseline` 一起重置为回包值 + `toast.success(t("saved"))` + `router.refresh()`；解析失败（不该发生）→ 保持原值 + `saveFailed` toast，**不重置基线**。
- 失败：401 直接 return（全局会话失效弹窗接管）；`looksLikeServiceFailure` → `common.serviceUnavailable`；其余（含 403 / 400）→ `t("saveFailed")`。
- 不发明 `param` → 行的错误映射：UI 只能送出合法键与合法类型，后端对本组的 400（项名 / 类型）不可达。

### 4.4 页面与表单形态

- `page.tsx`：h1 之下按 `read.status` 二选一——`ok` → `<SiteSettingsForm settings={read.values} />`；否则一行 `<p className="text-sm text-muted-foreground">`（403 → `t("noPermission")`；其余 → `t("loadFailed")`）。
- 表单：外层 `<form onSubmit={handleSubmit} noValidate>`（`handleSubmit` 只 `preventDefault` + `void save()`，照 `profile-form.tsx:108-111`）；行控件全部受控（`values.*` / `updateText`），`disabled={isSaving}`；开关 `checked={values.blockSearchEngines}` / `onCheckedChange={setBlockSearchEngines}`；表单末尾 `<ConsoleFloatingSaveButton isDirty={isDirty} isPending={isSaving} label={t("save")} />`。
- 行结构、图标、类名常量（`inputClassName` / `textareaClassName`）一律不动；`description` 仍用 `Textarea`。
- 状态优先级：表单只在读成功时存在，因此"读失败不得保存"由**结构**保证，不靠运行时判断。

### 4.5 文案键（`settings` 命名空间）

| 键 | zh-CN | en |
| --- | --- | --- |
| `noPermission` | 无权查看设置 | You do not have permission to view settings |
| `loadFailed` | 设置读取失败，请刷新重试 | Failed to load settings. Please refresh and try again. |
| `save` | 保存 | Save |
| `saved` | 已保存 | Saved |
| `saveFailed` | 保存失败，请稍后重试 | Saving failed. Please try again later. |

`en.json` 漏键**不会抛错**（只 `console.error` + 把键路径当文本渲染，见侧栏 spec §5 B6 的复验），每个加键任务必须跑键位比对自检。

---

## 5. 边界条件与已知风险

诚实登记，不藏：

- **B1 读路径的诊断信号**：两种提示文案就是"cookie 转发是否活着"的诊断——普通用户实机看到「无权查看设置」= 读路径通（拿到了后端 403）；看到「设置读取失败」= 转发 / 网络 / 解析坏了。**这条只能用户实机验收**（执行者不铺运行期探测，见 §8）。
- **B2 happy path 缺 owner 账号**（D9）：读到值 → 改 → 保存 → 刷新仍是新值的完整闭环，本轮默认只在有 owner 账号时才能实机走通。
- **B3 未保存离开无守卫**（D7）：改了没保存直接切页 / 关标签会静默丢改动；与个人资料页一致，登记为债务。
- **B4 多标签页后写覆盖**：同组设置无版本号 / ETag，两个标签页各自保存时，后保存者的"旧读值"可能覆盖前者已保存的键。部分更新语义下只在这几个键之间互相盖，不会整组清零；不做乐观锁。
- **B5 无自动测试运行器**（沿用侧栏 spec §5 B6）：`frontend/package.json:5-10` 无 `test` 脚本，全仓 0 个 `*.test.ts*`。防回归 = `tsc` / `eslint` / `prettier --check` + 一次性纯函数断言（`.scratch`，不入库、跑完即弃）+ 用户实机验收。引入 vitest 属未 request 的新依赖，**不做**。
- **B6 超长 / 异常文本直接落库**：不校验长度（对齐后端 D7），填超长描述不会拦——用户自己的事。

---

## 6. 明确不做

侧栏按权限隐藏设置入口（要先把 `permissions` 带进 `SessionUser`，留权限轮）；register 页（用户注册）与 `auth` 组接线；Logo / 图标上传接口与键；前台消费 `block_search_engines`（robots 元数据渲染）；后端任何改动；`lib/api.ts` / `public-settings.ts` / `auth-session.ts` 改动；测试运行器与测试目录；每行独立保存 / 自动保存。

---

## 7. 落地步骤

见 `docs/superpowers/plans/2026-10-15-neoblog-settings-general-wiring.md`。本 spec 不执行。

---

## 8. 依据（全部为本次实读）

- 页面与表单现状：`frontend/app/console/settings/general/page.tsx:1-14`、`site-settings-form.tsx:51-58`（六 state 空串初值）、`:92` / `:163` / `:171`（Logo / 图标 `disabled`）、`:225`（开关 `disabled`）
- 先例链：`frontend/lib/auth-session.ts:47-96`、`frontend/hooks/use-profile-save.ts:15-99`、`frontend/components/console/console-floating-save-button.tsx:8-12` / `:44-60`、`frontend/app/console/uc/profile/profile-form.tsx:108-111` / `:296-300`
- 请求与代理：`frontend/lib/api.ts:50-84`（同源 + 401 → `reportSessionExpired`）、`frontend/next.config.ts:10-16`（`/api/:path*` rewrite）、`frontend/lib/public-settings.ts:40-54`（`no-store` + 形状校验先例）
- 后端契约：`backend/internal/modules/settings/controller.go:33-34` / `:38`、`service.go:203`（`PatchGroup` 部分更新）、`registry.go:10-18`（site 六键）、`rbac.go:103`（`PresetPermissions`）、`backend/README.md`「设置」章节（271-288 行接口表与语义）
- 口径来源：`docs/superpowers/specs/2026-10-15-neoblog-site-settings-design.md`（D4 默认空串 / D7 不校验）、`docs/DEVLOG.md` 0.20.1 节（`block_search_engines`）、`plans/2026-10-15-neoblog-site-settings.md`（开发库无 owner 账号）
- 约束：侧栏 spec `2026-10-15-neoblog-sidebar-panel-state-design.md` §5 B6（无测试运行器 / 漏键不抛错）、`frontend/package.json:5-10`、`frontend/.gitignore:44-45`（`.scratch/` 忽略）
- 用户口径：运行期验收归用户（本会话审查轮原话「我只需要你检查代码本身有没有问题即可！不要实测了！我手动检验！」）；提交等点名（0.20.1 轮先例）
