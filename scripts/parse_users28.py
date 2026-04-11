#!/usr/bin/env python3
"""Debug parser for Habbo Origins USERS(28).

This is now optional/debug-only.
The Go app should no longer depend on this script for live trade/chat identity.

It parses:
[count:int]
repeat count times:
  [roomIndex:int]
  [name:string]
  [figure:string]
  [gender:string]
  [motto:string]
  [x:int]
  [y:int]
  [z:string]
  [poolFigure:string]
  [badgeCode:string]
  [entityType:int]
"""
import argparse
import binascii
import json
import re
import sys
from pathlib import Path


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
    pairs = re.findall(r"\b[0-9a-fA-F]{2}\b", txt)
    if pairs:
        return bytes(int(x, 16) for x in pairs)
    cleaned = re.sub(r"[^0-9a-fA-F]", "", txt)
    if len(cleaned) % 2 == 1:
        cleaned = cleaned[:-1]
    return bytes.fromhex(cleaned)


class Reader:
    def __init__(self, data: bytes):
        self.data = data
        self.pos = 0

    def read_integer(self) -> int:
        if self.pos >= len(self.data):
            raise ValueError("integer: out of data")
        n = vl64_decode_len(self.data[self.pos])
        if n <= 0 or self.pos + n > len(self.data):
            raise ValueError(f"integer: invalid length {n} at {self.pos}")
        chunk = self.data[self.pos:self.pos+n]
        self.pos += n
        return vl64_decode(chunk)

    def read_string(self) -> str:
        end = self.data.find(b"\x02", self.pos)
        if end == -1:
            raise ValueError(f"string terminator not found from {self.pos}")
        raw = self.data[self.pos:end]
        self.pos = end + 1
        return raw.decode("latin-1", errors="replace")


def parse_users28(data: bytes):
    if len(data) >= 2:
        try:
            if b64_decode(data[:2]) == 28:
                data = data[2:]
        except Exception:
            pass

    r = Reader(data)
    count = r.read_integer()
    users = []
    for _ in range(count):
        room_index = r.read_integer()
        name = r.read_string()
        figure = r.read_string()
        gender = r.read_string()
        motto = r.read_string()
        x = r.read_integer()
        y = r.read_integer()
        z = r.read_string()
        pool_figure = r.read_string()
        badge_code = r.read_string()
        entity_type = r.read_integer()

        token_hex = None
        if len(name) >= 5:
            prefix = name[:4]
            tail = name[4:]
            if all(64 <= ord(c) <= 125 for c in prefix) and tail and tail[0].isupper():
                token_hex = prefix.encode("latin-1").hex()
                name = tail

        users.append({
            "name": name,
            "detected_name": name,
            "aliases": [name] if name else [],
            "token_hex": token_hex,
            "short_token": None,
            "chat_id": room_index,
            "room_index": room_index,
            "figureString": figure,
            "motto": motto,
            "gender": gender,
            "x": x,
            "y": y,
            "z": z,
            "poolFigure": pool_figure,
            "badgeCode": badge_code,
            "entityType": entity_type,
            "figure_match": False,
            "matched_alias": None,
        })
    return {"users": users, "trades": []}


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--hex", "-x")
    p.add_argument("--file", "-f")
    p.add_argument("--json", action="store_true")
    args = p.parse_args()

    if args.hex:
        data = hex_from_hexdump(args.hex)
    elif args.file:
        data = Path(args.file).read_bytes()
    else:
        print("Need --hex or --file", file=sys.stderr)
        sys.exit(1)

    out = parse_users28(data)
    if args.json:
        print(json.dumps(out, ensure_ascii=False))
    else:
        print(json.dumps(out, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
