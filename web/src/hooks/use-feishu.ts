/**
 * 飞书扫码注册:拿 app_id/app_secret 的设备码流程。
 *
 * 后端接口(都要求 X-API-Key,不在白名单里):
 *   POST /feishu/registration/start -> { sessionId, verificationUriComplete, expiresIn, interval }
 *   GET  /feishu/registration/:id   -> { state: pending|complete|denied|expired|error, ... }
 *
 * 前端只拿到二维码地址和状态,永远拿不到 device_code —— 那玩意等同于轮询凭据,
 * 泄漏出去别人可以替你把应用注册走。
 */
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { apiMessage } from "@/lib/api-error";
import { qk } from "@/lib/query";
import { toast } from "sonner";
import i18n from "@/i18n";

export interface FeishuRegistrationStart {
  sessionId: string;
  verificationUriComplete: string;
  /** 秒。兼容了飞书的 expires_in / expire_in 两个字段,后端已 clamp 到 600 */
  expiresIn: number;
  /** 秒。建议轮询间隔,后端已 clamp 到 5~60 */
  interval: number;
}

export type FeishuPollState = "pending" | "complete" | "denied" | "expired" | "error";

export interface FeishuPollResult {
  status: "success" | "error";
  state: FeishuPollState;
  /** pending 时建议的等待秒数(slow_down 会变大) */
  retryAfter?: number;
  appId?: string;
  appSecretConfigured?: boolean;
  domain?: string;
  bound?: boolean;
  /** 扫码完成后回填：不暴露给未登录方，接口本身要 API Key */
  boundOpenId?: string;
  message?: string;
}

/** 开始一次扫码会话。每次点「扫码创建」都重新生成二维码。 */
export function useStartFeishuRegistration() {
  return useMutation({
    mutationFn: async () =>
      (await api.post<FeishuRegistrationStart>("/feishu/registration/start")).data,
    onError: (e: any) =>
      toast.error(apiMessage(e) || i18n.t("hooksMsg.feishu.startFailed")),
  });
}

/**
 * 轮询扫码状态。
 *
 * 用 refetchInterval 而不是 setInterval:间隔要跟着后端返回的 retryAfter 走
 * (飞书 slow_down 会把间隔加 5 秒),终态立刻停,组件卸载时 enabled 直接断开。
 */
export function useFeishuRegistrationPoll(sessionId: string | null) {
  return useQuery({
    queryKey: ["feishu", "registration", sessionId],
    enabled: !!sessionId,
    queryFn: async () =>
      (await api.get<FeishuPollResult>(`/feishu/registration/${sessionId}`)).data,
    refetchInterval: (query) => {
      const data = query.state.data;
      if (!data || data.state !== "pending") return false;
      // 2 秒是下限:后端就算让我们 1 秒轮询,也别把免费注册接口打成压测。
      return Math.max(2, data.retryAfter ?? 5) * 1000;
    },
    retry: false,
    gcTime: 0,
  });
}

export interface FeishuBindingCode {
  code: string;
  /** RFC3339,10 分钟后过期 */
  expiresAt: string;
}

/**
 * 生成手动绑定用的 6 位码。
 *
 * 只有能登录控制台(拿得到 API Key)的人才能生成;飞书那边必须用同一个
 * open_id 私聊机器人发 /bind <码> 才算绑定 —— 这就是"机器人被拉进别人私聊"
 * 时不会被抢先绑定成操作者的原因。
 */
export function useCreateFeishuBindingCode() {
  return useMutation({
    mutationFn: async () =>
      (await api.post<FeishuBindingCode>("/feishu/binding-code")).data,
    onError: (e: any) =>
      toast.error(apiMessage(e) || i18n.t("hooksMsg.feishu.bindCodeFailed")),
  });
}

/**
 * 解除接收人绑定。
 *
 * 解绑不跟着「保存设置」走：那样每次无关的保存都可能把绑定清掉。
 */
export function useUnbindFeishu() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.post("/feishu/unbind")).data,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.settings.config() });
      toast.success(i18n.t("settings.feishu.unbindToast"));
    },
    onError: (e: any) => toast.error(apiMessage(e) || i18n.t("hooksMsg.feishu.unbindFailed")),
  });
}
