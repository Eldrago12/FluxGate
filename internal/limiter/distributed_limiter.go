package limiter

import (
	"context"
	"fmt"
	"time"

	redisv1 "cloud.google.com/go/redis/apiv1"
	redispb "cloud.google.com/go/redis/apiv1/redispb"
	"github.com/go-redis/redis/v8"
)

type DistributedLimiter struct {
	client     *redis.Client
	rate       float64
	bucketSize float64
}

func NewDistributedLimiter(projectID, region, redisName string, rate, bucketSize float64) (*DistributedLimiter, error) {
	ctx := context.Background()

	// Create a Redis client
	redisClient, err := redisv1.NewCloudRedisClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create Redis client: %v", err)
	}
	defer redisClient.Close()

	// Get the Redis instance
	name := fmt.Sprintf("projects/%s/locations/%s/instances/%s", projectID, region, redisName)
	req := &redispb.GetInstanceRequest{
		Name: name,
	}
	instance, err := redisClient.GetInstance(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get Redis instance: %v", err)
	}

	// Connect to the Redis instance
	client := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", instance.Host, instance.Port),
	})

	_, err = client.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %v", err)
	}

	return &DistributedLimiter{
		client:     client,
		rate:       rate,
		bucketSize: bucketSize,
	}, nil
}

func (dl *DistributedLimiter) Allow(key string) bool {
	ctx := context.Background()
	now := time.Now().UnixNano()

	pipe := dl.client.Pipeline()
	tokenCount := pipe.ZCount(ctx, key, "-inf", fmt.Sprintf("%d", now))
	pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", now-int64(time.Second)))
	pipe.ZAdd(ctx, key, &redis.Z{Score: float64(now + int64(time.Second)), Member: now})
	pipe.Expire(ctx, key, time.Minute)
	_, err := pipe.Exec(ctx)

	if err != nil {
		// If there's an error, we'll allow the request
		return true
	}

	return tokenCount.Val() < int64(dl.bucketSize)
}

func (dl *DistributedLimiter) Close() error {
	return dl.client.Close()
}
