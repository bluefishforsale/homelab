# Common Issues Troubleshooting Guide

This guide covers frequently encountered issues in the homelab environment.

**Quick Reference:**

| Host | IP | Purpose |
|------|----|---------|
| ocean | 192.168.1.143 | Media/AI services, Prometheus, Grafana |
| dns01 | 192.168.1.2 | BIND9 DNS |
| pihole | 192.168.1.9 | DNS filtering |
| node005 | 192.168.1.105 | Proxmox hypervisor |
| node006 | 192.168.1.106 | Proxmox hypervisor (GPU) |

---

## Network Issues

### DNS Resolution Problems

```bash
# Test DNS resolution
nslookup gitlab.home 192.168.1.2
dig @192.168.1.2 ocean.home

# Check DNS server status (dns01)
ssh debian@192.168.1.2 "systemctl status bind9"

# Check pihole status
ssh debian@192.168.1.9 "systemctl status pihole-FTL"

# Restart DNS services
ssh debian@192.168.1.2 "sudo systemctl restart bind9"
ssh debian@192.168.1.9 "sudo systemctl restart pihole-FTL"

# Ansible: Redeploy DNS
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/core/services/dns.yaml --ask-vault-pass
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

## Virtualization Issues

### VM Won't Start

```bash
# Check VM configuration (on Proxmox node)
ssh root@192.168.1.106 "qm config VMID"

# Look for locks
ssh root@192.168.1.106 "qm unlock VMID"

# Check storage availability
ssh root@192.168.1.106 "pvesm status"

# Check resource availability
ssh root@192.168.1.106 "pvesh get /nodes/node006/status"

# Start with debug output
ssh root@192.168.1.106 "qm start VMID --debug"
```

#### VM Storage Issues
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

## Container Issues

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

## GPU Issues

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

## Storage Issues

### ZFS Problems (ocean VM)

```bash
# Check pool status
ssh terrac@192.168.1.143 "zpool status data01"

# Force import if needed
ssh terrac@192.168.1.143 "sudo zpool import -f data01"

# Check for errors
ssh terrac@192.168.1.143 "zpool status -x"

# Clear errors after fixing
ssh terrac@192.168.1.143 "sudo zpool clear data01"

# Monitor resilver progress
ssh terrac@192.168.1.143 "watch 'zpool status data01'"
```

See `docs/operations/zfs-disk-replacement.md` for disk replacement procedures.

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

## Security & Authentication Issues

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

```bash
# Test vault decryption
ansible-vault view vault/secrets.yaml --ask-vault-pass

# Edit vault
ansible-vault edit vault/secrets.yaml --ask-vault-pass

# Run playbook with vault
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/00_site.yaml --ask-vault-pass

# Debug vault issues
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/00_site.yaml --ask-vault-pass -vvv
```

## Service-Specific Issues

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

### Plex Issues (ocean)

```bash
# Check Plex service
ssh terrac@192.168.1.143 "docker ps | grep plex"
ssh terrac@192.168.1.143 "systemctl status plex"

# View logs
ssh terrac@192.168.1.143 "docker logs plex --tail 50"

# Restart Plex
ssh terrac@192.168.1.143 "sudo systemctl restart plex"

# Check GPU access
ssh terrac@192.168.1.143 "docker exec plex nvidia-smi"

# Redeploy via Ansible
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/media/plex.yaml --ask-vault-pass
```

## Monitoring & Alerting Issues (ocean)

### Grafana (port 8910)

```bash
# Check Grafana service
ssh terrac@192.168.1.143 "docker ps | grep grafana"
ssh terrac@192.168.1.143 "systemctl status grafana"

# Check logs
ssh terrac@192.168.1.143 "docker logs grafana --tail 50"

# Restart Grafana
ssh terrac@192.168.1.143 "sudo systemctl restart grafana"

# Access: http://192.168.1.143:8910

# Redeploy
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/monitoring/grafana_compose.yaml --ask-vault-pass
```

### Prometheus (port 9090)

```bash
# Check Prometheus
ssh terrac@192.168.1.143 "docker ps | grep prometheus"
ssh terrac@192.168.1.143 "systemctl status prometheus"

# Check targets
curl http://192.168.1.143:9090/api/v1/targets | jq '.data.activeTargets[] | {job: .labels.job, health: .health}'

# Check specific exporter
curl http://192.168.1.143:9100/metrics | head -20  # node-exporter
curl http://192.168.1.143:8912/metrics | head -20  # cadvisor

# Redeploy
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/monitoring/prometheus.yaml --ask-vault-pass
```

## Emergency Recovery Procedures

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

## Diagnostic Tools

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
