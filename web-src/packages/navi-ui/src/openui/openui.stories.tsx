import { GenUI } from '../genui';
import { createNaviOpenUIEngine } from './index';

export default { title: 'GenUI / OpenUI Lang' };

const engine = createNaviOpenUIEngine();

const PROGRAM = [
  'root = Card("Create project", [intro, form, tags])',
  'intro = Text("Rendered from OpenUI Lang through NAVI primitives.", "secondary")',
  'form = Form("create_project", [titleField], "Create")',
  'titleField = Field("title", "Project name", "e.g. Apollo")',
  'tags = Stack([act, draft], "row", "sm")',
  'act = Badge("ACT", "running")',
  'draft = Badge("draft", "idle")',
].join('\n');

export const RenderedThroughNaviPrimitives = () => <GenUI engine={engine} source={PROGRAM} onAction={() => {}} />;
