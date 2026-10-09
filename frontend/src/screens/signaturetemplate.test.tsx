/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { useToast } from "../design-system/toast";
import { jsonResponse, render } from "./settings.testkit";
import { SignatureSettingRow } from "./settingssignaturerow";
import { SignatureHtml } from "./signaturehtml";
import { SignatureTemplateCard } from "./signaturetemplatecard";

// A workspace signature template: an admin writes the layout once, and each
// member fills in their own title and phone.

type Call = { url: string; method: string; body: unknown };

const RENDERED = "<p><b>Ada Lovelace</b><br>Head of Sales</p>";
const LOGO_URL = "/v1/companies/c1/logo";

function backend(calls: Call[], templateActive: boolean) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = String(request ? request.url : input);
    const method = request ? request.method : (init?.method ?? "GET");
    const raw = request ? await request.text() : String(init?.body ?? "");
    calls.push({ url, method, body: raw ? JSON.parse(raw) : undefined });
    if (url.endsWith("/v1/me")) {
      return jsonResponse({
        ...meFixture({ allow: { installation_settings: ["read", "update"] } }),
        installation_brand: { display_name: "Company A", logo_url: LOGO_URL },
      });
    }
    if (url.endsWith(LOGO_URL)) {
      return new Response(new Blob(["logo"], { type: "image/png" }));
    }
    if (url.includes("/me/email-signature")) {
      return jsonResponse({
        body: "",
        title: "Head of Sales",
        phone: "",
        template_active: templateActive,
      });
    }
    if (url.includes("/emails:sign-off")) {
      return jsonResponse({
        text: "Ada Lovelace",
        html: RENDERED,
        kind: "template",
      });
    }
    if (url.includes("/email-signature-template")) {
      return jsonResponse({ template: "<p><b>{name}</b><br>{title}</p>" });
    }
    return jsonResponse({});
  });
}

function Row() {
  return <SignatureSettingRow toast={useToast()} />;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a member under a workspace template", () => {
  it("fills in title and phone instead of writing a sign-off, and sees it rendered", async () => {
    const user = userEvent.setup();
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(calls, true));
    render(<Row />);

    await user.click(
      await screen.findByRole("button", { name: "Edit signature" }),
    );
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).queryByRole("textbox", { name: "Sign-off" }),
    ).toBeNull();
    expect(await within(dialog).findByTitle("Signature preview")).toBeTruthy();
    await user.type(
      within(dialog).getByRole("textbox", { name: "Phone" }),
      "+49 30 1234",
    );
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(
        calls.some(
          (c) =>
            c.method === "PUT" &&
            c.url.endsWith("/me/email-signature") &&
            JSON.stringify(c.body) ===
              JSON.stringify({
                body: "",
                title: "Head of Sales",
                phone: "+49 30 1234",
              }),
        ),
      ).toBe(true),
    );
  });

  it("keeps the plain sign-off box when the workspace has no template", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend([], false));
    render(<Row />);
    await user.click(
      await screen.findByRole("button", { name: "Edit signature" }),
    );
    expect(
      await screen.findByRole("textbox", { name: "Sign-off" }),
    ).toBeTruthy();
  });
});

describe("the template card", () => {
  it("saves the admin's layout through one PUT", async () => {
    const user = userEvent.setup();
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(calls, true));
    render(<SignatureTemplateCard />);

    const box = await screen.findByRole("textbox", { name: "Template (HTML)" });
    await user.type(box, " Gradion");
    // The preview asks for the layout as typed, before it is saved.
    await waitFor(() =>
      expect(
        calls.some(
          (c) =>
            c.url.endsWith("/emails:sign-off") &&
            JSON.stringify(c.body).includes("{title}</p> Gradion"),
        ),
      ).toBe(true),
    );
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(
        calls.some(
          (c) =>
            c.method === "PUT" && c.url.endsWith("/email-signature-template"),
        ),
      ).toBe(true),
    );
  });
});

describe("a rendered signature", () => {
  it("shows the embedded logo from the workspace logo's bytes", async () => {
    vi.stubGlobal("fetch", backend([], true));
    render(
      <SignatureHtml
        html='<p><img src="cid:signature-logo@margince" alt="" width="150">Ada</p>'
        title="Signature preview"
      />,
    );
    const frame = await screen.findByTitle("Signature preview");
    await waitFor(() =>
      expect(frame.getAttribute("srcdoc")).toContain(
        'src="data:image/png;base64,',
      ),
    );
    expect(frame.getAttribute("srcdoc")).not.toContain("cid:");
  });
});
