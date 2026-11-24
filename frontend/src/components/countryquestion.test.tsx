/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { pickOption } from "../design-system/select-testing";
import { LocaleProvider, translate } from "../i18n";
import { SEND_PERMISSION_KEY } from "../lib/usesendpermission";
import { CountryQuestion } from "./countryquestion";

type Preview = components["schemas"]["SendAuthorizationPreview"];
type Recipient = components["schemas"]["SendAuthorizationPreviewRecipient"];

const CONTACT = "018f3a1b-0000-7000-8000-000000000042";

const refused = (over: Partial<Recipient>): Recipient => ({
  address: "dana@example.test",
  verdict: "deny",
  reason_code: "marketing_permission_required",
  evidence_subject: { entity_type: "contact", entity_id: CONTACT },
  ...over,
});

const preview = (...recipients: Recipient[]): Preview => ({
  allowed: false,
  recipients,
});

const json = (body: unknown) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });

function draw(answer: Preview) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  const invalidated = vi.spyOn(client, "invalidateQueries");
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CountryQuestion preview={answer} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return invalidated;
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("the question for an unknown recipient country", () => {
  it("shows no question when the preview does not ask for a country", () => {
    draw(preview(refused({})));
    expect(
      screen.queryByText(/Which country is dana@example.test in\?/),
    ).toBeNull();
  });

  it("shows no question for a lead", () => {
    draw(
      preview(
        refused({
          ask_country: true,
          evidence_subject: { entity_type: "lead", entity_id: CONTACT },
        }),
      ),
    );
    expect(screen.queryByText(/Which country is/)).toBeNull();
  });

  it("saves the answer on the contact with its other address parts and asks the preview again", async () => {
    const calls: { method: string; body: unknown; ifMatch: string | null }[] =
      [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const request = input as Request;
      calls.push({
        method: request.method,
        body: request.method === "PATCH" ? await request.clone().json() : null,
        ifMatch: request.headers.get("If-Match"),
      });
      return json({
        id: CONTACT,
        version: 7,
        address: { line1: "1 Main Street", country: null },
      });
    });
    const invalidated = draw(preview(refused({ ask_country: true })));
    const user = userEvent.setup();

    expect(
      screen.getByText(
        translate("en", "countryQuestion.title", {
          address: "dana@example.test",
        }),
      ),
    ).toBeVisible();
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Country" }),
      "United States",
    );
    await user.click(screen.getByRole("button", { name: "Save country" }));

    await waitFor(() =>
      expect(invalidated).toHaveBeenCalledWith({
        queryKey: [...SEND_PERMISSION_KEY],
      }),
    );
    const patch = calls.find((call) => call.method === "PATCH");
    expect(patch?.ifMatch).toBe("7");
    expect(patch?.body).toEqual({
      address: { line1: "1 Main Street", country: "US" },
    });
  });

  it("does not overwrite a country stored since the preview asked", async () => {
    const methods: string[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      methods.push((input as Request).method);
      return json({ id: CONTACT, version: 8, address: { country: "DE" } });
    });
    draw(preview(refused({ ask_country: true })));
    const user = userEvent.setup();
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Country" }),
      "United States",
    );
    await user.click(screen.getByRole("button", { name: "Save country" }));

    expect(
      await screen.findByText(translate("en", "countryQuestion.alreadySet")),
    ).toBeVisible();
    expect(methods).not.toContain("PATCH");
  });

  it("says so when the contact changed between the read and the write", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      if ((input as Request).method === "PATCH") {
        return new Response(
          JSON.stringify({ title: "Version skew", code: "version_skew" }),
          {
            status: 409,
            headers: { "Content-Type": "application/problem+json" },
          },
        );
      }
      return json({ id: CONTACT, version: 7, address: { country: null } });
    });
    draw(preview(refused({ ask_country: true })));
    const user = userEvent.setup();
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Country" }),
      "United States",
    );
    await user.click(screen.getByRole("button", { name: "Save country" }));

    expect(await screen.findByRole("alert")).toBeVisible();
  });
});
