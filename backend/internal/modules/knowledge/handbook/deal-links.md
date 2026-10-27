<!-- prose:plain -->
<!-- describes: frontend/src/features/deals/deal360/deallinkscard.tsx frontend/src/lib/recordlinks.queries.ts backend/internal/modules/recordlinks/write.go backend/internal/modules/recordlinks/read.go -->
# Links between records

A link says how two records belong together: a deal that follows up another deal, a renewal of an earlier sale, a
company tied to a deal. This page is for the user who links records and reads the links. A link joins any two of deal,
company and contact.

## Where do I find the links?

On a deal, the **Overview** has a **Links** card. Each row names the other record and the type of the link. The row
says which way the link points: **From this deal** or **To this deal**.

## How do I add or remove one?

| Step | What you do |
|---|---|
| 1 | Choose **Add link**. |
| 2 | Pick a **Link type**. Core has *Follow-up of*, *Renews* and *Related to*. An extension your admin turned on can add more. |
| 3 | Search for the record to link and pick it. |

**Remove** deletes the link. The two records stay as they are.

## What decides what I see and do?

- To add or remove a link, you need the right to change the deal the link starts on. You may link *to* any record you
  can open.
- A link to a record you may not read is left out of the card.
- A link to an archived record is hidden. It comes back when the record is restored.
- Erasing a contact removes every link that names them.

## Can I use links to find deals?

Yes. The deal list has a **Linked as** filter. It keeps the deals that hold a link of that type to another deal. In
Reports you can group deals by **Linked as**. A deal that holds several types falls in one group named after all of them,
and a deal with no link falls in **none**.
