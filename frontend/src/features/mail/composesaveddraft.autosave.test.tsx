/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { browserZone, UTC_ZONE } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { useKeptDraft } from "./composeautosave";
import { type SavedDraft, useSavedDraft } from "./composesaveddraft";
import type { SavedDraftFields } from "./composesaveddraftcalls";

// The saved draft keeping itself: what makes it unsaved, what a refused write
// does next, and what a page going away does to it.

type MailDraft = components["schemas"]["MailDraft"];

const PAUSE_MS = 1000;

const EMPTY: SavedDraftFields = {
  to: [],
  cc: [],
  bcc: [],
  subject: "",
  body: "",
  html: "",
  files: [],
  context: "",
  sendAt: "",
};

const typed = (body: string): SavedDraftFields => ({
  ...EMPTY,
  body,
  html: `<p>${body}</p>`,
});

const HELD: MailDraft = {
  id: "md-1",
  anchor_type: "contact",
  anchor_id: "p-1",
  to: [],
  cc: [],
  bcc: [],
  subject: "",
  body: "Half written",
  html_body: "<p>Half written</p>",
  attachments: [
    {
      id: "f-1",
      filename: "terms.pdf",
      content_type: "application/pdf",
      byte_size: 2048,
    },
  ],
  communication_context: "requested_followup",
  scheduled_at: "2026-11-20T08:00:00Z",
  scheduled_tz: UTC_ZONE,
  version: 3,
  agent_drafted: false,
  created_at: "2026-11-01T09:00:00Z",
  updated_at: "2026-11-01T09:05:00Z",
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function problem(code: string, status: number) {
  return new Response(JSON.stringify({ code, title: "Refused" }), {
    status,
    headers: { "Content-Type": "application/problem+json" },
  });
}

type Answer = (body: Record<string, unknown>) => Response | Promise<Response>;

// Every PUT the hook sends, answered in turn by `puts`; the last one repeats.
function stubDrafts(
  held: MailDraft | null | ((anchorId: string) => MailDraft | null),
  puts: Answer[],
) {
  const written: Record<string, unknown>[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      if (input.method === "GET") {
        const anchorId = new URL(input.url).searchParams.get("anchor_id") ?? "";
        const draft = typeof held === "function" ? held(anchorId) : held;
        return draft ? jsonResponse(draft) : problem("not_found", 404);
      }
      const body = (await input.clone().json()) as Record<string, unknown>;
      written.push(body);
      const answer = puts[Math.min(written.length, puts.length) - 1];
      if (!answer) throw new Error("no answer for this save");
      return answer(body);
    }),
  );
  return written;
}

const {
  attachments,
  communication_context,
  scheduled_at,
  scheduled_tz,
  ...BARE
} = HELD;

// The answer to a save of words alone: the draft as it was sent.
const savedAs = (body: Record<string, unknown>) =>
  jsonResponse({ ...BARE, ...body, version: 9 });
const refused = () => problem("internal", 503);

// The saved draft as the composer keeps it: the hook, and the keeping that
// needs the send's state beside it.
function useKeptComposer(
  input: Parameters<typeof useSavedDraft>[0] & { sending: boolean },
) {
  const draft = useSavedDraft(input);
  useKeptDraft(draft.keeper, input.sending);
  return draft;
}

// A rerender names `sending` only where a send is in flight.
type ComposerProps = Readonly<{
  fields: SavedDraftFields;
  sending?: boolean;
  entityId?: string;
  channel?: boolean;
}>;

function mount(initial: SavedDraftFields) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const onRestore = vi.fn();
  const onClose = vi.fn();
  const view = renderHook<SavedDraft, ComposerProps>(
    ({ fields, sending, entityId, channel }) =>
      useKeptComposer({
        where: {
          answering: undefined,
          entityType: "contact",
          entityId: entityId ?? "p-1",
          isChannelReply: channel === true,
        },
        open: true,
        fields,
        replyTo: undefined,
        offeredRecipient: undefined,
        onRestore,
        onClose,
        sending: sending === true,
      }),
    {
      initialProps: { fields: initial },
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>
          <LocaleProvider initial="en">{children}</LocaleProvider>
        </QueryClientProvider>
      ),
    },
  );
  return { ...view, onRestore, onClose };
}

// Past the pause, then through the save's answer, which lands on later turns.
async function pass(ms: number) {
  await act(() => vi.advanceTimersByTimeAsync(ms));
  for (let turn = 0; turn < 5; turn += 1) {
    await act(() => vi.advanceTimersByTimeAsync(0));
  }
}

