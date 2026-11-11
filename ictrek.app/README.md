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
| `scripts/image-ref.sh` | 生成镜像引用，带架构前缀 |
| `scripts/build_image.sh` | 构建并推送单架构镜像 |
| `scripts/package.sh` | 生成 pull 安装包 |
| `scripts/feishu_sheet.py` | 把镜像 tag 写入 VOS 发布表 |
| `scripts/release.sh` | 构建并推送本机架构镜像 |
| `scripts/update_version.sh` | 出包、打 tag、推送发布 |
| `image-tags` | 两种架构的推送 tag 记录，随代码提交 |
| `dist/` | 构建产物，不提交 |

## 版本号

```text
<上游版本>-<DATE>.<N>.<GITSHA>
```

示例 `0.2.1-20261109.1.158fe569`。

- `<上游版本>`：HEAD 上最近的稳定上游 tag，去掉 `v`。
- `<DATE>`：`Asia/Shanghai` 的 `YYYYMMDD`。
- `<N>`：远端同名日期 tag 的最大序号加一。
- `<GITSHA>`：最后一次改动 `ictrek.app/` 以外路径的提交，8 位起，遇前导零加长。
- 只改 `ictrek.app/` 的发布中，`<GITSHA>` 停在上一个代码提交，与镜像 tag 里的 SHA 相同。

推送的镜像 tag 带架构前缀，格式为 `<arch>_v<版本>`，与飞书发布表的值逐字相同：

```text
swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm:amd_v0.2.1_20261110.5.f827f4df
swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm:arm_v0.2.1_20261110.5.f827f4df
```

- 一台构建机只构建一种架构：amd64 在 x86_64 主机，arm64 在 arm64 主机。架构与本机不一致时 `build_image.sh` 直接退出。
- 本地构建（`--no-push`）的 tag 不带仓库前缀，由 `scripts/image-ref.sh` 生成，格式为 `garm:<arch>_v<版本>`。
- 推送后 `build_image.sh` 把本机架构的 tag 写入 `ictrek.app/image-tags`。安装包只读该文件，不读飞书。
- 两台机器的版本必须相同。`update_version.sh` 检查 `image-tags` 的两行；版本不同时直接退出。

## 发布

改了 garm 代码：

```bash
./ictrek.app/scripts/release.sh   # 本机架构构建并推送，写飞书、写 image-tags
# 在另一台架构的机器上再运行一次
# 代码改动先提交；image-tags 由 update_version.sh 提交
./ictrek.app/scripts/update_version.sh   # 提交 image-tags、出包、打 tag、推送
```

只改 `ictrek.app/`：

```bash
git commit
./ictrek.app/scripts/update_version.sh   # 出包、打 tag、推送，不构建镜像
```

`release.sh --no-push` 只构建本机架构的本地镜像。`--no-feishu` 跳过写入飞书发布表。`--dry-run` 按现有 `image-tags` 出包，不建镜像。`update_version.sh --no-push` 只出包和本地 tag，不推送。

推送成功后把镜像 tag 写入 VOS 发布表。凭据来自 `~/.feishu.json`，或用 `FEISHU_CONFIG_FILE` 指定文件。字段为 `feishu_app_id`、`feishu_app_secret`、`feishu_spreadsheet_token`。

- amd 镜像写入 `AMD_with_cuda`。
- arm 镜像写入 `ARM_without_cuda`。
- 列名为 `garm`，列不存在时自动创建并补写第 1、2 行。
- 单元格内容形如 `amd_v0.2.1_20261110.1.e8478193`。
- 写表失败时命令直接退出，不继续。

构建慢时可以换源：

```bash
APK_MIRROR=https://mirrors.aliyun.com/alpine \
NPM_REGISTRY=https://registry.npmmirror.com \
./ictrek.app/scripts/release.sh --no-push --no-feishu
```

`GITHUB_PROXY` 只影响构建期查询 release 与克隆 provider 仓库，默认 `https://gh-proxy.com`。国内直连 `github.com` 常超时，单次克隆会等上百秒。需要直连时显式传空值：

```bash
GITHUB_PROXY= ./ictrek.app/scripts/release.sh
```

provider 克隆用 `--depth 1`，失败自动重试 3 次。

Docker Hub 由宿主机 `registry-mirrors` 加速，脚本不处理。

CI 在 `.github/workflows/vos-release.yml`，由 `vos-garm-v*` tag 触发：校验 SHA、生成包、建公开 tag `v<版本>` 并发布 Release、推送 VOS App Store。CI 不构建镜像，发布前必须已推送 `amd_v<版本>` 与 `arm_v<版本>`。

## 约束

- 发布前工作区必须干净。`Dockerfile` 用 `git checkout ${GARM_REF}` 取镜像内容。
- 不得删除已发布的 tag。序号由 tag 反推。
- 不提交 `dist/`。
- `ictrek.app/image-tags` 必须提交。CI 用它生成安装包。
- 改了 garm 代码必须重建两种架构镜像；只改 `ictrek.app/` 时不重建。
