<!-- prose:plain -->
# Partners and commission

A **partner** in Margince is a company that brings you business or helps you win it. It may be a hosting company, a consulting company or a large business partner. A partner is a company that is already in Margince, with partner terms added. So the same company page shows both its own business and its partner work. The terms are a role, a certification status, a margin tier and a relationship stage.

A deal names the partner behind it. When a partner brought in a deal and the deal is won, the partner earns commission at their margin tier. How a deal itself moves and closes is on [The pipeline: how a deal moves](the-pipeline.md).

## Setting up partners

### How do I make a company a partner?
To make a company a partner in Margince, open the company, choose **More actions** → **Set up partner program**, fill the form and choose **Create**.
1. Open **Companies** in the sidebar and open the company. Add it first if it is not there yet.
2. Choose **More actions** → **Set up partner program**. The **Partner** tab opens on "Not a partner yet" and the **Make this a partner** form.
3. Pick a **Partner role** (required) and fill what you already know.
4. Choose **Create**.
The company now shows on the **Partners** list and in every deal's **via partner** picker.
Also called: add a partner, onboard a reseller, start a partner programme.

### How do I change a partner's terms?
To change a partner's terms in Margince, open the company's **Partner** tab and choose **Edit**, change the fields and choose **Save**.
You can change **Partner role**, **Certification status**, **Margin tier**, **Relationship stage**, **Next step**, **Next step due** and **Served segments**. A new margin tier counts for commission earned from then on. Commission already earned keeps the tier it was earned at.
Also called: certify a partner, suspend a partner, change a partner's tier.

### How do I see all my partners?
To see every partner in Margince, open **Companies** in the sidebar and choose **Partners** in the list's toolbar.
The **Partners** list shows each partner's **Company**, **Partner role**, **Certification status** and **Relationship stage**. Make it shorter with the **Partner role** and **Certification status** filters. It has no search box and no fixed order. Click a row to open the company.
Also called: partner list, partner overview, reseller list.

### What do the partner fields mean?
The partner fields in Margince are fixed lists, set on the company's **Partner** tab.
- **Partner role**: **Hosting** (they run it for their clients), **Consulting** (they help clients and bring you in), **Strategic** (a broader alliance).
- **Certification status**: **Applied** (the first value), **Certified**, **Suspended**. It says nothing else about the company.
- **Margin tier**: **Intro (15%)**, **Active collaboration (20%)**, **Partner closed (25%)**, or **Not set** until a tier is agreed.
- **Served segments**: free words, with commas between them.

### Partner relationship stages

The relationship stage of a partner in Margince says where the relationship is. It is apart from certification. A new partner starts at **Research**.

| Stage | Where the relationship is |
|---|---|
| **Research** | Still deciding if they are worth a first contact |
| **Identified** | A real possible partner |
| **Contacted** | You reached out; no real talk yet |
| **In conversation** | A real talk is going on |
| **Fit confirmed** | Both sides agree there is a fit |
| **Contract pending** | The contract is being worked on |
| **Active** | The partner relationship is live |
| **Active, referring** | Live, and sending business |
| **Dormant** | Was live, is not active now |
| **No fit** | You looked, and it is a no |

**Next step** and **Next step due** hold the one next thing to do in the relationship. An empty next step on a partner that is not Active or No fit most often means nobody owns the relationship.

### Can I add a partner role or change the partner lists?
No. The partner roles, certification statuses, margin tiers and relationship stages in Margince are fixed lists with no settings page. Changing one is a change to the product. To record something the fixed fields do not hold, an admin adds a company custom field under **Settings** → **Fields**.
Also called: new partner tier, custom partner stage.

## Partners on a deal

### How do I record that a partner brought a deal?
To give a partner credit for a deal in Margince, set **via partner** to the partner and choose the **Partner attribution**. Both are on the deal's form and in its **Details** panel.
1. On **New deal**, or in an open deal's **Details**, pick the partner under **via partner**.
2. Under **Partner attribution**, choose **Sourced the deal (earns commission)** or **Influenced an existing deal (no commission)**.
If you leave it empty, it reads **Not set (counted as sourced)**. The two fields show only once at least one company is a partner.
Also called: partner-sourced deal, referral, partner influenced.

A partner's own **Partner** tab lists **Partner deals**: the deals of other companies that came through this partner, sourced or influenced. Each shows the **Customer**, the **Attribution**, the **Deal value** and the **Status**. The partner's own Deals tab does not show them, because each of those deals belongs to its customer. On **Deals**, the **Partner-sourced** and **Partner** filters cut the list down to deals through any partner, or through one.

## Commission

Commission in Margince is added on its own when a deal that a partner **sourced** is won; nobody creates it by hand. A deal the partner only influenced earns nothing.

The partner's margin tier is fixed on the commission when it is added. So changing a partner's tier later does not change what they have already earned. A partner with no tier earns nothing, and no commission line shows at all. A zero would say "we owe them nothing"; no line says "we owe them nothing yet".

If a partner earned commission on a win, opening the deal again does not delete it. A line that takes it back is added, and the first line is marked **Reversed**. A deal that was won, opened again and won again shows three lines. Nothing is written over.

### How do I approve or pay a partner's commission?
To approve or mark a partner's commission paid in Margince, open the partner company's **Partner** tab and use the line's action in its **Commission** panel.
1. **Approve** an **Accrued** line: "It pays nothing: settle the payment in your finance system, then mark it paid here."
2. **Mark as paid** an **Approved** line once your finance system has paid it. Margince moves no money.
3. **Reverse** a line that should not stand, with a **Reversal reason**. A line that takes it back is added; nothing is deleted.
Without permission the column reads **No permission to decide**.
Also called: pay a partner, partner payout, referral fee.

### The Commission panel

The **Commission** panel on a partner's page holds a line for each deal. Each line shows the **Deal**, what was **Earned**, the **Rate**, the **Deal value** it was taken from, and a **Status**. Above it sits **Outstanding**: lines that are accrued or approved. Nothing earned yet reads "No commission yet".

| Status | What it means |
|---|---|
| **Accrued** | Earned, nobody has approved it |
| **Approved** | Approved for payment |
| **Paid** | Paid |
| **Reversed** | Taken back, with the first line left in place |
