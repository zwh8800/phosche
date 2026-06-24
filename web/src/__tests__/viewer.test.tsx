/**
 * 图片查看器模式单元测试
 * @vitest-environment jsdom
 *
 * 测试 PhotoDetailModal 中的全屏图片查看器功能：
 * - 点击图片区域进入/退出查看模式
 * - 旋转状态递增和重置
 * - 键盘 Escape 先退出查看模式再关闭弹窗
 * - 工具栏按钮渲染
 *
 * 使用 @testing-library/react 渲染组件，
 * mock react-zoom-pan-pinch 以避免 DOM 手势 API 依赖。
 *
 * 注意：PhotoDetailModal 使用 createPortal 渲染到 document.body，
 * 所以 DOM 查询必须在 document 上进行，而非 render() 返回的 container。
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom/vitest';
import PhotoDetailModal from '../components/PhotoDetail';
import type { PhotoDocument } from '../types';

// Mock react-zoom-pan-pinch — 提供 TransformWrapper/TransformComponent/useControls 的最小实现
vi.mock('react-zoom-pan-pinch', () => ({
  TransformWrapper: ({ children }: { children: React.ReactNode }) => {
    return <div data-testid="transform-wrapper">{children}</div>;
  },
  TransformComponent: ({ children }: { children: React.ReactNode }) => {
    return <div data-testid="transform-component">{children}</div>;
  },
  useControls: () => ({
    zoomIn: vi.fn(),
    zoomOut: vi.fn(),
    resetTransform: vi.fn(),
    centerView: vi.fn(),
  }),
}));

// Mock @tanstack/react-query — useQuery 返回空数据
vi.mock('@tanstack/react-query', () => ({
  useQuery: () => ({ data: null }),
}));

// Mock react-router-dom — Link 渲染为普通 a 标签
vi.mock('react-router-dom', () => ({
  Link: ({ to, children }: { to: string; children: React.ReactNode }) => (
    <a href={to}>{children}</a>
  ),
}));

/** 构造测试用的 PhotoDocument 对象 */
function createMockPhoto(overrides: Partial<PhotoDocument> = {}): PhotoDocument {
  return {
    id: 'test-photo-1',
    path: '/photos/test/image.jpg',
    mtime: 1717000000,
    size: 2048000,
    status: 'analyzed',
    created_at: 1716900000,
    analyzed_at: 1716950000,
    description: '测试照片描述',
    tags: ['测试'],
    objects: ['物体'],
    scene_type: 'outdoor',
    colors: [{ name: '红色', hex: '#FF0000' }],
    people_count: 0,
    has_text: false,
    text: '',
    confidence: 0.9,
    exif: {
      camera_model: 'Test Camera',
      iso: 100,
    },
    ...overrides,
  };
}

/** 默认 props */
const defaultProps = {
  photo: createMockPhoto(),
  onClose: vi.fn(),
};

/**
 * 查找图片区域 div。
 * PhotoDetailModal 使用 createPortal 渲染到 document.body，
 * 所以需要在 document 上查找。
 * 图片区域是包含 img 且 className 包含 bg-black 的 div。
 */
function findImageArea(): HTMLElement | null {
  const imgs = document.querySelectorAll('img');
  for (const img of imgs) {
    let el: Element | null = img.parentElement;
    while (el) {
      if (el.tagName === 'DIV' && el.className && el.className.includes('bg-black') && el.className.includes('overflow-hidden')) {
        return el as HTMLElement;
      }
      el = el.parentElement;
    }
  }
  return null;
}

