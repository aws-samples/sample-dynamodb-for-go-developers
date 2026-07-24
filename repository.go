package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// ============================================================================
// LAB BRANCH — fill-in-the-blanks worksheet
// ============================================================================
//
// This is the LAB version of the workshop. Functions marked TODO(lab) are left
// for you to implement as you work through the "DynamoDB for Go Developers"
// instructions. The reference solution for every function lives on the `main`
// branch (git switch main) if you get stuck.
//
// A few functions are already implemented as WORKED EXAMPLES — one per core
// concept. Read them first, then mirror their shape when filling in the stubs
// for the same concept:
//
//   Concept                         Worked example        You implement
//   ------------------------------  --------------------  --------------------
//   PutItem + marshaling            CreateUser            CreateOrder, CreateOrderItem
//   PutItem (conditional)           —                     CreateUserIfNotExists
//   BatchWriteItem (bulk load)      —                     BatchWriteItems, SeedData
//   GetItem                         GetUser               —
//   Query (base table)             GetOrdersByUserID     GetOrderItems
//   Query (paginated)               —                     GetAllOrdersPaginated
//   Query (inverted-index GSI)      —                     GetOrderByID
//   Query (sparse placed-index)     —                     GetPendingOrders
//   Query (status-date-index LSI)   —                     GetUserOrdersByStatus
//   Query (status-date-gsi)         —                     GetUserOrdersByStatusGSI
//   Scan                            —                     ScanAllItems
//   Scan (filtered)                 —                     ScanOrdersByStatus
//   Scan (parallel)                 —                     ParallelScan
//   UpdateItem                      —                     UpdateOrderStatus
//   UpdateItem (conditional)        —                     ShipOrder
//   DeleteItem                      —                     DeleteOrderItem
//   DeleteItem (conditional)        —                     CancelOrder
//   DeleteItem (cascade)            —                     DeleteOrderWithItems
//   TransactWriteItems              —                     PlaceOrder
//   TransactGetItems                —                     GetOrderSnapshot
//
// Every stub currently returns errNotImplemented so the package compiles and
// `go run . demo` runs from the first checkout — it prints exactly which
// patterns are still unimplemented, giving you a live progress checklist.
// Replace the TODO body (and remove the errNotImplemented return) as you go.

// errNotImplemented is returned by every lab stub you haven't filled in yet.
// Once you implement a function, delete its errNotImplemented return.
func errNotImplemented(fn string) error {
	return fmt.Errorf("TODO(lab): %s is not implemented yet — fill it in (see the reference on the main branch)", fn)
}

// Repository holds all DynamoDB data-plane operations. The table itself is
// provisioned with CloudFormation, so there is no CreateTable/DeleteTable here.
type Repository struct {
	client    *dynamodb.Client
	tableName string
}

func NewRepository(client *dynamodb.Client, tableName string) *Repository {
	return &Repository{
		client:    client,
		tableName: tableName,
	}
}

// ---------- Marshaling ----------
//
// These helpers turn a model struct into a DynamoDB item map: they let
// attributevalue.MarshalMap handle the struct fields, then add the
// single-table pk/sk and any derived index attributes. CreateXxx and SeedData
// share them so key construction lives in exactly one place.

// marshalUser is a WORKED EXAMPLE. Study this pattern — every write path
// follows it: marshal the struct, then set the single-table keys by hand.
func marshalUser(user User) (map[string]types.AttributeValue, error) {
	item, err := attributevalue.MarshalMap(user)
	if err != nil {
		return nil, err
	}
	item["pk"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("#USER#%s", user.Username)}
	item["sk"] = &types.AttributeValueMemberS{Value: "PROFILE"}
	return item, nil
}

func marshalOrder(order Order) (map[string]types.AttributeValue, error) {
	// TODO(lab): Marshal the order and add its single-table keys plus the
	// derived index attributes. Mirror marshalUser, then add:
	//   pk          = "#USER#<UserID>"
	//   sk          = "#ORDER#<ID>"
	//   status_date = "<Status>#<CreatedAt as 2006-01-02>"   (sort key for the LSI)
	// and, ONLY when the order is pending or confirmed, set:
	//   placed_id   = string(Status)                          (makes the sparse GSI sparse)
	return nil, errNotImplemented("marshalOrder")
}

