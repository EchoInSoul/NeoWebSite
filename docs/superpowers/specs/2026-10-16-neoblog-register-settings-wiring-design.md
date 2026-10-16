# NeoBlog 控制台「用户注册」设置页接后端（register + auth 两组六键）

**日期**：2026-10-16 · **范围**：纯前端（Next.js），后端零改动、零版本进位（前端轮惯例，`f512409` 先例） · **实施计划**：`docs/superpowers/plans/2026-10-16-neoblog-register-settings-wiring.md`

**状态**：v1 草案，待用户审阅。方案与三处裁定按 2026-10-16 对话落稿：**注册默认组本轮不接**（值形状待裁定）、**密码复杂度接成读写**（存了暂不生效）、**本地校验 + 后端 409 兜底双保险**（新增两条文案）。

**后续裁定（同日，已实施）**：用户把「域名清单为空时拦住」改判为**不拦**（后端 register 组已摘掉组级校验，空名单合法可存）。本 spec §1 背景行、§2 D6 / D7、§5 `validateRegisterSettings` 首条、§7 失败分支、§9 错误映射表中**所有域名清单相关口径均已被覆盖**：`domainControlRequiresList` 键与其两个调用点（本地校验、409 param 映射）已删除，`splitDomains` 已删，改在邮箱域名控制开启时给域名输入框挂 `placeholder` 提示「留空表示不限制任何邮箱域名」。`password_min_length` 的 8–64 双保险口径全部不变。正文原样留存，权威口径见 `backend/README.md` 与 DEVLOG 同日条目。

---

## 1. 意图与背景

用户 2026-10-16 点名：把 `/console/settings/register`（用户注册）页的设置行「接入真实的数据来源」。先决条件已就绪：后端 `register` / `auth` 两组键齐备，且**本轮后端（0.20.3）刚把其中四键接成真实行为**（域名白名单、密码策略、保存时组合校验）。

**现状（实读）**：

- 页面是纯 UI：`page.tsx:6-12` 只有 `generateMetadata`（标题「用户注册」），`:14-23` 渲染 `<h1>` + `<RegisterSettingsForm />`——没有任何读取链路。
- `register-settings-form.tsx`（全文 145 行）：六个 `useState` 本地初值（`:30-35`，`true` / `false` / 空串），全部非受控、无 `disabled`；「注册默认组」的 `<Select>`（`:51-60`）挂着一个**空的 `<SelectContent />`**（无数据源）；「允许的邮箱域名」（`:80-87`）与密码组三行（`:108-136`）都是裸 `Input` / `Switch`。
- 后端接口现成：`GET / PATCH /api/v1/admin/settings/:group`（`settings/controller.go:33-34`）。**权限已按组分流**（0.21.0 并行轮入库）：`register` / `auth` 两组归 `system.register.view` / `.edit`；owner / admin 由 `rbac.PresetPermissions`（`rbac.go:103`）运行时全量授予。`PATCH` 是**部分更新**：只写请求体里出现的键（`settings/service.go` `PatchGroup`），返回更新后的完整组记录。
- 后端新校验（0.20.3）：`PATCH` 在**写入命中管辖项**时跑组级校验——开域名控制必须清单条目非空（管辖 `email_domain_control_enabled` / `allowed_email_domains`）；`password_min_length` 取 8–64（管辖自身）；违规 409 `setting_conflict`（`param` = 判定项名）。
- 六键逐字（`registry.go`）：register 组 `enabled`(bool) / `email_domain_control_enabled`(bool) / `allowed_email_domains`(str)；auth 组 `password_requirements_enabled`(bool) / `password_min_length`(int64) / `password_complexity`(str)。
- 前端先例链（站点轮已验证可用）：服务端读（`lib/admin-settings.ts` cookie 转发 + 形状校验）＋ 纯形状模块（`lib/site-settings.ts`，零 import）＋ 客户端保存 hook（`hooks/use-site-settings-save.ts`：脏值比对 → `apiRequest` PATCH → 以响应重置基线 → toast → `router.refresh()`）＋ 悬浮保存按钮（`console-floating-save-button.tsx`，脏时出现、Ctrl/Cmd+S 内置）。
- 请求器 `lib/api.ts`：相对路径走 rewrite（同源、cookie 自动带）；`ApiError` 带 `status` / `code` / `param`（`:18-30`）；401 → 全局会话失效弹窗（`:73-75`）。
- 侧栏「用户注册」项已按 `system.register.view` 显隐（0.21.0 入库），本轮不动。

