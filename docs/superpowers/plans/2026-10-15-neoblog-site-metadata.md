# 站点设置接入前台（标题 / 元数据 / robots）+ 控制台页面标题 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.（执行方式由用户点名；本计划按本仓既有口径：**运行期验收归用户、提交等点名**。）

**Goal:** 把 `site` 组六键接进前台的真实位置（站点级标题拼装、meta 描述 / 关键词、OG 三文字字段、metadataBase、robots.txt + robots meta），并给四个控制台子页补上跟导航一致的标签标题。

**Architecture:** 新增零 import 的纯模块 `lib/site-metadata.ts`（六键选择器 + 标题拼装 + URL 校验，离线断言可测）；根 `layout.tsx` 静态 metadata → `generateMetadata()` 读公开设置；新增 `app/robots.ts`（ISR 60 秒）；公开设置读取链从 `no-store` 改 60 秒缓存（注册闸 / 元数据 / robots 共用，用户已知并接受连带延迟）。

**Tech Stack:** Next.js 16.4.0（App Router、`cacheComponents: true`、`instant = false`）、React 19.3.0、next-intl、既有 `lib/public-settings.ts` 读取层。

**Spec:** `docs/superpowers/specs/2026-10-15-neoblog-site-metadata-design.md`（本计划按 D1–D9 与 §4.1–§4.4 论证；两份一起读）

## Global Constraints

- 不新增依赖；不引入测试运行器；离线断言脚本放 `.superpowers/sdd/<本计划名>/`（根 `.gitignore` 已覆盖 `.superpowers/`；**别放 `frontend/.scratch/`**——它被 eslint / prettier 扫描，上一轮的教训）。
- **运行期验收归用户**：执行者不跑浏览器、不 curl 页面；`tsc` / `eslint` / `prettier --check`、离线纯函数断言、`next build` 照跑（后两者是路由配置与缓存的静态证据）。
- **不主动提交**：各任务 Commit 步骤只是建议分批边界；实际提交等用户点名。
- 口径：**配了才输出、一律不兜底**（空值不产出字段，不拿语言包编文字）；子页面标题不加任何后缀；控制台标题不加「\| 网站后台」；OG 只出 title / description / siteName 三个文字字段。
- 缓存 60 秒口径统一（读取链 + robots），不实时；唯一例外是页面 HTML 本身（`instant = false` 维持现状）。
- 不动：`/signin`、`/signup`、`/console` 首页（标题现状）；侧栏与页面 h1（「常规」不改名）；`backend/**`；`app/ui-check`、`app/tabs-demo`；`components/**`。
- 校验命令统一在 `frontend/` 下：`npx tsc --noEmit && npm run lint && npm run format:check`；`.next` 类型过期导致 tsc 报"找不到已删除路由"时先跑 `npx next typegen`。
- i18n 中文以 `zh-CN.json` 为准、`en.json` 同键位；漏键不抛错（只 console.error + 渲染键路径），加键任务必须跑键位比对自检。

## Review Focus

spec 隐含但无自动测试兜底、最可能咬人的六条，逐条钉在对应任务：

1. **标题拼装不产生悬空分隔符**（`"A - "` 永不出现；空段 / 纯空白段丢弃）→ Task 1 断言 + 变异①。
2. **关键词拆分**（半角 / 全角逗号、逐段 trim、丢空段）→ Task 1 断言 + 变异②。
3. **非法 URL 不能把元数据生成炸掉**：`siteBaseUrl` 只有 `new URL` 成功才返回、否则 `null`，根 layout 只判 `null` → Task 1 断言 + Task 3。
4. **fail-open 不回归**：公开设置读不到 = 全空（标题 / 描述 / 关键词 / OG 都不出、robots 走"放行 + /console disallow"），页面照常渲染 → Task 1 断言（缺组 / 缺键 / 非字符串）+ Task 3 / Task 4 语义 + 用户实机。
5. **robots 关闭态必须留 `/console` disallow**（不能缩成空规则）→ Task 4 步骤 + 用户实机对照。
6. **en 漏键不抛错**（渲染键路径而不是白屏）→ Task 5 键位比对。

---

## 文件结构

