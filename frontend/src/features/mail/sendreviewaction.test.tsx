/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import type { SendReview } from "./sendreview";
import { SendReviewActions } from "./sendreviewaction";

// Each vouch is its own write. A rep who retries after one failed must not
// record the others a second time.

const review: SendReview = {
  reviewId: "review-1",
  actions: ["overrule"],
  overrulable: [
    { contactId: "contact-a", category: "marketing" },
    { contactId: "contact-b", category: "marketing" },
  ],
};

function stubAllowDoor(failFirstTry: string) {
  const posted: string[] = [];
  let failed = false;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : null;
      const path = new URL(
        request ? request.url : String(input),
        "https://test.local",
      ).pathname;
      const contact = path.split("/")[3] ?? "";
      if (contact === failFirstTry && !failed) {
        failed = true;
        return new Response(JSON.stringify({ title: "boom" }), {
          status: 500,
          headers: { "Content-Type": "application/problem+json" },
        });
      }
      posted.push(contact);
      return new Response(JSON.stringify({ id: `override-${contact}` }), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  return posted;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("SendReviewActions", () => {
  it("records each vouch once when a retry follows a failure", async () => {
    const posted = stubAllowDoor("contact-b");
    const user = userEvent.setup();
    render(
      <LocaleProvider initial="en">
        <QueryClientProvider client={new QueryClient()}>
          <SendReviewActions review={review} />
        </QueryClientProvider>
      </LocaleProvider>,
    );

    await user.click(
      screen.getByRole("button", { name: en["compose.overrule.open"] }),
    );
    await user.type(
      screen.getByRole("textbox", {
        name: new RegExp(en["compose.overrule.reasonLabel"]),
      }),
      "The customer asked for this on the phone.",
    );
    const confirm = screen.getByRole("button", {
      name: en["compose.overrule.confirm"],
    });
    await user.click(confirm);
    expect(
      await screen.findByText(en["compose.overrule.failed"]),
    ).toBeInTheDocument();
    expect(posted).toEqual(["contact-a"]);

    await user.click(confirm);
    expect(
      await screen.findByText(en["compose.overrule.recorded"]),
    ).toBeInTheDocument();
    expect(posted).toEqual(["contact-a", "contact-b"]);
  });

  it("draws nothing when the server offers no overrule", () => {
    const { container } = render(
      <LocaleProvider initial="en">
        <QueryClientProvider client={new QueryClient()}>
          <SendReviewActions review={{ ...review, actions: [] }} />
        </QueryClientProvider>
      </LocaleProvider>,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
