# NeoBlog 设置组存储（后端内部机制）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 落地设置组的存取机制：一张 `setting_values` 表只记"被写入过的值" + 一份写在代码里的设置清单（初始为空）；五个动作（按类型读、整组读、按类型写、整组写、删项恢复默认）；本轮无任何路由。

**Architecture:** 单表 `(setting_group, name)` 复合唯一，按复合键 upsert（ent `sql/upsert` 特性）；清单在代码内并在构造时自检，空清单 = 全默认；`settings.New(db) (*Service, error)` 是仓库第一个返回 error 的模块构造器，`app.New` 随之改为 `(*App, error)`（三处调用点同步）。

**Tech Stack:** Go + ent（`--feature sql/lock,sql/upsert`）+ Atlas 版本化迁移 + PostgreSQL 17（容器 `neoblog-postgres`，端口 5434）。

**Spec:** `docs/superpowers/specs/2026-10-11-neoblog-settings-design.md`（v2，下称 spec；引用按其章节号。行号事实已于 2026-10-11 复核：`app_test.go` 两处调用点为 `:56` 与 `:375`）

## Global Constraints（每个任务默认携带）

- 后端 Go 代码**零人工注释**，`gofmt` 规范缩进；每个任务收尾 `cd backend && go build ./... && go vet ./... && gofmt -l .` 全干净。
- 提交信息格式对齐仓库历史：`type(backend): 中文描述`。
- 集成测试连 `DATABASE_URL_TEST`（缺省 `postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_test?sslmode=disable`），未设置时 `t.Skip`——`testDB` 照抄 `backend/internal/modules/access/migration_test.go:17-36`。
- 迁移纪律：diff 出迁移 → 人工审阅 → `atlas migrate hash && atlas migrate validate` → `scripts/db-migrate-apply.sh` 应用到 `neoblog` 与 `neoblog_test`；应用启动绝不自动迁移。
- 不新增第三方依赖；不注册任何路由（`app.go` 只加构造与持有，不加 `Register`）。
- 错误用 settings 包内导出哨兵（本计划 Task 4 定义）；**不进 `internal/common/apierror`**——接口轮才做映射（spec §6）。
- 日志用 `slog`（先例 `internal/modules/mail/mailer.go:76` 用 `slog.Default()`）：只记组名 / 项名 / 错误分类，**不记值本身**（spec §6）。
- 版本 0.7.0 → 0.8.0（`./scripts/bump-version.sh minor`，Task 6 执行）。

## Review Focus

1. **数字边界绕过**：`3.5`、`1e3`、超 int64 的值——写入路径根本进不来（`SetNumber` 签名即 int64；`SetGroup` 的 `ItemWrite.Value` 类型不符即拒），读取路径对库中既有坏值必须按坏行回退默认并记日志，不得 panic、不得错误返回 → Task 4 集成⑧前段 + 单测③钉死。
2. **用错类型方法**：拿文字方法读数字项 → `ErrMethodMismatch` 显式报错，**不回退默认值、不做转换** → Task 4 单测④钉死。
3. **整组写部分坏**：`SetGroup` 任一项不合法（未知组 / 未知项 / 类型不符）→ 整组报错且库里零落库 → Task 4 集成⑤钉死。
4. **删未知项被吞成成功**：删未知组 / 未知项 → 显式报错；只有"清单内但库里没有这行"才是幂等成功 → Task 4 集成③钉死。
5. **upsert 冲突分支不刷新 `updated_at`**：同一项二次写入后时间必须变大——实现直接在 upsert 构造上显式 `SetUpdatedAt(time.Now())`，不赌 ent 的 `UpdateDefault` 是否作用于 `ON CONFLICT` 分支（spec §4 的兜底路径，本计划直接钉为唯一实现）→ Task 3 集成钉死。

---

### Task 1: ent schema + 生成特性 + 迁移

**Files:**
- Create: `backend/internal/database/ent/schema/setting_value.go`
- Modify: `backend/scripts/db-generate.sh`（`--feature sql/lock` → `--feature sql/lock,sql/upsert`）
- Modify: `backend/scripts/db-migrate-diff.sh`（迁移名字面量 `totp_and_recovery_codes` → `setting_values`；**若届时脚本已被内容模块轮参数化为 `$1` 传参，则直接传参 `setting_values`，不回退参数化**）
- Create: `backend/internal/modules/settings/migration_test.go`
- Modify: `backend/migrations/`（生成 `*_setting_values.sql` + `atlas.sum`）

