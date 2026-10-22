// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { OAuthOutcomeNote } from "./connectors.notices";

// The note reads a landing the address bar carried, so both segments are
// reader-typed. It reads them against the roster, because a chained consent
// lands on the calendar's outcome after the mail grant already held.

type CaptureConnection = components["schemas"]["CaptureConnection"];

function connection(
  provider: CaptureConnection["provider"],
  backfill: CaptureConnection["backfill"] = { state: "done" },
): CaptureConnection {
  return {
    id: `connection-${provider}`,
    provider,
    can_backfill: true,
    status: "connected",
    scopes: [],
    backfill,
  };
}

function mount(
  outcome: string | undefined,
  provider: string | undefined,
  connections: CaptureConnection[] = [],
  connectCalendar?: Parameters<typeof OAuthOutcomeNote>[0]["connectCalendar"],
) {
  return render(
    <LocaleProvider initial="en">
      <OAuthOutcomeNote
        outcome={outcome}
        provider={provider}
        connections={connections}
        connectCalendar={connectCalendar}
      />
    </LocaleProvider>,
  );
}

afterEach(cleanup);

describe("the OAuth return note", () => {
  it("reports an outcome the server defines, even with no provider segment", () => {
    mount("ok", undefined);
    expect(screen.getByText("Connected")).toBeTruthy();
  });

  // On a plain object literal `constructor` and its siblings are inherited
  // members. A bare index would answer with a function and draw a note that
  // says nothing.
  it.each([
    "constructor",
    "toString",
    "__proto__",
    "valueOf",
    "hasOwnProperty",
  ])("draws nothing for the inherited member %s", (segment) => {
    const { container } = mount(segment, "gmail");
    expect(container.firstChild).toBeNull();
  });

  it("reads an inherited member as the provider like any unknown one", () => {
    mount("ok", "constructor");
    expect(screen.getByText("Connected")).toBeTruthy();
  });

  it("waits for the roster before reading a landing", () => {
    const { container } = render(
      <LocaleProvider initial="en">
        <OAuthOutcomeNote
          outcome="denied"
          provider="gcal"
          connections={undefined}
        />
      </LocaleProvider>,
    );
    expect(container.firstChild).toBeNull();
  });

  it("draws nothing for a segment this build simply does not know", () => {
    const { container } = mount("unrecognised", "gmail");
    expect(container.firstChild).toBeNull();
  });

  it("names the vendor and both services once a chain lands on its calendar", () => {
    mount("ok", "gcal", [connection("gmail"), connection("gcal")]);
    expect(screen.getByText("Google connected")).toBeTruthy();
    expect(screen.getByText("Mail and Calendar are syncing.")).toBeTruthy();
  });

  it("says the import keeps going while the mailbox's import is running", () => {
    mount("ok", "graph", [connection("graph", { state: "running" })]);
    expect(screen.getByText("Microsoft 365 connected")).toBeTruthy();
    expect(
      screen.getByText(
        "Mail is syncing. You can leave this page: the import of past email keeps running on its own.",
      ),
    ).toBeTruthy();
  });

  it("never claims a mailbox when only the calendar was connected", () => {
    mount("ok", "gcal", []);
    expect(screen.getByText("Calendar is syncing.")).toBeTruthy();
    expect(screen.queryByText(/Mail/)).toBeNull();
  });

  it("reports a declined calendar after its mailbox as a partial success with one way on", async () => {
    const onConnect = vi.fn();
    mount("denied", "gcal", [connection("gmail")], {
      onConnect,
      pending: false,
      error: null,
    });
    expect(
      screen.getByText(
        "Mail is connected. Calendar was not, because access to it was not allowed.",
      ),
    ).toBeTruthy();
    expect(screen.queryByText("Nothing was connected")).toBeNull();
    await userEvent.click(
      screen.getByRole("button", { name: "Connect calendar" }),
    );
    expect(onConnect).toHaveBeenCalledWith("gcal");
  });

  it("keeps the failure's own remedy when the calendar leg broke another way", () => {
    mount("bad_client", "graphcal", [connection("graph")]);
    expect(
      screen.getByText("Mail is connected. Calendar was not."),
    ).toBeTruthy();
    expect(
      screen.getByText(/refused this installation’s app credentials/),
    ).toBeTruthy();
  });

  it("shows why the calendar connect it offered failed", () => {
    mount("denied", "gcal", [connection("gmail")], {
      onConnect: vi.fn(),
      pending: false,
      error: "The calendar app is not configured.",
    });
    expect(
      screen.getByText("The calendar app is not configured."),
    ).toBeTruthy();
  });

  it("says nothing was connected when the calendar was declined with no mailbox", () => {
    mount("denied", "gcal", []);
    expect(screen.getByText("Nothing was connected")).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: "Connect calendar" }),
    ).toBeNull();
  });

  it("puts the note away until the landing changes", async () => {
    const { container } = mount("ok", "gmail", [connection("gmail")]);
    await userEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(container.firstChild).toBeNull();
  });
});
