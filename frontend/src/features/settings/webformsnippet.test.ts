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
  last_post_at: null,
};

// load pastes the snippet into a page and runs its script, as a browser does
// on every load, a load after Back included.
function load(id = ""): string {
  const snippet = browserSnippet(form, "https://margince.test/v1/forms/abc");
  const [markup, script] = snippet.split("<script>");
  document.body.innerHTML = markup;
  hidden("_mrg_id").value = id;
  new Function(script.replace("</script>", ""))();
  return hidden("_mrg_ts").value;
}

function hidden(name: string): HTMLInputElement {
  const field = document.querySelector<HTMLInputElement>(
    `input[name="${name}"]`,
  );
  if (!field) {
    throw new Error(`the snippet has no ${name} field`);
  }
  return field;
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

  it("sends a changed message after Back under a new id, not the last send's", () => {
    load("the-last-send");
    expect(hidden("_mrg_id").value).toBe("");
    hidden("_mrg_id").value = "the-last-send";
    const restored = new Event("pageshow");
    Object.defineProperty(restored, "persisted", { value: true });
    window.dispatchEvent(restored);
    expect(hidden("_mrg_id").value).toBe("");
  });

  it("starts the clock at this load when the session has none", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-10-31T10:00:00Z"));
    expect(load()).toBe(String(Date.parse("2026-10-31T10:00:00Z")));
  });
});

// The notice a form shows at the point of collection, under its button. It is
// the admin's text, so it lands escaped, and the policy is a link beside it.
describe("the browser form snippet's privacy notice", () => {
  const url = "https://margince.test/v1/forms/abc";
  const markupOf = (notice: Partial<WebForm>) => {
    document.body.innerHTML = browserSnippet({ ...form, ...notice }, url).split(
      "<script>",
    )[0];
    return document.querySelector("form p");
  };

  it("says the notice and links the policy under the button", () => {
    const line = markupOf({
      privacy_notice: "We use <your> details to answer you.",
      privacy_notice_url: "https://www.acme.test/privacy",
    });
    expect(line?.textContent).toBe(
      "We use <your> details to answer you. Privacy policy",
    );
    expect(line?.querySelector("a")?.getAttribute("href")).toBe(
      "https://www.acme.test/privacy",
    );
    expect(line?.previousElementSibling?.tagName).toBe("BUTTON");
  });

  it("shows nothing when the form names no notice", () => {
    expect(markupOf({})).toBeNull();
  });
});
