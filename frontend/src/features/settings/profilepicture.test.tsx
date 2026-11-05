// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import {
  ToastProvider,
  ToastRegion,
  useToast,
} from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { ProfilePictureRow } from "./profilepicture";

type User = components["schemas"]["User"];

const ME = meFixture().user;
const PICTURE = `/v1/users/${ME.id}/avatar?v=2`;
const COLLEAGUE_NAME = ["users", "name", ME.id];

type Sent = { method: string; path: string; body: unknown };

let client: QueryClient;
let sent: Sent[];

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  client.setQueryData(["me"], meFixture());
  client.setQueryData(COLLEAGUE_NAME, { name: ME.display_name });
  sent = [];
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// The server answers each write with `answer`, and every request is kept.
function serve(answer: (method: string) => Response) {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const request = input instanceof Request ? input : null;
    const method = request?.method ?? init?.method ?? "GET";
    const url = new URL(request?.url ?? String(input), "http://localhost");
    sent.push({ method, path: url.pathname, body: init?.body });
    return answer(method);
  });
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function Row({ user }: Readonly<{ user: User }>) {
  return <ProfilePictureRow user={user} toast={useToast()} />;
}

function show(user: User = ME) {
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <Row user={user} />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const picker = () => screen.getByLabelText("Profile picture");

it("refuses a file that is not PNG, JPEG or WebP before it is sent", async () => {
  serve(() => json(ME));
  const user = userEvent.setup({ applyAccept: false });
  show();
  await user.upload(
    picker(),
    new File(["gif"], "me.gif", { type: "image/gif" }),
  );
  expect(
    await screen.findByText(
      "Picture not uploaded. Use a PNG, JPEG or WebP file.",
    ),
  ).toBeInTheDocument();
  expect(sent).toEqual([]);
});

it("refuses a file over 5 MB before it is sent", async () => {
  serve(() => json(ME));
  const user = userEvent.setup();
  show();
  const big = new File([new Uint8Array(5 * 1024 * 1024 + 1)], "me.png", {
    type: "image/png",
  });
  await user.upload(picker(), big);
  expect(
    await screen.findByText("Picture not uploaded. Use a file up to 5 MB."),
  ).toBeInTheDocument();
  expect(sent).toEqual([]);
});

it("uploads the picked file and reads every chip of the seat again", async () => {
  serve(() => json({ ...ME, avatar_url: PICTURE }));
  const user = userEvent.setup();
  show();
  const file = new File(["png"], "me.png", { type: "image/png" });
  await user.upload(picker(), file);

  expect(await screen.findByText("Picture saved")).toBeInTheDocument();
  expect(sent.map(({ method, path }) => `${method} ${path}`)).toEqual([
    "POST /v1/me/avatar",
  ]);
  const body = sent[0]?.body;
  expect(body instanceof FormData ? body.get("file") : null).toBe(file);
  expect(client.getQueryState(["me"])?.isInvalidated).toBe(true);
  expect(client.getQueryState(COLLEAGUE_NAME)?.isInvalidated).toBe(true);
});

it("removes the picture on one press, with no question first", async () => {
  serve(() => json(ME));
  const user = userEvent.setup();
  show({ ...ME, avatar_url: PICTURE });
  await user.click(screen.getByRole("button", { name: "Remove picture" }));

  expect(await screen.findByText("Picture removed")).toBeInTheDocument();
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(sent.map(({ method, path }) => `${method} ${path}`)).toEqual([
    "DELETE /v1/me/avatar",
  ]);
  expect(client.getQueryState(["me"])?.isInvalidated).toBe(true);
});

it("offers no removal while the seat shows initials", () => {
  serve(() => json(ME));
  show();
  expect(
    screen.getByRole("button", { name: "Upload picture" }),
  ).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Remove picture" })).toBeNull();
});

it("says in words that the installation cannot store pictures", async () => {
  serve(
    () =>
      new Response(JSON.stringify({ status: 501, code: "not_implemented" }), {
        status: 501,
        headers: { "content-type": "application/problem+json" },
      }),
  );
  const user = userEvent.setup();
  show();
  await user.upload(
    picker(),
    new File(["png"], "me.png", { type: "image/png" }),
  );
  expect(
    await screen.findByText(
      "This installation has no file storage for pictures. Contact your system administrator.",
    ),
  ).toBeInTheDocument();
  expect(client.getQueryState(["me"])?.isInvalidated).toBe(false);
});
