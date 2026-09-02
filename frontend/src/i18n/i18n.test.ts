import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { en } from "./en";
import {
  catalogs,
  DEFAULT_LOCALE,
  detectLocale,
  LOCALES,
  localeNameKey,
  translate,
} from "./index";
import { vi as viCatalog } from "./vi";

// Keys whose vi value is identical to en on purpose. Derived by comparing
// every key's vi and en value (not guessed): anything identical
// to en that is NOT listed here is a key the translation pass missed, which
// no other check in this file catches. Grouped so a reviewer can tell "brand
// name" from "missed translation" at a glance — an addition to any group
// must be defensible on the same grounds as its neighbours.
const KEPT_IN_ENGLISH = new Set<string>([
  // The product name of the buyer surface.
  "room.card.title",
  // The area's name, which is the same word in all three catalogs by decision:
  // "Analytics" is what the product calls this surface, and both German and
  // Vietnamese borrow it as a term of art rather than translating it. The
  // section labels UNDER it are translated normally.
  "nav.analytics",
  // Two placeholders and a dash. Every word in the line comes from elsewhere —
  // the dimension's own label and the sentence the server wrote — so there is
  // nothing here for a locale to translate.
  "co.strip.healthSummary.because",
  // The record's own name beside the numeral that names its reading. There is
  // no word in it to translate — a locale that changed it would be changing
  // the account's name.
  "co.360.subject",
  // A number and the SI symbol for millisecond. The symbol is the same in every
  // language by definition — it is written "ms" in Vietnamese too — so a locale
  // that changed it would be naming a different unit.
  "aiHealth.ms",
  // An acronym, not a word: DNS is DNS in every language this product speaks,
  // and a "translation" of it would be a different protocol.
  "co.tech.lane.dns",
  // Vietnamese uses "Email" for the noun; German has its own spelling and
  // carries it. Only the vi value matches English, and it is the right word.
  "dealmail.title",
  // Same word, same reason, on the generic record mail box every other page
  // shares.
  "recordmail.title",
  // Same word, same reason: the exchange kind on the account's recent list.
  "co.recent.kind.email",
  // And on the record's chronology, for the same reason again.
  "timeline.kind.email",
  // And on the thread across the top of a record, third spelling of the same
  // noun.
  "co.spine.kind.email",
  // Same word again, this time the confirm page's own field label.
  "confirm.field.email",
  // The vendors' own field names. An admin reads these off the Google Cloud
  // console or the Entra portal, which show them in English whatever the
  // reader's locale, so translating them here would have the form ask for
  // something the page they are copying from does not call by that name. The
  // placeholders are id SHAPES rather than prose and are the same string
  // everywhere.
  "oauthApp.clientId",
  "oauthApp.clientSecret",
  "oauthApp.google.clientIdPlaceholder",
  "oauthApp.microsoft.clientIdPlaceholder",
  "oauthApp.tenant",
  "oauthApp.tenantPlaceholder",
  // A URL, which is the same string in every language.
  "aiRouting.baseUrl.placeholder",
  // The same noun, captioning a staged proposal's email field.
  "approval.field.email",
  // Vietnamese sales usage keeps "pipeline" as the loanword, the same way it
  // keeps "Email". German translates it, and does.
  "deal.forecast.pipeline",
  "persondealrooms.title",
  "room.create.defaultTitle",
  "buyer.poweredBy",
  "buyer.poweredByMargince",
  // TEMPORARY, with the release marker it labels (app/shell.tsx): "Alpha" is
  // the release stage's own name and Vietnamese keeps it, the same way it keeps
  // "Email" and "pipeline". Delete this entry with the marker.
  "shell.alpha",
  // A placeholder and a percent sign. Vietnamese writes a percentage the way
  // English does — digits then the sign, no space — so the value is identical by
  // agreement rather than by omission. German differs (it takes the space) and
  // carries its own.
  "home.pct",
  // Pure punctuation layouts: every word in them is a placeholder, so there is
  // nothing to translate and a "translation" could only reorder the slots.
  // Two phase names and an arrow.
  "project.history.moved",
  "home.digestPhaseChange",
  // An em dash standing in for a figure nobody can compute yet. A glyph, not a
  // word — the sentence explaining it is the detail line beside it.
  "co.strip.financeUnknown",
  // Brand and provider names: proper nouns, not translated in any locale.
  "connectors.provGmail",
  "connectors.provGcal",
  "connectors.provGraph",
  "connectors.provTelegram",
  "ob.s4.provGoogle",
  "ob.s4.provMicrosoft",
  "ob.conv.connect.linkedinName",
  // "Email" is the loanword vi uses for the field, as en spells it.
  // Employee-count bands: digits and an en dash, the same in every locale.
  "lead.signal.employees.1-10",
  "lead.signal.employees.11-50",
  "lead.signal.employees.51-200",
  "lead.signal.employees.201+",
  // "Email" is the Vietnamese word for email. de has "E-Mail" and differs;
  // vi does not, and inventing a difference would name the transport
  // something no Vietnamese speaker calls it.
  "person.composer.transportEmail",
  // The same proper noun as connectors.provGmail and its neighbours, one
  // surface over.
  "provider.profile.linkedin",
  "person.page.linkedin",
  "auth.coreProviderAnthropic",
  "auth.coreProviderGemini",
  "auth.coreProviderOllama",
  "auth.coreProviderOpenAI",
  "auth.coreProviderVllm",
  "overlay.userMap.principal.hubspot",
  "overlay.regionEu1",
  "overlay.budgetSources",
  "ob.ai.speaker",
  "ob.ai.speakerName",
  "auth.title",

  // The RFC 5322 header name. "Bcc" is the field's identity in every mail
  // client in every locale — a translated label would name a field the
  // recipient's own client does not call that, and the placeholder beside it
  // is what carries the meaning.
  "person.composer.bcc",

  // CRM domain nouns kept in English by design (glossary, design.md §6.1):
  // "deal", "pipeline", "timeline" etc. read the same in Vietnamese usage.
  "nav.deals",
  "tab.deals",
  "deals.pipeline",
  "deal.fcPipeline",
  "cf.obj.deal",
  "cf.obj.person",
  "cf.obj.lead",
  "co.brief.cite.deal",
  "co.brief.cite.person",
  "deals.unit",
  "history.actorAgent",

  // The alphabetical sort view: the Vietnamese alphabet also runs A to Z, so
  // the label names the same range in either catalog.
  "list.viewAZ",

  // Endonyms: a locale's own name for itself, identical in every catalog.
  "locale.name.en",
  "locale.name.de",
  "locale.name.vi",

  // Field labels where the English word is also the Vietnamese usage.
  "people.email",
  "create.email",
  "restricted.kind.email",
  "timeline.filters.kind.email",
  "auth.email",
  "person.identity.email",
  "person.action.email",
  "person.memory.email",
  "person.memory.channelEmail",
  "person.rail.email",
  "history.field.email",
  "settings.voice.register.email",
  "product.sku",
  "compose.cc",
  "settings.token",
  "passport.select",

  // Placeholders, examples and other machine-shaped literals: emails,
  // URLs, hostnames — content a translation would corrupt, not prose.
  "auth.emailPlaceholder",
  "users.emailPlaceholder",
  "consumerMail.domainPlaceholder",
  "consumerMail.baselinePlaceholder",
  "linkedinImport.profilePlaceholder",
  "ob.conv.linkedin.profilePlaceholder",
  "ob.s4.imapHostPlaceholder",
  "ob.s4.imapEmail",
  "ob.url",
  "ob.urlScheme",
  "ob.conv.triage.companyWebsite",
  "ob.conv.clarify.question",
  "ob.conv.clarify.optionDetail",
  "create.linkedin",
  "person.enriched.field.linkedin",

  // Units, version rows and other format-only strings: symbols/abbreviations
  // that do not translate (ms, a version-row template).
  "aicalls.ms",
  "voice.history.versionRow",
  "voice.history.deltaRow",
  "ob.conv.triage.omittedField",
  "ob.rail.tokensUnit",
  "share.ceiling.post",

  // Tab and section labels that are proper nouns in the product. The settings
  // ENTRY for the voice surface is no longer one of them — it was renamed off
  // the feature name to "Voice", which vi translates.
  "settings.voice.title",
  "co.decisions.group",
  "partner.role.hosting",

  // Actor labels built on "Agent" and "Connector", which vi carries as
  // loanwords everywhere else in this catalog — translating them only here
  // would make the same actor read as two different things.
  "trust.agentTag",
  "consent.actorAgent",
  "consent.actorConnector",
  "users.agentSeat",
  // Same reason, one level up: vi carries "AI" as the loanword throughout this
  // catalog, so spelling out "trí tuệ nhân tạo" on the settings entry alone
  // would make one subject read as two.
  "settings.tab.ai",
  // "Lead" is the loanword in both de and vi — every other lead key in this
  // catalog leaves it untranslated, and the marker on the record page names
  // the same object those keys do.
  "lead.marker",

  // Other cases verified individually against the source.
  "shell.logoAria",
  "ob.conv.connect.scopeMicrosoft",
]);

