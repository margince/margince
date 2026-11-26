import { NewContactAction } from "../../components/newcontact";
import { QuickCaptureAction } from "../../components/quickcapture";

// The Contacts list's typed creates; the list adds the vCard import after them.
export function ContactCreateActions({
  startCreating = false,
  refusalId,
}: Readonly<{
  startCreating?: boolean;
  // The list's one sentence saying why a refused create is refused.
  refusalId?: string;
}>) {
  return (
    <>
      <QuickCaptureAction refusalId={refusalId} />
      <NewContactAction
        listCreate={{ open: startCreating }}
        refusalId={refusalId}
      />
    </>
  );
}
