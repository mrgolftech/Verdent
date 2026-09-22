package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets/*
var embedded embed.FS

func Assets() http.Handler {
	sub, err := fs.Sub(embedded, "assets")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}

func Page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := embedded.ReadFile("assets/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	}
}
