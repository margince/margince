# Hunt for prompt rules that disagree

A band that stays low after a prompt is "fixed" is often not a weak model. Two rules in one
prompt can give different answers to one situation, and each model settles it in its own way. One
drops what another invents, and the two grades look like two problems. Hunt for these before you write
a new rule, because a rule on top of two that disagree makes three.

Work from the sweep loop in [re-certify-the-whole-corpus.md](re-certify-the-whole-corpus.md). The
steps below are what you do when a band will not move.

## 1. Read the prompt the model sees

`docs/reference/ai-prompts.json` holds each site's prompt with its shared blocks already put together.
Read that text, not the Go source. The disagreement is usually between a shared block and the
site's own lines, and the source keeps those apart.

## 2. Ask a strong model to review one site at a time

Give it the composed prompt and this definition of a conflict:

- two instructions that give different answers to one situation;
- a specific rule that overrides a general one without saying so;
- an example that breaks a rule;
- a rule the data the site is given cannot satisfy.

Ask for both quotes, one invented input where they diverge, what a small model does against what
a careful reader does, and which rule should win. Ask for the pairs it looked at and dropped, so you
can judge how many findings are noise.

Treat the answer as candidates. A blind run over prompts whose conflicts were already known found
about half of them. It missed a rule that was simply absent, and two fields that only conflict
together. It also could not see the user message, which the rendered file does not hold.

## 3. Confirm a candidate on real models before you edit

Take a recorded request from the payload trace (`request_payload` holds `system` and `messages`) and send
it again to two models, together with the candidate's invented input. For Gemini, set
`generationConfig.thinkingConfig` to `{"includeThoughts": true, "thinkingLevel": "high"}`. The reply then
holds parts marked `"thought": true`, which are the thought summaries. The Gemini presets set `low`, which
writes none, so the replay shows why a model answers the way it does, not what production answers.

Decide from the answers and use the summaries to explain them. A summary can leave out a conflict that
really drove the answer, and two models can quote two different rules that agree. The conflict is real when
the answers split the way the candidate predicts, for instance one model leaving out what the other invents.
If every model answers the same correct way, leave the prompt alone.

## 4. Give the case one owner

Write one rule that says what wins. Change the other places so they point to it or stop saying the
opposite. Do not add a third rule next to the two.

## 5. Measure it like any prompt change

Copy the records first. Run both preset sweeps and compare per scenario against the copy. A shared
block moves every site that carries it, so read the list of stale rows before you start. Say in the
pull request which records moved because of the edit and which moved for another reason.

## What this cannot find

- A rule that is missing, such as a sibling prompt having a line this one lacks.
- Whether a model reads a correct rule correctly. That is what the certification runs measure.
- A conflict that only exists when two input fields arrive together. Add the field pair to a case.
