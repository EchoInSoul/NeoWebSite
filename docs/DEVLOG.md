# 开发日志

按时间**倒序**记录每个开发阶段的变更、决策与已知缺口。设计细节与实施计划在 `docs/superpowers/{specs,plans}/`；接口与结构规范以 `backend/README.md` 为准。

---

## 2026-10-11 · 两步验证表结构预留（TOTP，仅落库）

**版本**：0.5.1 → **0.6.0**（minor：新能力域的数据插槽）

### 做了什么

- 兑现认证设计 §2 / §11 的预留（TOTP + 自管恢复码，库选 `pquerna/otp` 但本轮不引入）：`users` 加 `totp_secret` / `totp_enabled_at` 两个可空列（ent Optional + Nillable，组合出「未设置 / 设置中 / 已启用」三态）、新表 `recovery_codes`（`user_id` 级联外键 + `code_hash` + `used_at` 一次性标记）。接口、登录分叉、恢复码生成逻辑全部下一轮；当前无用户启用、无任何写入路径，上线即现状。
- 迁移 `totp_and_recovery_codes`（纯 DDL：加两列 + 建表 + 索引，无回填无 seed），人工审阅后 hash / validate、应用到 `neoblog` 与 `neoblog_test`；`db-migrate-diff.sh` 的迁移名字面量再换一次（仍是写死字面量、未参数化）。
- 迁移后置断言 `TestTOTPSchemaReserved`（auth 模块，`DATABASE_URL_TEST` 缺省 skip）：可空性、Nillable 读写回程、恢复码行随用户删除级联消失、`used_at` 默认 nil——全部用 ent 行为实证，不查 `information_schema`。

### 过程中的坑

- ent 命名转换不保留缩写词大写：`totp_secret` 生成的 Go 字段是 `TotpSecret`（不是 `TOTPSecret`）；计划原写法有误，以生成物为准。
- auth 包测试 helper 已有同签名 `testDB` / `uniqueTag`（`repository_test.go`），新测试文件重复声明直接编译失败——删重复声明、复用既有 helper。

### 已知缺口

- `recovery_codes` 在下一轮接口落地前是死表；`code_hash` 暂不设长度校验（写入方下一轮实现时钉 64 位 hex）。
- secret 按 base32 明文入列：DB 泄露即可伪造 TOTP。休眠期无存量，应用层加密可在任何用户启用前无痛补上（这正是先落结构、暂不启用的动机）。
- 登录限流仍未做（用户裁定与本轮分开，见认证章节已知缺口）。

### 关键决定

- 先落库表结构、暂不启用：插槽钉死后，下一轮接口不动迁移；secret 加密升级窗口留在无人启用阶段。
- 恢复码哈希用 sha256（高熵随机串无需慢哈希）；将来换低熵可读码时改算法不动表。

## 2026-10-11 · 邮件模块归位审查与修复

**版本**：0.5.0 → **0.5.1**（patch：测试基础设施缺陷修复 + 规范说明补全）

### 背景

邮件模块初版放在 `internal/mail/`（被当成与 `config` / `database` 平级的基础设施），用户指出它违背 README 硬性规则 1（一个功能一个模块目录）。归位提交 `56a8037`（`internal/mail` → `internal/modules/mail`，11 个文件 `R100`）后，用户要求派独立子代理**只读**复查。

### 过程（审查 → 验证 → 修复，可追溯）

