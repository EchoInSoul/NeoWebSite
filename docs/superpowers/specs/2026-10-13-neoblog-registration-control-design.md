# NeoBlog 注册控制设计（设置系统首个消费方 + 按组管理设置接口）

日期：2026-10-13
范围：`backend/`（Go + ent + gin），**零迁移**；前端一行不动
状态：v1 设计要点经用户确认（2026-10-13 对话三选一：只做开关 / 本轮加受保护的管理端接口 / 新码 `registration_closed` + 403），待用户过目本文后进 writing-plans

修订记录：
- v1（2026-10-13）：首版。响应两侧给裸对象；公开读只有本轮临时加的按组端点。
- v2（2026-10-13，用户后续要求「规范公开 API 以便复用」）：① 两侧统一为组记录 `{group, values}`、集合套 `items`（api-conventions §6 规则 4）；② 公开读改成**单个无鉴权聚合端点** `GET /api/v1/public/settings`，删除按组点查；③ 清单项新增 `Public` 标记位（白名单语义），`Service.PublicSettings` 按注册表顺序出组；④ 前端把专用 `lib/signup-status.ts` 收进通用层 `lib/public-settings.ts`，fail-open 与形状校验集中一处，`/signup` 关闭时 `notFound()`、登录页隐掉注册入口；⑤ 新增守卫 `TestBuiltinPublicAllowlist` 钉住公开项集合。版本 0.12.0 → **0.13.0**。

## 1. 意图与背景

用户目标：让管理员能关掉注册——关掉之后 `POST /api/v1/auth/register` 不再接受新账号，已登录用户与登录流程不受影响。

战略位置（比功能本身更重要）：这是设置系统（0.8.0 落的地子，见 `2026-10-11-neoblog-settings-design.md`）的**第一个真实消费方**，并同时兑现其 §10 插槽表第一行「接口（控制台按组管理设置）」。因此本轮的选择全部服从该机制已定的口径：清单定义在代码、库里只存被写过的值、加设置只改代码不动库。

约束（用户已定）：

- 本轮只改后端；前端注册页照常显示，不动一行。
- 只做开关。「新用户默认用户组」「邮箱/域名白名单」「注册频控」均不在本轮（§10）。
- 沿用既有规范：零注释、`DATABASE_URL_TEST` 集成测试缺省 skip、结构化日志、README 硬性规则。

## 2. 已定决策

| 项 | 决定 |
| --- | --- |
| 控制维度 | 只有一个布尔开关：是否开放注册 |
| 清单项 | 组 `register`、项 `enabled`、类型开关、默认 `true`（`^[a-z][a-z0-9_]*$` 合规） |
| 默认值取向 | 默认「开」= 现网行为零变化；库里无行即开，**不做任何 seed** |
| 存储 | 复用既有 `setting_values`；本轮零 DDL、零数据迁移 |
| 拦截位置 | `auth.Service.Register`（`internal/modules/auth/service.go:44`）函数第一句，先于字段校验、先于查重 |
| 关闭时的响应 | 403 / `permission_error` / `registration_closed`（apierror 新哨兵） |
| 开关读取失败 | 原样向外抛，不吞、不回退默认值（§6） |
| 初始化建号 | `auth.CreateAdminUser`（`auth/admin.go:16`）不经 `Service.Register`，**刻意保持不受开关影响**——否则关掉注册后无法建首个管理员 |
| 管理端接口 | 按组通用的 `GET` / `PATCH /api/v1/admin/settings/:group`，不是注册开关专用端点 |
| 鉴权 | 新增权限点 `system.settings.view` / `system.settings.edit`，走 `guard.RequirePermission`（`guard/permission.go:53`）；**不用** `RequirePresetGroup`（§9） |
| 权限点的存量数据 | 零迁移：owner/admin 的权限运行时取自代码目录（`access/service.go:344` → `rbac.PresetPermissions` → `rbac.All()`），库里 `permission` 列是纯字符串、无枚举无外键；新键对 owner/admin 立即可用，自建组默认不含（正确语义） |
| 请求体类型 | 值类型以服务端清单为准，客户端不申报类型（§5.2） |
| PATCH 粒度 | 只写请求体里出现的项，未出现的项不动；返回更新后的整组 |
| 缓存 | 不做。每次注册多一次单行点查（走既有复合唯一索引） |
| 公开读取（前台匿名读开关） | 不做——仍留设置 spec §10 插槽 |
| 界面 | 不做（本轮只后端） |
| 版本 | `0.11.0` → minor 进位（`internal/version/version.go:1`，执行时按当前值顺延，走 `scripts/bump-version.sh minor`） |