**本轮要什么**：页面从"屏幕上的假开关"变成"读到库里真值、改完能存、存完是真值"；读失败（含无权限）不出可编辑表单。

---

## 2. 已定决策

| # | 决策 | 口径 | 出处 |
| --- | --- | --- | --- |
| D1 | 读路径 | **服务端直读两个组**：`lib/admin-settings.ts` 追加 `getRegisterSettings()`（同一函数内 `fetch` register + auth 两次，cookie 转发 + `cache: "no-store"`）；`page.tsx` 取值作 props 下传。不做客户端拉取 | 照站点轮 D1 先例；用户点名「接入真实的数据来源」 |
| D2 | 模块分层 | 纯形状（类型 / 两组解析 / 脏检测 / 校验 / PATCH 构造）独立成 **`lib/register-settings.ts`**（零 import、服务端与客户端共用）；服务端读取放既有的 `lib/admin-settings.ts`（该文件已引 `next/headers`，仅服务端，不外泄给客户端组件） | 照站点轮 D2 |
| D3 | 读失败形态 | 403 → 「无权查看设置」；无 cookie / 401 / 非 2xx / 断网 / 任一组的形状坏 → 「设置读取失败」；**两种情形都不渲染表单**（结构保证"读失败不得保存"）。不做重试按钮 | 照站点轮 D3（用户已认可） |
| D4 | 保存交互 | 悬浮保存 + 脏值比对；**按组分别 `PATCH`、只发变动键**（register 组一处、auth 组一处；只改一组就只发一次请求）；成功后以**响应回包的组 values** 更新对应半区、重置基线；保存中输入 / 开关 `disabled` | 照 profile / site 先例；两组不可合并为一次请求（后端无多组写接口） |
| D5 | 字段口径 | 六键逐字，camelCase 映射单点（两张 `*_PATCH_KEYS` 表）；**不 trim**；**空串是合法值**（`allowed_email_domains` / `password_complexity` 清空 = 写空串）；`password_min_length` 表单态是字符串、构造 PATCH 时转**整数**（JSON number） | 照站点轮 D5 + 后端类型（int64） |
| D6 | 本地校验（新增口径） | 提交前只校验**本次 patch 命中的规则**（与后端「管辖项门槛」同语义）：patch 含 `email_domain_control_enabled` / `allowed_email_domains` → 检查「开关开 + 清单按 `,`/`，` 拆分后为空」；patch 含 `password_min_length` → 检查 8–64 整数。命中违规 → toast 对应文案、**不发送、不重置基线** | 用户 2026-10-16 确认「本地先拦 + 后端兜底」；文案两条见 §4.6 |
| D7 | 服务端错误兜底映射 | 保存失败分支：401 静默（全局弹窗接管）；403 → 「无权修改设置」（新增键）；`setting_conflict`（409）→ 按 `param` 映射同款文案（开关项 → 清单文案；`password_min_length` → 长度文案）；5xx / 断网 → `common.serviceUnavailable`；其余 → 「保存失败，请稍后重试」 | 用户 2026-10-16 确认 |
| D8 | 接哪些行 | 六个可接行全部接线：`enabled` / `email_domain_control_enabled` / `allowed_email_domains` / `password_requirements_enabled` / `password_min_length` / `password_complexity`。「注册默认组」下拉**原样不动**（值形状「组键 vs 组 ID」未裁定、后端未消费——现在接会把口径焊死）；`email_verification_enabled` 不在 UI、不接 | 用户 2026-10-16 裁定 |
| D9 | 权限口径 | 本页读 / 写分别需要 `system.register.view` / `.edit`（0.21.0 分流后口径）；owner / admin 自动全量；无权限直链 → 页面「无权查看设置」；**不做**页面内按权限禁用控件（后端会拒，前端不做第二套） | 并行轮已入库口径；照站点页先例 |
| D10 | 验收与防回归 | 运行期验收归用户（执行者不跑浏览器）；本项目无测试运行器 → 防回归 = `tsc` / `eslint` / `prettier --check` + 一次性纯函数断言（`.superpowers/` 下、不入库、跑完即弃）+ 用户实机 | 本仓既有口径（站点轮 §5 B5、`neoblog-browser-verification-tips`） |
| D11 | 版本与提交 | 纯前端轮**零版本进位**（`f512409` 先例）；提交等用户点名，按文件清单精准暂存 | 本仓惯例 |

