import boto3
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
