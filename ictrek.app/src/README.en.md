# GARM

Self-hosted runner manager for GitHub Actions and Gitea Actions.

## Access

- UI: `/app/com.ictrek.garm/ui/`
- Create the administrator account on the first page load.

## Configuration

- `GARM_LOG_LEVEL`: log level, defaults to `info`.
- Data lives in `${VOS_APP_STORAGE_PATH}/garm`, holding the SQLite database and secrets.

## Limits

- No runner provider is preconfigured. To create runners, add a `[[provider]]` section to this app's configuration file.
- The setup wizard writes the webhook, metadata, callback and agent URLs, all prefixed with `/app/com.ictrek.garm`.
