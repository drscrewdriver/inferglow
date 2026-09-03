/**
 * sidebar-enhance 插件入口。
 *
 * 导出：
 *  - registerSidebarEnhancePlugin()：注册 `sidebar.brand.mark` / `sidebar.brand.name`
 *    两个槽位的「默认品牌行」occupants（BrandMark ◈ Logo + BrandName "InferGlow"），
 *    返回注销函数。
 *  - SidebarEnhance：折叠/展开 + crossfade/slide + 滚动条 linger 的增强容器组件。
 *  - BrandRow：读取品牌槽位组合渲染的品牌行组件。
 */
import { registerSlot } from '../registry'
import { SidebarEnhance } from './SidebarEnhance'
import { BrandRow, BrandMark, BrandName } from './BrandRow'

export { SidebarEnhance, BrandRow }

/** 注册默认品牌行槽位（sidebar.brand.mark + sidebar.brand.name）。 */
export function registerSidebarEnhancePlugin(): () => void {
  const unmark = registerSlot(
    'sidebar.brand.mark',
    BrandMark,
    { id: 'sidebar-enhance:brand-mark', order: 0 },
  )
  const unname = registerSlot(
    'sidebar.brand.name',
    BrandName,
    { id: 'sidebar-enhance:brand-name', order: 0 },
  )
  return () => {
    unmark()
    unname()
  }
}