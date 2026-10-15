-- seed_admin_portal_data.sql
-- Realistic seed data for ShopSwift Admin Portal features:
-- Shipments enhancement, Support Tickets, Returns, Reviews, Activity Logs, Campaigns

BEGIN;

-- 1. Enhance existing shipments with carrier, destination, and estimated dates
UPDATE shipments s
SET 
  carrier = CASE (mod(abs(hashtext(s.id::text)), 4))
    WHEN 0 THEN 'FedEx'
    WHEN 1 THEN 'UPS'
    WHEN 2 THEN 'DHL Express'
    ELSE 'Royal Mail'
  END,
  destination = COALESCE(
    (SELECT a.city || ', ' || a.state FROM addresses a JOIN orders o ON o.user_id = a.user_id WHERE o.id = s.order_id LIMIT 1),
    CASE (mod(abs(hashtext(s.id::text)), 5))
      WHEN 0 THEN 'San Francisco, CA'
      WHEN 1 THEN 'Austin, TX'
      WHEN 2 THEN 'Chicago, IL'
      WHEN 3 THEN 'Seattle, WA'
      ELSE 'New York, NY'
    END
  ),
  estimated_delivery = s.created_at + INTERVAL '5 days';

-- 2. Seed Support Tickets
DELETE FROM support_tickets;
INSERT INTO support_tickets (id, ticket_number, customer_name, customer_email, subject, priority, status, last_reply, created_at, updated_at) VALUES
('a0000001-0000-4000-8000-000000000001', 'TKT-1001', 'Alice Johnson', 'alice.johnson@shopswift-demo.test', 'Order tracking not updating for 3 days', 'high', 'open', '25 mins ago', NOW() - INTERVAL '2 hours', NOW() - INTERVAL '25 mins'),
('a0000001-0000-4000-8000-000000000002', 'TKT-1002', 'Ben Carter', 'ben.carter@shopswift-demo.test', 'Request to modify delivery address before dispatch', 'urgent', 'in-progress', '10 mins ago', NOW() - INTERVAL '4 hours', NOW() - INTERVAL '10 mins'),
('a0000001-0000-4000-8000-000000000003', 'TKT-1003', 'Chloe Nguyen', 'chloe.nguyen@shopswift-demo.test', 'Refund receipt requested for business tax records', 'low', 'resolved', '1 day ago', NOW() - INTERVAL '2 days', NOW() - INTERVAL '1 day'),
('a0000001-0000-4000-8000-000000000004', 'TKT-1004', 'Daniel Smith', 'daniel.smith@shopswift-demo.test', 'Received wrong variant of wireless earbuds', 'high', 'in-progress', '1 hour ago', NOW() - INTERVAL '6 hours', NOW() - INTERVAL '1 hour'),
('a0000001-0000-4000-8000-000000000005', 'TKT-1005', 'Elena Ruiz', 'elena.ruiz@shopswift-demo.test', 'Questions regarding warranty coverage on ultrabook', 'medium', 'open', '3 hours ago', NOW() - INTERVAL '8 hours', NOW() - INTERVAL '3 hours'),
('a0000001-0000-4000-8000-000000000006', 'TKT-1006', 'James Brown', 'james.brown1@shopswift.test', 'Coupon code SUMMER25 gave invalid error at checkout', 'medium', 'resolved', '3 days ago', NOW() - INTERVAL '4 days', NOW() - INTERVAL '3 days'),
('a0000001-0000-4000-8000-000000000007', 'TKT-1007', 'Mary Miller', 'mary.miller2@shopswift.test', 'Package marked as delivered but not in mailbox', 'urgent', 'open', '45 mins ago', NOW() - INTERVAL '3 hours', NOW() - INTERVAL '45 mins'),
('a0000001-0000-4000-8000-000000000008', 'TKT-1008', 'John Martinez', 'john.martinez3@shopswift.test', 'Inquiry about bulk enterprise discount', 'low', 'in-progress', '5 hours ago', NOW() - INTERVAL '1 day', NOW() - INTERVAL '5 hours'),
('a0000001-0000-4000-8000-000000000009', 'TKT-1009', 'Patricia Gonzalez', 'patricia.gonzalez4@shopswift.test', 'Cancellation request within 30 min window', 'medium', 'resolved', '2 days ago', NOW() - INTERVAL '3 days', NOW() - INTERVAL '2 days'),
('a0000001-0000-4000-8000-000000000010', 'TKT-1010', 'Robert Thomas', 'robert.thomas5@shopswift.test', 'Payment charged twice on Mastercard', 'urgent', 'in-progress', '15 mins ago', NOW() - INTERVAL '5 hours', NOW() - INTERVAL '15 mins'),
('a0000001-0000-4000-8000-000000000011', 'TKT-1011', 'Jennifer Jackson', 'jennifer.jackson6@shopswift.test', 'Damaged retail box on arrival', 'medium', 'open', '4 hours ago', NOW() - INTERVAL '12 hours', NOW() - INTERVAL '4 hours'),
('a0000001-0000-4000-8000-000000000012', 'TKT-1012', 'Michael Perez', 'michael.perez7@shopswift.test', 'Question on international shipping customs fees', 'low', 'closed', '5 days ago', NOW() - INTERVAL '7 days', NOW() - INTERVAL '5 days');