---

## 3. 范围

**改动文件（7 个 + 一次性脚本）**：

| 文件 | 动作 | 职责 |
| --- | --- | --- |
| `frontend/lib/register-settings.ts` | 新建 | 纯形状：两组类型 / 解析 / 脏检测 / 校验 / PATCH 构造（零 import） |
| `frontend/lib/admin-settings.ts` | 改 | 追加 `RegisterSettingsRead` + `getRegisterSettings()`（读两组） |
| `frontend/hooks/use-register-settings-save.ts` | 新建 | 脏值 + 本地校验 + 按组保存 + 错误映射 + toast + `router.refresh()` |
| `frontend/app/console/settings/register/page.tsx` | 改 | 服务端读 + 两分支渲染（表单 / 提示行）；`generateMetadata` 不动 |
| `frontend/app/console/settings/register/register-settings-form.tsx` | 改 | props 接初值 → 六个可接行受控 + `<form>` + 悬浮保存；**默认组行与 Select 原样保留** |
| `frontend/messages/zh-CN.json` / `en.json` | 改 | `settings` 命名空间 +3 键（§4.6 表） |
| `.superpowers/sdd/2026-10-16-register-settings-wiring/check.cjs`（一次性、不入库） | 新建 | 纯函数断言 + 变异自检用 |
| `docs/DEVLOG.md` | 改 | 顶部倒序登记（计划 T4） |

**不动清单**：`components/ui/**`、`console-floating-save-button.tsx`、`console-settings-group.tsx` / `console-settings-row.tsx`、`lib/api.ts`、`lib/site-settings.ts`、`lib/public-settings.ts`、`lib/auth-session.ts`、`console-sidebar-*.tsx`、`app/console/settings/general/**`（常规页）、`app/console/settings/groups/**`、`next.config.ts`、`backend/**` 全部。

---

## 4. 设计条文

### 4.1 分层与依赖方向

```
服务端   register/page.tsx ──► lib/admin-settings.ts ──► lib/register-settings.ts（纯，零 import）
                                  （next/headers + backendUrl）        ▲
客户端   register-settings-form.tsx ──► hooks/use-register-settings-save.ts
                                  └──────────► lib/register-settings.ts（解析回包 / 脏检测 / 校验 / 构造 patch）
                                                       └─► lib/api.ts（PATCH，按组两次）
```

- `lib/register-settings.ts` 零 import：服务端（读）与客户端（写）共用同一套形状口径。
- `lib/admin-settings.ts` 只被服务端组件引用；写路径不经过它。

### 4.2 契约（`lib/register-settings.ts`）

