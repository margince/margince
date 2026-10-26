# The served MCP surface

<!-- Generated together with mcp-info.json; do not edit by hand. -->

Generated from the served MCP surface by `go test ./internal/compose/ -run TestPublishedMCPSurface -update-mcp-info`; do not edit by hand. It shows every tool and resource across all scopes. tools/list and resources/list are filtered per caller, so a passport holding fewer scopes is served less. Extension tools are left out. The surface is captured as a client that supports MCP Apps sees it, so a tool bound to a view carries `_meta.ui.resourceUri`. A client that declines the UI extension is served no such member, and a client on the older handshake protocol, which cannot declare the extension, is served views. The `ui://` view descriptors are included; a deployment publishes each only after its boot has fetched and admitted that document.

`mcp-info.json` beside this page is the same surface byte for byte, as a client
receives it. This page is rendered from that file. Each tool shows its description, with its
input and output schema in folds below it. What a tool may do and whether a human must approve it is in
[agent-tools.md](agent-tools.md).

## Totals

| | |
|---|---:|
| Tools | 37 |
| Resources | 9 |
| Tool catalog | 126.7 KB |
| Resource catalog | 3.5 KB |
| Approx. wire tokens | 33320 |
| Largest tool | `preview_import` (8.5 KB) |
| Scopes rendered | `read`, `draft`, `write`, `send`, `enrich` |

Those are the bytes on the wire: they carry each tool's output schema and the
governance clause the transport appends. The listing a run re-sends every step is
smaller (name, description and input schema only) and is held against its own
budget in `agenttooldescriptions_test.go`. The Prompt tokens column below is what each
tool adds to that listing, and what it costs each scheduled agent, agent by agent, is
[agent-tool-budget.md](agent-tool-budget.md).

### What the tool catalog is made of

| Part | Bytes | Share | In a run's prompt? |
|---|---:|---:|---|
| Output schemas | 25.0 KB | 19% | **No**: a result's shape, never listed to a model |
| Descriptions (incl. governance clause) | 32.4 KB | 25% | Yes, every step |
| Input schemas | 60.5 KB | 47% | Yes, every step |
| _Names, annotations, punctuation_ | 8.7 KB | 6% | Partly |
| **Description + input schema** | **93.0 KB** | **73%** | **the recurring cost** |

Output schemas are the largest part of the total and are never sent to a model;
descriptions and input schemas are what each step pays for.

## Index

### Resources (9)

