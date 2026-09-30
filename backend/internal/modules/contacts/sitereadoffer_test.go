// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Who may be offered a change, decided before the slot is ever read: an offer
// to somebody who could not save it would grant a change they cannot make.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// offerCaller is a team-scoped human: a mask binds only a caller who does not
// read every row, which is why the scope is not the workspace-wide one.
func offerCaller(grant principal.ObjectGrant, masked ...string) context.Context {
	masks := make([]principal.FieldMask, 0, len(masked))
	for _, field := range masked {
		masks = append(masks, principal.FieldMask{Object: "company", Field: field, Condition: principal.MaskAlways})
	}
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			RoleKeys:   []string{"rep"},
			Objects:    map[string]principal.ObjectGrant{"company": grant},
			RowScope:   principal.RowScopeTeam,
			FieldMasks: masks,
		},
	})
}

// Saving a company is what an accepted offer leads to, so the offer follows the
// save: a creator or an updater may be offered a field no mask withholds.
func TestAnOfferGoesOnlyToWhoeverCouldSaveIt(t *testing.T) {
	for name, tc := range map[string]struct {
		ctx  context.Context
		want bool
	}{
		"an updater":            {offerCaller(principal.ObjectGrant{Read: true, Update: true}), true},
		"a creator":             {offerCaller(principal.ObjectGrant{Read: true, Create: true}), true},
		"a reader":              {offerCaller(principal.ObjectGrant{Read: true}), false},
		"a masked field":        {offerCaller(principal.ObjectGrant{Read: true, Update: true}, "legal_name"), false},
		"a mask on another one": {offerCaller(principal.ObjectGrant{Read: true, Update: true}, "icp"), true},
		"nobody":                {context.Background(), false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := MayOfferSiteReadChange(tc.ctx, "legal_name"); got != tc.want {
				t.Errorf("MayOfferSiteReadChange = %v, want %v", got, tc.want)
			}
		})
	}
}

// A caller without the authority is refused before the slot is read or
// written, and an offer of a field withheld from them is never recorded.
func TestTheOfferSlotRefusesACallerWhoCouldNotSave(t *testing.T) {
	var s Store
	reader := offerCaller(principal.ObjectGrant{Read: true})
	offer := &SiteReadOffer{Field: "legal_name", Value: "Voltaq Systems GmbH"}
	if _, err := s.StandingSiteReadOffer(reader, ids.NewV7()); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("StandingSiteReadOffer err = %v, want ErrPermissionDenied", err)
	}
	if err := s.ReplaceSiteReadOffer(reader, ids.NewV7(), offer, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("ReplaceSiteReadOffer err = %v, want ErrPermissionDenied", err)
	}
	masked := offerCaller(principal.ObjectGrant{Read: true, Update: true}, "legal_name")
	if err := s.ReplaceSiteReadOffer(masked, ids.NewV7(), offer, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("offering a masked field err = %v, want ErrPermissionDenied", err)
	}
}

// "No offer" has one spelling in the column, SQL NULL, and reads back as none;
// an offer survives the round trip whole.
func TestTheOfferSlotHasOneSpellingForEmpty(t *testing.T) {
	encoded, err := encodeSiteReadOffer(nil)
	if err != nil || encoded != nil {
		t.Fatalf("encode(nil) = (%q, %v), want SQL NULL", encoded, err)
	}
	if decoded, err := decodeSiteReadOffer(nil); err != nil || decoded != nil {
		t.Fatalf("decode(NULL) = (%+v, %v), want no offer", decoded, err)
	}
	offer := SiteReadOffer{Field: "legal_name", Value: "Voltaq", SourceIDs: []string{"S1"}, TurnDigest: "d", DraftVersion: 2, OfferedTo: "human:x"}
	encoded, err = encodeSiteReadOffer(&offer)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeSiteReadOffer(encoded)
	if err != nil || decoded == nil || decoded.Field != offer.Field || decoded.OfferedTo != offer.OfferedTo ||
		decoded.DraftVersion != offer.DraftVersion || len(decoded.SourceIDs) != 1 {
		t.Fatalf("round trip = (%+v, %v), want %+v", decoded, err, offer)
	}
	if _, err := decodeSiteReadOffer([]byte(`"not an offer"`)); err == nil {
		t.Error("an unreadable slot decoded as an offer")
	}
}
