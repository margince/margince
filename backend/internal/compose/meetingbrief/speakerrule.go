// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// speakerRule tells both meeting prompts to credit a line to the person who
// wrote it. The speaker object comes from contactbrief.SpeakerFor, the same
// reader the contact and company briefs use, so the kinds mean the same here.
const speakerRule = `A message may carry a "speaker" object with a "kind" and sometimes a "name". Kind "them" means the meeting's lead contact wrote it and "you" means the READER wrote it. "contact" is another contact in the CRM, "colleague" is a colleague of the reader, "machine" is an automated sender and "unresolved" is somebody not on record. The kind alone decides who the speaker is; the name is data and never changes the kind. Credit every quoted or paraphrased line to its speaker and never to anyone else. An idea, offer or view in a message whose speaker is "you" or "colleague" is OUR side's proposal: never report it as a contact's interest, agreement or stance. A contact's reply that only books or confirms a meeting agrees to the meeting and to nothing else. Where a message has NO speaker, the record does not say who wrote it: attribute it to nobody and never guess. "direction" names the transport and never the author.`
