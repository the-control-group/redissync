package redissync

import (
	"os"

	"github.com/garyburd/redigo/redis"
	//"github.com/the-control-group/split-testing-service/pkg/db/redissync"
	"testing"
	"time"
)

var redisAddr = os.Getenv("REDIS_ADDR")
var password = ""
var dbIndex = 15

var pool = &redis.Pool{
	MaxIdle:     3,
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
	rs := &RedisSync{Key: "special flower", Pool: pool}
	rs.Lock()
	exists, err := redis.Bool(pool.Get().Do("EXISTS", "special flower.lock"))
	if err != nil {
		t.Fatal("eror checking if lock key exists")
	}
	if exists == false {
		t.Fatal("lock key doesn't exist after lock")
	}
	err = rs.Unlock()
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
	rs1 := &RedisSync{Key: "special flower", Pool: pool, Token: "token1"}
	rs1.Lock()
	rs2 := &RedisSync{Key: "special flower", Pool: pool, Token: "token2"}
	err = rs2.Unlock()
	if err == nil {
		t.Fatal("should have failed to unlock key", rs1.Key, err)
	}
}
