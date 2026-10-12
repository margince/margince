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
  // Markup with placeholders, which is the same in every language.
  "signatureTemplate.placeholder",
  // Latency percentiles and OpenRouter's own name read the same in every
  // language, as does a raw key = value line the summary falls back to.
  "aiFigures.col.p50",
  "aiFigures.col.p95",
  "aiFigures.line.p50",
  "aiServing.openRouter",
  "aiServing.openRouterDocs",
  "aiServing.say.raw",
  // The name of the network, offered as a profile field in the research drawer.
  // "LinkedIn" is the brand and is written the same in every catalog; the other
  // six field labels beside it are translated normally.
  "contact.research.field.linkedin",
  // Quoted words between quotation marks, which Vietnamese writes as English
  // does; only the words inside change, and they are the speaker's own.
  "commitment.quote",
  "filters.propose.unusedItem",
  // The product name of the buyer surface, on the card that names it and on
  // the tab that opens it.
  "room.card.title",
  "tab.dealRoom",
  "buyer.eyebrow",
  // Sales nouns Vietnamese borrows: vi writes "Tên deal" and "Pipeline" elsewhere,
  // so translating only these drill-through headers would split the vocabulary.
  "explain.col.record",
  "explain.col.pipeline",
  // The settings group heading. "AI" is the initialism Vietnamese uses too —
  // the vi catalog already carries it untranslated wherever it appears in a
  // sentence, so rendering "Trí tuệ nhân tạo" on one nav heading would give
  // that one surface a vocabulary the rest of the product does not use.
  "settings.group.ai",
  // Vendor and service names on the AI provider rows and the service picker:
  // each is the vendor's own brand, written the same in every catalog.
  "aiProviders.name.anthropic",
  "aiProviders.name.openai",
  "aiProviders.name.gemini",
  "aiProviders.name.jev",
  "aiProviders.name.ollama",
  "aiProviders.name.vllm",
  "aiProviderSettings.service.openrouter",
  "aiProviderSettings.service.openrouterEu",
  "aiProviderSettings.service.mistral",
  "aiProviderSettings.service.together",
  "aiProviderSettings.service.groq",
  "aiProviderSettings.service.deepseek",
  "aiProviderSettings.service.langdockEu",
  "aiProviderSettings.service.langdockUs",
  "aiProviderSettings.service.googleAiStudio",
  "aiProviderSettings.service.anthropic",
  "aiProviderSettings.service.openai",
  // Two signed counts and a slash, with no word to translate. Its spoken
  // form, lists.pulse.label, is translated normally.
  "lists.pulse.chip",
  // The area's name, which is the same word in all three catalogs by decision:
  // "Analytics" is what the product calls this surface, and both German and
  // Vietnamese borrow it as a term of art rather than translating it. The
  // section labels UNDER it are translated normally.
  "nav.analytics",
  // The release channel's own name on the rail head's stage marker. "Beta" is
  // the word all three catalogs use for it — Vietnamese borrows it as a term of
  // art the way it borrows "deal" and "pipeline" — and a marker four letters
  // long is also the only spelling that fits the 56px rail it must survive.
  // Temporary, with the badge that renders it: app/betabadge.tsx names this
  // entry among the things its deletion takes.
  "shell.beta",
  // Two placeholders and a colon. The field name is already translated one
  // level down (factFieldLabelKey) and the value is the page's own word, so
  // there is nothing left in this string for a locale to translate either.
  "ob.scan.tickerFact",
  // Two whole clauses and the space between them. The weekly's opening sentence
  // is built from result keys and carry keys that ARE translated; this joins the
  // two rendered clauses and contributes no word of its own, so a locale has
  // nothing here to change.
  "brief.week.andCarry",
  // The record's own name beside the numeral that names its reading. There is
  // no word in it to translate — a locale that changed it would be changing
  // the account's name.
  "co.360.subject",
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
  "scheduling.locationExample",
  "aiRouting.baseUrl.placeholder",
  "aiRouting.baseUrl.placeholder.jev",
  "aiRouting.baseUrl.placeholder.jevCompatible",
  "aiRouting.baseUrl.placeholder.gemini",
  "aiRouting.baseUrl.placeholder.anthropic",
  "aiRouting.baseUrl.placeholder.openai",
  // A pattern of placeholders with no words in it, and the EU's own
  // abbreviation, which Vietnamese writes the same way.
  "aiRouting.location.option",
  "aiRouting.location.optionBare",
  "aiRouting.location.group.eu",
  // The same noun, captioning a staged proposal's email field.
  "approval.field.email",
  // And naming the kind of evidence a tag suggestion cites.
  "tagSuggestion.kind.email",
  // Vietnamese sales usage keeps "pipeline" as the loanword, the same way it
  // keeps "Email". German translates it, and does.
  "deal.forecast.pipeline",
  "room.create.defaultTitle",
  // Pure punctuation layouts: every word in them is a placeholder, so there is
  // nothing to translate and a "translation" could only reorder the slots.
  "dealSuggestion.name",
  "lead.sla.answeredAt",
  "projectFiling.decision",
  // A filter clause's slots, a group's brackets, a value not given yet.
  "filters.sentence.clause",
  "filters.sentence.clauseBare",
  "filters.sentence.group",
  "filters.sentence.pendingValue",
  // Two phase names and an arrow.
  "project.history.moved",
  "brief.digestPhaseChange",
  // Two relationship-band names and an arrow, on the contact strip's latest
  // change. Same shape, same reason as the two above.
  "contact.intro.change.buckets",
  // A filename and the server's own refusal, separated by a colon. It is one
  // line of a list whose heading says what the list is, and both halves arrive
  // already in the reader's own words.
  "knowledge.upload.refused",
  // The sources a day could not read, joined into one line. Every phrase in it
  // is built from its own translated key (worklist.source.failed /
  // .withheld), so this value is the placeholder and a full stop.
  "worklist.partial",
  // Brand and provider names: proper nouns, not translated in any locale.
  "connectors.provGmail",
  "connectors.provGcal",
  "connectors.provGraph",
  "connectors.provTelegram",
  "ob.s4.provGoogle",
  "ob.s4.provMicrosoft",
  "ob.conv.connect.linkedinName",
  "magic.by.system",
  // A number and a plus sign, written alike in every locale.
  "lead.signal.employees.201+",
  // The same proper noun as connectors.provGmail and its neighbours, one
  // surface over.
  "provider.profile.linkedin",
  "contact.page.linkedin",
  "ob.ai.speakerName",
  "auth.title",

  // "deal", "pipeline" and "lead" are loanwords Vietnamese keeps.
  "deals.pipeline",
  "deal.fcPipeline",
  "filters.field.pipeline_id",
  "analytics.field.pipeline_id",
  "analytics.sectionPipeline",
  "reporting.pipeline",
  "lead.qualify.pipeline",
  "stageAutomation.pipeline",
  "review.colDeal",
  "worklist.category.leads",
  "filters.sentence.ref.pipeline_one",
  "cf.obj.deal",
  "cf.obj.lead",
  "co.brief.cite.deal",
  // The borrowed noun in the singular, which English and Vietnamese spell alike;
  // the plural arms differ because English pluralises.
  "today.scan.readDeals_one",
  "analytics.forecastDeals_one",
  "filters.library.records.deal_one",
  "filters.library.records.lead_one",
  "leadSources.leads_one",
  "acqSources.deals_one",
  "contracts.renew.deal",
  "contracts.deal",

  // The forecast categories Commit and Best case keep their English names.
  "deal.fcCommit",
  "deal.fcBestCase",
  "brief.weekly.outlook.bestCase",

  // Endonyms: a locale's own name for itself, identical in every catalog.
  "locale.name.en",
  "locale.name.de",
  "locale.name.vi",

  // Field labels where the English word is also the Vietnamese usage.
  "contacts.email",
  "users.emailLabel",
  "create.email",
  "restricted.kind.email",
  "timeline.filters.kind.email",
  "auth.email",
  "contact.action.email",
  "contact.memory.email",
  "contact.memory.channelEmail",
  "contact.rail.email",
  "history.field.email",
  "settings.voice.register.email",
  "product.sku",
  "compose.cc",
  "compose.bcc",
  "compose.transportEmail",
  "passport.select",

  // Placeholders, examples and other machine-shaped literals: emails,
  // URLs, hostnames — content a translation would corrupt, not prose.
  "users.emailPlaceholder",
  "consumerMail.domainPlaceholder",
  "consumerMail.baselinePlaceholder",
  "linkedinImport.profilePlaceholder",
  "ob.conv.linkedin.profilePlaceholder",
  "ob.s4.imapHostPlaceholder",
  "ob.s4.imapEmail",
  "ob.conv.clarify.question",
  "ob.conv.clarify.optionDetail",
  "create.linkedin",
  "contact.enriched.field.linkedin",

  // Units, version rows and other format-only strings: symbols/abbreviations
  // that do not translate (ms, a version-row template).
  "aicalls.ms",
  "voice.history.versionRow",
  "voice.history.deltaRow",
  "ob.conv.triage.omittedField",
  "share.ceiling.post",

  // Tab and section labels that are proper nouns in the product. The settings
  // ENTRY for the voice surface is no longer one of them — it was renamed off
  // the feature name to "Voice", which vi translates.
  "settings.voice.title",
  "co.decisions.group",

  // "Lead" is the loanword in both de and vi — every other lead key in this
  // catalog leaves it untranslated, and the marker on the record page names
  // the same object those keys do.
  "lead.marker",
  // The singular kind name on a search hit: "Deal" and "Lead" are loanwords, and
  // only the plural headings differ because English pluralises.
  "search.kind.deal",
  "search.kind.lead",
  // The lead rail's own deal card title, the same singular loanword as
  // search.kind.deal above it.
  "lead.rail.deal.title",

  // "Cc" is the mail header itself, which vi writes as the Latin abbreviation
  // exactly as en does. Translating it would name a field no mail client
  // labels that way.
  "email.detail.cc",

  // Other cases verified individually against the source.
  "shell.logoAria",
  "ob.conv.connect.scopeMicrosoft",

  // Worked examples of a German commercial register entry, the shape the
  // legal-notice read actually parses regardless of which locale a reader
  // picked. A "translated" court name or register prefix would show an
  // example that does not match what the product reads.
  "ob.fieldEg.legal_name",
  "ob.fieldEg.registered_address",
  "ob.fieldEg.register_vat",
  "ob.fieldEg.legal_form",
  "ob.fieldEg.register_court",
  "ob.fieldEg.register_number",
  // A fixture company name, which a locale would rename into a different company.
  "ob.fieldEg.display_name",

  // Brand names, a protocol's acronym, and the console Google itself names
  // in English.
  "firstRun.platform.google",
  "firstRun.platform.microsoft",
  "firstRun.platform.imap",
  "firstRun.google.helpConsole",
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
    translate("en", "contact.strip.days", { count: 96 });
    expect(translate("en", "contact.strip.days", { count: "96" })).toContain(
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

  // The allowlist is the one hand-written list in this file, and a key deleted
  // or translated since leaves an exemption behind that nothing else notices.
  it("the untranslated-copy allowlist names only keys vi still copies from en", () => {
    const reference: Record<string, string> = en;
    const translated: Record<string, string> = viCatalog;
    const stale = [...KEPT_IN_ENGLISH].filter(
      (key) => !(key in en) || translated[key] !== reference[key],
    );
    expect(stale, `stale allowlist entries: ${stale.join(", ")}`).toEqual([]);
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

/*
 * The tenant is called "company" to a reader (ADR-0061): one
 * installation serves one company, and "workspace" is the internal
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
  it("no catalog calls the company a workspace", () => {
    for (const [locale, catalog] of Object.entries(catalogs)) {
      for (const [key, value] of Object.entries(catalog)) {
        expect(
          TENANT_MISNOMER.test(value.replace(PRODUCT_NAME, "")),
          `${locale}: ${key} — "${value}" says workspace where the product says company`,
        ).toBe(false);
      }
    }
  });
});
