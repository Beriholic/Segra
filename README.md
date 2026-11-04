# Segra

Segra 是一个原生多线程分段下载器：Go + [egoist/mygo](https://github.com/egoist/mygo) 的 GPU 原生 UI，
不依赖 WebView；附带一个 Chromium MV3 扩展，可以在浏览器里右键把链接、图片、视频直接交给桌面端下载。

![Segra](docs/screenshot.png)

## 功能

- **分段下载**：HTTP Range 并发抓取（默认 32 个连接，可调 1–64），按 `Content-Range` 规划分片；
- **慢连接自动重连**：借鉴 [Surge](https://github.com/SurgeDM/Surge) 的健康检查——某条连接 3 秒没有新数据、或速度低于平均值的 30% 时丢弃并重开，避免被 CDN 的单连接限速拖住；
  服务器不支持 Range 时自动回退单流下载。
- **断点续传**：下载中的分片状态写入 `.part.segra.json`，校验大小、ETag、Last-Modified 后续传；
  暂停、退出、重启都不会丢进度。
- **队列与并发**：同时下载数可调（1–8），全部暂停 / 全部继续，失败重试。
- **原生界面**：mygo `ui` 包直接绘制，浅色/深色跟随系统；中英双语，可跟随系统或手动切换。
- **浏览器扩展**：右键链接 / 图片 / 视频 / 音频提交；工具栏弹窗提交当前页；
  自动发现端口，Bearer Token 鉴权。
- **桌面集成**：单实例、菜单栏托盘、系统通知、关闭到托盘、开机启动、Dock 角标。
- **本地 API**：只监听 `127.0.0.1`，方便脚本或其他客户端接入。
- **系统代理**：通过 `mygo/netproxy` 跟随系统代理设置。

## 环境要求

- macOS 12+ / Windows 10+ / Linux（GTK）
- Go 1.27+（`go.mod` 固定 `go 1.27.3`，`GOTOOLCHAIN=auto` 会自动下载工具链）
- 纯 Go，无 cgo

## 开发

```bash
# 可选：把构建缓存放进仓库，避免沙箱/权限问题
export GOCACHE="$PWD/.gocache"

go tool mygo dev        # 热重载开发
go test ./...           # 单元测试（含 Range / 续传 / 单流回退）
go tool mygo vet .      # 原生 UI 生命周期检查
```

代码怎么分文件、依赖往哪个方向走，见 [docs/architecture.md](docs/architecture.md)。

## 打包

```bash
go tool mygo build
# macOS: build/darwin-arm64/Segra.app 和 .dmg
# 也可以指定多平台：
go tool mygo build -platform darwin/universal,windows/amd64,linux/amd64

# 如果当前环境不允许 hdiutil 创建磁盘镜像，只出 .app：
go tool mygo build -skip-dmg
```

应用名、标识符、版本、图标、`segra://` URL scheme、i18n 目录都在 `mygo.json` 里配置。

## 安装浏览器扩展

1. 启动 Segra，打开 **设置 → 扩展令牌**，点 **复制令牌**。
2. 打开 `chrome://extensions`（Edge/Arc/Brave 同理），开启 **开发者模式**，点 **加载已解压的扩展程序**，选择本仓库的 `extension/` 目录。
3. 点扩展的 **选项**，填入端口 `47823` 和刚复制的令牌，点 **测试连接**。
4. 在网页上右键链接或图片 → **使用 Segra 下载**；或点工具栏图标提交当前页。
   也可以直接在 Segra 里点 **新建下载** 粘贴链接。

> 端口被占用时 Segra 会从 `47823` 开始向后找最多 10 个端口，扩展会自动发现；
> 也可以在设置里固定端口。

也可以用 `segra://download?url=<编码后的链接>` 深链直接打开新建下载弹窗：

```bash
open "segra://download?url=https%3A%2F%2Fexample.com%2Fbig.iso"
```

## 本地 API

Base URL：`http://127.0.0.1:47823`，除 `/api/health` 外都需要
`Authorization: Bearer <apiToken>`（或 `X-Segra-Token`）。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/health` | 存活检查，无需鉴权 |
| GET | `/api/info` | 版本、下载目录、默认连接数 |
| GET | `/api/tasks` | 任务列表（新→旧） |
| POST | `/api/tasks` | 新建任务 |
| GET | `/api/tasks/{id}` | 单个任务 |
| DELETE | `/api/tasks/{id}?files=1` | 删除任务，`files=1` 同时删文件 |
| POST | `/api/tasks/{id}/pause` | 暂停 |
| POST | `/api/tasks/{id}/resume` | 继续 / 重试 |
| POST | `/api/pause-all` | 全部暂停 |
| POST | `/api/resume-all` | 全部继续 |

新建任务：

```bash
TOKEN=$(python3 - <<'PY'
import json, os
p = os.path.expanduser("~/Library/Application Support/Segra/settings.json")
print(json.load(open(p))["apiToken"])
PY
)

curl -s http://127.0.0.1:47823/api/tasks \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com/big.iso",
    "fileName": "big.iso",
    "dir": "/Users/me/Downloads",
    "connections": 8,
    "referer": "https://example.com/"
  }'
```

字段：`url`（必填，支持 http/https，缺少 scheme 时补 `https://`）、`fileName`、`dir`、
`connections`（1–64，默认 32）、`referer`、`userAgent`、`cookie`。

## 结构

```
app.go                 应用生命周期、窗口/菜单/托盘、设置与任务动作
view.go                原生 UI 视图（列表、详情、设置、新建下载）
main_test.go           ui.NewTester 视图测试
internal/download/     任务模型、Range 探测、分片传输、续传元数据、调度器
internal/store/        JSON 设置与任务持久化（0600、原子写）
internal/server/       127.0.0.1 HTTP API + Token 鉴权
locales/               en / zh-Hans 文案与生成的类型安全函数
extension/             Chromium MV3 扩展（service worker + popup + options）
resources/             应用图标与托盘图标
```

下载文件先写成 `<name>.part`，完成后 `fsync` 并原子重命名；同名文件自动变成
`name (2).ext`。每个任务的分片进度保存在 `<name>.part.segra.json`。

## 说明与限制

- 仅支持 HTTP(S)，不支持 FTP / BitTorrent；鉴权站点请通过 `cookie` 字段或扩展的 referer 提交。
- 续传依赖服务器返回稳定的 `ETag` / `Last-Modified` 和文件大小；不一致会从头开始。
- 令牌保存在 `settings.json`（权限 `0600`）和扩展的 `chrome.storage.local`；
  删除 `settings.json` 里的 `apiToken` 后重启即可轮换，再在扩展里更新。
- macOS 首次写入 `~/Downloads` 等受保护目录时，系统会请求“文件与文件夹”权限；
  未签名的开发二进制可能直接被 TCC 拒绝，打包后的 `.app` 会正常弹出授权。
- 应用只监听回环地址，不对外暴露端口。

## 许可证

[GPL-3.0](LICENSE)
