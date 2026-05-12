package main

import (
	"net/http"

	"github.com/baditaflorin/go-common/config"
	"github.com/baditaflorin/go-common/server"
)

const Version = "1.0.0"

func main() {
	cfg := config.Load("go_tos_finder", Version)
	srv := server.New(cfg)
	srv.Mux.HandleFunc("/t/", Handler)
	srv.Mux.HandleFunc("/go_tos_finder", Handler)
	srv.Mux.HandleFunc("/", rootHandler)
	srv.Start()
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	Handler(w, r)
}
