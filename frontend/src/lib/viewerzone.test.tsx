/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { viewerZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { meFixture } from "../testing/mefixture";
import { installFetchStub, jsonResponse } from "../testing/story-utils";
import { useBrowserZoneOnFirstSignIn } from "./viewerzone";

type Me = components["schemas"]["MeResponse"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function Probe({ me }: Readonly<{ me: Me | undefined }>) {
  useBrowserZoneOnFirstSignIn(me);
  return null;
}

function wrap(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>
  );
}

function stubWrites() {
  const writes: unknown[] = [];
  installFetchStub({
    "PUT /me/timezone": (body) => {
      writes.push(body);
      return jsonResponse({ ...meFixture({}).user, timezone_chosen: false });
    },
  });
  return writes;
}

it("stores the browser's zone once, as a guess, when no zone is stored", async () => {
  const writes = stubWrites();
  const me = meFixture({});
  const unset: Me = { ...me, user: { ...me.user, timezone: undefined } };
  const view = render(wrap(<Probe me={unset} />));
  view.rerender(wrap(<Probe me={{ ...unset }} />));
  await vi.waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toEqual({
    timezone: viewerZone(),
    source: "browser",
  });
});

it("writes nothing when a zone is stored, or before /me has answered", async () => {
  const writes = stubWrites();
  render(wrap(<Probe me={meFixture({})} />));
  render(wrap(<Probe me={undefined} />));
  await new Promise((resolve) => setTimeout(resolve, 20));
  expect(writes).toHaveLength(0);
});