## 3. 范围

本轮做：

- 清单加第一项：`internal/modules/settings/registry.go:8` 的 `builtinRegistry` 由空切片变为含 `register/enabled`。
- 新错误码：`apierror` 加 `ErrRegistrationClosed`（`permission_error` / `registration_closed` / 403）并登记 `statusByCode`。
- 注册拦截：`auth` 模块 `Service` 多一个 `*settings.Service` 依赖，`Register` 首句把关。
- 设置模块长出路由：新增 `controller.go` / `module.go`，`settings.New(db)` 返回值由 `(*Service, error)` 改为 `(*Module, error)`（能力挂 `m.Service`），形状对齐 `mail`（`mail/module.go`，0.11.0 的「模块形状统一」轮）。
- 设置服务加一个方法：`PatchGroup`——把「原始 JSON ⊕ 清单类型」的解析与把关收在 service 层，controller 只碰 gin（§5.2）。
- 权限目录加两个键：`internal/common/rbac/rbac.go:43` 的 `catalog`。
- 装配：`internal/app/app.go` 里构造 settings 模块、把它注入 `auth.New`、并把设置路由挂到既有 `protected` 组（`app.go:70`）。
- 测试（§8）。
- 登记（§7）。

本轮不做：

- 任何前端改动；公开读取端点；界面。
- 新用户默认用户组、邮箱白名单、注册频控。
- 密钥类加密、类型之外的校验（范围/长度/选项）、导入导出。
- 缓存与并发控制。

## 4. 数据

**零迁移、零数据变更。** `setting_values` 表（`migrations/20261011104533_setting_values.sql`）已存在，本轮是它第一次被真实读写。

落库形状（只有关闭时才有行）：

| setting_group | name | value |
| --- | --- | --- |
| `register` | `enabled` | `false` |

- 恢复默认 = 删行（既有 `ResetItem` 语义）→ 注册重新开放。
- 库里若存在 `true` 的行，效果与无行相同（默认即 `true`）——写入路径不阻止这种冗余，与设置系统「只记被写入的值」的口径一致。

## 5. 接口

### 5.1 挂载与守卫

| 端点 | 守卫链 | 成功 |
| --- | --- | --- |
| `GET /api/v1/admin/settings/:group` | `RequireAuth → LoadIdentity → RequirePermission("system.settings.view")` | 200 + 组记录 `{"group": …, "values": {默认值 ⊕ 覆盖逐项合并}}`（v2 起，原为裸对象） |
| `PATCH /api/v1/admin/settings/:group` | 同上换成 `system.settings.edit` | 200 + 更新后的组记录（v2 起，原为裸对象） |

- 全局前缀 `/api/v1` 仍只在 `app.go` 定义一次（硬性规则 4）；两个端点都挂进既有 `protected` 组。
- `system.settings.edit` 经 `rbac.Expand` 自动隐含 `system.settings.view`（沿用既有 `impliedActions`，不新加规则）。
- 响应形状（v2 统一）：一个设置组的记录固定为 `{group, values}`，集合外套 `{items: […]}`（api-conventions §6 规则 4）；v1 写的「裸对象」已被取代。

`GET /api/v1/admin/settings/register` 的响应（无覆盖行时）：

```json
{ "enabled": true }
```

关闭注册：

