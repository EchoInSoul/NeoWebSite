# NeoBlog 会话管理设计（登录设备列表 + 单条吊销）

日期：2026-10-14
范围：`backend/`（Go + gin + scs + ent），**零迁移**；前端一行不动
状态：v1 设计要点经用户确认（2026-10-14 对话：列会话 / 踢单条 / IP 先信任 `X-Forwarded-For` / 最后活跃按 5 分钟节流；其余裁定授权本轮自行决定）

修订记录：
- v1（2026-10-14）：首版。
- v2（2026-10-14）：追加 §9 表结构（本轮不加表 + 两种「真表」备选及其代价），回应「数据表结构应该是咋样的」。

## 1. 意图与背景

用户目标：账号持有人能看见自己的登录会话（哪台设备、什么时间、从哪来），并能踢掉其中一条。

战略位置：D 盘 HaoBlog 有同名功能（`UserSession` 登记表 + 会话页 + 按设备吊销），本轮把它作为**功能参照**而非实现参照。HaoBlog 建登记表的根因是 NextAuth JWT 自包含、服务端数不出会话集合；NeoBlog 的会话本来就落在 Postgres（scs `postgresstore`），`sessions` 表即会话本体，能枚举、能单条删、过期由存储层每 5 分钟自动清理（`All()` 只返回未过期行）。因此本轮不建表、不迁移，只补三样：会话元数据、两个接口、（界面留待下一轮）。

## 2. 已定决策

| 项 | 决定 |
| --- | --- |
| 会话本体 | 沿用 scs `sessions` 表与 `Codec`；**不建登记表**、零 DDL |
| 元数据存放 | 会话数据键：`auth.user_agent` / `auth.ip` / `auth.created_at` / `auth.last_seen_at`，值一律 RFC3339 UTC 字符串（gob 只保证内建类型进 `interface{}`，`time.Time` 需 `gob.Register`，不用） |
| 元数据写入点 | 统一走 `establishSession` 助手：注册成功、单段登录成功、2FA verify 成功，三处 |
| 进入 2FA 待验态 | 清 `auth.user_id` **并清四个元数据键**（旧身份的设备信息不得挂在待验会话上） |
| 顺带闭合缺口 | `establishSession` 清 `auth.pending_2fa_user_id` / `auth.pending_2fa_attempts` —— DEVLOG 已登记的「已登录会话可残留另一账号 pending 身份」缺口（2026-10-14 记：2FA 轮遗留，非远程可利用） |
| 活跃时间 | `guard.TrackSessionActivity(sm)` 中间件，节流 5 分钟：距上次写入 ≥5 分钟才 `Put` 一次（未设 `IdleTimeout` 时 scs 只在数据变更时写库；`Put` 不动 `sd.deadline`，因此**不产生滑动续期**，7 天上限仍从登录时刻起算） |
| IP 取值 | `X-Forwarded-For` 首跳（trim）→ `X-Real-IP` → `c.ClientIP()`（当前 `SetTrustedProxies(nil)`，即 RemoteAddr）。**仅供展示，不作安全依据**；本轮不动代理信任配置 |
| 长度上限 | UA 截断 512 字节、IP 截断 64 字节（防止客户端用超长头把会话行撑大） |
| 会话 id | `base64url(sha256(token))`，与 scs `HashTokenInStore` 的键同形（将来开该开关则存储键即此 id）；**原始 token 永不出服务端** |
| 列表排序 | `last_seen_at` 降序（同格式定宽字符串可字典序比较），并列按 `id` 升序；空值（本轮之前建立的老会话）自然落到最后 |
| 鉴权 | 两个路由挂 `RequireAuth + LoadIdentity`，**不设权限点**（与 `PATCH /users/me` 同口径：改的是自己） |
| 删除当前会话 | **允许**：删掉当前这条 = 登出（语义正交，不做特判） |
| 未知/他人 id | 一律 404 `not_found`（不泄露他人会话存在性） |
| 版本 | `0.14.0` → `0.15.0`（`scripts/bump-version.sh minor`） |

