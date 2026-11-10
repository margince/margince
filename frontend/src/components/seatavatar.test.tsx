// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { SeatAvatar } from "./seatavatar";

const PICTURED = "00000000-0000-4000-8000-0000000000a1";
const PLAIN = "00000000-0000-4000-8000-0000000000a2";
const PICTURE = `/v1/users/${PICTURED}/avatar?v=7`;

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function namesRead() {
  return vi.spyOn(globalThis, "fetch").mockImplementation(
    async () =>
      new Response(
        JSON.stringify({
          data: [
            {
              id: PICTURED,
              display_name: "Lena Hoffmann",
              avatar_url: PICTURE,
              departed: false,
            },
            { id: PLAIN, display_name: "Jonas Becker", departed: false },
          ],
        }),
        { headers: { "Content-Type": "application/json" } },
      ),
  );
}

function Providers({ children }: Readonly<{ children: ReactNode }>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

const pictureOf = (container: HTMLElement) =>
  container.querySelector(".avatar-photo img");

it("draws the picture the names read carries for a seat named by id", async () => {
  namesRead();
  const { container } = render(
    <Providers>
      <SeatAvatar id={PICTURED} name="Lena Hoffmann" />
    </Providers>,
  );
  await waitFor(() =>
    expect(pictureOf(container)).toHaveAttribute("src", PICTURE),
  );
});

it("keeps the initials for a seat the read names without a picture", async () => {
  namesRead();
  const { container } = render(
    <Providers>
      <SeatAvatar id={PICTURED} name="Lena Hoffmann" />
      <SeatAvatar id={PLAIN} name="Jonas Becker" />
    </Providers>,
  );
  // One batch answers both, so once one picture is drawn the other has its answer.
  await waitFor(() => expect(pictureOf(container)).not.toBeNull());
  expect(container.querySelectorAll("img")).toHaveLength(1);
  expect(container.querySelector('[data-initials="JB"]')).not.toBeNull();
});

it("draws a held seat's own picture without asking for it", async () => {
  const fetch = namesRead();
  const { container } = render(
    <Providers>
      <SeatAvatar id={PICTURED} name="Lena Hoffmann" photo={null} />
      <SeatAvatar id={PLAIN} name="Jonas Becker" />
      <SeatAvatar id="u-held" name="Mia Wolf" photo={PICTURE} />
    </Providers>,
  );
  await waitFor(() => expect(fetch).toHaveBeenCalled());
  const [request] = fetch.mock.calls[0] ?? [];
  const asked =
    request instanceof Request
      ? new URL(request.url).searchParams.getAll("id")
      : [];
  expect(asked).toEqual([PLAIN]);
  expect(container.querySelectorAll(".avatar-photo img")).toHaveLength(1);
});
