#!/usr/bin/env python3
"""Parse a Habbo USERS (header 28) blob, extract usernames + tokens, and query Origins API.

Usage examples:
  python scripts/parse_users28.py --hex "405c4b..."
  python scripts/parse_users28.py --file users28.bin

The script scans for field separators (\x02), validates that the following bytes look like
known Habbo figure prefixes such as 'hd-', 'hr-', 'ch-', 'lg-' or 'sh-', extracts the
username immediately before the figure field, reads the 4-byte token immediately before the
username (adjusting for the uppercase marker rule), and pulls the in-packet figure string.
It then queries the Origins public user API to compare the figureString and print results.
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
    """Convert either a plain hex string or a hexdump (with offsets) into bytes."""
    import re
    # Try to find spaced hex byte pairs in the text (hexdump style)
    pairs = re.findall(r"\b[0-9a-fA-F]{2}\b", txt)
    if pairs:
        return bytes(int(x, 16) for x in pairs)
    # Otherwise strip non-hex and decode
    cleaned = re.sub(r"[^0-9a-fA-F]", "", txt)
    if len(cleaned) % 2 == 1:
        cleaned = cleaned[:-1]
    return bytes.fromhex(cleaned)


def is_likely_name_text(value: str) -> bool:
    value = value.strip()
    if len(value) < 2:
        return False
    return all(ch.isalnum() or ch in '_-' for ch in value)


def normalize_detected_name(name: str) -> tuple[str, list[str]]:
    name = name.strip()
    if not name:
        return "", []

    aliases = []
    seen = set()

    def add(candidate: str):
        candidate = candidate.strip()
        if not is_likely_name_text(candidate):
            return
        key = candidate.lower()
        if key in seen:
            return
        seen.add(key)
        aliases.append(candidate)

    add(name)
    for i in range(1, len(name) - 2):
        prefix = name[:i]
        if len(prefix) > 4:
            continue
        if not all(ch.islower() or ch.isdigit() or ch in '_-' for ch in prefix):
            continue
        if name[i].isupper() and name[i + 1].islower():
            add(name[i + 1:])

    if not aliases:
        aliases = [name]
    best = min(aliases, key=len)
    return best, aliases


def find_user_entries(data: bytes, window: int = 64):
    """Return list of detected entries with name, token, figureString and offsets."""
    entries = []
    seen = set()
    figure_prefixes = (
        b"hd-", b"hr-", b"ch-", b"lg-", b"sh-",
        b"ha-", b"he-", b"ea-", b"fa-", b"ca-",
        b"cc-", b"wa-", b"cp-"
    )
    name_pat = re.compile(r"([A-Za-z][A-Za-z0-9_-]{2,})$")
    fallback_pat = re.compile(r"([A-Za-z][A-Za-z0-9_-]{1,})$")

    for idx in range(len(data) - 4):
        if data[idx] != 0x02:
            continue
        if not any(data[idx + 1:idx + 1 + len(prefix)] == prefix for prefix in figure_prefixes):
            continue

        name_end = idx  # the 0x02 that precedes the figure marker
        pre_start = max(0, name_end - window)
        window_bytes = data[pre_start:name_end]
        window_str = window_bytes.decode("latin-1")

        m = name_pat.search(window_str) or fallback_pat.search(window_str)
        if not m:
            continue

        raw_name_start = pre_start + m.start(1)

        # Try multiple candidate starts within the matched window in case
        # preceding printable bytes confuse the marker-detection heuristic.
        name = None
        token_hex = None
        token_bytes = None
        token_start_idx = None
        short_token = None
        chat_id = None
        room_index = 0

        # Try multiple candidate starts within the matched window in case
        # preceding printable bytes confuse the marker-detection heuristic.
        # Collect all valid candidates and pick the one that yields the
        # longest username (to avoid splitting names like 'Fireman' into
        # a token + short tail).
        candidates = []
        # Determine the contiguous run of name-like characters and test
        # candidate starts from the right (closest to the hr- marker) leftwards.
        run_start = name_end - 1
        while run_start >= pre_start and (data[run_start:run_start+1].decode('latin-1', errors='ignore') != '' ) and ((65 <= data[run_start] <= 90) or (97 <= data[run_start] <= 122) or (48 <= data[run_start] <= 57) or data[run_start] in (95, 45)):
            run_start -= 1
        run_start += 1

        max_backtrack = min(32, name_end - run_start)
        for offset in range(0, max_backtrack):
            cand_raw = name_end - 2 - offset
            if cand_raw < run_start:
                break
            cand_adj = cand_raw
            if cand_raw + 1 < len(data) and (name_end - cand_raw) >= 3:
                b0, b1 = data[cand_raw], data[cand_raw + 1]
                if 65 <= b0 <= 90 and 65 <= b1 <= 90:
                    cand_adj = cand_raw + 1

            if cand_adj < 4:
                continue

            cand_token_start = cand_adj - 4
            scan_start = max(0, cand_token_start - 6)
            for start_off in range(scan_start, cand_token_start):
                try:
                    vlen = vl64_decode_len(data[start_off])
                except Exception:
                    continue
                if vlen > 0 and vlen <= 6 and start_off + vlen == cand_token_start:
                    try:
                        v = vl64_decode(data[start_off:cand_token_start])
                    except Exception:
                        continue
                    if v > 0:
                        token_b = data[cand_token_start:cand_adj]
                        name_b = data[cand_adj:name_end]
                        sc = None
                        chat = None
                        if cand_token_start >= 2:
                            sc = data[cand_token_start - 2:cand_token_start]
                            if len(sc) == 2 and all(32 <= c <= 126 for c in sc):
                                try:
                                    chat = b64_decode(sc)
                                except Exception:
                                    chat = None
                        candidates.append({
                            'cand_adj': cand_adj,
                            'cand_token_start': cand_token_start,
                            'room_index': v,
                            'token_bytes': token_b,
                            'token_hex': binascii.hexlify(token_b).decode(),
                            'name_bytes': name_b,
                            'short_token_bytes': sc,
                            'chat_id': chat,
                        })
                        break

        if candidates:
            # Prefer candidates whose username starts with an uppercase
            # letter (more likely to be the real display name). Among
            # those pick the one yielding the longest username. Otherwise
            # fall back to the longest username overall.
            upper_candidates = [c for c in candidates if len(c['name_bytes']) > 0 and 65 <= c['name_bytes'][0] <= 90]
            if upper_candidates:
                best = max(upper_candidates, key=lambda c: len(c['name_bytes']))
            else:
                best = max(candidates, key=lambda c: len(c['name_bytes']))
            adj_name_start = best['cand_adj']
            token_bytes = best['token_bytes']
            token_start_idx = best['cand_token_start']
            token_hex = best['token_hex']
            room_index = best['room_index']
            name_bytes = best['name_bytes']
            if best['short_token_bytes'] is not None:
                sc = best['short_token_bytes']
                if len(sc) == 2 and all(32 <= c <= 126 for c in sc):
                    short_token = sc.decode("latin-1", errors="replace")
                    try:
                        chat_id = b64_decode(sc)
                    except Exception:
                        chat_id = None
            try:
                name = name_bytes.decode("utf-8")
            except Exception:
                name = name_bytes.decode("latin-1", errors="replace")
            name, aliases = normalize_detected_name(name)

        if not candidates:
            # Permissive fallback: accept visible name even if token/room not found
            adj_name_start = raw_name_start
            if raw_name_start + 1 < len(data):
                b0, b1 = data[raw_name_start], data[raw_name_start + 1]
                if 65 <= b0 <= 90 and 65 <= b1 <= 90:
                    adj_name_start = raw_name_start + 1
            name_bytes = data[adj_name_start:name_end]
            try:
                name = name_bytes.decode("utf-8")
            except Exception:
                name = name_bytes.decode("latin-1", errors="replace")
            name, aliases = normalize_detected_name(name)

        # Figure string: bytes from name_end+1 until next 0x02
        figure = None
        fig_start = name_end + 1
        fig_end = data.find(b"\x02", fig_start)
        if fig_end != -1 and fig_end > fig_start:
            figure = data[fig_start:fig_end].decode("latin-1", errors="replace")

        # Motto heuristic: skip a single 'm' marker (0x6d) if present,
        # then read until the next 0x02 (optional)
        motto = None
        if fig_end != -1:
            p = fig_end + 1
            if p < len(data) and data[p] == 0x6d:  # 'm' marker
                # skip marker and the following 0x02 if present
                if p + 1 < len(data) and data[p + 1] == 0x02:
                    mot_start = p + 2
                    mot_end = data.find(b"\x02", mot_start)
                    if mot_end != -1 and mot_end > mot_start:
                        motto = data[mot_start:mot_end].decode("latin-1", errors="replace")
            else:
                mot_start = fig_end + 1
                mot_end = data.find(b"\x02", mot_start)
                if mot_end != -1 and mot_end > mot_start:
                    motto = data[mot_start:mot_end].decode("latin-1", errors="replace")

        # Short token is the two bytes immediately before the 4-byte token
        # Use the discovered token_start_idx when available; otherwise rely on
        # any short_token found during candidate scanning.
        if short_token is None:
            if 'token_start_idx' in locals() and token_start_idx is not None and token_start_idx >= 2:
                short_candidate = data[token_start_idx - 2:token_start_idx]
                if len(short_candidate) == 2 and all(32 <= c <= 126 for c in short_candidate):
                    short_token = short_candidate.decode("latin-1", errors="replace")
                    try:
                        chat_id = b64_decode(short_candidate)
                    except Exception:
                        chat_id = None

        key = (name, token_hex, room_index, name_end)
        if key in seen:
            continue
        seen.add(key)

        entries.append({
            "name": name,
            "aliases": aliases,
            "raw_name": data[adj_name_start:name_end].decode("latin-1", errors="replace"),
            "raw_name_start": raw_name_start,
            "adj_name_start": adj_name_start,
            "name_end": name_end,
            "token_hex": token_hex,
            "token_bytes": token_bytes,
            "short_token": short_token,
            "chat_id": chat_id,
            "room_index": room_index,
            "figureString": figure,
            "motto": motto,
        })

    return entries


def find_trade_entries(data: bytes):
    """Extract traded items from a TRADE packet blob.

    Returns a list of dicts: {item, colors, field_index, fields, raw}
    The function splits on 0x02 and searches each field for a lowercase
    item token (e.g. 'redhologram'). It also captures a following
    color palette field when present (comma-separated #RRGGBB values).
    """
    entries = []
    try:
        s = data.decode('latin-1')
    except Exception:
        s = data.decode('latin-1', errors='replace')

    parts = s.split('\x02')
    # require a minimum length (4 chars) to avoid short token false-positives
    lower_re = re.compile(r'([a-z][a-z0-9_]{3,})')
    color_re = re.compile(r'(?:#(?:[0-9A-Fa-f]{6})(?:,#(?:[0-9A-Fa-f]{6}))*)')

    for i, p in enumerate(parts):
        # skip the first field (usually the trade token/prefix)
        if i == 0:
            continue
        m = lower_re.search(p)
        if not m:
            continue
        item = m.group(1)
        colors = None
        # check the next field for a color palette
        if i + 1 < len(parts) and '#' in parts[i + 1]:
            # keep raw palette text
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


def query_origins(username: str, api_base: str = "https://origins.habbo.com/api/public/users"):
    """Query the Origins public users endpoint. Returns parsed JSON or None."""
    try:
        import requests
    except Exception:
        # fallback to urllib
        from urllib import request, parse

        url = f"{api_base}?name={parse.quote(username)}"
        try:
            with request.urlopen(url, timeout=10) as r:
                if r.status == 200:
                    return json.loads(r.read().decode())
        except Exception:
            return None
        return None

    try:
        r = requests.get(api_base, params={"name": username}, timeout=8)
        if r.status_code == 200:
            return r.json()
    except Exception:
        return None
    return None


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
        # Example sample (concise hex blob). Replace or pass --hex for real data.
        SAMPLE_HEX = (
            "405c4b4a615a774d466972656d616e0268722d3131352d313033352e68642d3138302d313032352e63682d3235352d313231382e6c672d3238302d313138392e73682d3330352d313236372e65612d313430342d31313839026d02533e206f73727320677020666f72206f726967696e730251415142302e30020202497374640273746402504148534e5041606d7b4d57656273656469740268722d3130352d313135352e68642d3139352d313031312e63682d3837372d313235352e6c672d3238312d313231332e73682d3330302d31323438026d020250415341302e30020202497374640273746402504148485141607f7b4d546f6d6164616368690268722d3130352d313034322e68642d3138352d313032352e63682d3231302d313238372e6c672d3238302d313238392e73682d3239302d31323632026d024f4320363433320250415341302e3002020249737464027374640250414848"
        )
        data = bytes.fromhex(SAMPLE_HEX)

    entries = find_user_entries(data, window=args.window)
    # If the user asked only for trade extraction, don't exit when no usernames
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
                # print the neighbouring fields for context
                nearby = []
                for k in range(max(0, t['field_index'] - 1), min(len(t['fields']), t['field_index'] + 3)):
                    nearby.append(f"[{k}] {t['fields'][k]}")
                print("  context:", " | ".join(nearby))

    # JSON output mode: emit both parsed users and trades as JSON and exit
    if args.json:
        out = {"users": [], "trades": []}
        # Prepare users (drop raw bytes, include token_hex)
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
