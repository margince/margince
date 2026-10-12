import { useQuery } from "@tanstack/react-query";
import { useMe } from "./common";
import "./signaturehtml.css";

// What {logo} points at in the markup: the image the message carries inside
// itself, which a page outside that message cannot resolve.
const EMBEDDED_LOGO_SRC = 'src="cid:signature-logo@margince"';

/**
 * A rendered signature, as the recipient's mail client draws it.
 *
 * The markup is the server's own: the send path sanitized it, and this shows
 * that markup. It still renders in a sandboxed frame with no permissions, so
 * nothing in it can run or reach this page. The embedded logo is swapped for
 * the workspace logo's bytes, since the frame can neither resolve the message's
 * content id nor send this session's cookie.
 */
export function SignatureHtml({
  html,
  title,
}: Readonly<{ html: string; title: string }>) {
  const showsLogo = html.includes(EMBEDDED_LOGO_SRC);
  const logo = useLogoDataUrl(showsLogo);
  const shown =
    showsLogo && logo !== undefined
      ? html.replaceAll(EMBEDDED_LOGO_SRC, `src="${logo}"`)
      : html;
  return (
    <iframe
      className="signature-html"
      title={title}
      sandbox=""
      srcDoc={`<!doctype html><meta charset="utf-8"><body style="margin:0;font:14px/1.4 sans-serif">${shown}</body>`}
    />
  );
}

// The workspace logo as a data URL, read only when a signature shows it.
function useLogoDataUrl(wanted: boolean): string | undefined {
  const logoUrl = useMe().data?.installation_brand?.logo_url;
  const query = useQuery({
    queryKey: ["signature-logo", logoUrl],
    enabled: wanted && logoUrl !== undefined,
    queryFn: async () => {
      // contract-fetch:allow image bytes: the generated client decodes JSON only, and this reads the logo's binary body
      const response = await fetch(logoUrl ?? "", {
        credentials: "same-origin",
      });
      if (!response.ok) {
        throw new Error(`the workspace logo answered ${response.status}`);
      }
      return dataUrl(await response.blob());
    },
    staleTime: Number.POSITIVE_INFINITY,
  });
  return query.data;
}

function dataUrl(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () =>
      typeof reader.result === "string"
        ? resolve(reader.result)
        : reject(new Error("the workspace logo did not read as a data URL"));
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(blob);
  });
}
