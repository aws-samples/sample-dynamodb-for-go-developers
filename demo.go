package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// loadData bulk-loads the sample dataset used by the demo.
func loadData(ctx context.Context, repo *Repository) {
	fmt.Println("Loading sample data...")
	users, orders, orderItems := buildSampleData()
	if err := repo.SeedData(ctx, users, orders, orderItems); err != nil {
		log.Fatalf("Failed to seed data: %v", err)
	}
	fmt.Printf("Successfully loaded %d items.\n", len(users)+len(orders)+len(orderItems))
}

// runDemo exercises every DynamoDB access pattern the workshop teaches, in the
// order they are introduced. Run 'load-data' first so the table is populated.
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

	fmt.Println("\n== Query: items belonging to an order ==")
	items, err := repo.GetOrderItems(ctx, "ord-aaa-001")
	if err != nil {
		log.Fatalf("GetOrderItems failed: %v", err)
	}
	for _, it := range items {
		fmt.Printf("  %s  %s  $%.2f x%d\n", it.ItemID, it.Name, it.Price, it.Quantity)
	}

	fmt.Println("\n== UpdateItem: confirm an order (active status -> keeps placed_id) ==")
	if err := repo.UpdateOrderStatus(ctx, "ord-aaa-001", OrderStatusConfirmed); err != nil {
		log.Fatalf("UpdateOrderStatus (confirm) failed: %v", err)
	}
	confirmed, err := repo.GetOrderByID(ctx, "ord-aaa-001")
	if err != nil {
		log.Fatalf("GetOrderByID after confirm failed: %v", err)
	}
	fmt.Printf("  %s  status=%s (now appears in confirmed placed-index)\n", confirmed.ID, confirmed.Status)

	fmt.Println("\n== UpdateItem: ship an order (inactive status -> removes placed_id) ==")
	if err := repo.UpdateOrderStatus(ctx, "ord-aaa-001", OrderStatusShipped); err != nil {
		log.Fatalf("UpdateOrderStatus (ship) failed: %v", err)
	}
	shipped, err := repo.GetOrderByID(ctx, "ord-aaa-001")
	if err != nil {
		log.Fatalf("GetOrderByID after ship failed: %v", err)
	}
	fmt.Printf("  %s  status=%s (dropped from sparse placed-index)\n", shipped.ID, shipped.Status)

	fmt.Println("\n== DeleteItem: remove an order item ==")
	if err := repo.DeleteOrderItem(ctx, "ord-aaa-001", "item-002"); err != nil {
		log.Fatalf("DeleteOrderItem failed: %v", err)
	}
	remaining, err := repo.GetOrderItems(ctx, "ord-aaa-001")
	if err != nil {
		log.Fatalf("GetOrderItems after delete failed: %v", err)
	}
	fmt.Printf("  order ord-aaa-001 now has %d item(s)\n", len(remaining))

	runAdvancedDemo(ctx, repo)

	fmt.Println("\nDemo complete.")
}

