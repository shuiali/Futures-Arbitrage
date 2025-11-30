package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"crossspread-md-ingest/internal/connector"

	"github.com/redis/go-redis/v9"
)

// RedisPublisher publishes market data to Redis Streams
type RedisPublisher struct {
	client *redis.Client
}

// NewRedisPublisher creates a new Redis publisher
func NewRedisPublisher(addr string) (*RedisPublisher, error) {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	// Test connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &RedisPublisher{client: client}, nil
}

// Client returns the underlying Redis client
func (p *RedisPublisher) Client() *redis.Client {
	return p.client
}

// Close closes the Redis connection
func (p *RedisPublisher) Close() error {
	return p.client.Close()
}

// PublishOrderbook publishes orderbook to Redis Stream AND Pub/Sub for real-time streaming
func (p *RedisPublisher) PublishOrderbook(ob *connector.Orderbook) error {
	data, err := json.Marshal(ob)
	if err != nil {
		return err
	}

	// Stream key: orderbook:{exchange}:{symbol}
	streamKey := fmt.Sprintf("orderbook:%s:%s", ob.ExchangeID, ob.Symbol)

	// Publish to Redis Stream (for historical data/replay)
	if err := p.client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: streamKey,
		MaxLen: 1000, // Keep last 1000 entries
		Approx: true,
		Values: map[string]interface{}{
			"data": string(data),
		},
	}).Err(); err != nil {
		return err
	}

	// Also publish to Pub/Sub for real-time WebSocket streaming
	if err := p.client.Publish(context.Background(), streamKey, string(data)).Err(); err != nil {
		return err
	}

	// Debug: log published channel for troubleshooting
	fmt.Printf("[md-ingest] Published orderbook to channel/stream %s (bids=%d, asks=%d)\n", streamKey, len(ob.Bids), len(ob.Asks))
	return nil
}

// PublishTrade publishes trade to Redis Stream
func (p *RedisPublisher) PublishTrade(trade *connector.Trade) error {
	data, err := json.Marshal(trade)
	if err != nil {
		return err
	}

	streamKey := fmt.Sprintf("trades:%s:%s", trade.ExchangeID, trade.Symbol)

	return p.client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: streamKey,
		MaxLen: 10000,
		Approx: true,
		Values: map[string]interface{}{
			"data": string(data),
		},
	}).Err()
}

// PublishSpread publishes computed spread to Redis Stream
func (p *RedisPublisher) PublishSpread(spread map[string]interface{}) error {
	data, err := json.Marshal(spread)
	if err != nil {
		return err
	}

	return p.client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: "spreads",
		MaxLen: 10000,
		Approx: true,
		Values: map[string]interface{}{
			"data": string(data),
		},
	}).Err()
}

// Publish publishes a message to a Redis channel (Pub/Sub)
func (p *RedisPublisher) Publish(channel, message string) error {
	return p.client.Publish(context.Background(), channel, message).Err()
}

// PublishOrderbookPubSub publishes orderbook update via Redis Pub/Sub for real-time streaming
func (p *RedisPublisher) PublishOrderbookPubSub(ob *connector.Orderbook) error {
	data, err := json.Marshal(ob)
	if err != nil {
		return err
	}

	// Pub/Sub channel: orderbook:{exchange}:{symbol}
	channel := fmt.Sprintf("orderbook:%s:%s", ob.ExchangeID, ob.Symbol)
	return p.client.Publish(context.Background(), channel, string(data)).Err()
}

// PublishSpreadPubSub publishes spread update via Redis Pub/Sub for real-time streaming
func (p *RedisPublisher) PublishSpreadPubSub(spreadID string, data []byte) error {
	channel := fmt.Sprintf("spread:%s", spreadID)
	return p.client.Publish(context.Background(), channel, string(data)).Err()
}

// SetSpread stores a spread in Redis as a key-value with expiration
func (p *RedisPublisher) SetSpread(spreadID string, data []byte) error {
	ctx := context.Background()
	key := fmt.Sprintf("spread:data:%s", spreadID)

	// Set with 5 minute expiration (spreads auto-expire if not updated)
	if err := p.client.Set(ctx, key, data, 5*time.Minute).Err(); err != nil {
		return err
	}

	// Also publish to Pub/Sub for real-time streaming to frontend
	if err := p.PublishSpreadPubSub(spreadID, data); err != nil {
		// Log but don't fail - Pub/Sub is best-effort
		fmt.Printf("Warning: failed to publish spread to Pub/Sub: %v\n", err)
	}

	// Also add to spreads set for listing
	return p.client.SAdd(ctx, "spreads:active", spreadID).Err()
}

// SetSpreadsList stores the list of active spreads summary
func (p *RedisPublisher) SetSpreadsList(data []byte) error {
	ctx := context.Background()
	return p.client.Set(ctx, "spreads:list", data, 30*time.Second).Err()
}
