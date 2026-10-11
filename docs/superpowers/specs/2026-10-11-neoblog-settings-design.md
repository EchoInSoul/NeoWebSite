# NeoBlog 设置组存储设计（后端内部机制，本轮不含接口）

日期：2026-10-11
范围：`backend/`（Go + ent + Atlas）；只做设置值的存取机制，不碰 HTTP 层与前端
状态：设计经用户确认（2026-10-11 对话：范围 = 只做内部机制、不做接口与公开读取；密钥类本轮不做；存储 = 一项一条记录 + 代码内清单；恢复默认 = 删记录）；本文为存档版，其后进 writing-plans 出实施计划

修订记录：
- v1（2026-10-11，本文）：首版。一次性把设置组的存取底子做扎实——以后新增设置只改代码、永不操作数据库。

## 1. 意图与背景

用户目标：把"设置"的底层机制一次做好——一张只记"被写入过的值"的表 + 一份写在代码里的设置清单。此后新增 / 删除设置项永远只是代码变更：数据库结构不随设置增减变化，也不需要任何 SQL 操作。

对标旧博客项目（`D:\protect\NextJs_Blog`，Prisma + TypeScript）的同类机制——其骨架（定义在代码、库里只存覆盖值、空表 = 全默认）经实践验证良好，本设计沿用，并修正两处：

- **组名与项名拆开存。** 旧项目把"组名.项名"拼成一个字符串主键，再冗余一列存组名，两者一致靠代码纪律维持；本设计拆成 `(setting_group, name)` 两列复合唯一——结构上杜绝不一致。
- **"恢复默认 = 删记录"定为语义。** 旧项目写路径只有覆盖写入、没有删除，"改回默认"只能把默认值再写一遍；本设计让删除成为一等操作，库中只保留被显式写入过的项。

对话中的"记账本"= §4 的表；"清单"= §5 的代码内注册表。（对比过的另一形态"一组一条记录、整组打包"已否：并发与粒度差；过程见对话。）

约束（用户已定）：

- 只做内部机制：无接口、无前端、无"不用登录就能读"的公开口子。
- 密钥类（加密存储）本轮不做；插槽见 §10。
- 沿用既有规范：零注释、ent + Atlas 迁移工作流、`DATABASE_URL_TEST` 集成测试约定、结构化日志。

## 2. 已定决策

| 项 | 决定 | 落地时点 |
| --- | --- | --- |
| 存储形态 | 单表 `setting_values`；`(setting_group, name)` 复合唯一；只保留被显式写入的项 | 本轮 |
| 默认值 | 全部在代码清单定义；库中零 seed；空表 = 全默认 | 本轮 |
| 恢复默认 | 删行；删除不存在的行按幂等成功处理 | 本轮 |
| 清单初始内容 | 空（不含任何真实设置）；第一批设置随使用方后续添加 | 本轮 |
| 字段类型 | 三种：文字 / 数字（整数）/ 开关；只做类型把关 | 本轮 |
| 密钥类（加密） | 不做；`value` 列天然可容纳信封对象，加它不动表 | 插槽（§10） |
| 接口 / 公开读取 / 界面 | 不做 | 后续轮次 |
| 启动自检 | 构造时校验清单，不合法启动失败（错误沿启动链返回，同 `config.Load` / `database.Open` 风格） | 本轮 |
| 版本 | minor 进位（执行时按当前版本顺延；当前 0.7.0） | 本轮 |

## 3. 范围

本轮做：

- ent schema 新增 `SettingValue`（`internal/database/ent/schema/setting_value.go`）；重生成生成代码并提交；`scripts/db-generate.sh` 增加 `sql/upsert` 特性（§7）。
- 迁移：一个（建表 + 复合唯一索引），纯 DDL、零数据；`scripts/db-migrate-diff.sh` 的迁移名字面量换为 `setting_values`（脚本仍是写死字面量，沿用现状只换字符串）；人工审阅、hash + validate、应用到 `neoblog` 与 `neoblog_test`。
- `internal/modules/settings`：清单（含启动自检）、按类型读取、整组读取、单项写入、整组写入、单项删除（恢复默认）；装配进 `app.New`；不注册任何路由。
- 测试：清单校验与类型把关（单测，无库）；存取与事务、坏行回退（集成，`DATABASE_URL_TEST` 缺省 skip）。
- README 更新（迁移清单补充 + 新增"设置"章节含"怎么加一个设置"配方）、`docs/DEVLOG.md` 条目、版本进位 minor。

