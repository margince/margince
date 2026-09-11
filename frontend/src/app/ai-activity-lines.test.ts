import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { de } from "../i18n/de";
import { en } from "../i18n/en";
import { vi } from "../i18n/vi";
import { NAMED, SAID, WROTE } from "./agentrail-copy";
import { ACTIVITY_LINE, NAMED_LINE } from "./ai-activity-lines";
import {
  displayedKinds,
  displayedLines,
  speak,
  spokenText,
} from "./ai-activity-speak";

/** The line as one string, which is what the unnamed cases are about. */
function lineFor(
  item: Parameters<typeof speak>[0],
  t: Parameters<typeof speak>[1],
): string | null {
  const line = speak(item, t);
  return line === null ? null : spokenText(line);
}

/** Every message key either map names, which is the whole reachable set. */
function namedKeys(): Set<string> {
  return new Set<string>([
    ...displayedLines().flatMap(([, byState]) => Object.values(byState)),
    ...Object.values(NAMED_LINE).flatMap((byState) => Object.values(byState)),
  ]);
}

/** The named variants, which are the only lines allowed a placeholder. */
function namedVariantKeys(): Set<string> {
  return new Set<string>(
    Object.values(NAMED_LINE).flatMap((byState) => Object.values(byState)),
  );
}

// A key the map names must exist in the catalog, and a translated
// `agent.activity.` key nothing names is copy three translators paid for and
// no reader can reach. Both directions are reported as lists, because a wiring
// fix wants the whole set rather than the first thing that broke.
describe("the activity copy set", () => {
  it("is exactly the key set the maps name", () => {
    const named = namedKeys();
    const inCatalog = Object.keys(en).filter((key) =>
      key.startsWith("agent.activity."),
    );
    expect(
      [...named].filter((key) => !(key in en)),
      "keys the map names and no catalog carries",
    ).toEqual([]);
    expect(
      inCatalog.filter((key) => !named.has(key)),
      "copy translated three times and named by nothing",
    ).toEqual([]);
  });

  // A placeholder is admissible in exactly one place: a NAMED variant, which
  // exists to put the subject's own name in the sentence. Everywhere else these
  // lines stay fixed literals, because a placeholder in a line nothing
  // interpolates renders `{name}` at a reader.
  //
  // The named ones are held to `{name}` and nothing else, in every locale: a
  // translator who invents a second placeholder writes a token no caller fills,
  // and it reaches the rail verbatim.
  it.each([
    { locale: "en", catalog: en },
    { locale: "de", catalog: de },
    { locale: "vi", catalog: vi },
  ])(
    "places a placeholder only in a named variant ($locale)",
    ({ catalog }) => {
      const named = namedVariantKeys();
      for (const [key, value] of Object.entries(catalog)) {
        if (!key.startsWith("agent.activity.")) {
          continue;
        }
        if (named.has(key)) {
          expect(value.match(/\{[^}]*\}/g) ?? [], key).toEqual(["{name}"]);
          continue;
        }
        expect(value, key).not.toMatch(/\{/);
      }
    },
  );

  // The feature's most dangerous failure mode is telling someone a run
  // finished when it only got partway, so this is pinned in every locale it
  // ships in rather than English alone: a translator adding "fertig" to a
  // German degraded line would pass an English-only version of this test.
  it.each([
    { locale: "en", catalog: en, done: ["done"], ready: ["ready"] },
    { locale: "de", catalog: de, done: ["fertig"], ready: ["bereit"] },
    { locale: "vi", catalog: vi, done: ["xong"], ready: ["sẵn sàng"] },
  ])(
    "never says done or ready about a run that stopped early ($locale)",
    ({ catalog, done, ready }) => {
      const degradedKeys = [
        ...displayedLines().map(([, byState]) => byState.degraded),
        ...Object.values(NAMED_LINE).map((byState) => byState.degraded),
      ];
      for (const key of degradedKeys) {
        if (key === undefined) continue;
        const degraded = catalog[key as keyof typeof catalog].toLowerCase();
        for (const word of [...done, ...ready]) {
          expect(degraded).not.toContain(word);
        }
      }
    },
  );

  // Totality over (kind, state) is the COMPILER's, since ACTIVITY_LINE is typed
  // as a full Record — a hard-coded state list here could only check the states
  // somebody remembered to write down, and that list is what goes stale. What
  // is left for runtime is that each key resolves to real copy: a key that
  // typechecks but names no message renders as the key string to a reader.
  it("names copy that exists, for every displayed kind and state", () => {
    let checked = 0;
    for (const [kind, byState] of displayedLines()) {
      for (const [state, key] of Object.entries(byState)) {
        expect(
          en[key as keyof typeof en],
          `${kind}.${state} -> ${key}`,
        ).toBeTruthy();
        checked++;
      }
    }
    // A map that lost its entries would pass every assertion above.
    expect(checked).toBe(displayedLines().length * 6);
    // And a map that narrated NOTHING would pass that too.
    expect(displayedLines().length).toBeGreaterThan(0);
  });

  // A kind that is not narrated says why, in a sentence a reader of this file
  // can weigh. An empty reason is the same silence the rail's totality exists
  // to prevent, one level further in: it records that somebody chose, without
  // recording what they chose it for.
  it("gives every undisplayed kind a reason", () => {
    const undisplayed = Object.entries(ACTIVITY_LINE).filter(
      ([, entry]) => "notDisplayed" in entry,
    );
    expect(
      undisplayed.length,
      "no kind is undisplayed, so this proves nothing",
    ).toBeGreaterThan(0);
    for (const [kind, entry] of undisplayed) {
      const reason = "notDisplayed" in entry ? entry.notDisplayed : "";
      expect(reason.trim(), kind).not.toBe("");
    }
  });

  // The one state a reader must always be told about, whatever the work was.
  // It is the only state no writer produces — the server derives it — so a kind
  // that forgot it would go silent for exactly the case it exists to report.
  it("gives every displayed kind a line for the derived stalled state", () => {
    for (const [kind, byState] of displayedLines()) {
      expect(byState.stalled, kind).toBeDefined();
      expect(en[byState.stalled as keyof typeof en], kind).toBeTruthy();
    }
  });
});