// Every invariant below derives from `catalogs`, so a locale added to the
// registry is covered without editing this file. That is the point: a
// hand-maintained locale list is a list that drifts.

function placeholders(message: string): string[] {
  return [...message.matchAll(/\{(\w+)\}/g)].map((match) => match[1]).sort();
}

describe("i18n catalogs", () => {
  it("LOCALES lists exactly the registered catalogs", () => {
    expect([...LOCALES].sort()).toEqual(Object.keys(catalogs).sort());
  });

  it("every catalog has exact key parity with en", () => {
    const expected = Object.keys(en).sort();
    for (const [locale, catalog] of Object.entries(catalogs)) {
      expect(Object.keys(catalog).sort(), locale).toEqual(expected);
    }
  });

  it("no catalog value is empty", () => {
    for (const [locale, catalog] of Object.entries(catalogs)) {
      for (const [key, value] of Object.entries(catalog)) {
        expect(value.trim(), `${locale}: ${key}`).not.toBe("");
      }
    }
  });

  // A translation that drops {count} passes key parity, passes the non-empty
  // check, compiles, and ships a label with a hole in it. Nothing else catches it.
  it("every catalog carries the same placeholders as en", () => {
    const reference: Record<string, string> = en;
    for (const [locale, catalog] of Object.entries(catalogs)) {
      for (const [key, value] of Object.entries(catalog)) {
        expect(placeholders(value), `${locale}: ${key}`).toEqual(
          placeholders(reference[key]),
        );
      }
    }
  });

  // An endonym is a language's name in its OWN language, so it is the same
  // string in every catalog: the German switcher says "Tiếng Việt" too. Both
  // loops run over LOCALES — proven above to be exactly the registered
  // catalogs — so a pair compared by hand cannot leave a third catalog free to
  // translate a name it should have carried verbatim. The untranslated-leftover
  // check below cannot stand in for this one: it only flags values EQUAL to
  // English, and a translated endonym differs from English by definition.
  it("every locale has a name key, and names are endonyms shared by all catalogs", () => {
    for (const named of LOCALES) {
      const key = localeNameKey(named);
      const endonym = translate(DEFAULT_LOCALE, key);
      expect(endonym.trim(), key).not.toBe("");
      for (const reader of LOCALES) {
        expect(translate(reader, key), `${reader}: ${key}`).toBe(endonym);
      }
    }
  });

  it("every catalog interpolates {params}", () => {
    for (const locale of LOCALES) {
      const rendered = translate(locale, "trust.agentTag", {
        agent: "capture",
      });
      expect(rendered, locale).toContain("capture");
      expect(rendered, locale).not.toContain("{agent}");
    }
  });

  it("an unknown placeholder is left visible, never silently dropped", () => {
    expect(translate("en", "trust.agentTag", {})).toBe("Automated by {agent}");
  });

  it("refuses a raw number as a param, because nobody would have grouped it", () => {
    // The gate for locale-blind figures, and it is the COMPILER rather than a
    // sweep: a number handed to a catalog sentence reaches it through string
    // coercion, which renders "1234" for a German reader whose every other
    // figure on the page reads "1.234". Narrowing this parameter to strings
    // makes every such site a build failure, so a new one cannot be written.
    //
    // Asserted here because the narrowing is invisible in the rendered output —
    // it is the kind of type that gets widened back by the next author who
    // meets it as an inconvenience, and nothing else would notice.
    // @ts-expect-error a magnitude must be formatted (format/format.ts) first
    translate("en", "person.strip.days", { count: 96 });
    expect(translate("en", "person.strip.days", { count: "96" })).toContain(
      "96",
    );
  });

  it("the default locale is en (A100: en-GB)", () => {
    expect(DEFAULT_LOCALE).toBe("en");
  });

  // Every other check in this file happily accepts a value that is just the
  // English string copied verbatim: key parity, non-empty and placeholder
  // parity all pass on an untranslated leftover. This is the one check that
  // actually proves the vi catalog was translated, not merely typed out.
  it("no vi value is an untranslated copy of en, outside the allowlist", () => {
    const reference: Record<string, string> = en;
    const leftovers = Object.entries(viCatalog)
      .filter(
        ([key, value]) => value === reference[key] && !KEPT_IN_ENGLISH.has(key),
      )
      .map(([key]) => key);
    expect(leftovers, `untranslated keys: ${leftovers.join(", ")}`).toEqual([]);
  });

  // The allowlist above is the one hand-written list in this file, and a key
  // deleted from the catalogs leaves its entry behind silently — an exemption
  // for a string that no longer exists, which the next reader has to research
  // before they can tell it is stale.
  it("the untranslated-copy allowlist names only keys that still exist", () => {
    const stale = [...KEPT_IN_ENGLISH].filter((key) => !(key in en));
    expect(stale, `allowlist entries with no key: ${stale.join(", ")}`).toEqual(
      [],
    );
  });
});

