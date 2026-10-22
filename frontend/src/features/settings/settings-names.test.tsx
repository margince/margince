/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import { SettingList } from "../../design-system/settingrow";
import { useToast } from "../../design-system/toast";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { NameSettingRows, useAccountNames } from "./settings-names";

function Rows() {
  const names = useAccountNames(useToast());
  return (
    <SettingList bleed="settings">
      <NameSettingRows names={names} />
    </SettingList>
  );
}

function mount(answer: () => Response) {
  const sent: unknown[] = [];
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture({})),
    "PUT /me/greeting-name": (body) => {
      sent.push(body);
      return answer();
    },
  });
  render(
    <StoryProviders>
      <SavePageProvider>
        <Rows />
        <PageSaveBar />
      </SavePageProvider>
    </StoryProviders>,
  );
  return sent;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("offers no save until a name moves", async () => {
  mount(() => jsonResponse({ greeting_name: "Ada" }));
  await screen.findByRole("textbox", { name: "Greeting name" });
  expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
});

it("writes the greeting name from the page's save bar", async () => {
  const user = userEvent.setup();
  const sent = mount(() => jsonResponse({ greeting_name: "Ada" }));
  await user.type(
    await screen.findByRole("textbox", { name: "Greeting name" }),
    "Ada",
  );
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(sent).toEqual([{ greeting_name: "Ada" }]));
});

it("says a refused save on the bar and under the refused field, and keeps what was typed", async () => {
  const user = userEvent.setup();
  mount(
    () =>
      new Response(
        JSON.stringify({ status: 503, code: "service_unavailable" }),
        {
          status: 503,
          headers: { "content-type": "application/problem+json" },
        },
      ),
  );
  const field = await screen.findByRole("textbox", { name: "Greeting name" });
  await user.type(field, "Ada");
  await user.click(screen.getByRole("button", { name: "Save" }));
  const alerts = await screen.findAllByRole("alert");
  expect(field).toHaveValue("Ada");
  // The bar carries the summary; the refused field carries the same sentence by
  // its own id, so a screen reader hears which field was refused.
  const named = (field.getAttribute("aria-describedby") ?? "").split(" ");
  expect(alerts.some((alert) => named.includes(alert.id))).toBe(true);
  const other = screen.getByRole("textbox", { name: "Display name" });
  expect(other.getAttribute("aria-describedby") ?? "").not.toContain(
    alerts.find((alert) => named.includes(alert.id))?.id ?? "none",
  );
});

it("does not save on the Enter that ends an IME composition", async () => {
  const user = userEvent.setup();
  const sent = mount(() => jsonResponse({ greeting_name: "Việt" }));
  const field = await screen.findByRole("textbox", { name: "Greeting name" });
  await user.type(field, "Việt");
  fireEvent.keyDown(field, { key: "Enter", isComposing: true });
  fireEvent.keyDown(field, { key: "Enter", keyCode: 229 });
  expect(sent).toHaveLength(0);
  fireEvent.keyDown(field, { key: "Enter" });
  await waitFor(() => expect(sent).toHaveLength(1));
});
