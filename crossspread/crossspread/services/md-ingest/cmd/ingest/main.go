package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"crossspread-md-ingest/internal/connector"
	"crossspread-md-ingest/internal/connector/binance"
	"crossspread-md-ingest/internal/connector/bingx"
	"crossspread-md-ingest/internal/connector/bitget"
	"crossspread-md-ingest/internal/connector/bybit"
	"crossspread-md-ingest/internal/connector/coinex"
	gateio "crossspread-md-ingest/internal/connector/gate"
	"crossspread-md-ingest/internal/connector/htx"
	"crossspread-md-ingest/internal/connector/kucoin"
	"crossspread-md-ingest/internal/connector/lbank"
	"crossspread-md-ingest/internal/connector/mexc"
	"crossspread-md-ingest/internal/connector/okx"
	"crossspread-md-ingest/internal/credentials"
	"crossspread-md-ingest/internal/loader"
	"crossspread-md-ingest/internal/metrics"
	"crossspread-md-ingest/internal/normalizer"
	"crossspread-md-ingest/internal/publisher"
	"crossspread-md-ingest/internal/spread"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Global credentials fetcher
var credsFetcher *credentials.CredentialsFetcher

func main() {
	// Setup logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Load config from environment
	redisHost := getEnv("REDIS_HOST", "localhost")
	redisPort := getEnv("REDIS_PORT", "6379")
	metricsPort := getEnv("METRICS_PORT", "9090")
	enabledExchanges := getEnv("ENABLED_EXCHANGES", "binance,bybit,okx,kucoin,mexc,bitget,gateio,bingx,coinex,lbank,htx")
	useTwoPhase := getEnv("USE_TWO_PHASE", "true") == "true"
	backendAPIURL := getEnv("BACKEND_API_URL", "http://localhost:8000")
	serviceSecret := getEnv("SERVICE_SECRET", "default-dev-secret")
	minSpreadBps := 5.0 // Minimum spread in basis points

	// Initialize credentials fetcher
	credsFetcher = credentials.NewCredentialsFetcher(backendAPIURL, serviceSecret)

	log.Info().
		Str("redis", redisHost+":"+redisPort).
		Str("metrics", ":"+metricsPort).
		Str("exchanges", enabledExchanges).
		Bool("two_phase", useTwoPhase).
		Str("backend_api", backendAPIURL).
		Msg("Starting market data ingestion service")

	// Log credential status (after a short delay to let backend start)
	go func() {
		time.Sleep(5 * time.Second)
		logCredentialStatus()
	}()

	// Start metrics server
	metricsServer := metrics.NewServer(":" + metricsPort)
	go func() {
		if err := metricsServer.Start(); err != nil {
			log.Error().Err(err).Msg("Metrics server error")
		}
	}()

	// Create Redis publisher
	pub, err := publisher.NewRedisPublisher(redisHost + ":" + redisPort)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create Redis publisher")
	}
	defer pub.Close()

	// Create normalizer
	norm := normalizer.NewInstrumentNormalizer()

	// Default symbols to subscribe (perpetual futures) - used for legacy mode
	defaultSymbols := []string{
		"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT", "XRPUSDT",
		"DOGEUSDT", "ADAUSDT", "MATICUSDT", "AVAXUSDT", "DOTUSDT",
		"LTCUSDT", "LINKUSDT", "UNIUSDT", "ATOMUSDT", "ETCUSDT",
	}

	// Create exchange connectors based on enabled exchanges
	exchanges := strings.Split(enabledExchanges, ",")
	connectors := make([]connector.Connector, 0)

	for _, ex := range exchanges {
		ex = strings.TrimSpace(strings.ToLower(ex))
		switch ex {
		case "binance":
			conn := binance.NewBinanceConnector(defaultSymbols, 20)
			connectors = append(connectors, conn)
			log.Info().Msg("Added Binance connector")

		case "bybit":
			conn := bybit.NewBybitConnector(defaultSymbols, 50)
			connectors = append(connectors, conn)
			log.Info().Msg("Added Bybit connector")

		case "okx":
			// Convert to OKX format: BTCUSDT -> BTC-USDT-SWAP
			okxSymbols := make([]string, len(defaultSymbols))
			for i, s := range defaultSymbols {
				okxSymbols[i] = convertToOKXSymbol(s)
			}
			conn := okx.NewOKXConnector(okxSymbols, 5)
			connectors = append(connectors, conn)
			log.Info().Msg("Added OKX connector")

		case "kucoin":
			// Convert to KuCoin format: BTCUSDT -> XBTUSDTM
			kucoinSymbols := make([]string, len(defaultSymbols))
			for i, s := range defaultSymbols {
				kucoinSymbols[i] = convertToKuCoinSymbol(s)
			}
			conn := kucoin.NewKuCoinConnector(kucoinSymbols, 20)
			connectors = append(connectors, conn)

			// Check if credentials are available (for future authenticated endpoint support)
			if creds := getCredentialsForExchange("kucoin"); creds != nil {
				log.Info().Msg("Added KuCoin connector (credentials available for future use)")
			} else {
				log.Info().Msg("Added KuCoin connector (public endpoints only)")
			}

		case "mexc":
			// Convert to MEXC format: BTCUSDT -> BTC_USDT
			mexcSymbols := make([]string, len(defaultSymbols))
			for i, s := range defaultSymbols {
				mexcSymbols[i] = convertToMEXCSymbol(s)
			}

			// Try to use credentials if available
			var conn connector.Connector
			if creds := getCredentialsForExchange("mexc"); creds != nil {
				conn = mexc.NewMEXCConnectorWithCredentials(mexcSymbols, 20, creds.APIKey, creds.APISecret)
				log.Info().Msg("Added MEXC connector with API credentials")
			} else {
				conn = mexc.NewMEXCConnector(mexcSymbols, 20)
				log.Info().Msg("Added MEXC connector (public endpoints only)")
			}
			connectors = append(connectors, conn)

		case "bitget":
			// Bitget uses BTCUSDT format
			conn := bitget.NewBitgetConnector(defaultSymbols, 20)
			connectors = append(connectors, conn)
			log.Info().Msg("Added Bitget connector")

		case "gateio":
			// Convert to Gate.io format: BTCUSDT -> BTC_USDT
			gateSymbols := make([]string, len(defaultSymbols))
			for i, s := range defaultSymbols {
				gateSymbols[i] = convertToGateSymbol(s)
			}

			// Try to use credentials if available
			var conn connector.Connector
			if creds := getCredentialsForExchange("gateio"); creds != nil {
				conn = gateio.NewGateConnectorWithCredentials(gateSymbols, 20, "usdt", creds.APIKey, creds.APISecret)
				log.Info().Msg("Added Gate.io connector with API credentials")
			} else {
				conn = gateio.NewGateConnector(gateSymbols, 20, "usdt")
				log.Info().Msg("Added Gate.io connector (public endpoints only)")
			}
			connectors = append(connectors, conn)

		case "bingx":
			// Convert to BingX format: BTCUSDT -> BTC-USDT
			bingxSymbols := make([]string, len(defaultSymbols))
			for i, s := range defaultSymbols {
				bingxSymbols[i] = convertToBingXSymbol(s)
			}

			// Try to use credentials if available
			var conn connector.Connector
			if creds := getCredentialsForExchange("bingx"); creds != nil {
				conn = bingx.NewBingXConnectorWithCredentials(bingxSymbols, 20, creds.APIKey, creds.APISecret)
				log.Info().Msg("Added BingX connector with API credentials")
			} else {
				conn = bingx.NewBingXConnector(bingxSymbols, 20)
				log.Info().Msg("Added BingX connector (public endpoints only)")
			}
			connectors = append(connectors, conn)

		case "coinex":
			// CoinEx uses BTCUSD format
			coinexSymbols := make([]string, len(defaultSymbols))
			for i, s := range defaultSymbols {
				coinexSymbols[i] = convertToCoinExSymbol(s)
			}
			conn := coinex.NewCoinExConnector(coinexSymbols, 20)
			connectors = append(connectors, conn)
			log.Info().Msg("Added CoinEx connector")

		case "lbank":
			// Convert to LBank format: BTCUSDT -> BTC_USDT
			lbankSymbols := make([]string, len(defaultSymbols))
			for i, s := range defaultSymbols {
				lbankSymbols[i] = convertToLBankSymbol(s)
			}
			conn := lbank.NewLBankConnector(lbankSymbols, 20)
			connectors = append(connectors, conn)
			log.Info().Msg("Added LBank connector")

		case "htx":
			// Convert to HTX format: BTCUSDT -> BTC-USDT
			htxSymbols := make([]string, len(defaultSymbols))
			for i, s := range defaultSymbols {
				htxSymbols[i] = convertToHTXSymbol(s)
			}
			conn := htx.NewHTXConnector(htxSymbols, 20)
			connectors = append(connectors, conn)
			log.Info().Msg("Added HTX connector")

		default:
			log.Warn().Str("exchange", ex).Msg("Unknown exchange, skipping")
		}
	}

	if len(connectors) == 0 {
		log.Fatal().Msg("No exchange connectors enabled")
	}

	// Create spread discovery service
	spreadDiscovery := spread.NewSpreadDiscovery(norm, pub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start spread discovery service
	go spreadDiscovery.Start(ctx)

	if useTwoPhase {
		// ========================================
		// TWO-PHASE APPROACH (Recommended)
		// ========================================
		log.Info().Msg("Using two-phase approach: REST first, then selective WebSocket")

		// PHASE 1: Load all data from REST APIs
		restLoader := loader.NewRestDataLoader(connectors)
		restLoader.SetMinSpreadBps(minSpreadBps)

		if err := restLoader.LoadAll(ctx); err != nil {
			log.Fatal().Err(err).Msg("Failed to load REST data in Phase 1")
		}

		// Get discovered spreads from REST data
		discoveredSpreads := restLoader.GetDiscoveredSpreads()
		log.Info().
			Int("spreads", len(discoveredSpreads)).
			Msg("Phase 1 complete: preliminary spreads discovered")

		// Publish preliminary spreads
		for _, sp := range discoveredSpreads {
			log.Debug().
				Str("canonical", sp.Canonical).
				Str("long", string(sp.LongExchange)).
				Str("short", string(sp.ShortExchange)).
				Float64("spread_bps", sp.SpreadBps).
				Msg("Preliminary spread found")
		}

		// Get symbols that need WebSocket subscription
		symbolsByExchange := restLoader.GetSymbolsForWebSocket()

		totalSymbols := 0
		for exchID, symbols := range symbolsByExchange {
			log.Info().
				Str("exchange", string(exchID)).
				Int("symbols", len(symbols)).
				Msg("Symbols requiring WebSocket subscription")
			totalSymbols += len(symbols)
		}

		if totalSymbols == 0 {
			log.Warn().Msg("No spreads found, no WebSocket connections needed")
			log.Info().Msg("Starting periodic REST refresh to check for new spreads")
			restLoader.StartPeriodicRefresh(ctx)
		} else {
			// PHASE 2: Connect WebSocket for discovered spreads only
			wsManager := loader.NewWebSocketManager(connectors)

			// Setup handlers
			wsManager.SetOrderbookHandler(func(ob *connector.Orderbook) {
				if err := pub.PublishOrderbook(ob); err != nil {
					log.Error().Err(err).Msg("Failed to publish orderbook")
				}
				spreadDiscovery.HandleOrderbook(ob)
			})

			wsManager.SetFundingHandler(func(fr *connector.FundingRate) {
				spreadDiscovery.HandleFundingRate(fr)
			})

			wsManager.SetErrorHandler(func(err error) {
				log.Error().Err(err).Msg("WebSocket error")
			})

			// Connect WebSocket only for spread symbols
			if err := wsManager.ConnectForSpreads(ctx, symbolsByExchange); err != nil {
				log.Error().Err(err).Msg("Some WebSocket connections failed")
			}

			log.Info().
				Int("total_symbols", wsManager.GetTotalSymbolCount()).
				Int("connected_exchanges", len(wsManager.GetConnectedExchanges())).
				Msg("Phase 2 complete: WebSocket connections established for spreads")

			// Start connection monitor
			go wsManager.MonitorConnections(ctx, 30*time.Second)

			// Start periodic REST refresh for new spread discovery
			restLoader.StartPeriodicRefresh(ctx)

			// Wait for shutdown signal
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh

			log.Info().Msg("Shutting down...")

			// Stop WebSocket manager
			wsManager.DisconnectAll()
		}
	} else {
		// ========================================
		// LEGACY MODE: Connect to all symbols immediately
		// ========================================
		log.Info().Msg("Using legacy mode: connecting to all symbols via WebSocket")

		// Setup handlers and connect
		for _, conn := range connectors {
			setupHandlers(conn, pub, spreadDiscovery)

			if err := conn.Connect(ctx); err != nil {
				log.Error().Err(err).Str("exchange", string(conn.ID())).Msg("Failed to connect")
				metrics.RecordConnectionError(string(conn.ID()), "connect_failed")
				continue
			}

			metrics.RecordConnectionStatus(string(conn.ID()), true)
			log.Info().Str("exchange", string(conn.ID())).Msg("Connected to exchange")
		}

		// Wait for shutdown signal
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
	}

	log.Info().Msg("Cleaning up...")

	// Stop spread discovery
	spreadDiscovery.Stop()

	// Disconnect all (in case legacy mode was used)
	for _, conn := range connectors {
		if conn.IsConnected() {
			metrics.RecordConnectionStatus(string(conn.ID()), false)
			if err := conn.Disconnect(); err != nil {
				log.Error().Err(err).Str("exchange", string(conn.ID())).Msg("Error disconnecting")
			}
		}
	}

	// Stop metrics server
	if err := metricsServer.Stop(); err != nil {
		log.Error().Err(err).Msg("Error stopping metrics server")
	}
}

