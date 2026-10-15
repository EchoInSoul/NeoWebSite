# NeoBlog 注册与认证设置组（register 扩充 + auth 新增；纯数据，不接实现）

**日期**：2026-10-15 · **范围**：纯后端（设置清单 + 测试 + 文档；零迁移、零接口、零前端） · **实施计划**：`docs/superpowers/plans/2026-10-15-neoblog-auth-settings.md`（随本 spec 一并交付）

**状态**：v1 草案，待用户审阅。分组方式（D1）、密码要求粒度（D3）、会话单位（D4）、注册总闸归属（D2）四处按 2026-10-15 对话用户裁定落稿。

---

## 1. 意图与背景

用户 2026-10-15 点名给后端加一批「注册与认证」设置，清单原样为：允许公开注册、注册默认组、启用邮箱域名控制、允许注册的邮箱后缀、启用邮箱认证、启用密码要求、密码要求、启用会话有效期、会话有效期（天）、两步验证（TOTP）、启用密码重置、启用人机验证、验证服务。

**关键口径（用户原话）**：「只是填写这些字段，并不接入这些具体的服务，只写最基础的数据，不接入实际上的实现」——本轮只往设置清单里加**定义与默认值**：值能存能读（管理端现成接口），但没有任何消费方：不接邮件、不让会话真过期、不验验证码。与上一轮 `site` 组同款做法（0.19.0）。

来源核对：该清单与 D 盘 HaoBlog「认证设置」页逐项对应（`lib/settings/groups/auth-settings.ts`；中文文案 `lib/console-i18n/dictionaries/zh-CN.ts` 的 `authForm` 块）。重复行（「启用密码要求」×2、「启用人机验证」×3）为表单 label 渲染重复，各按一个开关算；「普通用户 / haoryou.com / 天」为取值示例。

## 2. 已定决策（四处用户裁定 + 两处拟定）

| # | 决策 | 口径 | 出处 |
| --- | --- | --- | --- |
| D1 | 分组 | **拆两组**：注册与邮箱相关 4 项并入现有 `register` 组；密码 / 会话 / 2FA / 人机验证 9 项新建 `auth` 组 | 用户选「拆 register + auth」 |
| D2 | 注册总闸 | **沿用现有 `register/enabled`**，不再新增重复开关（D 盘自己就有 `allowPublicSignup` 与 `signupEnabled` 两个，NeoBlog 只保留一个真相源） | 用户选「沿用现有」 |
| D3 | 密码要求 | 拆两个字段：`password_min_length`（数字，默认 8）+ `password_complexity`（文字，默认空）；「启用密码要求」独立开关 | 用户选「长度+复杂度两个」 |
| D4 | 会话有效期 | `session_max_age_days`（数字，单位**天**，默认 **7**）；不照 D 盘存小时 | 用户选「天，默认 7」 |
| D5 | 公开性 | 新增 13 项**全部不标 `Public`**；公开端点的负载零变化（仍为 `site` 五项 + `register.enabled`） | 本轮对话拟定、用户未异议 |
| D6 | 默认值 | 布尔全 `false`、文字全 `""`；数字：`password_min_length=8`、`session_max_age_days=7`；`default_group="user"`（与 `rbac.GroupKeyUser` 一致）。`password_requirements_enabled` 取 **false**（D 盘为 true，本轮「默认全关」） | 拟定，列此备查 |

## 3. 范围

**改动文件（6 个）**：

| 文件 | 动作 | 职责 |
| --- | --- | --- |
| `backend/internal/modules/settings/registry.go` | 改 | `register` 组加 4 项；新增 `auth` 组 9 项——唯一生产代码改动 |
| `backend/internal/modules/settings/registry_test.go` | 改 | `TestBuiltinRegistryShape` 扩两组成员逐项断言（名字 / 类型 / 默认值 / 公开性） |
| `backend/internal/modules/settings/controller_test.go` | 改 | 新增两条 admin 用例：`register` 默认读、`auth` 默认读 + 写回 |
| `backend/README.md` | 改 | 「当前清单项」两处 + 顶部版本行（进位脚本代劳） |
| `docs/DEVLOG.md` | 改 | 顶部倒序登记本轮 |
| `backend/internal/version/version.go` | 改 | minor 进位 0.19.0 → 0.20.0 |

**不动清单**：`frontend/**` 全部（渲染接线是后续前端轮）；`controller.go` / `service.go` / `repository.go` / ent schema / 迁移（零数据库动作）；公开端点的行为与负载；`TestBuiltinPublicAllowlist` 的期望清单（依旧 6 项——新字段若被误标公开它会红，正是守卫用途）；`TestSettingsPublicGetAnonymously`（公开负载不变，验证「公开面零变化」）；API 规范文档（端点与口径零变化）。

