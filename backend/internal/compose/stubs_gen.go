// Code generated from internal/contracts/api_gen.go ServerInterface; DO NOT EDIT.
// Regenerate: make gen (tools/gen-stubs).

package compose

import (
	nethttp "net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// stubs answers every crmcontracts.ServerInterface operation with an explicit
// 501: one stub per operation the contract declares.
//
// NOTHING EMBEDS IT. Server (server.go) implements the interface itself across
// its module handler embeds and carries its own compile-time assertion, so an
// operation with no real handler fails that assertion at build time — no
// request ever reaches a stub below.
type stubs struct{}

var _ crmcontracts.ServerInterface = stubs{}

func (stubs) ListAcquisitionSources(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListAcquisitionSources")
}

func (stubs) CreateAcquisitionSource(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateAcquisitionSourceParams) {
	httperr.NotImplemented(w, r, "CreateAcquisitionSource")
}

func (stubs) UpdateAcquisitionSource(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateAcquisitionSourceParams) {
	httperr.NotImplemented(w, r, "UpdateAcquisitionSource")
}

func (stubs) ListActivities(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListActivitiesParams) {
	httperr.NotImplemented(w, r, "ListActivities")
}

func (stubs) LogActivity(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.LogActivityParams) {
	httperr.NotImplemented(w, r, "LogActivity")
}

func (stubs) RelinkActivities(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.RelinkActivitiesParams) {
	httperr.NotImplemented(w, r, "RelinkActivities")
}

func (stubs) RelinkThread(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.RelinkThreadParams) {
	httperr.NotImplemented(w, r, "RelinkThread")
}

func (stubs) SetThreadAudience(w nethttp.ResponseWriter, r *nethttp.Request, threadKey string) {
	httperr.NotImplemented(w, r, "SetThreadAudience")
}

func (stubs) ArchiveActivity(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ArchiveActivityParams) {
	httperr.NotImplemented(w, r, "ArchiveActivity")
}

func (stubs) GetActivity(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetActivity")
}

func (stubs) UpdateActivity(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateActivityParams) {
	httperr.NotImplemented(w, r, "UpdateActivity")
}

func (stubs) SetActivityAudience(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.SetActivityAudienceParams) {
	httperr.NotImplemented(w, r, "SetActivityAudience")
}

func (stubs) ClearActivityDisposition(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ClearActivityDispositionParams) {
	httperr.NotImplemented(w, r, "ClearActivityDisposition")
}

func (stubs) SetActivityDisposition(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "SetActivityDisposition")
}

func (stubs) DraftEmail(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DraftEmail")
}

func (stubs) GetEmailPresentation(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.GetEmailPresentationParams) {
	httperr.NotImplemented(w, r, "GetEmailPresentation")
}

func (stubs) GetMeetingBrief(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.GetMeetingBriefParams) {
	httperr.NotImplemented(w, r, "GetMeetingBrief")
}

func (stubs) ReadActivityPipelineTrace(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ReadActivityPipelineTrace")
}

func (stubs) RelinkActivity(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.RelinkActivityParams) {
	httperr.NotImplemented(w, r, "RelinkActivity")
}

func (stubs) GetReplyRecipient(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetReplyRecipient")
}

func (stubs) SendEmail(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.SendEmailParams) {
	httperr.NotImplemented(w, r, "SendEmail")
}

func (stubs) PreviewSendAuthorization(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "PreviewSendAuthorization")
}

func (stubs) SendMessage(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.SendMessageParams) {
	httperr.NotImplemented(w, r, "SendMessage")
}

func (stubs) ReadTranscriptForNextSteps(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ReadTranscriptForNextSteps")
}

func (stubs) GetLatestTranscriptRead(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetLatestTranscriptRead")
}

func (stubs) GetTranscriptRead(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, readId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "GetTranscriptRead")
}

func (stubs) GetCaptureHealth(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetCaptureHealth")
}

func (stubs) GetJobHealth(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetJobHealth")
}

func (stubs) ResetData(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ResetData")
}

func (stubs) ListAgentTools(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListAgentTools")
}

func (stubs) ListAiModelRates(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListAiModelRatesParams) {
	httperr.NotImplemented(w, r, "ListAiModelRates")
}

func (stubs) SetAiModelRate(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "SetAiModelRate")
}

func (stubs) ProposeAiModelRateRefresh(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ProposeAiModelRateRefresh")
}

func (stubs) ListAvailableModels(w nethttp.ResponseWriter, r *nethttp.Request, provider string, params crmcontracts.ListAvailableModelsParams) {
	httperr.NotImplemented(w, r, "ListAvailableModels")
}

func (stubs) ListAiCalls(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListAiCallsParams) {
	httperr.NotImplemented(w, r, "ListAiCalls")
}

func (stubs) GetAiCall(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetAiCall")
}

func (stubs) RecordAIFeedback(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "RecordAIFeedback")
}

func (stubs) GetAiHealth(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetAiHealth")
}

func (stubs) GetAiProfile(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetAiProfile")
}

func (stubs) ListAiProviderKeys(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListAiProviderKeys")
}

func (stubs) DeleteAiProviderKey(w nethttp.ResponseWriter, r *nethttp.Request, provider string) {
	httperr.NotImplemented(w, r, "DeleteAiProviderKey")
}

func (stubs) SetAiProviderKey(w nethttp.ResponseWriter, r *nethttp.Request, provider string) {
	httperr.NotImplemented(w, r, "SetAiProviderKey")
}

func (stubs) GetAiRouting(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetAiRouting")
}

func (stubs) ReplaceAiRouting(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ReplaceAiRouting")
}

func (stubs) GetAiUsage(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.GetAiUsageParams) {
	httperr.NotImplemented(w, r, "GetAiUsage")
}

func (stubs) GetAnalyticsContext(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetAnalyticsContext")
}

func (stubs) GetDataCoverage(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetDataCoverage")
}

func (stubs) ExplainAnalyticsCell(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ExplainAnalyticsCell")
}

func (stubs) RunAnalyticsQuery(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "RunAnalyticsQuery")
}

func (stubs) RenderAnalyticsReport(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "RenderAnalyticsReport")
}

func (stubs) GetReportRun(w nethttp.ResponseWriter, r *nethttp.Request, runId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "GetReportRun")
}

func (stubs) ExplainReportRunCell(w nethttp.ResponseWriter, r *nethttp.Request, runId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "ExplainReportRunCell")
}

func (stubs) GetAnalyticsSchema(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetAnalyticsSchema")
}

func (stubs) ApproveApprovalBundle(w nethttp.ResponseWriter, r *nethttp.Request, bundleId crmcontracts.BundleId) {
	httperr.NotImplemented(w, r, "ApproveApprovalBundle")
}

func (stubs) RejectApprovalBundle(w nethttp.ResponseWriter, r *nethttp.Request, bundleId crmcontracts.BundleId) {
	httperr.NotImplemented(w, r, "RejectApprovalBundle")
}

func (stubs) ListApprovals(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListApprovalsParams) {
	httperr.NotImplemented(w, r, "ListApprovals")
}

func (stubs) GetApproval(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetApproval")
}

func (stubs) ApproveApproval(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ApproveApprovalParams) {
	httperr.NotImplemented(w, r, "ApproveApproval")
}

func (stubs) RejectApproval(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "RejectApproval")
}

func (stubs) ArchiveRecordAssignment(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ArchiveRecordAssignment")
}

func (stubs) UpdateRecordAssignment(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateRecordAssignmentParams) {
	httperr.NotImplemented(w, r, "UpdateRecordAssignment")
}

func (stubs) GetAssistantProfile(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetAssistantProfile")
}

func (stubs) ListAttachments(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListAttachmentsParams) {
	httperr.NotImplemented(w, r, "ListAttachments")
}

func (stubs) UploadAttachment(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "UploadAttachment")
}

func (stubs) DeleteAttachment(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DeleteAttachment")
}

func (stubs) DownloadAttachment(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DownloadAttachment")
}

func (stubs) GetAttachmentExtraction(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetAttachmentExtraction")
}

func (stubs) ReadAttachmentForFields(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ReadAttachmentForFields")
}

func (stubs) AcceptAttachmentExtraction(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "AcceptAttachmentExtraction")
}

func (stubs) UpdateAttachmentMetadata(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "UpdateAttachmentMetadata")
}

func (stubs) RequestAttachmentAccess(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "RequestAttachmentAccess")
}

func (stubs) GetAttention(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetAttention")
}

func (stubs) ListAuditLog(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListAuditLogParams) {
	httperr.NotImplemented(w, r, "ListAuditLog")
}

func (stubs) GetAuthCapabilities(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetAuthCapabilities")
}

func (stubs) ChangePassword(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ChangePassword")
}

func (stubs) RequestPasswordReset(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "RequestPasswordReset")
}

func (stubs) Login(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "Login")
}

func (stubs) Logout(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "Logout")
}

func (stubs) OidcSignInCallback(w nethttp.ResponseWriter, r *nethttp.Request, provider crmcontracts.OidcSignInCallbackParamsProvider, params crmcontracts.OidcSignInCallbackParams) {
	httperr.NotImplemented(w, r, "OidcSignInCallback")
}

func (stubs) StartOidcSignIn(w nethttp.ResponseWriter, r *nethttp.Request, provider crmcontracts.StartOidcSignInParamsProvider) {
	httperr.NotImplemented(w, r, "StartOidcSignIn")
}

