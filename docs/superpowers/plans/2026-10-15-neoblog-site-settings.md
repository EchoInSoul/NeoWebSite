# 站点设置组 site Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在设置清单里落地 `site` 组（`title` / `subtitle` / `description` / `keywords` / `url` 五项，均文字、默认空串、全公开），同步四处测试与两处文档，版本进位 0.19.0。

**Architecture:** 唯一生产代码改动是 `builtinRegistry` 加一个组（数据字面量）；机制、接口、表全部不动。测试先行：先更新 / 新增断言（红），再落清单（绿），最后逐条变异证明每条断言真的会咬人。

**Tech Stack:** Go 1.27.2、ent、Gin（均不动）；测试走 `scripts/test.sh`（`-p 1` 串行）。

**Spec:** `docs/superpowers/specs/2026-10-15-neoblog-site-settings-design.md`（本计划按该 spec 的 D1–D9、§4–§6 论证；两份一起读）

## Global Constraints

- 零迁移、零新依赖、零新路由、零新权限点、零前端改动（spec §3 不动清单）。
- 五项字面量逐字照抄 spec §4——名字 / 类型 / 默认值 / 公开性，一项不多、一项不少。
- 后端 Go 零人工注释、gofmt 缩进；测试里的失败消息沿用既有风格。
- **不主动提交**：全部改动留在工作区；提交与否与批次由用户点名（建议批次见文末）。
- 跑测试前 `export PATH="/f/software/go/bin:$PATH"`、`cd backend`，再 `set -a && source .env && set +a` 导入环境（**实测修正**：`.env` 是裸赋值、不带 `export`，不加 `set -a` 时子进程看不到 `DATABASE_URL_TEST`，集成用例会被静默 skip——后文所有 `source .env` 写法一律按此展开执行）；一次只跑一个测试进程；**不用 `-race`**（本机 cgo 不可用，会挂起）。部分 Run 行省略了这些前缀——同一 shell 内先执行一次即可，换新 shell 重新执行。
- 不动的文件：`controller.go` / `service.go` / `repository.go` / ent schema / `scripts/*` / `frontend/**`。
- 测试夹具组名 `site`（`sampleGroups` / `TestServicePublicSettings` 等，测试内部约定）**不改名**；README 配方示例换 `demo` 只联动 `recipeGroups()` 与 `TestRegistryRecipeShape`（Task 3）。

## Review Focus

spec 隐含、最可能咬人的五条，逐条在对应任务里钉：

1. **公开面从 1 组变 2 组**：任何「只有 register 一组」的假设都会失效；三处既有断言 + 一条新用例就是为此（Task 1）。
2. **组序 = 公开负载排列**（`site` 在前是刻意的）：换序要被断言咬住（Task 2 变异③）。
3. **一组多项时组内 `map` 遍历随机**：白名单测试不排序会偶发红（Task 1 Step 2）。
4. **默认空串必须回键而不是缺键**：公开读对五项逐一断言（Task 1 Step 3）。
5. **只加五项、不给写入路径加校验**：spec D7 / §8 插槽——不许「顺手」。

---

## 文件结构

| 文件 | 动作 | 唯一职责 |
| --- | --- | --- |
| `backend/internal/modules/settings/registry_test.go` | Modify | 公开白名单 + 形状断言（Task 1）；配方守卫联动（Task 3） |
| `backend/internal/modules/settings/controller_test.go` | Modify | 公开读断言更新 + 新端到端用例（Task 1） |
| `backend/internal/modules/settings/registry.go` | Modify | 清单加 `site` 组（Task 2，唯一生产代码） |
| `backend/README.md` | Modify | 「当前清单项」两处 + 示例注记（Task 3） |
| `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md` | Modify | 端点表 #23 示例加「（节选）」（Task 3） |
| `docs/DEVLOG.md` | Modify | 顶部倒序登记（Task 3） |
| `backend/internal/version/version.go` | Modify | 版本进位（Task 3，脚本代劳） |

Task 1 只改测试（此时全红）；Task 2 落清单（转绿 + 变异）；Task 3 文档、配方消歧与版本；Task 4 全量验证。

---

### Task 1: 四处测试先红

**Files:**
- Modify: `backend/internal/modules/settings/registry_test.go`
- Modify: `backend/internal/modules/settings/controller_test.go`

**Interfaces:**
- Consumes: 既有 `builtinRegistry` / `compileRegistry`、包内测试助手 `testDB` / `resetGroups` / `newSettingsTestApp` / `seedClient` / `createUserInGroup` / `doJSON` / `expectStatus` / `decodeGroup` / `decodePublicItems`
- Produces: 期望值（供 Task 2 的清单字面量对齐）——公开项 `register.enabled` + `site` 五项；公开负载组序 `site` → `register`；`site` 五项全空串默认

