# The AI provenance notice

The sentence a model-written draft carries to say a model wrote it. It lives in
one place, `draftfloor.AIProvenanceNotice`, and this page is why it is shaped the
way it is.

This notice does not discharge an EU AI Act Art. 50 disclosure duty, and no
comment or field description should claim it does. A reader who believes the
claim (an engineer, or a customer's compliance officer reading the published
contract) concludes a legal obligation is handled and stops looking.

## What it is

It is an internal provenance signal. It tells the rep that these words came from
a model, so they read and edit before sending.

The notice prompts the rep to review the draft. That review is what keeps an
Art. 50(4) duty from applying, and it only works if the rep reads the sentence.
So there is one spelling of it, held by `TestTheAIProvenanceNoticeHasOneSpelling`. A rep shown a different
sentence by each surface learns to skim past all of them.

## Why no duty attaches

Art. 50(4) subparagraph 2 binds text "published with the purpose of informing
the public on matters of public interest". A sales email to one contact is
neither published nor public-interest, so the obligation in that subparagraph
does not reach it. There is nothing to discharge.

Art. 50(2), machine-readable marking, is a different obligation with a
different addressee. It means watermarking and signed metadata, and it binds the
**provider** of the AI system, which is the model vendor. It binds neither
Margince nor our customers. A human-readable sentence in a JSON field is not machine-readable
marking under any reading, so the `ai_disclosure` field descriptions do not claim
to be.

## Why the notice matters

Where an Art. 50(4) obligation *would* reach the text, it drops away where the
AI-generated content has undergone a process of human review or editorial
control and editorial responsibility is held by a natural or legal entity.
(Paraphrased; the regulation's own wording is worth reading in full before
relying on this.)

The notice supports that exemption. The confirm-first composer (a draft a rep
reads, edits and presses send on) clears the bar, because the
Commission's guidance requires careful examination by someone competent to
approve, alter or reject the substance. Superficial checks do not qualify.

A send-without-review path would not clear that bar. Examples are a model that
drafts and sends in one step, or an automation that fires outbound mail with
nobody in the loop. The analysis on this page does not hold for such a path, so
re-read this page before building one.

## Who sees it

Do not generalise from one surface; they differ, and the difference matters.

| Surface | Where the notice goes | Who reads it |
|---|---|---|
| Composer reply draft, account/contact/lead drafts | The `ai_disclosure` field, rendered in the draft band | The rep only |
| Offer regenerate | The `ai_disclosure` field, rendered in the offer banner | The rep only |
| Warm-intro path (`renderIntroDraft`) | Formatted **into** `draft_body` | The rep, and the recipient if the body is sent unchanged |

The first two never append it to an outgoing body: the send payload carries
subject, body, recipients and attachments, and the notice is not part of the
body. The third does embed it, which is why the blanket claim "a recipient never
sees it" is false and is not made anywhere.

## What is still open

The wording itself. All three translations still call the sentence an Art. 50
disclosure (`Offenlegung nach Art. 50`, `công bố theo Điều 50`, `EU AI Act
Art. 50 disclosure`), which is the claim this page exists to correct. It is
user-visible copy in three languages and a German-market question, so it is a
product decision rather than a rename: margince#5920.

## Standing caveat

This is desk research against the regulation text, the Commission's Art. 50 FAQ,
the Wettbewerbszentrale's February 2026 guidance and one law-firm analysis. It
is not legal advice and has not been confirmed by whoever gives us AI Act advice.
Confirm it before relying on any of it in a customer-facing statement.

The name `AIProvenanceNotice` holds under every reading considered.