func (stubs) ResetPassword(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ResetPassword")
}

func (stubs) ListAutomations(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListAutomationsParams) {
	httperr.NotImplemented(w, r, "ListAutomations")
}

func (stubs) CreateAutomation(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "CreateAutomation")
}

func (stubs) ListAutomationCatalog(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListAutomationCatalog")
}

func (stubs) RetryAutomationRun(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "RetryAutomationRun")
}

func (stubs) DeleteAutomation(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DeleteAutomation")
}

func (stubs) GetAutomation(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetAutomation")
}

func (stubs) UpdateAutomation(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateAutomationParams) {
	httperr.NotImplemented(w, r, "UpdateAutomation")
}

func (stubs) PreviewAutomation(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "PreviewAutomation")
}

func (stubs) ListAutomationRuns(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ListAutomationRunsParams) {
	httperr.NotImplemented(w, r, "ListAutomationRuns")
}

func (stubs) GetAutonomy(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetAutonomy")
}

func (stubs) UpdateAutonomy(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "UpdateAutonomy")
}

func (stubs) GetAvailability(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.GetAvailabilityParams) {
	httperr.NotImplemented(w, r, "GetAvailability")
}

func (stubs) BookMeeting(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.BookMeetingParams) {
	httperr.NotImplemented(w, r, "BookMeeting")
}

func (stubs) GetMorningBrief(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetMorningBrief")
}

func (stubs) GenerateMorningBrief(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GenerateMorningBrief")
}

func (stubs) AnnotateMorningBrief(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "AnnotateMorningBrief")
}

func (stubs) MarkBriefItemActed(w nethttp.ResponseWriter, r *nethttp.Request, itemId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "MarkBriefItemActed")
}

func (stubs) MarkBriefItemDismissed(w nethttp.ResponseWriter, r *nethttp.Request, itemId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "MarkBriefItemDismissed")
}

func (stubs) SnoozeBriefItem(w nethttp.ResponseWriter, r *nethttp.Request, itemId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "SnoozeBriefItem")
}

func (stubs) UnsnoozeBriefItem(w nethttp.ResponseWriter, r *nethttp.Request, itemId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "UnsnoozeBriefItem")
}

func (stubs) ListMyCaptureActivity(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListMyCaptureActivityParams) {
	httperr.NotImplemented(w, r, "ListMyCaptureActivity")
}

func (stubs) ListWorkspaceCaptureActivity(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListWorkspaceCaptureActivityParams) {
	httperr.NotImplemented(w, r, "ListWorkspaceCaptureActivity")
}

func (stubs) ListBlockedDomains(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListBlockedDomains")
}

func (stubs) SetBlockedDomain(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "SetBlockedDomain")
}

func (stubs) ListConsumerMailBaseline(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListConsumerMailBaselineParams) {
	httperr.NotImplemented(w, r, "ListConsumerMailBaseline")
}

func (stubs) ListConsumerMailDomains(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListConsumerMailDomains")
}

func (stubs) AddConsumerMailDomain(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "AddConsumerMailDomain")
}

func (stubs) RemoveConsumerMailDomain(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "RemoveConsumerMailDomain")
}

func (stubs) ListCaptureCounterpartyHolds(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListCaptureCounterpartyHolds")
}

func (stubs) CreateCaptureCounterpartyHold(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "CreateCaptureCounterpartyHold")
}

func (stubs) ShareCaptureCounterpartyHoldHistory(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ShareCaptureCounterpartyHoldHistory")
}

func (stubs) DeleteCaptureCounterpartyHold(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DeleteCaptureCounterpartyHold")
}

func (stubs) ListWorkspaceEmailDomains(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListWorkspaceEmailDomains")
}

func (stubs) CreateWorkspaceEmailDomain(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "CreateWorkspaceEmailDomain")
}

func (stubs) DeleteWorkspaceEmailDomain(w nethttp.ResponseWriter, r *nethttp.Request, domain string) {
	httperr.NotImplemented(w, r, "DeleteWorkspaceEmailDomain")
}

func (stubs) ListCaptureExclusions(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListCaptureExclusions")
}

func (stubs) CreateCaptureExclusion(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "CreateCaptureExclusion")
}

func (stubs) DeleteCaptureExclusion(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DeleteCaptureExclusion")
}

func (stubs) PurgeCaptureExclusion(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.PurgeCaptureExclusionParams) {
	httperr.NotImplemented(w, r, "PurgeCaptureExclusion")
}

func (stubs) ListHeldThreads(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListHeldThreads")
}

func (stubs) ListCaptureOwnerIdentities(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListCaptureOwnerIdentities")
}

func (stubs) CreateCaptureOwnerIdentity(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "CreateCaptureOwnerIdentity")
}

func (stubs) DeleteCaptureOwnerIdentity(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DeleteCaptureOwnerIdentity")
}

func (stubs) ListCaptureSenders(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListCaptureSenders")
}

func (stubs) DeleteCaptureSenderDecision(w nethttp.ResponseWriter, r *nethttp.Request, address string) {
	httperr.NotImplemented(w, r, "DeleteCaptureSenderDecision")
}

func (stubs) SetCaptureSenderDecision(w nethttp.ResponseWriter, r *nethttp.Request, address string) {
	httperr.NotImplemented(w, r, "SetCaptureSenderDecision")
}

func (stubs) GetCaptureSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetCaptureSettings")
}

func (stubs) UpdateCaptureSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "UpdateCaptureSettings")
}

func (stubs) ReadCaptureTracePipeline(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ReadCaptureTracePipeline")
}

func (stubs) ListChannelConnections(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListChannelConnections")
}

func (stubs) ConnectChannel(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ConnectChannel")
}

func (stubs) DisconnectChannel(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DisconnectChannel")
}

func (stubs) ReplaceChannelToken(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ReplaceChannelToken")
}

func (stubs) ListChannelProviders(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListChannelProviders")
}

func (stubs) SettleConversationClaim(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "SettleConversationClaim")
}

func (stubs) ColdStartReadback(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ColdStartReadback")
}

func (stubs) ColdStartPreview(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ColdStartPreview")
}

func (stubs) ListCommissionEntries(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListCommissionEntriesParams) {
	httperr.NotImplemented(w, r, "ListCommissionEntries")
}

func (stubs) GetCommissionSummary(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetCommissionSummary")
}

func (stubs) GetCommissionEntry(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetCommissionEntry")
}

func (stubs) DecideCommissionEntry(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.DecideCommissionEntryParams) {
	httperr.NotImplemented(w, r, "DecideCommissionEntry")
}

func (stubs) ListCommunicationReviews(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListCommunicationReviewsParams) {
	httperr.NotImplemented(w, r, "ListCommunicationReviews")
}

func (stubs) GetCommunicationReview(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "GetCommunicationReview")
}

func (stubs) RecordCommunicationContext(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "RecordCommunicationContext")
}

func (stubs) DirectCommunicationSend(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "DirectCommunicationSend")
}

func (stubs) RequestCommunicationDecision(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "RequestCommunicationDecision")
}

func (stubs) ListCompanies(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListCompaniesParams) {
	httperr.NotImplemented(w, r, "ListCompanies")
}

func (stubs) CreateCompany(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateCompanyParams) {
	httperr.NotImplemented(w, r, "CreateCompany")
}

func (stubs) ArchiveCompany(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ArchiveCompanyParams) {
	httperr.NotImplemented(w, r, "ArchiveCompany")
}

func (stubs) GetCompany(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetCompany")
}

func (stubs) UpdateCompany(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateCompanyParams) {
	httperr.NotImplemented(w, r, "UpdateCompany")
}

func (stubs) GetCompany360(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.GetCompany360Params) {
	httperr.NotImplemented(w, r, "GetCompany360")
}

func (stubs) AskAboutCompany(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "AskAboutCompany")
}

func (stubs) GetCompanyBrief(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.GetCompanyBriefParams) {
	httperr.NotImplemented(w, r, "GetCompanyBrief")
}

func (stubs) RegenerateCompanyBrief(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.RegenerateCompanyBriefParams) {
	httperr.NotImplemented(w, r, "RegenerateCompanyBrief")
}

func (stubs) ListCompanyContacts(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ListCompanyContactsParams) {
	httperr.NotImplemented(w, r, "ListCompanyContacts")
}

func (stubs) ListCompanyContracts(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ListCompanyContractsParams) {
	httperr.NotImplemented(w, r, "ListCompanyContracts")
}

func (stubs) GetCompanyCoverage(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetCompanyCoverage")
}

func (stubs) DeepReadCompany(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DeepReadCompany")
}

func (stubs) ListCompanyDocuments(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ListCompanyDocumentsParams) {
	httperr.NotImplemented(w, r, "ListCompanyDocuments")
}

func (stubs) GetCompanyDossier(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetCompanyDossier")
}

func (stubs) RefreshCompanyDossier(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "RefreshCompanyDossier")
}

func (stubs) DraftAccountEmail(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DraftAccountEmail")
}

func (stubs) ScrapeCompany(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ScrapeCompany")
}

func (stubs) GetClaimEvidence(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, entityType string, entityId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "GetClaimEvidence")
}

func (stubs) ListCompanyFacts(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ListCompanyFacts")
}

func (stubs) CreateCompanyFact(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.CreateCompanyFactParams) {
	httperr.NotImplemented(w, r, "CreateCompanyFact")
}

