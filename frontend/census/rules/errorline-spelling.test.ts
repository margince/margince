// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { type Area, plantedFindings, srcDir } from "../tree";
import {
  dangerOnlyClasses,
  findingsIn,
  rule,
} from "./errorline-spelling.census";

describe("the detector", () => {
  const read = (
    text: string,
    where = "frontend/src/screens/x.tsx",
    sheet = "",
  ) =>
    findingsIn(
      parseSource(where, text),
      where,
      dangerOnlyClasses([{ where: "frontend/src/screens/x.css", text: sheet }]),
    ).map(({ arm, says, waiver }) => `${arm} ${waiver}: ${says}`);

  it("reads an inline danger colour, and not another colour", () => {
    expect(
      read('<p style={{ color: "var(--dangerText)", marginTop: 8 }}>x</p>'),
    ).toEqual(["inline none: style colours the text var(--dangerText)"]);
    expect(read('<p style={{ color: "var(--textSecondary)" }}>x</p>')).toEqual(
      [],
    );
    expect(
      read('<p style={{ background: "var(--dangerText)" }}>x</p>'),
    ).toEqual([]);
  });

  it("reads the danger classes on any element, however assembled", () => {
    expect(read('<p className="t-danger">x</p>')).toEqual([
      "class none: <p> carries the t-danger class",
    ]);
    expect(read('<span className="t-caption form-error">x</span>')).toEqual([
      "class none: <span> carries the form-error class",
    ]);
    expect(read('<div className={cx("t-sub", "t-danger")}>x</div>')).toEqual([
      "class none: <div> carries the t-danger class",
    ]);
  });

  it("reads the danger class on a small, a cell and a component", () => {
    expect(read('<small className="t-danger">x</small>')).toEqual([
      "class none: <small> carries the t-danger class",
    ]);
    expect(read('<td className="t-num t-danger">x</td>')).toEqual([
      "class none: <td> carries the t-danger class",
    ]);
    expect(read('<Text className="t-danger">x</Text>')).toEqual([
      "class none: <Text> carries the t-danger class",
    ]);
  });

  it("reads a class a sheet spells as the danger ink, and not a box", () => {
    const line = '<p className="t-sub x-error">x</p>';
    expect(
      read(
        line,
        undefined,
        ".x-error { color: var(--dangerText); margin-top: 4px; }",
      ),
    ).toEqual([
      "sheet none: <p> carries .x-error, a rule that only sets the danger ink",
    ]);
    expect(
      read(
        line,
        undefined,
        ".x-error { color: var(--dangerText); border: 1px solid; }",
      ),
    ).toEqual([]);
    expect(
      read(
        line,
        undefined,
        ".x-error { color: var(--dangerText); }\n.x-error { display: flex; }",
      ),
    ).toEqual([]);
    const derives = [
      ".x-error { color: var(--dangerText) !important; }",
      ".x-error { color : var(--dangerText, #b00); }",
      ".x-error { color: var(--dangerText); }\n@media (max-width: 720px) { .x-error { margin-top: 0; } }",
    ];
    for (const sheet of derives) {
      expect(read(line, undefined, sheet), sheet).toHaveLength(1);
    }
    expect(
      read(line, undefined, ".row > .x-error { color: var(--dangerText); }"),
    ).toEqual([]);
    expect(
      dangerOnlyClasses([
        {
          where: "frontend/src/screens/x.css",
          text: ".t-danger { color: var(--dangerText); }\n.x-gap { margin: 0; }",
        },
        {
          where: "frontend/src/design-system/x.css",
          text: ".x-danger { color: var(--dangerText); }",
        },
      ]),
    ).toEqual(new Set());
  });

  it("does not read Field's slot or a longer class", () => {
    expect(read('<p className="field-error">x</p>')).toEqual([]);
    expect(read('<p className="t-danger-strong">x</p>')).toEqual([]);
  });

  it("reads a problem message said alone in an element", () => {
    expect(read("<p>{problemMessageOf(save.error, t)}</p>")).toEqual([
      "message none: <p> says a failure message outside ErrorLine",
    ]);
    expect(
      read('<span className="x">\n  {common.problemMessageOf(e, t)}\n</span>'),
    ).toEqual([
      "message none: <span> says a failure message outside ErrorLine",
    ]);
  });

  it("reads a problem message one hop away, through a variable or a prop", () => {
    expect(
      read(
        "const message = save.isError ? problemMessageOf(save.error, t) : null;\n" +
          "const a = <p>{message}</p>;\n" +
          "const b = <Row previewError={problemMessageOf(e, t)} />;\n" +
          'const c = <p className="row-error">{previewError}</p>;',
      ),
    ).toEqual([
      "message none: <p> says a failure message outside ErrorLine",
      "message none: <p> says a failure message outside ErrorLine",
    ]);
    expect(read('const label = t("x");\n<p>{label}</p>')).toEqual([]);
  });

  it("does not read a problem message among other children, or inside ErrorLine", () => {
    expect(read('<p>{t("x.lead")} {problemMessageOf(e, t)}</p>')).toEqual([]);
    expect(
      read("<p>{problemMessageOf(e, t)}<Button>Retry</Button></p>"),
    ).toEqual([]);
    expect(read("<ErrorLine>{problemMessageOf(e, t)}</ErrorLine>")).toEqual([]);
  });

  it("does not read a cause under an alert, and reads it under anything else", () => {
    const cause = "<p>{problemMessageOf(e, t)}</p>";
    const said = "message none: <p> says a failure message outside ErrorLine";
    expect(read(`<div role="alert"><h3>Failed</h3>${cause}</div>`)).toEqual([]);
    expect(read(`<Card role="status">${cause}</Card>`)).toEqual([said]);
    expect(
      read(`<div aria-live="polite"><AssistantBubble />${cause}</div>`),
    ).toEqual([said]);
    expect(read(`<div><h3>Failed</h3>${cause}</div>`)).toEqual([said]);
  });

  it("reads an alerting line, however the role and the words are written", () => {
    expect(read('<p role="alert">{t("x")}</p>')).toEqual([
      "alert none: <p> hand-rolls an alerting line",
    ]);
    expect(read('<span role={"alert"}>{msg}</span>')).toEqual([
      "alert none: <span> hand-rolls an alerting line",
    ]);
    // biome-ignore lint/suspicious/noTemplateCurlyInString: a fixture of source code
    expect(read('<div role="alert">Failed: {`${n} rows`}</div>')).toEqual([
      "alert none: <div> hand-rolls an alerting line",
    ]);
  });

  it("does not read a composite alert, another role, or a component", () => {
    expect(
      read('<div role="alert"><strong>Sync stopped.</strong> text</div>'),
    ).toEqual([]);
    expect(read('<div role="alert">\n  <Glyph />\n  {msg}\n</div>')).toEqual(
      [],
    );
    expect(read('<p role="status">x</p>')).toEqual([]);
    expect(read('<Callout role="alert">x</Callout>')).toEqual([]);
  });

  it("lets a design-system owner keep its alert, and only an owner", () => {
    const slot =
      '<p className="field-error" id={errorId} role="alert">{error}</p>';
    expect(read(slot, "frontend/src/design-system/field.tsx")).toEqual([]);
    expect(read(slot)).toEqual(["alert none: <p> hand-rolls an alerting line"]);
  });

  it("reads RefusalLine declared and rendered, and not imported", () => {
    expect(
      read(
        'import { RefusalLine } from "./common";\n' +
          "export function RefusalLine() { return null; }\n" +
          "const x = <RefusalLine error={e} />;",
      ),
    ).toEqual([
      "name none: RefusalLine is the retired spelling",
      "name none: RefusalLine is the retired spelling",
    ]);
  });

  it("reads a waiver on the line or above it, and a bare one as bare", () => {
    expect(
      read(
        "<>\n  {/* ds:ignore the host page owns this ink */}\n" +
          '  <p className="t-danger">x</p>\n' +
          '  <p className="t-danger">y</p> {/* ds:ignore */}\n' +
          '  <p style={{ color: "var(--dangerText)" }}>z</p> // ds:ignore embedded widget\n</>',
      ),
    ).toEqual([
      "class reasoned: <p> carries the t-danger class",
      "class bare: <p> carries the t-danger class",
      "inline reasoned: style colours the text var(--dangerText)",
    ]);
  });
  it("reads an announced problem message on both arms", () => {
    expect(read('<p role="alert">{problemMessageOf(e, t)}</p>')).toEqual([
      "message none: <p> says a failure message outside ErrorLine",
      "alert none: <p> hand-rolls an alerting line",
    ]);
  });
});

