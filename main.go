package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

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
		fmt.Printf("Unknown command: %s\n", command)
		fmt.Println("Usage: go run . [load-data|demo]")
		fmt.Println("  load-data  Bulk-load the sample dataset")
		fmt.Println("  demo       Run a full read/query/update walkthrough (default)")
		os.Exit(1)
	}
}

// loadData bulk-loads the sample dataset used by the demo.
func loadData(ctx context.Context, repo *Repository) {
	fmt.Println("Loading sample data...")
	items := buildSampleData()
	if err := repo.BatchWriteItems(ctx, items); err != nil {
		log.Fatalf("Failed to batch write: %v", err)
	}
	fmt.Printf("Successfully loaded %d items.\n", len(items))
}

// runDemo exercises every access pattern in the workshop.
func runDemo(ctx context.Context, repo *Repository) {
	fmt.Println("== GetItem: user profile ==")
	user, err := repo.GetUser(ctx, "alice")
	if err != nil {
		log.Fatalf("GetUser failed: %v (did you run 'go run . load-data' first?)", err)
	}
	fmt.Printf("  %s <%s>\n", user.FullName, user.Email)

	fmt.Println("\n== Query: alice's orders (primary table) ==")
	orders, err := repo.GetOrdersByUserID(ctx, "alice")
	if err != nil {
		log.Fatalf("GetOrdersByUserID failed: %v", err)
	}
	for _, o := range orders {
		fmt.Printf("  %s  status=%s\n", o.ID, o.Status)
	}

	fmt.Println("\n== Query: inverted-index GSI (find order by ID) ==")
	order, err := repo.GetOrderByID(ctx, "ord-aaa-001")
	if err != nil {
		log.Fatalf("GetOrderByID failed: %v", err)
	}
	fmt.Printf("  %s  user=%s  status=%s\n", order.ID, order.UserID, order.Status)

	fmt.Println("\n== Query: placed-index sparse GSI (all pending orders) ==")
	pending, err := repo.GetPendingOrders(ctx)
	if err != nil {
		log.Fatalf("GetPendingOrders failed: %v", err)
	}
	for _, o := range pending {
		fmt.Printf("  %s  user=%s\n", o.ID, o.UserID)
	}

	fmt.Println("\n== Query: status-date-index LSI (alice's pending orders) ==")
	lsiOrders, err := repo.GetUserOrdersByStatus(ctx, "alice", OrderStatusPending)
	if err != nil {
		log.Fatalf("GetUserOrdersByStatus failed: %v", err)
	}
	for _, o := range lsiOrders {
		fmt.Printf("  %s\n", o.ID)
	}

	fmt.Println("\n== Query: status-date-gsi multi-attribute GSI (alice's pending orders since 2024-01-01) ==")
	gsiOrders, err := repo.GetUserOrdersByStatusGSI(ctx, "alice", OrderStatusPending, "2024-01-01T00:00:00Z")
	if err != nil {
		log.Fatalf("GetUserOrdersByStatusGSI failed: %v", err)
	}
	for _, o := range gsiOrders {
		fmt.Printf("  %s  created=%s\n", o.ID, o.CreatedAt.Format("2006-01-02"))
	}

	fmt.Println("\n== Scan: full table count ==")
	all, err := repo.ScanAllItems(ctx)
	if err != nil {
		log.Fatalf("ScanAllItems failed: %v", err)
	}
	fmt.Printf("  %d items in table\n", len(all))

	fmt.Println("\n== Transaction: place a new order for alice ==")
	newOrder := &Order{
		ID:         "ord-demo-txn",
		UserID:     "alice",
		Status:     OrderStatusPending,
		AddressKey: "home",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	txnItems := []OrderItem{
		{ItemID: "demo-item-1", Name: "Desk Lamp", Price: 45.99, Quantity: 1},
	}
	if err := repo.PlaceOrder(ctx, newOrder, txnItems); err != nil {
		log.Fatalf("PlaceOrder failed: %v", err)
	}
	fmt.Printf("  Placed order %s with %d item(s)\n", newOrder.ID, len(txnItems))

	fmt.Println("\nDemo complete.")
}

func buildSampleData() []map[string]types.AttributeValue {
	var items []map[string]types.AttributeValue

	users := []struct {
		username, fullName, email string
	}{
		{"alice", "Alice Smith", "alice@example.com"},
		{"bob", "Bob Johnson", "bob@example.com"},
		{"carol", "Carol Williams", "carol@example.com"},
	}
	for _, u := range users {
		items = append(items, map[string]types.AttributeValue{
			"pk":        &types.AttributeValueMemberS{Value: "#USER#" + u.username},
			"sk":        &types.AttributeValueMemberS{Value: "PROFILE"},
			"full_name": &types.AttributeValueMemberS{Value: u.fullName},
			"email":     &types.AttributeValueMemberS{Value: u.email},
		})
	}

	orders := []struct {
		userID, orderID, status, date, placedID string
	}{
		{"alice", "ord-aaa-001", "pending", "2024-01-10", "pending"},
		{"alice", "ord-aaa-002", "confirmed", "2024-01-12", "confirmed"},
		{"alice", "ord-aaa-003", "shipped", "2024-01-08", ""},
		{"bob", "ord-bbb-001", "pending", "2024-01-14", "pending"},
		{"bob", "ord-bbb-002", "delivered", "2024-01-05", ""},
		{"carol", "ord-ccc-001", "pending", "2024-01-15", "pending"},
	}
	for _, o := range orders {
		item := map[string]types.AttributeValue{
			"pk":          &types.AttributeValueMemberS{Value: "#USER#" + o.userID},
			"sk":          &types.AttributeValueMemberS{Value: "#ORDER#" + o.orderID},
			"order_id":    &types.AttributeValueMemberS{Value: o.orderID},
			"user_id":     &types.AttributeValueMemberS{Value: o.userID},
			"status":      &types.AttributeValueMemberS{Value: o.status},
			"status_date": &types.AttributeValueMemberS{Value: o.status + "#" + o.date},
			"address_key": &types.AttributeValueMemberS{Value: "home"},
			"created_at":  &types.AttributeValueMemberS{Value: o.date + "T10:00:00Z"},
		}
		if o.placedID != "" {
			item["placed_id"] = &types.AttributeValueMemberS{Value: o.placedID}
		}
		items = append(items, item)
	}

	orderItems := []struct {
		orderID, itemID, name, price, qty string
	}{
		{"ord-aaa-001", "item-001", "Laptop", "1299.99", "1"},
		{"ord-aaa-001", "item-002", "Mouse", "29.99", "2"},
		{"ord-aaa-002", "item-003", "Keyboard", "79.99", "1"},
		{"ord-bbb-001", "item-004", "Monitor", "499.99", "1"},
		{"ord-bbb-001", "item-005", "USB Cable", "9.99", "3"},
		{"ord-ccc-001", "item-006", "Headphones", "199.99", "1"},
	}
	for _, oi := range orderItems {
		items = append(items, map[string]types.AttributeValue{
			"pk":       &types.AttributeValueMemberS{Value: "#ORDER#" + oi.orderID},
			"sk":       &types.AttributeValueMemberS{Value: "#ITEM#" + oi.itemID},
			"order_id": &types.AttributeValueMemberS{Value: oi.orderID},
			"item_id":  &types.AttributeValueMemberS{Value: oi.itemID},
			"name":     &types.AttributeValueMemberS{Value: oi.name},
			"price":    &types.AttributeValueMemberN{Value: oi.price},
			"quantity": &types.AttributeValueMemberN{Value: oi.qty},
		})
	}

	return items
}