describe("speak", () => {
  // The whole reason the line is three pieces and not one string: the name
  // sits in its own slot, with the record's page beside it, so a surface can
  // draw a link around exactly the name and nothing else.
  it("sets the record's name in its own slot, with the page it has", () => {
    const line = speak(
      {
        kind: "summarize",
        state: "done",
        subject_label: "Acme",
        subject_type: "company",
        subject_id: "019f7e65-fbf7-7114-b114-40af4af63a77",
      },
      (key) => en[key],
    );
    expect(line).toEqual({
      before: "What I know about ",
      subject: {
        name: "Acme",
        route: {
          screen: "companies",
          id: "019f7e65-fbf7-7114-b114-40af4af63a77",
        },
      },
      after: " is ready.",
    });
  });

  // A name that is also a word in the copy must still be the NAME that gets
  // the link. The split is on the template's placeholder and never on the
  // interpolated sentence, and this is the case that tells the two apart.
  it("finds the name by its slot, not by searching the sentence for it", () => {
    const line = speak(
      {
        kind: "summarize",
        state: "done",
        subject_label: "I",
        subject_type: "person",
        subject_id: "019f7e65-fbf7-7114-b114-40af4af63a77",
      },
      (key) => en[key],
    );
    expect(line?.before).toBe("What I know about ");
    expect(line?.subject?.name).toBe("I");
    expect(line?.subject?.route?.screen).toBe("contacts");
  });

  // No page, no route — and the name still stands in the sentence. Three ways
  // to have no page: a kind with no screen (a meeting is an activity), a kind
  // this build has never heard of, and a name that came with no identity.
  it.each([
    [
      "a meeting",
      {
        subject_type: "activity",
        subject_id: "019f7e65-fbf7-7114-b114-40af4af63a77",
      },
    ],
    [
      "an unknown kind",
      {
        subject_type: "constellation",
        subject_id: "019f7e65-fbf7-7114-b114-40af4af63a77",
      },
    ],
    ["a name with no id", { subject_type: "company" }],
    [
      "a name with no kind",
      { subject_id: "019f7e65-fbf7-7114-b114-40af4af63a77" },
    ],
  ])("keeps the name as text for %s", (_what, subject) => {
    const line = speak(
      { kind: "summarize", state: "done", subject_label: "Acme", ...subject },
      (key) => en[key],
    );
    expect(line?.subject).toEqual({ name: "Acme", route: null });
    expect(line && spokenText(line)).toBe("What I know about Acme is ready.");
  });

  it("renders the line for a state that has copy", () => {
    expect(
      lineFor({ kind: "morning_brief", state: "running" }, (key) => en[key]),
    ).toBe(en["agent.activity.morningBrief.running"]);
  });

  // The map is total over the contract, so the only way to miss is a value the
  // contract does not carry — which is exactly what an OLDER TAB receives from
  // a newer server that has added a state or a kind. translate() falls back to
  // the key string, so without the existence check that reader is shown
  // `agent.activity.morningBrief.undefined` instead of nothing.
  //
  // The casts are the point rather than a shortcut: this asserts the runtime
  // behaviour for a value the type system has already ruled out, and there is
  // no other way to express it.
  it.each([
    ["an unknown state", { kind: "morning_brief", state: "hibernating" }],
    ["an unknown kind", { kind: "weekly_digest", state: "running" }],
    // The third way to draw nothing, and the one the server now produces by
    // the thousand: a kind this build reports and deliberately does not
    // narrate. It must read as silence, not as a message key.
    // One case per REASON — all SIX. A single case would keep passing while the
    // others were narrated by accident, and the entries carrying a reason of
    // their own (`cert_judge`, `enrich`, and the site lanes) are exactly the
    // ones a "one example per shared constant" reading misses. A kind that
    // becomes displayed can no longer stand here, which is what this shape
    // catches.
    ["a background sweep", { kind: "brief_ranking", state: "done" }],
    ["work the asker is watching", { kind: "cold_start", state: "done" }],
    ["a task nothing has built", { kind: "nl_search", state: "done" }],
    ["the lane grading this build", { kind: "cert_judge", state: "done" }],
    // The two reasons carried by an entry of its own rather than a shared
    // constant, which is the pairing a hand-kept list drops first.
    ["a pass that reaches nobody", { kind: "enrich", state: "done" }],
    ["a site-read lane", { kind: "site_triage", state: "running" }],
  ])("renders nothing at all for %s", (_name, item) => {
    expect(lineFor(item, (key) => en[key])).toBeNull();
  });
});

