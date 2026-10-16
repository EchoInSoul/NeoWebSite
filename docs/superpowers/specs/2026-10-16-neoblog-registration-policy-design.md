# NeoBlog 注册域名控制与密码策略（register/auth 首批消费 + 设置写前校验插槽）

**日期**：2026-10-16 · **范围**：纯后端（Go + gin + ent；生产代码改动集中在 `settings` 与 `auth` 两个模块） · **实施计划**：`docs/superpowers/plans/2026-10-16-neoblog-registration-policy.md`（随本 spec 一并交付）

**状态**：v2（2026-10-16 独立审查后修订），待用户审阅。范围与三条行为口径按 2026-10-16 对话用户裁定（原话「可以，现在开始写spec和plan吧」，回复的是当轮复述：只做邮箱域名控制 + 密码要求/最短长度、不做注册默认组、保存时拦住、只拦公开注册、匹配口径三条）。

修订记录：
- v1（2026-10-16）：首版。
- v2（2026-10-16，独立审查后）：校验加「管辖项门槛」（脏存量不封锁无关写入，§5）；上限 72 → 64（§2 D3、§5、§8、§9）；版本不再写死、执行时现读（§2 D7、§11）；API 规范登记点收敛回 §7.3、§13.2 表 B 不动（§7）；证据与措辞修正（§4 Set* 口径、§6.2 失败模式、§6.3 CreateAdminUser 措辞、§9 匿名可见性）；变异表补 ⑥⑦。
- **后续裁定（2026-10-16，同日，已实施）**：用户改判「开关开 + 清单为空」**不拦截**、合法可存。本 spec 中「register 组挂组级校验、`setting_conflict` 覆盖空名单」的口径（§1 D5、§5、§8 表、`§9` 测试 ①、§10 活体 4）**已被覆盖**：register 组不再挂校验器，`setting_conflict` 现只剩 `password_min_length`；§1 D8 描述的 fail-open 运行时行为不变，并从「不可达态」升为唯一口径。正文按当轮决策原样留存，权威口径见 `backend/README.md`「设置」章节与 DEVLOG 同日条目。

---

## 1. 意图与背景

0.20.0 落地的 `register` / `auth` 两组设置（共 14 项）目前几乎全是纯数据：唯一真实消费方是注册总闸 `register/enabled`（0.13.0 轮，`auth/service.go:49` 首句读取）。用户 2026-10-16 点名先接其中四项，并明确**暂时不做注册默认组**：

| 设置项 | 现行为 | 本轮接线后的行为 |
| --- | --- | --- |
| `register.email_domain_control_enabled` + `register.allowed_email_domains` | 无消费方 | 开启且清单条目非空时，`POST /users`（公开注册）拒绝域名不在清单里的邮箱 |
| `auth.password_requirements_enabled` + `auth.password_min_length` | 无消费方；前后端各自写死「≥8 位、含字母和数字」 | 开启后注册与改密按设置里的最短长度校验；关闭时行为与现在逐字一致 |

明确不做（用户裁定或本轮范围外）：**注册默认组**（搁置——值形状「组键 vs 组 ID」尚未裁定）、密码复杂度（自由文字无可机器执行的语义，先当说明文字）、邮箱认证、会话有效期、2FA、人机验证；前端一行不动。

标题里的「设置写前校验插槽」是实施上必要的机制扩展：两条「保存时就拦住」的约束（§2 D4）落不进现有机制（写路径只做逐项类型把关），需要给设置组加一个落库前的组合校验钩子。

## 2. 已定决策

