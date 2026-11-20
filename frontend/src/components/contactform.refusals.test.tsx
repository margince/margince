/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { type Locale, LocaleProvider, useT } from "../i18n";
import { throwProblem } from "../lib/problem";
import { contactCreateFields } from "./contactformfields";
import { CreateAction } from "./create";

// New contact's own fields. A refusal of a phone, an address or a profile link
// reads beside the field it names, in the reader's language. It never reads as
// the server's English detail.

afterEach(() => {
  cleanup();
});

function refusal(field: string, code: string, message: string) {
  return {
    code: "validation_error",
    detail: message,
    details: { errors: [{ field, code, message }] },
  };
}

function NewContact({ problem }: Readonly<{ problem: unknown }>) {
  const t = useT();
  return (
    <CreateAction
      label="New contact"
      fields={contactCreateFields(t)}
      create={() => throwProblem(problem)}
      invalidate="contacts"
      screen="contacts"
    />
  );
}

async function submit(problem: unknown, locale: Locale = "en") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const { container } = rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <NewContact problem={problem} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await userEvent.click(screen.getByText("New contact"));
  const fullName = document.querySelector<HTMLInputElement>("input[maxlength]");
  if (!fullName) throw new Error("New contact has no name input");
  await userEvent.type(fullName, "Probe");
  // A contact needs a way to reach them before Create turns on.
  await userEvent.type(
    screen.getByLabelText("LinkedIn"),
    "https://www.linkedin.com/in/probe",
  );
  await userEvent.click(
    screen.getByRole("button", { name: /^(Create|Erstellen|Tạo)$/ }),
  );
  return container;
}

// The refusal sits inside the block of the field it names.
function blockOf(text: string): Element | null {
  return screen.getByText(text).closest(".field, .field-repeatable");
}

describe("New contact refusals", () => {
  it("says a number without its country prefix under the phone rows", async () => {
    await submit(
      refusal(
        "phone",
        "phone_needs_country_code",
        "the number needs its country prefix (+49…, 0049…)",
      ),
    );
    const sentence =
      "Start the number with its country code, such as +49 or 0049.";
    await waitFor(() => expect(screen.getByText(sentence)).toBeTruthy());
    expect(blockOf(sentence)?.classList).toContain("field-repeatable");
    expect(screen.queryByText(/country prefix/)).toBeNull();
  });

  it("says a malformed number in German", async () => {
    await submit(
      refusal("phone", "phone_malformed", "not an E.164 number"),
      "de",
    );
    await waitFor(() =>
      expect(screen.getByText(/^Das ist keine Telefonnummer/)).toBeTruthy(),
    );
    expect(screen.queryByText(/E\.164/)).toBeNull();
  });

  it("says a refused address under the email rows", async () => {
    await submit(
      refusal(
        "emails",
        "email_names_no_one",
        "this address belongs to a sending system, not a human; add the contact's own address",
      ),
      "vi",
    );
    await waitFor(() =>
      expect(screen.getByText(/^Địa chỉ này thuộc về/)).toBeTruthy(),
    );
  });

  it("says a link off LinkedIn beside the LinkedIn field", async () => {
    await submit(
      refusal(
        "linkedin_url",
        "linkedin_url_not_linkedin",
        "a LinkedIn profile URL is on linkedin.com",
      ),
    );
    const sentence = "Enter a link to a page on linkedin.com.";
    await waitFor(() => expect(screen.getByText(sentence)).toBeTruthy());
    expect(blockOf(sentence)?.textContent).toContain("LinkedIn");
    expect(document.querySelectorAll("[role=alert]").length).toBe(1);
  });
});
