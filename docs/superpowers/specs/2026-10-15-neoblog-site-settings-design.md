# NeoBlog 站点设置组 site（清单扩展，零迁移）

**日期**：2026-10-15 · **范围**：纯后端（设置清单 + 测试 + 文档；零迁移、零接口、零前端） · **实施计划**：`docs/superpowers/plans/2026-10-15-neoblog-site-settings.md`（随本 spec 一并交付）

**状态**：v1 草案，待用户审阅。第 2 节 D4 / D6 两处此前给过备选、未收到否决，为「按推荐拟定」——请重点看这两处。

---

## 1. 意图与背景

用户 2026-10-15 点名：给后端加「站点标题 / 副标题 / 站点描述 / 站点关键词 / 站点 URL」这类站点级设置。用途是前台元数据的公共来源——`<title>`、`meta description` / `meta keywords`、canonical / OG 链接；将来邮件模板的链接基址大概率也读 `site.url`。

这轮几乎零成本的原因（机制现状，实读）：

- 设置机制已落地（存储机制 0.8.0 → 管理路由 0.12.0 → 公开读 0.13.0）：`setting_values` 单表只存「被写入过的值」，空表 = 全默认；「有哪些设置」全部定义在 `backend/internal/modules/settings/registry.go` 的 `builtinRegistry`（settings spec §1 的既定口径：**新增设置只改代码、永不操作数据库**）。
- 接口已就位：管理侧 `GET / PATCH /api/v1/admin/settings/:group`（`system.settings.view` / `system.settings.edit`）、公开侧 `GET /api/v1/public/settings`（无鉴权，只出标 `Public` 的项）。
- 因此本轮的**全部生产代码改动 = 清单里加一个组**；路由、权限点、表结构、迁移全部为零。

键名口径：与 D 盘 HaoBlog 的 `site.title / site.subtitle / site.description / site.keywords / site.url`（`lib/settings/groups/site-settings.ts`）逐字一致，两个项目以后对照不打架。

## 2. 拟定决策（待审阅）

| # | 决策 | 口径 | 备注 |
| --- | --- | --- | --- |
| D1 | 组名 | `site` | 与 HaoBlog 同；组是设置的管理粒度 |
| D2 | 项与命名 | `title` / `subtitle` / `description` / `keywords` / `url` | 用户点名的五项；与 HaoBlog 逐字一致 |
| D3 | 类型 | 五项全 `FieldTypeString` | 框架三种类型中最直白的一种；五项本就是文字 |
| D4 | 默认值 | 全空串 `""` | 空串 = 未配置，前台语言包兜底。**备选**（`title` 默认 `"NeoBlog"`）未取：默认值会让人分不清「配过还是没配」 |
| D5 | 公开性 | 五项全 `Public: true` | 五项都是前台匿名要读的；无敏感项 |
| D6 | keywords 形态 | 英文逗号分隔的**单个字符串** | 框架无列表类型；HaoBlog 同形（界面标签输入、存库 join、读侧 split）。**备选**（本轮不做 keywords / 给框架加数组类型）未取 |
| D7 | 校验 | 不做长度 / URL 格式校验 | 「类型之外的校验」是机制留的插槽；单管理员场景填错自己看得见，首个真实校验需求出现再动写入把关 |
| D8 | 组序 | `site` 排在 `register` 之前 | 公开聚合按清单序输出 `items`；组序只影响这个数组的排列（前端按组名取值，不依赖顺序） |
| D9 | 落地方式 | 只动清单 + 测试 / 文档同步；不新增路由 / service 方法 / 权限点 / 迁移 | 机制已够用 |

## 3. 范围

**改动文件（7 个）**：