describe('PhotoDetailModal — 图片查看器模式', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('渲染详情模态框并显示图片', () => {
    render(<PhotoDetailModal {...defaultProps} />);

    // 模态框应该存在
    const dialog = screen.getByRole('dialog');
    expect(dialog).toBeInTheDocument();

    // 图片应该存在
    const img = screen.getByAltText('测试照片描述');
    expect(img).toBeInTheDocument();
  });

  it('点击图片区域进入查看模式', () => {
    render(<PhotoDetailModal {...defaultProps} />);

    // 找到图片区域
    const imageArea = findImageArea();
    expect(imageArea).toBeTruthy();

    // 点击进入查看模式
    fireEvent.click(imageArea!);

    // 验证 TransformWrapper 渲染（查看模式的标志）
    expect(screen.getByTestId('transform-wrapper')).toBeInTheDocument();

    // 验证信息面板隐藏（查看模式下有 hidden 类）
    const infoPanel = document.querySelector('[class*="lg:w-[45%]"]');
    expect(infoPanel?.className).toContain('hidden');
  });

  it('查看模式下点击图片区域退出', () => {
    render(<PhotoDetailModal {...defaultProps} />);

    const imageArea = findImageArea()!;

    // 进入查看模式
    fireEvent.click(imageArea);

    // 确认进入了查看模式
    expect(screen.getByTestId('transform-wrapper')).toBeInTheDocument();

    // 找到查看模式下的图片区域
    const viewerImageArea = findImageArea();
    expect(viewerImageArea).toBeTruthy();

    // 退出查看模式 — 点击该区域
    fireEvent.click(viewerImageArea!);

    // 验证 TransformWrapper 不再渲染（退出了查看模式）
    expect(screen.queryByTestId('transform-wrapper')).not.toBeInTheDocument();
  });

  it('查看模式下工具栏按钮渲染', () => {
    render(<PhotoDetailModal {...defaultProps} />);

    const imageArea = findImageArea()!;
    fireEvent.click(imageArea);

    // 4 个工具栏按钮应该存在
    expect(screen.getByTitle('放大')).toBeInTheDocument();
    expect(screen.getByTitle('缩小')).toBeInTheDocument();
    expect(screen.getByTitle('适应窗口')).toBeInTheDocument();
    expect(screen.getByTitle('旋转 90°（当前 0°）')).toBeInTheDocument();
  });

  it('旋转按钮点击后角度递增 90° 并循环回 0°', () => {
    render(<PhotoDetailModal {...defaultProps} />);

    const imageArea = findImageArea()!;
    fireEvent.click(imageArea);

    // 0° → 90°
    fireEvent.click(screen.getByTitle('旋转 90°（当前 0°）'));
    expect(screen.getByTitle('旋转 90°（当前 90°）')).toBeInTheDocument();

    // 90° → 180°
    fireEvent.click(screen.getByTitle('旋转 90°（当前 90°）'));
    expect(screen.getByTitle('旋转 90°（当前 180°）')).toBeInTheDocument();

    // 180° → 270°
    fireEvent.click(screen.getByTitle('旋转 90°（当前 180°）'));
    expect(screen.getByTitle('旋转 90°（当前 270°）')).toBeInTheDocument();

    // 270° → 0° (360 % 360)
    fireEvent.click(screen.getByTitle('旋转 90°（当前 270°）'));
    expect(screen.getByTitle('旋转 90°（当前 0°）')).toBeInTheDocument();
  });

  it('退出查看模式时旋转角度重置为 0°', () => {
    render(<PhotoDetailModal {...defaultProps} />);

    const imageArea = findImageArea()!;
    fireEvent.click(imageArea);

    // 旋转到 180°
    fireEvent.click(screen.getByTitle('旋转 90°（当前 0°）'));
    fireEvent.click(screen.getByTitle('旋转 90°（当前 90°）'));
    expect(screen.getByTitle('旋转 90°（当前 180°）')).toBeInTheDocument();

    // 退出查看模式
    const viewerImageArea = findImageArea()!;
    fireEvent.click(viewerImageArea);

    // 重新进入查看模式
    const restoredImageArea = findImageArea()!;
    fireEvent.click(restoredImageArea);

    // 旋转角度应该重置为 0°
    expect(screen.getByTitle('旋转 90°（当前 0°）')).toBeInTheDocument();
  });

  it('Escape 键先退出查看模式，不关闭弹窗', () => {
    const onClose = vi.fn();
    render(
      <PhotoDetailModal {...defaultProps} onClose={onClose} />,
    );

    const imageArea = findImageArea()!;
    fireEvent.click(imageArea);

    // 确认进入查看模式
    expect(screen.getByTestId('transform-wrapper')).toBeInTheDocument();

    // 按 Escape — 应退出查看模式但不关闭弹窗
    fireEvent.keyDown(document, { key: 'Escape' });

    // onClose 不应该被调用（只退出了查看模式）
    expect(onClose).not.toHaveBeenCalled();

    // 验证退出了查看模式
    expect(screen.queryByTestId('transform-wrapper')).not.toBeInTheDocument();
  });

  it('非查看模式下 Escape 键关闭弹窗', () => {
    const onClose = vi.fn();
    render(<PhotoDetailModal {...defaultProps} onClose={onClose} />);

    // 非查看模式下按 Escape — 应关闭弹窗
    fireEvent.keyDown(document, { key: 'Escape' });

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('关闭按钮始终可用', () => {
    render(<PhotoDetailModal {...defaultProps} />);

    const closeButtons = screen.getAllByLabelText('关闭');
    expect(closeButtons.length).toBeGreaterThanOrEqual(1);
  });

  it('下载按钮始终可用', () => {
    render(<PhotoDetailModal {...defaultProps} />);

    const downloadBtn = screen.getByLabelText('下载原图');
    expect(downloadBtn).toBeInTheDocument();
  });
});
