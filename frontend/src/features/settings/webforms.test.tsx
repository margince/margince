/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CustomField } from "../../components/customfields.form";
import { LocaleProvider } from "../../i18n";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { draftFor, webFormBodyOf } from "./webformdrawer";
import type { WebForm } from "./webformkinds";
import { WebFormsCard } from "./webforms";
import { browserSnippet, intakeUrlOf, serverSnippet } from "./webformsnippet";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const OPERATOR: GrantSpec = { integrations: ["create", "update", "delete"] };

const contactForm: WebForm = {
  id: "wf-1",
  name: "Contact page",
  form_key: "abcdefghijklmnopqrstuvwxyz012345",
  sender: "browser",
  allowed_origins: ["https://www.acme.test"],
  lead_origin: "website",
  owner_id: null,
  field_map: {},
  consent_required: false,
  consent_policy_version: null,
  consent_wording: null,
  redirect_url: null,
  success_message: null,
  state: "active",
  secret_configured: false,
  version: 3,
  created_at: "2026-09-01T09:00:00Z",
  updated_at: "2026-09-01T09:00:00Z",
  archived_at: null,
};

const budgetField: CustomField = {
  id: "cf-1",
  object: "lead",
  label: "Budget",
  slug: "budget",
  type: "number",
  status: "active",
  column_name: "cf_budget",
  show_in_create: true,
  reportable: false,
  created_by: "u1",
  created_at: "2026-09-01T09:00:00Z",
  updated_at: "2026-09-01T09:00:00Z",
  version: 1,
};

type Sent = { method: string; url: string; body: unknown; ifMatch: string };

// The backend the card talks to: the list, plus whatever a case writes. Every
// write is recorded so a case can assert what was sent.
function backendFor(
  allow: GrantSpec,
  forms: WebForm[],
  writes: Record<string, () => Response> = {},
) {
  const sent: Sent[] = [];
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req =
      input instanceof Request ? input : new Request(String(input), init);
    if (req.url.endsWith("/v1/me")) {
      return jsonResponse(meFixture({ allow }));
    }
    if (req.method === "GET" && req.url.includes("/v1/web-forms")) {
      return jsonResponse({ data: forms, secrets_available: true });
    }
    if (req.method === "GET" && req.url.includes("/v1/custom-fields")) {
      return jsonResponse({ data: [budgetField] });
    }
    const route = `${req.method} ${new URL(req.url).pathname}`;
    const answer = writes[route];
    if (!answer) {
      throw new Error(`unexpected request: ${route}`);
    }
    const text = await req.text();
    sent.push({
      method: req.method,
      url: req.url,
      body: text ? JSON.parse(text) : null,
      ifMatch: req.headers.get("If-Match") ?? "",
    });
    return answer();
  });
  return { fetch, sent };
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

