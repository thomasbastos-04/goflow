package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thomasbastos-04/goflow/internal/domain"
)

var ErrNotFound = errors.New("not found")

type Repository struct {
	DB *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Repository { return &Repository{DB: db} }

func (r *Repository) CreateUser(ctx context.Context, name, email, hash string) (domain.User, error) {
	u := domain.User{ID: uuid.NewString(), Name: name, Email: email}
	err := r.DB.QueryRow(ctx, `
		INSERT INTO users(id,name,email,password_hash)
		VALUES($1,$2,$3,$4)
		RETURNING created_at`, u.ID, u.Name, u.Email, hash).Scan(&u.CreatedAt)
	return u, err
}

func (r *Repository) UserCredentialsByEmail(ctx context.Context, email string) (domain.User, string, error) {
	var u domain.User
	var hash string
	err := r.DB.QueryRow(ctx, `
		SELECT id,name,email,password_hash,created_at FROM users WHERE email=$1`, email).
		Scan(&u.ID, &u.Name, &u.Email, &hash, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, "", ErrNotFound
	}
	return u, hash, err
}

func (r *Repository) CreateProduct(ctx context.Context, name, sku string, price int64, stock int) (domain.Product, error) {
	p := domain.Product{ID: uuid.NewString(), Name: name, SKU: sku, PriceCents: price, Stock: stock}
	err := r.DB.QueryRow(ctx, `
		INSERT INTO products(id,name,sku,price_cents,stock)
		VALUES($1,$2,$3,$4,$5)
		RETURNING created_at`, p.ID, p.Name, p.SKU, p.PriceCents, p.Stock).Scan(&p.CreatedAt)
	return p, err
}

func (r *Repository) ListProducts(ctx context.Context) ([]domain.Product, error) {
	rows, err := r.DB.Query(ctx, `SELECT id,name,sku,price_cents,stock,created_at FROM products ORDER BY created_at DESC`)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []domain.Product
	for rows.Next() {
		var p domain.Product
		if err := rows.Scan(&p.ID,&p.Name,&p.SKU,&p.PriceCents,&p.Stock,&p.CreatedAt); err != nil { return nil, err }
		out = append(out, p)
	}
	return out, rows.Err()
}

type NewOrderItem struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

func (r *Repository) CreateOrder(ctx context.Context, userID string, items []NewOrderItem) (domain.Order, error) {
	tx, err := r.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil { return domain.Order{}, err }
	defer tx.Rollback(ctx)

	order := domain.Order{ID: uuid.NewString(), UserID: userID, Status: domain.OrderPending}
	for _, in := range items {
		if in.Quantity <= 0 { return order, fmt.Errorf("quantity must be positive") }
		var price int64
		var stock int
		err = tx.QueryRow(ctx, `SELECT price_cents,stock FROM products WHERE id=$1 FOR UPDATE`, in.ProductID).Scan(&price,&stock)
		if errors.Is(err, pgx.ErrNoRows) { return order, fmt.Errorf("product %s: %w", in.ProductID, ErrNotFound) }
		if err != nil { return order, err }
		if stock < in.Quantity { return order, fmt.Errorf("insufficient stock for product %s", in.ProductID) }
		if _, err = tx.Exec(ctx, `UPDATE products SET stock=stock-$1 WHERE id=$2`, in.Quantity, in.ProductID); err != nil { return order, err }
		order.TotalCents += price * int64(in.Quantity)
		order.Items = append(order.Items, domain.OrderItem{ProductID: in.ProductID, Quantity: in.Quantity, UnitPriceCents: price})
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO orders(id,user_id,status,total_cents)
		VALUES($1,$2,$3,$4)
		RETURNING created_at,updated_at`, order.ID,order.UserID,order.Status,order.TotalCents).
		Scan(&order.CreatedAt,&order.UpdatedAt)
	if err != nil { return order, err }

	for _, item := range order.Items {
		_, err = tx.Exec(ctx, `
			INSERT INTO order_items(id,order_id,product_id,quantity,unit_price_cents)
			VALUES($1,$2,$3,$4,$5)`, uuid.NewString(), order.ID, item.ProductID, item.Quantity, item.UnitPriceCents)
		if err != nil { return order, err }
	}

	event := domain.OrderCreatedEvent{OrderID: order.ID, UserID: userID, TotalCents: order.TotalCents}
	payload, _ := json.Marshal(event)
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events(id,aggregate_id,event_type,payload)
		VALUES($1,$2,'order.created',$3)`, uuid.NewString(), order.ID, payload)
	if err != nil { return order, err }

	if err = tx.Commit(ctx); err != nil { return order, err }
	return order, nil
}

func (r *Repository) GetOrder(ctx context.Context, id, userID string) (domain.Order, error) {
	var o domain.Order
	err := r.DB.QueryRow(ctx, `
		SELECT id,user_id,status,total_cents,created_at,updated_at
		FROM orders WHERE id=$1 AND user_id=$2`, id,userID).
		Scan(&o.ID,&o.UserID,&o.Status,&o.TotalCents,&o.CreatedAt,&o.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) { return o, ErrNotFound }
	if err != nil { return o, err }

	rows, err := r.DB.Query(ctx, `SELECT product_id,quantity,unit_price_cents FROM order_items WHERE order_id=$1`, id)
	if err != nil { return o, err }
	defer rows.Close()
	for rows.Next() {
		var item domain.OrderItem
		if err := rows.Scan(&item.ProductID,&item.Quantity,&item.UnitPriceCents); err != nil { return o, err }
		o.Items = append(o.Items,item)
	}
	return o, rows.Err()
}

func (r *Repository) ListOrders(ctx context.Context, userID string) ([]domain.Order, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT id,user_id,status,total_cents,created_at,updated_at
		FROM orders WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []domain.Order
	for rows.Next() {
		var o domain.Order
		if err := rows.Scan(&o.ID,&o.UserID,&o.Status,&o.TotalCents,&o.CreatedAt,&o.UpdatedAt); err != nil { return nil, err }
		out = append(out,o)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateOrderStatus(ctx context.Context, id, status string) error {
	tag, err := r.DB.Exec(ctx, `UPDATE orders SET status=$1,updated_at=NOW() WHERE id=$2`, status,id)
	if err != nil { return err }
	if tag.RowsAffected() == 0 { return ErrNotFound }
	return nil
}