| 文件 | 动作 | 唯一职责 |
| --- | --- | --- |
| `frontend/lib/site-metadata.ts` | Create | 纯模块：六键选择器 + 标题拼装 + URL 校验（零 import） |
| `frontend/lib/public-settings.ts` | Modify | 读取由 `no-store` 改 60 秒缓存 |
| `frontend/app/layout.tsx` | Modify | metadata → `generateMetadata()`（标题 / 描述 / 关键词 / OG / metadataBase / robots meta） |
| `frontend/app/robots.ts` | Create | `MetadataRoute.Robots`，60 秒缓存 |
| `frontend/app/console/uc/profile/page.tsx` | Modify | +`generateMetadata`（个人信息） |
| `frontend/app/console/uc/session/page.tsx` | Modify | +`generateMetadata`（登录会话） |
| `frontend/app/console/settings/general/page.tsx` | Modify | +`generateMetadata`（站点设置） |
| `frontend/app/console/settings/register/page.tsx` | Modify | +`generateMetadata`（用户注册） |
| `frontend/messages/zh-CN.json` / `en.json` | Modify | +1 键 `settings.siteSettings` |
| `docs/DEVLOG.md` | Modify | 顶部倒序登记（T6） |

Task 1 只造零件不接线；Task 2 单独落缓存（行为面在登录 / 注册页可见——注册闸跟着 60 秒）；Task 3 / 4 / 5 各自独立。

---

### Task 1: 纯模块 `lib/site-metadata.ts`

**Files:**
- Create: `frontend/lib/site-metadata.ts`
- Create（一次性、不入库）: `.superpowers/sdd/2026-10-15-neoblog-site-metadata/site-metadata-check/check.cjs`

**Interfaces:**
- Consumes: 无（零 import、参数收 `unknown`）
- Produces（Task 3 / Task 4 按此签名调用，名称逐字一致）:

```ts
export function siteTitle(settings: unknown): string;
export function siteSubtitle(settings: unknown): string;
export function composeSiteTitle(title: string, subtitle: string): string;
export function siteDocumentTitle(settings: unknown): string;
export function siteDescription(settings: unknown): string;
export function siteKeywords(settings: unknown): string[];
export function siteBaseUrl(settings: unknown): URL | null;
export function blocksSearchEngines(settings: unknown): boolean;
```

- [ ] **Step 1: 写离线断言脚本（RED 先）**

先建目录与脚本：`mkdir -p .superpowers/sdd/2026-10-15-neoblog-site-metadata/site-metadata-check`，在其中写 `check.cjs`：

```js
const assert = require("node:assert/strict");
const {
  siteTitle,
  siteSubtitle,
  composeSiteTitle,
  siteDocumentTitle,
  siteDescription,
  siteKeywords,
  siteBaseUrl,
  blocksSearchEngines,
} = require("./site-metadata.js");

const settings = {
  site: {
    title: "My Blog",
    subtitle: "写点东西",
    description: "d",
    keywords: "go, blog ,, dev",
    url: "https://example.com/base/",
    block_search_engines: true,
  },
};

// 选择器：取值与 trim
assert.equal(siteTitle(settings), "My Blog");
assert.equal(siteTitle({ site: { title: "  X  " } }), "X");
assert.equal(siteSubtitle(settings), "写点东西");
assert.equal(siteDescription(settings), "d");
assert.equal(siteDocumentTitle(settings), "My Blog - 写点东西");

// 标题拼装：非空段 " - " 连接、无悬空分隔符（Review Focus 1）
assert.equal(composeSiteTitle("A", "B"), "A - B");
assert.equal(composeSiteTitle("A", ""), "A");
assert.equal(composeSiteTitle("", "B"), "B");
assert.equal(composeSiteTitle("A", "   "), "A");
assert.equal(composeSiteTitle("  A  ", "B"), "A - B");
assert.equal(composeSiteTitle("", ""), "");

// 关键词：半角/全角逗号、trim、丢空段（Review Focus 2）
assert.deepEqual(siteKeywords(settings), ["go", "blog", "dev"]);
assert.deepEqual(siteKeywords({ site: { keywords: "a，b,c" } }), ["a", "b", "c"]);

// URL：非法 / 空 = null（Review Focus 3）
assert.equal(siteBaseUrl(settings)?.href, "https://example.com/base/");
assert.equal(siteBaseUrl({ site: { url: "not a url" } }), null);
assert.equal(siteBaseUrl({ site: { url: "" } }), null);

// 开关与缺组缺键（fail-open，Review Focus 4）
assert.equal(blocksSearchEngines(settings), true);
assert.equal(blocksSearchEngines({}), false);
assert.equal(siteTitle({}), "");
assert.equal(siteTitle({ site: { title: 42 } }), "");
assert.deepEqual(siteKeywords({}), []);
assert.equal(siteDocumentTitle({}), "");

console.log("site-metadata checks: PASS");
```

