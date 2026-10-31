<!-- prose:plain -->
<!-- describes: frontend/src/features/projects/partners.tsx frontend/src/features/projects/partners.form.tsx frontend/src/features/settings/partnerstages.tsx frontend/src/features/companies/companymarks.tsx backend/internal/modules/contacts/partnerstage.go backend/internal/modules/contacts/partnerstagerule.go backend/internal/modules/contacts/partneractivate.go backend/internal/modules/contacts/leadpartner.go backend/internal/compose/partneractivation.go frontend/src/features/leads/leadpartner.tsx frontend/src/features/projects/partnercommissions.tsx frontend/src/features/projects/partnerdeals.tsx frontend/src/features/projects/commissiondecide.tsx backend/internal/modules/contacts/partner.go backend/internal/modules/contacts/partnerwrite.go backend/internal/modules/commissions -->
# Partners and commission

A **partner** in Margince is a company that brings you business or helps you win it. It may be a hosting company, a consulting company or a large business partner. A partner is a company that is already in Margince, with partner terms added. So the same company page shows both its own business and its partner work. The terms are one or more partner kinds, a certification status, a margin tier and a relationship stage.

Under the company's name, a **Customer** mark and a **Partner** mark say what the company is to you. A company can carry both. The **Partner** mark names the kinds and the stage, for example "Partner · Hosting, Referral · Contract signed".

A deal names the partner behind it. When a partner brought in a deal and the deal is won, the partner earns commission at their margin tier. How a deal itself moves and closes is on [The pipeline: how a deal moves](the-pipeline.md).

## Setting up partners

### How do I make a company a partner?
To make a company a partner in Margince, open the company, choose **More actions** → **Set up partner program**, fill the form and choose **Create**.
1. Open **Companies** in the sidebar and open the company. Add it first if it is not there yet.
2. Choose **More actions** → **Set up partner program**. The **Partner** tab opens on "Not a partner yet" and the **Make this a partner** form.
3. Pick one or more **Partner kinds** (required) and fill what you already know.
4. Choose **Create**.
The company now shows on the **Partners** list and in every deal's **via partner** picker.
Also called: add a partner, onboard a reseller, start a partner programme.

### Who may set up a partner program?
In Margince, anyone who may change the company may set up its partner program and change its terms. The **Margin tier** is the one part that needs more. It decides commission on every later deal, so only a role that may change partners sets it. Without that role the field shows "Your role cannot set the margin tier", and the program starts with no tier.
**Set up partner program** is turned off for a role that cannot see partner programs, with the reason beside it.
Also called: partner permissions, who can add a partner, margin tier rights.

### Can an AI assistant set up a partner?
Yes, in Margince an AI assistant can ask to make a company a partner, with its **Partner kinds**, a **Relationship stage** and a **Next step**. The request waits in the approvals inbox, and the program starts once someone who may change the company approves it.
The assistant cannot choose the **Margin tier**, because it decides commission on every later deal. It stays **Not set** until someone sets it on the company's **Partner** tab.
A partner program ends only when the company is archived, so an assistant cannot set one up on a company that already is a partner.
Also called: let the assistant add a partner, AI partner onboarding.

### How do I change a partner's terms?
To change a partner's terms in Margince, open the company's **Partner** tab and choose **Edit**, change the fields and choose **Save**.
You can change **Partner kinds**, **Certification status**, **Margin tier**, **Relationship stage**, **Next step**, **Next step due** and **Served segments**. The stage list shows the active stages in ladder order, and the stage the partner stands on even when it was retired. A new margin tier counts for commission earned from then on. Commission already earned keeps the tier it was earned at.
Also called: certify a partner, suspend a partner, change a partner's tier.

### How do I see all my partners?
To see every partner in Margince, open **Companies** in the sidebar and choose **Partners** in the list's toolbar.
The **Partners** list shows each partner's **Company**, **Partner kinds**, **Certification status** and **Relationship stage**. Make it shorter with the **Partner kinds**, **Relationship stage**, **Stage meaning** and **Certification status** filters. It has no search box and no fixed order. Click a row to open the company.

