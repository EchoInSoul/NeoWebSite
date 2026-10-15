# 注册与认证设置组（register 扩充 + auth 新增）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 设置清单里 `register` 组扩 4 项、新建 `auth` 组 9 项（纯数据、全非公开），同步测试与文档，版本进位 0.20.0。

**Architecture:** 唯一生产代码改动是 `builtinRegistry` 的字面量（加字段、加组）；机制、接口、表全部不动。测试先行：先扩形状断言、加两条 admin 用例（红）→ 落清单（绿）→ 变异自检三条 → 全量验证。

**Tech Stack:** Go 1.27.2、ent、Gin（均不动）；测试走 `scripts/test.sh`（`-p 1` 串行）。

**Spec:** `docs/superpowers/specs/2026-10-15-neoblog-auth-settings-design.md`（本计划按该 spec 的 D1–D6、§4–§6 论证；两份一起读）

## Global Constraints

- 零迁移、零新依赖、零新路由、零新权限点、零前端改动、**公开面零变化**（spec §3 不动清单）。
- 13 项字面量逐字照抄 spec §4——名字 / 类型 / 默认值 / 是否公开，一项不多、一项不少；**不加校验、不加 D 盘那些本轮没点的字段**（Turnstile 密钥、登录控制等）。
- 既有 `register.enabled` 断言块与 `site` 断言块、`TestBuiltinPublicAllowlist`（期望仍 6 项）、`TestSettingsPublicGetAnonymously`（公开负载仍两组）**一律不动**。
- 后端 Go 零人工注释、gofmt 缩进；测试失败消息沿用既有风格。
- **不主动提交**：全部改动留在工作区；提交与否与批次由用户点名（建议批次见文末）。
- 跑测试前 `export PATH="/f/software/go/bin:$PATH"`、`cd backend`，再 `set -a && source .env && set +a` 导入环境（`.env` 是裸赋值、不带 `export`，不加 `set -a` 子进程看不到 `DATABASE_URL_TEST`，集成用例会被静默 skip——后文所有 `source .env` 写法一律按此展开执行）；一次只跑一个测试进程；**不用 `-race`**（本机 cgo 不可用，会挂起）。部分 Run 行省略了这些前缀——同一 shell 内先执行一次即可。
- 不动的文件：`controller.go` / `service.go` / `repository.go` / ent schema / `scripts/*` / `frontend/**`。

## Review Focus

spec 隐含、最可能咬人的五条，逐条在对应任务里钉：

1. **公开面零变化**：新增 13 项全非公开——白名单期望仍 6 项、公开读仍两组；任何「公开项多了」必须被变异②咬住。
2. **JSON 数字解码是 `float64`**：admin 读回的 `session_max_age_days` / `password_min_length` 在 `map[string]any` 里是 `float64`；断言写 `float64(7)` / `float64(8)` / `float64(14)`——写 `int` 会恒假红。
3. **组内计数 + 逐项断言双保险**：`register` 恰 5 项、`auth` 恰 9 项（防多加 / 漏加）；逐项断名字 / 类型 / 默认值 / 非公开（防改名 / 改型 / 改默认 / 误标公开）。
4. **组内顺序无关**：组内是 `map`，断言不得依赖项顺序；组序 `site → register → auth` 只影响公开负载排列（`auth` 无公开项，不出现）。
5. **只加字段**：不许顺手给机制加校验（枚举 / 范围）、不许加 `Public: true`、不许动既有断言。

---

## 文件结构

| 文件 | 动作 | 唯一职责 |
| --- | --- | --- |
| `backend/internal/modules/settings/registry_test.go` | Modify | 形状断言扩两组成员（Task 1） |
| `backend/internal/modules/settings/controller_test.go` | Modify | 两条 admin 用例（Task 1） |
| `backend/internal/modules/settings/registry.go` | Modify | 清单扩字段、加 `auth` 组（Task 2，唯一生产代码） |
| `backend/README.md` | Modify | 「当前清单项」两处（Task 3） |
| `docs/DEVLOG.md` | Modify | 顶部倒序登记（Task 3） |
| `backend/internal/version/version.go` | Modify | 版本进位（Task 3，脚本代劳） |

Task 1 只改测试（此时新断言全红）；Task 2 落清单（转绿 + 变异）；Task 3 文档与版本；Task 4 全量验证。

