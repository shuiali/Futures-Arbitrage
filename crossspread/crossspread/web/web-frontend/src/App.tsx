import { Routes, Route, Navigate } from 'react-router-dom';
import { useAuthStore } from './store/auth';
import Layout from './components/Layout';
import Login from './pages/Login';
import SpreadList from './pages/SpreadList';
import SpreadDetail from './pages/SpreadDetail';
import Positions from './pages/Positions';
import AdminUsers from './pages/AdminUsers';
import ApiKeys from './pages/ApiKeys';

function PrivateRoute({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  const hasHydrated = useAuthStore((state) => state._hasHydrated);
  
  // Wait for hydration before checking auth
  if (!hasHydrated) {
    return <div className="min-h-screen bg-dark-500 flex items-center justify-center">
      <div className="text-white">Loading...</div>
    </div>;
  }
  
  return isAuthenticated ? <>{children}</> : <Navigate to="/login" />;
}

function AdminRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, user, _hasHydrated } = useAuthStore();
  
  if (!_hasHydrated) {
    return <div className="min-h-screen bg-dark-500 flex items-center justify-center">
      <div className="text-white">Loading...</div>
    </div>;
  }
  
  if (!isAuthenticated) return <Navigate to="/login" />;
  if (user?.role !== 'admin') return <Navigate to="/" />;
  return <>{children}</>;
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        path="/"
        element={
          <PrivateRoute>
            <Layout />
          </PrivateRoute>
        }
      >
        <Route index element={<SpreadList />} />
        <Route path="spread/:spreadId" element={<SpreadDetail />} />
        <Route path="positions" element={<Positions />} />
        <Route path="api-keys" element={<ApiKeys />} />
        <Route
          path="admin/users"
          element={
            <AdminRoute>
              <AdminUsers />
            </AdminRoute>
          }
        />
      </Route>
    </Routes>
  );
}
