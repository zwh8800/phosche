import { useInfiniteQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { useRef, useEffect, useMemo } from 'react';
import { fetchPhotos } from '../api/photos';
import type { PhotoDocument } from '../types';

function extractDate(photo: PhotoDocument): string {
  const exifDate = photo.exif?.date_time_original;
  if (exifDate) {
    const d = new Date(exifDate);
    if (!Number.isNaN(d.getTime())) return d.toISOString().slice(0, 10);
  }
  // Auto-detect unix-seconds vs milliseconds
  const ts =
    photo.created_at > 1e12 ? photo.created_at : photo.created_at * 1000;
  const d = new Date(ts);
  if (!Number.isNaN(d.getTime())) return d.toISOString().slice(0, 10);
  return '未知日期';
}

function formatDateLabel(dateStr: string): string {
  const parts = dateStr.split('-');
  if (parts.length !== 3) return dateStr;
  return `${parts[0]}年${Number.parseInt(parts[1], 10)}月${Number.parseInt(parts[2], 10)}日`;
}

function photoSrc(path: string): string {
  return `/photos/${path.replace(/^\/+/, '')}`;
}

function SkeletonCard() {
  return (
    <div className="animate-pulse">
      <div className="aspect-square rounded-lg bg-gray-200" />
      <div className="mt-2 space-y-1.5">
        <div className="h-3 w-3/4 rounded bg-gray-200" />
        <div className="h-3 w-1/2 rounded bg-gray-200" />
      </div>
    </div>
  );
}

function PhotoCard({ photo }: { photo: PhotoDocument }) {
  const navigate = useNavigate();

  return (
    <button
      type="button"
      onClick={() => navigate(`/photo/${encodeURIComponent(photo.path)}`)}
      className="group cursor-pointer text-left"
    >
      <div className="aspect-square overflow-hidden rounded-lg bg-gray-100">
        <img
          src={photoSrc(photo.path)}
          alt={photo.description || '照片'}
          className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
          loading="lazy"
        />
      </div>

      <div className="mt-2 space-y-1.5">
        {photo.description && (
          <p className="line-clamp-2 text-sm leading-snug text-gray-700">
            {photo.description}
          </p>
        )}
        {photo.tags && photo.tags.length > 0 && (
          <div className="flex flex-wrap gap-1">
            {photo.tags.slice(0, 3).map((tag) => (
              <span
                key={tag}
                className="inline-block rounded bg-purple-50 px-1.5 py-0.5 text-xs text-purple-700"
              >
                {tag}
              </span>
            ))}
            {photo.tags.length > 3 && (
              <span className="text-xs text-gray-400">
                +{photo.tags.length - 3}
              </span>
            )}
          </div>
        )}
      </div>
    </button>
  );
}

export default function Timeline() {
  const sentinelRef = useRef<HTMLDivElement>(null);
  const scrollPositionRef = useRef<number>(0);

  const {
    data,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    isLoading,
    isError,
    error,
  } = useInfiniteQuery({
    queryKey: ['photos'],
    queryFn: ({ pageParam = 1 }) =>
      fetchPhotos({ page: pageParam, page_size: 50 }),
    getNextPageParam: (lastPage) => {
      const totalPages = Math.ceil(lastPage.total / lastPage.page_size);
      return lastPage.page < totalPages ? lastPage.page + 1 : undefined;
    },
    initialPageParam: 1,
  });

  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasNextPage && !isFetchingNextPage) {
          scrollPositionRef.current = window.scrollY;
          fetchNextPage();
        }
      },
      { threshold: 0.1 },
    );

    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  useEffect(() => {
    if (scrollPositionRef.current > 0 && !isFetchingNextPage) {
      window.scrollTo(0, scrollPositionRef.current);
      scrollPositionRef.current = 0;
    }
  }, [data, isFetchingNextPage]);

  const groupedPhotos = useMemo(() => {
    const allPhotos = data?.pages.flatMap((page) => page.photos) ?? [];
    const groups = new Map<string, PhotoDocument[]>();

    for (const photo of allPhotos) {
      const dateStr = extractDate(photo);
      const bucket = groups.get(dateStr);
      if (bucket) {
        bucket.push(photo);
      } else {
        groups.set(dateStr, [photo]);
      }
    }

    return [...groups.entries()].sort(([a], [b]) => b.localeCompare(a));
  }, [data]);

  if (isError) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-gray-500">
        <svg
          className="mb-4 h-16 w-16 text-gray-300"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={1.5}
            d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4.5c-.77-.833-2.694-.833-3.464 0L3.34 16.5c-.77.833.192 2.5 1.732 2.5z"
          />
        </svg>
        <p className="text-lg font-medium">加载失败</p>
        <p className="mt-1 text-sm">
          {(error as Error)?.message || '请稍后重试'}
        </p>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-4">
        {Array.from({ length: 6 }).map((_, i) => (
          <SkeletonCard key={i} />
        ))}
      </div>
    );
  }

  if (groupedPhotos.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-gray-400">
        <svg
          className="mb-6 h-24 w-24 text-gray-200"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={1}
            d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"
          />
        </svg>
        <p className="text-xl font-medium text-gray-500">还没有照片</p>
        <p className="mt-2 text-sm text-gray-400">
          导入照片后将在此处按时间线展示
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      {groupedPhotos.map(([dateStr, photos]) => (
        <section key={dateStr}>
          <h2 className="sticky top-0 z-10 mb-4 bg-gray-50/90 py-2 text-lg font-semibold text-gray-800 backdrop-blur-sm">
            {formatDateLabel(dateStr)}
          </h2>
          <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-4">
            {photos.map((photo) => (
              <PhotoCard key={photo.id} photo={photo} />
            ))}
          </div>
        </section>
      ))}

      <div ref={sentinelRef} className="h-4" />

      {isFetchingNextPage && (
        <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <SkeletonCard key={`loading-${i}`} />
          ))}
        </div>
      )}

      {!hasNextPage && groupedPhotos.length > 0 && (
        <p className="py-8 text-center text-sm text-gray-400">已加载全部照片</p>
      )}
    </div>
  );
}
