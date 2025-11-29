import { Outlet, Link, useLocation } from 'react-router-dom';
import { useAuthStore } from '../store/auth';
import clsx from 'clsx';

export default function Layout() {
  const { user, logout } = useAuthStore();
  const location = useLocation();

  const navItems = [
    { path: '/', label: 'Spreads' },
    { path: '/positions', label: 'Positions' },
    { path: '/api-keys', label: 'API Keys' },
  ];

  if (user?.role === 'admin') {
    navItems.push({ path: '/admin/users', label: 'Users' });
  }

  return (
    <div className="min-h-screen flex flex-col">
      {/* Header */}
      <header className="bg-dark-300 border-b border-dark-100 px-4 py-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-8">
            <h1 className="text-xl font-bold text-primary-400">
              CrossSpread
            </h1>
            <nav className="flex gap-1">
              {navItems.map((item) => (
                <Link
                  key={item.path}
                  to={item.path}
                  className={clsx(
                    'px-4 py-2 rounded text-sm transition-colors',
                    location.pathname === item.path
                      ? 'bg-primary-600 text-white'
                      : 'text-gray-400 hover:text-white hover:bg-dark-200'
                  )}
                >
                  {item.label}
                </Link>
              ))}
            </nav>
          </div>
          <div className="flex items-center gap-4">
            <span className="text-sm text-gray-400">
              {user?.username}
              {user?.role === 'admin' && (
                <span className="ml-2 px-2 py-0.5 bg-primary-600/20 text-primary-400 rounded text-xs">
                  admin
                </span>
              )}
            </span>
            <button
              onClick={logout}
              className="text-sm text-gray-400 hover:text-white transition-colors"
            >
              Logout
            </button>
          </div>
        </div>
      </header>

      {/* Main content */}
      <main className="flex-1 p-4">
        <Outlet />
      </main>

      {/* Footer */}
      <footer className="bg-dark-300 border-t border-dark-100 px-4 py-2">
        <div className="flex items-center justify-between text-xs text-gray-500">
          <span>CrossSpread Futures Arbitrage Terminal</span>
          <span>UTC+2 • Connected</span>
        </div>
      </footer>
    </div>
  );
}
