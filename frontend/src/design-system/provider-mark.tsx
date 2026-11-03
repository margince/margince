import { KeyRound } from "lucide-react";
import "./provider-mark.css";

/*
 * SANCTIONED LITERALS — third-party brand marks.
 *
 * Google's, Microsoft's and LinkedIn's own colours, identifying their sign-in
 * (or connect) providers. These are not design-system colours and must not be
 * tokenised: a provider mark rendered in Ledger Green is a wrong mark, and
 * re-tinting another company's logo is not ours to do. This file is the ONE
 * named exclusion in both colour gates — `scripts/check-ds-purity.sh` and the
 * `keeps literal colours in tokens.css only` case in
 * `design-system/conformance.test.ts` — each carrying that reason.
 *
 * Lucide ships no brand logos, so the marks are inline SVG. They live in the
 * design system rather than in the auth screen because the mailbox-connect step
 * needs the same two (Google, Microsoft) and the network-connect step needs
 * LinkedIn: a second copy of any of these palettes is a second place for the
 * rule above to quietly stop holding.
 *
 * Webflow's and Typeform's marks come from the CC0 sets Simple Icons and SVG
 * Logos.
 *
 * The mark is chosen from the provider's `key`, which is the ONLY part of a
 * provider this frontend decides. The button's label is the installation's own
 * string, served by `/auth/capabilities` — see `ProviderButtons` in
 * `features/sign-in/auth-providers.tsx`.
 */

/**
 * The brand word for a key this frontend recognises, or null.
 *
 * Same knowledge as the mark, so it lives beside it: recognising `google` well
 * enough to draw its logo is recognising it well enough to name it. A narrow
 * layout can then show "Google" where the installation's full label
 * ("Continue with Google") does not fit on one line.
 *
 * Two things this deliberately is NOT. It is not a translation — these are proper
 * nouns and stay identical in every locale, the same rule the served label
 * follows (§11.5). And it is not derived from the label by trimming a leading
 * "Continue with": the next installation's label is "Sign in with Okta" or a
 * German sentence, and a trim would mangle both. An unrecognised key returns
 * null, and the caller shows the server's own words — which are the only thing
 * that can be right for a provider this file has never heard of.
 */
export function providerBrandName(providerKey: string): string | null {
  if (providerKey === "google") {
    return "Google";
  }
  if (providerKey === "microsoft") {
    return "Microsoft";
  }
  if (providerKey === "surfe") {
    return "Surfe";
  }
  return null;
}

/**
 * An unrecognised key still gets a mark.
 *
 * `AuthCapabilities.oidc_providers[].key` is an open string in the contract, so
 * a self-hosted installation can name a provider this frontend has never heard
 * of. Rendering nothing for it would hide a working sign-in path; a neutral key
 * icon says "a provider, not one I have a logo for", which is the honest answer.
 */
