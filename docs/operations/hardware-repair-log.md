# Hardware Repair Log

This is the source of truth for every physical part that goes into or comes out of the fleet. It exists because in November 2026 a failed data01 disk got pinned on the wrong seller: the swap history lived only in `zpool history`, Prometheus, and order emails, and stitching those back together after the fact went wrong once before it went right. From here on, a part swap is not finished until it has a row here.

## How to add an entry

Append a row to the swap table in the same change as the swap, newest at the bottom. Times are Pacific (ocean's clock, which is also what `zpool history` prints). For every part record the model (part number), serial, and for disks the WWN, since WWN is how the pool names members. Record the vendor, the order number, and the unit price before tax. For any used, refurbished or renewed Seagate drive also record its Seagate FARM power-on hours next to the SMART ones at install time, because resellers zero SMART and FARM is the only honest odometer:

```
docker exec smart-exporter smartctl -i -A -l farm /dev/disk/by-id/wwn-<wwn>
```

When a field is genuinely unknown write `unknown`, and when it was reconstructed rather than recorded at the time, mark it `(inferred)` and say from what in the notes. A guess written down as fact is how the wrong-seller mistake happened.

## data01 disk swaps (ocean, raidz2-0)

| # | Date (PT) | Removed: WWN / serial | Installed: model / serial / WWN | Vendor, order, unit price | Notes |
|---|---|---|---|---|---|
| 0 | 2017-12-03 | n/a | 8 x HGST (WWNs `0x5000cca2...`), models and serials not recorded | unknown | Pool created on `/dev/sdX` names. |
| 1 | 2025-01-03 13:49 | `0x5000cca269c859e6` (HGST) | ST12000NM0127 / ZJV1QG9S / `0x5000c500b2281882` | Amazon `114-5471242-3628258`, $129.99 | Jan 2025 full re-disk off the 2017 HGSTs. Order was 8 x ST12000NM0127, $1,146.48 total, seller DBSKY (inferred from the Jan 2025 return). |
| 2 | 2025-01-04 07:17 | `0x5000cca269c77313` (HGST) | ST12000NM0127 / ZJV27W0A / `0x5000c500b30c1db0` | Amazon `114-5471242-3628258`, $129.99 | |
| 3 | 2025-01-04 22:12 | `0x5000cca24ce32eab` (HGST) | ST12000NM0127 / unknown / `0x5000c500b66e2a23` | Amazon `114-5471242-3628258`, $129.99 | Serial unknown: removed before Prometheus retention starts (2026-05-14). |
| 4 | 2025-01-05 11:16 | `0x5000cca269c7a9c1` (HGST) | ST12000NM0127 / ZJV23ALN / `0x5000c500b49ff149` | Amazon `114-5471242-3628258`, $129.99 | Still in pool. |
| 5 | 2025-01-06 00:03 | `0x5000cca269c79da9` (HGST) | ST12000NM0127 / ZJV5DS4J / `0x5000c500b6a075dd` | Amazon `114-5471242-3628258`, $129.99 | |
| 6 | 2025-01-06 17:07 | `0x5000cca269c77d2a` (HGST) | ST12000NM0127 / ZJV2Y4FQ / `0x5000c500b38b622b` | Amazon `114-5471242-3628258`, $129.99 | |
| 7 | 2025-01-07 10:41 | `0x5000cca269c75364` (HGST) | ST12000NM0127 / ZJV5D97Q / `0x5000c500b69a3cd0` | Amazon `114-5471242-3628258`, $129.99 | Still in pool. |
| 8 | 2025-01-07 23:02 | `0x5000cca269c82d05` (HGST) | ST12000NM0127 / unknown / `0x5000c500b088b317` | Amazon `114-5471242-3628258`, $129.99 | Failed within 11 days. Returned to DBSKY, refund issued 2025-01-30. |
| 9 | 2025-01-18 17:59 | `0x5000c500b088b317` | ST12000NM0127 / ZJV2N4GW / `0x5000c500b345abd9` | Amazon `114-7497117-9965059`, $129.99 (inferred) | Only ST12000NM0127 order between the re-disk and this swap; shipped 2025-01-15. Still in pool. |
| 10 | 2025-12-15 14:00 | `0x5000c500b66e2a23` | ST12000NM0127 / ZJV2PCSA / `0x5000c500b3513cb2` | Amazon `111-0080245-7974619`, $179.99 | Delivered 2025-12-15. Still in pool; 24 reallocated sectors as of 2026-11-10, watch it. |
| 11 | 2026-08-16 19:36 | `0x5000c500b30c1db0` / ZJV27W0A | ST12000NM0127 / ZJV4M3HE / `0x5000c500b5bde8e2` | Amazon `114-9912532-0411436` (Renewed), $239.99 (inferred) | FAULTED 2026-11-09 (73 read errors, 8 reallocated). Inferred: the only unused ST12000NM0127 source left after swap 10; order was 2 x "Exos x12 12TB (ST12000NM0127) (Renewed)", $531.58 total, delivered 2026-02-22. Seller name not in the order email. |
| 12 | 2026-08-18 08:29 | `0x5000c500b38b622b` / ZJV2Y4FQ | ST12000NM0127 / ZJV3KB10 / `0x5000c500b43e1c50` | Amazon `114-9912532-0411436` (Renewed), $239.99 (inferred) | Second drive of that pair. Imported as `ata-ST12000NM0127_ZJV3KB10`, see zfs-disk-replacement.md. |
| 13 | 2026-08-20 11:38 | `0x5000c500b6a075dd` / ZJV5DS4J | ST14000NM005G / ZTM09X76 / `0x5000c500dbf44985` | Amazon `111-1354260-2167440` (Renewed), $399.99 | Wrong part: listed as Exos X16 14TB **ST14000NM001G**, drive reports **ST14000NM005G**. Delivered overnight 2026-08-20. Runs as 12TB in this vdev. |
| 14 | 2026-08-28 08:23 | `0x5000c500b2281882` / ZJV1QG9S | ST12000NM001G / ZL203PMM / `0x5000c500c5f18ac7` | eBay goharddrive `19-15026-15078`, $349.95 | Order was 4 x Exos X16 ST12000NM001G, $1,550.28 total, picked up from UPS 2026-08-27 11:23. Model matches listing. FARM 39,759 h vs SMART 1,774 h (2026-11-10). |
| 15 | 2026-11-10 10:12 | `0x5000c500b5bde8e2` / ZJV4M3HE | ST12000NM001G / ZLW0T9XR / `0x5000c500c5d784ee` | eBay goharddrive `19-15026-15078`, $349.95 | Replaces swap 11 after it FAULTED. Slot 1. Added by WWN after a clean scrub (0 errors, 2026-11-10 09:07). At install: SMART 0 h, FARM 40,021 h, 19 power cycles, short self-test passed. ZJV4M3HE kept as evidence for the Amazon claim. |

Rows 1 to 14 were reconstructed on 2026-11-10 from `zpool history data01`, the Prometheus `smartctl_device` series (which tie a serial to a WWN by when each one appeared and disappeared, 30 minute resolution, retained from 2026-05-14), and order and UPS emails. Removed-disk serials before 2026-05-14 are not recoverable.

## FARM vs SMART power-on hours (2026-11-10)

Every disk in the pool arrived with its SMART counters reset. FARM keeps the real lifetime.

| Serial | Model | Source (swap #) | SMART h | FARM h | Hours before reset | Reallocated |
|---|---|---|---|---|---|---|
| ZJV5D97Q | ST12000NM0127 | 7 | 16,093 | 24,090 | ~8,000 | 0 |
| ZJV23ALN | ST12000NM0127 | 4 | 16,141 | 39,711 | ~23,600 | 0 |
| ZJV2N4GW | ST12000NM0127 | 9 | 15,823 | 45,810 | ~30,000 | 0 |
| ZJV2PCSA | ST12000NM0127 | 10 | 7,912 | 60,007 | ~52,100 | 24 |
| ZJV4M3HE | ST12000NM0127 | 11 | 2,051 | 53,201 | ~51,150 | 8 (FAULTED) |
| ZJV3KB10 | ST12000NM0127 | 12 | 2,014 | 55,168 | ~53,150 | 0 |
| ZTM09X76 | ST14000NM005G | 13 | 1,965 | 16,080 | ~14,100 | 0 |
| ZL203PMM | ST12000NM001G | 14 | 1,774 | 39,759 | ~38,000 | 0 |
| ZLW0T9XR | ST12000NM001G | 15 | 0 | 40,021 | ~40,000 | 0 (47.8 TB written before reset) |

## Spares on hand

Read off the drive labels 2026-11-10. All three were ST12000NM001G, PN 2MV103-003, firmware SN03, the same build as ZL203PMM (swap 14), and the remaining 3 of the 4 from the goharddrive order (inferred: model and count match, no other ST12000NM001G purchase). PSIDs are on the labels but deliberately not recorded here: a PSID lets anyone with the drive factory-reset its encryption.

| Model | Serial | WWN | Vendor, order, unit price | FARM h | Notes |
|---|---|---|---|---|---|
| ST12000NM001G | ZLW0RKG6 | `0x5000c500c5d216f1` | eBay goharddrive `19-15026-15078`, $349.95 | 40,021 | Read 2026-11-10 in data01 slot 1 (not added to pool). SMART and device statistics zeroed; FARM shows 40,021 h, 19 power cycles, 47.6 TB written, 41.1 TB read. Identical hours and cycles to ZLW0VNXQ, so likely retired from the same array. 0 reallocated, 0 pending, 0 uncorrectable, 0 CRC lifetime. Short self-test passed. |
| ST12000NM001G | ZLW0VNXQ | `0x5000c500c5da3030` | eBay goharddrive `19-15026-15078`, $349.95 | 40,021 | Read 2026-11-10 in data01 slot 1 (not added to pool). SMART and device statistics zeroed (0 h, 1 power cycle, 0 written); FARM shows 40,021 h, 19 power cycles, 47.7 TB written, 40.3 TB read. 0 reallocated, 0 pending, 0 uncorrectable, 0 CRC lifetime. Short self-test passed. |

## Open claims

- ZJV4M3HE (swap 11): failed at 85 days in service, FARM shows ~51,000 hours before SMART was reset. Claim goes to the Amazon renewed seller on `114-9912532-0411436`, not goharddrive.
- ZTM09X76 (swap 13): wrong part number shipped against Amazon `111-1354260-2167440`.
- goharddrive `19-15026-15078`: a message about ZJV4M3HE was sent to them by mistake on 2026-11-10 and was retracted the same day. All 4 drives are the listed model. On the hidden hours (~38,000 to ~40,000 h per drive by FARM) the seller replied 2026-11-10: sold as refurbished, SMART reset is part of their recert, and they offered either $15 per drive ($60 total) to keep them or a return for a full refund. Keeping the drives and taking the $60, since all four test clean and an 18TB upgrade is not worth it yet. The drives carry goharddrive's 5-year seller warranty from purchase (2026-08), so a failure in one of these goes back to them under warranty.

## Plan

Run the two remaining spares down as the older ST12000NM0127s fail. When the last spare goes into the pool, plan the upgrade (larger drives, or the disk shelf) rather than buying more 12TB used drives.
