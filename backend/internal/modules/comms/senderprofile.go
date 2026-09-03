// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// Which gates a delivery answers to, decided by who is sending it.
//
// The dispatcher's gate chain was written for one kind of sender: a human,
// through their own connected mailbox. Four of its questions are about that
// human — is their OAuth grant still scoped to send, is their seat still live
// and mutation-capable, can their provider carry these files, and is their
// mailbox being paced. A message the INSTALLATION sends has none of those
// things, and asking anyway would refuse every one of them for the absence of
// something it was never meant to have.
//
// So the chain stays one chain and the profile says which links apply. The
// alternative — a second dispatcher for controller mail — is the shape this
// whole change exists to avoid: a second park path, a second retry ladder and
// a second bounce story, drifting apart from the first.
//
// What is NOT optional here: authorization and the retry-ladder bound. A
// controller message is still put to the engine and still gets a decision row
// per recipient, because "the installation sent it" is not a reason somebody
// may be written to.
type senderProfile struct {
	// requiresSendScope asks the provider whether the credential may still send.
	requiresSendScope bool
	// requiresLiveSeat asks this installation whether the human may still act.
	requiresLiveSeat bool
	// carriesFiles asks whether attachments can go and are still readable.
	carriesFiles bool
	// paced holds a mailbox to a rate its provider will tolerate.
	paced bool
}

// profileFor answers from the row itself rather than from a flag a caller
// passed, so a delivery cannot claim a profile it is not.
func profileFor(del Delivery) senderProfile {
	if del.IsController() {
		return senderProfile{}
	}
	return senderProfile{
		requiresSendScope: true,
		requiresLiveSeat:  true,
		carriesFiles:      true,
		paced:             true,
	}
}
