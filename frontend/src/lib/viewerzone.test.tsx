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

function wrap(
  ui: ReactNode,
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
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

function unsetFor(id: string): Me {
  const me = meFixture({});
  return { ...me, user: { ...me.user, id, timezone: undefined } };
}

it("asks again for a different person signing in on the same tab", async () => {
  const writes = stubWrites();
  const view = render(wrap(<Probe me={unsetFor("u-1")} />));
  await vi.waitFor(() => expect(writes).toHaveLength(1));

  view.rerender(wrap(<Probe me={unsetFor("u-2")} />));

  await vi.waitFor(() => expect(writes).toHaveLength(2));
});

it("tries a failed guess again on the next read of /me", async () => {
  let attempts = 0;
  installFetchStub({
    "PUT /me/timezone": () => {
      attempts++;
      return attempts === 1
        ? jsonResponse({ title: "Unavailable", status: 503 }, 503)
        : jsonResponse({ ...meFixture({}).user, timezone_chosen: false });
    },
  });
  const view = render(wrap(<Probe me={unsetFor("u-1")} />));
  await vi.waitFor(() => expect(attempts).toBe(1));
  await new Promise((resolve) => setTimeout(resolve, 20));

  view.rerender(wrap(<Probe me={unsetFor("u-1")} />));

  await vi.waitFor(() => expect(attempts).toBe(2));
});

it("reads /me again after the write instead of writing the answer into it", async () => {
  stubWrites();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const unset = unsetFor("u-1");
  client.setQueryData(["me"], unset);
  render(wrap(<Probe me={unset} />, client));

  await vi.waitFor(() =>
    expect(client.getQueryState(["me"])?.isInvalidated).toBe(true),
  );
  expect(client.getQueryData<Me>(["me"])?.user.timezone).toBeUndefined();
});
