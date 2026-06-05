/**
 * 地点聚合摘要工具函数
 *
 * 从日期组内的照片数据中提取地点分布信息，
 * 用于 Timeline 页面展示地点聚合标签和生成搜索链接参数。
 */
import type { PhotoDocument } from '../types';

/** 地点展示层级 */
export type LocationHierarchyLevel = 'cityDistrict' | 'provinceCity' | 'countryProvince';

/** 地点聚合结果 */
export interface LocationSummary {
  level: LocationHierarchyLevel;
  locations: { label: string; count: number; city?: string; district?: string; province?: string; country?: string }[];
}

/**
 * 根据日期组内所有位置数据的地理分布，决定展示层级
 */
export function determineHierarchyLevel(
  uniqueCities: Set<string>,
  uniqueCountries: Set<string>,
): LocationHierarchyLevel {
  if (uniqueCities.size <= 1) return 'cityDistrict';
  if (uniqueCountries.size > 1) return 'countryProvince';
  return 'provinceCity';
}

/**
 * 计算日期组的地点聚合摘要
 * - 按 (city, district) 组合统计出现频次
 * - 直辖市（city 为空）时 fallback 到 province 作为 effectiveCity
 * - 根据所有唯一城市/国家的分布决定展示层级
 * - 取频次最高的前 2 个地点
 * - 每个地点对象携带 city/district/province/country 字段，
 *   用于构建搜索页链接的 URL 参数
 */
export function computeLocationSummary(photos: PhotoDocument[]): LocationSummary | null {
  const freq = new Map<string, number>();
  const uniqueCountries = new Set<string>();
  const uniqueCities = new Set<string>();

  for (const p of photos) {
    // 直辖市场景下高德 API 的 city 字段为空，fallback 到 province
    const effectiveCity = p.city || p.province || '';
    const district = p.district || '';
    if (!effectiveCity && !district) continue;
    const key = `${effectiveCity}|${district}`;
    freq.set(key, (freq.get(key) ?? 0) + 1);
    if (effectiveCity) uniqueCities.add(effectiveCity);
    if (p.country) uniqueCountries.add(p.country);
  }

  if (freq.size === 0) return null;

  const level = determineHierarchyLevel(uniqueCities, uniqueCountries);

  const top = [...freq.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, 2);

  const findPhotoByKey = (cityPart: string, districtPart: string): PhotoDocument | undefined =>
    photos.find((p) =>
      (p.city || p.province || '') === cityPart && (p.district || '') === districtPart,
    );

  const locations = top.map(([key]) => {
    const [cityPart, districtPart] = key.split('|');
    const sample = findPhotoByKey(cityPart, districtPart);
    switch (level) {
      case 'cityDistrict':
        return {
          label: districtPart ? `${cityPart}·${districtPart}` : cityPart,
          count: freq.get(key)!,
          city: sample?.city || undefined,
          district: districtPart || undefined,
          province: sample?.province,
          country: sample?.country,
        };
      case 'provinceCity': {
        // 直辖市 fallback 时 province === cityPart，避免重复显示
        const prov = sample?.province;
        const label = !prov || prov === cityPart ? cityPart : `${prov}·${cityPart}`;
        return {
          label,
          count: freq.get(key)!,
          city: sample?.city || undefined,
          district: districtPart || undefined,
          province: sample?.province,
          country: sample?.country,
        };
      }
      case 'countryProvince': {
        // 用 Set 去重，避免 "中国·北京市·北京市" 这种重复
        const parts = [sample?.country, sample?.province, cityPart].filter(Boolean);
        return {
          label: [...new Set(parts)].join('·'),
          count: freq.get(key)!,
          city: sample?.city || undefined,
          district: districtPart || undefined,
          province: sample?.province,
          country: sample?.country,
        };
      }
    }
  });

  return { level, locations };
}
