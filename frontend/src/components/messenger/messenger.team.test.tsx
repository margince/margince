/** @vitest-environment happy-dom */
import { en as englishCopy } from "../../i18n/en";
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { MessengerTeamPicker } from "./messenger.team";
import { messengerContext, messengerScope } from "./messenger.testkit";

type Person = components["schemas"]["MessengerTeamMember"];
const self: Person = {
  crm_id: "018f3a1b-0000-7000-8000-00000000a101",
  kind: "user",
  name: "Alex Morgan",
  member_id: "U_SELF",
  state: "self",
};
const colleague: Person = {
  crm_id: "018f3a1b-0000-7000-8000-00000000a102",
  kind: "user",
  name: "Jordan Lee",
  member_id: "U_JORDAN",
  state: "matched",
};
function fixture(
  members: readonly (Omit<Person, "state"> & { state: string })[],
) {
  const requests: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      if (path.endsWith("/me"))
        return new Response(JSON.stringify(meFixture({ allow: {} })), {
          headers: { "content-type": "application/json" },
        });
      const body = path.endsWith("/team")
        ? { members }
        : path.endsWith("/setup")
          ? {
              provider: "slack",
              configured: true,
              callback_url: "",
              participant_scopes: [],
              max_group_recipients: 8,
            }
          : messengerContext;
      if (request.method === "POST") requests.push(request);
      return new Response(JSON.stringify(body), {
        headers: { "content-type": "application/json" },
      });
    }),
  );
  const chosen = vi.fn();
  render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            queries: { retry: false },
            mutations: { retry: false },
          },
        })
      }
    >
      <LocaleProvider initial="en">
        <MessengerTeamPicker scope={messengerScope} onChoose={chosen} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { requests, chosen };
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("opens exactly the resolved record team while excluding the sending person's own ID", async () => {
  const user = userEvent.setup();
  const { requests, chosen } = fixture([self, colleague]);
  const action = await screen.findByRole("button", {
    name: englishCopy["messenger.messageTeam"],
  });
  expect(action).toBeEnabled();
  await user.click(action);
  await screen.findByText("Jordan Lee", { exact: false });
  expect(requests).toHaveLength(1);
  expect(await requests[0].clone().json()).toEqual({ members: ["U_JORDAN"] });
  await waitFor(() => expect(chosen).toHaveBeenCalledWith("C_PILOT"));
});
it.each(["unresolved", "inactive", "new_provider_state"] as const)(
  "keeps an %s teammate visible and refuses an incomplete all-team conversation",
  async (state) => {
    const { requests } = fixture([
      self,
      colleague,
      {
        ...colleague,
        crm_id: "018f3a1b-0000-7000-8000-00000000a103",
        member_id: "",
        name: "Missing teammate",
        state,
      },
    ]);
    expect(await screen.findByText(/Missing teammate/)).toBeVisible();
    expect(
      screen.getByRole("button", {
        name: englishCopy["messenger.messageTeam"],
      }),
    ).toBeDisabled();
    expect(requests).toHaveLength(0);
  },
);
it("refuses a team above Slack's group limit without splitting the team into separate messages", async () => {
  const { requests } = fixture([
    self,
    ...Array.from({ length: 9 }, (_, index) => ({
      ...colleague,
      crm_id: `018f3a1b-0000-7000-8000-${String(index).padStart(12, "0")}`,
      member_id: `U${index}`,
      name: `Colleague ${index}`,
    })),
  ]);
  expect(await screen.findByText(/up to 8 recipients/)).toBeVisible();
  expect(
    screen.getByRole("button", { name: englishCopy["messenger.messageTeam"] }),
  ).toBeDisabled();
  expect(requests).toHaveLength(0);
});
it("refuses an ambiguous team where two CRM members resolve to the same provider account", async () => {
  fixture([
    self,
    colleague,
    {
      ...colleague,
      crm_id: "018f3a1b-0000-7000-8000-00000000a103",
      name: "Second CRM person",
    },
  ]);
  expect(await screen.findByText(/The full team needs review/)).toBeVisible();
  expect(
    screen.getByRole("button", { name: englishCopy["messenger.messageTeam"] }),
  ).toBeDisabled();
});
