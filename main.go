package main

import (
	"github.com/baditaflorin/go-common/config"
	"github.com/baditaflorin/go-common/server"
)

const Version = "1.3.4"
const serviceName = "go_tos_finder"

// main mirrors server.Run (config load + keystore auth + the canonical /,
// /<service>, and kebab-alias mounts) but additionally registers a REAL
// /selftest suite that exercises the live document-discovery path against a
// known-good target. server.Run's default /selftest is a static 200 and would
// not catch a broken fetch path; mounting our own suite makes the deploy smoke
// gate meaningful (200 when the service can do its job, 503 when its outbound
// fetch path is broken).
func main() {
	cfg := config.Load(serviceName, Version)
	srv := server.New(cfg, server.WithKeystoreAuth("default_token"))

	srv.Mux.HandleFunc("/selftest", newSelftest(serviceName, Version).Render)

	srv.Mux.HandleFunc("/", Handler)
	srv.Mux.HandleFunc("/"+serviceName, Handler)
	if alias := server.KebabAlias(serviceName); alias != "" && alias != serviceName {
		srv.Mux.HandleFunc("/"+alias, Handler)
	}

	srv.Start()
}
