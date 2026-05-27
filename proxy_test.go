package main

import (
	"fmt"
	"log"
	"net/http"

	"net/http/httputil"
	"net/http/httptest"
)

func main() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dump, _ := httputil.DumpRequest(r, true)
		fmt.Printf("Received Request:\n%s\n", string(dump))
		w.WriteHeader(200)
	})
	log.Fatal(http.ListenAndServe(":8080", handler))
}