---

### Task 1: 三处测试先红

**Files:**
- Modify: `backend/internal/modules/settings/registry_test.go`
- Modify: `backend/internal/modules/settings/controller_test.go`

**Interfaces:**
- Consumes: 既有 `builtinRegistry` / `compileRegistry`、包内测试助手 `testDB` / `resetGroups` / `newSettingsTestApp` / `seedClient` / `createUserInGroup` / `doJSON` / `expectStatus` / `decodeGroup`
- Produces: 期望值（供 Task 2 对齐）——`register` 恰 5 项、`auth` 恰 9 项，逐项名字 / 类型 / 默认值 / 非公开；admin 读 `auth` 的 `session_max_age_days=7`（float64）

- [ ] **Step 1: `registry_test.go` 加断言助手 + 扩两组断言**

在 `recipeGroups()` 之前（import 块之后）加助手：

```go
type wantItem struct {
	name   string
	typ    FieldType
	def    any
	public bool
}

func assertDefinitions(t *testing.T, items map[string]Definition, wants []wantItem) {
	t.Helper()

	for _, want := range wants {
		def, ok := items[want.name]
		if !ok {
			t.Fatalf("missing item %s", want.name)
		}
		if def.Type != want.typ {
			t.Fatalf("item %s type = %v, want %v", want.name, def.Type, want.typ)
		}
		if !reflect.DeepEqual(def.Default, want.def) {
			t.Fatalf("item %s default = %#v, want %#v", want.name, def.Default, want.def)
		}
		if def.Public != want.public {
			t.Fatalf("item %s public = %v, want %v", want.name, def.Public, want.public)
		}
	}
}
```

`TestBuiltinRegistryShape` 在既有 `site` 循环结束后、函数收尾 `}` 之前追加：

```go
	if len(reg.groups["register"]) != 5 {
		t.Fatalf("register items = %d, want 5", len(reg.groups["register"]))
	}
	assertDefinitions(t, reg.groups["register"], []wantItem{
		{name: "default_group", typ: FieldTypeString, def: "user"},
		{name: "email_domain_control_enabled", typ: FieldTypeBool, def: false},
		{name: "allowed_email_domains", typ: FieldTypeString, def: ""},
		{name: "email_verification_enabled", typ: FieldTypeBool, def: false},
	})

	if len(reg.groups["auth"]) != 9 {
		t.Fatalf("auth items = %d, want 9", len(reg.groups["auth"]))
	}
	assertDefinitions(t, reg.groups["auth"], []wantItem{
		{name: "password_requirements_enabled", typ: FieldTypeBool, def: false},
		{name: "password_min_length", typ: FieldTypeNumber, def: int64(8)},
		{name: "password_complexity", typ: FieldTypeString, def: ""},
		{name: "session_max_age_enabled", typ: FieldTypeBool, def: false},
		{name: "session_max_age_days", typ: FieldTypeNumber, def: int64(7)},
		{name: "totp_enabled", typ: FieldTypeBool, def: false},
		{name: "password_reset_enabled", typ: FieldTypeBool, def: false},
		{name: "captcha_enabled", typ: FieldTypeBool, def: false},
		{name: "captcha_provider", typ: FieldTypeString, def: ""},
	})
```

（既有 `register.enabled` 断言块与 `site` 块不动；`wantItem.public` 零值即 `false`——两项新组里没有公开项，全部按 `false` 断言。）

- [ ] **Step 2: `controller_test.go` 加两条 admin 用例**

追加到 `TestSettingsPatchSiteTitleReadsBackPublicly` 之后：