- [ ] **Step 2: 跑断言验证 RED**

Run:
```bash
cd frontend && npx tsc lib/site-metadata.ts --outDir ../.superpowers/sdd/2026-10-15-neoblog-site-metadata/site-metadata-check --module commonjs --target es2022 --skipLibCheck && node ../.superpowers/sdd/2026-10-15-neoblog-site-metadata/site-metadata-check/check.cjs
```
Expected: FAIL——`TS6053: File 'lib/site-metadata.ts' not found`（模块不存在，失败原因吻合）。

- [ ] **Step 3: 写模块（GREEN）**

语义逐条照 spec §4.1：缺组 / 缺键 / 非字符串一律空值；字符串先 trim 再判空；`composeSiteTitle` 过滤空段后 `" - "` 连接（**别用模板串直接拼**）；`siteKeywords` 用 `/[,，]/` 拆分；`siteBaseUrl` 用 `try { new URL(raw) } catch { null }`（空串直接 `null`）。

- [ ] **Step 4: 跑断言验证 GREEN**

Run: 同 Step 2 命令
Expected: `site-metadata checks: PASS`

- [ ] **Step 5: 变异自检①（悬空分隔符）**

把 `composeSiteTitle` 改成朴素模板串直拼（如 `[title, subtitle].join(" - ")`，空段过滤没了）→ 重跑 Step 2。
Expected: FAIL——`composeSiteTitle("A", "   ")` 一条红（实际 `"A -    "`、期望 `"A"`；`("", "B")` 那条也会红成 `" - B"`）。还原 → 重跑绿。

- [ ] **Step 6: 变异自检②（关键词拆分）**

把 `siteKeywords` 改成只按 `,` 拆、不 trim 不过滤（`raw.split(",")`）→ 重跑 Step 2。
Expected: FAIL——`["go", " blog ", "", " dev"]` 之类与期望不符。还原 → 重跑绿。

- [ ] **Step 7: 静态校验**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Expected: 全绿（新模块此刻无消费者，正常；prettier 若报格式，对单文件 `npx prettier --write` 后复跑）。

- [ ] **Step 8: Commit（建议边界①，等用户点名）**

```bash
git add frontend/lib/site-metadata.ts
git commit -m "feat(frontend): 站点元数据纯模块 site-metadata——选择器、标题拼装与 URL 校验"
```

`.superpowers/` 不入库。

---

### Task 2: 公开设置读取改 60 秒缓存

**Files:**
- Modify: `frontend/lib/public-settings.ts`（`getPublicSettings` 的 fetch 选项）

**Interfaces:**
- Consumes: 无
- Produces: 行为不变的 `getPublicSettings()`（失败仍返回 `{}`）——但**读数改为 60 秒缓存**；登录 / 注册页的注册闸跟随该口径（spec D1 的连带效果）

- [ ] **Step 1: 改缓存口径**

`getPublicSettings` 里 `fetch(..., { cache: "no-store" })` → `fetch(..., { next: { revalidate: 60 } })`。其余（fail-open、形状校验）一字不动。

**若 `npm run build` 拒绝该写法**（Next 16 `cacheComponents: true` 的路由配置限制）：改用 `unstable_cache` 包住读取（`{ revalidate: 60 }`），对外函数签名与 fail-open 行为不变；把实际采用的形式记进 DEVLOG。

