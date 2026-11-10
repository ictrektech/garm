# GARM

Self-hosted runner manager for GitHub Actions and Gitea Actions.

## Access

- UI: `/app/com.ictrek.garm/ui/`
- Create the administrator account on the first page load.

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
