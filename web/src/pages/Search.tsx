import {
  useState,
  useEffect,
  useCallback,
  useRef,
  memo,
  type FormEvent,
  type ChangeEvent,
} from 'react';
import { useSearchParams, Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { searchPhotos, fetchFilters } from '../api/photos';
import type {
  SearchResponse,
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
          src={`/photos/${photo.path.replace(/^\/+/, '')}`}
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
      </div>
    </Link>
  );
});

export default function Search() {
  const [searchParams, setSearchParams] = useSearchParams();

  const [query, setQuery] = useState(searchParams.get('query') || '');
  const [dateFrom, setDateFrom] = useState(
    searchParams.get('date_from') || '',
  );
  const [dateTo, setDateTo] = useState(searchParams.get('date_to') || '');
  const [sceneType, setSceneType] = useState(
    searchParams.get('scene_type') || '',
  );
  const [cameraModel, setCameraModel] = useState(
    searchParams.get('camera_model') || '',
  );
  const [selectedTags, setSelectedTags] = useState<string[]>(() => {
    const raw = searchParams.get('tags');
    return raw ? raw.split(',').filter(Boolean) : [];
  });

  const [showFilters, setShowFilters] = useState(false);
  const [results, setResults] = useState<SearchResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const initialMount = useRef(true);
  const scrollPositionRef = useRef<number>(0);

  const { data: filters } = useQuery<FiltersResponse>({
    queryKey: ['filters'],
    queryFn: fetchFilters,
    staleTime: 5 * 60 * 1000,
  });

  const buildParams = useCallback((): URLSearchParams => {
    const p = new URLSearchParams();
    if (query) p.set('query', query);
    if (dateFrom) p.set('date_from', dateFrom);
    if (dateTo) p.set('date_to', dateTo);
    if (sceneType) p.set('scene_type', sceneType);
    if (cameraModel) p.set('camera_model', cameraModel);
    if (selectedTags.length > 0) p.set('tags', selectedTags.join(','));
    return p;
  }, [query, dateFrom, dateTo, sceneType, cameraModel, selectedTags]);

  const doSearch = useCallback(
    async (page: number, append: boolean) => {
      setLoading(true);
      setError(null);
      try {
        const resp = await searchPhotos({
          query: query || undefined,
          date_from: dateFrom || undefined,
          date_to: dateTo || undefined,
          tags: selectedTags.length > 0 ? selectedTags : undefined,
          scene_type: sceneType || undefined,
          camera_model: cameraModel || undefined,
          page,
          page_size: PAGE_SIZE,
        });

        setResults((prev) => {
          if (append && prev) {
            return { ...resp, hits: [...prev.hits, ...resp.hits] };
          }
          return resp;
        });
      } catch (err) {
        setError(err instanceof Error ? err.message : '搜索失败，请重试');
      } finally {
        setLoading(false);
      }
    },
    [query, dateFrom, dateTo, sceneType, cameraModel, selectedTags],
  );

  const handleSearch = useCallback(
    (e?: FormEvent) => {
      e?.preventDefault();
      setSearchParams(buildParams(), { replace: true });
      doSearch(1, false);
    },
    [buildParams, doSearch, setSearchParams],
  );

  useEffect(() => {
    if (initialMount.current) {
      initialMount.current = false;
      if (searchParams.toString()) {
        doSearch(1, false);
      }
      return;
    }

    const timer = setTimeout(() => {
      setSearchParams(buildParams(), { replace: true });
      doSearch(1, false);
    }, 300);
    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dateFrom, dateTo, sceneType, cameraModel, selectedTags]);

  // Restore scroll position after results update (for load more)
  useEffect(() => {
    if (scrollPositionRef.current > 0) {
      window.scrollTo(0, scrollPositionRef.current);
      scrollPositionRef.current = 0;
    }
  }, [results]);

  const handleLoadMore = useCallback(() => {
    if (!results || loading) return;
    const nextPage = results.page + 1;
    if (nextPage > results.total_pages) return;
    
    // Save scroll position before loading more
    scrollPositionRef.current = window.scrollY;
    doSearch(nextPage, true);
  }, [results, loading, doSearch]);

  const toggleTag = useCallback((tag: string) => {
    setSelectedTags((prev) =>
      prev.includes(tag) ? prev.filter((t) => t !== tag) : [...prev, tag],
    );
  }, []);

  const resetFilters = useCallback(() => {
    setDateFrom('');
    setDateTo('');
    setSceneType('');
    setCameraModel('');
    setSelectedTags([]);
    setQuery('');
    setSearchParams({}, { replace: true });
    setResults(null);
    setError(null);
  }, [setSearchParams]);

  const activeFilterCount =
    (dateFrom ? 1 : 0) +
    (dateTo ? 1 : 0) +
    (sceneType ? 1 : 0) +
    (cameraModel ? 1 : 0) +
    selectedTags.length;

  const hasMore =
    results != null && results.page < results.total_pages;

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
            onChange={(e: ChangeEvent<HTMLInputElement>) =>
              setQuery(e.target.value)
            }
            placeholder="输入关键词搜索照片…"
            className="w-full pl-9 pr-4 py-2.5 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent bg-white placeholder-gray-400"
          />
        </div>
        <button
          type="submit"
          disabled={loading}
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
            showFilters
              ? 'text-purple-600'
              : 'text-gray-600 hover:text-gray-900'
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
                onChange={(e: ChangeEvent<HTMLInputElement>) =>
                  setDateFrom(e.target.value)
                }
                className="flex-1 px-3 py-2 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent bg-white"
              />
              <span className="text-xs text-gray-400">至</span>
              <input
                type="date"
                value={dateTo}
                onChange={(e: ChangeEvent<HTMLInputElement>) =>
                  setDateTo(e.target.value)
                }
                className="flex-1 px-3 py-2 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent bg-white"
              />
            </div>
          </div>

          <div>
            <label
              htmlFor="filter-scene"
              className="block text-xs font-medium text-gray-500 mb-2"
            >
              场景类型
            </label>
            <select
              id="filter-scene"
              value={sceneType}
              onChange={(e: ChangeEvent<HTMLSelectElement>) =>
                setSceneType(e.target.value)
              }
              className="w-full px-3 py-2 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent bg-white"
            >
              <option value="">全部</option>
              {(filters?.scene_types || []).map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label
              htmlFor="filter-camera"
              className="block text-xs font-medium text-gray-500 mb-2"
            >
              相机型号
            </label>
            <select
              id="filter-camera"
              value={cameraModel}
              onChange={(e: ChangeEvent<HTMLSelectElement>) =>
                setCameraModel(e.target.value)
              }
              className="w-full px-3 py-2 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent bg-white"
            >
              <option value="">全部</option>
              {(filters?.cameras || []).map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-xs font-medium text-gray-500 mb-2">
              标签
            </label>
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

      {loading && !results && <Spinner />}

      {error && (
        <div className="text-center py-12">
          <p className="text-sm text-red-500">{error}</p>
          <button
            type="button"
            onClick={() => doSearch(1, false)}
            className="mt-3 text-sm text-purple-600 hover:text-purple-700 font-medium"
          >
            重试
          </button>
        </div>
      )}

      {!loading && !error && results && results.hits.length === 0 && (
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
          <p className="text-sm text-gray-500">
            未找到匹配的照片，试试其他关键词
          </p>
        </div>
      )}

      {!loading && !error && results && results.hits.length > 0 && (
        <>
          <p className="text-xs text-gray-400">
            共找到{' '}
            <span className="font-medium text-gray-600">{results.total}</span>{' '}
            张照片
            {results.total_pages > 1 && (
              <>
                ，第 {results.page}/{results.total_pages} 页
              </>
            )}
          </p>

          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-4">
            {results.hits.map((photo) => (
              <PhotoCard
                key={photo.path}
                photo={photo}
              />
            ))}
          </div>

          {hasMore && (
            <div className="flex justify-center pt-2 pb-4">
              <button
                type="button"
                onClick={handleLoadMore}
                disabled={loading}
                className="px-6 py-2 text-sm font-medium text-purple-600 bg-purple-50 hover:bg-purple-100 disabled:opacity-50 rounded-lg transition-colors"
              >
                {loading ? '加载中…' : '加载更多'}
              </button>
            </div>
          )}

          {loading && results && <Spinner />}
        </>
      )}
    </div>
  );
}
