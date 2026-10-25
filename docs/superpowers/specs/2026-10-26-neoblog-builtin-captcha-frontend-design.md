# 自研人机验证前端接入（登录 / 注册滑块弹窗）设计

日期：2026-10-26
状态：待审阅
前序：2026-10-26 自研人机验证后端（`captcha_provider=builtin`，出题 / 交卷 / 一次性凭证已入库，0.24.0，未推送）
样式对照：`D:\protect\NextJs_Blog`（hao 博客）的自研滑块——`components/gateway/captcha/selfhosted-widget.tsx`、`components/gateway/captcha-verify-dialog.tsx`、`components/gateway/captcha-submit-gate.ts`

## 0. 一句话目标

把后端已经能用的 `builtin` 滑块接到登录 / 注册页：公开设置命中 `builtin` 时，提交表单先弹出滑块验证；验证通过拿到的一次性 `captcha_token` 原样带回现有登录 / 注册请求。界面、状态和动效按 hao 博客的滑块组件一比一落，协议按本仓后端（只验最终落点）适配。

## 1. 范围

### 1.1 本轮做

- `CaptchaConfig` 扩成 `turnstile | builtin` 判别联合；`getCaptchaConfig` 按 provider 返回对应配置，`builtin` 携带 `scope`。
- 新增自研滑块组件（图区 + 拼图块 + 滑轨 + 六种状态），样式取自 hao 博客 `selfhosted-widget.tsx`。
- 新增出题 / 交卷的前端传输层与登录 / 注册共用的「提交前预取题目」闸门 hook。
- `CaptchaDialog` 按 provider 分派：Turnstile 仍渲染原组件，builtin 渲染新滑块。
- 登录 / 注册表单接入闸门：提交时先预取题目再开弹窗；答错 / 过期必须换新题。
- 控制台「人机验证」设置页：作用域选择行对 `builtin` 也显示（当前只在 Turnstile 下显示，选了自研就配不了作用域）。
- 中英文案、DEVLOG、`backend/README.md` 过时的「前端未接 builtin」说明。
- 样式逐项照抄：弹窗 `sm:max-w-[340px]` / `p-5` / `gap-3`、滑轨 `h-10` + 40px 拇指、加载 / 校验中无文案与转圈（加载态不渲染占位）、失败占位、成功绿色条 + 耗时、失败红色条 + 弹窗摆动、800ms 后重试。

### 1.2 明确不做

- 后端任何代码、接口、迁移、设置键、错误码改动。
- 管理端自检接口的 builtin 形态（后端已知缺口，本轮不加按钮）。
- 拖动轨迹采集 / 轨迹判定：本仓后端只收最终 `x/y`，协议里没有 track 字段。
- 参考项目的 SVG 现场出题、`sharp` / WebP、前端生成答案；图片全部来自后端 data URL。
- 键盘操作、语音验证、记住验证结果、跨入口复用凭证。
- 新依赖、新路由、新前端测试框架。
- 版本进位：纯前端轮，`backend/internal/version` 保持 `0.24.0`（对齐既有「纯前端不单独进位」口径）。

## 2. 生效条件与整体数据流

登录 / 注册页面仍由服务端读 `GET /api/v1/public/settings`，`getCaptchaConfig(settings, scope)` 决定是否把 `captcha` 配置传给表单。命中条件与后端一致：

1. `captcha.captcha_enabled = true`；
2. `captcha.captcha_provider` 是 `turnstile` 或 `builtin`；
3. 当前页面的 scope（`login` / `register`）出现在 `captcha.captcha_scopes` 里；
4. Turnstile 还要求 `turnstile_site_key` 非空；builtin 无额外密钥条件。

数据流（builtin）：

```
提交表单
  → useCaptchaGate.begin()
  → POST /api/v1/captcha/challenges {scope}
  → 201 {id, master_image, tile_image, master_*, tile_*, tile_x, tile_y, expires_at}
  → 打开 CaptchaDialog，把 challenge 交给 BuiltinCaptcha
  → 拖动拼图块，松手
  → POST /api/v1/captcha/challenges/:id/solutions {x, y}
  → 200 {captcha_token, expires_at}
  → onSolved(token)（沿用现有 1618ms 成功停留）
  → submit(token) → POST /sessions 或 POST /users 带上 captcha_token
```

