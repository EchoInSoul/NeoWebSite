# NeoBlog 两步验证设计（TOTP 接口 + 登录分叉 + 密钥加密 + 重放防护）

日期：2026-10-11
范围：`backend/`（Go + Gin + ent + scs + pquerna/otp + Atlas 迁移）
状态：设计经用户确认（2026-10-11 对话：范围 = 五条新接口 + 登录分叉 + `/auth/me` 状态暴露 + secret AES-GCM 加密 + 二段限次 + `totp_last_counter` 列防重放 + enable/disable 吊销其它会话；登录限流仍另开一轮）；本文为 v1.2（两轮独立审查后的修订版），待用户过目后进执行

修订记录：
- v1（2026-10-11）：首版。兑现 `2026-10-11-neoblog-2fa-schema-design.md` §6 的消费规则。
- v1.1（2026-10-11）：独立审查修订——① 重放防护由「记缺口」升级为加列 `totp_last_counter`（用户裁定）；② enable/disable 复用 `revokeOtherSessions` 吊销其它会话（用户裁定）；③ pending 进入时清除 `auth.user_id`；④ 解密失败映射 500 `internal_error`（不耗限次、不销毁会话）；⑤ enable/disable/regen 单事务；⑥ 错误信封字面量改嵌套形状；⑦ 原文的版本/路由数基线改为「执行时顺延」；⑧ 新码新路由登记进 API 规范。
- v1.2（2026-10-11，本文）：复查修订——① 窗口计算收进单一时间接缝 `totpNow`，测试据此确定性推进窗口（否则「enable 之后立刻 verify」在主链上必红）；② `matchTOTPCounter` 返 error——secret 坏 = 500，不折成验证码错误；③ 吊销前置到落库之前（照改密先例），消除「已启用但没拿到恢复码」死角；④ `loadTOTPKey()` 钉在 `Load()` 末尾，既有负例零改；⑤ 迁移与文件清单两处路径订正（`migrations/atlas.sum`、Task 6 补 `repository.go`）。

## 1. 意图与背景

2FA schema 轮已把数据插槽钉死（双列三态 + `recovery_codes` 表），上线即休眠。本轮把它变成能真实启用的功能：接口、登录分叉、恢复码、密钥加密、重放防护。表结构大部分就位；本轮仅新增一列 `totp_last_counter`（防重放），其余零迁移。

沿用已定约束：

- 方式与库：TOTP（验证器 App）+ 自管恢复码；`pquerna/otp`（本轮引入）。
- 登录分叉只看 `totp_enabled_at`；「设置中」（secret 非 NULL、enabled_at NULL）不拦登录。
- 登录限流（README 已知缺口）与本轮分开，不混做。
- secret 加密（schema 轮预留插槽）随本轮落地。

## 2. 已定决策

| 项 | 决定 |
| --- | --- |
| 依赖 | `github.com/pquerna/otp`；参数 SHA1 / 6 位 / 30s / Skew=1（±1 窗口） |
| secret 存储 | AES-256-GCM 加密后入列；密钥走 `TOTP_ENCRYPTION_KEY`（base64、32 字节、必需，缺失/非法启动即失败） |
| 恢复码 | 10 个/批；每码 16 随机字节 → 32 位小写 hex（128 bit 熵）；库中只存 sha256 hex（64 字符）；无有效期 |
| 恢复码再生成 | 密码确认（照顾「验证器丢了」自救）；删旧发新单事务；不要求 TOTP 在场、不吊销会话（需要密码且已登录，旧码作废本身不扩大攻击面） |
| 登录分叉响应 | 401 嵌套信封 `{"error":{"type":"authentication_error","code":"two_factor_required","param":null}}`（与 `apierror.Write` 一致；200 永远 = 已完全登录） |
| pending 会话 | scs 键 `auth.pending_2fa_user_id`；**进入 pending 时清除 `auth.user_id`**（同一会话不可能「已登录」与「等二段」并存）；守卫零改动 |
| 二段限次 | 会话内 `auth.pending_2fa_attempts`，连错 5 次销毁会话、响应 `two_factor_locked`；**每轮登录 5 次、重登续杯——不构成在线暴破防御，闸门在登录限流轮（login 与 verify 同做），见 §9** |
| 重放防护 | `users` 加列 `totp_last_counter`（bigint 可空，ent `*int64`）；验证命中的时间窗计数 > 已存值且原子条件更新认领成功才通过（RFC 6238 §5.2）；同窗重放直接拒；窗口时间只经包内 `totpNow`（默认 `time.Now`）一处取得 |
| 会话吊销 | enable / disable **在验码/密码通过之后、落库之前**调用既有 `revokeOtherSessions`（保留当前会话）：怀疑密码泄露才开 2FA 的场景，攻击者旧会话必须立即失效。吊销失败 = 整个动作不生效（500），不留「已启用但没领到恢复码」死角 |
| 事务边界 | enable（置 enabled_at + 插 10 码）、disable（清三列 + 删全部码）、regen（删旧 + 插新）各自单事务（沿用 access/settings 的 `client.Tx` 先例） |
| 版本 | minor 进位，按执行时当前版本顺延。**mail 轮已入库并占用 0.9.0（HEAD `d38ce17`），故本轮实测为 0.9.0 → 0.10.0**；DEVLOG 记实际版本 |

