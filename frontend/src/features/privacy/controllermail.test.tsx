// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import {
  ControllerMailDialog,
  type ControllerMailKind,
} from "./controllermail";
import {
  mailPreview,
  NO_ADDRESS_PROBLEM,
  NO_MAIL_LANE_PROBLEM,
  previewMarker,
  RELAY_FROM,
} from "./controllermail.fixtures";

type MailLanguage = components["schemas"]["MailLanguage"];
type Recorded = { method: string; path: string; search: string; body: unknown };

const LANGUAGES: readonly MailLanguage[] = ["de", "en", "vi"];

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const ISSUED = {
  delivered_to: "lena@kps.test",
  expires_at: "2026-11-27T09:00:00Z",
  queued: true,
  sendable: true,
};

// The preview route renders in the language the request names, and the server's
// default when it names none, as the contract says.
function stubServer(preview: (language: MailLanguage | null) => Response) {
  const requests: Recorded[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const text = request ? await request.text() : String(init?.body ?? "");
      requests.push({
        method,
        path: url.pathname.replace(/^\/v1/, ""),
        search: url.search,
        body: text ? JSON.parse(text) : null,
      });
      if (method === "GET") {
        const asked = url.searchParams.get("language");
        return preview(LANGUAGES.find((one) => one === asked) ?? null);
      }
      return jsonResponse(ISSUED, 201);
    }),
  );
  return requests;
}

function previewIn(kind: ControllerMailKind) {
  return (language: MailLanguage | null) =>
    jsonResponse(mailPreview(kind, language ?? "de"));
}

function renderDialog(kind: ControllerMailKind) {
  const onClose = vi.fn();
  const onSent = vi.fn();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ControllerMailDialog
          contactId="contact-1"
          kind={kind}
          open
          onClose={onClose}
          onSent={onSent}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { onClose, onSent };
}

