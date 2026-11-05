/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { NO_TIMELINE_FILTERS } from "../../design-system/recordtimeline";
import { TimelineFilterBar } from "../../design-system/timelinefilterbar";
import { LocaleProvider } from "../../i18n";
import { StagedEditor } from "./approvaleditor";
import type { EditableField } from "./approvalkind";

afterEach(cleanup);

// A record page draws a staged edit and the timeline's own date filters side by side.
function RecordPage({
  fields,
  onApprove,
}: Readonly<{ fields: readonly EditableField[]; onApprove: () => void }>) {
  return (
    <LocaleProvider initial="en">
      <main>
        <StagedEditor
          fields={fields}
          draft={{}}
          onChange={() => {}}
          pending={false}
          onApprove={onApprove}
          onCancel={() => {}}
        />
        <TimelineFilterBar value={NO_TIMELINE_FILTERS} onChange={() => {}} />
      </main>
    </LocaleProvider>
  );
}

it("approves past a day the timeline filter cannot read", async () => {
  const user = userEvent.setup();
  const onApprove = vi.fn();
  render(
    <RecordPage
      fields={[{ field: "title", as: "text" }]}
      onApprove={onApprove}
    />,
  );
  await user.type(screen.getByRole("textbox", { name: "From" }), "99/99");
  await user.tab();

  await user.click(screen.getByRole("button", { name: "Approve edited" }));

  expect(onApprove).toHaveBeenCalledOnce();
});

it("holds the approve while its own date field holds text it cannot read", async () => {
  const user = userEvent.setup();
  const onApprove = vi.fn();
  render(
    <RecordPage
      fields={[{ field: "due_date", as: "date" }]}
      onApprove={onApprove}
    />,
  );
  const due = screen.getByRole("textbox", { name: "due_date" });
  await user.type(due, "99/99");

  await user.click(screen.getByRole("button", { name: "Approve edited" }));

  expect(onApprove).not.toHaveBeenCalled();
  expect(due).toHaveFocus();
});
