# NeoBlog 两步验证表结构预留（TOTP，仅落库）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把两步验证（TOTP + 自管恢复码）的数据插槽钉死：`users` 加两个可空列、新建 `recovery_codes` 表、一个迁移、迁移后置断言；本轮零业务代码、零接口，上线后行为与现状完全一致。

**Architecture:** TOTP 状态 1:1 跟人走（`users.totp_secret` / `users.totp_enabled_at`，Optional + Nillable 组成「未设置 / 设置中 / 已启用」三态）；恢复码 1:N（`recovery_codes`：`user_id` 级联外键 + `code_hash` + `used_at` 一次性标记）。ent Optional 字段不进 INSERT 列清单，因此生成代码先于迁移落地也不产生断窗。接口、登录分叉、恢复码生成逻辑全部是下一轮的事，本轮只让表结构和迁移就位。

**Tech Stack:** Go 1.27 + ent v0.14.6（既有 feature 集，本轮不加）+ Atlas + Postgres。**不引入 `pquerna/otp`**（go.mod 本轮不动）。

**Spec:** `docs/superpowers/specs/2026-10-11-neoblog-2fa-schema-design.md`（本计划实现它；执行者两份都要读，三态语义在 spec §4，消费规则在 §6）

## Global Constraints

- **开工前先同步 HEAD**：仓库可能有并行会话在推进其他层；改动只加不删，提交前 `git diff` 只应包含本任务的改动文件，禁止 `git add -A`。
- 所有命令的工作目录是 `backend/`（`go test ./internal/...`、`bash scripts/*.sh`、`./scripts/bump-version.sh` 都是相对路径）。
- 本机 Git Bash 先执行：`export PATH="/f/software/go/bin:/f/software/gopath/bin:$PATH" GOPATH='F:/software/gopath' GOCACHE='F:/software/gocache' GOPROXY='https://goproxy.cn,direct' DATABASE_URL='postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog?sslmode=disable' DATABASE_URL_TEST='postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_test?sslmode=disable'`（`db-migrate-apply.sh` 有 `set -u`，两个 URL 必须导出，它自己不读 `.env`）。
- 手写 Go 文件**零注释**（ent 生成物与工具链指令豁免）；`gofmt -l .` 必须为空。
- **本轮不引入任何新依赖**：`go.mod` / `go.sum` 不得出现变化。
- `scripts/db-migrate-diff.sh` 是写死字面量（未参数化，DEVLOG 已勘误过该口径）：Task 2 只把字符串 `groups_and_permissions` 换成 `totp_and_recovery_codes`，不做参数化改造。
- `atlas migrate diff` 会临时创建 `backend/.entc/`（正常自删）；中断残留时不要 `git add` 它。无论是否手改迁移 SQL，之后都必须 `atlas migrate hash` + `atlas migrate validate`。
- 集成测试约定：读 `DATABASE_URL_TEST`，缺省 `t.Skip`；**迁移后置断言必须先应用迁移再跑**；测试数据带随机后缀，`t.Cleanup` 清理。
- 提交信息中文、conventional 前缀（`feat(backend): ...` / `docs(backend): ...` / `chore(backend): ...`），每个任务一次提交。
- 版本：Task 3 执行 `./scripts/bump-version.sh minor`（当前 0.5.1 → 0.6.0；若届时已被其他层进位，按当前版本顺延，DEVLOG 记实际版本）。
- 固定值（计划内唯一来源，schema 与迁移 SQL 必须一致）：列名 `totp_secret` / `totp_enabled_at`；表名 `recovery_codes`；其列 `user_id` / `code_hash` / `created_at` / `used_at`；ent 边名 `recoveryCodes`；迁移名 `totp_and_recovery_codes`。

## Review Focus

