# NeoBlog 后端工程风格规范化实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
> **本机说明（2026-10-20）:** 本机未安装上述两个执行 skill；执行时按本计划逐步直接实施，不调用不存在的 skill。

**Goal:** 把后端已有的「Go 语法 + NestJS 分层」风格写成可执行的规范，加一道静态检查门禁，并收敛三处会造成新模块写法分裂的内部结构差异；不改任何 API 形状。

**Architecture:** 三层落地。第一层 `backend/STYLE.md` 固化模块解剖、依赖方向、错误分层、DTO 归属、响应形状、命名、测试与文档流程；第二层 `scripts/check.sh` 把 gofmt / vet / build / whitespace 变成一条命令；第三层做三件行为等价的小重构——共享集合信封、把散在 controller 的 DTO 归位、让 access 的 repository 不再 import `apierror`。

**Tech Stack:** Go 1.27.2、Gin、ent 0.14.6、Git Bash、既有 `scripts/test.sh`（`go test -p 1` + 真库集成）。

**Spec:**
- 用户本轮裁定：规范以早期设计文档为准；API 形状保持现状；只做内部结构小重构。
- `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md`（HTTP 表达与目标态；D4/D7 等分页与时间戳尚未实施）
- `docs/superpowers/specs/2026-10-11-neoblog-auth-design.md`（五件套、零注释、repository 具体类型）
- `docs/superpowers/specs/2026-10-11-neoblog-access-design.md`（模块边界靠 DTO、共享 ent client）
- `docs/superpowers/specs/2026-10-11-neoblog-mail-design.md`（能力模块、controller 经注入的 Mailer）
- `docs/superpowers/specs/2026-10-11-neoblog-settings-design.md`（模块哨兵、构造自检）
- `docs/superpowers/specs/2026-10-11-neoblog-data-layer-design.md`（不隔仓储接口层）
- `backend/README.md`（硬性规则 1–8 与接口契约）

**执行状态（2026-10-20）:** 规范与检查（Task 1–2）已入库（`5c2cdeb`）。Task 3–5 已实施并通过全仓测试；Task 6 的 DEVLOG 与版本进位未执行，提交在本轮后续请求中执行。实际实现把 `ItemList` / `Deleted` / `DeletedBatch` / `Empty` 放在 `apiresponse/response.go`，并把 `health.Status` 改名 `HealthResponse`、access 的 `permissionResponse` 移入 `dto.go`；这些比计划多出的项已同步进 STYLE 与工作区。

## Global Constraints

- **API 形状零变化**：`items` 集合、删除确认体、`{}` 空体、201 创建、错误信封、时间字段、`/healthy` 负载全部保持现状。D4 的 `data/total/limit/offset`、epoch 时间戳、认证响应补 `id` 是目标态，本轮不实施。
- **行为零变化**：所有改动是移动/提取/错误翻译，不改业务判定与 HTTP 状态码。
- **零注释**：新增代码不写注释，例外仅 `//go:build`、`//go:embed` 等工具链指令。
- **gofmt + LF**：所有 Go 文件走 gofmt；仓库 `.gitattributes` 已固定 LF。
- **不新增依赖**：只使用标准库与现有依赖。
- **不碰生成代码**：`internal/database/ent/` 下的文件不改；ent schema 与迁移零变化。
- **测试口径**：全仓测试必须经 `bash scripts/test.sh`（包串行，消除注册开关竞态）；集成用例需要 `DATABASE_URL_TEST`，缺省时按既有约定 skip。
- **提交口径**：用户默认一次提交；实施完成并验证后先请用户确认，确认后只暂存本计划涉及的文件。不得把并行会话的 `frontend/*` 改动或 `docs/superpowers/{specs,plans}/2026-10-20-neoblog-auth-rate-limit*` 带入提交。
- **文档语言**：规范与说明用中文；Go 标识符用英文。
- **命令工作目录**：命令默认在仓库根执行；命令块内出现 `cd backend` 时，后续命令以 `backend/` 为工作目录。

## Review Focus