1. **两路独立只读子代理审查**，角度错开：迁移完整性与引用面（逐字节比对 blob、残留引用、构建测试、git 历史完整性）／规范符合度与文档一致性（README 规则 1–8 逐条、三处文档互相一致、sed 批量替换残留）。两路**独立报出同一条真实缺陷**（下条），并另有规范说明与文档口径若干项。
2. **逐条实测验证**（不信转述）：`cd internal/modules/mail && test -f ../../.env` → 不存在（归位后指向 `backend/internal/.env`）；`test -f ../../../.env` → 存在（`backend/.env`）；`LoadDotEnvFile` 对缺文件静默返回 nil（`dotenv.go:16-19`）；README 规则 1/5 原文与 mail 实际形态逐条对照；spec/plan 里旧版本号（0.2.1 → 0.3.0）与旧路径描述逐条确认。**四条发现全部属实**。
3. **TDD 修复**：先写三个失败用例（`findEnvFile` 纯函数边界 / 「从包目录能找到 `backend/.env`」守护用例 / 经 `t.Chdir` 到临时目录树验证 `.env` 确实被载入到进程环境），跑出编译失败 → 实现「从包目录向上逐级查找 `.env`（深度上限 5）」并接入真实投递测试 → 全绿。守护用例专门钉住「包再移动导致静默失效」这一类回归；`TestRealDelivery` 的 skip 文案也点明了查找位置。
4. **文档同步**：spec 增 v4 修订记录 + §10 第 5 条改写 + §3/§14 版本表述修正；plan 增「实施后修订 2」+ Task 4 Step 5 改写；README 补规则 1/5/7 的「无路由模块」与「禁止自建 SMTP 客户端」说明、目录表 mail 行改写并移入模块行区域、「邮件」章节措辞与测试说明；本条目即过程记录。

### 缺陷根因

`realdelivery_test.go` 里写死的 `../../.env` 依赖包目录深度：归位只改了文件位置与含 `internal/mail` 字样的文本，这条不含路径 token 的相对深度没有跟着变；`LoadDotEnvFile` 对缺失文件静默返回 nil 又把问题藏住——用户按 README 填好 `backend/.env` 去跑真实投递测试时只会得到 SKIP，且没有任何提示。

### 关键决定

- **不写死相对深度**，改为有界向上查找（`findEnvFile` 深度上限 5）+ 守护用例：包位置再变会红，而不是静默失效。
- README 规则 5 采用「文档收编现状」方案（无路由模块由 `New` 直接返回能力接口、无状态包级纯函数可直接导出），而不是把 `mail.New` 包装成 `*Module`；若以后要形式统一，需同步改 spec/plan 与 `app.go`。

### 已知缺口

- 仅当变量已在进程环境导出或 `backend/.env` 存在且可读时，真实投递用例才会真跑；无凭据时按约定 SKIP。
- 邮件 spec/plan 仍是未跟踪文件（未入库），本次修订只存在于工作区。

---

## 2026-10-11 · 用户组与权限（层 2）

**版本**：0.4.1 → **0.5.0**（minor；认证层 2 与邮件基础设施先占用了 0.3.x / 0.4.x）
**提交**：`bbeef91`（rbac）→ `a3e5474`（版本进位）；终审修复 `f48568c`；设计与计划存档 `6088bf5`

### 做了什么

- 权限目录 `internal/common/rbac`（纯 Go，不依赖 gin / DB）：`模块.动作` 命名、动作只有 `view` / `edit`（`edit` 蕴含 `view`）、四个权限点（`system.users.view|edit`、`system.groups.view|edit`）、预设组常量（owner / admin / 普通用户 + 固定 UUID）。
- 新模块 `internal/modules/access/`：组 CRUD、用户管理接口（列表 / 详情 / 设组 / 设额外权限）、生效权限解析；路由全部挂在权限守卫下。
- 数据模型：新增 `groups` / `group_permissions` / `user_permissions` 三张表 + `users.group_id`（非空外键，ent 字段级默认值指向「普通用户」，**创建路径零改动**）；迁移里 seed 三个预设组、三步回填存量用户（生成物把 `ADD COLUMN … NOT NULL` 与外键写在同一句，人工拆开）。
- 守卫扩展：`guard.LoadIdentity`（解析一次身份与生效权限放进 `auth.identity`，用户已删除则销毁会话 + 401）与 `guard.RequirePermission`（权限不足 403 `forbidden`）。
- `/auth/me` 扩展为带 `group` 与 `permissions`；`app.go` 装配 access 模块与 `RequireAuth → LoadIdentity → RequirePermission` 链。
- `scripts/db-generate.sh` 开 `--feature sql/lock`（last-owner 检查用 `FOR UPDATE`）；`db-migrate-diff.sh` 的迁移名由 `init_users_sessions` 换成 `groups_and_permissions`（仍是写死字面量，未参数化）。
- 测试：rbac / guard 单测（不连库）+ 迁移后置断言 / service / HTTP / 端到端集成（`DATABASE_URL_TEST` 缺省 skip）。