**Interfaces:**
- Produces: ent 实体 `SettingValue` 与包 `settingvalue`（后续所有任务依赖）。schema 就是 spec §4 的 ent 摘要，逐字落地：

```go
type SettingValue struct {
	ent.Schema
}

func (SettingValue) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }),
		field.String("setting_group").NotEmpty(),
		field.String("name").NotEmpty(),
		field.JSON("value", json.RawMessage{}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (SettingValue) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("setting_group", "name").Unique(),
	}
}
```

- [ ] **Step 1: 写失败测试**——`migration_test.go`：`testDB` 照抄 access（`migration_test.go:17-36`）；`TestSettingValueSchema` 断言：`Client.SettingValue.Create().SetSettingGroup("site").SetName("title").SetValue(json.RawMessage(`"我的博客"`)).Save(ctx)` 成功且读回 `Value` 与写入字节一致；同组同名第二条创建返回 `ent.IsConstraintError`（复合唯一）；`SetValue(json.RawMessage(`"x"`))` 与数字、布尔各存一条读回原样（jsonb 三种标量形状）。
- [ ] **Step 2: 跑测试确认失败**——`go test ./internal/modules/settings/ -run TestSettingValueSchema -v`，编译失败（无实体）。
- [ ] **Step 3: 落 schema、改两个脚本、跑生成**——schema 按上方代码；`db-generate.sh` 加 `sql/upsert`；跑 `./scripts/db-generate.sh`。
- [ ] **Step 4: 出迁移**——按 Files 里的口径改 `db-migrate-diff.sh` 迁移名，跑 `./scripts/db-migrate-diff.sh`。
- [ ] **Step 5: 人工审阅迁移 SQL**（spec §4 审阅清单）：`setting_values` 五列 + 主键；`CREATE UNIQUE INDEX … ("setting_group", "name")`（索引名按 ent 既有规则，参照 `grouppermission_group_id_permission` 先例，以实际生成为准）；`value` 列类型为 jsonb（**全仓第一个 `field.JSON`，无先例可抄——生成形状与 spec §4 表格不符则人工修正 schema 重跑**）；无任何数据语句、无 seed。
- [ ] **Step 6: hash + validate + 应用两库**。
- [ ] **Step 7: 跑测试确认通过**。
- [ ] **Step 8: 提交**——`git commit -m "feat(backend): 设置值单表 ent schema 与迁移（复合唯一 + jsonb）"`

### Task 2: types.go + registry.go——清单、自检、README 配方（无库单测）

**Files:**
- Create: `backend/internal/modules/settings/types.go`、`backend/internal/modules/settings/registry.go`
- Modify: `backend/README.md`（新增「设置」章节 + "怎么加一个设置"配方——配方与测试同任务落，防漂移；目录对照表加行、规则 1/5 措辞留给 Task 6）
- Test: `backend/internal/modules/settings/registry_test.go`

**Interfaces:**
- Produces（Task 4/5 按这些签名消费）:

```go
package settings

type FieldType int

const (
	FieldTypeString FieldType = iota + 1
	FieldTypeNumber
	FieldTypeBool
)

type Definition struct {
	Name    string
	Type    FieldType
	Default any // 严格匹配：FieldTypeString→string、FieldTypeNumber→int64、FieldTypeBool→bool
}

type Group struct {
	Name  string
	Items []Definition
}

type Registry struct {
	// groups: 组名 → 项名 → 定义；编译产物，构造后只读
	groups map[string]map[string]Definition
	order  []string
}

func compileRegistry(groups []Group) (Registry, error)
var builtinRegistry = []Group{} // 清单初始为空（spec §2/§5）
```

自检规则（`compileRegistry` 内，spec §5）：组名 / 项名匹配 `^[a-z][a-z0-9_]*$`；组名不重复；组内项名不重复；`Default` 的 Go 类型与 `Type` 严格一致（用 type switch，`int` / `int32` 等一律拒，只收 `int64`）；数字默认值直接是 int64 类型即天然在 int64 界内。编译失败返回普通 `error`（含组名/项名定位，不触发哨兵）。

README 配方（字面代码，测试按它构造）：

