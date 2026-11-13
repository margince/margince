<!-- prose:plain -->
# The copied Windows zone mapping

`windowsZones.xml` is a byte-for-byte copy of `common/supplemental/windowsZones.xml`, and
`timezone.xml` of `common/bcp47/timezone.xml`, from [Unicode CLDR](https://github.com/unicode-org/cldr)
at the tag `release-48`. Both are under the Unicode License version 3 (© Unicode, Inc.). The license text is
copied in `LICENSE-cldr.txt`, and it must ship with every copy of this software that we give out.

`windowszone.go` reads them to send an Outlook event in the Windows zone that matches the zone the
host picked. `windowsZones.xml` names the Windows zone for each IANA zone. `timezone.xml` adds the
other names of each IANA zone, because `windowsZones.xml` uses only one of them.

## Zones a later release moved

`moved-windows-zones.txt` is made from these files, not copied. It lists the zones that a later
CLDR release maps to some other Windows zone. A zone moves when its rules change, such as a
switch to summer time all year, and Windows then names it a new zone. The old Windows zone states
the old rules, so an Outlook event in that zone would show the wrong local time. Microsoft may not
serve the new Windows zone yet, so Margince sends these zones in UTC: the time is right under
current rules. It was made against `release-49`, and lists only zones the copied files map.

To make it again, download the newest release's `windowsZones.xml` outside the repository, then run
this from `backend/`:

```
go test ./internal/modules/capture/graphcal/ -run TestMovedWindowsZonesAreCLDRZones -cldr-newer /path/to/windowsZones.xml
```

Do this on each new CLDR release, and when you re-sync the copied files below. After a re-sync,
make the list again against a release newer than the copy, or empty it when none exists yet.

## Re-syncing

```
curl -sSL -o windowsZones.xml https://raw.githubusercontent.com/unicode-org/cldr/release-48/common/supplemental/windowsZones.xml
curl -sSL -o timezone.xml https://raw.githubusercontent.com/unicode-org/cldr/release-48/common/bcp47/timezone.xml
curl -sSL -o LICENSE-cldr.txt https://raw.githubusercontent.com/unicode-org/cldr/release-48/LICENSE
```

Change `release-48` to the new tag in all three commands and here. Use a final release tag, not
`main`: `main` can name a Windows zone that Microsoft does not serve yet. Then run the package tests.
