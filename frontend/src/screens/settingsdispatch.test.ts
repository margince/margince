import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { SETTINGS_PAGES } from "./settingscatalog";

// Every catalog page renders something, and nothing renders that is not a page.
//
// The catalog decides which pages EXIST and `tabContent` decides what each one
// draws, and the two are separate declarations. TypeScript catches half of it —
// a `case` naming an id the union does not carry is an error — but it cannot
// catch the half that matters more: a page declared in the catalog with no arm
// in the switch. That page appears in navigation, is reachable by address, and
// renders nothing. A reader gets a blank column and no explanation.
//
// Read as source text rather than by calling tabContent, because calling it
// needs React, a query client and a fetch stub per page; what is being asserted
// is that the two LISTS agree, which is a fact about the files.

const here = dirname(fileURLToPath(import.meta.url));

function dispatchedIds(): Set<string> {
  const source = readFileSync(join(here, "settings.tsx"), "utf8");
  const start = source.indexOf("export function tabContent");
  expect(start).toBeGreaterThan(-1);
  // The switch ends at the first line-start `}` after it — the function's own
  // closing brace. Bounding the slice matters: `case "won":` appears further
  // down this file in an unrelated switch, and an unbounded scan would count it.
  const end = source.indexOf("\n}\n", start);
  expect(end).toBeGreaterThan(start);
  const body = source.slice(start, end);
  return new Set(
    [...body.matchAll(/case "([a-z-]+)":/g)].map((match) => match[1]),
  );
}

// Every card the old sixteen-entry register reached, written out from
// `git show origin/main` at the time of the split.
//
// A card silently dropped during a page split is the one defect none of the
// other gates can see: the dispatch still covers every page, every page still
// renders SOMETHING, and a surface a reader used yesterday is simply gone. The
// split moved eight cards out of a Data model wrapper and seven out of an AI
// tab strip, which is exactly the shape that loses one.
const CARDS_THE_REGISTER_REACHED = [
  "AccountCard",
  "AutonomySettingsCard",
  "VoiceDnaCard",
  "AgentsTab",
  "InstallationSettingsCard",
  "FxRatesCard",
  "CompanyContextCard",
  "SignInMethodsCard",
  "OAuthAppCard",
  "ExtensionAccessCard",
  "UsersAdminCard",
  "TeamsCard",
  "ConnectionsTab",
  "CaptureActivityTab",
  "IntegrationsTab",
  "OwnDomainsCard",
  "CaptureSettingsCard",
  "ConsumerMailDomainsCard",
  "BlockedDomainsCard",
  "CustomFieldsAdmin",
  "TagVocabularyCard",
  "PipelinesCard",
  "LeadSourcesCard",
  "LeadDisqualifyReasonsCard",
  "LeadHandlingCard",
  "ProductsAdmin",
  "OfferTemplatesAdmin",
  "KnowledgeCard",
  "AiRoutingCard",
  "AiProviderKeysCard",
  "AiHealthCard",
  "AutomationsAdmin",
  "AiUsageCard",
  "ModelCostsCard",
  "AiCallsCard",
  "ConsentPurposesCard",
  "RetentionCard",
  "RestrictedRecordsCard",
  "PrivacyInboxCard",
  "AuditLogCard",
  "LicenseCard",
  "ImportCard",
  "EmbedReindexCard",
  "JobHealthCard",
  "ResetDataCard",
];

describe("the split lost no card", () => {
  function renderedComponents(): Set<string> {
    const source = readFileSync(join(here, "settings.tsx"), "utf8");
    const start = source.indexOf("export function tabContent");
    const end = source.indexOf("\n}\n", start);
    return new Set(
      [...source.slice(start, end).matchAll(/<([A-Z][A-Za-z]+)\b/g)].map(
        (match) => match[1],
      ),
    );
  }

  it("still reaches every card the register reached", () => {
    const rendered = renderedComponents();
    const lost = CARDS_THE_REGISTER_REACHED.filter(
      (card) => !rendered.has(card),
    );
    expect(lost).toEqual([]);
  });

  it("names enough cards to make that meaningful", () => {
    // Two empty sets compare equal, and a scan that stopped matching would
    // report every card present by finding none of them.
    expect(renderedComponents().size).toBeGreaterThan(30);
  });
});

describe("the dispatch covers the catalog", () => {
  it("draws every page the catalog declares", () => {
    const dispatched = dispatchedIds();
    const missing = SETTINGS_PAGES.map((page) => page.id).filter(
      (id) => !dispatched.has(id),
    );
    expect(missing).toEqual([]);
  });

  it("draws nothing the catalog does not declare", () => {
    // The other direction: an arm left behind after a page was renamed or
    // dropped is dead code the type checker does catch, but only while the id
    // is absent from the union — a renamed page whose old id becomes a NEW
    // page's id would compile and dispatch the wrong content.
    const declared = new Set<string>(SETTINGS_PAGES.map((page) => page.id));
    const extra = [...dispatchedIds()].filter((id) => !declared.has(id));
    expect(extra).toEqual([]);
  });

  it("finds a realistic number of arms, so a broken scan cannot pass", () => {
    // Two empty sets compare equal. Without this, a regex or a slice that
    // stopped matching would report perfect agreement between nothing and
    // nothing — the census that fails short, which is the one way this gate
    // must not break.
    expect(dispatchedIds().size).toBe(SETTINGS_PAGES.length);
    expect(SETTINGS_PAGES.length).toBeGreaterThan(20);
  });
});
