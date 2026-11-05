import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { useSettings } from "@/hooks/use-settings";

/** 后端 GET /api/exchange/rates 的返回体 */
export interface ExchangeRates {
  active: boolean;
  base?: string;
  /** 1 单位外币 = N 人民币 */
  rates?: Record<string, number>;
  updatedAt?: string;
  fetchedAt?: string;
  provider?: string;
  nextAt?: string;
}

/**
 * 人民币参考价汇率。
 *
 * 只在用户开着"显示人民币参考价"时才请求；关掉之后连一次网络往返都不发生
 * （这正是把开关放在后端 Config 里的收益之一）。
 *
 * staleTime 30 分钟与"后端每天 0 点刷新"不冲突：前端需要的是同一天内相对
 * 稳定的一个数，而不是和后台瞬间同步；真正的权威值仍在后端。
 */
export function useExchangeRates() {
  const settings = useSettings();
  const enabled = (settings.data?.displayMode ?? "both") !== "off";
  return useQuery({
    queryKey: ["exchange", "rates"],
    queryFn: async () => (await api.get<ExchangeRates>("/exchange/rates")).data,
    enabled,
    staleTime: 30 * 60 * 1000,
    gcTime: 60 * 60 * 1000,
    retry: false,
  });
}
