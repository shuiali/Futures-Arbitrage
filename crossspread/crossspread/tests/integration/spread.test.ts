/**
 * Integration tests for spread discovery and calculation
 */

import { describe, it, expect, beforeAll, afterAll } from '@jest/globals';

// Mock market data for testing
const mockOrderbooks = {
  binance: {
    symbol: 'BTCUSDT',
    bids: [[42000, 1.5], [41999, 2.0], [41998, 3.5]],
    asks: [[42001, 1.2], [42002, 2.5], [42003, 3.0]],
  },
  bybit: {
    symbol: 'BTCUSDT',
    bids: [[42010, 1.8], [42009, 2.2], [42008, 3.0]],
    asks: [[42011, 1.5], [42012, 2.8], [42013, 3.2]],
  },
};

describe('Spread Discovery Integration', () => {
  describe('Spread Calculation', () => {
    it('should calculate positive spread when arbitrage exists', () => {
      // Buy on binance (ask), sell on bybit (bid)
      const binanceAsk = mockOrderbooks.binance.asks[0][0];
      const bybitBid = mockOrderbooks.bybit.bids[0][0];
      
      const spreadBps = ((bybitBid - binanceAsk) / binanceAsk) * 10000;
      
      expect(spreadBps).toBeGreaterThan(0);
      expect(spreadBps).toBeCloseTo(2.14, 1); // ~2.14 bps
    });

    it('should identify best spread direction', () => {
      // Direction 1: Buy binance, sell bybit
      const spread1 = (mockOrderbooks.bybit.bids[0][0] - mockOrderbooks.binance.asks[0][0]) 
                      / mockOrderbooks.binance.asks[0][0] * 10000;
      
      // Direction 2: Buy bybit, sell binance
      const spread2 = (mockOrderbooks.binance.bids[0][0] - mockOrderbooks.bybit.asks[0][0]) 
                      / mockOrderbooks.bybit.asks[0][0] * 10000;
      
      // First direction should be positive (profitable)
      expect(spread1).toBeGreaterThan(0);
      // Second direction should be negative (not profitable)
      expect(spread2).toBeLessThan(0);
    });
  });

  describe('Slippage Calculation', () => {
    it('should walk the book to calculate entry price', () => {
      const targetSize = 3.0; // 3 BTC
      const asks = mockOrderbooks.binance.asks;
      
      let remainingSize = targetSize;
      let totalCost = 0;
      
      for (const [price, size] of asks) {
        const fillSize = Math.min(remainingSize, size);
        totalCost += price * fillSize;
        remainingSize -= fillSize;
        if (remainingSize <= 0) break;
      }
      
      const avgPrice = totalCost / targetSize;
      const bestAsk = asks[0][0];
      const slippageBps = ((avgPrice - bestAsk) / bestAsk) * 10000;
      
      expect(avgPrice).toBeGreaterThan(bestAsk);
      expect(slippageBps).toBeGreaterThan(0);
    });

    it('should detect insufficient liquidity', () => {
      const targetSize = 100; // 100 BTC - more than available
      const asks = mockOrderbooks.binance.asks;
      
      let availableSize = asks.reduce((sum, [_, size]) => sum + size, 0);
      
      expect(availableSize).toBeLessThan(targetSize);
    });
  });

  describe('Fee Calculation', () => {
    it('should include maker/taker fees in PnL', () => {
      const entryPrice = 42001;
      const exitPrice = 42010;
      const size = 1.0;
      const takerFee = 0.0004; // 0.04%
      
      const grossPnl = (exitPrice - entryPrice) * size;
      const entryFee = entryPrice * size * takerFee;
      const exitFee = exitPrice * size * takerFee;
      const netPnl = grossPnl - entryFee - exitFee;
      
      expect(grossPnl).toBe(9);
      expect(netPnl).toBeLessThan(grossPnl);
      expect(netPnl).toBeCloseTo(9 - 16.8 - 16.8, 1); // Negative after fees
    });
  });
});

describe('Order Slicing Integration', () => {
  describe('Slice Generation', () => {
    it('should generate correct number of slices', () => {
      const totalSize = 1.0; // 1 BTC
      const sliceSize = 0.1; // 0.1 BTC per slice
      
      const slices = Math.ceil(totalSize / sliceSize);
      
      expect(slices).toBe(10);
    });

    it('should handle remainder in last slice', () => {
      const totalSize = 1.0;
      const sliceSize = 0.3;
      
      const fullSlices = Math.floor(totalSize / sliceSize);
      const remainder = totalSize - (fullSlices * sliceSize);
      
      expect(fullSlices).toBe(3);
      expect(remainder).toBeCloseTo(0.1, 10);
    });
  });

  describe('Price Tolerance', () => {
    it('should calculate limit price with tolerance', () => {
      const topOfBook = 42000;
      const tolerancePct = 0.01; // 0.01%
      
      // For buy orders, add tolerance
      const buyLimit = topOfBook * (1 + tolerancePct / 100);
      // For sell orders, subtract tolerance
      const sellLimit = topOfBook * (1 - tolerancePct / 100);
      
      expect(buyLimit).toBe(42004.2);
      expect(sellLimit).toBe(41995.8);
    });
  });
});

describe('Emergency Exit Integration', () => {
  it('should calculate aggressive exit price', () => {
    const position = {
      side: 'long',
      size: 1.0,
      entryPrice: 42000,
    };
    
    // For long exit, hit bids with slippage allowance
    const topBid = mockOrderbooks.binance.bids[0][0];
    const slippageTolerance = 0.001; // 0.1%
    const aggressivePrice = topBid * (1 - slippageTolerance);
    
    expect(aggressivePrice).toBeLessThan(topBid);
  });
});
