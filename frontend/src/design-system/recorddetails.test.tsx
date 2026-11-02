// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LockedValue } from "./lockedvalue";
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
    fireEvent.click(screen.getByRole("button", { name: "Hide empty fields" }));
    expect(screen.queryByText("VAT ID")).toBeNull();
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

describe("Record details — a record the reader may not change", () => {
  const archived = "This company is archived and takes no changes.";
  const lockedRows = (reason: string) => [
    {
      key: "about",
      label: "About",
      rows: [
        {
          key: "domain",
          term: "Domain",
          value: (
            <LockedValue label="Domain" reason={reason}>
              brandt.example
            </LockedValue>
          ),
        },
      ],
    },
  ];

  it("says why once, above the fields, and locks no row a second time", () => {
    render(
      <RecordDetails
        sections={lockedRows(archived)}
        refusal={{ text: archived }}
      />,
    );
    expect(screen.getByRole("note")).toHaveTextContent(archived);
    expect(screen.queryByRole("button", { name: /locked/ })).toBeNull();
  });

  it("locks the one field whose reason is its own", () => {
    render(<RecordDetails sections={lockedRows("Only an admin sets this.")} />);
    expect(
      screen.getByRole("button", { name: "Why Domain is locked" }),
    ).toBeInTheDocument();
  });

  it("draws no banner where the page's own band already says why", () => {
    render(
      <RecordDetails sections={lockedRows(archived)} lockedBy={archived} />,
    );
    expect(screen.queryByRole("note")).toBeNull();
    expect(screen.queryByRole("button", { name: /locked/ })).toBeNull();
  });
});
