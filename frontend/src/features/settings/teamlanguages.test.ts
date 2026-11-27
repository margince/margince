import { describe, expect, it } from "vitest";
import { teamLanguages } from "./teamlanguages";

describe("the languages a label needs a name in", () => {
  it("are those members read the app in, less the base language", () => {
    expect(teamLanguages(["en", "vi"], "en")).toEqual(["vi"]);
  });

  it("mark nothing while no member reads another language", () => {
    expect(teamLanguages(["en"], "en")).toEqual([]);
  });

  it("mark nothing before the settings answer", () => {
    expect(teamLanguages(undefined, "en")).toEqual([]);
    expect(teamLanguages(["vi"], undefined)).toEqual([]);
  });
});
