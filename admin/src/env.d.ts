/// <reference types="vite/client" />

declare const __ADMIN_VERSION__: string
declare const __ADMIN_COMMIT__: string
declare const __ADMIN_BUILD_TIME__: string

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, unknown>
  export default component
}
declare module 'bpmn-js-properties-panel' {
  export const BpmnPropertiesPanelModule: Record<string, unknown>
  export const BpmnPropertiesProviderModule: Record<string, unknown>
}
