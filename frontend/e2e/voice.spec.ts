import { expect, test } from "@playwright/test";
import { mockApi } from "./seed";

/**
 * The Voice DNA lifecycle through the screen a member actually uses: hand over
 * writing, build from it, watch it run, read what it learned, and choose it.
 *
 * Every step here failed in the running product while the backend's own tests
 * stayed green — those drive a scripted brain that answers correctly by
 * construction, and none of them renders a page. What broke was the reporting:
 * a build that failed said "the build didn't finish", a provider refusal blamed
 * the model, and the candidate asked to be accepted while showing none of
 * itself. This spec asserts what the SCREEN says at each step, which is the
 * only place those defects were visible.
 *
 * German chrome, as the app renders it.
 */

const PROFILE = {
  id: "vp-1",
  owner_id: "u-1",
  status: "collecting",
  maturity: "provisional",
  quality_band: "thin",
  voice_profile_md: "",
  profile_version: 0,
  personality_md: "",
  auto_learning_enabled: false,
  active_source_hash: null,
  candidate_version: null,
  last_built_at: null,
  source: "manual",
  captured_by: "human:u-1",
  version: 1,
  created_at: "2026-08-30T08:00:00Z",
  updated_at: "2026-08-30T08:00:00Z",
  archived_at: null,
};

const SUMMARY = {
  total_words: 4200,
  target_words: 30000,
  maturity: "provisional",
  quality_band: "thin",
  source_count: 3,
  register_words: { general: 4200 },
};

const SOURCE = {
  id: "vs-1",
  origin: "upload",
  kind: "document",
  register: "general",
  weight: 1,
  source_label: "emails.txt",
  source_ref: "voice:upload:x",
  word_count: 4200,
  included: true,
  exclusion_reason: null,
  extractor_version: "1",
  occurred_at: null,
  retention_until: null,
  content_erased_at: null,
  source: "manual",
  captured_by: "human:u-1",
  version: 1,
  created_at: "2026-08-30T08:00:00Z",
  updated_at: "2026-08-30T08:00:00Z",
  archived_at: null,
};

/** The candidate as a real build leaves it: a derived voice, and the evaluator's
 * reasons for holding it back. */
const CANDIDATE = {
  id: "vv-1",
  voice_profile_id: "vp-1",
  profile_version: 1,
  status: "candidate",
  voice_profile_md: "# Voice DNA",
  profile_json: {
    inference: {
      identity_summary: "Direkt, konkret, wenige Adjektive.",
      thinking_pattern: "Erst die Bitte, dann der Grund, dann die Frist.",
      signature_moves: [
        { move: "Verdict first", quote: "Passt das so?", sample_id: "s1" },
      ],
      avoid: ["Floskeln"],
    },
    sample_drafts: [
      {
        subject: "Re: Angebot",
        body: "Moin Stefan, kurz zum Angebot: Freitag steht.",
        voice_score: 0.56,
      },
    ],
    guidance: { next_best: "Mehr gesendete Mails." },
  },
  stats_json: { word_count: 4200, sample_count: 3, mean_sentence_words: 11 },
  source_hash: "h",
  source_count: 3,
  reason: "manual",
  predecessor_version: null,
  activation_policy_version: "2",
  model_provider: "openai_compatible",
  model_name: "anthropic/claude-haiku-4.5",
  builder_version: "voicebuilder/1",
  source: "build",
  captured_by: "human:u-1",
  evaluation: {
    held_out_prompts: 5,
    repeats_per_prompt: 3,
    active_median_voice_score: null,
    candidate_median_voice_score: 0.56,
    anti_ai_hard_failures: 0,
    structured_output_valid: false,
    corpus_citations_valid: true,
    identity_word_jaccard: 1,
    signature_set_jaccard: 1,
    removed_avoid_rules: 0,
    removed_register_rules: 0,
    classification: "material",
    passed: false,
  },
  review_reasons: ["median voice score 0.56 is below the 0.60 floor"],
  version: 1,
  created_at: "2026-08-30T08:05:00Z",
  updated_at: "2026-08-30T08:05:00Z",
  archived_at: null,
  activated_at: null,
};

const emptyPage = { page: { next_cursor: null, has_more: false } };

/** How many builds the screen actually started, so "takes no second press" is
 * counted at the wire rather than read off an attribute. */
let buildsStarted = 0;

/**
 * The voice endpoints, as a lifecycle rather than as fixed answers: what the
 * screen shows depends on what the previous step did, which is the whole
 * subject here. `buildStatus` and `versions` move as the test drives the page.
 */
