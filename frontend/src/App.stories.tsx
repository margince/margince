import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { App } from "./App";
import { LocaleProvider } from "./i18n";
import { emptyListAnswer } from "./testing/story-utils";

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
    // The shell's brand block draws the installation's own company, read off
    // GET /company. The list-shaped fallback below answers with a page
    // envelope, not a 204. The app then reads the installation as described,
    // and the name the block draws with is missing. A story of the
    // authenticated app is a story of a described installation, so it routes
    // that company.
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
    // that field required. A settled answer without one is a server this bundle
    // does not match, and the app reports it. The list-shaped fallback below is
    // such an answer, so this read is routed.
    if (url.endsWith("/v1/installation/settings")) {
      return new Response(JSON.stringify({ timezone: "Europe/Berlin" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }
    // The installation's own company, for the same reason: the brand block
    // draws a monogram from `display_name`. The list-shaped fallback is a
    // truthy object, so the shell reads it as a saved profile and splits a name
    // that is not there. A 204 is wrong for this story: it signals that nobody
    // has onboarded, and the gate would send this frame to the wizard.
    if (url.endsWith("/v1/company")) {
      return new Response(
        JSON.stringify({
          company_id: "01a00000-0000-7000-8000-0000000000ac",
          display_name: "Acme Fördertechnik",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }
    return emptyListAnswer(input);
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
