# 开发日志

按时间**倒序**记录每个开发阶段的变更、决策与已知缺口。设计细节与实施计划在 `docs/superpowers/{specs,plans}/`；接口与结构规范以 `backend/README.md` 为准。

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
