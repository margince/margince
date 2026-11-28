/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import { BusinessAccountsPanel } from "./connectors.business";
import type { BusinessProvider } from "./connectors.businessapi";

const team = { id: "018f3a1b-0000-7000-8000-0000000000e1", name: "Sales desk" };
function mount(
  provider: BusinessProvider,
  options: {
    denied?: boolean;
    createOnly?: boolean;
    failure?: boolean;
    notConfigured?: boolean;
    accounts?: unknown[];
  } = {},
) {
  const writes: unknown[] = [];
  const reads: string[] = [];
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      const json = (body: unknown, status = 200) =>
        new Response(JSON.stringify(body), {
          status,
          headers: { "Content-Type": "application/json" },
        });
      if (path.endsWith("/me"))
        return json(
          meFixture({
            allow: options.denied
              ? {}
              : {
                  channel_connection: options.createOnly
                    ? ["create"]
                    : ["read", "create", "update", "delete"],
                },
          }),
        );
      if (path.endsWith("/teams")) return json({ data: [team], page: {} });
      if (request.method === "GET") {
        reads.push(path);
        if (options.notConfigured)
          return json(
            {
              status: 503,
              code: "channel_credentials_not_configured",
              title: "Unavailable",
            },
            503,
          );
        return json({ data: options.accounts ?? [] });
      }
      writes.push(
        request.method === "DELETE"
          ? { method: "DELETE", path }
          : await request.json(),
      );
      if (request.method === "DELETE")
        return new Response(null, { status: 204 });
      if (options.failure)
        return json({ status: 409, code: "conflict", title: "Conflict" }, 409);
      return json(
        provider === "whatsapp"
          ? { connection: {}, verifyToken: "demo-verification-once" }
          : { id: "demo-oa" },
        201,
      );
    }),
  );
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <BusinessAccountsPanel provider={provider} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return Object.assign(writes, { client, reads });
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

async function fillAccount(
  user: ReturnType<typeof userEvent.setup>,
  provider: BusinessProvider,
) {
  await user.click(
    await screen.findByRole("button", {
      name: en["connectors.business.connect"],
    }),
  );
  const fields =
    provider === "whatsapp"
      ? [
          en["connectors.business.wabaId"],
          en["connectors.business.phoneNumberId"],
          en["connectors.business.phone"],
          en["connectors.business.accessToken"],
          en["connectors.business.appSecret"],
        ]
      : [
          en["connectors.business.oaId"],
          en["connectors.business.appId"],
          en["connectors.business.name"],
          en["connectors.business.accessToken"],
          en["connectors.business.refreshToken"],
          en["connectors.business.appSecret"],
          en["connectors.business.oaSecretKey"],
        ];
  for (const [index, label] of fields.entries())
    await user.type(
      screen.getByLabelText(label, { exact: false }),
      `demo-${index}`,
    );
  await pickOption(
    user,
    screen.getByRole("combobox", { name: en["sharedMailboxes.readerTeam"] }),
    "Sales desk",
  );
}