## 3. 范围

本轮做：

- `guard`：四个会话元数据键常量 + `TrackSessionActivity` 节流中间件；在 `app.go` 与 auth 测试装配处各挂一次。
- `auth`：`establishSession` / `enterPendingSession` 助手；`SessionMeta` 类型；登录、注册、2FA verify 三处改走助手；`requestSessionMeta`（controller 侧取值 + 截断）。
- `auth`：`ListSessions` / `RevokeSession` 服务方法 + `sessionFingerprint`。
- 两个接口：`GET /api/v1/users/me/sessions`、`DELETE /api/v1/users/me/sessions/:id`。
- 测试：集成用例若干（§7）。
- 文档：README 认证章节与端点表、API 规范表追加两行、DEVLOG、版本进位。

本轮不做（明确出局）：

- 界面（控制台「账号」分组下一页，留待用户点名）。
- 会话历史 / 状态筛选 / 清除历史（无状态列，Active 之外的状态不落库；HaoBlog 的 D19 同款结论：没有值得保留的历史）。
- 批量吊销端点与集合级 `DELETE /users/me/sessions`（「退出其它所有设备」：前端可逐条调；内部 `revokeOtherSessions` 继续服务改密/2FA 流程）。
- 管理员查看/吊销他人会话（HaoBlog 的 target-user + 批量）。
- `HashTokenInStore` 加固（详见 §6 已知缺口）。
- 分页、限流（沿用全站现状）。

## 4. 接口契约

### `GET /api/v1/users/me/sessions`

鉴权：`RequireAuth + LoadIdentity`，无权限点。

```json
{"items":[
  {"id":"<base64url sha256>","current":true,"user_agent":"Mozilla/5.0 …","ip":"203.0.113.7",
   "created_at":"2026-10-14T09:00:00Z","last_seen_at":"2026-10-14T09:04:11Z","expires_at":"2026-10-21T09:00:00Z"}
]}
```

- 只出：`All()` 返回的未过期会话中 `auth.user_id` 等于当前用户的那些（待验态、他人会话天然排除）。
- `current` = 该行 token 与本次请求 token 的指纹相等。
- 老会话缺元数据时字段为空字符串（不省略键）。
- 未知字段一律不返回；无分页（个人博客规模，单用户会话数在个位）。

### `DELETE /api/v1/users/me/sessions/:id`

鉴权同上。`id` 为列表返回的指纹。

- 命中且属于当前用户 → 200 `{"id":"<id>","deleted":true}`（照 `DELETE /admin/groups/:id` 的形状）。
- 未命中 / 属于他人 / 形状非法 → 404 `not_found`。
- 幂等：同 id 再删 → 404（第二删时行已不存在）。

## 5. 服务端行为

- **建立**：三处成功路径统一 `RenewToken → Put(user_id) → Put(四个元数据)` + 清 pending 两键。首次写入即 `created_at == last_seen_at`。
- **待验态**：进入时清身份与元数据，只留 pending 键；因此待验会话不出现在任何人的列表里。
- **列表**：一次 `Store.(scs.IterableStore).All()` + 逐条 `Codec.Decode`（与 `revokeOtherSessions` 同一手法）。解码失败的行跳过（与既有吊销逻辑同口径）。全表扫描 + 逐条解码的代价随全站活跃会话数增长，个人博客规模可接受；若将来要按用户建索引，唯一干净的路径是换存储实现，不在本轮。
- **吊销**：遍历定位 `user_id` 匹配且指纹匹配的那一条 → `Store.Delete(token)`；找到即删、不再扫。
- **活跃追踪**：中间件只对 `auth.user_id` 非空的请求生效；`last_seen_at` 缺失或距现在 ≥5 分钟才写。写操作会顺带重发一次 `Set-Cookie`（scs 提交时重写 cookie，expiry 不变），无副作用。

