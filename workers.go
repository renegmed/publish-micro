package main

import (
	"encoding/json"
	"log"
	"sync"

	redigo "github.com/garyburd/redigo/redis"
	"github.com/jmoiron/sqlx"
)

type Worker struct {
	cache Cache
	db    *sqlx.DB
	id    int
	queue string
}

func newWorker(id int, db *sqlx.DB, cache Cache, queue string) Worker {
	return Worker{cache: cache, db: db, id: id, queue: queue}
}

// process runs the queue and send data to the database. If the process fails,
// the method will will resend the data which will be held for the queue.
func (w Worker) process(id int) {
	for {
		conn := w.cache.Pool.Get()
		var channel string
		var uuid int

		if reply, err := redigo.Values(conn.Do("BLPOP", w.queue, 30+id)); err != nil { // pop out (consume) the data from the queue

			if _, err := redigo.Scan(reply, &channel, &uuid); err != nil { // populate channel and uuid with data i.e reply, from the queue
				w.cache.enqueueValue(w.queue, uuid)
				continue
			}

			values, err := redigo.String(conn.Do("GET", uuid))
			if err != nil { // if failed, return back the data to the queue.
				w.cache.enqueueValue(w.queue, uuid)
				continue
			}

			user := User{}
			if err := json.Unmarshal([]byte(values), &user); err != nil {
				w.cache.enqueueValue(w.queue, uuid) // if failed, return back the data to the queue.
				continue
			}

			log.Println(user)
			if err := user.create(w.db); err != nil {
				w.cache.enqueueValue(w.queue, uuid) // if failed, return back the data to the queue.
				continue
			}

			// At this point the user data in the cache has already removed and saved to database.

		} else if err != redigo.ErrNil {
			log.Fatal(err)
		}
		conn.Close()
	}
}

// UsersToDB calls goroutine function for processing data
// from the queue to database in asynchronous fashion.
func UsersToDB(numWorkers int, db *sqlx.DB, cache Cache, queue string) {
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		//???wg.Add(1)
		j := i
		go func(id int, db *sqlx.DB, cache Cache, queue string) {
			worker := newWorker(j, db, cache, queue)
			worker.process(j)
			defer wg.Done()
		}(i, db, cache, queue)
	}
	wg.Wait()
}
