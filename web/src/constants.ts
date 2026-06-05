/**
 * 标签颜色调色板（8 色）
 * 通过 DJB2 哈希确保同一标签始终同色
 */
export const TAG_COLORS = [
  'bg-red-100 text-red-700',
  'bg-amber-100 text-amber-700',
  'bg-emerald-100 text-emerald-700',
  'bg-sky-100 text-sky-700',
  'bg-orange-100 text-orange-700',
  'bg-rose-100 text-rose-700',
  'bg-teal-100 text-teal-700',
  'bg-pink-100 text-pink-700',
];

/**
 * DJB2 哈希 → 为标签分配确定性颜色
 */
export function tagColor(tag: string): string {
  let hash = 0;
  for (let i = 0; i < tag.length; i++) {
    hash = tag.charCodeAt(i) + ((hash << 5) - hash);
  }
  return TAG_COLORS[Math.abs(hash) % TAG_COLORS.length];
}

/**
 * 照片状态 → 中文标签映射表
 */
export const STATUS_LABELS: Record<string, string> = {
  analyzed: '已分析',
  analyzing: '分析中',
  failed: '失败',
  pending_analysis: '待分析',
  unanalyzed: '未分析',
};

/**
 * 照片状态 → Tailwind CSS 颜色类名映射
 */
export const STATUS_COLORS: Record<string, string> = {
  analyzed: 'bg-status-success-bg text-status-success',
  analyzing: 'bg-status-warning-bg text-status-warning',
  failed: 'bg-status-error-bg text-status-error',
  pending_analysis: 'bg-status-neutral-bg text-status-neutral',
  unanalyzed: 'bg-status-neutral-bg text-status-neutral',
};

/**
 * 场景类型 → 中文标签映射表
 */
export const SCENE_TYPE_LABELS: Record<string, string> = {
  outdoor: '室外',
  indoor: '室内',
  underwater: '水下',
  aerial: '航拍',
  studio: '影棚',
  night: '夜景',
  unknown: '未知',
};