describe("the errorline-spelling rule", () => {
  const area: Area = "src";
  const planted = (text: string) =>
    plantedFindings(rule, [
      { path: join(srcDir, "features", "planted.tsx"), area, text },
      {
        path: join(srcDir, "features", "planted.css"),
        area,
        text: ".x-error { color: var(--dangerText); }",
      },
    ]);

  it("reports a planted line through its own corpus, sheets included", () => {
    expect(
      planted('export const A = () => <p className="x-error">x</p>;'),
    ).toEqual([
      "frontend/src/features/planted.tsx:1  <p> carries .x-error, a rule that only sets the danger ink — use ErrorLine from design-system/errorline.tsx",
    ]);
  });

  it("passes a reasoned waiver and reports a bare one", () => {
    expect(
      planted(
        'export const A = () => <p className="t-danger">x</p>; // ds:ignore the host page owns this ink',
      ),
    ).toEqual([]);
    expect(
      planted(
        'export const A = () => <p className="t-danger">x</p>; // ds:ignore',
      ),
    ).toEqual([
      "frontend/src/features/planted.tsx:1  <p> carries the t-danger class — use ErrorLine from design-system/errorline.tsx",
      "frontend/src/features/planted.tsx:1  <p> carries the t-danger class — use ErrorLine from design-system/errorline.tsx (ds:ignore says a line was skipped and not why; write the reason after it)",
    ]);
  });
});
