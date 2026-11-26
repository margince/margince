// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  ToastProvider,
  ToastRegion,
  useToast,
} from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { ChatIdentityRows } from "./chatidentities";

type Sent = { method: string; path: string };
let sent: Sent[];
let listed: unknown[];

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

beforeEach(() => {
  sent = [];
  listed = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const request = input instanceof Request ? input : null;
    const method = request?.method ?? init?.method ?? "GET";
    const path = new URL(request?.url ?? String(input), "http://localhost")
      .pathname;
    sent.push({ method, path });
    if (method === "POST") {
      return json({
        network: "whatsapp",
        code: "K7M2QX9A",
        expires_at: "2099-01-01T00:00:00Z",
      });
    }
    if (method === "DELETE") {
      return new Response(null, { status: 204 });
    }
    return json({ data: listed });
  });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function Rows() {
  return <ChatIdentityRows toast={useToast()} />;
}

function show() {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ToastProvider>
          <Rows />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

it("asks for a code for the network and shows it with the instruction", async () => {
  const user = userEvent.setup();
  show();
  await user.click(
    (await screen.findAllByRole("button", { name: "Confirm with a code" }))[0],
  );
  expect(await screen.findByText(/K7M2QX9A/)).toBeInTheDocument();
  expect(screen.getByText(/within 10 minutes/)).toBeInTheDocument();
  expect(sent).toContainEqual({
    method: "POST",
    path: "/v1/me/chat-identities/whatsapp",
  });
});

it("shows a confirmed account with its hidden number and removes it", async () => {
  listed = [
    {
      network: "telegram",
      account_hint: "…4821",
      verified_at: "2026-11-01T10:00:00Z",
    },
  ];
  const user = userEvent.setup();
  show();
  expect(await screen.findByText("Confirmed: …4821")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Remove" }));
  await waitFor(() =>
    expect(sent).toContainEqual({
      method: "DELETE",
      path: "/v1/me/chat-identities/telegram",
    }),
  );
  expect(await screen.findByText("Account removed")).toBeInTheDocument();
});
