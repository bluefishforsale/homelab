# 🔧 Common Issues Troubleshooting Guide

## Overview

This guide covers the most frequently encountered issues in your homelab environment and their solutions.

## 🌐 Network Issues

### DNS Resolution Problems

#### Symptoms
- Services can't resolve hostnames
- Web interfaces inaccessible by domain name
- Internal service discovery failing

#### Diagnostics
```bash
# Test DNS resolution
nslookup gitlab.home 192.168.1.2
dig @192.168.1.2 ocean.home
host plex.terrac.com

# Check DNS server status
systemctl status bind9
systemctl status pihole-FTL

# Verify DNS configuration
cat /etc/resolv.conf
cat /etc/systemd/resolved.conf
```

#### Solutions
```bash
# Restart DNS services
sudo systemctl restart bind9
sudo systemctl restart pihole-FTL

# Flush DNS caches
sudo systemctl flush-dns
sudo resolvectl flush-caches

# Fix DNS configuration
echo "nameserver 192.168.1.2" | sudo tee /etc/resolv.conf
echo "nameserver 192.168.1.9" | sudo tee -a /etc/resolv.conf
```

### Network Connectivity Issues

#### No Internet Access
```bash
# Check gateway connectivity
ping 192.168.1.1

# Check external DNS
ping 8.8.8.8
ping 1.1.1.1

# Check routing table
ip route show
ip route get 8.8.8.8

# Fix routing if needed
sudo ip route add default via 192.168.1.1
```

#### Interface Problems
```bash
# Check interface status
ip link show
ip addr show

# Restart networking
sudo systemctl restart networking
sudo netplan apply  # Ubuntu with netplan

# Reload network configuration
sudo ifreload -a  # Proxmox/Debian
```

### DHCP Issues

#### Clients Not Getting IP Addresses
```bash
# Check DHCP server status
systemctl status isc-dhcp-server

# Check DHCP logs
journalctl -u isc-dhcp-server -f

# Test DHCP lease
sudo dhclient -v eth0

# Restart DHCP server
sudo systemctl restart isc-dhcp-server
```

## 🖥️ Virtualization Issues

### VM Won't Start

#### Common Causes & Solutions
```bash
# Check VM configuration
qm config 143

# Look for locks
qm unlock 143

# Check storage availability
pvesm status

# Check resource availability
pvesh get /nodes/proxmox01/status

# Start with more verbose output
qm start 143 --debug
```

#### Storage Issues
```bash
# Check Ceph cluster health
ceph -s
ceph health detail

# Check disk space
df -h
pvesm list local-lvm

# Clean up if needed
qm disk-cleanup 143
```

### Performance Issues

#### High CPU/Memory Usage
```bash
# Check resource usage
htop
iotop
vmstat 1

# Check VM resource allocation
qm config 143 | grep -E "memory|cores|cpu"

# Adjust VM resources if needed
qm set 143 --memory 8192
qm set 143 --cores 4
```

#### Storage Performance
```bash
# Check disk I/O
iotop -a
iostat -x 1

# Test storage performance
dd if=/dev/zero of=/tmp/test bs=1G count=1 oflag=direct
fio --name=test --ioengine=libaio --rw=randread --bs=4k --size=1G --direct=1
```

## 🐳 Container Issues

### Docker Service Problems

#### Docker Won't Start
```bash
# Check Docker status
systemctl status docker

# Check Docker logs
journalctl -u docker -f

# Restart Docker
sudo systemctl restart docker

# Check Docker daemon configuration
cat /etc/docker/daemon.json
```

#### Container Won't Start
```bash
# Check container status
docker ps -a

# View container logs
docker logs container-name
docker logs --tail 50 container-name

# Check resource usage
docker stats

# Recreate container
docker-compose down
docker-compose up -d
```

### Docker Compose Issues

#### Services Failing to Start
```bash
# Check compose file syntax
docker-compose config

# View service logs
docker-compose logs service-name
docker-compose logs -f  # Follow logs

# Check environment variables
docker-compose exec service-name env

# Rebuild and restart
docker-compose down
docker-compose build --no-cache
docker-compose up -d
```

#### Network Connectivity Between Services
```bash
# Check Docker networks
docker network ls
docker network inspect bridge-name

# Test connectivity between containers
docker exec container1 ping container2
docker exec container1 nslookup container2

# Recreate network if needed
docker-compose down
docker network prune
docker-compose up -d
```

