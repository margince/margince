// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { lineOfPath, parseProblem } from "./jsonfield";
import { scanJson } from "./jsonscan";

// The scanner stands in for JSON.parse when the editor asks where text stops
// being JSON, so it must accept exactly what JSON.parse accepts.
const CORPUS = [
  "{}",
  "[]",
  '{"a": [1, -2.5e3, true, false, null, "x\\n\\u00e9"]}',
  ' { "nested": { "deep": [ { } ] } } ',
  "0",
  '"just a string"',
  "{",
  '{"a": }',
  '{"a": 1,}',
  "[1, 2,]",
  "01",
  '{"a" 1}',
  "{a: 1}",
  '"unterminated',
  '"bad \\x escape"',
  '{"a": 1} trailing',
  "tru",
  "-",
  "1.",
  '"tab\tinside"',
];

function parses(text: string): boolean {
  try {
    JSON.parse(text);
    return true;
  } catch (error) {
    return !(error instanceof SyntaxError);
  }
}

describe("scanJson", () => {
  it.each(CORPUS)("agrees with JSON.parse on %j", (text) => {
    expect(scanJson(text).error === undefined).toBe(parses(text));
  });

  it("reports a document nested past its depth instead of throwing", () => {
    const deep = `${"[".repeat(100_000)}${"]".repeat(100_000)}`;
    expect(scanJson(deep).error).toBe(256);
  });

  it("puts the problem on the line it is written on, whatever the engine says", () => {
    expect(parseProblem('{\n  "provider": {\n    "sort": }\n}')).toEqual({
      line: 3,
    });
    expect(parseProblem('{"a": 1}')).toBeNull();
    expect(parseProblem("   ")).toBeNull();
  });
});

describe("lineOfPath", () => {
  it("finds a key under its own parent, not an earlier key of the same name", () => {
    const text = [
      "{",
      '  "provider": {"zdr": true},',
      '  "sort": {"by": "unrelated"},',
      '  "reasoning": {',
      '    "sort": {"by": "weight"}',
      "  }",
      "}",
    ].join("\n");
    expect(lineOfPath(text, "reasoning.sort.by")).toBe(5);
    expect(lineOfPath(text, "sort.by")).toBe(3);
  });

  it("marks a list entry, and falls back to the nearest written parent", () => {
    const text =
      '{\n  "provider": {\n    "only": [\n      "a",\n      "b"\n    ]\n  }\n}';
    expect(lineOfPath(text, "provider.only[1]")).toBe(5);
    expect(lineOfPath(text, "provider.ignore")).toBe(2);
    expect(lineOfPath(text, "reasoning.effort")).toBeUndefined();
  });
});