```go
func TestSettingsAdminReadsRegisterDefaults(t *testing.T) {
	db := testDB(t)
	resetGroups(t, db, "register")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	resp := doJSON(t, owner, http.MethodGet, ts.URL+"/api/v1/admin/settings/register", nil)
	expectStatus(t, resp, http.StatusOK)
	group, values := decodeGroup(t, resp)
	if group != "register" {
		t.Fatalf("group = %q, want register", group)
	}
	if len(values) != 5 {
		t.Fatalf("register values = %v, want the five defaults", values)
	}
	if values["enabled"] != true {
		t.Fatalf("register.enabled = %v, want true", values["enabled"])
	}
	if values["default_group"] != "user" {
		t.Fatalf("register.default_group = %v, want user", values["default_group"])
	}
	if values["email_domain_control_enabled"] != false || values["email_verification_enabled"] != false {
		t.Fatalf("register flags = %v, want false", values)
	}
	if values["allowed_email_domains"] != "" {
		t.Fatalf("register.allowed_email_domains = %v, want empty", values["allowed_email_domains"])
	}
}

func TestSettingsAdminPatchAuthRoundtrip(t *testing.T) {
	db := testDB(t)
	resetGroups(t, db, "auth")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	resp := doJSON(t, owner, http.MethodGet, ts.URL+"/api/v1/admin/settings/auth", nil)
	expectStatus(t, resp, http.StatusOK)
	group, values := decodeGroup(t, resp)
	if group != "auth" {
		t.Fatalf("group = %q, want auth", group)
	}
	if len(values) != 9 {
		t.Fatalf("auth values = %v, want the nine defaults", values)
	}
	if values["session_max_age_days"] != float64(7) {
		t.Fatalf("auth.session_max_age_days = %v, want 7", values["session_max_age_days"])
	}
	if values["password_min_length"] != float64(8) {
		t.Fatalf("auth.password_min_length = %v, want 8", values["password_min_length"])
	}
	if values["captcha_provider"] != "" {
		t.Fatalf("auth.captcha_provider = %v, want empty", values["captcha_provider"])
	}

	patch := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/auth", map[string]any{"session_max_age_days": 14})
	expectStatus(t, patch, http.StatusOK)
	_, values = decodeGroup(t, patch)
	if values["session_max_age_days"] != float64(14) {
		t.Fatalf("patched session_max_age_days = %v, want 14", values["session_max_age_days"])
	}
	if values["password_min_length"] != float64(8) {
		t.Fatalf("untouched password_min_length = %v, want 8", values["password_min_length"])
	}

	after := doJSON(t, owner, http.MethodGet, ts.URL+"/api/v1/admin/settings/auth", nil)
	expectStatus(t, after, http.StatusOK)
	_, values = decodeGroup(t, after)
	if values["session_max_age_days"] != float64(14) {
		t.Fatalf("readback session_max_age_days = %v, want 14", values["session_max_age_days"])
	}
}
```

- [ ] **Step 3: 跑测试确认红**

Run: `export PATH="/f/software/go/bin:$PATH" && cd F:/protect/NeoBlog/backend && go test ./internal/modules/settings/ -run 'TestBuiltin' -count=1 -v 2>&1 | tail -20`

Expected: `TestBuiltinRegistryShape` FAIL——「register items = 1, want 5」。

Run: `cd F:/protect/NeoBlog/backend && set -a && source .env && set +a && bash scripts/test.sh ./internal/modules/settings/ 2>&1 | tail -30`

Expected: 另有两条 FAIL——`TestSettingsAdminReadsRegisterDefaults`（values 只有 `enabled` 一项）、`TestSettingsAdminPatchAuthRoundtrip`（GET `/admin/settings/auth` 404）。如实留存失败输出；**集成用例若被静默 skip，停下报告，不许把 skip 当红。**

- [ ] **Step 4: 停在工作区**

不提交。`git status --short` 核对：只应新增这两个测试文件的改动。

---

### Task 2: 清单扩字段、加 `auth` 组（转绿）+ 变异自检

**Files:**
- Modify: `backend/internal/modules/settings/registry.go`

**Interfaces:**
- Consumes: Task 1 的期望值（register 5 项、auth 9 项、默认值逐项）
- Produces: `builtinRegistry` 含扩后的 `register` 组与新 `auth` 组（spec §4）

- [ ] **Step 1: 替换 `builtinRegistry`（整块替换：site 不动、register 扩 4 项、auth 追加）**

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

- [ ] **Step 2: 转绿**

Run: `export PATH="/f/software/go/bin:$PATH" && cd F:/protect/NeoBlog/backend && go test ./internal/modules/settings/ -run 'TestBuiltin' -count=1 -v 2>&1 | grep -E "PASS|FAIL|ok " | head -15`
Expected: 全 PASS。

Run: `cd F:/protect/NeoBlog/backend && set -a && source .env && set +a && bash scripts/test.sh ./internal/modules/settings/ 2>&1 | tail -8`
Expected: `ok website/backend/internal/modules/settings`（含两条新用例与全部既有用例）。

