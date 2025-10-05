# MySQL Database Server with Docker Compose

This deployment configures MySQL (Percona Server) as a database server using Docker Compose, replacing the previous systemd-based container deployment.

## Key Benefits

- **Network Integration**: Creates dedicated `mysql_legacy` network for database connectivity
- **Idempotent Deployment**: Safe to run multiple times without side effects
- **Health Monitoring**: Built-in health checks for service monitoring
- **Resource Management**: CPU and memory limits for stable operation
- **Security Hardening**: No new privileges and proper file permissions

## Architecture

### Service Configuration
- **Image**: percona/percona-server:5.7
- **Port**: 3306 (MySQL standard)
- **Network**: mysql_legacy (dedicated database network)
- **Health Check**: mysqladmin ping command
- **Resources**: 2 CPU cores, 2GB memory limit

### Directory Structure
```
/data01/services/mysql/
├── docker-compose.yml          # Docker Compose configuration
├── .env                       # Environment variables
├── data/                      # MySQL database files
├── logs/                      # MySQL logs (error, slow query)
└── conf.d/                    # MySQL configuration files
    └── custom.cnf             # Custom MySQL settings
```

### Database Configuration
Pre-configured databases and users:
- **Root User**: Full admin access with vault-encrypted password
- **Grafana User**: Limited access to grafana database
- **Grafana Database**: Dedicated database for Grafana monitoring

### Network Architecture
The MySQL container creates and joins the `mysql_legacy` network:
- **Subnet**: 172.21.0.0/16
- **Purpose**: Dedicated network for database services
- **Container Resolution**: `mysql:3306` for container-to-container communication
- **External Access**: `192.168.1.143:3306` for legacy services

## Database Users and Permissions

### Grafana Integration
- **Database**: grafana
- **User**: grafana
- **Password**: From vault (monitoring.grafana.database_password)
- **Connection**: mysql://grafana:password@mysql:3306/grafana

## Deployment

### Prerequisites
1. Docker and Docker Compose installed
2. ZFS mounts available at `/data01`
3. Vault secrets configured with database passwords

### Deploy
```bash
ansible-playbook playbook_ocean_mysql_compose.yaml
```

### Service Management
```bash
# Start service
sudo systemctl start mysql.service

# Stop service  
sudo systemctl stop mysql.service

# Restart service
sudo systemctl restart mysql.service

# Check status
sudo systemctl status mysql.service

# View logs
journalctl -u mysql.service -f
docker-compose -f /data01/services/mysql/docker-compose.yml logs -f
```

### Database Administration
```bash
# Connect to MySQL container
docker exec -it mysql mysql -u root -p

# Check database status
docker exec mysql mysqladmin ping -u root -p

# View databases
docker exec mysql mysql -u root -p -e "SHOW DATABASES;"

# Check Grafana user
docker exec mysql mysql -u root -p -e "SHOW GRANTS FOR 'grafana'@'%';"
```

## Configuration

### MySQL Settings
Custom configuration in `conf.d/custom.cnf`:
- **Buffer Pool**: 1GB InnoDB buffer pool size
- **Connections**: 200 max connections
- **Character Set**: UTF8MB4 with unicode collation
- **Logging**: Slow query log and error logging enabled
- **Security**: Bind to all interfaces, skip name resolution

### Performance Tuning
- **InnoDB Buffer Pool**: 1GB for improved performance
- **Query Cache**: 64MB query cache enabled
- **Slow Query Log**: Enabled for performance monitoring

## Network Integration

### Services Using mysql_legacy Network
1. **MySQL**: Creates the network
2. **Grafana**: Joins network for database connectivity

### Adding Services to Network
To connect other services to MySQL:

```yaml
# In service docker-compose.yml
services:
  my-service:
    networks:
      - mysql_legacy  # Add this line

networks:
  mysql_legacy:
    external: true
```

## Security

### Network Security
- **Isolated Network**: Database traffic on dedicated mysql_legacy network
- **Container Security**: No new privileges, restricted capabilities
- **File Permissions**: Proper ownership and permissions

### Database Security
- **Encrypted Passwords**: All passwords stored in Ansible vault
- **Limited Users**: Service-specific database users with minimal permissions
- **Network Binding**: Configured to accept connections from container network

## Troubleshooting

### Common Issues

1. **Connection Refused**
   - Check if MySQL service is running: `systemctl status mysql`
   - Verify container is healthy: `docker ps`
   - Test network connectivity: `docker exec grafana ping mysql`

2. **Authentication Failed**
   - Verify passwords in vault_secrets.yaml
   - Check user permissions: `SHOW GRANTS FOR 'grafana'@'%';`

3. **Database Not Found**
   - Ensure database was created: `SHOW DATABASES;`
   - Check container logs: `docker logs mysql`

### Network Debugging
```bash
# List Docker networks
docker network ls

# Inspect mysql_legacy network
docker network inspect mysql_legacy

# Test connectivity from Grafana
docker exec grafana ping mysql
docker exec grafana telnet mysql 3306
```

### Performance Monitoring
```bash
# Check MySQL status
docker exec mysql mysql -u root -p -e "SHOW STATUS;"

# View process list
docker exec mysql mysql -u root -p -e "SHOW PROCESSLIST;"

# Check slow queries
docker exec mysql tail -f /logs/mysql-slow.log
```

## Migration from Legacy Setup

If migrating from the previous systemd-based deployment:

1. **Backup existing data**: 
   ```bash
   docker exec mysql mysqldump -u root -p --all-databases > mysql_backup.sql
   ```

2. **Stop old service**: 
   ```bash
   sudo systemctl stop mysql.service && sudo systemctl disable mysql.service
   ```

3. **Deploy new setup**: 
   ```bash
   ansible-playbook playbook_ocean_mysql_compose.yaml
   ```

4. **Update dependent services**: Ensure Grafana and other services use new container connection

5. **Verify functionality**: Test database connectivity and service functionality

## Integration with Other Services

### Grafana Integration
Grafana automatically connects to MySQL via container name:
- **Connection String**: `mysql://grafana:password@mysql:3306/grafana`
- **Network**: Both services on mysql_legacy network
- **Health Checks**: Grafana waits for MySQL to be healthy

### Future Services
Other services can easily connect to MySQL:
- Join mysql_legacy network
- Use connection string: `mysql://user:password@mysql:3306/database`
- Configure appropriate database users and permissions

This establishes a robust, containerized database foundation for the homelab infrastructure.
