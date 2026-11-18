/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { installFetchStub, jsonResponse } from "../../../testing/story-utils";
import { EMPTY_DRAFT } from "../onboarding";
import { onboardingProgressOptions } from "../onboarding-progress";
import { useWizardStatePersist } from "./use-wizard-state";

// A journey another tab finished is final, and the server refuses each later
// write. The tab that learns of it goes to Home instead of offering a retry.

function stateAt(step: string, version: number) {
  return {
    step,
    version,
    voice_skipped: true,
    connect_skipped: step === "complete",
  };
}

function renderPersist() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: Readonly<{ children: ReactNode }>) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const { result } = renderHook(() => useWizardStatePersist(), { wrapper });
  return { persist: result.current.persist, client };
}

const finishInput = {
  step: "complete" as const,
  values: EMPTY_DRAFT.values,
  connectSkipped: true,
};

describe("a journey finished in another tab", () => {
  it("hands off when the state it loads is already complete", async () => {
    const put = vi.fn(() => jsonResponse({ code: "conflict" }, 409));
    installFetchStub({
      "GET /onboarding/state": () => jsonResponse(stateAt("complete", 4)),
      "PUT /onboarding/state": put,
    });
    const { persist, client } = renderPersist();

    expect(await persist(finishInput)).toBe(true);
    expect(put).not.toHaveBeenCalled();
    expect(
      client.getQueryData(onboardingProgressOptions.queryKey),
    ).toMatchObject({
      step: "complete",
    });
  });

  it("hands off when its finish is refused over a stale version", async () => {
    let stored = stateAt("voice", 2);
    installFetchStub({
      "GET /onboarding/state": () => jsonResponse(stored),
      "PUT /onboarding/state": () => {
        if (stored.step === "complete") {
          return jsonResponse({ code: "conflict" }, 409);
        }
        stored = stateAt("connect", stored.version + 1);
        return jsonResponse(stored);
      },
    });
    const { persist, client } = renderPersist();
    expect(await persist({ step: "connect", values: EMPTY_DRAFT.values })).toBe(
      true,
    );

    stored = stateAt("complete", 9);
    expect(await persist(finishInput)).toBe(true);
    expect(
      client.getQueryData(onboardingProgressOptions.queryKey),
    ).toMatchObject({
      step: "complete",
    });
  });

  it("still reports a write refused while the journey is unfinished", async () => {
    installFetchStub({
      "GET /onboarding/state": () => jsonResponse(stateAt("connect", 3)),
      "PUT /onboarding/state": () => jsonResponse({ code: "conflict" }, 409),
    });
    const { persist } = renderPersist();

    expect(await persist(finishInput)).toBe(false);
  });
});
