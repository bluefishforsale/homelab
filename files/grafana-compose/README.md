# Grafana Monitoring Dashboard

Docker Compose deployment with integrated MySQL database for Grafana monitoring platform.

---

## Quick Reference

| Component | Image | Port |
|-----------|-------|------|
| Grafana | grafana/grafana:11.5.4 | 8910:3000 |
| MySQL | percona/percona-server:5.7 | 3306:3306 |

---

## Deployment

```bash
# Deploy Grafana with integrated MySQL (requires vault)
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/monitoring/grafana_compose.yaml --ask-vault-pass
```

---

## Architecture

Two-container deployment with integrated database:

```text
┌─────────────────────────────────────────────────┐
│ Docker Compose Stack                            │
│                                                 │
│  ┌──────────────┐      ┌──────────────────┐    │
│  │   Grafana    │─────▶│  grafana-mysql   │    │
│  │   :8910      │      │     :3306        │    │
│  └──────────────┘      └──────────────────┘    │
│         │                      │               │
│         ▼                      ▼               │
│     /data01/               /data01/            │
│     services/              services/           │
│     grafana/data           grafana/mysql-data  │
└─────────────────────────────────────────────────┘
```

---

## Directory Structure

```text
/data01/services/grafana/
├── docker-compose.yml
├── .env
├── grafana.ini                 # Grafana configuration
├── mysql-init.sql              # Database initialization
├── data/                       # Grafana data
├── logs/                       # Grafana logs
├── plugins/                    # Grafana plugins
├── mysql-data/                 # MySQL data
├── mysql-logs/                 # MySQL logs
├── mysql-conf/
│   └── custom.cnf              # MySQL configuration
├── provisioning/
│   ├── users/
│   │   └── users.yml           # Auto-provisioned users
│   └── datasources/
│       └── loki.yml            # Loki datasource
└── dashboards/                 # Dashboard JSON files (27 items)
```

---

## Files

| File | Purpose |
|------|---------|
| `docker-compose.yml.j2` | Grafana + MySQL compose config |
| `grafana.service.j2` | Systemd service |
| `grafana.env.j2` | Environment variables |
| `grafana-mysql-container.ini.j2` | Grafana configuration |
| `mysql-init.sql.j2` | MySQL database initialization |
| `mysql-custom.cnf.j2` | MySQL configuration |
| `users.yml.j2` | User provisioning |
| `dashboards/` | Pre-built dashboard JSON files |

---

## Configuration

### Access

- **URL**: http://grafana.home (via nginx proxy)
- **Direct**: http://192.168.1.143:8910
- **MySQL**: localhost:3306 (from host)

### Pre-installed Plugins

- `marcusolsson-treemap-panel 2.0.0` - Hierarchical data visualization

### Provisioned Datasources

- **Loki**: http://192.168.1.143:3100 (log aggregation)

---

## Service Management

```bash
# Status
systemctl status grafana.service

# Restart
systemctl restart grafana.service

# Logs
journalctl -u grafana.service -f
docker compose -f /data01/services/grafana/docker-compose.yml logs -f

# Container health
docker ps --format "table {{.Names}}\t{{.Status}}" | grep grafana
```

---

## Health Checks

```bash
# Grafana API
curl http://localhost:8910/api/health

# MySQL
docker exec grafana-mysql mysqladmin ping -h localhost -u root -p

# Container status
docker inspect grafana --format='{{.State.Health.Status}}'
docker inspect grafana-mysql --format='{{.State.Health.Status}}'
```

---

## Troubleshooting

### Grafana won't start

```bash
# Check MySQL is healthy first (Grafana depends on it)
docker logs grafana-mysql

# Then check Grafana
docker logs grafana
```

### Database connection failed

```bash
# Verify MySQL container is running
docker ps | grep grafana-mysql

# Test MySQL connectivity
docker exec grafana-mysql mysql -u grafana -p -e "SELECT 1"

# Check Grafana config
cat /data01/services/grafana/grafana.ini | grep -A5 "\[database\]"
```

### Plugin installation failed

```bash
# Check plugin directory permissions
ls -la /data01/services/grafana/plugins/

# Manually install
docker exec grafana grafana-cli plugins install marcusolsson-treemap-panel
```

---

## Network

Uses **bridge** network mode (default Docker networking):

- Grafana → MySQL via container link (depends_on)
- External access via host ports (8910, 3306)
- nginx proxies via host IP: `http://192.168.1.143:8910`

---

## Security

- **no-new-privileges**: Both containers
- **Resource limits**: 1 CPU, 1GB memory each
- **Read-only config**: grafana.ini mounted read-only
- **Vault secrets**: MySQL passwords from encrypted vault
- **Health checks**: Auto-restart on failure