describe("WebFormsCard", () => {
  it("lists each form with its state and sender", async () => {
    vi.stubGlobal("fetch", backendFor(OPERATOR, [contactForm]).fetch);
    render(<WebFormsCard />);
    expect(await screen.findByText("Contact page")).toBeTruthy();
    expect(screen.getByText("https://www.acme.test")).toBeTruthy();
    expect(screen.getByText("Active")).toBeTruthy();
    expect(screen.getByText("Website")).toBeTruthy();
  });

  it("offers no write control to a role without the integrations grants", async () => {
    vi.stubGlobal("fetch", backendFor({}, [contactForm]).fetch);
    render(<WebFormsCard />);
    expect(await screen.findByText(/Read-only/)).toBeTruthy();
    expect(screen.queryByTestId("new-web-form")).toBeNull();
    expect(screen.queryByTestId("edit-web-form")).toBeNull();
  });

  it("creates a browser form and shows where to post", async () => {
    const created = { ...contactForm, name: "Demo request" };
    const backend = backendFor(OPERATOR, [], {
      "POST /v1/web-forms": () => jsonResponse(created, 201),
    });
    vi.stubGlobal("fetch", backend.fetch);
    const user = userEvent.setup();
    render(<WebFormsCard />);
    await user.click(await screen.findByTestId("new-web-form"));
    await user.type(screen.getByLabelText(/^Name/), "Demo request");
    await user.type(
      screen.getByLabelText(/^Websites/),
      "https://www.acme.test\nhttps://acme.test",
    );
    await user.click(screen.getByTestId("save-web-form"));

    await waitFor(() => expect(backend.sent).toHaveLength(1));
    expect(backend.sent[0]?.body).toMatchObject({
      name: "Demo request",
      sender: "browser",
      allowed_origins: ["https://www.acme.test", "https://acme.test"],
    });
    const address = await screen.findByTestId("web-form-address");
    expect(address.textContent).toContain(
      `/v1/public/forms/${contactForm.form_key}`,
    );
    expect(screen.getByTestId("web-form-snippet").textContent).toContain(
      'name="_mrg_hp"',
    );
    expect(
      (await screen.findByTestId("web-form-custom-fields")).textContent,
    ).toContain("cf_budget (Budget)");
  });

  it("shows a server form's minted secret once, with the caution", async () => {
    const created = {
      ...contactForm,
      sender: "server",
      allowed_origins: [],
      signing_secret: "mwf_minted",
    };
    const backend = backendFor(OPERATOR, [], {
      "POST /v1/web-forms": () => jsonResponse(created, 201),
    });
    vi.stubGlobal("fetch", backend.fetch);
    const user = userEvent.setup();
    render(<WebFormsCard />);
    await user.click(await screen.findByTestId("new-web-form"));
    await user.type(screen.getByLabelText(/^Name/), "Demo request");
    await user.click(screen.getByRole("combobox", { name: /Sent by/ }));
    await user.click(
      await screen.findByRole("option", { name: "Website server" }),
    );
    await user.click(screen.getByTestId("save-web-form"));

    expect((await screen.findByTestId("web-form-secret")).textContent).toBe(
      "mwf_minted",
    );
    expect(backend.sent[0]?.body).not.toHaveProperty("allowed_origins");
    expect(screen.getByText(/destroys the only copy/)).toBeTruthy();
  });

  it("pauses a form against the version it was read at", async () => {
    const backend = backendFor(OPERATOR, [contactForm], {
      "PATCH /v1/web-forms/wf-1": () =>
        jsonResponse({ ...contactForm, state: "paused", version: 4 }),
    });
    vi.stubGlobal("fetch", backend.fetch);
    const user = userEvent.setup();
    render(<WebFormsCard />);
    await user.click(
      await screen.findByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByTestId("toggle-web-form"));

    await waitFor(() => expect(backend.sent).toHaveLength(1));
    expect(backend.sent[0]).toMatchObject({
      body: { state: "paused" },
      ifMatch: "3",
    });
  });
});

