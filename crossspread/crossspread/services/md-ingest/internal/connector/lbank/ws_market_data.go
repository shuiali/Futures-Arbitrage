package lbank

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// MarketDataHandler defines callback functions for market data events
type MarketDataHandler struct {
	OnDepth      func(symbol string, asks, bids [][]float64, timestamp time.Time)
	OnTrade      func(symbol string, price, volume float64, side string, timestamp time.Time)
	OnTicker     func(symbol string, ticker *WsTickResponse)
	OnKline      func(symbol string, kline *WsKbarResponse)
	OnError      func(err error)
	OnConnect    func()
	OnDisconnect func()
}

// WsMarketDataClient handles WebSocket connections for public market data
type WsMarketDataClient struct {
	useContractAPI bool
	wsURL          string
	conn           *websocket.Conn
	subscriptions  map[string]map[string]bool // channel -> symbols
	handler        *MarketDataHandler
	pingInterval   time.Duration
	reconnectDelay time.Duration

	mu          sync.RWMutex
	done        chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
	isConnected bool
}

// NewWsMarketDataClient creates a new market data WebSocket client
func NewWsMarketDataClient(config *ClientConfig, handler *MarketDataHandler) *WsMarketDataClient {
	wsURL := SpotWsBaseURL
	if config.UseContractAPI {
		wsURL = ContractWsBaseURL
	}

	pingInterval := config.PingInterval
	if pingInterval == 0 {
		pingInterval = 30 * time.Second
	}

	reconnectDelay := config.ReconnectDelay
	if reconnectDelay == 0 {
		reconnectDelay = 5 * time.Second
	}

	return &WsMarketDataClient{
		useContractAPI: config.UseContractAPI,
		wsURL:          wsURL,
		subscriptions:  make(map[string]map[string]bool),
		handler:        handler,
		pingInterval:   pingInterval,
		reconnectDelay: reconnectDelay,
		done:           make(chan struct{}),
	}
}

// Connect establishes the WebSocket connection
func (c *WsMarketDataClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.isConnected {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	c.ctx, c.cancel = context.WithCancel(ctx)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	log.Info().Str("url", c.wsURL).Msg("Connecting to LBank market data WebSocket")

	conn, _, err := dialer.DialContext(c.ctx, c.wsURL, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.isConnected = true
	c.done = make(chan struct{})
	c.mu.Unlock()

	log.Info().Msg("Connected to LBank market data WebSocket")

	if c.handler != nil && c.handler.OnConnect != nil {
		c.handler.OnConnect()
	}

	// Start read and ping loops
	go c.readLoop()
	go c.pingLoop()

	// Resubscribe to existing subscriptions
	c.resubscribe()

	return nil
}

// Disconnect closes the WebSocket connection
func (c *WsMarketDataClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isConnected {
		return nil
	}

	c.isConnected = false
	close(c.done)

	if c.cancel != nil {
		c.cancel()
	}

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}

	if c.handler != nil && c.handler.OnDisconnect != nil {
		c.handler.OnDisconnect()
	}

	return nil
}

// IsConnected returns connection status
func (c *WsMarketDataClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isConnected
}

// SubscribeDepth subscribes to orderbook depth updates
func (c *WsMarketDataClient) SubscribeDepth(symbols []string, depth int) error {
	c.mu.Lock()
	if c.subscriptions[ChannelDepth] == nil {
		c.subscriptions[ChannelDepth] = make(map[string]bool)
	}
	for _, s := range symbols {
		c.subscriptions[ChannelDepth][s] = true
	}
	c.mu.Unlock()

	if !c.IsConnected() {
		return nil // Will subscribe on connect
	}

	depthStr := strconv.Itoa(depth)
	for _, symbol := range symbols {
		msg := WsMessage{
			Action:    ActionSubscribe,
			Subscribe: ChannelDepth,
			Depth:     depthStr,
			Pair:      symbol,
		}
		if err := c.sendMessage(msg); err != nil {
			return err
		}
	}

	return nil
}

// SubscribeTrades subscribes to trade updates
func (c *WsMarketDataClient) SubscribeTrades(symbols []string) error {
	c.mu.Lock()
	if c.subscriptions[ChannelTrade] == nil {
		c.subscriptions[ChannelTrade] = make(map[string]bool)
	}
	for _, s := range symbols {
		c.subscriptions[ChannelTrade][s] = true
	}
	c.mu.Unlock()

	if !c.IsConnected() {
		return nil
	}

	for _, symbol := range symbols {
		msg := WsMessage{
			Action:    ActionSubscribe,
			Subscribe: ChannelTrade,
			Pair:      symbol,
		}
		if err := c.sendMessage(msg); err != nil {
			return err
		}
	}

	return nil
}

// SubscribeTicker subscribes to ticker updates
func (c *WsMarketDataClient) SubscribeTicker(symbols []string) error {
	c.mu.Lock()
	if c.subscriptions[ChannelTick] == nil {
		c.subscriptions[ChannelTick] = make(map[string]bool)
	}
	for _, s := range symbols {
		c.subscriptions[ChannelTick][s] = true
	}
	c.mu.Unlock()

	if !c.IsConnected() {
		return nil
	}

	for _, symbol := range symbols {
		msg := WsMessage{
			Action:    ActionSubscribe,
			Subscribe: ChannelTick,
			Pair:      symbol,
		}
		if err := c.sendMessage(msg); err != nil {
			return err
		}
	}

	return nil
}

// SubscribeKline subscribes to kline/candlestick updates
func (c *WsMarketDataClient) SubscribeKline(symbols []string, interval string) error {
	channelKey := ChannelKbar + "_" + interval

	c.mu.Lock()
	if c.subscriptions[channelKey] == nil {
		c.subscriptions[channelKey] = make(map[string]bool)
	}
	for _, s := range symbols {
		c.subscriptions[channelKey][s] = true
	}
	c.mu.Unlock()

	if !c.IsConnected() {
		return nil
	}

	for _, symbol := range symbols {
		msg := WsMessage{
			Action:    ActionSubscribe,
			Subscribe: ChannelKbar,
			Kbar:      interval,
			Pair:      symbol,
		}
		if err := c.sendMessage(msg); err != nil {
			return err
		}
	}

	return nil
}

// UnsubscribeDepth unsubscribes from orderbook depth
func (c *WsMarketDataClient) UnsubscribeDepth(symbols []string, depth int) error {
	c.mu.Lock()
	if c.subscriptions[ChannelDepth] != nil {
		for _, s := range symbols {
			delete(c.subscriptions[ChannelDepth], s)
		}
	}
	c.mu.Unlock()

	if !c.IsConnected() {
		return nil
	}

	depthStr := strconv.Itoa(depth)
	for _, symbol := range symbols {
		msg := WsMessage{
			Action:    ActionUnsubscribe,
			Subscribe: ChannelDepth,
			Depth:     depthStr,
			Pair:      symbol,
		}
		if err := c.sendMessage(msg); err != nil {
			return err
		}
	}

	return nil
}

// RequestDepth makes a one-time request for orderbook depth
func (c *WsMarketDataClient) RequestDepth(symbol string, depth int) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	msg := WsMessage{
		Action:  ActionRequest,
		Request: ChannelDepth,
		Depth:   strconv.Itoa(depth),
		Pair:    symbol,
	}
	return c.sendMessage(msg)
}

