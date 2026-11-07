import { NewContactAction } from "../../components/newcontact";
import { QuickCaptureAction } from "../../components/quickcapture";

// The Contacts list's typed creates; the list adds the vCard import after them.
export function ContactCreateActions() {
  return (
    <>
      <QuickCaptureAction />
      <NewContactAction />
    </>
  );
}