## 4. 清单形状

```go
var builtinRegistry = []Group{
	{
		Name: "site",
		Items: []Definition{
			{Name: "title", Type: FieldTypeString, Default: "", Public: true},
			{Name: "subtitle", Type: FieldTypeString, Default: "", Public: true},
			{Name: "description", Type: FieldTypeString, Default: "", Public: true},
			{Name: "keywords", Type: FieldTypeString, Default: "", Public: true},
			{Name: "url", Type: FieldTypeString, Default: "", Public: true},
		},
	},
	{
		Name: "register",
		Items: []Definition{
			{Name: "enabled", Type: FieldTypeBool, Default: true, Public: true},
			{Name: "default_group", Type: FieldTypeString, Default: "user"},
			{Name: "email_domain_control_enabled", Type: FieldTypeBool, Default: false},
			{Name: "allowed_email_domains", Type: FieldTypeString, Default: ""},
			{Name: "email_verification_enabled", Type: FieldTypeBool, Default: false},
		},
	},
	{
		Name: "auth",
		Items: []Definition{
			{Name: "password_requirements_enabled", Type: FieldTypeBool, Default: false},
			{Name: "password_min_length", Type: FieldTypeNumber, Default: int64(8)},
			{Name: "password_complexity", Type: FieldTypeString, Default: ""},
			{Name: "session_max_age_enabled", Type: FieldTypeBool, Default: false},
			{Name: "session_max_age_days", Type: FieldTypeNumber, Default: int64(7)},
			{Name: "totp_enabled", Type: FieldTypeBool, Default: false},
			{Name: "password_reset_enabled", Type: FieldTypeBool, Default: false},
			{Name: "captcha_enabled", Type: FieldTypeBool, Default: false},
			{Name: "captcha_provider", Type: FieldTypeString, Default: ""},
		},
	},
}
```

逐行对照（用户清单 → 落点 → D 盘对应）：

| 用户清单行 | 组 / 键 | 类型 | 默认 | D 盘对应 |
| --- | --- | --- | --- | --- |
| （已有）允许公开注册 | `register` / `enabled` | 开关 | true | auth.allowPublicSignup（不取——已有） |
| 注册默认组（示例：普通用户） | `register` / `default_group` | 文字 | `"user"` | auth.defaultRole |
| 启用邮箱域名控制 | `register` / `email_domain_control_enabled` | 开关 | false | auth.signupEmailDomainControlEnabled |
| 允许注册的邮箱后缀（示例：haoryou.com） | `register` / `allowed_email_domains` | 文字（英文逗号分隔） | `""` | auth.allowedEmailDomains |
| 启用邮箱认证 | `register` / `email_verification_enabled` | 开关 | false | auth.emailCodeEnabled |
| 启用密码要求 | `auth` / `password_requirements_enabled` | 开关 | false | auth.passwordRequirementsEnabled（D 默认 true） |
| 密码要求（长度） | `auth` / `password_min_length` | 数字 | 8 | auth.passwordMinLength |
| 密码要求（复杂度） | `auth` / `password_complexity` | 文字 | `""` | auth.passwordComplexity |
| 启用会话有效期 | `auth` / `session_max_age_enabled` | 开关 | false | auth.sessionMaxAgeEnabled |
| 会话有效期（天） | `auth` / `session_max_age_days` | 数字 | 7 | auth.sessionMaxAgeHours（168h ≈ 7 天，改存「天」） |
| 两步验证（TOTP） | `auth` / `totp_enabled` | 开关 | false | auth.totpEnabled |
| 启用密码重置 | `auth` / `password_reset_enabled` | 开关 | false | auth.passwordResetEnabled |
| 启用人机验证 | `auth` / `captcha_enabled` | 开关 | false | auth.captchaEnabled |
| 验证服务 | `auth` / `captcha_provider` | 文字 | `""` | auth.captchaProvider（D 默认 selfhosted） |
| （D 盘有、本轮不要）Turnstile Site Key / Secret Key | — | — | — | 用户清单未含；secret 类型机制也不支持 |

说明：`captcha_provider` 是自由文字（机制无「选项」类型）；将来定供应方（如 Cloudflare Turnstile / 自研）与界面时再说。`default_group` 存的是组键字符串，数据层不校验存在性（D 盘有「该组不可用作默认组」的校验与文案，留到实现接线时）。

## 5. 行为