func marshalOrderItem(orderID string, orderItem OrderItem) (map[string]types.AttributeValue, error) {
	// TODO(lab): Marshal the order item and add its single-table keys:
	//   pk = "#ORDER#<orderID>"
	//   sk = "#ITEM#<ItemID>"
	// Order items are co-located under their order's partition key.
	return nil, errNotImplemented("marshalOrderItem")
}

// ---------- Write operations ----------

// CreateUser is a WORKED EXAMPLE of PutItem. It marshals the model with the
// shared helper, then writes the single item. The stubs below follow this shape.
func (r *Repository) CreateUser(ctx context.Context, user User) error {
	item, err := marshalUser(user)
	if err != nil {
		return err
	}
	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})
	return err
}

// CreateUserIfNotExists should write a user only when no profile already exists.
func (r *Repository) CreateUserIfNotExists(ctx context.Context, user User) error {
	// TODO(lab): Like CreateUser, but add a ConditionExpression of
	// "attribute_not_exists(pk)" to the PutItemInput so an existing user is not
	// silently overwritten. A duplicate write fails with
	// *types.ConditionalCheckFailedException (catch it with errors.As).
	return errNotImplemented("CreateUserIfNotExists")
}

func (r *Repository) CreateOrder(ctx context.Context, order *Order) error {
	// TODO(lab): Marshal *order with marshalOrder, then PutItem it into the
	// table. Mirror CreateUser.
	return errNotImplemented("CreateOrder")
}

func (r *Repository) CreateOrderItem(ctx context.Context, orderID string, orderItem *OrderItem) error {
	// TODO(lab): Marshal *orderItem with marshalOrderItem, then PutItem it into
	// the table. Mirror CreateUser.
	return errNotImplemented("CreateOrderItem")
}

func (r *Repository) BatchWriteItems(ctx context.Context, items []map[string]types.AttributeValue) error {
	// TODO(lab): Bulk-write items with BatchWriteItem. Two things to handle:
	//   1. Chunk `items` into batches of at most 25 (the BatchWriteItem limit).
	//   2. For each chunk, build a []types.WriteRequest of PutRequests and send
	//      it as RequestItems keyed by r.tableName, then retry any
	//      output.UnprocessedItems until none remain.
	return errNotImplemented("BatchWriteItems")
}

// SeedData marshals typed model objects and bulk-loads them with BatchWriteItems.
// It uses the same marshaling helpers as CreateUser/CreateOrder/CreateOrderItem,
// so the sample dataset is built from models rather than hand-written attribute maps.
func (r *Repository) SeedData(ctx context.Context, users []User, orders []Order, orderItems []OrderItem) error {
	// TODO(lab): Build one []map[string]types.AttributeValue by marshaling every
	// user (marshalUser), order (marshalOrder), and order item (marshalOrderItem),
	// then hand the combined slice to r.BatchWriteItems.
	return errNotImplemented("SeedData")
}

// ---------- Read operations ----------

// GetUser is a WORKED EXAMPLE of GetItem — the most efficient read, a direct
// lookup by full primary key (pk + sk).
func (r *Repository) GetUser(ctx context.Context, username string) (*User, error) {
	result, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("#USER#%s", username)},
			"sk": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
	})
	if err != nil {
		return nil, err
	}
	if result.Item == nil {
		return nil, fmt.Errorf("user not found: %s", username)
	}

	var user User
	if err := attributevalue.UnmarshalMap(result.Item, &user); err != nil {
		return nil, err
	}
	user.Username = username
	return &user, nil
}

// GetOrdersByUserID is a WORKED EXAMPLE of Query on the base table. It fetches
// every item in a partition whose sort key begins with a prefix. Use the same
// KeyConditionExpression shape (pk = :pk AND begins_with(sk, :prefix)) for the
// stubbed base-table and index queries below.
func (r *Repository) GetOrdersByUserID(ctx context.Context, userID string) ([]*Order, error) {
	result, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("pk = :pk AND begins_with(sk, :sk_prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":        &types.AttributeValueMemberS{Value: fmt.Sprintf("#USER#%s", userID)},
			":sk_prefix": &types.AttributeValueMemberS{Value: "#ORDER#"},
		},
	})
	if err != nil {
		return nil, err
	}

	return unmarshalOrders(result.Items, userID), nil
}