```ts
export type RegisterGroupValues = {
  enabled: boolean;
  emailDomainControlEnabled: boolean;
  allowedEmailDomains: string;
};

export type AuthGroupValues = {
  passwordRequirementsEnabled: boolean;
  passwordMinLength: string;
  passwordComplexity: string;
};

export type RegisterSettingsValues = RegisterGroupValues & AuthGroupValues;

export type RegisterSettingsPatch = {
  register?: Record<string, string | boolean>;
  auth?: Record<string, string | number | boolean>;
};

export type RegisterSettingsIssue =
  | "domainControlRequiresList"
  | "passwordMinLengthRange";

export function parseRegisterGroupValues(payload: unknown): RegisterGroupValues | null;
export function parseAuthGroupValues(payload: unknown): AuthGroupValues | null;
export function parseRegisterSettingsValues(
  registerPayload: unknown,
  authPayload: unknown,
): RegisterSettingsValues | null;
export function hasRegisterSettingsChanges(
  values: RegisterSettingsValues,
  baseline: RegisterSettingsValues,
): boolean;
export function validateRegisterSettings(
  values: RegisterSettingsValues,
  baseline: RegisterSettingsValues,
): RegisterSettingsIssue | null;
export function buildRegisterSettingsPatch(
  values: RegisterSettingsValues,
  baseline: RegisterSettingsValues,
): RegisterSettingsPatch;
```

- `parseRegisterGroupValues`：`payload.values` 非对象、或三键错型（两 bool 一 string）→ `null`；多余键忽略。
- `parseAuthGroupValues`：`password_requirements_enabled` 非 bool / `password_min_length` **非整数 number** / `password_complexity` 非 string → `null`；`password_min_length` 转成十进制字符串（`String(n)`）；多余键忽略。
- `parseRegisterSettingsValues`：两个 parse 都通过才返回合并值，任一半坏 → `null`（读侧据此走「读取失败」，绝不放行半截基线）。
- `hasRegisterSettingsChanges`：六字段逐一 `!==` 比较（字符串比原样文本——`"08"` 对 `"8"` 算变更，见 B5）。**脏判据与"能不能发"解耦**：非法输入也算脏（保存按钮要出现、让校验有机会说话，见 D6）。
- `validateRegisterSettings`：`baseline.domain` 无关——只看**命中**（与 `buildRegisterSettingsPatch` 同源的变更判定）：① `emailDomainControlEnabled` 或 `allowedEmailDomains` 有变更，且变更后的最终态是「开关开 + 清单按 `,`/`，` 拆分、trim、丢空后为空」→ `"domainControlRequiresList"`；② `passwordMinLength` 有变更且最终态不是 8–64 的整数字符串 → `"passwordMinLengthRange"`；其余 → `null`。
- `buildRegisterSettingsPatch`：只含变更键；无变更的组**不带该字段**（`patch.register === undefined`）；键名映射走模块内两张唯一表——`register` 组 `emailDomainControlEnabled → email_domain_control_enabled`（余同名）、`auth` 组同名键 + `passwordMinLength → password_min_length`；`password_min_length` 以 `Number(...)` 转数字（调用方已先过 `validateRegisterSettings`；离线断言钉"送出去的是数字"）。
- 模块内私有：`splitDomains(raw)`（`,`/`，` 拆分 + trim + 丢空，照后端 `SplitListValue` 口径）、两张 `*_PATCH_KEYS`。

### 4.3 保存路径（`hooks/use-register-settings-save.ts`）

```ts
export function useRegisterSettingsSave(initial: RegisterSettingsValues): {
  values: RegisterSettingsValues;
  isDirty: boolean;
  isSaving: boolean;
  setEnabled(checked: boolean): void;
  setEmailDomainControlEnabled(checked: boolean): void;
  updateAllowedEmailDomains(value: string): void;
  setPasswordRequirementsEnabled(checked: boolean): void;
  updatePasswordMinLength(value: string): void;
  updatePasswordComplexity(value: string): void;
  save(): Promise<void>;
};
```

`save()` 顺序（逐条）：

