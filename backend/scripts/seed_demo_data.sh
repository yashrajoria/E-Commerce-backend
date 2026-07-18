#!/usr/bin/env bash
set -euo pipefail

# Catalog-only seeding script (no users/admin/accounts).
# Seeds categories and products directly into LocalStack DynamoDB.

COMPOSE_FILES=(-f docker-compose.yml -f docker-compose.localstack.yml)
CATEGORIES_TABLE="${DDB_TABLE_CATEGORIES:-Categories}"
PRODUCTS_TABLE="${DDB_TABLE_PRODUCTS:-Products}"
INVENTORY_TABLE="${DDB_TABLE_INVENTORY:-Inventory}"
PRODUCT_CATEGORIES_TABLE="${DDB_TABLE_PRODUCT_CATEGORIES:-ProductCategories}"
S3_BUCKET="${AWS_S3_BUCKET:-shopswift}"
S3_PREFIX="${AWS_S3_PREFIX:-products/}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

ensure_localstack_running() {
  local status
  status="$(docker compose "${COMPOSE_FILES[@]}" ps --status=running --services | tr '\n' ' ')"
  if [[ "$status" != *"localstack"* ]]; then
    echo "LocalStack is not running." >&2
    echo "Start it with:" >&2
    echo "  docker compose -f docker-compose.yml -f docker-compose.localstack.yml up -d localstack localstack-init" >&2
    exit 1
  fi
}

ddb_put() {
  local table="$1"
  local item_json="$2"
  # Redirect stdin so docker compose exec does not consume a caller's pipe
  # (e.g. while-read over scan results).
  docker compose "${COMPOSE_FILES[@]}" exec -T localstack \
    awslocal dynamodb put-item --table-name "$table" --item "$item_json" </dev/null >/dev/null
}

reset_products() {
  echo "Resetting $PRODUCTS_TABLE and demo S3 images..."

  local product_ids
  product_ids="$(docker compose "${COMPOSE_FILES[@]}" exec -T localstack awslocal dynamodb scan --table-name "$PRODUCTS_TABLE" --projection-expression 'id' --output json | jq -r '.Items[]?.id.S')"
  if [[ -n "$product_ids" ]]; then
    while IFS= read -r id; do
      [[ -z "$id" ]] && continue
      docker compose "${COMPOSE_FILES[@]}" exec -T localstack awslocal dynamodb delete-item --table-name "$PRODUCTS_TABLE" --key "{\"id\":{\"S\":\"$id\"}}" >/dev/null
    done <<< "$product_ids"
  fi

  local demo_prefix
  demo_prefix="$(normalize_prefix "$S3_PREFIX")demo/"
  local object_keys
  object_keys="$(docker compose "${COMPOSE_FILES[@]}" exec -T localstack awslocal s3api list-objects-v2 --bucket "$S3_BUCKET" --prefix "$demo_prefix" --output json | jq -r '.Contents[]?.Key')"
  if [[ -n "$object_keys" ]]; then
    while IFS= read -r key; do
      [[ -z "$key" ]] && continue
      docker compose "${COMPOSE_FILES[@]}" exec -T localstack awslocal s3api delete-object --bucket "$S3_BUCKET" --key "$key" >/dev/null
    done <<< "$object_keys"
  fi
}

normalize_prefix() {
  local p="$1"
  p="${p#/}"
  if [[ -n "$p" && "${p: -1}" != "/" ]]; then
    p="$p/"
  fi
  echo "$p"
}

next_product_uuid() {
  local idx="$1"
  local tail
  printf -v tail "%012x" "$idx"
  echo "00000000-0000-4000-8000-${tail}"
}

