/**
 * BrandRow —— 品牌行（Logo + 名称）。
 *
 * 通过 renderSlot 读取 `sidebar.brand.mark` / `sidebar.brand.name` 两个槽位，
 * 组合渲染成一行品牌标识。两个槽位由 registerSidebarEnhancePlugin() 默认注册
 * （◈ Logo + "InferGlow"），其他插件可覆盖或追加 occupants。
 */
import { renderSlot } from '../registry'
import s from './sidebar-enhance.module.css'

export interface BrandRowProps {
  /** 折叠态下仅显示 logo（rail 场景） */
  collapsed?: boolean
}

/** 默认品牌 Logo 占用（sidebar.brand.mark 槽位默认 occupants）。 */
export function BrandMark() {
  return <span>◈</span>
}

/** 默认品牌名称占用（sidebar.brand.name 槽位默认 occupants）。 */
export function BrandName() {
  return <span>InferGlow</span>
}

export function BrandRow({ collapsed }: BrandRowProps) {
  const marks = renderSlot('sidebar.brand.mark')
  const names = renderSlot('sidebar.brand.name')

  if (collapsed) {
    return <div className={s.railMark}>{marks}</div>
  }

  return (
    <div className={s.brandRow}>
      <span className={s.brandMark}>{marks}</span>
      <span className={s.brandName}>{names}</span>
    </div>
  )
}