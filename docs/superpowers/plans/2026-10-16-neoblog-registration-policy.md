# 注册域名控制与密码策略（register/auth 首批消费 + 设置写前校验插槽）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 接线四项设置——`register.email_domain_control_enabled` / `register.allowed_email_domains`（注册邮箱域名白名单）与 `auth.password_requirements_enabled` / `auth.password_min_length`（密码策略）；给设置机制加「组级写前校验」插槽承载两条保存时约束；新增两个错误码；版本进位（执行时现读的下一档）。

**Architecture:** 三块生产代码改动，依赖链 `apierror → settings → auth`：① apierror 加两哨兵；② settings 机制加 `Group.Validate` 插槽（`PatchGroup` 合并最终态后、落库前调用）+ 两个校验器 + 导出 `SplitListValue`；③ auth 的 `Register` / `ChangePassword` 读设置执行域名与密码策略。零迁移、零新路由、前端零改动。

**Tech Stack:** Go 1.27.2、ent、Gin（均不动）；测试走 `scripts/test.sh`（`-p 1` 串行）。

**Spec:** `docs/superpowers/specs/2026-10-16-neoblog-registration-policy-design.md`（本计划按该 spec 的 D1–D8、§4–§8 论证；两份一起读）

**修订记录**：
- v1（2026-10-16）：首版。
- v2（2026-10-16，独立审查后）：校验改「管辖项门槛」（Task 2）；上限 72 → 64（Task 2/3）；版本去写死 + 开工预检（Global Constraints、Task 4）；API 规范表 B 两行删除（Task 4）；测试夹具昵称缩短（Task 3）；变异预期修正并补 ⑥⑦（Task 2）；锚点统一到 `last_owner`（Task 1）。

## Global Constraints

- 零迁移、零新依赖、零新路由、零新权限点、前端零改动、**公开面零变化**（spec §3 不动清单）；`app.go` 不动（新码与新插槽都不需要装配变化）。
- 顺序：Task 1（apierror）→ Task 2（settings）→ Task 3（auth）→ Task 4（文档与版本）→ Task 5（全量验证）。Task 2 依赖 1，Task 3 依赖 1 与 2。
- 后端 Go 零人工注释、gofmt 缩进；测试失败消息沿用既有风格。
- **不主动提交**：全部改动留在工作区；提交与否与批次由用户点名（建议批次见文末）。
- 跑测试前 `export PATH="/f/software/go/bin:$PATH"`、`cd backend`，再 `set -a && source .env && set +a` 导入环境（`.env` 是裸赋值、不带 `export`，不加 `set -a` 子进程看不到 `DATABASE_URL_TEST`，集成用例会被静默 skip——后文所有 `source .env` 写法一律按此展开执行）；一次只跑一个测试进程；**不用 `-race`**（本机 cgo 不可用，会挂起）。部分 Run 行省略了这些前缀——同一 shell 内先执行一次即可。
- **开工预检**：动手前 `git status --short`——`backend/internal/modules/settings/*`、`backend/README.md`、`docs/DEVLOG.md` 若带非本轮的未提交改动（并行会话在制），**停下报告**；不硬改、不用「恰 N 个文件」自证。
- **版本不写死**：0.21.0 已被并行轮占用（未提交）；一切版本号（文档、DEVLOG、提交 message、脚本预期）按执行时现读 `internal/version/version.go` 的下一档。
- 不动的文件：`frontend/**`、ent schema / 迁移、`app.go`、`guard/**`、`scripts/*`；既有注册总闸四条用例（`register_gate_test.go`）不改。

## Review Focus

spec 隐含、最可能咬人的五条，逐条在对应任务里钉：

1. **校验只挂 `PatchGroup`，且有管辖项门槛**：无校验器或未触碰管辖项的 PATCH 不多读、行为逐字不变；`Set*` / `SetGroup` / `ResetItem` 直写不校验是口径（内部与探针通道，tracked 代码零调用），也正是「可摆脏存量」的通道——门槛专为脏存量不封锁无关写入而设（变异⑦钉它）。
2. **`param` = 判定项**：`setting_conflict` 的 param 是 `email_domain_control_enabled` / `password_min_length`（开关与数值项），**不是**请求里被写的清单项；清空清单分支同样指开关项——变异⑤钉它。
3. **合并态只用于校验**：写库仍只写请求里出现的项（部分更新语义）；校验在 `upsertMany` 之前——被拒时库里零变化、也不发生回读。
4. **密码关 = 现状逐字**（≥8、字母+数字、≤72 字节）；**开 = 只能更严**（夹 [8, 64]，下限与保存校验同值；上限留 8 字节余量）——变异②④钉它。
5. **集成用例清理纪律**：凡写 `register` / `auth` 组行的用例必须 `t.Cleanup` 复位——`-p 1` 只防跨包竞态，残留行照样毒害同库后续用例（残留 `email_domain_control_enabled=true` 会让所有后续注册吃 400，症状是「不相干用例红」）。
6. **版本与文件占用**：0.21.0 已被并行轮占用（未提交）；版本号一律执行时现读；settings 三个目标文件可能带外轮 hunk——按清单暂存、提交前看 diff，别把外轮收进本轮 message。

---

## 文件结构

| 文件 | 动作 | 唯一职责 | 任务 |
| --- | --- | --- | --- |
| `backend/internal/common/apierror/apierror.go` | Modify | 两哨兵 + `statusByCode` | Task 1 |
| `backend/internal/common/apierror/apierror_test.go` | Modify | 两条用例 + 计数 31 | Task 1 |
| `backend/internal/modules/settings/types.go` | Modify | `Group.ValidateOn/Validate`、`groupValidator`、`Registry.validators` | Task 2 |
| `backend/internal/modules/settings/registry.go` | Modify | 装载校验器（成对 + 管辖项存在性把关）+ 两校验器 + `SplitListValue` | Task 2 |
| `backend/internal/modules/settings/service.go` | Modify | `PatchGroup` 管辖项门槛 + 合并态 + 校验调用 | Task 2 |
| `backend/internal/modules/settings/controller.go` | Modify | `setting_conflict` 映射 | Task 2 |
| `backend/internal/modules/settings/registry_test.go` | Modify | 校验器与 `SplitListValue` 单测 | Task 2 |
| `backend/internal/modules/settings/controller_test.go` | Modify | 两条 PATCH 冲突集成用例 | Task 2 |
| `backend/internal/modules/auth/validate.go` | Modify | 密码策略与域名匹配纯函数 | Task 3 |
| `backend/internal/modules/auth/service.go` | Modify | `Register` / `ChangePassword` 接线 | Task 3 |
| `backend/internal/modules/auth/validate_test.go` | Modify | 三纯函数单测 + 签名同步 | Task 3 |
| `backend/internal/modules/auth/register_policy_test.go` | Create | 域名 / 密码集成用例 | Task 3 |
| `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md` | Modify | §7.3 两行 + §13.2 表 B 两行 | Task 4 |
| `backend/README.md` | Modify | 目录表行 + 「当前清单项」两处 | Task 4 |
| `docs/DEVLOG.md` | Modify | 顶部倒序条目 | Task 4 |
| `backend/internal/version/version.go` | Modify | 进位脚本代劳 | Task 4 |

