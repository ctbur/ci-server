package main

import (
	"context"
	"fmt"
	"os"
	"time"

	pgx "github.com/jackc/pgx/v5"
)

func main() {
	conn, err := pgx.Connect(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(context.Background())

	for true {
		fmt.Println("server")
		time.Sleep(time.Second)
	}
}