- [ ] **Step 2: 静态校验 + 构建**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check && npm run build`
Expected: 全绿 / build 成功。取证：`grep -c "no-store" lib/public-settings.ts` → `0`。

- [ ] **Step 3: Commit（建议边界②，等用户点名）**

```bash
git add frontend/lib/public-settings.ts
git commit -m "perf(frontend): 公开设置读取由每请求改 60 秒缓存（注册闸随之）"
```

---

### Task 3: 根 `layout.tsx` 元数据接入

**Files:**
- Modify: `frontend/app/layout.tsx`（只换 metadata 导出与 imports；字体 / ThemeSync / Toaster / `instant = false` 一律不动）

**Interfaces:**
- Consumes: Task 1 的全部选择器 / 拼装函数；`getPublicSettings`（Task 2 后的缓存版）
- Produces: 站点级默认标题（`站名 - 副标题`）、description / keywords / metadataBase / `robots: { index: false }`（仅开关开）、openGraph 三文字字段

- [ ] **Step 1: 替换 metadata 导出**

删 `export const metadata: Metadata = { title: "Create Next App", description: "Generated by create next app" }`，改为（语义照 spec §4.2，形状如下）：

```tsx
export async function generateMetadata(): Promise<Metadata> {
  const settings = await getPublicSettings();
  const title = siteDocumentTitle(settings);
  const siteName = siteTitle(settings);
  const description = siteDescription(settings);
  const keywords = siteKeywords(settings);
  const baseUrl = siteBaseUrl(settings);
  const openGraph = {
    ...(title === "" ? {} : { title }),
    ...(description === "" ? {} : { description }),
    ...(siteName === "" ? {} : { siteName }),
  };

  return {
    ...(title === "" ? {} : { title }),
    ...(description === "" ? {} : { description }),
    ...(keywords.length === 0 ? {} : { keywords }),
    ...(baseUrl === null ? {} : { metadataBase: baseUrl }),
    ...(blocksSearchEngines(settings) ? { robots: { index: false } } : {}),
    ...(Object.keys(openGraph).length === 0 ? {} : { openGraph }),
  };
}
```

imports 增补：`getPublicSettings`（`@/lib/public-settings`）、`siteDocumentTitle` / `siteTitle` / `siteDescription` / `siteKeywords` / `siteBaseUrl` / `blocksSearchEngines`（`@/lib/site-metadata`）。`Metadata` 类型 import 保留。

- [ ] **Step 2: 静态校验 + 构建**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check && npm run build`
Expected: 全绿。取证：`grep -c "Create Next App" app/layout.tsx` → `0`。

- [ ] **Step 3: Commit（建议边界③，等用户点名）**

```bash
git add frontend/app/layout.tsx
git commit -m "feat(frontend): 根 layout 元数据接入站点设置（标题拼装 / 描述 / 关键词 / OG / metadataBase）"
```

---

### Task 4: `app/robots.ts`

**Files:**
- Create: `frontend/app/robots.ts`

**Interfaces:**
- Consumes: `getPublicSettings`（缓存版）、`blocksSearchEngines`（Task 1）
- Produces: `/robots.txt`——开关开 = 全站 `Disallow: /`；关 = 放行 + `/console` disallow

- [ ] **Step 1: 写路由**

```ts
import type { MetadataRoute } from "next";

import { getPublicSettings } from "@/lib/public-settings";
import { blocksSearchEngines } from "@/lib/site-metadata";

export const revalidate = 60;

export default async function robots(): Promise<MetadataRoute.Robots> {
  const settings = await getPublicSettings();

  if (blocksSearchEngines(settings)) {
    return { rules: [{ userAgent: "*", disallow: "/" }] };
  }

  return { rules: [{ userAgent: "*", allow: "/", disallow: "/console" }] };
}
```

若 `npm run build` 拒绝 `export const revalidate`（cacheComponents 限制），则删掉该行、把缓存落在数据读取层（Task 2 的 `unstable_cache` 兜底口径），行为口径不变（≤60 秒生效、非每请求）；实际采用形式记进 DEVLOG。

- [ ] **Step 2: 静态校验 + 构建**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check && npm run build`
Expected: 全绿；build 输出里出现 `/robots.txt` 路由。取证：`grep -c "disallow" app/robots.ts` → `2`（两个返回分支各一）。

- [ ] **Step 3: Commit（建议边界④，等用户点名）**

```bash
git add frontend/app/robots.ts
git commit -m "feat(frontend): robots.txt 接入屏蔽搜索引擎开关（60 秒缓存，/console 永久 disallow）"
```

---

### Task 5: 控制台四页标题 + 文案键

**Files:**
- Modify: `frontend/app/console/uc/profile/page.tsx`、`frontend/app/console/uc/session/page.tsx`、`frontend/app/console/settings/general/page.tsx`、`frontend/app/console/settings/register/page.tsx`
- Modify: `frontend/messages/zh-CN.json` / `frontend/messages/en.json`（+`settings.siteSettings`）

**Interfaces:**
- Consumes: 既有 `getTranslations`（服务端）
- Produces: 四页标签标题（个人信息 / 登录会话 / 站点设置 / 用户注册）；新文案键 `settings.siteSettings`（zh「站点设置」/ en「Site settings」）

- [ ] **Step 1: 两份 messages 加键**

`settings` 命名空间加 `siteSettings`（zh 在 `"siteGroup"` 附近、en 同键位）：zh `"站点设置"`、en `"Site settings"`。

- [ ] **Step 2: 四个页面各加 `generateMetadata`**

形状逐字（以 profile 为例，其余三页只换键名与 namespace）：

```ts
export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("sidebar");

  return { title: t("personalInfo") };
}
```

| 页面 | namespace | 键 |
| --- | --- | --- |
| `uc/profile` | `sidebar` | `personalInfo` |
| `uc/session` | `sidebar` | `sessions` |
| `settings/register` | `settings` | `register` |
| `settings/general` | `settings` | `siteSettings` |

各页原有内容（h1 等）一字不动；`Metadata` 类型 import 按现有子页先例加。

- [ ] **Step 3: 键位比对自检（Review Focus 6）**

Run:
```bash
cd frontend && node -e "const a=require('./messages/zh-CN.json'),b=require('./messages/en.json');console.log(Boolean(a.settings.siteSettings&&b.settings.siteSettings))"
```
Expected: `true`

- [ ] **Step 4: 静态校验**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check`
Expected: 全绿。

