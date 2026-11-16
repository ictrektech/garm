# Dev 镜像：Go 工具链 + 热重载（air）+ 调试器（dlv）。
# 源码不入镜像，运行时由 docker-compose.dev.yml 挂载到 /app，保存即重编译。
#
# 与生产镜像（仓库根 Dockerfile）的区别：
#   - 生产用 golang:alpine 静态编译、busybox 运行；dev 用 bookworm（glibc），
#     与宿主机 `make build` 一致，dlv 在 glibc 下可用
#   - 不 COPY 源码，不 upx 压缩，不内置 provider 二进制
FROM docker.io/golang:1.26-bookworm

# 国内网络可改 APT_MIRROR=mirrors.aliyun.com、GOPROXY。
ARG APT_MIRROR=
ARG GOPROXY=https://goproxy.cn,direct
# 固定版本：air 与 dlv 的 CLI/配置项会变，latest 会让同一次 commit 在不同日期
# 重建出不同工具链。升级时单独改这两行。
ARG AIR_VERSION=v1.67.4
ARG DELVE_VERSION=v1.27.2

RUN if [ -n "${APT_MIRROR}" ] && [ -f /etc/apt/sources.list.d/debian.sources ]; then \
      sed -i "s|deb.debian.org|${APT_MIRROR}|g" /etc/apt/sources.list.d/debian.sources; \
    fi

# sqlite3 走 cgo（mattn/go-sqlite3），编译需要 gcc；git 用于版本串。
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        gcc libc6-dev git ca-certificates curl wget \
    && rm -rf /var/lib/apt/lists/*

ENV GOPROXY=${GOPROXY} \
    CGO_ENABLED=1

RUN go install github.com/air-verse/air@${AIR_VERSION} \
    && go install github.com/go-delve/delve/cmd/dlv@${DELVE_VERSION}

WORKDIR /app

COPY docker/dev/entrypoint.sh /usr/local/bin/garm-dev-entrypoint
# 挂载进来的工作区属主是宿主用户，容器内是 root，git 会报 dubious ownership；
# go build 的 VCS stamping 与 air 里的 git rev-parse 都需要这一行。
RUN chmod +x /usr/local/bin/garm-dev-entrypoint \
    && git config --global --add safe.directory /app

EXPOSE 9997 2345

ENTRYPOINT ["/usr/local/bin/garm-dev-entrypoint"]
