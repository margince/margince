// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package targettype is the list of things an approval can name as its target,
// which a notice may also point at. It lives in shared because two modules
// refuse a value outside it, and a module never imports a sibling.
package targettype

import (
	"regexp"
)

// Type is what approval.target_entity_type and approval.co_target_entity_type
// store, and the part of notice.target_type an approval can name.
//
// This list is the one spelling. A unit's own table is the open arm, as
// Extension spells it.
//
// Held by: TestEveryDomainEnumMatchesItsSchemaCheck (backend/gates/enumsync_test.go)
// Held by: TestEveryApprovalStagerNamesAnApprovalTargetType (backend/gates/approvaltargettypes_test.go)
type Type string

// The staged target types. The first block is the record vocabulary; the rest
// are what is staged against something that is not a record a user opens.
// Stage is only ever a co-target: a deal's move names the stage it moves to
// beside the deal it moves.
const (
	Contact             Type = "contact"
	Company             Type = "company"
	Deal                Type = "deal"
	Lead                Type = "lead"
	Activity            Type = "activity"
	Project             Type = "project"
	Relationship        Type = "relationship"
	Partner             Type = "partner"
	Product             Type = "product"
	OfferTemplate       Type = "offer_template"
	Offer               Type = "offer"
	Commission          Type = "commission"
	DealRoom            Type = "deal_room"
	DealRoomThread      Type = "deal_room_thread"
	DealRoomComment     Type = "deal_room_comment"
	DealRoomDocument    Type = "deal_room_document"
	DealRoomParticipant Type = "deal_room_participant"
	List                Type = "list"
	Signal              Type = "signal"
	SavedView           Type = "saved_view"
	Tag                 Type = "tag"
	CustomField         Type = "custom_field"
	WebhookSubscription Type = "webhook_subscription"
	ImportRun           Type = "import_run"
	ReportDefinition    Type = "report_definition"
	AppUser             Type = "app_user"
	UserAdmin           Type = "user_admin"
	Team                Type = "team"
	RecordGrant         Type = "record_grant"
	DataSubjectRequest  Type = "data_subject_request"
	FxRate              Type = "fx_rate"
	AIModelRate         Type = "ai_model_rate"
	LegalReview         Type = "legal_review"
	Brief               Type = "brief"
	Stage               Type = "stage"
)

// Extension is the shape of a unit's own table, which an approval or a notice
// may also name.
var Extension = regexp.MustCompile(`^ext_[a-z0-9]+(_[a-z0-9]+)*$`)

var known = map[string]struct{}{
	string(Contact):             {},
	string(Company):             {},
	string(Deal):                {},
	string(Lead):                {},
	string(Activity):            {},
	string(Project):             {},
	string(Relationship):        {},
	string(Partner):             {},
	string(Product):             {},
	string(OfferTemplate):       {},
	string(Offer):               {},
	string(Commission):          {},
	string(DealRoom):            {},
	string(DealRoomThread):      {},
	string(DealRoomComment):     {},
	string(DealRoomDocument):    {},
	string(DealRoomParticipant): {},
	string(List):                {},
	string(Signal):              {},
	string(SavedView):           {},
	string(Tag):                 {},
	string(CustomField):         {},
	string(WebhookSubscription): {},
	string(ImportRun):           {},
	string(ReportDefinition):    {},
	string(AppUser):             {},
	string(UserAdmin):           {},
	string(Team):                {},
	string(RecordGrant):         {},
	string(DataSubjectRequest):  {},
	string(FxRate):              {},
	string(AIModelRate):         {},
	string(LegalReview):         {},
	string(Brief):               {},
	string(Stage):               {},
}

// Known reports whether a value is on the list or is a unit's own table. The
// schema CHECK on every column that holds the list refuses the same values.
func Known(value string) bool {
	if _, ok := known[value]; ok {
		return true
	}
	return Extension.MatchString(value)
}