// sendMessage sends a message over WebSocket
func (c *WsMarketDataClient) sendMessage(msg interface{}) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	log.Debug().Str("msg", string(data)).Msg("Sending WS message")
	return conn.WriteMessage(websocket.TextMessage, data)
}

// resubscribe resubscribes to all channels after reconnect
func (c *WsMarketDataClient) resubscribe() {
	c.mu.RLock()
	subs := make(map[string]map[string]bool)
	for k, v := range c.subscriptions {
		subs[k] = make(map[string]bool)
		for s := range v {
			subs[k][s] = true
		}
	}
	c.mu.RUnlock()

	for channel, symbols := range subs {
		for symbol := range symbols {
			var msg WsMessage

			if channel == ChannelDepth {
				msg = WsMessage{
					Action:    ActionSubscribe,
					Subscribe: ChannelDepth,
					Depth:     "100",
					Pair:      symbol,
				}
			} else if channel == ChannelTrade {
				msg = WsMessage{
					Action:    ActionSubscribe,
					Subscribe: ChannelTrade,
					Pair:      symbol,
				}
			} else if channel == ChannelTick {
				msg = WsMessage{
					Action:    ActionSubscribe,
					Subscribe: ChannelTick,
					Pair:      symbol,
				}
			} else if len(channel) > len(ChannelKbar)+1 {
				// Extract kline interval from channel key
				interval := channel[len(ChannelKbar)+1:]
				msg = WsMessage{
					Action:    ActionSubscribe,
					Subscribe: ChannelKbar,
					Kbar:      interval,
					Pair:      symbol,
				}
			}

			if msg.Subscribe != "" {
				if err := c.sendMessage(msg); err != nil {
					log.Error().Err(err).Str("channel", channel).Str("symbol", symbol).Msg("Failed to resubscribe")
				}
			}
		}
	}
}

// readLoop reads messages from WebSocket
func (c *WsMarketDataClient) readLoop() {
	defer func() {
		c.mu.Lock()
		c.isConnected = false
		c.mu.Unlock()

		if c.handler != nil && c.handler.OnDisconnect != nil {
			c.handler.OnDisconnect()
		}
	}()

	for {
		select {
		case <-c.done:
			return
		default:
			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()

			if conn == nil {
				return
			}

			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Error().Err(err).Msg("WebSocket read error")
				if c.handler != nil && c.handler.OnError != nil {
					c.handler.OnError(err)
				}
				return
			}

			c.handleMessage(message)
		}
	}
}