### 制定阶段修掉的坑（设计 → 计划）

设计前后各派过独立子代理：先摸清参照项目（D 盘 HaoBlog）的模型与其生态代价，再把写好的计划交给两路只读子代理——一路查「照做会不会撞墙」（逐条核对 ent / Atlas / scs 行为），一路做安全红队（穷举提权路径与覆盖缺口）。真正影响实现的几条：

- **组转移的授权差集没钉死**（红队判高）：原计划只写「只能授出自己持有的权限」，差集口径留白——若实现成「只对额外权限做差集」，两步即可提权（先把自己有的权限塞进一个组、把用户塞进去，再借那个用户反向给自己加权限）。现设计 §6 规则 1 写死：`next 生效集 − current 生效集`、同一请求内组与额外权限的变更**合并计算**、两侧先做蕴含展开。
- **`last_owner` 串行路径根本打不出来**：自己不能改自己、非 owner 碰不到 owner 组成员 ⇒ 串行下 owner 数不可能降到 0；原计划却让 HTTP 用例串行断言 409，会诱导实现把检查提前（把 403 变成 409）。现规定检查顺序 `cannot_modify_self` → `owner_protected` → `last_owner`，409 只留给并发用例。
- **版本口径与仓库现状脱节**：计划初稿写死 0.2.1 → 0.3.0（该原稿未随存档保留、无从核对；存档版写为「当前 0.3.1 → 0.4.0」并已带「若届时已被其他层进位按当前版本顺延」规则），实际执行时已被认证层 2 与邮件层推到 0.4.1。
- **迁移脚本的隐性前置**：`db-migrate-apply.sh` 带 `set -u` 且不读 `.env`，计划漏了导出 `DATABASE_URL` / `DATABASE_URL_TEST`，照做必报 unbound variable。
- 其余中级项：约束错误不能一刀切映射（组名冲突与外键冲突分开）、guard 单测里「会话已销毁」的断言是空转（第二次仍 401 只能证明 resolver 仍返回 not-found，改为断言清除 cookie）、输入校验整段缺失（组名长度 / 空 body / `group_id: ""` / `[]` vs `null`）、`/auth/me` 扩展与 app 装配拆开会留下红灯窗口（合并为一个任务）。

### 执行中发现并修掉的问题

- **ent 边声明的写法**：计划写的是 `edge.To("users", User.Type())`，生成器直接报 `not enough arguments in call to User.Type, want (User)`。回 ent v0.14.6 源码核实：`Schema.Type` 是边声明专用伪方法，取方法表达式、**不加括号**（`ent.go:27-41`），改为 `User.Type` 后生成通过。
- **迁移生成物在有数据的库上必失败**：`atlas migrate diff` 把 `ALTER TABLE users ADD COLUMN group_id uuid NOT NULL` 与外键写在同一条语句；人工拆成「加可空列 → `UPDATE` 回填「普通用户」→ `SET NOT NULL` → 加外键」，预设组 seed 落在 `CREATE TABLE groups` 之后，再 `atlas migrate hash` + `validate`（13 条语句应用到两库）。
- **三处测试期望写错，被实现的正确行为纠正**（改测试不改代码）：① 目标已在 admin 组时 `owner_protected` 优先于 `grant_forbidden`；② 「应该被拒」的用例里给操作者配多了权限，其实合法；③ 断言普通用户组有成员却没造人。
- **脚手架坑**：`task-done` 脚本在验证命令零输出时不记账（`grep -v` 空输入退出 1 + `set -e` 静默中止），后续命令统一追加哨兵输出。