function unloadAsked(): boolean {
  const event = new Event("beforeunload", { cancelable: true });
  globalThis.dispatchEvent(event);
  return event.defaultPrevented;
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("what the saved draft keeps", () => {
  it("writes a chosen file, with no words beside it", async () => {
    const written = stubDrafts(null, [savedAs]);
    const view = mount(EMPTY);
    await pass(0);

    view.rerender({
      fields: { ...EMPTY, files: [{ id: "f-1", filename: "terms.pdf" }] },
    });
    await pass(PAUSE_MS);

    expect(written).toEqual([
      {
        anchor_type: "contact",
        anchor_id: "p-1",
        to: [],
        cc: [],
        bcc: [],
        subject: "",
        body: "",
        html_body: null,
        attachment_ids: ["f-1"],
      },
    ]);
  });

  it("writes a reason for contact and a send time", async () => {
    const written = stubDrafts(null, [savedAs]);
    const view = mount(EMPTY);
    await pass(0);

    view.rerender({
      fields: {
        ...EMPTY,
        context: "requested_followup",
        sendAt: "2026-11-20T09:00",
      },
    });
    await pass(PAUSE_MS);

    expect(written[0]).toMatchObject({
      communication_context: "requested_followup",
      scheduled_at: new Date("2026-11-20T09:00").toISOString(),
      scheduled_tz: browserZone(),
    });
  });

  it("leaves out a cleared send time, so the saved one goes", async () => {
    const written = stubDrafts(HELD, [savedAs]);
    const view = mount(EMPTY);
    await pass(0);
    const restored = view.onRestore.mock.lastCall?.[0] as SavedDraftFields;
    expect(restored.files.map((file) => file.id)).toEqual(["f-1"]);
    expect(restored.context).toBe("requested_followup");
    expect(restored.sendAt).not.toBe("");
    view.rerender({ fields: restored });
    await pass(PAUSE_MS);
    expect(written).toHaveLength(0);

    view.rerender({ fields: { ...restored, sendAt: "" } });
    await pass(PAUSE_MS);

    expect(written).toHaveLength(1);
    expect(written[0]).toMatchObject({
      attachment_ids: ["f-1"],
      communication_context: "requested_followup",
    });
    expect(written[0]).not.toHaveProperty("scheduled_at");
    expect(written[0]).not.toHaveProperty("scheduled_tz");
  });
});

describe("a refused save", () => {
  it("is tried once, then again on the next edit", async () => {
    const written = stubDrafts(null, [refused, savedAs]);
    const view = mount(EMPTY);
    await pass(0);
    view.rerender({ fields: typed("Half") });
    await pass(PAUSE_MS);
    await vi.waitFor(() =>
      expect(view.result.current.error).toBe(
        "The draft was not saved. Retry, or keep writing to try again.",
      ),
    );

    await pass(PAUSE_MS * 10);
    expect(written).toHaveLength(1);

    view.rerender({ fields: typed("Half written") });
    await pass(PAUSE_MS);

    expect(written).toHaveLength(2);
    expect(written[1]).toMatchObject({ body: "Half written" });
    await vi.waitFor(() => expect(view.result.current.error).toBeNull());
  });

  it("is tried again when the connection comes back", async () => {
    const online = vi.spyOn(navigator, "onLine", "get").mockReturnValue(false);
    let puts = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request) => {
        if (input.method === "GET") return problem("not_found", 404);
        puts += 1;
        if (puts === 1) throw new TypeError("Failed to fetch");
        return savedAs({ body: "Half", html_body: "<p>Half</p>" });
      }),
    );
    const view = mount(EMPTY);
    await pass(0);
    view.rerender({ fields: typed("Half") });
    await pass(PAUSE_MS);
    await vi.waitFor(() => expect(view.result.current.retry).toBeDefined());

    online.mockReturnValue(true);
    act(() => {
      globalThis.dispatchEvent(new Event("online"));
    });
    await pass(0);

    expect(puts).toBe(2);
    await vi.waitFor(() => expect(view.result.current.kept).toBe(true));
    expect(view.result.current.error).toBeNull();
  });

  it("is written at once by Retry, without waiting for an edit", async () => {
    const written = stubDrafts(null, [refused, savedAs]);
    const view = mount(EMPTY);
    await pass(0);
    view.rerender({ fields: typed("Half") });
    await pass(PAUSE_MS);
    await vi.waitFor(() => expect(view.result.current.retry).toBeDefined());

    act(() => view.result.current.retry?.());
    await pass(0);

    expect(written).toHaveLength(2);
    await vi.waitFor(() => expect(view.result.current.kept).toBe(true));
    expect(view.onClose).not.toHaveBeenCalled();
  });

  it("is saved once more by a close before the close may discard", async () => {
    const written = stubDrafts(null, [refused]);
    const view = mount(EMPTY);
    await pass(0);
    view.rerender({ fields: typed("Half") });
    await pass(PAUSE_MS);
    await vi.waitFor(() => expect(view.result.current.retry).toBeDefined());

    act(() => view.result.current.requestClose());
    await pass(0);
    expect(written).toHaveLength(2);
    await vi.waitFor(() =>
      expect(view.result.current.error).toBe(
        "The draft was not saved. Retry, or close again to discard the text.",
      ),
    );
    expect(view.onClose).not.toHaveBeenCalled();

    act(() => view.result.current.requestClose());

    expect(view.onClose).toHaveBeenCalledTimes(1);
    expect(written).toHaveLength(2);
  });

  it("is not a conflict: a draft another window moved is never retried", async () => {
    const written = stubDrafts(HELD, [() => problem("version_skew", 409)]);
    const view = mount(EMPTY);
    await pass(0);
    const restored = view.onRestore.mock.lastCall?.[0] as SavedDraftFields;
    view.rerender({ fields: { ...restored, body: "Written here" } });
    await pass(PAUSE_MS);
    await vi.waitFor(() =>
      expect(view.result.current.changedElsewhere).toBe(true),
    );
    expect(view.result.current.error).toBeNull();
    expect(view.result.current.retry).toBeUndefined();

    view.rerender({ fields: { ...restored, body: "Written here, more" } });
    await pass(PAUSE_MS);
    act(() => {
      globalThis.dispatchEvent(new Event("online"));
    });
    await pass(0);

    expect(written).toHaveLength(1);
  });
});

