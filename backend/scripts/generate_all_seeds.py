#!/usr/bin/env python3
"""
ShopSwift Full Seeder
Generates and executes seeds for:
1. LocalStack DynamoDB: Categories (110), Products (125), Inventory (125), ProductCategories (250) + S3 SVGs.
2. Supabase & Local Postgres:
   - users (120)
   - addresses (150)
   - coupons (105)
   - orders (125)
   - order_items (350)
   - payments (125)
   - coupon_usages (105)
   - shipments (105)
   - notification_logs (130)
   - notification_events (110)
   - outbox_events (125)
   - payment_outbox_events (125)
   - refresh_tokens (120)
   - stripe_processed_events (110)
"""

import json
import uuid
import datetime
import random
import os

random.seed(42)

def gen_uuid():
    return str(uuid.uuid4())

def gen_product_uuid(idx):
    tail = f"{idx:012x}"
    return f"00000000-0000-4000-8000-{tail}"

NOW = datetime.datetime.now(datetime.timezone.utc)
NOW_ISO = NOW.strftime("%Y-%m-%dT%H:%M:%SZ")

ROOT_CATEGORIES = [
    ("electronics", "2ddfa8df-beb9-4e7f-bb43-e8dd8fe9b7c0"),
    ("home", "5e91a564-bd4d-4f7f-a1b8-b4dca117c04a"),
    ("fashion", "6f57e02c-3674-4d41-a996-82f58dc1d9dd"),
    ("sports", "9bc6de24-8dca-4bc7-b16a-2e2e2c2f3a21"),
    ("beauty", "fefcad24-ec1f-4ff0-8a7a-9ab8f6d4f100"),
    ("books", "3a11b8ef-1234-4567-89ab-cdef01234567"),
    ("toys", "4b22c9f0-2345-5678-90bc-def012345678"),
    ("food", "5c33da01-3456-6789-a1cd-ef0123456789"),
    ("automotive", "6d44eb12-4567-789a-b2de-f01234567890"),
    ("wellness", "7e55fc23-5678-89ab-c3ef-012345678901")
]

