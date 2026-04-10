#!/usr/bin/env python3
"""Parse a Habbo USERS (header 28) blob, extract usernames + tokens, and query Origins API.

Usage examples:
  python scripts/parse_users28.py --hex "405c4b..."
  python scripts/parse_users28.py --file users28.bin

This version treats the Origins API as part of username boundary detection.
For each USERS28 figure field it finds the visible name run immediately before
that field, walks candidate starts from left to right, and stops on the first
candidate confirmed by the Origins API.
"""
import re
import argparse
import binascii
import json
import sys
from typing import Any

# --- Shockwave encoding helpers (VL64 / B64) ---
def vl64_decode_len(first_byte: int) -> int:
    return (first_byte >> 3) & 7


def vl64_decode(b: bytes) -> int:
    if len(b) == 0:
        raise ValueError("vl64: empty")
    if (b[0] & 0x40) != 0x40:
        raise ValueError(f"vl64: invalid first byte {b[0]:02x}")
    value = int(b[0] & 3)
    n = vl64_decode_len(b[0])
    if n <= 0:
        raise ValueError("vl64: length 0")
    for i in range(1, n):
        if (b[i] & 0x40) != 0x40:
            raise ValueError(f"vl64: invalid byte {b[i]:02x}")
        value |= int(b[i] & 0x3f) << (2 + 6 * (i - 1))
    if (b[0] & 4) != 0:
        value *= -1
    return value


def b64_decode(b: bytes) -> int:
    v = 0
    for i in range(len(b)):
        if (b[i] & 0x40) != 0x40:
            raise ValueError(f"b64: invalid byte {b[i]:02x}")
        v |= int(b[i] & 0x3f) << ((len(b) - i - 1) * 6)
    return v


def hex_from_hexdump(txt: str) -> bytes:
    pairs = re.findall(r"[0-9a-fA-F]{2}", txt)
    if pairs:
        return bytes(int(x, 16) for x in pairs)
    cleaned = re.sub(r"[^0-9a-fA-F]", "", txt)
    if len(cleaned) % 2 == 1:
        cleaned = cleaned[:-1]
    return bytes.fromhex(cleaned)


def is_name_char(b: int) -> bool:
    return (65 <= b <= 90) or (97 <= b <= 122) or (48 <= b <= 57) or b in (95, 45)


def is_likely_name_text(value: str) -> bool:
    value = value.strip()
    if len(value) < 2:
        return False
    return all(ch.isalnum() or ch in '_-' for ch in value)


def visible_name_run(data: bytes, name_end: int, window: int) -> tuple[int, int]:
    left_bound = max(0, name_end - window)
    start = name_end
    i = name_end - 1
    while i >= left_bound and is_name_char(data[i]):
        start = i
        i -= 1
    return start, name_end


def extract_candidate_name(data: bytes, start: int, end: int) -> str:
    raw = data[start:end]
    if not raw:
        return ""
    try:
        text = raw.decode("utf-8")
    except Exception:
        text = raw.decode("latin-1", errors="replace")
    return text.strip()


def generate_name_candidates(data: bytes, start: int, end: int) -> list[dict[str, Any]]:
    candidates: list[dict[str, Any]] = []
    seen = set()
    for cand_start in range(start, end - 1):
        text = extract_candidate_name(data, cand_start, end)
        if not is_likely_name_text(text):
            continue
        key = text.lower()
        if key in seen:
            continue
        seen.add(key)
        candidates.append({
            "candidate": text,
            "start": cand_start,
            "end": end,
        })
    return candidates


_ORIGINS_CACHE: dict[str, Any] = {}


