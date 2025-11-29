import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { listApiKeys, addApiKey, deleteApiKey, getExchanges } from '../api';
import clsx from 'clsx';

interface Exchange {
  id: string;
  name: string;
  displayName: string;
  isActive: boolean;
}

interface ApiKey {
  id: string;
  exchangeId: string;
  exchangeName: string;
  label: string;
  isActive: boolean;
  createdAt: string;
}

interface ApiKeyForm {
  exchangeId: string;
  apiKey: string;
  apiSecret: string;
  passphrase: string;
  label: string;
}

// Exchange-specific info about required fields
const EXCHANGE_CONFIG: Record<string, { requiresPassphrase: boolean; note?: string }> = {
  kucoin: { requiresPassphrase: true, note: 'KuCoin requires a passphrase' },
  okx: { requiresPassphrase: true, note: 'OKX requires a passphrase' },
  bitget: { requiresPassphrase: true, note: 'Bitget requires a passphrase' },
  coinex: { requiresPassphrase: false },
  binance: { requiresPassphrase: false },
  bybit: { requiresPassphrase: false },
  mexc: { requiresPassphrase: false },
  gateio: { requiresPassphrase: false },
  bingx: { requiresPassphrase: false },
  lbank: { requiresPassphrase: false },
  htx: { requiresPassphrase: false },
};

