// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import { LocaleProvider } from "../../i18n";
import { InternalMailCaptureCard } from "./internalmailsettings";

const SOURCE = {
  connection_id: "11111111-1111-4111-8111-111111111111",
  owner_user_id: "22222222-2222-4222-8222-222222222222",
  provider: "graph",
  account_label: "operations@demo.test",
  owner_name: "Ana Fischer",
  generation: 3,
};
const POLICY = { enabled: false, sources: [], available_sources: [SOURCE] };
const reply = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function show({
  admin = true,
  refuse = false,
}: {
  admin?: boolean;
  refuse?: boolean;
} = {}) {
  const patches: unknown[] = [];
  const folders: URL[] = [];
  let saved = false;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      const url = new URL(request.url);
      if (url.pathname.endsWith("/capture/settings")) {
        if (request.method === "PATCH") {
          patches.push(await request.json());
          if (refuse)
            return reply(
              {
                code: "permission_denied",
                message: "You cannot change these settings.",
              },
              403,
            );
          saved = true;
        }
        return reply(
          admin
            ? {
                internal_mail: saved
                  ? {
                      ...POLICY,
                      enabled: true,
                      sources: [
                        {
                          ...SOURCE,
                          containers: [],
                          enrolled_at: "2026-01-01T00:00:00Z",
                          enrolled_by: "human:admin",
                        },
                      ],
                    }
                  : POLICY,
              }
            : {},
        );
      }
      if (url.pathname.endsWith("/connectors/graph/containers")) {
        folders.push(url);
        return reply({
          containers: [
            { id: "AbCd", name: "Customer Operations" },
            { id: "abcd", name: "Other Operations" },
          ],
          truncated: false,
        });
      }
      throw new Error(`Unexpected request: ${request.method} ${url.pathname}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <SavePageProvider>
          <InternalMailCaptureCard />
          <PageSaveBar />
        </SavePageProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { patches, folders };
}

it("requires a selected source and saves only an explicit future-capture authorization", async () => {
  const user = userEvent.setup();
  const { patches, folders } = show();
  await user.click(
    await screen.findByRole("checkbox", {
      name: "Enable internal operational mail capture",
    }),
  );
  expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  expect(screen.getByRole("alert")).toHaveTextContent("Select at least one");
  await user.click(
    screen.getByRole("checkbox", { name: /operations@demo.test/ }),
  );
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() =>
    expect(patches).toEqual([
      {
        internal_mail: {
          enabled: true,
          sources: [{ connection_id: SOURCE.connection_id, containers: [] }],
        },
      },
    ]),
  );
  expect(folders).toHaveLength(0);
});

it("lists the selected connection's real folders and preserves provider identifier case", async () => {
  const user = userEvent.setup();
  const { patches, folders } = show();
  await user.click(
    await screen.findByRole("checkbox", { name: /operations@demo.test/ }),
  );
  await user.click(
    screen.getByRole("checkbox", {
      name: "Enable internal operational mail capture",
    }),
  );
  await user.click(screen.getByRole("button", { name: "Choose folders" }));
  await waitFor(() => expect(folders).toHaveLength(1));
  expect(folders[0]?.searchParams.get("connection_id")).toBe(
    SOURCE.connection_id,
  );
  await user.click(
    await screen.findByRole("button", { name: "Choose folders" }),
  );
  await user.click(
    await screen.findByRole("checkbox", { name: "Customer Operations" }),
  );
  await user.click(screen.getByRole("button", { name: "Done" }));
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() =>
    expect(patches).toEqual([
      {
        internal_mail: {
          enabled: true,
          sources: [
            { connection_id: SOURCE.connection_id, containers: ["graph:AbCd"] },
          ],
        },
      },
    ]),
  );
});

it("keeps the source choices after the server refuses a save", async () => {
  const user = userEvent.setup();
  const { patches } = show({ refuse: true });
  await user.click(
    await screen.findByRole("checkbox", { name: /operations@demo.test/ }),
  );
  await user.click(
    screen.getByRole("checkbox", {
      name: "Enable internal operational mail capture",
    }),
  );
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(patches).toHaveLength(1));
  expect(
    screen.getByRole("checkbox", {
      name: "Enable internal operational mail capture",
    }),
  ).toBeChecked();
  expect(
    screen.getByRole("checkbox", { name: /operations@demo.test/ }),
  ).toBeChecked();
  expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
});

it("shows no source controls when the server omits administrator-only settings", async () => {
  show({ admin: false });
  await waitFor(() =>
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument(),
  );
  expect(
    screen.queryByText("Internal operational mail"),
  ).not.toBeInTheDocument();
});