本轮不做：

- 任何接口 / 路由；公开读取；控制台或任何界面。
- 密钥类（加密存储）。
- 类型之外的校验（范围 / 长度 / 选项）；标签、说明之类的展示元数据。
- 主题的动态设置空间（§10 插槽）；导入导出；缓存。
- 第一批真实设置（清单留空，见 §2）。

## 4. 数据模型

表 `setting_values`（ent 类型 `SettingValue`；表名由 ent 复数化，同 `RecoveryCode` → `recovery_codes` 机制）：

| 列 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | uuid | 主键，默认 UUIDv7（同 `user.go` 写法） | 代理主键；业务身份是下一行的复合键 |
| `setting_group` | varchar | NOT NULL | 组名（如 `site`）；不用 `group`——Postgres 保留字 |
| `name` | varchar | NOT NULL | 项名（如 `title`） |
| `value` | jsonb | NOT NULL | 值的原样 JSON（文字 / 整数 / 布尔三种标量） |
| `updated_at` | timestamptz | NOT NULL | 最后写入时间；不设 `created_at` |

索引：`(setting_group, name)` 复合唯一——同组同名只可能一行；点查（组 + 名）与整组读取（组）共用这一条索引。

ent 摘要（形状对齐 `group_permission.go`）：

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

迁移预期内容（人工审阅清单）：

- `CREATE TABLE "setting_values"`：上述五列，主键 `id`；
- `CREATE UNIQUE INDEX "settingvalue_setting_group_name" ON "setting_values" ("setting_group", "name")`（索引名按 ent 既有命名规则：类型名 + 字段名拼接，实证如 `grouppermission_group_id_permission`；以实际生成为准，不符人工修正）；
- **无数据语句、无 seed**——与权限迁移不同，本表建完即空，空表 = 全默认。

写入方式：按复合键的 upsert（`INSERT ... ON CONFLICT DO UPDATE`）。依赖 `ent generate --feature sql/upsert`（§7）。

## 5. 清单（代码内注册表）

清单是"有哪些设置"的唯一事实来源，全部写在 `internal/modules/settings` 内：

- 组：名字 + 若干项；项：名字、类型（三选一）、默认值。
- 名字规则（组名与项名同规则）：`^[a-z][a-z0-9_]*$`（小写字母开头，小写字母 / 数字 / 下划线）。
- 启动自检（构造时执行；不合法启动失败，不带着坏清单上线）：
  - 组名、项名格式合法；组名不重复；组内项名不重复；
  - 默认值类型与声明一致（文字 ↔ 字符串、数字 ↔ 整数、开关 ↔ 布尔）。
- 初始为空：不内置任何真实设置；加一条 = 清单里添一项（示意：`{Name: "title", Type: FieldTypeString, Default: "我的博客"}`；README 给完整配方）。

## 6. 行为（五个动作）

| 动作 | 行为 |
| --- | --- |
| 读一项 | 清单内、库中无行 → 默认值；有行 → 按类型校验后返回；行内容对不上声明 → 记日志 + 回退默认值（唯一静默兜底）；不在清单内 → 错误 |
| 读一整组 | 该组全部项的"默认值 ⊕ 覆盖"合并结果（供将来整组渲染）；未知组 → 错误 |
| 改一项 | 按清单类型把关（不符即错）；按复合键 upsert；未知组 / 未知项 → 错误 |
| 一组一起改 | 单一事务，要么全成要么全不算（供将来控制台"保存一整组"） |
| 删一项（恢复默认） | 删除该行；行不存在按成功处理（幂等——两种情况的读取结果都是默认值） |

错误语义：未知组、未知项、值类型不符 → 显式报错（哨兵错误，供后续接口层映射）。日志只记组名 / 项名 / 错误分类，不记值本身。

## 7. 模块与装配

