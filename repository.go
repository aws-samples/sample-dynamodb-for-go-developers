package main

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

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
	item, err := attributevalue.MarshalMap(order)
	if err != nil {
		return nil, err
	}
	item["pk"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("#USER#%s", order.UserID)}
	item["sk"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("#ORDER#%s", order.ID)}
	item["status_date"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s", order.Status, order.CreatedAt.Format("2006-01-02"))}

	// placed_id is only present for active orders, which is what makes the placed-index sparse.
	if order.Status == OrderStatusPending || order.Status == OrderStatusConfirmed {
		item["placed_id"] = &types.AttributeValueMemberS{Value: string(order.Status)}
	}
	return item, nil
}

func marshalOrderItem(orderID string, orderItem OrderItem) (map[string]types.AttributeValue, error) {
	item, err := attributevalue.MarshalMap(orderItem)
	if err != nil {
		return nil, err
	}
	item["pk"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("#ORDER#%s", orderID)}
	item["sk"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("#ITEM#%s", orderItem.ItemID)}
	return item, nil
}

// ---------- Write operations ----------

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

// CreateUserIfNotExists writes a user only when no profile already exists for
// that username, using a condition expression to prevent silent overwrites.
func (r *Repository) CreateUserIfNotExists(ctx context.Context, user User) error {
	item, err := marshalUser(user)
	if err != nil {
		return err
	}
	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(pk)"),
	})
	return err
}

func (r *Repository) CreateOrder(ctx context.Context, order *Order) error {
	item, err := marshalOrder(*order)
	if err != nil {
		return err
	}
	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})
	return err
}

func (r *Repository) CreateOrderItem(ctx context.Context, orderID string, orderItem *OrderItem) error {
	item, err := marshalOrderItem(orderID, *orderItem)
	if err != nil {
		return err
	}
	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})
	return err
}

func (r *Repository) BatchWriteItems(ctx context.Context, items []map[string]types.AttributeValue) error {
	for i := 0; i < len(items); i += 25 {
		end := i + 25
		if end > len(items) {
			end = len(items)
		}

		batch := items[i:end]
		var writeRequests []types.WriteRequest
		for _, item := range batch {
			writeRequests = append(writeRequests, types.WriteRequest{
				PutRequest: &types.PutRequest{Item: item},
			})
		}

		output, err := r.client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				r.tableName: writeRequests,
			},
		})
		if err != nil {
			return err
		}

		for len(output.UnprocessedItems) > 0 {
			output, err = r.client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
				RequestItems: output.UnprocessedItems,
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// SeedData marshals typed model objects and bulk-loads them with BatchWriteItems.
// It uses the same marshaling helpers as CreateUser/CreateOrder/CreateOrderItem,
// so the sample dataset is built from models rather than hand-written attribute maps.
func (r *Repository) SeedData(ctx context.Context, users []User, orders []Order, orderItems []OrderItem) error {
	var items []map[string]types.AttributeValue

	for _, u := range users {
		item, err := marshalUser(u)
		if err != nil {
			return err
		}
		items = append(items, item)
	}
	for _, o := range orders {
		item, err := marshalOrder(o)
		if err != nil {
			return err
		}
		items = append(items, item)
	}
	for _, oi := range orderItems {
		item, err := marshalOrderItem(oi.OrderID, oi)
		if err != nil {
			return err
		}
		items = append(items, item)
	}

	return r.BatchWriteItems(ctx, items)
}

// ---------- Read operations ----------

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

// GetAllOrdersPaginated walks every page of a user's orders by threading the
// LastEvaluatedKey from one Query into the ExclusiveStartKey of the next.
func (r *Repository) GetAllOrdersPaginated(ctx context.Context, userID string, pageSize int32) ([]*Order, error) {
	var allOrders []*Order
	var lastKey map[string]types.AttributeValue

	for {
		input := &dynamodb.QueryInput{
			TableName:              aws.String(r.tableName),
			KeyConditionExpression: aws.String("pk = :pk AND begins_with(sk, :sk_prefix)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":        &types.AttributeValueMemberS{Value: fmt.Sprintf("#USER#%s", userID)},
				":sk_prefix": &types.AttributeValueMemberS{Value: "#ORDER#"},
			},
			Limit: aws.Int32(pageSize),
		}
		if lastKey != nil {
			input.ExclusiveStartKey = lastKey
		}

		result, err := r.client.Query(ctx, input)
		if err != nil {
			return nil, err
		}

		allOrders = append(allOrders, unmarshalOrders(result.Items, userID)...)

		lastKey = result.LastEvaluatedKey
		if lastKey == nil {
			break
		}
	}

	return allOrders, nil
}

func (r *Repository) GetOrderItems(ctx context.Context, orderID string) ([]OrderItem, error) {
	result, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("pk = :pk AND begins_with(sk, :sk_prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":        &types.AttributeValueMemberS{Value: fmt.Sprintf("#ORDER#%s", orderID)},
			":sk_prefix": &types.AttributeValueMemberS{Value: "#ITEM#"},
		},
	})
	if err != nil {
		return nil, err
	}

	var items []OrderItem
	for _, item := range result.Items {
		var orderItem OrderItem
		if err := attributevalue.UnmarshalMap(item, &orderItem); err != nil {
			continue
		}
		items = append(items, orderItem)
	}
	return items, nil
}