export function ProviderMark({
  providerKey,
}: Readonly<{ providerKey: string }>) {
  if (providerKey === "google") {
    return (
      <svg
        className="provider-mark"
        viewBox="0 0 24 24"
        aria-hidden="true"
        focusable="false"
      >
        <path
          fill="#4285F4"
          d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.76h3.56c2.08-1.92 3.28-4.74 3.28-8.09Z"
        />
        <path
          fill="#34A853"
          d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.56-2.76c-.98.66-2.24 1.06-3.72 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84A11 11 0 0 0 12 23Z"
        />
        <path
          fill="#FBBC05"
          d="M5.84 14.11a6.6 6.6 0 0 1 0-4.22V7.05H2.18a11 11 0 0 0 0 9.9l3.66-2.84Z"
        />
        <path
          fill="#EA4335"
          d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1A11 11 0 0 0 2.18 7.05l3.66 2.84C6.71 7.31 9.14 5.38 12 5.38Z"
        />
      </svg>
    );
  }
  if (providerKey === "microsoft") {
    return (
      <svg
        className="provider-mark"
        viewBox="0 0 24 24"
        aria-hidden="true"
        focusable="false"
      >
        <path fill="#F25022" d="M2 2h9.5v9.5H2z" />
        <path fill="#7FBA00" d="M12.5 2H22v9.5h-9.5z" />
        <path fill="#00A4EF" d="M2 12.5h9.5V22H2z" />
        <path fill="#FFB900" d="M12.5 12.5H22V22h-9.5z" />
      </svg>
    );
  }
  if (providerKey === "linkedin") {
    return (
      <svg
        className="provider-mark"
        viewBox="0 0 24 24"
        aria-hidden="true"
        focusable="false"
      >
        {/* LinkedIn's own blue, not the design system's — the badge shape
            carries the fill, so it reads the same against either theme's
            page background rather than depending on currentColor. */}
        <rect width="24" height="24" rx="4" fill="#0A66C2" />
        <path
          fill="#fff"
          d="M7.12 9.4H4.4V19.4h2.72zm-1.36-4.36a1.58 1.58 0 1 0 0 3.16 1.58 1.58 0 0 0 0-3.16M19.6 19.4v-5.5c0-2.95-1.57-4.32-3.67-4.32a3.17 3.17 0 0 0-2.87 1.58V9.4H10.35c.03.72 0 10 0 10h2.71v-5.58c0-.3.02-.6.11-.81a1.79 1.79 0 0 1 1.63-1.2c1.15 0 1.6.87 1.6 2.16v5.43z"
        />
      </svg>
    );
  }
  if (providerKey === "webflow") {
    return (
      <svg
        className="provider-mark"
        viewBox="0 0 24 24"
        aria-hidden="true"
        focusable="false"
      >
        <path
          fill="#146EF5"
          d="m24 4.515-7.658 14.97H9.149l3.205-6.204h-.144C9.566 16.713 5.621 18.973 0 19.485v-6.118s3.596-.213 5.71-2.435H0V4.515h6.417v5.278l.144-.001 2.622-5.277h4.854v5.244h.144l2.72-5.244H24Z"
        />
      </svg>
    );
  }
  if (providerKey === "typeform") {
    return (
      <svg
        className="provider-mark"
        viewBox="0 0 256 257"
        aria-hidden="true"
        focusable="false"
      >
        {/* Typeform's mark is one ink, black on light and white on dark, so it
            takes the text colour rather than a literal that vanishes on the
            dark theme. */}
        <path
          fill="currentColor"
          d="M121.24.474c33.019-2.718 56.734 6.045 82.25 31.547c27.549 27.532 45.864 63.212 50.8 101.218c5.492 42.266-2.062 73.807-25.326 96.898c-22.247 22.08-54.74 29.364-98.146 25.735l-1.797-.158c-36.963-3.398-63.107-15.517-88.438-41.171C13.526 187.14-.732 156.026.029 123.37c.372-15.987 4.901-29.612 13.635-42.98c5.726-8.765 11.54-15.602 23.704-28.14l4.06-4.158l4.474-4.536C74.73 14.58 93.707 2.741 121.24.475m80.288 33.51C176.586 9.057 153.672.59 121.468 3.241c-27.14 2.234-45.78 14.091-74.95 43.635l-2.11 2.141c-14.21 14.448-20.87 21.835-26.738 30.378l-.854 1.26l-.828 1.252c-8.461 12.95-12.824 26.078-13.184 41.528c-.743 31.846 13.194 62.258 39.754 89.157c25.267 25.59 51.225 37.4 88.49 40.514l1.85.148c41.744 3.21 72.867-4.002 94.11-25.087c22.57-22.402 29.92-53.088 24.53-94.57c-4.855-37.382-22.884-72.503-50.01-99.613M166.984 93.39v13.56h-28.797v77.712h-14.345V106.95H95.045V93.39z"
        />
      </svg>
    );
  }
  if (providerKey === "surfe") {
    return (
      <svg
        className="provider-mark"
        viewBox="0 0 24 24"
        aria-hidden="true"
        focusable="false"
      >
        {/* Surfe's own dark, on the same badge shape the others use.
            Deliberately their INITIAL rather than a redrawing of their logo:
            the marks above are each vendor's published logo geometry, and
            approximating one from memory would put a wrong mark in their
            colours — worse than a plain letter that claims to be nothing more
            than a letter. Replace it if Surfe publish usage terms and artwork. */}
        <rect width="24" height="24" rx="4" fill="#1A1A2E" />
        <text
          x="12"
          y="17"
          textAnchor="middle"
          fill="#fff"
          fontSize="14"
          /* The weight comes from the token, and it has to come through
             `style`: `font-weight` here is an SVG presentation ATTRIBUTE, and
             an attribute value is not a CSS value — a var() in one resolves to
             nothing and the letter renders at the default weight. The property
             does take it. `fontSize` stays an attribute on purpose: it is a
             length in this mark's own 24-unit viewBox, not document type. */
          style={{ fontWeight: "var(--fontWeightBold)" }}
          fontFamily="system-ui, sans-serif"
        >
          S
        </text>
      </svg>
    );
  }
  return <KeyRound className="provider-mark" aria-hidden="true" />;
}