Turnstile 数据流不变：点提交直接开弹窗，组件拿 token 后走同一个 `onSolved` 延迟与 `submit(token)`。

## 3. 协议映射

### 3.1 出题

`POST /api/v1/captcha/challenges`，请求体 `{"scope": "login" | "register"}`。

前端类型（传输层映射成 camelCase）：

```ts
export type CaptchaChallenge = {
  id: string;
  masterImage: string;
  tileImage: string;
  masterWidth: number;
  masterHeight: number;
  tileWidth: number;
  tileHeight: number;
  tileX: number;
  tileY: number;
  expiresAt: string;
};
```

- `master_image` / `tile_image` 是 data URL，直接进 `<img src>`；不转码、不落 localStorage。
- `tile_x` / `tile_y` 是拼图块在主图上的初始显示位置；`tile_width` / `tile_height` 是拼图块尺寸。
- 响应字段缺失或类型不对 → 传输层抛错，界面进入「验证码加载失败」卡，不做默认值兜底。

### 3.2 交卷

`POST /api/v1/captcha/challenges/:id/solutions`，请求体 `{"x": <int>, "y": <int>}`。

- `x` / `y` 是拼图块左上角在主图坐标系里的最终位置，必须是整数。
- 成功 200 返回 `{captcha_token, expires_at}`；前端只取非空字符串 `captcha_token`，形状不对视为失败。
- 题目不存在 / 过期 / 已判负 / 答案错一律 400 `captcha_failed`；前端统一按「答错」处理。

### 3.3 前端要识别的错误

| 场景 | 识别方式 | 界面行为 |
| --- | --- | --- |
| 出题 429 | `ApiError.status === 429` | sonner toast「人机验证请求过于频繁，请稍后再试」，不开弹窗 |
| 出题 403 `captcha_disabled` | 其它非 2xx | 开弹窗；组件挂载时按对照项目再试一次，仍失败则落到加载失败卡 + 重新加载 |
| 出题 5xx / 网络 / 坏响应 | 其它错误 | 同 403：开弹窗 → 组件自动重试一次 → 仍失败显示加载失败卡 |
| 交卷 400 `captcha_failed` | 任意非 2x | 红色失败条 + 弹窗摆动 → 800ms 后重新出题 |
| 交卷 5xx / 网络 | 任意错误 | 同上；重试出题若也失败则落到加载失败卡 |
| 登录 / 注册 400 `captcha_failed` | 既有 `toMessage` | 保持现有 toast「人机验证未通过，请重试」 |

## 4. 与对照项目的样式 / 交互逐项映射

参照文件：`D:\protect\NextJs_Blog\components\gateway\captcha\selfhosted-widget.tsx` 与 `captcha-verify-dialog.tsx`。

| 元素 | 对照项目做法 | 本轮实现 |
| --- | --- | --- |
| 弹窗外壳 | `sm:max-w-[340px]`、`gap-3 p-5`、无 footer、点外部不关、阻止自动聚焦 | `DialogContent` 加 `className="gap-3 p-5 sm:max-w-[340px]"`，保留 `blockOpenAutoFocus` 与 `disablePointerDismissal`；标题仍为「人机验证」 |
| 阶段机 | `loading / ready / dragging / verifying / success / failed` | 同名六阶段 |
| 图区 | `relative w-full overflow-hidden bg-muted/20` + 320×160 宽高比 | 同结构，宽高比换成后端返回的 `masterWidth / masterHeight`（当前 300×220） |
| 主图 | `<img>` 填满、`object-cover`、`draggable={false}` | 相同 |
| 拼图块 | 绝对定位、初值 x=0/y=pieceY、随滑轨位移、`drop-shadow-md`、成功 / 失败 ring | 相同，但初值用后端 `tileX / tileY`（见 §5.2） |
| 滑轨 | `h-10`、`rounded-sm`、`border-border`、`bg-muted/30`、`touch-none`、`select-none` | 相同 |
| 拇指 | 40×40、`rounded-sm`、`border-border`、`bg-card`、`shadow-sm`；成功绿 / 失败红 | 相同；图标 `ArrowRight / Loader2 / Check / X` |
| 进度填充 | 左侧填充，宽度 `slidePx + min(拼图块宽, 40)`，成功变绿 | 相同 |
| 提示文案 | `slidePx === 0 && !locked` 时居中「拖动滑块完成拼图」 | 相同 |
| 验证中 | 半透明黑遮罩 + 转圈 +「校验中…」 | 不渲染文案 / 转圈 / 遮罩（用户 2026-10-26 点名移除）；仅锁定滑轨等待结果 |
| 成功 | 全图绿色蒙层 + 底部绿条 `Check` +「验证成功，用时 x.xs」+ 拼图块绿 ring | 相同；耗时从本次按下到交卷成功本地计时，`max(0.1, round(elapsed/100)/10)` |
| 失败 | 底部红条 `X` +「人机验证未通过」+ 弹窗快速缩放摆动 | 相同；摆动工具新增 `lib/dialog-nudge.ts`（关键帧、时长 140ms、`prefers-reduced-motion` 时不动） |
| 加载 / 失败占位 | 150px 高卡片，加载卡有转圈 +「加载验证码…」，失败卡红边框 +「重新加载」按钮 | 加载态按用户 2026-10-26 点名**整块移除**（不渲染占位）；失败卡保持同样样式与图标，高度按 300×220 预留（见 §6.4） |
| 答错重试 | 800ms 后重试；400 保留原题，其余换题 | 800ms 后**一律换新题**（本仓后端答错即判负，原题不可再用） |
| 出题前置 | 提交时先预取题目，成功才开弹窗；429 只 toast 不开窗 | 相同，抽进 `useCaptchaGate` |
| 成功停留 | 闸门延迟 1618ms 再提交 | 沿用现有 `CaptchaDialog` 的 1618ms，不新增第二层延迟 |
| 图片内容 | 前端 SVG 程序化纹理 | 后端 go-captcha 官方照片素材（样式骨架相同，图片内容不同） |