// convertToOKXSymbol converts Binance-style symbols to OKX format
// BTCUSDT -> BTC-USDT-SWAP
func convertToOKXSymbol(symbol string) string {
	// Handle common USDT pairs
	if strings.HasSuffix(symbol, "USDT") {
		base := strings.TrimSuffix(symbol, "USDT")
		return base + "-USDT-SWAP"
	}
	// Handle USDC pairs
	if strings.HasSuffix(symbol, "USDC") {
		base := strings.TrimSuffix(symbol, "USDC")
		return base + "-USDC-SWAP"
	}
	// Handle BUSD pairs
	if strings.HasSuffix(symbol, "BUSD") {
		base := strings.TrimSuffix(symbol, "BUSD")
		return base + "-BUSD-SWAP"
	}
	return symbol + "-USDT-SWAP"
}

// convertToKuCoinSymbol converts Binance-style symbols to KuCoin format
// BTCUSDT -> XBTUSDTM (BTC is XBT on KuCoin futures)
func convertToKuCoinSymbol(symbol string) string {
	if strings.HasSuffix(symbol, "USDT") {
		base := strings.TrimSuffix(symbol, "USDT")
		if base == "BTC" {
			base = "XBT"
		}
		return base + "USDTM"
	}
	return symbol + "M"
}

