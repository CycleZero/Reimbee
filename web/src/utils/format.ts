import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import 'dayjs/locale/zh-cn';

dayjs.extend(relativeTime);
dayjs.locale('zh-cn');

// 金额：分 → 元转换（后端 DB 存储分，前端展示元）
export function fenToYuan(fen: number): number {
  return fen / 100;
}

// 金额：元 → 分转换（提交时使用）
export function yuanToFen(yuan: number): number {
  return Math.round(yuan * 100);
}

// 金额格式化：¥1,234.56
export function formatAmount(fen: number, showSymbol = true): string {
  const yuan = fenToYuan(fen);
  const formatted = yuan.toLocaleString('zh-CN', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
  return showSymbol ? `¥${formatted}` : formatted;
}

// 日期格式化
export function formatDate(date: string, template = 'YYYY-MM-DD HH:mm:ss'): string {
  return dayjs(date).format(template);
}

// 相对时间（如 "3分钟前"）
export function fromNow(date: string): string {
  return dayjs(date).fromNow();
}