## 🎮 GPU Issues

### NVIDIA Driver Problems

#### GPU Not Detected
```bash
# Check if GPU is visible
lspci | grep -i nvidia

# Check driver status
nvidia-smi
lsmod | grep nvidia

# Reinstall drivers if needed
sudo apt remove --purge nvidia-*
sudo apt install nvidia-driver-470
sudo reboot
```

#### CUDA Issues
```bash
# Check CUDA version
nvcc --version
nvidia-smi | grep "CUDA Version"

# Test CUDA functionality
docker run --rm --gpus all nvidia/cuda:11.8-base-ubuntu20.04 nvidia-smi

# Fix CUDA path
export PATH=/usr/local/cuda/bin:$PATH
export LD_LIBRARY_PATH=/usr/local/cuda/lib64:$LD_LIBRARY_PATH
```

### GPU Performance Issues

#### Thermal Throttling
```bash
# Check GPU temperature
nvidia-smi --query-gpu=temperature.gpu --format=csv,noheader,nounits

# Check throttle reasons
nvidia-smi --query-gpu=clocks_throttle_reasons.active --format=csv

# Reduce power limit if overheating
sudo nvidia-smi -pl 65  # Reduce from 75W to 65W
```

## 💾 Storage Issues

### ZFS Problems

#### Pool Import Issues
```bash
# Check importable pools
zpool import

# Force import if needed
zpool import -f tank

# Check pool status
zpool status -v

# Clear errors after fixing issues
zpool clear tank
```

#### Disk Failures
```bash
# Check for failed disks
zpool status -x

# Replace failed disk
zpool replace tank /dev/old-disk /dev/new-disk

# Monitor resilver progress
watch 'zpool status tank'
```

### Ceph Storage Issues

#### Cluster Health Problems
```bash
# Check cluster health
ceph health detail
ceph -s

# Check OSD status
ceph osd tree
ceph osd stat

# Restart problematic OSDs
sudo systemctl restart ceph-osd@0
```

#### Performance Issues
```bash
# Check cluster performance
ceph osd perf
rados bench 60 write

# Check for slow requests
ceph osd dump | grep slow

# Review configuration
ceph daemon osd.0 config show
```

## 🔐 Security & Authentication Issues

### SSH Access Problems

#### Can't Connect via SSH
```bash
# Check SSH service status
systemctl status sshd

# Check SSH configuration
sudo sshd -T

# Check firewall rules
ufw status
iptables -L

# Check SSH logs
journalctl -u ssh -f
```

#### Key Authentication Issues
```bash
# Check SSH key permissions
ls -la ~/.ssh/
chmod 700 ~/.ssh
chmod 600 ~/.ssh/id_rsa
chmod 644 ~/.ssh/id_rsa.pub

# Check authorized_keys
cat ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys

# Test SSH key
ssh -vvv user@host
```

### SSL/TLS Certificate Issues

#### Certificate Expired
```bash
# Check certificate expiry
openssl x509 -in /path/to/cert.pem -text -noout | grep "Not After"

# Renew Let's Encrypt certificates
certbot renew --dry-run
certbot renew

# Restart services using certificates
systemctl restart nginx
systemctl restart apache2
```

### Ansible Vault Issues

#### Can't Decrypt Vault
```bash
# Check vault password file
ls -la ~/.ansible_vault_pass
chmod 600 ~/.ansible_vault_pass

# Test vault decryption
ansible-vault view vault_secrets.yaml

# Set environment variable
export ANSIBLE_VAULT_PASSWORD_FILE=~/.ansible_vault_pass

# Debug vault issues
ansible-playbook playbook.yaml --ask-vault-pass -vvv
```

## 🔧 Service-Specific Issues

### Cloudflare Tunnel Issues

#### Tunnel Connection Problems
```bash
# Check cloudflared status
systemctl status cloudflared

# Check tunnel configuration
cloudflared tunnel info tunnel-name

# Test tunnel connectivity
cloudflared tunnel run --url http://localhost:8080 tunnel-name

# Restart tunnel service
sudo systemctl restart cloudflared
```

### GitLab Issues