零新语义，全部沿机制既有行为：读（未写 → 默认值）、写（`PATCH` 部分更新、类型把关、未知项 400 带 `param`）、恢复默认（删行）；管理端 `GET/PATCH /api/v1/admin/settings/register` 与 `/api/v1/admin/settings/auth` 立即可用（权限点 `system.settings.view` / `.edit` 已覆盖，无新权限点）。公开端点的负载与本轮无关（新增项全非公开）。

## 6. 测试（三处，零新增文件）

| 文件 / 测试 | 改法 |
| --- | --- |
| `registry_test.go` · `TestBuiltinRegistryShape` | `register` 组：断言恰 5 项 + 新增 4 项逐项断名字 / 类型 / 默认值 / 非公开（既有 `register.enabled` 块与 `site` 块不动）；`auth` 组：断言恰 9 项 + 逐项同上 |
| `registry_test.go` · `TestBuiltinPublicAllowlist` | **不改**——期望仍 6 项；新增字段全非公开，误标公开它会红 |
| `controller_test.go` · **新增** `TestSettingsAdminReadsRegisterDefaults` | owner `GET /admin/settings/register` → 5 项整组默认值（`enabled=true`、`default_group="user"`、其余 false / `""`） |
| `controller_test.go` · **新增** `TestSettingsAdminPatchAuthRoundtrip` | owner `GET /admin/settings/auth` → 9 项默认（数字以 `float64` 解码：`session_max_age_days=7`、`password_min_length=8`）→ `PATCH {"session_max_age_days":14}` → 200 回显 14、未触碰项仍默认 → `GET` 回读 14 |
| `controller_test.go` · 既有公开读用例 | **不改**——公开负载仍是两组（验证「公开面零变化」正是它的活） |

变异自检（实施时逐条执行、证明会红再改回）：

| # | 变异 | 预期红 |
| --- | --- | --- |
| ① | `session_max_age_days` 默认值改 30 | 形状断言、`TestSettingsAdminPatchAuthRoundtrip` |
| ② | `captcha_enabled` 误标 `Public` | 白名单（7 项）、公开读（多一组） |
| ③ | `default_group` 从 `register` 挪进 `auth` | 形状断言（两组计数）、`TestSettingsAdminReadsRegisterDefaults` |

## 7. 已知缺口与风险

- **13 项全无消费方**：纯数据；控制台界面与实现接线（邮件模块真发信、会话模块读有效期、认证模块读密码策略等）都是后续轮。
- **`default_group` / `captcha_provider` 无取值约束**：自由文字，数据层不校验（机制无枚举校验插槽）。
- **`password_min_length` 无范围校验**：现在可以写 0 或负数（D 盘有 `minimum: 8`；NeoBlog 机制没有最小值插槽）——随实现接线一起加。
- **与 D 盘的三处口径差**：`password_requirements_enabled` 默认 false（D 为 true）；`session_max_age_days` 存天（D 存小时 168）；`captcha_provider` 默认 `""`（D 为 `selfhosted`）。两个项目对照时注意。
- **公开面零变化**是本轮的硬性质（D5）：将来前端要读（如密码最短长度提示）时，单独标 `Public` 即可。

## 8. 插槽（本轮不定）

| 插槽 | 时点 | 影响面 |
| --- | --- | --- |
| 各字段的消费方接线 | 用户点名逐个接 | 邮件 / 会话 / 认证 / 注册各模块读设置 |
| 校验（枚举 / 范围 / 域名格式） | 实现接线或界面出现时 | 机制加校验插槽（`Definition` 扩展） |
| secret 类型（Turnstile Secret Key 这类） | 需要密钥类设置时 | 机制级（信封加密 + 读取屏蔽） |
| 控制台设置界面 | 前端轮 | 前端；组即页、项即行 |

## 9. 验收标准

1. `gofmt -l` / `go vet ./...` / `go build ./...`（backend）干净。
2. `set -a && source .env && set +a && bash scripts/test.sh` 全绿。
3. 变异自检三条（§6）逐条实测红后再回绿。
4. 活体（建议）：临时实例公开读——负载与本轮前逐字一致（站点 + register 两组）；可选 `.scratch` 探针写读一组新字段后还原。
5. README 「当前清单项」两处更新；DEVLOG 条目；版本 0.19.0 → 0.20.0。
6. 零迁移、零新依赖、零新路由、零前端改动、公开面零变化。

## 10. 下一步

本 spec 经审阅后，按实施计划 `docs/superpowers/plans/2026-10-15-neoblog-auth-settings.md` 执行；执行方式由用户选（本轮改动仍小，建议 native）。**审阅通过前不动代码、不提交。**
