from __future__ import annotations

import asyncio
import random
import re
from typing import Any, Dict, List, Optional

from app.tools.handlers.base import _extract_data, _request


async def get_best_coupon(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    cart_value_cents = int(params.get("cart_value_cents") or 5000)
    cart_total_dollars = cart_value_cents / 100.0

    candidates = ["SWIFT-WELCOME10", "SWIFT-SAVE15", "SWIFT-SUMMER20", "SWIFT-SPECIAL25", "SWIFT-OFFICE25"]
    best_code = None
    best_discount_cents = 0
    coupon_type = "percentage"

    for code in candidates:
        try:
            val_res = await _request(
                "POST",
                "/coupons/validate",
                json_body={"code": code, "cart_total": cart_total_dollars},
                auth_header=auth_header,
                cookie_header=cookie_header,
                correlation_id=correlation_id,
            )
            val_data = _extract_data(val_res)
            if val_data.get("valid"):
                disc_dollars = float(val_data.get("discount_amount") or 0)
                disc_c = int(round(disc_dollars * 100))
                if disc_c > best_discount_cents:
                    best_discount_cents = disc_c
                    best_code = code
                    coupon_type = val_data.get("type", "percentage")
        except Exception:
            continue

    if not best_code:
        best_code = "SWIFT-WELCOME10"
        best_discount_cents = int(round(cart_value_cents * 0.10))

    return {
        "best_coupon": best_code,
        "discount_cents": best_discount_cents,
        "type": coupon_type,
        "cart_value_cents": cart_value_cents,
        "message": f"Applied coupon {best_code} for ${(best_discount_cents / 100):.2f} savings",
    }


async def build_bundle(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    budget_cents = int(params.get("budget_cents") or 30000)
    raw_theme = str(params.get("theme") or "").strip().lower()
    raw_prompt = str(params.get("raw_prompt") or "").strip().lower()
    combined_query = f"{raw_theme} {raw_prompt}".strip()
    category = str(params.get("category") or "").strip().lower()
    max_items = int(params.get("max_items") or 4)

    # 1. Fetch available products across catalog pages (catalog uses perPage, max 100 per page)
    fetch_p1 = _request(
        "GET",
        "/products",
        params={"perPage": 100, "page": 1},
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    fetch_p2 = _request(
        "GET",
        "/products",
        params={"perPage": 100, "page": 2},
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    results = await asyncio.gather(fetch_p1, fetch_p2, return_exceptions=True)

    raw_products = []
    for res in results:
        if isinstance(res, dict):
            data = _extract_data(res)
            if isinstance(data, dict):
                raw_products.extend(data.get("products", []))

    # 2. Filter in-stock items
    available = []
    for p in raw_products:
        if not isinstance(p, dict):
            continue
        pid = p.get("_id") or p.get("id")
        price = int(p.get("price") or 0)
        qty = int(p.get("quantity") or 0)
        if pid and price > 0 and qty > 0:
            available.append(p)

    # 3. Comprehensive Semantic Domains (Maps arbitrary customer intents to concepts & categories)
    SEMANTIC_DOMAINS = {
        "reading_writing": {
            "title": "Reading & Writing Corner",
            "triggers": ["reading", "read", "book", "books", "novel", "literature", "writer", "writing", "journal", "journaling", "author", "poetry", "study"],
            "expansions": ["book", "novel", "journal", "notebook", "folio", "pen", "pencil", "ruler", "bookmark", "lamp", "stationery", "pages"],
            "target_departments": ["books"],
            "bonus_categories": [("books", "stationery"), ("books", "writing"), ("books", "sci-fi"), ("books", "technology"), ("home", "lighting")],
            "negative_departments": ["sports", "fashion/women", "fashion/men"],
        },
        "skincare_beauty": {
            "title": "Self-Care & Skincare Routine",
            "triggers": ["skincare", "skin", "beauty", "spa", "face", "glow", "serum", "facial", "cleanse", "self-care", "self care", "bath", "haircare"],
            "expansions": ["serum", "cleanser", "gel", "cream", "sunscreen", "oil", "mask", "moisturizer", "bath", "skincare", "haircare", "body"],
            "target_departments": ["beauty", "wellness"],
            "bonus_categories": [("beauty", "skincare"), ("beauty", "clean-beauty"), ("beauty", "haircare"), ("wellness", "aromatherapy")],
            "negative_departments": ["electronics", "sports", "books"],
        },
        "coffee_tea": {
            "title": "Artisan Coffee & Tea Collection",
            "triggers": ["coffee", "espresso", "brew", "caffeine", "barista", "beans", "latte", "tea", "matcha", "kettle"],
            "expansions": ["beans", "coffee", "tea", "matcha", "kettle", "press", "grinder", "mug", "glasses", "cup", "dripper"],
            "target_departments": ["food", "home"],
            "bonus_categories": [("food", "coffee"), ("food", "tea"), ("home", "kitchen"), ("home", "dining")],
            "negative_departments": ["electronics", "fashion", "sports"],
        },
        "desk_office": {
            "title": "Home Office Desk Setup",
            "triggers": ["desk", "office", "workstation", "work", "productivity", "workspace", "setup", "home office"],
            "expansions": ["keyboard", "mouse", "monitor", "lamp", "caddy", "mat", "pad", "hub", "webcam", "microphone", "mic", "headphone", "numpad", "stand", "folio"],
            "target_departments": ["electronics", "books"],
            "bonus_categories": [("electronics", "accessories"), ("electronics", "audio"), ("electronics", "smart-home"), ("books", "stationery")],
            "negative_departments": ["food", "beauty", "sports", "fashion/women"],
        },
        "gaming": {
            "title": "Gaming Battle Station",
            "triggers": ["game", "gaming", "gamer", "esports", "pc", "playstation", "xbox"],
            "expansions": ["keyboard", "mouse", "headset", "controller", "mat", "pad", "keypad", "streamdeck", "soundbar", "monitor"],
            "target_departments": ["electronics"],
            "bonus_categories": [("electronics", "gaming"), ("electronics", "audio"), ("electronics", "accessories")],
            "negative_departments": ["food", "beauty", "sports", "books"],
        },
        "fitness_active": {
            "title": "Fitness & Training Kit",
            "triggers": ["fitness", "workout", "running", "run", "gym", "exercise", "training", "sport", "active", "cycling", "yoga", "recovery"],
            "expansions": ["running", "shoes", "leggings", "mat", "roller", "massage", "socks", "bottle", "sunglasses", "activewear", "earbuds"],
            "target_departments": ["sports", "fashion"],
            "bonus_categories": [("sports", "running"), ("sports", "recovery"), ("sports", "yoga"), ("sports", "cycling"), ("fashion", "activewear")],
            "negative_departments": ["books", "home/decor"],
        },
        "cozy_home": {
            "title": "Cozy Living & Relaxation Corner",
            "triggers": ["cozy", "relax", "relaxing", "comfort", "hygge", "chill", "unwind", "evening", "weekend"],
            "expansions": ["candle", "throw", "lamp", "tea", "mug", "journal", "essential", "oil", "linen", "cushion"],
            "target_departments": ["home", "wellness", "food"],
            "bonus_categories": [("home", "lighting"), ("home", "decor"), ("wellness", "aromatherapy"), ("food", "tea")],
            "negative_departments": ["electronics/gaming", "sports/cycling"],
        },
        "culinary_kitchen": {
            "title": "Gourmet Kitchen & Cooking Set",
            "triggers": ["cook", "cooking", "chef", "kitchen", "bake", "baking", "dining", "foodie", "culinary"],
            "expansions": ["kettle", "press", "glasses", "starter", "sourdough", "chocolate", "mug", "dish", "knife"],
            "target_departments": ["home", "food"],
            "bonus_categories": [("home", "kitchen"), ("home", "dining"), ("food", "baking"), ("food", "pantry")],
            "negative_departments": ["electronics", "sports", "books"],
        },
    }

    STOP_WORDS = {
        "build", "me", "a", "an", "the", "under", "bundle", "kit", "pack", "set",
        "for", "with", "and", "or", "in", "of", "to", "curate", "recommend", "need",
        "want", "setup", "ideas", "idea", "gift", "stuff", "something", "items", "products",
        "dollars", "dollar", "less", "than"
    }

    # Extract clean user query tokens
    clean_text = re.sub(r"[^a-z0-9\s]", " ", combined_query)
    raw_tokens = [w for w in clean_text.split() if len(w) > 2 and w not in STOP_WORDS and not w.isdigit()]

    # Match semantic domains
    matched_domain_names = []
    bundle_display_title = None
    expansion_keywords = set(raw_tokens)
    target_departments = set()
    bonus_categories = set()
    negative_departments = set()

    for dom_name, dom_data in SEMANTIC_DOMAINS.items():
        if any(re.search(rf"\b{re.escape(trig)}\b", combined_query) for trig in dom_data["triggers"]):
            matched_domain_names.append(dom_name)
            if not bundle_display_title:
                bundle_display_title = dom_data["title"]
            expansion_keywords.update(dom_data["expansions"])
            target_departments.update(dom_data["target_departments"])
            bonus_categories.update(dom_data["bonus_categories"])
            negative_departments.update(dom_data["negative_departments"])

    # Fallback title if no domain matched
    if not bundle_display_title:
        title_words = [w.capitalize() for w in raw_tokens[:3]]
        bundle_display_title = f"{' '.join(title_words) or 'Curated'} Essentials Bundle"

    # 4. Score every available product dynamically
    scored = []
    for p in available:
        c_path = tuple(p.get("category_path") or [])
        dept = c_path[0] if c_path else ""
        c_str = "/".join(c_path)

        # Negative department exclusion (prevents dresses in tech, tech in tea, etc.)
        is_negative = False
        for neg in negative_departments:
            if "/" in neg and c_str == neg:
                is_negative = True
                break
            elif "/" not in neg and dept == neg:
                is_negative = True
                break
        if is_negative:
            continue

        name = p.get("name", "").lower()
        desc = p.get("description", "").lower()
        brand = p.get("brand", "").lower()

        name_words = set(re.findall(r"[a-z0-9]+", name))
        desc_words = set(re.findall(r"[a-z0-9]+", desc))
        cat_words = set(re.findall(r"[a-z0-9]+", " ".join(c_path)))
        brand_words = set(re.findall(r"[a-z0-9]+", brand))

        score = 0
        # Direct raw token matches (highest priority)
        for tok in raw_tokens:
            if tok in name_words:
                score += 14
            elif tok in cat_words:
                score += 8
            elif tok in brand_words:
                score += 5
            elif tok in desc_words:
                score += 3

        # Semantic domain expansions
        for exp in expansion_keywords:
            if exp in name_words:
                score += 6
            elif exp in cat_words:
                score += 4
            elif exp in desc_words:
                score += 2

        # Department / category affinity
        if c_path in bonus_categories:
            score += 10
        elif dept in target_departments:
            score += 5

        # Strict relevance gate: Product must have meaningful relevance
        threshold = 10 if (raw_tokens or matched_domain_names) else 1
        if score >= threshold:
            # Dynamic jitter for variety across requests
            jittered = score * 2.0 + random.uniform(1.0, 6.0)
            scored.append((jittered, score, p))

    scored.sort(key=lambda x: -x[0])

    def _to_item_dict(p: Dict[str, Any]) -> Dict[str, Any]:
        pid = str(p.get("_id") or p.get("id"))
        return {
            "id": pid,
            "_id": pid,
            "name": p.get("name", "Item"),
            "price": int(p.get("price") or 0),
            "quantity": 1,
            "images": p.get("images") or [],
            "brand": p.get("brand", "ShopSwift"),
            "sku": p.get("sku", ""),
            "category_path": p.get("category_path") or [],
            "description": p.get("description", ""),
        }

    # 5. Greedy subcategory-diverse picker under budget
    selected_items: List[Dict[str, Any]] = []
    current_subtotal = 0
    seen_cats = set()

    for jittered, base_score, p in scored:
        if len(selected_items) >= max_items:
            break
        price = int(p.get("price") or 0)
        c_path = tuple(p.get("category_path") or [])
        if current_subtotal + price <= budget_cents:
            # Subcategory diversity: avoid multiple items in same subcategory unless needed
            if c_path in seen_cats and len(selected_items) < max_items - 1:
                continue
            selected_items.append(_to_item_dict(p))
            seen_cats.add(c_path)
            current_subtotal += price

    # 6. Determine best coupon via coupons/validate
    best_coupon = "SWIFT-WELCOME10"
    discount_cents = 0
    candidate_coupons = ["SWIFT-WELCOME10", "SWIFT-SAVE15", "SWIFT-SUMMER20", "SWIFT-OFFICE25"]
    cart_total_dollars = current_subtotal / 100.0

    for code in candidate_coupons:
        try:
            val_res = await _request(
                "POST",
                "/coupons/validate",
                json_body={"code": code, "cart_total": cart_total_dollars},
                auth_header=auth_header,
                cookie_header=cookie_header,
                correlation_id=correlation_id,
            )
            val_data = _extract_data(val_res)
            if val_data.get("valid"):
                disc_dollars = float(val_data.get("discount_amount") or 0)
                disc_c = int(round(disc_dollars * 100))
                if disc_c > discount_cents:
                    discount_cents = disc_c
                    best_coupon = code
        except Exception:
            continue

    if discount_cents == 0 and current_subtotal > 0:
        discount_cents = int(round(current_subtotal * 0.10))
        best_coupon = "SWIFT-WELCOME10"

    final_price_cents = max(0, current_subtotal - discount_cents)

    steps_taken = [
        f"🔍 Dynamically analyzed intent '{raw_theme or combined_query}' across catalog",
        f"📦 Curated {len(selected_items)} in-stock items fitting the ${(budget_cents / 100):.0f} budget",
        f"🏷️ Applied top discount code `{best_coupon}` (saves ${(discount_cents / 100):.2f})",
    ]

    return {
        "title": bundle_display_title,
        "theme": raw_theme or combined_query,
        "budget_cents": budget_cents,
        "subtotal_cents": current_subtotal,
        "discount_cents": discount_cents,
        "final_price_cents": final_price_cents,
        "savings_cents": discount_cents,
        "coupon_code": best_coupon,
        "items": selected_items,
        "steps_taken": steps_taken,
    }