upload_demo_image_to_s3() {
  local sku="$1"
  local title="$2"
  local variant="$3"
  local safe_sku key prefix svg

  prefix="$(normalize_prefix "$S3_PREFIX")"
  safe_sku="$(echo "$sku" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9-' '-')"
  key="${prefix}demo/${safe_sku}-${variant}.svg"

  svg="<svg xmlns='http://www.w3.org/2000/svg' width='1200' height='1200' viewBox='0 0 1200 1200'>\n<rect width='1200' height='1200' fill='#f2f4f8'/>\n<rect x='80' y='80' width='1040' height='1040' rx='40' fill='#dbe3ee'/>\n<text x='600' y='530' font-family='Arial, sans-serif' font-size='46' text-anchor='middle' fill='#2b3a4a'>${title}</text>\n<text x='600' y='600' font-family='Arial, sans-serif' font-size='32' text-anchor='middle' fill='#4d6073'>${sku}</text>\n</svg>"
  svg="<svg xmlns='http://www.w3.org/2000/svg' width='1200' height='1200' viewBox='0 0 1200 1200'>\n<rect width='1200' height='1200' fill='#f2f4f8'/>\n<rect x='80' y='80' width='1040' height='1040' rx='40' fill='#dbe3ee'/>\n<text x='600' y='520' font-family='Arial, sans-serif' font-size='46' text-anchor='middle' fill='#2b3a4a'>${title}</text>\n<text x='600' y='590' font-family='Arial, sans-serif' font-size='32' text-anchor='middle' fill='#4d6073'>${sku}</text>\n<text x='600' y='660' font-family='Arial, sans-serif' font-size='22' text-anchor='middle' fill='#6d7d8d'>${variant}</text>\n</svg>"

  docker compose "${COMPOSE_FILES[@]}" exec -T localstack sh -lc \
    "cat > /tmp/${safe_sku}.svg && awslocal s3api put-object --bucket '$S3_BUCKET' --key '$key' --content-type 'image/svg+xml' --body /tmp/${safe_sku}.svg >/dev/null && rm -f /tmp/${safe_sku}.svg" <<<"$svg"

  echo "http://localhost:4566/${S3_BUCKET}/${key}"
}

seed_product() {
  local idx="$1"
  local name="$2"
  local price="$3"
  local quantity="$4"
  local description="$5"
  local brand="$6"
  local sku="$7"
  local cat1="$8"
  local cat2="$9"
  local path1="${10}"
  local path2="${11}"
  local featured="${12}"
  local now id image_url_front image_url_back

  id="$(next_product_uuid "$idx")"
  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  image_url_front="$(upload_demo_image_to_s3 "$sku" "$name" "front")"
  image_url_back="$(upload_demo_image_to_s3 "$sku" "$name" "back")"

  ddb_put "$PRODUCTS_TABLE" "{\"id\":{\"S\":\"$id\"},\"name\":{\"S\":\"$name\"},\"price\":{\"N\":\"$price\"},\"quantity\":{\"N\":\"$quantity\"},\"description\":{\"S\":\"$description\"},\"images\":{\"L\":[{\"S\":\"$image_url_front\"},{\"S\":\"$image_url_back\"}]},\"brand\":{\"S\":\"$brand\"},\"sku\":{\"S\":\"$sku\"},\"category_ids\":{\"L\":[{\"S\":\"$cat1\"},{\"S\":\"$cat2\"}]},\"category_path\":{\"L\":[{\"S\":\"$path1\"},{\"S\":\"$path2\"}]},\"is_featured\":{\"S\":\"$featured\"},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"

  # Checkout reserves from Inventory (separate from product.quantity).
  ddb_put "$INVENTORY_TABLE" "{\"id\":{\"S\":\"$id\"},\"available\":{\"N\":\"$quantity\"},\"reserved\":{\"N\":\"0\"},\"threshold\":{\"N\":\"5\"},\"order_reservations\":{\"M\":{}},\"updated_at\":{\"S\":\"$now\"}}"

  # Category→product adjacency for Query (avoids contains() Scan).
  ddb_put "$PRODUCT_CATEGORIES_TABLE" "{\"category_id\":{\"S\":\"$cat1\"},\"product_id\":{\"S\":\"$id\"},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"
  ddb_put "$PRODUCT_CATEGORIES_TABLE" "{\"category_id\":{\"S\":\"$cat2\"},\"product_id\":{\"S\":\"$id\"},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"
}

