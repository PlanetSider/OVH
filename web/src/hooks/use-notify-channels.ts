import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { toast } from "sonner";
import i18n from "@/i18n";
import { apiMessage } from "@/lib/api-error";

export interface NotifyChannel {
  name: string;
  configured: boolean;
  ok: boolean;
  detail?: string;
}

export interface NotifyChannelsResult {
  channels: NotifyChannel[];
  anyAvailable: boolean;
  verified: boolean;
}

/**
 * 通知通道体检。
 *
 * 为什么不再直接用 useTelegramVerify 做订阅门禁：订阅现在只要求「至少有一条通道能用」。
 * 只看 Telegram 的话，一个只配了 webhook 的用户会被前端拦住，
 * 而后端其实是放行的——用户会看到一个自己无论如何都消不掉的报错。
 *
 * 默认不 verify：verify=true 时后端会**真的往每个通道发一条测试消息**。
 * 打开设置页/监控页就发消息是不可接受的副作用 —— 想验证必须用户主动点「重新检测」，
 * 走 useVerifyNotifyChannels()。
 */
export function useNotifyChannels(verify = false) {
  return useQuery<NotifyChannelsResult>({
    queryKey: ["notify", "channels", verify],
    queryFn: async () =>
      (await api.get<NotifyChannelsResult>(`/notify/channels?verify=${verify}`)).data,
    staleTime: 5 * 60 * 1000,
    gcTime: 30 * 60 * 1000,
    retry: 0,
    refetchOnWindowFocus: false,
  });
}

/** 订阅门禁：返回 [是否拦截, 原因, 是否还在查] */
export function useNotifyGate(): [boolean, string, boolean] {
  // 门禁只看"配没配"，绝不触发远端体检（否则每次打开订阅弹窗都给用户发一条测试消息）。
  const q = useNotifyChannels(false);
  if (q.isPending) return [false, "", true];
  // 形状不对(缺 channels / anyAvailable)按"无法判断"处理,不拦截 ——
  // 这只是前端预检,后端在提交时还有真正的强校验;宁可放行到后端,
  // 也不要因为一次响应抖动把订阅入口整个锁死,更不能在这里崩掉整页
  // (这个 hook 被 AddSubscriptionDialog 常驻调用,崩 = 监控页全白)
  if (!q.data || !Array.isArray(q.data.channels) || typeof q.data.anyAvailable !== "boolean") {
    return [false, "", false];
  }
  if (q.data.anyAvailable) return [false, "", false];
  const detail = q.data.channels
    .filter((c) => c.configured && !c.ok)
    .map((c) => i18n.t("hooksMsg.notify.channelLine", { name: c.name, detail: c.detail || i18n.t("hooksMsg.notify.channelUnavailable") }))
    .join(i18n.t("hooksMsg.notify.channelSeparator"));
  return [true, detail || i18n.t("hooksMsg.notify.noChannelConfigured"), false];
}

/**
 * 主动重新检测：后端会真的往每个已配置通道发一条"连通性测试"。
 * 与页面自动加载区分开——只有用户点「重新检测」才走这里，一次点击一次发送。
 */
export function useVerifyNotifyChannels() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () =>
      (await api.get<NotifyChannelsResult>("/notify/channels?verify=true")).data,
    onSuccess: (d) => {
      // 把体检结果写回页面正在读的 key，避免再发一次请求。
      qc.setQueryData<NotifyChannelsResult>(["notify", "channels", false], {
        channels: d.channels,
        anyAvailable: d.anyAvailable,
        verified: true,
      });
      const failed = (d.channels || []).filter((c) => c.configured && !c.ok);
      if (!d.anyAvailable) {
        toast.error(
          failed.length
            ? failed
                .map((c) => `${c.name}: ${c.detail || i18n.t("hooksMsg.notify.channelUnavailable")}`)
                .join(i18n.t("hooksMsg.notify.channelSeparator"))
            : i18n.t("hooksMsg.notify.noChannelConfigured")
        );
      } else if (failed.length) {
        toast.warning(
          i18n.t("hooksMsg.notify.verifyPartial", {
            names: failed.map((c) => c.name).join(i18n.t("hooksMsg.notify.nameSeparator")),
          })
        );
      } else {
        toast.success(i18n.t("hooksMsg.notify.verifyOk"));
      }
    },
    onError: (e: any) => toast.error(apiMessage(e) || i18n.t("hooksMsg.notify.detectFailed")),
  });
}

/**
 * 发一条测试通知到所有已配置的通道。
 *
 * 后端返回每条通道各自的结果，所以这里不做"成功/失败"的二元判断 ——
 * 两条通道配了、只有一条通得了，那既不是成功也不是失败，用户需要知道是哪条挂了。
 */
export function useTestNotification() {
  const qc = useQueryClient();
  const saveResult = (channels?: NotifyChannel[]) => {
    if (!Array.isArray(channels)) return;
    qc.setQueryData<NotifyChannelsResult>(["notify", "channels", false], {
      channels, anyAvailable: channels.some((c) => c.configured && c.ok), verified: true,
    });
  };
  return useMutation({
    mutationFn: async () =>
      (
        await api.post<{ delivered: number; message: string; channels: NotifyChannel[] }>(
          "/monitor/test-notification"
        )
      ).data,
    onSuccess: (d) => {
      // 使用本次发送结果,不重新体检,否则 Webhook 又会收到一条测试通知。
      saveResult(d.channels);
      const failed = (d.channels || []).filter((c) => c.configured && !c.ok);
      if (d.delivered === 0) {
        toast.error(d.message || i18n.t("hooksMsg.notify.noneDelivered"));
      } else if (failed.length) {
        toast.warning(
          i18n.t("hooksMsg.notify.partialDelivered", {
            delivered: d.delivered,
            names: failed.map((c) => c.name).join(i18n.t("hooksMsg.notify.nameSeparator")),
            reasons: failed
              .map((c) => c.detail || i18n.t("hooksMsg.notify.unknownReason"))
              .join(i18n.t("hooksMsg.notify.channelSeparator")),
          })
        );
      } else {
        toast.success(d.message || i18n.t("hooksMsg.notify.sentToChannels", { count: d.delivered }));
      }
    },
    onError: (e: any) => {
      const channels: NotifyChannel[] | undefined = e?.response?.data?.channels;
      saveResult(channels);
      const reason = Array.isArray(channels)
        ? channels.filter((c) => c.configured && !c.ok).map((c) => c.detail).filter(Boolean).join(i18n.t("hooksMsg.notify.channelSeparator"))
        : "";
      toast.error(reason || apiMessage(e) || i18n.t("hooksMsg.notify.sendFailed"));
    },
  });
}