// runAdvancedDemo exercises the remaining patterns the workshop teaches:
// conditional writes, pagination, filtered and parallel scans, cascade delete,
// and transactional reads. It runs after the core walkthrough so it can rely on
// the seeded data.
func runAdvancedDemo(ctx context.Context, repo *Repository) {
	fmt.Println("\n== PutItem (conditional): create a user only if absent ==")
	newUser := User{Username: "dave", FullName: "Dave Miller", Email: "dave@example.com"}
	if err := repo.CreateUserIfNotExists(ctx, newUser); err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			fmt.Println("  user 'dave' already exists — not overwritten")
		} else {
			log.Fatalf("CreateUserIfNotExists failed: %v", err)
		}
	} else {
		fmt.Println("  created user 'dave'")
	}

	fmt.Println("\n== Query (paginated): all of alice's orders, 2 per page ==")
	paged, err := repo.GetAllOrdersPaginated(ctx, "alice", 2)
	if err != nil {
		log.Fatalf("GetAllOrdersPaginated failed: %v", err)
	}
	fmt.Printf("  retrieved %d orders across pages\n", len(paged))

	fmt.Println("\n== UpdateItem (conditional): ship a confirmed order ==")
	// ord-aaa-002 is seeded as confirmed, so this conditional ship succeeds.
	if err := repo.ShipOrder(ctx, "ord-aaa-002"); err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			fmt.Println("  REJECTED: order was not in 'confirmed' status")
		} else {
			log.Fatalf("ShipOrder failed: %v", err)
		}
	} else {
		fmt.Println("  ord-aaa-002 shipped (was confirmed)")
	}

	fmt.Println("\n== UpdateItem (conditional): try to ship a pending order (expect rejection) ==")
	// ord-bbb-001 is pending, so the confirmed-only condition fails.
	err = repo.ShipOrder(ctx, "ord-bbb-001")
	var condErr *types.ConditionalCheckFailedException
	if errors.As(err, &condErr) {
		fmt.Println("  REJECTED as expected: order is not 'confirmed'")
	} else if err != nil {
		log.Fatalf("ShipOrder failed unexpectedly: %v", err)
	} else {
		fmt.Println("  (unexpectedly shipped)")
	}

	fmt.Println("\n== Scan (filtered): all pending orders across the table ==")
	pendingScan, err := repo.ScanOrdersByStatus(ctx, OrderStatusPending)
	if err != nil {
		log.Fatalf("ScanOrdersByStatus failed: %v", err)
	}
	fmt.Printf("  %d pending order items matched the filter\n", len(pendingScan))

	fmt.Println("\n== Scan (parallel): full table with 4 segments ==")
	parallelItems, err := repo.ParallelScan(ctx, 4)
	if err != nil {
		log.Fatalf("ParallelScan failed: %v", err)
	}
	fmt.Printf("  %d items read across 4 segments\n", len(parallelItems))

	fmt.Println("\n== TransactGetItems: consistent snapshot of an order + its items ==")
	snapOrder, snapItems, err := repo.GetOrderSnapshot(ctx, "bob", "ord-bbb-001")
	if err != nil {
		log.Fatalf("GetOrderSnapshot failed: %v", err)
	}
	fmt.Printf("  order %s (status=%s) with %d item(s)\n", snapOrder.ID, snapOrder.Status, len(snapItems))

	fmt.Println("\n== DeleteItem (cascade): delete an order and all its items ==")
	// carol's ord-ccc-001 and its single item are removed together.
	if err := repo.DeleteOrderWithItems(ctx, "ord-ccc-001"); err != nil {
		log.Fatalf("DeleteOrderWithItems failed: %v", err)
	}
	fmt.Println("  deleted ord-ccc-001 and its items")

	fmt.Println("\n== DeleteItem (conditional): cancel a pending order ==")
	// bob's ord-bbb-001 is still pending, so the pending-only cancel succeeds.
	if err := repo.CancelOrder(ctx, "ord-bbb-001"); err != nil {
		var cErr *types.ConditionalCheckFailedException
		if errors.As(err, &cErr) {
			fmt.Println("  REJECTED: order was not 'pending'")
		} else {
			log.Fatalf("CancelOrder failed: %v", err)
		}
	} else {
		fmt.Println("  cancelled ord-bbb-001 (was pending)")
	}
}

// orderDate builds a fixed timestamp (10:00 UTC on the given day) so the demo's
// date-based LSI/GSI queries are deterministic across runs.
func orderDate(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 10, 0, 0, 0, time.UTC)
}

// buildSampleData returns the sample dataset as typed model objects: three
// users, six orders in various states, and six order items. The repository's
// SeedData marshals these the same way CreateUser/CreateOrder/CreateOrderItem
// do, so no hand-written attribute maps are needed.
func buildSampleData() ([]User, []Order, []OrderItem) {
	users := []User{
		{Username: "alice", FullName: "Alice Smith", Email: "alice@example.com"},
		{Username: "bob", FullName: "Bob Johnson", Email: "bob@example.com"},
		{Username: "carol", FullName: "Carol Williams", Email: "carol@example.com"},
	}

	orders := []Order{
		{ID: "ord-aaa-001", UserID: "alice", Status: OrderStatusPending, AddressKey: "home", CreatedAt: orderDate(2024, 1, 10)},
		{ID: "ord-aaa-002", UserID: "alice", Status: OrderStatusConfirmed, AddressKey: "home", CreatedAt: orderDate(2024, 1, 12)},
		{ID: "ord-aaa-003", UserID: "alice", Status: OrderStatusShipped, AddressKey: "home", CreatedAt: orderDate(2024, 1, 8)},
		{ID: "ord-bbb-001", UserID: "bob", Status: OrderStatusPending, AddressKey: "home", CreatedAt: orderDate(2024, 1, 14)},
		{ID: "ord-bbb-002", UserID: "bob", Status: OrderStatusDelivered, AddressKey: "home", CreatedAt: orderDate(2024, 1, 5)},
		{ID: "ord-ccc-001", UserID: "carol", Status: OrderStatusPending, AddressKey: "home", CreatedAt: orderDate(2024, 1, 15)},
	}

	orderItems := []OrderItem{
		{OrderID: "ord-aaa-001", ItemID: "item-001", Name: "Laptop", Price: 1299.99, Quantity: 1},
		{OrderID: "ord-aaa-001", ItemID: "item-002", Name: "Mouse", Price: 29.99, Quantity: 2},
		{OrderID: "ord-aaa-002", ItemID: "item-003", Name: "Keyboard", Price: 79.99, Quantity: 1},
		{OrderID: "ord-bbb-001", ItemID: "item-004", Name: "Monitor", Price: 499.99, Quantity: 1},
		{OrderID: "ord-bbb-001", ItemID: "item-005", Name: "USB Cable", Price: 9.99, Quantity: 3},
		{OrderID: "ord-ccc-001", ItemID: "item-006", Name: "Headphones", Price: 199.99, Quantity: 1},
	}

	return users, orders, orderItems
}
