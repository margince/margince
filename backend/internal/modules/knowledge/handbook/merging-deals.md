<!-- prose:plain -->
<!-- describes: frontend/src/features/deals/deal360/dealactions.tsx backend/internal/modules/deals/dealmerge.go backend/internal/modules/deals/dealmergerelink.go -->
# Merging two deals

Two deals of one opportunity can be merged into one. The duplicate is archived and points to the deal that stays.

## How do I merge two deals?
To merge two deals in Margince, open the duplicate and choose **Merge deal** in its **More actions** menu. Pick the deal that stays, check the sentence that names both, and confirm. You need edit rights on both deals, and both must name the same company. A deal with no company has no **Merge deal**.
Also called: merge duplicate deals, combine two deals, fix a double-entered opportunity.

## What moves to the deal that stays?
These move to the deal that stays: offers, contracts, stakeholders, linked activities, lists, tags, shares and documents. So do the leads that were qualified into the duplicate. A stakeholder both deals hold stays once.
A description, priority, acquisition source, commercial motion or partner that the surviving deal leaves empty is filled from the duplicate. Its value is never copied, because the price of the surviving deal is its own.

## What stays on the duplicate?
The stage history and the closing review stay on the duplicate, with its price lines. They describe the path that deal took, and the surviving deal did not take it. Copying them would make the surviving deal look newer in the stage-age report.

## When is a merge refused?
A merge is refused in these cases:
- The deals name different companies. Merge the companies first.
- The duplicate is under a legal hold.
- Both deals hold a live commission, an open legal review, a live chat link or an open Deal Room.

A merged deal cannot be restored, because its offers and contracts now belong to the surviving deal. AI agents cannot merge deals.