---

### Task 1: apierror 两个新哨兵

**Files:**
- Modify: `backend/internal/common/apierror/apierror_test.go`
- Modify: `backend/internal/common/apierror/apierror.go`

**Interfaces:**
- Consumes: 既有哨兵表与 `TestRegistryIntegrity` / `TestRegistryMatchesSpec` 骨架
- Produces: `apierror.ErrEmailDomainNotAllowed`（400 / `invalid_request_error` / param `email`）与 `apierror.ErrSettingConflict`（409 / `conflict_error`）；供 Task 2、3 引用

- [ ] **Step 1: 测试先红——两条用例 + 计数 31**

`TestRegistryMatchesSpec` 的 `cases` 里加两条（`email_domain_not_allowed` 插在 `invalid_permission` 之后、`setting_conflict` 插在 `last_owner` 之后——哨兵块 / statusByCode / 用例表 / §7.3 表行四处锚点统一）：

```go
		{"email_domain_not_allowed", ErrEmailDomainNotAllowed, TypeInvalidRequest, "email_domain_not_allowed", http.StatusBadRequest, strPtr("email")},
		{"setting_conflict", ErrSettingConflict, TypeConflict, "setting_conflict", http.StatusConflict, nil},
```

`TestRegistryIntegrity`：`all` 切片在 `ErrInvalidPermission` 后加 `ErrEmailDomainNotAllowed`、在 `ErrLastOwner` 后加 `ErrSettingConflict`；两处硬编码计数 `29` 改 `31`（`:260`、`:263` 附近）。

- [ ] **Step 2: 跑测试确认红**

Run: `export PATH="/f/software/go/bin:$PATH" && cd F:/protect/NeoBlog/backend && go test ./internal/common/apierror/ -count=1 2>&1 | tail -6`

Expected: 编译失败（`undefined: ErrEmailDomainNotAllowed`）。

- [ ] **Step 3: 落哨兵与状态映射**

`apierror.go` 哨兵块：`ErrInvalidPermission` 之后插——

```go
	ErrEmailDomainNotAllowed   = &Error{Type: TypeInvalidRequest, Code: "email_domain_not_allowed", Param: strPtr("email")}
```

`ErrLastOwner` 之后插——

```go
	ErrSettingConflict         = &Error{Type: TypeConflict, Code: "setting_conflict"}
```

`statusByCode`：`"invalid_permission"` 之后插 `"email_domain_not_allowed": http.StatusBadRequest,`；`"last_owner"` 之后插 `"setting_conflict": http.StatusConflict,`。**对齐交给 gofmt，别手抠空格。**

- [ ] **Step 4: 转绿**

Run: `cd F:/protect/NeoBlog/backend && go test ./internal/common/apierror/ -count=1 2>&1 | tail -4`

Expected: `ok website/backend/internal/common/apierror`。

- [ ] **Step 5: 停在工作区**

不提交。`git status --short` 核对：只应有两个 apierror 文件的改动。

---

### Task 2: 设置写前校验插槽（转绿）+ 变异自检③⑤

**Files:**
- Modify: `backend/internal/modules/settings/registry_test.go`
- Modify: `backend/internal/modules/settings/controller_test.go`
- Modify: `backend/internal/modules/settings/types.go`
- Modify: `backend/internal/modules/settings/registry.go`
- Modify: `backend/internal/modules/settings/service.go`
- Modify: `backend/internal/modules/settings/controller.go`

**Interfaces:**
- Consumes: Task 1 的 `apierror.ErrSettingConflict`；既有助手 `testDB` / `resetGroups` / `newSettingsTestApp` / `seedClient` / `createUserInGroup` / `doJSON` / `expectStatus` / `decodeError` / `decodeGroup`
- Produces: `Group.ValidateOn` / `Group.Validate`、`groupValidator`、`Registry.validators`、`validatorApplies`、`validateRegisterGroup` / `validateAuthGroup` / `SplitListValue`（`SplitListValue` 供 Task 3 复用）；`PATCH /admin/settings/:group` 的组合冲突 409（仅命中管辖项时）

- [ ] **Step 1: 单测 + 集成用例先写（红）**

`registry_test.go`：import 加 `"errors"` 与 `"website/backend/internal/common/apierror"`；文件尾追加——

```go
func TestValidateRegisterGroup(t *testing.T) {
	cases := []struct {
		name   string
		values map[string]any
		want   string
	}{
		{"switch off", map[string]any{"email_domain_control_enabled": false, "allowed_email_domains": ""}, ""},
		{"switch on with list", map[string]any{"email_domain_control_enabled": true, "allowed_email_domains": "a.com, b.com"}, ""},
		{"switch on empty list", map[string]any{"email_domain_control_enabled": true, "allowed_email_domains": ""}, "email_domain_control_enabled"},
		{"switch on blank list", map[string]any{"email_domain_control_enabled": true, "allowed_email_domains": " , ，"}, "email_domain_control_enabled"},
	}
	for _, tc := range cases {
		err := validateRegisterGroup(tc.values)
		if tc.want == "" {
			if err != nil {
				t.Fatalf("%s: err = %v, want nil", tc.name, err)
			}
			continue
		}
		var itemErr *ItemError
		if !errors.As(err, &itemErr) || itemErr.Item != tc.want || !errors.Is(err, apierror.ErrSettingConflict) {
			t.Fatalf("%s: err = %v, want ItemError{%s, ErrSettingConflict}", tc.name, err, tc.want)
		}
	}
}

func TestValidateAuthGroup(t *testing.T) {
	for _, tc := range []struct {
		length  int64
		wantErr bool
	}{
		{8, false}, {16, false}, {64, false}, {7, true}, {0, true}, {-3, true}, {65, true},
	} {
		err := validateAuthGroup(map[string]any{"password_min_length": tc.length})
		if (err != nil) != tc.wantErr {
			t.Fatalf("length %d: err = %v, wantErr = %v", tc.length, err, tc.wantErr)
		}
	}
}

func TestSplitListValue(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"", []string{}},
		{" , ，", []string{}},
		{"a.com", []string{"a.com"}},
		{"a.com, b.com", []string{"a.com", "b.com"}},
		{"a.com，b.com", []string{"a.com", "b.com"}},
		{" a.com ,, b.com ", []string{"a.com", "b.com"}},
	}
	for _, tc := range cases {
		got := SplitListValue(tc.raw)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("SplitListValue(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}
```

