// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { ghostCount } from "./deal360/dealcommittee";
import { PersonNetworkTab, ringLayout } from "./personnetwork";

type GraphNode = components["schemas"]["PersonGraphNode"];
type DealCoverageRisk = components["schemas"]["DealCoverageRisk"];

const node = (id: string, group: GraphNode["group"]): GraphNode => ({
  id,
  type: group === "anchor" ? "contact" : "colleague",
  group,
  label: id,
});

describe("the ring places the contact at its centre", () => {
  it("puts the anchor in the middle and everyone else off it", () => {
    const placed = ringLayout([
      node("anchor", "anchor"),
      node("a", "direct"),
      node("b", "account"),
    ]);
    const anchor = placed.find((p) => p.node.id === "anchor");
    expect(anchor).toMatchObject({ x: 130, y: 130 });
    for (const other of placed.filter((p) => p.node.id !== "anchor")) {
      expect({ x: other.x, y: other.y }).not.toEqual({ x: 130, y: 130 });
    }
  });

  it("draws the same picture twice, so a rep learns where to look", () => {
    const nodes = [
      node("anchor", "anchor"),
      node("a", "direct"),
      node("b", "direct"),
    ];
    expect(ringLayout(nodes)).toEqual(ringLayout(nodes));
  });

  it("puts the first neighbour at twelve o'clock", () => {
    const placed = ringLayout([node("anchor", "anchor"), node("a", "direct")]);
    const first = placed.find((p) => p.node.id === "a");
    expect(first?.x).toBeCloseTo(130);
    expect(first?.y).toBeCloseTo(34);
  });

  // A contact nobody has spoken to arrives as an anchor alone. Dividing by an
  // empty ring would place it at NaN and the whole tab would draw blank.
  it("survives a contact with nobody around them", () => {
    const placed = ringLayout([node("anchor", "anchor")]);
    expect(placed).toHaveLength(1);
    expect(Number.isNaN(placed[0].x)).toBe(false);
  });

  // The server can withhold the anchor's own group. The ring then has no
  // centre, and every neighbour must still land somewhere drawable.
  it("survives a payload with no anchor at all", () => {
    const placed = ringLayout([node("a", "direct"), node("b", "account")]);
    expect(placed).toHaveLength(2);
    for (const p of placed) {
      expect(Number.isNaN(p.x)).toBe(false);
      expect(Number.isNaN(p.y)).toBe(false);
    }
  });
});

describe("a missing seat is counted from the coverage rules", () => {
  const risk = (kind: DealCoverageRisk["kind"]): DealCoverageRisk => ({
    kind,
    summary: kind,
  });

  it("counts the two rules that mean a seat is missing", () => {
    expect(
      ghostCount([risk("coverage_gap"), risk("single_threaded_theirs")]),
    ).toBe(2);
  });

  // A cold deal is not an uncovered one. Drawing a ghost for going_cold would
  // claim a seat nobody said was missing.
  it("does not count a rule that means cold rather than missing", () => {
    expect(
      ghostCount([
        risk("going_cold"),
        risk("champion_left"),
        risk("single_threaded_ours"),
      ]),
    ).toBe(0);
  });

  it("counts nothing on a deal with no findings", () => {
    expect(ghostCount([])).toBe(0);
  });
});

describe("the ring is drawn, and only when it says something", () => {
  const graphWith = (nodes: unknown[], edges: unknown[] = []) => ({
    person_id: "p-1",
    nodes,
    edges,
    groups_omitted: [],
    dropped_count: { direct: 0, account: 0 },
  });

  function draw(body: unknown) {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify(body), {
            status: 200,
            headers: { "content-type": "application/json" },
          }),
      ),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    return render(
      <QueryClientProvider client={client}>
        <PersonNetworkTab personId="p-1" />
      </QueryClientProvider>,
    );
  }

  // A ring carrying one dot is a smudge, not a picture — the lists beside it
  // already say nobody reaches this contact.
  it("draws no ring for a contact standing alone", async () => {
    const { container } = draw(
      graphWith([
        { id: "person:p-1", type: "contact", group: "anchor", label: "Dana" },
      ]),
    );
    await waitFor(() =>
      expect(screen.getByText(/Who reaches them/i)).toBeTruthy(),
    );
    expect(container.querySelector("svg.pn-ring")).toBeNull();
    vi.unstubAllGlobals();
  });

  it("draws the ring, and one node per person on it", async () => {
    const { container } = draw(
      graphWith(
        [
          { id: "person:p-1", type: "contact", group: "anchor", label: "Dana" },
          { id: "user:u-1", type: "colleague", group: "direct", label: "Lena" },
          {
            id: "person:p-2",
            type: "contact",
            group: "account",
            label: "Tomas",
          },
        ],
        [
          {
            from: "user:u-1",
            to: "person:p-1",
            strength_bucket: "strong",
            interactions_90d: 24,
          },
        ],
      ),
    );
    await waitFor(() =>
      expect(container.querySelector("svg.pn-ring")).toBeTruthy(),
    );
    const svg = container.querySelector("svg.pn-ring");
    expect(svg?.getAttribute("aria-hidden")).toBe("true");
    expect(svg?.querySelectorAll("circle")).toHaveLength(3);
    // The band drives the stroke, so the picture cannot disagree with the list.
    expect(svg?.querySelector("line")?.getAttribute("class")).toContain(
      "pn-edge-strong",
    );
    // Every node in the picture is a real button beside it.
    expect(screen.getByRole("button", { name: /Lena/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /Tomas/ })).toBeTruthy();
    vi.unstubAllGlobals();
  });
});
