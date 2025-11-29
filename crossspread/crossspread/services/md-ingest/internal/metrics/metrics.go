package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

// Metrics for the market data ingestion service
var (
	// Orderbook metrics
	OrderbookUpdates = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "md_orderbook_updates_total",
			Help: "Total number of orderbook updates received",
		},
		[]string{"exchange", "symbol"},
	)

	OrderbookDepth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "md_orderbook_depth",
			Help: "Current orderbook depth (number of levels)",
		},
		[]string{"exchange", "symbol", "side"},
	)

	OrderbookBestBid = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "md_orderbook_best_bid",
			Help: "Current best bid price",
		},
		[]string{"exchange", "symbol"},
	)

	OrderbookBestAsk = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "md_orderbook_best_ask",
			Help: "Current best ask price",
		},
		[]string{"exchange", "symbol"},
	)

	OrderbookSpread = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "md_orderbook_spread_bps",
			Help: "Current bid-ask spread in basis points",
		},
		[]string{"exchange", "symbol"},
	)

	// Trade metrics
	TradeCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "md_trades_total",
			Help: "Total number of trades received",
		},
		[]string{"exchange", "symbol", "side"},
	)

	TradeVolume = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "md_trade_volume_total",
			Help: "Total trade volume",
		},
		[]string{"exchange", "symbol"},
	)

	// Latency metrics
	MessageLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "md_message_latency_seconds",
			Help:    "Latency from exchange timestamp to processing",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		},
		[]string{"exchange", "message_type"},
	)

	ProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "md_processing_duration_seconds",
			Help:    "Time to process and publish a message",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
		},
		[]string{"exchange", "message_type"},
	)

	// Connection metrics
	ConnectionStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "md_connection_status",
			Help: "WebSocket connection status (1=connected, 0=disconnected)",
		},
		[]string{"exchange"},
	)

	ConnectionReconnects = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "md_reconnects_total",
			Help: "Total number of reconnection attempts",
		},
		[]string{"exchange"},
	)

	ConnectionErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "md_connection_errors_total",
			Help: "Total number of connection errors",
		},
		[]string{"exchange", "error_type"},
	)

	// Spread discovery metrics
	SpreadsDiscovered = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "md_spreads_discovered_total",
			Help: "Total number of spreads discovered",
		},
		[]string{"symbol"},
	)

	SpreadValue = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "md_spread_value_bps",
			Help: "Current spread value in basis points",
		},
		[]string{"symbol", "long_exchange", "short_exchange"},
	)

	SpreadSlippage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "md_spread_slippage_bps",
			Help: "Estimated slippage for spread entry",
		},
		[]string{"symbol", "long_exchange", "short_exchange"},
	)

	// Redis metrics
	RedisPublishDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "md_redis_publish_duration_seconds",
			Help:    "Time to publish message to Redis",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05},
		},
		[]string{"channel"},
	)

	RedisPublishErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "md_redis_publish_errors_total",
			Help: "Total number of Redis publish errors",
		},
		[]string{"channel"},
	)

	// REST API metrics (for two-phase approach)
	RestFetchDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "md_rest_fetch_duration_seconds",
			Help:    "Time to fetch data from exchange REST API",
			Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"exchange", "endpoint"},
	)

	RestFetchErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "md_rest_fetch_errors_total",
			Help: "Total number of REST API fetch errors",
		},
		[]string{"exchange", "endpoint"},
	)

	SpreadDiscoveryDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "md_spread_discovery_duration_seconds",
			Help:    "Time to discover spreads from REST data",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1},
		},
	)

	PreliminarySpreadsFound = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "md_preliminary_spreads_found",
			Help: "Number of preliminary spreads found from REST data",
		},
	)

	WebsocketSymbolsSubscribed = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "md_websocket_symbols_subscribed",
			Help: "Number of symbols subscribed via WebSocket (selective mode)",
		},
		[]string{"exchange"},
	)

	// Instrument metrics
	InstrumentsLoaded = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "md_instruments_loaded",
			Help: "Number of instruments loaded per exchange",
		},
		[]string{"exchange"},
	)

	InstrumentsSubscribed = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "md_instruments_subscribed",
			Help: "Number of instruments subscribed per exchange",
		},
		[]string{"exchange"},
	)

	// Funding rate metrics
	FundingRate = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "md_funding_rate",
			Help: "Current funding rate",
		},
		[]string{"exchange", "symbol"},
	)

	FundingRateUpdates = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "md_funding_rate_updates_total",
			Help: "Total number of funding rate updates",
		},
		[]string{"exchange"},
	)
)