> 在 `internal/modules/settings/registry.go` 的 `builtinRegistry` 里添加（组不存在就新增一个 `Group` 字面量）：
> ```go
> var builtinRegistry = []Group{
> 	{
> 		Name: "site",
> 		Items: []Definition{
> 			{Name: "title", Type: FieldTypeString, Default: "我的博客"},
> 			{Name: "port", Type: FieldTypeNumber, Default: int64(8080)},
> 			{Name: "maintenance", Type: FieldTypeBool, Default: false},
> 		},
> 	},
> }
> ```
> 保存后重启即生效：新设置立刻可读（默认值），**不需要任何数据库操作**。

- [ ] **Step 1: 写失败测试**——`registry_test.go` 覆盖（§8 单测①③ + ⑤的编译半段）：**配方防漂移用例——按 README 配方的字面形状构造**（`site` 组 + title/port/maintenance 三类型项，与配方代码逐字一致）断言编译通过，配方与代码形状一漂移这条就红（⑤的"未写库读到默认值"半段由 Task 4 集成②覆盖）；另取合法清单编译通过且读回定义；组名重复 / 组内项名重复 / 名字非法（`Site`、`1a`、`a-b`）/ `Default` 类型不符（string 项给 int64、number 项给 float64 或 int、bool 项给 string）各拒一例；空清单编译通过。
- [ ] **Step 2: 跑测试确认失败**——`go test ./internal/modules/settings/ -run TestRegistry -v`，编译失败。
- [ ] **Step 3: 实现**——types.go + registry.go + README 配方章节（含目录定位提示："模块见 `internal/modules/settings/`"）。
- [ ] **Step 4: 跑测试确认通过**。
- [ ] **Step 5: 提交**——`git commit -m "feat(backend): 设置清单类型与编译自检，README 配方"`

### Task 3: repository.go——upsert / 查询 / 删除（集成）

**Files:**
- Create: `backend/internal/modules/settings/repository.go`
- Test: `backend/internal/modules/settings/repository_test.go`

**Interfaces:**
- Consumes: Task 1 的 ent 包 `settingvalue`。
- Produces（Task 4 消费）:

```go
type repository struct{ client *ent.Client }

func (r *repository) upsert(ctx context.Context, group, name string, value json.RawMessage) error
// Create().SetSettingGroup(group).SetName(name).SetValue(value).SetUpdatedAt(time.Now()).
//   OnConflict(sql.ConflictColumns("setting_group", "name")).UpdateNewValues().Exec(ctx)
// 显式 SetUpdatedAt 是 Review Focus 5 的实现：冲突分支必须拿到新鲜时间
func (r *repository) get(ctx context.Context, group, name string) (json.RawMessage, bool, error) // 无行→(nil,false,nil)
func (r *repository) listByGroup(ctx context.Context, group string) ([]*ent.SettingValue, error)
func (r *repository) delete(ctx context.Context, group, name string) (int, error) // 返回受影响行数
```

- [ ] **Step 1: 写失败测试**——`repository_test.go`（`testDB` 复用）：upsert 首次写 → get 返回 (值,true)；二次 upsert 换值 → get 返回新值且 `updated_at` **严格变大**（第一次写后读 `updated_at`，`time.Sleep(2 * time.Millisecond)` 后第二次写再读比较——§8 集成⑧的仓储层版本）；listByGroup 只出该组；delete 返回 1 且行消失，再 delete 返回 0 不报错。
- [ ] **Step 2: 跑测试确认失败**——`go test ./internal/modules/settings/ -run TestSettingRepo -v`，编译失败。
- [ ] **Step 3: 实现**——upsert 的 `OnConflict(sql.ConflictColumns(...))` 需要 `sql/upsert` 特性（Task 1 已开）与 ent 生成的 `OnConflict` 方法；列名常量来自 `settingvalue` 包。**若 `OnConflict` 生成物或行为异常**（编译不过 / 生成非预期 SQL），退为事务内"先查后写"（spec §9 备选）：`Tx` 内 `get` 判存在 → `Update` 或 `Create`，本任务测试断言不变。
- [ ] **Step 4: 跑测试确认通过**。
- [ ] **Step 5: 提交**——`git commit -m "feat(backend): 设置值仓储——复合键 upsert 与查询删除"`

### Task 4: service.go——五个动作、哨兵、类型分方法（单测 + 集成）

**Files:**
- Create: `backend/internal/modules/settings/service.go`
- Test: `backend/internal/modules/settings/service_test.go`（单测段）、`backend/internal/modules/settings/service_integration_test.go`（集成段）

