import { useEffect, useCallback, useState } from 'react';
import { createPortal } from 'react-dom';
import type { PhotoDocument } from '../types';

interface PhotoDetailModalProps {
  photo: PhotoDocument;
  onClose: () => void;
  onPrev?: () => void;
  onNext?: () => void;
  hasPrev?: boolean;
  hasNext?: boolean;
}

const TAG_COLORS = [
  'bg-rose-100 text-rose-700',
  'bg-amber-100 text-amber-700',
  'bg-emerald-100 text-emerald-700',
  'bg-sky-100 text-sky-700',
  'bg-violet-100 text-violet-700',
  'bg-fuchsia-100 text-fuchsia-700',
  'bg-teal-100 text-teal-700',
  'bg-orange-100 text-orange-700',
];

const STATUS_COLORS: Record<string, string> = {
  analyzed: 'bg-green-100 text-green-700',
  analyzing: 'bg-yellow-100 text-yellow-700',
  failed: 'bg-red-100 text-red-700',
  pending_analysis: 'bg-gray-100 text-gray-600',
  unanalyzed: 'bg-gray-100 text-gray-600',
};

const STATUS_LABELS: Record<string, string> = {
  analyzed: '已分析',
  analyzing: '分析中',
  failed: '分析失败',
  pending_analysis: '等待分析',
  unanalyzed: '未分析',
};

function tagColor(tag: string): string {
  let hash = 0;
  for (let i = 0; i < tag.length; i++) {
    hash = tag.charCodeAt(i) + ((hash << 5) - hash);
  }
  return TAG_COLORS[Math.abs(hash) % TAG_COLORS.length];
}

const CHINESE_COLOR_MAP: Record<string, string> = {
  '红色': '#EF4444', '深红': '#DC2626', '浅红': '#FCA5A5',
  '橙色': '#F97316', '深橙': '#EA580C',
  '黄色': '#EAB308', '深黄': '#CA8A04', '金黄色': '#F59E0B',
  '绿色': '#22C55E', '深绿': '#166534', '浅绿': '#86EFAC', '碧绿': '#10B981',
  '蓝色': '#3B82F6', '深蓝': '#1D4ED8', '浅蓝': '#93C5FD', '天蓝': '#0EA5E9',
  '紫色': '#A855F7', '深紫': '#7E22CE', '浅紫': '#C4B5FD',
  '粉色': '#EC4899', '深粉': '#DB2777', '浅粉': '#F9A8D4',
  '棕色': '#92400E', '深棕': '#78350F',
  '黑色': '#1F2937',
  '白色': '#F9FAFB',
  '灰色': '#9CA3AF', '深灰': '#4B5563', '浅灰': '#E5E7EB',
  '青色': '#06B6D4',
  '金色': '#D97706',
  '银色': '#D1D5DB',
  '米色': '#F5DEB3',
  '卡其色': '#C3B091',
};

function resolveColor(name: string): string {
  if (CHINESE_COLOR_MAP[name]) return CHINESE_COLOR_MAP[name];
  let hash = 0;
  for (let i = 0; i < name.length; i++) {
    hash = name.charCodeAt(i) + ((hash << 5) - hash);
  }
  const h = Math.abs(hash) % 360;
  return `hsl(${h}, 55%, 55%)`;
}

function formatAperture(aperture: string): string {
  if (aperture.startsWith('f/') || aperture.startsWith('F/')) return aperture;
  const num = parseFloat(aperture);
  if (!isNaN(num)) return `f/${num}`;
  return aperture;
}

