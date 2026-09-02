import { readFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { de } from "../i18n/de";
import { en } from "../i18n/en";
import { vi } from "../i18n/vi";
import { NAMED, SAID, WROTE } from "./agentrail-copy";
import {
  ACTIVITY_LINE,
  displayedKinds,
  displayedLines,
  lineFor,
  NAMED_LINE,
  REASON_LINE,
  reasonFor,
} from "./ai-activity-lines";

/** Every message key the three maps name, which is the whole reachable set. */
function namedKeys(): Set<string> {
  return new Set<string>([
    ...displayedLines().flatMap(([, byState]) => Object.values(byState)),
    ...Object.values(NAMED_LINE).flatMap((byState) => Object.values(byState)),
    ...Object.values(REASON_LINE),
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

  // Every kind a person asks for ABOUT ONE RECORD has a named variant, because
  // that is the line the reader is waiting on: the rail exists to say what the
  // AI is drafting or reading, and "your reply" is not an answer to whom. The
  // scheduled runs are about no single record and are rightly absent here.
  // Listed rather than derived, because which kinds are "about one record" is a
  // fact about the product that no type carries.
  it("names the subject for every kind a person asks for about one record", () => {
    const aboutOneRecord = [
      "summarize",
      "draft_reply",
      "offer_draft",
      "growth_fit",
      "corpus_ask",
      "cold_start",
      "site_extract",
      "document_extract",
    ] as const;
    for (const kind of aboutOneRecord) {
      expect(NAMED_LINE[kind], kind).toBeDefined();
    }
    // The list above is also the CEILING, not only the floor: a kind added to
    // NAMED_LINE without a matching entry here would otherwise pass unnoticed,
    // and this list is the product's own statement of which kinds are about
    // one record.
    expect(Object.keys(NAMED_LINE).sort()).toEqual([...aboutOneRecord].sort());
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

describe("lineFor", () => {
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
    ["a task nothing has built", { kind: "nl_search", state: "done" }],
    ["the lane grading this build", { kind: "cert_judge", state: "done" }],
    // The two reasons carried by an entry of its own rather than a shared
    // constant, which is the pairing a hand-kept list drops first.
    ["a pass that reaches nobody", { kind: "enrich", state: "done" }],
    ["a step of a website read", { kind: "site_triage", state: "running" }],
  ])("renders nothing at all for %s", (_name, item) => {
    expect(lineFor(item, (key) => en[key])).toBeNull();
  });
});

// One website read is ONE line. The read files an occurrence per lane it runs
// under its own correlation id, so the profile lane is drawn and the other two
// are steps of the same read: drawing all three would list one afternoon's read
// three times under three sentences.
//
// Identity, not text: the two steps share one reason OBJECT and it is theirs
// alone, so re-pointing a step at the sweeps' sentence — false whenever a human
// asked for the read, since compose binds that person as on_behalf_of and the
// occurrence lands in their own feed — fails here rather than passing on a
// reworded string. And the drawn lane is held drawn, because the version of this
// file that kept all three silent left a reader who started a deep read watching
// an orb at rest for the whole of it.
describe("a website read is narrated once", () => {
  const STEPS = ["site_fact_extract", "site_triage"] as const;
  const entryFor = (kind: (typeof STEPS)[number] | "site_extract" | string) =>
    ACTIVITY_LINE[kind as (typeof STEPS)[number]];

  it("draws the profile lane every human-requested read runs", () => {
    expect("notDisplayed" in entryFor("site_extract")).toBe(false);
    expect(NAMED_LINE.site_extract).toBeDefined();
  });

  it("keeps the two steps on one reason object of their own", () => {
    expect(new Set(STEPS.map(entryFor)).size).toBe(1);
    const theirs = entryFor(STEPS[0]);
    expect("notDisplayed" in theirs).toBe(true);
    expect("notDisplayed" in theirs ? theirs.notDisplayed.trim() : "").not.toBe(
      "",
    );
    const steps = new Set<string>(STEPS);
    const trespassers = Object.entries(ACTIVITY_LINE)
      .filter(([kind, entry]) => !steps.has(kind) && entry === theirs)
      .map(([kind]) => kind);
    expect(trespassers).toEqual([]);
  });
});

// Why a run stopped reaches the reader ONLY through REASON_LINE, translated and
// paired with a repair. Two directions: every sentinel the router can write has
// copy in every locale, and a value off the vocabulary draws nothing — never the
// raw token, never a message key.
describe("the reason a run stopped", () => {
  it.each([
    { locale: "en", catalog: en },
    { locale: "de", catalog: de },
    { locale: "vi", catalog: vi },
  ])("has copy for every sentinel ($locale)", ({ catalog }) => {
    expect(Object.keys(REASON_LINE).length).toBeGreaterThan(0);
    for (const [sentinel, key] of Object.entries(REASON_LINE)) {
      expect(catalog[key], sentinel).toBeTruthy();
    }
  });

  it("renders the sentence for a known sentinel and nothing for the rest", () => {
    expect(
      reasonFor({ degrade_reason: "provider_quota" }, (key) => en[key]),
    ).toBe(en["agent.activity.reason.providerQuota"]);
    expect(reasonFor({ degrade_reason: null }, (key) => en[key])).toBeNull();
    expect(
      reasonFor(
        { degrade_reason: "brief_partial: crm_read_timeout" },
        (key) => en[key],
      ),
    ).toBeNull();
    // Off the wire, a bare lookup would answer from the prototype.
    expect(
      reasonFor({ degrade_reason: "constructor" }, (key) => en[key]),
    ).toBeNull();
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
      // The AI reading up on a company for the person who asked: the website
      // behind the Enrich card, the answer to a corpus question, the growth
      // fit, the deep website read. Each is personal — the handler runs under
      // the asker's own principal, or compose binds them as on_behalf_of — and
      // each names its record, so the line says WHICH company or corpus.
      "cold_start",
      "corpus_ask",
      "document_extract",
      "draft_reply",
      "growth_fit",
      "morning_brief",
      "offer_draft",
      "overnight_at_risk_sweep",
      "site_extract",
      "summarize",
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
    // The deep read: the ticker used to narrate the click that started it, and
    // the rail now narrates the reading itself, named for the site, for as
    // long as the AI is at it.
    ["site-read", "site_extract"],
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
  const SITES = ["../screens/compose.tsx", "../screens/persondrawers.tsx"];

  it.each(SITES)("keeps drafts off the send key in %s", async (rel) => {
    const src = await readSource(rel);
    // One draft and one send per screen: the draft is the AI call the rail
    // narrates, the send is the write the ticker narrates.
    expect(count(src, '["email-draft",')).toBe(1);
    expect(count(src, '["email",')).toBe(1);
  });
});

function count(haystack: string, needle: string): number {
  return haystack.split(needle).length - 1;
}

async function readSource(rel: string): Promise<string> {
  const here = dirname(fileURLToPath(import.meta.url));
  return readFile(join(here, rel), "utf8");
}
