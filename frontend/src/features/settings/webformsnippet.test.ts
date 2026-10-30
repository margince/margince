/** @vitest-environment happy-dom */
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WebForm } from "./webformkinds";
import { browserSnippet } from "./webformsnippet";

const form: WebForm = {
  id: "wf-1",
  name: "Contact page",
  form_key: "abcdefghijklmnopqrstuvwxyz012345",
  sender: "browser",
  allowed_origins: ["https://www.acme.test"],
  lead_origin: "website",
  owner_id: null,
  field_map: {},
  consent_required: false,
  consent_policy_version: null,
  consent_wording: null,
  redirect_url: null,
  success_message: null,
  state: "active",
  secret_configured: false,
  version: 3,
  created_at: "2026-09-01T09:00:00Z",
  updated_at: "2026-09-01T09:00:00Z",
  archived_at: null,
};

// load pastes the snippet into a page and runs its script, as a browser does
// on every load, a load after Back included.
function load(): string {
  const snippet = browserSnippet(form, "https://margince.test/v1/forms/abc");
  const [markup, script] = snippet.split("<script>");
  document.body.innerHTML = markup;
  new Function(script.replace("</script>", ""))();
  const field = document.querySelector<HTMLInputElement>(
    'input[name="_mrg_ts"]',
  );
  return field?.value ?? "";
}

afterEach(() => {
  vi.useRealTimers();
  sessionStorage.clear();
  document.body.innerHTML = "";
});

describe("the browser form snippet's fill time", () => {
  it("keeps the page's first load across a Back, so a visitor's quick second send lands", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-10-31T10:00:00Z"));
    const first = load();
    vi.setSystemTime(new Date("2026-10-31T10:00:03Z"));
    expect(load()).toBe(first);
  });

  it("starts the clock at this load when the session has none", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-10-31T10:00:00Z"));
    expect(load()).toBe(String(Date.parse("2026-10-31T10:00:00Z")));
  });
});
