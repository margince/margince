/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider, translate } from "../i18n";
import { ProblemError } from "../lib/problem";
import { QueryStates, WriteRefused } from "./querystates";

const t = (key: Parameters<typeof translate>[1]) => translate("en", key);

afterEach(() => {
  cleanup();
  window.location.hash = "";
  // Restored here rather than at the end of the case that installed a spy.
  // A case that fails its assertion never reaches its own teardown. A leaked
  // console spy then silences the rest of the file, which looks like a
  // suite that passes.
  vi.restoreAllMocks();
});

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("QueryStates", () => {
  const failing = (error: unknown) => ({
    isPending: false,
    isError: true,
    error,
    refetch: () => undefined,
  });

  it("prints the server's detail for a failed query", () => {
    render(
      <QueryStates
        query={failing(new ProblemError({ detail: "no seat" }))}
        pendingLabel="Loading the section"
      >
        {null}
      </QueryStates>,
    );
    expect(screen.getByText("no seat")).toBeTruthy();
  });

  it("prints the shared line, not the message, when the failure is not a problem", () => {
    render(
      <QueryStates
        query={failing(new TypeError("Failed to fetch"))}
        pendingLabel="Loading the section"
      >
        {null}
      </QueryStates>,
    );
    expect(screen.getByText(t("common.errorNoCause"))).toBeTruthy();
    expect(screen.queryByText(/Failed to fetch/)).toBeNull();
  });

  // Every query-backed card in the product renders its failure through here.
  // A failure that does not reach a reader who cannot see the screen fails
  // them on most of the product at once.
  it("announces a failed load, headline and cause in one region", () => {
    render(
      <QueryStates
        query={failing(new ProblemError({ detail: "no seat" }))}
        pendingLabel="Loading the section"
      >
        {null}
      </QueryStates>,
    );
    const announced = screen.getByRole("alert");
    // Both, in the one region: a reader who hears only "Couldn't load this
    // view" has been told less than the screen says.
    expect(announced.textContent).toContain(t("common.error"));
    expect(announced.textContent).toContain("no seat");
    // Retry is a control to reach, not a sentence to hear.
    expect(
      within(announced).queryByRole("button", { name: t("common.retry") }),
    ).toBeNull();
    expect(
      screen.getByRole("button", { name: t("common.retry") }),
    ).toBeTruthy();
  });

  it("says a 404 or 410 is gone when the screen names it, and offers no Retry", () => {
    for (const status of [404, 410]) {
      const { unmount } = render(
        <QueryStates
          query={failing(new ProblemError({ status, detail: "not found" }))}
          pendingLabel="Loading the section"
          goneKey="book.linkGone"
          goneAction={<a href="#back">Back</a>}
        >
          {null}
        </QueryStates>,
      );
      expect(screen.getByRole("alert").textContent).toBe(t("book.linkGone"));
      expect(
        screen.queryByRole("button", { name: t("common.retry") }),
      ).toBeNull();
      expect(screen.getByText("Back")).toBeTruthy();
      unmount();
    }
  });

  it("keeps a 404 a failed load with Retry when the screen names no gone sentence, since it may be a scope refusal", () => {
    render(
      <QueryStates
        query={failing(new ProblemError({ status: 404, detail: "not found" }))}
        pendingLabel="Loading the section"
      >
        {null}
      </QueryStates>,
    );
    expect(screen.getByText(t("common.error"))).toBeTruthy();
    expect(
      screen.getByRole("button", { name: t("common.retry") }),
    ).toBeTruthy();
  });

  it("keeps a 5xx a failed load with Retry even when the screen names a gone sentence", () => {
    render(
      <QueryStates
        query={failing(new ProblemError({ status: 503, detail: "down" }))}
        pendingLabel="Loading the section"
        goneKey="book.linkGone"
      >
        {null}
      </QueryStates>,
    );
    expect(screen.queryByText(t("book.linkGone"))).toBeNull();
    expect(
      screen.getByRole("button", { name: t("common.retry") }),
    ).toBeTruthy();
  });

  it("reports a load in progress as busy, since the shimmer bars say nothing", () => {
    render(
      <QueryStates
        pendingLabel="Loading the section"
        query={{
          isPending: true,
          isError: false,
          error: null,
          refetch: () => undefined,
        }}
      >
        {null}
      </QueryStates>,
    );
    const loading = screen.getByRole("status");
    expect(loading.getAttribute("aria-busy")).toBe("true");
    // And nothing claims to have failed while it is still loading.
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

describe("WriteRefused", () => {
  const tooLong = new ProblemError({
    status: 422,
    code: "validation_error",
    detail: "a signature's html is at most 10000 characters, this one is 10001",
    details: {
      errors: [
        {
          field: "html",
          code: "too_long",
          message:
            "a signature's html is at most 10000 characters, this one is 10001",
          max: 10000,
        },
      ],
    },
  });

  it("names the field by the form's own label, and its limit, in the reader's language", async () => {
    const client = new QueryClient();
    rtlRender(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="de">
          <WriteRefused
            titleKey="common.error"
            error={tooLong}
            fieldLabels={{ html: "HTML-Signatur" }}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    expect(
      await screen.findByText("HTML-Signatur: höchstens 10.000 Zeichen."),
    ).toBeTruthy();
  });
});
