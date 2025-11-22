#!/bin/bash

# Script to validate all Ansible playbooks
cd /Users/terrac/Projects/bluefishorsale/homelab || exit 1

echo "=== Validating All Playbooks ==="
echo ""

failed_playbooks=()
passed_playbooks=()

playbooks=(
    "playbooks/00_site.yaml"
    "playbooks/01_base_system.yaml"
    "playbooks/02_core_infrastructure.yaml"
    "playbooks/03_ocean_services.yaml"
    "playbooks/individual/base/io_cpu_ups.yaml"
    "playbooks/individual/base/packages.yaml"
    "playbooks/individual/base/ramdisk.yaml"
    "playbooks/individual/base/tz_sysctl_udev_logging.yaml"
    "playbooks/individual/base/unattended_upgrade.yaml"
    "playbooks/individual/base/users.yaml"
    "playbooks/individual/core/network/ethtool.yaml"
    "playbooks/individual/core/network/qdisc.yaml"
    "playbooks/individual/core/services/deb12_docker.yaml"
    "playbooks/individual/core/services/dhcp_ddns.yaml"
    "playbooks/individual/core/services/dns.yaml"
    "playbooks/individual/core/services/docker.yaml"
    "playbooks/individual/core/services/pi_hole.yaml"
    "playbooks/individual/core/storage/dell_perc_raid.yaml"
    "playbooks/individual/infrastructure/create_runner_vm.yaml"
    "playbooks/individual/infrastructure/docker_ce.yaml"
    "playbooks/individual/infrastructure/gcloud_sdk.yaml"
    "playbooks/individual/infrastructure/github_docker_runners.yaml"
    "playbooks/individual/infrastructure/github_runner_token_check.yaml"
    "playbooks/individual/infrastructure/github_runner_token_update_vault.yaml"
    "playbooks/individual/infrastructure/gitlab_docker.yaml"
    "playbooks/individual/infrastructure/gitlab_packages.yaml"
    "playbooks/individual/infrastructure/nfs_server.yaml"
    "playbooks/individual/infrastructure/node_exporter.yaml"
    "playbooks/individual/infrastructure/nvidia_containerd.yaml"
    "playbooks/individual/infrastructure/pki_tools.yaml"
    "playbooks/individual/infrastructure/proxmox_qemu_agent.yaml"
    "playbooks/individual/infrastructure/raspberry_pi.yaml"
    "playbooks/individual/infrastructure/update_github_token_in_vault.yaml"
    "playbooks/individual/ocean/ai/comfyui.yaml"
    "playbooks/individual/ocean/ai/llamacpp.yaml"
    "playbooks/individual/ocean/ai/n8n.yaml"
    "playbooks/individual/ocean/ai/open_webui.yaml"
    "playbooks/individual/ocean/media/bazarr.yaml"
    "playbooks/individual/ocean/media/nzbget.yaml"
    "playbooks/individual/ocean/media/overseerr.yaml"
    "playbooks/individual/ocean/media/plex.yaml"
    "playbooks/individual/ocean/media/prowlarr.yaml"
    "playbooks/individual/ocean/media/radarr.yaml"
    "playbooks/individual/ocean/media/sonarr.yaml"
    "playbooks/individual/ocean/media/tautulli.yaml"
    "playbooks/individual/ocean/media/tdarr.yaml"
    "playbooks/individual/ocean/monitoring/grafana.yaml"
    "playbooks/individual/ocean/monitoring/prometheus.yaml"
    "playbooks/individual/ocean/web/cloudflare_ddns.yaml"
    "playbooks/individual/ocean/web/cloudflared.yaml"
    "playbooks/individual/ocean/web/nextcloud.yaml"
    "playbooks/individual/ocean/web/nginx.yaml"
    "playbooks/individual/ocean/web/payloadcms.yaml"
    "playbooks/individual/ocean/web/strapi.yaml"
    "playbooks/individual/ocean/web/tinacms.yaml"
)

for playbook in "${playbooks[@]}"; do
    echo "Checking: $playbook"
    if ansible-playbook --syntax-check "$playbook" 2>&1 | grep -q "ERROR\|Could not find or access"; then
        echo "  ❌ FAILED"
        failed_playbooks+=("$playbook")
        ansible-playbook --syntax-check "$playbook" 2>&1
        echo ""
    else
        echo "  ✅ PASSED"
        passed_playbooks+=("$playbook")
    fi
done

echo ""
echo "=== Summary ==="
echo "Passed: ${#passed_playbooks[@]}"
echo "Failed: ${#failed_playbooks[@]}"

if [ ${#failed_playbooks[@]} -gt 0 ]; then
    echo ""
    echo "Failed playbooks:"
    for playbook in "${failed_playbooks[@]}"; do
        echo "  - $playbook"
    done
    exit 1
fi
