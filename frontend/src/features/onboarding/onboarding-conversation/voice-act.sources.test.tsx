/** @vitest-environment happy-dom */

import { useQueryClient } from "@tanstack/react-query";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useReducer, useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { en } from "../../../i18n/en";
import { conversationReducer } from "./conversation-machine";
import { VOICE_PROBE_KEY, type VoiceRestoreProbe } from "./restore";
import { VoiceAct } from "./voice-act";
import {
  collectingState,
  documentStats,
  gate,
  INGEST_WAITER_MS,
  render,
  requestsTo,
  stubApi,
  summaryOf,
} from "./voice-act.testkit";

type CorpusSource = components["schemas"]["VoiceCorpusSource"];

beforeEach(() => {
  vi.stubGlobal("scrollTo", vi.fn());
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function stored(
  over: Partial<CorpusSource> & Pick<CorpusSource, "id" | "source_label">,
): CorpusSource {
  return {
    origin: "manual",
    kind: "document",
    register: "general",
    weight: 1,
    source_ref: `voice:upload:${over.id}`,
    word_count: 350,
    included: true,
    exclusion_reason: null,
    extractor_version: "1",
    occurred_at: "2026-11-01T10:00:00Z",
    retention_until: null,
    content_erased_at: null,
    source: "manual",
    captured_by: "user-1",
    version: 4,
    created_at: "2026-11-01T10:00:00Z",
    updated_at: "2026-11-01T10:00:00Z",
    archived_at: null,
    ...over,
  };
}

function Resumed({
  sources,
  words = 701,
}: Readonly<{ sources: readonly CorpusSource[]; words?: number }>) {
  const client = useQueryClient();
  // Seeded before the step's first render, as the screen's restore leaves it.
  useState(() =>
    client.setQueryData<VoiceRestoreProbe>(VOICE_PROBE_KEY, {
      built: false,
      summary: summaryOf(words),
      sources,
    }),
  );
  const [state, dispatch] = useReducer(conversationReducer, collectingState());
  return <VoiceAct state={state} dispatch={dispatch} />;
}

describe("the Sources list on the voice step", () => {
  it("lists what the server already holds after a reload, leaving out an excluded sample", () => {
    stubApi({});
    render(
      <Resumed
        sources={[
          stored({ id: "s1", source_label: "offer.pdf" }),
          stored({ id: "s2", source_label: "notes.txt", word_count: 351 }),
          stored({ id: "s3", source_label: "colleague.txt", included: false }),
        ]}
      />,
    );

    expect(screen.getByText("offer.pdf")).toBeTruthy();
    expect(screen.getByText("notes.txt")).toBeTruthy();
    expect(screen.queryByText("colleague.txt")).toBeNull();
    expect(
      screen.getByRole("button", { name: "Remove offer.pdf" }),
    ).toBeTruthy();
  });

  it("removes a sample at its version and shows the meter without it", async () => {
    const calls = stubApi({ afterRemove: summaryOf(351) });
    const user = userEvent.setup();
    render(
      <Resumed
        sources={[
          stored({ id: "s1", source_label: "offer.pdf" }),
          stored({ id: "s2", source_label: "notes.txt", word_count: 351 }),
        ]}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Remove offer.pdf" }));

    await waitFor(() => expect(screen.queryByText("offer.pdf")).toBeNull());
    const [removal] = requestsTo(calls, "/sources/s1", "DELETE");
    expect(removal?.headers.get("If-Match")).toBe("4");
    expect(
      await screen.findByText("351 of 800 words", { exact: false }),
    ).toBeTruthy();
    expect(screen.getByText("notes.txt")).toBeTruthy();
  });

  // Until the server answers, the meter still counts the sample being removed.
  it("holds Build while a sample is being removed, and keeps the pressed Remove focusable", async () => {
    const removal = gate();
    const calls = stubApi({
      afterRemove: summaryOf(351),
      removeRelease: removal.held,
    });
    const user = userEvent.setup();
    render(
      <Resumed
        words={1052}
        sources={[
          stored({ id: "s1", source_label: "offer.pdf", word_count: 701 }),
          stored({ id: "s2", source_label: "notes.txt", word_count: 351 }),
        ]}
      />,
    );

    const pressed = screen.getByRole("button", { name: "Remove offer.pdf" });
    await user.click(pressed);
    await waitFor(() =>
      expect(pressed.getAttribute("aria-disabled")).toBe("true"),
    );
    expect(pressed).not.toHaveProperty("disabled", true);
    expect(
      screen.getByRole("button", { name: "Remove notes.txt" }),
    ).toHaveProperty("disabled", true);
    await user.click(
      screen.getByRole("button", { name: /Build my voice profile/ }),
    );
    expect(requestsTo(calls, "/builds", "POST")).toHaveLength(0);

    removal.open();
    await waitFor(() => expect(screen.queryByText("offer.pdf")).toBeNull());
  });

  it("drops the row once the server removed it, even when the meter cannot be read after", async () => {
    stubApi({ afterRemoveStatus: 500 });
    const user = userEvent.setup();
    render(
      <Resumed sources={[stored({ id: "s1", source_label: "offer.pdf" })]} />,
    );

    await user.click(screen.getByRole("button", { name: "Remove offer.pdf" }));

    await waitFor(() => expect(screen.queryByText("offer.pdf")).toBeNull());
    expect(screen.queryByText(/could not remove/i)).toBeNull();
  });

  // The meter still counts the removed sample until a read says otherwise.
  it("holds Build when the meter cannot be read after a removal", async () => {
    const calls = stubApi({ afterRemoveStatus: 500 });
    const user = userEvent.setup();
    render(
      <Resumed
        words={1052}
        sources={[
          stored({ id: "s1", source_label: "offer.pdf", word_count: 701 }),
          stored({ id: "s2", source_label: "notes.txt", word_count: 351 }),
        ]}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Remove offer.pdf" }));
    await waitFor(() => expect(screen.queryByText("offer.pdf")).toBeNull());
    await user.click(
      screen.getByRole("button", { name: /Build my voice profile/ }),
    );
    expect(requestsTo(calls, "/builds", "POST")).toHaveLength(0);
  });

  it("shows the word count as unavailable, not the old number, when the meter cannot be read", async () => {
    stubApi({ afterRemoveStatus: 500 });
    const user = userEvent.setup();
    render(
      <Resumed
        words={1052}
        sources={[
          stored({ id: "s1", source_label: "offer.pdf", word_count: 701 }),
          stored({ id: "s2", source_label: "notes.txt", word_count: 351 }),
        ]}
      />,
    );
    expect(screen.getAllByText(/1,052/).length).toBeGreaterThan(0);

    await user.click(screen.getByRole("button", { name: "Remove offer.pdf" }));
    expect(
      await screen.findByText(en["ob.conv.voice.meterUnknown"]),
    ).toBeTruthy();
    expect(screen.queryByText(/1,052/)).toBeNull();
  });

  it("names two pasted texts by their own opening words", async () => {
    stubApi({
      ingests: [
        { stats: documentStats, summary: summaryOf(900) },
        { stats: documentStats, summary: summaryOf(1800) },
      ],
    });
    const user = userEvent.setup();
    render(<Resumed sources={[]} />);

    for (const text of ["Hello from Zoe.", "Dear Sam, thanks."]) {
      await user.click(
        screen.getByRole("button", { name: en["ob.conv.voice.pasteInstead"] }),
      );
      await user.type(
        screen.getByLabelText(en["ob.conv.voice.composer"]),
        text,
      );
      await user.click(
        screen.getByRole("button", { name: en["ob.conv.voice.pasteAdd"] }),
      );
    }

    expect(
      await screen.findByText(
        "Pasted text – Dear Sam, thanks.",
        {},
        { timeout: INGEST_WAITER_MS },
      ),
    ).toBeTruthy();
    expect(screen.getByText("Pasted text – Hello from Zoe.")).toBeTruthy();
  });
});
