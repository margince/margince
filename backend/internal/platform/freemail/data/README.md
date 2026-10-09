<!-- prose:plain -->
# The copied list of free mail domains

`providers.txt` is a byte-for-byte copy of `generate/domains.txt` from
[`goware/emailproviders`](https://github.com/goware/emailproviders), under the MIT license
(© 2015 Pressly Inc.). The license text is copied in `LICENSE-emailproviders.txt`, and it must ship
with every copy of this software that we give out.

Copied at the commit state of 2025-02-08 (`remove xwaretech domains`), 8 758 lines.

## Why this source and not the popular ones

A domain list is only as clean as the chain it came from. The two most used npm packages have a
chain we cannot use in a commercial product:

- **`Kikobeats/free-email-domains`** shows an MIT badge, but its `domains.json` is generated at
  install time from the marketing CSV of HubSpot (`f.hubspotusercontent40.net/hubfs/…/free-domains-2.csv`).
  That is a curated asset of a competitor with no license grant, and the MIT comes from someone who
  never owned the data. The EU database right (Directive 96/9/EC) protects that kind of curated work.
- **`willwhite/freemail`** is ISC, but `data/free.txt` is put together from about 13 GitHub gists
  with no license. Same shape, less serious.

`goware/emailproviders` commits the data file inside the MIT repository itself, and its generator is
a plain text to Go map. So the grant covers what we copy.

## Known bugs in the source

The file is copied as it is and cleaned when it loads (`baseline.go`), so a new sync is a clean
overwrite. These are the bugs the cleaning code handles:

| Line | Content | Handling |
|---|---|---|
| 711 | `atlanticbb.net ` (trailing space) | trimmed |
| 3089 | `housefancom` (no dot) | dropped: cannot be a mail domain |
| 5829-5831 | `müll.email`, `müllemail.com`, `müllmail.com` | folded with IDNA to punycode, which is what a mail header carries |
| 8758 | `zzom.co.uk0-mail.com` | a missing newline glued `zzom.co.uk` and `0-mail.com`; the glued string is harmless (no mail domain equals it), but `0-mail.com` is therefore missing from the dataset and is carried in `pinnedBaseline` instead |

## Re-syncing

```
curl -sSL -o providers.txt https://raw.githubusercontent.com/goware/emailproviders/master/generate/domains.txt
curl -sSL -o LICENSE-emailproviders.txt https://raw.githubusercontent.com/goware/emailproviders/master/LICENSE
```

Then run the package tests. They check that the cleaning code still drops what it must, and that
every domain in `pinnedBaseline` still matches.
