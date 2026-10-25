# Network Segmentation: Camera and IoT VLANs

Tracking issue: #911. Status: planned, nothing changed yet.

**Goal:** cameras and IoT devices can't reach the homelab servers or the
trusted clients, and cameras can't reach the internet at all. The trusted
network keeps the full access it has today.

**Anti-goals:** no renumbering of the existing 192.168.1.0/24 network, no new
gateway or core switch, no change to how servers, VMs or Proxmox bridges are
addressed, no guest network.

---

## Design

| Tier | VLAN | Subnet | Gateway | Access |
|------|------|--------|---------|--------|
| Trusted ("VLAN 10" tier) | Default, untagged | 192.168.1.0/24 | 192.168.1.1 | Everything, as today |
| Cameras | 100 | 192.168.100.0/24 | 192.168.100.1 | UCG Max (Protect) only |
| IoT | 200 | 192.168.200.0/24 | 192.168.200.1 | Internet, plus DNS to dns01/dns02 |

The trusted tier stays on the existing Default network rather than a literal
VLAN 10. UniFi won't create a network with VLAN ID 1 and Default is always
untagged, so a real VLAN 10 means moving every server, VM bridge, Kea, DNS and
every hardcoded `192.168.1.x` in this repo onto a new subnet, and ending up with
the same access as today. Splitting servers from personal devices later is a
separate project; that is the only case where a tagged VLAN 10 would be worth it.

The third octet matches the VLAN ID so an address tells you its network.

The UCG Max stays the only router (router-on-a-stick). It is in every VLAN and
it runs Protect on a 512 GB NVMe, so camera streams go straight to it and never
cross into Default. Cross-VLAN traffic is a few tens of Mbps of camera video
plus IoT chatter, well under the 1G link from the UCG Max to the US-48.

### Firewall (UCG Max zone-based firewall)

- **Cameras zone:** allow to Gateway (Protect adoption, streams, NTP, DNS on the
  gateway). Block Internet, Trusted and IoT. Trusted does not need to reach
  cameras directly: the Protect app pulls video from the console, not the cameras.
- **IoT zone:** allow Internet. Allow DNS (udp/tcp 53) to 192.168.1.2 and
  192.168.1.3 only. Block everything else into Trusted and Cameras.
- **Trusted zone:** unchanged; may open connections into IoT (return traffic only
  comes back).
- **mDNS reflection:** off. The only device that needed local discovery, the HP
  printer, stays on Trusted. Turn it on later only if an IoT device has to be
  discovered from a phone.

### Wi-Fi

Three SSIDs, all broadcast on all three APs. The garage AP also serves the
phone and iPad, so it carries the trusted SSID like the others.

| SSID | Network | Bands |
|------|---------|-------|
| TerracnoDome (existing) | Trusted | 2.4 + 5 GHz |
| IoT SSID (name TBD) | VLAN 200 | 2.4 GHz (Emporia are 2.4 only) |
| Camera SSID (name TBD) | VLAN 100 | 2.4 + 5 GHz |

Three is the ceiling: every SSID adds beacon airtime, and 2.4 GHz is already at
30-36% utilization. PPSK (one SSID, VLAN chosen by password) would cut this to
two SSIDs, but it is unverified on AC Pro hardware; don't count on it.

### Device placement

| Device | Today | Goes to | How |
|--------|-------|---------|-----|
| G3 Bullet | US-8-150W port 3 | VLAN 100 | Port profile, native VLAN 100 |
| G5 Dome | US-8-150W port 4 | VLAN 100 | Port profile, native VLAN 100 |
| G3 Dome | Garage AP second port | VLAN 100 | Needs its own drop first (see Phase 0) |
| G3 Instant | Wi-Fi, home-1 | VLAN 100 | Camera SSID |
| G3 Micro x2 | Wi-Fi, home-2 and garage | VLAN 100 | Camera SSID |
| Emporia (desk, lab) | Wi-Fi | VLAN 200 | IoT SSID |
| Hon Hai `9c:30:5b:8f:eb:a9` | Wi-Fi, garage, 5 GHz | VLAN 200 or removed | Identify first |
| HP LaserJet, laptops, Mac mini, docks | Desk switch on US-48 port 19 | Trusted | No change |
| `34:e1:d1:82:1f:f4` (.185) | Desk switch on US-48 port 19 | Trusted | Identify before trusting |
| Servers, VMs, Proxmox | US-16-XG | Trusted | No change |

---

## Evidence behind the design

