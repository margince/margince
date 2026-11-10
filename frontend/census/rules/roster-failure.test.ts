// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { rule } from "./roster-failure.census";

const findings = (body: string) =>
  plantedFindings(rule, [
    {
      path: join(srcDir, "features", "planted.tsx"),
      area: "src",
      text: `function Picker() {\n${body}\n}\n`,
    },
  ]);

describe("roster failure", () => {
  it("reports a roster read only for its data", () => {
    expect(
      findings("const roster = useRoster();\nreturn roster.data ?? [];"),
    ).toEqual([
      "src/features/planted.tsx:2: `roster` is read only for its data",
    ]);
    expect(findings("const { data } = useRoster();")).toEqual([
      "src/features/planted.tsx:2: the destructuring takes no failure field",
    ]);
    expect(findings("const members = useRoster().data;")).toEqual([
      "src/features/planted.tsx:2: the call is read only for `.data`",
    ]);
  });

  it("passes a read of the failure, a hand-off, and a waiver", () => {
    expect(
      findings("const roster = useRoster();\nif (roster.isError) return null;"),
    ).toEqual([]);
    expect(findings("const { data, isError } = useRoster();")).toEqual([]);
    expect(
      findings(
        "const roster = useRoster();\nreturn <RosterFailedLine query={roster} />;",
      ),
    ).toEqual([]);
    expect(
      findings("// roster-failure: ids only\nconst { data } = useRoster();"),
    ).toEqual([]);
  });

  it("reports a call the text spells and the parse does not see", () => {
    expect(
      findings("// calls useRoster() below\nconst { error } = useRoster();"),
    ).toEqual([
      "src/features/planted.tsx: spells useRoster( 2 times, parses 1",
    ]);
  });
});
