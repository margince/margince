# How AI certification works

The [certification page](../reference/ai-certification.md) grades every AI feature
under every preset. This page explains, in plain words, how one grade is made. To
run the lane, see [certify-an-ai-model.md](../how-to/certify-an-ai-model.md); to
write a case, see [write-a-certification-case.md](../how-to/write-a-certification-case.md).
The exact rule and its numbers live in `backend/internal/compose/aicert/score.go`
and `thresholds.go`, and the certification page prints them.

A **scenario** (a test case) is one realistic situation — an email, an account, a
web page — plus the answer we expect. It lives in
`backend/internal/compose/aicert/corpus/<task>/*.yaml`.

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

1. The scenario's data goes through the **product's real prompt builder**, so the
   model sees exactly what production sends. A scenario never carries a prompt of
   its own — a copy would stay green while the real one broke.
2. The model answers.
3. **① Is it right?** The product's own validator and the expected answer decide
   pass or fail. Strict, and no AI involved.
4. **② Is it good?** A second model, the judge, scores the answer 0–100 against
   the scenario's rubric. It is asked **once**. A score within 10 points of one of
   the scenario's bars is asked for a **second** time, and two readings more than 5
   apart for a **third**; the middle one counts (the average, of two). So one odd
   reading cannot swing a close call, and a clear one is not paid for three times.
   It sees the product's rules, so it
   never marks down what the product allows. It never sees ①'s verdict. Where the
   "expected answer" is a check list (phrases that must not appear) rather than a
   model answer, it is not shown to the judge at all.

The judge is never the model being tested, nor one of its family: a Gemini
candidate is not graded by a Gemini judge. The default judge is
`claude_cli:claude-sonnet-4-6`.

## Several tries per scenario

```
 try 1 ─┐
 try 2 ─┼─► close to a threshold? ──yes──► 3 more tries (up to 9)
 try 3 ─┘            │
                     no
                     ▼
              scenario is decided
```

Models vary from run to run, so every scenario gets **3 tries**. If its result sits
near a threshold, it gets **3 more, up to 9**. Clear cases stop early; uncertain
ones gather more evidence instead of being settled by a coin flip. A feature with a
single scenario always runs to 9, because 3 out of 3 is not yet enough evidence.

## What each scenario must clear

- right in **at least half** of its tries;
- **no single answer below its floor** (for example 40) — one very bad answer
  blocks ✅;
- not **consistently mediocre**: its quality must be able to reach its bar.

A scenario that is clearly broken — wrong most of the time, or scoring under the
lower bar even on its best reading — **vetoes** the whole feature. Averaging cannot
hide it.

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
         ⚠️ Usable with care  ≥ 2/3 right, quality ≥ the lower bar
         ❌ Not reliable yet   anything less
         🔒 Not served here    local-only data on a preset with no local model
         ❔ Not measured       nobody has run it on this model yet
```

Why pooled: requiring *every* scenario to pass on its own multiplies the chances of
a stray miss — nine scenarios each right 90% of the time would reach ✅ only four
times in ten. Pooling allows a stray miss; the per-scenario gates and the veto still
refuse a real weakness.

## Saved and published

```
  records/<task>/<model>.json ──► docs/reference/ai-certification.md
                                  "gemini_cloud: N of M features ready…"
```

- The result is written to a **record** per feature and model
  (`backend/internal/compose/aicert/records/`). It names the model, the judge, the
  thinking level and when it ran.
- The certification page is generated from the records.
- When the prompt, a scenario or the grading rule changes, the record reads
  **re-check pending** until somebody runs it again. A grade always describes the
  prompt that was measured, never a later one.

## When a feature fails: fix in this order

1. **The case is wrong or not doable** — the expected answer is not the only right
   one, a fact the answer needs is missing, or the check contradicts the rubric.
   Fix the case.
2. **The prompt confuses the model** — rules that conflict, or the deciding rule
   buried. Fix the prompt ([prompt-principles.md](prompt-principles.md)).
3. **The model really cannot do it** — only then change its settings (thinking
   level) or move the feature to a stronger tier.

## What a run costs

Every try costs one candidate call and one to three judge calls — about 1.4 on
average, three only on a close and contested score — and an uncertain scenario can
run up to 9 tries, so the judge is still most of the cost. On the default
`claude_cli` judge that is subscription usage rather than money, and the
subscription's session limit is shared with everything else using it: keep a sweep
to two or three tasks in parallel.