describe("leaving the page", () => {
  it("asks first while words are unsaved, and not once they are kept", async () => {
    stubDrafts(null, [savedAs]);
    const view = mount(EMPTY);
    await pass(0);
    expect(unloadAsked()).toBe(false);

    view.rerender({ fields: typed("Half") });
    expect(unloadAsked()).toBe(true);

    await pass(PAUSE_MS);
    await vi.waitFor(() => expect(view.result.current.kept).toBe(true));
    expect(unloadAsked()).toBe(false);
  });

  it("asks first over a save that was refused", async () => {
    stubDrafts(null, [refused]);
    const view = mount(EMPTY);
    await pass(0);
    view.rerender({ fields: typed("Half") });
    await pass(PAUSE_MS);

    await vi.waitFor(() => expect(view.result.current.error).not.toBeNull());
    expect(unloadAsked()).toBe(true);
  });

  it("sends the words still waiting out the pause as the page goes", async () => {
    const written = stubDrafts(null, [savedAs]);
    const put = vi.spyOn(api, "PUT");
    const view = mount(EMPTY);
    await pass(0);
    view.rerender({ fields: typed("Half") });

    act(() => {
      globalThis.dispatchEvent(new Event("pagehide"));
    });
    await pass(0);

    expect(written).toEqual([expect.objectContaining({ body: "Half" })]);
    expect(put).toHaveBeenCalledWith(
      "/mail-drafts",
      expect.objectContaining({ keepalive: true }),
    );
    await pass(PAUSE_MS);
    expect(written).toHaveLength(1);
  });
});

describe("what counts as kept", () => {
  it("is an address in any case, as the server keeps it in lower case", async () => {
    stubDrafts(null, [
      (body) =>
        savedAs({
          ...body,
          to: (body.to as string[]).map((to) => to.toLowerCase()),
        }),
    ]);
    const view = mount(EMPTY);
    await pass(0);

    view.rerender({
      fields: { ...typed("Half"), to: ["Ada.Brandt@Firma.de"] },
    });
    await pass(PAUSE_MS);

    await vi.waitFor(() => expect(view.result.current.kept).toBe(true));
    expect(unloadAsked()).toBe(false);
  });

  it("is the files as sent, when the save left one out", async () => {
    const written = stubDrafts(null, [
      (body) =>
        savedAs({
          ...body,
          attachments: [HELD.attachments?.[0]],
          attachments_unavailable: 1,
        }),
    ]);
    const view = mount(EMPTY);
    await pass(0);

    view.rerender({
      fields: {
        ...typed("Half"),
        files: [
          { id: "f-1", filename: "terms.pdf" },
          { id: "f-2", filename: "empty.pdf" },
        ],
      },
    });
    await pass(PAUSE_MS);
    await vi.waitFor(() => expect(view.result.current.kept).toBe(true));
    await pass(PAUSE_MS * 5);

    expect(written).toHaveLength(1);
    expect(unloadAsked()).toBe(false);
  });
});

