package main

import "github.com/baditaflorin/go-common/server"

const Version = "1.2.1"
const serviceName = "go_tos_finder"

func main() {
	server.Run(serviceName, Version, Handler)
}
