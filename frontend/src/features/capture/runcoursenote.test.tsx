/** @vitest-environment happy-dom */
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { BackreadRun } from "../onboarding/onboarding-backread.run";
import { RunView } from "./backfillrunview";
import { RunCourseNote, runCourseKey } from "./runcoursenote";

// A live import's count can fall for two reasons the server tells apart. The
// provider refused the saved place and the scan began again, so counts restart.
// Or a page failed and is read again, so committed counts stay.

type BackfillStatus = components["schemas"]["BackfillStatus"];

const run = (over: Partial<BackfillStatus>): BackfillStatus => ({
  state: "running",
  window: "6m",
  estimated_messages: 5000,
  counts: { messages_scanned: 1240, captured: 900, contacts_created: 30 },
  ...over,
});

const RESTARTED = /started over because the mailbox lost its place/;
const RETRYING = /connection problem and is trying again/;

function render(ui: ReactNode) {
  return rtlRender(<LocaleProvider initial="en">{ui}</LocaleProvider>);
}

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

describe("the settings import card", () => {
  const view = (status: BackfillStatus) =>
    render(
      <RunView
        run={status}
        cancelling={false}
        cancelError={null}
        onCancel={() => undefined}
        onRestart={() => undefined}
      />,
    );

  it("explains a scan that restarted at zero", () => {
    view(
      run({
        window_restarts: 1,
        retry_attempts: 1,
        counts: { messages_scanned: 0, captured: 0, contacts_created: 30 },
      }),
    );
    expect(screen.getByText(RESTARTED)).toBeTruthy();
  });

  it("explains a retried page and keeps the committed count on screen", () => {
    view(run({ retry_attempts: 1 }));
    expect(screen.getByText(RETRYING)).toBeTruthy();
    expect(screen.getByText(/1,240 emails checked/)).toBeTruthy();
  });
});

describe("the onboarding import step", () => {
  it("explains a restart and a retry the same way", () => {
    const props = {
      cancelling: false,
      cancelProblem: null,
      onCancel: () => undefined,
      onRestart: () => undefined,
    };
    const { rerender } = render(
      <BackreadRun {...props} run={run({ window_restarts: 1 })} />,
    );
    expect(screen.getByText(RESTARTED)).toBeTruthy();
    rerender(
      <LocaleProvider initial="en">
        <BackreadRun {...props} run={run({ retry_attempts: 1 })} />
      </LocaleProvider>,
    );
    expect(screen.getByText(RETRYING)).toBeTruthy();
  });
});

describe("RunCourseNote", () => {
  it("renders nothing for a clean run", () => {
    const { container } = render(<RunCourseNote run={run({})} />);
    expect(container.textContent).toBe("");
  });
});
