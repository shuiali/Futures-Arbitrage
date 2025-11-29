import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import toast from 'react-hot-toast';
import { getPositions, exitTrade } from '../api';
import clsx from 'clsx';

export default function Positions() {
  const [showEmergencyConfirm, setShowEmergencyConfirm] = useState<string | null>(null);
  const queryClient = useQueryClient();

  const { data: positions, isLoading } = useQuery({
    queryKey: ['positions'],
    queryFn: getPositions,
    refetchInterval: 5000,
  });

  const exitMutation = useMutation({
    mutationFn: ({ positionId, mode }: { positionId: string; mode: 'normal' | 'emergency' }) =>
      exitTrade(positionId, mode),
    onSuccess: () => {
      toast.success('Exit order submitted');
      queryClient.invalidateQueries({ queryKey: ['positions'] });
      setShowEmergencyConfirm(null);
    },
    onError: () => toast.error('Failed to exit position'),
  });

  const handleEmergencyExit = (positionId: string) => {
    exitMutation.mutate({ positionId, mode: 'emergency' });
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-bold">Positions</h1>
      </div>

      <div className="bg-dark-300 rounded-lg border border-dark-100 overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-dark-200">
            <tr>
              <th className="text-left px-4 py-3 text-gray-400 font-medium">Spread</th>
              <th className="text-right px-4 py-3 text-gray-400 font-medium">Size</th>
              <th className="text-right px-4 py-3 text-gray-400 font-medium">Entry Price L/S</th>
              <th className="text-right px-4 py-3 text-gray-400 font-medium">Unrealized PnL</th>
              <th className="text-right px-4 py-3 text-gray-400 font-medium">Realized PnL</th>
              <th className="text-center px-4 py-3 text-gray-400 font-medium">Status</th>
              <th className="text-center px-4 py-3 text-gray-400 font-medium">Actions</th>
            </tr>
          </thead>
          <tbody>
            {isLoading ? (
              <tr>
                <td colSpan={7} className="px-4 py-8 text-center text-gray-500">
                  Loading positions...
                </td>
              </tr>
            ) : positions?.length === 0 ? (
              <tr>
                <td colSpan={7} className="px-4 py-8 text-center text-gray-500">
                  No positions
                </td>
              </tr>
            ) : (
              positions?.map((pos) => (
                <tr
                  key={pos.id}
                  className="border-t border-dark-100 hover:bg-dark-200 transition-colors"
                >
                  <td className="px-4 py-3 font-medium">{pos.spreadId}</td>
                  <td className="px-4 py-3 text-right">{pos.sizeInCoins.toFixed(4)}</td>
                  <td className="px-4 py-3 text-right text-xs">
                    ${pos.entryPriceLong.toLocaleString()} / ${pos.entryPriceShort.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <span
                      className={clsx(
                        'font-medium',
                        pos.unrealizedPnl >= 0 ? 'text-success' : 'text-danger'
                      )}
                    >
                      ${pos.unrealizedPnl.toFixed(2)}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-right">
                    <span
                      className={clsx(
                        'font-medium',
                        pos.realizedPnl >= 0 ? 'text-success' : 'text-danger'
                      )}
                    >
                      ${pos.realizedPnl.toFixed(2)}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-center">
                    <span
                      className={clsx(
                        'px-2 py-1 rounded text-xs',
                        pos.status === 'OPEN'
                          ? 'bg-success/20 text-success'
                          : pos.status === 'OPENING' || pos.status === 'CLOSING'
                          ? 'bg-warning/20 text-warning'
                          : 'bg-gray-500/20 text-gray-400'
                      )}
                    >
                      {pos.status}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-center">
                    {pos.status === 'OPEN' && (
                      <div className="flex gap-2 justify-center">
                        <button
                          onClick={() => exitMutation.mutate({ positionId: pos.id, mode: 'normal' })}
                          disabled={exitMutation.isPending}
                          className="px-3 py-1 bg-primary-600 hover:bg-primary-700 rounded text-xs transition-colors"
                        >
                          Exit
                        </button>
                        {showEmergencyConfirm === pos.id ? (
                          <div className="flex gap-1">
                            <button
                              onClick={() => handleEmergencyExit(pos.id)}
                              className="px-3 py-1 bg-danger hover:bg-red-600 rounded text-xs transition-colors"
                            >
                              Confirm
                            </button>
                            <button
                              onClick={() => setShowEmergencyConfirm(null)}
                              className="px-3 py-1 bg-dark-200 hover:bg-dark-100 rounded text-xs transition-colors"
                            >
                              Cancel
                            </button>
                          </div>
                        ) : (
                          <button
                            onClick={() => setShowEmergencyConfirm(pos.id)}
                            className="px-3 py-1 bg-danger/20 hover:bg-danger/30 text-danger rounded text-xs transition-colors"
                          >
                            Emergency
                          </button>
                        )}
                      </div>
                    )}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Emergency Exit Warning */}
      {showEmergencyConfirm && (
        <div className="bg-danger/10 border border-danger/30 rounded-lg p-4">
          <h3 className="text-danger font-medium mb-2">⚠️ Emergency Exit Warning</h3>
          <p className="text-sm text-gray-300">
            Emergency exit will place aggressive limit orders crossing the book to exit quickly. 
            This may result in significant slippage. Are you sure you want to proceed?
          </p>
        </div>
      )}
    </div>
  );
}
