# Which role can use which MCP tool

<!-- Generated together with mcp-tool-roles.json; do not edit by hand. -->

An agent has the access of the user it acts for. This page shows what that gives on each role a new
workspace starts with. [rbac-matrix.md](rbac-matrix.md) lists the grants themselves;
[agent-tools.md](agent-tools.md) lists the tools.

Nobody types this table. `TestTheToolRoleMatrixIsPublished` calls every tool and action as a passport on
each role, on records that user owns where the sweep can make one, and writes
[mcp-tool-roles.json](mcp-tool-roles.json). The test fails when the answers change and the file does not.

| Mark | Meaning |
|---|---|
| ✓ | The call ran for this role. |
| ✗ | A gate refused this role. |
| ? | Not proven either way. The call stopped first: no such record, an invalid body, a service this test does not start, or a relation it lacks. |

A cell with counts, such as `5✓ 2✗ 3?`, covers the actions of one router. Row scope, the seat and the passport's
scopes still apply to each call. A ✓ says the role passed on a record it owns, not on every record.

Regenerate from the repository root, then from `backend/`:

    UPDATE_MCP_TOOL_ROLES=1 make test-it DIR=backend/internal/compose/integration RUN=TestTheToolRoleMatrixIsPublished
    go test ./internal/compose/ -run TestTheToolRolePageIsTheMatrix -update-mcp-tool-roles-page