// convertToMEXCSymbol converts Binance-style symbols to MEXC format
// BTCUSDT -> BTC_USDT
func convertToMEXCSymbol(symbol string) string {
	if strings.HasSuffix(symbol, "USDT") {
		base := strings.TrimSuffix(symbol, "USDT")
		return base + "_USDT"
	}
	if strings.HasSuffix(symbol, "USDC") {
		base := strings.TrimSuffix(symbol, "USDC")
		return base + "_USDC"
	}
	return symbol
}

// convertToGateSymbol converts Binance-style symbols to Gate.io format
// BTCUSDT -> BTC_USDT
func convertToGateSymbol(symbol string) string {
	if strings.HasSuffix(symbol, "USDT") {
		base := strings.TrimSuffix(symbol, "USDT")
		return base + "_USDT"
	}
	if strings.HasSuffix(symbol, "USDC") {
		base := strings.TrimSuffix(symbol, "USDC")
		return base + "_USDC"
	}
	return symbol
}

// convertToBingXSymbol converts Binance-style symbols to BingX format
// BTCUSDT -> BTC-USDT
func convertToBingXSymbol(symbol string) string {
	if strings.HasSuffix(symbol, "USDT") {
		base := strings.TrimSuffix(symbol, "USDT")
		return base + "-USDT"
	}
	if strings.HasSuffix(symbol, "USDC") {
		base := strings.TrimSuffix(symbol, "USDC")
		return base + "-USDC"
	}
	return symbol
}

