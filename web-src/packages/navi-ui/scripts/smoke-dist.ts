// Type-level smoke test for the built standalone artifact.
//
// Typechecked by `tsconfig.smoke.json` against the *generated* declarations in
// dist/ (via the relative `../dist/index.js` specifier, which resolves to
// `../dist/index.d.ts`). This proves an external consumer gets working types
// from the published package, not just runnable JS. Never executed.
import {
  Button,
  naviSpecEngine,
  createOpenUIEngine,
  type ButtonProps,
  type NaviUINode,
  type GenUIEngine,
} from '../dist/index.js';

// Values are typed.
const _button: typeof Button = Button;
const _engine: GenUIEngine = naviSpecEngine;
const _openui: GenUIEngine = createOpenUIEngine();

// Public types are usable.
const _node: NaviUINode = { t: 'text', value: 'typed' };
const _props: ButtonProps = { variant: 'primary' };

const parsed = naviSpecEngine.parse(_node);
export const distTypesOk: boolean =
  typeof _button === 'function' && _engine.name.length > 0 && _openui.name === 'openui' && parsed.ok;

void _props;
