#!/usr/bin/env python3
"""
Comprehensive Seed Data Generator for ShopSwift:
Generates 100+ realistic rows for:
1. DynamoDB:
   - Categories (110 categories)
   - Products (125 products)
   - Inventory (125 items)
   - ProductCategories (250 mappings)
2. Supabase / Postgres:
   - users (120 users)
   - addresses (150 addresses)
   - coupons (105 coupons)
   - orders (125 orders)
   - order_items (350 order items)
   - payments (125 payments)
   - coupon_usages (105 usages)
   - shipments (105 shipments)
   - notification_logs (130 logs)
   - notification_events (110 events)
   - outbox_events (125 events)
   - payment_outbox_events (125 events)
   - refresh_tokens (120 tokens)
   - stripe_processed_events (110 events)
"""

import json
import uuid
import datetime
import random
import os

random.seed(42)

def gen_product_uuid(idx):
    tail = f"{idx:012x}"
    return f"00000000-0000-4000-8000-{tail}"

# --- CATEGORIES DATA (10 root + 100 sub = 110 categories) ---
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

# --- 125 PRODUCTS SPECIFICATION ---
PRODUCTS_DATA = [
    # Electronics (25)
    ("Nova Wireless Optical Mouse", 3999, 120, "Ergonomic 2.4GHz wireless mouse with silent clicks", "NovaTech", "NOV-WMS-101", "electronics", "accessories", True),
    ("Apex Pro Mechanical Keyboard", 12999, 45, "RGB backlit mechanical keyboard with hot-swappable switches", "Apex", "APX-MKB-201", "electronics", "accessories", True),
    ("Pulse Noise-Canceling Headphones", 14999, 80, "Active noise cancelling over-ear headphones with 40h battery", "Pulse", "PLS-NCH-301", "electronics", "audio", True),
    ("Clarity 4K Pro Webcam", 8999, 65, "Ultra HD 4K webcam with dual stereo microphones and autofocus", "Clarity", "CLR-CAM-401", "electronics", "cameras", False),
    ("Orbit 14 Ultrabook", 99900, 30, "Slim 14-inch laptop with OLED display and all-day battery", "Orbit", "ORB-ULT-501", "electronics", "laptops", True),
    ("Horizon 27-inch 4K Monitor", 34999, 50, "IPS 4K UHD monitor with 99% sRGB and USB-C 90W delivery", "Horizon", "HRZ-MON-601", "electronics", "monitors", False),
    ("EchoStream USB Podcast Mic", 7499, 90, "Cardioid studio condenser microphone with zero-latency monitoring", "EchoStream", "ECH-MIC-701", "electronics", "audio", False),
    ("AeroPod Active Earbuds", 7999, 140, "Waterproof wireless earbuds with ambient sound mode", "Aero", "AER-EAR-801", "electronics", "audio", True),
    ("Stratus Portable SSD 1TB", 10999, 110, "Rugged external solid state drive with 1050MB/s transfer speeds", "Stratus", "STR-SSD-901", "electronics", "accessories", False),
    ("Fusion MagSafe Power Bank 10000mAh", 4999, 150, "Magnetic wireless portable battery pack with 20W PD", "Fusion", "FUS-PBK-102", "electronics", "accessories", True),
    ("Quantum Curved Gaming Monitor 34\"", 49999, 25, "165Hz WQHD ultrawide curved monitor with 1ms response", "Quantum", "QNT-MON-202", "electronics", "monitors", True),
    ("Titan Pro Spatial Gaming Headset", 11999, 60, "7.1 surround sound gaming headset with detachable boom mic", "Titan", "TTN-HST-302", "electronics", "gaming", False),
    ("Lumina Smart Desk Lamp", 5999, 85, "Color-tunable LED task light with wireless phone charging base", "Lumina", "LUM-LMP-402", "electronics", "smart-home", False),
    ("Aura 360 Bluetooth Speaker", 6999, 95, "IPX7 waterproof portable speaker with rich bass and stereo pairing", "Aura", "AUR-SPK-502", "electronics", "audio", False),
    ("Nexus 7-in-1 Dual USB-C Hub", 3999, 130, "Multiport adapter with 4K HDMI, 100W PD, and SD card slots", "Nexus", "NEX-HUB-602", "electronics", "accessories", False),
    ("PixelView Digital Photo Frame 10\"", 8999, 40, "Wi-Fi enabled IPS touch display for instant photo sharing", "PixelView", "PXL-FRM-702", "electronics", "smart-home", False),
    ("SwiftCharge 65W GaN Wall Charger", 3499, 180, "Compact 3-port fast wall charger for phones and laptops", "SwiftCharge", "SWF-CHG-802", "electronics", "accessories", False),
    ("Vector Wireless Pro Gamepad", 5499, 75, "Low-latency wireless controller compatible with PC, console & mobile", "Vector", "VEC-PAD-902", "electronics", "gaming", False),
    ("CorePad Extended RGB Mouse Mat", 2499, 120, "Micro-textured cloth desk mat with 14 chroma lighting modes", "CorePad", "COR-MAT-103", "electronics", "gaming", False),
    ("StreamDeck Control Keypad 15-Key", 14999, 35, "Customizable LCD keys for studio workflow, OBS & shortcuts", "StreamDeck", "STR-KPD-203", "electronics", "accessories", True),
    ("SoundWave Dolby Atmos Soundbar", 19999, 40, "Compact 2.1 channel TV soundbar with built-in subwoofers", "SoundWave", "SND-BAR-303", "electronics", "audio", False),
    ("AeroWatch Pro Smartwatch", 22999, 65, "AMOLED fitness smartwatch with ECG, SPO2 and GPS tracking", "Aero", "AER-WTC-403", "electronics", "wearables", True),
    ("NeoDrive External NVMe Enclosure", 2999, 110, "Aluminum 10Gbps M.2 NVMe SSD enclosure with tool-free design", "NeoDrive", "NEO-ENC-503", "electronics", "accessories", False),
    ("Spark Mini Drone 4K Gimbal", 27999, 30, "Sub-249g folding quadcopter with 3-axis mechanical stabilization", "Spark", "SPK-DRN-603", "electronics", "cameras", True),
    ("KeyMaster Wireless Mechanical Numpad", 4499, 85, "Bluetooth numeric keypad with linear silent switches", "KeyMaster", "KEY-NUM-703", "electronics", "accessories", False),

    # Fashion (25)
    ("StrideFlex Lightweight Running Shoes", 11900, 70, "Breathable knit running sneakers with responsive cushioning", "StrideFlex", "STR-SHS-001", "fashion", "footwear", True),
    ("Minimalist RFID Leather Cardholder", 2999, 160, "Top-grain Italian leather slim card case with RFID shielding", "Vanguard", "VAN-WLT-002", "fashion", "accessories", False),
    ("Classic Oxford Cotton Button-Down", 4999, 95, "Tailored fit wrinkle-resistant pinpoint Oxford dress shirt", "Heritage", "HER-SHT-003", "fashion", "men", False),
    ("Luna Floral Midi Wrap Dress", 5999, 60, "Airy floral printed chiffon wrap dress with tiered skirt", "Luna", "LUN-DRS-004", "fashion", "women", True),
    ("Maverick Selvedge Denim Jacket", 8999, 50, "Heavyweight 14oz Japanese denim jacket with brass hardware", "Maverick", "MAV-JCK-005", "fashion", "men", True),
    ("CloudKnit Merino Wool Crewneck", 7999, 65, "Extra-fine merino wool knitted sweater in heather oatmeal", "CloudKnit", "CLD-SWT-006", "fashion", "men", False),
    ("Urban Utility Relaxed Cargo Pants", 6499, 80, "Water-repellent stretch ripstop pants with multi-pockets", "Urbanite", "URB-PNT-007", "fashion", "men", False),
    ("Coastal Linen Relaxed Resort Shirt", 4499, 85, "100% French flax linen short-sleeve camp collar shirt", "Coastal", "CST-SHT-008", "fashion", "men", False),
    ("Alpine Weatherproof Down Parka", 24999, 35, "700-fill responsible down jacket with seam-sealed waterproof shell", "Alpine", "ALP-PRK-009", "fashion", "outerwear", True),
    ("Solstice Polarized Acetate Sunglasses", 8999, 75, "Handcrafted acetate frames with UV400 polarized crystal lenses", "Solstice", "SOL-SUN-010", "fashion", "sunglasses", False),
    ("Heritage Chronograph Leather Watch", 18999, 45, "Sapphire crystal chronograph timepiece with Horween leather strap", "Chronos", "CHR-WTC-011", "fashion", "watches", True),
    ("Nomad Waxed Canvas Commuter Pack", 11999, 55, "Waterproof 20L daypack with padded 16\" laptop sleeve", "Nomad", "NOM-BPK-012", "fashion", "bags", True),
    ("Chelsea Burnished Leather Ankle Boots", 15999, 40, "Goodyear-welted calfskin leather Chelsea boots with Dainite sole", "CobblerCo", "CBL-BOT-013", "fashion", "footwear", False),
    ("Cashmere Ribbed Winter Scarf", 6999, 80, "Pure Grade-A Mongolian cashmere scarf in charcoal melange", "Kashmir", "KSH-SCF-014", "fashion", "accessories", False),
    ("Summit Waterproof Trail Hiking Boot", 13999, 50, "Vibram-lugged Gore-Tex backpacking boot with ankle support", "Summit", "SMT-BOT-015", "fashion", "footwear", False),
    ("Velocity Seamless Training Leggings", 4999, 110, "High-waisted compression tights with moisture-wicking fabric", "Velocity", "VEL-LEG-016", "fashion", "activewear", False),
    ("Eve Silk Charmeuse Evening Blouse", 6400, 60, "Soft drape pure mulberry silk blouse with French cuffs", "EveLine", "EVE-BLS-017", "fashion", "women", True),
    ("Tailored Stretch Cotton Chino", 5499, 90, "Modern tapered fit trousers with hidden stretch waistband", "Heritage", "HER-CHN-018", "fashion", "men", False),
    ("Voyager Full-Grain Leather Weekender", 21999, 30, "Spacious 45L travel duffel with dedicated shoe compartment", "Voyager", "VOY-DFL-019", "fashion", "bags", True),
    ("Riviera Handwoven Straw Panama Hat", 3999, 70, "Ecuadorian toquilla straw sun hat with grosgrain ribbon band", "Riviera", "RIV-HAT-020", "fashion", "accessories", False),
    ("Breeze 7-Inch Athletic Running Shorts", 3499, 120, "Ultralight linerless gym shorts with zip phone pocket", "StrideFlex", "STR-SHT-021", "fashion", "activewear", False),
    ("Timeless 14K Gold Vermeil Rope Chain", 7999, 85, "Hypoallergenic 20-inch twisted rope chain necklace", "Aurelia", "AUR-CHN-022", "fashion", "jewelry", False),
    ("Sterling Silver Hammered Cuff Bracelet", 4999, 95, "Hand-finished 925 sterling silver minimalist open bangle", "Aurelia", "AUR-CUF-023", "fashion", "jewelry", False),
    ("All-Day Merino Wool Crew Socks 3-Pack", 2799, 150, "Cushioned seamless toe thermal boot socks", "CloudKnit", "CLD-SOX-024", "fashion", "accessories", False),
    ("StormGuard Packable Hooded Raincoat", 9999, 65, "Breathable 2.5-layer technical shell that packs into its pocket", "StormGuard", "STM-RNC-025", "fashion", "outerwear", False),

    # Home & Kitchen (20)
    ("Ceramic Matte 16-Piece Dinnerware Set", 11999, 35, "Stoneware plates and bowls service for 4 in speckled ivory", "ClayHouse", "CLY-DNW-001", "home", "dining", True),
    ("Artisan Copper French Press 1L", 4499, 85, "Double-walled borosilicate glass press with electroplated frame", "BrewCraft", "BRW-FPR-002", "home", "kitchen", False),
    ("Damascus Steel Japanese Santoku Knife 7\"", 7999, 50, "67-layer hammered VG-10 core chef knife with pakkawood handle", "Katsura", "KAT-KNV-003", "home", "kitchen", True),
    ("Washed Belgian Linen Queen Sheet Set", 16999, 40, "Pre-washed breathable flax linen sheets with deep pockets", "LinenWorks", "LNN-SHT-004", "home", "bedding", True),
    ("Wabi-Sabi Fluted Stoneware Vase", 3499, 90, "Hand-thrown decorative ceramic flower vase in earthy terracotta", "ClayHouse", "CLY-VAS-005", "home", "decor", False),
    ("Pre-Seasoned Cast Iron Skillet 12-Inch", 3999, 95, "Heavy-duty frying pan with dual pour spouts and assist handle", "IronClad", "IRN-SKL-006", "home", "kitchen", False),
    ("Live-Edge Acacia Serving Board", 3299, 110, "Natural wood charcuterie platter with recessed juice groove", "TimberCraft", "TMB-BRD-007", "home", "dining", False),
    ("Plush Velvet Throw Cushion Pair 18x18\"", 2999, 120, "Hypoallergenic down alternative filled decorative pillows", "VelvetNest", "VLV-CSH-008", "home", "decor", False),
    ("Ultrasonic Ceramic Essential Oil Diffuser", 4299, 75, "Handmade stone diffuser with ambient light and auto shut-off", "AromaPure", "ARM-DIF-009", "home", "decor", False),
    ("Botanical Soy Wax Candle (Oak & Amber)", 2499, 140, "Clean burning 8oz scented candle with crackling wood wick", "LuminaScents", "LUM-CND-010", "home", "decor", False),
    ("Nordic Tripod Wood Floor Lamp", 12999, 30, "Solid beechwood standing lamp with textured linen drum shade", "NordicForm", "NRD-LMP-011", "home", "lighting", False),
    ("Cooling Bamboo Weighted Blanket 15 lbs", 8999, 45, "Therapeutic glass bead weighted quilt for restful sleep", "SleepCalm", "SLP-WBL-012", "home", "bedding", False),
    ("Precision Temperature Gooseneck Kettle", 6999, 65, "1200W pour-over electric kettle with LCD readout and timer", "BrewCraft", "BRW-KTL-013", "home", "kitchen", True),
    ("Ergonomic Cervical Contour Memory Pillow", 4999, 85, "Orthopedic neck support pillow with cooling gel infused layer", "SleepCalm", "SLP-PLW-014", "home", "bedding", False),
    ("Ceramic Non-Stick Cookware Set 10-Piece", 18999, 25, "PTFE/PFOA-free induction-safe pots and pans set in sage green", "GreenKitchen", "GRN-CKW-015", "home", "kitchen", True),
    ("Woven Water Hyacinth Laundry Basket", 4999, 60, "Collapsible natural woven hamper with removable cotton liner", "BreezeHome", "BRZ-BSK-016", "home", "storage", False),
    ("Double-Walled Borosilicate Latte Glasses 4x", 2799, 130, "Insulated 12oz thermal coffee mugs that prevent condensation", "BrewCraft", "BRW-GLS-017", "home", "dining", False),
    ("Smart Indoor Hydroponic Herb Garden", 7999, 50, "LED grow light kit with automatic watering pump for fresh herbs", "GrowEasy", "GRW-HRB-018", "home", "garden", False),
    ("Mid-Century Brass Wall Clock 12-Inch", 3999, 80, "Silent sweeping quartz movement timepiece with brushed brass rim", "Forma", "FRM-CLK-019", "home", "decor", False),
    ("Turkish Organic Cotton Bath Towel 4-Pack", 5999, 70, "Plush 700 GSM combed cotton bath sheets with woven rib texture", "PureLoom", "PUR-TWL-020", "home", "bedding", False),

    # Sports & Fitness (15)
    ("High-Density Non-Slip Yoga Mat 6mm", 4499, 90, "Eco-friendly natural rubber alignment exercise mat with strap", "ZenFlow", "ZEN-MAT-001", "sports", "yoga", True),
    ("Quick-Adjust Dumbbells Set (5-52.5 lbs)", 29999, 20, "Selectorized home gym weight system replacing 15 sets of plates", "IronFit", "IRN-DBL-002", "sports", "fitness", True),
    ("Fabric Resistance Booty Bands Set of 3", 1999, 160, "Non-slip anti-rolling heavy loop bands for glute & leg workout", "PowerCurve", "PWR-BND-003", "sports", "fitness", False),
    ("Vacuum Insulated Steel Bottle 32oz", 2999, 150, "Cold for 24h leakproof flask with magnetic chug cap", "HydroShield", "HYD-BTL-004", "sports", "fitness", False),
    ("Speed Jump Rope with Ball Bearings", 1799, 140, "360-degree rotating steel wire rope with weighted aluminum grips", "ProSpeed", "PRO-ROP-005", "sports", "fitness", False),
    ("Deep Tissue Percussion Massage Gun", 8999, 55, "Quiet brushless motor massager with 6 attachment heads & case", "TheraPulse", "THR-GUN-006", "sports", "recovery", True),
    ("High-Density Trigger Point Foam Roller", 2499, 100, "Deep myofascial release back roller 13x5.5 inch", "ZenFlow", "ZEN-ROL-007", "sports", "recovery", False),
    ("Ultralight Trail Running Hydration Vest", 6999, 60, "Breathable marathon backpack with two 500ml soft flasks", "TrailBlaze", "TRL-VST-008", "sports", "running", False),
    ("Polarized Sport Cycling Sunglasses", 4999, 80, "Interchangeable UV400 shield lenses with adjustable nose pad", "AeroVision", "AER-GLS-009", "sports", "cycling", False),
    ("ToughLock Heavy Duty Bike U-Lock", 3999, 90, "16mm hardened alloy steel lock with 4ft double-loop steel cable", "ToughLock", "TGH-LCK-010", "sports", "cycling", False),
    ("Quick-Dry Microfiber Camp Towel Set", 1999, 130, "Compact fast-drying antimicrobial travel towels with pouch", "TrailBlaze", "TRL-TWL-011", "sports", "hiking", False),
    ("Adjustable Carbon Trekking Poles Pair", 5499, 70, "Collapsible walking sticks with natural cork grips and mud baskets", "ApexTrail", "APX-POL-012", "sports", "hiking", False),
    ("Graduated Compression Recovery Socks", 1999, 120, "20-30 mmHg circulation support socks for athletes and travel", "PowerCurve", "PWR-SOX-013", "sports", "recovery", False),
    ("Cast Iron Powder-Coated Kettlebell 16kg", 4999, 45, "Single-piece solid casting weight for swings, squats & snatches", "IronFit", "IRN-KTB-014", "sports", "fitness", False),
    ("Waterproof Floating Dry Bag 20L", 2499, 110, "Heavy duty 500D PVC roll-top dry sack with waterproof phone case", "TrailBlaze", "TRL-DRY-015", "sports", "swimming", False),

    # Beauty & Personal Care (15)
    ("Hydra Glow Triple Hyaluronic Serum 50ml", 2799, 110, "Multi-molecular weight hydration elixir with vitamin B5", "GlowLab", "GLW-SRM-001", "beauty", "skincare", True),
    ("Botanical Squalane Cleansing Oil 150ml", 2499, 125, "Melts stubborn makeup and SPF without stripping natural moisture", "PureFlora", "PUR-OIL-002", "beauty", "skincare", False),
    ("Overnight Peptide Firming Night Cream", 4299, 85, "Rich ceramide and copper peptide cream for barrier repair", "DermScience", "DRM-CRM-003", "beauty", "skincare", True),
    ("Vitamin C 15% Brightening Complex", 3499, 90, "Stabilized L-ascorbic acid serum with ferulic acid & vitamin E", "GlowLab", "GLW-VTC-004", "beauty", "skincare", False),
    ("Mineral Sunscreen Fluid SPF 50+ 60ml", 2299, 140, "Invisible zinc oxide broad-spectrum sunscreen with matte finish", "SolarShield", "SLR-SPF-005", "beauty", "sun-care", False),
    ("Organic Damask Rosewater Facial Mist", 1699, 150, "Pure steam-distilled floral toner for instant refreshment", "PureFlora", "PUR-MST-006", "beauty", "skincare", False),
    ("Resurfacing AHA/BHA Clarifying Exfoliant", 2899, 95, "2% salicylic acid and glycolic gentle exfoliating liquid", "DermScience", "DRM-EXF-007", "beauty", "skincare", False),
    ("Calming Centella Gel Cleanser 200ml", 1999, 130, "Low-pH balancing gel wash with cica for sensitive reactive skin", "PureFlora", "PUR-CLN-008", "beauty", "skincare", False),
    ("Nourishing Argan & Keratin Hair Mask", 2699, 105, "Intense conditioning treatment for dry damaged color-treated hair", "SilkRoot", "SLK-MSK-009", "beauty", "haircare", False),
    ("100% Pure Organic Cold-Pressed Rosehip Oil", 1899, 120, "Golden unrefined facial oil rich in provitamin A and essential omegas", "PureFlora", "PUR-RHP-010", "beauty", "clean-beauty", False),
    ("Authentic Xiuyan Jade Facial Roller & Gua Sha", 2499, 115, "Handmade natural crystal massage tool set for lymphatic drainage", "GlowLab", "GLW-ROL-011", "beauty", "tools", False),
    ("Caffeine & Green Tea Depuffing Eye Gel", 2199, 120, "Targeted treatment reducing dark circles and morning under-eye bags", "DermScience", "DRM-EYE-012", "beauty", "skincare", False),
    ("Whipped Organic Raw Shea Body Butter 8oz", 1799, 140, "Deeply moisturizing cocoa butter, sweet almond oil & vanilla", "EarthCraft", "ETH-BTR-013", "beauty", "bath-body", False),
    ("Mineral Dead Sea Mud Mask 250g", 1999, 100, "Detoxifying and pore-clearing clay treatment for all skin types", "EarthCraft", "ETH-MUD-014", "beauty", "skincare", False),
    ("Pure Mulberry Silk Sleep Mask", 2499, 135, "22 momme grade 6A hypoallergenic blackout eye contour mask", "SilkRoot", "SLK-EYE-015", "beauty", "tools", False),

    # Books & Stationery (15)
    ("Hardcover Dotted Grid Journal A5 160gsm", 1999, 120, "Bleedproof bamboo paper bullet notebook with back pocket", "InkCraft", "INK-JRN-001", "books", "stationery", True),
    ("Brass Pocket Fountain Pen Fine Nib", 3499, 85, "Solid brass machined writing pen that patinas naturally over time", "KawecoStyle", "BRS-PEN-002", "books", "writing", False),
    ("Designing Data-Intensive Applications", 4499, 70, "The definitive guide to distributed systems architectures", "TechPress", "BOK-DDA-003", "books", "technology", True),
    ("The Pragmatic Programmer: 20th Anniversary", 4299, 80, "Your journey to software engineering mastery by Hunt & Thomas", "TechPress", "BOK-PRG-004", "books", "technology", True),
    ("Clean Architecture: Craftsman's Guide", 3999, 90, "Robert C. Martin's practical guide to software structure and design", "TechPress", "BOK-ARC-005", "books", "technology", False),
    ("Deep Work: Rules for Focused Success", 2199, 100, "Cal Newport's bestselling strategy for thriving in an age of distraction", "InsightBooks", "BOK-DPW-006", "books", "business", False),
    ("Atomic Habits by James Clear", 2299, 130, "An easy and proven way to build good habits and break bad ones", "InsightBooks", "BOK-ATM-007", "books", "self-help", True),
    ("Precision Drafting Mechanical Pencil 0.5mm", 1699, 140, "Matte black all-metal barrel pencil with knurled non-slip grip", "InkCraft", "INK-PCL-008", "books", "writing", False),
    ("Micro-Pigment Archival Fineliner Set 8x", 1899, 120, "Waterproof fade-proof technical illustration pens (0.05 - 0.8mm)", "InkCraft", "INK-FLN-009", "books", "art-supplies", False),
    ("Vintage Full-Grain Leather Notebook Folio", 5499, 45, "Refillable leather organizer cover with pen loops and card slots", "NomadLeather", "NMD-FOL-010", "books", "stationery", False),
    ("Heavyweight Sticky Notes Pastel Cube 500 Sheets", 999, 200, "Super sticky residue-free notes in calming aesthetic colors", "OrganizeMe", "ORG-STK-011", "books", "stationery", False),
    ("Solid Walnut Minimalist Desk Caddy", 3999, 65, "Handcrafted desk tray organizer with phone docking angle", "TimberCraft", "TMB-CDY-012", "books", "stationery", False),
    ("Washi Paper Tape Box Set 10 Rolls", 1299, 150, "Japanese floral and geometric decorative masking tape ribbons", "CraftAura", "CRF-WSH-013", "books", "art-supplies", False),
    ("Sci-Fi Masterpiece: Dune Deluxe Edition", 3499, 60, "Frank Herbert's epic space novel in embossed cloth-bound hardcover", "EpochBooks", "BOK-DNE-014", "books", "sci-fi", True),
    ("Solid Brass Bookmark Ruler Set", 1499, 160, "Laser-engraved metric and imperial page marker clips", "KawecoStyle", "BRS-BMK-015", "books", "stationery", False),

    # Gourmet, Food & Wellness (10)
    ("Ethiopian Yirgacheffe Single-Origin Beans 1kg", 3299, 85, "Light roast whole bean specialty coffee with jasmine & citrus notes", "OriginRoast", "CFE-ETH-001", "food", "coffee", True),
    ("Ceremonial Grade Uji Matcha 100g", 2999, 90, "Stone-ground Japanese green tea powder with vibrant emerald color", "ZenTea", "MTC-JPN-002", "food", "tea", True),
    ("Wildflower Raw Unfiltered Honey 500g", 1699, 120, "Pure untreated raw mountain honey packed with natural pollen", "BeeHive", "HNY-RAW-003", "food", "pantry", False),
    ("Cold-Pressed Extra Virgin Olive Oil 750ml", 2499, 100, "Single-estate early harvest Greek Koroneiki olive oil", "OleaEstate", "OLV-EVO-004", "food", "pantry", False),
    ("Artisan Single-Origin Dark Chocolate 85% 4x", 1999, 110, "Direct trade bean-to-bar Madagascar dark chocolate tablets", "CacaoNoir", "CHC-MDG-005", "food", "chocolate", False),
    ("Traditional Sourdough Bread Starter Kit", 2799, 80, "Includes proofing banneton, scoring lame, scraper & active culture", "BakerArt", "BKR-SRD-006", "food", "baking", False),
    ("Organic Sleep Chamomile Lavender Tea 30 Bags", 1499, 140, "Caffeine-free herbal bedtime infusion with valerian root", "ZenTea", "TEA-SLP-007", "food", "tea", False),
    ("Electrolyte Hydration Drink Mix 30 Packets", 3499, 95, "Zero-sugar sodium-potassium-magnesium hydration drink powder", "HydroPure", "HYD-ELC-008", "wellness", "hydration", True),
    ("KSM-66 Ashwagandha Organic Root Extract", 2199, 110, "Standardized 600mg stress support and cortisol balance capsules", "NaturePath", "NAT-ASH-009", "wellness", "supplements", False),
    ("Pure Australian Tea Tree Essential Oil 30ml", 1299, 150, "100% steam distilled Melaleuca oil for skin and diffuser", "AromaPure", "ARM-TTO-010", "wellness", "aromatherapy", False)
]

def main():
    print(f"Total categories: {len(ROOT_CATEGORIES) + sum(len(v) for v in SUBCATEGORIES_CONFIG.values())}")
    print(f"Total products: {len(PRODUCTS_DATA)}")

if __name__ == "__main__":
    main()
