# NeoBlog 两步验证基础设施设计（TOTP，本轮仅落库表结构）

日期：2026-10-11
范围：`backend/`（Go + ent + Atlas）
状态：设计经用户确认（2026-10-11 对话：方式沿用认证设计的既有预留；范围 = 本轮只落库表结构、把插槽钉死，接口与登录分叉下一轮；登录限流与本轮分开、另开一轮）；本文为存档版，其后进 writing-plans 出实施计划

修订记录：
- v1（2026-10-11，本文）：首版。表结构 = `users` 加 `totp_secret` / `totp_enabled_at` 两列 + 新表 `recovery_codes`；本轮零 Go 业务代码改动、零接口，上线后行为与现状完全一致。

## 1. 意图与背景

2FA 不是新决策，是**兑现既有预留**。`docs/superpowers/specs/2026-10-11-neoblog-auth-design.md` 三处明文预留：

- §2 选型表：`TOTP 2FA（+ 恢复码）→ pquerna/otp + 自管恢复码 → 层 2`（方式与库已定）；
- §11 插槽表：「2FA 的表结构与恢复码｜层 2｜影响面：users / 新表、登录流程」（表结构是留给本轮定的决策点）；
- 同表预留更后项：API Token / Passkey / 社交登录，不受本轮影响。

用户已确认的执行策略：**先落库表结构、暂不启用**。本轮把数据插槽钉死后，下一轮的接口与登录分叉就不用再动迁移；在任何人启用之前，升级 secret 存储形态（如加密）也是零成本（见 §9）。

已定约束（用户）：

- 本轮只做库表结构与迁移；service / controller / 守卫 / 登录流程一行不碰。
- 当前没有任何用户启用两步验证，上线即休眠。
- 登录限流（README 已知缺口标记「下一轮优先补」）与本轮分开，不混做。
- 沿用既有规范：零注释、ent + Atlas 迁移工作流、`DATABASE_URL_TEST` 集成测试约定。

## 2. 已定决策

| 项 | 决定 | 落地时点 |
| --- | --- | --- |
| 方式与库 | TOTP（验证器 App）+ 自管恢复码；`pquerna/otp` | 沿用 auth 设计 §2（**下一轮**引入依赖，本轮 go.mod 不动） |
| TOTP 状态存放 | `users` 加两列（1:1 跟人走，不另立新表） | 本轮 |
| 恢复码存放 | 新表 `recovery_codes`（1:N，必须独立成表） | 本轮 |
| secret 存储形态 | base32 明文入列；启用前可无痛升级应用层加密（休眠优势，见 §9） | 本轮（升级留插槽） |
| 恢复码哈希 | sha256 hex（恢复码是高熵随机串，无需慢哈希；列只是 `text`，将来换 bcrypt 不动表） | 本轮（列结构） |
| 版本 | minor 进位（当前 0.5.1 → 0.6.0；执行时若已被其他层进位，按当前版本顺延） | 本轮 |

## 3. 范围

本轮做：

- ent schema：`User` 增加 `totp_secret` / `totp_enabled_at` 两个 Optional 字段与 `recoveryCodes` 边；新增 `RecoveryCode` schema；重生成生成代码并提交。
- 迁移：一个迁移（`users` 加列 + 建表 + 索引），`scripts/db-migrate-diff.sh` 的迁移名字面量再换一次；人工审阅、hash + validate、应用到 `neoblog` 与 `neoblog_test`。
- 测试：迁移后置断言（放 `internal/modules/auth/`，2FA 属认证域；`DATABASE_URL_TEST` 缺省 skip）。
- README 更新（数据库章节表清单 + 认证章节预留说明）、DEVLOG 条目、版本进位 minor。

本轮不做：任何接口（setup / enable / disable / 登录二段）；登录流程分叉；恢复码生成与验证逻辑；`pquerna/otp` 依赖引入；secret 加密；登录限流；`/auth/me` 暴露 2FA 状态。

## 4. 名词与模型

- **TOTP**：基于时间的一次性验证码（RFC 6238），验证器 App 扫 `otpauth://` URI 后每 30s 产生 6 位码；服务端持有共享密钥（secret）。
- **恢复码**：启用时一次性发放的高熵随机串，每个只能消费一次，用于验证器丢失时兜底；「自管」指生成、哈希、消费、再生成全部由本项目实现（不依赖库）。
- **三态**（`users` 两个可空列组合出的状态机，下一轮接口按此消费）：

