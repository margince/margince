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

// A deal's stored status drawn as JSX text, `{deal.status}`, reaches every
// reader as the English wire value. The status reads through DealStatusBadge.
function rawStatusRenders(path: string, text: string): string[] {
  const found: string[] = [];
  const visit = (node: ts.Node): void => {
    if (
      ts.isJsxExpression(node) &&
      !ts.isJsxAttribute(node.parent) &&
      node.expression !== undefined &&
      ts.isPropertyAccessExpression(node.expression) &&
      node.expression.name.text === "status" &&
      /deal/i.test(node.expression.expression.getText())
    ) {
      found.push(node.getText());
    }
    ts.forEachChild(node, visit);
  };
  visit(parseSource(path, text));
  return found;
}

describe("screens the shipped bundle loads", () => {
  const screens = [...reachedModules().keys()].filter((path) =>
    path.endsWith(".tsx"),
  );

  it("finds the raw form in a planted screen, and passes the badge", () => {
    expect(
      rawStatusRenders(
        "/app/src/planted.tsx",
        `const a = <Badge>{deal.status}</Badge>; const b = <DealStatusBadge status={deal.status} />;`,
      ),
    ).toEqual(["{deal.status}"]);
  });

  it("never draw a deal's status as its stored value", () => {
    expect(screens.length).toBeGreaterThan(500);
    const offenders = screens.flatMap((path) =>
      rawStatusRenders(path, readFileSync(path, "utf8")).map(
        (found) => `${relative(SRC_ROOT, path)}: ${found}`,
      ),
    );
    expect(offenders).toEqual([]);
  });
});
