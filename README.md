# DynamoDB for Go Developers — Complete Solution

This is the complete, runnable reference solution for the **LGOD: DynamoDB for Go Developers** workshop. It demonstrates every DynamoDB access pattern the workshop teaches, using the AWS SDK for Go v2.

This code is supplementary material. In the workshop, you build these files step by step; here they are provided complete so you can run the finished application, compare against your own work, or use it as a reference.

## What it demonstrates

| DynamoDB operation | Where |
|--------------------|-------|
| `PutItem` | `repository.go` — `CreateUser`, `CreateOrder`, `CreateOrderItem` |
| `BatchWriteItem` | `repository.go` — `BatchWriteItems` |
| `GetItem` | `repository.go` — `GetUser` |
| `Query` (primary table) | `GetOrdersByUserID`, `GetOrderItems` |
| `Query` (inverted-index GSI) | `GetOrderByID` |
| `Query` (placed-index sparse GSI) | `GetPendingOrders` |
| `Query` (status-date-index LSI) | `GetUserOrdersByStatus` |
| `Query` (status-date-gsi multi-attribute GSI) | `GetUserOrdersByStatusGSI` |
| `UpdateItem` | `UpdateOrderStatus` |
| `DeleteItem` | `DeleteOrderItem` |
| `TransactWriteItems` | `PlaceOrder` |
| `Scan` | `ScanAllItems` |

## Prerequisites

- Go 1.21 or later
- AWS credentials configured (`aws configure` or environment variables)
- Permissions to create a CloudFormation stack and use DynamoDB

## 1. Provision the table (control plane)

The table and its indexes are defined in `template.yaml` and deployed with CloudFormation — not from application code. This mirrors production, where infrastructure is managed as code and the SDK is used only for data operations.

```bash
aws cloudformation deploy \
  --template-file template.yaml \
  --stack-name dynamodb-for-go-developers
```

Verify the table is active:

```bash
aws dynamodb describe-table --table-name simple-inventory --query "Table.TableStatus"
```

## 2. Load the sample data

```bash
go run . load-data
```

This bulk-loads three users, six orders (in various states), and six order items using `BatchWriteItem`.

## 3. Run the demo

```bash
go run .
```

This exercises every read, query, update, transaction, and scan pattern against the loaded data and prints the results.

## Configuration

Both settings default sensibly and can be overridden with environment variables:

| Variable | Default | Purpose |
|----------|---------|---------|
| `AWS_REGION` | `us-east-1` | Region for the DynamoDB client |
| `DYNAMODB_TABLE_NAME` | `simple-inventory` | Table name (must match the stack) |

## 4. Clean up

Because CloudFormation owns the table's lifecycle, tear it down by deleting the stack:

```bash
aws cloudformation delete-stack --stack-name dynamodb-for-go-developers
aws cloudformation wait stack-delete-complete --stack-name dynamodb-for-go-developers
```

## Project layout

```
.
├── template.yaml    # CloudFormation: table + GSIs + LSI (control plane)
├── models.go        # Entity structs (User, Order, OrderItem)
├── repository.go    # All DynamoDB data-plane operations
├── main.go          # CLI entry point (load-data, demo)
├── go.mod
└── go.sum
```

## The data model

Single table design with prefixed composite keys:

| Entity | pk | sk |
|--------|----|----|
| User | `#USER#<username>` | `PROFILE` |
| Order | `#USER#<username>` | `#ORDER#<order-id>` |
| Order Item | `#ORDER#<order-id>` | `#ITEM#<item-id>` |

Indexes:
- **inverted-index** (GSI): `sk` → `pk` — find an order by ID across all users
- **placed-index** (GSI, sparse): `placed_id` — active (pending/confirmed) orders only
- **status-date-index** (LSI): `pk`, `status_date` — a user's orders by status/date using a concatenated key
- **status-date-gsi** (GSI, multi-attribute): `pk`, `status`, `created_at` — the same access pattern using multi-attribute keys instead of a concatenated string
