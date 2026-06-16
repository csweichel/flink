module github.com/csweichel/flink/server

go 1.22

require (
	github.com/csweichel/flink/shared v0.0.0
	github.com/gorilla/websocket v1.5.3
	github.com/metoro-io/mcp-golang v0.16.1
	github.com/spf13/cobra v1.8.1
	go.etcd.io/bbolt v1.3.11
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/csweichel/flink/shared => ../shared

require (
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/invopop/jsonschema v0.12.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	golang.org/x/sys v0.25.0 // indirect
)