#### GitLab Won't Start
```bash
# Check GitLab status
gitlab-ctl status

# Check GitLab logs
gitlab-ctl tail

# Reconfigure GitLab
gitlab-ctl reconfigure
gitlab-ctl restart

# Check disk space (GitLab needs significant space)
df -h /var/opt/gitlab
```

#### Git Operations Failing
```bash
# Check GitLab configuration
gitlab-ctl show-config

# Check Git over SSH
ssh -T git@gitlab.home

# Reset user passwords
gitlab-rails console -e production
# user = User.find_by(username: 'root')
# user.password = 'new_password'
# user.save!
```

### Plex Issues

#### Plex Server Unreachable
```bash
# Check Plex service status
docker ps | grep plex
docker logs plex

# Check Plex configuration
docker exec plex cat /config/Library/Application\ Support/Plex\ Media\ Server/Preferences.xml

# Restart Plex container
docker restart plex
```

#### Transcoding Issues
```bash
# Check GPU access in Plex container
docker exec plex nvidia-smi

# Check transcoding logs
docker exec plex tail -f /config/Library/Application\ Support/Plex\ Media\ Server/Logs/Plex\ Media\ Scanner.log

# Verify hardware acceleration settings
# Plex Settings > Server > Transcoder > Use hardware acceleration
```

### N8N Workflow Issues

#### Database Connection Problems
```bash
# Check PostgreSQL status
docker ps | grep n8n-db
docker logs n8n-db

# Check N8N database connectivity
docker exec n8n-db pg_isready -U n8n

# Reset database connection
docker restart n8n
docker restart n8n-db
```

## 📊 Monitoring & Alerting Issues

### Grafana Access Problems

#### Can't Access Grafana Web Interface
```bash
# Check Grafana service status
docker ps | grep grafana
docker logs grafana

# Check port binding
netstat -tlnp | grep 8910
ss -tlnp | grep 8910

# Check firewall rules
ufw status
iptables -L | grep 8910
```

#### Data Source Issues
```bash
# Test Prometheus connectivity
curl http://192.168.1.143:9090/metrics

# Check Grafana data source configuration
# Grafana > Configuration > Data Sources

# Check Grafana logs for errors
docker logs grafana | grep -i error
```

### Prometheus Scraping Issues

#### Targets Down
```bash
# Check Prometheus targets
curl http://192.168.1.143:9090/targets

# Check target endpoint directly
curl http://target-ip:port/metrics

# Check network connectivity
ping target-ip
telnet target-ip port

# Check Prometheus configuration
docker exec prometheus cat /etc/prometheus/prometheus.yml
```

## 🚨 Emergency Recovery Procedures

### System Won't Boot

#### Proxmox Boot Issues
1. Boot from rescue media
2. Mount root filesystem
3. Check `/etc/fstab` for errors
4. Fix network configuration in `/etc/network/interfaces`
5. Repair GRUB if needed: `grub-install /dev/sda`

#### VM Boot Issues
```bash
# Check VM configuration
qm config vmid

# Boot from rescue ISO
qm set vmid --ide2 local:iso/rescue.iso,media=cdrom
qm start vmid

# Access VM console
qm monitor vmid
```

### Data Recovery

#### Accidental File Deletion
```bash
# ZFS snapshot recovery
zfs list -t snapshot | grep dataset
zfs rollback dataset@snapshot

# Ceph recovery (if versioning enabled)
rados -p pool-name listomapvals object-name
```

#### Database Corruption
```bash
# MySQL recovery
mysqlcheck --repair --all-databases

# PostgreSQL recovery
pg_resetwal /var/lib/postgresql/data
```

## 🔍 Diagnostic Tools

### Network Diagnostics
```bash
# Comprehensive network test
ping -c 4 google.com
traceroute google.com
mtr --report google.com
nmap -sn 192.168.1.0/24
```

### Performance Analysis
```bash
# System performance overview
htop
iotop -a
nethogs
bandwhich
```

### Log Analysis
```bash
# Centralized log viewing
journalctl -f --all
multitail /var/log/syslog /var/log/auth.log
dmesg -w
```

### Hardware Diagnostics
```bash
# Hardware health check
sensors
smartctl -a /dev/sda
memtest86+ (boot from USB)
stress-ng --cpu 4 --timeout 60s
```

This troubleshooting guide addresses the most common issues encountered in your homelab environment, providing systematic approaches to diagnosis and resolution while maintaining the idempotency principles essential to your infrastructure.
