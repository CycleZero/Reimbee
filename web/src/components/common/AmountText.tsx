import { formatAmount } from '@/utils/format';

interface Props {
  amount: number;
  showSymbol?: boolean;
  precision?: number;
}

export function AmountText({ amount, showSymbol = true, precision = 2 }: Props) {
  const yuan = amount / 100;
  const formatted = yuan.toLocaleString('zh-CN', {
    minimumFractionDigits: precision,
    maximumFractionDigits: precision,
  });
  return <span>{showSymbol ? `¥${formatted}` : formatted}</span>;
}
