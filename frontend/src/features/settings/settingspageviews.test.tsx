/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { use } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { render } from "../../testing/settings.testkit";
import { PageLoading, page } from "./settingspageviews";

afterEach(cleanup);

// A refusal the stale-build reload does not match, so only a new import helps.
const linkingFailure = () =>
  new SyntaxError("The requested module does not provide an export");

// The code a page imports, refused until the test lets it through.
function gatedCode() {
  const gate = { open: false };
  const load = vi.fn(async () => {
    if (!gate.open) throw linkingFailure();
    return { DrawnCard: () => <p>Card arrived</p> };
  });
  return { gate, load };
}

describe("a settings page whose code was refused", () => {
  it("imports its code again when the reader presses Retry", async () => {
    const quiet = vi.spyOn(console, "error").mockImplementation(() => {});
    const { gate, load } = gatedCode();
    const View = page(load, ({ DrawnCard }) => <DrawnCard />);
    const user = userEvent.setup();
    render(
      <PageLoading id="audit">
        <View tab="" />
      </PageLoading>,
    );
    const retry = await screen.findByRole("button", { name: "Retry" });
    gate.open = true;

    await user.click(retry);

    expect(await screen.findByText("Card arrived")).toBeInTheDocument();
    quiet.mockRestore();
  });

  it("imports its code again when the page is opened again", async () => {
    const quiet = vi.spyOn(console, "error").mockImplementation(() => {});
    const { gate, load } = gatedCode();
    const View = page(load, ({ DrawnCard }) => <DrawnCard />);
    const first = render(
      <PageLoading id="audit">
        <View tab="" />
      </PageLoading>,
    );
    expect(
      await screen.findByText("This card stopped working"),
    ).toBeInTheDocument();
    first.unmount();
    gate.open = true;

    render(
      <PageLoading id="audit">
        <View tab="" />
      </PageLoading>,
    );

    expect(await screen.findByText("Card arrived")).toBeInTheDocument();
    quiet.mockRestore();
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