// convertToCoinExSymbol converts Binance-style symbols to CoinEx format
// BTCUSDT -> BTCUSDT (CoinEx uses same format)
func convertToCoinExSymbol(symbol string) string {
	return symbol
}

// convertToLBankSymbol converts Binance-style symbols to LBank format
// BTCUSDT -> BTC_USDT
func convertToLBankSymbol(symbol string) string {
	if strings.HasSuffix(symbol, "USDT") {
		base := strings.TrimSuffix(symbol, "USDT")
		return base + "_USDT"
	}
	if strings.HasSuffix(symbol, "USDC") {
		base := strings.TrimSuffix(symbol, "USDC")
		return base + "_USDC"
	}
	return symbol
}

// convertToHTXSymbol converts Binance-style symbols to HTX format
// BTCUSDT -> BTC-USDT
func convertToHTXSymbol(symbol string) string {
	if strings.HasSuffix(symbol, "USDT") {
		base := strings.TrimSuffix(symbol, "USDT")
		return base + "-USDT"
	}
	if strings.HasSuffix(symbol, "USDC") {
		base := strings.TrimSuffix(symbol, "USDC")
		return base + "-USDC"
	}
	return symbol
}

func setupHandlers(conn connector.Connector, pub *publisher.RedisPublisher, sd *spread.SpreadDiscovery) {
	exchangeID := string(conn.ID())

	conn.SetOrderbookHandler(func(ob *connector.Orderbook) {
		timer := metrics.NewTimer()
		if err := pub.PublishOrderbook(ob); err != nil {
			log.Error().Err(err).Msg("Failed to publish orderbook")
			metrics.RedisPublishErrors.WithLabelValues("orderbook").Inc()
		} else {
			timer.ObserveDuration(metrics.RedisPublishDuration, "orderbook")

			// Record orderbook metrics
			bestBid := ob.BestBid
			bestAsk := ob.BestAsk
			if len(ob.Bids) > 0 && bestBid == 0 {
				bestBid = ob.Bids[0].Price
			}
			if len(ob.Asks) > 0 && bestAsk == 0 {
				bestAsk = ob.Asks[0].Price
			}
			metrics.RecordOrderbookUpdate(exchangeID, ob.Symbol, len(ob.Bids), len(ob.Asks), bestBid, bestAsk)

			// Forward to spread discovery
			sd.HandleOrderbook(ob)
		}
	})

	conn.SetTradeHandler(func(trade *connector.Trade) {
		if err := pub.PublishTrade(trade); err != nil {
			log.Error().Err(err).Msg("Failed to publish trade")
			metrics.RedisPublishErrors.WithLabelValues("trade").Inc()
		} else {
			metrics.RecordTrade(exchangeID, trade.Symbol, trade.Side, trade.Quantity)
		}
	})

	conn.SetFundingHandler(func(fr *connector.FundingRate) {
		// Forward to spread discovery
		sd.HandleFundingRate(fr)
		metrics.RecordFundingRate(exchangeID, fr.Symbol, fr.FundingRate)
	})

	conn.SetErrorHandler(func(err error) {
		log.Error().Err(err).Str("exchange", exchangeID).Msg("Connector error")
		metrics.RecordConnectionError(exchangeID, "runtime_error")
	})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getCredentialsForExchange tries to fetch API credentials for an exchange
// Returns nil if no credentials are found or if fetching fails
func getCredentialsForExchange(exchange string) *credentials.ExchangeCredentials {
	if credsFetcher == nil {
		return nil
	}

	creds, err := credsFetcher.GetFirstCredentials(exchange)
	if err != nil {
		log.Debug().Str("exchange", exchange).Msg("No API credentials available, using public endpoints only")
		return nil
	}

	log.Info().Str("exchange", exchange).Msg("Found API credentials, will use authenticated endpoints")
	return creds
}

// logCredentialStatus logs which exchanges have credentials configured
func logCredentialStatus() {
	if credsFetcher == nil {
		log.Warn().Msg("Credentials fetcher not initialized, running without authenticated endpoints")
		return
	}

	allCreds, err := credsFetcher.GetAllCredentials()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to fetch credentials from backend API")
		return
	}

	if len(allCreds) == 0 {
		log.Info().Msg("No API credentials configured. Add credentials via the web interface to enable authenticated endpoints.")
		return
	}

	for exchange, creds := range allCreds {
		log.Info().
			Str("exchange", exchange).
			Int("credential_count", len(creds)).
			Msg("API credentials available")
	}
}
