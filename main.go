package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jmoiron/sqlx"
)

const (
	createUsersQueue = "CREATE_USER"
	updateUsersQueue = "UPDATE_USER"
	deleteUsersQueue = "DELETE_USER"
)

func main() {

	var numWorkers int
	cache := Cache{Enable: true}
	flag.StringVar(&cache.Address, "redis_address", os.Getenv("APP_RD_ADDRESS"), "Redis Address") // address where Redis runs
	flag.StringVar(&cache.Auth, "redis_auth", os.Getenv("APP_RD_AUTH"), "Redis Auth")             // password used to connect to the Redis
	flag.StringVar(&cache.DB, "redis_db_name", os.Getenv("APP_RD_DBNAME"), "Redis DB name")       // Redis ban that will be used as cache
	flag.IntVar(&cache.MaxIdle, "redis_max_idle", 100, "Redis Max Idle")                          // maximum number of connections that can be idle
	flag.IntVar(&cache.MaxActive, "redis_max_active", 100, "Redis Max Active")                    // maximum number of connections that can be active
	flag.IntVar(&cache.IdleTimeoutSecs, "redis_timeout", 60, "Redis timeout in seconds")          //time a connection timeout leads to enter activity
	flag.IntVar(&numWorkers, "num_workers", 10, "Number of workds to consume queue")
	flag.Parse()
	cache.Pool = cache.NewCachePool()

	connectionString := fmt.Sprintf(
		"user=%s password=%s dbname=%s sslmode=disable",
		os.Getenv("APP_DB_USERNAME"),
		os.Getenv("APP_DB_PASSWORD"),
		os.Getenv("APP_DB_NAME"),
	)

	db, err := sqlx.Open("postgres", connectionString)
	if err != nil {
		log.Fatal(err)
	}

	go UsersToDB(numWorkers, db, cache, createUsersQueue)
	go UsersToDB(numWorkers, db, cache, updateUsersQueue)
	go UsersToDB(numWorkers, db, cache, deleteUsersQueue)

	a := App{}
	a.Initialize(cache, db)
	a.Run(":8080")
}
