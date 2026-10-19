# NeoBlog 后端工程风格

本文件约束 `backend/` 的模块内部工程风格。接口、环境变量与运维契约以 [README.md](README.md) 为准；HTTP 表达的目标态以 [API 设计规范](../docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md) 为准。代码重构的落地顺序见 [2026-10-20 风格规范化计划](../docs/superpowers/plans/2026-10-20-neoblog-backend-style-normalization.md)。

## 0. 地位与优先级

- 本文件描述的是工程风格，不重新定义业务语义与接口契约。
- 冲突裁决：早期设计文档的决策条款 > README > 现有实现中重复出现的写法。
- 标为「目标态」的条目尚未实施：不能拿来验收当前代码，也不得在日常迭代中提前实施。
- 标为「例外」的条目是已经接受的现状，不是待清理项；删除例外必须单独评审。

## 1. 模块解剖

一个功能一个目录：`internal/modules/<name>/`。

有路由的模块必须自包含：

| 文件 | 职责 | 禁止 |
| --- | --- | --- |
| `module.go` | 装配、`New`、`Register` / `RegisterProtected` | 业务逻辑、SQL、HTTP 编解码 |
| `controller.go` | 路由声明、请求绑定、响应编码、守卫挂载 | 数据库访问、跨层调用 |
| `service.go` | 业务规则、事务编排、跨模块能力调用 | import gin、直接拼 JSON |
| `dto.go` | 模块的数据形状：请求 / 响应 / 必要的领域类型 | 业务逻辑、数据库访问 |
| `repository.go` | ent 查询、事务、领域记录转换 | `apierror`、HTTP 状态码、gin |

无路由的能力模块可以省掉 `controller.go` / `dto.go` / `module.go`，只提供实现文件、`New` 入口与测试；不要为凑形状建空文件。

功能文件按职责拆：`totp.go`、`session.go`、`validate.go`、`registry.go`、`types.go` 都属于正常写法；单文件过大时按关注点继续拆，而不是把逻辑堆回 `service.go`。

`module.go` 只做装配与注册，不写业务判断。

`mail` 有路由但按能力模块处理：无 `service.go`，controller 直接调用注入的 `Mailer`；这是 §9.1 的既定例外。

## 2. 依赖方向与跨模块边界

- 单向依赖：`controller → service → repository`。禁止反向调用，禁止跨层直连。
- `repository` 是具体类型，不抽仓储接口（数据层设计「不隔仓储接口层」）。
- 消费者只需要一部分能力时，由消费方声明窄接口（如 health 的 `pinger`）；不要为仓储导出接口。
- 模块之间不互相 import 业务实现。跨模块能力只在 `app.go` 里显式注入：要么注入对方的 service，要么注入能力接口（如 `mail.Mailer`）。
- 共享表可以由各模块自己的 repository 读写（access 读 `users` 是既有先例），但不得复制对方的业务规则；边界靠 DTO。
- `internal/database` 不得 import 任何业务模块。
- Gin 与 HTTP 细节只允许出现在 `controller.go`、`module.go`、`app`、`internal/common/guard`、`internal/common/apierror`、`internal/common/apiresponse`；service 一律不 import gin。

## 3. 错误分层

- 硬规则：`repository.go` 不 import `apierror`。repository 只返回域哨兵（如 `errRecordNotFound`）或原始数据库错误；由 service 翻译。`scripts/check.sh` 里有对应的 grep 门禁。
- service 负责把域错误翻译成 `apierror`；controller 只调用 `apierror.Write`，不得在 controller 里现造错误码。
- settings 是既定例外：service 保留模块哨兵（`ErrUnknownGroup` / `ErrUnknownItem` / `ErrTypeMismatch` / `ErrMethodMismatch` / `ItemError`），由 controller 的 `writeSettingError` 映射成 `apierror`。这是设置模块「哨兵错误供接口层映射」的原始设计。
- 新增错误码必须先登记 `internal/common/apierror` 的注册表；错误码词汇只增不改义。
- 内部错误字符串小写，带包名或域前缀（`auth:`、`access:`、`settings:`、`database:`、`mail:`、`guard:`）；不得包含密码、token、恢复码、TOTP secret。例外：config 的启动校验错误直接点名环境变量或行号，`apierror.Error()` 返回 wire code，两者都不加前缀；第三方库错误原样透传时不强制加前缀（本仓构造的错误仍要加）。
- 未知错误一律落到 `internal_error`；不得把 SQL、栈、内部路径泄漏到响应里。