- [ ] **Step 3: 变异① `session_max_age_days` 默认值改 30**

把 `{Name: "session_max_age_days", Type: FieldTypeNumber, Default: int64(7)}` 改为 `int64(30)`。

Run: `go test ./internal/modules/settings/ -run 'TestBuiltin' -count=1 2>&1 | tail -6` → Expected: FAIL（「item session_max_age_days default = 30, want 7」）。
Run: `set -a && source .env && set +a && go test ./internal/modules/settings/ -run 'TestSettingsAdminPatchAuthRoundtrip' -count=1 2>&1 | tail -6` → Expected: FAIL（session_max_age_days = 30, want 7）。

改回 `int64(7)`，复跑两条转绿。

- [ ] **Step 4: 变异② `captcha_enabled` 误标 `Public`**

把 `{Name: "captcha_enabled", Type: FieldTypeBool, Default: false}` 改为 `..., Default: false, Public: true}`。

Run: `go test ./internal/modules/settings/ -run 'TestBuiltinPublicAllowlist' -count=1 2>&1 | tail -6` → Expected: FAIL（公开项 7 项 vs 6 项）。
Run: `set -a && source .env && set +a && go test ./internal/modules/settings/ -run 'TestSettingsPublicGetAnonymously' -count=1 2>&1 | tail -6` → Expected: FAIL（公开负载 3 组 vs 2 组）。

改回不带 `Public`，复跑转绿。

- [ ] **Step 5: 变异③ `default_group` 从 `register` 挪进 `auth`**

把 `{Name: "default_group", Type: FieldTypeString, Default: "user"}` 从 `register` 组挪到 `auth` 组。

Run: `go test ./internal/modules/settings/ -run 'TestBuiltin' -count=1 2>&1 | tail -6` → Expected: FAIL（register items = 4, want 5）。
Run: `set -a && source .env && set +a && go test ./internal/modules/settings/ -run 'TestSettingsAdminReadsRegisterDefaults' -count=1 2>&1 | tail -6` → Expected: FAIL（values 4 项 vs 5 项）。

改回 `register` 组，复跑转绿。

- [ ] **Step 6: 停在工作区**

不提交。`git status --short` 核对：三个后端文件（两个测试 + registry.go）有改动。

---

### Task 3: 文档同步 + 版本进位

**Files:**
- Modify: `backend/README.md`
- Modify: `docs/DEVLOG.md`
- Modify: `backend/internal/version/version.go`（脚本代劳）

- [ ] **Step 1: README 两处「当前清单项」**

第一处（目录对照表行，28 行附近）——把

`当前清单项 \`site\`（\`title\` / \`subtitle\` / \`description\` / \`keywords\` / \`url\` 五项，均文字、全公开）与 \`register/enabled\`，加设置见`

替换为：

`当前清单项 \`site\` 五项（全公开）、\`register\` 组五项（注册总闸 \`enabled\` 公开 + 注册策略四项）与 \`auth\` 组九项（密码 / 会话 / 两步验证 / 人机验证，均非公开），加设置见`

第二处（设置章节「当前清单项」段，281 行附近）——把

`② \`register/enabled\`（开关，默认 \`true\`，已标 \`Public\`）——\`auth.Service.Register\` 的第一句读它，为 \`false\` 时注册返回 403 \`registration_closed\`；登录与 \`auth.CreateAdminUser\` 不受它影响。`

替换为：

`② \`register\` 组五项——\`enabled\`（开关，默认 \`true\`，已标 \`Public\`）是注册总闸：\`auth.Service.Register\` 的第一句读它，为 \`false\` 时注册返回 403 \`registration_closed\`，登录与 \`auth.CreateAdminUser\` 不受它影响；另四项注册策略（\`default_group\` 默认 \`"user"\`、\`email_domain_control_enabled\`、\`allowed_email_domains\`、\`email_verification_enabled\`）均非公开、纯数据、暂无消费方；③ \`auth\` 组九项——\`password_requirements_enabled\`、\`password_min_length\`（默认 \`8\`）、\`password_complexity\`、\`session_max_age_enabled\`、\`session_max_age_days\`（默认 \`7\`，单位天）、\`totp_enabled\`、\`password_reset_enabled\`、\`captcha_enabled\`、\`captcha_provider\`，均非公开、纯数据、暂无消费方。`

