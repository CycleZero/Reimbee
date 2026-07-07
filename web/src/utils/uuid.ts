// UUID v7 生成（时间排序 + 随机）
// 用于生成 SSE 会话 ID（session_id），前端在每次新对话时生成
export function generateUUIDv7(): string {
  const ts = Date.now().toString(16).padStart(12, '0');
  const rand = crypto.getRandomValues(new Uint8Array(10));
  const randHex = Array.from(rand, (b) => b.toString(16).padStart(2, '0')).join('');
  return `${ts.slice(0, 8)}-${ts.slice(8)}-7${randHex.slice(0, 3)}-${randHex.slice(3, 7)}-${randHex.slice(7)}`;
}