1. **空集合必须序列化为 `[]` 而不是 `null`**——共享信封要带自定义 `MarshalJSON`；Task 3 的测试同时覆盖 nil 与空切片。
2. **access 的 404 / 409 映射一条都不能丢**——错误从 repository 哨兵翻译回 `apierror` 时，每个原本返回 `ErrNotFound` / `ErrGroupNameTaken` / `ErrGroupNotEmpty` / `ErrLastOwner` 的路径都要保持；Task 5 的新表驱动测试逐条钉住，既有 service 测试再兜一层。
3. **DTO 搬家不能改 JSON tag**——只是移动类型，字段名、大小写、指针性一律不动；Task 4 依赖既有 controller 测试验证线上形状。
4. **跨包使用的导出类型不能被误降级**——`auth.User` / `auth.UserResponse` 被 `internal/app/app_test.go` 使用，必须保持导出；Task 4 以 `go build ./...` 与 app 测试兜底。
5. **检查脚本必须真的会红**——Task 2 用一个临时未格式化文件做负向验证；只验证「能通过」不算完成。

---

### Task 1: 写 `backend/STYLE.md` 并更新 README 硬性规则（已完成 2026-10-20）

**Files:**
- Create: `backend/STYLE.md`
- Modify: `backend/README.md:5`（开篇 NestJS 段落后加规范入口）
- Modify: `backend/README.md:35-36`（硬性规则 1 / 2）

**Interfaces:**
- Consumes: 无
- Produces: `backend/STYLE.md` 的规范条款；后续任务的实现与审查都以它为准。

- [ ] **Step 1: 写 `backend/STYLE.md`，按以下章节与条款成文**

必须包含的章节与硬性条款：

```text
# NeoBlog 后端工程风格

## 0. 地位与优先级
- 本文件约束 backend/ 的模块内部工程风格；HTTP 契约以 README 与 API 规范为准。
- 冲突裁决：早期设计文档的决策条款 > README > 现有实现中重复出现的写法。
- 标为「目标态」的条目尚未实施，不得据此改代码。

## 1. 模块解剖
- 有路由的模块必须自包含 module.go / controller.go / service.go / dto.go；需要 DB 时同目录 repository.go。
- 无路由的能力模块可省 controller / dto / module.go；不为凑形状建空文件。
- mail 是能力模块的明确例外：无 service.go，controller 直接调用注入的 Mailer。
- module.go 只做装配与路由注册，不写业务逻辑。
- 功能文件按职责拆（totp.go / session.go / validate.go / registry.go），单文件过大按关注点再拆。

## 2. 依赖方向与跨模块边界
- controller → service → repository；禁止反向调用，禁止跨层直连。
- repository 是具体类型，不抽仓储接口（数据层设计「不隔仓储接口层」）。
- 消费方接口只在消费方声明（如 health 的 pinger），不导出仓储接口。
- 模块间不互相 import 业务实现；跨模块能力通过 app.go 注入的 service 或能力接口。
- 共享表可由各模块自己的 repository 读写（access 读 users 是既有先例），但不得复制对方的业务规则。
- internal/database 不得 import 任何业务模块。
- Gin 与 HTTP 细节只允许出现在 controller、module.go、app、common/guard、common/apierror、common/apiresponse。

## 3. 错误分层
- 硬规则：repository 不 import apierror；只返回域哨兵或原始错误。
- service 负责把域错误翻译成 apierror；controller 只调用 apierror.Write，不现造 code。
- settings 例外：service 保留模块哨兵（ItemError 带项名上下文），controller 的 writeSettingError 负责映射；这是早期设置设计的既定分工。
- 新错误码先登记 apierror 注册表；错误码词汇只增不改义。
- 内部错误字符串小写、带包/域前缀；不得包含密码、token、恢复码、TOTP secret。
- 未知错误一律落到 internal_error；不泄漏 SQL、栈与内部路径。

## 4. DTO 与数据类型
- 线上请求 / 响应类型放 dto.go，命名 XxxRequest / XxxResponse。
- access 的 Group / UserSummary / UserDetail 是模块 DTO，保留现名；模块边界靠 DTO。
- 领域类型（无 JSON tag）可放 dto.go 或 service.go；不得为了复用给领域类型硬加 wire tag。
- service 可以返回领域类型或模块 DTO；controller 决定最终响应形状。
- 集合字段为空必须输出 []，标量可空输出 null。
- JSON 字段一律 snake_case；时间字段按当前 README 口径，不擅自切 epoch（目标态见 §5）。

## 5. 响应形状
- 当前生效：单资源裸对象；集合 {"items":[...]}；删除 {"id","deleted":true} 或批量 {"ids","deleted":true}；无负载 {}；创建 201。
- 不使用 204；无负载成功一律 200 + {}。
- 错误信封固定 {"error":{"type","code","param"}}，不含 message；HTTP 状态码按现有映射。
- 共享集合信封统一用 internal/common/apiresponse.ItemList[T]；nil 自动输出 []。
- 目标态（未实施）：列表 data/total/limit/offset + 分页、epoch 时间戳、/healthy 字段改名、认证响应补 id；不得在日常迭代或本轮提前改。
- /healthy 是探针端点，豁免错误信封。

## 6. 命名与代码组织
- 包名小写单数；文件按职责命名。
- 接收者：Module 用 m、controller 用 c、Service 用 s、repository 用 r。
- gin.Context 与 context.Context 都叫 ctx，靠类型区分；handler 参数用 ctx。
- 模块构造器 New；包内构造器 newXxx；只导出跨包使用的类型 / 函数。
- 导出例外：auth.User / auth.UserResponse（app_test 使用）、auth.CreateAdminUser（测试入口）、settings.New 返回 error、settings 的 Group / Definition / FieldType / ItemWrite / GroupValues / ItemError（清单与测试注入的既有接口）、mail 的 Mailer / Message / Body / Render / ErrClosed。
- import 分三组：标准库、第三方、本仓；由 gofmt 保证格式。
- 零注释；只有工具链指令。说明写 README / STYLE / spec / DEVLOG。
- 运行期日志用 log/slog，键值对；main.go 的启动引导是唯一标准库 log 例外，本轮不迁。
- 不记录密码、token、恢复码、TOTP secret、邮件正文。

## 7. 测试
- 测试与实现同包，白盒。
- 集成用例必须真库；DATABASE_URL_TEST 缺省 skip；全仓用 scripts/test.sh（-p 1）。
- helper 必须 t.Helper()；每个包自带 testDB / uniqueTag / createTestUser，不建共享 testutil。
- 判定表用表驱动；每个行为修复先写失败用例。
- 纯移动 / 重构必须保持现有行为测试全绿。

## 8. 文档与流程
- spec → plan → 实现 → DEVLOG；接口变化同步 README；版本用 scripts/bump-version.sh。
- 提交信息 type(scope): 中文描述；默认一次提交，用户要求再拆。
- 静态检查：bash scripts/check.sh（gofmt / go vet / go build / whitespace）。
- 测试：bash scripts/test.sh。

## 9. 存量例外表
| 现状 | 原因 | 处理 |
| --- | --- | --- |
| mail 无 service.go | 能力模块；controller 经注入的 Mailer 调能力 | 保留，按 §1 例外 |
| settings service 返回模块哨兵 | 哨兵携带项名上下文，供接口层映射 | 保留，按 §3 例外 |
| access service 返回带 JSON tag 的 DTO | 模块边界靠 DTO | 保留，按 §4 |
| main.go 用标准库 log | 启动引导只打几条生命周期日志 | 保留，按 §6 例外 |
| access 读 users 共享表 | 无 auth ↔ access 双向依赖；边界靠 DTO | 保留，按 §2 |
| 各模块复制 testDB / uniqueTag | 避免 testutil 反向耦合业务模块 | 保留，按 §7 |
| auth.CreateAdminUser 导出 | 仅供测试建号 | 保留，业务代码不得调用 |
| /healthy 不套错误信封 | 机器探针语义 | 保留，按 §5 |
```

