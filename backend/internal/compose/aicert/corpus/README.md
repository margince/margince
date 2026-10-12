<!-- prose:plain -->
# corpus/

Scenario files for the AI certification harness, one folder per `ai.Task` (for example
`site_extract/empty_page.yaml`). `LoadCorpus` loads them.

A scenario holds the data production is given, never the prompt production sends. `site:` names
which registered call site is under certification, and `fixture:` holds what that site is handed. The
site's own case builds the request and applies the shipped validator. A scenario that held a prompt
would certify a copy of one, and a copy stays green through the change that breaks the original.
That includes the fence marker for each call, which the product makes and no scenario ever spells.

Every scenario is written by hand (`source: hand_authored`), and names who reviewed it for sensitive
content (`sanitized_by`); `LoadCorpus` refuses anything else. Every fixture under this tree is made
up for this corpus: no real company, deal or contact data.

## What a scenario checks

`expect.outcome` is one of the four things a certified reply can be (`internal/compose/aitasks`):
`accepted`, `wrong_answer`, `invalid` or `abstained`. A run passes when the site's own validator
reports the outcome the scenario named. No outcome ranks above the others, so a scenario whose right
answer is silence can exist.

`expect.answer` is written in the site's own words, because what tells a right answer from a wrong
one is not the same for each site. Read the site's own `Prepare` before you write one.
`site_extract/profile` accepts two spellings. A bare map from field to value fits when the whole
claim is what a crawl grounds. A `grounded`/`not_grounded` mapping fits when the scenario must also
say what must not be grounded. For a page that states nothing, that is the whole claim.

`expect.rubric` is read to the grader, and it may only ask for what the site's own reply envelope can
hold. Say a rubric line scores a field the schema does not declare, or gives credit for text the
envelope has no room for. Then it can only mark a right reply down, or never fire. Either way it
lowers a band for a reason no model can fix. Read the site's request builder and its answer schema
before you write one, the same way `expect.answer` is read out of its `Prepare`.

## The gates this tree passes

- `TestLoadCorpusCoversEveryShippedSite` (`corpus_test.go`) runs both ways from the task contract. A
  shipped site with no scenario is a prompt that ships without certification. A `planned` task that
  has one scores a prompt that does not ship. The unit is the site, since one task can have many
  (`cold_start` has 4). So one scenario can never stand for the other prompts of a task.
- `TestEveryCorpusScenarioPreparesAgainstItsSite` (`corpusprepare_test.go`) fails if the fixture of a
  scenario is not the shape its site takes. It also fails if the scenario expects a result that the
  validator of that site could never give.
- `TestEachAbstentionScenarioCatchesTheFabricationItTargets` (`corpusabstention_test.go`) runs each
  abstention scenario against both the answer it calls right and the made-up answer it exists to
  refuse. A scenario that passes whatever the model does hides the made-up answer it was written to
  catch.
- `TestEveryClosedAnswerKindCarriesAScenario` (`corpusanswerkinds_test.go`) covers what the census
  per site cannot. A site whose answer comes from a closed set of words can pass the census per site
  with one scenario, which leaves most values untested. This test needs every value in the shipped
  response schema enum of the site to show up in the `expect.answer` of some scenario.

  Before you write against it, know three things:
  - The words are read from the request the site's own code builds, never from a list in the test.
    So it cannot certify a copy of the enum.
  - The unit is the **enum**, because a schema can be shared. The onboarding conversation sites send
    one `companyReadMessageSchema`, and each one narrows it in text the gate cannot read. To demand
    every kind of every site would demand scenarios the prompt forbids. So a kind is covered when
    some site that shares the enum scores it.
  - Only an `accepted` scenario counts toward coverage. A refusal or an abstention names a kind
    without asking a model to make it, so to count it would hide a real miss.