seed_categories() {
  local now
  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  # Root categories
  local cat_electronics="2ddfa8df-beb9-4e7f-bb43-e8dd8fe9b7c0"
  local cat_home="5e91a564-bd4d-4f7f-a1b8-b4dca117c04a"
  local cat_fashion="6f57e02c-3674-4d41-a996-82f58dc1d9dd"
  local cat_sports="9bc6de24-8dca-4bc7-b16a-2e2e2c2f3a21"
  local cat_beauty="fefcad24-ec1f-4ff0-8a7a-9ab8f6d4f100"

  # Electronics subcategories
  local cat_audio="de9f4142-26f8-49a3-abfa-0680adf4db92"
  local cat_laptops="792d2f0c-d9bd-4f68-ba95-6d3a97df8dc7"
  local cat_accessories="8f50f9f4-d26d-4a20-a5f7-b9f6ed232f9d"

  # Home subcategories
  local cat_kitchen="760315df-f17b-4f7e-8cb8-9322d4cb7fd2"
  local cat_decor="8b9363d3-12a8-4fa3-9d9a-7d7bfecaf1dd"

  # Fashion subcategories
  local cat_men="9f17794b-1534-4e7f-a9ea-6d4cdccea1dd"
  local cat_women="f5644c43-2652-495a-8de5-a9bc9355137f"

  # Sports subcategories
  local cat_running="a9f5704a-e8f4-4df1-8d95-5ca4d70996cc"
  local cat_fitness="df444d53-d6ca-4d89-9a47-9dce2f7eb1a7"

  # Beauty subcategories
  local cat_skincare="c5f247ca-8cb2-4639-b6b0-bda6f1c30f63"

  ddb_put "$CATEGORIES_TABLE" "{\"id\":{\"S\":\"$cat_electronics\"},\"name\":{\"S\":\"electronics\"},\"slug\":{\"S\":\"electronics\"},\"path\":{\"L\":[{\"S\":\"electronics\"}]},\"level\":{\"N\":\"0\"},\"parent_ids\":{\"L\":[]},\"ancestors\":{\"L\":[]},\"is_active\":{\"BOOL\":true},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"
  ddb_put "$CATEGORIES_TABLE" "{\"id\":{\"S\":\"$cat_home\"},\"name\":{\"S\":\"home\"},\"slug\":{\"S\":\"home\"},\"path\":{\"L\":[{\"S\":\"home\"}]},\"level\":{\"N\":\"0\"},\"parent_ids\":{\"L\":[]},\"ancestors\":{\"L\":[]},\"is_active\":{\"BOOL\":true},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"
  ddb_put "$CATEGORIES_TABLE" "{\"id\":{\"S\":\"$cat_fashion\"},\"name\":{\"S\":\"fashion\"},\"slug\":{\"S\":\"fashion\"},\"path\":{\"L\":[{\"S\":\"fashion\"}]},\"level\":{\"N\":\"0\"},\"parent_ids\":{\"L\":[]},\"ancestors\":{\"L\":[]},\"is_active\":{\"BOOL\":true},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"
  ddb_put "$CATEGORIES_TABLE" "{\"id\":{\"S\":\"$cat_sports\"},\"name\":{\"S\":\"sports\"},\"slug\":{\"S\":\"sports\"},\"path\":{\"L\":[{\"S\":\"sports\"}]},\"level\":{\"N\":\"0\"},\"parent_ids\":{\"L\":[]},\"ancestors\":{\"L\":[]},\"is_active\":{\"BOOL\":true},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"
  ddb_put "$CATEGORIES_TABLE" "{\"id\":{\"S\":\"$cat_beauty\"},\"name\":{\"S\":\"beauty\"},\"slug\":{\"S\":\"beauty\"},\"path\":{\"L\":[{\"S\":\"beauty\"}]},\"level\":{\"N\":\"0\"},\"parent_ids\":{\"L\":[]},\"ancestors\":{\"L\":[]},\"is_active\":{\"BOOL\":true},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"

  ddb_put "$CATEGORIES_TABLE" "{\"id\":{\"S\":\"$cat_audio\"},\"name\":{\"S\":\"audio\"},\"slug\":{\"S\":\"audio\"},\"path\":{\"L\":[{\"S\":\"electronics\"},{\"S\":\"audio\"}]},\"level\":{\"N\":\"1\"},\"parent_ids\":{\"L\":[{\"S\":\"$cat_electronics\"}]},\"ancestors\":{\"L\":[{\"S\":\"$cat_electronics\"}]},\"is_active\":{\"BOOL\":true},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"
  ddb_put "$CATEGORIES_TABLE" "{\"id\":{\"S\":\"$cat_laptops\"},\"name\":{\"S\":\"laptops\"},\"slug\":{\"S\":\"laptops\"},\"path\":{\"L\":[{\"S\":\"electronics\"},{\"S\":\"laptops\"}]},\"level\":{\"N\":\"1\"},\"parent_ids\":{\"L\":[{\"S\":\"$cat_electronics\"}]},\"ancestors\":{\"L\":[{\"S\":\"$cat_electronics\"}]},\"is_active\":{\"BOOL\":true},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"
  ddb_put "$CATEGORIES_TABLE" "{\"id\":{\"S\":\"$cat_accessories\"},\"name\":{\"S\":\"accessories\"},\"slug\":{\"S\":\"accessories\"},\"path\":{\"L\":[{\"S\":\"electronics\"},{\"S\":\"accessories\"}]},\"level\":{\"N\":\"1\"},\"parent_ids\":{\"L\":[{\"S\":\"$cat_electronics\"}]},\"ancestors\":{\"L\":[{\"S\":\"$cat_electronics\"}]},\"is_active\":{\"BOOL\":true},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"

  ddb_put "$CATEGORIES_TABLE" "{\"id\":{\"S\":\"$cat_kitchen\"},\"name\":{\"S\":\"kitchen\"},\"slug\":{\"S\":\"kitchen\"},\"path\":{\"L\":[{\"S\":\"home\"},{\"S\":\"kitchen\"}]},\"level\":{\"N\":\"1\"},\"parent_ids\":{\"L\":[{\"S\":\"$cat_home\"}]},\"ancestors\":{\"L\":[{\"S\":\"$cat_home\"}]},\"is_active\":{\"BOOL\":true},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"
  ddb_put "$CATEGORIES_TABLE" "{\"id\":{\"S\":\"$cat_decor\"},\"name\":{\"S\":\"decor\"},\"slug\":{\"S\":\"decor\"},\"path\":{\"L\":[{\"S\":\"home\"},{\"S\":\"decor\"}]},\"level\":{\"N\":\"1\"},\"parent_ids\":{\"L\":[{\"S\":\"$cat_home\"}]},\"ancestors\":{\"L\":[{\"S\":\"$cat_home\"}]},\"is_active\":{\"BOOL\":true},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"

  ddb_put "$CATEGORIES_TABLE" "{\"id\":{\"S\":\"$cat_men\"},\"name\":{\"S\":\"men\"},\"slug\":{\"S\":\"men\"},\"path\":{\"L\":[{\"S\":\"fashion\"},{\"S\":\"men\"}]},\"level\":{\"N\":\"1\"},\"parent_ids\":{\"L\":[{\"S\":\"$cat_fashion\"}]},\"ancestors\":{\"L\":[{\"S\":\"$cat_fashion\"}]},\"is_active\":{\"BOOL\":true},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"
  ddb_put "$CATEGORIES_TABLE" "{\"id\":{\"S\":\"$cat_women\"},\"name\":{\"S\":\"women\"},\"slug\":{\"S\":\"women\"},\"path\":{\"L\":[{\"S\":\"fashion\"},{\"S\":\"women\"}]},\"level\":{\"N\":\"1\"},\"parent_ids\":{\"L\":[{\"S\":\"$cat_fashion\"}]},\"ancestors\":{\"L\":[{\"S\":\"$cat_fashion\"}]},\"is_active\":{\"BOOL\":true},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"

  ddb_put "$CATEGORIES_TABLE" "{\"id\":{\"S\":\"$cat_running\"},\"name\":{\"S\":\"running\"},\"slug\":{\"S\":\"running\"},\"path\":{\"L\":[{\"S\":\"sports\"},{\"S\":\"running\"}]},\"level\":{\"N\":\"1\"},\"parent_ids\":{\"L\":[{\"S\":\"$cat_sports\"}]},\"ancestors\":{\"L\":[{\"S\":\"$cat_sports\"}]},\"is_active\":{\"BOOL\":true},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"
  ddb_put "$CATEGORIES_TABLE" "{\"id\":{\"S\":\"$cat_fitness\"},\"name\":{\"S\":\"fitness\"},\"slug\":{\"S\":\"fitness\"},\"path\":{\"L\":[{\"S\":\"sports\"},{\"S\":\"fitness\"}]},\"level\":{\"N\":\"1\"},\"parent_ids\":{\"L\":[{\"S\":\"$cat_sports\"}]},\"ancestors\":{\"L\":[{\"S\":\"$cat_sports\"}]},\"is_active\":{\"BOOL\":true},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"

  ddb_put "$CATEGORIES_TABLE" "{\"id\":{\"S\":\"$cat_skincare\"},\"name\":{\"S\":\"skincare\"},\"slug\":{\"S\":\"skincare\"},\"path\":{\"L\":[{\"S\":\"beauty\"},{\"S\":\"skincare\"}]},\"level\":{\"N\":\"1\"},\"parent_ids\":{\"L\":[{\"S\":\"$cat_beauty\"}]},\"ancestors\":{\"L\":[{\"S\":\"$cat_beauty\"}]},\"is_active\":{\"BOOL\":true},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"

  echo "Categories seeded into $CATEGORIES_TABLE"
}

