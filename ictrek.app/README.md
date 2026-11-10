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
| `scripts/build_image.sh` | 多架构构建并推送镜像 |
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

推送的镜像 tag 只取后两段，即 `20261109.158fe569`，多架构共用这一个 manifest list。

本地构建（`--no-push`）的 tag 带架构前缀，格式为 `<arch>_v<版本>`，由 `scripts/image-ref.sh` 生成：

```text
garm:amd_v0.2.1_20261110.1.e8478193
garm:arm_v0.2.1_20261110.1.e8478193
```

## 发布

```bash
./ictrek.app/scripts/release.sh              # 出镜像、出包、建本地 tag
git push origin main vos-garm-v<版本>         # 公开，触发 CI
```

`--no-push` 只构建本地镜像，不推送仓库。`--no-feishu` 跳过写入飞书发布表。`--dry-run` 只出包，不建 tag。

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
GITHUB_PROXY=https://gh-proxy.example.com \
./ictrek.app/scripts/release.sh --no-push --no-feishu
```

`GITHUB_PROXY` 只影响构建期克隆 provider 仓库。provider 克隆用 `--depth 1`，失败自动重试 3 次。

Docker Hub 由宿主机 `registry-mirrors` 加速，脚本不处理。

CI 在 `.github/workflows/vos-release.yml`，由 `vos-garm-v*` tag 触发：校验镜像与 SHA、生成包、建公开 tag `v<版本>` 并发布 Release、推送 VOS App Store。

## 约束

- 发布前工作区必须干净。`Dockerfile` 用 `git checkout ${GARM_REF}` 取镜像内容。
- 不得删除已发布的 tag。序号由 tag 反推。
- 不提交 `dist/`。
