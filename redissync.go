package redissync

import (
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/garyburd/redigo/redis"
)

const (
	DefaultExpiry  = 5 * time.Second
	DefaultTimeout = 6 * time.Second
	DefaultDelay   = 50 * time.Millisecond
)

var ErrObtainingLock = "Unable to obtain lock (%s): %s"
var ErrUnownedLock = "Attempted to unlock a key owned by another locker: %s"

// TODO Check to see if key even exists in unlock script and return different error
// TODO Create list of tokens in redis to make sure they get a unique one, and not let them pass one in

var unlockScript = redis.NewScript(1, "if redis.call('get',KEYS[1]) == ARGV[1] then return redis.call('DEL',KEYS[1]) else return {err='Token does not match'} end")

// var checkScript = redis.NewScript(1, "if redis.call('get',KEYS[1]) == ARGV[1] then return redis.call('DEL',KEYS[1]) else return {err='Token does not match'} end")

var checkScript = redis.NewScript(1, "if redis.call('get',KEYS[1]) == ARGV[1] then return 'true' else return 'false' end")

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func Sync(key string, pool *redis.Pool, expiry, timeout, delay time.Duration, fn func() error) error {
	var lockErr error
	var fnErr error
	var locker = RedisSync{Key: key, Pool: pool, Expiry: expiry, Timeout: timeout, Delay: delay, ErrChan: make(chan error, 1)}
	locker.Lock()
	lockErr = <-locker.ErrChan
	if lockErr != nil {
		return lockErr
	}
	fnErr = fn()
	locker.Unlock()
	lockErr = <-locker.ErrChan
	if lockErr != nil {
		return lockErr
	}
	return fnErr
}

type RedisSync struct {
	Key     string
	LockKey string
	Pool    *redis.Pool
	Expiry  time.Duration
	Timeout time.Duration // Use 0 for no timeout
	Delay   time.Duration
	Token   string // the value of the lock key used to make sure only this locker can unlock it. will generate random string if one is not supplied.
	ErrChan chan error
}

func (s *RedisSync) Lock() {
	if s.LockKey == "" {
		s.LockKey = getLockKey(s.Key)
	}
	if s.Timeout == 0 {
		s.Timeout = DefaultTimeout
	}
	if s.Delay == 0 {
		s.Delay = DefaultDelay
	}
	if s.Expiry == 0 {
		s.Expiry = DefaultExpiry
	}
	if s.Token == "" {
		s.Token = generateToken(10)
	}
	var start = time.Now()
	var err error
	var conn = s.Pool.Get()
	defer conn.Close()
	var tries = 0
	for tries == 0 || time.Since(start) < s.Timeout {
		tries++
		_, err = redis.String(conn.Do("SET", s.LockKey, s.Token, "NX", "PX", int(s.Expiry/time.Millisecond)))
		if err == nil {
			if s.ErrChan != nil {
				s.ErrChan <- nil
			}
			return
		}
		time.Sleep(s.Delay)
	}
	if s.ErrChan != nil {
		s.ErrChan <- errors.New(fmt.Sprintf(ErrObtainingLock, err.Error(), s.Key))
	}
}

func (s *RedisSync) Unlock() {
	var conn = s.Pool.Get()
	defer conn.Close()
	_, err := unlockScript.Do(conn, s.LockKey, s.Token)
	if err != nil && s.ErrChan != nil {
		s.ErrChan <- errors.New(fmt.Sprintf(ErrUnownedLock, s.Key))
	} else {
		if s.ErrChan != nil {
			s.ErrChan <- nil
		}
	}
}

func (s *RedisSync) HasLock() bool {
	var conn = s.Pool.Get()
	defer conn.Close()
	ans, err := redis.Bool(checkScript.Do(conn, s.LockKey, s.Token))
	if s.ErrChan != nil {
		s.ErrChan <- err
	}
	return ans
}

func getLockKey(key string) string {
	return key + ".lock"
}

func generateToken(size int) string {
	b := make([]rune, size)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// References
// [0]: http://redis.io/topics/distlock "Distributed locks with Redis"
// [1]: http://stackoverflow.com/a/22892986/4187523 "How to generate a random string of a fixed length in golang?"
