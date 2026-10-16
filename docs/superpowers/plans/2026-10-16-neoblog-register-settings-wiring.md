# 「用户注册」设置页接后端（register + auth 两组六键）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.（执行方式由用户点名，见文末；本计划已按本仓既有口径写明：**运行期验收归用户、提交等点名、零版本进位**。）

**Goal:** 把 `/console/settings/register` 的六个可接设置行接上后端 `register` + `auth` 两组键值——读到真值、只提交改动键（按组分别 PATCH）、本地校验 + 409 兜底；读失败（含无权限）不出可编辑表单；「注册默认组」下拉原样不动。

**Architecture:** 服务端直读两组（`lib/admin-settings.ts` 追加 `getRegisterSettings`，cookie 转发 + 两次 fetch + 形状校验）→ props 下传 → 客户端受控表单 + 脏值保存（`hooks/use-register-settings-save.ts`：本地校验 → 按组 PATCH 改动键 → 以响应重置基线）。纯形状（解析 / 脏检测 / 校验 / PATCH 构造）独立在 `lib/register-settings.ts`（零 import），服务端与客户端共用。

**Tech Stack:** Next.js 16.4.0（App Router）、React 19.3.0、next-intl、sonner、既有 `ConsoleSettingsGroup` / `ConsoleSettingsRow` 与 `ConsoleFloatingSaveButton`、`lib/api.ts`。

**Spec:** `docs/superpowers/specs/2026-10-16-neoblog-register-settings-wiring-design.md`（本计划按 D1–D11 与 §4.1–§4.6 论证；两份一起读）

## Global Constraints

- 不新增依赖；不引入测试运行器；一次性断言脚本放 **`.superpowers/sdd/2026-10-16-register-settings-wiring/`**（仓库根、gitignore、**不被 eslint/prettier 扫描**——`frontend/.scratch` 会被扫到，别放那里），**不入库**。
- **运行期验收归用户**：执行者不跑浏览器、不 curl 接口、不拉 SSR 页面；`tsc` / `eslint` / `prettier --check` 与离线纯函数断言照跑。
- **不主动提交**：各任务的 Commit 步骤只是建议分批边界；实际提交等用户点名。**并行会话可能同时在改 `messages/*.json` 与 `docs/DEVLOG.md`**——只加键 / 只加节、按文件清单暂存、严禁裸 `git add -A`。
- **零版本进位**（前端轮惯例，`f512409` 先例）。
- 六键逐字：register 组 `enabled` / `email_domain_control_enabled` / `allowed_email_domains`；auth 组 `password_requirements_enabled` / `password_min_length` / `password_complexity`。前端 camelCase 映射单点在 `lib/register-settings.ts` 的两张 `*_PATCH_KEYS` 表；**蛇形键名只许出现在 `lib/register-settings.ts` 与 `hooks/use-register-settings-save.ts` 的 409 param 分支里**。
- 不做 trim；空串是合法值；`password_min_length` 送出的是**数字**（不是字符串）。
- 不动：`components/ui/**`、`console-floating-save-button.tsx`、`console-settings-group.tsx` / `console-settings-row.tsx`、`lib/api.ts`、`lib/site-settings.ts`、`lib/public-settings.ts`、`lib/auth-session.ts`、`console-sidebar-*.tsx`、`app/console/settings/general/**`、`app/console/settings/groups/**`、`next.config.ts`、`backend/**`。
- **「注册默认组」行与其 `<Select>` 原样保留**（不加 `value` / `onValueChange` / options）。
- i18n 中文以 `zh-CN.json` 为准、`en.json` 同键位；**只加键、不动既有键**；漏键不抛错但每个加键任务必须跑键位比对自检。
- 校验命令统一在 `frontend/` 下：`npx tsc --noEmit && npm run lint && npm run format:check`。`.next` 类型过期导致 tsc 报"找不到已删除路由的模块"时，先跑 `npx next typegen`（非本轮引入）。
- 单文件编译断言脚本**必须带 `--strict`**（忽略 tsconfig；不带会误报/漏报——2026-10-16 用户组轮实测）。