| 文件 | 动作 | 职责 |
| --- | --- | --- |
| `backend/internal/modules/settings/registry.go` | 改 | `builtinRegistry` 加 `site` 组（五项）——唯一生产代码改动 |
| `backend/internal/modules/settings/registry_test.go` | 改 | 公开白名单守卫更新 + 形状断言补 `site` 组；配方守卫随 README 示例换名联动（`recipeGroups` / `TestRegistryRecipeShape`） |
| `backend/internal/modules/settings/controller_test.go` | 改 | 公开读断言由「只有 register」改为两组；新增一条「管理写 → 公开读」端到端用例 |
| `backend/README.md` | 改 | 「当前清单项」两处 + 公开负载示例加「（节选）」注记 + 「怎么加一个设置」配方示例组名换 `demo` + 顶部版本行（进位脚本代劳） |
| `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md` | 改 | 端点表 #23 的示例负载尾部加「（节选）」 |
| `docs/DEVLOG.md` | 改 | 顶部倒序登记本轮 |
| `backend/internal/version/version.go` | 改 | minor 进位 0.18.0 → 0.19.0（`scripts/bump-version.sh minor`，连带 README 版本行） |

**不动清单**：`frontend/**` 全部（前端接线是后续前端轮）；`controller.go` / `service.go` / `repository.go` / ent schema / 迁移（零数据库动作）；权限目录（`system.settings.*` 已覆盖本组）；API 规范文档的端点与错误口径本身（对站点设置零变化——仅上述 #23 示例标注）；`sampleGroups` / `TestServicePublicSettings` 等测试夹具的组名（测试内部约定，不改名）。

## 4. 清单形状

`registry.go` 的 `builtinRegistry` 变为（`site` 在前）：

```go
var builtinRegistry = []Group{
	{
		Name: "site",
		Items: []Definition{
			{Name: "title", Type: FieldTypeString, Default: "", Public: true},
			{Name: "subtitle", Type: FieldTypeString, Default: "", Public: true},
			{Name: "description", Type: FieldTypeString, Default: "", Public: true},
			{Name: "keywords", Type: FieldTypeString, Default: "", Public: true},
			{Name: "url", Type: FieldTypeString, Default: "", Public: true},
		},
	},
	{
		Name: "register",
		Items: []Definition{
			{Name: "enabled", Type: FieldTypeBool, Default: true, Public: true},
		},
	},
}
```

公开端点负载前后对比（组序 = 清单序；组内键序是 Go 对 `map` 的 JSON 编码序，即字典序，与声明序无关）：

- 现在：`{"items":[{"group":"register","values":{"enabled":true}}]}`
- 加完：`{"items":[{"group":"site","values":{"description":"","keywords":"","subtitle":"","title":"","url":""}},{"group":"register","values":{"enabled":true}}]}`

## 5. 行为

零新语义，全部沿机制既有行为（settings spec §6）：

- 读：未写过 → 默认空串；写过 → 按类型校验后返回值。
- 写：`PATCH /api/v1/admin/settings/site`，可只给子集（如 `{"title":"我的博客"}`）；类型不符 / 未知项 → 400 带 `param`（既有机制）。
- 公开读：`GET /api/v1/public/settings` 自动多出 `site` 组（五项全公开）。
- 恢复默认（删行）暂无 HTTP 接口，与机制现状一致。

## 6. 测试（三处更新 + 一条新增，零新增文件）

研究阶段实读确认：**不补任何测试，本轮清单改动也会让两处既有断言先红**（公开白名单、匿名公开读）；再补一处形状断言与一条端到端用例，共四处：

| 文件 / 测试 | 现状（实读） | 改法 |
| --- | --- | --- |
| `registry_test.go` · `TestBuiltinPublicAllowlist` | 公开项集合钉死为 `[register.enabled]`（单元素断言） | 期望改六项（`register.enabled` + `site` 五项），并**先排序再比较**——现写法逐组遍历组内 `map`，一组多项时顺序随机，不排序会偶发红 |
| `registry_test.go` · `TestBuiltinRegistryShape` | 只断言 `register.enabled` | 补 `site` 组断言：五项齐、类型 string、默认空串、`Public`——把「地基」钉住，防后续误改 |
| `controller_test.go` · `TestSettingsPublicGetAnonymously` | 断言公开负载**只有** register 一组 | 改为两组、组序 site → register、`site` 五项空串默认；`resetGroups` 由 `"register"` 扩为 `"register", "site"`（防别处残留行污染） |
| `controller_test.go` · **新增** `TestSettingsPatchSiteTitleReadsBackPublicly` | — | 端到端一条：owner `PATCH /admin/settings/site` 写 `title` → 200 回显（其余项仍空串）→ 匿名 `GET /public/settings` 读回新值。证明「管理能写、前台能读」这条本轮的核心价值链 |

