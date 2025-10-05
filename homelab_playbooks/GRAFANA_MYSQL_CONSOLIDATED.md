# Grafana with Integrated MySQL - Consolidated Docker Compose

This deployment consolidates MySQL database into the Grafana docker-compose stack since MySQL is only serving Grafana. This provides better security, simpler management, and cleaner architecture.

## Architecture Benefits

### **Consolidated Stack**
- **Single deployment**: Both Grafana and MySQL in one docker-compose file
- **Private networking**: MySQL only accessible to Grafana via internal network
- **Service dependencies**: Grafana waits for MySQL to be healthy before starting
- **Unified management**: Single systemd service manages both services

### **Network Design**
```
┌─────────────┐    web_proxy     ┌─────────────┐    grafana_internal    ┌─────────────┐
│    nginx    │ ←──────────────→ │   grafana   │ ←────────────────────→ │    mysql    │
└─────────────┘                  └─────────────┘                        └─────────────┘
     Public                         Public/Private                         Private Only
```

**Networks:**
- **web_proxy**: External network for nginx → grafana communication
- **grafana_internal**: Internal network for grafana ↔ mysql communication (private)

### **Security Improvements**
- ✅ **MySQL isolation**: Database only accessible within container network
- ✅ **No external MySQL port**: MySQL not exposed to host network
- ✅ **Private communication**: All database traffic stays internal
- ✅ **Reduced attack surface**: No external database access points

## Configuration

### **MySQL Configuration**
**Optimized for Grafana workload:**
- **Buffer Pool**: 512M (smaller than standalone, sufficient for Grafana)
- **Max Connections**: 50 (Grafana doesn't need many connections)
- **Character Set**: UTF8MB4 for full Unicode support
- **Performance**: Query cache enabled for dashboard queries

### **Resource Allocation**
- **MySQL**: 1 CPU core, 1GB memory (dedicated to Grafana)
- **Grafana**: 1 CPU core, 1GB memory
- **Total**: 2 CPU cores, 2GB memory for the combined stack

### **Directory Structure**
```
/data01/services/grafana/
├── docker-compose.yml          # Combined Grafana + MySQL stack
├── .env                       # Environment variables
├── grafana.ini                # Grafana configuration
├── data/                      # Grafana data
├── logs/                      # Grafana logs
├── plugins/                   # Grafana plugins
├── mysql-data/                # MySQL database files
├── mysql-logs/                # MySQL logs
└── mysql-conf/                # MySQL configuration
    └── custom.cnf             # MySQL settings
```

## Database Configuration

### **Grafana Database Setup**
- **Database**: grafana
- **User**: grafana
- **Connection**: `mysql://grafana:password@mysql:3306/grafana`
- **Password**: From vault (monitoring.grafana.database_password)

### **MySQL Users**
- **Root User**: Administrative access (vault encrypted)
- **Grafana User**: Application access with limited permissions
- **No External Users**: Only accessible within container network

## Deployment

### **Prerequisites**
1. web_proxy network exists (created by nginx deployment)
2. Vault secrets configured with database passwords
3. Old standalone MySQL service cleaned up automatically

### **Deploy Consolidated Stack**
```bash
ansible-playbook playbook_ocean_grafana_compose.yaml
```

### **Service Management**
```bash
# Manage entire stack (both Grafana and MySQL)
sudo systemctl start grafana.service
sudo systemctl stop grafana.service
sudo systemctl restart grafana.service

# View logs for both services
journalctl -u grafana.service -f
docker-compose -f /data01/services/grafana/docker-compose.yml logs -f

# View individual service logs
docker logs grafana
docker logs grafana-mysql
```

### **Database Administration**
```bash
# Connect to MySQL from within the stack
docker exec -it grafana-mysql mysql -u root -p

# Check database status
docker exec grafana-mysql mysqladmin ping -u root -p

# View Grafana database
docker exec grafana-mysql mysql -u root -p -e "USE grafana; SHOW TABLES;"
```

## Migration from Separate MySQL

### **Automatic Migration**
The playbook automatically handles migration:
1. **Stops old MySQL service** if it exists
2. **Removes old MySQL directory** (`/data01/services/mysql`)
3. **Creates fresh MySQL instance** integrated with Grafana
4. **Configures private networking**

### **Data Migration**
Since the database was corrupted and empty, no data migration is needed. Grafana will:
1. Connect to fresh MySQL instance
2. Initialize database schema automatically
3. Ready for dashboard creation and configuration

### **Benefits of Fresh Start**
- ✅ **Clean database**: No corruption or permission issues
- ✅ **Proper schema**: Latest Grafana database schema
- ✅ **Consistent permissions**: All files owned by 1001:1001
- ✅ **Optimized configuration**: MySQL tuned for Grafana workload

## Troubleshooting

### **Service Dependencies**
If Grafana fails to start:
1. Check if MySQL is healthy: `docker exec grafana-mysql mysqladmin ping -u root -p`
2. View MySQL startup logs: `docker logs grafana-mysql`
3. Verify network connectivity: `docker exec grafana ping mysql`

### **Database Connection Issues**
```bash
# Test database connection from Grafana container
docker exec grafana mysql -h mysql -u grafana -p

# Check MySQL user permissions
docker exec grafana-mysql mysql -u root -p -e "SHOW GRANTS FOR 'grafana'@'%';"
```

### **Network Connectivity**
```bash
# List networks
docker network ls

# Inspect internal network
docker network inspect grafana_grafana_internal

# Test network connectivity
docker exec grafana ping mysql
```

## Comparison with Previous Architecture

### **Before: Separate Services**
```
nginx → grafana (web_proxy network)
grafana → mysql (mysql_legacy network)
```
- 2 separate docker-compose stacks
- 2 systemd services to manage
- MySQL accessible to other services
- Complex network topology

### **After: Consolidated Stack**
```
nginx → grafana → mysql (web_proxy + grafana_internal)
```
- 1 docker-compose stack
- 1 systemd service to manage
- MySQL private to Grafana only
- Simplified network topology

## Benefits Summary

- ✅ **Simpler Management**: Single deployment and service
- ✅ **Better Security**: MySQL isolated and private
- ✅ **Cleaner Architecture**: Purpose-built for Grafana
- ✅ **Reduced Resources**: Optimized MySQL configuration
- ✅ **Easier Troubleshooting**: Single stack to debug
- ✅ **Consistent Permissions**: All files use 1001:1001

This consolidated approach provides a more maintainable and secure monitoring infrastructure while reducing operational complexity.
