# NeoBlog 两步验证设置复用窗口设计（setup 窗口内复用待验证密钥）

日期：2026-10-14
范围：`backend/`（Go + gin + ent + pquerna/otp + Atlas 迁移，**一个可空列**）；前端一行不动
状态：v2 —— 窗口时长经用户裁定（2026-10-14：**5 分钟**）；独立审查 7 条已逐条落位（见修订记录）；命名口径、边界与兜底分支为本轮裁定，可逐条否决

修订记录：
- v1（2026-10-14）：首版（当时的窗口 = 30 秒，命名「防抖」）。
- v2（2026-10-14）：独立审查后修订——① 窗口 **30 秒 → 5 分钟**（用户裁定）：30 秒盖不住「展开→掏手机扫码→回浏览器输码」的真实节奏，收起再展开照样换钥，症状复发；且面板里没有主动换钥入口（重试按钮只在加载失败时出现），「等窗口过去」不可见也没人真去等。② 认领失败的兜底由「盲写候选」改为「**再认领一次 + 以库内现状为准**」，并补写并发互斥的依据（Read Committed 重算谓词）——原兜底既无用例覆盖、又破坏「至多一把」不变式。③ 「响应逐字节相同」限定为**同一账号名下**（`otpauth_url` 的账号名取自当次 `found.Email`，窗口内改邮箱会让 URL 变）。④ 复用重建加**空明文守卫**：`pquerna/otp` 在 `len(opts.Secret) == 0` 时会静默随机生成新密钥（实证源码），解码结果为空必须报错而不是换钥。⑤ 命名正名：**复用窗口**（「防抖」是用户口语，实现语义是窗口内复用/幂等）。⑥ README 补多设备行为变更一句。⑦ 变异自检 4 处 → 7 处。⑧ 登记残余：面板每次展开仍有骨架屏闪烁（前端轮）。⑨ 测试纪律：用假钟的用例禁止 `t.Parallel`。
- v3（2026-10-14，本文）：v2 审查后修订——① **删除 `updateTOTPSetup`**：认领路径接手后它成了无人调用的死方法（Go / `go vet` 都不报），两文档里的五处提法同步清掉，`errUserNotFound` 删后仍有 7 处使用、不引孤儿。② 变异自检判定改为「目标用例必红 + **连带红属预期**（逐条记录）」，期望集合改成实测口径（③→{5,11}、④→{1,2,8,9}、⑦→几乎全包），并把原 ⑥ 拆成「`isFreshTOTPSetup` 放宽（只红 7，唯一有区分度）」与「谓词删 `IsNil`（近乎全包，只作防线确认）」两条。③ 活体验收**不再真等 5 分钟**：边界已由用例 3 用假钟钉死，活体只需证「同刻同钥 + 陈旧换新」——后者用 `backend/.scratch` 的 Go 探针把该行 `totp_setup_at` 拨回 6 分钟前，几秒完事。④ 空明文守卫用 `fmt.Errorf`（`totp.go` 无 `errors` import，不为一行错误新增依赖）。⑤ 用例计数勘正：既有 `TestTwoFactor*` 实测 **19 条**（`totp_flow_test.go` 18 + `sessions_test.go` 1），非 v2 计划所写的 15 条。
- v4（2026-10-14）：交付后复审修订（用户实跑复核）——① §4.2/§2 的复读兜底措辞对齐代码实际行为：**复读也走 `新鲜` 判定**（`isFreshTOTPSetup`），非「有密钥就复用」——「有密钥但不新鲜」这一格正常流到不了（认领 0 行的行必然新鲜），到不了就走再认领（此刻谓词必真、认领必成）；文档改齐、代码不动。② 变异 ⑥ 拆成 ⑥（仅 NULL 放宽，实测 {7}）与 ⑥b（整条放宽连时间窗一起丢，实测 {3,7,10}）——首轮自检记录把两个改坏的数字串了行，实跑复核拆开。③ 变异 ④ 实测不可观测（慢路径与快路径行为等价），从列检清单移除、备查保留。④ 变异 ② 的确定性钉子（仓库级 `TestClaimTOTPSetupRejectedWhenFresh`）登进 §7 用例表（第 12 行），并注明并发用例对 ② 依赖时序、不可靠。⑤ 用例计数 9 → **10 条**、迁移断言口径改为独立用例 `TestTOTPSetupAtColumn`（两文档同步）。⑥ `-race` 本机不可用（无 cgo / gcc）如实登记：并发正确性由「认领互斥的仓库级断言 + 重复 8 次稳定绿」承载，未做竞态证明。

