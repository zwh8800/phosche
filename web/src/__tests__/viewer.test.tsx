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
import { act, render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom/vitest';
import PhotoDetailModal from '../components/PhotoDetail';
import type { PhotoDocument } from '../types';

// Mock 状态共享：TransformWrapper 捕获 onTransform，centerView/resetTransform 触发它，
// 模拟真实 react-zoom-pan-pinch 的变换回调行为
const mocks = vi.hoisted(() => {
  const state = {
    onTransform: null as
      | null
      | ((ref: unknown, s: { scale: number; positionX: number; positionY: number }) => void),
  };
  return {
    state,
    centerView: vi.fn((scale: number) => {
      state.onTransform?.(null, { scale, positionX: 0, positionY: 0 });
    }),
    resetTransform: vi.fn(() => {
      state.onTransform?.(null, { scale: 1, positionX: 0, positionY: 0 });
    }),
  };
});

// Mock react-zoom-pan-pinch — 提供 TransformWrapper/TransformComponent/useControls 的最小实现
vi.mock('react-zoom-pan-pinch', () => ({
  TransformWrapper: ({
    children,
    onTransform,
  }: {
    children: React.ReactNode;
    onTransform?: (ref: unknown, s: { scale: number; positionX: number; positionY: number }) => void;
  }) => {
    mocks.state.onTransform = onTransform ?? null;
    return <div data-testid="transform-wrapper">{children}</div>;
  },
  TransformComponent: ({ children }: { children: React.ReactNode }) => {
    return <div data-testid="transform-component">{children}</div>;
  },
  useControls: () => ({
    zoomIn: vi.fn(),
    zoomOut: vi.fn(),
    resetTransform: mocks.resetTransform,
    centerView: mocks.centerView,
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
    expect(screen.getByTitle('实际大小（1:1）')).toBeInTheDocument();
    expect(screen.getByTitle('旋转 90°（当前 0°）')).toBeInTheDocument();
  });

  it('第三个按钮在适应窗口与 1:1 实际大小之间切换', () => {
    render(<PhotoDetailModal {...defaultProps} />);

    const imageArea = findImageArea()!;
    fireEvent.click(imageArea);

    // 默认适应窗口，按钮提示为「实际大小（1:1）」
    const toggleBtn = screen.getByTitle('实际大小（1:1）');
    expect(toggleBtn).toBeInTheDocument();

    // mock 图片尺寸：显示宽度 400px，自然宽度 1600px → 1:1 缩放比为 4
    const img = screen.getByAltText('测试照片描述');
    Object.defineProperty(img, 'offsetWidth', { value: 400, configurable: true });
    Object.defineProperty(img, 'naturalWidth', { value: 1600, configurable: true });

    // 点击切换到 1:1，centerView 应以 4 倍缩放被调用（画面保持居中）
    fireEvent.click(toggleBtn);
    expect(mocks.centerView).toHaveBeenCalledWith(4, 200, 'easeOut');

    // onTransform 同步后按钮变为「适应窗口」
    expect(screen.getByTitle('适应窗口')).toBeInTheDocument();

    // 再次点击恢复适应窗口
    fireEvent.click(screen.getByTitle('适应窗口'));
    expect(mocks.resetTransform).toHaveBeenCalled();
    expect(screen.getByTitle('实际大小（1:1）')).toBeInTheDocument();
  });

  it('鼠标悬停在工具栏上时 3 秒后工具栏不消失，移出后 3 秒消失', () => {
    vi.useFakeTimers();
    // jsdom 环境 window 上自带 ontouchstart，会被组件判定为触屏设备（触屏下工具栏不自动隐藏），
    // 此处删除该属性以模拟桌面非触屏环境
    delete (window as { ontouchstart?: unknown }).ontouchstart;
    try {
      render(<PhotoDetailModal {...defaultProps} />);

      const imageArea = findImageArea()!;
      fireEvent.click(imageArea);

      // 工具栏容器是包含工具按钮的外层 div
      const toolbar = screen.getByTitle('实际大小（1:1）').parentElement!;
      expect(toolbar.className).toContain('opacity-100');

      // 鼠标进入工具栏（React 的 onMouseEnter 由 mouseover 事件合成）
      fireEvent.mouseOver(toolbar);

      // 超过 3 秒，工具栏仍应可见
      act(() => vi.advanceTimersByTime(5000));
      expect(toolbar.className).toContain('opacity-100');

      // 鼠标移出工具栏，3 秒后应自动隐藏
      fireEvent.mouseOut(toolbar);
      act(() => vi.advanceTimersByTime(3000));
      expect(toolbar.className).toContain('opacity-0');
    } finally {
      // 恢复 ontouchstart，避免影响同文件其他用例
      (window as { ontouchstart?: unknown }).ontouchstart = null;
      vi.useRealTimers();
    }
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
