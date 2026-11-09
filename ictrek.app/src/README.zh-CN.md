# GARM

自托管 GitHub Actions 与 Gitea Actions 运行器管理器。

## 访问

- 页面：`/app/com.ictrek.garm/ui/`
- 首次打开页面时创建管理员账号。

## 配置

- `GARM_LOG_LEVEL`：日志级别，默认 `info`。
- 数据保存在 `${VOS_APP_STORAGE_PATH}/garm`，包含 SQLite 数据库与密钥。

## 限制

- 未预置 runner provider。要创建运行器，需要先在本应用的配置文件里增加 `[[provider]]` 段。
- 管理员初始化由页面完成。初始化时会写入 webhook、metadata、callback、agent 四个地址，全部带 `/app/com.ictrek.garm` 前缀。
