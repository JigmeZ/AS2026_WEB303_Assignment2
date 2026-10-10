// Claim service (Service A): owns the "claims" resource and its own database.
// It calls the Item service (Service B) over gRPC, protected by a timeout,
// retries with exponential backoff and a circuit breaker.
package main

import (
	"fmt"
	"log"
	"net"
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

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		log.Printf("invalid value for %s, using default %d", key, def)
	}
	return def
}

func main() {
	port := getEnv("PORT", "50052")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnv("DB_HOST", "localhost"),
		getEnv("DB_PORT", "5432"),
		getEnv("DB_USER", "postgres"),
		getEnv("DB_PASSWORD", ""),
		getEnv("DB_NAME", "claim_db"),
	)
	db := connectDB(dsn)
	defer db.Close()

	// gRPC connection to Service B (connects lazily, so startup order doesn't matter).
	itemAddr := getEnv("ITEM_SERVICE_ADDR", "localhost:50051")
	conn, err := grpc.NewClient(itemAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("could not create item service client: %v", err)
	}
	defer conn.Close()

	itemClient := NewItemClient(itempb.NewItemServiceClient(conn), ItemClientConfig{
		Timeout:          time.Duration(getEnvInt("ITEM_CALL_TIMEOUT_MS", 2000)) * time.Millisecond,
		MaxAttempts:      getEnvInt("ITEM_MAX_ATTEMPTS", 3),
		BaseBackoff:      time.Duration(getEnvInt("ITEM_BACKOFF_MS", 200)) * time.Millisecond,
		FailureThreshold: getEnvInt("CB_FAILURE_THRESHOLD", 3),
		OpenDuration:     time.Duration(getEnvInt("CB_OPEN_SECONDS", 10)) * time.Second,
	})

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("failed to listen on port %s: %v", port, err)
	}
	grpcServer := grpc.NewServer()
	claimpb.RegisterClaimServiceServer(grpcServer, &server{db: db, items: itemClient})

	log.Printf("claim-service listening on :%s (item service at %s)", port, itemAddr)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC server stopped: %v", err)
	}
}
