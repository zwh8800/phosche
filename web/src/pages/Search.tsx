import {
  useState,
  useEffect,
  useRef,
  memo,
  type FormEvent,
  type ChangeEvent,
} from 'react';
import { useSearchParams, Link } from 'react-router-dom';
import { useQuery, useInfiniteQuery } from '@tanstack/react-query';
import { searchPhotos, fetchFilters } from '../api/photos';
import type {
  FiltersResponse,
  PhotoDocument,
} from '../types';

const PAGE_SIZE = 20;

function formatMtime(ts?: number): string {
  if (!ts) return '';
  return new Date(ts * 1000).toLocaleDateString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  });
}

function formatExifDate(raw?: string): string {
  if (!raw) return '';
  const d = new Date(raw);
  if (!Number.isNaN(d.getTime())) return d.toISOString().slice(0, 10);
  return raw.slice(0, 10);
}

function Spinner() {
  return (
    <div className="flex items-center justify-center py-12">
      <div className="w-8 h-8 border-3 border-gray-200 border-t-purple-500 rounded-full animate-spin" />
    </div>
  );
}

function SkeletonCard() {
  return (
    <div className="animate-pulse">
      <div className="aspect-[4/3] rounded-xl bg-gray-200" />
      <div className="mt-2 space-y-1.5 p-1">
        <div className="h-3 w-3/4 rounded bg-gray-200" />
        <div className="h-3 w-1/2 rounded bg-gray-200" />
      </div>
    </div>
  );
}

const STATUS_LABELS: Record<string, string> = {
  analyzed: '已分析',
  analyzing: '分析中',
  failed: '失败',
  pending_analysis: '待分析',
  unanalyzed: '未分析',
};

const STATUS_COLORS: Record<string, string> = {
  analyzed: 'bg-green-100 text-green-700',
  analyzing: 'bg-yellow-100 text-yellow-700',
  failed: 'bg-red-100 text-red-700',
  pending_analysis: 'bg-gray-100 text-gray-600',
  unanalyzed: 'bg-gray-100 text-gray-600',
};

const SCENE_TYPE_LABELS: Record<string, string> = {
  outdoor: '室外',
  indoor: '室内',
  underwater: '水下',
  aerial: '航拍',
  studio: '影棚',
  night: '夜景',
  unknown: '未知',
};

const PhotoCard = memo(function PhotoCard({ photo }: { photo: PhotoDocument }) {
  const dateLabel =
    formatExifDate(photo.exif?.date_time_original) ||
    formatMtime(photo.mtime);

  return (
    <Link
      to={`/photo/${encodeURIComponent(photo.path)}`}
      className="group block rounded-xl overflow-hidden bg-white border border-gray-200 shadow-sm hover:shadow-md transition-shadow"
    >
      <div className="relative aspect-[4/3] bg-gray-100 overflow-hidden">
        <img
          src={`/photos/${photo.path.replace(/^\/+/, '')}?thumb=1`}
          alt={photo.description || photo.path}
          loading="lazy"
          className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
          onError={(e) => {
            const el = e.currentTarget;
            el.style.display = 'none';
            el.nextElementSibling?.classList.remove('hidden');
          }}
        />
        <div className="hidden absolute inset-0 flex items-center justify-center bg-gray-100 text-gray-400 text-sm">
          无法加载图片
        </div>
        <span
          className={`absolute top-2 right-2 text-[10px] px-1.5 py-0.5 rounded font-medium ${STATUS_COLORS[photo.status] || STATUS_COLORS.unanalyzed}`}
        >
          {STATUS_LABELS[photo.status] || STATUS_LABELS.unanalyzed}
        </span>
      </div>
      <div className="p-3 space-y-2">
        {dateLabel && (
          <p className="text-xs text-gray-500">{dateLabel}</p>
        )}
        {photo.status === 'analyzing' ? (
          <>
            <div className="animate-pulse space-y-2">
              <div className="h-3 w-full rounded bg-gray-200" />
              <div className="h-3 w-3/4 rounded bg-gray-200" />
            </div>
            <div className="animate-pulse">
              <div className="inline-block h-5 w-12 rounded-full bg-gray-200" />
            </div>
            <div className="animate-pulse flex gap-1">
              <div className="h-4 w-10 rounded bg-gray-200" />
              <div className="h-4 w-12 rounded bg-gray-200" />
              <div className="h-4 w-8 rounded bg-gray-200" />
            </div>
          </>
        ) : (
          <>
            {photo.description && (
              <p className="text-sm text-gray-800 line-clamp-2 leading-snug">
                {photo.description}
              </p>
            )}
            {photo.scene_type && (
              <span className="inline-block text-[11px] px-2 py-0.5 bg-purple-50 text-purple-600 rounded-full font-medium">
                {SCENE_TYPE_LABELS[photo.scene_type] || photo.scene_type}
              </span>
            )}
            {photo.tags && photo.tags.length > 0 && (
              <div className="flex flex-wrap gap-1">
                {photo.tags.slice(0, 3).map((tag) => (
                  <span
                    key={tag}
                    className="text-[10px] px-1.5 py-px bg-gray-100 text-gray-600 rounded"
                  >
                    {tag}
                  </span>
                ))}
                {photo.tags.length > 3 && (
                  <span className="text-[10px] text-gray-400">
                    +{photo.tags.length - 3}
                  </span>
                )}
              </div>
            )}
          </>
        )}
      </div>
    </Link>
  );
});

