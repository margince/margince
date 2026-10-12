import { readFile } from "node:fs/promises";
import { expect, test } from "@playwright/test";

// The MCP App views, driven in a real browser, asserting the one property no
// static analysis can reach: the built document makes NO network request.
//
// WHY THIS LANE EXISTS. The api's admission check is a substring ratchet and its
// own comments admit computed properties and aliases bypass it — `node['inner' +
// 'HTML']`, `window[fetchName](url)`. The build-time parsed validator judges the
// document's nodes, which is stronger, but neither can see a request that is
// only assembled at run time. A browser can.
//
// OBSERVED AT CONTEXT LEVEL, with service workers blocked. Page-level routing
// misses service-worker-handled traffic, popups and workers, which is precisely
// where a computed sink would hide.
//
// FIVE STATES, not one happy fixture. A document that fetches nothing while
// rendering a populated list may still fetch something on the empty state, on a
// warning, on a malformed payload, or when the host changes the theme.

const APPROVAL = {
  staged_action_id: "0195c3a0-0000-7000-8000-0000000000b1",
  kind: "advance_deal",
  status: "pending",
  summary: "Move Acme renewal to Negotiation",
  proposed_by: "agent:claude",
  proposed_change: { stage: "Negotiation" },
};

const HELD = {
  record_type: "contact",
  id: "0195c3a0-0000-7000-8000-000000000003",
  fields: { job_title: "Head of Sales" },
  staged_approval: {
    approval_id: "0195c3a0-0000-7000-8000-0000000000c1",
    fields: ["job_title"],
    replay: {
      record_type: "contact",
      id: "0195c3a0-0000-7000-8000-000000000003",
      fields: { job_title: "VP Sales" },
    },
  },
};

const VIEWS = [
  { file: "create-followups", populated: () => FILED },
  { file: "approval", populated: () => APPROVAL },
  { file: "field-conflict", populated: () => HELD },
] as const;

const FILED = {
  record_type: "contact",
  id: "0195c3a0-0000-7000-8000-000000000002",
  fields: { full_name: "Anna Meyer" },
  duplicate_candidates: [
    {
      candidate_id: "0195c3a0-0000-7000-8000-0000000000aa",
      other_record_id: "0195c3a0-0000-7000-8000-000000000001",
      confidence: 0.92,
      evidence: [
        {
          field: "full_name",
          left_value: "Anna Meyer",
          right_value: "Anna Meyer",
        },
      ],
    },
  ],
};

/** The five payload/host states each view is driven through. */
function states(populated: unknown) {
  const empty = { record_type: "contact", id: "x", fields: {}, approvals: [] };
  return [
    { name: "populated", data: populated, warnings: [], theme: "light" },
    { name: "empty", data: empty, warnings: [], theme: "light" },
    {
      name: "warned",
      data: populated,
      warnings: [{ code: "duplicate_filed_for_review" }],
      theme: "light",
    },
    { name: "malformed", data: "not an object", warnings: [], theme: "light" },
    { name: "theme change", data: populated, warnings: [], theme: "dark" },
  ];
}

for (const view of VIEWS) {
  test(`the ${view.file} document reaches no network in any state`, async ({
    browser,
  }) => {
    const html = await readFile(
      `dist/mcp-apps/${view.file}.html`,
      "utf8",
    ).catch(() => {
      throw new Error(
        `dist/mcp-apps/${view.file}.html is missing — this lane runs after \`pnpm build\`, ` +
          "and a silently skipped zero-request check looks exactly like a passing one",
      );
    });

    const context = await browser.newContext({ serviceWorkers: "block" });
    const requests: string[] = [];
    context.on("request", (r) => requests.push(r.url()));
    const page = await context.newPage();

    // The host: an about:blank page that frames the document under the same
    // sandbox a real host applies, and plays the protocol at it.
    await page.setContent(
      `<iframe id="view" sandbox="allow-scripts" style="width:600px;height:600px;border:0"></iframe>`,
    );
    const before = requests.length;

    for (const state of states(view.populated())) {
      await page.evaluate(
        async ({ html, state }) => {
          const frame = document.getElementById("view") as HTMLIFrameElement;
          const child = () => frame.contentWindow;
          const ready = new Promise<void>((resolve) => {
            const onMessage = (e: MessageEvent) => {
              if (e.source !== child()) return;
              const msg = e.data as { id?: unknown; method?: unknown };
              if (msg?.method === "ui/initialize") {
                // targetOrigin "*": under this sandbox the child's origin is the
                // string "null", which is not a usable postMessage target, so the
                // usual event.source.postMessage(res, e.origin) reply cannot work.
                child()?.postMessage(
                  {
                    jsonrpc: "2.0",
                    id: msg.id,
                    result: { hostContext: { theme: state.theme } },
                  },
                  "*",
                );
              }
              if (msg?.method === "ui/notifications/initialized") {
                child()?.postMessage(
                  {
                    jsonrpc: "2.0",
                    method: "ui/notifications/tool-result",
                    params: {
                      structuredContent: {
                        data: state.data,
                        warnings: state.warnings,
                      },
                    },
                  },
                  "*",
                );
                window.removeEventListener("message", onMessage);
                resolve();
              }
            };
            window.addEventListener("message", onMessage);
          });
          // srcdoc reloads the frame, so the handshake runs fresh per state.
          frame.srcdoc = html;
          await ready;
        },
        { html, state },
      );

      // The explicit quiescence point: the rendered content is stable for 500ms.
      // Asserting straight after the post would measure a document that had not
      // yet had the chance to make the request this test is looking for.
      let previous = "";
      await expect
        .poll(
          async () => {
            const now = await page
              .frameLocator("#view")
              .locator("body")
              .innerHTML();
            const stable = now !== "" && now === previous;
            previous = now;
            return stable;
          },
          { timeout: 5_000, intervals: [500, 500, 500, 500] },
        )
        .toBe(true);
    }

    // Only the frames the host itself created. The document is srcdoc, so it is
    // never itself a request — anything counted here is the view reaching out.
    const reached = requests
      .slice(before)
      .filter((url) => !url.startsWith("about:"));
    expect(
      reached,
      `the ${view.file} view made requests: ${reached.join(", ")}`,
    ).toEqual([]);
    await context.close();
  });
}