## 1. 意图与背景

**症状（用户原话）**：每展开一次 2FA 设置面板，密钥就重新生成一次；扫完码后手一滑把面板收起、再展开，二维码和密钥全变了，手机上刚添加的那条作废。

**根因（已实测，两处叠加）**：

1. 前端：设置面板收起时会真实卸载（Base UI `Collapsible.Panel` 默认 `keepMounted=false`），再展开 = 重新挂载 `useTwoFactorSetup`，其挂载副作用立刻 `POST /users/me/2fa/setup`（`frontend/hooks/use-two-factor-setup.ts:75`）。
2. 后端：`SetupTOTP` 无条件生成新密钥、覆盖旧的（`backend/internal/modules/auth/service.go:256`），且**库里没有记「待验证密钥何时生成」**——无从判断"这是刚生成的，还是十天前的"。

**目标**：密钥生成后 **5 分钟**内的重复 setup 复用同一把密钥；超过 5 分钟才重新生成。

**窗口为什么是 5 分钟**（v1 曾定 30 秒，被审查否掉）：真实节奏是「展开面板 → 掏手机 → 打开验证器 → 扫码 → 回浏览器输 6 位码」，扫码这一段本身常超 30 秒；30 秒窗口下「扫完码手滑收起、再展开」照样换钥——最初报的症状原样复发。窗口必须覆盖整段同坐操作，取 5 分钟（上界由用户裁定）。副作用是「等窗口过去」不再是一条现实可用的换钥路径——但面板里本来就没有换钥按钮（`retrySetup` 只在加载失败时出现，`frontend/app/console/uc/profile/two-factor-panel.tsx:173`），所以不是退步，只是把「显式重新生成」留给前端轮（§11 插槽）。

**为什么必须落在后端**（对话中已裁决）：前端缓存只能覆盖"同一标签页、不刷新"；按 F5 或换标签页照样重新生成。后端记住生成时刻才能让任何客户端在任何时刻拿到同一条口径。

**已排除的方案**：前端缓存（覆盖不全）；「只要未启用就永远复用同一把」（密钥跨会话长期挂在库里，且连"窗口过期自动换新"这最后一次自愈也没有）。

**命名**：本文与实现、README、DEVLOG 一律叫「**setup 复用窗口**」（窗口内幂等/复用）；用户口语里的「防抖」（debounce）不再作为文档术语——防抖是"等安静期再动作"，与本机制不同义，留着会让未来的读者去代码里找一个并不存在的 debounce 逻辑。

## 2. 已定决策

