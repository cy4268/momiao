package main

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var errSessionStartup = errors.New("session Redis startup failed")

func openSessionRedis(ctx context.Context, path string) (*redis.Client, error) {
	options, err := readSessionRedisConfig(path)
	if err != nil {
		return nil, errSessionStartup
	}
	client := redis.NewClient(options)
	// After a host reboot Docker restarts containers without compose ordering, so
	// Redis may still be starting. Exiting here would restart the platform with a
	// new network namespace and strand the edge container that shares it.
	deadline := time.Now().Add(90 * time.Second)
	for {
		ping, cancel := context.WithTimeout(ctx, 2*time.Second)
		err = client.Ping(ping).Err()
		cancel()
		if err == nil {
			return client, nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			_ = client.Close()
			return nil, errSessionStartup
		}
		time.Sleep(time.Second)
	}
}
