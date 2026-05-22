# FF14 盛大自动签到工具 (ff14checkin)

[![Go Report Card](https://goreportcard.com/badge/github.com/SakagamiJun/ff14checkin)](https://goreportcard.com/report/github.com/SakagamiJun/ff14checkin)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
![Release](https://img.shields.io/github/v/release/SakagamiJun/ff14checkin)

一个基于 Go 语言编写的自动化工具，用于自动完成盛大（SDO）旗下网页的每日签到任务。完美绕过 EdgeOne WAF 防火墙拦截。

目前支持的签到任务：
*   **石之家 (ff14risingstones)** - 《最终幻想14》官方社区每日签到
*   **趣商城 (qu_sdo)** - 个人中心积分签到

## 🚀 核心特性

*   **智能指纹伪装**：底层采用 `bogdanfinn/tls-client` 模拟真实的 Chrome 124 TLS/JA3 指纹，配合直接的 API 请求。
*   **动态环境刷新**：集成 `chromedp` 无头浏览器。每次运行前自动静默访问页面，刷新并获取最新有效期的前端防爬 Token。
*   **优雅的鉴权自愈**：当盛大通行证的硬性 Session 过期导致登录失效时，程序不会直接崩溃。它会自动挂起并弹出一个可视化的 Chrome 窗口，让你扫码或输入密码登录。登录完成后在终端按下回车，程序会使用 CDP 底层协议 (`storage.GetCookies`) 跨子域提取最完整的全域 Cookie 进行“自愈”，并继续完成签到任务。
*   **多任务支持**：通过 `config.json` 灵活配置多个签到模块。
*   **隐私安全**：提取的所有凭证仅存储在本地配置中，配置文件自动锁定为 `0600` 权限，不包含任何第三方遥测上传。

## 🛠️ 安装与使用

### 前置要求
1.  已安装 **Chrome** 或 Chromium 浏览器（脚本需要调用本地浏览器）。

### 获取程序
你可以直接从 [Releases](../../releases) 页面下载适用于你系统的预编译可执行文件，或者自行编译：

```bash
git clone https://github.com/SakagamiJun/ff14checkin.git
cd ff14checkin
go build -o ff14checkin .
```

### 初始化与运行

1. 首次运行时，在当前目录下确保 `config.json` 不存在或为空，直接运行程序：
   ```bash
   ./ff14checkin
   ```
2. 程序会检测到没有有效的 Cookie，自动为你弹出一个 Chrome 浏览器窗口。
3. 在弹出的窗口中**手动完成登录**。
4. 页面加载完成后，**回到终端窗口按下 `Enter`（回车键）**。
5. 程序会自动抓取全站 Cookie，保存到同目录下的 `config.json` 中（权限会自动设置为 0600），并执行签到。

后续你只需要每天挂机运行 `./ff14checkin` 即可，它将以静默的方式全自动工作，直到你的核心通行证在服务器端强制过期（通常为数十天）。过期时会重复上述流程 2-5。

## ⏱️ 配置自动任务 (macOS 推荐)

如果你使用的是 macOS，可以利用 `launchctl` 配置每天定时执行。

1. 复制项目提供的 `.plist` 模板到用户的 LaunchAgents 目录：
   ```bash
   cp com.sakagami.ff14checkin.plist ~/Library/LaunchAgents/
   ```
2. 修改 `~/Library/LaunchAgents/com.sakagami.ff14checkin.plist` 中的 `ProgramArguments`（你的可执行文件绝对路径）和 `WorkingDirectory`（必须指定为可执行文件所在的文件夹目录）。
3. 加载定时任务（默认每天早上 00:05 运行）：
   ```bash
   launchctl load ~/Library/LaunchAgents/com.sakagami.ff14checkin.plist
   ```
> 运行日志会输出到 `/tmp/ff14checkin.log` 中。如果某天在日志中发现要求重新登录，进入终端手动运行一次 `./ff14checkin` 完成验证即可。

## 🤝 贡献与感谢

感谢开源社区提供的优秀库，特别是 [chromedp](https://github.com/chromedp/chromedp) 和 [tls-client](https://github.com/bogdanfinn/tls-client)。
欢迎提交 Issue 和 Pull Request 来增加更多自动签到模块。

## 📜 协议

本项目基于 [MIT License](LICENSE) 开源。请勿用于商业用途或恶意刷分行为，任何滥用导致的封号等后果由使用者自行承担。