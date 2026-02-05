## IP 优选（hosts）GUI

基于 Gio 的小工具：输入域名（或从 hosts / 文本文件导入），解析并测速，挑选最优 IP，最后写入系统 hosts（带备份/可恢复）。

## 使用方式

1. 打开程序后在「配置」页输入域名（每行一个），或用按钮导入。
2. 点击顶部「开始」执行测速。
3. 在「结果」页勾选需要写入 hosts 的域名映射。
4. 在「预览」页生成预览并确认后，点击「写入」写入 hosts；如需回滚，点击「恢复备份」。

## 从源码运行

```bash
cd demo/010_ip_opt_gui
go run .
```

### Windows 不弹出 CMD 窗口的构建方式

控制台窗口来自 Windows 子系统类型。构建为 GUI 子系统即可：

```powershell
cd demo/010_ip_opt_gui
.\build_windows_gui.ps1
```

或：

```bat
cd demo\010_ip_opt_gui
build_windows_gui.bat
```

## 自动打包与发布（GitHub Actions）

本仓库提供自动发布工作流：当你在 **release 分支** 上创建并推送 tag 后，会自动构建并打包各平台压缩包，并创建 GitHub Release（自动生成更新日志）。

- tag 命名：`ip-opt-gui/vX.Y.Z`（例如 `ip-opt-gui/v1.0.0`）
- 触发条件：推送上述 tag，且该 tag 指向的提交必须在 `release` 分支上
- 产物：Windows/Linux/macOS 的 amd64/arm64 二进制 + README，打包为 zip / tar.gz 并上传到 Release

### 发版命令

```bash
git checkout release
git pull
git tag ip-opt-gui/v1.0.0
git push origin ip-opt-gui/v1.0.0
```
