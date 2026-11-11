# ictrek.app

GARM 的 VOS 交付定义。

## 目录

| 路径 | 用途 |
| --- | --- |
| `src/` | 安装包内容源，含 manifest、Compose、配置、路由与文档 |
| `src/garm/base-path` | 应用公共路径前缀，构建与打包共用 |
| `src/garm/config.toml.tpl` | 运行配置模板，由入口脚本渲染 |
| `scripts/version.sh` | 生成版本串与镜像 tag |
| `scripts/git-sha.sh` | 生成合法 SemVer 的短 SHA |
| `scripts/image-ref.sh` | 生成本地镜像引用，带架构前缀 |
| `scripts/build_image.sh` | 在对应架构的构建机上构建并推送单架构镜像 |
| `scripts/package.sh` | 生成 pull 安装包 |
| `scripts/feishu_sheet.py` | 把镜像 tag 写入 VOS 发布表 |
| `scripts/release.sh` | 本地发布入口 |
| `dist/` | 构建产物，不提交 |

## 版本号

```text
<上游版本>-<DATE>.<N>.<GITSHA>
```

示例 `0.2.1-20261109.1.158fe569`。

- `<上游版本>`：HEAD 上最近的稳定上游 tag，去掉 `v`。
- `<DATE>`：`Asia/Shanghai` 的 `YYYYMMDD`。
- `<N>`：远端同名日期 tag 的最大序号加一。
- `<GITSHA>`：8 位起，遇前导零加长。

## 构建和发布

在本地选定已提交的版本，并将源码同步到 AMD64 和 ARM64 构建机。然后分别运行：

```bash
./ictrek.app/scripts/build_image.sh --target amd --version <版本>
./ictrek.app/scripts/build_image.sh --target arm --version <版本>
```

镜像分别使用 `amd_v<版本>` 和 `arm_v<版本>` tag，tag 内的连字符替换为下划线。脚本在推送前检查镜像架构和容器入口。推送成功后将 tag 分别写入飞书发布表 `AMD_with_cuda` 和 `ARM_without_cuda` 的 `garm` 列。写表凭据来自 `~/.feishu.json` 或 `FEISHU_CONFIG_FILE`。

`--no-push` 只构建和验证本机镜像。`--no-feishu` 跳过写表，供单独维护镜像使用。可通过 `APK_MIRROR`、`NPM_REGISTRY`、`GITHUB_PROXY` 调整构建源。

两种架构的镜像推送并记表后，在本地运行 `./ictrek.app/scripts/release.sh <版本>`。脚本核对本地提交与飞书记录后推送 `vos-garm-v<版本>`。CI 按发布表中的实际镜像引用打包，生成 GitHub Release，并发布到 VOS App Store。VOS 安装时选择与设备匹配的 `amd` 或 `arm` profile。

## 约束

- 发布前本地工作区必须干净。VOS 镜像使用同步到构建机的源码。
- 不得删除已发布的 tag。序号由 tag 反推。
- 不提交 `dist/`。
