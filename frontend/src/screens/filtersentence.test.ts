// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { viewerZone } from "../format/timezone";
import { type Locale, translate, translatePlural } from "../i18n";
import { de } from "../i18n/de";
import { en as enCatalog } from "../i18n/en";
import type { VocabularyField } from "./filterdata";
import {
  clauseWords,
  filterSentence,
  type SentenceWords,
} from "./filtersentence";
import { decode, type Node, newGroup, newLeaf } from "./segmentpredicate";

// The one place a filter becomes words, so these read like the sentences a
// reader sees under a Live List and on a saved view, clause by clause.

function wordsIn(locale: Locale): SentenceWords {
  return {
    t: (key, params) => translate(locale, key, params),
    plural: (base, count, params) =>
      translatePlural(locale, base, count, params),
    locale,
    // A calendar day reads as the same day in every zone, so the viewer's will do.
    zone: viewerZone(),
  };
}

const en = wordsIn("en");

const FIELDS: VocabularyField[] = [
  { name: "country", type: "picklist", operators: ["in"], custom: false },
  {
    name: "last_activity_at",
    type: "date",
    operators: ["lt", "lte", "gt", "gte"],
    custom: false,
  },
  { name: "created_at", type: "date", operators: ["gte"], custom: false },
  {
    name: "amount",
    type: "currency",
    operators: ["gte"],
    custom: false,
    currency: "EUR",
  },
  {
    name: "stage_id",
    type: "id",
    operators: ["in"],
    custom: false,
    references: "stage",
  },
  {
    name: "owner_id",
    type: "id",
    operators: ["in"],
    custom: false,
    references: "app_user",
  },
  { name: "email", type: "text", operators: ["exists"], custom: false },
  { name: "city", type: "text", operators: ["eq"], custom: false },
  {
    name: "tag",
    type: "id",
    operators: ["eq", "neq", "in"],
    custom: false,
    references: "tag",
  },
  {
    name: "employee_count",
    type: "number",
    operators: ["gte", "in"],
    custom: false,
  },
];

function read(tree: Node, words: SentenceWords = en): string {
  return filterSentence(tree, FIELDS, words);
}

const quietInDach = newGroup("and", [
  newLeaf("country", "in", ["DE", "AT", "CH"]),
  newLeaf("last_activity_at", "lt", { days_ago: 45 }),
]);