// pingLoop sends periodic ping messages
func (c *WsMarketDataClient) pingLoop() {
	ticker := time.NewTicker(c.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			if !c.IsConnected() {
				return
			}

			pingID := generatePingID()
			msg := map[string]string{
				"action": ActionPing,
				"ping":   pingID,
			}

			if err := c.sendMessage(msg); err != nil {
				log.Error().Err(err).Msg("Failed to send ping")
				if c.handler != nil && c.handler.OnError != nil {
					c.handler.OnError(err)
				}
			}
		}
	}
}

// handleMessage processes incoming WebSocket messages
func (c *WsMarketDataClient) handleMessage(message []byte) {
	// Try to determine message type
	var baseMsg map[string]interface{}
	if err := json.Unmarshal(message, &baseMsg); err != nil {
		log.Error().Err(err).Str("msg", string(message)).Msg("Failed to parse message")
		return
	}

	// Check for pong response
	if action, ok := baseMsg["action"].(string); ok && action == ActionPong {
		log.Debug().Msg("Received pong")
		return
	}

	// Check for action responses (subscription confirmations/errors)
	if action, ok := baseMsg["action"].(string); ok && action == "action" {
		// This is a response to a subscription request
		if errorCode, ok := baseMsg["errorCode"].(float64); ok && errorCode != 0 {
			// Non-zero error code - log but don't spam
			log.Debug().Float64("errorCode", errorCode).Msg("LBank subscription response")
		}
		return
	}

	// Determine message type from "type" field
	msgType, _ := baseMsg["type"].(string)

	switch msgType {
	case ChannelDepth:
		c.handleDepthMessage(message)
	case ChannelTrade:
		c.handleTradeMessage(message)
	case ChannelTick:
		c.handleTickerMessage(message)
	case ChannelKbar:
		c.handleKlineMessage(message)
	case "":
		// Empty type might be a different message format - ignore silently
		return
	default:
		log.Debug().Str("type", msgType).Msg("Unhandled message type")
	}
}

// handleDepthMessage processes orderbook depth updates
func (c *WsMarketDataClient) handleDepthMessage(message []byte) {
	if c.handler == nil || c.handler.OnDepth == nil {
		return
	}

	var resp WsDepthResponse
	if err := json.Unmarshal(message, &resp); err != nil {
		log.Error().Err(err).Msg("Failed to parse depth message")
		return
	}

	timestamp := parseTimestamp(resp.TS)
	c.handler.OnDepth(resp.Pair, resp.Depth.Asks, resp.Depth.Bids, timestamp)
}

// handleTradeMessage processes trade updates
func (c *WsMarketDataClient) handleTradeMessage(message []byte) {
	if c.handler == nil || c.handler.OnTrade == nil {
		return
	}

	var resp WsTradeResponse
	if err := json.Unmarshal(message, &resp); err != nil {
		log.Error().Err(err).Msg("Failed to parse trade message")
		return
	}

	timestamp := parseTimestamp(resp.Trade.TS)
	c.handler.OnTrade(resp.Pair, resp.Trade.Price, resp.Trade.Volume, resp.Trade.Direction, timestamp)
}

// handleTickerMessage processes ticker updates
func (c *WsMarketDataClient) handleTickerMessage(message []byte) {
	if c.handler == nil || c.handler.OnTicker == nil {
		return
	}

	var resp WsTickResponse
	if err := json.Unmarshal(message, &resp); err != nil {
		log.Error().Err(err).Msg("Failed to parse ticker message")
		return
	}

	c.handler.OnTicker(resp.Pair, &resp)
}

// handleKlineMessage processes kline updates
func (c *WsMarketDataClient) handleKlineMessage(message []byte) {
	if c.handler == nil || c.handler.OnKline == nil {
		return
	}

	var resp WsKbarResponse
	if err := json.Unmarshal(message, &resp); err != nil {
		log.Error().Err(err).Msg("Failed to parse kline message")
		return
	}

	c.handler.OnKline(resp.Pair, &resp)
}

// generatePingID generates a unique ping ID
func generatePingID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// parseTimestamp parses LBank timestamp format
func parseTimestamp(ts string) time.Time {
	// LBank uses format: "2019-06-28T17:49:22.722"
	t, err := time.Parse("2006-01-02T15:04:05.000", ts)
	if err != nil {
		// Try parsing as unix timestamp
		if ms, err := strconv.ParseInt(ts, 10, 64); err == nil {
			return time.UnixMilli(ms)
		}
		return time.Now()
	}
	return t
}

// Reconnect attempts to reconnect the WebSocket
func (c *WsMarketDataClient) Reconnect(ctx context.Context) error {
	c.Disconnect()
	time.Sleep(c.reconnectDelay)
	return c.Connect(ctx)
}
