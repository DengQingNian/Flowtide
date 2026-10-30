// BPMN 图形相关共享类型。

/** 活动高亮类型 */
export type ActivityHighlightType = 'completed' | 'active' | 'failed'

/** 活动高亮项 */
export interface ActivityHighlight {
  /** BPMN 元素 ID */
  id: string
  /** 高亮类型 */
  type: ActivityHighlightType
}