（`SplitListValue` 返回非 nil 空切片——`make(…, 0, len(parts))` 的行为，`FieldsFunc` 对空串返回 nil、循环不追加。）

`controller_test.go` 追加两条集成用例：

```go
func TestSettingsPatchRegisterDomainControlRequiresList(t *testing.T) {
	db := testDB(t)
	resetGroups(t, db, "register")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	enable := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"email_domain_control_enabled": true})
	expectStatus(t, enable, http.StatusConflict)
	body := decodeError(t, enable)
	if body["code"] != "setting_conflict" || body["param"] != "email_domain_control_enabled" {
		t.Fatalf("conflict body = %v, want setting_conflict/email_domain_control_enabled", body)
	}

	set := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"allowed_email_domains": "example.com"})
	expectStatus(t, set, http.StatusOK)

	enable = doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"email_domain_control_enabled": true})
	expectStatus(t, enable, http.StatusOK)

	clear := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"allowed_email_domains": ""})
	expectStatus(t, clear, http.StatusConflict)
	body = decodeError(t, clear)
	if body["code"] != "setting_conflict" || body["param"] != "email_domain_control_enabled" {
		t.Fatalf("clear conflict body = %v, want setting_conflict/email_domain_control_enabled", body)
	}
}

func TestSettingsPatchAuthMinLengthRange(t *testing.T) {
	db := testDB(t)
	resetGroups(t, db, "auth")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	for _, length := range []int{3, 65} {
		resp := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/auth", map[string]any{"password_min_length": length})
		expectStatus(t, resp, http.StatusConflict)
		body := decodeError(t, resp)
		if body["code"] != "setting_conflict" || body["param"] != "password_min_length" {
			t.Fatalf("length %d: conflict body = %v", length, body)
		}
	}

	ok := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/auth", map[string]any{"password_min_length": 16})
	expectStatus(t, ok, http.StatusOK)
	_, values := decodeGroup(t, ok)
	if values["password_min_length"] != float64(16) {
		t.Fatalf("password_min_length = %v, want 16", values["password_min_length"])
	}
}

func TestSettingsPatchIgnoresDirtyStateOutsideValidateOn(t *testing.T) {
	db := testDB(t)
	resetGroups(t, db, "register")
	module, err := New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	if err := module.Service.SetBool(context.Background(), "register", "email_domain_control_enabled", true); err != nil {
		t.Fatalf("seed dirty state: %v", err)
	}
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	resp := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"enabled": false})
	expectStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	touch := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"email_domain_control_enabled": false})
	expectStatus(t, touch, http.StatusOK)
	touch.Body.Close()
}
```

- [ ] **Step 2: 跑测试确认红**

Run: `cd F:/protect/NeoBlog/backend && go test ./internal/modules/settings/ -run 'TestValidate|TestSplitListValue' -count=1 2>&1 | tail -8`

Expected: 编译失败（`undefined: validateRegisterGroup` 等）。同包编译失败会连带挡住集成用例——如实留存，Task 落实现后再分开跑。

- [ ] **Step 3: 落实现（四处）**

`types.go` 整块替换两个类型：

```go
type Group struct {
	Name       string
	Items      []Definition
	ValidateOn []string
	Validate   func(values map[string]any) error
}

type groupValidator struct {
	items []string
	check func(values map[string]any) error
}

type Registry struct {
	groups     map[string]map[string]Definition
	validators map[string]groupValidator
	order      []string
}
```

`registry.go`：import 加 `"strings"` 与 `"website/backend/internal/common/apierror"`；`compileRegistry` 初始化与登记——

```go
	registry := Registry{
		groups:     make(map[string]map[string]Definition, len(groups)),
		validators: make(map[string]groupValidator, len(groups)),
		order:      make([]string, 0, len(groups)),
	}
```

组循环内、`registry.order = append(...)` 之前插：

```go
			if group.Validate != nil {
				if len(group.ValidateOn) == 0 {
					return Registry{}, fmt.Errorf("settings: group %q has a validator without items", group.Name)
				}
				for _, name := range group.ValidateOn {
					if _, ok := items[name]; !ok {
						return Registry{}, fmt.Errorf("settings: group %q validator references unknown item %q", group.Name, name)
					}
				}
				registry.validators[group.Name] = groupValidator{items: group.ValidateOn, check: group.Validate}
			}
```

`builtinRegistry`：在 `Name: "register",` 下一行插 `ValidateOn: []string{"email_domain_control_enabled", "allowed_email_domains"},` 与 `Validate: validateRegisterGroup,` 两行；`Name: "auth",` 下一行插 `ValidateOn: []string{"password_min_length"},` 与 `Validate: validateAuthGroup,`。两组的 `Items` 与既有项**一字不动**（缩进交给 gofmt）。

文件尾追加（零注释）：

```go
const (
	passwordMinLengthFloor = 8
	passwordMinLengthCap   = 64
)

func validateRegisterGroup(values map[string]any) error {
	enabled, ok := values["email_domain_control_enabled"].(bool)
	if !ok {
		return fmt.Errorf("settings: email_domain_control_enabled is not a bool")
	}
	if !enabled {
		return nil
	}
	raw, ok := values["allowed_email_domains"].(string)
	if !ok {
		return fmt.Errorf("settings: allowed_email_domains is not a string")
	}
	if len(SplitListValue(raw)) > 0 {
		return nil
	}
	return &ItemError{Item: "email_domain_control_enabled", Err: apierror.ErrSettingConflict}
}

func validateAuthGroup(values map[string]any) error {
	length, ok := values["password_min_length"].(int64)
	if !ok {
		return fmt.Errorf("settings: password_min_length is not a number")
	}
	if length < passwordMinLengthFloor || length > passwordMinLengthCap {
		return &ItemError{Item: "password_min_length", Err: apierror.ErrSettingConflict}
	}
	return nil
}

func SplitListValue(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '，' })
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
```

