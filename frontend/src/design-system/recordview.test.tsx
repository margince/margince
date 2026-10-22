// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { UTC_ZONE } from "../format/timezone";
import { RecordView } from "./recordview";

afterEach(cleanup);

const record = {
  name: "Brandt Automotive GmbH",
  identity: "o-1",
  zone: UTC_ZONE,
  pulse: <span>Customer</span>,
  aside: <p>Details</p>,
  tabs: <span>Tabs</span>,
  children: <p>Feed</p>,
} as const;

describe("the split record", () => {
  it("reads the name, then the details, then the tabs and the feed", () => {
    render(<RecordView {...record} />);
    const [name, details, tabs, feed] = [
      "Brandt Automotive GmbH",
      "Details",
      "Tabs",
      "Feed",
    ].map((text) => screen.getByText(text));
    const follows = (first?: Node, second?: Node) =>
      first && second
        ? Boolean(
            first.compareDocumentPosition(second) &
              Node.DOCUMENT_POSITION_FOLLOWING,
          )
        : false;
    expect(follows(name, details)).toBe(true);
    expect(follows(details, tabs)).toBe(true);
    expect(follows(tabs, feed)).toBe(true);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "Brandt Automotive GmbH",
    );
  });

  it("keeps the head when the record has no details", () => {
    render(<RecordView {...record} aside={undefined} />);
    expect(screen.queryByText("Details")).toBeNull();
    expect(screen.getByText("Customer")).toBeInTheDocument();
  });
});

describe("the record's feed", () => {
  const follows = (first: Element | null, second: Element | null) =>
    Boolean(
      first &&
        second &&
        first.compareDocumentPosition(second) &
          Node.DOCUMENT_POSITION_FOLLOWING,
    );

  it("draws the tab strip above the band, and the band above the tab's body", () => {
    render(
      <RecordView
        identity="r-1"
        name="Fleet retrofit"
        zone={UTC_ZONE}
        tabs={<span>Overview</span>}
        band={<p>Kick-off pending</p>}
      >
        <p>Body</p>
      </RecordView>,
    );
    const tabs = screen.getByTestId("record-tabs");
    const band = screen.getByText("Kick-off pending");
    const body = screen.getByText("Body");
    expect(follows(tabs, band), "the band is drawn above the strip").toBe(true);
    expect(follows(band, body), "the band is drawn below the body").toBe(true);
  });

  it("draws the tab strip on a record that carries no band", () => {
    const { container } = render(
      <RecordView
        identity="r-1"
        name="Fleet retrofit"
        zone={UTC_ZONE}
        tabs={<span>Overview</span>}
      >
        <p>Body</p>
      </RecordView>,
    );
    expect(container.querySelector(".record-band")).toBeNull();
    expect(
      follows(screen.getByTestId("record-tabs"), screen.getByText("Body")),
    ).toBe(true);
  });
});