| # | 决策 | 口径 | 出处 |
| --- | --- | --- | --- |
| D1 | 本轮范围 | 四项键接线：`register.email_domain_control_enabled`、`register.allowed_email_domains`、`auth.password_requirements_enabled`、`auth.password_min_length`；注册默认组明确搁置（另议） | 用户 2026-10-16 裁定 |
| D2 | 域名匹配口径 | 大小写不敏感（`EqualFold`）；清单按逗号拆分（`,` 与 `，` 都认）、条目两端空白去除、空条目丢弃；精确域名匹配——条目写 `example.com` **不**放行 `sub.example.com`；只拦 `POST /users` 公开注册，`auth.CreateAdminUser` 与 `PATCH /users/me`（改邮箱）不拦 | 用户 2026-10-16 确认（复述三条口径） |
| D3 | 密码策略口径 | 开关关 → 最短长度取现状写死的 8，其余（字母+数字、≤72 字节）逐字不变；开关开 → 取 `password_min_length` 并**防御性夹在 [8, 64]**（下限按 rune、密码上限按字节 72——取 72 会变成「合法但只允许 72 位纯 ASCII」的死值，64 留 8 字节余量；正常路径由 D4 保存校验保证，夹取兜旧行/直写库）；作用点 `Register` 与 `ChangePassword`（只查新密码）；`password_complexity` 不执行 | 用户 2026-10-16 确认；上限 64 为审查后修订 |
| D4 | 写前校验插槽 | 设置机制加「组级写前校验」：`PATCH /admin/settings/:group` 在合并最终态后、落库前调用；**只在本次写入与规则管辖项（`ValidateOn`）相交时触发**（脏存量不封锁无关写入）；两条规则——「开域名控制必须清单条目非空」（管辖 `email_domain_control_enabled` / `allowed_email_domains`）、「`password_min_length` ∈ [8, 64]」（管辖自身）；仅 admin PATCH 路径（唯一 HTTP 写路径）；`Set*` / `SetGroup` / `ResetItem` 为内部与探针通道，不校验（tracked 代码零调用） | 用户 2026-10-16 确认（「保存时就拦住」）；管辖项门槛为审查后修订 |
| D5 | 新错误码 | `email_domain_not_allowed`（400 / `invalid_request_error`，param 默认 `email`）；`setting_conflict`（409 / `conflict_error`，param 由设置端点覆盖为**判定项名**：`email_domain_control_enabled` / `password_min_length`——不是清单项） | 拟定，列此备查 |
| D6 | 公开面与前端 | 公开负载零变化（四项仍非公开）；前端一行不动（注册页提示动态化、控制台「用户注册」页接线都是后续前端轮） | 用户 2026-10-16 确认（「那是前端的事，这轮不动」） |
| D7 | 版本 | minor 进位，目标 = 执行时现读 `internal/version/version.go` 的下一档（**不在文档写死数字**；执行结果：用户 2026-10-16 直接设定 **0.20.2 → 0.20.3（patch）**，见计划「执行结果」节） | 惯例；审查后修订（去写死） |
| D8 | 不可达态处理 | 「开关开 + 清单条目为空」放行（fail-open）：HTTP 侧写不出该态（§5 校验拦住），可达通道只有 `Set*` / 直写库（`.scratch` 探针）；不因配置残缺把注册整体锁死；不加日志（`auth.Service` 无 logger 字段，补它要碰 `app.go`——留给日志轮） | 审查后定稿 |

口径夹注：D3 的「开着只会更严、不会更松」是硬性质——最短长度下限 8 与保存校验同值，开关打开不可能放宽到 8 位以下。

## 3. 范围

**改动文件（15 个 + 版本文件，共 16 项）**：