- [ ] **Step 1: `registry_test.go` 补 site 形状断言**

import 块改为：

```go
import (
	"reflect"
	"sort"
	"testing"
)
```

`TestBuiltinRegistryShape` 在现有 register 断言块之后追加：

```go
	siteItems, ok := reg.groups["site"]
	if !ok {
		t.Fatal("builtin registry missing the site group")
	}
	if len(siteItems) != 5 {
		t.Fatalf("site items = %d, want 5", len(siteItems))
	}
	for _, name := range []string{"title", "subtitle", "description", "keywords", "url"} {
		def, ok := siteItems[name]
		if !ok {
			t.Fatalf("builtin registry missing site.%s", name)
		}
		if def.Type != FieldTypeString || def.Default.(string) != "" {
			t.Fatalf("site.%s = %+v, want string default \"\"", name, def)
		}
		if !def.Public {
			t.Fatalf("site.%s must be Public, otherwise the frontend cannot read it", name)
		}
	}
```

- [ ] **Step 2: `TestBuiltinPublicAllowlist` 改排序比较（整函数替换）**

现写法逐组遍历组内 `map`，一组多项时顺序随机；改为排序后比较：

```go
func TestBuiltinPublicAllowlist(t *testing.T) {
	reg, err := compileRegistry(builtinRegistry)
	if err != nil {
		t.Fatalf("builtin registry rejected: %v", err)
	}

	var public []string
	for _, group := range reg.order {
		for name, def := range reg.groups[group] {
			if def.Public {
				public = append(public, group+"."+name)
			}
		}
	}
	sort.Strings(public)
	want := []string{
		"register.enabled",
		"site.description",
		"site.keywords",
		"site.subtitle",
		"site.title",
		"site.url",
	}
	if !reflect.DeepEqual(public, want) {
		t.Fatalf("public items = %v, want %v — marking a setting public exposes it anonymously", public, want)
	}
}
```

- [ ] **Step 3: `controller_test.go` 的 `TestSettingsPublicGetAnonymously` 更新（整函数替换）**

```go
func TestSettingsPublicGetAnonymously(t *testing.T) {
	db := testDB(t)
	resetGroups(t, db, "register", "site")
	ts := newSettingsTestApp(t, db)

	resp := doJSON(t, &http.Client{}, http.MethodGet, ts.URL+"/api/v1/public/settings", nil)
	expectStatus(t, resp, http.StatusOK)

	items := decodePublicItems(t, resp)
	if len(items) != 2 {
		t.Fatalf("public items = %v, want the site and register groups", items)
	}
	if items[0]["group"] != "site" {
		t.Fatalf("group = %v, want site first in registry order", items[0]["group"])
	}
	siteValues, ok := items[0]["values"].(map[string]any)
	if !ok {
		t.Fatalf("site values = %v, want an object", items[0]["values"])
	}
	if len(siteValues) != 5 {
		t.Fatalf("site values = %v, want exactly the five defaults", items[0]["values"])
	}
	for _, name := range []string{"title", "subtitle", "description", "keywords", "url"} {
		if siteValues[name] != "" {
			t.Fatalf("site.%s = %v, want the empty default", name, siteValues[name])
		}
	}
	if items[1]["group"] != "register" {
		t.Fatalf("group = %v, want register", items[1]["group"])
	}
	values, ok := items[1]["values"].(map[string]any)
	if !ok || values["enabled"] != true {
		t.Fatalf("values = %v, want enabled true merged from default", items[1]["values"])
	}
}
```

- [ ] **Step 4: 新增端到端用例 `TestSettingsPatchSiteTitleReadsBackPublicly`**

追加到 `controller_test.go`（放在 `TestSettingsPublicGetAnonymously` 之后）：

```go
func TestSettingsPatchSiteTitleReadsBackPublicly(t *testing.T) {
	db := testDB(t)
	resetGroups(t, db, "register", "site")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	resp := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/site", map[string]any{"title": "我的博客"})
	expectStatus(t, resp, http.StatusOK)
	group, values := decodeGroup(t, resp)
	if group != "site" {
		t.Fatalf("group = %q, want site", group)
	}
	if values["title"] != "我的博客" {
		t.Fatalf("site.title = %v, want the written title", values["title"])
	}
	if values["subtitle"] != "" {
		t.Fatalf("site.subtitle = %v, want the untouched empty default", values["subtitle"])
	}

	public := doJSON(t, &http.Client{}, http.MethodGet, ts.URL+"/api/v1/public/settings", nil)
	expectStatus(t, public, http.StatusOK)
	items := decodePublicItems(t, public)
	if len(items) != 2 || items[0]["group"] != "site" {
		t.Fatalf("public items = %v, want site first then register", items)
	}
	siteValues, ok := items[0]["values"].(map[string]any)
	if !ok || siteValues["title"] != "我的博客" {
		t.Fatalf("public site values = %v, want the written title", items[0]["values"])
	}
}
```