## 3. 范围

本轮做：

- 依赖引入、config 增量（`TOTP_ENCRYPTION_KEY`）、AES-GCM 原语。
- **迁移：`users` 加 `totp_last_counter`（可空 bigint）+ ent schema 同步**（唯一一个）。
- apierror 新增 6 个错误码；auth repository / service / controller / dto 增量。
- 5 条新接口 + 登录分叉 + `/auth/me` 加 `two_factor_enabled`。
- README / `.env.example` / API 规范登记 / DEVLOG / 版本进位。

本轮不做：登录限流；2FA 邮件通知；passkey / API token；前端接线；`recovery_codes` 表结构变更。

## 4. 三态与登录行为（承接 schema spec §4）

| `totp_secret` | `totp_enabled_at` | 状态 | 登录行为 |
| --- | --- | --- | --- |
| NULL | NULL | 未设置 | 单段登录，与今天一致 |
| 非 NULL | NULL | 设置中 | 单段登录（setup 可重复覆盖；disable 可中途放弃） |
| 非 NULL | 非 NULL | 已启用 | bcrypt 通过后：`RenewToken` → 清除 `auth.user_id` → 写 `auth.pending_2fa_user_id` → 401 `two_factor_required` |

## 5. 接口设计（全部在 `/api/v1` 前缀下）

命名口径：`/auth/2fa/<动作>` 属 `POST /<域>/<子域>/<动作>` 形态（动作段含资源名词 `2fa`），沿用 `auth/logout` 的动作 POST 先例；五条路由登记进 API 规范 §3 端点全集表。

### 5.1 `POST /auth/2fa/setup`（守卫内）

- 前置：已启用 → 409 `two_factor_already_enabled`（禁止覆盖已启用用户的 secret，先 disable）。
- 行为：`totp.Generate`（Issuer `NeoBlog`，AccountName 用户 email）生成 secret；AES-GCM 加密后写 `totp_secret`，**清 `totp_last_counter`**（新 secret 计数归零），`totp_enabled_at` 保持 NULL；重复 setup 覆盖旧 secret（旧码立即作废）。
- 响应 200：`{"secret":"<base32 明文，仅此一次>","otpauth_url":"<otpauth://totp/...>"}`。二维码前端渲染，后端只吐 URI。

### 5.2 `POST /auth/2fa/enable`（守卫内）

- 请求体：`{"code":"123456"}`。
- 前置：`totp_secret` 为 NULL → 409 `two_factor_not_setup`；已启用 → 409 `two_factor_already_enabled`。
- 行为：解密 secret（**解密/解码失败 → 500 `internal_error`**，不是验证码错误）；按 §5.5 的窗口匹配取候选计数与码（未命中 → 401 `two_factor_invalid_code`，param `code`）；匹配通过后**先吊销该用户其它会话**（保留当前，取 `session` cookie 令牌；失败 → 500 且库里一切不变）；再认领计数 + **单事务**写 `totp_enabled_at` 并插入 10 个恢复码（只存哈希）。
- 响应 200：`{"recovery_codes":["<32hex>",…×10]}`——明文只回这一次。

### 5.3 `POST /auth/2fa/disable`（守卫内）

