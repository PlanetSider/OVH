import i18n from "@/i18n";
/**
 * 金额显示的唯一入口。
 *
 * 为什么不能兜底成 EUR：币种是按 **子公司** 定的，不是按站点定的。
 * 实测 /v1/order/catalog/public/eco?ovhSubsidiary=X 的 locale.currencyCode：
 *   IE/FR/DE/NL/PT/FI/CZ/LT = EUR   GB = GBP   PL = PLN   MA = MAD   TN = TND   SN = XOF
 *   CA/QC = CAD   US/WE/WS = USD   SG = SGD   AU = AUD   IN = INR
 * 后端这一轮已经把"拿不到 currencyCode 就留空"落实了（internal/price/price.go、
 * internal/purchase/purchase.go 都明确写了"不再默认 EUR"），前端若再 `|| "EUR"`
 * 等于把后端的诚实留空重新伪造成欧元——美区/加区的订单会被显示成 €。
 * 所以：币种缺失时只显示数字，符号和币种码一个都不编。
 */

/** 币种符号表。表里没有的币种不猜符号，改成"数字 + 币种码"后缀显示。 */
const CURRENCY_SYMBOLS: Record<string, string> = {
  EUR: "€",
  USD: "$",
  GBP: "£",
  CAD: "CA$",
  SGD: "S$",
  AUD: "A$",
  INR: "₹",
};

/** 币种码 → 符号；未知或空一律返回空串（绝不回落成 €） */
export function currencySymbol(code?: string | null): string {
  const c = (code || "").trim().toUpperCase();
  if (!c) return "";
  return CURRENCY_SYMBOLS[c] || "";
}

/**
 * 金额 + 币种的统一渲染：
 *   有符号   → `€42.99`
 *   无符号   → `42.99 PLN`
 *   没币种   → `42.99`（不带任何货币标记，配合 CURRENCY_UNKNOWN_HINT 说明原因）
 */
export function formatMoney(value: number, code?: string | null, digits = 2): string {
  const n = value.toFixed(digits);
  const c = (code || "").trim().toUpperCase();
  if (!c) return n;
  const sym = CURRENCY_SYMBOLS[c];
  return sym ? `${sym}${n}` : `${n} ${c}`;
}

/** 币种缺失时给 title / 说明用的文案，别让用户以为是页面少渲染了一截 */
export const currencyUnknownHint = () => i18n.t("commons.currencyUnknownHint");

/** 升降序均将未知报价排最后;零元是有效报价。 */
export function comparePrices(a: number | undefined, b: number | undefined, direction: "asc" | "desc"): number {
  const aKnown = a !== undefined && Number.isFinite(a);
  const bKnown = b !== undefined && Number.isFinite(b);
  if (!aKnown || !bKnown) return aKnown ? -1 : bKnown ? 1 : 0;
  return direction === "asc" ? a! - b! : b! - a!;
}

/**
 * 子公司 → 计价币种。
 *
 * 与后端 `server/internal/monitor/price.go` 的 subsidiaryCurrency 同源：
 * 24 个子公司逐个用 /v1/order/catalog/public/eco?ovhSubsidiary=X 的
 * locale.currencyCode 实测得到。**两边必须一起改**——后端用它补通知文案，
 * 前端用它给"手上没有 catalog 的页面"（合同弹窗、历史记录）兜底。
 * 漂移的表现是"同一个机房在列表里是 USD、在弹窗里成了 €"，不报错，
 * 只会让人不敢下单。
 */
const SUBSIDIARY_CURRENCY: Record<string, string> = {
  // EU 站点
  CZ: "EUR", DE: "EUR", ES: "EUR", EU: "EUR", FI: "EUR", FR: "EUR",
  IE: "EUR", IT: "EUR", LT: "EUR", NL: "EUR", PT: "EUR",
  GB: "GBP", PL: "PLN", MA: "MAD", TN: "TND", SN: "XOF",
  // CA 站点
  CA: "CAD", QC: "CAD", ASIA: "USD", WE: "USD", WS: "USD",
  AU: "AUD", IN: "INR", SG: "SGD",
  // US 站点
  US: "USD",
};

/** 子公司 → 币种；未知子公司返回空串（宁可不显示，也不猜） */
export function currencyForSubsidiary(subsidiary?: string | null): string {
  const s = (subsidiary || "").trim().toUpperCase();
  return s ? SUBSIDIARY_CURRENCY[s] || "" : "";
}

/**
 * 人民币金额：取整 + 千分位。
 * 汇率本身是估算，再显示两位小数就是假精度（同一天两次刷新出现 ¥333.21 和
 * ¥333.87，用户会以为价格在变）。
 */
export function formatCNY(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "";
  return "¥" + Math.round(value).toString().replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

/** 人民币参考价的显示参数 */
export interface CnyOptions {
  /** 1 单位该币种 = N 人民币；缺失/<=0 时不显示人民币 */
  cnyRate?: number | null;
  /** 币种是推断出来的（按子公司查表），需要在末尾注明"按区域估算" */
  estimated?: boolean;
  /** 金额保留几位小数，默认 2 */
  digits?: number;
  /** 追加在原币金额后的周期单位，如 "/月"；人民币部分沿用同一个单位 */
  unit?: string;
}

/**
 * 原币金额 + 可选的人民币参考价，例如 `€42.99 · ≈¥307`。
 *
 * 失败姿态只有一个：**只显示原币**。汇率缺失、币种为空、金额非正数时，
 * 绝不返回 `≈¥--` / `≈¥0` 这样的占位——那看起来像一个真实报价，比不显示危险。
 */
export function formatMoneyWithCny(
  value: number,
  code: string | null | undefined,
  opts: CnyOptions = {},
): string {
  const base = formatMoney(value, code, opts.digits ?? 2);
  const rate = opts.cnyRate;
  const c = (code || "").trim().toUpperCase();
  if (!c || !rate || !Number.isFinite(rate) || rate <= 0) return base;
  const cny = formatCNY(value * rate);
  if (!cny) return base;
  const note = opts.estimated ? i18n.t("commons.cnyEstimated") : "";
  return `${base}${opts.unit ?? ""} · ≈${cny}${opts.unit ?? ""}${note}`;
}
