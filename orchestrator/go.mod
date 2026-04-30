module github.com/inferglow/orchestrator

go 1.25.0

require (
	github.com/inferglow/action v0.0.0
	github.com/inferglow/model v0.0.0
	github.com/inferglow/sandbox v0.0.0
	github.com/inferglow/session v0.0.0
)

require gopkg.in/yaml.v3 v3.0.1 // indirect

replace github.com/inferglow/action => ../action

replace github.com/inferglow/model => ../model

replace github.com/inferglow/sandbox => ../sandbox

replace github.com/inferglow/session => ../session