1. `isSaving || !isDirty` 直接 return。
2. **本地校验**：`validateRegisterSettings(values, baseline)`；命中 → `toast.error`（按 issue 映射 §4.6 文案）并 return（不发送、不清脏）。
3. 构造 `patch = buildRegisterSettingsPatch(values, baseline)`；**按组分别发送，只发有变更的组**：
   - `patch.register` 存在 → `PATCH /api/v1/admin/settings/register`，回包 `parseRegisterGroupValues` 通过 → 合并进 next 值；解析失败（不该发生）→ `saveFailed` toast、**不重置基线**。
   - `patch.auth` 存在 → `PATCH /api/v1/admin/settings/auth`，同上。
4. 全部成功：`values` 与 `baseline` 一起置为合并后的 next 值 → `toast.success(t("saved"))` → `router.refresh()`。
5. 失败分支（catch）：`ApiError && status === 401` → 静默 return；`status === 403` → 「无权修改设置」；`code === "setting_conflict"` → 按 `param` 映射（`email_domain_control_enabled` / `allowed_email_domains` → 清单文案；`password_min_length` → 长度文案；其余 → 保存失败）；`looksLikeServiceFailure` → `common.serviceUnavailable`；其余 → 保存失败。

**部分失败语义**（登记在 §5 B1）：先发的组成、后发的组失败时，已落库的半区不回滚、基线不重置，下次保存按差值重发（同值幂等）。

### 4.4 页面与表单形态

- `page.tsx`：h1 之下按 `read.status` 二选一——`ok` → `<RegisterSettingsForm settings={read.values} />`；否则一行 `<p className="text-sm text-muted-foreground">`（403 → `t("noPermission")`；其余 → `t("loadFailed")`）。结构照 `general/page.tsx:20-31`。
- 表单：外层 `<form onSubmit={handleSubmit} noValidate>`（`handleSubmit` 只 `preventDefault` + `void save()`），两卡顺序 / 行结构 / 图标 / 类名常量（`inputClassName`、`Select` 的 `w-45`）一律不动；六个可接行全部受控（`values.*` + 对应 setter），`disabled={isSaving}`；表单末尾 `<ConsoleFloatingSaveButton isDirty={isDirty} isPending={isSaving} label={t("save")} />`（在 `form` 内，Ctrl/Cmd+S 才命中 owning form）。
- **「注册默认组」行原样保留**：`<Select>` 不加 `value` / `onValueChange`、`<SelectContent />` 保持为空（D8）。
- 密码最短长度 Input 维持 `type="number"`，不加 `min` / `max` 属性（校验在 hook，见 D6——HTML 属性在 `noValidate` 下不参与）。

### 4.5 错误 → 文案映射表

| 场景 | 判定 | 文案键（`settings` 命名空间） |
| --- | --- | --- |
| 本地：开域名控制 + 空清单 | `validateRegisterSettings` | `domainControlRequiresList` |
| 本地：密长越界 / 非整数 | 同上 | `passwordMinLengthRange` |
| 服务端 409 `setting_conflict` | `param` = 开关项 | `domainControlRequiresList` |
| 服务端 409 `setting_conflict` | `param` = `password_min_length` | `passwordMinLengthRange` |
| 服务端 403 | 保存 | `noPermissionToSave` |
| 服务端 5xx / 断网 | `looksLikeServiceFailure` | `common.serviceUnavailable` |
| 其余失败 | 兜底 | `saveFailed` |

### 4.6 文案键（`settings` 命名空间新增 3 键）

| 键 | zh-CN | en |
| --- | --- | --- |
| `noPermissionToSave` | 无权修改设置 | You do not have permission to modify settings |
| `domainControlRequiresList` | 要开启邮箱域名控制，请先填写允许的邮箱域名 | Fill in at least one allowed email domain before turning on domain control |
| `passwordMinLengthRange` | 密码最短长度需在 8–64 之间 | Minimum password length must be between 8 and 64 |

`en.json` 漏键不抛错（渲染键路径），加键任务必须跑键位比对自检；**只加键、不动既有键**（并行会话可能同时在改 messages）。

---

## 5. 边界条件与已知风险