SUBCATEGORIES_CONFIG = {
    "electronics": [
        ("audio", "de9f4142-26f8-49a3-abfa-0680adf4db92"),
        ("laptops", "792d2f0c-d9bd-4f68-ba95-6d3a97df8dc7"),
        ("accessories", "8f50f9f4-d26d-4a20-a5f7-b9f6ed232f9d"),
        ("smartphones", "a1010001-0000-4000-8000-000000000001"),
        ("tablets", "a1010002-0000-4000-8000-000000000002"),
        ("cameras", "a1010003-0000-4000-8000-000000000003"),
        ("gaming", "a1010004-0000-4000-8000-000000000004"),
        ("wearables", "a1010005-0000-4000-8000-000000000005"),
        ("smart-home", "a1010006-0000-4000-8000-000000000006"),
        ("monitors", "a1010007-0000-4000-8000-000000000007")
    ],
    "home": [
        ("kitchen", "760315df-f17b-4f7e-8cb8-9322d4cb7fd2"),
        ("decor", "8b9363d3-12a8-4fa3-9d9a-7d7bfecaf1dd"),
        ("furniture", "a2020001-0000-4000-8000-000000000001"),
        ("bedding", "a2020002-0000-4000-8000-000000000002"),
        ("lighting", "a2020003-0000-4000-8000-000000000003"),
        ("storage", "a2020004-0000-4000-8000-000000000004"),
        ("garden", "a2020005-0000-4000-8000-000000000005"),
        ("cleaning", "a2020006-0000-4000-8000-000000000006"),
        ("dining", "a2020007-0000-4000-8000-000000000007"),
        ("improvement", "a2020008-0000-4000-8000-000000000008")
    ],
    "fashion": [
        ("men", "9f17794b-1534-4e7f-a9ea-6d4cdccea1dd"),
        ("women", "f5644c43-2652-495a-8de5-a9bc9355137f"),
        ("footwear", "a3030001-0000-4000-8000-000000000001"),
        ("bags", "a3030002-0000-4000-8000-000000000002"),
        ("watches", "a3030003-0000-4000-8000-000000000003"),
        ("jewelry", "a3030004-0000-4000-8000-000000000004"),
        ("sunglasses", "a3030005-0000-4000-8000-000000000005"),
        ("activewear", "a3030006-0000-4000-8000-000000000006"),
        ("outerwear", "a3030007-0000-4000-8000-000000000007"),
        ("accessories", "a3030008-0000-4000-8000-000000000008")
    ],
    "sports": [
        ("running", "a9f5704a-e8f4-4df1-8d95-5ca4d70996cc"),
        ("fitness", "df444d53-d6ca-4d89-9a47-9dce2f7eb1a7"),
        ("cycling", "a4040001-0000-4000-8000-000000000001"),
        ("hiking", "a4040002-0000-4000-8000-000000000002"),
        ("yoga", "a4040003-0000-4000-8000-000000000003"),
        ("swimming", "a4040004-0000-4000-8000-000000000004"),
        ("team-sports", "a4040005-0000-4000-8000-000000000005"),
        ("winter-sports", "a4040006-0000-4000-8000-000000000006"),
        ("recovery", "a4040007-0000-4000-8000-000000000007"),
        ("training", "a4040008-0000-4000-8000-000000000008")
    ],
    "beauty": [
        ("skincare", "c5f247ca-8cb2-4639-b6b0-bda6f1c30f63"),
        ("haircare", "a5050001-0000-4000-8000-000000000001"),
        ("makeup", "a5050002-0000-4000-8000-000000000002"),
        ("fragrances", "a5050003-0000-4000-8000-000000000003"),
        ("bath-body", "a5050004-0000-4000-8000-000000000004"),
        ("oral-care", "a5050005-0000-4000-8000-000000000005"),
        ("men-grooming", "a5050006-0000-4000-8000-000000000006"),
        ("tools", "a5050007-0000-4000-8000-000000000007"),
        ("sun-care", "a5050008-0000-4000-8000-000000000008"),
        ("clean-beauty", "a5050009-0000-4000-8000-000000000009")
    ],
    "books": [
        ("fiction", "a6060001-0000-4000-8000-000000000001"),
        ("non-fiction", "a6060002-0000-4000-8000-000000000002"),
        ("sci-fi", "a6060003-0000-4000-8000-000000000003"),
        ("technology", "a6060004-0000-4000-8000-000000000004"),
        ("business", "a6060005-0000-4000-8000-000000000005"),
        ("self-help", "a6060006-0000-4000-8000-000000000006"),
        ("stationery", "a6060007-0000-4000-8000-000000000007"),
        ("notebooks", "a6060008-0000-4000-8000-000000000008"),
        ("art-supplies", "a6060009-0000-4000-8000-000000000009"),
        ("writing", "a6060010-0000-4000-8000-000000000010")
    ],
    "toys": [
        ("action-figures", "a7070001-0000-4000-8000-000000000001"),
        ("board-games", "a7070002-0000-4000-8000-000000000002"),
        ("building-sets", "a7070003-0000-4000-8000-000000000003"),
        ("puzzles", "a7070004-0000-4000-8000-000000000004"),
        ("educational", "a7070005-0000-4000-8000-000000000005"),
        ("outdoor-toys", "a7070006-0000-4000-8000-000000000006"),
        ("video-games", "a7070007-0000-4000-8000-000000000007"),
        ("collectibles", "a7070008-0000-4000-8000-000000000008"),
        ("dolls", "a7070009-0000-4000-8000-000000000009"),
        ("rc-vehicles", "a7070010-0000-4000-8000-000000000010")
    ],
    "food": [
        ("coffee", "a8080001-0000-4000-8000-000000000001"),
        ("tea", "a8080002-0000-4000-8000-000000000002"),
        ("snacks", "a8080003-0000-4000-8000-000000000003"),
        ("pantry", "a8080004-0000-4000-8000-000000000004"),
        ("organic", "a8080005-0000-4000-8000-000000000005"),
        ("beverages", "a8080006-0000-4000-8000-000000000006"),
        ("breakfast", "a8080007-0000-4000-8000-000000000007"),
        ("spices", "a8080008-0000-4000-8000-000000000008"),
        ("chocolate", "a8080009-0000-4000-8000-000000000009"),
        ("baking", "a8080010-0000-4000-8000-000000000010")
    ],
    "automotive": [
        ("car-electronics", "a9090001-0000-4000-8000-000000000001"),
        ("detailing", "a9090002-0000-4000-8000-000000000002"),
        ("tools", "a9090003-0000-4000-8000-000000000003"),
        ("interior-accessories", "a9090004-0000-4000-8000-000000000004"),
        ("emergency", "a9090005-0000-4000-8000-000000000005"),
        ("motorcycle", "a9090006-0000-4000-8000-000000000006"),
        ("maintenance", "a9090007-0000-4000-8000-000000000007"),
        ("tires-wheels", "a9090008-0000-4000-8000-000000000008"),
        ("lighting", "a9090009-0000-4000-8000-000000000009"),
        ("oils-fluids", "a9090010-0000-4000-8000-000000000010")
    ],
    "wellness": [
        ("vitamins", "b1010001-0000-4000-8000-000000000001"),
        ("sleep", "b1010002-0000-4000-8000-000000000002"),
        ("aromatherapy", "b1010003-0000-4000-8000-000000000003"),
        ("trackers", "b1010004-0000-4000-8000-000000000004"),
        ("first-aid", "b1010005-0000-4000-8000-000000000005"),
        ("meditation", "b1010006-0000-4000-8000-000000000006"),
        ("massage", "b1010007-0000-4000-8000-000000000007"),
        ("hydration", "b1010008-0000-4000-8000-000000000008"),
        ("air-quality", "b1010009-0000-4000-8000-000000000009"),
        ("supplements", "b1010010-0000-4000-8000-000000000010")
    ]
}

# Build lookup table for subcategory uuid
CAT_LOOKUP = {}
for root_slug, root_id in ROOT_CATEGORIES:
    CAT_LOOKUP[root_slug] = root_id
    if root_slug in SUBCATEGORIES_CONFIG:
        for sub_slug, sub_id in SUBCATEGORIES_CONFIG[root_slug]:
            CAT_LOOKUP[f"{root_slug}/{sub_slug}"] = sub_id

from generate_seed_data import PRODUCTS_DATA

