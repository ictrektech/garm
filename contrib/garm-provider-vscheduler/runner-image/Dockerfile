# Runner image for garm-provider-vscheduler.
#
# V-Scheduler starts deployment containers with a fixed command, no volumes and
# exactly one environment variable (V_SCHEDULER_BACKEND_CONFIG_B64), so this
# image fetches its own bootstrap data at start.
#
# No RUN step: the arm64 image builds without emulation, and both base images
# already provide what the entrypoint needs (curl in the GitHub image, wget in
# the Gitea one).
#
# The default is the Gitea runner, which is what this provider is used with.
# Build a GitHub runner with --build-arg RUNNER_BASE=ghcr.io/actions/actions-runner:latest.
ARG RUNNER_BASE=docker.io/gitea/runner:latest
FROM ${RUNNER_BASE}
COPY --chmod=0755 entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
