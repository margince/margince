// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { WebForm } from "./webformkinds";

// The signing scope and endpoint name a server form's signature binds. They
// mirror the server's, and gates/frontendwebformsigning_test.go holds them equal.
const SIGNING_SCOPE = "margince-inbound-v1";
const SIGNING_SLUG = "web-form";

// The variable the server snippet reads its secret from, so a pasted example
// never carries the secret itself.
export const SECRET_VARIABLE = "MARGINCE_FORM_SECRET";

export function intakeUrlOf(form: Pick<WebForm, "form_key">, origin: string) {
  return `${origin}/v1/public/forms/${form.form_key}`;
}

// The attribute values a snippet writes are the form's own settings, so each
// is escaped before it lands in HTML the website owner pastes.
function escapeHtml(text: string): string {
  return text
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

// privacyNoticeLines is the notice the form shows under its button at the point
// of collection, and the policy it links to. Either alone still says something.
function privacyNoticeLines(form: WebForm): string[] {
  const notice = form.privacy_notice?.trim() ?? "";
  const policy = form.privacy_notice_url?.trim() ?? "";
  if (notice === "" && policy === "") return [];
  const link =
    policy === "" ? "" : `<a href="${escapeHtml(policy)}">Privacy policy</a>`;
  return [`  <p>${[escapeHtml(notice), link].filter(Boolean).join(" ")}</p>`];
}

// browserSnippet is a plain HTML form with the three spam-check fields. It
// works without the script; the script adds the fill time, the page and an id
// so a double click lands once.
export function browserSnippet(form: WebForm, url: string): string {
  const consent = form.consent_required
    ? [
        "  <label>",
        '    <input type="checkbox" name="consent" required>',
        `    ${escapeHtml(form.consent_wording ?? "")}`,
        "  </label>",
      ]
    : [];
  return [
    `<form action="${escapeHtml(url)}" method="post" data-margince-form>`,
    '  <label>Name <input name="full_name" autocomplete="name"></label>',
    '  <label>Email <input name="email" type="email" required autocomplete="email"></label>',
    '  <label>Company <input name="company" autocomplete="organization"></label>',
    '  <label>Message <textarea name="message"></textarea></label>',
    ...consent,
    '  <input name="_mrg_hp" tabindex="-1" autocomplete="off" aria-hidden="true" style="position:absolute;left:-10000px">',
    '  <input type="hidden" name="_mrg_ts">',
    '  <input type="hidden" name="_mrg_id">',
    '  <input type="hidden" name="page_url">',
    '  <button type="submit">Send</button>',
    ...privacyNoticeLines(form),
    "</form>",
    "<script>",
    '  document.querySelectorAll("form[data-margince-form]").forEach(function (form) {',
    // The first load's time survives a Back, which reloads the page and runs
    // this again: a visitor's second send within seconds still lands.
    '    var started = Date.now(), key = "margince-form-started:" + form.action;',
    "    try {",
    "      var kept = Number(sessionStorage.getItem(key));",
    "      if (kept > 0 && kept < started) started = kept;",
    "      sessionStorage.setItem(key, String(started));",
    "    } catch (unavailable) {",
    "      started = Date.now();",
    "    }",
    "    form.elements._mrg_ts.value = started;",
    "    form.elements.page_url.value = location.href;",
    // A page back from Back may hold the last send's id, and a changed message
    // under it would read as that send again. One page lifetime, one id.
    '    form.elements._mrg_id.value = "";',
    '    window.addEventListener("pageshow", function (event) {',
    '      if (event.persisted) form.elements._mrg_id.value = "";',
    "    });",
    '    form.addEventListener("submit", function () {',
    "      form.elements._mrg_id.value = form.elements._mrg_id.value || crypto.randomUUID();",
    "    });",
    "  });",
    "</script>",
  ].join("\n");
}

// serverSnippet signs one JSON post the way a server form verifies it.
export function serverSnippet(form: WebForm, url: string): string {
  return [
    'import crypto from "node:crypto";',
    "",
    "export async function sendLead(fields) {",
    "  const body = JSON.stringify(fields);",
    "  const timestamp = Math.floor(Date.now() / 1000).toString();",
    '  const nonce = crypto.randomBytes(16).toString("hex");',
    `  const signed = \`${SIGNING_SCOPE}\\n${SIGNING_SLUG}\\n${form.form_key}\\n\${timestamp}\\n\${nonce}\\n\${body}\`;`,
    "  const signature = crypto",
    `    .createHmac("sha256", process.env.${SECRET_VARIABLE})`,
    "    .update(signed)",
    '    .digest("hex");',
    `  const response = await fetch("${url}", {`,
    '    method: "POST",',
    "    headers: {",
    '      "Content-Type": "application/json",',
    '      "X-Margince-Timestamp": timestamp,',
    '      "X-Margince-Nonce": nonce,',
    '      "X-Margince-Signature": "sha256=" + signature,',
    "    },",
    "    body,",
    "  });",
    '  if (!response.ok) throw new Error("Margince refused the lead: " + response.status);',
    "}",
  ].join("\n");
}
