# Stage 1: Build webapp
FROM docker.io/node:lts-alpine AS webapp

# Public URL prefix the app is served under. Must match apiserver.webui.base_path.
ARG GARM_BASE_PATH=
ENV VITE_GARM_BASE_PATH=${GARM_BASE_PATH}

# npm registry used for the webapp build. Override with --build-arg NPM_REGISTRY=
# when building outside mainland China.
ARG NPM_REGISTRY=https://registry.npmmirror.com

WORKDIR /build/webapp
COPY webapp/package.json webapp/package-lock.json ./
RUN if [ -n "${NPM_REGISTRY}" ]; then npm config set registry "${NPM_REGISTRY}"; fi && npm ci
COPY webapp/ ./
RUN npm run build

# Stage 2: Build Go binaries
FROM docker.io/golang:alpine AS builder
ARG GARM_REF
# Alpine mirror used for the builder stage. Override with --build-arg APK_MIRROR=
# when building outside mainland China.
ARG APK_MIRROR=https://mirrors.aliyun.com/alpine
# Optional GitHub proxy prefix, for example https://gh-proxy.com. Only affects
# the provider clones below.
ARG GITHUB_PROXY=

LABEL stage=builder

RUN if [ -n "${APK_MIRROR}" ]; then \
      sed -i "s|https://dl-cdn.alpinelinux.org/alpine|${APK_MIRROR}|g" /etc/apk/repositories; \
    fi && \
    apk add --no-cache musl-dev gcc libtool m4 autoconf g++ make libblkid util-linux-dev git linux-headers upx curl jq
RUN git config --global --add safe.directory /build && git config --global --add advice.detachedHead false
RUN echo ${GARM_REF}

ADD . /build/garm

RUN git -C /build/garm checkout ${GARM_REF}

# Copy webapp build output to assets directory (after git checkout to override any checked-in assets)
COPY --from=webapp /build/webapp/build/ /build/garm/webapp/assets/

RUN cd /build/garm \
    && go build -o /bin/garm \
      -tags osusergo,netgo,sqlite_omit_load_extension \
      -ldflags "-linkmode external -extldflags '-static' -s -w -X github.com/cloudbase/garm/util/appdefaults.Version=$(git describe --tags --match='v[0-9]*' --dirty --always)" \
      /build/garm/cmd/garm && upx /bin/garm
RUN cd /build/garm/cmd/garm-cli \
    && go build -o /bin/garm-cli \
      -tags osusergo,netgo,sqlite_omit_load_extension \
      -ldflags "-linkmode external -extldflags '-static' -s -w -X github.com/cloudbase/garm/util/appdefaults.Version=$(git describe --tags --match='v[0-9]*' --dirty --always)" \
      . && upx /bin/garm-cli
# Only the providers used by this build are compiled. Add repositories here when
# runners are also managed on other platforms.
RUN set -ex; \
    mkdir -p /opt/garm/providers.d; \
    GITHUB_URL="https://github.com"; \
    API_URL="https://api.github.com"; \
    if [ -n "${GITHUB_PROXY}" ]; then \
      GITHUB_URL="${GITHUB_PROXY%/}/https://github.com"; \
      API_URL="${GITHUB_PROXY%/}/https://api.github.com"; \
    fi; \
    for repo in \
      cloudbase/garm-provider-lxd \
      cloudbase/garm-provider-incus; \
    do \
        export PROVIDER_NAME="$(basename $repo)"; \
        export PROVIDER_SUBDIR=""; \
        if [ "$GARM_REF" = "main" ]; then \
          export PROVIDER_REF="main"; \
        else \
          export PROVIDER_REF="$(curl -sS --retry 3 -L "$API_URL/repos/$repo/releases/latest" | jq -r '.tag_name')"; \
        fi; \
        for attempt in 1 2 3; do \
          rm -rf "/build/$PROVIDER_NAME"; \
          if git clone --depth 1 --branch "$PROVIDER_REF" "$GITHUB_URL/$repo" "/build/$PROVIDER_NAME"; then break; fi; \
          echo "clone failed for $repo, attempt $attempt"; \
          sleep 10; \
        done; \
        test -d "/build/$PROVIDER_NAME/.git"; \
        case $PROVIDER_NAME in \
        "garm-provider-k8s") \
            export PROVIDER_SUBDIR="cmd/garm-provider-k8s"; \
            export PROVIDER_LDFLAGS="-linkmode external -extldflags \"-static\" -s -w"; \
            ;; \
        "garm-provider-linode") \
            export PROVIDER_LDFLAGS="-linkmode external -extldflags \"-static\" -s -w"; \
            ;; \
        *) \
            export PROVIDER_VERSION=$(git -C /build/$PROVIDER_NAME describe --tags --match='v[0-9]*' --dirty --always); \
            export PROVIDER_LDFLAGS="-linkmode external -extldflags \"-static\" -s -w -X main.Version=$PROVIDER_VERSION"; \
            ;; \
        esac; \
        cd "/build/$PROVIDER_NAME/$PROVIDER_SUBDIR" \
        && go build -ldflags="$PROVIDER_LDFLAGS" -o /opt/garm/providers.d/$PROVIDER_NAME . \
        && upx /opt/garm/providers.d/$PROVIDER_NAME; \
    done

FROM busybox

COPY --from=builder /bin/garm /bin/garm
COPY --from=builder /bin/garm-cli /bin/garm-cli
COPY --from=builder /opt/garm/providers.d/garm-provider-lxd /opt/garm/providers.d/garm-provider-lxd
COPY --from=builder /opt/garm/providers.d/garm-provider-incus /opt/garm/providers.d/garm-provider-incus
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ENTRYPOINT ["/bin/garm", "-config", "/etc/garm/config.toml"]