- 位置：`internal/modules/settings/`，无路由的能力模块——照 `mail` 惯例：不建 controller / dto / module.go，只提供实现文件 + `New` 入口 + 测试（文件划分拟 `registry.go` / `types.go` / `service.go`，计划定稿）。
- 入口：`settings.New(db) (*settings.Service, error)`——构造即自检，错误沿启动链返回；`app.New` 相应为 `(*App, error)`（`main.go` 启动链本就是错误返回风格）。
- 装配：`app.New` 构造并持有 settings 实例（对齐 `mailer` 的处理），供后续轮次取用；**不注册任何路由**。
- 生成脚本：`scripts/db-generate.sh` 增加 `--feature sql/upsert`（ent 既有特性，生成标准 `ON CONFLICT`；标注 Experimental——低风险，备选为事务内先查后写）。
- 测试注入：生产入口用内置清单；包内另有细粒度构造供测试注入样例组（测试与实现同包，沿用 access / mail 惯例）。

## 8. 测试

| 层 | 连库 | 内容 |
| --- | --- | --- |
| 清单与类型（单测） | 否 | ① 自检三规则（格式 / 重复 / 默认值类型）各证一例通过与否；② 写入把关（数字收整数拒文字、开关收布尔拒字符等） |
| 存取（集成，`DATABASE_URL_TEST` 缺省 skip） | 是 | ① 三种类型写入后读回一致；② 未写过的项读出默认值；③ 删行后读回默认且行确实没了；④ 整组读取合并正确；⑤ 一组一起改的事务性（含非法项 → 全组不落库）；⑥ 坏行（绕过写入直接改库）读回默认且不报错；⑦ 样例组全程零迁移——证"新增设置不碰数据库" |
| 全量回归 | 否 | `go build` / `go vet` / `gofmt -l` 干净；无库 `go test ./...` 全绿（集成用例缺省 skip） |

集成测试数据用随机后缀 + `t.Cleanup` 清理（沿用 access 轮风格）。

## 9. 已知缺口与风险

- **休眠窗口**：清单为空、无消费方期间，表恒空、模块无生产调用——与 2FA 轮"上线即休眠"同性质；首次真实使用（加组 + 消费方）只涉代码，不动库。
- **`sql/upsert` 特性标注 Experimental**：ent 既有特性，生成的是标准 `ON CONFLICT` 语句；不接受时备选为事务内"先查后写"。
- **类型面窄**：三种类型、数字限整数、无范围校验；首个需要小数 / 选项 / 长度限制的设置出现时按加类型处理——代码变更，不动表。
- **值可直接读**：本表不做加密，不适合放密钥类；启用密钥类时以信封对象入同一 `value` 列 + 读取路径屏蔽（§10）。
- **并发**：同一项两端同时写 → 后写覆盖（行级，无版本号）；单管理员场景足够，需要时再评估乐观锁。

## 10. 插槽（本轮不定，定它的时点）

| 插槽 | 什么时候必须定 | 影响面 |
| --- | --- | --- |
| 接口（控制台按组管理设置） | 控制台轮 | 新 controller + 权限点 + apierror 映射 |
| 公开读取（前台匿名读站名等） | 前台开始消费设置时 | 读取路径加公开过滤（清单项加标记） |
| 密钥类加密 | 第一个密钥设置出现之前 | 值信封格式 + 读取屏蔽 + 根密钥环境变量 |
| 类型之外的校验（范围 / 长度 / 选项） | 首个需要的设置出现时 | 清单项加字段 + 写入把关扩展 |
| 主题的动态设置空间 | 主题包机制落地时 | 同构第二张表（多一段主题维度）或 scope 列 |
| 导入导出 | 需要时 | 快照 / 回写路径 |

## 11. 验收标准

1. `bash scripts/db-generate.sh` 重生成后 `go build ./... && go vet ./... && gofmt -l .` 干净；生成代码与 schema 一起提交。
2. 迁移 SQL 按 §4 审阅清单核对、`atlas migrate hash` / `validate` 通过、应用到两个库。
3. 有库：§8 集成用例全过；无库：`go test ./...` 全绿（集成用例缺省 skip）。
4. README 更新（迁移清单 + "设置"章节与"怎么加一个设置"配方）；`docs/DEVLOG.md` 加条目；版本 `./scripts/bump-version.sh minor` 进位。
5. 路由注册与现状一致（本轮零新增）；无新增依赖。

## 12. 下一步

本文经用户审阅通过后，进 writing-plans 出实施计划（范围 = §3，验收 = §11）。
