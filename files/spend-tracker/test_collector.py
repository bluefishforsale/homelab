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


def test_a_merchant_that_only_contains_a_vendor_name_is_not_that_vendor():
    assert vendor("Sunoco", -52.10) is None
    assert vendor("Dockers", -64.00) is None
    assert vendor("YouTube TV", -82.99) is None
    assert vendor("Cloudflare Inc", -11.17) is None


def test_every_merchant_spelling_seen_in_monarch_still_matches():
    seen = {
        "Anthropic": "Anthropic API", "OpenAI": "OpenAI", "Massive.com": "Massive", "Suno Inc": "Suno",
        "Suno Inc.": "Suno", "YouTube": "YouTube", "NEWSDEMON.COM": "Newsdemon", "Newsdemon.com": "Newsdemon",
        "Monarch Money App": "Monarch", "Docker Inc": "Docker", "CLOUDFLARE": "Cloudflare Registrar",
    }
    for merchant, expected in seen.items():
        assert vendor(merchant, -1.00) == expected, merchant


def test_a_transaction_hidden_in_monarch_is_not_counted():
    hidden = dict(txn("PAYPAL", -4.00), hideFromReports=True)
    assert collector.classify(hidden, RULES) is None


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


def anthropic(descriptor, amount, date):
    charge = dict(txn("Anthropic", amount, date=date), plaidName=descriptor)
    row = collector.classify(charge, RULES)
    return row and (row["vendor"], row["cadence"])


def test_anthropic_plan_charges_and_api_credits_are_separate_vendors():
    assert anthropic("ANTHROPIC", -37.05, "2026-11-11") == ("Anthropic API", "usage")
    assert anthropic("ANTHROPIC* CLAUDE SUB", -200.00, "2026-11-12") == ("Anthropic Sub", "monthly")


def test_the_annual_plan_is_told_from_the_monthly_one_by_its_renewal_day():
    assert anthropic("ANTHROPIC* CLAUDE SUB", -200.00, "2026-11-02") == ("Anthropic Sub (wife)", "annual")
    assert anthropic("ANTHROPIC* CLAUDE SUB", -200.00, "2027-11-02") == ("Anthropic Sub (wife)", "annual")
    assert anthropic("ANTHROPIC* CLAUDE SUB", -200.00, "2026-10-12") == ("Anthropic Sub", "monthly")


def test_an_api_credit_bought_on_the_annual_renewal_day_stays_api():
    assert anthropic("ANTHROPIC", -20.02, "2026-11-02") == ("Anthropic API", "usage")


def test_an_annual_renewal_that_posts_a_few_days_off_is_not_a_second_monthly_charge():
    assert anthropic("ANTHROPIC* CLAUDE SUB", -200.00, "2027-11-01") == ("Anthropic Sub (wife)", "annual")
    assert anthropic("ANTHROPIC* CLAUDE SUB", -200.00, "2027-11-04") == ("Anthropic Sub (wife)", "annual")
    assert anthropic("ANTHROPIC* CLAUDE SUB", -200.00, "2027-11-12") == ("Anthropic Sub", "monthly")


def test_a_plan_charge_with_a_reworded_bank_description_is_not_counted_as_api():
    assert anthropic("ANTHROPIC *CLAUDE SUB", -200.00, "2026-12-12") == ("Anthropic Sub", "monthly")
    assert anthropic("ANTHROPIC* CLAUDE SUB SAN FRANCISCO CA", -200.00, "2026-12-12") == ("Anthropic Sub", "monthly")


def test_a_renewal_date_that_is_not_month_dash_day_fails_at_load():
    import json
    import tempfile

    with tempfile.NamedTemporaryFile("w", suffix=".json") as fh:
        json.dump([{"vendor": "X", "match": "x", "renews": "11/02", "cadence": "annual"}], fh)
        fh.flush()
        try:
            collector.load_rules(fh.name)
        except ValueError:
            return
    raise AssertionError("a malformed renews value loaded")


def test_every_shipped_rule_has_a_cadence_the_dashboard_knows():
    assert {r["cadence"] for r in RULES} <= {"monthly", "annual", "usage"}


if __name__ == "__main__":
    tests = [v for k, v in sorted(globals().items()) if k.startswith("test_")]
    for t in tests:
        t()
        print(f"ok  {t.__name__}")
    print(f"{len(tests)} passed")
