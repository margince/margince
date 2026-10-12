<!-- prose:plain -->
# How AI certification works

The [certification page](../reference/ai-certification.md) grades every AI feature under every preset.
Here, in plain words, is how one grade is made. To run the lane, see
[certify-an-ai-model.md](../how-to/certify-an-ai-model.md); to write a case, see
[write-a-certification-case.md](../how-to/write-a-certification-case.md). The full rule and its numbers
are in `backend/internal/compose/aicert/score.go` and `thresholds.go`, and the certification page prints
them.

A **scenario** (a test case) is one real case (an email, an account, a web page) plus the answer we
expect. It sits in `backend/internal/compose/aicert/corpus/<task>/*.yaml`.

## One try

```
 ┌──────────────────────────── ONE TRY ────────────────────────────┐
 │                                                                  │
 │  scenario.yaml                                                   │
 │  (fixture data + expected answer + rubric + quality bars)        │
 │        │                                                         │
 │        ▼                                                         │
 │  PRODUCT's own prompt builder  ── the exact prompt users get     │
 │        │                                                         │
 │        ▼                                                         │
 │  CANDIDATE model (e.g. gemini-3.1-flash-lite)                    │
 │        │  answer                                                 │
 │        ▼                                                         │
 │  ① MECHANICAL CHECK  (product's validator + expected answer)     │
 │     right label? right record cited? nothing invented?           │
 │        │ pass / fail                                             │
 │        ▼                                                         │
 │  ② JUDGE (a second model) asked once; again if near a bar, and a │
 │     third time if those two disagree; the middle score counts    │
 │     sees: rubric, product rules, input, expected answer, answer  │
 │     gives: 0–100 "is it GOOD?"                                   │
 │                                                                  │
 │  result of one try = (pass/fail, quality score)                  │
 └──────────────────────────────────────────────────────────────────┘
```

1. The scenario's data goes through the **product's real prompt builder**, so the model sees what
   production sends.
2. A scenario never holds a prompt of its own, because a copy would stay green while the real one failed.
3. The model answers.
4. **① Is it right?** The product's own validator and the expected answer decide pass or fail. It is
   strict, and no AI takes part.
5. **② Is it good?** A second model, the judge, scores the answer 0–100 against the scenario's rubric.
   The judge is asked once.
   - If the score is within 10 points of one of the scenario's bars, or below the lowest bar, the judge
     is asked a second time. If those two scores differ by more than 5, it is asked a third time.
   - The middle score counts; with two scores, the average counts. So one odd reading cannot change a
     close call, and a clear one is not run three times.
   - The judge sees the product's rules, so it never marks down what the product allows. It never sees
     the verdict of ①.
   - The "expected answer" may be a check list (words that must not appear), not a model answer. Then
     the judge does not see it at all.

Some scenarios skip ②. The answer may be a closed label, a tool name, a number or an empty list. Then the
check in ① already sees all that a rubric would ask for. Such a scenario declares `judge: none` and says
why in `judge_none_reason`, and it has no rubric and no quality bars. It counts among the feature's right
answers like any other, and adds nothing to the quality average.

A gate runs each one against the wrong
answer a judge would catch, and that answer must fail. The certification page marks these scenarios
`checked mechanically`.

The judge is never the exact model under test. A judge from the same vendor or family may still grade,
and the record marks that run as self judged. The default judge is `claude_cli:claude-sonnet-4-6`.

## Many tries per scenario

```
 try 1 ─┐
 try 2 ─┼─► close to a threshold? ──yes──► 3 more tries (up to 9)
 try 3 ─┘            │
                     no
                     ▼
              scenario is decided
```

Models change from run to run, so every scenario gets **3 tries**. If its result sits near a threshold,
it gets **3 more, up to 9**. Clear cases stop early; cases near the line get more evidence, so chance does
not settle them. A feature with a single scenario always runs to 9, because 3 out of 3 is not
yet enough evidence.

## What each scenario must clear

- right in **at least half** of its tries;
- **no single answer below its floor** (for example 40): one very bad answer blocks ✅;
- not **always just middling**: its quality must be able to reach its bar.

A scenario that clearly fails **blocks** the whole feature: ❌, no matter what its siblings score. Such a
scenario is wrong most of the time, or under its floor even on its best reading. An average cannot hide it. A
scenario whose best reading stays under the lower bar shows ❌ on its own row and blocks ✅, but the pool
may still reach ⚠️.

## The feature's grade: all scenarios together

```
              all tries of all scenarios, pooled
                          │
     ┌────────────────────┼─────────────────────┐
     ▼                    ▼                     ▼
 ≥90% right,          average quality        no scenario
 and ≥80% even        above each             vetoed
 at the cautious      scenario's bar,
 end of the margin    with a margin
     └────────────────────┼─────────────────────┘
                          ▼
         ✅ Ready             all three hold
         ⚠️ Usable with care  ≥ 2/3 right, quality ≥ the lower bar, no veto
         ❌ Not reliable yet   anything less
         🔒 Not served here    local-only data on a preset with no local model
         ❔ Not measured       nobody has run it on this model yet
```

Why one pool: if *every* scenario had to pass on its own, the chances of a single miss would add up. Take
9 scenarios that are each right 90% of the time: they would reach ✅ only four times in ten.
A pool allows a single miss, while the gates per scenario and the block above still refuse a real weak spot.

## Stored and published

```
  records/<task>/<model>.json ──► docs/reference/ai-certification.md
                                  "gemini_cloud: N of M features ready…"
```

- The result is written to a **record** per feature and model
  (`backend/internal/compose/aicert/records/`). It names the model, the judge, the thinking level and
  when it was run.
- The certification page is generated from the records.
- When the prompt, a scenario or the grading rule changes, the record reads **`re-check pending`** until
  someone runs it again. A grade always describes the prompt that was measured, never a later one.

## When a feature fails: fix in this order

1. **The case is wrong or cannot be done.** The expected answer may not be the only right one. A fact the
   answer needs may be missing, or the check and the rubric may disagree. Fix the case.
2. **The prompt confuses the model.** Rules conflict, or the rule that decides is hard to find in the
   text. Fix the prompt ([prompt-principles.md](prompt-principles.md)).
3. **The model really cannot do it.** Only then change its settings (thinking level) or move the feature
   to a stronger tier.

## What a run costs

Every try costs one candidate call and one to three judge calls. That is about 1.4 judge calls on
average, and three only on a close score where the two first scores disagree. A scenario near the line
can run up to 9 tries, so the judge is still most of the cost. On the default `claude_cli` judge that is
use of a subscription, not money. The subscription's session limit is shared with all else that uses
it, so keep a sweep to two or three tasks at the same time.
