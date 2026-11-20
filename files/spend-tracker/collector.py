#!/usr/bin/env python3
"""Daily pull of subscription, API and cloud charges from Monarch into Postgres,
so Grafana can chart monthly spend from one table.

Monarch already aggregates every card, which makes it the one source that sees
every vendor. vendors.json decides which merchants count and what to call them;
the first rule that matches wins.
"""
import asyncio
import base64
import datetime
import json
import logging
import os
import re
import time
from decimal import Decimal

HERE = os.path.dirname(os.path.abspath(__file__))
PAGE = 500

SCHEMA = """
CREATE TABLE IF NOT EXISTS spend (
    id      text PRIMARY KEY,
    date    date NOT NULL,
    vendor  text NOT NULL,
    amount  numeric(10, 2) NOT NULL,
    cadence text NOT NULL,
    source  text NOT NULL
)
"""

log = logging.getLogger("spend-tracker")


def load_rules(path):
    with open(path) as fh:
        rules = json.load(fh)
    for rule in rules:
        rule["re"] = re.compile(rule["match"], re.I)
        if "amount" in rule:
            rule["amount"] = Decimal(str(rule["amount"]))
    return rules


def classify(txn, rules):
    """The spend row for one Monarch transaction, or None when no rule claims it."""
    # Monarch signs expenses negative. Income and refunds are not spend, and a
    # paycheck from an employer that is also a vendor must not match its rule.
    if txn["amount"] >= 0:
        return None
    amount = -Decimal(str(txn["amount"]))
    merchant = (txn.get("merchant") or {}).get("name") or txn.get("plaidName") or ""
    for rule in rules:
        if not rule["re"].search(merchant):
            continue
        if "amount" in rule and rule["amount"] != amount:
            continue
        return {
            "id": f"monarch:{txn['id']}",
            "date": txn["date"],
            "vendor": rule["vendor"],
            "amount": amount,
            "cadence": rule["cadence"],
            "source": "monarch",
        }
    return None


async def _fetch(start, end):
    from monarchmoney import MonarchMoney

    mm = MonarchMoney()
    mm.set_cookies(json.loads(base64.b64decode(os.environ["MONARCH_COOKIES_B64"])))
    txns, offset = [], 0
    while True:
        page = (await mm.get_transactions(limit=PAGE, offset=offset, start_date=start, end_date=end))["allTransactions"]
        txns += page["results"]
        offset += PAGE
        if offset >= page["totalCount"]:
            return txns


def _store(rows, start, end):
    import psycopg

    # NOTE: the window is replaced whole, in one transaction, rather than
    # upserted. A rule edit, a deleted rule and a pending charge that posts
    # under a new id all need old rows to go away, and an upsert would leave
    # them counted twice. Rows older than the window are never touched.
    with psycopg.connect("") as conn:
        conn.execute(SCHEMA)
        conn.execute("DELETE FROM spend WHERE source = 'monarch' AND date BETWEEN %s AND %s", (start, end))
        with conn.cursor() as cur:
            cur.executemany(
                "INSERT INTO spend (id, date, vendor, amount, cadence, source)"
                " VALUES (%(id)s, %(date)s, %(vendor)s, %(amount)s, %(cadence)s, %(source)s)",
                rows,
            )


def sync(rules, lookback_days):
    end = datetime.date.today()
    start = end - datetime.timedelta(days=lookback_days)
    txns = asyncio.run(_fetch(start.isoformat(), end.isoformat()))
    rows = [row for row in (classify(t, rules) for t in txns) if row]
    _store(rows, start, end)
    log.info("stored %d spend rows from %d transactions, %s to %s", len(rows), len(txns), start, end)
    return len(rows)


def main():
    from prometheus_client import Gauge, start_http_server

    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    rules = load_rules(os.path.join(HERE, "vendors.json"))
    lookback_days = int(os.environ["SPEND_LOOKBACK_DAYS"])
    interval = int(os.environ["SPEND_SYNC_SECONDS"])

    last_success = Gauge("spend_tracker_last_success_timestamp_seconds", "Unix time of the last completed sync")
    row_count = Gauge("spend_tracker_rows", "Spend rows written by the last sync")
    start_http_server(int(os.environ["SPEND_PORT"]))

    while True:
        # The Monarch session is a browser cookie that expires on Monarch's
        # schedule, so a failed sync is expected now and then. Stay up and let
        # SpendTrackerStale page on the gauge instead of crash-looping.
        try:
            row_count.set(sync(rules, lookback_days))
            last_success.set_to_current_time()
            time.sleep(interval)
        except Exception:
            log.exception("sync failed, retrying in an hour")
            time.sleep(3600)


if __name__ == "__main__":
    main()
