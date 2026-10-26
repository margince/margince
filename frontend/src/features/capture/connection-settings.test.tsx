/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { AccountSettings } from "./connection-settings";

// A mailbox's posture waits on the page's save bar. A narrowing asks beside the
// choice whether captured mail narrows too, and only a narrowing sends that.

type CaptureConnection = components["schemas"]["CaptureConnection"];

const mailbox = {
  id: "c1",
  provider: "gmail",
  status: "connected",
  scopes: ["read"],
  mail_posture: "classified",
} as CaptureConnection;

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderPosture() {
  const sent: unknown[] = [];
  installFetchStub({
    "GET /capture/settings": () =>
      jsonResponse({ shared_posture_allowed: true }),
    "PUT /connectors/gmail/mail-posture": (body: unknown) => {
      sent.push(body);
      return jsonResponse({});
    },
  });
  rtlRender(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <SavePageProvider>
          <AccountSettings mailbox={mailbox} others={[]} accountName="Ada" />
          <PageSaveBar />
        </SavePageProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return sent;
}

async function pickPosture(name: string) {
  await userEvent.click(
    await screen.findByRole("combobox", {
      name: en["connectors.mailPosture.label"],
    }),
  );
  await userEvent.click(
    within(screen.getByRole("listbox")).getByRole("option", { name }),
  );
}

it("sends a narrowing with its history answer once Save is pressed", async () => {
  const sent = renderPosture();
  await pickPosture(en["connectors.mailPosture.held"]);
  await userEvent.click(
    screen.getByRole("checkbox", {
      name: en["connectors.mailPosture.historyApply"],
    }),
  );
  expect(sent).toEqual([]);

  await userEvent.click(
    screen.getByRole("button", { name: en["saveBar.save"] }),
  );
  await waitFor(() =>
    expect(sent).toEqual([{ posture: "held", apply_to_history: true }]),
  );
});

it("asks nothing about history on a widening, and sends false", async () => {
  const sent = renderPosture();
  await pickPosture(en["connectors.mailPosture.shared"]);
  expect(
    screen.queryByRole("checkbox", {
      name: en["connectors.mailPosture.historyApply"],
    }),
  ).toBeNull();

  await userEvent.click(
    screen.getByRole("button", { name: en["saveBar.save"] }),
  );
  await waitFor(() =>
    expect(sent).toEqual([{ posture: "shared", apply_to_history: false }]),
  );
});

it("puts the stored posture back on Discard and sends nothing", async () => {
  const sent = renderPosture();
  await pickPosture(en["connectors.mailPosture.held"]);
  await userEvent.click(
    screen.getByRole("button", { name: en["saveBar.discard"] }),
  );

  expect(
    screen.getByRole("combobox", { name: en["connectors.mailPosture.label"] })
      .textContent,
  ).toContain(en["connectors.mailPosture.classified"]);
  expect(
    screen.queryByRole("checkbox", {
      name: en["connectors.mailPosture.historyApply"],
    }),
  ).toBeNull();
  expect(sent).toEqual([]);
});