seed_products() {
  local i
  i=1

  local cat_electronics="2ddfa8df-beb9-4e7f-bb43-e8dd8fe9b7c0"
  local cat_audio="de9f4142-26f8-49a3-abfa-0680adf4db92"
  local cat_laptops="792d2f0c-d9bd-4f68-ba95-6d3a97df8dc7"
  local cat_accessories="8f50f9f4-d26d-4a20-a5f7-b9f6ed232f9d"
  local cat_home="5e91a564-bd4d-4f7f-a1b8-b4dca117c04a"
  local cat_kitchen="760315df-f17b-4f7e-8cb8-9322d4cb7fd2"
  local cat_decor="8b9363d3-12a8-4fa3-9d9a-7d7bfecaf1dd"
  local cat_fashion="6f57e02c-3674-4d41-a996-82f58dc1d9dd"
  local cat_men="9f17794b-1534-4e7f-a9ea-6d4cdccea1dd"
  local cat_women="f5644c43-2652-495a-8de5-a9bc9355137f"
  local cat_sports="9bc6de24-8dca-4bc7-b16a-2e2e2c2f3a21"
  local cat_running="a9f5704a-e8f4-4df1-8d95-5ca4d70996cc"
  local cat_fitness="df444d53-d6ca-4d89-9a47-9dce2f7eb1a7"
  local cat_beauty="fefcad24-ec1f-4ff0-8a7a-9ab8f6d4f100"
  local cat_skincare="c5f247ca-8cb2-4639-b6b0-bda6f1c30f63"

  # Core curated set
  seed_product "$i" "Nova Wireless Mouse" "39.99" "120" "Ergonomic wireless mouse with silent clicks and USB-C receiver" "NovaTech" "NT-MSE-001" "$cat_electronics" "$cat_accessories" "electronics" "accessories" "true"; i=$((i+1))
  seed_product "$i" "Apex Mechanical Keyboard" "89.99" "80" "Hot-swappable 87-key keyboard with tactile switches" "ApexGear" "AG-KBD-087" "$cat_electronics" "$cat_accessories" "electronics" "accessories" "true"; i=$((i+1))
  seed_product "$i" "Pulse Noise-Cancel Headphones" "149.99" "65" "Over-ear ANC headphones with 40-hour battery life" "Pulse" "PLS-HDP-040" "$cat_electronics" "$cat_audio" "electronics" "audio" "true"; i=$((i+1))
  seed_product "$i" "Echo Bluetooth Speaker" "59.99" "95" "Portable splash-resistant speaker with deep bass" "EchoOne" "EC-SPK-010" "$cat_electronics" "$cat_audio" "electronics" "audio" "false"; i=$((i+1))
  seed_product "$i" "Orbit 14 Ultrabook" "999.00" "24" "14-inch ultrabook, 16GB RAM, 512GB SSD" "Orbit" "ORB-LTP-014" "$cat_electronics" "$cat_laptops" "electronics" "laptops" "true"; i=$((i+1))
  seed_product "$i" "Aura Desk Lamp" "29.99" "60" "Warm LED desk lamp with touch dimmer" "AuraHome" "AH-LMP-100" "$cat_home" "$cat_decor" "home" "decor" "false"; i=$((i+1))
  seed_product "$i" "ChefPro Nonstick Pan Set" "79.99" "45" "3-piece nonstick pan set for everyday cooking" "ChefPro" "CP-PAN-3PC" "$cat_home" "$cat_kitchen" "home" "kitchen" "true"; i=$((i+1))
  seed_product "$i" "BrewMate French Press" "34.50" "70" "1L borosilicate french press for rich coffee" "BrewMate" "BM-FRP-1L" "$cat_home" "$cat_kitchen" "home" "kitchen" "false"; i=$((i+1))
  seed_product "$i" "StrideFlex Running Shoes" "119.00" "58" "Lightweight running shoes with responsive foam sole" "StrideFlex" "SF-RUN-001" "$cat_sports" "$cat_running" "sports" "running" "true"; i=$((i+1))
  seed_product "$i" "CoreBand Resistance Set" "24.99" "140" "5-level resistance bands for home workouts" "CoreBand" "CB-RST-005" "$cat_sports" "$cat_fitness" "sports" "fitness" "false"; i=$((i+1))
  seed_product "$i" "Urban Cotton Tee" "19.99" "180" "Soft-fit everyday t-shirt, breathable cotton" "UrbanThread" "UT-TEE-001" "$cat_fashion" "$cat_men" "fashion" "men" "false"; i=$((i+1))
  seed_product "$i" "Luna Midi Dress" "59.99" "75" "Flowy midi dress for casual and evening wear" "LunaWear" "LW-DRS-210" "$cat_fashion" "$cat_women" "fashion" "women" "true"; i=$((i+1))
  seed_product "$i" "Hydra Glow Serum" "27.99" "110" "Niacinamide and hyaluronic acid daily glow serum" "HydraLab" "HL-SRM-030" "$cat_beauty" "$cat_skincare" "beauty" "skincare" "true"; i=$((i+1))
  seed_product "$i" "Velvet Night Cream" "33.49" "92" "Rich peptide night cream for hydration and repair" "VelvetSkin" "VS-CRM-050" "$cat_beauty" "$cat_skincare" "beauty" "skincare" "false"; i=$((i+1))

  # Expanded demo catalog (adds breadth across all subcategories)
  seed_product "$i" "Sonic Wired Earbuds" "29.00" "220" "In-ear earbuds with inline mic" "Sonic" "SON-EBD-001" "$cat_electronics" "$cat_audio" "electronics" "audio" "false"; i=$((i+1))
  seed_product "$i" "Wave Studio Mic" "129.00" "35" "USB condenser mic for streaming and calls" "WaveLab" "WAV-MIC-220" "$cat_electronics" "$cat_audio" "electronics" "audio" "false"; i=$((i+1))
  seed_product "$i" "Nimbus Gaming Laptop" "1499.00" "18" "RTX-class gaming laptop with 165Hz display" "Nimbus" "NMB-LTP-165" "$cat_electronics" "$cat_laptops" "electronics" "laptops" "true"; i=$((i+1))
  seed_product "$i" "ZenBook Air 13" "849.00" "26" "Thin and light productivity laptop" "ZenBook" "ZEN-AIR-013" "$cat_electronics" "$cat_laptops" "electronics" "laptops" "false"; i=$((i+1))
  seed_product "$i" "ChargeMax 65W Adapter" "39.00" "150" "GaN fast charger with dual USB-C" "ChargeMax" "CM-CHG-65W" "$cat_electronics" "$cat_accessories" "electronics" "accessories" "false"; i=$((i+1))
  seed_product "$i" "Flex USB-C Hub" "49.00" "130" "6-in-1 USB-C docking hub" "FlexPort" "FX-HUB-006" "$cat_electronics" "$cat_accessories" "electronics" "accessories" "false"; i=$((i+1))

  seed_product "$i" "Oak Cutting Board" "25.00" "84" "Solid oak board with juice groove" "OakCraft" "OC-BRD-001" "$cat_home" "$cat_kitchen" "home" "kitchen" "false"; i=$((i+1))
  seed_product "$i" "SteamPro Kettle" "44.00" "55" "Electric kettle with temperature presets" "SteamPro" "SP-KTL-170" "$cat_home" "$cat_kitchen" "home" "kitchen" "false"; i=$((i+1))
  seed_product "$i" "Nordic Wall Clock" "39.00" "67" "Silent sweep wall clock in oak finish" "Nordic" "NRD-CLK-120" "$cat_home" "$cat_decor" "home" "decor" "false"; i=$((i+1))
  seed_product "$i" "Cloud Throw Pillow" "22.00" "140" "Soft textured pillow for sofa and bed" "CloudNest" "CLD-PLW-018" "$cat_home" "$cat_decor" "home" "decor" "false"; i=$((i+1))

  seed_product "$i" "Summit Trail Shoes" "109.00" "42" "Durable trail runners for mixed terrain" "Summit" "SUM-TRL-009" "$cat_sports" "$cat_running" "sports" "running" "true"; i=$((i+1))
  seed_product "$i" "Pace Running Cap" "18.00" "175" "Breathable lightweight cap for runs" "Pace" "PAC-CAP-002" "$cat_sports" "$cat_running" "sports" "running" "false"; i=$((i+1))
  seed_product "$i" "IronCore Dumbbell Set" "139.00" "33" "Adjustable dumbbell set for strength training" "IronCore" "IRC-DMB-40" "$cat_sports" "$cat_fitness" "sports" "fitness" "true"; i=$((i+1))
  seed_product "$i" "Grip Yoga Mat" "35.00" "90" "Non-slip 6mm yoga mat with carry strap" "GripFit" "GRP-MAT-006" "$cat_sports" "$cat_fitness" "sports" "fitness" "false"; i=$((i+1))

  seed_product "$i" "Classic Oxford Shirt" "45.00" "100" "Slim-fit cotton oxford shirt" "TailorLine" "TL-SRT-101" "$cat_fashion" "$cat_men" "fashion" "men" "false"; i=$((i+1))
  seed_product "$i" "Denim Rider Jacket" "89.00" "48" "Mid-wash denim jacket with stretch" "BlueRider" "BR-JKT-501" "$cat_fashion" "$cat_men" "fashion" "men" "true"; i=$((i+1))
  seed_product "$i" "Eve Silk Blouse" "64.00" "61" "Soft drape blouse with clean collar" "EveLine" "EVE-BLS-310" "$cat_fashion" "$cat_women" "fashion" "women" "false"; i=$((i+1))
  seed_product "$i" "Willow Knit Cardigan" "72.00" "57" "Cozy knit cardigan with front pockets" "Willow" "WIL-CDG-211" "$cat_fashion" "$cat_women" "fashion" "women" "true"; i=$((i+1))

  seed_product "$i" "PureFoam Cleanser" "18.50" "210" "Gentle daily foaming face cleanser" "PureFoam" "PF-CLN-150" "$cat_beauty" "$cat_skincare" "beauty" "skincare" "false"; i=$((i+1))
  seed_product "$i" "SunShield SPF 50" "22.00" "160" "Broad-spectrum sunscreen with matte finish" "SunShield" "SS-SPF-050" "$cat_beauty" "$cat_skincare" "beauty" "skincare" "true"; i=$((i+1))
  seed_product "$i" "Calm Mist Toner" "16.00" "125" "Hydrating toner with chamomile extract" "CalmLab" "CLM-TNR-120" "$cat_beauty" "$cat_skincare" "beauty" "skincare" "false"; i=$((i+1))
  seed_product "$i" "Retinol Repair Gel" "31.00" "88" "Night repair gel with encapsulated retinol" "DermEase" "DER-RET-030" "$cat_beauty" "$cat_skincare" "beauty" "skincare" "true"; i=$((i+1))

  # Higher-end and budget-friendly additions.
  seed_product "$i" "Vertex Pro Tablet 11" "699.00" "32" "11-inch productivity tablet with pen support" "Vertex" "VTX-TAB-011" "$cat_electronics" "$cat_accessories" "electronics" "accessories" "true"; i=$((i+1))
  seed_product "$i" "Astra Studio Monitor" "1299.00" "12" "32-inch 4K HDR monitor for creators" "AstraDisplay" "ASD-MON-032" "$cat_electronics" "$cat_laptops" "electronics" "laptops" "true"; i=$((i+1))
  seed_product "$i" "Budget Basic Mouse" "14.99" "300" "Simple wired mouse for everyday office use" "OfficeLite" "OL-MSE-014" "$cat_electronics" "$cat_accessories" "electronics" "accessories" "false"; i=$((i+1))
  seed_product "$i" "Everyday Cotton Socks 5-Pack" "12.00" "240" "Breathable cotton socks in a value pack" "Everyday Basics" "EB-SCK-005" "$cat_fashion" "$cat_men" "fashion" "men" "false"; i=$((i+1))
  seed_product "$i" "Mini Pantry Organizer Set" "18.00" "190" "Stackable pantry organizers for dry goods" "HomeLoop" "HL-ORG-003" "$cat_home" "$cat_kitchen" "home" "kitchen" "false"; i=$((i+1))
  seed_product "$i" "Starter Skincare Duo" "24.00" "170" "Cleanser and moisturizer bundle for daily use" "GlowRoute" "GR-DUO-024" "$cat_beauty" "$cat_skincare" "beauty" "skincare" "false"; i=$((i+1))

  echo "Products seeded into $PRODUCTS_TABLE"
}

