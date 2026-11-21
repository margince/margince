// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { jsonResponse, render } from "../../testing/settings.testkit";
import { JurisdictionEntitiesPanel } from "./jurisdictionentities";

// The Jurisdiction page lists each company's jurisdiction: the default company
// follows the selection, a further one states its own.

const entity = (id: string, fields: object) => ({
  id,
  address: {},
  member_count: 1,
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  ...fields,
});

function serve(rows: object[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse({ data: rows })),
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("lists each company with the jurisdiction whose law it answers under, not its address country", async () => {
  serve([
    entity("le-home", { is_default: true, display_name: "Gradion GmbH" }),
    entity("le-vn", {
      is_default: false,
      display_name: "Gradion Vietnam",
      country: "sg",
      jurisdiction: "vn",
    }),
  ]);
  render(<JurisdictionEntitiesPanel selected="de" />);
  expect(await screen.findByText("Gradion Vietnam")).toBeInTheDocument();
  expect(screen.getByText("Vietnam")).toBeInTheDocument();
  expect(screen.queryByText("Singapore")).toBeNull();
  expect(
    screen.getByText(/^Germany, the selection above$/),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("link", { name: en["jurisdiction.entities.edit"] }),
  ).toBeInTheDocument();
});

it("leads with the list, without the sentence about each country", async () => {
  serve([
    entity("le-home", { is_default: true, display_name: "Gradion GmbH" }),
    entity("le-vn", { is_default: false, display_name: "Gradion Vietnam" }),
  ]);
  render(<JurisdictionEntitiesPanel selected="de" />);
  expect(await screen.findByText("Gradion Vietnam")).toBeInTheDocument();
  expect(screen.queryByText(/sets the law for the mail/)).toBeNull();
  expect(
    screen.getByText(en["jurisdiction.entities.none"]),
  ).toBeInTheDocument();
});

it("shows nothing while the installation trades as one company", async () => {
  serve([
    entity("le-home", { is_default: true, display_name: "Gradion GmbH" }),
  ]);
  const { container } = render(<JurisdictionEntitiesPanel selected="de" />);
  await vi.waitFor(() => expect(vi.mocked(fetch)).toHaveBeenCalled());
  expect(container).toBeEmptyDOMElement();
});