// GetOrderByID uses the inverted-index GSI to find an order without knowing its user.
func (r *Repository) GetOrderByID(ctx context.Context, orderID string) (*Order, error) {
	result, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("inverted-index"),
		KeyConditionExpression: aws.String("sk = :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("#ORDER#%s", orderID)},
		},
		Limit: aws.Int32(1),
	})
	if err != nil {
		return nil, err
	}
	if len(result.Items) == 0 {
		return nil, fmt.Errorf("order not found: %s", orderID)
	}

	var order Order
	if err := attributevalue.UnmarshalMap(result.Items[0], &order); err != nil {
		return nil, err
	}
	if pkValue, ok := result.Items[0]["pk"]; ok {
		if pkStr, ok := pkValue.(*types.AttributeValueMemberS); ok {
			order.UserID = pkStr.Value[6:]
		}
	}
	order.ID = orderID
	return &order, nil
}

// GetPendingOrders uses the sparse placed-index GSI.
func (r *Repository) GetPendingOrders(ctx context.Context) ([]*Order, error) {
	result, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("placed-index"),
		KeyConditionExpression: aws.String("placed_id = :placed_id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":placed_id": &types.AttributeValueMemberS{Value: string(OrderStatusPending)},
		},
	})
	if err != nil {
		return nil, err
	}

	var orders []*Order
	for _, item := range result.Items {
		var order Order
		if err := attributevalue.UnmarshalMap(item, &order); err != nil {
			continue
		}
		orders = append(orders, &order)
	}
	return orders, nil
}

// GetUserOrdersByStatus uses the status-date-index LSI (concatenated status_date key).
func (r *Repository) GetUserOrdersByStatus(ctx context.Context, userID string, status OrderStatus) ([]*Order, error) {
	result, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("status-date-index"),
		KeyConditionExpression: aws.String("pk = :pk AND begins_with(status_date, :status_prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":            &types.AttributeValueMemberS{Value: fmt.Sprintf("#USER#%s", userID)},
			":status_prefix": &types.AttributeValueMemberS{Value: string(status) + "#"},
		},
	})
	if err != nil {
		return nil, err
	}

	return unmarshalOrders(result.Items, userID), nil
}

// GetUserOrdersByStatusGSI uses the multi-attribute status-date-gsi (status + created_at
// as two separate sort key attributes — no concatenation).
func (r *Repository) GetUserOrdersByStatusGSI(ctx context.Context, userID string, status OrderStatus, since string) ([]*Order, error) {
	result, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("status-date-gsi"),
		KeyConditionExpression: aws.String("pk = :pk AND #status = :status AND created_at > :since"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: fmt.Sprintf("#USER#%s", userID)},
			":status": &types.AttributeValueMemberS{Value: string(status)},
			":since":  &types.AttributeValueMemberS{Value: since},
		},
	})
	if err != nil {
		return nil, err
	}

	return unmarshalOrders(result.Items, userID), nil
}