| 项 | 决定 |
| --- | --- |
| 新列 | `users` 加 `totp_setup_at`（timestamptz、可空、无默认值、无回填）；语义 = **当前 `totp_secret` 的生成时刻** |
| 不用 `updated_at` | `users.updated_at` 会被改资料、改密等一切写入刷新，与「密钥生成时刻」不同义，不可借用 |
| 复用窗口 | **5 分钟，固定窗**（用户裁定 2026-10-14）：`totpNow() - totp_setup_at < 5min` 复用；`>= 5min` 重新生成并重新计时 |
| 计时起点 | 从**生成时刻**起算，不是从"关闭面板时刻"起算——服务端看不到"关闭"事件（面板收起不产生任何请求）；5 分钟这个量级下两种起点的差异已不可感知，不值得为它加协议 |
| 非滑动窗 | 复用路径**零写入**（不刷新 `totp_setup_at`）：状态只有一个来源（库里那一行），边界可预测；反复调 setup 不会让同一把密钥无限续命 |
| 判定与写入 | 先读复用（快路径，零写入）；需要生成时走**原子条件更新认领**（照 `claimTOTPCounter` / `consumeRecoveryCode` 先例）：条件 = `totp_enabled_at IS NULL AND (totp_setup_at IS NULL OR totp_setup_at <= now - 窗口)` |
| 并发互斥的依据（正确性支点） | 单条 `UPDATE … WHERE` 在同一行上由 Postgres **Read Committed 语义串行化**：后到者等前者提交/回滚，若前者提交则**重算 WHERE**，条件已假则影响 0 行。因此同一时刻至多一人认领成功，其余一律回到"以库内现状为准"的复用 |
| 认领失败的兜底 | **不再盲写**：认领 0 行 → 复读该行 → 行没了 401（同 `activeUser`）/ 已启用 409 / **新鲜（§4.2 的 `新鲜` 判定）→ 复用它**（此时它必然是并发对手刚写入的那把；快慢路径共用同一判定入口）/ 有密钥但不新鲜（仅手工改库可造出，正常流到不了）或库内无密钥（并发 disable 清掉）→ **再认领一次**（此刻认领谓词必为真，认领必成）。两次认领仍无法落位 → 500 `internal_error`（理论上不可达，**不写库**；登记在 §9） |
| 并发不变式 | 同一用户、同一时刻的 N 次并发 setup **至多产生一把密钥**，其余全部复用它（每一次 200 响应的 secret 都等于库内当前那一行） |
| 复用响应的构造 | 解密存量密文 → base32 解码 → `totp.Generate{Secret: 原始字节, 同 Issuer/AccountName/Period/Digits/Algorithm}` 重建 Key → 返回 `key.Secret()` + `key.URL()`；**空明文守卫**：解码结果长度为 0 直接报错（`pquerna/otp` 在 `len(Secret)==0` 时会静默随机生成新密钥，必须挡住） |
| 逐字节相同（限定） | 与首次响应 `secret` 与 `otpauth_url` **逐字节相同的前提是同一账号名（邮箱未变）**——`otpauth_url` 里的账号名取自当次 `found.Email`；窗口内改了邮箱则 secret 相同、URL 带新邮箱（口径见 §6） |
| 复用路径的解密/解码失败 | 500 `internal_error`（沿用 enable 的既有口径：密钥轮换/数据损坏不得伪装成正常响应，也不得悄悄换成新密钥）；覆盖三种坏法：密文打不开、base32 解不开、明文为空 |
| `enable` 与 `setup_at` | enable **不动** `totp_setup_at`（列语义 = 当前密钥的生成时刻，启用后依然成立）；已启用时 setup 照旧 409，与 `setup_at` 无关 |
| `disable` 与 `setup_at` | disable 在既有单事务里**一并清掉** `totp_setup_at`（回到「未设置」时四列全空） |
| 遗留行（本部署前「设置中」） | `totp_secret` 非 NULL 但 `totp_setup_at` 为 NULL：认领谓词里的 `TotpSetupAtIsNil()` 分支让它照常落位（生成新密钥 + 生成本刻） |
| 多设备 / 多标签页 | 窗口内各端拿到**同一把**（此前是各生成一把、后写覆盖前者）——不是 bug，是行为变更，README 端点表写明 |
| 响应契约 | 形状零变化：`200 {"secret","otpauth_url"}`；新增的只是**保证**——响应里的 secret 一定等于库里当前待验证的密钥，其验证码必定能 enable 成功 |
| 前端 | **一行不动**（hook 每次展开照常请求，复用对它透明）；残余：每次展开仍有一次请求 + 骨架屏闪烁（见 §9） |
| API 规范 | 零改动（路径、请求体、响应形状、错误码全未变）；README 端点表与迁移清单随轮更新 |
| 版本 | `0.16.0` → `0.17.0`（`scripts/bump-version.sh minor`） |

