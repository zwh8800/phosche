import { useEffect, useCallback } from 'react';
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

function tagColor(tag: string): string {
  let hash = 0;
  for (let i = 0; i < tag.length; i++) {
    hash = tag.charCodeAt(i) + ((hash << 5) - hash);
  }
  return TAG_COLORS[Math.abs(hash) % TAG_COLORS.length];
}

function formatAperture(aperture: string): string {
  if (aperture.startsWith('f/') || aperture.startsWith('F/')) return aperture;
  const num = parseFloat(aperture);
  if (!isNaN(num)) return `f/${num}`;
  return aperture;
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

  const imageUrl = `/photos/${photo.path}`;

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
          <button
            onClick={onClose}
            className="absolute top-4 right-4 z-20 flex items-center justify-center w-9 h-9 rounded-full bg-black/20 hover:bg-black/40 text-white backdrop-blur-sm transition-colors cursor-pointer"
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

          <div className="relative flex items-center justify-center bg-gray-900 lg:w-[55%] min-h-[280px] lg:min-h-0 max-h-[50vh] lg:max-h-full">
            <img
              src={imageUrl}
              alt={photo.description || '照片'}
              className="w-full h-full object-contain"
              loading="eager"
            />
          </div>

          <div className="flex flex-col lg:w-[45%] overflow-y-auto">
            <div className="p-6 lg:p-8 space-y-6">
              {hasAnalysis ? (
                <section>
                  <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-4">
                    AI 分析
                  </h3>

                  {photo.description && (
                    <p className="text-base lg:text-lg font-medium text-gray-900 leading-relaxed mb-5">
                      {photo.description}
                    </p>
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

                  {photo.colors.length > 0 && (
                    <div className="flex items-center gap-2 mb-4">
                      <span className="text-xs text-gray-400">主色调</span>
                      <div className="flex gap-1.5">
                        {photo.colors.map((c) => (
                          <div
                            key={c}
                            className="w-6 h-6 rounded-full border border-gray-200 shadow-sm"
                            style={{ backgroundColor: c }}
                            title={c}
                          />
                        ))}
                      </div>
                    </div>
                  )}

                  {photo.objects.length > 0 && (
                    <div>
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
                </section>
              ) : (
                <section>
                  <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-4">
                    AI 分析
                  </h3>
                  <p className="text-sm text-gray-400 italic">
                    {photo.status === 'analyzing'
                      ? '正在分析中...'
                      : photo.status === 'unanalyzed'
                        ? '尚未分析'
                        : photo.status === 'failed'
                          ? '分析失败'
                          : '暂无分析数据'}
                  </p>
                </section>
              )}

              {hasExif ? (
                <section>
                  <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-4">
                    拍摄信息
                  </h3>

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
                </section>
              ) : (
                <section>
                  <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-4">
                    拍摄信息
                  </h3>
                  <p className="text-sm text-gray-400 italic">
                    暂无 EXIF 数据
                  </p>
                </section>
              )}

              <section className="pt-4 border-t border-gray-100">
                <div className="flex items-center justify-between text-xs text-gray-400">
                  <span className="font-mono">{photo.id}</span>
                  <span>{photo.size > 0 ? `${(photo.size / 1024 / 1024).toFixed(1)} MB` : ''}</span>
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
