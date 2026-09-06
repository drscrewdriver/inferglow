module github.com/inferglow/server

go 1.25.0

replace github.com/inferglow/model => ../model

replace github.com/inferglow/orchestrator => ../orchestrator

replace github.com/inferglow/session => ../session

replace github.com/inferglow/observability => ../observability

replace github.com/inferglow/action => ../action

replace github.com/inferglow/audit => ../audit

replace github.com/inferglow/sandbox => ../sandbox

replace github.com/inferglow/flow => ../flow

replace github.com/inferglow/schema => ../schema

replace github.com/inferglow/approval => ../approval

replace github.com/inferglow/storage => ../storage

replace github.com/inferglow/messagebus => ../messagebus

replace github.com/inferglow/workspace => ../workspace

replace github.com/inferglow/rag => ../rag

replace github.com/inferglow/mcpserver => ../mcpserver

require (
	github.com/aymanbagabas/go-pty v0.2.3
	github.com/fsnotify/fsnotify v1.10.1
	github.com/go-playground/validator/v10 v10.30.3
	github.com/gorilla/websocket v1.5.3
	github.com/inferglow/action v0.0.0
	github.com/inferglow/approval v0.0.0
	github.com/inferglow/audit v0.0.0
	github.com/inferglow/flow v0.0.0
	github.com/inferglow/mcpserver v0.0.0
	github.com/inferglow/messagebus v0.0.0-00010101000000-000000000000
	github.com/inferglow/model v0.0.0
	github.com/inferglow/observability v0.0.0
	github.com/inferglow/orchestrator v0.0.0-00010101000000-000000000000
	github.com/inferglow/rag v0.0.0
	github.com/inferglow/sandbox v0.0.0
	github.com/inferglow/session v0.0.0
	github.com/inferglow/storage v0.0.0-00010101000000-000000000000
	github.com/inferglow/workspace v0.0.0-00010101000000-000000000000
	golang.org/x/text v0.37.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/docker v27.5.1+incompatible // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/inferglow/builtins v0.0.0
	github.com/inferglow/context v0.0.0 // indirect
	github.com/inferglow/memory v0.0.0 // indirect
	github.com/inferglow/schema v0.0.0 // indirect
	github.com/inferglow/skill v0.0.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/u-root/u-root v0.16.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.69.0 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/mod v0.35.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/tools v0.44.0 // indirect
)

replace github.com/inferglow/builtins => ../builtins

replace github.com/inferglow/context => ../context

replace github.com/inferglow/memory => ../memory

replace github.com/inferglow/skill => ../skill
