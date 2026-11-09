/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { LearnFromSentMail, useSentMailLearning } from "./voice-sentmail";

// Sent mail is read on the owner's press and at no other time, and the press
// answers with what it learned in one sentence.

type Learned = components["schemas"]["VoiceSentMailLearned"];

const SUMMARY: components["schemas"]["VoiceCorpusSummary"] = {
  total_words: 900,
  target_words: 30000,
  maturity: "collecting",
  quality_band: "thin",
  source_count: 3,
  register_words: { email: 900 },
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// Every request the row makes, and the answer the press gets.
function stubApi(answer: { status: number; body: unknown }) {
  const presses: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname.replace(/^\/v1/, "");
      if (path === "/voice-profiles") {
        return jsonResponse({
          data: [{ id: "vp-1" }],
          page: { next_cursor: null, has_more: false },
        });
      }
      if (path === "/voice-profiles/vp-1/sources/sent-mail") {
        presses.push(request.method);
        return jsonResponse(answer.body, answer.status);
      }
      return jsonResponse({});
    }),
  );
  return presses;
}

function learned(counts: Partial<Learned>): { status: number; body: Learned } {
  return {
    status: 200,
    body: {
      read: 0,
      added: 0,
      already_present: 0,
      no_own_words: 0,
      summary: SUMMARY,
      ...counts,
    },
  };
}

function Row({ onChanged }: Readonly<{ onChanged: () => void }>) {
  return <LearnFromSentMail learning={useSentMailLearning(onChanged)} />;
}

function renderRow(onChanged: () => void = () => undefined) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <Row onChanged={onChanged} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const button = () =>
  screen.getByRole("button", { name: "Learn from sent mail" });

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("learning the voice from sent mail", () => {
  it("reads no mail until the owner presses, and says what the press reads", () => {
    const presses = stubApi(learned({}));
    renderRow();
    expect(presses).toHaveLength(0);
    expect(screen.getByText(/only from your own mailbox/)).toBeTruthy();
    expect(
      screen.getByText(/remove single mails or clear all samples/),
    ).toBeTruthy();
  });

  it("reads once per press and reports how many mails taught the voice", async () => {
    const presses = stubApi(learned({ read: 40, added: 36, no_own_words: 4 }));
    const onChanged = vi.fn();
    const user = userEvent.setup();
    renderRow(onChanged);

    await user.click(button());

    expect(
      await screen.findByText(
        "Learned from 36 of your sent mails. Build your Voice DNA to use them.",
      ),
    ).toBeTruthy();
    expect(screen.getByText(/4 mails were left out/)).toBeTruthy();
    expect(presses).toEqual(["POST"]);
    expect(onChanged).toHaveBeenCalledTimes(1);
  });

  it("says plainly when there is no sent mail to learn from", async () => {
    stubApi(learned({}));
    const user = userEvent.setup();
    renderRow();

    await user.click(button());

    expect(await screen.findByText(/No sent mail to learn from/)).toBeTruthy();
  });

  it("says nothing new when every mail is already a sample", async () => {
    stubApi(learned({ read: 12, already_present: 12 }));
    const user = userEvent.setup();
    renderRow();

    await user.click(button());

    expect(await screen.findByText(/Nothing new to learn/)).toBeTruthy();
  });

  it("shows the server's problem when the press is refused", async () => {
    stubApi({
      status: 404,
      body: {
        code: "not_found",
        title: "Not found",
        detail: "That voice profile was not found.",
      },
    });
    const user = userEvent.setup();
    renderRow();

    await user.click(button());

    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toMatch(
        /Sent mail was not read/,
      ),
    );
  });
});
