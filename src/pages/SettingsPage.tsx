import { useState, useEffect } from "react";
import { createPortal } from "react-dom";
import { motion, AnimatePresence } from "framer-motion";
import { useAPI } from "@/context/APIContext";
import { toast } from "sonner";
import { useNavigate } from "react-router-dom";
import { CacheManager } from "@/components/CacheManager";
import { useIsMobile } from "@/hooks/use-mobile";
import { getApiSecretKey, setApiSecretKey } from "@/utils/apiClient";
import { api } from "@/utils/apiClient";
import { X, AlertCircle, FileText, CheckCircle, AlertTriangle } from "lucide-react";

const SettingsPage = () => {
  const isMobile = useIsMobile();
  const navigate = useNavigate();
  const { 
    tgToken,
    tgChatId,
    feishuEnabled,
    feishuAppId,
    feishuAppSecret,
    feishuVerificationToken,
    feishuEncryptKey,
    serversAutoRefreshEnabled,
    serversAutoRefreshIntervalSeconds,
    availabilityAutoRefreshEnabled,
    availabilityAutoRefreshIntervalSeconds,
    serversNewServerNotifyEnabled,
    availabilityNewServerNotifyEnabled,
    primaryRefreshAccountId,
    serverInventoryRefreshEnabled,
    serverInventoryRefreshIntervalSeconds,
    accounts,
    isLoading,
    checkAuthentication
  } = useAPI();

  const [formValues, setFormValues] = useState({
    apiSecretKey: "",
    tgToken: "",
    tgChatId: "",
    feishuEnabled: false,
    feishuAppId: "",
    feishuAppSecret: "",
    feishuVerificationToken: "",
    feishuEncryptKey: "",
    serversAutoRefreshEnabled: false,
    serversAutoRefreshIntervalSeconds: 3600,
    availabilityAutoRefreshEnabled: false,
    availabilityAutoRefreshIntervalSeconds: 3600,
    serversNewServerNotifyEnabled: false,
    availabilityNewServerNotifyEnabled: false,
    primaryRefreshAccountId: "",
    serverInventoryRefreshEnabled: false,
    serverInventoryRefreshIntervalSeconds: 3600,
    sshKey: ""
  });
  const [isSaving, setIsSaving] = useState(false);
  const [showValues, setShowValues] = useState({
    apiSecretKey: false,
    tgToken: false,
    feishuAppSecret: false
  });
  
  // Telegram Webhook 相关状态
  const [webhookUrl, setWebhookUrl] = useState("");
  const [webhookInfo, setWebhookInfo] = useState<any>(null);
  const [isSettingWebhook, setIsSettingWebhook] = useState(false);
  const [isLoadingWebhookInfo, setIsLoadingWebhookInfo] = useState(false);
  const [showErrorHistoryDialog, setShowErrorHistoryDialog] = useState(false);
  const [apiKeyValid, setApiKeyValid] = useState<boolean | null>(null);
  const [ovhAuthValid, setOvhAuthValid] = useState<boolean | null>(null);
  const [feishuBinding, setFeishuBinding] = useState<any>(null);
  const [isLoadingFeishuBinding, setIsLoadingFeishuBinding] = useState(false);
  const [isSendingFeishuTestCard, setIsSendingFeishuTestCard] = useState(false);
  const [isClearingFeishuBinding, setIsClearingFeishuBinding] = useState(false);

  useEffect(() => {
    setFormValues(prev => ({
      ...prev,
      apiSecretKey: getApiSecretKey() || "",
      tgToken: tgToken || "",
      tgChatId: tgChatId || "",
      feishuEnabled: !!feishuEnabled,
      feishuAppId: feishuAppId || "",
      feishuAppSecret: feishuAppSecret || "",
      feishuVerificationToken: feishuVerificationToken || "",
      feishuEncryptKey: feishuEncryptKey || "",
      serversAutoRefreshEnabled: !!serversAutoRefreshEnabled,
      serversAutoRefreshIntervalSeconds: serversAutoRefreshIntervalSeconds || 3600,
      availabilityAutoRefreshEnabled: !!availabilityAutoRefreshEnabled,
      availabilityAutoRefreshIntervalSeconds: availabilityAutoRefreshIntervalSeconds || 3600,
      serversNewServerNotifyEnabled: !!serversNewServerNotifyEnabled,
      availabilityNewServerNotifyEnabled: !!availabilityNewServerNotifyEnabled,
      primaryRefreshAccountId: primaryRefreshAccountId || "",
      serverInventoryRefreshEnabled: !!serverInventoryRefreshEnabled,
      serverInventoryRefreshIntervalSeconds: serverInventoryRefreshIntervalSeconds || 3600
    }));
  }, [tgToken, tgChatId, feishuEnabled, feishuAppId, feishuAppSecret, feishuVerificationToken, feishuEncryptKey, serversAutoRefreshEnabled, serversAutoRefreshIntervalSeconds, availabilityAutoRefreshEnabled, availabilityAutoRefreshIntervalSeconds, serversNewServerNotifyEnabled, availabilityNewServerNotifyEnabled, primaryRefreshAccountId, serverInventoryRefreshEnabled, serverInventoryRefreshIntervalSeconds]);

  // 加载后端设置中的 SSH 公钥
  useEffect(() => {
    (async () => {
      try {
        const resp = await api.get('/settings');
        const cfg = resp.data || {};
        setFormValues(prev => ({
          ...prev,
          sshKey: cfg.sshKey || "",
          tgToken: cfg.tgToken || prev.tgToken || "",
          tgChatId: cfg.tgChatId || prev.tgChatId || "",
          feishuEnabled: !!cfg.feishuEnabled,
          feishuAppId: cfg.feishuAppId || prev.feishuAppId || "",
          feishuAppSecret: cfg.feishuAppSecret || prev.feishuAppSecret || "",
          feishuVerificationToken: cfg.feishuVerificationToken || prev.feishuVerificationToken || "",
          feishuEncryptKey: cfg.feishuEncryptKey || prev.feishuEncryptKey || "",
          serversAutoRefreshEnabled: !!cfg.serversAutoRefreshEnabled,
          serversAutoRefreshIntervalSeconds: Number(cfg.serversAutoRefreshIntervalSeconds || prev.serversAutoRefreshIntervalSeconds || 3600),
          availabilityAutoRefreshEnabled: !!cfg.availabilityAutoRefreshEnabled,
          availabilityAutoRefreshIntervalSeconds: Number(cfg.availabilityAutoRefreshIntervalSeconds || prev.availabilityAutoRefreshIntervalSeconds || 3600),
          serversNewServerNotifyEnabled: !!cfg.serversNewServerNotifyEnabled,
          availabilityNewServerNotifyEnabled: !!cfg.availabilityNewServerNotifyEnabled,
          primaryRefreshAccountId: cfg.primaryRefreshAccountId || prev.primaryRefreshAccountId || "",
          serverInventoryRefreshEnabled: !!cfg.serverInventoryRefreshEnabled,
          serverInventoryRefreshIntervalSeconds: Number(cfg.serverInventoryRefreshIntervalSeconds || prev.serverInventoryRefreshIntervalSeconds || 3600)
        }));
      } catch {}
    })();
  }, []);

  // 加载 Webhook 信息（可选功能，失败不显示错误）
  const loadWebhookInfo = async () => {
    if (!tgToken) {
      // 没有token时，不尝试加载
      setIsLoadingWebhookInfo(false);
      return;
    }
    
    setIsLoadingWebhookInfo(true);
    try {
      const response = await api.get('/telegram/get-webhook-info');
      const data = response.data;
      if (data.success && data.webhook_info) {
        setWebhookInfo(data.webhook_info);
        if (data.webhook_info.url) {
          setWebhookUrl(data.webhook_info.url.replace('/api/telegram/webhook', ''));
        }
      }
    } catch (error: any) {
      // 静默失败，webhook是可选的
      console.log('Webhook 功能未配置或不可用（这是可选的）');
      setWebhookInfo(null);
    } finally {
      setIsLoadingWebhookInfo(false);
    }
  };

  // 自动检测 Webhook URL（使用当前页面的域名）
  const autoDetectWebhookUrl = () => {
    const currentUrl = window.location.origin;
    setWebhookUrl(currentUrl);
  };

  // 设置 Webhook
  const handleSetWebhook = async () => {
    if (!tgToken) {
      toast.error('请先配置 Telegram Bot Token');
      return;
    }
    
    if (!webhookUrl.trim()) {
      toast.error('请输入 Webhook URL');
      return;
    }

    if (!webhookUrl.startsWith('https://')) {
      toast.error('Webhook URL 必须为 HTTPS，例如：https://your-domain.com');
      return;
    }

    setIsSettingWebhook(true);
    try {
      const response = await api.post('/telegram/set-webhook', {
        webhook_url: webhookUrl
      });
      
      const data = response.data;
      
      if (data.success) {
        toast.success('Webhook 设置成功！');
        setWebhookInfo(data.webhook_info);
        // 重新加载信息
        await loadWebhookInfo();
      } else {
        toast.error(data.error || '设置失败');
      }
    } catch (error: any) {
      const errorMsg = error.response?.data?.error || error.message || '未知错误';
      toast.error('设置失败：' + errorMsg);
    } finally {
      setIsSettingWebhook(false);
    }
  };

  // 组件加载时获取 Webhook 信息
  useEffect(() => {
    if (tgToken) {
      loadWebhookInfo();
      autoDetectWebhookUrl();
    }
  }, [tgToken]);

  const loadFeishuBinding = async () => {
    setIsLoadingFeishuBinding(true);
    try {
      const response = await api.get('/feishu/binding');
      setFeishuBinding(response.data || null);
    } catch {
      setFeishuBinding(null);
    } finally {
      setIsLoadingFeishuBinding(false);
    }
  };

  const handleSendFeishuTestCard = async () => {
    setIsSendingFeishuTestCard(true);
    try {
      const response = await api.post('/feishu/test-card');
      if (response.data?.success) {
        toast.success(response.data.message || '测试交互卡片已发送');
      } else {
        toast.error(response.data?.error || '发送失败');
      }
    } catch (error: any) {
      toast.error(error?.response?.data?.error || error?.message || '发送失败');
    } finally {
      setIsSendingFeishuTestCard(false);
    }
  };

  const handleClearFeishuBinding = async () => {
    setIsClearingFeishuBinding(true);
    try {
      const response = await api.delete('/feishu/binding');
      if (response.data?.success) {
        toast.success(response.data.cleared ? '已清除飞书绑定' : '当前没有飞书绑定');
        await loadFeishuBinding();
      } else {
        toast.error('清除失败');
      }
    } catch (error: any) {
      toast.error(error?.response?.data?.error || error?.message || '清除失败');
    } finally {
      setIsClearingFeishuBinding(false);
    }
  };

  useEffect(() => {
    loadFeishuBinding();
  }, []);

  // 计算错误信息辅助函数
  const getErrorInfo = () => {
    if (!webhookInfo?.last_error_date) return null;
    
    const errorDate = new Date(webhookInfo.last_error_date * 1000);
    const now = new Date();
    const msSinceError = now.getTime() - errorDate.getTime();
    const hoursSinceError = msSinceError / (1000 * 60 * 60);
    const daysSinceError = msSinceError / (1000 * 60 * 60 * 24);
    const isRecentError = hoursSinceError < 24;
    
    const formatRelativeTime = () => {
      if (hoursSinceError < 1) {
        const minutes = Math.floor(msSinceError / (1000 * 60));
        return `${minutes}分钟前`;
      } else if (hoursSinceError < 24) {
        return `${Math.floor(hoursSinceError)}小时前`;
      } else if (daysSinceError < 7) {
        return `${Math.floor(daysSinceError)}天前`;
      } else {
        return errorDate.toLocaleDateString('zh-CN');
      }
    };
    
    return {
      errorDate,
      isRecentError,
      formatRelativeTime,
      hoursSinceError,
      daysSinceError
    };
  };

  useEffect(() => {
    (async () => {
      try {
        const key = getApiSecretKey();
        if (!key) {
          setApiKeyValid(false);
        } else {
          await api.get('/settings');
          setApiKeyValid(true);
        }
      } catch {
        setApiKeyValid(false);
      }
    })();
  }, []);

  

  // Handle input changes
  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>) => {
    const { name, value, type } = e.target;
    setFormValues({
      ...formValues,
      [name]: type === 'checkbox' ? (e.target as HTMLInputElement).checked : value
    });
  };

  // Toggle password visibility
  const toggleShowValue = (field: keyof typeof showValues) => {
    setShowValues({
      ...showValues,
      [field]: !showValues[field]
    });
  };

  // Save settings
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    // Validate API Secret Key
    const normalizedApiSecretKey = (formValues.apiSecretKey || '').trim();
    if (!normalizedApiSecretKey) {
      toast.error("请设置访问密码");
      return;
    }
    
    setIsSaving(true);
    try {
      // 1. 先保存访问密码到 localStorage，并立即用 /settings 校验密码是否正确
      setApiSecretKey(normalizedApiSecretKey);
      setFormValues(prev => ({ ...prev, apiSecretKey: normalizedApiSecretKey }));
      
      // 等待一下确保 localStorage 写入完成
      await new Promise(resolve => setTimeout(resolve, 100));

      try {
        await api.get('/settings');
      } catch (error: any) {
        const status = error?.response?.status;
        if (status === 401) {
          toast.error('访问密码不正确，请检查后重试');
        } else {
          toast.error(error?.response?.data?.error || error?.message || '无法验证访问密码');
        }
        setIsSaving(false);
        return;
      }
      
      let settingsSaved = false;
      try {
        await api.post('/settings', {
          tgToken: formValues.tgToken || undefined,
          tgChatId: formValues.tgChatId || undefined,
          feishuEnabled: formValues.feishuEnabled,
          feishuAppId: formValues.feishuAppId || undefined,
          feishuAppSecret: formValues.feishuAppSecret || undefined,
          feishuVerificationToken: formValues.feishuVerificationToken || undefined,
          feishuEncryptKey: formValues.feishuEncryptKey || undefined,
          serversAutoRefreshEnabled: formValues.serversAutoRefreshEnabled,
          serversAutoRefreshIntervalSeconds: Number(formValues.serversAutoRefreshIntervalSeconds || 3600),
          availabilityAutoRefreshEnabled: formValues.availabilityAutoRefreshEnabled,
          availabilityAutoRefreshIntervalSeconds: Number(formValues.availabilityAutoRefreshIntervalSeconds || 3600),
          serversNewServerNotifyEnabled: formValues.serversNewServerNotifyEnabled,
          availabilityNewServerNotifyEnabled: formValues.availabilityNewServerNotifyEnabled,
          primaryRefreshAccountId: formValues.primaryRefreshAccountId || undefined,
          serverInventoryRefreshEnabled: formValues.serverInventoryRefreshEnabled,
          serverInventoryRefreshIntervalSeconds: Number(formValues.serverInventoryRefreshIntervalSeconds || 3600),
          sshKey: formValues.sshKey || undefined
        });
        settingsSaved = true;
      } catch (err: any) {
        console.error('Error saving notification settings:', err);
      }

      if (settingsSaved) {
        toast.success("访问密码与通知配置已保存，页面将刷新");
      } else {
        toast.success("访问密码验证通过，页面将刷新进入面板");
        toast.warning("其它设置保存失败，请进入面板后重新保存通知配置");
      }

      setTimeout(() => { window.location.reload(); }, 800);

      // 无论是否有OVH配置，确保SSH设置已同步保存
      try {
        await api.post('/settings', {
          sshKey: formValues.sshKey || undefined
        });
      } catch {}
    } catch (error) {
      console.error("Error saving settings:", error);
      toast.error("保存设置失败");
      setIsSaving(false);
    }
  };

  

  return (
    <div className="space-y-4 sm:space-y-6">
      <motion.div
        initial={{ opacity: 0, y: -20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.3 }}
      >
        <h1 className={`${isMobile ? 'text-2xl' : 'text-3xl'} font-bold mb-1 cyber-glow-text`}>设置</h1>
        <p className="text-cyber-muted text-sm mb-4 sm:mb-6">配置通知、自动刷新和系统运行参数</p>
      </motion.div>

      {isLoading ? (
        <div className="flex justify-center items-center h-64">
          <div className="animate-spin w-10 h-10 border-4 border-cyber-accent border-t-transparent rounded-full"></div>
          <span className="ml-3 text-cyber-muted">加载中...</span>
        </div>
      ) : (
        <form onSubmit={handleSubmit} className="grid grid-cols-1 lg:grid-cols-3 gap-4 sm:gap-6">
          <div className="lg:col-span-2">
            <div className="cyber-panel p-4 sm:p-6 space-y-4 sm:space-y-6">
              {/* 访问密码（仅用于更新浏览器内保存的访问密钥） */}
              <div>
                <h2 className={`${isMobile ? 'text-lg' : 'text-xl'} font-bold mb-3 sm:mb-4`}>🔐 访问密码</h2>
                <div className="bg-yellow-500/10 border border-yellow-500/30 rounded-lg p-3 mb-4">
                  <p className="text-xs text-yellow-300">
                    ⚠️ 此密码用于保护前后端通信和面板访问，需要与后端配置保持一致。请妥善保管，不要泄露！
                  </p>
                </div>
                
                <div>
                  <label className="block text-cyber-muted mb-1 text-xs sm:text-sm">
                    访问密码 <span className="text-red-400">*</span>
                  </label>
                  <div className="relative">
                    <input
                      type={showValues.apiSecretKey ? "text" : "password"}
                      name="apiSecretKey"
                      value={formValues.apiSecretKey}
                      onChange={handleChange}
                      className="cyber-input w-full pr-10 text-sm"
                      placeholder="输入访问密码（在Docker设置的environment或后端.env文件中的API_SECRET_KEY）"
                      required
                    />
                    <button
                      type="button"
                      onClick={() => toggleShowValue("apiSecretKey")}
                      className="absolute inset-y-0 right-0 px-3 text-cyber-muted hover:text-cyber-accent"
                    >
                      {showValues.apiSecretKey ? (
                        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                          <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"></path>
                          <line x1="1" y1="1" x2="23" y2="23"></line>
                        </svg>
                      ) : (
                        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                          <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"></path>
                          <circle cx="12" cy="12" r="3"></circle>
                        </svg>
                      )}
                    </button>
                  </div>
                  <div className="text-xs text-cyan-400 mt-2 space-y-1">
                    <p>💡 请在Docker的 <code className="bg-cyan-500/20 px-1 py-0.5 rounded">environment</code> 参数或 <code className="bg-cyan-500/20 px-1 py-0.5 rounded">backend/.env</code> 文件中查找 <code className="bg-cyan-500/20 px-1 py-0.5 rounded">API_SECRET_KEY</code> 的值并复制到此处</p>
                    <p className="text-purple-300">
                      <strong>用途：</strong>用于前后端通信安全验证，同时作为面板登录密码
                    </p>
                    <p className="text-yellow-300">
                      ⚡ 登录入口已独立为 <strong>/login</strong> 页面；此处主要用于更新当前浏览器保存的访问密钥
                    </p>
                  </div>
                </div>
              </div>
              
              

              {/* SSH 公钥（全局） */}
              <div className="cyber-grid-line pt-4">
                <h2 className="text-xl font-bold mb-4">全局 SSH公钥 (可选)</h2>
                <p className="text-xs text-cyber-muted mb-2">为所有账户的Linux系统安装统一预置SSH免密登录公钥</p>
                <textarea
                  name="sshKey"
                  value={formValues.sshKey}
                  onChange={handleChange}
                  placeholder="ssh-rsa 或 ssh-ed25519 公钥行（完整）"
                  className="cyber-input w-full h-24"
                />
                <p className="text-xs text-cyan-400 mt-1">Windows 模板不适用 SSH 公钥（会被忽略）</p>
              </div>
              
              <div className="cyber-grid-line pt-4">
                <h2 className="text-xl font-bold mb-4">📱 Telegram 通知设置 (可选)</h2>
                
                <div className="space-y-4">
                  <div>
                    <label className="block text-cyber-muted mb-1">
                      Telegram Bot Token
                    </label>
                    <div className="relative">
                      <input
                        type={showValues.tgToken ? "text" : "password"}
                        name="tgToken"
                        value={formValues.tgToken}
                        onChange={handleChange}
                        className="cyber-input w-full pr-10"
                        placeholder="123456789:ABCDEFGH..."
                      />
                      <button
                        type="button"
                        onClick={() => toggleShowValue("tgToken")}
                        className="absolute inset-y-0 right-0 px-3 text-cyber-muted hover:text-cyber-accent"
                      >
                        {showValues.tgToken ? (
                          <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                            <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"></path>
                            <line x1="1" y1="1" x2="23" y2="23"></line>
                          </svg>
                        ) : (
                          <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                            <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"></path>
                            <circle cx="12" cy="12" r="3"></circle>
                          </svg>
                        )}
                      </button>
                    </div>
                  </div>
                  
                  <div>
                    <label className="block text-cyber-muted mb-1">
                      Telegram Chat ID
                    </label>
                    <input
                      type="text"
                      name="tgChatId"
                      value={formValues.tgChatId}
                      onChange={handleChange}
                      className="cyber-input w-full"
                      placeholder="-100123456789"
                    />
                  </div>

                  {/* Telegram Webhook 设置 */}
                  <div className="cyber-grid-line pt-4 mt-4">
                    <h3 className="text-lg font-semibold mb-3">📱 Telegram Webhook 设置 (可选)</h3>
                    <p className="text-xs text-cyber-muted mb-4">
                      设置 Webhook 后，当服务器有货时可以在 Telegram 中直接点击按钮加入抢购队列
                    </p>
                    
                    <div className="space-y-3">
                      <div>
                        <label className="block text-cyber-muted mb-1 text-sm">
                          Webhook URL（自动检测当前域名，可手动修改）
                        </label>
                        <div className="flex flex-col sm:flex-row gap-2">
                          <input
                            type="text"
                            value={webhookUrl}
                            onChange={(e) => setWebhookUrl(e.target.value)}
                            className="cyber-input flex-1 min-w-0"
                            placeholder="https://your-domain.com"
                          />
                          <button
                            type="button"
                            onClick={autoDetectWebhookUrl}
                            className="cyber-button px-3 sm:px-4 whitespace-nowrap flex-shrink-0 text-xs sm:text-sm"
                            title="自动检测当前域名"
                          >
                            自动检测
                          </button>
                        </div>
                        <p className="text-xs text-cyber-muted mt-1 break-words">
                          完整 URL 将自动添加：{webhookUrl || 'https://your-domain.com'}/api/telegram/webhook
                        </p>
                      </div>

                      <div className="flex flex-col sm:flex-row gap-2">
                        <button
                          type="button"
                          onClick={handleSetWebhook}
                          disabled={isSettingWebhook || !tgToken}
                          className="cyber-button flex-1 text-xs sm:text-sm"
                        >
                          {isSettingWebhook ? (
                            <span className="flex items-center justify-center">
                              <svg className="animate-spin -ml-1 mr-2 h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                              </svg>
                              设置中...
                            </span>
                          ) : (
                            '设置 Webhook'
                          )}
                        </button>
                        <button
                          type="button"
                          onClick={loadWebhookInfo}
                          disabled={isLoadingWebhookInfo || !tgToken}
                          className="cyber-button px-3 sm:px-4 flex-shrink-0 text-xs sm:text-sm"
                          title="刷新状态"
                        >
                          {isLoadingWebhookInfo ? (
                            <svg className="animate-spin h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                            </svg>
                          ) : (
                            '刷新'
                          )}
                        </button>
                      </div>

                      {/* 显示 Webhook 状态 */}
                      {webhookInfo && (
                        <div className="bg-gradient-to-br from-cyber-dark/50 to-cyber-dark/30 border border-cyber-accent/20 rounded-lg p-3 sm:p-4 space-y-3">
                          <div className="flex items-center justify-between pb-3 border-b border-cyber-accent/10">
                            <span className="text-xs sm:text-sm text-cyber-muted font-medium">当前状态</span>
                            <div className={`flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg ${
                              webhookInfo.url 
                                ? 'bg-green-500/20 border border-green-500/40' 
                                : 'bg-yellow-500/20 border border-yellow-500/40'
                            }`}>
                              {webhookInfo.url ? (
                                <CheckCircle className="w-3.5 h-3.5 text-green-400 flex-shrink-0" />
                              ) : (
                                <AlertTriangle className="w-3.5 h-3.5 text-yellow-400 flex-shrink-0" />
                              )}
                              <span className={`text-xs sm:text-sm font-medium ${
                                webhookInfo.url ? 'text-green-400' : 'text-yellow-400'
                              }`}>
                                {webhookInfo.url ? '已设置' : '未设置'}
                              </span>
                            </div>
                          </div>
                          
                          {webhookInfo.url && (
                            <>
                              <div>
                                <span className="text-xs text-cyber-muted block mb-1.5 font-medium">Webhook URL</span>
                                <code className="text-xs sm:text-sm bg-cyber-dark/80 p-2 rounded border border-cyber-accent/10 block break-all font-mono leading-relaxed">
                                  {webhookInfo.url}
                                </code>
                              </div>
                              
                              {webhookInfo.pending_update_count !== undefined && (
                                <div className="flex items-center justify-between p-2 bg-cyber-dark/30 rounded border border-cyber-accent/10">
                                  <span className="text-xs text-cyber-muted">待处理更新</span>
                                  <span className={`text-xs font-mono font-semibold px-2 py-0.5 rounded ${
                                    webhookInfo.pending_update_count === 0
                                      ? 'bg-green-500/20 text-green-400'
                                      : 'bg-yellow-500/20 text-yellow-400'
                                  }`}>
                                    {webhookInfo.pending_update_count}
                                  </span>
                                </div>
                              )}
                              
                              {webhookInfo.last_error_date && (() => {
                                const errorInfo = getErrorInfo();
                                if (!errorInfo) return null;
                                
                                return (
                                  <button
                                    type="button"
                                    onClick={() => setShowErrorHistoryDialog(true)}
                                    className={`w-full mt-2 border rounded-lg p-3 transition-all text-left hover:opacity-80 ${
                                      errorInfo.isRecentError
                                        ? 'bg-red-500/10 border-red-500/30 hover:bg-red-500/15'
                                        : 'bg-yellow-500/10 border-yellow-500/30 hover:bg-yellow-500/15'
                                    }`}
                                  >
                                    <div className="flex items-center justify-between gap-2 mb-1">
                                      <div className="flex items-center gap-1.5">
                                        <FileText className={`w-3.5 h-3.5 ${
                                          errorInfo.isRecentError ? 'text-red-400' : 'text-yellow-400'
                                        }`} />
                                        <span className={`text-xs font-semibold ${
                                          errorInfo.isRecentError ? 'text-red-400' : 'text-yellow-400'
                                        }`}>
                                          {errorInfo.isRecentError ? '⚠️ 最后错误' : '📋 历史错误'}
                                        </span>
                                      </div>
                                      <span className={`text-[10px] px-1.5 py-0.5 rounded ${
                                        errorInfo.isRecentError
                                          ? 'bg-red-500/20 text-red-300'
                                          : 'bg-yellow-500/20 text-yellow-300'
                                      }`}>
                                        {errorInfo.formatRelativeTime()}
                                      </span>
                                    </div>
                                    <div className="text-xs text-cyber-muted mt-1">
                                      点击查看详细信息
                                    </div>
                                  </button>
                                );
                              })()}
                            </>
                          )}
                        </div>
                      )}

                      {!tgToken && (
                        <div className="bg-yellow-500/10 border border-yellow-500/30 rounded p-3">
                          <p className="text-xs text-yellow-300">
                            ⚠️ 请先配置 Telegram Bot Token 才能设置 Webhook（此功能为可选）
                          </p>
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              </div>

              <div className="cyber-grid-line pt-4">
                <h2 className="text-xl font-bold mb-4">🪽 飞书应用机器人设置 (可选)</h2>
                <div className="space-y-4">
                  <label className="flex items-center gap-2 text-sm text-cyber-text">
                    <input
                      type="checkbox"
                      name="feishuEnabled"
                      checked={formValues.feishuEnabled}
                      onChange={handleChange}
                      className="form-checkbox cyber-input h-4 w-4"
                    />
                    启用飞书应用机器人通道
                  </label>
                  <div>
                    <label className="block text-cyber-muted mb-1">飞书 App ID</label>
                    <input
                      type="text"
                      name="feishuAppId"
                      value={formValues.feishuAppId}
                      onChange={handleChange}
                      className="cyber-input w-full"
                      placeholder="cli_xxxxxxxxxxxxx"
                    />
                  </div>
                  <div>
                    <label className="block text-cyber-muted mb-1">飞书 App Secret</label>
                    <div className="relative">
                      <input
                        type={showValues.feishuAppSecret ? 'text' : 'password'}
                        name="feishuAppSecret"
                        value={formValues.feishuAppSecret}
                        onChange={handleChange}
                        className="cyber-input w-full pr-10"
                        placeholder="应用密钥"
                      />
                      <button type="button" onClick={() => toggleShowValue('feishuAppSecret' as keyof typeof showValues)} className="absolute inset-y-0 right-0 px-3 text-cyber-muted hover:text-cyber-accent">
                        {showValues.feishuAppSecret ? '🙈' : '👁️'}
                      </button>
                    </div>
                  </div>
                  <div>
                    <label className="block text-cyber-muted mb-1">校验 Token</label>
                    <input
                      type="text"
                      name="feishuVerificationToken"
                      value={formValues.feishuVerificationToken}
                      onChange={handleChange}
                      className="cyber-input w-full"
                      placeholder="事件订阅 Verification Token"
                    />
                  </div>
                  <div>
                    <label className="block text-cyber-muted mb-1">Encrypt Key</label>
                    <input
                      type="text"
                      name="feishuEncryptKey"
                      value={formValues.feishuEncryptKey}
                      onChange={handleChange}
                      className="cyber-input w-full"
                      placeholder="事件订阅 Encrypt Key（可选）"
                    />
                  </div>
                  <div className="bg-cyber-grid/10 border border-cyber-accent/20 rounded-lg p-3 text-xs text-cyber-muted space-y-1">
                    <p>事件回调地址：<code>{window.location.origin}/api/feishu/events</code></p>
                    <p>卡片回调地址：<code>{window.location.origin}/api/feishu/card-action</code></p>
                    <p>私聊用户首次给机器人发消息后，系统会自动绑定该用户用于后续飞书通知。</p>
                  </div>
                  <div className="bg-cyber-dark/30 border border-cyber-accent/10 rounded-lg p-3 space-y-3">
                    <div className="flex items-center justify-between gap-3">
                      <div>
                        <div className="text-sm text-cyber-text font-medium">当前账户飞书绑定</div>
                        <div className="text-xs text-cyber-muted mt-1">
                          {isLoadingFeishuBinding ? '加载中...' : feishuBinding?.bound ? '已绑定飞书私聊用户' : '未绑定飞书私聊用户'}
                        </div>
                      </div>
                      <button
                        type="button"
                        onClick={loadFeishuBinding}
                        className="cyber-button px-3 text-xs"
                        disabled={isLoadingFeishuBinding}
                      >
                        刷新绑定
                      </button>
                    </div>
                    {feishuBinding?.bound && (
                      <div className="text-xs text-cyber-muted space-y-1">
                        <p>Open ID：<code>{feishuBinding.binding?.open_id}</code></p>
                        {feishuBinding.binding?.user_name && <p>用户标识：<code>{feishuBinding.binding.user_name}</code></p>}
                      </div>
                    )}
                    <div className="flex flex-col sm:flex-row gap-2">
                      <button
                        type="button"
                        onClick={handleSendFeishuTestCard}
                        className="cyber-button flex-1 text-xs sm:text-sm"
                        disabled={isSendingFeishuTestCard || !feishuBinding?.bound || !formValues.feishuEnabled}
                      >
                        {isSendingFeishuTestCard ? '发送中...' : '发送飞书测试交互卡片'}
                      </button>
                      <button
                        type="button"
                        onClick={handleClearFeishuBinding}
                        className="cyber-button px-3 sm:px-4 text-xs sm:text-sm bg-red-900/30 border-red-700/40 text-red-300 hover:bg-red-800/40 hover:border-red-600/50 hover:text-red-200"
                        disabled={isClearingFeishuBinding}
                      >
                        {isClearingFeishuBinding ? '清除中...' : '清除绑定'}
                      </button>
                    </div>
                  </div>
                </div>
              </div>

              <div className="flex justify-end pt-4">
                <button
                  type="submit"
                  className="cyber-button px-6"
                  disabled={isSaving}
                >
                  {isSaving ? (
                    <span className="flex items-center">
                      <svg className="animate-spin -ml-1 mr-2 h-4 w-4 text-cyber-text" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                        <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                        <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                      </svg>
                      保存中...
                    </span>
                  ) : "保存设置"}
                </button>
              </div>
            </div>
          </div>
          
          <div>
            <div className="cyber-panel p-6">
              <h2 className="text-lg font-bold mb-4">连接状态</h2>
              
              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <div className={`w-3 h-3 rounded-full ${apiKeyValid ? 'bg-green-400 animate-pulse' : 'bg-red-400'}`}></div>
                    <span className={`${apiKeyValid ? 'text-green-400' : 'text-red-400'} text-sm`}>访问密码</span>
                  </div>
                  <span className="text-xs text-cyber-muted">{apiKeyValid === null ? '检测中' : apiKeyValid ? '已通过' : '未设置或不匹配'}</span>
                </div>
                
                
                <div className="cyber-grid-line pt-4">
                  <div className="bg-purple-500/10 border border-purple-500/30 rounded-lg p-3 mb-4">
                    <p className="text-xs text-purple-300 font-semibold mb-1.5">🔐 访问密码提示</p>
                    <p className="text-xs text-purple-200 leading-relaxed">
                      面板登录入口已独立为 <strong>/login</strong>。如果当前浏览器保存的访问密钥需要更新，可在这里重新填写并保存。
                    </p>
                  </div>
                  
                </div>
              </div>
            </div>

            <div className="cyber-panel p-6 mt-6">
              <h2 className="text-lg font-bold mb-4">🔄 自动刷新与新增服务器通知</h2>
              <div className="space-y-4">
                <div>
                  <label className="block text-cyber-muted mb-1 text-sm">主刷新账号</label>
                  <select
                    name="primaryRefreshAccountId"
                    value={formValues.primaryRefreshAccountId}
                    onChange={handleChange}
                    className="cyber-input w-full"
                  >
                    <option value="">自动选择首个账号</option>
                    {accounts.map((account: any) => (
                      <option key={account.id} value={account.id}>
                        {account.conversationLabel || account.alias || account.email || account.id}
                      </option>
                    ))}
                  </select>
                  <p className="text-xs text-cyber-muted mt-2">仅用于服务器列表自动刷新和实时可用性自动刷新，其他监控与任务仍按各自账号独立执行。</p>
                </div>

                <label className="flex items-center gap-2 text-sm text-cyber-text">
                  <input type="checkbox" name="serversAutoRefreshEnabled" checked={formValues.serversAutoRefreshEnabled} onChange={handleChange} className="form-checkbox cyber-input h-4 w-4" />
                  启用服务器列表自动刷新
                </label>
                <input type="number" min={3600} step={3600} name="serversAutoRefreshIntervalSeconds" value={formValues.serversAutoRefreshIntervalSeconds} onChange={handleChange} className="cyber-input w-full" placeholder="服务器列表刷新间隔（秒，最小3600）" />
                <label className="flex items-center gap-2 text-sm text-cyber-text">
                  <input type="checkbox" name="serversNewServerNotifyEnabled" checked={formValues.serversNewServerNotifyEnabled} onChange={handleChange} className="form-checkbox cyber-input h-4 w-4" />
                  启用服务器列表新增服务器通知
                </label>

                <label className="flex items-center gap-2 text-sm text-cyber-text">
                  <input type="checkbox" name="availabilityAutoRefreshEnabled" checked={formValues.availabilityAutoRefreshEnabled} onChange={handleChange} className="form-checkbox cyber-input h-4 w-4" />
                  启用实时可用性自动刷新
                </label>
                <input type="number" min={3600} step={3600} name="availabilityAutoRefreshIntervalSeconds" value={formValues.availabilityAutoRefreshIntervalSeconds} onChange={handleChange} className="cyber-input w-full" placeholder="实时可用性刷新间隔（秒，最小3600）" />
                <label className="flex items-center gap-2 text-sm text-cyber-text">
                  <input type="checkbox" name="availabilityNewServerNotifyEnabled" checked={formValues.availabilityNewServerNotifyEnabled} onChange={handleChange} className="form-checkbox cyber-input h-4 w-4" />
                  启用实时可用性新增服务器通知
                </label>

                <div className="pt-2 border-t border-cyber-accent/20">
                  <label className="flex items-center gap-2 text-sm text-cyber-text">
                    <input type="checkbox" name="serverInventoryRefreshEnabled" checked={formValues.serverInventoryRefreshEnabled} onChange={handleChange} className="form-checkbox cyber-input h-4 w-4" />
                    启用已购服务器资产自动刷新
                  </label>
                  <input type="number" min={3600} step={3600} name="serverInventoryRefreshIntervalSeconds" value={formValues.serverInventoryRefreshIntervalSeconds} onChange={handleChange} className="cyber-input w-full mt-3" placeholder="已购服务器资产刷新间隔（秒，最小3600）" />
                  <p className="text-xs text-cyber-muted mt-2">该任务会遍历所有 API 账户，周期性刷新服务器控制页面所需的已购服务器数据缓存。</p>
                </div>

                <div className="text-xs text-cyber-muted bg-cyber-grid/10 border border-cyber-accent/20 rounded-lg p-3">
                  最小刷新间隔为 3600 秒。服务器列表新增服务器将发送交互式通知，实时可用性新增服务器将发送文本通知；已购服务器资产刷新会更新服务器控制页面缓存。
                </div>
              </div>
            </div>
            
            {/* 缓存管理器 */}
            <div className="mt-6">
              <CacheManager />
            </div>

          </div>
        </form>
      )}

      {/* 错误历史模态框 */}
      {createPortal(
        <AnimatePresence>
          {showErrorHistoryDialog && webhookInfo?.last_error_date && (
            <div className="fixed inset-0 z-[9999] flex items-center justify-center p-4 pointer-events-none">
              <motion.div
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0 }}
                onClick={() => setShowErrorHistoryDialog(false)}
                className="absolute inset-0 bg-black/70 backdrop-blur-sm pointer-events-auto"
              />
              <motion.div
                initial={{ opacity: 0, scale: 0.9 }}
                animate={{ opacity: 1, scale: 1 }}
                exit={{ opacity: 0, scale: 0.9 }}
                onClick={(e) => e.stopPropagation()}
                className="cyber-card max-w-2xl w-full max-h-[90vh] overflow-y-auto pointer-events-auto relative"
              >
                <div className="flex items-center justify-between mb-4 pb-4 border-b border-cyber-accent/20">
                  <div className="flex items-center gap-2">
                    <AlertCircle className="w-5 h-5 text-yellow-400" />
                    <h3 className="text-xl font-semibold text-cyber-text">
                      {(() => {
                        const errorInfo = getErrorInfo();
                        return errorInfo?.isRecentError ? '最后错误' : '历史错误';
                      })()}
                    </h3>
                  </div>
                  <button
                    onClick={() => setShowErrorHistoryDialog(false)}
                    className="p-2 hover:bg-cyber-grid/50 rounded-lg transition-colors text-cyber-muted hover:text-cyber-text"
                  >
                    <X className="w-5 h-5" />
                  </button>
                </div>

                <div className="space-y-4">
                  {(() => {
                    const errorInfo = getErrorInfo();
                    if (!errorInfo) return null;
                    
                    const hasNoPendingUpdates = webhookInfo.pending_update_count === 0;
                    
                    return (
                      <div className={`border rounded-lg p-4 transition-all ${
                        errorInfo.isRecentError 
                          ? 'bg-red-500/10 border-red-500/30' 
                          : 'bg-yellow-500/10 border-yellow-500/30'
                      }`}>
                        <div className="flex items-start justify-between gap-2 mb-3">
                          <div className="flex items-center gap-1.5">
                            <span className={`text-sm font-semibold ${
                              errorInfo.isRecentError ? 'text-red-400' : 'text-yellow-400'
                            }`}>
                              {errorInfo.isRecentError ? '⚠️' : '📋'} {errorInfo.isRecentError ? '最后错误' : '历史错误'}
                            </span>
                          </div>
                          <span className={`text-xs px-2 py-1 rounded ${
                            errorInfo.isRecentError 
                              ? 'bg-red-500/20 text-red-300' 
                              : 'bg-yellow-500/20 text-yellow-300'
                          }`}>
                            {errorInfo.formatRelativeTime()}
                          </span>
                        </div>
                        
                        <div className={`text-sm font-mono mb-3 ${
                          errorInfo.isRecentError ? 'text-red-300/80' : 'text-yellow-300/80'
                        }`}>
                          {errorInfo.errorDate.toLocaleString('zh-CN', {
                            year: 'numeric',
                            month: '2-digit',
                            day: '2-digit',
                            hour: '2-digit',
                            minute: '2-digit',
                            second: '2-digit',
                            hour12: false
                          })}
                        </div>
                        
                        {webhookInfo.last_error_message && (
                          <div className={`text-sm leading-relaxed break-words p-3 rounded bg-black/20 mb-3 ${
                            errorInfo.isRecentError ? 'text-red-200' : 'text-yellow-200'
                          }`}>
                            {webhookInfo.last_error_message}
                          </div>
                        )}
                        
                        {!errorInfo.isRecentError && hasNoPendingUpdates && (
                          <div className="text-sm text-green-300/90 pt-3 border-t border-yellow-500/20 flex items-start gap-2">
                            <span className="text-base">💡</span>
                            <span>待处理更新为 0，Webhook 可能已恢复正常。如需清除此错误记录，请重新设置 Webhook。</span>
                          </div>
                        )}
                        
                        {errorInfo.isRecentError && (
                          <div className="text-sm text-red-300/80 pt-3 border-t border-red-500/20 flex items-start gap-2">
                            <span className="text-base">🔍</span>
                            <span>这是最近的错误，请检查 Webhook 配置和服务器状态。</span>
                          </div>
                        )}
                      </div>
                    );
                  })()}
                </div>
              </motion.div>
            </div>
          )}
        </AnimatePresence>,
        document.body
      )}
    </div>
  );
};

export default SettingsPage;
  
