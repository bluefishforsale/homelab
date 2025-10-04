# 🖥️ Dell Hardware Operations Guide

## Overview

This guide covers management, monitoring, and maintenance of Dell PowerEdge servers in your homelab environment.

## 📋 Hardware Monitoring

### iDRAC (Integrated Dell Remote Access Controller)

#### Initial Setup
```bash
# Access iDRAC web interface
# Default: https://server-ip:443
# Default credentials: root/calvin (change immediately!)

# Configure iDRAC network settings
racadm config -g cfgLanNetworking -o cfgNicIpAddress 192.168.1.106
racadm config -g cfgLanNetworking -o cfgNicNetmask 255.255.255.0
racadm config -g cfgLanNetworking -o cfgNicGateway 192.168.1.1
racadm config -g cfgLanNetworking -o cfgNicUseDHCP 0

# Set strong password
racadm set iDRAC.Users.2.Password "YourStrongPassword"
racadm set iDRAC.Users.2.Enable 1
```

#### System Health Monitoring
```bash
# Check system health via racadm
racadm getsysinfo
racadm getsvctag
racadm getsensorinfo

# Temperature monitoring
racadm get System.ThermalSettings
racadm getsensorinfo | grep -i temp

# Fan monitoring
racadm getsensorinfo | grep -i fan
racadm get System.ThermalSettings.FanSpeedOffset

# Power monitoring
racadm getpwrbudget
racadm getpwrconsumption
```

### OpenManage Linux Agent (OMSA)

#### Installation
```bash
# Add Dell repository
echo 'deb http://linux.dell.com/repo/community/software/openmanage/10500/focal focal main' | sudo tee /etc/apt/sources.list.d/linux.dell.com.sources.list

# Add GPG key
curl -fsSL https://linux.dell.com/repo/pgp_pubkeys/0x1285491434D8786F.asc | sudo gpg --dearmor -o /etc/apt/trusted.gpg.d/dell.gpg

# Install OMSA
sudo apt update
sudo apt install srvadmin-all

# Start services
sudo systemctl enable --now dsm_om_connsvc
sudo systemctl enable --now dataeng
```

#### OMSA Commands
```bash
# System information
omreport system summary
omreport system version

# Hardware inventory
omreport chassis info
omreport chassis processors
omreport chassis memory
omreport chassis pwrsupplies

# Storage information
omreport storage controller
omreport storage pdisk controller=0
omreport storage vdisk controller=0

# Network information
omreport chassis nics
omreport chassis remoteaccess
```

## 🔧 RAID Management

### PERC RAID Controller

#### Basic RAID Operations
```bash
# List controllers
omreport storage controller

# List physical disks
omreport storage pdisk controller=0

# List virtual disks
omreport storage vdisk controller=0

# Create RAID array
omconfig storage controller action=createvdisk \
  controller=0 \
  raidlevel=r1 \
  size=max \
  pdisk=0:1:0,0:1:1

# Delete virtual disk (CAREFUL!)
omconfig storage controller action=deletevdisk controller=0 vdisk=0
```

#### RAID Monitoring
```bash
# Check RAID status
omreport storage controller controller=0
omreport storage vdisk controller=0

# Monitor rebuild progress
watch 'omreport storage pdisk controller=0 | grep -A5 "Rebuild"'

# Check for failed disks
omreport storage pdisk controller=0 | grep -i failed
```

#### Disk Replacement
```bash
# Identify failed disk
omreport storage pdisk controller=0 | grep -i "predictive\|failed"

# Prepare disk for removal (if supported)
omconfig storage pdisk action=offline controller=0 pdisk=0:1:2

# After physical replacement
omconfig storage pdisk action=online controller=0 pdisk=0:1:2

# Start rebuild (usually automatic)
omconfig storage pdisk action=rebuild controller=0 pdisk=0:1:2
```

## 🌡️ Thermal Management

### Temperature Monitoring
```bash
# Current temperatures
racadm getsensorinfo | grep -E "Temp|Temperature"
ipmitool sdr list | grep -i temp

# Set temperature thresholds
racadm set System.ThermalSettings.ThermalProfile "Maximum Performance"
racadm set System.ThermalSettings.FanSpeedOffset 0  # 0-4, higher = more aggressive
```

### Fan Control
```bash
# Check fan speeds
racadm getsensorinfo | grep -i fan
ipmitool sdr list | grep -i fan

# Set fan profile
racadm set System.ThermalSettings.ThermalProfile "Optimal"
# Options: "Optimal", "Maximum Performance", "Acoustic"

# Manual fan control (advanced)
ipmitool raw 0x30 0x30 0x01 0x00  # Enable manual control
ipmitool raw 0x30 0x30 0x02 0xff 0x40  # Set fan to ~25%
```