1. **级联方向挂错边**：`OnDelete(entsql.Cascade)` 必须挂在 `User.recoveryCodes` 的 `edge.To` 侧——挂到 `RecoveryCode` 侧或漏挂，删用户会因 FK 拒绝或留下孤儿行。Task 2 断言③（删用户后恢复码行数归零）钉住。
2. **漏 `.Nillable()`**：只写 `.Optional()` 时 Go 侧拿零值而非 `nil`，NULL 与空串不可区分，「设置中」三态丢失。Task 2 断言①（不带字段建用户读回 `nil`）钉住。
3. **生成迁移意外带 NOT NULL / DEFAULT**：schema 误写 `Required` 或 ent 行为偏差都会让旧库与生成代码不一致。Task 1 Step 3 的无库全量测试（证明无断窗）+ Task 2 Step 2 审阅清单钉住。
4. **`recovery_codes.user_id` 索引漏建**：Postgres 不为外键自动建索引，恢复码验证会全表扫。Task 2 Step 2 审阅清单里的 `CREATE INDEX` 行钉住。
5. **迁移只应用了一个库**：`neoblog` 与 `neoblog_test` 任一漏应用都会造成两库 schema 漂移（access 轮起两库并行）。Task 2 Step 3 核对脚本输出覆盖两个库名。

## 文件结构

| 文件 | 职责 |
| --- | --- |
| `backend/internal/database/ent/schema/user.go`（改） | `users` 加 `totp_secret` / `totp_enabled_at` 字段与 `recoveryCodes` 边 |
| `backend/internal/database/ent/schema/recovery_code.go`（新） | 恢复码表 schema |
| `backend/internal/database/ent/**`（生成，提交） | ent 生成代码 |
| `backend/scripts/db-migrate-diff.sh`（改） | 迁移名字面量换为 `totp_and_recovery_codes` |
| `backend/migrations/<ts>_totp_and_recovery_codes.sql`（新） | 加列 + 建表 + 索引 |
| `backend/internal/modules/auth/totp_schema_test.go`（新） | 迁移后置断言（2FA 属认证域） |
| `backend/README.md`、`docs/DEVLOG.md`、`backend/internal/version/version.go`（改） | 文档、开发日志、版本 |

---

### Task 1: ent schema（users 两列 + recovery_codes + 重生成）

**Files:**
- Modify: `backend/internal/database/ent/schema/user.go`
- Create: `backend/internal/database/ent/schema/recovery_code.go`
- Generated（提交）: `backend/internal/database/ent/**`

**Interfaces:**
- Consumes: 无（起点任务）。
- Produces: `ent.User` 新增 `TotpSecret`（`*string`）、`TotpEnabledAt`（`*time.Time`）与边 `RecoveryCodes`（查询构造器 `WithRecoveryCodes()`）；`ent.RecoveryCode` 实体与 `client.RecoveryCode.Create()`（`SetUserID(uuid.UUID)` / `SetCodeHash(string)` / `SetUsedAt(time.Time)` / `SetNillableUsedAt(*time.Time)`）、谓词 `recoverycode.UserIDEQ`；`client.User.DeleteOne(*ent.User)`。（注：ent 命名转换不保留缩写词大写，`totp_secret` 生成 Go 名为 `TotpSecret` 而非 `TOTPSecret`，已实证）

- [ ] **Step 1: 改 `user.go`、写 `recovery_code.go`（零注释，形状对齐 `user_permission.go`）**

`user.go`：Fields 追加 `field.String("totp_secret").Optional().Nillable()` 与 `field.Time("totp_enabled_at").Optional().Nillable()`；Edges 追加 `edge.To("recoveryCodes", RecoveryCode.Type).Annotations(entsql.OnDelete(entsql.Cascade))`（级联注解挂 `edge.To` 侧，与既有 `permissions` 边同写法）。
`recovery_code.go`：Fields `id`（UUIDv7 默认，照抄 `user.go` 的 `id` 行）、`user_id`（UUID）、`code_hash`（String、NotEmpty）、`created_at`（Default(time.Now).Immutable()）、`used_at`（Time、Optional、Nillable）；Edges `edge.From("user", User.Type).Ref("recoveryCodes").Unique().Required().Field("user_id")`；Indexes `index.Fields("user_id")`（普通索引，非唯一）。

