// Core's default instance: the go.mod a checkout composes. It requires core
// and each extension it turns on, and nothing else turns one on. A client
// instance is the same file, with released versions instead of these folders.
module github.com/margince/margince/instance

go 1.27.4

require (
	github.com/margince/margince/backend v0.0.0
	github.com/margince/margince/composition v0.0.0
	github.com/margince/margince/extensions/apac v0.0.0
	github.com/margince/margince/extensions/eu v0.0.0
	github.com/margince/margince/extensions/finance-demo v0.0.0 // margince:development-only
	github.com/margince/margince/extensions/openchannel v0.0.0
	github.com/margince/margince/extensions/us v0.0.0
)

replace (
	github.com/margince/margince/backend => ../backend
	github.com/margince/margince/composition => ../composition
	github.com/margince/margince/extensions/apac => ../extensions/apac
	github.com/margince/margince/extensions/eu => ../extensions/eu
	github.com/margince/margince/extensions/finance-demo => ../extensions/finance-demo
	github.com/margince/margince/extensions/openchannel => ../extensions/openchannel
	github.com/margince/margince/extensions/us => ../extensions/us
)
