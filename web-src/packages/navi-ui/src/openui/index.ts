// @navi/ui/openui — opt-in OpenUI Lang integration.
//
//   import { createNaviOpenUIEngine } from '@navi/ui/openui';
//   import { GenUI } from '@navi/ui';
//   <GenUI engine={createNaviOpenUIEngine()} source={openUILangText} streaming />
//
// Requires the optional peer `@openuidev/react-lang`. The CORE library
// (`@navi/ui`) never imports this subpath, so apps that don't use OpenUI pay
// nothing and the core build stays dependency-free.

export { createNaviOpenUIAdapter, createNaviOpenUIEngine } from './adapter';
export { mapOpenUIElement } from './map';
export {
  naviOpenUISchema,
  naviOpenUISystemPrompt,
  NAVI_OPENUI_ROOT,
  NAVI_OPENUI_COMPONENTS,
} from './library';
