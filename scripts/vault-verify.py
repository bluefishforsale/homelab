#!/usr/bin/env python3
"""Check that the credentials in the vault still work against their own services.

A key can die without anything noticing: a token deleted in a dashboard, a
secret rotated by hand and never written back, a cert whose embedded token was
revoked. The first sign used to be a deploy failing weeks later. This asks each
service directly, read-only, and reports per vault path.

  vault-verify.py            probe everything, print ok / DEAD / ERR per path
  vault-verify.py cloudflare only paths under a prefix (any number of prefixes)
  vault-verify.py -v         also list the vault paths that have no probe

DEAD means the service answered and rejected the credential. ERR means the
service could not be asked (down, off-LAN, unexpected reply), so it says
nothing about the key. Exit 1 if anything is DEAD, else 0.

Every probe is a read (whoami, status, list) and never sends a value anywhere
but the service it belongs to. Values are never printed. *.home services need
the LAN or the offsite.sh tunnel (eval "$(scripts/offsite.sh env)").
"""
import base64
import json
import os
import smtplib
import sys
import urllib.error
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import offsite_proxy  # noqa: E402,F401  (off-LAN: urllib through the offsite.sh tunnel)
import vault  # noqa: E402

OCEAN = os.environ.get("MEDIA_HOST", "ocean.home")
# Plex requires https; only its home.terrac.com cert chains to a trusted root.
PLEX = "https://home.terrac.com:32400"
CF ="https://api.cloudflare.com/client/v4"
TIMEOUT = 15


class Dead(Exception):
    """The service answered and rejected the credential."""


def http(url, headers=None, dead=(401, 403)):
    # Cloudflare-fronted APIs (discord, ntfy) 403 urllib's default User-Agent,
    # which reads exactly like a rejected key.
    req = urllib.request.Request(url, headers={"User-Agent": "vault-verify/1", **(headers or {})})
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as r:
            body = r.read()
    except urllib.error.HTTPError as e:
        # NOTE: only the codes a probe declares mean "rejected"; anything else
        # (404, 5xx) is the service misbehaving, and calling that DEAD would send
        # someone to rotate a key that is fine.
        if e.code in dead:
            raise Dead(f"HTTP {e.code}")
        raise RuntimeError(f"HTTP {e.code}")
    try:
        return json.loads(body)
    except ValueError:
        return body


def cf_token(tok):
    r = http(f"{CF}/user/tokens/verify", {"Authorization": f"Bearer {tok}"})
    if r.get("result", {}).get("status") != "active":
        raise Dead(r.get("result", {}).get("status", "inactive"))
    return f"active id={r['result']['id'][:8]}"


def cf_global(v):
    c = v["cloudflare"]
    h = {"X-Auth-Email": c["api_email"], "X-Auth-Key": c["api_key"]}
    return http(f"{CF}/user", h)["result"]["email"].split("@")[0] + "@…", h


def cf_cert(pem):
    b64 = "".join(l for l in pem.strip().splitlines() if not l.startswith("-----"))
    return "embedded " + cf_token(json.loads(base64.b64decode(b64))["apiToken"])


def cf_tunnel_secret(v):
    # The secret only exists in two places, the vault and Cloudflare, so compare
    # them: a mismatch is a connector that will fail on its next restart.
    c = v["cloudflare"]
    _, h = cf_global(v)
    tid, acct = c["tunnel_credentials"]["main"]["tunnel_id"], c["tunnel_credentials"]["main"]["account_tag"]
    tok = http(f"{CF}/accounts/{acct}/cfd_tunnel/{tid}/token", h)["result"]
    if json.loads(base64.b64decode(tok))["s"] != c["tunnel_credentials"]["main"]["tunnel_secret"]:
        raise Dead("vault secret differs from Cloudflare's")
    return "matches Cloudflare"


def arr(port, ver="v3"):
    return lambda k: http(f"http://{OCEAN}:{port}/api/{ver}/system/status", {"X-Api-Key": k})["version"]


def bazarr(k):
    return http(f"http://{OCEAN}:6767/api/system/status", {"X-API-KEY": k})["data"]["bazarr_version"]


