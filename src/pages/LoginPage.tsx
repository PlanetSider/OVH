import { useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { motion } from 'framer-motion';
import { Shield, KeyRound } from 'lucide-react';
import { api, setApiSecretKey } from '@/utils/apiClient';
import { toast } from 'sonner';


const LoginPage = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const [apiSecretKey, setApiSecretKeyInput] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  const targetPath = ((location.state as { from?: { pathname?: string } } | null)?.from?.pathname) || '/';

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    const normalizedKey = apiSecretKey.trim();
    if (!normalizedKey) {
      toast.error('请输入访问密码');
      return;
    }

    setIsSubmitting(true);
    try {
      setApiSecretKey(normalizedKey);
      await api.get('/auth/check');
      toast.success('访问密码验证成功');
      navigate(targetPath, { replace: true });
    } catch (error: any) {
      if (error?.response?.status === 401) {
        toast.error('访问密码不正确，请检查后重试');
      } else {
        toast.error(error?.response?.data?.error || error?.message || '无法验证访问密码');
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="min-h-screen bg-cyber-bg cyber-grid-bg text-cyber-text flex items-center justify-center px-4">
      <motion.div
        initial={{ opacity: 0, y: 18 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.25 }}
        className="w-full max-w-md cyber-panel p-8"
      >
        <div className="flex items-center gap-3 mb-6">
          <div className="p-3 rounded-xl bg-cyber-accent/10 border border-cyber-accent/30">
            <Shield className="w-6 h-6 text-cyber-accent" />
          </div>
          <div>
            <h1 className="text-2xl font-bold cyber-glow-text">访问登录</h1>
            <p className="text-sm text-cyber-muted mt-1">请输入后端配置的访问密码以进入面板</p>
          </div>
        </div>

        <form onSubmit={handleSubmit} className="space-y-5">
          <div>
            <label className="block text-sm text-cyber-muted mb-2">访问密码</label>
            <div className="relative">
              <KeyRound className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-cyber-muted" />
              <input
                type="password"
                value={apiSecretKey}
                onChange={(e) => setApiSecretKeyInput(e.target.value)}
                className="cyber-input w-full pl-10"
                placeholder="请输入 API_SECRET_KEY"
                autoFocus
              />
            </div>
            <p className="text-xs text-cyber-muted mt-2">该密码需与容器环境变量或 `backend/.env` 中的 `API_SECRET_KEY` 完全一致。</p>
          </div>

          <button
            type="submit"
            disabled={isSubmitting}
            className="cyber-button w-full flex items-center justify-center gap-2"
          >
            {isSubmitting ? '验证中...' : '进入面板'}
          </button>
        </form>
      </motion.div>
    </div>
  );
};


export default LoginPage;