**本轮自行裁定的口径（可逐条否决）**：计时起点 = 生成时刻（5 分钟量级下差异不可感知）；固定窗而非滑动窗；`enable` 保留 `totp_setup_at`；复用路径解密/解码/空明文失败报 500；命名用「复用窗口」。

## 3. 范围

本轮做：

- 迁移：`users` 加 `totp_setup_at`（可空 timestamptz）+ ent schema 同步 + 重生成生成代码。
- `auth`：`totp.go` 加包内常量与重建助手（含空明文守卫）；`repository.go` 加原子认领（含落时刻）、**删除**被取代的 `updateTOTPSetup`（Go 不报未使用方法，必须手动删）、`disableTOTP` 清时刻；`service.go` 改写 `SetupTOTP`（读复用 / 认领 / 复读 / 再认领）。
- 迁移后置断言（`totp_schema_test.go` 独立用例）+ 集成用例 **10 条**新增（含变异自检补救时新增的仓库级 `TestClaimTOTPSetupRejectedWhenFresh`）、2 条既有用例按新口径调整。
- README（迁移清单 / setup 行 / disable 行 / 两步验证行）、DEVLOG、版本进位。

本轮不做（明确出局）：

- 前端任何文件（含文案、「重新生成」按钮）。
- 滑动窗、"关闭时刻"起算（要"关闭时刻"必须让前端上报关闭事件，协议复杂度不值）。
- API 规范文档改动（无形状变化）。
- 冻结的历史 spec 回改（`2026-10-11-neoblog-2fa-design.md` §5.1 的"重复 setup 覆盖旧 secret"口径、`2026-10-11-neoblog-2fa-schema-design.md` §5 的列清单，均保持原样存档；现行口径以 `backend/README.md` + 本文为准——沿用 0.16.0 改名轮"旧 spec 不回改"的先例）。
- 待验证密钥的有效期/清理（窗口外的旧密钥照旧躺在库里等下次覆盖或被 disable 清掉，本轮不改）。
- 限流、`TOTP_ENCRYPTION_KEY` 轮换、恢复码策略。

## 4. 行为规格

### 4.1 时间线（`T` = 第一次 setup 的时刻，`W` = 5 分钟）

| 时刻 | 动作 | 结果 |
| --- | --- | --- |
| T | setup | 生成密钥 A，`totp_setup_at = T`，响应 A（明文 + otpauth_url） |
| T+10s | setup（收起面板后重开） | **复用**：响应与上一次逐字节相同（A），库内零写入 |
| T+4m59s | setup | 复用 A |
| T+5m | setup | **重新生成** B，`totp_setup_at = T+5m`；A 的码此后 enable 401 `two_factor_invalid_code` |
| T+5m10s | enable（用 A 的码） | 401 `two_factor_invalid_code`（库里是 B） |
| T+5m20s | enable（用 B 的码） | 200 + 10 个恢复码；`totp_setup_at` 保持 `T+5m` |
| 任意时刻 | disable | 清 `totp_secret` / `totp_enabled_at` / `totp_last_counter` / `totp_setup_at` |

### 4.2 判定顺序（服务端）

定义 `新鲜(row, now)` = `row.TotpSecret != nil && row.TotpSetupAt != nil && now.Sub(*row.TotpSetupAt) < W`。

1. 取活跃用户（不存在 → 销毁会话 + 401 `unauthorized`，既有 `activeUser` 语义）。
2. `totp_enabled_at` 非 NULL → 409 `two_factor_already_enabled`（既有前置，不动）。
3. `now := totpNow()`（**整个请求只取一次**，与既有"时间基准单点"纪律一致）。
4. `新鲜` → **复用**（解密 → base32 解码 → 重建 ‖ 失败 → 500 `internal_error`）。
5. 否则：生成候选 + 加密 → **认领**（条件更新）：
   - 认领成功（影响 1 行）→ 返回候选（库内 = 响应，必然一致）；
   - 认领 0 行 → **复读**该行：行没了 → 401；已启用 → 409；`新鲜` → 复用它（以库内现状为准）；其余（有密钥但不新鲜——正常流到不了；或库内无密钥——并发 disable 清掉）→ 回到认领，**再试一次**（此刻认领谓词必为真）；
   - 两次认领都无法落位且库内始终无密钥 → 500 `internal_error`（理论不可达：只发生在并发反复 disable 的毫秒级窗口内；**不写库**、不伪造响应）。