describe("browser-language detection", () => {
  it("picks the first supported language, region-insensitive", () => {
    expect(detectLocale(["en-US"])).toBe("en");
    expect(detectLocale(["de-AT", "en"])).toBe("de");
    expect(detectLocale(["EN-GB"])).toBe("en");
  });

  it("recognises Vietnamese, with or without a region", () => {
    expect(detectLocale(["vi-VN"])).toBe("vi");
    expect(detectLocale(["vi"])).toBe("vi");
  });

  it("skips unsupported languages to the first one we ship", () => {
    expect(detectLocale(["fr-FR", "es", "en-US"])).toBe("en");
  });

  it("falls back to the A100 default when nothing matches or the list is empty", () => {
    expect(detectLocale(["fr", "ja"])).toBe(DEFAULT_LOCALE);
    expect(detectLocale([])).toBe(DEFAULT_LOCALE);
  });

  it("never matches an inherited Object property", () => {
    expect(detectLocale(["constructor"])).toBe(DEFAULT_LOCALE);
    expect(detectLocale(["toString"])).toBe(DEFAULT_LOCALE);
  });
});

// The two guards below read the source tree rather than a written list of
// what is allowed, for the reason every fitness function here does: a list
// records one moment's answer and then drifts, while the tree is the answer.

const SRC_ROOT = dirname(dirname(fileURLToPath(import.meta.url)));