def query_origins(username: str, api_base: str = "https://origins.habbo.com/api/public/users"):
    username = username.strip()
    if username == "":
        return None
    cache_key = f"{api_base}|{username.lower()}"
    if cache_key in _ORIGINS_CACHE:
        return _ORIGINS_CACHE[cache_key]
    try:
        import requests
    except Exception:
        from urllib import request, parse
        url = f"{api_base}?name={parse.quote(username)}"
        try:
            with request.urlopen(url, timeout=10) as r:
                if r.status == 200:
                    res = json.loads(r.read().decode())
                    _ORIGINS_CACHE[cache_key] = res
                    return res
        except Exception:
            _ORIGINS_CACHE[cache_key] = None
            return None
        _ORIGINS_CACHE[cache_key] = None
        return None

    try:
        r = requests.get(api_base, params={"name": username}, timeout=8)
        if r.status_code == 200:
            res = r.json()
            _ORIGINS_CACHE[cache_key] = res
            return res
    except Exception:
        pass
    _ORIGINS_CACHE[cache_key] = None
    return None


def api_name_matches(candidate: str, api: Any) -> bool:
    if not api or not isinstance(api, dict):
        return False
    api_name = str(api.get("name") or "").strip()
    return api_name != "" and api_name.lower() == candidate.lower()


def choose_best_origins_match(entry: dict[str, Any], api_base: str = "https://origins.habbo.com/api/public/users"):
    figure = entry.get("figureString") or ""
    for cand in entry.get("api_candidates") or []:
        api = query_origins(cand["candidate"], api_base=api_base)
        if not api_name_matches(cand["candidate"], api):
            continue
        api_figure = str(api.get("figureString") or "")
        return {
            "candidate": cand["candidate"],
            "api": api,
            "figure_match": bool(figure and api_figure and figure == api_figure),
        }
    return None


def infer_token_fields(data: bytes, name_start: int):
    token_hex = None
    token_bytes = None
    token_start_idx = None
    room_index = 0
    short_token = None
    chat_id = None

    if name_start < 4:
        return token_hex, token_bytes, token_start_idx, short_token, chat_id, room_index

    cand_token_start = name_start - 4
    scan_start = max(0, cand_token_start - 6)
    for start_off in range(scan_start, cand_token_start):
        try:
            vlen = vl64_decode_len(data[start_off])
        except Exception:
            continue
        if vlen <= 0 or vlen > 6 or start_off + vlen != cand_token_start:
            continue
        try:
            v = vl64_decode(data[start_off:cand_token_start])
        except Exception:
            continue
        if v <= 0:
            continue
        token_bytes = data[cand_token_start:name_start]
        token_hex = binascii.hexlify(token_bytes).decode()
        token_start_idx = cand_token_start
        room_index = v
        if cand_token_start >= 2:
            sc = data[cand_token_start - 2:cand_token_start]
            if len(sc) == 2 and all(32 <= c <= 126 for c in sc):
                short_token = sc.decode("latin-1", errors="replace")
                try:
                    chat_id = b64_decode(sc)
                except Exception:
                    chat_id = None
        break

    return token_hex, token_bytes, token_start_idx, short_token, chat_id, room_index


