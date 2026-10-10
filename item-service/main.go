// Item service (Service B): owns the "items" resource and its own database.
package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"google.golang.org/grpc"

	pb "lostfound/gen/itempb"
)

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	port := getEnv("PORT", "50051")

	// All configuration comes from environment variables (nothing hard-coded).
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnv("DB_HOST", "localhost"),
		getEnv("DB_PORT", "5432"),
		getEnv("DB_USER", "postgres"),
		getEnv("DB_PASSWORD", ""),
		getEnv("DB_NAME", "item_db"),
	)

	db := connectDB(dsn)
	defer db.Close()

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("failed to listen on port %s: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterItemServiceServer(grpcServer, &server{db: db})

	log.Printf("item-service listening on :%s", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC server stopped: %v", err)
	}
}
