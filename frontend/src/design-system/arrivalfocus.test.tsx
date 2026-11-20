/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { announceAddressChanged, navigate } from "../lib/router";
import {
  awaitArrival,
  useArrivalFocus,
  useAwaitedArrivalFocus,
} from "./arrivalfocus";
import { Heading } from "./heading";

function Page({ arrived = true }: Readonly<{ arrived?: boolean }>) {
  const title = useArrivalFocus<HTMLHeadingElement>(arrived);
  return (
    <Heading size="xlarge" ref={title} tabIndex={-1}>
      Arrived
    </Heading>
  );
}

function SharedPage() {
  const title = useAwaitedArrivalFocus<HTMLHeadingElement>();
  return (
    <Heading size="xlarge" ref={title} tabIndex={-1}>
      Record
    </Heading>
  );
}

// Leaving the page is what forgets a press, so each case starts on none.
afterEach(() => {
  cleanup();
  window.location.hash = "";
  announceAddressChanged();
});

// Each move is heard before the next, as a reader's clicks are.
function go(route: Parameters<typeof navigate>[0]) {
  act(() => {
    navigate(route);
    announceAddressChanged();
  });
}

describe("useArrivalFocus", () => {
  it("gives the title the focus the page change dropped on the body", () => {
    render(<Page />);
    expect(screen.getByRole("heading", { name: "Arrived" })).toHaveFocus();
  });

  it("leaves the body focused for a mount no press caused", () => {
    render(<Page arrived={false} />);
    expect(screen.getByRole("heading", { name: "Arrived" })).not.toHaveFocus();
  });

  it("leaves focus where the reader put it", () => {
    const field = document.createElement("input");
    document.body.append(field);
    field.focus();

    render(<Page />);

    expect(field).toHaveFocus();
    field.remove();
  });
});

describe("useAwaitedArrivalFocus", () => {
  const title = () => screen.getByRole("heading", { name: "Record" });

  it("gives the title focus on the page a press said it opens", () => {
    awaitArrival({ screen: "contacts", id: "c1" });
    window.location.hash = "#/contacts/c1/activity";
    render(<SharedPage />);
    expect(title()).toHaveFocus();
  });

  it("leaves focus alone on a page no press named", () => {
    awaitArrival({ screen: "contacts", id: "c1" });
    window.location.hash = "#/contacts/c2";
    render(<SharedPage />);
    expect(title()).not.toHaveFocus();
  });

  it("forgets the press once the reader leaves that page", () => {
    window.location.hash = "#/contacts";
    awaitArrival({ screen: "contacts", id: "c1" });
    go({ screen: "contacts", id: "c1" });
    go({ screen: "contacts" });
    go({ screen: "contacts", id: "c1" });
    render(<SharedPage />);
    expect(title()).not.toHaveFocus();
  });
});
