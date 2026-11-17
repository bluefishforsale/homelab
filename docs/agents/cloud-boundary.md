# Cloud infrastructure lives in terraform-root

**Ansible in this repo no longer creates cloud infrastructure.** Cloudflare, GCP
and Azure resources are owned by
[`bluefishforsale/terraform-root`](https://github.com/bluefishforsale/terraform-root)
(local checkout `../terraform-root`), applied by its GitHub Actions on merge to
`main`, with a daily drift check. If a change here needs a cloud resource to
exist, that resource gets added in terraform-root first, merged and applied,
and only then does the homelab change that depends on it ship.

What terraform-root owns:

- **Cloudflare:** the three zones (terrac.com, saetnere.com,
  radiantatmospheres.com) and every DNS record in them, Cloudflare Tunnels
  (the tunnel objects, including the per-host `ssh-<host>` ones), Access apps
  and the reusable Access policies, the Zero Trust org and identity provider,
  zone rulesets (rate limits, redirects, cache rules, security header
  transforms, WAF custom rules), zone TLS/security settings, Email Routing,
  R2 buckets and Pages projects.
- **GCP** (`homelab-390805`): service accounts, the `homelab-db-backups`
  bucket and its IAM, KMS, enabled APIs, project IAM.
- **Azure:** the Artifact Signing account and app registrations for
  signal-agent.

What stays here, because it's host config or runtime operations rather than
creating cloud resources:

- Running `cloudflared` on each host. Its rendered `/etc/cloudflared/config.yaml`
  holds process settings only; every tunnel's ingress is remote config in
  terraform-root (`cloudflare-tunnel-configs.tf`).
- The DDNS timer rewriting `home.terrac.com`'s IP (terraform-root ignores that
  record's content on purpose), certbot's short-lived DNS-01 TXT records, and
  cache purges.
- Using cloud resources: db-backup writing to `gs://homelab-db-backups`, the
  cloudflare exporter reading analytics.

Nothing here creates Cloudflare resources any more. `cloudflared.yaml` only
looks up the host's `ssh-<host>` tunnel and fetches its token, and it fails if
the tunnel is missing rather than creating it, so a new host's tunnel, CNAME,
ingress and Access app come from terraform-root first. The old Cloudflare
write scripts (`cloudflare-harden.py`, `cloudflare-waf.py`,
`cloudflare-pages-domain.sh`, `cf.sh dns-add`) are deleted; their settings
live in terraform-root's `cloudflare-<zone>-config.tf`.

Proxmox VMs are terraform-root's too (`lan/proxmox-node005-vms.tf`,
`lan/proxmox-node006-vms.tf`, applied from the terraform-lan runners). The
`qm clone` playbooks, `vm_multiqueue.yaml` and `ocean_cpu_host.yaml` are
deleted; a new VM, or a CPU, memory, disk or NIC change, is a PR there, and a
hand `qm set` shows up as lan drift. Homelab configures what runs inside a VM
once it boots.

The old GCP KMS playbook (`gcloud_sdk.yaml` with `tasks/gcloud.yaml` and
`tasks/pki_gcloud_kms.yaml`) is deleted too. Its vars were defined nowhere, and
the empty `homelab-sops-key` ring it left behind is modeled in terraform-root.

**Cloudflare Access (tunnel) user emails.** The friends' allow list lives in
terraform-root, in the SOPS file `secrets/access.enc.yaml` (key `plex_users`,
terraform-root #53), and merging a change there is what applies it. The admin
address is inline in its `allow_admin` policy. The old vault copies under
`cloudflare.access_allow_emails` and terraform-root's
`TF_VAR_ACCESS_PLEX_USER_EMAILS` repo secret are no longer read by anything.
To resync from Plex invites, pipe `scripts/plex-shared-users.py` into that SOPS
file (the command is in terraform-root's agents.md, "Where the Access (tunnel)
user emails live").
