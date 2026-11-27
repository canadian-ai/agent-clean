package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/canadian-ai/agent-clean/web"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:3000", "address to serve AgentClean on")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/{$}", web.Default.Home)
	mux.HandleFunc("/api/repositories", web.Default.Repositories)
	mux.HandleFunc("/api/repository", web.Default.Repository)
	assets := http.FileServer(http.Dir("public"))
	mux.Handle("/styles/", assets)
	mux.Handle("/app.js", assets)
	server := &http.Server{
		Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("AgentClean listening on http://%s", *addr)
	log.Fatal(server.ListenAndServe())
}