def build_dynamodb_script():
    categories_items = []
    
    # 1. Root categories (10 items)
    for root_slug, root_id in ROOT_CATEGORIES:
        categories_items.append({
            "id": {"S": root_id},
            "name": {"S": root_slug},
            "slug": {"S": root_slug},
            "path": {"L": [{"S": root_slug}]},
            "level": {"N": "0"},
            "parent_ids": {"L": []},
            "ancestors": {"L": []},
            "is_active": {"BOOL": True},
            "created_at": {"S": NOW_ISO},
            "updated_at": {"S": NOW_ISO}
        })

    # 2. Subcategories (100 items)
    for root_slug, subs in SUBCATEGORIES_CONFIG.items():
        root_id = CAT_LOOKUP[root_slug]
        for sub_slug, sub_id in subs:
            categories_items.append({
                "id": {"S": sub_id},
                "name": {"S": sub_slug},
                "slug": {"S": sub_slug},
                "path": {"L": [{"S": root_slug}, {"S": sub_slug}]},
                "level": {"N": "1"},
                "parent_ids": {"L": [{"S": root_id}]},
                "ancestors": {"L": [{"S": root_id}]},
                "is_active": {"BOOL": True},
                "created_at": {"S": NOW_ISO},
                "updated_at": {"S": NOW_ISO}
            })

    # 3. Products, Inventory, ProductCategories
    products_items = []
    inventory_items = []
    product_categories_items = []
    svg_uploads = []

    for idx, (name, price, qty, desc, brand, sku, cat_root, cat_sub, is_feat) in enumerate(PRODUCTS_DATA, start=1):
        pid = gen_product_uuid(idx)
        root_id = CAT_LOOKUP[cat_root]
        sub_key = f"{cat_root}/{cat_sub}"
        sub_id = CAT_LOOKUP.get(sub_key, root_id)

        safe_sku = sku.lower().replace(" ", "-")
        front_img = f"http://localhost:4566/shopswift/products/demo/{safe_sku}-front.svg"
        back_img = f"http://localhost:4566/shopswift/products/demo/{safe_sku}-back.svg"

        # Product item
        products_items.append({
            "id": {"S": pid},
            "name": {"S": name},
            "price": {"N": str(price)},
            "quantity": {"N": str(qty)},
            "description": {"S": desc},
            "images": {"L": [{"S": front_img}, {"S": back_img}]},
            "brand": {"S": brand},
            "sku": {"S": sku},
            "category_ids": {"L": [{"S": root_id}, {"S": sub_id}]},
            "category_path": {"L": [{"S": cat_root}, {"S": cat_sub}]},
            "is_featured": {"S": "true" if is_feat else "false"},
            "created_at": {"S": NOW_ISO},
            "updated_at": {"S": NOW_ISO}
        })

        # Inventory item
        inventory_items.append({
            "id": {"S": pid},
            "available": {"N": str(qty)},
            "reserved": {"N": "0"},
            "threshold": {"N": "5"},
            "order_reservations": {"M": {}},
            "updated_at": {"S": NOW_ISO}
        })

        # ProductCategories mappings (2 per product)
        product_categories_items.append({
            "category_id": {"S": root_id},
            "product_id": {"S": pid},
            "created_at": {"S": NOW_ISO},
            "updated_at": {"S": NOW_ISO}
        })
        product_categories_items.append({
            "category_id": {"S": sub_id},
            "product_id": {"S": pid},
            "created_at": {"S": NOW_ISO},
            "updated_at": {"S": NOW_ISO}
        })

        # S3 SVG upload tuple
        svg_uploads.append({"safe_sku": safe_sku, "name": name, "brand": brand, "sku": sku})

    return {
        "categories": categories_items,
        "products": products_items,
        "inventory": inventory_items,
        "product_categories": product_categories_items,
        "svg_list": svg_uploads
    }

def build_dynamodb_worker():
    return '''import boto3
import json
import sys

s3 = boto3.client('s3', endpoint_url='http://localhost:4566', region_name='us-east-1', aws_access_key_id='test', aws_secret_access_key='test')
dynamodb = boto3.client('dynamodb', endpoint_url='http://localhost:4566', region_name='us-east-1', aws_access_key_id='test', aws_secret_access_key='test')

try:
    s3.create_bucket(Bucket='shopswift')
except Exception:
    pass

with open('/tmp/dynamodb_data.json', 'r') as f:
    data = json.load(f)

categories_items = data['categories']
products_items = data['products']
inventory_items = data['inventory']
product_categories_items = data['product_categories']
svg_list = data['svg_list']

print(f"Uploading {len(svg_list)} SVG image pairs to S3...")
for item in svg_list:
    safe_sku = item['safe_sku']
    name = item['name']
    brand = item['brand']
    sku = item['sku']

    svg_front = f"""<svg xmlns='http://www.w3.org/2000/svg' width='1200' height='1200' viewBox='0 0 1200 1200'>
<rect width='1200' height='1200' fill='#f4f6fb'/>
<rect x='80' y='80' width='1040' height='1040' rx='36' fill='#ffffff' stroke='#e2e8f0' stroke-width='4'/>
<circle cx='600' cy='380' r='160' fill='#4f46e5' opacity='0.12'/>
<text x='600' y='395' font-family='sans-serif' font-size='72' font-weight='bold' text-anchor='middle' fill='#4f46e5'>{brand[:2].upper()}</text>
<text x='600' y='640' font-family='sans-serif' font-size='42' font-weight='600' text-anchor='middle' fill='#1e293b'>{name[:32]}</text>
<text x='600' y='710' font-family='sans-serif' font-size='28' text-anchor='middle' fill='#64748b'>{brand} • {sku}</text>
<rect x='520' y='760' width='160' height='40' rx='20' fill='#e0e7ff'/>
<text x='600' y='788' font-family='sans-serif' font-size='20' font-weight='600' text-anchor='middle' fill='#4338ca'>PREMIUM</text>
</svg>"""
    svg_back = f"""<svg xmlns='http://www.w3.org/2000/svg' width='1200' height='1200' viewBox='0 0 1200 1200'>
<rect width='1200' height='1200' fill='#f8fafc'/>
<rect x='80' y='80' width='1040' height='1040' rx='36' fill='#ffffff' stroke='#e2e8f0' stroke-width='4'/>
<text x='600' y='580' font-family='sans-serif' font-size='36' font-weight='600' text-anchor='middle' fill='#334155'>{name[:32]} (Rear)</text>
<text x='600' y='640' font-family='sans-serif' font-size='24' text-anchor='middle' fill='#94a3b8'>{sku}</text>
</svg>"""
    s3.put_object(Bucket='shopswift', Key=f'products/demo/{safe_sku}-front.svg', Body=svg_front.encode('utf-8'), ContentType='image/svg+xml')
    s3.put_object(Bucket='shopswift', Key=f'products/demo/{safe_sku}-back.svg', Body=svg_back.encode('utf-8'), ContentType='image/svg+xml')

def batch_write(table_name, items):
    print(f"Writing {len(items)} items to {table_name}...")
    for i in range(0, len(items), 25):
        chunk = items[i:i+25]
        reqs = [{'PutRequest': {'Item': it}} for it in chunk]
        dynamodb.batch_write_item(RequestItems={table_name: reqs})

batch_write('Categories', categories_items)
batch_write('Products', products_items)
batch_write('Inventory', inventory_items)
batch_write('ProductCategories', product_categories_items)

print("DynamoDB successfully seeded!")
'''