- [ ] **Step 2: 重生成**

Run: `bash scripts/db-generate.sh`
Expected: 退出码 0；`backend/internal/database/ent/` 下出现 `recoverycode.go` / `recoverycode_create.go` / `recoverycode_query.go` 等，`user.go`（生成物）含 `TOTPSecret` / `TOTPEnabledAt` 字段，`user_query.go` 含 `WithRecoveryCodes`。

- [ ] **Step 3: 无库验证（证明生成代码与现有库不冲突）**

Run: `go build ./... && go vet ./... && gofmt -l .` → 三者干净（`gofmt -l` 无输出）。
Run: `go test ./...`（不导出 `DATABASE_URL_TEST`）→ 全绿（集成用例缺省 skip；本轮无任何必填列变化，预期零红）。

- [ ] **Step 4: Commit**

```bash
git add backend/internal/database
git commit -m "feat(backend): TOTP 两步验证预留字段与恢复码 schema"
```

---

### Task 2: 迁移（加列 + 建表 + 索引）与后置断言

**Files:**
- Modify: `backend/scripts/db-migrate-diff.sh`（第 6 行的迁移名字面量）
- Create: `backend/migrations/<时间戳>_totp_and_recovery_codes.sql`
- Test: `backend/internal/modules/auth/totp_schema_test.go`

**Interfaces:**
- Consumes: Task 1 的 `ent.RecoveryCode` 与 `ent.User` 新字段。
- Produces: 版本化迁移 `<ts>_totp_and_recovery_codes.sql`（两库已应用）；集成测试 `TestTOTPSchemaReserved`。

- [ ] **Step 1: 换迁移名字面量并生成迁移**

`scripts/db-migrate-diff.sh` 内 `groups_and_permissions` → `totp_and_recovery_codes`（只换字符串）。
Run: `bash scripts/db-migrate-diff.sh`
Expected: `backend/migrations/` 出现 `<ts>_totp_and_recovery_codes.sql`，`atlas.sum` 更新。

- [ ] **Step 2: 人工审阅迁移 SQL（对照 spec §7 清单，逐行核对）**

必须恰好包含：`ALTER TABLE "users" ADD COLUMN "totp_secret" ...`（可空、无 DEFAULT）、`ALTER TABLE "users" ADD COLUMN "totp_enabled_at" ...`（可空、无 DEFAULT）、`CREATE TABLE "recovery_codes"`（FK `ON DELETE CASCADE`）、`CREATE INDEX "recoverycode_user_id"`（ent 命名 = 类型名_字段名拼接，现有库实证 `user_group_id` 同款）。必须**没有**：NOT NULL、DEFAULT、回填 UPDATE、任何数据语句。生成物与清单不符时人工修正 SQL。

- [ ] **Step 3: hash + validate + 应用两库**

Run: `atlas migrate hash && atlas migrate validate` → 干净。
Run: `bash scripts/db-migrate-apply.sh` → 输出核对 `neoblog` 与 `neoblog_test` 两个库都应用成功（Review Focus 5）。

- [ ] **Step 4: 写迁移后置断言 `TestTOTPSchemaReserved`**

文件 `backend/internal/modules/auth/totp_schema_test.go`，连库约定沿用 access 轮迁移测试（`DATABASE_URL_TEST` 缺省 skip；随机后缀 + `t.Cleanup`）。`testDB` / `uniqueTag` 是 access 包的私有 helper，auth 包自带一份等价实现——引导照抄 `access/migration_test.go:17-41`：`database.Open(context.Background(), config.Config{DatabaseURL: url, DBMaxOpenConns: 10, DBMaxIdleConns: 5, DBConnMaxLifetime: 30 * time.Minute})`，用 `db.Client`，`t.Cleanup` 里 `db.Close()`。四个断言，全部用 ent 行为、不查 `information_schema`：

