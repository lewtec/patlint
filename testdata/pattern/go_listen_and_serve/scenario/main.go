package main

import "net/http"

func run(addr string, h http.Handler) error {
	return http.ListenAndServe(addr, h)
}

func other() {
	_ = http.StatusOK
}
