package main

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/tls"
	"io"
	"log"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/pkg/errors"
)

// RedisCFG holds the configuration parameters for, well, no surprise here
// but for Redis (Sentinel Cluster specifically).
type RedisCFG struct {
	Enabled    bool          `conf:"env:SM_CACHE_REDIS_ENABLED"`
	Compress   bool          `conf:"env:SM_CACHE_REDIS_COMPRESS"`
	EncryptKey string        `conf:"env:SM_CACHE_REDIS_ENCRYPT_KEY,noprint"`
	DefaultTTL time.Duration `conf:"env:SM_CACHE_REDIS_DEFAULT_TTL"`
	// The master name.
	MasterName string `conf:"env:SM_CACHE_REDIS_MASTER_NAME"`
	// A seed list of host:port addresses of sentinel nodes.
	SentinelAddrs string `conf:"env:SM_CACHE_REDIS_SENTINEL_ADDRESSES"`

	// If specified with SentinelPassword, enables ACL-based authentication (via
	// AUTH <user> <pass>).
	SentinelUsername string `conf:"env:SM_CACHE_REDIS_SENTINEL_USERNAME"`
	// Sentinel password from "requirepass <password>" (if enabled) in Sentinel
	// configuration, or, if SentinelUsername is also supplied, used for ACL-based
	// authentication.
	SentinelPassword string `conf:"env:SM_CACHE_REDIS_SENTINEL_PASSWORD,mask"`

	// Allows routing read-only commands to the closest master or slave node.
	// This option only works with NewFailoverClusterClient.
	RouteByLatency bool `conf:"env:SM_CACHE_REDIS_ROUTE_BY_LATENCY,default:true"`

	Username string `conf:"env:SM_CACHE_REDIS_USERNAME"`
	Password string `conf:"env:SM_CACHE_REDIS_PASSWORD,mask"`
	DB       int    `conf:"env:SM_CACHE_REDIS_DB_NUM"`

	MaxRetries      int           `conf:"env:SM_CACHE_REDIS_MAX_RETRIES"`
	MinRetryBackoff time.Duration `conf:"env:SM_CACHE_REDIS_MIN_RETRY_BACKOFF"`
	MaxRetryBackoff time.Duration `conf:"env:SM_CACHE_REDIS_MAX_RETRY_BACKOFF"`

	DialTimeout  time.Duration `conf:"env:SM_CACHE_REDIS_DIAL_TIMEOUT"`
	ReadTimeout  time.Duration `conf:"env:SM_CACHE_REDIS_READ_TIMEOUT"`
	WriteTimeout time.Duration `conf:"env:SM_CACHE_REDIS_WRITE_TIMEOUT"`

	PoolSize     int           `conf:"env:SM_CACHE_REDIS_POOL_SIZE"`
	MinIdleConns int           `conf:"env:SM_CACHE_REDIS_MIN_IDLE_CONNECTIONS"`
	MaxConnAge   time.Duration `conf:"env:SM_CACHE_REDIS_MAX_CONNECTION_AGE"`
	PoolTimeout  time.Duration `conf:"env:SM_CACHE_REDIS_POOL_TIMEOUT"`
	IdleTimeout  time.Duration `conf:"env:SM_CACHE_REDIS_IDLE_TIMEOUT"`

	// IdleCheckFrequency specifies how often we check if connection is expired.
	// This is needed in case app is idle and there is no activity. Normally idle
	// connections are closed when go-redis asks pool for a (healthy) connection.
	IdleCheckFrequency time.Duration `conf:"env:SM_CACHE_REDIS_IDLE_CHECK_FREQUENCY"`

	TLSConfig *tls.Config
}

type CFG struct {
	Redis RedisCFG
}

// Redis implements the Storage interface.
type Redis struct {
	client *redis.Client
	cfg    *RedisCFG
}

// redInit gets the Redis client prepped and ready for action.
func (r *Redis) redInit(cfg *CFG) error {
	ctx := context.Background()

	redisClient := redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:         cfg.Redis.MasterName,                        //TODO: replace with k8s secret
		SentinelAddrs:      strings.Split(cfg.Redis.SentinelAddrs, ","), //TODO: replace with k8s secret
		SentinelUsername:   cfg.Redis.SentinelUsername,                  //TODO: replace with k8s secret
		SentinelPassword:   cfg.Redis.SentinelPassword,                  //TODO: replace with k8s secret
		RouteByLatency:     cfg.Redis.RouteByLatency,
		Username:           cfg.Redis.Username, //TODO: replace with k8s secret
		Password:           cfg.Redis.Password, //TODO: replace with k8s secret
		DB:                 cfg.Redis.DB,
		MaxRetries:         cfg.Redis.MaxRetries,
		MinRetryBackoff:    cfg.Redis.MinRetryBackoff,
		MaxRetryBackoff:    cfg.Redis.MaxRetryBackoff,
		DialTimeout:        cfg.Redis.DialTimeout,
		ReadTimeout:        cfg.Redis.ReadTimeout,
		WriteTimeout:       cfg.Redis.WriteTimeout,
		PoolSize:           cfg.Redis.PoolSize,
		MinIdleConns:       cfg.Redis.MinIdleConns,
		MaxConnAge:         cfg.Redis.MaxConnAge,
		PoolTimeout:        cfg.Redis.PoolTimeout,
		IdleTimeout:        cfg.Redis.IdleTimeout,
		IdleCheckFrequency: cfg.Redis.IdleCheckFrequency,
		TLSConfig:          &tls.Config{},
	})

	start := time.Now()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return errors.Wrap(err, "redis ping")
	}
	msecs := time.Since(start).Milliseconds()

	log.Print("info", "redis client started", map[string]interface{}{
		"msecs": msecs,
	})

	r.client = redisClient
	r.cfg = &cfg.Redis

	return nil
}

