import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import toast from 'react-hot-toast';
import { listUsers, createUser, updateUser } from '../api';

export default function AdminUsers() {
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [newUsername, setNewUsername] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [expiryDays, setExpiryDays] = useState('30');
  const queryClient = useQueryClient();

  const { data, isLoading } = useQuery({
    queryKey: ['users'],
    queryFn: () => listUsers(1, 100),
  });

  const createMutation = useMutation({
    mutationFn: () =>
      createUser({
        username: newUsername,
        password: newPassword,
        expiryDays: parseInt(expiryDays, 10),
      }),
    onSuccess: () => {
      toast.success('User created successfully');
      queryClient.invalidateQueries({ queryKey: ['users'] });
      setShowCreateModal(false);
      setNewUsername('');
      setNewPassword('');
    },
    onError: () => toast.error('Failed to create user'),
  });

  const toggleMutation = useMutation({
    mutationFn: ({ userId, isActive }: { userId: string; isActive: boolean }) =>
      updateUser(userId, { isActive }),
    onSuccess: () => {
      toast.success('User updated');
      queryClient.invalidateQueries({ queryKey: ['users'] });
    },
    onError: () => toast.error('Failed to update user'),
  });

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-bold">User Management</h1>
        <button
          onClick={() => setShowCreateModal(true)}
          className="px-4 py-2 bg-primary-600 hover:bg-primary-700 rounded text-sm transition-colors"
        >
          Create User
        </button>
      </div>

      <div className="bg-dark-300 rounded-lg border border-dark-100 overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-dark-200">
            <tr>
              <th className="text-left px-4 py-3 text-gray-400 font-medium">Username</th>
              <th className="text-left px-4 py-3 text-gray-400 font-medium">Role</th>
              <th className="text-center px-4 py-3 text-gray-400 font-medium">Status</th>
              <th className="text-left px-4 py-3 text-gray-400 font-medium">Expires</th>
              <th className="text-left px-4 py-3 text-gray-400 font-medium">Created</th>
              <th className="text-center px-4 py-3 text-gray-400 font-medium">Actions</th>
            </tr>
          </thead>
          <tbody>
            {isLoading ? (
              <tr>
                <td colSpan={6} className="px-4 py-8 text-center text-gray-500">
                  Loading users...
                </td>
              </tr>
            ) : (
              data?.users?.map((user: any) => (
                <tr
                  key={user.id}
                  className="border-t border-dark-100 hover:bg-dark-200 transition-colors"
                >
                  <td className="px-4 py-3 font-medium">{user.username}</td>
                  <td className="px-4 py-3">
                    <span
                      className={
                        user.role === 'admin'
                          ? 'text-primary-400'
                          : 'text-gray-400'
                      }
                    >
                      {user.role}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-center">
                    <span
                      className={`px-2 py-1 rounded text-xs ${
                        user.isActive
                          ? 'bg-success/20 text-success'
                          : 'bg-danger/20 text-danger'
                      }`}
                    >
                      {user.isActive ? 'Active' : 'Disabled'}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-gray-400 text-sm">
                    {user.expiresAt
                      ? new Date(user.expiresAt).toLocaleDateString()
                      : 'Never'}
                  </td>
                  <td className="px-4 py-3 text-gray-400 text-sm">
                    {new Date(user.createdAt).toLocaleDateString()}
                  </td>
                  <td className="px-4 py-3 text-center">
                    {user.role !== 'admin' && (
                      <button
                        onClick={() =>
                          toggleMutation.mutate({
                            userId: user.id,
                            isActive: !user.isActive,
                          })
                        }
                        className={`px-3 py-1 rounded text-xs transition-colors ${
                          user.isActive
                            ? 'bg-danger/20 hover:bg-danger/30 text-danger'
                            : 'bg-success/20 hover:bg-success/30 text-success'
                        }`}
                      >
                        {user.isActive ? 'Disable' : 'Enable'}
                      </button>
                    )}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Create User Modal */}
      {showCreateModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-dark-300 p-6 rounded-lg border border-dark-100 w-full max-w-md">
            <h2 className="text-lg font-bold mb-4">Create User</h2>
            <div className="space-y-4">
              <div>
                <label className="block text-sm text-gray-400 mb-1">Username</label>
                <input
                  type="text"
                  value={newUsername}
                  onChange={(e) => setNewUsername(e.target.value)}
                  className="w-full px-4 py-2 bg-dark-400 border border-dark-100 rounded focus:outline-none focus:border-primary-500"
                />
              </div>
              <div>
                <label className="block text-sm text-gray-400 mb-1">Password</label>
                <input
                  type="password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  className="w-full px-4 py-2 bg-dark-400 border border-dark-100 rounded focus:outline-none focus:border-primary-500"
                />
              </div>
              <div>
                <label className="block text-sm text-gray-400 mb-1">Expiry (days)</label>
                <input
                  type="number"
                  value={expiryDays}
                  onChange={(e) => setExpiryDays(e.target.value)}
                  className="w-full px-4 py-2 bg-dark-400 border border-dark-100 rounded focus:outline-none focus:border-primary-500"
                />
              </div>
              <div className="flex gap-2 pt-2">
                <button
                  onClick={() => setShowCreateModal(false)}
                  className="flex-1 py-2 bg-dark-200 hover:bg-dark-100 rounded transition-colors"
                >
                  Cancel
                </button>
                <button
                  onClick={() => createMutation.mutate()}
                  disabled={createMutation.isPending || !newUsername || !newPassword}
                  className="flex-1 py-2 bg-primary-600 hover:bg-primary-700 rounded transition-colors disabled:opacity-50"
                >
                  {createMutation.isPending ? 'Creating...' : 'Create'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