func (stubs) DeleteCompanyFact(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, factKey crmcontracts.FactKey, params crmcontracts.DeleteCompanyFactParams) {
	httperr.NotImplemented(w, r, "DeleteCompanyFact")
}

func (stubs) UpdateCompanyFact(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, factKey crmcontracts.FactKey, params crmcontracts.UpdateCompanyFactParams) {
	httperr.NotImplemented(w, r, "UpdateCompanyFact")
}

func (stubs) ConfirmCompanyFact(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, factKey crmcontracts.FactKey, params crmcontracts.ConfirmCompanyFactParams) {
	httperr.NotImplemented(w, r, "ConfirmCompanyFact")
}

func (stubs) GetCompanyFinanceSummary(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetCompanyFinanceSummary")
}

func (stubs) GetCompanyGraph(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetCompanyGraph")
}

func (stubs) GetCompanyGrowthFit(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetCompanyGrowthFit")
}

func (stubs) RefreshCompanyGrowthFit(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "RefreshCompanyGrowthFit")
}

func (stubs) GetCompanyHierarchyRollup(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.GetCompanyHierarchyRollupParams) {
	httperr.NotImplemented(w, r, "GetCompanyHierarchyRollup")
}

func (stubs) DraftIntroRequest(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DraftIntroRequest")
}

func (stubs) GetCompanyLogo(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetCompanyLogo")
}

func (stubs) GetCompanyLogoIcon(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetCompanyLogoIcon")
}

func (stubs) MergeCompany(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.MergeCompanyParams) {
	httperr.NotImplemented(w, r, "MergeCompany")
}

func (stubs) GetPartner(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetPartner")
}

func (stubs) UpsertPartner(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpsertPartnerParams) {
	httperr.NotImplemented(w, r, "UpsertPartner")
}

func (stubs) ListCompanyProfileFields(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ListCompanyProfileFields")
}

func (stubs) UpdateCompanyProfileField(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, field crmcontracts.ProfileFieldKey, params crmcontracts.UpdateCompanyProfileFieldParams) {
	httperr.NotImplemented(w, r, "UpdateCompanyProfileField")
}

func (stubs) ConfirmCompanyProfileField(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, field crmcontracts.ProfileFieldKey, params crmcontracts.ConfirmCompanyProfileFieldParams) {
	httperr.NotImplemented(w, r, "ConfirmCompanyProfileField")
}

func (stubs) RejectCompany(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.RejectCompanyParams) {
	httperr.NotImplemented(w, r, "RejectCompany")
}

func (stubs) GetCompanyScan(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetCompanyScan")
}

func (stubs) EnsureCompanyScan(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "EnsureCompanyScan")
}

func (stubs) GetLatestSiteRead(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetLatestSiteRead")
}

func (stubs) GetSiteRead(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, readId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "GetSiteRead")
}

func (stubs) GetCompanyStrength(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetCompanyStrength")
}

func (stubs) DismissCompanySuggestion(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DismissCompanySuggestion")
}

func (stubs) TechnicalEnrichCompany(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "TechnicalEnrichCompany")
}

func (stubs) GetLatestTechnicalEnrich(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetLatestTechnicalEnrich")
}

func (stubs) GetCompanyVatCheck(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetCompanyVatCheck")
}

func (stubs) RequestCompanyVatCheck(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "RequestCompanyVatCheck")
}

func (stubs) AcknowledgeCompanyView(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "AcknowledgeCompanyView")
}

func (stubs) GetAnchorCompany(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetAnchorCompany")
}

func (stubs) PutAnchorCompany(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "PutAnchorCompany")
}

func (stubs) GetAnchorCompanyContext(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.GetAnchorCompanyContextParams) {
	httperr.NotImplemented(w, r, "GetAnchorCompanyContext")
}

func (stubs) GetAnchorCompanyContextCapabilities(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetAnchorCompanyContextCapabilities")
}

func (stubs) DeleteAnchorCompanyLogo(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "DeleteAnchorCompanyLogo")
}

func (stubs) UploadAnchorCompanyLogo(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "UploadAnchorCompanyLogo")
}

func (stubs) DeleteAnchorCompanyLogoIcon(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "DeleteAnchorCompanyLogoIcon")
}

func (stubs) UploadAnchorCompanyLogoIcon(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "UploadAnchorCompanyLogoIcon")
}

func (stubs) StartAnchorCompanySiteRead(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.StartAnchorCompanySiteReadParams) {
	httperr.NotImplemented(w, r, "StartAnchorCompanySiteRead")
}

func (stubs) GetAnchorCompanySiteRead(w nethttp.ResponseWriter, r *nethttp.Request, readId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "GetAnchorCompanySiteRead")
}

func (stubs) ConfirmAnchorCompanySiteRead(w nethttp.ResponseWriter, r *nethttp.Request, readId openapi_types.UUID, params crmcontracts.ConfirmAnchorCompanySiteReadParams) {
	httperr.NotImplemented(w, r, "ConfirmAnchorCompanySiteRead")
}

func (stubs) GetAnchorCompanySiteReadLogo(w nethttp.ResponseWriter, r *nethttp.Request, readId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "GetAnchorCompanySiteReadLogo")
}

func (stubs) MessageAnchorCompanySiteRead(w nethttp.ResponseWriter, r *nethttp.Request, readId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "MessageAnchorCompanySiteRead")
}

func (stubs) ListConnectors(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListConnectors")
}

func (stubs) CancelConnectorBackfill(w nethttp.ResponseWriter, r *nethttp.Request, provider crmcontracts.CaptureProvider) {
	httperr.NotImplemented(w, r, "CancelConnectorBackfill")
}

func (stubs) GetConnectorBackfillStatus(w nethttp.ResponseWriter, r *nethttp.Request, provider crmcontracts.CaptureProvider) {
	httperr.NotImplemented(w, r, "GetConnectorBackfillStatus")
}

func (stubs) StartConnectorBackfill(w nethttp.ResponseWriter, r *nethttp.Request, provider crmcontracts.CaptureProvider) {
	httperr.NotImplemented(w, r, "StartConnectorBackfill")
}

func (stubs) PreviewConnectorBackfill(w nethttp.ResponseWriter, r *nethttp.Request, provider crmcontracts.CaptureProvider) {
	httperr.NotImplemented(w, r, "PreviewConnectorBackfill")
}

func (stubs) ConnectorOAuthCallback(w nethttp.ResponseWriter, r *nethttp.Request, provider crmcontracts.CaptureProvider, params crmcontracts.ConnectorOAuthCallbackParams) {
	httperr.NotImplemented(w, r, "ConnectorOAuthCallback")
}

func (stubs) ConnectConnector(w nethttp.ResponseWriter, r *nethttp.Request, provider crmcontracts.CaptureProvider) {
	httperr.NotImplemented(w, r, "ConnectConnector")
}

func (stubs) SetConnectorContextTag(w nethttp.ResponseWriter, r *nethttp.Request, provider crmcontracts.CaptureProvider) {
	httperr.NotImplemented(w, r, "SetConnectorContextTag")
}

func (stubs) DisconnectConnector(w nethttp.ResponseWriter, r *nethttp.Request, provider crmcontracts.CaptureProvider) {
	httperr.NotImplemented(w, r, "DisconnectConnector")
}

func (stubs) SetConnectorMailPosture(w nethttp.ResponseWriter, r *nethttp.Request, provider crmcontracts.CaptureProvider) {
	httperr.NotImplemented(w, r, "SetConnectorMailPosture")
}

func (stubs) SetConnectorSignatureEnrichment(w nethttp.ResponseWriter, r *nethttp.Request, provider crmcontracts.CaptureProvider) {
	httperr.NotImplemented(w, r, "SetConnectorSignatureEnrichment")
}

func (stubs) ListConsentPurposes(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListConsentPurposes")
}

func (stubs) CreateConsentPurpose(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "CreateConsentPurpose")
}

func (stubs) ListContacts(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListContactsParams) {
	httperr.NotImplemented(w, r, "ListContacts")
}

func (stubs) CreateContact(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateContactParams) {
	httperr.NotImplemented(w, r, "CreateContact")
}

func (stubs) QuickCaptureContact(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.QuickCaptureContactParams) {
	httperr.NotImplemented(w, r, "QuickCaptureContact")
}

func (stubs) ImportVCards(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ImportVCards")
}

func (stubs) ArchiveContact(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ArchiveContactParams) {
	httperr.NotImplemented(w, r, "ArchiveContact")
}

func (stubs) GetContact(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetContact")
}

func (stubs) UpdateContact(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateContactParams) {
	httperr.NotImplemented(w, r, "UpdateContact")
}

func (stubs) GetContact360(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.GetContact360Params) {
	httperr.NotImplemented(w, r, "GetContact360")
}

func (stubs) GetContactBrief(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetContactBrief")
}

func (stubs) RegenerateContactBrief(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "RegenerateContactBrief")
}

func (stubs) RecordConversationClaim(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "RecordConversationClaim")
}

func (stubs) GetContactConsent(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetContactConsent")
}

func (stubs) RecordConsent(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.RecordConsentParams) {
	httperr.NotImplemented(w, r, "RecordConsent")
}

func (stubs) RequestDetailsConfirmation(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "RequestDetailsConfirmation")
}

func (stubs) IssueDoubleOptIn(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "IssueDoubleOptIn")
}

