/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { installFetchStub, jsonResponse } from "../testing/story-utils";
import { fromHere } from "./navigatefrom";
import { useLeaveIfOutOfReach } from "./outofreach";

const originalFetch = globalThis.fetch;

beforeEach(() => {
  globalThis.history.replaceState(null, "", "#/contacts/c1");
});

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

function leaveIfOutOfReach() {
  const client = new QueryClient();
  const wrapper = ({ children }: Readonly<{ children: ReactNode }>) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return renderHook(() => useLeaveIfOutOfReach("contact"), { wrapper }).result
    .current;
}

it("puts the list in place of a record the reader can no longer open", async () => {
  installFetchStub({
    "GET /contacts/c1": () => jsonResponse({ status: 404 }, 404),
  });
  const leave = leaveIfOutOfReach();
  const entries = globalThis.history.length;

  await expect(leave("c1", "Handed over.", fromHere())).resolves.toBe(true);
  expect(globalThis.location.hash).toBe("#/contacts");
  expect(globalThis.history.length).toBe(entries);
});

it("takes a reader who opened another tab of the record during the write to the list", async () => {
  installFetchStub({
    "GET /contacts/c1": () => jsonResponse({ status: 404 }, 404),
  });
  const leave = leaveIfOutOfReach();
  const from = fromHere();
  globalThis.history.replaceState(null, "", "#/contacts/c1/timeline");

  await expect(leave("c1", "Handed over.", from)).resolves.toBe(true);
  expect(globalThis.location.hash).toBe("#/contacts");
});

it("leaves a reader who moved on during the write where they are", async () => {
  installFetchStub({
    "GET /contacts/c1": () => jsonResponse({ status: 404 }, 404),
  });
  const leave = leaveIfOutOfReach();
  const from = fromHere();
  globalThis.history.replaceState(null, "", "#/deals/d1");

  await expect(leave("c1", "Handed over.", from)).resolves.toBe(true);
  expect(globalThis.location.hash).toBe("#/deals/d1");
});

it("leaves a reader who moved on while the record was checked where they are", async () => {
  let answer: (response: Response) => void = () => undefined;
  const asked = vi.fn(
    () =>
      new Promise<Response>((resolve) => {
        answer = resolve;
      }),
  );
  installFetchStub({ "GET /contacts/c1": asked });
  const leave = leaveIfOutOfReach();

  const left = leave("c1", "Handed over.", fromHere());
  await vi.waitFor(() => expect(asked).toHaveBeenCalled());
  globalThis.history.replaceState(null, "", "#/deals/d1");
  answer(jsonResponse({ status: 404 }, 404));

  await expect(left).resolves.toBe(true);
  expect(globalThis.location.hash).toBe("#/deals/d1");
});