| `totp_secret` | `totp_enabled_at` | 状态 | 登录行为（下一轮实现） |
| --- | --- | --- | --- |
| NULL | NULL | 未设置（现状，所有用户） | 单段登录，与今天一致 |
| 非 NULL | NULL | 设置中（已生成 secret、尚未验首码） | 单段登录（未启用不拦） |
| 非 NULL | 非 NULL | 已启用 | bcrypt 通过后进入二段验证 |

- 登录分叉只看 `totp_enabled_at`；「设置中」状态允许用户中途放弃（下一轮的 disable/重新 setup 直接覆盖 secret 即回到设置中）。

## 5. 数据模型（ent schema 摘要）

`users` 增量（两个 Optional 字段，全部可空、**无默认值、无回填**）：

- `totp_secret`：`field.String(...).Optional().Nillable()`——base32 密钥（pquerna/otp 默认 20 字节 → 32 字符）；**必须 Nillable**，Go 侧 `*string`，NULL 与空串可区分。
- `totp_enabled_at`：`field.Time(...).Optional().Nillable()`——Go 侧 `*time.Time`。
- 两个字段都不加索引：登录分叉发生在「已按密码找到用户之后」，按主键取行即可顺带读到，无需独立索引。

`recovery_codes`（新 schema `RecoveryCode`，形状对齐 `user_permission.go`）：

- `id`：uuid、主键、默认 UUIDv7（同 `user.go` 写法）
- `user_id`：uuid、必填、外键 → `users`，**级联删除**
- `code_hash`：String、NotEmpty（sha256 hex，64 字符；本轮不设长度上限校验，形状约束属下一轮写入路径）
- `created_at`：Default(time.Now).Immutable()
- `used_at`：Optional、Nillable（`*time.Time`）——非 NULL 即已消费；NULL = 可用
- 索引：`index.Fields("user_id")`（普通索引；Postgres 不为外键自动建索引，登录验证按用户查行）
- 级联注解挂 `User.recoveryCodes` 的 `edge.To` 侧：`edge.To("recoveryCodes", RecoveryCode.Type).Annotations(entsql.OnDelete(entsql.Cascade))`——**挂在 User 侧**（删用户清恢复码），与 `permissions` 边同一写法；`RecoveryCode` 侧用 `edge.From("user", User.Type).Ref("recoveryCodes").Unique().Required().Field("user_id")`

要点：

- ent 的 Optional 字段不进 INSERT 列清单（未显式设值时），因此**本节奏否存在 access 轮「生成代码带新必填列、旧库没有该列」的断窗**——重生成后未迁移的库照样能跑全部现有测试。
- 新表名由 ent 复数化得出：`RecoveryCode` → `recovery_codes`（与 `UserPermission` → `user_permissions` 同机制）。

## 6. 本轮消费规则（写给下一轮，表结构已支持）

- **setup**（下一轮）：生成 secret 写 `totp_secret`，`totp_enabled_at` 保持 NULL（三态之「设置中」）；重复 setup 覆盖旧 secret。
- **enable**（下一轮）：用当前码验证通过后写 `totp_enabled_at`，同请求发放一批恢复码（明文只回一次，库里只存 hash）。
- **disable**（下一轮）：要求密码确认后清 `totp_secret` 与 `totp_enabled_at`（回到「未设置」），并作废该用户全部未用恢复码。
- **登录**（下一轮）：bcrypt 通过后按 `totp_enabled_at` 分叉；二段端点验证 TOTP 码或任一未消费恢复码（命中即置 `used_at`）。pending 态用会话的独立键承载（如 `auth.pending_2fa_user_id`），`RequireAuth` 只认 `auth.user_id` 的现状保证 pending 会话天然不被守卫当已登录。
- **恢复码再生成**（下一轮）：作废旧批（置 `used_at` 或删除）+ 发新批；表结构无需变更。

## 7. 迁移

1. `scripts/db-migrate-diff.sh` 的迁移名字面量由 `groups_and_permissions` 换为 `totp_and_recovery_codes`（该脚本仍是写死字面量、未参数化——DEVLOG 已勘误过这一口径，本轮沿用现状，只换字符串）。
2. `atlas migrate diff` 生成迁移；预期内容（人工审阅清单）：
   - `ALTER TABLE "users" ADD COLUMN "totp_secret" character varying`（可空、无 DEFAULT）
   - `ALTER TABLE "users" ADD COLUMN "totp_enabled_at" timestamptz`（可空、无 DEFAULT）
   - `CREATE TABLE "recovery_codes"`（含 FK `ON DELETE CASCADE`）
   - `CREATE INDEX "recoverycode_user_id" ON "recovery_codes" ("user_id")`（ent 索引命名 = 类型名_字段名拼接，现有库实证：`user_group_id` / `user_created_at`）
   - **无 NOT NULL、无回填、无数据语句**——全部可空列 + 新表，不存在 access 轮「非空列与外键同句」的拆句风险；生成物若与预期不符（如列类型偏差）人工修正。