def find_user_entries(data: bytes, window: int = 64, api_base: str = "https://origins.habbo.com/api/public/users"):
    entries = []
    seen = set()
    figure_prefixes = (
        b"hd-", b"hr-", b"ch-", b"lg-", b"sh-",
        b"ha-", b"he-", b"ea-", b"fa-", b"ca-",
        b"cc-", b"wa-", b"cp-"
    )

    for idx in range(len(data) - 4):
        if data[idx] != 0x02:
            continue
        if not any(data[idx + 1:idx + 1 + len(prefix)] == prefix for prefix in figure_prefixes):
            continue

        name_end = idx
        run_start, _ = visible_name_run(data, name_end, window)
        api_candidates = generate_name_candidates(data, run_start, name_end)
        if not api_candidates:
            continue

        best_match = None
        best_candidate = None
        for cand in api_candidates:
            api = query_origins(cand["candidate"], api_base=api_base)
            if not api_name_matches(cand["candidate"], api):
                continue
            best_candidate = cand
            best_match = api
            break

        if best_candidate is None:
            best_candidate = max(api_candidates, key=lambda c: len(c["candidate"]))

        adj_name_start = best_candidate["start"]
        name = best_candidate["candidate"]
        aliases = [c["candidate"] for c in api_candidates]

        token_hex, token_bytes, token_start_idx, short_token, chat_id, room_index = infer_token_fields(data, adj_name_start)

        fig_start = name_end + 1
        fig_end = data.find(b"", fig_start)
        figure = None
        if fig_end != -1 and fig_end > fig_start:
            figure = data[fig_start:fig_end].decode("latin-1", errors="replace")

        motto = None
        if fig_end != -1:
            p = fig_end + 1
            if p < len(data) and data[p] == 0x6d:
                if p + 1 < len(data) and data[p + 1] == 0x02:
                    mot_start = p + 2
                    mot_end = data.find(b"", mot_start)
                    if mot_end != -1 and mot_end > mot_start:
                        motto = data[mot_start:mot_end].decode("latin-1", errors="replace")
            else:
                mot_start = fig_end + 1
                mot_end = data.find(b"", mot_start)
                if mot_end != -1 and mot_end > mot_start:
                    motto = data[mot_start:mot_end].decode("latin-1", errors="replace")

        key = (name.lower(), token_hex, room_index, name_end)
        if key in seen:
            continue
        seen.add(key)

        entries.append({
            "name": name,
            "aliases": aliases,
            "raw_name": extract_candidate_name(data, run_start, name_end),
            "raw_name_start": run_start,
            "adj_name_start": adj_name_start,
            "name_end": name_end,
            "token_hex": token_hex,
            "token_bytes": token_bytes,
            "short_token": short_token,
            "chat_id": chat_id,
            "room_index": room_index,
            "figureString": figure,
            "motto": motto,
            "api_candidates": api_candidates,
            "api_match_name": str(best_match.get("name") or "") if isinstance(best_match, dict) else None,
        })

    return entries


def find_trade_entries(data: bytes):
    entries = []
    try:
        s = data.decode('latin-1')
    except Exception:
        s = data.decode('latin-1', errors='replace')

    parts = s.split('')
    lower_re = re.compile(r'([a-z][a-z0-9_]{3,})')
    color_re = re.compile(r'(?:#(?:[0-9A-Fa-f]{6})(?:,#(?:[0-9A-Fa-f]{6}))*)')

    for i, p in enumerate(parts):
        if i == 0:
            continue
        m = lower_re.search(p)
        if not m:
            continue
        item = m.group(1)
        colors = None
        if i + 1 < len(parts) and '#' in parts[i + 1]:
            colors = parts[i + 1]
        else:
            cm = color_re.search(p)
            if cm:
                colors = cm.group(0)

        entries.append({
            'item': item,
            'colors': colors,
            'field_index': i,
            'fields': parts,
            'raw': s,
        })
    return entries