`service.go` `PatchGroup`：`resolveGroupWrites` 与 `upsertMany` 之间插校验块；`validatorApplies` 与 `mergedGroupValues` 定义归位在 `resolveGroupWrites` 之后（零注释）：

```go
	if validator, ok := s.registry.validators[group]; ok && validatorApplies(validator, writes) {
		merged, err := s.mergedGroupValues(ctx, group, writes)
		if err != nil {
			return nil, err
		}
		if err := validator.check(merged); err != nil {
			return nil, err
		}
	}
```

```go
func validatorApplies(validator groupValidator, writes []ItemWrite) bool {
	for _, write := range writes {
		for _, item := range validator.items {
			if write.Name == item {
				return true
			}
		}
	}
	return false
}
```

```go
func (s *Service) mergedGroupValues(ctx context.Context, group string, items []ItemWrite) (map[string]any, error) {
	merged, err := s.readGroup(ctx, group, func(Definition) bool { return true })
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		merged[item.Name] = item.Value
	}
	return merged, nil
}
```

`controller.go` `writeSettingError` 的 `ItemError` 分支改为：

```go
	var itemErr *ItemError
	if errors.As(err, &itemErr) {
		if errors.Is(err, apierror.ErrSettingConflict) {
			apierror.Write(ctx, apierror.WithParam(apierror.ErrSettingConflict, itemErr.Item))
			return
		}
		apierror.Write(ctx, apierror.WithParam(apierror.ErrInvalidRequest, itemErr.Item))
		return
	}
```

- [ ] **Step 4: 单测转绿**

Run: `cd F:/protect/NeoBlog/backend && go test ./internal/modules/settings/ -run 'TestValidate|TestSplitListValue' -count=1 -v 2>&1 | grep -E "^(=== RUN|--- (PASS|FAIL)|PASS|FAIL|ok )" | tail -12`

Expected: 三条全 PASS。

- [ ] **Step 5: 集成转绿（含全部既有用例）**

Run: `cd F:/protect/NeoBlog/backend && set -a && source .env && set +a && bash scripts/test.sh ./internal/modules/settings/ 2>&1 | tail -8`

Expected: `ok website/backend/internal/modules/settings`（三条新用例 + 全部既有用例；若集成被静默 skip，停下报告，不许把 skip 当绿）。

- [ ] **Step 6: 变异③——`validateRegisterGroup` 恒返回 nil**

函数体首行插 `return nil`。

Run: `go test ./internal/modules/settings/ -run 'TestSettingsPatchRegisterDomainControlRequiresList' -count=1 2>&1 | tail -6` → Expected: FAIL（status = 200, want 409）。

改回，复跑转绿。

- [ ] **Step 7: 变异⑤——合并态不读存量**

`mergedGroupValues` 临时改为 `merged := map[string]any{}` 起步（不读存量）。

Run: `set -a && source .env && set +a && go test ./internal/modules/settings/ -run 'TestSettingsPatchRegisterDomainControlRequiresList' -count=1 2>&1 | tail -6` → Expected: FAIL（**首个 PATCH 即红**：合并态缺 `allowed_email_domains` 键，校验器类型断言失败 → 500 `internal_error`，而非预期的 409）。

改回 `s.readGroup(...)` 起步，复跑转绿。

- [ ] **Step 8: 变异⑥——校验器不装载**

注释掉 `compileRegistry` 里 `registry.validators[group.Name] = …` 一行。

Run: `set -a && source .env && set +a && go test ./internal/modules/settings/ -run 'TestSettingsPatchRegisterDomainControlRequiresList|TestSettingsPatchAuthMinLengthRange' -count=1 2>&1 | tail -6` → Expected: 两条 FAIL（写入顺利通过，200）。

改回，复跑转绿。

- [ ] **Step 9: 变异⑦——管辖项门槛恒真**

`validatorApplies` 首行插 `return true`。

Run: `set -a && source .env && set +a && go test ./internal/modules/settings/ -run 'TestSettingsPatchIgnoresDirtyStateOutsideValidateOn' -count=1 2>&1 | tail -6` → Expected: FAIL（`{"enabled": false}` 被 409 拦，want 200）。

改回，复跑转绿。

- [ ] **Step 10: 停在工作区**

不提交。`git status --short` 核对：恰六个 settings 文件（两个测试 + types / registry / service / controller）。

---

### Task 3: auth 消费（转绿）+ 变异自检①②④

**Files:**
- Modify: `backend/internal/modules/auth/validate_test.go`
- Modify: `backend/internal/modules/auth/register_policy_test.go`（新建）
- Modify: `backend/internal/modules/auth/validate.go`
- Modify: `backend/internal/modules/auth/service.go`

**Interfaces:**
- Consumes: Task 1 的 `apierror.ErrEmailDomainNotAllowed`；Task 2 的 `settings.SplitListValue`；既有助手 `testDB` / `uniqueTag` / `newTestAppWithSessionsAndSettings` / `testClient` / `postJSON` / `decodeError` / `cleanupUserByEmail` / `userCount` / `createTestUser`（签名现读现引）
- Produces: `validPassword(password, minLength)`、`emailDomainAllowed`、`effectivePasswordMinLength`（纯函数）；`Service.passwordMinLength` / `Service.ensureRegistrationDomain`

- [ ] **Step 1: 单测 + 集成用例先写（红）**

`validate_test.go` 追加三条（既有 `validPassword` 用例同步改新签名，传 `passwordBaselineMinLength` 保持原断言）：

