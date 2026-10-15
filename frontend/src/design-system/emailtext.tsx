import { splitEmailBody } from "../format/emailtext";
import { useT } from "../i18n";
import { Disclosure } from "./disclosure";
import "./emailtext.css";

// Both the full reader and preview preserve the author's paragraphs and sign-off.
export function EmailText({ body }: Readonly<{ body: string }>) {
  const t = useT();
  // Envelope fields are rendered by the reader; the body splitter can also
  // identify a quoted envelope header, which is not part of the message text.
  const parts = splitEmailBody(body);
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
