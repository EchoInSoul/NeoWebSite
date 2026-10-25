# 人机验证设置组归位与自检路由迁移 设计

日期：2026-10-25
状态：待审阅
前序：2026-10-25 人机验证拆成独立能力模块（captcha 模块已存在，无路由）；登录/注册/会话拆分轮

## 1. 背景与目标

人机验证已拆成独立能力模块 `internal/modules/captcha`，但仍有三处历史遗留让它"名不正"：

1. 5 个设置键（`captcha_enabled`、`captcha_provider`、`captcha_scopes`、`turnstile_site_key`、`turnstile_secret_key`）住在 `auth` 设置组里；
2. 管理端自检路由 `POST /admin/captcha/verifications` 及其 DTO 长在 `auth` 包里，守卫借的是 `system.register.edit`；
3. 前端设置页读写 `/admin/settings/auth`、公开设置从 `auth` 组取 site key。

本轮把这三处归位：设置拆出独立的 `captcha` 组、自检路由搬进 captcha 模块、权限改用新增的 `system.captcha` 对。同时为下一轮 go-captcha（自研人机验证）铺路：将来的 `captcha_mode`、难度等新设置项直接落进新组。

### 明确不做（本轮范围外）

- go-captcha 实现：`builtin` 选项本轮仍是"选了不拦"的现状（`captcha.Service` 只认 turnstile），四形态、难度、挑战表、管理端测试接口新形态全部留给下一轮。
- `findUserByIdentifier` 双用途拆分（邮箱登录轮）。
- 历史自建组的权限补授脚本（见 §6 已接受代价）。

## 2. 设置组：`auth` 拆出 `captcha`

### 2.1 清单（registry.go）

新增 `captcha` 组，声明在 `auth` 组之后：

| 键 | 类型 | 默认值 | Public | 说明 |
| --- | --- | --- | --- | --- |
| `captcha_enabled` | bool | `false` | 是 | 原样搬 |
| `captcha_provider` | string | `"none"` | 是 | 原样搬 |
| `captcha_scopes` | string | `"login,register"` | 是 | 原样搬 |
| `turnstile_site_key` | string | `""` | 是 | 原样搬 |
| `turnstile_secret_key` | string | `""` | 否 | 原样搬 |

`auth` 组删掉这 5 项，剩 7 项；`ValidateOn` 去掉 `captcha_scopes`，剩 `password_min_length`、`session_max_age_days`、`session_max_age_enabled`。

组校验拆分：`captcha_scopes` 的合法性检查（只接受 `login`/`register`、空清单合法）从 `validateAuthGroup` 拆出 `validateCaptchaGroup`，`ValidateOn: ["captcha_scopes"]`。

`settings.CaptchaScopeLogin` / `CaptchaScopeRegister` 常量留在 settings 包不动（login/registration 模块在引用）。

### 2.2 消费方（captcha 服务）

`captcha/service.go` 的四处读取组名 `"auth"` → `"captcha"`。其余消费方不变：

- `settings.New`（模块构造）、registry 自检逻辑不动；
- login / registration 模块通过 `settings.CaptchaScope*` 传 scope，不直接读组，无需改动。

### 2.3 数据迁移

一条新迁移（编号紧接 `20261025120001`，配 `atlas migrate hash` 重算 `atlas.sum`），三步：

```sql
-- ① 现值搬家（保住已配置的密钥）
INSERT INTO "public"."setting_values" ("id", "setting_group", "name", "value", "updated_at")
SELECT gen_random_uuid(), 'captcha', "name", "value", now()
FROM "public"."setting_values"
WHERE "setting_group" = 'auth'
  AND "name" IN ('captcha_enabled','captcha_provider','captcha_scopes','turnstile_site_key','turnstile_secret_key')
ON CONFLICT ("setting_group", "name") DO NOTHING;

-- ② 字面默认值兜底（全新库没有 auth 组旧行时靠这条；与清单默认值一致）
-- 5 行 INSERT ... ON CONFLICT DO NOTHING，值与 2.1 表格的默认值相同

-- ③ 删旧行
DELETE FROM "public"."setting_values"
WHERE "setting_group" = 'auth'
  AND "name" IN ('captcha_enabled','captcha_provider','captcha_scopes','turnstile_site_key','turnstile_secret_key');
```

①在②之前执行：老库里用户的现值优先于默认值；②的 `ON CONFLICT DO NOTHING` 保证①已搬的行不被覆盖。

### 2.4 历史 seed 迁移改写

seed 对账测试（`seed_migration_test.go`）的语义：按文件名顺序读取所有 `*_seed_*.sql` 的行，`组.键` 跨文件去重（后一条覆盖前一条），去重后的总数必须恰好等于清单全集，且每行值等于清单默认值。