```go
func TestValidPasswordMinLength(t *testing.T) {
	cases := []struct {
		password  string
		minLength int
		want      bool
	}{
		{"pass1234", 8, true},
		{"pass1234", 9, false},
		{"pass12345678", 11, true},
		{"pass12345678", 12, true},
		{"pass12345678", 13, false},
	}
	for _, tc := range cases {
		if got := validPassword(tc.password, tc.minLength); got != tc.want {
			t.Fatalf("validPassword(%q, %d) = %v, want %v", tc.password, tc.minLength, got, tc.want)
		}
	}
}

func TestEmailDomainAllowed(t *testing.T) {
	allowed := []string{"example.com", "sub.example.org"}
	cases := []struct {
		email string
		want  bool
	}{
		{"a@example.com", true},
		{"a@EXAMPLE.COM", true},
		{"a@sub.example.com", false},
		{"a@example.org", false},
		{"a@sub.example.org", true},
		{"no-at", false},
	}
	for _, tc := range cases {
		if got := emailDomainAllowed(tc.email, allowed); got != tc.want {
			t.Fatalf("emailDomainAllowed(%q) = %v, want %v", tc.email, got, tc.want)
		}
	}
}

func TestEffectivePasswordMinLength(t *testing.T) {
	cases := []struct {
		enabled    bool
		configured int64
		want       int
	}{
		{false, 0, 8},
		{false, 16, 8},
		{true, 3, 8},
		{true, 7, 8},
		{true, 8, 8},
		{true, 16, 16},
		{true, 64, 64},
		{true, 100, 64},
	}
	for _, tc := range cases {
		if got := effectivePasswordMinLength(tc.enabled, tc.configured); got != tc.want {
			t.Fatalf("effectivePasswordMinLength(%v, %d) = %d, want %d", tc.enabled, tc.configured, got, tc.want)
		}
	}
}
```

`register_policy_test.go`（新建）——助手 + 三用例：

```go
package auth

import (
	"context"
	"net/http"
	"testing"

	"website/backend/internal/modules/settings"
)

func enableDomainControl(t *testing.T, svc *settings.Service, domains string) {
	t.Helper()
	ctx := context.Background()
	if err := svc.SetString(ctx, "register", "allowed_email_domains", domains); err != nil {
		t.Fatalf("set allowed_email_domains: %v", err)
	}
	if err := svc.SetBool(ctx, "register", "email_domain_control_enabled", true); err != nil {
		t.Fatalf("set email_domain_control_enabled: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ResetItem(ctx, "register", "email_domain_control_enabled"); err != nil {
			t.Fatalf("reset email_domain_control_enabled: %v", err)
		}
		if err := svc.ResetItem(ctx, "register", "allowed_email_domains"); err != nil {
			t.Fatalf("reset allowed_email_domains: %v", err)
		}
	})
}

func enablePasswordRequirements(t *testing.T, svc *settings.Service, minLength int64) {
	t.Helper()
	ctx := context.Background()
	if err := svc.SetNumber(ctx, "auth", "password_min_length", minLength); err != nil {
		t.Fatalf("set password_min_length: %v", err)
	}
	if err := svc.SetBool(ctx, "auth", "password_requirements_enabled", true); err != nil {
		t.Fatalf("set password_requirements_enabled: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ResetItem(ctx, "auth", "password_requirements_enabled"); err != nil {
			t.Fatalf("reset password_requirements_enabled: %v", err)
		}
		if err := svc.ResetItem(ctx, "auth", "password_min_length"); err != nil {
			t.Fatalf("reset password_min_length: %v", err)
		}
	})
}

func TestRegisterDomainControlBlocksOtherDomains(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, _, settingsService := newTestAppWithSessionsAndSettings(t, db)
	enableDomainControl(t, settingsService, "example.com")

	blocked := "blocked+" + tag + "@gmail.com"
	before := userCount(t, db)
	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users", RegisterRequest{Email: blocked, Nickname: "blk" + tag, Password: "pass1234"})
	if resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("blocked status = %d, want 400", resp.StatusCode)
	}
	if code := decodeError(t, resp); code != "email_domain_not_allowed" {
		t.Fatalf("blocked code = %q, want email_domain_not_allowed", code)
	}
	if after := userCount(t, db); after != before {
		t.Fatalf("users = %d after blocked register, want %d", after, before)
	}

	cleanupUserByEmail(t, db, "ok+"+tag+"@example.com")
	allowed := postJSON(t, testClient(t), ts.URL+"/api/v1/users", RegisterRequest{Email: "ok+" + tag + "@Example.COM", Nickname: "ok" + tag, Password: "pass1234"})
	allowed.Body.Close()
	if allowed.StatusCode != http.StatusCreated {
		t.Fatalf("allowed status = %d, want 201", allowed.StatusCode)
	}
}

func TestRegisterPasswordPolicyMinLength(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, _, settingsService := newTestAppWithSessionsAndSettings(t, db)
	enablePasswordRequirements(t, settingsService, 10)

	weak := postJSON(t, testClient(t), ts.URL+"/api/v1/users", RegisterRequest{Email: "weak+" + tag + "@example.com", Nickname: "wk" + tag, Password: "pass12345"})
	if weak.StatusCode != http.StatusBadRequest {
		weak.Body.Close()
		t.Fatalf("weak status = %d, want 400", weak.StatusCode)
	}
	if code := decodeError(t, weak); code != "weak_password" {
		t.Fatalf("weak code = %q, want weak_password", code)
	}

	cleanupUserByEmail(t, db, "strong+"+tag+"@example.com")
	strong := postJSON(t, testClient(t), ts.URL+"/api/v1/users", RegisterRequest{Email: "strong+" + tag + "@example.com", Nickname: "str" + tag, Password: "pass123456"})
	strong.Body.Close()
	if strong.StatusCode != http.StatusCreated {
		t.Fatalf("strong status = %d, want 201", strong.StatusCode)
	}
}

func TestChangePasswordFollowsRequirementsSetting(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts, _, settingsService := newTestAppWithSessionsAndSettings(t, db)
	enablePasswordRequirements(t, settingsService, 10)

	client := testClient(t)
	login := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: password})
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", login.StatusCode)
	}

	weak := postJSON(t, client, ts.URL+"/api/v1/users/me/password", PasswordChangeRequest{CurrentPassword: password, NewPassword: "pass12345"})
	if weak.StatusCode != http.StatusBadRequest {
		weak.Body.Close()
		t.Fatalf("weak change status = %d, want 400", weak.StatusCode)
	}
	if code := decodeError(t, weak); code != "weak_password" {
		t.Fatalf("weak change code = %q, want weak_password", code)
	}

	ok := postJSON(t, client, ts.URL+"/api/v1/users/me/password", PasswordChangeRequest{CurrentPassword: password, NewPassword: "pass123456"})
	ok.Body.Close()
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("ok change status = %d, want 200", ok.StatusCode)
	}
}
```

（`userCount` 在 `register_gate_test.go`、`createTestUser` 在 `auth_test.go`，同包直接复用；`createTestUser` 返回的密码长度已满足默认策略。昵称前缀一律 ≤3 字：`uniqueTag` 是「PID-19 位纳秒」（6 位 PID 时 26 字），`blocked` 这类 7 字前缀会顶到 33 字 → 以 `invalid_request` 假红、排查被带偏。）