describe("a filter read as one sentence", () => {
  it("joins a flat AND's clauses and reads a count back as how long ago", () => {
    expect(read(quietInDach)).toBe(
      "Country code is any of DE, AT, CH and Last activity is more than 45 days ago",
    );
    expect(
      read(
        newGroup("and", [newLeaf("last_activity_at", "gte", { days_ago: 30 })]),
      ),
    ).toBe("Last activity is within the last 30 days");
    expect(
      read(
        newGroup("and", [newLeaf("last_activity_at", "gt", { days_ago: 1 })]),
      ),
    ).toBe("Last activity is within the last day");
    expect(
      read(
        newGroup("and", [newLeaf("last_activity_at", "lte", { days_ago: 1 })]),
      ),
    ).toBe("Last activity is at least 1 day ago");
  });

  it("says is for one value, a day as the reader reads it, and money in its currency", () => {
    expect(read(newGroup("and", [newLeaf("country", "in", ["DE"])]))).toBe(
      "Country code is DE",
    );
    expect(
      read(newGroup("and", [newLeaf("created_at", "gte", "2026-12-31")])),
    ).toBe("Created is on or after 31 Dec 2026");
    expect(read(newGroup("and", [newLeaf("amount", "gte", 125_000)]))).toBe(
      "Converted amount is at least €1,250.00",
    );
  });

  it("states no amount whose currency the reader may not learn", () => {
    const unpriced: VocabularyField[] = [
      { name: "amount", type: "currency", operators: ["gte"], custom: false },
    ];
    // Minor units without their currency would read as the amount itself.
    expect(
      filterSentence(
        newGroup("and", [newLeaf("amount", "gte", 125_000)]),
        unpriced,
        en,
      ),
    ).toBe("Converted amount is at least …");
  });

  it("groups a quantity the way the reader writes numbers", () => {
    const many = {
      field: "employee_count",
      op: "gte",
      operand: 1_000_000,
    } as const;
    expect(clauseWords(many, FIELDS, en)).toMatchObject({
      op: "is at least",
      operand: "1,000,000",
    });
    expect(clauseWords(many, FIELDS, wordsIn("de")).operand).toBe("1.000.000");
    expect(
      clauseWords(
        { field: "employee_count", op: "in", operand: ["1500", "20000"] },
        FIELDS,
        wordsIn("de"),
      ).operand,
    ).toBe("1.500, 20.000");
  });

  it("marks a value not given yet as a gap rather than ending the clause", () => {
    for (const operand of ["", []]) {
      expect(read(newGroup("and", [newLeaf("country", "in", operand)]))).toBe(
        "Country code is any of …",
      );
    }
    // An id not chosen yet is not one id to count.
    expect(read(newGroup("and", [newLeaf("stage_id", "eq", "")]))).toBe(
      "Stage is …",
    );
  });

  it("reads exists as having a value or being empty, with no operand", () => {
    const filled = clauseWords(
      { field: "email", op: "exists", operand: true },
      FIELDS,
      en,
    );
    expect(filled).toEqual({ field: "Email", op: "has a value" });
    expect(
      read(
        newGroup("or", [
          newLeaf("email", "exists", true),
          newLeaf("email", "exists", false),
        ]),
      ),
    ).toBe("Email has a value or Email is empty");
  });

  it("brackets a nested group at every depth and names an empty one", () => {
    const nested = newGroup("and", [
      newLeaf("city", "eq", "Berlin"),
      newGroup("or", [
        newLeaf("country", "in", ["DE"]),
        newGroup("and", [
          newLeaf("email", "exists", true),
          newLeaf("country", "in", ["AT"]),
        ]),
      ]),
    ]);
    expect(read(nested)).toBe(
      "City is Berlin and (Country code is DE or (Email has a value and Country code is AT))",
    );
    expect(
      read(newGroup("and", [newLeaf("city", "eq", "Berlin"), newGroup("or")])),
    ).toBe("City is Berlin and an empty group");
  });

  it("calls a clause on a field the vocabulary no longer offers retired, and counts ids in their noun", () => {
    expect(read(newGroup("and", [newLeaf("cf_tier", "eq", "gold")]))).toBe(
      "a retired field",
    );
    // Without its type, an operator would read as a date and minor units as
    // the amount, so neither is said.
    const retired = read(
      newGroup("and", [
        newLeaf("cf_score", "gt", 50),
        newLeaf("cf_budget", "gte", 125_000),
      ]),
    );
    expect(retired).toBe("a retired field and a retired field");
    expect(retired).not.toMatch(/after|50|125/);
    expect(
      read(newGroup("and", [newLeaf("stage_id", "in", ["s1", "s2"])])),
    ).toBe("Stage is any of 2 stages");
    expect(
      read(newGroup("and", [newLeaf("owner_id", "in", ["u1", "u2", "u3"])])),
    ).toBe("Owner is any of 3 team members");
    // One counted id is still a count, never the stage itself.
    expect(read(newGroup("and", [newLeaf("stage_id", "in", ["s1"])]))).toBe(
      "Stage is any of 1 stage",
    );
  });

  it("names a retired tag archived under every operator, and says no record carries one", () => {
    const retiring = { ...en, retiredTags: new Set(["t-old", "t-gone"]) };
    const tagged = (op: "eq" | "neq" | "in", value: string | string[]) =>
      read(newGroup("and", [newLeaf("tag", op, value)]), retiring);
    expect(tagged("eq", "t-old")).toBe("Tag is 1 archived tag");
    expect(tagged("neq", "t-old")).toBe("Tag is not 1 archived tag");
    expect(tagged("in", ["t-old", "t-gone"])).toBe(
      "Tag is any of 2 archived tags",
    );
    expect(tagged("in", ["t-live", "t-old", "t-new"])).toBe(
      "Tag is any of 3 tags, 1 archived",
    );
    expect(tagged("eq", "t-live")).toBe("Tag is 1 tag");
    // Beside an "or", the clause is one way in among others, not the group's answer.
    expect(
      read(
        newGroup("or", [
          newLeaf("city", "eq", "Berlin"),
          newLeaf("tag", "eq", "t-old"),
        ]),
        retiring,
      ),
    ).toBe("City is Berlin or Tag is 1 archived tag");

    const note = enCatalog["filters.sentence.retiredTagNote"];
    for (const [op, operand] of [
      ["eq", "t-old"],
      ["neq", "t-old"],
      ["in", ["t-live", "t-gone"]],
    ] as const) {
      expect(
        clauseWords({ field: "tag", op, operand }, FIELDS, retiring),
      ).toMatchObject({ note });
    }
    expect(
      clauseWords(
        { field: "tag", op: "eq", operand: "t-live" },
        FIELDS,
        retiring,
      ).note,
    ).toBeUndefined();
    // Words that know of no retired tag count every id as a tag.
    expect(read(newGroup("and", [newLeaf("tag", "eq", "t-old")]))).toBe(
      "Tag is 1 tag",
    );
    expect(
      read(newGroup("and", [newLeaf("tag", "eq", "t-old")]), {
        ...wordsIn("de"),
        retiredTags: retiring.retiredTags,
      }),
    ).toMatch(/ 1 archiviertes Tag$/);
  });

  it("counts ids in German with no article that must agree with the noun", () => {
    const german = wordsIn("de");
    expect(
      read(newGroup("and", [newLeaf("stage_id", "in", ["s1", "s2"])]), german),
    ).toBe(
      `${de["filters.field.stage_id"]} ${de["filters.sentence.inCounted"]} 2 Phasen`,
    );
    expect(
      read(newGroup("and", [newLeaf("owner_id", "in", ["u1"])]), german),
    ).toMatch(/ 1 Teammitglied$/);
    expect(
      read(newGroup("and", [newLeaf("country", "in", ["DE", "AT"])]), german),
    ).toBe(`${de["filters.field.country"]} ${de["filters.op.in"]} DE, AT`);
  });

  it("keeps a field's own name until the vocabulary answers, and only counts the tree", () => {
    const three = newGroup("and", [
      ...quietInDach.children,
      newLeaf("city", "eq", "Berlin"),
    ]);
    expect(filterSentence(three, undefined, en)).toBe("3 conditions");
    expect(
      clauseWords(
        { field: "cf_tier", op: "eq", operand: "gold" },
        undefined,
        en,
      ).field,
    ).toBe("cf_tier");
    expect(read(newGroup("and"))).toBe("");
    expect(filterSentence(newGroup("and"), undefined, en)).toBe("");
  });

  it("reads a stored single clause, which decodes to a bare leaf", () => {
    const stored = decode({ field: "city", op: "eq", value: "Berlin" });
    if (stored === null) {
      throw new Error("the stored clause did not decode");
    }
    expect(read(stored)).toBe("City is Berlin");
  });

  it("says the whole sentence in the reader's language", () => {
    const german = read(quietInDach, wordsIn("de"));
    expect(german).toBe(
      [
        de["filters.field.country"],
        de["filters.op.in"],
        "DE, AT, CH",
        de["filters.join.and"],
        de["filters.field.last_activity_at"],
        de["filters.sentence.moreThanAgo"],
        "45 Tage zurück",
      ].join(" "),
    );
    expect(german).not.toMatch(/\b(is|and|days|ago)\b/);
  });
});