3. `atlas migrate hash` + `atlas migrate validate`（无论是否手改都要重算校验和）。
4. `scripts/db-migrate-apply.sh` 应用到 `neoblog` 与 `neoblog_test`。

## 8. 测试

| 层 | 连库 | 内容 |
| --- | --- | --- |
| 迁移后置断言（`internal/modules/auth/`，新文件） | 是（`DATABASE_URL_TEST` 缺省 skip） | ① 不带新字段建用户成功，读回 `TOTPSecret` / `TOTPEnabledAt` 均为 nil（可空性实证）；② 设值后读回一致（`*string` / `*time.Time` 正确落列）；③ 给用户插一条恢复码行，删用户后行数归零（级联实证）；④ 恢复码行 `UsedAt` 默认 nil |
| 全量回归 | 否 | `go build` / `go vet` / `gofmt -l` 干净；无库 `go test ./...` 全绿（本轮不改任何业务代码，预期零红） |

测试数据用随机后缀 + `t.Cleanup` 清理；断言用 ent 行为而非 `information_schema`（沿用 access 轮 `TestMigrationSeed` 的风格）。

## 9. 已知缺口与风险

- **死表窗口**：接口下一轮才有消费方，期间 `recovery_codes` 恒空、两列恒 NULL，无任何写入路径；表结构先行仅存在「下一轮实现发现列不足需微调」的可能，而可空加列属零风险 alter。
- **secret 明文**：DB 泄露即可伪造 TOTP。休眠期无 secret 存在；升级为应用层加密（如 AES-GCM，密钥走环境变量）可在任何用户启用之前随时完成，无存量迁移。启用后若再升级则需重加密回填——这是选在「暂不启用」阶段先落结构的核心动机。
- **恢复码哈希无盐**：sha256 无盐对高熵随机码（≥128 bit）足够，无彩虹表价值；若下一轮改用低熵可读码（如 8 位数字），需换 bcrypt/argon2——列仍是 `text`，换算法不动表。
- **`code_hash` 不设长度上限**：本轮无写入方；下一轮实现写入路径时以校验钉住 64 字符 hex（或届时换编码后同步收紧）。
- **版本口径**：minor 进位按执行时的当前版本顺延（access 轮同款规则）。

## 10. 验收标准

1. `bash scripts/db-generate.sh` 重生成后 `go build ./... && go vet ./... && gofmt -l .` 干净；生成代码与 schema 一起提交。
2. 迁移 SQL 按 §7 审阅清单核对、`atlas migrate hash` / `validate` 通过、应用到两个库。
3. 有库环境：迁移后置断言（§8）通过；`go test -p 1 ./...` 全绿。无库环境：`go test ./...` 全绿（集成用例缺省 skip）。
4. README 更新（数据库章节表清单补 `recovery_codes` 与 `users` 新列、认证章节加预留说明）；DEVLOG 加条目；版本 `./scripts/bump-version.sh minor` 进位。
5. 全仓无任何 2FA 接口/路由/登录分叉出现（本轮的「不做」也要可验证：`grep` 路由注册只应有既有 13 条 auth/access 路由（auth 5 + access 8）+ healthy）。

## 11. 插槽（本轮不定，定它的时点）

| 插槽 | 什么时候必须定 | 影响面 |
| --- | --- | --- |
| 接口与登录分叉（setup / enable / disable / 二段端点 / pending 会话键） | 下一轮 | auth 模块、守卫、`/auth/me` |
| 恢复码策略细节（数量、长度、字符集、有效期、再生成是否要求 TOTP 在场） | 下一轮（与接口同轮） | 恢复码写入 / 验证路径 |
| secret 应用层加密 | 任何用户启用之前的任意时点（越早越好） | service 层读写两处 |
| 登录限流 | 另开一轮（README 已知缺口） | login controller / service / 配置 |
| `/auth/me` 暴露 `2fa_enabled` | 接口轮 | auth dto |
| Passkey / API Token / 社交登录 | 更后（auth 设计 §2 已列库选型） | 新表、新守卫 |

## 12. 下一步

本文经用户审阅通过后，进 writing-plans 出实施计划（范围 = §3，验收 = §10）。
