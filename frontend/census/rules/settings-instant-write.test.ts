// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import { instantWritesIn, rule } from "./settings-instant-write.census";

// One write on change per component, in each path a handler takes to it.
const PLANTED = `
function Direct() {
  return <Select onChange={(key) => save.mutate({ key })} />;
}
function Named() {
  const pick = (key) => save.mutateAsync({ key });
  return <SegmentedControl onChange={pick} />;
}
function Called() {
  function commit(key) {
    save.mutate({ key });
  }
  return <ChoiceList onChange={(key) => commit(key)} />;
}
function Field({ onPick }) {
  return <Radio onChange={() => onPick("all")} />;
}
function Page() {
  return <Field onPick={(scope) => update.mutate({ scope })} />;
}
function Toggle() {
  return <Switch onChange={(on) => update.mutate({ on })} />;
}
`;

// What the rule leaves alone: a draft, a verb, and a prop it cannot see fed.
const SPARED = `
function Draft() {
  return <Select onChange={(next) => { setRole(next); faults.edited("role"); }} />;
}
function Verb() {
  return <Button onClick={() => save.mutate({})} />;
}
function Passed({ onPick }) {
  return <Select onChange={onPick} />;
}
function Mutation() {
  return <Select onChange={(next) => setMutate(next)} />;
}
`;

describe("a settings pick waits for the page's save", () => {
  it("finds a write on change directly, through a function and through a prop", () => {
    const planted = parseSource("planted.tsx", PLANTED);
    expect(instantWritesIn(planted).map((write) => write.tag)).toEqual([
      "Select",
      "SegmentedControl",
      "ChoiceList",
      "Radio",
      "Switch",
    ]);
  });

  it("leaves a draft, a pressed verb and an unfed prop alone", () => {
    const spared = parseSource("spared.tsx", SPARED);
    expect(instantWritesIn(spared)).toEqual([]);
  });

  it("reports only a settings module, and lets a Switch write", () => {
    const text = [
      "const a = <Select onChange={(k) => save.mutate(k)} />;",
      "const b = <Switch onChange={(on) => save.mutate(on)} />;",
    ].join("\n");
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "settings", "p.tsx"),
          area: "src",
          text,
        },
        {
          path: join(srcDir, "features", "settings", "p.test.tsx"),
          area: "src",
          text,
        },
        { path: join(srcDir, "features", "deals", "p.tsx"), area: "src", text },
      ]),
    ).toEqual(["src/features/settings/p.tsx:1: <Select> writes on change"]);
  });
});

describe("the waivers", () => {
  const waived = join(srcDir, "features", "settings", "lossreasons.tsx");
  const roles = join(srcDir, "features", "settings", "roles-detail.tsx");
  const one = "const a = <Select onChange={(k) => decide.mutate(k)} />;\n";
  const toggle = "const t = <Switch onChange={(on) => save.mutate(on)} />;\n";

  it("spare a waived file that holds its counted writes", () => {
    expect(
      plantedFindings(rule, [
        { path: waived, area: "src", text: one + toggle },
        { path: roles, area: "src", text: one },
      ]),
    ).toEqual([]);
  });

  it("fail a waived file whose writes no longer match the count", () => {
    expect(
      plantedFindings(rule, [
        { path: waived, area: "src", text: toggle },
        { path: roles, area: "src", text: one + one },
      ]),
    ).toEqual([
      "src/features/settings/lossreasons.tsx: waived for 1 and holds 0; change its waiver to match",
      "src/features/settings/roles-detail.tsx: waived for 1 and holds 2; change its waiver to match",
    ]);
  });
});
