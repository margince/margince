// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en as englishCopy } from "../i18n/en";
import { RecordDetails } from "./recorddetails";

afterEach(cleanup);

const domain = { key: "domain", term: "Domain", value: "brandt.example" };
const sections = [
  {
    key: "about",
    label: "About",
    rows: [domain, { key: "size", term: "Size", value: "Add…", empty: true }],
  },
  {
    key: "registration",
    label: "Registration",
    rows: [{ key: "vat", term: "VAT ID", value: "Add…", empty: true }],
  },
];

describe("Record details", () => {
  it("folds the empty rows of every group behind one count", () => {
    render(<RecordDetails sections={sections} />);
    expect(screen.getByText("brandt.example")).toBeInTheDocument();
    expect(screen.queryByText("Size")).toBeNull();
    expect(
      screen.getByRole("button", { name: "Show 2 empty fields" }),
    ).toHaveAttribute("aria-expanded", "false");
  });

  it("draws no label for a group whose every row is folded", () => {
    render(<RecordDetails sections={sections} />);
    expect(screen.getByRole("heading", { name: "About" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Registration" })).toBeNull();
  });

  it("opens the empty rows in place, and folds them again", () => {
    render(<RecordDetails sections={sections} />);
    fireEvent.click(
      screen.getByRole("button", { name: "Show 2 empty fields" }),
    );
    expect(screen.getByText("Size")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Registration" }),
    ).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", {
        name: englishCopy["record.details.hideEmpty"],
      }),
    );
    expect(screen.queryByText("VAT ID")).toBeNull();
  });

  it("draws an Edit link only on a section that can be edited, named for it", async () => {
    const user = userEvent.setup();
    const onEdit = vi.fn();
    render(
      <RecordDetails
        sections={[
          { key: "about", label: "About", rows: [domain], edit: { onEdit } },
          { key: "lists", label: "Lists", children: "Key accounts" },
        ]}
      />,
    );
    const link = screen.getByRole("button", { name: "Edit About" });
    expect(link).toHaveTextContent("Edit");
    expect(screen.queryByRole("button", { name: "Edit Lists" })).toBeNull();
    await user.click(link);
    expect(onEdit).toHaveBeenCalledTimes(1);
  });

  it("keeps an editable section's pane and Edit link while all its rows are folded", async () => {
    const user = userEvent.setup();
    render(
      <RecordDetails
        sections={[
          { key: "about", label: "About", rows: [domain] },
          {
            key: "address",
            label: "Address",
            edit: { onEdit: vi.fn() },
            rows: [
              { key: "street", term: "Street", value: "Add…", empty: true },
            ],
          },
          {
            key: "registration",
            label: "Registration",
            rows: [{ key: "vat", term: "VAT ID", value: "Add…", empty: true }],
          },
        ]}
      />,
    );
    expect(
      screen.getByRole("heading", { name: "Address" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Edit Address" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("Street")).toBeNull();
    expect(screen.queryByRole("heading", { name: "Registration" })).toBeNull();

    await user.click(
      screen.getByRole("button", { name: "Show 2 empty fields" }),
    );
    expect(screen.getByText("Street")).toBeInTheDocument();
  });

  it("offers no toggle when nothing is empty, and prints the footnote", () => {
    render(
      <RecordDetails
        sections={[{ key: "a", label: "About", rows: [domain] }]}
        footnote="Closing a project asks for a reason."
      />,
    );
    expect(screen.queryByRole("button")).toBeNull();
    expect(
      screen.getByText("Closing a project asks for a reason."),
    ).toBeInTheDocument();
  });
});