// A choice card acts through the HOST: the click becomes a tools/call the host
// proxies, never a request the view makes itself. This drives the real built
// card against a host that advertises serverTools and records what reaches it.
test("the duplicate card asks its host for exactly the tools it declares and reaches no network", async ({
  browser,
}) => {
  const html = await readFile(
    "dist/mcp-apps/create-followups.html",
    "utf8",
  ).catch(() => {
    throw new Error(
      "dist/mcp-apps/create-followups.html is missing — this lane runs after `pnpm build`",
    );
  });
  const context = await browser.newContext({ serviceWorkers: "block" });
  const requests: string[] = [];
  context.on("request", (r) => requests.push(r.url()));
  const page = await context.newPage();
  await page.setContent(
    `<iframe id="view" sandbox="allow-scripts" style="width:600px;height:600px;border:0"></iframe>`,
  );
  const before = requests.length;

  await page.evaluate(
    async ({ html, data }) => {
      const frame = document.getElementById("view") as HTMLIFrameElement;
      const child = () => frame.contentWindow;
      const w = window as unknown as {
        calls: { name: string; args: unknown }[];
      };
      w.calls = [];
      const ready = new Promise<void>((resolve) => {
        window.addEventListener("message", (e: MessageEvent) => {
          if (e.source !== child()) return;
          const msg = e.data as {
            id?: number;
            method?: string;
            params?: { name: string; arguments: unknown };
          };
          if (msg?.method === "ui/initialize") {
            child()?.postMessage(
              {
                jsonrpc: "2.0",
                id: msg.id,
                result: {
                  hostContext: {},
                  hostCapabilities: { serverTools: {} },
                },
              },
              "*",
            );
          } else if (msg?.method === "ui/notifications/initialized") {
            child()?.postMessage(
              {
                jsonrpc: "2.0",
                method: "ui/notifications/tool-result",
                params: { structuredContent: { data, warnings: [] } },
              },
              "*",
            );
            resolve();
          } else if (msg?.method === "tools/call") {
            w.calls.push({
              name: msg.params?.name ?? "",
              args: msg.params?.arguments,
            });
            child()?.postMessage(
              {
                jsonrpc: "2.0",
                id: msg.id,
                result: { structuredContent: { data: {}, warnings: [] } },
              },
              "*",
            );
          }
        });
      });
      frame.srcdoc = html;
      await ready;
    },
    { html, data: FILED },
  );

  const card = page.frameLocator("#view");
  await card
    .getByRole("button", { name: "Merge into the existing record" })
    .click();
  await expect(
    card.getByText("Merged into the record already on file."),
  ).toBeVisible();

  const calls = await page.evaluate(
    () => (window as unknown as { calls: { name: string }[] }).calls,
  );
  expect(calls).toEqual([
    {
      name: "merge_records",
      args: {
        record_type: "contact",
        source_id: "0195c3a0-0000-7000-8000-000000000002",
        target_id: "0195c3a0-0000-7000-8000-000000000001",
      },
    },
  ]);
  const reached = requests
    .slice(before)
    .filter((url) => !url.startsWith("about:"));
  expect(reached).toEqual([]);
  await context.close();
});