- [ ] **Step 2: 在 README 开篇链接规范**

在 `backend/README.md` 第 5 行的 NestJS 段落后补一句：

```markdown
模块内部的工程风格（文件职责、错误分层、DTO 归属、命名与测试）见 [`STYLE.md`](STYLE.md)；本 README 继续只负责接口、环境与运维契约。
```

- [ ] **Step 3: 更新 README 硬性规则 1 / 2**

把规则 1 改成「有路由的模块必须自包含 module / controller / service / dto；无路由能力模块可省 controller / dto / module.go」，并把 mail 的无 service 例外写进规则 2：

```markdown
1. 一个功能一个模块目录 `internal/modules/<name>/`。**有路由的模块必须自包含 module / controller / service / dto；需要 DB 时同目录 `repository.go`。** 无路由的能力模块可省 controller / dto / module.go，只提供实现文件 + `New` 入口 + 测试，不要为凑形状建空文件。
2. 依赖方向单向：controller → service → repository（需要 DB 时同目录 `repository.go`）。禁止反向调用，禁止跨层直连。**例外：`mail` 是无 service 的能力模块，controller 可直接调用注入的 `Mailer`。**
```

规则 3 的 `apiresponse` 补充放在 Task 3（包落地之后），避免 README 提前引用不存在的包。

- [ ] **Step 4: 验证文档改动**