### 终审与修复

独立子代理整分支终审（fresh context；明确排除并行会话的邮件层提交）：**0 Critical / 2 Important / 6 Minor**，5 条预设审查焦点（组转移差集、`last_owner` 顺序与可达性、owner 保护两向、`edit` 蕴含 `view`、并集撤权假象）逐条通过，三条攻击者视角未发现接口组合提权路径。

- **已修（先复现后修）**：并发 `PATCH` 同一个组或同一个用户时，替换是「先删后插」，READ COMMITTED 下第二笔看不到第一笔未提交的行，随后 `INSERT` 撞唯一索引 → **裸 500**。先写 `TestConcurrentReplaceKeepsBothWritesServing` 复现（组 / 用户两条路径各 5 轮并发，3 次运行里稳定打到 500），再在替换事务内先 `FOR UPDATE` 锁父行（组 / 用户）串行化 → 15 轮并发全 200、全量套件绿。根因：替换事务之间没有可串行化的锚点。
- **裁定不修**：owner 保护判定用的是**请求开始时的快照**——并发把目标提升进 owner/admin 组的瞬间，非 owner 可能刚好完成一次对目标的额外权限编辑。不产生越权（可授出集合仍受差集限制）、不损坏数据，与设计 §13 已写明的「在途请求用旧快照」同属一种语义；要堵得把整段判定搬进事务，收益不抵复杂度。
- **未处理的小问题（待定）**：README「新用户由数据库默认值落普通用户」措辞（实为 ent 字段级默认值；后已修正）；删空组后权限行级联消失未断言；用户列表「按创建时间倒序」的断言是空转；`rbac.Expand` 对目录外的 `x.edit` 也会合成 `x.view`（仅脏数据可触发）；约束映射一刀切（未来新增约束需同步）；`/auth/me` 在 `LoadIdentity` 之后多查一次用户。

### 验证

- `gofmt -l` / `go vet` / `go build` 干净，`atlas migrate validate` 通过；带库与不带库各跑一次 `go test -p 1 ./...` 全绿（access 集成用例依赖 `DATABASE_URL_TEST`，缺省 skip；**带库全量用 `-p 1`**——access 与 app 都会造 owner，包级并行会互相干扰）。
- 手工 curl 清单：普通用户 403、owner 200、`/auth/me` 带 group 与 permissions、预设组改动 403 `preset_group_locked`、重名 409 `group_name_taken`、非空组删除 409 `group_not_empty`、改自己 403 `cannot_modify_self`、目录外权限 key 400 `invalid_permission`、删空组 204、用户被删后旧 cookie 401。
- 迁移后置断言（`TestMigrationSeed`）：三预设组按固定 UUID / key / 名称存在、权限行数为 0、**不显式设组建用户自动落「普通用户」**（ent 字段级默认值在 v0.14.6 生效的实证）。

### 关键决定

- **预设组权限由代码计算、不入库**（owner / admin = 全量、普通用户 = 空），只有自建组写 `group_permissions`——参照项目（D 盘 HaoBlog）因为「权限行入库 + 首建号一次性 seed」导致新增权限要手工补种、漏种即全站锁死（权限从库里读，fail-closed），这里从根上绕开：加权限点只改代码。
- 生效权限 = 所属组 ∪ 用户额外权限（都做蕴含展开）；**只能授出自己持有的权限**（按目标生效权限集的前后差集、同一请求合并计算）、**摘除不受限**、**不能改自己**、**owner / admin 组的成员只有 owner 能改**、**至少保留一个 owner**（409 `last_owner` 只在并发双降级时可达，串行路径被前两条挡住）。
- 组以 UUID 标识、组名唯一（不做 slug）；带成员的自建组不可删；预设组不可删 / 改名 / 改权限。
- 组名 trim 后 1–64 字符；读接口一律返回展开后的权限集，写接口收原始勾选。
- 守卫原则变更：从「不查库」改为「只依赖 `app.go` 注入进来的 resolver，不 import 业务模块」。
- 权限改动**下一次请求即生效**（不吊销会话）；在途请求使用请求开始时的权限快照。

