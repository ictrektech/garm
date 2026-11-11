# GARM

Self-hosted runner manager for GitHub Actions and Gitea Actions.

## Install

Install GARM from VOS App Store and select the AMD64 or ARM64 profile for the device. Initialize LXD on the host before creating runners. GARM uses English labels inside the management UI.

## Access

- UI: `/app/com.ictrek.garm/ui/`
- Opened inside VOS, the UI uses VOS single sign-on. No manual credentials are needed.

## VOS accounts and roles

Open GARM from VOS App Drawer to sign in automatically. GARM links accounts by VOS user ID and creates a GARM account with the VOS username on first access. Reopening the app synchronizes the username and role while retaining the linked GARM account ID. The VOS `system_admin` role grants GARM administrator access to the built-in admin and other VOS system administrators. Other VOS roles sign in as regular users. Multiple VOS administrators can manage GARM.

VOS installations use VOS sign-in exclusively. After upgrading, sessions for older unlinked accounts must sign in again. If an unlinked account already uses the same username, GARM retains and disables that record and creates a separate account for the VOS user. Once a role change has synchronized, sessions issued before the change are rejected. Standalone installations retain GARM's local sign-in and initialization.

### VOS sign-in API

`POST /app/com.ictrek.garm/api/v1/auth/vos` accepts JSON `{"access_token":"<VOS OAuth2 access token>"}`. Obtain the token via VOS OAuth2 with PKCE for an enabled VOS user. A successful request returns HTTP 200 and `{"token":"<GARM JWT>","username":"<VOS username>"}`. Use `Authorization: Bearer <GARM JWT>` for subsequent management requests.

HTTP 400 indicates invalid JSON or a missing token. HTTP 401 indicates an invalid token, an unavailable account or disabled VOS authentication. HTTP 409 indicates the username is already bound to a different VOS user. Other storage failures return HTTP 500. For a 401, reopen the app from VOS. For a 409, ask the administrator to check the conflicting account binding.

## Local Gitea discovery

When a VOS system administrator opens GARM, it fills missing controller URLs from the current VOS gateway. If Gitea is available on the same device, Endpoints shows `vos-gitea`. Existing URLs and endpoints are preserved. When local Gitea is absent, GARM remains usable and a remote Gitea endpoint can be added manually. Discovery configures addresses. Set credentials for the target repository separately. VOS sign-in triggers discovery without changing the sign-in request, response or success status.

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
