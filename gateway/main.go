// API Gateway: the single REST entry point. It translates REST calls into gRPC
// calls to the Item and Claim services and maps gRPC errors to HTTP status codes.
package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	claimpb "lostfound/gen/claimpb"
	itempb "lostfound/gen/itempb"
)

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func dial(addr string) *grpc.ClientConn {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("could not create gRPC client for %s: %v", addr, err)
	}
	return conn
}

func main() {
	port := getEnv("PORT", "8080")
	itemAddr := getEnv("ITEM_SERVICE_ADDR", "localhost:50051")
	claimAddr := getEnv("CLAIM_SERVICE_ADDR", "localhost:50052")
	timeoutSec, err := strconv.Atoi(getEnv("REQUEST_TIMEOUT_SECONDS", "15"))
	if err != nil || timeoutSec <= 0 {
		timeoutSec = 15
	}

	itemConn := dial(itemAddr)
	defer itemConn.Close()
	claimConn := dial(claimAddr)
	defer claimConn.Close()

	g := &Gateway{
		items:   itempb.NewItemServiceClient(itemConn),
		claims:  claimpb.NewClaimServiceClient(claimConn),
		timeout: time.Duration(timeoutSec) * time.Second,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Items (Service B)
	mux.HandleFunc("POST /items", g.createItem)
	mux.HandleFunc("GET /items", g.listItems)
	mux.HandleFunc("GET /items/{id}", g.getItem)
	mux.HandleFunc("PUT /items/{id}", g.updateItem)
	mux.HandleFunc("DELETE /items/{id}", g.deleteItem)

	// Claims (Service A)
	mux.HandleFunc("POST /claims", g.createClaim)
	mux.HandleFunc("GET /claims", g.listClaims)
	mux.HandleFunc("GET /claims/{id}", g.getClaim)
	mux.HandleFunc("PUT /claims/{id}", g.updateClaim)
	mux.HandleFunc("DELETE /claims/{id}", g.deleteClaim)

	log.Printf("gateway listening on :%s (items=%s, claims=%s)", port, itemAddr, claimAddr)
	if err := http.ListenAndServe(":"+port, logRequests(mux)); err != nil {
		log.Fatalf("gateway stopped: %v", err)
	}
}

// logRequests logs every request with its duration.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start))
	})
}