## Review Focus

spec 隐含但无自动测试兜底、最可能咬人的六条，逐条在对应任务里钉检查步骤：

1. **读失败绝不给出可保存的表单**（403 / 断网 / 任一组的形状坏 → 只有提示行）→ Task 1 断言（坏形状 → `null`）+ Task 2 页面分支。
2. **本地校验只拦"本次动到的规则"**（与后端管辖项门槛同语义）：只改「允许公开注册」时，开着域名控制 + 空清单的旧态**不拦**；只改域名单两键或密长时才校验 → Task 1 断言 + 变异①。
3. **只发变动键、按组分别发**：register 组变化 → patch 只有 `register` 字段；两组都不变 → `{}`；空串清空是合法改动 → Task 1 断言 + 变异②。
4. **密长送数字**：`password_min_length` 在 patch 里必须是 `number` → Task 1 断言。
5. **键名映射单点**：PATCH body 用后端蛇形，映射只在纯模块两张表 → Task 1 断言 + Task 3 grep 取证。
6. **校验按命中触发 + 409 兜底两条路径的文案**（本地 toast 与后端 param 映射结果一字不差）→ Task 3 grep + 实机清单（Task 4）。

---

## 文件结构

| 文件 | 动作 | 唯一职责 |
| --- | --- | --- |
| `frontend/lib/register-settings.ts` | Create | 纯形状：两组类型 / 解析 / 脏检测 / 校验 / PATCH 构造（零 import） |
| `frontend/lib/admin-settings.ts` | Modify | 追加 `RegisterSettingsRead` + `getRegisterSettings`（T2） |
| `frontend/app/console/settings/register/page.tsx` | Modify | 服务端读 + 两分支渲染（T2） |
| `frontend/app/console/settings/register/register-settings-form.tsx` | Modify | T2 接 props 初值 → T3 受控 + `<form>` + 悬浮保存 |
| `frontend/hooks/use-register-settings-save.ts` | Create | 校验 + 按组保存 + 错误映射 + toast + `router.refresh()`（T3） |
| `frontend/messages/zh-CN.json` / `en.json` | Modify | `settings` 命名空间 +3 键（T3） |
| `docs/DEVLOG.md` | Modify | 顶部倒序登记（T4） |

Task 2 只接"读"（表单显示真值、还不能存——自洽的中间态，且**编译必须过**）；Task 3 接"写"。

---

### Task 1: 纯形状模块 `lib/register-settings.ts`

**Files:**
- Create: `frontend/lib/register-settings.ts`
- Create（一次性、不入库）: `.superpowers/sdd/2026-10-16-register-settings-wiring/check.cjs`

**Interfaces:**
- Consumes: 无（模块零 import）
- Produces（Task 2 / Task 3 按此签名调用，名称与类型逐字一致）:

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

- [ ] **Step 1: 写模块**

语义逐条照 spec §4.2：

- `parseRegisterGroupValues`：`payload.values` 非对象、或 `enabled` / `email_domain_control_enabled` 非 boolean、`allowed_email_domains` 非 string → `null`；多余键忽略；返回 camelCase。
- `parseAuthGroupValues`：`password_requirements_enabled` 非 boolean、`password_min_length` **非整数 number**（`typeof === "number" && Number.isInteger`）、`password_complexity` 非 string → `null`；`password_min_length` 用 `String(n)` 转十进制文本；多余键忽略。
- `parseRegisterSettingsValues`：两个 parse 都过才返回 `{ ...register, ...auth }`；任一半 `null` → `null`。
- `hasRegisterSettingsChanges`：六字段逐一 `!==` 比较（minLength 比原始文本）。
- `validateRegisterSettings`：**只看命中**——`emailDomainControlEnabled` 或 `allowedEmailDomains` 有变更时，若最终态为「`emailDomainControlEnabled === true` 且 `splitDomains(allowedEmailDomains).length === 0`」→ `"domainControlRequiresList"`；`passwordMinLength` 有变更时，若 `Number(raw)` 非整数或不在 [8, 64]（`""` / `"abc"` → `NaN` 拦、`"7"` / `"65"` / `"8.5"` 拦、`"8"` / `"64"` 过）→ `"passwordMinLengthRange"`；否则 `null`。
- `buildRegisterSettingsPatch`：只含变更键、无变更的组不带字段；两张映射表（register：`emailDomainControlEnabled → email_domain_control_enabled`，余同名；auth：`passwordMinLength → password_min_length`，`passwordMinLength` 以 `Number(...)` 转数字，其余同名）；私有 `splitDomains(raw)`（`,` 与 `，` 都认、trim、丢空，照后端 `SplitListValue` 口径）。