def tautulli(k):
    r = http(f"http://{OCEAN}:8905/api/v2?apikey={k}&cmd=arnold")["response"]
    if r["result"] != "success":
        raise Dead(r.get("message") or "rejected")
    return "ok"


def jellyfin(k):
    return http(f"http://{OCEAN}:8096/System/Info", {"Authorization": f'MediaBrowser Token="{k}"'})["Version"]


def plex_tv(tok):
    return http("https://plex.tv/api/v2/user", {"X-Plex-Token": tok, "Accept": "application/json"})["username"]


def plex_local(tok):
    # /identity answers without auth; /library/sections is the cheapest call that checks
    http(f"{PLEX}/library/sections?X-Plex-Token={tok}")
    return "local server ok"


def bearer(url, dead=(401, 403)):
    def probe(tok):
        http(url, {"Authorization": f"Bearer {tok}"}, dead)
        return "ok"
    return probe


def github(tok):
    r = http("https://api.github.com/rate_limit",
             {"Authorization": f"Bearer {tok}", "User-Agent": "vault-verify"})
    return f"rate remaining {r['resources']['core']['remaining']}"


def query(url_fmt, dead=(401, 403)):
    def probe(k):
        http(url_fmt.format(urllib.parse.quote(k)), dead=dead)
        return "ok"
    return probe


def header(url, name):
    def probe(k):
        http(url, {name: k})
        return "ok"
    return probe


def anthropic(k):
    http("https://api.anthropic.com/v1/models", {"x-api-key": k, "anthropic-version": "2023-06-01"})
    return "ok"


def telegram(tok):
    return "@" + http(f"https://api.telegram.org/bot{tok}/getMe")["result"]["username"]


def discord(tok):
    return http("https://discord.com/api/v10/users/@me", {"Authorization": f"Bot {tok}"})["username"]


def alpaca(v):
    p = v["financial"]["alpaca"]["paper"]
    r = http("https://paper-api.alpaca.markets/v2/account",
             {"APCA-API-KEY-ID": p["api_key"], "APCA-API-SECRET-KEY": p["api_secret"]})
    return f"account {r['status']}"


def basic(url, user, pw):
    tok = base64.b64encode(f"{user}:{pw}".encode()).decode()
    http(url, {"Authorization": f"Basic {tok}"})
    return "ok"


def smtp(host, port, user, pw):
    try:
        with smtplib.SMTP(host, port, timeout=TIMEOUT) as s:
            s.starttls()
            s.login(user, pw)
    except smtplib.SMTPAuthenticationError as e:
        raise Dead(f"SMTP {e.smtp_code}")
    return "login ok"


def docker_hub(v):
    d = v["development"]["docker_registry"]
    return basic("https://auth.docker.io/token?service=registry.docker.io", d["username"], d["token"])


def at(v, path):
    return vault.resolve(v, path.split("."))[0]