func (stubs) GetContactConsentGuard(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetContactConsentGuard")
}

func (stubs) RecordQualifyingEvent(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "RecordQualifyingEvent")
}

func (stubs) SuppressContact(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "SuppressContact")
}

func (stubs) LiftSuppression(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, suppressionId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "LiftSuppression")
}

func (stubs) DraftContactEmail(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DraftContactEmail")
}

func (stubs) CreateContactEnrichmentRun(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.CreateContactEnrichmentRunParams) {
	httperr.NotImplemented(w, r, "CreateContactEnrichmentRun")
}

func (stubs) GetContactEnrichmentRun(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, runId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "GetContactEnrichmentRun")
}

func (stubs) GetContactGraph(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetContactGraph")
}

func (stubs) DraftIntroNote(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DraftIntroNote")
}

func (stubs) ListIntroRequests(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ListIntroRequests")
}

func (stubs) CreateIntroRequest(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "CreateIntroRequest")
}

func (stubs) MergeContact(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.MergeContactParams) {
	httperr.NotImplemented(w, r, "MergeContact")
}

func (stubs) DismissContactMoment(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DismissContactMoment")
}

func (stubs) GetContactNetwork(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "GetContactNetwork")
}

func (stubs) RestoreRelationshipNudge(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "RestoreRelationshipNudge")
}

func (stubs) DismissRelationshipNudge(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DismissRelationshipNudge")
}

func (stubs) GetContactProfileFields(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetContactProfileFields")
}

func (stubs) RestoreContactProfileField(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, field crmcontracts.ContactProfileFieldKey) {
	httperr.NotImplemented(w, r, "RestoreContactProfileField")
}

func (stubs) PublishCapturedContact(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "PublishCapturedContact")
}

func (stubs) RunContactResearch(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "RunContactResearch")
}

func (stubs) SaveContactResearch(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "SaveContactResearch")
}

func (stubs) GetContactStrength(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetContactStrength")
}

func (stubs) AcknowledgeContactView(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "AcknowledgeContactView")
}

func (stubs) CreateContract(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateContractParams) {
	httperr.NotImplemented(w, r, "CreateContract")
}

func (stubs) ArchiveContract(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ArchiveContract")
}

func (stubs) GetContract(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetContract")
}

func (stubs) UpdateContract(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateContractParams) {
	httperr.NotImplemented(w, r, "UpdateContract")
}

func (stubs) CancelContract(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.CancelContractParams) {
	httperr.NotImplemented(w, r, "CancelContract")
}

func (stubs) RenewContract(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.RenewContractParams) {
	httperr.NotImplemented(w, r, "RenewContract")
}

func (stubs) ChangeContractStatus(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ChangeContractStatusParams) {
	httperr.NotImplemented(w, r, "ChangeContractStatus")
}

func (stubs) ListCustomFields(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListCustomFieldsParams) {
	httperr.NotImplemented(w, r, "ListCustomFields")
}

func (stubs) CreateCustomField(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateCustomFieldParams) {
	httperr.NotImplemented(w, r, "CreateCustomField")
}

func (stubs) RenameCustomField(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.RenameCustomFieldParams) {
	httperr.NotImplemented(w, r, "RenameCustomField")
}

func (stubs) UpdateCustomFieldOptions(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateCustomFieldOptionsParams) {
	httperr.NotImplemented(w, r, "UpdateCustomFieldOptions")
}

func (stubs) RetireCustomField(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.RetireCustomFieldParams) {
	httperr.NotImplemented(w, r, "RetireCustomField")
}

func (stubs) ListDataSubjectRequests(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListDataSubjectRequestsParams) {
	httperr.NotImplemented(w, r, "ListDataSubjectRequests")
}

func (stubs) CreateDataSubjectRequest(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateDataSubjectRequestParams) {
	httperr.NotImplemented(w, r, "CreateDataSubjectRequest")
}

func (stubs) UpdateDataSubjectRequest(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "UpdateDataSubjectRequest")
}

func (stubs) DownloadDataSubjectPackage(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "DownloadDataSubjectPackage")
}

func (stubs) ListDealRooms(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListDealRoomsParams) {
	httperr.NotImplemented(w, r, "ListDealRooms")
}

func (stubs) CreateDealRoom(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateDealRoomParams) {
	httperr.NotImplemented(w, r, "CreateDealRoom")
}

func (stubs) ArchiveDealRoom(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ArchiveDealRoomParams) {
	httperr.NotImplemented(w, r, "ArchiveDealRoom")
}

func (stubs) GetDealRoom(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetDealRoom")
}

func (stubs) UpdateDealRoom(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateDealRoomParams) {
	httperr.NotImplemented(w, r, "UpdateDealRoom")
}

func (stubs) CloseDealRoom(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "CloseDealRoom")
}

func (stubs) ListDealRoomDocuments(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ListDealRoomDocuments")
}

func (stubs) AddDealRoomDocument(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "AddDealRoomDocument")
}

func (stubs) RemoveDealRoomDocument(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, documentId openapi_types.UUID, params crmcontracts.RemoveDealRoomDocumentParams) {
	httperr.NotImplemented(w, r, "RemoveDealRoomDocument")
}

func (stubs) UpdateDealRoomDocument(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, documentId openapi_types.UUID, params crmcontracts.UpdateDealRoomDocumentParams) {
	httperr.NotImplemented(w, r, "UpdateDealRoomDocument")
}

func (stubs) SetDealRoomExpiry(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.SetDealRoomExpiryParams) {
	httperr.NotImplemented(w, r, "SetDealRoomExpiry")
}

func (stubs) ListDealRoomParticipants(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ListDealRoomParticipantsParams) {
	httperr.NotImplemented(w, r, "ListDealRoomParticipants")
}

func (stubs) InviteDealRoomParticipant(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "InviteDealRoomParticipant")
}

func (stubs) UpdateDealRoomParticipant(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, participantId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "UpdateDealRoomParticipant")
}

func (stubs) ResendDealRoomInvitation(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, participantId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "ResendDealRoomInvitation")
}

func (stubs) RevokeDealRoomParticipant(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, participantId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "RevokeDealRoomParticipant")
}

func (stubs) PauseDealRoom(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "PauseDealRoom")
}

func (stubs) PreviewDealRoom(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "PreviewDealRoom")
}

func (stubs) ResumeDealRoom(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ResumeDealRoom")
}

func (stubs) ListDealRoomThreads(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ListDealRoomThreadsParams) {
	httperr.NotImplemented(w, r, "ListDealRoomThreads")
}

func (stubs) OpenDealRoomThread(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "OpenDealRoomThread")
}

func (stubs) ReplyDealRoomThread(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, threadId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "ReplyDealRoomThread")
}

func (stubs) ResolveDealRoomThread(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, threadId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "ResolveDealRoomThread")
}

func (stubs) ListDeals(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListDealsParams) {
	httperr.NotImplemented(w, r, "ListDeals")
}

func (stubs) CreateDeal(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateDealParams) {
	httperr.NotImplemented(w, r, "CreateDeal")
}

func (stubs) ArchiveDeal(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ArchiveDealParams) {
	httperr.NotImplemented(w, r, "ArchiveDeal")
}

func (stubs) GetDeal(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetDeal")
}

func (stubs) UpdateDeal(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateDealParams) {
	httperr.NotImplemented(w, r, "UpdateDeal")
}

func (stubs) AdvanceDeal(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.AdvanceDealParams) {
	httperr.NotImplemented(w, r, "AdvanceDeal")
}

func (stubs) GetDealCoverage(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "GetDealCoverage")
}

func (stubs) ListDealDocuments(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ListDealDocumentsParams) {
	httperr.NotImplemented(w, r, "ListDealDocuments")
}

func (stubs) UnhideDealDocument(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, attachmentId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "UnhideDealDocument")
}

func (stubs) HideDealDocument(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, attachmentId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "HideDealDocument")
}

func (stubs) ListDealOffers(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ListDealOffersParams) {
	httperr.NotImplemented(w, r, "ListDealOffers")
}

func (stubs) CreateOffer(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.CreateOfferParams) {
	httperr.NotImplemented(w, r, "CreateOffer")
}

func (stubs) ProposeDealRoles(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ProposeDealRoles")
}

func (stubs) ListStageEvidence(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ListStageEvidence")
}

func (stubs) RevertStageProgression(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, approvalId openapi_types.UUID, params crmcontracts.RevertStageProgressionParams) {
	httperr.NotImplemented(w, r, "RevertStageProgression")
}

func (stubs) ListDealStakeholders(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ListDealStakeholders")
}

func (stubs) GetDealStatus(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.GetDealStatusParams) {
	httperr.NotImplemented(w, r, "GetDealStatus")
}

func (stubs) ListDedupeCandidates(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListDedupeCandidatesParams) {
	httperr.NotImplemented(w, r, "ListDedupeCandidates")
}

func (stubs) GetDedupeCandidate(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "GetDedupeCandidate")
}

func (stubs) DisposeDedupeCandidate(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "DisposeDedupeCandidate")
}

func (stubs) UndoDedupeDisposition(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "UndoDedupeDisposition")
}

func (stubs) GetMorningDigest(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.GetMorningDigestParams) {
	httperr.NotImplemented(w, r, "GetMorningDigest")
}

func (stubs) SendAccountEmail(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.SendAccountEmailParams) {
	httperr.NotImplemented(w, r, "SendAccountEmail")
}

