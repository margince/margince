// The repo's build and gate tooling: the code generators (contract-overlay,
// gen-stubs, gen-agentpolicy, gen-composition and the rest of gen-*) and
// extmigrategate, the extension migration gate. A separate module so the
// oapi-codegen tool directive's dependency zoo never lands in the product
// module's go.mod. The backend require (a directory replace) exists so
// gen-composition validates unit names through the ONE published
// extension.Name rule — scan-time acceptance must never drift from boot-time
// validation — and so extmigrategate reuses dbmigrate's namespace mapping and
// migration loader rather than restating either.
//
// NOT all of it is offline any more: extmigrategate opens database
// connections (pgx), which is why this module now carries a driver. It is
// still tooling — nothing here ships in a binary a customer runs.
module github.com/margince/margince/backend/tools

go 1.27.1

tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen

require (
	github.com/getkin/kin-openapi v0.149.0
	github.com/jackc/pgx/v5 v5.11.0
	github.com/margince/margince/backend v0.0.0
	github.com/oapi-codegen/oapi-codegen/v2 v2.8.0
	golang.org/x/mod v0.41.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/margince/margince/backend => ../

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/dprotaso/go-yit v0.0.0-20220510233725-9ba8df137936 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/go-chi/chi/v5 v5.3.2 // indirect
	github.com/go-openapi/jsonpointer v0.23.1 // indirect
	github.com/go-openapi/swag/jsonname v0.26.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/oapi-codegen/runtime v1.7.0 // indirect
	github.com/oasdiff/yaml v0.1.1 // indirect
	github.com/oasdiff/yaml3 v0.0.14 // indirect
	github.com/prometheus/client_golang v1.24.1 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.71.0 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	github.com/redis/go-redis/v9 v9.22.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	github.com/speakeasy-api/jsonpath v0.6.3 // indirect
	github.com/speakeasy-api/openapi v1.24.0 // indirect
	github.com/vmware-labs/yaml-jsonpath v0.3.2 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)
