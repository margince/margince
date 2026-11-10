// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { type Area, plantedFindings, srcDir } from "../tree";
import { monoViolations, rule } from "./mono.census";

describe("mono is for code", () => {
  it("refuses mono on a rule that does not dress code", () => {
    const refused = (css: string, file = "src/screens/x.css") =>
      monoViolations(file, css).length > 0;
    expect(refused(".foo { font-family: var(--fontFamilyMono); }")).toBe(true);
    expect(refused('.foo { font: 12px/1 "Geist Mono", monospace; }')).toBe(
      true,
    );
    expect(refused(".foo { font-family: ui-monospace; }")).toBe(true);
    // One selector in the list that is not code puts mono on that element.
    expect(refused(".foo, code { font-family: var(--fontFamilyMono); }")).toBe(
      true,
    );
    // The subject is what is dressed: here it is the label, not the code.
    expect(refused("code .label { font-family: var(--fontFamilyMono); }")).toBe(
      true,
    );
    expect(
      refused(".code-block .label { font-family: var(--fontFamilyMono); }"),
    ).toBe(true);
    expect(
      refused(".a { code & { font-family: var(--fontFamilyMono); } }"),
    ).toBe(true);
    expect(
      refused("@media (width > 1px) { .a { font-family: monospace; } }"),
    ).toBe(true);
    // A token of its own is a second way to spell the face.
    expect(refused(":root { --code-face: var(--fontFamilyMono); }")).toBe(true);
    expect(refused(':root { --fontFamilyMono: "Geist Mono"; }')).toBe(true);
    expect(refused(".t-mono { color: red; }")).toBe(true);
  });

  it("allows mono on code, pre, samp and .code-block, and on the token", () => {
    const allowed = (css: string, file = "src/screens/x.css") =>
      monoViolations(file, css);
    expect(
      allowed(".foo code { font-family: var(--fontFamilyMono); }"),
    ).toEqual([]);
    expect(
      allowed("code, pre, samp { font-family: var(--fontFamilyMono); }"),
    ).toEqual([]);
    expect(allowed("pre.code-block:hover { font-family: monospace; }")).toEqual(
      [],
    );
    expect(
      allowed(".code-block { font-family: var(--fontFamilyMono); }"),
    ).toEqual([]);
    expect(
      allowed(".a { & code { font-family: var(--fontFamilyMono); } }"),
    ).toEqual([]);
    expect(
      allowed("@media (width > 1px) { pre { font-family: monospace; } }"),
    ).toEqual([]);
    expect(
      allowed(
        ':root { --fontFamilyMono: "Geist Mono", ui-monospace, monospace; }',
        "src/design-system/tokens.css",
      ),
    ).toEqual([]);
    // A comment discussing the rule is not breaking it.
    expect(
      allowed("/* .t-mono and font-family: monospace were */ .a { }"),
    ).toEqual([]);
  });

  it("reads the class and the family in markup and in strings", () => {
    const refused = (source: string, file = "src/screens/x.tsx") =>
      monoViolations(file, source).length > 0;
    expect(refused('export const A = () => <span className="t-mono" />;')).toBe(
      true,
    );
    expect(
      refused(
        // biome-ignore lint/suspicious/noTemplateCurlyInString: a fixture of source code; the `${x}` is the template literal the scan reads
        "export const A = (x: string) => <b className={`${x} t-mono`} />;",
      ),
    ).toBe(true);
    expect(
      refused(
        'export const A = () => <b style={{ fontFamily: "var(--fontFamilyMono)" }} />;',
      ),
    ).toBe(true);
    expect(refused("const face = { fontFamily: 'monospace' };")).toBe(true);
    expect(refused('const sheet = ".a { font-family: Geist Mono; }";')).toBe(
      true,
    );
    expect(
      refused('<div style="font-family: monospace">x</div>', "index.html"),
    ).toBe(true);
    expect(refused('<b class="t-mono">1</b>', "index.html")).toBe(true);

    const allowed = (source: string, file = "src/screens/x.tsx") =>
      monoViolations(file, source);
    expect(allowed('const a = "t-monochrome t-num";')).toEqual([]);
    expect(
      allowed("// the t-mono class and var(--fontFamilyMono) are gone"),
    ).toEqual([]);
    expect(
      allowed(
        'const sheet = "pre code { font-family: var(--fontFamilyMono); }";',
      ),
    ).toEqual([]);
    expect(
      allowed("<style>pre { font-family: monospace; }</style>", "index.html"),
    ).toEqual([]);
  });

  it("reports a planted mono face through its own corpus", () => {
    const area: Area = "src";
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "planted.css"),
          area,
          text: ".id { font-family: var(--fontFamilyMono); }",
        },
        {
          path: join(srcDir, "features", "planted.test.ts"),
          area,
          text: 'const a = "t-mono";',
        },
      ]),
    ).toEqual([
      'src/features/planted.css:1 font-family names a mono family on ".id" — mono is for code, pre, samp and .code-block only',
    ]);
  });
});