Run: `git diff --check`
Expected: 无输出（无空白错误）。

Run: `grep -n "STYLE.md" backend/README.md`
Expected: 至少一行命中开篇链接。

- [ ] **Step 5: 人工自检**

逐条对照本计划 Global Constraints：README 不得出现 `data/total`、epoch 时间戳等目标态被写成「当前生效」的表述。

---

### Task 2: 加 `scripts/check.sh` 静态检查门禁（已完成 2026-10-20）

**Files:**
- Create: `backend/scripts/check.sh`
- Modify: `backend/README.md`（「测试」章节，`scripts/test.sh` 说明之前或之后）

**Interfaces:**
- Consumes: 无
- Produces: `bash scripts/check.sh`，成功输出 `check ok`，失败以非零退出并列出问题文件。

- [ ] **Step 1: 写脚本**

`backend/scripts/check.sh` 全文：

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

unformatted="$(
  { git ls-files -z '*.go'; git ls-files --others --exclude-standard -z '*.go'; } |
    xargs -0 gofmt -l
)"
if [ -n "$unformatted" ]; then
  echo "gofmt needed:" >&2
  printf '%s\n' "$unformatted" >&2
  exit 1
fi

go vet ./...
go build ./...
git diff --check
git diff --cached --check

echo "check ok"
```

- [ ] **Step 2: 让脚本可执行**

Run: `chmod +x backend/scripts/check.sh`
Expected: 无输出。

- [ ] **Step 3: 正向验证**

Run: `bash backend/scripts/check.sh`
Expected: 末尾输出 `check ok`，退出码 0。

- [ ] **Step 4: 负向验证（必须真的会红）**

Run:

```bash
printf 'package internal\nfunc  broken( ){}\n' > backend/internal/zz_check_probe.go
bash backend/scripts/check.sh
```

Expected: 非零退出，输出 `gofmt needed:` 并列出 `internal/zz_check_probe.go`。

清理探针并复验：

```bash
rm backend/internal/zz_check_probe.go
bash backend/scripts/check.sh
```

Expected: `check ok`。

- [ ] **Step 5: README 测试章节补一句**

在 `backend/README.md` 的「测试」章节开头补：

```markdown
静态检查（格式 / vet / 编译 / 空白）用 `bash scripts/check.sh`；它不是测试，不会替代下面的全仓测试。
```

- [ ] **Step 6: 复验**

Run: `bash backend/scripts/check.sh`
Expected: `check ok`。

---

### Task 3: 提取共享集合信封 `apiresponse.ItemList[T]`（已完成 2026-10-20）

**Files:**
- Create: `backend/internal/common/apiresponse/list.go`
- Create: `backend/internal/common/apiresponse/list_test.go`
- Modify: `backend/internal/modules/auth/controller.go:20`（删本地 `itemList`）、`:283`（改用共享类型）
- Modify: `backend/internal/modules/access/controller.go:14`（删本地 `itemList`）、`:64`、`:73`、`:137`
- Modify: `backend/internal/modules/access/access_test.go:108`（测试 helper 改用共享类型）
- Modify: `backend/internal/modules/settings/controller.go:24-25`（删 `publicSettingsView`）、`:63`（改用共享类型）
- Modify: `backend/README.md:37`（硬性规则 3 的允许位置清单补 `internal/common/apiresponse`）

**Interfaces:**
- Consumes: 无
- Produces:
  - `apiresponse.ItemList[T any]`，字段 `Items []T`，JSON 键 `items`
  - 实现 `MarshalJSON`：`Items == nil` 时输出 `[]`
  - 线上形状恒为 `{"items":[...]}`，与当前完全一致

- [ ] **Step 1: 写失败测试**

`backend/internal/common/apiresponse/list_test.go`：

```go
package apiresponse

import (
	"encoding/json"
	"testing"
)