- 请求体：`{"password":"..."}`；密码错 → 401 `invalid_credentials`。
- 前置：未设置（`totp_secret` NULL）→ 409 `two_factor_not_setup`。「设置中」允许调（中途放弃）。
- 行为：密码通过后**先吊销其它会话**（保留当前；失败 → 500，库里不变）；再 **单事务**清 `totp_secret` / `totp_enabled_at` / `totp_last_counter`（回到「未设置」）+ 删除该用户全部恢复码行（含已用的，不留历史；后果见 §9）。
- 响应 200：`{}`。

### 5.4 `POST /auth/2fa/recovery-codes`（守卫内，再生成）

- 请求体：`{"password":"..."}`；前置：未启用 → 409 `two_factor_not_enabled`；密码错 → `invalid_credentials`。
- 行为：**单事务**删旧批全部行 + 插新批 10 个。
- 响应 200：`{"recovery_codes":[…]}`——明文只回这一次。

### 5.5 `POST /auth/2fa/verify`（**公开路由**，靠 pending 会话）

- 请求体：`{"code":"..."}`；空 → 400 `invalid_request`（param `code`）。
- 前置：会话无 `auth.pending_2fa_user_id` → 401 `unauthorized`。pending 用户不存在或 `totp_enabled_at` 为 NULL → 销毁会话、401 `unauthorized`。
- TOTP 分支：解密 secret；**解密/解码失败、或 secret 本身非法（base32 解不开）→ 500 `internal_error`，不消耗限次、不销毁会话**（密钥轮换/数据损坏不得伪装成验证码错误）。时间基准只经包内 `totpNow`（默认 `time.Now`）取一次。按候选窗 `{⌊now/30⌋-1, ⌊now/30⌋, ⌊now/30⌋+1}` 逐窗比对码（常数时间比较），命中计数 `c` 后**原子认领**：`UPDATE users SET totp_last_counter=c WHERE id=? AND (totp_last_counter IS NULL OR totp_last_counter < c)` 影响行数 = 1 才通过；0 = 重放或并发竞败 → 按验证码错误处理（计入限次）。认领与后续会话写入不在同一事务：认领成功而会话写失败（500）时该窗作废，重试须待下一窗（见 §9）。
- 恢复码分支：输入 trim + 转小写后 sha256，单条 `UPDATE … WHERE user_id=? AND code_hash=? AND used_at IS NULL` 原子消费；影响行数 = 1 即命中。
- 未命中：`auth.pending_2fa_attempts` +1；达到 5 → 销毁会话、401 `two_factor_locked`；否则 401 `two_factor_invalid_code`（param `code`）。
- 命中：`RenewToken` → 写 `auth.user_id` → 清两个 pending 键 → 200 `{"email":"…","nickname":"…"}`（与 login 成功同形）。

### 5.6 `GET /auth/me`（改）

响应顶层加 `"two_factor_enabled": bool`（= `totp_enabled_at` 非 NULL）。其余字段不动。

### 5.7 登录分叉（改 `POST /auth/login`）

bcrypt 通过后：已启用 → `RenewToken`、**`Remove` 会话中的 `auth.user_id`**、写 `auth.pending_2fa_user_id` → 401 `two_factor_required`（嵌套信封）；未启用/设置中 → 照旧 200。失败路径（密码错、dummy bcrypt）与今天一致。

## 6. 会话与安全

- 会话键：`auth.pending_2fa_user_id`（string）、`auth.pending_2fa_attempts`（int），常量在 auth 模块内；**guard 包零改动**（`RequireAuth` 只认 `auth.user_id`）。scs v2.9.0 无 `Del`，清理用 `Remove`；读键用 `GetString`/`GetInt`。
- 两阶段各 `RenewToken` 一次；pending 进入时清 `auth.user_id`——已登录会话再登已启用账号，必须重新完成二段才算登录（贴合「401 = 未完成登录」语义）。
- AES-GCM：随机 12 字节 nonce 前置、密文 base64 入列；密钥在 `config.Load` 解码一次（base64 → 恰 32 字节），缺失/非法/长度错启动即报错（信息指名变量 + 生成命令 `openssl rand -base64 32`）。
- 重放防护：`totp_last_counter` 认领规则见 §5.5；`setup` 覆盖与 `disable` 都会清该列。窗口计算的时间基准收在包内变量 `totpNow`（默认 `time.Now`，全模块唯一取时点）——测试据此把窗口确定性前移，不靠真实 30 秒等待，也不靠碰运气跨窗。
- 会话吊销：`revokeOtherSessions` 是既有能力（`auth/service.go`，改密已在用），本轮在 enable / disable 的**验码或密码通过之后、落库之前**调用；顺序如此是为了让任何失败点都不产生「库里已启用、恢复码却没发出去」的死角（否则重试 enable 会撞 409，那批码永久拿不到，只能走再生成）。
- 归一化单点：恢复码的 trim + 转小写只在 `hashRecoveryCode` 内做一次，调用方传原始输入；TOTP 码在匹配函数内 trim。
- 恢复码哈希 sha256 无盐（128 bit 熵，schema §9 已论证）；`code_hash` 的 64 字符 hex 由构造钉住（sha256 hex），不加 DB 约束、零迁移。

