/**
 * @file API 客户端单元测试。
 *
 * 使用 msw (Mock Service Worker) 拦截 HTTP 请求，
 * 测试 photos.ts 中的 API 调用函数。
 *
 * 测试策略：
 * - 使用 msw 模拟后端 API 响应，避免真实网络请求
 * - 测试正常响应的数据类型和字段正确性
 * - 测试参数传递（分页、搜索条件等）
 * - 覆盖错误场景（404 不存在的照片 ID）
 *
 * 覆盖范围：
 * - fetchPhotos：获取照片时间线列表
 * - searchPhotos：全文搜索照片
 * - fetchPhotoDetail：获取单张照片详情（含 404 错误处理）
 */

import { http, HttpResponse } from 'msw';
import { setupServer } from 'msw/node';
import { describe, it, expect, beforeAll, afterAll, afterEach } from 'vitest';
import { fetchPhotos, searchPhotos, fetchPhotoDetail } from '../api/photos';
import type { PhotoDocument, SearchRequest } from '../types';

/** 测试用的模拟照片数据 */
const mockPhoto: PhotoDocument = {
  id: 'photo-1',
  path: '/photos/test.jpg',
  mtime: 1717000000,
  size: 1024000,
  status: 'analyzed',
  created_at: 1716900000,
  analyzed_at: 1716950000,
  description: 'A beautiful sunset',
  tags: ['sunset', 'landscape'],
  objects: ['sun', 'ocean'],
  scene_type: 'outdoor',
  colors: [{ name: '橙红', hex: '#FF5733' }, { name: '金黄', hex: '#FFC300' }],
  people_count: 0,
  has_text: false,
  text: '',
  confidence: 0.95,
  exif: {
    camera_model: 'Canon EOS R5',
    iso: 100,
  },
};

/** msw 请求处理器，模拟后端 API 响应 */
const handlers = [
  /** 照片详情接口：根据 ID 返回照片或 404 */
  http.get('http://localhost:8080/api/photos/:id', ({ params }) => {
    if (params.id === 'abc123def456') {
      return HttpResponse.json(mockPhoto);
    }
    return new HttpResponse(null, { status: 404 });
  }),
  /** 照片列表接口：返回分页的照片列表 */
  http.get('http://localhost:8080/api/photos', ({ request }) => {
    const url = new URL(request.url);
    const page = Number(url.searchParams.get('page')) || 1;
    const pageSize = Number(url.searchParams.get('page_size')) || 20;
    return HttpResponse.json({
      hits: [mockPhoto],
      total: 1,
      page,
      page_size: pageSize,
    });
  }),
  /** 搜索接口：根据请求体返回搜索结果 */
  http.post('http://localhost:8080/api/search', async ({ request }) => {
    const body = (await request.json()) as SearchRequest;
    return HttpResponse.json({
      hits: [mockPhoto],
      total: 1,
      page: body.page,
      page_size: body.page_size,
      total_pages: 1,
    });
  }),
];

/** 创建 msw 测试服务器 */
const server = setupServer(...handlers);

beforeAll(() => server.listen());
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

describe('照片列表 API', () => {
  describe('fetchPhotos', () => {
    it('应返回正确类型的照片列表数据', async () => {
      const result = await fetchPhotos({ page: 1, page_size: 20 });
      expect(result.photos).toHaveLength(1);
      expect(result.photos[0].id).toBe('photo-1');
      expect(result.photos[0].description).toBe('A beautiful sunset');
      expect(result.total).toBe(1);
      expect(result.page).toBe(1);
      expect(result.page_size).toBe(20);
    });
  });
});

describe('照片搜索 API', () => {
  describe('searchPhotos', () => {
    it('应返回正确类型的搜索结果', async () => {
      const req: SearchRequest = {
        query: 'sunset',
        page: 1,
        page_size: 10,
      };
      const result = await searchPhotos(req);
      expect(result.hits).toHaveLength(1);
      expect(result.hits[0].id).toBe('photo-1');
      expect(result.total).toBe(1);
      expect(result.total_pages).toBe(1);
    });
  });
});

describe('照片详情 API', () => {
  describe('fetchPhotoDetail', () => {
    it('应根据 ID 返回照片详情', async () => {
      const result = await fetchPhotoDetail('abc123def456');
      expect(result.id).toBe('photo-1');
      expect(result.description).toBe('A beautiful sunset');
    });

    it('应在照片不存在时抛出错误', async () => {
      await expect(fetchPhotoDetail('nonexistent')).rejects.toThrow();
    });
  });
});
