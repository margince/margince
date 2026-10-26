/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { auditEntry, render } from "../../testing/settings.testkit";
import { AuditDetail } from "./settings-audit.detail";
import type { AuditLogEntry } from "./settings-audit.format";

afterEach(cleanup);

// A day entry's time is stored at noon UTC of its date; the detail names the
// date alone, as the record's History tab does.
const MOVED_DAY: AuditLogEntry = {
  id: auditEntry.id,
  actor_type: "agent",
  actor_id: auditEntry.actor_id,
  action: "update",
  entity_type: "activity",
  entity_id: auditEntry.entity_id,
  occurred_at: "2026-10-22T09:00:00Z",
  before: { occurred_at: "2026-10-22T12:00:00Z", occurred_precision: "day" },
  after: { occurred_at: "2026-10-20T12:00:00Z", occurred_precision: "day" },
};

describe("AuditDetail", () => {
  it("names a day entry's moved date without a clock", () => {
    render(<AuditDetail entry={MOVED_DAY} />);

    expect(screen.getByText("22/10/2026")).toHaveClass("field-diff-from");
    expect(screen.getByText("20/10/2026")).toHaveClass("field-diff-to");
    expect(screen.queryByText(/T12:00:00/)).toBeNull();
  });

  it("names a created day entry's date alone", () => {
    render(
      <AuditDetail entry={{ ...MOVED_DAY, action: "create", before: null }} />,
    );

    expect(screen.getByText("20/10/2026")).toBeInTheDocument();
  });
});
