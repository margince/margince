import { NewContactAction } from "../../components/newcontact";
import { QuickCaptureAction } from "../../components/quickcapture";

// The Contacts list's typed creates; the list adds the vCard import after them.
export function ContactCreateActions({
  startCreating = false,
}: Readonly<{ startCreating?: boolean }>) {
  return (
    <>
      <QuickCaptureAction />
      <NewContactAction listCreate={{ open: startCreating }} />
    </>
  );
}
