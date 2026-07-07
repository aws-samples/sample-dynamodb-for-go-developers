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

// ---------- Write operations ----------

func (r *Repository) CreateUser(ctx context.Context, user User) error {
	userMap, err := attributevalue.MarshalMap(user)
	if err != nil {
		return err
	}

	userMap["pk"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("#USER#%s", user.Username)}
	userMap["sk"] = &types.AttributeValueMemberS{Value: "PROFILE"}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      userMap,
	})
	return err
}

func (r *Repository) CreateOrder(ctx context.Context, order *Order) error {
	orderMap, err := attributevalue.MarshalMap(order)
	if err != nil {
		return err
	}

	orderMap["pk"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("#USER#%s", order.UserID)}
	orderMap["sk"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("#ORDER#%s", order.ID)}

	statusDate := fmt.Sprintf("%s#%s", order.Status, order.CreatedAt.Format("2006-01-02"))
	orderMap["status_date"] = &types.AttributeValueMemberS{Value: statusDate}

	if order.Status == OrderStatusPending || order.Status == OrderStatusConfirmed {
		orderMap["placed_id"] = &types.AttributeValueMemberS{Value: string(order.Status)}
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      orderMap,
	})
	return err
}

func (r *Repository) CreateOrderItem(ctx context.Context, orderID string, item *OrderItem) error {
	itemMap, err := attributevalue.MarshalMap(item)
	if err != nil {
		return err
	}

	itemMap["pk"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("#ORDER#%s", orderID)}
	itemMap["sk"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("#ITEM#%s", item.ItemID)}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      itemMap,
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

// ---------- Update operations ----------

func (r *Repository) UpdateOrderStatus(ctx context.Context, orderID string, newStatus OrderStatus) error {
	order, err := r.GetOrderByID(ctx, orderID)
	if err != nil {
		return err
	}

	statusDate := fmt.Sprintf("%s#%s", newStatus, time.Now().Format("2006-01-02"))

	updateExpr := "SET #status = :status, #status_date = :status_date, #updated_at = :updated_at"
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

	if newStatus == OrderStatusPending || newStatus == OrderStatusConfirmed {
		updateExpr += " SET #placed_id = :placed_id"
		exprNames["#placed_id"] = "placed_id"
		exprValues[":placed_id"] = &types.AttributeValueMemberS{Value: string(newStatus)}
	} else {
		updateExpr += " REMOVE #placed_id"
		exprNames["#placed_id"] = "placed_id"
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
