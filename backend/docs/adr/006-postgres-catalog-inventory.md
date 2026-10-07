# ADR 006: Postgres for the catalog and inventory

## Status
Accepted (supersedes [ADR 004](./004-dynamodb-inventory.md))

## Context
The catalog (products, categories, links) and inventory lived in DynamoDB. In practice:

- The data is relational: many-to-many product/category links, a unique SKU, referential integrity, and filtered listings that needed three GSIs plus `Scan` fallbacks.
- The project cannot host LocalStack. The hosted container ran an in-memory DynamoDB Local (a JVM) that was wiped and reseeded on every restart, so the production catalog was not persistent.
- ADR 004 chose DynamoDB to avoid row-lock contention under flash-sale load. That concern is real for extreme hot keys, but the reserve path holds a row lock only for one short in-database transaction (never across network round trips), and a Redis virtual waiting room exists for flash sales (it is not yet enforced on checkout).

## Decision
Move the catalog and inventory to Postgres, in a dedicated `catalog` schema owned by catalog-service:

1. Tables `products`, `categories`, `product_categories`, `inventory`, `stock_reservations` (migrations `000015`, `000016`).
2. Reserve/release/confirm each run in one short transaction: a `stock_reservations` row keyed `(order_id, product_id)` is inserted, then `UPDATE inventory … WHERE available >= qty` decrements stock. Status moves `reserved → confirmed | released` once; replays are no-ops.
3. Products are always locked in id order, so concurrent multi-product orders cannot deadlock. CHECK constraints (`available >= 0`) are a second line of defence against overselling.
4. The service connects as a least-privilege role (`catalog_svc`) that can only use the `catalog` schema. No cross-schema joins or foreign keys, so the schema can move to its own database later by changing a connection string.

## Consequences
- **Positive**: one datastore to run and back up; real constraints and joins instead of GSIs and scans; persistent hosted catalog; the hosted image no longer carries a JVM; reservations are inspectable with SQL.
- **Negative**: a single hot product serialises its updates on one row (thousands per second is plenty at this scale; the waiting room is available to throttle a real flash sale); catalog traffic shares the instance's connection budget with the other services (pool capped at 10); losing DynamoDB's horizontal scaling for an extreme hot key.
- **Follow-ups**: keyset paging if deep product pages get slow; a single grouped query instead of one count per category in the category tree.