const CATALOG_FILES = new Set(
  ["en.ts", "de.ts", "vi.ts"].map((file) => join(SRC_ROOT, "i18n", file)),
);

/**
 * Every file that can RENDER a key: the whole source tree minus the catalogs
 * themselves and minus the tests.
 *
 * Tests are excluded deliberately. A key a test names but no screen renders is
 * exactly the dead weight the orphan check looks for, and counting tests would
 * also let this file's own allowlist vouch for keys nothing displays.
 */
function renderingFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) {
      return renderingFiles(path);
    }
    const rendersKeys =
      /\.tsx?$/.test(entry) &&
      !/\.test\.tsx?$/.test(entry) &&
      !CATALOG_FILES.has(path);
    return rendersKeys ? [path] : [];
  });
}

const RENDERING_SOURCE = renderingFiles(SRC_ROOT).map((path) =>
  readFileSync(path, "utf8"),
);

// A key written out in full, in any of the three quote styles. The character
// class is the alphabet the catalogs actually use: the enum-shaped keys carry
// ':' (lead.factor.manual:employees), '-' and '+' (the employee bands).
const QUOTED_LITERAL = /["'`]([A-Za-z0-9_.:+-]+)["'`]/g;

// The stem of a key built at runtime — t(`ob.readStatus.${status}`). Whatever
// follows the stem is a value this file cannot see, so every key under it
// counts as rendered: the alternative is a guard that tells the next person to
// delete a string a screen is displaying.
const TEMPLATE_STEM = /`([A-Za-z0-9_.]*)\$\{/g;

function renderedLiterals(): Set<string> {
  const literals = new Set<string>();
  for (const source of RENDERING_SOURCE) {
    for (const [, literal] of source.matchAll(QUOTED_LITERAL)) {
      literals.add(literal);
    }
  }
  return literals;
}

function renderedStems(): string[] {
  const stems = new Set<string>();
  for (const source of RENDERING_SOURCE) {
    for (const [, stem] of source.matchAll(TEMPLATE_STEM)) {
      // A dot is what makes a stem a key stem. Without one the template is a
      // class name, a URL or a message, and treating it as a stem would vouch
      // for the entire catalog.
      if (stem.includes(".")) {
        stems.add(stem);
      }
    }
  }
  return [...stems];
}

/**
 * The plural bases this catalog carries: every key with an `_one` arm whose
 * `_other` arm is here too.
 *
 * A plural base is a key stem like any other, reached the same way a template
 * stem is — `plural("share.teamMembers", n)` renders `share.teamMembers_one` or
 * `_other` and writes neither in full. Without this, every arm of every plural
 * pair reads as an orphan and this gate tells the next person to delete
 * ninety-four strings the product is displaying.
 *
 * DERIVED from the catalog rather than listed, and derived from the PAIR rather
 * than from a suffix: `x_one` alone vouches for nothing, because half a
 * translated pair is the orphan this check exists to find.
 */
function pluralBases(): Set<string> {
  const keys = new Set(Object.keys(en));
  const bases = new Set<string>();
  for (const key of keys) {
    const base = key.endsWith("_one") ? key.slice(0, -"_one".length) : null;
    if (base !== null && keys.has(`${base}_other`)) {
      bases.add(base);
    }
  }
  return bases;
}

describe("catalog keys against the surfaces that render them", () => {
  it("every key is rendered by a source file, literally or under a stem", () => {
    const literals = renderedLiterals();
    const stems = renderedStems();
    const bases = pluralBases();
    // An arm is rendered when its BASE is: the call site names the base and the
    // reader's own plural rule picks the arm, so the arm's full key never
    // appears in source at all.
    //
    // Only the two ARMS get this, not any key whose last underscore happens to
    // follow a plural base. Cutting at the last `_` exempted
    // `<renderedBase>_anythingElse` as well, which is a hole in exactly the
    // direction that matters: this gate's job is to find a key nothing renders,
    // and an exemption that reaches further than the convention it models makes
    // it report PASS over one.
    const arms: readonly ["_one", "_other"] = ["_one", "_other"];
    const underPluralBase = (key: string): boolean => {
      const arm = arms.find((suffix) => key.endsWith(suffix));
      if (arm === undefined) {
        return false;
      }
      const base = key.slice(0, -arm.length);
      return bases.has(base) && literals.has(base);
    };
    const orphans = Object.keys(en).filter(
      (key) =>
        !literals.has(key) &&
        !underPluralBase(key) &&
        !stems.some((stem) => key.startsWith(stem)),
    );
    expect(
      orphans,
      `keys translated three times and rendered nowhere: ${orphans.join(", ")}`,
    ).toEqual([]);
  });
});

/*
 * The tenant is called "organization" to a reader (A107/ADR-0061): one
 * installation serves one organization, and "workspace" is the internal
 * boundary the schema and the RBAC code use. A second noun for one thing reads
 * as two concepts the product does not have.
 *
 * Matched against VALUES only. Key names keep the internal spelling on purpose
 * — jobs.workspaceKinds, extUnits.workspace.*, captureActivity.scope.workspace
 * — because a key is an identifier the screens import, not copy anybody reads.
 *
 * "Google Workspace" is the one legitimate reading of the word: a product name
 * rather than the tenant, so the guard drops it before it looks.
 */
const TENANT_MISNOMER = /workspace|arbeitsbereich|không gian làm việc/i;
const PRODUCT_NAME = /Google Workspace/g;

describe("the product's word for the tenant", () => {
  it("no catalog calls the organization a workspace", () => {
    for (const [locale, catalog] of Object.entries(catalogs)) {
      for (const [key, value] of Object.entries(catalog)) {
        expect(
          TENANT_MISNOMER.test(value.replace(PRODUCT_NAME, "")),
          `${locale}: ${key} — "${value}" says workspace where the product says organization`,
        ).toBe(false);
      }
    }
  });
});