func TestItemListMarshalsItems(t *testing.T) {
	raw, err := json.Marshal(ItemList[int]{Items: []int{1, 2}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != `{"items":[1,2]}` {
		t.Fatalf("json = %s, want {\"items\":[1,2]}", raw)
	}
}

func TestItemListMarshalsNilAsEmptyArray(t *testing.T) {
	raw, err := json.Marshal(ItemList[string]{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != `{"items":[]}` {
		t.Fatalf("json = %s, want {\"items\":[]}", raw)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/common/apiresponse/`
Expected: 编译失败，提示 `undefined: ItemList`。

- [ ] **Step 3: 实现共享类型**

`backend/internal/common/apiresponse/list.go`：

```go
package apiresponse

import "encoding/json"

type ItemList[T any] struct {
	Items []T `json:"items"`
}

func (l ItemList[T]) MarshalJSON() ([]byte, error) {
	items := l.Items
	if items == nil {
		items = []T{}
	}
	return json.Marshal(struct {
		Items []T `json:"items"`
	}{Items: items})
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test ./internal/common/apiresponse/`
Expected: `ok`。

- [ ] **Step 5: 替换 4 个调用点**

- `auth/controller.go`：删除本地 `type itemList[T any]`，`listSessions` 改为 `apiresponse.ItemList[SessionResponse]{Items: items}`，导入 `website/backend/internal/common/apiresponse`。
- `access/controller.go`：删除本地 `type itemList[T any]`；`listPermissions`、`listGroups`、`listUsers` 三处改用 `apiresponse.ItemList[...]`，导入同一包。
- `access/access_test.go`：`decodeItems` 内的 `var body itemList[T]` 改为 `var body apiresponse.ItemList[T]`，导入同一包。
- `settings/controller.go`：删除 `publicSettingsView`；`getPublic` 改为 `apiresponse.ItemList[groupView]{Items: items}`，导入同一包；`groupView` 保持原样（Task 4 再搬）。
- `README.md` 规则 3 的允许位置清单补 `internal/common/apiresponse`：
  ```markdown
  3. Gin 与 HTTP 细节只允许出现在 controller、module.go（挂载路由）、app、`internal/common/guard`（中间件）、`internal/common/apierror`（统一错误写出）和 `internal/common/apiresponse`（共享响应信封）；service 一律不 import gin。
  ```

- [ ] **Step 6: 跑相关包测试**

Run（先按 README 导出 `DATABASE_URL_TEST`）：

```bash
cd backend
bash scripts/test.sh ./internal/common/apiresponse/ ./internal/modules/auth/... ./internal/modules/access/... ./internal/modules/settings/...
```

Expected: 所有包 `ok`；auth / access / settings 的既有列表响应断言全部通过。

- [ ] **Step 7: 跑静态检查**

Run: `bash scripts/check.sh`
Expected: `check ok`。

---

### Task 4: 把散在 controller 的 DTO 归位到 `dto.go`（已完成 2026-10-20）

**Files:**
- Modify: `backend/internal/modules/access/controller.go:23-35`（移走三个 request 类型）
- Modify: `backend/internal/modules/access/dto.go`（接收三个 request 类型）
- Create: `backend/internal/modules/settings/dto.go`
- Modify: `backend/internal/modules/settings/controller.go:19-22`（移走 `groupView`）
- Create: `backend/internal/modules/mail/dto.go`
- Modify: `backend/internal/modules/mail/controller.go:17-25`（移走 request / response 类型）

**Interfaces:**
- Consumes: Task 3 的 `apiresponse.ItemList[T]`
- Produces: 无新 API；所有类型原名、原字段、原 JSON tag 不变。

- [ ] **Step 1: 搬 access 的请求类型**

把 `groupRequest`、`userRequest`、`userBatchRequest` 原样从 `access/controller.go` 移到 `access/dto.go`，不改变字段、tag、导出性。`access/controller.go` 保留使用处。

- [ ] **Step 2: 建 settings 的 `dto.go`**

新建 `settings/dto.go`，内容只包含 `groupView`（原样从 controller 移入）：

```go
package settings

type groupView struct {
	Group  string         `json:"group"`
	Values map[string]any `json:"values"`
}
```

`settings/controller.go` 删除该类型，保留 `publicSettingsView` 已删除后的共享信封用法。

- [ ] **Step 3: 建 mail 的 `dto.go`**

新建 `mail/dto.go`，原样移入：

```go
package mail

type createMessageRequest struct {
	To string `json:"to"`
}

type createMessageResponse struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Sent    bool   `json:"sent"`
}
```

`mail/controller.go` 删除这两个类型，其余不变。

- [ ] **Step 4: 编译与格式验证**

Run: `cd backend && gofmt -w internal/modules/access internal/modules/settings internal/modules/mail && go build ./...`
Expected: 无输出、退出码 0。

- [ ] **Step 5: 确认没有重复定义与漏搬**

Run:

```bash
cd backend
grep -R "type groupRequest\|type userRequest\|type userBatchRequest\|type groupView\|type createMessageRequest\|type createMessageResponse" -n internal/modules
```

Expected: 每个类型恰好一处定义，且都在对应模块的 `dto.go`。

- [ ] **Step 6: 跑相关包测试**

Run:

```bash
cd backend
bash scripts/test.sh ./internal/modules/access/... ./internal/modules/settings/... ./internal/modules/mail/... ./internal/app/
```

Expected: 所有包 `ok`；线上 JSON 形状未变。

- [ ] **Step 7: 跑静态检查**

Run: `bash scripts/check.sh`
Expected: `check ok`。

---

### Task 5: 让 access repository 不再依赖 `apierror`（已完成 2026-10-20）

**Files:**
- Modify: `backend/internal/modules/access/repository.go`（移除 `apierror` import，改为私有域哨兵）
- Modify: `backend/internal/modules/access/service.go`（新增 `mapRepoError`，在所有 repository 调用边界翻译）
- Modify: `backend/scripts/check.sh`（加 repository 不得 import `apierror` 的门禁）
- Create: `backend/internal/modules/access/errors_test.go`（或追加到 `service_test.go`；新建文件更聚焦）

**Interfaces:**
- Consumes: 现有 `apierror` 哨兵与 access 的 repository 方法
- Produces:
  - 私有哨兵：`errRecordNotFound`、`errGroupNameTaken`、`errGroupNotEmpty`、`errLastOwner`
  - `func mapRepoError(err error) error`：把上述四个域哨兵翻译成对应的 `apierror`，其余原样返回
  - repository 包不再 import `apierror`

- [ ] **Step 1: 写失败的表驱动测试**

`backend/internal/modules/access/errors_test.go`：

```go
package access

import (
	"errors"
	"fmt"
	"testing"

	"website/backend/internal/common/apierror"
)

func TestMapRepoError(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want error
	}{
		{"record not found", errRecordNotFound, apierror.ErrNotFound},
		{"group name taken", errGroupNameTaken, apierror.ErrGroupNameTaken},
		{"group not empty", errGroupNotEmpty, apierror.ErrGroupNotEmpty},
		{"last owner", errLastOwner, apierror.ErrLastOwner},
		{"wrapped sentinel", fmt.Errorf("query: %w", errRecordNotFound), apierror.ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapRepoError(tc.in)
			if !errors.Is(got, tc.want) {
				t.Fatalf("mapRepoError(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}

	boom := errors.New("boom")
	if got := mapRepoError(boom); got != boom {
		t.Fatalf("unknown error = %v, want the original error", got)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/modules/access/ -run TestMapRepoError`
Expected: 编译失败，提示 `undefined: errRecordNotFound` 或 `undefined: mapRepoError`。

- [ ] **Step 3: 在 repository 增加私有哨兵并替换返回**

在 `access/repository.go` 的 import 中删除 `website/backend/internal/common/apierror`，加入标准库 `errors`，声明：

```go
var (
	errRecordNotFound = errors.New("record not found")
	errGroupNameTaken = errors.New("group name taken")
	errGroupNotEmpty  = errors.New("group not empty")
	errLastOwner      = errors.New("last owner")
)
```

按下表替换（位置按当前文件）：
- `findUser` 的 `apierror.ErrNotFound` → `errRecordNotFound`
- `findGroup` 的 `apierror.ErrNotFound` → `errRecordNotFound`
- `createGroup` 的约束错误 `apierror.ErrGroupNameTaken` → `errGroupNameTaken`
- `updateGroupName` 的约束错误 `apierror.ErrGroupNameTaken` → `errGroupNameTaken`
- `replaceGroupPermissions` 的 `apierror.ErrNotFound` → `errRecordNotFound`
- `deleteGroup` 的约束错误 `apierror.ErrGroupNotEmpty` → `errGroupNotEmpty`
- `deleteUsers` 的 `apierror.ErrLastOwner` → `errLastOwner`；`apierror.ErrNotFound` → `errRecordNotFound`
- `updateUser` 的 `apierror.ErrLastOwner` → `errLastOwner`；约束错误 `apierror.ErrNotFound` → `errRecordNotFound`

完成后 Run: `grep -n "apierror" backend/internal/modules/access/repository.go`
Expected: 无输出。

- [ ] **Step 4: 在 service 增加翻译函数**

在 `access/service.go`（`newService` 之后、导出方法之前）加：

```go
func mapRepoError(err error) error {
	switch {
	case errors.Is(err, errRecordNotFound):
		return apierror.ErrNotFound
	case errors.Is(err, errGroupNameTaken):
		return apierror.ErrGroupNameTaken
	case errors.Is(err, errGroupNotEmpty):
		return apierror.ErrGroupNotEmpty
	case errors.Is(err, errLastOwner):
		return apierror.ErrLastOwner
	default:
		return err
	}
}
```

- [ ] **Step 5: 把 service 的 repository 错误出口接上翻译**

规则：**凡是来自 `s.repo.*` 的 `err`，返回前过 `mapRepoError`；service 自己产生的校验 / 授权 / UUID 解析错误保持原样。** 覆盖以下方法：

- `IdentityByUserID`：`findUser` 的错误先用 `errors.Is(err, errRecordNotFound)` 判断并直接返回 `guard.ErrUserNotFound`；其余错误过 `mapRepoError`。**不要先翻译成 apierror 再找域哨兵**——翻译后域哨兵已经不在错误链的判定位置上。
- `CreateGroup`：`repo.createGroup`。
- `UpdateGroup`：`repo.findGroup`、`repo.groupPermissions`、`repo.updateGroupName`、`repo.replaceGroupPermissions`。
- `DeleteGroup`：`repo.findGroup`、`repo.memberCount`、`repo.deleteGroup`。
- `GetUser`：`repo.findUser`、`repo.userPermissions`、`effectivePermissions` 内的 repository 错误。
- `UpdateUser`：`repo.findUser`、`repo.findGroup`、`repo.userPermissions`、`permissionsOfGroup`、`repo.updateUser`。
- `DeleteUsers`：`repo.findUsers`、`repo.deleteUsers`。
- `groupDetail`：`repo.findGroup`、`repo.memberCount`、`permissionsOfGroup`。
- `effectivePermissions`：`permissionsOfGroup`、`repo.userPermissions`。
- `permissionsOfGroup`：`repo.groupPermissions`。

`mapRepoError` 对无哨兵的原始错误是恒等返回，所以统一包一层不会改变现有 500 行为。

用 `grep -n "s\.repo\." internal/modules/access/service.go` 列出所有 repository 调用，逐一确认错误出口已接上 `mapRepoError`，不要漏掉 `effectivePermissions` / `permissionsOfGroup` 这类嵌套调用。

- [ ] **Step 6: 把「repository 不 import apierror」写进检查脚本**

在 `backend/scripts/check.sh` 的 `go build ./...` 之前加：

```bash
import_violations="$(grep -R -l 'website/backend/internal/common/apierror' --include='repository.go' internal/modules || true)"
if [ -n "$import_violations" ]; then
  echo "repository must not import apierror:" >&2
  printf '%s\n' "$import_violations" >&2
  exit 1
fi
```

这样规则不依赖代码评审。跑 `bash scripts/check.sh`，Expected: `check ok`（此时 access repository 已清干净）。

- [ ] **Step 7: 跑 access 测试与新增测试**

Run:

```bash
cd backend
bash scripts/test.sh ./internal/modules/access/...
```

Expected: 全部 `ok`，包括 `TestMapRepoError` 与既有的 404 / 409 / 403 断言。

- [ ] **Step 8: 跑全仓测试与静态检查**

Run（先导出 `DATABASE_URL_TEST`）：

```bash
cd backend
bash scripts/test.sh
bash scripts/check.sh
```

Expected: 全仓 `ok`；`check ok`。

---

### Task 6: DEVLOG、版本进位与最终验证（DEVLOG / 版本未执行；全仓测试已完成；提交随本轮入库）

**Files:**
- Modify: `docs/DEVLOG.md`（顶部新增本轮条目）
- Modify: `backend/internal/version/version.go`（由脚本改）
- Modify: `backend/README.md:3`（由脚本改「当前版本」）

**Interfaces:**
- Consumes: Task 1–5 的全部产物
- Produces: 0.21.1 版本号与可追溯的 DEVLOG 条目

- [ ] **Step 1: 版本进位一次**

Run: `cd backend && bash scripts/bump-version.sh patch`
Expected: `internal/version/version.go` 变为 `0.21.1`，README 顶部「当前版本」同步为 0.21.1。

只跑一次；不要重复执行。

- [ ] **Step 2: 写 DEVLOG 条目**

在 `docs/DEVLOG.md` 顶部「---」之后新增一条，按现有格式包含：

```markdown
## 2026-10-20 · 后端工程风格收口（规范文档 + 检查脚本 + 内部结构小重构）（0.21.1）

**范围**：新增 `backend/STYLE.md`、`backend/scripts/check.sh`、`internal/common/apiresponse`；移动作业 3 个模块的 DTO；access repository 错误改为域哨兵。零迁移、零新路由、零新依赖、**零 API 形状变化**。

**动机**：项目从首个后端提交起就是 NestJS 式模块化，但规范只写到骨架；DTO 放置、错误所有权、集合信封在 5 个模块里出现三种写法，新模块只能靠模仿。

**口径**：
- 规范以 2026-10-11 的早期设计文档为准；README 硬性规则 1–3 同步补上 mail 无 service、DTO 归位与共享信封。
- 仓库不 import apierror：错误由 service 翻译；settings 的模块哨兵 + controller 映射按早期设置设计保留。
- 集合信封统一为 `apiresponse.ItemList[T]`，线上仍是 `items`；D4 的 `data/total/limit/offset`、epoch 时间戳等目标态未实施。
- mail 仍无 service；main.go 仍用标准库 log；测试 helper 仍按包复制——三条都写进 STYLE 的存量例外表。

**验证**：`bash scripts/check.sh` 通过（含负向探针：未格式化文件会红）；`bash scripts/test.sh` 全仓通过；`go build ./...`、`go vet ./...` 零输出。**运行期未验**：本轮无接口形状与业务行为变化，未另起服务做手工请求。

**已知缺口**：日志仍是 main 的 stdlib log + 模块 slog 两套（已在规范标注，统一日志与 request id 另开轮次）；settings / mail / access 的三处结构性例外未消除。

**提交状态**：待用户确认后一次提交。
```

- [ ] **Step 3: 最终静态与测试验证**

Run:

```bash
cd backend
bash scripts/check.sh
bash scripts/test.sh
```

Expected: `check ok`；全仓测试 `ok`。

- [ ] **Step 4: 检查工作区，确认只动了本计划的文件**

Run: `git status --short`
Expected: 本计划涉及的文件，外加并行会话可能仍在工作区的改动（以当时输出为准，名单可能已被并行会话提交或撤销）。**并行文件保持未暂存**；提交时逐文件 `git add`，绝不使用 `git add -A` / `git add .`。

- [ ] **Step 5: 请用户确认后提交（一次提交）**

确认后只暂存本计划的文件，提交信息：

```text
refactor(backend): 收口工程风格——规范文档、检查脚本与模块结构统一
```

提交前再次 `git diff --cached --stat` 核对；不要把并行会话的改动带入。

---

## Self-Review

**1. Spec coverage**
- 早期设计文档的模块五件套 / 零注释 / repository 具体类型 → Task 1 的 STYLE §1、§6、§9；不改代码结构。
- access 的「边界靠 DTO」→ STYLE §4；access 的 DTO 保留原名，Task 4 只搬家不改名。
- mail 的能力模块例外 → STYLE §1、§9；README 规则 2；不补 service。
- settings 的模块哨兵 → STYLE §3、§9；不强行改 settings。
- 数据层「不隔仓储接口层」→ STYLE §2；无代码改动。
- API 目标态不实施 → Global Constraints、STYLE §5、Task 6 DEVLOG。
- 用户选择的「文档 + 检查 + 小重构」→ Task 1、2、3、4、5 全部覆盖。

**2. Step scan**
- 每个实现步骤都有精确文件、类型名、签名或替换表；没有 TBD / 「酌情处理」。
- 文档步骤给出了必须包含的条款，而不是让执行者自行发明。
- 负向验证步骤给出探针内容、预期失败输出与清理命令。

**3. Type consistency**
- `apiresponse.ItemList[T]` 在 Task 3 定义，Task 3 / 4 使用；字段名 `Items`、JSON 键 `items` 前后一致。
- `mapRepoError` 与四个私有哨兵在 Task 5 定义并使用；测试名与函数名一致。
- Task 4 搬家的类型名、字段、tag 与当前代码逐字一致。

**4. Review Focus**
- 空集合 `[]`：Task 3 的 `TestItemListMarshalsNilAsEmptyArray`。
- access 404 / 409 映射：Task 5 的 `TestMapRepoError` + 既有 service 测试。
- DTO tag 不变：Task 4 的既有 controller 测试 + Step 5 的 grep。
- 导出类型不降级：Task 4 的 `go build ./...` + `./internal/app/` 测试。
- 检查脚本会红：Task 2 的负向探针。

**5. Proportion**
- 计划约 300 行，对应 1 份规范文档 + 1 个脚本 + 1 个共享包 + 3 个模块的小改动；没有把实现代码抄进计划，只给出签名、替换表与测试断言。
