#!/usr/bin/env bash
# offsite.sh — reach the homelab from off the LAN through the edge gateway.
#
# The way in is a SOCKS tunnel to home.terrac.com on port 6759 (not 22; 22, 443
# and 2222 are filtered). With it up, curl-based scripts (prom.sh, loki.sh,
# alerts.sh) work once ALL_PROXY points at it, and ssh to *.home / 192.168.1.*
# works through the Match block printed by `ssh-config`. The Python scripts that
# reach the LAN over HTTP import offsite_proxy.py, which reads the same ALL_PROXY.
#
# Usage:
#   offsite.sh up            start the tunnel if off-LAN and not already up
#   offsite.sh down          stop the tunnel (do this back home: the gateway
#                            hairpin is not something to depend on)
#   offsite.sh status        LAN, tunnel, and Prometheus-through-tunnel
#   eval "$(offsite.sh env)" export ALL_PROXY while the tunnel is in use, unset otherwise
#   offsite.sh active        exit 0 iff the tunnel is up and the LAN is not reachable
#                            (what the ssh Match block runs)
#   offsite.sh ssh-config    print the ~/.ssh/config block
#
# Env: HOMELAB_GATEWAY (default home.terrac.com), HOMELAB_GATEWAY_PORT (default 6759),
#      HOMELAB_SOCKS_PORT (default 8080)
set -uo pipefail

GATEWAY="${HOMELAB_GATEWAY:-home.terrac.com}"
GATEWAY_PORT="${HOMELAB_GATEWAY_PORT:-6759}"
SOCKS="${HOMELAB_SOCKS_PORT:-8080}"
PROBE_HOST=192.168.1.143

die() { printf '%s\n' "$*" >&2; exit 1; }
usage() { grep -E '^#' "$0" | sed -n '/Usage:/,$p' | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

on_lan() { nc -z -G 1 "$PROBE_HOST" 22 2>/dev/null; }
tunnel_up() { lsof -nP -iTCP:"$SOCKS" -sTCP:LISTEN >/dev/null 2>&1; }
via_tunnel() { curl -fsS -m 8 --socks5-hostname "127.0.0.1:$SOCKS" -o /dev/null "http://$PROBE_HOST:9090/-/ready" 2>/dev/null; }

case "${1:-}" in
  up)
    if on_lan; then echo "on the LAN, no tunnel needed"; exit 0; fi
    if tunnel_up; then
      echo "tunnel already listening on :$SOCKS"
    else
      ssh -f -N -o ExitOnForwardFailure=yes -o ServerAliveInterval=30 -o ControlPath=none \
        -D "$SOCKS" "$GATEWAY" -p "$GATEWAY_PORT" || die "could not reach $GATEWAY:$GATEWAY_PORT"
      echo "tunnel up on :$SOCKS via $GATEWAY:$GATEWAY_PORT"
    fi
    via_tunnel || die "tunnel is listening but Prometheus did not answer through it"
    echo "ocean reachable; now: eval \"\$($0 env)\""
    ;;
  down)
    pkill -f "ssh .*-D $SOCKS $GATEWAY" && echo "tunnel stopped" || echo "no tunnel running"
    ;;
  status)
    on_lan && echo "lan:     reachable" || echo "lan:     not reachable"
    tunnel_up && echo "tunnel:  listening on :$SOCKS" || echo "tunnel:  down"
    if tunnel_up; then via_tunnel && echo "through: ocean answers" || echo "through: ocean does NOT answer"; fi
    [ -n "${ALL_PROXY:-}" ] && echo "shell:   ALL_PROXY=$ALL_PROXY" || echo "shell:   ALL_PROXY unset"
    ;;
  env)
    if tunnel_up && ! on_lan; then echo "export ALL_PROXY=socks5h://127.0.0.1:$SOCKS"; else echo "unset ALL_PROXY"; fi
    ;;
  active)
    tunnel_up && ! on_lan
    ;;
  ssh-config)
    cat <<EOF
# Homelab off-LAN: route fleet ssh through the offsite.sh SOCKS tunnel, only
# while it is up and the LAN is not reachable. host comes first so the exec
# check runs only for fleet hosts.
Match host "*.home,192.168.1.*" exec "$(cd "$(dirname "$0")" && pwd)/offsite.sh active"
    ProxyCommand nc -X 5 -x 127.0.0.1:$SOCKS %h %p
EOF
    ;;
  -h|--help) usage 0 ;;
  *) usage 1 ;;
esac