### 已知缺口

- **首个 owner 只能手工指定**（`UPDATE users SET group_id = '00000000-0000-4000-8000-000000000001' WHERE email = '…'`）；「首次建号入口」仍待定。
- 登录 / 注册仍无限流；角色与权限操作无审计留痕；用户列表不分页、不做可见性遮蔽；组编辑是「后写者获胜」（无乐观锁）。
- 权限目录删除权限点会留下 DB 孤儿行（本轮不清理）。

---

## 2026-10-11 · 邮件基础设施（只做后端）

**版本**：0.3.1 → **0.4.1**（minor 0.4.0 落地；模块归位 patch 0.4.1，归位审查修复见上一条）

### 做了什么

- 新增模块 `internal/modules/mail/`（无路由；对外入口 `New(cfg) Mailer`，不含 controller / dto）：`Mailer` 接口（`Send` / `Close`，`Close` 后 `Send` 返回哨兵 `ErrClosed`）+ `Message` / `Body` + `Render(name, data)`；业务代码不 import go-mail。
- 发送链路：缓冲队列（32）+ 单 worker 串行投递；每封最多 3 次尝试、退避 1s / 5s、单次 20s 超时；**4xx 与连接类错误重试、5xx 不重试**（判定用 `SendError.IsTemp()` / `ErrorCode()`——go-mail 把网络类失败判为「永久」，只信 `IsTemp()` 会一次抖动就丢信）；失败记 `log/slog` 结构化日志。
- 双实现：SMTP（`wneessen/go-mail` v0.8.1；`starttls` / `ssl` 两种 TLS 模式、无用户名则不做 AUTH、有用户名走 `SMTPAuthAutoDiscover`）与日志（**`SMTP_HOST` 为空即降级**，邮件全文写日志，开发环境无凭据可跑通全流程）。
- 模板：`templates/<name>.html|txt` 成对 + `embed.FS`；HTML 侧 `html/template` 自动转义；内置一封 `example`（邮件配置自检）。
- 配置：7 个 `SMTP_*`（`SMTP_HOST` 为空时全部跳过校验，其余 fail-fast，含端口 ≤ 65535）与 `LoadDotEnvFile(path)`（`LoadDotEnv` 变其包装，行为不变）。
- `app.go`：装配 `mail.New(cfg)`；`server` 提为字段并抽出 `shutdown(ctx)`（先 `server.Shutdown` 再 `mailer.Close`，共用 10s 关闭预算）。
- 文档：README（目录表 / 环境变量表 / 新增「邮件」章节 / 认证章节的「没有邮件基础设施」措辞更新）、`.env.example`（腾讯云 SES 为主示例）。

### 验证

- 单测（无外部依赖）：队列满阻塞、`Close` 后 `Send` 返回 `ErrClosed`、4xx 重试 / 5xx 不重试 / 重试耗尽记日志、drain、`Close` 幂等、并发 `Send`/`Close` 不 panic、日志实现内容与缺省 From。
- 集成（进程内 `emersion/go-smtp` + 自签证书，不依赖 Docker 与外网）：multipart 双版本正文、**中文主题 RFC 2047 可解**、550 永久 / 451 临时 / 连接被拒可重试。
- 真库：`TestShutdownClosesMailer`（关闭 drain 后 `Send` 返回 `ErrClosed`）；带与不带 `DATABASE_URL_TEST` 各跑一次全量 `go test ./...` 全绿；`gofmt -l` 与 `go vet` 干净。
- 可选真实投递 `TestRealDelivery`：无凭据时 SKIP；需在 `.env` 填好 `SMTP_*` 与 `MAIL_TEST_RECIPIENT` 后手动跑。

