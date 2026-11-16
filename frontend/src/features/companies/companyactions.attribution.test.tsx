/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import { healthScope } from "../../lib/healthscopekey";
import { meFixture } from "../../testing/mefixture";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { NewDealAction } from "./companyactions";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it.each([undefined, "project-1"])(
  "creates a deal with owner, source and partner from record context %s",
  async (projectId) => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    });
    const me = meFixture({ allow: { deal: ["create"], company: ["read"] } });
    client.setQueryData(["me"], me);
    const readerScope = healthScope(me);
    client.setQueryData(["users"], {
      entries: [
        me.user,
        { ...me.user, id: "colleague", display_name: "Colleague" },
      ],
      partial: false,
    });
    client.setQueryData(
      ["partners", "options", readerScope],
      [{ company_id: "partner-1" }],
    );
    client.setQueryData(
      ["partners", "names", readerScope],
      [{ id: "partner-1", display_name: "Referring partner" }],
    );
    client.setQueryData(
      ["acquisition-sources"],
      [{ key: "referral", label: "Referral", active: true }],
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
    const sent: unknown[] = [];
    installFetchStub({
      "POST /deals": (body) => {
        sent.push(body);
        return jsonResponse({ id: "deal-1", name: "New deal" });
      },
    });
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <NewDealAction
            companyId="company-1"
            companyName="Company"
            projectId={projectId}
            onCreated={() => {}}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "New deal" }));
    await user.type(
      screen.getByRole("textbox", { name: "Deal name" }),
      "Partner introduction",
    );
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Owner" }),
      "Colleague",
    );
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Acquisition source" }),
      "Referral",
    );
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Referring partner" }),
      "Referring partner",
    );
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Partner attribution" }),
      "Influenced an existing deal (no commission)",
    );
    await user.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toMatchObject({
      name: "Partner introduction",
      company_id: "company-1",
      project_id: projectId ?? null,
      owner_id: "colleague",
      acquisition_source: "referral",
      partner_company_id: "partner-1",
      partner_attribution: "influenced",
      currency: null,
      amount_minor: null,
    });
  },
);
