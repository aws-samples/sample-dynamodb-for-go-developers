# DynamoDB for Go Developers — Lab Branch

> **This is the `lab` branch: a fill-in-the-blanks worksheet.**
> `repository.go` ships with the DynamoDB data-plane operations left as
> `TODO(lab)` stubs for you to implement as you work through the
> **LGOD: DynamoDB for Go Developers** workshop instructions. The complete,
> runnable reference solution lives on the **`main`** branch
> (`git switch main`).

## How the lab works

1. **Pull this branch.** Everything except the DynamoDB operations in
   `repository.go` is provided and working: the models, the CLI entry point,
   the demo/seed harness, the CloudFormation template, and the Taskfile.
2. **Fill in the sections.** Each function to implement is marked with a
   `// TODO(lab):` comment describing exactly what to do and which worked
   example to mirror. Follow the workshop instructions module by module.
3. **Watch the placeholders clear.** Every unimplemented function returns an
   `errNotImplemented("<name>")` error, so the project compiles and runs from
   the first checkout. `go run . demo` prints exactly which patterns are still
   unimplemented — a live progress checklist. Delete the `errNotImplemented`
   return as you complete each function.

### Worked examples vs. what you implement

`repository.go` keeps one **worked example per concept** so you always have a
pattern to follow. The rest are yours to write:

| Concept | Worked example (provided) | You implement (`TODO(lab)`) |
|---------|---------------------------|------------------------------|
| `PutItem` + marshaling | `marshalUser`, `CreateUser` | `marshalOrder`, `marshalOrderItem`, `CreateOrder`, `CreateOrderItem` |
| `PutItem` (conditional) | — | `CreateUserIfNotExists` |
| `BatchWriteItem` (bulk load) | `SeedData` | `BatchWriteItems` |
| `GetItem` | `GetUser` | — |
| `Query` (base table) | `GetOrdersByUserID` | `GetOrderItems` |
| `Query` (paginated) | — | `GetAllOrdersPaginated` |
| `Query` (inverted-index GSI) | — | `GetOrderByID` |
| `Query` (placed-index sparse GSI) | — | `GetPendingOrders` |
| `Query` (status-date-index LSI) | — | `GetUserOrdersByStatus` |
| `Query` (status-date-gsi multi-attribute GSI) | — | `GetUserOrdersByStatusGSI` |
| `Scan` | — | `ScanAllItems` |
| `Scan` (filtered) | — | `ScanOrdersByStatus` |
| `Scan` (parallel) | — | `ParallelScan` |
| `UpdateItem` | `UpdateOrderStatus` | — |
| `UpdateItem` (conditional) | — | `ShipOrder` |
| `DeleteItem` | `DeleteOrderItem` | — |
| `DeleteItem` (conditional) | — | `CancelOrder` |
| `DeleteItem` (cascade) | — | `DeleteOrderWithItems` |
| `TransactWriteItems` | `PlaceOrder` | — |
| `TransactGetItems` | — | `GetOrderSnapshot` |

> **Stuck?** Compare against the reference: `git show main:repository.go`
> (or `git switch main` to browse the whole solution, then `git switch lab`).

## What the finished solution demonstrates

| DynamoDB operation | Where |
|--------------------|-------|
| `PutItem` | `repository.go` — `CreateUser`, `CreateOrder`, `CreateOrderItem` |
| `PutItem` (conditional) | `CreateUserIfNotExists` |
| `BatchWriteItem` | `repository.go` — `BatchWriteItems`, `SeedData` |
| `GetItem` | `repository.go` — `GetUser` |
| `Query` (primary table) | `GetOrdersByUserID`, `GetOrderItems` |
| `Query` (paginated) | `GetAllOrdersPaginated` |
| `Query` (inverted-index GSI) | `GetOrderByID` |
| `Query` (placed-index sparse GSI) | `GetPendingOrders` |
| `Query` (status-date-index LSI) | `GetUserOrdersByStatus` |
| `Query` (status-date-gsi multi-attribute GSI) | `GetUserOrdersByStatusGSI` |
| `UpdateItem` | `UpdateOrderStatus` |
| `UpdateItem` (conditional) | `ShipOrder` |
| `DeleteItem` | `DeleteOrderItem` |
| `DeleteItem` (conditional) | `CancelOrder` |
| `DeleteItem` (cascade) | `DeleteOrderWithItems` |
| `TransactWriteItems` | `PlaceOrder` |
| `TransactGetItems` | `GetOrderSnapshot` |
| `Scan` | `ScanAllItems` |
| `Scan` (filtered) | `ScanOrdersByStatus` |
| `Scan` (parallel) | `ParallelScan` |

All write paths marshal the model structs (`User`, `Order`, `OrderItem`) with `attributevalue.MarshalMap` via shared helpers in `repository.go`, then add the single-table `pk`/`sk` and derived index attributes — so no hand-written attribute maps are needed anywhere.

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

This builds three users, six orders (in various states), and six order items as typed model objects and bulk-loads them with `BatchWriteItem` (see `SeedData` in `repository.go`).

> On the `lab` branch this fails with a `TODO(lab)` error until you implement
> `marshalOrder`, `marshalOrderItem`, and `BatchWriteItems`. That is expected —
> the message tells you which function to fill in next. (`SeedData` is already
> provided; it just marshals the models and calls `BatchWriteItems`.)

## 3. Run the demo

```bash
go run .
```

This exercises every read, query, update, transaction, and scan pattern against the loaded data and prints the results.

> On the `lab` branch the demo stops at the first unimplemented function. Work
> through the workshop modules in order; each function you complete lets the
> demo progress one step further, so the demo doubles as your progress
> checklist.

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
├── repository.go    # DynamoDB data-plane operations — worked examples + TODO(lab) stubs
├── demo.go          # Sample dataset (typed models) and the demo walkthrough
├── main.go          # CLI entry point: config, client, subcommand dispatch
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

## Security

See [CONTRIBUTING](CONTRIBUTING.md#security-issue-notifications) for more information.

## License

This library is licensed under the MIT-0 License. See the [LICENSE](LICENSE) file.