func (r *Repository) ScanAllItems(ctx context.Context) ([]map[string]types.AttributeValue, error) {
	var allItems []map[string]types.AttributeValue

	paginator := dynamodb.NewScanPaginator(r.client, &dynamodb.ScanInput{
		TableName: aws.String(r.tableName),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		allItems = append(allItems, page.Items...)
	}
	return allItems, nil
}

// ScanOrdersByStatus scans the whole table and applies a filter expression.
// The filter reduces what is returned to the client, but DynamoDB still reads
// (and charges for) every item scanned — prefer an index for real workloads.
func (r *Repository) ScanOrdersByStatus(ctx context.Context, status OrderStatus) ([]map[string]types.AttributeValue, error) {
	var allItems []map[string]types.AttributeValue

	paginator := dynamodb.NewScanPaginator(r.client, &dynamodb.ScanInput{
		TableName:        aws.String(r.tableName),
		FilterExpression: aws.String("#status = :status AND begins_with(sk, :order_prefix)"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status":       &types.AttributeValueMemberS{Value: string(status)},
			":order_prefix": &types.AttributeValueMemberS{Value: "#ORDER#"},
		},
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		allItems = append(allItems, page.Items...)
	}
	return allItems, nil
}

// ParallelScan splits a scan across totalSegments goroutines. DynamoDB divides
// the table's key space evenly across segments so each worker reads a distinct
// portion concurrently.
func (r *Repository) ParallelScan(ctx context.Context, totalSegments int) ([]map[string]types.AttributeValue, error) {
	type segmentResult struct {
		items []map[string]types.AttributeValue
		err   error
	}

	results := make(chan segmentResult, totalSegments)

	for segment := 0; segment < totalSegments; segment++ {
		go func(seg int) {
			var items []map[string]types.AttributeValue

			paginator := dynamodb.NewScanPaginator(r.client, &dynamodb.ScanInput{
				TableName:     aws.String(r.tableName),
				Segment:       aws.Int32(int32(seg)),
				TotalSegments: aws.Int32(int32(totalSegments)),
			})

			for paginator.HasMorePages() {
				page, err := paginator.NextPage(ctx)
				if err != nil {
					results <- segmentResult{err: err}
					return
				}
				items = append(items, page.Items...)
			}

			results <- segmentResult{items: items}
		}(segment)
	}

	var allItems []map[string]types.AttributeValue
	for i := 0; i < totalSegments; i++ {
		result := <-results
		if result.err != nil {
			return nil, result.err
		}
		allItems = append(allItems, result.items...)
	}
	return allItems, nil
}

// ---------- Update operations ----------

func (r *Repository) UpdateOrderStatus(ctx context.Context, orderID string, newStatus OrderStatus) error {
	order, err := r.GetOrderByID(ctx, orderID)
	if err != nil {
		return err
	}

	statusDate := fmt.Sprintf("%s#%s", newStatus, time.Now().Format("2006-01-02"))

	// An UpdateExpression may use each keyword (SET/REMOVE) only once, so the
	// placed_id change is folded into the same SET or REMOVE clause rather than
	// appended as a second SET.
	setExpr := "SET #status = :status, #status_date = :status_date, #updated_at = :updated_at"
	exprNames := map[string]string{
		"#status":      "status",
		"#status_date": "status_date",
		"#updated_at":  "updated_at",
	}
	exprValues := map[string]types.AttributeValue{
		":status":      &types.AttributeValueMemberS{Value: string(newStatus)},
		":status_date": &types.AttributeValueMemberS{Value: statusDate},
		":updated_at":  &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	var updateExpr string
	if newStatus == OrderStatusPending || newStatus == OrderStatusConfirmed {
		// Active order: set placed_id so it appears in the sparse placed-index.
		setExpr += ", #placed_id = :placed_id"
		exprNames["#placed_id"] = "placed_id"
		exprValues[":placed_id"] = &types.AttributeValueMemberS{Value: string(newStatus)}
		updateExpr = setExpr
	} else {
		// Inactive order: drop placed_id so it falls out of the sparse index.
		exprNames["#placed_id"] = "placed_id"
		updateExpr = setExpr + " REMOVE #placed_id"
	}

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("#USER#%s", order.UserID)},
			"sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("#ORDER#%s", orderID)},
		},
		UpdateExpression:          aws.String(updateExpr),
		ExpressionAttributeNames:  exprNames,
		ExpressionAttributeValues: exprValues,
	})
	return err
}