| 文件 | 动作 | 职责 |
| --- | --- | --- |
| `backend/internal/common/apierror/apierror.go` | 改 | 新哨兵 `ErrEmailDomainNotAllowed` / `ErrSettingConflict` + `statusByCode` 两行 |
| `backend/internal/common/apierror/apierror_test.go` | 改 | `TestRegistryMatchesSpec` 两条用例；`TestRegistryIntegrity` 计数 29 → 31 |
| `backend/internal/modules/settings/types.go` | 改 | `Group` 加 `ValidateOn` / `Validate` 字段；新增 `groupValidator`；`Registry` 的 `validators` 表改存 `groupValidator` |
| `backend/internal/modules/settings/registry.go` | 改 | `compileRegistry` 收 `ValidateOn` / `Validate`（成对 + 管辖项存在性把关）；register / auth 两组挂校验器；新增导出 `SplitListValue`（唯一拆分实现） |
| `backend/internal/modules/settings/service.go` | 改 | `PatchGroup` 管辖项门槛 + 合并最终态 + 调校验器 |
| `backend/internal/modules/settings/controller.go` | 改 | `writeSettingError` 识别 `ErrSettingConflict` → 409 + 判定项名 |
| `backend/internal/modules/settings/registry_test.go` | 改 | 校验器纯函数单测 + `SplitListValue` 表 |
| `backend/internal/modules/settings/controller_test.go` | 改 | 两条 PATCH 冲突集成用例 |
| `backend/internal/modules/auth/validate.go` | 改 | `validPassword` 增 `minLength` 参数；`emailDomainAllowed`；`effectivePasswordMinLength`；基线常量 |
| `backend/internal/modules/auth/service.go` | 改 | `Register` 增密码策略与域名检查；`ChangePassword` 用策略；两个读设置的小方法 |
| `backend/internal/modules/auth/validate_test.go` | 改 | 三个纯函数单测（签名变更同步） |
| `backend/internal/modules/auth/register_policy_test.go` | 新增 | 域名拦/放、密码长度（注册 + 改密）集成用例 |
| `docs/superpowers/specs/2026-10-11-neoblog-api-conventions-design.md` | 改 | §7.3 前言 + 两行；§13.2 表 B 两行 |
| `backend/README.md` | 改 | 目录表行 + 「当前清单项」两处 + 新增「怎么加一条组级保存校验」配方段 |
| `docs/DEVLOG.md` | 改 | 顶部倒序条目 |
| `backend/internal/version/version.go` | 改 | 进位脚本代劳（README「当前版本」随动） |

**不动清单**：`frontend/**` 全部；ent schema 与迁移（**零迁移**）；routes（零新路由、零新权限点）；公开端点的行为与负载；`TestBuiltinPublicAllowlist`（期望仍 7 项——四项新消费项仍非公开，白名单不动）；`TestSettingsPublicGetAnonymously`；注册总闸与 2FA/会话既有逻辑。

## 4. 现状核对（已核事实，行号会漂、实施时现读现引）

- 消费方唯一：`auth/service.go:49` 读 `register.enabled`；其测试在 `auth/register_gate_test.go`。
- **tracked 代码零 `Set*` / `SetGroup` / `ResetItem` 调用**（`rg -n "\.SetBool\(|\.SetString\(|\.SetNumber\(|\.SetGroup\(" backend --glob '*.go' | grep -v _test` 无输出）——校验只挂 `PatchGroup` 即覆盖全部 HTTP 写路径；但 `backend/.scratch/` 探针（`regctl` 等，gitignored）**在用 `Set*` 直写**——它是 README「关掉注册的实际操作（暂无界面）」的落点，也是「脏存量 / 不可达态」的真实来源通道（D8、§5 门槛的动机）；测试助手（如 `closeRegistration`）同样走直写、不受校验影响。
- `PatchGroup` 现流程（`settings/service.go:203`）：逐项解码 → `resolveGroupWrites`（类型把关，未出现的项不动）→ `upsertMany` → `GetGroup` 回读。
- 设置端点错误映射（`settings/controller.go:81`）：`ItemError` → 400 + 项名；未知组 → 404；其余按哨兵。
- 前后端密码规则双写死：`auth/validate.go:45-59`（≥8 rune、字母+数字、≤72 字节）与 `frontend/lib/password.ts`（≥8、字母+数字、≤72 字节）；本轮只动后端（D6）。
- 公开白名单现值 7 项（`settings/registry_test.go:140-148`）；`register` 5 项 / `auth` 9 项的形状断言在 `TestBuiltinRegistryShape`。
- 集成用例清理纪律（0.13.0 立）：凡写 `register` / `auth` 组行的用例必须 `t.Cleanup` 复位——`scripts/test.sh` 的 `-p 1` 只能防跨包竞态，残留行照样毒害同库后续用例（如残留 `email_domain_control_enabled=true` 会让所有后续注册吃 400）。

## 5. 机制：设置写前校验插槽

**类型（`settings/types.go`）**：

