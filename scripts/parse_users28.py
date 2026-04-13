#!/usr/bin/env python3
import argparse
import json
import re
import sys
from typing import List, Dict, Optional, Tuple

FIGURE_RE = re.compile(r'^(hr|hd|ch|lg|sh|ea|ha|fa|ca|wa)-\d+-\d+')
MOTTO_SKIP_RE = re.compile(r'^[A-Z]{3,4}\d')


def read_packet_bytes(input_path: Optional[str]) -> bytes:
    if input_path:
        with open(input_path, 'rb') as f:
            return f.read()
    return sys.stdin.buffer.read()


def split_packet_fields(packet: bytes) -> List[str]:
    return [part.decode('ascii', errors='ignore').strip() for part in packet.split(b'\x02')]


def vl64_chunk_length(text: str) -> int:
    if not text:
        return 0
    length = (ord(text[0]) >> 3) & 7
    if length <= 0:
        length = 1
    return length


def decode_vl64(chunk: str) -> Optional[int]:
    if not chunk:
        return None
    try:
        vals = [ord(c) for c in chunk]
        first = vals[0]
        total_bytes = (first >> 3) & 7
        if total_bytes <= 0:
            total_bytes = 1
        negative = (first & 4) != 0
        value = first & 3
        shift = 2
        for b in vals[1:total_bytes]:
            value |= (b & 0x3F) << shift
            shift += 6
        if negative:
            value = -value
        return value
    except Exception:
        return None


def read_fixed_vl64_prefix(text: str, count: int) -> Tuple[List[str], str]:
    parts: List[str] = []
    remaining = text
    for _ in range(count):
        if not remaining:
            break
        length = vl64_chunk_length(remaining)
        current = remaining[:length]
        parts.append(current)
        remaining = remaining[length:]
    return parts, remaining


def prefix_count_from_live_block(text: str) -> int:
    if not text:
        return 3
    first_len = vl64_chunk_length(text)
    if first_len == 2:
        return 5
    return 3


def extract_entity_and_username(name_block: str) -> Dict[str, object]:
    original = name_block
    working = name_block
    if working.startswith('@\\'):
        working = working[2:]

    num_ints = prefix_count_from_live_block(working)
    parsed_ints, remaining = read_fixed_vl64_prefix(working, num_ints)

    chat_id_raw = parsed_ints[-2] if len(parsed_ints) >= 2 else ''
    trade_id_raw = parsed_ints[-1] if len(parsed_ints) >= 1 else ''
    entity_id = ''.join(parsed_ints)

    return {
        'entity_id': entity_id,
        'chat_id_raw': chat_id_raw,
        'trade_id_raw': trade_id_raw,
        'chat_id': decode_vl64(chat_id_raw),
        'trade_id': decode_vl64(trade_id_raw),
        'username': remaining,
        'raw_name_block': original,
    }


def parse_users28(packet: bytes) -> List[Dict[str, object]]:
    fields = split_packet_fields(packet)
    users: List[Dict[str, object]] = []

    for i, field in enumerate(fields):
        if not FIGURE_RE.match(field):
            continue
        if i == 0:
            continue

        parsed = extract_entity_and_username(fields[i - 1])
        sex = fields[i + 1] if i + 1 < len(fields) else ''

        motto = ''
        if i + 2 < len(fields):
            candidate = fields[i + 2]
            if (
                len(candidate) > 5
                and not MOTTO_SKIP_RE.match(candidate)
                and candidate not in {'Istd', 'std'}
            ):
                motto = candidate

        users.append({
            'username': parsed['username'],
            'trade_id': parsed['trade_id'],
            'trade_id_raw': parsed['trade_id_raw'],
            'chat_id': parsed['chat_id'],
            'chat_id_raw': parsed['chat_id_raw'],
            'entity_id': parsed['entity_id'],
            'figure': field,
            'sex': sex,
            'motto': motto,
        })

    users.sort(key=lambda u: (
        u.get('chat_id') if isinstance(u.get('chat_id'), int) and u.get('chat_id') is not None else -1,
        u.get('trade_id') if isinstance(u.get('trade_id'), int) and u.get('trade_id') is not None else -1,
        str(u.get('username', '')),
    ))
    return users


def main() -> int:
    parser = argparse.ArgumentParser(description='Parse raw USERS[28] packet bytes into JSON.')
    parser.add_argument('--input', help='Path to a file containing the exact raw packet bytes.')
    parser.add_argument('--json', action='store_true', help='Emit JSON only.')
    args = parser.parse_args()

    packet = read_packet_bytes(args.input)
    users = parse_users28(packet)

    if args.json:
        sys.stdout.write(json.dumps(users, ensure_ascii=False, separators=(',', ':')))
        return 0

    for user in users:
        sys.stdout.write(json.dumps(user, ensure_ascii=False) + '\n')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
