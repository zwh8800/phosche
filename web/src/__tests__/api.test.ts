import { http, HttpResponse } from 'msw';
import { setupServer } from 'msw/node';
import { describe, it, expect, beforeAll, afterAll, afterEach } from 'vitest';
import { fetchPhotos, searchPhotos } from '../api/photos';
import type { PhotoDocument, SearchRequest } from '../types';

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
  colors: ['#FF5733', '#FFC300'],
  people_count: 0,
  has_text: false,
  text: '',
  confidence: 0.95,
  exif: {
    camera_model: 'Canon EOS R5',
    iso: 100,
  },
};

const handlers = [
  http.get('http://localhost:8080/api/photos', ({ request }) => {
    const url = new URL(request.url);
    const page = Number(url.searchParams.get('page')) || 1;
    const pageSize = Number(url.searchParams.get('page_size')) || 20;
    return HttpResponse.json({
      photos: [mockPhoto],
      total: 1,
      page,
      page_size: pageSize,
    });
  }),
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

const server = setupServer(...handlers);

beforeAll(() => server.listen());
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

describe('fetchPhotos', () => {
  it('returns correctly typed data', async () => {
    const result = await fetchPhotos({ page: 1, page_size: 20 });
    expect(result.photos).toHaveLength(1);
    expect(result.photos[0].id).toBe('photo-1');
    expect(result.photos[0].description).toBe('A beautiful sunset');
    expect(result.total).toBe(1);
    expect(result.page).toBe(1);
    expect(result.page_size).toBe(20);
  });
});

describe('searchPhotos', () => {
  it('returns correctly typed search results', async () => {
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
