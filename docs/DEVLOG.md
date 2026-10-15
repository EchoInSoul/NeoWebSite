# 开发日志

按时间**倒序**记录每个开发阶段的变更、决策与已知缺口。设计细节与实施计划在 `docs/superpowers/{specs,plans}/`；接口与结构规范以 `backend/README.md` 为准。

---

## 2026-10-16 · 权限矩阵按页面拆行（新增 system.register 键，前后端 0.21.0）

**范围**：前端（用户组弹窗矩阵 + 侧栏三项门控）+ 后端（权限目录 + 设置接口守卫按组分流）；版本 0.20.1 → 0.21.0。

**动机**：用户点名——「设置项下有 3 个（常规 / 用户注册 / 用户组），权限节点就应该显示这 3 个」。原矩阵三行是后端抽象资源（用户 / 用户组 / 设置），其中「设置」一把钥匙同时管着「常规」与「用户注册」两个页面；用户在两个方案里选了「拆成独立权限」。

**本轮口径**：

- **后端**：目录加 `system.register.view` / `.edit`（名称「查看 / 管理用户注册」）；目录顺序调整为 users → settings → register → groups（目录顺序只决定 `/admin/permissions` 的返回顺序；前端矩阵的行序与分组由 `frontend/lib/groups.ts` 的分组表定）。设置接口 `GET / PATCH /admin/settings/:group` 守卫由单一 `system.settings.*` 改为**按组分流**（`requireGroupPermission`）：`site` 组 → `system.settings.*`，`register` / `auth` 组 → `system.register.*`；未知组仍落到 settings 档、由服务层 404。
- **前端**：矩阵标签 `system.settings` 行「设置」→「常规」、新增 `system.register` 行「用户注册」（i18n 键 `resourceGeneral` / `resourceRegister` 替换 `resourceSettings`，共 48 键）；侧栏「常规」「用户注册」两项按各自 view 权限显隐（与用户组一致；三枚布尔 layout → sidebar → nav 下传；**三项全无权限时 main 面板「设置」入口整体不渲染**，避免点进空面板）；弹窗矩阵**表头对齐修复**——原表头是「勾选框 + 文字」整体居中，与正文列按勾选框居中的位置错开小半个字，改为「文字在上、勾选框在下」各自居中，勾选框与列严格对齐。
- **侧栏再分组**（同轮后续，用户点名）：设置子面板从「一个组标签 + 三项」改为**三个分组**——设置（常规）/ 注册与登录（用户注册）/ 权限组（用户组），组间留白（后续分组标签 `mt-4`）；空组（无权限项）整组不渲染；`ConsoleSidebarSection` 改为接收 `groups: { label, items }[]`，uc 面板同步改形（单组）。
- **矩阵分三组 + 暂隐藏「用户」**（同轮后续，用户点名）：弹窗矩阵照侧栏拆三组（设置 / 注册与登录 / 权限组）加小标题、组间留白；小组标题复用 `sidebar` 命名空间同款文案（与侧栏一字不差，标签 2026-10-16 按用户点名由「登录与注册」改为「注册与登录」）。**「用户」行暂不显示**——用户页未建：`system.users.*` 键仍在后端目录、owner / admin 自动持有；自建组暂不能新授「用户」权限，已有该权限的自建组保存时原样保留（隐藏键不进勾选、不清除）。列表「权限点数」改为只计可见资源（排序同步），与弹窗可勾数量一致（owner 由 8 → 6）。分组表在 `frontend/lib/groups.ts`（`MATRIX_GROUPS`），与后端目录顺序解耦；离线断言补分组 / 隐藏用例，并做过变异自检（把 users 塞回分组表 → 断言红，还原复绿）。
- **删只读提示文案**（同轮后续，用户点名）：预设组只读弹窗底部的两档提示行（owner·admin / user 文案）删除，`readOnlyPreset` / `readOnlyPrivileged` 两键同轮退役（`groups` 命名空间 48 → 46 键）；可写弹窗的「勾选『编辑』会自动勾选『查看』。」保留。
- **工具栏排布**（同轮后续，用户点名）：「新建」按钮（点名后文案由「新建用户组」改「新建」、档位 `sm`→`default`，32 → 36px）由右槽移至左槽，右侧只保留刷新按钮。
- **权限语义变化**：自建组原持有的 `system.settings.*` 现在只管「常规」页；要管「用户注册」页须另授 `system.register.*`（owner / admin 由代码全量授予、自动包含；已存在的自建组需在用户组页重新勾选）。无数据迁移。
- **文档同步**：`backend/README.md`（模块表 / 用户组与权限节 / 设置节接口表）、用户组 spec（D6、§5 顺序、§7 键表、§12 时间线）、实施计划（口径修订段追加行）。

**验证（本机）**：后端全量测试绿（`scripts/test.sh` 串行；新增 `TestSettingsGroupPermissionsSplitByGroup` 覆盖 register / auth / site 三组 × 读写的两把钥匙穿越矩阵）；**变异自检**：把守卫改回单钥匙 → 新测试红（`status = 403, want 200`）→ 还原复绿；`gofmt -l` 干净。前端：离线断言 `groups checks: PASS`（含四资源标签映射）；`tsc` / `eslint`（0 error，2 条 `sonner.tsx` 存量 warning）/ `prettier --check` 全绿。**运行期未验**——矩阵四行、侧栏三项显隐、表头对齐交用户实机。

**提交前独立评审（2026-10-16，只读子代理，requesting-code-review）**：无 Critical（嵌套 guard 的 handler 执行次数 / abort 语义、隐藏键往返、三态与计数一致性三项逐行验证成立）；1 Important **已修**——busy 锁没盖到 ⋯ 行菜单与「新建」按钮（spec §4.1 明文要求，`ConsoleListTable` 的 `isBusy` 实际只作用于行选择框），现行菜单触发钮与编辑菜单项、新建按钮均随 `isListLocked` 禁用，并删掉传给表格的无效 `isBusy`；另修两处 Minor（列头在无可见键时禁用、refresh 403 改用 `forbidden` 文案）并补三处测试断言（未知组两档位、register 钥匙 PATCH `auth`、隐藏键勾选往返——变异自检：`auth` 档位一摘测试即红）。余下 Minor（`canViewSettings` 命名、settings 组 tier 数据化、`/nav` 入口 href 取首个可访问项）留待点名。

**提交状态**：随本轮三批入库——① `feat(backend)`（权限目录与按组守卫，含 0.21.0 版本进位）② `feat(frontend)`（用户组接线 + 矩阵分组 + 侧栏分组 + 文案清理）③ `docs`（本节与 spec / 计划）；**未推送**。

---

## 2026-10-16 · 控制台「用户组」页接后端（access 接口，纯前端）

**范围**：纯前端，后端零改动，零新依赖。spec：`docs/superpowers/specs/2026-10-15-neoblog-console-groups-design.md`（v3）；计划：`docs/superpowers/plans/2026-10-15-neoblog-console-groups.md`（v2 + 口径修订段 + 执行结果节）。

**动机**：用户组页自 b5ad652 起是纯 UI（本地空数组 + 写死的 6 条权限清单，"新建 / 编辑 / 删除"只活在内存里）。后端 access 八条接口（0.5.0）与 `/users/me` 的 `permissions` 字段早就在位；本轮按用户点名「开始执行，直接执行；移除掉老的数据」把页面接到真接口。

**本轮口径（D1–D22 摘要，含两处随轮修订）**：

- **页面落设置子项** `/console/settings/groups`（原定的顶层 `/console/groups` 作废）：侧栏「设置」子面板第三项，标签复用 `settings.groups`；**搜索框删除**（用户点名）；表格视图保留（刷新 / 新建 / ⋯ 行菜单 / 权限矩阵弹窗）。
- **读**：服务端直读 `lib/admin-groups.ts`（`GET /api/v1/admin/groups` + `GET /api/v1/admin/permissions`，cookie 转发 + `cache:"no-store"`）；读失败 / 403 → 不渲染列表；**权限清单从接口取**（前端零硬编码清单，资源 / 动作动态推导）。
- **门控**：`SessionUser` 补解析 `/users/me` 的 `permissions`；`system.groups.view` 控侧栏项显隐（layout → sidebar → nav 三层传参）、`system.groups.edit` 控「新建」与操作列（`canEdit=true` 才传 `rowActions` / `actionsHeader`）。
- **写**：`use-groups.ts`（对齐 `use-sessions`）：新建 `POST` / 编辑 `PATCH`（整份 `{name, permissions}`）/ 删除 `DELETE`；成功后用响应对象就地更新列表（**不调 `router.refresh()`**）；删除 404 视为成功；401 静默（全局会话失效弹窗接管）；错误按码映射 i18n（`preset_group_locked` / `group_name_taken` / `group_not_empty` / `grant_forbidden` / `forbidden` / `not_found` 全列，兜底按 create / edit / delete 分档）。
- 唯一硬编码规则 = `edit⇒view` 蕴含（对齐后端 `rbac.impliedActions`）；展示计数用后端已展开的 `permissions`，提交原始勾选集合。
- 轮次口径：无版本进位（前端轮惯例，同 f512409）；随本轮三批入库（**未推送**）。

**验证（离线，本机）**：离线断言 `groups checks: PASS`（解析畸形面 / 矩阵 / 联动 / 排序 / 错码全表）；变异自检三条 RED→GREEN（删反向蕴含清理 → 「取消 view → 清 edit」红；删 `grant_forbidden` 映射 → 红（落成 `saveFailed`）；排序直返 → 红（预设分区丢失））；`tsc` / `eslint`（0 error，2 条 `sonner.tsx` 存量 warning）/ `prettier --check` 全绿；键位比对 47/47。**运行期未验**——读 / 写 / 门控六条链路交用户实机。

**过程发现（值得后续点名）**：单文件编译断言脚本（`tsc lib/groups.ts --outDir …`）会忽略项目 tsconfig，**须补 `--strict`**——默认非 strict 下 `unknown` 经 `x !== null && typeof x !== "string"` 复合收窄的行为不同，会误报 `TS2322`（项目 strict 下同代码合法）。

**已知缺口**：读失败态只有一行文案、无「重试」按钮（读失败不渲染面板；会话页的 `Empty` + retry 版本未套用，刷新整页重试）；错误码 / 标签映射前端各存一份（集中 `lib/groups.ts` 单点，后端清单变化时要同步）；前端显隐非安全边界（服务端 403 兜底）。

---

## 2026-10-15 · 站点设置接入前台（元数据 / robots）+ 控制台页面标题（纯前端）

**范围**：纯前端，后端零改动，零新依赖。spec：`docs/superpowers/specs/2026-10-15-neoblog-site-metadata-design.md`；计划：`docs/superpowers/plans/2026-10-15-neoblog-site-metadata.md`。

**动机**：site 六键与「常规」页的读写都做完了，但值在项目里**零消费方**——根 `layout.tsx` 的 metadata 至今是 create-next-app 脚手架文字（"Create Next App"），`/console` 除首页外没标题（标签吃根默认），`block_search_engines` / `site.url` 无人读。本轮把六键接进真实位置，并补齐控制台页面标题。

**本轮口径（D1–D9 摘要，逐条为 2026-10-15 讨论轮拍板）**：

- **公开设置读取链**（`lib/public-settings.ts`）从每请求 `no-store` 改 **60 秒缓存**（`next: { revalidate: 60 }`）；连带注册闸（登录 / 注册页）≤60 秒生效——用户拍板接受该延迟。
- **根 `layout.tsx`** 改 `generateMetadata()`：默认标题 = **`站名 - 副标题`**（非空才拼、不产生悬空分隔符；**不兜底**）；`description` / `keywords` 配了才输出；OG 只出 `og:title` / `og:description` / `og:site_name` 三个文字字段（无 `og:image`）；`metadataBase` 仅 URL 合法时设；`robots: { index: false }` 仅开关开着时。**子页面标题保持原样、不加任何后缀**。
- 新增 **`app/robots.ts`**：开关开 = 全站 `Disallow: /`；关 = 放行 + **`/console` 永久 disallow**。
- **控制台四页补标题**（跟导航走、无「\| 网站后台」后缀）：个人信息 / 登录会话 / **站点设置**（新键 `settings.siteSettings`，界面文字仍叫「常规」）/ 用户注册。
- 纯形状独立成**零 import 的 `lib/site-metadata.ts`**（参数收 `unknown`；选择器 / 标题拼装 / URL 校验单点）。

**实现发现（重要）**：`cacheComponents: true` 下**路由级 `export const revalidate` 被构建拒绝**（`Route segment config "revalidate" is not compatible with nextConfig.cacheComponents`）——按计划预案删掉该行，缓存落在读取层 fetch 的 `next: { revalidate: 60 }`；构建表实测 **`○ /robots.txt` + `Revalidate 1m`（ISR 静态 + 60 秒再生）**，`/signin` / `/signup` 等动态页也带 `1m`（数据缓存反射）。行为口径成立：≤60 秒生效、非每请求。

**验证（离线，本机）**：纯模块离线断言 RED（模块不存在，`TS6053`）→ GREEN（`site-metadata checks: PASS`）；变异自检两条：① 标题朴素直拼 → 红（`'A - '` 悬空分隔符）；② 关键词裸 `split(",")` → 红（`' blog '` / 空串未清）；均已还原复绿。`tsc` / `eslint`（0 error，2 条 `sonner.tsx` 存量 warning）/ `prettier --check` 与本轮每次 **`next build`** 全程绿（build 是本轮缓存与路由配置的直接证据）。**运行期未验**——标签 / meta / robots 两态交用户实机。

**评审后加固（当晚）**：独立代码评审（requesting-code-review，只读子代理）结论 Critical / Important 零、三条 Minor，**三条均已落地**——① `siteBaseUrl` 补**协议白名单**（仅 http / https；离线断言先红（`javascript:alert(1)` 原实现会放行）后绿）；② 公开设置读取补 **5 秒超时**（`AbortSignal.timeout(5000)`；离线检查先红（模拟"卡死后端"8 秒不返回）后绿（5014ms fail-open 返回空设置））；③ spec §5 B5 措辞按实际更正（register 页标签与 h1 同键、其余三页各持两处键）。

**已知缺口**：① 注册闸随缓存 ≤60 秒延迟（用户接受）；② 全空配置 = 无标题无 description（不兜底，有意）；③ `settings.siteSettings`（标签）与 `settings.general`（侧栏 / h1）是两个键，将来改名要改两处；④ `/console` 的 disallow 硬编码在 `robots.ts`，换后台前缀要同步改；⑤ 无测试运行器（离线断言不入库）；⑥ 后台页面会继承站点级 description / OG（无实际影响）。

**参考实现对齐（当晚后续）**：屏蔽搜索引擎对齐 WordPress——根 metadata 的 `robots` 由 `{ index: false }` 改为 **`{ index: false, follow: false }`**（`noindex, nofollow`，WP `wp_robots_no_robots` 同款）；robots.txt 保持"开着 = 全站 Disallow"的**双保险**不动（用户点名保留；WordPress 5.3 起已不再封 robots.txt、只靠 meta 去收录，本仓的差异按用户口径登记在 spec D8）。

**提交状态**：本轮按批入库（① `lib/site-metadata.ts` → ② `lib/public-settings.ts` 缓存 → ③ 根 layout → ④ `app/robots.ts` → ⑤ 控制台四页 + 两份 messages → ⑥ 本节 + 两份 spec / plan 文档，共 6 个提交）；**评审后加固三条随后另成一小批**——均未推送。

---

## 2026-10-15 · 控制台「常规」设置页接后端（site 组六键，纯前端）

**范围**：纯前端，后端零改动，零新依赖。spec：`docs/superpowers/specs/2026-10-15-neoblog-settings-general-wiring-design.md`；计划：`docs/superpowers/plans/2026-10-15-neoblog-settings-general-wiring.md`。

**动机**：常规页（`/console/settings/general`）自 f512409 起是纯 UI——六行设置全是本地 state、初值空串，「屏蔽搜索引擎」开关 disabled；后端 `site` 组六键齐备（0.19.0 五项 + 0.20.1 `block_search_engines`）但页面读写都没接。本轮把它接到"读到真值、改完能存、存完是真值"。

**本轮口径（D1–D9 摘要）**：

- **读**：服务端直读（`lib/admin-settings.ts`：读 cookie → 带 cookie 转发 `GET /api/v1/admin/settings/site` → 形状校验，`cache: "no-store"`），值作 props 下传；不做客户端 `useEffect` 拉取。
- **写**：右下角悬浮保存（`ConsoleFloatingSaveButton`，Ctrl/Cmd+S 已内置）＋ 脏值比对，**只 PATCH 改动过的键**（空串变化、开关双向都算变化）；成功后以响应 values 重置基线 + `router.refresh()`。
- **读失败三态**：403 →「无权查看设置」；401 / 5xx / 断网 / 形状坏 →「设置读取失败」；**两种都不渲染表单**（不变量：读失败路径不存在可保存的表单，杜绝"空基线保存"把真实配置清空）。
- 纯形状独立成 `lib/site-settings.ts`（零 import，服务端读与客户端存共用同一套解析；键名映射 `blockSearchEngines → block_search_engines` 单点）；`block_search_engines` 开关解禁；Logo / 图标维持 disabled（无键、无上传接口）；不校验、不 trim，空串是合法值。

**验证（离线，本机）**：

- 纯形状模块走"编译成 JS + node 断言"（脚本在台账工作区、不入库）：**RED**（模块不存在，`TS6053`）→ **GREEN**（`site-settings checks: PASS`；断言覆盖解析失败面、多余键忽略、空串变化、布尔双向、键名映射）。
- 变异自检两条：① 让"空串变化"被跳过 → 红（`{}` vs `{ title: '' }`）；② 键名映射改 `blockSearchEngines` → 红（`{blockSearchEngines:true}` vs `{block_search_engines:true}`）；均已还原复绿。
- 静态门禁：`tsc --noEmit` ✓、`eslint` **0 error**（剩 2 条 `sonner.tsx` 存量 warning，非本轮引入）、`prettier --check` ✓；取证 grep：表单内 `useState` 0 处、`ConsoleFloatingSaveButton` 2 处（import + 使用）、蛇形键名只出现在 `lib/site-settings.ts`。
- **运行期未验**（浏览器 / 接口一律不铺，用户口径）：读路径与保存闭环交用户实机验收。

**过程发现（值得后续点名）**：`frontend/.scratch/` 只被 git 忽略，**仍被 `npm run lint` 与 `npm run format:check` 扫描**——断言脚本放进去直接让门禁红（`require()` 撞 `@typescript-eslint/no-require-imports`），本轮把脚本挪到台账工作区 `.superpowers/sdd/…`（在 `frontend/` 之外，eslint / prettier 都扫不到）。该目录原只靠本机 `.git/info/exclude` 忽略（不随仓库分发），随本轮补进**新建的仓库根 `.gitignore`**，新克隆同样忽略。要不要给 `.scratch` 补 eslint / prettier ignore，未做、待点名。

**已知缺口**：

- **读路径只能用户实机诊断**：403 →「无权查看设置」= cookie 转发通；「设置读取失败」= 转发 / 网络 / 解析坏了。
- **happy path 的 owner 账号**：实现时开发库没有 owner（冒烟账号是普通用户）→ 默认只验 403 路径；同日按用户点名将其本人账号 `366247714@qq.com` 提权为「所有者」（照 README 的手工指定口径，固定 UUID `…0001`），完整闭环改为他本人实机验；403 路径仍可用 `gate-check@example.com` 验。
- **未保存离开无守卫**（无 `beforeunload` / 路由拦截，与个人资料页一致，登记为债务）；**多标签页后写覆盖**（无版本号 / ETag，部分更新只在改过的键之间互相盖，不做乐观锁）。
- **无测试运行器**（沿用既有缺口，spec §5 B5）：前端防回归 = 静态门禁 + 一次性离线断言 + 用户实机验收；**不校验长度 / 格式**（对齐后端 site 组 D7）。

**提交状态**：本轮按批入库（① `lib/site-settings.ts` 纯形状模块 → ② 服务端读取 + 受控表单 + 保存 hook + 两份 messages 的接线提交（page 与表单 / hook 互相引用，拆开会留下编译不过的中间提交）→ ③ spec / 计划两份文档 → ④ 本节 + 根 `.gitignore`，共 4 个提交，均未推送）。

---

## 2026-10-15 · 站点设置组 site 扩充：屏蔽搜索引擎（0.20.1）

**版本**：后端 0.20.0 → **0.20.1**（patch）。零迁移、零新接口、零新依赖、前端零改动——生产代码改动只有设置清单 `site` 组加一项。用户点名「先补后端键」、按「直接开始修改」口径执行，**无 spec / 计划文档**；版本按用户裁定的**小进位**（一个布尔设置不动 minor）。

