import { useCallback } from "react";
import { useSettings } from "@/hooks/use-settings";
import { useExchangeRates } from "@/hooks/use-exchange-rates";
import { formatMoneyWithCny, type CnyOptions } from "@/lib/money";

/**
 * 页面里的金额格式化：原币 + 可选的人民币参考价。
 *
 * 用它的页面不需要关心"开关关了没 / 汇率到了没 / 这个币种有没有汇率"——
 * 三种情况都退化成只显示原币，不会出现占位符。
 */
export function useMoneyFormat() {
  const settings = useSettings();
  const rates = useExchangeRates();
  const enabled =
    (settings.data?.displayMode ?? "both") !== "off" && rates.data?.active === true;

  /** 取某个币种的汇率（1 单位外币 = N 人民币）；不可用时返回 undefined */
  const rateFor = useCallback(
    (code?: string | null) => {
      const c = (code || "").trim().toUpperCase();
      return enabled && c ? rates.data?.rates?.[c] : undefined;
    },
    [enabled, rates.data],
  );

  const formatPrice = useCallback(
    (value: number, code?: string | null, opts: CnyOptions = {}) => {
      const c = (code || "").trim().toUpperCase();
      const rate = enabled && c ? rates.data?.rates?.[c] : undefined;
      return formatMoneyWithCny(value, code, { ...opts, cnyRate: rate });
    },
    [enabled, rates.data],
  );

  return { formatPrice, rateFor, cnyEnabled: enabled };
}