// The site-read lanes carry ONE reason of their own, shared with nothing else.
//
// A prose reason cannot be checked by reading it, but the OBJECT a kind carries
// can be. These three sat on SYSTEM_SWEEP, whose sentence says the work belongs
// to nobody in particular — false whenever a human asked for the read, because
// compose binds that person as on_behalf_of and the occurrence lands in their
// own feed. A reader who trusted the shared sentence would conclude no site read
// can reach a person, and stop looking.
//
// Identity, not text: the reason may be reworded freely and only re-pointing a
// lane at another entry fails. Three assertions, because each catches a
// different way to be wrong and the obvious one-liner catches only the first:
//
//   same object   — the three cannot drift into three near-copies
//   notDisplayed  — a lane given a real LINE TABLE fails here. The version of
//                   this gate that compared reason STRINGS passed happily when
//                   two of the three were displayed, because the absent reason
//                   read as "" and "" is not the sweep's sentence. That is the
//                   inverse of the documented decision passing its own guard.
//   unshared      — "not the sweep's" is not "its own". Pointing all three at
//                   WATCHED_BY_THE_ASKER, or at any new shared sentence, is
//                   still the bug this exists to prevent.
describe("the site-read lanes carry one reason of their own", () => {
  const LANES = ["site_extract", "site_fact_extract", "site_triage"] as const;
  const entryFor = (kind: (typeof LANES)[number] | string) =>
    ACTIVITY_LINE[kind as (typeof LANES)[number]];

  it("is one entry object, not three near-copies", () => {
    expect(new Set(LANES.map(entryFor)).size).toBe(1);
  });

  it.each(LANES)("%s is undisplayed and says why", (kind) => {
    const entry = entryFor(kind);
    expect("notDisplayed" in entry).toBe(true);
    expect("notDisplayed" in entry ? entry.notDisplayed.trim() : "").not.toBe(
      "",
    );
  });

  it("shares that entry with no other kind", () => {
    const lanes = new Set<string>(LANES);
    const theirs = entryFor(LANES[0]);
    const trespassers = Object.entries(ACTIVITY_LINE)
      .filter(([kind, entry]) => !lanes.has(kind) && entry === theirs)
      .map(([kind]) => kind);
    expect(trespassers).toEqual([]);
  });
});