**动机**：前端控制台「常规」页（`/console/settings/general`，纯 UI 轮 f512409）有「屏蔽搜索引擎」开关行，但后端 `site` 组只有五项文字键、没有对应键，接不上；这一项是前端设置页接入后端键值的前置。口径对齐 D 盘 HaoBlog 的同名设置 `site.blockSearchEngines`（boolean、默认 false、`isPublic: true`——它的前台用它渲染 robots 元数据，见 `lib/theme/robots.ts`）。

**口径**：

- `site` 组加 `block_search_engines`：开关、默认 `false`、标 `Public: true`（前台要匿名读它决定 robots / meta，理由同 `register.enabled` 必须公开）。
- 键名用蛇形 `block_search_engines`：本仓项名规则 `^[a-z][a-z0-9_]*$` 不收大写，不能照抄 D 盘的 camelCase。
- `site` 组由此从全文字变成混合类型（五个 string + 一个 bool）；公开项集合由 6 项变 7 项（`GET /public/settings` 的 site 负载多一个键）。

**测试同步**：形状断言 site 五 → 六项（新项单独走 `assertDefinitions`：类型 / 默认值 / 公开性）；公开白名单加 `site.block_search_engines`；匿名公开读用例断言六键（含新键默认 `false`）；「管理写 → 公开读」用例扩成 `title` + `block_search_engines` 双键同 patch 并改名 `TestSettingsPatchSiteReadsBackPublicly`（原名 `…TitleReadsBack…`；顺带覆盖 site 组的 bool 写入路径）。三条变异自检（逐条把代码改坏跑 settings 包确认红后还原）：

| # | 变异 | 实测红 |
| --- | --- | --- |
| ① | 新项默认值 `false` → `true` | {形状（default = true, want false）、匿名公开读（= true, want the false default）} |
| ② | 漏标 `Public` | {形状（public = false, want true）、公开白名单（少一项）、匿名公开读（五个键 vs 六个）、管理写公开读（公开负载里没有新键）——四条全红} |
| ③ | 类型改 `FieldTypeString` | {形状（type = 1, want 3）、匿名公开读（空串 vs false）、管理写（PATCH 400 类型不符）} |

**文档同步**：README 三处（模块对照表行、设置章节「当前清单项」、公开负载节选注记）。

**已知缺口**：前端控制台「常规」页与前台 robots 渲染均未接（本轮零前端改动）；后端零消费方。

**提交状态**：本轮按批入库（① 清单 + 测试 → ② README 与 DEVLOG → ③ 版本进位，共 3 个提交，均未推送）。

---

**版本**：后端 0.19.0 → **0.20.0**（minor）。零迁移、零新接口、零新依赖、前端零改动——生产代码改动只有设置清单：`register` 组加 4 项、新建 `auth` 组 9 项。spec：`docs/superpowers/specs/2026-10-15-neoblog-auth-settings-design.md`；实施计划：`docs/superpowers/plans/2026-10-15-neoblog-auth-settings.md`。

**动机**：用户点名加一批「注册与认证」字段，口径是**只填字段、不接实现**——值能存能读，但不接邮件、不让会话真过期、不验验证码。字段与 D 盘 HaoBlog「认证设置」页逐项对应；分组与粒度按用户裁定：拆 `register` + `auth` 两组、沿用 `register/enabled` 做注册总闸、密码要求拆长度 + 复杂度、会话有效期按天默认 7。

**口径**：

- `register` 组五项：`enabled`（总闸，公开）+ `default_group`（默认 `"user"`）、`email_domain_control_enabled`、`allowed_email_domains`（英文逗号分隔）、`email_verification_enabled`。
- `auth` 组九项：`password_requirements_enabled`、`password_min_length`（默认 `8`）、`password_complexity`、`session_max_age_enabled`、`session_max_age_days`（默认 `7`，单位天）、`totp_enabled`、`password_reset_enabled`、`captcha_enabled`、`captcha_provider`。
- 新增 13 项**全部非公开**：公开端点负载零变化（仍 `site` 五项 + `register.enabled`）。
- 与 D 盘的三处口径差：`password_requirements_enabled` 默认 false（D 为 true）；`session_max_age_days` 存天（D 存小时 168）；`captcha_provider` 默认 `""`（D 为 selfhosted）。
- 不做校验（枚举 / 范围 / 长度）；不加 Turnstile 密钥两项（用户清单未含；secret 类型机制也没有）。

**测试与变异自检**：形状断言扩两组成员（名字 / 类型 / 默认值 / 非公开逐项）+ 两条 admin 用例（`register` 默认读、`auth` 默认读 + 写回）。变异自检三条：

| # | 变异 | 实测红 |
| --- | --- | --- |
| ① | `session_max_age_days` 默认改 30 | {形状（default = 30, want 7）、auth 往返（= 30, want 7）} |
| ② | `captcha_enabled` 误标 `Public` | {白名单（7 项 vs 6）、形状（public = true, want false）、匿名公开读（3 组 vs 2）、站点标题公开读（3 组 vs 2）——四条全红} |
| ③ | `default_group` 挪去 `auth` | {形状（register items = 4）、register 默认读（4 项）} |

**已知缺口**：13 项全无消费方（实现接线是后续轮）；`default_group` / `captcha_provider` 无取值约束；`password_min_length` 无范围校验；控制台界面未做。

**提交状态**：本轮按批入库（① 代码+测试 → ② spec/计划 → ③ README 与 DEVLOG → ④ 版本进位，共 4 个提交，均未推送）。

---

## 2026-10-15 · 站点设置组 site（0.19.0）

**版本**：后端 0.18.0 → **0.19.0**（minor）。零迁移、零新接口、零新依赖、前端零改动——唯一生产代码改动是设置清单加一个组。spec：`docs/superpowers/specs/2026-10-15-neoblog-site-settings-design.md`；实施计划：`docs/superpowers/plans/2026-10-15-neoblog-site-settings.md`。

**动机**：用户点名加「站点标题 / 副标题 / 站点描述 / 站点关键词 / 站点 URL」。用途是前台元数据（`<title>`、meta、canonical/OG）的公共来源，将来邮件链接基址大概率也读 `site.url`。设置机制从 0.8.0 落地时定的口径就是「新增设置只改代码、永不操作数据库」，本轮是首个纯清单扩展。

**口径**：

- 组 `site`，五项 `title` / `subtitle` / `description` / `keywords` / `url`：全文字、默认空串、全部 `Public`。键名与 D 盘 HaoBlog 的 `site.*` 逐字一致。
- `keywords` 存英文逗号分隔的单个字符串（框架无列表类型；HaoBlog 同形）。
- 不做长度 / URL 格式校验（校验是机制留的插槽，首个真实需求再动）。
- 组序 `site` 在 `register` 前；公开聚合按清单序输出。
- 前端零改动：本轮只保证公开端点可读；渲染接线是后续前端轮。
- README「怎么加一个设置」的配方示例组名由 `site` 换成 `demo`（与真实组消歧），`recipeGroups()` / `TestRegistryRecipeShape` 原子联动；README 公开负载示例与 API 规范 #23 示例补「（节选）」。

**测试与变异自检**：测试先行——四处断言先红（公开白名单、形状断言、匿名公开读、新端到端用例，均因 site 组不存在）→ 落清单转绿 → 变异自检三条：

| # | 变异 | 实测红 |
| --- | --- | --- |
| ① | `site.url` 取消 `Public` | {公开白名单（5 vs 6）、形状断言、公开读（site 只剩 4 键）} |
| ② | `site.title` 默认值改非空串 | {形状断言、公开读（title=NeoBlog）} |
| ③ | 两组换序 | {公开读、端到端（均因组序）；白名单保持绿——证排序生效} |

**已知缺口**：五项后端暂无消费方（前端未接线，界面不变）；无长度校验；空串默认 = 未配置、前台自行兜底；`site.url` 属「每个部署一份」的值。

**提交状态**：本轮按批入库（① 代码+测试 → ② spec/计划 → ③ README、API 规范与 DEVLOG → ④ 版本进位，共 4 个提交，均未推送）。

---

## 2026-10-15 · 用户简介 user_bio（0.18.0）

**版本**：后端 0.17.0 → **0.18.0**（minor）。一个迁移（`user_bio`，纯 DDL）、零新依赖；前端两处小改（回填 `bio`，见下）。

**动机**：前端个人资料页早有「用户简介」输入框，保存时把 `bio` 一并发出，但后端 `users` 表没有这一列、`PATCH /users/me` 的请求结构体也只有 `nickname` / `email`——Go 反序列化**静默丢弃未知字段**，所以请求照样 200、页面照样弹「已保存」，实际什么都没存、刷新即空。用户 2026-10-15 核对后选择「后端补字段」（另一选项「前端撤掉输入框」未取）。

**口径（四条，实现即按此落）**：

- 存储：`users.bio` 可空 varchar（`field.String("bio").Optional().Nillable()`），「未设置」= `NULL`——对齐 API 规范 §5「标量可空字段输出 `null`」与 D 盘旧项目的 `String?` 形态。
- 请求：`PATCH /users/me` 加可选字段 `bio`（`*string`）；缺省 / `null` = 不修改（沿用既有 PATCH 指针约定）；提交 `""` 或纯空白 = 清空（库中落 `NULL`）。「至少给一个字段」守卫同步扩到三个字段。
- 归一与校验：trim（保留内部换行）、≤1000 字符按 rune（与 HaoBlog 的 `PROFILE_BIO_MAX_LENGTH` 同值）、越界 400 `invalid_request` + `param: "bio"`。
- 响应：`UserResponse`（注册 / 登录 / verify / PATCH）与 `MeResponse`（`GET /users/me`）统一带 `bio`（未设置为 `null`）——用户对象保持单一形状。

**实现要点**：

- 迁移 `20261015003344_user_bio.sql`：`ALTER TABLE "public"."users" ADD COLUMN "bio" character varying NULL;`，两库已应用；`scripts/db-migrate-diff.sh` 的字面量随轮换为 `user_bio`。
- 仓储三分支：`nil` 不动 / 空串 `ClearBio()` / 非空 `SetBio()`（`repository.go` 的 `updateProfile`）。
- 前端接线（用户指出「前端没有接入上」后补，只动两个文件）：`lib/auth-session.ts` 的 `SessionUser` 加 `bio: string | null` 并解析（`undefined` 容忍 → 老后端返回里没这个键也不会让整站会话解析炸掉，落 `null`）；`hooks/use-profile-save.ts` 本地 state 改从 `profile?.bio` 取初值、保存后用响应里的 `bio` 回填（沿用「保存后回填后端归一值」的既有口径，顺带吃掉后端 trim）。不碰并行会话正在改的 `messages/*.json`，**因此本轮零新增文案键**。

**测试与变异自检**：新增 `TestUpdateProfileBio`（初始 `null`、写读一致且 trim 与换行保留、只改简介不动身份字段、1000 / 1001 rune 边界、空白清空后 API 与库内**双核对 `NULL`**、只改昵称保留简介、空串显式清空）；`TestUpdateProfileEndToEnd` 的两个边界清单补 `{"bio": null}` 与 `{"bio": 42}`。变异自检六处（改坏 → 确认目标红 → 改回 → 全包绿）：

| # | 变异 | 实测红 |
| --- | --- | --- |
| ① | `normalizeBio` 去掉 trim | {trim + 换行断言} |
| ② | `validBio` 边界 `<=` 改 `<` | {1000 rune 边界} |
| ③ | 仓储删清空分支（一律 `SetBio`） | {空白清空 → 落空串而非 `NULL`} |
| ④ | `toUser` 不映射 `bio` | {写后回读} |
| ⑤ | 校验错误丢 `param` | {1001 rune 的 `param` 断言} |
| ⑥ | 「至少给一个字段」守卫忽略 `bio` | {纯 `bio` 的 PATCH} |

**活体证据（临时实例 5733，`gate-check@example.com`）**：登录响应即带 `"bio":null` → PATCH 带首尾空格与换行 → 200 且 trim 生效 → 只改昵称 bio 保留 → 1001 字符 400 `param:"bio"` → `{"bio":""}` 清空回 `null` → 直连 `neoblog` 库确认为 `NULL`（账号复位原状）。临时实例已关闭。

**已知缺口**：

- **简介超长暂无专属错误文案**：`param: "bio"` 现在落进 `use-profile-save.ts` 的兜底分支，页面提示通用「保存失败，请稍后重试」。加专属键要动 `messages/*.json`，而那两个文件正被并行会话占用——等用户点名再补。
- **前端侧未做运行期验收**（按既有口径，运行期 / 视觉验收由用户自己在浏览器里做）：`tsc --noEmit` 退出 0、`eslint` 对改动的两个文件 0 问题、`prettier --check` 通过；页面实际表现（填写 → 保存 → 刷新仍在）待用户验。
- 简介除长度外无其他校验（无 Markdown / HTML 清洗）；纯文本存储，展示侧自行转义。
- 全仓测试无 `-race`（本机无 cgo）；本轮无并发路径新增，与既有口径一致。

**提交状态**：本轮改动已按批入库（`schema`+迁移 → 接口+测试 → 前端接线 → 本文档 → 版本进位，共 5 个提交，均未推送）。工作区里并行会话的侧栏设置页前端文件（`console-sidebar-nav.tsx`、两份 `messages`、`app/console/settings/`、`console-sidebar-section.tsx`）仍未提交，**不属于本轮**。

---

## 2026-10-15 · 侧栏面板状态化（无动画）

**范围**：纯前端，后端零改动，零新依赖。spec：`docs/superpowers/specs/2026-10-15-neoblog-sidebar-panel-state-design.md`；计划：`docs/superpowers/plans/2026-10-15-neoblog-sidebar-panel-state.md`。

**动机**：主侧栏导航区在 `/console` 下整片空白——`console-sidebar-nav.tsx` 在路径不是 `/console/uc` 打头时直接 `return null`，而 `SidebarContent` 是 `flex-1`，空了就整片留白；且「个人设置」那栏的形状每次都要重写、面板显示完全由 URL 决定。

**本轮口径**：

- **state 为真源、URL 只同步**：`displayedPanel` / `syncedRoutePanel` 双 state，渲染期比对接管（不用 `useEffect`），两个初值都取路由面板，避免首屏先渲染主栏再纠正。
- **动画整条不做**（推翻上一轮定的 ±48px 对滑）：无叠层绝对定位、无新增 transition 常量、无 `outgoingPanel` / `setTimeout` / `onTransitionEnd`，面板切换即显隐。`sidebarLabelFade`（折叠时文字淡出）属既有折叠机制，保留。
- **返回语义两套并存**：settings 面板返回是按钮，只改 state、URL 不动；uc 面板返回仍是 Link（未改）。这是"最小化 + 尊重上轮口径"换来的不一致，登记为债务。
- 抽出复用件 `console-sidebar-section.tsx`：`back` 用判别联合自带 `onClick`（不留可能永不触发的必填回调）；`items[].className` 承接 `31fac91` 的 `mb-1`，防止注册表形态把它丢掉。
- 主侧栏只加「设置」一个入口；子栏只有「常规」一项 + 一页只有标题的占位页，**不接任何真实系统设置内容**。

**已知缺口 / 未实测**：

- **八条交互实测一条都没做成**：dev server 在 3000，但 `/console`、`/console/settings/general`、`/console/uc/profile` 全部回 `307 → /signin`，无登录态就拿不到侧栏 SSR HTML；开发库注册是关闭态、也没有可用凭据。已证明的只有三件：`tsc --noEmit` 退出码 0、`eslint .` **0 error**（剩 2 条 warning 在 `components/ui/sonner.tsx`，非本轮引入）、`npm run format:check` 全通过（**作用域是 `frontend/`**——仓库根没有 prettier 配置，`docs/*.md` 不在格式检查范围内），外加三条路由返回 307 而非 500（说明模块能编译）。
- **B1 错位窗口**：点返回后 URL 仍停在 `/console/settings/general`，此时**刷新会回到 settings 面板**（state 不持久）。这是 D1+D2 合取的既定结果，不是 bug，但尚未眼验。
- **B5 移动端**：Sheet 抽屉里点「设置」后是否自动关闭未设计、未测。
- **B6 无测试兜底**：前端无测试运行器（`package.json` 只有 `dev/build/start/lint/format`，全仓 0 个 `*.test.ts*`），本轮不引入 ⇒ 面板切换不变量拆在判定层与编排层，**没有自动测试承载**。
- **B8 键盘焦点缺口（审查发现，本轮未修）**：settings 面板的「返回」是按钮，点击后整棵子面板卸载，聚焦的按钮消失 → `activeElement` 退化到 `<body>`，再按 Tab 从文档起点重来、侧栏被整体跳过。鼠标用户无感；旧版 `uc` 那栏的返回是真链接，跳页后浏览器自行重置焦点，所以这条缺口是 settings 路径新增的。修法是在返回后把焦点交给主栏「设置」项，需要引 ref/焦点转移，与本轮「最小化」口径冲突，**按用户口径登记不修**。

**审查后更正两处**：

- 代码 `console-sidebar-nav.tsx:117` 原写 `isActive={routePanel === "settings"}`，与 spec §4.5 / plan Task 3 的全式 `displayedPanel === "main" && routePanel === "settings"` 不一致（行为等价：该判断在 `:111` 的 `displayedPanel === "main"` 提前 return 之内，前缀恒真），已补全以对齐文档并防后续改成叠层时静默变语义。
- plan Review Focus 5 原写「`en.json` 漏键会抛 `MissingMessageError`」**是错的**：实测 `use-intl` 的 `defaultOnError` 是 `console.error(error)`（`initializeConfig-CUsOI8u2.js:17-19`），缺键走 `getFallbackFromErrorAndNotify` 返回 `namespace.key`（同文件 `:95-103`），不抛错、不会让任何校验命令变红；本项目全文无 `onError` 覆盖。plan 已同步更正。

**提交状态**：五个代码文件（两份 messages、`app/console/settings/general/page.tsx`、`console-sidebar-section.tsx`、`console-sidebar-nav.tsx`）与本节**均未提交**（用户口径：不要擅自提交）；本轮已入库的只有 `46052b8`（spec + plan 两份文档，未推送）。

---

## 2026-10-14 · 两步验证 setup 复用窗口（0.17.0）

**版本**：后端 0.16.0 → **0.17.0**（minor）。一个迁移（`totp_setup_at`，纯 DDL）、零新依赖、前端零改动。设计 spec：`docs/superpowers/specs/2026-10-14-neoblog-2fa-setup-reuse-window-design.md`；实施计划：`docs/superpowers/plans/2026-10-14-neoblog-2fa-setup-reuse-window.md`（用户口语称「防抖」，实现语义是**复用窗口**）。

### 动机与窗口口径

症状：2FA 设置面板每次展开都重新生成密钥（前端收起即卸载 `useTwoFactorSetup` → 挂载即 `POST /users/me/2fa/setup`；后端无条件覆盖）。v1 方案定 30 秒，被独立审查否掉——30 秒盖不住「展开→掏手机扫码→回浏览器输码」的真实节奏，症状照样复发；且面板没有主动换钥入口，「等窗口过去」不可见。**窗口定为 5 分钟（固定窗，从生成时刻起算）**：`now - totp_setup_at < 5min` 复用（零写入、同一账号名下响应逐字节相同），`>= 5min` 重新生成（旧码作废、窗号归零，既有语义）。`disable` 清四列（secret / enabled_at / last_counter / **setup_at**）；`enable` 不动 `setup_at`。

### 实现要点

- `users` 加 `totp_setup_at`（可空 timestamptz，迁移 `20261014134240_totp_setup_at.sql`，两库已应用）。
- 复用响应 = 解密存量密文 → base32 解码 → `totp.Generate{Secret: 原始字节}` 重建；**空明文守卫**必须有——`pquerna/otp` 在 `len(Secret)==0` 时会静默随机生成新密钥（实证源码）。
- 生成路径走**原子条件更新认领** `claimTOTPSetup`（条件：未启用 且 `setup_at` 为 NULL 或早于 `now - 5min`），互斥依据是 Postgres Read Committed 下单条 UPDATE 重算谓词——**同一时刻 N 个并发 setup 至多产生一把密钥**，其余复用。认领 0 行 → 复读以库内现状为准（无密钥 → 再认领一次，共两次；两次都不行 500，理论不可达、不写库）。
- **删除了 `updateTOTPSetup`**：认领路径接手后它成了无人调用的死方法（Go / `go vet` 都不报），全仓 grep 确认无痕。曾考虑的「认领失败盲写候选」兜底在审查中被否：既无用例覆盖、又破坏「至多一把」。

### 测试与变异自检

新增 **10 条**集成用例（复用零写入 / 5m−1s 与 5m 边界 / 8 并发同钥 / disable 清 / enable 保留 / 遗留行 / 复用解密失败 500 / 空明文 500 / 仓库级认领互斥——最后这条是变异②自检补救时新增的）+ 2 条既有用例调整（`TestTwoFactorSetupOverwritesSecret` 推进到窗口外、`TestTwoFactorDisableLifecycle` 补断言）；时间一律经 `totpNow` 假钟固定，零真实等待。

