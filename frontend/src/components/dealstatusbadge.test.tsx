/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { relative } from "node:path";
import { cleanup, render, screen } from "@testing-library/react";
import ts from "typescript";
import { afterEach, describe, expect, it } from "vitest";
import { reachedModules, SRC_ROOT } from "../../scripts/lib/bundle-reach";
import { parseSource } from "../../scripts/lib/source-tree";
import { LocaleProvider } from "../i18n";
import { DealStatusBadge } from "./dealstatusbadge";

afterEach(cleanup);

describe("the deal status badge", () => {
  it("names each status in the reader's language", () => {
    render(
      <LocaleProvider initial="vi">
        <DealStatusBadge status="open" />
        <DealStatusBadge status="won" />
      </LocaleProvider>,
    );
    expect(screen.getByText("Đang mở")).toBeTruthy();
    expect(screen.getByText("Thắng")).toBeTruthy();
  });

  it("reads a status this build has no word for as sent", () => {
    render(
      <LocaleProvider initial="vi">
        <DealStatusBadge status="paused" />
      </LocaleProvider>,
    );
    expect(screen.getByText("paused")).toBeTruthy();
  });
});

// A stored status drawn as JSX text, `{row.status}`, reaches every reader as
// the English wire code. Any receiver counts, since a deal is often `row`.
// Blind spot: a status first held in a local (`const s = row.status`) and drawn
// as `{s}` is not followed.
function rawStatusRenders(path: string, text: string): string[] {
  const found: string[] = [];
  const visit = (node: ts.Node): void => {
    if (
      ts.isJsxExpression(node) &&
      !ts.isJsxAttribute(node.parent) &&
      node.expression !== undefined &&
      drawsStatus(node.expression)
    ) {
      found.push(node.getText());
    }
    ts.forEachChild(node, visit);
  };
  visit(parseSource(path, text));
  return found;
}

// `key ? t(key) : …`: the branch translates the very key the condition tested.
function translatesItsCondition(expression: ts.ConditionalExpression): boolean {
  const branch = expression.whenTrue;
  return (
    ts.isCallExpression(branch) &&
    ts.isIdentifier(branch.expression) &&
    branch.expression.text === "t" &&
    branch.arguments[0]?.getText() === expression.condition.getText()
  );
}

// What a wrapper still draws: a cast, a `!`, `String(x)` or a template slot.
function inner(expression: ts.Expression): readonly ts.Expression[] {
  if (
    ts.isParenthesizedExpression(expression) ||
    ts.isAsExpression(expression) ||
    ts.isSatisfiesExpression(expression) ||
    ts.isNonNullExpression(expression) ||
    ts.isTypeAssertionExpression(expression)
  ) {
    return [expression.expression];
  }
  if (
    ts.isCallExpression(expression) &&
    ts.isIdentifier(expression.expression) &&
    expression.expression.text === "String"
  ) {
    return expression.arguments.slice(0, 1);
  }
  if (ts.isTemplateExpression(expression)) {
    return expression.templateSpans.map((span) => span.expression);
  }
  return [];
}

// The value a child expression draws: a fallback or a branch draws it too.
function drawsStatus(expression: ts.Expression): boolean {
  const wrapped = inner(expression);
  if (wrapped.length > 0) {
    return wrapped.some(drawsStatus);
  }
  if (ts.isConditionalExpression(expression)) {
    // `key ? t(key) : row.status` shows a code this build has no word for.
    if (translatesItsCondition(expression)) {
      return false;
    }
    return (
      drawsStatus(expression.whenTrue) || drawsStatus(expression.whenFalse)
    );
  }
  if (
    ts.isBinaryExpression(expression) &&
    [ts.SyntaxKind.QuestionQuestionToken, ts.SyntaxKind.BarBarToken].includes(
      expression.operatorToken.kind,
    )
  ) {
    return drawsStatus(expression.left) || drawsStatus(expression.right);
  }
  // `cond && row.status` draws only its right side.
  if (
    ts.isBinaryExpression(expression) &&
    expression.operatorToken.kind === ts.SyntaxKind.AmpersandAmpersandToken
  ) {
    return drawsStatus(expression.right);
  }
  // `row.status.replace(…)` and `.toUpperCase()` still draw the code.
  if (
    ts.isCallExpression(expression) &&
    ts.isPropertyAccessExpression(expression.expression)
  ) {
    return drawsStatus(expression.expression.expression);
  }
  return (
    ts.isPropertyAccessExpression(expression) &&
    expression.name.text === "status"
  );
}

describe("screens the shipped bundle loads", () => {
  const screens = [...reachedModules().keys()].filter((path) =>
    path.endsWith(".tsx"),
  );

  it("finds the raw form in a planted screen, and passes the badge", () => {
    expect(
      rawStatusRenders(
        "/app/src/planted.tsx",
        [
          `const a = <Badge>{deal.status}</Badge>;`,
          `const b = <Badge>{row.status}</Badge>;`,
          `const c = <span>{offer?.status ?? "-"}</span>;`,
          `const d = <span>{ok ? label : (item.status)}</span>;`,
          `const e = <DealStatusBadge status={deal.status} />;`,
          `const f = <span>{row.statusLabel}</span>;`,
          `const g = <span>{key ? t(key) : row.status}</span>;`,
          `const h = <span>{row.status as string}</span>;`,
          `const i = <span>{row.status!}</span>;`,
          `const j = <span>{\`\${row.status}\`}</span>;`,
          `const k = <span>{String(row.status)}</span>;`,
          `const l = <span>{ok ? t("deal.status.open") : row.status}</span>;`,
          `const m = <span>{(row.status satisfies string)}</span>;`,
          `const n = <span>{shown && row.status}</span>;`,
          `const o = <span>{row.status.replace("_", " ")}</span>;`,
          `const p = <span>{row.status?.toUpperCase()}</span>;`,
          `const q = <span>{row.status === "won" && <b>Won</b>}</span>;`,
          `const r = <span>{statusLabel(row.status)}</span>;`,
        ].join("\n"),
      ),
    ).toEqual([
      "{deal.status}",
      "{row.status}",
      '{offer?.status ?? "-"}',
      "{ok ? label : (item.status)}",
      "{row.status as string}",
      "{row.status!}",
      `{\`\${row.status}\`}`,
      "{String(row.status)}",
      '{ok ? t("deal.status.open") : row.status}',
      "{(row.status satisfies string)}",
      "{shown && row.status}",
      '{row.status.replace("_", " ")}',
      "{row.status?.toUpperCase()}",
    ]);
  });

  it("never draw a record's status as its stored code", () => {
    expect(screens.length).toBeGreaterThan(500);
    const offenders = screens.flatMap((path) =>
      rawStatusRenders(path, readFileSync(path, "utf8")).map(
        (found) => `${relative(SRC_ROOT, path)}: ${found}`,
      ),
    );
    expect(offenders).toEqual([]);
  });
});
