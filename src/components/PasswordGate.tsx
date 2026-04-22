import { ReactNode, useEffect, useState } from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { api, getApiSecretKey } from '@/utils/apiClient';

interface PasswordGateProps {
  children: ReactNode;
}

const PasswordGate: React.FC<PasswordGateProps> = ({ children }) => {
  const location = useLocation();
  const [status, setStatus] = useState<'checking' | 'allowed' | 'blocked'>('checking');

  if (location.pathname === '/login') {
    return <>{children}</>;
  }

  useEffect(() => {
    let active = true;

    const verify = async () => {
      const apiSecretKey = getApiSecretKey().trim();
      if (!apiSecretKey) {
        if (active) setStatus('blocked');
        return;
      }

      try {
        await api.get('/settings');
        if (active) setStatus('allowed');
      } catch {
        if (active) setStatus('blocked');
      }
    };

    verify();
    return () => {
      active = false;
    };
  }, [location.pathname]);

  if (status === 'checking') {
    return (
      <div className="min-h-screen flex items-center justify-center bg-cyber-bg cyber-grid-bg text-cyber-text">
        <div className="flex items-center gap-3 cyber-panel px-6 py-4">
          <div className="animate-spin w-6 h-6 border-2 border-cyber-accent border-t-transparent rounded-full"></div>
          <span className="text-sm text-cyber-muted">正在验证访问权限...</span>
        </div>
      </div>
    );
  }

  if (status === 'blocked') {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }

  return <>{children}</>;
};

export default PasswordGate;