```
PATCH /api/v1/admin/settings/register
{ "enabled": false }
→ 200 { "enabled": false }
```

### 5.2 请求体解析与错误映射

类型以服务端清单为准：controller 把体收进 `map[string]json.RawMessage`，交给 `Service.PatchGroup` 按清单项声明的类型逐项解码。

新方法签名（`internal/modules/settings/service.go`）：

```go
func (s *Service) PatchGroup(ctx context.Context, group string, raw map[string]json.RawMessage) (map[string]any, error)

type ItemError struct {
	Item string
	Err  error
}
```

`ItemError` 只为把「哪一项出错」带出到 controller（用于 `apierror.WithParam` 的 param），`Unwrap` 返回内层哨兵，故 `errors.Is` 仍能命中既有哨兵。内部复用既有 `resolveGroupWrites` + `repository.upsertMany`，不新起第二条写路径；成功后返回 `GetGroup(ctx, group)`。

| 情形 | 结果 |
| --- | --- |
| URL 组名不在清单 | 404 `not_found`（与 `PATCH /admin/groups/:id` 同口径） |
| `PATCH` 空体 / 零项 | 400 `invalid_request`（沿用 access 的「全缺省 PATCH → 400」规则） |
| 体不是合法 JSON 或不是对象 | 400 `invalid_request`（gin 绑定失败的现状口径，与 `signup` 一致） |
| 体里项名不在清单 | 400 `invalid_request` + `param` = 该项名 |
| 值与清单声明类型不符（如 `"enabled": "no"`） | 400 `invalid_request` + `param` = 该项名 |
| 项合法但库半路失败 | 整组事务回滚（既有 `upsertMany` 语义），错误原样抛出，由 apierror 映射：库不可用类 → 503，其余 → 500 |
| 已登录但无对应权限点 | 403 `forbidden`（`RequirePermission` 现状） |
| 未登录 | 401 `unauthorized`（`RequireAuth` 现状） |
| 库不可用 | 503 `service_unavailable`（`apierror.isUnavailable` 现状映射） |

零项判定：`PatchGroup` 对「组合法但零项」返回 `ErrUnknownItem`（与既有 `resolveGroupWrites` 一致），controller 映射为 400 且不带 param。

### 5.3 注册拦截的行为口径

`Service.Register` 第一句读 `register.enabled`：

- `false` → 立即返回 `apierror.ErrRegistrationClosed`；不做邮箱/昵称/密码校验、不查重、不建用户、不碰会话。
- 读取得到错误（未知组 / 库挂）→ **原样返回**，不吞、不回退默认值。清单与代码同仓同改，清单项缺失属编程错误，应当响亮失败（表现为 500 / 503），而不是静默放行。
- `true` → 现有流程一字不改。

明确不受开关影响的：`POST /auth/login`、`POST /auth/2fa/verify`、`auth.CreateAdminUser`、任何已登录用户的接口。

## 6. 装配与改动点

`internal/app/app.go`：

```go
settingsModule, err := settings.New(db)   // 原为 settings.New(db) 返回 (*Service, error)
authModule := auth.New(db, sessions, cfg.TOTPEncryptionKey, settingsModule.Service)
// protected 组内新增：settingsModule.RegisterProtected(protected)
```

- `App` 结构体现有的私有字段 `settings *settings.Service`（`app.go:40`、`:95`，构造后无任何出口、当前是事实上的死代码）**本轮删除**，settings 只作 `New` 内的局部变量（构造 → 注入 auth → 挂路由）。对照 `mailer`：那个字段留着的理由是 `shutdown` 要 `a.mailer.Close(ctx)`（`app.go:162`），settings 没有任何生命周期动作，不值得为它保留字段。
- `auth.New` 加一个参数，**同步改动点共两处**：`app.go:66` 与测试脚手架 `auth_test.go:30`（`New(db, sm, testTOTPKey)`）。改完由 `go build ./...` 驱动确认没有第三处。
- `settings.New` 返回值形状变了，调用点只有 `app.go:44` 一处（`grep -rn "settings.New(" backend`）。
- 无新增第三方依赖；无 ent 生成物变化；`scripts/db-generate.sh` 与 `db-migrate-diff.sh` 本轮不动。