### 关键决定

- 选型 `wneessen/go-mail` v0.8.1（依赖仅 x/crypto + x/text）；排除：`gomail.v2` 停在 2016、`jordan-wright/email` 停在 2021、标准库 `net/smtp` 已 frozen 且无 context。
- **不做持久化 outbox**：Gitea / Vikunja 的生产实现都是「异步即发、失败只记日志」，本项目量级用「重发」兜底；触发条件＝出现「邮件丢了不可恢复」的流程。
- 不引入 `riverqueue/river`：`InsertTx` 需 `*sql.Tx`，而 ent 的 `*ent.Tx` 不暴露底层事务，原子入队要绕非公开 API。
- 不做应用内 DKIM（域验类服务商侧签）、不做连接复用（每封 Dial + Close）、不做退信处理。
- 包位置：按 README 硬性规则 1 归入 `internal/modules/mail/`（初版误放在 `internal/mail/`，用户裁定后迁移，版本进位 0.4.1）。

### 已知缺口

- `internal/modules/mail` **没有任何消费者**（注册不验证邮箱）；第一个消费方出现时在 `app.go` 注入。
- 内存队列无持久化：进程崩溃丢在途邮件；`Close` 返回错误 ≠ worker 已停（退避不响应关闭，上界约 66s）。
- 无退信 / 投诉处理与抑制表、无连接复用、无应用内 DKIM、模板无多语言——触发时点见 `docs/superpowers/specs/2026-10-11-neoblog-mail-design.md` §12、§15。

---

## 2026-10-11 · 认证层 2（注册与更改密码）

**版本**：0.2.1 → **0.3.0**（minor）

### 做了什么

- 新接口（前缀 `/api/v1`）：
  - `POST /auth/register`（公开，注册即登录）→ 201 + 会话 cookie；校验与注册页 UI 对齐（邮箱小写入库、昵称 trim 后 2–32 字符且禁空白与 `@`、密码 ≥8 字符含 ASCII 字母与数字且 ≤72 字节）；错误码 400 `invalid_request` / `weak_password`、409 `email_taken` / `nickname_taken` / `already_taken`（并发竞态兜底）。
  - `POST /auth/password`（需登录）→ 204；**保留当前设备、吊销该用户其他所有设备的会话**（scs `IterableStore.All()` + `Codec` 按 `auth.user_id` 定位；零 schema 改动）；失败契约 fail-closed——吊销或写库失败都返回 500 且密码未改，旧密码可直接重试。
- 校验规则抽成 `internal/modules/auth/validate.go` 纯函数（表驱动单测，覆盖全角空格、ASCII 字母判定、`net/mail` 边界等）。
- 无 schema 改动、无新依赖、`app.go` 未改。测试新增 10 组 / 25+ 用例（双设备吊销、坏会话行容错、删号后改密码、已登录时注册切换会话、昵称/密码边界）。
- 文档：`backend/README.md` 认证章节重写（接口表 + 规则 + 已知缺口）；spec/plan 见 `docs/superpowers/{specs,plans}/2026-10-11-neoblog-auth-layer2*`。

### 计划评审

1 个独立子代理先审计划：2 Critical（scs API 名错、吊销顺序与失败契约自相矛盾）+ 3 Important（DTO 缺失、handler 与既有方法同名、路由归属写错）——全部修进计划后才开工；执行中编译期又抓到一处计划笔误（`IterableStore` 只有 `All()`、没有 `Delete`，删除改用 `Store.Delete`）。

### 已知缺口

- **登录 / 注册暂无限流**（本轮按用户要求不做；下一轮优先补，含部署时的可信代理配置）。
- 邮箱验证 / 找回密码（无 SMTP）、角色区分（还没有管理员专属接口）——均留到需要时。
- 前端未接：`/api` 代理与注册/登录表单由用户侧接线，后端只提供上面的接口契约。