## 6. 安全与边界

- 返回体只含指纹，不含 token；指纹不可逆推 token（256 bit 随机源）。
- 他人会话的 id 与不存在的 id 响应完全一致（404 同体），不构成枚举面。
- `X-Forwarded-For` 可被客户端伪造 → 展示字段，绝不可用作鉴权/限流依据；README 与本节同时留档（限流轮要用真实 IP 时必须先配 `SetTrustedProxies`）。
- 会话数据是 gob 编码、**非加密**（scs 默认 `GobCodec`）：UA / IP 以明文形态入库 —— 与 HaoBlog 的列存储同等暴露面，可接受。
- **已知缺口（登记，不在本轮）**：`sessions.token` 列存原始 token，库可读即会话可伪造。scs 自带 `HashTokenInStore` 可消除，代价是开启时全部存量会话失效（用户被登出一次）+ `revokeOtherSessions` 的「跳过当前会话」比较要改为指纹比较。等用户点名。
- 无速率限制的现状不变：登录/注册/verify 仍可被刷（全站已知缺口）。

## 7. 测试口径

集成测试（`internal/modules/auth/`，`DATABASE_URL_TEST` 缺省 skip；测试装配补挂 `TrackSessionActivity` 以与 `app.go` 一致）：

1. 列表含当前会话、`current=true`、元数据齐全（UA 原样、IP 取 `X-Forwarded-For` 首跳、三个时间戳可解析）；`id` ≠ cookie 里的 token。
2. 两个用户各自登录 → 各自列表只有自己的那条。
3. 直接造一行「只带 pending 键」的会话（测试侧用 `Codec.Encode` + `Store.Commit`，照 `app_test.go` 直插先例）→ 不出现在该用户的列表里。
4. 同一用户两个客户端登录 → A 看到 2 条；A 删 B 的 id → B 的受保护请求变 401，A 不受影响，A 列表回到 1 条。
5. 删他人 id / 不存在的 id / 再删同一 id → 均 404，且被瞄的会话仍在。
6. 无 cookie 打两个接口 → 401。
7. 删当前会话 → 200，随后同一客户端 401。
8. 节流：登录后连续两次受保护请求之间，`sessions` 行字节不变（<5 分钟不写）；把该行的 `last_seen_at` 改写成 6 分钟前再请求 → 行被更新为近期时间。
9. 顺带缺口：造一行带 pending 键的会话，用同一 cookie 正常登录 → 行里 pending 两键消失、`user_id` 与新元数据到位。

## 8. 文档与提交

- 提交口径：按文件清单逐个 `git add`，禁止 `git add -A`；分四到五轮提交（文档 / guard / auth 元数据 / 接口+测试 / 文档收口+版本）。
- 收口更新：`backend/README.md`（版本号、认证章节规则与缺口、端点表）、`docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md` 端点表追加两行（标 0.15.0）、`docs/DEVLOG.md`（倒序插开头）。
- 运行期验收（带真实会话在浏览器里点）归用户；本轮的活体证据是另起端口的 curl 全链路。

## 9. 表结构：本轮不加表，与两种「真表」备选

### 9.1 本轮（零迁移）：还是 scs 的 `sessions` 表

现有 DDL（`migrations/20261010223715_init_users_sessions.sql`，本轮一字节不动）：

```sql
CREATE TABLE sessions (
  id     bigint      GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
  token  varchar     NOT NULL,   -- 会话 token 明文，唯一索引 sessions_token_key
  data   bytea       NOT NULL,   -- gob：{Deadline time.Time, Values map[string]any}
  expiry timestamptz NOT NULL    -- 登录时刻 + SESSION_LIFETIME（默认 168h）
);
CREATE INDEX session_expiry ON sessions (expiry);
CREATE UNIQUE INDEX sessions_token_key ON sessions (token);
```

本轮之后一行的实际内容（`data` 解出来）：