describe("business account setup", () => {
  it.each<BusinessProvider>(["whatsapp", "zalo-oa"])(
    "connects %s through its API and preserves the one-time WhatsApp token",
    async (provider) => {
      const writes = mount(provider);
      const user = userEvent.setup({ delay: null });
      await fillAccount(user, provider);
      await user.click(
        within(screen.getByRole("dialog")).getByRole("button", {
          name: en["connectors.business.connect"],
        }),
      );
      await waitFor(() => expect(writes).toHaveLength(1));
      expect(writes[0]).toEqual(
        provider === "whatsapp"
          ? {
              readerTeamId: team.id,
              wabaId: "demo-0",
              phoneNumberId: "demo-1",
              displayPhoneNumber: "demo-2",
              accessToken: "demo-3",
              appSecret: "demo-4",
            }
          : {
              readerTeamId: team.id,
              oaId: "demo-0",
              appId: "demo-1",
              displayName: "demo-2",
              accessToken: "demo-3",
              refreshToken: "demo-4",
              appSecret: "demo-5",
              oaSecretKey: "demo-6",
            },
      );
      if (provider === "whatsapp") {
        expect(
          await screen.findByRole("textbox", {
            name: en["connectors.business.verifyToken"],
          }),
        ).toHaveTextContent("demo-verification-once");
        expect(
          screen.queryByLabelText(en["connectors.business.accessToken"], {
            exact: false,
          }),
        ).not.toBeInTheDocument();
        await user.keyboard("{Escape}");
        const discard = await screen.findByRole("dialog", {
          name: en["unsaved.title"],
        });
        expect(discard).toHaveTextContent(
          en["connectors.business.tokenDiscard"],
        );
        await user.click(
          within(discard).getByRole("button", { name: "Cancel" }),
        );
        expect(
          screen.getByRole("textbox", {
            name: en["connectors.business.verifyToken"],
          }),
        ).toHaveTextContent("demo-verification-once");
        await user.click(
          screen.getByRole("button", { name: en["connectors.business.done"] }),
        );
      } else
        await waitFor(() =>
          expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
        );
    },
  );
  it("keeps credentials in the form when connecting is refused", async () => {
    const writes = mount("whatsapp", { failure: true });
    const user = userEvent.setup({ delay: null });
    await fillAccount(user, "whatsapp");
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: en["connectors.business.connect"],
      }),
    );
    await waitFor(() => expect(writes).toHaveLength(1));
    expect(
      screen.getByLabelText(en["connectors.business.accessToken"], {
        exact: false,
      }),
    ).toHaveValue("demo-3");
    expect(await screen.findByRole("alert")).toBeInTheDocument();
  });
  it.each(["none", "create only"])(
    "hides setup with %s grants after the session has loaded",
    async (grants) => {
      const writes = mount("whatsapp");
      expect(
        await screen.findByRole("button", {
          name: en["connectors.business.connect"],
        }),
      ).toBeInTheDocument();
      await act(async () => {
        writes.client.setQueryData(
          ["me"],
          meFixture({
            allow: grants === "none" ? {} : { channel_connection: ["create"] },
          }),
        );
      });
      await waitFor(() =>
        expect(
          screen.queryByRole("button", {
            name: en["connectors.business.connect"],
          }),
        ).not.toBeInTheDocument(),
      );
    },
  );
  it("explains missing credentials without offering a connect that must fail", async () => {
    mount("whatsapp", { notConfigured: true });
    expect(
      await screen.findByText(en["connectors.telegramNotConfiguredTitle"]),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: en["connectors.business.connect"] }),
    ).not.toBeInTheDocument();
  });
  it("keeps an account status from a newer server neutral", async () => {
    mount("whatsapp", {
      accounts: [
        {
          id: "wa-future",
          displayPhoneNumber: "+49 170 1234567",
          status: "awaiting_review",
          readerTeamId: team.id,
          mailVisibility: "held",
        },
      ],
    });
    expect(
      await screen.findByText(en["common.unknownValue"]),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", {
        name: en["connectors.tgbiz.changeReaders"],
      }),
    ).not.toBeInTheDocument();
  });
  it("changes readers through the provider's visibility route", async () => {
    const writes = mount("whatsapp", {
      accounts: [
        {
          id: "wa-demo",
          displayPhoneNumber: "+49 170 1234567",
          status: "connected",
          readerTeamId: team.id,
          mailVisibility: "held",
        },
      ],
    });
    const user = userEvent.setup({ delay: null });
    await user.click(
      await screen.findByRole("button", {
        name: en["connectors.tgbiz.changeReaders"],
      }),
    );
    await user.click(screen.getByRole("radio", { name: /Shared/ }));
    await user.click(
      screen.getByRole("button", {
        name: en["connectors.tgbiz.visibilityConfirm"],
      }),
    );
    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]).toEqual({ mail_visibility: "shared", confirm: true });
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
  });
  it("asks explicitly before disconnecting an account whose access expired", async () => {
    const writes = mount("zalo-oa", {
      accounts: [
        {
          id: "oa-demo",
          displayName: "Customer desk",
          status: "reauth_required",
          readerTeamId: team.id,
          mailVisibility: "shared",
        },
      ],
    });
    const user = userEvent.setup({ delay: null });
    expect(
      await screen.findByText(en["connectors.business.reauth"]),
    ).toBeInTheDocument();
    await user.click(
      screen.getByRole("button", {
        name: /Actions.*Customer desk|Customer desk.*actions/i,
      }),
    );
    await user.click(
      screen.getByRole("button", { name: en["connectors.disconnect"] }),
    );
    expect(screen.getByRole("dialog")).toHaveTextContent(
      en["connectors.business.disconnectHelp"],
    );
    expect(writes).toHaveLength(0);
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: en["connectors.disconnect"],
      }),
    );
    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]).toEqual({
      method: "DELETE",
      path: expect.stringMatching(/channel-connections\/zalo-oa\/oa-demo$/),
    });
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
  });
});