本轮是**组名搬家**，与品牌轮"只改值"不同，没有"新增一条 seed 覆盖"的出路：不改历史 seed 的话，旧文件里 `auth` 组的 captcha 行在新清单里不存在，计入总数导致 `total != want`；同时新 `captcha` 组缺行报 missing。所以必须同批改三条历史 seed（品牌轮 40b462e 已有改历史 seed 的先例，且实测不影响 `atlas status`）：

- `20261023080025_seed_setting_values.sql`：删掉 `auth` 组的 4 行 captcha（`captcha_enabled`、`captcha_provider`、`turnstile_site_key`、`turnstile_secret_key`）。该文件种的 `captcha_provider` 是 `""`，新清单默认是 `"none"`，新组不从该文件取值。
- `20261024120000_seed_captcha_provider_none.sql`：组名 `auth` → `captcha`。该迁移的原始职责是"把已存在的空串 provider 修正为 `none`"（带 `WHERE value = '""'` 的 upsert）；搬到 `captcha` 组后，老库 `auth` 组的空串行不再命中——但其职责已由新迁移 §2.3 ①（现值随迁）完整继承：老库搬过来的是什么值就用什么值，`none` 语义由 `20261024` 已应用过的效果（老库此时已是 `none`，不是 `""`）保证，空串只存在于"从没应用过该迁移的库"，而那种库必然是全新库，走②默认值即为 `none`。
- `20261025120000_seed_captcha_scopes.sql`：组名 `auth` → `captcha`。

### 2.5 重置行为的注意点

`ResetItem` 把代码默认值写回该项——迁移后 captcha 项重置写进 `captcha` 组，与旧行为（写 `auth` 组）不同，但旧行已被迁移删除，无残留读取方。无需特殊处理。

## 3. 自检路由搬进 captcha 模块

### 3.1 captcha 升级为有路由模块

新增文件（STYLE.md §1 有路由模块形态）：

- `module.go`：`Module` 结构、`New(db, settingsService) *Module`——生产构造 `captcha.New(settings)` 并挂 controller；`RegisterProtected(r)` 挂自检路由。
- `controller.go`：`POST /admin/captcha/verifications` handler，逻辑原样从 `auth/controller.go` 的 `verifyCaptchaConfig` 搬入（含 `requireJSONRequest` 同款守卫：Content-Type 必须 JSON、Origin 与 Host/X-Forwarded-Host 同源检查）。
- `dto.go`：`CaptchaVerificationRequest`（`secret_key`、`token`）、`CaptchaVerificationResponse`（`verified`）随迁。

`requireJSONRequest` 是 auth 包私有函数，captcha 包需要同款。方案：把该函数**复制**到 captcha 包（约 25 行），不从 auth 导出——它是 HTTP 细节，各 controller 自持比跨模块导出共享 helper 更符合本仓"边界靠 DTO、模块自包含"的取向。auth 保留自己那份（login/signup 还在用）。

`Service` 上的 `VerifyConfig` 方法不动；`New` / `NewService` / `Service.Verify` 不动。

### 3.2 auth 侧清理

- 删 `auth/controller.go` 的 `verifyCaptchaConfig` handler 与路由行；
- 删 `auth/service.go` 的 `VerifyCaptchaConfig` 方法与 `captchaVerifier` 接口（auth 不再依赖 captcha）；
- `auth.New` 去掉 captcha 参数；`app.go` 不再给 auth 传 captchaService；
- login / registration 各自的 `captchaVerifier` 窄接口（只有 `Verify`）保持不变。

### 3.3 路由与路径

路径不变：`POST /api/v1/admin/captcha/verifications`。挂载点从 `authModule.RegisterProtected(protected)` 移到 `captchaModule.RegisterProtected(protected)`。

## 4. 权限点 `system.captcha`

### 4.1 rbac 目录

`internal/common/rbac` 新增：

```go
PermissionCaptchaView = "system.captcha.view" // 名称：查看人机验证
PermissionCaptchaEdit = "system.captcha.edit" // 名称：管理人机验证
```

加进 `catalog`，rbac 测试的目录断言同步。`edit` 蕴含 `view`（既有 `Expand` 语义自动生效）。owner / admin 预设组运行时全量授权，无需数据迁移。

### 4.2 settings 接口的组权限分流

`settings/controller.go` 的 `requireGroupPermission` 从两路改三路：

- `register`、`auth` → `system.register.view` / `system.register.edit`（不变）；
- `captcha` → `system.captcha.view` / `system.captcha.edit`（新增）；
- 其余（`site`、`mail`）→ `system.settings.view` / `system.settings.edit`（不变）。

README 的权限点描述同步（当前四对 → 五对）。

### 4.3 自检路由守卫

`POST /admin/captcha/verifications` 从 `system.register.edit` 改挂 `system.captcha.edit`（与保存 captcha 设置组同一把钥匙）。

### 4.4 前端权限矩阵

`frontend/lib/groups.ts`：