- [`margince://capabilities`](#capabilities): What this installation can do
- [`margince://schema/query`](#query_vocabulary): Workspace query vocabulary
- [`margince://schema/record-fields`](#record_fields): Record write vocabulary
- [`margince://schema/reports`](#report_vocabulary): Report plan vocabulary
- [`margince://schema/report-blocks`](#report_blocks): Report block grammar
- [`margince://schema/analytics`](#analytics-schema): Analytics query vocabulary
- [`ui://margince/create-followups.html`](#create_followups_view): Next steps for this record
- [`ui://margince/approval.html`](#approval_view): Waiting for a decision
- [`ui://margince/field-conflict.html`](#field_conflict_view): Edited by hand

### Tools (37)

| Tool | What it is for | Read-only | View | Prompt tokens |
|---|---|:-:|---|---:|
| [`annotate_brief`](#annotate_brief) | Write findings onto the morning brief |  |  | 417 |
| [`archive_record`](#archive_record) | Archive a record |  |  | 300 |
| [`bulk_update_records`](#bulk_update_records) | Change many records at once |  |  | 808 |
| [`change_activities`](#change_activities) | Log, plan and file activities |  |  | 1386 |
| [`change_lists`](#change_lists) | Make and change lists |  |  | 620 |
| [`change_record_parts`](#change_record_parts) | Change the parts of a record |  |  | 567 |
| [`change_state`](#change_state) | Move a lead, deal, project or partner |  |  | 1117 |
| [`change_tags`](#change_tags) | Change tags |  |  | 553 |
| [`check_availability`](#check_availability) | Check calendar availability | yes |  | 337 |
| [`commit_import`](#commit_import) | Commit an import |  |  | 279 |
| [`create_record`](#create_record) | Create a record |  | [`ui://margince/create-followups.html`](#create_followups_view) | 748 |
| [`decide_approvals`](#decide_approvals) | Approve or reject staged actions |  |  | 429 |
| [`draft_outbound`](#draft_outbound) | Draft an email or follow-ups |  |  | 517 |
| [`enrich`](#enrich) | Enrich a company from its website |  |  | 395 |
| [`find_records`](#find_records) | Find records | yes |  | 1738 |
| [`list_approvals`](#list_approvals) | List what is waiting for a decision | yes |  | 401 |
| [`list_documents`](#list_documents) | List a record's documents | yes |  | 216 |
| [`preview_import`](#preview_import) | Preview an import |  |  | 1001 |
| [`read_analytics`](#read_analytics) | Write a report or query data | yes |  | 1045 |
| [`read_approval`](#read_approval) | Read one staged action in full | yes | [`ui://margince/approval.html`](#approval_view) | 154 |
| [`read_attention`](#read_attention) | See what needs attention | yes |  | 326 |
| [`read_context`](#read_context) | Get the story so far | yes |  | 395 |
| [`read_forecast`](#read_forecast) | Read the forecast and how it moved | yes |  | 824 |
| [`read_import`](#read_import) | Read an import run or the import format | yes |  | 220 |
| [`read_lists`](#read_lists) | Find and read lists | yes |  | 773 |
| [`read_project`](#read_project) | Read a project or its handover | yes |  | 292 |
| [`read_record`](#read_record) | Read a record | yes |  | 307 |
| [`read_relationships`](#read_relationships) | Read who we know, and how well | yes |  | 359 |
| [`read_reports`](#read_reports) | Read sales reports | yes |  | 1891 |
| [`read_tags`](#read_tags) | Read tags | yes |  | 351 |
| [`read_view`](#read_view) | Read a view | yes |  | 993 |
| [`read_workspace`](#read_workspace) | Read how this workspace is set up | yes |  | 488 |
| [`resolve_duplicates`](#resolve_duplicates) | Merge two records or dismiss a flagged pair |  |  | 393 |
| [`schedule_meeting`](#schedule_meeting) | Book or invite to a meeting |  |  | 469 |
| [`send_outbound`](#send_outbound) | Send a message |  |  | 1014 |
| [`update_record`](#update_record) | Update a record |  | [`ui://margince/field-conflict.html`](#field_conflict_view) | 620 |
| [`whoami`](#whoami) | Who this passport acts for | yes |  | 129 |

## Resources

A resource takes no arguments and changes nothing, so it carries no autonomy
tier. It is scope-filtered as a tool is, so a passport holding
fewer scopes is served fewer documents.

### capabilities

`margince://capabilities` · application/json

**What this installation can do**

```text
The verbs this passport may call, which of them execute directly and which stage for a human
decision, and the scopes it holds. Names and governance only — the input schemas live in
tools/list.
```

### query_vocabulary

`margince://schema/query` · application/json

**Workspace query vocabulary**

```text
Everything a query plan may say, for you: the record types you can ask about, the fields you can
name on each, the operators each field admits, and the single relationship hop a plan may take. A
plan naming anything outside it is refused rather than approximated.
```

### record_fields

`margince://schema/record-fields` · application/json

**Record write vocabulary**

```text
The fields create_record and update_record accept for each record_type: which are required, what
shape each takes, and the values the closed ones admit. The two tools name this document instead of
carrying it.
```

### report_vocabulary

`margince://schema/reports` · application/json

**Report plan vocabulary**

```text
What each prebuilt report accepts in a report plan: the names its group_by, filters and aggregates
admit, what it answers with no plan at all, and what a filter means when its name alone does not
say. read_reports with kind=run names this document instead of carrying it.
```

### report_blocks

`margince://schema/report-blocks` · application/json

**Report block grammar**

```text
The blocks a report may carry: each kind, whether it renders figures, words or both, and the
severities a callout may state. read_analytics with step=compose names this document instead of
carrying it.
```

### analytics-schema

`margince://schema/analytics` · text/plain

**Analytics query vocabulary**

```text
The populations an analytics query plan may name, each with its group_by dimensions and its
measures, derived for this seat. read_analytics with step=query names this document instead of
carrying it.
```

### create_followups_view

`ui://margince/create-followups.html` · text/html;profile=mcp-app

**Next steps for this record**

```text
What a new record leaves to decide: a possible duplicate with the choice to merge it or keep it
apart, and a tag word offered for it.
```

<details><summary>Sandbox policy (<code>_meta.ui</code>)</summary>

```json
{
  "ui": {
    "csp": {
      "baseUriDomains": [],
      "connectDomains": [],
      "frameDomains": [],
      "resourceDomains": []
    },
    "prefersBorder": true
  }
}
```

</details>

### approval_view

`ui://margince/approval.html` · text/html;profile=mcp-app

**Waiting for a decision**

```text
A change an assistant proposed, what it would do, and the choice to approve or reject it.
```

<details><summary>Sandbox policy (<code>_meta.ui</code>)</summary>

```json
{
  "ui": {
    "csp": {
      "baseUriDomains": [],
      "connectDomains": [],
      "frameDomains": [],
      "resourceDomains": []
    },
    "prefersBorder": true
  }
}
```

</details>

### field_conflict_view

`ui://margince/field-conflict.html` · text/html;profile=mcp-app

**Edited by hand**

```text
Fields last edited by hand that an update would overwrite, with the value on the record beside the
one proposed and the choice to keep or replace it.
```

<details><summary>Sandbox policy (<code>_meta.ui</code>)</summary>

```json
{
  "ui": {
    "csp": {
      "baseUriDomains": [],
      "connectDomains": [],
      "frameDomains": [],
      "resourceDomains": []
    },
    "prefersBorder": true
  }
}
```

</details>

## Tools

### annotate_brief

**Write findings onto the morning brief**

```text
Write what you found onto the morning brief you just read: one sentence about the night as a whole,
and for each deal you looked at, why it is on the list, what changed, and the one next move you
would make. It writes onto that user's own brief for today and nothing else — it cannot be pointed
at another user, another day, or a deal that is not already in their queue, and it cannot change the
ranking. Every evidence id you cite must be one the brief already recorded for that item; citing
anything else refuses the whole write, so cite from what read_context with about=brief gave you
rather than from memory. Use change_activities with action=log to record something that happened on
a deal, which belongs on the record itself and outlives today's brief. Calling it again replaces
what you wrote before, so a second pass is a correction rather than an addition. (Runs immediately;
requires passport scope "write".)
```

<details><summary>Input schema</summary>

```json
{
  "additionalProperties": false,
  "properties": {
    "idempotency_key": {
      "description": "A retry without it may repeat the write. Same key, same result; a key reused with other arguments is refused.",
      "maxLength": 255,
      "type": "string"
    },
    "items": {
      "items": {
        "additionalProperties": false,
        "properties": {
          "cited_evidence": {
            "description": "Evidence ids this item already carries, at least one. A finding citing nothing is refused: the whole point is that the claim is grounded in a record you read.",
            "items": {
              "format": "uuid",
              "type": "string"
            },
            "minItems": 1,
            "type": "array"
          },
          "finding": {
            "description": "Why this is on the list, what changed, and the one next move.",
            "type": "string"
          },
          "item_id": {
            "description": "A brief item from the queue you just read.",
            "format": "uuid",
            "type": "string"
          }
        },
        "required": [
          "item_id",
          "finding",
          "cited_evidence"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "narrative": {
      "description": "One sentence about the night as a whole. Empty when there is nothing worth saying.",
      "type": "string"
    }
  },
  "type": "object"
}
```

</details>

<details><summary>Output schema</summary>

```json
{
  "properties": {
    "data": {
      "properties": {
        "items_annotated": {
          "type": "integer"
        },
        "narrative_written": {
          "type": "boolean"
        }
      },
      "required": [
        "items_annotated",
        "narrative_written"
      ],
      "type": "object"
    },
    "evidence": {
      "items": {
        "properties": {
          "captured_by": {
            "type": "string"
          },
          "record_id": {
            "format": "uuid",
            "type": "string"
          },
          "record_type": {
            "type": "string"
          },
          "source": {
            "type": "string"
          }
        },
        "required": [
          "record_id",
          "record_type"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "freshness": {
      "properties": {
        "authoritative": {
          "type": "boolean"
        },
        "last_synced_at": {
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "authoritative"
      ],
      "type": "object"
    },
    "schema_version": {
      "type": "string"
    },
    "trace_id": {
      "type": "string"
    },
    "trust": {
      "type": "string"
    },
    "warnings": {
      "items": {
        "properties": {
          "code": {
            "type": "string"
          },
          "message": {
            "type": "string"
          }
        },
        "required": [
          "code",
          "message"
        ],
        "type": "object"
      },
      "type": "array"
    }
  },
  "required": [
    "data",
    "evidence",
    "freshness",
    "schema_version",
    "trace_id",
    "trust",
    "warnings"
  ],
  "type": "object"
}
```

</details>

### archive_record

**Archive a record**

```text
Retire a record that should no longer be worked — a duplicate, a dead company, a project that
ended. Archiving hides the record from day-to-day work; it does not delete it and does not move
anything attached to it, so an archived duplicate still holds the activities and deals that were
logged against it. Use resolve_duplicates when a duplicate's history should end up on the record
that survives, and change_state with action=disqualify to close out a lead going nowhere — a
lead's own transition records the reason where archiving would not. By default the record is
archived when this call answers; where an installation has raised this verb to confirm first, the
answer is a staged approval and you must not report the record as archived until the retry that
carries their approval has answered. (Runs immediately; requires passport scope "write".)
```

<details><summary>Input schema</summary>

```json
{
  "additionalProperties": false,
  "properties": {
    "approval_id": {
      "description": "Set on approved retry",
      "format": "uuid",
      "type": "string"
    },
    "id": {
      "format": "uuid",
      "type": "string"
    },
    "idempotency_key": {
      "description": "A retry without it may repeat the write. Same key, same result; a key reused with other arguments is refused.",
      "maxLength": 255,
      "type": "string"
    },
    "record_type": {
      "enum": [
        "contact",
        "company",
        "deal",
        "project",
        "relationship",
        "activity",
        "product",
        "offer_template",
        "offer"
      ],
      "type": "string"
    }
  },
  "required": [
    "record_type",
    "id"
  ],
  "type": "object"
}
```

</details>

<details><summary>Output schema</summary>

```json
{
  "properties": {
    "data": {
      "properties": {
        "archived": {
          "type": "boolean"
        },
        "id": {
          "format": "uuid",
          "type": "string"
        },
        "record_type": {
          "type": "string"
        }
      },
      "required": [
        "archived",
        "id",
        "record_type"
      ],
      "type": "object"
    },
    "evidence": {
      "items": {
        "properties": {
          "captured_by": {
            "type": "string"
          },
          "record_id": {
            "format": "uuid",
            "type": "string"
          },
          "record_type": {
            "type": "string"
          },
          "source": {
            "type": "string"
          }
        },
        "required": [
          "record_id",
          "record_type"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "freshness": {
      "properties": {
        "authoritative": {
          "type": "boolean"
        },
        "last_synced_at": {
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "authoritative"
      ],
      "type": "object"
    },
    "schema_version": {
      "type": "string"
    },
    "trace_id": {
      "type": "string"
    },
    "trust": {
      "type": "string"
    },
    "warnings": {
      "items": {
        "properties": {
          "code": {
            "type": "string"
          },
          "message": {
            "type": "string"
          }
        },
        "required": [
          "code",
          "message"
        ],
        "type": "object"
      },
      "type": "array"
    }
  },
  "required": [
    "data",
    "evidence",
    "freshness",
    "schema_version",
    "trace_id",
    "trust",
    "warnings"
  ],
  "type": "object"
}
```

</details>

### bulk_update_records

**Change many records at once**

```text
Hand up to 500 contacts, companies, deals or leads to one owner, archive them, add them to or take
them off a Shortlist, put a tag on or take it off, or file one task under each, in one change; or
mark up to 500 of the user's Worklist tasks done (record_type worklist_item, verb complete) — or
undo such a change. A Worklist commitment is not yours to mark done: it is skipped as
commitment_needs_the_user. Call mode preview first and show the user what it says: how many records
change, which are left alone and why, and the sample rows. Execute only after they agree. Each
record is changed only if it still has the version you sent and you may change it. To undo, pass the
batch_id to undo_preview, show the user the answer, and call undo after they agree; a change is
undone once, and records changed since are left alone. update_record and archive_record change one
record. Above 10 records, execute and undo need the confirm_token their preview answered; it is good
once. Keep batch_id from the answer: undo names the change by it. (Runs immediately; requires
passport scope "write".)
```

<details><summary>Input schema</summary>

```json
{
  "additionalProperties": false,
  "else": {
    "required": [
      "batch_id"
    ]
  },
  "if": {
    "properties": {
      "mode": {
        "enum": [
          "preview",
          "execute"
        ]
      }
    }
  },
  "properties": {
    "batch_id": {
      "description": "For undo_preview and undo: the batch_id execute answered",
      "format": "uuid",
      "type": "string"
    },
    "confirm_token": {
      "description": "The token preview or undo_preview answered; needed above 10 records",
      "type": "string"
    },
    "idempotency_key": {
      "description": "A retry without it may repeat the write. Same key, same result; a key reused with other arguments is refused.",
      "maxLength": 255,
      "type": "string"
    },
    "items": {
      "items": {
        "additionalProperties": false,
        "properties": {
          "id": {
            "format": "uuid",
            "type": "string"
          },
          "version": {
            "type": "integer"
          }
        },
        "required": [
          "id",
          "version"
        ],
        "type": "object"
      },
      "maxItems": 500,
      "minItems": 1,
      "type": "array"
    },
    "list_id": {
      "description": "The Shortlist, for add_to_list and remove_from_list",
      "format": "uuid",
      "type": "string"
    },
    "mode": {
      "description": "preview says what would change; execute changes it; undo_preview and undo do the same for putting back the change batch_id names",
      "enum": [
        "preview",
        "execute",
        "undo_preview",
        "undo"
      ],
      "type": "string"
    },
    "note": {
      "description": "Why, for add_to_list and remove_from_list",
      "maxLength": 500,
      "type": "string"
    },
    "owner_id": {
      "description": "The new owner, for reassign_owner",
      "format": "uuid",
      "type": "string"
    },
    "record_type": {
      "description": "A lead takes every verb but archive and complete. A worklist_item is a Worklist task, and takes complete alone; a Worklist commitment is refused, because the user marks it done",
      "enum": [
        "contact",
        "company",
        "deal",
        "lead",
        "worklist_item"
      ],
      "type": "string"
    },
    "tag_id": {
      "description": "The tag, for add_tag and remove_tag",
      "format": "uuid",
      "type": "string"
    },
    "task": {
      "additionalProperties": false,
      "description": "The task create_task files under each record",
      "properties": {
        "assignee_id": {
          "description": "Who owes it; defaults to the user",
          "format": "uuid",
          "type": "string"
        },
        "due_at": {
          "description": "RFC 3339 WITH Z or the user's offset (…T14:35:00Z); never invent an hour or offset.",
          "format": "date-time",
          "type": "string"
        },
        "subject": {
          "maxLength": 500,
          "minLength": 1,
          "type": "string"
        }
      },
      "required": [
        "subject"
      ],
      "type": "object"
    },
    "verb": {
      "enum": [
        "reassign_owner",
        "archive",
        "add_to_list",
        "remove_from_list",
        "add_tag",
        "remove_tag",
        "create_task",
        "complete"
      ],
      "type": "string"
    }
  },
  "required": [
    "mode"
  ],
  "then": {
    "required": [
      "record_type",
      "verb",
      "items"
    ]
  },
  "type": "object"
}
```

</details>

<details><summary>Output schema</summary>

```json
{
  "properties": {
    "data": {
      "properties": {
        "affected": {
          "items": {
            "format": "uuid",
            "type": "string"
          },
          "type": "array"
        },
        "batch_id": {
          "format": "uuid",
          "type": "string"
        },
        "changed": {
          "type": "integer"
        },
        "confirm_token": {
          "type": "string"
        },
        "count": {
          "type": "integer"
        },
        "excluded": {
          "items": {
            "properties": {
              "id": {
                "format": "uuid",
                "type": "string"
              },
              "message": {
                "type": "string"
              },
              "reason": {
                "type": "string"
              }
            },
            "required": [
              "id",
              "reason"
            ],
            "type": "object"
          },
          "type": "array"
        },
        "expires_at": {
          "format": "date-time",
          "type": "string"
        },
        "left_behind": {
          "items": {
            "properties": {
              "id": {
                "format": "uuid",
                "type": "string"
              },
              "kind": {
                "type": "string"
              },
              "ref_id": {
                "format": "uuid",
                "type": "string"
              }
            },
            "required": [
              "id",
              "kind",
              "ref_id"
            ],
            "type": "object"
          },
          "type": "array"
        },
        "record_type": {
          "type": "string"
        },
        "requires_confirmation": {
          "type": "boolean"
        },
        "sample": {
          "items": {
            "properties": {
              "after": {
                "properties": {
                  "archived": {
                    "type": "boolean"
                  },
                  "done": {
                    "type": "boolean"
                  },
                  "listed": {
                    "type": "boolean"
                  },
                  "owner_id": {
                    "format": "uuid",
                    "type": "string"
                  },
                  "tagged": {
                    "type": "boolean"
                  },
                  "task_id": {
                    "format": "uuid",
                    "type": "string"
                  }
                },
                "required": [
                  "archived"
                ],
                "type": "object"
              },
              "before": {
                "properties": {
                  "archived": {
                    "type": "boolean"
                  },
                  "done": {
                    "type": "boolean"
                  },
                  "listed": {
                    "type": "boolean"
                  },
                  "owner_id": {
                    "format": "uuid",
                    "type": "string"
                  },
                  "tagged": {
                    "type": "boolean"
                  },
                  "task_id": {
                    "format": "uuid",
                    "type": "string"
                  }
                },
                "required": [
                  "archived"
                ],
                "type": "object"
              },
              "id": {
                "format": "uuid",
                "type": "string"
              },
              "label": {
                "type": "string"
              }
            },
            "required": [
              "after",
              "before",
              "id",
              "label"
            ],
            "type": "object"
          },
          "type": "array"
        },
        "skipped": {
          "items": {
            "properties": {
              "id": {
                "format": "uuid",
                "type": "string"
              },
              "message": {
                "type": "string"
              },
              "reason": {
                "type": "string"
              }
            },
            "required": [
              "id",
              "reason"
            ],
            "type": "object"
          },
          "type": "array"
        },
        "undo_of": {
          "format": "uuid",
          "type": "string"
        },
        "verb": {
          "type": "string"
        }
      },
      "type": "object"
    },
    "evidence": {
      "items": {
        "properties": {
          "captured_by": {
            "type": "string"
          },
          "record_id": {
            "format": "uuid",
            "type": "string"
          },
          "record_type": {
            "type": "string"
          },
          "source": {
            "type": "string"
          }
        },
        "required": [
          "record_id",
          "record_type"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "freshness": {
      "properties": {
        "authoritative": {
          "type": "boolean"
        },
        "last_synced_at": {
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "authoritative"
      ],
      "type": "object"
    },
    "schema_version": {
      "type": "string"
    },
    "trace_id": {
      "type": "string"
    },
    "trust": {
      "type": "string"
    },
    "warnings": {
      "items": {
        "properties": {
          "code": {
            "type": "string"
          },
          "message": {
            "type": "string"
          }
        },
        "required": [
          "code",
          "message"
        ],
        "type": "object"
      },
      "type": "array"
    }
  },
  "required": [
    "data",
    "evidence",
    "freshness",
    "schema_version",
    "trace_id",
    "trust",
    "warnings"
  ],
  "type": "object"
}
```

</details>

### change_activities

**Log, plan and file activities**

```text
Log, plan and file activities. log records an email, call, meeting, note or message that already
happened, on its records. task creates a task for a seat. attach attaches a file to a record. relink
moves one activity to another record; relink_many moves a named set. relink and relink_many wait for
a human when they file under a project, and run at once anywhere else. attach checks the record
first; read list_documents before attaching the same file again. Use schedule_meeting to book a
meeting still to come, and change_state with move_deal when the same event moves a deal, so move and
note are one change. Keep the activity id; a relink needs it. A staged answer's approval id is what
the retry carries. (Governance: runs immediately for attach, log, task; decided per call from its
arguments for relink, relink_many; requires passport scope "write".)
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "action": {
      "description": "log: record an email, call, meeting, note or message; task: create a task; attach: attach a file; relink: move one activity to another record; relink_many: move a named set",
      "enum": [
        "attach",
        "log",
        "relink",
        "relink_many",
        "task"
      ],
      "type": "string"
    },
    "activity_id": {
      "description": "relink only: The captured activity to re-associate",
      "format": "uuid",
      "type": "string"
    },
    "activity_ids": {
      "description": "relink_many only",
      "items": {
        "format": "uuid",
        "type": "string"
      },
      "maxItems": 500,
      "minItems": 1,
      "type": "array"
    },
    "approval_id": {
      "description": "relink, relink_many only: Set on approved retry",
      "format": "uuid",
      "type": "string"
    },
    "assignee_id": {
      "description": "task only: Defaults to the human you act for.",
      "format": "uuid",
      "type": "string"
    },
    "attendee_user_ids": {
      "description": "log only: Meeting or call only: the workspace members who attended besides you, ids from the colleague roster. The meeting then counts toward each one's relationship with every linked contact. An id that is not an active member is refused.",
      "items": {
        "format": "uuid",
        "type": "string"
      },
      "maxItems": 25,
      "type": "array"
    },
    "body": {
      "description": "log, task only: Prose a colleague reads. Same language rule as subject.",
      "type": "string"
    },
    "channel_provider": {
      "description": "log only: Required when kind is \"message\", else refused; a provider the workspace's channel provider list names.",
      "type": "string"
    },
    "content_base64": {
      "description": "attach only: The whole file, standard base64 with = padding",
      "type": "string"
    },
    "content_type": {
      "description": "attach only: The file's media type, e.g. application/pdf",
      "type": "string"
    },
    "contract_id": {
      "description": "attach only: The agreement this file is paper for, when it is one",
      "format": "uuid",
      "type": "string"
    },
    "direction": {
      "description": "log only",
      "enum": [
        "inbound",
        "outbound"
      ],
      "type": "string"
    },
    "due_at": {
      "description": "log, task only: RFC 3339 WITH Z or the user's offset (…T14:35:00Z); never invent an hour or offset.",
      "format": "date-time",
      "type": "string"
    },
    "entity_id": {
      "description": "attach, relink, relink_many only",
      "format": "uuid",
      "type": "string"
    },
    "entity_type": {
      "description": "attach, relink, relink_many only",
      "enum": [
        "activity",
        "company",
        "contact",
        "deal",
        "lead",
        "project"
      ],
      "type": "string"
    },
    "filename": {
      "description": "attach only: The file's name, with its extension",
      "type": "string"
    },
    "follow_up_task": {
      "description": "log only",
      "properties": {
        "assignee_id": {
          "format": "uuid",
          "type": "string"
        },
        "body": {
          "type": "string"
        },
        "due_at": {
          "description": "RFC 3339 WITH Z or the user's offset (…T14:35:00Z); never invent an hour or offset.",
          "format": "date-time",
          "type": "string"
        },
        "subject": {
          "type": "string"
        }
      },
      "required": [
        "subject"
      ],
      "type": "object"
    },
    "idempotency_key": {
      "description": "A retry without it may repeat the write. Same key, same result; a key reused with other arguments is refused.",
      "maxLength": 255,
      "type": "string"
    },
    "kind": {
      "description": "log only",
      "enum": [
        "email",
        "call",
        "meeting",
        "note",
        "task",
        "message"
      ],
      "type": "string"
    },
    "links": {
      "description": "log, task only: Every record this was about, ALL OF THEM in this call — EXCEPT a project, which this verb REFUSES: filing under a project marks the activity as commercial correspondence, so it is made by relinking the activity, which waits for the user's yes. A meeting or a call is with a CONTACT and reaches their company through them — linking one to a company is REFUSED, so name the contact who was there and the company follows from where they work. A meeting linked to the deal alone sits on no attendee's timeline and the company sees nothing. Adding a link AFTERWARDS is a second write — and a later link onto a project stages an approval that waits for the user's yes before it takes effect.",
      "items": {
        "additionalProperties": false,
        "properties": {
          "entity_id": {
            "format": "uuid",
            "type": "string"
          },
          "entity_type": {
            "enum": [
              "contact",
              "company",
              "deal",
              "lead",
              "project"
            ],
            "type": "string"
          }
        },
        "required": [
          "entity_type",
          "entity_id"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "occurred_at": {
      "description": "log only: RFC 3339 WITH Z or the user's offset (…T14:35:00Z); never invent an hour or offset.",
      "format": "date-time",
      "type": "string"
    },
    "replace_existing_of_type": {
      "default": false,
      "description": "relink, relink_many only: Replace the existing link of the same entity_type (move) rather than adding one (associate)",
      "type": "boolean"
    },
    "source_activity_id": {
      "description": "log, task only: A non-task source, including email or message. Caller and assignee need access to the source content. Empty links inherit its visible links; its audience is preserved. This does not accept a request.",
      "format": "uuid",
      "type": "string"
    },
    "source_id": {
      "description": "log only",
      "type": "string"
    },
    "source_system": {
      "description": "log only",
      "type": "string"
    },
    "subject": {
      "description": "log, task only: Prose a colleague reads. Write it in whoami's prose_language, whatever language this conversation is in; do not translate names or quoted text.",
      "type": "string"
    }
  },
  "required": [
    "action"
  ],
  "type": "object"
}
```

</details>

### change_lists

**Make and change lists**

```text
Make a Live List from a filter or a Shortlist of chosen records, change its name, purpose, filter,
sharing or steward, archive or restore it, and add or remove one Shortlist member with a note on
why. Only the steward of a list or a list admin may change it. A change must carry the version you
read; a list changed since is refused. An archived list is read-only. A Live List's members follow
its filter and cannot be added or removed by hand. Preview a filter with read_lists before saving
it. bulk_update_records adds or removes many records at once, with a confirmation. Keep the list id
and its new version from the answer. (Runs immediately; requires passport scope "write".)
```

<details><summary>Input schema</summary>

```json
{
  "additionalProperties": false,
  "properties": {
    "definition": {
      "description": "A Live List's filter tree",
      "properties": {
        "and": {
          "items": {
            "properties": {
              "and": {
                "type": "array"
              },
              "field": {
                "type": "string"
              },
              "op": {
                "type": "string"
              },
              "or": {
                "type": "array"
              },
              "value": {}
            },
            "type": "object"
          },
          "type": "array"
        },
        "field": {
          "type": "string"
        },
        "op": {
          "enum": [
            "eq",
            "neq",
            "gt",
            "gte",
            "lt",
            "lte",
            "in",
            "contains",
            "exists",
            "within_radius"
          ],
          "type": "string"
        },
        "or": {
          "items": {
            "properties": {
              "and": {
                "type": "array"
              },
              "field": {
                "type": "string"
              },
              "op": {
                "type": "string"
              },
              "or": {
                "type": "array"
              },
              "value": {}
            },
            "type": "object"
          },
          "type": "array"
        },
        "value": {
          "description": "A value, a list for in, true or false for exists, {\"days_ago\": N} for a date, or {\"lat\": N, \"lon\": N, \"radius_km\": N, \"label\": \"optional place name\"} for within_radius"
        }
      },
      "type": "object"
    },
    "entity_type": {
      "description": "For create, and the record type of record_id",
      "enum": [
        "contact",
        "company",
        "deal",
        "lead",
        "project"
      ],
      "type": "string"
    },
    "idempotency_key": {
      "description": "A retry without it may repeat the write. Same key, same result; a key reused with other arguments is refused.",
      "maxLength": 255,
      "type": "string"
    },
    "list_id": {
      "description": "The list, for every mode but create",
      "format": "uuid",
      "type": "string"
    },
    "list_type": {
      "description": "For create: static is a Shortlist, dynamic a Live List",
      "enum": [
        "static",
        "dynamic"
      ],
      "type": "string"
    },
    "mode": {
      "enum": [
        "create",
        "update",
        "archive",
        "restore",
        "add_member",
        "remove_member"
      ],
      "type": "string"
    },
    "name": {
      "type": "string"
    },
    "note": {
      "description": "For add_member and remove_member: why",
      "maxLength": 500,
      "type": "string"
    },
    "purpose": {
      "type": "string"
    },
    "record_id": {
      "description": "For add_member and remove_member",
      "format": "uuid",
      "type": "string"
    },
    "sharing": {
      "enum": [
        "private",
        "team",
        "workspace"
      ],
      "type": "string"
    },
    "steward_id": {
      "format": "uuid",
      "type": "string"
    },
    "team_id": {
      "format": "uuid",
      "type": "string"
    },
    "version": {
      "description": "For update: the version you read",
      "type": "integer"
    }
  },
  "required": [
    "mode"
  ],
  "type": "object"
}
```

</details>

<details><summary>Output schema</summary>

```json
{
  "properties": {
    "data": {
      "properties": {
        "mode": {
          "type": "string"
        },
        "result": {
          "type": "object"
        }
      },
      "required": [
        "mode",
        "result"
      ],
      "type": "object"
    },
    "evidence": {
      "items": {
        "properties": {
          "captured_by": {
            "type": "string"
          },
          "record_id": {
            "format": "uuid",
            "type": "string"
          },
          "record_type": {
            "type": "string"
          },
          "source": {
            "type": "string"
          }
        },
        "required": [
          "record_id",
          "record_type"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "freshness": {
      "properties": {
        "authoritative": {
          "type": "boolean"
        },
        "last_synced_at": {
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "authoritative"
      ],
      "type": "object"
    },
    "schema_version": {
      "type": "string"
    },
    "trace_id": {
      "type": "string"
    },
    "trust": {
      "type": "string"
    },
    "warnings": {
      "items": {
        "properties": {
          "code": {
            "type": "string"
          },
          "message": {
            "type": "string"
          }
        },
        "required": [
          "code",
          "message"
        ],
        "type": "object"
      },
      "type": "array"
    }
  },
  "required": [
    "data",
    "evidence",
    "freshness",
    "schema_version",
    "trace_id",
    "trust",
    "warnings"
  ],
  "type": "object"
}
```

</details>

### change_record_parts

**Change the parts of a record**

```text
Change what is kept under a record: an offer's line items, a company's facts and profile fields, a
project's companies and stakeholders. Prices are in minor units; read a product for its
unit_price_minor rather than inventing one. An offer's totals are computed from its lines and cannot
be supplied, and a sent or accepted offer keeps its lines. Pass if_version from your read so a
change made since is refused. Each action answers as its route does. Use update_record for a
record's own fields, and create_record with record_type offer to make an offer with its first lines.
(Runs immediately; requires passport scope "write".)
```

<details><summary>Input schema</summary>

```json
{
  "additionalProperties": false,
  "properties": {
    "action": {
      "enum": [
        "add_company_fact",
        "add_offer_line",
        "confirm_company_fact",
        "confirm_company_profile_field",
        "remove_company_fact",
        "remove_offer_line",
        "remove_project_company",
        "remove_project_stakeholder",
        "set_project_company",
        "set_project_stakeholder",
        "update_company_fact",
        "update_company_profile_field",
        "update_offer_line"
      ],
      "type": "string"
    },
    "category": {
      "type": "string"
    },
    "company_id": {
      "format": "uuid",
      "type": "string"
    },
    "contact_id": {
      "format": "uuid",
      "type": "string"
    },
    "fact_key": {
      "description": "One fact of the company, spelled \u003cfield\u003e:\u003cvalue_key\u003e from its read",
      "type": "string"
    },
    "field": {
      "type": "string"
    },
    "idempotency_key": {
      "description": "A retry without it may repeat the write. Same key, same result; a key reused with other arguments is refused.",
      "maxLength": 255,
      "type": "string"
    },
    "if_version": {
      "description": "The version you read, so a change made since is refused rather than overwritten",
      "type": "integer"
    },
    "line": {
      "additionalProperties": false,
      "description": "The line to add, or the fields of it to change. quantity is required to add one; product_id is taken only on add.",
      "properties": {
        "billing_interval_months": {
          "enum": [
            1,
            3,
            6,
            12
          ],
          "type": "integer"
        },
        "billing_model": {
          "enum": [
            "one_time",
            "recurring"
          ],
          "type": "string"
        },
        "description": {
          "type": "string"
        },
        "discount_pct": {
          "type": "number"
        },
        "interval_count": {
          "type": "integer"
        },
        "position": {
          "type": "integer"
        },
        "product_id": {
          "format": "uuid",
          "type": "string"
        },
        "quantity": {
          "type": "number"
        },
        "tax_rate": {
          "type": "number"
        },
        "unit": {
          "type": "string"
        },
        "unit_price_minor": {
          "type": "integer"
        }
      },
      "type": "object"
    },
    "line_item_id": {
      "format": "uuid",
      "type": "string"
    },
    "offer_id": {
      "format": "uuid",
      "type": "string"
    },
    "project_id": {
      "format": "uuid",
      "type": "string"
    },
    "role": {
      "type": "string"
    },
    "value": {
      "type": "string"
    }
  },
  "required": [
    "action"
  ],
  "type": "object"
}
```

</details>

<details><summary>Output schema</summary>

```json
{
  "properties": {
    "data": {
      "type": "object"
    },
    "evidence": {
      "items": {
        "properties": {
          "captured_by": {
            "type": "string"
          },
          "record_id": {
            "format": "uuid",
            "type": "string"
          },
          "record_type": {
            "type": "string"
          },
          "source": {
            "type": "string"
          }
        },
        "required": [
          "record_id",
          "record_type"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "freshness": {
      "properties": {
        "authoritative": {
          "type": "boolean"
        },
        "last_synced_at": {
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "authoritative"
      ],
      "type": "object"
    },
    "schema_version": {
      "type": "string"
    },
    "trace_id": {
      "type": "string"
    },
    "trust": {
      "type": "string"
    },
    "warnings": {
      "items": {
        "properties": {
          "code": {
            "type": "string"
          },
          "message": {
            "type": "string"
          }
        },
        "required": [
          "code",
          "message"
        ],
        "type": "object"
      },
      "type": "array"
    }
  },
  "required": [
    "data",
    "evidence",
    "freshness",
    "schema_version",
    "trace_id",
    "trust",
    "warnings"
  ],
  "type": "object"
}
```

</details>

### change_state

**Move a lead, deal, project or partner**

```text
Move a lead, deal or project on, or set up a company as a partner. qualify fills what a lead's own
data already implies, today the company name from its email domain, and names the fields still
empty. promote turns a lead who has genuinely engaged into a contact. disqualify closes out a lead
going nowhere. demote reverses a promotion. move_deal moves a deal to a stage, with a note.
move_project moves a project to a phase; closed closes it. set_up_partner starts a partner program
on a company, with company_id, partner_role and optionally relationship_stage, next_step and
next_step_due_at. promote needs a trigger naming the engagement; cold outreach nobody answered never
promotes. demote needs a reason and is refused once the contact owns a deal. set_up_partner always
waits for a human, takes no margin tier (a human sets that on the company's Partner tab) and is
refused for a company that already is a partner. move_deal to a won or lost stage waits for a human;
to an open stage it runs at once. move_project to closed needs a reason and leaves the project's
tasks open. When the user asked for the close, report what is still open and close: an open task is
news for them, not a reason to hold back. read_workspace topic pipelines gives the stage ids. Use
update_record for a record's other fields, and enrich to learn about a company from its website. A
promotion answers with the new contact's id. A staged answer's approval id is what the retry
carries; do not report the change until the retry has answered. (Governance: runs immediately for
demote, disqualify, move_project, promote, qualify; a human approves first for set_up_partner;
decided per call from its arguments for move_deal; requires passport scope "write".)
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "action": {
      "enum": [
        "demote",
        "disqualify",
        "move_deal",
        "move_project",
        "promote",
        "qualify",
        "set_up_partner"
      ],
      "type": "string"
    },
    "approval_id": {
      "description": "demote, disqualify, move_deal, move_project, promote, set_up_partner only: Set on approved retry",
      "format": "uuid",
      "type": "string"
    },
    "company_id": {
      "description": "set_up_partner: the company",
      "format": "uuid",
      "type": "string"
    },
    "deal_id": {
      "description": "move_deal only",
      "format": "uuid",
      "type": "string"
    },
    "evidence_note": {
      "description": "promote only",
      "type": "string"
    },
    "idempotency_key": {
      "description": "A retry without it may repeat the write. Same key, same result; a key reused with other arguments is refused.",
      "maxLength": 255,
      "type": "string"
    },
    "if_version": {
      "description": "move_deal, move_project only",
      "type": "integer"
    },
    "lead_id": {
      "description": "The lead",
      "format": "uuid",
      "type": "string"
    },
    "lost_reason": {
      "description": "move_deal only: Required when the target stage closes the deal as lost",
      "type": "string"
    },
    "next_step": {
      "description": "set_up_partner only",
      "type": "string"
    },
    "next_step_due_at": {
      "description": "set_up_partner only",
      "format": "date",
      "type": "string"
    },
    "note": {
      "description": "move_deal only: Logged as a note on the deal's timeline after the move",
      "type": "string"
    },
    "partner_role": {
      "description": "set_up_partner, required: hosting, consulting or strategic",
      "enum": [
        "hosting",
        "consulting",
        "strategic"
      ],
      "type": "string"
    },
    "project_id": {
      "description": "move_project only",
      "format": "uuid",
      "type": "string"
    },
    "reason": {
      "description": "demote, required: why the promotion is reversed. move_project: required when to_phase is closed. Recorded in the audit trail",
      "minLength": 1,
      "type": "string"
    },
    "relationship_stage": {
      "description": "set_up_partner only",
      "enum": [
        "research",
        "identified",
        "contacted",
        "in_conversation",
        "fit_confirmed",
        "agreement_pending",
        "active",
        "active_referring",
        "dormant",
        "no_fit"
      ],
      "type": "string"
    },
    "to_phase": {
      "description": "move_project only",
      "enum": [
        "initiative",
        "pursuing",
        "delivering",
        "closed"
      ],
      "type": "string"
    },
    "to_stage_id": {
      "description": "move_deal only: The target stage, by id — obtain it from the workspace's pipeline list, since a deal you have read carries only the stage it is already IN. That stage's semantic decides what happens next: open executes immediately, won or lost is staged for a human's approval.",
      "format": "uuid",
      "type": "string"
    },
    "trigger": {
      "description": "promote only, required: the genuine engagement justifying it; cold outreach with no reply never promotes",
      "enum": [
        "inbound_reply",
        "meeting_booked",
        "meeting_held",
        "human_qualify"
      ],
      "type": "string"
    },
    "won_without_contract_detail": {
      "description": "move_deal only: What the reason was, required when it is other",
      "maxLength": 500,
      "type": "string"
    },
    "won_without_contract_reason": {
      "description": "move_deal only: Why this win has no contract behind it. Omit when the deal has a signed contract with its paper attached; a win claiming neither is refused.",
      "enum": [
        "imported",
        "purchase_order",
        "verbal",
        "renewal_by_email",
        "other"
      ],
      "type": "string"
    }
  },
  "required": [
    "action"
  ],
  "type": "object"
}
```

</details>

### change_tags

**Change tags**

```text
Change the workspace's tags. apply and remove put one tag on, or take it off, one contact, company,
deal, lead or project, by tag_id or by the tag_name of a tag that already exists. create coins a new
word. update renames, recolours or describes one; fields left out are unchanged. merge folds a
duplicate word into the one kept, once a human approves. apply never creates a tag: an unknown name
is refused, a name matches case-insensitively, an archived word is refused as archived, and the same
tag twice is a conflict. Before create, read the list — a near-duplicate splits records that
belong together, and a taken name is a conflict, even a retired one, which only Settings restores.
create and update need tag grants an ordinary seat lacks. update sends no version, so read the tag
right before editing; it changes what a word is called, never what carries it. Removing a tag that
is not there succeeds. archive retires a word for every record; the records keep it. (Governance:
runs immediately for apply, archive, create, remove, update; a human approves first for merge;
requires passport scope "write".)
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "action": {
      "enum": [
        "apply",
        "archive",
        "create",
        "merge",
        "remove",
        "update"
      ],
      "type": "string"
    },
    "approval_id": {
      "description": "merge only: Set on approved retry",
      "format": "uuid",
      "type": "string"
    },
    "color": {
      "description": "create and update; none clears it (update only)",
      "enum": [
        "amber",
        "lime",
        "none",
        "orange",
        "rose",
        "sky",
        "slate",
        "teal",
        "violet"
      ],
      "type": "string"
    },
    "description": {
      "description": "create, update only",
      "type": "string"
    },
    "idempotency_key": {
      "description": "A retry without it may repeat the write. Same key, same result; a key reused with other arguments is refused.",
      "maxLength": 255,
      "type": "string"
    },
    "into_tag_id": {
      "description": "merge only: The word that survives",
      "format": "uuid",
      "type": "string"
    },
    "name": {
      "description": "create, update only",
      "maxLength": 64,
      "minLength": 1,
      "type": "string"
    },
    "record_id": {
      "description": "apply, remove only",
      "format": "uuid",
      "type": "string"
    },
    "record_type": {
      "description": "apply, remove only",
      "enum": [
        "company",
        "contact",
        "deal",
        "lead",
        "project"
      ],
      "type": "string"
    },
    "tag_id": {
      "description": "apply, archive, merge, remove, update only",
      "format": "uuid",
      "type": "string"
    },
    "tag_name": {
      "description": "apply, remove only: Instead of tag_id: the name of a tag the workspace ALREADY has. An unknown name is refused, never created",
      "maxLength": 64,
      "type": "string"
    }
  },
  "required": [
    "action"
  ],
  "type": "object"
}
```

</details>

### check_availability

**Check calendar availability**

```text
Find candidate times for a host without booking or sending anything. Set reliable=true to check the
acting host's selected Google or Microsoft calendars and pending reservations. Otherwise only
meetings recorded in this CRM are considered. calendar_backing=calendar is the evidence that live
occupancy was checked. This never checks invitees' availability. Use schedule_meeting with
action=invite to request an approved calendar invitation after a time is agreed. schedule_meeting
with action=book only records a meeting. read_context with about=meeting prepares for an existing
meeting. Keep the exact start and end, the calendar_backing value, and the truncated flag.
Availability is checked again when reserving the time.
```

<details><summary>Input schema</summary>

```json
{
  "additionalProperties": false,
  "properties": {
    "duration_minutes": {
      "maximum": 480,
      "minimum": 15,
      "type": "integer"
    },
    "from": {
      "description": "RFC 3339 WITH Z or the user's offset (…T14:35:00Z); never invent an offset.",
      "format": "date-time",
      "type": "string"
    },
    "host_user_id": {
      "description": "Defaults to the acting principal's user",
      "format": "uuid",
      "type": "string"
    },
    "reliable": {
      "default": false,
      "description": "Require live calendar occupancy and booking policy; fails if unavailable",
      "type": "boolean"
    },
    "to": {
      "description": "RFC 3339 WITH Z or the user's offset (…T14:35:00Z); never invent an offset.",
      "format": "date-time",
      "type": "string"
    }
  },
  "required": [
    "from",
    "to"
  ],
  "type": "object"
}
```

</details>

<details><summary>Output schema</summary>

```json
{
  "properties": {
    "data": {
      "properties": {
        "calendar_backing": {
          "type": "string"
        },
        "slots": {
          "items": {
            "properties": {
              "end": {
                "format": "date-time",
                "type": "string"
              },
              "start": {
                "format": "date-time",
                "type": "string"
              }
            },
            "required": [
              "end",
              "start"
            ],
            "type": "object"
          },
          "type": "array"
        },
        "truncated": {
          "type": "boolean"
        }
      },
      "required": [
        "calendar_backing",
        "slots",
        "truncated"
      ],
      "type": "object"
    },
    "evidence": {
      "items": {
        "properties": {
          "captured_by": {
            "type": "string"
          },
          "record_id": {
            "format": "uuid",
            "type": "string"
          },
          "record_type": {
            "type": "string"
          },
          "source": {
            "type": "string"
          }
        },
        "required": [
          "record_id",
          "record_type"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "freshness": {
      "properties": {
        "authoritative": {
          "type": "boolean"
        },
        "last_synced_at": {
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "authoritative"
      ],
      "type": "object"
    },
    "schema_version": {
      "type": "string"
    },
    "trace_id": {
      "type": "string"
    },
    "trust": {
      "type": "string"
    },
    "warnings": {
      "items": {
        "properties": {
          "code": {
            "type": "string"
          },
          "message": {
            "type": "string"
          }
        },
        "required": [
          "code",
          "message"
        ],
        "type": "object"
      },
      "type": "array"
    }
  },
  "required": [
    "data",
    "evidence",
    "freshness",
    "schema_version",
    "trace_id",
    "trust",
    "warnings"
  ],
  "type": "object"
}
```

</details>

### commit_import

**Commit an import**

```text
Write a checked import into the workspace. The dry run is the check; this starts the write and
answers at once with the run running. Only from awaiting_approval, the state a run reaches by
producing a dry-run report, so there is always a report first, or from a failed commit, which
resumes at the row it stopped at. This cannot be undone from here — undoing an import needs the
web app. A request to get a file in is not a review of it: unless the asker says they have already
been through these rows, show the counts and the mapping and wait for their go-ahead. Use
read_import for the report first: numbers nobody read are not a check. The write goes on after this
answers. Read read_import's report until its state is complete or failed, then report what landed:
the counts and the mapping the run used, each column by the field it went to. (Runs immediately;
requires passport scope "write".)
```

<details><summary>Input schema</summary>

```json
{
  "additionalProperties": false,
  "properties": {
    "approval_id": {
      "description": "Set on approved retry",
      "format": "uuid",
      "type": "string"
    },
    "idempotency_key": {
      "description": "A retry without it may repeat the write. Same key, same result; a key reused with other arguments is refused.",
      "maxLength": 255,
      "type": "string"
    },
    "run_id": {
      "format": "uuid",
      "type": "string"
    }
  },
  "required": [
    "run_id"
  ],
  "type": "object"
}
```

</details>

<details><summary>Output schema</summary>

```json
{
  "properties": {
    "data": {
      "properties": {
        "checkpoint": {
          "type": "integer"
        },
        "error": {
          "type": "string"
        },
        "object": {
          "type": "string"
        },
        "run_id": {
          "type": "string"
        },
        "state": {
          "type": "string"
        }
      },
      "required": [
        "checkpoint",
        "object",
        "run_id",
        "state"
      ],
      "type": "object"
    },
    "evidence": {
      "items": {
        "properties": {
          "captured_by": {
            "type": "string"
          },
          "record_id": {
            "format": "uuid",
            "type": "string"
          },
          "record_type": {
            "type": "string"
          },
          "source": {
            "type": "string"
          }
        },
        "required": [
          "record_id",
          "record_type"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "freshness": {
      "properties": {
        "authoritative": {
          "type": "boolean"
        },
        "last_synced_at": {
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "authoritative"
      ],
      "type": "object"
    },
    "schema_version": {
      "type": "string"
    },
    "trace_id": {
      "type": "string"
    },
    "trust": {
      "type": "string"
    },
    "warnings": {
      "items": {
        "properties": {
          "code": {
            "type": "string"
          },
          "message": {
            "type": "string"
          }
        },
        "required": [
          "code",
          "message"
        ],
        "type": "object"
      },
      "type": "array"
    }
  },
  "required": [
    "data",
    "evidence",
    "freshness",
    "schema_version",
    "trace_id",
    "trust",
    "warnings"
  ],
  "type": "object"
}
```

</details>

### create_record

**Create a record**

```text
Create a contact, company, deal, lead, project, activity or relationship that does not exist yet.
Creating a deal requires a pipeline_id and a stage_id, and the workspace's pipeline list is what
yields them for a deal that does not exist yet. Only the fields the chosen record_type actually
stores are accepted, and a field belonging to a neighbouring type is refused rather than dropped. A
CONTACT created here is visible to everyone in the workspace, the same as one a human types in. No
occurred_at means now; for a past day with no time, ask. Search first when the record might already
exist — a second copy of a contact or company is a problem that then needs resolve_duplicates to
undo. A record that shares only a name, with a different email, phone or employer, is not proof of a
duplicate. When the search finds a contact with the same name at the same employer, ask the human
whether it is the same contact before you link anything to it; two colleagues can share a name.
Giving a contact its employer by creating an employment relationship files any same-name contact at
that employer for review: the result carries it as duplicate_candidates, and you report the pair as
filed, unresolved until the reviewer decides. Several contacts, companies or leads at once — a
pasted list, a CSV — go through preview_import, which checks every row and writes nothing;
creating them one by one skips that check. The new record's id comes back in the result; keep it for
anything that links to it. When the user says where they met the contact, pass that word as
offer_tag: it only offers the tag, and change_tags still needs their yes. (Runs immediately;
requires passport scope "write".)
```

Renders its result in [`ui://margince/create-followups.html`](#create_followups_view), visible to `model`.

<details><summary>Input schema</summary>

```json
{
  "additionalProperties": false,
  "properties": {
    "approval_id": {
      "description": "Set on approved retry",
      "format": "uuid",
      "type": "string"
    },
    "fields": {
      "description": "The crm.yaml body for the record_type. The fields each record_type takes, which of them are REQUIRED, and their shapes are published at margince://schema/record-fields, and answered by read_workspace with topic=record_fields — that document, not this description, is what says what a write may name. An extra key must be cf_\u003cslug\u003e for a custom field; any other key is refused BY NAME and never dropped in silence, so a wrong guess is answered with the vocabulary rather than lost. An rfc3339 value carries Z or the offset the user gave. Any field holding a sentence — a description, a summary, a note — is written in whoami's prose_language, whatever language this conversation is in.",
      "type": "object"
    },
    "idempotency_key": {
      "description": "A retry without it may repeat the write. Same key, same result; a key reused with other arguments is refused.",
      "maxLength": 255,
      "type": "string"
    },
    "offer_tag": {
      "description": "A tag word the user said fits this record, such as the event they met at. It offers the tag to the user and applies nothing.",
      "maxLength": 64,
      "type": "string"
    },
    "record_type": {
      "enum": [
        "contact",
        "company",
        "deal",
        "lead",
        "activity",
        "project",
        "relationship",
        "product",
        "offer_template",
        "offer",
        "deal_room",
        "deal_room_thread",
        "deal_room_comment"
      ],
      "type": "string"
    }
  },
  "required": [
    "record_type",
    "fields"
  ],
  "type": "object"
}
```

</details>

<details><summary>Output schema</summary>

```json
{
  "properties": {
    "data": {
      "properties": {
        "duplicate_candidates": {
          "items": {
            "properties": {
              "candidate_id": {
                "type": "string"
              },
              "confidence": {
                "type": "number"
              },
              "evidence": {
                "items": {
                  "properties": {
                    "field": {
                      "type": "string"
                    },
                    "left_value": {
                      "type": "string"
                    },
                    "right_value": {
                      "type": "string"
                    },
                    "score": {
                      "type": "number"
                    },
                    "signal": {
                      "type": "string"
                    }
                  },
                  "required": [
                    "field"
                  ],
                  "type": "object"
                },
                "type": "array"
              },
              "other_record_id": {
                "type": "string"
              }
            },
            "required": [
              "candidate_id",
              "confidence",
              "evidence",
              "other_record_id"
            ],
            "type": "object"
          },
          "type": "array"
        },
        "fields": {
          "type": "object"
        },
        "id": {
          "format": "uuid",
          "type": "string"
        },
        "record_type": {
          "type": "string"
        },
        "tag_offer": {
          "properties": {
            "exists": {
              "type": "boolean"
            },
            "may_create": {
              "type": "boolean"
            },
            "name": {
              "type": "string"
            },
            "tag_id": {
              "format": "uuid",
              "type": "string"
            }
          },
          "required": [
            "exists",
            "may_create",
            "name"
          ],
          "type": "object"
        },
        "trust_tier": {
          "type": "string"
        },
        "version": {
          "type": "integer"
        }
      },
      "required": [
        "fields",
        "id",
        "record_type"
      ],
      "type": "object"
    },
    "evidence": {
      "items": {
        "properties": {
          "captured_by": {
            "type": "string"
          },
          "record_id": {
            "format": "uuid",
            "type": "string"
          },
          "record_type": {
            "type": "string"
          },
          "source": {
            "type": "string"
          }
        },
        "required": [
          "record_id",
          "record_type"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "freshness": {
      "properties": {
        "authoritative": {
          "type": "boolean"
        },
        "last_synced_at": {
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "authoritative"
      ],
      "type": "object"
    },
    "schema_version": {
      "type": "string"
    },
    "trace_id": {
      "type": "string"
    },
    "trust": {
      "type": "string"
    },
    "warnings": {
      "items": {
        "properties": {
          "code": {
            "type": "string"
          },
          "message": {
            "type": "string"
          }
        },
        "required": [
          "code",
          "message"
        ],
        "type": "object"
      },
      "type": "array"
    }
  },
  "required": [
    "data",
    "evidence",
    "freshness",
    "schema_version",
    "trace_id",
    "trust",
    "warnings"
  ],
  "type": "object"
}
```

</details>

### decide_approvals

**Approve or reject staged actions**

```text
Answer staged actions for the colleague asking you: approve, which lets the change happen, or
reject, which discards it. Name staged_action_id for one action, or bundle_id for every
still-waiting proposal one act staged together. The verdict is theirs — take an explicit approve
or reject rather than deciding what they would have wanted. Approving is what makes the change real,
including sending a message that was only drafted; a rejection cannot be taken back. You may approve
your own proposal only when it is a change that would have gone straight through but for a human's
earlier edit — no deal close, no relink, no tag merge, no schema change, no send; the rest is
theirs to release in the CRM. An item already answered, or lapsed, is reported as such and nothing
is written; each bundle member is answered and reported on its own, and a bundle holding none of
theirs reads as not found. read_approval when they have not seen what it holds; list_approvals is
where both ids come from. If the proposal is your OWN refused call, approving does not perform it
— re-issue that same call with approval_id set. (Runs immediately; requires passport scope
"write".)
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "bundle_id": {
      "description": "Every still-waiting proposal one act staged; leave out when naming staged_action_id",
      "format": "uuid",
      "type": "string"
    },
    "decision": {
      "enum": [
        "approve",
        "reject"
      ],
      "type": "string"
    },
    "idempotency_key": {
      "description": "A retry without it may repeat the write. Same key, same result; a key reused with other arguments is refused.",
      "maxLength": 255,
      "type": "string"
    },
    "reason": {
      "description": "Why, in the decider's own words; with bundle_id it is recorded against every member",
      "type": "string"
    },
    "staged_action_id": {
      "description": "One staged action; leave out when naming bundle_id",
      "format": "uuid",
      "type": "string"
    }
  },
  "required": [
    "decision"
  ],
  "type": "object"
}
```

</details>

### draft_outbound

**Draft an email or follow-ups**

```text
Write messages and send none. one_message: one email — a reply to a recorded thread (activity_id),
or a FIRST message to a contact or lead (links naming the recipient and, for a contact, the company,
deal or project it is about). slipping_deals: a follow-up for each deal in a segment at once —
today `slipping` — each left on its deal's timeline. A first email waits on the recipient's page,
marked as drafted by an agent, until the human you act for sends it there; a reply, or one not_saved
because they keep an unsent draft of their own, is not saved, so show it in full. slipping_deals
covers the deals read_attention with view=slipping reports, at most 25. send_outbound sends an email
once it is written. Keep the subject, body, to and the activity_id or links a message echoes; the
send takes them unchanged. Each follow-up comes back with its deal_id and draft_activity_id. (Runs
immediately; requires passport scope "draft".)
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "activity_id": {
      "description": "one_message only: The thread replied to; omit and give links for a first message",
      "format": "uuid",
      "type": "string"
    },
    "for": {
      "description": "one_message drafts one email; slipping_deals drafts a follow-up per slipping deal",
      "enum": [
        "one_message",
        "slipping_deals"
      ],
      "type": "string"
    },
    "idempotency_key": {
      "description": "A retry without it may repeat the write. Same key, same result; a key reused with other arguments is refused.",
      "maxLength": 255,
      "type": "string"
    },
    "intent": {
      "description": "one_message only",
      "type": "string"
    },
    "limit": {
      "description": "slipping_deals only: How many of the top-ranked deals to draft for; omit it for 25, the server-side ceiling on records one call may write",
      "maximum": 25,
      "minimum": 1,
      "type": "integer"
    },
    "links": {
      "description": "one_message only",
      "items": {
        "additionalProperties": false,
        "properties": {
          "entity_id": {
            "format": "uuid",
            "type": "string"
          },
          "entity_type": {
            "enum": [
              "contact",
              "company",
              "deal",
              "lead",
              "project"
            ],
            "type": "string"
          }
        },
        "required": [
          "entity_type",
          "entity_id"
        ],
        "type": "object"
      },
      "maxItems": 25,
      "minItems": 1,
      "type": "array"
    },
    "segment": {
      "description": "slipping_deals only: The deal set to draft follow-ups for; drafts land on each deal's timeline and are NEVER sent",
      "enum": [
        "slipping"
      ],
      "type": "string"
    }
  },
  "required": [
    "for"
  ],
  "type": "object"
}
```

</details>

### enrich

**Enrich a company from its website**

```text
Learn about a company by reading its public website, and propose what was found for a human to
accept onto the record. It reaches OUTSIDE the workspace, and what it returns is a PROPOSAL —
nothing lands on the record until someone accepts it, which is the review that guards this, not an
approval on the call. Reading one page answers immediately; reading a whole site is queued and
answers with a read id rather than the content. What it finds is captured text from a third party,
not a fact this workspace has verified. Use change_state with action=qualify to qualify a lead when
the missing values are already derivable from the record itself, which costs no external read and
needs no approval. Keep the company_id you enriched, and the read id when a whole-site read was
queued — the result is collected against it later. (Governance: a human approves every call before
it runs; requires passport scope "enrich".)
```

<details><summary>Input schema</summary>

```json
{
  "additionalProperties": false,
  "properties": {
    "approval_id": {
      "description": "Set on approved retry",
      "format": "uuid",
      "type": "string"
    },
    "company_id": {
      "description": "The company to enrich",
      "format": "uuid",
      "type": "string"
    },
    "depth": {
      "default": "page",
      "description": "page reads one page and returns a staged proposal; site queues a multi-page crawl and returns its read id; technical queues a lookup of what the company publicly runs (DNS, certificate logs, one homepage fingerprint) and returns its queue state",
      "enum": [
        "page",
        "site",
        "technical"
      ],
      "type": "string"
    },
    "idempotency_key": {
      "description": "A retry without it may repeat the write. Same key, same result; a key reused with other arguments is refused.",
      "maxLength": 255,
      "type": "string"
    },
    "url": {
      "description": "Absolute http(s) URL to read instead of the company's own domain",
      "format": "uri",
      "type": "string"
    }
  },
  "required": [
    "company_id"
  ],
  "type": "object"
}
```

</details>

<details><summary>Output schema</summary>

```json
{
  "properties": {
    "data": {
      "type": "object"
    },
    "evidence": {
      "items": {
        "properties": {
          "captured_by": {
            "type": "string"
          },
          "record_id": {
            "format": "uuid",
            "type": "string"
          },
          "record_type": {
            "type": "string"
          },
          "source": {
            "type": "string"
          }
        },
        "required": [
          "record_id",
          "record_type"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "freshness": {
      "properties": {
        "authoritative": {
          "type": "boolean"
        },
        "last_synced_at": {
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "authoritative"
      ],
      "type": "object"
    },
    "schema_version": {
      "type": "string"
    },
    "trace_id": {
      "type": "string"
    },
    "trust": {
      "type": "string"
    },
    "warnings": {
      "items": {
        "properties": {
          "code": {
            "type": "string"
          },
          "message": {
            "type": "string"
          }
        },
        "required": [
          "code",
          "message"
        ],
        "type": "object"
      },
      "type": "array"
    }
  },
  "required": [
    "data",
    "evidence",
    "freshness",
    "schema_version",
    "trace_id",
    "trust",
    "warnings"
  ],
  "type": "object"
}
```

</details>

### find_records

**Find records**

```text
Find records, choosing the engine by the question. search ranks contacts, companies, deals, leads
and projects by how well their text matches your words, and products, offer templates or activity
when named. list pages one record type by its filters and sort, newest first, and reaches the
timeline (activity), offers, products and deal rooms. query runs a typed plan of fields, operators
and one join, for counts, sums and the records near a place. meaning finds passages whose meaning
matches, in notes, emails and transcripts. identify says which existing contact or company a name,
email or domain refers to. search needs words and reads a timeline only when activity is named;
meaning reads it and answers passages, not records. list needs record_type, and read_workspace topic
list_filters names the filters each type takes. query names only fields read_workspace topic query
lists. identify answers unresolved for a record you may not see, as for one that does not exist. Use
read_record for one record you already have the id of, and read_context to learn what has been
happening on it. Keep each record's id and version, and next_cursor to read on.
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "candidates": {
      "description": "identify only",
      "items": {
        "additionalProperties": false,
        "properties": {
          "domains": {
            "description": "Company domains claimed by the payload. Read for a company only.",
            "items": {
              "type": "string"
            },
            "maxItems": 10,
            "type": "array"
          },
          "emails": {
            "description": "Every address on the payload, not just the primary one. For a company each address also contributes its domain, unless it is a consumer mail domain.",
            "items": {
              "type": "string"
            },
            "maxItems": 10,
            "type": "array"
          },
          "kind": {
            "description": "Which record type this payload is asking about. Leads are not resolved.",
            "enum": [
              "contact",
              "company"
            ],
            "type": "string"
          },
          "legal_name": {
            "description": "The registered company name, when it differs from the trading name. Read for a company only.",
            "type": "string"
          },
          "name": {
            "description": "Full name for a contact, trading name for a company.",
            "type": "string"
          },
          "phones": {
            "description": "Phone numbers in E.164 form; one that does not normalize is not a key and is ignored.",
            "items": {
              "type": "string"
            },
            "maxItems": 10,
            "type": "array"
          },
          "ref": {
            "description": "Your own label for this candidate, echoed back on its answer so a batch can be lined up. Any string; it is never stored.",
            "type": "string"
          }
        },
        "required": [
          "kind"
        ],
        "type": "object"
      },
      "maxItems": 20,
      "minItems": 1,
      "type": "array"
    },
    "cursor": {
      "description": "list, search only: Keyset cursor from a previous page's next_cursor",
      "type": "string"
    },
    "engine": {
      "description": "search: records ranked by how well their text matches your words; list: one record type by its filters, newest first; query: a typed plan of fields, operators and one join; meaning: passages whose meaning matches; identify: which existing contact or company a name, email or domain is",
      "enum": [
        "identify",
        "list",
        "meaning",
        "query",
        "search"
      ],
      "type": "string"
    },
    "filters": {
      "additionalProperties": false,
      "description": "list only: Narrow the list. Every operand is a string. Which filters each record_type takes, and the values each takes, is in read_workspace with topic=list_filters. A pipeline_id or stage_id comes from the workspace's pipeline list; nothing else on this surface yields one.",
      "properties": {
        "acquisition_source": {
          "type": "string"
        },
        "active": {
          "type": "string"
        },
        "active_only": {
          "type": "string"
        },
        "ai_written": {
          "type": "string"
        },
        "assignee_id": {
          "type": "string"
        },
        "captured_by_kind": {
          "type": "string"
        },
        "cert_status": {
          "type": "string"
        },
        "channel_provider": {
          "type": "string"
        },
        "commercial_motion": {
          "type": "string"
        },
        "company_id": {
          "type": "string"
        },
        "deal_id": {
          "type": "string"
        },
        "deal_room_id": {
          "type": "string"
        },
        "document_id": {
          "type": "string"
        },
        "domain": {
          "type": "string"
        },
        "entity_id": {
          "type": "string"
        },
        "entity_type": {
          "type": "string"
        },
        "forecast_category": {
          "type": "string"
        },
        "from_contact_id": {
          "type": "string"
        },
        "include_anchor": {
          "type": "string"
        },
        "include_archived": {
          "type": "string"
        },
        "industry": {
          "type": "string"
        },
        "key": {
          "type": "string"
        },
        "kind": {
          "type": "string"
        },
        "lifecycle": {
          "type": "string"
        },
        "list_id": {
          "type": "string"
        },
        "locale": {
          "type": "string"
        },
        "min_score": {
          "type": "string"
        },
        "occurred_after": {
          "type": "string"
        },
        "occurred_before": {
          "type": "string"
        },
        "origin": {
          "type": "string"
        },
        "owner_id": {
          "type": "string"
        },
        "owner_team_id": {
          "type": "string"
        },
        "participant_email": {
          "type": "string"
        },
        "partner_attribution": {
          "type": "string"
        },
        "partner_company_id": {
          "type": "string"
        },
        "partner_role": {
          "type": "string"
        },
        "partner_sourced": {
          "type": "string"
        },
        "phase": {
          "type": "string"
        },
        "pipeline_id": {
          "type": "string"
        },
        "priority": {
          "type": "string"
        },
        "project_id": {
          "type": "string"
        },
        "relationship_mark": {
          "type": "string"
        },
        "relationship_type": {
          "type": "string"
        },
        "reminder_due": {
          "type": "string"
        },
        "size_band": {
          "type": "string"
        },
        "sla_state": {
          "type": "string"
        },
        "sort": {
          "type": "string"
        },
        "source": {
          "type": "string"
        },
        "source_activity_id": {
          "type": "string"
        },
        "source_id": {
          "type": "string"
        },
        "source_system": {
          "type": "string"
        },
        "stage_id": {
          "type": "string"
        },
        "stalled": {
          "type": "string"
        },
        "state": {
          "type": "string"
        },
        "status": {
          "type": "string"
        },
        "tag_id": {
          "type": "string"
        },
        "tag_mode": {
          "type": "string"
        },
        "thread_key": {
          "type": "string"
        },
        "unassigned": {
          "type": "string"
        },
        "waiting_reply": {
          "type": "string"
        }
      },
      "type": "object"
    },
    "limit": {
      "description": "list, meaning, search only; at most 25 for meaning, search.",
      "maximum": 50,
      "minimum": 1,
      "type": "integer"
    },
    "per_type": {
      "description": "search only: Answer up to this many of each type instead of one ranked list, at most 25 records in all. Takes no cursor or limit; types_with_more names the types holding more.",
      "maximum": 20,
      "minimum": 1,
      "type": "integer"
    },
    "plan": {
      "description": "query only: A query plan, in the grammar published at margince://schema/query. That document, not this description, holds the record types, fields, operators and relationships this workspace admits: a name outside it is refused by name, never guessed at.",
      "type": "object"
    },
    "q": {
      "description": "list: text to match alongside any filters; search: the words to rank by, required",
      "type": "string"
    },
    "query": {
      "description": "meaning only: What to look for, in your own words. The wording is matched by meaning as well as by the words themselves, so a phrase that appears nowhere on a record can still rank it.",
      "maxLength": 1000,
      "type": "string"
    },
    "record_type": {
      "description": "list, search only",
      "enum": [
        "activity",
        "commission",
        "company",
        "contact",
        "deal",
        "deal_room",
        "deal_room_document",
        "deal_room_participant",
        "deal_room_thread",
        "lead",
        "offer",
        "offer_template",
        "partner",
        "product",
        "project"
      ],
      "type": "string"
    },
    "record_types": {
      "description": "search: these types in place of record_type, omitted for contact, company, deal, lead and project; meaning: these types, omitted for all five",
      "items": {
        "enum": [
          "activity",
          "company",
          "contact",
          "deal",
          "lead",
          "offer_template",
          "product",
          "project"
        ],
        "type": "string"
      },
      "type": "array"
    },
    "with_employees": {
      "description": "search only: Also find the contacts who work at a matching company; each carries match.works_at.",
      "type": "boolean"
    }
  },
  "required": [
    "engine"
  ],
  "type": "object"
}
```

</details>

### list_approvals

**List what is waiting for a decision**

```text
The staged actions waiting for a human's decision: what was proposed and what each would do. It is
where a proposal that is already waiting turns up — a message staged and unsent is not one that
needs writing again. It lists what the colleague you act for could decide themselves; anything else
is absent rather than refused. A proposal past its expiry reads as expired and can no longer be
answered. Personal mutes hide eligible pending rows, so a bundle may be partial. Use include_muted
for a complete authorized bundle review. Each item carries its one-line summary, not the change
itself. read_approval opens one and shows what it holds; decide_approvals answers it. Keep the
staged_action_id you mean to act on, the bundle_id when one act staged several, and next_cursor.
```

<details><summary>Input schema</summary>

```json
{
  "additionalProperties": false,
  "properties": {
    "bundle_id": {
      "description": "The approvals staged together as one bundle.",
      "format": "uuid",
      "type": "string"
    },
    "cursor": {
      "description": "next_cursor from a previous page.",
      "type": "string"
    },
    "include_muted": {
      "description": "Include your muted proposals for explicit review; visibility rules still apply.",
      "type": "boolean"
    },
    "kind": {
      "description": "One staged action's kind, e.g. advance_deal.",
      "type": "string"
    },
    "limit": {
      "maximum": 50,
      "minimum": 1,
      "type": "integer"
    },
    "status": {
      "description": "Defaults to pending — what is still waiting.",
      "enum": [
        "pending",
        "approved",
        "rejected"
      ],
      "type": "string"
    },
    "target_entity_id": {
      "format": "uuid",
      "type": "string"
    },
    "target_entity_type": {
      "description": "With target_entity_id, the approvals staged against one record.",
      "type": "string"
    }
  },
  "type": "object"
}
```

</details>

<details><summary>Output schema</summary>

```json
{
  "properties": {
    "data": {
      "properties": {
        "approvals": {
          "items": {
            "properties": {
              "bundle_id": {
                "format": "uuid",
                "type": "string"
              },
              "created_at": {
                "format": "date-time",
                "type": "string"
              },
              "decided_at": {
                "format": "date-time",
                "type": "string"
              },
              "decided_by": {
                "format": "uuid",
                "type": "string"
              },
              "diff_hash": {
                "type": "string"
              },
              "evidence": {
                "items": {
                  "properties": {
                    "evidence_snippet": {
                      "type": "string"
                    },
                    "source_id": {
                      "format": "uuid",
                      "type": "string"
                    },
                    "source_type": {
                      "type": "string"
                    }
                  },
                  "required": [
                    "evidence_snippet"
                  ],
                  "type": "object"
                },
                "type": "array"
              },
              "expires_at": {
                "format": "date-time",
                "type": "string"
              },
              "kind": {
                "type": "string"
              },
              "proposed_by": {
                "type": "string"
              },
              "proposed_change": {
                "type": "object"
              },
              "staged_action_id": {
                "format": "uuid",
                "type": "string"
              },
              "status": {
                "type": "string"
              },
              "summary": {
                "type": "string"
              },
              "target_id": {
                "format": "uuid",
                "type": "string"
              },
              "target_type": {
                "type": "string"
              }
            },
            "required": [
              "created_at",
              "kind",
              "proposed_by",
              "staged_action_id",
              "status"
            ],
            "type": "object"
          },
          "type": "array"
        },
        "next_cursor": {
          "type": "string"
        }
      },
      "required": [
        "approvals"
      ],
      "type": "object"
    },
    "evidence": {
      "items": {
        "properties": {
          "captured_by": {
            "type": "string"
          },
          "record_id": {
            "format": "uuid",
            "type": "string"
          },
          "record_type": {
            "type": "string"
          },
          "source": {
            "type": "string"
          }
        },
        "required": [
          "record_id",
          "record_type"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "freshness": {
      "properties": {
        "authoritative": {
          "type": "boolean"
        },
        "last_synced_at": {
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "authoritative"
      ],
      "type": "object"
    },
    "schema_version": {
      "type": "string"
    },
    "trace_id": {
      "type": "string"
    },
    "trust": {
      "type": "string"
    },
    "warnings": {
      "items": {
        "properties": {
          "code": {
            "type": "string"
          },
          "message": {
            "type": "string"
          }
        },
        "required": [
          "code",
          "message"
        ],
        "type": "object"
      },
      "type": "array"
    }
  },
  "required": [
    "data",
    "evidence",
    "freshness",
    "schema_version",
    "trace_id",
    "trust",
    "warnings"
  ],
  "type": "object"
}
```

</details>

### list_documents

**List a record's documents**

```text
List the files stored on a company, contact, deal, lead, project or activity, newest first: name,
type, size, who added them and the contract each is filed against. It lists and never returns a
file's contents. Check it before attaching a file again, so the record does not carry the same file
twice. Use read_view with view company_documents or deal_documents for a company's or deal's
document register, filtered by category, contract or state. Keep next_cursor to read the next page.
```

<details><summary>Input schema</summary>

```json
{
  "additionalProperties": false,
  "properties": {
    "cursor": {
      "description": "next_cursor from a previous page",
      "type": "string"
    },
    "entity_id": {
      "format": "uuid",
      "type": "string"
    },
    "entity_type": {
      "enum": [
        "company",
        "contact",
        "deal",
        "lead",
        "project",
        "activity"
      ],
      "type": "string"
    },
    "limit": {
      "maximum": 50,
      "minimum": 1,
      "type": "integer"
    }
  },
  "required": [
    "entity_type",
    "entity_id"
  ],
  "type": "object"
}
```

</details>

<details><summary>Output schema</summary>

```json
{
  "properties": {
    "data": {
      "properties": {
        "documents": {
          "items": {
            "properties": {
              "attachment_id": {
                "format": "uuid",
                "type": "string"
              },
              "byte_size": {
                "type": "integer"
              },
              "captured_by": {
                "type": "string"
              },
              "category": {
                "type": "string"
              },
              "checksum": {
                "type": "string"
              },
              "content_type": {
                "type": "string"
              },
              "contract_id": {
                "format": "uuid",
                "type": "string"
              },
              "created_at": {
                "format": "date-time",
                "type": "string"
              },
              "entity_id": {
                "format": "uuid",
                "type": "string"
              },
              "entity_type": {
                "type": "string"
              },
              "filename": {
                "type": "string"
              },
              "origin": {
                "properties": {
                  "activity_id": {
                    "format": "uuid",
                    "type": "string"
                  },
                  "kind": {
                    "type": "string"
                  },
                  "occurred_at": {
                    "format": "date-time",
                    "type": "string"
                  },
                  "sender_address": {
                    "type": "string"
                  },
                  "sender_name": {
                    "type": "string"
                  },
                  "subject": {
                    "type": "string"
                  }
                },
                "required": [
                  "activity_id",
                  "kind",
                  "occurred_at"
                ],
                "type": "object"
              },
              "url": {
                "type": "string"
              }
            },
            "required": [
              "attachment_id",
              "byte_size",
              "captured_by",
              "checksum",
              "content_type",
              "created_at",
              "entity_id",
              "entity_type",
              "filename"
            ],
            "type": "object"
          },
          "type": "array"
        },
        "next_cursor": {
          "type": "string"
        }
      },
      "required": [
        "documents"
      ],
      "type": "object"
    },
    "evidence": {
      "items": {
        "properties": {
          "captured_by": {
            "type": "string"
          },
          "record_id": {
            "format": "uuid",
            "type": "string"
          },
          "record_type": {
            "type": "string"
          },
          "source": {
            "type": "string"
          }
        },
        "required": [
          "record_id",
          "record_type"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "freshness": {
      "properties": {
        "authoritative": {
          "type": "boolean"
        },
        "last_synced_at": {
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "authoritative"
      ],
      "type": "object"
    },
    "schema_version": {
      "type": "string"
    },
    "trace_id": {
      "type": "string"
    },
    "trust": {
      "type": "string"
    },
    "warnings": {
      "items": {
        "properties": {
          "code": {
            "type": "string"
          },
          "message": {
            "type": "string"
          }
        },
        "required": [
          "code",
          "message"
        ],
        "type": "object"
      },
      "type": "array"
    }
  },
  "required": [
    "data",
    "evidence",
    "freshness",
    "schema_version",
    "trace_id",
    "trust",
    "warnings"
  ],
  "type": "object"
}
```

</details>

### preview_import

**Preview an import**

```text
Bring a spreadsheet in: send the CSV as text with a `mapping` saying what each column is, and this
checks every row against the workspace and reports what importing it would do. Writes nothing.
`object` is company, contact, lead, deal or activity. Use `contact` for a file the business already
knows — a migration off another CRM, a corrected export coming back. Use `lead` for a
machine-sourced list nobody has worked yet; those land unworked and a human promotes them. A row
naming a record already here is counted in `duplicates`, and created unless on_duplicate is skip —
except a contact whose email is already held, which is always refused, because an email is a real
key. A company's Website or Domain column maps to `domain`, which is what identifies a company —
import it and dedupe stops guessing from names. To link contacts to their employers, map the company
column to `company_name` — import the companies FIRST, because a name that matches nothing links
nothing and says so. To CORRECT companies rather than add them, map a column to `id`, then give a
row the id of the company it corrects — read them out first. A row whose `id` is EMPTY is a new
company, so one file may both correct and add. Files that link to each other go together in `files`,
at most one per type, instead of `object` and `csv`: a `ref` column is a row's own key, and a link
such as a contact's `company_ref` names a company's ref. read_import with view format lists each
file's columns. create_record for one record you already know. Keep the run_id. The counts its
report answers — created, duplicates, skipped — and the mapping it settled on are what the user
weighs, so report both: a column this placed by a name they did not write is a decision they did not
make. (Runs immediately; requires passport scope "write".)
```

<details><summary>Input schema</summary>

```json
{
  "additionalProperties": false,
  "properties": {
    "csv": {
      "description": "The file's contents, header row first.",
      "type": "string"
    },
    "files": {
      "description": "Several files as one run, in place of object, csv and mapping.",
      "items": {
        "additionalProperties": false,
        "properties": {
          "csv": {
            "description": "The file's contents, header row first.",
            "type": "string"
          },
          "mapping": {
            "additionalProperties": {
              "type": "string"
            },
            "description": "As mapping, for this file.",
            "type": "object"
          },
          "name": {
            "type": "string"
          },
          "object": {
            "enum": [
              "company",
              "lead",
              "contact",
              "deal",
              "activity"
            ],
            "type": "string"
          }
        },
        "required": [
          "name",
          "object",
          "csv"
        ],
        "type": "object"
      },
      "maxItems": 5,
      "minItems": 1,
      "type": "array"
    },
    "idempotency_key": {
      "description": "A retry without it may repeat the write. Same key, same result; a key reused with other arguments is refused.",
      "maxLength": 255,
      "type": "string"
    },
    "mapping": {
      "additionalProperties": {
        "type": "string"
      },
      "description": "Source column name → field name. Omit to accept the proposal this call would make, which it will only make if it can place EVERY column — a file whose headers are spelled the way a human would (\"Company\", \"City\") matches no field by name and is refused with the list, so send a mapping for those. Map a column to \"id\" to name the company a row corrects: that row updates it instead of creating one. A row whose \"id\" is empty is a new company, so one file may both correct and add. On a CONTACT run, map the company column to \"company_name\" to link each contact to their employer: the company must already be in the CRM, so import companies first, and a name matching none or matching two links nothing while the contact still lands.",
      "type": "object"
    },
    "object": {
      "enum": [
        "company",
        "lead",
        "contact",
        "deal",
        "activity"
      ],
      "type": "string"
    },
    "on_broken_link": {
      "description": "A link naming no record: skip_row (default) leaves the row out, drop_link lands it unlinked.",
      "enum": [
        "skip_row",
        "drop_link"
      ],
      "type": "string"
    },
    "on_duplicate": {
      "description": "A record already here: create (default) lands a second and files the pair for review; skip leaves the incumbent. For contacts an address already held is refused either way — an email is a real key, a company name is not.",
      "enum": [
        "create",
        "skip"
      ],
      "type": "string"
    },
    "source_name": {
      "description": "The system the files came from. A later import under the same name updates the rows it landed.",
      "type": "string"
    }
  },
  "type": "object"
}
```

</details>

<details><summary>Output schema</summary>

```json
{
  "properties": {
    "data": {
      "properties": {
        "columns": {
          "items": {
            "type": "string"
          },
          "type": "array"
        },
        "files": {
          "items": {
            "properties": {
              "columns": {
                "items": {
                  "type": "string"
                },
                "type": "array"
              },
              "mapping": {
                "additionalProperties": {
                  "type": "string"
                },
                "type": "object"
              },
              "name": {
                "type": "string"
              },
              "object": {
                "type": "string"
              },
              "part": {
                "type": "string"
              },
              "unmapped": {
                "items": {
                  "type": "string"
                },
                "type": "array"
              }
            },
            "required": [
              "columns",
              "mapping",
              "name",
              "object",
              "part"
            ],
            "type": "object"
          },
          "type": "array"
        },
        "mapping": {
          "additionalProperties": {
            "type": "string"
          },
          "type": "object"
        },
        "next": {
          "type": "string"
        },
        "report": {
          "properties": {
            "disposition": {
              "properties": {
                "created": {
                  "type": "integer"
                },
                "duplicates": {
                  "type": "integer"
                },
                "skipped": {
                  "type": "integer"
                },
                "unchanged": {
                  "type": "integer"
                },
                "updated": {
                  "type": "integer"
                }
              },
              "required": [
                "created",
                "skipped",
                "unchanged",
                "updated"
              ],
              "type": "object"
            },
            "estimated_duration_seconds": {
              "type": "integer"
            },
            "issues": {
              "items": {
                "properties": {
                  "column": {
                    "type": "string"
                  },
                  "file": {
                    "type": "string"
                  },
                  "kind": {
                    "type": "string"
                  },
                  "line": {
                    "type": "integer"
                  },
                  "params": {
                    "additionalProperties": {
                      "type": "string"
                    },
                    "type": "object"
                  },
                  "reason": {
                    "type": "string"
                  },
                  "reason_code": {
                    "type": "string"
                  },
                  "value": {
                    "type": "string"
                  }
                },
                "required": [
                  "reason"
                ],
                "type": "object"
              },
              "type": "array"
            },
            "links": {
              "properties": {
                "applied": {
                  "type": "integer"
                },
                "offered": {
                  "type": "integer"
                },
                "unresolved": {
                  "items": {
                    "properties": {
                      "from": {
                        "type": "string"
                      },
                      "params": {
                        "additionalProperties": {
                          "type": "string"
                        },
                        "type": "object"
                      },
                      "reason": {
                        "type": "string"
                      },
                      "reason_code": {
                        "type": "string"
                      },
                      "to": {
                        "type": "string"
                      }
                    },
                    "required": [
                      "from",
                      "reason",
                      "to"
                    ],
                    "type": "object"
                  },
                  "type": "array"
                }
              },
              "required": [
                "applied",
                "offered"
              ],
              "type": "object"
            },
            "mapping": {
              "additionalProperties": {
                "type": "string"
              },
              "type": "object"
            },
            "parts": {
              "items": {
                "properties": {
                  "closed_lost": {
                    "type": "integer"
                  },
                  "closed_won": {
                    "type": "integer"
                  },
                  "disposition": {
                    "properties": {
                      "created": {
                        "type": "integer"
                      },
                      "duplicates": {
                        "type": "integer"
                      },
                      "skipped": {
                        "type": "integer"
                      },
                      "unchanged": {
                        "type": "integer"
                      },
                      "updated": {
                        "type": "integer"
                      }
                    },
                    "required": [
                      "created",
                      "skipped",
                      "unchanged",
                      "updated"
                    ],
                    "type": "object"
                  },
                  "file": {
                    "type": "string"
                  },
                  "object": {
                    "type": "string"
                  },
                  "rows_read": {
                    "type": "integer"
                  }
                },
                "required": [
                  "disposition",
                  "file",
                  "object",
                  "rows_read"
                ],
                "type": "object"
              },
              "type": "array"
            },
            "rows_read": {
              "type": "integer"
            },
            "run_id": {
              "format": "uuid",
              "type": "string"
            },
            "source_key_used": {
              "type": "string"
            },
            "status": {
              "type": "string"
            },
            "undo": {
              "properties": {
                "errored": {
                  "items": {
                    "properties": {
                      "id": {
                        "format": "uuid",
                        "type": "string"
                      },
                      "object": {
                        "type": "string"
                      },
                      "reason": {
                        "type": "string"
                      },
                      "reason_code": {
                        "type": "string"
                      }
                    },
                    "required": [
                      "id",
                      "object",
                      "reason"
                    ],
                    "type": "object"
                  },
                  "type": "array"
                },
                "kept": {
                  "items": {
                    "properties": {
                      "id": {
                        "format": "uuid",
                        "type": "string"
                      },
                      "object": {
                        "type": "string"
                      },
                      "params": {
                        "additionalProperties": {
                          "type": "string"
                        },
                        "type": "object"
                      },
                      "reason": {
                        "type": "string"
                      },
                      "reason_code": {
                        "type": "string"
                      }
                    },
                    "required": [
                      "id",
                      "object"
                    ],
                    "type": "object"
                  },
                  "type": "array"
                },
                "reversed_count": {
                  "type": "integer"
                },
                "run_id": {
                  "format": "uuid",
                  "type": "string"
                },
                "status": {
                  "type": "string"
                },
                "updates_not_reversed": {
                  "type": "integer"
                }
              },
              "required": [
                "errored",
                "kept",
                "reversed_count",
                "run_id",
                "status"
              ],
              "type": "object"
            }
          },
          "required": [
            "disposition",
            "issues",
            "rows_read",
            "run_id",
            "source_key_used",
            "status"
          ],
          "type": "object"
        },
        "run": {
          "properties": {
            "checkpoint": {
              "type": "integer"
            },
            "error": {
              "type": "string"
            },
            "object": {
              "type": "string"
            },
            "run_id": {
              "type": "string"
            },
            "state": {
              "type": "string"
            }
          },
          "required": [
            "checkpoint",
            "object",
            "run_id",
            "state"
          ],
          "type": "object"
        },
        "unmapped": {
          "items": {
            "type": "string"
          },
          "type": "array"
        }
      },
      "required": [
        "run"
      ],
      "type": "object"
    },
    "evidence": {
      "items": {
        "properties": {
          "captured_by": {
            "type": "string"
          },
          "record_id": {
            "format": "uuid",
            "type": "string"
          },
          "record_type": {
            "type": "string"
          },
          "source": {
            "type": "string"
          }
        },
        "required": [
          "record_id",
          "record_type"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "freshness": {
      "properties": {
        "authoritative": {
          "type": "boolean"
        },
        "last_synced_at": {
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "authoritative"
      ],
      "type": "object"
    },
    "schema_version": {
      "type": "string"
    },
    "trace_id": {
      "type": "string"
    },
    "trust": {
      "type": "string"
    },
    "warnings": {
      "items": {
        "properties": {
          "code": {
            "type": "string"
          },
          "message": {
            "type": "string"
          }
        },
        "required": [
          "code",
          "message"
        ],
        "type": "object"
      },
      "type": "array"
    }
  },
  "required": [
    "data",
    "evidence",
    "freshness",
    "schema_version",
    "trace_id",
    "trust",
    "warnings"
  ],
  "type": "object"
}
```

</details>

### read_analytics

**Write a report or query data**

```text
Write a report document a reader keeps, such as a board-pack section, or ask the data a question.
For a document: query with save, then compose the saved results into it. query alone groups and
measures a population. read_workspace topic analytics names the populations, dimensions and measures
you may use. A figure in a composed document is cited from a saved query, never typed in. Use
read_reports for the standard metrics and named reports. Keep a saved query's id; compose cites it.
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "blocks": {
      "description": "compose only",
      "items": {
        "additionalProperties": false,
        "properties": {
          "cells": {
            "items": {
              "additionalProperties": false,
              "description": "Choose exactly one saved-run cell, metric_ref, or edition_ref.",
              "properties": {
                "column": {
                  "type": "string"
                },
                "edition_ref": {
                  "additionalProperties": false,
                  "properties": {
                    "edition_id": {
                      "format": "uuid",
                      "type": "string"
                    },
                    "metric": {
                      "type": "string"
                    }
                  },
                  "required": [
                    "edition_id",
                    "metric"
                  ],
                  "type": "object"
                },
                "group": {
                  "type": "array"
                },
                "metric_ref": {
                  "description": "A `pipeline_id` or `stage_id` used in a plan comes from the workspace's pipeline list.",
                  "properties": {
                    "metric": {
                      "type": "string"
                    },
                    "selection": {
                      "properties": {
                        "blocks": {
                          "items": {
                            "type": "string"
                          },
                          "type": "array"
                        },
                        "close_window": {
                          "type": "string"
                        },
                        "interval": {
                          "properties": {
                            "end_at": {
                              "description": "RFC 3339 WITH Z or the user's offset (…T14:35:00Z); never invent an offset.",
                              "format": "date-time",
                              "type": "string"
                            },
                            "start_at": {
                              "description": "RFC 3339 WITH Z or the user's offset (…T14:35:00Z); never invent an offset.",
                              "format": "date-time",
                              "type": "string"
                            }
                          },
                          "required": [
                            "end_at",
                            "start_at"
                          ],
                          "type": "object"
                        },
                        "layout": {
                          "items": {
                            "properties": {
                              "cards": {
                                "items": {
                                  "properties": {
                                    "id": {
                                      "type": "string"
                                    },
                                    "kind": {
                                      "type": "string"
                                    },
                                    "metric": {
                                      "type": "string"
                                    },
                                    "question": {
                                      "properties": {
                                        "entity": {
                                          "type": "string"
                                        },
                                        "group_by": {
                                          "items": {
                                            "type": "string"
                                          },
                                          "type": "array"
                                        },
                                        "limit": {
                                          "type": "integer"
                                        },
                                        "measures": {
                                          "items": {
                                            "properties": {
                                              "as": {
                                                "type": "string"
                                              },
                                              "field": {
                                                "type": "string"
                                              },
                                              "fn": {
                                                "type": "string"
                                              }
                                            },
                                            "required": [
                                              "fn"
                                            ],
                                            "type": "object"
                                          },
                                          "type": "array"
                                        }
                                      },
                                      "required": [
                                        "entity",
                                        "measures"
                                      ],
                                      "type": "object"
                                    },
                                    "width": {
                                      "type": "string"
                                    }
                                  },
                                  "required": [
                                    "id",
                                    "kind",
                                    "width"
                                  ],
                                  "type": "object"
                                },
                                "type": "array"
                              },
                              "title": {
                                "type": "string"
                              }
                            },
                            "required": [
                              "cards"
                            ],
                            "type": "object"
                          },
                          "type": "array"
                        },
                        "metrics": {
                          "items": {
                            "type": "string"
                          },
                          "type": "array"
                        },
                        "period": {
                          "type": "string"
                        },
                        "pipeline_id": {
                          "format": "uuid",
                          "type": "string"
                        },
                        "scope": {
                          "properties": {
                            "id": {
                              "format": "uuid",
                              "type": "string"
                            },
                            "kind": {
                              "type": "string"
                            },
                            "label": {
                              "type": "string"
                            }
                          },
                          "required": [
                            "kind"
                          ],
                          "type": "object"
                        },
                        "target_basis": {
                          "type": "string"
                        }
                      },
                      "required": [
                        "blocks",
                        "close_window",
                        "metrics",
                        "period",
                        "scope",
                        "target_basis"
                      ],
                      "type": "object"
                    }
                  },
                  "required": [
                    "metric",
                    "selection"
                  ],
                  "type": "object"
                },
                "run_id": {
                  "type": "string"
                }
              },
              "type": "object"
            },
            "type": "array"
          },
          "kind": {
            "type": "string"
          },
          "severity": {
            "type": "string"
          },
          "text": {
            "type": "string"
          }
        },
        "required": [
          "kind"
        ],
        "type": "object"
      },
      "minItems": 1,
      "type": "array"
    },
    "entity": {
      "description": "query only: A population from margince://schema/analytics. An unknown name is refused with the allowed set.",
      "type": "string"
    },
    "filters": {
      "description": "query only",
      "items": {
        "additionalProperties": false,
        "properties": {
          "field": {
            "type": "string"
          },
          "op": {
            "enum": [
              "eq",
              "ne",
              "lt",
              "lte",
              "gt",
              "gte",
              "is_null",
              "is_not_null"
            ]
          },
          "value": {}
        },
        "required": [
          "field",
          "op"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "group_by": {
      "description": "query only",
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "limit": {
      "description": "query only",
      "type": "integer"
    },
    "measures": {
      "description": "query only",
      "items": {
        "additionalProperties": false,
        "properties": {
          "as": {
            "type": "string"
          },
          "field": {
            "description": "A measure name. Omit only with fn=count.",
            "type": "string"
          },
          "fn": {
            "enum": [
              "count",
              "count_distinct",
              "sum",
              "avg",
              "min",
              "max",
              "median",
              "p75"
            ]
          }
        },
        "required": [
          "fn"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "save": {
      "description": "query only: Persist the run; the answer then carries a citable run_id.",
      "type": "boolean"
    },
    "scope_id": {
      "description": "query only",
      "type": "string"
    },
    "scope_kind": {
      "description": "query only: Omit for this seat's own default population.",
      "enum": [
        "workspace",
        "team",
        "owner"
      ]
    },
    "step": {
      "description": "query: ask a question of the data; compose: build a report document from saved queries",
      "enum": [
        "compose",
        "query"
      ],
      "type": "string"
    }
  },
  "required": [
    "step"
  ],
  "type": "object"
}
```

</details>

### read_approval

**Read one staged action in full**

```text
Read one staged action in full: the exact change proposed, the record it acts on, and the evidence
it was formed on — enough to answer it without opening the app. Reading performs nothing. An id
the colleague you act for could not decide answers as not found, exactly as an id naming nothing
does. list_approvals yields the id; decide_approvals answers it. Keep the staged_action_id, and the
bundle_id if the item names one.
```

Renders its result in [`ui://margince/approval.html`](#approval_view), visible to `model`.

<details><summary>Input schema</summary>

```json
{
  "additionalProperties": false,
  "properties": {
    "staged_action_id": {
      "description": "From list_approvals.",
      "format": "uuid",
      "type": "string"
    }
  },
  "required": [
    "staged_action_id"
  ],
  "type": "object"
}
```

</details>

<details><summary>Output schema</summary>

```json
{
  "properties": {
    "data": {
      "properties": {
        "bundle_id": {
          "format": "uuid",
          "type": "string"
        },
        "created_at": {
          "format": "date-time",
          "type": "string"
        },
        "decided_at": {
          "format": "date-time",
          "type": "string"
        },
        "decided_by": {
          "format": "uuid",
          "type": "string"
        },
        "diff_hash": {
          "type": "string"
        },
        "evidence": {
          "items": {
            "properties": {
              "evidence_snippet": {
                "type": "string"
              },
              "source_id": {
                "format": "uuid",
                "type": "string"
              },
              "source_type": {
                "type": "string"
              }
            },
            "required": [
              "evidence_snippet"
            ],
            "type": "object"
          },
          "type": "array"
        },
        "expires_at": {
          "format": "date-time",
          "type": "string"
        },
        "kind": {
          "type": "string"
        },
        "proposed_by": {
          "type": "string"
        },
        "proposed_change": {
          "type": "object"
        },
        "staged_action_id": {
          "format": "uuid",
          "type": "string"
        },
        "status": {
          "type": "string"
        },
        "summary": {
          "type": "string"
        },
        "target_id": {
          "format": "uuid",
          "type": "string"
        },
        "target_type": {
          "type": "string"
        }
      },
      "required": [
        "created_at",
        "kind",
        "proposed_by",
        "staged_action_id",
        "status"
      ],
      "type": "object"
    },
    "evidence": {
      "items": {
        "properties": {
          "captured_by": {
            "type": "string"
          },
          "record_id": {
            "format": "uuid",
            "type": "string"
          },
          "record_type": {
            "type": "string"
          },
          "source": {
            "type": "string"
          }
        },
        "required": [
          "record_id",
          "record_type"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "freshness": {
      "properties": {
        "authoritative": {
          "type": "boolean"
        },
        "last_synced_at": {
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "authoritative"
      ],
      "type": "object"
    },
    "schema_version": {
      "type": "string"
    },
    "trace_id": {
      "type": "string"
    },
    "trust": {
      "type": "string"
    },
    "warnings": {
      "items": {
        "properties": {
          "code": {
            "type": "string"
          },
          "message": {
            "type": "string"
          }
        },
        "required": [
          "code",
          "message"
        ],
        "type": "object"
      },
      "type": "array"
    }
  },
  "required": [
    "data",
    "evidence",
    "freshness",
    "schema_version",
    "trace_id",
    "trust",
    "warnings"
  ],
  "type": "object"
}
```

</details>

### read_attention

**See what needs attention**

```text
See what needs attention. slipping lists deals at risk this week and why. at_risk lists the open
deals that rest on a single contact or whose contacts have gone quiet. commitments lists promises
made to customers that are still open. Each view covers only what you may see, and says why each
item is on it. Use read_context to dig into one item. Keep each item's record id to act on it.
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "assignee_id": {
      "description": "commitments only: Narrow to one owner's promises; omit for everyone's",
      "format": "uuid",
      "type": "string"
    },
    "limit": {
      "description": "commitments, slipping only: Cap the set; omit for 50, the server-side ceiling",
      "maximum": 50,
      "minimum": 1,
      "type": "integer"
    },
    "project_id": {
      "description": "commitments only: Keep only promises filed under this project or under none",
      "format": "uuid",
      "type": "string"
    },
    "quiet_days": {
      "default": 60,
      "description": "slipping only: Days without recorded activity after which a deal counts as gone quiet; omit it for the product-wide stalled threshold",
      "maximum": 365,
      "minimum": 1,
      "type": "integer"
    },
    "view": {
      "description": "slipping: deals at risk this week; at_risk: relationships going cold; commitments: promises still open",
      "enum": [
        "at_risk",
        "commitments",
        "slipping"
      ],
      "type": "string"
    }
  },
  "required": [
    "view"
  ],
  "type": "object"
}
```

</details>

### read_context

**Get the story so far**

```text
Get the story so far. catch_up tells what has been happening on one contact, company, deal or
project. meeting prepares for a meeting: who is coming, the open threads and what was promised.
brief reads this morning's brief for the seat. It reads and writes nothing. catch_up summarises the
timeline; a field's current value is read_record's. brief answers the brief already made, never a
fresh one. Use read_attention for what is at risk across the whole pipeline. Keep the ids the answer
cites to read or change those records next.
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "about": {
      "description": "catch_up: what has been happening on one record; meeting: what to know before a meeting; brief: this morning's brief",
      "enum": [
        "brief",
        "catch_up",
        "meeting"
      ],
      "type": "string"
    },
    "max_items": {
      "description": "catch_up, meeting only",
      "maximum": 20,
      "minimum": 1,
      "type": "integer"
    },
    "project_id": {
      "description": "catch_up, meeting only: Keep only what is filed under this project or under none",
      "format": "uuid",
      "type": "string"
    },
    "record_id": {
      "description": "catch_up, meeting only: The record to build around. Give this or record_name, not both.",
      "format": "uuid",
      "type": "string"
    },
    "record_name": {
      "description": "catch_up, meeting only: The record named in words, found the way find_records with engine=search finds the same words. Refused with the candidate ids when the name matches more than one, rather than guessing.",
      "type": "string"
    },
    "record_type": {
      "description": "catch_up, meeting only",
      "enum": [
        "activity",
        "company",
        "contact",
        "deal",
        "lead",
        "project"
      ],
      "type": "string"
    }
  },
  "required": [
    "about"
  ],
  "type": "object"
}
```

</details>

### read_forecast

**Read the forecast and how it moved**

```text
Read the forecast under the installation's fiscal calendar and base currency. readings: what a
period is expected to close — `won`, `evidence`, `best_case`, `open` and `weighted` — and
`snapshots`, the period's frozen states, newest first. movement: why the forecast changed between
two snapshots, as named causes that sum to the whole difference. input_summary: whether the inputs
are sound enough to quote, and how much of the pipeline last night's check reached.
input_exceptions: the open input problems, most material first. input_sources: which connectors the
check could read, and how far back. `won` counts deals by the day they ACTUALLY closed. `evidence`
is committed pipeline whose close date somebody confirmed. `coverage_note` says what the totals
leave out; quote it with any total. movement needs `from` then `to`, both from ONE readings answer's
`snapshots`; none listed means nothing was frozen yet. Reading twice and subtracting is not the same
answer: it says nothing of where the difference went. In movement, `definition` means the snapshots
used different rules and `model` a re-scored probability: neither is sales movement.
`checks_incomplete` is not a worse `needs_review`: one says the pipeline has problems, the other
that nobody could look. An empty input_exceptions cannot be told from a clean pipeline;
input_sources tells them apart, and needs a coverage grant sellers lack. read_reports and
find_records with engine query also produce numbers, and neither is the forecast. For a board-pack
section, use read_analytics: query with save, then compose. Quote `as_of`, `timezone` and
`base_currency` with the number. Every `_minor` figure is in minor units of `base_currency`: 3100000
in EUR is EUR 31,000.
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "as_of": {
      "description": "readings only: Which period to read, by naming a day inside it. Omit for the current one.",
      "format": "date",
      "type": "string"
    },
    "from": {
      "description": "movement only: The opening snapshot.",
      "format": "uuid",
      "type": "string"
    },
    "period": {
      "description": "readings only: The window length. Quarters and months follow the installation's own financial year, which may not start in January; a week runs Monday to Sunday.",
      "enum": [
        "quarter",
        "month",
        "week"
      ],
      "type": "string"
    },
    "reading": {
      "description": "movement only: Which money answer this movement explains. A waterfall is drawn for ONE of them; mixing two adds figures that do not belong in one total.",
      "enum": [
        "open",
        "weighted",
        "evidence",
        "best_case"
      ],
      "type": "string"
    },
    "scope_id": {
      "description": "input_exceptions, readings only: The team or owner, for a team or owner scope_kind",
      "format": "uuid",
      "type": "string"
    },
    "scope_kind": {
      "description": "input_exceptions, readings only: Only the findings on the deals of this population. Omit for every finding you can see.",
      "enum": [
        "owner",
        "team",
        "workspace"
      ],
      "type": "string"
    },
    "to": {
      "description": "movement only: The closing snapshot.",
      "format": "uuid",
      "type": "string"
    },
    "view": {
      "description": "readings: what a period is expected to close; movement: why it changed between two snapshots; input_summary, input_exceptions and input_sources: whether its inputs can be trusted",
      "enum": [
        "input_exceptions",
        "input_sources",
        "input_summary",
        "movement",
        "readings"
      ],
      "type": "string"
    }
  },
  "required": [
    "view"
  ],
  "type": "object"
}
```

</details>

### read_import

**Read an import run or the import format**

```text
Read one import run, or the format an import takes. report, by run_id: the run's state and files,
and once validated what it will do or did, in rows created, updated, failed, unusable and
duplicates. format: each file's columns, their order and the key rules. The report's counts are what
the user weighs before committing. After a commit, read report again until the state is complete or
failed.
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "lang": {
      "description": "format only: The language of the guide text; the installation's when absent",
      "enum": [
        "en",
        "de",
        "vi"
      ],
      "type": "string"
    },
    "run_id": {
      "description": "report only",
      "format": "uuid",
      "type": "string"
    },
    "view": {
      "description": "report is one run's state and, once validated, what it will do or did; format is the files and columns an import takes",
      "enum": [
        "format",
        "report"
      ],
      "type": "string"
    }
  },
  "required": [
    "view"
  ],
  "type": "object"
}
```

</details>

### read_lists

**Find and read lists**

```text
Find the team's Live Lists (saved filters whose members join and leave on their own) and Shortlists
(records chosen by hand), read one, page through its members, say why a record is or is not on it,
read what changed on it — including which records a Live List was seen to gain and lose, and how
many since the user last opened it — or preview what a filter would select before a Live List is
saved. Every count, member and reason is what the user you act for may see: a list shared with them
never shows a member record they cannot read, so two users may see different counts for one list.
Live Lists are checked every 15 minutes, longest-unchecked first, so with very many lists one can
wait longer (last_check says when); a record that joined and left between two checks is not
recorded. A preview is logged as a read of those records. A list with health retired_field still
works but filters on a retired custom field, named in retired_fields; its steward should replace
that clause. A list with health retired_tag filters on an archived or merged-away tag, named in
retired_tags, and that tag no longer matches any record, so its steward should name the live tag.
find_records with engine=search finds records by name; tags are applied with change_tags, not lists.
Keep list_id, the version for a later change, and next_cursor to read the next page.
```

<details><summary>Input schema</summary>

```json
{
  "additionalProperties": false,
  "properties": {
    "cursor": {
      "description": "For members and history: the next_cursor a page answered",
      "type": "string"
    },
    "definition": {
      "description": "For preview: a filter tree, as a Live List stores it",
      "properties": {
        "and": {
          "items": {
            "properties": {
              "and": {
                "type": "array"
              },
              "field": {
                "type": "string"
              },
              "op": {
                "type": "string"
              },
              "or": {
                "type": "array"
              },
              "value": {}
            },
            "type": "object"
          },
          "type": "array"
        },
        "field": {
          "type": "string"
        },
        "op": {
          "enum": [
            "eq",
            "neq",
            "gt",
            "gte",
            "lt",
            "lte",
            "in",
            "contains",
            "exists",
            "within_radius"
          ],
          "type": "string"
        },
        "or": {
          "items": {
            "properties": {
              "and": {
                "type": "array"
              },
              "field": {
                "type": "string"
              },
              "op": {
                "type": "string"
              },
              "or": {
                "type": "array"
              },
              "value": {}
            },
            "type": "object"
          },
          "type": "array"
        },
        "value": {
          "description": "A value, a list for in, true or false for exists, {\"days_ago\": N} for a date, or {\"lat\": N, \"lon\": N, \"radius_km\": N, \"label\": \"optional place name\"} for within_radius"
        }
      },
      "type": "object"
    },
    "entity_type": {
      "description": "For find (optional) and preview (required)",
      "enum": [
        "contact",
        "company",
        "deal",
        "lead",
        "project"
      ],
      "type": "string"
    },
    "limit": {
      "maximum": 100,
      "minimum": 1,
      "type": "integer"
    },
    "list_id": {
      "description": "The list, for get, members, why and history",
      "format": "uuid",
      "type": "string"
    },
    "mode": {
      "enum": [
        "find",
        "get",
        "members",
        "why",
        "history",
        "preview"
      ],
      "type": "string"
    },
    "query": {
      "description": "For find: matches name or purpose",
      "type": "string"
    },
    "record_id": {
      "description": "For why: the record to explain",
      "format": "uuid",
      "type": "string"
    },
    "sharing": {
      "description": "For find (optional): only lists with one of these sharing settings; private alone is the caller's own private lists",
      "items": {
        "enum": [
          "private",
          "team",
          "workspace"
        ],
        "type": "string"
      },
      "type": "array"
    }
  },
  "required": [
    "mode"
  ],
  "type": "object"
}
```

</details>

<details><summary>Output schema</summary>

```json
{
  "properties": {
    "data": {
      "properties": {
        "mode": {
          "type": "string"
        },
        "result": {
          "type": "object"
        }
      },
      "required": [
        "mode",
        "result"
      ],
      "type": "object"
    },
    "evidence": {
      "items": {
        "properties": {
          "captured_by": {
            "type": "string"
          },
          "record_id": {
            "format": "uuid",
            "type": "string"
          },
          "record_type": {
            "type": "string"
          },
          "source": {
            "type": "string"
          }
        },
        "required": [
          "record_id",
          "record_type"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "freshness": {
      "properties": {
        "authoritative": {
          "type": "boolean"
        },
        "last_synced_at": {
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "authoritative"
      ],
      "type": "object"
    },
    "schema_version": {
      "type": "string"
    },
    "trace_id": {
      "type": "string"
    },
    "trust": {
      "type": "string"
    },
    "warnings": {
      "items": {
        "properties": {
          "code": {
            "type": "string"
          },
          "message": {
            "type": "string"
          }
        },
        "required": [
          "code",
          "message"
        ],
        "type": "object"
      },
      "type": "array"
    }
  },
  "required": [
    "data",
    "evidence",
    "freshness",
    "schema_version",
    "trace_id",
    "trust",
    "warnings"
  ],
  "type": "object"
}
```

</details>

### read_project

**Read a project or its handover**

```text
Read one project. overview: its whole page — company, phase history with time per phase, deals,
stakeholders, contracts, documents, open commitments, timeline, filing coverage, totals. handoff:
what the delivery side needs from sales — who owns it, who to call at the client, what was sold,
by when, what is already promised — with a named gap for each the records do not answer. overview
cuts each section at 25 rows and flags it truncated; sections_omitted names what your grants
withhold. A handoff gap means the field is empty as far as the caller can see. Preparing a handover
is not performing one. read_record for the project's stored fields alone, read_context with
about=catch_up for what has been happening. The project_id, each gap's source field, and the deal,
contact and task ids a follow-up acts on.
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "project_id": {
      "description": "The project being handed to delivery",
      "format": "uuid",
      "type": "string"
    },
    "view": {
      "description": "overview is the project's whole page; handoff is what delivery needs from sales, with its gaps",
      "enum": [
        "handoff",
        "overview"
      ],
      "type": "string"
    }
  },
  "required": [
    "view",
    "project_id"
  ],
  "type": "object"
}
```

</details>

### read_record

**Read a record**

```text
Read one record's own stored fields — the values a reader would see on its detail page — when
you already know which record you mean. It returns that record and nothing around it: no timeline,
no related contacts, no deals on the company. Use read_context with about=catch_up when the goal is
what has been happening on the record rather than what it currently says. Keep the version from the
result and pass it back as if_version on a later update, so a write is refused rather than silently
overwriting a change made in between. Its `owner` says who holds it. When `owner.is_you` is false,
say whose it is when you report the record (or that its owner could not be named, when `owner.name`
is absent), and treat contacting it as theirs to decide rather than advising an approach as though
the record were unowned.
```

<details><summary>Input schema</summary>

```json
{
  "additionalProperties": false,
  "properties": {
    "id": {
      "format": "uuid",
      "type": "string"
    },
    "record_type": {
      "description": "partner is addressed by its COMPANY's id: the row is that company's partner terms, not a separate record.",
      "enum": [
        "contact",
        "company",
        "deal",
        "lead",
        "activity",
        "project",
        "partner",
        "product",
        "offer_template",
        "offer",
        "commission",
        "deal_room"
      ],
      "type": "string"
    }
  },
  "required": [
    "record_type",
    "id"
  ],
  "type": "object"
}
```

</details>

<details><summary>Output schema</summary>

```json
{
  "properties": {
    "data": {
      "properties": {
        "fields": {
          "type": "object"
        },
        "id": {
          "format": "uuid",
          "type": "string"
        },
        "owner": {
          "properties": {
            "id": {
              "format": "uuid",
              "type": "string"
            },
            "is_you": {
              "type": "boolean"
            },
            "name": {
              "type": "string"
            }
          },
          "required": [
            "id",
            "is_you"
          ],
          "type": "object"
        },
        "record_type": {
          "type": "string"
        },
        "trust_tier": {
          "type": "string"
        },
        "version": {
          "type": "integer"
        }
      },
      "required": [
        "fields",
        "id",
        "record_type"
      ],
      "type": "object"
    },
    "evidence": {
      "items": {
        "properties": {
          "captured_by": {
            "type": "string"
          },
          "record_id": {
            "format": "uuid",
            "type": "string"
          },
          "record_type": {
            "type": "string"
          },
          "source": {
            "type": "string"
          }
        },
        "required": [
          "record_id",
          "record_type"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "freshness": {
      "properties": {
        "authoritative": {
          "type": "boolean"
        },
        "last_synced_at": {
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "authoritative"
      ],
      "type": "object"
    },
    "schema_version": {
      "type": "string"
    },
    "trace_id": {
      "type": "string"
    },
    "trust": {
      "type": "string"
    },
    "warnings": {
      "items": {
        "properties": {
          "code": {
            "type": "string"
          },
          "message": {
            "type": "string"
          }
        },
        "required": [
          "code",
          "message"
        ],
        "type": "object"
      },
      "type": "array"
    }
  },
  "required": [
    "data",
    "evidence",
    "freshness",
    "schema_version",
    "trace_id",
    "trust",
    "warnings"
  ],
  "type": "object"
}
```

</details>

### read_relationships

**Read who we know, and how well**

```text
Read the relationships this workspace has recorded, from the one id you name. contact_id: the
colleagues who know that contact, warmest first. company_id: a warm route into the company — who
we know there and which colleague could introduce us. deal_id: which roles at the deal's company we
have a relationship with, and where the deal rests on a single contact. Only interactions somebody
recorded count, so an unlogged relationship is absent, and never spoken is no relationship rather
than a score of zero. A company nobody here has spoken to has no warm path, and saying so is the
answer. Deal coverage says nothing about whether the deal will close. read_attention with
view=at_risk finds the thin relationships across every open deal at once. Keep the user_id of each
colleague and the contact ids a path names. For a deal, say WHO fills an uncovered seat by
`contact_name`; a seat with no name is one this caller may not read, so report the gap and do not
guess.
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "company_id": {
      "description": "A warm route into this company; name exactly one of the three ids",
      "format": "uuid",
      "type": "string"
    },
    "contact_id": {
      "description": "The colleagues who know this contact; name exactly one of the three ids",
      "format": "uuid",
      "type": "string"
    },
    "deal_id": {
      "description": "Which roles at this deal's company we cover; name exactly one of the three ids",
      "format": "uuid",
      "type": "string"
    }
  },
  "type": "object"
}
```

</details>

### read_reports

**Read sales reports**

```text
Read sales reports. standard reads the standard metrics, saved reports and their frozen editions,
and the open partner commission liability. run runs one of the named reports, grouped and filtered.
evidence searches the passages behind a saved run. Read only. Every figure names its coverage and
when it was captured; quote both. read_workspace topic report_vocabulary names what each run report
takes. For a written section a reader keeps, such as a board pack, use read_analytics: query with
save, then compose. Use read_forecast for what a period is expected to close. Keep a run's id to
search its evidence, and quote coverage with each number.
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "aggregates": {
      "description": "run only: Omit for the report's own default aggregates.",
      "items": {
        "additionalProperties": false,
        "properties": {
          "as": {
            "description": "Output column name for this aggregate",
            "type": "string"
          },
          "field": {
            "description": "A measure name from this report's list. Omit only with fn=count.",
            "type": "string"
          },
          "fn": {
            "description": "How to aggregate the field. count takes no field; every other function names one from this report's aggregates list.",
            "enum": [
              "avg",
              "count",
              "max",
              "median",
              "min",
              "p75",
              "sum"
            ],
            "type": "string"
          }
        },
        "required": [
          "fn"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "cell": {
      "description": "evidence only: One cell's group key values, in the run's group_by order. Omit to search every record the run measured.",
      "items": {},
      "type": "array"
    },
    "cursor": {
      "description": "standard only",
      "type": "string"
    },
    "evaluated_at": {
      "description": "standard only: RFC 3339 WITH Z or the user's offset (…T14:35:00Z); never invent an offset.",
      "format": "date-time",
      "type": "string"
    },
    "evaluation_key": {
      "description": "standard only",
      "type": "string"
    },
    "filters": {
      "description": "run only: Equality predicates using this report's published filter names.",
      "type": "object"
    },
    "framework_revision": {
      "description": "standard only",
      "type": "integer"
    },
    "group_by": {
      "description": "run only: Published dimension names; omit for default grouping.",
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "id": {
      "description": "standard only",
      "format": "uuid",
      "type": "string"
    },
    "kind": {
      "description": "standard: the standard sales metrics, saved reports and frozen editions; run: one of the named reports, grouped and filtered; evidence: the passages behind a saved run",
      "enum": [
        "evidence",
        "run",
        "standard"
      ],
      "type": "string"
    },
    "limit": {
      "description": "evidence, standard only: How many citations, counterexamples and abstentions to return, each; at most 25 for evidence.",
      "maximum": 100,
      "minimum": 1,
      "type": "integer"
    },
    "mode": {
      "description": "standard only",
      "enum": [
        "catalog",
        "evaluate",
        "reports",
        "report",
        "report_evaluation",
        "editions",
        "edition",
        "evidence",
        "compare",
        "commission_summary"
      ],
      "type": "string"
    },
    "query": {
      "description": "evidence only: The words the evidence would carry.",
      "maxLength": 1000,
      "type": "string"
    },
    "reference": {
      "description": "standard only",
      "properties": {
        "context_id": {
          "type": "string"
        },
        "group_key": {
          "type": "string"
        },
        "metric": {
          "type": "string"
        },
        "through": {
          "description": "RFC 3339 WITH Z or the user's offset (…T14:35:00Z); never invent an offset.",
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "context_id",
        "metric"
      ],
      "type": "object"
    },
    "report": {
      "description": "run only: The prebuilt report to run. Send `report` ALONE for the default answer listed below — that call takes no other argument and needs nothing read first. activities-by-kind: count as activities grouped by kind. deals-by-stage: count as deals, sum(amount_minor) as amount_minor_sum grouped by stage_id, currency. forecast: count as deals, sum(amount_minor) as unweighted_minor, sum(weighted_amount_minor) as weighted_minor grouped by forecast_category, currency. leads-by-status: count as leads grouped by status. meeting-conversion: count as meetings grouped by became_opportunity. open-deals-per-company: count as open_deals grouped by company_id. pipeline-current: count as deals, sum(amount_base_minor) as amount_base_minor_sum, sum(weighted_base_minor) as weighted_base_minor_sum, count(amount_base_minor) as priced_deals grouped by stage_id. project-commitments: sum(overdue_commitments), sum(open_commitments) grouped by project_id, name, key, phase, owner_id. projects-by-phase: count as projects, sum(open_deal_value_minor), sum(won_deal_value_minor), sum(open_deal_unpriced), sum(won_deal_unpriced) grouped by phase. projects-gone-quiet: count as projects grouped by project_id, name, key, phase, owner_id, last_activity_at, quiet_since. stage-age: count as deals, median(days_in_stage) as median_days, p75(days_in_stage) as p75_days grouped by stage_id. win-loss: count as deals, sum(amount_minor) as amount_minor_sum, median(days_to_close) as median_days_to_close, p75(days_to_close) as p75_days_to_close grouped by status, currency. A default is not a report's reach: each slices by dimensions the line above does not name, so a breakdown no default shows is usually still one of these reports. Those dimension names, and its `filters` and `aggregates`, are that report's ALONE, published at margince://schema/reports and answered by read_workspace with topic=report_vocabulary; a name outside them is refused with the accepted list. A `pipeline_id` or `stage_id` used in a plan comes from the workspace's pipeline list.",
      "enum": [
        "activities-by-kind",
        "deals-by-stage",
        "forecast",
        "leads-by-status",
        "meeting-conversion",
        "open-deals-per-company",
        "pipeline-current",
        "project-commitments",
        "projects-by-phase",
        "projects-gone-quiet",
        "stage-age",
        "win-loss"
      ],
      "type": "string"
    },
    "revision": {
      "description": "standard only",
      "minimum": 1,
      "type": "integer"
    },
    "right_id": {
      "description": "standard only",
      "format": "uuid",
      "type": "string"
    },
    "run_id": {
      "description": "evidence only: A saved run: an analytics query with save answers one.",
      "format": "uuid",
      "type": "string"
    },
    "scheduled": {
      "description": "standard only",
      "type": "boolean"
    },
    "scope": {
      "description": "run only",
      "properties": {
        "id": {
          "format": "uuid",
          "type": "string"
        },
        "kind": {
          "type": "string"
        }
      },
      "required": [
        "kind"
      ],
      "type": "object"
    },
    "selection": {
      "description": "standard only",
      "properties": {
        "blocks": {
          "items": {
            "type": "string"
          },
          "type": "array"
        },
        "close_window": {
          "type": "string"
        },
        "interval": {
          "properties": {
            "end_at": {
              "description": "RFC 3339 WITH Z or the user's offset (…T14:35:00Z); never invent an offset.",
              "format": "date-time",
              "type": "string"
            },
            "start_at": {
              "description": "RFC 3339 WITH Z or the user's offset (…T14:35:00Z); never invent an offset.",
              "format": "date-time",
              "type": "string"
            }
          },
          "required": [
            "end_at",
            "start_at"
          ],
          "type": "object"
        },
        "layout": {
          "items": {
            "properties": {
              "cards": {
                "items": {
                  "properties": {
                    "id": {
                      "type": "string"
                    },
                    "kind": {
                      "type": "string"
                    },
                    "metric": {
                      "type": "string"
                    },
                    "question": {
                      "properties": {
                        "entity": {
                          "type": "string"
                        },
                        "group_by": {
                          "items": {
                            "type": "string"
                          },
                          "type": "array"
                        },
                        "limit": {
                          "type": "integer"
                        },
                        "measures": {
                          "items": {
                            "properties": {
                              "as": {
                                "type": "string"
                              },
                              "field": {
                                "type": "string"
                              },
                              "fn": {
                                "type": "string"
                              }
                            },
                            "required": [
                              "fn"
                            ],
                            "type": "object"
                          },
                          "type": "array"
                        }
                      },
                      "required": [
                        "entity",
                        "measures"
                      ],
                      "type": "object"
                    },
                    "width": {
                      "type": "string"
                    }
                  },
                  "required": [
                    "id",
                    "kind",
                    "width"
                  ],
                  "type": "object"
                },
                "type": "array"
              },
              "title": {
                "type": "string"
              }
            },
            "required": [
              "cards"
            ],
            "type": "object"
          },
          "type": "array"
        },
        "metrics": {
          "items": {
            "type": "string"
          },
          "type": "array"
        },
        "period": {
          "type": "string"
        },
        "pipeline_id": {
          "format": "uuid",
          "type": "string"
        },
        "scope": {
          "properties": {
            "id": {
              "format": "uuid",
              "type": "string"
            },
            "kind": {
              "type": "string"
            },
            "label": {
              "type": "string"
            }
          },
          "required": [
            "kind"
          ],
          "type": "object"
        },
        "target_basis": {
          "type": "string"
        }
      },
      "required": [
        "blocks",
        "close_window",
        "metrics",
        "period",
        "scope",
        "target_basis"
      ],
      "type": "object"
    }
  },
  "required": [
    "kind"
  ],
  "type": "object"
}
```

</details>

### read_tags

**Read tags**

```text
Read the workspace's tags. list: every word, with the tag_id the other tag calls take. get: one tag
and how many contacts, companies, deals and leads carry it — the records themselves come from
find_records with engine=list. record: the tags on one contact, company, deal or lead, with who
applied each and when. list returns archived words only with include_archived, and they cannot be
applied; `truncated` means a word missing from the list may still exist. In record, `withheld` true
means the vocabulary is hidden from this caller, NOT that the record has no tags — never report it
as none. A word the user says fits a contact you are creating goes to create_record as offer_tag,
which offers it without applying it.
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "include_archived": {
      "description": "list only: Also list retired words; they cannot be applied",
      "type": "boolean"
    },
    "mode": {
      "description": "list every word, get one tag, or record: the tags on one record",
      "enum": [
        "get",
        "list",
        "record"
      ],
      "type": "string"
    },
    "record_id": {
      "description": "record only",
      "format": "uuid",
      "type": "string"
    },
    "record_type": {
      "description": "record only",
      "enum": [
        "contact",
        "company",
        "deal",
        "lead"
      ],
      "type": "string"
    },
    "tag_id": {
      "description": "get only",
      "format": "uuid",
      "type": "string"
    },
    "with_carried_by": {
      "description": "list only: Also count the records you can see carrying each word",
      "type": "boolean"
    }
  },
  "required": [
    "mode"
  ],
  "type": "object"
}
```

</details>

### read_view

**Read a view**

```text
Read what the record tools do not answer: a record's derived views (company_360, contact_graph,
deal_stakeholders, company_facts), the workspace's reference lists (stages, lead_sources, teams,
record_roles) and reporting objects (reporting_targets, forecast_calls). Each view answers exactly
what the matching REST route answers. Name the view, then the id it reads and any of its filters. A
view refuses an argument it does not take and names the ones it does.
```

<details><summary>Input schema</summary>

```json
{
  "additionalProperties": false,
  "properties": {
    "agg": {
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "as_of": {
      "type": "string"
    },
    "blocks": {
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "by": {
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "category": {
      "type": "string"
    },
    "close_window": {
      "type": "string"
    },
    "company_id": {
      "type": "string"
    },
    "contact_id": {
      "type": "string"
    },
    "contract_id": {
      "type": "string"
    },
    "cursor": {
      "type": "string"
    },
    "deal_id": {
      "type": "string"
    },
    "doc_state": {
      "type": "string"
    },
    "end_at": {
      "type": "string"
    },
    "entity_id": {
      "description": "one value; a list of values for list_members",
      "type": "string"
    },
    "entity_type": {
      "type": "string"
    },
    "evaluated_at": {
      "type": "string"
    },
    "evaluation_key": {
      "type": "string"
    },
    "facts_only": {
      "type": "boolean"
    },
    "framework_revision": {
      "type": "integer"
    },
    "handle_version": {
      "type": "string"
    },
    "id": {
      "description": "one value; a list of values for name_seats",
      "type": "string"
    },
    "include_archived": {
      "type": "boolean"
    },
    "include_hidden": {
      "type": "boolean"
    },
    "kind": {
      "type": "string"
    },
    "limit": {
      "type": "integer"
    },
    "list_type": {
      "type": "string"
    },
    "metrics": {
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "object": {
      "type": "string"
    },
    "period": {
      "type": "string"
    },
    "period_start": {
      "type": "string"
    },
    "pinned_only": {
      "type": "boolean"
    },
    "pipeline_id": {
      "description": "comes from the workspace's pipeline list",
      "type": "string"
    },
    "project_id": {
      "type": "string"
    },
    "q": {
      "type": "string"
    },
    "readId": {
      "type": "string"
    },
    "record_id": {
      "type": "string"
    },
    "record_type": {
      "type": "string"
    },
    "refresh": {
      "type": "boolean"
    },
    "report": {
      "type": "string"
    },
    "resolution_state": {
      "type": "string"
    },
    "resource": {
      "type": "string"
    },
    "retired": {
      "type": "boolean"
    },
    "scope": {
      "type": "string"
    },
    "scope_id": {
      "type": "string"
    },
    "scope_kind": {
      "type": "string"
    },
    "sharing": {
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "sort": {
      "type": "string"
    },
    "start_at": {
      "type": "string"
    },
    "status": {
      "type": "string"
    },
    "target_basis": {
      "type": "string"
    },
    "thread_cursor": {
      "type": "string"
    },
    "view": {
      "enum": [
        "acquisition_sources",
        "activity_review_templates",
        "agent_open_api_document",
        "automation",
        "automation_catalog",
        "automations",
        "bulk_change",
        "company_360",
        "company_contacts",
        "company_coverage",
        "company_documents",
        "company_facts",
        "company_finance_summary",
        "company_graph",
        "company_hierarchy_rollup",
        "company_profile_fields",
        "company_strength",
        "company_vat_check",
        "consent_purposes",
        "contact_360",
        "contact_graph",
        "contact_strength",
        "custom_field_live_lists",
        "custom_fields",
        "deal_commitments",
        "deal_documents",
        "deal_outcome_reviews",
        "deal_stakeholders",
        "deal_status",
        "dedupe_candidate",
        "dedupe_candidates",
        "email_presentation",
        "email_signature_template",
        "explain_report",
        "export_reporting_edition",
        "export_reporting_evaluation",
        "filter_vocabulary",
        "follow_up_settings",
        "forecast_calls",
        "import_run",
        "latest_site_read",
        "latest_technical_enrich",
        "latest_transcript_read",
        "lead_disqualify_reasons",
        "lead_origins",
        "lead_settings",
        "lead_sources",
        "list_members",
        "lists",
        "meeting_brief",
        "meeting_invitation",
        "name_seats",
        "pipeline",
        "preview_contact_employment_import",
        "project_health_assessments",
        "project_stakeholders",
        "record_assignments",
        "record_lists",
        "record_roles",
        "relationships",
        "reply_recipient",
        "reporting_execution",
        "reporting_executions",
        "reporting_framework",
        "reporting_schedules",
        "reporting_target",
        "reporting_targets",
        "scheduling_profile",
        "signal",
        "signal_intro_path",
        "signal_warmth",
        "signals",
        "site_read",
        "stage",
        "stage_automation_report",
        "stage_evidence",
        "stage_exit_criteria",
        "stages",
        "teams",
        "transcript_read",
        "transition_policies"
      ],
      "type": "string"
    },
    "window_days": {
      "type": "integer"
    }
  },
  "required": [
    "view"
  ],
  "type": "object"
}
```

</details>

<details><summary>Output schema</summary>

```json
{
  "properties": {
    "data": {
      "type": "object"
    },
    "evidence": {
      "items": {
        "properties": {
          "captured_by": {
            "type": "string"
          },
          "record_id": {
            "format": "uuid",
            "type": "string"
          },
          "record_type": {
            "type": "string"
          },
          "source": {
            "type": "string"
          }
        },
        "required": [
          "record_id",
          "record_type"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "freshness": {
      "properties": {
        "authoritative": {
          "type": "boolean"
        },
        "last_synced_at": {
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "authoritative"
      ],
      "type": "object"
    },
    "schema_version": {
      "type": "string"
    },
    "trace_id": {
      "type": "string"
    },
    "trust": {
      "type": "string"
    },
    "warnings": {
      "items": {
        "properties": {
          "code": {
            "type": "string"
          },
          "message": {
            "type": "string"
          }
        },
        "required": [
          "code",
          "message"
        ],
        "type": "object"
      },
      "type": "array"
    }
  },
  "required": [
    "data",
    "evidence",
    "freshness",
    "schema_version",
    "trace_id",
    "trust",
    "warnings"
  ],
  "type": "object"
}
```

</details>

### read_workspace

**Read how this workspace is set up**

```text
Read how this workspace is set up. colleagues: the seats that can receive work, not the contacts
stored as records; q narrows by name. pipelines: every pipeline with its stages, each open, won or
lost. channel_providers: the messaging transports. The other topics name what a tool accepts: query
(find_records engine query), analytics (read_analytics), record_fields (create_record and
update_record), list_filters (find_records engine list), report_vocabulary (read_reports kind run),
report_blocks (read_analytics step compose). A q matching nobody answers with every colleague and a
warning. A stage's semantic, not its name, says whether it closes a deal. None of the name topics is
a prerequisite: each tool refuses an unknown name with the accepted list. Use find_records to find a
customer contact; colleagues are seats here. user_id is what assignee_id and owner_id take; never
assign to an is_agent seat. Keep pipeline_id and stage_id for a deal. Take every name verbatim; a
custom field is spelled cf_<slug>.
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "include_archived": {
      "description": "pipelines only: Also list retired pipelines; a deal cannot be moved into one",
      "type": "boolean"
    },
    "include_inactive": {
      "description": "colleagues only: Also list deactivated and suspended seats, for a member administrator only",
      "type": "boolean"
    },
    "include_invited": {
      "description": "colleagues only: Also list invited seats that have not signed in yet",
      "type": "boolean"
    },
    "q": {
      "description": "colleagues only: Narrow by name or email; omit for the whole roster",
      "type": "string"
    },
    "topic": {
      "description": "colleagues, pipelines or channel_providers; or which names a tool accepts: query, analytics, record_fields, list_filters, report_blocks, report_vocabulary",
      "enum": [
        "analytics",
        "channel_providers",
        "colleagues",
        "list_filters",
        "pipelines",
        "query",
        "record_fields",
        "report_blocks",
        "report_vocabulary"
      ],
      "type": "string"
    }
  },
  "required": [
    "topic"
  ],
  "type": "object"
}
```

</details>

### resolve_duplicates

**Merge two records or dismiss a flagged pair**

```text
Settle two records that may be one. With candidate_id and decision: a pair create_record flagged in
duplicate_candidates — not_the_same says they are two, so the pair is not flagged again, and
reopen takes that back; neither record changes. With record_type, source_id and target_id: merge two
contacts or two companies into one, moving the source's activities, deals and links onto the
survivor. A merge archives the source and redirects it to the target; it cannot be reversed by
merging the other way round, and by default nothing holds it. A merged pair cannot be reopened. Use
archive_record when the extra record has nothing worth keeping. target_id is the record that
survives and source_id the one merged away — read both records before choosing. (Runs immediately;
requires passport scope "write".)
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "approval_id": {
      "description": "Set on approved retry",
      "format": "uuid",
      "type": "string"
    },
    "candidate_id": {
      "description": "A flagged pair's candidate_id, to dismiss or reopen it; leave out when merging",
      "format": "uuid",
      "type": "string"
    },
    "decision": {
      "description": "not_the_same dismisses the pair; reopen takes a dismissal back",
      "enum": [
        "not_the_same",
        "reopen"
      ],
      "type": "string"
    },
    "idempotency_key": {
      "description": "A retry without it may repeat the write. Same key, same result; a key reused with other arguments is refused.",
      "maxLength": 255,
      "type": "string"
    },
    "record_type": {
      "enum": [
        "contact",
        "company"
      ],
      "type": "string"
    },
    "source_id": {
      "description": "The record merged away (archived, redirected to the survivor); leave out with candidate_id",
      "format": "uuid",
      "type": "string"
    },
    "target_id": {
      "description": "The surviving record everything relinks to",
      "format": "uuid",
      "type": "string"
    }
  },
  "type": "object"
}
```

</details>

### schedule_meeting

**Book or invite to a meeting**

```text
Put a meeting on the calendar. book places it on the calendar of the seat this passport acts for.
invite sends the invitation to its attendees. book runs at once; invite reaches attendees outside
the workspace and waits for a human. Read check_availability before booking a time. Use
change_activities with log to record a meeting that already happened. Keep the meeting's id to send
its invitation. (Governance: runs immediately for book; a human approves first for invite; requires
passport scope "send".)
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "action": {
      "description": "book: put a meeting on the calendar; invite: send the invitation to the attendees",
      "enum": [
        "book",
        "invite"
      ],
      "type": "string"
    },
    "approval_id": {
      "description": "book only: Set on approved retry",
      "format": "uuid",
      "type": "string"
    },
    "attendee_email": {
      "description": "invite only",
      "type": "string"
    },
    "contact_id": {
      "description": "invite only",
      "format": "uuid",
      "type": "string"
    },
    "description": {
      "description": "invite only",
      "type": "string"
    },
    "end": {
      "description": "RFC 3339 WITH Z or the user's offset (…T14:35:00Z); never invent an offset.",
      "format": "date-time",
      "type": "string"
    },
    "host_user_id": {
      "description": "book only",
      "format": "uuid",
      "type": "string"
    },
    "idempotency_key": {
      "description": "A retry without it may repeat the write. Same key, same result; a key reused with other arguments is refused.",
      "maxLength": 255,
      "type": "string"
    },
    "links": {
      "description": "book only: Who and what the meeting is about; at least one. The booking is refused without it.",
      "items": {
        "additionalProperties": false,
        "properties": {
          "entity_id": {
            "format": "uuid",
            "type": "string"
          },
          "entity_type": {
            "enum": [
              "contact",
              "company",
              "deal",
              "lead",
              "project"
            ],
            "type": "string"
          }
        },
        "required": [
          "entity_type",
          "entity_id"
        ],
        "type": "object"
      },
      "maxItems": 25,
      "minItems": 1,
      "type": "array"
    },
    "location": {
      "description": "invite only",
      "type": "string"
    },
    "start": {
      "description": "RFC 3339 WITH Z or the user's offset (…T14:35:00Z); never invent an offset.",
      "format": "date-time",
      "type": "string"
    },
    "subject": {
      "type": "string"
    },
    "video_call": {
      "description": "invite only",
      "type": "boolean"
    }
  },
  "required": [
    "action",
    "start",
    "end"
  ],
  "type": "object"
}
```

</details>

### send_outbound

**Send a message**

```text
Put a message on the wire to a real recipient, exactly as given; it composes nothing. email_reply: a
mail answering a thread recorded here, by activity_id. email_new: a first mail starting a new
conversation, filed on `links` naming the records it is about. chat_reply: an answer on a captured
chat conversation, by activity_id, on the channel it came from. Every recipient must have granted
the consent purpose the call names. A sent message cannot be recalled, and by default nothing holds
it: where an installation has raised a send to confirm first, the answer is a staged approval
instead. chat_reply cannot start a conversation or choose a channel. Use draft_outbound first when
the message does not exist yet, and change_activities with action=log to record that something was
said rather than say it. Send the subject, body and activity_id or links the draft produced, and
keep the staged approval id: the approval binds that exact message, so changed text needs a new one.
email_new answers with the new conversation's activity_id. (Runs immediately; requires passport
scope "send".)
```

<details><summary>Input schema</summary>

```json
{
  "properties": {
    "activity_id": {
      "description": "chat_reply, email_reply only: The captured conversation being replied to",
      "format": "uuid",
      "type": "string"
    },
    "approval_id": {
      "description": "Set on approved retry",
      "format": "uuid",
      "type": "string"
    },
    "as": {
      "description": "email_reply answers a recorded mail thread, email_new starts one filed on links, chat_reply answers a captured chat",
      "enum": [
        "chat_reply",
        "email_new",
        "email_reply"
      ],
      "type": "string"
    },
    "body": {
      "minLength": 1,
      "type": "string"
    },
    "cc": {
      "description": "email_new, email_reply only",
      "items": {
        "format": "email",
        "type": "string"
      },
      "type": "array"
    },
    "communication_context": {
      "description": "What kind of message this is. Omit to let the server resolve it from the thread; the claim is recorded and grants nothing.",
      "enum": [
        "account_notice",
        "active_deal_followup",
        "contract_notice",
        "customer_service",
        "invoice_or_payment",
        "marketing",
        "precontract_quote",
        "reply_to_inbound",
        "requested_followup"
      ],
      "type": "string"
    },
    "consent_purpose": {
      "description": "Legacy purpose key, optional. communication_context is what the engine decides on; this is read only where the context leaves the question open. Naming neither is allowed and the server resolves what it can from the thread, but a message it cannot place is refused rather than guessed at",
      "type": "string"
    },
    "evidence": {
      "additionalProperties": false,
      "description": "The record that bears out the category: required for invoice_or_payment, contract_notice and precontract_quote, which cannot be allowed without one",
      "properties": {
        "contract_id": {
          "format": "uuid",
          "type": "string"
        },
        "deal_id": {
          "format": "uuid",
          "type": "string"
        },
        "invoice_id": {
          "format": "uuid",
          "type": "string"
        }
      },
      "type": "object"
    },
    "idempotency_key": {
      "description": "A retry without it may repeat the write. Same key, same result; a key reused with other arguments is refused.",
      "maxLength": 255,
      "type": "string"
    },
    "links": {
      "description": "email_new only: The records this conversation is filed under; at least one. The send is refused without it.",
      "items": {
        "additionalProperties": false,
        "properties": {
          "entity_id": {
            "format": "uuid",
            "type": "string"
          },
          "entity_type": {
            "enum": [
              "contact",
              "company",
              "deal",
              "lead",
              "project"
            ],
            "type": "string"
          }
        },
        "required": [
          "entity_type",
          "entity_id"
        ],
        "type": "object"
      },
      "maxItems": 25,
      "minItems": 1,
      "type": "array"
    },
    "marketing_purpose": {
      "description": "For marketing, the purpose key naming the topic",
      "type": "string"
    },
    "operator_reason": {
      "description": "Why this first message is being sent. Recorded; grants nothing.",
      "maxLength": 500,
      "type": "string"
    },
    "scheduled_at": {
      "description": "email_new, email_reply only: RFC 3339 WITH Z or the user's offset (…T14:35:00Z); never invent an hour or offset.",
      "format": "date-time",
      "type": "string"
    },
    "scheduled_tz": {
      "description": "email_new, email_reply only: IANA zone name the moment was chosen in (e.g. Europe/Berlin), required with scheduled_at. The send is deferred to that instant: no activity exists until it fires, and every gate re-runs then.",
      "type": "string"
    },
    "subject": {
      "description": "email_new, email_reply only",
      "type": "string"
    },
    "to": {
      "description": "email_new, email_reply only",
      "items": {
        "format": "email",
        "type": "string"
      },
      "minItems": 1,
      "type": "array"
    }
  },
  "required": [
    "as",
    "body"
  ],
  "type": "object"
}
```

</details>

### update_record

**Update a record**

```text
Change stored field values on a record that already exists — a corrected title, an amount, an
expected close date. Only the fields you send change, and only the fields the record type stores (a
contact's email addresses are not among them). A field a HUMAN last set is not overwritten: that
part is staged for a human and named in the result, and that part of the write has not happened. It
names the record by id; when a name matches two records, a human picks. owner_id is NOT neutral —
ownership decides visibility, so reassigning moves the record onto someone else's book and can take
it off the owner's. An activity moved to another day keeps its time of day unless the user names
one. Use change_state with action=move_deal to move a deal between stages, and change_activities
with action=relink to change what an activity is about; neither is a field edit. Send if_version
with the version you read, and keep the staged approval id from the result if you intend to retry
the same change once a human has released it. (Runs immediately; requires passport scope "write".)
```

Renders its result in [`ui://margince/field-conflict.html`](#field_conflict_view), visible to `model`, `app`.

<details><summary>Input schema</summary>

```json
{
  "additionalProperties": false,
  "properties": {
    "approval_id": {
      "description": "Set on retry after a human approved overwriting their edit; send it with exactly the staged replay arguments",
      "format": "uuid",
      "type": "string"
    },
    "fields": {
      "description": "Only sent fields change. Fields a human last edited are not applied: they are staged for approval and named in the result's staged_approval. The crm.yaml body for the record_type. The fields each record_type takes, which of them are REQUIRED, and their shapes are published at margince://schema/record-fields, and answered by read_workspace with topic=record_fields — that document, not this description, is what says what a write may name. An extra key must be cf_\u003cslug\u003e for a custom field; any other key is refused BY NAME and never dropped in silence, so a wrong guess is answered with the vocabulary rather than lost. An rfc3339 value carries Z or the offset the user gave. Any field holding a sentence — a description, a summary, a note — is written in whoami's prose_language, whatever language this conversation is in.",
      "type": "object"
    },
    "id": {
      "format": "uuid",
      "type": "string"
    },
    "idempotency_key": {
      "description": "A retry without it may repeat the write. Same key, same result; a key reused with other arguments is refused.",
      "maxLength": 255,
      "type": "string"
    },
    "if_version": {
      "description": "Optimistic-concurrency guard: the last-seen record version",
      "type": "integer"
    },
    "record_type": {
      "enum": [
        "contact",
        "company",
        "deal",
        "lead",
        "activity",
        "project",
        "relationship",
        "product",
        "offer_template",
        "offer"
      ],
      "type": "string"
    }
  },
  "required": [
    "record_type",
    "id",
    "fields"
  ],
  "type": "object"
}
```

</details>

<details><summary>Output schema</summary>

```json
{
  "properties": {
    "data": {
      "properties": {
        "fields": {
          "type": "object"
        },
        "id": {
          "format": "uuid",
          "type": "string"
        },
        "record_type": {
          "type": "string"
        },
        "staged_approval": {
          "properties": {
            "approval_id": {
              "format": "uuid",
              "type": "string"
            },
            "fields": {
              "items": {
                "type": "string"
              },
              "type": "array"
            },
            "message": {
              "type": "string"
            },
            "replay": {
              "type": "object"
            }
          },
          "required": [
            "approval_id",
            "fields",
            "message",
            "replay"
          ],
          "type": "object"
        },
        "trust_tier": {
          "type": "string"
        },
        "version": {
          "type": "integer"
        }
      },
      "required": [
        "fields",
        "id",
        "record_type"
      ],
      "type": "object"
    },
    "evidence": {
      "items": {
        "properties": {
          "captured_by": {
            "type": "string"
          },
          "record_id": {
            "format": "uuid",
            "type": "string"
          },
          "record_type": {
            "type": "string"
          },
          "source": {
            "type": "string"
          }
        },
        "required": [
          "record_id",
          "record_type"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "freshness": {
      "properties": {
        "authoritative": {
          "type": "boolean"
        },
        "last_synced_at": {
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "authoritative"
      ],
      "type": "object"
    },
    "schema_version": {
      "type": "string"
    },
    "trace_id": {
      "type": "string"
    },
    "trust": {
      "type": "string"
    },
    "warnings": {
      "items": {
        "properties": {
          "code": {
            "type": "string"
          },
          "message": {
            "type": "string"
          }
        },
        "required": [
          "code",
          "message"
        ],
        "type": "object"
      },
      "type": "array"
    }
  },
  "required": [
    "data",
    "evidence",
    "freshness",
    "schema_version",
    "trace_id",
    "trust",
    "warnings"
  ],
  "type": "object"
}
```

</details>

### whoami

**Who this passport acts for**

```text
Name the human this passport acts for: their id, display name, email and language. It reads only,
and answers this call's acting user — not a directory. acting_user_id is what owner_id and
assignee_id take for "me". prose_language is the language every stored sentence is written in — a
note, a description, a summary — whatever language the conversation itself is in; it is always
answered, where locale is absent until this contact chooses one.
```

<details><summary>Input schema</summary>

```json
{
  "additionalProperties": false,
  "properties": {},
  "type": "object"
}
```

</details>

<details><summary>Output schema</summary>

```json
{
  "properties": {
    "data": {
      "properties": {
        "acting_user_id": {
          "format": "uuid",
          "type": "string"
        },
        "display_name": {
          "type": "string"
        },
        "email": {
          "type": "string"
        },
        "locale": {
          "type": "string"
        },
        "prose_language": {
          "type": "string"
        },
        "timezone": {
          "type": "string"
        }
      },
      "required": [
        "acting_user_id",
        "display_name",
        "email",
        "prose_language"
      ],
      "type": "object"
    },
    "evidence": {
      "items": {
        "properties": {
          "captured_by": {
            "type": "string"
          },
          "record_id": {
            "format": "uuid",
            "type": "string"
          },
          "record_type": {
            "type": "string"
          },
          "source": {
            "type": "string"
          }
        },
        "required": [
          "record_id",
          "record_type"
        ],
        "type": "object"
      },
      "type": "array"
    },
    "freshness": {
      "properties": {
        "authoritative": {
          "type": "boolean"
        },
        "last_synced_at": {
          "format": "date-time",
          "type": "string"
        }
      },
      "required": [
        "authoritative"
      ],
      "type": "object"
    },
    "schema_version": {
      "type": "string"
    },
    "trace_id": {
      "type": "string"
    },
    "trust": {
      "type": "string"
    },
    "warnings": {
      "items": {
        "properties": {
          "code": {
            "type": "string"
          },
          "message": {
            "type": "string"
          }
        },
        "required": [
          "code",
          "message"
        ],
        "type": "object"
      },
      "type": "array"
    }
  },
  "required": [
    "data",
    "evidence",
    "freshness",
    "schema_version",
    "trace_id",
    "trust",
    "warnings"
  ],
  "type": "object"
}
```

</details>

