// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { UNDERLAY_HOSTS } from "../../scripts/lib/surface-ladder";
import { probe, ref } from "../../scripts/lib/surface-sheets";
import { plantedFindings, srcDir } from "../tree";
import {
  hostFindings,
  knockoutFills,
  rule,
  underlayHosts,
} from "./surfaces.layers.census";

describe("the layer detectors", () => {
  it("reads a layer's host through a var(), a calc() and a state", () => {
    expect(
      underlayHosts([
        probe(":root", "--under: -1"),
        probe(".f::before:hover", "z-index: -1"),
        probe(".g", `z-index: ${ref("--under")}`),
        probe(".h::after", "z-index: calc(-1 * 2)"),
        probe(".k:hover", "background: radial-gradient(red, blue)"),
        probe(".flat", "background: linear-gradient(red, red)"),
        probe(".up::before", "z-index: calc(2 - 1)"),
        probe(".m::after:focus-visible", "z-index: -1"),
        probe(".n:focus-within", "background: radial-gradient(red, blue)"),
      ]),
    ).toEqual([".f", ".h", ".k", ".m", ".n", ":has(> .g)"]);
  });

  it("reads an opaque fill for a layer that hides the border-box one under it", () => {
    const planted = probe(
      ".probe",
      "background: linear-gradient(var(--ground), var(--ground)) padding-box, " +
        "linear-gradient(var(--pane) , var(--pane)) padding-box, " +
        "linear-gradient(red, blue) border-box",
    );
    expect(knockoutFills([planted])).toEqual([".probe: var(--pane) in dark"]);
    expect(knockoutFills([])).toEqual(["no knockout background was read"]);
  });

  it("names a host on one side of UNDERLAY_HOSTS only", () => {
    const declared = [...UNDERLAY_HOSTS, ".auth-surface"];
    expect(hostFindings(declared)).toEqual([]);
    expect(hostFindings([...declared, ".stray"])).toEqual([
      ".stray stacks a layer and is not in UNDERLAY_HOSTS",
    ]);
    expect(hostFindings(declared.slice(1))).toEqual([
      `${UNDERLAY_HOSTS[0]} is in UNDERLAY_HOSTS and stacks no layer`,
    ]);
  });
});

describe("the surfaces.layers rule", () => {
  it("reports a planted host through its own corpus and across", () => {
    const findings = plantedFindings(rule, [
      {
        path: join(srcDir, "features", "planted.css"),
        area: "src",
        text: ".stray::before { z-index: -1; }\n",
      },
    ]);
    expect(findings).toContain(
      ".stray stacks a layer and is not in UNDERLAY_HOSTS",
    );
    expect(findings).toContain("no knockout background was read");
  });
});