变异自检八处（改坏→确认目标红→改回→全包绿），实测集合：

| # | 变异 | 实测红 |
| --- | --- | --- |
| ① | 复用条件 `<`→`<=` | {3 边界} |
| ② | 认领删时间条件 | **首测并发用例未红**（服务层全先读后写，时序运气）→ 补仓库级用例 `TestClaimTOTPSetupRejectedWhenFresh`（新鲜行二次认领必拒、过期必放）后红 {该用例} |
| ③ | disable 漏清 setup_at | {5, 11} |
| ④ | 删读路径复用分支 | **不可观测，移出列检**：慢路径（认领 0 行→复读→复用）行为与快路径等价（探针实证三零：响应同 / 密文同 / 时刻同，全包不红）；真正防线是「认领与复用互斥」（②⑥ 钉住） |
| ⑤ | enable 误加 Clear | {6} |
| ⑥ | `isFreshTOTPSetup` 把 NULL 时刻当新鲜（时间窗保留） | {7}（遗留行走复用返旧钥；独立复核实跑确认） |
| ⑥b | 新鲜判定整条放宽成「有密钥就算新鲜」（时间窗一并丢） | {3, 7, 10}——首轮 DEVLOG 误把 ⑥b 的实测数字记在 ⑥ 名下（两个改坏串行）；复核实跑把两者拆开，⑥={7}、⑥b={3,7,10}，spec §7 同步 |
| ⑦ | 认领谓词删 `IsNil` | 25 条（几乎全包：首次 setup 都落 NULL 行 → 500）；未逐条实跑，逻辑复核确认 |
| ⑧ | 删空明文守卫 | {9} |

②④⑥三条是自检真发现：并发用例单靠 goroutine 抓不住确定性互斥（须仓库级用例）；快路径只是慢路径的免写优化、行为分叉在 `isFreshTOTPSetup` 的前提上（不在快路径有无）；首轮自检曾把 ⑥b（整条放宽）的实测数字误记在 ⑥（仅 NULL 放宽）名下，复审拆开更正。变异⑦未逐条实跑（逻辑复核）。`-race` 本机不可用（无 cgo），并发正确性靠「认领与复用互斥的仓库级断言 + 重复 8 次稳定绿」承载，未做竞态证明。

### 活体证据（5732 实例，`gate-check@example.com`）

登录 → setup A → 立刻再 setup B（A == B，`setup_at` 落值）→ 探针把 `totp_setup_at` 拨回 6 分钟前 → setup C（C ≠ B）→ 用 C 算当前窗码 enable → 200 + 10 恢复码。**注意**：第一轮活体在旧二进制上跑出「同刻两把」假结果——dev server 是 `go run`，改代码后必须重启才吃到新实现（已重启复测）。冒烟账号事后已复位（四列清空）。

### 已知缺口

同坐超 5 分钟仍换钥（残余，接受）；面板每次展开仍有一次请求 + 骨架屏闪烁（前端轮）；主动换钥无 UI 入口（disable 接口可用，按钮待前端轮）；陈旧待验证密钥不主动清理。

---

## 2026-10-14 · 路由改名：`/auth/*` 前缀取消（会话式方案，0.16.0）

**版本**：后端 0.15.0 → **0.16.0**（minor：破坏性路径变更）。**零迁移、零新依赖**。**状态：未提交**（工作区留待检查）。起因：用户发现「读自己 `GET /auth/me`」与「改自己 `PATCH /users/me`」挂在两个前缀下，要求把认证域收进 `users`；先做了一轮**只读调研**（大厂公开 API / 框架官方生成器 / 「登录=建会话」流派，逐条核对官方文档），用户在三个候选里选定**会话式**。

### 新路径（旧 → 新逐条）

| 旧 | 新 | 说明 |
| --- | --- | --- |
| `POST /auth/register` | `POST /users` | 注册 = 建用户 |
| `POST /auth/login` | `POST /sessions` | 登录 = 建会话 |
| `POST /auth/logout` | `DELETE /sessions/current` | 登出 = 删当前会话（方法 POST → DELETE） |
| `POST /auth/2fa/verify` | `POST /sessions/current/verify` | 二步验证 = 对当前（待二段）会话的验证 |
| `GET /auth/me` | `GET /users/me` | 与 `PATCH /users/me` 合成同一条资源（`/auth/me` 删除） |
| `POST /auth/password` | `POST /users/me/password` | 改密 |
| `POST /auth/2fa/{setup,enable,disable,recovery-codes}` | `POST /users/me/2fa/*` | 2FA 管理，只搬家、名字不变 |
| `PATCH /users/me`、`GET/DELETE /users/me/sessions` | 不动 | |

`/auth` 前缀从此不存在；守卫链、错误码、响应形状、会话数据键（`auth.user_id` 等，内部键不是 URL）全部不变。

### 方案依据（调研出处，均有官方文档原文）

- 「当前用户」= `/users/me`：Twitter/X（`/2/users/me`）、Okta（`/api/v1/users/me`）、Discord（`/users/@me`）、Strapi（`/api/users/me`）一致；GitHub 是唯一的单数 `/user` 派。
- 登录/登出从不放 `/users` 下：大厂一律放认证命名空间或会话资源；登出要么是动作端点，要么是**删会话**（Okta `DELETE /api/v1/sessions/{id}`）。
- 「登录 = 建会话」有真实采用者：Rails 8 官方生成器（`POST/DELETE /session`）、Ghost（`POST /admin/session/`，2FA 是对本次会话的 `PUT /admin/session/verify/`）、Zitadel（`POST /v2/sessions`、`PATCH /v2/sessions/{id}` 提交 TOTP）。
- 2FA「登录挑战」与「设置管理」分居两处是主流常态（Okta `/authn/factors/…` vs `/users/{id}/factors`；Fortify `/two-factor-challenge` vs `/user/two-factor-*`；ASP.NET `/Account/LoginWith2fa` vs `/Manage/*`）。
- 被否的两个候选：`/users/login` 全动词式（大厂零先例；Zalando REST 规范 Rule 141「URL 必须无动词」是 MUST 级；唯一先例是 Phoenix 生成器）；保留 `/auth` 只收匿名（主流但违背用户「不要单独 auth」的初衷）。

### 改动面

- 后端：`internal/modules/auth/controller.go` 两个注册函数（10 行路径 + logout 方法改 DELETE）；`GET /users/me` 复用原 `me` handler。
- 测试：5 个文件路径同步（`app_test.go`、`auth_test.go`、`register_gate_test.go`、`sessions_test.go`、`totp_flow_test.go`）；405 用例的「GET-only」样本从 `/auth/me` 换到 `/users/me/sessions`（`/users/me` 现在 GET+PATCH 双方法）；2FA 测试助手 `post2FA` 改走 `/users/me/2fa/`、`logoutOK` 改走 `doDELETE /sessions/current`。
- 前端：4 个真实调用点换名——`use-signin`（→ `/api/v1/sessions`）、`use-signup`（→ `/api/v1/users`）、`use-signout`（→ `DELETE /api/v1/sessions/current`）、`lib/auth-session.ts`（→ `/api/v1/users/me`，含错误文案）。
- 文档：API 规范 v1.5 修订条目 + §3 规则 3 示例 / D11 / 端点表 + §4 无负载体示例；`backend/README.md` 接口表与改名对照；版本 0.15.0 → 0.16.0。

### 验证

- 全仓 `scripts/test.sh`（串行、真库）：**全绿**（`internal/app` 4.7s、`internal/modules/auth` 13.6s，共 11 包）。
- 前端 `npx tsc --noEmit`：零错误。
- 全仓源码扫描：`auth/login|logout|register|me|password|2fa` 在 `backend/`、`frontend/` 的代码里**零残留**；`docs/` 下仅存档保留旧路径原文（更早轮次的 spec/plan 与日志旧条目），本轮同步的活文档（README / 规范 / 本日志 / 同日会话管理 plan）已全部换新。

### 备注

- 上一轮提交过程里出现过「并行会话把同款改名写进 `controller.go` 又撤出」的插曲（见本文件下一条「提交过程中的插曲」）；本轮是从干净树重做的完整版（含测试、前端接线与文档），不是半成品。
- 运行期验收（浏览器登录链路）留用户自查；需要的话可另跑 curl 冒烟。

### 评审回补（同日）

- `app_test.go` 405 表新增一行 `PUT /users/me → Allow: GET, PATCH`——`/users/me` 变 GET+PATCH 双方法后的新形状此前没有用例钉住；已做变异验证（临时把 PATCH 路由改名，该行立刻红：`Allow = "GET", want "GET,PATCH"`；还原后全绿）。
- 同日 `docs/superpowers/plans/2026-10-14-neoblog-session-management.md` 的 3 处 `GET /auth/me`（含冒烟 curl 全链路）同步为 `GET /users/me`，文件顶加「路径口径（0.16.0 起）」说明——它是可复跑配方，不是历史存档。
- 口径修正：规范 v1.5 条目原写「实现随 0.16.0 入库」（本轮未提交，措辞不实）改为「实现见 0.16.0」并注明 §11 及以后为历史存档；§3 端点表列名「声明位置（2026-10-11 时点）」改为「随轮次更新」（该列本就逐轮更新，本轮写入了 0.16.0 时点的新行号）；本条目「零残留」句的存档清单补全。

---

## 2026-10-14 · 会话管理：审查后修补（测试有效性 3 条 + 文档口径 5 处 + 小项）

**版本**：0.15.0（未进位）。针对两份独立审查（终审子代理 + 外部复核）的结论做修补。**状态：未提交**（等用户决定提交切分）。用例总数 16 → 18。

### 测试有效性（三条都是「改坏了也会绿」）

- **`TestSessionsRevokeCurrentEndsSession` 原本没钉住复活路径**：它把 `last_seen_at` 改到 6 分钟前之后**又调了一次列表接口**，时间被刷回新鲜 → 删行时本请求不写库，`Destroy` 与裸 `Store.Delete` 无差别。改为「先列表取 id → 再改写 → 立即 DELETE」。**变异验证**：把 `Destroy` 换成 `Store.Delete(target)`，该用例立刻红（`status = 200, want 401`）；还原后绿。
- **节流「新鲜不写库」两处断言对「每次都写」全盲**：基准值原是「now」，与任何新写入的秒级字符串相同。基准改为 **1 分钟前**（仍新鲜，但与新写入必然不同）。**变异验证**：把节流条件改成恒真后，`TestTrackSessionActivitySkipsWhenFresh` 与 `TestSessionActivityTouchThrottled` 同时红。
- **`enterPendingSession` 清四个元数据键零覆盖**：新增 `TestTwoFactorLoginClearsSessionMetadata`（登录 → 开 2FA → 同一会话再登录吃 `two_factor_required` → 断言 `user_id` 与四个元数据键全消失、只剩 pending 键）。**变异验证**：删掉那四行 `Remove`，该用例红。
- 新增 `TestLoginTruncatesHeadersAtRuneBoundary`（多字节 UA 截断后必须是合法 UTF-8）。

### 代码小项

- `truncateHeader` 由按字节硬切改为回退到最后一个 rune 起始边界，不再截出非法 UTF-8。
- `Login` 去掉重复的 `RenewToken`：非 2FA 分支只由 `establishSession` 换发一次；2FA 分支单独换发（防会话固定语义不变）。

### 文档口径

- 设计文档：§2「删当前会话」与 §5「活跃追踪」纠正为「**必须**对当前 token 特判走 `Destroy`，裸删会被本请求的节流写提交回写复活」；§6 的 `HashTokenInStore` 代价改为**三处比较**（跳过当前会话会踢掉自己 / `current` 徽标不亮 / 判不出当前会话掉回复活坑）；§7 补「进入待验态清元数据」用例、给删当前会话用例写明前置条件、并写明测试存储是 **memstore**（Postgres 由活体冒烟覆盖）；§7 第 3 条的「照 `app_test.go` 直插先例」纠正为「新写助手（原生 SQL vs scs Codec/Store 通道不同）」。
- 计划：冒烟起进程 `PORT` → **`ADDR=:5733`**（地址只有 `ADDR` 这个口子）；节流用例基准改 1 分钟；删当前会话用例的关键前置写进用例说明；冒烟收尾补「断言残留为零」。
- API 规范表 §3 表头补消歧义一句（本表每轮追加；§13.1「表 A」是 2026-10-11 的冻结对照表、固定 14 行）。
- `README` 的 `HashTokenInStore` 缺口同步改写为三处比较。

### 附带：`internal/app` 一条既有偶发红（非本轮引入）

`TestSessionCookieMaxAgeMatchesLifetime` 偶发 `MaxAge = 3601, want 3600`。机制（读 scs 源码）：`RenewToken` 把 deadline 重设为「此刻 + lifetime」（`data.go:301`），而写 cookie 时 **Max-Age 向上取整**（`int(time.Until(expiry).Seconds() + 1)`）——两者落在同一时钟刻度就得 3601。该测试与登录公开路由都不经过本轮代码；在**当前 HEAD 与改动前提交（9144dad）各跑 25 次，两侧都是 0/25 复现**（稀有竞态，两边同概率）。断言放宽为 `want || want+1`。

### 提交过程中的插曲：并行会话在同一文件里改了路由

第一批提交时 `controller.go` 被另一会话改成整套路由改名（`/auth/login` → `POST /sessions`、`/auth/me` → `GET /users/me`、`/auth/2fa/*` → `/users/me/2fa/*`、`/auth/logout` → `DELETE /sessions/current`；该文件 mtime 05:42:29，提交发生在 05:42:42）——**F1/F2 两个提交把对方的在制品一起吞了进去**，而那时测试还全在老路径上（那棵树跑测试是红的）。处置：两个提交已撤回，按「只暂存我的 hunk」（先把我的改动恢复成独立版本暂存、再把对方的版本放回工作区）重提，**对方的改名原样留在工作区、不进提交**。教训：**按文件清单逐个 `git add` 挡不住「同一文件里的外部 hunk」**——提交前要对每个文件核对 `git diff` 的内容而不只是文件名，与并行会话共用工作区时提交窗口越短越好。

---

## 2026-10-14 · 会话管理：登录设备列表 + 单条吊销（后端一轮）

**版本**：后端 0.14.0 → **0.15.0**（minor）。**零迁移**：`sessions` 表与 `atlas.sum` 一字节未动，元数据走会话数据键。**状态：全部未提交**（用户要求本轮只做改动、不提交，工作区留待检查）。设计：`docs/superpowers/specs/2026-10-14-neoblog-session-management-design.md`；计划：`docs/superpowers/plans/2026-10-14-neoblog-session-management.md`。

### 做了什么

- **元数据四键**：`auth.user_agent` / `auth.ip` / `auth.created_at` / `auth.last_seen_at`（RFC3339 UTC 字符串），在注册成功、单段登录成功、2FA verify 成功三处经 `auth.establishSession` 统一写入；进入「待二段」改走 `enterPendingSession`（清身份 + 清元数据）。
- **节流中间件**：`guard.TrackSessionActivity`（5 分钟），`app.go` 与 auth 测试装配各挂一次，链序 `RequireAuth → TrackSessionActivity → LoadIdentity`。
- **两个接口**（都只挂 `RequireAuth + LoadIdentity`、不设权限点）：`GET /api/v1/users/me/sessions`（`{"items":[…]}`，`id = base64url(sha256(token))` 指纹，`current` 徽标，RFC3339 时间）与 `DELETE /api/v1/users/me/sessions/:id`（200 `{"id","deleted":true}`；未知 / 他人 / 二次删一律 404 `not_found`）。
- **收敛重复实现**：改密 / 2FA 开关在用的 `revokeOtherSessions` 抽到共用的 `eachSession` 迭代器上，行为不变（三处调用点同步去掉已不需要的 ctx 参数）。
- **测试**：新增 16 条用例（guard 4 + auth 12），全仓 11 包全绿（`scripts/test.sh`，串行）。
- **活体冒烟 12/12**：:5733 另起新二进制 + curl 全链路（两条登录 → 列表 2 条且 IP 来自 `X-Forwarded-For`、`id` ≠ cookie 值 → 删一条 → 被删方 401 / 自己 200 → 列表回 1 → 乱造 id 404 → 匿名双接口 401 → 登出 200），跑完会话行已清、进程已停。

### 过程中的关键事实（写进计划前先证过的）

- **`Put` 不移动 deadline**：scs 的 `Commit` 用 `sd.deadline`，`Put` 只置 `Modified`——节流刷新 `last_seen_at` **不会**把 7 天绝对有效期变成滑动续期。
- **`postgresstore` 自带清理**：`New()` 就带 5 分钟一轮的过期删除 goroutine，`All()` 的 SQL 已带 `WHERE current_timestamp < expiry`，列表天然只出活跃行——**不需要另做清理任务**（写计划时的初始判断是错的，读源码纠正）。
- **gob 只吃内建类型**：会话数据是 gob 编码的 `map[string]interface{}`，`time.Time` 进 interface 需要 `gob.Register` 全局副作用，因此四个元数据键一律存 RFC3339 字符串。
- **删当前会话有「复活」坑**：中间件若在本请求内 `Put` 过（节流到点），handler 之后再删行，scs 会因 `Modified` 在响应提交处把行**写回**。`RevokeSession` 对「当前 token」改走 `sessions.Destroy`（置 `Destroyed`、清 cookie），绕开提交分支；`TestSessionsRevokeCurrentEndsSession` 先把 `last_seen_at` 改到 6 分钟前、**紧邻 DELETE 之前**来钉这条路径（新鲜会话不会触发，光跑默认路径测不出来）。**注意：该用例的首版写法失效**——改写之后又发了一次列表请求，时间被刷回新鲜，删行时本请求不再写库；已在本日顶部「审查后修补」条目修正并加变异验证。
- **顺带闭合 2FA 轮登记的缺口**：`establishSession` 清 `auth.pending_2fa_user_id` / `auth.pending_2fa_attempts`，DEVLOG 里「已登录会话可能残留另一账号待验身份」这条不再存在，用 `TestEstablishSessionClearsPendingKeys` 钉住（此前无任何用例覆盖这两个键）。

### 已知取舍

- **列表是全表扫**：`eachSession` = `Store.(scs.IterableStore).All()` + 逐条 `Codec.Decode` 后按 `auth.user_id` 过滤（与既有吊销同一手法）；代价随全站活跃会话数线性增长，个人博客规模可接受。要 SQL 化见设计文档 §9.2（`sessions` 扩列 + 自写 Store）。
- **无历史 / 无分页 / 无批量**：吊销即删行，没有「已吊销」状态可筛；「退出其它所有设备」未开接口（改密 / 2FA 流程内部在用）；无会话上限（限流轮一起看）。
- **IP 可伪造**：`X-Forwarded-For` 客户端可自填，本轮按用户裁定「先信任」，**仅供展示**；限流轮要用真实 IP 必须先配 `SetTrustedProxies`。UA / IP 截断 512 / 64 字节，防超长头撑大会话行。
- **原始 token 落库**：`sessions.token` 存明文 token（scs 默认），DB 可读即会话可伪造；`HashTokenInStore` 加固待点名（代价：存量会话全失效 + 相关比较改指纹）。
- 运行期验收（浏览器里点）归用户；本轮证据是集成测试 + 另起端口的 curl 冒烟，浏览器里还没点过。

---

## 2026-10-13 · 补测试缺口：改资料接口的原子性、param、并发兜底与边界值 + 测试入口脚本

**版本**：后端 **0.14.0**（未进位）。**零源码语义变更、零迁移**：本轮只动测试与测试入口，`internal/` 下的非测试代码一行未改。起因是对上一轮（改资料接口）的独立审查列出 5 条测试缺口，用户点名全做。

### 做了什么

