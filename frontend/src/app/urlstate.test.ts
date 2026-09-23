/** @vitest-environment happy-dom */
import { describe, expect, it } from "vitest";
import {
  ASK_PARAM,
  ASK_QUESTION_PARAM,
  closeAsk,
  currentParams,
  hashWithParams,
  NO_URL_PARAMS,
  openAsk,
  openAsk as openAskDialog,
  parseParams,
} from "./urlstate";

const dials = (entries: Record<string, string>) =>
  new Map(Object.entries(entries));

describe("parseParams — the dials an address carries", () => {
  it("reads nothing out of an address with no query", () => {
    expect(parseParams("#/companies")).toBe(NO_URL_PARAMS);
  });

  it("reads a dial per key", () => {
    expect([...parseParams("#/companies?q=acme&sort=-created_at")]).toEqual([
      ["q", "acme"],
      ["sort", "-created_at"],
    ]);
  });

  it("keeps the path out of the dials", () => {
    // The path names the screen and the query names the view of it. A parser
    // that let one leak into the other would filter a list by its own name.
    expect(parseParams("#/companies/c-42?tab=work").get("tab")).toBe("work");
    expect(parseParams("#/companies/c-42?tab=work").has("companies")).toBe(
      false,
    );
  });

  it("reads an empty value as absent, so ?q= and no q are one address", () => {
    expect(parseParams("#/companies?q=").has("q")).toBe(false);
  });

  it("takes the last of a repeated key, which is what the writer meant", () => {
    expect(parseParams("#/companies?sort=name&sort=-name").get("sort")).toBe(
      "-name",
    );
  });

  it("decodes a value that had to be escaped", () => {
    // A company name with a space or an ampersand is ordinary, and a search for
    // one has to survive the round trip.
    expect(parseParams("#/companies?q=A%26B%20Ltd").get("q")).toBe("A&B Ltd");
  });
});

describe("hashWithParams — the address a set of dials produces", () => {
  it("keeps the path and hangs the dials off it", () => {
    expect(hashWithParams("#/companies", dials({ q: "acme" }))).toBe(
      "#/companies?q=acme",
    );
  });

  it("replaces the dials that were already there rather than adding to them", () => {
    expect(
      hashWithParams("#/companies?q=old&sort=name", dials({ q: "new" })),
    ).toBe("#/companies?q=new");
  });

  it("drops the query entirely when no dial is set", () => {
    expect(hashWithParams("#/companies?q=acme", NO_URL_PARAMS)).toBe(
      "#/companies",
    );
  });

  it("drops a dial whose value is empty rather than writing ?q=", () => {
    expect(hashWithParams("#/companies", dials({ q: "", sort: "name" }))).toBe(
      "#/companies?sort=name",
    );
  });

  it("sorts the keys, so one set of dials has one address", () => {
    // Two readers who chose the same filters in a different order must get
    // comparable links, and the composite owner chip deletes and re-adds its
    // param, which would otherwise move insertion order without changing what
    // the list shows.
    expect(
      hashWithParams("#/companies", dials({ sort: "name", owner_id: "u1" })),
    ).toBe(
      hashWithParams("#/companies", dials({ owner_id: "u1", sort: "name" })),
    );
  });

  it("spells a path for a bare document, so the query hangs off something", () => {
    expect(hashWithParams("", dials({ q: "acme" }))).toBe("#/?q=acme");
    expect(hashWithParams("", NO_URL_PARAMS)).toBe("#/");
  });

  it("escapes a value that would otherwise end the query", () => {
    expect(hashWithParams("#/companies", dials({ q: "A&B Ltd" }))).toBe(
      "#/companies?q=A%26B+Ltd",
    );
  });

  it("round-trips whatever it wrote", () => {
    const set = dials({ q: "A&B Ltd", sort: "-created_at", owner_id: "u1" });
    expect(parseParams(hashWithParams("#/companies", set))).toEqual(set);
  });
});

// The two dials the Ask dialog rides on. They are TWO because parseParams drops
// a dial with an empty value — a rule worth keeping, since a filter set to
// nothing is not a filter — and "open with an empty box" is exactly that shape.
describe("the Ask dials", () => {
  it("opens over the address the reader is on, keeping its other dials", () => {
    window.location.hash = "#/deals?owner_id=u1";
    openAsk("how long are messages kept");
    const dials = currentParams();
    expect(window.location.hash.startsWith("#/deals")).toBe(true);
    expect(dials.get(ASK_PARAM)).toBe("1");
    expect(dials.get(ASK_QUESTION_PARAM)).toBe("how long are messages kept");
    expect(dials.get("owner_id")).toBe("u1");
  });

  // An empty question must still OPEN it: the palette's own row carries none,
  // and a dial written empty would not survive the address to say so.
  it("opens with no question at all", () => {
    window.location.hash = "#/home";
    openAskDialog("");
    expect(currentParams().get(ASK_PARAM)).toBe("1");
    expect(currentParams().has(ASK_QUESTION_PARAM)).toBe(false);
  });

  it("drops a question the reader asked before, rather than carrying it", () => {
    window.location.hash = "#/home";
    openAsk("first question");
    openAsk("");
    expect(currentParams().has(ASK_QUESTION_PARAM)).toBe(false);
  });

  it("closes both dials and leaves the page's own alone", () => {
    window.location.hash = "#/deals?owner_id=u1";
    openAsk("a question");
    closeAsk();
    const dials = currentParams();
    expect(dials.has(ASK_PARAM)).toBe(false);
    expect(dials.has(ASK_QUESTION_PARAM)).toBe(false);
    expect(dials.get("owner_id")).toBe("u1");
  });
});
