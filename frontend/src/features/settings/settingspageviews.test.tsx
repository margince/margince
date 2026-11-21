/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { useQuery } from "@tanstack/react-query";
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { use } from "react";
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  type Mock,
  vi,
} from "vitest";
import { STORAGE_KEYS, writeStored } from "../../lib/storage";
import { render } from "../../testing/settings.testkit";
import { PageLoading, page } from "./settingspageviews";

let reload: Mock<() => void>;

beforeEach(() => {
  sessionStorage.clear();
  reload = vi.fn<() => void>();
  vi.spyOn(window, "location", "get").mockReturnValue({
    ...window.location,
    reload,
  });
  // React reports every error a boundary catches, which is these cases' subject.
  vi.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

function renderPage(View: (at: { tab: string }) => React.ReactNode) {
  return render(
    <PageLoading id="audit">
      <View tab="" />
    </PageLoading>,
  );
}

// Not a stale chunk: the browser keeps a module whose evaluation threw.
function renderRefusedPage() {
  const load = vi.fn(async (): Promise<{ Card: () => React.ReactNode }> => {
    throw new SyntaxError("The requested module does not provide an export");
  });
  return renderPage(page(load, ({ Card }) => <Card />));
}

describe("Retry on a settings page that failed", () => {
  it("reloads the app when the page's code import was refused", async () => {
    const user = userEvent.setup();
    renderRefusedPage();

    await user.click(await screen.findByRole("button", { name: "Retry" }));

    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("reloads even when an automatic reload ran a moment ago", async () => {
    const user = userEvent.setup();
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(1_000_000);
    writeStored(STORAGE_KEYS.staleBuildReloadedAt, String(1_000_000 - 5_000));
    renderRefusedPage();

    await user.click(await screen.findByRole("button", { name: "Retry" }));

    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("reloads even when session storage is refused", async () => {
    const user = userEvent.setup();
    renderRefusedPage();
    const retry = await screen.findByRole("button", { name: "Retry" });
    vi.spyOn(window, "sessionStorage", "get").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });

    await user.click(retry);

    expect(reload).toHaveBeenCalledTimes(1);
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
    expect(reload).not.toHaveBeenCalled();
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
    expect(reload).not.toHaveBeenCalled();
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
