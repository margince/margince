/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { ConnectorContextTagRow } from "./connectors.contexttag";
import { installFetchStub, jsonResponse } from "./story-utils";

// The word a connector files under, as the operator sets it.
//
// What matters on this side is that the control offers only words the
// vocabulary carries, that a retired word says so rather than going quiet, and
// that clearing it sends null rather than an empty string — the contract's own
// spelling for "files nothing".

type CaptureConnection = components["schemas"]["CaptureConnection"];

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

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function connection(over: Partial<CaptureConnection> = {}): CaptureConnection {
  return {
    id: "c1",
    provider: "gmail",
    status: "connected",
    scopes: ["read"],
    ...over,
  } as CaptureConnection;
}

const vocabulary = {
  data: [
    { id: "t-nord", name: "Nord inbox" },
    { id: "t-sued", name: "Sued inbox" },
  ],
  page: { has_more: false },
};

it("offers the words the vocabulary carries, and none of its own", async () => {
  installFetchStub({ "GET /tags": () => jsonResponse(vocabulary) });
  render(<ConnectorContextTagRow conn={connection()} />);

  await userEvent.click(
    await screen.findByRole("combobox", {
      name: en["connectors.contextTag.label"],
    }),
  );
  const options = within(screen.getByRole("listbox"))
    .getAllByRole("option")
    .map((option) => option.textContent);
  expect(options).toEqual([
    en["connectors.contextTag.none"],
    "Nord inbox",
    "Sued inbox",
  ]);
});

it("clears the choice as null, which is the contract's word for files nothing", async () => {
  const sent: unknown[] = [];
  installFetchStub({
    "GET /tags": () => jsonResponse(vocabulary),
    // The stub hands a non-GET handler the DECODED body, which is the shape
    // this test is about.
    "PUT /connectors/gmail/context-tag": (body: unknown) => {
      sent.push(body);
      return jsonResponse({});
    },
  });
  render(
    <ConnectorContextTagRow
      conn={connection({
        context_tag: { id: "t-nord", name: "Nord inbox", archived: false },
      })}
    />,
  );

  await userEvent.click(
    await screen.findByRole("combobox", {
      name: en["connectors.contextTag.label"],
    }),
  );
  await userEvent.click(
    within(screen.getByRole("listbox")).getByRole("option", {
      name: en["connectors.contextTag.none"],
    }),
  );
  // An empty string is what a <select> yields and NOT what the contract takes:
  // tag_id is a uuid or null, so an empty string would be refused as malformed
  // rather than read as "no word".
  await waitFor(() => expect(sent).toEqual([{ tag_id: null }]));
});

// A word retired after it was chosen leaves the connector filing nothing. The
// row says so, because the alternative is a setting that quietly stopped
// working with nothing on screen to look at.
it("says a retired word has stopped filing anything", async () => {
  installFetchStub({ "GET /tags": () => jsonResponse(vocabulary) });
  render(
    <ConnectorContextTagRow
      conn={connection({
        context_tag: { id: "t-old", name: "Retired inbox", archived: true },
      })}
    />,
  );

  expect(
    await screen.findByText(
      en["connectors.contextTag.archived"].replace("{name}", "Retired inbox"),
    ),
  ).toBeTruthy();
});
