/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { OpenSourceProvider } from "../lib/opensource";
import { NoticeRow } from "./notificationrow";

afterEach(cleanup);
it("opens the exact note flagged for manual review without settling the notice", async () => {
  const open = vi.fn();
  const settle = vi.fn();
  const activate = vi.fn();
  const user = userEvent.setup();
  render(
    <LocaleProvider initial="en">
      <OpenSourceProvider openSource={open}>
        <ul>
          <NoticeRow
            notice={{
              id: "review",
              kind: "privacy_review_required",
              subject: "Manual privacy review needed",
              created_at: "2026-10-01T12:00:00Z",
              target: { type: "activity", id: "matching-note" },
            }}
            onSettle={settle}
            onActivate={activate}
            settling={false}
          />
        </ul>
      </OpenSourceProvider>
    </LocaleProvider>,
  );
  await user.click(screen.getByRole("button", { name: "Open original" }));
  expect(open).toHaveBeenCalledWith("matching-note");
  expect(activate).toHaveBeenCalledOnce();
  expect(settle).not.toHaveBeenCalled();
});