### 4.1 必要的协议偏离

1. **不采集轨迹**：没有 `slider-track`，不维护点数组；松手只算最终坐标。
2. **拼图块起点不是 0**：用后端 `tile_x / tile_y`，滑轨拇指仍从 0 起。这样拼图块与主图缺口在同一坐标系里能真正对齐；若强行让拼图块从 0 起，视觉对齐位置会与提交坐标错开。
3. **答错必定换题**：不能照搬对照项目的「400 原地复位」，后端失败交卷已把题目置为判负。
4. **占位高度**：对照项目固定 150px，本仓的失败卡按后端画布宽高比预留，避免重试成功后弹窗跳高；加载态与校验中不显示任何文案 / 转圈 / 遮罩（用户 2026-10-26 点名），加载态不渲染占位。

## 5. 代码结构与职责

### 5.1 新增

| 文件 | 职责 |
| --- | --- |
| `frontend/lib/captcha-challenge.ts` | 传输层：`requestCaptchaChallenge(scope, signal?)`、`solveCaptchaChallenge(id, x, y, signal?)`、`CaptchaChallenge` 类型；响应形状校验；只抛错，不吞错、不兜底 |
| `frontend/hooks/use-captcha-gate.ts` | 逻辑层：按 provider 决定「直接开窗」还是「先出题再开窗」；429 toast；关闭 / 卸载时 abort；向表单暴露 `isOpen / challenge / begin / cancel / handleOpenChange` |
| `frontend/components/auth/builtin-captcha.tsx` | 自研滑块组件：六阶段、拖拽、坐标换算、交卷、失败重试、摆动、成功耗时；只通过 props 拿 `scope / initialChallenge / onToken` |
| `frontend/lib/dialog-nudge.ts` | 弹窗快速缩放摆动（照抄对照项目的 `quick-scale` 预设） |

### 5.2 修改

| 文件 | 改动 |
| --- | --- |
| `frontend/lib/api.ts` | `RequestOptions` 增加可选 `signal`，透传给 `fetch`；其余行为不变 |
| `frontend/lib/public-settings.ts` | `CaptchaConfig` 改为判别联合：`{provider:"turnstile"; siteKey:string}` 或 `{provider:"builtin"; scope:CaptchaScope}`；`getCaptchaConfig` 按 provider 返回，builtin 不再要求 siteKey |
| `frontend/components/auth/captcha-dialog.tsx` | props 从 `siteKey` 改为 `config: CaptchaConfig`；新增可选 `challenge`；按 provider 渲染 `TurnstileCaptcha` 或 `BuiltinCaptcha`；弹窗尺寸对齐对照项目；用户主动关闭时清掉 1618ms 定时器 |
| `frontend/app/login/signin-form.tsx` | 用 `useCaptchaGate(captcha)`；`onCaptchaRequired` 改接 `begin`；关闭 / 成功时走 `cancel`；弹窗传 config 与 challenge |
| `frontend/app/register/signup-form.tsx` | 同上 |
| `frontend/app/console/settings/captcha/captcha-settings-form.tsx` | 作用域行从「仅 Turnstile」改为「Turnstile 或 builtin」都显示；Turnstile 的 site/secret key 与「验证配置」按钮仍只在 Turnstile 下出现；自检弹窗改传 `{provider:"turnstile", siteKey}` |
| `frontend/messages/zh-CN.json` / `en.json` | 新增滑块文案（见 §8） |