- **组合字段原子性**（新用例 `TestUpdateProfileRejectedWriteLeavesBothFieldsUnchanged`）：同时提交「可用的昵称 + 撞车的邮箱」→ 409 `email_taken`，随后回查确认**昵称也没被写进去**；反方向（撞车昵称 + 可用邮箱）同样回查。此前**失败路径**上从未把两个字段一起提交过（成功路径有过两次），"要么全改、要么全不改"这条只存在于代码读出来的形状里，谁改成两条 UPDATE 分两次写测试也照样绿。
- **`param` 断言**：`{"nickname":"a"}` → 400 + `param:"nickname"`；`{"email":"not-an-email"}` → `param:"email"`。前端 `use-profile-save` 全靠这个字段决定弹哪条文案，之前一个断言都没有。同时钉反向：请求体形状/类型错（`{"nickname":42}`）时 `param` 为空。
- **并发用例**（新用例 `TestUpdateProfileConcurrentSameNickname`）：两个已登录用户同时把昵称改成同一个值，正好一个 200、一个 409，且 409 的 code 落在 `{nickname_taken, already_taken}` 内。照 `TestRegisterConcurrentSameEmail` 的形状写，但**没有**复用 `exchangeJSON`——它内部 `t.Fatalf`，从测试 goroutine 里调用是错的（既有那条用例就有这个毛病）；另立了不在 goroutine 里断言的 `patchJSON`。
- **约束兜底确定性用例**（新用例 `TestUpdateProfileConstraintBackstopReturnsAlreadyTaken`）：并发那条打不到 `already_taken`（原因见下），改用测试夹具直取——在测试库上装一个 `BEFORE UPDATE` 触发器，昵称以 `collide` 开头时先插一行同名数据，使撞名预检仍然查不到、而紧接着真正的 UPDATE 撞上唯一索引，于是**确定性**拿到 409 `already_taken`。触发器与函数在 `t.Cleanup` 里连同行一起拆掉。
- **边界值**（并入 `TestUpdateProfileEndToEnd`）：拒绝侧——数组体、字符串体、数字体、`{"nickname":42}`、纯空格昵称、33 rune 昵称、261 字节邮箱 → 全部 400；接受侧——2 rune、32 rune、20 个 emoji（20 rune / 80 字节，专钉"按字符数而非字节数算长度"）、254 字节邮箱 → 200 且原样读回；把当前值**带前后空格**重提 → 200（trim 后自我匹配，正是前端"只提交改动字段"会走的那条）。用例按语义分成两组：**「两个字段都没给」**（空体、裸 `null`、`{"nickname":null}`）与**「请求体形状/类型错」**（数组/字符串/数字/字段类型错），两组都断言 `param` 为空——前者走的是 service 的"两个都缺"判定、后者才是解码失败，之前混在一张表里写，标签是错的。
- **会话清理**：新增 `loginWithLogout` 辅助（登录 + 在 `t.Cleanup` 里打一次登出），三个改资料用例改用它，跑完不再在 `sessions` 表留行。
- **测试入口脚本**（`scripts/test.sh`，新）：`go test ./... -count=1 -p 1`。**始终**确认 `DATABASE_URL_TEST` 已设置——不传包名时没设直接退出码 1，传了包名只打一行警告再照跑（此时该包的集成用例会静默 skip，`ok` 不代表跑过）。README 新增「测试」章节说明为什么必须串行。

### 过程中的发现

- **`already_taken` 兜底靠并发打不到，得用夹具。** 把并发用例的断言临时收紧成「必须是 `already_taken`」跑了 10 遍（审查复核又跑 20 遍），输家**每次**都是预检给出的 `nickname_taken`。原因：两个请求虽同时发出，但都要先走完会话加载 + `LoadIdentity` 查库才轮到预检，而预检（一次 SELECT）到 UPDATE 之间只有几十微秒的窗口，撞进去的概率极低。**结论不是"测不了"**——加一条测试库触发器夹具即可确定性触发（见上），并发用例的断言因此保持「两者皆可」不收紧。
- **`-p 1` 是绕开、不是根治。** 偶发红的根因是两个包共用同一个测试库，而 `register/enabled` 是**存在数据库里的全局开关**：会去关它的不止一处——`app` 包的 `TestRegistrationControlEndToEnd`，加上 `auth` 包 `register_gate_test.go` 里的 `closeRegistration`（四个用例共用）与 `settings/controller_test.go` 里那条；任何一处关着的时候，另一个包不相干的注册用例撞进去就吃 403。根治要给每个包独立的库或 schema，但测试库是**预先迁移好的**、代码里没有迁移入口（`database.Open` 只连库、不动 schema），代价远超收益，所以在入口脚本上收口。
- 本轮真遇上了那次红：首跑 `go test ./...` 时 `TestRegisterRejectsDuplicateNickname` 返回 403 而不是 409；串行与随后两次并行都绿——典型时序偶发。
- **装夹具时先吃了一个 500。** 触发器第一次跑给的是 500 `internal_error` 而不是 409：`INSERT` 漏了 `group_id`，而该列在 `groups_and_permissions` 迁移里被 `SET NOT NULL`（且没有默认值），于是报的是**非空约束**——ent 的 `IsConstraintError` 只认 `violates unique constraint` / 外键 / check 三种文案（`dialect/sql/sqlgraph/errors.go` 是纯字符串匹配），非空约束一条都不沾，径直落到 500。改成从被更新的行借 `NEW.group_id` 之后才拿到唯一约束。顺带说明：这条兜底对**错误种类**是敏感的，真实场景里能让它生效的只有唯一索引冲突本身。
- **试守卫时踩到本机 `env` 是坏的。** 起初用 `env -u DATABASE_URL_TEST bash scripts/test.sh` 试守卫，什么都没输出、退出码 0，一度以为守卫写错。真因：这台机器上 `env` 解析到 `C:\Users\Multu\.local\bin\env`——一个只往 PATH 里塞目录、**完全忽略参数**的 shim 脚本，`env -u X cmd` 里的命令根本没被执行。本仓脚本不靠 `env` 传参所以不受影响，但这个坑在本机是通用的：任何 `env VAR=x cmd` / `env -u VAR cmd` 写法都会静默空转（不报错、不执行、退出码 0）。

### 验证口径

- `bash scripts/test.sh` 11 包全绿，无 `DATABASE_URL_TEST` 相关 skip（本机 5434；`mail` 那条真实投递用例在未配 `SMTP_*` 时仍按设计 skip）；`gofmt -l internal` 空、`go vet ./...` 空。
- 并发用例 `-count=20`、约束兜底用例 `-count=5` 稳定通过。
- 夹具不留垃圾：跑完用探针查 `pg_trigger` / `pg_proc` 与 `users` 里的 `@collision.test` 行，三项残留均为 0。
- 原子性用例非空转：断言的是「回查后昵称仍等于**改之前**的值」，而请求里给的是另一个值——真发生半写就会红。（独立审查另用变异测试复核过：把实现改成分两步写，两个方向各命中一次。）
- 脚本守卫实测：不设 `DATABASE_URL_TEST` 时全仓跑退出码 1；单包跑打一行警告后照跑（`ok` 但集成用例其实已 skip）。

### 已知缺口

- 并行冲突只做在入口脚本层面：绕过脚本直接敲 `go test ./...` 仍会偶发红，没有从代码层阻止这种跑法的手段。
- 约束兜底夹具依赖 Postgres 触发器，并按昵称前缀 `collide` 触发——将来若有别的用例用这个前缀会互相干扰；`gen_random_uuid()` 需 PG 13+（本机 17）。
- 只改了本轮这三个改资料用例的登出清理，其他既有用例仍在登录后不登出，`sessions` 表仍会有别处留下的行（一小时过期）。
- 既有 `TestRegisterConcurrentSameEmail` 在测试 goroutine 里调 `t.Fatalf` 的毛病本轮没顺手修（属既有代码，另一轮的事）。

---

## 2026-10-13 · 改自己的资料：后端接口 `PATCH /users/me` + 前端把保存接上

**版本**：后端 0.13.0 → **0.14.0**（minor：新对外契约）。**零迁移、零新依赖**。起因：用户在 `/console/uc/profile` 页上改「用户名 / 邮箱」点保存没反应，问"是不是接口的问题"——查明后端**从来没有**写这两个字段的路径（读只有 `GET /auth/me`；`PATCH /admin/users/:id` 只接 `group_id` / `extra_permissions`），于是按用户当场定的四条裁定把接口补上；随后用户一句"可以，请你完成吧"把前端也接上。

### 做了什么

- 新端点 `PATCH /api/v1/users/me`（命名走用户的 A 案）：归 `auth` 模块的「当前用户」能力，声明在 `RegisterProtected`（`auth/controller.go:29`），只挂 `RequireAuth + LoadIdentity`、**不挂权限点**——改的是自己。实现落在 `service.go` 的 `UpdateProfile` + `repository.go` 的 `updateProfile`。
- 请求体 `{"nickname"?,"email"?}`：两个字段都可选、只改给出的那个；**两个都不给 400**。归一与校验复用注册那套（`normalizeNickname` / `validNickname` / `normalizeEmail` / `validEmail`），不符 400 `invalid_request` 且 `param` 指向出问题的字段。
- 撞名：先按 identifier 查一次，**查到的是自己就放行**（所以"原样重提"是 200 而不是 409），并发竞态由 ent 约束错误兜底 409 `already_taken`——与注册同构。错误码按用户裁定直接复用注册的 `nickname_taken` / `email_taken`。
- **不校验当前密码**（裁定 2"不用"）；**改邮箱不验证新邮箱**、**前端不重拉 `/auth/me`**（裁定 4"暂时不做"）。会话只存 `userID`，所以改完不需要重新登录，`/auth/me` 与侧栏下次读取即是新值。
- **前端接上（同一轮的后半段）**：新增 `hooks/use-profile-save.ts`（照 `use-signup` 的套路：`apiRequest` + `ApiError` 错误码逐条映射 + sonner 单条 toast；用的是 `lib/api.ts` 现成封装，没有自己写 fetch），`profile-form.tsx` 改受控并把 `isPending` 传给悬浮保存按钮。**只提交改动过的字段**（后端本来就是部分更新）；成功后 `toast.success` + `router.refresh()`；401 → `replace("/signin")`；5xx/网络错 → `common.serviceUnavailable`，**刻意不走 `useServiceHealth`**（它在挂载时会自己弹一条，而控制台布局里的 `ConsoleServiceToast` 已经管这事，会重复）。
- 前端不做本地预校验（昵称/邮箱规则只留在后端一处，避免两处规则漂移），把 `invalid_request` 的 `param` 映射成字段级文案。

### 验证口径

- 新增 `TestUpdateProfileEndToEnd`（`app_test.go`，真 `app.New` 装配链 + 真库）：匿名 401；空体 400；五组非法值（过短昵称 / 含空格 / 含 `@` / 非法邮箱 / 类型不对的 `42`）各 400；正常改（顺带验 trim 与小写）200；**原值重提 200**（"自己不算撞名"那条的回归）；改完 `GET /auth/me` 读到新值；撞别人昵称 409 `nickname_taken`、撞别人邮箱 409 `email_taken`；只给昵称时邮箱保持不动。
- 全仓 `go test ./... -count=1` 11 包全绿无 skip（本机 5434 Postgres）；`gofmt -l internal` 空、`go vet` 干净。
- README 模块表描述订正 + 接口表补一行 + 规则补一条；api-conventions §3「现状端点全集」补第 24 行。
- 前端 `prettier --check` / `tsc --noEmit` / `eslint` 干净；两条**只读**探测（不带会话、不写库）：`PATCH /api/v1/users/me` 直连 5732 与经 `:3000` 的 `/api` 转发**都返回 401**——说明路由、守卫、Next rewrite 三处都在，且 5732 上跑的进程已经带这条新路由（若是 404 就是旧二进制没重启）。带真会话的保存动作留给用户在浏览器里点。

### 已知缺口

- 带真会话的"点保存"我没跑（用户明确要求运行期验收归他自己）；因此撞名 / 校验失败这些失败态只做到了错误码到文案的映射，没端到端点过一遍。
- 保存成功后靠 `router.refresh()` 让表单与侧栏昵称一起跟上（裁定 4 里"要不要重拉 `/auth/me`"那半条实际就这么落了）；若刷新那一下也失败，页面会停在旧值上，没有额外提示。
- 改邮箱不验证新邮箱、不改密码就不要求当前密码——都是本轮显式裁定，不是遗漏。
- 撞名是"查一次 + 约束兜底"：两人同时改成同名时第二个吃 409 `already_taken` 而不是精确的 `nickname_taken`，与注册一致。

---

## 2026-10-13 · 公开设置读取收口：信封统一 + 聚合端点 + 前端通用层

**版本**：后端 0.12.0 → **0.13.0**（minor：新对外契约）。**零迁移**。起因是用户对上一轮结果的追加要求：「规范这个公共 API，让我方便复用」——当时公开读只有一个刚加的按组端点，前端为注册开关单独写了 `lib/signup-status.ts`，每加一项就得再抄一遍。

### 做了什么

- **信封统一**：一个设置组的记录固定为 `{group, values}`；集合外外套 `{items: […]}`（与 access 的 `itemList` 同形）。管理侧 `GET` / `PATCH /admin/settings/:group` 从裸对象改为组记录，两侧同形，前端一套解析通吃。已写进 api-conventions §6 规则 4，并记明 D4/§9 列表收敛落地时 `items` 与 access 一起迁 `data`，不留第二套形状。
- **公开侧只留一个聚合端点**：`GET /api/v1/public/settings`（无鉴权）只出清单里标了 `Public` 的项，没有任何公开项的组整条不出现；按组点查 `GET /public/settings/:group` 删除（未推送，收口零成本）。前台要读公开设置就一次取全。
- **白名单语义**：`Definition` 新增 `Public` 字段，`Service.PublicSettings` 按注册表顺序出组；新守卫 `TestBuiltinPublicAllowlist` 钉住「公开项集合 == `[register.enabled]`」，谁把敏感项标成公开就红——这是匿名端点唯一的安全闸。
- **前端收进通用层**：`lib/public-settings.ts` 一处负责 fetch（`cache: "no-store"`，漏了会被 Next 静态化）、形状校验、**fail-open**（读不到返回 `{}`，选择器 `settings.register?.enabled !== false` 即视为开放）；`/signup` 关闭时 `notFound()`，登录页 `signupOpen=false` 时整个 footer 不传；专用 `lib/signup-status.ts` 删除，页面里的 try/catch 一并收掉。加下一个公开设置：后端 `Public: true` → 前端加一行选择器 → 页面消费。
- 文案补 `registrationClosed`（zh「注册功能已关闭」/ en "Sign-ups are currently closed"），`use-signup.ts` 加 `registration_closed` 分支，不再掉进兜底的「注册失败，请稍后重试」。

### 实测

后端 11 包 `-count=1` 全绿无 skip；前端 `prettier` / `tsc` / `eslint` 干净。真实链路双向对照：关闭态 `{"items":[{"group":"register","values":{"enabled":false}}]}` + `/signup` **404** + 登录页 `href="/signup"` **0** 次；开启态 `/signup` **200** + 链接 **1** 次；再关回 404。降级新增一行：库不可达时匿名聚合端点 503（前端 fail-open 的依据就在这）。

### 过程中的坑

- **形状一改就击穿了 app 层旧断言**（`default get = 200 map[group:register values:…]` 对不上 `values["enabled"]`）——这条红是好消息，说明断言真的在验形状而不是随手写的绿灯；改判用新的 `envelopeValues()` 解嵌套。
- **判「链接有没有隐掉」不能 grep 中文串**：`去注册` 出现在 next-intl 随页面流下来的文案表 payload 里，HTML 里恒有，我据此一度误判成「没隐掉」。要 grep `href="/signup"`，并且必须做开启态正向对照，否则「0 次」既可能是隐掉了也可能是页面根本没渲染。
- **Next 16 不允同一目录起第二个 dev server**（`Another next dev server is already running`），所以前端只能借在跑的 :3000 验；期间 :5732 的后端进程被并行会话重启过两次，每次都要先确认它的 uptime 与 `/api/v1/public/settings` 的负载形状，才知道跑的是不是当前二进制。
- **前端无测试运行器**（package.json 里没有 vitest/jest），选择器与形状校验只能靠 `tsc` + 后端守卫兜；要真验需要单独立一轮前端测试底座。

### 已知缺口

- 界面仍没有：改注册开关还是得 `PATCH /admin/settings/register`；本轮只把「关」这件事在路由层做实了。
- 聚合端点无缓存，且每组一次查询（当前组数=1，无感）；设置项多了再评估单查询。
- `items` → `data` 的列表信封收敛仍欠着（api-conventions §0 未实施项），本轮已把公开端点登记进待迁移清单。

---

## 2026-10-13 · 注册控制：设置系统接上第一个消费方

**版本**：后端 0.11.0 → **0.12.0**（minor：新能力域）。提交 `f3d762a`…`de1e871`（7 个提交，全部在 `backend/`，**零迁移**）。spec 与实施计划在 `docs/superpowers/{specs,plans}/2026-10-13-neoblog-registration-control*`。

### 做了什么

- 清单落第一项 `register/enabled`（开关，默认 `true` = 现网行为零变化）；`auth.Service.Register` 首句读它，关闭时 403 `registration_closed`（新错误码，`permission_error`），**判定先于字段校验**；`auth.CreateAdminUser` 不经 `Register`，保持关闭态仍能建首个管理员。
- 设置模块从「无路由能力模块」变成有路由模块：新增 `module.go` / `controller.go`，`New(db)` 改回返回 `(*Module, error)`（照 `mail` 形状，仍是唯一返回 error 的构造器），`GET` / `PATCH /api/v1/admin/settings/:group` 按组通用读写；`App` 里那个无出口的 settings 私有字段删除（`mailer` 留字段是因为 `shutdown` 要 `Close` 它）。
- `Service.PatchGroup` + `ItemError`：请求体值类型以服务端清单为准（复用 `decodeTyped`，`3.5` / `1e3` 这类越界天然被拒），任一项不合法即整组零落库；controller 用 `errors.As` 取项名填进 `apierror.WithParam`。
- 鉴权走**新权限点** `system.settings.view` / `.edit`（`edit` 经 `rbac.Expand` 隐含 `view`），不是 `RequirePresetGroup`——取向是将来控制台能按角色勾选。核过：owner/admin 的生效权限运行时取自代码目录（`PresetPermissions → All()`），`permission` 列是纯字符串无枚举无外键，**加权限点不需要迁任何数据**；新键追加在 catalog 末尾以免漂掉既有断言。

### 过程中的坑

- **计划漏了一个守卫**：`rbac_test.go` 的 `TestCatalogMatchesFixedValues` 把权限目录**逐条**钉死，加键必红。审查计划时只核了 `access` 侧那个用 `len(rbac.Catalog())` 动态比对的用例，以为全仓没有第二处。
- **开关是落到库里的状态，换一个新 app 实例并不能重置它**：对照用例（关闭 403 → 打开后同一个体 400）必须在同一个服务实例上显式 `SetBool(..., true)`；另配 `t.Cleanup` 里 `ResetItem` 删行，否则测试库里残留 `false`，同库后续不相干的 auth 用例会莫名吃 403。
- **关闭态的字段校验对照不能用空体**：`signup` 在 controller 层就因三个字段为空判 400（`auth/controller.go:52-57`），请求根本进不到 service——要用「非空但 `validEmail` 会拒」的体（邮箱无 `@`）才证得了门在服务第一句。
- **降级态造不出带身份的客户端**：`newDegradedServer` 起的是库不可达的 app，登录本身就 503，所以设置端点的降级用例只能断未登录 401；注册侧的 503 由既有 `app_test.go:548` 那条合法体用例继续覆盖，不必新建函数。
- 测试脚手架没装 NoRoute 信封，PATCH 打到只注册了 GET 的路径时返回的是 gin 默认 404 纯文本，解 `code` 字段直接红——原本担心的一条会假绿，实际不存在。

### 验证口径

- 集成用例是**真跑**不是 skip（本机 5434 Postgres，`SKIP` 计数为 0）：设置包 4 条 GET + 7 条 PATCH + 4 条注册把关 + `TestRegistrationControlEndToEnd`（真 `app.New` 装配链，owner 关开关 → 匿名注册 403 → 打开 → 同一个体 201）。
- 真实进程冒烟：`ADDR=:5741 go run .` 的 gin 路由表里有这两条；匿名 `GET` / `PATCH` 均 401（不是 404 也不是 405，说明路由与守卫都在），非法邮箱注册 400（说明把关那次读库成功、默认判为开放）。

### 已知缺口

- 前端零感知：注册页照常显示，提交才吃 403；并行会话那份前端设置页骨架（五个分区）里**也没有注册落点**，且已被其自行回滚。要做到「按钮直接标为已关闭」需要设置 spec §10 的「公开读取」插槽。
- **改开关目前只能打接口**（本轮只后端，没有界面）：`PATCH /api/v1/admin/settings/register` 带 owner 会话，或 `ResetItem` 语义上恢复默认。
- 权限点可授出：`system.settings.edit` 能授给自建组，即非 owner/admin 的角色也可能被授「改注册开关」——这是选权限点而非预设组守卫的既定取向，不是疏漏。
- 每次注册多一次单行点查，无缓存；两个管理员同时 PATCH 为最后写赢（行级 upsert，无版本号）。
- 孤儿行沿用：若将来删掉 `register/enabled` 这一项，库里残留行不会被任何读取路径碰到，只占复合键，清理仍走人工 SQL。

---

## 2026-10-13 · 环境轮的复核与入库：几条取舍留档

**版本**：后端 **0.11.0**（未进位）；本轮**零源码语义变更**，只做审查复核并把环境轮改动入库。入库提交 `6ff236d`（10 个文件、+309/−30，未推送，`development` 领先 `origin/development` 3）。

### 复核怎么做的（为什么不照读文档结论）