- [ ] **Step 2: 跑测试确认红**

Run: `cd F:/protect/NeoBlog/backend && go test ./internal/modules/auth/ -run 'TestValidPassword|TestEmailDomainAllowed|TestEffectivePasswordMinLength' -count=1 2>&1 | tail -8`

Expected: 编译失败（`too many arguments in call to validPassword` / `undefined: emailDomainAllowed`）。

- [ ] **Step 3: 落实现（两个文件）**

`validate.go`：`validPassword` 改签名；文件尾追加两纯函数与两个常量（零注释）：

```go
const (
	passwordBaselineMinLength = 8
	passwordMinLengthMax      = 64
)

func validPassword(password string, minLength int) bool {
	if utf8.RuneCountInString(password) < minLength || len(password) > maxPasswordBytes {
		return false
	}
	var hasLetter, hasDigit bool
	for _, r := range password {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	return hasLetter && hasDigit
}

func effectivePasswordMinLength(requirementsEnabled bool, configured int64) int {
	if !requirementsEnabled {
		return passwordBaselineMinLength
	}
	if configured < passwordBaselineMinLength {
		return passwordBaselineMinLength
	}
	if configured > passwordMinLengthMax {
		return passwordMinLengthMax
	}
	return int(configured)
}

func emailDomainAllowed(email string, allowed []string) bool {
	at := strings.LastIndexByte(email, '@')
	if at < 0 {
		return false
	}
	domain := email[at+1:]
	for _, entry := range allowed {
		if strings.EqualFold(entry, domain) {
			return true
		}
	}
	return false
}
```

`service.go`：`Register` 里把 `if !validPassword(password) { … }` 整块替换为——

```go
	minLength, err := s.passwordMinLength(ctx)
	if err != nil {
		return User{}, err
	}
	if !validPassword(password, minLength) {
		return User{}, apierror.ErrWeakPassword
	}

	if err := s.ensureRegistrationDomain(ctx, email); err != nil {
		return User{}, err
	}
```

`ChangePassword` 里把 `if !validPassword(newPassword) { … }` 替换为——

```go
	minLength, err := s.passwordMinLength(ctx)
	if err != nil {
		return err
	}
	if !validPassword(newPassword, minLength) {
		return apierror.ErrWeakPassword
	}
```

`service.go` 文件尾追加两个方法（零注释）：

```go
func (s *Service) passwordMinLength(ctx context.Context) (int, error) {
	requirementsEnabled, err := s.settings.GetBool(ctx, "auth", "password_requirements_enabled")
	if err != nil {
		return 0, err
	}
	configured := int64(0)
	if requirementsEnabled {
		configured, err = s.settings.GetNumber(ctx, "auth", "password_min_length")
		if err != nil {
			return 0, err
		}
	}
	return effectivePasswordMinLength(requirementsEnabled, configured), nil
}

func (s *Service) ensureRegistrationDomain(ctx context.Context, email string) error {
	controlEnabled, err := s.settings.GetBool(ctx, "register", "email_domain_control_enabled")
	if err != nil {
		return err
	}
	if !controlEnabled {
		return nil
	}
	raw, err := s.settings.GetString(ctx, "register", "allowed_email_domains")
	if err != nil {
		return err
	}
	allowed := settings.SplitListValue(raw)
	if len(allowed) == 0 {
		return nil
	}
	if emailDomainAllowed(email, allowed) {
		return nil
	}
	return apierror.ErrEmailDomainNotAllowed
}
```

- [ ] **Step 4: 单测转绿**

Run: `cd F:/protect/NeoBlog/backend && go test ./internal/modules/auth/ -run 'TestValidPassword|TestEmailDomainAllowed|TestEffectivePasswordMinLength' -count=1 -v 2>&1 | grep -E "^(--- (PASS|FAIL)|PASS|FAIL|ok )" | tail -8`

Expected: 全 PASS（含既有 `validPassword` 用例的新签名版本）。

- [ ] **Step 5: 集成转绿**

Run: `cd F:/protect/NeoBlog/backend && set -a && source .env && set +a && bash scripts/test.sh ./internal/modules/auth/ 2>&1 | tail -8`

Expected: `ok website/backend/internal/modules/auth`（三条新用例 + 全部既有用例）。

- [ ] **Step 6: 变异自检**

① `ensureRegistrationDomain` 函数体首行插 `return nil` → `go test ./internal/modules/auth/ -run 'TestRegisterDomainControlBlocksOtherDomains' -count=1` → Expected: FAIL（201 而非 400）；改回复绿。

② `effectivePasswordMinLength` 函数体首行插 `return passwordBaselineMinLength` → 跑 `TestRegisterPasswordPolicyMinLength` 与 `TestChangePasswordFollowsRequirementsSetting` → Expected: 两条 FAIL（9 位通过）；改回复绿。

④ `effectivePasswordMinLength` 的 `if configured < passwordBaselineMinLength` 一行改为 `if configured < 1` → 跑 `TestEffectivePasswordMinLength` → Expected: FAIL（3 → want 8）；改回复绿。

- [ ] **Step 7: 停在工作区**

不提交。`git status --short` 核对：恰四个 auth 文件——`validate.go` / `service.go` / `validate_test.go` / `register_policy_test.go`。

---

### Task 4: 文档同步 + 版本进位

**Files:**
- Modify: `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md`
- Modify: `backend/README.md`
- Modify: `docs/DEVLOG.md`
- Modify: `backend/internal/version/version.go`（脚本代劳）

**前置预检**：`git status --short`——`settings/*`、`README.md`、`DEVLOG.md`、`version.go` 若仍带并行轮的未提交改动，先停下报告（等用户拍入库顺序），不在这条状态下动文档。

- [ ] **Step 1: API 规范（现读现引，行号会漂）**

§7.3 前言句（`0.12.0 起新增 registration_closed …` 之后）续写（版本号按执行时现读取值，**不写死**）：

`；<执行时版本> 起新增 \`email_domain_not_allowed\`（注册邮箱域名白名单拒绝，400 / \`invalid_request_error\`，param 默认 \`email\`）与 \`setting_conflict\`（设置写前组合校验不满足，409 / \`conflict_error\`，param 由设置端点覆盖为判定项名）`

表格两行——`invalid_permission` 行之后插：