### 5.3 组件契约

```ts
export type CaptchaConfig =
  | { provider: "turnstile"; siteKey: string }
  | { provider: "builtin"; scope: CaptchaScope };

export function useCaptchaGate(config: CaptchaConfig | null): {
  isOpen: boolean;
  challenge: CaptchaChallenge | null;
  begin: () => void;
  cancel: () => void;
  handleOpenChange: (open: boolean) => void;
};

export function BuiltinCaptcha(props: {
  scope: CaptchaScope;
  initialChallenge: CaptchaChallenge | null;
  onToken: (token: string) => void;
}): React.ReactElement;
```

`CaptchaDialog`：

```ts
{
  open: boolean;
  config: CaptchaConfig;
  challenge?: CaptchaChallenge | null;
  onOpenChange: (open: boolean) => void;
  onSolved: (token: string) => void;
}
```

## 6. 交互细节

### 6.1 预取与开窗

- 点提交且表单字段通过现有校验后，`useSigninForm` / `useSignupForm` 发现需要 captcha 且没有 token，调用 `onCaptchaRequired` → `gate.begin()`。
- **Turnstile**：`begin()` 立即 `setIsOpen(true)`，与今天完全一致。
- **builtin**：先请求出题；成功后才开窗，弹窗直接显示题目，不闪加载态；429 只 toast；其它失败也开窗，`challenge = null`。组件挂载时发现没有初始题目，会像对照项目一样自动再请求一次：成功则直接显示题目，仍失败（含 429）才显示「验证码加载失败 / 重新加载」。
- 预取期间重复点提交只发一次请求；弹窗已打开时 `begin()` 直接返回，不再出题；按钮不加 spinner、不改文案（与对照项目一致）。
- 关闭弹窗（X / 主动取消）会 abort 未完成的预取并清空 challenge；卸载时同样 abort。
- 成功停留 1618ms 由现有 `CaptchaDialog` 负责；`onSolved` 触发后表单先 `gate.cancel()` 再 `submit(token)`，不产生第二层延迟。

### 6.2 拖拽坐标

```ts
const scale = renderedWidth / challenge.masterWidth;
const maxDrag = Math.max(
  0,
  (challenge.masterWidth - challenge.tileWidth - challenge.tileX) * scale,
);
const submitX = Math.round(challenge.tileX + slidePx / scale);
const submitY = challenge.tileY;
```

- 拼图块渲染位置：`left = tileX * scale + slidePx`、`top = tileY * scale`、宽高按 `tileWidth / tileHeight` 乘 `scale`。
- 拖动范围 `0 … maxDrag`；未量到宽度前不允许拖动（`scale` 不能是 0）。
- 横向拖动即可；后端在 ModeBasic 下 `answer_y === tile_y`，纵向始终提交 `tile_y`。
- 容差由后端 ±4 主图像素决定；前端只做四舍五入，不自行放宽容差。

### 6.3 交卷与重试

- `pointerup` / `pointercancel` 结束拖动 → `verifying` → 调交卷接口。
- 成功 → 记录耗时、`phase=success`、`onToken(token)`；同一题只成功一次。
- 失败 → `phase=failed`、弹窗摆动、底部红条；800ms 后**重新出题**（`slidePx=0`，新图新题），若出题 429 则 toast 并显示加载失败卡。
- 题目 TTL 3 分钟：用户在弹窗停留超时后交卷会收到 400，按失败流程换题。
- 卸载 / 关闭时清掉 800ms 定时器并 abort 在途请求；卸载后到达的响应丢弃。

### 6.4 占位高度