- [ ] **Step 5: 跑测试确认红**

Run: `export PATH="/f/software/go/bin:$PATH" && cd F:/protect/NeoBlog/backend && go test ./internal/modules/settings/ -run 'TestBuiltin' -count=1 -v`

Expected: `TestBuiltinRegistryShape`（"missing the site group"）与 `TestBuiltinPublicAllowlist`（公开项只有 1 项）FAIL。

Run: `cd F:/protect/NeoBlog/backend && source .env && bash scripts/test.sh ./internal/modules/settings/`

Expected: 另有两条 FAIL——`TestSettingsPublicGetAnonymously`（items = 1 组）与 `TestSettingsPatchSiteTitleReadsBackPublicly`（PATCH 404 `not_found`）。如实留存失败输出备 DEVLOG / 汇报用；**若集成用例被静默 skip（环境缺 `DATABASE_URL_TEST`），停下报告，不许把 skip 当红。**

- [ ] **Step 6: 停在工作区**

不提交。`git status --short` 核对：只应新增这两个测试文件的改动。

---

### Task 2: 清单加 `site` 组（转绿）+ 变异自检

**Files:**
- Modify: `backend/internal/modules/settings/registry.go`

**Interfaces:**
- Consumes: Task 1 的期望值（六项公开、组序 site → register、五项空串默认）
- Produces: `builtinRegistry` 含 `site` 组；公开负载新形状（spec §4）

- [ ] **Step 1: 替换 `builtinRegistry`（整块替换，site 在前）**

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
		},
	},
}
```

- [ ] **Step 2: 转绿**

Run: `export PATH="/f/software/go/bin:$PATH" && cd F:/protect/NeoBlog/backend && go test ./internal/modules/settings/ -run 'TestBuiltin' -count=1 -v`
Expected: 全 PASS。

Run: `cd F:/protect/NeoBlog/backend && source .env && bash scripts/test.sh ./internal/modules/settings/`
Expected: 全 PASS（含四条相关用例；其余集成用例照常）。

- [ ] **Step 3: 变异① `site.url` 取消 `Public`**

把 `{Name: "url", ..., Public: true}` 的 `Public: true` 删掉（只删这一处）。

Run: `go test ./internal/modules/settings/ -run 'TestBuiltin' -count=1` → Expected: FAIL（白名单六项变五项；形状断言 url）。
Run: `source .env && go test ./internal/modules/settings/ -run 'TestSettingsPublicGetAnonymously' -count=1` → Expected: FAIL（site values 只剩四项）。

改回 `Public: true`，复跑两条转绿。

- [ ] **Step 4: 变异② `site.title` 默认值改非空串**

把 `Default: ""` 改为 `Default: "NeoBlog"`（只改 title 一行）。

Run: `go test ./internal/modules/settings/ -run 'TestBuiltin' -count=1` → Expected: FAIL（形状断言默认值）。
Run: `source .env && go test ./internal/modules/settings/ -run 'TestSettingsPublicGetAnonymously' -count=1` → Expected: FAIL（title 非空）。

改回 `Default: ""`，复跑转绿。

- [ ] **Step 5: 变异③ 两组互换顺序（register 在前）**

把 `register` 组字面量挪到 `site` 之前。

Run: `source .env && go test ./internal/modules/settings/ -run 'TestSettingsPublicGetAnonymously|TestSettingsPatchSiteTitleReadsBackPublicly' -count=1` → Expected: FAIL（组序断言）。
Run: `go test ./internal/modules/settings/ -run 'TestBuiltinPublicAllowlist' -count=1` → Expected: **PASS**（排序使白名单不敏感于组序——证排序生效）。

改回 site 在前，复跑转绿。

- [ ] **Step 6: 停在工作区**

不提交。`git status --short` 核对：三个后端文件（两个测试 + registry.go）有改动。

---

### Task 3: 文档同步（README / API 规范 / DEVLOG）+ 配方消歧 + 版本进位

**Files:**
- Modify: `backend/README.md`
- Modify: `backend/internal/modules/settings/registry_test.go`（配方守卫联动）
- Modify: `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md`
- Modify: `docs/DEVLOG.md`
- Modify: `backend/internal/version/version.go`（脚本代劳）

- [ ] **Step 1: README 四处更新**

**① 目录对照表行（28 行附近）**——把句尾的

`当前清单项 \`register/enabled\`，加设置见「设置」章节配方`