- [ ] **Step 5: Commit（建议边界⑤，等用户点名）**

```bash
git add frontend/app/console/uc/profile/page.tsx frontend/app/console/uc/session/page.tsx frontend/app/console/settings/general/page.tsx frontend/app/console/settings/register/page.tsx frontend/messages/zh-CN.json frontend/messages/en.json
git commit -m "feat(frontend): 控制台四页标签标题跟导航（站点设置 / 个人信息 / 登录会话 / 用户注册）"
```

---

### Task 6: DEVLOG 登记 + 交付清单

**Files:**
- Modify: `docs/DEVLOG.md`（顶部 `---` 之后倒序插入新一节）

**Interfaces:**
- Consumes: Task 1–5 的实现、变异自检记录、build 结果（Task 2 / 4 的缓存形式）
- Produces: 本轮决策与已知缺口入库；交用户的验收清单

- [ ] **Step 1: 写 DEVLOG 一节**

标题 `## 2026-10-15 · 站点设置接入前台（元数据 / robots）+ 控制台页面标题`。必须包含：D1–D9 口径摘要；缓存实际采用的形式（`next.revalidate` 还是 `unstable_cache`）；**变异自检两条的实测输出**；**已知缺口**：B1（注册闸随缓存 ≤60 秒延迟）、B2（全空配置 = 无标题无描述，不兜底）、B5（general 页标题与侧栏名两处键）、B7（`/console` disallow 硬编码）、B8（无测试运行器）；提交批次。

- [ ] **Step 2: 全量校验 + 文件核对**

Run: `cd frontend && npx tsc --noEmit && npm run lint && npm run format:check && npm run build`
Run: `git -C . diff --stat HEAD`
Expected: 校验全绿、build 成功；diff 只含本轮 9 个文件 + DEVLOG。

- [ ] **Step 3: 交付（给用户的验收清单——不代跑）**

1. 后台把「站点标题」「副标题」配上 → **等最多 1 分钟** → 刷新 `/`：标签应显示「站名 - 副标题」；
2. 只配站名 / 只配副标题各试一次：标签分别是那一段、没有多余的「 - 」；
3. 浏览器查看 `/` 的页面源码：`<meta name="description">`、`<meta name="keywords">`、`og:title` 等只在该配的配了时出现；
4. 「屏蔽搜索引擎」开 → 等最多 1 分钟 → `/robots.txt` 应全站 `Disallow: /`；关 → 应放行但有一行 `Disallow: /console`；
5. 控制台四页的标签：个人信息 / 登录会话 / **站点设置** / 用户注册；`/signin` 仍是「欢迎回来」（无后缀）；
6. （可选）后端停着打开 `/`：页面照常、标题为空——fail-open 不炸。

- [ ] **Step 4: Commit（建议边界⑥，等用户点名）**

```bash
git add docs/DEVLOG.md
git commit -m "docs(devlog): 登记站点设置接入前台——缓存口径与已知缺口"
```

---

## 执行方式

待用户点名：**Native**（本会话逐任务实现）或子代理驱动。两条既有约束不变：运行期验收归用户（执行者只跑静态门禁、离线断言与 `next build`）；提交等用户点名（Commit 步骤里的分批边界照用）。
