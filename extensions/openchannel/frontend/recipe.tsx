import { useT } from "@margince/frontend/app";
import { Callout, SectionHeader } from "@margince/frontend/design-system";
import { type Endpoint, inboundUrl } from "./contract";

// The member's own address, and a request that actually verifies against it.
//
// A `curl` that does not verify is worse than no `curl` at all: the person who
// pastes it learns that the connector is broken rather than that the example
// is, and the refusal they get back is the same opaque 401 a forged request
// gets — deliberately, because a refusal that said which part was wrong would
// enumerate this installation's endpoints. So every part of the command below
// is derived from the same rule the verifier applies: HMAC-SHA256 over
// the scope, slug, ref, unix seconds, nonce and body joined by newlines, hex,
// under the `sha256=` prefix the comparison is made with.

/**
 * The signing scope, which must equal extension.ScopeInbound in
 * backend/pkg/extension/inbound.go VERBATIM — it is one invariant spelled on
 * both sides of a wire, so it is held by backend/gates/inboundsigningrecipe_test.go,
 * which fails in both directions. A member pastes what this file generates; if
 * it and the verifier disagree, the command produces an opaque refusal and
 * there is nothing in the answer to say why.
 */
export const SCOPE_INBOUND = "margince-inbound-v1";

const SCOPE_LITERAL = `'${SCOPE_INBOUND}'`;

/**
 * The signing material, spelled once for the reader and once for the shell.
 *
 * It is `printf` rather than `echo` because `echo` appends a newline on every
 * shell and the verifier hashes the bytes it was given: one trailing byte is
 * the difference between a request that lands and an opaque refusal.
 */
const SIGNED_MATERIAL = `${SCOPE_INBOUND} / <slug> / <ref> / <timestamp> / <nonce> / <body>, newline-separated`;

/**
 * A value made safe to sit inside single quotes in a POSIX shell.
 *
 * The three values interpolated below are a translated placeholder, a URL and
 * a JSON document built from translated copy — so an apostrophe in any
 * catalogue would otherwise end the quoting and leave a command that fails at
 * a shell parse error the reader would blame on this connector.
 */
function shellQuoted(value: string): string {
  return `'${value.replaceAll("'", `'\\''`)}'`;
}

/**
 * The whole command, as one block a person copies.
 *
 * Kept a pure function of its inputs so what a member pastes is exactly what a
 * test holds against the verifier's rule, rather than something assembled
 * across a render.
 */
export function curlRecipe(
  url: string,
  slug: string,
  ref: string,
  secretPlaceholder: string,
  body: string,
): string {
  return [
    `SECRET=${shellQuoted(secretPlaceholder)}`,
    `URL=${shellQuoted(url)}`,
    `SLUG=${shellQuoted(slug)}`,
    `REF=${shellQuoted(ref)}`,
    `BODY=${shellQuoted(body)}`,
    `TS=$(date +%s)`,
    `NONCE=$(openssl rand -hex 16)`,
    // The scope, the slug and the ref are signed alongside the timestamp,
    // nonce and body: without them a request captured on one endpoint is valid
    // on every other, and a message this installation SENDS is a valid message
    // back to it. Newline-separated because no field before the body may
    // contain one, which is what makes the split unambiguous — a separator the
    // nonce could carry would let the same bytes be re-signed under a
    // different replay key.
    `SIG=$(printf '%s\\n%s\\n%s\\n%s\\n%s\\n%s' ${SCOPE_LITERAL} "$SLUG" "$REF" "$TS" "$NONCE" "$BODY" \\`,
    `  | openssl dgst -sha256 -hmac "$SECRET" -r | cut -d' ' -f1)`,
    `curl -i -X POST "$URL" \\`,
    `  -H 'Content-Type: application/json' \\`,
    `  -H "X-Margince-Timestamp: $TS" \\`,
    `  -H "X-Margince-Nonce: $NONCE" \\`,
    `  -H "X-Margince-Signature: sha256=$SIG" \\`,
    `  --data-raw "$BODY"`,
  ].join("\n");
}

export function Recipe({ endpoint }: Readonly<{ endpoint: Endpoint }>) {
  const t = useT();
  // The address is same-origin with the app: the anonymous edge is mounted on
  // the API this screen is already talking to, beside the /v1 surface rather
  // than under it, because /v1 carries the session middleware this edge has
  // none of.
  const origin = globalThis.location.origin;
  const url = inboundUrl(origin, endpoint);
  // A document this connector can land, so a member who pastes the command
  // sees a timeline entry rather than a parked request: `message_id` is what
  // makes a redelivery land nothing, and it is the one member the record
  // builder refuses a body for missing.
  const body = JSON.stringify({
    message_id: "demo-1",
    subject: t("extOpenchannel.recipe.demoSubject"),
    body: t("extOpenchannel.recipe.demoBody"),
    from: {
      email: "sender@example.net",
      name: t("extOpenchannel.recipe.demoFrom"),
    },
  });
  return (
    <>
      <SectionHeader
        title={t("extOpenchannel.recipe.title")}
        sub={t("extOpenchannel.recipe.sub")}
      />
      {/* Said WHERE THE URL IS, not in a tooltip: a member who believes the
          link is the secret will paste it where a secret goes, and the link is
          in every access log and proxy between a sender and this
          installation. */}
      <Callout tone="warn">{t("extOpenchannel.recipe.urlNotSecret")}</Callout>
      <p className="t-caption">{t("extOpenchannel.recipe.urlLabel")}</p>
      <pre className="code-block t-mono" data-testid="openchannel-inbound-url">
        {url}
      </pre>
      <p className="t-caption">
        {t("extOpenchannel.recipe.signedOver", { material: SIGNED_MATERIAL })}
      </p>
      <pre className="code-block t-mono" data-testid="openchannel-curl">
        {curlRecipe(
          url,
          endpoint.slug,
          endpoint.ref,
          t("extOpenchannel.recipe.secretPlaceholder"),
          body,
        )}
      </pre>
    </>
  );
}
