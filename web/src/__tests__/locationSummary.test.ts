/**
 * @file computeLocationSummary 单元测试
 *
 * 测试地点聚合摘要函数的正确性，重点验证：
 * - location 对象包含 city/district/province/country 字段
 * - 不同地理分布层级下的字段填充逻辑
 * - 直辖市场景（city 为空时的 fallback）
 *
 * 覆盖场景：
 * - S1: cityDistrict — 单一城市，按 district 分组
 * - S2: provinceCity — 同省多城市
 * - S3: countryProvince — 跨国场景
 * - S4: 直辖市场景（city 为空，依赖 province fallback）
 */
import { describe, it, expect } from 'vitest';
import { computeLocationSummary } from '../pages/locationSummary';
import type { PhotoDocument } from '../types';

/** 最小可用的 PhotoDocument 工厂 */
function makePhoto(overrides: Partial<PhotoDocument> = {}): PhotoDocument {
  return {
    id: 'test-id',
    path: '/photos/test.jpg',
    mtime: 1717000000,
    size: 1024000,
    status: 'analyzed',
    analyzed_at: 1717000000,
    created_at: 1717000000,
    description: 'test photo',
    tags: [],
    objects: [],
    scene_type: 'outdoor',
    colors: [],
    people_count: 0,
    has_text: false,
    confidence: 0.9,
    ...overrides,
  };
}

describe('computeLocationSummary', () => {
  describe('S1: cityDistrict — 同一城市，多个 district', () => {
    it('返回的 location 对象应携带 city, district, province, country', () => {
      const photos: PhotoDocument[] = [
        makePhoto({ city: '杭州市', district: '西湖区', province: '浙江省', country: '中国' }),
        makePhoto({ city: '杭州市', district: '西湖区', province: '浙江省', country: '中国' }),
        makePhoto({ city: '杭州市', district: '上城区', province: '浙江省', country: '中国' }),
      ];

      const result = computeLocationSummary(photos);
      expect(result).not.toBeNull();
      expect(result!.level).toBe('cityDistrict');

      // 取前 2 个地点，应按频次排序（西湖区第一，上城区第二）
      expect(result!.locations.length).toBe(2);

      const westLake = result!.locations[0];
      expect(westLake.label).toBe('杭州市·西湖区');
      expect(westLake.city).toBe('杭州市');
      expect(westLake.district).toBe('西湖区');
      expect(westLake.province).toBe('浙江省');
      expect(westLake.country).toBe('中国');

      const shangcheng = result!.locations[1];
      expect(shangcheng.label).toBe('杭州市·上城区');
      expect(shangcheng.city).toBe('杭州市');
      expect(shangcheng.district).toBe('上城区');
      expect(shangcheng.province).toBe('浙江省');
      expect(shangcheng.country).toBe('中国');
    });
  });

  describe('S2: provinceCity — 同省多城市', () => {
    it('返回的 location 对象应携带 province, city, country', () => {
      const photos: PhotoDocument[] = [
        makePhoto({ city: '杭州市', province: '浙江省', country: '中国' }),
        makePhoto({ city: '宁波市', province: '浙江省', country: '中国' }),
      ];

      const result = computeLocationSummary(photos);
      expect(result).not.toBeNull();
      expect(result!.level).toBe('provinceCity');

      expect(result!.locations.length).toBe(2);

      for (const loc of result!.locations) {
        expect(loc.province).toBe('浙江省');
        expect(loc.country).toBe('中国');
        // city 字段应当被填充
        expect(loc.city).toBeDefined();
        expect(typeof loc.city).toBe('string');
      }
    });
  });

  describe('S3: countryProvince — 跨国场景', () => {
    it('返回的 location 对象应携带 country, province, city', () => {
      const photos: PhotoDocument[] = [
        makePhoto({ city: '上海市', province: '上海市', country: '中国' }),
        makePhoto({ city: '东京', district: '千代田区', province: '东京都', country: '日本' }),
      ];

      const result = computeLocationSummary(photos);
      expect(result).not.toBeNull();
      expect(result!.level).toBe('countryProvince');

      for (const loc of result!.locations) {
        expect(loc.country).toBeDefined();
        // city 应当有值
        expect(loc.city).toBeDefined();
      }
    });
  });

  describe('S4: 直辖市场景 — city 为空，fallback 到 province', () => {
    it('city 字段为 undefined，province 和 district 仍正常填充', () => {
      const photos: PhotoDocument[] = [
        makePhoto({ city: '', district: '朝阳区', province: '北京市', country: '中国' }),
        makePhoto({ city: '', district: '朝阳区', province: '北京市', country: '中国' }),
        makePhoto({ city: '', district: '海淀区', province: '北京市', country: '中国' }),
      ];

      const result = computeLocationSummary(photos);
      expect(result).not.toBeNull();
      // 直辖市的 city 字段为空，effectiveCity fallback 到 province（北京市）
      // 所有照片的 effectiveCity 相同 → cityDistrict
      expect(result!.level).toBe('cityDistrict');

      // 北京市·朝阳区：city 在原始数据中为空
      const beijing = result!.locations[0];
      expect(beijing.label).toContain('朝阳区');
      // 直辖市的 city 字段为空，返回 undefined
      expect(beijing.city).toBeUndefined();
      expect(beijing.district).toBe('朝阳区');
      expect(beijing.province).toBe('北京市');
      expect(beijing.country).toBe('中国');
    });
  });

  describe('边界条件', () => {
    it('无有效地点数据时返回 null', () => {
      const photos: PhotoDocument[] = [
        makePhoto({ city: '', district: '', province: '', country: '' }),
      ];

      const result = computeLocationSummary(photos);
      expect(result).toBeNull();
    });
  });
});
