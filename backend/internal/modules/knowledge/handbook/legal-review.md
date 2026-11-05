<!-- prose:plain -->
<!-- describes: frontend/src/features/privacy/legalreviews.tsx frontend/src/features/privacy/legalreviewpack.tsx frontend/src/features/settings/legalreviewsettings.tsx frontend/src/components/legalreviewstatus.tsx backend/internal/modules/deals/legalreview.go backend/internal/modules/deals/legalreviewdecide.go backend/internal/modules/deals/legalreviewgate.go backend/internal/modules/deals/handlers_legalreview.go -->
# Legal review

A **legal review** in Margince is a request from a deal's side to your legal team: read this deal
before it moves on. Margince builds the summary legal reads from the deal itself, so nobody has
to write one.

Legal review is on unless an administrator switched it off. Where it is off, none of what this page
describes is shown.

### How do I ask legal to review a deal?
To ask for a legal review in Margince, open the deal and click **Request legal review**.
1. Open **Deals** in the sidebar and open the deal.
2. Click **Request legal review** in the deal's menu, or choose the **Legal review** tab and click it there.
3. Write what legal should look at in **Note for legal**, if you want to.
4. Click **Send request**.
You need to be able to change the deal. A deal waits for one answer at a time, so the button is not
shown while a request is open.
Also called: legal sign-off, contract review, send to legal.

### What does legal see?
Legal sees the deal as it was when you asked. The **Review pack** shows the company, value, stage,
close date, current offer, stage history, and the contract, legal and offer documents.
For each offer line it shows how often the buyer pays and for how many periods.
It also lists any other offer the buyer was sent or accepted.

It keeps those facts, so a later edit to the deal does not change what legal answered on.
The names of the deal, its company and its owner are always shown as they are today.
A reviewer opens a file only if their own access lets them. A file that came with an email
follows who may read that email; anyone else does not see it in the summary at all.

### What answer can legal give?
Legal answers **Approve** or **Changes needed**, with comments. The deal's **Legal review** tab
shows the answer, its comments and who answered.
After **Changes needed**, make the changes and click **Request again**. The new request links the
earlier one, so legal reads both answers side by side.
The colleague who asked, and the deal's owner, cannot answer the request.

### Can a stage require legal's approval?
Yes. An administrator opens the account menu → **Settings** → **Pipelines** and adds a condition
of the kind **Legal approved** to a stage, under its **Exit criteria**. A deal then cannot move past that stage until legal approves it.
When you try, the deal stays where it is and Margince says what is missing.
- Closing a deal as lost is always allowed, and so is moving it back onto or before that stage.
- Reopening a lost or won deal onto a later stage needs the approval too.
- A new request replaces the earlier approval, until legal answers again.
- If the deal's value, or any part of its current offer (text, end date, lines, totals), changes after
  the approval, ask legal again. The same holds when the buyer is sent a second offer.
- A stage that needs legal's approval also holds deals already past it, for example after an
  administrator adds the condition or moves the stage.
- A new deal cannot start on a stage after one that needs legal's approval.
- Only an administrator can make that condition optional, archive it or its stage, or move stages
  in that pipeline.

### Who answers legal reviews?
A colleague holding the `legal_review` permission answers them. An administrator gives it in a
role, under account menu → **Settings** → **Roles**. The admin role has it from the start; no other role does.
A reviewer sees the deal's facts, money and offer only as far as their own role reads deals and offers.
A reviewer opens the queue from **Deals**, with the **Legal reviews** button. The largest deal value
comes first, then the earliest close date.

### How do I switch legal review off?
An administrator opens the account menu → **Settings** → **Pipelines** and turns off **Legal review
on deals**.
Off hides the request, the queue and the **Legal approved** kind, and a stage stops waiting for
legal. Every review already recorded is kept, and comes back when legal review is on again.
