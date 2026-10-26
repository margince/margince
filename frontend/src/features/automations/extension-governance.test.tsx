/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { UnitGovernance } from "./extension-governance";

// One unit's governance block: the overwrite switch and the settings the unit
// declares. The transport is stubbed at `fetch`; only the settings route is read.

type Call = { method: string; url: string; body: unknown };

function backend(calls: Call[]) {
  return async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = new Request(input, init);
    const body = req.method === "GET" ? undefined : await req.clone().json();
    if (req.method !== "GET") {
      calls.push({ method: req.method, url: req.url, body });
      return Response.json(body);
    }
    return Response.json({
      overwrites_edited_fields: false,
      declared: [
        {
          key: "region",
          label: "Region",
          hint: "Where notes are filed",
          kind: "text",
          value: "eu-west",
        },
        { key: "live", label: "Live", kind: "switch", value: "false" },
      ],
    });
  };
}

function renderGovernance() {
  const calls: Call[] = [];
  vi.stubGlobal("fetch", backend(calls));
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <UnitGovernance unit="notes" canManage />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return calls;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("UnitGovernance", () => {
  it("PUTs a unit's overwrite choice to that unit's own settings", async () => {
    const calls = renderGovernance();

    const control = await screen.findByTestId("ext-overwrite-notes");
    await waitFor(() =>
      expect(control.getAttribute("aria-busy")).not.toBe("true"),
    );
    expect(control.getAttribute("aria-checked")).toBe("false");
    await userEvent.click(control);

    await waitFor(() => {
      const put = calls.find((call) => call.method === "PUT");
      expect(put?.url).toContain("/v1/extensions/notes/settings");
      expect(put?.body).toEqual({ overwrites_edited_fields: true });
    });
    await waitFor(() =>
      expect(control.getAttribute("aria-checked")).toBe("true"),
    );
  });

  it("draws the settings a unit declares and PUTs only the one that changed", async () => {
    const calls = renderGovernance();

    const region = await screen.findByTestId("ext-setting-notes-region");
    expect((region as HTMLInputElement).value).toBe("eu-west");
    expect(screen.getAllByText("Where notes are filed").length).toBeGreaterThan(
      0,
    );
    const user = userEvent.setup();
    await user.clear(region);
    await user.type(region, "us-east");
    await user.tab();

    await waitFor(() => {
      const put = calls.find((call) => call.method === "PUT");
      expect(put?.url).toContain("/v1/extensions/notes/settings");
      expect(put?.body).toEqual({
        overwrites_edited_fields: false,
        declared: [{ key: "region", value: "us-east" }],
      });
    });
  });

  it("PUTs a declared switch as the text true", async () => {
    const calls = renderGovernance();

    const live = await screen.findByTestId("ext-setting-notes-live");
    await waitFor(() =>
      expect(live.getAttribute("aria-busy")).not.toBe("true"),
    );
    await userEvent.click(live);

    await waitFor(() =>
      expect(calls.find((call) => call.method === "PUT")?.body).toEqual({
        overwrites_edited_fields: false,
        declared: [{ key: "live", value: "true" }],
      }),
    );
  });
});
