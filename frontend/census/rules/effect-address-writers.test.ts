// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir, treeOf } from "../tree";
import { addressWriters, rule } from "./effect-address-writers.census";

const URL =
  'import { replaceDial, replaceParams } from "../../lib/urlstate";\n';
const ROUTER = 'import { navigateReplacing } from "../../lib/router";\n';
const REACT =
  'import { useCallback, useEffect, useEffectEvent } from "react";\n';

// One file per shape, so a finding names the shape it came from.
const FLAGGED: Record<string, string> = {
  direct: `${URL}${REACT}useEffect(() => { replaceDial("q", undefined); }, []);`,
  aliased: `import { replaceDial as setDial } from "../../lib/urlstate";\n${REACT}useEffect(() => { setDial("q", undefined); }, []);`,
  namespace: `import * as url from "../../lib/urlstate";\n${REACT}useEffect(() => { url.replaceDial("q", undefined); }, []);`,
  dynamic: `${REACT}useEffect(() => { void import("../../lib/urlstate").then((m) => m.replaceDial("q", undefined)); }, []);`,
  namedFunction: `${URL}${REACT}function sync() { replaceParams(new Map()); }\nuseEffect(sync, []);`,
  namedArrow: `${URL}${REACT}const sync = () => { replaceParams(new Map()); };\nuseEffect(sync, []);`,
  keptCallback: `${URL}${REACT}const sync = useCallback(() => { replaceParams(new Map()); }, []);\nuseEffect(sync, [sync]);`,
  helper: `${URL}${REACT}const clear = () => replaceDial("q", undefined);\nuseEffect(() => clear(), []);`,
  asValue: `${URL}${REACT}useEffect(() => { const w = replaceDial; w("q", undefined); }, []);`,
  asCallback: `${URL}${REACT}declare const onDone: (f: unknown) => void;\nuseEffect(() => { onDone(replaceParams); }, []);`,
  cleanup: `${URL}${REACT}useEffect(() => () => replaceDial("q", undefined), []);`,
  reactNamespace: `${URL}import * as React from "react";\nReact.useEffect(() => { replaceDial("q", undefined); }, []);`,
  aliasedEffect: `${URL}import { useEffect as useE } from "react";\nuseE(() => { replaceDial("q", undefined); }, []);`,
  effectEvent: `${URL}${REACT}const onSync = useEffectEvent(() => replaceDial("q", undefined));`,
  onSuccess: `${ROUTER}declare const useMutation: (o: object) => void;\nuseMutation({ onSuccess: () => navigateReplacing({ screen: "home" }) });`,
  promiseThen: `${ROUTER}declare const saved: Promise<void>;\nvoid saved.then(() => navigateReplacing({ screen: "home" }));`,
  afterAwait: `${ROUTER}declare const save: () => Promise<void>;\nexport async function submit() { await save(); navigateReplacing({ screen: "home" }); }`,
  crossFile: `import { openAsk } from "../../lib/urlstate";
declare const useMutation: (o: object) => void;
useMutation({ onSuccess: () => openAsk("") });`,
};

const CLEAN: Record<string, string> = {
  boundShadow: `import { replaceDial, useAddressWriters } from "../../lib/urlstate";\n${REACT}export function C() { const { replaceDial } = useAddressWriters(); useEffect(() => { replaceDial("q", undefined); }, [replaceDial]); }\nexport const press = () => replaceDial("q", "1");`,
  boundParam: `${URL}${REACT}export function C({ replaceDial }: { replaceDial: (k: string, v?: string) => void }) { useEffect(() => { replaceDial("q", undefined); }, []); }`,
  shadowInEffect: `${URL}${REACT}declare const w: { replaceDial: () => void };\nuseEffect(() => { const replaceDial = w.replaceDial; replaceDial(); }, []);`,
  boundRenamed: `import { useAddressWriters } from "../../lib/urlstate";\n${REACT}export function C() { const { replaceDial: setDial } = useAddressWriters(); useEffect(() => { setDial("q", undefined); }, [setDial]); }`,
  click: `${URL}export const onClick = () => replaceDial("q", "1");`,
  departure: `${ROUTER}const from = () => ({ go: () => navigateReplacing({ screen: "home" }) });\ndeclare const useMutation: (o: object) => void;\nuseMutation({ onSuccess: () => from() });`,
  lazyScreen: `${URL}export function Screen() { replaceDial("q", "1"); return null; }
void Promise.resolve().then(() => Screen);`,
};

const planted = (files: Record<string, string>) =>
  Object.entries(files).map(([name, text]) => ({
    path: join(srcDir, "features", "probe", `${name}.tsx`),
    area: "src" as const,
    text,
  }));

describe("the effect-address-writers rule", () => {
  const findings = plantedFindings(rule, [
    ...planted(FLAGGED),
    ...planted(CLEAN),
  ]);
  const flaggedIn = new Set(
    findings.map((line) => /features\/probe\/(\w+)\.tsx/.exec(line)?.[1]),
  );

  it.each(Object.keys(FLAGGED))("sees the %s shape", (shape) => {
    expect(flaggedIn.has(shape), findings.join("\n")).toBe(true);
  });

  it.each(Object.keys(CLEAN))("leaves the %s shape", (shape) => {
    expect(flaggedIn.has(shape), findings.join("\n")).toBe(false);
  });

  it("names the line and the writer it runs", () => {
    expect(findings).toContain(
      "src/features/probe/crossFile.tsx:3 — runs the module-level `replaceParams` through `openAsk` after its render; take it from useAddressWriters()",
    );
  });

  it("takes its writers from AddressWriters, each an exported function", () => {
    const tree = treeOf(planted({ direct: FLAGGED.direct }));
    const { byFunction, floors } = addressWriters(tree.checked());
    expect(floors).toEqual([]);
    expect(new Set(byFunction.values())).toEqual(
      new Set(["navigateReplacing", "replaceDial", "replaceParams"]),
    );
  });
});
