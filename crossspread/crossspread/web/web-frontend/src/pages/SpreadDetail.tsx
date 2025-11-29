import { useState, useEffect, useRef } from 'react';
import { useParams } from 'react-router-dom';
import { useQuery, useMutation } from '@tanstack/react-query';
import { createChart, IChartApi, ISeriesApi, CandlestickData, Time } from 'lightweight-charts';
import toast from 'react-hot-toast';
import { 
  getSpreadDetail, 
  getSpreadCandles,
  calculateSlippage, 
  enterTrade,
  getAssetInfo,
  SlippageResult,
  AssetInfo,
  CandleData 
} from '../api';
import { subscribeToSpread, subscribeToOrderbook, SpreadUpdate, OrderbookUpdate } from '../api/socket';
import clsx from 'clsx';

interface OrderbookLevel {
  price: number;
  size: number;
}

interface DepositWithdrawStatus {
  exchange: string;
  depositEnabled: boolean;
  withdrawEnabled: boolean;
  minDeposit?: number;
  minWithdraw?: number;
  withdrawFee?: number;
  network?: string;
  lastUpdated: Date;
}

type CandleInterval = '1m' | '5m' | '15m' | '1h' | '4h' | '1d';

export default function SpreadDetail() {
  const { spreadId } = useParams<{ spreadId: string }>();
  const chartContainerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<IChartApi | null>(null);
  const candleSeriesRef = useRef<ISeriesApi<'Candlestick'> | null>(null);

  const [sizeInCoins, setSizeInCoins] = useState('0.01');
  const [sliceSize, setSliceSize] = useState('0.001');
  const [intervalMs, setIntervalMs] = useState('100');
  const [mode, setMode] = useState<'sim' | 'live'>('sim');
  const [slippage, setSlippage] = useState<SlippageResult | null>(null);
  const [longOrderbook, setLongOrderbook] = useState<{ bids: OrderbookLevel[]; asks: OrderbookLevel[] }>({ bids: [], asks: [] });
  const [shortOrderbook, setShortOrderbook] = useState<{ bids: OrderbookLevel[]; asks: OrderbookLevel[] }>({ bids: [], asks: [] });
  const [candleInterval, setCandleInterval] = useState<CandleInterval>('1m');
  const [currentCandle, setCurrentCandle] = useState<CandlestickData | null>(null);
  const [realtimeSpread, setRealtimeSpread] = useState<{
    spreadPercent: number;
    longPrice: number;
    shortPrice: number;
  } | null>(null);

  const { data: spread, isLoading } = useQuery({
    queryKey: ['spread', spreadId],
    queryFn: () => getSpreadDetail(spreadId!),
    enabled: !!spreadId,
    refetchInterval: 2000, // Faster refresh for real-time
  });

  // Fetch OHLC candles instead of raw history
  const { data: candles, refetch: refetchCandles } = useQuery({
    queryKey: ['spreadCandles', spreadId, candleInterval],
    queryFn: () => getSpreadCandles(spreadId!, candleInterval),
    enabled: !!spreadId,
    refetchInterval: 3000, // Refetch every 3 seconds for real-time updates
  });

  const slippageMutation = useMutation({
    mutationFn: () => calculateSlippage(spreadId!, parseFloat(sizeInCoins)),
    onSuccess: (data) => setSlippage(data),
    onError: () => toast.error('Failed to calculate slippage'),
  });

  const tradeMutation = useMutation({
    mutationFn: () =>
      enterTrade({
        spreadId: spreadId!,
        sizeInCoins: parseFloat(sizeInCoins),
        slicing: {
          sliceSizeInCoins: parseFloat(sliceSize),
          intervalMs: parseInt(intervalMs, 10),
        },
        mode,
      }),
    onSuccess: () => toast.success('Trade submitted successfully'),
    onError: () => toast.error('Failed to submit trade'),
  });

  // Initialize candlestick chart
  useEffect(() => {
    if (!chartContainerRef.current) return;

    chartRef.current = createChart(chartContainerRef.current, {
      width: chartContainerRef.current.clientWidth,
      height: 350,
      layout: {
        background: { color: '#0f1419' },
        textColor: '#94a3b8',
      },
      grid: {
        vertLines: { color: '#1e293b' },
        horzLines: { color: '#1e293b' },
      },
      rightPriceScale: {
        borderColor: '#1e293b',
        scaleMargins: {
          top: 0.1,
          bottom: 0.1,
        },
      },
      timeScale: {
        borderColor: '#1e293b',
        timeVisible: true,
        secondsVisible: false,
      },
      crosshair: {
        mode: 1, // Normal crosshair
        vertLine: {
          color: '#475569',
          width: 1,
          style: 2, // Dashed
          labelBackgroundColor: '#1e293b',
        },
        horzLine: {
          color: '#475569',
          width: 1,
          style: 2,
          labelBackgroundColor: '#1e293b',
        },
      },
    });

    // Add candlestick series
    candleSeriesRef.current = chartRef.current.addCandlestickSeries({
      upColor: '#22c55e',
      downColor: '#ef4444',
      borderUpColor: '#22c55e',
      borderDownColor: '#ef4444',
      wickUpColor: '#22c55e',
      wickDownColor: '#ef4444',
    });

    // Handle resize
    const handleResize = () => {
      if (chartContainerRef.current && chartRef.current) {
        chartRef.current.applyOptions({
          width: chartContainerRef.current.clientWidth,
        });
      }
    };

    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      chartRef.current?.remove();
    };
  }, []);

  // Update chart with candle data
  useEffect(() => {
    if (!candleSeriesRef.current || !candles || candles.length === 0) return;

    const chartData: CandlestickData[] = candles.map((c: CandleData) => ({
      time: c.time as Time,
      open: c.open,
      high: c.high,
      low: c.low,
      close: c.close,
    }));

    candleSeriesRef.current.setData(chartData);
    
    // Store the last candle for real-time updates
    if (chartData.length > 0) {
      setCurrentCandle(chartData[chartData.length - 1]);
    }
    
    // Fit content
    chartRef.current?.timeScale().fitContent();
  }, [candles]);

  // Subscribe to real-time updates
  useEffect(() => {
    if (!spreadId || !spread) return;

    const unsubSpread = subscribeToSpread(spreadId, (update: SpreadUpdate) => {
      // Update realtime spread display
      setRealtimeSpread({
        spreadPercent: update.spreadPercent,
        longPrice: update.longPrice,
        shortPrice: update.shortPrice,
      });
      
      // Update the current candle in real-time
      if (candleSeriesRef.current && currentCandle) {
        const now = Math.floor(Date.now() / 1000);
        const intervalSeconds = getIntervalSeconds(candleInterval);
        const candleTime = Math.floor(now / intervalSeconds) * intervalSeconds;
        
        // If we're in the same candle period, update it
        if (candleTime === currentCandle.time) {
          const updatedCandle: CandlestickData = {
            time: currentCandle.time,
            open: currentCandle.open,
            high: Math.max(currentCandle.high, update.spreadPercent),
            low: Math.min(currentCandle.low, update.spreadPercent),
            close: update.spreadPercent,
          };
          candleSeriesRef.current.update(updatedCandle);
          setCurrentCandle(updatedCandle);
        } else {
          // New candle period - refetch candles
          refetchCandles();
        }
      }
    });

    const unsubOrderbook = subscribeToOrderbook(
      spread.symbol,
      [spread.longExchange, spread.shortExchange],
      (update: OrderbookUpdate) => {
        if (update.exchange === spread.longExchange) {
          setLongOrderbook({ bids: update.bids, asks: update.asks });
        } else if (update.exchange === spread.shortExchange) {
          setShortOrderbook({ bids: update.bids, asks: update.asks });
        }
      }
    );

    return () => {
      unsubSpread();
      unsubOrderbook();
    };
  }, [spreadId, spread, currentCandle, candleInterval, refetchCandles]);

  // Initialize orderbooks from spread data
  useEffect(() => {
    if (spread) {
      setLongOrderbook(spread.longOrderbook || { bids: [], asks: [] });
      setShortOrderbook(spread.shortOrderbook || { bids: [], asks: [] });
    }
  }, [spread]);

  // Helper function to convert interval to seconds
  const getIntervalSeconds = (interval: CandleInterval): number => {
    const intervals: Record<CandleInterval, number> = {
      '1m': 60,
      '5m': 300,
      '15m': 900,
      '1h': 3600,
      '4h': 14400,
      '1d': 86400,
    };
    return intervals[interval];
  };

  if (isLoading) {
    return <div className="text-center py-8 text-gray-500">Loading spread...</div>;
  }

  if (!spread) {
    return <div className="text-center py-8 text-danger">Spread not found</div>;
  }

  // Use realtime values when available, otherwise use fetched spread data
  const displaySpread = realtimeSpread?.spreadPercent ?? spread.spreadPercent;
  const displayLongPrice = realtimeSpread?.longPrice ?? spread.longPrice;
  const displayShortPrice = realtimeSpread?.shortPrice ?? spread.shortPrice;

  const intervalOptions: CandleInterval[] = ['1m', '5m', '15m', '1h', '4h', '1d'];

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="bg-dark-300 p-4 rounded-lg border border-dark-100">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-bold">{spread.symbol}</h1>
            <p className="text-gray-400 text-sm">
              Long: {spread.longExchange} • Short: {spread.shortExchange}
            </p>
          </div>
          <div className="text-right">
            <div
              className={clsx(
                'text-3xl font-bold transition-colors',
                displaySpread > 0 ? 'text-success' : 'text-danger'
              )}
            >
              {displaySpread.toFixed(4)}%
            </div>
            <p className="text-gray-400 text-sm">Current Spread</p>
          </div>
        </div>
        {/* Prices row */}
        <div className="flex items-center gap-8 mt-4 pt-4 border-t border-dark-100">
          <div>
            <span className="text-gray-500 text-xs block">Long Price ({spread.longExchange})</span>
            <span className="text-lg font-medium text-success">${displayLongPrice.toLocaleString(undefined, {minimumFractionDigits: 2, maximumFractionDigits: 6})}</span>
          </div>
          <div>
            <span className="text-gray-500 text-xs block">Short Price ({spread.shortExchange})</span>
            <span className="text-lg font-medium text-danger">${displayShortPrice.toLocaleString(undefined, {minimumFractionDigits: 2, maximumFractionDigits: 6})}</span>
          </div>
          <div>
            <span className="text-gray-500 text-xs block">Funding (L / S)</span>
            <span className="text-sm">
              <span className={spread.fundingLong >= 0 ? 'text-success' : 'text-danger'}>
                {((spread.fundingLong ?? 0) * 100).toFixed(4)}%
              </span>
              {' / '}
              <span className={spread.fundingShort >= 0 ? 'text-success' : 'text-danger'}>
                {((spread.fundingShort ?? 0) * 100).toFixed(4)}%
              </span>
            </span>
          </div>
          <div>
            <span className="text-gray-500 text-xs block">Min Depth</span>
            <span className="text-sm text-gray-300">${((spread.minDepthUsd ?? 0) / 1000).toFixed(1)}k</span>
          </div>
        </div>
      </div>

      <div className="grid grid-cols-3 gap-4">
        {/* Chart with Candlesticks */}
        <div className="col-span-2 bg-dark-300 p-4 rounded-lg border border-dark-100">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-medium text-gray-400">Spread Chart</h2>
            {/* Interval Selector */}
            <div className="flex gap-1">
              {intervalOptions.map((interval) => (
                <button
                  key={interval}
                  onClick={() => setCandleInterval(interval)}
                  className={clsx(
                    'px-2 py-1 text-xs rounded transition-colors',
                    candleInterval === interval
                      ? 'bg-primary-600 text-white'
                      : 'bg-dark-200 text-gray-400 hover:bg-dark-100'
                  )}
                >
                  {interval}
                </button>
              ))}
            </div>
          </div>
          <div ref={chartContainerRef} className="w-full h-[350px]" />
          {/* Chart Legend */}
          <div className="flex items-center gap-4 mt-2 text-xs text-gray-500">
            <span className="flex items-center gap-1">
              <span className="w-3 h-3 bg-success rounded-sm"></span> Bullish
            </span>
            <span className="flex items-center gap-1">
              <span className="w-3 h-3 bg-danger rounded-sm"></span> Bearish
            </span>
            <span className="ml-auto">
              {candles?.length === 1 ? (
                <span className="text-warning">No historical data - showing real-time only</span>
              ) : (
                `${candles?.length || 0} candles loaded`
              )}
            </span>
          </div>
        </div>

        {/* Trade Ticket */}
        <div className="bg-dark-300 p-4 rounded-lg border border-dark-100 space-y-4">
          <h2 className="text-sm font-medium text-gray-400">Trade Ticket</h2>
          
          <div>
            <label className="block text-xs text-gray-500 mb-1">Size (coins)</label>
            <input
              type="number"
              value={sizeInCoins}
              onChange={(e) => setSizeInCoins(e.target.value)}
              className="w-full px-3 py-2 bg-dark-400 border border-dark-100 rounded text-sm focus:outline-none focus:border-primary-500"
              step="0.001"
            />
          </div>

          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className="block text-xs text-gray-500 mb-1">Slice Size</label>
              <input
                type="number"
                value={sliceSize}
                onChange={(e) => setSliceSize(e.target.value)}
                className="w-full px-3 py-2 bg-dark-400 border border-dark-100 rounded text-sm focus:outline-none focus:border-primary-500"
                step="0.0001"
              />
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Interval (ms)</label>
              <input
                type="number"
                value={intervalMs}
                onChange={(e) => setIntervalMs(e.target.value)}
                className="w-full px-3 py-2 bg-dark-400 border border-dark-100 rounded text-sm focus:outline-none focus:border-primary-500"
              />
            </div>
          </div>

          <div className="flex gap-2">
            <button
              onClick={() => setMode('sim')}
              className={clsx(
                'flex-1 py-2 rounded text-sm transition-colors',
                mode === 'sim'
                  ? 'bg-primary-600 text-white'
                  : 'bg-dark-200 text-gray-400'
              )}
            >
              Simulate
            </button>
            <button
              onClick={() => setMode('live')}
              className={clsx(
                'flex-1 py-2 rounded text-sm transition-colors',
                mode === 'live'
                  ? 'bg-danger text-white'
                  : 'bg-dark-200 text-gray-400'
              )}
            >
              Live
            </button>
          </div>

          <button
            onClick={() => slippageMutation.mutate()}
            disabled={slippageMutation.isPending}
            className="w-full py-2 bg-dark-200 hover:bg-dark-100 rounded text-sm text-gray-300 transition-colors"
          >
            {slippageMutation.isPending ? 'Calculating...' : 'Calculate Slippage'}
          </button>

          {slippage && (
            <div className="bg-dark-400 p-3 rounded text-xs space-y-1">
              <div className="flex justify-between">
                <span className="text-gray-400">Entry Slippage</span>
                <span className={slippage.entrySlippageBps > 10 ? 'text-warning' : 'text-gray-300'}>
                  {slippage.entrySlippageBps.toFixed(2)} bps
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Est. Fees</span>
                <span className="text-gray-300">${slippage.totalFeesUsd.toFixed(2)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Projected PnL</span>
                <span className={slippage.projectedPnlUsd >= 0 ? 'text-success' : 'text-danger'}>
                  ${slippage.projectedPnlUsd.toFixed(2)}
                </span>
              </div>
              {slippage.liquidityWarning && (
                <div className="text-warning mt-2">⚠️ Insufficient liquidity</div>
              )}
            </div>
          )}

          <button
            onClick={() => tradeMutation.mutate()}
            disabled={tradeMutation.isPending}
            className={clsx(
              'w-full py-3 rounded font-medium transition-colors',
              mode === 'live'
                ? 'bg-danger hover:bg-red-600 text-white'
                : 'bg-success hover:bg-green-600 text-white'
            )}
          >
            {tradeMutation.isPending
              ? 'Submitting...'
              : mode === 'live'
              ? 'Execute Trade (LIVE)'
              : 'Execute Trade (Simulation)'}
          </button>
        </div>
      </div>

      {/* Deposit/Withdraw Status */}
      <div className="grid grid-cols-2 gap-4">
        <DepositWithdrawStatusCard 
          exchange={spread.longExchange} 
          symbol={spread.symbol}
          side="Long" 
        />
        <DepositWithdrawStatusCard 
          exchange={spread.shortExchange} 
          symbol={spread.symbol}
          side="Short" 
        />
      </div>

      {/* Orderbooks */}
      <div className="grid grid-cols-2 gap-4">
        <div className="bg-dark-300 p-4 rounded-lg border border-dark-100">
          <h2 className="text-sm font-medium text-gray-400 mb-2">
            {spread.longExchange} Orderbook (Long)
          </h2>
          <OrderbookDisplay orderbook={longOrderbook} side="long" />
        </div>
        <div className="bg-dark-300 p-4 rounded-lg border border-dark-100">
          <h2 className="text-sm font-medium text-gray-400 mb-2">
            {spread.shortExchange} Orderbook (Short)
          </h2>
          <OrderbookDisplay orderbook={shortOrderbook} side="short" />
        </div>
      </div>
    </div>
  );
}