**Interfaces:**
- Consumes: Task 2 的 `Registry` / `builtinRegistry`、Task 3 的 `repository`。
- Produces（Task 5 装配消费；也是模块对外能力的全部）:

```go
type Service struct {
	repo     *repository
	registry Registry
	logger   *slog.Logger
}

func New(db *database.Database) (*Service, error)                       // builtinRegistry + slog.Default()；编译失败错误沿启动链返回
func newServiceWith(db *database.Database, groups []Group, logger *slog.Logger) (*Service, error) // 包内测试注入；db 可为 nil（仅用于不触库的校验路径单测）
func (s *Service) GetString(ctx context.Context, group, name string) (string, error)
func (s *Service) GetNumber(ctx context.Context, group, name string) (int64, error)
func (s *Service) GetBool(ctx context.Context, group, name string) (bool, error)
func (s *Service) GetGroup(ctx context.Context, group string) (map[string]any, error)
func (s *Service) SetString(ctx context.Context, group, name, value string) error
func (s *Service) SetNumber(ctx context.Context, group, name string, value int64) error
func (s *Service) SetBool(ctx context.Context, group, name string, value bool) error
func (s *Service) SetGroup(ctx context.Context, group string, items []ItemWrite) error
func (s *Service) ResetItem(ctx context.Context, group, name string) error

type ItemWrite struct {
	Name  string
	Type  FieldType
	Value any // 严格匹配 Type：string/int64/bool
}

var (
	ErrUnknownGroup    = errors.New("settings: unknown group")    // 未知组
	ErrUnknownItem     = errors.New("settings: unknown item")     // 未知项
	ErrTypeMismatch    = errors.New("settings: type mismatch")    // ItemWrite/默认值与声明不符
	ErrMethodMismatch  = errors.New("settings: method/type misuse") // 用错类型的读写方法
)
```

**行为口径（全部来自 spec §6，实现顺序钉死）**：
- 所有方法第一步查清单：未知组 → `ErrUnknownGroup`；未知项 → `ErrUnknownItem`——这两步**先于任何数据库访问**（保证 Task 4 单测可用 `newServiceWith(nil, …)` 跑）。
- 类型方法用错（如 `GetString` 打在 `FieldTypeNumber` 项上）→ `ErrMethodMismatch`，不回退、不转换（spec §6 末段）。
- 读单项：无行 → 默认值；有行 → 按声明类型解码，**解码不符 → `logger.Warn`（组名/项名/错误分类）+ 回退默认值**——这是唯一静默兜底。
- 数字读取算法（Review Focus 1 的实现，唯一需要写出主体的算法）：`json.Decoder` + `UseNumber()` 解出 `json.Number` → `strconv.ParseInt(string(n), 10, 64)`——`3.5`、`1e3`、超界、负溢出全被 ParseInt 拒绝，错误归入"解码不符"走坏行回退；`GetString`/`GetBool` 同样用 UseNumber 解出后要求 `string` / `bool` 类型，否则坏行回退。
- `GetGroup`：遍历清单内该组全部项逐项合并（默认 ⊕ 覆盖），坏行只回退该项并记日志，整组不失败；未知组 → `ErrUnknownGroup`。
- `SetGroup`：先对全部 `items` 做清单 + 类型校验（任何一项 `ErrTypeMismatch` / 未知 → 整组返回该错，**零落库**），再单事务逐项 `repo.upsert`。
- `ResetItem`：`repo.delete` 返回受影响 0 行且项在清单内 → 成功（幂等只指这个，spec §6）；未知组 / 项在查清单时就报错。