- 图区按后端返回的 `masterWidth / masterHeight` 设置 `aspect-ratio`。
- 失败卡使用常量 `DEFAULT_CAPTCHA_CANVAS = {width: 300, height: 220}`（后端库默认画布）设置同样的宽高比，保证重试成功后弹窗不跳高；加载态不渲染占位（弹窗先预取题目，正常路径不会出现加载态）。
- 该常量只用于占位；实际尺寸始终以后端字段为准。若后端画布调整，常量同步更新。

## 7. 设置页

- 作用域下拉行显示条件：`captchaProvider === "turnstile" || captchaProvider === "builtin"`；`none` 与未知值维持现状。
- Turnstile 的站点密钥、私钥、验证配置按钮仍只在 `turnstile` 下出现。
- builtin 没有管理端自检端点，因此不新增「验证配置」按钮；保存后直接生效。
- 设置写接口、权限、表单字段零改动。

## 8. 文案

新增到 `auth` 命名空间（两份语言包同键）：

| key | zh-CN | en |
| --- | --- | --- |
| `captchaImageAlt` | 拼图验证图片 | Puzzle verification image |
| `captchaSliderHint` | 拖动滑块完成拼图 | Drag the slider to complete the puzzle |
| `captchaLoadFailed` | 验证码加载失败 | The puzzle failed to load |
| `captchaReload` | 重新加载 | Reload |
| `captchaSuccess` | 验证成功，用时 {seconds}s | Verified in {seconds}s |
| `captchaError` | 人机验证未通过 | Human verification failed |
| `captchaRateLimited` | 人机验证请求过于频繁，请稍后再试 | Too many human verification requests. Try again later. |

现有 `captchaDialogTitle` / `captchaUnavailable` 保留；登录 / 注册各自的 `captchaFailed`（服务端错误文案）不动。

## 9. 不变量

- **I1 Turnstile 行为不变**：loader、signed token、`onUnavailable`、自检接口、登录 / 注册提交路径逐条保持；仅弹窗宽度 / 内边距对齐对照项目。
- **I2 只按公开设置生效**：前端不写死 provider；`captcha_enabled` / provider / scope 任一不满足就不渲染滑块。
- **I3 token 来源唯一**：只有交卷 200 返回的非空 `captcha_token` 会进入登录 / 注册；不构造、不复用、不缓存到 storage。
- **I4 一题一次**：同一 challenge 只交一次；答错、过期、网络失败后必须重新出题，绝不重复提交旧题。
- **I5 坐标在后端坐标系**：提交值以主图像素为单位，公式见 §6.2；容差只由后端决定。
- **I6 失败不降级**：出题失败显示失败卡，交卷失败显示失败条并换题；不自动提交、不伪造 token、不放行。
- **I7 无新依赖**：复用 shadcn / base-ui、sonner、lucide、next-intl；不引入测试框架、图片库、track 库。
- **I8 无布局跳动**：占位按最终宽高比预留，弹窗高度不由异步加载改变。
- **I9 单一在途请求**：预取与交卷各有 abort 控制；卸载 / 关闭后不再 setState。
- **I10 后端零改动**：本轮不碰 `backend/` 生产代码、迁移、接口和设置键。

## 10. 决策

| 编号 | 决策 | 理由 |
| --- | --- | --- |
| D1 | 样式 / 状态 / 动效按 hao 博客滑块一比一落 | 用户明确要求「样式和对照项目一样」 |
| D2 | 弹窗也按对照项目的 340px / p-5 / gap-3 | 滑块图区宽度与观感由弹窗决定，只改组件不改外壳会对不上 |
| D3 | builtin 先预取题目再开窗 | 对照项目同款，避免弹窗内闪加载态 |
| D4 | 答错后一律换新题 | 本仓后端交卷失败即判负，原题不可重试 |
| D5 | 不搬轨迹采集 | 后端协议只收最终 x/y；采集了也没有消费方 |
| D6 | 拼图块从 `tile_x / tile_y` 起 | 与主图缺口同坐标系，视觉和判分一致 |
| D7 | 占位用 300×220 宽高比预留 | 遵守「异步控件预留最终高度」；对照项目 150px 会在加载后跳高 |
| D8 | 设置页作用域行对 builtin 显示 | 否则选 builtin 后无法配置生效入口，属于接线必需项 |
| D9 | 不加 builtin 自检按钮 | 后端没有对应端点；不伪造一个前端本地校验 |
| D10 | 纯前端不升版本 | 对齐既有 DEVLOG 口径 |
| D11 | 加载态不渲染占位；校验中不显示文案 / 转圈 / 遮罩 | 用户 2026-10-26 点名整块移除加载占位与 `captchaVerifying`；沿用全站移除加载文案 / 转圈的口径 |