| 列 | 内容 |
| --- | --- |
| `token` | `F9xK…`（43 字符随机串，cookie 里携带的那个值） |
| `data` | `{Deadline: 2026-10-21T09:00:00Z, Values: {"auth.user_id": "<uuid>", "auth.user_agent": "Mozilla/5.0 …", "auth.ip": "203.0.113.7", "auth.created_at": "2026-10-14T09:00:00Z", "auth.last_seen_at": "2026-10-14T09:04:11Z"}}` |
| `expiry` | `2026-10-21 09:00:00+08` |

- 元数据（UA / IP / 时间）**没有独立列**，在 `data` 的 gob 里；gob 只编码不加密（人可读）。
- 列表 = `All()`（`WHERE current_timestamp < expiry`）全表扫 + 逐条解码后按 `auth.user_id` 过滤；吊销 = `DELETE WHERE token = …`。
- 代价（并列在 §6）：时间随全站活跃会话数线性增长；不能按用户 SQL 查、无分页、无历史（吊销即删行）。
- 适用判据：单用户会话数个位数、站点规模个人博客 → 成立。

### 9.2 备选一（将来的推荐升级）：`sessions` 扩列 + 自写 scs Store

```sql
ALTER TABLE sessions
  ADD COLUMN user_id      uuid        NULL,
  ADD COLUMN user_agent   varchar     NULL,
  ADD COLUMN ip           varchar     NULL,
  ADD COLUMN created_at   timestamptz NULL,
  ADD COLUMN last_seen_at timestamptz NULL;
CREATE INDEX sessions_user_id_idx ON sessions (user_id) WHERE user_id IS NOT NULL;
```

- 自写一个约 80 行的 `scs.Store`（`Find` / `Commit` / `Delete` + `IterableStore.All`）替换 `postgresstore`；`Commit` 时顺手把 gob 里的 `auth.*` 抽进列（或由服务层按 token 补一条 `UPDATE`）。
- 收益：列表 / 踢人各一条 SQL（`WHERE user_id = $1 AND expiry > now()`）、节流 touch 变一条 `UPDATE`、不再全表扫；要历史时再加 `status` / `revoked_at` 列即可（`Find` 拒绝非 active，历史自行清理）。
- 代价：一次迁移 + 自维护存储实现（scs 升级时的兼容面）。本轮不做。
- 触发判据：活跃会话数上万，或列表/筛选/分页需求真实出现。

### 9.3 备选二（HaoBlog 式）：另建 `user_sessions` 登记表

```sql
CREATE TABLE user_sessions (
  id           uuid        PRIMARY KEY,
  user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  token_hash   varchar     NOT NULL UNIQUE,   -- base64url(sha256(token))，即本轮接口返回的 id
  status       varchar     NOT NULL,          -- active / revoked
  user_agent   varchar     NULL,
  ip           varchar     NULL,
  created_at   timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL,
  expires_at   timestamptz NOT NULL,
  revoked_at   timestamptz NULL
);
CREATE INDEX user_sessions_user_last_seen_idx ON user_sessions (user_id, last_seen_at DESC);
CREATE INDEX user_sessions_expires_idx ON user_sessions (expires_at);
```

- 唯一能直接拿到「状态与历史」的形状（筛选活跃/已吊销、清除历史、批量、管理员代管他人会话）；HaoBlog 就是这张表。
- 代价：会话状态**两份**（scs 行 = 认证真源，登记表 = 读模型）→ 双写一致性、过期清理要自己管（scs 的 5 分钟 goroutine 不认识这张表）、吊销要两处同时删。
- 触发判据：明确要做历史 / 状态筛选 / 管理端会话运维；只做「列表 + 踢一条」不值得。

### 9.4 结论

- 本轮走 9.1（零迁移）：能用 `All()` 满足的「列表 + 单条吊销」，不建第二份状态。
- 升级路径：先 9.2（表格化但不引入第二状态），要历史再在 9.2 的表上加 `status` / `revoked_at` 语义——届时无需双写。

