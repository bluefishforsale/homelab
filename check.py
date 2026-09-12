import re

roadmap_content = open('ROADMAP.md').read()

evidence_playbooks = """
agentbox/agentbox
base/authorized_keys
base/fail2ban
base/gpu_power
base/io_cpu_ups
base/logging
base/mask_deprecated_units
base/packages
base/ramdisk
base/tz_sysctl_udev
base/unattended_upgrade
base/users
core/network/ethtool
core/network/qdisc
core/network/vm_multiqueue
core/services/deb12_docker
core/services/dns01_vm
core/services/dns02_vm
core/services/dns_ha_stack
core/services/docker
core/services/pi_hole
core/storage/dell_perc_raid
infrastructure/create_agentbox_vm
infrastructure/create_registry_cache_vm
infrastructure/create_runner_vm
infrastructure/docker_ce
infrastructure/fail2ban_exporter
infrastructure/gcloud_sdk
infrastructure/github_docker_runners
infrastructure/github_runner_token_check
infrastructure/github_runner_token_update_vault
infrastructure/ipmi_exporter
infrastructure/nfs_server
infrastructure/node_exporter
infrastructure/nvidia_containerd
infrastructure/ocean_cpu_host
infrastructure/pki_tools
infrastructure/proxmox_qemu_agent
infrastructure/proxmox_repos
infrastructure/pve_exporter
infrastructure/raspberry_pi
infrastructure/registry_cache
infrastructure/update_github_token_in_vault
ocean/ai/llamacpp
ocean/ai/mem0
ocean/ai/open_webui
ocean/ai/paia
ocean/ai/terminalbench
ocean/ai/terminalbench_model
ocean/ai/terminalbench_run
ocean/data01_zfs
ocean/gpu-test
ocean/loki
ocean/media/bazarr
ocean/media/jellyfin
ocean/media/nzbget
ocean/media/overseerr
ocean/media/plex
ocean/media/prowlarr
ocean/media/radarr
ocean/media/sonarr
ocean/media/tautulli
ocean/media/tdarr
ocean/monitoring/cloudflare_exporter
ocean/monitoring/grafana_compose
ocean/monitoring/ndt_speedtest
ocean/monitoring/ntfy
ocean/monitoring/nvidia_dcgm
ocean/monitoring/nvidia_power_limit
ocean/monitoring/prometheus
ocean/monitoring/unpoller
ocean/network/certbot
ocean/network/cloudflare_ddns
ocean/network/cloudflared
ocean/network/mail_relay
ocean/network/nginx_compose
ocean/services/afp
ocean/services/audible_downloader
ocean/services/blog_saetnere_com_wp
ocean/services/blog_terrac_com_static
ocean/services/gethomepage
ocean/services/globalview
ocean/services/globalview_backend
ocean/services/globalview_stack
ocean/services/homeassistant_compose
ocean/services/my_ta_jose
ocean/services/photonic_inventory
ocean/services/ra_mirror
ocean/services/terrac_com
""".strip().split('\n')

evidence_targets = """
agentbox-otelcol
alertmanager
blackbox-api
blackbox-dns
blackbox-http-4xx
blackbox-http-status
blackbox-http
blackbox-icmp
blackbox-internal
blackbox-tcp-plex
blackbox-tcp
blackbox-tls
cadvisor
cloudflare-exporter-terrac
fail2ban-exporter
ipmi
jellyfin-exporter
kea-exporter
llamacpp
mtr-exporter
my-ta-jose
ndt-exporter
node_exporter
nvidia-exporter
nvidia-gpu-exporter
nzbget-exporter
paia
plex-exporter
powerdns
process-exporter
prometheus
promtail
prowlarr-exporter
pve
radarr-exporter
registry-cache
smart-exporter
sonarr-exporter
tautulli-exporter
unpoller
""".strip().split('\n')

missing_playbooks = []
for pb in evidence_playbooks:
    name = pb.split('/')[-1]
    if name not in roadmap_content and name.replace('_', ' ') not in roadmap_content and name.replace('_', '-') not in roadmap_content:
        missing_playbooks.append(pb)

missing_targets = []
for t in evidence_targets:
    if t not in roadmap_content and t.replace('-', ' ') not in roadmap_content and t.replace('-', '_') not in roadmap_content:
        missing_targets.append(t)

print("Missing playbooks:", missing_playbooks)
print("Missing targets:", missing_targets)

