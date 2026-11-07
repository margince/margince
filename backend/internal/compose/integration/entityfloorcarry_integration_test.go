// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A member's mail is kept under the floor of the legal entity they work for,
// and never under less than the default entity's floor. Moving a member or
// changing an entity's country runs the carry a jurisdiction change runs. So
// mail already held keeps the floor it had. An entity's country must be an
// assigned ISO code. So this file registers aq with zq's six-year floor, and
// bv with none.

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// entityCountryPack is a pack under an assigned code: aq keeps six years
// like zq, bv declares no floor like zk.
type entityCountryPack struct{ code jurisdiction.Code }

func (p entityCountryPack) Code() jurisdiction.Code { return p.code }

func (p entityCountryPack) Retention() jurisdiction.Retention {
	if p.code == "bv" {
		return nil
	}
	return goBDFloorClasses{}
}

func init() {
	jurisdiction.Register(entityCountryPack{code: "aq"})
	jurisdiction.Register(entityCountryPack{code: "bv"})
}

// entityFloorFixture is a floorFixture with a further legal entity and a
// colleague, written through the contacts store as compose wires it.
type entityFloorFixture struct {
	floorFixture
	store     *contacts.Store
	colleague ids.UserID
	entity    contacts.LegalEntity
}

func newEntityFloorFixture(t *testing.T, defaultCountry, entityCountry string) entityFloorFixture {
	t.Helper()
	f := entityFloorFixture{floorFixture: newFloorFixture(t)}
	f.exec(t, `INSERT INTO setting (key, value) VALUES ('installation.country', to_jsonb($1::text))`, defaultCountry)
	f.store = f.e.Contacts.WithFloorCarry(compose.EntityFloorCarry(f.e.Pool))
	colleague := ids.NewV7()
	f.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Colleague')`,
		colleague, "colleague-"+colleague.String()+"@floor.test")
	f.colleague = ids.From[ids.UserKind](colleague)
	entity, err := f.store.CreateLegalEntity(f.governor(), contacts.LegalEntityInput{
		DisplayName: new("Further"), LegalName: new("Further GmbH"), Country: new(entityCountry),
	})
	if err != nil {
		t.Fatalf("creating the further entity: %v", err)
	}
	f.entity = entity
	return f
}

// colleagueMail is a 400-day-old Handelsbrief that arrived in the colleague's
// mailbox.
func (f entityFloorFixture) colleagueMail(t *testing.T) ids.UUID {
	t.Helper()
	id := f.mail(t, 400)
	f.exec(t, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, id, f.colleague)
	return id
}

func (f entityFloorFixture) assign(t *testing.T, entity ids.LegalEntityID) {
	t.Helper()
	if err := f.store.AssignLegalEntityMember(f.governor(), entity, f.colleague); err != nil {
		t.Fatalf("assigning the colleague: %v", err)
	}
}

// The default entity keeps six years and the further one none: the
// colleague's mail keeps the default floor after they move there.
func TestMovingToAnEntityWithoutAFloorKeepsTheDefaultFloor(t *testing.T) {
	f := newEntityFloorFixture(t, "zq", "bv")
	held := f.colleagueMail(t)
	f.assign(t, f.entity.ID)
	f.retentionPass(t)
	if f.erased(t, held) {
		t.Fatal("the colleague's 400-day mail was erased after they moved to a no-floor entity; the default floor must still keep it")
	}
}

// The further entity keeps six years and the default none. Its country then
// changes to one with no floor: mail it already held keeps six years.
func TestAnEntityCountryChangeCarriesTheOldFloor(t *testing.T) {
	f := newEntityFloorFixture(t, "zk", "aq")
	f.assign(t, f.entity.ID)
	held := f.colleagueMail(t)
	version := f.entity.Version
	if _, err := f.store.UpdateLegalEntity(f.governor(), f.entity.ID, contacts.LegalEntityInput{Country: new("bv")}, &version); err != nil {
		t.Fatalf("changing the entity's country: %v", err)
	}
	f.retentionPass(t)
	if f.erased(t, held) {
		t.Fatal("mail held under the entity's six-year floor was erased once its country changed; the old floor must carry")
	}
}

// Leaving the further entity for the default runs the same carry.
func TestLeavingAnEntityCarriesItsFloor(t *testing.T) {
	f := newEntityFloorFixture(t, "zk", "aq")
	f.assign(t, f.entity.ID)
	held := f.colleagueMail(t)
	var defaultEntity ids.LegalEntityID
	entities, err := f.store.ListLegalEntities(f.governor(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, entity := range entities {
		if entity.IsDefault {
			defaultEntity = entity.ID
		}
	}
	f.assign(t, defaultEntity)
	f.retentionPass(t)
	if f.erased(t, held) {
		t.Fatal("the colleague's mail was erased after they left the six-year entity; its floor must carry")
	}
}
