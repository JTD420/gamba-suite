# Identity, Chat & Trade Detection — How It Works

This document explains exactly how `main.go` finds and tracks users in the room, detects who is talking,
and handles all trade events. Written as a reference so the logic can be understood and maintained at any time.

---

## Table of Contents

1. [Framework Overview](#framework-overview)
2. [Packet Header Quick Reference](#packet-header-quick-reference)
3. [User Identity System](#user-identity-system)
   - [Data Structures](#data-structures)
   - [USERS28 — How We Learn Who Is In The Room](#users28--how-we-learn-who-is-in-the-room)
   - [RoomEntities — Structured User Cache](#romentities--structured-user-cache)
   - [Identity Lookup Functions](#identity-lookup-functions)
   - [Requesting a Room User Refresh](#requesting-a-room-user-refresh)
4. [Chat Detection](#chat-detection)
   - [How We Resolve Who Said Something](#how-we-resolve-who-said-something)
   - [Game Choice Matching](#game-choice-matching)
5. [Trade Detection](#trade-detection)
   - [How a Trade is Opened](#how-a-trade-is-opened)
   - [Resolving the Trade Partner's Name](#resolving-the-trade-partners-name)
   - [Trade Items (Header 108)](#trade-items-header-108)
   - [Trade Accept / Confirm Flow](#trade-accept--confirm-flow)
   - [Trade Closed (Header 110)](#trade-closed-header-110)
   - [Trade Completed (Header 112)](#trade-completed-header-112)
6. [Trade Guard — Blocking Unwanted Trades](#trade-guard--blocking-unwanted-trades)
7. [Payout Trade Flow](#payout-trade-flow)
8. [Hand / Inventory Scanning](#hand--inventory-scanning)
9. [Encoding Formats](#encoding-formats)
10. [Full Resolution Chain Summary](#full-resolution-chain-summary)

---

## Framework Overview

The extension runs on **`xabbo.b7c.io/goearth`** — a Go library that acts as a proxy between the Habbo
Shockwave client and the game server. Everything is event-driven via packet intercepts.

```go
// All packet routing is wired up in setupExt()
a.ext.Intercept(in.CHAT, in.CHAT_2, in.CHAT_3).With(a.handleIncomingChat)
a.ext.Intercept(in.USERS).With(a.handleRoomUsers)
a.ext.Intercept(in.SPACENODEUSERS).With(a.handleRoomUsers)
a.ext.Intercept(out.CHAT).With(a.handleTalk)
a.ext.InterceptAll(func(e *g.Intercept) {
    handleTradePacket(a, e)
    handleUsers28Packet(a, e)
    // ...
})
```

`g.In` = packets arriving from the server.  
`g.Out` = packets the client is sending to the server.  
`e.Block()` prevents the packet from reaching its destination.

---

## Packet Header Quick Reference

| Header | Direction | Name / Purpose |
|--------|-----------|----------------|
| 28 | In | **USERS28** — raw room user list with tokens, names, indices |
| 61 | Out | **G_USRS** — request to get the room user list |
| 65 | Out | **GETSTRIP** — request a page of the dealer's hand/inventory |
| 71 | Out | **TRADE_OPEN** — open a trade with a target by room index |
| 72 | Out | **TRADE_ADDITEM** — add an item to an open trade |
| 104 | In | **TRADE_OPEN** (server confirmation) — trade window opened |
| 108 | In | **TRADE_ITEMS** — full current state of both trade offers |
| 109 | In | **TRADE_ACCEPT** — partner accepted the trade |
| 110 | In | **TRADE_CLOSE** — trade window closed |
| 111 | In | **TRADE_CONFIRM** — both sides locked in, waiting for final confirm |
| 112 | In | **TRADE_COMPLETED** — trade successfully completed |
| 140 | In | **STRIPINFO_2** — a page of the dealer's hand inventory |
| CHAT | In | Normal in-room chat |
| CHAT_2 | In | Whisper |
| CHAT_3 | In | Shout |

---

## User Identity System

### Data Structures

There are **four parallel caches** that all store the same underlying information in different keying strategies.
They are all guarded by `users28Mu sync.Mutex`.

```go
users28ByToken     = map[string]string{}            // 4-byte raw token  -> username
users28ByIndex     = map[int]string{}               // room index        -> username
users28ByShortToken= map[string]string{}            // 2-byte chat token -> username
roomIdentityByShortToken = map[string]RoomIdentityEntry{} // 2-byte token -> full identity entry
```

`RoomIdentityEntry` stores all known data about one person:
```go
type RoomIdentityEntry struct {
    Name      string // display name
    Token     string // 4-byte raw token
    Short     string // 2-byte chat token
    ChatIndex int    // index used in CHAT packets
    RoomIndex int    // index used in TRADE_OPEN and USERS packets
}
```

There is also a separate room entity cache populated from the structured USERS packet (not header 28):
```go
roomEntities = map[int]room.Entity{} // guarded by roomMu
```

---

### USERS28 — How We Learn Who Is In The Room

**Trigger:** Any incoming packet with header value `28`.

**Handler:** `handleUsers28Packet()` → calls `extractUsers28Entries()`.

#### Raw Packet Layout

Habbo Shockwave USERS (header 28) is a raw binary blob. It is **not** a standard structured packet.
Each user record is located by scanning for figure-string markers (`hr-`, `hd-`), then working backwards
to extract the name, 4-byte token, and VL64-encoded room index.

```
... [VL64 roomIndex] [4-byte token] [optional uppercase marker byte] [username] \x02 hr-...\x02 hd-...
```

**Key parsing detail — the uppercase marker byte:**
Some usernames are prefixed with a capital-letter length marker (e.g. `MWebsedit` where `M` is the marker).
If the first **two** bytes of the extracted name candidate are both uppercase (`[A-Z]`), the first byte is
treated as a marker and skipped:

```go
adjNameStart := rawNameStart
if b[nameStart] >= 'A' && b[nameStart] <= 'Z' && b[nameStart+1] >= 'A' && b[nameStart+1] <= 'Z' {
    adjNameStart = rawNameStart + 1  // skip the marker byte
}
name := string(b[adjNameStart:nameEnd])
// Token is the 4 bytes immediately before adjNameStart (not rawNameStart)
token := string(b[adjNameStart-4 : adjNameStart])
```

> **Why this matters:** Getting `adjNameStart` wrong causes the 4-byte token to be extracted from the wrong
> offset, producing useless tokens and "Unknown" trade partner names. This was a previously fixed bug.

#### What Gets Stored

For each parsed user entry, all four caches are populated:
- `users28ByToken[token] = name`
- `users28ByIndex[roomIndex] = name`
- `users28ByShortToken[shortToken] = name` (2-byte sub-token candidates derived from the 4-byte token)
- `roomIdentityByShortToken[shortToken] = RoomIdentityEntry{...}`

Additionally, short-token candidates are generated from all 2-byte sub-slices of the 4-byte token that
pass printability checks (`isLikelyChatToken`).

The caches are **fully cleared** on every USERS28 arrival (not merged), so they always reflect the
current room state. Joined/left diffs are computed from the previous snapshot and logged.

---

### RoomEntities — Structured User Cache

**Trigger:** Any incoming packet handled by `handleRoomUsers()` that is NOT header 28.

The framework's `room.Entity` parser is used to read structured user data. Each parsed entity is stored
in `roomEntities[entity.Index]` and its name/token are also back-filled into the `users28By*` caches for
consistency.

If a name has a `[token][name]` format (detected by `splitTokenAndName()`), the token is separated and
stored in `users28ByToken` as well.

---

### Identity Lookup Functions

| Function | Key Used | Description |
|----------|----------|-------------|
| `lookupUsers28Token(token)` | 4-byte token | Returns name for a raw 4-byte trade token |
| `lookupUsers28Index(index)` | room index | Returns name for a room index; falls back to short-token derivation |
| `lookupUsers28NameIndex(name)` | name string | Reverse lookup: name → room index |
| `lookupRoomIdentityByChatIndex(index)` | chat index | Scans `roomIdentityByShortToken` for 2-byte tokens that decode to this chat index |
| `lookupRoomEntityIndexByName(name)` | name string | Scans `roomEntities` for a matching name |
| `lookupRoomEntityNameByIndex(index)` | room index | Returns name from `roomEntities` |
| `resolveTradeTokenToRoomIndex(token)` | 4-byte token | Scans `roomEntities` for an entity whose name starts with this token |

**Blocking wait variants** poll every 75ms until resolved or timeout:
- `waitForUsers28IndexName(index, timeout)` — waits for `lookupUsers28Index` to succeed
- `waitForUsers28NameIndex(name, timeout)` — waits for `lookupUsers28NameIndex` to succeed
- `waitForRoomEntityIndexByName(name, timeout)` — waits for `lookupRoomEntityIndexByName` to succeed

---

### Requesting a Room User Refresh

```go
func requestRoomUsers(a *App) {
    a.ext.Send(out.G_USRS)            // header 61
    a.ext.Send(out.GETSPACENODEUSERS) // legacy secondary request
    startIncomingHeaderSniff(8 * time.Second) // log all new incoming headers for 8s
}
```

This is called:
- On every `TRADE_OPEN` (header 104) to keep the index → name mapping fresh
- On `ROOM_READY` (room change)
- At the start of each payout attempt
- When `awaitingTradeOpen` is first set after dice setup

A rate-limit guard (`shouldRefreshRoomUsers()`) prevents hammering: only re-requests if last call was
more than 5 seconds ago.

---

## Chat Detection

### Registering the Intercept

```go
a.ext.Intercept(in.CHAT, in.CHAT_2, in.CHAT_3).With(a.handleIncomingChat)
```

- `in.CHAT` = normal room chat
- `in.CHAT_2` = whisper
- `in.CHAT_3` = shout

### Packet Layout

```
[int: sender chat-index] [string: message text]
```

The `index` here is the **chat index**, which is typically a small integer (≤ 512 in most rooms)
representing the sender's position in the room's entity list.

### How We Resolve Who Said Something

`resolveChatSenderName(index int)` is the canonical function. It tries four methods in order:

**1. Structured room entities cache (`roomEntities`):**
```go
entity, ok := roomEntities[index]  // fast O(1) lookup
```
If found, strip any embedded 4-byte token prefix using `splitTokenAndName()`.

**2. USERS28 index cache (`users28ByIndex`):**
Direct lookup, with a short-token fallback (derives 2-byte token from index via `shortTokenFromIndex`
and looks up in `users28ByShortToken`).

**3. roomIdentityByShortToken by chat index:**
Scans all entries in `roomIdentityByShortToken`, decodes the short token via `chatIndexFromShortToken`,
and matches the result to the requested index.

**4. Short token — final fallback:**
Derives a 2-byte B64 string from the index (`encodeB64(index, 2)`), validates it is printable,
and looks it up in `users28ByShortToken`.

**If still unresolved:** triggers `requestRoomUsers()` if the cache is stale (> 5s), then waits up to
300ms using `waitForUsers28IndexName(index, 300ms)`.

---

### Game Choice Matching

After a trade completes, `awaitingGameChoice = true` and every incoming chat message is checked.

The player's game choice (`"poker"`, `"21"`, `"13"`) is validated against the expected trade partner
using a 5-level fallback hierarchy:

| Level | Method |
|-------|--------|
| 1 | Direct chat index match: `index == awaitingGameChoicePartnerID` |
| 2 | Name match: `senderName == awaitingGameChoicePartnerName` (case-insensitive) |
| 3 | `lookupRoomEntityIndexByName(partnerName)` → compare to incoming index |
| 4 | `lookupUsers28NameIndex(partnerName)` → compare to incoming index |
| 5 | `lookupRoomIdentityByChatIndex(index)` → compare resolved name to partner name |

If the partner name is "Unknown" or empty, a last-resort path accepts the first chat from a known
sender (resolved name not empty, index ≤ 512).

An accepted game choice blocks the packet (`e.Block()`) so the word is not visible to the room, then
launches the appropriate game roll sequence.

---

## Trade Detection

All trade logic lives inside `handleTradePacket(a *App, e *g.Intercept)`, called from `InterceptAll`.

### How a Trade is Opened

**Server sends header 104** when a trade window opens between two players.

Payload layout:
```
[VL64: trader-room-index] [optional: more VL64/token data]
```

Processing steps:
1. `decodeLeadingVL64(e.Packet.Data)` — extract the trader's room index
2. `matchesRecentOutgoingTradeOpen(data, incomingTraderID)` — check if **we** opened this trade
3. Payout guard check (see [Trade Guard](#trade-guard--blocking-unwanted-trades))
4. Dealer-open guard check: if not awaiting a trade and we didn't initiate it → block and close
5. `go requestRoomUsers(a)` — refresh user list immediately
6. **Name resolution** (see below)
7. Log and announce via `ext.Send(out.SHOUT, "Trade Opened: \"Name\"")`

### How We Remember Our Own TRADE_OPEN

When the extension sends `out.TRADE_OPEN` (header 71 outgoing), the target ID is remembered:

```go
// Outgoing header 71 intercept
rememberOutgoingTradeOpenTarget(targetID)

// Stored in:
lastOutgoingTradeOpenID int
lastOutgoingTradeOpenAt time.Time
```

`matchesRecentOutgoingTradeOpen()` then checks incoming header 104 against this:
- If the target ID matches the leading VL64 or any VL64 value anywhere in the packet → it's our own trade
- Expires after 8 seconds

---

### Resolving the Trade Partner's Name

When header 104 arrives, name resolution uses this priority order:

**Step 1 — USERS28 index lookup:**
```go
indexName, indexOk := lookupUsers28Index(traderRoomIndex)
if !indexOk {
    // Wait up to 900ms for the just-requested room refresh
    indexName, indexOk = waitForUsers28IndexName(traderRoomIndex, 900*time.Millisecond)
}
```

**Step 2 — Token-based fallback:**
The first 4 bytes of the packet data are treated as the partner's raw legacy token:
```go
tradeToken := string(e.Packet.Data[:4])
if name, ok := lookupUsers28Token(tradeToken); ok {
    lastTradePartnerName = name
}
```

**Step 3 — Token-to-room-index resolution:**
```go
if idx, name, ok := resolveTradeTokenToRoomIndex(tradeToken); ok {
    lastTradePartnerID = idx
    lastTradePartnerName = name
}
```

**Step 4 — `extractTradePartnerID` regex fallback:**
If all else fails, a regex pattern (`tradeUserPattern = regexp.MustCompile(`\[(\d+)\]`$)`) is applied
to the raw payload to find a numeric ID.

The final resolved name and ID are stored in `lastTradePartnerName` and `lastTradePartnerID`.

---

### Trade Items (Header 108)

Every time an item is added or removed from either side, the server sends header 108 with the **full
combined item list** for the entire trade window.

**Ownership attribution:**
- When **we** send `out.TRADE_ADDITEM` (header 72 outgoing), `lastAddItemWasOurs = true`
- The next header 108 arrival reads this flag:
  - `wasOurs = true` → new item is in our offer → `currentOwnTradeItems = diffItems(allItems, currentTradeItems)`
  - `wasOurs = false` → new item is in partner's offer → `currentTradeItems = diffItems(allItems, currentOwnTradeItems)`

**Item parsing — `parseTradeItemsPacket()`:**

The raw data is split on `\x02` byte delimiters. Each field is passed to `extractTradeItemAndQuantity()`:
- **Legacy format:** `[token]|[classname]` — splits on `|`, takes the last part
- **Current format:** `[garbage]{[classname*quantity]` — splits on `{`, takes the right side

Class names must pass `isKnownTradeClassName()` — they must either match the `stripItemNameRe` regex
(e.g. `chair_plasty`, `cf_10_coin_gold`) or appear in the dealer's current hand scan.

---

### Trade Accept / Confirm Flow

**Header 109 — Partner accepted:** `scheduleAutoTradeAccept()` waits 2 seconds then sends `out.TRADE_ACCEPT` (header 69).

**Header 111 — Both locked in:** `scheduleAutoTradeConfirm()` waits 4 seconds then sends `out.TRADE_CONFIRM_ACCEPT` (header 402, registered as a custom header at startup).

Both auto-flows are cancelled if the trade closes early (via the `tradeAutoFlowID` counter incremented every reset).

---

### Trade Closed (Header 110)

Fired when the trade window closes for any reason (either player cancels, or after completion).

Logic flow:
1. Stop all trade monitors
2. Suppress the "Trade Closed" shout if `suppressNextTradeCloseAnnouncement` is set
3. If `!wasCompleted` (closed before confirm):
   - If in payout mode (`pokerPayoutTradeActive`) → retry payout
   - Otherwise → `go a.resyncHandThenOpenDealer()` (full reset)
4. If `wasCompleted` and `pokerPayoutTradeActive` → payout finished, clear flag and resync
5. Clear `currentTradeItems` and `currentOwnTradeItems`

---

### Trade Completed (Header 112)

Fired on successful trade confirmation. At this point item transfer is final.

- Sets `tradeCompleted = true`
- If payout trade: logs completion, shouts `"Trade Completed: \"Name\"`
- If normal bet trade:
  - Calls `sendTradeCompletionMessage()` which:
    - Snapshots `currentTradeItems` → `pokerGameBetItems` (items bet by player, used for payout math)
    - Shouts the game choice prompt: `"PlayerName what game do you want to play?"`
    - Sets `awaitingGameChoice = true`
  - Triggers a hand rescan for payout preparation

---

## Trade Guard — Blocking Unwanted Trades

When an incoming header 104 (trade-open) arrives, a multi-layer guard decides whether to allow it.

### Layer 1 — Payout Mode Guard

If `pokerPayoutMode == true`:
- **`pokerPayoutTradeSent == true`** → this is our own payout trade opening → allow, set up payout state
- **`pokerPayoutTradeSent == false`** → someone else opened a trade during our payout → block + close it silently

### Layer 2 — Dealer Ready Guard

If the dealer is not in a ready state and the trade was not initiated by us:
```go
if !dealerReadyForNewTrade() && !matchedRecentOutgoing {
    // block
}
```

`dealerReadyForNewTrade()` returns `true` only when:
- `awaitingTradeOpen == true` (dealer is open)
- `dealerTradeWindowOpen == true` (dealer open was announced)
- `!dealerGameActive()` (no game currently rolling)
- `!dealerResyncInProgress` (not currently syncing hand)

### Silent Close Behaviour

When the guard blocks a trade:
```go
suppressNextTradeCloseAnnouncement = true
tradeCloseAnnounced = true
e.Block()
ext.Send(out.TRADE_CLOSE)
```

The `suppressNextTradeCloseAnnouncement` flag causes the resulting header 110 (TRADE_CLOSE) to skip the
"Trade Closed" shout, so the room doesn't see anything.

### `ignoreNextGuardCloseRecovery` Flag

If a trade is blocked **during an active game** (`dealerGameActive() == true`), setting this flag
prevents the resulting header 110 from triggering a full dealer reset — the game continues uninterrupted.

---

## Payout Trade Flow

When a game ends and the dealer owes a payout:

### 1. `startPokerPayout(a, targetID, targetName)`

Sets `pokerPayoutMode = true` and starts a goroutine that retries up to 5 times:

```
for attempt 1..5:
    requestRoomUsers  →  try to refresh the winner's room index
    waitForRoomEntityIndexByName / waitForUsers28NameIndex  →  correct any stale index
    rememberOutgoingTradeOpenTarget(targetID)               →  mark as our own TRADE_OPEN
    pokerPayoutTradeSent = true
    ext.Send(out.TRADE_OPEN, targetID)                      →  header 71 outgoing
    wait 5 seconds for header 104 confirmation
    if not confirmed → next attempt
```

### 2. Header 104 Opens Successfully

The payout guard detects `pokerPayoutTradeSent == true` → calls `stopPokerPayout()` (kills retry loop),
sets `pokerPayoutTradeActive = true`, then calls `go a.autoAddPayoutItems()`.

### 3. `autoAddPayoutItems()`

- Waits 600ms (settle time)
- Reads `pokerGameBetItems` to know what was bet
- Calculates `payoutRequirementsFromBetItems()` — bet quantity × 2 per item type
- Scans the dealer's hand (`snapshotHandItemIDs()`) for matching item IDs
- Sends `out.TRADE_ADDITEM` (header 72) for each item ID with 550ms delay between sends
- Up to 3 hand re-scan attempts if short on stock
- Auto-accepts the trade once all items are placed

### 4. Trade Cancellation / Retry

If header 110 (TRADE_CLOSE) fires while `pokerPayoutTradeActive == true` (and `!wasCompleted`):
```go
pokerPayoutTradeActive = false
startPokerPayout(a, retryTargetID, retryTargetName)  // retry
```

The payout never resets to dealer-open until either the trade completes or all 5 attempts exhaust.

---

## Hand / Inventory Scanning

The dealer's own furni hand is scanned by requesting `GETSTRIP` pages and parsing `STRIPINFO_2` responses.

### Request

```go
ext.Send(g.Out.Id("GETSTRIP"), []byte("new"))   // first page
ext.Send(g.Out.Id("GETSTRIP"), []byte("next"))  // subsequent pages
```

### Response — Header 140 (`STRIPINFO_2`)

Parsed by `parseStripInfoPageRaw()`:

Each record in the page contains:
- `mainItemID` (VL64) — primary physical item ID
- `extraCount` (VL64) — number of stacked copies
- `extraItemIDs` (N × VL64) — IDs of stacked items
- `typeChar` (`S` for floor item / `I` for wall item)
- `className` (string until `\x02`) — the furni class key (e.g. `chair_plasty`)

Quantity per record = `1 + extraCount`.

Pagination continues until a page repeats its first item ID (wrap detection) or 25 pages have been read.

Final results are stored in:
```go
currentHandItems   []TradeItem            // name + quantity per type
currentHandItemIDs map[string][]int       // name -> list of physical item IDs
```

---

## Encoding Formats

Habbo Shockwave uses two proprietary binary encoding formats in packet payloads:

### VL64

Variable-length signed integer. Length of the integer is encoded in the first byte.

```go
gencoding.VL64DecodeLen(firstByte)        // returns how many bytes to read
gencoding.VL64Decode(data[:vlen])         // decodes the integer
gencoding.VL64Encode(buf, value)          // encodes into buf
```

Used for: room indices, item IDs, record counts, trade target IDs.

### B64

Fixed-width base-64 encoding. Width specified at encode time (typically 2 or 3 bytes).

```go
gencoding.B64Decode([]byte(token))        // decodes 2-byte chat short-token
gencoding.B64Encode(buf, value)           // encodes into fixed-width buf
```

Used for: 2-byte chat tokens (which are how CHAT packets identify the speaker).

### Why Both Formats Matter

- Trade-open packets (headers 71 and 104) use **VL64** for the room index.
- Chat packets use **B64** (2-byte short token) for the sender index.
- USERS28 packets contain **both** — the room index is a VL64 immediately before the 4-byte raw token,
  and a 2-byte B64 short-token appears 2 bytes before the raw token.

---

## Full Resolution Chain Summary

### "Who opened this trade?"

```
Header 104 arrives
│
├─ decodeLeadingVL64(data)          →  traderRoomIndex
│
├─ lookupUsers28Index(roomIndex)
│   └─ if miss: shortTokenFromIndex → users28ByShortToken
│   └─ if still miss: waitForUsers28IndexName(roomIndex, 900ms)
│
├─ Token fallback: data[:4]         →  lookupUsers28Token(token)
│
└─ resolveTradeTokenToRoomIndex(token) via roomEntities
```

### "Who said this in chat?"

```
Header CHAT/CHAT_2/CHAT_3 arrives
│
Chat index = first int in packet
│
├─ roomEntities[index]              →  structured user, strip embedded token
│
├─ lookupUsers28Index(index)        →  users28ByIndex or users28ByShortToken
│
├─ lookupRoomIdentityByChatIndex(index) →  scan roomIdentityByShortToken
│
└─ shortTokenFromIndex(index) → users28ByShortToken[short]

If still unresolved → requestRoomUsers + wait 300ms
```

### "Is this the right trade partner for a game choice?"

```
Incoming CHAT message from index, senderName resolved
│
├─ index == awaitingGameChoicePartnerID ?          (direct match)
│
├─ senderName == awaitingGameChoicePartnerName ?   (name match)
│
├─ lookupRoomEntityIndexByName(partnerName) == index ?
│
├─ lookupUsers28NameIndex(partnerName) == index ?
│
├─ lookupRoomIdentityByChatIndex(index).Name == partnerName ?
│
└─ if partner name unknown → accept first chat from any resolved sender
```
