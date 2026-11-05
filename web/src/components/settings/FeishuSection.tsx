/**
 * 设置页「飞书机器人」一节。
 *
 * 两条建应用的路:
 *   1) 扫码创建(推荐):走飞书 PersonalAgent 设备码注册,一次扫码拿到
 *      app_id/app_secret 并顺手绑定扫码者 open_id;
 *   2) 手动填写:注册通道不可用时兜底,自己到开放平台建应用再填。
 *
 * 扫码 ≠ 能用:应用还要在开放平台开事件订阅(长连接)并创建版本发布,
 * 否则机器人收不到任何消息 —— 这段说明必须留在界面上,不能只在文档里。
 */
import { useEffect, useState, type ReactNode } from "react";
import { QRCodeSVG } from "qrcode.react";
import { AlertTriangle, CheckCircle2, KeyRound, Link2, MessageSquare, QrCode, RefreshCw, Unlink } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Section } from "@/routes/settings-sections";
import { LoadFailedBanner } from "@/components/common/LoadFailed";
import { Skeleton } from "@/components/common/Skeleton";
import { Chip } from "@/components/common/Chip";
import { fmtDateTime } from "@/i18n/format";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  useCreateFeishuBindingCode,
  useFeishuRegistrationPoll,
  useFeishuStatus,
  useStartFeishuRegistration,
  useUnbindFeishu,
} from "@/hooks/use-feishu";
import { qk } from "@/lib/query";
import type { SettingsConfig } from "@/hooks/use-settings";
import { cn } from "@/lib/utils";

/** 剩余时间:过期后直接显示"已过期",绝不显示负数 */
function useCountdown(expiresAt?: number): { expired: boolean; label: string } {
  const { t } = useTranslation();
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    if (!expiresAt) return;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [expiresAt]);
  if (!expiresAt) return { expired: false, label: "" };
  const left = Math.floor((expiresAt - now) / 1000);
  if (left <= 0) return { expired: true, label: t("settings.feishu.qrExpired") };
  return {
    expired: false,
    label: t("settings.feishu.qrCountdown", {
      min: Math.floor(left / 60),
      sec: String(left % 60).padStart(2, "0"),
    }),
  };
}