function DepositWithdrawStatusCard({ 
  exchange, 
  symbol,
  side 
}: { 
  exchange: string; 
  symbol: string;
  side: string;
}) {
  // Fetch real asset info from the API
  const [status, setStatus] = useState<DepositWithdrawStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    // Fetch deposit/withdraw status from API
    const fetchStatus = async () => {
      setLoading(true);
      setError(null);
      
      try {
        // Extract base asset from symbol (e.g., "BTC-USDT-PERP" -> "BTC")
        const baseAsset = symbol.split('-')[0] || symbol;
        
        // Call the real API endpoint
        const response = await getAssetInfo(exchange.toLowerCase(), baseAsset);
        
        // Check if response has error property
        if ('error' in response) {
          throw new Error(String(response.error));
        }
        
        const assetInfo = response as AssetInfo;
        const mappedStatus: DepositWithdrawStatus = {
          exchange: assetInfo.exchangeId || exchange,
          depositEnabled: assetInfo.depositEnabled,
          withdrawEnabled: assetInfo.withdrawEnabled,
          minWithdraw: assetInfo.minWithdraw,
          withdrawFee: assetInfo.withdrawFee,
          network: assetInfo.networks?.[0] || baseAsset,
          lastUpdated: new Date(assetInfo.timestamp),
        };
        setStatus(mappedStatus);
      } catch (err) {
        console.error(`Failed to fetch asset info for ${exchange}/${symbol}:`, err);
        setError('Failed to load status');
        // Fall back to default enabled state
        setStatus({
          exchange,
          depositEnabled: true,
          withdrawEnabled: true,
          lastUpdated: new Date(),
        });
      } finally {
        setLoading(false);
      }
    };

    fetchStatus();
    // Refresh every 2 minutes
    const interval = setInterval(fetchStatus, 120000);
    return () => clearInterval(interval);
  }, [exchange, symbol]);

  if (loading) {
    return (
      <div className="bg-dark-300 p-4 rounded-lg border border-dark-100">
        <h2 className="text-sm font-medium text-gray-400 mb-2">
          {exchange} Deposit/Withdraw ({side})
        </h2>
        <div className="text-center text-gray-500 py-2">Loading...</div>
      </div>
    );
  }

  if (!status) return null;

  return (
    <div className="bg-dark-300 p-4 rounded-lg border border-dark-100">
      {error && (
        <div className="text-xs text-yellow-400 mb-2">
          ⚠️ {error}
        </div>
      )}
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-medium text-gray-400">
          {exchange} Deposit/Withdraw ({side})
        </h2>
        <span className="text-xs text-gray-500">
          Updated {new Date(status.lastUpdated).toLocaleTimeString()}
        </span>
      </div>
      
      <div className="grid grid-cols-2 gap-4">
        {/* Deposit Status */}
        <div className={clsx(
          'p-3 rounded border',
          status.depositEnabled 
            ? 'bg-success/10 border-success/30' 
            : 'bg-danger/10 border-danger/30'
        )}>
          <div className="flex items-center gap-2 mb-1">
            <span className={clsx(
              'w-2 h-2 rounded-full',
              status.depositEnabled ? 'bg-success' : 'bg-danger'
            )} />
            <span className="text-sm font-medium">Deposit</span>
          </div>
          <div className={clsx(
            'text-xs',
            status.depositEnabled ? 'text-success' : 'text-danger'
          )}>
            {status.depositEnabled ? 'Available' : 'Suspended'}
          </div>
          {status.depositEnabled && status.minDeposit && (
            <div className="text-xs text-gray-400 mt-1">
              Min: {status.minDeposit} {symbol}
            </div>
          )}
        </div>

        {/* Withdraw Status */}
        <div className={clsx(
          'p-3 rounded border',
          status.withdrawEnabled 
            ? 'bg-success/10 border-success/30' 
            : 'bg-danger/10 border-danger/30'
        )}>
          <div className="flex items-center gap-2 mb-1">
            <span className={clsx(
              'w-2 h-2 rounded-full',
              status.withdrawEnabled ? 'bg-success' : 'bg-danger'
            )} />
            <span className="text-sm font-medium">Withdraw</span>
          </div>
          <div className={clsx(
            'text-xs',
            status.withdrawEnabled ? 'text-success' : 'text-danger'
          )}>
            {status.withdrawEnabled ? 'Available' : 'Suspended'}
          </div>
          {status.withdrawEnabled && (
            <div className="text-xs text-gray-400 mt-1 space-y-0.5">
              {status.minWithdraw && <div>Min: {status.minWithdraw} {symbol}</div>}
              {status.withdrawFee && <div>Fee: {status.withdrawFee} {symbol}</div>}
            </div>
          )}
        </div>
      </div>

      {status.network && (
        <div className="mt-2 text-xs text-gray-500">
          Network: {status.network}
        </div>
      )}
      
      {(!status.depositEnabled || !status.withdrawEnabled) && (
        <div className="mt-2 p-2 bg-warning/10 border border-warning/30 rounded text-xs text-warning">
          ⚠️ Trading may be affected due to disabled deposit/withdrawal
        </div>
      )}
    </div>
  );
}

