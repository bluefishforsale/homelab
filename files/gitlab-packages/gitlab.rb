# External URL
################################################################################
################################################################################
##                Configuration Settings for GitLab CE and EE                 ##
################################################################################
################################################################################

external_url 'http://{{ hostname }}

gitlab_rails['initial_root_password'] = "password1234"
gitlab_rails['store_initial_root_password'] = true

# GitLab Shell
gitlab_rails['gitlab_shell_ssh_port'] = 22

### Default Theme
### Available values:
##! `1`  for Indigo
##! `2`  for Dark
##! `3`  for Light
##! `4`  for Blue
##! `5`  for Green
##! `6`  for Light Indigo
##! `7`  for Light Blue
##! `8`  for Light Green
##! `9`  for Red
##! `10` for Light Red
gitlab_rails['gitlab_default_theme'] = 2


# Database settings
# Uncomment and configure these settings if you want to use an external database
# postgresql['enable'] = false
# gitlab_rails['db_adapter'] = 'postgresql'
# gitlab_rails['db_encoding'] = 'unicode'
# gitlab_rails['db_database'] = 'gitlabhq_production'
# gitlab_rails['db_pool'] = 10
# gitlab_rails['db_username'] = 'gitlab'
# gitlab_rails['db_password'] = 'secure_password'
# gitlab_rails['db_host'] = '127.0.0.1'
# gitlab_rails['db_port'] = 5432

# Nginx settings
nginx['enable'] = true
nginx['listen_port'] = 80
nginx['listen_https'] = false
# nginx['proxy_set_headers'] = {
#   "Host" => "$http_host",
#   "X-Real-IP" => "$remote_addr",
#   "X-Forwarded-For" => "$proxy_add_x_forwarded_for",
#   "X-Forwarded-Proto" => "http"
# }

# GitLab data directories
git_data_dirs({
  "default" => {
    "path" => "/var/opt/gitlab/git-data"
  }
})

# Log files
logging['log_directory'] = "/var/log/gitlab"

# Backup settings
gitlab_rails['backup_path'] = "/var/opt/gitlab/backups"
gitlab_rails['backup_keep_time'] = 604800 # 7 days

# Email settings
gitlab_rails['smtp_enable'] = false
# gitlab_rails['smtp_address'] = "smtp.example.com"
# gitlab_rails['smtp_port'] = 587
# gitlab_rails['smtp_user_name'] = "smtp_user"
# gitlab_rails['smtp_password'] = "smtp_password"
# gitlab_rails['smtp_domain'] = "example.com"
# gitlab_rails['smtp_authentication'] = "login"
# gitlab_rails['smtp_enable_starttls_auto'] = true

# Registry settings
# registry_external_url 'http://registry.example.com'

# Prometheus monitoring
prometheus_monitoring['enable'] = true

# Redis settings
redis['enable'] = true

# Sidekiq concurrency settings
sidekiq['concurrency'] = 25