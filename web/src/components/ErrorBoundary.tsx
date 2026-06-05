/**
 * React 错误边界组件
 *
 * 使用类组件的 getDerivedStateFromError 生命周期方法捕获渲染错误，
 * 防止整个应用白屏崩溃，同时向用户展示友好的错误提示界面。
 *
 * 错误边界只能捕获以下场景的异常：
 * - 渲染期间（render 方法）
 * - 生命周期方法中
 * - 构造函数中
 *
 * 无法捕获以下场景：
 * - 事件处理器中的错误（需使用 try-catch）
 * - 异步代码中的错误
 * - 服务端渲染错误
 * - 错误边界自身抛出的错误
 */
import { Component, type ReactNode } from 'react';
import { Link } from 'react-router-dom';

/** ErrorBoundary 组件的属性类型 */
interface Props {
  /** 需要被错误边界包裹的子组件树 */
  children: ReactNode;
}

/** ErrorBoundary 组件的内部状态类型 */
interface State {
  /** 是否发生了未被捕获的渲染错误 */
  hasError: boolean;
  /** 捕获到的错误对象，用于在 UI 中展示错误信息 */
  error: Error | null;
}

/**
 * ErrorBoundary — React 错误边界类组件
 *
 * 使用类组件而非函数组件是因为 React 错误边界机制
 * 依赖于类组件的 getDerivedStateFromError 静态方法，
 * 函数组件中暂不支持此能力。
 */
class ErrorBoundary extends Component<Props, State> {
  /**
   * 构造函数，初始化状态
   * 初始状态：无错误（hasError: false, error: null）
   */
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  /**
   * 当子组件树抛出异常时，React 自动调用此静态方法
   * 返回新的状态对象，触发重新渲染以展示降级 UI
   *
   * @param error — 子组件抛出的错误对象
   * @returns 更新后的状态，标记 hasError 为 true 并保存错误信息
   */
  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  /** 刷新整个页面，尝试恢复正常渲染 */
  handleRefresh = () => {
    window.location.reload();
  };

  /**
   * 渲染方法
   * - 如果 hasError 为 true，展示错误降级 UI（警告图标 + 错误信息 + 操作按钮）
   * - 否则正常渲染子组件
   */
  render() {
    if (this.state.hasError) {
      return (
        <div className="flex min-h-screen flex-col items-center justify-center bg-gray-50 px-4 text-center">
          {/*
            警告三角形图标
            使用紫色柔化视觉效果，避免红色引起焦虑
           */}
          <svg
            className="mb-6 h-20 w-20 text-red-300"
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
          {/* 错误标题 — 简洁明了 */} 
          <h1 className="text-2xl font-semibold text-gray-800">
            出错了
          </h1>
          {/* 错误描述 — 引导用户下一步操作 */} 
          <p className="mt-2 max-w-md text-sm text-gray-500">
            页面渲染时发生错误，请尝试刷新页面。
          </p>
          {/*
            具体错误信息
            仅在存在错误对象时才渲染，用红色背景突出显示
            使用 break-all 确保长错误信息不会溢出
           */}
          {this.state.error && (
            <p className="mt-3 max-w-lg rounded-lg bg-red-50 px-4 py-3 text-xs text-red-700 break-all">
              {this.state.error.message}
            </p>
          )}
          {/*
            操作按钮组
            提供两个选项：刷新重试 / 返回首页
            水平排列，间距一致
           */}
          <div className="mt-8 flex items-center gap-3">
            {/*
              刷新按钮
              点击调用 handleRefresh，重新加载整个页面
              紫色主色调，与品牌色一致
            */}
            <button
              type="button"
              onClick={this.handleRefresh}
              className="inline-flex items-center gap-2 rounded-lg bg-red-600 px-5 py-2.5 text-sm font-medium text-white transition-colors hover:bg-red-700 cursor-pointer"
            >
              {/*
                刷新图标（旋转箭头）
                使用 SVG 以减少外部依赖
               */}
              <svg
                className="h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                strokeWidth={2}
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                />
              </svg>
              刷新页面
            </button>
            {/*
              返回首页链接
              使用 React Router 的 Link 组件，无需重新加载页面
              白色背景 + 灰色边框，作为辅助操作
            */}
            <Link
              to="/"
              className="rounded-lg bg-white px-5 py-2.5 text-sm font-medium text-gray-700 shadow-sm ring-1 ring-gray-200 transition-colors hover:bg-gray-50"
            >
              返回首页
            </Link>
          </div>
        </div>
      );
    }

    // 未发生错误时，正常渲染子组件
    return this.props.children;
  }
}

export default ErrorBoundary;
