import { splitEmailBody } from "../format/emailtext";
import { useT } from "../i18n";
import { Disclosure } from "./disclosure";
import { type MessageLanguage, ReaderRendering } from "./readerrendering";
import "./emailtext.css";

// Both the full reader and preview preserve the author's paragraphs and sign-off.
export function EmailText({
  body,
  subject,
}: Readonly<{
  body: string;
  /** The mail's own subject, which says whether it forwards. */
  subject: string | null | undefined;
}>) {
  const t = useT();
  // Envelope fields are rendered by the reader; the body splitter can also
  // identify a quoted envelope header, which is not part of the message text.
  const parts = splitEmailBody(body, subject ?? "");
  return (
    <div className="emailtext">
      <p className="emailtext__main">{parts.main}</p>
      {parts.tail === "signature" && (
        <p className="emailtext__signoff">{parts.trimmed}</p>
      )}
      {parts.tail === "quote" && (
        <Disclosure
          className="emailtext__quoted"
          summary={t("email.detail.showQuoted")}
        >
          <p>{parts.trimmed}</p>
        </Disclosure>
      )}
    </div>
  );
}

// A received mail as the reader reads it. A mail with no text has nothing to
// translate, so it says so instead of offering the press.
export function EmailReading({
  activityId,
  language,
  body,
  subject,
  textUnreadable = false,
}: Readonly<{
  activityId: string;
  language: MessageLanguage | null | undefined;
  body: string | null | undefined;
  /** The mail's own subject, which says whether it forwards. */
  subject: string | null | undefined;
  /** The reader already says why the text is missing. */
  textUnreadable?: boolean;
}>) {
  const t = useT();
  if (!body?.trim()) {
    return textUnreadable ? null : (
      <p className="emailtext__none t-caption">{t("email.thread.noText")}</p>
    );
  }
  return (
    <ReaderRendering activityId={activityId} language={language}>
      <EmailText body={body} subject={subject} />
    </ReaderRendering>
  );
}