// Get fetches the cache from Redis and will decompress and/or decrypt
// the cache if the configuration is set for either respectively.
func (r *Redis) Get(ctx context.Context, key string) ([]byte, error) {
	raw, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return nil, errors.Wrap(err, "redis get")
	}

	b := bytes.NewReader([]byte(raw))

	reader, err := zlib.NewReader(b)
	if err != nil {
		return nil, errors.Wrap(err, "zlib reader")
	}

	var out bytes.Buffer
	if _, err := io.Copy(&out, reader); err != nil {
		return nil, errors.Wrap(err, "io copy")
	}

	if err := reader.Close(); err != nil {
		log.Print("error", "zlib reader close", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// The data was not encrypted -- just return the uncompressed value.
	return out.Bytes(), nil
}

// Set stores the cache in Redis encrypted and/or compressed if
// the configuration is set for either respectively. The opts
// allow the caller to either specify a time.Duration or use the
// default ttl in the configuration.
func (r *Redis) Set(ctx context.Context, key string, val []byte, opts ...time.Duration) error {
	ttl := r.cfg.DefaultTTL
	if len(opts) > 0 {
		ttl = opts[0]
	}

	
	var b bytes.Buffer
	w := zlib.NewWriter(&b)

	if _, err := w.Write(val); err != nil {
		return errors.Wrap(err, "zlib write")
	}

	if err := w.Close(); err != nil {
		log.Print("error", "zlib writer close", map[string]interface{}{
			"error": err.Error(),
		})
	}

	val = b.Bytes()
	

	if _, err := r.client.Set(ctx, key, val, ttl).Result(); err != nil {
		return errors.Wrap(err, "redis set")
	}

	return nil
}

// Delete removes the cache entry.
func (r *Redis) Delete(ctx context.Context, key string) error {
	if _, err := r.client.Del(ctx, key).Result(); err != nil {
		return errors.Wrap(err, "redis delete")
	}

	return nil
}

//TODO: implement the generic function for like SCAN commands and junk...
func (r *Redis) Cmd(ctx context.Context, key, cmd string) ([]byte, error) {
	return nil, nil
}

// Close like closes stuff and thangs...
func (r *Redis) Close() {
	r.client.Close()
}


func main() {
	// Giving some time for Redis to initialize...
	time.Sleep(time.Second * 5)

	log.Println("Starting...")

	redisClient := redis.NewFailoverClusterClient(&redis.FailoverOptions{
		MasterName:         "redis1",
		SentinelAddrs:      []string{"10.10.1.5:26379", "10.10.1.6:26379", "10.10.1.7:26379"},
		SentinelUsername:   "buttman",
		SentinelPassword:   "buttword",
		RouteByLatency:     true,
		Username:           "assman",
		Password:           "assword",
		DB:                 0,
		MaxRetries:         3,
		MinRetryBackoff:    time.Microsecond * 500,
		MaxRetryBackoff:    time.Millisecond * 2,
		DialTimeout:        time.Second * 3,
		ReadTimeout:        time.Second * 5,
		WriteTimeout:       time.Second * 10,
		PoolSize:           2,
		MinIdleConns:       1,
		MaxConnAge:         time.Minute * 5,
		PoolTimeout:        time.Second * 2,
		IdleTimeout:        time.Second * 30,
		IdleCheckFrequency: time.Second * 5,
		//TLSConfig:          &tls.Config{},
	})

	start := time.Now()
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		log.Fatal(errors.Wrap(err, "redis ping"))
	}
	msecs := time.Since(start).Milliseconds()

	log.Print("info", "redis client started ", map[string]interface{}{
		"msecs": msecs,
	})

	str, err := redisClient.Set(context.Background(), "foo", "kiss my bar ass", time.Second * 100).Result()
	log.Println(str, err)

	res, err := redisClient.Get(context.Background(), "foo").Result()
	log.Println(res, err)

	log.Println("Finished!")
}