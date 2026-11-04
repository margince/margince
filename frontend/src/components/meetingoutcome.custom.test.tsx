// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import type { components } from "../api/schema";
import { ToastProvider } from "../design-system/toast";
import { renderWithQueryClient } from "../testing/queryrender";
import { installFetchStub, jsonResponse } from "../testing/story-utils";
import { MeetingOutcome } from "./meetingoutcome";

type CustomField = components["schemas"]["CustomField"];

const MEETING_ID = "01a05500-0000-7000-8000-0000000000c2";

function activityField(over: Partial<CustomField>): CustomField {
  return {
    id: "cf",
    object: "activity",
    label: "Field",
    slug: "field",
    type: "text",
    status: "active",
    column_name: "cf_field",
    show_in_create: true,
    reportable: false,
    created_by: "human:u-1",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  };
}

const FIELDS = [
  activityField({
    id: "visit",
    label: "Visit report",
    column_name: "cf_visit_report",
    activity_kinds: ["meeting"],
  }),
  activityField({ id: "region", label: "Region", column_name: "cf_region" }),
  // Narrowed to calls after this meeting was given a value: shown to clear.
  activityField({
    id: "dialled",
    label: "Number dialled",
    column_name: "cf_dialled",
    activity_kinds: ["call"],
  }),
  activityField({
    id: "script",
    label: "Call script",
    column_name: "cf_script",
    activity_kinds: ["call"],
  }),
];

afterEach(cleanup);

it("prefills the meeting's fields and sends only the one the reader edited", async () => {
  const patched: Record<string, unknown>[] = [];
  installFetchStub({
    "GET /custom-fields": () =>
      jsonResponse({ data: FIELDS, page: { has_more: false } }),
    [`GET /activities/${MEETING_ID}`]: () =>
      jsonResponse({
        id: MEETING_ID,
        kind: "meeting",
        subject: "Site visit",
        body: "Agenda",
        occurred_at: "2026-07-06T09:00:00Z",
        occurred_precision: "instant",
        meeting_status: "booked",
        version: 7,
        writable: true,
        offers_email: false,
        source: "manual",
        captured_by: "human:u1",
        cf_visit_report: "Saw the new line",
        cf_region: "North",
        cf_dialled: "+49 30 1234",
      }),
    [`PATCH /activities/${MEETING_ID}`]: (body) => {
      patched.push({ ...Object(body) });
      return jsonResponse({ id: MEETING_ID, version: 8 });
    },
  });
  const user = userEvent.setup();
  renderWithQueryClient(
    <ToastProvider>
      <MeetingOutcome
        id={MEETING_ID}
        version={7}
        title="Site visit"
        invalidateKeys={[]}
        cancellable
      />
    </ToastProvider>,
  );
  await user.click(screen.getByRole("button", { name: /Add notes/ }));
  const report = await screen.findByRole("textbox", { name: "Visit report" });
  expect(report).toHaveProperty("value", "Saw the new line");
  expect(
    screen.getByRole("textbox", { name: "Number dialled" }),
  ).toHaveProperty("value", "+49 30 1234");
  expect(screen.queryByRole("textbox", { name: "Call script" })).toBeNull();

  await user.clear(report);
  await user.type(report, "Line two is running");
  await user.click(screen.getByRole("button", { name: "Log" }));

  await waitFor(() => expect(patched).toHaveLength(1));
  expect(patched[0]).toMatchObject({ cf_visit_report: "Line two is running" });
  expect(
    Object.keys(patched[0] ?? {}).filter((key) => key.startsWith("cf_")),
  ).toEqual(["cf_visit_report"]);
});
