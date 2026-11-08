# 动效

界面是"安静纸张"：暖白纸面、近白卡片、一个墨绿。动效也照这个性子来——短、轻、
有物理感，只在下载落地的那一刻允许一点庆祝。所有时长与弹簧集中在
`internal/app/motion.go`，改节奏只改那一处。

## 三档动效，成本差一个数量级

| 档位 | API | 代价 |
| --- | --- | --- |
| 值动画 | `Animate`、`AnimateTo`、`AnimateColor`、`Transition`、`Keyframes`、`Timeline` | 每帧重建视图 |
| 绘制动画 | `AnimatePose`、`ContentTransition`、`Draw` + `Painter.AnimationFrame` | 只重绘，不跑 view |
| 系统动画 | `Toast` 自带淡入淡出、`NewFlyout(Popover: true)` 的 NSPopover | 免费 |

下载中引擎每 200ms 就 `Window.Update` 一次，所以**值动画在下载过程中几乎不额外花钱**；
真正要省的是空闲时的高频交互——悬停、指针反馈一律用绘制档。

## 令牌

| 名称 | 值 | 用在 |
| --- | --- | --- |
| `motionFast` | 120ms | 颜色：悬停、chip、状态点 |
| `motionBase` | 180ms | 小折叠：弹窗、侧栏高亮淡入 |
| `motionSlow` | 260ms | 布局：面板、页面 |
| `springSoft` | 340ms，无回弹 | 布局位移：行进出、面板折叠、页面切换 |
| `springSnap` | `ui.Snappy` | 指针反馈、侧栏高亮滑动 |
| `fadeCurve` | 180ms `EaseOut` | 由值驱动的淡入淡出 |
| `freshFor` | 1.5s | 新下载算"新"的窗口 |
| `finishFor` | 900ms | 下载落地的高亮时长 |

`fadeIn(y)` 是**只位移不淡入**的入场：第一帧文字就可读，列表滚回来时也不会闪白。

## 各处动效

- **行卡**：悬停时 `AnimatePose` 抬起 1px、放大 1.004（纯绘制）；背景与边框用
  `Transition{Colors}` 淡入。选中行底色 `colCardSel`，左侧绿条常驻树中、用 pose 的
  `Fade` 显隐，不闪烁。
- **行进出**：新下载（`freshFor` 内）入场从下方 14px 滑入；移除即时重排，其余行
  用同一弹簧补位。列表是虚拟化的，所以入场用"新下载"时间戳门控，滚动不会触发；
  也不用 `Exit`——滚出可视范围的行同样会被卸载，退出副本会在边缘乱飞。
- **完成瞬间**：`drawLanded` 在行上描一圈淡绿，900ms 内淡出，`AnimationFrame`
  只驱动重绘。
- **侧栏**：整组只画一个高亮块，由 `navGroup.Draw` 画在条目**下面**（自带 Box 会盖住
  文字）；位置用 `AnimateTo` 弹簧滑动，条目颜色用 `AnimateColor` 过渡。
- **页面切换**：内容列没有自己的底色，两页交叉淡入会互相透出、标题重影，所以
  切页是**即时替换**；变化感由侧栏高亮的滑动承担（高亮 180ms 缓出）。
- **详情面板**：宽度用 `springSoft` 折叠；面板体 `Enter` 滑入；换任务时整块按
  `t.ID` 直接替换、新内容上浮 8px（不做交叉淡入：面板没有自己的底色，两层文字
  会同时可见）；进度条 `AnimateTo` 滑到当前比例。
- **分片热力图**：不再是一格一个 Box（详情 96 个、面板 3 行 288 个），改为
  `Draw` 画一格一个圆角矩形，按 `heatFade` 从上一帧的状态混色到当前状态，
  `AnimationFrame` 只重绘。绘制时按可用宽高取格子尺寸，自动居中。
- **速度曲线**：新样本到达后 500ms 内从旧曲线流到新曲线（按右端对齐），
  用 `p.Now()` 计时，窗口遮挡后恢复不会重播。
- **新建下载**：Modal 的面板当帧新建，`Enter` 没有父级可落，所以用 `Timeline`
  （`Run: a.newRun`）做 0.97→1 的缩放和 0.6→1 的淡入。
- **托盘面板**：飞窗瞬间出现，内容用 `Timeline` 从下方 5px 滑入、0.7→1 淡入。
- **按钮**：`pressPose` 按压 0.97、悬停 1.012（pose），颜色 `Transition{Colors}`。
- **数字**：直接替换，不做滚动或淡入。速度、百分比这类数字每 200ms 变一次，
  任何过渡都会和下一次上报叠在一起，反而显得抖；让它们干脆地跳到新值。

## 红线

1. 悬停、滚动、常驻动画只用绘制档；一次性、低频才用值动画。
2. 不在视图里起 goroutine，不每帧新建动画状态；动画 key 必须稳定。
3. 动画停下必须真的静止：`Transitioning()` / `Settled()` 之后不再请求帧，空闲 CPU 为 0。
4. 每帧只做该做的事：dock 角标只在数量变化时 `SetBadgeCount`（曾经每帧都调）。
5. `ReduceMotion` 由 mygo 对值动画、pose、Transition、Timeline 自动降级；
   自绘动画（`Draw`）若做位移类效果要自己读 `c.Preferences().ReduceMotion`。
6. 窗口被遮挡时 mygo 自动停帧；自绘动画用 `c.Now()` 计时，不要数帧。
7. 入场不要从 `Opacity: 0` 起步，否则首帧文字不可见，测试的 `HasText` 也会失手。

## 验证

- 纯函数单测：`motion_test.go` 覆盖热力图插值、曲线流、新下载门控与清理。
- `ui.NewTester` + `tt.Advance` 可推进动画时钟，检查帧不再请求（`FramePending`）。
- `go tool mygo vet .`、`CGO_ENABLED=0 go test ./...`。
- 手动清单：空闲 0 帧、下载中 60fps、悬停顺滑、系统"减弱动态"生效、
  窗口遮挡再恢复、虚拟列表滚动不触发行淡入。