-- 3. Seed Returns
DELETE FROM returns;
INSERT INTO returns (id, return_number, order_id, order_number, customer_name, product_name, reason, status, amount, created_at, updated_at)
SELECT 
  gen_random_uuid(),
  'RTN-' || (1000 + row_number() OVER ()),
  o.id,
  o.order_number,
  COALESCE(u.name, 'Valued Customer'),
  COALESCE((SELECT p.name FROM order_items oi JOIN catalog.products p ON p.id = oi.product_id WHERE oi.order_id = o.id LIMIT 1), 'Electronic Accessory Pro'),
  CASE (row_number() OVER () % 5)
    WHEN 0 THEN 'Defective item on first use'
    WHEN 1 THEN 'Wrong size received'
    WHEN 2 THEN 'Changed mind before unboxing'
    WHEN 3 THEN 'Product color did not match picture'
    ELSE 'Not as described on product page'
  END,
  CASE (row_number() OVER () % 4)
    WHEN 0 THEN 'pending'
    WHEN 1 THEN 'approved'
    WHEN 2 THEN 'completed'
    ELSE 'rejected'
  END,
  ROUND((o.amount::numeric / 100.0) * 0.8, 2),
  o.created_at + INTERVAL '2 days',
  o.created_at + INTERVAL '3 days'
FROM orders o
LEFT JOIN users u ON u.id = o.user_id
LIMIT 20;

-- 4. Seed Reviews
DELETE FROM reviews;
INSERT INTO reviews (id, product_id, product_name, customer_name, rating, comment, status, helpful_count, created_at, updated_at)
SELECT 
  gen_random_uuid(),
  p.id,
  p.name,
  u.name,
  CASE ((row_number() OVER ()) % 5)
    WHEN 0 THEN 5
    WHEN 1 THEN 4
    WHEN 2 THEN 5
    WHEN 3 THEN 3
    ELSE 5
  END,
  CASE ((row_number() OVER ()) % 6)
    WHEN 0 THEN 'Absolutely incredible build quality! Fast delivery and exceeds expectations. Will definitely order again.'
    WHEN 1 THEN 'Great overall value for money. Minor setup required but once running it is smooth and dependable.'
    WHEN 2 THEN 'Super sleek aesthetics and premium feel. Everyone in the office has been asking where I got it.'
    WHEN 3 THEN 'Decent product for casual use. Battery life is slightly less than advertised under heavy load.'
    WHEN 4 THEN 'One of the best purchases I have made this quarter. Highly recommended for daily productivity.'
    ELSE 'Fantastic sound clarity, crisp highs, and deep responsive bass. 10/10 purchase.'
  END,
  CASE ((row_number() OVER ()) % 10)
    WHEN 0 THEN 'pending'
    WHEN 1 THEN 'flagged'
    ELSE 'published'
  END,
  ((row_number() OVER ()) * 3) % 45,
  NOW() - (INTERVAL '1 day' * ((row_number() OVER ()) % 60)),
  NOW() - (INTERVAL '1 day' * ((row_number() OVER ()) % 60))
FROM (SELECT id, name, row_number() OVER () as rn FROM catalog.products LIMIT 30) p
JOIN (SELECT name, row_number() OVER () as rn FROM users LIMIT 30) u ON u.rn = p.rn;