export default function ApiKeys() {
  const queryClient = useQueryClient();
  const [showForm, setShowForm] = useState(false);
  const [formData, setFormData] = useState<ApiKeyForm>({
    exchangeId: '',
    apiKey: '',
    apiSecret: '',
    passphrase: '',
    label: '',
  });
  const [error, setError] = useState<string | null>(null);

  // Fetch exchanges from database
  const { data: exchanges = [], isLoading: exchangesLoading } = useQuery({
    queryKey: ['exchanges'],
    queryFn: getExchanges,
  });

  // Fetch user's API keys
  const { data: apiKeys = [], isLoading: keysLoading } = useQuery({
    queryKey: ['apiKeys'],
    queryFn: listApiKeys,
  });

  // Add API key mutation
  const addKeyMutation = useMutation({
    mutationFn: (data: { exchangeId: string; apiKey: string; apiSecret: string; passphrase?: string; label?: string }) =>
      addApiKey(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['apiKeys'] });
      setShowForm(false);
      resetForm();
      setError(null);
    },
    onError: (err: any) => {
      setError(err.response?.data?.message || 'Failed to add API key');
    },
  });

  // Delete API key mutation
  const deleteKeyMutation = useMutation({
    mutationFn: (keyId: string) => deleteApiKey(keyId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['apiKeys'] });
    },
    onError: (err: any) => {
      setError(err.response?.data?.message || 'Failed to delete API key');
    },
  });

  const resetForm = () => {
    setFormData({
      exchangeId: '',
      apiKey: '',
      apiSecret: '',
      passphrase: '',
      label: '',
    });
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    if (!formData.exchangeId || !formData.apiKey || !formData.apiSecret) {
      setError('Please fill in all required fields');
      return;
    }

    const exchange = exchanges.find((ex: Exchange) => ex.id === formData.exchangeId);
    const config = EXCHANGE_CONFIG[exchange?.name?.toLowerCase() || ''];

    if (config?.requiresPassphrase && !formData.passphrase) {
      setError(`${exchange?.displayName || 'This exchange'} requires a passphrase`);
      return;
    }

    addKeyMutation.mutate({
      exchangeId: formData.exchangeId,
      apiKey: formData.apiKey,
      apiSecret: formData.apiSecret,
      passphrase: formData.passphrase || undefined,
      label: formData.label || undefined,
    });
  };

  const handleDelete = (keyId: string, exchangeName: string) => {
    if (confirm(`Are you sure you want to delete the API key for ${exchangeName}?`)) {
      deleteKeyMutation.mutate(keyId);
    }
  };

  const selectedExchange = exchanges.find((ex: Exchange) => ex.id === formData.exchangeId);
  const selectedExchangeConfig = EXCHANGE_CONFIG[selectedExchange?.name?.toLowerCase() || ''];

  // Group API keys by exchange
  const keysByExchange = (apiKeys as ApiKey[]).reduce((acc, key) => {
    if (!acc[key.exchangeName]) {
      acc[key.exchangeName] = [];
    }
    acc[key.exchangeName].push(key);
    return acc;
  }, {} as Record<string, ApiKey[]>);

  const isLoading = exchangesLoading || keysLoading;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">API Keys</h1>
          <p className="text-gray-400 text-sm mt-1">
            Manage your exchange API credentials for trading and market data access
          </p>
        </div>
        <button
          onClick={() => setShowForm(true)}
          className="px-4 py-2 bg-primary-600 text-white rounded hover:bg-primary-700 transition-colors flex items-center gap-2"
        >
          <span>+</span>
          <span>Add API Key</span>
        </button>
      </div>

      {/* Security Notice */}
      <div className="bg-yellow-900/20 border border-yellow-700/50 rounded-lg p-4">
        <div className="flex items-start gap-3">
          <span className="text-yellow-500 text-xl">⚠️</span>
          <div className="text-sm">
            <p className="text-yellow-400 font-medium">Security Notice</p>
            <p className="text-yellow-400/80 mt-1">
              Your API keys are encrypted before being stored. For safety, we recommend:
            </p>
            <ul className="list-disc list-inside text-yellow-400/70 mt-2 space-y-1">
              <li>Use API keys with IP whitelist enabled on the exchange</li>
              <li>Only enable permissions needed for trading (futures trading, no withdrawal)</li>
              <li>Use separate API keys for this platform, not your main keys</li>
            </ul>
          </div>
        </div>
      </div>

      {/* Add API Key Form Modal */}
      {showForm && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-dark-300 rounded-lg p-6 w-full max-w-md border border-dark-100">
            <h2 className="text-xl font-bold text-white mb-4">Add API Key</h2>
            
            {error && (
              <div className="mb-4 p-3 bg-red-900/20 border border-red-700/50 rounded text-red-400 text-sm">
                {error}
              </div>
            )}

            <form onSubmit={handleSubmit} className="space-y-4">
              {/* Exchange Select */}
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">
                  Exchange *
                </label>
                <select
                  value={formData.exchangeId}
                  onChange={(e) => setFormData({ ...formData, exchangeId: e.target.value })}
                  className="w-full px-3 py-2 bg-dark-400 border border-dark-100 rounded text-white focus:outline-none focus:border-primary-500"
                >
                  <option value="">Select an exchange</option>
                  {(exchanges as Exchange[])
                    .filter((ex) => ex.isActive)
                    .map((ex) => (
                      <option key={ex.id} value={ex.id}>
                        {ex.displayName}
                      </option>
                    ))}
                </select>
                {selectedExchangeConfig?.note && (
                  <p className="mt-1 text-xs text-yellow-500">{selectedExchangeConfig.note}</p>
                )}
              </div>

              {/* API Key */}
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">
                  API Key *
                </label>
                <input
                  type="text"
                  value={formData.apiKey}
                  onChange={(e) => setFormData({ ...formData, apiKey: e.target.value })}
                  className="w-full px-3 py-2 bg-dark-400 border border-dark-100 rounded text-white focus:outline-none focus:border-primary-500 font-mono text-sm"
                  placeholder="Enter your API key"
                />
              </div>

              {/* API Secret */}
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">
                  API Secret *
                </label>
                <input
                  type="password"
                  value={formData.apiSecret}
                  onChange={(e) => setFormData({ ...formData, apiSecret: e.target.value })}
                  className="w-full px-3 py-2 bg-dark-400 border border-dark-100 rounded text-white focus:outline-none focus:border-primary-500 font-mono text-sm"
                  placeholder="Enter your API secret"
                />
              </div>

              {/* Passphrase (conditional) */}
              {selectedExchangeConfig?.requiresPassphrase && (
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-1">
                    Passphrase *
                  </label>
                  <input
                    type="password"
                    value={formData.passphrase}
                    onChange={(e) => setFormData({ ...formData, passphrase: e.target.value })}
                    className="w-full px-3 py-2 bg-dark-400 border border-dark-100 rounded text-white focus:outline-none focus:border-primary-500 font-mono text-sm"
                    placeholder="Enter your passphrase"
                  />
                </div>
              )}

              {/* Label */}
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">
                  Label (optional)
                </label>
                <input
                  type="text"
                  value={formData.label}
                  onChange={(e) => setFormData({ ...formData, label: e.target.value })}
                  className="w-full px-3 py-2 bg-dark-400 border border-dark-100 rounded text-white focus:outline-none focus:border-primary-500 text-sm"
                  placeholder="e.g., Trading Key, Main Account"
                />
              </div>

              {/* Actions */}
              <div className="flex justify-end gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => {
                    setShowForm(false);
                    resetForm();
                    setError(null);
                  }}
                  className="px-4 py-2 text-gray-400 hover:text-white transition-colors"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={addKeyMutation.isPending}
                  className="px-4 py-2 bg-primary-600 text-white rounded hover:bg-primary-700 transition-colors disabled:opacity-50"
                >
                  {addKeyMutation.isPending ? 'Adding...' : 'Add Key'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* API Keys List */}
      {isLoading ? (
        <div className="text-center py-8 text-gray-400">Loading...</div>
      ) : (apiKeys as ApiKey[]).length === 0 ? (
        <div className="bg-dark-300 rounded-lg border border-dark-100 p-8 text-center">
          <div className="text-gray-400 mb-4">
            <span className="text-4xl">🔑</span>
          </div>
          <h3 className="text-lg font-medium text-white mb-2">No API Keys Yet</h3>
          <p className="text-gray-400 text-sm mb-4">
            Add your exchange API keys to enable trading and access real-time market data.
          </p>
          <button
            onClick={() => setShowForm(true)}
            className="px-4 py-2 bg-primary-600 text-white rounded hover:bg-primary-700 transition-colors"
          >
            Add Your First API Key
          </button>
        </div>
      ) : (
        <div className="space-y-4">
          {Object.entries(keysByExchange).map(([exchangeName, keys]) => (
            <div key={exchangeName} className="bg-dark-300 rounded-lg border border-dark-100 overflow-hidden">
              <div className="px-4 py-3 bg-dark-200 border-b border-dark-100 flex items-center justify-between">
                <h3 className="font-medium text-white">{exchangeName}</h3>
                <span className="text-xs text-gray-400">{keys.length} key(s)</span>
              </div>
              <div className="divide-y divide-dark-100">
                {keys.map((key) => (
                  <div key={key.id} className="px-4 py-3 flex items-center justify-between">
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-white font-medium">{key.label}</span>
                        <span
                          className={clsx(
                            'text-xs px-2 py-0.5 rounded',
                            key.isActive
                              ? 'bg-green-900/30 text-green-400'
                              : 'bg-red-900/30 text-red-400'
                          )}
                        >
                          {key.isActive ? 'Active' : 'Inactive'}
                        </span>
                      </div>
                      <div className="text-xs text-gray-500 mt-1">
                        Added {new Date(key.createdAt).toLocaleDateString()}
                      </div>
                    </div>
                    <button
                      onClick={() => handleDelete(key.id, key.exchangeName)}
                      disabled={deleteKeyMutation.isPending}
                      className="text-red-400 hover:text-red-300 transition-colors text-sm"
                    >
                      Delete
                    </button>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Exchange Status Overview */}
      <div className="bg-dark-300 rounded-lg border border-dark-100 p-4">
        <h3 className="text-lg font-medium text-white mb-4">Exchange Coverage</h3>
        <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-6 gap-3">
          {(exchanges as Exchange[])
            .filter((ex) => ex.isActive)
            .map((ex) => {
              const hasKey = (apiKeys as ApiKey[]).some(
                (k) => k.exchangeId === ex.id && k.isActive
              );
              return (
                <div
                  key={ex.id}
                  className={clsx(
                    'px-3 py-2 rounded-lg border text-sm text-center',
                    hasKey
                      ? 'bg-green-900/20 border-green-700/50 text-green-400'
                      : 'bg-dark-200 border-dark-100 text-gray-500'
                  )}
                >
                  <div className="font-medium">{ex.displayName}</div>
                  <div className="text-xs mt-0.5">
                    {hasKey ? '✓ Connected' : 'Not connected'}
                  </div>
                </div>
              );
            })}
        </div>
      </div>
    </div>
  );
}