// The set the rail asks the server for, pinned.
//
// Not a tautology restating the map: it is the one place a kind ENTERS the
// product, and adding one has consequences a compiler cannot see. `enrich` was
// added here and reverted, because every occurrence of it is workspace-scoped —
// its only production site runs under a system principal with no on_behalf_of,
// and the personal feed selects on actor_user_id, so no reader could ever have
// been shown the copy that came with it.
//
// A failure here is not a bug. It means somebody widened what the rail draws,
// and owes two answers: can an occurrence of that kind reach ONE person's feed,
// and does it still fit inside `recent`'s cap of ten alongside the rest.
describe("the kinds the rail asks for", () => {
  it("is exactly the reviewed set", () => {
    expect([...displayedKinds()].sort()).toEqual([
      // The account scan's sentence. It reaches one person's feed — the read
      // runs under the reader's own principal, for the account they opened,
      // so the occurrence is theirs rather than the workspace's. And it fits:
      // one per reader per account opened, held to one read an hour by the
      // rescan floor, so a morning's reading does not push the rest out of
      // `recent`'s ten.
      "account_scan",
      "document_extract",
      "draft_reply",
      "morning_brief",
      "offer_draft",
      "overnight_at_risk_sweep",
      // A company's website read end to end. It reaches one person's feed:
      // the dossier is announced under the requester as on_behalf_of, so a
      // read a rep started from a company page is scoped to that rep, while
      // the sweeps' reads name no human and never reach anyone. And it fits:
      // a rep reads a handful of companies in a day, not ten.
      "site_read",
      "summarize",
      // Reading a meeting transcript for the next steps in it. It reaches one
      // person's feed: the run row records who pressed the button, so the
      // occurrence is theirs rather than the workspace's — which is also why it
      // is no longer SYSTEM_SWEEP's "belongs to nobody in particular". And it
      // fits: a rep reads the transcript of a meeting they sat in, a handful a
      // week, and uq_transcript_read_inflight allows one in flight per
      // transcript however often the button is pressed.
      "transcript_propose",
      // Learning the reader's own writing voice. It reaches one person's feed:
      // the build names the requester and visibleProfile admits only the
      // profile's owner, so the occurrence can belong to nobody else. And it
      // fits: a voice is learned when a rep asks for it or when their corpus
      // has drifted, which is a handful of builds in a profile's life, and
      // activeVoiceBuild refuses a second build over the same corpus while one
      // is in flight.
      "voice_build",
      // What the week TAUGHT, on the same terms as the sentence above it: one
      // occurrence per rep per week, scoped to that rep by ResolveActor. It
      // earns real copy rather than the system-sweep line for the same reason —
      // a rep waiting on their own week should be told what is being done to
      // it, and "lessons" is a different promise from "a summary".
      "weekly_learnings",
      // The weekly retrospective's sentence. It reaches one person's feed —
      // the pass runs under that rep's own principal over their own week, so
      // ResolveActor scopes the occurrence to them rather than to the
      // workspace. And it fits: one occurrence per rep per week is the rarest
      // thing on this rail, nowhere near `recent`'s cap of ten.
      "weekly_review",
    ]);
  });
});