# --- POSTGRES / SUPABASE DATA GENERATION ---

FIRST_NAMES = [
    "James", "Mary", "John", "Patricia", "Robert", "Jennifer", "Michael", "Linda",
    "William", "Elizabeth", "David", "Barbara", "Richard", "Susan", "Joseph", "Jessica",
    "Thomas", "Sarah", "Charles", "Karen", "Christopher", "Nancy", "Daniel", "Lisa",
    "Matthew", "Betty", "Anthony", "Margaret", "Mark", "Sandra", "Donald", "Ashley",
    "Steven", "Kimberly", "Paul", "Emily", "Andrew", "Donna", "Joshua", "Michelle",
    "Kenneth", "Dorothy", "Kevin", "Carol", "Brian", "Amanda", "George", "Melissa",
    "Edward", "Deborah", "Ronald", "Stephanie", "Timothy", "Rebecca", "Jason", "Sharon",
    "Jeffrey", "Laura", "Ryan", "Cynthia", "Jacob", "Kathleen", "Gary", "Amy",
    "Nicholas", "Angela", "Eric", "Shirley", "Jonathan", "Anna", "Stephen", "Brenda",
    "Larry", "Pamela", "Justin", "Emma", "Scott", "Nicole", "Brandon", "Helen",
    "Benjamin", "Samantha", "Samuel", "Katherine", "Gregory", "Christine", "Frank", "Debra",
    "Alexander", "Rachel", "Raymond", "Carolyn", "Patrick", "Janet", "Jack", "Catherine",
    "Dennis", "Maria", "Jerry", "Heather", "Tyler", "Diane", "Aaron", "Ruth",
    "Jose", "Julie", "Adam", "Olivia", "Nathan", "Joyce", "Henry", "Virginia",
    "Douglas", "Victoria", "Zachary", "Kelly", "Peter", "Lauren", "Kyle", "Christina"
]

LAST_NAMES = [
    "Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller", "Davis",
    "Rodriguez", "Martinez", "Hernandez", "Lopez", "Gonzalez", "Wilson", "Anderson", "Thomas",
    "Taylor", "Moore", "Jackson", "Martin", "Lee", "Perez", "Thompson", "White",
    "Harris", "Sanchez", "Clark", "Ramirez", "Lewis", "Robinson", "Walker", "Young",
    "Allen", "King", "Wright", "Scott", "Torres", "Nguyen", "Hill", "Flores",
    "Green", "Adams", "Nelson", "Baker", "Hall", "Rivera", "Campbell", "Mitchell",
    "Carter", "Roberts", "Gomez", "Phillips", "Evans", "Turner", "Diaz", "Parker",
    "Cruz", "Edwards", "Collins", "Reyes", "Stewart", "Morris", "Morales", "Murphy",
    "Cook", "Rogers", "Gutierrez", "Ortiz", "Morgan", "Cooper", "Peterson", "Bailey"
]

CITIES_STATES = [
    ("New York", "NY", "10001", "US"),
    ("Los Angeles", "CA", "90001", "US"),
    ("Chicago", "IL", "60601", "US"),
    ("Houston", "TX", "77001", "US"),
    ("Phoenix", "AZ", "85001", "US"),
    ("Philadelphia", "PA", "19101", "US"),
    ("San Antonio", "TX", "78201", "US"),
    ("San Diego", "CA", "92101", "US"),
    ("Dallas", "TX", "75201", "US"),
    ("San Jose", "CA", "95101", "US"),
    ("Austin", "TX", "78701", "US"),
    ("Jacksonville", "FL", "32201", "US"),
    ("Fort Worth", "TX", "76101", "US"),
    ("Columbus", "OH", "43201", "US"),
    ("Charlotte", "NC", "28201", "US"),
    ("San Francisco", "CA", "94102", "US"),
    ("Indianapolis", "IN", "46201", "US"),
    ("Seattle", "WA", "98101", "US"),
    ("Denver", "CO", "80201", "US"),
    ("Washington", "DC", "20001", "US"),
    ("Boston", "MA", "02108", "US"),
    ("Miami", "FL", "33101", "US"),
    ("London", "ENG", "EC1A 1BB", "GB"),
    ("Manchester", "ENG", "M1 1AE", "GB"),
    ("Toronto", "ON", "M5H 2N2", "CA")
]

STREET_NAMES = [
    "Maple Ave", "Oak St", "Cedar Lane", "Pine Road", "Market St", "Main St",
    "Broadway", "Sunset Blvd", "Ocean Ave", "Highland Park", "Washington St",
    "Park Avenue", "Beacon St", "Michigan Ave", "Peachtree St", "Wall Street",
    "Elm Street", "Chestnut St", "Lincoln Way", "Lexington Ave"
]

DEMO_PASSWORD_HASH = "$2a$10$.y7nJxrJv1pmQELdYvPv9uOwgHwTD6GjVFaKDYRawkB58JE5oxPMe"