替换为：

`当前清单项 \`site\`（\`title\` / \`subtitle\` / \`description\` / \`keywords\` / \`url\` 五项，均文字、全公开）与 \`register/enabled\`，加设置见「设置」章节配方`

**② 设置章节「当前清单项」段（281 行附近）**——把段首的

`**当前清单项**：\`register/enabled\`（开关，默认 \`true\`，已标 \`Public\`）——`

替换为：

`**当前清单项**：① \`site\` 组五项——\`title\` / \`subtitle\` / \`description\` / \`keywords\` / \`url\`，均为文字、默认空串、全部标 \`Public\`（供前台读站名与 SEO 元数据；\`keywords\` 存英文逗号分隔的单个字符串；后端暂无消费方）；② \`register/enabled\`（开关，默认 \`true\`，已标 \`Public\`）——`

（该段其余句子不动。）

**③ 公开负载示例（284 行附近的代码块）**——块内容不动，在代码块与「**怎么加一个设置**」之间补一行：

`（上例 \`GET\` 响应为节选——本轮起完整响应还含 \`site\` 组。）`

**④ 「怎么加一个设置」配方示例换中性组名**——该代码块里 `Name: "site",` 改为 `Name: "demo",`（块内 title / port / maintenance 三项与其余文字不动）。④ 与 Step 2 必须同批完成——配方与守卫是同一份约定的两个副本。

- [ ] **Step 2: `registry_test.go` 配方守卫联动**

`recipeGroups()` 里 `Name: "site",` 改为 `Name: "demo",`；`TestRegistryRecipeShape` 里 `reg.groups["site"]["port"]` 改为 `reg.groups["demo"]["port"]`。（`TestBuiltinRegistryShape` / `TestBuiltinPublicAllowlist` 已在 Task 1 改过，别重复动。）

Run: `go test ./internal/modules/settings/ -run 'TestRegistryRecipeShape' -count=1 -v`
Expected: PASS（守卫仍绿——配方与示例仍逐字对应）。

- [ ] **Step 3: API 规范 #23 示例加「（节选）」**

`docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md` 端点表第 23 行里的

`形如 \`{"items":[{"group":"register","values":{"enabled":false}}]}\``

改为

`形如 \`{"items":[{"group":"register","values":{"enabled":false}}]}\`（节选）`

（该行其余描述不动。）

- [ ] **Step 4: DEVLOG 顶部插入条目**

插在 `## 2026-10-15 · 用户简介 user_bio（0.18.0）` 之前（倒序；**并行会话的侧栏条目保持不动**），并在新条目后补 `---`。条目全文：

```md
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

**测试与变异自检**：两处既有断言随清单改动被迫更新（公开白名单、匿名公开读），另补形状断言与一条「管理写 → 公开读」端到端用例。变异自检三条：

| # | 变异 | 实测红 |
| --- | --- | --- |
| ① | `site.url` 取消 `Public` | {白名单、形状、公开读} |
| ② | `site.title` 默认值改非空串 | {形状、公开读} |
| ③ | 两组换序 | {公开读、端到端}（白名单保持绿——证排序生效） |

**已知缺口**：五项后端暂无消费方（前端未接线，界面不变）；无长度校验；空串默认 = 未配置、前台自行兜底；`site.url` 属「每个部署一份」的值。

**提交状态**：全部改动（代码 / 测试 / README / API 规范示例标注 / 本条目 / 版本）与 spec / 计划两份文档均未提交（用户口径：不要擅自提交）。
```

变异表的「实测红」按 Task 2 的实际结果填写；与预期不符时如实写，并在汇报里点出。

- [ ] **Step 5: 版本进位**

Run: `cd F:/protect/NeoBlog/backend && bash scripts/bump-version.sh minor`
Expected: 两行输出——`version: 0.18.0 -> 0.19.0` 与 `remember: commit internal/version/version.go (and README.md) together with your change`；`grep "当前版本" README.md` 显示 `当前版本：0.19.0`。

- [ ] **Step 6: 停在工作区**

不提交。`git status --short` 核对恰为七个文件：`registry.go`、两个测试、`README.md`、API 规范文档、`DEVLOG.md`、`version.go`（外加用户既有的前端 / DEVLOG 并行改动，不动它们）。

