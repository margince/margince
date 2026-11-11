/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { IntroDrawer } from "./introdrawer";

const route = {
  route_id: "direct:replacement",
  route_type: "direct",
  via_user_id: "replacement",
  via_display_name: "New colleague",
  strength_bucket: "strong",
  availability: "available",
  evidence: {
    interactions_90d: 2,
    inbound_90d: 1,
    outbound_90d: 1,
    two_way: true,
  },
} satisfies NonNullable<Parameters<typeof IntroDrawer>[0]["route"]>;

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("reviews a fresh ask and keeps its words when the replacement is refused", async () => {
  const user = userEvent.setup();
  const writes: { path: string; body: unknown }[] = [];
  const closed = vi.fn();
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      if (!(input instanceof Request))
        throw new Error("expected the API transport's Request");
      if (input.method === "POST") {
        writes.push({
          path: new URL(input.url).pathname,
          body: await input.json(),
        });
        return new Response(
          JSON.stringify({ title: "Choose an active colleague", status: 422 }),
          {
            status: 422,
            headers: { "content-type": "application/problem+json" },
          },
        );
      }
      return new Response(JSON.stringify({}), {
        headers: { "content-type": "application/json" },
      });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <IntroDrawer
        contactId="contact"
        contactName="Dana"
        route={route}
        replacement={{ id: "original", version: 2 }}
        open
        onClose={closed}
      />
    </QueryClientProvider>,
  );
  const submit = screen.getByRole("button", { name: "Request introduction" });
  expect(submit).toBeDisabled();
  await user.type(
    screen.getByRole("textbox", { name: "Why are you replacing this ask?" }),
    "The original colleague left.",
  );
  await user.type(
    screen.getByRole("textbox", { name: "Reason for request" }),
    "Please consider this new route.",
  );
  await user.click(submit);
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toEqual({
    path: "/v1/intro-requests/original/replace",
    body: {
      version: 2,
      reason: "The original colleague left.",
      request: {
        introducer_user_id: "replacement",
        route_type: "direct",
        internal_reason: "Please consider this new route.",
        note_generated_by: "human",
        note_ai_generated: false,
        name_drop_allowed: false,
        fallback_policy: "none",
      },
    },
  });
  expect(await screen.findByRole("alert")).toBeVisible();
  expect(
    screen.getByRole("textbox", { name: "Reason for request" }),
  ).toHaveValue("Please consider this new route.");
  expect(closed).not.toHaveBeenCalled();
});