// Timer is a helper for measuring operation duration
type Timer struct {
	start time.Time
}

// NewTimer creates a new timer
func NewTimer() *Timer {
	return &Timer{start: time.Now()}
}

// ObserveDuration records the elapsed time to a histogram
func (t *Timer) ObserveDuration(histogram *prometheus.HistogramVec, labels ...string) {
	histogram.WithLabelValues(labels...).Observe(time.Since(t.start).Seconds())
}

// RecordOrderbookUpdate records metrics for an orderbook update
func RecordOrderbookUpdate(exchange, symbol string, bidDepth, askDepth int, bestBid, bestAsk float64) {
	OrderbookUpdates.WithLabelValues(exchange, symbol).Inc()
	OrderbookDepth.WithLabelValues(exchange, symbol, "bid").Set(float64(bidDepth))
	OrderbookDepth.WithLabelValues(exchange, symbol, "ask").Set(float64(askDepth))

	if bestBid > 0 {
		OrderbookBestBid.WithLabelValues(exchange, symbol).Set(bestBid)
	}
	if bestAsk > 0 {
		OrderbookBestAsk.WithLabelValues(exchange, symbol).Set(bestAsk)
	}

	if bestBid > 0 && bestAsk > 0 {
		midPrice := (bestBid + bestAsk) / 2
		spreadBps := (bestAsk - bestBid) / midPrice * 10000
		OrderbookSpread.WithLabelValues(exchange, symbol).Set(spreadBps)
	}
}

// RecordTrade records metrics for a trade
func RecordTrade(exchange, symbol, side string, volume float64) {
	TradeCount.WithLabelValues(exchange, symbol, side).Inc()
	TradeVolume.WithLabelValues(exchange, symbol).Add(volume)
}

// RecordSpread records metrics for a discovered spread
func RecordSpread(symbol, longExchange, shortExchange string, spreadBps, slippageBps float64) {
	SpreadsDiscovered.WithLabelValues(symbol).Inc()
	SpreadValue.WithLabelValues(symbol, longExchange, shortExchange).Set(spreadBps)
	SpreadSlippage.WithLabelValues(symbol, longExchange, shortExchange).Set(slippageBps)
}

// RecordConnectionStatus records connection status
func RecordConnectionStatus(exchange string, connected bool) {
	status := 0.0
	if connected {
		status = 1.0
	}
	ConnectionStatus.WithLabelValues(exchange).Set(status)
}

// RecordReconnect records a reconnection attempt
func RecordReconnect(exchange string) {
	ConnectionReconnects.WithLabelValues(exchange).Inc()
}

// RecordConnectionError records a connection error
func RecordConnectionError(exchange, errorType string) {
	ConnectionErrors.WithLabelValues(exchange, errorType).Inc()
}

// RecordFundingRate records a funding rate update
func RecordFundingRate(exchange, symbol string, rate float64) {
	FundingRate.WithLabelValues(exchange, symbol).Set(rate)
	FundingRateUpdates.WithLabelValues(exchange).Inc()
}

// Server starts the Prometheus metrics HTTP server
type Server struct {
	addr   string
	server *http.Server
}

// NewServer creates a new metrics server
func NewServer(addr string) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	return &Server{
		addr: addr,
		server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}
}

// Start starts the metrics server
func (s *Server) Start() error {
	log.Info().Str("addr", s.addr).Msg("Starting metrics server")
	return s.server.ListenAndServe()
}

// Stop stops the metrics server gracefully
func (s *Server) Stop() error {
	return s.server.Close()
}
