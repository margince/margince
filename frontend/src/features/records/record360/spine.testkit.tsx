import { render } from "@testing-library/react";
import type { components } from "../../../api/schema";
import { LocaleProvider } from "../../../i18n";
import { company360 } from "../../companies/company.fixtures";
import { RecordSpine } from "./spine";

// The thread's one rule: it may only draw what the payload supports. Every
// stop is a record or a date. The one stop with neither behind it (the
// silence) is arithmetic between two dates the payload does carry.

export type View = components["schemas"]["Company360"];

export const page = { has_more: false, next_cursor: null };

export const AS_OF = "2026-08-25T09:00:00Z";

export const SPOKE = "2026-08-18T09:00:00Z";

export function view(overrides: Record<string, unknown> = {}): View {
  return {
    // The assembled-and-empty backstop, so the account this thread hangs off
    // is the shape the wire sends rather than the five fields this file needs.
    ...company360,
    as_of: AS_OF,
    company: { ...company360.company, display_name: "Kugellager" },
    ...overrides,
  };
}

export function draw(
  v: View,
  nameOf?: (entityType: string, entityId: string) => string | undefined,
  onOpenEmail?: (activityId: string) => void,
) {
  render(
    <LocaleProvider initial="en">
      <RecordSpine
        source={v}
        commercial={v?.state_strip?.commercial}
        nameOf={nameOf}
        onOpenEmail={onOpenEmail}
      />
    </LocaleProvider>,
  );
}

// The 360 serves activities newest-first (`ORDER BY occurred_at DESC`). The
// head of the list is the most recent, and the thread makes no ordering
// decision of its own.
export function activities(
  rows: readonly {
    id: string;
    kind: string;
    subject: string;
    at: string;
    thread?: string;
    links?: readonly { entity_type: string; entity_id: string }[];
    // The colleague who held it. On every row, meeting or not, because the
    // column is on every row. A row without it proves only that it is absent
    // there, not that the reading is gated on kind.
    host_user_id?: string;
    direction?: "inbound" | "outbound";
    // What the server sends for a `kind=email` row and for no other kind. A
    // mail fixture without it models a shape the API does not return, so a
    // stop judged against it proves nothing about a real message.
    emailStatus?: "team" | "participants" | "selected" | "withheld";
  }[],
) {
  return {
    data: rows.map((row) => ({
      id: row.id,
      kind: row.kind,
      is_done: false,
      subject: row.subject,
      occurred_at: row.at,
      thread_key: row.thread ?? null,
      links: row.links ?? [],
      host_user_id: row.host_user_id ?? null,
      direction: row.direction ?? null,
      email_summary: row.emailStatus
        ? {
            activity_id: row.id,
            occurred_at: row.at,
            version: 1,
            subject: row.subject,
            display_status: row.emailStatus,
            move: "none",
            attachment_count: 0,
          }
        : null,
      source: "manual",
      captured_by: "human:test",
      created_at: row.at,
      updated_at: row.at,
    })),
    page,
  };
}