## 11. 测试与验证

前端没有测试框架，本轮不新增；验证分三层。

### 11.1 静态门禁

- `cd frontend && npx tsc --noEmit`
- `cd frontend && npm run lint`（0 error；既有 warning 不算本轮引入）
- `cd frontend && npm run format:check`
- `git diff --check`

### 11.2 浏览器实测（dev server + 后端）

准备：用测试库启动后端并临时把 `captcha` 组设为 `enabled=true / provider=builtin / scopes=login`，前端 `BACKEND_URL` 指向该后端；准备一个可登录的测试账号。验证只读测试库的题目答案用于拖拽定位。

逐项：

1. `/login` 打开后提交空 / 错字段：不发出题请求（现有字段校验先拦）。
2. 合法字段提交：只发一次出题请求；弹窗打开时题目已就绪，无加载闪烁。
3. 拖动滑块到正确 `answer_x`：成功绿条与耗时出现，1618ms 后弹窗关闭，登录请求带 `captcha_token`，登录成功。
4. 拖动到错误位置：红条 + 弹窗摆动；800ms 后自动换新题；期间不发登录请求。
5. 成功停留期间点 X：不提交登录请求（清理定时器）。
6. 预取阶段连续点提交：只发一次出题请求。
7. 后端停掉 / 出题返回错误：弹窗显示失败卡与「重新加载」；恢复后按钮能重新出题。
8. 出题限流：用同一 IP 触发 429，只弹 toast，不打开弹窗。
9. `/register` 同样走一遍（若测试库注册关闭则临时开启，测完还原）。
10. 设置页选 builtin：作用域行可见且能保存；选 turnstile：密钥行与验证按钮出现；两种 provider 下弹窗宽度与滑块外观一致。
11. Turnstile 回归：切回 turnstile，登录仍弹原组件、token 提交成功。
12. 样式对照：对 `sm:max-w-[340px]`、滑轨 `h-10` / 拇指 40px、六种状态逐一与对照项目源码比对；截图供用户确认。

### 11.3 不可在本机完成的部分

- 生产环境 CDN / 代理下的真实 IP 限流体验（后端既有问题，不在本轮）。
- 多浏览器 / 真机触屏：至少用浏览器 pointer 事件模拟一次；真机交用户。
- 公开设置 60 秒缓存下的 provider 切换窗口：只记录行为，不做修复。

## 12. 文档与版本

- `docs/DEVLOG.md`：新增 2026-10-26 前端滑块轮条目（动机、范围、决策、验证、已知缺口、未提交状态）。
- `backend/README.md`：把「前端尚未接 builtin，切 provider 前必须先上前端滑块组件」改成「前端已接 builtin（登录 / 注册弹窗按 provider 渲染）；切换 provider 后公开设置最多缓存 60 秒，期间可能仍下发旧 provider」。API 表与设置章节其余内容不动。
- 版本：保持 `0.24.0`，不单独进位。

## 13. 已接受的代价与风险

- **预取期间无按钮反馈**：与对照项目一致；后端慢时点击提交会短暂「没反应」，重复点击已做单请求保护。
- **滑块画布比对照项目高**：后端默认 300×220，对照项目 320×160；骨架与控件尺寸一致，弹窗整体更高。
- **设置页滑块与 Turnstile 共用 340px 弹窗**：Turnstile 组件宽度随之变化，属样式对齐的预期影响。
- **每次重试重新下载约 62KB 图片**：后端既有出题成本；不在前端做缓存，避免旧题复用。
- **provider 切换缓存窗口**：切换后最多 60 秒内旧配置仍可能生效，期间用户可能拿到旧 provider 的验证结果并收到 `captcha_failed`。

## 14. 已知缺口（本轮不做）

- 管理端自检接口的 builtin 形态与设置页「验证配置」按钮。
- 键盘 / 读屏操作滑块（对照项目也没有；`role="slider"` 仅提供语义）。
- 前端自动化测试（仓库无测试运行器）。
- 六种状态之外的网络异常细分 UI（统一走失败卡 / 失败条）。
- 真实访客 IP 下的出题限流精度（后端部署口径问题）。
