// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DocumentTile } from "./documenttile";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("a tile's line of facts", () => {
  it("draws each fact as its own unit, skipping the empty ones", () => {
    render(
      <DocumentTile
        filename="offer.pdf"
        title="Offer"
        meta={["docs.google.com", null, "Offer", undefined, "19 Nov 2026"]}
      />,
    );
    const tile = screen.getByRole("article", { name: "Offer" });
    const facts = [...tile.querySelectorAll(".board-doc-fact")].map(
      (fact) => fact.textContent,
    );
    expect(facts).toEqual(["docs.google.com · ", "Offer · ", "19 Nov 2026"]);
  });

  it("keeps a fact that repeats, without a duplicate key", () => {
    const logged = vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <DocumentTile
        filename="contract.pdf"
        title="Contract"
        meta={["Contract", "Contract", "19 Nov 2026"]}
      />,
    );
    const tile = screen.getByRole("article", { name: "Contract" });
    expect(tile.querySelector(".board-doc-meta")?.textContent).toBe(
      "Contract · Contract · 19 Nov 2026",
    );
    expect(logged).not.toHaveBeenCalled();
  });

  it("draws no line when every fact is empty", () => {
    render(<DocumentTile filename="a.pdf" title="A" meta={[null, ""]} />);
    expect(document.querySelector(".board-doc-meta")).toBeNull();
  });
});
