# 开发日志

按时间**倒序**记录每个开发阶段的变更、决策与已知缺口。设计细节与实施计划在 `docs/superpowers/{specs,plans}/`；接口与结构规范以 `backend/README.md` 为准。

---

## 2026-10-13 · 补测试缺口：改资料接口的原子性、param、并发与边界值 + 测试入口脚本

**版本**：后端 **0.14.0**（未进位）。**零源码语义变更、零迁移**：本轮只动测试与测试入口，`internal/` 下的非测试代码一行未改。起因是对上一轮（改资料接口）的独立审查列出 5 条测试缺口，用户点名全做。

### 做了什么

- **组合字段原子性**（新用例 `TestUpdateProfileRejectedWriteLeavesBothFieldsUnchanged`）：同时提交「可用的昵称 + 撞车的邮箱」→ 409 `email_taken`，随后回查确认**昵称也没被写进去**；反方向（撞车昵称 + 可用邮箱）同样回查。此前两个字段从来没有一起提交过，"要么全改、要么全不改"这条只存在于代码读出来的形状里，谁改成两条 UPDATE 分两次写测试也照样绿。
- **`param` 断言**：`{"nickname":"a"}` → 400 + `param:"nickname"`；`{"email":"not-an-email"}` → `param:"email"`。前端 `use-profile-save` 全靠这个字段决定弹哪条文案，之前一个断言都没有。同时钉反向：请求体形状/类型错（`{"nickname":42}`）时 `param` 为空。
- **并发用例**（新用例 `TestUpdateProfileConcurrentSameNickname`）：两个已登录用户同时把昵称改成同一个值，正好一个 200、一个 409，且 409 的 code 落在 `{nickname_taken, already_taken}` 内。照 `TestRegisterConcurrentSameEmail` 的形状写，但**没有**复用 `exchangeJSON`——它内部 `t.Fatalf`，从测试 goroutine 里调用是错的（既有那条用例就有这个毛病）；另立了不在 goroutine 里断言的 `patchJSON`。
- **边界值**（并入 `TestUpdateProfileEndToEnd`）：裸 `null` 体、数组体、字符串体、数字体、`{"nickname":null}`、纯空格昵称、33 rune 昵称、261 字节邮箱 → 全部 400；emoji 昵称（1 emoji + 6 数字）→ 200 且能原样读回；把当前值**带前后空格**重提 → 200（trim 后自我匹配，正是前端"只提交改动字段"会走的那条）。空体与类型错的用例顺带补上 `param` 为空的断言。
- **会话清理**：新增 `loginWithLogout` 辅助（登录 + 在 `t.Cleanup` 里打一次登出），三个改资料用例改用它，跑完不再在 `sessions` 表留行。
- **测试入口脚本**（`scripts/test.sh`，新）：`go test ./... -count=1 -p 1`。不传包名时先确认 `DATABASE_URL_TEST` 已设置（避免集成用例静默 skip 后假装全绿），传包名原样转发。README 新增「测试」章节说明为什么必须串行。

### 过程中的发现

- **`already_taken` 兜底在端到端里打不到。** 把并发用例的断言临时收紧成「必须是 `already_taken`」跑了 10 遍，输家**每次**都是预检给出的 `nickname_taken`。原因：两个请求虽同时发出，但都要先走完会话加载 + `LoadIdentity` 查库才轮到预检，而预检（一次 SELECT）到 UPDATE 之间只有几十微秒的窗口，撞进去的概率极低。所以这条兜底仍是**只有代码、没有证据**的状态；要确定性覆盖，需要给 auth 模块一个可注入的仓储（当前 `repo` 是具体结构体，没有注入点）或在 ent 上挂钩子强制返回约束错误——本轮没做，留作已知缺口。断言因此写成「两者皆可」。
- **`-p 1` 是绕开、不是根治。** 偶发红的根因是两个包共用同一个测试库，而 `register/enabled` 是**存在数据库里的全局开关**：`app` 包的 `TestRegistrationControlEndToEnd` 会把它关掉再打开，`auth` 包不相干的注册用例撞进那段窗口就吃 403。根治要给每个包独立的库或 schema，但测试库是**预先迁移好的**、代码里没有迁移入口（`database.Open` 只连库、不动 schema），代价远超收益，所以在入口脚本上收口。
- 本轮真遇上了那次红：首跑 `go test ./...` 时 `TestRegisterRejectsDuplicateNickname` 返回 403 而不是 409；串行与随后两次并行都绿——典型时序偶发。

### 验证口径

- `bash scripts/test.sh` 11 包全绿无 skip（本机 5434）；`gofmt -l internal` 空、`go vet ./...` 空。
- 并发用例 `-count=20` 稳定通过（6.1s）。
- 原子性用例非空转：断言的是「回查后昵称仍等于**改之前**的值」，而请求里给的是另一个值——真发生半写就会红。
- 脚本守卫实测：不设 `DATABASE_URL_TEST` 时退出码 1 并打印提示。（先用 `env -u` 试跑时它没触发，查明是 Git Bash 下 `env -u` 在子 shell 里的行为差异，不是脚本问题——直接跑确认守卫生效。）

### 已知缺口

- `already_taken` 兜底仍无测试覆盖（原因见上），本轮只钉住「409 且 code 属于合法集合」。
- 并行冲突只做在入口脚本层面：绕过脚本直接敲 `go test ./...` 仍会偶发红，没有从代码层阻止这种跑法的手段。
- 只改了本轮这三个改资料用例的登出清理，其他既有用例仍在登录后不登出，`sessions` 表仍会有别处留下的行（一小时过期）。

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