（该段其余句子与下方示例不动；`GET` 示例的「（节选）」注记维持现状。）

- [ ] **Step 2: DEVLOG 顶部插入条目**

插在 `## 2026-10-15 · 站点设置组 site（0.19.0）` 之前（倒序），并在新条目后补 `---`。条目全文：

```md
## 2026-10-15 · 注册与认证设置组（register 扩充 + auth 新增；纯数据）（0.20.0）

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
| ① | `session_max_age_days` 默认改 30 | {形状、auth 往返} |
| ② | `captcha_enabled` 误标 `Public` | {白名单、公开读} |
| ③ | `default_group` 挪去 `auth` | {形状、register 默认读} |

**已知缺口**：13 项全无消费方（实现接线是后续轮）；`default_group` / `captcha_provider` 无取值约束；`password_min_length` 无范围校验；控制台界面未做。

**提交状态**：待用户审查后按批提交。

---
```

变异表的「实测红」按 Task 2 的实际结果填写；与预期不符时如实写，并在汇报里点出。

- [ ] **Step 3: 版本进位**

Run: `cd F:/protect/NeoBlog/backend && bash scripts/bump-version.sh minor`
Expected: 两行输出——`version: 0.19.0 -> 0.20.0` 与 `remember: commit internal/version/version.go (and README.md) together with your change`；`grep "当前版本" README.md` 显示 `当前版本：0.20.0`。

- [ ] **Step 4: 停在工作区**

不提交。`git status --short` 核对恰为六个文件：两个测试 + `registry.go` + `README.md` + `DEVLOG.md` + `version.go`。

---

### Task 4: 全量验证

- [ ] **Step 1: 格式与静态检查**

Run: `export PATH="/f/software/go/bin:$PATH" && cd F:/protect/NeoBlog/backend && gofmt -l . && go vet ./... && go build ./...`
Expected: `gofmt -l` 无输出；`vet` / `build` 退出 0。

- [ ] **Step 2: 全量测试**

Run: `cd F:/protect/NeoBlog/backend && set -a && source .env && set +a && bash scripts/test.sh 2>&1 | tail -40`
Expected: 全仓 ok（串行 `-p 1`）。有任何红：停下汇报，不许继续。

- [ ] **Step 3: 活体（建议；公开面零变化的直接证据）**

1. 起临时实例：`cd backend && ADDR=:5733 go run .`（后台任务 / `Start-Process`，别用裸 `&`）
2. `curl -s http://127.0.0.1:5733/api/v1/public/settings` → 与本轮前**逐字一致**：`{"items":[{"group":"site",...五项空串...},{"group":"register","values":{"enabled":...}}]}`（两组、无 `auth`）。
3. （可选）`.scratch` 探针写读：照上一轮 `site-probe` 骨架，`SetNumber(ctx, "auth", "session_max_age_days", 14)` → 服务读取 → `ResetItem` 还原；或直接以 admin 测试结论为准，活体只做第 2 步。
4. 关闭临时实例。

- [ ] **Step 4: 汇总证据，不提交**

把全量测试结果、变异三条实测红、活体输出汇总进汇报；工作区保持未提交。

---

## 提交建议（仅在用户点名提交时执行）

| 批次 | 文件 | 建议 message |
| --- | --- | --- |
| ① | `registry.go` + 两个测试 | `feat(backend): 设置清单扩 register 组并新增 auth 组（注册与认证字段，纯数据）` |
| ② | spec + 计划两份文档 | `docs(backend): 注册与认证设置组 spec 与实施计划入库` |
| ③ | `README.md` + `docs/DEVLOG.md` | `docs(backend): 注册与认证设置组口径入 README 与 DEVLOG` |
| ④ | `version.go`（README 版本行随 ③，同既有口径） | `chore(backend): 版本进位 0.20.0（注册与认证设置组）` |

按文件清单暂存（严禁裸 `git add -A`），提交前 `git show --cached --stat` 核对只含本轮内容。

## 执行方式（待用户选）

- **Native**：在本会话按任务顺序执行（本轮改动小，建议此档）。
- **Subagent-driven**：每任务独立子代理 + 审查（最严，成本高）。
