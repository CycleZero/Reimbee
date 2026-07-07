import { Steps } from 'antd';
import { useChatStore } from '../stores/chatStore';

const PHASES = [
  { key: 'phase1_collect', title: '信息收集' },
  { key: 'phase2_validate', title: '校验确认' },
  { key: 'phase3_execute', title: '执行提交' },
] as const;

/**
 * 报销流程阶段指示器
 * 仅在非 idle 状态时显示
 */
export function PhaseIndicator() {
  const phase = useChatStore((s) => s.currentPhase);
  if (phase === 'idle') return null;

  const idx = PHASES.findIndex((p) => p.key === phase);
  return (
    <Steps
      size="small"
      current={idx >= 0 ? idx : 0}
      items={PHASES.map((p) => ({ title: p.title }))}
      style={{ marginBottom: 16 }}
    />
  );
}