The **Companies** list has the **Partner kinds**, **Relationship stage** and **Stage meaning** filters too, for a role that may see partners. **Stage meaning** finds, for example, all signed partners, whatever your stages are called.
Also called: partner list, partner overview, reseller list.

### What do the partner fields mean?
The partner fields in Margince are set on the company's **Partner** tab. The relationship stages come from a list each installation keeps; the other fields are fixed lists.
- **Partner kinds**: one or more of these. **Hosting**: they run it for their clients. **Consulting**: they help clients and bring you in. **Strategic**: a broader alliance. **Referral**: they send you clients. There is no kind for building software for a client.
- **Certification status**: **Applied** (the first value), **Certified**, **Suspended**. It says nothing else about the company.
- **Margin tier**: the share the partner earns on each won deal they sourced. **Intro (15%)**: they only introduced the client. **Active collaboration (20%)**: the deal was worked together. **Partner closed (25%)**: they closed it on their own. **Not set** until a tier is agreed, and it stays **Not set** when the partner's agreement pays differently.
- **Served segments**: who the partner sells to, for example small companies or schools. Free words, with commas between them.

### Partner relationship stages

The relationship stage of a partner in Margince says where the relationship is. It is apart from certification. Each installation keeps its own list of stages. Every stage has one meaning from a fixed list of six, and the filters and rules read the meaning, never the name. A new partner starts on the first active stage that means **Candidate**.

| Stage Margince comes with | Meaning | Where the relationship is |
|---|---|---|
| **Research** | **Candidate** | Still deciding if they are worth a first contact |
| **Identified** | **Candidate** | A real possible partner |
| **Contacted** | **Candidate** | You reached out; no real talk yet |
| **In conversation** | **Candidate** | A real talk is going on |
| **Fit confirmed** | **Candidate** | Both sides agree there is a fit |
| **Contract pending** | **Candidate** | The contract is being worked on |
| **Contract signed** | **Signed** | The contract is signed |
| **Ready** | **Ready** | The partner is set up to work with you |
| **Activated** | **Active** | The partner is activated |
| **Active** | **Active** | The partner relationship is live |
| **Active, referring** | **Active** | Live, and sending business |
| **Dormant** | **Dormant** | Was live, is not active now |
| **No fit** | **Ended** | You looked, and it is a no |

**Next step** and **Next step due** hold the one next thing to do in the relationship. An empty next step on a partner that is not active or ended most often means nobody owns the relationship.

### What does activated mean?
In Margince only an activated partner stands on a stage that means **Active**. A move to such a stage before that is refused, and the **Partner** tab says why.
One of two things activates a partner, and the first one counts:

- Someone accepts the first lead the partner sent. On the lead, pick the partner under **Referred by partner**, then choose **Accept for partner**. Only a role that may change partners sees that button.
- A deal the partner **sourced** is won.

Help on a deal that was already running (**Influenced**) never activates a partner. An activated partner on a stage that means **Candidate**, **Signed** or **Ready** moves to the first active stage that means **Active**.
The **Partner** tab shows the date and the lead or deal behind it. Partners that were on **Active**, **Active, referring** or **Dormant** when the stage list arrived count as activated. Their tab says "Active before the stage list was introduced", because nobody recorded a date.
Also called: activated partner, first partner lead, first partner win.

### How do I change the partner stages?
To change the partner stages in Margince, open **Settings** → **Partners**. An admin can rename, reorder, add and retire stages there, and pick each stage's meaning.
- **New stage** adds a stage at the end of the list, meaning **Candidate**. Change its meaning in its row.
- The meaning of a stage stays fixed while any partner stands on it, archived partners included.
- The switch retires a stage: it leaves the list you pick from, and partners on it keep it.
- A stage that came with Margince is never removed. A stage you added can be removed only while no partner carries it.
- The last active stage meaning **Candidate** cannot be retired, because every new partner starts on one.
Also called: custom partner stage, rename a partner stage, partner pipeline.

### Can I add a partner kind or change the other partner lists?
No. The partner kinds, certification statuses and margin tiers in Margince are fixed lists. Changing one is a change to the product. To record something the fixed fields do not hold, an admin adds a company custom field under **Settings** → **Fields**.
Also called: new partner tier, new partner kind.

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