错误信封固定为：

```json
{"error":{"type":"...","code":"...","param":null}}
```

不含 `message` 字段；HTTP 状态码按现有注册表映射。

## 4. DTO 与数据类型

- 线上请求 / 响应类型放 `dto.go`，命名 `XxxRequest` / `XxxResponse`。
- access 的 `Group` / `GroupRef` / `UserSummary` / `UserDetail` 是模块 DTO，保留现名；该模块的边界靠 DTO。
- 模块 service 的输入 / 输出 DTO 作为能力契约保持导出；controller 的 request / response 可以导出也可以未导出，但必须放在 `dto.go`。
- settings 的 `PATCH /admin/settings/:group` 是动态键请求体，允许 controller 直接绑定 `map[string]json.RawMessage`；键集合由清单约束。
- 领域类型（无 JSON tag）可以放 `dto.go` 或 `service.go`；不得为了复用给领域类型硬加 wire tag。
- service 可以返回领域类型或模块 DTO；最终响应形状由 controller 决定。
- 集合字段为空时必须输出 `[]`，不能输出 `null`；标量可空字段输出 `null`。
- JSON 字段一律 snake_case，连续缩写整体小写（`api_key`、`user_agent`）。
- 时间字段按当前 README 口径输出；epoch 秒是目标态，见 §5。

## 5. 响应形状

当前生效：

- 单资源：裸 JSON 对象，不套 `{"data": ...}`。
- 集合：`{"items":[...]}`。
- 删除：`{"id":"...","deleted":true}`；批量删除：`{"ids":[...],"deleted":true}`。
- 无负载成功：200 + `{}`；不使用 204。
- 创建资源：201 + 裸对象。
- 错误：§3 的信封。
- `/healthy` 是探针端点，200 / 503 都返回健康状态对象（Go 类型 `HealthResponse`，JSON 形状不变），不套错误信封。
- 集合与删除确认的共享类型在 `internal/common/apiresponse`：`ItemList[T]` / `Deleted` / `DeletedBatch` / `Empty`；`ItemList` 与 `DeletedBatch` 会把 nil 集合输出为 `[]`。

目标态（尚未实施）：

- 集合信封改为 `data` / `total` / `limit` / `offset` 并接入分页。
- 时间戳改为 epoch 秒整数。
- `/healthy` 的 `time` → `reported_at`、`uptime` → `uptime_seconds`。
- 认证响应补 `id`。

以上条目只随显式的 API 规范化批次实施，日常迭代不得提前改。

## 6. 命名与代码组织

- 包名小写单数。
- 接收者：`Module` 用 `m`，controller 用 `c`，`Service` 用 `s`，repository 用 `r`。
- `gin.Context` 与 `context.Context` 都叫 `ctx`，靠类型区分；handler 参数用 `ctx`。
- 模块构造器用 `New`；包内构造器用 `newXxx`。
- 只导出跨包使用的 helper 类型 / 函数；模块的 HTTP DTO 与 service 输入 / 输出 DTO 可以保持导出，作为契约类型。现有跨包例外：
  - `auth.User` / `auth.UserResponse`：`internal/app/app_test.go` 使用。
  - `auth.CreateAdminUser`：仅供测试建号，业务代码不得调用。
  - `settings.New` / settings 的 `Group` / `Definition` / `FieldType` / `ItemWrite` / `GroupValues` / `ItemError`：清单与测试注入的既有接口。
  - `mail.Mailer` / `Message` / `Body` / `Render` / `ErrClosed`：能力模块的对外契约。
