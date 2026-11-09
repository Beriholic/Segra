# 代码结构

Segra 是单一 Go 二进制：根目录只留一个 `main.go` 负责启动，桌面端（窗口、视图、
动作）在 `internal/app`，下载引擎、本地 API 和持久化各在自己的 `internal` 包里。
文件按"一件事一个文件"切，新增功能按职责落位。

## 目录

```
.
├── main.go                   入口：嵌入菜单栏图标，调用 app.Run()
├── internal/
│   ├── app/                  桌面端
│   │   ├── run.go            Run：单实例、窗口、托盘、菜单、生命周期
│   │   ├── app.go            App 状态：刷新、事件、通知、持久化
│   │   ├── api.go            本地 API（扩展端口）启停、令牌、深链
│   │   ├── actions.go        界面动作：新建/暂停/继续/移除/打开位置、设置保存
│   │   ├── theme.go          亮/暗两套调色板、外观判定与 mygo 主题（颜色、圆角、滚动条）
│   │   ├── motion.go         动效令牌：时长、弹簧、入场位移
│   │   ├── heat.go           分片热力图的自绘与逐格插值
│   │   ├── dock.go           程序坞图标跟随窗口（macOS 的 accessory 策略）
│   │   ├── widgets.go        无状态控件：胶囊按钮、图标按钮、chip、开关行
│   │   ├── view.go           窗口骨架：侧栏、内容列、标题行、空状态、状态栏
│   │   ├── view_list.go      下载列表的行、行内统计、行内操作、右键菜单
│   │   ├── view_inspector.go 任务详情面板（含折叠动画）
│   │   ├── view_settings.go  设置页
│   │   ├── view_dialog.go    新建下载弹窗
│   │   ├── update.go         检查更新：GitHub release、通知、打开下载页
│   │   ├── view_update.go    更新状态行与版本弹窗
│   │   ├── format.go         纯函数：字节/速度/时长/百分比/状态文案
│   │   ├── icons.go          图标注册与按扩展名选图标
│   │   ├── icons/            Lucide SVG（ISC）
│   │   └── view_test.go      视图与交互测试
│   ├── download/             下载引擎，不依赖 UI
│   │   ├── manager.go            状态、事件、查询
│   │   ├── manager_queue.go      入队、暂停、继续、移除、清理
│   │   ├── manager_run.go        调度循环与任务生命周期
│   │   ├── manager_transfer.go   探测、分片传输、续传元数据
│   │   ├── fetch.go              HTTP Range 探测与单流回退
│   │   ├── meta.go               `.part.segra.json` 读写
│   │   ├── health.go             慢连接健康检查
│   │   ├── filename.go           文件名推断与清洗
│   │   └── task.go               Task / Status 模型
│   ├── server/               127.0.0.1 上的本地 HTTP API
│   ├── update/               GitHub Release 查询与版本比较
│   └── store/                settings.json / tasks.json 的原子读写
├── locales/                  中英文目录与生成的访问器
├── resources/                icon.png（打包图标，mygo.json 引用）与 tray.png（菜单栏）
├── extension/                Chromium MV3 扩展
└── docs/                     截图、架构与动效说明
```

## 分层与依赖方向

```
main.go ──▶ internal/app ──┬─▶ internal/download
                           ├─▶ internal/store
                           ├─▶ internal/server
                           └─▶ internal/update
```

- `internal/download` 不知道 UI 的存在，进度通过 `notify` 回调和 `Event` 上报。
- `internal/server` 只依赖 `download.Manager` 和令牌，不碰窗口状态。
- `internal/update` 只查 GitHub Release 并比较版本，不下载、不安装更新；界面把安装包
  交给浏览器，避免在 ad-hoc 签名的应用上擅自替换自身。
- `internal/app` 单向依赖其它 `internal` 包；`view_*.go` 只读 `App` 状态并调用
  `actions.go`，不直接改引擎内部字段。
- `format.go` 与 `theme.go` 无副作用，可以直接单测；视图一律从 `App.pal` 取色，
  不写字面颜色，换肤只是换一套 `palette`。

## 约定

- 一个文件一件事，超过 ~300 行就该考虑拆；`view.go` 只留窗口骨架。
- 视图函数只做布局；改变状态的逻辑放 `actions.go`。
- 所有面向用户的文案进 `locales/*.json`，改完跑 `go tool mygo generate`。
- 新图标放 `internal/app/icons/`，在 `icons.go` 里注册，不内联 SVG。
- 测试与被测代码同包；引擎行为在 `internal/download/engine_test.go` 覆盖。

## 后续优化

1. 把 `widgets.go` 里不依赖 `App` 的控件抽到 `internal/appui`，视图只负责组合，
   控件可以单独做快照测试。
2. `format.go` 抽到 `internal/format`，补表驱动测试（单位、边界、负数）。
3. 给主要视图加 golden 测试：`ui.NewTester` 渲染固定状态，比对 PNG/文本快照。
4. `App` 拆出 presenter：视图只读 view-model，脱离 mygo 也能测筛选、排序和选中逻辑。
5. 端口、深链、令牌的错误处理收敛到 `api.go`，统一 toast 与日志出口。
6. `fetch.go` 与 `manager_transfer.go` 的 Range 规划可再拆一层（`ranges.go`），
   便于单独测分片边界。