`| \`email_domain_not_allowed\` | 400 | \`invalid_request_error\` | \`email\` | 域名控制开启且清单条目非空时，注册邮箱域名不在清单里（<执行时版本> 新增） | 该邮箱域名不允许注册 |`

`last_owner` 行之后插：

`| \`setting_conflict\` | 409 | \`conflict_error\` | 设置端点覆盖为判定项名 | 设置写入后的最终组合不满足组级约束：开域名控制必须清单非空；\`password_min_length\` 取 8–64（<执行时版本> 新增）；param=判定项，未必是要改的那一项——域名规则下要补的是 \`allowed_email_domains\` | 该设置项与其他设置冲突 |`

**§13.2 表 B 不动**：它是 2026-10-11 冻结快照，历轮新码（`mail_delivery_failed` / `two_factor_*` / `method_not_allowed` / `registration_closed`）均只进 §7.3；本轮沿用惯例，不为新码加行。

- [ ] **Step 2: README 三处（现读现引，行号与引文若有出入以现读为准）**

目录对照表行（`当前清单项 …` 那行尾部）替换为：

`当前清单项 \`site\` 组六项（全公开）、\`register\` 组五项（\`enabled\` 公开；\`email_domain_control_enabled\` / \`allowed_email_domains\` 已有消费方）与 \`auth\` 组九项（\`password_requirements_enabled\` / \`password_min_length\` 已有消费方，其余非公开），加设置见「设置」章节配方`

「设置」章节「当前清单项」段 ② 里，把「另四项注册策略（…）均非公开、纯数据、暂无消费方；」替换为：

`\`email_domain_control_enabled\`（开关，默认 \`false\`）与 \`allowed_email_domains\`（文字，逗号分隔、中英文逗号都认）是注册邮箱域名白名单：开关开启且清单条目非空时 \`auth.Service.Register\` 拒绝域名不在清单里的邮箱（400 \`email_domain_not_allowed\`；大小写不敏感、精确匹配域名、不放行子域），只拦公开注册，\`CreateAdminUser\` 与 \`PATCH /users/me\` 不受影响；开启时清单必须非空，\`PATCH /admin/settings/register\` 拒绝不一致的组合（409 \`setting_conflict\`）；\`default_group\`（默认 \`"user"\`）与 \`email_verification_enabled\` 仍非公开、纯数据、暂无消费方；`

③ 句替换为：

`③ \`auth\` 组九项——\`password_requirements_enabled\`（开关，默认 \`false\`）与 \`password_min_length\`（默认 \`8\`，取值 8–64）是密码策略：开启后 \`POST /users\` 与 \`POST /users/me/password\` 按 \`password_min_length\` 校验（关闭时沿用写死的 8；字母+数字与 ≤72 字节恒生效），\`PATCH /admin/settings/auth\` 拒绝 8–64 之外的写入（409 \`setting_conflict\`）；\`password_complexity\`、\`session_max_age_enabled\`、\`session_max_age_days\`（默认 \`7\`，单位天）、\`totp_enabled\`、\`password_reset_enabled\`、\`captcha_enabled\`、\`captcha_provider\` 均非公开、纯数据、暂无消费方。`

「设置」章节若逐条列了 PATCH 的错误情形（404 / 400 …），补一条 409 `setting_conflict`「组合约束不满足」；`GET` / `PATCH` 示例段（节选注记）不动。

另在「怎么加一个公开设置」段之后补一段「怎么加一条组级保存校验」（照该段格式，放在 `## 运行` 之前），内容：

````markdown
**怎么加一条组级保存校验**（保存时拦住组合非法的写入）：给 `Group` 字面量同时加 `ValidateOn`（管辖项：只有改动这些项的 `PATCH` 才触发校验）与 `Validate`（拿到「存量 ⊕ 本次写入」的合并最终态；不合规返回 `*ItemError{Item: 判定项名, Err: apierror.ErrSettingConflict}`）：

```go
{
	Name:       "demo",
	ValidateOn: []string{"enabled", "list"},
	Validate:   validateDemoGroup,
	Items: []Definition{
		{Name: "enabled", Type: FieldTypeBool, Default: false},
		{Name: "list", Type: FieldTypeString, Default: ""},
	},
}
```

两者必须成对（只写 `Validate` 构造即报错，管辖项不存在也报错）。校验只覆盖 `PATCH /admin/settings/:group`（唯一 HTTP 写路径）；`Set*` / `SetGroup` / `ResetItem` 直写不校验（测试与探针通道）。冲突回 409 `setting_conflict`，`param` 指向**判定项**（未必是要改的那一项）；未触碰管辖项的写入不读合并态、零开销。
````

- [ ] **Step 3: DEVLOG 顶部插入条目**

插在 `## 2026-10-15 · 站点设置接入前台（元数据 / robots）+ 控制台页面标题（纯前端）` 之前（倒序），并在新条目后补 `---`。条目全文：

```md
## 2026-10-16 · 注册域名控制与密码策略接线（register/auth 首批消费 + 设置写前校验插槽）（<执行时版本>）

**版本**：后端 <现读> → **<下一档>**（minor；执行时现读——0.21.0 已被并行轮占用未提交）。零迁移、零新路由、零新依赖、前端零改动。

**动机**：register/auth 两组十四项此前是纯数据（唯一消费方是注册总闸），用户点名先接四项——邮箱域名控制 + 允许的邮箱域名、密码要求 + 密码最短长度；注册默认组明确搁置。spec：`docs/superpowers/specs/2026-10-16-neoblog-registration-policy-design.md`；计划：`docs/superpowers/plans/2026-10-16-neoblog-registration-policy.md`。

**口径（D1–D8 摘要）**：

- 域名白名单：大小写不敏感、逗号分隔（中英文逗号都认）、条目去空白、精确匹配域名（不放行子域）；只拦 `POST /users` 公开注册。
- 密码策略：开关关 = 现状逐字（≥8、字母+数字、≤72 字节）；开 = 取 `password_min_length` 并夹在 [8, 64]。「开着只会更严」。
- 保存时拦截：`PATCH /admin/settings/:group` 在「写入命中管辖项（`ValidateOn`）」时读合并最终态、落库前跑组级校验（机制新插槽 `Group.ValidateOn` / `Group.Validate`）；开域名控制必须清单条目非空；`password_min_length` 取 8–64；无关项写入不受脏存量牵连。
- 新错误码：`email_domain_not_allowed`（400）、`setting_conflict`（409，param=判定项名）。
- 公开面零变化（四项仍非公开）；`Set*` / `ResetItem` 内部接口不校验（生产零调用）。

**测试与变异自检**：单元（校验器 / `SplitListValue` / 域名匹配 / 密码策略纯函数）+ 集成（PATCH 冲突两例、注册域名两例、注册与改密密码策略三例；全部 `t.Cleanup` 复位）。变异五条：

| # | 变异 | 实测红 |
| --- | --- | --- |
| ① | 域名校验恒放行 | 域名集成 |
| ② | 密码策略恒 8 | 密码集成（注册 + 改密） |
| ③ | register 校验器恒 nil | PATCH 冲突集成 |
| ④ | 密码长度下限夹取去掉 | 纯函数单测 |
| ⑤ | 合并态不读存量 | PATCH 冲突集成的首个 PATCH（500，非 409） |
| ⑥ | 校验器不装载（compileRegistry 不登记） | 两条 PATCH 冲突集成 |
| ⑦ | 管辖项门槛恒真 | 门槛集成（无关写入被 409） |

**已知缺口**：注册页提示与本地校验仍写死 8（前端轮再跟，可能涉及 `password_min_length` 公开化）；新码前端未映射、暂落通用文案；域名控制不管改邮箱；复杂度不执行；无缓存（每次注册最多 5 次设置点查）。

**提交状态**：待用户审查后按批提交。

---
```

