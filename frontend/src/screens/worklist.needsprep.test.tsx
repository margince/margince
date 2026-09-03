// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { meetingRow } from "./home.fixtures";
import { render } from "./home.testkit";
import { WorklistRow } from "./worklist.row";

// A meeting nothing is prepared for, on the row.
//
// The server decides readiness — `meeting_unprepared` reaches the client as a
// reason like any other. What this file is about is that the row draws it as a
// STATE rather than as one weighed fact, and says it exactly once: a rep
// scanning for the meeting to open before it starts must see it without reading
// the line under the title, and a finding reported twice in two registers reads
// as two findings about one meeting.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a meeting with nothing prepared", () => {
  it("says so as a badge", () => {
    render(
      <WorklistRow item={meetingRow("m1", false)} position={1} owner="" />,
    );

    expect(screen.getByText(en["worklist.needsPrep"])).toBeTruthy();
  });

  // The same fact in the phrase line as well would be the row making one
  // finding look like two.
  it("does not also say it in the line of reasons", () => {
    render(
      <WorklistRow item={meetingRow("m1", false)} position={1} owner="" />,
    );

    expect(
      screen.queryByText(en["worklist.because.meeting_unprepared"]),
    ).toBeNull();
  });

  it("says nothing about prep on a meeting that has some", () => {
    render(<WorklistRow item={meetingRow("m1", true)} position={1} owner="" />);

    expect(screen.queryByText(en["worklist.needsPrep"])).toBeNull();
  });

  // Dropping the badged reason must not take the row's OTHER reasons with it.
  it("keeps the reasons that are not badges", () => {
    const item = meetingRow("m1", false);
    render(
      <WorklistRow
        item={{
          ...item,
          because: [...item.because, { kind: "meeting_soon" }],
        }}
        position={1}
        owner=""
      />,
    );

    expect(screen.getByText(en["worklist.needsPrep"])).toBeTruthy();
    expect(
      screen.getByText(new RegExp(en["worklist.because.meeting_soon"])),
    ).toBeTruthy();
  });
});