async function mockVoice(
  page: import("@playwright/test").Page,
  script: {
    buildStatus: string;
    buildDetail?: string | null;
    versions: unknown[];
  },
): Promise<void> {
  await page.route(/\/v1\/voice-profiles/, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^\/v1/, "");
    const json = (body: unknown, status = 200) =>
      route.fulfill({
        status,
        contentType: "application/json",
        body: JSON.stringify(body),
      });

    if (path === "/voice-profiles") {
      return json({ data: [PROFILE], ...emptyPage });
    }
    if (path === "/voice-profiles/vp-1/sources") {
      return json({ data: [SOURCE], summary: SUMMARY, ...emptyPage });
    }
    if (path === "/voice-profiles/vp-1/versions") {
      return json({ data: script.versions, ...emptyPage });
    }
    if (path === "/voice-profiles/vp-1/builds") {
      if (route.request().method() === "POST") {
        buildsStarted += 1;
      }
      return json({ id: "vb-1", status: "queued" }, 201);
    }
    if (path === "/voice-profiles/vp-1/builds/vb-1") {
      return json({
        id: "vb-1",
        status: script.buildStatus,
        status_code:
          script.buildStatus === "failed" ? "model_unavailable" : null,
        status_detail: script.buildDetail ?? null,
      });
    }
    if (path.endsWith("/apply") || path.endsWith("/reject")) {
      return json({}, 200);
    }
    if (path === "/voice-profiles/vp-1/learning") {
      return json({
        drafted: 4,
        accepted: 3,
        edited_sent: 1,
        rejected: 0,
        qualifying_source_count: 3,
        qualifying_words: 4200,
        transformations: [],
      });
    }
    // Everything else the card reads is a plain list: deltas, and the paged
    // reads behind the disclosures.
    return json({ data: [], ...emptyPage });
  });
}

test.beforeEach(async ({ page }) => {
  buildsStarted = 0;
  await mockApi(page);
});

test("AC-voice-1: the card says what to hand over and why it matters", async ({
  page,
}) => {
  await mockVoice(page, { buildStatus: "running", versions: [] });
  await page.goto("/#/settings/voice");

  // The zone is a real file control, not a decorated div.
  const zone = page.locator('input[type="file"]');
  await expect(zone).toHaveAttribute("accept", ".txt,.md,.vtt,.srt,.json");
  // And the card answers "what should I upload?" where the uploading happens.
  await expect(page.getByText("Was am besten funktioniert")).toBeVisible();
  await expect(page.getByText("Warum das wichtig ist")).toBeVisible();
});

test("AC-voice-2: a running build says so, and takes no second press", async ({
  page,
}) => {
  await mockVoice(page, { buildStatus: "running", versions: [] });
  await page.goto("/#/settings/voice");

  const build = page.getByRole("button", { name: /Voice DNA/ }).first();
  await build.click();

  // In WORDS, not only as a spinner: a reader looking at the page was told
  // nothing, pressed again, and had no idea whether a second build started.
  await expect(page.getByText(/wird gerade gebaut/)).toBeVisible();
  await expect(page.getByText(/Seite verlassen/)).toBeVisible();
  // Busy, but still reachable by keyboard — a natively disabled button would
  // drop out of the tab order for the minute the build runs.
  await expect(build).toHaveAttribute("aria-busy", "true");
  await expect(build).not.toHaveAttribute("disabled", "");

  // And a second press starts NOTHING. Asserted by counting what reaches the
  // server, because a button that merely looks busy while still firing its
  // handler is the defect this claim is about.
  await build.click({ force: true });
  await build.click({ force: true });
  await expect.poll(() => buildsStarted).toBe(1);
});

test("AC-voice-3: a failed build names the cause the owner can act on", async ({
  page,
}) => {
  await mockVoice(page, {
    buildStatus: "failed",
    buildDetail:
      "Unser KI-Anbieter hat den Aufruf abgelehnt: Das Konto dahinter ist ohne Budget.",
    versions: [],
  });
  await page.goto("/#/settings/voice");

  await page
    .getByRole("button", { name: /Voice DNA/ })
    .first()
    .click();

  // The server's own sentence, not a fixed "the build didn't finish".
  await expect(page.getByText(/ohne Budget/)).toBeVisible();
});

test("AC-voice-4: a candidate can be read before it is chosen", async ({
  page,
}) => {
  await mockVoice(page, { buildStatus: "succeeded", versions: [CANDIDATE] });
  await page.goto("/#/settings/voice");

  // What the build learned — the decision is about something visible.
  await expect(
    page.getByText("Erst die Bitte, dann der Grund, dann die Frist."),
  ).toBeVisible();
  await expect(page.getByText(/noch nicht im Einsatz/)).toBeVisible();

  // The evidence a reader judges the voice BY, not only the summary of it: a
  // fixture whose keys the parser does not read renders an empty section and
  // an assertion on the summary alone stays green through it.
  await expect(
    page.getByText("Moin Stefan, kurz zum Angebot: Freitag steht."),
  ).toBeVisible();
  await expect(page.getByText("Re: Angebot")).toBeVisible();

  // Why it waits, in words about this voice rather than the evaluator's log,
  // and in the reader's own notation.
  await expect(page.getByText(/0,56/)).toBeVisible();
  await expect(page.getByText(/below the 0.60 floor/)).toHaveCount(0);
  // Nothing on the card reads as a broken number, which is what an unread
  // fixture key produces.
  await expect(page.getByText(/NaN/)).toHaveCount(0);

  // Both answers are offered, and neither is taken for the reader.
  await expect(
    page.getByRole("button", { name: "Diese Version verwenden" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Meine aktuelle Stimme behalten" }),
  ).toBeVisible();
});

test("AC-voice-5: choosing a candidate applies that version", async ({
  page,
}) => {
  await mockVoice(page, { buildStatus: "succeeded", versions: [CANDIDATE] });
  const applied: string[] = [];
  await page.route(/\/versions\/1\/apply/, async (route) => {
    applied.push(route.request().method());
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: "{}",
    });
  });
  await page.goto("/#/settings/voice");

  await page.getByRole("button", { name: "Diese Version verwenden" }).click();

  // The decision reaches the server as a write against THAT version.
  await expect.poll(() => applied).toContain("POST");
});