func (stubs) PreviewAccountSendAuthorization(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "PreviewAccountSendAuthorization")
}

func (stubs) EmbedReindexStart(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "EmbedReindexStart")
}

func (stubs) EmbedReindexPreview(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "EmbedReindexPreview")
}

func (stubs) EmbedReindexStatus(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "EmbedReindexStatus")
}

func (stubs) CreateFilteredExport(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "CreateFilteredExport")
}

func (stubs) ListExtensions(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListExtensions")
}

func (stubs) GetFieldHistory(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.GetFieldHistoryParams) {
	httperr.NotImplemented(w, r, "GetFieldHistory")
}

func (stubs) PreviewFilter(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "PreviewFilter")
}

func (stubs) GetFilterVocabulary(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.GetFilterVocabularyParams) {
	httperr.NotImplemented(w, r, "GetFilterVocabulary")
}

func (stubs) GetForecast(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.GetForecastParams) {
	httperr.NotImplemented(w, r, "GetForecast")
}

func (stubs) GetForecastAssurance(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetForecastAssurance")
}

func (stubs) ListInputChecks(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListInputChecks")
}

func (stubs) ResolveInputCheck(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "ResolveInputCheck")
}

func (stubs) ListForecastCalls(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListForecastCallsParams) {
	httperr.NotImplemented(w, r, "ListForecastCalls")
}

func (stubs) RecordForecastCall(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "RecordForecastCall")
}

func (stubs) GetForecastMovement(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.GetForecastMovementParams) {
	httperr.NotImplemented(w, r, "GetForecastMovement")
}

func (stubs) OpenForecastShare(w nethttp.ResponseWriter, r *nethttp.Request, token string) {
	httperr.NotImplemented(w, r, "OpenForecastShare")
}

func (stubs) ExportForecastShare(w nethttp.ResponseWriter, r *nethttp.Request, token string) {
	httperr.NotImplemented(w, r, "ExportForecastShare")
}

func (stubs) CreateForecastShare(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "CreateForecastShare")
}

func (stubs) RevokeForecastShare(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "RevokeForecastShare")
}

func (stubs) ListFxRates(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListFxRatesParams) {
	httperr.NotImplemented(w, r, "ListFxRates")
}

func (stubs) SetFxRate(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "SetFxRate")
}

func (stubs) ProposeFxRateRefresh(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ProposeFxRateRefresh")
}

func (stubs) CreateImportRun(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "CreateImportRun")
}

func (stubs) UploadImportSource(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "UploadImportSource")
}

func (stubs) GetImportRun(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "GetImportRun")
}

func (stubs) ApproveImportRun(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "ApproveImportRun")
}

func (stubs) GetImportRunReport(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "GetImportRunReport")
}

func (stubs) UndoImportRun(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "UndoImportRun")
}

func (stubs) GetAuthenticationPolicy(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetAuthenticationPolicy")
}

func (stubs) GetLicenseEntitlement(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetLicenseEntitlement")
}

func (stubs) DeleteOauthApp(w nethttp.ResponseWriter, r *nethttp.Request, provider string) {
	httperr.NotImplemented(w, r, "DeleteOauthApp")
}

func (stubs) GetOauthApp(w nethttp.ResponseWriter, r *nethttp.Request, provider string) {
	httperr.NotImplemented(w, r, "GetOauthApp")
}

func (stubs) SetOauthApp(w nethttp.ResponseWriter, r *nethttp.Request, provider string) {
	httperr.NotImplemented(w, r, "SetOauthApp")
}

func (stubs) GetSeatUsage(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetSeatUsage")
}

func (stubs) GetInstallationSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetInstallationSettings")
}

func (stubs) UpdateInstallationSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "UpdateInstallationSettings")
}

func (stubs) GetInstallationSetup(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetInstallationSetup")
}

func (stubs) GetIntegrationsSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetIntegrationsSettings")
}

func (stubs) UpdateIntegrationsSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "UpdateIntegrationsSettings")
}

func (stubs) CancelIntroRequest(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "CancelIntroRequest")
}

func (stubs) CompleteIntroRequest(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "CompleteIntroRequest")
}

func (stubs) DecideIntroRequest(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DecideIntroRequest")
}

func (stubs) ListCorpora(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListCorpora")
}

func (stubs) CreateCorpus(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "CreateCorpus")
}

func (stubs) ArchiveCorpus(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ArchiveCorpus")
}

func (stubs) ReadCorpus(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ReadCorpus")
}

func (stubs) UpdateCorpus(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "UpdateCorpus")
}

func (stubs) AskCorpus(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "AskCorpus")
}

func (stubs) ListCorpusDocuments(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ListCorpusDocuments")
}

func (stubs) UploadCorpusDocument(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "UploadCorpusDocument")
}

func (stubs) DeleteCorpusDocument(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DeleteCorpusDocument")
}

func (stubs) DownloadCorpusDocument(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DownloadCorpusDocument")
}

func (stubs) ListLeadDisqualifyReasons(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListLeadDisqualifyReasons")
}

func (stubs) CreateLeadDisqualifyReason(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateLeadDisqualifyReasonParams) {
	httperr.NotImplemented(w, r, "CreateLeadDisqualifyReason")
}

func (stubs) DeleteLeadDisqualifyReason(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DeleteLeadDisqualifyReason")
}

func (stubs) UpdateLeadDisqualifyReason(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateLeadDisqualifyReasonParams) {
	httperr.NotImplemented(w, r, "UpdateLeadDisqualifyReason")
}

func (stubs) ListLeadSources(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListLeadSources")
}

func (stubs) CreateLeadSource(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateLeadSourceParams) {
	httperr.NotImplemented(w, r, "CreateLeadSource")
}

func (stubs) DeleteLeadSource(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DeleteLeadSource")
}

func (stubs) UpdateLeadSource(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateLeadSourceParams) {
	httperr.NotImplemented(w, r, "UpdateLeadSource")
}

func (stubs) ListLeads(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListLeadsParams) {
	httperr.NotImplemented(w, r, "ListLeads")
}

func (stubs) CreateLead(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateLeadParams) {
	httperr.NotImplemented(w, r, "CreateLead")
}

func (stubs) AssignLeads(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "AssignLeads")
}

func (stubs) GetLeadSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetLeadSettings")
}

func (stubs) UpdateLeadSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "UpdateLeadSettings")
}

func (stubs) DisqualifyLead(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DisqualifyLead")
}

func (stubs) GetLead(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetLead")
}

func (stubs) UpdateLead(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateLeadParams) {
	httperr.NotImplemented(w, r, "UpdateLead")
}

func (stubs) DemoteLead(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.DemoteLeadParams) {
	httperr.NotImplemented(w, r, "DemoteLead")
}

func (stubs) DraftLeadEmail(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DraftLeadEmail")
}

func (stubs) ListLeadManualSignals(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ListLeadManualSignals")
}

func (stubs) SetLeadManualSignal(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "SetLeadManualSignal")
}

func (stubs) ClearLeadManualSignal(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, factor string) {
	httperr.NotImplemented(w, r, "ClearLeadManualSignal")
}

func (stubs) PromoteLead(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.PromoteLeadParams) {
	httperr.NotImplemented(w, r, "PromoteLead")
}

func (stubs) PreviewLeadPromotion(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "PreviewLeadPromotion")
}

func (stubs) ReopenLead(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ReopenLeadParams) {
	httperr.NotImplemented(w, r, "ReopenLead")
}

func (stubs) ExplainLeadScore(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ExplainLeadScoreParams) {
	httperr.NotImplemented(w, r, "ExplainLeadScore")
}

func (stubs) GetMagic(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.GetMagicParams) {
	httperr.NotImplemented(w, r, "GetMagic")
}

func (stubs) GetCurrentPrincipal(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetCurrentPrincipal")
}

func (stubs) ListMyAgentGrants(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListMyAgentGrants")
}

func (stubs) SetMyAgentGrant(w nethttp.ResponseWriter, r *nethttp.Request, spec crmcontracts.ScheduledAgentName) {
	httperr.NotImplemented(w, r, "SetMyAgentGrant")
}

func (stubs) GetMyAiActivity(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.GetMyAiActivityParams) {
	httperr.NotImplemented(w, r, "GetMyAiActivity")
}

func (stubs) GetMyBriefDelivery(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetMyBriefDelivery")
}

func (stubs) SaveMyBriefDelivery(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "SaveMyBriefDelivery")
}

func (stubs) SaveMyDisplayName(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "SaveMyDisplayName")
}

func (stubs) GetMyEmailSignature(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetMyEmailSignature")
}

func (stubs) SaveMyEmailSignature(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "SaveMyEmailSignature")
}

func (stubs) GetMyLinkedInAccount(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetMyLinkedInAccount")
}

func (stubs) SaveMyLinkedInAccount(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "SaveMyLinkedInAccount")
}

func (stubs) ImportLinkedInConnections(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ImportLinkedInConnections")
}

func (stubs) GetMyLinkedInReach(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.GetMyLinkedInReachParams) {
	httperr.NotImplemented(w, r, "GetMyLinkedInReach")
}

func (stubs) SaveMyLocale(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "SaveMyLocale")
}

func (stubs) GetMyWorkingHours(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetMyWorkingHours")
}

func (stubs) SaveMyWorkingHours(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "SaveMyWorkingHours")
}

func (stubs) RaiseNotice(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "RaiseNotice")
}

