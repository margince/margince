import { describe, expect, it } from "vitest";
import { de } from "./de";
import { en } from "./en";
import { vi } from "./vi";

// The person record is called a person — "People" in the nav, "person" in a
// sentence — and never a contact. The older word survives only where it names
// the ACT of reaching someone ("never contacted", "last contact yesterday"),
// the details one reaches them by ("contact data", "contact email"), or a
// buyer's counterpart in a room ("your contact"). Those keys are waived here by
// name, so a new value that says "contact" for the record fails, and a waiver
// whose value stops carrying the word fails too: a stale waiver would let the
// next author reintroduce the record noun under a key nobody looks at.
type Catalog = Record<string, string>;

const RECORD_NOUN: Record<string, RegExp> = {
  en: /\bcontacts?\b/i,
  de: /kontakt/i,
  vi: /liên hệ|\bcontacts?\b/i,
};

// A company website's Contact page, as the deep read and the onboarding
// digest classify what they cited: a kind of page, not a record.
const WEBSITE_PAGE_KIND_KEYS = [
  "deepread.kindContact",
  "ob.digest.pageKind.contact",
];

// Shared across the locales: the act of reaching someone, and the details one
// reaches them by. A locale adds the keys where only its own wording says the
// word — German says "Kontaktstand" where English says "Engagement".
const ACT_OR_DETAIL_KEYS_SHARED = [
  "co.routeIn.band.strong",
  "co.routeIn.band.some",
  "co.routeIn.band.faint",
  "co.routeIn.band.unknown",
  "co.factField.contact_email",
  "deal.strip.momentum.detail",
  "passport.scope.enrich",
  "connectors.signatureEnrich.label",
  "captureSettings.signatureEnrich.label",
  "network.empty",
  "network.neverSpoken",
  "network.bucket.none",
  "license.holder.contact",
  "person.intro.evidenceLastContact",
  "person.intro.stripWhoMix",
  "person.intro.whenToday",
  "person.intro.whenYesterday",
  "person.intro.whenDays",
  "person.intro.whenNever",
  "person.band.none",
  "person.research.notConnected",
  "provider.title",
  "provider.sub",
  "provider.profile.title",
  ...WEBSITE_PAGE_KIND_KEYS,
];

// The buyer room: "your contact" is the steward on the seller's side, a
// counterpart rather than a record.
const BUYER_COUNTERPART_KEYS = [
  "buyer.deadAskContact",
  "buyer.expiredBody",
  "buyer.contact",
  "buyer.stewardUnknown",
  "buyer.docs.downloadFailed",
];

const ACT_OR_DETAIL_KEYS: Record<string, readonly string[]> = {
  en: [
    ...ACT_OR_DETAIL_KEYS_SHARED,
    ...BUYER_COUNTERPART_KEYS,
    // What a phone exports is called contacts by the phone.
    "vcardImport.whichFile",
    "coverage.quiet",
    // "as a sponsor, a contact, or whoever else": a role on the delivery.
    "personProjects.empty",
  ],
  de: [
    ...ACT_OR_DETAIL_KEYS_SHARED,
    "partner.stage.contacted",
    "co.strip.lastTouch",
    "co.strip.engagement.never_contacted",
    "co.health.means.relationship",
    "co.rail.people.inTouch",
    // LinkedIn calls a connection a Kontakt in German, and these describe
    // the import of LinkedIn's own file.
    "linkedinImport.title",
    "linkedinImport.connectedNote",
    "linkedinImport.notConnectedNote",
    "linkedinImport.importLabel",
    "linkedinImport.noMatchesYet",
    "linkedinImport.imported",
    "ob.conv.linkedin.cardBody",
    "ob.conv.linkedin.importLater",
    "co.people.engagement",
    "co.people.filter.status",
    "co.people.filter.statusAll",
    "co.people.band.untried",
    "co.people.band.showUntried",
    "lead.statusContacted",
    "lead.status.contacted",
    "lead.ladder.new",
    "home.readings.leads",
    "vcardImport.whichFile",
    "acctCoverage.noneButPartial",
    "compose.why.requestedFollowup",
    "confirm.marketing.title",
    "coverage.quiet",
    "person.thin.logFirst",
    "person.intro.evidenceOneSided_one",
    "person.intro.evidenceOneSided_other",
    "person.network.twoWay",
    "person.network.oneSided",
    "person.rail.nobodyYet",
    "person.rail.exchanges",
    "person.meeting.what_changed",
    "person.meeting.arc",
    "deal.strip.lastTouch",
  ],
  vi: [
    ...ACT_OR_DETAIL_KEYS_SHARED,
    ...BUYER_COUNTERPART_KEYS,
    "partner.stage.contacted",
    "co.strip.noInboundEver",
    "co.strip.engagement.never_contacted",
    "signal.kind.reengagement",
    "co.rail.people.inTouch",
    "co.people.engagement",
    "co.people.filter.status",
    "lead.source.inbound",
    "lead.statusContacted",
    "lead.status.contacted",
    "lead.ladder.new",
    "compose.why.requestedFollowup",
    "person.intro.fallbackNameDropHelp",
    "person.intro.answerNameDropHelp",
  ],
};

const CATALOGS: Record<string, Catalog> = { en, de, vi };

// `{contact}` in "introduce you to {contact}" is the placeholder's name, which
// the reader never sees; only the visible words are judged.
function visibleWords(value: string): string {
  return value.replace(/\{[^}]*\}/g, "");
}

describe.each(Object.keys(CATALOGS))(
  "%s names the record a person",
  (locale) => {
    const catalog = CATALOGS[locale];
    const pattern = RECORD_NOUN[locale];
    const waived = new Set(ACT_OR_DETAIL_KEYS[locale]);

    it("says contact only where the key is waived as the act or the details", () => {
      const offenders = Object.entries(catalog)
        .filter(
          ([key, value]) =>
            !waived.has(key) && pattern.test(visibleWords(value)),
        )
        .map(([key, value]) => `${key}: ${value}`);
      expect(offenders).toEqual([]);
    });

    it("keeps every waiver pointing at a value that still says contact", () => {
      const stale = [...waived].filter(
        (key) => !(key in catalog) || !pattern.test(visibleWords(catalog[key])),
      );
      expect(stale).toEqual([]);
    });
  },
);
