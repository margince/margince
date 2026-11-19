/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ProblemError } from "../lib/problem";
import { Button } from "./atoms";
import { ConnectAiFirst, ModelWorkError, useConnectAiGate } from "./connectai";
import { TranscriptReadCard } from "./transcriptread";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function setupAnswer(configured: boolean) {
  return json({
    complete: configured,
    steps: [{ step: "ai_models", configured, blocking: false }],
  });
}

function mount(ui: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function AiButton({ onPress }: Readonly<{ onPress: () => void }>) {
  const gate = useConnectAiGate("ai-button-reason");
  return (
    <>
      <Button reasonId={gate.reasonId} onClick={onPress}>
        Draft with AI
      </Button>
      {gate.notice}
    </>
  );
}

describe("a button that asks the AI to work", () => {
  it("stays visible, refuses the press and links to AI settings while no model is connected", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => setupAnswer(false)),
    );
    const onPress = vi.fn();
    mount(<AiButton onPress={onPress} />);

    const link = await screen.findByRole("link", { name: "Open AI settings" });
    expect(link.getAttribute("href")).toContain("settings/models");
    expect(screen.getByText(/Connect an AI first/)).toBeTruthy();
    const button = screen.getByRole("button", { name: "Draft with AI" });
    await userEvent.setup().click(button);
    expect(onPress).not.toHaveBeenCalled();
  });

  it("works as usual once a model is connected", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => setupAnswer(true)),
    );
    const onPress = vi.fn();
    mount(<AiButton onPress={onPress} />);

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalled());
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Draft with AI" }));
    expect(onPress).toHaveBeenCalledTimes(1);
    expect(screen.queryByText(/Connect an AI first/)).toBeNull();
  });

  it("is not refused when the setup answer does not list the model step", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => json({ unrelated: true })),
    );
    const onPress = vi.fn();
    mount(<AiButton onPress={onPress} />);

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalled());
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Draft with AI" }));
    expect(onPress).toHaveBeenCalledTimes(1);
  });
});

describe("the line under a button whose AI work failed", () => {
  it("says Connect an AI first with the settings link for ai_not_configured", () => {
    mount(
      <ModelWorkError
        error={
          new ProblemError({
            status: 409,
            code: "ai_not_configured",
            detail: "No AI model is connected.",
          })
        }
      />,
    );
    expect(screen.getByRole("alert").textContent).toContain(
      "Connect an AI first",
    );
    expect(screen.getByRole("link", { name: "Open AI settings" })).toBeTruthy();
  });

  it("keeps the server's words for any other problem", () => {
    mount(
      <ModelWorkError
        error={
          new ProblemError({
            status: 422,
            code: "validation_error",
            detail: "Too long.",
          })
        }
      />,
    );
    expect(screen.getByRole("alert").textContent).toBe("Too long.");
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("renders the sentence on its own", () => {
    mount(<ConnectAiFirst />);
    expect(screen.getByRole("link", { name: "Open AI settings" })).toBeTruthy();
  });
});

describe("the transcript card with no model connected", () => {
  it("shows Connect an AI first before the press and after a refused one", async () => {
    let refused = false;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = new URL(
          input instanceof Request ? input.url : String(input),
          "http://localhost",
        );
        const method =
          init?.method ?? (input instanceof Request ? input.method : "GET");
        if (url.pathname.endsWith("/installation/setup")) {
          return setupAnswer(false);
        }
        if (
          url.pathname.endsWith("/transcript-proposals") &&
          method === "POST"
        ) {
          refused = true;
          return json(
            { status: 409, code: "ai_not_configured", detail: "No AI model." },
            409,
          );
        }
        return json({}, 404);
      }),
    );
    mount(<TranscriptReadCard activityId="a-1" />);

    expect(await screen.findByText(/Connect an AI first/)).toBeTruthy();
    expect(screen.getByRole("link", { name: "Open AI settings" })).toBeTruthy();
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "Read transcript" }));
    expect(refused).toBe(false);
  });
});