- **B1 两组的两次 PATCH 非事务**：无多组写接口；第一组成功、第二组失败 → 半区已落库、基线不重置、重试幂等。不做"部分成功"特判文案（不发明）。
- **B2 校验按命中触发**（D6）：库里若存在探针直写的脏态（如「开 + 空」）而你本次没动那两个键 → 不误拦（与后端门槛同语义）；反之只改「允许公开注册」时，开 + 空旧态既不拦也不修——修它要动域名单两键，属用户自己的事。
- **B3 未保存离开无守卫**：与个人资料页 / 站点页一致，不做 `beforeunload`，登记为债务。
- **B4 多标签页后写覆盖**：同组设置无版本号；后保存者按自己的旧基线提交变动键，只在这几键间互相覆盖。不做乐观锁。
- **B5 文本相等即"无变更"的边界**：`"08"` 对 `"8"` 判脏并会重发（值等价 8）；保存后由回包归一为 `"8"`。不特判。
- **B6 客户端 8–64 与后端常量的一致性**：后端两处常量（settings 校验 / auth 夹取）与本页校验全靠口径一致，前端拿不到后端常量；本轮以文本对齐（spec §4.2 + 离线断言边界 8 / 64），**不为共享常量动后端**。
- **B7 读两组"要么一起好、要么一起坏"**：两组同源同权限，正常一起过；任一半 parse 失败按整页 `failed` 处理（不出表单）。

---

## 6. 明确不做

「注册默认组」下拉接线（值形状待裁定，含组列表接口与 `system.groups.view` 的连带）；`email_verification_enabled`（不在 UI）；页面内按权限禁用控件（D9）；未保存离开守卫；乐观锁 / ETag；测试运行器与新依赖；`reading` 骨架屏 / 重试按钮；后端任何改动；版本进位；`docs/DEVLOG.md` 之外的历史文档回补；常规页 P2-1（403 提示的旧账，等用户点名再顺手）。

---

## 7. 落地步骤

见 `docs/superpowers/plans/2026-10-16-neoblog-register-settings-wiring.md`。本 spec 不执行。

---

## 8. 依据（全部为本次实读）

- 页面与表单现状：`frontend/app/console/settings/register/page.tsx:6-23`（generateMetadata + h1 + 表单，无读取）；`register-settings-form.tsx:30-35`（六 state）、`:51-60`（空 Select）、`:80-87`（域名单 Input）、`:108-136`（密码组三行）
- 先例链：`frontend/lib/admin-settings.ts`（`getSiteSettings`，cookie 转发 + 403/failed 三分支）、`frontend/lib/site-settings.ts:1-67`（纯形状）、`frontend/hooks/use-site-settings-save.ts:13-79`（脏值 + 保存 + 错误分支）、`frontend/app/console/settings/general/page.tsx:20-31`（两分支渲染）、`frontend/components/console/console-floating-save-button.tsx:8-12` / `:44-60`（props 与 Ctrl/Cmd+S）、`frontend/lib/api.ts:18-30` / `:50-84`（ApiError 带 code/param、401 全局弹窗）
- 后端契约（0.20.3 入库）：`backend/internal/modules/settings/controller.go:33-34`（按组守卫）、`service.go` `PatchGroup`（部分更新 + 门槛校验）、`registry.go`（两组键与 `ValidateOn` 集合）、`backend/README.md`「设置」章节（接口表与 409 语义）、`docs/superpowers/specs/2026-10-16-neoblog-registration-policy-design.md`（D4/D5/D6 口径）
- 前端工程事实：`frontend/package.json:5-12`（`lint` / `format:check`，无 `test`）、`messages/zh-CN.json:32-68`（settings 命名空间现 35 键，zh / en 一致）、站点轮 spec §5 B5（无测试运行器）与 `.superpowers` 断言脚本先例（`2026-10-15-neoblog-site-metadata` 计划）
- 用户口径：2026-10-16 对话三条裁定（D8 / 复杂度接读写 / 校验双保险）；运行期验收归用户（本会话原话「我手动检验」）；提交等点名。
