package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// main wires up the DynamoDB client and dispatches to a subcommand. The two
// subcommands (load-data, demo) live in demo.go so this file stays a small,
// readable entry point.
func main() {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	tableName := os.Getenv("DYNAMODB_TABLE_NAME")
	if tableName == "" {
		tableName = "simple-inventory"
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	client := dynamodb.NewFromConfig(cfg)
	repo := NewRepository(client, tableName)
	ctx := context.Background()

	// Default to the demo walkthrough when no subcommand is given.
	command := "demo"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	switch command {
	case "load-data":
		loadData(ctx, repo)
	case "demo":
		runDemo(ctx, repo)
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		usage()
		os.Exit(1)
	}
}

// usage prints the available subcommands.
func usage() {
	fmt.Println("Usage: go run . [load-data|demo]")
	fmt.Println("  load-data  Bulk-load the sample dataset")
	fmt.Println("  demo       Run a full read/query/update walkthrough (default)")
}