- [ ] **Step 2: 写离线断言脚本**

先建目录 `.superpowers/sdd/2026-10-16-register-settings-wiring/`，写 `check.cjs`（require 编译产物 `./register-settings.js`），内容逐字：

```js
const assert = require("node:assert/strict");
const {
  parseRegisterGroupValues,
  parseAuthGroupValues,
  parseRegisterSettingsValues,
  hasRegisterSettingsChanges,
  validateRegisterSettings,
  buildRegisterSettingsPatch,
} = require("./register-settings.js");

const registerPayload = {
  group: "register",
  values: {
    enabled: true,
    default_group: "user",
    email_domain_control_enabled: false,
    allowed_email_domains: "",
    email_verification_enabled: false,
  },
};

const authPayload = {
  group: "auth",
  values: {
    password_requirements_enabled: false,
    password_min_length: 8,
    password_complexity: "",
    session_max_age_enabled: false,
    session_max_age_days: 7,
    totp_enabled: false,
    password_reset_enabled: false,
    captcha_enabled: false,
    captcha_provider: "",
  },
};

const register = {
  enabled: true,
  emailDomainControlEnabled: false,
  allowedEmailDomains: "",
};
const auth = {
  passwordRequirementsEnabled: false,
  passwordMinLength: "8",
  passwordComplexity: "",
};
const values = { ...register, ...auth };

assert.deepEqual(parseRegisterGroupValues(registerPayload), register);
assert.deepEqual(parseAuthGroupValues(authPayload), auth);
assert.deepEqual(parseRegisterSettingsValues(registerPayload, authPayload), values);

assert.equal(parseRegisterGroupValues(null), null);
assert.equal(parseRegisterGroupValues({ group: "register" }), null);
assert.equal(
  parseRegisterGroupValues({
    ...registerPayload,
    values: { ...registerPayload.values, enabled: "true" },
  }),
  null,
);
assert.equal(
  parseAuthGroupValues({
    ...authPayload,
    values: { ...authPayload.values, password_min_length: 8.5 },
  }),
  null,
);
assert.equal(
  parseAuthGroupValues({
    ...authPayload,
    values: { ...authPayload.values, password_min_length: "8" },
  }),
  null,
);
assert.equal(parseRegisterSettingsValues(registerPayload, null), null);
assert.equal(parseRegisterSettingsValues(null, authPayload), null);

assert.equal(hasRegisterSettingsChanges(values, values), false);
assert.equal(
  hasRegisterSettingsChanges({ ...values, enabled: false }, values),
  true,
);
assert.equal(
  hasRegisterSettingsChanges({ ...values, allowedEmailDomains: "a.com" }, values),
  true,
);
assert.equal(
  hasRegisterSettingsChanges({ ...values, passwordMinLength: "08" }, values),
  true,
);

assert.equal(validateRegisterSettings(values, values), null);

assert.equal(
  validateRegisterSettings({ ...values, emailDomainControlEnabled: true }, values),
  "domainControlRequiresList",
);
assert.equal(
  validateRegisterSettings(
    { ...values, emailDomainControlEnabled: true, allowedEmailDomains: " , ，" },
    values,
  ),
  "domainControlRequiresList",
);
assert.equal(
  validateRegisterSettings(
    {
      ...values,
      emailDomainControlEnabled: true,
      allowedEmailDomains: "a.com，b.com",
    },
    values,
  ),
  null,
);
assert.equal(
  validateRegisterSettings({ ...values, enabled: false }, values),
  null,
);
assert.equal(
  validateRegisterSettings(
    { ...values, emailDomainControlEnabled: true, enabled: false },
    { ...values, emailDomainControlEnabled: true },
  ),
  null,
);

for (const bad of ["7", "65", "abc", "", "8.5"]) {
  assert.equal(
    validateRegisterSettings({ ...values, passwordMinLength: bad }, values),
    "passwordMinLengthRange",
  );
}
assert.equal(
  validateRegisterSettings({ ...values, passwordMinLength: "64" }, values),
  null,
);

assert.deepEqual(buildRegisterSettingsPatch(values, values), {});
assert.deepEqual(
  buildRegisterSettingsPatch({ ...values, enabled: false }, values),
  { register: { enabled: false } },
);
assert.deepEqual(
  buildRegisterSettingsPatch({ ...values, allowedEmailDomains: "a.com" }, values),
  { register: { allowed_email_domains: "a.com" } },
);
assert.deepEqual(
  buildRegisterSettingsPatch(
    {
      ...values,
      emailDomainControlEnabled: true,
      allowedEmailDomains: "a.com",
    },
    values,
  ),
  {
    register: {
      email_domain_control_enabled: true,
      allowed_email_domains: "a.com",
    },
  },
);
assert.deepEqual(
  buildRegisterSettingsPatch(
    { ...values, allowedEmailDomains: "" },
    { ...values, allowedEmailDomains: "a.com" },
  ),
  { register: { allowed_email_domains: "" } },
);
assert.deepEqual(
  buildRegisterSettingsPatch({ ...values, passwordMinLength: "10" }, values),
  { auth: { password_min_length: 10 } },
);
assert.deepEqual(
  buildRegisterSettingsPatch(
    { ...values, passwordRequirementsEnabled: true },
    values,
  ),
  { auth: { password_requirements_enabled: true } },
);
assert.deepEqual(
  buildRegisterSettingsPatch(
    { ...values, enabled: false, passwordComplexity: "x" },
    values,
  ),
  {
    register: { enabled: false },
    auth: { password_complexity: "x" },
  },
);

console.log("register-settings checks: PASS");
```

