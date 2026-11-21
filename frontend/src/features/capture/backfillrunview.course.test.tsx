/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { RunView } from "./backfillrunview";

type BackfillStatus = components["schemas"]["BackfillStatus"];

const run = (over: Partial<BackfillStatus>): BackfillStatus => ({
  state: "running",
  window: "6m",
  estimated_messages: 5000,
  counts: { messages_scanned: 1240, captured: 900, contacts_created: 30 },
  ...over,
});

const view = (status: BackfillStatus) =>
  render(
    <LocaleProvider initial="en">
      <RunView
        run={status}
        cancelling={false}
        cancelError={null}
        onCancel={() => undefined}
        onRestart={() => undefined}
      />
    </LocaleProvider>,
  );

afterEach(cleanup);

describe("the settings import card", () => {
  it("explains a scan that restarted at zero", () => {
    view(
      run({
        window_restarts: 1,
        retry_attempts: 1,
        counts: { messages_scanned: 0, captured: 0, contacts_created: 30 },
      }),
    );
    expect(
      screen.getByText(/started over because the mailbox lost/),
    ).toBeTruthy();
  });

  it("shows the server's count beside a sentence that says the page is counted again", () => {
    view(run({ retry_attempts: 1 }));
    expect(screen.getByText(/1,240 emails checked/)).toBeTruthy();
    expect(screen.getByText(/so its emails are counted again/)).toBeTruthy();
  });
});
