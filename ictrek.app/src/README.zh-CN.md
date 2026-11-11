# GARM

自托管 GitHub Actions 与 Gitea Actions 运行器管理器。

## 安装

在 VOS 应用商店安装 GARM，按设备架构选 AMD64 或 ARM64 profile。创建 Runner 前请确认宿主已初始化 LXD。VOS 菜单提供中文入口，GARM 管理页面使用英文。

## 访问

- 页面：`/app/com.ictrek.garm/ui/`
- 在 VOS 中打开时使用 VOS 单点登录，不需要手动输入账号密码。

## VOS 账户和权限

从 VOS 应用抽屉打开时自动单点登录。GARM 按 VOS 用户 ID 关联账号，首次访问创建与 VOS 同名的账户。再次打开时同步用户名和角色，保留已绑定账户的 GARM ID。VOS `system_admin`（包括 admin 和其他系统管理员）拥有 GARM 管理权限，其他 VOS 角色按普通用户处理。多位 VOS 系统管理员可以同时管理 GARM。

VOS 部署仅使用 VOS 登录。升级后，旧版本未绑定 VOS 身份的会话需要重新登录。若存在同名未绑定账户，GARM 保留并停用该记录，为 VOS 用户创建独立账户。角色变更同步后，变更前签发的 GARM 会话失效。独立部署仍使用 GARM 原有的本地登录和初始化。

### VOS 登录 API

`POST /app/com.ictrek.garm/api/v1/auth/vos` 接收 JSON `{"access_token":"<VOS OAuth2 访问令牌>"}`。令牌通过 VOS 的 OAuth2 PKCE 登录获得，代表已启用的 VOS 用户。成功时返回 HTTP 200 及 `{"token":"<GARM JWT>","username":"<VOS 用户名>"}`。后续管理 API 使用 `Authorization: Bearer <GARM JWT>`。

HTTP 400 表示 JSON 无效或令牌缺失，401 表示令牌未通过验证、账户不可用或未启用 VOS 认证，409 表示用户名已绑定另一 VOS 用户。其他存储故障返回 500。遇到 401 时重新从 VOS 打开应用；遇到 409 时由管理员检查同名账户，避免按名称合并不同用户。

## 配置

- `GARM_LOG_LEVEL`：日志级别，默认 `info`。
- `GARM_LXD_INSTANCE_TYPE`：运行器形态，`container`（默认）或 `virtual-machine`。
- `GARM_LXD_PROJECT`：运行器使用的 LXD 项目，默认 `default`。
- `GARM_LXD_SOCKET_PATH`：宿主机 LXD unix socket，默认 `/var/snap/lxd/common/lxd/unix.socket`。
- 数据保存在 `${VOS_APP_STORAGE_PATH}/garm`，包含 SQLite 数据库与密钥。

## Provider

已预置 `lxd` provider，指向镜像内的 `garm-provider-lxd`，因此创建运行器时可以直接选择 `lxd`。

宿主机需要满足：

- 已安装并初始化 LXD：`sudo lxd init --auto`。
- 运行器的镜像默认从 `ubuntu` / `images` 远程拉取，宿主需能访问 `cloud-images.ubuntu.com`。

## 限制

- 未安装 LXD 的宿主机仍可启动本应用，但创建运行器会失败。
- LXD 与 Docker 在 iptables 上可能冲突，导致运行器无外网。需要执行一次 `sudo iptables -I DOCKER-USER -j ACCEPT`，该规则重启后失效。
- 运行器需要反向访问 GARM 的 metadata 与 callback 地址，初始化时请填写运行器可达的 URL。
- 管理员初始化由页面完成。初始化时会写入 webhook、metadata、callback、agent 四个地址，全部带 `/app/com.ictrek.garm` 前缀。
