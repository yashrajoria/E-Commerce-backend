#!/usr/bin/env python3
"""Emit SQL that loads DynamoDB-typed JSON ({"categories":[...],"products":[...],
"product_categories":[...],"inventory":[...]}) into the catalog schema.

  python3 seed_catalog_postgres.py dynamodb_data.json | psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -1

Idempotent (ON CONFLICT DO NOTHING). Soft-deleted rows are skipped.
"""
import json
import sys


def un(v):
    (t, x), = v.items()
    if t == "N":
        return float(x) if "." in x else int(x)
    if t == "L":
        return [un(i) for i in x]
    if t == "M":
        return {k: un(i) for k, i in x.items()}
    if t == "NULL":
        return None
    return x  # S, BOOL


def q(v):
    if v is None:
        return "NULL"
    if isinstance(v, bool):
        return "true" if v else "false"
    if isinstance(v, (int, float)):
        return str(v)
    return "'" + str(v).replace("'", "''") + "'"


def jb(v):
    return q(json.dumps(v)) + "::jsonb"


def rows(name):
    return [{k: un(v) for k, v in item.items()} for item in data.get(name, [])]


data = json.load(open(sys.argv[1] if len(sys.argv) > 1 else "dynamodb_data.json"))
cats = [c for c in rows("categories") if "deleted_at" not in c]
prods = [p for p in rows("products") if "deleted_at" not in p]
live_prods = {p["id"] for p in prods}
live_cats = {c["id"] for c in cats}
links = [l for l in rows("product_categories")
         if l["product_id"] in live_prods and l["category_id"] in live_cats]

print("-- categories")
for c in cats:
    print("INSERT INTO catalog.categories (id, name, slug, image, level, is_active, parent_ids, ancestors, path, created_at, updated_at) VALUES "
          f"({q(c['id'])}, {q(c['name'])}, {q(c['slug'])}, {q(c.get('image', ''))}, {c.get('level', 0)}, {q(c.get('is_active', True))}, "
          f"{jb(c.get('parent_ids', []))}, {jb(c.get('ancestors', []))}, {jb(c.get('path', []))}, "
          f"{q(c['created_at'])}, {q(c['updated_at'])}) ON CONFLICT (id) DO NOTHING;")

print("-- products")
for p in prods:
    featured = str(p.get("is_featured", "false")).lower() == "true"
    print("INSERT INTO catalog.products (id, name, sku, price, quantity, description, brand, images, category_path, is_featured, created_at, updated_at) VALUES "
          f"({q(p['id'])}, {q(p['name'])}, {q(p['sku'])}, {p['price']}, {p.get('quantity', 0)}, {q(p.get('description', ''))}, {q(p.get('brand', ''))}, "
          f"{jb(p.get('images', []))}, {jb(p.get('category_path', []))}, {q(featured)}, "
          f"{q(p['created_at'])}, {q(p['updated_at'])}) ON CONFLICT (id) DO NOTHING;")

print("-- product_categories")
for l in links:
    print(f"INSERT INTO catalog.product_categories (category_id, product_id) VALUES ({q(l['category_id'])}, {q(l['product_id'])}) ON CONFLICT DO NOTHING;")

print("-- inventory")
for i in rows("inventory"):
    if i["id"] not in live_prods:
        continue
    print("INSERT INTO catalog.inventory (product_id, available, reserved, threshold) VALUES "
          f"({q(i['id'])}, {i.get('available', 0)}, {i.get('reserved', 0)}, {i.get('threshold', 0)}) ON CONFLICT (product_id) DO NOTHING;")