print_summary() {
  local category_count product_count inventory_count link_count object_count
  category_count="$(docker compose "${COMPOSE_FILES[@]}" exec -T localstack awslocal dynamodb scan --table-name "$CATEGORIES_TABLE" --select COUNT --query 'Count' --output text)"
  product_count="$(docker compose "${COMPOSE_FILES[@]}" exec -T localstack awslocal dynamodb scan --table-name "$PRODUCTS_TABLE" --select COUNT --query 'Count' --output text)"
  inventory_count="$(docker compose "${COMPOSE_FILES[@]}" exec -T localstack awslocal dynamodb scan --table-name "$INVENTORY_TABLE" --select COUNT --query 'Count' --output text)"
  link_count="$(docker compose "${COMPOSE_FILES[@]}" exec -T localstack awslocal dynamodb scan --table-name "$PRODUCT_CATEGORIES_TABLE" --select COUNT --query 'Count' --output text 2>/dev/null || echo 0)"
  object_count="$(docker compose "${COMPOSE_FILES[@]}" exec -T localstack awslocal s3api list-objects-v2 --bucket "$S3_BUCKET" --prefix "$(normalize_prefix "$S3_PREFIX")demo/" --query 'length(Contents)' --output text 2>/dev/null || echo 0)"
  echo "Seed complete. Categories: $category_count, Products: $product_count, Inventory: $inventory_count, ProductCategories: $link_count, S3 demo images: $object_count"
}

