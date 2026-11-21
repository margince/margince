/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { useQuery } from "@tanstack/react-query";
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { use } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { reloadForNewBuild } from "../../lib/stalebuild";
import { render } from "../../testing/settings.testkit";
import { PageLoading, page } from "./settingspageviews";

vi.mock("../../lib/stalebuild", async (importActual) => ({
  ...(await importActual<typeof import("../../lib/stalebuild")>()),
  reloadForNewBuild: vi.fn(() => true),
}));

beforeEach(() => {
  vi.mocked(reloadForNewBuild).mockClear();
  // React reports every error a boundary catches, which is these cases' subject.
  vi.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function renderPage(View: (at: { tab: string }) => React.ReactNode) {
  return render(
    <PageLoading id="audit">
      <View tab="" />
    </PageLoading>,
  );
}

describe("Retry on a settings page that failed", () => {
  it("reloads the app when the page's code import was refused", async () => {
    // Not a stale chunk: the browser keeps a module whose evaluation threw.
    const load = vi.fn(async (): Promise<{ Card: () => React.ReactNode }> => {
      throw new SyntaxError("The requested module does not provide an export");
    });
    const View = page(load, ({ Card }) => <Card />);
    const user = userEvent.setup();
    renderPage(View);

    await user.click(await screen.findByRole("button", { name: "Retry" }));

    expect(reloadForNewBuild).toHaveBeenCalledTimes(1);
  });

  it("draws the page again, without a reload, after a card threw", async () => {
    const gate = { open: false };
    function Card() {
      if (!gate.open) throw new TypeError("rows is undefined");
      return <p>Card arrived</p>;
    }
    const View = page(
      async () => ({ Card }),
      (parts) => <parts.Card />,
    );
    const user = userEvent.setup();
    renderPage(View);
    const retry = await screen.findByRole("button", { name: "Retry" });
    gate.open = true;

    await user.click(retry);

    expect(await screen.findByText("Card arrived")).toBeInTheDocument();
    expect(reloadForNewBuild).not.toHaveBeenCalled();
  });

  it("reads again, without a reload, after a read failed", async () => {
    const gate = { open: false };
    function Card() {
      const answer = useQuery({
        queryKey: ["settings-page-answer"],
        queryFn: async () => {
          if (!gate.open) throw new Error("503");
          return "Answered";
        },
        throwOnError: true,
      });
      return answer.data ? <p>{answer.data}</p> : null;
    }
    const View = page(
      async () => ({ Card }),
      (parts) => <parts.Card />,
    );
    const user = userEvent.setup();
    renderPage(View);
    const retry = await screen.findByRole("button", { name: "Retry" });
    gate.open = true;

    await user.click(retry);

    expect(await screen.findByText("Answered")).toBeInTheDocument();
    expect(reloadForNewBuild).not.toHaveBeenCalled();
  });
});

const neverArrives = new Promise<never>(() => {});

function ArrivingPage() {
  return use(neverArrives);
}

describe("a settings page while its code arrives", () => {
  it("says which page is loading", async () => {
    render(
      <PageLoading id="audit">
        <ArrivingPage />
      </PageLoading>,
    );

    expect(await screen.findByRole("status")).toHaveTextContent(
      "Loading Audit log…",
    );
  });
});
