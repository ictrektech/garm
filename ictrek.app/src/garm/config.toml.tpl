[logging]
enable_log_streamer = true
log_format = "text"
log_level = "__LOG_LEVEL__"
log_source = false

[metrics]
enable = true
disable_auth = false

[jwt_auth]
secret = "__JWT_SECRET__"
time_to_live = "8760h"

[apiserver]
  bind = "0.0.0.0"
  port = 9997
  use_tls = false
  [apiserver.webui]
    enable = true
    base_path = "__BASE_PATH__"

[database]
  backend = "sqlite3"
  passphrase = "__DB_PASSPHRASE__"
  [database.sqlite3]
    db_file = "/data/garm.db"

[[provider]]
  name = "lxd"
  provider_type = "external"
  description = "Local LXD installation"
  [provider.external]
    provider_executable = "/opt/garm/providers.d/garm-provider-lxd"
    config_file = "/data/garm-provider-lxd.toml"
