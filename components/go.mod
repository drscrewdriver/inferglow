module github.com/inferglow/components

go 1.25.0

require github.com/inferglow/model v0.0.0-00010101000000-000000000000

require (
	github.com/kr/pretty v0.3.1 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/inferglow/action => ../action

replace github.com/inferglow/model => ../model

// sandbox is pulled in transitively by action; its replace directive is not
// inherited by this module, so it must be re-declared here.
replace github.com/inferglow/sandbox => ../sandbox

replace github.com/inferglow/approval => ../approval
