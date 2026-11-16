<!-- prose:plain -->
# The copied CLDR zone list

`timezone.xml` is a byte-for-byte copy of `common/bcp47/timezone.xml` from
[Unicode CLDR](https://github.com/unicode-org/cldr), under the Unicode License version 3
(© Unicode, Inc.), whose text is in `LICENSE-cldr.txt`. It ships with every copy of this software.

Two readers use it. `cldrzone.CountryZone` finds the one zone of a country, so the consent module
can say when a mailed link stops working on the recipient's own clock. The Outlook calendar code in
`backend/internal/modules/capture/graphcal` reads the other names of each zone. That package's
`data/README.md` says how to re-sync this file together with its own copy of `windowsZones.xml`.
