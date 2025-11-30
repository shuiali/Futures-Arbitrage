/// <reference types="vite/client" />

import { io, Socket } from 'socket.io-client';
import { useAuthStore } from '../store/auth';

let socket: Socket | null = null;

export function getSocket(): Socket {
  if (!socket) {
    const token = useAuthStore.getState().token;
    // Connect to backend WebSocket - use the API base URL
    const wsUrl = import.meta.env.VITE_API_URL || 'http://localhost:8000';
    socket = io(`${wsUrl}/ws/market`, {
      auth: { token },
      transports: ['websocket', 'polling'], // Allow fallback to polling
      reconnection: true,
      reconnectionAttempts: 5,
      reconnectionDelay: 1000,
      timeout: 10000,
    });

    socket.on('connect', () => {
      console.log('WebSocket connected');
    });

    socket.on('disconnect', () => {
      console.log('WebSocket disconnected');
    });

    socket.on('connect_error', (error) => {
      console.error('WebSocket connection error:', error);
    });

    // Debug: log all incoming events and payloads
    socket.onAny((event, ...args) => {
      // Use console.debug so it's easy to filter in devtools
      console.debug('[WS EVENT]', event, args);
    });
  }

  return socket;
}

export function disconnectSocket(): void {
  if (socket) {
    socket.disconnect();
    socket = null;
  }
}

export function subscribeToOrderbook(
  symbol: string,
  exchanges: string[],
  callback: (data: OrderbookUpdate) => void
): () => void {
  const sock = getSocket();
  
  sock.emit('subscribe', { channel: 'orderbook', symbol, exchanges });
  
  const handler = (data: OrderbookUpdate) => {
    if (exchanges.includes(data.exchange)) {
      callback(data);
    }
  };
  
  sock.on('orderbook', handler);
  
  return () => {
    sock.emit('unsubscribe', { channel: 'orderbook', symbol, exchanges });
    sock.off('orderbook', handler);
  };
}

export function subscribeToSpread(
  spreadId: string,
  callback: (data: SpreadUpdate) => void
): () => void {
  const sock = getSocket();
  
  sock.emit('subscribe', { channel: 'spread', spreadId });
  sock.on('spread', callback);
  
  return () => {
    sock.emit('unsubscribe', { channel: 'spread', spreadId });
    sock.off('spread', callback);
  };
}

export function subscribeToTrades(
  callback: (data: TradeUpdate) => void
): () => void {
  const sock = getSocket();
  
  sock.emit('subscribe', { channel: 'trades' });
  sock.on('trade', callback);
  
  return () => {
    sock.emit('unsubscribe', { channel: 'trades' });
    sock.off('trade', callback);
  };
}

export interface OrderbookUpdate {
  exchange: string;
  symbol: string;
  bids: Array<{ price: number; size: number }>;
  asks: Array<{ price: number; size: number }>;
  timestamp: number;
}

export interface SpreadUpdate {
  spreadId: string;
  spreadPercent: number;
  longPrice: number;
  shortPrice: number;
  timestamp: number;
}

export interface TradeUpdate {
  type: 'fill' | 'order' | 'position';
  orderId?: string;
  positionId?: string;
  status?: string;
  filledQuantity?: number;
  price?: number;
  timestamp: number;
}