// GetAllOrdersPaginated should page through a user's orders explicitly.
func (r *Repository) GetAllOrdersPaginated(ctx context.Context, userID string, pageSize int32) ([]*Order, error) {
	// TODO(lab): Run the same base-table Query as GetOrdersByUserID, but in a
	// loop that walks every page:
	//   - Set Limit to pageSize.
	//   - After each Query, append unmarshalOrders(result.Items, userID) to your
	//     accumulator.
	//   - If result.LastEvaluatedKey is non-nil, set it as the next request's
	//     ExclusiveStartKey and continue; otherwise stop.
	return nil, errNotImplemented("GetAllOrdersPaginated")
}

func (r *Repository) GetOrderItems(ctx context.Context, orderID string) ([]OrderItem, error) {
	// TODO(lab): Query the base table for every item belonging to an order.
	// Mirror GetOrdersByUserID, but:
	//   pk        = "#ORDER#<orderID>"
	//   sk_prefix = "#ITEM#"
	// Unmarshal each result into an OrderItem and return the slice.
	return nil, errNotImplemented("GetOrderItems")
}

// GetOrderByID should use the inverted-index GSI to find an order without
// knowing its user.
func (r *Repository) GetOrderByID(ctx context.Context, orderID string) (*Order, error) {
	// TODO(lab): Query the "inverted-index" GSI (sk -> pk). Set IndexName to
	// "inverted-index", KeyConditionExpression to "sk = :sk" with
	// :sk = "#ORDER#<orderID>", and Limit to 1.
	// The order's user is not stored as an attribute — recover it from the pk of
	// the returned item by stripping the "#USER#" prefix (pkValue[6:]), and set
	// order.ID = orderID before returning.
	return nil, errNotImplemented("GetOrderByID")
}

// GetPendingOrders should use the sparse placed-index GSI.
func (r *Repository) GetPendingOrders(ctx context.Context) ([]*Order, error) {
	// TODO(lab): Query the "placed-index" GSI. Set IndexName to "placed-index"
	// and KeyConditionExpression to "placed_id = :placed_id" with
	// :placed_id = string(OrderStatusPending). Because only pending/confirmed
	// orders carry placed_id, this sparse index returns just the active orders.
	// Unmarshal each item into an *Order.
	return nil, errNotImplemented("GetPendingOrders")
}

// GetUserOrdersByStatus should use the status-date-index LSI (concatenated
// status_date key).
func (r *Repository) GetUserOrdersByStatus(ctx context.Context, userID string, status OrderStatus) ([]*Order, error) {
	// TODO(lab): Query the "status-date-index" LSI. Set IndexName to
	// "status-date-index" and KeyConditionExpression to
	// "pk = :pk AND begins_with(status_date, :status_prefix)" with
	//   :pk            = "#USER#<userID>"
	//   :status_prefix = string(status) + "#"
	// The LSI sorts a user's orders by the concatenated "<status>#<date>" key.
	// Return unmarshalOrders(result.Items, userID).
	return nil, errNotImplemented("GetUserOrdersByStatus")
}

// GetUserOrdersByStatusGSI should use the multi-attribute status-date-gsi
// (status + created_at as two separate sort key attributes — no concatenation).
func (r *Repository) GetUserOrdersByStatusGSI(ctx context.Context, userID string, status OrderStatus, since string) ([]*Order, error) {
	// TODO(lab): Query the "status-date-gsi" multi-attribute GSI. Its sort key is
	// composed of two native attributes, so the KeyConditionExpression is
	// "pk = :pk AND #status = :status AND created_at > :since".
	//   - "status" is a reserved word, so alias it via ExpressionAttributeNames
	//     ("#status" -> "status").
	//   - Range conditions are only allowed on the LAST sort key attribute
	//     (created_at here); the earlier ones must use equality.
	// Values: :pk = "#USER#<userID>", :status = string(status), :since = since.
	// Return unmarshalOrders(result.Items, userID).
	return nil, errNotImplemented("GetUserOrdersByStatusGSI")
}

