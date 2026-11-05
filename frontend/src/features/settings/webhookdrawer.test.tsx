/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { subscribableEventTypeValues } from "../../api/public-events";
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import { WebhooksCard } from "./webhooks";

// The subscription drawer, driven through the card the way an operator reaches
// it. The tests pin what a create and an edit send, how the picker and header
// rows read, and the template preview.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const SUBSCRIPTION = {
  id: "sub-1",
  owner_id: "user-1",
  target_url: "https://example.test/hooks/margince",
  event_types: ["deal.stage_changed"],
  state: "active",
  custom_headers: [{ name: "X-Api-Key", configured: true }],
  body_template: null,
  body_content_type: null,
  version: 3,
  created_at: "2026-07-01T00:00:00Z",
  updated_at: "2026-07-01T00:00:00Z",
  archived_at: null,
};

type Call = {
  method: string;
  path: string;
  ifMatch: string | null;
  body: unknown;
};

function backend(
  subscriptions: unknown[],
  respond: (call: Call) => Response | undefined = () => undefined,
) {
  const calls: Call[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      const path = new URL(req.url).pathname;
      if (path.endsWith("/v1/me")) {
        return jsonResponse(
          meFixture({
            allow: { webhook_subscription: ["create", "update", "delete"] },
          }),
        );
      }
      if (path.endsWith("/webhook-subscriptions") && req.method === "GET") {
        return jsonResponse({
          data: subscriptions,
          page: { next_cursor: null, has_more: false },
          delivery_enabled: true,
        });
      }
      const body = req.method === "GET" ? null : await req.clone().json();
      const call = {
        method: req.method,
        path,
        ifMatch: req.headers.get("If-Match"),
        body,
      };
      calls.push(call);
      const answer = respond(call);
      if (answer) {
        return answer;
      }
      if (path.endsWith("/body-preview")) {
        const { body_template: template } = body as { body_template: string };
        return jsonResponse(
          template === "{{ data | json }}"
            ? {
                body: '{"to_stage_name":"Sample"}',
                content_type: "text/plain",
                byte_length: 26,
              }
            : {
                body: '{"text":"rendered"}',
                content_type: "application/json",
                byte_length: 19,
              },
        );
      }
      if (req.method === "POST") {
        return jsonResponse(
          {
            subscription: { ...SUBSCRIPTION, id: "sub-new" },
            signing_secret: "whsec_abcDEF123==",
          },
          201,
        );
      }
      if (req.method === "PATCH") {
        return jsonResponse({ ...SUBSCRIPTION, version: 4 });
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  return calls;
}

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

async function chooseEvents(
  user: ReturnType<typeof userEvent.setup>,
  labels: string[],
) {
  await user.click(
    screen.getByRole("button", { name: en["webhooks.events.add"] }),
  );
  for (const label of labels) {
    await user.click(
      screen.getByRole("checkbox", { name: new RegExp(`^${label}`) }),
    );
  }
  await user.click(
    screen.getByRole("button", { name: en["webhooks.events.done"] }),
  );
}

describe("the webhook subscription drawer", () => {
  it("creates a subscription with its events and an Authorization header, and reveals the secret once", async () => {
    const user = userEvent.setup();
    const calls = backend([]);
    render(<WebhooksCard />);

    await user.click(await screen.findByTestId("new-webhook-subscription"));
    await user.type(
      screen.getByLabelText(new RegExp(en["webhooks.field.targetUrl"])),
      "https://example.test/inbound",
    );
    await chooseEvents(user, ["Deal moved to another stage", "Lead promoted"]);
    await user.click(
      screen.getByRole("button", {
        name: new RegExp(en["webhooks.headers.addAuthorization"]),
      }),
    );
    await user.type(
      screen.getByLabelText(en["webhooks.headers.value"]),
      "test-token-not-real",
    );
    await user.click(screen.getByTestId("save-webhook-subscription"));

    await waitFor(() =>
      expect(calls.some((c) => c.method === "POST")).toBe(true),
    );
    expect(calls.find((c) => c.method === "POST")?.body).toEqual({
      target_url: "https://example.test/inbound",
      event_types: ["deal.stage_changed", "lead.promoted"],
      custom_headers: [
        { name: "Authorization", value: "Bearer test-token-not-real" },
      ],
      body_template: null,
      body_content_type: null,
    });
    expect(await screen.findByText("whsec_abcDEF123==")).toBeTruthy();
  });

  it("offers every subscribable event, grouped, with what each one carries", async () => {
    const user = userEvent.setup();
    backend([]);
    render(<WebhooksCard />);

    await user.click(await screen.findByTestId("new-webhook-subscription"));
    await user.click(
      screen.getByRole("button", { name: en["webhooks.events.add"] }),
    );
    expect(screen.getAllByRole("checkbox")).toHaveLength(
      subscribableEventTypeValues.length,
    );

    const deals = screen.getByRole("region", {
      name: en["webhooks.eventGroup.deals"],
    });
    await user.click(
      within(deals).getByRole("button", {
        name: en["webhooks.events.selectAll"],
      }),
    );
    expect(
      within(deals)
        .getAllByRole("checkbox")
        .every((box) => (box as HTMLInputElement).checked),
    ).toBe(true);

    await user.click(
      screen.getByRole("button", {
        name: `About ${en["webhooks.event.deal.stage_changed.label"]}`,
      }),
    );
    expect(await screen.findByText("to_stage_name")).toBeTruthy();
    expect(
      screen.getByText(en["webhooks.event.deal.stage_changed.when"], {
        selector: "p",
      }),
    ).toBeTruthy();
  });

  it("edits under If-Match, keeping a sealed header by name", async () => {
    const user = userEvent.setup();
    const calls = backend([SUBSCRIPTION]);
    render(<WebhooksCard />);

    await user.click(await screen.findByTestId("edit-webhook-subscription"));
    expect(screen.getByText(en["webhooks.headers.valueSet"])).toBeTruthy();
    await pickOption(
      user,
      screen.getByRole("combobox", { name: /^State/ }),
      "Paused",
    );
    await user.click(screen.getByTestId("save-webhook-subscription"));

    await waitFor(() =>
      expect(calls.some((c) => c.method === "PATCH")).toBe(true),
    );
    const patch = calls.find((c) => c.method === "PATCH");
    expect(patch?.ifMatch).toBe("3");
    // The headers were left alone, so the edit names none and the server keeps them.
    expect(patch?.body).toEqual({
      state: "paused",
      event_types: ["deal.stage_changed"],
      body_template: null,
      body_content_type: null,
    });
  });

  it("sends a replaced value, keeps an abandoned Replace, and drops a removed header", async () => {
    const user = userEvent.setup();
    const calls = backend([
      {
        ...SUBSCRIPTION,
        custom_headers: [
          { name: "X-Api-Key", configured: true },
          { name: "X-Tenant", configured: true },
          { name: "X-Old", configured: true },
        ],
      },
    ]);
    render(<WebhooksCard />);

    await user.click(await screen.findByTestId("edit-webhook-subscription"));
    const replaces = screen.getAllByRole("button", {
      name: en["webhooks.headers.replace"],
    });
    await user.click(replaces[0]);
    await user.type(
      screen.getByLabelText(en["webhooks.headers.value"]),
      "new-key-not-real",
    );
    await user.click(replaces[1]);
    await user.click(
      screen.getByRole("button", { name: "Remove header X-Old" }),
    );
    await user.click(screen.getByTestId("save-webhook-subscription"));

    await waitFor(() =>
      expect(calls.some((c) => c.method === "PATCH")).toBe(true),
    );
    const patch = calls.find((c) => c.method === "PATCH");
    expect(
      (patch?.body as { custom_headers?: unknown })?.custom_headers,
    ).toEqual([
      { name: "X-Api-Key", value: "new-key-not-real" },
      { name: "X-Tenant" },
    ]);
  });

  it("says a refusal on a field the drawer has no place for at its foot", async () => {
    const user = userEvent.setup();
    backend([SUBSCRIPTION], (call) =>
      call.method === "PATCH"
        ? jsonResponse(
            {
              title: "Unprocessable Entity",
              code: "validation_error",
              details: {
                errors: [
                  {
                    field: "state",
                    code: "invalid",
                    message: "must be active or paused",
                  },
                ],
              },
            },
            422,
          )
        : undefined,
    );
    render(<WebhooksCard />);

    await user.click(await screen.findByTestId("edit-webhook-subscription"));
    await user.click(screen.getByTestId("save-webhook-subscription"));
    expect(await screen.findByText("must be active or paused")).toBeTruthy();
  });

  it("shows the server's refusal at the field it names", async () => {
    const user = userEvent.setup();
    backend([SUBSCRIPTION], (call) =>
      call.method === "PATCH"
        ? jsonResponse(
            {
              title: "Unprocessable Entity",
              code: "validation_error",
              details: {
                errors: [
                  {
                    field: "custom_headers",
                    code: "invalid",
                    message:
                      "webhook-signature is reserved: Margince sets webhook-signature",
                  },
                ],
              },
            },
            422,
          )
        : undefined,
    );
    render(<WebhooksCard />);

    await user.click(await screen.findByTestId("edit-webhook-subscription"));
    await user.click(screen.getByTestId("save-webhook-subscription"));
    expect(
      await screen.findByText(/webhook-signature is reserved/),
    ).toBeTruthy();
  });

  it("fills a custom template from a preset and previews it with the server's engine", async () => {
    const user = userEvent.setup();
    const calls = backend([SUBSCRIPTION]);
    render(<WebhooksCard />);

    await user.click(await screen.findByTestId("edit-webhook-subscription"));
    await pickOption(
      user,
      screen.getByRole("combobox", {
        name: new RegExp(en["webhooks.body.format"]),
      }),
      en["webhooks.body.custom"],
    );
    await pickOption(
      user,
      screen.getByRole("combobox", { name: en["webhooks.body.startFrom"] }),
      en["webhooks.body.preset.slack"],
    );
    expect(
      (
        screen.getByRole("textbox", {
          name: en["webhooks.body.template"],
        }) as HTMLTextAreaElement
      ).value,
    ).toContain('"blocks"');

    await user.click(
      screen.getByRole("button", { name: en["webhooks.body.tab.preview"] }),
    );
    expect(
      (await screen.findByTestId("webhook-preview-body")).textContent,
    ).toBe('{"text":"rendered"}');
    const preview = calls.find(
      (c) =>
        c.path.endsWith("/body-preview") &&
        (c.body as { body_template: string }).body_template.includes("blocks"),
    );
    expect(preview?.body).toMatchObject({
      event_type: "deal.stage_changed",
      body_content_type: "application/json",
    });
  });

  it("refuses to save without an event", async () => {
    const user = userEvent.setup();
    const calls = backend([]);
    render(<WebhooksCard />);

    await user.click(await screen.findByTestId("new-webhook-subscription"));
    await user.click(screen.getByTestId("save-webhook-subscription"));
    expect(calls.some((c) => c.method === "POST")).toBe(false);
    expect(screen.getByText(en["webhooks.events.required"])).toBeTruthy();
  });
});