- 文档写「全绿」不算结论，全部重跑：`gofmt -l internal` 空、`go vet ./...` 空、`go test ./...` 全绿（本机 5434 的 Postgres 在跑，集成用例是真执行的，不是 skip 出来的绿）。
- 环境这类判据必须拿产物自证：`bash scripts/build.sh .scratch/envprobe/prod.exe` 打一份发布产物，在库不可达（`DATABASE_URL` 指到死端口）的情况下启动，`GET /api/v1/healthy` 的 `environment` 为 **production**，启动日志 `[GIN-debug]` 行数为 **0**，只有 `listening on :5739` 一行——与上一节实测表里「build.sh 产物，不设 `APP_ENV`」那行对上。探针产物用完即删，`.scratch/` 已在 `.gitignore` 内，不在仓库路径里留 34 MB 二进制。
- 改动范围按消费者收口核一遍：非测试代码里读 `cfg.Environment` 的只有三处——`app.go:49` 的 gin 模式、`app.go:126` 的 cookie `Secure`、`modules/health/service.go:38` 的健康检查响应；`"production"` 字面量已全部收敛到常量，没有漏网的字符串比较。

### 关键取舍

- **`ginMode()` 的 `default → release` 兜底保留**（`app.go:113-114`，对应断言 `app_test.go:240` 的 `"bogus"` 用例）。审查一度把它列为「生产路径走不到」的多余分支，结论是不删：它钉住的是「未知值走安全侧」这个方向，删掉之后谁把 default 改成 `gin.DebugMode` 测试照样全绿，等于这个选择没人守；成本一行，零维护。同时记明它的价值上限——守的只是手造 `Config` 漂移这类测试内场景，真正的把关在 `TestLoadRejectsUnknownEnvironment`，兜底只是第二道。
- **两类兜底要分开判**：展示层的等效构造（后端挂了时前端造空状态那类）按既定口径整条删；配置层与纯函数的 fail-closed 默认值连同测试保留。判据是「删掉之后方向会不会被静默反过来」，不是「可达不可达」。
- **已定形的改动不重做**：`Load()` 里那层只转发 `EffectiveEnvironment()` 的 `loadEnvironment()`，在复核与拍板之间已被并行会话删掉（`config.go:67` 现为直调）。处理方式是先重核工作区（grep 无残留、文档无过时引用、文件 mtime 落在两轮之间）确认形状已对，再决定不动手，而不是按原计划改第二遍。同类项还有 README「运行」章节末段与 `app_test.go` 的一行，均为并行会话在制品，按「一起提交」的口径随同一提交入库，提交消息据实际内容反推。

### 本轮没做 / 仍未验证

- panic 日志随模式漂移、request id（10-12 定的「对内可诊断」）仍未做。
- `APP_ENV` 在加载层硬拦（过滤 `.env` 里这个键）仍是可选后续项，现状只有文档约定，代码照读。
- **发布产物默认 `Cookie.Secure=true` 这条分支依旧没有端到端验证**：非 localhost 的明文 http 部署下浏览器会直接丢弃 session cookie，症状是登录接口 200、下一步 `/auth/me` 全 401，看着像「登录没生效」。要证它需要真浏览器 + 非 localhost 明文地址。
- 仍无容器镜像与部署定义：`deploy/` 只有 postgres 的 `init.sql`，口径里的「镜像」目前等价于 `build.sh` 产出的单个可执行文件。

---

## 2026-10-13 · 后端环境口径收口：构建产物即生产，开发态只属于 `go run`

**版本**：后端 **0.11.0**（本轮未进位）；接口字段名与类型不变，但 `/api/v1/healthy` 的 `environment` **取值**现在随**构建方式**而变（`development` → `production`），依赖该字段判断环境的按变更处理。

注：本节写于本轮改动入库前；其中「未做 / 未验证」各条在同日的审查修复轮里已处理掉两条（`APP_ENV` 空值口径、生产态 `.env` 自举），下方补记标注了实际落地形状。

起因是把「日志有没有颜色 / 有没有路由表」当成了生产与开发的判据，核查后发现两个 exe 跑的都是开发态，且所谓「生产实例」在自己脚边生成了一份开发形状的 `.env`。

### 做了什么

- `internal/config/config.go`：新增 `var DefaultEnvironment = "development"`（发布构建用 `-X` 覆盖）与 `EnvironmentDevelopment/Production/Test` 三常量；`APP_ENV` 从「任意字符串」收紧成只认这三个值，非法值 `Load()` 直接报错点名 `APP_ENV`（此前 `prod` / `Production` / `staging` 会静默按非生产跑）；新增 `Config.IsProduction()` 作为唯一判定入口。
- `internal/app/app.go`：`gin.SetMode` 从「只管 production」改成按环境三分（production→release、development→debug、test→test）；`Cookie.Secure` 改走 `cfg.IsProduction()`。
- `scripts/build.sh`（新）：发布构建唯一入口——`CGO_ENABLED=0` + `-trimpath` + `-ldflags "-s -w -X website/backend/internal/config.DefaultEnvironment=production"`；版本号从 `internal/version/version.go` 读，产物名 `bin/neoblog-<版本>-<GOOS>-<GOARCH>[.exe]`，支持位置参数指定输出路径与 `GOOS`/`GOARCH` 交叉构建。
- 测试：`config_test.go` 新增 3 条（编译期默认值 / 显式覆盖三值 / 非法值拒绝）；`app_test.go` 的 `TestGinModeFollowsEnvironment` 直接对 `ginMode()` 纯函数断言四值（含兜底 release），不依赖数据库、且退出时恢复 gin 全局模式。`go test ./...` 全绿、`gofmt -l` 空、`go vet` 干净。
- 文档：`backend/README.md` 新增「环境与构建」章节；同步 `APP_ENV` 行、目录对照表加 `scripts/build.sh` 行、「认证」章节 cookie `Secure` 的口径、「运行」章节加指针。

### 关键决定

- **环境不做成两份代码，靠编译期注入一个变量**：源码一份，`go run .` / 裸 `go build` 出开发态，`scripts/build.sh` 出生产态。与 `internal/version.Version` 同一个 `-X` 机制。
- **`APP_ENV` 刻意不进 `.env` 与 `.env.example`**：写进 `.env` 会让同一目录下的发布二进制静默退回开发态——那正是本轮起因那类误判的来源。环境开关只能来自「怎么构建的」或「显式设环境变量」（代码仍会读 `.env` 里的 `APP_ENV`，已在「未验证 / 已知缺口」标注为可选的后续硬拦项）。
- **gin 模式完全由 `APP_ENV` 决定，`GIN_MODE` 不再能单独生效**：此前两个开关脱钩（只设 `GIN_MODE=release` 会日志安静、但 cookie 仍非 Secure、health 仍报 development）。代价是 `APP_ENV=development` 现在会强制把 gin 打回 debug，即使用户同时设了 `GIN_MODE=release`——取单一真源。
- **本轮只收口环境，`.env` 自举语义按同一轮审查结论另行收紧**：自举从「无条件」改为「仅开发态 / `test`」，生产态不自举，见缺口第一条的处理结果。

### 实测到的边界（2026-10-13，同一份源码四种跑法，探针在 `backend/.scratch/modeprobe/`，该目录已进 `.gitignore` 且临时钥匙文件已删）

| 跑法 | `[GIN-debug]` 行数 | `/healthy` 的 `environment` |
| --- | --- | --- |
| `build.sh` 产物，不设 `APP_ENV` | **0** | **production** |
| `build.sh` 产物，`APP_ENV=development` | 21 | development |
| 裸 `go build` 产物，不设 `APP_ENV` | 21 | development |
| `build.sh` 产物，`APP_ENV=prod` | 0 | 启动即失败：`APP_ENV must be one of "development", "production" or "test", got "prod"` |

顺带钉死两条日志口径（此前被当作环境判据，是错的）：`[GIN-debug]` 路由表**只在启动那一刻打印、且永不上色**（`gin/debug.go:69` 那句 `Fprintf` 里没有任何 ANSI 码）；`[GIN] 时间 | 状态码 | 耗时 | IP | 方法 "路径"` 是每条请求都打的访问日志，**debug 与 release 都打**（`app.go` 无条件 `engine.Use(gin.Logger())`），带不带颜色只取决于 stdout 是不是终端（`gin/logger.go:258-262` 算 `isTerm`、`:163` 用它决定上色，`SetMode` 不碰这个变量）。生产态那次实测照样打出 `[GIN] … | 503 | … | GET "/api/v1/healthy"`。（gin 行号以当时 `go.mod` 的版本为准，仅备查。）

### 补记（同轮收尾时补齐两笔入库的日志）

- `92e5afa`：没有 `.env` 时首次启动自动生成一份纯键值文件（`EnsureDotEnv`，`O_EXCL` 绝不覆盖、含随机 32 字节 2FA 钥匙），不再因缺环境变量退出。
- `63ffbe9`：数据库连不上也照常启动，碰库端点统一 503 `service_unavailable`（连接级错误判定），连接池惰性重连、库恢复无需重启。
- 同日审查修复：`APP_ENV` 空串也判非法（`LookupEnv` + trim，不再静默吃成默认值）；`.env` 自举收紧为**仅开发态 / `test`**，`production` 不生成文件（修复下方缺口第一条）；`ginMode()` 提成纯函数并补兜底 release，测试不再依赖数据库；`backend/.gitignore` 增加 `.scratch/`，探针产物与其中的临时钥匙文件不再有误入仓库的路径。

### 未验证 / 已知缺口