```go
type Group struct {
	Name       string
	Items      []Definition
	ValidateOn []string
	Validate   func(values map[string]any) error
}

type groupValidator struct {
	items []string
	check func(values map[string]any) error
}

type Registry struct {
	groups     map[string]map[string]Definition
	validators map[string]groupValidator
	order      []string
}
```

**装载（`compileRegistry`）**：`Validate` 与 `ValidateOn` 必须成对；登记时校验两个前置（无管辖项、管辖项不存在都属编程错误，构造即失败）：

```go
			if group.Validate != nil {
				if len(group.ValidateOn) == 0 {
					return Registry{}, fmt.Errorf("settings: group %q has a validator without items", group.Name)
				}
				for _, name := range group.ValidateOn {
					if _, ok := items[name]; !ok {
						return Registry{}, fmt.Errorf("settings: group %q validator references unknown item %q", group.Name, name)
					}
				}
				registry.validators[group.Name] = groupValidator{items: group.ValidateOn, check: group.Validate}
			}
```

`builtinRegistry` 上：register 组 `ValidateOn: []string{"email_domain_control_enabled", "allowed_email_domains"}`、auth 组 `ValidateOn: []string{"password_min_length"}`，`Validate` 分别指向两个校验器。

**调用点（`PatchGroup`，落库前）**：`resolveGroupWrites` 成功后，若该组带校验器**且本次写入与管辖项相交**（`validatorApplies`），先读当前组值（默认值 ⊕ 存量覆盖）再叠上本次写入得到**合并最终态**，交给校验器；校验器返回错误则整笔拒绝（不落库、回读也不发生）。写库仍只写请求里出现的项（部分更新语义不变）。

```go
	values, err := s.resolveGroupWrites(group, writes)
	if err != nil {
		return nil, err
	}
	if validator, ok := s.registry.validators[group]; ok && validatorApplies(validator, writes) {
		merged, err := s.mergedGroupValues(ctx, group, writes)
		if err != nil {
			return nil, err
		}
		if err := validator.check(merged); err != nil {
			return nil, err
		}
	}
	if err := s.repo.upsertMany(ctx, group, values); err != nil {
		return nil, err
	}
	return s.GetGroup(ctx, group)
```

```go
func validatorApplies(validator groupValidator, writes []ItemWrite) bool {
	for _, write := range writes {
		for _, item := range validator.items {
			if write.Name == item {
				return true
			}
		}
	}
	return false
}
```

**校验器（`settings/registry.go`）**：

```go
const (
	passwordMinLengthFloor = 8
	passwordMinLengthCap   = 64
)

func validateRegisterGroup(values map[string]any) error {
	enabled, ok := values["email_domain_control_enabled"].(bool)
	if !ok {
		return fmt.Errorf("settings: email_domain_control_enabled is not a bool")
	}
	if !enabled {
		return nil
	}
	raw, ok := values["allowed_email_domains"].(string)
	if !ok {
		return fmt.Errorf("settings: allowed_email_domains is not a string")
	}
	if len(SplitListValue(raw)) > 0 {
		return nil
	}
	return &ItemError{Item: "email_domain_control_enabled", Err: apierror.ErrSettingConflict}
}

func validateAuthGroup(values map[string]any) error {
	length, ok := values["password_min_length"].(int64)
	if !ok {
		return fmt.Errorf("settings: password_min_length is not a number")
	}
	if length < passwordMinLengthFloor || length > passwordMinLengthCap {
		return &ItemError{Item: "password_min_length", Err: apierror.ErrSettingConflict}
	}
	return nil
}

func SplitListValue(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '，' })
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
```

要点：