func (stubs) MarkNoticeRead(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "MarkNoticeRead")
}

func (stubs) GetConsentRequest(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.GetConsentRequestParams) {
	httperr.NotImplemented(w, r, "GetConsentRequest")
}

func (stubs) ListOfferTemplates(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListOfferTemplatesParams) {
	httperr.NotImplemented(w, r, "ListOfferTemplates")
}

func (stubs) CreateOfferTemplate(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateOfferTemplateParams) {
	httperr.NotImplemented(w, r, "CreateOfferTemplate")
}

func (stubs) ArchiveOfferTemplate(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ArchiveOfferTemplate")
}

func (stubs) GetOfferTemplate(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetOfferTemplate")
}

func (stubs) UpdateOfferTemplate(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateOfferTemplateParams) {
	httperr.NotImplemented(w, r, "UpdateOfferTemplate")
}

func (stubs) ArchiveOffer(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ArchiveOffer")
}

func (stubs) GetOffer(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetOffer")
}

func (stubs) UpdateOffer(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateOfferParams) {
	httperr.NotImplemented(w, r, "UpdateOffer")
}

func (stubs) AcceptOffer(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.AcceptOfferParams) {
	httperr.NotImplemented(w, r, "AcceptOffer")
}

func (stubs) AddOfferLineItem(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "AddOfferLineItem")
}

func (stubs) RemoveOfferLineItem(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, lineItemId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "RemoveOfferLineItem")
}

func (stubs) UpdateOfferLineItem(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, lineItemId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "UpdateOfferLineItem")
}

func (stubs) DownloadOfferPdf(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DownloadOfferPdf")
}

func (stubs) RegenerateOffer(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.RegenerateOfferParams) {
	httperr.NotImplemented(w, r, "RegenerateOffer")
}

func (stubs) RejectOffer(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.RejectOfferParams) {
	httperr.NotImplemented(w, r, "RejectOffer")
}

func (stubs) RenderOffer(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.RenderOfferParams) {
	httperr.NotImplemented(w, r, "RenderOffer")
}

func (stubs) SendOffer(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.SendOfferParams) {
	httperr.NotImplemented(w, r, "SendOffer")
}

func (stubs) MessageOnboardingCompany(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "MessageOnboardingCompany")
}

func (stubs) GetOnboardingCompanyProposal(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.GetOnboardingCompanyProposalParams) {
	httperr.NotImplemented(w, r, "GetOnboardingCompanyProposal")
}

func (stubs) GetOnboardingState(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetOnboardingState")
}

func (stubs) PutOnboardingState(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.PutOnboardingStateParams) {
	httperr.NotImplemented(w, r, "PutOnboardingState")
}

func (stubs) GetOverlayBudget(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetOverlayBudget")
}

func (stubs) DisconnectOverlay(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "DisconnectOverlay")
}

func (stubs) GetOverlayConnection(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetOverlayConnection")
}

func (stubs) ConnectOverlay(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ConnectOverlay")
}

func (stubs) DownloadOverlayExport(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "DownloadOverlayExport")
}

func (stubs) ExecuteOverlayFlip(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ExecuteOverlayFlip")
}

func (stubs) PreflightOverlayFlip(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "PreflightOverlayFlip")
}

func (stubs) ListOverlayOwners(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListOverlayOwners")
}

func (stubs) ReconcileOverlay(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ReconcileOverlay")
}

func (stubs) GetOverlaySyncStatus(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetOverlaySyncStatus")
}

func (stubs) ListOverlayUserMap(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListOverlayUserMapParams) {
	httperr.NotImplemented(w, r, "ListOverlayUserMap")
}

func (stubs) DeleteOverlayUserMap(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DeleteOverlayUserMap")
}

func (stubs) SetOverlayUserMap(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "SetOverlayUserMap")
}

func (stubs) ListPartners(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListPartnersParams) {
	httperr.NotImplemented(w, r, "ListPartners")
}

func (stubs) ListPassports(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListPassports")
}

func (stubs) IssuePassport(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "IssuePassport")
}

func (stubs) RevokePassport(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "RevokePassport")
}

func (stubs) ListPipelines(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListPipelinesParams) {
	httperr.NotImplemented(w, r, "ListPipelines")
}

func (stubs) CreatePipeline(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreatePipelineParams) {
	httperr.NotImplemented(w, r, "CreatePipeline")
}

func (stubs) ArchivePipeline(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ArchivePipelineParams) {
	httperr.NotImplemented(w, r, "ArchivePipeline")
}

func (stubs) GetPipeline(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetPipeline")
}

func (stubs) UpdatePipeline(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdatePipelineParams) {
	httperr.NotImplemented(w, r, "UpdatePipeline")
}

func (stubs) RestorePipeline(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "RestorePipeline")
}

func (stubs) ListProducts(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListProductsParams) {
	httperr.NotImplemented(w, r, "ListProducts")
}

func (stubs) CreateProduct(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateProductParams) {
	httperr.NotImplemented(w, r, "CreateProduct")
}

func (stubs) ArchiveProduct(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ArchiveProduct")
}

func (stubs) GetProduct(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetProduct")
}

func (stubs) UpdateProduct(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateProductParams) {
	httperr.NotImplemented(w, r, "UpdateProduct")
}

func (stubs) ListProjects(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListProjectsParams) {
	httperr.NotImplemented(w, r, "ListProjects")
}

func (stubs) CreateProject(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateProjectParams) {
	httperr.NotImplemented(w, r, "CreateProject")
}

func (stubs) TransferProjectOwnership(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.TransferProjectOwnershipParams) {
	httperr.NotImplemented(w, r, "TransferProjectOwnership")
}

func (stubs) ArchiveProject(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ArchiveProjectParams) {
	httperr.NotImplemented(w, r, "ArchiveProject")
}

func (stubs) GetProject(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetProject")
}

func (stubs) UpdateProject(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateProjectParams) {
	httperr.NotImplemented(w, r, "UpdateProject")
}

func (stubs) GetProject360(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetProject360")
}

func (stubs) AdvanceProjectPhase(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.AdvanceProjectPhaseParams) {
	httperr.NotImplemented(w, r, "AdvanceProjectPhase")
}

func (stubs) SetProjectCompany(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.SetProjectCompanyParams) {
	httperr.NotImplemented(w, r, "SetProjectCompany")
}

func (stubs) RemoveProjectCompany(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, companyId openapi_types.UUID, params crmcontracts.RemoveProjectCompanyParams) {
	httperr.NotImplemented(w, r, "RemoveProjectCompany")
}

func (stubs) ListProjectStakeholders(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ListProjectStakeholders")
}

func (stubs) SetProjectStakeholder(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.SetProjectStakeholderParams) {
	httperr.NotImplemented(w, r, "SetProjectStakeholder")
}

func (stubs) RemoveProjectStakeholder(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, contactId openapi_types.UUID, params crmcontracts.RemoveProjectStakeholderParams) {
	httperr.NotImplemented(w, r, "RemoveProjectStakeholder")
}

func (stubs) ListProviderConnections(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListProviderConnections")
}

func (stubs) DisconnectProvider(w nethttp.ResponseWriter, r *nethttp.Request, provider crmcontracts.Provider) {
	httperr.NotImplemented(w, r, "DisconnectProvider")
}

func (stubs) UpdateProviderConnection(w nethttp.ResponseWriter, r *nethttp.Request, provider crmcontracts.Provider, params crmcontracts.UpdateProviderConnectionParams) {
	httperr.NotImplemented(w, r, "UpdateProviderConnection")
}

func (stubs) ConnectProvider(w nethttp.ResponseWriter, r *nethttp.Request, provider crmcontracts.Provider, params crmcontracts.ConnectProviderParams) {
	httperr.NotImplemented(w, r, "ConnectProvider")
}

func (stubs) DeleteProviderData(w nethttp.ResponseWriter, r *nethttp.Request, provider crmcontracts.Provider) {
	httperr.NotImplemented(w, r, "DeleteProviderData")
}

func (stubs) BookPublicMeeting(w nethttp.ResponseWriter, r *nethttp.Request, hostSlug string, params crmcontracts.BookPublicMeetingParams) {
	httperr.NotImplemented(w, r, "BookPublicMeeting")
}

func (stubs) GetPublicAvailability(w nethttp.ResponseWriter, r *nethttp.Request, hostSlug string, params crmcontracts.GetPublicAvailabilityParams) {
	httperr.NotImplemented(w, r, "GetPublicAvailability")
}

func (stubs) GetConfirmDetails(w nethttp.ResponseWriter, r *nethttp.Request, token string) {
	httperr.NotImplemented(w, r, "GetConfirmDetails")
}

func (stubs) SubmitConfirmDetails(w nethttp.ResponseWriter, r *nethttp.Request, token string) {
	httperr.NotImplemented(w, r, "SubmitConfirmDetails")
}

func (stubs) GetPreferenceCenter(w nethttp.ResponseWriter, r *nethttp.Request, token string) {
	httperr.NotImplemented(w, r, "GetPreferenceCenter")
}

func (stubs) UpdatePreferences(w nethttp.ResponseWriter, r *nethttp.Request, token string) {
	httperr.NotImplemented(w, r, "UpdatePreferences")
}

func (stubs) OneClickUnsubscribe(w nethttp.ResponseWriter, r *nethttp.Request, token string, params crmcontracts.OneClickUnsubscribeParams) {
	httperr.NotImplemented(w, r, "OneClickUnsubscribe")
}

