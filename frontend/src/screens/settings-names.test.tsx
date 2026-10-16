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
import { SettingList } from "../design-system/settingrow";
import { meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { GreetingNameSettingRow } from "./settings-names";

const NO_TOAST = { show: () => 0, dismiss: () => {} };

function mount(answer: () => Promise<Response> | Response) {
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
      <SettingList bleed="settings">
        <GreetingNameSettingRow toast={NO_TOAST} />
      </SettingList>
    </StoryProviders>,
  );
  return sent;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
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

it("sends one write for two Enters while the first is still out", async () => {
  const user = userEvent.setup();
  let land: (response: Response) => void = () => {};
  const sent = mount(
    () =>
      new Promise<Response>((resolve) => {
        land = resolve;
      }),
  );
  const field = await screen.findByRole("textbox", { name: "Greeting name" });
  await user.type(field, "Ada{Enter}");
  await waitFor(() => expect(sent).toHaveLength(1));
  await user.keyboard("{Enter}");
  land(jsonResponse({ greeting_name: "Ada" }));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "Save" })).not.toHaveAttribute(
      "aria-busy",
    ),
  );
  expect(sent).toHaveLength(1);
});

it("hands focus back to the field after Save", async () => {
  const user = userEvent.setup();
  const sent = mount(() => jsonResponse({ greeting_name: "Ada" }));
  const field = await screen.findByRole("textbox", { name: "Greeting name" });
  await user.type(field, "Ada");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(sent).toHaveLength(1));
  await waitFor(() => expect(field).toHaveFocus());
});

it("hands focus back to the field after a refused Save", async () => {
  const user = userEvent.setup();
  mount(
    () =>
      new Response(JSON.stringify({ status: 503, code: "service_unavailable" }), {
        status: 503,
        headers: { "content-type": "application/problem+json" },
      }),
  );
  const field = await screen.findByRole("textbox", { name: "Greeting name" });
  await user.type(field, "Ada");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByRole("alert");
  await waitFor(() => expect(field).toHaveFocus());
});

it("leaves focus where the reader moved it while the save was out", async () => {
  const user = userEvent.setup();
  let land: (response: Response) => void = () => {};
  const sent = mount(
    () =>
      new Promise<Response>((resolve) => {
        land = resolve;
      }),
  );
  const elsewhere = document.body.appendChild(document.createElement("button"));
  const field = await screen.findByRole("textbox", { name: "Greeting name" });
  await user.type(field, "Ada");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(sent).toHaveLength(1));
  elsewhere.focus();
  land(jsonResponse({ greeting_name: "Ada" }));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "Save" })).not.toHaveAttribute(
      "aria-busy",
    ),
  );
  expect(elsewhere).toHaveFocus();
  elsewhere.remove();
});
