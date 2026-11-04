// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { toggleOptions } from "../../design-system/select-testing";
import { ToastProvider } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { FieldBuilder } from "./customfields.builder";
import { activityKindsOf } from "./customfields.kinds";
import { FieldTable } from "./customfields.table";

type CustomField = components["schemas"]["CustomField"];

afterEach(cleanup);

const noop = () => {};

it("narrows a new activity field to the types the admin picks", async () => {
  const user = userEvent.setup();
  const onSubmit = vi.fn();
  render(
    <LocaleProvider initial="en">
      <ToastProvider>
        <FieldBuilder
          object="activity"
          pending={false}
          onSubmit={onSubmit}
          onCancel={noop}
        />
      </ToastProvider>
    </LocaleProvider>,
  );
  await user.type(screen.getByLabelText(/Label/i), "Visit report");
  await toggleOptions(user, screen.getByLabelText(en["cf.activityKinds"]), [
    "Meeting",
  ]);
  await user.click(screen.getByRole("button", { name: "Add field" }));
  expect(onSubmit).toHaveBeenCalledWith(
    expect.objectContaining({ object: "activity", activityKinds: ["meeting"] }),
  );
});

it("asks no types of a field on another object", () => {
  render(
    <LocaleProvider initial="en">
      <ToastProvider>
        <FieldBuilder
          object="deal"
          pending={false}
          onSubmit={noop}
          onCancel={noop}
        />
      </ToastProvider>
    </LocaleProvider>,
  );
  expect(screen.queryByLabelText(en["cf.activityKinds"])).toBeNull();
});

it("writes the types a listed activity field shows on", async () => {
  const user = userEvent.setup();
  const onKinds = vi.fn();
  const visit: CustomField = {
    id: "visit",
    object: "activity",
    label: "Visit report",
    slug: "visit_report",
    type: "text",
    status: "active",
    column_name: "cf_visit_report",
    show_in_create: true,
    reportable: false,
    activity_kinds: ["meeting"],
    created_by: "u1",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
  render(
    <LocaleProvider initial="en">
      <FieldTable
        object="activity"
        fields={[visit]}
        canEdit
        meUserId="u1"
        onRename={noop}
        onArchive={noop}
        onSetting={noop}
        onKinds={onKinds}
      />
    </LocaleProvider>,
  );
  await toggleOptions(
    user,
    screen.getByLabelText("Types “Visit report” shows on"),
    ["Call"],
  );
  expect(onKinds).toHaveBeenCalledWith(visit, ["meeting", "call"]);
});

it("sends the kinds in the server's order and drops a word it does not take", () => {
  expect(activityKindsOf(["meeting", "visit", "call"])).toEqual([
    "call",
    "meeting",
  ]);
});