def build_postgres_sql():
    statements = []
    statements.append("BEGIN;")
    statements.append("""
DELETE FROM shipments;
DELETE FROM payments;
DELETE FROM order_items;
DELETE FROM coupon_usages;
DELETE FROM orders;
DELETE FROM coupons;
DELETE FROM notification_logs;
DELETE FROM notification_events;
DELETE FROM outbox_events;
DELETE FROM payment_outbox_events;
DELETE FROM stripe_processed_events;
DELETE FROM refresh_tokens WHERE user_id != '9a22d068-a164-4300-b7a6-555cffea3390';
UPDATE users SET billing_address_id = NULL, shipping_address_id = NULL WHERE id != '9a22d068-a164-4300-b7a6-555cffea3390';
DELETE FROM addresses WHERE user_id != '9a22d068-a164-4300-b7a6-555cffea3390';
DELETE FROM users WHERE email != 'admin@example.com';
""")

    # 1. USERS (120 users)
    users = []
    used_emails = set()
    used_phones = set()

    for i in range(1, 121):
        uid = f"10000000-0000-4000-8000-{i:012x}"
        fname = FIRST_NAMES[(i - 1) % len(FIRST_NAMES)]
        lname = LAST_NAMES[(i * 3) % len(LAST_NAMES)]
        name = f"{fname} {lname}"
        email = f"{fname.lower()}.{lname.lower()}{i}@shopswift.test"
        while email in used_emails:
            email = f"{fname.lower()}.{lname.lower()}{i}_{random.randint(10,99)}@shopswift.test"
        used_emails.add(email)

        phone = f"+155501{i:04d}"
        used_phones.add(phone)

        role = "admin" if i <= 3 else "user"
        store_name = "NULL"

        # Random days ago (1 to 180 days)
        days_ago = (120 - i) + 5
        c_at = (NOW - datetime.timedelta(days=days_ago, hours=i%24)).strftime("%Y-%m-%d %H:%M:%S+00")
        
        users.append({
            "id": uid,
            "email": email,
            "name": name,
            "phone": phone,
            "role": role,
            "store_name": store_name,
            "created_at": c_at
        })

    # Users SQL
    user_vals = []
    for u in users:
        safe_name = u['name'].replace("'", "''")
        user_vals.append(
            f"('{u['id']}', '{u['email']}', '{DEMO_PASSWORD_HASH}', '{safe_name}', true, NULL, 0, NULL, 0, NULL, {u['store_name']}, '{u['role']}', '{u['phone']}', NULL, NULL, '{u['created_at']}', '{u['created_at']}', NULL)"
        )
    statements.append(
        "INSERT INTO users (id, email, password, name, email_verified, verification_code, verification_attempts, verification_locked_until, login_attempts, login_locked_until, store_name, role, phone_number, billing_address_id, shipping_address_id, created_at, updated_at, deleted_at) VALUES\n"
        + ",\n".join(user_vals)
        + "\nON CONFLICT (id) DO NOTHING;"
    )

    # 2. ADDRESSES (150 addresses: 120 primary shipping + 30 alternate billing)
    addresses = []
    for i in range(1, 151):
        aid = f"20000000-0000-4000-8000-{i:012x}"
        uid = users[(i - 1) % len(users)]["id"]
        addr_type = "shipping" if i <= 120 else "billing"
        city, state, zip_code, country = CITIES_STATES[i % len(CITIES_STATES)]
        street = f"{100 + i * 7} {STREET_NAMES[i % len(STREET_NAMES)]}"
        c_at = users[(i - 1) % len(users)]["created_at"]
        addresses.append({
            "id": aid,
            "user_id": uid,
            "type": addr_type,
            "street": street,
            "city": city,
            "state": state,
            "postal_code": zip_code,
            "country": country,
            "created_at": c_at
        })

    addr_vals = []
    for a in addresses:
        safe_street = a['street'].replace("'", "''")
        addr_vals.append(
            f"('{a['id']}', '{a['user_id']}', '{a['type']}', '{safe_street}', '{a['city']}', '{a['state']}', '{a['postal_code']}', '{a['country']}', '{a['created_at']}', '{a['created_at']}', NULL)"
        )
    statements.append(
        "INSERT INTO addresses (id, user_id, type, street, city, state, postal_code, country, created_at, updated_at, deleted_at) VALUES\n"
        + ",\n".join(addr_vals)
        + "\nON CONFLICT (id) DO NOTHING;"
    )

    # Link primary shipping & billing address on users
    for i in range(1, 121):
        uid = users[i-1]["id"]
        ship_id = addresses[i-1]["id"]
        bill_id = ship_id if i > 30 else addresses[120 + (i-1) % 30]["id"]
        statements.append(f"UPDATE users SET shipping_address_id = '{ship_id}', billing_address_id = '{bill_id}' WHERE id = '{uid}';")

    # 3. COUPONS (105 coupons)
    coupons = []
    curated_codes = [
        ("SWIFT-WELCOME10", "percentage", 10.0, 25.0, 5000, 142),
        ("SWIFT-SAVE20", "percentage", 20.0, 50.0, 2000, 310),
        ("SWIFT-SUMMER25", "percentage", 25.0, 75.0, 1000, 89),
        ("SWIFT-VIP50", "flat", 50.0, 150.0, 500, 45),
        ("SWIFT-BLACKFRIDAY30", "percentage", 30.0, 100.0, 5000, 890),
        ("SWIFT-FREESHIP", "freeshipping", 0.0, 30.0, 10000, 420),
        ("SWIFT-TECH15", "percentage", 15.0, 100.0, 1500, 115),
        ("SWIFT-SPRING10", "percentage", 10.0, 40.0, 2000, 95),
        ("SWIFT-FLASH40", "percentage", 40.0, 120.0, 500, 80),
        ("SWIFT-HOLIDAY25", "percentage", 25.0, 80.0, 3000, 230)
    ]
    for idx, (code, ctype, val, min_order, limit, used) in enumerate(curated_codes, start=1):
        cid = f"30000000-0000-4000-8000-{idx:012x}"
        coupons.append({
            "id": cid, "code": code, "type": ctype, "value": val,
            "min_order": min_order, "limit": limit, "used": used
        })

    # Add 95 loyalty coupons to reach 105 total
    for idx in range(11, 106):
        cid = f"30000000-0000-4000-8000-{idx:012x}"
        code = f"SWIFT-LOYALTY{idx:03d}"
        ctype = "percentage" if idx % 2 == 0 else "flat"
        val = 15.0 if ctype == "percentage" else 20.0
        min_order = 40.0
        limit = 500
        used = idx % 25
        coupons.append({
            "id": cid, "code": code, "type": ctype, "value": val,
            "min_order": min_order, "limit": limit, "used": used
        })

    coupon_vals = []
    for c in coupons:
        exp = (NOW + datetime.timedelta(days=365)).strftime("%Y-%m-%d %H:%M:%S+00")
        coupon_vals.append(
            f"('{c['id']}', '{c['code']}', '{c['type']}', {c['value']}, {c['min_order']}, {c['limit']}, {c['used']}, '{exp}', true, '{NOW_ISO}', '{NOW_ISO}', NULL)"
        )
    statements.append(
        "INSERT INTO coupons (id, code, type, value, min_order_value, usage_limit, used_count, expires_at, active, created_at, updated_at, deleted_at) VALUES\n"
        + ",\n".join(coupon_vals)
        + "\nON CONFLICT (id) DO NOTHING;"
    )

    # 4. ORDERS & ORDER ITEMS & PAYMENTS (125 orders)
    orders = []
    order_items = []
    payments = []
    coupon_usages = []
    shipments = []
    outbox_events = []
    payment_outbox_events = []

    statuses = [
        "delivered", "delivered", "delivered", "delivered", # heavy delivered
        "shipped", "shipped",
        "paid", "paid",
        "completed",
        "pending_payment",
        "cancelled"
    ]

    for i in range(1, 126):
        oid = f"40000000-0000-4000-8000-{i:012x}"
        onum = f"ORD-2026-{1000 + i}"
        idemp = f"idemp-order-2026-{i:04d}"
        u_idx = (i - 1) % len(users)
        uid = users[u_idx]["id"]
        stat = statuses[i % len(statuses)]

        days_ago = (125 - i) + 1
        created_dt = NOW - datetime.timedelta(days=days_ago, hours=(i*3)%24)
        c_at = created_dt.strftime("%Y-%m-%d %H:%M:%S+00")

        comp_at = "NULL"
        canc_at = "NULL"
        if stat in ("delivered", "completed"):
            comp_at = f"'{(created_dt + datetime.timedelta(days=2)).strftime('%Y-%m-%d %H:%M:%S+00')}'"
        elif stat == "cancelled":
            canc_at = f"'{(created_dt + datetime.timedelta(hours=4)).strftime('%Y-%m-%d %H:%M:%S+00')}'"

        # Apply coupon occasionally
        coupon_code = "NULL"
        coupon_id = "NULL"
        discount_amount = 0
        if i <= 105:
            c_idx = (i - 1) % len(coupons)
            c = coupons[c_idx]
            coupon_code = f"'{c['code']}'"
            coupon_id = f"'{c['id']}'"
            discount_amount = int(c['value'] * 100) if c['type'] == 'flat' else 1500

        # Create 2-3 order items
        num_items = 2 if i % 2 == 0 else 3
        order_total = 0
        for it_idx in range(num_items):
            item_id = gen_uuid()
            p_idx = ((i * 7) + it_idx * 11) % len(PRODUCTS_DATA) + 1
            prod_uuid = gen_product_uuid(p_idx)
            item_price = PRODUCTS_DATA[p_idx - 1][1]
            qty = 1 if it_idx > 0 else (1 + (i % 2))
            order_total += item_price * qty
            order_items.append({
                "id": item_id,
                "order_id": oid,
                "product_id": prod_uuid,
                "quantity": qty,
                "price": item_price
            })

        final_amount = max(0, order_total - discount_amount)

        orders.append({
            "id": oid,
            "order_number": onum,
            "idempotency_key": idemp,
            "user_id": uid,
            "amount": final_amount,
            "coupon_code": coupon_code,
            "coupon_id": coupon_id,
            "discount_amount": discount_amount,
            "status": stat,
            "completed_at": comp_at,
            "canceled_at": canc_at,
            "created_at": c_at
        })

        if coupon_id != "NULL":
            coupon_usages.append({
                "id": gen_uuid(),
                "coupon_id": coupons[c_idx]["id"],
                "order_id": oid,
                "user_id": uid,
                "created_at": c_at
            })

        # Payment record
        pmt_id = f"50000000-0000-4000-8000-{i:012x}"
        pmt_status = "succeeded" if stat in ("delivered", "shipped", "paid", "completed") else ("failed" if stat == "cancelled" else "pending")
        stripe_pi = f"pi_test_{i:06d}_{gen_uuid()[:8]}"
        succ_at = f"'{c_at}'" if pmt_status == "succeeded" else "NULL"
        fail_at = canc_at if pmt_status == "failed" else "NULL"
        event_payload = json.dumps({"id": stripe_pi, "amount": final_amount, "currency": "usd", "status": pmt_status}).replace("'", "''")

        payments.append({
            "payment_id": pmt_id,
            "event_id": f"evt_pmt_{i:06d}",
            "idempotency_key": f"idemp-pmt-{i:04d}",
            "correlation_id": f"corr-pmt-{i:04d}",
            "order_id": oid,
            "user_id": uid,
            "amount": final_amount,
            "currency": "USD",
            "status": pmt_status,
            "checkout_url": f"https://checkout.stripe.com/c/pay/cs_test_{i:06d}",
            "stripe_payment_id": stripe_pi,
            "stripe_event_payload": f"'{event_payload}'::jsonb",
            "succeeded_at": succ_at,
            "failed_at": fail_at,
            "created_at": c_at
        })

        # Shipment record for shipped/delivered/completed
        if stat in ("delivered", "shipped", "completed"):
            ship_id = gen_uuid()
            track_code = f"1Z999AA101234{1000+i}" if i % 2 == 0 else f"940010000000{100000+i}"
            ship_status = "delivered" if stat in ("delivered", "completed") else "in_transit"
            shipments.append({
                "id": ship_id,
                "order_id": oid,
                "tracking_code": track_code,
                "status": ship_status,
                "created_at": c_at
            })

        # Outbox event
        outbox_payload = json.dumps({"order_id": oid, "order_number": onum, "user_id": uid, "amount": final_amount, "status": stat}).replace("'", "''")
        outbox_events.append({
            "id": gen_uuid(),
            "aggregate_type": "ORDER",
            "aggregate_id": oid,
            "event_type": "order.created" if stat == "pending_payment" else "order.paid",
            "destination_type": "sns",
            "destination": "arn:aws:sns:us-east-1:000000000000:order-events",
            "payload": f"'{outbox_payload}'::jsonb",
            "status": "published",
            "created_at": c_at
        })

        # Payment outbox event
        pmt_outbox_payload = json.dumps({"payment_id": pmt_id, "order_id": oid, "amount": final_amount, "status": pmt_status}).replace("'", "''")
        payment_outbox_events.append({
            "id": gen_uuid(),
            "aggregate_type": "PAYMENT",
            "aggregate_id": pmt_id,
            "event_type": f"payment.{pmt_status}",
            "destination_type": "sns",
            "destination": "arn:aws:sns:us-east-1:000000000000:payment-events",
            "payload": f"'{pmt_outbox_payload}'::jsonb",
            "status": "published",
            "created_at": c_at
        })

    # Orders SQL
    ord_vals = []
    for o in orders:
        ord_vals.append(
            f"('{o['id']}', '{o['order_number']}', '{o['idempotency_key']}', '{o['user_id']}', {o['amount']}, {o['coupon_code']}, {o['discount_amount']}, '{o['status']}', {o['canceled_at']}, {o['completed_at']}, '{o['created_at']}', '{o['created_at']}', NULL, {o['coupon_id']})"
        )
    statements.append(
        "INSERT INTO orders (id, order_number, idempotency_key, user_id, amount, coupon_code, discount_amount, status, canceled_at, completed_at, created_at, updated_at, deleted_at, coupon_id) VALUES\n"
        + ",\n".join(ord_vals)
        + "\nON CONFLICT (id) DO NOTHING;"
    )

    # Order Items SQL
    it_vals = []
    for it in order_items:
        it_vals.append(
            f"('{it['id']}', '{it['order_id']}', '{it['product_id']}', {it['quantity']}, {it['price']})"
        )
    statements.append(
        "INSERT INTO order_items (id, order_id, product_id, quantity, price) VALUES\n"
        + ",\n".join(it_vals)
        + "\nON CONFLICT (id) DO NOTHING;"
    )

    # Payments SQL
    pmt_vals = []
    for p in payments:
        pmt_vals.append(
            f"('{p['payment_id']}', '{p['event_id']}', '{p['idempotency_key']}', '{p['correlation_id']}', '{p['order_id']}', '{p['user_id']}', {p['amount']}, '{p['currency']}', '{p['status']}', '{p['checkout_url']}', '{p['stripe_payment_id']}', {p['stripe_event_payload']}, {p['succeeded_at']}, {p['failed_at']}, '{p['created_at']}', '{p['created_at']}', NULL)"
        )
    statements.append(
        "INSERT INTO payments (payment_id, event_id, idempotency_key, correlation_id, order_id, user_id, amount, currency, status, checkout_url, stripe_payment_id, stripe_event_payload, succeeded_at, failed_at, created_at, updated_at, deleted_at) VALUES\n"
        + ",\n".join(pmt_vals)
        + "\nON CONFLICT (payment_id) DO NOTHING;"
    )

    # Coupon Usages SQL
    if coupon_usages:
        cu_vals = []
        for cu in coupon_usages:
            cu_vals.append(f"('{cu['id']}', '{cu['coupon_id']}', '{cu['order_id']}', '{cu['user_id']}', '{cu['created_at']}')")
        statements.append(
            "INSERT INTO coupon_usages (id, coupon_id, order_id, user_id, created_at) VALUES\n"
            + ",\n".join(cu_vals)
            + "\nON CONFLICT (id) DO NOTHING;"
        )

    # Shipments SQL (105+ shipments)
    # If not already 105, pad shipments
    while len(shipments) < 105:
        idx = len(shipments) + 1
        o = orders[idx % len(orders)]
        shipments.append({
            "id": gen_uuid(),
            "order_id": o["id"],
            "tracking_code": f"DHL-EXPRESS-{900000 + idx}",
            "status": "delivered",
            "created_at": o["created_at"]
        })

    ship_vals = []
    for s in shipments:
        ship_vals.append(f"('{s['id']}', '{s['order_id']}', '{s['tracking_code']}', '{s['status']}', '{s['created_at']}', '{s['created_at']}')")
    statements.append(
        "INSERT INTO shipments (id, order_id, tracking_code, status, created_at, updated_at) VALUES\n"
        + ",\n".join(ship_vals)
        + "\nON CONFLICT (id) DO NOTHING;"
    )

    # Outbox Events SQL (125)
    outbox_vals = []
    for ob in outbox_events:
        outbox_vals.append(
            f"('{ob['id']}', '{ob['aggregate_type']}', '{ob['aggregate_id']}', '{ob['event_type']}', '{ob['destination_type']}', '{ob['destination']}', {ob['payload']}, '{ob['status']}', 1, '{ob['created_at']}', NULL, NULL, '{ob['created_at']}', '{ob['created_at']}', NULL, '{ob['created_at']}', '{ob['created_at']}')"
        )
    statements.append(
        "INSERT INTO outbox_events (id, aggregate_type, aggregate_id, event_type, destination_type, destination, payload, status, attempts, available_at, lease_owner, lease_expires_at, claimed_at, published_at, last_error, created_at, updated_at) VALUES\n"
        + ",\n".join(outbox_vals)
        + "\nON CONFLICT (id) DO NOTHING;"
    )

    # Payment Outbox Events SQL (125)
    p_outbox_vals = []
    for pob in payment_outbox_events:
        p_outbox_vals.append(
            f"('{pob['id']}', '{pob['aggregate_type']}', '{pob['aggregate_id']}', '{pob['event_type']}', '{pob['destination_type']}', '{pob['destination']}', {pob['payload']}, '{pob['status']}', 1, '{pob['created_at']}', NULL, NULL, '{pob['created_at']}', '{pob['created_at']}', NULL, '{pob['created_at']}', '{pob['created_at']}')"
        )
    statements.append(
        "INSERT INTO payment_outbox_events (id, aggregate_type, aggregate_id, event_type, destination_type, destination, payload, status, attempts, available_at, lease_owner, lease_expires_at, claimed_at, published_at, last_error, created_at, updated_at) VALUES\n"
        + ",\n".join(p_outbox_vals)
        + "\nON CONFLICT (id) DO NOTHING;"
    )

    # 5. REFRESH TOKENS (120 tokens, 1 per user)
    rt_vals = []
    for i, u in enumerate(users, start=1):
        tid = f"rt_live_{i:04d}_{gen_uuid()[:16]}"
        exp = (NOW + datetime.timedelta(days=30)).strftime("%Y-%m-%d %H:%M:%S+00")
        rt_vals.append(
            f"('{gen_uuid()}', '{tid}', '{u['id']}', '{gen_uuid()}', false, '{exp}', '{u['created_at']}', NULL)"
        )
    statements.append(
        "INSERT INTO refresh_tokens (id, token_id, user_id, family_id, revoked, expires_at, created_at, revoked_at) VALUES\n"
        + ",\n".join(rt_vals)
        + "\nON CONFLICT (token_id) DO NOTHING;"
    )

    # 6. NOTIFICATION LOGS (130 logs)
    notif_vals = []
    for i in range(1, 131):
        u = users[(i - 1) % len(users)]
        ntype = "ORDER_CONFIRMATION" if i % 2 == 0 else ("SHIPPING_UPDATE" if i % 3 == 0 else "WELCOME_EMAIL")
        chan = "email" if i % 4 != 0 else "sms"
        recip = u["email"] if chan == "email" else u["phone"]
        c_at = u["created_at"]
        notif_vals.append(
            f"('{u['id']}', '{recip}', '{ntype}', '{chan}', 'sent', NULL, 0, '{c_at}')"
        )
    statements.append(
        "INSERT INTO notification_logs (user_id, recipient, type, channel, status, error, retry_count, created_at) VALUES\n"
        + ",\n".join(notif_vals)
        + ";"
    )

    # 7. NOTIFICATION EVENTS (110 events)
    ne_vals = []
    for i in range(1, 111):
        eid = f"notif_evt_{i:04d}_{gen_uuid()[:12]}"
        c_at = (NOW - datetime.timedelta(days=(110 - i))).strftime("%Y-%m-%d %H:%M:%S+00")
        ne_vals.append(f"('{eid}', 'delivered', '{c_at}')")
    statements.append(
        "INSERT INTO notification_events (event_id, status, processed_at) VALUES\n"
        + ",\n".join(ne_vals)
        + "\nON CONFLICT (event_id) DO NOTHING;"
    )

    # 8. STRIPE PROCESSED EVENTS (110 events)
    spe_vals = []
    for i in range(1, 111):
        eid = f"evt_stripe_{i:04d}_{gen_uuid()[:12]}"
        etype = "payment_intent.succeeded" if i % 2 == 0 else "checkout.session.completed"
        c_at = (NOW - datetime.timedelta(days=(110 - i))).strftime("%Y-%m-%d %H:%M:%S+00")
        spe_vals.append(f"('{eid}', '{etype}', '{c_at}')")
    statements.append(
        "INSERT INTO stripe_processed_events (event_id, event_type, processed_at) VALUES\n"
        + ",\n".join(spe_vals)
        + "\nON CONFLICT (event_id) DO NOTHING;"
    )

    statements.append("COMMIT;")
    return statements

def main():
    # 1. Output DynamoDB data JSON & worker script
    ddb_data = build_dynamodb_script()
    with open("/Users/yashrajoria/ShopSwift/E-Commerce-backend/backend/scripts/dynamodb_data.json", "w") as f:
        json.dump(ddb_data, f)
    print("Generated dynamodb_data.json")

    worker_code = build_dynamodb_worker()
    with open("/Users/yashrajoria/ShopSwift/E-Commerce-backend/backend/scripts/seed_dynamodb_worker.py", "w") as f:
        f.write(worker_code)
    print("Generated seed_dynamodb_worker.py")

    # 2. Output Postgres SQL
    stmts = build_postgres_sql()
    full_sql = "\n\n".join(stmts)
    with open("/Users/yashrajoria/ShopSwift/E-Commerce-backend/backend/scripts/seed_supabase_full.sql", "w") as f:
        f.write(full_sql)
    print(f"Generated seed_supabase_full.sql ({len(stmts)} statements, {len(full_sql)} bytes)")

if __name__ == "__main__":
    main()
