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
	ping, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err = client.Ping(ping).Err(); err != nil {
		_ = client.Close()
		return nil, errSessionStartup
	}
	return client, nil
}
