// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// TargetType is what an approval's target_entity_type and co_target_entity_type
// name: the kind of thing the approval decides or is pinned against.
//
// This list is the one spelling. TestEveryDomainEnumMatchesItsSchemaCheck holds
// the CHECK on both columns equal to it. A unit's own table is the open arm, as
// extensionTableGrammar spells it. TestEveryApprovalWriterVocabularyIsAnApprovalTargetType
// holds the vocabularies that writers stage from inside it.
type TargetType string

// The staged target types. The first block is the record vocabulary; the rest
// are what is staged against something that is not a record a user opens.
const (
	TargetTypeContact             TargetType = "contact"
	TargetTypeCompany             TargetType = "company"
	TargetTypeDeal                TargetType = "deal"
	TargetTypeLead                TargetType = "lead"
	TargetTypeActivity            TargetType = "activity"
	TargetTypeProject             TargetType = "project"
	TargetTypeRelationship        TargetType = "relationship"
	TargetTypePartner             TargetType = "partner"
	TargetTypeProduct             TargetType = "product"
	TargetTypeOfferTemplate       TargetType = "offer_template"
	TargetTypeOffer               TargetType = "offer"
	TargetTypeCommission          TargetType = "commission"
	TargetTypeDealRoom            TargetType = "deal_room"
	TargetTypeDealRoomThread      TargetType = "deal_room_thread"
	TargetTypeDealRoomComment     TargetType = "deal_room_comment"
	TargetTypeDealRoomDocument    TargetType = "deal_room_document"
	TargetTypeDealRoomParticipant TargetType = "deal_room_participant"
	TargetTypeList                TargetType = "list"
	TargetTypeSignal              TargetType = "signal"
	TargetTypeSavedView           TargetType = "saved_view"
	TargetTypeTag                 TargetType = "tag"
	TargetTypeCustomField         TargetType = "custom_field"
	TargetTypeWebhookSubscription TargetType = "webhook_subscription"
	TargetTypeImportRun           TargetType = "import_run"
	TargetTypeReportDefinition    TargetType = "report_definition"
	TargetTypeAppUser             TargetType = "app_user"
	TargetTypeUserAdmin           TargetType = "user_admin"
	TargetTypeTeam                TargetType = "team"
	TargetTypeRecordGrant         TargetType = "record_grant"
	TargetTypeDataSubjectRequest  TargetType = "data_subject_request"
	TargetTypeFxRate              TargetType = "fx_rate"
	TargetTypeAIModelRate         TargetType = "ai_model_rate"
	TargetTypeLegalReview         TargetType = "legal_review"
	TargetTypeBrief               TargetType = "brief"
	// TargetTypeStage is only ever a co-target: a deal's move names the stage
	// it moves to beside the deal it moves.
	TargetTypeStage TargetType = "stage"
)

// knownTargetTypes is the set the constants above declare, for the one check
// that runs before a row is written. A second list here would be a second copy
// of the constants, so it is built from them.
var knownTargetTypes = map[string]struct{}{
	string(TargetTypeContact): {}, string(TargetTypeCompany): {}, string(TargetTypeDeal): {},
	string(TargetTypeLead): {}, string(TargetTypeActivity): {}, string(TargetTypeProject): {},
	string(TargetTypeRelationship): {}, string(TargetTypePartner): {}, string(TargetTypeProduct): {},
	string(TargetTypeOfferTemplate): {}, string(TargetTypeOffer): {}, string(TargetTypeCommission): {},
	string(TargetTypeDealRoom): {}, string(TargetTypeDealRoomThread): {}, string(TargetTypeDealRoomComment): {},
	string(TargetTypeDealRoomDocument): {}, string(TargetTypeDealRoomParticipant): {}, string(TargetTypeList): {},
	string(TargetTypeSignal): {}, string(TargetTypeSavedView): {}, string(TargetTypeTag): {},
	string(TargetTypeCustomField): {}, string(TargetTypeWebhookSubscription): {}, string(TargetTypeImportRun): {},
	string(TargetTypeReportDefinition): {}, string(TargetTypeAppUser): {}, string(TargetTypeUserAdmin): {},
	string(TargetTypeTeam): {}, string(TargetTypeRecordGrant): {}, string(TargetTypeDataSubjectRequest): {},
	string(TargetTypeFxRate): {}, string(TargetTypeAIModelRate): {}, string(TargetTypeLegalReview): {},
	string(TargetTypeBrief): {}, string(TargetTypeStage): {},
}

// requireKnownTargetType refuses a staging whose target type the schema CHECK
// would refuse, so the caller reads a validation answer and not a constraint
// error. An empty type is a staging with no target on that side.
func requireKnownTargetType(column, targetType string) error {
	if targetType == "" {
		return nil
	}
	if _, known := knownTargetTypes[targetType]; known || extensionTableGrammar.MatchString(targetType) {
		return nil
	}
	return fmt.Errorf("%s %q is not a type an approval can name: %w", column, targetType, apperrors.ErrInvalidArgument)
}