6. 复用永不写库；认领成功的写入与返回的候选同源同值；复读/重试路径只读。

### 4.3 边界与例外

- 边界闭合：正好 5 分 0 秒 → 重新生成（`< W` 复用 / `>= W` 生成）。
- `totp_setup_at` 在未来（时钟回拨）→ `now - setup_at` 为负 → 落在复用侧；无害，不做特殊处理。
- 复用不消耗、不改动 `totp_last_counter`；重新生成照旧清该列（既有行为）。
- 窗口常量 `totpSetupReuseWindow = 5 * time.Minute` 与 TOTP 的 30 秒步长**无任何关系**：独立命名、禁止复用 `totpPeriodSeconds`（混用会造成"窗口对齐"的假象）。
- 认领谓词里的 `TotpSetupAtIsNil()` 分支承担两件事：遗留行（本部署前「设置中」）与并发 disable 后的重新落位——删除它会让遗留行走"以库内现状为准"的复用分支、返回旧密钥（用例 7 + 变异⑥ 钉住）。

## 5. 数据模型（ent + 迁移）

`users` 增量（1 列，可空、无默认值、无回填、无索引）：

- `totp_setup_at`：`field.Time("totp_setup_at").Optional().Nillable()`——Go 侧 `*time.Time`。
- 不加索引：读取路径永远是"按主键取行"（`activeUser`），无独立查询。

迁移（`atlas migrate diff` 生成，人工审阅）：

```sql
ALTER TABLE "public"."users" ADD COLUMN "totp_setup_at" timestamptz NULL;
```

必须**没有**：NOT NULL、DEFAULT、回填 UPDATE、任何数据语句、索引。迁移名字面量（`scripts/db-migrate-diff.sh:7` 内写死的那串）换为 `totp_setup_at`。`atlas migrate hash` + `validate` 后应用到 `neoblog` 与 `neoblog_test` 两个库。

## 6. 接口契约（形状不变，只加保证）

### `POST /api/v1/users/me/2fa/setup`（守卫内，路径与响应不变）

- 首次与窗口外：200 `{"secret":"<base32 明文>","otpauth_url":"<otpauth://totp/NeoBlog:…>"}`。
- 窗口内重复调用：200，**同一账号名（邮箱未变）下与上一次响应逐字节相同**；窗口内改了邮箱则 secret 相同、`otpauth_url` 带新邮箱（账号名取自当次读到的 `Email`，与首次生成同源）。
- 窗口内多设备/多标签页：拿到同一把密钥（行为变更，见 §2 表）。
- 已启用：409 `two_factor_already_enabled`（不变）。
- 复用路径解密/解码/空明文失败：500 `internal_error`（新登记行为，与 enable 同口径）。
- **保证**：任何 200 响应的 `secret` 都等于库里当前待验证的密钥，其验证码必定能 enable 成功。

### 安全口径（明文再回一次的解释）

窗口内复用会把同一把明文 secret 再回一次；不放宽信任模型：能带 session 打到本端点的人，在窗口外本来也能拿到（一把新的）明文 secret，且该密钥在 enable 之前不具备任何登录效力。README 的"明文 secret 只回这一次"措辞随本轮回正为"明文 secret 仅经本端点暴露（首发与窗口内复用均可能返回明文），窗口外重新生成"。

## 7. 测试口径

集成测试（`internal/modules/auth/totp_flow_test.go`，`DATABASE_URL_TEST` 缺省 skip；时间一律经 `totpNow` 接缝固定，禁止真实等待 5 分钟）：