func (stubs) ListBuyerRoomDocuments(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListBuyerRoomDocuments")
}

func (stubs) DownloadBuyerRoomDocument(w nethttp.ResponseWriter, r *nethttp.Request, documentId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "DownloadBuyerRoomDocument")
}

func (stubs) ExchangeDealRoomCredential(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ExchangeDealRoomCredential")
}

func (stubs) RequestDealRoomLink(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "RequestDealRoomLink")
}

func (stubs) GetBuyerRoom(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetBuyerRoom")
}

func (stubs) PeekDealRoomCredential(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "PeekDealRoomCredential")
}

func (stubs) SignOutBuyerRoom(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "SignOutBuyerRoom")
}

func (stubs) ListBuyerRoomThreads(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListBuyerRoomThreadsParams) {
	httperr.NotImplemented(w, r, "ListBuyerRoomThreads")
}

func (stubs) OpenBuyerRoomThread(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "OpenBuyerRoomThread")
}

func (stubs) ReplyBuyerRoomThread(w nethttp.ResponseWriter, r *nethttp.Request, threadId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "ReplyBuyerRoomThread")
}

func (stubs) ListRecordGrants(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListRecordGrantsParams) {
	httperr.NotImplemented(w, r, "ListRecordGrants")
}

func (stubs) CreateRecordGrant(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateRecordGrantParams) {
	httperr.NotImplemented(w, r, "CreateRecordGrant")
}

func (stubs) RevokeRecordGrant(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.RevokeRecordGrantParams) {
	httperr.NotImplemented(w, r, "RevokeRecordGrant")
}

func (stubs) ListRecordRoles(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListRecordRoles")
}

func (stubs) CreateRecordRole(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateRecordRoleParams) {
	httperr.NotImplemented(w, r, "CreateRecordRole")
}

func (stubs) UpdateRecordRole(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateRecordRoleParams) {
	httperr.NotImplemented(w, r, "UpdateRecordRole")
}

func (stubs) GetRecordTags(w nethttp.ResponseWriter, r *nethttp.Request, entityType string, entityId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "GetRecordTags")
}

func (stubs) GetRecordContext(w nethttp.ResponseWriter, r *nethttp.Request, entityType string, id crmcontracts.Id, params crmcontracts.GetRecordContextParams) {
	httperr.NotImplemented(w, r, "GetRecordContext")
}

func (stubs) GetRecordHistory(w nethttp.ResponseWriter, r *nethttp.Request, entityType string, id crmcontracts.Id, params crmcontracts.GetRecordHistoryParams) {
	httperr.NotImplemented(w, r, "GetRecordHistory")
}

func (stubs) RestoreRecordChange(w nethttp.ResponseWriter, r *nethttp.Request, entityType string, id crmcontracts.Id, auditId openapi_types.UUID, params crmcontracts.RestoreRecordChangeParams) {
	httperr.NotImplemented(w, r, "RestoreRecordChange")
}

func (stubs) ClaimRecord(w nethttp.ResponseWriter, r *nethttp.Request, recordType string, id crmcontracts.Id, params crmcontracts.ClaimRecordParams) {
	httperr.NotImplemented(w, r, "ClaimRecord")
}

func (stubs) ListRecordAssignments(w nethttp.ResponseWriter, r *nethttp.Request, recordType crmcontracts.AssignmentRecordType, recordId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "ListRecordAssignments")
}

func (stubs) CreateRecordAssignment(w nethttp.ResponseWriter, r *nethttp.Request, recordType crmcontracts.AssignmentRecordType, recordId openapi_types.UUID, params crmcontracts.CreateRecordAssignmentParams) {
	httperr.NotImplemented(w, r, "CreateRecordAssignment")
}

func (stubs) ListRelationships(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListRelationshipsParams) {
	httperr.NotImplemented(w, r, "ListRelationships")
}

func (stubs) CreateRelationship(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "CreateRelationship")
}

func (stubs) ArchiveRelationship(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ArchiveRelationshipParams) {
	httperr.NotImplemented(w, r, "ArchiveRelationship")
}

func (stubs) UpdateRelationship(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateRelationshipParams) {
	httperr.NotImplemented(w, r, "UpdateRelationship")
}

func (stubs) RunReport(w nethttp.ResponseWriter, r *nethttp.Request, report string) {
	httperr.NotImplemented(w, r, "RunReport")
}

func (stubs) ExplainReport(w nethttp.ResponseWriter, r *nethttp.Request, report string, params crmcontracts.ExplainReportParams) {
	httperr.NotImplemented(w, r, "ExplainReport")
}

func (stubs) ListRetentionPolicies(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListRetentionPolicies")
}

func (stubs) CreateRetentionPolicy(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "CreateRetentionPolicy")
}

func (stubs) DeleteRetentionPolicy(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DeleteRetentionPolicy")
}

func (stubs) UpdateRetentionPolicy(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "UpdateRetentionPolicy")
}

func (stubs) ListRestrictedActivities(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListRestrictedActivitiesParams) {
	httperr.NotImplemented(w, r, "ListRestrictedActivities")
}

func (stubs) PinActivityToFloor(w nethttp.ResponseWriter, r *nethttp.Request, activityId openapi_types.UUID, params crmcontracts.PinActivityToFloorParams) {
	httperr.NotImplemented(w, r, "PinActivityToFloor")
}

func (stubs) ReleaseRestrictedActivity(w nethttp.ResponseWriter, r *nethttp.Request, activityId openapi_types.UUID, params crmcontracts.ReleaseRestrictedActivityParams) {
	httperr.NotImplemented(w, r, "ReleaseRestrictedActivity")
}

func (stubs) GetRetentionSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetRetentionSettings")
}

func (stubs) UpdateRetentionSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "UpdateRetentionSettings")
}

func (stubs) ListRoles(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListRoles")
}

func (stubs) SetRoleObjectGrant(w nethttp.ResponseWriter, r *nethttp.Request, key string, object string, params crmcontracts.SetRoleObjectGrantParams) {
	httperr.NotImplemented(w, r, "SetRoleObjectGrant")
}

func (stubs) ListScheduledSends(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListScheduledSendsParams) {
	httperr.NotImplemented(w, r, "ListScheduledSends")
}

func (stubs) GetScheduledSend(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetScheduledSend")
}

func (stubs) RescheduleScheduledSend(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.RescheduleScheduledSendParams) {
	httperr.NotImplemented(w, r, "RescheduleScheduledSend")
}

func (stubs) CancelScheduledSend(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "CancelScheduledSend")
}

func (stubs) Search(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.SearchParams) {
	httperr.NotImplemented(w, r, "Search")
}

func (stubs) ListSignals(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListSignalsParams) {
	httperr.NotImplemented(w, r, "ListSignals")
}

func (stubs) CreateSignal(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateSignalParams) {
	httperr.NotImplemented(w, r, "CreateSignal")
}

func (stubs) ArchiveSignal(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ArchiveSignal")
}

func (stubs) GetSignal(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetSignal")
}

func (stubs) UpdateSignal(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateSignalParams) {
	httperr.NotImplemented(w, r, "UpdateSignal")
}

func (stubs) GetSignalIntroPath(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetSignalIntroPath")
}

func (stubs) ResolveSignal(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ResolveSignalParams) {
	httperr.NotImplemented(w, r, "ResolveSignal")
}

func (stubs) GetSignalWarmth(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetSignalWarmth")
}

func (stubs) ListTransitionPolicies(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ListTransitionPolicies")
}

func (stubs) SetTransitionPolicy(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.SetTransitionPolicyParams) {
	httperr.NotImplemented(w, r, "SetTransitionPolicy")
}

func (stubs) ResumeTransitionPolicy(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ResumeTransitionPolicy")
}

func (stubs) GetStageAutomationReport(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.GetStageAutomationReportParams) {
	httperr.NotImplemented(w, r, "GetStageAutomationReport")
}

func (stubs) ListStages(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListStagesParams) {
	httperr.NotImplemented(w, r, "ListStages")
}

func (stubs) CreateStage(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateStageParams) {
	httperr.NotImplemented(w, r, "CreateStage")
}

func (stubs) ArchiveStage(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ArchiveStageParams) {
	httperr.NotImplemented(w, r, "ArchiveStage")
}

func (stubs) GetStage(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetStage")
}

func (stubs) UpdateStage(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateStageParams) {
	httperr.NotImplemented(w, r, "UpdateStage")
}

func (stubs) ListStageExitCriteria(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ListStageExitCriteriaParams) {
	httperr.NotImplemented(w, r, "ListStageExitCriteria")
}

func (stubs) CreateStageExitCriterion(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.CreateStageExitCriterionParams) {
	httperr.NotImplemented(w, r, "CreateStageExitCriterion")
}

func (stubs) ArchiveStageExitCriterion(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, criterionId openapi_types.UUID, params crmcontracts.ArchiveStageExitCriterionParams) {
	httperr.NotImplemented(w, r, "ArchiveStageExitCriterion")
}

func (stubs) UpdateStageExitCriterion(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, criterionId openapi_types.UUID, params crmcontracts.UpdateStageExitCriterionParams) {
	httperr.NotImplemented(w, r, "UpdateStageExitCriterion")
}

func (stubs) ListTags(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListTagsParams) {
	httperr.NotImplemented(w, r, "ListTags")
}

