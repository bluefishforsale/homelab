#!/usr/bin/env python3
"""Pins collector.classify(): run `python3 files/spend-tracker/test_collector.py` (no pytest).

A rule that claims the wrong merchant puts a paycheck or a hardware purchase on
the subscription dashboard, and nothing downstream can tell.
"""
import importlib.util
import os
from decimal import Decimal

HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("collector", os.path.join(HERE, "collector.py"))
collector = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(collector)

RULES = collector.load_rules(os.path.join(HERE, "vendors.json"))


def txn(merchant, amount, id="1", date="2026-11-11"):
    return {"id": id, "date": date, "amount": amount, "merchant": {"name": merchant}}


def vendor(merchant, amount):
    row = collector.classify(txn(merchant, amount), RULES)
    return row and row["vendor"]


def test_income_from_a_vendor_name_is_not_spend():
    assert vendor("Cloudflare Inc", 3699.04) is None
    assert vendor("OpenAI", 10.26) is None


def test_paypal_charge_is_claimed_only_at_the_exact_amount():
    assert vendor("PAYPAL", -4.00) == "GitHub"
    assert vendor("PAYPAL", -240.00) == "Cloudflare Pro"
    assert vendor("PAYPAL", -19.91) is None


def test_google_hardware_purchase_is_not_google_cloud():
    assert vendor("Google", -279.00) is None
    assert vendor("Google Cloud", -19.37) == "Google Cloud"
    assert vendor("Google One", -249.99) == "Google One"


def test_both_alpaca_merchant_spellings_map_to_one_vendor():
    assert vendor("Alpacadb", -99.00) == "Alpaca"
    assert vendor("Alpacad", -99.00) == "Alpaca"


def test_apple_is_claimed_only_for_the_developer_fee():
    assert vendor("Apple", -99.00) == "Apple Developer"
    assert vendor("Apple", -2.99) is None


def test_amount_is_positive_and_keeps_its_cents():
    row = collector.classify(txn("OpenAI", -14.37, id="abc"), RULES)
    assert row == {
        "id": "monarch:abc",
        "date": "2026-11-11",
        "vendor": "OpenAI",
        "amount": Decimal("14.37"),
        "cadence": "usage",
        "source": "monarch",
    }


def test_merchant_falls_back_to_the_bank_description():
    row = collector.classify({"id": "1", "date": "2026-11-11", "amount": -30.0, "merchant": None, "plaidName": "SUNO INC"}, RULES)
    assert row["vendor"] == "Suno"


def test_every_shipped_rule_has_a_cadence_the_dashboard_knows():
    assert {r["cadence"] for r in RULES} <= {"monthly", "annual", "usage"}


if __name__ == "__main__":
    tests = [v for k, v in sorted(globals().items()) if k.startswith("test_")]
    for t in tests:
        t()
        print(f"ok  {t.__name__}")
    print(f"{len(tests)} passed")
