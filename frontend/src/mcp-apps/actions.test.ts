// @vitest-environment happy-dom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

type Sent = { msg: Record<string, unknown>; target: string };

const detach: Array<() => void> = [];
const HOST = "https://host.example";

async function load(parent: Window) {
  vi.resetModules();
  Object.defineProperty(window, "parent", {
    value: parent,
    configurable: true,
  });
  const added: Array<[string, EventListenerOrEventListenerObject]> = [];
  const attach = window.addEventListener.bind(window);
  const spy = vi
    .spyOn(window, "addEventListener")
    .mockImplementation((type, listener, options) => {
      added.push([type, listener]);
      attach(type, listener, options);
    });
  const actions = await import("./actions");
  spy.mockRestore();
  detach.push(() => {
    for (const [type, listener] of added)
      window.removeEventListener(type, listener);
  });
  return actions;
}

function stubParent(): { win: Window; sent: Sent[] } {
  const sent: Sent[] = [];
  const win = {
    postMessage: (msg: Record<string, unknown>, target: string) => {
      sent.push({ msg, target });
    },
  } as unknown as Window;
  return { win, sent };
}

function deliver(source: Window, origin: string, data: unknown) {
  window.dispatchEvent(
    new MessageEvent("message", {
      source: source as MessageEventSource,
      origin,
      data,
    }),
  );
}

function handshake(
  parent: { win: Window; sent: Sent[] },
  hostCapabilities: unknown,
) {
  deliver(parent.win, HOST, {
    jsonrpc: "2.0",
    id: parent.sent[0].msg.id,
    result: { hostContext: {}, hostCapabilities },
  });
}

const callsSent = (parent: { sent: Sent[] }) =>
  parent.sent.filter((s) => s.msg.method === "tools/call");

beforeEach(() => {
  document.body.replaceChildren();
});

afterEach(() => {
  for (const off of detach.splice(0)) off();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("a view asks its host to run a declared tool", () => {
  it("sends the call to the pinned host origin and resolves with the tool's own data", async () => {
    const parent = stubParent();
    const actions = await load(parent.win);
    actions.declareActions(["merge_records"]);
    handshake(parent, { serverTools: {} });

    const outcome = actions.callServerTool("merge_records", { source_id: "a" });
    const call = callsSent(parent)[0];
    expect(call.target).toBe(HOST);
    expect(call.msg.params).toEqual({
      name: "merge_records",
      arguments: { source_id: "a" },
    });

    deliver(parent.win, HOST, {
      jsonrpc: "2.0",
      id: call.msg.id,
      result: { structuredContent: { data: { merged: true }, warnings: [] } },
    });
    await expect(outcome).resolves.toEqual({
      ok: true,
      data: { merged: true },
      warnings: [],
    });
  });

  it("refuses a tool the view did not declare, without sending anything", async () => {
    const parent = stubParent();
    const actions = await load(parent.win);
    actions.declareActions(["merge_records"]);
    handshake(parent, { serverTools: {} });

    const outcome = await actions.callServerTool("delete_everything", {});
    expect(outcome.ok).toBe(false);
    expect(callsSent(parent)).toHaveLength(0);
  });

  it("does not call before the host has announced it proxies tools", async () => {
    const parent = stubParent();
    const actions = await load(parent.win);
    actions.declareActions(["merge_records"]);
    expect(actions.canCallTools()).toBe(false);
    handshake(parent, {});
    expect(actions.canCallTools()).toBe(false);

    const outcome = await actions.callServerTool("merge_records", {});
    expect(outcome.ok).toBe(false);
    expect(callsSent(parent)).toHaveLength(0);
  });

  it("reports the server's refusal text rather than a generic failure", async () => {
    const parent = stubParent();
    const actions = await load(parent.win);
    actions.declareActions(["merge_records"]);
    handshake(parent, { serverTools: {} });

    const outcome = actions.callServerTool("merge_records", {});
    deliver(parent.win, HOST, {
      jsonrpc: "2.0",
      id: callsSent(parent)[0].msg.id,
      result: {
        isError: true,
        content: [{ type: "text", text: "Both companies have projects." }],
      },
    });
    await expect(outcome).resolves.toEqual({
      ok: false,
      reason: "Both companies have projects.",
    });
  });

  it("reports a host error, and ignores a response whose id it never sent", async () => {
    const parent = stubParent();
    const actions = await load(parent.win);
    actions.declareActions(["merge_records"]);
    handshake(parent, { serverTools: {} });

    const outcome = actions.callServerTool("merge_records", {});
    deliver(parent.win, HOST, {
      jsonrpc: "2.0",
      id: 9999,
      result: { structuredContent: { data: 1 } },
    });
    deliver(parent.win, HOST, {
      jsonrpc: "2.0",
      id: callsSent(parent)[0].msg.id,
      error: { code: -32000, message: "Tool not permitted." },
    });
    await expect(outcome).resolves.toEqual({
      ok: false,
      reason: "Tool not permitted.",
    });
  });

  it("gives up when the host never answers", async () => {
    vi.useFakeTimers();
    const parent = stubParent();
    const actions = await load(parent.win);
    actions.declareActions(["merge_records"]);
    handshake(parent, { serverTools: {} });

    const outcome = actions.callServerTool("merge_records", {});
    await vi.advanceTimersByTimeAsync(actions.CALL_TIMEOUT_MS);
    await expect(outcome).resolves.toMatchObject({ ok: false });
  });

  it("ignores a response from anything but the embedding frame", async () => {
    const parent = stubParent();
    const actions = await load(parent.win);
    actions.declareActions(["merge_records"]);
    handshake(parent, { serverTools: {} });

    const outcome = actions.callServerTool("merge_records", {});
    const stranger = stubParent();
    deliver(stranger.win, HOST, {
      jsonrpc: "2.0",
      id: callsSent(parent)[0].msg.id,
      result: { structuredContent: { data: "forged" } },
    });
    vi.useFakeTimers();
    let settled = false;
    void outcome.then(() => {
      settled = true;
    });
    await vi.advanceTimersByTimeAsync(1);
    expect(settled).toBe(false);
  });

  it("asks the assistant in chat when the host will not proxy a call", async () => {
    const parent = stubParent();
    const actions = await load(parent.win);
    handshake(parent, {});

    actions.askAssistant("Merge Anna Meyer into the existing record.");
    const message = parent.sent.find((s) => s.msg.method === "ui/message");
    expect(message?.target).toBe(HOST);
    expect(message?.msg.params).toEqual({
      role: "user",
      content: {
        type: "text",
        text: "Merge Anna Meyer into the existing record.",
      },
    });
  });
});
