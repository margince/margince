// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { translate } from "../../i18n";
import { signatureReviewHref } from "../../lib/signaturereviewlink";
import { meFixture } from "../../testing/mefixture";
import {
  jsonResponse,
  render,
  renderSettings,
} from "../../testing/settings.testkit";
import { steppedClock } from "../../testing/steppedclock";
import { RetainedSignaturePanel } from "./retainedsignature";
import { SettingsSavePage } from "./settingssavepage";

const ID = "00000000-0000-4000-8000-0000000000a1";
const OWNER = "00000000-0000-4000-8000-0000000000b1";
const RAW = '<p>Saved HTML</p><img src="https://tracker.test/pixel">';
const original = (): components["schemas"]["RetainedEmailSignature"] => ({
  id: ID,
  owner_id: OWNER,
  owner_name: "Former colleague",
  version: 7,
  signature: {
    body: "Original plain text",
    html: RAW,
    title: "Original title",
    phone: "+49 555",
    template_active: false,
  },
  preview_html: "<p>Saved HTML</p>",
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

it("shows every retained field and previews only the saved sanitized markup", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      if (!(input instanceof Request))
        throw new Error("Expected a generated-client request");
      const path = new URL(input.url).pathname;
      if (path.endsWith("/me")) return jsonResponse(meFixture({}));
      if (path.endsWith(`/privacy/retained-signatures/${ID}`))
        return jsonResponse(original());
      throw new Error(`Unexpected request ${path}`);
    }),
  );
  render(
    <SettingsSavePage>
      <RetainedSignaturePanel id={ID} />
    </SettingsSavePage>,
  );
  expect(
    await screen.findByRole("textbox", {
      name: translate("en", "settings.signatureLabel"),
    }),
  ).toHaveValue("Original plain text");
  expect(
    screen.getByRole("textbox", {
      name: translate("en", "settings.signatureHtmlLabel"),
    }),
  ).toHaveValue(RAW);
  expect(
    screen.getByRole("textbox", {
      name: translate("en", "settings.signatureTitle"),
    }),
  ).toHaveValue("Original title");
  expect(
    screen.getByRole("textbox", {
      name: translate("en", "settings.signaturePhone"),
    }),
  ).toHaveValue("+49 555");
  const frame = screen.getByTitle("Saved signature preview");
  expect(frame.getAttribute("srcdoc")).toContain("Saved HTML");
  expect(frame.getAttribute("srcdoc")).not.toContain("tracker.test");
  expect(
    screen.queryByRole("button", { name: "Save" }),
  ).not.toBeInTheDocument();
});

it("keeps the original until the page saves the exact signature version", async () => {
  const user = steppedClock();
  let saved = original();
  const writes: components["schemas"]["SaveRetainedEmailSignatureRequest"][] =
    [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      if (!(input instanceof Request))
        throw new Error("Expected a generated-client request");
      const path = new URL(input.url).pathname;
      if (path.endsWith("/me")) return jsonResponse(meFixture({}));
      if (path.endsWith(`/privacy/retained-signatures/${ID}`)) {
        if (input.method === "PUT") {
          const body: components["schemas"]["SaveRetainedEmailSignatureRequest"] =
            await input.clone().json();
          writes.push(body);
          saved = {
            ...saved,
            version: 8,
            signature: {
              ...saved.signature,
              body: body.body,
              html: body.html ?? saved.signature.html,
              title: body.title ?? saved.signature.title,
              phone: body.phone ?? saved.signature.phone,
            },
          };
        }
        return jsonResponse(saved);
      }
      throw new Error(`Unexpected request ${path}`);
    }),
  );
  render(
    <SettingsSavePage>
      <RetainedSignaturePanel id={ID} />
    </SettingsSavePage>,
  );
  const text = await screen.findByRole("textbox", {
    name: translate("en", "settings.signatureLabel"),
  });
  await user.clear(text);
  await user.type(text, "Reviewed plain text");
  expect(writes).toHaveLength(0);
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() =>
    expect(writes).toEqual([
      {
        body: "Reviewed plain text",
        html: RAW,
        title: "Original title",
        phone: "+49 555",
        expected_version: 7,
      },
    ]),
  );
  await waitFor(() =>
    expect(
      screen.queryByRole("button", { name: "Save" }),
    ).not.toBeInTheDocument(),
  );
  expect(
    screen.getByRole("textbox", {
      name: translate("en", "settings.signatureLabel"),
    }),
  ).toHaveValue("Reviewed plain text");
});

