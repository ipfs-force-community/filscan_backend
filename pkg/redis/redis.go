package redis

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	redigo "github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
)

func NewRedis(conf *config.Config) *Redis {
	RedisConn := &redigo.Pool{
		MaxIdle:     *conf.Redis.MaxIdle,
		MaxActive:   *conf.Redis.MaxActive,
		IdleTimeout: time.Duration(*conf.Redis.IdleTimeout),
		Dial: func() (redigo.Conn, error) {
			c, err := redigo.Dial("tcp", *conf.Redis.RedisAddress)
			if err != nil {
				return nil, err
			}
			return c, err
		},
		TestOnBorrow: func(c redigo.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}

	return &Redis{RedisConn: RedisConn}
}

type Redis struct {
	RedisConn *redigo.Pool
}

type Cache interface {
	Set(key string, data interface{}, time time.Duration) error
	Exists(key string) (bool, error)
	Get(key string) ([]byte, error)
	Delete(key string) (bool, error)
	Keys(key string) ([]string, error)
}

type LockClient interface {
	SetNEX(key string, data interface{}, expireSeconds int64) (int64, error)
	Eval(src string, keyCount int, keysAndArgs []interface{}) (interface{}, error)
}

func (r Redis) Set(key string, data interface{}, time time.Duration) error {
	value, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if value != nil {
		conn := r.RedisConn.Get()
		defer func(conn redigo.Conn) {
			_ = conn.Close()
		}(conn)

		_, err = conn.Do("SET", key, value)
		if err != nil {
			return err
		}

		_, err = conn.Do("EXPIRE", key, int(time.Seconds()))
		if err != nil {
			return err
		}
	}

	return nil
}

func (r Redis) SetNoExpire(key string, data interface{}) error {
	value, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if value != nil {
		conn := r.RedisConn.Get()
		defer func(conn redigo.Conn) {
			_ = conn.Close()
		}(conn)

		_, err = conn.Do("SET", key, value)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r Redis) SetNEX(key string, data interface{}, expireSeconds int64) (int64, error) {
	value, err := json.Marshal(data)
	if err != nil {
		return -1, err
	}
	if value == nil {
		return -1, nil
	}
	conn := r.RedisConn.Get()
	defer func(conn redigo.Conn) {
		_ = conn.Close()
	}(conn)

	reply, err := conn.Do("SET", key, value, "EX", expireSeconds, "NX")
	if err != nil {
		return -1, err
	}

	if respStr, ok := reply.(string); ok && strings.ToLower(respStr) == "ok" {
		return 1, nil
	}
	return redigo.Int64(reply, err)
}

func (r Redis) SetNX(key string, data interface{}) (int64, error) {
	value, err := json.Marshal(data)
	if err != nil {
		return -1, err
	}
	if value == nil {
		return -1, nil
	}
	conn := r.RedisConn.Get()
	defer func(conn redigo.Conn) {
		_ = conn.Close()
	}(conn)

	reply, err := conn.Do("SET", key, value, "NX")
	if err != nil {
		return -1, err
	}

	if respStr, ok := reply.(string); ok && strings.ToLower(respStr) == "ok" {
		return 1, nil
	}

	return redigo.Int64(reply, err)
}

// 支持使用 lua 脚本
func (r Redis) Eval(src string, keyCount int, keysAndArgs []interface{}) (interface{}, error) {
	args := make([]interface{}, 2+len(keysAndArgs))
	args[0] = src      //脚本
	args[1] = keyCount //key的数量
	copy(args[2:], keysAndArgs)

	conn := r.RedisConn.Get()
	defer func(conn redigo.Conn) {
		_ = conn.Close()
	}(conn)
	return conn.Do("EVAL", args...)
}

func (r Redis) Incr(key string) (int64, error) {
	if key == "" {
		return -1, errors.New("redis INCR key can't be empty")
	}
	conn := r.RedisConn.Get()
	defer func(conn redigo.Conn) {
		conn.Close()
	}(conn)

	return redigo.Int64(conn.Do("INCR", key))
}

func (r Redis) Keys(pattern string) ([]string, error) {
	conn := r.RedisConn.Get()
	defer func(conn redigo.Conn) {
		conn.Close()
	}(conn)

	return redigo.Strings(conn.Do("KEYS", pattern))
}

func (r Redis) Exists(key string) (bool, error) {
	conn := r.RedisConn.Get()
	defer func(conn redigo.Conn) {
		conn.Close()
	}(conn)

	exists, err := redigo.Bool(conn.Do("EXISTS", key))
	if err != nil {
		return false, err
	}

	return exists, nil
}

func (r Redis) Get(key string) ([]byte, error) {
	conn := r.RedisConn.Get()
	defer func(conn redigo.Conn) {
		conn.Close()
	}(conn)

	reply, err := redigo.Bytes(conn.Do("GET", key))
	if err != nil {
		return nil, err
	}

	return reply, nil
}

func (r Redis) Delete(key string) (bool, error) {
	conn := r.RedisConn.Get()
	defer func(conn redigo.Conn) {
		conn.Close()
	}(conn)

	return redigo.Bool(conn.Do("DEL", key))
}

func (r Redis) LikeDeletes(key string) error {
	conn := r.RedisConn.Get()
	defer func(conn redigo.Conn) {
		conn.Close()
	}(conn)

	keys, err := redigo.Strings(conn.Do("KEYS", "*"+key+"*"))
	if err != nil {
		return err
	}

	for _, key = range keys {
		_, err = r.Delete(key)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r Redis) HexCacheKey(ctx context.Context, req interface{}) (resp string, err error) {
	urlReq := ctx.Value("url")
	jsonReq, err := json.Marshal(req)
	if err != nil {
		return
	}
	combinedReq := []byte(urlReq.(string) + string(jsonReq))
	resp = hex.EncodeToString(combinedReq)
	return
}

func (r Redis) GetCacheResult(key string) (value []byte, err error) {
	isExist, err := r.Exists(key)
	if err != nil {
		return
	}
	if isExist {
		value, err = r.Get(key)
		if err != nil {
			return
		}
	}
	return
}
