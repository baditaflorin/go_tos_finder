package main

import "github.com/baditaflorin/go-common/server"

const Version = "1.3.0"
const serviceName = "go_tos_finder"

func main() {
	server.Run(serviceName, Version, Handler)
}