---

## 2026-10-11 · 仓库换行符治理（.gitattributes）

**提交**：`acc8703`（`development`）；版本未变（后端 0.2.1 / 前端 0.1.0）。

### 问题

本机 git 的**全局** `core.autocrlf=true`（`D:/SoftWare/Git/etc/gitconfig`，不是仓库自己的配置）：索引存 LF、Windows 检出写 CRLF。于是任何让 git 重新写出文件的操作——clone、checkout、`reset --hard`、cherry-pick、rebase、重写历史——都会把文本文件在工作区变成 CRLF。`gofmt` 只认 LF，随即把 Go 文件报成「未格式化」，而 `git diff` 是空的（仓库内容其实没变）。重写认证分支历史后撞到过一次，当时误以为代码被改坏。

规模实测：**未加规则时克隆本仓库，158 个文本文件被检出为 CRLF，`gofmt -l` 报出全部 54 个 Go 文件。**

### 做了什么

- 新增 `.gitattributes`，内容一行：`* text=auto eol=lf`——在仓库内压过全局 autocrlf。
- 把当时工作区仍是 CRLF 的 5 个文件归一回 LF：`backend/README.md`、`backend/go.mod`、`backend/go.sum`、`docs/superpowers/{plans,specs}/2026-10-11-neoblog-auth*.md`。**跟踪内容零变化**（归一后 blob 与索引逐字节相同），因此本次提交只含 `.gitattributes` 一个文件。
- 例外情况无需处理：`frontend/app/favicon.ico` 是二进制（`text=auto` 自动跳过）、6 个单行 svg 没有行尾可转；仓库内没有 `.bat/.cmd/.ps1`，6 个 `*.sh` 本就要求 LF。

### 验证

临时克隆做 A/B（同一份内容，删除文件后强制重新检出全部文件）：

| 状态 | 重新检出结果 |
| --- | --- |
| 加规则前 | 158 个文件 CRLF；`gofmt -l` 报 54 个文件 |
| 加规则后 | 158 个文件全 LF；`gofmt -l` 无输出；`git status` 干净 |

规则提交后再克隆复验：160 个文件全 LF、`gofmt -l` 空、`git status` 完全干净——新克隆开箱不再出假警报。

### 排查与善后（复发时）

- 排查别先看 `gofmt -l`，用 `git ls-files --eol | grep -v 'w/lf'` 直接列出工作区非 LF 的文件；再用 `git diff -- <file> | wc -c` 为 0 确认仓库内容没变。
- 手动归一（`sed -i 's/\r$//'` 或对 Go 文件 `gofmt -w`）之后，这些文件会出现**假的 ` M`**：stat 缓存过期，blob 其实逐字节相同。`git update-index --refresh` 与 `git add --refresh` 都清不掉它（前者还会打印 "needs update" 误导人），必须 `git add -- <files>`；因 blob 相同，它不会暂存任何内容，用 `git diff --cached` 复核为空即可。

### 关键决定

- 采用全仓 `* text=auto eol=lf`，而不是最小化的 `*.go text eol=lf`：换行符与语言无关，前端 tsx/json/md 同样受 autocrlf 影响；`text=auto` 只对 git 判定为文本的文件生效，二进制安全。
- 用仓库内 `.gitattributes` 而不是改本机全局配置：规则随仓库分发，任何克隆与机器都免疫。

---

## 2026-10-11 · 认证层 1（只做后端）

**版本**：0.1.0 → **0.2.1**（minor＝新功能，patch＝补测试）

### 做了什么

- **接口**（前缀 `/api/v1`）：
  - `POST /auth/login`：body `{"identifier":"邮箱或昵称","password":"..."}` → 200 `{"email","nickname"}` + 会话 cookie；凭据错 401 `invalid_credentials`；请求不合法 400 `invalid_request`。
  - `POST /auth/logout` → 204（销毁会话）。
  - `GET /auth/me` → 200 `{"email","nickname"}`；未登录 401 `unauthorized`。