# Sync Inventory rows from existing Products (quantity → available).
# Use when Products were seeded without Inventory (checkout would 409).
sync_inventory_from_products() {
  echo "Syncing $INVENTORY_TABLE from $PRODUCTS_TABLE quantities..."
  local now
  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  local items
  items="$(docker compose "${COMPOSE_FILES[@]}" exec -T localstack awslocal dynamodb scan \
    --table-name "$PRODUCTS_TABLE" \
    --projection-expression 'id,#q' \
    --expression-attribute-names '{"#q":"quantity"}' \
    --output json </dev/null)"

  local count=0
  local id qty
  while IFS=$'\t' read -r id qty; do
    [[ -z "$id" ]] && continue
    qty="${qty:-0}"
    # order_reservations must exist so ReserveAll can SET map paths.
    ddb_put "$INVENTORY_TABLE" "{\"id\":{\"S\":\"$id\"},\"available\":{\"N\":\"$qty\"},\"reserved\":{\"N\":\"0\"},\"threshold\":{\"N\":\"5\"},\"order_reservations\":{\"M\":{}},\"updated_at\":{\"S\":\"$now\"}}"
    count=$((count + 1))
  done < <(echo "$items" | jq -r '.Items[]? | [(.id.S // empty), (.quantity.N // "0")] | @tsv')

  echo "Inventory sync complete ($count items)."
}

