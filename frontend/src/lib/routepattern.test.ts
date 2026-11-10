import { describe, expect, it } from "vitest";
import { routePattern } from "./routepattern";
import { parseHash } from "./router";

describe("routePattern", () => {
  const units = ["crm-hello"];
  const pattern = (hash: string) => routePattern(parseHash(hash), units);

  it("replaces record ids with placeholders", () => {
    expect(pattern("#/contacts/01J9ZK")).toBe("contacts/:id");
    expect(pattern("#/deals")).toBe("deals");
  });
  it("replaces every id of a three-id screen", () => {
    expect(pattern("#/contacts/a/b/c")).toBe("contacts/:id/:id2/:id3");
  });
  it("keeps a composed unit's name, which is not a record id", () => {
    expect(pattern("#/ext/crm-hello")).toBe("ext/crm-hello");
  });
  it("drops the extra ids of a unit address", () => {
    expect(pattern("#/ext/crm-hello/a/b")).toBe("ext/crm-hello");
  });
  it("hides a unit name that nothing composed", () => {
    expect(pattern("#/ext/some-typed-secret")).toBe("ext/:unit");
    expect(pattern("#/ext")).toBe("ext/:unit");
  });
  it("never names a fork screen's key", () => {
    expect(pattern("#/x/typed-by-someone")).toBe("x/:key");
    expect(pattern("#/x")).toBe("x/:key");
  });
  it("never carries the query", () => {
    expect(pattern("#/reset-password?token=secret")).toBe("reset-password");
  });
});
