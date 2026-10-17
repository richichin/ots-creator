# The 13.x dat format (OTCv8 clients)

Tibia 13 clients built on OTCv8 (KingsVale among them) keep using a `Tibia.dat` +
`Tibia.spr` pair, but the dat flag list changed enough that the 10.10+ table no
longer fits: one flag lost its payload, one changed meaning and eleven new flags
appeared, one of them holding the NPC sale list. OTS Creator reads these clients
with `table7`, named "13.10+", which the client versions from 13.10 up select.

Everything here was read from the parser of such a client (the function behind the
`g_things.loadDat` binding) and checked against a real 6.27 MB dat: all 61 710
things parse up to the exact last byte and writing them back keeps the file size.

## File structure

| Field | Type |
| --- | --- |
| signature | `u32` (`0x4A10` for 13.10) |
| items, outfits, effects, missiles | `u16` each, the highest id of the category |
| then, for every thing in that order | flags, frame groups, sprite ids |

Items start at id 100, the other categories at 1. There is no extra section and no
name table: item names and descriptions live in `items.otb`, not in the dat.

## Flags

The client remaps every flag byte before switching on it, on clients 10.00 and up:

```
attr = raw        when raw <= 0x0F
attr = 0xFD       when raw == 0x10
attr = raw - 1    when raw  > 0x10
```

The switch then reads the payload of that attribute. Attributes without a case read
nothing, and an unknown flag byte behaves the same way: the client only marks the
attribute as present. That is why a newer client that adds flags still works with
the reader that ignores them.

| Flag | Attribute | Payload | OTS Creator |
| --- | --- | --- | --- |
| `0x00` | ground | `u16` speed | `groundSpeed` |
| `0x01`-`0x07` | | none | groundBorder … multiUse |
| `0x08` | writable | `u16` | `writable` |
| `0x09` | writable once | `u16` | `writableOnce` |
| `0x0A`-`0x0F` | | none | fluidContainer … blockPathfind |
| `0x10` | no move animation | none | `noMoveAnimation` |
| `0x11`-`0x15` | | none | pickupable … rotatable |
| `0x16` | light | `u16` level, `u16` color | `hasLight` |
| `0x17`, `0x18` | | none | dontHide, translucent |
| `0x19` | displacement | `i16` x, `i16` y | `hasOffset` |
| `0x1A` | elevation | `u16` | `hasElevation` |
| `0x1B`, `0x1C` | | none | lyingObject, animateAlways |
| `0x1D` | minimap | `u16` color | `miniMap` |
| `0x1E` | lens help | `u16` | `lensHelp` |
| `0x1F`, `0x20` | | none | fullGround, ignoreLook |
| `0x21` | cloth | `u16` slot | `cloth` |
| `0x22` | market | `u16` category, `u16` tradeAs, `u16` showAs, string name, `u16` profession, `u16` level | `market` |
| `0x23` | usable | `u16` | `hasDefaultAction` |
| `0x24`-`0x26` | | none | wrappable, unwrappable, topEffect |
| `0x27` | | **none** | not stored |
| `0x2F` | | `u16` | `Thing13` |
| `0x30` | NPC sale list | `u16` count, then per entry `u32`, `u32`, string, string | `npcSales` |
| `0x31`, `0x32` | | `u16` | `Thing13` |
| `0x33`-`0x36` | | none | `Thing13` |
| `0x37` | | `u32` count, then one `u32` each | `Thing13` |
| `0x38`, `0x39` | | `u32` | `Thing13` |
| `0x3A`, `0x3B` | | none | `Thing13` |
| `0xFE` | no move animation | none | `usable` |

`0x27` is the trap. In 10.x the bones flag carries four `(i16, i16)` offsets, so a
10.10+ table desyncs on the first item that has it. In this format the flag has no
payload at all and does not appear in a 13.10 dat.

`0x23` is called "usable" by the client but it is a `u16`; OTS Creator stores it in
the `u16` property it already has, the default action.

## NPC sale list (`0x30`)

A `u16` count, then for every entry:

| Field | Type | Meaning |
| --- | --- | --- |
| npc pays | `u32` | what the NPC pays the player → `NpcSale.BuyPrice` |
| player pays | `u32` | what the player pays the NPC → `NpcSale.SalePrice` |
| name | `u16` length + latin-1 | `NpcSale.Name` |
| location | `u16` length + latin-1 | `NpcSale.Location` |

The order of the two prices was checked against the data: an item a NPC sells has
the second one set (a health potion at 50), an item a NPC buys has the first one
set (Rashid paying 7500). There is no currency field, so `CurrencyObjectID` and
`CurrencyQuestFlag` stay zero. A real 13.10 dat has 2 622 things with a list and
15 204 entries.

## Frame groups

Identical to 10.57+: an outfit starts with a `u8` group count and every group with
a `u8` type; each group reads width, height, a `u8` exact size when either is above
one, layers, pattern x, pattern y, pattern z, frames, the animation block when
frames is above one (mode, `i32` loop count, `i8` start frame, one `(u32 min, u32
max)` pair per frame) and finally one `u32` per sprite slot.

## Flags the model has no field for

The eleven flags marked `Thing13` are read and kept verbatim in `dat.Thing13`, hung
off `thing.Thing.Extra` so `thing.Properties` stays comparable, and written back in
flag byte order. Nothing published describes them yet, so they are keyed by their
flag byte and only preserved.

## The otfi next to the dat

A 13.x client needs its `Tibia.otfi` beside the dat and the spr. Without it the
editor falls back to the defaults of the version and, because `transparency`
defaults to false, every sprite decodes with the wrong number of channels:

```
DatSpr
  extended: true
  transparency: true
  frame-durations: true
  frame-groups: true
  metadata-file: Tibia.dat
  sprites-file: Tibia.spr
  sprite-size: 32
  sprite-data-size: 4096
```

`sprite-data-size` must be `sprite-size * sprite-size * 4` with transparency and
`* 3` without it; the editor rejects an otfi that contradicts itself instead of
failing later on a pixel overflow.

## How it was found

The client ships without symbols, so the parser was located by a binding name: the
string `loadDat` leads to the Lua binding registration, that to the wrapper around
the thing reader, and that to the function whose switch covers exactly these flags
and whose error message is `corrupt data (id: %d, category: %d, ...)`. Reading the
switch gives the payload of every attribute; the remap above sits right before it.

## Checking a table

A wrong table does not fail, it desyncs and reads garbage things forever. The only
reliable check is that parsing ends exactly on the last byte of the file, which is
what `dat.Decode` enforces and what `dat.Detect` uses to pick a table when the one
the version asks for does not fit. To check a client by hand:

```
go run ./cmd/kvcheck "path/to/client"
```

It needs `Tibia.dat`, `Tibia.spr` and `Tibia.otfi` in that folder, and reports the
format it found, a few things, and whether a decode/encode/decode cycle of the
whole dat changes anything.