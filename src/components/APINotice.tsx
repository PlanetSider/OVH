import { Link } from "react-router-dom";
import { motion } from "framer-motion";
import { useEffect, useState } from "react";
import { useAPI } from "@/context/APIContext";
import { api, getApiSecretKey } from "@/utils/apiClient";

const APINotice = () => {
  const { isAuthenticated } = useAPI();
  const [passwordValid, setPasswordValid] = useState<boolean | null>(null);
  useEffect(() => {
    (async () => {
      try {
        const key = getApiSecretKey();
        if (!key) {
          setPasswordValid(false);
          return;
        }
        await api.get('/auth/check');
        setPasswordValid(true);
      } catch {
        setPasswordValid(false);
      }
    })();
  }, []);
  const showPasswordNotice = passwordValid === false;
  const showAPINotice = passwordValid === true && !isAuthenticated;
  if (!showPasswordNotice && !showAPINotice) return null;
  const targetLink = showPasswordNotice ? '/login' : '/api-accounts';
  return (
    <motion.div
      initial={{ opacity: 0, y: -20 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3 }}
      className="mb-6 cyber-panel p-4 text-center"
    >
      <div className="flex flex-col md:flex-row items-center justify-center gap-4">
        <svg
          xmlns="http://www.w3.org/2000/svg"
          width="24"
          height="24"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="text-yellow-400"
        >
          <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path>
          <line x1="12" y1="9" x2="12" y2="13"></line>
          <line x1="12" y1="17" x2="12.01" y2="17"></line>
        </svg>
        <div className="text-cyber-muted">
          {showPasswordNotice ? '尚未通过访问密码验证，请先登录进入面板。' : '您尚未配置 OVH API，某些功能将无法正常使用。'}
        </div>
        <Link
          to={targetLink}
          className="cyber-button text-xs px-3 py-1 flex items-center justify-center"
        >
          {showPasswordNotice ? '前往登录' : '配置 API'}
        </Link>
      </div>
    </motion.div>
  );
};

export default APINotice;