func (r *Repository) ScanAllItems(ctx context.Context) ([]map[string]types.AttributeValue, error) {
	// TODO(lab): Read every item in the table using a Scan. The SDK's
	// dynamodb.NewScanPaginator(r.client, &dynamodb.ScanInput{TableName: ...})
	// handles the LastEvaluatedKey/ExclusiveStartKey loop for you: while
	// paginator.HasMorePages(), call paginator.NextPage(ctx) and append
	// page.Items to your result slice.
	return nil, errNotImplemented("ScanAllItems")
}

// ScanOrdersByStatus should scan the table with a filter expression.
func (r *Repository) ScanOrdersByStatus(ctx context.Context, status OrderStatus) ([]map[string]types.AttributeValue, error) {
	// TODO(lab): Like ScanAllItems, but give the ScanInput a FilterExpression of
	// "#status = :status AND begins_with(sk, :order_prefix)" with
	//   ExpressionAttributeNames  {"#status": "status"}  ("status" is reserved)
	//   :status       = string(status)
	//   :order_prefix = "#ORDER#"
	// Remember: the filter shrinks the RESULT, not the data DynamoDB reads or
	// charges for. Collect page.Items across all pages.
	return nil, errNotImplemented("ScanOrdersByStatus")
}

// ParallelScan should scan the table concurrently across totalSegments workers.
func (r *Repository) ParallelScan(ctx context.Context, totalSegments int) ([]map[string]types.AttributeValue, error) {
	// TODO(lab): Launch totalSegments goroutines. Each builds its own
	// ScanPaginator with ScanInput.Segment = its index and
	// ScanInput.TotalSegments = totalSegments, drains all its pages, and sends
	// its items (or an error) back on a channel. The caller collects results
	// from the channel — return the first error, otherwise the combined items.
	return nil, errNotImplemented("ParallelScan")
}

// ---------- Update operations ----------

func (r *Repository) UpdateOrderStatus(ctx context.Context, orderID string, newStatus OrderStatus) error {
	// TODO(lab): Change an order's status with UpdateItem, keeping the sparse
	// index consistent.
	//   1. Look up the order first (r.GetOrderByID) to learn its UserID for the key.
	//   2. Build a SET clause for status, status_date ("<newStatus>#<today>"),
	//      and updated_at. "status" is reserved — alias every name via
	//      ExpressionAttributeNames.
	//   3. If newStatus is pending or confirmed, fold "#placed_id = :placed_id"
	//      into the SAME SET clause (an UpdateExpression may use SET only once).
	//      Otherwise append " REMOVE #placed_id" so the order drops out of the
	//      sparse placed-index.
	//   4. UpdateItem with Key pk="#USER#<UserID>", sk="#ORDER#<orderID>".
	// (Add the "time" import: time.Now().Format("2006-01-02") for status_date
	// and time.RFC3339 for updated_at are handy here.)
	return errNotImplemented("UpdateOrderStatus")
}

// ShipOrder should mark an order shipped only if it is currently confirmed.
func (r *Repository) ShipOrder(ctx context.Context, orderID string) error {
	// TODO(lab): Look up the order (r.GetOrderByID) for its UserID, then
	// UpdateItem with:
	//   UpdateExpression    "SET #status = :new_status, #status_date = :status_date REMOVE #placed_id"
	//   ConditionExpression "#status = :expected_status"   (:expected_status = "confirmed")
	// so the write is rejected unless the order is confirmed. Alias status,
	// status_date, and placed_id via ExpressionAttributeNames. A rejection
	// surfaces as *types.ConditionalCheckFailedException (handle with errors.As).
	return errNotImplemented("ShipOrder")
}

// ---------- Delete operations ----------

