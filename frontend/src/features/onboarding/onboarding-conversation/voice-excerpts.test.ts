import { describe, expect, it } from "vitest";
import {
  EXCERPT_LINES_PER_SOURCE,
  emphasisIndices,
  excerptLines,
} from "./voice-excerpts";

describe("excerptLines", () => {
  it("keeps whole sentences of a readable length, in the order written", () => {
    const text =
      "Thanks. We should move the kickoff to Thursday so the data team can join. " +
      "Best, Anna\nI have attached the revised offer with the two changes we discussed.";
    expect(excerptLines(text)).toEqual([
      "We should move the kickoff to Thursday so the data team can join.",
      "I have attached the revised offer with the two changes we discussed.",
    ]);
  });

  it("yields nothing for a text with no sentence of that length", () => {
    expect(excerptLines("Ok.\nThanks!\nSee you.")).toEqual([]);
  });

  it("stops at the per-source ceiling", () => {
    const sentence = "This sentence has exactly seven words in it. ";
    expect(excerptLines(sentence.repeat(20))).toHaveLength(
      EXCERPT_LINES_PER_SOURCE,
    );
  });
});

describe("emphasisIndices", () => {
  it("lights the two longest words, earliest first on a tie, and never a short one", () => {
    const words = "We should move the kickoff to Thursday, please".split(" ");
    expect([...emphasisIndices(words)].sort()).toEqual([4, 6]);
    expect(emphasisIndices("so it is".split(" ")).size).toBe(0);
  });
});