| Tool | admin | management | manager | ops | read_only | rep | Actions some role is refused |
|---|---|---|---|---|---|---|---|
| `annotate_brief` | ? | ✓ | ✓ | ✓ | ✓ | ✓ |  |
| `archive_record` | ? | ? | ? | ? | ✗ | ✗ |  |
| `bulk_update_records` | ? | ? | ? | ? | ? | ? |  |
| `change_access` | 1✓ 22? | 12✗ 11? | 12✗ 11? | 10✗ 13? | 16✗ 7? | 12✗ 11? | <details><summary>16</summary>`add_team_member` (management, manager, ops, read_only, rep) `assign` (read_only) `assign_legal_entity` (management, manager, ops, read_only, rep) `change_role` (management, manager, ops, read_only, rep) `claim_record` (read_only) `create_record_role` (management, manager, read_only, rep) `create_team` (management, manager, ops, read_only, rep) `deactivate_member` (management, manager, ops, read_only, rep) `hand_over_work` (management, manager, ops, read_only, rep) `reactivate_member` (management, manager, ops, read_only, rep) `remove_member` (management, manager, ops, read_only, rep) `remove_team_member` (management, manager, ops, read_only, rep) `request_ownership` (read_only) `share` (read_only) `update_record_role` (management, manager, read_only, rep) `update_team` (management, manager, ops, read_only, rep)</details> |
| `change_activities` | 9✓ 17? | 9✓ 17? | 9✓ 17? | 9✓ 17? | 2✓ 12✗ 12? | 9✓ 17? | <details><summary>12</summary>`cancel_send` (read_only) `check_send` (read_only) `discard_draft` (read_only) `hide_document` (read_only) `log` (read_only) `relink` (read_only) `request_document_access` (read_only) `save_draft` (read_only) `set_audience` (read_only) `set_thread_audience` (read_only) `show_document` (read_only) `task` (read_only)</details> |
| `change_attention` | 5✓ 28? | 5✓ 28? | 5✓ 28? | 5✓ 28? | 3✓ 8✗ 22? | 5✓ 28? | <details><summary>8</summary>`accept_deal_suggestion` (read_only) `add_commitment` (read_only) `answer_teammate` (read_only) `ask_for_help` (read_only) `dismiss_deal_suggestion` (read_only) `edit_commitment` (read_only) `set_commitment_state` (read_only) `start_weekly_plan` (read_only)</details> |
| `change_lists` | ? | ? | ? | ? | ? | ? |  |
| `change_offers` | 2✓ 22? | 2✓ 22? | 2✓ 22? | 2✓ 22? | 21✗ 3? | 2✓ 3✗ 19? | <details><summary>21</summary>`accept_offer` (read_only) `archive_contract` (read_only, rep) `archive_room` (read_only, rep) `close_room` (read_only) `pause_room` (read_only) `record_contract` (read_only) `regenerate_offer` (read_only) `reject_offer` (read_only) `remove_room_document` (read_only, rep) `render_offer` (read_only) `renew_contract` (read_only) `resolve_room_thread` (read_only) `resume_room` (read_only) `revoke_room_participant` (read_only) `send_offer` (read_only) `set_contract_status` (read_only) `set_deal_products` (read_only) `update_contract` (read_only) `update_room` (read_only) `update_room_document` (read_only) `update_room_participant` (read_only)</details> |
| `change_record_parts` | ? | ? | ? | ? | ? | ? |  |
| `change_relationships` | 3✓ 13? | 2✓ 1✗ 13? | 2✓ 1✗ 13? | 2✓ 1✗ 13? | 12✗ 4? | 2✓ 1✗ 13? | <details><summary>12</summary>`apply_employment_import` (read_only) `backfill_employment_import` (management, manager, ops, read_only, rep) `cancel_intro_request` (read_only) `check_vat` (read_only) `complete_intro_request` (read_only) `decide_intro_request` (read_only) `record_promise` (read_only) `request_intro` (read_only) `restore_profile_field` (read_only) `save_research` (read_only) `settle_promise` (read_only) `take_over_promise` (read_only)</details> |
| `change_reports` | ? | ? | ? | ✗ | ✗ | ? | <details><summary>1</summary>`draft` (ops, read_only)</details> |
| `change_settings` | 11✓ 21? | 1✓ 25✗ 6? | 1✓ 25✗ 6? | 3✓ 8✗ 21? | 1✓ 27✗ 4? | 1✓ 25✗ 6? | <details><summary>27</summary>`archive_legal_entity` (management, manager, ops, read_only, rep) `create_acquisition_source` (management, manager, read_only, rep) `create_disqualify_reason` (management, manager, read_only, rep) `create_lead_origin` (management, manager, read_only, rep) `create_lead_source` (management, manager, read_only, rep) `create_legal_entity` (management, manager, ops, read_only, rep) `create_loss_reason` (management, manager, read_only, rep) `decide_legacy_loss_reason` (management, manager, read_only, rep) `delete_disqualify_reason` (management, manager, read_only, rep) `delete_lead_origin` (management, manager, read_only, rep) `delete_lead_source` (management, manager, read_only, rep) `remove_legal_entity_logo` (management, manager, ops, read_only, rep) `remove_own_logo` (management, manager, ops, read_only, rep) `remove_own_logo_icon` (management, manager, ops, read_only, rep) `save_booking_alias` (read_only) `save_booking_page` (read_only) `update_acquisition_source` (management, manager, read_only, rep) `update_disqualify_reason` (management, manager, read_only, rep) `update_duplicate_settings` (management, manager, ops, read_only, rep) `update_follow_up_settings` (management, manager, read_only, rep) `update_lead_origin` (management, manager, read_only, rep) `update_lead_settings` (management, manager, read_only, rep) `update_lead_source` (management, manager, read_only, rep) `update_legal_entity` (management, manager, ops, read_only, rep) `update_legal_review_settings` (management, manager, ops, read_only, rep) `update_loss_reason` (management, manager, read_only, rep) `update_review_template` (management, manager, read_only, rep)</details> |
| `change_state` | 2✓ 19? | 2✓ 2✗ 17? | 10✗ 11? | 2✓ 2✗ 17? | 16✗ 5? | 10✗ 11? | <details><summary>16</summary>`accept_partner_lead` (manager, read_only, rep) `clear_lead_signal` (read_only) `decide_legal_review` (management, manager, ops, read_only, rep) `demote` (manager, read_only, rep) `disqualify` (manager, read_only, rep) `link_lead_company` (manager, read_only, rep) `merge_member_accounts` (management, manager, ops, read_only, rep) `move_project` (manager, read_only, rep) `promote` (manager, read_only, rep) `publish_contact` (read_only) `qualify` (manager, read_only, rep) `reopen_lead` (manager, read_only, rep) `request_legal_review` (read_only) `revert_stage` (read_only) `set_lead_signal` (read_only) `withdraw_legal_review` (read_only)</details> |
| `change_tags` | 1✓ 6? | 5✗ 2? | 5✗ 2? | 7? | 5✗ 2? | 5✗ 2? | <details><summary>5</summary>`archive` (management, manager, read_only, rep) `create` (management, manager, read_only, rep) `merge` (management, manager, read_only, rep) `restore` (management, manager, read_only, rep) `update` (management, manager, read_only, rep)</details> |
| `check_availability` | ? | ? | ? | ? | ? | ? |  |
| `commit_import` | ? | ✗ | ✗ | ? | ✗ | ✗ |  |
| `create_record` | ? | ? | ? | ? | ? | ? |  |
| `decide_approvals` | 2? | 2? | 2? | 2? | 2? | 2? |  |
| `draft_outbound` | 1✓ 1? | 1✓ 1? | 1✓ 1? | 1✓ 1? | 1✓ 1? | 1✓ 1? |  |
| `enrich` | ? | ? | ? | ? | ? | ? |  |
| `find_records` | 2✓ 3? | 2✓ 3? | 2✓ 3? | 2✓ 3? | 2✓ 3? | 2✓ 3? |  |
| `list_approvals` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |  |
| `list_documents` | 2✓ | 2✓ | 2✓ | 2✓ | 2✓ | 2✓ |  |
| `preview_import` | ? | ? | ? | ? | ? | ? |  |
| `read_analytics` | 2? | 2? | 2? | 2? | 2? | 2? |  |
| `read_approval` | ? | ? | ? | ? | ? | ? |  |
| `read_attention` | 3✓ | 3✓ | 3✓ | 3✓ | 3✓ | 3✓ |  |
| `read_context` | 3? | 1✓ 2? | 1✓ 2? | 1✓ 2? | 1✓ 2? | 1✓ 2? |  |
| `read_details` | 34✓ 36? | 33✓ 4✗ 33? | 33✓ 4✗ 33? | 34✓ 2✗ 34? | 32✓ 5✗ 33? | 28✓ 10✗ 32? | <details><summary>10</summary>`forecast_shares` (read_only, rep) `hidden_backlog` (rep) `hidden_backlog_rows` (rep) `import_problems` (management, manager, read_only, rep) `import_template` (management, manager, read_only, rep) `legal_review` (management, manager, ops, read_only, rep) `legal_reviews` (management, manager, ops, read_only, rep) `response_metrics` (rep) `team_board` (rep) `team_exceptions` (rep)</details> |
| `read_forecast` | 2✓ 3? | 2✓ 1✗ 2? | 2✓ 1✗ 2? | 2✓ 3? | 2✓ 1✗ 2? | 2✓ 1✗ 2? | <details><summary>1</summary>`input_sources` (management, manager, read_only, rep)</details> |
| `read_import` | 1✓ 1? | 2✗ | 2✗ | 1✓ 1? | 2✗ | 2✗ | <details><summary>2</summary>`format` (management, manager, read_only, rep) `report` (management, manager, read_only, rep)</details> |
| `read_lists` | ? | ? | ? | ? | ? | ? |  |
| `read_project` | 2? | 2? | 2? | 2? | 2? | 2? |  |
| `read_record` | ? | ? | ? | ? | ? | ? |  |
| `read_relationships` | 3✓ | 3✓ | 3✓ | 3✓ | 3✓ | 3✓ |  |
| `read_reports` | 11✓ 6? | 11✓ 6? | 11✓ 6? | 11✓ 6? | 11✓ 6? | 11✓ 6? |  |
| `read_tags` | 2✓ 1? | 2✓ 1? | 2✓ 1? | 2✓ 1? | 2✓ 1? | 2✓ 1? |  |
| `read_view` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |  |
| `read_workspace` | 55✓ 29? | 43✓ 16✗ 25? | 36✓ 23✗ 25? | 53✓ 5✗ 26? | 36✓ 23✗ 25? | 36✓ 23✗ 25? | <details><summary>23</summary>`ai_budget` (manager, read_only, rep) `ai_call_stats` (manager, read_only, rep) `ai_health` (manager, read_only, rep) `ai_profile` (management, manager, read_only, rep) `ai_provider_health` (manager, read_only, rep) `ai_routing_schema` (management, manager, read_only, rep) `ai_status` (manager, read_only, rep) `ai_task_overrides` (management, manager, read_only, rep) `ai_usage` (manager, read_only, rep) `ai_usage_summary` (manager, read_only, rep) `anchor_company` (management, manager, ops, read_only, rep) `assignable_roles` (management, manager, ops, read_only, rep) `available_models` (management, manager, read_only, rep) `extension_settings` (management, manager, read_only, rep) `extensions` (management, manager, read_only, rep) `import_runs` (management, manager, read_only, rep) `legacy_loss_reasons` (management, manager, read_only, rep) `member` (management, manager, ops, read_only, rep) `member_retirement` (management, manager, ops, read_only, rep) `provider_locations` (management, manager, read_only, rep) `retention_policies` (management, manager, read_only, rep) `roles` (management, manager, read_only, rep) `user_handover` (management, manager, ops, read_only, rep)</details> |
| `resolve_duplicates` | 2? | 2? | 2? | 2? | 2? | 2? |  |
| `run_research` | 8✓ | 8✓ | 8✓ | 8✓ | 8✓ | 8✓ |  |
| `schedule_meeting` | ? | ? | ? | ? | ? | ? |  |
| `send_outbound` | 2✓ 8? | 1✓ 1✗ 8? | 1✓ 1✗ 8? | 2✓ 8? | 1✓ 4✗ 5? | 1✓ 1✗ 8? | <details><summary>4</summary>`change_invitation` (read_only) `chat_reply` (read_only) `deal_room_resend` (read_only) `signature_template` (management, manager, read_only, rep)</details> |
| `update_record` | ? | ? | ? | ? | ? | ? |  |
| `whoami` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |  |
