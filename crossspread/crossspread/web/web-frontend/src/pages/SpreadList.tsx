import { useState, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { getSpreads } from '../api';
import clsx from 'clsx';

type SortField = 'spread' | 'volume' | 'fundingLong' | 'fundingShort' | 'indexPrice';
type SortDirection = 'asc' | 'desc';

interface Filters {
  token: string;
  minSpread: number | null;
  maxSpread: number | null;
  minVolume: number | null;
  maxVolume: number | null;
  minFunding: number | null;
  maxFunding: number | null;
  exchanges: string[];
}

const AVAILABLE_EXCHANGES = [
  'binance', 'bybit', 'okx', 'kucoin', 'mexc', 
  'bitget', 'gateio', 'bingx', 'coinex', 'lbank', 'htx'
];

export default function SpreadList() {
  const [filters, setFilters] = useState<Filters>({
    token: '',
    minSpread: null,
    maxSpread: null,
    minVolume: null,
    maxVolume: null,
    minFunding: null,
    maxFunding: null,
    exchanges: [],
  });
  const [sortBy, setSortBy] = useState<SortField>('spread');
  const [sortDir, setSortDir] = useState<SortDirection>('desc');
  const [showAdvancedFilters, setShowAdvancedFilters] = useState(false);

  const { data: spreads, isLoading } = useQuery({
    queryKey: ['spreads', filters.token],
    queryFn: () => getSpreads(filters.token || undefined, 200),
    refetchInterval: 3000, // Reduced from 1000ms to 3000ms - WebSocket provides real-time updates
    staleTime: 2000, // Consider data stale after 2 seconds
  });

  // Apply all filters and sorting
  const filteredAndSortedSpreads = useMemo(() => {
    if (!spreads) return [];
    
    let result = [...spreads];

    // Filter by spread range
    if (filters.minSpread !== null) {
      result = result.filter(s => s.spreadPercent >= filters.minSpread!);
    }
    if (filters.maxSpread !== null) {
      result = result.filter(s => s.spreadPercent <= filters.maxSpread!);
    }

    // Filter by volume (in millions)
    if (filters.minVolume !== null) {
      result = result.filter(s => s.volume24h >= filters.minVolume! * 1_000_000);
    }
    if (filters.maxVolume !== null) {
      result = result.filter(s => s.volume24h <= filters.maxVolume! * 1_000_000);
    }

    // Filter by funding rate (combined)
    if (filters.minFunding !== null) {
      const minFund = filters.minFunding / 100; // Convert from % input
      result = result.filter(s => s.fundingLong >= minFund || s.fundingShort >= minFund);
    }
    if (filters.maxFunding !== null) {
      const maxFund = filters.maxFunding / 100;
      result = result.filter(s => s.fundingLong <= maxFund && s.fundingShort <= maxFund);
    }

    // Filter by exchanges
    if (filters.exchanges.length > 0) {
      result = result.filter(s => 
        filters.exchanges.includes(s.longExchange.toLowerCase()) ||
        filters.exchanges.includes(s.shortExchange.toLowerCase())
      );
    }

    // Sort
    result.sort((a, b) => {
      let comparison = 0;
      switch (sortBy) {
        case 'spread':
          comparison = a.spreadPercent - b.spreadPercent;
          break;
        case 'volume':
          comparison = a.volume24h - b.volume24h;
          break;
        case 'fundingLong':
          comparison = a.fundingLong - b.fundingLong;
          break;
        case 'fundingShort':
          comparison = a.fundingShort - b.fundingShort;
          break;
        case 'indexPrice':
          comparison = a.longPrice - b.longPrice;
          break;
      }
      return sortDir === 'desc' ? -comparison : comparison;
    });

    return result;
  }, [spreads, filters, sortBy, sortDir]);

  const handleSort = (field: SortField) => {
    if (sortBy === field) {
      setSortDir(sortDir === 'desc' ? 'asc' : 'desc');
    } else {
      setSortBy(field);
      setSortDir('desc');
    }
  };

  const toggleExchange = (exchange: string) => {
    setFilters(prev => ({
      ...prev,
      exchanges: prev.exchanges.includes(exchange)
        ? prev.exchanges.filter(e => e !== exchange)
        : [...prev.exchanges, exchange],
    }));
  };

  const clearFilters = () => {
    setFilters({
      token: '',
      minSpread: null,
      maxSpread: null,
      minVolume: null,
      maxVolume: null,
      minFunding: null,
      maxFunding: null,
      exchanges: [],
    });
  };

  const SortIcon = ({ field }: { field: SortField }) => (
    <span className="ml-1 inline-block w-3">
      {sortBy === field && (sortDir === 'desc' ? '▼' : '▲')}
    </span>
  );

  return (
    <div className="space-y-4">
      {/* Basic Filters */}
      <div className="bg-dark-300 p-4 rounded-lg border border-dark-100 space-y-4">
        <div className="flex items-center gap-4">
          <input
            type="text"
            placeholder="Filter by token (e.g., BTC, ETH)"
            value={filters.token}
            onChange={(e) => setFilters(prev => ({ ...prev, token: e.target.value }))}
            className="flex-1 px-4 py-2 bg-dark-400 border border-dark-100 rounded focus:outline-none focus:border-primary-500 text-white text-sm"
          />
          <button
            onClick={() => setShowAdvancedFilters(!showAdvancedFilters)}
            className={clsx(
              'px-4 py-2 rounded text-sm transition-colors flex items-center gap-2',
              showAdvancedFilters
                ? 'bg-primary-600 text-white'
                : 'bg-dark-200 text-gray-400 hover:text-white'
            )}
          >
            <span>Filters</span>
            <span className="text-xs">{showAdvancedFilters ? '▲' : '▼'}</span>
          </button>
          {(filters.minSpread !== null || filters.maxSpread !== null || 
            filters.minVolume !== null || filters.maxVolume !== null ||
            filters.minFunding !== null || filters.maxFunding !== null ||
            filters.exchanges.length > 0) && (
            <button
              onClick={clearFilters}
              className="px-4 py-2 rounded text-sm bg-danger/20 text-danger hover:bg-danger/30 transition-colors"
            >
              Clear Filters
            </button>
          )}
        </div>

        {/* Advanced Filters Panel */}
        {showAdvancedFilters && (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 pt-4 border-t border-dark-100">
            {/* Spread Range */}
            <div className="space-y-2">
              <label className="text-xs text-gray-400 font-medium">Spread Range (%)</label>
              <div className="flex gap-2">
                <input
                  type="number"
                  step="0.001"
                  placeholder="Min"
                  value={filters.minSpread ?? ''}
                  onChange={(e) => setFilters(prev => ({ 
                    ...prev, 
                    minSpread: e.target.value ? parseFloat(e.target.value) : null 
                  }))}
                  className="w-full px-3 py-1.5 bg-dark-400 border border-dark-100 rounded text-sm text-white focus:outline-none focus:border-primary-500"
                />
                <input
                  type="number"
                  step="0.001"
                  placeholder="Max"
                  value={filters.maxSpread ?? ''}
                  onChange={(e) => setFilters(prev => ({ 
                    ...prev, 
                    maxSpread: e.target.value ? parseFloat(e.target.value) : null 
                  }))}
                  className="w-full px-3 py-1.5 bg-dark-400 border border-dark-100 rounded text-sm text-white focus:outline-none focus:border-primary-500"
                />
              </div>
            </div>

            {/* Volume Range */}
            <div className="space-y-2">
              <label className="text-xs text-gray-400 font-medium">Volume 24h (Millions $)</label>
              <div className="flex gap-2">
                <input
                  type="number"
                  step="0.1"
                  placeholder="Min"
                  value={filters.minVolume ?? ''}
                  onChange={(e) => setFilters(prev => ({ 
                    ...prev, 
                    minVolume: e.target.value ? parseFloat(e.target.value) : null 
                  }))}
                  className="w-full px-3 py-1.5 bg-dark-400 border border-dark-100 rounded text-sm text-white focus:outline-none focus:border-primary-500"
                />
                <input
                  type="number"
                  step="0.1"
                  placeholder="Max"
                  value={filters.maxVolume ?? ''}
                  onChange={(e) => setFilters(prev => ({ 
                    ...prev, 
                    maxVolume: e.target.value ? parseFloat(e.target.value) : null 
                  }))}
                  className="w-full px-3 py-1.5 bg-dark-400 border border-dark-100 rounded text-sm text-white focus:outline-none focus:border-primary-500"
                />
              </div>
            </div>

            {/* Funding Rate Range */}
            <div className="space-y-2">
              <label className="text-xs text-gray-400 font-medium">Funding Rate (%)</label>
              <div className="flex gap-2">
                <input
                  type="number"
                  step="0.001"
                  placeholder="Min"
                  value={filters.minFunding ?? ''}
                  onChange={(e) => setFilters(prev => ({ 
                    ...prev, 
                    minFunding: e.target.value ? parseFloat(e.target.value) : null 
                  }))}
                  className="w-full px-3 py-1.5 bg-dark-400 border border-dark-100 rounded text-sm text-white focus:outline-none focus:border-primary-500"
                />
                <input
                  type="number"
                  step="0.001"
                  placeholder="Max"
                  value={filters.maxFunding ?? ''}
                  onChange={(e) => setFilters(prev => ({ 
                    ...prev, 
                    maxFunding: e.target.value ? parseFloat(e.target.value) : null 
                  }))}
                  className="w-full px-3 py-1.5 bg-dark-400 border border-dark-100 rounded text-sm text-white focus:outline-none focus:border-primary-500"
                />
              </div>
            </div>

            {/* Exchange Filter */}
            <div className="space-y-2">
              <label className="text-xs text-gray-400 font-medium">Exchanges</label>
              <div className="flex flex-wrap gap-1">
                {AVAILABLE_EXCHANGES.map(exchange => (
                  <button
                    key={exchange}
                    onClick={() => toggleExchange(exchange)}
                    className={clsx(
                      'px-2 py-0.5 rounded text-xs transition-colors capitalize',
                      filters.exchanges.includes(exchange)
                        ? 'bg-primary-600 text-white'
                        : 'bg-dark-200 text-gray-400 hover:text-white'
                    )}
                  >
                    {exchange}
                  </button>
                ))}
              </div>
            </div>
          </div>
        )}

        {/* Results count */}
        <div className="flex items-center justify-between text-sm text-gray-400">
          <span>
            Showing {filteredAndSortedSpreads.length} of {spreads?.length ?? 0} spreads
          </span>
          <div className="flex gap-2">
            <span className="text-xs">Sort by:</span>
            {(['spread', 'volume', 'fundingLong', 'indexPrice'] as SortField[]).map(field => (
              <button
                key={field}
                onClick={() => handleSort(field)}
                className={clsx(
                  'px-2 py-0.5 rounded text-xs transition-colors capitalize',
                  sortBy === field
                    ? 'bg-primary-600 text-white'
                    : 'bg-dark-200 text-gray-400 hover:text-white'
                )}
              >
                {field === 'fundingLong' ? 'Funding' : field === 'indexPrice' ? 'Price' : field}
                {sortBy === field && <span className="ml-1">{sortDir === 'desc' ? '↓' : '↑'}</span>}
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* Spread Table */}
      <div className="bg-dark-300 rounded-lg border border-dark-100 overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-dark-200">
            <tr>
              <th className="text-left px-4 py-3 text-gray-400 font-medium">Symbol</th>
              <th className="text-left px-4 py-3 text-gray-400 font-medium">Long</th>
              <th className="text-left px-4 py-3 text-gray-400 font-medium">Short</th>
              <th 
                className="text-right px-4 py-3 text-gray-400 font-medium cursor-pointer hover:text-white"
                onClick={() => handleSort('spread')}
              >
                Spread<SortIcon field="spread" />
              </th>
              <th 
                className="text-right px-4 py-3 text-gray-400 font-medium cursor-pointer hover:text-white"
                onClick={() => handleSort('indexPrice')}
              >
                Long Price<SortIcon field="indexPrice" />
              </th>
              <th className="text-right px-4 py-3 text-gray-400 font-medium">Short Price</th>
              <th 
                className="text-right px-4 py-3 text-gray-400 font-medium cursor-pointer hover:text-white"
                onClick={() => handleSort('volume')}
              >
                Volume 24h<SortIcon field="volume" />
              </th>
              <th 
                className="text-right px-4 py-3 text-gray-400 font-medium cursor-pointer hover:text-white"
                onClick={() => handleSort('fundingLong')}
              >
                Funding L/S<SortIcon field="fundingLong" />
              </th>
            </tr>
          </thead>
          <tbody>
            {isLoading ? (
              <tr>
                <td colSpan={8} className="px-4 py-8 text-center text-gray-500">
                  Loading spreads...
                </td>
              </tr>
            ) : filteredAndSortedSpreads.length === 0 ? (
              <tr>
                <td colSpan={8} className="px-4 py-8 text-center text-gray-500">
                  No spreads found matching filters
                </td>
              </tr>
            ) : (
              filteredAndSortedSpreads.map((spread) => (
                <tr
                  key={spread.id}
                  className="border-t border-dark-100 hover:bg-dark-200 transition-colors"
                >
                  <td className="px-4 py-3">
                    <Link
                      to={`/spread/${spread.id}`}
                      className="text-primary-400 hover:underline font-medium"
                    >
                      {spread.symbol}
                    </Link>
                  </td>
                  <td className="px-4 py-3 text-gray-300">{spread.longExchange}</td>
                  <td className="px-4 py-3 text-gray-300">{spread.shortExchange}</td>
                  <td className="px-4 py-3 text-right">
                    <span
                      className={clsx(
                        'font-medium',
                        spread.spreadPercent > 0 ? 'text-success' : 'text-danger'
                      )}
                    >
                      {spread.spreadPercent.toFixed(4)}%
                    </span>
                  </td>
                  <td className="px-4 py-3 text-right text-gray-300">
                    ${spread.longPrice.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-right text-gray-300">
                    ${spread.shortPrice.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-right text-gray-300">
                    ${(spread.volume24h / 1_000_000).toFixed(2)}M
                  </td>
                  <td className="px-4 py-3 text-right text-xs">
                    <span className={spread.fundingLong >= 0 ? 'text-success' : 'text-danger'}>
                      {(spread.fundingLong * 100).toFixed(4)}%
                    </span>
                    {' / '}
                    <span className={spread.fundingShort >= 0 ? 'text-success' : 'text-danger'}>
                      {(spread.fundingShort * 100).toFixed(4)}%
                    </span>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
