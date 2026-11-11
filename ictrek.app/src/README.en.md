# GARM

Self-hosted runner manager for GitHub Actions and Gitea Actions.

## Install

Install GARM from VOS App Store and select the AMD64 or ARM64 profile for the device. Initialize LXD on the host before creating runners. GARM uses English labels inside the management UI.

## Access

- UI: `/app/com.ictrek.garm/ui/`
- Opened inside VOS, the UI uses VOS single sign-on. No manual credentials are needed.

## Login

- The UI runs the OIDC PKCE flow through `window.vos_platform.api.v1000.oauth2`.
- The backend exchanges the VOS access token on `POST /api/v1/auth/vos`.
- The userinfo endpoint defaults to `http://172.17.0.1:8105/v1000/oauth2/userinfo` and can be overridden with `GARM_VOS_USERINFO_URL`.
- The first VOS user becomes the GARM administrator.
- Local accounts still work when the UI is opened outside VOS.

## Configuration

- `GARM_LOG_LEVEL`: log level, defaults to `info`.
- `GARM_LXD_INSTANCE_TYPE`: runner kind, `container` (default) or `virtual-machine`.
- `GARM_LXD_PROJECT`: LXD project used for runners, defaults to `default`.
- `GARM_LXD_SOCKET_PATH`: host LXD unix socket, defaults to `/var/snap/lxd/common/lxd/unix.socket`.
- Data lives in `${VOS_APP_STORAGE_PATH}/garm`, holding the SQLite database and secrets.

## Provider

The `lxd` provider is preconfigured and points at the bundled `garm-provider-lxd`, so `lxd` is selectable when creating a runner.

The host must have:

- LXD installed and initialized: `sudo lxd init --auto`.
- Access to `cloud-images.ubuntu.com`, since runner images are pulled from the `ubuntu` / `images` remotes.

## Limits

- Hosts without LXD can still start this app, but creating runners will fail.
- LXD and Docker may conflict on iptables, leaving runners without internet. Run `sudo iptables -I DOCKER-USER -j ACCEPT` once; the rule does not survive a reboot.
- Runners must reach GARM's metadata and callback URLs, so use a URL that is reachable from the runner network during setup.
- The setup wizard writes the webhook, metadata, callback and agent URLs, all prefixed with `/app/com.ictrek.garm`.