变异表的「实测红」按 Task 2 / 3 的实际结果填写；与预期不符时如实写，并在汇报里点出。

- [ ] **Step 4: 版本进位**

Run: `cd F:/protect/NeoBlog/backend && bash scripts/bump-version.sh minor`

Expected: 两行输出——`version: <执行时现读> -> <下一档>`（执行结果：用户裁定 `0.20.2 -> 0.20.3`，见文末「执行结果」节）与 `remember: commit internal/version/version.go (and README.md) together with your change`；`grep "当前版本" README.md` 显示进位后的版本。

- [ ] **Step 5: 停在工作区**

不提交。`git status --short` 核对恰为本轮 16 项：apierror 2 + settings 6 + auth 4 + 文档 3（API 规范 / `README.md` / `DEVLOG.md`）+ `version.go` 1。

---

### Task 5: 全量验证

- [ ] **Step 1: 格式与静态检查**

Run: `export PATH="/f/software/go/bin:$PATH" && cd F:/protect/NeoBlog/backend && gofmt -l . && go vet ./... && go build ./...`

Expected: `gofmt -l` 无输出；`vet` / `build` 退出 0。

- [ ] **Step 2: 全量测试**

Run: `cd F:/protect/NeoBlog/backend && set -a && source .env && set +a && bash scripts/test.sh 2>&1 | tail -40`

Expected: 全仓 ok（串行 `-p 1`）。有任何红：停下汇报，不许继续。

- [ ] **Step 3: 活体（建议）**

1. 起临时实例：`cd backend && ADDR=:5733 go run .`（后台任务 / `Start-Process`，别用裸 `&`）。
2. 登录拿 owner 会话后：`PATCH /api/v1/admin/settings/register` 带 `{"email_domain_control_enabled": true}`（清单空）→ 409 `setting_conflict`；补 `{"allowed_email_domains": "example.com"}` → 200；匿名 `POST /api/v1/users` 用 `@gmail.com` 邮箱 → 400 `email_domain_not_allowed`；`PATCH /api/v1/admin/settings/auth` 带 `{"password_min_length": 3}` → 409。测完 `ResetItem` 或直接删测试行还原（`register` / `auth` 组复位）。
3. 关闭临时实例。

- [ ] **Step 4: 汇总证据，不提交**

把全量测试结果、变异五条实测红、活体输出汇总进汇报；工作区保持未提交。

---

## 提交建议（仅在用户点名提交时执行）

| 批次 | 文件 | 建议 message |
| --- | --- | --- |
| ① | `apierror.go` + `apierror_test.go` | `feat(backend): apierror 增注册域名与设置冲突两个错误码` |
| ② | settings 四文件 + 两个测试文件 | `feat(backend): 设置机制加组级写前校验插槽（register/auth 组合约束）` |
| ③ | auth 四文件 | `feat(backend): 注册邮箱域名白名单与密码策略接线（register/auth 首批消费）` |
| ④ | spec + 计划两份文档 | `docs(backend): 注册域名控制与密码策略 spec 与实施计划入库` |
| ⑤ | `README.md` + API 规范 + `docs/DEVLOG.md` | `docs(backend): 注册策略口径入 README、API 规范与 DEVLOG` |
| ⑥ | `version.go`（README 版本行随 ⑤） | `chore(backend): 版本进位 <执行时>（注册域名控制与密码策略）` |

按文件清单暂存（严禁裸 `git add -A`），提交前 `git show --cached --stat` 核对只含本轮内容。

## 执行方式（待用户选）

- **Native**：在本会话按任务顺序执行（本轮改动集中、依赖链清晰，建议此档）。
- **Subagent-driven**：每任务独立子代理 + 审查（最严，成本高）。

## 执行结果（2026-10-16，native 执行）

- **版本**：用户直接设定 **0.20.2 → 0.20.3（patch）**，未走 0.21.0 线（并行轮 README 曾自称 0.21.0、其 `version.go` 实为 0.20.2，不回补）；`version.go` / README / DEVLOG / API 规范四处按 0.20.3 统一。
- **静态与全量**：`gofmt -l` 干净、`go vet` / `go build` 通过；`scripts/test.sh` 全仓绿（含三条新 settings 集成、三条新 auth 集成、apierror 哨兵计数 31）。
- **变异七条实测红**：① 域名恒放行 → 201 want 400；② 密码恒 8 → 注册 201 / 改密 200，want 400；③ 校验器恒 nil → 200 want 409；④ 下限夹取去掉 → 3 want 8；⑤ 合并态不读存量 → 500 want 409；⑥ 校验器不装载 → 两条 200 want 409；⑦ 门槛恒真 → 409 want 200。
- **过程发现**：计划里 `validPassword` 边界用例期望错一位（`pass12345678` 是 12 字符、min=12 应通过），实施时修正测试与计划（12 → true、13 → false）。
- **未做**：活体（需 owner 会话，交用户）；前端轮事项（注册页提示动态化、新码 `toFailure` 映射、`password_min_length` 是否公开化）。
- **提交**：按文末六批清单、待用户点名；工作区未提交。