- **管辖项门槛**：只有本次 PATCH 写到的项命中 `ValidateOn` 才读合并态并校验——脏存量（如 `Set*` 摆出的「开关开 + 清单空」）不会封锁无关写入（`PATCH {"enabled": false}` 照常 200），auth 组改无关项也省一次组读。这是审查抓到的设计缺口（原设计按整组校验，越界存量会让该组写路径被误锁）。
- 类型断言失败返回内部错误（500）——机制保证合并态类型正确，断言失败即编程错误，响亮失败。
- 判定项是**开关/数值项**：`email_domain_control_enabled`（不是 `allowed_email_domains`）、`password_min_length`；语义是「这项现在的值与其他项冲突」。清空清单（请求写 `allowed_email_domains`）而开关仍开，同样判该组冲突、param 仍指开关项——口径统一、实现简单（替代方案「param 指向请求里被写的那项」被否：多写场景无唯一解）。注意错误信封无 message，param 是唯一信号，而域名规则下要补的其实是 `allowed_email_domains`——前端与文档按此渲染（§7、README 配方段）。
- `SplitListValue` 是「列表型字符串字段」的唯一拆分实现，校验器（判非空）与 auth 消费端（域名匹配）共用，杜绝两处口径漂移。**不**做小写化、**不**剥 `@` 前缀（匹配时用 `EqualFold`）。

**错误映射（`settings/controller.go`）**：

```go
	var itemErr *ItemError
	if errors.As(err, &itemErr) {
		if errors.Is(err, apierror.ErrSettingConflict) {
			apierror.Write(ctx, apierror.WithParam(apierror.ErrSettingConflict, itemErr.Item))
			return
		}
		apierror.Write(ctx, apierror.WithParam(apierror.ErrInvalidRequest, itemErr.Item))
		return
	}
```

（`ItemError.Unwrap` 保证 `errors.Is` 命中；`WithParam` 覆盖 param。）

**零开销声明**：无校验器或未触碰管辖项的 PATCH 不多读、行为逐字不变（site 组全部、register / auth 的无关项写入）；命中的 register / auth PATCH 每次多一条组读（单组 5 / 9 行的小查询，与既有「不做缓存」口径一致）。

## 6. 行为

### 6.1 注册（`auth.Service.Register`）

检查顺序（新增部分加粗）：

1. 读 `register.enabled`（现状）→ 关闭返回 403 `registration_closed`。
2. 规格化 + 语法校验（邮箱 / 昵称）。
3. **密码策略**：读 `auth.password_requirements_enabled`（读失败原样抛）；为真时再读 `auth.password_min_length` → `effectivePasswordMinLength` 夹 [8, 64] → `validPassword(password, minLength)` 不过返回 400 `weak_password`。
4. **域名策略**：读 `register.email_domain_control_enabled`（读失败原样抛）；关闭直接过；开启再读 `register.allowed_email_domains` → `SplitListValue` 解析：条目为空则放行（D8 不可达态），否则邮箱 `@` 后域名不在清单里返回 400 `email_domain_not_allowed`（param `email`）。
5. 查重 → 建号 → 建会话（现状不变）。

顺序说明：语法在前（与现状一致，非法邮箱先吃 `invalid_request`），密码与域名策略在查重之前（策略判定先于数据库唯一性查询）；两条策略内部先密码后域名（锁定口径，测试按此写）。

### 6.2 改密（`auth.Service.ChangePassword`）

`validPassword(newPassword)` → `validPassword(newPassword, s.passwordMinLength(ctx))`；读设置失败原样抛。其余顺序（当前密码确认、新旧相同判定、撤销其他会话）不变。历史密码不重算（只查新设值）。另注新增失败模式：读取设置失败（库不可用类）会让本端点由既有的 401 `invalid_credentials` 变为 503 `service_unavailable`（读失败原样抛的既定口径，接受）。

### 6.3 明确不受影响

`auth.CreateAdminUser`（包级测试/探针入口，暂无 CLI，不走策略）、`PATCH /users/me`（改邮箱不查白名单）、登录、2FA、会话、公开读端点（负载零变化）。

## 7. 接口与错误码

零新路由；两个新码进注册表与 API 规范：

| code | HTTP | type | 默认 param | 场景 |
| --- | --- | --- | --- | --- |
| `email_domain_not_allowed` | 400 | `invalid_request_error` | `email` | 域名控制开启且清单条目非空时，注册邮箱域名不在清单里 |
| `setting_conflict` | 409 | `conflict_error` | null（设置端点覆盖为判定项名） | 设置写入后的最终组合不满足组级约束：开域名控制必须清单非空；`password_min_length` 取 8–64（param=判定项，未必是要改的那一项；域名规则下要补的是 `allowed_email_domains`） |