- [ ] **Step 3: 跑断言**

Run:
```bash
cd frontend && npx tsc lib/register-settings.ts --outDir ../.superpowers/sdd/2026-10-16-register-settings-wiring --module commonjs --target es2022 --skipLibCheck --strict && node ../.superpowers/sdd/2026-10-16-register-settings-wiring/check.cjs
```
Expected: `register-settings checks: PASS`（单文件 tsc 调用忽略 tsconfig、flags 必须自足且带 `--strict`；纯函数级离线断言——不碰应用 / 浏览器 / 接口）。

- [ ] **Step 4: 变异自检①（命中门槛）**

把 `validateRegisterSettings` 的「开 + 空」分支改成「只要最终态是开 + 空就拦」（不看命中）→ 重跑 Step 3。
Expected: FAIL——「只改允许公开注册不拦」与「旧态开 + 空但没动那两键不拦」两条断言红。还原 → 重跑绿。

- [ ] **Step 5: 变异自检②（只发变动键）**

把 `buildRegisterSettingsPatch` 改成恒发全部键（去掉 `!==` 比较）→ 重跑 Step 3。
Expected: FAIL——「无变化 → `{}`」与「只含变动键」多条断言红。还原 → 重跑绿。

- [ ] **Step 6: 变异自检③（整数把关）**

把 `parseAuthGroupValues` 的整数检查放宽为 `typeof === "number"`（去掉 `Number.isInteger`）→ 重跑 Step 3。
Expected: FAIL——`password_min_length: 8.5` 那条断言红。还原 → 重跑绿。

- [ ] **Step 7: 变异自检④（全角逗号）**

