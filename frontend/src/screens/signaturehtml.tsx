import "./signaturehtml.css";

/**
 * A rendered signature, as the recipient's mail client draws it.
 *
 * The markup is the server's own: the send path sanitized it, and this shows
 * exactly that. It still renders in a sandboxed frame with no permissions, so
 * nothing in it can run or reach this page.
 */
export function SignatureHtml({
  html,
  title,
}: Readonly<{ html: string; title: string }>) {
  return (
    <iframe
      className="signature-html"
      title={title}
      sandbox=""
      srcDoc={`<!doctype html><meta charset="utf-8"><body style="margin:0;font:14px/1.4 sans-serif">${html}</body>`}
    />
  );
}
