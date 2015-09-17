package redissync

import (
	"os"

	"github.com/garyburd/redigo/redis"
	"testing"
	"time"
)

var redisAddr = os.Getenv("REDIS_ADDR")
var password = ""
var dbIndex = 15

var pool = &redis.Pool{
	MaxIdle:     3,
	Wait:        true,
	IdleTimeout: 240 * time.Second,
	Dial: func() (redis.Conn, error) {
		var server = redisAddr
		if server == "" {
			server = "127.0.0.1:6379"
		}
		c, err := redis.Dial("tcp", server)
		if err != nil {
			return nil, err
		}
		if password != "" {
			if _, err := c.Do("AUTH", password); err != nil {
				c.Close()
				return nil, err
			}
		}
		_, err = c.Do("SELECT", dbIndex)
		if err != nil {
			panic("Error selecting database")
		}
		return c, err
	},
	TestOnBorrow: func(c redis.Conn, t time.Time) error {
		_, err := c.Do("PING")
		return err
	},
}

// Intended usage
func TestRedisSync_IntendedUsage(t *testing.T) {
	_, err := pool.Get().Do("FLUSHDB")
	if err != nil {
		t.Fatal("Error flushing database")
	}
	rs := &RedisSync{Key: "special flower", Pool: pool, ErrChan: make(chan error, 1), Timeout: 1 * time.Second}
	rs.Lock()
	err = <-rs.ErrChan
	if err != nil {
		t.Fatal("failed to obtain lock", err)
	}
	exists, err := redis.Bool(pool.Get().Do("EXISTS", "special flower.lock"))
	if err != nil {
		t.Fatal("eror checking if lock key exists")
	}
	if exists == false {
		t.Fatal("lock key doesn't exist after lock")
	}
	rs.Unlock()
	err = <-rs.ErrChan
	if err != nil {
		t.Fatal("failed to unlock key", rs.Key, err)
	}
	exists, err = redis.Bool(pool.Get().Do("EXISTS", "special flower.lock"))
	if err != nil {
		t.Fatal("eror checking if lock key exists", err)
	}
	if exists == true {
		t.Fatal("lock key still exist after unlock")
	}
}

// Error trying to unlock key owned by another locker
func TestRedisSync_ErrTryingToUnlockKeyOwnedByAnotherLocker(t *testing.T) {
	_, err := pool.Get().Do("FLUSHDB")
	if err != nil {
		t.Fatal("Error flushing database", err)
	}
	_, err = pool.Get().Do("SET", "special flower", "super delicate")
	if err != nil {
		t.Fatal("Error setting key to test", err)
	}
	rs1 := &RedisSync{Key: "special flower", Pool: pool, Token: "token1", ErrChan: make(chan error, 1), Timeout: 1 * time.Second}
	rs1.Lock()
	err = <-rs1.ErrChan
	if err != nil {
		t.Fatal("failed to obtain lock", err)
	}
	rs2 := &RedisSync{Key: "special flower", Pool: pool, Token: "token2", ErrChan: make(chan error, 1), Timeout: 1 * time.Second}
	rs2.Unlock()
	err = <-rs2.ErrChan
	if err == nil {
		t.Fatal("should have failed to unlock key", rs1.Key, err)
	}
}

// Error trying to obtain lock
func TestRedisSync_ErrTryingToObtainLock(t *testing.T) {
	_, err := pool.Get().Do("FLUSHDB")
	if err != nil {
		t.Fatal("Error flushing database", err)
	}
	_, err = pool.Get().Do("SET", "special flower", "super delicate")
	if err != nil {
		t.Fatal("Error setting key to test", err)
	}
	rs1 := &RedisSync{Key: "special flower", Pool: pool, Token: "token1", ErrChan: make(chan error, 1), Timeout: 1 * time.Second}
	rs1.Lock()
	err = <-rs1.ErrChan
	if err != nil {
		t.Fatal("failed to obtain lock", err)
	}
	rs2 := &RedisSync{Key: "special flower", Pool: pool, Token: "token2", ErrChan: make(chan error, 1), Timeout: 1 * time.Millisecond}
	rs2.Lock()
	err = <-rs2.ErrChan
	if err == nil {
		t.Fatal("should have failed to obtain lock", rs1.Key, err)
	}
}
