// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { viewerZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { RecordRefusal, RecordView } from "./recordview";

afterEach(cleanup);

function renderRecord(aside: boolean) {
  return render(
    <LocaleProvider initial="en">
      <RecordView
        identity="r-1"
        name="Brandt Automotive GmbH"
        zone={viewerZone()}
        actionsInline
        actions={
          <>
            <RecordRefusal id="log-refused">
              You do not have permission to log activities on this record.
            </RecordRefusal>
            <button type="button" disabled aria-describedby="log-refused">
              Log activity
            </button>
          </>
        }
        aside={aside ? <p>Fields</p> : undefined}
        asideOpen
      />
    </LocaleProvider>,
  );
}

// A refused verb's reason is said once, in the access note at the foot of the
// fields column, and the verb still names it as its description from the head.
describe("a record's refusals", () => {
  it("stand in the fields column's access note", () => {
    const { container } = renderRecord(true);
    const reason = screen.getByText(/permission to log activities/);
    expect(reason.closest(".record-access-lines")).toBeTruthy();
    expect(container.querySelector(".record-actions .record-refusal")).toBeNull();
    expect(
      screen.getByRole("button", { name: "Log activity" }).getAttribute(
        "aria-describedby",
      ),
    ).toBe(reason.id);
  });

  it("stand in the head's verb row when the record has no fields column", () => {
    const { container } = renderRecord(false);
    const reason = screen.getByText(/permission to log activities/);
    expect(reason.closest(".record-actions .record-refusal")).toBeTruthy();
    expect(container.querySelector(".record-access")).toBeNull();
  });
});
