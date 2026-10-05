// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A meeting row says who the meeting was with, which account they work for,
// and who hosted it, and draws no host name the server withheld.

/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { translate } from "../i18n";
import { hostText } from "./worklist.meetingparties";
import { day, renderWorklist, row, stub } from "./worklist.testkit";

const CUSTOMER = "01a05500-0000-7000-8000-0000000000c1";
const ACME = "01a05500-0000-7000-8000-0000000000c2";
const LENA = "01a05500-0000-7000-8000-0000000000c3";
const READER = "01a05500-0000-7000-8000-0000000000c4";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function aMeetingOwedAnAnswer(over = {}) {
  return row({
    id: "01a05500-0000-7000-8000-0000000000b1",
    source: "meeting_outcome",
    category: "meetings",
    title: "Weekly sync",
    destination: "today",
    actions: ["decide"],
    subject: { type: "activity", id: "01a05500-0000-7000-8000-0000000000b1" },
    contact: {
      id: CUSTOMER,
      label: "Sonya Beck",
      employer: { company_id: ACME, company_name: "Acme GmbH" },
    },
    host: { kind: "user", id: LENA, label: "Lena Fischer" },
    ...over,
  });
}

const t = (
  key: "worklist.meeting.hostedBy" | "worklist.meeting.hostedByYou",
  values?: { name: string },
) => translate("en", key, values);

describe("a meeting row names its parties", () => {
  it("links the customer and their account, and names the host", async () => {
    stub(day({ queue: [aMeetingOwedAnAnswer()] }));
    renderWorklist();
    const customer = await screen.findByRole("link", { name: "Sonya Beck" });
    expect(customer.getAttribute("href")).toContain(CUSTOMER);
    const account = screen.getByRole("link", { name: "Acme GmbH" });
    expect(account.getAttribute("href")).toContain(ACME);
    expect(screen.getByText("hosted by Lena Fischer")).toBeTruthy();
  });

  it("draws no host the server named without a label", async () => {
    stub(
      day({
        queue: [aMeetingOwedAnAnswer({ host: { kind: "user", id: LENA } })],
      }),
    );
    renderWorklist();
    await screen.findByRole("link", { name: "Sonya Beck" });
    expect(screen.queryByText(/hosted by/)).toBeNull();
  });

  it("draws no account for a contact the server sent none for", async () => {
    stub(
      day({
        queue: [
          aMeetingOwedAnAnswer({
            contact: { id: CUSTOMER, label: "Sonya Beck" },
          }),
        ],
      }),
    );
    renderWorklist();
    await screen.findByRole("link", { name: "Sonya Beck" });
    expect(screen.queryByRole("link", { name: "Acme GmbH" })).toBeNull();
  });
});

describe("hostText", () => {
  it("says the reader hosted their own meeting, without a name", () => {
    const item = aMeetingOwedAnAnswer({ host: { kind: "user", id: READER } });
    expect(hostText(item, READER, t)).toBe("hosted by you");
  });

  it("names a colleague by the label the server resolved", () => {
    expect(hostText(aMeetingOwedAnAnswer(), READER, t)).toBe(
      "hosted by Lena Fischer",
    );
  });

  it("says nothing for a row with no host", () => {
    const item = aMeetingOwedAnAnswer({ host: undefined });
    expect(hostText(item, READER, t)).toBeNull();
  });
});