登记点（实施时现读现引）：

- `apierror.go` 哨兵 + `statusByCode`；`apierror_test.go` 两条用例 + `TestRegistryIntegrity` 计数 `29 → 31`（两处）；哨兵 / 状态表 / 用例表 / §7.3 表行**四处锚点统一插在 `last_owner` 之后**。
- `2026-10-11-neoblog-api-conventions-design.md`：§7.3 前言续写「<执行时版本> 起新增 …」+ 表格两行（400 组插在 `invalid_permission` 后、409 组插在 `last_owner` 后）。**§13.2 表 B 不动**——它是 2026-10-11 冻结快照，历轮新码（`mail_delivery_failed` / `two_factor_*` / `method_not_allowed` / `registration_closed`）均只进 §7.3，本轮沿用惯例。

`setting_conflict` 归 409 的依据：违不违规取决于组内**存量状态**（与 `preset_group_locked` / `group_not_empty` 同类），不是请求体自身格式错误。

## 8. 测试

**单元（不触库）**：

| 位置 | 内容 |
| --- | --- |
| `settings/registry_test.go` | `validateRegisterGroup` 四态（关着放行 / 开着有清单放行 / 开着空串拦 / 开着空白串拦）；`validateAuthGroup` 边界 8 / 16 / 64 过，7 / 0 / -3 / 65 拦；`SplitListValue` 表（空串、纯分隔符、半角/全角、空白条目） |
| `auth/validate_test.go` | `emailDomainAllowed` 表（命中、大小写混写、子域不命中、无 `@` 不命中、多条目）；`validPassword` 参数化（8/9/11/12 与 72 字节上限）；`effectivePasswordMinLength`（关 → 8；开：3→8、7→8、8→8、16→16、64→64、100→64） |

**集成（连库，`scripts/test.sh` 串行）**：

| 位置 | 内容 |
| --- | --- |
| `settings/controller_test.go` | ① owner `PATCH register {"email_domain_control_enabled": true}`（清单空）→ 409 `setting_conflict` + param `email_domain_control_enabled`；补清单 → 200；再开 → 200；`{"allowed_email_domains": ""}`（开关仍开）→ 409 同 param。② owner `PATCH auth {"password_min_length": 3}` → 409；`65` → 409；`16` → 200 且读回 `float64(16)`。③ 管辖项门槛：直写摆出「开关开 + 清单空」后，`PATCH register {"enabled": false}` → 200（无关写入不被脏存量牵连），再 `{"email_domain_control_enabled": false}` → 200。三用例 `resetGroups` 开头清 + `t.Cleanup` 复位。 |
| `auth/register_policy_test.go` | ① 域名控制开、清单 `example.com`：`@gmail.com` → 400 `email_domain_not_allowed` 且 `users` 行数不变；`@Example.COM` → 201（大小写不敏感）。② 密码要求开、长度 10：9 位 → 400 `weak_password`；10 位 → 201。③ 改密：登录后新密码 9 位 → 400 `weak_password`；10 位 → 200。全部用例 `t.Cleanup` 复位四项键（`ResetItem`）。 |

**回归**：形状断言与公开白名单（仍 7 项）、公开读负载不变；既有注册总闸四条用例不受影响（默认配置下策略全关）。

**变异自检（实施时逐条实测红后再改回）**：

| # | 变异 | 预期红 |
| --- | --- | --- |
| ① | `ensureRegistrationDomain` 恒返回 nil | 域名集成用例① |
| ② | `effectivePasswordMinLength` 恒返回 8 | 密码集成用例②③ |
| ③ | `validateRegisterGroup` 恒返回 nil | PATCH 冲突用例① |
| ④ | `effectivePasswordMinLength` 去掉下限夹取（3 直接用 3） | `effectivePasswordMinLength` 单测 |
| ⑤ | `mergedGroupValues` 不读存量（空 map 起步） | PATCH 冲突用例① 的首个 PATCH 即红（校验器缺键 → 500 `internal_error`，而非 409） |
| ⑥ | `compileRegistry` 不登记 `validators` | 两条 PATCH 冲突用例同红（校验从未发生） |
| ⑦ | 管辖项门槛去掉（`validatorApplies` 恒真） | 门槛用例③红（无关写入被 409） |

