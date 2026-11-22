/** @vitest-environment happy-dom */
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { runCourseKey } from "../lib/capture-progress";
import { RunCourseNote } from "./runcoursenote";

// A live import's count can fall for two reasons the server tells apart. The
// provider refused the saved place and the scan began again, so counts restart.
// Or a page failed and is read again, so its emails are counted again.

type BackfillStatus = components["schemas"]["BackfillStatus"];

const run = (over: Partial<BackfillStatus>): BackfillStatus => ({
  state: "running",
  window: "6m",
  estimated_messages: 5000,
  counts: { messages_scanned: 1240, captured: 900, contacts_created: 30 },
  ...over,
});

afterEach(cleanup);

describe("runCourseKey", () => {
  it("names a restart before a retry, since a restart also counts as a failed page", () => {
    expect(runCourseKey(run({ window_restarts: 1, retry_attempts: 1 }))).toBe(
      "backfill.restartedNote",
    );
  });

  it("names a retried page", () => {
    expect(runCourseKey(run({ retry_attempts: 2 }))).toBe("backfill.retryNote");
  });

  it("says nothing about a run that never stumbled, or one that has ended", () => {
    expect(runCourseKey(run({ retry_attempts: 0, window_restarts: 0 }))).toBe(
      null,
    );
    expect(runCourseKey(run({}))).toBe(null);
    expect(runCourseKey(run({ state: "done", window_restarts: 1 }))).toBe(null);
  });
});

describe("RunCourseNote", () => {
  const view = (status: BackfillStatus) =>
    rtlRender(
      <LocaleProvider initial="en">
        <RunCourseNote run={status} />
      </LocaleProvider>,
    );

  it("renders nothing for a clean run", () => {
    const { container } = view(run({}));
    expect(container.textContent).toBe("");
  });

  it("says a restarted scan began again and kept its contacts", () => {
    view(run({ window_restarts: 1 }));
    expect(
      screen.getByText(/started over because the mailbox lost/),
    ).toBeTruthy();
  });

  it("says a retried page is read and counted again, never that the count stays", () => {
    view(run({ retry_attempts: 1 }));
    expect(
      screen.getByText(
        /last page is read again, so its emails are counted again/,
      ),
    ).toBeTruthy();
    expect(screen.queryByText(/stay/)).toBeNull();
  });
});