把 `splitDomains` 的分隔符只留半角 `,`（删 `，`）→ 重跑 Step 3。
Expected: FAIL——先炸在「纯分隔符（含全角逗号）→ 拦」断言（全角逗号被当成清单条目），「全角逗号分隔的有值清单 → 不拦」一类随之也会红。还原 → 重跑绿。

- [ ] **Step 8: 静态校验**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Expected: 全绿（新模块此刻无消费者，正常）。

- [ ] **Step 9: Commit（建议边界①，等用户点名）**

```bash
git add frontend/lib/register-settings.ts
git commit -m "feat(frontend): 注册设置页纯形状模块——两组解析、脏检测与组合校验"
```

`.superpowers/` 不入库。

---

### Task 2: 服务端读取两组 + 页面接线（只读）

**Files:**
- Modify: `frontend/lib/admin-settings.ts`
- Modify: `frontend/app/console/settings/register/page.tsx`（整页替换，见 Step 2）
- Modify: `frontend/app/console/settings/register/register-settings-form.tsx`（仅签名 + 初值，见 Step 3）

**Interfaces:**
- Consumes: Task 1 的 `RegisterSettingsValues`、`parseRegisterSettingsValues`
- Produces: `RegisterSettingsRead`（`ok` / `forbidden` / `failed` 判别联合）、`getRegisterSettings(): Promise<RegisterSettingsRead>`；表单新签名 `RegisterSettingsForm({ settings }: { settings: RegisterSettingsValues })`

- [ ] **Step 1: `lib/admin-settings.ts` 追加读取（import 补 `parseRegisterSettingsValues` / `type RegisterSettingsValues`）**

```ts
export type RegisterSettingsRead =
  | { status: "ok"; values: RegisterSettingsValues }
  | { status: "forbidden" }
  | { status: "failed" };

export async function getRegisterSettings(): Promise<RegisterSettingsRead> {
  const session = (await cookies()).get("session")?.value;

  if (!session) {
    return { status: "failed" };
  }

  try {
    const [registerResponse, authResponse] = await Promise.all([
      fetch(`${backendUrl}/api/v1/admin/settings/register`, {
        headers: { cookie: `session=${session}` },
        cache: "no-store",
      }),
      fetch(`${backendUrl}/api/v1/admin/settings/auth`, {
        headers: { cookie: `session=${session}` },
        cache: "no-store",
      }),
    ]);

    if (registerResponse.status === 403 || authResponse.status === 403) {
      return { status: "forbidden" };
    }

    if (!registerResponse.ok || !authResponse.ok) {
      return { status: "failed" };
    }

    const registerPayload = await registerResponse.json().catch(() => null);
    const authPayload = await authResponse.json().catch(() => null);
    const values = parseRegisterSettingsValues(registerPayload, authPayload);

    if (values === null) {
      return { status: "failed" };
    }

    return { status: "ok", values };
  } catch {
    return { status: "failed" };
  }
}
```

（403 判定放在 `ok` 判定之前——两组同源同权限，任一 403 即整页 `forbidden`；形状坏 → `failed`。）

- [ ] **Step 2: 替换 `page.tsx`（整页逐字；`generateMetadata` 保留）**

```tsx
import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { getRegisterSettings } from "@/lib/admin-settings";

import { RegisterSettingsForm } from "./register-settings-form";

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("settings");

  return {
    title: t("register"),
  };
}

export default async function SettingsRegisterPage() {
  const t = await getTranslations("settings");
  const read = await getRegisterSettings();

  return (
    <div className="mx-auto flex w-full max-w-[1000px] flex-col gap-4 p-4 min-[1200px]:p-6">
      <h1 className="text-xl font-semibold tracking-tight">{t("register")}</h1>
      {read.status === "ok" ? (
        <RegisterSettingsForm settings={read.values} />
      ) : (
        <p className="text-sm text-muted-foreground">
          {read.status === "forbidden" ? t("noPermission") : t("loadFailed")}
        </p>
      )}
    </div>
  );
}
```

（`noPermission` / `loadFailed` 两键站点轮已入库，本任务**不加键**。）

