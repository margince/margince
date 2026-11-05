/** @vitest-environment happy-dom */
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
import { OvernightGrantChoice } from "../../components/overnight-grant-choice";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { OvernightGrantCard } from "./overnight-grant";

// The rep's standing overnight authority, asked in two places.
//
// What is worth holding: the onboarding default is on, and turning it off names
// what stops working. Onboarding never writes. Settings writes on flip and,
// once off, says so with an Off badge instead of a warning.

/** The rep's one answer is held for both overnight agents. */
const bothAgents = (grants: Record<string, unknown>[]) =>
  grants.flatMap((grant) => [
    grant,
    { ...grant, spec: "overnight_at_risk_sweep" },
  ]);

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

/** The switch owns no state (the onboarding act does), so a test holds it. */
function Harness({ initial }: Readonly<{ initial: boolean }>) {
  const [checked, setChecked] = useState(initial);
  return <OvernightGrantChoice checked={checked} onChange={setChecked} />;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("arrives on, and says nothing alarming while it is", () => {
  installFetchStub({});
  render(<Harness initial={true} />);
  expect(
    screen
      .getByRole("switch", { name: en["overnightGrant.label"] })
      .getAttribute("aria-checked"),
  ).toBe("true");
  // The warning is the cost of saying no. Showing it against the default
  // would make the default look like the dangerous answer.
  expect(screen.queryByText(en["overnightGrant.danger"])).toBeNull();
});

it("names what goes empty the moment it is turned off", async () => {
  installFetchStub({});
  render(<Harness initial={true} />);
  await userEvent.click(
    screen.getByRole("switch", { name: en["overnightGrant.label"] }),
  );
  const warning = await screen.findByText(en["overnightGrant.danger"]);
  expect(warning).toBeTruthy();
  // Stated, never announced: off is the rep's own answer, and interrupting
  // them with its cost reads their decision back as a fault.
  expect(warning.closest("[role='alert']")).toBeNull();
});

it("writes nothing on its own: the onboarding step carries the answer", async () => {
  const calls: string[] = [];
  installFetchStub({
    "PUT /me/agent-grants/morning_brief": () => {
      calls.push("write");
      return jsonResponse({});
    },
  });
  render(<Harness initial={true} />);
  await userEvent.click(
    screen.getByRole("switch", { name: en["overnightGrant.label"] }),
  );
  await userEvent.click(
    screen.getByRole("switch", { name: en["overnightGrant.label"] }),
  );
  // The onboarding switch holds an answer the step submits on continue. One
  // that wrote on flip would grant for every reader passing through, since it
  // arrives on.
  expect(calls).toEqual([]);
});

it("the settings card shows the stored answer and writes when flipped", async () => {
  const written: unknown[] = [];
  installFetchStub({
    "GET /me/agent-grants": () =>
      jsonResponse({
        data: bothAgents([
          { spec: "morning_brief", state: "granted", credential_usable: true },
        ]),
      }),
    "PUT /me/agent-grants/morning_brief": (body: unknown) => {
      written.push(["morning_brief", body]);
      return jsonResponse({
        spec: "morning_brief",
        state: "declined",
        credential_usable: false,
      });
    },
    "PUT /me/agent-grants/overnight_at_risk_sweep": (body: unknown) => {
      written.push(["overnight_at_risk_sweep", body]);
      return jsonResponse({
        spec: "overnight_at_risk_sweep",
        state: "declined",
        credential_usable: false,
      });
    },
  });
  render(<OvernightGrantCard />);

  const toggle = await screen.findByTestId("overnight-grant-toggle");
  await waitFor(() => expect(toggle.getAttribute("aria-checked")).toBe("true"));
  await userEvent.click(toggle);
  // Unlike onboarding's, this switch is the action: flipping it writes, and it
  // writes the one answer for both overnight agents.
  await waitFor(() =>
    expect(written).toEqual([
      ["morning_brief", { granted: false }],
      ["overnight_at_risk_sweep", { granted: false }],
    ]),
  );
});

it("says a refused flip once for the card, and the switch returns to the stored answer", async () => {
  installFetchStub({
    "GET /me/agent-grants": () =>
      jsonResponse({
        data: bothAgents([
          { spec: "morning_brief", state: "granted", credential_usable: true },
        ]),
      }),
    "PUT /me/agent-grants/morning_brief": () =>
      jsonResponse(
        { title: "Unprocessable", detail: "the credential store refused" },
        422,
      ),
  });
  render(<OvernightGrantCard />);

  const toggle = await screen.findByTestId("overnight-grant-toggle");
  await waitFor(() => expect(toggle.getAttribute("aria-checked")).toBe("true"));
  await userEvent.click(toggle);
  expect(
    await screen.findByText(en["overnightGrant.writeFailedTitle"]),
  ).toBeTruthy();
  await waitFor(() => expect(toggle.getAttribute("aria-checked")).toBe("true"));
});

it("tells the rep the agent outgrew their authority, not that it lapsed", async () => {
  installFetchStub({
    "GET /me/agent-grants": () =>
      jsonResponse({
        data: bothAgents([
          // Live authority, neither revoked nor expired, for a job this agent
          // no longer does. The run degrades and prepares nothing at 2am.
          {
            spec: "morning_brief",
            state: "granted",
            credential_usable: true,
            credential_funds_agent: false,
          },
        ]),
      }),
  });
  render(<OvernightGrantCard />);

  expect(await screen.findByText(en["overnightGrant.renewScope"])).toBeTruthy();
  // NOT the expiry notice: nothing expired, and telling the rep it did sends
  // them looking for a lapse that never happened.
  expect(screen.queryByText(en["overnightGrant.renew"])).toBeNull();
  expect(screen.queryByText(en["overnightGrant.offNote"])).toBeNull();
});

it("says nothing when the authority still covers the agent", async () => {
  installFetchStub({
    "GET /me/agent-grants": () =>
      jsonResponse({
        data: bothAgents([
          {
            spec: "morning_brief",
            state: "granted",
            credential_usable: true,
            credential_funds_agent: true,
          },
        ]),
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
        data: bothAgents([
          // They said yes; the passport behind it has expired or been revoked.
          { spec: "morning_brief", state: "granted", credential_usable: false },
        ]),
      }),
  });
  render(<OvernightGrantCard />);

  expect(await screen.findByText(en["overnightGrant.renew"])).toBeTruthy();
  // Not the off sentence. They already answered, and it would put a settled
  // question back in front of them. Not the scope notice either. A lapsed
  // passport funds nothing, so both would be true. A rep reading two notices
  // cannot tell which to act on.
  expect(screen.queryByText(en["overnightGrant.offNote"])).toBeNull();
  expect(screen.queryByText(en["overnightGrant.renewScope"])).toBeNull();
});

it("reads a decline back quietly on Settings: an Off badge, no warning", async () => {
  installFetchStub({
    "GET /me/agent-grants": () =>
      jsonResponse({
        data: bothAgents([
          {
            spec: "morning_brief",
            state: "declined",
            credential_usable: false,
          },
        ]),
      }),
  });
  render(<OvernightGrantCard />);

  expect(await screen.findByText(en["overnightGrant.offNote"])).toBeTruthy();
  expect(screen.getByText(en["overnightGrant.off"])).toBeTruthy();
  // The warning belongs where the answer is first given. On Settings the answer
  // is settled, and a warning on every visit reads it back as a fault.
  expect(screen.queryByText(en["overnightGrant.danger"])).toBeNull();
});

it("drops the Off badge once the rep turns it on", async () => {
  installFetchStub({
    "GET /me/agent-grants": () =>
      jsonResponse({
        data: bothAgents([
          {
            spec: "morning_brief",
            state: "granted",
            credential_usable: true,
            credential_funds_agent: true,
          },
        ]),
      }),
  });
  render(<OvernightGrantCard />);

  await screen.findByTestId("overnight-grant-toggle");
  expect(screen.queryByText(en["overnightGrant.off"])).toBeNull();
  expect(screen.queryByText(en["overnightGrant.offNote"])).toBeNull();
});

it("says the same thing about the same authority in both places", async () => {
  const user = userEvent.setup();
  let state = "granted";
  installFetchStub({
    "GET /me/agent-grants": () =>
      jsonResponse({
        data: bothAgents([
          { spec: "morning_brief", state, credential_usable: true },
        ]),
      }),
    "PUT /me/agent-grants/morning_brief": () => {
      state = "declined";
      return jsonResponse({ spec: "morning_brief", state });
    },
  });
  const settings = render(<OvernightGrantCard />);
  await user.click(await screen.findByTestId("overnight-grant-toggle"));
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

it("shows the question open to a rep who granted only the morning brief", async () => {
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
          {
            spec: "overnight_at_risk_sweep",
            state: "declined",
            credential_usable: false,
          },
        ],
      }),
  });
  render(<OvernightGrantCard />);

  const toggle = await screen.findByTestId("overnight-grant-toggle");
  // The sweep runs only for a rep who granted it, so a brief-only answer is not
  // "on": turning it on once grants both.
  expect(toggle.getAttribute("aria-checked")).toBe("false");
});

it("reads the grants again when the second agent's write fails", async () => {
  let reads = 0;
  installFetchStub({
    "GET /me/agent-grants": () => {
      reads += 1;
      return jsonResponse({
        data: bothAgents([
          { spec: "morning_brief", state: "granted", credential_usable: true },
        ]),
      });
    },
    "PUT /me/agent-grants/morning_brief": () =>
      jsonResponse({
        spec: "morning_brief",
        state: "declined",
        credential_usable: false,
      }),
    "PUT /me/agent-grants/overnight_at_risk_sweep": () =>
      jsonResponse(
        { title: "Unprocessable", detail: "the credential store refused" },
        422,
      ),
  });
  render(<OvernightGrantCard />);

  const toggle = await screen.findByTestId("overnight-grant-toggle");
  await waitFor(() => expect(toggle.getAttribute("aria-checked")).toBe("true"));
  await userEvent.click(toggle);
  // The first write landed and the second did not. The card must ask the server
  // how the grants stand instead of showing what it cached.
  await waitFor(() => expect(reads).toBeGreaterThanOrEqual(2));
});