// ShipOrder marks an order shipped only if it is currently confirmed, using a
// condition expression for optimistic locking. If the order is in any other
// state the write is rejected with a ConditionalCheckFailedException.
func (r *Repository) ShipOrder(ctx context.Context, orderID string) error {
	order, err := r.GetOrderByID(ctx, orderID)
	if err != nil {
		return err
	}

	statusDate := fmt.Sprintf("shipped#%s", time.Now().Format("2006-01-02"))

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("#USER#%s", order.UserID)},
			"sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("#ORDER#%s", orderID)},
		},
		UpdateExpression:    aws.String("SET #status = :new_status, #status_date = :status_date REMOVE #placed_id"),
		ConditionExpression: aws.String("#status = :expected_status"),
		ExpressionAttributeNames: map[string]string{
			"#status":      "status",
			"#status_date": "status_date",
			"#placed_id":   "placed_id",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":new_status":      &types.AttributeValueMemberS{Value: "shipped"},
			":expected_status": &types.AttributeValueMemberS{Value: "confirmed"},
			":status_date":     &types.AttributeValueMemberS{Value: statusDate},
		},
	})
	return err
}

// ---------- Delete operations ----------

func (r *Repository) DeleteOrderItem(ctx context.Context, orderID, itemID string) error {
	_, err := r.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("#ORDER#%s", orderID)},
			"sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("#ITEM#%s", itemID)},
		},
	})
	return err
}

// CancelOrder deletes an order only while it is still pending, guarding the
// delete with a condition expression. ReturnValues=AllOld hands back the
// deleted item's attributes for logging or confirmation.
func (r *Repository) CancelOrder(ctx context.Context, orderID string) error {
	order, err := r.GetOrderByID(ctx, orderID)
	if err != nil {
		return err
	}

	_, err = r.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("#USER#%s", order.UserID)},
			"sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("#ORDER#%s", orderID)},
		},
		ConditionExpression: aws.String("#status = :expected"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":expected": &types.AttributeValueMemberS{Value: string(OrderStatusPending)},
		},
		ReturnValues: types.ReturnValueAllOld,
	})
	return err
}

// DeleteOrderWithItems removes an order and all of its items. DynamoDB has no
// cascade delete, so this queries the items and deletes each explicitly before
// deleting the order. Note this is NOT atomic — a crash mid-way leaves partial
// state; the transactions module shows how to make multi-item writes atomic.
func (r *Repository) DeleteOrderWithItems(ctx context.Context, orderID string) error {
	items, err := r.GetOrderItems(ctx, orderID)
	if err != nil {
		return err
	}

	for _, item := range items {
		if err := r.DeleteOrderItem(ctx, orderID, item.ItemID); err != nil {
			return fmt.Errorf("failed to delete item %s: %w", item.ItemID, err)
		}
	}

	order, err := r.GetOrderByID(ctx, orderID)
	if err != nil {
		return err
	}

	_, err = r.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("#USER#%s", order.UserID)},
			"sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("#ORDER#%s", orderID)},
		},
	})
	return err
}

// ---------- Transactions ----------

