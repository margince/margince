import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { App } from "./App";
import { LocaleProvider } from "./i18n";

const meta: Meta = {
  title: "Shell/Application",
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj;

function installAppStub() {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  // Land on a list screen whose empty state needs nothing but the page-shaped
  // fallback below, rather than the Brief, whose dashboard queries want richer
  // fixtures than this shell smoke test cares about. A screen the router does
  // not know would draw Not found under the name of this story.
  globalThis.location.hash = "#/contacts";
  globalThis.fetch = (async (input: Request | string | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.endsWith("/v1/me")) {
      return new Response(
        JSON.stringify({
          user: { email: "ada@acme.test" },
          roles: ["admin"],
          teams: [],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }
    // The shell's brand block draws the installation's OWN company, read off
    // GET /company. The list-shaped fallback below answers that read with a
    // page envelope, which is not a 404 — so the app reads the installation as
    // described, the block renders it, and the name it draws with is missing.
    // A story of the authenticated app is a story of a described installation,
    // so it says which company that is.
    if (url.endsWith("/v1/company")) {
      return new Response(
        JSON.stringify({
          company_id: "018f3a1b-0000-7000-8000-0000000000a1",
          display_name: "Acme Freight",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }
    // The record zone reads the installation's timezone, and the contract makes
    // that field REQUIRED — so a settled answer without one is a server this
    // bundle does not match, which the app reports rather than papers over. The
    // list-shaped fallback below is exactly that answer, so this one is routed.
    if (url.endsWith("/v1/installation/settings")) {
      return new Response(JSON.stringify({ timezone: "Europe/Berlin" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }
    // The installation's own company, for the same reason: the brand block
    // draws a monogram from `display_name`, and the list-shaped fallback is an
    // OBJECT — truthy, so the shell reads it as a saved profile and then splits
    // a name that is not there. A 404 is the other real answer here and the
    // wrong one for this story: it is the signal that nobody has onboarded, and
    // the gate would send this frame to the wizard.
    if (url.endsWith("/v1/company")) {
      return new Response(
        JSON.stringify({
          company_id: "01a00000-0000-7000-8000-0000000000ac",
          display_name: "Acme Fördertechnik",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }
    return new Response(
      JSON.stringify({
        data: [],
        page: { next_cursor: null, has_more: false },
      }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    );
  }) as typeof fetch;
}

export const AuthenticatedHome: Story = {
  render: () => {
    installAppStub();
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    return (
      <QueryClientProvider client={client}>
        <LocaleProvider>
          <App />
        </LocaleProvider>
      </QueryClientProvider>
    );
  },
};