def main():
    p = argparse.ArgumentParser(description="Parse USERS(28) blob and query Origins API for usernames.")
    p.add_argument("--hex", "-x", help="Hex string or hexdump text containing the packet")
    p.add_argument("--file", "-f", help="Path to binary USERS28 packet file")
    p.add_argument("--api", default="https://origins.habbo.com/api/public/users", help="Origins API base URL")
    p.add_argument("--window", type=int, default=64, help="Bytes to look back when extracting username")
    p.add_argument("--trade", action="store_true", help="Extract trade item(s) from packet and print them")
    p.add_argument("--json", action="store_true", help="Output results as JSON (users + trades)")
    args = p.parse_args()

    if args.hex:
        data = hex_from_hexdump(args.hex)
    elif args.file:
        with open(args.file, "rb") as fh:
            data = fh.read()
    else:
        SAMPLE_HEX = (
            "405c4b4a615a774d466972656d616e0268722d3131352d313033352e68642d3138302d313032352e63682d3235352d313231382e6c672d3238302d313138392e73682d3330352d313236372e65612d313430342d31313839026d02533e206f73727320677020666f72206f726967696e730251415142302e30020202497374640273746402504148534e5041606d7b4d57656273656469740268722d3130352d313135352e68642d3139352d313031312e63682d3837372d313235352e6c672d3238312d313231332e73682d3330302d31323438026d020250415341302e30020202497374640273746402504148485141607f7b4d546f6d6164616368690268722d3130352d313034322e68642d3138352d313032352e63682d3231302d313238372e6c672d3238302d313238392e73682d3239302d31323632026d024f4320363433320250415341302e3002020249737464027374640250414848"
        )
        data = bytes.fromhex(SAMPLE_HEX)

    entries = find_user_entries(data, window=args.window, api_base=args.api)
    if not entries and not args.trade:
        print("No username entries found.")
        sys.exit(0)

    if args.trade:
        trades = find_trade_entries(data)
        if not trades:
            print("No trade items found.")
        else:
            for j, t in enumerate(trades, 1):
                print(f"\nTrade {j}:")
                print("  item:", t.get('item'))
                print("  colors:", t.get('colors'))
                print("  field_index:", t.get('field_index'))
                nearby = []
                for k in range(max(0, t['field_index'] - 1), min(len(t['fields']), t['field_index'] + 3)):
                    nearby.append(f"[{k}] {t['fields'][k]}")
                print("  context:", " | ".join(nearby))

    if args.json:
        out = {"users": [], "trades": []}
        for e in entries:
            try:
                best_match = choose_best_origins_match(e, api_base=args.api)
            except Exception:
                best_match = None

            origins = best_match["api"] if best_match else None
            canonical_name = e.get("name")
            if origins and origins.get("name"):
                canonical_name = origins.get("name")

            ue = {
                "name": canonical_name,
                "detected_name": e.get("name"),
                "aliases": e.get("aliases") or [],
                "raw_name": e.get("raw_name"),
                "raw_name_start": e.get("raw_name_start"),
                "adj_name_start": e.get("adj_name_start"),
                "name_end": e.get("name_end"),
                "token_hex": e.get("token_hex"),
                "short_token": e.get("short_token"),
                "chat_id": e.get("chat_id"),
                "room_index": e.get("room_index"),
                "figureString": e.get("figureString"),
                "motto": e.get("motto"),
                "figure_match": bool(best_match and best_match.get("figure_match")),
                "matched_alias": best_match.get("candidate") if best_match else None,
            }
            if origins:
                ue["origins"] = origins
            out["users"].append(ue)

        trades_list = find_trade_entries(data)
        for t in trades_list:
            out["trades"].append({"item": t.get("item"), "colors": t.get("colors"), "field_index": t.get("field_index")})

        print(json.dumps(out, ensure_ascii=False))
        sys.exit(0)

    for i, e in enumerate(entries, 1):
        print(f"\nEntry {i}:")
        print("  detected_name:", e.get('name'))
        print("  aliases:", ", ".join(e.get('aliases') or []))
        print("  room_index:", e.get('room_index'))
        print("  chat_id:", e.get('chat_id'))
        print("  short_token:", e.get('short_token'))
        print("  token_hex:", e.get('token_hex'))
        print("  figureString(packet):", e.get('figureString'))
        print("  motto(packet):", e.get('motto'))
        best_match = choose_best_origins_match(e, api_base=args.api)
        if best_match:
            api = best_match.get("api") or {}
            print("  Origins API: found")
            print("   matched_alias:", best_match.get("candidate"))
            print("   uniqueId:", api.get("uniqueId"))
            print("   name:", api.get("name"))
            print("   figureString(api):", api.get("figureString"))
            print("   figure match:", best_match.get("figure_match"))
        else:
            print("  Origins API: not found or request failed")


if __name__ == "__main__":
    main()
