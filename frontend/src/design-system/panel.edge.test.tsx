/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render } from "@testing-library/react";
import { type RefObject, useEffect } from "react";
import { afterEach, expect, it } from "vitest";
import { Panel, usePanelEdge } from "./panel";

afterEach(cleanup);

type Edge = RefObject<HTMLElement | null> | null;

function ReportsEdge({ onEdge }: Readonly<{ onEdge: (edge: Edge) => void }>) {
  const edge = usePanelEdge();
  useEffect(() => {
    onEdge(edge);
  }, [edge, onEdge]);
  return null;
}

it("hands a control inside a Panel the Panel's own box", () => {
  const seen: { edge: Edge } = { edge: null };
  const { container } = render(
    <Panel title="Brand">
      <ReportsEdge
        onEdge={(edge) => {
          seen.edge = edge;
        }}
      />
    </Panel>,
  );
  expect(seen.edge?.current).toBe(container.querySelector("section.panel"));
});

it("hands a control outside any Panel nothing", () => {
  const seen: { edge: Edge } = { edge: { current: null } };
  render(
    <ReportsEdge
      onEdge={(edge) => {
        seen.edge = edge;
      }}
    />,
  );
  expect(seen.edge).toBeNull();
});