- **发布产物默认 `Cookie.Secure=true`，这条分支此前从未真跑过**：生产若走 `http://`（非 localhost 的明文域名 / IP），浏览器会直接丢弃 session cookie，症状是登录接口 200、下一步 `/auth/me` 全 401，看起来像「登录没生效」。**未做端到端验证**（需要真浏览器 + 非 localhost 明文地址）。
- **`EnsureDotEnv` 生产态自举已修**（同日审查修复）：自举只在开发态 / `test` 生效，`production` 遇到缺 `.env` 不造文件。原缺口记为：生产产物在没有 `.env` 的目录里会自造一份开发形状配置（本地 compose 的 `DATABASE_URL`、`DATABASE_URL_TEST`、外加一把全新随机 2FA 钥匙）；本轮起因 `F:\backend.exe` 在 `F:\` 生成的正是这种文件（sha256 前缀 `c4ce87ff` vs `backend/.env` 的 `3124744e`）。
- **没有容器镜像与部署定义**：`deploy/` 仍只有 postgres 的 `init.sql`，无 Dockerfile、无 app 服务、无 systemd 单元。用户口径里的「镜像」目前等价于 `build.sh` 产出的单个 exe。
- **`APP_ENV` 加载层硬拦是可选后续项**：文档只做「约定不写进 `.env`」，代码仍会读 `.env` 里的 `APP_ENV`（`LoadDotEnvFile` 不按 key 过滤）。要在加载层过滤或告警，需按「用户点名才做」的惯例另行决定。
- panic 日志随模式变（debug 额外 dump 整份请求，release 只打堆栈，见 `gin/recovery.go` 的 `IsDebugging()` 分支）——本轮未统一；10-12 定的「对内可诊断（500 / panic 日志 + request id）」仍未做。

---

## 2026-10-13 · 后端连不上时三条路由不再 500（外壳 / 表单照常渲染 + 一条 toast）

**版本**：前端 `package.json` 仍 **0.1.0**；本轮**零后端改动**、零接口契约变更。

### 做了什么

- `app/console/layout.tsx`：外壳 JSX 原样提成 `ConsoleShell` 函数；权威判定的抛错改为 `catch` → 渲染**无昵称的外壳** + 新组件 `ConsoleServiceToast`，`redirect("/signin")` 仍在 `try` 之外（`:46`）。
- `app/signin/page.tsx`、`app/signup/page.tsx`：同形改动——判定抛错时照常渲染 `-view`。此前实测：带 cookie 且后端停机时登录页自己返 500，用户连输密码重试的地方都没有。
- 新 `components/console/console-service-toast.tsx`：client 组件，effect 里 `toast.error(t("serviceUnavailable"), { id })`，`useRef` 挡 StrictMode 双跑，自身 `return null`。复用既有键 `common.serviceUnavailable`，两份语言包**零新增**。
- 三条路由各加 `export const instant = false`（阻塞式读 cookie 的段按 `instant.md:69` 声明「允许阻塞」）；root layout 那条**没撤**。

### 关键决定

- **用户拍板「两侧都放行」**，推翻 `specs/2026-10-12-neoblog-console-auth-design.md` v1.1 里「不做『后端挂了先放控制台外壳』的降级」那一行，也超出 10-12 只放宽访客页的提议范围。spec 同步升到 **v1.2**（§2 两行、§5 的 I1/I4、§6 用例 6、§7 两条、§8 三个插槽、§3 范围），plan 的 Review Focus 1 与 Task 2 Step 7 期望值同步改写（10-12 的实测存档不改写，另加订正段）。
- **判定层三态一字未改**：`lib/auth-session.ts` 故障时照旧抛错，变的只是抛错之后由谁接。「故障 ≠ 未登录」因此没退化——`/console` 带 cookie 遇故障是 200 外壳，若被错写成未登录就会是 307 回 `/signin`，鉴别点仍然成立。
- **`catch` 的粒度本轮不动**（用户决定）：连不上 / 非 200 / 响应体畸形三种原因被同一个 `catch` 合并，服务端零日志，于是真 bug 也会被显示成「服务不可用」，与 10-12 定的「先做对内可诊断」相反。已登记到 spec §8。
- **不新建 `error.tsx`**：沿 v1.1「避免造兜底 UI」，可读提示就交给那一条 toast。外壳形状不随后端数据变——无昵称时用户菜单只是少一段文字（`console-user-menu.tsx:79` 本来就在 `displayName` 为空时不渲染），没有任何新构造。
- **`/console` 上不会双 toast**：客户端健康探测 `useServiceHealth` 只挂在 `use-signin` / `use-signup`，控制台外壳里没有任何组件自己发请求，所以这条是唯一一条。

### 实测到的边界（2026-10-13，`:5732` 无人监听即后端不在）

| 请求 | 实测 |
| --- | --- |
| `Cookie: session=whatever` → `/console` | **200**，响应体含 `console-sidebar`（改动前该组合是 500） |
| 无 cookie → `/console` | `307 /signin`，proxy 未受本次改动波及 |
| 带 cookie → `/signin` 与 `/signup` | **200**，表单照常渲染 |

`npx tsc --noEmit`、`npx eslint`、`npx prettier --check` 三项退出码 0。

### 未验证 / 已知缺口

- **toast 的画面未取证**：它在水合后由 effect 发出，本轮只用 curl 证了状态码与响应体；「弹几条、导航时会不会重弹」需浏览器侧确认（`id` 去重 + `useRef` 是双保险，未实测）。
- D 态（2FA pending）与 prefetch 撞 307 仍未取证，沿 10-12 的欠账，本轮未动。
- 故障期 `/console` 属「未验明身份也渲染」，I1 已由充要降为必要；**该路由下新增子页不得假定布局已判过身份**。
- 后端 pending 键破损（G 态）、`docs/DEVLOG.md:399` 的 logout 204 陈旧等 10-12 已登记项本轮未动。

---

## 2026-10-12 · 服务不可用报错（两个表单不再谎报凭证错误）

**版本**：前端 `package.json` 仍 **0.1.0**；后端 0.11.0 不动，本轮**零后端改动**、零接口契约变更。

### 做了什么

数据库连不上时，登录/注册原本一律弹「登录失败，请稍后重试」（`use-signin.ts` 的兜底分支），把服务侧故障说成用户填错。现在：

- `lib/api.ts` +2 个导出：`looksLikeServiceFailure(error)`（HTTP 0 或 ≥500，只回答"这次错误值不值得去查一次健康"）与 `isServiceUp()`（打 `/api/v1/healthy`，**非 200 一律算不可用**，是唯一的结论来源）。`apiRequest` / `ApiError` 一字未动。
- 新 `hooks/use-service-health.ts`：`useServiceHealth()` 返回 `{ serviceDown, resolveServiceError }`。`resolveServiceError(error?)` 是统一入口（传错误 = 提交场景，不传 = 纯探测），两种场景给的是同一句话、这句只在 hook 顶部算一次；挂载时在 effect 里探测一次，不可用除弹一条外还把 `serviceDown` 置真，带 `active` 标记防卸载后弹（StrictMode 双跑也只弹一次）。
- `hooks/use-signin.ts` / `use-signup.ts`：提交失败的兜底改走统一入口，拿不到才落回原业务文案；`toMessage` / `toFailure` 里的 code→文案映射一条没删，注册页故障时不落焦点到任何字段。`serviceDown` 经 hook 透传给表单组件。
- `app/signin|signup/*-form.tsx`：`serviceDown` 为真时提交按钮文案换成「服务不可用」，**按钮不加 `disabled`**；`*-view.tsx` 本轮零改动。
- 两份语言包各 +2 键 `common.serviceUnavailable`（「服务暂时不可用，请稍后重试」）与 `common.serviceUnavailableButton`（「服务不可用」）。

### 关键决定

- **判定收成单一信息源**（用户拍板"两处都只认 `/healthy` 探测"）。初版是进页探测 `/healthy`、提交时看接口自己的状态码——一个判定两个信息源，读起来像实现了两遍。现在提交侧不再自己下结论，状态码只当闸门。
- **闸门保留**，否则密码每打错一次就多发的一个健康请求（harness 实测：有闸门请求数 0，无闸门 1）。
- 原命名 `isServiceUnavailable` 已改名为 `looksLikeServiceFailure`：收敛之后它不再下判决，旧名字读起来像在下判决。同轮里 hook 文件也从 `use-service-error.ts` 改名为 `use-service-health.ts`（它现在管的是状态而不只是报错文案），`c5408dc` 那个提交里记的是旧名。
- **没新增 503 `database_unavailable`**：会推翻 api-conventions §4「500 之外的 5xx 一律归并为 500、503 只给 `/healthy`」，还要判定哪些 ent 错误算数据库掉线（连接池耗尽 / 超时 / 慢查询语义不同），误报会把普通 bug 说成库故障。界面因此只说"服务"不可用，不特指数据库。
- 文案放 `common` 而非两个 `auth.*.failed` 各加一条：三个消费者共用同一句，拆开必然漂移。
- **不做去重**，登录/注册两页之间来回跳各弹一次——沿 `0e8a4be` 定的"连点提交弹几次算几次"。
- **故障时不禁用提交按钮，只改文案**（用户拍板"服务不可用 按照这个改吧"）：禁用会把「点提交」这条最省事的恢复检测堵掉——`/healthy` 的结论只是进页那一刻的快照，服务恢复后前端并不知道，而用户点一次提交恰好是一次免费重探；灰色本身不解释原因，要诚实就还得再挂一条常驻说明，等于把已有那条 toast 换成两套东西；另外沿用 2026-10-01 加载占位那轮定过的「常驻行必须真可用，禁用＝灰色遮蔽」否例。配套地，`resolveServiceError` 在拿到 4xx 业务错时会 `setServiceDown(false)`——服务器能回 4xx 就说明它活着，按钮文案当场恢复正常。

### 实测到的边界（记下来免得下次重查）

- **后端进程不在时，浏览器看到的不是网络错误，是 Next 自己返 500 + 纯文本 `Internal Server Error`**（`curl -i POST /api/v1/auth/login`，dev 3000 端口实测）。因为 `/api/*` 由 `next.config.ts` 的 rewrites 转发，断点在服务端那一跳；`apiRequest` 解析不出 JSON 于是给 `ApiError(500, "invalid_response")`。`status === 0` 只在"浏览器到 Next 这一跳也断了"时才出现。
- 后端 boot 时 ping 失败即 `log.Fatal` 退出（`backend/main.go:22-25`），所以"数据库连不上"最常见的形态是整个 HTTP 服务不存在。
- `/healthy` 在库掉线时返 **503** + `{"status":"degraded","database":"error"}`（`health/controller.go`），且它不受错误信封约束，非 200 时 `apiRequest` 拿不到 `error.code`。
- `useTranslations()` 的返回值是 `useMemo` 过的（依赖 cache / formatters / messages / locale / timeZone，见 `node_modules/use-intl/dist/esm/production/react.js`），放进 `useEffect` 依赖数组不会导致重复探测。
- **`react-hooks/set-state-in-effect` 会追进 effect 调用的回调里**：只要那个函数存在任何一条"未经 `await` 就 setState"的分支（本例是 4xx 直接 `setServiceDown(false)` 后 return），规则就报 "Calling setState synchronously within an effect"，哪怕实际调用路径必过 `await`。解法：effect 不复用那个入口函数，自己 `isServiceUp().then(...)` 置状态；文案在 hook 顶部算成常量串供两边用。
- **Qoder 内置浏览器跑不动这站**：37 个 JS chunk 全 200、`window.next` 已定义，但 6 秒内 React 事件属性始终没挂到 DOM（`__react*` 键为空）、`window.__next_f` 长度 0、`/__nextjs_font/*` 与 HMR WebSocket 一律 403 → 页面不水合，客户端 effect 根本不执行。**结论：本项目的 toast / 交互类验收不能靠内置浏览器**，只能无头 harness + 人工眼验。
- 无头 harness 四态矩阵（判定逻辑逐条实测，非推演）：后端不在 / 库运行中掉线 / 浏览器到 Next 断了 → 进页与提交都报同一句并各发 1 次探测；一切正常但密码错 → 仍报「用户名或密码错误」且探测请求数 0。`tsc --noEmit`、`eslint`、`prettier --check` 全绿。

### 已知缺口

- **`/console` 的整页可读错误页仍未做**（`2026-10-12-neoblog-console-auth-design.md:35` 与 `:168` 登记给"需要可读错误页那轮"）。后端故障时现在还是 Next 默认错误页 / dev 红堆栈。
- **本轮未验**：toast 与「服务不可用」按钮文案的实际渲染都只在浏览器里才看得见，而内置浏览器不水合（见上），需要人工开 `http://127.0.0.1:<dev 端口>/signin` 在停机状态下各看一眼：进页弹一条、按钮文字变成「服务不可用」、点下去仍会重探。
- 登出失败文案 `userMenu.signOutFailed` 没接这套判定（本轮点名范围只有两个表单）。
- 前台公开页（`app/page.tsx` 当前是空 div）将来接上内容后需要自己的故障态。
- `isServiceUp()` 把 `/healthy` 任何非 200 都算不可用 → 该路由若改名或被摘掉，进页会弹「服务暂时不可用」，属误导性诊断。

---

## 2026-10-12 · 控制台访问控制（proxy 短路 + 服务端权威校验 + 登出）

**版本**：前端 `package.json` 仍 **0.1.0**（前端版本进位口径未定，见 spec §8 插槽）；后端 0.11.0 不动，本轮**零后端改动**。

### 做了什么

- 两层门禁。新 `frontend/proxy.ts`（Next 16.4 里 middleware 已改名 proxy）只做乐观短路：`matcher: ["/console/:path*"]`，有 `session` cookie 就 `next()`、没有就 307 `/signin`；不调后端、不引共享模块。权威校验集中在新文件 `frontend/lib/auth-session.ts` 的 `getAuthenticatedUser()`——带 cookie 请求 `${backendUrl}/api/v1/auth/me`，**三态**：200 → `{email, nickname}`、401 → `null`、其余（5xx / 404 / 405 / 连不上 / 体解析不出）→ **抛错**。
- `app/console/layout.tsx` 由同步改异步：判到 `null` 就 `redirect("/signin")`；把 `nickname` 经 `ConsoleSidebar` 传给一直空传的 `ConsoleSidebarFooter` / `ConsoleUserMenu` 的 `displayName`。
- `app/signin/page.tsx`、`app/signup/page.tsx` 加反向跳：`if (await getAuthenticatedUser()) redirect("/console")`。判定放服务端 page 而非 `-view.tsx`（那是 Client Component，`instant` 在客户端会直接报错）。
- 登出：新 `frontend/hooks/use-signout.ts`（`{ isSigningOut, signOut }`）+ 用户菜单「退出登录」项，`POST /api/v1/auth/logout` 成功 `router.replace("/signin")`、失败 `toast.error`；两份语言包各 +2 键（49 键对齐）。逻辑留 hook、菜单只渲染，延续「UI 只管 UI，逻辑只做逻辑」。
- `hooks/use-signup.ts` 成功去向 `/signin` → `/console`：注册即登录与反向跳两条叠加后，推 `/signin` 会被立刻弹回、白跳一次。
- 新 `lib/backend-url.ts` 收口后端地址，`next.config.ts` 的 rewrites 改引它。
- 设计由两个独立只读审查代理逐条证伪后修订成 v1.1；v1.0 有 6 处事实错误（列在 spec 头部修订记录）。spec `2026-10-12-neoblog-console-auth-design.md`、plan `2026-10-12-neoblog-console-auth.md`。

### 关键决定

- **反向跳故意不放 proxy**（官方示例就放在 proxy，`authentication.md:1061-1069`）：后端对已开 TOTP 的账号会下发**有效 cookie 却不写 `auth.user_id`**（`service.go:109-113`），此时 proxy 的「有 cookie 就弹走」与布局的「401 就送回」会拼成 `/signin ⇄ /console` 无限重定向。两处判定必须共用同一个权威函数才不成环。
- **后端故障 ≠ 未登录**：v1.0 的「非 200 一律当未登录」被推翻——那样后端一挂，所有人（含已登录者）被静默判成未登录，连登录页都进不去且看不到原因。故障现在走 `throw` → Next 默认错误页。
- **session 读取故意放 layout 顶层**（官方 `authentication-with-cache-components.md:156` 明确劝退，理由是连 children 一起拖住）：门禁必须先于 children 渲染，代价是控制台整段多等一次内网 RTT。
- `matcher` 收窄到 `/console/:path*`，与官方「鉴权场景建议 proxy 跑全部路由」的 Tips 相反：`/api` 若进 matcher，会在 rewrites 之前被重定向劫持（proxy 执行在第 3 步、`beforeFiles` rewrites 在第 4 步，`proxy.md:240-244`）。
- **没用 `server-only` 包**（本机未安装，为一个防护 import 引新依赖不值当）；该模块 import 了 `next/headers`，被 Client Component 误引时 Next 自身就会构建失败，这层保护是白送的。

### 实测到的边界（记下来免得下次重查）

- `NextResponse.redirect` 默认 **307**（`node_modules/next/dist/server/web/spec-extension/response.js:95`）；服务端布局里抛 `redirect()` 对 HTML 请求同样是 307。
- 后端停机时：带任意 cookie 访问 `/console` → **500，响应体含 `session lookup failed`**；无 cookie → 仍 307（proxy 不依赖后端）；`/signin` 无 cookie → 200（压根不发那次请求）。
- **脏 cookie 不自愈**：`session=garbage` 的两个响应都不带 `Set-Cookie`（本项目未设 `IdleTimeout`，未改动的会话不回写），要等浏览器侧 `Max-Age` 到期或被一次成功登录覆盖。
- 登出实测 `set-cookie: session=; Path=/; Expires=Thu, 01 Jan 1970 ...; Max-Age=0; HttpOnly; SameSite=Lax`；同一 token 再进 `/console` 由布局层拒（proxy 放行）——两层分工的现场证据。
- dev 的 Server Component fetch 有 **HMR 缓存**，且对 `cache: 'no-store'` 同样生效，只在导航/整页刷新时清（`serverComponentsHmrCache.md:9-11`）。状态类验收必须冷请求或整页刷新，否则会得出「已退出却还像登录态」的错误结论。
- `proxy.ts` 与 `next.config.ts` 的改动都被 dev 自动热加载，无需重启；`/console/whatever` 这类不存在的子路由同样被 matcher 命中（proxy 先于路由执行）。
- 注册成功码是 **201**（`controller.go:65`），不是 200。

### 已知缺口

- **D 态（2FA pending 会话）本轮未实测**：不造开 TOTP 的账号，无环结论目前只有代码与推演支撑；取证步骤整段留在 plan 的 Task 7（已标推后）。
- **prefetch 撞 307 的行为未取证**：dev 不 prefetch，要验只能 `next build` + `next start`。
- 会话中途失效时，界面内的接口调用不会自动跳登录（`lib/api.ts` 不处理 401）。控制台开始消费真实数据那轮补，落点是它的单一出口。
- 本轮让 `/console`、`/signin`、`/signup` 每次渲染各多一次后端会话查询，而全站**零限流**（`backend/README.md:152`）。
- **后端不变量破损（本轮不改后端）**：`register` 与正常 `login` 都只写 `auth.user_id`、**不清** `auth.pending_2fa_user_id`（`Remove` 只出现在 verify 成功路径 `service.go:378-379`），而 `POST /auth/2fa/verify` 是公开路由、按该键放行 → 一枚已登录会话可能残留另一个账号的 pending 身份。利用前提苛刻（先掌握对方口令、再拿到对方动态码），不是远程可利用，但与 2FA spec 的不变量承诺相悖。
- 上一轮做的纯 CSR 门控（`hooks/use-is-client.ts`）已被并行会话撤销，本轮未恢复，登记为插槽。

### 更正既有文档口径

- `plans/2026-10-11-neoblog-auth.md:13` 与 `specs/2026-10-11-neoblog-auth-design.md:10` 仍写「前端未接 / Task 9 作废」——前端接线已随 `16e210a`–`22d272c` 入库，本轮又加门禁，两处已就地补修订说明。
- 本文件 0.11.0 条目里「本轮前端仍无任何对 `/api/v1` 的调用（全仓 grep 无 fetch 与后端地址常量）」现已失效。
- `logout` / `password` 成功码在 `DEVLOG:335`、`DEVLOG:399`、`specs/2026-10-11-neoblog-auth-layer2-design.md:48/85/100`、`specs/2026-10-11-neoblog-auth-design.md:86` 写成 **204**，代码是 200 `{}`（`controller.go:73/130`）；`backend/README.md:135` 本来就对。逐处加更正标注。

---

## 2026-10-12 · 方法不匹配改判 405（路由层错误分流）

**版本**：0.10.0 → **0.11.0**（minor：新增对外 HTTP 行为 + 1 个错误码）

### 做了什么

- 起点是用户要「方法用错也返回 404 隐藏端点」，实测确认 gin 默认行为已经是 404 信封（`HandleMethodNotAllowed` 默认 `false`，方法不匹配当作路由未匹配走 `NoRoute`）；用户随后改判取规范口径：**方法不匹配 405 + `method_not_allowed` / `invalid_request_error` + `Allow` 头**，路径不存在仍 404 + `not_found` 且不带 `Allow`。这推翻 API 规范 v1.3 的 §4 规则 2，规范补 v1.4 修订记录并同步 §4 状态码清单、§7.2、§7.3。
- `apierror` 注册表 +1（26 → **27** 条）：哨兵 `ErrMethodNotAllowed` 与 `"method_not_allowed" → 405` 映射。`Allow` 头由 gin 自己按 RFC 9110 填（`gin.go:738-752`），代码里不手拼方法列表。
- `app.go`：`engine.HandleMethodNotAllowed = true` + `engine.NoMethod(...)` 写同一信封；`NoRoute` 原样保留。
- 测试：`app_test.go` 新增 3 个用例函数——方法不匹配表驱动 4 例（POST-only 用 GET、GET-only 用 DELETE、PATCH+DELETE 路由用 PUT、OPTIONS），逐例断状态码 + `type` + `code` + `param` 为 `null` + `Allow` 集合（排序后比对，不依赖注册顺序）；HEAD 单独一例断 405 + `Allow: GET` + **响应体为空**；反空转守卫 2 条——不存在的路径用 GET/POST/DELETE 三种方法仍 404 且无 `Allow`、同一路径方法写对时仍进认证守卫拿 401（证明真路由没被 `NoMethod` 劫走）。`TestRegistryIntegrity` 的哨兵数与 `statusTypes`（405 → `invalid_request_error`）同步。
- 文档：README 信封段补 405 / `Allow` / 回退开关 / HEAD 与 OPTIONS 边界；API 规范补 v1.4。

### 关键决定

- **405 而非 404**（用户知情选择）：`Allow` 头等于向匿名探测者确认该端点存在及其允许的方法。回退只需把 `HandleMethodNotAllowed` 关回 `false`，方法不匹配立刻与路径不存在同为 404——这条开关在 README 与规范里都写了。
- **归 `invalid_request_error`，不新增 type**：400 与 405 同 type 不破坏「`type` 由状态码唯一决定」的不变式（状态码→type 仍是函数），也不必给 §7.2 加新分类。
- **405 发生在认证之前**：gin 的 `NoMethod` 是引擎级处理链（只含全局 Logger / recovery），受保护分组的路由也在这里分出 405，未登录请求同样能拿到 `Allow`。

### 实测到的边界（记下来免得下次重查）

- **HEAD**：gin 没注册任何 HEAD 路由，`HandleMethodNotAllowed` 仍进 405 分支并把该路径的其它方法列进 `Allow`；按 HTTP 语义 HEAD 响应体必须为空，所以它拿到的是 405 + 空体而**不是**错误信封（通用信封断言对它不适用，故单独成例）。
- **OPTIONS**：与 HEAD 同理按 405 + `Allow` 处理（`/api/v1/auth/login` 得 `Allow: POST`）。

### 已知缺口

- **跨域预检未开路**：将来若给浏览器跨域调用，`OPTIONS` 必须先绕开 405（预检会被现在的分流打回），届时 CORS 中间件要么注册 `OPTIONS` 要么在 `NoMethod` 之前放行。本轮前端仍无任何对 `/api/v1` 的调用（全仓 grep 无 fetch 与后端地址常量），不构成现行回归。（2026-10-12 更正：该前提已失效——`frontend/lib/api.ts`、`lib/auth-session.ts`、`next.config.ts` 的 rewrites 与 `proxy.ts` 都已存在并调用 `/api/v1`。）
- **HEAD 探活拿不到 2xx**：监控若用 HEAD 打 `/healthy` 仍是 405（改前是 404，同样不是本轮引入的回归），探活必须用 GET。

---

## 2026-10-12 · 两步验证接口轮（TOTP + 恢复码 + 登录分叉 + 重放防护）

**版本**：0.9.0 → **0.10.0**（minor：新能力域 + 5 条接口）

### 做了什么

- 5 条接口全部落地：`POST /auth/2fa/setup` / `enable` / `disable` / `recovery-codes`（守卫内）+ `POST /auth/2fa/verify`（公开，靠「待二段」会话）；`GET /auth/me` 增加 `two_factor_enabled`。路由 15 → **20 条**。
- **登录二段分叉**：bcrypt 通过后只看 `totp_enabled_at`——已启用则 `RenewToken` → 清除 `auth.user_id` → 写 `auth.pending_2fa_user_id` → 401 `two_factor_required`。「设置中」不拦登录，待验会话打受保护接口必 401（两态不并存）。`guard` 包零改动。
- **重放防护兑现**（用户裁定加列）：新迁移 `totp_last_counter`（可空 bigint、无默认、零回填）；命中窗号必须大于存量，且靠单条条件 UPDATE「影响行数 = 1」原子认领；恢复码消费同样单条条件 UPDATE（`used_at IS NULL`），并发同码恰一人成功。
- **secret 加密**：AES-256-GCM（12 字节随机 nonce 前置 + 整体 base64），密钥走新环境变量 `TOTP_ENCRYPTION_KEY`（base64、解码后恰 32 字节，缺失/非法启动即失败）。校验点 `loadTOTPKey()` 钉在 `Load()` 末尾，因此既有 8 个「断言错误里含某变量名」的负例零改动，只有 7 个要求 `Load()` 成功的用例补了合法 key。
- **enable / disable 吊销其它会话**（用户裁定）：复用改密在用的 `revokeOtherSessions`，且排在落库之前——失败即 500 且库里不变，不留「已启用但没领到恢复码、重试又撞 409」的死角。
- **半程失败收进单事务**：enable（启用时间 + 插 10 码）、disable（清三列 + 删全部码行）、recovery-codes（删旧 + 插新）各一条 `client.Tx`（照 access/settings 先例）；setup 覆盖 secret 时一并清窗号。
- **解密失败不再伪装成验证码错误**：`openSecret` 失败、已启用但 secret 为 NULL 的损坏态、`matchTOTPCounter` 因 secret 非法返回 error——三种一律 500 `internal_error`，不消耗限次、不销毁会话、不认领窗号；把列修回密文后同一枚码仍 200（有用例钉住）。
- `apierror` 新增 6 个码（三个 401 `authentication_error` + 三个 409 `conflict_error`），注册表 20 → **26 条**（`TestRegistryIntegrity` 同步）。
- 测试：config 3 例 + secretbox 5 例 + totp 原语 7 例 + repository 并发改判 3 例 + 2FA 生命周期集成 18 例（含双 jar 吊销、同窗重放、连错 5 次锁定、密文落库反解）。集成用例**零 `sleep`**：时间只经包内 `totpNow` 一处取，测试用 `pinWindow` 把窗号确定性前移。
- 文档：README（环境变量表、迁移清单、目录对照表 auth 行、认证接口表 +5 行、两步验证章节整段重写、已知缺口并入 verify）；API 规范 §3 端点表 +5 行、§7.3 注册表 +6 行并订正 `invalid_credentials` 的场景列（改密与 2FA 密码确认都在用，`current_password_mismatch` 拆分按 §0 v1.3 仍未实施）。

### 关键决定

- **待验态走 scs 独立会话键**，不加中间件、不改守卫：`RequireAuth` 仍只认 `auth.user_id`，待验会话天然拿不到权限。
- **认领与后续会话写不在同一事务**：认领成功而会话写失败会废掉一个窗，重试要下一窗——换取「不因会话存储抖动而回滚已验证的凭据」，代价记进已知缺口。
- **恢复码再生成不吊销会话**：要密码且已登录，旧码作废本身不扩大攻击面；enable/disable 才吊销（动机是「怀疑密码泄露才开 2FA」）。
- **限次只是会话内闸门**：每轮登录 5 次、重登续杯，不构成在线暴破防御；真正的闸门在登录限流轮，届时 login 与 verify 同做。

### 执行中的口径修正（与计划/上一轮的出入，逐条实测后定）

- **auth 模块构造器由 `New(db, sessions)` 变 `New(db, sessions, totpKey)`**：推翻 2FA schema 轮「该模块不需要配置」的旧记录——密钥是配置项，必须经 `app.go` 显式注入（README 目录表与 spec §6 同步）。
- `pquerna/otp` 的引入落在**第一个 import 它的提交**（TOTP 原语那条），不在依赖引入那条：`go mod tidy` 会把尚无人引用的依赖从 go.mod 剔回去，先 `go get` 再 tidy 等于白加。
- 新增错误码必须连 `apierror_test.go` 的注册表计数一起改（20 → 26），否则门禁红；计划该任务的文件清单漏了这个文件。
- 「disable 吊销其它会话」的集成用例从 Task 5 挪到本轮：Task 5 时二段登录还不存在，第二个会话只能用「未启用期的单段登录」造出来，那种写法到本轮必然红。
- 测试接缝 `codeAt` 带 `t *testing.T`（计划写的是不带的形状），取码失败要 `t.Fatalf`，不该向上层返回 error 让用例各自处理。
- 请求 DTO 用 `TwoFactorCodeRequest` 一个类型（enable 与 verify 的 body 同为 `{"code"}`），不留两个同名同形的结构。

### 已知缺口

- **`TOTP_ENCRYPTION_KEY` 是部署面新增强依赖**：缺失启动即失败；密钥丢失 = 已启用用户验码一律 500，只能 DB 清列重置（`.env.example` 与 README 都写了备份要求）。恢复码在密钥丢失时同样救不了——解密发生在恢复码分支之前。
- **重放防护的代价**：一枚码用过之后它所在窗整窗作废；用户在同一 30 秒内登出再登录需等下一窗，**启用那一刻也占掉当窗**（RFC 6238 §5.2 标准取舍）。
- **认领不回滚**：verify 的计数认领与会话写非同事务，会话写失败该窗已废；enable 的认领与落位分两步，落位事务失败同样废一窗。只影响一次重试体验，不产生权限或数据不一致。
- **限次续杯**：见上，暴破闸门等登录限流轮。
- **disable 删全部恢复码行**（含已消费的）：不留存根，无法统计用户消费过几个码；需审计得改成标记作废。
- **「吊销失败 → 整个动作不生效」这条分支本轮未构造测试**：要造会话存储故障才触发，代价大于收益，只靠调用顺序本身保证（改密轮的同类用例可参照）。
- **本机 `-race` 仍不可用**（`CGO_ENABLED=1` 缺 gcc）：并发改判的正确性依据是「单条条件 UPDATE + 判断影响行数」在 Postgres 侧的行锁语义，两个并发用例各断言「恰一人成功」；不是靠 race 检测器证明的。

---

## 2026-10-11 · 管理端发信接口（邮件模块升级为路由模块）

**版本**：0.8.0 → **0.9.0**（minor：新 HTTP 能力）

### 做了什么

- 邮件模块获得第一个路由 `POST /api/v1/admin/mail/messages`（创建邮件消息即发送）：body `{"to"}` → 用当前 SMTP 配置**同步真发**一封 `example` 自检邮件 → 201 `{"to","subject","sent":true}`；400 `invalid_request`（`param:"to"`，`net/mail.ParseAddress`，支持 `Name <addr>` 取纯地址）/ 401 `unauthorized` / 403 `forbidden` / 500 `mail_delivery_failed`。主题与正文由服务器控制（`example` 模板），请求方不能指定内容。
- 模块形状统一（兑现邮件 spec v4 时期「若以后要形式统一」的预案）：`mail.New(cfg)` 改返回 `*Module`（`m.Mailer` 暴露能力接口，app.go 装配处同步）；新增 `module.go` / `controller.go`，硬性规则 1、5 的「无路由模块」清单只剩 `settings`。
- `Mailer` 接口新增 `SendSync(ctx, msg) error`：同步单次尝试、不重试、拨号死线 20s（go-mail 逐命令续期，非总上限；`Close` 后同样返回 `ErrClosed`），失败记结构化日志；异步 `Send`（队列 + 3 次重试）语义不变，将来业务流程一律走它。
- 守卫新增 `guard.RequirePresetGroup(keys...)`（预设组白名单，owner/admin 过、其余 403 `forbidden`、无 identity 500）：路由链 `RequireAuth → LoadIdentity → RequirePresetGroup(owner, admin)`。**刻意不走权限目录**——SMTP 是服务器级配置（环境变量），不属于可授出的业务权限域，自建组不可授；将来邮件配置若搬进设置系统，可再切权限点。
- `apierror` 新增哨兵 `ErrMailDeliveryFailed`（`server_error` / 500，错误码注册表第 20 条）。
- 测试：guard 预设组守卫单测（owner 过 / 普通用户与自建组 403 / 未接线 500）、`SendSync` 单测（成功一次调用 / 失败不重试 / `Close` 后 `ErrClosed` / 失败日志）、controller HTTP 用例（201 + 内容断言 / 展示名地址提取 / 三种 400 / 两种 403 / 500 映射）；`TestRegistryIntegrity` 同步 20 条。
- 文档：README 目录对照表（mail 行、guard 行）、硬性规则 1 / 5、邮件章节重写（接口表 + SendSync 语义）；邮件 spec v5 修订记录。

### 关键决定

- **路由命名**：初稿 `POST /admin/mail/test` 被用户否决（不要动词尾巴、要求严格按既有 API 规范、不自造轮子）。按 API 规范 §3 资源名词复数，三选一由用户点名 `POST /admin/mail/messages`（对标 OpenAI「操作即资源」，创建即发送；`POST /<集合>/:id/<动作>` 子动作形态本项目仍无先例）。
- **同步而非入队**：自检接口的意义在于即时告知 SMTP 通不通；异步队列只能回「已入队」，对排障无用。`SendSync` 单次不重试（最多等 20s），失败由操作者自行重试。
- **权限用预设组守卫而非权限点**：权限目录动作只有 `view`/`edit`，`mail.test` 之类的第三动作会破坏既有命名规则；`RequirePresetGroup` 不动目录、不加戏。

### 已知缺口

- 无限流（与登录/注册同属已知缺口，等统一补）。
- 发信成功 ≠ 对方收件箱收到（SPF/DKIM/DMARC 与服务商合规仍见邮件 spec §11）。
- `SendSync` 与队列 worker 可能并发各起一条 SMTP 连接（自检请求与后台投递同时发生时）；对 SMTP 服务商的并发连接数无压力，暂不互斥。
- **20s 不是总上限**：go-mail 只在拨号阶段吃 ctx 死线（`client.go:1197`），之后对每条 SMTP 命令续期（`client.go:1237/1811`），慢速对端总耗时可超 20s（异步 `Send` 同构，既有行为）。
- `SendSync` 死线 20s 大于关停预算 10s：SIGTERM 撞上在途自检请求时 `server.Shutdown` 带 DeadlineExceeded 返回、SMTP 事务被硬切（`writeTimeout` 60s 不构成额外问题）。

### 终审

独立子代理整分支终审（fresh context）：**0 Critical / 4 Important / 8 Minor**，门禁全绿（`go test -count=1` 强制非缓存）、无越界文件、异步 `Send` 零改动、`RequirePresetGroup` 的身份来源链（自建组无法携带预设 `system_key`）源码取证通过。

- **已修（按用户裁定）**：
  1. **I-1 状态码违规**——`mail_delivery_failed` 初版 502 违反 API 规范 §4「500 之外的 5xx 一律归并为 500」的白名单；用户裁定**改回 500**（语义区分交给 code，不动冻结规范）。I-3（README:126 type 枚举缺 502）随本条自动消解。
  2. **I-2 词汇表漏登记**——API 规范 §7.3 补 `mail_delivery_failed` 行（500 / `server_error` / param null）+ 引言记账。
  3. **I-4 spec 正文未随修订**——邮件 spec §2 决策表 / §5 接口块（`SendSync` + `New` 签名）/ §6 装配与消费方四处按 v5 改写，修订记录不再「只记账不修文」。
  4. **M-1 超时措辞失实**——README 与 spec 的「20s 超时」改为「拨号 20s 死线、逐命令续期、非总上限」（go-mail 源码取证）。
  5. **M-2 ctx 断言缺失**——补 `TestSendSyncAppliesDeadline`（死线 ≈20s）与 `TestSendSyncPropagatesParentCancellation`（父取消传导到 transport）。
  6. **M-4 关停预算**——已知缺口补 `SendSync` 20s > `shutdownTimeout` 10s 一条。
  7. **M-5 同码不设防**——`TestRegistryIntegrity` 加「code 去重后仍 20 条」断言（两个哨兵共用 code 原本可静默通过）。
  8. **M-6 集成腿缺失**——补 `TestMailMessagesEndpointEndToEnd`：匿名 401 / 普通用户 403 / 自建组 403 / owner 201（走真实 app 装配 + 登录会话，`DATABASE_URL_TEST` 缺省 skip）。
- **留档不修（用户裁定）**：M-3（`ErrClosed` 单列错误码——其建议的 503 本身违反 §4 白名单，503 仅 `/healthy` 豁免；且该窗口实际不可达）；M-7（「操作即资源」判定标准待出现第二个此类端点时写入规范 §3/§4）；M-8（未跟踪的 2FA 两份文档写着 0.8.0 → 0.9.0，本轮已占用 0.9.0，2FA 轮启动时需重编版本号）。
- **审查手段缺陷记录**：本机 `-race` 不可用（`CGO_ENABLED=1` 缺 gcc），数据竞争结论以 go-mail v0.8.1 源码取证替代（每次投递新建连接 + `c.mutex.RLock()`）；`gofmt -l` 恒退出 0、`-race` 首跑退出码被管道吞掉——两处都要求门禁脚本用显式判定，不信工具默认行为。
- **过程中撞出的存量 flake（非本轮引入；2026-10-12 用户点名后已修）**：全仓并行跑时 auth 一次 `TestLoginRejectsWrongPasswordAndUnknownAccount` 报 `email or nickname already exists`——两包共用 `neoblog_test` 并行执行，而 `auth/repository_test.go` 与 `access/migration_test.go` 的 tag 生成器同为纯 `UnixNano`、建号昵称都派生自它（如 `"nick"+tag`），取到同一时钟刻度即撞唯一约束（当时单独重跑与后续两轮全仓 `-count=1` 均绿，属偶发）。修法：tag 改为「pid + `-` + UnixNano」，跨进程必不同；两包各加守护用例 `TestUniqueTagIncludesProcessID`（先红后绿）。修复后全仓 `-count=1`（带库）重跑绿。

---

## 2026-10-11 · 设置组存储机制（内部能力，无路由）

**版本**：0.7.0 → **0.8.0**（minor：新能力域）

### 做了什么

- spec v2 全落：`setting_values` 单表（复合唯一 `(setting_group, name)` + jsonb `value`，纯 DDL 零数据，全仓第一列 `field.JSON`）；模块 `internal/modules/settings/`（types / registry / repository / service 四文件，无路由无 controller / dto）；三种类型读写按类型分方法（用错即 `ErrMethodMismatch`）、整组读逐项合并坏行只回退该项、整组写单事务零落库、删项恢复默认（幂等只指"清单内但库里没有这行"）；清单定义在代码内（初始为空），构造自检不过启动失败。
- `settings.New(db) (*Service, error)` 是仓库第一个返回 error 的模块构造器；`app.New` 随之改 `(*App, error)`，三处调用点同步（`main.go` 拆链式调用、`app_test.go` 两处判错）。
- upsert 冲突分支显式 `SetUpdatedAt(time.Now())`（不赌 ent 的 `UpdateDefault` 是否作用于 `ON CONFLICT` 路径），集成用例⑧断言同项二次写入后时间变大。
- 测试：清单自检与配方防漂移（单测，无库）+ 集成①–⑧（`DATABASE_URL_TEST` 缺省 skip）；README 四处同步（目录对照表加行、硬性规则 1 / 5 措辞、迁移清单、「设置」章节 + 加设置配方——配方防漂移测试 `TestRegistryRecipeShape` 把关）。

### 过程中的坑

- **jsonb 归一化**：`1e3` 落库即被 Postgres 存成 `1000`，读取侧永远见不到指数记法原文；数字边界的真身（`3.5` / 超 int64）照拒，`1e3` 的拒绝由内核 `TestDecodeNumberBoundary`（`UseNumber` + `ParseInt`）在原始 JSON 文本上钉死。
- `.env` 直接 `source` 只设 shell 变量、不导出，`go test` 子进程看不到 `DATABASE_URL_TEST`——要 `set -a; source .env; set +a`。
- `gofmt -l` 退出码恒为 0（列出问题文件也不报错），门禁必须用 `test -z "$(gofmt -l .)"` 判定。

### 终审

独立子代理整分支终审（fresh context）：**0 Critical / 2 Important / 4 Minor**，Review Focus 五条逐条实测通过；build / vet / gofmt / 全仓测试 11 包全绿。

- 已修（每个都先红后绿）：
  1. **jsonb `null` 坏行对 string / bool 静默穿透**——`decodeTyped` 原来直接 `Unmarshal` 到零值（`null` 解入 string / bool 无错无日志，返回零值而非默认值）；改为三种类型统一 `UseNumber` + type switch，`null` 全部判坏行回退默认。新增 `TestDecodeTypedRejectsNull` 与 null 坏行集成断言（string / bool 各一）。
  2. **Task 1 提交混入范围外文件**——`git add -A` 卷走了会话开始前就存在的 `frontend/app/page.tsx` 改动与四份 content / mail 存档文档，backend feat 提交历史语义失真。历史不重写，本条即补记；**防再犯：分轮提交按文件清单暂存，不裸 `git add -A`**。
- 延后小项（待定）：`ConflictColumns` 用字面列名（接口轮映射 apierror 时顺带换生成常量）；集成测试用固定组名而非 spec 字面的"随机后缀"（隔离效果等价）；`marshalJSON` 吞 Marshal 错误（当前三种类型安全，将来加类型时回看）；`newServiceWith(nil)` 触库会 panic（已限定为不触库单测路径）。

### 已知缺口

- 清单为空、无消费方：模块上线即休眠（与 2FA 轮同性质）；首次真实使用（加组 + 消费方）只改代码不动库。
- 孤儿行：清单删项后旧行滞留并占着复合键，需人工 SQL 清理（spec §9）。
- 密钥类加密、接口 / 公开读取、导入导出：见 spec §10 插槽。

---

## 2026-10-11 · 两步验证表结构预留（TOTP，仅落库）

**版本**：0.5.1 → **0.6.0**（minor：新能力域的数据插槽）

### 做了什么

- 兑现认证设计 §2 / §11 的预留（TOTP + 自管恢复码，库选 `pquerna/otp` 但本轮不引入）：`users` 加 `totp_secret` / `totp_enabled_at` 两个可空列（ent Optional + Nillable，组合出「未设置 / 设置中 / 已启用」三态）、新表 `recovery_codes`（`user_id` 级联外键 + `code_hash` + `used_at` 一次性标记）。接口、登录分叉、恢复码生成逻辑全部下一轮；当前无用户启用、无任何写入路径，上线即现状。
- 迁移 `totp_and_recovery_codes`（纯 DDL：加两列 + 建表 + 索引，无回填无 seed），人工审阅后 hash / validate、应用到 `neoblog` 与 `neoblog_test`；`db-migrate-diff.sh` 的迁移名字面量再换一次（仍是写死字面量、未参数化）。
- 迁移后置断言 `TestTOTPSchemaReserved`（auth 模块，`DATABASE_URL_TEST` 缺省 skip）：可空性、Nillable 读写回程、恢复码行随用户删除级联消失、`used_at` 默认 nil——全部用 ent 行为实证，不查 `information_schema`。

### 过程中的坑

- ent 命名转换不保留缩写词大写：`totp_secret` 生成的 Go 字段是 `TotpSecret`（不是 `TOTPSecret`）；计划原写法有误，以生成物为准。
- auth 包测试 helper 已有同签名 `testDB` / `uniqueTag`（`repository_test.go`），新测试文件重复声明直接编译失败——删重复声明、复用既有 helper。

### 终审

独立子代理整分支终审（fresh context）：**0 Critical / 1 Important / 4 Minor**，Review Focus 五条（级联方向、Nillable、迁移无 NOT NULL/回填、索引、两库一致）与 spec §10 五条验收逐条实测通过；go.mod/go.sum 确认未动，业务模块 grep `totp|recovery` 零匹配。

- **已修**：README 已引用本轮 spec 路径而 plan/spec 两份文档此前未入库（悬空引用、违背前四轮存档惯例）——两份文档随本提交补交存档；spec §10.5 的既有路由计数同步更正（auth 5 + access 8 = 13 条 + healthy）。
- **延后小项（待定）**：迁移后置断言可选加固——`used_at` 改为从 DB 重查后再断言 nil、`TotpEnabledAt` 补值相等校验（当前由迁移 SQL 人工审阅 + `atlas migrate validate` 把关）。

### 已知缺口

- `recovery_codes` 在下一轮接口落地前是死表；`code_hash` 暂不设长度校验（写入方下一轮实现时钉 64 位 hex）。
- secret 按 base32 明文入列：DB 泄露即可伪造 TOTP。休眠期无存量，应用层加密可在任何用户启用前无痛补上（这正是先落结构、暂不启用的动机）。
- 登录限流仍未做（用户裁定与本轮分开，见认证章节已知缺口）。

### 关键决定

- 先落库表结构、暂不启用：插槽钉死后，下一轮接口不动迁移；secret 加密升级窗口留在无人启用阶段。
- 恢复码哈希用 sha256（高熵随机串无需慢哈希）；将来换低熵可读码时改算法不动表。

## 2026-10-11 · 邮件模块归位审查与修复

**版本**：0.5.0 → **0.5.1**（patch：测试基础设施缺陷修复 + 规范说明补全）

### 背景

邮件模块初版放在 `internal/mail/`（被当成与 `config` / `database` 平级的基础设施），用户指出它违背 README 硬性规则 1（一个功能一个模块目录）。归位提交 `56a8037`（`internal/mail` → `internal/modules/mail`，11 个文件 `R100`）后，用户要求派独立子代理**只读**复查。

### 过程（审查 → 验证 → 修复，可追溯）

1. **两路独立只读子代理审查**，角度错开：迁移完整性与引用面（逐字节比对 blob、残留引用、构建测试、git 历史完整性）／规范符合度与文档一致性（README 规则 1–8 逐条、三处文档互相一致、sed 批量替换残留）。两路**独立报出同一条真实缺陷**（下条），并另有规范说明与文档口径若干项。
2. **逐条实测验证**（不信转述）：`cd internal/modules/mail && test -f ../../.env` → 不存在（归位后指向 `backend/internal/.env`）；`test -f ../../../.env` → 存在（`backend/.env`）；`LoadDotEnvFile` 对缺文件静默返回 nil（`dotenv.go:16-19`）；README 规则 1/5 原文与 mail 实际形态逐条对照；spec/plan 里旧版本号（0.2.1 → 0.3.0）与旧路径描述逐条确认。**四条发现全部属实**。
3. **TDD 修复**：先写三个失败用例（`findEnvFile` 纯函数边界 / 「从包目录能找到 `backend/.env`」守护用例 / 经 `t.Chdir` 到临时目录树验证 `.env` 确实被载入到进程环境），跑出编译失败 → 实现「从包目录向上逐级查找 `.env`（深度上限 5）」并接入真实投递测试 → 全绿。守护用例专门钉住「包再移动导致静默失效」这一类回归；`TestRealDelivery` 的 skip 文案也点明了查找位置。
4. **文档同步**：spec 增 v4 修订记录 + §10 第 5 条改写 + §3/§14 版本表述修正；plan 增「实施后修订 2」+ Task 4 Step 5 改写；README 补规则 1/5/7 的「无路由模块」与「禁止自建 SMTP 客户端」说明、目录表 mail 行改写并移入模块行区域、「邮件」章节措辞与测试说明；本条目即过程记录。

### 缺陷根因

`realdelivery_test.go` 里写死的 `../../.env` 依赖包目录深度：归位只改了文件位置与含 `internal/mail` 字样的文本，这条不含路径 token 的相对深度没有跟着变；`LoadDotEnvFile` 对缺失文件静默返回 nil 又把问题藏住——用户按 README 填好 `backend/.env` 去跑真实投递测试时只会得到 SKIP，且没有任何提示。

### 关键决定

- **不写死相对深度**，改为有界向上查找（`findEnvFile` 深度上限 5）+ 守护用例：包位置再变会红，而不是静默失效。
- README 规则 5 采用「文档收编现状」方案（无路由模块由 `New` 直接返回能力接口、无状态包级纯函数可直接导出），而不是把 `mail.New` 包装成 `*Module`；若以后要形式统一，需同步改 spec/plan 与 `app.go`。

### 已知缺口

- 仅当变量已在进程环境导出或 `backend/.env` 存在且可读时，真实投递用例才会真跑；无凭据时按约定 SKIP。
- 邮件 spec/plan 仍是未跟踪文件（未入库），本次修订只存在于工作区。

---

## 2026-10-11 · 用户组与权限（层 2）

**版本**：0.4.1 → **0.5.0**（minor；认证层 2 与邮件基础设施先占用了 0.3.x / 0.4.x）
**提交**：`bbeef91`（rbac）→ `a3e5474`（版本进位）；终审修复 `f48568c`；设计与计划存档 `6088bf5`

### 做了什么

- 权限目录 `internal/common/rbac`（纯 Go，不依赖 gin / DB）：`模块.动作` 命名、动作只有 `view` / `edit`（`edit` 蕴含 `view`）、四个权限点（`system.users.view|edit`、`system.groups.view|edit`）、预设组常量（owner / admin / 普通用户 + 固定 UUID）。
- 新模块 `internal/modules/access/`：组 CRUD、用户管理接口（列表 / 详情 / 设组 / 设额外权限）、生效权限解析；路由全部挂在权限守卫下。
- 数据模型：新增 `groups` / `group_permissions` / `user_permissions` 三张表 + `users.group_id`（非空外键，ent 字段级默认值指向「普通用户」，**创建路径零改动**）；迁移里 seed 三个预设组、三步回填存量用户（生成物把 `ADD COLUMN … NOT NULL` 与外键写在同一句，人工拆开）。
- 守卫扩展：`guard.LoadIdentity`（解析一次身份与生效权限放进 `auth.identity`，用户已删除则销毁会话 + 401）与 `guard.RequirePermission`（权限不足 403 `forbidden`）。
- `/auth/me` 扩展为带 `group` 与 `permissions`；`app.go` 装配 access 模块与 `RequireAuth → LoadIdentity → RequirePermission` 链。
- `scripts/db-generate.sh` 开 `--feature sql/lock`（last-owner 检查用 `FOR UPDATE`）；`db-migrate-diff.sh` 的迁移名由 `init_users_sessions` 换成 `groups_and_permissions`（仍是写死字面量，未参数化）。
- 测试：rbac / guard 单测（不连库）+ 迁移后置断言 / service / HTTP / 端到端集成（`DATABASE_URL_TEST` 缺省 skip）。

### 制定阶段修掉的坑（设计 → 计划）

设计前后各派过独立子代理：先摸清参照项目（D 盘 HaoBlog）的模型与其生态代价，再把写好的计划交给两路只读子代理——一路查「照做会不会撞墙」（逐条核对 ent / Atlas / scs 行为），一路做安全红队（穷举提权路径与覆盖缺口）。真正影响实现的几条：

- **组转移的授权差集没钉死**（红队判高）：原计划只写「只能授出自己持有的权限」，差集口径留白——若实现成「只对额外权限做差集」，两步即可提权（先把自己有的权限塞进一个组、把用户塞进去，再借那个用户反向给自己加权限）。现设计 §6 规则 1 写死：`next 生效集 − current 生效集`、同一请求内组与额外权限的变更**合并计算**、两侧先做蕴含展开。
- **`last_owner` 串行路径根本打不出来**：自己不能改自己、非 owner 碰不到 owner 组成员 ⇒ 串行下 owner 数不可能降到 0；原计划却让 HTTP 用例串行断言 409，会诱导实现把检查提前（把 403 变成 409）。现规定检查顺序 `cannot_modify_self` → `owner_protected` → `last_owner`，409 只留给并发用例。
- **版本口径与仓库现状脱节**：计划初稿写死 0.2.1 → 0.3.0（该原稿未随存档保留、无从核对；存档版写为「当前 0.3.1 → 0.4.0」并已带「若届时已被其他层进位按当前版本顺延」规则），实际执行时已被认证层 2 与邮件层推到 0.4.1。
- **迁移脚本的隐性前置**：`db-migrate-apply.sh` 带 `set -u` 且不读 `.env`，计划漏了导出 `DATABASE_URL` / `DATABASE_URL_TEST`，照做必报 unbound variable。
- 其余中级项：约束错误不能一刀切映射（组名冲突与外键冲突分开）、guard 单测里「会话已销毁」的断言是空转（第二次仍 401 只能证明 resolver 仍返回 not-found，改为断言清除 cookie）、输入校验整段缺失（组名长度 / 空 body / `group_id: ""` / `[]` vs `null`）、`/auth/me` 扩展与 app 装配拆开会留下红灯窗口（合并为一个任务）。

### 执行中发现并修掉的问题

- **ent 边声明的写法**：计划写的是 `edge.To("users", User.Type())`，生成器直接报 `not enough arguments in call to User.Type, want (User)`。回 ent v0.14.6 源码核实：`Schema.Type` 是边声明专用伪方法，取方法表达式、**不加括号**（`ent.go:27-41`），改为 `User.Type` 后生成通过。
- **迁移生成物在有数据的库上必失败**：`atlas migrate diff` 把 `ALTER TABLE users ADD COLUMN group_id uuid NOT NULL` 与外键写在同一条语句；人工拆成「加可空列 → `UPDATE` 回填「普通用户」→ `SET NOT NULL` → 加外键」，预设组 seed 落在 `CREATE TABLE groups` 之后，再 `atlas migrate hash` + `validate`（13 条语句应用到两库）。
- **三处测试期望写错，被实现的正确行为纠正**（改测试不改代码）：① 目标已在 admin 组时 `owner_protected` 优先于 `grant_forbidden`；② 「应该被拒」的用例里给操作者配多了权限，其实合法；③ 断言普通用户组有成员却没造人。
- **脚手架坑**：`task-done` 脚本在验证命令零输出时不记账（`grep -v` 空输入退出 1 + `set -e` 静默中止），后续命令统一追加哨兵输出。

### 终审与修复

独立子代理整分支终审（fresh context；明确排除并行会话的邮件层提交）：**0 Critical / 2 Important / 6 Minor**，5 条预设审查焦点（组转移差集、`last_owner` 顺序与可达性、owner 保护两向、`edit` 蕴含 `view`、并集撤权假象）逐条通过，三条攻击者视角未发现接口组合提权路径。

- **已修（先复现后修）**：并发 `PATCH` 同一个组或同一个用户时，替换是「先删后插」，READ COMMITTED 下第二笔看不到第一笔未提交的行，随后 `INSERT` 撞唯一索引 → **裸 500**。先写 `TestConcurrentReplaceKeepsBothWritesServing` 复现（组 / 用户两条路径各 5 轮并发，3 次运行里稳定打到 500），再在替换事务内先 `FOR UPDATE` 锁父行（组 / 用户）串行化 → 15 轮并发全 200、全量套件绿。根因：替换事务之间没有可串行化的锚点。
- **裁定不修**：owner 保护判定用的是**请求开始时的快照**——并发把目标提升进 owner/admin 组的瞬间，非 owner 可能刚好完成一次对目标的额外权限编辑。不产生越权（可授出集合仍受差集限制）、不损坏数据，与设计 §13 已写明的「在途请求用旧快照」同属一种语义；要堵得把整段判定搬进事务，收益不抵复杂度。
- **未处理的小问题（待定）**：README「新用户由数据库默认值落普通用户」措辞（实为 ent 字段级默认值；后已修正）；删空组后权限行级联消失未断言；用户列表「按创建时间倒序」的断言是空转；`rbac.Expand` 对目录外的 `x.edit` 也会合成 `x.view`（仅脏数据可触发）；约束映射一刀切（未来新增约束需同步）；`/auth/me` 在 `LoadIdentity` 之后多查一次用户。

### 验证

- `gofmt -l` / `go vet` / `go build` 干净，`atlas migrate validate` 通过；带库与不带库各跑一次 `go test -p 1 ./...` 全绿（access 集成用例依赖 `DATABASE_URL_TEST`，缺省 skip；**带库全量用 `-p 1`**——access 与 app 都会造 owner，包级并行会互相干扰）。
- 手工 curl 清单：普通用户 403、owner 200、`/auth/me` 带 group 与 permissions、预设组改动 403 `preset_group_locked`、重名 409 `group_name_taken`、非空组删除 409 `group_not_empty`、改自己 403 `cannot_modify_self`、目录外权限 key 400 `invalid_permission`、删空组 204、用户被删后旧 cookie 401。
- 迁移后置断言（`TestMigrationSeed`）：三预设组按固定 UUID / key / 名称存在、权限行数为 0、**不显式设组建用户自动落「普通用户」**（ent 字段级默认值在 v0.14.6 生效的实证）。

### 关键决定

- **预设组权限由代码计算、不入库**（owner / admin = 全量、普通用户 = 空），只有自建组写 `group_permissions`——参照项目（D 盘 HaoBlog）因为「权限行入库 + 首建号一次性 seed」导致新增权限要手工补种、漏种即全站锁死（权限从库里读，fail-closed），这里从根上绕开：加权限点只改代码。
- 生效权限 = 所属组 ∪ 用户额外权限（都做蕴含展开）；**只能授出自己持有的权限**（按目标生效权限集的前后差集、同一请求合并计算）、**摘除不受限**、**不能改自己**、**owner / admin 组的成员只有 owner 能改**、**至少保留一个 owner**（409 `last_owner` 只在并发双降级时可达，串行路径被前两条挡住）。
- 组以 UUID 标识、组名唯一（不做 slug）；带成员的自建组不可删；预设组不可删 / 改名 / 改权限。
- 组名 trim 后 1–64 字符；读接口一律返回展开后的权限集，写接口收原始勾选。
- 守卫原则变更：从「不查库」改为「只依赖 `app.go` 注入进来的 resolver，不 import 业务模块」。
- 权限改动**下一次请求即生效**（不吊销会话）；在途请求使用请求开始时的权限快照。

### 已知缺口

- **首个 owner 只能手工指定**（`UPDATE users SET group_id = '00000000-0000-4000-8000-000000000001' WHERE email = '…'`）；「首次建号入口」仍待定。
- 登录 / 注册仍无限流；角色与权限操作无审计留痕；用户列表不分页、不做可见性遮蔽；组编辑是「后写者获胜」（无乐观锁）。
- 权限目录删除权限点会留下 DB 孤儿行（本轮不清理）。

---

## 2026-10-11 · 邮件基础设施（只做后端）

**版本**：0.3.1 → **0.4.1**（minor 0.4.0 落地；模块归位 patch 0.4.1，归位审查修复见上一条）

### 做了什么

- 新增模块 `internal/modules/mail/`（无路由；对外入口 `New(cfg) Mailer`，不含 controller / dto）：`Mailer` 接口（`Send` / `Close`，`Close` 后 `Send` 返回哨兵 `ErrClosed`）+ `Message` / `Body` + `Render(name, data)`；业务代码不 import go-mail。
- 发送链路：缓冲队列（32）+ 单 worker 串行投递；每封最多 3 次尝试、退避 1s / 5s、单次 20s 超时；**4xx 与连接类错误重试、5xx 不重试**（判定用 `SendError.IsTemp()` / `ErrorCode()`——go-mail 把网络类失败判为「永久」，只信 `IsTemp()` 会一次抖动就丢信）；失败记 `log/slog` 结构化日志。
- 双实现：SMTP（`wneessen/go-mail` v0.8.1；`starttls` / `ssl` 两种 TLS 模式、无用户名则不做 AUTH、有用户名走 `SMTPAuthAutoDiscover`）与日志（**`SMTP_HOST` 为空即降级**，邮件全文写日志，开发环境无凭据可跑通全流程）。
- 模板：`templates/<name>.html|txt` 成对 + `embed.FS`；HTML 侧 `html/template` 自动转义；内置一封 `example`（邮件配置自检）。
- 配置：7 个 `SMTP_*`（`SMTP_HOST` 为空时全部跳过校验，其余 fail-fast，含端口 ≤ 65535）与 `LoadDotEnvFile(path)`（`LoadDotEnv` 变其包装，行为不变）。
- `app.go`：装配 `mail.New(cfg)`；`server` 提为字段并抽出 `shutdown(ctx)`（先 `server.Shutdown` 再 `mailer.Close`，共用 10s 关闭预算）。
- 文档：README（目录表 / 环境变量表 / 新增「邮件」章节 / 认证章节的「没有邮件基础设施」措辞更新）、`.env.example`（腾讯云 SES 为主示例）。

### 验证

- 单测（无外部依赖）：队列满阻塞、`Close` 后 `Send` 返回 `ErrClosed`、4xx 重试 / 5xx 不重试 / 重试耗尽记日志、drain、`Close` 幂等、并发 `Send`/`Close` 不 panic、日志实现内容与缺省 From。
- 集成（进程内 `emersion/go-smtp` + 自签证书，不依赖 Docker 与外网）：multipart 双版本正文、**中文主题 RFC 2047 可解**、550 永久 / 451 临时 / 连接被拒可重试。
- 真库：`TestShutdownClosesMailer`（关闭 drain 后 `Send` 返回 `ErrClosed`）；带与不带 `DATABASE_URL_TEST` 各跑一次全量 `go test ./...` 全绿；`gofmt -l` 与 `go vet` 干净。
- 可选真实投递 `TestRealDelivery`：无凭据时 SKIP；需在 `.env` 填好 `SMTP_*` 与 `MAIL_TEST_RECIPIENT` 后手动跑。

### 关键决定

- 选型 `wneessen/go-mail` v0.8.1（依赖仅 x/crypto + x/text）；排除：`gomail.v2` 停在 2016、`jordan-wright/email` 停在 2021、标准库 `net/smtp` 已 frozen 且无 context。
- **不做持久化 outbox**：Gitea / Vikunja 的生产实现都是「异步即发、失败只记日志」，本项目量级用「重发」兜底；触发条件＝出现「邮件丢了不可恢复」的流程。
- 不引入 `riverqueue/river`：`InsertTx` 需 `*sql.Tx`，而 ent 的 `*ent.Tx` 不暴露底层事务，原子入队要绕非公开 API。
- 不做应用内 DKIM（域验类服务商侧签）、不做连接复用（每封 Dial + Close）、不做退信处理。
- 包位置：按 README 硬性规则 1 归入 `internal/modules/mail/`（初版误放在 `internal/mail/`，用户裁定后迁移，版本进位 0.4.1）。

### 已知缺口

- `internal/modules/mail` **没有任何消费者**（注册不验证邮箱）；第一个消费方出现时在 `app.go` 注入。
- 内存队列无持久化：进程崩溃丢在途邮件；`Close` 返回错误 ≠ worker 已停（退避不响应关闭，上界约 66s）。
- 无退信 / 投诉处理与抑制表、无连接复用、无应用内 DKIM、模板无多语言——触发时点见 `docs/superpowers/specs/2026-10-11-neoblog-mail-design.md` §12、§15。

---

## 2026-10-11 · 认证层 2（注册与更改密码）

**版本**：0.2.1 → **0.3.0**（minor）

### 做了什么

- 新接口（前缀 `/api/v1`）：
  - `POST /auth/register`（公开，注册即登录）→ 201 + 会话 cookie；校验与注册页 UI 对齐（邮箱小写入库、昵称 trim 后 2–32 字符且禁空白与 `@`、密码 ≥8 字符含 ASCII 字母与数字且 ≤72 字节）；错误码 400 `invalid_request` / `weak_password`、409 `email_taken` / `nickname_taken` / `already_taken`（并发竞态兜底）。
  - `POST /auth/password`（需登录）→ **200 `{}`**（2026-10-12 对账更正：原文写 204，代码是 `controller.go:130` 的 200）；**保留当前设备、吊销该用户其他所有设备的会话**（scs `IterableStore.All()` + `Codec` 按 `auth.user_id` 定位；零 schema 改动）；失败契约 fail-closed——吊销或写库失败都返回 500 且密码未改，旧密码可直接重试。
- 校验规则抽成 `internal/modules/auth/validate.go` 纯函数（表驱动单测，覆盖全角空格、ASCII 字母判定、`net/mail` 边界等）。
- 无 schema 改动、无新依赖、`app.go` 未改。测试新增 10 组 / 25+ 用例（双设备吊销、坏会话行容错、删号后改密码、已登录时注册切换会话、昵称/密码边界）。
- 文档：`backend/README.md` 认证章节重写（接口表 + 规则 + 已知缺口）；spec/plan 见 `docs/superpowers/{specs,plans}/2026-10-11-neoblog-auth-layer2*`。

### 计划评审

1 个独立子代理先审计划：2 Critical（scs API 名错、吊销顺序与失败契约自相矛盾）+ 3 Important（DTO 缺失、handler 与既有方法同名、路由归属写错）——全部修进计划后才开工；执行中编译期又抓到一处计划笔误（`IterableStore` 只有 `All()`、没有 `Delete`，删除改用 `Store.Delete`）。

### 已知缺口

- **登录 / 注册暂无限流**（本轮按用户要求不做；下一轮优先补，含部署时的可信代理配置）。
- 邮箱验证 / 找回密码（无 SMTP）、角色区分（还没有管理员专属接口）——均留到需要时。
- 前端未接：`/api` 代理与注册/登录表单由用户侧接线，后端只提供上面的接口契约。

---

## 2026-10-11 · 仓库换行符治理（.gitattributes）

**提交**：`acc8703`（`development`）；版本未变（后端 0.2.1 / 前端 0.1.0）。

### 问题

本机 git 的**全局** `core.autocrlf=true`（`D:/SoftWare/Git/etc/gitconfig`，不是仓库自己的配置）：索引存 LF、Windows 检出写 CRLF。于是任何让 git 重新写出文件的操作——clone、checkout、`reset --hard`、cherry-pick、rebase、重写历史——都会把文本文件在工作区变成 CRLF。`gofmt` 只认 LF，随即把 Go 文件报成「未格式化」，而 `git diff` 是空的（仓库内容其实没变）。重写认证分支历史后撞到过一次，当时误以为代码被改坏。

规模实测：**未加规则时克隆本仓库，158 个文本文件被检出为 CRLF，`gofmt -l` 报出全部 54 个 Go 文件。**

### 做了什么

- 新增 `.gitattributes`，内容一行：`* text=auto eol=lf`——在仓库内压过全局 autocrlf。
- 把当时工作区仍是 CRLF 的 5 个文件归一回 LF：`backend/README.md`、`backend/go.mod`、`backend/go.sum`、`docs/superpowers/{plans,specs}/2026-10-11-neoblog-auth*.md`。**跟踪内容零变化**（归一后 blob 与索引逐字节相同），因此本次提交只含 `.gitattributes` 一个文件。
- 例外情况无需处理：`frontend/app/favicon.ico` 是二进制（`text=auto` 自动跳过）、6 个单行 svg 没有行尾可转；仓库内没有 `.bat/.cmd/.ps1`，6 个 `*.sh` 本就要求 LF。

### 验证

临时克隆做 A/B（同一份内容，删除文件后强制重新检出全部文件）：

| 状态 | 重新检出结果 |
| --- | --- |
| 加规则前 | 158 个文件 CRLF；`gofmt -l` 报 54 个文件 |
| 加规则后 | 158 个文件全 LF；`gofmt -l` 无输出；`git status` 干净 |

规则提交后再克隆复验：160 个文件全 LF、`gofmt -l` 空、`git status` 完全干净——新克隆开箱不再出假警报。

### 排查与善后（复发时）

- 排查别先看 `gofmt -l`，用 `git ls-files --eol | grep -v 'w/lf'` 直接列出工作区非 LF 的文件；再用 `git diff -- <file> | wc -c` 为 0 确认仓库内容没变。
- 手动归一（`sed -i 's/\r$//'` 或对 Go 文件 `gofmt -w`）之后，这些文件会出现**假的 ` M`**：stat 缓存过期，blob 其实逐字节相同。`git update-index --refresh` 与 `git add --refresh` 都清不掉它（前者还会打印 "needs update" 误导人），必须 `git add -- <files>`；因 blob 相同，它不会暂存任何内容，用 `git diff --cached` 复核为空即可。

### 关键决定

- 采用全仓 `* text=auto eol=lf`，而不是最小化的 `*.go text eol=lf`：换行符与语言无关，前端 tsx/json/md 同样受 autocrlf 影响；`text=auto` 只对 git 判定为文本的文件生效，二进制安全。
- 用仓库内 `.gitattributes` 而不是改本机全局配置：规则随仓库分发，任何克隆与机器都免疫。

---

## 2026-10-11 · 认证层 1（只做后端）

**版本**：0.1.0 → **0.2.1**（minor＝新功能，patch＝补测试）

### 做了什么

- **接口**（前缀 `/api/v1`）：
  - `POST /auth/login`：body `{"identifier":"邮箱或昵称","password":"..."}` → 200 `{"email","nickname"}` + 会话 cookie；凭据错 401 `invalid_credentials`；请求不合法 400 `invalid_request`。
  - `POST /auth/logout` → **200 `{}`**（2026-10-12 对账更正：原文写 204，代码是 `controller.go:73`；`backend/README.md:135` 一直是对的）（销毁会话）。
  - `GET /auth/me` → 200 `{"email","nickname"}`；未登录 401 `unauthorized`。
- **会话**：`scs` + Postgres 存储（`sessions` 表）；cookie 名 `session`、HttpOnly、SameSite=Lax、`Secure` 仅当 `APP_ENV=production`；有效期 `SESSION_LIFETIME`（默认 168h）；登录成功换发 token（防会话固定），登出与「用户已删除」都会销毁会话。
- **密码**：bcrypt；拒绝空密码与超 72 字节密码；未知账号也做一次等价比较（抗时序探测）。
- **数据**：`ent` schema `users` / `sessions`（对齐数据层设计的 ent + Atlas 路线）；Atlas 首个迁移进 `migrations/`（生成后人工审阅）；`users` 字段对齐 Neo 项目：`email` / `nickname` 双唯一 + `password_hash` + 时间戳。
- **结构**：`internal/modules/auth/`（module / controller / service / repository / dto）+ `internal/common/guard/`（`RequireAuth`，会话键 `auth.user_id`）；装配只在 `internal/app/app.go`；`/api/v1` 前缀仍只定义一次。
- **测试**：单元 + 真库集成——登录 / 登出 / me 全流程、cookie 属性（HttpOnly / SameSite / Path、production 才 Secure）、删号后旧会话吊销、存储故障返回 500、大小写邮箱、超长 / 空密码、畸形请求、cookie `Max-Age` == `SESSION_LIFETIME`；无 `DATABASE_URL_TEST` 时集成用例自动 skip。
- **文档**：`backend/README.md` 更新（认证章节、环境变量、目录对照、迁移工作流、硬性规则 3 / 5 补充）；规格与计划见 `docs/superpowers/specs/2026-10-11-neoblog-auth-design.md`、`docs/superpowers/plans/2026-10-11-neoblog-auth.md`。

### 移除（用户要求；已从分支历史移除，不是 revert 提交）

- 前端接线：`next.config` 的 `/api` 转发规则、登录表单提交接线、语言包两条错误文案。
- `cmd/create-admin` 管理员创建命令。

### 评审与修复

独立子代理整分支终审：**0 Critical / 2 Important / 5 Minor**。

- 已修（每个都先有失败测试）：
  1. scs 存储写失败时登录 / 登出曾「假成功」——防会话固定会静默失效、登出会假报 204；现在登录失败直接 500 不落会话，登出失败返回 500。
  2. `CreateAdminUser` 曾接受空密码（会造出永远登不上的账号）；现在拒绝。
- 未处理的小问题（待定）：`Only()` 的 NotSingular 未映射（多用户后「邮箱撞昵称」会 500）；测试库残留 `sessions` 行未清理；`.env.example` 未列 `SESSION_LIFETIME`；`dummyHash` 生成失败时静默降级。

### 已知缺口

- **没有「首个管理员」的建号入口**（命令已按用户要求移除、注册未实现）；首次建号方式待定。
- 登录接口**无限流**（有意留到层 2；现有缓解：bcrypt 慢哈希、错误信息不区分账号、未知账号等价比较）。
- **前端未接**：`/api` 代理与登录表单由用户侧处理；后端只提供上面的接口契约。
- 生产 cookie 域（前端域 / API 子域）未定，部署轮再处理。

### 关键决定

- 选型：`scs` + `bcrypt` + `ent` + Atlas（对齐已批准的数据层设计，取代早期草案里的 sqlc + goose）。
- 会话 cookie 由 **Go 自己 Set-Cookie**（早期 BFF 草案「Go 不 Set-Cookie、Next 种 cookie」未采用）。
- 控制器把非哨兵错误映射为 500（凭据 / 授权问题才是 401）；`auth.New(db, sessions)` 不带 `cfg`（模块不需要配置）。