- `RESOURCE_LABEL_KEYS` 加 `"system.captcha": "resourceCaptcha"`；
- `MATRIX_GROUPS` 单独一节 `{ labelKey: "captchaGroup", resources: ["system.captcha"] }`，放在「注册与登录」节之后。labelKey 取 `settings` 命名空间已有键 `captchaGroup`（「人机验证」/ "CAPTCHA"），无需新键。

i18n（zh-CN / en）：仅新增资源名 `groups.resourceCaptcha`（「人机验证」/ "Human verification"）。

## 5. 前端读点切换

| 文件 | 改动 |
| --- | --- |
| `hooks/use-captcha-settings-save.ts` | PATCH `/api/v1/admin/settings/auth` → `/api/v1/admin/settings/captcha` |
| `lib/admin-settings.ts` 的 `getCaptchaSettings` | `fetchAdminPayload("/api/v1/admin/settings/auth")` → captcha 组 |
| `lib/public-settings.ts` 的 `getCaptchaConfig` | `settings.auth.captcha_*` / `settings.auth.turnstile_*` → `settings.captcha.*` |

登录 / 注册页、`captcha-settings.ts` 的解析器、`CaptchaDialog`、设置页表单零变化——键名、类型、形状都没变，只是组名换了。`lib/captcha-settings.ts` 的 `parseCaptchaSettingsValues` 读 `payload.values`，与组名无关，无需改动。

登录 / 注册设置页（`use-login-settings-save`、`use-register-settings-save`、`admin-settings.ts` 的另两处 `/admin/settings/auth`）继续读写 `auth` 组，不在本轮范围。

## 6. 已接受的代价与风险

- **历史自建组失去 captcha 设置访问权**：新权限点不会自动授给库里已有的自建组。授了「管理用户注册」的管理组不再能看/改人机验证设置页（403）与调自检接口。这是选 `system.captcha` 而非沿用 `system.register` 的直接代价，用户已裁定接受；补授靠管理界面手工操作，不做脚本。
- owner / admin 不受影响（预设组运行时全量授权）。
- 开发库与测试库当前只有预设组成员，无实际 403 风险。
- 公开设置响应形状变化（`auth` 组公开值 5→1，新增 `captcha` 组 4 值）：`GET /public/settings` 的消费方只有本项目前端，本轮同步切换，无第三方兼容负担。README 的设置清单描述同步更新。

## 7. 测试与验证计划

### 后端

- `settings/registry_test.go`：auth 组计数 12→7；新增 captcha 组 5 项断言。
- `settings/controller_test.go`：公开设置形状——4 组、顺序 site→register→auth→captcha；auth 公开值只剩 `totp_enabled`；captcha 公开 4 值（密钥不出现）。`resetBuiltin` 补 `captcha` 组。PATCH 权限测试补：无 `system.captcha.edit` 的普通用户改 captcha 组 → 403；owner 可改。
- `settings/seed_migration_test.go`：改写后的三条历史 seed + 新迁移合计恰好覆盖清单全集（auth 7 + captcha 5 + register 5 + site 6 + mail 12）。
- `captcha` 包：现有流程测试（登录/注册拦截、scope 过滤、空清单、自检路由 400/503/403/401）全部保留，设置写入的组名改 `captcha`；自检权限测试从 `system.register.edit` 改为新权限点（含普通用户 403、未登录 401、owner 200）。
- `app_test`、`auth` 包测试：因 `auth.New` 签名变化与设置组迁移的间接影响处逐一修正。
- 变异自检（防假绿）：至少三条——①迁移删行删掉（老库残留 auth 组行）→ seed 对账或公开形状测试红；②权限分流漏 `captcha` 分支 → captcha 组 403 测试红；③`validateCaptchaGroup` 去掉 → 非法 scope 写入不被拒的测试红。
- 全量 `scripts/test.sh` + `scripts/check.sh`。

### 前端

- `npx tsc --noEmit`、ESLint、改文件 Prettier。
- 手工核对点（运行期归用户）：设置页人机验证保存/读回走 captcha 组；登录注册页 captcha 配置能取到 site key；权限矩阵出现「人机验证」节。

### 迁移实测

- 开发库：应用迁移后 `captcha` 组 5 行值与迁移前 `auth` 组现值一致；`auth` 组无残留 captcha 行。
- 测试库：同样应用并验证；`ResetItem`、PATCH、公开接口走一遍。

## 8. 文档

- `backend/README.md`：设置清单章节（auth 7 项、captcha 5 项、公开标记）、权限点四对→五对、auth 模块表行（自检路由移出）、captcha 模块表行（升级为有路由模块）。
- `docs/DEVLOG.md`：本轮条目（动机、决策含 §6 代价、验证、已知缺口——`builtin` 未实现、`findUserByIdentifier` 双用途、自建组需手工补授）。
