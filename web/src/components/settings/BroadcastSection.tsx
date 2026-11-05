/**
 * 设置页「定时播报」一节。
 *
 * 每天固定时间把抢购队列、独服监控、VPS 监控的当前任务清单发到通知通道，
 * 另外可选带一份 24 小时抢购战报。
 *
 * 两个容易踩的点，界面上必须写清楚:
 *   1) 时间固定按北京时间解释，不跟容器的 TZ 走 —— 部署到美国机器上时，
 *      用户以为设的是当地时间，实际是北京时间；
 *   2) 停机期间错过的那一次不补发。早上九点的任务清单，下午补发只会误导。
 */
import { useState, type ReactNode } from "react";
import { CalendarClock, Eye, Play } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useMutation } from "@tanstack/react-query";
import { toast } from "sonner";

import { Section } from "@/routes/settings-sections";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { api } from "@/lib/api";
import { apiMessage } from "@/lib/api-error";
import type { SettingsConfig } from "@/hooks/use-settings";

interface BroadcastSectionProps {
  form: SettingsConfig;
  set: <K extends keyof SettingsConfig>(key: K, value: SettingsConfig[K]) => void;
}

export function BroadcastSection({ form, set }: BroadcastSectionProps) {
  const { t } = useTranslation();
  const [preview, setPreview] = useState<string[] | null>(null);

  const previewMutation = useMutation({
    mutationFn: async () => {
      const { data } = await api.post("/broadcast/preview", {});
      return (data?.messages ?? []) as string[];
    },
    onSuccess: (messages) => {
      setPreview(messages);
      if (messages.length === 0) {
        toast.info(t("settings.broadcast.previewEmpty"));
      }
    },
    onError: (err) => toast.error(apiMessage(err)),
  });

  const runMutation = useMutation({
    mutationFn: async () => {
      const { data } = await api.post("/broadcast/run", {});
      return (data?.sent ?? 0) as number;
    },
    onSuccess: (sent) => toast.success(t("settings.broadcast.runDone", { count: sent })),
    onError: (err) => toast.error(apiMessage(err)),
  });

  // 分类开关后端按「缺省即开」处理，界面也按开显示。
  const enabled = form.taskBroadcastEnabled === true;
  const startupEnabled = form.taskBroadcastOnStartup === true;
  const categories: Array<{ key: "queue" | "monitor" | "vps" | "report"; label: string }> = [
    { key: "queue", label: t("settings.broadcast.queueLabel") },
    { key: "monitor", label: t("settings.broadcast.monitorLabel") },
    { key: "vps", label: t("settings.broadcast.vpsLabel") },
    { key: "report", label: t("settings.broadcast.reportLabel") },
  ];
  const categoryValue = (key: (typeof categories)[number]["key"]): boolean => {
    switch (key) {
      case "queue":
        return form.taskBroadcastQueueEnabled ?? true;
      case "monitor":
        return form.taskBroadcastMonitorEnabled ?? true;
      case "vps":
        return form.taskBroadcastVpsEnabled ?? true;
      default:
        return form.taskBroadcastReportEnabled ?? true;
    }
  };
  const setCategory = (key: (typeof categories)[number]["key"], value: boolean) => {
    switch (key) {
      case "queue":
        set("taskBroadcastQueueEnabled", value);
        return;
      case "monitor":
        set("taskBroadcastMonitorEnabled", value);
        return;
      case "vps":
        set("taskBroadcastVpsEnabled", value);
        return;
      default:
        set("taskBroadcastReportEnabled", value);
    }
  };

  return (
    <Section title={t("settings.broadcast.title")}>
      <p className="text-sm text-muted-foreground">{t("settings.broadcast.intro")}</p>

      <Row label={t("settings.broadcast.enabledLabel")} hint={t("settings.broadcast.enabledHint")}>
        <Switch checked={enabled} onCheckedChange={(v) => set("taskBroadcastEnabled", v)} />
      </Row>

      <Row label={t("settings.broadcast.timeLabel")} hint={t("settings.broadcast.timeHint")}>
        <Input
          type="time"
          className="w-40"
          value={form.taskBroadcastTime || "09:00"}
          onChange={(e) => set("taskBroadcastTime", e.target.value)}
        />
      </Row>

      <div className="space-y-2">
        <div className="text-sm font-medium">{t("settings.broadcast.categoriesLabel")}</div>
        {categories.map((category) => (
          <div key={category.key} className="flex items-center justify-between gap-4 rounded-md border px-3 py-2">
            <span className="text-sm">{category.label}</span>
            <Switch checked={categoryValue(category.key)} onCheckedChange={(v) => setCategory(category.key, v)} />
          </div>
        ))}
      </div>

      <Row label={t("settings.broadcast.startupLabel")} hint={t("settings.broadcast.startupHint")}>
        <Switch checked={startupEnabled} onCheckedChange={(v) => set("taskBroadcastOnStartup", v)} />
      </Row>

      <div className="flex flex-wrap items-center gap-2">
        <Button variant="outline" onClick={() => previewMutation.mutate()} disabled={previewMutation.isPending}>
          <Eye className="mr-2 h-4 w-4" />
          {previewMutation.isPending ? t("settings.broadcast.previewing") : t("settings.broadcast.preview")}
        </Button>
        <Button onClick={() => runMutation.mutate()} disabled={runMutation.isPending}>
          <Play className="mr-2 h-4 w-4" />
          {runMutation.isPending ? t("settings.broadcast.running") : t("settings.broadcast.run")}
        </Button>
      </div>

      {preview && preview.length > 0 && (
        <div className="space-y-2">
          <div className="text-sm font-medium">{t("settings.broadcast.previewTitle", { count: preview.length })}</div>
          <pre className="max-h-80 overflow-auto whitespace-pre-wrap rounded-md border bg-muted/40 p-3 text-xs leading-relaxed">
            {preview.join("\n\n---\n\n")}
          </pre>
        </div>
      )}

      <p className="flex items-start gap-2 text-xs text-muted-foreground">
        <CalendarClock className="mt-0.5 h-3.5 w-3.5 shrink-0" />
        {t("settings.broadcast.footnote")}
      </p>
    </Section>
  );
}

function Row({ label, hint, children }: { label: string; hint: string; children: ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4 border-b pb-4 last:border-0">
      <div className="space-y-1">
        <div className="text-sm font-medium">{label}</div>
        <p className="text-xs text-muted-foreground">{hint}</p>
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  );
}
