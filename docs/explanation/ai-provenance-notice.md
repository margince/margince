<!-- prose:plain -->
# The AI provenance notice

The sentence a draft from a model holds to say that a model wrote it. It sits in one place,
`draftfloor.AIProvenanceNotice`, and this page says why it has the shape it has.

This notice does not meet a disclosure duty under EU AI Act Art. 50, and no comment or field description
should claim it does. A reader who trusts the claim stops looking. That reader may be an engineer, or a
compliance lead at a customer who reads the published contract. They would think a legal duty is
handled.

## What it is

It is a provenance signal inside the product. It tells the rep that these words are from a model, so
they read and edit before sending.

The notice asks the rep to review the draft. That review is what keeps an Art. 50(4) duty from applying,
and it only works if the rep reads the sentence. So there is one spelling of it, held by
`TestTheAIProvenanceNoticeHasOneSpelling`. A rep shown a different sentence by each surface learns to
skip past all of them.

## Why no duty attaches

Art. 50(4), second part, binds text `published with the purpose of informing the public on matters of public interest`.
A sales email to one contact is not published and not a matter of public interest. So the duty
in that part does not reach it. There is nothing to meet.

Art. 50(2), marking that a machine can read, is a different duty for a different party. It means a
watermark and signed metadata, and it binds the **provider** of the AI system, which is the model
vendor. It binds neither Margince nor our customers. A sentence for humans in a JSON field is not marking
that a machine can read, under any reading. So the `ai_disclosure` field descriptions do not claim to be.

## Why the notice matters

Where an Art. 50(4) duty *would* reach the text, it drops away in one case. That case is when the content
the AI made has been through human review or editorial control. A human or a legal entity must also hold the
editorial duty for it. (This is our own short version; read the law's own words in full before you
depend on this.)

The notice supports that exception. The composer that needs a confirm first (a draft a rep reads, edits
and presses send on) clears the bar. The guidance from the Commission needs a close look by someone able to
approve, change or reject the content. A light check does not count.

A path that sends with no review would not clear that bar. One example is a model that drafts and sends
in one step. Another is an automation that sends outbound mail with nobody in the loop. What this page
says does not hold for such a path, so read this page again before you build one.

## Who sees it

Do not take one surface as the rule for all; they differ, and that matters.

| Surface | Where the notice goes | Who reads it |
|---|---|---|
| A reply draft in the composer, account, contact and lead drafts | The `ai_disclosure` field, shown in the draft band | The rep only |
| Offer made again | The `ai_disclosure` field, shown in the offer banner | The rep only |
| The path for an intro draft (`renderIntroDraft`) | Put **into** `draft_body` | The rep, and the receiver if the body is sent with no change |

The first two never add it to the body that is sent. The send payload holds subject, body, receivers and
attached files, and the notice is not part of the body. The third does put it in the body. So a claim for all paths like
"a receiver never sees it" is false, and no text makes it.

## What is still open

The wording itself. All three languages still call the sentence an Art. 50 disclosure:
`Offenlegung nach Art. 50`, `công bố theo Điều 50`, `EU AI Act Art. 50 disclosure`. That is the claim
this page is here to correct. It is copy users see in three languages, and a question that turns on the
law in Germany. So it is a product decision, not a rename:
[issue 5920](https://github.com/margince/margince/issues/5920).

## Standing note

This is research done by reading these sources:

- the text of the law;
- the Art. 50 FAQ from the Commission;
- the February 2026 guidance from the Wettbewerbszentrale;
- one outside legal review.

It is not legal advice, and the one who gives us AI Act advice has not confirmed it. Confirm it before
you depend on any of it in a statement to customers.

The name `AIProvenanceNotice` holds under every reading we looked at.