-- 5. Seed Activity Logs
DELETE FROM activity_logs;
INSERT INTO activity_logs (id, user_name, action, target, type, ip_address, created_at) VALUES
(gen_random_uuid(), 'Admin', 'Updated product pricing and stock alerts', 'Nova Wireless Mouse', 'update', '192.168.1.100', NOW() - INTERVAL '15 mins'),
(gen_random_uuid(), 'System', 'Automated nightly database checkpoint and vacuum completed', 'PostgreSQL ecommerce', 'system', '127.0.0.1', NOW() - INTERVAL '45 mins'),
(gen_random_uuid(), 'Morgan Lee', 'Created promotional discount coupon', 'SUMMER25', 'create', '192.168.1.105', NOW() - INTERVAL '2 hours'),
(gen_random_uuid(), 'Morgan Lee', 'Updated order shipping status to in-transit', 'DEMO-ORD-1003', 'update', '192.168.1.105', NOW() - INTERVAL '3 hours'),
(gen_random_uuid(), 'Admin', 'Merchant administrative login via MFA', 'Admin Console', 'auth', '192.168.1.100', NOW() - INTERVAL '5 hours'),
(gen_random_uuid(), 'System', 'Stripe payment webhook verified & reconciled', 'pi_demo_1002 ($999.00)', 'system', '54.187.21.90', NOW() - INTERVAL '7 hours'),
(gen_random_uuid(), 'Admin', 'Exported monthly financial revenue report', 'Finance CSV', 'system', '192.168.1.100', NOW() - INTERVAL '10 hours'),
(gen_random_uuid(), 'Morgan Lee', 'Archived seasonal category', 'Spring Clearance', 'delete', '192.168.1.105', NOW() - INTERVAL '1 day'),
(gen_random_uuid(), 'Admin', 'Adjusted low-stock threshold warning', 'Pulse Noise-Cancel Headphones', 'update', '192.168.1.100', NOW() - INTERVAL '1 day 3 hours'),
(gen_random_uuid(), 'System', 'Order processing queue dispatched 15 fulfillment tasks', 'SQS order-processing-queue', 'system', '127.0.0.1', NOW() - INTERVAL '1 day 6 hours'),
(gen_random_uuid(), 'Admin', 'Approved customer return request', 'RTN-1002', 'update', '192.168.1.100', NOW() - INTERVAL '2 days'),
(gen_random_uuid(), 'Morgan Lee', 'Updated store primary brand theme color', 'Store Settings', 'update', '192.168.1.105', NOW() - INTERVAL '2 days 4 hours');

-- 6. Seed Campaigns
DELETE FROM campaigns;
INSERT INTO campaigns (id, name, status, reach, conversions, revenue, start_date, end_date, created_at, updated_at) VALUES
('b0000001-0000-4000-8000-000000000001', 'Autumn Tech Kickoff 2026', 'active', 28500, 640, 18450.00, NOW() - INTERVAL '10 days', NOW() + INTERVAL '20 days', NOW() - INTERVAL '15 days', NOW()),
('b0000001-0000-4000-8000-000000000002', 'Black Friday & Cyber Week Early Access', 'scheduled', 0, 0, 0.00, NOW() + INTERVAL '30 days', NOW() + INTERVAL '37 days', NOW() - INTERVAL '5 days', NOW()),
('b0000001-0000-4000-8000-000000000003', 'Summer Flash Weekend Sale', 'completed', 19800, 480, 12900.00, NOW() - INTERVAL '60 days', NOW() - INTERVAL '57 days', NOW() - INTERVAL '65 days', NOW() - INTERVAL '57 days'),
('b0000001-0000-4000-8000-000000000004', 'VIP Customer Loyalty Rewards', 'active', 8400, 310, 9200.00, NOW() - INTERVAL '25 days', NOW() + INTERVAL '35 days', NOW() - INTERVAL '30 days', NOW()),
('b0000001-0000-4000-8000-000000000005', 'Spring Clearance Blowout', 'expired', 14200, 290, 7150.00, NOW() - INTERVAL '120 days', NOW() - INTERVAL '100 days', NOW() - INTERVAL '125 days', NOW() - INTERVAL '100 days');

COMMIT;