| # | 用例 | 断言要点 |
| --- | --- | --- |
| 1 | `TestTwoFactorSetupReusesSecretImmediately` | 固定钟两次 setup → 两个响应体 `secret` 与 `otpauth_url` **逐字相等**；行内 `totp_secret`（密文）与 `totp_setup_at` 前后**逐字不变**（复用 = 零写入）；用该 secret 的当前窗码 enable 成功（证明响应 = 库内待验证密钥） |
| 2 | `TestTwoFactorSetupReusesSecretJustInsideWindow` | T → **T+5m−1s** 再 setup → 同 secret |
| 3 | `TestTwoFactorSetupRegeneratesAtWindowBoundary` | T → **T+5m** 再 setup → secret 不同；`totp_setup_at == T+5m`；旧 secret 的码 enable 401 `two_factor_invalid_code`；新 secret 的码 enable 200 |
| 4 | `TestTwoFactorSetupConcurrentCallsShareSecret` | 新用户 + 固定钟，服务层 8 个 goroutine 并发 `SetupTOTP`（照 `TestClaimTOTPCounterConcurrent` 的形状；不引 HTTP 层并发噪声）→ 8 个返回值 secret **全部相等**，且等于行内密文解密后的明文（至多一把不变式） |
| 5 | `TestTwoFactorSetupAtClearedByDisable` | setup → 先断言行内 `totp_setup_at` 非 nil（防空洞断言）→ disable → **nil**（且 `totp_secret` 同清） |
| 6 | `TestTwoFactorSetupAtKeptByEnable` | setup → 先断言 `totp_setup_at` 非 nil → enable → 与 setup 后**逐字相同**（钉住"enable 不碰它"） |
| 7 | `TestTwoFactorSetupLegacyPendingWithoutSetupAt` | setup → 用 ent 手工清 `totp_setup_at`（模拟本部署前的遗留「设置中」行）→ setup → 新 secret 且 `totp_setup_at` 重新落值（钉住认领谓词的 `IsNil` 分支） |
| 8 | `TestTwoFactorSetupReuseDecryptFailureIsServerError` | 同窗口内把列内密文改坏（照既有解密失败用例手法）再 setup → 500 `internal_error`；行内密文**未被替换**（不得悄悄换成新密钥） |
| 9 | `TestTwoFactorSetupReuseEmptySecretIsServerError` | 同窗口内把列改成"合法密文但明文为空串"（`sealSecret(key, "")`）再 setup → 500 `internal_error` 且列未变（钉住空明文守卫：`pquerna/otp` 对空 Secret 会静默随机生成） |
| 10 | `TestTwoFactorSetupOverwritesSecret`（**改**） | 第二次 setup 前把钟推进 **5m+1s**，其余断言不变（旧码失效、新码有效）——本用例钉的是"窗口外覆盖"语义，不删 |
| 11 | `TestTwoFactorDisableLifecycle`（**增一行**） | disable 后补断言 `totp_setup_at` 也为 nil（与其余三列并列） |
| 12 | `TestClaimTOTPSetupRejectedWhenFresh`（仓库级，变异自检补救时新增） | 同刻第一次 `claimTOTPSetup` 成功 → **同刻第二次必拒**（`setup_at` 已落值、谓词 `LTE(now-W)` 为假）→ 钟推进过窗后认领必成——变异②（认领删时间条件）唯一的确定性钉子；并发用例 {4} 对该变异依赖 goroutine 时序、不可靠（实测首测未红） |

**测试纪律**：使用假钟（`fakeTOTPClock` 改写包级 `totpNow`）的用例**禁止 `t.Parallel`**（会串改全局接缝）；现状全包零 `t.Parallel`，保持。

**变异自检（必做，防假把式用例）**：实现完成、全绿之后，按下表逐处故意改坏并确认红，再改回。**判定口径**：目标用例必须红，**连带红属预期**（逐条记录实际红了哪几个）；改回后全包绿即通过——不得为了让"其它用例不红"去改测试或加补丁代码。④ 实测不可观测已移出列检（见行内说明）；⑦ 未逐条实跑（逻辑复核：首次 setup 必落 NULL 行 → 两次认领 0 行 → 全包 500），其余各行均经实跑确认。

