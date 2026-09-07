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
  // Land on a screen with a known-good empty-state story rather than Brief,
  // whose dashboard queries need richer fixtures than this shell smoke test cares about.
  globalThis.location.hash = "#/products";
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
          organization_id: "01a00000-0000-7000-8000-0000000000ac",
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