describe("WebFormsCard actions", () => {
  const serverForm: WebForm = {
    ...contactForm,
    sender: "server",
    allowed_origins: [],
    secret_configured: true,
  };

  it("rotates a server form's secret only after confirming, and shows the new one", async () => {
    const backend = backendFor(OPERATOR, [serverForm], {
      "POST /v1/web-forms/wf-1/rotate-secret": () =>
        jsonResponse({ ...serverForm, signing_secret: "mwf_rotated" }),
    });
    vi.stubGlobal("fetch", backend.fetch);
    const user = userEvent.setup();
    render(<WebFormsCard />);
    await user.click(
      await screen.findByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByTestId("rotate-web-form-secret"));
    expect(backend.sent).toHaveLength(0);
    const confirm = await screen.findAllByRole("button", {
      name: "Rotate secret",
    });
    await user.click(confirm[confirm.length - 1] as HTMLElement);

    expect((await screen.findByTestId("web-form-secret")).textContent).toBe(
      "mwf_rotated",
    );
    expect(backend.sent[0]?.url).toContain("/rotate-secret");
  });

  it("edits a form's consent against the version it was read at", async () => {
    const backend = backendFor(OPERATOR, [contactForm], {
      "PATCH /v1/web-forms/wf-1": () =>
        jsonResponse({ ...contactForm, consent_required: true, version: 4 }),
    });
    vi.stubGlobal("fetch", backend.fetch);
    const user = userEvent.setup();
    render(<WebFormsCard />);
    await user.click(await screen.findByTestId("edit-web-form"));
    await user.click(screen.getByLabelText(/Require consent/));
    await user.type(
      screen.getByLabelText(/^Consent wording/),
      "You may answer me.",
    );
    await user.type(
      screen.getByLabelText(/^Privacy policy version/),
      "2026-10",
    );
    await user.click(screen.getByTestId("save-web-form"));

    await waitFor(() => expect(backend.sent).toHaveLength(1));
    expect(backend.sent[0]).toMatchObject({
      method: "PATCH",
      ifMatch: "3",
      body: {
        consent_required: true,
        consent_wording: "You may answer me.",
        consent_policy_version: "2026-10",
      },
    });
    expect(backend.sent[0]?.body).not.toHaveProperty("sender");
  });

  it("puts a refused setting under its own field until it is corrected", async () => {
    const backend = backendFor(OPERATOR, [contactForm], {
      "PATCH /v1/web-forms/wf-1": () =>
        jsonResponse(
          {
            type: "about:blank",
            title: "Unprocessable",
            status: 422,
            code: "validation_error",
            details: {
              errors: [
                {
                  field: "redirect_url",
                  code: "invalid",
                  message: "must be an https address",
                },
              ],
            },
          },
          422,
        ),
    });
    vi.stubGlobal("fetch", backend.fetch);
    const user = userEvent.setup();
    render(<WebFormsCard />);
    await user.click(await screen.findByTestId("edit-web-form"));
    await user.type(screen.getByLabelText(/^Thank-you page/), "x");
    await user.click(screen.getByTestId("save-web-form"));

    expect(await screen.findByText("must be an https address")).toBeTruthy();
    await user.type(screen.getByLabelText(/^Thank-you page/), "y");
    expect(screen.queryByText("must be an https address")).toBeNull();
  });
});

describe("web form settings", () => {
  it("keeps a builder's stored secret when the field is left empty", () => {
    const builder = { ...contactForm, sender: "typeform" as const };
    expect(webFormBodyOf(draftFor(builder))).not.toHaveProperty(
      "sender_secret",
    );
    expect(
      webFormBodyOf({ ...draftFor(builder), senderSecret: " s3cret " }),
    ).toMatchObject({ sender_secret: "s3cret" });
  });

  it("clears an emptied optional setting instead of storing blank text", () => {
    const body = webFormBodyOf({
      ...draftFor({ ...contactForm, redirect_url: "https://acme.test/x" }),
      redirectUrl: "  ",
    });
    expect(body.redirect_url).toBeNull();
  });
});

describe("web form snippets", () => {
  const url = intakeUrlOf(contactForm, "https://crm.acme.test");

  it("posts to the form's public address", () => {
    expect(url).toBe(
      `https://crm.acme.test/v1/public/forms/${contactForm.form_key}`,
    );
  });

  it("escapes the consent wording it writes into the page", () => {
    const html = browserSnippet(
      {
        ...contactForm,
        consent_required: true,
        consent_wording: 'I agree <script>"x"</script>',
      },
      url,
    );
    expect(html).toContain(
      "I agree &lt;script&gt;&quot;x&quot;&lt;/script&gt;",
    );
    expect(html).toContain('name="consent" required');
  });

  it("signs the same material the server verifies", () => {
    const code = serverSnippet({ ...contactForm, sender: "server" }, url);
    expect(code).toContain(
      `margince-inbound-v1\\nweb-form\\n${contactForm.form_key}\\n\${timestamp}\\n\${nonce}\\n\${body}`,
    );
    expect(code).toContain("process.env.MARGINCE_FORM_SECRET");
    expect(code).not.toContain("mwf_");
  });
});