### Thermal Events
```bash
# Check thermal events
racadm getsel | grep -i thermal
dmesg | grep -i thermal

# Set thermal shutdown threshold
racadm set System.ThermalSettings.ThermalShutdown "Enabled"
```

## ⚡ Power Management

### Power Monitoring
```bash
# Current power consumption
racadm getpwrconsumption
ipmitool dcmi power reading

# Power budget settings
racadm getpwrbudget
racadm setpwrbudget watts=500  # Set 500W budget

# Power supply status
omreport chassis pwrsupplies
racadm get System.Power.PSUCapacity
```

### Power Profiles
```bash
# Set power profile
racadm set System.Power.PowerProfile "MaxPerf"
# Options: "MaxPerf", "BalancedPerf", "PowerSaver"

# CPU power management
racadm set BIOS.SysProfileSettings.SysProfile "PerfPerWatt(DAPC)"
# Options: "PerfPerWatt(DAPC)", "PerfPerWatt(OS)", "Performance"
```

### UPS Integration
```bash
# Install NUT (Network UPS Tools)
sudo apt install nut nut-client

# Configure for Dell UPS
cat > /etc/nut/ups.conf << EOF
[dell-ups]
    driver = usbhid-ups
    port = auto
    desc = "Dell UPS"
EOF

# Start NUT services
sudo systemctl enable --now nut-server
sudo systemctl enable --now nut-client
```

## 💾 Firmware Management

### Firmware Updates
```bash
# Check current firmware versions
racadm get System.BIOS.BIOSReleaseDate
racadm get iDRAC.Info.Version

# Download Dell Repository Manager (DRM)
wget https://downloads.dell.com/FOLDER07107368M/1/DRMInstaller_4.4.0_A00.bin
chmod +x DRMInstaller_4.4.0_A00.bin
sudo ./DRMInstaller_4.4.0_A00.bin

# Update firmware (use Dell Update Packages)
sudo dsu --non-interactive
sudo dsu --preview  # Preview available updates
```

### BIOS Configuration
```bash
# Export BIOS configuration
racadm get BIOS > bios-config-backup.cfg

# Import BIOS configuration
racadm set -f bios-config.cfg

# Key BIOS settings for virtualization
racadm set BIOS.ProcSettings.LogicalProc "Enabled"      # Hyperthreading
racadm set BIOS.ProcSettings.Virtualization "Enabled"   # VT-x
racadm set BIOS.ProcSettings.VtForDirectIo "Enabled"    # VT-d
racadm set BIOS.SysProfileSettings.SysProfile "PerfPerWatt(DAPC)"
```

## 🔍 Diagnostics & Troubleshooting

### Hardware Diagnostics
```bash
# Dell diagnostic tools
omreport system summary | grep -i error
omreport system alertlog

# Memory diagnostics
omreport chassis memory | grep -i error
dmidecode -t memory | grep -i error

# Storage diagnostics
omreport storage pdisk controller=0 | grep -i error
smartctl -a /dev/sda  # SMART data for individual disks
```

### Event Log Analysis
```bash
# System Event Log (SEL)
racadm getsel
ipmitool sel list

# Clear SEL (after investigation)
racadm clrsel
ipmitool sel clear

# Linux system logs
journalctl -f | grep -i "dell\|raid\|thermal"
dmesg | grep -i "dell\|raid\|hardware"
```

### Network Troubleshooting
```bash
# Check NIC status
omreport chassis nics
ethtool eth0

# Link aggregation status
cat /proc/net/bonding/bond0

# Network performance testing
iperf3 -s  # Server mode
iperf3 -c server-ip -t 60  # 60-second test
```

## 🏗️ Hardware-Specific Optimizations

### Dell R720/R730 Optimizations
```bash
# Disable unnecessary services
systemctl disable dell-flash-unlock.service
systemctl disable dcism.service

# Optimize for virtualization workloads
echo 'elevator=noop' >> /etc/default/grub  # For SSDs
echo 'intel_idle.max_cstate=1' >> /etc/default/grub  # Reduce latency
update-grub

# Memory optimization
echo 'vm.swappiness=10' >> /etc/sysctl.conf
echo 'vm.dirty_ratio=15' >> /etc/sysctl.conf
echo 'vm.dirty_background_ratio=5' >> /etc/sysctl.conf
```

### Storage Optimizations
```bash
# RAID controller cache settings
omconfig storage controller action=setbgipolicy \
  controller=0 policy=adaptive

# Enable write-back cache (with battery backup)
omconfig storage vdisk action=changepolicy \
  controller=0 vdisk=0 \
  writepolicy=writeback

# Disk alignment for VMs
parted -a optimal /dev/sdb mklabel gpt
parted -a optimal /dev/sdb mkpart primary 0% 100%
```