| 改坏点 | 实测 / 期望变红（目标 + 已知连带） |
| --- | --- |
| ① 复用条件 `<` 改 `<=` | {3}（5 分钟边界；实测） |
| ② 认领谓词的时间条件整段删掉（退化为"只查未启用"） | 确定性钉子 = 仓库级用例 `TestClaimTOTPSetupRejectedWhenFresh`（新鲜行二次认领必拒；实测红）；并发用例 {4} 对该变异依赖 goroutine 时序、单靠它不可靠（实测首测未红） |
| ③ `disableTOTP` 去掉 `.ClearTotpSetupAt()` | {5, 11}（两处都断 disable 后为 nil） |
| ④ 删掉读路径的复用分支（总是生成） | **不可观测**（实测：慢路径「认领 0 行→复读→复用」与快路径行为等价，全包不红）；零写入与复用语义的真正防线是「认领与复用互斥」（②⑥ 钉住），④ 从列检清单移除、记录在此备查 |
| ⑤ `enableTOTP` 误加 `.ClearTotpSetupAt()` | {6} |
| ⑥ `isFreshTOTPSetup` 把 `TotpSetupAt == nil` 也当新鲜（时间窗保留） | {7}（唯一有区分度的钉法：遗留行走复用、返回旧密钥；实测确认） |
| ⑥b 新鲜判定整条放宽成「有密钥就算新鲜」（时间窗一并丢弃） | {3, 7, 10}（窗口外覆盖场景也中招）；与 ⑥ 是两个不同改坏，⑥b 非本轮列检项、实测记录在此备查 |
| ⑦ 认领谓词里删掉 `TotpSetupAtIsNil()` | 几乎全包（每个用例首次 setup 都落在 `setup_at` 为 NULL 的行上 → 两次认领 0 行 → 500）；只作"该防线会被抓住"的确认，区分度靠 ⑥ |
| ⑧ `rebuildTOTPSetup` 删掉空明文守卫 | {9}（`pquerna/otp` 静默随机生成，响应 ≠ 原密钥） |

另有迁移后置断言（`totp_schema_test.go`，**独立用例 `TestTOTPSetupAtColumn`**）：不带值建用户读回 `TotpSetupAt == nil`；设值读回一致（`.Equal()` 比较）；Clear 后回 `nil`。

## 8. 文档与提交

- `backend/README.md`：迁移清单（`:143`）追加 `totp_setup_at` 一段；端点表 setup 行（`:177`）改为 5 分钟复用窗口口径（含**多设备拿同一把**的行为变更）+ 明文措辞回正；disable 行（`:179`）"清空 secret / 启用时间 / 窗号"补"设置时刻"；两步验证总述行（`:196`）加本轮 spec 指路。
- `docs/DEVLOG.md`：倒序插开头（动机与症状复发分析、5 分钟窗口裁定、认领重试与互斥依据、`updateTOTPSetup` 的死方法清理、迁移名、测试面、变异自检列检项结论 + ⑥/⑥b 拆分更正 + ⑦ 逻辑复核 + `-race` 不可用声明）。术语用「setup 复用窗口」（可注一句"用户口语称防抖"）。
- 版本：`scripts/bump-version.sh minor` → `0.17.0`（`version.go` + README 当前版本行）。
- 提交：按文件清单逐个 `git add`，禁止 `git add -A`；**是否提交与提交时点按用户当轮点名**（默认改动留工作区待检查）。
- API 规范（`2026-10-11-neoblog-api-conventions-design.md`）：零改动；冻结的 2FA 两份 spec：零改动（§3 已声明）。
- 运行期验收（在浏览器里收合面板实测）归用户；本轮的活体证据是 curl 全链路（setup → 立刻再 setup 取同一把 → 用 `backend/.scratch` 的 Go 探针把该行 `totp_setup_at` 拨回 6 分钟前 → 再 setup 取新一把 → enable）。**不真等 5 分钟**：边界已由用例 3 用假钟钉死，活体只需证"同刻同钥 + 陈旧换新"，而"陈旧"由探针几秒内造出来。

