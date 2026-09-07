import { describe, expect, it } from "vitest";
import { de } from "./de";
import { en } from "./en";
import { vi } from "./vi";

// The person record is NAMED a contact — "Contacts" on the nav row, and the
// same noun on every tab, group heading, import object, counter and unit that
// offers the record TYPE as a thing to pick, count, file under or navigate to.
// Those keys are listed below, and this is the only place the set is written
// down.
//
// It fails in both directions: a listed value that stops carrying the noun, and
// a listed value that carries the retired one. Half a rename is what puts two
// names for one record on one screen, and each half looks correct on its own.
//
// The list is hand-kept because nothing in a catalog marks which key names a
// record type — a value is a string, and "Contacts" as a tab and "contacts" as
// a table unit are indistinguishable to a sweep. A label that names this type
// and is missing here is a label free to drift to a second word, so add it.
//
// This gate has no opinion about "person" or "people" elsewhere. Those are
// still the right words for a human being, and the catalogs say them in
// hundreds of sentences about PEOPLE rather than about the record type — "the
// people on this message", "Their key people", "3 people match". Only the
// type's own name is held here.
type Catalog = Record<string, string>;

const RECORD_TYPE_NAME_KEYS = [
  "nav.contacts",
  "search.group.person",
  "filters.tab.contacts",
  "tab.contacts",
  "tagResult.contacts",
  "import.object.person",
  "org.contactCount",
  "users.access.object.person",
  "backfill.statContacts",
  "ob.digest.contacts",
  "ob.conv.triage.contactsLabel",
  "brief.digestContacts",
  "unit.contacts",
  "worklist.sync.class.contacts",
  // The one sentence that names the type rather than labelling it: it tells a
  // reader what a lead turns into, so it drifts the way a label does and costs
  // more when it does.
  "lead.segregation",
];

const RECORD_NOUN: Record<string, RegExp> = {
  en: /\bcontacts?\b/i,
  de: /kontakt/i,
  vi: /liên hệ/i,
};

const RETIRED_NOUN: Record<string, RegExp> = {
  en: /\bpeople\b|\bpersons?\b/i,
  de: /\bpersonen?\b/i,
  vi: /\bngười\b/i,
};

const CATALOGS: Record<string, Catalog> = { en, de, vi };

// `{contact}` in "introduce you to {contact}" is the placeholder's name, which
// the reader never sees; only the visible words are judged.
function visibleWords(value: string): string {
  return value.replace(/\{[^}]*\}/g, "");
}

describe.each(Object.keys(CATALOGS))(
  "%s names the record a contact",
  (locale) => {
    const catalog = CATALOGS[locale];
    const noun = RECORD_NOUN[locale];
    const retired = RETIRED_NOUN[locale];

    it("every key that names the record type carries the noun", () => {
      const missing = RECORD_TYPE_NAME_KEYS.filter(
        (key) => !(key in catalog) || !noun.test(visibleWords(catalog[key])),
      );
      expect(missing).toEqual([]);
    });

    it("no key that names the record type carries the retired noun", () => {
      const stale = RECORD_TYPE_NAME_KEYS.filter(
        (key) => key in catalog && retired.test(visibleWords(catalog[key])),
      );
      expect(stale).toEqual([]);
    });
  },
);
