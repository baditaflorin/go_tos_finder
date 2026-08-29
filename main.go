package main

import (
	"embed"
	"github.com/baditaflorin/go-common/config"
	"github.com/baditaflorin/go-common/server"
)

//go:embed agent.json
var agentFS embed.FS

const Version = "1.8.8"
const serviceName = "go_tos_finder"

// main mirrors server.Run (config load + keystore auth + the canonical /,
// /<service>, and kebab-alias mounts) but registers a real, deterministic
// document-discovery selftest. Deployment smoke must validate parsing and
// classification without depending on a third-party website being reachable
// during a fresh container rollout.
func main() {
	cfg := config.Load(serviceName, Version)
	srv := server.New(cfg, server.WithKeystoreAuth("default_token"), server.WithAgentFromEmbed(agentFS, "agent.json"), server.WithMCP())

	srv.Mux.HandleFunc("/selftest", newSelftest(serviceName, Version).Render)

	srv.Mux.HandleFunc("/", Handler)
	srv.Mux.HandleFunc("/"+serviceName, Handler)
	if alias := server.KebabAlias(serviceName); alias != "" && alias != serviceName {
		srv.Mux.HandleFunc("/"+alias, Handler)
	}

	srv.Start()
}