func (r *Repository) DeleteOrderItem(ctx context.Context, orderID, itemID string) error {
	// TODO(lab): Delete a single order item with DeleteItem, keyed by
	//   pk = "#ORDER#<orderID>"
	//   sk = "#ITEM#<itemID>"
	// DeleteItem is idempotent — deleting a missing item is not an error.
	return errNotImplemented("DeleteOrderItem")
}

// CancelOrder should delete an order only while it is still pending.
func (r *Repository) CancelOrder(ctx context.Context, orderID string) error {
	// TODO(lab): Look up the order (r.GetOrderByID) for its UserID, then
	// DeleteItem keyed by pk="#USER#<UserID>", sk="#ORDER#<orderID>" with a
	// ConditionExpression "#status = :expected" (:expected = OrderStatusPending)
	// so only pending orders can be cancelled. Set ReturnValues =
	// types.ReturnValueAllOld to get the deleted attributes back.
	return errNotImplemented("CancelOrder")
}

// DeleteOrderWithItems should delete an order and all of its items. DynamoDB
// has no cascade delete, so you must remove the related items yourself.
func (r *Repository) DeleteOrderWithItems(ctx context.Context, orderID string) error {
	// TODO(lab): First r.GetOrderItems(orderID) and r.DeleteOrderItem for each
	// one, then r.GetOrderByID to learn the UserID and DeleteItem the order
	// itself (pk="#USER#<UserID>", sk="#ORDER#<orderID>"). Note this is NOT
	// atomic — a crash mid-loop leaves partial state; the transactions module
	// shows the atomic alternative.
	return errNotImplemented("DeleteOrderWithItems")
}

// ---------- Transactions ----------

func (r *Repository) PlaceOrder(ctx context.Context, order *Order, items []OrderItem) error {
	// TODO(lab): Place an order atomically with TransactWriteItems — either
	// everything below succeeds or nothing is written. Build a
	// []types.TransactWriteItem containing:
	//   1. A ConditionCheck that the user's PROFILE exists
	//      (Key pk="#USER#<UserID>", sk="PROFILE",
	//       ConditionExpression "attribute_exists(pk)").
	//   2. A Put for the order item map (pk="#USER#<UserID>", sk="#ORDER#<ID>",
	//      plus order_id, user_id, status, status_date, placed_id, address_key,
	//      created_at, updated_at).
	//   3. A Put for each element of `items`
	//      (pk="#ORDER#<ID>", sk="#ITEM#<ItemID>", plus order_id, item_id, name,
	//       price as N, quantity as N).
	// Then call r.client.TransactWriteItems with those TransactItems.
	return errNotImplemented("PlaceOrder")
}

// GetOrderSnapshot should read an order and all its items as one consistent
// snapshot using TransactGetItems.
func (r *Repository) GetOrderSnapshot(ctx context.Context, userID, orderID string) (*Order, []OrderItem, error) {
	// TODO(lab): First r.GetOrderItems(orderID) so you know which item keys to
	// read. Build a []types.TransactGetItem whose first Get is the order
	// (pk="#USER#<userID>", sk="#ORDER#<orderID>") followed by one Get per item
	// (pk="#ORDER#<orderID>", sk="#ITEM#<ItemID>"). Call
	// r.client.TransactGetItems; responses come back in request order, so
	// result.Responses[0] is the order and the rest are items. Unmarshal each
	// (stamp order.UserID/order.ID) and return them.
	return nil, nil, errNotImplemented("GetOrderSnapshot")
}

// ---------- Helpers ----------

// unmarshalOrders is provided for you. Base-table and index queries that return
// orders share this decoding: it unmarshals each item, stamps the userID, and
// recovers the order ID from the sort key ("#ORDER#<id>", so index 7 onward).
func unmarshalOrders(items []map[string]types.AttributeValue, userID string) []*Order {
	var orders []*Order
	for _, item := range items {
		var order Order
		if err := attributevalue.UnmarshalMap(item, &order); err != nil {
			continue
		}
		order.UserID = userID
		if skValue, ok := item["sk"]; ok {
			if skStr, ok := skValue.(*types.AttributeValueMemberS); ok {
				if len(skStr.Value) > 7 {
					order.ID = skStr.Value[7:]
				}
			}
		}
		orders = append(orders, &order)
	}
	return orders
}
