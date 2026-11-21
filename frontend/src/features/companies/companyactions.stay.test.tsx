/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { CompanyNewDealAction, NewDealAction } from "./companyactions";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function drawWith(children: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(
    ["me"],
    meFixture({ allow: { deal: ["create"], company: ["read"] } }),
  );
  client.setQueryData(
    ["pipelines", "all"],
    [
      {
        id: "pipeline-1",
        name: "Sales",
        is_default: true,
        position: 0,
        stages: [
          {
            id: "stage-1",
            pipeline_id: "pipeline-1",
            name: "New",
            semantic: "open",
            position: 0,
            win_probability: 10,
          },
        ],
      },
    ],
  );
  installFetchStub({
    "POST /deals": () => jsonResponse({ id: "deal-1", name: "Renewal" }),
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          {children}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function createDealNamed(name: string) {
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "New deal" }));
  await user.type(screen.getByRole("textbox", { name: "Deal name" }), name);
  await user.click(screen.getByRole("button", { name: "Create" }));
  return user;
}

describe("a deal made from the company's Deals tab", () => {
  it("stays on the company, says what was added and opens it on request", async () => {
    window.location.hash = "#/companies/company-1";
    drawWith(<CompanyNewDealAction companyId="company-1" companyName="Co" />);
    const user = await createDealNamed("Renewal");

    expect(await screen.findByText("Renewal added")).toBeTruthy();
    expect(window.location.hash).toBe("#/companies/company-1");
    await user.click(screen.getByRole("button", { name: "Open" }));
    expect(window.location.hash).toBe("#/deals/deal-1");
  });
});

describe("a deal made from a project", () => {
  it("opens the deal", async () => {
    window.location.hash = "#/projects/project-1";
    drawWith(
      <NewDealAction
        companyId="company-1"
        companyName="Co"
        projectId="project-1"
        link
      />,
    );
    await createDealNamed("Renewal");

    await vi.waitFor(() => expect(window.location.hash).toBe("#/deals/deal-1"));
    expect(screen.queryByText("Renewal added")).toBeNull();
  });
});
