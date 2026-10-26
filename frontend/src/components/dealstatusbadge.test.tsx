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

function translates(expression: ts.Expression): boolean {
  return (
    ts.isCallExpression(expression) &&
    ts.isIdentifier(expression.expression) &&
    expression.expression.text === "t"
  );
}

// The value a child expression draws: a fallback or a branch draws it too.
function drawsStatus(expression: ts.Expression): boolean {
  if (ts.isParenthesizedExpression(expression)) {
    return drawsStatus(expression.expression);
  }
  if (ts.isConditionalExpression(expression)) {
    // `key ? t(key) : row.status` shows a code this build has no word for.
    if (translates(expression.whenTrue)) {
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
        ].join("\n"),
      ),
    ).toEqual([
      "{deal.status}",
      "{row.status}",
      '{offer?.status ?? "-"}',
      "{ok ? label : (item.status)}",
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