## 📊 Monitoring Integration

### Prometheus Exporter
```bash
# Install Dell hardware exporter
wget https://github.com/galexrt/dellhw_exporter/releases/download/v1.12.6/dellhw_exporter-1.12.6.linux-amd64.tar.gz
tar xzf dellhw_exporter-1.12.6.linux-amd64.tar.gz
sudo cp dellhw_exporter /usr/local/bin/

# Create systemd service
cat > /etc/systemd/system/dellhw-exporter.service << EOF
[Unit]
Description=Dell Hardware Exporter
After=network.target

[Service]
Type=simple
User=nobody
ExecStart=/usr/local/bin/dellhw_exporter --collectors.omreport.enabled
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl enable --now dellhw-exporter
```

### SNMP Monitoring
```bash
# Configure SNMP on iDRAC
racadm set iDRAC.SNMP.SNMPProtocol "All"
racadm set iDRAC.SNMP.CommunityName "public"
racadm set iDRAC.SNMP.AgentEnable "Enabled"

# Test SNMP
snmpwalk -v2c -c public idrac-ip 1.3.6.1.4.1.674.10892.5.1.3.50.1.8
```

## 🚨 Alert Configuration

### iDRAC Alerts
```bash
# Configure email alerts
racadm set iDRAC.EmailAlert.Enable "Enabled"
racadm set iDRAC.EmailAlert.SMTPServerIPAddress "192.168.1.2"
racadm set iDRAC.EmailAlert.SMTPPort "25"
racadm set iDRAC.EmailAlert.SMTPAuthentication "Disabled"

# Set alert destinations
racadm set iDRAC.EmailAlert.Address.1 "admin@homelab.local"
racadm set iDRAC.EmailAlert.CustomMsg "Dell Server Alert"

# Configure SNMP traps
racadm set iDRAC.SNMPTrap.TrapFormat "SNMPv1"
racadm set iDRAC.SNMPTrap.SNMPv1TrapDestination "192.168.1.143:162"
```

### System Health Scripts
```bash
#!/bin/bash
# /usr/local/bin/dell-health-check.sh

# Check for hardware errors
ERRORS=$(omreport system alertlog | grep -i "error\|critical\|failure" | wc -l)

if [ "$ERRORS" -gt 0 ]; then
    echo "CRITICAL: $ERRORS hardware errors found"
    omreport system alertlog | grep -i "error\|critical\|failure" | tail -5
    exit 2
fi

# Check RAID status
RAID_STATUS=$(omreport storage vdisk controller=0 | grep "Status" | grep -v "Ok")
if [ -n "$RAID_STATUS" ]; then
    echo "WARNING: RAID issues detected"
    echo "$RAID_STATUS"
    exit 1
fi

echo "OK: All hardware checks passed"
exit 0
```

## 🔧 Maintenance Procedures

### Routine Maintenance
```bash
# Monthly hardware health check
omreport system summary
racadm getsysinfo
racadm getsel | tail -20

# Quarterly firmware updates
sudo dsu --non-interactive --preview
# Review and apply updates

# Fan cleaning (physical)
# 1. Power down server
# 2. Remove power cables
# 3. Use compressed air to clean fans and heatsinks
# 4. Check for dust buildup in air filters
```

### Disk Replacement Procedure
```bash
# 1. Identify failing disk
omreport storage pdisk controller=0 | grep -i "predictive\|failed"

# 2. Note disk location (slot number)
# 3. If supported, blink LED for identification
omconfig storage pdisk action=blink controller=0 pdisk=0:1:2

# 4. Hot-swap the disk (server can remain online)
# 5. Wait for automatic rebuild
watch 'omreport storage vdisk controller=0'

# 6. Verify rebuild completion
omreport storage pdisk controller=0 | grep "State.*Online"
```

## 📅 Maintenance Schedule

### Daily Monitoring
- [ ] Check iDRAC dashboard for alerts
- [ ] Monitor system temperatures
- [ ] Verify all services are running

### Weekly Tasks
- [ ] Review system event logs
- [ ] Check RAID array status
- [ ] Monitor power consumption trends
- [ ] Verify backup completion

### Monthly Tasks
- [ ] Run comprehensive hardware diagnostics
- [ ] Clean system event logs (after review)
- [ ] Check firmware update availability
- [ ] Physical inspection (dust, connections)

### Quarterly Tasks
- [ ] Apply firmware updates
- [ ] Deep clean hardware (compressed air)
- [ ] Review and update monitoring thresholds
- [ ] Test UPS functionality
- [ ] Update hardware documentation

This Dell hardware operations guide ensures your PowerEdge servers run reliably while following the idempotency and automation principles essential to your homelab environment.