## 9. 已知缺口与风险

- **同坐操作中超过 5 分钟的间隙仍会换钥**：切出去开会/吃饭回来再展开面板 → 新密钥（须重扫）。5 分钟是上界裁定后的残余，接受；要更长的"不变"就走 §11 的显式按钮插槽，而不是继续放大窗口。
- **展开时的骨架屏闪烁仍在**：面板每次展开都从 `setup === null` 起步（`two-factor-panel.tsx:73`），必然有一次请求 + 一次骨架屏。后端复用只保证"内容不变"，不消除这次请求与闪烁——消掉它属前端轮（`forceMount` / 把 hook 状态提到外层），本轮不做。
- **认领重试的 500 理论上不可达**：认领 0 行的行必然新鲜（认领成功者刚写入 `setup_at = now`），复读时「有密钥但不新鲜」只在手工改库时出现、且此刻认领谓词必真（认领必成）；该分支**不写库**、不伪造响应，只报错。不用例覆盖。
- **复用回明文 secret**：见 §6 安全口径，仅限同账号已登录、窗口内；不放宽信任模型。
- **时钟依赖性**：窗口判定用应用侧 `totpNow()`，与 DB 的 `now()` 无关；应用与 DB 时钟不一致不影响正确性。
- **不清理陈旧待验证密钥**：窗口外不主动删，等覆盖或 disable（现状不变）。
- **主动换钥没有入口**：窗口放大后，"想换一把"既无按钮、也（现实上）无法靠等待——接口层仍有 disable（含「设置中」）可清空，UI 层待前端轮补齐。

## 10. 验收标准

1. `go build ./... && go vet ./...` 干净；`test -z "$(gofmt -l .)"`；无库 `go test ./...` 全绿，有库 `bash scripts/test.sh ./...` 全绿（含 9 条新用例与 2 条按新口径调整的既有用例）。
2. 集成用例**不含真实 5 分钟等待**；同一用例重复跑结果稳定（时间一律经 `totpNow` 固定；用假钟的用例无 `t.Parallel`）。
3. 迁移经审阅（单条可空 ADD COLUMN、无 NOT NULL / DEFAULT / 回填 / 索引）、`atlas migrate hash` + `validate` 通过、应用到 `neoblog` 与 `neoblog_test`。
4. 变异自检列检项（①②③⑤⑥⑦⑧；④ 实测不可观测移出、⑥b 备查）确认"目标用例必红（连带红逐条记录）、改回即全包绿"，结论记入 DEVLOG；⑦ 允许逻辑复核替代逐条实跑。
5. `go.mod` / `go.sum` 零变化；本轮不改 `frontend/` 任何文件（工作区若存在并行会话的前端未提交改动，一律不并入本轮提交）；API 规范与两份历史 2FA spec 零 diff。
6. README（四处）、DEVLOG、版本 `0.17.0` 就位；curl 活体全链路走通（复用同刻同钥 + 拨时刻探针验"陈旧换新"，不阻塞等 5 分钟）；`grep -rn "updateTOTPSetup" backend/` 无输出（死方法已删）。

## 11. 插槽（本轮不定）

| 插槽 | 什么时候定 |
| --- | --- |
| 显式「重新生成」按钮（面板里一个显眼入口，后端加"强制绕过窗口"的入口） | 前端轮（本轮已确认要；UI 与接口形态届时一起定） |
| "关闭后 N 分钟"精确语义（前端上报关闭时刻） | 若用户否决 §2 的计时起点口径 |
| 待验证密钥有效期/主动清理（如 24 小时未验证作废） | 出现安全需求时 |
| `totp_setup_at` 的"密钥年龄"产品化（提示用户轮换） | 更后 |
| 登录限流（login + verify） | 另开一轮（README 已知缺口） |

## 12. 下一步

本文经用户过目后进 writing-plans 出实施计划（范围 = §3，验收 = §10）。