function formatSize(bytes: number): string {
  if (bytes >= 1024 * 1024) {
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  }
  if (bytes >= 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`;
  }
  return `${bytes} B`;
}

function formatTimestamp(ts: number): string {
  return new Date(ts * 1000).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // fallback: ignore
    }
  }, [text]);

  return (
    <button
      onClick={handleCopy}
      className="flex-shrink-0 p-1 rounded hover:bg-gray-200 transition-colors cursor-pointer"
      title={copied ? '已复制' : '复制'}
      aria-label="复制"
    >
      {copied ? (
        <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
          <path d="M2 6l3 3 5-6" />
        </svg>
      ) : (
        <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
          <rect x="4" y="4" width="7" height="7" rx="1" />
          <path d="M8 4V2.5A0.5 0.5 0 0 0 7.5 2H2.5A0.5 0.5 0 0 0 2 2.5V7.5A0.5 0.5 0 0 0 2.5 8H4" />
        </svg>
      )}
    </button>
  );
}

function PhotoDetailModal({
  photo,
  onClose,
  onPrev,
  onNext,
  hasPrev,
  hasNext,
}: PhotoDetailModalProps) {
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
      if (e.key === 'ArrowLeft' && hasPrev && onPrev) onPrev();
      if (e.key === 'ArrowRight' && hasNext && onNext) onNext();
    },
    [onClose, onPrev, onNext, hasPrev, hasNext],
  );

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown);
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.body.style.overflow = prev;
    };
  }, [handleKeyDown]);

  const imageUrl = `/photos/${photo.path.replace(/^\/+/, '')}`;

  const dateStr = photo.exif?.date_time_original
    ? new Date(photo.exif.date_time_original).toLocaleDateString('zh-CN', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
        weekday: 'long',
        hour: '2-digit',
        minute: '2-digit',
      })
    : null;

  const hasExif =
    photo.exif &&
    (photo.exif.camera_model ||
      photo.exif.lens_model ||
      photo.exif.focal_length ||
      photo.exif.aperture ||
      photo.exif.iso ||
      photo.exif.date_time_original ||
      photo.exif.gps_lat != null);

  const hasAnalysis =
    photo.description ||
    photo.tags.length > 0 ||
    photo.objects.length > 0 ||
    photo.scene_type ||
    photo.colors.length > 0;

  return createPortal(
    <div className="fixed inset-0 z-50" role="dialog" aria-modal="true">
      <div
        className="absolute inset-0 bg-black/75 backdrop-blur-xs"
        onClick={onClose}
      />

      <div className="relative z-10 flex items-center justify-center w-full h-full p-3 sm:p-6">
        <div className="relative flex flex-col lg:flex-row w-full max-w-[1100px] max-h-[92vh] bg-white rounded-2xl shadow-2xl overflow-hidden animate-[fadeIn_0.2s_ease-out]">
          <div className="absolute top-4 right-4 z-20 flex items-center gap-2">
            <a
              href={imageUrl}
              download
              className="flex items-center justify-center w-9 h-9 rounded-full bg-black/20 hover:bg-black/40 text-white backdrop-blur-sm transition-colors cursor-pointer"
              title="下载原图"
              aria-label="下载原图"
            >
              <svg
                width="16"
                height="16"
                viewBox="0 0 16 16"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <path d="M8 2v8" />
                <path d="M4 7l4 4 4-4" />
                <path d="M2 12v1.5A0.5 0.5 0 0 0 2.5 14h11a0.5 0.5 0 0 0 .5-.5V12" />
              </svg>
            </a>

            <button
              onClick={onClose}
              className="flex items-center justify-center w-9 h-9 rounded-full bg-black/20 hover:bg-black/40 text-white backdrop-blur-sm transition-colors cursor-pointer"
              aria-label="关闭"
            >
              <svg
                width="18"
                height="18"
                viewBox="0 0 18 18"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
              >
                <path d="M4 4l10 10M14 4L4 14" />
              </svg>
            </button>
          </div>

          {hasPrev && (
            <button
              onClick={onPrev}
              className="absolute top-1/2 -translate-y-1/2 left-3 z-20 flex items-center justify-center w-10 h-10 rounded-full bg-black/20 hover:bg-black/40 text-white backdrop-blur-sm transition-colors cursor-pointer"
              aria-label="上一张"
            >
              <svg
                width="20"
                height="20"
                viewBox="0 0 20 20"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <path d="M12 4l-6 6 6 6" />
              </svg>
            </button>
          )}
          {hasNext && (
            <button
              onClick={onNext}
              className="absolute top-1/2 -translate-y-1/2 right-3 z-20 flex items-center justify-center w-10 h-10 rounded-full bg-black/20 hover:bg-black/40 text-white backdrop-blur-sm transition-colors cursor-pointer"
              aria-label="下一张"
            >
              <svg
                width="20"
                height="20"
                viewBox="0 0 20 20"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <path d="M8 4l6 6-6 6" />
              </svg>
            </button>
          )}

          {/* ── Image area ── */}
          <div className="relative flex items-center justify-center bg-gray-900 lg:w-[55%] min-h-[280px] lg:min-h-0 max-h-[50vh] lg:max-h-full">
            <img
              src={imageUrl}
              alt={photo.description || '照片'}
              className="w-full h-full object-contain"
              loading="eager"
            />
          </div>

          {/* ── Sidebar ── */}
          <div className="flex flex-col lg:w-[45%] overflow-y-auto">
            <div className="p-6 lg:p-8 space-y-6">
              {/* ── Section 1: AI 分析 ── */}
              <section>
                <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-4">
                  AI 分析
                </h3>

                {hasAnalysis ? (
                  <>
                    {photo.description && (
                      <p className="text-base lg:text-lg font-medium text-gray-900 leading-relaxed mb-5">
                        {photo.description}
                      </p>
                    )}

                    {(photo.scene_type ||
                      photo.people_count > 0 ||
                      photo.has_text) && (
                      <div className="flex flex-wrap items-center gap-3 mb-4">
                        {photo.scene_type && (
                          <span className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-gray-100 text-gray-700 text-xs font-medium">
                            <svg
                              width="14"
                              height="14"
                              viewBox="0 0 14 14"
                              fill="none"
                              stroke="currentColor"
                              strokeWidth="1.5"
                            >
                              <rect
                                x="1.5"
                                y="1.5"
                                width="11"
                                height="11"
                                rx="2"
                              />
                            </svg>
                            {photo.scene_type}
                          </span>
                        )}
                        {photo.people_count > 0 && (
                          <span className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-gray-100 text-gray-700 text-xs font-medium">
                            <svg
                              width="14"
                              height="14"
                              viewBox="0 0 14 14"
                              fill="none"
                              stroke="currentColor"
                              strokeWidth="1.5"
                            >
                              <circle cx="5" cy="4.5" r="1.5" />
                              <circle cx="9" cy="4.5" r="1.5" />
                              <path d="M3 10c0-1.5 1.5-2.5 4-2.5s4 1 4 2.5" />
                            </svg>
                            {photo.people_count} 人
                          </span>
                        )}
                        {photo.has_text && (
                          <span className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-blue-50 text-blue-700 text-xs font-medium">
                            <svg
                              width="14"
                              height="14"
                              viewBox="0 0 14 14"
                              fill="none"
                              stroke="currentColor"
                              strokeWidth="1.5"
                            >
                              <path d="M2 3h10M4 6h6M4 9h4" />
                            </svg>
                            含文字
                          </span>
                        )}
                      </div>
                    )}

                    {photo.tags.length > 0 && (
                      <div className="flex flex-wrap gap-2 mb-4">
                        {photo.tags.map((t) => (
                          <span
                            key={t}
                            className={`inline-flex items-center px-2.5 py-1 rounded-full text-xs font-medium ${tagColor(t)}`}
                          >
                            {t}
                          </span>
                        ))}
                      </div>
                    )}

                    {photo.colors.length > 0 && (
                      <div className="flex items-center gap-2 mb-4">
                        <span className="text-xs text-gray-400">主色调</span>
                        <div className="flex gap-1.5">
                          {photo.colors.map((c) => (
                            <div key={c} className="flex items-center gap-1.5">
                              <div
                                className="w-5 h-5 rounded-full border border-gray-200 shadow-sm"
                                style={{ backgroundColor: resolveColor(c) }}
                              />
                              <span className="text-xs text-gray-500">{c}</span>
                            </div>
                          ))}
                        </div>
                      </div>
                    )}

                    {photo.objects.length > 0 && (
                      <div className="mb-4">
                        <span className="text-xs text-gray-400 block mb-2">
                          识别物体
                        </span>
                        <div className="flex flex-wrap gap-2">
                          {photo.objects.map((obj) => (
                            <span
                              key={obj}
                              className="inline-flex items-center px-2.5 py-1 rounded-md bg-gray-50 text-gray-600 text-xs border border-gray-200"
                            >
                              {obj}
                            </span>
                          ))}
                        </div>
                      </div>
                    )}

                    {photo.has_text && photo.text && (
                      <div className="mb-4 p-3 rounded-lg border border-gray-200 bg-gray-50">
                        <span className="text-xs text-gray-400 block mb-2">
                          提取文字
                        </span>
                        <p className="text-sm text-gray-700 leading-relaxed whitespace-pre-wrap break-words">
                          {photo.text}
                        </p>
                      </div>
                    )}

                    {photo.confidence != null && (
                      <p className="text-xs text-gray-400">
                        置信度: {Math.round(photo.confidence * 100)}%
                      </p>
                    )}
                  </>
                ) : (
                  <p className="text-sm text-gray-400 italic">
                    {photo.status === 'analyzing'
                      ? '正在分析中...'
                      : photo.status === 'unanalyzed'
                        ? '尚未分析'
                        : photo.status === 'failed'
                          ? '分析失败'
                          : '暂无分析数据'}
                  </p>
                )}
              </section>

              {/* ── Section 2: 拍摄信息 ── */}
              <section>
                <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-4">
                  拍摄信息
                </h3>

                {hasExif ? (
                  <>
                    {(photo.exif!.camera_model ||
                      photo.exif!.lens_model) && (
                      <div className="mb-4">
                        {photo.exif!.camera_model && (
                          <p className="text-sm font-semibold text-gray-900">
                            {photo.exif!.camera_model}
                          </p>
                        )}
                        {photo.exif!.lens_model && (
                          <p className="text-sm text-gray-500">
                            {photo.exif!.lens_model}
                          </p>
                        )}
                      </div>
                    )}

                    {(photo.exif!.focal_length ||
                      photo.exif!.aperture ||
                      photo.exif!.iso != null) && (
                      <div className="flex flex-wrap gap-3 mb-4">
                        {photo.exif!.focal_length && (
                          <span className="inline-flex items-center px-3 py-1.5 rounded-lg bg-gray-100 text-gray-700 text-sm font-mono">
                            {photo.exif!.focal_length}
                          </span>
                        )}
                        {photo.exif!.aperture && (
                          <span className="inline-flex items-center px-3 py-1.5 rounded-lg bg-gray-100 text-gray-700 text-sm font-mono">
                            {formatAperture(photo.exif!.aperture)}
                          </span>
                        )}
                        {photo.exif!.iso != null && (
                          <span className="inline-flex items-center px-3 py-1.5 rounded-lg bg-gray-100 text-gray-700 text-sm font-mono">
                            ISO {photo.exif!.iso}
                          </span>
                        )}
                      </div>
                    )}

                    {dateStr && (
                      <p className="text-sm text-gray-500 mb-2">{dateStr}</p>
                    )}

                    {photo.exif!.gps_lat != null &&
                      photo.exif!.gps_lon != null && (
                        <p className="text-xs text-gray-400 font-mono">
                          {photo.exif!.gps_lat.toFixed(6)},{' '}
                          {photo.exif!.gps_lon.toFixed(6)}
                        </p>
                      )}
                  </>
                ) : (
                  <p className="text-sm text-gray-400 italic">
                    暂无 EXIF 数据
                  </p>
                )}
              </section>

              {/* ── Section 3: 文件信息 ── */}
              <section>
                <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-4">
                  文件信息
                </h3>

                <div className="space-y-2.5 text-sm">
                  <div className="flex items-start gap-2">
                    <span className="text-gray-400 shrink-0">路径</span>
                    <span className="font-mono text-gray-600 break-all text-xs leading-relaxed flex-1">
                      {photo.path}
                    </span>
                    <CopyButton text={photo.path} />
                  </div>

                  <div className="flex items-center gap-2">
                    <span className="text-gray-400 shrink-0">大小</span>
                    <span className="font-mono text-gray-700 text-sm">
                      {formatSize(photo.size)}
                    </span>
                  </div>

                  <div className="flex items-center gap-2">
                    <span className="text-gray-400 shrink-0">修改时间</span>
                    <span className="font-mono text-gray-700 text-sm">
                      {formatTimestamp(photo.mtime)}
                    </span>
                  </div>

                  <div className="flex items-center gap-2">
                    <span className="text-gray-400 shrink-0">创建时间</span>
                    <span className="font-mono text-gray-700 text-sm">
                      {formatTimestamp(photo.created_at)}
                    </span>
                  </div>

                  {photo.analyzed_at != null && (
                    <div className="flex items-center gap-2">
                      <span className="text-gray-400 shrink-0">分析时间</span>
                      <span className="font-mono text-gray-700 text-sm">
                        {formatTimestamp(photo.analyzed_at)}
                      </span>
                    </div>
                  )}

                  <div className="flex items-center gap-2">
                    <span className="text-gray-400 shrink-0">状态</span>
                    <span
                      className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${STATUS_COLORS[photo.status] || STATUS_COLORS.unanalyzed}`}
                    >
                      {STATUS_LABELS[photo.status] || photo.status}
                    </span>
                  </div>
                </div>
              </section>
            </div>
          </div>
        </div>
      </div>

      <style>{`
        @keyframes fadeIn {
          from { opacity: 0; transform: scale(0.97); }
          to { opacity: 1; transform: scale(1); }
        }
      `}</style>
    </div>,
    document.body,
  );
}

export default PhotoDetailModal;
