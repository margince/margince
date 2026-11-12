// @vitest-environment happy-dom
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import {
  AUTOMATION_CATALOG,
  configuredAutomation,
} from "./automations.fixtures";
import { ConfiguredAutomations } from "./automations.instances";

afterEach(() => {
  cleanup();
  globalThis.localStorage.clear();
});

function mount(previewFails = false) {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  const rule: components["schemas"]["Automation"] = {
    ...configuredAutomation("au-1", Date.parse("2026-09-01T12:00:00Z")),
    name: "Quiet deals",
    status: "paused",
    initial_run_preview_required: true,
  };
  const bodies: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({ automation: ["read", "update"] }),
    "POST /automations/au-1/preview": () =>
      previewFails
        ? jsonResponse({ title: "Preview failed", status: 500 }, 500)
        : jsonResponse({
            preview_kind: "population",
            matches_now_available: true,
            matches_now: 4,
            window_days: 30,
          }),
    "PATCH /automations/au-1": () =>
      jsonResponse({ ...rule, status: "enabled" }),
  });
  const stubbed = globalThis.fetch;
  globalThis.fetch = async (input, init) => {
    const request = new Request(input, init);
    if (request.method === "PATCH") {
      bodies.push(await request.clone().json());
    }
    return stubbed(input, init);
  };
  render(
    <StoryProviders>
      <ConfiguredAutomations
        automations={[rule]}
        entryFor={(key) =>
          AUTOMATION_CATALOG.find((entry) => entry.key === key)
        }
        canEdit
        canViewRuns={false}
        canDelete={false}
      />
    </StoryProviders>,
  );
  return bodies;
}

it("previews existing work before the enable request carries its confirmation", async () => {
  const user = userEvent.setup();
  const bodies = mount();
  await user.click(await screen.findByRole("switch"));
  const dialog = within(await screen.findByRole("dialog"));
  expect(await dialog.findByText("Matches now: 4")).toBeVisible();
  expect(bodies).toEqual([]);
  await user.click(
    dialog.getByRole("button", { name: en["auto.initialRun.confirm"] }),
  );
  await waitFor(() =>
    expect(bodies).toEqual([
      { status: "enabled", initial_run_confirmed: true },
    ]),
  );
});

it("keeps enable blocked when the initial preview fails", async () => {
  const user = userEvent.setup();
  const bodies = mount(true);
  await user.click(await screen.findByRole("switch"));
  const dialog = within(await screen.findByRole("dialog"));
  expect(await dialog.findByRole("alert")).toBeVisible();
  expect(
    dialog.getByRole("button", { name: en["auto.initialRun.confirm"] }),
  ).toBeDisabled();
  expect(bodies).toEqual([]);
});