// One action, one vocabulary.
//
// The taskbar ticker names work by react-query key; the rail names it by AI
// task. Where a key and a displayed kind denote THE SAME action, a reader meets
// two different sentences for one thing — the bar saying "Writing to Anna"
// while the panel says "I'm drafting your reply."
//
// The pairing is HAND-MAINTAINED and cannot be otherwise: nothing in the types
// connects a mutation key to the task it triggers, and that missing link is
// exactly what made `enrich` look visible when it was not — the ticker has an
// `enrich` key for work that never runs ai.TaskEnrich. So this list is a record
// of collisions somebody checked by reading both sides, and its value is that
// re-adding either half fails HERE rather than in front of a user.
describe("the ticker and the rail never narrate one action twice", () => {
  const COLLISIONS: readonly (readonly [
    tickerKey: string,
    railKind: string,
  ])[] = [
    // The DRAFT mutations carry ["email-draft", …] and are the draft_reply
    // call; the SEND mutations keep ["email", …] and are a plain write the rail
    // knows nothing about. They were ONE key until this pairing forced the
    // question, and deleting the shared entry silenced the sends — the split is
    // what lets each be narrated exactly once.
    ["email-draft", "draft_reply"],
  ];

  it.each(COLLISIONS)(
    "does not carry ticker key %s while the rail draws %s",
    (tickerKey, railKind) => {
      // A string compare over the drawn kinds, not `includes(x as never)`:
      // the cast would suppress a typo in COLLISIONS and quietly pass.
      const drawn = displayedKinds().some((kind) => kind === railKind);
      const narrated =
        Object.hasOwn(WROTE, tickerKey) ||
        Object.hasOwn(SAID, tickerKey) ||
        Object.hasOwn(NAMED, tickerKey);
      expect(drawn && narrated).toBe(false);
    },
  );
});

// The other half of the split, gated at the CALL SITES.
//
// The pairing above reads the ticker's tables, so it catches somebody re-adding
// a key there. It does not catch the same collision arriving from the other
// direction — a draft mutation renamed back to ["email", …] leaves both tables
// untouched and quietly restores two vocabularies for one action. That is the
// hole this closes, and finding it took mutating the call site and watching the
// first gate stay green.
//
// Read from source rather than imported: a mutationKey is a literal inside a
// hook, exported by nothing.
describe("the email mutation keys stay split", () => {
  // DERIVED from the tree, never listed. The list was two named files, and one
  // of them stopped writing mail at all when the person page's second composer
  // was retired — so the gate failed on a file that had become innocent, and
  // would have gone on passing for any NEW screen that took up the old key. A
  // census that can fail short has already failed: what this protects is the
  // split itself, so its corpus is every screen that spells either key.
  const sites = mailMutationSites();

  it("finds the screens that write mail at all", () => {
    // Fail-closed. A miswired scan reads an empty tree, reports PASS, and there
    // is no failing assertion to notice.
    expect(sites.length).toBeGreaterThan(0);
  });

  it.each(sites)("keeps drafts off the send key in %s", (file) => {
    const src = readFileSync(join(screensDir(), file), "utf8");
    // At most one of each per screen: the draft is the AI call the rail
    // narrates, the send is the write the ticker narrates, and a screen that
    // spelled either twice would narrate one action under two vocabularies.
    expect(count(src, '["email-draft",')).toBeLessThanOrEqual(1);
    expect(count(src, '["email",')).toBeLessThanOrEqual(1);
  });
});

function screensDir(): string {
  return join(dirname(fileURLToPath(import.meta.url)), "..", "screens");
}

/** Every screen that spells an email mutation key, in a stable order. */
function mailMutationSites(): string[] {
  return readdirSync(screensDir())
    .filter((file) => file.endsWith(".tsx") && !file.includes(".test."))
    .filter((file) => {
      const src = readFileSync(join(screensDir(), file), "utf8");
      return src.includes('["email-draft",') || src.includes('["email",');
    })
    .sort();
}

function count(haystack: string, needle: string): number {
  return haystack.split(needle).length - 1;
}
