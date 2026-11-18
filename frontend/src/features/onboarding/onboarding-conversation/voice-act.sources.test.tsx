/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useReducer } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { en } from "../../../i18n/en";
import { conversationReducer } from "./conversation-machine";
import { VoiceAct } from "./voice-act";
import {
  collectingState,
  documentStats,
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

function Resumed({ sources }: Readonly<{ sources: readonly CorpusSource[] }>) {
  const [state, dispatch] = useReducer(conversationReducer, collectingState());
  return (
    <VoiceAct
      state={state}
      dispatch={dispatch}
      initialSummary={summaryOf(701)}
      initialSources={sources}
    />
  );
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
    render(
      <Resumed
        sources={[
          stored({ id: "s1", source_label: "offer.pdf" }),
          stored({ id: "s2", source_label: "notes.txt", word_count: 351 }),
        ]}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Remove offer.pdf" }),
    );

    await waitFor(() => expect(screen.queryByText("offer.pdf")).toBeNull());
    const [removal] = requestsTo(calls, "/sources/s1", "DELETE");
    expect(removal?.headers.get("If-Match")).toBe("4");
    expect(
      await screen.findByText("351 of 800 words", { exact: false }),
    ).toBeTruthy();
    expect(screen.getByText("notes.txt")).toBeTruthy();
  });

  it("names two pasted texts by their own opening words", async () => {
    stubApi({
      ingests: [
        { stats: documentStats, summary: summaryOf(900) },
        { stats: documentStats, summary: summaryOf(1800) },
      ],
    });
    render(<Resumed sources={[]} />);

    for (const text of ["Hello from Zoe.", "Dear Sam, thanks."]) {
      await userEvent.click(
        screen.getByRole("button", { name: en["ob.conv.voice.pasteInstead"] }),
      );
      await userEvent.type(
        screen.getByLabelText(en["ob.conv.voice.composer"]),
        text,
      );
      await userEvent.click(
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