Gathered 2026-10-25 from one scrape of unpoller (`192.168.1.143:9130/metrics`)
and Ubiquiti's spec pages.

**Inventory and topology.** UCG Max (gateway, Protect) connects to the US-48 at
1G (`unifi_device_uplink_speed_mbps 1000`, max 2500). The US-48 connects to the
US-16-XG over a 20G LAG and to the US-8-150W over a 2G LAG. The APs are three
UAP-AC-Pro (`U7PG2`, firmware 6.8.2): home-1 and home-2 on the US-8, garage-1 on
US-48 port 15.

**Capabilities checked:**

- **UCG Max** (techspecs.ui.com/unifi/cloud-gateways/ucg-max): zone-based
  firewall, VLAN segmentation, mDNS, Virtual Network Override, 2.3 Gbps IDS/IPS,
  Protect limit "(15) HD (8) 2K (5) 4K", NVMe up to 2 TB.
- **UAP-AC-Pro** (techspecs.ui.com): 802.1Q, 8 BSSIDs per radio. Its second
  Ethernet port is a data-only bridge onto the AP's own untagged network: no PoE
  and no per-port VLAN.
- **Switches:** US-48 Gen1 (no PoE), US-16-XG and US-8-150W all do 802.1Q. The
  US-8 draws about 17.5 W of its 150 W budget.
- **Lifecycle** (help.ui.com "Vintage and Legacy Products"): the switches, APs
  and UCG Max are not listed. All G3 cameras are "Vintage", which still means
  "critical bug fixes and security updates".

**Problems found:**

- home-1 and garage-1 are linked at 100 Mbps (`uplink_speed_mbps 100`) despite
  gigabit ports. That points at a cable, keystone or injector fault.
- home-1 and home-2 are both on 5 GHz channel 161 with zero 5 GHz clients, while
  phones, the iPad and the Mac mini (-48 dBm) sit on 2.4 GHz.
- Garage 5 GHz clients are weak: MacBook -72 dBm, Pixel 10 -77, G3 Micro -77.
- Client satisfaction is 0.86 to 1.0 everywhere, so capacity is not the problem.

**DHCP.** Kea is the only DHCP server: a hot-standby HA pair on dns01
(192.168.1.2) and dns02 (192.168.1.3) with one subnet, listening on `eth0`
(`roles/dns_infrastructure/templates/kea/kea-dhcp4.conf.j2`). Hot-standby only
fails over if both peers see the requests, so the UCG Max has to relay to both.

---

## Hardware

- **No new switching** is needed for VLANs.
- **Optional:** a USW-Flex-Mini (VLAN-aware, about $30) only if a device behind
  the desk switch ever has to leave Trusted.
- **Garage:** a new cable run is required for the G3 Dome. If garage 5 GHz still
  reads around -75 dBm after Phase 0, add one Wi-Fi 6 or 7 AP on that run. That
  is the only AP purchase in this plan.
- **Protect storage:** check retention in Protect > Settings > Storage. Rough,
  unmeasured math: about 30 Mbps of continuous recording is about 320 GB a day,
  under two days on 512 GB. If retention is short, the UCG Max takes NVMe up to
  2 TB.

---

## Runbook

Each phase stands alone and is reversible. Do them in order, and don't stack
phases in one sitting.

### Phase 0: fix the physical layer (no VLAN changes)

- [ ] Re-terminate or replace the home-1 cable (US-8) until the port links at 1000.
- [ ] Re-terminate or replace the garage-1 cable (US-48 port 15) and check its
      PoE injector until it links at 1000.
- [ ] Verify: `unifi_device_uplink_speed_mbps` reads 1000 for both APs.
- [ ] Move home-1 and home-2 onto different, non-overlapping 5 GHz channels.
- [ ] Find out why nothing joins home 5 GHz (band steering, minimum RSSI or a
      per-band SSID setting).
- [ ] Run a cable to the G3 Dome location (from the US-8, or a small PoE switch
      in the garage) so it no longer hangs off the AP's second port.
- [ ] Identify the Hon Hai device: check the UniFi client fingerprint, or block
      the MAC for a day and see what complains.
- [ ] Identify `34:e1:d1:82:1f:f4` at 192.168.1.185.
- [ ] Re-check garage signal and decide on the extra AP.

Backout: none needed. These are repairs.

### Phase 1: DHCP and DNS in the repo (PR, deploys to dns01/dns02)

Land this before UniFi has anything to relay, so a new VLAN never comes up
without leases. It deploys through `dns_ha_stack`, which already rolls serially
with a health gate.