func (stubs) CreateTag(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "CreateTag")
}

func (stubs) ArchiveTag(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ArchiveTag")
}

func (stubs) GetTag(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetTag")
}

func (stubs) UpdateTag(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateTagParams) {
	httperr.NotImplemented(w, r, "UpdateTag")
}

func (stubs) RemoveTag(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "RemoveTag")
}

func (stubs) ApplyTag(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ApplyTag")
}

func (stubs) MergeTags(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "MergeTags")
}

func (stubs) RestoreTag(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "RestoreTag")
}

func (stubs) CreateTask(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateTaskParams) {
	httperr.NotImplemented(w, r, "CreateTask")
}

func (stubs) ListTeams(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListTeamsParams) {
	httperr.NotImplemented(w, r, "ListTeams")
}

func (stubs) CreateTeam(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "CreateTeam")
}

func (stubs) UpdateTeam(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "UpdateTeam")
}

func (stubs) RemoveTeamMember(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, userId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "RemoveTeamMember")
}

func (stubs) AddTeamMember(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, userId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "AddTeamMember")
}

func (stubs) ListUsers(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListUsersParams) {
	httperr.NotImplemented(w, r, "ListUsers")
}

func (stubs) InviteUser(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "InviteUser")
}

func (stubs) PreviewAccess(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.PreviewAccessParams) {
	httperr.NotImplemented(w, r, "PreviewAccess")
}

func (stubs) GetUserAccess(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetUserAccess")
}

func (stubs) DeactivateUser(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "DeactivateUser")
}

func (stubs) IssueUserPasswordLink(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "IssueUserPasswordLink")
}

func (stubs) ReactivateUser(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ReactivateUser")
}

func (stubs) ChangeUserRole(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ChangeUserRole")
}

func (stubs) ListSavedViews(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListSavedViewsParams) {
	httperr.NotImplemented(w, r, "ListSavedViews")
}

func (stubs) CreateSavedView(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "CreateSavedView")
}

func (stubs) ArchiveSavedView(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ArchiveSavedView")
}

func (stubs) GetSavedView(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetSavedView")
}

func (stubs) UpdateSavedView(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateSavedViewParams) {
	httperr.NotImplemented(w, r, "UpdateSavedView")
}

func (stubs) ListVoiceProfiles(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListVoiceProfiles")
}

func (stubs) CreateVoiceProfile(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateVoiceProfileParams) {
	httperr.NotImplemented(w, r, "CreateVoiceProfile")
}

func (stubs) DeleteVoiceProfile(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.DeleteVoiceProfileParams) {
	httperr.NotImplemented(w, r, "DeleteVoiceProfile")
}

func (stubs) GetVoiceProfile(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetVoiceProfile")
}

func (stubs) UpdateVoiceProfile(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateVoiceProfileParams) {
	httperr.NotImplemented(w, r, "UpdateVoiceProfile")
}

func (stubs) CreateVoiceBuild(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.CreateVoiceBuildParams) {
	httperr.NotImplemented(w, r, "CreateVoiceBuild")
}

func (stubs) GetVoiceBuild(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, buildId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "GetVoiceBuild")
}

func (stubs) ClearVoiceCorpus(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ClearVoiceCorpusParams) {
	httperr.NotImplemented(w, r, "ClearVoiceCorpus")
}

func (stubs) ListVoiceProfileDeltas(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ListVoiceProfileDeltasParams) {
	httperr.NotImplemented(w, r, "ListVoiceProfileDeltas")
}

func (stubs) RejectVoiceDraft(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.RejectVoiceDraftParams) {
	httperr.NotImplemented(w, r, "RejectVoiceDraft")
}

func (stubs) GetVoiceLearningSummary(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetVoiceLearningSummary")
}

func (stubs) ListVoiceCorpusSources(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ListVoiceCorpusSources")
}

func (stubs) IngestVoiceCorpusSource(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.IngestVoiceCorpusSourceParams) {
	httperr.NotImplemented(w, r, "IngestVoiceCorpusSource")
}

func (stubs) PreviewVoiceCorpusSource(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "PreviewVoiceCorpusSource")
}

func (stubs) DeleteVoiceCorpusSource(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, sourceId openapi_types.UUID, params crmcontracts.DeleteVoiceCorpusSourceParams) {
	httperr.NotImplemented(w, r, "DeleteVoiceCorpusSource")
}

func (stubs) UpdateVoiceCorpusSource(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, sourceId openapi_types.UUID, params crmcontracts.UpdateVoiceCorpusSourceParams) {
	httperr.NotImplemented(w, r, "UpdateVoiceCorpusSource")
}

func (stubs) ListVoiceProfileVersions(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ListVoiceProfileVersionsParams) {
	httperr.NotImplemented(w, r, "ListVoiceProfileVersions")
}

func (stubs) ApplyVoiceProfileVersion(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, profileVersion crmcontracts.VoiceProfileVersionNumber, params crmcontracts.ApplyVoiceProfileVersionParams) {
	httperr.NotImplemented(w, r, "ApplyVoiceProfileVersion")
}

func (stubs) RejectVoiceProfileVersion(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, profileVersion crmcontracts.VoiceProfileVersionNumber, params crmcontracts.RejectVoiceProfileVersionParams) {
	httperr.NotImplemented(w, r, "RejectVoiceProfileVersion")
}

func (stubs) RollbackVoiceProfileVersion(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, profileVersion crmcontracts.VoiceProfileVersionNumber, params crmcontracts.RollbackVoiceProfileVersionParams) {
	httperr.NotImplemented(w, r, "RollbackVoiceProfileVersion")
}

func (stubs) ListWebhookSubscriptions(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.ListWebhookSubscriptionsParams) {
	httperr.NotImplemented(w, r, "ListWebhookSubscriptions")
}

func (stubs) CreateWebhookSubscription(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.CreateWebhookSubscriptionParams) {
	httperr.NotImplemented(w, r, "CreateWebhookSubscription")
}

func (stubs) ArchiveWebhookSubscription(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "ArchiveWebhookSubscription")
}

func (stubs) GetWebhookSubscription(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "GetWebhookSubscription")
}

func (stubs) UpdateWebhookSubscription(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.UpdateWebhookSubscriptionParams) {
	httperr.NotImplemented(w, r, "UpdateWebhookSubscription")
}

func (stubs) ListWebhookDeliveries(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, params crmcontracts.ListWebhookDeliveriesParams) {
	httperr.NotImplemented(w, r, "ListWebhookDeliveries")
}

func (stubs) ReplayWebhookDelivery(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id, deliveryId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "ReplayWebhookDelivery")
}

func (stubs) RotateWebhookSecret(w nethttp.ResponseWriter, r *nethttp.Request, id crmcontracts.Id) {
	httperr.NotImplemented(w, r, "RotateWebhookSecret")
}

func (stubs) AddWeeklyPlanCommitment(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "AddWeeklyPlanCommitment")
}

func (stubs) EditWeeklyPlanCommitment(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "EditWeeklyPlanCommitment")
}

func (stubs) AskForWeeklyPlanHelp(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "AskForWeeklyPlanHelp")
}

func (stubs) AnswerWeeklyPlanCommitment(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "AnswerWeeklyPlanCommitment")
}

func (stubs) SetWeeklyPlanCommitmentState(w nethttp.ResponseWriter, r *nethttp.Request, id openapi_types.UUID) {
	httperr.NotImplemented(w, r, "SetWeeklyPlanCommitmentState")
}

func (stubs) GetCurrentWeeklyPlan(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetCurrentWeeklyPlan")
}

func (stubs) StartWeeklyPlan(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "StartWeeklyPlan")
}

func (stubs) SetWeeklyPlanContract(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "SetWeeklyPlanContract")
}

func (stubs) GetTeammateWeeklyPlan(w nethttp.ResponseWriter, r *nethttp.Request, ownerId openapi_types.UUID) {
	httperr.NotImplemented(w, r, "GetTeammateWeeklyPlan")
}

func (stubs) ListWeeklyReviews(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "ListWeeklyReviews")
}

func (stubs) GetLatestWeeklyReview(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.GetLatestWeeklyReviewParams) {
	httperr.NotImplemented(w, r, "GetLatestWeeklyReview")
}

func (stubs) GetTeamWeeklyReview(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.GetTeamWeeklyReviewParams) {
	httperr.NotImplemented(w, r, "GetTeamWeeklyReview")
}

func (stubs) GetWorklist(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.GetWorklistParams) {
	httperr.NotImplemented(w, r, "GetWorklist")
}

func (stubs) GetTeamExceptions(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetTeamExceptions")
}

func (stubs) GetHandledForYou(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetHandledForYou")
}

func (stubs) GetHiddenBacklog(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetHiddenBacklog")
}

func (stubs) UnpinWorklistRow(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.UnpinWorklistRowParams) {
	httperr.NotImplemented(w, r, "UnpinWorklistRow")
}

func (stubs) PinWorklistRow(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "PinWorklistRow")
}

func (stubs) GetResponseMetrics(w nethttp.ResponseWriter, r *nethttp.Request, params crmcontracts.GetResponseMetricsParams) {
	httperr.NotImplemented(w, r, "GetResponseMetrics")
}

func (stubs) GetTeamBoard(w nethttp.ResponseWriter, r *nethttp.Request) {
	httperr.NotImplemented(w, r, "GetTeamBoard")
}
