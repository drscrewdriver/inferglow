module github.com/inferglow/examples

go 1.25.0

require (
	github.com/inferglow/action v0.0.0
	github.com/inferglow/audit v0.0.0
	github.com/inferglow/context v0.0.0
	github.com/inferglow/model v0.0.0
	github.com/inferglow/orchestrator v0.0.0
	github.com/inferglow/sandbox v0.0.0
	github.com/inferglow/session v0.0.0
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/docker v27.5.1+incompatible // indirect
	github.com/docker/go-connections v0.7.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/inferglow/approval v0.0.0 // indirect
	github.com/inferglow/flow v0.0.0-00010101000000-000000000000 // indirect
	github.com/inferglow/schema v0.0.0 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.69.0 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260720211330-0afa2a65878a // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260720155508-bb71a54f79dc // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/inferglow/model => ./../model

replace github.com/inferglow/schema => ./../schema

replace github.com/inferglow/flow => ./../flow

replace github.com/inferglow/action => ./../action

replace github.com/inferglow/session => ./../session

replace github.com/inferglow/sandbox => ./../sandbox

replace github.com/inferglow/audit => ./../audit

replace github.com/inferglow/context => ./../context

replace github.com/inferglow/orchestrator => ./../orchestrator

replace github.com/inferglow/security => ./../security

replace github.com/inferglow/workspace => ./../workspace

replace github.com/inferglow/server => ../server

replace github.com/inferglow/approval => ./../approval

replace github.com/inferglow/observability => ./../observability

replace github.com/docker/go-connections v0.7.0 => github.com/docker/go-connections v0.4.0