- import 分三组：标准库、第三方、本仓；格式由 gofmt 保证。
- 零注释：只有 `//go:build`、`//go:embed` 等工具链指令。说明写 README / STYLE / spec / DEVLOG。
- 运行期日志用 `log/slog`，键值对；`main.go` 的启动引导是唯一标准库 `log` 例外，本轮不迁。
- 不用 `fmt.Printf` 打日志；不记录密码、token、恢复码、TOTP secret；邮件正文只允许在未配置 SMTP 的 log sender 开发态降级里记录，生产 SMTP 路径不记录。

## 7. 测试

- 测试与实现同包，白盒。
- 集成用例必须打真库；`DATABASE_URL_TEST` 缺省时按既有约定 `t.Skip`。
- 全仓测试必须经 `bash scripts/test.sh`（包串行 `-p 1`，消除注册开关的跨包竞态）。
- helper 必须 `t.Helper()`；每个包自带 `testDB` / `uniqueTag` / `createTestUser`，不建共享 testutil。
- 判定表用表驱动；每个行为修复先写失败用例。
- 纯移动 / 重构必须保持现有行为测试全绿，不为重构改测试预期。

## 8. 文档与流程

- spec → plan → 实现 → DEVLOG；接口变化同步 README；版本用 `scripts/bump-version.sh`。
- 提交信息 `type(scope): 中文描述`；默认一次提交，用户要求再拆。
- 静态检查：`bash scripts/check.sh`（gofmt / go vet / go build / whitespace / repository 不得 import apierror）。
- 测试：`bash scripts/test.sh`。
- 应用启动绝不自动迁移；迁移走 Atlas 流程并人工审阅 SQL。

## 9. 存量例外与当前差距

§9.2 是规范先行于实现的待办，不是允许新代码继续照做的例外；新代码一律按 §1–§8 的现行条款执行。

### 9.1 已接受的例外

| 现状 | 原因 |
| --- | --- |
| `mail` 无 `service.go` | 能力模块；controller 经注入的 `Mailer` 调能力 |
| settings 用模块哨兵 | 哨兵携带项名上下文，由接口层映射 |
| access service 返回带 JSON tag 的 DTO | 模块边界靠 DTO |
| `main.go` 用标准库 `log` | 启动引导只打生命周期日志；运行期模块用 slog |
| access 读 `users` 共享表 | 避免 auth ↔ access 双向依赖；边界靠 DTO |
| 各包复制 `testDB` / `uniqueTag` | 避免 testutil 反向耦合业务模块 |
| `auth.CreateAdminUser` 导出 | 仅供测试建号 |
| `/healthy` 不套错误信封 | 机器探针语义 |
| settings 的注册表类型导出 | 清单与测试注入的既有接口 |
| config 启动校验错误 / apierror wire code 不加前缀 | 前者点名环境变量或行号；后者就是客户端依赖的 code |
| 第三方库错误原样透传 | bcrypt / scs / go-mail 的错误保留原始上下文；本仓构造的错误仍加域前缀 |
| mail 的 log sender 记录邮件正文 | 未配置 SMTP 的开发态降级；生产 SMTP 路径不走该实现 |
| settings 的 PATCH 直接绑定动态键 map | 键集合由清单约束 |
| 模块 DTO 可保持导出 | 作为 HTTP / service 契约；helper 类型与函数只在跨包时导出 |

### 9.2 当前差距

2026-10-20 已收敛：共享集合信封、DTO 归位、access repository 去 `apierror`、检查门禁、错误字符串前缀。剩余：

| 规则 | 现状 | 对应计划 |
| --- | --- | --- |
| 日志可观测性 | 无 request id；模块用 `slog.Default()`，没有统一注入 | 另开日志轮次 |

对应计划：[2026-10-20 后端工程风格规范化](../docs/superpowers/plans/2026-10-20-neoblog-backend-style-normalization.md)。

## 10. 变更记录

- 2026-10-20：首版。从 2026-10-11 的 auth / access / mail / settings / data-layer 设计文档、README 硬性规则与现有实现提炼；明确目标态、已接受例外与已知差距。
- 2026-10-20：内部结构收敛。共享 `apiresponse`、DTO 归位、access repository 去 `apierror`、错误字符串前缀统一、检查门禁落地；目标态 API 形状仍未实施。