const posts = (requests: Recorded[]) =>
  requests.filter((one) => one.method === "POST");

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the preview of a mail the installation sends for a rep", () => {
  it("shows sender, recipient, subject and text in the installation's language", async () => {
    const requests = stubServer(previewIn("record_confirmation"));
    renderDialog("record_confirmation");

    const dialog = await screen.findByRole("dialog");
    expect(
      await within(dialog).findByText("Ihre Kontaktdaten prüfen"),
    ).toBeVisible();
    expect(within(dialog).getByText(RELAY_FROM)).toBeVisible();
    expect(within(dialog).getByText("lena@kps.test")).toBeVisible();
    expect(dialog).toHaveTextContent(previewMarker("de"));
    expect(
      within(dialog).getByText(en["controllerMail.relayTitle"]),
    ).toBeVisible();
    expect(within(dialog).getByText(en["controllerMail.relay"])).toBeVisible();
    expect(
      within(dialog).getByText("The installation’s mail language is Deutsch."),
    ).toBeVisible();
    expect(
      within(dialog).getByRole("combobox", { name: "Language" }),
    ).toHaveTextContent("Deutsch");
    // The first read asks for the default rather than guessing one.
    expect(requests[0]?.path).toBe("/contacts/contact-1/consent/mail-preview");
    expect(requests[0]?.search).toBe("?kind=record_confirmation");
    expect(posts(requests)).toEqual([]);
  });

  it("reads the mail again in the language the rep picks", async () => {
    const requests = stubServer(previewIn("privacy_notice"));
    renderDialog("privacy_notice");
    const user = userEvent.setup();

    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByText(
      "Informationen zur Verarbeitung Ihrer personenbezogenen Daten",
    );
    await pickOption(
      user,
      within(dialog).getByRole("combobox", { name: "Language" }),
      "Tiếng Việt",
    );

    expect(
      await within(dialog).findByText(
        "Thông tin về việc xử lý dữ liệu cá nhân của quý khách",
      ),
    ).toBeVisible();
    expect(
      within(dialog).getByText(/Quý khách không cần trả lời/),
    ).toBeVisible();
    expect(
      within(dialog).queryByText(
        "Informationen zur Verarbeitung Ihrer personenbezogenen Daten",
      ),
    ).toBeNull();
    expect(requests.at(-1)?.search).toBe("?kind=privacy_notice&language=vi");
    expect(posts(requests)).toEqual([]);
  });

  it("sends in the language on screen, and only on the confirm", async () => {
    const requests = stubServer(previewIn("record_confirmation"));
    const { onSent } = renderDialog("record_confirmation");
    const user = userEvent.setup();

    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByText("Ihre Kontaktdaten prüfen");
    await pickOption(
      user,
      within(dialog).getByRole("combobox", { name: "Language" }),
      "English",
    );
    await within(dialog).findByText("Review your contact details");
    expect(posts(requests)).toEqual([]);

    await user.click(
      within(dialog).getByRole("button", {
        name: en["controllerMail.confirmSend"],
      }),
    );

    await waitFor(() => expect(onSent).toHaveBeenCalledWith(ISSUED));
    expect(posts(requests)).toEqual([
      {
        method: "POST",
        path: "/contacts/contact-1/consent/confirm-request",
        search: "",
        body: { language: "en" },
      },
    ]);
  });

  it("posts the privacy notice to its own route", async () => {
    const requests = stubServer(previewIn("privacy_notice"));
    const { onSent } = renderDialog("privacy_notice");
    const user = userEvent.setup();

    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByText(
      "Informationen zur Verarbeitung Ihrer personenbezogenen Daten",
    );
    await user.click(
      within(dialog).getByRole("button", { name: en["notice.sendConfirm"] }),
    );

    await waitFor(() => expect(onSent).toHaveBeenCalled());
    expect(posts(requests).map((one) => [one.path, one.body])).toEqual([
      ["/contacts/contact-1/consent/privacy-notice", { language: "de" }],
    ]);
  });

  it("sends nothing on Cancel", async () => {
    const requests = stubServer(previewIn("privacy_notice"));
    const { onClose, onSent } = renderDialog("privacy_notice");
    const user = userEvent.setup();

    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByText(RELAY_FROM);
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    expect(onClose).toHaveBeenCalled();
    expect(onSent).not.toHaveBeenCalled();
    expect(posts(requests)).toEqual([]);
  });

  it("sends nothing on Escape", async () => {
    const requests = stubServer(previewIn("record_confirmation"));
    const { onClose, onSent } = renderDialog("record_confirmation");
    const user = userEvent.setup();

    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByText(RELAY_FROM);
    await user.keyboard("{Escape}");

    expect(onClose).toHaveBeenCalled();
    expect(onSent).not.toHaveBeenCalled();
    expect(posts(requests)).toEqual([]);
  });

  it("says the relay names no sender when the installation names none", async () => {
    stubServer(() =>
      jsonResponse(
        mailPreview("record_confirmation", "de", { from_address: null }),
      ),
    );
    renderDialog("record_confirmation");

    const dialog = await screen.findByRole("dialog");
    expect(
      await within(dialog).findByText(en["controllerMail.noSender"]),
    ).toBeVisible();
  });

  it("shows a contact with no address as a refusal and offers no send", async () => {
    const requests = stubServer(() => jsonResponse(NO_ADDRESS_PROBLEM, 422));
    renderDialog("record_confirmation");

    const dialog = await screen.findByRole("dialog");
    expect(
      await within(dialog).findByText(/no live email address/),
    ).toBeVisible();
    expect(
      within(dialog).getByRole("button", {
        name: en["controllerMail.confirmSend"],
      }),
    ).toBeDisabled();
    expect(
      within(dialog).getByText(en["controllerMail.noPreview"]),
    ).toBeVisible();
    expect(
      within(dialog).queryByRole("button", { name: en["common.retry"] }),
    ).toBeNull();
    expect(posts(requests)).toEqual([]);
  });

  it("shows an installation that cannot send its mail as a refusal", async () => {
    const requests = stubServer(() => jsonResponse(NO_MAIL_LANE_PROBLEM, 409));
    renderDialog("privacy_notice");

    const dialog = await screen.findByRole("dialog");
    expect(
      await within(dialog).findByText(
        /cannot send its own mail yet; configure the relay, public URL and key vault/,
      ),
    ).toBeVisible();
    expect(
      within(dialog).getByRole("button", { name: en["notice.sendConfirm"] }),
    ).toBeDisabled();
    expect(posts(requests)).toEqual([]);
  });

  it("offers a retry after a server fault, and none after a refusal", async () => {
    let calls = 0;
    const requests = stubServer(() => {
      calls += 1;
      return calls === 1
        ? jsonResponse(
            { title: "Internal Server Error", status: 500, code: "internal" },
            500,
          )
        : jsonResponse(mailPreview("record_confirmation"));
    });
    renderDialog("record_confirmation");
    const user = userEvent.setup();

    const dialog = await screen.findByRole("dialog");
    await user.click(
      await within(dialog).findByRole("button", { name: en["common.retry"] }),
    );

    expect(
      await within(dialog).findByText("Ihre Kontaktdaten prüfen"),
    ).toBeVisible();
    expect(posts(requests)).toEqual([]);
  });
});
