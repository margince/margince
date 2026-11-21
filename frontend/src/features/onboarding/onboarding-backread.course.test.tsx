/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { BackreadRun } from "./onboarding-backread.run";

type BackfillStatus = components["schemas"]["BackfillStatus"];

const run = (over: Partial<BackfillStatus>): BackfillStatus => ({
  state: "running",
  window: "6m",
  estimated_messages: 5000,
  counts: { messages_scanned: 1240, captured: 900, contacts_created: 30 },
  ...over,
});

const props = {
  cancelling: false,
  cancelProblem: null,
  onCancel: () => undefined,
  onRestart: () => undefined,
};

afterEach(cleanup);

describe("the onboarding import step", () => {
  it("explains a restart and a retry the same way as the settings card", () => {
    const { rerender } = render(
      <LocaleProvider initial="en">
        <BackreadRun {...props} run={run({ window_restarts: 1 })} />
      </LocaleProvider>,
    );
    expect(
      screen.getByText(/started over because the mailbox lost/),
    ).toBeTruthy();
    rerender(
      <LocaleProvider initial="en">
        <BackreadRun {...props} run={run({ retry_attempts: 1 })} />
      </LocaleProvider>,
    );
    expect(screen.getByText(/so its emails are counted again/)).toBeTruthy();
  });
});