describe("a send in flight", () => {
  it("holds the autosave, and the page-hide save, until it is over", async () => {
    const written = stubDrafts(null, [savedAs]);
    const view = mount(EMPTY);
    await pass(0);
    view.rerender({ fields: typed("Half") });
    await pass(PAUSE_MS / 2);

    view.rerender({ fields: typed("Half"), sending: true });
    await pass(PAUSE_MS);
    act(() => {
      globalThis.dispatchEvent(new Event("pagehide"));
    });
    await pass(0);
    expect(written).toHaveLength(0);

    view.rerender({ fields: typed("Half"), sending: false });
    await pass(PAUSE_MS);
    expect(written).toHaveLength(1);
  });
});

describe("the page going away", () => {
  it("sends a draft too large for keepalive as an ordinary request", async () => {
    stubDrafts(null, [savedAs]);
    const put = vi.spyOn(api, "PUT");
    const view = mount(EMPTY);
    await pass(0);
    view.rerender({ fields: typed("x".repeat(70_000)) });

    act(() => {
      globalThis.dispatchEvent(new Event("pagehide"));
    });
    await pass(0);

    expect(put).toHaveBeenCalledWith(
      "/mail-drafts",
      expect.objectContaining({ keepalive: false }),
    );
  });
});

describe("the connection coming back", () => {
  it("does not retry a save the server refused as a request", async () => {
    const written = stubDrafts(null, [() => problem("validation_error", 422)]);
    const view = mount(EMPTY);
    await pass(0);
    view.rerender({ fields: typed("Half") });
    await pass(PAUSE_MS);
    await vi.waitFor(() => expect(view.result.current.retry).toBeDefined());

    act(() => {
      globalThis.dispatchEvent(new Event("offline"));
      globalThis.dispatchEvent(new Event("online"));
    });
    await pass(0);

    expect(written).toHaveLength(1);
  });
});

describe("the answer to a save", () => {
  it("that was lost does not read as another window's change", async () => {
    let reads = 0;
    const written = stubDrafts(() => {
      reads += 1;
      return reads === 1
        ? null
        : { ...BARE, body: "Half", html_body: "<p>Half</p>", version: 9 };
    }, [() => problem("version_skew", 409)]);
    const view = mount(EMPTY);
    await pass(0);
    view.rerender({ fields: typed("Half") });
    await pass(PAUSE_MS);

    await vi.waitFor(() => expect(view.result.current.kept).toBe(true));
    expect(view.result.current.changedElsewhere).toBe(false);
    expect(view.result.current.error).toBeNull();
    expect(written).toHaveLength(1);
  });

  it("from the server, a 5xx included, waits for an edit and not the connection", async () => {
    const written = stubDrafts(null, [refused]);
    const view = mount(EMPTY);
    await pass(0);
    view.rerender({ fields: typed("Half") });
    await pass(PAUSE_MS);
    await vi.waitFor(() => expect(view.result.current.retry).toBeDefined());

    act(() => {
      globalThis.dispatchEvent(new Event("offline"));
      globalThis.dispatchEvent(new Event("online"));
    });
    await pass(0);

    expect(written).toHaveLength(1);
  });
});

