package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/pkg/errors"
)

func main() {
	redisClient := redis.NewClient(&redis.Options{
		Network: "tcp",
		Addr:    "0.0.0.0:6379",

		// Dialer creates new network connection and has priority over
		// Network and Addr options.
		// Dialer func(ctx context.Context, network, addr string) (net.Conn, error)

		// Hook that is called when new connection is established.
		// OnConnect func(ctx context.Context, cn *Conn) error

		Username:           "admin",
		Password:           "password",
		DB:                 0,
		MaxRetries:         3,
		MinRetryBackoff:    time.Millisecond*1,
		MaxRetryBackoff:    time.Millisecond*5,
		DialTimeout:        time.Second*5,
		ReadTimeout:        time.Second*1,
		WriteTimeout:       time.Second*2,
		PoolSize:           2,
		MinIdleConns:       1,
		MaxConnAge:         time.Second*60,
		PoolTimeout:        time.Second*5,
		IdleTimeout:        time.Second*60,
		IdleCheckFrequency: time.Second*5,

		// TLS Config to use. When set TLS will be negotiated.
		// TLSConfig *tls.Config

		// Limiter interface used to implemented circuit breaker or rate limiter.
		// Limiter Limiter
	})

	ctx := context.Background()

	start := time.Now()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Println(errors.Wrap(err, "redis ping"))
		return
	}
	msecs := time.Since(start).Milliseconds()

	log.Print("info", "redis client started", map[string]interface{}{
		"msecs": msecs,
	})


	clientName, _ := redisClient.ClientGetName(ctx).Result()
	clientID, _ := redisClient.ClientID(ctx).Result()
	clientList, _ := redisClient.ClientList(ctx).Result()
	poolStats := fmt.Sprintf("%#v", redisClient.PoolStats())

	log.Print("info", "redis info", map[string]interface{}{
		"clientName": clientName,
		"clientId":   clientID,
		"clientList": clientList,
		"poolStats":  poolStats,
	})
}