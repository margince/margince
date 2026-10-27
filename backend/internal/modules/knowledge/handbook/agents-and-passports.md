<!-- prose:plain -->
<!-- describes: frontend/src/features/settings/settings-passports.tsx frontend/src/features/settings/settings.aiaccess.tsx frontend/src/features/automations/connect-ai-tool.tsx frontend/src/features/automations/agentallowances.tsx backend/internal/modules/identity/passport.go backend/internal/modules/identity/passportscope.go backend/internal/modules/agents/tierfloor.go backend/internal/platform/agentgrant -->
# Agents, passports and what they may do

Margince is made to be worked in by AI agents as much as by humans. Here you
find what an agent is allowed to do, and what waits for you. You also find what
it is refused outright, and how a passport is created and revoked.

For what the AI makes (drafts, document reads, briefs), see
[What the AI does, and what it does not](what-the-ai-does.md).

There is one rule under everything here:

> An agent can do what the colleague behind it could do alone, and nothing
> more. It is checked against that colleague on every call.

## Connecting and controlling agents

### How do I connect my AI tool to Margince?
To connect your AI tool to Margince, open **Settings → AI access** and answer the two questions on the **Connect your AI tool** card. Copilot on a personal account cannot connect; the card says so. Gemini through work or school needs [an admin to register it first](settings.md#how-do-i-let-gemini-through-work-or-school-connect). If the card says AI tools "cannot connect to Margince yet", an administrator or operations user must turn it on first.
1. Open the account menu, choose **Settings**, then **AI access**.
2. Under **Which AI do you use?**, choose **Claude**, **ChatGPT**, **Microsoft Copilot** or **Gemini**.
3. Under **Whose plan is it?**, choose **My company’s** or **My own**.
4. If the card says "First, your admin.", choose **Download guide for your admin** and send them the file. It holds their steps and your Margince address.
5. If the card shows your Margince address, choose **Copy address** and paste it where the steps say.
6. Follow the numbered steps the card shows for that one case.
7. When Margince opens, sign in. **Authorize access** ticks all five permissions; beside **Send messages** it says "sends messages as you, without asking first". Clear any permissions the tool should not get, then choose **Authorize**.
To refuse, choose **Deny access**.
Authorizing creates the connection, listed under **Your connections**, so you need no passport.
Also called: connect Claude, connect ChatGPT, Copilot, Gemini, AI assistant, connector, MCP.

### What is an agent passport?
An agent passport in Margince is your personal access token: whatever uses it, an AI tool or your own code, acts as you. It carries your own seat, permissions and record visibility, cut down to the **Agent permissions** you tick.
Those are **Read records**, **Draft messages**, **Change records**, **Send messages** and **Buy contact data**. An agent can never do more than you can. You create and revoke passports yourself in **Settings → AI access**, under **For developers**.
Also called: API key, token, agent credential, access token.

### How do I give an agent access to Margince?
To give a script or your own code access to Margince, open **Settings → AI access**, then **For developers**. Choose **New passport** on the **API keys** tab.
1. Open the account menu, choose **Settings**, then **AI access**.
2. Open **For developers**.
3. Choose **New passport**.
4. Fill **Agent name**.
5. Under **Agent permissions**, tick at least one permission.
6. Choose **Mint passport**.
7. Choose **Copy passport** now: "Passport created. It is shown only once, so copy it now."
8. Under **Next, use it**, choose **Download skill** for an AI tool, or **Copy example** for your own code.
A passport lasts 30 days. To use Margince in Claude, ChatGPT, Copilot or Gemini, [connect the tool](#how-do-i-connect-my-ai-tool-to-margince) instead.
Also called: create an API key, script access, integrate an agent.

### How do I use a passport in my AI tool?
To use a passport in an AI tool such as Claude Code, Codex or Antigravity, download the Margince skill. Then save your passport where the tool can find it.
1. Open **Settings → AI access**, then **For developers**.
2. On the **API keys** tab, under **In a script or another AI tool**, choose **Download skill**.
3. Open `margince-skill.zip`. It holds one folder, `margince`.
4. For Claude Code, move the folder to `~/.claude/skills/margince`. For Claude Desktop, add the zip file as a skill in the app's settings.
5. For Codex or Antigravity, follow the `README.md` in the folder. You move the folder, then add one line to your `AGENTS.md` or `GEMINI.md`.
6. Before you start the AI tool, set `MARGINCE_PASSPORT` to your passport, the only place the tool reads it from. The `README.md` shows how, also from your Keychain or 1Password.
Never paste your passport into a chat with the AI tool. The skill tells the tool where to find it. No passport is in the download, so one download works for all your passports.
Also called: Claude skill, add Margince to Claude Code, Codex, Antigravity, AI tool setup.

### How do I call Margince from my own code?
To call Margince from your own code, send your passport with each call.
On the **API keys** tab in **For developers**, under **In your own code**, choose **curl**, **Python** or **JavaScript**, then **Copy example**. It reads the passport from `MARGINCE_PASSPORT`, so set that first.
The `openapi.yaml` file in the skill lists every call a passport can make.
Also called: REST API, API key, script, access token, integration.

### How do I connect a coding agent to Margince?
For Claude Code, Codex or Antigravity, open **Settings → AI access**, then **For developers**, the **Coding agents** tab and your tool.
Do what the tab shows for your tool: run the command, or for Antigravity add the block to the file it names. Then sign in in the browser and choose **Authorize** on **Authorize access**. You need no passport for this.
If the page says AI tools "cannot connect to Margince yet", an administrator or operations user must turn it on first.
Also called: MCP client, MCP server, command line, AI assistant integration.

### How do I revoke an agent's access?
To stop an agent, open **Settings → AI access**. For a passport, open **For developers**, then the **…** menu on its row, and choose **Revoke**: "The passport’s credential is invalidated immediately. The agent loses access on its next call."
For a connected AI tool, open the **…** menu on its row under **Your connections** and choose **Disconnect**, which ends the whole connection. Turning off a user revokes all their passports at once.
Also called: kill switch, remove an agent, disconnect, delete an API key.

### How do I let Margince work overnight for me?
To let Margince prepare your Morning brief overnight, open **Settings → Connections**. On the **Overnight preparation** card, turn on **Let Margince prepare the Morning brief overnight**.
It acts as you and sees only what you can see; it reads and writes but can never send. If it says **Overnight authority expired**, turn it off and on again to renew it.
Also called: overnight agent, scheduled agent, background agent.

## The two tiers

Every action an agent can take has one of two labels. The label applies no
matter how the agent connects.

**auto-execute.** An auto-execute action happens at once, with the agent named
on it in the audit trail. This is the default, and it covers most of what an
agent does. That is looking records up, searching, writing summaries, drafting, and
ordinary field updates. It is also logging an activity, promoting a lead,
archiving a record, and sending an email or a message.

**confirm-first.** A confirm-first action does not happen. What the agent means
to do is written down as an approval, and a human decides.

An agent on your passport does what you could do yourself in the app, without
an approval. For it, only enriching from the web is confirm-first.

### Why sending is not confirm-first by default
A passport carries the seat, permissions and record visibility of the colleague who
created it. So a send an agent can make is one that colleague could already make
alone, in the app. Asking that same colleague to confirm it again would add a
click without adding a check.

What is kept behind a confirmation is narrower: the calls where the passport
holder did not choose where they go.

### What actually waits for a human
**Enriching from the web.** This includes reading a company's site. Here the
AI names the address Margince reads. Someone who talks the AI round could make
it reach an address nobody holding the passport ever picked. That is a question
about where your data goes, so it waits.

The waits below apply only to automatic work with no passport behind it.
**Folding one tag into another.** A merge renames a word on every record
carrying it, and renaming it back does not undo it.

**Closing or reopening a deal.** Moving a deal forward is most often immediate.
But a move with a won or lost stage at *either* end waits. Winning cannot be
put back and touches money. Reopening takes revenue back out of a quarter that has
already been reported. If Margince cannot tell what a stage means, the move
waits.

**Filing an activity under a project.** Relinking is most often immediate. But
filing under a project marks the message as commercial correspondence, and
relinking it away does not remove that. A member can take the filing back with
**Undo filing**; an agent cannot. See
[Capture](capture.md#filing-under-a-project-starts-a-retention-clock).

The **Autonomy tiers** card in **Settings → AI access**, on the **Agent rules** tab under **For developers**, states the same rule.
It reads "Send email, book meetings, update a contact or deal: runs immediately if the
agent has that permission. Granting the permission is the approval." and
"Enrichment from the web: waits in Approvals."

See [Approvals](approvals.md) for what happens to a card once it is staged.

## An agent never has more rights than the colleague behind it

An agent does not stand on its own. It acts *for* a colleague, using a
passport they created, and it is checked against that colleague's
permissions on every call.

So:

- If you cannot see a record, the agent working for you cannot see it either.
  It does not get a "permission denied" that shows the record exists. It
  gets the same nothing you would get.
- If you are not allowed to do something, the agent cannot do it by asking
  again. Approving a staged action needs the same permissions as the action
  itself.
- The passport also carries its own narrower limits (see **Passports** below).
  Your permissions are the most an agent can ever have.

## Things the AI is refused outright

Margince refuses these; they never reach Approvals.

**Approving someone else's proposals.** An agent may approve a proposal only
in these cases:

- An agent never approves a proposal staged for somebody else. If it could, two
  colleagues could each give out a passport. One agent could stage a confirm-first
  action and the other approve it, and nobody would have looked.
- An agent may approve its own proposal only in a narrow case. The change must
  be one that would have applied by itself if a human had not edited the record
  first, and could be put back. That leaves out closing a deal, relinking,
  merging tags, sending, and anything your installation always holds for a
  human.
  Any of the colleague's passports may approve such a proposal. That includes
  the one that proposed it, and a connected agent.
- Anything else an agent proposed is approved by the colleague in the app, or
  through a passport they created by hand. Neither the passport that proposed
  it nor a connected agent may approve it, because nobody had to be there for
  either.

So an agent may answer a card staged for the colleague it acts for, its own
proposal included, within the rules above. That is what the colleague could
have answered in the app, and could put back later. A scheduled agent
never answers a card at all, because nobody is watching it to have said yes.

**Raising its own allowance.** An agent can never answer a request to raise its
own allowance, either way. A passport that could raise its own limit would have
no limit.

An agent *may* reject its own proposal. Rejecting drops the proposal and cannot
widen anything. So an agent can take its own request back off your list.

**Consent and privacy decisions.** Granting consent or taking it back, and
carrying out or rejecting a privacy request, are for humans only.

**Asking a document set.** Asking a set, and setting up or changing one, are a
human's work. An answer that rests on passages is only good if the
reader can open the passage under each sentence and disagree with it. An agent
acting on that answer with nobody watching cannot do that. This is why the
question box works over a named set of documents rather than over everything.

**Changing the pipeline or its stages.** The stage ladder is what the "advance
a deal" approval is judged against. An agent that could edit the ladder could
change the meaning of the approval it was asking for.

**Custom fields, webhooks, saved views and sharing.** These are settings for
members, and an agent can neither read nor change them. It can still read which
custom fields exist, so it can search on them.

For these and for consent, document sets and pipeline settings, a passport
cannot sign in at all. Approvals work in another way: the passport is let in,
and the check happens on the card itself.

**New actions that change data.** Agents cannot use a new action that changes
data until it is given a label. A missing label means the agent cannot do it,
never that it can do it with nobody watching.

This covers changes only. A new way to *read* data is open to an agent as soon
as it exists. Reading is still limited by the passport's own permissions and
record visibility, the same limit the colleague behind it has.

## What protects a send

Agent sending is not held behind a confirmation by default. So it helps to know
what *does* stand in the way. There are four things, and none of them is a
click.

**Consent, refused by default, per purpose.** A send is refused unless an
active consent, with its evidence, exists for the *purpose* that send falls under. Consent
for a different purpose does not count. "Unknown" blocks, and so does "Withdrawn".
Where a purpose requires double opt-in, consent that was granted but not yet
confirmed does not send. Every recipient is checked, including anyone copied in.

**Your seat, checked again at sending.** It is checked when the message goes
out, not only when the message is staged. Someone moved to a read-only seat
between writing a message and its going out is refused, no matter who staged it.
The message stays held and is not tried again, because the lower seat still
applies.

**The passport's permissions.** Sending needs **Send messages**, which covers
four actions: sending an email, sending one to an account, sending a message,
and booking a meeting. A passport without it cannot do any of them, no matter
the tier. **Change records** does not include **Send messages**. Each
permission must be ticked on its own.

**A fixed limit on calls that go out.** Each passport gets an allowance of
calls that leave Margince in each window, 20 by default. The allowance for
changes can be widened by approving a card; this one cannot. It ends when
the window ends.

## An agent edit applies at once

When an agent updates a record, every field in the update is saved in the same
call, including one you typed. Nothing waits for approval. Each change is in the
record's history with who made it, and **Handled for you** and the record
history offer Undo.

What still waits is the work that always waits for another reason, such as
enrichment and the confirm-first actions above.

## Passports: how an agent is connected

A **passport** is the credential that ties one agent to one colleague. You
create it yourself in **Settings → AI access**, under **For developers**, and you
can revoke it yourself. Revoking cuts off that one agent. The **Passports** card
says: "A passport is your personal credential for scripts and your own code.
Whatever uses it acts as you, with the permissions you tick, and never more."

A passport carries narrower permissions than the colleague has. Each one stands
alone: ticking one never includes another. The app names them **Agent
permissions**:

- **Read records**: reads only. The only permission a read-only seat may use at
  all.
- **Draft messages**: proposes text. This is not read-only: one drafting action
  saves a draft on the deal's timeline.
- **Change records**: every change that stays inside your company.
- **Send messages**: the four actions that send something out of Margince,
  booking a meeting included.
- **Buy contact data**: the one action that gets data from an outside provider.

A passport with only **Read records** can read your approvals but cannot decide
them. A passport needs **Change records** to approve most things, and **Send
messages** too where approving sends a message.

A passport lasts 30 days by default, at least an hour and at most 90 days. A
passport created in Settings always gets the 30-day default. The credential is
shown once and never again: "Passport created. It is shown only once, so copy it now."
Margince keeps no copy it can read, so nobody, including an administrator, can
get it back for you.

A passport does the same through the REST API as through MCP. It sees the same
records, filters, permissions and approvals; an MCP page holds 50 rows. Some
reads are for humans only either way: installation, capture and integration
settings, the digest, backfill progress, rates, AI prices, access and consent.

Revoking takes effect at the agent's next call. So does moving the colleague
behind it to a lower role. Margince checks the colleague's rights on every call,
so a change applies at once rather than at the next sign-in.

### Letting an agent work overnight for you

A scheduled agent runs while you are away, so it cannot take your rights from
a session you are not in. You have to give them in advance. In
the app this is the **Overnight preparation** card in **Settings →
Connections**, with the switch **Let Margince prepare the Morning brief
overnight**. You were asked the same question once when you first joined.

Turning it on creates your own passport in the same step. The overnight run
then carries a passport that is yours. So everything it does is limited to
what you could have done yourself, and is recorded under your name. The card
says it "cannot send: the permission given here covers reading and writing
only, never sending." Turning it off revokes that passport, so the authority
ends.

A passport expires after 30 days, so a grant you agreed to can stop working.
The card then shows **Overnight authority expired**: "Turn this off and on again
to renew it. Until then, your Morning brief is not prepared." Margince may now do
more than when you agreed. Then it shows **Authority no longer covers
the work**, and you do the same thing.

Nobody can do this for you, and no agent can do it for itself. An agent
that could grant itself lasting authority would be deciding its own rights.

### Allowances per window

Each passport gets an allowance per window: changes made, calls that go out,
and total calls. Every call counts once, a read too. A long list is still one
call. By default the window is 24 hours, and the allowances are 200
changes, 20 calls that go out and 1,000 calls in total. An administrator
changes them under **Settings › AI usage › Agent passport allowances**: press
**Edit allowances**, change the values, and press **Save**. One more count shows
how many AI tokens the agent used; it is only for you to see, and limits
nothing.

A connected app's passports share one allowance: a renewed passport goes on
counting where the last one stopped.

The allowances work differently when they run out. For changes, the agent is
refused, and a card goes to whoever approved the connection. Their approval
widens the window by one more allowance. Calls that go out and total calls are
fixed stops. No approval raises them, and only the end of the window clears
them. An agent that only reads can still reach the total and stop.

The window is fixed, not rolling, so every allowance starts over at the same
moment. A changed allowance applies within a minute, in the window that is
running. Raise one, and a stopped agent can go on. Lower one, and an agent
already past it is refused on its next call. A new window size waits for the
next reset, so no agent starts over early; the card says when it starts.

In **Settings → AI access**, each row under **Your connections** and **Passports**
shows the allowance closest to running out. Open a row to see all three. Revoked passports are hidden; the button under the list shows them.
**AI usage › Agent usage by user** shows how much each user's agents used, never
which passport or what it read. Administrators, Management and operations users see it.

Only whoever approved the connection can answer a request to widen it. An
administrator cannot, and neither can the owner of the company. An agent's
limit is that colleague's own authority.

## Files and agents

### Can an agent put a file on a record?
Yes. A connected agent whose connection has **Change records** ticked can put a file on a company, contact, deal, lead, project or activity. The file shows with the record's files, as if you had uploaded it. This works the same whether the agent connects through Claude or uses its passport with the Margince API.
- It is done at once, and never waits for an approval.
- The agent can also file it against one of the account's contracts, as you can when you upload.
- A file can be up to about 6.2 MB, or your installation's upload limit if that is smaller.
- The same kinds of file are taken as in the app; see [Which kinds of file can I upload?](documents-and-files.md#which-kinds-of-file-can-i-upload).
An agent can also list the files on a record, but it cannot download a file or read what is in it. A scheduled agent never puts a file on a record.
Also called: agent attach file, upload a file from Claude, add a PDF through an agent.

## What is not cleaned: attachments

Everything an agent sends *to the AI model* is cleaned of secrets first. API
keys, tokens, private keys and passwords are removed, and a marker stands in
their place.

This applies only to what goes to the model. An email an agent sends to a
customer is not cleaned.

The cleaning removes secrets; it does not protect personal data. Names, email
addresses and phone numbers pass through. It also reads only text, so a secret
*inside an attached document* is not something it can find.

Attaching a document is a decision to send it as it is. Act on it that way.