describe("a second anchor", () => {
  it("is measured against its own draft, not the last save at the first", async () => {
    const other: MailDraft = {
      ...BARE,
      id: "md-2",
      anchor_id: "p-2",
      body: "Other words",
      html_body: "<p>Other words</p>",
      version: 9,
    };
    const written = stubDrafts(
      (anchorId) => (anchorId === "p-2" ? other : null),
      [savedAs],
    );
    const view = mount(EMPTY);
    await pass(0);
    view.rerender({ fields: typed("Half") });
    await pass(PAUSE_MS);
    await vi.waitFor(() => expect(view.result.current.kept).toBe(true));

    view.rerender({ fields: typed("Half"), entityId: "p-2" });
    await pass(0);
    view.rerender({ fields: typed("Other words"), entityId: "p-2" });
    await pass(PAUSE_MS);

    await vi.waitFor(() => expect(view.result.current.kept).toBe(true));
    expect(written).toHaveLength(1);
  });

  it("gets the same words saved again, as words never tried there", async () => {
    const written = stubDrafts(null, [savedAs]);
    const view = mount(EMPTY);
    await pass(0);
    view.rerender({ fields: typed("Half") });
    await pass(PAUSE_MS);
    await vi.waitFor(() => expect(written).toHaveLength(1));

    view.rerender({ fields: typed("Half"), entityId: "p-2" });
    await pass(PAUSE_MS);

    expect(written).toHaveLength(2);
    expect(written[1]).toMatchObject({ anchor_id: "p-2", body: "Half" });
  });
});

describe("words not yet kept", () => {
  it("are sent as the page goes while the saved draft is still being read", async () => {
    const written: Record<string, unknown>[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request) => {
        if (input.method === "GET") return new Promise<Response>(() => {});
        written.push((await input.clone().json()) as Record<string, unknown>);
        return savedAs({});
      }),
    );
    const view = mount(EMPTY);
    await pass(0);
    view.rerender({ fields: typed("Half") });

    act(() => {
      globalThis.dispatchEvent(new Event("pagehide"));
    });
    await pass(0);

    expect(written).toEqual([expect.objectContaining({ body: "Half" })]);
  });

  it("ask first on leaving a composer that keeps no draft", async () => {
    stubDrafts(null, [savedAs]);
    const view = mount(EMPTY);
    view.rerender({ fields: typed("Half"), channel: true });

    expect(unloadAsked()).toBe(true);
  });
});

describe("a close after a refused close", () => {
  it("saves words written since the refusal instead of discarding them", async () => {
    const written = stubDrafts(null, [refused, savedAs]);
    const view = mount(EMPTY);
    await pass(0);
    view.rerender({ fields: typed("Half") });
    act(() => view.result.current.requestClose());
    await pass(0);
    await vi.waitFor(() =>
      expect(view.result.current.error).toBe(
        "The draft was not saved. Retry, or close again to discard the text.",
      ),
    );

    view.rerender({ fields: typed("Half written") });
    act(() => view.result.current.requestClose());
    await pass(0);

    expect(written).toHaveLength(2);
    expect(written[1]).toMatchObject({ body: "Half written" });
    await vi.waitFor(() => expect(view.onClose).toHaveBeenCalledTimes(1));
    expect(view.onRestore).not.toHaveBeenCalledWith(
      expect.objectContaining({ body: "" }),
    );
  });
});

describe("a close pressed while a quiet save is out", () => {
  // The quiet save answers only when the test lets it.
  function heldSave(answer: () => Response) {
    let release: () => void = () => {};
    const written = stubDrafts(null, [
      () =>
        new Promise<Response>((resolve) => {
          release = () => resolve(answer());
        }),
      refused,
    ]);
    return { written, release: () => release() };
  }

  it("closes once that save lands", async () => {
    const out = heldSave(() =>
      savedAs({ body: "Half", html_body: "<p>Half</p>" }),
    );
    const view = mount(EMPTY);
    await pass(0);
    view.rerender({ fields: typed("Half") });
    await pass(PAUSE_MS);
    await vi.waitFor(() => expect(view.result.current.saving).toBe(true));

    act(() => view.result.current.requestClose());
    expect(view.onClose).not.toHaveBeenCalled();
    act(() => out.release());
    await pass(0);

    await vi.waitFor(() => expect(view.onClose).toHaveBeenCalledTimes(1));
    expect(out.written).toHaveLength(1);
  });

  it("tries once more as a close when that save is refused, and stays open", async () => {
    const out = heldSave(refused);
    const view = mount(EMPTY);
    await pass(0);
    view.rerender({ fields: typed("Half") });
    await pass(PAUSE_MS);

    act(() => view.result.current.requestClose());
    act(() => out.release());
    await pass(0);

    await vi.waitFor(() =>
      expect(view.result.current.error).toBe(
        "The draft was not saved. Retry, or close again to discard the text.",
      ),
    );
    expect(out.written).toHaveLength(2);
    expect(view.onClose).not.toHaveBeenCalled();
  });
});