- [ ] **Step 1: 写失败单测**——`service_test.go`（`newServiceWith(nil, 样例组, slog.New(…bytes.Buffer…))`，不触库）：方法用错（GetString 打数字项 / SetBool 打文字项 / GetNumber 打开关项）→ `ErrMethodMismatch` 且**不回退默认值**（错误即返回）；未知组 / 未知项读写删全拒；`ItemWrite` 类型不符 → `ErrTypeMismatch`（§8 单测②）；数字边界写入侧天然不可能（签名即 int64），断言 `ItemWrite{Type: FieldTypeNumber, Value: 3.5}` 与 `Value: int32(3)` 被拒。
- [ ] **Step 2: 写失败集成测试**——`service_integration_test.go`（样例组注入，§8 集成①–⑧逐条）：①三类型写读一致；②未写项读默认；③删行后读默认且行消失、删清单内但无行成功、删未知项报错；④整组读取合并正确、**手工 SQL 造坏行（`UPDATE setting_values SET value='3.5'`）后只回退该项其余照常**；⑤SetGroup 一好一坏 → 整组报错且逐行核对库里零落库；⑥坏行读回默认不报错（含 `1e3`、超 int64）；⑦再注入一个新样例组的用例不执行任何迁移、不依赖任何 DDL 即通过（"加设置只改代码"的测试面证据；迁移目录文件数不变由 Task 6 验收 grep 把关）；⑧同项二次写入 `updated_at` 变大（service 层复验 Task 3 断言）。
- [ ] **Step 3: 跑测试确认失败**——两组测试均编译失败。
- [ ] **Step 4: 实现**——按行为口径。
- [ ] **Step 5: 跑测试确认通过**（单测无库也跑：`go test ./internal/modules/settings/ -v` 全绿）。
- [ ] **Step 6: 提交**——`git commit -m "feat(backend): 设置服务五动作——类型分方法/整组事务/恢复默认"`

### Task 5: app.New 改签名 + 装配 + 三处调用点

**Files:**
- Modify: `backend/internal/app/app.go`（`New(cfg, db) (*App, error)`；`App` 结构体加私有字段 `settings *settings.Service` 并构造——对齐 `mailer` 的处理，**不加访问器**，供后续接口轮再定）
- Modify: `backend/main.go:29`（链式 `app.New(cfg, db).Run(cfg.Addr)` 拆为"取实例 + 判错"两句，错误 `log.Fatal`）
- Modify: `backend/internal/app/app_test.go:56`（`New(cfg, db).Handler()` → 判错后取 `Handler()`）与 `:375`（`a := New(cfg, db)` → `a, err := New(cfg, db)` + 判错）
- Test: 既有 `internal/app/app_test.go` 全量回归即测试

- [ ] **Step 1: 改 app.go 与三处调用点**——本任务由编译驱动：改签名后 `go build ./...` 会精确指出未同步处，三处之外不应有第四处（已实测全仓仅此三处调用）。
- [ ] **Step 2: 编译 + 全量测试**——`go build ./... && go vet ./... && gofmt -l .` 干净；`go test ./internal/app/ -v` 全绿（原用例行为不变，只是多判一次错）；`go test ./...` 全绿。
- [ ] **Step 3: 手工冒烟**——`scripts/dev.sh` 起 server（`Start-Process`），日志无 settings 相关错误、健康检查 200，确认空清单构造路径在真实启动链上可用；停掉。
- [ ] **Step 4: 提交**——`git commit -m "feat(backend): app.New 改返回 error 并装配 settings（三处调用点同步）"`

### Task 6: README 收尾、DEVLOG、版本与验收

**Files:**
- Modify: `backend/README.md`（①迁移清单补一行；②目录对照表（26 行附近）加 `internal/modules/settings` 行；③规则 1 的"（当前仅 `mail`）"→"（当前 `mail`、`settings`）"、规则 5 的"（当前仅 `mail.New(cfg) Mailer`）"→并列 `settings.New(db) (*Service, error)` 且注明它**返回 error**——两处措辞以 spec §3/§7 为准；④确认 Task 2 的"设置"章节在位）
- Modify: `docs/DEVLOG.md`（条目：设置组存储机制落地）
- Modify: `docs/superpowers/specs/2026-10-11-neoblog-settings-design.md` §4 末段（按 §11.1 回写实施事实：upsert 走 repository 显式 `SetUpdatedAt`）
- Modify: 版本（`./scripts/bump-version.sh minor`：0.7.0 → 0.8.0）

- [ ] **Step 1: README 四处更新**——目录对照表行写明"无路由模块（`New` 返回 `(*Service, error)`）：设置清单 + 复合键 upsert 存储"。
- [ ] **Step 2: DEVLOG + 版本进位**。
- [ ] **Step 3: spec §4 回写**。
- [ ] **Step 4: 全量验收（spec §11 逐条）**——`go build ./... && go vet ./... && gofmt -l .` 干净；有库跑 `go test ./...`（集成全过）、无库全绿 skip；`app.New` 三处调用点已同步；README 三处 + 设置章节齐；无新增路由（`grep -n "settings" internal/app/app.go` 只有构造行）。
- [ ] **Step 5: 提交**——`git commit -m "docs(backend): 设置模块 README/DEVLOG，版本进位 0.8.0"`
