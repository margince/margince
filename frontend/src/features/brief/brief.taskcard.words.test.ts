// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { translate, translatePlural, type Words } from "../../i18n";
import type { WorklistItem } from "../../lib/worklisttypes";
import { taskRow, waitingEmailRow } from "../../testing/brief.fixtures";
import { cardWhen, taskName } from "./brief.taskcard";

const english: Words = {
  t: (key, params) => translate("en", key, params),
  plural: (base, count, params) => translatePlural("en", base, count, params),
  locale: "en",
};

it("dates a late task by its due day and the days it is late", () => {
  const late = {
    ...taskRow("task-1", "Send the sheet"),
    due_at: "2026-09-10T08:30:00Z",
    overdue: true,
  };
  const now = new Date("2026-09-13T09:00:00Z");
  expect(cardWhen(late, english, "UTC", "UTC", now)).toMatch(/ · 3 days late$/);
  expect(
    cardWhen(taskRow("task-2", "Call"), english, "UTC", "UTC", now),
  ).toBeNull();
});

it("names a reply after the sender, not another contact on the message", () => {
  const waiting = waitingEmailRow();
  const mail = waiting.email_summary;
  if (!mail) throw new Error("the waiting row carries its message");
  const sent: WorklistItem = {
    ...waiting,
    email_summary: { ...mail, counterparty_contact_id: "contact-sonya" },
  };
  expect(taskName(sent, undefined, english.t, "en")).toBe(
    "Reply to Sonya Beck: Meet next Tues?",
  );
  const filedUnderColleague: WorklistItem = {
    ...waiting,
    contact: { id: "contact-ida", label: "Ida Roth" },
    email_summary: { ...mail, counterparty: "Lena Wolf" },
  };
  expect(taskName(filedUnderColleague, undefined, english.t, "en")).toBe(
    "Reply to Lena Wolf: Meet next Tues?",
  );
});

it("names an approval card by its sentence, as the opened row does", () => {
  const approval: WorklistItem = {
    ...taskRow("approval", "Update record deal: name=Acme, Inc."),
    source: "approval",
    summary_parts: {
      sentence: "Update record deal",
      fields: [],
      more_fields: 0,
    },
  };
  expect(taskName(approval, undefined, english.t, "en")).toBe(
    "Update record deal",
  );
});
