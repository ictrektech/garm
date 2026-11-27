# garm-provider-vscheduler

GARM 外部 provider。把一个 runner 的生命周期翻译成 V-Scheduler 的
`backend-deployments` 调用。一个 GARM 控制面因此可以把 runner 放到所有已在
V-Scheduler 登记的节点上。

## 链路

```
garm pool ──▶ 本 provider ──HTTP──▶ V-Scheduler /backend-deployments
                                          │ 后台 tick
                                          ▼
                              scheduler-node-manager ──SSH──▶ docker run
```

## 1. V-Scheduler 侧

1. 在「节点管理 → SSH 算力节点」登记目标节点。
2. 为 GARM 创建服务凭据，`app_id` 例如 `com.ictrek.garm`。
3. 管理员授权节点与仓库：

```bash
curl -X PUT "$SCHEDULER/backend-deployment-access/com.ictrek.garm" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  --data '{"targets":[{"kind":"ssh","id":"node-35","profile":"jetson-thor"}],
           "repositories":["registry.example.com/ci"]}'
```

`repositories` 必须写**去掉 tag 的完整镜像名**，且 registry 段要带点或端口。
V-Scheduler 的规则见 `backend/platform/shared/application_backend.rs` 的
`repository_valid`：registry 必须含 `.`、`:` 或等于 `localhost`，所以
`gitea/runner` 会被拒（收到 422 `registry "gitea" needs a dot or a port`）。
授权里的字符串与 pool 的 `image` 去掉 tag 后必须完全一致。

`targets` 里必须包含每一个允许落地的节点。新加节点时要重新提交。

## 2. runner 镜像

```bash
# 在哪个架构的节点上跑，就构建哪个 --target。没有 RUN 步骤，可以交叉构建。
ictrek.app/scripts/build_runner_image.sh --target arm --version 6.0.1

# GitHub 底包
ictrek.app/scripts/build_runner_image.sh --target arm --runner-base github --version 2.338.2

# 两个架构都推完后，把 latest 指向该版本（多架构清单）
ictrek.app/scripts/build_runner_image.sh --latest --version 6.0.1

# 列出仓库里已有的 tag，新的在前
ictrek.app/scripts/build_runner_image.sh --list
```

tag 是 `<amd|arm>_v<底包版本>_<内容哈希8位>`。哈希只覆盖 Dockerfile、
entrypoint.sh 与底包引用，所以仓库里其他代码变动不影响 tag。同一份内容重复
构建会直接跳过。

`--version` 的前三段是底包版本。脚本把镜像推到
`swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm-runner:<amd|arm>_v<版本>`。
推完后必须人工把池的 `image` 改成仓库名。provider 会按池的 `os_arch` 补上
`amd_` 或 `arm_` 前缀的 tag，用户不用记 tag。池不要填 `latest`：
V-Scheduler 用 `docker run --pull=never`，节点本地已有同名 tag 就不会再拉。
`--list` 列出可用的 tag。

容器启动后从 `V_SCHEDULER_BACKEND_CONFIG_B64` 读取 GARM 的 metadata 地址与实例
token。GitHub 走 JIT 文件；Gitea 没有 JIT，改走 `/runner-registration-token/` 加
注册令牌，与 `internal/templates/userdata/gitea_linux_userdata.tmpl` 同一条路径。

V-Scheduler 不传卷、不传自定义命令、只传这一个环境变量，所以引导信息必须走
`config` 字段。这是本方案的核心约束。

## 3. GARM 侧

`config.toml`：

```toml
[[provider]]
  name = "vscheduler"
  provider_type = "external"
  description = "V-Scheduler nodes"
  [provider.external]
    provider_executable = "/opt/garm/providers.d/garm-provider-vscheduler"
    config_file = "/etc/garm/garm-provider-vscheduler.json"
```

`/etc/garm/garm-provider-vscheduler.json`：

```json
{
  "base_url": "http://v-scheduler:38120/api/v1",
  "token": "<GARM 的服务凭据>",
  "container_port": 8080,
  "timeout_seconds": 30,
  "image_tag": "v6.0.1_3b01e082"
}
```

`image_tag` 是 tag 去掉架构前缀的部分。池的 `image` 只写仓库名时，provider 用池的
`os_arch` 拼出 `:amd_v6.0.1_3b01e082` 或 `:arm_v6.0.1_3b01e082`。一个配置值同时服务
两个架构。池的 `extra_specs.image_tag` 可以覆盖单个池。

`image_tag` 不要带 `amd_` / `arm_` 前缀，provider 会自己加。带了会直接报错。

provider 会比对池的 `os_arch` 与选中节点的架构，不一致时报错。V-Scheduler 上报
`aarch64` / `x86_64`，provider 归一后比对。

pool 的 `extra_specs`：

```json
{
  "profile": "jetson-thor",
  "image_tag": "v6.0.1_3b01e082",
  "container_port": 8080,
  "cpu_millicores": 4000,
  "memory_mib": 8192
}
```

pool 的 `image` 可以只写仓库（`swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm-runner`），
也可以写完整的带 tag 引用。带 tag 的引用原样使用，不做补全。

| 字段 | 必填 | 说明 |
|---|---|---|
| `profile` | 是，除非给了 `target_id` | 按 Profile 选在线节点。取 `compatible_profiles` 命中者。 |
| `target_id` | 否 | 固定落到某个节点，关闭自动放置。 |
| `image_tag` | 否 | 覆盖 provider 配置里的 `image_tag`，只对本池生效。 |
| `target_kind` | 否 | `ssh`（默认）或 `cluster`。 |
| `container_port` | 否 | 容器内端口，默认取 provider 配置。runner 不监听端口。 |
| `cpu_millicores` / `memory_mib` | 否 | 传给 V-Scheduler 的容器上限。 |

pool 的 `image` 填 runner 镜像。`flavor` 不使用，填 `container` 即可。

## 面板里选节点

provider 名选中 `vscheduler` 后，池的表单会出现 Deployment Target 下拉。它来自
garm 的 `GET /api/v1/providers/{provider}/targets`，garm 读本 provider 的配置里的
`base_url` 与 `token`，再去 V-Scheduler 拉 `/managed-nodes`。

- 选项分两组：按 Profile（交给 V-Scheduler 按负载选节点）和具体节点（`target_id`，钉死一台）。
- 选具体节点时会把池的 `os_arch` 一起改成该节点架构，避免架构不匹配。
- 选中结果写进 `extra_specs`，JSON 编辑器里仍可手改。
- V-Scheduler 不可达或该 provider 没有节点清单时，下拉不显示，表单退回手写 `extra_specs`。

放置规则：在 Profile 命中的在线节点里选存活部署最少的一台。同分时按实例名散列
选择。

## 限制

- 每个 runner 占一个宿主机端口。端口池是每台节点 `42000-42999`，单节点上限
  1000 个并发 runner。
- 容器无卷、无特权、无 docker socket。`docker build` 类 job 无法运行。
- 部署列表接口最多返回最近 200 条，列表用于选节点和回收，超过 200 个存活 runner
  时放置统计会偏。
- 回收是异步的。`DeleteInstance` 最多等 40 秒确认 `removed`。
- runner 跑完一个 job 后进程退出，容器会保持运行（脚本末尾 `sleep`），等 garm 回收。
  直接退出会被 `--restart unless-stopped` 重启，重启时注册令牌已用过，只会得到 4xx。

## 测试

```bash
cd contrib/garm-provider-vscheduler && go test ./...
```

测试覆盖放置选择、状态映射与一次建单请求的字段。不覆盖真实 V-Scheduler。