- [ ] **Step 3: 表单最小接线（初值来自 props；编译必须过）**

`register-settings-form.tsx` 三处小改（其余不动——开关仍无 `disabled`、没有保存）：

1. 加 `import type { RegisterSettingsValues } from "@/lib/register-settings";`
2. `export function RegisterSettingsForm() {` → `export function RegisterSettingsForm({ settings }: { settings: RegisterSettingsValues }) {`
3. 六个初值改为：`useState(settings.enabled)` / `useState(settings.emailDomainControlEnabled)` / `useState(settings.allowedEmailDomains)` / `useState(settings.passwordRequirementsEnabled)` / `useState(settings.passwordMinLength)` / `useState(settings.passwordComplexity)`（变量名沿用现状六个 state）。

- [ ] **Step 4: 静态校验**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Expected: 全绿。此时页面已直出真值（读成功时），但不能保存。

- [ ] **Step 5: Commit（建议边界②，等用户点名）**

```bash
git add frontend/lib/admin-settings.ts frontend/app/console/settings/register/page.tsx frontend/app/console/settings/register/register-settings-form.tsx
git commit -m "feat(frontend): 用户注册设置页服务端读取两组与读失败分支"
```

---

### Task 3: 受控表单 + 悬浮保存 + 校验（打通写路径）

**Files:**
- Create: `frontend/hooks/use-register-settings-save.ts`
- Modify: `frontend/app/console/settings/register/register-settings-form.tsx`
- Modify: `frontend/messages/zh-CN.json` / `frontend/messages/en.json`（`settings` 命名空间 +3 键）

**Interfaces:**
- Consumes: Task 1 的六个纯函数与类型；`lib/api.ts` 的 `ApiError` / `apiRequest` / `looksLikeServiceFailure`；`Input` / `Switch` 受控件；`ConsoleFloatingSaveButton`
- Produces:

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

- [ ] **Step 1: 写 `hooks/use-register-settings-save.ts`（逐字语义照 spec §4.3）**

要点逐条：`isDirty = hasRegisterSettingsChanges(values, baseline)`（脏判据与"能不能发"解耦——非法输入也算脏，保存按钮要出现让校验说话）；`save()` 顺序 = 短路 → **本地校验**（命中 → 对应文案 toast、return、不发送不清脏）→ `buildRegisterSettingsPatch` → **按组分别 PATCH**（`patch.register !== undefined` 才发 register，`patch.auth !== undefined` 才发 auth；回包用 `parseRegisterGroupValues` / `parseAuthGroupValues` 解析后 `next = { ...next, ...parsed }`；解析失败 → `saveFailed` 且不重置基线）→ 全部成功才 `setValues/setBaseline(next)` + `toast.success(t("saved"))` + `router.refresh()`；catch 顺序 = 401 静默 → 403 `noPermissionToSave` → `code === "setting_conflict"` 按 `param` 映射（`email_domain_control_enabled` / `allowed_email_domains` → `domainControlRequiresList`；`password_min_length` → `passwordMinLengthRange`；其余 → `saveFailed`）→ `looksLikeServiceFailure` → `common.serviceUnavailable` → 兜底 `saveFailed`。issue → 文案的映射用穷尽 switch（两个 case 都 return，无 default）。

- [ ] **Step 2: 表单改受控 + 包 `<form>` + 悬浮保存**

`register-settings-form.tsx`：

