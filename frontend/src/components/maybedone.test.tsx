/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
} from "../testing/story-utils";
import { MayBeDone, type MayBeDoneQuestion } from "./maybedone";

// A promise task the reader may have kept in a later mail asks on the
// Worklist as it does on the contact page. Done completes the task. Not yet
// is the contact page's own set-aside under the same key.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const ASKED: MayBeDoneQuestion = {
  contact_id: "p-1",
  email_activity_id: "e-1",
  wrote_at: "2026-08-12T10:00:00Z",
  wrote_precision: "instant",
  claim_key: "moment:may_be_done:a-9",
  evidence_fingerprint: "fp-1",
};

// The task the question is about, as either list hands it in.
const TASK = { id: "a-9", version: 4, canComplete: true };

function show(question: MayBeDoneQuestion | undefined) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <MayBeDone question={question} task={TASK} refresh={[["worklist"]]} />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("a Worklist promise our later mail may have kept", () => {
  it("asks, and Not yet sets the question aside under the contact page's key", async () => {
    const setAside = vi.fn();
    installFetchStub({
      "GET /me": meRoute({ activity: ["read", "update"] }),
      "POST /contacts/p-1/moment/dismiss": (body) => {
        setAside(body);
        return new Response(null, { status: 204 });
      },
    });
    show(ASKED);

    expect(
      screen.getByText(/You may have done this: you wrote to them on/),
    ).toBeTruthy();
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "Not yet" }));
    await waitFor(() =>
      expect(setAside).toHaveBeenCalledWith({
        claim_key: "moment:may_be_done:a-9",
        evidence_fingerprint: "fp-1",
      }),
    );
  });

  it("completes the task on Done", async () => {
    const patched = vi.fn();
    installFetchStub({
      "GET /me": meRoute({ activity: ["read", "update"] }),
      "PATCH /activities/a-9": (body) => {
        patched(body);
        return jsonResponse({ id: "a-9", is_done: true, version: 5 });
      },
    });
    show(ASKED);

    await userEvent.setup().click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() =>
      expect(patched).toHaveBeenCalledWith({ is_done: true }),
    );
  });

  it("asks nothing on a row without the question", () => {
    show(undefined);
    expect(screen.queryByRole("button", { name: "Not yet" })).toBeNull();
  });
});