- [ ] In `roles/dns_infrastructure/templates/kea/kea-dhcp4.conf.j2`, add two
      `subnet4` entries (id 100, 192.168.100.0/24 and id 200, 192.168.200.0/24),
      each with its own pool (for example `.100 - .239`).
- [ ] Give each subnet its own `routers` and `broadcast-address`. NOTE: the
      global `option-data` hardcodes `routers 192.168.1.1` and
      `broadcast-address 192.168.1.255`. A new subnet that only inherits those
      hands clients a gateway they can't reach.
- [ ] Decide DNS per subnet: cameras get the gateway (192.168.100.1) or none;
      IoT gets 192.168.1.2 and 192.168.1.3 (matches the firewall allow).
- [ ] Add camera reservations so each camera keeps a stable address and name.
- [ ] Add reverse zones `100.168.192.in-addr.arpa.` and
      `200.168.192.in-addr.arpa.` to
      `roles/dns_infrastructure/templates/powerdns/import-zones.sh.j2`, matching
      the existing `1.168.192` block.
- [ ] Add matching `reverse-ddns` domains to
      `roles/dns_infrastructure/templates/kea/kea-dhcp-ddns.conf.j2`.
- [ ] Verify after deploy: `kea-dhcp4 -t` passes (the container healthcheck),
      kea-exporter shows the new subnets, and existing leases on
      192.168.1.0/24 still renew.

Backout: revert the PR. Nothing uses the new subnets yet.

### Phase 2: create the networks in UniFi (no devices moved)

- [ ] Create network "Cameras", VLAN 100, gateway 192.168.100.1/24,
      DHCP mode Relay to 192.168.1.2 and 192.168.1.3.
- [ ] Create network "IoT", VLAN 200, gateway 192.168.200.1/24, same relay.
- [ ] Confirm the relay accepts both Kea peers. If UniFi only takes one, relay
      to the hot-standby primary and record that the new VLANs lose DHCP
      failover. Don't fall back to UniFi's own DHCP: Kea stays the only source.
- [ ] Rename Default to "Trusted" (display name only).
- [ ] Switch trunk ports already carry all VLANs by default. Confirm the US-48
      to US-8 and US-48 to UCG Max links allow VLAN 100 and 200.

Backout: delete the two networks.

### Phase 3: move one wired camera as a canary

- [ ] Set US-8-150W port 3 (G3 Bullet) to native VLAN 100.
- [ ] Verify: the camera gets a 192.168.100.x lease from Kea, Protect shows it
      online and recording, and live view works from the phone over Wi-Fi.
- [ ] Wait a day. Check Protect for gaps.
- [ ] Move port 4 (G5 Dome) and the G3 Dome's new port.

Backout: set the port back to Default. Protect keeps the camera adopted
through an IP change.

### Phase 4: SSIDs and wireless devices

- [ ] Create the camera SSID (VLAN 100) and the IoT SSID (VLAN 200, 2.4 GHz),
      on all APs.
- [ ] Move one Wi-Fi camera (the G3 Instant) to the camera SSID through the
      Protect app. Unverified whether Protect can switch a camera's SSID in
      place; if not, re-provision it over Bluetooth or a QR code.
- [ ] Verify, then move both G3 Micros.
- [ ] Move both Emporias to the IoT SSID and confirm the Emporia app still
      reports.
- [ ] Move or remove the Hon Hai device, depending on what it turned out to be.

Backout: rejoin the device to TerracnoDome.

### Phase 5: firewall rules

Last, so every earlier step is debugged with traffic flowing.

- [ ] Add the Cameras and IoT zone policies from the Design section.
- [ ] Verify from a camera: Protect still records. Verify the camera can't reach
      the internet or 192.168.1.143.
- [ ] Verify from an IoT device: the Emporia app works, DNS resolves through
      dns01/dns02, and it can't reach 192.168.1.143 or any 192.168.100.x.
- [ ] Verify from Trusted: unchanged access to servers and internet, and Protect
      live view works.

Backout: disable the policies. The networks keep working without them.

### Phase 6: monitoring and docs

- [ ] Alert when a known camera MAC shows up on a network other than Cameras.
      Unpoller already exports `network` on `unifi_client_*`.
- [ ] Update `docs/architecture/networking.md` and `network-topology.md`. Both
      still describe the old `192.0.2.x` flat layout with pihole first.

---

## Open items

- Names for the camera and IoT SSIDs.
- Whether the UniFi relay accepts two servers (Phase 2).
- Protect retention on the 512 GB drive (Hardware).
