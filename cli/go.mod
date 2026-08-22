module github.com/inferglow/cli

go 1.25.0

require (
	charm.land/bubbles/v2 v2.1.0
	charm.land/bubbletea/v2 v2.0.7
	charm.land/lipgloss/v2 v2.0.4
	github.com/atotto/clipboard v0.1.4
	github.com/google/uuid v1.6.0
	github.com/inferglow/action v0.0.0
	github.com/inferglow/audit v0.0.0
	github.com/inferglow/builtins v0.0.0
	github.com/inferglow/context v0.0.0
	github.com/inferglow/memory v0.0.0
	github.com/inferglow/model v0.0.0
	github.com/inferglow/orchestrator v0.0.0
	github.com/inferglow/session v0.0.0
	github.com/inferglow/skill v0.0.0
)

require (
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/ultraviolet v0.0.0-20260525132238-948f4557a654 // indirect
	github.com/charmbracelet/x/ansi v0.11.7 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/docker v27.5.1+incompatible // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/inferglow/approval v0.0.0 // indirect
	github.com/inferglow/flow v0.0.0 // indirect
	github.com/inferglow/sandbox v0.0.0 // indirect
	github.com/inferglow/schema v0.0.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.0 // indirect
	github.com/mattn/go-runewidth v0.0.23 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0-rc3 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.69.0 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	golang.org/x/mod v0.35.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/tools v0.44.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/inferglow/action => ../action

replace github.com/inferglow/audit => ../audit

replace github.com/inferglow/builtins => ../builtins

replace github.com/inferglow/context => ../context

replace github.com/inferglow/model => ../model

replace github.com/inferglow/orchestrator => ../orchestrator

replace github.com/inferglow/sandbox => ../sandbox

replace github.com/inferglow/schema => ../schema

replace github.com/inferglow/session => ../session

replace github.com/inferglow/flow => ../flow

replace github.com/inferglow/observability => ../observability

replace github.com/inferglow/approval => ../approval

replace github.com/inferglow/memory => ../memory

replace github.com/inferglow/skill => ../skill