## 7. 登记点（行号会漂，实施时现读现引）

- `backend/README.md` 四处：① 目录对照表 `internal/modules/settings` 那行（`:28`）改成为有路由模块；② 硬性规则 1（`:35`）——「无路由的能力模块（当前 `settings`）」清单本轮起为空；③ 硬性规则 5（`:39`）——settings 从「`New` 直接返回能力接口」改为返回 `*Module`，与 `mail` 并列；④ 「设置」章节（`:247` 起）——删掉「本轮清单不含任何真实设置」这句过期话，补两个端点与 `register/enabled` 实例。迁移清单不变（本轮零迁移）。
- `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md`：§3 路由表追加两条（现表止于第 20 条）；§7.3 code 词汇表追加 `registration_closed`（403 / `permission_error` / param null），前言那句「0.11.0 起新增 `method_not_allowed`」按惯例续写本轮新增；§13.1 表 A 若逐端点列举则同步。
- `backend/internal/common/apierror/apierror_test.go`：`TestRegistryIntegrity` 的哨兵切片加 `ErrRegistrationClosed`，两处硬编码计数 `28 → 29`（`:259`、`:262`）——不改则测试红；`TestRegistryMatchesSpec` 增一条 403/`permission_error` 用例。
- `internal/version/version.go` 进位；`docs/DEVLOG.md` 加条目。

## 8. 测试

| 层 | 连库 | 内容 |
| --- | --- | --- |
| 清单形状（单测） | 否 | 新增一条断言内置 `builtinRegistry` 确含 `register/enabled`、类型 `FieldTypeBool`、默认 `true`——防「项被误删导致注册控制静默失效」。既有 `TestRegistryRecipeShape` / `TestRegistryEmpty` 用的是本地字面量与 `compileRegistry(nil)`，不碰内置清单，不受影响 |
| `PatchGroup`（单测，注入样例组，不触库） | 否 | 未知项 → `ItemError{Item}` 且 `errors.Is(…, ErrUnknownItem)`；类型不符 → `ItemError{Item}` 且命中 `ErrTypeMismatch`；零项 → `ErrUnknownItem`；未知组 → `ErrUnknownGroup` |
| 设置端点（HTTP 集成） | 是 | `GET` 无覆盖行时返回全默认；`PATCH {"enabled":false}` → 200 且读回 false；未知组 → 404；空体 → 400；`{"enabled":"no"}` → 400 且 `param=enabled`；`{"nope":1}` → 400 且 `param=nope`；普通用户（无 `system.settings.*`）→ 403；未登录 → 401 |
| 注册拦截（HTTP 集成） | 是 | ① 库里无行 → 注册 201（默认开）；② `PATCH` 关掉后 → 403 `registration_closed`，且 `users` 表行数不变；③ 开关优先于**服务内**的字段校验——关闭态下拿「非空但格式非法」的体（`{"email":"no-at","nickname":"nick","password":"pass1234"}`）打过来是 403 而非 400，同一个体在开启态是 400。**不能用空体 `{}` 当证据**：`signup` 在 controller 层就因三个字段为空判 400（`auth/controller.go:54`），进不到 service，测不出门的位置；④ 再打开 → 201；⑤ 关闭态下 `POST /auth/login` 对既有用户照常成功；⑥ 关闭态下 `auth.CreateAdminUser` 仍能建号 |
| 回归 | 否 | `go build ./... && go vet ./... && gofmt -l .` 干净；无库 `go test ./...` 全绿（集成 skip）；`TestRegistryIntegrity` 计数 29；`auth.New` 两处调用点同步 |

