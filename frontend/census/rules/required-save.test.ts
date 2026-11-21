// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import { heldSubmitsIn, rule } from "./required-save.census";

// One greyed-out submit per line, in each shape the emptiness test takes.
const PLANTED = [
  'const a = <ConfirmModal confirmDisabled={name.trim() === ""} />;',
  "const b = <Button disabled={!subject.trim()} />;",
  'const ready = reason.trim() !== "";',
  "const c = <Button disabled={pending || !ready} />;",
  "const d = <ConfirmModal confirmDisabled={picked.length === 0} />;",
] as const;

// What the rule leaves alone: a held submit that says why, one held for a
// reason other than an empty value, and a plain disabled control.
const SPARED = [
  'const a = <ConfirmModal confirmDisabled={name === ""} confirmReason={why} />;',
  'const b = <Button disabled={note === ""} reason={why} />;',
  "const c = <Button disabled={pending} />;",
  "const d = <ConfirmModal confirmDisabled={unchanged} />;",
  'const e = <TextInput disabled={mode === ""} />;',
] as const;

describe("a form with a required field keeps its submit enabled", () => {
  it("finds each submit greyed out on an empty value", () => {
    const planted = parseSource("planted.tsx", PLANTED.join("\n"));
    expect(heldSubmitsIn(planted)).toEqual([1, 2, 4, 5]);
  });

  it("leaves explained, unrelated and plain controls alone", () => {
    const spared = parseSource("spared.tsx", SPARED.join("\n"));
    expect(heldSubmitsIn(spared)).toEqual([]);
  });

  it("reports through its own corpus, and spares tests and stories", () => {
    const text = 'const a = <Button disabled={x.trim() === ""} />;\n';
    expect(
      plantedFindings(rule, [
        { path: join(srcDir, "features", "p.tsx"), area: "src", text },
        { path: join(srcDir, "features", "p.test.tsx"), area: "src", text },
        { path: join(srcDir, "features", "p.stories.tsx"), area: "src", text },
      ]),
    ).toEqual(["src/features/p.tsx:1"]);
  });
});
