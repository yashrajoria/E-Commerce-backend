#!/usr/bin/env python3
"""
Executes full seed SQL against Supabase Management API
"""

import json
import urllib.request
import sys

import os

PROJECT_REF = os.environ.get("SUPABASE_PROJECT_REF", "vtfryikkounwjuyceztn")
TOKEN = os.environ.get("SUPABASE_ACCESS_TOKEN", "")
URL = f"https://api.supabase.com/v1/projects/{PROJECT_REF}/database/query"

def run_query(sql):
    req = urllib.request.Request(
        URL,
        data=json.dumps({"query": sql}).encode("utf-8"),
        headers={
            "Authorization": f"Bearer {TOKEN}",
            "Content-Type": "application/json"
        }
    )
    try:
        with urllib.request.urlopen(req) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        err_msg = e.read().decode("utf-8")
        print(f"HTTP Error {e.code}: {err_msg}", file=sys.stderr)
        raise

def main():
    print(f"Reading seed_supabase_full.sql...")
    with open("/Users/yashrajoria/ShopSwift/E-Commerce-backend/backend/scripts/seed_supabase_full.sql", "r") as f:
        sql = f.read()

    print(f"Executing full seed script ({len(sql)} bytes) against Supabase {PROJECT_REF}...")
    res = run_query(sql)
    print("Seed executed successfully!")

    # Verify counts
    tables = [
        "users", "addresses", "coupons", "orders", "order_items",
        "payments", "coupon_usages", "shipments", "notification_logs",
        "notification_events", "outbox_events", "payment_outbox_events",
        "refresh_tokens", "stripe_processed_events"
    ]

    print("\n--- Verifying Row Counts in Supabase ---")
    for t in tables:
        cnt_res = run_query(f"SELECT count(*) FROM {t};")
        cnt = cnt_res[0]["count"]
        print(f"Table '{t}': {cnt} rows")

if __name__ == "__main__":
    main()
