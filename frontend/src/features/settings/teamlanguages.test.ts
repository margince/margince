import { describe, expect, it } from "vitest";
import { teamLanguages } from "./teamlanguages";

describe("the languages a label needs a name in", () => {
  it("are those an active member reads the app in, less the base language", () => {
    const members = [
      { status: "active", locale: "vi" },
      { status: "active", locale: "en" },
      { status: "deactivated", locale: "de" },
      { status: "active", locale: undefined },
    ] as const;
    expect(teamLanguages(members, "en")).toEqual(["vi"]);
  });

  it("mark nothing while no member reads another language", () => {
    expect(teamLanguages([{ status: "active", locale: "en" }], "en")).toEqual(
      [],
    );
  });

  it("mark nothing before the roster and the settings answer", () => {
    expect(teamLanguages(undefined, "en")).toEqual([]);
    expect(
      teamLanguages([{ status: "active", locale: "vi" }], undefined),
    ).toEqual([]);
  });
});
