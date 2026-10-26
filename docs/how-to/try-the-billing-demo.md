# Try the billing demo

The `billing-demo` extension shows an extension writing back to the core. A pretend ERP holds invoices. A paid invoice moves its deal to Won. A deal amount a user edited is not overwritten. This page is for someone running the demo. The code is in `extensions/billing-demo/`.

## What it shows

| Step | What happens in the core |
|---|---|
| Connect | The unit seeds sample invoices and lands one deal per invoice on the open stage. |
| Mark an invoice paid | The deal moves to the won stage and is filed as an imported win. |
| Change an invoice amount after a user edited the deal | The core keeps the user's amount. `status` lists `amount_minor` and `currency` as withheld. |

The feed runs as the unit's [service member](declare-a-service-member.md). The core's human-precedence rule applies to it like any other source. See [land-companies-and-deals-from-an-extension.md](land-companies-and-deals-from-an-extension.md).

## Run it

Start the dev stack with `make dev` and `make seed-dev`, then sign in as an admin. The three calls need the admin role.

```bash
B=http://localhost:8081
curl -c jar -X POST -H 'Content-Type: application/json' \
  -d '{"email":"admin@demo.test","password":"demo-password-123"}' $B/v1/auth/login

# Connect: pass the member, pipeline and stages the feed should use.
curl -b jar -X PUT -H 'Content-Type: application/json' $B/v1/ext/billing-demo/connection \
  -d '{"member_id":"<service member id>","pipeline_id":"<pipeline>","open_stage_id":"<open stage>","won_stage_id":"<won stage>"}'

# Pay an invoice, then read the answer. The sync runs on the next tick.
curl -b jar -X POST -H 'Content-Type: application/json' $B/v1/ext/billing-demo/invoice-update \
  -d '{"erp_id":"INV-1001","paid":true}'
curl -b jar $B/v1/ext/billing-demo/status
```

The service member is created when the stack boots, and the Users screen lists it. The sync job ticks about once a minute, so give a change that long to land.

## Show the protection

1. Edit the amount of a landed deal in the app.
2. Change that invoice's amount with `invoice-update` and `{"erp_id":"INV-1002","amount_minor":1000000}`.
3. After the next tick, `status` shows the invoice as synced with `amount_minor` and `currency` withheld. The deal keeps your amount.
4. To see the other side, turn on "Overwrite fields a user edited" on the unit's card in Extensions, and change the invoice again.

## What it does not do

- It has no screen. The calls above drive it.
- It cannot notify the deal owner. A job tick has no caller, and the notice door refuses unattended calls.
- It exposes no MCP tool yet. See [declare-a-service-member.md](declare-a-service-member.md#no-mcp-tools-yet).
- A deal it creates is owned by the service member, because the core falls back to the acting member.