func (r *Repository) PlaceOrder(ctx context.Context, order *Order, items []OrderItem) error {
	var transactItems []types.TransactWriteItem

	transactItems = append(transactItems, types.TransactWriteItem{
		ConditionCheck: &types.ConditionCheck{
			TableName: aws.String(r.tableName),
			Key: map[string]types.AttributeValue{
				"pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("#USER#%s", order.UserID)},
				"sk": &types.AttributeValueMemberS{Value: "PROFILE"},
			},
			ConditionExpression: aws.String("attribute_exists(pk)"),
		},
	})

	statusDate := fmt.Sprintf("%s#%s", order.Status, order.CreatedAt.Format("2006-01-02"))
	orderItem := map[string]types.AttributeValue{
		"pk":          &types.AttributeValueMemberS{Value: fmt.Sprintf("#USER#%s", order.UserID)},
		"sk":          &types.AttributeValueMemberS{Value: fmt.Sprintf("#ORDER#%s", order.ID)},
		"order_id":    &types.AttributeValueMemberS{Value: order.ID},
		"user_id":     &types.AttributeValueMemberS{Value: order.UserID},
		"status":      &types.AttributeValueMemberS{Value: string(order.Status)},
		"status_date": &types.AttributeValueMemberS{Value: statusDate},
		"placed_id":   &types.AttributeValueMemberS{Value: string(order.Status)},
		"address_key": &types.AttributeValueMemberS{Value: order.AddressKey},
		"created_at":  &types.AttributeValueMemberS{Value: order.CreatedAt.Format(time.RFC3339)},
		"updated_at":  &types.AttributeValueMemberS{Value: order.UpdatedAt.Format(time.RFC3339)},
	}
	transactItems = append(transactItems, types.TransactWriteItem{
		Put: &types.Put{TableName: aws.String(r.tableName), Item: orderItem},
	})

	for _, item := range items {
		transactItems = append(transactItems, types.TransactWriteItem{
			Put: &types.Put{
				TableName: aws.String(r.tableName),
				Item: map[string]types.AttributeValue{
					"pk":       &types.AttributeValueMemberS{Value: fmt.Sprintf("#ORDER#%s", order.ID)},
					"sk":       &types.AttributeValueMemberS{Value: fmt.Sprintf("#ITEM#%s", item.ItemID)},
					"order_id": &types.AttributeValueMemberS{Value: order.ID},
					"item_id":  &types.AttributeValueMemberS{Value: item.ItemID},
					"name":     &types.AttributeValueMemberS{Value: item.Name},
					"price":    &types.AttributeValueMemberN{Value: fmt.Sprintf("%.2f", item.Price)},
					"quantity": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", item.Quantity)},
				},
			},
		})
	}

	_, err := r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: transactItems,
	})
	return err
}

// GetOrderSnapshot reads an order and all its items as a consistent point-in-time
// snapshot using TransactGetItems. TransactGetItems returns responses in the same
// order as the request, so the first response is the order and the rest are items.
func (r *Repository) GetOrderSnapshot(ctx context.Context, userID, orderID string) (*Order, []OrderItem, error) {
	// The item IDs must be known up front to build the Get requests.
	orderItems, err := r.GetOrderItems(ctx, orderID)
	if err != nil {
		return nil, nil, err
	}

	var transactItems []types.TransactGetItem

	transactItems = append(transactItems, types.TransactGetItem{
		Get: &types.Get{
			TableName: aws.String(r.tableName),
			Key: map[string]types.AttributeValue{
				"pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("#USER#%s", userID)},
				"sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("#ORDER#%s", orderID)},
			},
		},
	})

	for _, item := range orderItems {
		transactItems = append(transactItems, types.TransactGetItem{
			Get: &types.Get{
				TableName: aws.String(r.tableName),
				Key: map[string]types.AttributeValue{
					"pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("#ORDER#%s", orderID)},
					"sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("#ITEM#%s", item.ItemID)},
				},
			},
		})
	}

	result, err := r.client.TransactGetItems(ctx, &dynamodb.TransactGetItemsInput{
		TransactItems: transactItems,
	})
	if err != nil {
		return nil, nil, err
	}

	var order Order
	if len(result.Responses) > 0 && result.Responses[0].Item != nil {
		if err := attributevalue.UnmarshalMap(result.Responses[0].Item, &order); err != nil {
			return nil, nil, err
		}
		order.UserID = userID
		order.ID = orderID
	}

	var items []OrderItem
	for _, resp := range result.Responses[1:] {
		if resp.Item != nil {
			var item OrderItem
			if err := attributevalue.UnmarshalMap(resp.Item, &item); err != nil {
				continue
			}
			items = append(items, item)
		}
	}

	return &order, items, nil
}

// ---------- Helpers ----------

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
