// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { useToast } from "../../design-system/toast";
import { meFixture } from "../../testing/mefixture";
import { jsonResponse, render } from "../../testing/settings.testkit";
import { steppedClock } from "../../testing/steppedclock";
import { SignatureSettingRow } from "./settingssignaturerow";

function Row() {
  return <SignatureSettingRow toast={useToast()} />;
}
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

it.each([true, false])(
  "shows server-sanitized personal markup and clears it in Plain text mode (template %s)",
  async (template) => {
    const user = steppedClock();
    const raw = '<p onclick="bad()">Personal<script>bad()</script></p>';
    let saved: components["schemas"]["EmailSignature"] = {
      body: "Saved plain",
      html: raw,
      title: "Sales",
      phone: "",
      template_active: template,
    };
    const writes: components["schemas"]["SaveEmailSignatureRequest"][] = [];
    const previews: components["schemas"]["EmailSignOffRequest"][] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (!(input instanceof Request))
          throw new Error("Expected a generated-client request");
        const path = new URL(input.url).pathname;
        if (path.endsWith("/me")) return jsonResponse(meFixture({}));
        if (path.endsWith("/me/email-signature")) {
          if (input.method === "PUT") {
            const value: components["schemas"]["SaveEmailSignatureRequest"] =
              await input.clone().json();
            writes.push(value);
            saved = { ...saved, ...value, html: value.html ?? saved.html };
          }
          return jsonResponse(saved);
        }
        if (path.endsWith("/emails:sign-off")) {
          const value: components["schemas"]["EmailSignOffRequest"] =
            await input.clone().json();
          previews.push(value);
          return jsonResponse(
            value.draft?.html
              ? {
                  kind: "signature",
                  text: "Server sanitized",
                  html: "<p><b>Server sanitized</b></p>",
                }
              : template
                ? { kind: "template", text: "Company", html: "<p>Company</p>" }
                : { kind: "signature", text: "Saved plain" },
          );
        }
        throw new Error(`Unexpected request ${path}`);
      }),
    );
    render(<Row />);
    await screen.findByText("Your HTML signature");
    await user.click(screen.getByRole("button", { name: "Edit signature" }));
    const dialog = await screen.findByRole("dialog");
    const box = within(dialog).getByRole<HTMLTextAreaElement>("textbox", {
      name: "HTML signature",
    });
    expect(box.value).toBe(raw);
    const frame = await within(dialog).findByTitle("Signature preview");
    expect(frame.getAttribute("srcdoc")).toContain("<b>Server sanitized</b>");
    expect(frame.getAttribute("srcdoc")).not.toContain("onclick");
    expect(frame.getAttribute("srcdoc")).not.toContain("<script>");
    await user.click(
      within(dialog).getByRole("button", { name: "Plain text" }),
    );
    await waitFor(() =>
      expect(previews.some((value) => value.draft?.html === "")).toBe(true),
    );
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(writes).toEqual([
        { body: "Saved plain", title: "Sales", phone: "", html: "" },
      ]),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  },
);

it("keeps the current edit open while its save is pending", async () => {
  const user = steppedClock();
  let finish: ((response: Response) => void) | undefined;
  const receipt = new Promise<Response>((resolve) => {
    finish = resolve;
  });
  const saved: components["schemas"]["EmailSignature"] = {
    body: "Saved plain",
    html: "<p>Personal</p>",
    title: "",
    phone: "",
    template_active: false,
  };
  let writing = false;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      if (!(input instanceof Request))
        throw new Error("Expected a generated-client request");
      const path = new URL(input.url).pathname;
      if (path.endsWith("/me")) return jsonResponse(meFixture({}));
      if (path.endsWith("/me/email-signature")) {
        if (input.method === "PUT") {
          writing = true;
          return receipt;
        }
        return jsonResponse(saved);
      }
      if (path.endsWith("/emails:sign-off"))
        return jsonResponse({
          kind: "signature",
          text: "Personal",
          html: "<p>Personal</p>",
        });
      throw new Error(`Unexpected request ${path}`);
    }),
  );
  render(<Row />);
  await screen.findByText("Your HTML signature");
  await user.click(screen.getByRole("button", { name: "Edit signature" }));
  const dialog = await screen.findByRole("dialog");
  const box = within(dialog).getByRole("textbox", { name: "HTML signature" });
  await user.type(box, " Updated");
  await user.click(within(dialog).getByRole("button", { name: "Save" }));
  await waitFor(() => expect(writing).toBe(true));
  expect(
    within(dialog).getByRole<HTMLButtonElement>("button", { name: "Cancel" })
      .disabled,
  ).toBe(true);
  expect(
    screen.getByRole<HTMLButtonElement>("button", { name: "Edit signature" })
      .disabled,
  ).toBe(true);
  await user.keyboard("{Escape}");
  expect(screen.getByRole("dialog")).toBe(dialog);
  if (finish === undefined)
    throw new Error("The save response has no resolver");
  finish(jsonResponse(saved));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});

it("keeps Save disabled when changing format would submit the same empty HTML", async () => {
  const user = steppedClock();
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      if (!(input instanceof Request))
        throw new Error("Expected a generated-client request");
      const path = new URL(input.url).pathname;
      if (path.endsWith("/me")) return jsonResponse(meFixture({}));
      if (path.endsWith("/me/email-signature") && input.method === "GET")
        return jsonResponse({
          body: "Saved plain",
          html: "",
          title: "",
          phone: "",
          template_active: false,
        });
      if (path.endsWith("/emails:sign-off"))
        return jsonResponse({ kind: "signature", text: "Saved plain" });
      throw new Error(`Unexpected request ${path}`);
    }),
  );
  render(<Row />);
  await screen.findByText("Saved plain");
  await user.click(screen.getByRole("button", { name: "Edit signature" }));
  const dialog = await screen.findByRole("dialog");
  await user.click(within(dialog).getByRole("button", { name: "HTML" }));
  expect(
    within(dialog).getByRole<HTMLButtonElement>("button", { name: "Save" })
      .disabled,
  ).toBe(true);
});