1. **可空性**：`client.User.Create().SetEmail(随机).SetNickname(随机).SetPasswordHash("x").Save(ctx)` 成功；读回 `u.TotpSecret == nil` 且 `u.TotpEnabledAt == nil`。
2. **Nillable 读写**：`u.Update().SetTotpSecret("JBSWY3DPEHPK3PXP").SetTotpEnabledAt(time.Now()).Save(ctx)` 后重查，两值非 nil 且 secret 相等。
3. **级联**：`client.RecoveryCode.Create().SetUserID(u.ID).SetCodeHash(strings.Repeat("a", 64)).Save(ctx)` 成功且 `UsedAt == nil`；`client.User.DeleteOne(u).Exec(ctx)` 后按 `recoverycode.UserIDEQ(u.ID)` 计数为 0。
4. **一次性标记默认**：断言 3 里保存后的恢复码行 `UsedAt` 为 nil（未消费）。

- [ ] **Step 5: 带库验证**

Run: `go test ./internal/modules/auth/ -run TestTOTPSchemaReserved -count=1 -v`（已导出 `DATABASE_URL_TEST`）→ PASS。
Run: `go test -p 1 ./...` → 全绿。

- [ ] **Step 6: Commit**

```bash
git add backend/migrations backend/scripts/db-migrate-diff.sh backend/internal/modules/auth
git commit -m "feat(backend): TOTP 与恢复码迁移（预留两步验证数据插槽）"
```

---

### Task 3: README、DEVLOG、版本进位与终验

**Files:**
- Modify: `backend/README.md`、`docs/DEVLOG.md`、`backend/internal/version/version.go`（由 `bump-version.sh` 连带 README 版本行）

- [ ] **Step 1: README**

数据库章节（表清单/迁移说明处）补 `recovery_codes` 一行（`user_id` 级联外键、`code_hash`、`used_at` 一次性）与 `users` 新列说明（`totp_secret` / `totp_enabled_at` 可空、无 DB 默认值）；认证章节「已知缺口」附近加一句预留说明：「两步验证（TOTP + 自管恢复码）的表结构已预留（spec：`docs/superpowers/specs/2026-10-11-neoblog-2fa-schema-design.md`），接口与登录分叉下一轮；当前无用户启用、无任何写入路径」。

- [ ] **Step 2: DEVLOG 条目**

`docs/DEVLOG.md` 新增条目：动机（兑现 auth 设计 §2/§11 预留）、范围裁定（只落库表结构、限流另开一轮）、迁移名与两库应用、后置断言四断言、`db-migrate-diff.sh` 字面量再换一次的说明。

- [ ] **Step 3: 版本进位**

Run: `./scripts/bump-version.sh minor` → `version.go` 与 README 版本行同步为 0.6.0（或按当前版本顺延）。

- [ ] **Step 4: 终验**

Run: `go build ./... && go vet ./... && gofmt -l .` → 干净。
Run: `go test -p 1 ./...`（带库）→ 全绿。
Run: `grep -rn "totp\|recovery_codes" backend/internal/modules/ --include="*.go" | grep -v _test.go | grep -v "internal/database/ent"` → 无输出（本轮「不做接口」的可验证性，spec §10.5：业务模块不得出现任何 2FA 路由或读写路径）。

- [ ] **Step 5: Commit（三个）**

```bash
git add backend/README.md
git commit -m "docs(backend): 两步验证表结构预留说明（接口下一轮）"
git add docs/DEVLOG.md
git commit -m "docs(backend): 新增开发日志（2026-10-11 两步验证表结构预留）"
git add backend/internal/version/version.go backend/README.md
git commit -m "chore(backend): 版本进位 0.6.0（两步验证表结构预留）"
```

---

## 收尾：终审与修复

全部任务完成后做一次整分支终审（fresh context 只读子代理），对照 Review Focus 五项 + spec §10 验收标准逐条核对；发现缺陷先复现再修，修复后按层分别提交（只 `git add` 实际改动文件，禁止 `git add -A`）。无缺陷则跳过本步。

DEVLOG 条目同时记录终审结果与实际进位版本。
