package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

type User struct {
	ID        int64 `bun:",pk,autoincrement"`
	Name      string
	Email     string
	CreatedAt time.Time `bun:",nullzero,notnull"`
	// db 忽略次字段
	db *bun.DB `bun:"-" json:"-"`
}

type UserRepository interface {
	Create(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User, column string) error
	Delete(ctx context.Context, id int64) error
	FindByID(ctx context.Context, id int64) (*User, error)
	FindAll(ctx context.Context) ([]*User, error)
}

func (u *User) Create(ctx context.Context, user *User) error {
	_, err := u.db.NewInsert().Model(user).Exec(ctx)
	return err
}

func (u *User) Update(ctx context.Context, user *User, column string) error {
	_, err := u.db.NewUpdate().Model(user).Column(column).WherePK().Exec(ctx)
	return err
}

func main() {
	ctx := context.Background()
	dsn := "postgres://user:password@localhost:5432/land_contract?sslmode=disable"

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	db := bun.NewDB(sqldb, pgdialect.New())
	defer db.Close()

	user := &User{Name: "alice", Email: "alice@example.com", CreatedAt: time.Now()}

	// Insert
	if _, err := db.NewInsert().Model(user).Exec(ctx); err != nil {
		panic(err)
	}
	fmt.Println("inserted ID:", user.ID)

	// Update (显式)
	user.Email = "alice2@example.com"
	if _, err := db.NewUpdate().Model(user).Column("email").WherePK().Exec(ctx); err != nil {
		panic(err)
	}
	fmt.Println("updated")
}