- **会话**：`scs` + Postgres 存储（`sessions` 表）；cookie 名 `session`、HttpOnly、SameSite=Lax、`Secure` 仅当 `APP_ENV=production`；有效期 `SESSION_LIFETIME`（默认 168h）；登录成功换发 token（防会话固定），登出与「用户已删除」都会销毁会话。
- **密码**：bcrypt；拒绝空密码与超 72 字节密码；未知账号也做一次等价比较（抗时序探测）。
- **数据**：`ent` schema `users` / `sessions`（对齐数据层设计的 ent + Atlas 路线）；Atlas 首个迁移进 `migrations/`（生成后人工审阅）；`users` 字段对齐 Neo 项目：`email` / `nickname` 双唯一 + `password_hash` + 时间戳。
- **结构**：`internal/modules/auth/`（module / controller / service / repository / dto）+ `internal/common/guard/`（`RequireAuth`，会话键 `auth.user_id`）；装配只在 `internal/app/app.go`；`/api/v1` 前缀仍只定义一次。
- **测试**：单元 + 真库集成——登录 / 登出 / me 全流程、cookie 属性（HttpOnly / SameSite / Path、production 才 Secure）、删号后旧会话吊销、存储故障返回 500、大小写邮箱、超长 / 空密码、畸形请求、cookie `Max-Age` == `SESSION_LIFETIME`；无 `DATABASE_URL_TEST` 时集成用例自动 skip。
- **文档**：`backend/README.md` 更新（认证章节、环境变量、目录对照、迁移工作流、硬性规则 3 / 5 补充）；规格与计划见 `docs/superpowers/specs/2026-10-11-neoblog-auth-design.md`、`docs/superpowers/plans/2026-10-11-neoblog-auth.md`。

### 移除（用户要求；已从分支历史移除，不是 revert 提交）

- 前端接线：`next.config` 的 `/api` 转发规则、登录表单提交接线、语言包两条错误文案。
- `cmd/create-admin` 管理员创建命令。

### 评审与修复

独立子代理整分支终审：**0 Critical / 2 Important / 5 Minor**。

- 已修（每个都先有失败测试）：
  1. scs 存储写失败时登录 / 登出曾「假成功」——防会话固定会静默失效、登出会假报 204；现在登录失败直接 500 不落会话，登出失败返回 500。
  2. `CreateAdminUser` 曾接受空密码（会造出永远登不上的账号）；现在拒绝。
- 未处理的小问题（待定）：`Only()` 的 NotSingular 未映射（多用户后「邮箱撞昵称」会 500）；测试库残留 `sessions` 行未清理；`.env.example` 未列 `SESSION_LIFETIME`；`dummyHash` 生成失败时静默降级。

### 已知缺口

- **没有「首个管理员」的建号入口**（命令已按用户要求移除、注册未实现）；首次建号方式待定。
- 登录接口**无限流**（有意留到层 2；现有缓解：bcrypt 慢哈希、错误信息不区分账号、未知账号等价比较）。
- **前端未接**：`/api` 代理与登录表单由用户侧处理；后端只提供上面的接口契约。
- 生产 cookie 域（前端域 / API 子域）未定，部署轮再处理。

### 关键决定

- 选型：`scs` + `bcrypt` + `ent` + Atlas（对齐已批准的数据层设计，取代早期草案里的 sqlc + goose）。
- 会话 cookie 由 **Go 自己 Set-Cookie**（早期 BFF 草案「Go 不 Set-Cookie、Next 种 cookie」未采用）。
- 控制器把非哨兵错误映射为 500（凭据 / 授权问题才是 401）；`auth.New(db, sessions)` 不带 `cfg`（模块不需要配置）。