it("reviews a conflicting saved version without losing the draft, then saves that version", async () => {
  const user = steppedClock();
  let saved = original();
  const versions: number[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      if (!(input instanceof Request))
        throw new Error("Expected a generated-client request");
      const path = new URL(input.url).pathname;
      if (path.endsWith("/me")) return jsonResponse(meFixture({}));
      if (path.endsWith(`/privacy/retained-signatures/${ID}`)) {
        if (input.method === "PUT") {
          const body: components["schemas"]["SaveRetainedEmailSignatureRequest"] =
            await input.clone().json();
          versions.push(body.expected_version);
          if (versions.length === 1)
            saved = {
              ...saved,
              version: 8,
              signature: {
                ...saved.signature,
                body: "Another controller's saved text",
              },
            };
          if (body.expected_version !== saved.version)
            return jsonResponse({ status: 409, code: "version_skew" }, 409);
          saved = {
            ...saved,
            version: 9,
            signature: { ...saved.signature, body: body.body },
          };
        }
        return jsonResponse(saved);
      }
      throw new Error(`Unexpected request ${path}`);
    }),
  );
  render(
    <SettingsSavePage>
      <RetainedSignaturePanel id={ID} />
    </SettingsSavePage>,
  );
  const text = await screen.findByRole("textbox", {
    name: translate("en", "settings.signatureLabel"),
  });
  await user.clear(text);
  await user.type(text, "Unsaved controller review");
  await user.click(screen.getByRole("button", { name: "Save" }));
  expect(await screen.findByText("Latest saved version")).toBeInTheDocument();
  expect(text).toHaveValue("Unsaved controller review");
  expect(
    screen.getByDisplayValue("Another controller's saved text"),
  ).toBeDisabled();
  expect(versions).toEqual([7]);
  await user.click(screen.getByRole("button", { name: "Keep my draft" }));
  expect(text).toHaveValue("Unsaved controller review");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(versions).toEqual([7, 8]));
  await waitFor(() =>
    expect(
      screen.queryByRole("button", { name: "Save" }),
    ).not.toBeInTheDocument(),
  );
  expect(saved.signature.body).toBe("Unsaved controller review");
});

it("keeps another controller's untouched fields when only the sign-off was edited", async () => {
  const user = steppedClock();
  let saved = original();
  const writes: components["schemas"]["SaveRetainedEmailSignatureRequest"][] =
    [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      if (!(input instanceof Request))
        throw new Error("Expected a generated-client request");
      const path = new URL(input.url).pathname;
      if (path.endsWith("/me")) return jsonResponse(meFixture({}));
      if (path.endsWith(`/privacy/retained-signatures/${ID}`)) {
        if (input.method === "PUT") {
          const body: components["schemas"]["SaveRetainedEmailSignatureRequest"] =
            await input.clone().json();
          writes.push(body);
          if (writes.length === 1)
            saved = {
              ...saved,
              version: 8,
              signature: { ...saved.signature, title: "Their updated title" },
            };
          if (body.expected_version !== saved.version)
            return jsonResponse({ status: 409, code: "version_skew" }, 409);
          saved = {
            ...saved,
            version: 9,
            signature: {
              ...saved.signature,
              body: body.body,
              title: body.title ?? saved.signature.title,
            },
          };
        }
        return jsonResponse(saved);
      }
      throw new Error(`Unexpected request ${path}`);
    }),
  );
  render(
    <SettingsSavePage>
      <RetainedSignaturePanel id={ID} />
    </SettingsSavePage>,
  );
  const text = await screen.findByRole("textbox", {
    name: translate("en", "settings.signatureLabel"),
  });
  await user.clear(text);
  await user.type(text, "My reviewed sign-off");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(writes).toHaveLength(2));
  expect(writes.map((body) => body.expected_version)).toEqual([7, 8]);
  expect(writes[1]).toMatchObject({
    body: "My reviewed sign-off",
    title: "Their updated title",
  });
  await waitFor(() =>
    expect(
      screen.queryByRole("button", { name: "Save" }),
    ).not.toBeInTheDocument(),
  );
});

it("opens the controller's linked review without requiring privacy-settings access", async () => {
  const previous = globalThis.location.hash;
  globalThis.history.replaceState(null, "", signatureReviewHref(ID));
  try {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (!(input instanceof Request))
          throw new Error("Expected a generated-client request");
        const path = new URL(input.url).pathname;
        if (path.endsWith("/me"))
          return jsonResponse(
            meFixture({ allow: { contact: ["read", "delete"] } }),
          );
        if (path.endsWith(`/privacy/retained-signatures/${ID}`))
          return jsonResponse(original());
        throw new Error(`Unexpected request ${path}`);
      }),
    );
    renderSettings("privacy", "requests");
    expect(
      await screen.findByRole("textbox", {
        name: translate("en", "settings.signatureLabel"),
      }),
    ).toHaveValue("Original plain text");
  } finally {
    globalThis.history.replaceState(null, "", previous || "#/");
  }
});
