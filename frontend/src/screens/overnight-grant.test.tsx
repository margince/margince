/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { OvernightGrantCard, OvernightGrantChoice } from "./overnight-grant";
import { installFetchStub, jsonResponse } from "./story-utils";

// The rep's standing overnight authority, asked in two places.
//
// What is worth holding is not that a checkbox renders: it is that the DEFAULT
// is ticked, that clearing it says out loud what stops working, and that the
// two surfaces say the SAME thing about the same authority — a rep who reads
// the warning in onboarding and again in Settings must not be told two
// different costs.

function render(ui: ReactNode) {
  return rtlRender(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

/** The checkbox owns no state — the onboarding act does — so a test has to
 * hold it for the box to be tickable at all. */
function Harness({ initial }: Readonly<{ initial: boolean }>) {
  const [checked, setChecked] = useState(initial);
  return <OvernightGrantChoice checked={checked} onChange={setChecked} />;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("arrives ticked, and says nothing alarming while it is", () => {
  installFetchStub({});
  render(<Harness initial={true} />);
  expect(
    screen.getByRole("checkbox", { name: en["overnightGrant.label"] }),
  ).toHaveProperty("checked", true);
  // The warning is the cost of saying no. Showing it against a ticked box
  // would make the default look like the dangerous answer.
  expect(screen.queryByText(en["overnightGrant.danger"])).toBeNull();
});

it("names what goes empty the moment it is cleared", async () => {
  installFetchStub({});
  render(<Harness initial={true} />);
  await userEvent.click(
    screen.getByRole("checkbox", { name: en["overnightGrant.label"] }),
  );
  const warning = await screen.findByText(en["overnightGrant.danger"]);
  // role=alert, so it is announced rather than only seen: a reader who cleared
  // the box with the keyboard never looks at the space below it.
  expect(warning.closest("[role='alert']")).toBeTruthy();
});

it("writes nothing on its own — the onboarding step carries the answer", async () => {
  const calls: string[] = [];
  installFetchStub({
    "PUT /me/agent-grants/morning_brief": () => {
      calls.push("write");
      return jsonResponse({});
    },
  });
  render(<Harness initial={true} />);
  await userEvent.click(
    screen.getByRole("checkbox", { name: en["overnightGrant.label"] }),
  );
  await userEvent.click(
    screen.getByRole("checkbox", { name: en["overnightGrant.label"] }),
  );
  // Ticking is an INTENT the connect step submits. A box that wrote on tick
  // would grant an authority for every reader who merely passed through the
  // screen, since it arrives ticked.
  expect(calls).toEqual([]);
});

it("the settings card shows the stored answer and writes when flipped", async () => {
  const written: unknown[] = [];
  installFetchStub({
    "GET /me/agent-grants": () =>
      jsonResponse({
        data: [
          { spec: "morning_brief", state: "granted", credential_usable: true },
        ],
      }),
    "PUT /me/agent-grants/morning_brief": (body: unknown) => {
      written.push(body);
      return jsonResponse({
        spec: "morning_brief",
        state: "declined",
        credential_usable: false,
      });
    },
  });
  render(<OvernightGrantCard />);

  const toggle = await screen.findByTestId("overnight-grant-toggle");
  await waitFor(() => expect(toggle.getAttribute("aria-checked")).toBe("true"));
  await userEvent.click(toggle);
  // Unlike the checkbox, a Switch IS the action: flipping it writes.
  await waitFor(() => expect(written).toEqual([{ granted: false }]));
});

it("tells the rep the agent outgrew their authority, not that it lapsed", async () => {
  installFetchStub({
    "GET /me/agent-grants": () =>
      jsonResponse({
        data: [
          // Live authority — neither revoked nor expired — for a job this
          // agent no longer does. The run does not fail; it degrades the
          // unfunded tools away and prepares nothing, silently, at 2am.
          {
            spec: "morning_brief",
            state: "granted",
            credential_usable: true,
            credential_funds_agent: false,
          },
        ],
      }),
  });
  render(<OvernightGrantCard />);

  expect(await screen.findByText(en["overnightGrant.renewScope"])).toBeTruthy();
  // NOT the expiry notice: nothing expired, and telling the rep it did sends
  // them looking for a lapse that never happened.
  expect(screen.queryByText(en["overnightGrant.renew"])).toBeNull();
  expect(screen.queryByText(en["overnightGrant.danger"])).toBeNull();
});

it("says nothing when the authority still covers the agent", async () => {
  installFetchStub({
    "GET /me/agent-grants": () =>
      jsonResponse({
        data: [
          {
            spec: "morning_brief",
            state: "granted",
            credential_usable: true,
            credential_funds_agent: true,
          },
        ],
      }),
  });
  render(<OvernightGrantCard />);

  await screen.findByTestId("overnight-grant-toggle");
  expect(screen.queryByText(en["overnightGrant.renewScope"])).toBeNull();
  expect(screen.queryByText(en["overnightGrant.renew"])).toBeNull();
});

it("tells the rep their authority died rather than that they declined", async () => {
  installFetchStub({
    "GET /me/agent-grants": () =>
      jsonResponse({
        data: [
          // They said yes; the passport behind it has expired or been revoked.
          { spec: "morning_brief", state: "granted", credential_usable: false },
        ],
      }),
  });
  render(<OvernightGrantCard />);

  expect(await screen.findByText(en["overnightGrant.renew"])).toBeTruthy();
  // NOT the decline warning: they already answered, and showing that would put
  // a settled question back in front of them. And not the scope notice either:
  // a lapsed passport funds nothing, so BOTH would be true of it and a rep
  // reading two notices cannot tell which one to act on.
  expect(screen.queryByText(en["overnightGrant.danger"])).toBeNull();
  expect(screen.queryByText(en["overnightGrant.renewScope"])).toBeNull();
});

it("says the same thing about the same authority in both places", async () => {
  installFetchStub({
    "GET /me/agent-grants": () =>
      jsonResponse({
        data: [
          {
            spec: "morning_brief",
            state: "declined",
            credential_usable: false,
          },
        ],
      }),
  });
  const settings = render(<OvernightGrantCard />);
  const inSettings = await screen.findByText(en["overnightGrant.danger"]);
  const settingsWords = inSettings.textContent;
  settings.unmount();

  installFetchStub({});
  render(<Harness initial={false} />);
  const inOnboarding = await screen.findByText(en["overnightGrant.danger"]);
  // One catalog key renders both, so this holds the SHARING rather than the
  // wording: a second spelling of the warning would let the two drift into
  // describing different costs for one decision.
  expect(inOnboarding.textContent).toBe(settingsWords);
});
