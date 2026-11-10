// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// ownershipTransferWords are the sentences a transfer's card and its notices
// carry, in the installation's base language. The record and colleague names
// are quoted as they are stored.
var ownershipTransferWords = struct {
	// AskedContact and AskedCompany: the colleague asking, then the count of
	// open deals and of open tasks that move with the record.
	AskedContact, AskedCompany langcopy.Phrase
	// Accepted: the colleague who handed it over, then the record.
	Accepted langcopy.Phrase
	// AcceptedMoved: the deals, then the tasks that moved with it.
	AcceptedMoved langcopy.Phrase
	// KeptDeals: the colleague the deals stayed with, then how many.
	KeptDeals langcopy.Phrase
	// KeptTasks: the colleague the tasks stayed with, then how many.
	KeptTasks langcopy.Phrase
	// Declined, Expired and Withdrawn: the owner, then the record.
	Declined, DeclinedBody   langcopy.Phrase
	Expired, ExpiredBody     langcopy.Phrase
	Withdrawn, WithdrawnBody langcopy.Phrase
	// OfferedContact and OfferedCompany: the owner offering, then the counts
	// AskedContact carries.
	OfferedContact, OfferedCompany langcopy.Phrase
	// OfferAccepted: the colleague who took it, then the record.
	OfferAccepted langcopy.Phrase
	// OfferAcceptedMoved: the deals, then the tasks that moved with it.
	OfferAcceptedMoved langcopy.Phrase
	// OfferDeclined, OfferExpired and OfferWithdrawn: the colleague offered
	// to, then the record.
	OfferDeclined, OfferDeclinedBody   langcopy.Phrase
	OfferExpired, OfferExpiredBody     langcopy.Phrase
	OfferWithdrawn, OfferWithdrawnBody langcopy.Phrase
}{
	AskedContact: langcopy.Phrase{
		En: systemtext.Copy("ownership.AskedContact", textlang.English),
		De: systemtext.Copy("ownership.AskedContact", textlang.German),
		Vi: systemtext.Copy("ownership.AskedContact", textlang.Vietnamese),
	},
	AskedCompany: langcopy.Phrase{
		En: systemtext.Copy("ownership.AskedCompany", textlang.English),
		De: systemtext.Copy("ownership.AskedCompany", textlang.German),
		Vi: systemtext.Copy("ownership.AskedCompany", textlang.Vietnamese),
	},
	Accepted: langcopy.Phrase{
		En: systemtext.Copy("ownership.Accepted", textlang.English),
		De: systemtext.Copy("ownership.Accepted", textlang.German),
		Vi: systemtext.Copy("ownership.Accepted", textlang.Vietnamese),
	},
	AcceptedMoved: langcopy.Phrase{
		En: systemtext.Copy("ownership.AcceptedMoved", textlang.English),
		De: systemtext.Copy("ownership.AcceptedMoved", textlang.German),
		Vi: systemtext.Copy("ownership.AcceptedMoved", textlang.Vietnamese),
	},
	KeptDeals: langcopy.Phrase{
		En: systemtext.Copy("ownership.KeptDeals", textlang.English),
		De: systemtext.Copy("ownership.KeptDeals", textlang.German),
		Vi: systemtext.Copy("ownership.KeptDeals", textlang.Vietnamese),
	},
	KeptTasks: langcopy.Phrase{
		En: systemtext.Copy("ownership.KeptTasks", textlang.English),
		De: systemtext.Copy("ownership.KeptTasks", textlang.German),
		Vi: systemtext.Copy("ownership.KeptTasks", textlang.Vietnamese),
	},
	Declined: langcopy.Phrase{
		En: systemtext.Copy("ownership.Declined", textlang.English),
		De: systemtext.Copy("ownership.Declined", textlang.German),
		Vi: systemtext.Copy("ownership.Declined", textlang.Vietnamese),
	},
	DeclinedBody: langcopy.Phrase{
		En: systemtext.Copy("ownership.DeclinedBody", textlang.English),
		De: systemtext.Copy("ownership.DeclinedBody", textlang.German),
		Vi: systemtext.Copy("ownership.DeclinedBody", textlang.Vietnamese),
	},
	Expired: langcopy.Phrase{
		En: systemtext.Copy("ownership.Expired", textlang.English),
		De: systemtext.Copy("ownership.Expired", textlang.German),
		Vi: systemtext.Copy("ownership.Expired", textlang.Vietnamese),
	},
	ExpiredBody: langcopy.Phrase{
		En: systemtext.Copy("ownership.ExpiredBody", textlang.English),
		De: systemtext.Copy("ownership.ExpiredBody", textlang.German),
		Vi: systemtext.Copy("ownership.ExpiredBody", textlang.Vietnamese),
	},
	Withdrawn: langcopy.Phrase{
		En: systemtext.Copy("ownership.Withdrawn", textlang.English),
		De: systemtext.Copy("ownership.Withdrawn", textlang.German),
		Vi: systemtext.Copy("ownership.Withdrawn", textlang.Vietnamese),
	},
	WithdrawnBody: langcopy.Phrase{
		En: systemtext.Copy("ownership.WithdrawnBody", textlang.English),
		De: systemtext.Copy("ownership.WithdrawnBody", textlang.German),
		Vi: systemtext.Copy("ownership.WithdrawnBody", textlang.Vietnamese),
	},
	OfferedContact: langcopy.Phrase{
		En: systemtext.Copy("ownership.OfferedContact", textlang.English),
		De: systemtext.Copy("ownership.OfferedContact", textlang.German),
		Vi: systemtext.Copy("ownership.OfferedContact", textlang.Vietnamese),
	},
	OfferedCompany: langcopy.Phrase{
		En: systemtext.Copy("ownership.OfferedCompany", textlang.English),
		De: systemtext.Copy("ownership.OfferedCompany", textlang.German),
		Vi: systemtext.Copy("ownership.OfferedCompany", textlang.Vietnamese),
	},
	OfferAccepted: langcopy.Phrase{
		En: systemtext.Copy("ownership.OfferAccepted", textlang.English),
		De: systemtext.Copy("ownership.OfferAccepted", textlang.German),
		Vi: systemtext.Copy("ownership.OfferAccepted", textlang.Vietnamese),
	},
	OfferAcceptedMoved: langcopy.Phrase{
		En: systemtext.Copy("ownership.OfferAcceptedMoved", textlang.English),
		De: systemtext.Copy("ownership.OfferAcceptedMoved", textlang.German),
		Vi: systemtext.Copy("ownership.OfferAcceptedMoved", textlang.Vietnamese),
	},
	OfferDeclined: langcopy.Phrase{
		En: systemtext.Copy("ownership.OfferDeclined", textlang.English),
		De: systemtext.Copy("ownership.OfferDeclined", textlang.German),
		Vi: systemtext.Copy("ownership.OfferDeclined", textlang.Vietnamese),
	},
	OfferDeclinedBody: langcopy.Phrase{
		En: systemtext.Copy("ownership.OfferDeclinedBody", textlang.English),
		De: systemtext.Copy("ownership.OfferDeclinedBody", textlang.German),
		Vi: systemtext.Copy("ownership.OfferDeclinedBody", textlang.Vietnamese),
	},
	OfferExpired: langcopy.Phrase{
		En: systemtext.Copy("ownership.OfferExpired", textlang.English),
		De: systemtext.Copy("ownership.OfferExpired", textlang.German),
		Vi: systemtext.Copy("ownership.OfferExpired", textlang.Vietnamese),
	},
	OfferExpiredBody: langcopy.Phrase{
		En: systemtext.Copy("ownership.OfferExpiredBody", textlang.English),
		De: systemtext.Copy("ownership.OfferExpiredBody", textlang.German),
		Vi: systemtext.Copy("ownership.OfferExpiredBody", textlang.Vietnamese),
	},
	OfferWithdrawn: langcopy.Phrase{
		En: systemtext.Copy("ownership.OfferWithdrawn", textlang.English),
		De: systemtext.Copy("ownership.OfferWithdrawn", textlang.German),
		Vi: systemtext.Copy("ownership.OfferWithdrawn", textlang.Vietnamese),
	},
	OfferWithdrawnBody: langcopy.Phrase{
		En: systemtext.Copy("ownership.OfferWithdrawnBody", textlang.English),
		De: systemtext.Copy("ownership.OfferWithdrawnBody", textlang.German),
		Vi: systemtext.Copy("ownership.OfferWithdrawnBody", textlang.Vietnamese),
	},
}

// cardPhrase is the decider's card sentence for the transfer's direction and
// record type.
func cardPhrase(transfer approvalsubject.OwnershipTransfer) langcopy.Phrase {
	code := cardCode(transfer)
	return langcopy.Phrase{En: systemtext.Copy(code, textlang.English), De: systemtext.Copy(code, textlang.German), Vi: systemtext.Copy(code, textlang.Vietnamese)}
}

func cardCode(transfer approvalsubject.OwnershipTransfer) string {
	company := transfer.RecordType == string(recordTypeCompany)
	switch {
	case transfer.Offered() && company:
		return "ownership.OfferedCompany"
	case transfer.Offered():
		return "ownership.OfferedContact"
	case company:
		return "ownership.AskedCompany"
	default:
		return "ownership.AskedContact"
	}
}
