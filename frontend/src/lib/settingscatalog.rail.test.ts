import { describe, expect, it } from "vitest";
import { meFixture } from "../testing/mefixture";
import { settingsReach, visibleSettingsPages } from "./settingscatalog";

// What a page's `changes` buys a real reader: which pages reach the rail, and
// which stay reachable from the settings home and search without cluttering it.
//
// Built on the SEEDED rep's grants rather than a hand-picked few. A partial
// fixture would answer a question about a fixture — the point of this case is
// what the product actually shows the role Lars ships, so a policy change that
// widens a rep shows up here as a failing list rather than silently.
describe("what the rail carries and what it leaves behind", () => {
  const seededRep = meFixture({
    roles: ["rep"],
    allow: {
      activity: ["create", "read", "update"],
      automation: ["read"],
      capture_settings: ["create", "read"],
      channel_connection: ["read"],
      commission: ["read"],
      computed_field: ["read"],
      contract: ["create", "read", "update"],
      custom_field: ["read"],
      deal: ["create", "read", "update"],
      deal_room: ["create", "read", "update"],
      finance: ["read"],
      forecast: ["read"],
      installation_settings: ["read"],
      integrations: ["read"],
      introduction: ["create", "read", "update"],
      knowledge_corpus: ["read"],
      knowledge_document: ["read"],
      lead: ["create", "read", "update", "delete"],
      list: ["create", "read", "update"],
      offer: ["create", "read", "update"],
      offer_template: ["create", "read", "update"],
      company: ["create", "read", "update"],
      partner: ["read"],
      contact: ["create", "read", "update"],
      pipeline: ["read"],
      product: ["create", "read", "update"],
      project: ["create", "read", "update"],
      relationship: ["create", "read", "update"],
      saved_view: ["create", "read", "update", "delete"],
      signal: ["create", "read", "update"],
      tag: ["read"],
      voice_profile: ["create", "read", "update"],
      webhook_subscription: ["read"],
      weekly_plan: ["create", "read", "update"],
    },
  });

  // Company is absent: its profile is the admin's, and a rep holds neither
  // grant that opens the page (the catalog's `company` entry).
  it("gives a rep the pages they work in, and only those", () => {
    const reach = settingsReach(seededRep);
    expect(reach.acts.map((page) => page.id)).toEqual([
      // Their own six, minus Voice — a rep holds voice_profile create and
      // update, so Voice IS theirs; it is here for that reason and not because
      // the page sits under their own heading.
      "account",
      "meetings",
      "voice",
      "agents",
      "notifications",
      "connections",
      "capture-activity",
      // Products and offer templates: a rep authors both.
      "products",
      // Capture rules, because a rep holds `company:update` and
      // BlockedDomainsCard writes it. If a rep editing the company's blocked
      // domains is not wanted, that CARD's grant is the thing to change — the
      // rail is only reporting what the card already allows.
      "capture",
    ]);
  });

  it("leaves the pages a rep can only read out of the rail, not out of reach", () => {
    const reach = settingsReach(seededRep);
    expect(reach.looksUp.map((page) => page.id)).toEqual([
      "pipelines",
      // Stage automation MOVED here when the page grew its switches. A rep
      // holds pipeline read and not update, so the report is still theirs to
      // consult and every control on it is refused — which is what looksUp
      // means. It sat in the acted-on half while the page was read-only.
      "stageautomation",
      "leads",
      // The acquisition-source catalog, gated on custom_field like the lead
      // vocabulary beside it: a rep reads the channels and changes none.
      "acquisition",
      // The loss reasons, gated the same way: a rep picks one when a deal is
      // lost and changes the list nowhere.
      "lossreasons",
      // The outcome-review questions: a rep is asked them when a deal closes
      // and reads them here, but editing them is custom_field update.
      "reviewtemplates",
      // The responsibility-role vocabulary, gated the same way: a rep reads
      // the roles and changes none.
      "recordroles",
      "fields",
      "tags",
      "knowledge",
    ]);
    // Integrations and Automations are absent from BOTH halves, and that is
    // #4650's doing rather than this change's: their `requires` asks the write,
    // so a rep never opens them at all. A page has to be visible before the
    // partition has anything to say about it.
    // Still openable, every one of them: the partition decides prominence, and
    // a page in `looksUp` answers its address, appears in search and is listed
    // on the settings home. Losing that distinction would turn a tidier rail
    // into a permission change nobody asked for.
    const openable = visibleSettingsPages(seededRep).map((page) => page.id);
    for (const page of reach.looksUp) {
      expect(openable).toContain(page.id);
    }
  });

  // The partition is exhaustive and disjoint — every visible page in exactly
  // one half. Without this a page could fall out of both and vanish from the
  // rail AND the home while still answering its address, which is the one
  // shape of this bug nobody would report.
  it("puts every page a reader may open in exactly one half", () => {
    for (const snapshot of [seededRep, meFixture({ roles: ["admin"] })]) {
      const reach = settingsReach(snapshot);
      const partitioned = [...reach.acts, ...reach.looksUp]
        .map((page) => page.id)
        .sort();
      expect(partitioned).toEqual(
        visibleSettingsPages(snapshot)
          .map((page) => page.id)
          .sort(),
      );
      expect(new Set(partitioned).size).toBe(partitioned.length);
    }
  });
});