# vault path -> probe. A probe takes the value at that path, or the whole vault
# when it needs a sibling (a username, an account id); those are marked WHOLE.
WHOLE = object()
PROBES = {
    "cloudflare.api_key": (WHOLE, lambda v: cf_global(v)[0]),
    "cloudflare.cert_pem": cf_cert,
    "cloudflare.exporter_tokens.terrac": cf_token,
    "cloudflare.tunnel_credentials.main.tunnel_secret": (WHOLE, cf_tunnel_secret),
    "external_apis.ddns.cloudflare_token": cf_token,
    "media_services.sonarr.api_key": arr(8902),
    "media_services.radarr.api_key": arr(8903),
    "media_services.prowlarr.api_key": arr(9696, "v1"),
    "media_services.bazarr.api_key": bazarr,
    "media_services.tautulli.api_key": tautulli,
    "media_services.jellyfin.api_key": jellyfin,
    "media_services.plex.api_key": plex_tv,
    "media_services.plex.plex_token": plex_local,
    "external_apis.tmdb.api_key": query("https://api.themoviedb.org/3/configuration?api_key={}"),
    "external_apis.brevo.api_key": header("https://api.brevo.com/v3/account", "api-key"),
    "external_apis.smtp_relay.password": (WHOLE, lambda v: smtp(
        "smtp-relay.brevo.com", 587, at(v, "external_apis.smtp_relay.login"), at(v, "external_apis.smtp_relay.password"))),
    "smtp.gmail_app_password": (WHOLE, lambda v: smtp(
        "smtp.gmail.com", 587, at(v, "smtp.gmail_user"), at(v, "smtp.gmail_app_password"))),
    "ai_services.huggingface.token": bearer("https://huggingface.co/api/whoami-v2"),
    "ai_services.openai.api_key": bearer("https://api.openai.com/v1/models"),
    "ai_services.claude.etrading_boy.api_key": anthropic,
    "ai_services.google_gemini.api_key": query("https://generativelanguage.googleapis.com/v1beta/models?key={}", (400, 401, 403)),
    "ai_services.tavily.api_key": bearer("https://api.tavily.com/usage"),
    "ai_services.openclaw.telegram_bot_token": telegram,
    "ai_services.openclaw.discord_bot_token": discord,
    "financial.polygon_io.api_key": query("https://api.polygon.io/v3/reference/tickers?limit=1&apiKey={}"),
    "financial.finnhub.api_key": query("https://finnhub.io/api/v1/quote?symbol=AAPL&token={}"),
    "financial.alpaca.paper.api_secret": (WHOLE, alpaca),
    "development.github.token": github,
    "development.github.deploy_dispatch_token": github,
    "development.github.packages_read_token": github,
    "development.github.actions_token_terrac-com-deploy-dispatch": github,
    "development.github.terrac-com-deploy-dispatch-classic": github,
    "my_ta_jose.github_homelab_deploy_pat": github,
    "photonic_inventory.github_homelab_deploy_pat": github,
    "agentbox.fleet_pat": github,
    "development.docker_registry.token": (WHOLE, docker_hub),
    "infrastructure.dns_stack.pdns_api_key": header("http://dns01.home:8081/api/v1/servers", "X-API-Key"),
    "monitoring.grafana.api_key": bearer(f"http://{OCEAN}:8910/api/org"),
    "ntfy.password": (WHOLE, lambda v: basic("https://ntfy.terrac.com/v1/account", at(v, "ntfy.user"), at(v, "ntfy.password"))),
}
PROBES.update({f"ai_services.llamacpp.api_keys.{n}": bearer(f"http://{OCEAN}:8080/v1/models")
               for n in ("terrac", "jba", "my_ta_jose", "spare2", "spare3", "openwebui")})
PROBES["ai_services.llamacpp.api_key"] = bearer(f"http://{OCEAN}:8080/v1/models")


def run(path, probe, v):
    fn, arg = (probe[1], v) if isinstance(probe, tuple) else (probe, at(v, path))
    if arg in (None, ""):
        return path, "ERR", "empty in vault"
    try:
        return path, "ok", fn(arg)
    except Dead as e:
        return path, "DEAD", str(e)
    except Exception as e:  # noqa: BLE001 - any other failure is "could not ask", never DEAD
        # never str() a urllib error built from a URL: some probes carry the key in the query
        return path, "ERR", type(e).__name__ + (f": {e}" if isinstance(e, RuntimeError) else "")


def main():
    args = [a for a in sys.argv[1:] if a != "-v"]
    v = vault.load(vault.read())
    chosen = {p: f for p, f in PROBES.items() if not args or any(p.startswith(a) for a in args)}
    with ThreadPoolExecutor(8) as pool:
        results = sorted(pool.map(lambda kv: run(kv[0], kv[1], v), chosen.items()))
    width = max(len(p) for p, _, _ in results)
    for path, status, detail in results:
        print(f"{status:4}  {path:{width}}  {detail}")
    counts = {s: sum(r[1] == s for r in results) for s in ("ok", "DEAD", "ERR")}
    unprobed = [".".join(p) for p, _ in vault.flatten(v) if ".".join(p) not in PROBES]
    print(f"\n{counts['ok']} ok, {counts['DEAD']} DEAD, {counts['ERR']} ERR; {len(unprobed)} vault paths have no probe"
          + ("" if "-v" in sys.argv else " (-v lists them)"))
    if "-v" in sys.argv:
        print("\n".join("  " + p for p in unprobed))
    sys.exit(1 if counts["DEAD"] else 0)


if __name__ == "__main__":
    main()
