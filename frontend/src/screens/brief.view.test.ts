import { describe, expect, it } from "vitest";
import { NO_URL_PARAMS, type UrlParams } from "../app/urlstate";
import {
  addressFrom,
  DEFAULT_ADDRESS,
  paramsFor,
  SCOPES,
  scopesFor,
  VIEWS,
} from "./brief.view";

// The Brief's address.
//
// The plan's decision 5 states the rule these are about: no visible dial
// combination may resolve to a legacy, empty or substitute screen. So the
// matrix below is not a nicety — it is the assertion that would have caught
// `view=weekly&scope=team` being selectable three PRs before the team weekly
// existed.

function params(entries: Record<string, string>): UrlParams {
  return new Map(Object.entries(entries));
}

describe("the Brief's address", () => {
  it("opens on the morning, on the reader's own brief", () => {
    expect(addressFrom(NO_URL_PARAMS, false)).toEqual(DEFAULT_ADDRESS);
    expect(addressFrom(NO_URL_PARAMS, true)).toEqual(DEFAULT_ADDRESS);
  });

  it("round-trips every combination it offers", () => {
    for (const view of VIEWS) {
      for (const scope of SCOPES) {
        const address = { view, scope } as const;
        // `offered` true, because a lead is the reader who can reach all four.
        expect(addressFrom(paramsFor(address), true)).toEqual(address);
      }
    }
  });

  // An address is something a person types, a link carries from an older build,
  // and a colleague sends from a seat with wider reach. None of those may
  // produce a broken page.
  it("falls back rather than breaking on a value it does not know", () => {
    expect(addressFrom(params({ view: "quarterly" }), true).view).toBe(
      "morning",
    );
    expect(addressFrom(params({ scope: "everyone" }), true).scope).toBe("mine");
  });

  // THE ONE THAT MATTERS FOR PERMISSIONS. A lead sends a rep a link to their
  // team's brief. The rep may not read it — the server would refuse — so the
  // page must resolve to the nearest Brief they ARE entitled to rather than
  // sitting on a dial it cannot fill.
  it("narrows a team address for a reader whose scope does not reach one", () => {
    expect(addressFrom(params({ scope: "team" }), false).scope).toBe("mine");
    expect(addressFrom(params({ scope: "team" }), true).scope).toBe("team");
  });

  // A control with one option asks a reader to confirm what they cannot change.
  it("offers a scope dial only to a reader who has a second scope", () => {
    for (const view of VIEWS) {
      expect(scopesFor(view, false)).toEqual(["mine"]);
      expect(scopesFor(view, true)).toEqual([...SCOPES]);
    }
  });

  // The default is absent from the address, so a reader who changed nothing can
  // copy the short link and get the same page.
  it("writes only what differs from the default", () => {
    expect([...paramsFor(DEFAULT_ADDRESS)]).toEqual([]);
    expect([...paramsFor({ view: "weekly", scope: "mine" })]).toEqual([
      ["view", "weekly"],
    ]);
    expect([...paramsFor({ view: "morning", scope: "team" })]).toEqual([
      ["scope", "team"],
    ]);
  });

  // DECISION 5, AS A MATRIX. Every combination the dials actually offer has to
  // survive a round trip AND be one this reader may see — which together is
  // what "no dial resolves to nothing" means at this layer.
  it("offers no combination that does not resolve to itself", () => {
    for (const offered of [false, true]) {
      for (const view of VIEWS) {
        for (const scope of scopesFor(view, offered)) {
          const resolved = addressFrom(paramsFor({ view, scope }), offered);
          expect(resolved).toEqual({ view, scope });
        }
      }
    }
  });
});
