# corpus/

Scenario files for the AI certification harness, one subdirectory per
`ai.Task` (e.g. `site_extract/empty_page.yaml`), loaded by `LoadCorpus`.

A scenario holds the data production is given, never the prompt production
sends. `site:` names which registered invocation site is under certification,
`fixture:` carries what that site is handed, and the site's own case builds the
request and applies the shipped validator. A scenario that carried a prompt
would certify a copy of one, and a copy stays green through the change that
breaks the original. That includes the per-call fence marker, which the product
mints and no scenario ever spells.

Every scenario is hand-authored (`source: hand_authored`) and names who
reviewed it for sensitive content (`sanitized_by`); `LoadCorpus` refuses
anything else. Every fixture under this tree is synthetic, invented for this
corpus: no real company, deal, or contact data.

## What a scenario asserts

`expect.outcome` is one of the four things a certified reply can be
(`internal/compose/aitasks`): `accepted`, `wrong_answer`, `invalid`, or
`abstained`. A run passes when the site's own validator reports the outcome the
scenario named. No outcome is privileged over the others, so a scenario
whose right answer is silence can exist.

`expect.answer` is written in the site's own vocabulary, because what separates
a right answer from a wrong one differs per site. Read the site's
own `Prepare` before authoring one. `site_extract/profile` accepts two
spellings. A bare field-to-value map fits when the whole claim is what a crawl
grounds. A `grounded`/`not_grounded` mapping fits when the scenario also needs to
say what must not be grounded; for a page that states nothing, that is the
whole claim.

`expect.rubric` is read to the grader, and it may only ask for what the site's
own reply envelope can carry. A rubric clause that scores a field the schema does not
declare, or offers credit for prose the envelope has no room for, can only mark
a correct reply down, or never fire. Either way it lowers a band for a reason no
model can fix. Read the site's request builder and its
answer schema before authoring one, the same way `expect.answer` is read out of
its `Prepare`.

## The gates this tree passes

- `TestLoadCorpusCoversEveryShippedSite` (`corpus_test.go`) runs both ways off
  the task contract: a shipped site with no scenario is a prompt that ships
  uncertified, and a `planned` task carrying one scores a prompt that does not
  ship. The unit is the site, since one task can have several (`cold_start` has
  four), so one scenario can never stand for a task's other prompts.
- `TestEveryCorpusScenarioPreparesAgainstItsSite` (`corpusprepare_test.go`)
  fails if a scenario's fixture is not the shape its site takes, or its
  expectation is one that site's validator could never satisfy.
- `TestEachAbstentionScenarioCatchesTheFabricationItTargets`
  (`corpusabstention_test.go`) runs each abstention scenario against both the
  answer it calls correct and the fabrication it exists to refuse, because a
  scenario that passes whatever the model does hides the fabrication it was
  written to catch.
- `TestEveryClosedAnswerKindCarriesAScenario`
  (`corpusanswerkinds_test.go`) covers what the per-site census cannot. A site
  whose answer comes from a closed vocabulary can pass the per-site census with
  one scenario, which leaves most values untested. This test requires every
  value in the site's shipped response-schema enum to appear in some scenario's
  `expect.answer`.

  Before authoring against it, know three properties:
  - The vocabulary is read off the request the site's own code builds, never a
    list kept in the test, so it cannot certify a copy of the enum.
  - The unit is the **enum**, because a schema can be shared. The onboarding
    conversation sites send one `companyReadMessageSchema` and each narrows it in
    prose the gate cannot read. Demanding every kind of every site would demand
    scenarios the prompt forbids, so a kind is covered when some site sharing the
    enum scores it.
  - Only an `accepted` scenario credits coverage. A refusal or an abstention names
    a kind without asking a model to produce it, so counting it would hide a real
    miss.