## 7. 错误码（apierror 增量，6 个）

| Code | Type | HTTP | 语义 |
| --- | --- | --- | --- |
| `two_factor_required` | authentication_error | 401 | 密码已对，等二段 |
| `two_factor_invalid_code` | authentication_error | 401 | 验证码/恢复码不对（enable 与 verify 共用；重放被拒同码；param `code`） |
| `two_factor_locked` | authentication_error | 401 | 二段连错 5 次，会话已销毁 |
| `two_factor_already_enabled` | conflict_error | 409 | 已启用还 setup / enable |
| `two_factor_not_setup` | conflict_error | 409 | 没有 secret 就 enable / disable |
| `two_factor_not_enabled` | conflict_error | 409 | 未启用就再生成恢复码 |

**每个码必须同时登记 `statusByCode`**（`Body()` 对未登记码兜底 `internal_error`）；并登记进 API 规范 §7.3 注册表。注：disable / regen 的密码确认沿用 `invalid_credentials`（401）——与 API 规范 §0 v1.3 裁定一致（该轮明确不拆 `current_password_mismatch`），规范 §7.3 的「仅登录端点」场景列随本轮登记一并订正。

## 8. 测试

| 层 | 连库 | 内容 |
| --- | --- | --- |
| config 单测 | 否 | 缺 `TOTP_ENCRYPTION_KEY` 失败；非 base64 / 非 32 字节失败；合法值解出 32 字节（每个用例先设 `DATABASE_URL`，失败用例按前置顺序给断言） |
| secretbox 单测 | 否 | 回环一致；篡改报错；错钥报错；同明文两密文不同 |
| totp 原语单测 | 否 | 当前窗命中计数 = ⌊now/30⌋；±30s 邻窗命中；±90s 拒；乱码拒；**非法 base32 secret → 返回 error（不是「不匹配」）**；不同窗码不同；恢复码 10 个 32 位小写 hex 唯一；哈希归一（`" ABcd "` == `"abcd"`）且 64 字符 |
| 迁移后置断言 | 是（缺省 skip） | `totp_last_counter` 不带值建用户读回 nil；设值读回一致；Clear 后回 nil |
| 集成（auth 域） | 是 | ① 全生命周期：setup→enable（10 码）→登出→login 401 `two_factor_required`→pending 打受保护接口 401→verify 正确码 200→me `two_factor_enabled:true`；② 恢复码登录成功+复用被拒；③ 连错 4 次后第 5 次 `two_factor_locked`、会话销毁、重走登录可恢复；④ disable 后单段登录、旧码全灭、me false、三列清空；⑤ regen 后旧批失效；⑥ setup 覆盖：旧 secret 码失效、新码有效；⑦ 已启用 setup / 未 setup enable / 未 setup disable / 未启用 regen 各 409；⑧ 密文落库（列值 ≠ 明文、可解密回环）；⑨ `consumeRecoveryCode` 并发同码恰一人成功；⑩ `claimTOTPCounter` 并发同窗恰一人成功、存量高计数时低窗码被拒；⑪ 同码第二次 verify（同窗重放）401 `two_factor_invalid_code`；⑫ 解密失败（列塞明文）→ 500 `internal_error`、不耗限次、不销毁会话（修回密文后正确码仍 200）；⑬ enable 吊销其它会话（第二 jar 401、当前 jar 存活）；disable 同理（先以新会话登录再 disable，旧 jar 401）；⑭ 已登录会话再登已启用账号 → 401 `two_factor_required` 且原 `auth.user_id` 已清（受保护访问 401） |

测试数据随机后缀 + `t.Cleanup`；集成走 `DATABASE_URL_TEST`；fixture 固定在 auth 包（复用 `auth_test.go` 手法），测试密钥用包内固定常量（供密文反解断言）。

