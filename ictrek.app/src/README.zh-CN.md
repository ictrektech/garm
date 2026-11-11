# GARM

自托管 GitHub Actions 与 Gitea Actions 运行器管理器。

## 访问

- 页面：`/app/com.ictrek.garm/ui/`
- 在 VOS 中打开时使用 VOS 单点登录，不需要手动输入账号密码。

## 登录

- 前端通过 `window.vos_platform.api.v1000.oauth2` 走 PKCE 授权码流程，取得 VOS 访问令牌。
- 后端用 `POST /api/v1/auth/vos` 换取 GARM 令牌。
- 令牌校验地址默认是 `http://172.17.0.1:8105/v1000/oauth2/userinfo`，可用 `GARM_VOS_USERINFO_URL` 覆盖。
- 第一个登录的 VOS 用户自动成为 GARM 管理员。
- 不通过 VOS 访问时，仍然可以使用本地账号登录。

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
