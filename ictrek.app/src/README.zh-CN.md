# GARM

自托管 GitHub Actions 与 Gitea Actions 运行器管理器。

## 访问

- 页面：`/app/com.ictrek.garm/ui/`
- 首次打开页面时创建管理员账号。

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