export default function Search() {
  const [searchParams, setSearchParams] = useSearchParams();

  const [query, setQuery] = useState(searchParams.get('query') || '');
  const [dateFrom, setDateFrom] = useState(searchParams.get('date_from') || '');
  const [dateTo, setDateTo] = useState(searchParams.get('date_to') || '');
  const [sceneType, setSceneType] = useState(searchParams.get('scene_type') || '');
  const [cameraModel, setCameraModel] = useState(searchParams.get('camera_model') || '');
  const [selectedTags, setSelectedTags] = useState<string[]>(() => {
    const raw = searchParams.get('tags');
    return raw ? raw.split(',').filter(Boolean) : [];
  });
  const [showFilters, setShowFilters] = useState(false);
  const [searchActive, setSearchActive] = useState(false);
  const sentinelRef = useRef<HTMLDivElement>(null);
  const initialMount = useRef(true);

  const { data: filters } = useQuery<FiltersResponse>({
    queryKey: ['filters'],
    queryFn: fetchFilters,
    staleTime: 5 * 60 * 1000,
  });

  const {
    data,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    isLoading,
    isError,
    error,
    refetch,
  } = useInfiniteQuery({
    queryKey: ['search', query, dateFrom, dateTo, sceneType, cameraModel, selectedTags],
    queryFn: ({ pageParam = 1 }) =>
      searchPhotos({
        query: query || undefined,
        date_from: dateFrom || undefined,
        date_to: dateTo || undefined,
        tags: selectedTags.length > 0 ? selectedTags : undefined,
        scene_type: sceneType || undefined,
        camera_model: cameraModel || undefined,
        page: pageParam,
        page_size: PAGE_SIZE,
      }),
    getNextPageParam: (lastPage) => {
      const totalPages = Math.ceil(lastPage.total / lastPage.page_size);
      return lastPage.page < totalPages ? lastPage.page + 1 : undefined;
    },
    initialPageParam: 1,
    enabled: searchActive,
  });

  // Auto-search on mount if URL has params; debounce on filter/query changes
  useEffect(() => {
    if (initialMount.current) {
      initialMount.current = false;
      if (searchParams.toString()) {
        setSearchActive(true);
        refetch();
      }
      return;
    }

    const timer = setTimeout(() => {
      setSearchParams(buildParams(), { replace: true });
      if (!searchActive) setSearchActive(true);
      refetch();
    }, 300);
    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, dateFrom, dateTo, sceneType, cameraModel, selectedTags]);

  // Infinite scroll sentinel
  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasNextPage && !isFetchingNextPage) {
          fetchNextPage();
        }
      },
      { threshold: 0.1 },
    );

    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const handleSearch = (e?: FormEvent) => {
    e?.preventDefault();
    setSearchParams(buildParams(), { replace: true });
    setSearchActive(true);
    refetch();
  };

  const buildParams = (): URLSearchParams => {
    const p = new URLSearchParams();
    if (query) p.set('query', query);
    if (dateFrom) p.set('date_from', dateFrom);
    if (dateTo) p.set('date_to', dateTo);
    if (sceneType) p.set('scene_type', sceneType);
    if (cameraModel) p.set('camera_model', cameraModel);
    if (selectedTags.length > 0) p.set('tags', selectedTags.join(','));
    return p;
  };

  const toggleTag = (tag: string) => {
    setSelectedTags((prev) =>
      prev.includes(tag) ? prev.filter((t) => t !== tag) : [...prev, tag],
    );
  };

  const resetFilters = () => {
    setDateFrom('');
    setDateTo('');
    setSceneType('');
    setCameraModel('');
    setSelectedTags([]);
    setQuery('');
    setSearchParams({}, { replace: true });
    setSearchActive(false);
  };

  const activeFilterCount =
    (dateFrom ? 1 : 0) +
    (dateTo ? 1 : 0) +
    (sceneType ? 1 : 0) +
    (cameraModel ? 1 : 0) +
    selectedTags.length;

  const allPhotos = data?.pages.flatMap((page) => page.hits) ?? [];
  const total = data?.pages[0]?.total ?? 0;
  const totalPages = data?.pages[0]?.total_pages ?? 0;

  return (
    <div className="space-y-6">
      <form onSubmit={handleSearch} className="flex gap-2">
        <div className="relative flex-1">
          <svg
            className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400 pointer-events-none"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
            />
          </svg>
          <input
            type="text"
            value={query}
            onChange={(e: ChangeEvent<HTMLInputElement>) => setQuery(e.target.value)}
            placeholder="输入关键词搜索照片…"
            className="w-full pl-9 pr-4 py-2.5 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent bg-white placeholder-gray-400"
          />
        </div>
        <button
          type="submit"
          disabled={isLoading}
          className="px-5 py-2.5 text-sm font-medium text-white bg-purple-600 hover:bg-purple-700 disabled:opacity-50 rounded-lg transition-colors"
        >
          搜索
        </button>
      </form>

      <div>
        <button
          type="button"
          onClick={() => setShowFilters((v) => !v)}
          className={`inline-flex items-center gap-1.5 text-sm font-medium transition-colors ${
            showFilters ? 'text-purple-600' : 'text-gray-600 hover:text-gray-900'
          }`}
        >
          <svg
            className={`w-4 h-4 transition-transform ${showFilters ? 'rotate-180' : ''}`}
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M19 9l-7 7-7-7"
            />
          </svg>
          筛选条件
          {activeFilterCount > 0 && (
            <span className="inline-flex items-center justify-center w-5 h-5 text-[11px] font-semibold text-white bg-purple-500 rounded-full">
              {activeFilterCount}
            </span>
          )}
        </button>
      </div>

      {showFilters && (
        <div className="p-5 bg-white border border-gray-200 rounded-xl space-y-5">
          <div>
            <label className="block text-xs font-medium text-gray-500 mb-2">
              拍摄日期
            </label>
            <div className="flex items-center gap-2">
              <input
                type="date"
                value={dateFrom}
                onChange={(e: ChangeEvent<HTMLInputElement>) => setDateFrom(e.target.value)}
                className="flex-1 px-3 py-2 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent bg-white"
              />
              <span className="text-xs text-gray-400">至</span>
              <input
                type="date"
                value={dateTo}
                onChange={(e: ChangeEvent<HTMLInputElement>) => setDateTo(e.target.value)}
                className="flex-1 px-3 py-2 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent bg-white"
              />
            </div>
          </div>

          <div>
            <label htmlFor="filter-scene" className="block text-xs font-medium text-gray-500 mb-2">
              场景类型
            </label>
            <select
              id="filter-scene"
              value={sceneType}
              onChange={(e: ChangeEvent<HTMLSelectElement>) => setSceneType(e.target.value)}
              className="w-full px-3 py-2 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent bg-white"
            >
              <option value="">全部</option>
              {(filters?.scene_types || []).map((s) => (
                <option key={s} value={s}>{s}</option>
              ))}
            </select>
          </div>

          <div>
            <label htmlFor="filter-camera" className="block text-xs font-medium text-gray-500 mb-2">
              相机型号
            </label>
            <select
              id="filter-camera"
              value={cameraModel}
              onChange={(e: ChangeEvent<HTMLSelectElement>) => setCameraModel(e.target.value)}
              className="w-full px-3 py-2 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent bg-white"
            >
              <option value="">全部</option>
              {(filters?.cameras || []).map((c) => (
                <option key={c} value={c}>{c}</option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-xs font-medium text-gray-500 mb-2">标签</label>
            {filters?.tags && filters.tags.length > 0 ? (
              <div className="flex flex-wrap gap-1.5">
                {filters.tags.map((tag) => {
                  const active = selectedTags.includes(tag);
                  return (
                    <button
                      key={tag}
                      type="button"
                      onClick={() => toggleTag(tag)}
                      className={`text-xs px-2.5 py-1 rounded-full font-medium transition-colors ${
                        active
                          ? 'bg-purple-100 text-purple-700 border border-purple-300'
                          : 'bg-gray-100 text-gray-600 border border-transparent hover:bg-gray-200'
                      }`}
                    >
                      {tag}
                    </button>
                  );
                })}
              </div>
            ) : (
              <p className="text-xs text-gray-400">暂无可用标签</p>
            )}
          </div>

          {activeFilterCount > 0 && (
            <div className="pt-1">
              <button
                type="button"
                onClick={resetFilters}
                className="text-xs text-gray-500 hover:text-gray-700 underline"
              >
                重置所有筛选条件
              </button>
            </div>
          )}
        </div>
      )}

      {isLoading && <Spinner />}

      {isError && (
        <div className="text-center py-12">
          <p className="text-sm text-red-500">{(error as Error)?.message || '搜索失败'}</p>
          <button
            type="button"
            onClick={() => refetch()}
            className="mt-3 text-sm text-purple-600 hover:text-purple-700 font-medium"
          >
            重试
          </button>
        </div>
      )}

      {!isLoading && !isError && allPhotos.length === 0 && data && (
        <div className="text-center py-16">
          <svg
            className="w-12 h-12 mx-auto text-gray-300 mb-4"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={1.5}
              d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
            />
          </svg>
          <p className="text-sm text-gray-500">未找到匹配的照片，试试其他关键词</p>
        </div>
      )}

      {allPhotos.length > 0 && (
        <>
          <p className="text-xs text-gray-400">
            共找到 <span className="font-medium text-gray-600">{total}</span> 张照片
            {totalPages > 1 && <>，第 {data?.pages.length}/{totalPages} 页</>}
          </p>

          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-4">
            {allPhotos.map((photo) => (
              <PhotoCard key={photo.path} photo={photo} />
            ))}
          </div>

          <div ref={sentinelRef} className="h-4" />

          {isFetchingNextPage && (
            <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-4">
              {Array.from({ length: 4 }).map((_, i) => (
                <SkeletonCard key={`loading-${i}`} />
              ))}
            </div>
          )}

          {!hasNextPage && (
            <p className="py-8 text-center text-sm text-gray-400">已加载全部照片</p>
          )}
        </>
      )}
    </div>
  );
}
