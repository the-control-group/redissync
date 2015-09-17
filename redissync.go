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
	DefaultRetries = 16
	DefaultDelay   = 50 * time.Millisecond
)

var ErrObtainingLock = "Unable to obtain lock in %d retries with %d millisecond delay"
var ErrUnownedLock = "Attempted to unlock a key owned by another locker: %s"

// TODO Check to see if key even exists in unlock script and return different error
// TODO Create list of tokens in redis to make sure they get a unique one, and not let them pass one in
// TODO Handle redis connection internally instead of requiring pool

var unlockScript = redis.NewScript(1, "if redis.call('get',KEYS[1]) == ARGV[1] then return redis.call('DEL',KEYS[1]) else return {err='Token does not match'} end")

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

type RedisSync struct {
	Key     string
	LockKey string
	Pool    *redis.Pool
	Expiry  time.Duration
	Retries int // Use -1 for no limit
	Delay   time.Duration
	Token   string // the value of the lock key used to make sure only this locker can unlock it. will generate random string if one is not supplied.
	ErrChan chan error
}

func (s *RedisSync) Lock() {
	if s.LockKey == "" {
		s.LockKey = getLockKey(s.Key)
	}
	if s.Retries == 0 {
		s.Retries = DefaultRetries
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
	for i := 0; i < s.Retries; i++ {
		_, err := s.Pool.Get().Do("SET", s.LockKey, s.Token, "NX", "PX", int(s.Expiry/time.Millisecond))
		if err == nil {
			s.ErrChan <- nil
			return
		}
		time.Sleep(s.Delay)
	}
	if s.ErrChan != nil {
		s.ErrChan <- errors.New(fmt.Sprintf(ErrObtainingLock, s.Retries, s.Delay))
	}
}

func (s *RedisSync) Unlock() {
	_, err := unlockScript.Do(s.Pool.Get(), s.LockKey, s.Token)
	if err != nil && s.ErrChan != nil {
		s.ErrChan <- errors.New(fmt.Sprintf(ErrUnownedLock))
	} else {
		s.ErrChan <- nil
	}
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