**清理硬要求**：凡写过 `register/enabled` 行的用例必须在 `t.Cleanup` 里删行（`ResetItem` 或直接 SQL）。测试库里那行若残留，同库后续所有 auth 用例会莫名继承「注册已关闭」而吃 403——表现为毫不相干的测试红，难查。集成测试既有约定（随机后缀 + `t.Cleanup`）照抄，注意 `settings` 包内既有集成用例用的是固定组名 `site`，与本轮的 `register` 不撞。

## 9. 已知缺口与风险

- **前端零感知**：注册页照旧显示表单与按钮，访客提交才吃 403。要做到「按钮直接标为已关闭」需要设置 spec §10 的「公开读取」插槽，本轮刻意不做。
- **与前端设置页的关系（记时间戳，会漂）**：2026-10-13 写本 spec 时核工作区，见到并行会话在做 `frontend/app/console/settings/[section]`，分区定义在 `frontend/lib/console-settings-sections.ts`，五个分区 `info` / `storage` / `notify` / `credentials` / `code-injection`，每页只渲染一张空卡片、无取数代码；**出本计划时再核，那套改动已被该会话回滚**（`[section]` 目录留空，`console-settings-sections.ts` 与 `console-sidebar-nav.tsx` 已删，侧边栏回到 `<SidebarContent />` 空挂）。两点照旧成立：① 五个分区里**没有注册/账号落点**，本轮开关在界面无处可改，与「界面不做」一致；② 前端 `SettingsGroup` 指侧边栏**导航分组**，后端 `settings.Group` 指**存储组**，同名不同义，那套页面回来时不要互相套用。
- **改开关只能打接口**：没有界面，管理员需用 PowerShell / curl 打 `PATCH /api/v1/admin/settings/register`（带会话 cookie）。
- **权限点可授出**：走权限目录意味着 `system.settings.edit` 可以授给自建组，即非 owner/admin 的角色也可能被授「改注册开关」。这是选权限点而非 `RequirePresetGroup` 的既定取向（可组合、将来控制台能按角色勾），不是疏漏；若要改成「只允许 owner/admin」，代价是失去按角色组合的能力，届时再议。
- **`register/enabled` 项若被将来删掉**：库里残留行不会被任何读取路径碰到，只占复合键——设置 spec §9 已记的孤儿行缺口，本轮沿用人工 SQL 处理。
- **无缓存**：注册高频时每次多一次点查。单管理员博客量级不构成问题，出现再评估。
- **并发写**：两个管理员同时 PATCH → 最后写赢（行级 upsert，无版本号）。同既有口径。

## 10. 验收标准

1. 零迁移：`migrations/` 目录文件数与 `atlas.sum` 不变。
2. 关掉开关后 `POST /api/v1/auth/register` 返回 403 `registration_closed`；打开后恢复 201；库里无行时行为与打开一致。
3. 两个设置端点按 §5 的守卫链与错误映射逐条对上，owner/admin 可读写，其余角色 403。
4. `POST /auth/login` 与 `auth.CreateAdminUser` 在关闭态不受影响（有测试为证）。
5. §7 四处 README、API 规范两处表、apierror 两处计数、版本进位、DEVLOG 全部登记到位。
6. `go build` / `go vet` / `gofmt -l` 干净；有库跑 §8 集成全过；无库 `go test ./...` 全绿。
7. 本轮不碰 `frontend/`：不新增、不修改任何前端文件。写本计划时工作区里并行会话的前端设置页在制改动已被其自行回滚（只剩一个空的 `app/console/settings/[section]/` 目录，git 不跟踪空目录）——无论它是否回来，本轮都不接管、不提交、也不去删那个空目录。
8. 提交按文件清单暂存，**不裸 `git add -A`**——`docs/DEVLOG.md` 此刻正带着并行会话未提交的环境轮条目，本轮追加条目时只加不删（`DEVLOG.md:358` 立的防再犯规矩）。

## 11. 下一步

本文经用户审阅通过后，进 writing-plans 出实施计划（范围 = §3，验收 = §10）。