# Sync ProductCategories adjacency from product.category_ids.
sync_product_categories_from_products() {
  echo "Syncing $PRODUCT_CATEGORIES_TABLE from $PRODUCTS_TABLE category_ids..."
  local now
  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  local items
  items="$(docker compose "${COMPOSE_FILES[@]}" exec -T localstack awslocal dynamodb scan \
    --table-name "$PRODUCTS_TABLE" \
    --projection-expression 'id,category_ids' \
    --output json </dev/null)"

  local count=0
  echo "$items" | jq -c '.Items[]?' | while read -r item; do
    local id
    id="$(echo "$item" | jq -r '.id.S // empty')"
    [[ -z "$id" ]] && continue
    echo "$item" | jq -r '.category_ids.L[]?.S // empty' | while read -r cat; do
      [[ -z "$cat" ]] && continue
      ddb_put "$PRODUCT_CATEGORIES_TABLE" "{\"category_id\":{\"S\":\"$cat\"},\"product_id\":{\"S\":\"$id\"},\"created_at\":{\"S\":\"$now\"},\"updated_at\":{\"S\":\"$now\"}}"
    done
    count=$((count + 1))
  done
  # recount (subshell count is lost)
  link_count="$(docker compose "${COMPOSE_FILES[@]}" exec -T localstack awslocal dynamodb scan --table-name "$PRODUCT_CATEGORIES_TABLE" --select COUNT --query 'Count' --output text </dev/null)"
  echo "ProductCategories sync complete ($link_count links)."
}

main() {
  require_cmd docker
  require_cmd jq
  ensure_localstack_running

  if [[ "${1:-}" == "--sync-inventory-only" ]]; then
    sync_inventory_from_products
    print_summary
    return 0
  fi

  if [[ "${1:-}" == "--sync-category-links-only" ]]; then
    sync_product_categories_from_products
    print_summary
    return 0
  fi

  reset_products
  seed_categories
  seed_products
  print_summary
}

main "$@"