## 9. 已知缺口与风险

- **前端不一致窗口**：注册页提示与本地校验仍写死 8 位；最短长度被调高时，前端会放行、后端拒绝（用户看到 `weak_password` 文案）。前端轮再跟（涉及把 `password_min_length` 标 Public 或读管理端，未裁定）。新码 `email_domain_not_allowed` 前端未映射 `toFailure`，暂落「注册失败，请稍后重试」通用文案；前端轮补 case + 中英文案。
- **域名控制不管改邮箱**：`PATCH /users/me` 可把邮箱改成任意域名（有意为之，只拦注册；将来要管是单独一轮）。
- **不可达态放行**（D8）：HTTP 写不出；可达通道只有 `Set*` / 探针直写。若将来要让消费端 fail-closed，改一行即可。
- **`password_min_length` 上下限三处出现**：settings 校验（8/64）、auth 夹取（`passwordBaselineMinLength` / `passwordMinLengthMax` = 8/64）、本文口径；本 spec 钉住一致，实施时若引入共享常量属计划外改动（不建议本轮做）。
- **白名单存在性对匿名可见**：`POST /users` 无鉴权，`email_domain_not_allowed` 等于告诉访客「本站在管域名且你的域名不在白名单」（不含清单内容）；可接受口径，登记于此。
- **`setting_conflict` 只覆盖 PATCH**：`Set*` / `SetGroup` / `ResetItem` 直写可摆出违规组合（内部与测试接口，生产零调用）；如果将来这些接口上了 HTTP 或有了生产调用方，校验要挪到更底层。
- **无缓存**：每次注册最多 5 次设置点查（总闸 + 密码开关 + 长度 + 域名开关 + 清单），单管理员博客量级无虞；与总闸轮「不做缓存」口径一致。
- **复杂度 / 邮箱认证 / 会话有效期 / 2FA / 验证码仍零消费方**；注册默认组搁置（值形状未定）。

## 10. 插槽（本轮不定）

| 插槽 | 时点 | 影响面 |
| --- | --- | --- |
| 控制台「用户注册」页接线（读 / PATCH 键、真实组下拉） | 前端轮 | 前端；含把 `setting_conflict` 渲染成具体文案 |
| 注册页提示动态化 | 前端轮 | 前端 + 可能需要 `password_min_length` 公开化（公开面变化需另走批） |
| 注册默认组（值形状「组键 vs 组 ID」、特权组不可选） | 用户点名 | auth + settings + 前端 |
| 密码复杂度语义（可机器执行的规则） | 用户点名 | 设计先行 |
| 域名控制扩到改邮箱 / 后台建号 | 用户点名 | auth |

## 11. 验收标准

1. `gofmt -l` 无输出；`go vet ./...` / `go build ./...` 干净。
2. `set -a && source .env && set +a && bash scripts/test.sh` 全绿（串行；一次只跑一个测试进程；不用 `-race`）。
3. 变异自检七条（§8）逐条实测红后再回绿。
4. 活体（建议）：PATCH 开域名控制（清单空）→ 409 `setting_conflict`；补清单 → 200；注册非白名单域名 → 400 `email_domain_not_allowed`；`password_min_length` 设 3 → 409。
5. 零迁移、零新路由、零新权限点、前端零改动、公开面零变化；README / API 规范 / DEVLOG / apierror 计数 / 版本（执行时现读的下一档）全部登记到位。

## 12. 下一步

本 spec 经审阅后按实施计划 `docs/superpowers/plans/2026-10-16-neoblog-registration-policy.md` 执行；执行方式由用户选（建议 native）。**审阅通过前不动代码、不提交**。v2 已吸收 2026-10-16 独立审查的全部修法（见修订记录）；D5 / D8 / param 三处维持原拟，D3 上限调整为 64。