1. `import { useState } from "react";` → `import type { FormEvent } from "react";`（`useState` 不再用）。
2. 加 imports：`ConsoleFloatingSaveButton`（`@/components/console/console-floating-save-button`）、`useRegisterSettingsSave`（`@/hooks/use-register-settings-save`）。
3. 删六个 `useState`，换成：`const { values, isDirty, isSaving, setEnabled, setEmailDomainControlEnabled, updateAllowedEmailDomains, setPasswordRequirementsEnabled, updatePasswordMinLength, updatePasswordComplexity, save } = useRegisterSettingsSave(settings);`
4. 六个可接行受控（`disabled={isSaving}` 一律加上）：
   - 允许公开注册：`checked={values.enabled}` / `onCheckedChange={setEnabled}`
   - **注册默认组：原样不动**（不加 `value` / `onValueChange`）
   - 启用邮箱域名控制：`checked={values.emailDomainControlEnabled}` / `onCheckedChange={setEmailDomainControlEnabled}`
   - 允许的邮箱域名：`value={values.allowedEmailDomains}` / `onChange={(event) => updateAllowedEmailDomains(event.target.value)}`
   - 启用密码要求：`checked={values.passwordRequirementsEnabled}` / `onCheckedChange={setPasswordRequirementsEnabled}`
   - 密码最短长度：`value={values.passwordMinLength}` / `onChange={(event) => updatePasswordMinLength(event.target.value)}`（`type="number"` 保留，不加 `min` / `max`）
   - 密码复杂度：`value={values.passwordComplexity}` / `onChange={(event) => updatePasswordComplexity(event.target.value)}`
5. 加 `handleSubmit`：`event.preventDefault(); void save();`。
6. 外层 `<>...</>` → `<form onSubmit={handleSubmit} noValidate>...</form>`（两卡顺序不动）；表单末尾加 `<ConsoleFloatingSaveButton isDirty={isDirty} isPending={isSaving} label={t("save")} />`。

- [ ] **Step 3: 两份 messages 各加 3 键**

`settings` 命名空间（zh-CN 在 `"passwordComplexity"` 附近加，en 同键位），值照 spec §4.6 表：`noPermissionToSave` / `domainControlRequiresList` / `passwordMinLengthRange`。

- [ ] **Step 4: 键位比对自检**

Run:
```bash
cd frontend && node -e "const a=require('./messages/zh-CN.json'),b=require('./messages/en.json');console.log(['noPermissionToSave','domainControlRequiresList','passwordMinLengthRange'].every(k=>a.settings[k]&&b.settings[k]))"
```
Expected: `true`

- [ ] **Step 5: 静态校验 + 取证 grep（Review Focus 3 / 5 / 6）**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Expected: 全绿。

Run:
```bash
cd frontend && grep -c "useState" app/console/settings/register/register-settings-form.tsx; grep -c "onValueChange" app/console/settings/register/register-settings-form.tsx; grep -c "ConsoleFloatingSaveButton" app/console/settings/register/register-settings-form.tsx; grep -c "email_domain_control_enabled\|password_min_length" lib/register-settings.ts hooks/use-register-settings-save.ts; grep -c "email_domain_control_enabled\|password_min_length" app/console/settings/register/register-settings-form.tsx app/console/settings/register/page.tsx
```
Expected: `0`（表单不再有自己的 state）、`0`（默认组 Select 未接）、`2`（import + 使用）、`7` 与 `2`（蛇形键只许在 `lib/register-settings.ts`——映射表 2 处 + 解析函数读 raw 键 5 处——与 `hooks/use-register-settings-save.ts` 的 409 param 2 处）、`0` 与 `0`（表单与页面零蛇形键）。

- [ ] **Step 6: Commit（建议边界③，等用户点名）**

```bash
git add frontend/hooks/use-register-settings-save.ts frontend/app/console/settings/register/register-settings-form.tsx frontend/messages/zh-CN.json frontend/messages/en.json
git commit -m "feat(frontend): 用户注册设置页接线——受控表单、本地校验与按组保存"
```

---

### Task 4: DEVLOG 登记 + 交付清单

**Files:**
- Modify: `docs/DEVLOG.md`（顶部 `---` 之后倒序插入新一节）

- [ ] **Step 1: 写 DEVLOG 一节**

标题 `## 2026-10-16 · 控制台「用户注册」设置页接后端（register + auth 两组六键，纯前端）`。必须包含：D1–D11 口径摘要；**变异自检四条实测输出**（Task 1 Step 4–7）；**已知缺口**逐条（B1 两次 PATCH 非事务、B3 未保存无守卫、B4 多标签页覆盖、B5 文本相等边界、B6 客户端与后端常量一致性、B7 读两组同源同坏）；**「注册默认组」不接的理由**（值形状待裁定）；**提交批次**（按实际发生记录）。