export function FeishuSection({
  form,
  set,
}: {
  form: SettingsConfig;
  set: <K extends keyof SettingsConfig>(k: K, v: SettingsConfig[K]) => void;
}) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const start = useStartFeishuRegistration();
  const bindCode = useCreateFeishuBindingCode();
  const unbind = useUnbindFeishu();
  const [session, setSession] = useState<{ sessionId: string; url: string; expiresAt: number } | null>(
    null
  );
  const poll = useFeishuRegistrationPoll(session?.sessionId ?? null);
  const { expired, label: countdown } = useCountdown(session?.expiresAt);

  const bound = (form.feishuBoundOpenId || "").trim();
  const enabled = form.feishuEnabled === true;

  // 长连接状态:用后端真实连接态,不要用表单里的 enabled 猜 —— 表单改了还没保存时
  // 连接还是旧的,那正是用户最容易误判"我已经开了怎么没反应"的时刻。
  const status = useFeishuStatus();
  const bot = status.data;
  const connection = !bot?.enabled
    ? "disabled"
    : !bot.configured
      ? "unconfigured"
      : bot.connected
        ? "connected"
        : bot.started
          ? "connecting"
          : "stopped";

  // 扫码完成的三种终态:成功回填表单,取消/过期收掉二维码并提示。
  // error 不在这里处理 —— 后端保留会话可重试,二维码继续显示。
  useEffect(() => {
    const data = poll.data;
    if (!data) return;
    if (data.state === "complete") {
      set("feishuAppId", data.appId || "");
      set("feishuDomain", data.domain === "lark" ? "lark" : "feishu");
      set("feishuEnabled", true);
      // 服务端已经绑定了扫码者。必须回填，否则用户马上点「保存设置」时
      // 表单里的空值会被当作「要解绑」——虽然后端有保留兜底，这里也不该依赖它。
      if (data.boundOpenId) set("feishuBoundOpenId", data.boundOpenId);
      setSession(null);
      void qc.invalidateQueries({ queryKey: qk.settings.config() });
      toast.success(t("settings.feishu.createdToast"));
    } else if (data.state === "denied" || data.state === "expired") {
      setSession(null);
      toast.error(
        data.state === "denied" ? t("settings.feishu.deniedToast") : t("settings.feishu.expiredToast")
      );
    }
    // set 是稳定的 setState 包装,不需要进依赖
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [poll.data]);

  const startRegistration = () => {
    start.mutate(undefined, {
      onSuccess: (data) => {
        setSession({
          sessionId: data.sessionId,
          url: data.verificationUriComplete,
          expiresAt: Date.now() + Math.max(30, data.expiresIn) * 1000,
        });
      },
    });
  };

  return (
    <Section title={t("settings.feishu.title")}>
      <p className="text-[11px] text-muted-foreground leading-relaxed">
        {t("settings.feishu.introPre")}
        <b>{t("settings.feishu.introBold")}</b>
        {t("settings.feishu.introPost")}
      </p>

      {/* 长连接状态:飞书是服务端主动连出去的,断了没有入站请求可依靠 ——
          这里不显示,用户就完全没有反馈 */}
      <div className="rounded-2xl border border-border p-4 space-y-2.5 text-[13px]">
        <div className="flex items-center justify-between gap-3">
          <h3 className="text-[13px] font-medium flex items-center gap-1.5">
            <MessageSquare className="w-3.5 h-3.5 text-muted-foreground" />
            {t("settings.feishu.statusTitle")}
          </h3>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => status.refetch()}
            disabled={status.isFetching}
          >
            <RefreshCw className={cn("w-3.5 h-3.5", status.isFetching && "animate-spin")} />
            {t("common.refresh")}
          </Button>
        </div>
        <p className="text-[11px] text-muted-foreground leading-relaxed">
          {t("settings.feishu.statusDesc")}
        </p>

        {status.isPending ? (
          <Skeleton className="h-16 rounded-xl" />
        ) : status.isError ? (
          <LoadFailedBanner
            title={t("settings.feishu.statusFailed")}
            error={status.error}
            onRetry={() => status.refetch()}
          />
        ) : (
          <>
            <InfoRow
              label={t("settings.feishu.connectionLabel")}
              value={
                connection === "connected" ? (
                  <Chip tone="success">
                    <CheckCircle2 className="w-3 h-3" />
                    {t("settings.feishu.statusConnected")}
                  </Chip>
                ) : connection === "connecting" ? (
                  <Chip tone="warning">
                    <RefreshCw className="w-3 h-3 animate-spin" />
                    {t("settings.feishu.statusConnecting")}
                  </Chip>
                ) : connection === "stopped" ? (
                  <Chip tone="danger">
                    <AlertTriangle className="w-3 h-3" />
                    {t("settings.feishu.statusStopped")}
                  </Chip>
                ) : connection === "unconfigured" ? (
                  <Chip>{t("settings.feishu.statusUnconfigured")}</Chip>
                ) : (
                  <Chip>{t("settings.feishu.statusDisabled")}</Chip>
                )
              }
            />
            <InfoRow
              label={t("settings.feishu.lastConnected")}
              value={
                bot?.connectedAt ? (
                  <span className="font-mono text-[12px]">{fmtDateTime(bot.connectedAt)}</span>
                ) : (
                  <span className="text-muted-foreground">
                    {t("settings.feishu.neverConnected")}
                  </span>
                )
              }
            />
            <InfoRow
              label={t("settings.feishu.lastEvent")}
              value={
                bot?.lastEventAt ? (
                  <span className="font-mono text-[12px]">{fmtDateTime(bot.lastEventAt)}</span>
                ) : (
                  <span className="text-muted-foreground">{t("settings.feishu.neverEvent")}</span>
                )
              }
            />
            <InfoRow
              label={t("settings.feishu.appIdLabel")}
              value={<span className="font-mono text-[12px]">{bot?.appId || "—"}</span>}
            />
            <InfoRow
              label={t("settings.feishu.domainLabel")}
              value={
                bot?.domain === "lark"
                  ? t("settings.feishu.domainLark")
                  : t("settings.feishu.domainFeishu")
              }
            />
            {bot?.lastError ? (
              <div className="rounded-xl border border-destructive/40 bg-destructive/5 px-3 py-2 text-[12px] flex items-start gap-2">
                <AlertTriangle className="w-4 h-4 text-destructive flex-shrink-0 mt-0.5" />
                <div className="min-w-0">
                  <p className="font-semibold text-destructive">
                    {t("settings.feishu.lastErrorTitle")}
                  </p>
                  <p className="mt-0.5 break-words">{bot.lastError}</p>
                </div>
              </div>
            ) : null}
            {connection === "stopped" && (
              <p className="text-[11px] text-destructive">
                {t("settings.feishu.notConnectedWarning")}
              </p>
            )}
          </>
        )}
      </div>

      {/* 扫码创建 */}
      <div className="rounded-2xl border border-border p-4 space-y-3">
        <div className="flex items-center justify-between gap-3">
          <h3 className="text-[13px] font-medium flex items-center gap-1.5">
            <QrCode className="w-3.5 h-3.5 text-muted-foreground" />
            {t("settings.feishu.scanTitle")}
          </h3>
          <label className="flex items-center gap-2 text-[11px] text-muted-foreground cursor-pointer">
            {t("settings.feishu.enabled")}
            <Switch checked={enabled} onCheckedChange={(v) => set("feishuEnabled", v)} />
          </label>
        </div>

        {session && !expired ? (
          <div className="flex gap-4 items-start">
            <div className="p-2.5 bg-white rounded-xl border border-border flex-shrink-0">
              <QRCodeSVG value={session.url} size={120} />
            </div>
            <div className="min-w-0 flex-1 space-y-2">
              <p className="text-[12px] text-muted-foreground leading-relaxed">
                {t("settings.feishu.scanHint")}
              </p>
              <p className="text-[11px] text-muted-foreground flex items-center gap-1">
                <RefreshCw className={cn("w-3 h-3", poll.isFetching && "animate-spin")} />
                {countdown}
              </p>
              {poll.data?.state === "error" && poll.data.message && (
                <p className="text-[11px] text-destructive">{poll.data.message}</p>
              )}
              <Button size="sm" variant="outline" onClick={startRegistration} disabled={start.isPending}>
                {t("settings.feishu.regenerate")}
              </Button>
            </div>
          </div>
        ) : (
          <div className="space-y-2">
            <Button size="sm" onClick={startRegistration} disabled={start.isPending}>
              <QrCode className="w-3.5 h-3.5" />
              {start.isPending ? t("settings.feishu.starting") : t("settings.feishu.startScan")}
            </Button>
            {expired && (
              <p className="text-[11px] text-muted-foreground">{t("settings.feishu.qrExpiredHint")}</p>
            )}
          </div>
        )}
      </div>

      {/* 手动填写兜底 */}
      <div className="rounded-2xl border border-border p-4 space-y-3">
        <h3 className="text-[13px] font-medium flex items-center gap-1.5">
          <MessageSquare className="w-3.5 h-3.5 text-muted-foreground" />
          {t("settings.feishu.manualTitle")}
        </h3>
        <p className="text-[11px] text-muted-foreground leading-relaxed">
          {t("settings.feishu.manualDesc")}
        </p>
        <label className="block space-y-1">
          <span className="text-[12px] font-medium">{t("settings.feishu.appIdLabel")}</span>
          <Input
            value={form.feishuAppId || ""}
            onChange={(e) => set("feishuAppId", e.target.value)}
            placeholder="cli_xxx"
            autoComplete="off"
          />
          <span className="text-[10.5px] text-muted-foreground">{t("settings.feishu.appIdHint")}</span>
        </label>
        <label className="block space-y-1">
          <span className="text-[12px] font-medium">{t("settings.feishu.appSecretLabel")}</span>
          <Input
            type="password"
            value={form.feishuAppSecret || ""}
            onChange={(e) => set("feishuAppSecret", e.target.value)}
            placeholder={
              form.feishuAppId && !form.feishuAppSecret
                ? t("settings.feishu.appSecretSaved")
                : t("settings.feishu.appSecretPlaceholder")
            }
            autoComplete="new-password"
          />
          <span className="text-[10.5px] text-muted-foreground">{t("settings.feishu.appSecretHint")}</span>
        </label>
        <label className="block space-y-1">
          <span className="text-[12px] font-medium">{t("settings.feishu.domainLabel")}</span>
          <Select
            value={form.feishuDomain === "lark" ? "lark" : "feishu"}
            onValueChange={(v) => set("feishuDomain", v)}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="feishu">{t("settings.feishu.domainFeishu")}</SelectItem>
              <SelectItem value="lark">{t("settings.feishu.domainLark")}</SelectItem>
            </SelectContent>
          </Select>
        </label>
      </div>

      {/* 接收人绑定 */}
      <div className="rounded-2xl border border-border p-4 space-y-2">
        <h3 className="text-[13px] font-medium flex items-center gap-1.5">
          <Link2 className="w-3.5 h-3.5 text-muted-foreground" />
          {t("settings.feishu.bindingTitle")}
        </h3>
        <div className="flex items-center justify-between gap-3">
          <p className="text-[12px] text-muted-foreground break-all">
            {bound
              ? t("settings.feishu.bindingBound", { openId: bound.slice(0, 12) })
              : t("settings.feishu.bindingNone")}
          </p>
          {bound && (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={unbind.isPending}
              className="flex-shrink-0 text-destructive border-destructive/40 hover:bg-destructive/10"
              onClick={() =>
                unbind.mutate(undefined, { onSuccess: () => set("feishuBoundOpenId", "") })
              }
            >
              <Unlink className="w-3.5 h-3.5" />
              {t("settings.feishu.unbind")}
            </Button>
          )}
        </div>
        {/* 扫码路径会自动绑定；手动填写的应用靠这个 6 位码 */}
        {!bound && (
          <div className="flex items-center gap-2 flex-wrap">
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={bindCode.isPending}
              onClick={() => bindCode.mutate()}
            >
              <KeyRound className="w-3.5 h-3.5" />
              {bindCode.isPending
                ? t("settings.feishu.bindingCodeCreating")
                : t("settings.feishu.bindingCodeCreate")}
            </Button>
            {bindCode.data && (
              <span className="font-mono text-[13px] tracking-[0.2em] rounded-md border border-border px-2 py-1">
                {bindCode.data.code}
              </span>
            )}
          </div>
        )}
        {bindCode.data && !bound && (
          <p className="text-[10.5px] text-muted-foreground">
            {t("settings.feishu.bindingCodeHowTo")}
          </p>
        )}
        <p className="text-[10.5px] text-muted-foreground leading-relaxed">
          {bound ? t("settings.feishu.bindingHint") : t("settings.feishu.bindingCodeHint")}
        </p>
      </div>

      {/* 发布提醒:扫码拿到的只是凭据,事件订阅不配好等于哑巴 */}
      <div className="rounded-xl border border-warning/40 bg-warning/5 p-3 flex gap-2">
        <AlertTriangle className="w-3.5 h-3.5 text-warning flex-shrink-0 mt-0.5" />
        <p className="text-[11px] text-muted-foreground leading-relaxed">
          {t("settings.feishu.publishHint")}
        </p>
      </div>
    </Section>
  );
}

/** 状态卡片的一行:左灰标签、右对齐值。settings.tsx 里那份是私有的,不能直接复用 */
function InfoRow({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex justify-between items-start gap-3">
      <span className="text-muted-foreground flex-shrink-0">{label}</span>
      <span className="font-medium text-right min-w-0">{value}</span>
    </div>
  );
}
