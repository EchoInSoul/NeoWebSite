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
- Gin 与 HTTP 细节只允许出现在 `controller.go`、`module.go`、`app`、`internal/common/guard`、`internal/common/apierror`；service 一律不 import gin。

## 3. 错误分层

- 硬规则：`repository.go` 不 import `apierror`。repository 只返回域哨兵（如 `errRecordNotFound`）或原始数据库错误。`access/repository.go` 目前仍直接返回 `apierror`，属于 §9.2 的待收敛差距；新代码必须遵守本规则。
- service 负责把域错误翻译成 `apierror`；controller 只调用 `apierror.Write`，不得在 controller 里现造错误码。
- settings 是既定例外：service 保留模块哨兵（`ErrUnknownGroup` / `ErrUnknownItem` / `ErrTypeMismatch` / `ErrMethodMismatch` / `ItemError`），由 controller 的 `writeSettingError` 映射成 `apierror`。这是设置模块「哨兵错误供接口层映射」的原始设计。
- 新增错误码必须先登记 `internal/common/apierror` 的注册表；错误码词汇只增不改义。
- 内部错误字符串小写，带包名或域前缀（`settings:`、`database:`、`mail:`）；不得包含密码、token、恢复码、TOTP secret、邮件正文。
- 未知错误一律落到 `internal_error`；不得把 SQL、栈、内部路径泄漏到响应里。

错误信封固定为：

```json
{"error":{"type":"...","code":"...","param":null}}
```

不含 `message` 字段；HTTP 状态码按现有注册表映射。

## 4. DTO 与数据类型

- 线上请求 / 响应类型放 `dto.go`，命名 `XxxRequest` / `XxxResponse`。
- access 的 `Group` / `UserSummary` / `UserDetail` 是模块 DTO，保留现名；该模块的边界靠 DTO。
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
- `/healthy` 是探针端点，200 / 503 都返回 `Status` 对象，不套错误信封。

目标态（尚未实施）：

- 集合信封改为 `data` / `total` / `limit` / `offset` 并接入分页。
- 时间戳改为 epoch 秒整数。
- `/healthy` 的 `time` → `reported_at`、`uptime` → `uptime_seconds`。
- 认证响应补 `id`。
- 集合信封收敛到共享的 `internal/common/apiresponse.ItemList[T]`。

以上条目只随显式的 API 规范化批次实施，日常迭代不得提前改。

## 6. 命名与代码组织

- 包名小写单数。
- 接收者：`Module` 用 `m`，controller 用 `c`，`Service` 用 `s`，repository 用 `r`。
- `gin.Context` 与 `context.Context` 都叫 `ctx`，靠类型区分；handler 参数用 `ctx`。
- 模块构造器用 `New`；包内构造器用 `newXxx`。
- 只导出跨包使用的类型 / 函数。现有导出例外：
  - `auth.User` / `auth.UserResponse`：`internal/app/app_test.go` 使用。
  - `auth.CreateAdminUser`：仅供测试建号，业务代码不得调用。
  - `settings.New` / settings 的 `Group` / `Definition` / `FieldType` / `ItemWrite` / `GroupValues` / `ItemError`：清单与测试注入的既有接口。
  - `mail.Mailer` / `Message` / `Body` / `Render` / `ErrClosed`：能力模块的对外契约。
- import 分三组：标准库、第三方、本仓；格式由 gofmt 保证。
- 零注释：只有 `//go:build`、`//go:embed` 等工具链指令。说明写 README / STYLE / spec / DEVLOG。
- 运行期日志用 `log/slog`，键值对；`main.go` 的启动引导是唯一标准库 `log` 例外，本轮不迁。
- 不用 `fmt.Printf` 打日志；不记录密码、token、恢复码、TOTP secret、邮件正文。

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
- 静态检查：`bash scripts/check.sh`（gofmt / go vet / go build / whitespace）。
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

### 9.2 已知差距（计划收敛，不算例外）

| 规则 | 现状 | 对应计划 |
| --- | --- | --- |
| 集合信封共享 | `itemList` 在 auth / access 各一份，settings 另有 view | 提取 `apiresponse.ItemList[T]` |
| DTO 归位 | access 的请求类型在 controller；settings / mail 无 `dto.go` | 搬回 `dto.go`，不改字段与 tag |
| repository 不 import `apierror` | `access/repository.go` 仍直接返回 `apierror` | 改域哨兵 + service 翻译 |
| 检查门禁 | `scripts/check.sh` 尚未拦住 repository 的 `apierror` import | 错误分层落地时补 grep 门禁 |
| 日志统一 | main 用标准库 log，运行期用 slog；无 request id | 另开日志轮次 |

对应计划：[2026-10-20 后端工程风格规范化](../docs/superpowers/plans/2026-10-20-neoblog-backend-style-normalization.md)。

## 10. 变更记录

- 2026-10-20：首版。从 2026-10-11 的 auth / access / mail / settings / data-layer 设计文档、README 硬性规则与现有实现提炼；明确目标态、已接受例外与已知差距。
