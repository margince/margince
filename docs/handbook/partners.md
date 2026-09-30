# Partners and commission

A **partner** in Margince is a company that brings you business or helps you win
it — a hosting firm, a consultancy, an alliance. A partner is not a separate
record: it is extra partner terms on a company that already exists, so the same
company page shows both its own business and its partner life. The terms are a
role, a certification status, a margin tier and a relationship stage.

A deal names the partner behind it, and a won deal a partner sourced earns them
commission at their margin tier. How a deal itself moves and closes is on
[The pipeline: how a deal moves](the-pipeline.md).

## Setting up partners

### How do I make a company a partner?
To make a company a partner in Margince, open the company, choose **More actions** → **Set up partner program**, fill the form and choose **Create**.
1. Open **Companies** in the sidebar and open the company. Add it first if it is not there yet.
2. Choose **More actions** → **Set up partner program**. The **Partner** tab opens on "Not a partner yet" and the **Make this a partner** form.
3. Pick a **Partner role** (required) and fill what you already know.
4. Choose **Create**.
The company now appears on the **Partners** list and in every deal's **via partner** picker. Also called: add a partner, onboard a reseller, start a partner programme.

### How do I change a partner's terms?
To change a partner's terms in Margince, open the company's **Partner** tab and choose **Edit**, change the fields and choose **Save**.
You can change **Partner role**, **Certification status**, **Margin tier**, **Relationship stage**, **Next step**, **Next step due** and **Served segments**. A new margin tier applies to commission earned from then on; commission already earned keeps the tier it was earned at. Also called: certify a partner, suspend a partner, change a partner's tier.

### How do I see all my partners?
To see every partner in Margince, open **Companies** in the sidebar and choose **Partners** in the list's toolbar.
The **Partners** list shows each partner's **Company**, **Partner role**, **Certification status** and **Relationship stage**. Narrow it with the **Partner role** and **Certification status** filters. It has no search box and no fixed order. Click a row to open the company. Also called: partner list, partner overview, reseller list.

### What do the partner fields mean?
The partner fields in Margince are fixed lists, set on the company's **Partner** tab.
- **Partner role**: **Hosting** (they run the software for their clients), **Consulting** (they advise clients and bring you in), **Strategic** (a broader alliance).
- **Certification status**: **Applied** (the starting value), **Certified**, **Suspended**. It says nothing else about the company.
- **Margin tier**: **Intro (15%)**, **Active collaboration (20%)**, **Partner closed (25%)**, or **Not set** until a tier is agreed.
- **Served segments**: free words, comma-separated.

### Partner relationship stages

The relationship stage of a partner in Margince says where the relationship is,
separately from certification. A new partner starts at **Research**.

| Stage | Where the relationship is |
|---|---|
| **Research** | Still deciding whether they are worth approaching |
| **Identified** | A real prospect |
| **Contacted** | You reached out; no real conversation yet |
| **In conversation** | A dialogue is running |
| **Fit confirmed** | Both sides agree there is a fit |
| **Contract pending** | The paperwork is in motion |
| **Active** | The partnership is live |
| **Active, referring** | Live, and sending business |
| **Dormant** | Was live, has gone quiet |
| **No fit** | You looked, and it is a no |

**Next step** and **Next step due** hold the one thing that moves the
relationship forward. An empty next step on a partner that is neither Active nor
No fit usually means nobody owns the relationship.

### Can I add a partner role or change the partner lists?
No. The partner roles, certification statuses, margin tiers and relationship stages in Margince are fixed lists with no settings page; changing one is a product change. To record something the fixed fields do not hold, an administrator adds a company custom field under **Settings** → **Fields**. Also called: new partner tier, custom partner stage.

## Partners on a deal

### How do I record that a partner brought a deal?
To credit a partner on a deal in Margince, set **via partner** to the partner and choose the **Partner attribution** on the deal's form or in its **Details** panel.
1. On **New deal**, or in an open deal's **Details**, pick the partner under **via partner**.
2. Under **Partner attribution**, choose **Sourced the deal (earns commission)** or **Influenced an existing deal (no commission)**.
Left unchosen, it reads **Not set (counted as sourced)**. The two fields appear only once at least one company is a partner. Also called: partner-sourced deal, referral, partner influenced.

A partner's own **Partner** tab lists **Partner deals**: the deals of other
companies that came through this partner, with the **Customer**, the
**Attribution**, the **Deal value** and the **Status**. The partner's own Deals
tab does not show them, because each of those deals belongs to its customer. On
**Deals**, the **Partner-sourced** and **Partner** filters narrow the list to
deals through any partner, or through one.

## Commission

Commission in Margince accrues on its own when a deal a partner **sourced** is
won; nobody creates it by hand. An influenced deal earns nothing.

The partner's margin tier is frozen onto the commission at the moment it accrues,
so changing a partner's tier later does not rewrite what they have already
earned. A partner with no tier earns nothing, and no commission row is written at
all — which is different from writing a zero. "We owe them nothing" and "we owe
them nothing yet" are not the same claim, and a zero row would blur them.

If a partner earned commission on a win, reopening the deal does not delete it. A
reversal row is added and the original is marked **Reversed**. A deal that was
won, reopened and won again shows three rows. Nothing is rewritten.

### How do I approve or pay a partner's commission?
To approve or mark a partner's commission paid in Margince, open the partner company's **Partner** tab and use the row's action in its **Commission** panel.
1. **Approve** an **Accrued** row: "It pays nothing: settle the payment in your finance system, then mark it paid here."
2. **Mark as paid** an **Approved** row once your finance system has paid it. Margince moves no money.
3. **Reverse** a row that should not stand, with a **Reversal reason**. An offsetting row is added; nothing is deleted.
Without permission the column reads **No permission to decide**. Also called: pay a partner, partner payout, referral fee.

### The commission ledger

The **Commission** panel on a partner's page carries a row per deal: the
**Deal**, what was **Earned**, the **Rate**, the **Deal value** it was taken
from, and a **Status**. Above it sits **Outstanding**, accrued or approved.
Nothing earned yet reads "No commission yet".

| Status | What it means |
|---|---|
| **Accrued** | Earned, nobody has approved it |
| **Approved** | Approved for payment |
| **Paid** | Paid |
| **Reversed** | Undone, with the original left standing |