function OrderbookDisplay({
  orderbook,
  side: _side,
}: {
  orderbook: { bids: OrderbookLevel[]; asks: OrderbookLevel[] };
  side: 'long' | 'short';
}) {
  const maxVolume = Math.max(
    ...orderbook.bids.map((b) => b.size),
    ...orderbook.asks.map((a) => a.size)
  );

  return (
    <div className="grid grid-cols-2 gap-4 text-xs">
      {/* Bids */}
      <div>
        <div className="grid grid-cols-3 text-gray-500 mb-1 px-2">
          <span>Price</span>
          <span className="text-right">Size</span>
          <span className="text-right">Total</span>
        </div>
        {orderbook.bids.slice(0, 15).map((bid, i) => {
          const depthWidth = (bid.size / maxVolume) * 100;
          return (
            <div
              key={i}
              className="orderbook-row orderbook-row-bid text-success"
              style={{ '--depth-width': `${depthWidth}%` } as React.CSSProperties}
            >
              <span>{bid.price.toFixed(2)}</span>
              <span className="text-right">{bid.size.toFixed(4)}</span>
              <span className="text-right text-gray-400">
                {(bid.price * bid.size).toFixed(2)}
              </span>
            </div>
          );
        })}
      </div>

      {/* Asks */}
      <div>
        <div className="grid grid-cols-3 text-gray-500 mb-1 px-2">
          <span>Price</span>
          <span className="text-right">Size</span>
          <span className="text-right">Total</span>
        </div>
        {orderbook.asks.slice(0, 15).map((ask, i) => {
          const depthWidth = (ask.size / maxVolume) * 100;
          return (
            <div
              key={i}
              className="orderbook-row orderbook-row-ask text-danger"
              style={{ '--depth-width': `${depthWidth}%` } as React.CSSProperties}
            >
              <span>{ask.price.toFixed(2)}</span>
              <span className="text-right">{ask.size.toFixed(4)}</span>
              <span className="text-right text-gray-400">
                {(ask.price * ask.size).toFixed(2)}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