**窗口纪律（必须，否则主链用例必红）**：enable 本身就认领掉当前窗，因此凡「先 enable/verify 成功、之后还要再验一次正码」的用例（①③⑫⑭），后续那一次必须把 `totpNow` 前移到更大的窗再取码；严禁依赖真实时钟恰好跨过 30 秒边界来碰运气通过。⑪ 是同窗重放的专门用例，反而要钉在同一个窗内。

## 9. 已知缺口与风险

- **`TOTP_ENCRYPTION_KEY` 是部署面新增强依赖**：缺失启动即失败；密钥丢失 = 已启用用户验码时解密失败（500），只能 DB 清列重置；README 记录备份要求。
- **二段限次不是在线暴破防御**：每轮登录 5 次、重登续杯，持密码者可无限开轮试 6 位码；真正的闸门在登录限流轮（login 与 verify 同做）。
- **重放防护的代价**：同一时间窗内已认领的码整窗作废——用户在该 30 秒内登出再登录需等下一窗（标准取舍，RFC 6238 §5.2）。**启用那一刻也消耗当窗**：enable 用的是哪一窗的码，那一窗此后不能再用于 verify。
- **认领不回滚**：verify 的计数认领与后续会话写不在同一事务，会话写失败（500）时该窗已作废，用户重试需下一窗；enable 的认领与落位分两步，落位 Tx 失败同样废一窗。两者都只影响一次重试体验，不产生权限或数据不一致。
- **吊销是启用/关闭的前置步**：`revokeOtherSessions` 失败则本次 enable/disable 整体不生效（500，库里不变）。副作用是极端情况下其它设备已被登出而 2FA 没开成——用户在未启用状态下重登是单段的，再点一次启用即可。
- **disable 删全部恢复码行**（含已消费的）：不复留存根，无法再统计用户消费过几个码；如需审计须改为标记作废（本轮不需要）。
- **pending 会话与 `SESSION_LIFETIME`**：pending 沿用默认 168h，限次 5 次是唯一闸门（见上）。
- **恢复码明文只出现一次**；丢批只能再生成（密码确认）。
- **`two_factor_required` 复用 401**：前端按 `code` 分叉而非状态码。

## 10. 验收标准

1. `go build ./... && go vet ./...` 干净；`test -z "$(gofmt -l .)"`；无库 `go test ./...` 全绿，有库 `go test -p 1 ./...` 全绿。集成用例**不含真实 30 秒等待、也不得靠恰好跨窗通过**（时间一律经 `totpNow` 接缝控制；反复跑同一个用例结果必须稳定）。
2. 迁移 `totp_last_counter` 经审阅（单条可空 ADD COLUMN、无回填）、`atlas migrate hash` / `validate` 通过、应用到 `neoblog` 与 `neoblog_test`。
3. 路由从执行时基线 +5 条。**修订记录基线实测**：mail 轮已入库（HEAD `d38ce17`），现仓 15 条（auth 5 + access 8 + mail 1 + healthy 1）→ 本轮后应为 **20 条**；执行时仍以 `grep` 现值为准。guard 包相对执行起点零 diff。
4. `go.mod` 新增且仅新增 `pquerna/otp` 及其传递依赖。
5. 手动验收：curl 全流程（setup 取 otpauth_url + secret → 一次性命令算当前码 → enable 收恢复码 → 登出 → 二段登录 → 恢复码兜底 → disable）走通。
6. README（环境变量表 + 认证章节 + 接口清单）、`.env.example`、API 规范（§3 +5 行、§7.3 +6 行）、DEVLOG 更新；版本按执行时顺延进位。

## 11. 插槽（本轮不定）

| 插槽 | 什么时候定 |
| --- | --- |
| 登录限流（login + verify，IP/账号维度） | 另开一轮（README 已知缺口） |
| 2FA 相关邮件通知（新设备启用/关闭提醒） | 邮件消费轮 |
| secret 换钥（TOTP_ENCRYPTION_KEY 轮换 + totp_last_counter 一并处置） | 出现需求时（需重加密回填工具） |
| Passkey / API Token / 社交登录 | auth 设计 §2 更后项 |

## 12. 下一步

本文经用户过目后进 writing-plans 出实施计划（范围 = §3，验收 = §10）。