- [ ] **Step 2: 全量静态校验 + 文件核对**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Run: `git -C .. status --short`
Expected: 校验全绿；改动只含本轮 7 个文件 + `docs/DEVLOG.md`（`.superpowers/` 不出现）。

- [ ] **Step 3: 交付（给用户的验收清单——不代跑）**

正文里逐条给出"打开哪条 URL 看什么"：

1. owner 打开 `/console/settings/register` → 六行显示库里真值（改过 `register.enabled` 就应看到关）。
2. 改「允许公开注册」→ 右下角浮出「保存」→ 点保存 → toast「已保存」→ 刷新后仍是新值。
3. 开「启用邮箱域名控制」+ 清空域名 → 保存 → toast「要开启邮箱域名控制，请先填写允许的邮箱域名」（**本地拦截**）。
4. 域名填 `example.com` → 保存成功；密码最短长度填 `7` → 保存 → toast「密码最短长度需在 8–64 之间」。
5. 密码最短长度填 `10`、密码复杂度填几个字 → 保存 → 刷新仍在（复杂度**存了但不生效**，预期如此）。
6. 冒烟账号（普通用户）直链打开 → 应显示「无权查看设置」（同时证明 cookie 转发通）。
7. 停掉后端进程 → 刷新 → 应显示「设置读取失败」而不是空表单。
8. 改任一开关后按 Ctrl/Cmd+S → 应触发保存（悬浮按钮的 owning form 命中）。

- [ ] **Step 4: Commit（建议边界④，等用户点名）**

```bash
git add docs/DEVLOG.md
git commit -m "docs(devlog): 登记用户注册设置页接线——两组读取、按组保存与校验门槛"
```

---

## 执行方式

待用户点名：**Native**（本会话逐任务实现）或子代理驱动。两条既有约束不变：运行期验收归用户（执行者只跑静态校验与离线纯函数断言）；提交等用户点名（Commit 步骤里的分批边界照用）。

## 执行结果（2026-10-16，native 执行）

- **Task 1**：`lib/register-settings.ts` 落盘；断言 `register-settings checks: PASS`；**四条变异实测红**——① 校验去掉命中门槛 →「只改允许公开注册」断言红（`domainControlRequiresList` vs `null`）；② patch 恒发全部键 →「无变化 → `{}`」断言红；③ 放宽整数把关 → `8.5` 被放行、断言红；④ 全角逗号不认 → 先炸在「纯分隔符（含全角）→ 拦」断言。
- **Task 2 / 3**：读两组 + 两分支渲染 + 表单受控 + 悬浮保存 + 3 条 i18n 键；键位比对 `true`；取证 grep 见 Task 3 Step 5——其中 `lib/register-settings.ts` 的蛇形键实测为 `7`（映射表 2 + 解析函数读 raw 键 5），预期已按实测修正。
- **静态门**：`npx tsc --noEmit` / `npm run lint`（0 error，2 条 `sonner.tsx` 存量 warning）/ `npm run format:check` 全绿。
- **未做**：运行期验收（归用户，清单见 Task 4 Step 3）；提交（等点名，边界见各任务 Commit 步骤）。
- **收口（同日稍晚）**：并行轮把「开关开 + 清单空」改判为**不拦**（后端 register 组摘掉组级校验、前端摘掉本地域名拦截与 `domainControlRequiresList` 键，空清单时给域名输入框挂 placeholder 提示；权威口径见 DEVLOG「注册域名控制『空清单』改为放行」条目）。本计划正文与 spec 正文的域名相关口径**原样留存、已被覆盖**；断言脚本已同步（删 domain 校验分支、补「开 + 空 → null」两条，重新编译跑 **PASS**）；`splitDomains` 由并行轮改为导出函数 `domainListIsEmpty`（表单以 `placeholder` + 行内 `description` 双通道提示）以仓库现文件为准。