变异自检（实施时逐条执行，证明会红再改回）：

| # | 变异 | 预期红 |
| --- | --- | --- |
| ① | `site.url` 取消 `Public` | 白名单（六项变五项）、形状断言、公开读断言（site 只剩四项） |
| ② | `site.title` 默认值改为非空串 | 形状断言、公开读断言 |
| ③ | 两组互换顺序（register 在前） | 公开读断言（组序）、端到端用例；白名单保持绿（**证排序生效**） |

另：README「怎么加一个设置」的配方示例组名与真实组重名（同文档会出现两个 `site.title`），本轮顺手把示例组名由 `site` 换成 `demo`——`registry_test.go` 的 `recipeGroups()` 与 `TestRegistryRecipeShape` 原子联动；纯文档消歧，不涉红绿（改完该守卫仍绿）。

## 7. 已知缺口与风险

- **五项后端均无消费方**：本轮加完，只有公开端点能读到；真正出现在页面（`<title>`、meta）要等前端接线轮，界面现状不变。
- **keywords 是逗号分隔字符串**：将来做标签输入时前端负责 split / join（HaoBlog 同款）；若真要列表类型，属框架级变更（types / service / repository 三处），不在本轮。
- **空串默认 = 未配置**：前台需自行兜底；`title` 为空时 `<title>` 的空缺由前台负责。
- **无长度校验**：超长值原样入库并公开返回（单管理员场景风险低；校验插槽留待需要时）。
- **`site.url` 是「每个部署一份」的值**：换域名要记得改；将来邮件模板若读它，是第二消费方。
- **公开面扩大**：公开端点多出 site 组；`TestBuiltinPublicAllowlist` 的本次更新即此后每次公开项变化的强制复核点。

## 8. 插槽（本轮不定）

| 插槽 | 时点 | 影响面 |
| --- | --- | --- |
| 长度 / URL 格式校验 | 首个校验需求出现时 | `Definition` 加字段 + 写入把关（service.go） |
| 更多站点项（logo / favicon / 备案号 / 页脚 / 时区 / 屏蔽搜索引擎等） | 用户点名时 | 纯清单加行、零迁移（HaoBlog 的 site 组还有一批同类项可对照） |
| 前端接线（选择器 + `generateMetadata`） | 前端轮 | `frontend/lib/public-settings.ts` + 根 layout |
| keywords 列表类型 / 标签输入 | 若确需 | 框架级类型扩展 |

## 9. 验收标准

1. `go build ./...` / `go vet ./...` / `gofmt -l .`（backend 下）干净。
2. `source .env && bash scripts/test.sh` 全绿（串行；含上述四处测试）。
3. 变异自检三条（§6）逐条实测红后再回绿。
4. 活体（建议；用 `.scratch` 探针，不依赖管理员账号）：临时实例公开端点多出 site 组（五项空串）→ 探针 `SetString` 写 `title` → 公开读回新值 → 探针 `ResetItem` 删行还原 → 读回空串，随后关闭实例。开发库无 owner 账号（冒烟账号是普通用户，`PATCH` 必 403），管理路由的写链路由测试库 e2e 用例覆盖。
5. README 四处更新（两处「当前清单项」+ 公开负载示例「（节选）」注记 + 配方示例换 `demo`）；API 规范 #23 示例加「（节选）」；DEVLOG 条目；版本 0.18.0 → 0.19.0。
6. 零迁移、零新依赖、零新路由、零前端改动。

## 10. 下一步

本 spec 经审阅（重点 D4 / D6）后，按实施计划 `docs/superpowers/plans/2026-10-15-neoblog-site-settings.md` 执行；执行方式由用户选（subagent-driven / native）。**在用户明确说开始之前，不动任何代码、不提交任何文件。**
