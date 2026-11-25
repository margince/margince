/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import type { RequiredGrant } from "../lib/capability";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import { MergeAction } from "./merge";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mount(allow: GrantSpec, requires: RequiredGrant) {
  vi.stubGlobal("fetch", async () => Response.json(meFixture({ allow })));
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <MergeAction
          label="Merge contact"
          sourceId="c-1"
          sourceName="Dana B."
          searchTargets={async () => []}
          merge={async () => ({ id: "c-2" })}
          invalidate="contacts"
          recordKey="contact"
          survivorRoute={(id) => ({ screen: "contacts", id })}
          requires={requires}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// Refused, the button is wrapped beside its reason, so it is a new element.
async function refusal(): Promise<string | null | undefined> {
  await waitFor(() =>
    expect(screen.getByTestId("merge-record").hasAttribute("disabled")).toBe(
      true,
    ),
  );
  const merge = screen.getByTestId("merge-record");
  return document.getElementById(merge.getAttribute("aria-describedby") ?? "")
    ?.textContent;
}

describe("MergeAction refused", () => {
  it("names the role when the role may not change the record", async () => {
    mount({ contact: ["read"] }, { object: "contact", action: "update" });

    expect(await refusal()).toBe(en["record.roleReadsOnly"]);
  });

  it("names the missing right to a role that writes the record", async () => {
    mount(
      { contact: ["read", "update"] },
      { object: "contact", action: "delete" },
    );

    expect(await refusal()).toBe(en["record.actionRefused"]);
  });
});
