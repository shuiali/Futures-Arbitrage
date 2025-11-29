import axios from 'axios';
import { useAuthStore } from '../store/auth';

const api = axios.create({
  baseURL: '/api/v1',
  headers: {
    'Content-Type': 'application/json',
  },
});

// Add auth token to requests
api.interceptors.request.use((config) => {
  // First try direct localStorage (sync) then fallback to zustand state
  const token = localStorage.getItem('token') || useAuthStore.getState().token;
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Handle auth errors
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      // Only redirect if we're not already on the login page
      if (!window.location.pathname.includes('/login')) {
        useAuthStore.getState().logout();
        window.location.href = '/login';
      }
    }
    return Promise.reject(error);
  }
);

export interface LoginRequest {
  username: string;
  password: string;
}

export interface LoginResponse {
  accessToken: string;
  expiresIn: number;
  user: {
    id: string;
    username: string;
    role: string;
  };
}

export interface Spread {
  id: string;
  symbol: string;
  longExchange: string;
  shortExchange: string;
  spreadPercent: number;
  longPrice: number;
  shortPrice: number;
  volume24h: number;
  fundingLong: number;
  fundingShort: number;
  minDepthUsd?: number;
  score?: number;
  updatedAt: string;
}

export interface SpreadDetail extends Spread {
  longOrderbook: Orderbook;
  shortOrderbook: Orderbook;
  longInstrument?: Instrument;
  shortInstrument?: Instrument;
  longSymbol?: string;
  shortSymbol?: string;
  spreadBps?: number;
  netFunding?: number;
  longDepthUsd?: number;
  shortDepthUsd?: number;
}

export interface Orderbook {
  bids: OrderbookLevel[];
  asks: OrderbookLevel[];
}

export interface OrderbookLevel {
  price: number;
  size: number;
}

export interface Instrument {
  id: string;
  symbol: string;
  baseAsset: string;
  quoteAsset: string;
}

export interface SlippageResult {
  entryPriceLong: number;
  entryPriceShort: number;
  exitPriceLong: number;
  exitPriceShort: number;
  entrySlippageBps: number;
  exitSlippageBps: number;
  totalFeesUsd: number;
  projectedPnlUsd: number;
  liquidityWarning: boolean;
}

export interface Position {
  id: string;
  spreadId: string;
  sizeInCoins: number;
  entryPriceLong: number;
  entryPriceShort: number;
  status: string;
  realizedPnl: number;
  unrealizedPnl: number;
  createdAt: string;
}

export interface EnterTradeRequest {
  spreadId: string;
  sizeInCoins: number;
  slicing: {
    sliceSizeInCoins: number;
    intervalMs: number;
  };
  mode: 'live' | 'sim';
}

// Auth
export const login = (data: LoginRequest) =>
  api.post<LoginResponse>('/auth/login', data).then((r) => r.data);

// Spreads
export const getSpreads = (token?: string, limit = 50) =>
  api.get<Spread[]>('/spreads', { params: { token, limit } }).then((r) => r.data);

export const getSpreadDetail = (spreadId: string) =>
  api.get<SpreadDetail>(`/spreads/${spreadId}`).then((r) => r.data);

export const getSpreadHistory = (spreadId: string, from?: string, to?: string) =>
  api.get(`/spreads/${spreadId}/history`, { params: { from, to } }).then((r) => r.data);

// OHLC Candles for spread chart
export interface CandleData {
  time: number;
  open: number;
  high: number;
  low: number;
  close: number;
  volume?: number;
}

export const getSpreadCandles = (
  spreadId: string, 
  interval: string = '1m',
  from?: string, 
  to?: string,
  limit: number = 500
) =>
  api.get<CandleData[]>(`/spreads/${spreadId}/candles`, { 
    params: { interval, from, to, limit } 
  }).then((r) => r.data);

export const calculateSlippage = (spreadId: string, sizeInCoins: number) =>
  api.get<SlippageResult>(`/spreads/${spreadId}/slippage`, { params: { sizeInCoins } }).then((r) => r.data);

// Trading
export const enterTrade = (data: EnterTradeRequest) =>
  api.post('/trade/enter', data).then((r) => r.data);

export const exitTrade = (positionId: string, mode: 'normal' | 'emergency' = 'normal') =>
  api.post(`/trade/exit/${positionId}`, { mode }).then((r) => r.data);

export const getPositions = () =>
  api.get<Position[]>('/trade/positions').then((r) => r.data);

export const cancelOrder = (orderId: string) =>
  api.post(`/trade/orders/${orderId}/cancel`).then((r) => r.data);

// Tokens & Exchanges
export const getTokens = () =>
  api.get('/tokens').then((r) => r.data);

export const getExchanges = () =>
  api.get('/exchanges').then((r) => r.data);

// Admin
export const createUser = (data: { username: string; password: string; expiryDays?: number }) =>
  api.post('/admin/users', data).then((r) => r.data);

export const listUsers = (page = 1, limit = 20) =>
  api.get('/admin/users', { params: { page, limit } }).then((r) => r.data);

export const updateUser = (userId: string, data: { isActive?: boolean; expiryDays?: number }) =>
  api.put(`/admin/users/${userId}`, data).then((r) => r.data);

// API Keys
export const addApiKey = (data: { exchangeId: string; apiKey: string; apiSecret: string; passphrase?: string }) =>
  api.post('/api_keys', data).then((r) => r.data);

export const listApiKeys = () =>
  api.get('/api_keys').then((r) => r.data);

export const deleteApiKey = (keyId: string) =>
  api.delete(`/api_keys/${keyId}`).then((r) => r.data);

// Asset Info (Deposit/Withdraw Status)
export interface AssetInfo {
  exchangeId: string;
  asset: string;
  depositEnabled: boolean;
  withdrawEnabled: boolean;
  withdrawFee?: number;
  minWithdraw?: number;
  networks?: string[];
  timestamp: string;
}

export interface ExchangeAssetInfo {
  exchange: string;
  assets: AssetInfo[];
  lastUpdated: string;
}

export const getAssetInfo = (exchange: string, asset: string) =>
  api.get<AssetInfo>(`/assets/${exchange}/${asset}`).then((r) => r.data);

export const getExchangeAssetInfo = (exchange: string) =>
  api.get<ExchangeAssetInfo>(`/assets/exchange/${exchange}`).then((r) => r.data);

export const getAllAssetInfo = () =>
  api.get<ExchangeAssetInfo[]>('/assets').then((r) => r.data);

export default api;