---

### Task 4: 全量验证

- [ ] **Step 1: 格式与静态检查**

Run: `export PATH="/f/software/go/bin:$PATH" && cd F:/protect/NeoBlog/backend && gofmt -l . && go vet ./... && go build ./...`
Expected: `gofmt -l` 无输出；`vet` / `build` 退出 0。

- [ ] **Step 2: 全量测试**

Run: `cd F:/protect/NeoBlog/backend && source .env && bash scripts/test.sh`
Expected: 全仓 ok（串行 `-p 1`）。有任何红：停下汇报，不许继续。

- [ ] **Step 3: 活体（建议；用 `.scratch` 探针，不依赖管理员账号）**

1. 起临时实例（后台任务 / `Start-Process`，别用裸 `&`）：`cd backend && ADDR=:5733 go run .`
2. `curl -s http://127.0.0.1:5733/api/v1/public/settings` → `items` 含 `site` 组、五项空串。
3. 写探针：在 `backend/.scratch/site-probe/main.go` 建临时程序（骨架照 `backend/.scratch/regctl/main.go`；`.scratch/` 已被 gitignore，不入库）：

```go
package main

import (
	"context"
	"fmt"
	"os"

	"website/backend/internal/config"
	"website/backend/internal/database"
	"website/backend/internal/modules/settings"
)

func main() {
	action := "write"
	if len(os.Args) > 1 {
		action = os.Args[1]
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}

	ctx := context.Background()
	db, err := database.Open(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database:", err)
		os.Exit(1)
	}
	defer db.Close()

	module, err := settings.New(db)
	if err != nil {
		fmt.Fprintln(os.Stderr, "settings:", err)
		os.Exit(1)
	}

	switch action {
	case "write":
		if err := module.Service.SetString(ctx, "site", "title", "临时标题"); err != nil {
			fmt.Fprintln(os.Stderr, "write:", err)
			os.Exit(1)
		}
	case "reset":
		if err := module.Service.ResetItem(ctx, "site", "title"); err != nil {
			fmt.Fprintln(os.Stderr, "reset:", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: site-probe [write|reset]")
		os.Exit(2)
	}
	fmt.Println("ok:", action)
}
```

4. 写：Run: `cd backend && go run ./.scratch/site-probe write` → 打印 `ok: write`。
5. 公开回读：再 GET public → `site.title` = `临时标题`（其余四项仍空串）。
6. 还原：Run: `cd backend && go run ./.scratch/site-probe reset` → 再 GET → `site.title` 回空串（删行 = 恢复默认，与机制语义一致）；关闭临时实例。

说明：开发库没有 owner 账号，冒烟账号 `gate-check` 是普通用户、`PATCH /admin/settings/site` 必 403——写腿不用它；管理路由的「owner 写 → 公开读」链路由测试库的 `TestSettingsPatchSiteTitleReadsBackPublicly` 覆盖。

- [ ] **Step 4: 汇总证据，不提交**

把全量测试结果、变异三条实测红、活体输出（若做）汇总进汇报；工作区保持未提交。

---

## 提交建议（仅在用户点名提交时执行）

| 批次 | 文件 | 建议 message |
| --- | --- | --- |
| ① | `registry_test.go` + `controller_test.go` + `registry.go` | `feat(backend): 设置清单加 site 组（标题/副标题/描述/关键词/URL）` |
| ② | `README.md` + `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md` + `docs/DEVLOG.md` | `docs(backend): 站点设置组口径入 README 与 DEVLOG` |
| ③ | `version.go` + `README.md`（版本行） | `chore(backend): 版本进位 0.19.0（站点设置组）` |

⚠️ **`docs/DEVLOG.md` 目前混着并行会话的未提交条目（「2026-10-15 · 侧栏面板状态化（无动画）」等，约 +25 行、可能随并行会话变动）**：直接 `git add docs/DEVLOG.md` 会把它一起入库。批次②里该文件必须 **hunk 级暂存**（`git add -p docs/DEVLOG.md`，只取本轮新条目），提交前 `git show --cached --stat` 与 `git diff --cached` 核对只含本轮内容；或等并行条目先入库 / 撤回后再提交本条目。

其余照旧：按文件清单暂存（严禁裸 `git add -A`，避免卷入并行会话的前端改动），提交前 `git show --stat` 核对。

## 执行方式（待用户选）

- **Subagent-driven**：每个任务由独立子代理实现、独立审查后进下一个（最严，成本最高）。
- **Native**：在本会话按任务顺序实现，最后整体复查（最快，本轮改动小，建议此档）。
